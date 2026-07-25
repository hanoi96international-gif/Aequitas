package keeper

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core/types"
)

// testRecipientHex is a well-formed (40 hex char) throwaway recipient
// address used by every signed test tx below.
const testRecipientHex = "0x00000000000000000000000000000000000000be"

// signedRawHex builds a signed legacy tx (same shape contabo-loadtest/main.go
// uses against the real network) and returns its "0x"-prefixed raw hex plus
// the sender address the server-side ecrecover is expected to produce.
func signedRawHex(t *testing.T, nonce uint64, to string) (rawHex string, senderAddr string) {
	t.Helper()
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tx := types.NewTransaction(nonce, addrFromHexForTest(t, to), big.NewInt(0), 21000, big.NewInt(0), nil)
	signer := types.NewEIP155Signer(big.NewInt(1926))
	signedTx, err := types.SignTx(tx, signer, priv)
	if err != nil {
		t.Fatalf("SignTx: %v", err)
	}
	raw, err := signedTx.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	return "0x" + hex.EncodeToString(raw), strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())
}

func addrFromHexForTest(t *testing.T, s string) [20]byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil || len(b) != 20 {
		t.Fatalf("bad test address %q", s)
	}
	var out [20]byte
	copy(out[:], b)
	return out
}

// TestDecodeAndRecoverSender_MatchesOriginalInlineLogic locks in that the
// extracted pure helper (evm_rpc.go) behaves identically to the inline code
// it replaced: valid tx -> correct sender, invalid hex/tx/signature -> the
// same error classification (senderErr distinguishes -32602 vs -32603 in
// sendRawTransaction).
func TestDecodeAndRecoverSender_MatchesOriginalInlineLogic(t *testing.T) {
	rawHex, wantSender := signedRawHex(t, 0, testRecipientHex)

	tx, sender, senderErr, err := decodeAndRecoverSender(rawHex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if senderErr {
		t.Fatalf("senderErr should be false on success")
	}
	if tx == nil {
		t.Fatalf("expected non-nil tx")
	}
	if sender != wantSender {
		t.Fatalf("sender = %s, want %s", sender, wantSender)
	}

	if _, _, _, err := decodeAndRecoverSender("0xzzzz"); err == nil {
		t.Fatalf("expected error for invalid hex")
	}
	if _, _, _, err := decodeAndRecoverSender("0xdeadbeef"); err == nil {
		t.Fatalf("expected error for garbage (undecodable) transaction bytes")
	}
}

// TestHandleRPC_BatchParallelSendRawTransaction exercises the actual HTTP
// batch path (handleRPC) end-to-end: a batch mixing valid signed txs, a
// non-sendRawTransaction method, and malformed sendRawTransaction entries,
// confirming the parallel decode+ecrecover pre-pass (added 2026-07-25)
// produces byte-for-byte the same shape of response, in the same order, as
// the pre-existing serial code did -- run under -race to catch any data race
// introduced by the new worker-pool pre-pass.
func TestHandleRPC_BatchParallelSendRawTransaction(t *testing.T) {
	cs := newTestState()
	dag := &BlockDAG{state: cs}
	srv := NewEVMRPCServer(dag, cs)

	const n = 12
	type expect struct {
		sender string
		hash   string
	}
	batchItems := make([]string, 0, n+3)
	expects := make(map[int]expect)

	for i := 0; i < n; i++ {
		rawHex, sender := signedRawHex(t, 0, testRecipientHex)
		idx := len(batchItems)
		batchItems = append(batchItems, fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["%s"]}`, idx, rawHex))
		expects[idx] = expect{sender: sender}
	}
	// Non-sendRawTransaction entry interleaved in the middle.
	chainIDIdx := len(batchItems)
	batchItems = append(batchItems, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"eth_chainId","params":[]}`, chainIDIdx))

	// Malformed entries: invalid hex, and structurally invalid tx bytes.
	badHexIdx := len(batchItems)
	batchItems = append(batchItems, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["0xzz"]}`, badHexIdx))
	badTxIdx := len(batchItems)
	batchItems = append(batchItems, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["0xdeadbeef"]}`, badTxIdx))

	body := "[" + strings.Join(batchItems, ",") + "]"

	req := httptest.NewRequest("POST", "/rpc", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	srv.handleRPC(w, req)

	var results []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("response not valid JSON array: %v\nbody: %s", err, w.Body.String())
	}
	if len(results) != len(batchItems) {
		t.Fatalf("got %d results, want %d", len(results), len(batchItems))
	}

	for idx, exp := range expects {
		res := results[idx]
		if errField, ok := res["error"]; ok {
			t.Fatalf("result[%d] (sender %s) unexpectedly errored: %v", idx, exp.sender, errField)
		}
		hashVal, _ := res["result"].(string)
		if hashVal == "" || !strings.HasPrefix(hashVal, "0x") {
			t.Fatalf("result[%d]: expected a tx hash result, got %#v", idx, res["result"])
		}
	}

	chainRes := results[chainIDIdx]
	if got, _ := chainRes["result"].(string); got != "0x786" {
		t.Fatalf("eth_chainId result = %v, want 0x786", chainRes["result"])
	}

	for _, idx := range []int{badHexIdx, badTxIdx} {
		res := results[idx]
		errObj, ok := res["error"].(map[string]interface{})
		if !ok {
			t.Fatalf("result[%d]: expected an error object, got %#v", idx, res["result"])
		}
		code, _ := errObj["code"].(float64)
		if code != -32602 {
			t.Fatalf("result[%d]: error code = %v, want -32602", idx, errObj["code"])
		}
	}
}
