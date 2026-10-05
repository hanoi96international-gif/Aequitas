package keeper

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// numAccountShards partitions account storage into this many independent
// locks/maps. Its own per-shard mutexes are what let transfer_concurrent.go
// and transfer_wal.go's TryLockAddrs take a non-blocking, address-specific
// lock instead of the full cs.mu -- concurrent access to DIFFERENT shards
// never contends on the same lock, the entire point of sharding.
//
// FIX (2026-07-23, 50k-TPS-goal TPS-benchmark investigation): raised from
// the original 64 to 16384 after measuring real shard-collision cost. 64
// was picked as "more than typical core counts" but never tuned against an
// actual concurrent-address workload -- with 100 concurrently active
// addresses (a real benchmark, not a hypothetical), the birthday paradox
// against only 64 shards means roughly half of them collide with some
// other address on the same shard, forcing TryLockAddrs to bail to the
// slow batcher path far more often than the addresses' own (disjoint,
// mutually unrelated) access pattern would suggest -- confirmed live via
// CPU profiling: processTransferBatch accounted for half of cumulative CPU
// time in a benchmark scenario specifically designed to avoid the slow
// path. Disjoint-recipient WAL-path throughput roughly doubled (measured
// ~1400-2100 TPS -> ~3100-3750 TPS across repeated runs) after raising the
// shard count; 4096 already captured most of that gain, 16384 a further
// small increment, per-shard cost (one sync.Mutex + one lazily-allocated
// map) being cheap enough that erring toward more headroom for a larger
// real-world concurrent-account count costs essentially nothing. A
// contended single hot address (e.g. the same recipient) is unaffected by
// this at any shard count, by construction -- more shards only helps
// address sets that are ACTUALLY unrelated stop colliding by accident.
// 2026-09-01: aus der Konstanten wurde eine Variable, und die Vorgabe stieg
// von 16.384 auf 262.144.
//
// # WARUM JETZT UND NICHT FRUEHER
//
// Die Erhoehung war seit Langem der rechnerisch staerkste Hebel gegen
// Shard-Kollisionen und trotzdem verboten, weil sie am 23.07.2026 einmal
// katastrophal war: bootstrapMultiplierLocked zaehlte damals Menschen ueber
// cs.accounts.Range(), also ueber ALLE Shards, JE UEBERWEISUNG -- gemessene
// 57 % der gesamten Rechenzeit, nachdem die Zahl auf 16.384 gestiegen war.
// Mehr Shards machten den Knoten damals linear langsamer.
//
// Diese Ursache ist behoben (humanCountLocked ist in cs.useDB-Betrieb ein
// gepflegter Zaehler, O(1)) und seit dem 29.08.2026 durch einen Waechter
// gesichert: range_auf_heissem_pfad_test.go verfolgt den Aufrufgraphen von
// acht Einstiegspunkten und schlaegt an, wenn accounts.Range je wieder auf
// den Ueberweisungspfad geraet. Erst damit ist diese Zahl ueberhaupt
// diskutierbar.
//
// # DIE RECHNUNG
//
// Eine Ueberweisung sperrt zwei Shards. Bei C gleichzeitigen Ueberweisungen
// sind 2C Shards belegt, und die Wahrscheinlichkeit, dass eine neue kollidiert,
// ist 1-(1-2C/N)^2. Gemessen am 01.09.2026 mit 400 disjunkten Paaren:
//
//	N =  16.384, 2C = 800  ->  9,3 % erwartet, 7,5 % gemessen
//	N = 262.144, 2C = 800  ->  0,61 %
//
// Jede Kollision faellt auf den Buendler zurueck, und der haelt die GLOBALE
// Schreibsperre -- Go's RWMutex sperrt ankommende Leser aus, sobald ein
// Schreiber ansteht. Ein Zwoelftel der Rueckfaelle heisst ein Zwoelftel der
// Aussperrung.
//
// # WAS ES KOSTET
//
// GEMESSEN (TestShardZahl_WasSieKostet, 01.09.2026), nicht geschaetzt -- die
// erste Schaetzung lag beim Speicher um Faktor 1,7 daneben:
//
//	Shards    16.384:  1,6 MB,  Range   307 us,  Bau   2 ms
//	Shards   262.144: 26,0 MB,  Range  5,13 ms,  Bau  30 ms
//
// Range waechst also mit Faktor 16,7 bei 16-facher Shard-Zahl -- linear, wie
// es soll. Der Test prueft genau diesen Faktor und nicht die Wanduhr: eine
// absolute Grenze stand dort zuerst und ist im Race-Detektor sofort gerissen,
// der jeden Sperrvorgang um eine Groessenordnung verlangsamt.
//
// 26 MB gegen die 1,28 GB, die der Knoten unter Last ohnehin haelt. Der eine
// reale Preis war Range: es sperrte JEDEN Shard einzeln, auch die leeren.
// Seit dem 05.10.2026 sperrt es nur belegte Shards (siehe "NUR BELEGTE
// SHARDS" unten; gemessen 14 MB, Range-Faktor 1,5x statt 16x). Auf dem
// Ueberweisungspfad liegt es trotzdem nicht (range_auf_heissem_pfad_test.go).
//
//	AEQUITAS_ACCOUNT_SHARDS   Zahl der Shards (Vorgabe 262144)
//
// Nur beim Start gelesen; ein unbrauchbarer Wert ergibt die Vorgabe. Ein
// Rueckweg auf 16384 ist damit eine Umgebungsvariable und kein Deploy.
const numAccountShardsVorgabe = 262144

