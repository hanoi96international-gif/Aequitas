package keeper

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
)

// Ausgangszeile und Kontostand eines WAL-Satzes getrennt nachweisen
// (docs/OFFEN.md, Punkt 9; Pruefung von #331, Befund 1 und 2. Durchgang).
//
// Der Wiederanlauf entschied bisher allein ueber chain_accounts.wal_seq je
// Konto. Das heisst "der Saldo enthaelt den Satz", aber nicht "seine Zeile
// in pending_txs ist geschrieben":
//
//	(a) Serielle Speicherungen (saveAccountToDBInnerCtx,
//	    saveAccountsToDBBatchCtx) schrieben den Saldo samt noch nicht
//	    geflushter WAL-Saetze, aber nie wal_seq. Ein Absturz im
//	    Flush-Fenster liess den Wiederanlauf die Abbuchung ein zweites Mal
//	    anwenden (PoC: 2104,1 -> 1203,2).
//	(b) Der Flush schreibt je Konto den AKTUELLEN WALSeq, der Saetze
//	    spaeterer oder parallel laufender Stapel einschliesst. Committen
//	    andere Stapel beide Konten eines Satzes, dessen eigener Stapel noch
//	    offen ist, hielt der Wiederanlauf ihn fuer erledigt -- seine Zeile
//	    fehlte dem Netz, der Saldo dieses Knotens trug ihn trotzdem.
//
// Deshalb jetzt zwei Nachweise:
//
//   - Saldo: wal_seq je Konto, ab jetzt von JEDER Speicherung mitgeschrieben
//     (monoton, GREATEST), die einen Saldo mit WAL-Saetzen schreibt. Vorher
//     wartet sie, bis diese Saetze haltbar sind (walSeqZumSpeichern) -- sonst
//     staende nach einem Absturz eine Seq in Postgres, die das WAL nach dem
//     Neustart noch einmal vergibt.
//   - Ausgangszeile: ein Vermerk je Satz (wal_ausgang_vermerk), geschrieben
//     in DERSELBEN Transaktion wie die Zeile. Fehlt er, schreibt der
//     Wiederanlauf die Zeile nach -- auch wenn beide Salden den Satz schon
//     tragen.
//
// Untergrenze (chain_config walVermerkAbKey): unter ihr gibt es keine
// Vermerke -- Saetze von vor dieser Fassung (beim ersten Start mit ihr auf
// das Dateiende gesetzt), Saetze, die schon in einem Block stehen
// (Speicherkorb ausgeschaltet), und Saetze, die die Kompaktierung als
// geschrieben nachweist (walKuerzenBis; dort werden die Vermerke geloescht).
// Unter ihr gilt die alte Regel. Speicher: die Tabelle haelt nur Saetze ueber
// der letzten Kompaktierungsgrenze; der Wiederanlauf liest sie in Portionen.
//
// RUECKSCHRITT auf eine Fassung ohne Vermerke: die schreibt Zeilen ohne
// Vermerk. Vor dem erneuten Wechsel auf diese Fassung den Schluessel
// walVermerkAbKey loeschen -- dann setzt der naechste Start die Untergrenze
// neu auf das Dateiende (alte Regel fuer alles bis dahin). Sonst schriebe der
// Wiederanlauf fuer jene Saetze eine zweite Zeile.
const walVermerkAbKey = "wal_ausgang_vermerk_ab"

// walVermerkPortion: so viele Vermerke liest der Wiederanlauf auf einmal.
const walVermerkPortion = 10_000

// walSeqHaltbarFrist: so lange wartet eine Speicherung auf die Haltbarkeit
// der WAL-Saetze ihres Saldos -- dieselbe Frist wie flushWALBatch. var: Tests
// kuerzen sie.
var walSeqHaltbarFrist = 2 * time.Second

var (
	walSeqOhneHaltbarkeit atomic.Int64
	// walAusgangNachgeholt: Zeilen, die der Wiederanlauf nachschrieb, weil
	// der Vermerk fehlte, beide Salden den Satz aber schon trugen.
	walAusgangNachgeholt atomic.Int64
)

