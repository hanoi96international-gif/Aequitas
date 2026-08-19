package mpc

import (
	"fmt"
	"testing"
)

func candidateSet(n int) []Party {
	out := make([]Party, n)
	for i := 0; i < n; i++ {
		out[i] = Party{
			URL:     fmt.Sprintf("https://validator-%02d.example", i),
			Address: fmt.Sprintf("0x%040x", i+1),
		}
	}
	return out
}

// TestEveryNodeSelectsTheSameCommittee is the property that removes the manual
// config: no election, no coordinator, no MPC_PEERS edited by hand on every box.
func TestEveryNodeSelectsTheSameCommittee(t *testing.T) {
	candidates := candidateSet(12)

	first, err := SelectCommittee(candidates, 3, "block-hash-abc")
	if err != nil {
		t.Fatal(err)
	}
	// Another node sees the same validators in a different order — peer lists
	// arrive in whatever order the network produced them.
	shuffled := make([]Party, len(candidates))
	for i := range candidates {
		shuffled[i] = candidates[len(candidates)-1-i]
	}
	second, err := SelectCommittee(shuffled, 3, "block-hash-abc")
	if err != nil {
		t.Fatal(err)
	}

	if first.ID != second.ID {
		t.Fatalf("two nodes computed different committees (%s vs %s) from the same validator set — "+
			"they would each convene different parties and no comparison could complete",
			first.ID, second.ID)
	}
	for i := range first.Parties {
		if first.Parties[i].Address != second.Parties[i].Address {
			t.Fatalf("party %d differs: %s vs %s — party indices must agree or the shares are "+
				"combined in the wrong order", i, first.Parties[i].Address, second.Parties[i].Address)
		}
	}
}

// TestNewValidatorNeedsNoConfigChange is the user-visible answer: a validator
// joins, and existing nodes need no edit and no restart to know about it.
func TestNewValidatorNeedsNoConfigChange(t *testing.T) {
	before := candidateSet(5)
	after := candidateSet(6) // one more validator has appeared on chain

	c1, err := SelectCommittee(before, 3, "seed")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := SelectCommittee(after, 3, "seed")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("committee before: %v", c1.Addresses())
	t.Logf("committee after:  %v", c2.Addresses())

	// Whether the newcomer displaces someone is up to the hash, but the
	// committee must remain valid and the right size either way.
	if c2.Size() != 3 {
		t.Errorf("committee size is %d after a validator joined, want 3", c2.Size())
	}
}

// TestCommitteeIDChangesWithMembership: an enrolment records the ID of the
// committee holding its shares, so the ID must distinguish memberships.
func TestCommitteeIDChangesWithMembership(t *testing.T) {
	a, err := SelectCommittee(candidateSet(6), 3, "seed-one")
	if err != nil {
		t.Fatal(err)
	}
	b, err := SelectCommittee(candidateSet(6), 3, "seed-two")
	if err != nil {
		t.Fatal(err)
	}
	sameMembers := true
	for i := range a.Parties {
		if a.Parties[i].Address != b.Parties[i].Address {
			sameMembers = false
		}
	}
	if sameMembers && a.ID != b.ID {
		t.Error("identical membership produced different IDs — enrolments would be orphaned")
	}
	if !sameMembers && a.ID == b.ID {
		t.Error("different membership produced the same ID — a comparison would convene the " +
			"wrong parties, who do not hold the shares")
	}
}

// TestCommitteeIDIgnoresHostnames: a party that moves is the same party holding
// the same shares. An ID tied to its URL would orphan every enrolment it holds.
func TestCommitteeIDIgnoresHostnames(t *testing.T) {
	orig := candidateSet(4)
	moved := candidateSet(4)
	for i := range moved {
		moved[i].URL = fmt.Sprintf("https://relocated-%02d.example", i)
	}

	a, err := SelectCommittee(orig, 3, "seed")
	if err != nil {
		t.Fatal(err)
	}
	b, err := SelectCommittee(moved, 3, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Error("moving a validator to a new hostname changed the committee ID, which would " +
			"orphan every enrolment that committee holds — the shares did not move")
	}
}

