package keeper

import (
	"fmt"
	"sync"
	"time"
)

// AUSDRUCKS-INDIZES FUER lower(...)-ABFRAGEN.
//
// Viele Abfragen filtern mit `WHERE lower(address) = $1` (Adressen kommen
// gemischt geschrieben an: Pruefsummen-Schreibweise aus Wallets, klein aus
// dem Knoten). Ein Primaerschluessel auf `address` hilft dabei nicht --
// Postgres kann einen Index auf der Spalte nicht fuer eine Funktion der
// Spalte benutzen und liest die ganze Tabelle.
//
// Gemessen am 01.10.2026 auf dem lokalen Pruefstand: LoadNonce (jede
// Ueberweisung ueber RPC) lief als Seq Scan ueber evm_nonces. Mit 1.000
// Konten 0,7 ms, linear wachsend mit jedem Konto des Netzes -- auf der alten
// Kette mit Hunderttausenden Adressen ein Vielfaches davon, und das auf dem
// Weg, auf dem jede Ueberweisung auf eine Datenbankverbindung wartet.
//
// Die Indizes aendern KEIN Ergebnis: dieselben Abfragen, dieselbe Semantik,
// nur mit Index. Gebaut nebenlaeufig (CREATE INDEX CONCURRENTLY, ohne
// Tabellensperre) im Hintergrund nach dem Start -- auf einer grossen Tabelle
// dauert das, blockiert aber weder Lesen noch Schreiben. Schlaegt einer fehl,
// laufen die Abfragen wie bisher (nur langsamer), nichts wird uebersprungen.
var lowerIndizes = []struct{ name, definition string }{
	{"idx_evm_nonces_lower", "evm_nonces (lower(address))"},
	{"idx_chain_accounts_lower", "chain_accounts (lower(address))"},
	{"idx_evm_storage_lower", "evm_storage (lower(address), slot)"},
	{"idx_evm_contracts_lower", "evm_contracts (lower(address))"},
	{"idx_chain_blocks_proposer_lower", "chain_blocks (lower(proposer), timestamp)"},
	{"idx_guardians_wallet_lower", "guardians (lower(wallet_address))"},
	{"idx_guardians_guardian_lower", "guardians (lower(guardian_address))"},
	{"idx_registered_nodes_signing_lower", "registered_nodes (lower(signing_address))"},
}

// lowerIndizesSicherstellen: im Hintergrund, einmal je Start. Ein fehlender
// Index (Tabelle noch nicht angelegt) ist kein Fehler des Knotens.
func (cs *ChainState) lowerIndizesSicherstellen() {
	if cs == nil || cs.db == nil {
		return
	}
	SafeGoroutine("lower-indizes", func() {
		// Erst nach dem Start: CREATE INDEX CONCURRENTLY haelt eine Sperre,
		// mit der ein ALTER TABLE ... ADD COLUMN nicht gleichzeitig laufen
		// kann. Beim Start ziehen mehrere Stellen Spalten nach (einmalig,
		// Fehler werden dort nicht wiederholt) -- gemessen am 01.10.2026 auf
		// dem Pruefstand: das ALTER von chain_blocks.blue_score wartete hinter
		// dem Indexbau, lief ins 5-s-Limit, und der Knoten beendete sich mit
		// "column blue_score does not exist".
		time.Sleep(lowerIndizesVerzoegerung)
		cs.lowerIndizesBauen()
	})
}

// lowerIndizesVerzoegerung: Abstand zum Start (siehe oben). In Tests 0.
var lowerIndizesVerzoegerung = 30 * time.Second

var lowerIndizesMu sync.Mutex

// lowerIndizesBauen baut alle fehlenden Indizes und liefert die Namen der
// nicht gebauten.
func (cs *ChainState) lowerIndizesBauen() (fehlend []string) {
	// Einer zur Zeit: zwei gleichzeitige CREATE INDEX CONCURRENTLY auf
	// denselben Namen scheitern beide oder hinterlassen einen ungueltigen.
	lowerIndizesMu.Lock()
	defer lowerIndizesMu.Unlock()
	// Die Spalten-Migrationen der Tabellen, die hier einen Index bekommen,
	// VOR dem Indexbau -- sie sind einmalig je Prozess und idempotent.
	cs.ensureGHOSTDAGColumns()
	cs.ensureReplayedColumn()
	cs.ensureTxRootColumn()
	for _, ix := range lowerIndizes {
		if err := cs.indexNebenlaeufigSicherstellen(ix.name, ix.definition); err != nil {
			fmt.Printf("[INDEX] %s nicht gebaut: %v -- Abfragen laufen weiter ohne ihn\n", ix.name, err)
			fehlend = append(fehlend, ix.name)
		}
	}
	return fehlend
}
