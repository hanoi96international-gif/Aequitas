package keeper

import (
	"context"
	"errors"
	"testing"
)

// Missbrauchstests aus der getrennten Sicherheitspruefung des Diffs zum
// Audit 2026-10-21 (F1-F6).

// F1: Unternehmen -> Mensch mit Gebuehr. Das serielle Nachrechnen lehnt sie
// im strengen Modus ab; der schnelle Pfad (parallelReplayMinBatch = 1) nahm
// sie vorher ungeprueft, und der Block galt.
func TestSicherheit_LohnGebuehrNichtUeberDenSchnellenPfad(t *testing.T) {
	nachrechnenModus(t, true)
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()
	dag, cs := newDeterminismTestDAG()
	addHuman(cs, auditB, 1000)
	cs.accounts.Set(auditU, &AccountState{Address: auditU, Balance: NewDecimal(5000), LastActivityAt: jetzt})
	cs.wirt().unternehmen[auditU] = &unternehmenEintrag{Adresse: auditU, Verantwortliche: []string{auditB}, EroeffnetAm: 1}
	acct(cs, auditB).LastActivityAt = jetzt
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(10), LastActivityAt: jetzt})
	lohn := Transaction{Type: "transfer", Wallet: auditU, To: auditB, Amount: 1000, Gebuehr: grundGebuehr(1000), BuchAt: jetzt}

	cs.mu.Lock()
	serr := cs.nachrechnenTxLocked(&lohn, jetzt)
	cs.mu.Unlock()
	if !errors.Is(serr, ErrZustandLehntAb) {
		t.Fatalf("serielles Nachrechnen lehnt nicht ab: %v", serr)
	}
	if b := cs.lohnGebuehrPraefix([]Transaction{lohn}); len(b) != 0 {
		t.Fatalf("Lohn mit Gebuehr bleibt im schnellen Lauf (%d)", len(b))
	}
	vorU := acct(cs, auditU).Balance
	block := &Block{Height: 1, Hash: "0xf1", Timestamp: jetzt, Transactions: []Transaction{lohn}}
	if dag.replayTransactions(block, true) {
		t.Fatalf("Block mit Gebuehr auf einem Lohn angenommen")
	}
	if acct(cs, auditU).Balance != vorU {
		t.Fatalf("Unternehmen belastet trotz Ablehnung: %v -> %v", vorU.Float(), acct(cs, auditU).Balance.Float())
	}

	// Gegenprobe: derselbe Lohn ohne Gebuehr geht durch, und eine
	// Ueberweisung zwischen Menschen bleibt im schnellen Lauf.
	ohne := lohn
	ohne.Gebuehr = 0
	if b := cs.lohnGebuehrPraefix([]Transaction{ohne}); len(b) != 1 {
		t.Fatalf("Lohn ohne Gebuehr aus dem Lauf genommen")
	}
	if !dag.replayTransactions(&Block{Height: 1, Hash: "0xf1b", Timestamp: jetzt, Transactions: []Transaction{ohne}}, true) {
		t.Fatalf("ehrlicher Lohn abgelehnt")
	}
	mensch := Transaction{Type: "transfer", Wallet: auditB, To: auditA, Amount: 10, Gebuehr: grundGebuehr(10)}
	if b := cs.lohnGebuehrPraefix([]Transaction{mensch, lohn}); len(b) != 1 {
		t.Fatalf("Lauf nicht genau vor dem Lohn gekuerzt: %d", len(b))
	}
}

