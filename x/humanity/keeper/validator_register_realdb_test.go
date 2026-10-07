package keeper

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Validator-Register auf der Kette (validator_register.go), gegen eine echte
// Datenbank: das Register steht in validator_register.

type registerFall struct {
	t     *testing.T
	cs    *ChainState
	dag   *BlockDAG
	jetzt int64
	n     int
}

func neuerRegisterFall(t *testing.T) *registerFall {
	t.Helper()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-validator-register-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	// Die Tests der Register-Regeln binden im Minutenabstand; der Abstand je
	// Betreiber (validatorBindungAbstand) hat eigene Tests und steht hier
	// auf einer Sekunde.
	validatorBindungAbstandOverride.Store(1)
	t.Cleanup(func() { validatorBindungAbstandOverride.Store(0) })
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	return &registerFall{t: t, cs: cs, dag: dag, jetzt: nowUnix()}
}

// betreiber: ein registrierter Mensch mit eigenem Schluessel.
func (f *registerFall) betreiber() *ecdsa.PrivateKey {
	f.t.Helper()
	k, w := neuerSchluessel(f.t)
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	acc := &AccountState{Address: w, IsHuman: true, Balance: NewDecimal(10), LastActivityAt: f.jetzt}
	if err := f.cs.saveAccountToDB(acc); err != nil {
		f.t.Fatal(err)
	}
	f.cs.accounts.Set(w, acc)
	f.cs.updateAccountLeafLocked(acc)
	return k
}

func (f *registerFall) block(zeit int64, txs ...Transaction) bool {
	f.t.Helper()
	f.n++
	b := &Block{Height: int64(f.n), Hash: fmt.Sprintf("register-%s-%d", f.t.Name(), f.n), Timestamp: zeit, Transactions: txs}
	return f.dag.replayTransactions(b, true)
}

// eintrag: Signieradresse und Zeitpunkt der Bindung von betreiber.
func (f *registerFall) eintrag(betreiber *ecdsa.PrivateKey) (string, int64, bool) {
	f.t.Helper()
	var s string
	var z int64
	err := f.cs.db.QueryRow(`SELECT signing_address, bindung_ts FROM validator_register WHERE operator_wallet = $1`, adrVon(betreiber)).Scan(&s, &z)
	if err != nil {
		return "", 0, false
	}
	return s, z, true
}

func (f *registerFall) anzahl() int {
	f.t.Helper()
	var n int
	if err := f.cs.db.QueryRow(`SELECT COUNT(*) FROM validator_register`).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *registerFall) summe() [32]byte {
	f.cs.mu.RLock()
	defer f.cs.mu.RUnlock()
	return f.cs.validatorSetXOR
}

// bindungsBlaetter: das Blatt einer Bindung im Register und das ihrer Zeile
// im Verlauf.
func bindungsBlaetter(betreiber, signing string, zeit int64, ueberholt bool) [32]byte {
	x := validatorBlatt(betreiber, signing, zeit, ueberholt)
	xorInto(&x, validatorVerlaufBlatt(betreiber, signing, zeit))
	return x
}

// neuAufgebaut: wie ein frisch gestarteter Knoten die Summe rechnet.
func (f *registerFall) neuAufgebaut() [32]byte {
	f.t.Helper()
	x, err := f.cs.validatorSummeAusDB()
	if err != nil {
		f.t.Fatal(err)
	}
	return x
}

func TestValidatorRegister_OhneEintraegeUnveraendert_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.betreiber()
	if got, alt := f.cs.StateRoot(), stateRootOhneTreuhand(f.cs); got != alt {
		t.Fatalf("ohne Register hat sich die StateRoot geaendert: %s statt %s", got, alt)
	}
	if c := f.cs.StateRootComponentBreakdown(); c.ValidatorSetXOR != "" {
		t.Fatalf("ohne Register meldet die Aufschluesselung eine Summe: %s", c.ValidatorSetXOR)
	}
	leer, _ := json.Marshal(f.cs.ExportSnapshot(nil, 1, false))
	if strings.Contains(string(leer), `"validatoren"`) {
		t.Fatal("Snapshot ohne Register traegt das Feld")
	}
}

