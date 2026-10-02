package keeper

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
)

// Den Flush der Kontostaende auf mehrere Datenbankverbindungen aufteilen.
//
// GEMESSEN AM 01.10.2026 (Pruefstand Lauf 7, Speicherkorb an): ~12.500
// Ketten-TPS, die Annahme lehnte 927.000-mal wegen WAL-Druck ab -- der Flush
// der Kontostaende nach Postgres schaffte ~13.000 Eintraege/s. Je Flush:
// ~880 Eintraege, ~400 Adressen, 57 ms Haltezeit, aber 250 ms insgesamt --
// ~190 ms wartete er auf die Sperren. Vier Flush-Arbeiter nehmen
// aufeinanderfolgende Stuecke der Warteschlange; bei heissen Konten
// ueberschneiden sich ihre Adressen fast immer, und weil jeder seine Adressen
// ueber die GANZE Postgres-Transaktion haelt (die Invariante gegen
// Postgres-Deadlocks, flushWALBatch Schicht 2), laufen sie praktisch
// nacheinander. Postgres arbeitete dabei auf einem von acht Kernen.
//
// HIER: ein Buendel wird nach Adresse (FNV-Hash) in `teile` DISJUNKTE
// Gruppen geteilt; jede Gruppe in eigener Transaktion auf eigener Verbindung,
// gleichzeitig. Jede Gruppe sperrt NUR ihre Adressen und haelt sie bis zu
// IHREM Commit -- dieselbe Invariante wie bisher, je Zeile: wer eine Zeile
// schreibt, haelt ihre Shard-Sperre, bis seine Transaktion zu ist. Zwei
// Gruppen beruehren nie dieselbe Zeile.
//
// NUR OHNE OUTBOX. Mit Zeilen fuer pending_txs muesste die Outbox in genau
// einer Teiltransaktion stehen; scheitert eine andere, wird das Buendel
// wiederholt und die Outbox-Zeilen stuenden doppelt. Deshalb nur, wenn jede
// Ueberweisung im Speicherkorb steht (ohneOutbox) -- dann ist ein
// wiederholtes Buendel harmlos: der UPSERT schreibt nur, wenn die wal_seq
// steigt.
//
// Schalter: AEQUITAS_WAL_FLUSH_TEILE (Vorgabe 1 = wie bisher, hoechstens 16).

const walFlushTeileEnv = "AEQUITAS_WAL_FLUSH_TEILE"

// walFlushTeileLaeufe: wie oft ein Buendel aufgeteilt geflusht wurde.
var walFlushTeileLaeufe atomic.Int64

// Unter so vielen Adressen lohnt das Aufteilen nicht (eine Transaktion je
// Teil kostet selbst ~10 ms Commit).
const walFlushTeileAbAdressen = 64

var walFlushTeileWert = walFlushTeileAusUmgebung()

func walFlushTeileAusUmgebung() int {
	n := envPositiveInt(walFlushTeileEnv)
	if n <= 0 {
		return 1
	}
	if n > 16 {
		return 16
	}
	return n
}

// walFlushTeileFuer: in wie viele Teile dieses Buendel geht (1 = nicht teilen).
func walFlushTeileFuer(batch []walFlushItem, adressen int) int {
	teile := walFlushTeileWert
	if teile <= 1 || adressen < walFlushTeileAbAdressen {
		return 1
	}
	for _, it := range batch {
		if !it.ohneOutbox {
			return 1
		}
	}
	if max := adressen / (walFlushTeileAbAdressen / 2); teile > max {
		teile = max
	}
	if teile < 2 {
		return 1
	}
	return teile
}

// adressenAufteilen: disjunkte, je sortierte Gruppen; dieselbe Adresse landet
// immer in derselben Gruppe.
func adressenAufteilen(adressen []string, teile int) [][]string {
	gruppen := make([][]string, teile)
	for _, a := range adressen {
		h := fnv.New32a()
		h.Write([]byte(a))
		i := int(h.Sum32() % uint32(teile))
		gruppen[i] = append(gruppen[i], a)
	}
	for i := range gruppen {
		sort.Strings(gruppen[i])
	}
	return gruppen
}

