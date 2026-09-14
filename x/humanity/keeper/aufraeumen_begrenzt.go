package keeper

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"
)

// Begrenztes Aufraeumen: Stuecke mit Zeitbudget statt "loesche die Welt".
//
// WAS AM 14.09.2026 AUF C2 STAND. evm_tx_receipts hielt 13.206.916 Zeilen
// (4,2 GB), C1 95.775. Der Aufraeumer war EINE Anweisung -- DELETE ... WHERE
// tx_hash NOT IN (die 10.000 neuesten) --, und die kann bei 13 Millionen
// Zeilen nie in den 5 s der Verbindung fertig werden. Also scheiterte sie
// seit Tagen jede Minute, die Tabelle wuchs weiter, jede Einfuegung wurde
// zur Zufalls-I/O im 1,6-GB-Index, und das Schreiben der Quittungen selbst
// lief in dasselbe Limit: 406.268 Quittungen aus einem Lastlauf, 5.000er-
// Stueck fuer Stueck abgelehnt, alle 5 s aufs Neue. Postgres 322 % CPU,
// der Knoten 675 %, Annahme 4,8 s je Buendel statt 1,1 s auf C1.
// Selbstverstaerkend: sobald die Tabelle einmal groesser ist, als das Limit
// erlaubt, kann sie nur noch wachsen.
//
// Daneben lagen in pending_txs 1.120.327 Zeilen vom 12.09., alle als
// eingebaut markiert und nie geloescht (913 MB). ResetStaleIncludedPendingTxs
// haette sie bei jedem Start wieder als offen markiert -- 1,1 Millionen
// zwei Tage alte Ueberweisungen in neue Bloecke --, und nur der Timeout
// derselben Anweisung hat das verhindert.
//
// DIE REGEL: jede Aufraeum-Anweisung trifft hoechstens ein Stueck, das
// bequem unter das Limit passt; ein Lauf hat ein Zeitbudget und macht beim
// naechsten weiter; nichts, was gescheitert ist, waechst dadurch. Und die
// Indizes, die dafuer noetig sind, entstehen nebenlaeufig und ohne Limit --
// einmal, im Hintergrund, ohne die Tabelle zu sperren.

const (
	// aufraeumStueck: Zeilen je Anweisung. 10.000 Loeschungen ueber den
	// Primaerschluessel liegen im dreistelligen Millisekundenbereich.
	aufraeumStueck = 10000
	// aufraeumBudget: laenger laeuft ein Lauf nicht; der Rest wartet auf den
	// naechsten. Auf C2 sind 13 Millionen Zeilen damit in etwa 20 Minuten weg.
	aufraeumBudget = 20 * time.Second
	// pendingLeicheAlter: eine markierte, nie geloeschte Zeile, die aelter
	// ist, ist keine Absturzluecke mehr, sondern Muell. Ein Knoten, der
	// laenger als einen Tag weg war, braucht ohnehin einen Resync.
	pendingLeicheAlter = 24 * time.Hour
	// pendingLeichenTakt: wie oft der Sweep nach dem Start wiederkehrt.
	pendingLeichenTakt = time.Hour
)

var (
	receiptPruneGeloescht      atomic.Int64
	receiptPruneLaeufe         atomic.Int64
	receiptPruneFehler         atomic.Int64
	receiptPruneLetzterLauf    atomic.Int64
	receiptPruneBudgetErsch    atomic.Int64
	receiptIndexBereit         atomic.Bool
	pendingLeichenGeloescht    atomic.Int64
	pendingLeichenWiederOffen  atomic.Int64
	pendingLeichenLaeufe       atomic.Int64
	pendingLeichenFehler       atomic.Int64
	pendingLeichenLetzterLauf  atomic.Int64
	pendingLeichenBudgetErsch  atomic.Int64
	pendingLeichenIndexBereit  atomic.Bool
	pendingLeichenSweepLaeuft  atomic.Bool
	pendingLeichenTickerLaeuft atomic.Bool
)