// Gutfall: binden, neu binden, die frei gewordene Adresse an einen anderen.
func TestValidatorRegister_BindenUndNeuBinden_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b1, b2 := f.betreiber(), f.betreiber()
	s1, _ := neuerSchluessel(t)
	s2, _ := neuerSchluessel(t)

	t1 := f.jetzt - 120
	if !f.block(f.jetzt, bindungUnterschrieben(t, b1, s1, t1)) {
		t.Fatal("gueltige Bindung abgewiesen")
	}
	if s, z, ok := f.eintrag(b1); !ok || s != adrVon(s1) || z != t1 {
		t.Fatalf("Eintrag nach der Bindung: %s %d %v", s, z, ok)
	}
	if got := f.summe(); got != bindungsBlaetter(adrVon(b1), adrVon(s1), t1, false) {
		t.Fatalf("Summe %x, erwartet das Blatt der Bindung", got[:6])
	}
	if got := f.neuAufgebaut(); got != f.summe() {
		t.Fatalf("neu aufgebaute Summe %x, laufende %x", got[:6], f.summe())
	}
	if got, ohne := f.cs.StateRoot(), stateRootOhneTreuhand(f.cs); got == ohne {
		t.Fatal("die StateRoot traegt das Register nicht")
	}
	if c := f.cs.StateRootComponentBreakdown(); c.ValidatorSetXOR == "" {
		t.Fatal("die Aufschluesselung zeigt das Register nicht")
	}
	// Ein frisch gestarteter Knoten (Summe null) rechnet dieselbe Wurzel.
	vorher := f.cs.StateRoot()
	f.cs.mu.Lock()
	f.cs.validatorSetXOR = [32]byte{}
	f.cs.rebuildStateAccumulators()
	f.cs.mu.Unlock()
	if got := f.cs.StateRoot(); got != vorher {
		t.Fatalf("nach dem Neuaufbau andere StateRoot: %s statt %s", got, vorher)
	}

	t2 := f.jetzt - 60
	if !f.block(f.jetzt, bindungUnterschrieben(t, b1, s2, t2)) {
		t.Fatal("neue Bindung abgewiesen")
	}
	if s, z, _ := f.eintrag(b1); s != adrVon(s2) || z != t2 {
		t.Fatalf("Eintrag nach der neuen Bindung: %s %d", s, z)
	}
	// Im Register nur noch die neue Bindung, im Verlauf beide.
	erwartet := validatorBlatt(adrVon(b1), adrVon(s2), t2, false)
	xorInto(&erwartet, validatorVerlaufBlatt(adrVon(b1), adrVon(s1), t1))
	xorInto(&erwartet, validatorVerlaufBlatt(adrVon(b1), adrVon(s2), t2))
	if got := f.summe(); got != erwartet {
		t.Fatal("die alte Bindung ist nicht aus der Summe heraus")
	}
	// s1 ist frei geworden.
	if !f.block(f.jetzt, bindungUnterschrieben(t, b2, s1, t2)) {
		t.Fatal("Bindung an die frei gewordene Adresse abgewiesen")
	}
	if s, _, ok := f.eintrag(b2); !ok || s != adrVon(s1) {
		t.Fatalf("frei gewordene Adresse nicht gebunden: %s %v", s, ok)
	}
	if got := f.neuAufgebaut(); got != f.summe() {
		t.Fatal("laufende und neu aufgebaute Summe weichen ab")
	}
}

