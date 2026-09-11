package keeper

import (
	"os"
	"strings"
	"testing"
)

// Diese Begrenzung war zweimal hintereinander scheinbar gesund und tatsaechlich
// wirkungslos, und beide Male sah es von aussen gleich aus: "0 entfernt". Erst
// summierte sie pg_column_size ueber 51 Millionen Zeilen und lief ins
// statement_timeout; dann sortierte sie nach block_height, wofuer es keinen
// Index gab, und lief wieder ins Zeitlimit. Gemeldet wurde beide Male eine
// beruhigende Null, waehrend die Tabelle bei 13,8 GB stand und kurz darauf
// beide Boxen mit voller Platte standen.
//
// Die Tests hier pruefen darum nicht, DASS geloescht wird -- das braucht eine
// Datenbank --, sondern die vier Eigenschaften, deren Verlust genau diese
// stille Null zurueckbringt.

func quelle(t *testing.T, datei string) string {
	t.Helper()
	b, err := os.ReadFile(datei)
	if err != nil {
		t.Fatalf("%s nicht lesbar: %v", datei, err)
	}
	return string(b)
}

func TestTxIndexPrune_LoeschtOhneSortierungUeberEineIndizierteSpalte(t *testing.T) {
	body := quelle(t, "tx_index_prune.go")

	if strings.Contains(body, "ORDER BY block_height") {
		t.Error("das Loeschen sortiert wieder nach block_height. Eine sortierte Auswahl kann " +
			"den Hoehenindex nicht nutzen, wenn sie ueber ctid geht -- sie scannt die ganze " +
			"Tabelle, reisst das Zeitlimit und entfernt nichts, meldet aber 0 statt eines " +
			"Fehlers. Stattdessen eine Bedingung WHERE block_height < grenze verwenden.")
	}
	if !strings.Contains(body, "DELETE FROM chain_tx_block_index WHERE block_height < $1") {
		t.Error("die Loeschbedingung ueber die Hoehe ist weg. Sie ist die einzige Form, die " +
			"den Index nutzen kann und trotzdem die aeltesten Zeilen zuerst trifft.")
	}
}

func TestTxIndexPrune_KuerztNichtOhneIndex(t *testing.T) {
	body := quelle(t, "tx_index_prune.go")

	if !strings.Contains(body, "hoehenIndexDa()") {
		t.Fatal("die Begrenzung prueft nicht mehr, ob der Hoehenindex existiert. Ohne ihn ist " +
			"jedes Loeschen ein Tabellenscan: es laeuft ins Zeitlimit und meldet 0 entfernt. " +
			"Lieber gar nicht kuerzen und das sagen, als eine Null melden, die nach Ordnung " +
			"aussieht.")
	}
	vorIndex := strings.Index(body, "hoehenIndexDa()")
	vorDelete := strings.Index(body, "DELETE FROM chain_tx_block_index")
	if vorIndex > vorDelete {
		t.Error("die Indexpruefung steht hinter dem Loeschen statt davor und kann es damit " +
			"nicht mehr verhindern.")
	}
}

func TestTxIndexPrune_BautDenIndexOhneSchreibsperre(t *testing.T) {
	body := quelle(t, "tx_index_prune.go")

	if !strings.Contains(body, "CREATE INDEX CONCURRENTLY idx_tx_block_index_hoehe") {
		t.Error("der Hoehenindex wird nicht mehr CONCURRENTLY gebaut. Ein gewoehnliches " +
			"CREATE INDEX sperrt chain_tx_block_index gegen Schreibzugriffe, und in diese " +
			"Tabelle schreibt JEDER angenommene Block -- auf 49 Millionen Zeilen waere das " +
			"ein Stillstand der Blockhoehe von Minuten.")
	}
	// CONCURRENTLY laeuft nicht in einer Transaktion, also greift das bewaehrte
	// SET LOCAL aus snapshot.go hier nicht; es braucht eine eigene Verbindung.
	if !strings.Contains(body, "cs.db.Conn(ctx)") || !strings.Contains(body, "SET statement_timeout = 0") {
		t.Error("die lange Anweisung laeuft nicht mehr auf einer eigenen Verbindung mit " +
			"aufgehobenem Zeitlimit. Ein SET auf dem Verbindungsvorrat traefe irgendeine " +
			"Verbindung, und der Indexaufbau liefe wieder in die fuenf Sekunden.")
	}
	if !strings.Contains(body, "DROP INDEX IF EXISTS idx_tx_block_index_hoehe") {
		t.Error("ein fehlgeschlagener CONCURRENTLY-Aufbau hinterlaesst in Postgres einen " +
			"ungueltigen Index, den der naechste Versuch nicht ueberschreibt. Ohne das " +
			"vorherige Aufraeumen bliebe die Begrenzung dauerhaft aus.")
	}
}

