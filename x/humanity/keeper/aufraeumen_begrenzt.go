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
//
// WARUM SEITENBEREICHE (ctid). Die erste Fassung loeschte "WHERE tx_hash IN
// (SELECT ... LIMIT 10000)". Live auf C2 (21:29Z) lief auch das in den
// Timeout: 10.000 Einzelzugriffe auf den kalten 1,6-GB-Primaerschluessel
// sind Zufalls-I/O, rund 10 s je Stueck. Und jede Form, die "die naechsten
// n Zeilen" sucht, muss nach Millionen Loeschungen erst die toten Zeilen
// am Tabellenanfang ueberspringen, bis Vacuum sie wegraeumt -- wird je
// Stueck langsamer, bis wieder das Limit reisst. Deshalb laeuft der
// Quittungs-Aufraeumer ueber die physischen Seiten: "ctid >= '(p,0)' AND
// ctid < '(p+n,0)'" ist in Postgres 16 ein Tid Range Scan, liest genau
// diese n Seiten, nie etwas zweimal, ohne Index, egal wie gross die
// Tabelle ist. Ein Durchlauf merkt sich die naechste Seite und macht nach
// erschoepftem Budget dort weiter; ein neuer Durchlauf beginnt nur, wenn
// die Statistik mehr als das Doppelte des Behalts meldet.

const (
	// aufraeumStueck: Zeilen je Anweisung (pending_txs-Sweep).
	aufraeumStueck = 10000
	// aufraeumSeiten: Heap-Seiten je Anweisung (Quittungen). 512 Seiten sind
	// 4 MB, rund 20.000 Zeilen -- sequenziell gelesen, zweistellige
	// Millisekunden.
	aufraeumSeiten = 512
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
	receiptPruneSeite          atomic.Int64 // naechste Heap-Seite des laufenden Durchlaufs, 0 = von vorn
	receiptPruneDurchlaeufe    atomic.Int64
	receiptPruneImDurchlauf    atomic.Int64 // im laufenden Durchlauf geloescht (fuer die Nachsorge)
	receiptNachsorgen          atomic.Int64
	receiptBlaehungGeprueft    atomic.Int64 // unix, letzte Blaehungspruefung
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
	// Tor: nur wenn die Statistik deutlich mehr als den Behalt meldet.
	// n_live_tup ist eine Schaetzung, aber eine billige -- ein count(*)
	// waere ein Lauf ueber die ganze Tabelle, und genau den sparen wir uns,
	// wenn es nichts zu tun gibt.
	if receiptPruneSeite.Load() == 0 {
		var lebend int64
		if err := cs.db.QueryRow(
			`SELECT COALESCE(n_live_tup, 0) FROM pg_stat_user_tables WHERE relname = 'evm_tx_receipts'`,
		).Scan(&lebend); err == nil && lebend <= 2*receiptPruneKeep {
			// Nichts zu loeschen -- aber vielleicht ein aufgeblaehter Index
			// aus einem frueheren Durchlauf (auch aus einem frueheren
			// Prozess: der Zaehler dafuer lebt nur im Speicher).
			cs.receiptIndexBlaehungPruefen(lebend)
			return
		}
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
		receiptPruneSeite.Store(0)
		return // weniger als keep Zeilen: nichts zu tun
	}
	if err != nil {
		receiptPruneFehler.Add(1)
		fmt.Printf("[EVM] receipt prune: Grenze nicht bestimmbar: %v — retrying at the next interval\n", err)
		return
	}
	var seiten int64
	if err := cs.db.QueryRow(
		`SELECT pg_relation_size('evm_tx_receipts') / current_setting('block_size')::bigint`,
	).Scan(&seiten); err != nil {
		receiptPruneFehler.Add(1)
		fmt.Printf("[EVM] receipt prune: Tabellengroesse nicht bestimmbar: %v — retrying at the next interval\n", err)
		return
	}
	frist := time.Now().Add(aufraeumBudget)
	var geloescht int64
	for {
		von := receiptPruneSeite.Load()
		if von >= seiten {
			receiptPruneSeite.Store(0)
			receiptPruneDurchlaeufe.Add(1)
			imDurchlauf := receiptPruneImDurchlauf.Swap(0)
			if imDurchlauf >= receiptNachsorgeAb {
				cs.receiptNachsorge(imDurchlauf)
			}
			break // Durchlauf fertig
		}
		bis := von + aufraeumSeiten
		// Die Seitenzahlen sind eigene Ganzzahlen, keine Eingabe -- deshalb
		// duerfen sie in den Text: als Parameter wuerde der Planer den
		// Tid Range Scan nicht sicher waehlen.
		res, err := cs.db.Exec(fmt.Sprintf(
			`DELETE FROM evm_tx_receipts WHERE ctid >= '(%d,0)'::tid AND ctid < '(%d,0)'::tid AND created_at < $1`,
			von, bis), grenze)
		if err != nil {
			receiptPruneFehler.Add(1)
			fmt.Printf("[EVM] receipt prune failed at page %d after %d row(s): %v — retrying at the next interval\n", von, geloescht, err)
			break
		}
		n, _ := res.RowsAffected()
		geloescht += n
		receiptPruneGeloescht.Add(n)
		receiptPruneImDurchlauf.Add(n)
		receiptPruneSeite.Store(bis)
		if time.Now().After(frist) {
			receiptPruneBudgetErsch.Add(1)
			// Sofort weitermachen duerfen: das Intervall gilt fuer den
			// Normalfall, nicht fuer einen Berg.
			receiptPruneLastAt.Store(0)
			fmt.Printf("[EVM] receipt prune: %d Zeilen in diesem Lauf, Seite %d von %d, Budget erschoepft -- weiter gleich\n", geloescht, bis, seiten)
			break
		}
	}
	if geloescht >= aufraeumStueck {
		fmt.Printf("[EVM] receipt prune: %d Zeile(n) entfernt (Grenze created_at < %d)\n", geloescht, grenze)
	}
}

