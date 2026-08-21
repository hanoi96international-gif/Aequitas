package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// A peer's answer to /api/blocks/by-hash is read under a 20 MB cap, and
// maxBlocksByHashPerRequest is 500 because api.go sized it against blocks of
// "~2 KB each ... at 500 hashes is ~1 MB — still comfortably under the 20 MB
// client read cap".
//
// That estimate holds for near-empty blocks and breaks the moment blocks are
// full. On 2026-08-21 a throughput run put a few thousand transfers into each
// block; Contabo2 restarted, fell ~500 blocks behind, and its request for 412
// ancestors asked for hundreds of MB. The read stopped at the cap, the
// truncated body reached json.Unmarshal, and the node logged
//
//	Could not batch-fetch 412 missing ancestor(s): unexpected end of JSON input
//
// every cycle. It could never obtain the ancestors it needed to catch up,
// every later block queued as an orphan, and it stopped advancing entirely --
// while its own logs and /api/status looked healthy.
//
// These pin the two properties that stop it recurring: an oversized response
// is REPORTED as oversized instead of parsed, and the batch adapts to what
// blocks actually weigh rather than to a constant chosen years earlier.

// oversizedJSON returns syntactically valid JSON larger than the read cap.
// Valid on purpose: the point is that it is refused on size, not that it
// happens to be malformed.
func oversizedJSON() string {
	var b strings.Builder
	b.WriteString("[")
	filler := `{"hash":"0x` + strings.Repeat("a", 4000) + `"},`
	for b.Len() < blocksByHashReadCap+1024 {
		b.WriteString(filler)
	}
	b.WriteString(`{"hash":"0xend"}]`)
	return b.String()
}

func TestOversizedPeerResponseIsReportedNotParsed(t *testing.T) {
	_, err := decodeBlocksByHashResponse(strings.NewReader(oversizedJSON()))
	if err == nil {
		t.Fatal("a response past the read cap was accepted — a truncated body must never be " +
			"parsed as though it were complete")
	}
	if !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("got %v, want errResponseTooLarge.\n"+
			"  A size failure reported as a parse error is what made this cost a live validator: "+
			"'unexpected end of JSON input' gives no hint that the fix is a smaller batch.", err)
	}
}

func TestResponseUnderTheCapStillDecodes(t *testing.T) {
	body, _ := json.Marshal([]map[string]any{{"hash": "0xaa"}, {"hash": "0xbb"}})
	blocks, err := decodeBlocksByHashResponse(strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("an ordinary response was rejected: %v", err)
	}
	if len(blocks) != 2 || blocks[0].Hash != "0xaa" || blocks[1].Hash != "0xbb" {
		t.Fatalf("decoded %d blocks (%+v), want the two that were sent", len(blocks), blocks)
	}
}

func TestSplitOnOversizeHalvesUntilItFits(t *testing.T) {
	// Oversized whenever asked for more than four at once, mirroring the real
	// shape of the problem: the peer is fine, the batch is simply too heavy.
	const fits = 4
	var asked []int

	fetch := func(chunk []string) ([]*Block, bool, error) {
		asked = append(asked, len(chunk))
		if len(chunk) > fits {
			return nil, false, errResponseTooLarge
		}
		out := make([]*Block, 0, len(chunk))
		for _, h := range chunk {
			out = append(out, &Block{Hash: h})
		}
		return out, false, nil
	}

	want := make([]string, 16)
	for i := range want {
		want[i] = fmt.Sprintf("0x%02x", i)
	}

	blocks, _, err := splitOnOversize(want, "test-peer", fetch)
	if err != nil {
		t.Fatalf("splitting gave up: %v\n"+
			"  This is the whole point: a node that cannot fetch its missing ancestors never "+
			"catches up, and stops advancing while looking healthy.", err)
	}
	if len(blocks) != len(want) {
		t.Fatalf("got %d blocks, want %d — a split that loses blocks is worse than one that fails, "+
			"because the caller reads a missing block as one the peer does not have",
			len(blocks), len(want))
	}
	for i, b := range blocks {
		if b.Hash != want[i] {
			t.Fatalf("block %d is %q, want %q — the halves must be reassembled in order", i, b.Hash, want[i])
		}
	}
	if len(asked) < 2 {
		t.Fatalf("only %d attempt(s) were made; an oversized batch has to be split", len(asked))
	}
	if asked[0] != len(want) {
		t.Errorf("first attempt asked for %d, want the full %d — trying the whole batch first keeps "+
			"an ordinary catch-up at one round trip", asked[0], len(want))
	}
	for _, n := range asked {
		if n > fits && n != 16 && n != 8 {
			t.Errorf("asked for %d hashes, which is not a halving step from %d", n, len(want))
		}
	}
}

