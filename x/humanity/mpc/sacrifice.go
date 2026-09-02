package mpc

import (
	"context"
	"fmt"
)

// Triple verification, so a dishonest or simply buggy dealer cannot decide who
// counts as a duplicate.
//
// # THE ATTACK THIS CLOSES
//
// GenerateTriples is a trusted dealer. Until now the parties took its output on
// faith, and a triple with c != a*b makes the multiplication that consumes it
// produce a wrong value — silently, with no error anywhere. Feed the right
// wrong triples and the final threshold bit flips: a person who is already
// registered is declared new, and registers again. Nothing in the protocol
// notices, and nothing in the logs looks unusual.
//
// That is a Sybil attack executed entirely from the offline phase, and it does
// not require seeing a single template.
//
// # WHAT SACRIFICING DOES
//
// A triple is checked by destroying a second one. With a public random r, the
// parties open two blinded values and verify one identity:
//
//	rho   = r*a - a'
//	sigma = b - b'
//	r*c - c' - sigma*a' - rho*b' - rho*sigma  ==  0
//
// which holds exactly when c = a*b, given c' = a'*b'. The sacrificed triple is
// fresh and uniformly random, so rho and sigma reveal nothing about a, b or c —
// the checked triple stays usable afterwards.
//
// # WHAT IT DOES NOT DO
//
// It does not make the dealer harmless. A dealer that knows a and b, AND can
// observe the openings of a multiplication that consumes the triple, recovers
// that multiplication's inputs — the templates. Verification cannot help with
// that, because the triples are perfectly valid. Only keeping the dealer off
// the wire does, which is a deployment property (see DEPLOY.md), not something
// this file can enforce.
//
// So: verification reduces dealer trust from "can silently decide who is a
// duplicate AND read templates" to "can read templates only by also
// controlling the network between the parties". That is a real reduction and
// it is not the same as removing the dealer.

// SacrificeVerify checks every triple in use, consuming one triple from
// sacrifice per check.
//
// Returns nil only if every triple satisfies c = a*b. Any failure aborts the
// whole batch: a batch containing one forged triple is evidence that the
// offline phase is compromised, and continuing with the remaining triples would
// mean trusting the same source that just produced a bad one.
func (s *Session) SacrificeVerify(ctx context.Context, use, sacrifice []Triple) error {
	if len(use) == 0 {
		return nil
	}
	if len(sacrifice) < len(use) {
		return fmt.Errorf("mpc: verifying %d triples needs %d to sacrifice, got %d — an unverified "+
			"triple is one a dealer could have forged", len(use), len(use), len(sacrifice))
	}
	n := len(use)

	// Round 1: a public random challenge per triple.
	//
	// It must be unpredictable TO THE DEALER. Deriving it from the session id
	// or any other value fixed before the triples were made would let a dealer
	// craft triples that pass the check it knows is coming, which is the whole
	// attack this is meant to stop. So each party contributes fresh randomness
	// and the challenge is the opened sum.
	myR := make([]Element, n)
	for k := range myR {
		v, err := randomElement()
		if err != nil {
			return err
		}
		myR[k] = v
	}
	s.round++
	r, err := OpenVia(ctx, s.Transport, s.round, myR)
	if err != nil {
		return fmt.Errorf("mpc: opening the verification challenge: %w", err)
	}
	for k, v := range r {
		if v == 0 {
			// With r = 0 the identity degenerates and stops saying anything
			// about the checked triple. It happens with probability 2^-61;
			// refusing is safe and simpler than re-drawing.
			return fmt.Errorf("mpc: verification challenge %d came out zero, which would make the "+
				"check vacuous; retry the offline phase", k)
		}
	}

	// Round 2: open rho and sigma together.
	blinded := make([]Element, 0, 2*n)
	for k := 0; k < n; k++ {
		rho := Sub(Mul(r[k], use[k].A), sacrifice[k].A)
		sigma := Sub(use[k].B, sacrifice[k].B)
		blinded = append(blinded, rho, sigma)
	}
	s.round++
	opened, err := OpenVia(ctx, s.Transport, s.round, blinded)
	if err != nil {
		return fmt.Errorf("mpc: opening rho/sigma: %w", err)
	}

	// Round 3: open the check value, which must be zero for every triple.
	first := s.Transport.Index() == 0
	checks := make([]Element, n)
	for k := 0; k < n; k++ {
		rho, sigma := opened[2*k], opened[2*k+1]
		z := Sub(Mul(r[k], use[k].C), sacrifice[k].C)
		z = Sub(z, Mul(sigma, sacrifice[k].A))
		z = Sub(z, Mul(rho, sacrifice[k].B))
		if first {
			// Public term, subtracted once rather than once per party.
			z = Sub(z, Mul(rho, sigma))
		}
		checks[k] = z
	}
	s.round++
	results, err := OpenVia(ctx, s.Transport, s.round, checks)
	if err != nil {
		return fmt.Errorf("mpc: opening the verification result: %w", err)
	}

	var bad []int
	for k, v := range results {
		if v != 0 {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		show := bad
		if len(show) > 8 {
			show = show[:8]
		}
		return fmt.Errorf("mpc: %d of %d triples failed verification (first: %v) — c != a*b, so "+
			"the offline phase produced triples that would silently corrupt a comparison. "+
			"Treat the dealer as compromised; do not fall back to using them", len(bad), n, show)
	}
	return nil
}

// VerifiedTriples splits a party's supply into a verified working set and the
// triples spent verifying it.
//
// Verification costs half the supply. That is the price of not taking the
// dealer's word, and it is why TriplesForVerifiedWork exists: size the offline
// phase for it up front rather than running out mid-registration.
func VerifiedTriples(ctx context.Context, s *Session, all []Triple) ([]Triple, error) {
	if len(all) < 2 {
		return nil, fmt.Errorf("mpc: need at least two triples to verify one, got %d", len(all))
	}
	half := len(all) / 2
	use, sacrifice := all[:half], all[half:2*half]
	if err := s.SacrificeVerify(ctx, use, sacrifice); err != nil {
		return nil, err
	}
	return use, nil
}

// TriplesForVerifiedWork is how many raw triples the dealer must supply so that
// `work` of them survive verification.
func TriplesForVerifiedWork(work int) int { return 2 * work }