const numAccountShardsEnv = "AEQUITAS_ACCOUNT_SHARDS"

var numAccountShards = shardZahlAusUmgebung()

func shardZahlAusUmgebung() int {
	roh := strings.TrimSpace(os.Getenv(numAccountShardsEnv))
	if roh == "" {
		return numAccountShardsVorgabe
	}
	n, err := strconv.Atoi(roh)
	if err != nil || n < 1 {
		fmt.Printf("[SHARDS] %s=%q ist keine positive Zahl -- bleibe bei %d\n",
			numAccountShardsEnv, roh, numAccountShardsVorgabe)
		return numAccountShardsVorgabe
	}
	if n != numAccountShardsVorgabe {
		fmt.Printf("[SHARDS] Kontenpartitionen auf %d gesetzt (Vorgabe %d) via %s\n",
			n, numAccountShardsVorgabe, numAccountShardsEnv)
	}
	return n
}

// accountShard is one partition: its own map, its own mutex. Concurrent
// access to DIFFERENT shards never contends on the same lock -- the
// entire point of sharding. Native Go maps are not safe for ANY
// concurrent access from multiple goroutines, not even to different keys
// (a write can trigger a rehash that touches the whole internal
// structure) -- so this MUST be N separate map instances, never one
// shared map with striped locks around it.
type accountShard struct {
	mu   sync.Mutex
	data map[string]*AccountState
	// partialXOR is this shard's own contribution to the state-root
	// account accumulator (see SCALING_ARCHITECTURE.md Phase 4 / the
	// XorLeaf/CombinedXOR methods below) -- guarded by this shard's own mu,
	// not cs.mu, by design: the whole point is that two goroutines updating
	// accounts in DIFFERENT shards can update their respective partialXOR
	// without contending on any shared lock.
	partialXOR [32]byte
}

// shardedAccounts is a map[string]*AccountState replacement, safe for
// concurrent use on its own (every method takes and releases its own
// shard lock(s) internally -- callers do not need to hold any external
// lock to call these methods safely). Deliberately mirrors Go's native
// map semantics (comma-ok Get, Range instead of `for range`, explicit
// Delete/Len) so migrating existing `cs.accounts[...]` call sites is a
// mechanical, low-risk transformation -- see SCALING_ARCHITECTURE.md's
// Phase 2 for that migration, NOT done by this file.
type shardedAccounts struct {
	shards []*accountShard
	// belegt: ein Bit je Shard, gesetzt, solange der Shard Eintraege hat.
	// Damit sperren Range und Len nur die belegten Shards statt aller
	// numAccountShards (siehe unten, "NUR BELEGTE SHARDS").
	belegt []atomic.Uint64
}

