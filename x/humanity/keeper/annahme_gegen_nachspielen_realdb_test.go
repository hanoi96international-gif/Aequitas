package keeper

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// DAS EXPERIMENT MIT ZWEI ECHTEN DATENBANKEN -- zwei Knoten, nicht zwei
// Codepfade in einem Speicher.
//
// annahme_gegen_nachspielen_test.go vergleicht Annahme und Nachspielen ohne
// Datenbank. Das reicht fuer den seriellen Pfad, aber nicht fuer die drei
// Pfade, die den Verkehr der Boxen tatsaechlich tragen: der Buendler, der
// Shard-Schnellpfad und der WAL-Schnellpfad steigen alle sofort aus, wenn
// cs.db nil ist. Genau dort vermutet die Checkliste die Staub-Divergenz
// ("Rundung im Batch-Pfad"), und genau dort war sie bisher nicht messbar.
//
// Dieses Experiment baut den Fall nach, wie er auf den Boxen entsteht:
//
//	Knoten A  nimmt die Ueberweisungen an (TransferAtomic, nebenlaeufig, also
//	          ueber die echten Schnellpfade) und schreibt sie in seinen
//	          Ausgangskorb -- eigene Datenbank.
//	Block     wird aus dem Ausgangskorb gebildet, in GENAU der Reihenfolge,
//	          in der ProduceBlock ihn liest: ORDER BY wal_seq, id.
//	Knoten B  spielt dieselben Bloecke nach -- eigene Datenbank, nie eine
//	          Ueberweisung selbst angenommen.
//
// Verglichen wird danach jedes Konto in MIKRO-AEQ, der kleinsten Einheit, die
// die Kette kennt. Eine Rundungsdifferenz im Buendelpfad muesste hier
// auftauchen, und zwar als genau die Groessenordnung, um die die Boxen
// auseinanderliefen.
//
// Die Ueberweisungsbetraege sind absichtlich unfreundlich gewaehlt: Werte, die
// in Mikro-AEQ nicht aufgehen (1/3, 0,1234565, 0,0000005 -- der klassische
// Halbe-Einheit-Fall), und Konten, die sich WIEDERHOLEN. Disjunkte Paare
// waeren der einfache Fall; die Boxen haben 649 Konten und Hunderttausende
// Ueberweisungen, dort trifft jedes Konto staendig auf sich selbst, und erst
// dann entscheidet die Reihenfolge ueber das Ergebnis.
//
// # WAS ER BISHER ERGEBEN HAT (18.09.2026)
//
// Keine Abweichung. Weder mit vollen Konten noch in dem Zustand, der den
// Lasttest ausmacht -- Konten, die leerlaufen:
//
//	Startguthaben 500        25.600 Ueberweisungen   0 Konten abweichend
//	Startguthaben 0,02        8.024 Ueberweisungen   0 Konten abweichend,
//	                                                 0 uebersprungen
//	                          (17.576 schon bei der Annahme abgelehnt)
//
// Damit sind zwei der drei Kandidaten aus der Checkliste erledigt: die
// RUNDUNG im Buendelpfad erzeugt auf 25.600 Ueberweisungen mit absichtlich
// unfreundlichen Betraegen keine einzige Mikro-Differenz, und die
// REIHENFOLGE (Ausgangskorb-Ordnung ist nicht Annahme-Ordnung) bleibt auch
// dann folgenlos, wenn Konten leerlaufen.
//
// Was dieses Experiment noch NICHT nachbaut, und was auf den Boxen die
// verbliebene Asymmetrie ist: C2 faehrt AEQUITAS_WAL_ENABLED=1 und
// ENABLE_MULTI_BLOCK_TICK, C1 nicht (siehe ANALYSE_STATEROOT_DIVERGENZ.md,
// Abschnitt F). Beide Knoten hier fahren dieselben Regeln. Das ist der
// naechste Kandidat.
//
// # AUFBAU
//
// Opt-in wie jeder andere _RealDB-Test hier: AEQUITAS_TPS_BENCH=1 und
// DATABASE_URL. Zusaetzlich AEQUITAS_DB_B mit einer ZWEITEN, ebenfalls
// entbehrlichen Datenbank -- ohne sie ueberspringt der Test, statt beide
// Knoten in dieselbe Datenbank zu schreiben (das waere kein Experiment,
// sondern ein Selbstgespraech). Zwei leere Datenbanken genuegen, das Schema
// legt der Test selbst an:
//
//	createdb aequitas_test && createdb aequitas_test_b
//	export AEQUITAS_TPS_BENCH=1
//	export DATABASE_URL='postgres://.../aequitas_test?sslmode=disable'
//	export AEQUITAS_DB_B='postgres://.../aequitas_test_b?sslmode=disable'
//	go test ./x/humanity/keeper/ -run TestAnnahmeGegenNachspielen_RealDB -v
//
// Groesser messen: die beiden Konstanten `konten` und `laeufe` in
// experimentLauf hochdrehen (64/400 ergibt die 25.600 oben).
func TestAnnahmeGegenNachspielen_RealDB_MikroGenau(t *testing.T) {
	experimentLauf(t, 500.0, false)
}

