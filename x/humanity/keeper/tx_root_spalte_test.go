package keeper

import (
	"testing"
)

// DER KOPF-MODUS GRIFF NIE, WEIL DEM KOPF DER tx_root FEHLTE.
//
// stripBlocksForPeer bricht als Erstes ab, wenn b.TxRoot leer ist, und
// liefert den Block MIT Rumpf aus. chain_blocks hatte keine Spalte dafuer --
// also traf das auf jeden Block zu, der aus der Datenbank kam. Ergebnis:
// beide Boxen luden alle zwei Sekunden die letzten zwanzig Hoehen komplett
// mit Ruempfen voneinander nach (15.09.2026, Lauf 7: 1,92 GB ueber
// /api/blocks in neun Minuten; im Leerlaufprofil handleBlocks 36 %,
// doSyncOnce 28 %).
//
// Braucht eine echte Datenbank -- ohne sie gibt es keine Spalte, in der
// etwas fehlen koennte.

func TestTxRootSpalte_BlockAusDerDatenbankTraegtSeinenTxRoot_RealDB(t *testing.T) {
	truncateDistTestTables(t) // auch das Opt-in-Tor
	cs := NewChainState("unused-txroot-spalte-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	if _, err := cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, "0xtxroot-probe"); err != nil {
		t.Fatalf("aufraeumen: %v", err)
	}

	block := &Block{
		Hash: "0xtxroot-probe", Height: 4242, Timestamp: nowUnix(),
		Proposer: distTestAddr(900), ParentHashes: []string{"0xeltern"},
		Transactions: []Transaction{
			{Type: "transfer", Wallet: distTestAddr(901), To: distTestAddr(902), Amount: 1, TxHash: "0xt1"},
		},
	}
	block.TxRoot = txBatchRootJSON(block.transaktionenJSON(), len(block.Transactions))
	if block.TxRoot == "" {
		t.Fatal("der Testblock hat selbst keinen tx_root -- dann misst dieser Test nichts")
	}
	if err := cs.SaveBlockToDB(block, true); err != nil {
		t.Fatalf("Block speichern: %v", err)
	}

	// So kommt er beim Sync zurueck: als Kopf aus der Datenbank.
	geladen := cs.LoadBlockFromDBByHash("0xtxroot-probe")
	if geladen == nil {
		t.Fatal("Block nicht wieder geladen")
	}
	if geladen.TxRoot != block.TxRoot {
		t.Fatalf("tx_root ueberlebt die Datenbank nicht: gespeichert %q, geladen %q\n"+
			"Ohne ihn strippt stripBlocksForPeer nicht, und beide Boxen laden alle zwei "+
			"Sekunden die letzten zwanzig Hoehen mit Ruempfen voneinander nach.",
			block.TxRoot, geladen.TxRoot)
	}
}

// Und der Weg, den der Sync wirklich geht: die Seitenabfrage.
func TestTxRootSpalte_SeitenabfrageLiefertDenTxRootMit_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := NewChainState("unused-txroot-seite-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}

	// Eigene Zeilen aus frueheren Laeufen weg: SaveBlockToDB schreibt mit
	// ON CONFLICT (hash) DO NOTHING, eine bestehende Zeile bekaeme ihren
	// tx_root also nicht nachtraeglich -- genau das dokumentierte Verhalten
	// fuer Altbestand (siehe ensureTxRootColumn), hier aber ein Stolperstein.
	for i := 0; i < 3; i++ {
		if _, err := cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`,
			"0xtxroot-seite-"+string(rune('a'+i))); err != nil {
			t.Fatalf("aufraeumen: %v", err)
		}
	}

	var erwartet string
	for i := 0; i < 3; i++ {
		b := &Block{
			Hash: "0xtxroot-seite-" + string(rune('a'+i)), Height: int64(5000 + i), Timestamp: nowUnix(),
			Proposer: distTestAddr(910), ParentHashes: []string{"0xeltern"},
			Transactions: []Transaction{
				{Type: "transfer", Wallet: distTestAddr(911), To: distTestAddr(912), Amount: float64(i + 1), TxHash: "0xs" + string(rune('a'+i))},
			},
		}
		b.TxRoot = txBatchRootJSON(b.transaktionenJSON(), len(b.Transactions))
		if i == 0 {
			erwartet = b.TxRoot
		}
		if err := cs.SaveBlockToDB(b, true); err != nil {
			t.Fatalf("Block %d speichern: %v", i, err)
		}
	}

	// minHeight ist STRIKT kleiner-als (selectBlocksSince): 5000 wuerde den
	// ersten Block gerade ausschliessen.
	seite, err := cs.LoadBlocksSinceFromDB(4999, "", 10)
	if err != nil {
		t.Fatalf("Seitenabfrage: %v", err)
	}
	if len(seite) == 0 {
		t.Fatal("die Seitenabfrage lieferte nichts")
	}
	for _, b := range seite {
		if b.TxRoot == "" {
			t.Fatalf("Block %s kommt ohne tx_root aus der Seitenabfrage -- "+
				"genau der Weg, ueber den der Sync die letzten Hoehen holt", b.Hash)
		}
	}
	// Und der konkrete Block traegt genau seinen eigenen tx_root. Nicht ueber
	// seite[0]: die Seite kann Bloecke anderer Tests in derselben Hoehenlage
	// enthalten, und dann prueft die Position etwas anderes als gemeint.
	var gefunden bool
	for _, b := range seite {
		if b.Hash == "0xtxroot-seite-a" {
			gefunden = true
			if b.TxRoot != erwartet {
				t.Fatalf("tx_root von %s: %q, erwartet %q", b.Hash, b.TxRoot, erwartet)
			}
		}
	}
	if !gefunden {
		t.Fatal("der gespeicherte Block kam in der Seite nicht vor")
	}
}
