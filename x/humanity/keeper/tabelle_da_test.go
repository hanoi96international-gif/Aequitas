package keeper

import (
	"os"
	"testing"
)

// Schema-Rauschen und Reihenfolge (01.10.2026): auf jeder neuen Datenbank
// schrieb der Knoten beim Start ERRORs ins Postgres-Protokoll -- ein DELETE
// auf eine Tabelle, die es nicht mehr gibt (mpc_share_buckets), und ein Index
// auf nullifiers, BEVOR die Tabelle angelegt war (Index erst ab dem zweiten
// Start). Und RestorePreUpgradeRelationshipSlots las jeden Abfragefehler als
// "Tabelle gibt es nicht".
func neuerSchemaTestZustand(t *testing.T) *ChainState {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("braucht DATABASE_URL (Wegwerf-Postgres)")
	}
	cs := NewChainState("unused-tabelle-da-test.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank -- DATABASE_URL pruefen")
	}
	t.Cleanup(func() { cs.db.Close() })
	return cs
}

func TestTabelleDa(t *testing.T) {
	cs := neuerSchemaTestZustand(t)
	if da, err := cs.tabelleDa("nullifiers"); err != nil || !da {
		t.Fatalf("nullifiers: da=%v err=%v, erwartet da", da, err)
	}
	if da, err := cs.tabelleDa("gibt_es_nicht_xyz"); err != nil || da {
		t.Fatalf("unbekannte Tabelle: da=%v err=%v, erwartet nicht da und kein Fehler", da, err)
	}
	// Ein Name mit SQL darin ist ein Wert, kein Befehl.
	if da, err := cs.tabelleDa("nullifiers; DROP TABLE nullifiers"); da {
		t.Fatalf("Name mit Anweisung als Tabelle erkannt (err=%v)", err)
	}
	if da, _ := cs.tabelleDa("nullifiers"); !da {
		t.Fatal("nullifiers ist verschwunden")
	}
}

func TestNullifierIndex_SchonBeimErstenStart(t *testing.T) {
	cs := neuerSchemaTestZustand(t)
	// Frische Datenbank fuer diese Tabelle nachstellen: weg damit, dann ein
	// neuer Start.
	if _, err := cs.db.Exec(`DROP TABLE IF EXISTS nullifiers`); err != nil {
		t.Fatal(err)
	}
	cs2 := neuerSchemaTestZustand(t)
	var da bool
	if err := cs2.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_nullifiers_wallet')`).Scan(&da); err != nil {
		t.Fatal(err)
	}
	if !da {
		t.Fatal("idx_nullifiers_wallet fehlt nach dem ersten Start auf frischer Tabelle")
	}
}

func TestRestoreRelationshipSlots_OhneTabelleKeinFehler(t *testing.T) {
	cs := neuerSchemaTestZustand(t)
	if _, err := cs.db.Exec(`DROP TABLE IF EXISTS ` + upgradeRelationshipSlotsTable); err != nil {
		t.Fatal(err)
	}
	if err := cs.RestorePreUpgradeRelationshipSlots("0x" + "ab"); err != nil {
		t.Fatalf("ohne Schnappschuss-Tabelle: %v, erwartet nil", err)
	}
	// Ohne Tabelle darf das Aufraeumen nach einer Einschreibung nicht scheitern.
	cs.deleteLeftoverMPCBuckets("egal")
}

func TestTxIndexBegrenzung_GreiftAufFrischerDatenbank(t *testing.T) {
	cs := neuerSchemaTestZustand(t)
	if _, err := cs.db.Exec(`DROP TABLE IF EXISTS chain_tx_block_index`); err != nil {
		t.Fatal(err)
	}
	cs2 := neuerSchemaTestZustand(t)
	cs2.pruneTxIndex()
	if !cs2.hoehenIndexDa() {
		t.Fatal("Hoehenindex fehlt nach dem ersten Durchgang auf frischer Datenbank -- Begrenzung greift nicht")
	}
	if da, err := cs2.tabelleDa("chain_tx_batches"); err != nil {
		t.Fatal(err)
	} else if !da {
		cs2.pruneTxBatches()
		if da, _ := cs2.tabelleDa("chain_tx_batches"); !da {
			t.Fatal("chain_tx_batches fehlt nach dem ersten Durchgang")
		}
	}
}