// TestAnnahmeGegenNachspielen_RealDB_KontenLaufenLeer ist derselbe Lauf unter
// der Bedingung, die den Lasttest von jedem bisherigen Test unterscheidet: die
// Konten sind zu KLEIN fuer die Ueberweisungen, die auf sie zukommen.
//
// Auf den Boxen war "guthaben" mit 14.444 der EINZIGE Ablehnungsgrund, der
// ueberhaupt auftrat -- die Wegwerfkonten des Lasttests liefen leer. Und genau
// dort koennen Annahme und Nachspielen auseinanderlaufen, ohne dass ein Block
// fehlt: der Ausgangskorb wird NICHT in der Reihenfolge gelesen, in der die
// Annahme die Ueberweisungen angewandt hat (ORDER BY wal_seq, id -- siehe
// ausgangskorbLesen). Solange kein Konto leerlaeuft, ist das folgenlos, weil
// Addition die Reihenfolge nicht kennt. Laeuft eines leer, ist es nicht mehr
// folgenlos: dieselbe Ueberweisung ist in der einen Reihenfolge bezahlbar und
// in der anderen nicht.
func TestAnnahmeGegenNachspielen_RealDB_KontenLaufenLeer(t *testing.T) {
	experimentLauf(t, 0.02, false) // die Groessenordnung der Lasttest-Konten
}

// TestAnnahmeGegenNachspielen_RealDB_WALAsymmetrie baut die letzte Asymmetrie
// der Boxen nach: der annehmende Knoten faehrt den WAL-Schnellpfad, der
// nachspielende nicht.
//
// Das ist der Zustand seit enable-wal-contabo2.yml: AEQUITAS_WAL_ENABLED=1
// nur auf C2 (ANALYSE_STATEROOT_DIVERGENZ.md, Abschnitt F). Der
// WAL-Schnellpfad hat eigene Arithmetik, eine eigene Reihenfolge im
// Ausgangskorb (wal_seq statt id -- und ORDER BY wal_seq, id sortiert jede
// Nicht-WAL-Zeile VOR jede WAL-Zeile, unabhaengig davon, wann sie entstand)
// und seine eigene, asynchrone Versoehnung mit Postgres. Genau diese drei
// Unterschiede konnte bisher kein Test messen.
func TestAnnahmeGegenNachspielen_RealDB_WALAsymmetrie(t *testing.T) {
	experimentLauf(t, 500.0, true)
}

// TestAnnahmeGegenNachspielen_RealDB_WALUndKontenLaufenLeer ist die
// Kombination beider Bedingungen des Lasttests: WAL auf der annehmenden Box
// UND Konten, die leerlaufen.
func TestAnnahmeGegenNachspielen_RealDB_WALUndKontenLaufenLeer(t *testing.T) {
	experimentLauf(t, 0.02, true)
}

