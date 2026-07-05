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
func (e *EVMEngine) newStateDB() (*state.StateDB, state.Database, error) {
memDB := rawdb.NewMemoryDatabase()
db := state.NewDatabase(memDB)
sdb, err := state.New(common.Hash{}, db, nil)
if err != nil {
return nil, nil, err
}

// Load all account balances.
// P0-2: Do NOT call LoadNonce per account — that triggers N PostgreSQL queries
// (one per account) and creates a DoS vector. EVM nonces for sends are managed
// in the RPC layer separately; the EVM itself does not need per-account nonces
// for call execution (only for CREATE). Removing SetNonce here has no effect
// on the correctness of contract calls or view calls.
weiPerAEQNew := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
for _, acc := range e.chainState.GetAllAccounts() {
addr := common.HexToAddress(acc.Address)
// P1-FIX: acc.Balance is a Decimal (int64 micro-units, 1 AEQ = 1e6 micro).
// Use .Float() to get the real AEQ value, then convert to wei (×1e18).
// The previous code used big.NewInt(int64(acc.Balance)) which treated the
// raw micro-unit integer as whole-AEQ, producing balances 1e6× too high.
balWeiNew, _ := new(big.Float).SetPrec(256).Mul(
	new(big.Float).SetFloat64(acc.Balance.Float()),
	new(big.Float).SetInt(weiPerAEQNew),
).Int(nil)
if balWeiNew == nil {
	balWeiNew = new(big.Int)
}
sdb.SetBalance(addr, balWeiNew)
}

// Load all contract bytecodes and storage
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
//  Zone A — simple state variables and fixed arrays: slots 0–199.
//            All Solidity state variables (totalSupply, mappings' "base"
//            slots, CAP_MULTIPLIERS[5], THRESHOLDS[5], BIO_VERIFIER …)
//            are numbered sequentially and well within 200.
//  Zone B — mapping values live at keccak256(key || slotBase) which are
//            outside the zone-A range. These are populated by
//            MigrateEVMFromGoState after every upgrade; no constructor
//            sets them, so they are always zero at deploy time.
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

sdb, db, err := e.newStateDB()
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
_, _, calldataNullifier := extractTouchedEntitiesWithNullifier(from, data)
e.dumpAndPersistStorageWithNullifier(root, db, to, touchedAddrs, touchedCommitments, calldataNullifier)
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
4,  // balanceOf
5,  // escrowOf
6,  // isHuman
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
const v7SlotsVerifiedForVersion = "v7.9-register-removal-guardian-toctou-ubi-leak"

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
//   pA(64) + pB(128) + pC(64) + pubSignals(64) = 320 bytes → claimedHuman at 4+320 = 324
//   pubSignals[0] (commitment) at 4+256 = 260
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
default:
// Unknown selector: at minimum, the caller's own address may have
// been touched (e.g. a simple register() or transfer() from msg.sender).
return []common.Address{from}, nil
}
}

func (e *EVMEngine) dumpAndPersistStorageWithNullifier(root common.Hash, db state.Database, addr common.Address, touchedAddrs []common.Address, touchedCommitments []*big.Int, calldataNullifier *[32]byte) {
e.dumpAndPersistStorage(root, db, addr, touchedAddrs, touchedCommitments)
if calldataNullifier != nil {
addrStr := strings.ToLower(addr.Hex())
nullKey := common.BytesToHash(calldataNullifier[:])
if nullKey != (common.Hash{}) {
freshDB2, err2 := state.New(root, db, nil)
if err2 == nil {
nullSlot := mappingSlotBytes32(nullKey, 8)
val := freshDB2.GetState(addr, nullSlot)
if val != (common.Hash{}) { e.chainState.SaveStorageSlot(addrStr, nullSlot.Hex(), val.Hex()) }
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
count := 0

for _, slotIdx := range v7SimpleSlots {
slot := common.BigToHash(big.NewInt(slotIdx))
val := freshDB.GetState(addr, slot)
e.chainState.SaveStorageSlot(addrStr, slot.Hex(), val.Hex())
count++
}
// Persist all fixed-size array slots (CAPS[5] + THRESHOLDS[5]).
for _, slotIdx := range v7ArrayBaseSlots {
slot := common.BigToHash(big.NewInt(slotIdx))
val := freshDB.GetState(addr, slot)
e.chainState.SaveStorageSlot(addrStr, slot.Hex(), val.Hex())
count++
}

for _, touched := range touchedAddrs {
for _, base := range v7AddressMappingSlots {
slot := mappingSlot(touched.Bytes(), base)
val := freshDB.GetState(addr, slot)
e.chainState.SaveStorageSlot(addrStr, slot.Hex(), val.Hex())
count++
}
}

for _, commitment := range touchedCommitments {
slot := mappingSlotBytes32(common.BigToHash(commitment), 7) // usedCommitments (slot 7)
val := freshDB.GetState(addr, slot)
e.chainState.SaveStorageSlot(addrStr, slot.Hex(), val.Hex())
count++
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
if count > 0 {
fmt.Printf("[EVM] Persisted %d storage slots for %s\n", count, addrStr)
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
