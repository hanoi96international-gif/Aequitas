package keeper

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// verifyPersonalSign checks that signature is a valid personal_sign
// signature of message, produced by the private key behind claimedWallet.
// This mirrors what MetaMask's personal_sign does client-side: it hashes
// "\x19Ethereum Signed Message:\n" + len(message) + message, then
// recovers the signer's address from the signature and compares it
// against claimedWallet.
//
// This exists because, unlike registerWithSig (which runs through the V7
// contract and lets the contract itself verify the signature on-chain),
// the swap/liquidity/faucet actions below are pure ChainState operations
// with no contract involved — so this Go code has to do the signature
// check itself. Without it, anyone could POST {"wallet": "someone else's
// address", ...} and act on a wallet they don't actually control.
func verifyPersonalSign(message, signatureHex, claimedWallet string) error {
	sigHex := strings.TrimPrefix(signatureHex, "0x")
	sigBytes := common.Hex2Bytes(sigHex)
	if len(sigBytes) != 65 {
		return fmt.Errorf("signature must be 65 bytes, got %d", len(sigBytes))
	}

	// go-ethereum's recovery expects the V value (last byte) to be 0 or 1,
	// but personal_sign / eth_sign in wallets produce 27 or 28 (EIP-191
	// convention) — normalize before recovery.
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}

	prefixed := []byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message))
	hash := crypto.Keccak256(prefixed)

	pubKey, err := crypto.SigToPub(hash, sigBytes)
	if err != nil {
		return fmt.Errorf("could not recover public key: %w", err)
	}

	recovered := strings.ToLower(crypto.PubkeyToAddress(*pubKey).Hex())
	claimed := strings.ToLower(claimedWallet)
	if recovered != claimed {
		return fmt.Errorf("signature does not match claimed wallet (recovered %s)", recovered)
	}
	return nil
}

// ── SWAP ──────────────────────────────────────────────────────────────────

type SwapRequest struct {
	Wallet    string  `json:"wallet"`
	Direction string  `json:"direction"`
	Amount    float64 `json:"amount"`
	// MinAmountOut is an optional client-supplied slippage floor: if the
	// actual swap output would be less than this, the swap is rejected
	// before any state is mutated instead of silently executing at
	// whatever price the pool happens to be at. Zero (the default, and
	// what every pre-existing client sends) means "no protection" —
	// preserving old behavior for clients that don't set it. Intentionally
	// NOT part of the signed message: it's a safety floor for the caller's
	// own benefit, not a security-critical value that needs tamper-proofing
	// (a stripped/altered MinAmountOut only removes the caller's own
	// protection, it cannot move funds anywhere).
	MinAmountOut float64 `json:"min_amount_out,omitempty"`
	Nonce        int64   `json:"nonce"`     // per-wallet monotonic counter — atomically consumed on use
	Timestamp    int64   `json:"timestamp"` // Unix time — secondary guard against stale requests
	Signature    string  `json:"signature"`
}

type SwapResponse struct {
	Success   bool    `json:"success"`
	Message   string  `json:"message"`
	AmountOut float64 `json:"amount_out"`
	NewAEQ    float64 `json:"new_aeq_balance"`
	NewTUSD   float64 `json:"new_tusd_balance"`
}

