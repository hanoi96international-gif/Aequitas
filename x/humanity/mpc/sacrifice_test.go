package mpc

import (
	"context"
	"math/rand"
	"sync"
	"testing"
)

// verifyAcross runs SacrificeVerify on every party concurrently and returns
// each party's verdict. Both must reach the same one.
func verifyAcross(t *testing.T, parties int, triples [][]Triple, count int) []error {
	t.Helper()
	transports, err := NewLocalTransports(parties)
	if err != nil {
		t.Fatal(err)
	}
	errs := make([]error, parties)
	var wg sync.WaitGroup
	for p := 0; p < parties; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			s := NewSession(transports[p], NewTripleStore(nil))
			use := triples[p][:count]
			sacrifice := triples[p][count : 2*count]
			errs[p] = s.SacrificeVerify(context.Background(), use, sacrifice)
		}(p)
	}
	wg.Wait()
	return errs
}

// TestHonestTriplesPassVerification: the check must not reject good triples, or
// it would halt registration for everyone.
func TestHonestTriplesPassVerification(t *testing.T) {
	for _, parties := range []int{2, 3} {
		triples, err := GenerateTriples(64, parties)
		if err != nil {
			t.Fatal(err)
		}
		for p, err := range verifyAcross(t, parties, triples, 32) {
			if err != nil {
				t.Errorf("parties=%d party %d rejected honest triples: %v", parties, p, err)
			}
		}
	}
}

// TestForgedTripleIsCaught is the attack, executed.
//
// A dealer that ships one triple with c != a*b makes the multiplication
// consuming it produce a wrong value with no error anywhere — enough to flip a
// duplicate check to "new" and mint a second account for someone already
// registered.
func TestForgedTripleIsCaught(t *testing.T) {
	const parties, count = 2, 40

	for _, corruptAt := range []int{0, 17, count - 1} {
		triples, err := GenerateTriples(2*count, parties)
		if err != nil {
			t.Fatal(err)
		}
		// The dealer tampers with exactly one triple, on one party's copy —
		// the cheapest possible forgery.
		triples[1][corruptAt].C = Add(triples[1][corruptAt].C, 1)

		errs := verifyAcross(t, parties, triples, count)
		for p, err := range errs {
			if err == nil {
				t.Errorf("party %d accepted a batch containing a forged triple at index %d — "+
					"a dealer could silently decide that a returning person is new",
					p, corruptAt)
			}
		}
		if errs[0] != nil {
			t.Logf("corrupt at %d, detected: %v", corruptAt, errs[0])
		}
	}
}

// TestForgedTripleIsCaughtWhateverIsTampered: a and b are as good a target as
// c, and the check must not have a blind spot.
func TestForgedTripleIsCaughtWhateverIsTampered(t *testing.T) {
	const parties, count = 2, 24

	cases := []struct {
		name  string
		apply func(*Triple)
	}{
		{"c off by one", func(tr *Triple) { tr.C = Add(tr.C, 1) }},
		{"a off by one", func(tr *Triple) { tr.A = Add(tr.A, 1) }},
		{"b off by one", func(tr *Triple) { tr.B = Add(tr.B, 1) }},
		{"c replaced with zero", func(tr *Triple) { tr.C = 0 }},
		{"c scaled", func(tr *Triple) { tr.C = Mul(tr.C, 7) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			triples, err := GenerateTriples(2*count, parties)
			if err != nil {
				t.Fatal(err)
			}
			tc.apply(&triples[0][5])
			for p, err := range verifyAcross(t, parties, triples, count) {
				if err == nil {
					t.Errorf("party %d accepted a triple tampered as %q", p, tc.name)
				}
			}
		})
	}
}

// TestVerificationSurvivesManyForgeries: a dealer corrupting a large fraction
// must not overwhelm the check into passing by cancellation.
func TestVerificationSurvivesManyForgeries(t *testing.T) {
	const parties, count = 2, 64
	triples, err := GenerateTriples(2*count, parties)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(21))
	for k := 0; k < count; k++ {
		if rng.Intn(2) == 0 {
			triples[0][k].C = Add(triples[0][k].C, Element(1+rng.Intn(1000)))
		}
	}
	for p, err := range verifyAcross(t, parties, triples, count) {
		if err == nil {
			t.Errorf("party %d accepted a batch with roughly half the triples forged", p)
		}
	}
}

// TestVerificationRevealsNothingAboutTheTriple: the checked triple stays in use
// afterwards, so the check must not leak it. What crosses the wire is rho and
// sigma, both blinded by the sacrificed triple's fresh randomness.
func TestVerificationRevealsNothingAboutTheTriple(t *testing.T) {
	const parties, count = 2, 200
	triples, err := GenerateTriples(2*count, parties)
	if err != nil {
		t.Fatal(err)
	}

	// Force every checked triple to share the same a, so any dependence of the
	// opened values on a would show up as a repeated pattern.
	fixedA, err := randomElement()
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < count; k++ {
		triples[0][k].A = fixedA
		triples[1][k].A = 0
		b := Add(triples[0][k].B, triples[1][k].B)
		c := Mul(fixedA, b)
		cs, err := Split(c, parties)
		if err != nil {
			t.Fatal(err)
		}
		triples[0][k].C, triples[1][k].C = cs[0], cs[1]
	}

	transports, err := NewLocalTransports(parties)
	if err != nil {
		t.Fatal(err)
	}
	// Capture what actually crosses the wire.
	rec := &recordingTransport{Transport: transports[0]}

	var wg sync.WaitGroup
	errs := make([]error, parties)
	for p := 0; p < parties; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			tr := transports[p]
			if p == 0 {
				tr = rec
			}
			s := NewSession(tr, NewTripleStore(nil))
			errs[p] = s.SacrificeVerify(context.Background(),
				triples[p][:count], triples[p][count:2*count])
		}(p)
	}
	wg.Wait()
	for p, err := range errs {
		if err != nil {
			t.Fatalf("party %d: %v", p, err)
		}
	}

	// The rho values (round 2, even positions) must not repeat despite every
	// triple sharing the same a.
	if len(rec.sent) < 2 {
		t.Fatalf("expected at least two rounds on the wire, saw %d", len(rec.sent))
	}
	rhoSigma := rec.sent[1]
	seen := map[Element]int{}
	for i := 0; i < len(rhoSigma); i += 2 {
		seen[rhoSigma[i]]++
	}
	for v, n := range seen {
		if n > 2 {
			t.Errorf("the opened value %d appeared %d times across triples that share the same a — "+
				"the blinding is not fresh per triple", v, n)
		}
	}
}

// recordingTransport keeps a copy of everything this party publishes, so a test
// can assert on what actually left the machine rather than on what the protocol
// was supposed to send.
type recordingTransport struct {
	Transport
	mu   sync.Mutex
	sent [][]Element
}

func (r *recordingTransport) Exchange(ctx context.Context, round int, mine []Element) ([][]Element, error) {
	r.mu.Lock()
	r.sent = append(r.sent, append([]Element(nil), mine...))
	r.mu.Unlock()
	return r.Transport.Exchange(ctx, round, mine)
}

func TestVerifiedTriplesRefusesTooSmallASupply(t *testing.T) {
	transports, err := NewLocalTransports(2)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(transports[0], NewTripleStore(nil))
	if _, err := VerifiedTriples(context.Background(), s, []Triple{{A: 1, B: 2, C: 2}}); err == nil {
		t.Error("a supply too small to verify was accepted")
	}
	if got := TriplesForVerifiedWork(1000); got != 2000 {
		t.Errorf("TriplesForVerifiedWork(1000) = %d, want 2000", got)
	}
}
