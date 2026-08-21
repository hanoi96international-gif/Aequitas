package wal

import (
	"sync/atomic"
	"time"
)

// What the single WAL writer actually does per second.
//
// WHY THIS EXISTS. Throughput sat at ~3,000 TPS on 2026-08-21 with about one
// core of six in use, and three separate hypotheses were measured and rejected
// in turn: parallel replay (already 99.8% parallel on a real 50,000-transfer
// block), WAL flush batch size (the default beat every smaller value, because
// items_per_flush was 402 against a 4,000 cap that therefore never bound), and
// WAL flush interval (shortening it moved addrs_per_flush 348 -> 134 and
// hold_ms 40 -> 21 exactly as predicted, and throughput did not follow).
//
// What survives all three is the shape of the number itself: throughput scales
// with neither cores nor senders, which is the signature of one serialised
// stage. There is exactly one -- runWriter, a single goroutine whose loop is
// one buffer, one Write, one Sync per batch. Its ceiling is therefore
//
//	records/second = fsyncs/second x records per batch
//
// and this project has measured the disk at 133-462 fsyncs/s. At ~10 records
// per batch that is 1,300-4,600/s, which brackets the observed 3,000.
//
// That is an arithmetic fit, not evidence. The two numbers it rests on --
// how large batches actually get, and how long a sync actually takes -- were
// never recorded anywhere, so the fit could equally be a coincidence. These
// counters record them, so the next decision is made on the mechanism rather
// than on a plausible story. Same rule the flush path already follows with
// WALFlushStats, and it is what turned three plausible stories into three
// rejected ones.
//
// Cost: four atomic adds per batch, not per record.

var writerStats struct {
	batches    atomic.Int64
	records    atomic.Int64
	syncNanos  atomic.Int64
	syncMaxNs  atomic.Int64
	writeNanos atomic.Int64
	maxBatch   atomic.Int64
}

func noteWriterBatch(records int, write, sync time.Duration) {
	writerStats.batches.Add(1)
	writerStats.records.Add(int64(records))
	writerStats.writeNanos.Add(int64(write))
	writerStats.syncNanos.Add(int64(sync))
	for {
		cur := writerStats.syncMaxNs.Load()
		if int64(sync) <= cur || writerStats.syncMaxNs.CompareAndSwap(cur, int64(sync)) {
			break
		}
	}
	for {
		cur := writerStats.maxBatch.Load()
		if int64(records) <= cur || writerStats.maxBatch.CompareAndSwap(cur, int64(records)) {
			break
		}
	}
}

// WriterStats reports the writer's behaviour for an operator endpoint.
//
// avg_batch is the number that decides everything here: the writer's ceiling is
// fsyncs/second times this. A small average with a large MaxBatchSize means the
// batch cap is not the limit and requests are simply not arriving together --
// which points back at whatever is holding callers up before they reach Append,
// not at the WAL.
func WriterStats() map[string]interface{} {
	b := writerStats.batches.Load()
	r := writerStats.records.Load()
	avgBatch := float64(0)
	avgSyncUs := int64(0)
	avgWriteUs := int64(0)
	if b > 0 {
		avgBatch = float64(r) / float64(b)
		avgSyncUs = writerStats.syncNanos.Load() / b / 1000
		avgWriteUs = writerStats.writeNanos.Load() / b / 1000
	}
	return map[string]interface{}{
		"batches":         b,
		"records":         r,
		"avg_batch":       avgBatch,
		"max_batch":       writerStats.maxBatch.Load(),
		"cfg_max_batch":   MaxBatchSize,
		"cfg_max_wait_us": MaxBatchWait.Microseconds(),
		"sync_avg_us":     avgSyncUs,
		"sync_max_us":     writerStats.syncMaxNs.Load() / 1000,
		"write_avg_us":    avgWriteUs,
	}
}
