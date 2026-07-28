package keeper

import (
	"fmt"
	"testing"
)

// note() used to allocate a fresh backing array and copy the whole thing on
// EVERY call once the shard was full. An allocation profile under 597-sender
// load put 3.29GB — 36.05% of every byte the node allocated — in this one
// function, which made it the largest single contributor to the 18.3% of CPU
// that allocation and GC were costing.
//
// This is the regression guard for the fix. It asserts the property that
// matters (steady state allocates nothing) rather than the implementation, so a
// future rewrite is free to change the shape as long as it does not reintroduce
// per-call allocation.
func TestTxMetaShard_NoteDoesNotAllocateInSteadyState(t *testing.T) {
	sh := &txMetaShard{
		status:   map[string]bool{},
		errMsg:   map[string]string{},
		senders:  map[string]string{},
		tos:      map[string]string{},
		deployed: map[string]string{},
	}
	// Fill past capacity so every measured call is an evicting one — the case
	// that used to copy the entire buffer.
	for i := 0; i < txMetaMaxPerShard+10; i++ {
		sh.note(fmt.Sprintf("0x%064x", i))
	}

	// Hashes are pre-built: generating them inside the measured function would
	// attribute the formatting's own allocations to note().
	hashes := make([]string, 128)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("0x%064x", i+txMetaMaxPerShard+10)
	}
	i := 0
	avg := testing.AllocsPerRun(100, func() {
		sh.note(hashes[i%len(hashes)])
		i++
	})
	if avg != 0 {
		t.Fatalf("note() allocated %.1f times per call in steady state; the whole point of the ring buffer is that it allocates zero", avg)
	}
}

// The ring must keep exactly the most recent txMetaMaxPerShard hashes and drop
// the oldest — the same policy the copying version had. This is what a wallet
// polling for its receipt right after submitting depends on, so a ring that
// silently evicted the WRONG entry would look like a lost transaction.
func TestTxMetaShard_EvictsOldestFirst(t *testing.T) {
	sh := &txMetaShard{
		status:   map[string]bool{},
		errMsg:   map[string]string{},
		senders:  map[string]string{},
		tos:      map[string]string{},
		deployed: map[string]string{},
	}

	total := txMetaMaxPerShard + 50
	for i := 0; i < total; i++ {
		h := fmt.Sprintf("h%d", i)
		sh.status[h] = true
		sh.senders[h] = "0xabc"
		sh.note(h)
	}

	if len(sh.status) != txMetaMaxPerShard {
		t.Fatalf("status holds %d entries, want exactly the %d-entry bound", len(sh.status), txMetaMaxPerShard)
	}

	// The 50 oldest must be gone.
	for i := 0; i < 50; i++ {
		h := fmt.Sprintf("h%d", i)
		if _, ok := sh.status[h]; ok {
			t.Fatalf("%s is the %d-th oldest of %d and should have been evicted, but is still present", h, i+1, total)
		}
	}
	// Every one of the most recent txMetaMaxPerShard must have survived.
	for i := 50; i < total; i++ {
		h := fmt.Sprintf("h%d", i)
		if _, ok := sh.status[h]; !ok {
			t.Fatalf("%s is among the most recent %d and must still be cached, but was evicted", h, txMetaMaxPerShard)
		}
		if sh.senders[h] != "0xabc" {
			t.Fatalf("%s lost its senders entry while its status entry survived — the five caches must evict together", h)
		}
	}
}
