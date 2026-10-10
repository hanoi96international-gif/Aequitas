package keeper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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

const walAusgangAbKey = "wal_ausgang_ab_seq"

// walAusgangVermerkSQL: $1 bigint[] der Saetze, deren Zeile in derselben
// Transaktion geschrieben wird.
const walAusgangVermerkSQL = `INSERT INTO wal_ausgang (seq) SELECT unnest($1::bigint[]) ON CONFLICT (seq) DO NOTHING`

// walAusgangHoechstens begrenzt die Vermerke, die der Wiederanlauf in den
// Speicher laedt (8 Byte je Satz). Gekuerzt wird mit dem WAL; mehr als das
// heisst, die Kuerzung laeuft nicht -- dann scheitert der Wiederanlauf
// (fail-closed) statt den Speicher zu sprengen.
var walAusgangHoechstens = 50_000_000

// walAusgangAb liest die Uebergangsgrenze. da=false: noch nie gesetzt --
// dieser Wiederanlauf ist der erste mit Vermerken.
func (cs *ChainState) walAusgangAb() (ab uint64, da bool, err error) {
	v := strings.TrimSpace(cs.getConfigValueDB(walAusgangAbKey))
	if v == "" {
		return 0, false, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s=%q nicht lesbar: %w", walAusgangAbKey, v, err)
	}
	return n, true, nil
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
// nicht mehr traegt (Kompaktierung) oder die nie wieder abgespielt werden
// (Untergrenze). In Stuecken, damit keine lange Sperre entsteht.
func (cs *ChainState) walAusgangKuerzen(bis uint64) error {
	if cs.db == nil || bis == 0 {
		return nil
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
