package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var registerRateLimit sync.Map

// registerWalletLocks serializes the full registration flow (IsHuman check
// → EVM registerWithSig call → Go-state RegisterHuman) per wallet.
//
// FIX: without this, two near-simultaneous requests for the SAME wallet
// (a double-click, an app retry after a slow response, etc.) could both
// pass the early IsHuman()==false check and both proceed into
// registerOnV7. Each EVM CallContract builds a fresh StateDB from
// Postgres independently, so two concurrent registerWithSig calls can both
// read isHuman[wallet]==false and both succeed on-chain (double-crediting
// totalSupply/totalHumans there) before either commits. Both callers then
// race into state.RegisterHuman(wallet): the first sets
// chain_accounts.is_human=true and succeeds; the second sees it already
// true, returns an "already registered" error that's just logged and
// discarded — leaving the contract's isHuman mapping permanently true
// while chain_accounts.is_human stayed false for that caller. The wallet
// is then stuck forever: every future attempt's EVM dry-run reverts with
// "Already registered" (the EVM mapping never got cleared) while the
// dashboard/API show the wallet as unregistered. Serializing the entire
// flow per wallet closes the race at its root instead of only patching
// the symptom.
var registerWalletLocks sync.Map

// lockWallet returns the mutex for wallet, creating one on first use.
//
// NOTE (audit 2026-06-29, considered and deliberately not "fixed"):
// registerWalletLocks has no cleanup, unlike registerRateLimit's periodic
// sweep below — every distinct wallet that ever attempts registration
// (successful, failed, or retried) leaves a permanent *sync.Mutex entry for
// the lifetime of the process. This is a genuine, slow unbounded-growth
// resource use, not nothing — but periodically deleting old entries the
// way the rate limiter does would be UNSAFE here specifically: a goroutine
// that already loaded a *sync.Mutex reference via LoadOrStore keeps using
// that exact object even after a concurrent cleanup pass deletes the map
// entry, while a NEW caller racing in right after would LoadOrStore a
// DIFFERENT, freshly-allocated mutex — the two callers would then no
// longer exclude each other, silently reintroducing the exact
// double-registration race this map exists to prevent. A correct fix needs
// reference counting (only delete once no goroutine holds the mutex), not
// implemented here given how rarely this matters in practice: growth is
// bounded by the number of distinct wallets that ever attempt registration
// across the chain's entire lifetime, which for a one-time-per-human action
// stays orders of magnitude below the point where a few KB of *sync.Mutex
// overhead per entry would be a real operational concern.
func lockWallet(wallet string) *sync.Mutex {
	v, _ := registerWalletLocks.LoadOrStore(wallet, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func init() {
	// P3-4: periodically clean up expired rate-limit entries to prevent unbounded growth.
	SafeGoroutine("registerRateLimit-cleanup", func() {
		for {
			time.Sleep(60 * time.Second)
			// FIX (P0-3, beta-launch audit 2026-07-05): recover per-iteration — see safeCall's comment.
			SafeCall("registerRateLimit-cleanup-tick", func() {
				now := time.Now()
				registerRateLimit.Range(func(k, v interface{}) bool {
					if now.Sub(v.(time.Time)) > 35*time.Second { // must exceed maximum rate limit window (30s)
						registerRateLimit.Delete(k)
					}
					return true
				})
			})
		}
	})
}

// isPrivateOrLoopback returns true for RFC-1918 private ranges, RFC-6598
// carrier-grade-NAT space, and loopback addresses — used to decide whether
// to trust X-Forwarded-For (only safe when the direct connection comes from
// a known reverse-proxy, not the open internet).
//
// FIX (P0, merge-reliability audit 2026-07-03): 100.64.0.0/10 was missing.
// Confirmed live: Railway's internal edge/load-balancer proxies this node's
// inbound traffic through a rotating pool of addresses in exactly this
// range (100.64.0.2, .3, .5, .7, .9, .10, .12, .13, .15, .16, ... observed
// live). Without it here, clientIP() never trusted X-Forwarded-For for any
// request that arrived through Railway's proxy — which is every external
// request this node ever receives — so every peer's HTTP block push (see
// blockPushRecordOutcome) was attributed to whichever rotating internal
// Railway IP happened to proxy that particular connection instead of the
// peer's real address. A legitimate peer's pushes, split across many
// different apparent "source IPs" that each individually looked like a
// new, low-volume sender, still occasionally tripped the flood shield on
// whichever internal IP happened to accumulate enough of them — dropping
// real, valid block pushes and starving Primary/Contabo's catch-up sync of
// data it needed, worsening the exact orphan-flood/cadence symptoms this
// audit pass was trying to fix elsewhere.
func isPrivateOrLoopback(ipStr string) bool {
	parsed := net.ParseIP(ipStr)
	if parsed == nil {
		return false
	}
	for _, cidr := range []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"100.64.0.0/10", "::1/128", "fc00::/7",
	} {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil && ipnet.Contains(parsed) {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	// Only trust X-Forwarded-For when the TCP connection itself comes from a
	// private/loopback address — i.e. through the trusted platform proxy.
	// A direct internet client must not be able to spoof their IP via this header.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" && isPrivateOrLoopback(host) {
		first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if ip, _, err := net.SplitHostPort(first); err == nil {
			return ip
		}
		if first != "" {
			return first
		}
	}
	return host
}

// V7 ABI fragment for registerWithSig — used only to encode calldata correctly,
// including the dynamic `bytes signature` parameter (offset/length tail encoding).
const registerWithSigABI = `[{
"name": "registerWithSig",
"type": "function",
"inputs": [
{"name": "pA", "type": "uint256[2]"},
{"name": "pB", "type": "uint256[2][2]"},
{"name": "pC", "type": "uint256[2]"},
{"name": "pubSignals", "type": "uint256[2]"},
{"name": "claimedHuman", "type": "address"},
{"name": "signature", "type": "bytes"},
{"name": "nullifier", "type": "bytes32"}
]
}]`

const bioVerifierABI = `[{
"name": "verifyProof",
"type": "function",
"inputs": [
{"name": "pA", "type": "uint256[2]"},
{"name": "pB", "type": "uint256[2][2]"},
{"name": "pC", "type": "uint256[2]"},
{"name": "pubSignals", "type": "uint256[2]"}
],
"outputs": [{"name": "", "type": "bool"}],
"stateMutability": "view"
}]`

type RegisterRequest struct {
	Wallet     string     `json:"wallet"`
	PubSignals []string   `json:"pubSignals"`
	PA         []string   `json:"pA"`
	PB         [][]string `json:"pB"`
	PC         []string   `json:"pC"`
	Signature  string     `json:"signature"` // hex-encoded, 65 bytes, from personal_sign
	// BioHash is the raw bigint biometric hash from the device, used to
	// record the registration in bio_registrations so the app can poll for
	// its own registration by bioHash on startup.
	BioHash string `json:"bioHash"`
	// BioHashKey is the keccak256-hashed version of BioHash, as computed
	// by the proof server. Stored in bio_hashes so the proof server's
	// duplicate check (which also uses keccak256) can detect re-registrations
	// even when the user generates a new wallet on the same device.
	BioHashKey string `json:"bioHashKey"`
	// Nullifier is SHA256(bioHash + ":aequitas-ubi-v1") for v1 circuit, or
	// the hex representation of pubSignals[1] for v2 circuit (ZK-bound).
	Nullifier string `json:"nullifier"`
	// ZKNullifier is pubSignals[1] from the v2 circuit — the nullifier
	// derived INSIDE the ZK proof, making it cryptographically binding.
	// When present, it overrides the client-SHA256 nullifier.
	ZKNullifier    string `json:"zkNullifier"`
	CircuitVersion int    `json:"circuitVersion"`
}

type RegisterResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Balance float64 `json:"balance"`
	TxHash  string  `json:"tx_hash"`
}

func (a *APIServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "POST required"})
		return
	}

	ip := clientIP(r)
	now := time.Now()
	if last, ok := registerRateLimit.Load(ip); ok && now.Sub(last.(time.Time)) < 10*time.Second {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "too many requests — please wait 10 seconds"})
		return
	}
	registerRateLimit.Store(ip, now)

	r.Body = http.MaxBytesReader(w, r.Body, 256<<10) // 256 KB — ZK proofs are large
	body, _ := io.ReadAll(r.Body)
	var req RegisterRequest
	if err := json.Unmarshal(body, &req); err != nil {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "invalid request"})
		return
	}

	wallet := strings.ToLower(req.Wallet)
	if wallet == "" {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "wallet required"})
		return
	}
	// P2-09 (audit): validate format before locking, EVM, or nullifier work.
	// common.HexToAddress tolerates malformed input by silently zeroing bytes;
	// we must reject here so bad inputs never reach consensus state.
	if !isValidWalletAddr(wallet) {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "invalid wallet address format"})
		return
	}

	// Serialize the entire registration flow for this wallet — see
	// registerWalletLocks doc comment for why this is required, not optional.
	walletLock := lockWallet(wallet)
	walletLock.Lock()
	defer walletLock.Unlock()

	if len(req.PA) < 2 || len(req.PB) < 2 || len(req.PC) < 2 || len(req.PubSignals) < 2 {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "incomplete ZK proof"})
		return
	}
	if req.Signature == "" {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "signature required"})
		return
	}

	// Early reject: wallet already registered — saves the expensive EVM call.
	// FIX: message now names the exact layer that blocked, so "already
	// registered" reports from different layers (chain_accounts.is_human vs.
	// nullifiers vs. bio_registrations vs. bio_hashes) are no longer
	// indistinguishable when debugging a stuck registration.
	if a.state.IsHuman(wallet) {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "wallet already registered (chain_accounts.is_human)"})
		return
	}

	fmt.Printf("[REGISTER] Relaying registerWithSig for: %s\n", wallet)

	// Use the shared EVMRPCServer so all parallel registrations share
	// the same nonce map + mutex — prevents two concurrent registrations
	// from reading the same DB-Nonce and writing the same follower value.
	evmRPC := a.evmRPC
	if evmRPC == nil || evmRPC.evm == nil {
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: "EVM engine unavailable"})
		return
	}

	txHash, err := a.registerOnV7(evmRPC, wallet, req)
	if err != nil {
		fmt.Printf("[REGISTER] V7 registerWithSig failed: %v\n", err)
		json.NewEncoder(w).Encode(RegisterResponse{Success: false, Message: err.Error()})
		return
	}

	fmt.Printf("[REGISTER] ✓ Relayed registerWithSig for %s, tx=%s\n", wallet, txHash)
	json.NewEncoder(w).Encode(RegisterResponse{
		Success: true,
		Message: "Registered as human on Aequitas V7! 1,000 AEQ granted.",
		Balance: 1000,
		TxHash:  txHash,
	})
}

