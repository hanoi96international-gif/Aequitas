package keeper

import (
	"fmt"
	"os"
	"testing"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// TestRowEncodingRoundTrips: the stored bytes ARE the share. A silent
// corruption here does not produce an error, it produces a wrong distance and
// therefore a wrong answer about whether someone is a duplicate.
func TestRowEncodingRoundTrips(t *testing.T) {
	row := mpc.PartyTemplate{0, 1, 2, mpc.Element(mpc.Prime - 1), 987654321}
	got, err := decodeRow(encodeRow(row))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(row) {
		t.Fatalf("got %d features, want %d", len(got), len(row))
	}
	for i := range row {
		if got[i] != row[i] {
			t.Errorf("feature %d: got %d, want %d", i, got[i], row[i])
		}
	}
}

// TestCorruptStoredRowIsRejected: a value outside the field cannot be a share,
// and using it would silently corrupt every comparison it takes part in.
func TestCorruptStoredRowIsRejected(t *testing.T) {
	bad := make([]byte, 8)
	for i := range bad {
		bad[i] = 0xFF
	}
	if _, err := decodeRow(bad); err == nil {
		t.Error("a stored value outside the field was accepted")
	}
	if _, err := decodeRow([]byte{1, 2, 3}); err == nil {
		t.Error("a truncated row was accepted")
	}
}

// TestSaveMPCShareRefusesUnusableInput: every rejection here is a way an
// enrolment could be stored and then never compared against anyone — which
// looks exactly like a working system while duplicates walk through.
func TestSaveMPCShareRefusesUnusableInput(t *testing.T) {
	cs := &ChainState{} // no db: the first guard should fire before anything else
	if err := cs.SaveMPCShare("e1", "c1", 0, mpc.PartyTemplate{1, 2}, []mpc.BucketKey{7}); err == nil {
		t.Error("a share was accepted with no database — the index would vanish on restart")
	}
}

// mpcTestState opens a real database, or skips.
func mpcTestState(t *testing.T) *ChainState {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("needs DATABASE_URL: this exercises SQL, which no in-memory fake would test")
	}
	cs := NewChainState("unused-mpc-store-test.json")
	if !cs.useDB || cs.db == nil {
		t.Skip("no live PostgreSQL connection")
	}
	cs.mpcSchema(func(q string, args ...interface{}) {
		if _, err := cs.db.Exec(q, args...); err != nil {
			t.Fatalf("schema: %v", err)
		}
	})
	return cs
}

// TestSharesSurviveAndAreFoundByBucket is the property the whole store exists
// for: an enrolment written now must still be found as a candidate later, by
// bucket, after the process that wrote it is gone.
func TestSharesSurviveAndAreFoundByBucket(t *testing.T) {
	cs := mpcTestState(t)

	prefix := fmt.Sprintf("test-%d", os.Getpid())
	committee := prefix + "-committee"
	id := prefix + "-enrolment"
	t.Cleanup(func() { _ = cs.DeleteMPCShare(id) })

	row := mpc.PartyTemplate{11, 22, 33, 44}
	keys := []mpc.BucketKey{101, 202, 303}
	if err := cs.SaveMPCShare(id, committee, 0, row, keys); err != nil {
		t.Fatal(err)
	}

	// A candidate matching in ONE table must be found: that is what multi-table
	// LSH buys, and a lookup requiring all tables would defeat it.
	ids, rows, err := cs.MPCCandidateShares(committee, []mpc.BucketKey{999, 202, 999}, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := -1
	for i, got := range ids {
		if got == id {
			found = i
		}
	}
	if found < 0 {
		t.Fatalf("the enrolment was not found by its second-table bucket key; every duplicate "+
			"it should catch would walk through. got ids=%v", ids)
	}
	for i := range row {
		if rows[found][i] != row[i] {
			t.Fatalf("feature %d came back as %d, want %d — the stored share is not the one "+
				"written", i, rows[found][i], row[i])
		}
	}
}

// TestCandidatesAreScopedToTheirCommittee: rows from another committee have no
// counterpart on the peers convened here, so returning them would produce
// arithmetic noise and present it as a verdict about a person.
func TestCandidatesAreScopedToTheirCommittee(t *testing.T) {
	cs := mpcTestState(t)

	prefix := fmt.Sprintf("test-%d-scope", os.Getpid())
	mine, theirs := prefix+"-mine", prefix+"-theirs"
	t.Cleanup(func() { _ = cs.DeleteMPCShare(mine); _ = cs.DeleteMPCShare(theirs) })

	key := []mpc.BucketKey{4242}
	if err := cs.SaveMPCShare(mine, prefix+"-A", 0, mpc.PartyTemplate{1, 2}, key); err != nil {
		t.Fatal(err)
	}
	if err := cs.SaveMPCShare(theirs, prefix+"-B", 0, mpc.PartyTemplate{3, 4}, key); err != nil {
		t.Fatal(err)
	}

	ids, _, err := cs.MPCCandidateShares(prefix+"-A", key, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range ids {
		if got == theirs {
			t.Error("an enrolment from another committee was returned as a candidate; the " +
				"comparison would combine shares that do not belong together")
		}
	}
}

// TestDeletionIsHonoured: biometric data that cannot be deleted is data a
// person can never withdraw.
func TestDeletionIsHonoured(t *testing.T) {
	cs := mpcTestState(t)

	prefix := fmt.Sprintf("test-%d-delete", os.Getpid())
	id := prefix + "-enrolment"
	key := []mpc.BucketKey{7777}
	if err := cs.SaveMPCShare(id, prefix+"-c", 0, mpc.PartyTemplate{5, 6}, key); err != nil {
		t.Fatal(err)
	}
	if err := cs.DeleteMPCShare(id); err != nil {
		t.Fatal(err)
	}

	ids, _, err := cs.MPCCandidateShares(prefix+"-c", key, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range ids {
		if got == id {
			t.Fatal("a deleted enrolment is still returned as a candidate — the share is gone " +
				"but the index still points at it")
		}
	}

	// And the bucket rows must be gone too, or the index grows forever with
	// entries pointing at nothing.
	var orphans int
	if err := cs.db.QueryRow(
		`SELECT COUNT(*) FROM mpc_share_buckets WHERE enrollment_id = $1`, id).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Errorf("%d bucket rows survived the deletion", orphans)
	}
}

// TestRewritingAnEnrolmentDoesNotDuplicateItsBuckets: a retried write must not
// leave the enrolment listed twice, or it is compared twice — wasting triples
// and revealing its multiplicity.
func TestRewritingAnEnrolmentDoesNotDuplicateItsBuckets(t *testing.T) {
	cs := mpcTestState(t)

	prefix := fmt.Sprintf("test-%d-rewrite", os.Getpid())
	id := prefix + "-enrolment"
	t.Cleanup(func() { _ = cs.DeleteMPCShare(id) })

	keys := []mpc.BucketKey{1, 2, 3}
	for i := 0; i < 3; i++ {
		if err := cs.SaveMPCShare(id, prefix+"-c", 0, mpc.PartyTemplate{9, 9}, keys); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := cs.db.QueryRow(
		`SELECT COUNT(*) FROM mpc_share_buckets WHERE enrollment_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(keys) {
		t.Errorf("after three writes the enrolment has %d bucket rows, want %d", n, len(keys))
	}

	ids, _, err := cs.MPCCandidateShares(prefix+"-c", keys, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, got := range ids {
		if got == id {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("the enrolment was returned %d times as a candidate, want once", seen)
	}
}
