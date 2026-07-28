package keeper

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

// Per-endpoint request accounting, added to answer one question that three
// separate measurements could not.
//
// A CPU profile taken under load put 12.87% in handleBlocks and 11.60% in
// handleBlocksByHash — a quarter of the node's CPU — at a moment when a
// simultaneous sample of established sockets on :8080 showed nothing but the
// 599 local connections belonging to the load generator, and the generator only
// ever polls /api/status. Both explanations that were reached for turned out to
// be wrong: it is not the operator's browser (the same cost appears with no
// browser open) and it is not a peer catching up after a restart (the primary
// and this node measured at exactly the same height).
//
// Rather than guess a fourth time, this counts the calls and records who made
// them. Cheap enough to leave on: two atomic adds per request, and a bounded
// map of caller hosts written only when a host is seen for the first time.

// endpointCallerMax bounds the caller map. Far above the number of peers a node
// has, and small enough that an unexpected flood of distinct sources cannot
// turn this diagnostic into a memory problem of its own.
const endpointCallerMax = 64

type endpointCounter struct {
	calls atomic.Int64
	bytes atomic.Int64

	mu      sync.Mutex
	callers map[string]int64
}

func (c *endpointCounter) note(remote string, n int) {
	c.calls.Add(1)
	c.bytes.Add(int64(n))

	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	c.mu.Lock()
	if c.callers == nil {
		c.callers = make(map[string]int64, 8)
	}
	// A new host is only added while there is room, so the map cannot grow
	// without bound; hosts already known keep counting either way.
	if _, seen := c.callers[host]; seen || len(c.callers) < endpointCallerMax {
		c.callers[host]++
	}
	c.mu.Unlock()
}

func (c *endpointCounter) snapshot() map[string]interface{} {
	c.mu.Lock()
	callers := make(map[string]int64, len(c.callers))
	for k, v := range c.callers {
		callers[k] = v
	}
	c.mu.Unlock()
	return map[string]interface{}{
		"calls":   c.calls.Load(),
		"bytes":   c.bytes.Load(),
		"callers": callers,
	}
}

var (
	statBlocks       endpointCounter
	statBlocksByHash endpointCounter
	statCanonical    endpointCounter
)

// EndpointStats reports what the block-serving endpoints actually cost and who
// is driving them.
func EndpointStats() map[string]interface{} {
	return map[string]interface{}{
		"api_blocks":           statBlocks.snapshot(),
		"api_blocks_by_hash":   statBlocksByHash.snapshot(),
		"api_blocks_canonical": statCanonical.snapshot(),
	}
}

// countingWriter counts the bytes a handler produces. It wraps the writer the
// handler was given, which on this server is already the gzip writer, so the
// figure is COMPRESSED bytes — the same thing that leaves the machine.
type countingWriter struct {
	http.ResponseWriter
	n int
}

func (w *countingWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.n += n
	return n, err
}

// countEndpoint wraps a handler so its calls, response size and callers are
// recorded.
func countEndpoint(c *endpointCounter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cw := &countingWriter{ResponseWriter: w}
		next(cw, r)
		c.note(r.RemoteAddr, cw.n)
	}
}
