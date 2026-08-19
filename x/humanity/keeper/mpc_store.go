package keeper

import (
	"encoding/binary"
	"fmt"
	"strings"

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

	// The multi-table LSH index, one row per (enrolment, table). Separate from
	// the share so a candidate lookup is an index scan rather than a full scan
	// of every enrolment — at population scale the difference is the whole
	// design (see mpc/bucket.go).
	dbExec(`CREATE TABLE IF NOT EXISTS mpc_share_buckets (
enrollment_id TEXT NOT NULL,
table_index   INTEGER NOT NULL,
bucket_key    BIGINT NOT NULL
)`)
	dbExec(`CREATE INDEX IF NOT EXISTS idx_mpc_buckets_lookup ON mpc_share_buckets (table_index, bucket_key)`)
	dbExec(`CREATE INDEX IF NOT EXISTS idx_mpc_buckets_enrollment ON mpc_share_buckets (enrollment_id)`)
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
	row mpc.PartyTemplate, keys []mpc.BucketKey) error {

	if cs.db == nil {
		return fmt.Errorf("mpc: no database configured; the enrolment index would be lost on restart")
	}
	if enrollmentID == "" || committeeID == "" {
		return fmt.Errorf("mpc: enrolment and committee ids are both required")
	}
	if len(row) == 0 {
		return fmt.Errorf("mpc: refusing to store an empty row for enrolment %q", enrollmentID)
	}
	if len(keys) == 0 {
		return fmt.Errorf("mpc: enrolment %q has no bucket keys — it would be stored and then "+
			"never compared against anyone, which is indistinguishable from not storing it",
			enrollmentID)
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

	// Replace this enrolment's keys rather than appending, so a retried write
	// cannot leave an enrolment listed in a bucket twice and compared twice.
	if _, err := tx.Exec(`DELETE FROM mpc_share_buckets WHERE enrollment_id = $1`, enrollmentID); err != nil {
		return fmt.Errorf("mpc: clearing old bucket keys: %w", err)
	}
	for tbl, key := range keys {
		if _, err := tx.Exec(
			`INSERT INTO mpc_share_buckets (enrollment_id, table_index, bucket_key) VALUES ($1,$2,$3)`,
			enrollmentID, tbl, int64(key)); err != nil {
			return fmt.Errorf("mpc: storing bucket key for table %d: %w", tbl, err)
		}
	}
	return tx.Commit()
}

// MPCCandidateShares returns the rows of every enrolment sharing a bucket with
// these keys in at least one table, restricted to one committee.
//
// The committee restriction is not an optimisation. Rows from another committee
// have no counterpart on the peers convened here, so including them would
// produce arithmetic noise and call it a verdict about a person.
func (cs *ChainState) MPCCandidateShares(committeeID string, keys []mpc.BucketKey, limit int) (
	ids []string, rows []mpc.PartyTemplate, err error) {

	if cs.db == nil {
		return nil, nil, fmt.Errorf("mpc: no database configured")
	}
	if len(keys) == 0 {
		return nil, nil, nil
	}
	if limit <= 0 {
		limit = 5000
	}

	// One query over the union of the searched buckets. DISTINCT because an
	// enrolment matching in several tables is still one candidate, and
	// comparing it twice would waste triples and leak its multiplicity.
	var conds []string
	args := []interface{}{committeeID}
	for tbl, key := range keys {
		args = append(args, tbl, int64(key))
		conds = append(conds, fmt.Sprintf("(b.table_index = $%d AND b.bucket_key = $%d)",
			len(args)-1, len(args)))
	}
	args = append(args, limit)

	q := fmt.Sprintf(`SELECT DISTINCT s.enrollment_id, s.row_data
	                  FROM mpc_shares s
	                  JOIN mpc_share_buckets b ON b.enrollment_id = s.enrollment_id
	                  WHERE s.committee_id = $1 AND (%s)
	                  LIMIT $%d`, strings.Join(conds, " OR "), len(args))

	cur, err := cs.db.Query(q, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("mpc: candidate lookup: %w", err)
	}
	defer cur.Close()

	for cur.Next() {
		var id string
		var blob []byte
		if err := cur.Scan(&id, &blob); err != nil {
			return nil, nil, fmt.Errorf("mpc: reading candidate: %w", err)
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
