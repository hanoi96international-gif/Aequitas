package keeper

import (
	"database/sql"
	"fmt"
	"testing"
)

// B5: nach einem Lesefehler wird ein frisches Konto fuer diese Adresse nicht
// geschrieben; nach einem gelungenen Lesen wieder.
func TestKontoUnlesbar_FrischesKontoNichtUeberschreiben(t *testing.T) {
	cs := newTestState()
	a := "0x00000000000000000000000000000000000000a1"
	cs.kontoUnlesbarMerken(a)
	if err := cs.saveAccountToDBInnerCtx(t.Context(), &AccountState{Address: a}); err == nil {
		t.Fatal("frisches Konto trotz Lesefehler geschrieben")
	}
	// Ein bekanntes Konto (Version > 0) laeuft ueber die Versionspruefung und
	// ist nicht betroffen.
	if err := cs.saveAccountToDBInnerCtx(t.Context(), &AccountState{Address: a, Version: 3}); err != nil {
		t.Fatalf("bekanntes Konto abgewiesen: %v", err)
	}
	cs.kontoLesbar("0x00000000000000000000000000000000000000A1") // Gross/Klein egal
	if err := cs.saveAccountToDBInnerCtx(t.Context(), &AccountState{Address: a}); err != nil {
		t.Fatalf("nach gelungenem Lesen weiter gesperrt: %v", err)
	}
}

// Begrenzt: bei Ueberlauf gilt jede Adresse als unlesbar (fail-closed), die
// Liste waechst nicht ueber die Grenze.
func TestKontoUnlesbar_Begrenzt(t *testing.T) {
	cs := newTestState()
	for i := 0; i < kontoUnlesbarHoechstens+50; i++ {
		cs.kontoUnlesbarMerken(fmt.Sprintf("0x%040x", i))
	}
	if n := cs.unlesbareAnzahl.Load(); n > kontoUnlesbarHoechstens {
		t.Fatalf("%d Eintraege, Grenze %d", n, kontoUnlesbarHoechstens)
	}
	if !cs.kontoUnlesbar("0x00000000000000000000000000000000deadbeef") {
		t.Fatal("Ueberlauf nicht fail-closed")
	}
	for i := 0; i < kontoUnlesbarHoechstens; i++ {
		cs.kontoLesbar(fmt.Sprintf("0x%040x", i))
	}
	if cs.kontoUnlesbar("0x00000000000000000000000000000000deadbeef") {
		t.Fatal("Ueberlauf bleibt nach Erholung bestehen")
	}
}

// Pruefung von #515, Befund 2: der oeffentliche Lesepfad (kontoNurLesen)
// merkt sich Datenbankfehler NICHT. Sonst fuellte eine Lese-Flut mit
// erfundenen Adressen waehrend eines Datenbank-Aussetzers die Liste bis zum
// Ueberlauf, und danach galt jede Adresse als unlesbar.
func TestKontoUnlesbar_LesepfadFuelltNicht(t *testing.T) {
	cs := newTestState()
	// Port 1: Verbindung wird abgelehnt -- ein echter Fehler, kein ErrNoRows.
	db, err := sql.Open("postgres", "host=127.0.0.1 port=1 user=x dbname=x sslmode=disable connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cs.db = db
	for i := 0; i < 50; i++ {
		if _, ok := cs.kontoNurLesen(fmt.Sprintf("0x%040x", i)); ok {
			t.Fatal("Lesen ohne Datenbank gelungen")
		}
	}
	if n := cs.unlesbareAnzahl.Load(); n != 0 {
		t.Fatalf("Lesepfad hat %d Adressen als unlesbar vermerkt", n)
	}
	if cs.kontoUnlesbar("0x0000000000000000000000000000000000000001") {
		t.Fatal("Lesepfad sperrt das Schreiben")
	}
}
