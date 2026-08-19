package keeper

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

// BioVerifierAddress is the deployed Groth16 verifier contract on Aequitas Chain.
const BioVerifierAddress = "0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2"

// HumanityCredential is a self-verifiable Proof-of-Humanity credential.
// Any external system can independently verify it via three mechanisms:
//  1. ZKProof: call BioVerifier.verifyProof(a,b,c,pubSignals) on any EVM that
//     has the same Groth16 verifier deployed.
//  2. Nullifier: confirms the biometric was unique at registration time.
//  3. ValidatorSignature: ECDSA attestation from an authorized Aequitas node.
type HumanityCredential struct {
	Wallet             string   `json:"wallet"`
	IsHuman            bool     `json:"is_human"`
	ChainID            int      `json:"chain_id"`
	RegisteredAtHeight int64    `json:"registered_at_height,omitempty"`
	RegisteredAtBlock  string   `json:"registered_at_block,omitempty"`
	RegisteredAtUnix   int64    `json:"registered_at_unix,omitempty"`
	Nullifier          string   `json:"nullifier,omitempty"`
	ZKProof            *ZKProof `json:"zk_proof,omitempty"`
	VerifierContract   string   `json:"verifier_contract"`
	ValidatorAddress   string   `json:"validator_address"`
	ValidatorSignature string   `json:"validator_signature"`
}

type ZKProof struct {
	A          []string   `json:"a"`
	B          [][]string `json:"b"`
	C          []string   `json:"c"`
	PubSignals []string   `json:"pub_signals"`
}

// handleHumanityCredential serves GET /api/humanity/credential?wallet=0x...
//
// Returns a signed, self-verifiable Proof-of-Humanity credential for the
// given wallet. The credential bundles:
//   - isHuman status from local chain state
//   - the original Groth16 ZK proof from the registration block
//   - the nullifier that proves biometric uniqueness
//   - an ECDSA signature from this node's signing key
//
// External verifiers (other chains, dApps, websites) can verify the ZK proof
// independently by deploying the same BioVerifier circuit — no trust in
// Aequitas nodes required for the cryptographic part.
func (a *APIServer) handleHumanityCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// FIX (P2, security audit 2026-07-21, ported to main 2026-08-14): this
	// endpoint is public and unauthenticated, but unlike its neighbours
	// (handleCheckRegistrationByBioHash, handleRecoverEscrow,
	// handleSetGuardian, handleConfirmAlive — all of which use the shared
	// registerRateLimit cooldown) it had no rate limit at all, even though
	// GetRegistrationProof below does a sequential scan over chain_blocks
	// (millions of rows and growing) with a JSONB transaction-array expansion
	// for every is_human=true lookup. Same package-level registerRateLimit
	// map and same per-IP 5s window as the biohash check, keyed separately so
	// it cannot be used to also throttle other endpoints.
	ip := clientIP(r)
	if ts, loaded := registerRateLimit.Load("credential:" + ip); loaded {
		if time.Since(ts.(time.Time)) < 5*time.Second {
			jsonError(w, "rate limited, try again shortly", http.StatusTooManyRequests)
			return
		}
	}
	registerRateLimit.Store("credential:"+ip, time.Now())

	wallet := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wallet")))
	if len(wallet) != 42 || wallet[:2] != "0x" {
		http.Error(w, `{"error":"missing or malformed wallet — expected 0x-prefixed 20-byte address"}`, http.StatusBadRequest)
		return
	}

	isHuman := a.state.IsHuman(wallet)

	cred := HumanityCredential{
		Wallet:           wallet,
		IsHuman:          isHuman,
		ChainID:          1926,
		VerifierContract: BioVerifierAddress,
	}

	if isHuman {
		// Nullifier for this wallet.
		nullifier := a.state.GetNullifierByWallet(wallet)
		cred.Nullifier = nullifier

		// ZK proof + block metadata from the registration block.
		height, blockHash, blockTime, proof := a.state.GetRegistrationProof(wallet)
		if proof != nil {
			cred.RegisteredAtHeight = height
			cred.RegisteredAtBlock = blockHash
			cred.RegisteredAtUnix = blockTime
			cred.ZKProof = proof
		}
	}

	// Sign the credential with the node's ECDSA key.
	// Message: "Aequitas PoH: <wallet> chain=1926 nullifier=<nullifier> block=<blockHash>"
	// This lets any verifier confirm the attestation without trusting the JSON.
	key := a.blockchain.GetSigningKey()
	if key != nil {
		signerAddr := crypto.PubkeyToAddress(key.PublicKey)
		cred.ValidatorAddress = strings.ToLower(signerAddr.Hex())

		msg := fmt.Sprintf("Aequitas PoH: %s chain=1926 nullifier=%s block=%s",
			wallet, cred.Nullifier, cred.RegisteredAtBlock)
		msgHash := accounts.TextHash([]byte(msg))
		sig, err := crypto.Sign(msgHash, key)
		if err == nil {
			// eth_sign uses recovery id 27/28 (not 0/1)
			sig[64] += 27
			cred.ValidatorSignature = "0x" + fmt.Sprintf("%x", sig)
		}
	}

	json.NewEncoder(w).Encode(cred)
}

