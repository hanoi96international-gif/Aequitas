package keeper

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// chain_tx_block_index begrenzen -- die Tabelle hatte keine Obergrenze.
//
// Sie beantwortet "in welchem Block lag diese Transaktion" fuer
// eth_getTransactionByHash und eth_getTransactionReceipt. Geschrieben wird je
// Transaktion eine Zeile; entfernt wurde nie eine. Fuer chain_tx_batches gibt
// es seit dem 28.07.2026 ein Byte-Budget (tx_batch_prune.go), fuer diesen
// Index nicht -- dessen eigener Kommentar nennt ihn sogar ausdruecklich als
// unabhaengig gefuehrt.
//
// WAS DARAUS WURDE. Am 09.09.2026 mass das Launch-Gate-Audit 13 GB fuer diese
// eine Tabelle. Am 11.09.2026 standen BEIDE Boxen bei 0 MB freiem Platz,
// Postgres war auf beiden gestorben und die Produktion seit Stunden
// stillgelegt. Die Lasttests dieser Woche haben Millionen Zeilen erzeugt --
// jede einzelne bleibt bis heute liegen.
//
// WARUM DAS KUERZEN SICHER IST. Der Index ist kein Konsens. Beide Aufrufer
// behandeln einen Fehlschlag schon heute als nicht toedlich und sagen auch
// warum: "the block itself is valid and committed, and a missing index entry
// degrades to the pre-existing fallback behaviour rather than rejecting
// anything". Die Transaktionen selbst liegen in chain_blocks.transactions und
// bleiben unangetastet -- verloren geht nur die schnelle Antwort auf eine
// Abfrage nach einem sehr alten Transaktionshash, und die faellt auf denselben
// Ersatzweg zurueck, den ein fehlender Eintrag ohnehin nimmt.
//
// WARUM EIN BYTE-BUDGET. Dieselbe Begruendung wie bei chain_tx_batches: eine
// Zeilenzahl sagt nichts ueber den belegten Platz, und Platz ist das, was
// ausgeht. Gemessen wird ueber pg_column_size der lebenden Zeilen, nicht ueber
// die physische Tabellengroesse -- ein DELETE gibt die Seiten erst mit VACUUM
// zurueck, eine Schleife auf der physischen Groesse wuerde also loeschen, bis
// die Tabelle leer ist.
//
// Die aeltesten zuerst: wer nach einem Transaktionshash fragt, fragt fast
// immer nach einem juengeren.
const txIndexMaxBytesVorgabe int64 = 2 << 30 // 2 GiB

const (
	txIndexPruneIntervall = 10 * time.Minute
	txIndexLoeschStueck   = 50000
	txIndexBudgetEnv      = "AEQUITAS_TX_INDEX_MAX_BYTES"
)

var (
	txIndexPruneOnce sync.Once
	txIndexGeloescht atomic.Int64
	txIndexLaeufe    atomic.Int64
	txIndexLetzteMB  atomic.Int64
	txIndexZeilen    atomic.Int64
)

// txIndexBytesJeZeile ist die gemessene Groesse einer Zeile einschliesslich
// ihres Indexanteils: 15 GB auf 51.155.972 Zeilen am 11.09.2026 auf C1.
const txIndexBytesJeZeile int64 = 293

