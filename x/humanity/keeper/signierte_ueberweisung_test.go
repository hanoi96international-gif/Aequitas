package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// Stufe 1.0: jede Ueberweisung ist von jedem Validator nachpruefbar.
// Siehe signierte_ueberweisung.go und docs/SKALIERUNG_DEZENTRAL.md.

type testSchluessel struct {
	key  *ecdsa.PrivateKey
	addr string
}

func neuerTestSchluessel(t *testing.T) testSchluessel {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return testSchluessel{key: k, addr: strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())}
}

func aeqWei(aeq int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(aeq), weiPerAEQ)
}

// signiere baut eine Ueberweisung genau so, wie sendRawTransaction sie in den
// Block schreibt: Absender, Empfaenger, Betrag, TxHash und die Rohform.
func signiere(t *testing.T, von testSchluessel, an string, wei *big.Int, nonce uint64, chainID int64) Transaction {
	t.Helper()
	to := common.HexToAddress(an)
	inner := &types.DynamicFeeTx{
		ChainID: big.NewInt(chainID), Nonce: nonce, To: &to, Value: wei,
		Gas: 21000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
	}
	stx, err := types.SignNewTx(von.key, types.LatestSignerForChainID(big.NewInt(chainID)), inner)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := stx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{
		Type: "transfer", Wallet: von.addr, To: strings.ToLower(to.Hex()),
		Amount: betragAusWei(wei), TxHash: stx.Hash().Hex(), Roh: "0x" + hex.EncodeToString(raw),
	}
}

// signiereV7 baut transfer(address,uint256) an den V7-Vertrag.
func signiereV7(t *testing.T, von testSchluessel, an string, wei *big.Int, nonce uint64) Transaction {
	t.Helper()
	vertrag := common.HexToAddress(V7_CONTRACT_ADDR)
	data := make([]byte, 68)
	copy(data[:4], []byte{0xa9, 0x05, 0x9c, 0xbb})
	copy(data[16:36], common.HexToAddress(an).Bytes())
	wei.FillBytes(data[36:68])
	inner := &types.DynamicFeeTx{
		ChainID: big.NewInt(1926), Nonce: nonce, To: &vertrag, Value: big.NewInt(0), Data: data,
		Gas: 60000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
	}
	stx, err := types.SignNewTx(von.key, types.LatestSignerForChainID(big.NewInt(1926)), inner)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := stx.MarshalBinary()
	return Transaction{
		Type: "transfer", Wallet: von.addr, To: strings.ToLower(common.HexToAddress(an).Hex()),
		Amount: betragAusWei(wei), TxHash: stx.Hash().Hex(), Roh: "0x" + hex.EncodeToString(raw),
	}
}

func TestSignierteUeberweisung_EchteWirdAngenommen(t *testing.T) {
	a := neuerTestSchluessel(t)
	b := neuerTestSchluessel(t)
	for name, tx := range map[string]Transaction{
		"einfach": signiere(t, a, b.addr, aeqWei(5), 7, 1926),
		"v7":      signiereV7(t, a, b.addr, aeqWei(5), 7),
	} {
		n, err := pruefeUeberweisungsRoh(&tx)
		if err != nil {
			t.Fatalf("%s: echte Ueberweisung abgelehnt: %v", name, err)
		}
		if n != 7 {
			t.Fatalf("%s: Nonce %d statt 7", name, n)
		}
	}
}

func TestSignierteUeberweisung_FaelschungenWerdenAbgelehnt(t *testing.T) {
	opfer := neuerTestSchluessel(t)
	angreifer := neuerTestSchluessel(t)
	empf := neuerTestSchluessel(t)
	echt := signiere(t, opfer, empf.addr, aeqWei(5), 1, 1926)

	faelle := map[string]func() Transaction{
		"fremdes Konto belastet": func() Transaction {
			// Der Angreifer signiert selbst, traegt aber das Opfer als Absender ein.
			tx := signiere(t, angreifer, angreifer.addr, aeqWei(500), 1, 1926)
			tx.Wallet = opfer.addr
			return tx
		},
		"Empfaenger getauscht": func() Transaction { tx := echt; tx.To = angreifer.addr; return tx },
		"Betrag erhoeht":       func() Transaction { tx := echt; tx.Amount = echt.Amount * 100; return tx },
		"TxHash getauscht":     func() Transaction { tx := echt; tx.TxHash = "0x" + strings.Repeat("ab", 32); return tx },
		"ohne Rohform":         func() Transaction { tx := echt; tx.Roh = ""; return tx },
		"Rohform kaputt":       func() Transaction { tx := echt; tx.Roh = "0x00ff"; return tx },
		"fremde Chain-ID":      func() Transaction { return signiere(t, opfer, empf.addr, aeqWei(5), 1, 1) },
	}
	for name, bau := range faelle {
		tx := bau()
		if _, err := pruefeUeberweisungsRoh(&tx); !errors.Is(err, ErrUeberweisungNichtSigniert) {
			t.Fatalf("%s: nicht abgelehnt (err=%v)", name, err)
		}
	}
}

