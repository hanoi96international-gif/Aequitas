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
	t.Setenv("IS_PRIMARY_NODE", "")
	genesisFuerTest(t, `{"chain_id":"aequitas-testnetz-7","genesis_time":"2026-10-01T00:00:00Z"}`, nil)
	if err := PruefeProduzentenliste(); err != nil {
		t.Fatalf("ausdruecklich offene Liste im Testnetz abgewiesen: %v", err)
	}
}

// Sicherheitspruefung des Diffs, F6: im Hauptnetz oeffnet die Erklaerung die
// Liste nicht -- und ist das Netz nicht feststellbar, gilt es als Hauptnetz.
func TestProduzentenliste_OffenNieImHauptnetz(t *testing.T) {
	t.Setenv("AUTHORIZED_VALIDATORS", "")
	t.Setenv("AEQUITAS_PRODUZENTEN_OFFEN", "ja")
	t.Setenv("IS_PRIMARY_NODE", "")
	for name, g := range map[string]string{
		"Hauptnetz":    `{"chain_id":"aequitas-1","genesis_time":"2026-10-21T15:00:00Z"}`,
		"unlesbar":     `{kaputt`,
		"ohne Kennung": `{"genesis_time":"2026-10-01T00:00:00Z"}`,
	} {
		genesisFuerTest(t, g, nil)
		if PruefeProduzentenliste() == nil {
			t.Errorf("%s: offene Liste zugelassen", name)
		}
	}
	genesisFuerTest(t, `{"chain_id":"aequitas-testnetz-7","genesis_time":"2026-10-01T00:00:00Z"}`, nil)
	t.Setenv("IS_PRIMARY_NODE", "true")
	if PruefeProduzentenliste() == nil {
		t.Errorf("IS_PRIMARY_NODE: offene Liste zugelassen")
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
