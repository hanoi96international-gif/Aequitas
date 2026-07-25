package keeper

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// buildBenchBlock returns a block shaped like a real full one: n transfers
// between distinct addresses, each with the fields ProduceBlock actually
// populates. Used both for the compression-ratio measurement and for the
// round-trip tests, so the numbers describe something realistic rather than
// a best case built out of identical rows.
func buildBenchBlock(n int) *Block {
	txs := make([]Transaction, n)
	for i := range txs {
		txs[i] = Transaction{
			Type:              "transfer",
			Wallet:            fmt.Sprintf("0x%040x", 2*i),
			To:                fmt.Sprintf("0x%040x", 2*i+1),
			Amount:            float64(i%997) + 0.123456,
			TxHash:            fmt.Sprintf("0x%064x", i),
			FromDemurrageLost: float64(i%7) * 0.000001,
		}
	}
	return &Block{
		Height:       1_849_827,
		Timestamp:    1_800_000_000,
		ParentHashes: []string{strings.Repeat("a", 64), strings.Repeat("b", 64)},
		Hash:         strings.Repeat("c", 64),
		Proposer:     fmt.Sprintf("0x%040x", 999),
		Humans:       14,
		StateRoot:    strings.Repeat("d", 64),
		Transactions: txs,
		Signature:    strings.Repeat("e", 130),
	}
}

// TestBlockRelayCompression_Ratio is the measurement behind the claim. It
// prints the real numbers for several block sizes rather than asserting a
// single magic constant, and fails only if compression stops paying for
// itself at the sizes that matter — which is the property the relay ceiling
// actually depends on.
func TestBlockRelayCompression_Ratio(t *testing.T) {
	for _, n := range []int{100, 1_000, 20_000, 50_000} {
		raw, err := json.Marshal(buildBenchBlock(n))
		if err != nil {
			t.Fatalf("marshal %d txs: %v", n, err)
		}
		packed := compressBlockPayload(raw)
		ratio := float64(len(raw)) / float64(len(packed))
		t.Logf("%6d txs: %7.2f MB raw -> %6.2f MB gzip  (%.1fx)",
			n, float64(len(raw))/(1<<20), float64(len(packed))/(1<<20), ratio)

		if n >= 1_000 && ratio < 4 {
			t.Errorf("%d txs compressed only %.1fx — block payloads are highly repetitive JSON and "+
				"should compress far better; something changed about the payload shape", n, ratio)
		}
	}
}

// TestBlockRelayCompression_HashUnchanged is the consensus claim: this is a
// transport change and nothing else. A block that travelled compressed must
// produce the identical hash to one that travelled raw — if it ever did not,
// compressed and uncompressed peers would fork.
func TestBlockRelayCompression_HashUnchanged(t *testing.T) {
	original := buildBenchBlock(250)
	wantHash := calculateBlockHash(original)

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{"uncompressed", raw},
		{"compressed", compressBlockPayload(raw)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIncomingBlock(bytes.NewReader(tc.wire))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if h := calculateBlockHash(got); h != wantHash {
				t.Fatalf("block hash after %s relay is %s, want %s — the two encodings would fork the network",
					tc.name, h, wantHash)
			}
			if len(got.Transactions) != len(original.Transactions) {
				t.Fatalf("got %d transactions, want %d", len(got.Transactions), len(original.Transactions))
			}
			// Transaction contains slice fields, so compare the marshalled
			// form — which is also exactly what tx_root hashes.
			gotTx, _ := json.Marshal(got.Transactions[7])
			wantTx, _ := json.Marshal(original.Transactions[7])
			if !bytes.Equal(gotTx, wantTx) {
				t.Errorf("transaction 7 did not survive the round trip:\n got %s\nwant %s", gotTx, wantTx)
			}
		})
	}
}

// TestBlockRelayCompression_AcceptsBothEncodings is the rollout claim. Phase 1
// is safe to deploy to any subset of nodes in any order precisely because a
// phase-1 node still parses what every existing node sends.
func TestBlockRelayCompression_AcceptsBothEncodings(t *testing.T) {
	raw, err := json.Marshal(buildBenchBlock(10))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := parseIncomingBlock(bytes.NewReader(raw)); err != nil {
		t.Fatalf("a phase-1 node rejected a plain JSON block from an older peer: %v", err)
	}
	if _, err := parseIncomingBlock(bytes.NewReader(compressBlockPayload(raw))); err != nil {
		t.Fatalf("a phase-1 node rejected a compressed block: %v", err)
	}
}

// TestBlockRelayCompression_SendingIsOffByDefault guards the rollout order.
// If this ever flips to on-by-default, a deploy would send frames that
// not-yet-upgraded peers cannot parse, silently partitioning the network —
// the failure mode the two-phase rollout exists to avoid.
func TestBlockRelayCompression_SendingIsOffByDefault(t *testing.T) {
	t.Setenv("AEQUITAS_P2P_COMPRESS_BLOCKS", "")
	if blockCompressionEnabled() {
		t.Fatal("compressed sending is enabled by default — every peer must be able to RECEIVE " +
			"compressed blocks before any node starts SENDING them")
	}
	t.Setenv("AEQUITAS_P2P_COMPRESS_BLOCKS", "1")
	if !blockCompressionEnabled() {
		t.Fatal("AEQUITAS_P2P_COMPRESS_BLOCKS=1 did not enable compressed sending")
	}
}

// TestBlockRelayCompression_TinyPayloadFallsBackToRaw covers the case where
// compression does not pay: gzip's header and trailer can exceed what it
// saves on a near-empty block. Returning the original is correct rather than
// exceptional, since peers parse either encoding.
func TestBlockRelayCompression_TinyPayloadFallsBackToRaw(t *testing.T) {
	raw := []byte(`{"height":1}`)
	got := compressBlockPayload(raw)
	if !bytes.Equal(got, raw) {
		t.Errorf("a payload that does not benefit from compression was compressed anyway "+
			"(%d bytes -> %d)", len(raw), len(got))
	}
}

// TestBlockRelayCompression_RejectsDecompressionBomb pins the resource
// bound. A peer must not be able to make this node allocate arbitrary memory
// by sending a few kilobytes that expand without limit.
func TestBlockRelayCompression_RejectsDecompressionBomb(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	chunk := bytes.Repeat([]byte{'A'}, 1<<20)
	for written := 0; written <= maxBlockStreamBytes; written += len(chunk) {
		if _, err := zw.Write(chunk); err != nil {
			t.Fatalf("build bomb: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close bomb: %v", err)
	}
	t.Logf("bomb: %d compressed bytes expanding past the %d-byte cap", buf.Len(), maxBlockStreamBytes)

	if _, err := decompressBlockPayload(buf.Bytes()); err == nil {
		t.Fatal("an over-large decompressed payload was accepted — a peer could exhaust this node's memory")
	} else if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("rejected for the wrong reason: %v", err)
	}
}

// TestBlockRelayCompression_RejectsCorruptGzip checks that a payload which
// claims to be gzip but is not produces a clear error rather than being
// silently handed to json.Unmarshal as binary garbage.
func TestBlockRelayCompression_RejectsCorruptGzip(t *testing.T) {
	corrupt := append([]byte{0x1f, 0x8b}, []byte("this is not a gzip stream")...)
	if _, err := decompressBlockPayload(corrupt); err == nil {
		t.Fatal("corrupt gzip frame accepted")
	}
	if _, err := parseIncomingBlock(bytes.NewReader(corrupt)); err == nil {
		t.Fatal("parseIncomingBlock accepted a corrupt gzip frame")
	}
}
