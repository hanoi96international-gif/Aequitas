package keeper

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

// weiPerAEQ is 10^18, the fixed AEQ-to-wei scaling factor. Performance audit
// 2026-07-06: this used to be recomputed via big.Int.Exp at 13 separate call
// sites across evm_engine.go/evm_storage.go/evm_rpc.go/evm_v6mirror.go/
// api.go/register.go, several in the hottest path (getBalance, the
// balanceOf/eth_call intercepts, syncBalanceLocked). big.Int.Exp isn't free
// (repeated-squaring over a 60-bit result), and 10^18 never changes — one
// package-level value computed once. Callers must never mutate this shared
// instance in place (e.g. weiPerAEQ.Mul(weiPerAEQ, x) would corrupt every
// other reader) — always use it as a read-only input to new(big.Int)/
// new(big.Float), the same way every existing call site already did.
var weiPerAEQ = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

// aeqToWei converts a whole/fractional AEQ amount (a float64, as stored in
// AccountState.Balance.Float()) to its wei representation, rounding toward
// zero on any leftover fraction (matching (*big.Float).Int's documented
// behavior). Performance audit 2026-07-06: this exact 256-bit-precision
// big.Float multiply was hand-copied at 7 separate call sites across
// evm_engine.go/evm_storage.go — precisely the duplication pattern that let
// the historical P1 precision bug (raw micro-units treated as whole-AEQ,
// overstating balances 1e6×) live on at some call sites after being fixed at
// others. One helper now; a future precision fix only has one place to land.
func aeqToWei(aeq float64) *big.Int {
	wei, _ := new(big.Float).SetPrec(256).Mul(
		new(big.Float).SetFloat64(aeq),
		new(big.Float).SetInt(weiPerAEQ),
	).Int(nil)
	if wei == nil {
		wei = new(big.Int)
	}
	// A negative balance must never reach the EVM state, and this is not
	// tidiness. A negative big.Int cannot be RLP-encoded, so ONE negative
	// account makes the whole state fail to commit — go-ethereum aborts with
	// "rlp: cannot encode negative big.Int". Measured on Contabo2 on
	// 2026-08-18: the V7 contract deploy died with exactly that at every boot,
	// the EVM mirror was therefore never rebuilt, and from then on every
	// single block produced a StateRoot the primary disagreed with. The node
	// spent the day resyncing itself over and over against a cause no resync
	// could touch.
	//
	// So the clamp is the lesser wrong by a wide margin: one account mirrored
	// as zero is a bug to hunt down (evmBalanceWei logs which one), while an
	// uncommittable mirror silently diverges the entire node from the network.
	if wei.Sign() < 0 {
		return new(big.Int)
	}
	return wei
}

// evmBalanceWei converts an account balance for the EVM mirror and says so
// loudly when it had to clamp. aeqToWei alone would hide the account: by the
// time the value reaches it there is no address left to name, and a negative
// balance is a real defect in whatever wrote it — worth finding, not worth
// crashing the mirror over.
func evmBalanceWei(addrHex string, aeq float64) *big.Int {
	if aeq < 0 {
		fmt.Printf("[ALERT] [EVM-MIRROR] %s holds a negative balance (%.6f AEQ) — mirrored as 0. "+
			"A negative big.Int cannot be RLP-encoded, so mirroring it as-is would make the EVM state "+
			"fail to commit and this node's StateRoot diverge from the network's. Find the write that produced it.\n",
			addrHex, aeq)
		return new(big.Int)
	}
	return aeqToWei(aeq)
}

// EVMEngine wraps go-ethereum EVM for contract deployment and calls.
// Design principle: every operation gets a fresh StateDB loaded from PostgreSQL.
// This avoids all stale-trie issues at the cost of slightly more DB reads.
type EVMEngine struct {
	chainState *ChainState
}

func NewEVMEngine(cs *ChainState) (*EVMEngine, error) {
	cs.InitV6StateTables()
	e := &EVMEngine{chainState: cs}
	e.RestoreV6FromMirror()
	checkV7SlotsMatchDeployedVersion()
	return e, nil
}

// ─── CHAIN CONFIG ─────────────────────────────────────────────────────────────

func chainConfig() *params.ChainConfig {
	shanghai := uint64(0)
	return &params.ChainConfig{
		ChainID:             big.NewInt(1926),
		HomesteadBlock:      big.NewInt(0),
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		BerlinBlock:         big.NewInt(0),
		LondonBlock:         big.NewInt(0),
		ShanghaiTime:        &shanghai,
	}
}

// blockContext takes an explicit ts (unix seconds) instead of reading
// time.Now() internally. Every CallContract/DeployContract invocation now
// captures its own timestamp ONCE at the call site and passes it through,
// rather than blockContext() silently re-reading the wall clock on every
// call. This matters because the only EVM-level functions that consult
// block.timestamp (Escrow timelocks, Guardian delays, inactivity rules) are
// currently reachable through exactly one persist=true execution per logical
// user action (gated by the knownPublicSelectors allowlist in evm_rpc.go) —
// no other node ever independently replays the same call, so wall-clock time
// is safe *today*. Making the timestamp an explicit parameter rather than a
// hidden time.Now() read means that guarantee is visible and enforceable at
// every call site, and if a future change ever needs to replay one of these
// calls deterministically (e.g. from a block's own Timestamp field instead
// of wall-clock), there's a single obvious parameter to redirect — not a
// buried global clock read that would need to be hunted down.
func blockContext(ts uint64) vm.BlockContext {
	return vm.BlockContext{
		CanTransfer: func(_ vm.StateDB, _ common.Address, _ *big.Int) bool { return true },
		Transfer:    func(_ vm.StateDB, _, _ common.Address, _ *big.Int) {},
		GetHash:     func(_ uint64) common.Hash { return common.Hash{} },
		Coinbase:    common.Address{},
		BlockNumber: big.NewInt(1), // P2-8: fixed — AequitasV7 uses block.timestamp not block.number; wall-clock is non-deterministic between nodes
		Time:        ts,
		Difficulty:  big.NewInt(0),
		GasLimit:    30_000_000,
		BaseFee:     big.NewInt(0),
	}
}

// ─── FRESH STATE DB ───────────────────────────────────────────────────────────