// registerOnV7 builds, signs (with the relayer key), and submits a registerWithSig
// transaction on the V7 contract. claimedHuman in the contract call is the user's
// own wallet — verified via the signature, not via msg.sender (which is the relayer).
func (a *APIServer) registerOnV7(evmRPC *EVMRPCServer, wallet string, req RegisterRequest) (string, error) {
	relayerPK := os.Getenv("RELAYER_PRIVATE_KEY")
	if relayerPK == "" {
		return "", fmt.Errorf("server misconfigured: RELAYER_PRIVATE_KEY not set")
	}
	relayerPK = strings.TrimPrefix(relayerPK, "0x")

	relayerKey, err := crypto.HexToECDSA(relayerPK)
	if err != nil {
		return "", fmt.Errorf("invalid relayer key: %w", err)
	}
	relayerAddr := crypto.PubkeyToAddress(relayerKey.PublicKey)

	// Guard: relayer wallet cannot register as human. This prevents the relayer
	// from submitting a ZK proof for a device it controls and self-crediting
	// the 1,000 AEQ registration grant without genuine biometric proof.
	if strings.ToLower(wallet) == strings.ToLower(relayerAddr.Hex()) {
		return "", fmt.Errorf("relayer wallet cannot register as human — use a separate wallet")
	}

	parsedABI, err := abi.JSON(strings.NewReader(registerWithSigABI))
	if err != nil {
		return "", fmt.Errorf("abi parse failed: %w", err)
	}

	pA, err := parseUint2(req.PA)
	if err != nil {
		return "", fmt.Errorf("invalid pA: %w", err)
	}
	pB, err := parseUint2x2(req.PB)
	if err != nil {
		return "", fmt.Errorf("invalid pB: %w", err)
	}
	pC, err := parseUint2(req.PC)
	if err != nil {
		return "", fmt.Errorf("invalid pC: %w", err)
	}
	pubSignals, err := parseUint2(req.PubSignals)
	if err != nil {
		return "", fmt.Errorf("invalid pubSignals: %w", err)
	}

	sigHex := strings.TrimPrefix(req.Signature, "0x")
	sigBytes := common.Hex2Bytes(sigHex)
	if len(sigBytes) != 65 {
		return "", fmt.Errorf("signature must be 65 bytes, got %d", len(sigBytes))
	}

	claimedHuman := common.HexToAddress(wallet)

	// FIX: AequitasV7.sol now REQUIRES the ZK-circuit-bound nullifier
	// (pubSignals[1] != 0) — the old fallback that trusted a caller-supplied
	// nullifier whenever a v1-circuit proof omitted it was removed because it
	// had zero cryptographic binding to the proof. Reject v1 requests here,
	// before spending a relayer dry-run, with a clear message instead of
	// letting them fail later on the contract's own (correct) revert.
	// FIX (P2-01): require exactly v3 (Poseidon nullifier circuit). Accepting
	// any version >= 2 allowed a downgrade to the old SHA256-based v2 circuit,
	// which has a weaker nullifier construction than the Poseidon-based v3.
	if req.CircuitVersion != 3 || req.ZKNullifier == "" {
		return "", fmt.Errorf("only circuit version 3 is accepted: upgrade your app — v3 Poseidon nullifier is required")
	}

	// Prefer ZK-circuit-derived nullifier (v2 circuit, pubSignals[1]) over
	// client-SHA256 nullifier. When the v2 circuit is used, the nullifier is
	// cryptographically attested by the Groth16 proof itself.
	effectiveNullifier := req.Nullifier
	if req.ZKNullifier != "" && req.CircuitVersion >= 2 {
		effectiveNullifier = req.ZKNullifier
		fmt.Printf("[REGISTER] Using ZK-bound nullifier (circuit v%d)\n", req.CircuitVersion)
	}
	// Nullifier is mandatory — reject registrations without one.
	if effectiveNullifier == "" {
		return "", fmt.Errorf("nullifier required")
	}
	// FIX (P0-02): use IsNullifierUsed (EXISTS check) rather than
	// GetWalletByNullifier != "" — public snapshots store nullifiers with
	// wallet_address="" and GetWalletByNullifier returns "" for those rows,
	// making the old check conclude the nullifier is free when it's not.
	if a.state.IsNullifierUsed(effectiveNullifier) {
		existingWallet := a.state.GetWalletByNullifier(effectiveNullifier)
		if existingWallet == "" {
			existingWallet = "another wallet (public snapshot — wallet address redacted)"
		}
		return "", fmt.Errorf("identity already registered: nullifier used by %s (nullifiers table)", existingWallet)
	}
	// NOTE: the v1-circuit BioHash/SHA256-nullifier-derivation checks that
	// used to live here are gone along with v1 support (see the early
	// CircuitVersion < 2 rejection above) — every request reaching this point
	// already has a v2 ZK-bound nullifier. bioHash, if supplied, is still
	// checked for duplicates below (secondary defense-in-depth dedup layer,
	// independent of the nullifier).
	// FIX: BioHashKey used to be trusted as-is from the client/proof-server
	// response, with nothing on the chain verifying it actually equals
	// keccak256(BioHash). The nullifier remains the real on-chain uniqueness
	// guarantee (the EVM contract enforces that), so this isn't a way to
	// double-register — but a buggy or manipulated client could submit a
	// BioHashKey that doesn't correspond to its own BioHash, desyncing the
	// chain's bio_hashes bookkeeping from the proof-server's (which always
	// computes the key itself, never trusts a caller-supplied one) and
	// making duplicate-biometric diagnostics unreliable. Recompute and
	// compare whenever both are present; reject on mismatch rather than
	// silently trusting whichever one the client decided to claim.
	if req.BioHash != "" && req.BioHashKey != "" {
		expectedKey, keyErr := computeBioHashKeyFromBioHash(req.BioHash)
		if keyErr == nil && !strings.EqualFold(expectedKey, req.BioHashKey) {
			return "", fmt.Errorf("bioHashKey does not match keccak256(bioHash)")
		}
	}
	// FIX: GetWalletByBioHash only checks bio_registrations. The chain also
	// maintains its own, separate bio_hashes table (see SaveBioHash) — check
	// that too, using whichever key SaveBioHash would have used (BioHashKey
	// preferred, BioHash fallback), so the two tables can't silently
	// disagree about whether a biometric is already claimed.
	storedBioHashKey := req.BioHashKey
	if storedBioHashKey == "" {
		storedBioHashKey = req.BioHash
	}
	if storedBioHashKey != "" {
		if existingWallet := a.state.GetWalletByStoredBioHash(storedBioHashKey); existingWallet != "" {
			return "", fmt.Errorf("biometric already registered to %s (chain bio_hashes table)", existingWallet)
		}
	}
	if req.BioHash != "" {
		if existingWallet := a.state.GetWalletByBioHash(req.BioHash); existingWallet != "" {
			return "", fmt.Errorf("biometric already registered to %s (chain bio_registrations table)", existingWallet)
		}
	}

	// Encode nullifier as bytes32 (big-endian uint256).
	// v1 nullifiers are SHA256 hex strings ("0xabc..." or "abc...").
	// v2 nullifiers are pubSignals[1] — a decimal integer string like "17579322874185".
	// Both must be encoded as big-endian 32-byte integers for the contract.
	var nullifierBytes [32]byte
	if effectiveNullifier != "" {
		n := new(big.Int)
		if strings.HasPrefix(effectiveNullifier, "0x") || strings.HasPrefix(effectiveNullifier, "0X") {
			// Explicit hex prefix → always parse as hex (v1 circuit SHA256 output)
			n.SetString(strings.TrimPrefix(strings.TrimPrefix(effectiveNullifier, "0x"), "0X"), 16)
		} else if req.CircuitVersion >= 2 {
			// v2: decimal string (ZK-bound pubSignals[1])
			if _, ok := n.SetString(effectiveNullifier, 10); !ok {
				n.SetString(effectiveNullifier, 16)
			}
		} else {
			// v1 without 0x prefix: SHA256 output is always hex
			n.SetString(effectiveNullifier, 16)
		}
		b := n.Bytes()
		if len(b) <= 32 {
			copy(nullifierBytes[32-len(b):], b) // right-align (big-endian)
		}
	}

	calldata, err := parsedABI.Pack("registerWithSig", pA, pB, pC, pubSignals, claimedHuman, sigBytes, nullifierBytes)
	if err != nil {
		return "", fmt.Errorf("encoding failed: %w", err)
	}

	to := common.HexToAddress(V7_CONTRACT_ADDR)

	// Dry-run first: simulate the call before spending a real transaction/nonce.
	// If the contract would revert (invalid signature, invalid proof, already
	// registered, etc.), CallContract returns an error and we abort here with
	// the real reason — instead of reporting false success to the caller.
	// persist=false is critical here: this is ONLY a simulation. Previously
	// CallContract always persisted its result regardless of intent, which
	// meant this dry-run alone — even for an attempt that was never actually
	// submitted, or whose real submission later failed — already wrote
	// isHuman=true/balance=1000 to evm_storage as a side effect of merely
	// checking whether registration would succeed.
	_, dryRunErr := evmRPC.evm.CallContract(relayerAddr, to, calldata, big.NewInt(0), false)
	if dryRunErr != nil {
		errStr := strings.ToLower(dryRunErr.Error())
		contractMissing := strings.Contains(errStr, "no code") ||
			strings.Contains(errStr, "empty code") ||
			strings.Contains(errStr, "contract not deployed")
		if !contractMissing {
			// Proof invalid, already registered, bad signature, etc. — surface
			// the real EVM revert reason instead of silently bypassing via mirror.
			return "", fmt.Errorf("registration rejected: %w", dryRunErr)
		}
		// V7 not yet deployed (startup race). Mirror validates the proof via
		// BioVerifier and writes storage slots directly. Only allowed here.
		//
		// FIX (Gesamtaudit 2026-06-28, P0-2): this fallback writes EVM mirror
		// slots via persistRegisterWithSigMirror using compensating writes
		// (load old values, write new ones, write old ones back on failure)
		// instead of a real DB transaction — it can't easily become one,
		// since it doesn't run inside any cs.mu-held critical section the
		// way the registration's Go-state side does (see SaveStorageSlot's
		// own comment on why routing it through cs.dbExec() without holding
		// cs.mu would itself be a race). If a revert-write in that
		// compensation chain ever fails too, the EVM mirror is left
		// half-written with no automatic recovery. This path is normally
		// dead in production (V7 stays deployed once auto-deployed at
		// startup) — gating it behind an explicit opt-in means it can never
		// silently fire and create that half-written state; an operator who
		// actually hits the V7-missing startup race has to deliberately
		// acknowledge the risk first.
		if os.Getenv("ALLOW_V7_MIRROR_FALLBACK") != "true" {
			return "", fmt.Errorf("registration rejected: V7 contract not yet deployed on this node and ALLOW_V7_MIRROR_FALLBACK is not set — wait for V7 auto-deploy to finish, or set ALLOW_V7_MIRROR_FALLBACK=true to use the (less safe) mirror registration fallback: %w", dryRunErr)
		}
		fmt.Printf("[REGISTER] V7 not yet deployed — using mirror registration for %s\n", wallet)
		// FIX (TOCTOU): claim the nullifier atomically BEFORE writing any mirror
		// storage slots, not after. persistRegisterWithSigMirror's own internal
		// "already used" checks (LoadStorageSlot reads) are plain reads with no
		// locking — two concurrent requests sharing the same nullifier could
		// both pass those checks and both write totalSupply/totalHumans/balance
		// slots before the old post-hoc TryClaimNullifier call ever ran, double-
		// crediting the registration grant. TryClaimNullifier's DB-level
		// INSERT...ON CONFLICT (or mutex-guarded map) is the only atomic
		// primitive here, so only the winner may proceed to mutate storage.
		claimed, claimErr := a.state.TryClaimNullifier(effectiveNullifier, wallet)
		if claimErr != nil {
			return "", fmt.Errorf("nullifier claim failed: %w", claimErr)
		}
		if !claimed {
			return "", fmt.Errorf("nullifier already claimed: %s", effectiveNullifier)
		}
		// FIX (audit recheck 2, P1 #8/#11): pendingRegTx used to be built
		// AFTER persistRegisterWithSigMirror returned, then persisted via a
		// separate SavePendingTx call — non-atomic with the Go-state mutation
		// persistRegisterWithSigMirror did internally. A failure in that
		// separate SavePendingTx call (or a crash between the two) left the
		// registration fully applied locally with no secondary ever learning
		// about it, exactly the gap RegisterHumanAtomic already closed for
		// the non-mirror path. Build it BEFORE the call instead and pass it
		// in — persistRegisterWithSigMirror now calls RegisterHumanAtomic
		// instead of RegisterHuman, so the Go-state mutation and the outbox
		// insert commit or roll back together as one DB transaction, same as
		// the non-mirror path.
		mirrorCommitment := ""
		if len(req.PubSignals) > 0 {
			mirrorCommitment = req.PubSignals[0]
		}
		var pendingRegTx Transaction
		if effectiveNullifier != "" {
			mirrorPAslice := []*big.Int{pA[0], pA[1]}
			mirrorPCslice := []*big.Int{pC[0], pC[1]}
			mirrorPSslice := []*big.Int{pubSignals[0], pubSignals[1]}
			pendingRegTx = Transaction{
				Type:       "register_human",
				Wallet:     wallet,
				TxHash:     crypto.Keccak256Hash(append(calldata, claimedHuman.Bytes()...)).Hex(),
				Nullifier:  effectiveNullifier,
				Commitment: mirrorCommitment,
				ProofA:     bigIntsToHexStrings(mirrorPAslice),
				ProofB:     bigInt2x2ToHexStrings(pB),
				ProofC:     bigIntsToHexStrings(mirrorPCslice),
				PubSignals: bigIntsToHexStrings(mirrorPSslice),
			}
		}
		txHash, mirrorErr := a.persistRegisterWithSigMirror(evmRPC, to, claimedHuman, pA, pB, pC, pubSignals, sigBytes, calldata, effectiveNullifier, pendingRegTx)
		if mirrorErr != nil {
			// Registration didn't actually happen — release the claim so the
			// legitimate owner of this nullifier isn't permanently locked out.
			a.state.ReleaseNullifier(effectiveNullifier)
			return "", fmt.Errorf("registration failed (V7 missing + mirror failed): %w; mirror: %v", dryRunErr, mirrorErr)
		}
		if len(req.PubSignals) > 0 {
			commitment := req.PubSignals[0]
			if saveErr := a.state.SaveBioRegistration(commitment, wallet, txHash, req.BioHash); saveErr != nil {
				fmt.Printf("[REGISTER] Warning: could not save bio registration link: %v\n", saveErr)
			}
		}
		// FIX (audit 2026-06-28 recheck 4, P1-5): this mirror path saved
		// bio_hashes locally but never notified the proof server at all —
		// unlike the main path below, which does both. Bring it in line:
		// check SaveBioHash's error instead of discarding it, and queue
		// the proof-server notify the same way.
		mirrorBioHashKey := req.BioHashKey
		if mirrorBioHashKey == "" {
			mirrorBioHashKey = req.BioHash
		}
		if mirrorBioHashKey != "" {
			if err := a.state.SaveBioHash(mirrorBioHashKey, wallet); err != nil {
				fmt.Printf("[REGISTER] Warning: local bio_hashes sync failed for %s: %v\n", wallet, err)
			}
			SafeGoroutine("notifyProofServerWithRetryQueue", func() { notifyProofServerWithRetryQueue(a.state, mirrorBioHashKey, wallet) })
		}
		return txHash, nil
	}

	relayerAddrStr := strings.ToLower(relayerAddr.Hex())
	nonce := a.state.LoadNonce(relayerAddrStr)

	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    big.NewInt(0),
		Gas:      6_000_000,
		GasPrice: big.NewInt(0),
		Data:     calldata,
	})

	signer := types.NewEIP155Signer(big.NewInt(1926))
	signedTx, err := types.SignTx(tx, signer, relayerKey)
	if err != nil {
		return "", fmt.Errorf("relayer signing failed: %w", err)
	}

	rawBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("tx encoding failed: %w", err)
	}

	rawHex := "0x" + common.Bytes2Hex(rawBytes)
	// Layer 2: actually submit. sendRawTransaction itself now returns a real
	// RPCError if the on-chain execution reverts (see evm_rpc.go), so a
	// non-nil rpcErr here already means the registration genuinely failed —
	// not just that submission failed.
	// ── NATIVE BALANCE (Phase 1 of native-coin migration) ────────────────
	// Always save effectiveNullifier (the ZK-derived one when using v2 circuit),
	// not just req.Nullifier — req.Nullifier may be empty or the old SHA256 value
	// while effectiveNullifier is the one actually stored on-chain. Computed
	// here (before RegisterHuman, not after) so the pendingTx below can be
	// built completely BEFORE the atomic call that needs it.
	nullifierToStore := effectiveNullifier
	if nullifierToStore == "" {
		nullifierToStore = req.Nullifier
	}
	// A missing nullifier means no outbox TX can be created — secondary nodes
	// would never learn about this registration and balances would diverge.
	// Reject before submitting the EVM tx so we never reach the EVM-committed-
	// but-Go-state-failed half-state that requires manual recovery.
	if nullifierToStore == "" {
		return "", fmt.Errorf("registration rejected: nullifier missing from proof — re-submit with a valid V3 circuit proof")
	}
	commitment := ""
	if len(req.PubSignals) > 0 {
		commitment = req.PubSignals[0]
	}

	// Build pendingRegTx before EVM — TxHash is "" initially, filled in after
	// EVM confirms.  Building it here lets us store the full proof data in the
	// intent record so background recovery can reconstruct the outbox TX even if
	// the process crashes between EVM commit and RegisterHumanAtomic.
	pAslice := []*big.Int{pA[0], pA[1]}
	pCslice := []*big.Int{pC[0], pC[1]}
	psSlice := []*big.Int{pubSignals[0], pubSignals[1]}
	pendingRegTx := Transaction{
		Type:       "register_human",
		Wallet:     wallet,
		Nullifier:  nullifierToStore,
		Commitment: commitment,
		ProofA:     bigIntsToHexStrings(pAslice),
		ProofB:     bigInt2x2ToHexStrings(pB),
		ProofC:     bigIntsToHexStrings(pCslice),
		PubSignals: bigIntsToHexStrings(psSlice),
	}

	// ── REGISTRATION STATE MACHINE (audit P1-02) ─────────────────────────
	// Step 1: Write a durable intent BEFORE submitting the EVM tx.
	// If the process crashes after EVM commits but before SaveRegistrationRecovery
	// is called, the intent is already there for background retry.
	intentID, intentErr := a.state.SaveRegistrationIntent(wallet, nullifierToStore, pendingRegTx)
	if intentErr != nil {
		Log.Error("Could not write registration intent — proceeding without durable pre-EVM record", "err", intentErr)
	}

	// Step 2: Submit the EVM tx.
	result, rpcErr := evmRPC.sendRawTransaction([]json.RawMessage{
		mustMarshal(rawHex),
	})
	if rpcErr != nil {
		if intentID > 0 {
			a.state.DeleteRegistrationIntent(intentID) // EVM never happened — clean up intent
		}
		return "", fmt.Errorf("registration failed on-chain: %s", rpcErr.Message)
	}

	txHash, ok := result.(string)
	if !ok {
		if intentID > 0 {
			a.state.DeleteRegistrationIntent(intentID)
		}
		return "", fmt.Errorf("unexpected response from relay")
	}

	// Step 3: Stamp the confirmed EVM tx hash on the intent so background retry
	// knows the EVM part is done and only Go-state needs to be retried.
	pendingRegTx.TxHash = txHash
	if intentID > 0 {
		a.state.UpdateRegistrationIntentEVMTxHash(intentID, txHash)
	}

	// Step 4: Sync Go-state + outbox atomically.
	registered := false
	var regErr error
	for retry := 1; retry <= 3; retry++ {
		regErr = a.state.RegisterHumanAtomic(wallet, pendingRegTx)
		if regErr == nil {
			registered = true
			break
		}
		if retry < 3 {
			time.Sleep(time.Duration(retry) * 500 * time.Millisecond)
		}
	}
	if !registered {
		// Step 5 (failure path): the intent already exists in registration_recovery
		// with evm_tx_hash set — background RetryRegistrationRecoveries will keep
		// retrying RegisterHumanAtomic until Go-state catches up.
		Log.Error("CRITICAL: RegisterHumanAtomic failed 3x after EVM success — intent left for background retry",
			"wallet", wallet, "txHash", txHash, "intentID", intentID, "error", regErr)
		a.state.SetBootstrapDegraded(fmt.Sprintf(
			"registration_recovery: EVM tx %s registered %s on-chain but Go-state sync failed (%v) — "+
				"background recovery is retrying; check /api/admin/registration-recovery",
			txHash, wallet, regErr))
		return "", fmt.Errorf("registration succeeded on-chain (tx %s) but failed to sync locally after retries: %w — "+
			"recovery is queued, check /api/admin/registration-recovery?wallet=%s", txHash, regErr, wallet)
	}

	// Step 5 (success path): mark the intent closed so it doesn't get retried.
	if intentID > 0 {
		a.state.MarkRegistrationIntentRecovered(intentID)
	}
	a.state.SyncBalancesToEVM(V7_CONTRACT_ADDR, wallet)

	// Record which wallet this proof's commitment actually registered to,
	// so the app can later ask "did MY proof get registered, and where?"
	// instead of reading the last entry in a global, unfiltered list.
	// Deliberately NOT part of the atomic scope above: non-consensus side
	// bookkeeping (doesn't affect StateRoot), same reasoning as block.go's
	// replay-rollback excluding bio_registrations/bio_hashes.
	if len(req.PubSignals) > 0 {
		if saveErr := a.state.SaveBioRegistration(commitment, wallet, txHash, req.BioHash); saveErr != nil {
			fmt.Printf("[REGISTER] Warning: could not save bio registration link: %v\n", saveErr)
		}
	}

	// Keep bio_hashes in sync so the proof server's /check and /prove
	// endpoints can block duplicate biometric registrations via that table.
	// Use the keccak256 bioHashKey when available — it matches the format
	// the proof server uses for its own duplicate checks.
	bioHashKey := req.BioHashKey
	if bioHashKey == "" {
		bioHashKey = req.BioHash
	}
	if bioHashKey != "" {
		if err := a.state.SaveBioHash(bioHashKey, wallet); err != nil {
			fmt.Printf("[REGISTER] Warning: local bio_hashes sync failed for %s: %v\n", wallet, err)
		}
		// Non-blocking — a slow proof server must not delay the
		// registration response. No longer pure fire-and-forget: a
		// failure is queued for retry instead of silently lost. See
		// notifyProofServerWithRetryQueue's comment.
		SafeGoroutine("notifyProofServerWithRetryQueue", func() { notifyProofServerWithRetryQueue(a.state, bioHashKey, wallet) })
	}
	// FIX (audit recheck 2, P1 #7/#10): SaveNullifier used to be called
	// here, as a separate, non-atomic step AFTER RegisterHumanAtomic's
	// transaction had already committed — a failure here left this node's
	// own nullifiers bookkeeping permanently missing this entry despite
	// the registration itself having succeeded (StateRoot hashes the
	// sorted set of nullifier keys, so this caused a permanent mismatch
	// against any secondary that correctly replayed the same registration).
	// RegisterHumanAtomic now claims the nullifier itself, inside the same
	// DB transaction as the account mutation and the outbox insert — see
	// its comment. Nothing left to do here.

	return txHash, nil
}