// NUR BELEGTE SHARDS (05.10.2026).
//
// Range und Len sperrten jeden der 262.144 Shards einzeln, auch die leeren
// (oben: "Der eine reale Preis ist Range"). Gemessen am 05.10.2026 in der
// Testsuite des Pakets: 40 % der gesamten CPU in Range, 31 % allein in
// Lock/Unlock leerer Shards, dazu 10 % im Anlegen der Shards. Unter -race,
// das jede Sperre um eine Groessenordnung verteuert, brauchte ein einziger
// Test (TestVerteilteAnnahme_DreiAnnehmende...) 450 s, und die Suite riss
// das 30-Minuten-Limit des CI. Auf dem Blockpfad kostet dasselbe Range je
// Block Millisekunden.
//
// Das Bitfeld wird nur unter der Sperre des jeweiligen Shards geaendert
// (Set/SetLocked setzen, Delete loescht beim letzten Eintrag) -- atomar,
// weil 64 Shards ein Wort teilen. Range liest ein Wort und sperrt nur die
// Shards mit gesetztem Bit, in derselben aufsteigenden Reihenfolge wie
// bisher. Was Range damit sieht, ist dasselbe wie vorher: jeder Eintrag,
// der waehrend des ganzen Durchlaufs besteht, hat sein Bit seit dem Eintragen
// gesetzt (geloescht wird es erst, wenn der Shard leer ist). Ein Eintrag,
// der waehrend des Durchlaufs hinzukommt, kann gesehen werden oder nicht --
// genau wie vorher, wenn Range seinen Shard schon hinter sich hatte. Einen
// Schnappschuss hat Range nie versprochen.
//
// Die Shards liegen ausserdem in EINER Zuweisung, und ihre Map entsteht
// erst beim ersten Eintrag: Lesen, Loeschen und Iterieren einer nil-Map
// sind in Go erlaubt.

func newShardedAccounts() *shardedAccounts {
	sa := &shardedAccounts{
		shards: make([]*accountShard, numAccountShards),
		belegt: make([]atomic.Uint64, (numAccountShards+63)/64),
	}
	alle := make([]accountShard, numAccountShards)
	for i := range sa.shards {
		sa.shards[i] = &alle[i]
	}
	return sa
}

// eintragenLocked: m[addr] = acc in Shard idx, dessen Sperre gehalten wird.
func (sa *shardedAccounts) eintragenLocked(idx int, addr string, acc *AccountState) {
	s := sa.shards[idx]
	if s.data == nil {
		s.data = make(map[string]*AccountState)
	}
	s.data[addr] = acc
	if len(s.data) == 1 {
		sa.belegt[idx>>6].Or(uint64(1) << (idx & 63))
	}
}

// belegteShards ruft fn fuer jeden Shard mit gesetztem Bit auf, aufsteigend.
// fn sperrt selbst.
func (sa *shardedAccounts) belegteShards(fn func(s *accountShard) bool) {
	for w := range sa.belegt {
		b := sa.belegt[w].Load()
		for b != 0 {
			i := bits.TrailingZeros64(b)
			b &= b - 1
			if !fn(sa.shards[w<<6+i]) {
				return
			}
		}
	}
}

// shardIndexFor is the routing function every operation on a given
// address must agree on -- used both to pick a single shard (Get/Set/
// Delete) and, in a later phase, to decide cross-shard lock ordering for
// operations touching two addresses. FNV-1a: fast, well-distributed for
// short string keys, no cryptographic properties needed here (this is
// purely a load-balancing hash, not security-relevant, and this sharding
// is a pure in-memory runtime detail never persisted or compared across
// processes -- nothing depends on matching hash/fnv's own numeric output,
// only on being deterministic and well-distributed within one process).
//
// FIX (2026-07-23, 50k-TPS-goal TPS-benchmark investigation): this used to
// call hash/fnv's New32a() + Write([]byte(addr)) + Sum32() -- on every
// single call, since this runs on EVERY account touch (Get/Set/Delete/
// LockAddrs/TryLockAddrs/GetLocked, i.e. the single hottest function in
// this whole package), that's a hash.Hash32 interface allocation plus a
// string-to-[]byte conversion (a real copy -- Go strings are immutable,
// []byte is not) on every call. Inlined here as a manual FNV-1a loop
// directly over the string's bytes (Go allows indexing a string by byte
// without converting it), producing the identical algorithm with zero
// allocations per call.
func shardIndexFor(addr string) int {
	const offset32 = 2166136261
	const prime32 = 16777619
	h := uint32(offset32)
	for i := 0; i < len(addr); i++ {
		h ^= uint32(addr[i])
		h *= prime32
	}
	return int(h % uint32(numAccountShards))
}

