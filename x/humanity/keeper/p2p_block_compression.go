package keeper

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

// p2p_block_compression.go attacks the relay ceiling from the side that does
// not need a consensus change.
//
// Roadmap step 4 ("block bodies as references") targets the same ceiling by
// replacing transaction bodies in a block with their hashes. That step has
// an unstated prerequisite which makes it much larger than a format
// migration: it only helps if peers ALREADY hold the bodies when the
// reference-only block arrives, and on this network they do not.
// pending_txs is a purely local outbox — SavePendingTx writes to the node's
// own database and the P2P layer broadcasts blocks and nothing else, so a
// peer learns a transaction's contents exclusively from the block carrying
// it. Referencing hashes without a transaction-gossip layer first would
// force every peer to fetch every body on receipt: the same bytes, plus a
// round trip. That layer is real work and is called out in the roadmap
// notes accompanying this change.
//
// Compression needs none of that, and attacks the same bytes:
//
//	MEASURED (TestBlockRelayCompression_Ratio, blocks of realistic transfers):
//
//	    100 txs:   0.02 MB ->  0.00 MB  (15.9x)
//	  1,000 txs:   0.24 MB ->  0.01 MB  (17.6x)
//	 20,000 txs:   4.80 MB ->  0.27 MB  (17.7x)
//	 50,000 txs:  12.00 MB ->  0.68 MB  (17.7x)
//
// The ratio is structural rather than lucky: a block payload is the same
// handful of JSON field names repeated once per transaction, with addresses
// and hashes drawn from a 16-character alphabet. That is close to the ideal
// case for LZ77, and the ratio holds as block size grows — which is exactly
// the direction that matters, since the ceiling is about full blocks.
//
// Nothing about consensus changes. The block hash is computed over the
// decoded block (calculateBlockHash marshals the transaction list itself),
// so compression is purely a transport concern — a compressed and an
// uncompressed relay of the same block produce byte-identical hashes, which
// TestBlockRelayCompression_HashUnchanged pins.
//
// ROLLOUT (two phases, deliberately). A node running an older binary cannot
// parse a gzip frame, so sending one to a network that is not ready would
// silently partition it:
//
//	Phase 1 — this commit. Every node ACCEPTS both encodings; no node sends
//	          compressed. Deployable in any order, to any subset, with no
//	          coordination, because it only widens what a node tolerates.
//	Phase 2 — after every node runs a phase-1 binary, set
//	          AEQUITAS_P2P_COMPRESS_BLOCKS=1 to start sending compressed.
//
// Sniffing is safe in both directions and needs no negotiation: gzip frames
// begin with 0x1f 0x8b, and a JSON block message always begins with '{'
// (or whitespace). The two can never be confused.

// gzipMagic is the first two bytes of every gzip frame (RFC 1952).
var gzipMagic = []byte{0x1f, 0x8b}

// blockCompressionEnabled reports whether this node should SEND compressed
// blocks. Off unless explicitly enabled — see the rollout note above.
func blockCompressionEnabled() bool {
	return os.Getenv("AEQUITAS_P2P_COMPRESS_BLOCKS") == "1"
}

// compressBlockPayload gzips an already-marshalled block message.
//
// Returns the ORIGINAL bytes unchanged if compression fails or fails to pay
// for itself. Both are real cases and neither is an error: a tiny block
// (an empty one is a few hundred bytes) can come out larger than it went in
// because of the ~18-byte gzip header and trailer, and a peer parses either
// encoding, so falling back costs nothing.
func compressBlockPayload(raw []byte) []byte {
	var buf bytes.Buffer
	// BestSpeed, not BestCompression: this runs on the block-production path
	// and once more per relay hop. The payload is highly redundant JSON, so
	// the cheap level already captures most of the win, and spending
	// milliseconds of CPU per hop to shave a further few percent off a
	// payload that is already small would trade the wrong resource.
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return raw
	}
	if _, err := zw.Write(raw); err != nil {
		zw.Close()
		return raw
	}
	if err := zw.Close(); err != nil {
		return raw
	}
	if buf.Len() >= len(raw) {
		return raw
	}
	return buf.Bytes()
}

// decompressBlockPayload returns body decoded, transparently handling both
// encodings. A payload that is not a gzip frame is returned untouched.
//
// The decompressed size is bounded by maxBlockStreamBytes for the same
// reason the compressed read is: a small compressed payload can expand
// without limit, and a peer must not be able to make this node allocate
// arbitrary memory by sending a few kilobytes. Hitting the bound is treated
// as a hard error rather than silently truncating — a truncated block would
// fail to parse anyway, and saying so names the real cause.
func decompressBlockPayload(body []byte) ([]byte, error) {
	if !bytes.HasPrefix(body, gzipMagic) {
		return body, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("block payload began with the gzip magic but is not a valid gzip frame: %w", err)
	}
	defer zr.Close()

	// Read one byte past the cap so hitting it exactly is distinguishable
	// from a payload that legitimately ends there.
	out, err := io.ReadAll(io.LimitReader(zr, maxBlockStreamBytes+1))
	if err != nil {
		return nil, fmt.Errorf("could not decompress block payload: %w", err)
	}
	if len(out) > maxBlockStreamBytes {
		return nil, fmt.Errorf("decompressed block payload exceeds %d bytes — refusing to buffer it", maxBlockStreamBytes)
	}
	return out, nil
}