// walSeqHaltbarAbwarten: eine Speicherung schreibt mit dem Saldo dessen
// wal_seq (acc.WALSeq: die hoechste Seq, deren Wirkung der Saldo traegt; der
// Aufrufer haelt die Sperre des Kontos oder cs.mu). Vorher muessen diese
// Saetze haltbar sein -- sonst stuende nach einem Absturz eine Seq in
// Postgres, die das WAL beim Neustart noch einmal vergibt. Wie der Flush:
// kommt die Haltbarkeit nicht, wird trotzdem geschrieben, aber der schnelle
// Pfad schliesst (walSchnellpfadDefekt), und danach wartet keine
// Speicherung mehr (Pruefung #332, MEDIUM-2: sonst je Speicherung 2 s unter
// cs.mu). Eine so geschriebene wal_seq ueber dem WAL-Kopf haelt der naechste
// Start an (walVermerkeStimmigMitDatei, MEDIUM-1).
func (cs *ChainState) walSeqHaltbarAbwarten(seq uint64, wer string) {
	w := cs.wal
	if seq == 0 || w == nil || w.DurableSeq() >= seq || walSchnellpfadDefekt.Load() {
		return
	}
	if w.WaitDurable(seq, walSeqHaltbarFrist) {
		return
	}
	walSeqOhneHaltbarkeit.Add(1)
	if !walSchnellpfadDefekt.Swap(true) {
		fmt.Printf("[WAL] ✗ Speicherung (%s) wartet vergeblich auf die Haltbarkeit von Seq %d -- schneller Pfad geschlossen; wal_seq wird trotzdem geschrieben, ein Neustart prueft sie gegen den WAL-Kopf\n", wer, seq)
	}
}

// walVermerkAbLesen: die Untergrenze; da=false, wenn sie nie gesetzt wurde.
// Ein Lesefehler oder ein unlesbarer Wert ist ein Fehler (fail-closed): eine
// zu niedrige Grenze schriebe Zeilen ein zweites Mal, eine zu hohe liesse
// fehlende weg.
func (cs *ChainState) walVermerkAbLesen() (ab uint64, da bool, err error) {
	var v string
	err = cs.db.QueryRow(`SELECT value FROM chain_config WHERE key = $1`, walVermerkAbKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("%s nicht lesbar: %w", walVermerkAbKey, err)
	}
	n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s unlesbar (%q): %w", walVermerkAbKey, v, err)
	}
	return n, true, nil
}