// Missbrauch, der den Block ungueltig macht: Unterschriften, Form, Zeit.
func TestValidatorRegister_UngueltigerBlock_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b := f.betreiber()
	s, _ := neuerSchluessel(t)
	fremd, _ := neuerSchluessel(t)
	z := f.jetzt - 60
	gut := bindungUnterschrieben(t, b, s, z)
	msg := validatorBindungNachricht(gut.To, gut.Wallet, z)

	mit := func(aendern func(tx *Transaction)) Transaction {
		tx := gut
		n := *gut.Nachweis
		tx.Nachweis = &n
		aendern(&tx)
		return tx
	}
	faelle := map[string]Transaction{
		// Audit H1 auf der Kette: fremde Signieradresse ohne deren Zustimmung.
		"Signierschluessel stimmt nicht zu": mit(func(tx *Transaction) { tx.Nachweis.Sig2 = personalSign(t, fremd, msg) }),
		"ohne zweite Unterschrift":          mit(func(tx *Transaction) { tx.Nachweis.Sig2 = "" }),
		"Betreiber gefaelscht":              mit(func(tx *Transaction) { tx.Nachweis.Sig = personalSign(t, fremd, msg) }),
		"umgelenkte Signieradresse":         mit(func(tx *Transaction) { tx.To = adrVon(fremd) }),
		"alte Form ohne Zeitpunkt": mit(func(tx *Transaction) {
			tx.Nachweis.Sig = personalSign(t, b, "Aequitas: authorize validator "+gut.To)
		}),
		"zwei Stunden alt":       bindungUnterschrieben(t, b, s, f.jetzt-2*3600),
		"eine Stunde voraus":     bindungUnterschrieben(t, b, s, f.jetzt+3600),
		"ohne Zeitpunkt":         bindungUnterschrieben(t, b, s, 0),
		"ohne Nachweis":          mit(func(tx *Transaction) { tx.Nachweis = nil }),
		"Grossbuchstaben":        mit(func(tx *Transaction) { tx.To = "0x" + strings.ToUpper(gut.To[2:]) }),
		"Betreiber ohne Adresse": mit(func(tx *Transaction) { tx.Wallet = "" }),
	}
	for name, tx := range faelle {
		if f.block(f.jetzt, tx) {
			t.Fatalf("%s: Block angenommen", name)
		}
		if f.anzahl() != 0 || f.summe() != ([32]byte{}) {
			t.Fatalf("%s: abgewiesener Block hat einen Eintrag hinterlassen", name)
		}
	}
	if !f.block(f.jetzt, gut) {
		t.Fatal("danach die echte Bindung abgewiesen")
	}
}

// Missbrauch gegen das Register: uebersprungen, der Block bleibt gueltig.
func TestValidatorRegister_ZustandsRegeln_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b1, b2 := f.betreiber(), f.betreiber()
	s1, _ := neuerSchluessel(t)
	s2, _ := neuerSchluessel(t)
	t1, t2 := f.jetzt-300, f.jetzt-200
	alt := bindungUnterschrieben(t, b1, s1, t1)
	if !f.block(f.jetzt, alt) || !f.block(f.jetzt, bindungUnterschrieben(t, b1, s2, t2)) {
		t.Fatal("Vorbedingung: beide Bindungen muessen gelten")
	}
	stand := f.summe()

	// Die alte Bindung wiedereinspielen, um die neue zurueckzudrehen.
	if !f.block(f.jetzt, alt) {
		t.Fatal("Block mit wiederholter Bindung abgewiesen -- sie ist zu ueberspringen, nicht der Block")
	}
	if s, z, _ := f.eintrag(b1); s != adrVon(s2) || z != t2 {
		t.Fatalf("alte Bindung hat die neue zurueckgedreht: %s %d", s, z)
	}
	// Dieselbe Bindung noch einmal.
	if !f.block(f.jetzt, bindungUnterschrieben(t, b1, s2, t2)) {
		t.Fatal("Block mit doppelter Bindung abgewiesen")
	}
	if f.summe() != stand {
		t.Fatal("Wiederholung hat die Summe veraendert")
	}

	// Kein Mensch.
	nichtMensch, _ := neuerSchluessel(t)
	s3, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, nichtMensch, s3, f.jetzt-100)) {
		t.Fatal("Block mit Bindung eines Nicht-Menschen abgewiesen")
	}
	if _, _, ok := f.eintrag(nichtMensch); ok {
		t.Fatal("Nicht-Mensch als Betreiber eingetragen")
	}
	if f.summe() != stand || f.neuAufgebaut() != stand {
		t.Fatal("uebersprungene Bindungen haben die Summe veraendert")
	}

	// Zwei Bindungen desselben Betreibers in einem Block: die spaetere gilt.
	// Eine zweite mit demselben Zeitpunkt liegt unter dem Abstand je
	// Betreiber (validatorBindungAbstand) und wird abgewiesen -- welche
	// Adresse die groessere ist, spielt keine Rolle mehr.
	s4, _ := neuerSchluessel(t)
	s5, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, b2, s4, f.jetzt-50), bindungUnterschrieben(t, b2, s5, f.jetzt-40)) {
		t.Fatal("Block mit zwei Bindungen abgewiesen")
	}
	if s, _, _ := f.eintrag(b2); s != adrVon(s5) {
		t.Fatalf("spaetere Bindung im selben Block gilt nicht: %s", s)
	}
	s6, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, b2, s6, f.jetzt-40)) {
		t.Fatal("Block abgewiesen")
	}
	if s, _, _ := f.eintrag(b2); s != adrVon(s5) {
		t.Fatalf("bei gleichem Zeitpunkt gilt %s statt der ersten %s", s, adrVon(s5))
	}
	if f.neuAufgebaut() != f.summe() {
		t.Fatal("laufende und neu aufgebaute Summe weichen ab")
	}
}

