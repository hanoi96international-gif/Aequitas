package mpc

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/crypto/sha3"
)

// Committee selection: which validators actually hold the shares.
//
// # WHY THIS IS NOT "EVERY VALIDATOR"
//
// The obvious reading of "decentralised" is that every validator becomes an MPC
// party. Measured, that does not work, and the numbers are not close.
//
// Traffic grows with the number of ORDERED PAIRS of parties, n(n-1), because
// every party publishes each round to every other. Measured on one identical
// comparison: 2 parties 4.0 KB, 3 parties 12.2 KB, 4 parties 24.4 KB — exactly
// 1x, 3x, 6x. Applied to a real registration, which costs 37.5 MB between two
// parties:
//
//	 2 parties     37.5 MB
//	 3 parties    112 MB
//	 5 parties    375 MB
//	10 parties    1.7 GB
//	50 parties     46 GB
//
// Availability moves the same way and worse. Additive sharing is all-or-
// nothing: EVERY party in a committee must answer or the comparison cannot
// finish. With independent operators at 99% uptime, 2 parties are available
// 98% of the time and 50 parties 61% of the time. A larger committee is a
// system that is more often unable to register anyone.
//
// So the committee is a small, fixed-size subset of the validator set. Growing
// the validator set makes committee membership more decentralised — more
// independent operators eligible, chosen without anyone's say-so — without
// making every registration more expensive.
//
// # HOW MEMBERSHIP IS DECIDED WITHOUT A COORDINATOR
//
// Deterministically, from data every node already has. Candidates are sorted by
// a hash of (seed, address); the first `size` win. Every node computes the same
// committee from the same validator set, with no election, no coordinator, and
// nothing to configure by hand.
//
// # THE CONSTRAINT THAT MAKES THIS SUBTLE
//
// An enrolment's shares belong to the committee that created them. If the
// committee later changes, those shares do not move — nobody can move them,
// because moving them would mean reconstructing the template, which is the one
// thing that must never happen.
//
// So every enrolment records its Committee.ID, and a comparison against it must
// convene THAT committee. Committees are therefore append-only history, not a
// single current value, and a committee whose members are permanently gone
// takes its enrolments with it: those people can register again undetected.
// That is the cost of never being able to reconstruct a template, and it is why
// committee members should be long-lived validators and why committees should
// change rarely.

// Party is one committee member: where to reach it and which key it signs with.
type Party struct {
	URL     string
	Address string // signing address, lower-case hex, as bound by Node Binding
}

// Committee is an ordered set of parties that jointly hold a set of templates.
type Committee struct {
	// ID identifies this exact membership. Recorded with every enrolment the
	// committee holds, so a later comparison convenes the right parties even
	// after the validator set has moved on.
	ID string

	// Parties in index order. The index is the party number in the protocol,
	// so this ordering must be stable for the committee's whole life.
	Parties []Party
}

// MinCommitteeSize is two, because one party holds every share.
const MinCommitteeSize = 2

// MaxCommitteeSize bounds the damage a misconfiguration can do. Beyond this,
// one registration costs gigabytes and needs every member online at once; see
// the table above.
const MaxCommitteeSize = 7