// newStateDB creates a fresh in-memory StateDB loaded from PostgreSQL.
// Called before every Deploy or Call to ensure consistent state.
//
// FIX (G5, beta-launch audit 2026-07-05): this used to unconditionally load
// EVERY registered account's balance via GetAllAccounts() on every single
// call — including a read-only eth_call like balanceOf(address), which only
// ever needs ONE account's balance. That's an O(N) cost (N = registered
// humans) on the hottest path in this codebase, and the earlier /rpc rate
// limit (evm_rpc.go) only bounds how OFTEN this runs, not the O(N) cost of
// each run. onlyAddrs, when non-empty, restricts loading to exactly those
// addresses (via ChainState.GetAccountsForAddresses, which also correctly
// pages in a cold/evicted account — GetAllAccounts has no such fallback, so
// this is strictly more correct for the addresses it's given, not just
// faster). Passing no addresses preserves the original eager-load-
// everything behavior — used by DeployContract, a rare, one-time,
// already-completed-for-this-chain's-two-contracts operation where the
// full-load cost has never mattered and isn't worth touching.
func (e *EVMEngine) newStateDB(onlyAddrs ...common.Address) (*state.StateDB, state.Database, error) {
	memDB := rawdb.NewMemoryDatabase()
	db := state.NewDatabase(memDB)
	sdb, err := state.New(common.Hash{}, db, nil)
	if err != nil {
		return nil, nil, err
	}

	// Load account balances — either everyone (onlyAddrs empty, e.g. DeployContract)
	// or just the requested addresses (a normal CallContract invocation).
	// P0-2: Do NOT call LoadNonce per account — that triggers N PostgreSQL queries
	// (one per account) and creates a DoS vector. EVM nonces for sends are managed
	// in the RPC layer separately; the EVM itself does not need per-account nonces
	// for call execution (only for CREATE). Removing SetNonce here has no effect
	// on the correctness of contract calls or view calls.
	var accountsToLoad []*AccountState
	if len(onlyAddrs) == 0 {
		accountsToLoad = e.chainState.GetAllAccounts()
	} else {
		addrStrs := make([]string, len(onlyAddrs))
		for i, a := range onlyAddrs {
			addrStrs[i] = strings.ToLower(a.Hex())
		}
		accountsToLoad = e.chainState.GetAccountsForAddresses(addrStrs)
	}
	for _, acc := range accountsToLoad {
		addr := common.HexToAddress(acc.Address)
		// P1-FIX: acc.Balance is a Decimal (int64 micro-units, 1 AEQ = 1e6 micro).
		// Use .Float() to get the real AEQ value, then convert to wei (×1e18).
		// The previous code used big.NewInt(int64(acc.Balance)) which treated the
		// raw micro-unit integer as whole-AEQ, producing balances 1e6× too high.
		sdb.SetBalance(addr, evmBalanceWei(acc.Address, acc.Balance.Float()))
	}

	// Load all contract bytecodes and storage. Performance audit 2026-07-06:
	// GetAllContracts() only ever returns 2 rows today (V7_CONTRACT_ADDR +
	// BIO_VERIFIER_ADDR — see evm_v6mirror.go), so the per-contract storage
	// query below is cheap in practice. But it IS an unconditional per-contract
	// DB round trip inside this loop, the same N-scaling shape the accounts
	// loop above used to have before G5 fixed it to load only the accounts a
	// call actually touches. If a 3rd contract is ever deployed, revisit
	// whether this still needs bounding the same way.
	for _, addrStr := range e.chainState.GetAllContracts() {
		addr := common.HexToAddress(addrStr)

		code, err := e.chainState.LoadContract(addrStr)
		if err != nil || len(code) == 0 {
			continue
		}
		sdb.SetCode(addr, code)

		// Load storage slots
		if e.chainState.db != nil {
			rows, err := e.chainState.db.Query(
				`SELECT slot, value FROM evm_storage WHERE address = $1`, addrStr)
			if err == nil {
				for rows.Next() {
					var slot, val string
					if err := rows.Scan(&slot, &val); err != nil {
						fmt.Printf("[WARN] EVM storage scan error for %s: %v\n", addr.Hex(), err)
						continue
					}
					sdb.SetState(addr, common.HexToHash(slot), common.HexToHash(val))
				}
				rows.Close()
			}
		}
	}

	// Don't commit — keep state in dirty/pending form so EVM can read it directly
	return sdb, db, nil
}

// newStateDBLocked is newStateDB's lock-free sibling for callers that
// already hold cs.mu (see ChainState.getAccountsForAddressesLocked's own
// comment for the live deadlock this closes). Unlike newStateDB, it
// requires at least one address — the empty/"load everyone" path goes
// through GetAllAccounts, which has no lock-free sibling today because
// nothing needs one; adding it just for symmetry here would be unused,
// untested surface.
func (e *EVMEngine) newStateDBLocked(onlyAddrs ...common.Address) (*state.StateDB, state.Database, error) {
	if len(onlyAddrs) == 0 {
		return nil, nil, fmt.Errorf("newStateDBLocked: at least one address required")
	}
	memDB := rawdb.NewMemoryDatabase()
	db := state.NewDatabase(memDB)
	sdb, err := state.New(common.Hash{}, db, nil)
	if err != nil {
		return nil, nil, err
	}

	addrStrs := make([]string, len(onlyAddrs))
	for i, a := range onlyAddrs {
		addrStrs[i] = strings.ToLower(a.Hex())
	}
	for _, acc := range e.chainState.getAccountsForAddressesLocked(addrStrs) {
		addr := common.HexToAddress(acc.Address)
		sdb.SetBalance(addr, evmBalanceWei(acc.Address, acc.Balance.Float()))
	}

	// Contract code/storage loading is plain DB I/O — GetAllContracts/
	// LoadContract never touch cs.mu, so no locked sibling is needed here.
	for _, addrStr := range e.chainState.GetAllContracts() {
		addr := common.HexToAddress(addrStr)
		code, err := e.chainState.LoadContract(addrStr)
		if err != nil || len(code) == 0 {
			continue
		}
		sdb.SetCode(addr, code)
		if e.chainState.db != nil {
			rows, err := e.chainState.db.Query(
				`SELECT slot, value FROM evm_storage WHERE address = $1`, addrStr)
			if err == nil {
				for rows.Next() {
					var slot, val string
					if err := rows.Scan(&slot, &val); err != nil {
						fmt.Printf("[WARN] EVM storage scan error for %s: %v\n", addr.Hex(), err)
						continue
					}
					sdb.SetState(addr, common.HexToHash(slot), common.HexToHash(val))
				}
				rows.Close()
			}
		}
	}
	return sdb, db, nil
}

// ─── DEPLOY ───────────────────────────────────────────────────────────────────

