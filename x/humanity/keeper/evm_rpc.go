package keeper

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// evmRPCVerboseLog gates the per-call [RPC] diagnostic lines below (eth_call
// dispatch, balanceOf intercept result) that used to fire unconditionally
// on EVERY call including read-only polls — with rpcRateLimit's own comment
// already noting a legitimate dashboard/wallet fires several RPC calls per
// page load, that drowned out genuinely important warnings (e.g. a slot
// mismatch) in routine traffic (Performance audit 2026-07-06). Off by
// default; set EVM_RPC_VERBOSE_LOG=true to restore the old per-call trace
// for local debugging.
func evmRPCVerboseLog() bool {
	return os.Getenv("EVM_RPC_VERBOSE_LOG") == "true"
}

// rpcRateLimit bounds /rpc requests per IP. FIX (P1, beta-launch audit
// 2026-07-05): unlike every other mutating/expensive endpoint in this
// codebase, /rpc had no rate limiting at all — only a per-batch size cap
// (maxBatchSize) and a 1 MB body limit. Every single request, including a
// read-only eth_call, triggers EVMEngine.newStateDB() to reload account
// balances (bounded to the handful a call actually touches since the G5 fix
// — see that function's own comment) and every contract's entire storage
// from Postgres — with no rate limit, an unauthenticated caller could fire
// requests as fast as the network allows, each one forcing that reload. A
// sliding-window counter (not the single-
// cooldown-timestamp pattern used elsewhere in this codebase) since a
// legitimate wallet/dashboard needs to make several RPC calls per page
// load, not just one every few seconds.
var rpcRateLimit sync.Map // ip -> *rpcRateLimitEntry

type rpcRateLimitEntry struct {
	mu          sync.Mutex
	windowStart time.Time
	count       int
}

const rpcRateLimitWindow = 10 * time.Second

// rpcRateLimitMax is the per-IP request ceiling within rpcRateLimitWindow.
// 200 (i.e. 20 requests/s) is generous headroom for a busy dashboard polling
// several endpoints, while still bounding worst-case newStateDB() reload spam
// per IP. That default is UNCHANGED and applies to every node that does not
// explicitly opt out.
//
// Made overridable via AEQUITAS_RPC_RATE_LIMIT_MAX (2026-07-24) because it —
// not the chain — became the binding constraint on throughput measurement.
// The limiter is checked once per HTTP request, before the body is parsed, so
// a JSON-RPC batch of maxBatchSize=100 transfers costs exactly one tick. That
// puts a single source at 20 × 100 = 2,000 transfers/s, which is 25× short of
// the 50,000 the chain is being tuned for (maxTxsPerBlock=50000 at
// BLOCK_TIME=1s). No amount of load-generator work can get past it, since the
// rejection happens before the request is even read.
//
// This is deliberately an ENV OVERRIDE rather than a raised default: the
// limiter is real protection for a publicly-reachable /rpc endpoint, and
// every operator who has not set the variable keeps exactly the behaviour
// they have today. Raising it is the same class of explicit, per-box operator
// decision as BLOCK_TIME or AEQUITAS_WAL_ENABLED — appropriate on a
// load-test box, not something to inherit silently.
//
// Invalid or non-positive values fall back to the default rather than
// disabling the limiter: a typo in an env var must never turn protection off.
var rpcRateLimitMax = rpcRateLimitMaxFromEnv()

const rpcRateLimitMaxDefault = 200

func rpcRateLimitMaxFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("AEQUITAS_RPC_RATE_LIMIT_MAX"))
	if raw == "" {
		return rpcRateLimitMaxDefault
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		fmt.Printf("[RPC] ⚠ AEQUITAS_RPC_RATE_LIMIT_MAX=%q is not a positive integer — keeping the default of %d requests per %s per IP\n",
			raw, rpcRateLimitMaxDefault, rpcRateLimitWindow)
		return rpcRateLimitMaxDefault
	}
	if n != rpcRateLimitMaxDefault {
		fmt.Printf("[RPC] ⚠ Per-IP RPC rate limit raised to %d requests per %s (default %d) via AEQUITAS_RPC_RATE_LIMIT_MAX — this weakens a protection on a publicly reachable endpoint and is intended for a load-test box only\n",
			n, rpcRateLimitWindow, rpcRateLimitMaxDefault)
	}
	return n
}

