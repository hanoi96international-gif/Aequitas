package keeper

import (
	"encoding/binary"
	"fmt"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// Storage for THIS party's rows of the shared biometric templates.
//
// # WHAT IS STORED HERE, AND WHAT MUST NEVER BE
//
// One row per enrolment: a vector of uniformly random field elements that is
// this validator's additive share and nothing else. On its own it is noise —
// that is the entire security property, and it survives only as long as no
// process ever holds two parties' rows for the same enrolment. Nothing in this
// file may be extended to accept, fetch, or cache another party's row.
//
// # WHY THIS EXISTS AT ALL
//
// Without persistence the enrolment index lives in memory, and a restart
// silently empties it. The comparison then finds no candidates, reports
// "not a duplicate" for everybody, and registration continues looking perfectly
// healthy while the duplicate check has stopped working. Given how routinely
// these nodes restart, that is not a hypothetical.
//
// # WHY THE COMMITTEE ID IS PART OF THE ROW
//
// Shares belong to the committee that produced them, and they cannot be moved
// to another one: moving them would mean reconstructing the template. So an
// enrolment is only comparable by its own committee, and every row records
// which. A lookup that ignored it would hand a committee rows it holds no
// counterpart for, and the comparison would return arithmetic noise rather than
// an answer about a person.

// mpcSchema creates the tables. Called from the same place as the rest of the
// schema so schema_completeness_test can see it.
func (cs *ChainState) mpcSchema(dbExec func(string, ...interface{})) {
	// The row itself. enrollment_id is opaque here on purpose: this table must
	// not become a way to look up which wallet a biometric belongs to.
	dbExec(`CREATE TABLE IF NOT EXISTS mpc_shares (
enrollment_id TEXT PRIMARY KEY,
committee_id  TEXT NOT NULL,
party_index   INTEGER NOT NULL,
feature_count INTEGER NOT NULL,
row_data      BYTEA NOT NULL,
created_at    TIMESTAMP DEFAULT NOW()
)`)
	dbExec(`CREATE INDEX IF NOT EXISTS idx_mpc_shares_committee ON mpc_shares (committee_id)`)

	// KEIN LSH-Eimerindex mehr. Entfernt am 25.08.2026, und der Grund ist der
	// Zweck dieser ganzen Datei.
	//
	// Ein Eimerschluessel IST ein Stueck des Sketches: er besteht aus k Bits
	// davon, im Klartext. Bei 20 Tabellen à 27 Bit deckten die Schluessel
	// zusammen 338 der 512 Bits ab -- nachgerechnet, 66 %. Sie lagen in
	// derselben Datenbank wie der Anteil, der "allein nur Rauschen" sein soll.
	//
	// Damit war die Zusicherung dieser Bauart aufgehoben: eine einzelne Partei
	// konnte aus ihren eigenen zwei Tabellen zwei Drittel jedes eingeschriebenen
	// Sketches lesen. Wer ein Foto einer Person hat, haette damit pruefen
	// koennen, ob sie registriert ist -- genau die Preisgabe, die additive
	// Anteile verhindern sollen.
	//
	// Der Index war ausserdem NUTZLOS: die Kandidatensuche wurde am 24.08.2026
	// durch MPCAllShares ersetzt, weil ihre Trefferquote an der Schwelle bei
	// 0,008 % lag (20 Tabellen à 27 Bit, cos 0,40). Seitdem las ihn niemand
	// mehr -- geschrieben wurde er weiter.
	//
	// Zwei Drittel jedes Gesichtsmerkmals im Klartext, fuer einen Index, den
	// nichts benutzt.
	//
	// Vorhandene Zeilen werden geleert statt die Tabelle zu loeschen: ein DROP
	// waere gegenueber einer aelteren Node-Fassung, die noch schreibt, nicht
	// vertraeglich. Leer ist sie harmlos.
	dbExec(`DELETE FROM mpc_share_buckets`)
}

// encodeRow packs one party's row as big-endian uint64 per feature.
func encodeRow(row mpc.PartyTemplate) []byte {
	buf := make([]byte, 8*len(row))
	for i, v := range row {
		binary.BigEndian.PutUint64(buf[8*i:], uint64(v))
	}
	return buf
}

func decodeRow(buf []byte) (mpc.PartyTemplate, error) {
	if len(buf)%8 != 0 {
		return nil, fmt.Errorf("mpc: stored row of %d bytes is not a whole number of features", len(buf))
	}
	row := make(mpc.PartyTemplate, len(buf)/8)
	for i := range row {
		v := binary.BigEndian.Uint64(buf[8*i:])
		if v >= mpc.Prime {
			return nil, fmt.Errorf("mpc: stored feature %d is %d, outside the field", i, v)
		}
		row[i] = mpc.Element(v)
	}
	return row, nil
}

// SaveMPCShare stores this party's row for one enrolment, with its bucket keys.
//
// Both tables are written in one transaction: a share without its bucket keys
// is invisible to every future lookup, and bucket keys without a share point at
// nothing. Either half alone is a duplicate that will never be caught, so a
// partial write must not survive.
func (cs *ChainState) SaveMPCShare(enrollmentID, committeeID string, partyIndex int,
	row mpc.PartyTemplate) error {

	if cs.db == nil {
		return fmt.Errorf("mpc: no database configured; the enrolment index would be lost on restart")
	}
	if enrollmentID == "" || committeeID == "" {
		return fmt.Errorf("mpc: enrolment and committee ids are both required")
	}
	if len(row) == 0 {
		return fmt.Errorf("mpc: refusing to store an empty row for enrolment %q", enrollmentID)
	}
	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("mpc: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO mpc_shares (enrollment_id, committee_id, party_index, feature_count, row_data)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (enrollment_id) DO NOTHING`,
		enrollmentID, committeeID, partyIndex, len(row), encodeRow(row)); err != nil {
		return fmt.Errorf("mpc: storing share: %w", err)
	}

	// Alte Eimerschluessel dieser Einschreibung entfernen, falls die Tabelle
	// aus der Zeit vor dem 25.08.2026 noch existiert -- siehe mpcSchema.
	if _, err := tx.Exec(`DELETE FROM mpc_share_buckets WHERE enrollment_id = $1`, enrollmentID); err != nil {
		// Kein Abbruch: fehlt die Tabelle, ist nichts zu loeschen.
		_ = err
	}
	return tx.Commit()
}

// MPCAllShares returns every share this committee holds.
//
// # WHY THE BUCKET FILTER IS NOT USED FOR A DUPLICATE CHECK
//
// MPCCandidateShares narrows the field with an LSH pre-filter: 20 tables of 27
// bits each, and a candidate is only surfaced when one whole 27-bit key matches
// exactly. The probability of that for a genuine returning person at sketch
// distance d is
//
//	1 - (1 - (1 - d/512)^27)^20
//
// which is 100% for an identical capture, 62% at d = 55 -- and, measured on
// 2026-08-24 against the two real captures this project has:
//
//	d = 135 (same person, with glasses)   ->  0.5%
//	d = 165 (the match threshold itself)  ->  0.05%
//
// So the filter removes almost exactly the candidates the comparison exists to
// find. It is not a tuning problem: the threshold accepts a 32% bit difference,
// and any exact-match key of length k survives that only with probability
// 0.68^k. Short enough keys to keep the true match also match nearly everyone,
// which costs triples instead.
//
// A duplicate check therefore compares against everything. The cost is linear
// in the number of enrolments -- 2048 triples each -- which is affordable at
// the scale this runs at (100 enrolments need about 243 MB of triples per
// party in total) and is the honest price of a check that actually finds
// duplicates.
//
// Die Eimer werden seit dem 25.08.2026 GAR NICHT MEHR geschrieben. Der Satz,
// der hier stand -- sie kosteten fast nichts und ein spaeter passender Filter
// koenne sie nutzen -- war falsch: ein Eimerschluessel besteht aus k Bits des
// Sketches im Klartext, und 20 Tabellen à 27 Bit deckten 338 der 512 Bits ab.
// Sie kosteten also zwei Drittel der Vertraulichkeit, fuer die es diese
// Bauart ueberhaupt gibt. Siehe mpcSchema.
func (cs *ChainState) MPCAllShares(committeeID string, limit int) (
	ids []string, rows []mpc.PartyTemplate, err error) {

	if cs.db == nil {
		return nil, nil, fmt.Errorf("mpc: no database configured")
	}
	if limit <= 0 {
		limit = 5000
	}
	cur, err := cs.db.Query(
		`SELECT enrollment_id, row_data FROM mpc_shares WHERE committee_id = $1 LIMIT $2`,
		committeeID, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("mpc: share lookup: %w", err)
	}
	defer cur.Close()

	for cur.Next() {
		var id string
		var blob []byte
		if err := cur.Scan(&id, &blob); err != nil {
			return nil, nil, fmt.Errorf("mpc: reading share: %w", err)
		}
		row, err := decodeRow(blob)
		if err != nil {
			return nil, nil, fmt.Errorf("mpc: enrolment %s: %w", id, err)
		}
		ids = append(ids, id)
		rows = append(rows, row)
	}
	return ids, rows, cur.Err()
}

// DeleteMPCShare removes an enrolment from this party's store.
//
// Not optional. Biometric data that cannot be deleted is data a person can
// never withdraw, and an enrolment found to be wrong could otherwise refuse its
// subject forever. Deleting one party's row is enough to make the enrolment
// permanently uncomparable, which is the intended effect.
func (cs *ChainState) DeleteMPCShare(enrollmentID string) error {
	if cs.db == nil {
		return fmt.Errorf("mpc: no database configured")
	}
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM mpc_share_buckets WHERE enrollment_id = $1`, enrollmentID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM mpc_shares WHERE enrollment_id = $1`, enrollmentID); err != nil {
		return err
	}
	return tx.Commit()
}

