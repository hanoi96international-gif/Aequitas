package keeper

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
)

// SCALING_ARCHITECTURE.md's "Realistisches Zielbild" closes with three
// quantities it explicitly says must be MEASURED rather than assumed before
// 50k TPS can be called reachable. Two of them had no harness at all. This
// file adds one.
//
// Everything here is opt-in via AEQUITAS_SCALING_BENCH=1 so a normal `go test
// ./...` (and therefore CI and the deploy gate) stays fast. Run on the real
// target hardware, not a laptop:
//
//	AEQUITAS_SCALING_BENCH=1 go test ./x/humanity/keeper/ \
//	    -run 'TestScalingUnknown' -v -timeout 20m
//
// The numbers only mean something on the machine the node actually runs on —
// that is the whole point of the entries in the roadmap.

func scalingBenchEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("AEQUITAS_SCALING_BENCH") != "1" {
		t.Skip("set AEQUITAS_SCALING_BENCH=1 to run the scaling measurements (they are slow and hardware-specific)")
	}
}

// targetTPS is the number this whole roadmap exists to reach. Used to turn a
// measured per-operation cost into the answer that actually matters: how many
// CPU cores does this one step consume at the target rate.
const targetTPS = 50000

// TestScalingUnknown_SignatureVerification measures secp256k1 public-key
// recovery — the cost every transaction pays before any state is touched, and
// the one cost that cannot be batched away by better I/O scheduling because it
// is pure CPU.
//
// A live 30s profile of sendRawTransaction (see maybePruneTxReceipts' comment
// in evm_storage.go) already attributed 17.5% of that path to
// types.Sender/Ecrecover while the node was doing ~150 TPS. This test answers
// the forward-looking question that profile cannot: at 50,000 TPS, how many
// cores does recovery alone occupy, and does it scale across cores or contend.
func TestScalingUnknown_SignatureVerification(t *testing.T) {
	scalingBenchEnabled(t)

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	digest := crypto.Keccak256([]byte("aequitas-signature-verification-benchmark"))
	sig, err := crypto.Sign(digest, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Sanity: the signature must actually recover, or we would be timing a
	// fast error path instead of real work.
	if _, err := crypto.Ecrecover(digest, sig); err != nil {
		t.Fatalf("reference recovery failed: %v", err)
	}

	const singleN = 20000
	start := time.Now()
	for i := 0; i < singleN; i++ {
		if _, err := crypto.Ecrecover(digest, sig); err != nil {
			t.Fatalf("recovery %d failed: %v", i, err)
		}
	}
	singleElapsed := time.Since(start)
	perOp := singleElapsed / singleN
	singleRate := float64(singleN) / singleElapsed.Seconds()
	coresAtTarget := float64(targetTPS) / singleRate

	t.Logf("single-core: %d recoveries in %s → %.0f/s (%.1f µs each) → %.1f cores needed at %d TPS",
		singleN, singleElapsed.Round(time.Millisecond), singleRate,
		float64(perOp.Nanoseconds())/1000.0, coresAtTarget, targetTPS)

	// Does it scale across cores, or is there hidden contention? Recovery is
	// pure computation over stack-local data, so near-linear is expected —
	// but "expected" is exactly what this file exists to replace.
	workers := runtime.NumCPU()
	perWorker := singleN / workers
	if perWorker < 1000 {
		perWorker = 1000
	}
	var wg sync.WaitGroup
	var failures atomic.Int64
	start = time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if _, err := crypto.Ecrecover(digest, sig); err != nil {
					failures.Add(1)
					return
				}
			}
		}()
	}
	wg.Wait()
	parElapsed := time.Since(start)
	if failures.Load() > 0 {
		t.Fatalf("%d parallel recoveries failed", failures.Load())
	}
	parTotal := perWorker * workers
	parRate := float64(parTotal) / parElapsed.Seconds()

	t.Logf("%d cores:  %d recoveries in %s → %.0f/s (%.1fx over single core)",
		workers, parTotal, parElapsed.Round(time.Millisecond), parRate, parRate/singleRate)

	if parRate >= float64(targetTPS) {
		t.Logf("VERDICT: %d cores clear %d TPS of signature verification with %.0f%% headroom",
			workers, targetTPS, (parRate/float64(targetTPS)-1)*100)
	} else {
		t.Logf("VERDICT: %d cores reach only %.0f/s — %d TPS needs ~%.1f cores for signature verification ALONE, "+
			"before any state work. Batch verification or a cheaper curve for a non-EVM transfer type would be required.",
			workers, parRate, targetTPS, float64(targetTPS)/(parRate/float64(workers)))
	}
}