// SelectCommittee picks a committee from the eligible validators, the same way
// on every node.
//
// seed makes the choice unpredictable in advance without making it disagree:
// use a value every node already agrees on and nobody chooses freely, such as a
// finalised block hash. A constant seed is allowed and simply makes membership
// a pure function of the validator set.
func SelectCommittee(candidates []Party, size int, seed string) (*Committee, error) {
	if size < MinCommitteeSize {
		return nil, fmt.Errorf("mpc: committee size %d is below %d — with fewer than two parties "+
			"one machine holds every share and can reconstruct every template",
			size, MinCommitteeSize)
	}
	if size > MaxCommitteeSize {
		return nil, fmt.Errorf("mpc: committee size %d exceeds %d — traffic grows with n(n-1), so "+
			"a registration would cost roughly %.0f MB, and every member would have to be online "+
			"for any registration to complete at all",
			size, MaxCommitteeSize, 37.5*float64(size*(size-1))/2)
	}

	seen := map[string]bool{}
	valid := make([]Party, 0, len(candidates))
	for _, c := range candidates {
		addr := strings.ToLower(strings.TrimSpace(c.Address))
		url := strings.TrimRight(strings.TrimSpace(c.URL), "/")
		if addr == "" || url == "" {
			continue // not advertising an endpoint; cannot take part
		}
		if seen[addr] {
			return nil, fmt.Errorf("mpc: address %s appears twice among the candidates — one key "+
				"holding two shares defeats the split entirely", addr)
		}
		seen[addr] = true
		valid = append(valid, Party{URL: url, Address: addr})
	}
	if len(valid) < size {
		return nil, fmt.Errorf("mpc: %d validators advertise an MPC endpoint, need %d for a "+
			"committee of that size", len(valid), size)
	}

	// Deterministic order: hash of (seed, address). Every node sorts the same
	// way from the same inputs, so the committee needs no election.
	type scored struct {
		p     Party
		score [32]byte
	}
	ranked := make([]scored, len(valid))
	for i, p := range valid {
		h := sha3.New256()
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(seed)))
		h.Write(n[:])
		h.Write([]byte(seed))
		h.Write([]byte(p.Address))
		copy(ranked[i].score[:], h.Sum(nil))
		ranked[i].p = p
	}
	sort.Slice(ranked, func(i, j int) bool {
		if c := compareBytes(ranked[i].score[:], ranked[j].score[:]); c != 0 {
			return c < 0
		}
		// Ties cannot happen with a 256-bit hash, but a deterministic
		// tiebreaker costs nothing and removes the possibility of two nodes
		// disagreeing.
		return ranked[i].p.Address < ranked[j].p.Address
	})

	chosen := make([]Party, size)
	for i := 0; i < size; i++ {
		chosen[i] = ranked[i].p
	}
	// Index order is by address, not by score: stable, and independent of the
	// seed, so a committee's party numbering never depends on how it was drawn.
	sort.Slice(chosen, func(i, j int) bool { return chosen[i].Address < chosen[j].Address })

	return &Committee{ID: committeeID(chosen), Parties: chosen}, nil
}

func compareBytes(a, b []byte) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// committeeID hashes the ordered membership.
//
// Membership only — not the URLs. A party that moves to a new address is the
// same party holding the same shares, and an ID that changed with its hostname
// would orphan every enrolment it holds.
func committeeID(parties []Party) string {
	h := sha3.New256()
	h.Write([]byte("aequitas-mpc-committee-v1"))
	for _, p := range parties {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(p.Address)))
		h.Write(n[:])
		h.Write([]byte(p.Address))
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// IndexOf returns this node's party number, or -1 if it is not a member.
//
// Not being a member is the ordinary case for most validators and must not be
// treated as an error: a node simply does not take part in matching.
func (c *Committee) IndexOf(address string) int {
	want := strings.ToLower(strings.TrimSpace(address))
	for i, p := range c.Parties {
		if p.Address == want {
			return i
		}
	}
	return -1
}

// URLs returns the peer base URLs in party-index order.
func (c *Committee) URLs() []string {
	out := make([]string, len(c.Parties))
	for i, p := range c.Parties {
		out[i] = p.URL
	}
	return out
}

// Addresses returns the signing addresses in party-index order.
func (c *Committee) Addresses() []string {
	out := make([]string, len(c.Parties))
	for i, p := range c.Parties {
		out[i] = p.Address
	}
	return out
}

// Size is the number of parties.
func (c *Committee) Size() int { return len(c.Parties) }

// EstimatedTrafficMB projects one registration's traffic for this committee,
// scaling the measured two-party figure by the number of ordered pairs.
//
// Worth logging when a committee is formed: it is the number that decides
// whether registrations remain affordable, and it grows quadratically, which is
// not the direction people expect when they add "just one more" validator.
func (c *Committee) EstimatedTrafficMB(twoPartyMB float64) float64 {
	n := len(c.Parties)
	if n < 2 {
		return 0
	}
	return twoPartyMB * float64(n*(n-1)) / 2
}
