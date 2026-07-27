package keeper

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Batched nonce reservation.
//
// WHY. A CPU profile taken under load on Contabo2 attributed 23.9% of all
// samples to ReserveNonce — one synchronous Postgres round trip per
// transaction, in its own autocommit transaction, before the transfer's own
// transaction even begins. Postgres reported 29,293,162 committed transactions
// against 3,096,743 chain transactions: roughly 9.5 database transactions for
// every transfer. Meanwhile the node ran at 198% of 600% available CPU with a
// 99.88% buffer-cache hit rate, so the cost was never the query or the disk —
// it was the sheer number of round trips.
//
// WHAT THIS DOES NOT CHANGE. The reservation is still an exact
// compare-and-swap per address: a row moves from `expected` to `next` only if
// it still holds `expected`, and RETURNING reports which ones actually did.
// Folding many of those into one statement changes how they travel to
// Postgres, not what they mean. That matters because this is the replay guard
// on the payment path — a weaker check here would mean a transaction could be
// accepted twice.
//
// WHY TWO ENTRIES FOR ONE ADDRESS CANNOT COLLIDE IN A BATCH. ReserveNonce is
// called while holding that sender's nonce shard lock (see sendRawTransaction),
// so a second request for the same address cannot even reach this code until
// the first has returned. A batch therefore always holds distinct addresses,
// which is what makes a single multi-row UPDATE equivalent to N separate ones.
//
// FIRST-EVER TRANSACTIONS ARE NOT BATCHED. expected == 0 means the account has
// no row yet and needs an INSERT ... ON CONFLICT DO NOTHING, which does not
// combine cleanly with the UPDATE above. That path happens once per address in
// its lifetime, so it keeps its own round trip rather than complicating the
// hot one.

// nonceBatchMaxSize bounds one statement. Well above the concurrency any
// single node has been observed to sustain, so the batch is limited by how
// many requests actually arrive rather than by this number.
const nonceBatchMaxSize = 512

// nonceBatchMaxWait is how long the collector waits for more reservations
// before sending what it has. Same tradeoff as transferBatchMaxWait, and the
// same value: every caller blocks until its batch returns, so a longer wait
// adds latency to each one and slows the arrival of the next.
const nonceBatchMaxWait = 1 * time.Millisecond

type nonceReserveRequest struct {
	address  string
	expected uint64
	next     uint64
	result   chan nonceReserveResult
}

type nonceReserveResult struct {
	reserved bool
	err      error
}

// ensureNonceBatcherStarted brings the collector up on first use, so a
// ChainState that never reserves a nonce never starts a goroutine.
func (cs *ChainState) ensureNonceBatcherStarted() {
	cs.nonceBatchOnce.Do(func() {
		cs.nonceBatchCh = make(chan *nonceReserveRequest, nonceBatchMaxSize*4)
		go cs.runNonceBatcher()
	})
}

// runNonceBatcher collects reservations and executes each batch as one
// statement. Mirrors runTransferBatcher's shape deliberately: same collect
// loop, same bounded wait, so the two behave alike under load.
func (cs *ChainState) runNonceBatcher() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVERED] nonce batcher: %v\n", r)
		}
	}()
	for first := range cs.nonceBatchCh {
		batch := []*nonceReserveRequest{first}
		timer := time.NewTimer(nonceBatchMaxWait)
	collect:
		for len(batch) < nonceBatchMaxSize {
			select {
			case req := <-cs.nonceBatchCh:
				batch = append(batch, req)
			case <-timer.C:
				break collect
			}
		}
		timer.Stop()
		cs.processNonceBatch(batch)
	}
}

// processNonceBatch runs one multi-row compare-and-swap and answers every
// caller.
//
// A caller that never hears back would hang forever holding its nonce shard
// lock, so every path here must reply to every request — including the error
// paths.
func (cs *ChainState) processNonceBatch(batch []*nonceReserveRequest) {
	reply := func(req *nonceReserveRequest, reserved bool, err error) {
		select {
		case req.result <- nonceReserveResult{reserved: reserved, err: err}:
		default: // caller gave up; never block the batcher on it
		}
	}
	if cs.db == nil {
		for _, req := range batch {
			reply(req, true, nil)
		}
		return
	}

	// VALUES (...),(...) with the parameters flattened in the same order.
	var sb strings.Builder
	args := make([]interface{}, 0, len(batch)*3)
	sb.WriteString(`UPDATE evm_nonces AS n SET nonce = v.next FROM (VALUES `)
	for i, req := range batch {
		if i > 0 {
			sb.WriteByte(',')
		}
		base := i * 3
		sb.WriteString(`($` + strconv.Itoa(base+1) + `::text,$` + strconv.Itoa(base+2) + `::bigint,$` + strconv.Itoa(base+3) + `::bigint)`)
		args = append(args, req.address, int64(req.expected), int64(req.next))
	}
	// The WHERE is the compare-and-swap, unchanged from the single-row version:
	// a row moves only if it still holds exactly the nonce the caller expected.
	sb.WriteString(`) AS v(address, expected, next) WHERE n.address = v.address AND n.nonce = v.expected RETURNING n.address`)

	rows, err := cs.db.Query(sb.String(), args...)
	if err != nil {
		// Report the failure rather than guessing: claiming a reservation
		// succeeded when the database never saw it would let the same nonce be
		// used twice.
		for _, req := range batch {
			reply(req, false, fmt.Errorf("batched nonce reservation: %w", err))
		}
		return
	}
	won := make(map[string]bool, len(batch))
	for rows.Next() {
		var addr string
		if scanErr := rows.Scan(&addr); scanErr == nil {
			won[strings.ToLower(addr)] = true
		}
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		for _, req := range batch {
			reply(req, false, fmt.Errorf("batched nonce reservation: %w", rowsErr))
		}
		return
	}
	// Anyone not in RETURNING lost the compare-and-swap — exactly what a
	// single-row UPDATE reporting zero affected rows means.
	for _, req := range batch {
		reply(req, won[req.address], nil)
	}
}

// reserveNonceBatched queues one reservation and waits for its answer.
func (cs *ChainState) reserveNonceBatched(address string, expected, next uint64) (bool, error) {
	cs.ensureNonceBatcherStarted()
	req := &nonceReserveRequest{
		address:  address,
		expected: expected,
		next:     next,
		// Buffered so processNonceBatch never blocks writing the answer.
		result: make(chan nonceReserveResult, 1),
	}
	cs.nonceBatchCh <- req
	res := <-req.result
	return res.reserved, res.err
}
