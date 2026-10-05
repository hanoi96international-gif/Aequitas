package keeper

import (
	"encoding/json"
	"errors"
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