// CountMPCShares reports how many enrolments this party holds, per committee.
//
// Worth surfacing: if this is zero while registrations are being accepted, the
// duplicate check is comparing against nothing and approving everyone — which
// looks exactly like a healthy system from the outside.
func (cs *ChainState) CountMPCShares() (map[string]int, error) {
	if cs.db == nil {
		return nil, fmt.Errorf("mpc: no database configured")
	}
	cur, err := cs.db.Query(`SELECT committee_id, COUNT(*) FROM mpc_shares GROUP BY committee_id`)
	if err != nil {
		return nil, err
	}
	defer cur.Close()
	out := map[string]int{}
	for cur.Next() {
		var id string
		var n int
		if err := cur.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, cur.Err()
}

// ReshareMPCShare schreibt den neuen Anteil einer Einschreibung und bindet sie
// zugleich an das neue Komitee.
//
// # WARUM BEIDES IN EINEM SCHRITT
//
// Die beiden Angaben gehoeren zusammen: ein neuer Anteil unter alter
// Komitee-Kennung wird von Parteien verglichen, die dazu keinen Gegenpart
// haben, und ein alter Anteil unter neuer Kennung ebenso. Beides ergibt
// Rauschen, und Rauschen beantwortet jede Frage mit "kein Duplikat" -- der
// Mensch waere geloescht, ohne dass es jemand bemerkt.
//
// Deshalb eine Transaktion und eine Bedingung: geschrieben wird nur, wenn die
// Zeile noch dem ALTEN Komitee gehoert. Ein zweiter Lauf derselben
// Neuteilung trifft dann null Zeilen und meldet das, statt einen bereits
// umgezogenen Anteil ein zweites Mal zu ueberschreiben.
func (cs *ChainState) ReshareMPCShare(enrollmentID, altesKomitee, neuesKomitee string,
	partyIndex int, row mpc.PartyTemplate) error {

	if cs.db == nil {
		return fmt.Errorf("mpc: keine Datenbank konfiguriert")
	}
	if enrollmentID == "" || altesKomitee == "" || neuesKomitee == "" {
		return fmt.Errorf("mpc: Einschreibung sowie altes und neues Komitee sind alle noetig")
	}
	if altesKomitee == neuesKomitee {
		return fmt.Errorf("mpc: altes und neues Komitee sind dasselbe (%q) -- eine Neuteilung "+
			"waere sinnlos und wuerde nur frische Zufaelligkeit verbrauchen", altesKomitee)
	}
	if len(row) == 0 {
		return fmt.Errorf("mpc: leerer Anteil fuer Einschreibung %q", enrollmentID)
	}

	res, err := cs.db.Exec(
		`UPDATE mpc_shares
		    SET committee_id = $1, party_index = $2, feature_count = $3, row_data = $4
		  WHERE enrollment_id = $5 AND committee_id = $6`,
		neuesKomitee, partyIndex, len(row), encodeRow(row), enrollmentID, altesKomitee)
	if err != nil {
		return fmt.Errorf("mpc: Neuteilung von %q: %w", enrollmentID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mpc: Neuteilung von %q, betroffene Zeilen unbekannt: %w", enrollmentID, err)
	}
	if n == 0 {
		return fmt.Errorf("mpc: Einschreibung %q gehoert nicht (mehr) dem Komitee %q -- "+
			"nichts geaendert. Entweder wurde sie bereits umgezogen, oder diese Partei "+
			"rechnet mit einem veralteten Komitee",
			enrollmentID, altesKomitee)
	}
	return nil
}

// MPCEnrollmentsOfCommittee nennt die Einschreibungen, die noch am alten
// Komitee haengen -- die Arbeitsliste einer Neuteilung.
func (cs *ChainState) MPCEnrollmentsOfCommittee(committeeID string) ([]string, error) {
	if cs.db == nil {
		return nil, fmt.Errorf("mpc: keine Datenbank konfiguriert")
	}
	rows, err := cs.db.Query(
		`SELECT enrollment_id FROM mpc_shares WHERE committee_id = $1 ORDER BY enrollment_id`,
		committeeID)
	if err != nil {
		return nil, fmt.Errorf("mpc: Einschreibungen von %q: %w", committeeID, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