// nachspielKnoten: ein Knoten, der Bloecke nachspielt, mit einem Konto, das
// Geld hat, und aktivierter Pruefung.
func nachspielKnoten(t *testing.T, konten map[string]float64) (*BlockDAG, *ChainState) {
	t.Helper()
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(0) })
	dag, cs := newDeterminismTestDAG()
	cs.mu.Lock()
	for a, bal := range konten {
		cs.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(bal)})
	}
	cs.mu.Unlock()
	return dag, cs
}

func testBlock(n int, txs ...Transaction) *Block {
	return &Block{Height: int64(n), Hash: fmt.Sprintf("0xsig-%d", n), Timestamp: nowUnix(), Transactions: txs}
}

func kontoVon(t *testing.T, cs *ChainState, a string) AccountState {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	acc, ok := cs.accounts.Get(a)
	if !ok {
		return AccountState{}
	}
	return *acc
}

func TestSignierteUeberweisung_NachspielenPrueftUndSetztNonce(t *testing.T) {
	a := neuerTestSchluessel(t)
	b := neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, map[string]float64{a.addr: 1000})

	tx := signiere(t, a, b.addr, aeqWei(10), 41, 1926)
	if ok := dag.replayTransactions(testBlock(1, tx), true); !ok {
		t.Fatal("gueltiger Block abgelehnt")
	}
	if got := kontoVon(t, cs, a.addr); got.NaechsteNonce != 42 {
		t.Fatalf("NaechsteNonce %d statt 42", got.NaechsteNonce)
	}
	if got := kontoVon(t, cs, b.addr).Balance.Float(); got != 10 {
		t.Fatalf("Empfaenger hat %.6f statt 10", got)
	}

	// Dieselbe signierte Ueberweisung noch einmal, in einem neuen Block: das
	// ist das erneute Einreichen, vor dem die Nonce schuetzt.
	if ok := dag.replayTransactions(testBlock(2, tx), true); ok {
		t.Fatal("wiederholte Ueberweisung (Nonce 41 verbraucht) wurde angenommen")
	}
	if got := kontoVon(t, cs, b.addr).Balance.Float(); got != 10 {
		t.Fatalf("nach der abgelehnten Wiederholung hat der Empfaenger %.6f statt 10", got)
	}
	if got := kontoVon(t, cs, a.addr); got.NaechsteNonce != 42 {
		t.Fatalf("abgelehnter Block hat NaechsteNonce veraendert: %d", got.NaechsteNonce)
	}
}

func TestSignierteUeberweisung_GefaelschterBlockAendertNichts(t *testing.T) {
	opfer := neuerTestSchluessel(t)
	angreifer := neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, map[string]float64{opfer.addr: 5000})

	// Der Erzeuger schreibt eine Ueberweisung vom Opfer an sich selbst in den
	// Block -- signiert hat er sie mit seinem eigenen Schluessel.
	faelschung := signiere(t, angreifer, angreifer.addr, aeqWei(4000), 0, 1926)
	faelschung.Wallet = opfer.addr
	if ok := dag.replayTransactions(testBlock(1, faelschung), true); ok {
		t.Fatal("Block mit gefaelschter Ueberweisung wurde angenommen")
	}
	if got := kontoVon(t, cs, opfer.addr).Balance.Float(); got != 5000 {
		t.Fatalf("Opfer hat %.6f statt 5000", got)
	}
	if got := kontoVon(t, cs, angreifer.addr).Balance.Float(); got != 0 {
		t.Fatalf("Angreifer hat %.6f statt 0", got)
	}
	if ungueltigeSignaturBloecke.Load() == 0 {
		t.Fatal("abgelehnter Block nicht gezaehlt")
	}
}