// rpcRateLimited reports whether ip has exceeded rpcRateLimitMax requests
// within the current rpcRateLimitWindow, incrementing its counter either way.
func rpcRateLimited(ip string) bool {
	v, _ := rpcRateLimit.LoadOrStore(ip, &rpcRateLimitEntry{windowStart: time.Now()})
	entry := v.(*rpcRateLimitEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if time.Since(entry.windowStart) > rpcRateLimitWindow {
		entry.windowStart = time.Now()
		entry.count = 0
	}
	entry.count++
	return entry.count > rpcRateLimitMax
}

func init() {
	// Periodically clean up expired rate-limit entries to prevent unbounded
	// growth — mirrors registerRateLimit's own cleanup goroutine
	// (register.go). Safe to delete an entry a concurrent request is mid-way
	// through reading: the counter's only job is "count within window", so
	// an orphaned entry simply gets garbage collected once released; unlike
	// registerWalletLocks' mutex (see lockWallet's comment), there's no
	// exclusion property that a stale reference could silently break.
	SafeGoroutine("rpcRateLimit-cleanup", func() {
		for {
			time.Sleep(60 * time.Second)
			// FIX (P0-3, beta-launch audit 2026-07-05): recover per-iteration —
			// see safeCall's comment. A panicked cleanup pass must not
			// permanently end this loop; the map would just grow unbounded from
			// that point on with nothing to notice.
			SafeCall("rpcRateLimit-cleanup-tick", func() {
				now := time.Now()
				rpcRateLimit.Range(func(k, v interface{}) bool {
					entry := v.(*rpcRateLimitEntry)
					entry.mu.Lock()
					stale := now.Sub(entry.windowStart) > 2*rpcRateLimitWindow
					entry.mu.Unlock()
					if stale {
						rpcRateLimit.Delete(k)
					}
					return true
				})
			})
		}
	})
}

// EVMRPCServer handles Ethereum JSON-RPC requests
type EVMRPCServer struct {
	dag               *BlockDAG
	state             *ChainState
	evm               *EVMEngine
	mu                sync.Mutex // guards all map fields below against concurrent writes
	deployedContracts map[string]string // txHash -> contractAddress (lowercase)
	txStatus          map[string]bool   // txHash -> true if execution succeeded
	txError           map[string]string // txHash -> error message if failed
	txSenders         map[string]string // txHash -> sender address (lowercase)
	txTos             map[string]string // txHash -> to address (lowercase, "" for contract creation)
	// txOrder is the insertion order of the four txHash-keyed maps above, so
	// the oldest entries can be dropped once they exceed txMetaMax.
	//
	// FIX (2026-07-26, measured): those four maps had no eviction at all. Each
	// transaction added roughly four permanent entries, so they grew for the
	// whole process lifetime. That is a memory leak, but the throughput cost
	// came first: Go grows and rehashes a map while the caller holds the lock,
	// and all of these live under the single mu above, taken eleven times
	// across this file. Goroutine dumps taken under load on Contabo2 showed all
	// 72 in-flight requests parked in runtime_SemacquireMutex at
	// sendRawTransaction, with none in the WAL - the hold time grew with map
	// size, so throughput decayed with uptime and a restart "fixed" it. That
	// matches both incidents seen that day, on nodes with long uptimes.
	//
	// Bounding them is safe because they are short-lived caches, not the
	// record of truth: SaveTxReceipt persists the same status durably, and
	// chain_tx_block_index records which block a transaction landed in. They
	// only need to cover the window where a wallet polls straight after
	// getting its hash back.
	txOrder []string
	// nonceShards shards the nonce critical section by sender address.
	//
	// FIX (2026-07-26, measured): sendRawTransaction used to hold the single mu
	// across the whole load-check-reserve sequence -- INCLUDING two synchronous
	// Postgres calls, LoadNonce and ReserveNonce. Every transaction on the node
	// therefore waited for the previous one's database round trip, which caps
	// throughput at roughly one DB write at a time. Goroutine dumps under load
	// on Contabo2 showed 142 of ~147 in-flight requests blocked on that mutex,
	// and measured throughput sat at 428-466 TPS across every configuration
	// tried -- WAL, disk, GC, signature verification and the rate limiter were
	// all ruled out as the cause before this was found.
	//
	// The race the original comment guards against is between two requests from
	// the SAME sender both reserving the same nonce. Sharding by sender keeps
	// that mutual exclusion exactly as strict as before -- same address, same
	// lock, same ordering -- while letting different accounts proceed in
	// parallel, which they can always do safely because a nonce is per-account
	// by definition.
	//
	// Fixed-size array rather than a map: no allocation, no eviction, and no
	// lock needed to find a lock.
	nonceShards [nonceShardCount]nonceShard
}

// nonceShard is one slice of the nonce cache: a mutex and the map it alone
// guards.
//
// FIX (2026-07-26): the map MUST be sharded together with the mutex. The first
// version of this sharding kept a single `nonces map[string]uint64` on the
// server and only split the lock, so two senders landing in different shards
// held different mutexes while writing the same Go map. A Go map is unsafe for
// concurrent writes regardless of whether the keys differ, and the runtime
// aborts the whole process with "fatal error: concurrent map writes" when it
// notices -- an unrecoverable crash, not a recoverable panic, which is exactly
// the failure mode to expect from a node under multi-sender load. Giving each
// shard its own map restores the invariant the sharding was supposed to
// preserve: every access to a map happens under the one mutex beside it.
type nonceShard struct {
	mu     sync.Mutex
	nonces map[string]uint64
}

// nonceShardCount is a power of two so the shard index is a mask rather than a
// division. 256 is far above the core count of any node this runs on, so two
// senders colliding on a shard is rare and costs only what the old global lock
// cost for every pair.
const nonceShardCount = 256

// nonceShardFor returns the shard owning addr's nonce -- both the lock and the
// map that lock guards. Callers must use this for EVERY nonce access; reaching
// a nonce map through any other shard would leave it guarded by two different
// mutexes, which is no guard at all.
func (s *EVMRPCServer) nonceShardFor(addr string) *nonceShard {
	h := uint32(2166136261)
	for i := 0; i < len(addr); i++ {
		h = (h ^ uint32(addr[i])) * 16777619
	}
	return &s.nonceShards[h&(nonceShardCount-1)]
}

// txMetaMax bounds the four txHash-keyed caches. Generous enough to cover any
// realistic poll window - at the 50k TPS the chain is tuned for this is still
// several seconds of history - while keeping the maps small enough that a
// rehash under the lock stays cheap.
const txMetaMax = 100000

// noteTxLocked records that txHash was just written into the tx metadata maps
// and drops the oldest entries once the cache is over its bound. Callers must
// already hold s.mu, exactly as they do for the map writes themselves.
func (s *EVMRPCServer) noteTxLocked(txHash string) {
	s.txOrder = append(s.txOrder, txHash)
	if len(s.txOrder) <= txMetaMax {
		return
	}
	drop := len(s.txOrder) - txMetaMax
	for _, old := range s.txOrder[:drop] {
		delete(s.txStatus, old)
		delete(s.txError, old)
		delete(s.txSenders, old)
		delete(s.txTos, old)
	}
	// Re-slice into a fresh backing array: keeping the old one would retain
	// every evicted string and defeat the point of evicting them.
	kept := make([]string, len(s.txOrder)-drop)
	copy(kept, s.txOrder[drop:])
	s.txOrder = kept
}

func NewEVMRPCServer(dag *BlockDAG, state *ChainState) *EVMRPCServer {
	engine, err := NewEVMEngine(state)
	if err != nil {
		fmt.Printf("[EVM] Warning: could not init EVM engine: %v\n", err)
	}
	// Share the EVMEngine with the DAG so replayTransactions can call
	// BioVerifier directly when verifying ZK proofs in register_human TXs.
	if engine != nil {
		dag.evm = engine
		// Must run AFTER dag.evm is set (verifyZKProof needs it) and can only
		// start here — dag.evm is nil for the whole of NewBlockchain. See
		// repairUnreplayedBlocks' own comment.
		//
		// FIX (P0 availability, 2026-07-25 night — Primary served 502 for ~35
		// minutes): this used to run SYNCHRONOUSLY, blocking the rest of boot
		// (including the HTTP listener) until every unreplayed block was
		// repaired. The unbounded finder (70e5b27, same day) surfaced a
		// multi-thousand-block backlog including 50k-transfer loadtest blocks
		// — each taking minutes — so the whole node was unreachable for the
		// duration while the rest of the network kept running. The repair is
		// pure background catch-up of HISTORICAL effects (live replay
		// serializes against it per-block via replayMu, and production is
		// separately gated by hasCaughtUpWithAllPeers) — nothing in serving
		// HTTP requires it to have finished. Run it in the background so the
		// node is reachable immediately.
		SafeGoroutine("repairUnreplayedBlocks", dag.repairUnreplayedBlocks)
	}
	s := &EVMRPCServer{
		dag:               dag,
		state:             state,
		evm:               engine,
		deployedContracts: make(map[string]string),
		txStatus:          make(map[string]bool),
		txError:           make(map[string]string),
		txSenders:         make(map[string]string),
		txTos:             make(map[string]string),
	}
	s.initNonceShards()
	return s
}

// initNonceShards gives every shard its own map up front, so no code path has
// to lazily initialise one and no nil map write can reach a live node. Split
// out so the tests build the shards exactly the way production does.
func (s *EVMRPCServer) initNonceShards() {
	for i := range s.nonceShards {
		s.nonceShards[i].nonces = make(map[string]uint64)
	}
}

// ─── HTTP HANDLER ─────────────────────────────────────────────────────────────

func (s *EVMRPCServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}

	if rpcRateLimited(clientIP(r)) {
		writeError(w, -32005, "rate limited: too many requests, try again shortly", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit — prevents memory exhaustion via /rpc
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, -32700, "Parse error", nil)
		return
	}

	// Handle batch requests
	if len(body) > 0 && body[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil {
			writeError(w, -32700, "Parse error", nil)
			return
		}
		// P2-AUDIT: Limit batch size to prevent DoS via 1 MB batch of expensive calls.
		// 100 requests per batch is generous for any legitimate client use case.
		const maxBatchSize = 100
		if len(batch) > maxBatchSize {
			writeError(w, -32600, fmt.Sprintf("batch too large: max %d requests, got %d", maxBatchSize, len(batch)), nil)
			return
		}
		// FIX (2026-07-25, 50k-TPS deep-dive): decode + ecrecover every
		// eth_sendRawTransaction item in the batch up front, in parallel,
		// before the serial dispatch loop below. types.Sender (secp256k1
		// recovery) is the single most CPU-expensive step per tx (profiled at
		// evm_storage.go:2230 — 17.54% of a benchmark run) and is a pure,
		// side-effect-free function: decoding one tx's signature can never
		// depend on or interfere with another's, so this is a safe, purely
		// mechanical parallelization, unlike batching the signatures
		// algebraically (not viable for standard ECDSA — see
		// SCALING_ARCHITECTURE.md's 2026-07-25 deep-dive, finding D) or
		// parallelizing the actual state mutation below (which stays fully
		// serial — nonce reservation and dispatch order must not change).
		precomputed := make([]*precomputedSendTx, len(batch))
		var pending []int
		for i, raw := range batch {
			var env struct {
				Method string            `json:"method"`
				Params []json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(raw, &env); err != nil || env.Method != "eth_sendRawTransaction" || len(env.Params) == 0 {
				continue
			}
			var rawHex string
			if err := json.Unmarshal(env.Params[0], &rawHex); err != nil {
				continue
			}
			pending = append(pending, i)
			precomputed[i] = &precomputedSendTx{rawHex: rawHex}
		}
		if len(pending) > 0 {
			workers := runtime.NumCPU()
			if workers > len(pending) {
				workers = len(pending)
			}
			var wg sync.WaitGroup
			jobs := make(chan int)
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := range jobs {
						p := precomputed[i]
						p.tx, p.sender, p.senderErr, p.err = decodeAndRecoverSender(p.rawHex)
					}
				}()
			}
			for _, i := range pending {
				jobs <- i
			}
			close(jobs)
			wg.Wait()
		}
		var results []interface{}
		for i, raw := range batch {
			result := s.handleSingle(raw, precomputed[i])
			results = append(results, result)
		}
		json.NewEncoder(w).Encode(results)
		return
	}

	result := s.handleSingle(body, nil)
	json.NewEncoder(w).Encode(result)
}

