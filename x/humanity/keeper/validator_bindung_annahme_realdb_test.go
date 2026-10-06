package keeper

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Validator-Register, Schritt 2, gegen eine echte Datenbank: die Annahme
// schreibt Register und Ausgang in EINER Transaktion, und was im Ausgang
// liegt, nimmt jeder Nachspielende an.

func (f *registerFall) ausgang() []Transaction {
	f.t.Helper()
	rows, err := f.cs.db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 ORDER BY wal_seq, id`)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []Transaction
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			f.t.Fatal(err)
		}
		var tx Transaction
		if err := json.Unmarshal([]byte(s), &tx); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, tx)
	}
	return out
}

func TestValidatorBinden_AnnahmeUndNachspielen_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	betreiber := f.betreiber()
	knoten, kw := neuerSchluessel(t)

	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten, f.jetzt)); err != nil {
		t.Fatalf("gueltige Bindung abgewiesen: %v", err)
	}
	if s, z, ok := f.eintrag(betreiber); !ok || s != kw || z != f.jetzt {
		t.Fatalf("Register: %q %d %v, erwartet %s %d", s, z, ok, kw, f.jetzt)
	}
	aus := f.ausgang()
	if len(aus) != 1 || aus[0].Type != "validator_bindung" {
		t.Fatalf("Ausgang: %+v", aus)
	}
	summe := f.summe()
	if summe == ([32]byte{}) || summe != f.neuAufgebaut() {
		t.Fatalf("Summe nach der Annahme %x, neu aufgebaut %x", summe, f.neuAufgebaut())
	}

	// Ein anderer Knoten (hier: derselbe nach Leeren des Registers) spielt den
	// Auftrag aus dem Ausgang nach und kommt auf dieselbe Summe.
	if _, err := f.cs.db.Exec(`TRUNCATE validator_register`); err != nil {
		t.Fatal(err)
	}
	f.cs.mu.Lock()
	f.cs.validatorSetXOR = [32]byte{}
	f.cs.mu.Unlock()
	if !f.block(f.jetzt+30, aus[0]) {
		t.Fatal("der ausgesandte Auftrag besteht das Nachspielen nicht")
	}
	if f.summe() != summe {
		t.Fatalf("Summe nachgespielt %x, beim Annehmenden %x", f.summe(), summe)
	}

	// Wiederholung derselben Bindung und eine aeltere: abgewiesen, nichts
	// Neues im Ausgang.
	for name, zeit := range map[string]int64{"Wiederholung": f.jetzt, "aelter": f.jetzt - 60} {
		err := f.cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten, zeit))
		if !istZustandsAblehnung(err) {
			t.Fatalf("%s: erwartet Zustandsablehnung, bekam %v", name, err)
		}
	}
	if n := len(f.ausgang()); n != 1 {
		t.Fatalf("nach abgewiesenen Bindungen %d Auftraege im Ausgang", n)
	}
	if f.summe() != summe {
		t.Fatal("abgewiesene Bindung hat die Summe veraendert")
	}

	// Eine neuere Bindung an einen anderen Schluessel ersetzt die alte.
	knoten2, kw2 := neuerSchluessel(t)
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten2, f.jetzt+1)); err != nil {
		t.Fatalf("neuere Bindung abgewiesen: %v", err)
	}
	if s, _, _ := f.eintrag(betreiber); s != kw2 {
		t.Fatalf("Register nach Neubindung: %s", s)
	}
	if f.summe() != f.neuAufgebaut() {
		t.Fatal("Summe nach Neubindung weicht vom Neuaufbau ab")
	}
}

// Missbrauch: wer kein registrierter Mensch ist, bindet nichts -- und die
// Ablehnung hinterlaesst weder Register noch Ausgang noch Summe.
func TestValidatorBinden_NichtMensch_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	fremd, _ := neuerSchluessel(t) // kein Konto
	knoten, _ := neuerSchluessel(t)

	err := f.cs.ValidatorBinden(bindungUnterschrieben(t, fremd, knoten, f.jetzt))
	if !istZustandsAblehnung(err) {
		t.Fatalf("erwartet Zustandsablehnung, bekam %v", err)
	}
	if f.anzahl() != 0 || len(f.ausgang()) != 0 || f.summe() != ([32]byte{}) {
		t.Fatalf("Ablehnung hinterliess Spuren: Register %d, Ausgang %d, Summe %x", f.anzahl(), len(f.ausgang()), f.summe())
	}
}

// Missbrauch: ein Folger nimmt nicht an, auch wenn die Bindung gueltig ist
// -- sonst laegen Bindungen in Geschwisterbloecken.
func TestValidatorBinden_NurDerLeiter_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	betreiber := f.betreiber()
	knoten, _ := neuerSchluessel(t)

	f.cs.leitung.Store(&Leitung{})
	t.Cleanup(func() { f.cs.leitung.Store(nil) })
	err := f.cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten, f.jetzt))
	if !errors.Is(err, ErrNichtLeiter) {
		t.Fatalf("Folger: erwartet ErrNichtLeiter, bekam %v", err)
	}
	if f.anzahl() != 0 || len(f.ausgang()) != 0 {
		t.Fatalf("Folger hat angenommen: Register %d, Ausgang %d", f.anzahl(), len(f.ausgang()))
	}
	if n := f.cs.annahmenLaufend.Load(); n != 0 {
		t.Fatalf("annahmenLaufend %d", n)
	}
}

// Ein Fehler NACH dem Anwenden (hier: der Ausgang nimmt die Zeile nicht)
// rollt alles zurueck -- auch die Ueberholt-Markierung der frueheren Bindung
// eines anderen Betreibers an denselben Schluessel und die Summe.
func TestValidatorBinden_RollbackNachAusgangsfehler_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	a, b := f.betreiber(), f.betreiber()
	knoten, _ := neuerSchluessel(t)
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, a, knoten, f.jetzt-60)); err != nil {
		t.Fatal(err)
	}
	summe := f.summe()
	if _, err := f.cs.db.Exec(`ALTER TABLE pending_txs ADD CONSTRAINT test_keine_bindung CHECK (tx_json NOT LIKE '%"validator_bindung"%') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE pending_txs DROP CONSTRAINT IF EXISTS test_keine_bindung`) })
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, b, knoten, f.jetzt)); err == nil {
		t.Fatal("Bindung trotz Ausgangsfehler angenommen")
	}
	var ueberholt bool
	if err := f.cs.db.QueryRow(`SELECT ueberholt FROM validator_register WHERE operator_wallet = $1`, adrVon(a)).Scan(&ueberholt); err != nil || ueberholt {
		t.Fatalf("fruehere Bindung nach dem Rollback ueberholt=%v (%v)", ueberholt, err)
	}
	if _, _, ok := f.eintrag(b); ok {
		t.Fatal("Bindung von B nach dem Rollback im Register")
	}
	if f.summe() != summe || f.summe() != f.neuAufgebaut() {
		t.Fatal("Summe nach dem Rollback veraendert")
	}
	if n := f.cs.annahmenLaufend.Load(); n != 0 {
		t.Fatalf("annahmenLaufend %d", n)
	}
}

// Wallets, die v 0/1 oder Grossbuchstaben liefern, binden trotzdem -- im
// Register und im Ausgang steht die kanonische Form, die jeder Nachspielende
// verlangt.
func TestValidatorBinden_AngeglicheneSignatur_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	op := f.betreiber()
	knoten, _ := neuerSchluessel(t)
	tx := bindungUnterschrieben(t, op, knoten, f.jetzt)
	kanon := tx.Nachweis.Sig
	v := "00"
	if kanon[130:] == "1c" {
		v = "01"
	}
	tx.Nachweis.Sig = "0x" + strings.ToUpper(kanon[2:130]) + v
	// Felder, die zur Bindung nicht gehoeren -- die Unterschrift deckt sie
	// nicht, also duerfen sie weder in Ausgang noch Block.
	tx.Nachweis.Nonce, tx.Nachweis.Von2, tx.Nachweis.Betrag = 5, "0x"+strings.Repeat("9", 40), 1.5
	if err := f.cs.ValidatorBinden(tx); err != nil {
		t.Fatalf("angeglichene Signatur abgewiesen: %v", err)
	}
	aus := f.ausgang()
	if len(aus) != 1 || aus[0].Nachweis.Sig != kanon {
		t.Fatalf("im Ausgang: %+v", aus)
	}
	if aus[0].Nachweis.Nonce != 0 || aus[0].Nachweis.Von2 != "" || aus[0].Nachweis.Betrag != 0 {
		t.Fatal("fremde Nachweis-Felder im Ausgang")
	}
}

// Ueber HTTP: je Betreiber eine angenommene Bindung je 30 s; ein Erfolg
// verbraucht keinen Fehlversuch der IP.
func TestHandleValidatorBindung_JeBetreiber_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	op := f.betreiber()
	k1, _ := neuerSchluessel(t)
	k2, _ := neuerSchluessel(t)
	a := &APIServer{state: f.cs}
	h := a.bindungsGrenze(a.handleValidatorBindung)
	ip := "192.0.2.77"
	t.Cleanup(func() {
		registerRateLimit.Delete("validator-bindung-fehl:" + ip)
		registerRateLimit.Delete("validator-bindung-betreiber:" + adrVon(op))
	})
	posten := func(tx Transaction) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/validator-bindung", bytes.NewReader(bindungsKoerper(t, tx)))
		req.RemoteAddr = ip + ":1"
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	rec := posten(bindungUnterschrieben(t, op, k1, f.jetzt))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "next block") {
		t.Fatalf("erste Bindung: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := registerRateLimit.Load("validator-bindung-fehl:" + ip); ok {
		t.Fatal("Erfolg als Fehlversuch gezaehlt")
	}
	if rec := posten(bindungUnterschrieben(t, op, k2, f.jetzt+1)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("zweite Bindung desselben Betreibers binnen 30 s: %d %s", rec.Code, rec.Body.String())
	}
	if s, _, _ := f.eintrag(op); s != adrVon(k1) {
		t.Fatalf("Register: %s", s)
	}
}

// Missbrauch: ein Fremder mit zwei frischen Schluesseln (gueltige
// Unterschriften, aber kein Mensch) erreicht die Schreibsperre nicht -- die
// Vorpruefung weist ihn ab.
func TestValidatorBinden_VorpruefungOhneSperre_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	fremd, _ := neuerSchluessel(t)
	knoten, _ := neuerSchluessel(t)
	// snapshotNs waechst bei jedem Eintritt in die atomare Sektion (unter
	// der Schreibsperre), auch wenn sie danach scheitert.
	vorher := atomicPhasenStand.snapshotNs.Load()
	err := f.cs.ValidatorBinden(bindungUnterschrieben(t, fremd, knoten, f.jetzt))
	if !istZustandsAblehnung(err) {
		t.Fatalf("erwartet Zustandsablehnung, bekam %v", err)
	}
	if atomicPhasenStand.snapshotNs.Load() != vorher {
		t.Fatal("die Bindung eines Fremden hat die Schreibsperre erreicht")
	}
	// Nicht neuer: ebenso vor der Sperre.
	op := f.betreiber()
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, op, knoten, f.jetzt)); err != nil {
		t.Fatal(err)
	}
	vorher = atomicPhasenStand.snapshotNs.Load()
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, op, knoten, f.jetzt-5)); !istZustandsAblehnung(err) {
		t.Fatalf("aeltere Bindung: %v", err)
	}
	if atomicPhasenStand.snapshotNs.Load() != vorher {
		t.Fatal("eine aeltere Bindung hat die Schreibsperre erreicht")
	}
}