func TestValidatorRegister_VorDemStichtag_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	validatorRegisterOverride.Store(f.jetzt + 86400)
	b := f.betreiber()
	s, _ := neuerSchluessel(t)
	vorher := f.cs.StateRoot()
	if f.block(f.jetzt, bindungUnterschrieben(t, b, s, f.jetzt-60)) {
		t.Fatal("validator_bindung vor dem Stichtag angenommen")
	}
	if f.anzahl() != 0 || f.cs.StateRoot() != vorher {
		t.Fatal("validator_bindung vor dem Stichtag hat etwas veraendert")
	}
}

// Ein Block, der nach der Bindung scheitert, nimmt sie mit zurueck --
// Zeile und Summe. Der ehrliche Block danach legt sie genau einmal an.
func TestValidatorRegister_ZurueckgewiesenerBlock_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b := f.betreiber()
	s, _ := neuerSchluessel(t)
	gut := bindungUnterschrieben(t, b, s, f.jetzt-60)
	kaputt := gut
	kaputt.To = "0xkaputt"
	vorher := f.cs.StateRoot()
	if f.block(f.jetzt, gut, kaputt) {
		t.Fatal("Vorbedingung: der Block muss scheitern")
	}
	if f.anzahl() != 0 || f.summe() != ([32]byte{}) {
		t.Fatal("zurueckgewiesener Block hat die Bindung hinterlassen")
	}
	if got := f.cs.StateRoot(); got != vorher {
		t.Fatalf("StateRoot nach dem Zurueckrollen %s statt %s", got, vorher)
	}
	if !f.block(f.jetzt, gut) {
		t.Fatal("ehrlicher Block abgewiesen")
	}
	if f.anzahl() != 1 || f.summe() != bindungsBlaetter(gut.Wallet, gut.To, gut.Nachweis.Zeit, false) {
		t.Fatal("ehrlicher Block hat die Bindung nicht genau einmal angelegt")
	}
}

// Fail-closed: ist das Register nicht lesbar, wird der Block abgewiesen --
// nie die Bindung als "uebersprungen" verbucht.
func TestValidatorRegister_LesefehlerWeistBlockAb_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b := f.betreiber()
	s, _ := neuerSchluessel(t)
	if _, err := f.cs.db.Exec(`DROP TABLE validator_register`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.cs.InitValidatorRegisterTable(); err != nil {
			t.Error(err)
		}
	})
	vorher, vorherB := uebersprungeneUeberweisungen.Load(), uebersprungeneBindungen.Load()
	if f.block(f.jetzt, bindungUnterschrieben(t, b, s, f.jetzt-60)) {
		t.Fatal("Block trotz unlesbarem Register angenommen")
	}
	if uebersprungeneUeberweisungen.Load() != vorher || uebersprungeneBindungen.Load() != vorherB {
		t.Fatal("Lesefehler als Zustandsablehnung verbucht")
	}
	if f.summe() != ([32]byte{}) {
		t.Fatal("Summe trotz abgewiesenem Block veraendert")
	}
}