// TestScalingUnknown_GCUnderAllocationLoad measures what the Go garbage
// collector does when the node allocates at the rate 50k TPS implies.
//
// The roadmap lists GC pauses as an open unknown and warns against optimizing
// before measuring. This produces the number to decide on: total pause time
// and worst individual pause while a workload allocates transaction-shaped
// objects as fast as it can.
//
// It deliberately allocates the SAME shapes the hot path does (Transaction
// structs plus their string fields) rather than synthetic byte slices, since
// pointer-dense objects are what actually cost the mark phase — the profile
// behind PR #47 showed exactly that: short-lived maps drove ~27% of runtime
// into gcBgMarkWorker/scanObject.
func TestScalingUnknown_GCUnderAllocationLoad(t *testing.T) {
	scalingBenchEnabled(t)

	var before, after debug.GCStats
	debug.ReadGCStats(&before)

	const batches = 500
	const perBatch = 1000
	sink := make([]Transaction, 0, perBatch)
	start := time.Now()
	for b := 0; b < batches; b++ {
		sink = sink[:0]
		for i := 0; i < perBatch; i++ {
			sink = append(sink, Transaction{
				Type:   "transfer",
				Wallet: fmt.Sprintf("0x%040x", b*perBatch+i),
				To:     fmt.Sprintf("0x%040x", b*perBatch+i+1),
				Amount: 1.0,
				TxHash: fmt.Sprintf("0x%064x", b*perBatch+i),
			})
		}
		// Keep the batch reachable just long enough to be promoted, the way a
		// block's transaction list is while it is being assembled and hashed.
		if len(sink) != perBatch {
			t.Fatalf("batch %d: unexpected length", b)
		}
	}
	elapsed := time.Since(start)
	debug.ReadGCStats(&after)

	allocated := batches * perBatch
	rate := float64(allocated) / elapsed.Seconds()
	cycles := after.NumGC - before.NumGC
	pause := after.PauseTotal - before.PauseTotal

	var worst time.Duration
	// PauseQuantiles is cheaper to reason about than scanning the ring buffer,
	// and after.Pause holds the most recent 256 pauses newest-first.
	for i, p := range after.Pause {
		if i >= int(cycles) {
			break
		}
		if p > worst {
			worst = p
		}
	}

	t.Logf("allocated %d transaction-shaped objects in %s (%.0f/s, %.1fx the %d TPS target)",
		allocated, elapsed.Round(time.Millisecond), rate, rate/float64(targetTPS), targetTPS)
	t.Logf("GC: %d cycles, total pause %s (%.3f%% of wall time), worst single pause %s",
		cycles, pause.Round(time.Microsecond),
		100*float64(pause)/float64(elapsed), worst.Round(time.Microsecond))

	// BLOCK_TIME is 1s; a single pause approaching that would be a consensus
	// problem, not just a latency one. This is a report, not a gate — the
	// point is to have the number on the real hardware.
	if worst > 100*time.Millisecond {
		t.Logf("WARNING: worst pause %s is a significant fraction of a 1s BLOCK_TIME — "+
			"object pooling for the transaction path would be justified", worst)
	}
}