func TestSignierteUeberweisung_NoncenImBlockMuessenSteigen(t *testing.T) {
	a := neuerTestSchluessel(t)
	b := neuerTestSchluessel(t)
	c := neuerTestSchluessel(t)

	dag, cs := nachspielKnoten(t, map[string]float64{a.addr: 1000})
	falsch := testBlock(1, signiere(t, a, b.addr, aeqWei(1), 5, 1926), signiere(t, a, c.addr, aeqWei(1), 3, 1926))
	if ok := dag.replayTransactions(falsch, true); ok {
		t.Fatal("Block mit fallender Nonce desselben Absenders angenommen")
	}

	dag2, cs2 := nachspielKnoten(t, map[string]float64{a.addr: 1000})
	richtig := testBlock(1, signiere(t, a, b.addr, aeqWei(1), 5, 1926), signiere(t, a, c.addr, aeqWei(1), 6, 1926))
	if ok := dag2.replayTransactions(richtig, true); !ok {
		t.Fatal("Block mit steigenden Nonces abgelehnt")
	}
	if got := kontoVon(t, cs2, a.addr).NaechsteNonce; got != 7 {
		t.Fatalf("NaechsteNonce %d statt 7", got)
	}
	_ = cs
}

func TestSignierteUeberweisung_VorDerAktivierungUnveraendert(t *testing.T) {
	// Ohne Aktivierung gilt die alte Welt: kein Roh noetig, keine Nonce.
	signierteUeberweisungenOverride.Store(0)
	a := neuerTestSchluessel(t)
	b := neuerTestSchluessel(t)
	dag, cs := newDeterminismTestDAG()
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(100)})
	cs.mu.Unlock()
	alt := Transaction{Type: "transfer", Wallet: a.addr, To: b.addr, Amount: 3, TxHash: "0xalt"}
	if ok := dag.replayTransactions(testBlock(1, alt), true); !ok {
		t.Fatal("alter Block ohne Rohform vor der Aktivierung abgelehnt")
	}
	if got := kontoVon(t, cs, a.addr).NaechsteNonce; got != 0 {
		t.Fatalf("NaechsteNonce vor der Aktivierung gesetzt: %d", got)
	}
}

func TestAccountLeaf_NaechsteNonceNurWennGesetzt(t *testing.T) {
	ohne := &AccountState{Address: "0xabc", Balance: NewDecimal(12), IsHuman: true}
	mit := *ohne
	mit.NaechsteNonce = 3
	if accountLeaf(ohne) == accountLeaf(&mit) {
		t.Fatal("NaechsteNonce geht nicht in den Blattwert ein -- Knoten koennten unbemerkt verschiedene Noncen fuehren")
	}
	null := *ohne
	null.NaechsteNonce = 0
	if accountLeaf(ohne) != accountLeaf(&null) {
		t.Fatal("NaechsteNonce 0 aendert den Blattwert -- alte Bloecke ergaeben einen anderen StateRoot")
	}
}

func TestSignierteUeberweisung_AnnahmeSetztDieselbeNonceWieNachspielen(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(0) })
	a := neuerTestSchluessel(t)
	b := neuerTestSchluessel(t)
	tx := signiere(t, a, b.addr, aeqWei(10), 9, 1926)

	// Annehmender Knoten (ohne Datenbank: transferAtomicDirect).
	annehmer := newTestState()
	annehmer.mu.Lock()
	annehmer.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	annehmer.mu.Unlock()
	if err := annehmer.pruefeAnnahmeNonce(a.addr, 9); err != nil {
		t.Fatalf("Annahme lehnt gueltige Nonce ab: %v", err)
	}
	vorlage := Transaction{Type: "transfer", Wallet: a.addr, To: b.addr, Amount: tx.Amount, TxHash: tx.TxHash, Roh: tx.Roh}
	if _, _, err := annehmer.TransferAtomic(a.addr, b.addr, tx.Amount, vorlage); err != nil {
		t.Fatalf("Annahme: %v", err)
	}
	if got := kontoVon(t, annehmer, a.addr).NaechsteNonce; got != 10 {
		t.Fatalf("Annahme: NaechsteNonce %d statt 10", got)
	}
	if err := annehmer.pruefeAnnahmeNonce(a.addr, 9); err == nil {
		t.Fatal("Annahme nimmt dieselbe Nonce ein zweites Mal an")
	}

	// Nachspielender Knoten.
	dag, nachspieler := nachspielKnoten(t, map[string]float64{a.addr: 1000})
	if ok := dag.replayTransactions(testBlock(1, tx), true); !ok {
		t.Fatal("Nachspielen lehnt die angenommene Ueberweisung ab")
	}
	if got := kontoVon(t, nachspieler, a.addr).NaechsteNonce; got != 10 {
		t.Fatalf("Nachspielen: NaechsteNonce %d statt 10", got)
	}
}