func (a *APIServer) handleSwap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: "POST required"})
		return
	}

	// FIX (Monster Audit 2026-07-12, P2): see readBodyLimited's comment —
	// the discarded error here used to hide an oversized body behind a
	// generic "invalid request" instead of a real 413.
	body, tooLarge, err := readBodyLimited(w, r, 64<<10) // 64 KB
	if tooLarge {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: "request body too large"})
		return
	}
	var req SwapRequest
	if err != nil || json.Unmarshal(body, &req) != nil {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: "invalid request"})
		return
	}

	wallet := strings.ToLower(req.Wallet)
	if wallet == "" || req.Amount <= 0 {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: "wallet and positive amount required"})
		return
	}
	if req.Direction != "aeq_to_tusd" && req.Direction != "tusd_to_aeq" {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: "direction must be aeq_to_tusd or tusd_to_aeq"})
		return
	}
	// Reject stale or future-dated requests to prevent replay attacks.
	// The timestamp is part of the signed message, so an attacker cannot
	// strip or change it without invalidating the signature.
	if diff := time.Now().Unix() - req.Timestamp; diff < -60 || diff > 60 {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: "request expired or timestamp out of range"})
		return
	}

	message := fmt.Sprintf("Aequitas Swap: %s %.8f nonce:%d ts:%d", req.Direction, req.Amount, req.Nonce, req.Timestamp)
	if err := verifyPersonalSign(message, req.Signature, wallet); err != nil {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: "signature invalid: " + err.Error()})
		return
	}
	// Consume nonce FIRST, atomically. This blocks parallel requests with the
	// same signature — only one can win the atomic increment; the other gets
	// "already used" before the swap even runs, preventing double-execution.
	if err := a.state.ConsumeSwapNonce(wallet, req.Nonce); err != nil {
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: err.Error()})
		return
	}

	txType := "swap_aeq_tusd"
	aeqToTusd := req.Direction == "aeq_to_tusd"
	if !aeqToTusd {
		txType = "swap_tusd_aeq"
	}
	// FIX (atomic outbox): SwapAtomic commits the state mutation and the
	// pending_tx outbox insert as a single DB transaction (see
	// runAtomicWithOutbox / TransferAtomic's comment) — either both happen
	// or neither does, instead of the old separate Swap-then-SavePendingTx
	// sequence where the outbox write could fail independently after the
	// swap had already committed (swaps are likely the most frequent
	// state-changing operation in the system, so this was the most
	// frequently-hit version of the durability gap).
	pendingTxTemplate := Transaction{Type: txType, Wallet: wallet, Amount: req.Amount}
	amountOut, _, err := a.state.SwapAtomic(wallet, req.Amount, aeqToTusd, req.MinAmountOut, pendingTxTemplate)
	if err != nil {
		// Swap failed — restore nonce so user can retry with the same nonce.
		a.state.RestoreSwapNonce(wallet, req.Nonce)
		json.NewEncoder(w).Encode(SwapResponse{Success: false, Message: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(SwapResponse{
		Success:   true,
		Message:   "swap successful",
		AmountOut: amountOut,
		NewAEQ:    a.state.GetBalance(wallet),
		NewTUSD:   a.state.GetTUsdBalance(wallet),
	})
}

// ── ADD LIQUIDITY ────────────────────────────────────────────────────────

type AddLiquidityRequest struct {
	Wallet     string  `json:"wallet"`
	AmountAEQ  float64 `json:"amount_aeq"`
	AmountTUSD float64 `json:"amount_tusd"`
	Nonce      int64   `json:"nonce"`
	Timestamp  int64   `json:"timestamp"`
	Signature  string  `json:"signature"`
}

type AddLiquidityResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (a *APIServer) handleAddLiquidity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: "POST required"})
		return
	}

	body, tooLarge, err := readBodyLimited(w, r, 64<<10)
	if tooLarge {
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: "request body too large"})
		return
	}
	var req AddLiquidityRequest
	if err != nil || json.Unmarshal(body, &req) != nil {
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: "invalid request"})
		return
	}

	wallet := strings.ToLower(req.Wallet)
	if wallet == "" || req.AmountAEQ <= 0 || req.AmountTUSD <= 0 {
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: "wallet and positive amounts required"})
		return
	}
	if diff := time.Now().Unix() - req.Timestamp; diff < -60 || diff > 60 {
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: "request expired or timestamp out of range"})
		return
	}

	message := fmt.Sprintf("Aequitas Add Liquidity: %.8f AEQ + %.8f tUSD nonce:%d ts:%d", req.AmountAEQ, req.AmountTUSD, req.Nonce, req.Timestamp)
	if err := verifyPersonalSign(message, req.Signature, wallet); err != nil {
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: "signature invalid: " + err.Error()})
		return
	}
	if err := a.state.ConsumeSwapNonce(wallet, req.Nonce); err != nil {
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: err.Error()})
		return
	}
	// FIX (atomic outbox): AddLiquidityAtomic commits the state mutation and
	// the pending_tx outbox insert as a single DB transaction — see
	// SwapAtomic's comment above / TransferAtomic's comment in state.go.
	pendingTxTemplate := Transaction{Type: "add_liquidity", Wallet: wallet, Amount: req.AmountAEQ, AmountOut: req.AmountTUSD}
	_, err = a.state.AddLiquidityAtomic(wallet, req.AmountAEQ, req.AmountTUSD, pendingTxTemplate)
	if err != nil {
		a.state.RestoreSwapNonce(wallet, req.Nonce)
		json.NewEncoder(w).Encode(AddLiquidityResponse{Success: false, Message: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(AddLiquidityResponse{Success: true, Message: "liquidity added"})
}

// ── REMOVE LIQUIDITY ─────────────────────────────────────────────────────

type RemoveLiquidityRequest struct {
	Wallet       string  `json:"wallet"`
	SharesToBurn float64 `json:"shares"`
	Nonce        int64   `json:"nonce"`
	Timestamp    int64   `json:"timestamp"`
	Signature    string  `json:"signature"`
}

type RemoveLiquidityResponse struct {
	Success    bool    `json:"success"`
	Message    string  `json:"message"`
	AmountAEQ  float64 `json:"amount_aeq"`
	AmountTUSD float64 `json:"amount_tusd"`
}

func (a *APIServer) handleRemoveLiquidity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: "POST required"})
		return
	}

	body, tooLarge, err := readBodyLimited(w, r, 64<<10)
	if tooLarge {
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: "request body too large"})
		return
	}
	var req RemoveLiquidityRequest
	if err != nil || json.Unmarshal(body, &req) != nil {
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: "invalid request"})
		return
	}

	wallet := strings.ToLower(req.Wallet)
	if wallet == "" || req.SharesToBurn <= 0 {
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: "wallet and positive shares required"})
		return
	}
	if diff := time.Now().Unix() - req.Timestamp; diff < -60 || diff > 60 {
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: "request expired or timestamp out of range"})
		return
	}

	message := fmt.Sprintf("Aequitas Remove Liquidity: %.8f shares nonce:%d ts:%d", req.SharesToBurn, req.Nonce, req.Timestamp)
	if err := verifyPersonalSign(message, req.Signature, wallet); err != nil {
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: "signature invalid: " + err.Error()})
		return
	}
	if err := a.state.ConsumeSwapNonce(wallet, req.Nonce); err != nil {
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: err.Error()})
		return
	}
	// FIX (atomic outbox): RemoveLiquidityAtomic commits the state mutation
	// and the pending_tx outbox insert as a single DB transaction — see
	// SwapAtomic's comment above / TransferAtomic's comment in state.go.
	pendingTxTemplate := Transaction{Type: "remove_liquidity", Wallet: wallet, Amount: req.SharesToBurn}
	outAEQ, outTUSD, _, err := a.state.RemoveLiquidityAtomic(wallet, req.SharesToBurn, pendingTxTemplate)
	if err != nil {
		a.state.RestoreSwapNonce(wallet, req.Nonce)
		json.NewEncoder(w).Encode(RemoveLiquidityResponse{Success: false, Message: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(RemoveLiquidityResponse{
		Success:    true,
		Message:    "liquidity removed",
		AmountAEQ:  outAEQ,
		AmountTUSD: outTUSD,
	})
}

// ── LP POSITION STATUS ───────────────────────────────────────────────────

func (a *APIServer) handleLPPosition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	wallet := strings.ToLower(r.URL.Query().Get("wallet"))
	if wallet == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"shares": 0, "total_shares": 0, "pool_share_pct": 0})
		return
	}
	mine, total := a.state.GetLPShares(wallet)
	pct := 0.0
	if total > 0 {
		pct = mine / total * 100
	}
	reserveAEQ, reserveTUSD := a.state.GetPoolReserves()
	// Floor to 6 decimal places (not round) to prevent "insufficient balance"
	// when the user clicks MAX: float division can produce e.g. 99.9999997 which
	// rounds up to 100.000000 but the actual balance is 99.999999.
	floorD := func(v float64) float64 {
		return math.Floor(v*1_000_000) / 1_000_000
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"shares":         mine,
		"total_shares":   total,
		"pool_share_pct": pct,
		"withdrawable_aeq": func() float64 {
			if total == 0 {
				return 0
			}
			return floorD(reserveAEQ * (mine / total))
		}(),
		"withdrawable_tusd": func() float64 {
			if total == 0 {
				return 0
			}
			return floorD(reserveTUSD * (mine / total))
		}(),
	})
}