// indexNebenlaeufigSicherstellen baut einen Index ohne Tabellensperre und
// ohne das 5-s-Limit der Verbindung -- auf einer eigenen Verbindung, damit
// SET statement_timeout keine andere Abfrage trifft. Ein unvollstaendiger
// Index aus einem abgebrochenen CONCURRENTLY-Lauf gilt fuer IF NOT EXISTS
// als vorhanden, ist aber unbrauchbar; der wird vorher entfernt.
func (cs *ChainState) indexNebenlaeufigSicherstellen(name, definition string) error {
	ctx := context.Background()
	conn, err := cs.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var gueltig sql.NullBool
	_ = conn.QueryRowContext(ctx,
		`SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = $1`,
		name).Scan(&gueltig)
	if gueltig.Valid && gueltig.Bool {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `SET statement_timeout = 0`); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, `RESET statement_timeout`)
	if gueltig.Valid && !gueltig.Bool {
		if _, err := conn.ExecContext(ctx, `DROP INDEX CONCURRENTLY IF EXISTS `+name); err != nil {
			return err
		}
	}
	begonnen := time.Now()
	if _, err := conn.ExecContext(ctx, `CREATE INDEX CONCURRENTLY IF NOT EXISTS `+name+` ON `+definition); err != nil {
		return err
	}
	fmt.Printf("[AUFRAEUMEN] Index %s gebaut (%s, nebenlaeufig, ohne Sperre)\n", name, time.Since(begonnen).Round(time.Millisecond))
	return nil
}

// pruneTxReceiptsBegrenzt haelt evm_tx_receipts bei den receiptPruneKeep
// neuesten Zeilen -- in Stuecken, mit Budget. Ersetzt die eine grosse
// Anweisung (siehe Kopf). Laeuft im Hintergrund, einfach belegt
// (receiptPruneRunning).
func (cs *ChainState) pruneTxReceiptsBegrenzt() {
	receiptPruneLaeufe.Add(1)
	receiptPruneLetzterLauf.Store(time.Now().Unix())
	if !receiptIndexBereit.Load() {
		if err := cs.indexNebenlaeufigSicherstellen("idx_evm_tx_receipts_created_at", "evm_tx_receipts (created_at)"); err != nil {
			receiptPruneFehler.Add(1)
			fmt.Printf("[EVM] receipt prune: Index auf created_at fehlt noch: %v -- naechster Versuch im naechsten Intervall\n", err)
			return
		}
		receiptIndexBereit.Store(true)
	}
	// Grenze: created_at der (keep+1)-neuesten Zeile. Ueber den Index ist
	// das ein kurzer Rueckwaertslauf; ohne ihn waere es die Sortierung der
	// ganzen Tabelle -- genau der Fehler von vorher.
	var grenze int64
	err := cs.db.QueryRow(
		`SELECT created_at FROM evm_tx_receipts ORDER BY created_at DESC OFFSET $1 LIMIT 1`,
		receiptPruneKeep,
	).Scan(&grenze)
	if err == sql.ErrNoRows {
		return // weniger als keep Zeilen: nichts zu tun
	}
	if err != nil {
		receiptPruneFehler.Add(1)
		fmt.Printf("[EVM] receipt prune: Grenze nicht bestimmbar: %v — retrying at the next interval\n", err)
		return
	}
	frist := time.Now().Add(aufraeumBudget)
	var geloescht int64
	for {
		res, err := cs.db.Exec(
			`DELETE FROM evm_tx_receipts WHERE tx_hash IN (
			   SELECT tx_hash FROM evm_tx_receipts WHERE created_at < $1 LIMIT $2)`,
			grenze, aufraeumStueck,
		)
		if err != nil {
			receiptPruneFehler.Add(1)
			fmt.Printf("[EVM] receipt prune failed after %d row(s): %v — retrying at the next interval\n", geloescht, err)
			break
		}
		n, _ := res.RowsAffected()
		geloescht += n
		receiptPruneGeloescht.Add(n)
		if n < aufraeumStueck {
			break // fertig
		}
		if time.Now().After(frist) {
			receiptPruneBudgetErsch.Add(1)
			// Sofort weitermachen duerfen: das Intervall gilt fuer den
			// Normalfall, nicht fuer einen Berg.
			receiptPruneLastAt.Store(0)
			fmt.Printf("[EVM] receipt prune: %d Zeilen in diesem Lauf, Budget erschoepft -- weiter im naechsten\n", geloescht)
			break
		}
	}
	if geloescht >= aufraeumStueck {
		fmt.Printf("[EVM] receipt prune: %d Zeile(n) entfernt (Grenze created_at < %d)\n", geloescht, grenze)
	}
}

