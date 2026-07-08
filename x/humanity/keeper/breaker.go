package keeper

import (
	"sync"
	"time"
)

// boundedBreaker is a generic per-key circuit breaker: Threshold consecutive
// failures for a key trips a cooldown, during which further attempts for
// that EXACT key are dropped without additional processing; any success
// clears the key's run immediately. Reopen after cooldown is bounded (only
// ReopenProbes chances before re-tripping), not a full reset — a single
// unlucky probe (a transient gossip-ordering blip, not real divergence)
// must not cost another full cooldown, while a genuinely bad actor re-trips
// within a handful of attempts instead of a full fresh run. The tracked-key
// set itself is capped at MaxTracked to bound memory against an
// unauthenticated/attacker-chosen key space — once at cap, brand-new keys
// are silently admitted-free (never tracked, never trips) rather than
// evicting an existing entry, so an attacker who already tripped their own
// key can't clear it early just by flooding fresh keys.
//
// Extracted (performance audit 2026-07-06) from what were two near-duplicate
// implementations: BlockDAG's per-proposer breaker (block.go) and api.go's
// per-IP blockPush breaker. The duplication was a structural risk, not just
// a style issue — the blockPush copy never picked up the P2-c fix
// (proposerFailRun's unbounded-map DoS cap) the proposer copy got, i.e.
// blockPushFailRun was STILL genuinely unbounded (any source IP is an
// unauthenticated, attacker-chosen key) until this consolidation gave every
// user of this type the same MaxTracked protection for free.
type boundedBreaker struct {
	mu           sync.Mutex
	failRun      map[string]int
	breakerUntil map[string]int64

	// Threshold is a plain mutable field, not a constructor-only value,
	// because the proposer breaker rescales it once at startup for the
	// configured BLOCK_TIME (see TuneProposerBreakerForBlockTime) — always
	// before any concurrent access begins, so that one write needs no lock.
	Threshold    int
	Cooldown     time.Duration
	ReopenProbes int
	MaxTracked   int
}

// newBoundedBreaker constructs a ready-to-use breaker. threshold may be
// rescaled later (see Threshold's own comment) by writing the field
// directly before any concurrent access starts.
func newBoundedBreaker(threshold int, cooldown time.Duration, reopenProbes, maxTracked int) *boundedBreaker {
	return &boundedBreaker{
		failRun:      make(map[string]int),
		breakerUntil: make(map[string]int64),
		Threshold:    threshold,
		Cooldown:     cooldown,
		ReopenProbes: reopenProbes,
		MaxTracked:   maxTracked,
	}
}

// ShouldDrop reports whether key's traffic should be dropped now because its
// breaker is still inside its cooldown window. On cooldown expiry, seeds a
// bounded reopen run instead of a full reset (see this type's own comment).
// Nil-safe (matches the nil-map read semantics the inline maps this type
// replaced always had) so a BlockDAG built directly as a struct literal
// without going through NewBlockchain — every lightweight test helper in
// this package does exactly that — never trips on a missing breaker instead
// of a missing map.
func (b *boundedBreaker) ShouldDrop(key string) bool {
	if b == nil || key == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.breakerUntil[key]
	if !ok {
		return false
	}
	if time.Now().UnixNano() >= until {
		delete(b.breakerUntil, key)
		b.failRun[key] = b.Threshold - b.ReopenProbes
		return false
	}
	return true
}

// Clear wipes all tracked state and returns how many keys had an open
// (tripped, still-cooling-down) breaker at the time — used after an event
// that invalidates every key's history at once (e.g. a resync), where
// keeping old fail-runs/cooldowns around would only recreate the exact
// deadlock the event was meant to resolve.
func (b *boundedBreaker) Clear() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(b.breakerUntil)
	b.failRun = make(map[string]int)
	b.breakerUntil = make(map[string]int64)
	return n
}

// RecordOutcome feeds the breaker after an attempt for key completes.
// succeeded clears key's failure run; !succeeded advances it and trips the
// breaker once the run reaches Threshold.
func (b *boundedBreaker) RecordOutcome(key string, succeeded bool) {
	if b == nil || key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if succeeded {
		delete(b.failRun, key)
		delete(b.breakerUntil, key)
		return
	}
	if _, tracked := b.failRun[key]; !tracked && len(b.failRun) >= b.MaxTracked {
		return
	}
	b.failRun[key]++
	if b.failRun[key] >= b.Threshold {
		b.breakerUntil[key] = time.Now().Add(b.Cooldown).UnixNano()
		delete(b.failRun, key) // breaker gates now; rebuild the run after cooldown
	}
}
