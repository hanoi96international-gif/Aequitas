package mpc

import (
	"context"
	"fmt"
	"sync"
)

// Transport is the one place where MPC stops being arithmetic and becomes a
// distributed system.
//
// Every Beaver multiplication has to open two blinded values, and opening
// means each party publishes its own share and receives the others'. That
// exchange is the ONLY thing that crosses a network in this protocol —
// everything else is local. Isolating it behind one interface has three
// consequences worth stating:
//
//   - the protocol code stays testable without any network, by exchanging
//     through memory instead of sockets;
//   - a networked deployment changes exactly one implementation, not the
//     protocol;
//   - the amount of data that leaves a machine is visible in one place, which
//     is what makes "no party ever sees a template" checkable rather than
//     merely claimed.
//
// # WHAT CROSSES THE WIRE, AND WHY IT IS SAFE
//
// Only blinded differences: d = x - a and e = y - b, where a and b are fresh,
// uniformly random values from a multiplication triple. Because a and b are
// used once and never reused, d and e are themselves uniformly distributed and
// independent of x and y. An observer who records every byte of every round
// learns nothing about any template — that is Beaver's construction, not a
// property of this implementation.
//
// What an observer DOES learn is metadata: how many multiplications ran, and
// therefore how many candidates a registration was compared against. That is
// why the batched paths matter for privacy too, not only for speed: they emit
// one exchange for a whole comparison instead of one per feature.
type Transport interface {
	// Exchange publishes this party's values for one round and returns every
	// party's values, indexed by party number. The returned slice must have
	// one entry per party, and entry [Index()] must equal mine.
	//
	// Implementations must be safe to call from one goroutine at a time per
	// session, and must fail rather than block forever: a stalled round is a
	// stalled registration, and a human is waiting.
	Exchange(ctx context.Context, round int, mine []Element) ([][]Element, error)

	// Index is which party this is, 0-based.
	Index() int

	// Parties is how many parties take part.
	Parties() int
}

// LocalTransport runs every party inside one process.
//
// It is the reference implementation and what the tests use. It is NOT a
// deployment: a single process holding every share can reconstruct every
// secret, so the privacy property this package exists for does not hold here.
// It exists so the protocol can be exercised deterministically, and so a
// networked transport has something to be checked against.
type LocalTransport struct {
	index   int
	parties int
	hub     *localHub
}

type localHub struct {
	mu      sync.Mutex
	parties int
	rounds  map[int]*localRound
}

// localRound is one barrier.
//
// Exchange must BLOCK until every party has contributed, not fail because a
// peer has not arrived yet. Parties run concurrently and reach a round at
// different moments — which is the condition a networked deployment lives in
// permanently, not an edge case. The first version of this hub returned an
// error instead of waiting; it passed every sequential test and failed the
// moment the parties became real goroutines.
type localRound struct {
	slots [][]Element
	have  int
	done  chan struct{}
}

// NewLocalTransports returns one transport per party, all sharing a hub.
func NewLocalTransports(parties int) ([]Transport, error) {
	if parties < 2 {
		return nil, fmt.Errorf("mpc: %d parties cannot hide anything", parties)
	}
	hub := &localHub{parties: parties, rounds: map[int]*localRound{}}
	out := make([]Transport, parties)
	for i := range out {
		out[i] = &LocalTransport{index: i, parties: parties, hub: hub}
	}
	return out, nil
}

// Index implements Transport.
func (t *LocalTransport) Index() int { return t.index }

// Parties implements Transport.
func (t *LocalTransport) Parties() int { return t.parties }