// receiptNachsorgeAb: ab so vielen Loeschungen in einem Durchlauf sind die
// Indizes danach zum groessten Teil tot -- und Vacuum verkleinert keinen
// Index. 14.09.2026, C2: nach 13,2 Millionen Loeschungen trug der
// Primaerschluessel 1,6 GB mit 9 Millionen toten Eintraegen, jede
// Einfuegung war Zufalls-I/O, der Flush lief weiter in den Timeout.
const receiptNachsorgeAb = 1_000_000

// receiptIndexBlaehungPruefen: ist der Primaerschluessel viel groesser, als
// die lebenden Zeilen rechtfertigen, wird er neu gebaut. Ein gesunder
// Eintrag (66 Zeichen Hash) kostet um die 100 Byte; ueber 1 KB je lebender
// Zeile bei mehr als 64 MB ist ein Index, der fast nur aus Toten besteht.
// Einmal je Prozess und hoechstens alle 10 Minuten geprueft (eine
// Katalogabfrage).
func (cs *ChainState) receiptIndexBlaehungPruefen(lebend int64) {
	jetzt := time.Now().Unix()
	if jetzt-receiptBlaehungGeprueft.Load() < 600 {
		return
	}
	receiptBlaehungGeprueft.Store(jetzt)
	var pkeyBytes int64
	if err := cs.db.QueryRow(`SELECT pg_relation_size('evm_tx_receipts_pkey')`).Scan(&pkeyBytes); err != nil {
		return
	}
	if lebend < 1 {
		lebend = 1
	}
	if pkeyBytes > 64<<20 && pkeyBytes/lebend > 1024 {
		fmt.Printf("[AUFRAEUMEN] evm_tx_receipts_pkey ist %d MB fuer %d lebende Zeilen -- fast nur Tote, wird neu gebaut\n", pkeyBytes>>20, lebend)
		cs.receiptNachsorge(0)
	}
}

