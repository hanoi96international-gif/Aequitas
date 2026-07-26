package keeper

import "testing"

// The four txHash-keyed caches had no eviction: each transaction added about
// four permanent entries and they grew for the whole process lifetime. That is
// a leak, but the throughput cost came first — Go rehashes a growing map while
// the caller holds the lock, and all four live under one mutex taken eleven
// times across evm_rpc.go. Under load on Contabo2 every in-flight request sat
// in runtime_SemacquireMutex there.
func TestNoteTxLocked_BoundsEveryTxKeyedCache(t *testing.T) {
	s := &EVMRPCServer{
		txStatus:  make(map[string]bool),
		txError:   make(map[string]string),
		txSenders: make(map[string]string),
		txTos:     make(map[string]string),
	}
	const n = txMetaMax + 5000
	for i := 0; i < n; i++ {
		h := "0x" + string(rune('a'+i%26)) + itoa(i)
		s.txStatus[h] = true
		s.txError[h] = "e"
		s.txSenders[h] = "0xfrom"
		s.txTos[h] = "0xto"
		s.noteTxLocked(h)
	}
	for name, got := range map[string]int{
		"txStatus": len(s.txStatus), "txError": len(s.txError),
		"txSenders": len(s.txSenders), "txTos": len(s.txTos),
		"txOrder": len(s.txOrder),
	} {
		if got > txMetaMax {
			t.Fatalf("%s grew to %d, past the %d bound — the cache is still unbounded and will keep rehashing under the lock", name, got, txMetaMax)
		}
	}
	// The newest entry must survive: a wallet polls right after it gets its
	// hash back, so evicting the most recent write would break exactly the
	// case these caches exist for.
	newest := "0x" + string(rune('a'+(n-1)%26)) + itoa(n-1)
	if _, ok := s.txStatus[newest]; !ok {
		t.Fatal("the most recently recorded transaction must never be the one evicted")
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
