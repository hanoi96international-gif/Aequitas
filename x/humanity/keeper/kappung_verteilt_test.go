package keeper

import (
	"fmt"
	"testing"
)

func stufe2An(t *testing.T) {
	t.Helper()
	verteilteAnnahmeOverride.Store(1)
	t.Cleanup(func() { verteilteAnnahmeOverride.Store(0) })
}

// kappungsKnoten: 40 Menschen (Grenze 25.000), Y knapp unter der Grenze.
func kappungsKnoten(t *testing.T, name string) *annahmeKnoten {
	t.Helper()
	dag, cs := newDeterminismTestDAG()
	seedHumanAccounts(cs, 40, 1000)
	cs.mu.Lock()
	for a, b := range map[string]float64{"0xy": 24_000, "0xx": 5_000, "0xz": 100} {
		cs.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(b), LastActivityAt: nowUnix()})
	}
	cs.mu.Unlock()
	k := &annahmeKnoten{name: name, dag: dag, cs: cs}
	cs.ausgangOhneDB = func(tx Transaction) { k.korb = append(k.korb, tx) }
	return k
}

func (k *annahmeKnoten) block(t *testing.T, n int) *Block {
	t.Helper()
	b := &Block{Height: int64(n), Hash: fmt.Sprintf("0xkap-%s-%d", k.name, n), Timestamp: nowUnix(), Transactions: k.korb}
	k.cs.gebuehrenInsGrundeinkommen(gebuehrenSumme(k.korb))
	k.korb = nil
	return b
}

func TestKappungVerteilt_GutschriftKapptNichtDerZustaendigeSchon(t *testing.T) {
	stufe2An(t)
	a := kappungsKnoten(t, "a")
	if _, _, err := a.cs.TransferAtomic("0xx", "0xy", 2_000, Transaction{Type: "transfer", Wallet: "0xx", To: "0xy", Amount: 2_000, TxHash: "0x1"}); err != nil {
		t.Fatal(err)
	}
	if got := stand(a.cs, "0xy"); got != 26_000 {
		t.Fatalf("Gutschrift kappte selbst: Y hat %.6f statt 26.000", got)
	}
	if n := a.cs.KappungenAbarbeiten(); n != 1 {
		t.Fatalf("%d Kappungen statt 1", n)
	}
	if got := stand(a.cs, "0xy"); got != 25_000 {
		t.Fatalf("nach der Kappung hat Y %.6f statt 25.000", got)
	}
	var kap *Transaction
	for i := range a.korb {
		if a.korb[i].Type == "kappung" {
			kap = &a.korb[i]
		}
	}
	if kap == nil || kap.Wallet != "0xy" || kap.Amount != 1_000 {
		t.Fatalf("keine Kappungs-Transaktion ueber 1.000 im Ausgang: %+v", a.korb)
	}
	if n := a.cs.KappungenAbarbeiten(); n != 0 {
		t.Fatalf("zweite Runde kappte erneut (%d)", n)
	}
}

// Y gehoert Knoten A. A nimmt eine Ausgabe von Y an, B gleichzeitig eine
// Gutschrift an Y, die Y ueber die Grenze hebt. Ohne Stufe-2-Kappung kappte B
// gegen SEINEN Stand von Y (ohne die Ausgabe), jeder andere Knoten in seiner
// Reihenfolge anders. Mit ihr: B kappt nicht, A kappt mit festem Betrag, und
// alle drei Knoten landen -- in verschiedener Reihenfolge -- beim selben
// Zustand.
func TestKappungVerteilt_DreiKnotenGleicherZustand(t *testing.T) {
	stufe2An(t)
	a, b, c := kappungsKnoten(t, "a"), kappungsKnoten(t, "b"), kappungsKnoten(t, "c")

	// Gleichzeitig: A (zustaendig fuer Y) nimmt Y -> Z an, B nimmt X -> Y an.
	if _, _, err := a.cs.TransferAtomic("0xy", "0xz", 500, Transaction{Type: "transfer", Wallet: "0xy", To: "0xz", Amount: 500, TxHash: "0xa1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.cs.TransferAtomic("0xx", "0xy", 2_000, Transaction{Type: "transfer", Wallet: "0xx", To: "0xy", Amount: 2_000, TxHash: "0xb1"}); err != nil {
		t.Fatal(err)
	}
	blkA1, blkB1 := a.block(t, 1), b.block(t, 2)

	// A spielt B nach -- die Gutschrift merkt Y vor; A ist zustaendig und kappt.
	if !a.dag.replayTransactions(blkB1, true) {
		t.Fatal("A lehnte B's Block ab")
	}
	if n := a.cs.KappungenAbarbeiten(); n != 1 {
		t.Fatalf("A kappte %d-mal statt einmal", n)
	}
	blkA2 := a.block(t, 3)
	// B ist nicht zustaendig: es kappt nicht (im Test: ruft nicht ab).
	if !b.dag.replayTransactions(blkA1, true) || !b.dag.replayTransactions(blkA2, true) {
		t.Fatal("B lehnte A's Bloecke ab")
	}
	// C in anderer Reihenfolge: B1 vor A1.
	for _, blk := range []*Block{blkB1, blkA1, blkA2} {
		if !c.dag.replayTransactions(blk, true) {
			t.Fatalf("C lehnte %s ab", blk.Hash)
		}
	}
	for _, konto := range []string{"0xx", "0xy", "0xz", ubiPoolAddr, validatorsPoolAddr, lpPoolAddr, treasuryPoolAddr} {
		sa, sb, sc := stand(a.cs, konto), stand(b.cs, konto), stand(c.cs, konto)
		if sa != sb || sb != sc {
			t.Errorf("%s: A %.6f, B %.6f, C %.6f", konto, sa, sb, sc)
		}
	}
	if ra, rb, rc := a.cs.StateRoot(), b.cs.StateRoot(), c.cs.StateRoot(); ra != rb || rb != rc {
		t.Fatalf("StateRoot weicht ab:\n A %s\n B %s\n C %s", ra, rb, rc)
	}
	// Y = 24.000 - 500 + 2.000 = 25.500, gekappt auf 25.000.
	if got := stand(a.cs, "0xy"); got != 25_000 {
		t.Fatalf("Y hat %.6f statt 25.000", got)
	}
}

// Vor der Aktivierung kappt die Gutschrift wie immer selbst.
func TestKappungVerteilt_VorDerAktivierungUnveraendert(t *testing.T) {
	a := kappungsKnoten(t, "a")
	if _, _, err := a.cs.TransferAtomic("0xx", "0xy", 2_000, Transaction{Type: "transfer", Wallet: "0xx", To: "0xy", Amount: 2_000, TxHash: "0x1"}); err != nil {
		t.Fatal(err)
	}
	if got := stand(a.cs, "0xy"); got != 25_000 {
		t.Fatalf("vor der Aktivierung nicht sofort gekappt: %.6f", got)
	}
	if n := a.cs.KappungenAbarbeiten(); n != 0 {
		t.Fatalf("vor der Aktivierung %d Kappungs-Transaktionen", n)
	}
}