func (s *EVMRPCServer) handleSingle(body []byte, pre *precomputedSendTx) map[string]interface{} {
	var req struct {
		JSONRPC string            `json:"jsonrpc"`
		ID      interface{}       `json:"id"`
		Method  string            `json:"method"`
		Params  []json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return errorResponse(nil, -32700, "Parse error")
	}

	result, rpcErr := s.dispatch(req.Method, req.Params, pre)
	if rpcErr != nil {
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]interface{}{
				"code":    rpcErr.Code,
				"message": rpcErr.Message,
			},
		}
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  result,
	}
}

// ─── DISPATCH ─────────────────────────────────────────────────────────────────

func (s *EVMRPCServer) dispatch(method string, params []json.RawMessage, pre *precomputedSendTx) (interface{}, *RPCError) {
	switch method {

	case "eth_chainId":
		return "0x786", nil // 1926

	case "net_version":
		return "1926", nil

	case "eth_blockNumber":
		block := s.dag.LatestBlock()
		if block == nil {
			return "0x0", nil
		}
		return fmt.Sprintf("0x%x", block.Height), nil

	case "eth_gasPrice":
		return "0x0", nil

	case "eth_maxPriorityFeePerGas":
		return "0x0", nil

	case "eth_feeHistory":
		return map[string]interface{}{
			"oldestBlock":   "0x0",
			"baseFeePerGas": []string{"0x0"},
			"gasUsedRatio":  []float64{0},
			"reward":        [][]string{{"0x0"}},
		}, nil

	case "eth_estimateGas":
		return "0x5B8D80", nil // 6M gas

	case "eth_getTransactionCount":
		return s.getTransactionCount(params)

	case "eth_getBalance":
		return s.getBalance(params)

	case "eth_getCode":
		return s.getCode(params)

	case "eth_call":
		return s.ethCall(params)

	case "eth_sendRawTransaction":
		return s.sendRawTransaction(params, pre)

	case "eth_getTransactionReceipt":
		return s.getTransactionReceipt(params)

	case "eth_getTransactionByHash":
		return s.getTransactionByHash(params)

	case "eth_getBlockByNumber":
		return s.getBlockByNumber(params)

	case "eth_getBlockByHash":
		return s.getBlockByHash(params)

	case "eth_getLogs":
		return []interface{}{}, nil

	case "eth_accounts":
		return []string{}, nil

	case "web3_clientVersion":
		return "AequitasChain/v0.3.0/go", nil

	case "eth_syncing":
		return false, nil

	case "eth_mining":
		return false, nil

	case "eth_coinbase":
		return "0x0000000000000000000000000000000000000000", nil

	case "net_listening":
		return true, nil

	case "net_peerCount":
		return "0x1", nil

	default:
		fmt.Printf("[RPC] Unknown method: %s\n", method)
		return nil, &RPCError{Code: -32601, Message: "Method not found"}
	}
}

// ─── HANDLERS ─────────────────────────────────────────────────────────────────

func (s *EVMRPCServer) getTransactionCount(params []json.RawMessage) (interface{}, *RPCError) {
	if len(params) == 0 {
		return "0x0", nil
	}
	var addr string
	if err := json.Unmarshal(params[0], &addr); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}
	addr = strings.ToLower(addr)

	// Read DB outside the lock (avoids blocking other goroutines on a DB call).
	dbNonce := s.state.LoadNonce(addr)
	// Lock only for the map read/write — brief critical section. Must be the
	// SAME shard sendRawTransaction uses for this address, or the nonce would
	// be guarded by two different mutexes.
	shard := s.nonceShardFor(addr)
	shard.mu.Lock()
	if dbNonce > shard.nonces[addr] {
		shard.nonces[addr] = dbNonce
	}
	result := shard.nonces[addr]
	shard.mu.Unlock()
	return fmt.Sprintf("0x%x", result), nil
}