type FaucetRequest struct {
	Wallet    string `json:"wallet"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"timestamp"`
}

type FaucetResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	Granted float64 `json:"granted"`
}

func (a *APIServer) handleFaucet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: "POST required"})
		return
	}

	body, tooLarge, err := readBodyLimited(w, r, 64<<10)
	if tooLarge {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: "request body too large"})
		return
	}
	var req FaucetRequest
	if err != nil || json.Unmarshal(body, &req) != nil {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: "invalid request"})
		return
	}

	wallet := strings.ToLower(req.Wallet)
	if wallet == "" {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: "wallet required"})
		return
	}

	// FIX 8: validate timestamp — reject requests older than 5 minutes or
	// more than 30 seconds in the future to prevent replay attacks.
	now := time.Now().Unix()
	if req.Timestamp == 0 {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: "timestamp required"})
		return
	}
	if diff := now - req.Timestamp; diff > 300 || diff < -30 {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: "timestamp expired or invalid"})
		return
	}
	// Message must include the timestamp to match the client-side signing message.
	message := fmt.Sprintf("Aequitas tUSD Faucet Claim: %s ts:%d", wallet, req.Timestamp)
	if err := verifyPersonalSign(message, req.Signature, wallet); err != nil {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: "signature invalid: " + err.Error()})
		return
	}

	// FIX (atomic outbox): ClaimTUsdFaucetAtomic commits the state mutation
	// and the pending_tx outbox insert as a single DB transaction — see
	// SwapAtomic's comment above / TransferAtomic's comment in state.go.
	pendingTx := Transaction{Type: "faucet", Wallet: wallet, Amount: tusdFaucetAmount}
	if err := a.state.ClaimTUsdFaucetAtomic(wallet, pendingTx); err != nil {
		json.NewEncoder(w).Encode(FaucetResponse{Success: false, Message: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(FaucetResponse{Success: true, Message: "faucet claimed", Granted: tusdFaucetAmount})
}

// ── POOL STATUS ──────────────────────────────────────────────────────────

func (a *APIServer) handlePoolStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	reserveAEQ, reserveTUSD := a.state.GetPoolReserves()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reserve_aeq":  reserveAEQ,
		"reserve_tusd": reserveTUSD,
		"price_aeq_in_tusd": func() float64 {
			if reserveAEQ == 0 {
				return 0
			}
			return reserveTUSD / reserveAEQ
		}(),
	})
}