// Der Snapshot traegt das Register mit beiden Unterschriften; der
// importierende Knoten prueft sie selbst, ein falscher Eintrag verwirft den
// ganzen Import.
func TestValidatorRegister_Snapshot_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b1, b2 := f.betreiber(), f.betreiber()
	s1, _ := neuerSchluessel(t)
	s2, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, b1, s1, f.jetzt-60), bindungUnterschrieben(t, b2, s2, f.jetzt-50)) {
		t.Fatal("Bindungen abgewiesen")
	}
	stand := f.summe()
	snap := f.cs.ExportSnapshot(nil, 3, false)
	if snap == nil {
		t.Fatal("kein Snapshot")
	}
	if len(snap.Validatoren) != 2 || snap.Validatoren[0].Betreiber > snap.Validatoren[1].Betreiber {
		t.Fatalf("Register im Snapshot: %+v", snap.Validatoren)
	}
	for _, e := range snap.Validatoren {
		if e.SigOperator == "" || e.SigSigning == "" {
			t.Fatalf("Snapshot-Eintrag ohne Unterschriften: %+v", e)
		}
	}
	importiere := func(liste []SnapshotValidator) error {
		tx, err := f.cs.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		bis := snapshotValidatorenBis(nowUnix())
		if err := pruefeSnapshotValidatoren(liste, bis); err != nil {
			tx.Rollback()
			return err
		}
		if err := validatorenImportieren(tx, liste, true, bis); err != nil {
			tx.Rollback()
			return err
		}
		// Wie der Resync: der Verlauf wird mit ersetzt (hier ohne eigene
		// Verlaufszeilen -- die Registereintraege kommen immer mit).
		if err := verlaufImportieren(tx, nil, liste, true); err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	falsch := append([]SnapshotValidator(nil), snap.Validatoren...)
	_, fremd := neuerSchluessel(t)
	falsch[1].Signing = fremd
	if err := importiere(falsch); err == nil {
		t.Fatal("Snapshot mit gefaelschtem Eintrag importiert")
	}
	if f.anzahl() != 2 || f.neuAufgebaut() != stand {
		t.Fatal("verworfener Import hat das Register veraendert")
	}
	if err := importiere(snap.Validatoren[:1]); err != nil {
		t.Fatalf("Import: %v", err)
	}
	erster := snap.Validatoren[0] // nach Betreiber geordnet
	if f.anzahl() != 1 || f.neuAufgebaut() != bindungsBlaetter(erster.Betreiber, erster.Signing, erster.Zeit, false) {
		t.Fatal("ersetzender Import hat das Register nicht auf den Snapshot gesetzt")
	}
	if err := importiere(snap.Validatoren); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if f.neuAufgebaut() != stand {
		t.Fatal("Register nach dem Import weicht ab")
	}
}

// Eine wiederholte Bindung zaehlt nur bei den Bindungen, nie bei den
// Ueberweisungen -- deren Zahl ist ein Alarm fuer Kontoabweichungen.
func TestValidatorRegister_WiederholungZaehltEigen_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b := f.betreiber()
	s, _ := neuerSchluessel(t)
	tx := bindungUnterschrieben(t, b, s, f.jetzt-60)
	if !f.block(f.jetzt, tx) {
		t.Fatal("Bindung abgewiesen")
	}
	u, ub := uebersprungeneUeberweisungen.Load(), uebersprungeneBindungen.Load()
	if !f.block(f.jetzt, tx) {
		t.Fatal("Wiederholung hat den Block abgewiesen")
	}
	if uebersprungeneUeberweisungen.Load() != u {
		t.Fatal("wiederholte Bindung als uebersprungene Ueberweisung gezaehlt")
	}
	if uebersprungeneBindungen.Load() != ub+1 {
		t.Fatal("wiederholte Bindung nicht gezaehlt")
	}
}

// Ein Betreiber, dessen Konto nicht im Speicher steht (kalt), wird aus der
// Datenbank geladen -- nicht als "kein Mensch" uebersprungen.
func TestValidatorRegister_KaltesKonto_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b := f.betreiber()
	s, _ := neuerSchluessel(t)
	f.cs.mu.Lock()
	f.cs.accounts.Delete(adrVon(b))
	f.cs.mu.Unlock()
	if !f.block(f.jetzt, bindungUnterschrieben(t, b, s, f.jetzt-60)) {
		t.Fatal("Bindung mit kaltem Konto abgewiesen")
	}
	if _, _, ok := f.eintrag(b); !ok {
		t.Fatal("Bindung mit kaltem Konto als Nicht-Mensch uebersprungen")
	}
	// Auch ohne den Rueckroll-Snapshot des Blocks, der Konten vorab laedt:
	// die Bindung laedt ihren Betreiber selbst.
	b2 := f.betreiber()
	s2, _ := neuerSchluessel(t)
	tx := bindungUnterschrieben(t, b2, s2, f.jetzt-60)
	f.cs.mu.Lock()
	f.cs.accounts.Delete(adrVon(b2))
	err := f.cs.applyValidatorBindungLocked(context.Background(), &tx, f.jetzt)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatalf("Bindung mit kaltem Konto (direkt): %v", err)
	}
}