func (e *EVMEngine) DeployContract(from common.Address, bytecode []byte, value *big.Int) (contractAddr common.Address, ret []byte, err error) {
	// Same reasoning as CallContract: the EVM layer has no real wei ledger,
	// so a deployment carrying value > 0 would silently drop it rather than
	// crediting the new contract.
	if value != nil && value.Sign() > 0 {
		return common.Address{}, nil, fmt.Errorf("contract deployment with msg.value > 0 is not supported on this chain")
	}
	ts := uint64(time.Now().Unix())
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("EVM panic: %v", r)
			contractAddr = common.Address{}
			ret = nil
		}
	}()

	sdb, _, err := e.newStateDB()
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("stateDB: %w", err)
	}

	// NOTE (audit 2026-06-29, considered, not changed): when called from
	// eth_sendRawTransaction, evm_rpc.go's nonce reservation has already
	// committed nonce+1 to the DB by the time we get here (see its own
	// comment: "Do NOT call SaveNonce here"), so LoadNonce returns nonce+1,
	// not the tx's actual tx.Nonce(). evm.Create derives the new contract's
	// address from (from, nonce-as-set-on-sdb), so the address computed
	// here is offset by one from what a standards-compliant Ethereum CREATE
	// would produce for the same signed transaction. Not fixed: contract
	// deployment is restricted to RELAYER_ADDRESS/the node's own signing
	// key (see the caller's own access check), used only for this chain's
	// own one-time bootstrap deploys (V7, BioVerifier — both already live
	// at their hardcoded addresses), and nothing here cross-checks the
	// resulting address against an independent CREATE computation — so the
	// address this function produces is used consistently by this same
	// implementation going forward. Restructuring DeployContract to accept
	// the pre-reservation nonce as an explicit parameter would fix the
	// offset but touches a rarely-exercised, already-completed code path
	// for no live consensus or fund-safety benefit.
	nonce := e.chainState.LoadNonce(strings.ToLower(from.Hex()))
	sdb.SetNonce(from, nonce)

	txCtx := vm.TxContext{Origin: from, GasPrice: big.NewInt(0)}
	evm := vm.NewEVM(blockContext(ts), txCtx, sdb, chainConfig(), vm.Config{})

	_, contractAddr, _, err = evm.Create(
		vm.AccountRef(from),
		bytecode,
		30_000_000,
		value,
	)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("deploy failed: %w", err)
	}

	// Commit to get runtime code into trie
	sdb.Commit(0, false)

	// Read runtime bytecode directly from stateDB after commit
	runtimeCode := sdb.GetCode(contractAddr)
	if len(runtimeCode) == 0 {
		return common.Address{}, nil, fmt.Errorf("deploy succeeded but no runtime code found")
	}

	addrStr := strings.ToLower(contractAddr.Hex())
	fromStr := strings.ToLower(from.Hex())

	// Persist to PostgreSQL
	e.chainState.SaveContract(addrStr, runtimeCode, fromStr)
	// Do NOT call SaveNonce here — when invoked via eth_sendRawTransaction the
	// RPC layer already reserved (nonce+1) before calling DeployContract.
	// Calling SaveNonce a second time would advance the nonce to nonce+2,
	// causing every subsequent tx from the same sender to fail with "nonce too low".

	// P2-8: Persist ALL storage slots from stateDB to PostgreSQL.
	// go-ethereum v1.13.0 StateDB does not expose a public ForEachStorage
	// iterator, so we probe slots explicitly.
	//
	// Strategy: V7 uses two layout zones:
	//
	//	Zone A — simple state variables and fixed arrays: slots 0–199.
	//	          All Solidity state variables (totalSupply, mappings' "base"
	//	          slots, CAP_MULTIPLIERS[5], THRESHOLDS[5], BIO_VERIFIER …)
	//	          are numbered sequentially and well within 200.
	//	Zone B — mapping values live at keccak256(key || slotBase) which are
	//	          outside the zone-A range. These are populated by
	//	          MigrateEVMFromGoState after every upgrade; no constructor
	//	          sets them, so they are always zero at deploy time.
	//
	// Scanning 0–199 costs one GetState call per slot (cheap, in-memory
	// after Commit) and is deterministic regardless of which keys users
	// have registered.
	savedCount := 0
	for i := int64(0); i < 200; i++ {
		slot := common.BigToHash(big.NewInt(i))
		val := sdb.GetState(contractAddr, slot)
		if val != (common.Hash{}) {
			e.chainState.SaveStorageSlot(addrStr, slot.Hex(), val.Hex())
			savedCount++
		}
	}
	fmt.Printf("[EVM] Constructor stored %d non-zero slots (probed 0–199)\n", savedCount)

	fmt.Printf("[EVM] ✓ Deployed %s (%d bytes)\n", contractAddr.Hex(), len(runtimeCode))
	return contractAddr, runtimeCode, nil
}

// ─── CALL ─────────────────────────────────────────────────────────────────────

// CallContract executes a contract call against a fresh StateDB built from
// PostgreSQL. The persist parameter controls whether the resulting state
// changes are written back to PostgreSQL:
//   - persist=true:  use ONLY for a call that represents a real, intended
//     state change (the actual execution inside sendRawTransaction).
//   - persist=false: use for read-only queries (eth_call, isHuman/balanceOf
//     lookups in api.go) AND for dry-run simulations (register.go's
//     pre-flight check before the real submit). Nothing is written back.
//
// Previously this function ALWAYS persisted, regardless of why it was
// called. That meant a pure eth_call (e.g. checking someone's balance) or
// a dry-run simulation (checking whether a registration WOULD succeed,
// before actually submitting it) had the exact same side effect as a real,
// committed registration: isHuman/balanceOf were written to evm_storage as
// if the call had truly happened. In practice this meant every attempt to
// register — even ones whose real submission later failed or was never
// sent — already "registered" the wallet the moment the dry-run ran,
// making "already registered" errors appear for wallets that, from the
// chain's own database tables, looked completely unregistered. Database
// resets could never fix this because the very next read-only status
// check would silently re-create the same state.
// knownV7PublicPersistSelectors lists every V7 selector a persisting call
// is allowed to reach — see checkPersistedCallAllowed's comment. Kept in
// sync with sendRawTransaction's (evm_rpc.go) own copy of this list; both
// call checkPersistedCallAllowed rather than checking independently.
var knownV7PublicPersistSelectors = map[string]bool{
	"a9059cbb": true, // transfer(address,uint256) — normally intercepted and routed through Go-state before reaching here; allowed defensively
	"70a08231": true, // balanceOf
	"dd62ed3e": true, // allowance (read-only view)
	"18160ddd": true, // totalSupply
	"06fdde03": true, // name
	"95d89b41": true, // symbol
	"313ce567": true, // decimals
}

