package keeper

import "testing"

// Missbrauch H1 (Audit 2026-10-21): ohne Liste haette ein Knoten jedem
// registrierten Menschen Bloecke geglaubt. Jetzt startet er nicht.
func TestProduzentenliste_LeerHaeltAn(t *testing.T) {
	t.Setenv("AUTHORIZED_VALIDATORS", "")
	t.Setenv("AEQUITAS_PRODUZENTEN_OFFEN", "")
	if PruefeProduzentenliste() == nil {
		t.Fatal("leere Produzentenliste ohne Erklaerung zugelassen")
	}
	t.Setenv("AEQUITAS_PRODUZENTEN_OFFEN", "vielleicht")
	if PruefeProduzentenliste() == nil {
		t.Fatal("nur \"ja\" erklaert die offene Liste")
	}
	t.Setenv("AEQUITAS_PRODUZENTEN_OFFEN", "ja")
	if err := PruefeProduzentenliste(); err != nil {
		t.Fatalf("ausdruecklich offene Liste abgewiesen: %v", err)
	}
}

func TestProduzentenliste_TippfehlerHaeltAn(t *testing.T) {
	t.Setenv("AEQUITAS_PRODUZENTEN_OFFEN", "ja") // haelt trotzdem: gesetzt heisst gemeint
	t.Setenv("AUTHORIZED_VALIDATORS", "0xa0d1000000000000000000000000000000000001,0xkaputt")
	if PruefeProduzentenliste() == nil {
		t.Fatal("Liste mit unlesbarem Eintrag zugelassen")
	}
	t.Setenv("AUTHORIZED_VALIDATORS", " 0xa0d1000000000000000000000000000000000001 , 0xA0D1000000000000000000000000000000000002")
	if err := PruefeProduzentenliste(); err != nil {
		t.Fatalf("gueltige Liste abgewiesen: %v", err)
	}
}
