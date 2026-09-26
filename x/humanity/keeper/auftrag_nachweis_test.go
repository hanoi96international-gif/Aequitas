package keeper

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// persoenlichSignieren: was eine Wallet bei personal_sign liefert (V = 27/28).
func persoenlichSignieren(t *testing.T, k testSchluessel, msg string) string {
	t.Helper()
	h := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)))
	sig, err := crypto.Sign(h, k.key)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	return "0x" + hex.EncodeToString(sig)
}

// signierterTausch baut einen Tausch so, wie handleSwap ihn in den Block schreibt.
func signierterTausch(t *testing.T, k testSchluessel, betrag, aus float64, nonce, zeit int64) Transaction {
	t.Helper()
	msg := fmt.Sprintf("Aequitas Swap: %s %.8f nonce:%d ts:%d", "aeq_to_tusd", betrag, nonce, zeit)
	return Transaction{Type: "swap_aeq_tusd", Wallet: k.addr, Amount: betrag, AmountOut: aus,
		Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, k, msg), Nonce: nonce, Zeit: zeit, Betrag: betrag}}
}

func TestAuftragsNachweis_JedeArtPruefbarUndFaelschungenFallen(t *testing.T) {
	a, b, c := neuerTestSchluessel(t), neuerTestSchluessel(t), neuerTestSchluessel(t)
	jetzt := int64(1_800_000_000)
	sig := func(k testSchluessel, msg string) string { return persoenlichSignieren(t, k, msg) }

	eroeffnen := unternehmenEroeffnenNachricht(a.addr, b.addr, "Laden", "handel", jetzt)
	mitinhaber := unternehmenMitinhaberNachricht(a.addr, c.addr, jetzt)
	gueltig := []Transaction{
		signierterTausch(t, a, 12.5, 11, 3, jetzt),
		{Type: "add_liquidity", Wallet: a.addr, Amount: 10, AmountOut: 20, Nachweis: &Auftragsnachweis{
			Sig:   sig(a, fmt.Sprintf("Aequitas Add Liquidity: %.8f AEQ + %.8f tUSD nonce:%d ts:%d", 10.0, 20.0, int64(4), jetzt)),
			Nonce: 4, Zeit: jetzt, Betrag: 10, Betrag2: 20}},
		{Type: "remove_liquidity", Wallet: a.addr, Amount: 0.5, Nachweis: &Auftragsnachweis{
			Sig: sig(a, fmt.Sprintf("Aequitas Remove Liquidity: %.8f shares nonce:%d ts:%d", 0.5, int64(5), jetzt)), Nonce: 5, Zeit: jetzt, Betrag: 0.5}},
		{Type: "faucet", Wallet: a.addr, Amount: 100, Nachweis: &Auftragsnachweis{
			Sig: sig(a, fmt.Sprintf("Aequitas tUSD Faucet Claim: %s ts:%d", a.addr, jetzt)), Zeit: jetzt}},
		{Type: "escrow_recover", Wallet: a.addr, Amount: 7, Nachweis: &Auftragsnachweis{Sig: sig(a, "Aequitas: recover escrow "+a.addr)}},
		{Type: "unternehmen_eroeffnen", Wallet: a.addr, To: b.addr, Name: "Laden", Kategorie: "handel", Nachweis: &Auftragsnachweis{
			Sig: sig(a, eroeffnen), Sig2: sig(b, eroeffnen), Zeit: jetzt}},
		{Type: "unternehmen_mitinhaber", Wallet: a.addr, To: c.addr, Nachweis: &Auftragsnachweis{
			Sig: sig(c, mitinhaber), Sig2: sig(b, mitinhaber), Von2: b.addr, Zeit: jetzt}},
		{Type: "unternehmen_schliessen", Wallet: a.addr, To: b.addr, Nachweis: &Auftragsnachweis{
			Sig: sig(b, unternehmenSchliessenNachricht(a.addr, jetzt)), Zeit: jetzt}},
	}
	for _, tx := range gueltig {
		tx := tx
		if err := pruefeAuftragsNachweis(&tx, jetzt+10); err != nil {
			t.Fatalf("%s: gueltiger Nachweis abgelehnt: %v", tx.Type, err)
		}
	}

	faelschungen := map[string]func(tx Transaction) Transaction{
		"ohne Nachweis":        func(tx Transaction) Transaction { tx.Nachweis = nil; return tx },
		"anderer Auftraggeber": func(tx Transaction) Transaction { tx.Wallet = c.addr; return tx },
		"anderer Betrag":       func(tx Transaction) Transaction { tx.Amount *= 2; return tx },
		"Nachweis-Betrag mit": func(tx Transaction) Transaction {
			n := *tx.Nachweis
			n.Betrag *= 2
			tx.Amount = n.Betrag
			tx.Nachweis = &n
			return tx
		},
		"andere Nonce":           func(tx Transaction) Transaction { n := *tx.Nachweis; n.Nonce++; tx.Nachweis = &n; return tx },
		"Zeit zu alt fuer Block": func(tx Transaction) Transaction { n := *tx.Nachweis; n.Zeit -= 7200; tx.Nachweis = &n; return tx },
	}
	for name, f := range faelschungen {
		tx := f(gueltig[0])
		if err := pruefeAuftragsNachweis(&tx, jetzt+10); !errors.Is(err, ErrUeberweisungNichtSigniert) {
			t.Errorf("Tausch, %s: angenommen (%v)", name, err)
		}
	}
	// Unternehmen: die zweite Unterschrift von jemand anderem.
	u := gueltig[5]
	n := *u.Nachweis
	n.Sig2 = sig(c, eroeffnen)
	u.Nachweis = &n
	if err := pruefeAuftragsNachweis(&u, jetzt); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Errorf("Unternehmen mit fremder zweiter Unterschrift angenommen: %v", err)
	}
	// Zeit zu weit in der Zukunft des Blocks.
	f := gueltig[3]
	if err := pruefeAuftragsNachweis(&f, jetzt-nachweisHoechstensVoraus-1); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Errorf("Faucet aus der Zukunft angenommen: %v", err)
	}
}

