package keeper

import "testing"

// The producer must not build blocks the network cannot replay.
//
// MEASURED 2026-08-21 on the live boxes. Contabo2 produced block #4391949 with
// exactly 50,000 transfers -- the cap at the time. Contabo1 needed ~4.7s to
// replay it while holding the exclusive state lock, against BLOCK_TIME=1s. It
// fell behind by ~4.7x per such block, stopped merging, orphaned everything
// arriving afterwards, and could not recover without operator intervention.
//
// The trap is that a full block looks like a throughput achievement. It is the
// opposite: it means production had already collapsed (0.43 blocks/s under
// load), a backlog accumulated, and it then went out in one lump that pushed
// the next node further behind. The circle only turns one way.
//
// So the cap belongs to the SLOWEST REPLAYING node, not to the producer. These
// constants encode that relationship; changing one without the other is what
// this test exists to stop.

// measuredReplayPerSecond is what one node actually replayed, from the
// incident above: 50,000 transfers in 4.7 seconds.
//
// Deliberately conservative in two ways. It came from a node that was ALSO
// thrashing on orphans, so a healthy node is likely faster; and it is a single
// observation. Re-measure before treating it as a budget to spend rather than
// a bound to respect.
const measuredReplayPerSecond = 10600

func TestBlockCapStaysWithinReplayCapacity(t *testing.T) {
	// One block must replay inside one block time, or the replaying node falls
	// behind by construction and every later block arrives to a node already
	// losing ground.
	const blockTimeSeconds = 1

	budget := measuredReplayPerSecond * blockTimeSeconds
	if maxTxsPerBlock > budget {
		t.Errorf("maxTxsPerBlock is %d but a replaying node absorbs about %d per block time.\n"+
			"  A block above that budget cannot be replayed within BLOCK_TIME, so the replaying\n"+
			"  node falls behind on every one -- and once behind it stops merging, orphans what\n"+
			"  arrives, and does not recover on its own. That is not a throughput ceiling being\n"+
			"  approached, it is a network that stops converging.\n"+
			"  Raising this needs a NEW replay measurement, not an estimate.",
			maxTxsPerBlock, budget)
	}

	// The other direction matters too: a cap far below capacity throttles the
	// chain for no reason. Flagged, not failed -- being conservative is a
	// legitimate choice, but it should be a deliberate one.
	if maxTxsPerBlock < budget/4 {
		t.Logf("note: maxTxsPerBlock (%d) is well under the measured replay budget (%d). "+
			"If that is deliberate caution, fine; if it is left over from an incident, there is "+
			"throughput being given away.", maxTxsPerBlock, budget)
	}
}

// Multi-block tick keeps producing while each block comes back full, so a
// single tick can emit maxExtraBlocksPerTick+1 full blocks. Replay cost is per
// TRANSACTION, not per block, so splitting the same work across more blocks
// does not make it cheaper -- it only makes each lock hold shorter. A tick that
// emits more transactions than a replayer absorbs in that tick is the same
// overshoot the block cap was lowered to prevent, just relocated.
func TestOneTickCannotOutrunReplay(t *testing.T) {
	const blockTimeSeconds = 1
	perTick := maxTxsPerBlock * (maxExtraBlocksPerTick + 1)
	budget := measuredReplayPerSecond * blockTimeSeconds

	if perTick > budget {
		t.Logf("a full tick can emit %d transactions (maxTxsPerBlock=%d x %d blocks) against a "+
			"replay budget of about %d.\n"+
			"  This is only reachable while a backlog exists -- ProduceBlocksForTick produces "+
			"another block only when the previous one came back full -- but a backlog is exactly "+
			"when the replayer is already behind, so draining it at %dx capacity pushes it "+
			"further behind rather than catching it up.\n"+
			"  ENABLE_MULTI_BLOCK_TICK is a per-node operator flag; leaving it OFF keeps one "+
			"tick within the replay budget.",
			perTick, maxTxsPerBlock, maxExtraBlocksPerTick+1, budget, perTick/budget)
	}
}
