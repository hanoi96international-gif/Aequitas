package keeper

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http/httptest"
	"strconv"
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

// Ein Buendel mit Ueberweisungen EINES Absenders, Nonces in beliebiger
// Reihenfolge: mit Stufe 1 muss jede angenommen werden. Nebenlaeufig
// verarbeitet lief Nonce 6 vor 5, und 5 war "zu niedrig" (Generalprobe
// 26.09.2026: 440.169 von 605.430 abgelehnt).
func TestStufe1_BuendelEinesAbsendersWirdVollstaendigAngenommen(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	noteBlockProduced()
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	cs.mu.Unlock()

	const n = 12
	reihenfolge := []int{5, 0, 11, 3, 1, 7, 2, 9, 4, 10, 6, 8}
	var posten []string
	for k, nonce := range reihenfolge {
		tx := signiere(t, a, b.addr, aeqWei(1), uint64(nonce), 1926)
		posten = append(posten, `{"jsonrpc":"2.0","id":`+strconv.Itoa(k)+`,"method":"eth_sendRawTransaction","params":["`+tx.Roh+`"]}`)
	}
	req := httptest.NewRequest("POST", "/rpc", strings.NewReader("["+strings.Join(posten, ",")+"]"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleRPC(w, req)
	var antworten []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &antworten); err != nil {
		t.Fatalf("Antwort: %v (%s)", err, w.Body.String())
	}
	fehler := 0
	for _, r := range antworten {
		if r["error"] != nil {
			fehler++
			t.Logf("abgelehnt: %v", r["error"])
		}
	}
	if fehler != 0 || len(antworten) != n {
		t.Fatalf("%d von %d Posten abgelehnt", fehler, n)
	}
	if got := kontoVon(t, cs, a.addr).NaechsteNonce; got != n {
		t.Fatalf("NaechsteNonce %d, erwartet %d", got, n)
	}
}

func TestStufe1_BuendelGruppenNachAbsenderUndNonce(t *testing.T) {
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	mk := func(k testSchluessel, nonce uint64) *precomputedSendTx {
		tx := signiere(t, k, b.addr, aeqWei(1), nonce, 1926)
		dec, err := decodeRohTransaktion(tx.Roh)
		if err != nil {
			t.Fatal(err)
		}
		return &precomputedSendTx{tx: dec, sender: k.addr}
	}
	pre := []*precomputedSendTx{mk(a, 2), mk(b, 0), mk(a, 0), nil, mk(a, 1)}
	g := buendelGruppen(len(pre), pre, true)
	if fmt.Sprint(g) != "[[2 4 0] [1] [3]]" {
		t.Fatalf("Gruppen %v, erwartet [[2 4 0] [1] [3]]", g)
	}
	if g := buendelGruppen(len(pre), pre, false); len(g) != len(pre) {
		t.Fatalf("ohne Stufe 1: %d Gruppen, erwartet %d (alles nebenlaeufig)", len(g), len(pre))
	}
}

// Im Vorlauf setzt die Annahme NaechsteNonce, sobald eine Ueberweisung ihre
// Rohform traegt; das Nachspielen muss dasselbe tun (26.09.2026: 1.772
// Konten mit verschiedenem Blatt auf C1 und C2). Ungueltige Rohformen wirken
// nicht und fuehren nicht zur Ablehnung; die Nonce sinkt nie.
func TestStufe1_VorlaufNachspielenSetztNonceWieAnnahme(t *testing.T) {
	cs := newTestState()
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	cs.accounts.Set(b.addr, &AccountState{Address: b.addr, Balance: NewDecimal(1000), NaechsteNonce: 9})
	cs.mu.Unlock()

	falsch := signiere(t, a, b.addr, aeqWei(1), 50, 1926)
	falsch.Wallet = b.addr // Rohform von a, im Block als b -- darf nichts bewirken
	ohneRoh := Transaction{Type: "transfer", Wallet: a.addr, To: b.addr, Amount: 1}
	txs := []Transaction{
		signiere(t, a, b.addr, aeqWei(1), 1, 1926),
		signiere(t, a, b.addr, aeqWei(1), 0, 1926),
		signiere(t, a, b.addr, aeqWei(1), 2, 1926),
		signiere(t, b, a.addr, aeqWei(1), 3, 1926), // unter der Nonce von b: nie herunter
		falsch,
		ohneRoh,
	}
	cs.mu.Lock()
	err := cs.setzeVorlaufNoncenLocked(context.Background(), txs, nil)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("Vorlauf-Nonces: %v", err)
	}
	if got := kontoVon(t, cs, a.addr).NaechsteNonce; got != 3 {
		t.Fatalf("NaechsteNonce von a %d, erwartet 3 (wie bei der Annahme)", got)
	}
	if got := kontoVon(t, cs, b.addr).NaechsteNonce; got != 9 {
		t.Fatalf("NaechsteNonce von b %d, erwartet 9 (sinkt nie, falsche Rohform wirkt nicht)", got)
	}
	acc := kontoVon(t, cs, a.addr)
	if acc.leafHash != accountLeaf(&acc) {
		t.Fatal("Blatt nicht nachgezogen -- account_set_xor liefe auseinander")
	}
}

// eth_getTransactionCount auf einem Knoten, dessen evm_nonces nichts weiss
// (nur lesend), muss trotzdem die Nonce aus der Kette liefern.
func TestStufe1_TransactionCountFolgtDerKette(t *testing.T) {
	cs := newTestState()
	a := neuerTestSchluessel(t)
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(10), NaechsteNonce: 7})
	cs.mu.Unlock()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	p, _ := json.Marshal(a.addr)
	got, rerr := srv.getTransactionCount([]json.RawMessage{p})
	if rerr != nil || got != "0x7" {
		t.Fatalf("eth_getTransactionCount = %v (%v), erwartet 0x7", got, rerr)
	}
}
