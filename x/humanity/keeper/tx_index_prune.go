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
)

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
		var lebend int64
		if err := cs.db.QueryRow(
			`SELECT COALESCE(sum(pg_column_size(tx_hash) + pg_column_size(block_hash) + 16),0) FROM chain_tx_block_index`,
		).Scan(&lebend); err != nil {
			// Fehlt die Tabelle, gibt es nichts zu begrenzen.
			return
		}
		txIndexLetzteMB.Store(lebend >> 20)
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
		"zeilen_entfernt": txIndexGeloescht.Load(),
		"laeufe":          txIndexLaeufe.Load(),
	}
}
