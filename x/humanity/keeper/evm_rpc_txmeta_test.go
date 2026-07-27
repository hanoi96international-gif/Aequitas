package keeper

import "testing"

// The six txHash-keyed caches had no eviction at all: each transaction added
// roughly four permanent entries and they grew for the whole process lifetime.
// That is a leak, and it also made the lock progressively more expensive, since
// Go rehashes a growing map while the caller holds it.
func TestTxMetaShard_BoundsEveryCache(t *testing.T) {
	s := &EVMRPCServer{}
	s.initTxMetaShards()

	const n = txMetaMax + 20000
	var lastHash string
	for i := 0; i < n; i++ {
		h := "0x" + itoa(i)
		sh := s.txMetaShardFor(h)
		sh.mu.Lock()
		sh.status[h] = true
		sh.errMsg[h] = "e"
		sh.senders[h] = "0xfrom"
		sh.tos[h] = "0xto"
		sh.deployed[h] = "0xcontract"
		sh.note(h)
		sh.mu.Unlock()
		lastHash = h
	}

	total := 0
	for i := range s.txMeta {
		sh := &s.txMeta[i]
		for name, got := range map[string]int{
			"status": len(sh.status), "errMsg": len(sh.errMsg),
			"senders": len(sh.senders), "tos": len(sh.tos),
			"deployed": len(sh.deployed), "order": len(sh.order),
		} {
			if got > txMetaMaxPerShard {
				t.Fatalf("shard %d %s holds %d, past its %d share — the caches are still unbounded and will keep rehashing under the lock", i, name, got, txMetaMaxPerShard)
			}
		}
		total += len(sh.status)
	}
	if total > txMetaMax {
		t.Fatalf("across all shards %d entries survive, past the %d budget", total, txMetaMax)
	}

	// The newest entry must survive: a wallet polls right after it gets its
	// hash back, so evicting the most recent write would break the exact case
	// these caches exist for.
	sh := s.txMetaShardFor(lastHash)
	sh.mu.Lock()
	_, ok := sh.status[lastHash]
	sh.mu.Unlock()
	if !ok {
		t.Fatal("the most recently recorded transaction must never be the one evicted")
	}
}

// Sharding is only correct if a hash always lands on the same shard. If it did
// not, the same transaction's status could be written under one lock and read
// under another — the map would be guarded by two mutexes, which is no guard.
func TestTxMetaShardFor_IsStableAndSpreads(t *testing.T) {
	s := &EVMRPCServer{}
	s.initTxMetaShards()

	const h = "0xabc123"
	if s.txMetaShardFor(h) != s.txMetaShardFor(h) {
		t.Fatal("the same hash must always resolve to the same shard, or its entry is guarded by two different locks")
	}

	seen := map[*txMetaShard]bool{}
	for i := 0; i < 5000; i++ {
		seen[s.txMetaShardFor("0x"+itoa(i))] = true
	}
	// Perfect spread is not required, but landing on only a handful of shards
	// would mean transactions still queue behind each other for no reason.
	if len(seen) < txMetaShardCount/2 {
		t.Fatalf("hashes reached only %d of %d shards — contention would survive the sharding", len(seen), txMetaShardCount)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
