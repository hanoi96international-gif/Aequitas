package mpc

import (
	"context"
	"testing"
)

const testParties = 3

func mustTemplate(t *testing.T, bits []uint8) *SharedTemplate {
	t.Helper()
	st, err := NewSharedTemplate(bits, testParties)
	if err != nil {
		t.Fatalf("NewSharedTemplate: %v", err)
	}
	return st
}

func storesFor(t *testing.T, comparisons, templateLen int) []*TripleStore {
	t.Helper()
	// A little headroom: SecureMatch spends TriplesPerComparison per pair.
	return newStores(t, comparisons*TriplesPerComparison(templateLen)+8, testParties)
}

// TestMatcherFindsAReturningPerson is the case the whole system exists for: a
// person who already registered comes back with a slightly different capture,
// and must be recognised despite the bits not being identical.
func TestMatcherFindsAReturningPerson(t *testing.T) {
	ctx := context.Background()
	enrolled := []uint8{1, 0, 1, 1, 0, 0, 1, 0}
	recapture := []uint8{1, 0, 1, 1, 0, 1, 1, 0} // one bit of sensor noise

	reg := NewMemoryRegistry()
	if err := reg.Add(ctx, Enrollment{ID: "person-1", Template: mustTemplate(t, enrolled)}); err != nil {
		t.Fatal(err)
	}

	m := &Matcher{Threshold: 3, Registry: reg}
	stores := storesFor(t, 1, len(enrolled))

	got, err := m.CheckCandidate(ctx, mustTemplate(t, recapture), stores)
	if err != nil {
		t.Fatalf("CheckCandidate: %v", err)
	}
	if got.Outcome != OutcomeReview {
		t.Errorf("outcome = %v, want review — a returning person was treated as new, "+
			"which is exactly the duplicate registration this protocol exists to catch", got.Outcome)
	}
	if got.MatchedID != "person-1" {
		t.Errorf("MatchedID = %q, want person-1", got.MatchedID)
	}
}

// TestMatcherLetsAStrangerThrough is the other half: a different person must
// NOT be sent to review, or the system manufactures false accusations against
// people whose only mistake was existing.
func TestMatcherLetsAStrangerThrough(t *testing.T) {
	ctx := context.Background()
	enrolled := []uint8{1, 0, 1, 1, 0, 0, 1, 0}
	stranger := []uint8{0, 1, 0, 0, 1, 1, 0, 1} // maximally different

	reg := NewMemoryRegistry()
	if err := reg.Add(ctx, Enrollment{ID: "person-1", Template: mustTemplate(t, enrolled)}); err != nil {
		t.Fatal(err)
	}

	m := &Matcher{Threshold: 3, Registry: reg}
	stores := storesFor(t, 1, len(enrolled))

	got, err := m.CheckCandidate(ctx, mustTemplate(t, stranger), stores)
	if err != nil {
		t.Fatalf("CheckCandidate: %v", err)
	}
	if got.Outcome != OutcomeDistinct {
		t.Errorf("outcome = %v, want distinct — a stranger was flagged, and under this project's "+
			"own rule a false match must never become a permanent exclusion", got.Outcome)
	}
	if got.Compared != 1 {
		t.Errorf("Compared = %d, want 1", got.Compared)
	}
}