// F3: ein Einsatz von 1 Mikro-AEQ gegen eine kleinere Gegenreserve rundet
// auf null. Die Annahme buchte ihn trotzdem (Einsatz weg, Reserve gewachsen),
// das Nachspielen eines direkten Tauschs ueberspringt Ergebnis null -- die
// Knoten gingen still auseinander. Jetzt weist die Annahme ihn ab.
func TestSicherheit_TauschMitErgebnisNullWirdAbgewiesen(t *testing.T) {
	for _, aeqToTusd := range []bool{true, false} {
		cs := newTestState()
		addHuman(cs, auditA, 1000)
		acct(cs, auditA).TUsdBalance = NewDecimal(10)
		cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(100), TotalLPShares: NewDecimal(100)}
		if !aeqToTusd {
			cs.pool = &PoolState{ReserveAEQ: NewDecimal(100), ReserveTUSD: NewDecimal(1000), TotalLPShares: NewDecimal(100)}
		}
		vorKonto, vorTUSD, vorPool := acct(cs, auditA).Balance, acct(cs, auditA).TUsdBalance, *cs.pool
		cs.mu.Lock()
		out, _, _, err := cs.swapLockedMitAbgabe(context.Background(), auditA, 1e-6, aeqToTusd, 0)
		cs.mu.Unlock()
		if !errors.Is(err, errSwapErgebnisNull) {
			t.Fatalf("aeqToTusd=%v: Tausch mit Ergebnis %v angenommen: %v", aeqToTusd, out, err)
		}
		if acct(cs, auditA).Balance != vorKonto || acct(cs, auditA).TUsdBalance != vorTUSD || *cs.pool != vorPool {
			t.Fatalf("aeqToTusd=%v: Zustand veraendert trotz Abweisung", aeqToTusd)
		}
	}
}

// F3 (Gegenprobe): beim Nachspielen bleibt Ergebnis null erlaubt -- alte
// Bloecke tragen solche Vorbehalts-Ausfuehrungen, und ihr Nachspielen darf
// sich nicht aendern. Negativ bleibt ungueltig.
func TestSicherheit_NachspielenTauschErgebnisNullWieFrueher(t *testing.T) {
	cs := auditPoolZustand()
	cs.mu.Lock()
	err := cs.applySwapDeltaLockedMitAbgabe(context.Background(), auditA, 1e-6, 0, true, 0, 1, 0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("Ergebnis null beim Nachspielen abgelehnt: %v", err)
	}
	cs = auditPoolZustand()
	cs.mu.Lock()
	err = cs.applySwapDeltaLockedMitAbgabe(context.Background(), auditA, 1, -1, true, 0, 1, 0)
	cs.mu.Unlock()
	if !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("negatives Ergebnis angenommen: %v", err)
	}
}

// F2: eine Registrierung ohne Bescheinigung, angenommen vor dem Vorlauf, aber
// noch nicht verblockt, wenn "jetzt" den Stichtag ueberschreitet. Liegen die
// Eltern noch davor, waehlt die Erzeugung eine Blockzeit vor dem Stichtag,
// statt gar keinen Block zu bauen.
func TestSicherheit_AlteRegistrierungBekommtBlockzeitVorDemStichtag(t *testing.T) {
	coord := neuerEd25519(t)
	ab := int64(1_800_000_000)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: ab, Modus: "test", Quorum: 1, Coordinator: []string{coord.pub}})
	alt := Transaction{Type: "register_human", Wallet: bTestWallet, RegAt: ab - 2*86400, Nullifier: "123"}
	tr := Transaction{Type: "transfer", Wallet: auditA, To: auditB, Amount: 1}
	for name, txs := range map[string][]Transaction{
		"Registrierung zuerst": {alt, tr},
		"Registrierung danach": {tr, alt},
	} {
		k, zeit, err := blockZeitPraefix(txs, ab+100, ab-50)
		if err != nil || k != 2 || zeit >= ab {
			t.Fatalf("%s: k=%d Blockzeit=%d (Stichtag %d), err=%v", name, k, zeit, ab, err)
		}
		for i := range txs {
			if err := blockTauglich(&txs[i], zeit); err != nil {
				t.Fatalf("%s: Blockzeit %d nicht tauglich: %v", name, zeit, err)
			}
		}
	}
	// Liegen schon die Eltern nach dem Stichtag, gibt es keine gueltige
	// Blockzeit -- kein Block, den die anderen abweisen. Fail-closed.
	if k, _, err := blockZeitPraefix([]Transaction{alt}, ab+100, ab+50); err == nil && k > 0 {
		t.Fatalf("Registrierung ohne Bescheinigung nach dem Stichtag verblockt")
	}
}