// checkPersistedCallAllowed is the single source of truth for which
// state-changing EVM calls may persist — called both by sendRawTransaction
// (evm_rpc.go), the public entry point for external raw transactions, and
// by CallContract itself right below.
//
// FIX (G9, beta-launch audit 2026-07-05): before this, the allowlist lived
// ONLY inside sendRawTransaction's handler — CallContract trusted its
// caller completely for the persist=true decision. That was never an
// exploitable bug (the only persist=true call site in the whole codebase
// IS sendRawTransaction, already gated by an identical check — verified by
// grepping every .CallContract( call site), but it made the allowlist a
// single point of enforcement: a future call site that added persist=true
// without knowing to replicate this exact check would silently reopen the
// Go/EVM ledger divergence this allowlist exists to prevent (see
// dumpAndPersistStorageWithNullifier's hardcoded V7-slot-layout comment for
// what "divergence" means concretely here). Enforcing it inside
// CallContract too makes bypassing it structurally impossible, not just a
// convention every future caller has to remember.
func checkPersistedCallAllowed(to common.Address, data []byte, senderAddr string) error {
	if len(data) < 4 {
		return nil // no selector to check — mirrors the original gate's own `len(tx.Data()) >= 4` condition
	}
	if !strings.EqualFold(to.Hex(), V7_CONTRACT_ADDR) {
		return fmt.Errorf("state-changing calls are only supported for the V7 contract")
	}
	sel := hex.EncodeToString(data[:4])
	if knownV7PublicPersistSelectors[sel] {
		return nil
	}
	if sel == "13b81eb0" { // registerWithSig — only the relayer may call it directly (on behalf of /api/register)
		relayerAddr := relayerAddressFromEnv()
		if relayerAddr != "" && strings.ToLower(senderAddr) == relayerAddr {
			return nil
		}
		return fmt.Errorf("registerWithSig must be called via /api/register (direct calls bypass Go-state updates)")
	}
	return fmt.Errorf("selector %s not supported for persisting calls — use /api/* endpoints", sel)
}

// addressLikeWordsInCalldata scans data (assumed ABI-encoded: a 4-byte
// selector followed by 32-byte-aligned words) for words that are valid
// ABI-encoded addresses — the top 12 bytes zero, matching how solidity/abi
// packs an `address` parameter into a 32-byte slot. Used by CallContract to
// figure out which accounts (beyond from/to) a call might need to read, so
// newStateDB can load just those instead of every registered account — see
// its own comment.
//
// Deliberately a byte-pattern heuristic instead of per-selector ABI
// decoding (extractTouchedEntities, used elsewhere in this file for a
// different purpose — persistence bookkeeping, not state loading, and only
// handles the one selector that needs it there): every currently-reachable
// selector here (balanceOf, allowance, registerWithSig) encodes its address
// argument(s) exactly this way, and the false-positive case (a random
// 256-bit value, e.g. inside registerWithSig's ZK proof arrays, that
// happens to have 12 zero leading bytes) only costs one extra harmless
// account load — a coincidence astronomically unlikely for a real
// pseudorandom field element in the first place. There is no false-negative
// case for any CURRENT selector: this only needs to be conservative
// (never miss a real address), not exact.
func addressLikeWordsInCalldata(data []byte) []common.Address {
	if len(data) <= 4 {
		return nil
	}
	body := data[4:]
	var out []common.Address
	for i := 0; i+32 <= len(body); i += 32 {
		word := body[i : i+32]
		isAddrShaped := true
		for _, b := range word[:12] {
			if b != 0 {
				isAddrShaped = false
				break
			}
		}
		if !isAddrShaped {
			continue
		}
		out = append(out, common.BytesToAddress(word[12:32]))
	}
	return out
}

// CallContractLocked is CallContract's lock-free, read-only sibling for
// callers that already hold cs.mu — currently only verifyZKProof, called
// from inside replayTransactions' cs.mu-locked section (see
// ChainState.getAccountsForAddressesLocked's comment for the deadlock this
// closes). Persisted calls need dumpAndPersistStorageWithNullifier, which
// itself touches ChainState via SaveContractStorage-style writes that
// aren't safe to reason about re-entrantly here, so this only supports the
// read-only (persist=false) path CallContract's own callers already use for
// every already-locked case — a persist=true request is rejected outright
// rather than silently skipping persistence.
func (e *EVMEngine) CallContractLocked(from, to common.Address, data []byte, value *big.Int) (ret []byte, err error) {
	if value != nil && value.Sign() > 0 {
		return nil, fmt.Errorf("contract calls with msg.value > 0 are not supported on this chain (no native value-transfer mechanism in the EVM layer); use a plain transfer or the V7 transfer() selector instead")
	}
	ts := uint64(time.Now().Unix())
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("EVM panic: %v", r)
			ret = nil
		}
	}()

	onlyAddrs := append([]common.Address{from, to}, addressLikeWordsInCalldata(data)...)
	sdb, _, err := e.newStateDBLocked(onlyAddrs...)
	if err != nil {
		return nil, fmt.Errorf("stateDB: %w", err)
	}

	code := sdb.GetCode(to)
	if len(code) == 0 {
		toHex := strings.ToLower(to.Hex())
		if toHex == "0xca11bde05977b3631167028862be2a173976ca11" ||
			toHex == "0x0000000000000000000000000000000000000000" {
			return make([]byte, 32), nil
		}
		return nil, fmt.Errorf("no code at %s", to.Hex())
	}

	txCtx := vm.TxContext{Origin: from, GasPrice: big.NewInt(0)}
	evm := vm.NewEVM(blockContext(ts), txCtx, sdb, chainConfig(), vm.Config{})

	var execErr error
	ret, _, execErr = evm.Call(
		vm.AccountRef(from),
		to,
		data,
		30_000_000,
		value,
	)
	if execErr != nil {
		reason := decodeRevertReason(ret)
		if reason != "" {
			return nil, fmt.Errorf("%s", reason)
		}
		return nil, fmt.Errorf("call failed: %w", execErr)
	}
	return ret, nil
}