// flushWALKontenAufgeteilt: Aufrufer haelt cs.mu.RLock (wie flushWALBatch).
// Scheitert ein Teil, scheitert das Buendel und wird wiederholt; schon
// geschriebene Teile sind dann harmlos (wal_seq-Waechter im UPSERT).
func (cs *ChainState) flushWALKontenAufgeteilt(batch []walFlushItem, adressen []string, teile int) error {
	start := time.Now()
	gruppen := adressenAufteilen(adressen, teile)
	fehler := make([]error, len(gruppen))
	var wg sync.WaitGroup
	for i, g := range gruppen {
		if len(g) == 0 {
			continue
		}
		wg.Add(1)
		go func(i int, g []string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					fehler[i] = fmt.Errorf("Flush-Teil %d: panic: %v", i, r)
				}
			}()
			fehler[i] = cs.flushWALKontenTeil(g)
		}(i, g)
	}
	wg.Wait()
	noteWALFlush(len(batch), len(adressen), 0, time.Since(start))
	walFlushTeileLaeufe.Add(1)
	for _, err := range fehler {
		if err != nil {
			return err
		}
	}
	return nil
}

// flushWALKontenTeil schreibt die Kontostaende EINER Adressgruppe in eigener
// Transaktion -- Sperren, Momentaufnahme, Warten auf Haltbarkeit, UPSERT,
// Buchkonten, Commit; die Sperren bis nach dem Commit (Invariante wie
// flushWALBatch Schicht 2).
func (cs *ChainState) flushWALKontenTeil(adressen []string) error {
	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("could not begin WAL flush part: %w", err)
	}
	ctx := withTx(context.Background(), tx)
	unlock := cs.accounts.LockAddrs(adressen...)
	defer unlock()

	addrs := make([]string, 0, len(adressen))
	salden := make([]float64, 0, len(adressen))
	seqs := make([]int64, 0, len(adressen))
	nonces := make([]int64, 0, len(adressen))
	aktiv := make([]int64, 0, len(adressen))
	var hoechste uint64
	for _, a := range adressen {
		acc, ok := cs.accounts.GetLocked(a)
		if !ok {
			tx.Rollback()
			return fmt.Errorf("flushWALKontenTeil: address %s vanished from cs.accounts between apply and flush -- this should never happen", a)
		}
		addrs = append(addrs, a)
		salden = append(salden, acc.Balance.Float())
		seqs = append(seqs, int64(acc.WALSeq))
		nonces = append(nonces, acc.NaechsteNonce)
		aktiv = append(aktiv, acc.LastActivityAt)
		if acc.WALSeq > hoechste {
			hoechste = acc.WALSeq
		}
	}
	// Nichts nach Postgres, was im WAL noch nicht haltbar ist (wie flushWALBatch).
	if cs.wal != nil && hoechste > 0 && !cs.wal.WaitDurable(hoechste, 2*time.Second) {
		walFlushOhneHaltbarkeit.Add(1)
		fmt.Printf("[WAL] ⚠ Flush-Teil schreibt Salden bis Seq %d nach Postgres, obwohl das WAL sie nicht als haltbar bestaetigt hat (Sync defekt oder zu langsam)\n", hoechste)
	}
	if _, err := cs.dbExecCtx(ctx).Exec(walKontenUpsertSQL,
		pq.Array(addrs), pq.Array(salden), pq.Array(seqs), pq.Array(nonces), pq.Array(aktiv)); err != nil {
		tx.Rollback()
		return fmt.Errorf("could not upsert %d account(s) during WAL flush part: %w", len(addrs), err)
	}
	if wirtschaftAktiv(nowUnix()) {
		if err := cs.speichereBuchCtx(ctx, adressen...); err != nil {
			tx.Rollback()
			return fmt.Errorf("could not save books during WAL flush part: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("WAL flush part commit failed: %w", err)
	}
	return nil
}