func (sa *shardedAccounts) shardFor(addr string) *accountShard {
	return sa.shards[shardIndexFor(addr)]
}

// Get mirrors `acc, ok := m[addr]`, including on a nil receiver: reading a
// nil native map never panics (returns the zero value, ok=false), and
// several existing tests construct a bare &ChainState{} whose accounts
// field is left as a nil *shardedAccounts -- those tests only ever read
// (e.g. via snapshotForRollbackLocked), so this nil check preserves their
// pre-migration behavior exactly instead of turning a safe no-op into a
// nil-pointer panic.
func (sa *shardedAccounts) Get(addr string) (*AccountState, bool) {
	if sa == nil {
		return nil, false
	}
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	acc, ok := s.data[addr]
	return acc, ok
}

// Set mirrors `m[addr] = acc` -- including panicking on a nil receiver, the
// same as assigning into a nil native map. Every real construction path
// (NewChainState, newTestState, etc.) always calls newShardedAccounts(), so
// this can only fire for a test that both zero-value-constructs ChainState
// AND then tries to mutate cs.accounts, which would have been an equally
// invalid nil-map write before this migration.
func (sa *shardedAccounts) Set(addr string, acc *AccountState) {
	idx := shardIndexFor(addr)
	s := sa.shards[idx]
	s.mu.Lock()
	defer s.mu.Unlock()
	sa.eintragenLocked(idx, addr, acc)
}

// Delete mirrors `delete(m, addr)`, including on a nil receiver: deleting
// from a nil native map is always a safe no-op in Go, never a panic -- see
// Get's comment for why that nil-safety matters here too.
func (sa *shardedAccounts) Delete(addr string) {
	if sa == nil {
		return
	}
	idx := shardIndexFor(addr)
	s := sa.shards[idx]
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[addr]; !ok {
		return
	}
	delete(s.data, addr)
	if len(s.data) == 0 {
		sa.belegt[idx>>6].And(^(uint64(1) << (idx & 63)))
	}
}

// XorLeaf folds addr's leaf change into its shard's own partialXOR,
// swapping oldLeaf out and newLeaf in -- the same self-inverse XOR-out/
// XOR-in technique ChainState.updateAccountLeafLocked uses for the single
// global cs.accountSetXOR (state.go), just scoped to one shard's own lock
// instead of cs.mu.
//
// SCALING_ARCHITECTURE.md Phase 4: this and CombinedXOR below are a
// complete, independently-tested primitive for a per-shard state-root
// accumulator, built the same way shardedAccounts itself was in Phase 1 --
// NOT YET wired into cs.accountSetXOR/StateRoot(), which remain the single
// global accumulator described in state.go and stay the actual source of
// truth for now. The real benefit of routing leaf updates through here
// instead of cs.accountSetXOR only materializes once a LATER phase (5)
// moves account mutation off cs.mu onto these same per-shard locks --
// before that, cs.mu already serializes every leaf update anyway, so
// switching StateRoot's source of truth over would add a second lock
// acquisition for zero present benefit. Kept here, tested, and ready for
// that wiring rather than built later under time pressure.
//
// Nil-safe no-op, consistent with every other method here.
func (sa *shardedAccounts) XorLeaf(addr string, oldLeaf, newLeaf [32]byte) {
	if sa == nil {
		return
	}
	s := sa.shardFor(addr)
	s.mu.Lock()
	defer s.mu.Unlock()
	xorInto(&s.partialXOR, oldLeaf)
	xorInto(&s.partialXOR, newLeaf)
}

// CombinedXOR returns the XOR of every shard's partialXOR -- equal to the
// XOR of every stored account's current leaf, by the same associative/
// commutative XOR-accumulator reasoning cs.accountSetXOR already relies on
// (see state.go), just computed as a combination of N independent partial
// sums instead of one running total. O(numAccountShards): acceptable for
// the same reason Len() is -- this is a read-time combination step, not a
// per-mutation cost.
func (sa *shardedAccounts) CombinedXOR() [32]byte {
	if sa == nil {
		return [32]byte{}
	}
	var total [32]byte
	for _, s := range sa.shards {
		s.mu.Lock()
		xorInto(&total, s.partialXOR)
		s.mu.Unlock()
	}
	return total
}