// F2: die Wiederholung (RetryRegistrationRecoveries) bucht ab dem Vorlauf
// keine Registrierung ohne gueltige Bescheinigung mehr ein.
func TestSicherheit_WiederholungOhneBescheinigung(t *testing.T) {
	coord, zeuge := neuerEd25519(t), neuerEd25519(t)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{coord.pub}, Bezeuger: []string{zeuge.pub}})
	gut := bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit))
	gut.Nullifier = gut.PubSignals[1]
	if err := recoveryBescheinigungPruefen(gut); err != nil {
		t.Fatalf("gueltige Bescheinigung abgewiesen: %v", err)
	}
	ohne := gut
	ohne.BioBescheinigung = nil
	ohneNullifier := gut
	ohneNullifier.Nullifier = ""
	falsch := bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, "0xb0d1000000000000000000000000000000000666", bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit))
	falsch.Nullifier = falsch.PubSignals[1]
	for name, tx := range map[string]Transaction{"ohne Bescheinigung": ohne, "ohne Nullifier": ohneNullifier, "fremde Wallet": falsch} {
		if recoveryBescheinigungPruefen(tx) == nil {
			t.Errorf("%s: nachgeholt", name)
		}
	}
}

// F4: ein falsch geschriebener Eintrag liess die Regel still schlafen.
func TestSicherheit_GenesisTippfehlerHaeltAn(t *testing.T) {
	k := neuerEd25519(t)
	for _, name := range []string{"Bescheinigung", "bescheinigungen", "BESCHEINIGUNG"} {
		g := `{"chain_id":"x","` + name + `":{"ab_unix":1800000000,"modus":"real","quorum":1,"coordinator":["` + k.pub + `"]}}`
		if _, err := ladeBescheinigungKonfig([]byte(g)); err == nil {
			t.Errorf("Eintrag %q still ignoriert", name)
		}
	}
	// Gegenprobe: der genaue Name gilt, und ein fremder Eintrag stoert nicht.
	g := `{"chain_id":"x","beschreibung":"egal","bescheinigung":{"ab_unix":1800000000,"modus":"real","quorum":1,"coordinator":["` + k.pub + `"]}}`
	if c, err := ladeBescheinigungKonfig([]byte(g)); err != nil || c.AbUnix != 1800000000 {
		t.Fatalf("genauer Name nicht gelesen: %+v %v", c, err)
	}
}

// F4: ohne genesis.json haelt die Startpruefung an, statt die Regel still
// schlafen zu lassen.
func TestSicherheit_OhneGenesisKeinStart(t *testing.T) {
	t.Chdir(t.TempDir())
	if PruefeBescheinigungGenesis() == nil {
		t.Fatalf("Start ohne genesis.json erlaubt")
	}
}

// F4: verschiedene Schluessellisten haben verschiedene Fingerabdruecke.
func TestSicherheit_FingerabdruckUnterscheidetSchluessel(t *testing.T) {
	a, b := neuerEd25519(t), neuerEd25519(t)
	k1 := bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 1, Coordinator: []string{a.pub}}
	k2 := bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 1, Coordinator: []string{b.pub}}
	if f1, f2 := bescheinigungFingerabdruck(k1), bescheinigungFingerabdruck(k2); f1 == "" || f1 == f2 {
		t.Fatalf("Fingerabdruck unterscheidet nicht: %q %q", f1, f2)
	}
	if bescheinigungFingerabdruck(k1) != bescheinigungFingerabdruck(k1) {
		t.Fatalf("Fingerabdruck nicht stabil")
	}
}