// TestMatcherScansTheWholeRegistry pins that a candidate is checked against
// every enrollment, not just the first. A short-circuit here would let the
// second registered person be duplicated freely.
func TestMatcherScansTheWholeRegistry(t *testing.T) {
	ctx := context.Background()
	a := []uint8{1, 1, 1, 1, 0, 0, 0, 0}
	b := []uint8{0, 0, 0, 0, 1, 1, 1, 1}
	candidate := []uint8{0, 0, 0, 0, 1, 1, 1, 0} // one bit from b, far from a

	reg := NewMemoryRegistry()
	if err := reg.Add(ctx, Enrollment{ID: "first", Template: mustTemplate(t, a)}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(ctx, Enrollment{ID: "second", Template: mustTemplate(t, b)}); err != nil {
		t.Fatal(err)
	}

	m := &Matcher{Threshold: 2, Registry: reg}
	stores := storesFor(t, 2, len(a))

	got, err := m.CheckCandidate(ctx, mustTemplate(t, candidate), stores)
	if err != nil {
		t.Fatalf("CheckCandidate: %v", err)
	}
	if got.Outcome != OutcomeReview || got.MatchedID != "second" {
		t.Errorf("outcome=%v matched=%q, want review/second — the scan stopped before reaching "+
			"the enrollment it should have matched", got.Outcome, got.MatchedID)
	}
	if got.Compared != 2 {
		t.Errorf("Compared = %d, want 2", got.Compared)
	}
}

// TestMatcherEmptyRegistryIsDistinct covers the very first registration.
func TestMatcherEmptyRegistryIsDistinct(t *testing.T) {
	ctx := context.Background()
	m := &Matcher{Threshold: 3, Registry: NewMemoryRegistry()}
	got, err := m.CheckCandidate(ctx, mustTemplate(t, []uint8{1, 0, 1, 0}), nil)
	if err != nil {
		t.Fatalf("CheckCandidate: %v", err)
	}
	if got.Outcome != OutcomeDistinct || got.Compared != 0 {
		t.Errorf("first ever registration: outcome=%v compared=%d, want distinct/0", got.Outcome, got.Compared)
	}
}

// TestMatcherWithoutRegistryFails pins that a misconfigured matcher cannot
// report "distinct" — the dangerous default, since it silently disables
// de-duplication entirely while looking like it is working.
func TestMatcherWithoutRegistryFails(t *testing.T) {
	m := &Matcher{Threshold: 3}
	if _, err := m.CheckCandidate(context.Background(), mustTemplate(t, []uint8{1, 0}), nil); err == nil {
		t.Error("a matcher with no registry must fail, not report 'distinct'")
	}
}

// TestMatcherRejectsMismatchedTemplateLength pins that templates from
// different capture pipelines are refused rather than compared into a
// meaningless distance.
func TestMatcherRejectsMismatchedTemplateLength(t *testing.T) {
	ctx := context.Background()
	reg := NewMemoryRegistry()
	if err := reg.Add(ctx, Enrollment{ID: "x", Template: mustTemplate(t, []uint8{1, 0, 1, 0})}); err != nil {
		t.Fatal(err)
	}
	m := &Matcher{Threshold: 1, Registry: reg}
	stores := storesFor(t, 1, 8)

	if _, err := m.CheckCandidate(ctx, mustTemplate(t, []uint8{1, 0, 1, 0, 1, 1}), stores); err == nil {
		t.Error("comparing templates of different lengths must fail loudly")
	}
}

// TestTriplesNeededMatchesActualConsumption keeps the provisioning helper
// honest: if it under-reports, a real check runs out of triples halfway
// through and aborts a person's registration for an operational reason.
func TestTriplesNeededMatchesActualConsumption(t *testing.T) {
	ctx := context.Background()
	tmpl := []uint8{1, 0, 1, 1, 0, 0, 1, 0}

	reg := NewMemoryRegistry()
	for _, id := range []string{"a", "b", "c"} {
		// All far from the candidate, so the scan runs to completion.
		if err := reg.Add(ctx, Enrollment{ID: id, Template: mustTemplate(t, []uint8{0, 1, 0, 0, 1, 1, 0, 1})}); err != nil {
			t.Fatal(err)
		}
	}
	m := &Matcher{Threshold: 1, Registry: reg}

	need, err := m.TriplesNeeded(ctx, len(tmpl))
	if err != nil {
		t.Fatalf("TriplesNeeded: %v", err)
	}

	stores := newStores(t, need, testParties)
	if _, err := m.CheckCandidate(ctx, mustTemplate(t, tmpl), stores); err != nil {
		t.Fatalf("a full scan must fit in the reported triple budget, but: %v", err)
	}
}