// A single block that will not fit cannot be split any further, so it has to
// be a real error rather than an endless halving.
func TestSplitOnOversizeStopsAtOneHash(t *testing.T) {
	calls := 0
	fetch := func(chunk []string) ([]*Block, bool, error) {
		calls++
		if calls > 100 {
			t.Fatal("splitting did not terminate on a single oversized block")
		}
		return nil, false, errResponseTooLarge
	}
	_, _, err := splitOnOversize([]string{"0xaa"}, "test-peer", fetch)
	if err == nil {
		t.Fatal("a single block over the cap was reported as success")
	}
	if errors.Is(err, errResponseTooLarge) {
		t.Errorf("got the raw sentinel back: %v\n"+
			"  The caller cannot act on it — it needs to know that no further split is possible.", err)
	}
	if !strings.Contains(err.Error(), "single block") {
		t.Errorf("error %q does not say a single block is the problem", err)
	}
}

// The server-side budget is the fix that actually keeps a lagging node moving.
// Splitting on the client is correct but wasteful: every failed attempt still
// transfers the full cap first, and on 2026-08-21 Contabo1 spent its entire
// bandwidth on discarded responses (327 -> 163 -> 81 -> 40 -> 21 -> 10) while
// falling further behind the whole time. Serving what fits costs one round
// trip and transfers nothing twice.

func TestResponseBudgetServesWhatFitsAndSaysItTruncated(t *testing.T) {
	// Each block ~1 MB, like a block full of transfers.
	big := strings.Repeat("c", 1<<20)
	blocks := make([]*Block, 40)
	for i := range blocks {
		blocks[i] = &Block{Hash: fmt.Sprintf("0x%02x", i), StateRoot: big}
	}

	kept, truncated := capBlocksByResponseBytes(blocks)
	if !truncated {
		t.Fatalf("40 blocks of ~1 MB fit in a %d MB budget without truncation?",
			blocksByHashResponseBudget>>20)
	}
	if len(kept) == 0 {
		t.Fatal("kept nothing — an empty answer tells the caller the peer has none of them, " +
			"and it would stop asking")
	}
	if len(kept) >= len(blocks) {
		t.Fatalf("kept all %d blocks; the budget did nothing", len(kept))
	}
	total := 2
	for _, b := range kept {
		enc, _ := json.Marshal(b)
		total += len(enc) + 1
	}
	if total > blocksByHashResponseBudget {
		t.Errorf("kept %d bytes, over the %d byte budget — the client's read cap is what this "+
			"has to stay under, and exceeding it puts us back at a truncated body",
			total, blocksByHashResponseBudget)
	}
}

func TestResponseBudgetNeverStallsOnOneHugeBlock(t *testing.T) {
	// One block bigger than the whole budget. Dropping it would leave the
	// caller asking for it forever and making no progress; keeping it lets the
	// client report an honest oversize error instead.
	huge := &Block{Hash: "0xhuge", StateRoot: strings.Repeat("d", blocksByHashResponseBudget+1024)}
	kept, _ := capBlocksByResponseBytes([]*Block{huge})
	if len(kept) != 1 {
		t.Fatalf("kept %d blocks, want the single oversized one — dropping it silently is how a "+
			"catch-up stalls on one bad block with nothing in any log to say why", len(kept))
	}
}

func TestResponseBudgetLeavesOrdinaryResponsesAlone(t *testing.T) {
	blocks := []*Block{{Hash: "0xaa"}, {Hash: "0xbb"}, {Hash: "0xcc"}}
	kept, truncated := capBlocksByResponseBytes(blocks)
	if truncated || len(kept) != 3 {
		t.Fatalf("three small blocks were truncated (kept %d, truncated=%v) — the budget must "+
			"only bite on genuinely heavy responses", len(kept), truncated)
	}
}