// zuruecksetzen: leeres Register und leerer Verlauf, Summe null -- wie ein
// frischer Knoten.
func (f *registerFall) zuruecksetzen() {
	f.t.Helper()
	if _, err := f.cs.db.Exec(`TRUNCATE validator_register, validator_verlauf`); err != nil {
		f.t.Fatal(err)
	}
	f.cs.mu.Lock()
	f.cs.validatorSetXOR = [32]byte{}
	f.cs.mu.Unlock()
}

func (f *registerFall) zuSignieradresse(s string) string {
	f.t.Helper()
	b, err := f.cs.validatorZuSignieradresseCtx(context.Background(), s)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

// Geschwisterbloecke kommen bei jedem Knoten in anderer Reihenfolge an. Das
// Register und wem eine Adresse gehoert muessen in jeder Reihenfolge gleich
// sein. Jede Folge wird hier vorwaerts und rueckwaerts nachgespielt.
func TestValidatorRegister_ReihenfolgeEgal_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	a, c := f.betreiber(), f.betreiber()
	s1, _ := neuerSchluessel(t)
	s2, _ := neuerSchluessel(t)
	z := f.jetzt

	faelle := []struct {
		name    string
		vorab   []Transaction // gemeinsamer Vorlauf (Vorfahren)
		txs     []Transaction // in Geschwisterbloecken
		nachher []Transaction // gemeinsamer Nachlauf (Nachfahren)
		pruef   func(t *testing.T)
	}{
		{
			name: "zwei Betreiber, eine Adresse",
			txs:  []Transaction{bindungUnterschrieben(t, a, s1, z-300), bindungUnterschrieben(t, c, s1, z-200)},
			pruef: func(t *testing.T) {
				if got := f.zuSignieradresse(adrVon(s1)); got != adrVon(c) {
					t.Fatalf("s1 gehoert %s, erwartet die spaetere Bindung %s", got, adrVon(c))
				}
			},
		},
		{
			name:  "A gibt s1 frei, C nimmt s1",
			vorab: []Transaction{bindungUnterschrieben(t, a, s1, z-400)},
			txs:   []Transaction{bindungUnterschrieben(t, a, s2, z-200), bindungUnterschrieben(t, c, s1, z-300)},
			pruef: func(t *testing.T) {
				if got := f.zuSignieradresse(adrVon(s1)); got != adrVon(c) {
					t.Fatalf("s1 gehoert %s, erwartet %s", got, adrVon(c))
				}
				if got := f.zuSignieradresse(adrVon(s2)); got != adrVon(a) {
					t.Fatalf("s2 gehoert %s, erwartet %s", got, adrVon(a))
				}
			},
		},
		{
			// C uebernimmt s1 von A in einem Geschwisterblock und zieht
			// spaeter weiter: A's Bindung bleibt ueberholt, in beiden
			// Reihenfolgen -- s1 gehoert danach keinem.
			name:    "uebernommen, dann weitergezogen",
			txs:     []Transaction{bindungUnterschrieben(t, a, s1, z-300), bindungUnterschrieben(t, c, s1, z-200)},
			nachher: []Transaction{bindungUnterschrieben(t, c, s2, z-100)},
			pruef: func(t *testing.T) {
				if got := f.zuSignieradresse(adrVon(s1)); got != "" {
					t.Fatalf("s1 gehoert nach dem Weiterziehen %s -- ueberholte Bindung wieder aufgelebt", got)
				}
			},
		},
		{
			name: "gleicher Zeitpunkt, zwei Betreiber: umstritten",
			txs:  []Transaction{bindungUnterschrieben(t, a, s1, z-100), bindungUnterschrieben(t, c, s1, z-100)},
			pruef: func(t *testing.T) {
				if got := f.zuSignieradresse(adrVon(s1)); got != "" {
					t.Fatalf("umstrittene Adresse gehoert %s", got)
				}
			},
		},
	}
	for _, fall := range faelle {
		var summen [2][32]byte
		var eintraege [2][]SnapshotValidator
		for richtung := 0; richtung < 2; richtung++ {
			f.zuruecksetzen()
			if len(fall.vorab) > 0 && !f.block(z, fall.vorab...) {
				t.Fatalf("%s: Vorlauf abgewiesen", fall.name)
			}
			folge := append([]Transaction(nil), fall.txs...)
			if richtung == 1 {
				for i, j := 0, len(folge)-1; i < j; i, j = i+1, j-1 {
					folge[i], folge[j] = folge[j], folge[i]
				}
			}
			for _, tx := range folge { // jede in ihrem eigenen Geschwisterblock
				if !f.block(z, tx) {
					t.Fatalf("%s: Block abgewiesen", fall.name)
				}
			}
			if len(fall.nachher) > 0 && !f.block(z, fall.nachher...) {
				t.Fatalf("%s: Nachlauf abgewiesen", fall.name)
			}
			summen[richtung] = f.summe()
			e, err := f.cs.validatorRegisterLesen()
			if err != nil {
				t.Fatal(err)
			}
			eintraege[richtung] = e
			if fall.pruef != nil {
				fall.pruef(t)
			}
		}
		if summen[0] != summen[1] {
			t.Fatalf("%s: die Reihenfolge entscheidet ueber das Register\n vorwaerts:  %+v\n rueckwaerts: %+v", fall.name, eintraege[0], eintraege[1])
		}
	}
}

