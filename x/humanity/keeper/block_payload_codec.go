package keeper

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Compressed block payloads.
//
// WHY. SaveBlockWithPendingTxsAtomic runs while dag.mu is held, and under load
// it is the binding constraint on the whole chain: block production fell to 9
// blocks in 85 seconds where 85 were due, and AddPeerBlock cannot take the lock
// during that window either, which is what piles up orphans and collapses the
// DAG to a single tip. TestSaveBlockCostAtScale put the INSERT at roughly 74%
// of that save and showed it scaling with payload bytes.
//
// MEASURED, on the target hardware against a disposable Postgres
// (TestCompressedPayloadInsertCost):
//
//	txs      raw        insert     gzip       compress+insert   share
//	 1,000   0.14 MB     14.4 ms   0.01 MB     7.9 ms            55%
//	10,000   1.45 MB     43.9 ms   0.08 MB    18.6 ms            42%
//	50,000   7.24 MB    179.1 ms   0.39 MB    51.4 ms            29%
//
// 3.5x at full-block size with the compression time already counted, and the
// win grows with the block. Postgres does TOAST-compress a large text value on
// its own, which was the reason to measure rather than assume — but that
// happens after the bytes have crossed the wire and costs server CPU, so doing
// it first still wins by a wide margin.
//
// ONE-WAY, AND THAT IS WHY IT IS OFF BY DEFAULT. A block written compressed
// leaves `transactions` empty. Code that predates this column reads that as a
// block with NO transactions — and reads it SILENTLY, because the existing
// loader discards json.Unmarshal's error. A node rolled back to an older build
// after this was enabled would therefore apply real blocks as empty ones rather
// than failing, which is the worst failure mode this codebase has. There is no
// format that avoids this: any encoding older code cannot parse ends up in the
// same place.
//
// So enabling it is a decision about the whole network, not about one node:
// every node must carry this code before any node writes with it, and going
// back means restoring from a snapshot rather than redeploying.

// blockPayloadCompressionEnabled reports whether new blocks should be stored
// with a compressed payload.
func blockPayloadCompressionEnabled() bool {
	v := os.Getenv("AEQUITAS_COMPRESS_BLOCK_PAYLOAD")
	return v != "" && v != "0" && v != "false"
}

// compressBlockPayload returns the gzip form of a marshalled transaction list.
//
// BestSpeed rather than the default: the measurement above used it, the ratio
// on this data was still about 18:1, and the levels above it spend their time
// on a longest-match search this payload does not need.
func compressBlockPayload(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("gzip writer: %w", err)
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, fmt.Errorf("compress block payload: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finish block payload: %w", err)
	}
	return buf.Bytes(), nil
}

// decodeBlockPayload turns whichever of the two stored forms is present back
// into a transaction list.
//
// The compressed column wins when it holds anything, because a row written by
// this code leaves the plain column empty. A row from before carries only the
// plain column and decodes exactly as it always did.
//
// Unlike the loader this replaces, a malformed payload is REPORTED rather than
// discarded. That loader did `_ = json.Unmarshal(...)`, so a block whose
// transactions could not be read became a block with no transactions, silently
// — and a block with no transactions still hashes correctly via its TxRoot, so
// nothing downstream would have objected either.
func decodeBlockPayload(plain string, compressed []byte) ([]Transaction, error) {
	if len(compressed) > 0 {
		zr, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, fmt.Errorf("open compressed block payload (%d bytes): %w", len(compressed), err)
		}
		defer zr.Close()
		raw, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("read compressed block payload: %w", err)
		}
		var txs []Transaction
		if err := json.Unmarshal(raw, &txs); err != nil {
			return nil, fmt.Errorf("decode compressed block payload (%d bytes raw): %w", len(raw), err)
		}
		return txs, nil
	}
	if plain == "" {
		return nil, nil
	}
	var txs []Transaction
	if err := json.Unmarshal([]byte(plain), &txs); err != nil {
		return nil, fmt.Errorf("decode block payload (%d bytes): %w", len(plain), err)
	}
	return txs, nil
}