// computeBioHashKeyFromBioHash replicates the proof server's
// computeBioHashKey(bioNum): keccak256(0x + bioNum left-padded to 32 bytes),
// where bioNum is the raw decimal biometric value. Used to verify a
// client-supplied BioHashKey actually corresponds to its BioHash instead of
// trusting it unconditionally.
func computeBioHashKeyFromBioHash(bioHash string) (string, error) {
	bioNum, ok := new(big.Int).SetString(strings.TrimSpace(bioHash), 10)
	if !ok {
		return "", fmt.Errorf("bioHash is not a valid decimal integer")
	}
	return crypto.Keccak256Hash(common.LeftPadBytes(bioNum.Bytes(), 32)).Hex(), nil
}

// notifyProofServer POSTs the registered bioHashKey to the proof server's
// /store-bio endpoint so its duplicate check stays in sync with the chain.
// Requires PROOF_SERVER_URL and CHAIN_SERVICE_TOKEN env vars on the chain node;
// if either is missing the call is skipped (registration already succeeded;
// attempted=false tells the caller this wasn't a failure worth retrying —
// a node with no proof server configured at all would otherwise queue
// retries that can never succeed).
func notifyProofServer(bioHashKey, wallet string) (attempted bool, err error) {
	// FIX (audit recheck2, P2 #1): used to fall back to this project's own
	// original Railway URL when PROOF_SERVER_URL was unset — see api.go's
	// proofServerBaseURL comment for why that's backwards for a
	// decentralized-operator project. If unset, skip exactly like the
	// existing "missing CHAIN_SERVICE_TOKEN" skip below already does
	// (registration already succeeded; this notify is best-effort sync).
	proofServerURL := os.Getenv("PROOF_SERVER_URL")
	if proofServerURL == "" {
		return false, nil
	}
	token := os.Getenv("CHAIN_SERVICE_TOKEN")
	if token == "" {
		return false, nil
	}
	body, _ := json.Marshal(map[string]string{"bioHashKey": bioHashKey, "wallet": wallet})
	req, reqErr := http.NewRequest("POST", proofServerURL+"/store-bio", bytes.NewReader(body))
	if reqErr != nil {
		return true, fmt.Errorf("could not build proof-server notify request: %w", reqErr)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-chain-token", token)
	// FIX 2: use httpSyncClient (pinningDialer + redirect blocking) instead of
	// a bare http.Client, preventing SSRF via PROOF_SERVER_URL redirect chains
	// or DNS-rebinding to internal/cloud-metadata addresses.
	resp, reqErr := httpSyncClient.Do(req)
	if reqErr != nil {
		return true, fmt.Errorf("proof-server /store-bio call failed: %w", reqErr)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return true, fmt.Errorf("proof-server /store-bio returned %d", resp.StatusCode)
	}
	return true, nil
}

// notifyProofServerWithRetryQueue calls notifyProofServer and, on a genuine
// failure (not a deliberate skip), persists the attempt to
// proof_server_sync_queue so RetryProofServerSyncQueue can catch up later.
//
// FIX (audit 2026-06-28 recheck 4, P1-5; carried over from audit3 P0-1):
// this used to be entirely fire-and-forget with no durable record of a
// failed sync — see proof_server_sync_queue's table comment (state.go) for
// the operational-divergence risk this closed.
func notifyProofServerWithRetryQueue(cs *ChainState, bioHashKey, wallet string) {
	attempted, err := notifyProofServer(bioHashKey, wallet)
	if !attempted {
		return
	}
	if err != nil {
		fmt.Printf("[REGISTER] Warning: proof-server sync failed, queued for retry: %v\n", err)
		cs.QueueProofServerSync(bioHashKey, wallet, err.Error())
		return
	}
	// Succeeded — in case this wallet had a stale queue entry from an
	// earlier failed attempt, clear it now.
	cs.RemoveFromProofServerSyncQueue(bioHashKey)
}

// RetryProofServerSyncQueue attempts every pending proof_server_sync_queue
// entry once. Intended to be called periodically (see NewAPIServer's
// startup goroutine) so a transient proof-server outage doesn't leave a
// permanent gap — see proof_server_sync_queue's table comment.
func RetryProofServerSyncQueue(cs *ChainState) {
	for _, entry := range cs.LoadProofServerSyncQueue() {
		attempted, err := notifyProofServer(entry.BioHashKey, entry.Wallet)
		if !attempted {
			// No proof server configured on this node at all — nothing to
			// retry against; leave the queue alone rather than spin on it.
			return
		}
		if err != nil {
			cs.QueueProofServerSync(entry.BioHashKey, entry.Wallet, err.Error())
			continue
		}
		cs.RemoveFromProofServerSyncQueue(entry.BioHashKey)
	}
}

func (a *APIServer) persistRegisterWithSigMirror(evmRPC *EVMRPCServer, contractAddr, claimedHuman common.Address, pA [2]*big.Int, pB [2][2]*big.Int, pC [2]*big.Int, pubSignals [2]*big.Int, sigBytes []byte, calldata []byte, nullifierHex string, pendingTx Transaction) (string, error) {
	if nullifierHex == "" {
		return "", fmt.Errorf("nullifier required")
	}
	// Mirror the V7 contract's binding check: for circuit v2, pubSignals[1]
	// IS the nullifier output. If the caller's nullifierHex doesn't match
	// pubSignals[1] the proof and the nullifier are from different sessions,
	// which would break the "one human, one nullifier" invariant.
	if pubSignals[1] != nil && pubSignals[1].Sign() > 0 {
		// Compare numerically — effectiveNullifier for v2 circuit is a decimal
		// string ("17579322874185"), not hex. String comparison would always fail.
		providedBig := new(big.Int)
		s := strings.TrimPrefix(nullifierHex, "0x")
		if nullifierHex != s {
			providedBig.SetString(s, 16) // had 0x prefix → hex
		} else if _, ok := providedBig.SetString(s, 10); !ok {
			providedBig.SetString(s, 16) // no prefix → try decimal, then hex
		}
		if providedBig.Cmp(pubSignals[1]) != 0 {
			return "", fmt.Errorf("nullifier mismatch: provided %s != circuit output %s", nullifierHex, pubSignals[1].Text(10))
		}
	}
	wallet := strings.ToLower(claimedHuman.Hex())
	if a.state.IsHuman(wallet) {
		return "", fmt.Errorf("already registered")
	}

	commitment := pubSignals[0]
	addrStr := strings.ToLower(contractAddr.Hex())
	usedSlot := mappingSlotBytes32(common.BigToHash(commitment), 7)
	used, err := a.state.LoadStorageSlot(addrStr, usedSlot.Hex())
	if err != nil {
		return "", err
	}
	if common.HexToHash(used) != (common.Hash{}) {
		return "", fmt.Errorf("commitment used")
	}

	// Check nullifier slot (slot 8) for replay protection.
	// Parse nullifierHex correctly: v2 nullifiers are decimal strings, not hex.
	nullBigCheck := new(big.Int)
	if strings.HasPrefix(nullifierHex, "0x") || strings.HasPrefix(nullifierHex, "0X") {
		nullBigCheck.SetString(strings.TrimPrefix(strings.TrimPrefix(nullifierHex, "0x"), "0X"), 16)
	} else {
		if _, ok := nullBigCheck.SetString(nullifierHex, 10); !ok {
			nullBigCheck.SetString(nullifierHex, 16)
		}
	}
	nullKey := common.BigToHash(nullBigCheck)
	nullSlotCheck := mappingSlotBytes32(nullKey, 8)
	nullVal, _ := a.state.LoadStorageSlot(addrStr, nullSlotCheck.Hex())
	if common.HexToHash(nullVal) != (common.Hash{}) {
		return "", fmt.Errorf("nullifier already used")
	}

	if !validRegisterSignature(contractAddr, claimedHuman, commitment, nullifierHex, sigBytes) {
		return "", fmt.Errorf("invalid signature")
	}

	verifierABI, err := abi.JSON(strings.NewReader(bioVerifierABI))
	if err != nil {
		return "", err
	}
	verifyData, err := verifierABI.Pack("verifyProof", pA, pB, pC, pubSignals)
	if err != nil {
		return "", err
	}
	ret, err := evmRPC.evm.CallContract(claimedHuman, common.HexToAddress(BIO_VERIFIER_ADDR), verifyData, big.NewInt(0), false)
	if err != nil {
		return "", fmt.Errorf("invalid proof: %w", err)
	}
	if len(ret) != 32 || ret[31] != 1 {
		return "", fmt.Errorf("invalid proof")
	}

	initialGrant := new(big.Int).Mul(big.NewInt(1000), weiPerAEQ)
	totalSupply := loadSlotBig(a.state, addrStr, 0)
	totalHumans := loadSlotBig(a.state, addrStr, 1)
	ubiAccumulated := loadSlotBig(a.state, addrStr, 3)
	now := big.NewInt(time.Now().Unix())

	// usedNullifiers[nullifier] = claimedHuman — slot 8 (bytes32 → address mapping)
	nullSlot := mappingSlotBytes32(nullKey, 8)
	addrVal := common.BigToHash(claimedHuman.Big())

	// FIX (audit recheck3, P1 — "Mirror schreibt EVM-Slots ohne
	// Fehlerauswertung"): SaveStorageSlot already returns an error, but
	// every call below discarded it — a write failing partway through
	// (e.g. a transient DB error on the 5th of 9 slots) left some slots
	// updated and others not, then proceeded to RegisterHumanAtomic
	// anyway with that half-written mirror, no error surfaced anywhere.
	// writeSlot checks every write and, on the first failure, reverts
	// every slot already written in THIS call (to its pre-mirror value,
	// captured in oldValues before any write happens) before returning —
	// the same "leave nothing partial" guarantee the existing
	// RegisterHumanAtomic-failure rollback below already gives the
	// Go-state side, now extended to cover this function's OWN writes too.
	slots := []struct{ slot, value string }{
		{common.BigToHash(big.NewInt(0)).Hex(), common.BigToHash(new(big.Int).Add(totalSupply, initialGrant)).Hex()},
		{common.BigToHash(big.NewInt(1)).Hex(), common.BigToHash(new(big.Int).Add(totalHumans, big.NewInt(1))).Hex()},
		{mappingSlot(claimedHuman.Bytes(), 4).Hex(), common.BigToHash(initialGrant).Hex()},
		{mappingSlot(claimedHuman.Bytes(), 6).Hex(), common.HexToHash("0x01").Hex()},
		{usedSlot.Hex(), common.HexToHash("0x01").Hex()},
		{mappingSlot(claimedHuman.Bytes(), 9).Hex(), common.BigToHash(commitment).Hex()},
		{mappingSlot(claimedHuman.Bytes(), 10).Hex(), common.BigToHash(now).Hex()},
		{mappingSlot(claimedHuman.Bytes(), 11).Hex(), common.BigToHash(now).Hex()},
		{mappingSlot(claimedHuman.Bytes(), 12).Hex(), common.BigToHash(ubiAccumulated).Hex()},
		{nullSlot.Hex(), addrVal.Hex()},
	}
	oldValues := make([]string, len(slots))
	for i, s := range slots {
		oldValues[i], _ = a.state.LoadStorageSlot(addrStr, s.slot) // "" (zero value) if unset, same as a fresh slot
	}
	for i, s := range slots {
		if err := a.state.SaveStorageSlot(addrStr, s.slot, s.value); err != nil {
			for j := 0; j < i; j++ {
				if revertErr := a.state.SaveStorageSlot(addrStr, slots[j].slot, oldValues[j]); revertErr != nil {
					fmt.Printf("[REGISTER] CRITICAL: mirror slot write failed AND revert of slot %d also failed for %s — EVM mirror may be inconsistent: %v\n", j, wallet, revertErr)
				}
			}
			return "", fmt.Errorf("mirror EVM slot write failed (reverted %d already-written slot(s)): %w", i, err)
		}
	}

	// FIX: RegisterHuman (the Go-state, authoritative side) used to be called
	// separately by the caller, with a 3-retry loop and a "contact support"
	// comment if all 3 failed. That left the wallet permanently EVM-mirror-
	// registered (isHuman=true above) but NOT Go-state-registered, with its
	// nullifier and commitment already consumed — an unrecoverable stuck
	// state for that user. Calling it here means a failure can undo the
	// EVM-mirror slot writes above in the same place that knows their old
	// values, leaving no partial state for the caller to deal with at all.
	//
	// FIX (audit recheck 2, P1 #8/#11): now RegisterHumanAtomic instead of
	// RegisterHuman — the Go-state mutation and the register_human outbox
	// insert (pendingTx, built by the caller before this call) commit or
	// roll back together as one DB transaction, closing the same
	// non-atomic-outbox gap RegisterHumanAtomic already closed for the
	// non-mirror path. A failure here still triggers the EVM-mirror-slot
	// rollback below exactly as before.
	if regErr := a.state.RegisterHumanAtomic(wallet, pendingTx); regErr != nil {
		// FIX (audit recheck3, P1): reuse the same slots/oldValues this
		// function already built above instead of a second hardcoded list —
		// the previous version of this rollback zeroed every slot
		// unconditionally (common.Hash{}.Hex()) rather than restoring its
		// true pre-mirror value, which happened to be correct only because
		// every one of these slots is fresh for a never-registered wallet;
		// reverting to oldValues is the actually-correct operation and
		// checks each write's error instead of discarding it.
		for j := range slots {
			if revertErr := a.state.SaveStorageSlot(addrStr, slots[j].slot, oldValues[j]); revertErr != nil {
				fmt.Printf("[REGISTER] CRITICAL: Go-state register failed AND revert of mirror slot %d also failed for %s — EVM mirror may be inconsistent: %v\n", j, wallet, revertErr)
			}
		}
		return "", fmt.Errorf("mirror EVM slots written but Go-state RegisterHuman failed (rolled back): %w", regErr)
	}

	// txHash is the same value the caller already put in pendingTx.TxHash
	// (computed there, before this call, so it could be included in the
	// same atomic transaction above) — reuse it rather than recomputing,
	// guaranteeing they can never disagree.
	txHash := pendingTx.TxHash
	fmt.Printf("[REGISTER] Native V7 mirror persisted for %s, tx=%s\n", wallet, txHash)
	return txHash, nil
}

func validRegisterSignature(contractAddr, claimedHuman common.Address, commitment *big.Int, nullifierHex string, sigBytes []byte) bool {
	nullBigSig := new(big.Int)
	if strings.HasPrefix(nullifierHex, "0x") || strings.HasPrefix(nullifierHex, "0X") {
		nullBigSig.SetString(strings.TrimPrefix(strings.TrimPrefix(nullifierHex, "0x"), "0X"), 16)
	} else {
		if _, ok := nullBigSig.SetString(nullifierHex, 10); !ok {
			nullBigSig.SetString(nullifierHex, 16)
		}
	}
	nullKey := common.BigToHash(nullBigSig)
	packed := make([]byte, 0, 32+20+8+32+32)
	packed = append(packed, common.LeftPadBytes(big.NewInt(1926).Bytes(), 32)...)
	packed = append(packed, contractAddr.Bytes()...)
	packed = append(packed, []byte("register")...)
	packed = append(packed, common.LeftPadBytes(commitment.Bytes(), 32)...)
	packed = append(packed, nullKey.Bytes()...)
	messageHash := crypto.Keccak256Hash(packed)
	ethHash := accounts.TextHash(messageHash.Bytes())
	sig := append([]byte(nil), sigBytes...)
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(ethHash, sig)
	if err != nil {
		return false
	}
	return bytes.Equal(crypto.PubkeyToAddress(*pub).Bytes(), claimedHuman.Bytes())
}

func loadSlotBig(state *ChainState, addr string, slot int64) *big.Int {
	value, _ := state.LoadStorageSlot(addr, common.BigToHash(big.NewInt(slot)).Hex())
	out := new(big.Int)
	if value != "" {
		out.SetBytes(common.HexToHash(value).Bytes())
	}
	return out
}

// uint256Max is the maximum value for a Solidity uint256 parameter.
var uint256Max = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

func parseUint2(values []string) ([2]*big.Int, error) {
	var out [2]*big.Int
	for i := 0; i < 2; i++ {
		n := new(big.Int)
		_, ok := n.SetString(values[i], 10)
		if !ok {
			return out, fmt.Errorf("invalid number at index %d: %s", i, values[i])
		}
		// P2-17: reject values that exceed uint256 — ABI-encode would silently truncate them.
		if n.Sign() < 0 || n.Cmp(uint256Max) > 0 {
			return out, fmt.Errorf("value at index %d exceeds uint256 range", i)
		}
		out[i] = n
	}
	return out, nil
}

func parseUint2x2(values [][]string) ([2][2]*big.Int, error) {
	var out [2][2]*big.Int
	for i := 0; i < 2; i++ {
		if len(values[i]) < 2 {
			return out, fmt.Errorf("row %d has fewer than 2 elements", i)
		}
		for j := 0; j < 2; j++ {
			n := new(big.Int)
			_, ok := n.SetString(values[i][j], 10)
			if !ok {
				return out, fmt.Errorf("invalid number at [%d][%d]: %s", i, j, values[i][j])
			}
			// P2-AUDIT: Reject values exceeding uint256 — ABI-encode would
			// silently truncate them, letting attackers pass oversize values
			// that appear to be valid ZK proof components but are not.
			if n.Sign() < 0 || n.Cmp(uint256Max) > 0 {
				return out, fmt.Errorf("value at [%d][%d] exceeds uint256 range", i, j)
			}
			out[i][j] = n
		}
	}
	return out, nil
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// bigIntsToHexStrings converts a slice of *big.Int to their decimal string
// representations for JSON serialization in block transactions.
// Decimal (base-10) is used instead of hex to match the ZK proof wire format
// used by the client and the Groth16 verifier ABI.
func bigIntsToHexStrings(vals []*big.Int) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		if v == nil {
			out[i] = "0"
		} else {
			out[i] = v.Text(10)
		}
	}
	return out
}

// bigInt2x2ToHexStrings converts a [2][2]*big.Int matrix to [][]string
// for JSON serialization in block transactions.
func bigInt2x2ToHexStrings(vals [2][2]*big.Int) [][]string {
	out := make([][]string, 2)
	for i := 0; i < 2; i++ {
		out[i] = make([]string, 2)
		for j := 0; j < 2; j++ {
			if vals[i][j] == nil {
				out[i][j] = "0"
			} else {
				out[i][j] = vals[i][j].Text(10)
			}
		}
	}
	return out
}
