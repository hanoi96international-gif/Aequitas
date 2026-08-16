package keeper

import "testing"

// The daily round — UBI, validator, LP, escrow — must apply exactly once per
// day, network-wide, regardless of which node's block a peer sees it in
// first. distributionRoundToSkip and isDistributionRoundTxType are the two
// pure decisions that guarantee that; see their own doc comments for the two
// real, measured bugs this closes:
//
//   - a round that paid validators/LP while the UBI pool was empty (the
//     everyday case — measured live, pool_ubi 0.0000) emitted no anchor at
//     all, so a duplicate delivery of it replayed in full;
//   - escrow_move/escrow_release were never in the skip list, so they
//     re-applied even when the round around them was correctly detected as
//     a duplicate.
//
// These are extracted, side-effect-free functions specifically so they can be
// tested directly — the mechanism they implement reads two DB config values
// that are unavailable in every no-DB unit test in this package, so it had
// never actually been exercised by a test before this file.

func ubiRoundTxs(at int64, perHuman float64, wallets ...string) []Transaction {
	txs := make([]Transaction, 0, len(wallets)+1)
	for _, w := range wallets {
		txs = append(txs, Transaction{Type: "ubi_distribution", Wallet: w, Amount: perHuman})
	}
	txs = append(txs, Transaction{Type: "ubi_distribution_finalize", DistributionAt: at})
	txs = append(txs, Transaction{Type: "distribution_round_marker", DistributionAt: at})
	return txs
}

func TestDistributionRoundToSkip_DuplicateUBIRoundIsDetected(t *testing.T) {
	const at = int64(1_700_000_002) // 2s after the "first" node's round
	txs := ubiRoundTxs(at, 50, "0xa", "0xb")

	// This node already recorded the SAME calendar round via both anchors.
	skip := distributionRoundToSkip(txs, at-2, at-2)
	if skip != at {
		t.Fatalf("distributionRoundToSkip = %d, want %d (the round must be flagged as a duplicate)", skip, at)
	}
}

func TestDistributionRoundToSkip_EmptyUBIPoolRoundStillHasAnAnchor(t *testing.T) {
	// The bug: a round that pays validators/LP with an empty UBI pool emits
	// NO ubi_distribution_finalize (state.go only appends it `if ubiTotal >
	// 0`) — but it always emits distribution_round_marker, unconditionally,
	// whenever the round produced any transaction at all.
	const at = int64(1_700_000_050)
	txs := []Transaction{
		{Type: "validator_distribution", Wallet: "0xv1", Amount: 30},
		{Type: "validator_distribution_pool_zero"},
		{Type: "lp_distribution", Wallet: "0xlp1", Amount: 60},
		{Type: "lp_distribution_pool_zero"},
		{Type: "distribution_round_marker", DistributionAt: at},
		// deliberately NO ubi_distribution_finalize
	}

	// last_ubi_at is stale (UBI hasn't paid in weeks) but
	// last_distribution_round_at was stamped by this exact round already.
	skip := distributionRoundToSkip(txs, at-5, 0 /* last_ubi_at never touched */)
	if skip != at {
		t.Fatalf("distributionRoundToSkip = %d, want %d\n"+
			"  a validator/LP-only round must still be detectable as already-applied", skip, at)
	}
}

func TestDistributionRoundToSkip_NewRoundIsNotSkipped(t *testing.T) {
	// The other failure direction matters just as much: a GENUINELY new
	// round — first time this node has seen it — must never be skipped, or
	// nobody ever gets paid.
	const at = int64(1_700_100_000)
	txs := ubiRoundTxs(at, 50, "0xa", "0xb")

	if skip := distributionRoundToSkip(txs, 0, 0); skip != 0 {
		t.Errorf("first-ever round: distributionRoundToSkip = %d, want 0 (never applied before)", skip)
	}
	if skip := distributionRoundToSkip(txs, at-30*24*3600, at-30*24*3600); skip != 0 {
		t.Errorf("round 30 days after the last one: distributionRoundToSkip = %d, want 0 (a new day, not a duplicate)", skip)
	}
}

func TestDistributionRoundToSkip_CrossesMidnightSecondsApartStillMatches(t *testing.T) {
	// The exact scenario the 24h-window design targets: two nodes firing the
	// same calendar round a couple of seconds apart.
	skip := distributionRoundToSkip(
		[]Transaction{{Type: "distribution_round_marker", DistributionAt: 1_700_000_003}},
		1_700_000_001, 0)
	if skip != 1_700_000_003 {
		t.Errorf("a 2-second gap within the same round must still match, got skip=%d", skip)
	}
}

func TestIsDistributionRoundTxType_CoversEveryRoundTxIncludingEscrow(t *testing.T) {
	mustSkip := []string{
		"ubi_distribution", "ubi_distribution_finalize",
		"validator_distribution", "validator_distribution_pool_zero",
		"lp_distribution", "lp_distribution_pool_zero",
		"escrow_move", "escrow_release", // the exact bug: these were missing
		"distribution_round_marker",
	}
	for _, ty := range mustSkip {
		if !isDistributionRoundTxType(ty) {
			t.Errorf("%q must be treated as a distribution-round TX type", ty)
		}
	}
}

func TestIsDistributionRoundTxType_OrdinaryTxTypesAreUnaffected(t *testing.T) {
	// A duplicate-round skip must never accidentally swallow ordinary money
	// movement — that would be its own, opposite bug (money that should have
	// moved, silently didn't).
	for _, ty := range []string{"transfer", "register_human", "swap_aeq_tusd", "faucet", "escrow_recover", "slash_equivocation"} {
		if isDistributionRoundTxType(ty) {
			t.Errorf("%q must NOT be treated as a distribution-round TX type", ty)
		}
	}
}


// IsBlockReplayedInDB is the durable backstop for the in-memory dag.replayedBlocks
// cache, which is wiped wholesale every ~50,000 entries (~14h at BLOCK_TIME=1s).
// Without a database it must report false — never true, which would let a
// caller treat an unverifiable claim as "already applied" and silently skip a
// block that genuinely needs replaying.
func TestIsBlockReplayedInDB_NoDatabaseReportsFalse(t *testing.T) {
	cs := newTestState()
	if cs.IsBlockReplayedInDB("0xanything") {
		t.Fatal("without a database this must report false, never a confident true")
	}
}
