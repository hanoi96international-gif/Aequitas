package keeper

import (
	"crypto/ecdsa"
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

// Wechsel und Uebernahme stehen im Verlauf; eine aeltere Bindung, die erst
// spaeter ankommt, wird abgewiesen und hinterlaesst keine Zeile. Die
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
	// Aelter als b1's Bindung an k2: abgewiesen, weder Register noch Verlauf.
	vorher := f.summe()
	alt := bindungUnterschrieben(t, b1, k3, j-250)
	if !f.block(j, alt) {
		t.Fatal("Block abgewiesen")
	}
	if s, _, _ := f.eintrag(b1); s == adrVon(k3) {
		t.Fatal("aeltere Bindung hat das Register geaendert")
	}
	if n := f.verlaufAnzahl(); n != 3 || f.summe() != vorher || f.neuAufgebaut() != vorher {
		t.Fatalf("aeltere Bindung: %d Zeilen, Summe unveraendert: %v", n, f.summe() == vorher)
	}
	// Wiederholung einer angenommenen Bindung: nichts doppelt.
	if !f.block(j, bindungUnterschrieben(t, b2, k1, j-100)) {
		t.Fatal("Block abgewiesen")
	}
	if f.verlaufAnzahl() != 3 || f.summe() != vorher {
		t.Fatal("Wiederholung hat den Verlauf veraendert")
	}
	// Zurueckgewiesener Block: keine Zeile, die Summe wie vorher.
	k4, _ := neuerSchluessel(t)
	if f.block(j, bindungUnterschrieben(t, b2, k4, j-10), gift()) {
		t.Fatal("vergifteter Block angenommen")
	}
	if f.verlaufAnzahl() != 3 || f.summe() != vorher || f.neuAufgebaut() != vorher {
		t.Fatal("zurueckgewiesener Block hat den Verlauf veraendert")
	}
	// Im Verlauf steht, wer s1 wann hielt -- b1 bis zu seinem Wechsel, b2
	// seit der Uebernahme.
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
		vonB1 = vonB1 || (i.betreiber == adrVon(b1) && i.von == j-300 && i.bis == j-200)
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
	// Je eine der beiden Unterschriften aus einer anderen Zeile: gueltig
	// geformt, aber nicht ueber diese Bindung.
	for _, welche := range []string{"Betreiber", "Signierschluessel"} {
		vertauscht := append([]SnapshotValidator(nil), snap.ValidatorVerlauf...)
		if welche == "Betreiber" {
			vertauscht[0].SigOperator = vertauscht[1].SigOperator
		} else {
			vertauscht[0].SigSigning = vertauscht[1].SigSigning
		}
		if pruefeSnapshotVerlauf(vertauscht, bis) == nil {
			t.Fatalf("fremde Unterschrift des %s angenommen", welche)
		}
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

// Missbrauch (H1, #303): ein Mensch fuellt den Verlauf nicht. Je Betreiber
// hoechstens eine Bindung je validatorBindungAbstand -- 50 Bindungen
// desselben Schluessels in einem Block ergeben eine Zeile; eine neuere
// innerhalb des Abstands wird abgewiesen, ohne Zeile; nach dem Abstand gilt
// die naechste.
func TestValidatorVerlauf_HoechstensEineBindungJeAbstand_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	validatorBindungAbstandOverride.Store(0) // die echte Regel: ein Tag
	if _, err := f.cs.db.Exec(`DELETE FROM validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	b := f.betreiber()
	k, _ := neuerSchluessel(t)
	j := f.jetzt
	var viele []Transaction
	for i := 0; i < 50; i++ {
		viele = append(viele, bindungUnterschrieben(t, b, k, j-3000+int64(i)*60))
	}
	if !f.block(j, viele...) {
		t.Fatal("Block abgewiesen")
	}
	if n := f.verlaufAnzahl(); n != 1 {
		t.Fatalf("%d Zeilen aus 50 Bindungen, erwartet 1", n)
	}
	if _, z, _ := f.eintrag(b); z != j-3000 {
		t.Fatalf("Register auf %d, erwartet die erste Bindung %d", z, j-3000)
	}
	// Nach dem Abstand: die naechste gilt.
	spaeter := j - 3000 + validatorBindungAbstandSek
	k2, s2 := neuerSchluessel(t)
	if !f.block(spaeter, bindungUnterschrieben(t, b, k2, spaeter)) {
		t.Fatal("Block abgewiesen")
	}
	if s, z, _ := f.eintrag(b); s != s2 || z != spaeter || f.verlaufAnzahl() != 2 {
		t.Fatalf("nach dem Abstand: %s %d, %d Zeilen", s, z, f.verlaufAnzahl())
	}
	if f.summe() != f.neuAufgebaut() {
		t.Fatal("Summe weicht vom Neuaufbau ab")
	}
}

// Auch ein Snapshot bringt keinen dichteren Verlauf mit.
func TestPruefeSnapshotVerlauf_Abstand(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	b, _ := neuerSchluessel(t)
	k1, _ := neuerSchluessel(t)
	k2, _ := neuerSchluessel(t)
	j := nowUnix()
	zeile := func(k *ecdsa.PrivateKey, zeit int64) SnapshotValidator {
		tx := bindungUnterschrieben(t, b, k, zeit)
		return SnapshotValidator{Betreiber: tx.Wallet, Signing: tx.To, Zeit: zeit, SigOperator: tx.Nachweis.Sig, SigSigning: tx.Nachweis.Sig2}
	}
	bis := snapshotValidatorenBis(j)
	weit := []SnapshotValidator{zeile(k1, j-validatorBindungAbstandSek-60), zeile(k2, j-60)}
	if err := pruefeSnapshotVerlauf(weit, bis); err != nil {
		t.Fatalf("Abstand eingehalten, trotzdem abgewiesen: %v", err)
	}
	dicht := []SnapshotValidator{zeile(k1, j-120), zeile(k2, j-60)}
	if pruefeSnapshotVerlauf(dicht, bis) == nil {
		t.Fatal("zwei Bindungen eines Betreibers im Minutenabstand angenommen")
	}
}
