package keeper

import (
	"compress/gzip"
	"io"
	"sync"
)

// Pooled gzip writers for the API's compression middleware.
//
// WHY. A CPU profile taken on Contabo2 while 597 senders drove the node put
// 48.52% of all CPU in handleBlocks, of which compress/flate was 20.90% --
// against 15.32% for the entire signature-recovery path. That is the peer sync
// path: /api/blocks serves up to 500 blocks per request and, under load, a
// block carries thousands of transactions. Alongside it,
// runtime.mallocgcLarge accounted for 7.03%, which is gzip.NewWriter building
// a fresh compressor for every single request.
//
// Both costs are avoidable without giving up compression, which peers do
// benefit from: a real /api/blocks response measured 505KB raw against 88KB
// compressed.

// gzipLevel is deliberately BestSpeed rather than the default.
//
// flate's findMatch -- the longest-match search that the higher levels spend
// their time on -- was 7.05% of the node's CPU on its own. Level 1 skips most
// of that. The node was measured at 262% of 600% CPU with peers waiting on it,
// and has never been close to bandwidth-bound, so trading some ratio for a
// large share of that CPU is the right direction. It is a named constant so
// the choice is visible and revisitable rather than buried in a call.
const gzipLevel = gzip.BestSpeed

var gzipWriterPool = sync.Pool{
	New: func() interface{} {
		// io.Discard, not nil: NewWriterLevel needs a destination, and every
		// caller Resets onto the real one before writing. The error is
		// impossible for a constant, valid level, so the writer is used
		// directly rather than pretending the failure needs handling.
		w, _ := gzip.NewWriterLevel(io.Discard, gzipLevel)
		return w
	},
}

// acquireGzipWriter returns a writer already pointed at w.
func acquireGzipWriter(w io.Writer) *gzip.Writer {
	gz := gzipWriterPool.Get().(*gzip.Writer)
	gz.Reset(w)
	return gz
}

// releaseGzipWriter finishes the response and returns the writer to the pool.
//
// Close must happen before Put and must not be skipped: it writes gzip's
// trailer, without which the body a client receives is truncated and
// undecodable. Reset (in acquireGzipWriter) is what makes a closed writer
// usable again, so returning a closed writer to the pool is correct.
func releaseGzipWriter(gz *gzip.Writer) {
	gz.Close()
	// Drop the reference to the response writer, so a pooled entry cannot keep
	// a finished request's ResponseWriter (and whatever it retains) alive until
	// the next Get.
	gz.Reset(io.Discard)
	gzipWriterPool.Put(gz)
}
