package keeper

import (
	"fmt"
	"testing"
)

// EINE ANGENOMMENE UEBERWEISUNG DARF NICHT EINE STUNDE LIEGEN BLEIBEN.
//
// LoadPendingTxsWithLimit markiert die geladenen Zeilen mit included_at und
// COMMITET das sofort -- bevor in ProduceBlock irgendein Tor gelaufen ist.
// Bricht eines ab (kein Vorrang, Sync noch am Aufholen, Partner zu weit
// zurueck, Zustandswurzel nicht bildbar), bleiben bis zu blockTxCap() Zeilen
// markiert liegen, ohne je in einem Block zu stehen.
//
// Der Kommentar in ProduceBlock behauptete das Gegenteil ("die Transaktionen
// bleiben pending, weil nichts markiert wurde"). Eingesammelt wurden sie erst
// vom Aufraeumer -- Nachlauf bis zu einer Stunde. Fuer den Menschen heisst
// das: die Ueberweisung ist angenommen, sie ist bezahlt, und sie passiert
// eine Stunde lang nicht.
//
// Braucht eine echte Datenbank: ohne sie ist pending_txs ein No-op in beide
// Richtungen, ein Test ohne Postgres waere gruen, ohne etwas zu pruefen.

func TestAusgangskorb_FreigabeHoltAbgebrocheneProduktionZurueck_RealDB(t *testing.T) {
	truncateDistTestTables(t) // auch das Opt-in-Tor
	cs := testKnoten(t, "unused-ausgangskorb-freigabe-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}

	const anzahl = 5
	for i := 0; i < anzahl; i++ {
		tx := Transaction{
			Type: "transfer", Wallet: distTestAddr(700 + i), To: distTestAddr(800 + i),
			Amount: 1, TxHash: fmt.Sprintf("0xfreigabe-%d", i),
		}
		if err := cs.SavePendingTx(tx); err != nil {
			t.Fatalf("Ausgangskorb fuellen: %v", err)
		}
	}

	// Das tut ProduceBlock als Erstes: laden -- und damit markieren.
	txs, ids := cs.LoadPendingTxsWithLimit(anzahl)
	if len(txs) != anzahl || len(ids) != anzahl {
		t.Fatalf("geladen: %d Ueberweisungen / %d IDs, erwartet je %d", len(txs), len(ids), anzahl)
	}
	if offen := offeneAusgangskorbZeilen(t, cs); offen != 0 {
		t.Fatalf("nach dem Laden sind %d Zeilen offen, erwartet 0 -- der Test trifft die Markierung nicht", offen)
	}

	// Jetzt bricht ein Tor ab. Ohne Freigabe blieben sie bis zum Aufraeumer
	// liegen; genau das ist der Befund.
	cs.PendingTxIDsFreigeben(ids)

	if offen := offeneAusgangskorbZeilen(t, cs); offen != anzahl {
		t.Fatalf("nach dem Abbruch sind %d von %d Zeilen wieder offen, erwartet alle -- "+
			"sonst wartet eine angenommene Ueberweisung bis zu eine Stunde auf den Aufraeumer", offen, anzahl)
	}

	// Und der naechste Produktionsversuch sieht sie tatsaechlich wieder.
	txs2, ids2 := cs.LoadPendingTxsWithLimit(anzahl)
	if len(txs2) != anzahl || len(ids2) != anzahl {
		t.Fatalf("der naechste Block sieht %d Ueberweisungen, erwartet %d", len(txs2), anzahl)
	}
}

// Eine Zeile, die inzwischen doch in einem Block gelandet ist, darf die
// Freigabe NICHT anfassen -- sonst wird sie ein zweites Mal eingebaut.
func TestAusgangskorb_FreigabeLaesstEingebauteZeilenInRuhe_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-ausgangskorb-freigabe-eingebaut-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}

	tx := Transaction{Type: "transfer", Wallet: distTestAddr(701), To: distTestAddr(801), Amount: 1, TxHash: "0xfreigabe-eingebaut"}
	if err := cs.SavePendingTx(tx); err != nil {
		t.Fatalf("Ausgangskorb fuellen: %v", err)
	}
	_, ids := cs.LoadPendingTxsWithLimit(1)
	if len(ids) != 1 {
		t.Fatalf("geladen: %d IDs, erwartet 1", len(ids))
	}
	// So sieht es aus, wenn ein Block sie traegt.
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_block_hash = $1 WHERE id = $2`, "0xblock", ids[0]); err != nil {
		t.Fatalf("Blockbindung setzen: %v", err)
	}

	cs.PendingTxIDsFreigeben(ids)

	if offen := offeneAusgangskorbZeilen(t, cs); offen != 0 {
		t.Fatalf("%d Zeile(n) wieder offen -- eine Ueberweisung, die in einem Block steht, "+
			"darf nicht ein zweites Mal eingebaut werden", offen)
	}
}

func offeneAusgangskorbZeilen(t *testing.T, cs *ChainState) int {
	t.Helper()
	var n int
	if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE included_at = 0`).Scan(&n); err != nil {
		t.Fatalf("offene Zeilen zaehlen: %v", err)
	}
	return n
}