func (s *EVMRPCServer) getBalance(params []json.RawMessage) (interface{}, *RPCError) {
	if len(params) == 0 {
		return "0x0", nil
	}
	var addr string
	if err := json.Unmarshal(params[0], &addr); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}
	addr = strings.ToLower(addr)

	balance := s.state.GetBalance(addr)
	// Convert AEQ float to wei (× 10^18)
	wei := new(big.Float).Mul(
		big.NewFloat(balance),
		new(big.Float).SetInt(weiPerAEQ),
	)
	weiInt, _ := wei.Int(nil)
	fmt.Printf("[RPC] eth_getBalance %s = %.2f\n", addr, balance)
	return "0x" + weiInt.Text(16), nil
}

func (s *EVMRPCServer) getCode(params []json.RawMessage) (interface{}, *RPCError) {
	if len(params) == 0 {
		return "0x", nil
	}
	var addr string
	if err := json.Unmarshal(params[0], &addr); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}
	addrLow := strings.ToLower(addr)

	// Try EVM StateDB first
	if s.evm != nil {
		code := s.evm.GetCode(common.HexToAddress(addr))
		if len(code) > 0 {
			return "0x" + hex.EncodeToString(code), nil
		}
	}

	// Fallback: load from PostgreSQL
	bytecode, err := s.state.LoadContract(addrLow)
	if err == nil && len(bytecode) > 0 {
		return "0x" + hex.EncodeToString(bytecode), nil
	}

	return "0x", nil
}

func (s *EVMRPCServer) ethCall(params []json.RawMessage) (interface{}, *RPCError) {
	if len(params) == 0 || s.evm == nil {
		return "0x", nil
	}

	var callObj map[string]string
	if err := json.Unmarshal(params[0], &callObj); err != nil {
		return "0x", nil
	}

	from := common.HexToAddress(callObj["from"])
	to := common.HexToAddress(callObj["to"])
	toStr := strings.ToLower(to.Hex())
	data, _ := hex.DecodeString(strings.TrimPrefix(callObj["data"], "0x"))

	if evmRPCVerboseLog() {
		fmt.Printf("[RPC] eth_call to=%s data=%x\n", toStr, data[:min4(len(data), 4)])
	}

	// Intercept isHuman(address) calls (selector 0xf72c436f — keccak256(
	// "isHuman(address)")[:4]) to V7. Read from Go state directly instead of
	// going through the EVM, for the same reason as the balanceOf intercept
	// below: it's the authoritative source and avoids a full EVM invocation
	// for a simple lookup.
	//
	// FIX (2026-07-06): this used to check for 0x2f543389, which is not the
	// selector for isHuman(address) at all (verified against keccak256) —
	// that typo meant this intercept never actually matched a real
	// isHuman(address) call, silently falling through to the EVM path below
	// for every one of them. Investigated as part of the G5 lazy-loading
	// verification after a live eth_call for isHuman appeared to revert;
	// the EVM path itself turned out to be correct for well-formed calldata
	// (verified directly and via TestReproMappingReadOnNeverCommittedTrie —
	// reading a never-written mapping key does not revert), so this was
	// dead defensive code, not the cause of that revert (a separate,
	// malformed-calldata request) — but worth fixing since a correct
	// intercept here is faster and one less path through the EVM per call.
	if len(data) >= 4 && hex.EncodeToString(data[:4]) == "f72c436f" &&
		toStr == strings.ToLower(V7_CONTRACT_ADDR) {
		if len(data) >= 36 {
			addrHex := "0x" + hex.EncodeToString(data[16:36])
			isHuman := s.state.IsHuman(addrHex)
			result := make([]byte, 32) // ABI-encode bool: 32 bytes, 0 or 1
			if isHuman {
				result[31] = 1
			}
			return "0x" + hex.EncodeToString(result), nil
		}
	}

	// Intercept balanceOf(address) calls (selector 0x70a08231) to the V7
	// contract — MetaMask Mobile uses this ERC-20 call to display token
	// balances, but AEQ is now a native currency so the contract returns 0.
	// We redirect these to the real native balance so Mobile shows the
	// correct amount, matching what eth_getBalance returns.
	if len(data) >= 4 && hex.EncodeToString(data[:4]) == "70a08231" &&
		toStr == strings.ToLower(V7_CONTRACT_ADDR) {
		// ABI-decode the address argument (bytes 4..36, left-padded to 32 bytes)
		if len(data) >= 36 {
			addrBytes := data[16:36] // last 20 bytes of the 32-byte padded argument
			addrHex := "0x" + hex.EncodeToString(addrBytes)
			balance := s.state.GetBalance(addrHex)
			wei := new(big.Float).Mul(
				big.NewFloat(balance),
				new(big.Float).SetInt(weiPerAEQ),
			)
			weiInt, _ := wei.Int(nil)
			// ABI-encode as uint256 (32 bytes, big-endian)
			result := make([]byte, 32)
			weiBytes := weiInt.Bytes()
			copy(result[32-len(weiBytes):], weiBytes)
			if evmRPCVerboseLog() {
				fmt.Printf("[RPC] balanceOf(%s) → native balance %.4f AEQ\n", addrHex, balance)
			}
			return "0x" + hex.EncodeToString(result), nil
		}
	}

	// Always reload contract from DB before call to ensure fresh state
	bytecode, err := s.state.LoadContract(toStr)
	if err == nil && len(bytecode) > 0 {
		s.evm.SetCode(to, bytecode)
		s.evm.LoadContractStorage(to)
	}

	result, callErr := s.evm.CallContract(from, to, data, big.NewInt(0), false)
	if callErr != nil {
		fmt.Printf("[RPC] eth_call error: %v\n", callErr)
		return nil, &RPCError{Code: -32603, Message: "execution reverted: " + callErr.Error()}
	}

	return "0x" + hex.EncodeToString(result), nil
}

// precomputedSendTx carries an eth_sendRawTransaction batch item's decode +
// sender-recovery result, computed ahead of time (in parallel, across the
// whole batch — see handleRPC's batch branch) so sendRawTransaction's serial
// dispatch loop doesn't redo the most CPU-expensive step per tx. rawHex is
// set by the pre-pass before the worker pool fills in the rest; a nil
// *precomputedSendTx (the non-batch, single-request path) means "not
// precomputed, decode inline" — sendRawTransaction handles both.
type precomputedSendTx struct {
	rawHex    string
	tx        *types.Transaction
	sender    string
	senderErr bool // true if err came from sender recovery (-32603), not decode (-32602)
	err       error
}

