package keeper

import (
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
