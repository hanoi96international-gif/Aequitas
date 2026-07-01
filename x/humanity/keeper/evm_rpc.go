package keeper

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// EVMRPCServer handles Ethereum JSON-RPC requests
type EVMRPCServer struct {
	dag               *BlockDAG
	state             *ChainState
	evm               *EVMEngine
	mu                sync.Mutex // guards all map fields below against concurrent writes
	nonces            map[string]uint64
	deployedContracts map[string]string // txHash -> contractAddress (lowercase)
	txStatus          map[string]bool   // txHash -> true if execution succeeded
	txError           map[string]string // txHash -> error message if failed
	txSenders         map[string]string // txHash -> sender address (lowercase)
	txTos             map[string]string // txHash -> to address (lowercase, "" for contract creation)
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
	}
	return &EVMRPCServer{
		dag:               dag,
		state:             state,
		evm:               engine,
		nonces:            make(map[string]uint64),
		deployedContracts: make(map[string]string),
		txStatus:          make(map[string]bool),
		txError:           make(map[string]string),
		txSenders:         make(map[string]string),
		txTos:             make(map[string]string),
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
		var results []interface{}
		for _, raw := range batch {
			result := s.handleSingle(raw)
			results = append(results, result)
		}
		json.NewEncoder(w).Encode(results)
		return
	}

	result := s.handleSingle(body)
	json.NewEncoder(w).Encode(result)
}