// receiptNachsorge baut die Indizes von evm_tx_receipts nebenlaeufig neu
// (REINDEX CONCURRENTLY: sperrt weder Lesen noch Schreiben) und frischt die
// Statistik auf -- auf der eigenen Verbindung ohne Zeitlimit. Laeuft im
// Aufraeum-Goroutine, also einfach belegt.
func (cs *ChainState) receiptNachsorge(geloescht int64) {
	ctx := context.Background()
	conn, err := cs.db.Conn(ctx)
	if err != nil {
		fmt.Printf("[AUFRAEUMEN] Nachsorge evm_tx_receipts: keine Verbindung: %v\n", err)
		return
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SET statement_timeout = 0`); err != nil {
		fmt.Printf("[AUFRAEUMEN] Nachsorge evm_tx_receipts: %v\n", err)
		return
	}
	defer conn.ExecContext(ctx, `RESET statement_timeout`)
	fmt.Printf("[AUFRAEUMEN] Nachsorge evm_tx_receipts nach %d Loeschungen: Indizes neu bauen (nebenlaeufig)\n", geloescht)
	for _, idx := range []string{"evm_tx_receipts_pkey", "idx_evm_tx_receipts_created_at"} {
		begonnen := time.Now()
		if _, err := conn.ExecContext(ctx, `REINDEX INDEX CONCURRENTLY `+idx); err != nil {
			fmt.Printf("[AUFRAEUMEN] REINDEX %s fehlgeschlagen: %v -- beim naechsten grossen Durchlauf erneut\n", idx, err)
			continue
		}
		fmt.Printf("[AUFRAEUMEN] REINDEX %s fertig (%s)\n", idx, time.Since(begonnen).Round(time.Millisecond))
	}
	if _, err := conn.ExecContext(ctx, `ANALYZE evm_tx_receipts`); err != nil {
		fmt.Printf("[AUFRAEUMEN] ANALYZE evm_tx_receipts: %v\n", err)
	}
	receiptNachsorgen.Add(1)
}

// ReceiptPruneStand fuer /api/health/combined.
func ReceiptPruneStand() map[string]interface{} {
	letzter := ""
	if t := receiptPruneLetzterLauf.Load(); t > 0 {
		letzter = time.Unix(t, 0).UTC().Format(time.RFC3339)
	}
	return map[string]interface{}{
		"bedeutung": "evm_tx_receipts wird auf die " + fmt.Sprint(receiptPruneKeep) + " neuesten Zeilen gehalten: Durchlauf ueber die Heap-Seiten in Bereichen von " +
			fmt.Sprint(aufraeumSeiten) + " Seiten mit " + aufraeumBudget.String() + " Budget je Lauf, naechste_seite zeigt den Stand (0 = kein Durchlauf offen). " +
			"budget_erschoepft > 0 heisst: es lag ein Berg, der ueber mehrere Laeufe abgetragen wird. index=false: der Index auf created_at ist noch nicht gebaut, bis dahin raeumt nichts.",
		"geloescht":         receiptPruneGeloescht.Load(),
		"naechste_seite":    receiptPruneSeite.Load(),
		"durchlaeufe":       receiptPruneDurchlaeufe.Load(),
		"nachsorgen":        receiptNachsorgen.Load(),
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
func (cs *ChainState) PendingLeichenAufraeumen(maxAge time.Duration) (fertig bool) {
	if cs.db == nil {
		return true
	}
	if !pendingLeichenSweepLaeuft.CompareAndSwap(false, true) {
		return true
	}
	defer pendingLeichenSweepLaeuft.Store(false)
	fertig = true
	pendingLeichenLaeufe.Add(1)
	pendingLeichenLetzterLauf.Store(time.Now().Unix())
	if !pendingLeichenIndexBereit.Load() {
		if err := cs.indexNebenlaeufigSicherstellen("idx_pending_txs_markiert", "pending_txs (included_at) WHERE included_at > 0"); err != nil {
			pendingLeichenFehler.Add(1)
			fmt.Printf("[TX] pending_txs-Sweep: Index fehlt noch: %v -- naechster Versuch im naechsten Lauf\n", err)
			return false
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
			 WHERE ctid = ANY(ARRAY(
			   SELECT ctid FROM pending_txs
			    WHERE included_at > 0 AND included_at < $1 AND included_at >= $2
			      AND (included_block_hash IS NULL
			           OR NOT EXISTS (SELECT 1 FROM chain_blocks WHERE hash = pending_txs.included_block_hash))
			    LIMIT $3))`,
			frischGrenze, leichenGrenze, aufraeumStueck,
		)
		if err != nil {
			pendingLeichenFehler.Add(1)
			fmt.Printf("[TX] pending_txs-Sweep (wieder oeffnen) error: %v\n", err)
			return false
		}
		n, _ := res.RowsAffected()
		wiederOffen += n
		pendingLeichenWiederOffen.Add(n)
		if n < aufraeumStueck {
			break
		}
		if time.Now().After(frist) {
			fertig = false
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
			`DELETE FROM pending_txs WHERE ctid = ANY(ARRAY(
			   SELECT ctid FROM pending_txs WHERE included_at > 0 AND included_at < $1 LIMIT $2))`,
			leichenGrenze, aufraeumStueck,
		)
		if err != nil {
			pendingLeichenFehler.Add(1)
			fmt.Printf("[TX] pending_txs-Sweep (loeschen) error after %d row(s): %v\n", geloescht, err)
			return false
		}
		n, _ := res.RowsAffected()
		geloescht += n
		pendingLeichenGeloescht.Add(n)
		if n < aufraeumStueck {
			break
		}
		if time.Now().After(frist) {
			pendingLeichenBudgetErsch.Add(1)
			fertig = false
			fmt.Printf("[TX] pending_txs-Sweep: %d Leichen in diesem Lauf, Budget erschoepft -- weiter gleich\n", geloescht)
			break
		}
	}
	if geloescht > 0 {
		fmt.Printf("[TX] pending_txs-Sweep: %d markierte Zeile(n) aelter als %s geloescht -- nie geloeschte Reste, keine Absturzluecke\n", geloescht, pendingLeicheAlter)
	}
	return fertig
}