// ReceiptPruneStand fuer /api/health/combined.
func ReceiptPruneStand() map[string]interface{} {
	letzter := ""
	if t := receiptPruneLetzterLauf.Load(); t > 0 {
		letzter = time.Unix(t, 0).UTC().Format(time.RFC3339)
	}
	return map[string]interface{}{
		"bedeutung": "evm_tx_receipts wird auf die " + fmt.Sprint(receiptPruneKeep) + " neuesten Zeilen gehalten, in Stuecken von " +
			fmt.Sprint(aufraeumStueck) + " mit " + aufraeumBudget.String() + " Budget je Lauf. budget_erschoepft > 0 heisst: es lag ein Berg, " +
			"der ueber mehrere Laeufe abgetragen wird. index=false: der Index auf created_at ist noch nicht gebaut, bis dahin raeumt nichts.",
		"geloescht":         receiptPruneGeloescht.Load(),
		"laeufe":            receiptPruneLaeufe.Load(),
		"fehler":            receiptPruneFehler.Load(),
		"budget_erschoepft": receiptPruneBudgetErsch.Load(),
		"letzter_lauf":      letzter,
		"index":             receiptIndexBereit.Load(),
	}
}

// PendingLeichenAufraeumen ersetzt ResetStaleIncludedPendingTxs: markierte
// Zeilen zwischen maxAge und pendingLeicheAlter werden wieder geoeffnet
// (Absturz vor dem Broadcast, wie bisher), aeltere geloescht (Muell aus
// einer gescheiterten Loeschung, siehe Kopf) -- beides in Stuecken mit
// Budget. Einfach belegt; ein zweiter Aufruf waehrend eines Laufs kehrt
// sofort zurueck.
func (cs *ChainState) PendingLeichenAufraeumen(maxAge time.Duration) {
	if cs.db == nil {
		return
	}
	if !pendingLeichenSweepLaeuft.CompareAndSwap(false, true) {
		return
	}
	defer pendingLeichenSweepLaeuft.Store(false)
	pendingLeichenLaeufe.Add(1)
	pendingLeichenLetzterLauf.Store(time.Now().Unix())
	if !pendingLeichenIndexBereit.Load() {
		if err := cs.indexNebenlaeufigSicherstellen("idx_pending_txs_markiert", "pending_txs (included_at) WHERE included_at > 0"); err != nil {
			pendingLeichenFehler.Add(1)
			fmt.Printf("[TX] pending_txs-Sweep: Index fehlt noch: %v -- naechster Versuch im naechsten Lauf\n", err)
			return
		}
		pendingLeichenIndexBereit.Store(true)
	}
	jetzt := time.Now()
	frischGrenze := jetzt.Add(-maxAge).Unix()
	leichenGrenze := jetzt.Add(-pendingLeicheAlter).Unix()
	frist := jetzt.Add(aufraeumBudget)

	// 1. Wieder oeffnen, was jung genug ist, um eine Absturzluecke zu sein.
	var wiederOffen int64
	for {
		res, err := cs.db.Exec(
			`UPDATE pending_txs SET included_at = 0, included_block_hash = NULL
			 WHERE id IN (
			   SELECT id FROM pending_txs
			    WHERE included_at > 0 AND included_at < $1 AND included_at >= $2
			      AND (included_block_hash IS NULL
			           OR NOT EXISTS (SELECT 1 FROM chain_blocks WHERE hash = pending_txs.included_block_hash))
			    LIMIT $3)`,
			frischGrenze, leichenGrenze, aufraeumStueck,
		)
		if err != nil {
			pendingLeichenFehler.Add(1)
			fmt.Printf("[TX] pending_txs-Sweep (wieder oeffnen) error: %v\n", err)
			return
		}
		n, _ := res.RowsAffected()
		wiederOffen += n
		pendingLeichenWiederOffen.Add(n)
		if n < aufraeumStueck || time.Now().After(frist) {
			break
		}
	}
	if wiederOffen > 0 {
		fmt.Printf("[TX] Reset %d stale-included pending_txs row(s) for retry (likely a crash before broadcast)\n", wiederOffen)
	}

	// 2. Loeschen, was zu alt ist, um noch irgendwohin zu gehoeren.
	var geloescht int64
	for {
		res, err := cs.db.Exec(
			`DELETE FROM pending_txs WHERE id IN (
			   SELECT id FROM pending_txs WHERE included_at > 0 AND included_at < $1 LIMIT $2)`,
			leichenGrenze, aufraeumStueck,
		)
		if err != nil {
			pendingLeichenFehler.Add(1)
			fmt.Printf("[TX] pending_txs-Sweep (loeschen) error after %d row(s): %v\n", geloescht, err)
			return
		}
		n, _ := res.RowsAffected()
		geloescht += n
		pendingLeichenGeloescht.Add(n)
		if n < aufraeumStueck {
			break
		}
		if time.Now().After(frist) {
			pendingLeichenBudgetErsch.Add(1)
			fmt.Printf("[TX] pending_txs-Sweep: %d Leichen in diesem Lauf, Budget erschoepft -- weiter im naechsten\n", geloescht)
			break
		}
	}
	if geloescht > 0 {
		fmt.Printf("[TX] pending_txs-Sweep: %d markierte Zeile(n) aelter als %s geloescht -- nie geloeschte Reste, keine Absturzluecke\n", geloescht, pendingLeicheAlter)
	}
}