// walVermerkAbAnheben setzt die Untergrenze auf mindestens ab und loescht
// die Vermerke darunter -- in einer Transaktion. Nie abwaerts.
func (cs *ChainState) walVermerkAbAnheben(ab uint64) error {
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
WHERE chain_config.value !~ '^[0-9]+$' OR chain_config.value::numeric < EXCLUDED.value::numeric`, walVermerkAbKey, strconv.FormatUint(ab, 10)); err != nil {
		return fmt.Errorf("%s nicht gesetzt: %w", walVermerkAbKey, err)
	}
	if _, err := tx.Exec(`DELETE FROM wal_ausgang_vermerk WHERE seq < $1`, int64(ab)); err != nil {
		return fmt.Errorf("Vermerke unter %d nicht geloescht: %w", ab, err)
	}
	return tx.Commit()
}

// walVermerkeSchreiben: Vermerke fuer seqs, in der Transaktion von ctx.
func (cs *ChainState) walVermerkeSchreiben(ctx context.Context, seqs []int64) error {
	if len(seqs) == 0 {
		return nil
	}
	_, err := cs.dbExecCtx(ctx).Exec(`INSERT INTO wal_ausgang_vermerk (seq) SELECT unnest($1::bigint[]) ON CONFLICT DO NOTHING`, pq.Array(seqs))
	return err
}

// walAusgangNachholen schreibt die Zeile eines Satzes, dessen Salden schon in
// Postgres stehen, und seinen Vermerk -- zusammen. Gegen eine doppelte Zeile
// geschuetzt, solange die erste noch in pending_txs steht.
func (cs *ChainState) walAusgangNachholen(t Transaction, seq uint64) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO pending_txs (tx_json, created_at, wal_seq)
SELECT $1::text, $2::bigint, $3::bigint
WHERE NOT EXISTS (SELECT 1 FROM pending_txs WHERE wal_seq = $3::bigint AND tx_json = $1::text)`,
		string(data), time.Now().Unix(), int64(seq)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO wal_ausgang_vermerk (seq) VALUES ($1) ON CONFLICT DO NOTHING`, int64(seq)); err != nil {
		return err
	}
	return tx.Commit()
}

// walAusgang beantwortet fuer den Wiederanlauf und die Startpruefung, ob die
// Zeile eines Satzes nachweislich geschrieben ist (oder keine noetig war).
// Saetze kommen in steigender Seq (Dateireihenfolge); die Vermerke werden in
// Portionen ab der gefragten Seq gelesen. Eine kleinere Seq als die zuletzt
// gefragte fragt einzeln nach.
type walAusgang struct {
	db             *sql.DB
	ab             uint64 // Untergrenze: darunter alte Regel
	markeDa        bool   // Speicherkorb-Marke: bis dahin in Bloecken
	marke          uint64
	puffer         []uint64
	bis            uint64 // gelesen bis hier (einschliesslich)
	bereichGelesen bool
	letzteGefragte uint64
}

func (cs *ChainState) neuerWALAusgang(ab uint64, markeDa bool, marke uint64) *walAusgang {
	return &walAusgang{db: cs.db, ab: ab, markeDa: markeDa, marke: marke}
}

// alteRegel: unter der Untergrenze gibt es keine Vermerke.
func (a *walAusgang) alteRegel(seq uint64) bool { return seq < a.ab }

// erledigt: die Zeile ist geschrieben, oder der Satz steht schon in einem
// Block (Speicherkorb-Marke). Nur fuer Saetze ueber der Untergrenze.
func (a *walAusgang) erledigt(seq uint64) (bool, error) {
	if a.markeDa && seq <= a.marke {
		return true, nil
	}
	if seq < a.letzteGefragte {
		var n int
		err := a.db.QueryRow(`SELECT count(*) FROM wal_ausgang_vermerk WHERE seq = $1`, int64(seq)).Scan(&n)
		return n > 0, err
	}
	a.letzteGefragte = seq
	if !a.bereichGelesen || seq > a.bis {
		if err := a.lesen(seq); err != nil {
			return false, err
		}
	}
	for len(a.puffer) > 0 && a.puffer[0] < seq {
		a.puffer = a.puffer[1:]
	}
	return len(a.puffer) > 0 && a.puffer[0] == seq, nil
}

// lesen: die naechste Portion Vermerke ab seq. Ist sie voll, reicht der
// gelesene Bereich bis zum letzten Vermerk, sonst bis zum Ende.
func (a *walAusgang) lesen(seq uint64) error {
	rows, err := a.db.Query(`SELECT seq FROM wal_ausgang_vermerk WHERE seq >= $1 ORDER BY seq LIMIT $2`, int64(seq), walVermerkPortion)
	if err != nil {
		return fmt.Errorf("Vermerke nicht lesbar: %w", err)
	}
	defer rows.Close()
	a.puffer = a.puffer[:0]
	for rows.Next() {
		var s int64
		if err := rows.Scan(&s); err != nil {
			return fmt.Errorf("Vermerke nicht lesbar: %w", err)
		}
		a.puffer = append(a.puffer, uint64(s))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("Vermerke nicht lesbar: %w", err)
	}
	a.bereichGelesen = true
	a.bis = ^uint64(0)
	if len(a.puffer) == walVermerkPortion {
		a.bis = a.puffer[len(a.puffer)-1]
	}
	return nil
}

// walVermerkeStimmigMitDatei: kein Vermerk und keine Untergrenze ueber dem,
// was das WAL schon vergeben hat. Sonst wurde die Datei ersetzt oder
// geloescht, und die Zaehlung beginnt neu -- neue Saetze truegen dann fremde
// Vermerke (fail-closed, siehe PruefeWALRest: WAL-Datei nie loeschen).
func (cs *ChainState) walVermerkeStimmigMitDatei(naechste uint64) error {
	ab, da, err := cs.walVermerkAbLesen()
	if err != nil {
		return err
	}
	if da && ab > naechste {
		return fmt.Errorf("Untergrenze der Ausgangsvermerke (%d) liegt ueber der naechsten Seq des WAL (%d) -- wurde die WAL-Datei ersetzt oder geloescht?", ab, naechste)
	}
	var hoechster sql.NullInt64
	if err := cs.db.QueryRow(`SELECT max(seq) FROM wal_ausgang_vermerk`).Scan(&hoechster); err != nil {
		return fmt.Errorf("Vermerke nicht lesbar: %w", err)
	}
	if hoechster.Valid && uint64(hoechster.Int64) >= naechste {
		return fmt.Errorf("Ausgangsvermerk fuer Seq %d, das WAL vergibt erst ab %d -- wurde die WAL-Datei ersetzt oder geloescht?", hoechster.Int64, naechste)
	}
	// Dasselbe fuer die Salden (Pruefung #332, MEDIUM-1): eine wal_seq, die
	// das WAL noch einmal vergibt, liesse einen neuen Satz auf diesem Konto
	// als "schon angewandt" gelten -- die Gegenseite bekaeme ihn, dieses
	// Konto nicht. Entsteht, wenn eine Speicherung nach ausgebliebener
	// Haltbarkeit schrieb und der Satz mit dem Absturz verloren ging.
	var hoechsteKonto sql.NullInt64
	if err := cs.db.QueryRow(`SELECT max(wal_seq) FROM chain_accounts`).Scan(&hoechsteKonto); err != nil {
		return fmt.Errorf("wal_seq der Konten nicht lesbar: %w", err)
	}
	if hoechsteKonto.Valid && hoechsteKonto.Int64 > 0 && uint64(hoechsteKonto.Int64) >= naechste {
		return fmt.Errorf("ein Konto traegt wal_seq %d, das WAL vergibt erst ab %d -- ein Saldo enthaelt einen Satz, den die Datei nicht hat (Datei ersetzt/geloescht oder Sync-Ausfall vor einem Absturz). Bewusst verwerfen: AEQUITAS_WAL_REST_VERWERFEN", hoechsteKonto.Int64, naechste)
	}
	return nil
}

// walVermerkeVerwerfen: nach einem Zustandsersatz oder bewussten Verwerfen
// bis head (markWALSupersededByStateReplacement) gehoert nichts bis head mehr
// zum Wiederanlauf. Vermerke weg, Untergrenze auf head+1 (auch abwaerts --
// eine Untergrenze aus einer fremden Datei waere sonst nie mehr zu loesen),
// wal_seq ueber head auf 0: unter dem Boden wird ohnehin nichts angewandt, und
// eine hoehere wal_seq stammt aus einer anderen Zaehlung (Pruefung #332,
// LOW-4: sonst drehte sich VERWERFEN im Kreis).
func (cs *ChainState) walVermerkeVerwerfen(head uint64) error {
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM wal_ausgang_vermerk`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, walVermerkAbKey, strconv.FormatUint(head+1, 10)); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE chain_accounts SET wal_seq = 0 WHERE wal_seq > $1`, int64(head)); err != nil {
		return err
	}
	return tx.Commit()
}

// walSatzVerblockt: steht die Ueberweisung laut Block-Index schon in einem
// Block? Nur ein zusaetzlicher Schutz vor dem Nachholen einer Zeile (der
// Index wird nachgezogen und gekuerzt, ein fehlender Eintrag beweist nichts):
// ein Rueckschritt auf eine Fassung ohne Vermerke, ohne den Schluessel zu
// loeschen, schriebe sonst verblockte Ueberweisungen ein zweites Mal
// (Pruefung #332, MEDIUM-3). Ein Lesefehler ist ein Fehler.
func (cs *ChainState) walSatzVerblockt(txHash string) (bool, error) {
	h := strings.ToLower(strings.TrimSpace(txHash))
	if h == "" {
		return false, nil
	}
	cs.ensureTxBlockIndexTable()
	var da bool
	if err := cs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM chain_tx_block_index WHERE tx_hash = $1)`, h).Scan(&da); err != nil {
		return false, fmt.Errorf("Block-Index nicht lesbar: %w", err)
	}
	return da, nil
}

// walNurVermerk: der Satz steht schon in einem Block, nur der Vermerk fehlt.
func (cs *ChainState) walNurVermerk(seq uint64) error {
	_, err := cs.db.Exec(`INSERT INTO wal_ausgang_vermerk (seq) VALUES ($1) ON CONFLICT DO NOTHING`, int64(seq))
	return err
}
