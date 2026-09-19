package keeper

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
)

// ZWEI PRODUZENTEN -- HIER REISST DIE STAUB-DIVERGENZ.
//
// ANGRIFFSTEST: dieser Lauf zeigt einen Fehler, der noch offen ist.
//
// Er laeuft NUR, wenn man ihn ausdruecklich anfordert
// (AEQUITAS_REPRODUZIERE_DIVERGENZ=1, zusaetzlich zu den zwei Datenbanken),
// und ueberspringt sonst mit genau diesem Hinweis. Ein Test, der dauerhaft
// rot steht, bringt dem Leser bei, Rot zu ignorieren -- dieselbe Begruendung,
// mit der in diesem Durchgang tx_block_index_async_test.go geradegezogen
// wurde. Es waere schlecht, hier die Ausnahme zu machen.
//
// Angefordert FAELLT ER DURCH, und das ist der Befund, nicht ein kaputter
// Test. Solange die Wurzel offen ist, gehoert er genau so.
//
// # WAS ER REPRODUZIERT
//
// Genau den Fingerabdruck von den Boxen, bis ins Detail:
//
//	32 Konten, 25 Runden, Startguthaben 0,02 AEQ (Lasttest-Groesse)
//	-> 4 von 32 Konten weichen ab
//	-> die Abweichungen sind ganze Vielfache eines Ueberweisungsbetrags
//	   (7.100 Mikro = genau eine Ueberweisung zu 0,0071)
//	-> DIE SUMMEN BEIDER KNOTEN SIND GLEICH (640.000 = 640.000 Mikro)
//	-> kein fehlender Block, kein Rollback, kein Flush-Fehler
//
// Die dritte Zeile ist die, die es entscheidet. Die Checkliste hielt am
// 15.09. fest: "alle 18 Menschen identisch, Summen bis aufs Mikro-AEQ
// gleich" -- und trotzdem einzelne Konten abweichend. Genau das steht hier.
//
// # DER MECHANISMUS
//
// Beide Boxen produzieren Bloecke. Jeder Knoten wendet seine eigenen
// Ueberweisungen bei der ANNAHME an -- also bevor irgendein Block ihre
// Reihenfolge festlegt -- und die des anderen beim NACHSPIELEN. Damit
// durchlaeuft jeder Knoten eine andere Zustandsfolge.
//
// Solange kein Konto leerlaeuft, ist das folgenlos: Addition kennt keine
// Reihenfolge, und beide landen am selben Ende.
//
// Laeuft ein Konto leer, ist es nicht mehr folgenlos. Der annehmende Knoten
// hat die Ueberweisung gegen SEINE Sicht geprueft und angewandt. Der
// nachspielende prueft sie ein ZWEITES Mal, gegen seine eigene, anders
// zustande gekommene Sicht -- und wenn sie dort nicht bezahlbar ist,
// ueberspringt er sie (ErrZustandLehntAb) und laeuft weiter. Ab diesem
// Augenblick sind sich die beiden ueber dieses Konto uneins, dauerhaft.
//
// Die Zahl steht in der Ausgabe je Runde, und sie ist NICHT symmetrisch:
// "A uebersprang 16, B uebersprang 32". Genau diese Differenz ist die
// Divergenz.
//
// # WARUM AUF DEN BOXEN TROTZDEM "0 UEBERSPRUNGEN" STAND
//
// uebersprungeneUeberweisungen ist ein atomic.Int64 im Prozess
// (zustand_ablehnung.go) -- er faengt nach jedem Neustart und jedem Resync
// wieder bei null an. Die Null vom 15.09. belegt also nicht, dass nie
// uebersprungen wurde, sondern nur, dass seit dem letzten Start nichts mehr
// uebersprungen wurde. Beim naechsten Lastlauf gehoert er auf BEIDEN Boxen
// vorher und nachher abgelesen, ohne Neustart dazwischen
// (/api/health/combined -> zustands_ablehnung). /api/wache faerbt bei > 0
// bereits rot (api_wache.go) -- der Alarm ist da, er wird nur von jedem
// Neustart geloescht.
//
// # WAS DAS FUER EINEN FIX HEISST
//
// Das ist kein Rechenfehler, den man an einer Stelle geradezieht. Die
// Ursache ist, dass eine Ueberweisung ZWEIMAL gegen ZWEI VERSCHIEDENE
// Zustaende auf Bezahlbarkeit geprueft wird: einmal bei der Annahme, gegen
// den Zustand des annehmenden Knotens, und einmal beim Nachspielen, gegen
// den des nachspielenden. Solange ein Knoten seine eigenen Ueberweisungen
// anwendet, BEVOR ein Block ihre Reihenfolge festlegt, koennen die beiden
// Pruefungen verschieden ausgehen.
//
// Sauber zu schliessen ist das nur an der Wurzel: Zustand aendert sich
// ausschliesslich beim Anwenden eines Blocks, die Annahme reiht nur ein.
// Das ist die uebliche Bauform einer Kette und ein grosser Eingriff -- keine
// Sache fuer eine Nacht vor einem Start, und nichts, was hier nebenbei
// mitgeliefert wird. Die Zwischenloesung, die heute schon greift: die Wache
// ist rot, sobald ueberspringen ueberhaupt vorkommt, und der Resync stellt
// beide Boxen wieder gleich.
//
// Was NICHT hilft und ausdruecklich nicht getan werden sollte: das
// Ueberspringen wieder in ein hardFailure zurueckdrehen. Das war der Zustand
// vor dem 05.09., und er kostete sechs Minuten Stillstand an einer Wand, die
// sich nie von selbst aufloest (zustand_ablehnung.go). Der laute Fehler war
// nicht besser als der leise -- er war nur lauter.
func TestZweiProduzenten_RealDB_KontenLaufenLeer(t *testing.T) {
	zweiProduzentenLauf(t, 0.02)
}

