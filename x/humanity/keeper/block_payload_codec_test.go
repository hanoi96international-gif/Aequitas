package keeper

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func codecTxs(n int) []Transaction {
	txs := make([]Transaction, n)
	for i := range txs {
		txs[i] = Transaction{Type: "transfer", Wallet: "0xaaa", To: "0xbbb", Amount: float64(i) + 0.25}
	}
	return txs
}

// Round trip, because this is the payment record: what comes back has to be
// exactly what went in, field for field.
func TestBlockPayload_RoundTripsCompressed(t *testing.T) {
	want := codecTxs(500)
	raw := mustMarshalTxs(t, want)

	z, err := compressBlockPayload(raw)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if len(z) >= len(raw) {
		t.Fatalf("compressed to %d bytes from %d — no saving, the whole change is pointless", len(z), len(raw))
	}

	got, err := decodeBlockPayload("", z)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("decoded %d transactions, want %d", len(got), len(want))
	}
	// reflect.DeepEqual, not ==: Transaction carries a slice field, so the
	// struct is not comparable — and a shallow check would miss exactly the
	// kind of corruption a codec change can introduce.
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Fatalf("transaction %d changed across the round trip:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

// Rows written before this existed carry only the plain column and must keep
// decoding exactly as they always did.
func TestBlockPayload_StillReadsPlainRows(t *testing.T) {
	want := codecTxs(10)
	got, err := decodeBlockPayload(string(mustMarshalTxs(t, want)), nil)
	if err != nil {
		t.Fatalf("decode plain: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("decoded %d transactions from a plain row, want 10", len(got))
	}
}

// The compressed column wins when both are present: a row this code writes
// leaves the plain column empty, so anything found there is stale.
func TestBlockPayload_CompressedWinsOverPlain(t *testing.T) {
	z, err := compressBlockPayload(mustMarshalTxs(t, codecTxs(3)))
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	got, err := decodeBlockPayload(string(mustMarshalTxs(t, codecTxs(99))), z)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("decoded %d transactions; the compressed column must take precedence", len(got))
	}
}

// A payload that cannot be read must FAIL, not come back empty.
//
// The loader this replaces did `_ = json.Unmarshal(...)`, so an unreadable
// payload silently became a block with no transactions — and such a block still
// hashes correctly through its TxRoot, so nothing downstream would have
// objected either. That is how a page of transfers disappears without a trace.
func TestBlockPayload_UnreadableIsAnErrorNotAnEmptyBlock(t *testing.T) {
	if txs, err := decodeBlockPayload("", []byte("this is not gzip")); err == nil {
		t.Fatalf("corrupt compressed payload decoded to %d transactions instead of failing", len(txs))
	}
	if txs, err := decodeBlockPayload("{not json", nil); err == nil {
		t.Fatalf("corrupt plain payload decoded to %d transactions instead of failing", len(txs))
	}
	// An empty block is legitimately empty and must NOT be an error.
	if txs, err := decodeBlockPayload("", nil); err != nil || len(txs) != 0 {
		t.Fatalf("an empty payload must decode to zero transactions without error, got %d / %v", len(txs), err)
	}
}

// Compression has to actually pay on realistic block JSON, or the change is
// cost without benefit.
func TestBlockPayload_CompressesBlockJSONWell(t *testing.T) {
	raw := mustMarshalTxs(t, codecTxs(5000))
	z, err := compressBlockPayload(raw)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	ratio := float64(len(raw)) / float64(len(z))
	if ratio < 5 {
		t.Fatalf("only %.1fx on block JSON (%d -> %d); measured about 18x, so something is wrong with the level or the data", ratio, len(raw), len(z))
	}
}

func mustMarshalTxs(t *testing.T, txs []Transaction) []byte {
	t.Helper()
	b, err := json.Marshal(txs)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if !strings.HasPrefix(string(b), "[") {
		t.Fatalf("fixture is not a JSON array: %.40s", b)
	}
	return b
}