// TestScalingUnknown_WALFsyncOnThisHost re-runs the WAL group-commit
// throughput measurement in a form that can be executed ON A CONTABO NODE.
//
// SCALING_ARCHITECTURE.md records ~112,700 appends/s but flags the number as
// having come from a cloud container with an overlay filesystem, explicitly
// NOT the target hardware, and says to re-measure before any production
// decision. AEQUITAS_WAL_BENCH_PATH points the log at a real directory on the
// host (e.g. /root/aequitas-wal-data) so the number reflects that disk.
func TestScalingUnknown_WALFsyncOnThisHost(t *testing.T) {
	scalingBenchEnabled(t)

	dir := os.Getenv("AEQUITAS_WAL_BENCH_PATH")
	if dir == "" {
		dir = t.TempDir()
		t.Logf("AEQUITAS_WAL_BENCH_PATH unset — measuring %s (set it to the real WAL directory, e.g. /root/aequitas-wal-data, for a number that means something)", dir)
	}
	path := fmt.Sprintf("%s/scaling_bench_%d.wal", dir, time.Now().UnixNano())
	defer os.Remove(path)

	w, err := wal.Open(path)
	if err != nil {
		t.Fatalf("open WAL at %s: %v", path, err)
	}
	defer w.Close()

	const concurrent = 1000
	const perWorker = 20
	payload := []byte(`{"from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","amount":1.5}`)

	var wg sync.WaitGroup
	var failed atomic.Int64
	start := time.Now()
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				if _, err := w.Append(payload); err != nil {
					failed.Add(1)
					return
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	if failed.Load() > 0 {
		t.Fatalf("%d appends failed", failed.Load())
	}

	total := concurrent * perWorker
	rate := float64(total) / elapsed.Seconds()
	t.Logf("WAL group-commit on %s: %d appends from %d goroutines in %s → %.0f appends/s",
		dir, total, concurrent, elapsed.Round(time.Millisecond), rate)
	if rate < float64(targetTPS) {
		t.Logf("VERDICT: %.0f appends/s is BELOW the %d TPS target — on this disk the WAL fsync is the binding constraint, "+
			"not the in-memory path", rate, targetTPS)
	} else {
		t.Logf("VERDICT: %.0f appends/s clears the %d TPS target with %.1fx headroom on this disk",
			rate, targetTPS, rate/float64(targetTPS))
	}
}

// TestScalingUnknown_Ed25519VsSecp256k1 measures the one alternative that can
// move the signature wall.
//
// secp256k1 recovery was measured on Contabo2 at 101.1 µs per operation with
// cgo/libsecp256k1 (the production build), giving 40,970/s across 6 cores —
// ~7.3 cores for 50k TPS, and roughly 10 for 100k. That is a CPU budget the
// target hardware does not have, and no amount of scheduling work changes it:
// FAFO (arXiv 2507.10757) reaches 1.1M TPS by removing EXECUTION contention,
// on 96 cores, on synthetic workloads that do not appear to verify signatures
// at all. Once execution stops being the limit, this is the whole remaining
// budget.
//
// Ed25519 is the standard answer because verification BATCHES: checking n
// signatures together costs far less than n individual checks, which secp256k1
// ECDSA recovery cannot do at all (you cannot batch a recovery — there is no
// public key to aggregate against until you have done the work).
//
// This measures stdlib crypto/ed25519, i.e. SINGLE verification, deliberately
// WITHOUT adding a batch-verification dependency: the point is to decide
// whether that dependency is worth taking, and adding it first would be
// assuming the answer. Single verification is therefore a LOWER BOUND on what
// Ed25519 offers — batch verification is typically several times better again.
func TestScalingUnknown_Ed25519VsSecp256k1(t *testing.T) {
	scalingBenchEnabled(t)

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	msg := []byte("aequitas-signature-verification-benchmark")
	sig := ed25519.Sign(priv, msg)
	if !ed25519.Verify(pub, msg, sig) {
		t.Fatal("reference verification failed — the benchmark would be timing a fast error path")
	}

	// Same shape and count as the secp256k1 test above, so the two numbers are
	// directly comparable rather than needing to be normalised.
	const singleN = 20000
	start := time.Now()
	for i := 0; i < singleN; i++ {
		if !ed25519.Verify(pub, msg, sig) {
			t.Fatalf("verification %d failed", i)
		}
	}
	elapsed := time.Since(start)
	perOp := elapsed / singleN
	rate := float64(singleN) / elapsed.Seconds()
	coresAtTarget := float64(targetTPS) / rate

	t.Logf("ed25519 single-core: %d verifications in %s → %.0f/s (%.1f µs each) → %.1f cores needed at %d TPS",
		singleN, elapsed.Round(time.Millisecond), rate, float64(perOp.Nanoseconds())/1000, coresAtTarget, targetTPS)

	// All cores, matching the secp256k1 test, so the comparison holds at the
	// scale the decision is actually about.
	cores := runtime.NumCPU()
	perCore := singleN / cores
	var wg sync.WaitGroup
	start = time.Now()
	for c := 0; c < cores; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perCore; i++ {
				ed25519.Verify(pub, msg, sig)
			}
		}()
	}
	wg.Wait()
	multiElapsed := time.Since(start)
	multiRate := float64(perCore*cores) / multiElapsed.Seconds()

	t.Logf("ed25519 %d cores: %d verifications in %s → %.0f/s (%.1fx over single core)",
		cores, perCore*cores, multiElapsed.Round(time.Millisecond), multiRate, multiRate/rate)
	t.Logf("VERDICT: at %d TPS ed25519 single-verification needs ~%.1f cores against secp256k1 recovery's measured ~7.3. "+
		"Batch verification, which secp256k1 cannot do at all, would improve this further — this number is the floor, not the ceiling.",
		targetTPS, coresAtTarget)
}