// PendingLeichenAufraeumenStart: einmal jetzt (Absturzluecke vor der ersten
// Produktion schliessen, wie bisher), dann stuendlich im Hintergrund, damit
// sich Reste nie wieder zu Millionen ansammeln.
func (cs *ChainState) PendingLeichenAufraeumenStart(maxAge time.Duration) {
	if cs.db == nil {
		return
	}
	cs.PendingLeichenAufraeumen(maxAge)
	if !pendingLeichenTickerLaeuft.CompareAndSwap(false, true) {
		return
	}
	SafeGoroutine("pendingLeichenSweep", func() {
		t := time.NewTicker(pendingLeichenTakt)
		defer t.Stop()
		for range t.C {
			SafeCall("pendingLeichenSweep-tick", func() { cs.PendingLeichenAufraeumen(maxAge) })
		}
	})
}

// PendingLeichenStand fuer /api/health/combined.
func PendingLeichenStand() map[string]interface{} {
	letzter := ""
	if t := pendingLeichenLetzterLauf.Load(); t > 0 {
		letzter = time.Unix(t, 0).UTC().Format(time.RFC3339)
	}
	return map[string]interface{}{
		"bedeutung": "pending_txs: markierte Zeilen juenger als " + pendingLeicheAlter.String() + " werden wieder geoeffnet (Absturz vor dem Broadcast), " +
			"aeltere geloescht (Reste einer gescheiterten Loeschung). Beim Start und alle " + pendingLeichenTakt.String() + ", in Stuecken von " +
			fmt.Sprint(aufraeumStueck) + ". geloescht sollte nach dem ersten Lauf nicht mehr wachsen.",
		"geloescht":         pendingLeichenGeloescht.Load(),
		"wieder_offen":      pendingLeichenWiederOffen.Load(),
		"laeufe":            pendingLeichenLaeufe.Load(),
		"fehler":            pendingLeichenFehler.Load(),
		"budget_erschoepft": pendingLeichenBudgetErsch.Load(),
		"letzter_lauf":      letzter,
		"index":             pendingLeichenIndexBereit.Load(),
	}
}