func (e *EVMEngine) CallContract(from, to common.Address, data []byte, value *big.Int, persist bool) (ret []byte, err error) {
	// FIX: CanTransfer/Transfer in blockContext() are permanent no-op stubs —
	// there is no real wei ledger backing the EVM StateDB (Go-state/PostgreSQL
	// is authoritative for AEQ balances). Without this check, a contract call
	// carrying value > 0 would execute "successfully" while the value is
	// silently dropped: never debited from the sender, never credited to
	// anyone, on either ledger. The two value-bearing flows that ARE real
	// (plain native transfer with no calldata, and the a9059cbb ERC-20
	// transfer selector) are intercepted and routed through Go-state
	// (Transfer/TransferWithV7Fee) BEFORE reaching this function — see
	// sendRawTransaction in evm_rpc.go. Any other call that still carries
	// value here would otherwise be a silent fund-loss bug, so reject it
	// outright instead of pretending it succeeded.
	if value != nil && value.Sign() > 0 {
		return nil, fmt.Errorf("contract calls with msg.value > 0 are not supported on this chain (no native value-transfer mechanism in the EVM layer); use a plain transfer or the V7 transfer() selector instead")
	}
	if persist {
		if err := checkPersistedCallAllowed(to, data, strings.ToLower(from.Hex())); err != nil {
			return nil, err
		}
	}
	ts := uint64(time.Now().Unix())
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("EVM panic: %v", r)
			ret = nil
		}
	}()

	// FIX (G5, beta-launch audit 2026-07-05): load only the accounts this
	// specific call could plausibly need (sender, target contract, and any
	// address-shaped argument in the calldata) instead of every registered
	// human — see newStateDB's own comment.
	onlyAddrs := append([]common.Address{from, to}, addressLikeWordsInCalldata(data)...)
	sdb, db, err := e.newStateDB(onlyAddrs...)
	if err != nil {
		return nil, fmt.Errorf("stateDB: %w", err)
	}

	// Verify contract code is loaded
	code := sdb.GetCode(to)
	fmt.Printf("[EVM] CallContract to=%s codeLen=%d data=%x persist=%v\n",
		to.Hex(), len(code), data[:min4b(len(data), 4)], persist)

	if len(code) == 0 {
		// MetaMask Mobile (and some other wallets) call well-known system
		// contracts that exist on mainnet but not on a custom chain:
		//   - Multicall3 (0xcA11bde...) — used to batch eth_call requests
		//   - Zero address (0x0000...) — probed for token symbol/decimals
		//
		// Returning a hard error here makes MetaMask Mobile abort the
		// entire transaction flow. Instead we return an empty 32-byte
		// result (the standard ABI-encoded zero/empty value) so the wallet
		// gracefully treats the call as "not supported" and falls back to
		// its single-call path. This is the same behavior as geth when
		// calling a non-existent contract with staticcall.
		toHex := strings.ToLower(to.Hex())
		if toHex == "0xca11bde05977b3631167028862be2a173976ca11" ||
			toHex == "0x0000000000000000000000000000000000000000" {
			fmt.Printf("[EVM] Known system contract %s — returning empty result\n", to.Hex())
			return make([]byte, 32), nil
		}
		return nil, fmt.Errorf("no code at %s", to.Hex())
	}

	// FIX (pre-launch audit 2026-08-16): some calls modify state for an address
	// or nullifier that is only reachable THROUGH storage which the call itself
	// then clears (a swept human's guardian and biometric records, a revoker's
	// guardian). Read those pointers now, while sdb is still the pre-call state —
	// after the call the pointer is gone and the write can no longer be found.
	var preAddrs []common.Address
	var releasedNullifiers [][32]byte
	if persist {
		preAddrs, releasedNullifiers = extractPreCallEntities(sdb, to, from, data)
	}

	txCtx := vm.TxContext{Origin: from, GasPrice: big.NewInt(0)}
	evm := vm.NewEVM(blockContext(ts), txCtx, sdb, chainConfig(), vm.Config{})

	var execErr error
	ret, _, execErr = evm.Call(
		vm.AccountRef(from),
		to,
		data,
		30_000_000,
		value,
	)
	if execErr != nil {
		reason := decodeRevertReason(ret)
		if reason != "" {
			return nil, fmt.Errorf("%s", reason)
		}
		return nil, fmt.Errorf("call failed: %w", execErr)
	}

	if !persist {
		fmt.Printf("[EVM] Call result (not persisted): %d bytes: %x\n", len(ret), ret)
		return ret, nil
	}

	// Persist any state changes from the call.
	// IMPORTANT: per go-ethereum docs, sdb is no longer reliable for reads
	// after Commit() — we must open a fresh StateDB on the returned root
	// to safely dump and persist the resulting storage.
	root, commitErr := sdb.Commit(0, false)
	if commitErr != nil {
		fmt.Printf("[EVM] revert Commit failed: %v\n", commitErr)
	} else {
		touchedAddrs, touchedCommitments := extractTouchedEntities(from, data)
		touchedAddrs = append(touchedAddrs, preAddrs...)
		_, _, calldataNullifier := extractTouchedEntitiesWithNullifier(from, data)
		e.dumpAndPersistStorageWithNullifier(root, db, to, touchedAddrs, touchedCommitments, calldataNullifier, releasedNullifiers)
	}
	// P0-3: Go-State is authoritative. Removed syncBalancesFromDB — it overwrote
	// correct Go-state with stale EVM-memory values causing balance divergence.

	fmt.Printf("[EVM] Call result: %d bytes: %x\n", len(ret), ret)
	return ret, nil
}

// dumpAndPersistStorage opens a fresh, read-only StateDB on the given root
// and writes every populated storage slot for addr into PostgreSQL.
// This is the generic, contract-agnostic replacement for guessing slot
// numbers manually — it works correctly for any mapping or simple storage
// variable, regardless of how its slot is computed.
// knownV7Slots lists every storage slot AequitasV7.sol declares, in
// declaration order. Simple slots are plain integers; mapping slots are
// listed by their base slot index and require mappingSlot()/
// mappingSlotBytes32() with a key to compute the actual storage location.
// This is explicit, contract-specific knowledge — not generic — because
// go-ethereum's StateDB offers no reliable generic "what changed" API in
// this version (verified: RawDump does not find accounts after Commit,
// even on the same backing database).
// v7SimpleSlots: single-value slots that are always persisted.
var v7SimpleSlots = []int64{0, 1, 2, 3} // totalSupply, totalHumans, ubiPool, ubiPerHumanAccumulated

// v7AddressMappingSlots: per-address mapping slots (all 13 address mappings in V7).
var v7AddressMappingSlots = []int64{
	4, // balanceOf
	5, // escrowOf
	6, // isHuman
	// 7 = usedCommitments (uint256 key, not address — handled separately)
	// 8 = usedNullifiers  (bytes32 key, not address — handled separately)
	9,  // commitmentOf
	10, // lastActivity
	11, // lastDemurrage
	12, // ubiClaimed
	13, // guardianOf
	14, // pendingGuardian
	15, // guardianRequestedAt
	16, // wardCount
	// Slots 17-26 are CAPS[5]+THRESHOLDS[5] (see v7ArrayBaseSlots below).
	// Added by the pre-launch audit 2026-08-16, declared last in the contract
	// precisely so nothing above renumbers:
	27, // escrowedAt   (RED 2 — anchors the escrow recovery window)
	28, // nullifierOf  (RED 3 — lets a sweep release the biometric record)
}

// v7ArrayBaseSlots: the 10 fixed-size-array slots (CAPS[5] + THRESHOLDS[5]).
var v7ArrayBaseSlots = []int64{17, 18, 19, 20, 21, 22, 23, 24, 25, 26}

