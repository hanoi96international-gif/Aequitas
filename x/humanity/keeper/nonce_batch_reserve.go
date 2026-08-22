package keeper

import (
	"time"
)

// Reserve a whole batch's nonce range in one database operation.
//
// THE MEASUREMENT. rpc_phase_stats.go split a transfer into phases under load,
// with the rate limiter out of the way:
//
//	sendRawTransaction   75.63 ms
//	  nonce reservation  19.59 ms   <- 26% of every transfer
//	  TransferAtomic     52.12 ms
//	  everything else     3.92 ms
//
// The nonce batcher's own wait is capped at 1 ms (nonceBatchMaxWait), so those
// 19.6 ms are the Postgres round trip itself — and the per-sender shard lock is
// held across it.
//
// WHY IT IS PURE WASTE HERE. One JSON-RPC batch carries up to 100 transfers
// from a SINGLE sender with CONSECUTIVE nonces; that is what the load
// generator sends and what any batching client naturally sends, since
// consecutive nonces are the only way a sender can have more than one
// transaction in flight. The dispatch loop must apply them in order for
// exactly that reason. So the batch already knows, before it applies anything,
// that it is going to reserve n, n+1, ... n+99 — and it pays a separate
// round trip for each one anyway.
//
// One compare-and-swap from n to n+100 gives the identical result. The check
// is the same check, held under the same shard lock, against the same stored
// value; only the number of round trips changes, from 100 to 1.
//
// WHAT THIS DOES NOT CHANGE. A transfer that fails after its nonce is
// reserved — insufficient balance, say — does not give the nonce back today
// either (sendRawTransaction returns the error and leaves the reservation
// standing, which is also how Ethereum treats a failed transaction). So
// reserving the range up front consumes exactly the nonces the per-item path
// would have consumed.
//
// WHAT IT DOES CHANGE, STATED PLAINLY. If the process dies midway through a
// batch, the database already reads n+100 while only some of those transfers
// were applied. The remaining nonces are skipped, and the sender's next
// transaction at n+50 is answered "nonce too low" until it re-reads its nonce
// with eth_getTransactionCount.
//
// That is the safe direction to fail. The reservation is still fully durable
// BEFORE any transfer is applied, so no transaction can ever be replayed — the
// property the nonce check exists for. A crash costs a client one nonce
// refetch; the opposite trade, reserving lazily, would cost a replay.
//
// CONSERVATIVE BY CONSTRUCTION. Only a run that starts exactly at the stored
// nonce and increases by one each step is reserved as a range. Anything else —
// a gap, a repeat, a decode failure in the middle, a single-item batch, a
// second sender — is left to the per-item path untouched, and answers exactly
// what it answers today.

// preReserveBatchNonces reserves each sender's consecutive nonce run in one
// database round trip, and marks the items it covered so sendRawTransaction
// skips its own reservation for them.
//
// Called from handleRPC's batch path after the parallel decode, so tx and
// sender are already available for every item.
func (s *EVMRPCServer) preReserveBatchNonces(precomputed []*precomputedSendTx, pending []int) {
	if s.state == nil || len(pending) < 2 {
		return
	}

	// Group by sender, keeping batch order. Order is what makes a run
	// contiguous, so it must not be disturbed.
	order := make([]string, 0, 4)
	bySender := make(map[string][]*precomputedSendTx, 4)
	for _, i := range pending {
		p := precomputed[i]
		if p == nil || p.err != nil || p.tx == nil || p.sender == "" {
			continue
		}
		if _, seen := bySender[p.sender]; !seen {
			order = append(order, p.sender)
		}
		bySender[p.sender] = append(bySender[p.sender], p)
	}

	for _, sender := range order {
		items := bySender[sender]
		if len(items) < 2 {
			// A single transaction gains nothing from a range reservation —
			// it is the same one round trip either way.
			continue
		}
		s.reserveOneSenderRun(sender, items)
	}
}

// reserveOneSenderRun reserves the longest run of consecutive nonces starting
// at this sender's stored nonce, in one compare-and-swap.
//
// The shard lock is held across the load, the check and the reservation, for
// the same reason the per-item path holds it: releasing between reading the
// stored nonce and swapping it is the TOCTOU race that once let two goroutines
// both reserve the same nonce (see sendRawTransaction's own P0-AUDIT note).
func (s *EVMRPCServer) reserveOneSenderRun(sender string, items []*precomputedSendTx) {
	start := time.Now()

	lock := s.nonceShardFor(sender)
	lock.mu.Lock()
	defer lock.mu.Unlock()

	if lock.nonces[sender] == 0 {
		if dbNonce := s.state.LoadNonce(sender); dbNonce > 0 {
			lock.nonces[sender] = dbNonce
		}
	}
	stored := lock.nonces[sender]

	// How far does the run reach? Stop at the first item that is not exactly
	// the next nonce; everything from there on keeps the per-item path, which
	// produces the same "nonce too low"/"nonce too high" answers it does now.
	run := 0
	for _, p := range items {
		if p.tx.Nonce() != stored+uint64(run) {
			break
		}
		run++
	}
	if run < 2 {
		return
	}

	next := stored + uint64(run)
	reserved, err := s.state.ReserveNonce(sender, stored, next)
	if err != nil || !reserved {
		// Leave every item unmarked. The per-item path then runs exactly as
		// before and reports the real reason, rather than this pre-pass having
		// to reproduce that error handling.
		return
	}
	lock.nonces[sender] = next

	for i := 0; i < run; i++ {
		items[i].nonceReserved = true
	}

	// Counted so the win is visible in the same place the cost was found:
	// rpc_phases' nonce_ms should fall toward zero as coverage rises.
	noteBatchNonceRun(run, time.Since(start))
}