// Len mirrors `len(m)`, including on a nil receiver (len(nilMap) == 0 in
// Go, never a panic -- see Get's comment). Sperrt nur belegte Shards (siehe
// "NUR BELEGTE SHARDS"); liest dazu numAccountShards/64 Woerter.
func (sa *shardedAccounts) Len() int {
	if sa == nil {
		return 0
	}
	total := 0
	sa.belegteShards(func(s *accountShard) bool {
		s.mu.Lock()
		total += len(s.data)
		s.mu.Unlock()
		return true
	})
	return total
}

// Range calls fn for every account across all shards, stopping early if
// fn returns false. Mirrors `for addr, acc := range m`. Iteration order
// is not guaranteed (varies by shard and by Go's own native map
// iteration order within a shard) and must not be relied on for
// determinism -- exactly the same non-guarantee Go's native map range
// already has, so no existing caller can have relied on order.
//
// fn must NOT call back into sa for the same shard (Get/Set/Delete/Range)
// -- that would deadlock on the shard mutex Range is already holding.
// Existing cs.accounts range-loop bodies in this codebase call other
// ChainState methods, not cs.accounts itself, from inside the loop, so
// this restriction matches how the map is actually used today.
//
// Nil-safe (zero iterations), mirroring `range` over a nil native map --
// see Get's comment.
func (sa *shardedAccounts) Range(fn func(addr string, acc *AccountState) bool) {
	if sa == nil {
		return
	}
	sa.belegteShards(func(s *accountShard) bool {
		s.mu.Lock()
		for addr, acc := range s.data {
			if !fn(addr, acc) {
				s.mu.Unlock()
				return false
			}
		}
		s.mu.Unlock()
		return true
	})
}

// Clone returns a new, independent shardedAccounts holding a deep copy of
// every account currently stored (each *AccountState copied by value, not
// shared by pointer -- mutating the original after Clone() never affects
// the clone, mirroring the `accCopy := *acc` pattern every existing
// backup-before-mutating call site already used against the plain map).
// Used by callers that need "snapshot now, restore verbatim later if
// something fails" (see ResyncFromSnapshotURL) -- restoring is then just
// reassigning cs.accounts to the clone, since cs.accounts is a pointer
// field.
//
// Does NOT carry over the source's per-shard partialXOR state (see
// XorLeaf/CombinedXOR): Clone only ever calls Set, and Set/XorLeaf are
// deliberately independent operations here, the same way mutating an
// AccountState's fields and calling updateAccountLeafLocked are two
// separate steps in state.go today -- nothing silently keeps them in sync.
// A caller that needs the clone's own CombinedXOR() to be meaningful must
// rebuild it explicitly (Range + XorLeaf per account) after cloning.
func (sa *shardedAccounts) Clone() *shardedAccounts {
	clone := newShardedAccounts()
	sa.Range(func(addr string, acc *AccountState) bool {
		cp := *acc
		clone.Set(addr, &cp)
		return true
	})
	return clone
}

// LockAddrs acquires every distinct shard touched by addrs, in a fixed
// deterministic order (ascending shard index), and returns an unlock
// function that releases them all -- callers MUST call it exactly once
// (typically via defer) when done. SCALING_ARCHITECTURE.md Phase 5: this
// is the primitive that lets an operation touching a KNOWN, SMALL set of
// addresses (e.g. a transfer's sender+recipient) hold just those shards'
// locks for a whole multi-step read-modify-write, instead of relying on
// cs.mu's full exclusivity -- concurrent callers whose address sets don't
// overlap never contend with each other at all.
//
// Deterministic ordering is what makes this deadlock-free: two concurrent
// callers whose touched addresses land in shards {3, 7} and {7, 3}
// respectively both lock in the order [3, 7] (never [7, 3]), so neither
// can ever hold shard 7 while waiting for shard 3 that the other already
// holds. This is the standard "lock in a global total order" deadlock
// prevention technique, applied to shard indices specifically.
//
// While held, GetLocked/SetLocked (not Get/Set) must be used for any
// address whose shard is among those locked here -- Get/Set acquire the
// shard's lock internally, and Go's sync.Mutex is not reentrant, so
// calling them for an already-locked shard would deadlock the calling
// goroutine against itself.
func (sa *shardedAccounts) LockAddrs(addrs ...string) (unlock func()) {
	seen := make(map[int]bool, len(addrs))
	indices := make([]int, 0, len(addrs))
	for _, a := range addrs {
		idx := shardIndexFor(a)
		if !seen[idx] {
			seen[idx] = true
			indices = append(indices, idx)
		}
	}
	sort.Ints(indices)
	for _, idx := range indices {
		sa.shards[idx].mu.Lock()
	}
	return func() {
		for i := len(indices) - 1; i >= 0; i-- {
			sa.shards[indices[i]].mu.Unlock()
		}
	}
}