// v7SlotsVerifiedForVersion must be bumped by hand alongside v7SimpleSlots/
// v7AddressMappingSlots/v7ArrayBaseSlots whenever AequitasV7.sol's storage
// layout changes (a new state variable added/removed/reordered) — see
// checkV7SlotsMatchDeployedVersion, called at startup, which turns a
// forgotten update here into a loud warning instead of the silent
// data-loss failure mode described above these slot lists: a future
// contract upgrade that adds a 14th address-mapping or 6th array would have
// every write to it succeed in the live EVM call but never persist, with
// nothing surfacing the mismatch until someone eventually notices a field
// that "never saves" (beta-launch audit 2026-07-05).
// v7.11-guardian-sweep-wardcount-guard (audit 2026-07-06, P2-a): verified —
// adds a require() check reading the existing wardCount mapping, no new,
// removed, or reordered state variable, so the slot lists above are unchanged.
// v7.12-tx-fee-bps-zero (audit 2026-07-06, P2-e): verified — TX_FEE_BPS is a
// `constant`, which Solidity inlines at compile time and never gives a
// storage slot at all; changing its value cannot affect the slot lists above.
// v7.13-redundant-sload-cleanup (performance audit 2026-07-06, P3-a):
// verified — transfer()/_applyDemurrage()/_applyWealthCap() now cache
// balanceOf reads into locals instead of re-reading storage, and _calcFee
// was split into _calcFee(sender,amount) + a new internal
// _calcFeeWithBalance(amount,balance) helper; none of that adds, removes,
// or reorders a state variable (CAPS/THRESHOLDS stayed storage arrays —
// solc 0.8.28 doesn't support constant/immutable arrays, see the
// contract's own comment on those two lines), so the slot lists above are
// unchanged.
// v7.14-prelaunch-audit (pre-launch audit 2026-08-16): verified —
// this version DOES add state, the first version to do so since these lists
// were written. Three mappings were appended AFTER THRESHOLDS
// (escrowedAt=27, nullifierOf=28, grantIssuedTo=29), deliberately last so no
// existing slot moved: slots 0-26 are byte-for-byte where they were. The two
// address-keyed ones are listed in v7AddressMappingSlots above;
// grantIssuedTo is keyed by bytes32 like usedNullifiers, so it is persisted
// alongside slot 8 in dumpAndPersistStorageWithNullifier instead.
//
// KNOWN LIMIT, pre-existing and NOT introduced here: extractTouchedEntities
// has an explicit case only for registerWithSig; every other selector falls
// to `default`, which reports only the CALLER as touched. So a third party
// calling triggerEscrowToUBI(human) persists nothing for `human` — including
// RED 3's release of usedNullifiers/nullifierOf. That path already failed to
// persist isHuman/escrowOf/wardCount the same way, so this is one more
// instance of an existing gap rather than a new one; it is written down here
// because RED 3's on-chain half depends on it.
const v7SlotsVerifiedForVersion = "v7.14-prelaunch-audit"

// checkV7SlotsMatchDeployedVersion prints a prominent warning if
// V7ContractVersion has been bumped (contract_deploy.go) without a
// corresponding update to v7SlotsVerifiedForVersion above — see that
// constant's comment. Deliberately does not panic/abort: a mismatch means
// "verify the slot lists are still complete," not "the node cannot run" —
// most version bumps only change function logic, not storage layout, so a
// human still needs to judge whether THIS particular bump added new state.
func checkV7SlotsMatchDeployedVersion() {
	if !v7SlotsVerifiedFor(V7ContractVersion) {
		fmt.Printf("[EVM] ⚠ WARNING: V7ContractVersion (%q) has changed since the storage-slot persistence lists in evm_engine.go were last verified (%q). If this version added, removed, or reordered any state variable in AequitasV7.sol, update v7SimpleSlots/v7AddressMappingSlots/v7ArrayBaseSlots (and v7SlotsVerifiedForVersion) accordingly — otherwise writes to any new slot will silently never persist.\n",
			V7ContractVersion, v7SlotsVerifiedForVersion)
	}
}

// v7SlotsVerifiedFor is checkV7SlotsMatchDeployedVersion's testable
// comparison, split out so a unit test can verify the match/mismatch logic
// itself without needing to capture stdout.
func v7SlotsVerifiedFor(deployedVersion string) bool {
	return deployedVersion == v7SlotsVerifiedForVersion
}

// extractTouchedEntities returns which addresses and commitments a given
// call may have modified, based on an explicit, verified table of byte
// offsets per function selector. This is NOT a heuristic — each offset was
// confirmed against real ABI-encoded calldata before being hardcoded here.
// Add a new case here whenever a new state-changing function is wired up.
// extractTouchedEntitiesWithNullifier extends extractTouchedEntities to also
// return the nullifier (bytes32) used in a registerWithSig call, so it can be
// persisted to usedNullifiers slot 8. Returns nil nullifier for other calls.
func extractTouchedEntitiesWithNullifier(from common.Address, data []byte) ([]common.Address, []*big.Int, *[32]byte) {
	addrs, commits := extractTouchedEntities(from, data)
	if len(data) < 4 {
		return addrs, commits, nil
	}
	sel := fmt.Sprintf("%x", data[:4])
	// ABI layout for registerWithSig(uint256[2],uint256[2][2],uint256[2],uint256[2],address,bytes,bytes32):
	// selector(4) + pA(64) + pB(128) + pC(64) + pubSignals(64) + claimedHuman(32) + sig_offset(32) + nullifier(32)
	// = 4 + 64 + 128 + 64 + 64 + 32 + 32 + 32 = 420 bytes minimum
	if sel == "13b81eb0" && len(data) >= 420 {
		var nullifier [32]byte
		copy(nullifier[:], data[388:420]) // bytes32 nullifier is at offset 388
		return addrs, commits, &nullifier
	}
	return addrs, commits, nil
}