func TestTxIndexPrune_IndexEntstehtNichtImStartpfad(t *testing.T) {
	body := quelle(t, "tx_block_index.go")

	if strings.Contains(body, "CREATE INDEX") {
		t.Error("der Hoehenindex wird wieder beim Anlegen der Tabelle erzeugt. Auf einer " +
			"leeren Tabelle ist das kostenlos, auf einer gewachsenen dauert es Minuten -- " +
			"in einer Anweisung mit fuenf Sekunden Zeitlimit, die zudem die Blockannahme " +
			"sperrt. Er gehoert in den Hintergrund, siehe legeHoehenIndexAn().")
	}
}

// Die Zeilenzahl darf nicht per count(*) kommen: auch das ist in Postgres ein
// vollstaendiger Scan und liefe in dasselbe Zeitlimit wie die Byte-Summe, die
// hier schon einmal eine 0 MB bei 15 GB Tabelle gemeldet hat.
func TestTxIndexPrune_MisstUeberReltuplesUndMeldetFehler(t *testing.T) {
	body := quelle(t, "tx_index_prune.go")

	if strings.Contains(body, "SELECT count(*) FROM chain_tx_block_index") {
		t.Error("die Groesse wird wieder per count(*) gemessen -- ein Tabellenscan ueber " +
			"Millionen Zeilen, der ins Zeitlimit laeuft.")
	}
	if !strings.Contains(body, "reltuples") {
		t.Error("reltuples ist weg; ohne die Statistik-Schaetzung ist die Messung wieder ein Scan.")
	}
	if !strings.Contains(body, "Groesse nicht messbar") {
		t.Error("ein Messfehler wird nicht mehr gemeldet. Genau dieses stille Schlucken hat " +
			"die Begrenzung wie funktionierend aussehen lassen: 0 MB bei 15 GB Tabelle.")
	}
}

// Der zweite Fehler war das Gegenteil des ersten: die Begrenzung loeschte
// nicht zu wenig, sondern zu viel. Aus 49.366.584 Zeilen wurden 95.357 statt
// der rund sieben Millionen, die ins Budget gepasst haetten. Grund: reltuples
// aendert sich durch ein DELETE nicht, sondern erst durch ANALYZE -- die
// Schleife sah nach jedem Loeschen weiterhin den alten Stand.
func TestTxIndexPrune_SchreibtDieSchaetzungFortStattSieNeuZuLesen(t *testing.T) {
	body := quelle(t, "tx_index_prune.go")

	schleife := strings.Index(body, "for i := 0; i < 200; i++ {")
	if schleife < 0 {
		t.Fatal("die Loeschschleife ist nicht mehr auffindbar")
	}
	messung := strings.Index(body, "cs.zeilenSchaetzung()")
	if messung < 0 {
		t.Fatal("die Zeilenschaetzung ist weg")
	}
	if messung > schleife {
		t.Error("die Schaetzung wird wieder INNERHALB der Schleife gelesen. reltuples folgt " +
			"einem DELETE nicht, also sieht jeder Durchgang den alten Stand und loescht " +
			"weiter -- am 11.09.2026 bis auf 0,2 Prozent des Budgets herunter.")
	}
	if !strings.Contains(body, "zeilen -= n") {
		t.Error("die geloeschte Zeilenzahl wird nicht mehr von der Schaetzung abgezogen. " +
			"Ohne sie kann die Schleife nicht erkennen, dass sie das Budget bereits " +
			"erreicht hat.")
	}
	if !strings.Contains(body, "ANALYZE chain_tx_block_index") {
		t.Error("ANALYZE ist weg. Die Fortschreibung traegt nur innerhalb eines Durchgangs; " +
			"ueber Durchgaenge hinweg braucht der naechste Lauf den echten Wert.")
	}
}
