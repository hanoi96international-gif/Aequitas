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

	fetch := func(chunk []string) ([]*Block, error) {
		asked = append(asked, len(chunk))
		if len(chunk) > fits {
			return nil, errResponseTooLarge
		}
		out := make([]*Block, 0, len(chunk))
		for _, h := range chunk {
			out = append(out, &Block{Hash: h})
		}
		return out, nil
	}

	want := make([]string, 16)
	for i := range want {
		want[i] = fmt.Sprintf("0x%02x", i)
	}

	blocks, err := splitOnOversize(want, "test-peer", fetch)
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
	fetch := func(chunk []string) ([]*Block, error) {
		calls++
		if calls > 100 {
			t.Fatal("splitting did not terminate on a single oversized block")
		}
		return nil, errResponseTooLarge
	}
	_, err := splitOnOversize([]string{"0xaa"}, "test-peer", fetch)
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