func extractTouchedEntities(from common.Address, data []byte) ([]common.Address, []*big.Int) {
	if len(data) < 4 {
		return []common.Address{from}, nil
	}

	selector := fmt.Sprintf("%x", data[:4])
	switch selector {
	case "13b81eb0": // registerWithSig(uint256[2],uint256[2][2],uint256[2],uint256[2],address,bytes,bytes32)
		// ABI offsets (measured from byte 4, i.e. after selector):
		//
		//	pA(64) + pB(128) + pC(64) + pubSignals(64) = 320 bytes → claimedHuman at 4+320 = 324
		//	pubSignals[0] (commitment) at 4+256 = 260
		addrs := []common.Address{from}
		var commitments []*big.Int
		if len(data) >= 324+32 {
			claimedHuman := common.BytesToAddress(data[324 : 324+32])
			addrs = append(addrs, claimedHuman)
		}
		if len(data) >= 260+32 {
			commitment := new(big.Int).SetBytes(data[260 : 260+32])
			commitments = append(commitments, commitment)
		}
		return addrs, commitments

	// FIX (pre-launch audit 2026-08-16): every one of these used to fall through
	// to `default` below, which reports ONLY the caller. Each of them writes
	// per-address state for an address given in the CALLDATA, not for msg.sender:
	//
	//	transfer(to, …)              balanceOf[to], lastActivity[to], lastDemurrage[to]
	//	triggerEscrow(human)         balanceOf/escrowOf/escrowedAt/lastDemurrage[human]
	//	triggerEscrowToUBI(human)    isHuman/escrowOf/ubiClaimed/nullifierOf[human]
	//	guardianConfirmAlive(ward)   the ward's whole escrow recovery
	//	applyWealthCap(human)        balanceOf[human]
	//	applyDemurrage(human)        balanceOf/lastDemurrage[human]
	//	proposeGuardian(guardian)    pendingGuardian/guardianRequestedAt[msg.sender]
	//
	// Every one of those writes succeeded in the live EVM and was then dropped
	// on the floor, because the address never entered touchedAddrs. The single
	// shared ABI shape makes this cheap: one leading `address` argument occupies
	// bytes 4..36, and common.BytesToAddress takes the low 20 bytes of it.
	case "a9059cbb", // transfer(address,uint256)
		"d3d2770c", // triggerEscrow(address)
		"e54655d2", // triggerEscrowToUBI(address)
		"35a1e72b", // guardianConfirmAlive(address)
		"434c099e", // applyWealthCap(address)
		"e14f2020", // applyDemurrage(address)
		"c304555f": // proposeGuardian(address)
		addrs := []common.Address{from}
		if len(data) >= 36 {
			subject := common.BytesToAddress(data[4:36])
			if subject != (common.Address{}) && subject != from {
				addrs = append(addrs, subject)
			}
		}
		return addrs, nil
	default:
		// Unknown selector: at minimum, the caller's own address may have
		// been touched (e.g. a simple register() or transfer() from msg.sender).
		// Anything reached here that writes state for a THIRD address needs its
		// own case above, and — if that address is only discoverable from
		// storage rather than calldata — an entry in extractPreCallEntities.
		return []common.Address{from}, nil
	}
}

// extractPreCallEntities covers what calldata alone cannot: addresses and
// nullifiers that a call modifies but only NAMES in storage, which means they
// have to be read BEFORE the call runs — afterwards the very writes we want to
// persist have already erased the pointer to them.
//
// FIX (pre-launch audit 2026-08-16). Three cases, all real:
//
//   - triggerEscrowToUBI(human) releases the human's biometric records
//     (usedNullifiers/grantIssuedTo, keyed by the nullifier held in
//     nullifierOf[human]) and decrements wardCount on guardianOf[human].
//     Both pointers are cleared by the call itself.
//   - revokeGuardian() decrements wardCount on the CALLER's guardian, an
//     address that appears nowhere in the calldata.
//   - confirmGuardian() moves the caller between pendingGuardian and
//     guardianOf, changing wardCount on both.
//
// sdb is the pre-call state, which is exactly what this needs. Reads are
// cheap (a handful of GetState calls on slots this package already computes
// elsewhere) and only happen for the three selectors below.
func extractPreCallEntities(sdb *state.StateDB, contract, from common.Address, data []byte) ([]common.Address, [][32]byte) {
	if len(data) < 4 {
		return nil, nil
	}
	readAddr := func(holder common.Address, base int64) (common.Address, bool) {
		v := sdb.GetState(contract, mappingSlot(holder.Bytes(), base))
		a := common.BytesToAddress(v.Bytes())
		return a, a != (common.Address{})
	}

	var addrs []common.Address
	var nullifiers [][32]byte

	switch fmt.Sprintf("%x", data[:4]) {
	case "e54655d2": // triggerEscrowToUBI(address)
		if len(data) < 36 {
			return nil, nil
		}
		human := common.BytesToAddress(data[4:36])
		if g, ok := readAddr(human, 13); ok { // guardianOf[human]
			addrs = append(addrs, g)
		}
		// nullifierOf[human] (slot 28) is the key into usedNullifiers (slot 8)
		// and grantIssuedTo (slot 29). The sweep zeroes the first two; the
		// zeroing is the state change that has to survive, so it is persisted
		// unconditionally rather than only when non-zero.
		if n := sdb.GetState(contract, mappingSlot(human.Bytes(), 28)); n != (common.Hash{}) {
			var key [32]byte
			copy(key[:], n.Bytes())
			nullifiers = append(nullifiers, key)
		}
	case "b44be095": // revokeGuardian()
		if g, ok := readAddr(from, 13); ok {
			addrs = append(addrs, g)
		}
	case "1e0ea61e": // confirmGuardian()
		if g, ok := readAddr(from, 14); ok { // pendingGuardian[msg.sender]
			addrs = append(addrs, g)
		}
		if g, ok := readAddr(from, 13); ok { // the guardian being replaced
			addrs = append(addrs, g)
		}
	}
	return addrs, nullifiers
}

func (e *EVMEngine) dumpAndPersistStorageWithNullifier(root common.Hash, db state.Database, addr common.Address, touchedAddrs []common.Address, touchedCommitments []*big.Int, calldataNullifier *[32]byte, releasedNullifiers [][32]byte) {
	e.dumpAndPersistStorage(root, db, addr, touchedAddrs, touchedCommitments)
	// FIX (pre-launch audit 2026-08-16): nullifiers RELEASED by a sweep must be
	// persisted even though their new value is zero. The guard below deliberately
	// skips zero values — correct for a registration, where a zero means "this
	// call did not touch that slot" — but a release IS the zeroing, so skipping
	// it would leave the old non-zero value in the database and the ban would
	// silently come back on the next restart. These keys were read from the
	// pre-call state precisely because the call erases the pointer to them.
	if len(releasedNullifiers) > 0 {
		addrStr := strings.ToLower(addr.Hex())
		if freshDB, err := state.New(root, db, nil); err == nil {
			for _, n := range releasedNullifiers {
				key := common.BytesToHash(n[:])
				for _, base := range []int64{8, 29} { // usedNullifiers, grantIssuedTo
					slot := mappingSlotBytes32(key, base)
					e.chainState.SaveStorageSlot(addrStr, slot.Hex(), freshDB.GetState(addr, slot).Hex())
				}
			}
		}
	}
	if calldataNullifier != nil {
		addrStr := strings.ToLower(addr.Hex())
		nullKey := common.BytesToHash(calldataNullifier[:])
		if nullKey != (common.Hash{}) {
			freshDB2, err2 := state.New(root, db, nil)
			if err2 == nil {
				nullSlot := mappingSlotBytes32(nullKey, 8)
				val := freshDB2.GetState(addr, nullSlot)
				if val != (common.Hash{}) {
					e.chainState.SaveStorageSlot(addrStr, nullSlot.Hex(), val.Hex())
				}
				// FIX (RED 3, pre-launch audit 2026-08-16): grantIssuedTo (slot 29) is
				// keyed by the same bytes32 nullifier, so it needs the same treatment
				// as slot 8 above — it is not an address mapping and would otherwise
				// never be persisted at all. It is what stops a swept-and-re-registered
				// human from drawing a second INITIAL_GRANT; if it silently failed to
				// persist, the guard would evaporate on the next restart and the
				// re-registration path would mint fresh money every time.
				grantSlot := mappingSlotBytes32(nullKey, 29)
				if gv := freshDB2.GetState(addr, grantSlot); gv != (common.Hash{}) {
					e.chainState.SaveStorageSlot(addrStr, grantSlot.Hex(), gv.Hex())
				}
			}
		}
	}
}