func txIndexBudget() int64 {
	if roh := os.Getenv(txIndexBudgetEnv); roh != "" {
		if n, err := strconv.ParseInt(roh, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return txIndexMaxBytesVorgabe
}

// starteTxIndexPrune haelt chain_tx_block_index in seinem Budget.
func (cs *ChainState) starteTxIndexPrune() {
	if cs.db == nil {
		return
	}
	txIndexPruneOnce.Do(func() {
		fmt.Printf("[TX-INDEX] Begrenzung aktiv: chain_tx_block_index wird auf %d MB gehalten (aelteste zuerst)\n",
			txIndexBudget()>>20)
		SafeGoroutine("txIndexPrune", func() {
			t := time.NewTicker(txIndexPruneIntervall)
			defer t.Stop()
			cs.pruneTxIndex()
			for range t.C {
				cs.pruneTxIndex()
			}
		})
	})
}

func (cs *ChainState) pruneTxIndex() {
	if cs.db == nil {
		return
	}
	budget := txIndexBudget()
	txIndexLaeufe.Add(1)
	// Gedeckelt, damit ein entgleister Zustand hier nicht endlos dreht; was
	// uebrig bleibt, holt der naechste Durchgang.
	for i := 0; i < 200; i++ {
		// ZEILEN ZAEHLEN, NICHT BYTES SUMMIEREN.
		//
		// Der erste Entwurf summierte pg_column_size ueber alle Zeilen -- die
		// Abfrage, die chain_tx_batches benutzt. Dort sind es ein paar tausend
		// Zeilen; hier waren es am 11.09.2026 51.155.972. Der Scan lief damit
		// in das statement_timeout von fuenf Sekunden, der Fehler wurde still
		// verschluckt, und die Begrenzung meldete 0 MB bei 15 GB Tabelle. Ein
		// Zaehler, der bei einem Fehler eine Null meldet, ist schlimmer als
		// keiner: er sagt "alles in Ordnung".
		//
		// Zeilen zaehlen ist hier auch sachlich richtiger. Der Kommentar in
		// tx_batch_prune.go verwirft eine Zeilengrenze, weil DORT die
		// Zeilengroesse um drei Groessenordnungen schwankt (ein Rumpf kann
		// 620 KB haben). Diese Tabelle hat feste Spalten -- zwei Hashes, eine
		// Hoehe, ein Index -- und damit eine nahezu konstante Zeilengroesse:
		// gemessen 15 GB auf 51.155.972 Zeilen, also rund 293 Byte
		// einschliesslich Indexanteil. Aus Bytes wird so eine verlaessliche
		// Zeilenzahl.
		// reltuples, NICHT count(*). Auch ein count(*) ist in Postgres ein
		// vollstaendiger Tabellenscan und liefe bei 51 Millionen Zeilen in
		// dasselbe Zeitlimit wie die Byte-Summe davor. reltuples ist die
		// Schaetzung aus den Tabellenstatistiken und antwortet sofort; sie
		// weicht zwischen zwei ANALYZE-Laeufen ab, was fuer eine Obergrenze
		// mit Gigabyte-Budget bedeutungslos ist. Ein DELETE loest ohnehin ein
		// Autovacuum aus, das die Zahl nachfuehrt.
		var zeilen int64
		if err := cs.db.QueryRow(
			`SELECT GREATEST(reltuples::bigint, 0) FROM pg_class WHERE relname = 'chain_tx_block_index'`,
		).Scan(&zeilen); err != nil {
			fmt.Printf("[TX-INDEX] ⚠ Groesse nicht messbar, Begrenzung greift diesen Durchgang nicht: %v\n", err)
			return
		}
		lebend := zeilen * txIndexBytesJeZeile
		txIndexLetzteMB.Store(lebend >> 20)
		txIndexZeilen.Store(zeilen)
		if lebend <= budget {
			return
		}
		res, err := cs.db.Exec(
			`DELETE FROM chain_tx_block_index WHERE ctid IN (
			   SELECT ctid FROM chain_tx_block_index ORDER BY block_height ASC LIMIT $1
			 )`, txIndexLoeschStueck)
		if err != nil {
			fmt.Printf("[TX-INDEX] ⚠ konnte nicht kuerzen (bleibt uebergross bis zum naechsten Durchgang): %v\n", err)
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return
		}
		txIndexGeloescht.Add(n)
		fmt.Printf("[TX-INDEX] %d Zeilen entfernt (%d MB lebend, Budget %d MB)\n", n, lebend>>20, budget>>20)
	}
}

// TxIndexPruneStand meldet, ob die Begrenzung greift.
func TxIndexPruneStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "chain_tx_block_index beantwortet nur eth_getTransactionByHash und hat mit " +
			"Konsens nichts zu tun; beide Aufrufer behandeln einen fehlenden Eintrag schon heute " +
			"als unkritisch. Ohne Begrenzung wuchs sie unbegrenzt -- am 09.09.2026 auf 13 GB " +
			"gemessen, am 11.09. standen beide Boxen mit voller Platte.",
		"budget_mb":       txIndexBudget() >> 20,
		"lebend_mb":       txIndexLetzteMB.Load(),
		"zeilen":          txIndexZeilen.Load(),
		"zeilen_entfernt": txIndexGeloescht.Load(),
		"laeufe":          txIndexLaeufe.Load(),
	}
}