// PendingLeichenAufraeumenStart: einmal jetzt (Absturzluecke vor der ersten
// Produktion schliessen, wie bisher), ein Berg wird gleich im Hintergrund
// weiter abgetragen, danach stuendlich, damit sich Reste nie wieder zu
// Millionen ansammeln. Startet ausserdem den Quittungs-Flush-Worker: an ihm
// haengt der Quittungs-Aufraeumer, und der darf nicht auf die erste
// Ueberweisung nach einem Neustart warten (C2 am 14.09.2026: 13 Millionen
// Zeilen, kein Verkehr, nichts raeumte).
func (cs *ChainState) PendingLeichenAufraeumenStart(maxAge time.Duration) {
	if cs.db == nil {
		return
	}
	cs.ensureReceiptFlushWorkerStarted()
	fertig := cs.PendingLeichenAufraeumen(maxAge)
	if !pendingLeichenTickerLaeuft.CompareAndSwap(false, true) {
		return
	}
	SafeGoroutine("pendingLeichenSweep", func() {
		for !fertig {
			time.Sleep(2 * time.Second) // der Datenbank Luft lassen
			SafeCall("pendingLeichenSweep-berg", func() { fertig = cs.PendingLeichenAufraeumen(maxAge) })
		}
		t := time.NewTicker(pendingLeichenTakt)
		defer t.Stop()
		for range t.C {
			SafeCall("pendingLeichenSweep-tick", func() {
				for !cs.PendingLeichenAufraeumen(maxAge) {
					time.Sleep(2 * time.Second)
				}
			})
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