func tauschKnoten(t *testing.T, konten map[string]float64) (*BlockDAG, *ChainState) {
	t.Helper()
	dag, cs := nachspielKnoten(t, konten)
	cs.mu.Lock()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000), TotalLPShares: NewDecimal(1000)}
	cs.mu.Unlock()
	return dag, cs
}

func TestAuftragsNachweis_NachspielenPrueftUndSetztNonce(t *testing.T) {
	a := neuerTestSchluessel(t)
	dag, cs := tauschKnoten(t, map[string]float64{a.addr: 1000})

	tx := signierterTausch(t, a, 10, 9.9, 7, nowUnix())
	if ok := dag.replayTransactions(testBlock(1, tx), true); !ok {
		t.Fatal("gueltiger Tausch abgelehnt")
	}
	if got := kontoVon(t, cs, a.addr); got.NaechsteAuftragsNonce != 8 || got.Balance.Float() != 990 {
		t.Fatalf("nach dem Tausch: NaechsteAuftragsNonce %d (statt 8), Guthaben %.6f (statt 990)", got.NaechsteAuftragsNonce, got.Balance.Float())
	}
	// Derselbe unterschriebene Tausch noch einmal.
	if ok := dag.replayTransactions(testBlock(2, tx), true); ok {
		t.Fatal("wiederholter Tausch (Nonce 7 verbraucht) angenommen")
	}
	if got := kontoVon(t, cs, a.addr).Balance.Float(); got != 990 {
		t.Fatalf("abgelehnter Block hat das Guthaben veraendert: %.6f", got)
	}
}

func TestAuftragsNachweis_GefaelschterTauschAendertNichts(t *testing.T) {
	opfer, angreifer := neuerTestSchluessel(t), neuerTestSchluessel(t)
	dag, cs := tauschKnoten(t, map[string]float64{opfer.addr: 5000})
	faelschung := signierterTausch(t, angreifer, 4000, 3900, 0, nowUnix())
	faelschung.Wallet = opfer.addr
	if ok := dag.replayTransactions(testBlock(1, faelschung), true); ok {
		t.Fatal("Tausch mit fremder Unterschrift angenommen")
	}
	ohne := Transaction{Type: "swap_aeq_tusd", Wallet: opfer.addr, Amount: 4000, AmountOut: 3900}
	if ok := dag.replayTransactions(testBlock(2, ohne), true); ok {
		t.Fatal("Tausch ohne Nachweis angenommen")
	}
	if got := kontoVon(t, cs, opfer.addr).Balance.Float(); got != 5000 {
		t.Fatalf("Opfer hat %.6f statt 5000", got)
	}
}

func TestAuftragsNachweis_VorDerAktivierungUnveraendert(t *testing.T) {
	a := neuerTestSchluessel(t)
	dag, cs := tauschKnoten(t, map[string]float64{a.addr: 1000})
	signierteUeberweisungenOverride.Store(math.MaxInt64) // aus
	alt := Transaction{Type: "swap_aeq_tusd", Wallet: a.addr, Amount: 10, AmountOut: 9.9}
	if ok := dag.replayTransactions(testBlock(1, alt), true); !ok {
		t.Fatal("alter Tausch ohne Nachweis vor der Aktivierung abgelehnt")
	}
	if got := kontoVon(t, cs, a.addr); got.NaechsteAuftragsNonce != 0 {
		t.Fatalf("vor der Aktivierung NaechsteAuftragsNonce gesetzt: %d", got.NaechsteAuftragsNonce)
	}
}