// Exchange implements Transport by collecting contributions in memory and
// waiting until the round is complete.
//
// The wait respects ctx: a party that never arrives must surface as a failed
// registration, not as a goroutine parked forever. Someone is waiting at the
// other end of this call.
func (t *LocalTransport) Exchange(ctx context.Context, round int, mine []Element) ([][]Element, error) {
	t.hub.mu.Lock()
	r, ok := t.hub.rounds[round]
	if !ok {
		r = &localRound{
			slots: make([][]Element, t.hub.parties),
			done:  make(chan struct{}),
		}
		t.hub.rounds[round] = r
	}
	if r.slots[t.index] != nil {
		t.hub.mu.Unlock()
		return nil, fmt.Errorf("mpc: party %d contributed to round %d twice — a reused round "+
			"number means two computations are overwriting each other", t.index, round)
	}
	cp := make([]Element, len(mine))
	copy(cp, mine)
	r.slots[t.index] = cp
	r.have++
	if r.have == t.hub.parties {
		close(r.done)
	}
	t.hub.mu.Unlock()

	select {
	case <-r.done:
		// Safe to read without the lock: every write happened under it, before
		// the close that this receive synchronises with.
		return r.slots, nil
	case <-ctx.Done():
		t.hub.mu.Lock()
		var missing []int
		for i, slot := range r.slots {
			if slot == nil {
				missing = append(missing, i)
			}
		}
		t.hub.mu.Unlock()
		return nil, fmt.Errorf("mpc: round %d gave up waiting for parties %v: %w — a comparison "+
			"cannot complete without every party, and refusing is better than deciding on partial "+
			"information", round, missing, ctx.Err())
	}
}

// OpenVia reconstructs a batch of shared values through a transport.
//
// This is the single function that turns local shares into an opened value,
// and therefore the single place where anything leaves a machine.
func OpenVia(ctx context.Context, tr Transport, round int, mine []Element) ([]Element, error) {
	all, err := tr.Exchange(ctx, round, mine)
	if err != nil {
		return nil, fmt.Errorf("mpc: round %d: %w", round, err)
	}
	if len(all) != tr.Parties() {
		return nil, fmt.Errorf("mpc: round %d returned %d parties, expected %d",
			round, len(all), tr.Parties())
	}
	width := len(mine)
	out := make([]Element, width)
	for p, vals := range all {
		if len(vals) != width {
			return nil, fmt.Errorf("mpc: round %d: party %d sent %d values, expected %d — "+
				"the parties are not running the same computation", round, p, len(vals), width)
		}
		for i, v := range vals {
			out[i] = Add(out[i], v)
		}
	}
	return out, nil
}

// Session carries the per-party state one distributed computation needs.
type Session struct {
	Transport Transport
	Triples   *TripleStore

	round int
}

// NewSession starts a computation for one party.
func NewSession(tr Transport, triples *TripleStore) *Session {
	return &Session{Transport: tr, Triples: triples}
}

// MulBatch multiplies many pairs of shared secrets in ONE network round.
//
// Each party calls this with its own shares; the returned slice is that party's
// share of each product. The round counter advances in lockstep because every
// party performs the same sequence of operations — which is also why a
// mismatch shows up immediately as a width error rather than as a wrong answer.
func (s *Session) MulBatch(ctx context.Context, xs, ys []Element) ([]Element, error) {
	if len(xs) != len(ys) {
		return nil, fmt.Errorf("mpc: batch length mismatch (%d xs, %d ys)", len(xs), len(ys))
	}
	if len(xs) == 0 {
		return nil, nil
	}

	triples := make([]Triple, len(xs))
	blinded := make([]Element, 0, 2*len(xs))
	for k := range xs {
		t, err := s.Triples.Next()
		if err != nil {
			return nil, fmt.Errorf("mpc: pair %d: %w", k, err)
		}
		triples[k] = t
		blinded = append(blinded, Sub(xs[k], t.A), Sub(ys[k], t.B))
	}

	s.round++
	opened, err := OpenVia(ctx, s.Transport, s.round, blinded)
	if err != nil {
		return nil, err
	}

	first := s.Transport.Index() == 0
	out := make([]Element, len(xs))
	for k := range xs {
		d, e := opened[2*k], opened[2*k+1]
		v := Add(triples[k].C, Mul(d, triples[k].B))
		v = Add(v, Mul(e, triples[k].A))
		if first {
			// The public d*e term is added by exactly one party, or the sum
			// would count it once per party.
			v = Add(v, Mul(d, e))
		}
		out[k] = v
	}
	return out, nil
}

// Rounds is how many network round trips this session has used so far. Worth
// logging per registration: it is the number that decides how long a human
// waits, and a sudden rise means a batching path stopped batching.
func (s *Session) Rounds() int { return s.round }