// decodeAndRecoverSender does the pure, side-effect-free half of
// eth_sendRawTransaction: hex-decode, RLP/binary-unmarshal, and ecrecover
// the sender. No shared state, no locks — safe to run concurrently for
// distinct transactions, which is exactly what handleRPC's batch pre-pass
// does. Kept identical in behavior to the inline code this replaced.
func decodeAndRecoverSender(rawHex string) (tx *types.Transaction, senderAddr string, senderErr bool, err error) {
	rawHex = strings.TrimPrefix(rawHex, "0x")

	rawBytes, hexErr := hex.DecodeString(rawHex)
	if hexErr != nil {
		return nil, "", false, fmt.Errorf("Invalid hex")
	}

	t := new(types.Transaction)
	// UnmarshalBinary handles all tx types: legacy (RLP), EIP-2930 (type 1), EIP-1559 (type 2)
	if binErr := t.UnmarshalBinary(rawBytes); binErr != nil {
		// Fallback to RLP for legacy transactions
		if err2 := rlp.DecodeBytes(rawBytes, t); err2 != nil {
			return nil, "", false, fmt.Errorf("Invalid transaction: %v", binErr)
		}
	}

	// Recover sender
	signer := types.LatestSignerForChainID(big.NewInt(1926))
	sender, sErr := types.Sender(signer, t)
	if sErr != nil {
		signer = types.NewEIP155Signer(big.NewInt(1926))
		sender, sErr = types.Sender(signer, t)
		if sErr != nil {
			return nil, "", true, fmt.Errorf("Cannot recover sender: %v", sErr)
		}
	}

	return t, strings.ToLower(sender.Hex()), false, nil
}