// TestZweiProduzenten_RealDB_VolleKonten ist die Gegenprobe: derselbe Aufbau,
// aber Konten, die nie leerlaufen. Weicht hier etwas ab, liegt es NICHT an
// der Reihenfolge, sondern an der Arithmetik -- die Unterscheidung ist der
// ganze Zweck dieses Paares.
func TestZweiProduzenten_RealDB_VolleKonten(t *testing.T) {
	zweiProduzentenLauf(t, 500.0)
}

func zweiProduzentenLauf(t *testing.T, startGuthaben float64) {
	truncateDistTestTables(t) // auch das Opt-in-Tor
	if os.Getenv("AEQUITAS_REPRODUZIERE_DIVERGENZ") != "1" {
		t.Skip("zeigt einen offenen Fehler und faellt deshalb durch -- " +
			"mit AEQUITAS_REPRODUZIERE_DIVERGENZ=1 anfordern (siehe Dateikopf)")
	}
	urlA := os.Getenv("DATABASE_URL")
	urlB := os.Getenv("AEQUITAS_DB_B")
	if urlB == "" {
		t.Skip("braucht AEQUITAS_DB_B: eine zweite, entbehrliche Postgres-Datenbank fuer den zweiten Produzenten")
	}

	const (
		konten  = 32
		runden  = 25
		jeRunde = 8 // Ueberweisungen je Konto und Runde, je Knoten
	)
	betraege := []float64{1.0 / 3.0, 0.1234565, 0.0000005, 0.0000015, 0.0071, 0.25}
	adr := func(i int) string { return fmt.Sprintf("0xd0000000000000000000000000000000000%04x", i) }

	csA := NewChainState("unused-zwei-produzenten-a.json")
	if !csA.useDB {
		t.Fatal("Knoten A hat keine Datenbank -- DATABASE_URL pruefen")
	}
	t.Setenv("DATABASE_URL", urlB)
	schemaAnlegenUndLeeren(t, urlB)
	csB := NewChainState("unused-zwei-produzenten-b.json")
	if !csB.useDB {
		t.Fatal("Knoten B hat keine Datenbank -- AEQUITAS_DB_B pruefen")
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
	seed(csB)

	dagFuer := func(cs *ChainState) *BlockDAG {
		d := newOrphanTestDAG()
		d.state = cs
		d.bootHeight = 0
		d.replayedBlocks = make(map[string]bool)
		d.replayFailures = make(map[string]replayFailureState)
		d.stateRootMismatches = map[string]int{}
		d.stateRootMismatchLastAt = map[string]int64{}
		return d
	}
	dagA, dagB := dagFuer(csA), dagFuer(csB)

	vorher := uebersprungeneUeberweisungen.Load()
	// atomic: beide Produzenten zaehlen hier gleichzeitig hoch. Als blosses
	// abgelehnt++ bricht -race den Lauf ab, bevor er zu dem Vergleich kommt,
	// wegen dem es ihn gibt.
	var abgelehnt atomic.Int64
	var hoehe int64

	for runde := 0; runde < runden; runde++ {
		// Beide Knoten nehmen gleichzeitig an -- auf denselben Konten. Die
		// Sender sind bewusst geteilt: nur dann kann ein Konto auf dem einen
		// Knoten schon leer sein, waehrend der andere es noch fuer gefuellt
		// haelt.
		var wg sync.WaitGroup
		// Die beiden Lasten muessen VERSCHIEDEN sein, nicht gespiegelt. Ein
		// erster Anlauf liess Knoten B dieselbe Arithmetik mit verschobenem
		// Index fahren -- dann uebersprangen beide Knoten in jeder Runde
		// exakt gleich viele Ueberweisungen (18/18, 2/2, ...), die Skips
		// hoben sich auf und der Test war gruen, ohne etwas zu zeigen. Zwei
		// Boxen im Lasttest sind sich nicht symmetrisch.
		annehmen := func(cs *ChainState, knoten int) {
			defer wg.Done()
			anzahl := jeRunde
			if knoten == 1 {
				anzahl = jeRunde - 3 // B schickt weniger
			}
			for k := 0; k < konten; k++ {
				for r := 0; r < anzahl; r++ {
					// DIESELBEN Sender auf beiden Knoten -- nur andere
					// Empfaenger, andere Betraege, andere Zahl. Die Sender
					// muessen sich decken, sonst laeuft keines der Konten auf
					// dem einen Knoten leer, waehrend der andere es noch fuer
					// gefuellt haelt; die Empfaenger duerfen sich nicht
					// decken, sonst sind die beiden Lasten gespiegelt und die
					// Skips heben sich gegenseitig auf.
					var from, to string
					if knoten == 0 {
						from, to = adr(k), adr((k+1+r)%konten)
					} else {
						from, to = adr(k), adr((k+5+2*r)%konten)
					}
					if from == to {
						continue
					}
					betrag := betraege[(k*3+r*2+runde+knoten*4)%len(betraege)]
					hash := fmt.Sprintf("0xzp-%d-%d-%d-%d", knoten, runde, k, r)
					tmpl := Transaction{Type: "transfer", Wallet: from, To: to, Amount: betrag, TxHash: hash}
					if _, _, err := cs.TransferAtomic(from, to, betrag, tmpl); err != nil {
						abgelehnt.Add(1)
					}
				}
			}
		}
		wg.Add(2)
		go annehmen(csA, 0)
		go annehmen(csB, 1)
		wg.Wait()

		// Jeder bildet seinen Block aus seinem Ausgangskorb ...
		txsA := korbLeeren(t, urlA)
		txsB := korbLeeren(t, urlB)

		// ... und spielt den des ANDEREN nach.
		spielen := func(dag *BlockDAG, txs []Transaction, wer string) {
			if len(txs) == 0 {
				return
			}
			hoehe++
			b := &Block{
				Height:       hoehe,
				Hash:         fmt.Sprintf("0xzp-block-%s-%d", wer, hoehe),
				Timestamp:    nowUnix(),
				Transactions: txs,
			}
			if ok := dag.replayTransactions(b, true); !ok {
				t.Fatalf("Knoten %s wies Block %d ab", wer, hoehe)
			}
		}
		vorA := uebersprungeneUeberweisungen.Load()
		spielen(dagA, txsB, "a") // A spielt B's Block nach
		nachA := uebersprungeneUeberweisungen.Load()
		spielen(dagB, txsA, "b") // B spielt A's Block nach
		nachB := uebersprungeneUeberweisungen.Load()

		var diff int
		for i := 0; i < konten; i++ {
			if abzugVon(csA, adr(i)).guthabenMikro != abzugVon(csB, adr(i)).guthabenMikro {
				diff++
			}
		}
		t.Logf("Runde %2d: A uebersprang %d, B uebersprang %d, danach %d Konten verschieden",
			runde, nachA-vorA, nachB-nachA, diff)
	}

	uebersprungen := uebersprungeneUeberweisungen.Load() - vorher
	t.Logf("Startguthaben %.6f: %d Ueberweisungen bei der Annahme abgelehnt, %d beim Nachspielen uebersprungen",
		startGuthaben, abgelehnt.Load(), uebersprungen)

	var abweichend int
	var summeA, summeB int64
	for i := 0; i < konten; i++ {
		a, b := abzugVon(csA, adr(i)), abzugVon(csB, adr(i))
		summeA += a.guthabenMikro
		summeB += b.guthabenMikro
		if a.guthabenMikro != b.guthabenMikro {
			abweichend++
			if abweichend <= 10 {
				t.Errorf("Konto %d (%s): Knoten A %d Mikro, Knoten B %d Mikro, Differenz %d Mikro",
					i, adr(i), a.guthabenMikro, b.guthabenMikro, b.guthabenMikro-a.guthabenMikro)
			}
		}
	}
	if abweichend > 0 {
		t.Fatalf("%d von %d Konten weichen ab (Summen: A %d Mikro, B %d Mikro, Differenz %d).\n"+
			"Zwei Produzenten, Konten die leerlaufen -- und die Divergenz entsteht ohne fehlenden Block,\n"+
			"ohne Rollback und ohne Flush-Fehler, genau wie auf den Boxen. %d uebersprungene Ueberweisungen.",
			abweichend, konten, summeA, summeB, summeB-summeA, uebersprungen)
	}
	t.Logf("%d Konten: beide Knoten stimmen auf das Mikro-AEQ ueberein (Summe %d Mikro)", konten, summeA)
}

// korbLeeren liest den Ausgangskorb in Blockreihenfolge und raeumt ihn ab --
// die gelesenen Zeilen sind damit in einem Block und duerfen in keinen
// zweiten. Entspricht dem, was SaveBlockWithPendingTxsAtomic nach der
// Blockbildung mit ihnen tut.
func korbLeeren(t *testing.T, url string) []Transaction {
	t.Helper()
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("Ausgangskorb oeffnen: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, tx_json FROM pending_txs WHERE included_at = 0 ORDER BY wal_seq, id`)
	if err != nil {
		t.Fatalf("Ausgangskorb lesen: %v", err)
	}
	var out []Transaction
	var ids []int64
	for rows.Next() {
		var id int64
		var js string
		if err := rows.Scan(&id, &js); err != nil {
			rows.Close()
			t.Fatalf("Ausgangskorb-Zeile: %v", err)
		}
		var tx Transaction
		if err := json.Unmarshal([]byte(js), &tx); err != nil {
			rows.Close()
			t.Fatalf("Ausgangskorb-Zeile ist kein JSON: %v", err)
		}
		out = append(out, tx)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := db.Exec(`DELETE FROM pending_txs WHERE id = $1`, id); err != nil {
			t.Fatalf("Ausgangskorb abraeumen: %v", err)
		}
	}
	return out
}
