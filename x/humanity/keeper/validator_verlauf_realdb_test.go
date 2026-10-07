package keeper

import (
	"testing"
)

// Der Verlauf der Bindungen (validator_verlauf, validator_register.go): jede
// gueltige Bindung, die je in einem Block stand, in der Summe und im
// Snapshot.

func (f *registerFall) verlaufAnzahl() int {
	f.t.Helper()
	var n int
	if err := f.cs.db.QueryRow(`SELECT COUNT(*) FROM validator_verlauf`).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// Wechsel, Uebernahme und eine aeltere Bindung, die erst spaeter ankommt:
// jede steht im Verlauf, auch die, die das Register nicht aendert. Die
// fortgeschriebene Summe bleibt gleich der neu aufgebauten; eine
// Wiederholung und ein zurueckgewiesener Block aendern nichts.
func TestValidatorVerlauf_JedeBindungInDerSumme_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	if _, err := f.cs.db.Exec(`DELETE FROM validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	b1, b2 := f.betreiber(), f.betreiber()
	k1, s1 := neuerSchluessel(t)
	k2, _ := neuerSchluessel(t)
	k3, _ := neuerSchluessel(t)
	j := f.jetzt
	if !f.block(j, bindungUnterschrieben(t, b1, k1, j-300)) ||
		!f.block(j, bindungUnterschrieben(t, b1, k2, j-200)) || // Wechsel
		!f.block(j, bindungUnterschrieben(t, b2, k1, j-100)) { // Uebernahme
		t.Fatal("Bindung abgewiesen")
	}
	if n := f.verlaufAnzahl(); n != 3 {
		t.Fatalf("%d Zeilen im Verlauf, erwartet 3", n)
	}
	if f.summe() != f.neuAufgebaut() {
		t.Fatal("Summe weicht vom Neuaufbau ab")
	}
	// Aelter als b1's Bindung an k2: das Register bleibt, der Verlauf waechst.
	alt := bindungUnterschrieben(t, b1, k3, j-250)
	if !f.block(j, alt) {
		t.Fatal("Block abgewiesen")
	}
	if s, _, _ := f.eintrag(b1); s == adrVon(k3) {
		t.Fatal("aeltere Bindung hat das Register geaendert")
	}
	if n := f.verlaufAnzahl(); n != 4 || f.summe() != f.neuAufgebaut() {
		t.Fatalf("aeltere Bindung: %d Zeilen, Summe gleich Neuaufbau: %v", n, f.summe() == f.neuAufgebaut())
	}
	// Wiederholung: nichts doppelt.
	vorher := f.summe()
	if !f.block(j, alt) {
		t.Fatal("Block abgewiesen")
	}
	if f.verlaufAnzahl() != 4 || f.summe() != vorher {
		t.Fatal("Wiederholung hat den Verlauf veraendert")
	}
	// Zurueckgewiesener Block: keine Zeile, die Summe wie vorher.
	k4, _ := neuerSchluessel(t)
	if f.block(j, bindungUnterschrieben(t, b2, k4, j-10), gift()) {
		t.Fatal("vergifteter Block angenommen")
	}
	if f.verlaufAnzahl() != 4 || f.summe() != vorher || f.neuAufgebaut() != vorher {
		t.Fatal("zurueckgewiesener Block hat den Verlauf veraendert")
	}
	// Im Verlauf steht, wer s1 wann hielt -- b1 bis zur (spaet angekommenen)
	// Bindung an k3, b2 seit der Uebernahme.
	zeilen, err := verlaufLesen(f.cs.db, []string{s1})
	if err != nil {
		t.Fatal(err)
	}
	iv := bindungsIntervalle(zeilen)
	var vonB1, vonB2 bool
	for _, i := range iv {
		if i.signing != s1 {
			continue
		}
		vonB1 = vonB1 || (i.betreiber == adrVon(b1) && i.von == j-300 && i.bis == j-250)
		vonB2 = vonB2 || (i.betreiber == adrVon(b2) && i.von == j-100 && i.bis == ewig)
	}
	if !vonB1 || !vonB2 {
		t.Fatalf("Zeitraeume fuer s1: %+v", iv)
	}
}

// Der Snapshot traegt den Verlauf; der Import prueft jede Zeile selbst
// (Missbrauch: eine gefaelschte oder doppelte Zeile verwirft den ganzen
// Snapshot). Ersetzen setzt den Verlauf auf den Snapshot, Zusammenfuehren
// ergaenzt nur.
func TestValidatorVerlauf_Snapshot_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	if _, err := f.cs.db.Exec(`DELETE FROM validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	b1, b2 := f.betreiber(), f.betreiber()
	k1, _ := neuerSchluessel(t)
	k2, _ := neuerSchluessel(t)
	j := f.jetzt
	if !f.block(j, bindungUnterschrieben(t, b1, k1, j-300), bindungUnterschrieben(t, b1, k2, j-200), bindungUnterschrieben(t, b2, k1, j-100)) {
		t.Fatal("Bindungen abgewiesen")
	}
	stand := f.summe()
	snap := f.cs.ExportSnapshot(nil, 3, false)
	if snap == nil {
		t.Fatal("kein Snapshot")
	}
	if len(snap.ValidatorVerlauf) != 3 {
		t.Fatalf("Verlauf im Snapshot: %+v", snap.ValidatorVerlauf)
	}
	bis := snapshotValidatorenBis(nowUnix())
	if err := pruefeSnapshotVerlauf(snap.ValidatorVerlauf, bis); err != nil {
		t.Fatalf("echter Verlauf abgewiesen: %v", err)
	}
	gefaelscht := append([]SnapshotValidator(nil), snap.ValidatorVerlauf...)
	_, fremd := neuerSchluessel(t)
	gefaelscht[0].Signing = fremd
	if pruefeSnapshotVerlauf(gefaelscht, bis) == nil {
		t.Fatal("gefaelschte Zeile angenommen")
	}
	if pruefeSnapshotVerlauf(append(snap.ValidatorVerlauf, snap.ValidatorVerlauf[0]), bis) == nil {
		t.Fatal("doppelte Zeile angenommen")
	}
	importiere := func(verlauf []SnapshotValidator, ersetzen bool) {
		t.Helper()
		tx, err := f.cs.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := validatorenImportieren(tx, snap.Validatoren, ersetzen, bis); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := verlaufImportieren(tx, verlauf, snap.Validatoren, ersetzen); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	// Ersetzen mit nur einer Verlaufszeile: Verlauf = diese plus die
	// Registereintraege (die immer mitkommen).
	var nichtImRegister SnapshotValidator
	for _, v := range snap.ValidatorVerlauf {
		imRegister := false
		for _, r := range snap.Validatoren {
			imRegister = imRegister || (r.Betreiber == v.Betreiber && r.Signing == v.Signing && r.Zeit == v.Zeit)
		}
		if !imRegister {
			nichtImRegister = v
		}
	}
	importiere(nil, true)
	if n := f.verlaufAnzahl(); n != len(snap.Validatoren) {
		t.Fatalf("ersetzt ohne Verlauf: %d Zeilen, erwartet %d (das Register)", n, len(snap.Validatoren))
	}
	importiere([]SnapshotValidator{nichtImRegister}, false)
	if n := f.verlaufAnzahl(); n != 3 || f.neuAufgebaut() != stand {
		t.Fatalf("zusammengefuehrt: %d Zeilen, Summe gleich: %v", n, f.neuAufgebaut() == stand)
	}
	importiere(snap.ValidatorVerlauf, true)
	if n := f.verlaufAnzahl(); n != 3 || f.neuAufgebaut() != stand {
		t.Fatalf("ersetzt: %d Zeilen, Summe gleich: %v", n, f.neuAufgebaut() == stand)
	}
}

// Fail-closed: laesst sich der Verlauf nicht schreiben, wird der Block
// abgewiesen -- die Bindung landet weder im Register noch in der Summe.
func TestValidatorVerlauf_NichtSchreibbarWeistBlockAb_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	b := f.betreiber()
	k, _ := neuerSchluessel(t)
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_verlauf RENAME TO validator_verlauf_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`) })
	vorher := f.summe()
	if f.block(f.jetzt, bindungUnterschrieben(t, b, k, f.jetzt-60)) {
		t.Fatal("Block trotz unschreibbarem Verlauf angenommen")
	}
	if f.anzahl() != 0 || f.summe() != vorher {
		t.Fatal("abgewiesener Block hat das Register veraendert")
	}
}