// GetNullifierByWallet returns the nullifier hex string for a wallet, or "".
// Queries the nullifiers table which is indexed on lower(wallet_address).
func (cs *ChainState) GetNullifierByWallet(wallet string) string {
	wallet = strings.ToLower(wallet)
	// Fast path: in-memory cache
	cs.mu.RLock()
	for nullifier, w := range cs.nullifiers {
		if w == wallet {
			cs.mu.RUnlock()
			return nullifier
		}
	}
	cs.mu.RUnlock()

	// DB fallback
	if cs.db == nil {
		return ""
	}
	var nullifier string
	row := cs.db.QueryRow(
		`SELECT nullifier FROM nullifiers WHERE lower(wallet_address) = $1 LIMIT 1`,
		wallet,
	)
	if err := row.Scan(&nullifier); err != nil {
		return ""
	}
	return nullifier
}

// GetRegistrationProof finds the register_human transaction for wallet in
// chain_blocks and returns the block height, hash, unix timestamp, and the
// Groth16 ZK proof fields. Returns nil proof if no registration block is found.
func (cs *ChainState) GetRegistrationProof(wallet string) (height int64, blockHash string, blockTime int64, proof *ZKProof) {
	wallet = strings.ToLower(wallet)
	if cs.db == nil {
		return
	}

	// Query chain_blocks: find the first block containing a register_human TX
	// for this wallet. The transactions column is stored as a JSON array.
	rows, err := cs.db.Query(`
		SELECT b.hash, b.height, b.timestamp, tx.value
		FROM chain_blocks b,
		     jsonb_array_elements(b.transactions::jsonb) AS tx(value)
		WHERE tx.value->>'type' = 'register_human'
		  AND lower(tx.value->>'wallet') = $1
		ORDER BY b.height ASC
		LIMIT 1
	`, wallet)
	if err != nil {
		return
	}
	defer rows.Close()

	if !rows.Next() {
		return
	}

	var txJSON []byte
	if err := rows.Scan(&blockHash, &height, &blockTime, &txJSON); err != nil {
		return
	}

	var tx struct {
		ProofA     []string   `json:"proof_a"`
		ProofB     [][]string `json:"proof_b"`
		ProofC     []string   `json:"proof_c"`
		PubSignals []string   `json:"pub_signals"`
	}
	if err := json.Unmarshal(txJSON, &tx); err != nil {
		return
	}

	// Validate proof fields are present and well-formed before returning.
	if len(tx.ProofA) != 2 || len(tx.ProofB) != 2 || len(tx.ProofC) != 2 || len(tx.PubSignals) < 2 {
		return
	}
	for _, s := range append(tx.ProofA, tx.ProofC...) {
		n := new(big.Int)
		if _, ok := n.SetString(s, 10); !ok {
			return
		}
	}
	proof = &ZKProof{
		A:          tx.ProofA,
		B:          tx.ProofB,
		C:          tx.ProofC,
		PubSignals: tx.PubSignals,
	}
	return
}