func (s *EVMRPCServer) handleSingle(body []byte) map[string]interface{} {
	var req struct {
		JSONRPC string            `json:"jsonrpc"`
		ID      interface{}       `json:"id"`
		Method  string            `json:"method"`
		Params  []json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return errorResponse(nil, -32700, "Parse error")
	}

	result, rpcErr := s.dispatch(req.Method, req.Params)
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

func (s *EVMRPCServer) dispatch(method string, params []json.RawMessage) (interface{}, *RPCError) {
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
		return s.sendRawTransaction(params)

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
	// Lock only for the map read/write — brief critical section.
	s.mu.Lock()
	if dbNonce > s.nonces[addr] {
		s.nonces[addr] = dbNonce
	}
	result := s.nonces[addr]
	s.mu.Unlock()
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
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
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

	fmt.Printf("[RPC] eth_call to=%s data=%x\n", toStr, data[:min4(len(data), 4)])

	// Intercept isHuman(address) calls (selector 0x2f543389) to V7.
	// The EVM engine sometimes returns an error for this simple storage read
	// causing ethers.js to throw "could not decode result data" and the proof
	// server to report "Chain unavailable". Read from Go state directly instead.
	if len(data) >= 4 && hex.EncodeToString(data[:4]) == "2f543389" &&
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
				new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
			)
			weiInt, _ := wei.Int(nil)
			// ABI-encode as uint256 (32 bytes, big-endian)
			result := make([]byte, 32)
			weiBytes := weiInt.Bytes()
			copy(result[32-len(weiBytes):], weiBytes)
			fmt.Printf("[RPC] balanceOf(%s) → native balance %.4f AEQ\n", addrHex, balance)
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

func (s *EVMRPCServer) sendRawTransaction(params []json.RawMessage) (interface{}, *RPCError) {
	if len(params) == 0 {
		return nil, &RPCError{Code: -32602, Message: "Missing params"}
	}

	var rawHex string
	if err := json.Unmarshal(params[0], &rawHex); err != nil {
		return nil, &RPCError{Code: -32602, Message: "invalid params"}
	}
	rawHex = strings.TrimPrefix(rawHex, "0x")

	rawBytes, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, &RPCError{Code: -32602, Message: "Invalid hex"}
	}

	tx := new(types.Transaction)
	// UnmarshalBinary handles all tx types: legacy (RLP), EIP-2930 (type 1), EIP-1559 (type 2)
	if err := tx.UnmarshalBinary(rawBytes); err != nil {
		// Fallback to RLP for legacy transactions
		if err2 := rlp.DecodeBytes(rawBytes, tx); err2 != nil {
			return nil, &RPCError{Code: -32602, Message: "Invalid transaction: " + err.Error()}
		}
	}

	// Recover sender
	signer := types.LatestSignerForChainID(big.NewInt(1926))
	sender, err := types.Sender(signer, tx)
	if err != nil {
		signer = types.NewEIP155Signer(big.NewInt(1926))
		sender, err = types.Sender(signer, tx)
		if err != nil {
			return nil, &RPCError{Code: -32603, Message: "Cannot recover sender: " + err.Error()}
		}
	}

	senderAddr := strings.ToLower(sender.Hex())
	txHash := tx.Hash().Hex() // already has 0x prefix

	fmt.Printf("[RPC] eth_sendRawTransaction hash=%s from=%s to=%v data=%d bytes\n",
		txHash, senderAddr, tx.To(), len(tx.Data()))

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
	//
	// KNOWN LIMITATION (audit 2026-07-01, P3-5): this reservation still
	// commits in its own statement, separate from whichever downstream
	// Atomic transfer/call actually applies the tx below — the same class
	// of crash-window gap fixed for swap/liquidity nonces in
	// ConsumeSwapNonce's comment (P1-4). Unlike swap, this entry point
	// branches into several structurally different downstream paths (plain
	// transfer, ERC20-transfer interception, generic contract calls,
	// registration submission) under a DIFFERENT lock (s.mu, not cs.mu), so
	// folding the reservation into each one's DB transaction isn't a
	// same-shape change and was deliberately deferred rather than risk a
	// regression in this RPC entry point (used by every MetaMask-facing
	// transaction). Impact if hit: a crash in this exact window burns a
	// nonce without applying the tx — the user must sign a new transaction;
	// no funds are lost.
	s.mu.Lock()
	// Populate from DB on first sight to recover correct nonce after restart.
	if s.nonces[senderAddr] == 0 {
		if dbNonce := s.state.LoadNonce(senderAddr); dbNonce > 0 {
			s.nonces[senderAddr] = dbNonce
		}
	}
	storedNonce := s.nonces[senderAddr]
	txNonce := tx.Nonce()
	if txNonce < storedNonce {
		s.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: fmt.Sprintf("nonce too low: tx=%d expected=%d", txNonce, storedNonce)}
	}
	if txNonce > storedNonce {
		s.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: fmt.Sprintf("nonce too high: tx=%d expected=%d", txNonce, storedNonce)}
	}
	// Reserve nonce immediately — prevents replay even if two identical
	// requests arrive concurrently.
	nextNonce := storedNonce + 1
	reserved, err := s.state.ReserveNonce(senderAddr, storedNonce, nextNonce)
	if err != nil {
		s.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: "nonce reservation failed: " + err.Error()}
	}
	if !reserved {
		dbNonce := s.state.LoadNonce(senderAddr)
		s.nonces[senderAddr] = dbNonce
		s.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: fmt.Sprintf("nonce already reserved: tx=%d expected=%d", txNonce, dbNonce)}
	}
	s.nonces[senderAddr] = nextNonce
	// Record sender (and to) for receipt construction — done inside the lock
	// while we already hold it, so no extra lock cycle needed.
	toAddrForReceipt := ""
	if tx.To() != nil {
		toAddrForReceipt = strings.ToLower(tx.To().Hex())
	}
	s.txSenders[txHash] = senderAddr
	s.txTos[txHash] = toAddrForReceipt
	s.mu.Unlock()

	// ── SIMPLE AEQ TRANSFER (native value transfer, no calldata) ─────────────
	if tx.To() != nil && len(tx.Data()) == 0 && tx.Value().Sign() > 0 {
		toAddr := strings.ToLower(tx.To().Hex())
		decimals := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		valueFloat, _ := new(big.Float).Quo(new(big.Float).SetInt(tx.Value()), decimals).Float64()

		// FIX P0-RACE: Set txStatus=true and persist receipt BEFORE calling
		// Transfer(). MetaMask polls getTransactionReceipt immediately after
		// receiving txHash. Without this, the window while Transfer() executes
		// (DB write, ~10-100ms) returned null receipts → MetaMask showed
		// "Senden fehlgeschlagen" even for successful transfers.
		s.mu.Lock()
		s.txStatus[txHash] = true
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
			s.mu.Unlock()
			s.state.SaveTxReceipt(txHash, senderAddr, toAddr, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: "Transfer failed: " + err.Error()}
		}
		s.state.SyncBalancesToEVM(V7_CONTRACT_ADDR, senderAddr, toAddr)
		fmt.Printf("[RPC] ✓ Transfer %.4f AEQ: %s → %s\n", valueFloat, senderAddr, toAddr)
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
		decimals := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		amountFloat, _ := new(big.Float).Quo(new(big.Float).SetInt(amountBig), decimals).Float64()

		// FIX P0-RACE: Set txStatus=true and persist receipt BEFORE calling
		// TransferWithV7Fee. Same race window as the native transfer path — MetaMask
		// polls getTransactionReceipt immediately after receiving txHash; without this
		// the window while TransferWithV7Fee executes returned null receipts.
		s.mu.Lock()
		s.txStatus[txHash] = true
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
	var knownPublicSelectors = map[string]bool{
		// transfer(address,uint256) — intercepted above and routed through Go-state
		"a9059cbb": true,
		// Read-only ERC-20 calls: safe to forward to EVM
		// NOTE: approve (095ea7b3) removed — it writes EVM state (allowance mapping)
		// but the V7 contract has no transferFrom, making approve/allowance a dead
		// flow. Keeping it in the allow-list is misleading.
		"70a08231": true, // balanceOf
		"dd62ed3e": true, // allowance (read-only view)
		"18160ddd": true, // totalSupply
		"06fdde03": true, // name
		"95d89b41": true, // symbol
		"313ce567": true, // decimals
		// NOTE: registerWithSig (13b81eb0) is intentionally NOT listed here.
		// Public callers must use /api/register which updates BOTH EVM and Go-state.
		// A raw /rpc call to registerWithSig would update only the EVM contract,
		// leaving RegisterHuman, bio_registrations, bio_hashes, and Go-balance unset.
		// Selector: registerWithSig(uint256[2],uint256[2][2],uint256[2],uint256[2],address,bytes,bytes32)
	}
	if tx.To() != nil && len(tx.Data()) >= 4 {
		sel := hex.EncodeToString(tx.Data()[:4])
		isV7 := strings.ToLower(tx.To().Hex()) == strings.ToLower(V7_CONTRACT_ADDR)
		// FIX: previously this allowlist gate only fired `if isV7`, leaving any
		// OTHER deployed contract (the relayer can deploy arbitrary bytecode,
		// see the deployment branch above) wide open to arbitrary calldata via
		// a signed raw tx with persist=true. The storage-persistence logic
		// downstream (dumpAndPersistStorageWithNullifier and the v7*Slots
		// tables in evm_engine.go) is hardcoded to V7's specific slot layout —
		// calling and persisting state for any other contract through this
		// path would silently write using the wrong slot semantics, corrupting
		// that contract's actual storage. No legitimate flow needs a raw
		// state-changing call to a non-V7 contract today (BioVerifier's
		// verifyProof is always invoked read-only, persist=false, elsewhere),
		// so reject it outright rather than letting it pass uninspected.
		if !isV7 {
			// FIX (audit 2026-06-29, Brutal-Audit P2-04): the nonce was already
			// reserved above before this gate runs. Returning bare without ever
			// calling SaveTxReceipt left txHash permanently receipt-less even
			// though its nonce slot was consumed — getTransactionReceipt(txHash)
			// returns null forever, which MetaMask renders as "still pending"
			// rather than failed, instead of resolving one way or the other.
			// Persist a failed (0x0) receipt, matching the pattern every other
			// reject-after-reservation path in this function already uses
			// (lines ~499, ~542, ~681), so the wallet gets a definitive answer.
			s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x0", "")
			return nil, &RPCError{Code: -32603, Message: "state-changing calls via /rpc are only supported for the V7 contract"}
		}
		if !knownPublicSelectors[sel] {
			// Special case: registerWithSig is only allowed when the signer is the
			// relayer itself (i.e. called internally by /api/register). External wallets
			// must go through /api/register so Go-state is updated atomically.
			if sel == "13b81eb0" {
				relayerAddr := relayerAddressFromEnv()
				if relayerAddr == "" || strings.ToLower(senderAddr) != relayerAddr {
					// FIX (audit 2026-06-29, Brutal-Audit P2-04): same
					// receipt-less-but-nonce-consumed gap as the !isV7 branch above.
					s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x0", "")
					return nil, &RPCError{Code: -32603, Message: "registerWithSig must be called via /api/register (direct RPC calls bypass Go-state updates)"}
				}
				// Allow: relayer is calling on behalf of /api/register
			} else {
				// FIX (audit 2026-06-29, Brutal-Audit P2-04): same gap as above.
				s.state.SaveTxReceipt(txHash, senderAddr, toAddrForReceipt, "0x0", "")
				return nil, &RPCError{Code: -32603, Message: "selector " + sel + " not supported directly via /rpc — use /api/* endpoints"}
			}
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

	block := s.dag.LatestBlock()
	height := uint64(0)
	blockHash := "0x" + strings.Repeat("0", 63) + "1"
	if block != nil {
		height = uint64(block.Height)
		blockHash = "0x" + block.Hash
	}

	return map[string]interface{}{
		"transactionHash":   txHash,
		"transactionIndex":  "0x0",
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

	return map[string]interface{}{
		"hash":             txHash,
		"nonce":            "0x0",
		"blockHash":        "0x" + strings.Repeat("0", 63) + "1",
		"blockNumber":      "0x1",
		"transactionIndex": "0x0",
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
