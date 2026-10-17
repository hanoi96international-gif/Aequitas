package keeper

import (
	"os"
	"strconv"
	"testing"
)

// Missbrauch / Fehlerfall (Sicherheitspruefung #389, info): beim Nachspielen
// einer Tagesrunde liest jeder Knoten last_distribution_round_at, um eine
// zweite Runde binnen 24 h zu verwerfen (distributionRoundToSkip). Frueher
// machte ein unlesbarer Wert still eine 0 daraus -- die Runde galt als "nie
// gelaufen" und wurde angewandt, obwohl sie haette verworfen werden muessen.
// Jetzt: nicht lesbar -> Block nicht angewandt (Backoff versucht spaeter
// erneut), nichts geschrieben. Gegenproben: lesbare Marke binnen 24 h ->
// Runde verworfen; lesbare alte Marke -> Runde angewandt.
func TestReplay_VerteilungsmarkeUnlesbar_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-verteilmarke-nachspielen-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = map[string]replayFailureState{}
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	jetzt := nowUnix()
	runde := func(n int) *Block {
		return &Block{
			Height:       int64(n),
			Hash:         "verteilmarke-nachspielen-" + strconv.Itoa(n),
			Timestamp:    jetzt,
			Transactions: []Transaction{{Type: "distribution_round_marker", DistributionAt: jetzt}},
		}
	}
	marke := func() string { return cs.getConfigValueDB("last_distribution_round_at") }

	for i, kaputt := range []string{"kaputt", "1792000000x", "1.79e9"} {
		if err := cs.setConfigValueDB("last_distribution_round_at", kaputt); err != nil {
			t.Fatal(err)
		}
		if dag.replayTransactions(runde(1+i), true) {
			t.Fatalf("Marke %q unlesbar: Block angewandt, want abgelehnt (fail-closed)", kaputt)
		}
		if got := marke(); got != kaputt {
			t.Fatalf("Marke nach abgelehntem Block = %q, want unveraendert %q", got, kaputt)
		}
	}
	if err := cs.setConfigValueDB("last_distribution_round_at", "1"); err != nil {
		t.Fatal(err)
	}
	if err := cs.setConfigValueDB("last_ubi_at", "kaputt"); err != nil {
		t.Fatal(err)
	}
	if dag.replayTransactions(runde(10), true) {
		t.Fatal("last_ubi_at unlesbar: Block angewandt, want abgelehnt (fail-closed)")
	}
	if err := cs.setConfigValueDB("last_ubi_at", "0"); err != nil {
		t.Fatal(err)
	}

	// Nach einer Ablehnung heilt der Block, sobald die Marke wieder lesbar
	// ist: nicht als nachgespielt vermerkt, der naechste Versuch wendet ihn an.
	dag.replayedMu.Lock()
	_, fehlVermerkt := dag.replayFailures[runde(1).Hash]
	schonNachgespielt := dag.replayedBlocks[runde(1).Hash]
	dag.replayedMu.Unlock()
	if !fehlVermerkt || schonNachgespielt {
		t.Fatalf("abgelehnter Block: replayFailures=%v replayed=%v, want true false", fehlVermerkt, schonNachgespielt)
	}
	if err := cs.setConfigValueDB("last_distribution_round_at", strconv.FormatInt(jetzt-2*86400, 10)); err != nil {
		t.Fatal(err)
	}
	if !dag.replayTransactions(runde(1), true) {
		t.Fatal("abgelehnter Block nach Heilung der Marke nicht angewandt")
	}
	if got := marke(); got != strconv.FormatInt(jetzt, 10) {
		t.Fatalf("nach Heilung: Marke %q, want %d", got, jetzt)
	}

	// MISSBRAUCH (Befund mittel): ein negativer Zeitpunkt ist von der Kette
	// schreibbar (ubi_distribution_finalize mit DistributionAt -1). Er darf
	// keinen Knoten anhalten -- er gilt wie vorher als "nie gelaufen".
	if err := cs.setConfigValueDB("last_distribution_round_at", "1"); err != nil {
		t.Fatal(err)
	}
	if err := cs.setConfigValueDB("last_ubi_at", "-1"); err != nil {
		t.Fatal(err)
	}
	if !dag.replayTransactions(runde(15), true) {
		t.Fatal("last_ubi_at = -1: Knoten steht (Block abgelehnt), want nachgespielt")
	}
	if err := cs.setConfigValueDB("last_ubi_at", "0"); err != nil {
		t.Fatal(err)
	}

	// Lesbar, letzte Runde vor 60 s: zweite Runde verworfen, Marke bleibt.
	frisch := strconv.FormatInt(jetzt-60, 10)
	if err := cs.setConfigValueDB("last_distribution_round_at", frisch); err != nil {
		t.Fatal(err)
	}
	if !dag.replayTransactions(runde(20), true) {
		t.Fatal("lesbare Marke: Block muss nachgespielt werden (Runde darin verworfen)")
	}
	if got := marke(); got != frisch {
		t.Fatalf("zweite Runde binnen 24 h angewandt: Marke %q, want %q", got, frisch)
	}

	// Lesbar, letzte Runde vor zwei Tagen: Runde angewandt.
	if err := cs.setConfigValueDB("last_distribution_round_at", strconv.FormatInt(jetzt-2*86400, 10)); err != nil {
		t.Fatal(err)
	}
	if !dag.replayTransactions(runde(30), true) {
		t.Fatal("Gutfall: Block nicht nachgespielt")
	}
	if got := marke(); got != strconv.FormatInt(jetzt, 10) {
		t.Fatalf("Gutfall: Marke %q, want %d", got, jetzt)
	}
}