func experimentLauf(t *testing.T, startGuthaben float64, mitWAL bool) {
	truncateDistTestTables(t) // auch das Opt-in-Tor
	dbB := os.Getenv("AEQUITAS_DB_B")
	if dbB == "" {
		t.Skip("braucht AEQUITAS_DB_B: eine zweite, entbehrliche Postgres-Datenbank fuer den nachspielenden Knoten")
	}

	const (
		konten = 24
		laeufe = 60 // laeufe * konten Ueberweisungen insgesamt
	)
	// Betraege, die in Mikro-AEQ nicht aufgehen. 0,0000005 ist der Fall, an
	// dem sich math.Round entscheiden muss.
	betraege := []float64{1.0 / 3.0, 0.1234565, 0.0000005, 0.7000001, 2.0 / 7.0, 0.0000015}

	adr := func(i int) string { return fmt.Sprintf("0xe0000000000000000000000000000000000%04x", i) }

	// ---- Knoten A: ANNAHME ----
	if mitWAL {
		t.Setenv("AEQUITAS_WAL_ENABLED", "1")
		t.Setenv("AEQUITAS_WAL_PATH", filepath.Join(t.TempDir(), "annahme.wal"))
	}
	csA := testKnoten(t, "unused-annahme-gegen-nachspielen-a.json")
	if mitWAL {
		if csA.wal == nil {
			t.Fatal("Knoten A sollte den WAL-Schnellpfad fahren, tut es aber nicht -- siehe die [WAL]-Zeilen oben")
		}
		t.Cleanup(csA.stopWALFlushWorkerForTest)
	}
	if !csA.useDB {
		t.Fatal("Knoten A hat keine Datenbank -- DATABASE_URL pruefen")
	}
	seed := func(cs *ChainState) {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		for i := 0; i < konten; i++ {
			cs.accounts.Delete(adr(i))
			acc := &AccountState{Address: adr(i), Balance: NewDecimal(startGuthaben), LastActivityAt: nowUnix()}
			if err := cs.saveAccountToDB(acc); err != nil {
				t.Fatalf("Konto %d anlegen: %v", i, err)
			}
		}
	}
	seed(csA)

	// Zaehlerstand vor dem Lauf: danach muss belegt sein, dass der Pfad, den
	// dieses Experiment messen soll, ueberhaupt Verkehr getragen hat. Ein
	// gruener Test, dessen Pfad nie betreten wurde, ist schlimmer als kein
	// Test -- er trainiert den Leser, gruen zu glauben.
	walVorher := txPhaseCount.Load()

	// Nebenlaeufig, damit der Buendler ueberhaupt buendelt: laeuft alles
	// seriell, nimmt jede Ueberweisung den Shard-Schnellpfad und der
	// Buendelpfad -- der eigentliche Verdaechtige -- wird nie betreten.
	var wg sync.WaitGroup
	var fehler sync.Map
	for k := 0; k < konten; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			for r := 0; r < laeufe; r++ {
				from := adr(k)
				to := adr((k + 1 + r%(konten-1)) % konten)
				if from == to {
					continue
				}
				betrag := betraege[(k+r)%len(betraege)]
				hash := fmt.Sprintf("0xexp-%d-%d", k, r)
				tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: betrag, TxHash: hash}
				if _, _, err := csA.TransferAtomic(from, to, betrag, tmpl); err != nil {
					fehler.Store(hash, err)
				}
			}
		}(k)
	}
	wg.Wait()
	anzFehler := 0
	fehler.Range(func(_, _ any) bool { anzFehler++; return true })
	if anzFehler > 0 {
		t.Logf("%d Ueberweisungen wurden bei der Annahme abgelehnt (erwartbar, wenn ein Konto leerlaeuft) -- "+
			"sie stehen dann auch in keinem Block und sind fuer den Vergleich unerheblich", anzFehler)
	}

	if mitWAL {
		walAngewandt := txPhaseCount.Load() - walVorher
		if walAngewandt == 0 {
			t.Fatal("der WAL-Schnellpfad hat keine einzige Ueberweisung angewandt -- " +
				"dieser Lauf misst dann nicht, was er zu messen vorgibt")
		}
		t.Logf("WAL-Schnellpfad hat %d Ueberweisungen angewandt", walAngewandt)
	}

	// Der WAL-Pfad versoehnt sich asynchron mit Postgres: die
	// Ausgangskorb-Zeile entsteht erst, wenn der Flush-Arbeiter sie
	// geschrieben hat. Abwarten, sonst liest der Block einen halbleeren Korb.
	if mitWAL {
		for i := 0; i < 600; i++ {
			if csA.WALFlushQueueDepth() == 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if d := csA.WALFlushQueueDepth(); d != 0 {
			t.Fatalf("WAL-Flush kam nicht nach: %d Posten stehen noch aus", d)
		}
		time.Sleep(500 * time.Millisecond) // die letzte Runde noch fertig schreiben lassen
	}

	// ---- Der Block, so wie ProduceBlock ihn bildet ----
	txs := ausgangskorbLesen(t, os.Getenv("DATABASE_URL"))
	if len(txs) == 0 {
		t.Fatal("der Ausgangskorb ist leer -- dann hat die Annahme nichts angenommen")
	}
	t.Logf("Annahme: %d Ueberweisungen im Ausgangskorb", len(txs))

	// ---- Knoten B: NACHSPIELEN ----
	origURL := os.Getenv("DATABASE_URL")
	t.Setenv("DATABASE_URL", dbB)
	schemaAnlegenUndLeeren(t, dbB)
	csB := testKnoten(t, "unused-annahme-gegen-nachspielen-b.json")
	if !csB.useDB {
		t.Fatal("Knoten B hat keine Datenbank -- AEQUITAS_DB_B pruefen")
	}
	seed(csB)

	dagB := newOrphanTestDAG()
	dagB.state = csB
	dagB.bootHeight = 0
	dagB.replayedBlocks = make(map[string]bool)
	dagB.replayFailures = make(map[string]replayFailureState)
	dagB.stateRootMismatches = map[string]int{}
	dagB.stateRootMismatchLastAt = map[string]int64{}

	// Der Zaehler ist prozessweit; vor dem Nachspielen auf einen bekannten
	// Stand bringen, damit die Zahl danach diesem Lauf gehoert.
	vorher := uebersprungeneUeberweisungen.Load()

	const proBlock = 64
	hoehe := int64(0)
	for i := 0; i < len(txs); i += proBlock {
		j := i + proBlock
		if j > len(txs) {
			j = len(txs)
		}
		hoehe++
		b := &Block{
			Height:       hoehe,
			Hash:         fmt.Sprintf("0xexp-block-%d", hoehe),
			Timestamp:    nowUnix(),
			Transactions: txs[i:j],
		}
		if ok := dagB.replayTransactions(b, true); !ok {
			t.Fatalf("Nachspielen wies Block %d ab", hoehe)
		}
	}
	_ = origURL
	uebersprungen := uebersprungeneUeberweisungen.Load() - vorher
	if uebersprungen > 0 {
		t.Logf("Nachspielen hat %d Ueberweisungen uebersprungen (nicht bezahlbar in Blockreihenfolge) -- "+
			"genau der Zaehler, der auf den Boxen als uebersprungene_ueberweisungen sichtbar ist", uebersprungen)
	}

	// ---- Der Vergleich, mikro-genau ----
	var abweichend int
	for i := 0; i < konten; i++ {
		a, b := abzugVon(csA, adr(i)), abzugVon(csB, adr(i))
		if a.guthabenMikro != b.guthabenMikro {
			abweichend++
			t.Errorf("Konto %d (%s): Annahme %d Mikro, Nachspielen %d Mikro, Differenz %d Mikro",
				i, adr(i), a.guthabenMikro, b.guthabenMikro, b.guthabenMikro-a.guthabenMikro)
		}
	}
	if abweichend > 0 {
		t.Fatalf("%d von %d Konten weichen ab -- der Unterschied im Rechenweg zwischen Annahme und "+
			"Nachspielen ist reproduziert, mit %d Ueberweisungen und ohne eine einzige Box",
			abweichend, konten, len(txs))
	}
	t.Logf("%d Konten, %d Ueberweisungen, Startguthaben %.6f, WAL=%v: Annahme und Nachspielen stimmen auf das Mikro-AEQ ueberein",
		konten, len(txs), startGuthaben, mitWAL)
}