func TestAuftragsNachweis_MitinhaberNurMitVerantwortlichem(t *testing.T) {
	wirtschaftAn(t)
	firma, inhaber, fremd, neu := neuerTestSchluessel(t), neuerTestSchluessel(t), neuerTestSchluessel(t), neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, nil)
	addHuman(cs, inhaber.addr, 100)
	addHuman(cs, neu.addr, 100)
	addHuman(cs, fremd.addr, 100)
	jetzt := nowUnix()
	eroeffnen := unternehmenEroeffnenNachricht(firma.addr, inhaber.addr, "Laden", "handel", jetzt)
	b1 := Transaction{Type: "unternehmen_eroeffnen", Wallet: firma.addr, To: inhaber.addr, Name: "Laden", Kategorie: "handel",
		Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, firma, eroeffnen), Sig2: persoenlichSignieren(t, inhaber, eroeffnen), Zeit: jetzt}}
	if ok := dag.replayTransactions(testBlock(1, b1), true); !ok {
		t.Fatal("Eroeffnen abgelehnt")
	}
	// Ein Fremder unterschreibt als "bisher verantwortlich": Unterschrift
	// gueltig, aber er ist es nicht.
	msg := unternehmenMitinhaberNachricht(firma.addr, neu.addr, jetzt)
	b2 := Transaction{Type: "unternehmen_mitinhaber", Wallet: firma.addr, To: neu.addr, Nachweis: &Auftragsnachweis{
		Sig: persoenlichSignieren(t, neu, msg), Sig2: persoenlichSignieren(t, fremd, msg), Von2: fremd.addr, Zeit: jetzt}}
	if ok := dag.replayTransactions(testBlock(2, b2), true); ok {
		t.Fatal("Mitinhaber mit Zustimmung eines Nicht-Verantwortlichen angenommen")
	}
	b3 := b2
	n := *b2.Nachweis
	n.Sig2, n.Von2 = persoenlichSignieren(t, inhaber, msg), inhaber.addr
	b3.Nachweis = &n
	if ok := dag.replayTransactions(testBlock(3, b3), true); !ok {
		t.Fatal("Mitinhaber mit Zustimmung des Verantwortlichen abgelehnt")
	}
	cs.wirt().mu.Lock()
	e := cs.wirt().offenesUnternehmenLocked(firma.addr)
	ok := e != nil && e.istVerantwortlich(neu.addr)
	cs.wirt().mu.Unlock()
	if !ok {
		t.Fatal("neuer Mitinhaber nicht eingetragen")
	}
}

// Die Annahme setzt dieselbe Nonce wie das Nachspielen und lehnt eine
// verbrauchte ab -- sonst erzeugte sie Bloecke, die alle anderen verwerfen.
func TestAuftragsNachweis_AnnahmeSetztUndPrueftNonce(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	a := neuerTestSchluessel(t)
	cs := newTestState()
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000), TotalLPShares: NewDecimal(1000)}
	cs.mu.Unlock()

	tx := signierterTausch(t, a, 10, 0, 5, nowUnix())
	tmpl := Transaction{Type: tx.Type, Wallet: a.addr, Amount: 10, Nachweis: nachweisFuerAnnahme(*tx.Nachweis)}
	if tmpl.Nachweis == nil {
		t.Fatal("aktiviert, aber kein Nachweis in der Vorlage")
	}
	if _, _, err := cs.SwapAtomic(a.addr, 10, true, 0, tmpl); err != nil {
		t.Fatalf("Tausch: %v", err)
	}
	if got := kontoVon(t, cs, a.addr).NaechsteAuftragsNonce; got != 6 {
		t.Fatalf("NaechsteAuftragsNonce %d statt 6", got)
	}
	if err := cs.pruefeAuftragsNonce(a.addr, tmpl.Nachweis); err == nil {
		t.Fatal("verbrauchte Nonce 5 besteht die Vorpruefung")
	}
	neu := *tmpl.Nachweis
	neu.Nonce = 6
	if err := cs.pruefeAuftragsNonce(a.addr, &neu); err != nil {
		t.Fatalf("naechste Nonce 6 abgelehnt: %v", err)
	}
}

// Die lokale swap_nonces-Tabelle kennt nur, was DIESER Knoten angenommen hat.
// Nimmt im naechsten Term ein anderer an (Stufe 2), muss er die Nonce aus dem
// gemeinsamen Zustand nennen und annehmen -- sonst signierte die App eine
// laengst verbrauchte.
func TestAuftragsNonce_LokaleTabelleFolgtDemGemeinsamenZustand_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-auftragsnonce-realdb-test.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	w := distTestAddr(1600)
	cs.db.Exec(`DELETE FROM swap_nonces WHERE wallet_address = $1`, w)
	cs.mu.Lock()
	acc := &AccountState{Address: w, Balance: NewDecimal(10), NaechsteAuftragsNonce: 5}
	cs.accounts.Set(w, acc)
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	cs.mu.Unlock()
	if got := cs.GetSwapNonce(w); got != 5 {
		t.Fatalf("GetSwapNonce %d statt 5", got)
	}
	if err := cs.ConsumeSwapNonce(w, 3); err == nil {
		t.Fatal("verbrauchte Nonce 3 angenommen")
	}
	if err := cs.ConsumeSwapNonce(w, 5); err != nil {
		t.Fatalf("Nonce 5 abgelehnt: %v", err)
	}
	if got := cs.GetSwapNonce(w); got != 6 {
		t.Fatalf("danach %d statt 6", got)
	}
}