// TestCommitteeSizeIsBounded encodes the measurement: traffic grows with
// n(n-1), so an unbounded committee makes registration unaffordable and
// requires everyone online at once.
func TestCommitteeSizeIsBounded(t *testing.T) {
	if _, err := SelectCommittee(candidateSet(50), 1, "s"); err == nil {
		t.Error("a one-party committee was accepted — that party holds every share")
	}
	if _, err := SelectCommittee(candidateSet(50), 50, "s"); err == nil {
		t.Error("a 50-party committee was accepted; one registration would cost tens of " +
			"gigabytes and need all 50 online simultaneously")
	}
	if _, err := SelectCommittee(candidateSet(2), 3, "s"); err == nil {
		t.Error("a committee larger than the candidate pool was accepted")
	}
}

// TestTrafficProjectionIsQuadratic makes the cost of "just add everyone"
// concrete, since that is the intuition this design has to argue against.
func TestTrafficProjectionIsQuadratic(t *testing.T) {
	const twoPartyMB = 37.5
	for _, size := range []int{2, 3, 5, 7} {
		c, err := SelectCommittee(candidateSet(10), size, "seed")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%d parties: ~%.0f MB per registration", size, c.EstimatedTrafficMB(twoPartyMB))
	}
	c, _ := SelectCommittee(candidateSet(10), 2, "seed")
	if got := c.EstimatedTrafficMB(twoPartyMB); got != twoPartyMB {
		t.Errorf("two parties projected as %.1f MB, want the measured %.1f", got, twoPartyMB)
	}
	c4, _ := SelectCommittee(candidateSet(10), 4, "seed")
	if got := c4.EstimatedTrafficMB(twoPartyMB); got != 6*twoPartyMB {
		t.Errorf("four parties projected as %.1f MB, want 6x the two-party figure (measured "+
			"4.0/12.2/24.4 KB for 2/3/4 parties)", got)
	}
}

// TestNodeFindsItsOwnIndex: most validators are not members, and that must be
// an ordinary answer rather than an error.
func TestNodeFindsItsOwnIndex(t *testing.T) {
	c, err := SelectCommittee(candidateSet(8), 3, "seed")
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range c.Parties {
		if got := c.IndexOf(p.Address); got != i {
			t.Errorf("IndexOf(%s) = %d, want %d", p.Address, got, i)
		}
		// Case must not matter: addresses arrive checksummed from some sources.
		if got := c.IndexOf("0X" + p.Address[2:]); got != i {
			t.Error("IndexOf is case-sensitive; a checksummed address was not recognised")
		}
	}
	if got := c.IndexOf("0xdeadbeef00000000000000000000000000000000"); got != -1 {
		t.Errorf("a non-member got index %d, want -1", got)
	}
}

// TestCandidatesWithoutAnEndpointAreSkipped: a validator that does not
// advertise an MPC endpoint cannot take part, and must not be silently chosen.
func TestCandidatesWithoutAnEndpointAreSkipped(t *testing.T) {
	candidates := candidateSet(5)
	candidates[1].URL = ""
	candidates[3].URL = "   "

	c, err := SelectCommittee(candidates, 3, "seed")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range c.Parties {
		if p.URL == "" {
			t.Fatal("a validator with no endpoint was selected; the committee could never convene")
		}
		if p.Address == candidates[1].Address || p.Address == candidates[3].Address {
			t.Errorf("validator %s has no endpoint but was selected", p.Address)
		}
	}
}

func TestDuplicateAddressIsRefused(t *testing.T) {
	candidates := candidateSet(4)
	candidates[2].Address = candidates[0].Address
	if _, err := SelectCommittee(candidates, 2, "seed"); err == nil {
		t.Error("two candidates with the same signing address were accepted — one key holding " +
			"two shares defeats the split")
	}
}
