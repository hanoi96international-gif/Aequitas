package keeper

import (
	"context"
	"fmt"
)

// account_write_buffer.go removes the last per-transaction database round
// trip from block replay (Roadmap step 6, "parallel execution of disjoint
// transfers" — see this file's closing note on why the answer turned out
// not to be parallelism).
//
// MEASURED, in this order, on a local Postgres replaying a block of
// disjoint transfers:
//
//	baseline                              382 tx/s   3.0 SQL statements/tx
//	+ expression index on lower(address) 1250 tx/s   3.0   (each was a seq scan)
//	+ ctx-scoped resolved-address set    2000 tx/s   2.0   (dropped the re-lookup)
//	+ this buffer                          see replay_throughput_bench_test.go
//
// A CPU profile at the baseline showed 13% CPU and 87% wait: replay is
// bound by HOW MANY times it talks to Postgres, not by what it computes.
// That is why the reverted 2026-07-25 attempt at running transfers on
// several goroutines could not have paid off even had it been safe — the
// arithmetic it parallelised was never the cost. Cutting statements is.
//
// The mechanism: while a buffer is present in the operation's ctx,
// saveAccountToDBCtx records the account instead of writing it, and one
// multi-row upsert (saveAccountsToDBBatchCtx, which already existed for the
// transfer batcher) writes all of them at the end. Accounts are held by
// POINTER, so an account mutated by several transactions in one block
// collapses into a single row write carrying its final state.
//
// Why that is safe rather than merely faster:
//
//   - Reads inside the block never consult the database for an account
//     that the block touched — blockTouchedAddresses warms every one of them
//     into cs.accounts before replay begins, and every apply*DeltaLocked
//     reads that in-memory copy. So a deferred row write is invisible to
//     the only readers that exist during the block.
//   - accountSetXOR arithmetic is unchanged. updateAccountLeafLocked XORs
//     an account's cached OLD leaf out and its new one in; doing that once
//     with the final state, instead of once per intermediate state, yields
//     the identical accumulator (each intermediate leaf would be XORed in
//     and straight back out again). The flush happens before the StateRoot
//     comparison, so the root is computed against fully-written state.
//   - Version is an optimistic-locking counter and is deliberately NOT part
//     of accountLeaf, so collapsing several writes into one — which
//     increments it once rather than N times — cannot change consensus
//     state. saveAccountsToDBBatchCtx does the same version check and the
//     same conflict reporting the single-row path does.
//   - Error semantics are preserved: a failure at flush is returned to
//     replayTransactions, which rolls the whole block back — exactly what a
//     failure of any individual save already did, since every one of them
//     sets hardFailure.
//
// The one case that would NOT be safe is a transaction type that reads
// account rows back out of SQL mid-block — ubi_distribution enumerates
// every human with `WHERE is_human = true`, and would miss a register_human
// still sitting in the buffer. FlushBeforeNonTransfer handles exactly that:
// replay flushes before any transaction that is not a plain transfer, so
// such a read never runs against a partially-written table.

// accountWriteBuffer collects account row writes for one operation.
//
// Not safe for concurrent use, and deliberately so: it lives in a single
// operation's ctx, and that operation holds cs.mu for its whole duration.
type accountWriteBuffer struct {
	order []*AccountState
	seen  map[string]int
}

type writeBufferKey struct{}

// withAccountWriteBuffer returns ctx carrying buf, plus buf itself.
func withAccountWriteBuffer(ctx context.Context, sizeHint int) (context.Context, *accountWriteBuffer) {
	buf := &accountWriteBuffer{
		order: make([]*AccountState, 0, sizeHint),
		seen:  make(map[string]int, sizeHint),
	}
	return context.WithValue(ctx, writeBufferKey{}, buf), buf
}

func accountWriteBufferFrom(ctx context.Context) *accountWriteBuffer {
	buf, _ := ctx.Value(writeBufferKey{}).(*accountWriteBuffer)
	return buf
}

// add records acc for the next flush. Repeated adds of the same address
// keep ONE entry: the account is held by pointer, so the entry already
// refers to whatever its latest state is.
//
// Re-adding by address rather than by pointer identity matters — replay can
// legitimately hold two different *AccountState values for one address
// across a rollback restore, and writing both would make the batch's
// optimistic-version check see the same row twice.
func (b *accountWriteBuffer) add(acc *AccountState) {
	if acc == nil {
		return
	}
	if i, ok := b.seen[acc.Address]; ok {
		b.order[i] = acc
		return
	}
	b.seen[acc.Address] = len(b.order)
	b.order = append(b.order, acc)
}

// len reports how many distinct accounts are pending.
func (b *accountWriteBuffer) len() int { return len(b.order) }

// drain returns the pending accounts and empties the buffer, so a flush can
// never write the same pending set twice.
func (b *accountWriteBuffer) drain() []*AccountState {
	out := b.order
	b.order = make([]*AccountState, 0, cap(out))
	b.seen = make(map[string]int, len(out))
	return out
}

// flushAccountWriteBuffer writes everything the buffer holds as one
// multi-row upsert and empties it. A no-op when ctx carries no buffer or
// the buffer is empty, so callers can flush unconditionally.
//
// IMPORTANT: the write itself must NOT go through the buffer again, so this
// passes a ctx with the buffer stripped — otherwise the batch writer's own
// bookkeeping path could re-buffer what it just wrote.
func (cs *ChainState) flushAccountWriteBuffer(ctx context.Context) error {
	buf := accountWriteBufferFrom(ctx)
	if buf == nil || buf.len() == 0 {
		return nil
	}
	pending := buf.drain()
	if err := cs.saveAccountsToDBBatchCtx(context.WithValue(ctx, writeBufferKey{}, (*accountWriteBuffer)(nil)), pending); err != nil {
		return fmt.Errorf("deferred account write flush (%d accounts): %w", len(pending), err)
	}
	return nil
}