func (s *EVMRPCServer) sendRawTransaction(params []json.RawMessage, pre *precomputedSendTx) (interface{}, *RPCError) {
	if len(params) == 0 {
		return nil, &RPCError{Code: -32602, Message: "Missing params"}
	}

	var rawHex string
	if err := json.Unmarshal(params[0], &rawHex); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}

	var tx *types.Transaction
	var senderAddr string
	if pre != nil {
		// Precomputed by handleRPC's batch pre-pass (see precomputedSendTx's
		// own comment) — reuse it instead of decoding+recovering again.
		if pre.err != nil {
			if pre.senderErr {
				return nil, &RPCError{Code: -32603, Message: pre.err.Error()}
			}
			return nil, &RPCError{Code: -32602, Message: pre.err.Error()}
		}
		tx = pre.tx
		senderAddr = pre.sender
	} else {
		t, sender, senderErr, err := decodeAndRecoverSender(rawHex)
		if err != nil {
			if senderErr {
				return nil, &RPCError{Code: -32603, Message: err.Error()}
			}
			return nil, &RPCError{Code: -32602, Message: err.Error()}
		}
		tx = t
		senderAddr = sender
	}
	// common.Address form, needed below for DeployContract/CallContract —
	// round-tripping through the lowercased hex is exact (common.HexToAddress
	// is case-insensitive), same value types.Sender originally returned.
	sender := common.HexToAddress(senderAddr)

	txHash := tx.Hash().Hex() // already has 0x prefix

	// Gated: two unbuffered Printf per transaction is 100,000 log lines/s at
	// the 50,000 TPS target. See rpcQuietTx (evm_rpc_throughput.go) — default
	// is unchanged, and applied throughput is reported once per second either
	// way.
	if !rpcQuietTx {
		fmt.Printf("[RPC] eth_sendRawTransaction hash=%s from=%s to=%v data=%d bytes\n",
			txHash, senderAddr, tx.To(), len(tx.Data()))
	}

	// ── NONCE CHECK + RESERVATION ─────────────────────────────────────────────
	// Check tx.Nonce() against the stored per-account nonce and atomically
	// reserve it before executing. Without this, the same signed transaction
	// can be replayed repeatedly until the account balance is exhausted.
	//
	// P0-AUDIT: The previous two-lock pattern had a TOCTOU race: two goroutines
	// for the same sender could both read nonce=0 from the map (first lock),
	// both load nonce=0 from DB (DB read outside lock), and then both pass the
	// second lock's check — both reserving nonce 0 and executing the same tx.
	// Fix: hold the mutex for the entire DB-load + check + reserve sequence.
	nonceLock := s.nonceShardFor(senderAddr)
	nonceLock.mu.Lock()
	// Populate from DB on first sight to recover correct nonce after restart.
	if nonceLock.nonces[senderAddr] == 0 {
		if dbNonce := s.state.LoadNonce(senderAddr); dbNonce > 0 {
			nonceLock.nonces[senderAddr] = dbNonce
		}
	}
	storedNonce := nonceLock.nonces[senderAddr]
	txNonce := tx.Nonce()
	if txNonce < storedNonce {
		nonceLock.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: fmt.Sprintf("nonce too low: tx=%d expected=%d", txNonce, storedNonce)}
	}
	if txNonce > storedNonce {
		nonceLock.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: fmt.Sprintf("nonce too high: tx=%d expected=%d", txNonce, storedNonce)}
	}
	// Reserve nonce immediately — prevents replay even if two identical
	// requests arrive concurrently.
	nextNonce := storedNonce + 1
	reserved, err := s.state.ReserveNonce(senderAddr, storedNonce, nextNonce)
	if err != nil {
		nonceLock.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: "nonce reservation failed: " + err.Error()}
	}
	if !reserved {
		dbNonce := s.state.LoadNonce(senderAddr)
		nonceLock.nonces[senderAddr] = dbNonce
		nonceLock.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: fmt.Sprintf("nonce already reserved: tx=%d expected=%d", txNonce, dbNonce)}
	}
	nonceLock.nonces[senderAddr] = nextNonce
	// Derived here, but recorded below under mu rather than this lock.
	toAddrForReceipt := ""
	if tx.To() != nil {
		toAddrForReceipt = strings.ToLower(tx.To().Hex())
	}
	nonceLock.mu.Unlock()
	// Receipt metadata is keyed by txHash and shared across senders, so it
	// belongs under mu rather than a per-sender lock. Taken separately and
	// briefly, after the nonce lock is released, so the two never nest.
	s.mu.Lock()
	s.txSenders[txHash] = senderAddr
	s.txTos[txHash] = toAddrForReceipt
	s.mu.Unlock()

	// ── SIMPLE AEQ TRANSFER (native value transfer, no calldata) ─────────────
	if tx.To() != nil && len(tx.Data()) == 0 && tx.Value().Sign() > 0 {
		toAddr := strings.ToLower(tx.To().Hex())
		decimals := new(big.Float).SetInt(weiPerAEQ)
		valueFloat, _ := new(big.Float).Quo(new(big.Float).SetInt(tx.Value()), decimals).Float64()

		// FIX P0-RACE: Set txStatus=true and persist receipt BEFORE calling
		// Transfer(). MetaMask polls getTransactionReceipt immediately after
		// receiving txHash. Without this, the window while Transfer() executes
		// (DB write, ~10-100ms) returned null receipts → MetaMask showed
		// "Senden fehlgeschlagen" even for successful transfers.
		s.mu.Lock()
		s.txStatus[txHash] = true
		s.noteTxLocked(txHash)
		s.mu.Unlock()
		s.state.SaveTxReceipt(txHash, senderAddr, toAddr, "0x1", "")

		// FIX (atomic outbox): TransferAtomic commits the state mutation and
		// the pending_tx outbox insert as a single DB transaction — either
		// both happen or neither does (see runAtomicWithOutbox), instead of
		// the old Transfer()-then-SavePendingTx() sequence where the outbox
		// write could fail independently after the transfer had already
		// committed, permanently hiding it from every other node.
		pendingTxTemplate := Transaction{Type: "transfer", Wallet: senderAddr, To: toAddr, Amount: valueFloat, TxHash: txHash}
		_, _, err := s.state.TransferAtomic(senderAddr, toAddr, valueFloat, pendingTxTemplate)
		if err != nil {
			// Transfer failed — mark receipt as failed so MetaMask shows correct status.
			s.mu.Lock()
			s.txStatus[txHash] = false
			s.noteTxLocked(txHash)
			s.mu.Unlock()
			s.state.SaveTxReceipt(txHash, senderAddr, toAddr, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: "Transfer failed: " + err.Error()}
		}
		s.state.SyncBalancesToEVM(V7_CONTRACT_ADDR, senderAddr, toAddr)
		// Counted before it is (optionally) printed: the count is what the
		// throughput report is built from, and it must stay accurate whether
		// or not the per-transaction line is suppressed.
		recordAppliedTransfer()
		if !rpcQuietTx {
			fmt.Printf("[RPC] ✓ Transfer %.4f AEQ: %s → %s\n", valueFloat, senderAddr, toAddr)
		}
		return txHash, nil
	}

	// ── EVM TOKEN TRANSFER INTERCEPTION (AEQ V7, selector a9059cbb) ──────────
	// Route transfer(address,uint256) calls to the V7 contract through Go state
	// so both ledgers stay in sync (Go state is authoritative for balances).
	if tx.To() != nil && len(tx.Data()) >= 68 &&
		strings.ToLower(tx.To().Hex()) == strings.ToLower(V7_CONTRACT_ADDR) &&
		hex.EncodeToString(tx.Data()[:4]) == "a9059cbb" {
		toBytes := tx.Data()[16:36]
		toAddr := strings.ToLower(common.BytesToAddress(toBytes).Hex())
		amountBig := new(big.Int).SetBytes(tx.Data()[36:68])
		decimals := new(big.Float).SetInt(weiPerAEQ)
		amountFloat, _ := new(big.Float).Quo(new(big.Float).SetInt(amountBig), decimals).Float64()

		// FIX P0-RACE: Set txStatus=true and persist receipt BEFORE calling
		// TransferWithV7Fee. Same race window as the native transfer path — MetaMask
		// polls getTransactionReceipt immediately after receiving txHash; without this
		// the window while TransferWithV7Fee executes returned null receipts.
		s.mu.Lock()
		s.txStatus[txHash] = true
		s.noteTxLocked(txHash)
		s.mu.Unlock()
		s.state.SaveTxReceipt(txHash, senderAddr, toAddr, "0x1", "")

		// E2-FIX: TransferWithV7Fee returns the exact net amount credited to the
		// recipient (computed inside the lock), eliminating the TOCTOU race where
		// preRecipientBalance/postRecipientBalance were read outside the lock and
		// concurrent transfers to the same recipient could produce wrong netAmt.
		// FIX (atomic outbox): TransferWithV7FeeAtomic commits the state
		// mutation and the pending_tx outbox insert as a single DB
		// transaction — see TransferAtomic's comment.
		pendingTxV7Template := Transaction{Type: "transfer", Wallet: senderAddr, To: toAddr, TxHash: txHash}
		_, _, _, err := s.state.TransferWithV7FeeAtomic(senderAddr, toAddr, amountFloat, pendingTxV7Template)
		if err != nil {
			// Mark as failed
			s.mu.Lock()
			s.txStatus[txHash] = false
			s.noteTxLocked(txHash)
			s.mu.Unlock()
			s.state.SaveTxReceipt(txHash, senderAddr, toAddr, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: "Transfer failed: " + err.Error()}
		}
		s.state.SyncBalancesToEVM(V7_CONTRACT_ADDR, senderAddr, toAddr)
		fmt.Printf("[RPC] ✓ Token transfer %.4f AEQ (with V7 fee): %s → %s\n", amountFloat, senderAddr, toAddr)
		return txHash, nil
	}

	// ── CONTRACT DEPLOYMENT ──────────────────────────────────────────────────
	// Restricted to RELAYER_ADDRESS or the node's own signing key address.
	// Open deployment allows arbitrary bytecode execution and DB writes with
	// no balance check — a trivial CPU/DB DoS vector.
	if tx.To() == nil && len(tx.Data()) > 0 && s.evm != nil {
		allowedDeployer := relayerAddressFromEnv()
		if senderAddr != allowedDeployer {
			fmt.Printf("[RPC] ✗ Deploy rejected from %s (only %s may deploy)\n", senderAddr, allowedDeployer)
			// FIX (audit 2026-06-29, Brutal-Audit P2-04): nonce already
			// reserved above — without a receipt, txHash stays "pending"
			// forever from the wallet's point of view. See the matching fix
			// a little further down in this function (the !isV7 branch) for
			// the full explanation; same pattern here.
			s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: "contract deployment restricted to authorized address"}
		}

		fmt.Printf("[EVM] Deploying contract from %s, bytecode=%d bytes\n", senderAddr, len(tx.Data()))

		contractAddr, _, deployErr := s.evm.DeployContract(sender, tx.Data(), tx.Value())
		if deployErr != nil {
			fmt.Printf("[RPC] ✗ Deploy failed: %v\n", deployErr)
			// FIX (audit 2026-06-29, Brutal-Audit P2-04): same receipt gap.
			s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: "Deploy failed: " + deployErr.Error()}
		}

		contractAddrStr := strings.ToLower(contractAddr.Hex())
		s.mu.Lock()
		s.deployedContracts[txHash] = contractAddrStr
		s.txStatus[txHash] = true
		s.noteTxLocked(txHash)
		s.mu.Unlock()
		// FIX 7: Persist receipt so post-restart MetaMask gets correct status for deployment.
		// FIX: contractAddrStr is now persisted too (see SaveTxReceipt) — it used
		// to be dropped here, so getTransactionReceipt's DB fallback after a
		// restart returned contractAddress: null for every old deployment TX.
		s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x1", contractAddrStr)
		fmt.Printf("[RPC] ✓ Contract deployed: %s tx=%s\n", contractAddrStr, txHash)
		return txHash, nil
	}

	// ── CONTRACT CALL ────────────────────────────────────────────────────────
	// Only allow calls to known, Go-state-integrated selectors to prevent
	// Go/EVM ledger divergence. Unknown selectors could change EVM state
	// without updating Go-state (PostgreSQL), creating permanent inconsistency.
	// FIX (G9, beta-launch audit 2026-07-05): this check now lives in
	// checkPersistedCallAllowed (evm_engine.go), shared with CallContract's
	// own defense-in-depth copy of the same check — see that function's
	// comment for why. Preserves this function's own receipt-writing
	// behavior (SaveTxReceipt on every reject path) around the shared
	// decision.
	if tx.To() != nil && len(tx.Data()) >= 4 {
		if err := checkPersistedCallAllowed(*tx.To(), tx.Data(), senderAddr); err != nil {
			// FIX (audit 2026-06-29, Brutal-Audit P2-04): the nonce was already
			// reserved above before this gate runs. Returning bare without ever
			// calling SaveTxReceipt left txHash permanently receipt-less even
			// though its nonce slot was consumed — getTransactionReceipt(txHash)
			// returns null forever, which MetaMask renders as "still pending"
			// rather than failed, instead of resolving one way or the other.
			// Persist a failed (0x0) receipt so the wallet gets a definitive answer.
			s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: err.Error()}
		}
	}
	if tx.To() != nil && len(tx.Data()) > 0 && s.evm != nil {
		toAddr := *tx.To()
		toStr := strings.ToLower(toAddr.Hex())

		// Reload contract from DB
		bytecode, dbErr := s.state.LoadContract(toStr)
		if dbErr == nil && len(bytecode) > 0 {
			s.evm.SetCode(toAddr, bytecode)
			s.evm.LoadContractStorage(toAddr)
		}

		// FIX (BRUTAL-P2-05): persist an optimistic success receipt before
		// executing so MetaMask gets a non-null receipt immediately after
		// receiving the txHash. If execution fails, update to status=0x0 and
		// persist the failure durably — previously a failed contract call only
		// set in-memory txStatus/txError with no SaveTxReceipt call, so after
		// a restart MetaMask would see null receipt and show "pending" forever.
		s.mu.Lock()
		s.txStatus[txHash] = true
		s.noteTxLocked(txHash)
		s.mu.Unlock()
		s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x1", "")

		// persist=true: this is the actual execution of a real, signed
		// transaction submitted via sendRawTransaction — the one place where a
		// state change should genuinely be written to PostgreSQL.
		result, callErr := s.evm.CallContract(sender, toAddr, tx.Data(), tx.Value(), true)
		// Nonce was already reserved atomically at the top of eth_sendRawTransaction.
		// Do NOT increment here — that would double-count, skipping every other nonce.

		if callErr != nil {
			fmt.Printf("[RPC] ✗ Contract call failed: %v\n", callErr)
			s.mu.Lock()
			s.txStatus[txHash] = false
			s.noteTxLocked(txHash)
			s.txError[txHash] = callErr.Error()
			s.mu.Unlock()
			s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: "execution reverted: " + callErr.Error()}
		}

		fmt.Printf("[RPC] ✓ Contract call result: %x\n", result)
		s.evm.PersistContractStorage(toAddr)
		return txHash, nil
	}

	// FIX (audit 2026-06-29, Brutal-Audit P2-04): reachable for a legitimate
	// but unusual raw tx shape — zero value, empty data, to != nil (a pure
	// nonce-advancing no-op), or to == nil with empty data — that none of
	// the transfer/deploy/call branches above match. The nonce was already
	// reserved at the top of this function; without this, txHash would have
	// no receipt at all despite consuming a nonce slot, same "stuck
	// pending forever" gap as the other reject paths fixed above. Trivially
	// succeeds since there's nothing to execute.
	s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x1", "")
	return txHash, nil
}

