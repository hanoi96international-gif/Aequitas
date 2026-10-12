package keeper

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WAL-AUSGANG (docs/OFFEN.md, Punkt 9; Pruefung von #331, Befund 1).
//
// recoverFromWAL entschied frueher allein ueber chain_accounts.wal_seq, ob
// ein Satz noch angewandt UND ob seine Ausgangszeile (pending_txs) noch
// geschrieben werden muss. wal_seq heisst aber nur "der Saldo enthaelt den
// Satz":
//
//   - Serielle Speicherungen (saveAccountToDBCtx, saveAccountsToDBBatch)
//     schreiben den Saldo samt noch nicht geflushter WAL-Deltas. Ohne
//     wal_seq buchte der Wiederanlauf sie ein zweites Mal ab. Jetzt
//     schreiben sie wal_seq monoton mit.
//   - Ein Flush-Stapel schreibt je Konto den AKTUELLEN Stand samt WALSeq --
//     der kann Saetze einschliessen, deren Ausgangszeile erst ein spaeterer
//     Stapel schreibt. Stuerzte der Knoten dazwischen ab, hielt der
//     Wiederanlauf sie fuer angewandt und schrieb keine Zeile: der Transfer
//     fehlte dem Netz, der Knoten wich ab. Seit serielle Speicherungen
//     wal_seq mitschreiben, gilt das fuer sie ebenso.
//
// Darum steht in wal_ausgang je Satz, ob seine Zeile geschrieben ist --
// in derselben Transaktion wie die Zeile (flushWALBatch)
// und, anders als die Zeile, nicht geloescht, wenn ein Block sie aufnimmt
// oder ein Aufraeumen sie verwirft. Der Wiederanlauf holt fuer jeden Satz
// ohne Vermerk die Zeile nach -- unabhaengig von wal_seq.
//
// UEBERGANG. Saetze, die vor dieser Aenderung geschrieben wurden, haben
// keinen Vermerk, auch wenn ihre Zeile laengst in einem Block steht. Fuer sie
// gilt die alte Regel. Die Grenze (walAusgangAbKey) setzt der erste
// Wiederanlauf mit dieser Fassung auf den hoechsten Satz der Datei; alles
// darueber schreibt ein Flush dieser Fassung mit Vermerk.
//
// ERLEDIGT. Saetze bis walAusgangErledigtKey brauchen nie eine neue Zeile,
// auch ohne Vermerk: die Kompaktierung kuerzt nur, was nachweislich in
// Postgres ist (und loescht danach deren Vermerke), und bis zur Korb-Marke
// steht alles in Bloecken (die Marke verschwindet nach dem Ausschalten des
// Korbs). Der Wert steigt nur und wird geschrieben, BEVOR die Information
// anderswo verschwindet -- sonst holte ein spaeterer Start (oder die alte
// Datei nach einer verlorenen Umbenennung) Zeilen fuer Verblocktes nach
// (Pruefung, 1. Durchgang, H-1 und M-3).
//
// HALTBARKEIT. Postgres darf nur Seqs kennen, die auf der Platte sind:
// serielle Speicherungen warten wie der Flush auf die Haltbarkeit
// (walHaltbarAbwarten), und nach dem Oeffnen vergibt das WAL keine Seq, die
// Postgres schon kennt (walSeqPostgresHoechste) -- sonst verdeckte eine
// wiederverwendete Seq einen Saldo oder einen Vermerk (M-1, M-2).

const walAusgangAbKey = "wal_ausgang_ab_seq"

const walAusgangErledigtKey = "wal_ausgang_erledigt_bis"

// walAusgangErledigtSQL hebt walAusgangErledigtKey auf $1, nie zurueck.
const walAusgangErledigtSQL = `INSERT INTO chain_config (key, value) VALUES ('` + walAusgangErledigtKey + `', $1::bigint::text)
ON CONFLICT (key) DO UPDATE SET value = GREATEST(chain_config.value::bigint, EXCLUDED.value::bigint)::text`

// walHaltbarFrist: so lange wartet eine serielle Speicherung hoechstens auf
// die Haltbarkeit -- dieselbe Frist wie der Flush (flushWALBatch).
const walHaltbarFrist = 2 * time.Second

// walAusgangVermerkSQL: $1 bigint[] der Saetze, deren Zeile in derselben
// Transaktion geschrieben wird.
const walAusgangVermerkSQL = `INSERT INTO wal_ausgang (seq) SELECT unnest($1::bigint[]) ON CONFLICT (seq) DO NOTHING`

// walAusgangHoechstens begrenzt die Vermerke, die der Wiederanlauf in den
// Speicher laedt (8 Byte je Satz). Gekuerzt wird mit dem WAL; mehr als das
// heisst, die Kuerzung laeuft nicht -- dann scheitert der Wiederanlauf
// (fail-closed) statt den Speicher zu sprengen.
var walAusgangHoechstens = 50_000_000

// configZahlLesen liest eine Zahl aus chain_config -- fail-closed:
// getConfigValueDB machte aus jedem Lesefehler "nicht gesetzt", und fuer
// Grenze, Untergrenze und Korb-Marke heisst das "alles nach der alten Regel"
// bzw. "nichts verworfen" (Pruefung, 1. Durchgang, M-4). Nur eine fehlende
// Zeile (oder ein leerer Wert) ist "nicht gesetzt".
func (cs *ChainState) configZahlLesen(key string) (n uint64, da bool, err error) {
	if cs.db == nil {
		return 0, false, nil
	}
	var v string
	err = cs.db.QueryRow(`SELECT value FROM chain_config WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("%s nicht lesbar: %w", key, err)
	}
	if v = strings.TrimSpace(v); v == "" {
		return 0, false, nil
	}
	if n, err = strconv.ParseUint(v, 10, 64); err != nil {
		return 0, false, fmt.Errorf("%s=%q nicht lesbar: %w", key, v, err)
	}
	return n, true, nil
}

// walAusgangAb liest die Uebergangsgrenze. da=false: noch nie gesetzt --
// dieser Wiederanlauf ist der erste mit Vermerken.
func (cs *ChainState) walAusgangAb() (ab uint64, da bool, err error) {
	return cs.configZahlLesen(walAusgangAbKey)
}

// walAusgangErledigt liest walAusgangErledigtKey (0: nichts erledigt).
func (cs *ChainState) walAusgangErledigt() (uint64, error) {
	n, _, err := cs.configZahlLesen(walAusgangErledigtKey)
	return n, err
}

// walHaltbarAbwarten: eine serielle Speicherung schreibt den Saldo aus dem
// Speicher samt WAL-Deltas bis seq. Ist seq noch nicht auf der Platte und
// stirbt der Prozess, fehlt der Satz im WAL, Postgres traegt aber die eine
// Seite (Geld verschwindet, M-2) und wal_seq = seq, die das WAL nach dem
// Neustart neu vergibt (M-1). Darum wie der Flush: warten, hoechstens
// walHaltbarFrist. Kommt die Haltbarkeit nicht (Sync defekt), gilt dieselbe
// Regel wie dort -- geschrieben wird trotzdem, der Schnellpfad ist dann zu,
// und walSeqPostgresHoechste verhindert die Wiederverwendung der Seq.
func (cs *ChainState) walHaltbarAbwarten(seq uint64) {
	w := cs.wal
	if w == nil || seq == 0 || w.DurableSeq() >= seq {
		return
	}
	if !w.WaitDurable(seq, walHaltbarFrist) {
		walFlushOhneHaltbarkeit.Add(1)
		fmt.Printf("[WAL] ⚠ serielle Speicherung schreibt einen Saldo bis Seq %d, obwohl das WAL ihn nicht als haltbar bestaetigt hat (Sync defekt oder zu langsam)\n", seq)
	}
}

// walSeqPostgresHoechste: die hoechste Seq, die Postgres kennt -- wal_seq
// eines Kontos, Vermerk, Untergrenze, "erledigt", Uebergangsgrenze,
// Korb-Marke. Das WAL vergibt nach dem Oeffnen nur darueber: eine Seq, die
// gespeichert, aber nie haltbar wurde, kaeme sonst ein zweites Mal vor (M-1),
// und eine neue Seq unter einer der Grenzen galte als ersetzt, erledigt oder
// verblockt (etwa nachdem das Dateiende verloren ging). Im Normalfall liegt
// alles unter dem Dateikopf, dann aendert sich nichts.
func (cs *ChainState) walSeqPostgresHoechste() (uint64, error) {
	if cs.db == nil {
		return 0, nil
	}
	var konten, vermerke int64
	if err := cs.db.QueryRow(`SELECT COALESCE(MAX(wal_seq), 0) FROM chain_accounts`).Scan(&konten); err != nil {
		return 0, fmt.Errorf("hoechste wal_seq nicht lesbar: %w", err)
	}
	if err := cs.db.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM wal_ausgang`).Scan(&vermerke); err != nil {
		return 0, fmt.Errorf("hoechster Vermerk nicht lesbar: %w", err)
	}
	hoechste := uint64(max(konten, vermerke, 0))
	for _, key := range []string{walRecoveryFloorKey, walAusgangErledigtKey, walAusgangAbKey, speicherKorbMarke} {
		n, _, err := cs.configZahlLesen(key)
		if err != nil {
			return 0, err
		}
		hoechste = max(hoechste, n)
	}
	return hoechste, nil
}

// walAusgangVermerke laedt die Vermerke ueber ab, aufsteigend.
func (cs *ChainState) walAusgangVermerke(ab uint64) (walAusgangMenge, error) {
	var n int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM wal_ausgang WHERE seq > $1`, int64(ab)).Scan(&n); err != nil {
		return nil, fmt.Errorf("wal_ausgang nicht lesbar: %w", err)
	}
	if n > walAusgangHoechstens {
		return nil, fmt.Errorf("wal_ausgang haelt %d Vermerke ueber %d (hoechstens %d) -- die WAL-Kuerzung laeuft offenbar nicht", n, ab, walAusgangHoechstens)
	}
	rows, err := cs.db.Query(`SELECT seq FROM wal_ausgang WHERE seq > $1 ORDER BY seq`, int64(ab))
	if err != nil {
		return nil, fmt.Errorf("wal_ausgang nicht lesbar: %w", err)
	}
	defer rows.Close()
	m := make(walAusgangMenge, 0, n)
	for rows.Next() {
		var s int64
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("wal_ausgang nicht lesbar: %w", err)
		}
		m = append(m, uint64(s))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("wal_ausgang nicht lesbar: %w", err)
	}
	return m, nil
}

// walAusgangMenge: aufsteigend sortierte Saetze mit Vermerk.
type walAusgangMenge []uint64

func (m walAusgangMenge) hat(seq uint64) bool {
	i := sort.Search(len(m), func(i int) bool { return m[i] >= seq })
	return i < len(m) && m[i] == seq
}

// walAusgangKuerzen loescht die Vermerke unter bis -- Saetze, die das WAL
// nicht mehr traegt (Kompaktierung). Vorher wird walAusgangErledigtKey auf
// bis-1 gehoben: liegt nach einer verlorenen Umbenennung die alte Datei
// wieder da, gelten ihre Saetze trotzdem als erledigt (M-3). Scheitert das,
// bleiben die Vermerke. In Stuecken, damit keine lange Sperre entsteht.
func (cs *ChainState) walAusgangKuerzen(bis uint64) error {
	if cs.db == nil || bis == 0 {
		return nil
	}
	if _, err := cs.db.Exec(walAusgangErledigtSQL, int64(bis-1)); err != nil {
		return fmt.Errorf("%s nicht auf %d gehoben, Vermerke bleiben: %w", walAusgangErledigtKey, bis-1, err)
	}
	const stueck = 50_000
	for {
		res, err := cs.db.Exec(`DELETE FROM wal_ausgang WHERE seq IN (SELECT seq FROM wal_ausgang WHERE seq < $1 ORDER BY seq LIMIT $2)`, int64(bis), stueck)
		if err != nil {
			return fmt.Errorf("wal_ausgang unter %d nicht gekuerzt: %w", bis, err)
		}
		if n, _ := res.RowsAffected(); n < stueck {
			return nil
		}
	}
}
