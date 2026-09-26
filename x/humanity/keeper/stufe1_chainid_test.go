package keeper

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// Mit Stufe 1 verlangt die Blockpruefung Chain-ID 1926. Eine alte,
// ungeschuetzte Transaktion (vor EIP-155) stellt LatestSignerForChainID
// trotzdem wieder her -- angenommen, haette jeder andere Validator den Block
// verworfen. Sie muss schon an der Annahme scheitern.
func TestStufe1_UngeschuetzteTransaktionWirdAbgewiesen(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	noteBlockProduced()
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)

	sende := func(signer types.Signer) *RPCError {
		t.Helper()
		priv, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		absender := strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())
		cs.mu.Lock()
		cs.accounts.Set(absender, &AccountState{Address: absender, Balance: NewDecimal(1000)})
		cs.mu.Unlock()
		tx := types.NewTransaction(0, addrFromHexForTest(t, testRecipientHex), aeqWei(1), 21000, big.NewInt(0), nil)
		stx, err := types.SignTx(tx, signer, priv)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := stx.MarshalBinary()
		p, _ := json.Marshal("0x" + hex.EncodeToString(raw))
		_, rerr := srv.sendRawTransaction([]json.RawMessage{p}, nil)
		return rerr
	}

	if rerr := sende(types.HomesteadSigner{}); rerr == nil || !strings.Contains(rerr.Message, "chain id 1926") {
		t.Fatalf("ungeschuetzte Transaktion nicht abgewiesen: %v", rerr)
	}
	if rerr := sende(types.NewEIP155Signer(big.NewInt(1926))); rerr != nil && strings.Contains(rerr.Message, "chain id 1926") {
		t.Fatalf("geschuetzte Transaktion mit Chain-ID 1926 abgewiesen: %v", rerr)
	}
}