func (s *EVMRPCServer) getTransactionReceipt(params []json.RawMessage) (interface{}, *RPCError) {
	if len(params) == 0 {
		return nil, nil
	}
	var txHash string
	if err := json.Unmarshal(params[0], &txHash); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}
	txHash = strings.ToLower(txHash)

	s.mu.Lock()
	_, knownStatus := s.txStatus[txHash]
	_, knownDeploy := s.deployedContracts[txHash]
	inMemory := knownStatus || knownDeploy
	var contractAddr interface{} = nil
	if addr, ok := s.deployedContracts[txHash]; ok {
		contractAddr = addr
	}
	status := "0x1"
	if succeeded, ok := s.txStatus[txHash]; ok && !succeeded {
		status = "0x0"
	}
	fromAddr := s.txSenders[txHash]
	toAddrMem := s.txTos[txHash]
	s.mu.Unlock()

	// If not in memory (node restarted), fall back to DB-persisted receipt.
	// This prevents MetaMask from showing successful transactions as "failed"
	// after a node restart clears the in-memory maps.
	if !inMemory {
		if dbFrom, dbTo, dbStatus, dbContract, found := s.state.GetTxReceipt(txHash); found {
			fromAddr = dbFrom
			toAddrMem = dbTo
			status = dbStatus
			if dbContract != "" {
				contractAddr = dbContract
			}
			inMemory = true // treat DB hit same as memory hit
		}
	}
	if !inMemory {
		return nil, nil
	}
	if fromAddr == "" {
		fromAddr = "0x0000000000000000000000000000000000000000"
	}
	toField := interface{}(nil)
	if toAddrMem != "" && contractAddr == nil {
		toField = toAddrMem
	}

	// The block this transaction was ACTUALLY included in.
	//
	// FIX (2026-07-26, reported from a live wallet): this used to report
	// dag.LatestBlock() — the current chain head — as the transaction's
	// block, so the answer changed on every call (measured 0x1d5102, then
	// 0x1d511a seconds later). A wallet computes confirmations as
	// head - receipt.blockNumber, which under that behaviour is permanently
	// zero, so MetaMask eventually gave up and showed a 150 AEQ transfer that
	// had landed correctly in block #1918326 as "Senden fehlgeschlagen".
	// See tx_block_index.go.
	txHeight, txBlockHash, txIndex, indexed := s.state.LookupTxBlock(txHash)
	height := uint64(0)
	blockHash := "0x" + strings.Repeat("0", 63) + "1"
	logIndex := 0
	if indexed {
		height = uint64(txHeight)
		blockHash = "0x" + txBlockHash
		logIndex = txIndex
	} else if block := s.dag.LatestBlock(); block != nil {
		// Not indexed (a transaction from before this index existed): fall
		// back to the old behaviour rather than returning nothing, so historic
		// transactions still produce a receipt.
		height = uint64(block.Height)
		blockHash = "0x" + block.Hash
	}

	return map[string]interface{}{
		"transactionHash":   txHash,
		"transactionIndex":  fmt.Sprintf("0x%x", logIndex),
		"blockHash":         blockHash,
		"blockNumber":       fmt.Sprintf("0x%x", height),
		"from":              fromAddr,
		"to":                toField,
		"cumulativeGasUsed": "0x5B8D80",
		"gasUsed":           "0x5208", // realistic: 21000 for simple ops
		"contractAddress":   contractAddr,
		"logs":              []interface{}{},
		"logsBloom":         "0x" + strings.Repeat("0", 512),
		"status":            status,
		"type":              "0x2",
	}, nil
}