// Merge-Import: je Betreiber gilt die neuere Bindung -- die eigene, wenn
// sie neuer ist, die des Snapshots sonst.
func TestValidatorRegister_MergeImportNimmtDieNeuere_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	a, c := f.betreiber(), f.betreiber()
	s1, _ := neuerSchluessel(t)
	s2, _ := neuerSchluessel(t)
	s3, _ := neuerSchluessel(t)
	eintrag := func(b, s *ecdsa.PrivateKey, z int64) SnapshotValidator {
		tx := bindungUnterschrieben(t, b, s, z)
		return SnapshotValidator{Betreiber: tx.Wallet, Signing: tx.To, Zeit: z, SigOperator: tx.Nachweis.Sig, SigSigning: tx.Nachweis.Sig2}
	}
	// Lokal: a neu (s2 um jetzt-100), c alt (s3 um jetzt-300).
	if !f.block(f.jetzt, bindungUnterschrieben(t, a, s2, f.jetzt-100), bindungUnterschrieben(t, c, s3, f.jetzt-300)) {
		t.Fatal("Bindungen abgewiesen")
	}
	// Snapshot: a alt, c neu.
	snap := []SnapshotValidator{eintrag(a, s1, f.jetzt-200), eintrag(c, s1, f.jetzt-200)}
	tx, err := f.cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := validatorenImportieren(tx, snap, false, snapshotValidatorenBis(nowUnix())); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := f.eintrag(a); s != adrVon(s2) {
		t.Fatalf("aeltere Bindung aus dem Snapshot hat die neuere eigene ersetzt: %s", s)
	}
	if s, _, _ := f.eintrag(c); s != adrVon(s1) {
		t.Fatalf("neuere Bindung aus dem Snapshot nicht uebernommen: %s", s)
	}

	// Nach dem Merge stimmen die Markierungen: lokal bindet d an s4 um
	// jetzt-300, der Snapshot bringt e an s4 um jetzt-150 -- d ist danach
	// ueberholt, s4 gehoert e.
	d, e := f.betreiber(), f.betreiber()
	s4, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, d, s4, f.jetzt-300)) {
		t.Fatal("Bindung d abgewiesen")
	}
	tx2, err := f.cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := validatorenImportieren(tx2, []SnapshotValidator{eintrag(e, s4, f.jetzt-150)}, false, snapshotValidatorenBis(nowUnix())); err != nil {
		tx2.Rollback()
		t.Fatal(err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := f.zuSignieradresse(adrVon(s4)); got != adrVon(e) {
		t.Fatalf("nach dem Merge gehoert s4 %q statt e -- d's fruehere Bindung nicht als ueberholt markiert", got)
	}
}

// Ein Snapshot, dessen Register nicht lesbar ist, wird nicht ausgegeben --
// und der Endpunkt antwortet nicht mit einem unvollstaendigen,
// unterschriebenen.
func TestValidatorRegister_ExportOhneRegisterGibtKeinenSnapshot_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	if f.cs.ExportSnapshot(nil, 1, false) == nil {
		t.Fatal("Vorbedingung: lesbares Register muss einen Snapshot ergeben")
	}
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_register RENAME COLUMN sig_signing TO sig_kaputt`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.cs.db.Exec(`ALTER TABLE validator_register RENAME COLUMN sig_kaputt TO sig_signing`); err != nil {
			t.Error(err)
		}
	})
	if snap := f.cs.ExportSnapshot(nil, 2, false); snap != nil {
		t.Fatalf("Snapshot trotz unlesbarem Register ausgegeben (%d Validatoren)", len(snap.Validatoren))
	}
}

// Eine ueberholte Bindung lebt nicht wieder auf (zweiter Sicherheitsdurchgang
// zu #292): A bindet s1, s1 stimmt spaeter C zu, C zieht weiter -- s1 gehoert
// danach KEINEM, nicht wieder A. Erst eine neue Bindung von A mit neuer
// Zustimmung von s1 gibt sie A zurueck.
func TestValidatorRegister_UeberholteLebtNichtAuf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	a, c := f.betreiber(), f.betreiber()
	s1, _ := neuerSchluessel(t)
	s3, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, a, s1, f.jetzt-300)) {
		t.Fatal("Bindung A abgewiesen")
	}
	if got := f.zuSignieradresse(adrVon(s1)); got != adrVon(a) {
		t.Fatalf("s1 gehoert %s statt A", got)
	}
	if !f.block(f.jetzt, bindungUnterschrieben(t, c, s1, f.jetzt-200)) {
		t.Fatal("Bindung C abgewiesen")
	}
	if got := f.zuSignieradresse(adrVon(s1)); got != adrVon(c) {
		t.Fatalf("s1 gehoert %s statt C", got)
	}
	// Die Markierung steht in der Summe (also in der StateRoot): A's Blatt
	// ist jetzt das der ueberholten Bindung.
	var erwartet [32]byte
	xorInto(&erwartet, bindungsBlaetter(adrVon(a), adrVon(s1), f.jetzt-300, true))
	xorInto(&erwartet, bindungsBlaetter(adrVon(c), adrVon(s1), f.jetzt-200, false))
	if f.summe() != erwartet || f.neuAufgebaut() != erwartet {
		t.Fatal("die Ueberholt-Markierung steht nicht in der Summe")
	}
	if !f.block(f.jetzt, bindungUnterschrieben(t, c, s3, f.jetzt-100)) {
		t.Fatal("Weiterziehen von C abgewiesen")
	}
	if got := f.zuSignieradresse(adrVon(s1)); got != "" {
		t.Fatalf("s1 gehoert nach dem Weiterziehen wieder %s -- ohne neue Zustimmung des Schluessels", got)
	}
	if f.neuAufgebaut() != f.summe() {
		t.Fatal("laufende und neu aufgebaute Summe weichen ab (Markierung nicht in der Summe)")
	}
	// Neu binden mit neuer Zustimmung: s1 gehoert wieder A.
	if !f.block(f.jetzt, bindungUnterschrieben(t, a, s1, f.jetzt-50)) {
		t.Fatal("neue Bindung A abgewiesen")
	}
	if got := f.zuSignieradresse(adrVon(s1)); got != adrVon(a) {
		t.Fatalf("nach neuer Bindung gehoert s1 %s statt A", got)
	}
	// Rueckrollen nimmt die Markierung mit: ein Block, der C noch einmal
	// auf s1 bindet und dann scheitert, laesst A's Bindung unberuehrt.
	stand := f.summe()
	kaputt := bindungUnterschrieben(t, c, s1, f.jetzt-10)
	kaputt2 := kaputt
	kaputt2.To = "0xkaputt"
	if f.block(f.jetzt, kaputt, kaputt2) {
		t.Fatal("Vorbedingung: der Block muss scheitern")
	}
	if got := f.zuSignieradresse(adrVon(s1)); got != adrVon(a) || f.summe() != stand {
		t.Fatalf("zurueckgewiesener Block hat die Markierung hinterlassen (s1 -> %s)", got)
	}
}
