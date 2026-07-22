package keeper

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestParseIncomingBlock_LargeBlockNoLongerTruncated is the direct
// regression test for the bug this file's sibling change (p2p.go's
// maxBlockStreamBytes) fixes: a block whose JSON payload is bigger than the
// OLD 512 KB cap (but still comfortably under the new 32 MB one) must parse
// completely and correctly -- not get silently truncated into invalid JSON
// and dropped. 3,000 representative transactions lands comfortably over
// 512 KB (see makeRepresentativeTxs/TestBlockCostAtScale's own measurements:
// roughly 0.23 MB per 1,000 txs) while staying well under maxBlockStreamBytes.
func TestParseIncomingBlock_LargeBlockNoLongerTruncated(t *testing.T) {
	txs := makeRepresentativeTxs(3000)
	want := &Block{
		Height:       999,
		Timestamp:    time.Now().Unix(),
		ParentHashes: []string{"0xparent1", "0xparent2"},
		Hash:         "0xdeadbeef",
		Proposer:     "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266",
		Humans:       1000,
		StateRoot:    "0xstateroot",
		Transactions: txs,
	}
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if len(payload) <= 512<<10 {
		t.Fatalf("test setup bug: payload is only %d bytes, want > 512 KB to actually exercise the old bug's threshold", len(payload))
	}

	got, err := parseIncomingBlock(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("parseIncomingBlock: %v (a %d-byte block should parse cleanly under the %d-byte cap)", err, len(payload), maxBlockStreamBytes)
	}
	if got.Height != want.Height || got.Hash != want.Hash {
		t.Fatalf("got Height=%d Hash=%s, want Height=%d Hash=%s", got.Height, got.Hash, want.Height, want.Hash)
	}
	if len(got.Transactions) != len(want.Transactions) {
		t.Fatalf("got %d transactions, want %d -- block was truncated mid-array", len(got.Transactions), len(want.Transactions))
	}
	if got.Transactions[len(got.Transactions)-1].TxHash != want.Transactions[len(want.Transactions)-1].TxHash {
		t.Fatal("last transaction's TxHash mismatch -- block payload was truncated before the end of the array")
	}
}

// TestParseIncomingBlock_OversizedMessageStillBounded proves the cap still
// does its original job (bounding a single stream read) for a message
// bigger than maxBlockStreamBytes itself -- this must fail to parse (the
// truncated bytes are not valid JSON), not silently succeed with partial
// data, and it must not attempt to read unboundedly past the cap.
func TestParseIncomingBlock_OversizedMessageStillBounded(t *testing.T) {
	// One JSON array element repeated far past maxBlockStreamBytes -- cheap
	// to build without actually allocating a huge []Transaction slice.
	element := `{"type":"transfer","wallet":"0xaaaa","to":"0xbbbb","amount":1},`
	repeats := (maxBlockStreamBytes / len(element)) + 1000
	var sb strings.Builder
	sb.WriteString(`{"height":1,"transactions":[`)
	for i := 0; i < repeats; i++ {
		sb.WriteString(element)
	}
	sb.WriteString(`{"type":"transfer"}]}`)
	oversized := sb.String()
	if len(oversized) <= maxBlockStreamBytes {
		t.Fatalf("test setup bug: message is only %d bytes, want > maxBlockStreamBytes (%d)", len(oversized), maxBlockStreamBytes)
	}

	_, err := parseIncomingBlock(strings.NewReader(oversized))
	if err == nil {
		t.Fatal("want an error for a message larger than maxBlockStreamBytes -- it should be truncated to invalid JSON, not silently accepted")
	}
}

// TestParseIncomingBlock_EmptyMessageErrors preserves handleBlockStream's
// original behavior of treating a zero-byte read as "nothing to do" rather
// than crashing on an empty Block -- json.Unmarshal("") would itself error,
// but parseIncomingBlock short-circuits with a clearer error first.
func TestParseIncomingBlock_EmptyMessageErrors(t *testing.T) {
	_, err := parseIncomingBlock(strings.NewReader(""))
	if err == nil {
		t.Fatal("want an error for an empty message")
	}
}

// TestParseIncomingBlock_MalformedJSONErrors preserves the existing
// "log and drop" behavior for garbage input (e.g. a peer running
// incompatible/corrupt software) -- must return an error, never panic.
func TestParseIncomingBlock_MalformedJSONErrors(t *testing.T) {
	_, err := parseIncomingBlock(strings.NewReader("{not valid json"))
	if err == nil {
		t.Fatal("want an error for malformed JSON")
	}
}