func (e *EVMEngine) dumpAndPersistStorage(root common.Hash, db state.Database, addr common.Address, touchedAddrs []common.Address, touchedCommitments []*big.Int) {
	freshDB, err := state.New(root, db, nil)
	if err != nil {
		fmt.Printf("[EVM] revert Could not open committed state for persistence: %v\n", err)
		return
	}

	addrStr := strings.ToLower(addr.Hex())
	// FIX (performance audit 2026-07-06): this used to call SaveStorageSlot
	// individually for every slot below — up to ~40 round trips per call, on
	// every registration and every intercepted V7 transfer. Collect every
	// (slot, value) pair here and persist them all in ONE round trip via
	// SaveStorageSlots instead.
	slots := make(map[string]string, len(v7SimpleSlots)+len(v7ArrayBaseSlots)+len(touchedAddrs)*len(v7AddressMappingSlots)+len(touchedCommitments))

	for _, slotIdx := range v7SimpleSlots {
		slot := common.BigToHash(big.NewInt(slotIdx))
		slots[slot.Hex()] = freshDB.GetState(addr, slot).Hex()
	}
	// Persist all fixed-size array slots (CAPS[5] + THRESHOLDS[5]).
	for _, slotIdx := range v7ArrayBaseSlots {
		slot := common.BigToHash(big.NewInt(slotIdx))
		slots[slot.Hex()] = freshDB.GetState(addr, slot).Hex()
	}

	for _, touched := range touchedAddrs {
		for _, base := range v7AddressMappingSlots {
			slot := mappingSlot(touched.Bytes(), base)
			slots[slot.Hex()] = freshDB.GetState(addr, slot).Hex()
		}
	}

	for _, commitment := range touchedCommitments {
		slot := mappingSlotBytes32(common.BigToHash(commitment), 7) // usedCommitments (slot 7)
		slots[slot.Hex()] = freshDB.GetState(addr, slot).Hex()
	}
	// SECURITY/PERF (P0, launch audit 2026-07-03): this used to also run a
	// `SELECT nullifier, wallet_address FROM nullifiers` scan here and
	// re-persist a storage slot for EVERY nullifier ever recorded, on every
	// single call to this function -- i.e. on every registration and every
	// intercepted V7 transfer, regardless of whether that call touched a
	// nullifier at all. dumpAndPersistStorage has exactly one caller,
	// dumpAndPersistStorageWithNullifier (above), which already persists the
	// ONE nullifier slot this specific call actually touched (via
	// calldataNullifier) right after calling this function -- so the
	// full-table scan was pure redundant O(N) DB load on every action, growing
	// linearly (and, past a few tens of thousands of registered humans,
	// connection-pool-exhausting) with the total registered population.
	// Removed; the wrapper's targeted persist already covers the only case
	// that matters.
	if len(slots) > 0 {
		if err := e.chainState.SaveStorageSlots(addrStr, slots); err != nil {
			fmt.Printf("[EVM] Warning: could not batch-persist %d storage slots for %s: %v\n", len(slots), addrStr, err)
			return
		}
		fmt.Printf("[EVM] Persisted %d storage slots for %s\n", len(slots), addrStr)
	}
}

// ─── HELPERS ─────────────────────────────────────────────────────────────────

func (e *EVMEngine) GetCode(addr common.Address) []byte {
	code, _ := e.chainState.LoadContract(strings.ToLower(addr.Hex()))
	return code
}

func (e *EVMEngine) SetCode(addr common.Address, code []byte) {
	// No-op: code is always loaded fresh from DB
}

// persistStorageFromDB reads dirty storage slots from stateDB and saves to PostgreSQL
func (e *EVMEngine) persistStorageFromDB(sdb *state.StateDB, addr common.Address) {
	e.PersistContractStorage(addr)
}

// FIX (audit 2026-06-29): syncBalancesFromDB was supposedly already removed
// by P0-3 (see the comment at this function's former only call site,
// elsewhere in this file: "Removed syncBalancesFromDB — it overwrote
// correct Go-state with stale EVM-memory values causing balance
// divergence") — but only that ONE call site was actually deleted. The
// function itself, and ChainState.SetBalance (state.go) which it was the
// only caller of, were both left in place: dead code, but still fully
// reachable to any future caller within this package, with no warning at
// the definition itself (only at the unrelated call site 200 lines away)
// that calling it re-introduces exactly the Go-state-authority violation
// P0-3 was about. Confirmed zero remaining callers of either function
// before deleting both here and in state.go.

func (e *EVMEngine) LoadContractStorage(addr common.Address) {
	// No-op: storage loaded in newStateDB()
}

// decodeRevertReason extracts the human-readable message from EVM revert bytes.
// Solidity require(cond, "message") encodes as: Error(string) selector (0x08c379a0)
// followed by standard ABI-encoded string (offset, length, padded bytes).
// Returns "" if the bytes don't match this standard format (e.g. a panic or
// a require() without a message).
func decodeRevertReason(ret []byte) string {
	if len(ret) < 4 {
		return ""
	}
	// Error(string) selector
	if ret[0] != 0x08 || ret[1] != 0xc3 || ret[2] != 0x79 || ret[3] != 0xa0 {
		return ""
	}
	payload := ret[4:]
	if len(payload) < 64 {
		return ""
	}
	// payload[0:32] = offset (always 0x20 for a single string param)
	// payload[32:64] = string length
	strLen := new(big.Int).SetBytes(payload[32:64]).Uint64()
	if uint64(len(payload)) < 64+strLen {
		return ""
	}
	return string(payload[64 : 64+strLen])
}

func HexToBytecode(hexStr string) ([]byte, error) {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	return hex.DecodeString(hexStr)
}

func min4b(a, b int) int {
	if a < b {
		return a
	}
	return b
}