func (s *EVMRPCServer) getTransactionByHash(params []json.RawMessage) (interface{}, *RPCError) {
	if len(params) == 0 {
		return nil, nil
	}
	var txHash string
	if err := json.Unmarshal(params[0], &txHash); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}
	txHash = strings.ToLower(txHash)

	// P2-AUDIT: Return the real sender and destination stored at submission time
	// instead of always returning the zero address. MetaMask and block explorers
	// use this to display the correct from/to fields for a transaction.
	s.mu.Lock()
	fromAddr, known := s.txSenders[txHash]
	toAddr := s.txTos[txHash]
	s.mu.Unlock()
	// FIX: unlike getTransactionReceipt, this never fell back to the DB-persisted
	// receipt when the in-memory txSenders map didn't have the hash (i.e. after
	// a node restart) — so MetaMask/explorers would get a receipt (status
	// known via getTransactionReceipt's DB fallback) but getTransactionByHash
	// for the same hash returned null, an inconsistent pair of RPC responses
	// for one transaction.
	if !known {
		if dbFrom, dbTo, _, _, found := s.state.GetTxReceipt(txHash); found {
			fromAddr = dbFrom
			toAddr = dbTo
			known = true
		}
	}
	if !known {
		// Unknown txHash — return null per Ethereum spec (not a synthetic object)
		return nil, nil
	}
	var toField interface{} = nil
	if toAddr != "" {
		toField = toAddr
	}

	// FIX (Monster Audit 2026-07-12, P3): nonce/value/input below are
	// hardcoded placeholders, not "the real value happens to be zero" —
	// flagging honestly rather than leaving it unexplained. This chain has no
	// single per-account EVM-style nonce counter spanning every tx type
	// (transfers, registrations, swaps, liquidity ops each have their own
	// replay-protection scheme, e.g. ConsumeSwapNonce for swaps only); txSenders/
	// txTos/GetTxReceipt (this function's only data sources) don't track a
	// per-tx amount either. Populating these correctly needs a real
	// per-account nonce counter and persisted tx value, threaded through
	// every transaction-recording path — a genuine feature, not a one-line
	// fix, so left as an honest placeholder rather than a fabricated number.
	// FIX (2026-07-26): blockHash/blockNumber are no longer placeholders.
	// The comment above was right that this needed a real transaction-
	// recording path rather than a fabricated number — chain_tx_block_index
	// (tx_block_index.go) is that path, written by every node when it accepts
	// a block. Claiming block 1 for a transaction that landed in #1918326 is
	// what left a wallet unable to follow it, so it declared a successful
	// 150 AEQ transfer failed.
	//
	// A transaction that is known but not yet in a block reports null for
	// both fields, which is exactly what the Ethereum spec prescribes for a
	// pending transaction — and what tells a wallet to keep waiting instead
	// of concluding anything.
	var blockHashField, blockNumberField, txIndexField interface{} = nil, nil, nil
	if h, bh, idx, ok := s.state.LookupTxBlock(txHash); ok {
		blockHashField = "0x" + bh
		blockNumberField = fmt.Sprintf("0x%x", uint64(h))
		txIndexField = fmt.Sprintf("0x%x", idx)
	}

	return map[string]interface{}{
		"hash":             txHash,
		"nonce":            "0x0",
		"blockHash":        blockHashField,
		"blockNumber":      blockNumberField,
		"transactionIndex": txIndexField,
		"from":             fromAddr,
		"to":               toField,
		"value":            "0x0",
		"gas":              "0x5B8D80",
		"gasPrice":         "0x0",
		"input":            "0x",
	}, nil
}

func (s *EVMRPCServer) getBlockByNumber(params []json.RawMessage) (interface{}, *RPCError) {
	// FIX (audit 2026-06-29): this used to ignore params entirely and always
	// return the latest block, even when a caller asked for a specific
	// historical height — silently wrong for any client that fetches a
	// block by number to verify something about that exact height (a block
	// explorer, a confirmation-count check). dag.GetBlockByHeight already
	// existed for this (used elsewhere for the real /api/blocks lookups)
	// but wasn't wired up here. "latest"/"pending"/"earliest" and any
	// unparseable value keep the old always-return-latest behavior, which
	// is the correct interpretation for those tags anyway.
	var tag string
	if len(params) > 0 {
		json.Unmarshal(params[0], &tag) //nolint:errcheck — fall through to latest on bad input
	}
	if tag != "" && tag != "latest" && tag != "pending" && tag != "earliest" {
		var height int64
		if _, err := fmt.Sscanf(strings.TrimPrefix(tag, "0x"), "%x", &height); err == nil {
			if block := s.dag.GetBlockByHeight(height); block != nil {
				return s.blockToMap(block), nil
			}
			return nil, nil
		}
	}
	block := s.dag.LatestBlock()
	if block == nil {
		return nil, nil
	}
	return s.blockToMap(block), nil
}

func (s *EVMRPCServer) getBlockByHash(params []json.RawMessage) (interface{}, *RPCError) {
	// FIX (audit 2026-06-29): same gap as getBlockByNumber above — a
	// specific requested hash was always ignored in favor of the latest
	// block. dag.GetBlockByHash already existed; wire it up.
	var hash string
	if len(params) > 0 {
		json.Unmarshal(params[0], &hash) //nolint:errcheck — fall through to latest on bad input
	}
	hash = strings.TrimPrefix(strings.ToLower(hash), "0x")
	if hash != "" {
		if block := s.dag.GetBlockByHash(hash); block != nil {
			return s.blockToMap(block), nil
		}
		return nil, nil
	}
	block := s.dag.LatestBlock()
	if block == nil {
		return nil, nil
	}
	return s.blockToMap(block), nil
}

func (s *EVMRPCServer) blockToMap(block *Block) map[string]interface{} {
	return map[string]interface{}{
		"number":           fmt.Sprintf("0x%x", block.Height),
		"hash":             "0x" + block.Hash,
		"parentHash":       "0x" + strings.Repeat("0", 64),
		"timestamp":        fmt.Sprintf("0x%x", block.Timestamp),
		"transactions":     []interface{}{},
		"gasLimit":         "0x1000000",
		"gasUsed":          "0x0",
		"difficulty":       "0x0",
		"totalDifficulty":  "0x0",
		"miner":            "0x0000000000000000000000000000000000000000",
		"extraData":        "0x",
		"logsBloom":        "0x" + strings.Repeat("0", 512),
		"sha3Uncles":       "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		"stateRoot":        "0x" + block.Hash,
		"receiptsRoot":     "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		"transactionsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
		"size":             "0x1",
		"uncles":           []interface{}{},
		"nonce":            "0x0000000000000000",
		"baseFeePerGas":    "0x0",
	}
}

// ─── HELPERS ─────────────────────────────────────────────────────────────────

type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string {
	return e.Message
}

func writeError(w http.ResponseWriter, code int, message string, id interface{}) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}

func errorResponse(id interface{}, code int, message string) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
}

func min4(a, b int) int {
	if a < b {
		return a
	}
	return b
}
