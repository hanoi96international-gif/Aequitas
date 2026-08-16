package mpc

import (
	"context"
	"fmt"
)

// Enrollment is one previously registered person's template, as held by the
// whole party set. ID is an opaque handle (a registration reference), never
// anything that identifies the human outside this system.
type Enrollment struct {
	ID       string
	Template *SharedTemplate
}

// Registry is the storage each validator set keeps. A real implementation is
// backed by per-validator rows: validator i persists Template.Party(i) and
// nothing else, so no single database contains a reconstructable template.
type Registry interface {
	// List returns every enrollment to compare a candidate against.
	List(ctx context.Context) ([]Enrollment, error)
	// Add stores a new enrollment.
	Add(ctx context.Context, e Enrollment) error
}

// Outcome is what the matcher concluded.
type Outcome int

const (
	// OutcomeDistinct means the candidate matched nobody: register normally.
	OutcomeDistinct Outcome = iota
	// OutcomeReview means the candidate is close to an existing enrollment.
	//
	// This is NOT "reject". Biometric matching has a false-match rate that is
	// never zero, and a wrongly rejected person is locked out of a currency
	// whose whole premise is that existing is enough to belong. Every path
	// acting on this value must lead somewhere a human can come back from.
	OutcomeReview
)

func (o Outcome) String() string {
	switch o {
	case OutcomeDistinct:
		return "distinct"
	case OutcomeReview:
		return "review"
	default:
		return fmt.Sprintf("Outcome(%d)", int(o))
	}
}

// Decision is the matcher's result.
type Decision struct {
	Outcome Outcome
	// MatchedID is the enrollment the candidate resembled, set only for
	// OutcomeReview. It lets a reviewer look at the specific prior
	// registration rather than the whole population.
	MatchedID string
	// Compared is how many enrollments were checked, for observability: a
	// sudden drop means the registry query is failing, which would otherwise
	// look exactly like "nobody matched".
	Compared int
}

// Matcher decides whether a candidate template belongs to someone already
// registered, without any party learning a template or a distance.
type Matcher struct {
	// Threshold is the maximum Hamming distance still considered the same
	// person. It is a policy value, not a constant of nature: too low and
	// genuine re-captures of one person look like strangers (duplicate
	// registrations slip through), too high and strangers collide (real people
	// get sent to review). It must be calibrated against measured
	// same-person / different-person distance distributions for the actual
	// capture pipeline, and re-calibrated whenever that pipeline changes.
	Threshold int
	Registry  Registry
}

// TriplesNeeded reports how many multiplication triples one CheckCandidate
// call will consume against the current registry size. Callers must provision
// at least this many in the offline phase; running out mid-check aborts the
// check rather than silently weakening it.
func (m *Matcher) TriplesNeeded(ctx context.Context, templateLen int) (int, error) {
	enrollments, err := m.Registry.List(ctx)
	if err != nil {
		return 0, err
	}
	return len(enrollments) * TriplesPerComparison(templateLen), nil
}

// CheckCandidate compares a candidate against every enrollment.
//
// SCALING, STATED HONESTLY: this is a linear scan, so cost grows with the
// number of registered people — the standard problem for private 1:N
// biometric de-duplication. At the population this chain has today it is
// trivial; at millions it is not, and the answer there is the same as in the
// published work on large-scale private iris matching: bucket the search space
// so a candidate is only compared against a small candidate set, and batch the
// comparisons. Neither changes the protocol below, both change what it is
// called with. This implementation is correct first; it is deliberately not
// pretending to be the scaled one.
func (m *Matcher) CheckCandidate(ctx context.Context, candidate *SharedTemplate, stores []*TripleStore) (Decision, error) {
	if m.Registry == nil {
		return Decision{}, fmt.Errorf("mpc: matcher has no registry — refusing to report " +
			"'distinct' when nothing was actually compared")
	}
	if m.Threshold < 0 {
		return Decision{}, fmt.Errorf("mpc: negative threshold %d", m.Threshold)
	}

	enrollments, err := m.Registry.List(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("mpc: could not list enrollments: %w", err)
	}

	decision := Decision{Outcome: OutcomeDistinct}
	for _, e := range enrollments {
		if err := ctx.Err(); err != nil {
			return Decision{}, err
		}
		if e.Template.Len() != candidate.Len() {
			return Decision{}, fmt.Errorf("mpc: enrollment %s has %d features, candidate has %d — "+
				"templates from different capture pipelines cannot be compared, and comparing them "+
				"anyway would produce a meaningless distance", e.ID, e.Template.Len(), candidate.Len())
		}
		res, err := SecureMatch(e.Template, candidate, m.Threshold, stores)
		if err != nil {
			return Decision{}, fmt.Errorf("mpc: comparing against %s: %w", e.ID, err)
		}
		decision.Compared++
		if res.Similar {
			decision.Outcome = OutcomeReview
			decision.MatchedID = e.ID
			// Stop at the first match: continuing would consume triples and
			// reveal additional bits about the candidate's relationship to
			// other enrollments without changing the outcome.
			return decision, nil
		}
	}
	return decision, nil
}

// MemoryRegistry is an in-memory Registry for tests and for a single-process
// dry run. Production storage is per-validator and durable; this exists so the
// protocol can be exercised without one.
type MemoryRegistry struct {
	entries []Enrollment
}

// NewMemoryRegistry returns an empty in-memory registry.
func NewMemoryRegistry() *MemoryRegistry { return &MemoryRegistry{} }

// List implements Registry.
func (r *MemoryRegistry) List(context.Context) ([]Enrollment, error) {
	out := make([]Enrollment, len(r.entries))
	copy(out, r.entries)
	return out, nil
}

// Add implements Registry.
func (r *MemoryRegistry) Add(_ context.Context, e Enrollment) error {
	if e.Template == nil {
		return fmt.Errorf("mpc: enrollment %q has no template", e.ID)
	}
	for _, existing := range r.entries {
		if existing.ID == e.ID {
			return fmt.Errorf("mpc: enrollment %q already exists", e.ID)
		}
	}
	r.entries = append(r.entries, e)
	return nil
}