// ausgangskorbLesen liest die noch nicht eingebauten Ausgangskorb-Zeilen in
// GENAU der Reihenfolge, in der LoadPendingTxs sie fuer einen Block liest
// (evm_storage.go): ORDER BY wal_seq, id. Die Reihenfolge ist Teil des
// Experiments -- sie ist nicht dieselbe, in der die Annahme sie angewandt hat.
func ausgangskorbLesen(t *testing.T, url string) []Transaction {
	t.Helper()
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("Ausgangskorb oeffnen: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 ORDER BY wal_seq, id`)
	if err != nil {
		t.Fatalf("Ausgangskorb lesen: %v", err)
	}
	defer rows.Close()
	var out []Transaction
	for rows.Next() {
		var js string
		if err := rows.Scan(&js); err != nil {
			t.Fatalf("Ausgangskorb-Zeile: %v", err)
		}
		var tx Transaction
		if err := json.Unmarshal([]byte(js), &tx); err != nil {
			t.Fatalf("Ausgangskorb-Zeile ist kein JSON: %v", err)
		}
		out = append(out, tx)
	}
	return out
}

func schemaAnlegenUndLeeren(t *testing.T, url string) {
	t.Helper()
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("Datenbank B oeffnen: %v", err)
	}
	defer db.Close()
	// NICHT ensureRealDBSchema: das haengt an einem sync.Once, der fuer
	// Datenbank A schon gelaufen ist, und liefe hier als stiller No-op --
	// Knoten B bekaeme nie ein Schema. Direkt anlegen.
	(&ChainState{db: db, useDB: true}).initDB()
	if _, err := db.Exec(`TRUNCATE chain_accounts, chain_config, nullifiers, liquidity_pool, escrow_accounts, registered_nodes, pending_txs CASCADE`); err != nil {
		t.Fatalf("Datenbank B leeren: %v", err)
	}
}