// TryLockAddrs is LockAddrs's non-blocking counterpart: it attempts to
// acquire every distinct shard touched by addrs, in the same deterministic
// ascending order, but NEVER WAITS for a contended shard -- if any shard
// is already locked by another goroutine, it immediately releases whatever
// it already acquired (in reverse order, same as unlock() would) and
// returns ok=false with a nil unlock func. On success (ok=true), behaves
// exactly like LockAddrs -- same deadlock-freedom argument applies since
// the acquire order is identical, just each individual acquisition uses
// sync.Mutex.TryLock instead of Lock.
//
// This exists for exactly one caller (transferConcurrent): a fast path
// that holds these locks across a real DB round trip (Begin/save/Commit)
// must not turn into serializing every contending transfer behind that
// round trip on a hot shard -- measured directly, doing so made a
// concentrated-recipient workload (many senders paying one address, e.g. a
// pool/exchange/merchant) roughly 2x SLOWER than the plain batched path,
// because every contender queued behind one shard's lock for a whole solo
// commit instead of getting folded into the batcher's shared commit. Bailing
// instantly instead lets a hot shard's traffic fall straight through to the
// batcher (which amortizes commits across many transfers) rather than
// queuing on a lock that only benefits genuinely disjoint traffic.
func (sa *shardedAccounts) TryLockAddrs(addrs ...string) (unlock func(), ok bool) {
	seen := make(map[int]bool, len(addrs))
	indices := make([]int, 0, len(addrs))
	for _, a := range addrs {
		idx := shardIndexFor(a)
		if !seen[idx] {
			seen[idx] = true
			indices = append(indices, idx)
		}
	}
	sort.Ints(indices)
	acquired := 0
	for _, idx := range indices {
		if !sa.shards[idx].mu.TryLock() {
			for i := acquired - 1; i >= 0; i-- {
				sa.shards[indices[i]].mu.Unlock()
			}
			return nil, false
		}
		acquired++
	}
	return func() {
		for i := len(indices) - 1; i >= 0; i-- {
			sa.shards[indices[i]].mu.Unlock()
		}
	}, true
}

// GetLocked is Get's counterpart for a caller already holding addr's
// shard lock via LockAddrs -- see LockAddrs's own comment for why Get
// itself must not be used instead (double-lock deadlock).
func (sa *shardedAccounts) GetLocked(addr string) (*AccountState, bool) {
	s := sa.shardFor(addr)
	acc, ok := s.data[addr]
	return acc, ok
}

// SetLocked is Set's counterpart for a caller already holding addr's
// shard lock via LockAddrs -- see LockAddrs's own comment.
func (sa *shardedAccounts) SetLocked(addr string, acc *AccountState) {
	sa.eintragenLocked(shardIndexFor(addr), addr, acc)
}

// MarshalJSON lets `json.Marshal(sa)` behave like `json.Marshal(m)` did
// for the plain map -- needed for cs.save()'s no-DB fallback path, the
// one production call site that serializes the whole account map
// directly rather than through individual Get/Set calls.
func (sa *shardedAccounts) MarshalJSON() ([]byte, error) {
	combined := make(map[string]*AccountState, sa.Len())
	sa.Range(func(addr string, acc *AccountState) bool {
		combined[addr] = acc
		return true
	})
	return json.Marshal(combined)
}
