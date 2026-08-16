package mpc

import (
	"math/big"
	"testing"
)

// bigPrime mirrors Prime for cross-checking the fast Mersenne arithmetic
// against math/big, which is slow but obviously correct.
var bigPrime = new(big.Int).SetUint64(Prime)

func refMul(a, b Element) Element {
	x := new(big.Int).SetUint64(uint64(a))
	y := new(big.Int).SetUint64(uint64(b))
	x.Mul(x, y).Mod(x, bigPrime)
	return Element(x.Uint64())
}

// TestFieldArithmeticMatchesBigInt checks the hand-rolled Mersenne reduction
// against a reference implementation. A wrong reduction here would not crash —
// it would produce plausible-looking garbage, and every guarantee above it
// (hiding, correctness of the distance) would quietly stop holding.
func TestFieldArithmeticMatchesBigInt(t *testing.T) {
	cases := []Element{0, 1, 2, 7, 1 << 30, 1 << 60, Element(Prime - 1), Element(Prime - 2), 123456789}
	for _, a := range cases {
		for _, b := range cases {
			if got, want := Mul(a, b), refMul(a, b); got != want {
				t.Errorf("Mul(%d,%d) = %d, want %d", a, b, got, want)
			}
			sum := new(big.Int).Add(new(big.Int).SetUint64(uint64(a)), new(big.Int).SetUint64(uint64(b)))
			sum.Mod(sum, bigPrime)
			if got, want := Add(a, b), Element(sum.Uint64()); got != want {
				t.Errorf("Add(%d,%d) = %d, want %d", a, b, got, want)
			}
			diff := new(big.Int).Sub(new(big.Int).SetUint64(uint64(a)), new(big.Int).SetUint64(uint64(b)))
			diff.Mod(diff, bigPrime)
			if got, want := Sub(a, b), Element(diff.Uint64()); got != want {
				t.Errorf("Sub(%d,%d) = %d, want %d", a, b, got, want)
			}
		}
	}
}

func TestInvRoundTrips(t *testing.T) {
	for _, a := range []Element{1, 2, 3, 1 << 40, Element(Prime - 1)} {
		inv, err := Inv(a)
		if err != nil {
			t.Fatalf("Inv(%d): %v", a, err)
		}
		if got := Mul(a, inv); got != 1 {
			t.Errorf("a * a^-1 = %d, want 1 (a=%d)", got, a)
		}
	}
	if _, err := Inv(0); err == nil {
		t.Error("Inv(0) must fail rather than return a plausible wrong value")
	}
}

// TestSplitReconstructRoundTrip is the basic sharing contract.
func TestSplitReconstructRoundTrip(t *testing.T) {
	for _, secret := range []Element{0, 1, 42, Element(Prime - 1)} {
		for _, n := range []int{2, 3, 5} {
			sh, err := Split(secret, n)
			if err != nil {
				t.Fatalf("Split: %v", err)
			}
			if got := Reconstruct(sh); got != secret {
				t.Errorf("n=%d: reconstructed %d, want %d", n, got, secret)
			}
		}
	}
}

// TestSplitRefusesSingleParty pins that the package will not hand back a
// "sharing" that is just the plaintext.
func TestSplitRefusesSingleParty(t *testing.T) {
	if _, err := Split(5, 1); err == nil {
		t.Error("Split with n=1 must fail: one share IS the secret")
	}
}

// TestStrictSubsetRevealsNothing is the security property that makes this
// usable on biometric data: any n-1 shares must be independent of the secret.
//
// It checks the observable consequence — sharing the same secret twice yields
// unrelated shares, and shares of two very different secrets are
// indistinguishable in the only way a test can assert cheaply: the first n-1
// shares of the two sharings collide no more often than chance.
func TestStrictSubsetRevealsNothing(t *testing.T) {
	const n = 3
	a, err := Split(0, n)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Split(Element(Prime-1), n)
	if err != nil {
		t.Fatal(err)
	}
	if a[0] == b[0] && a[1] == b[1] {
		t.Error("two sharings of very different secrets produced identical prefixes — " +
			"randomness is not being drawn per share")
	}
	// Same secret twice must not produce the same shares either.
	c, err := Split(42, n)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Split(42, n)
	if err != nil {
		t.Fatal(err)
	}
	if c[0] == d[0] && c[1] == d[1] && c[2] == d[2] {
		t.Error("sharing the same secret twice produced identical shares — shares are deterministic, " +
			"which would let an observer link two registrations of the same template")
	}
}

func newStores(t *testing.T, count, n int) []*TripleStore {
	t.Helper()
	triples, err := GenerateTriples(count, n)
	if err != nil {
		t.Fatalf("GenerateTriples: %v", err)
	}
	stores := make([]*TripleStore, n)
	for i := range stores {
		stores[i] = NewTripleStore(triples[i])
	}
	return stores
}

// TestMulSharesComputesTheProduct verifies Beaver multiplication end to end.
func TestMulSharesComputesTheProduct(t *testing.T) {
	const n = 3
	pairs := [][2]Element{{0, 0}, {1, 1}, {3, 7}, {123456, 654321}, {Element(Prime - 1), 2}}
	stores := newStores(t, len(pairs), n)

	for _, p := range pairs {
		x, err := Split(p[0], n)
		if err != nil {
			t.Fatal(err)
		}
		y, err := Split(p[1], n)
		if err != nil {
			t.Fatal(err)
		}
		z, err := MulShares(x, y, stores)
		if err != nil {
			t.Fatalf("MulShares: %v", err)
		}
		if got, want := Reconstruct(z), refMul(p[0], p[1]); got != want {
			t.Errorf("%d * %d = %d, want %d", p[0], p[1], got, want)
		}
	}
}

// TestTripleExhaustionIsAnError pins that a depleted store fails loudly.
// Silently reusing a triple would keep producing correct products while
// destroying the privacy the whole package exists for — the worst possible
// failure mode, because nothing would look wrong.
func TestTripleExhaustionIsAnError(t *testing.T) {
	const n = 2
	stores := newStores(t, 1, n)
	x, _ := Split(3, n)
	y, _ := Split(4, n)

	if _, err := MulShares(x, y, stores); err != nil {
		t.Fatalf("first multiplication should succeed: %v", err)
	}
	if _, err := MulShares(x, y, stores); err == nil {
		t.Error("second multiplication must fail — reusing a triple stops blinding the openings")
	}
}

// plainHamming is the obvious reference implementation.
func plainHamming(a, b []uint8) int {
	d := 0
	for i := range a {
		if a[i] != b[i] {
			d++
		}
	}
	return d
}

// TestSecureHammingMatchesPlaintext runs the private distance against the
// obvious one for a range of templates.
func TestSecureHammingMatchesPlaintext(t *testing.T) {
	const n = 3
	cases := [][2][]uint8{
		{{0, 0, 0, 0}, {0, 0, 0, 0}},
		{{1, 1, 1, 1}, {0, 0, 0, 0}},
		{{1, 0, 1, 0}, {1, 0, 0, 1}},
		{{0, 1, 1, 0, 1, 0, 0, 1}, {0, 1, 0, 0, 1, 1, 0, 1}},
	}
	for _, c := range cases {
		stores := newStores(t, TriplesPerComparison(len(c[0])), n)
		a, err := NewSharedTemplate(c[0], n)
		if err != nil {
			t.Fatal(err)
		}
		b, err := NewSharedTemplate(c[1], n)
		if err != nil {
			t.Fatal(err)
		}
		dist, err := SecureHammingDistance(a, b, stores)
		if err != nil {
			t.Fatalf("SecureHammingDistance: %v", err)
		}
		if got, want := Reconstruct(dist), Element(uint64(plainHamming(c[0], c[1]))); got != want {
			t.Errorf("distance(%v,%v) = %d, want %d", c[0], c[1], got, want)
		}
	}
}

// TestThresholdPolynomialIsAStepFunction checks the interpolation itself in
// the clear, before it is trusted inside the protocol.
func TestThresholdPolynomialIsAStepFunction(t *testing.T) {
	const maxDistance = 12
	for _, threshold := range []int{0, 1, 5, 12, 13} {
		coeffs, err := thresholdPolynomial(threshold, maxDistance)
		if err != nil {
			t.Fatalf("threshold=%d: %v", threshold, err)
		}
		for d := 0; d <= maxDistance; d++ {
			// Evaluate by Horner in the clear.
			acc := coeffs[len(coeffs)-1]
			for i := len(coeffs) - 2; i >= 0; i-- {
				acc = Add(Mul(acc, Element(uint64(d))), coeffs[i])
			}
			want := Element(0)
			if d < threshold {
				want = 1
			}
			if acc != want {
				t.Errorf("threshold=%d d=%d: f(d)=%d, want %d", threshold, d, acc, want)
			}
		}
	}
}

// TestSecureMatchDecidesCorrectly is the end-to-end property the whole package
// exists to provide: same person in, "similar" out; different people in,
// "not similar" out — with the distance itself never revealed.
func TestSecureMatchDecidesCorrectly(t *testing.T) {
	const n = 3
	const threshold = 3 // fewer than 3 differing bits counts as the same person

	base := []uint8{1, 0, 1, 1, 0, 0, 1, 0, 1, 1, 0, 1}
	nearlySame := []uint8{1, 0, 1, 1, 0, 0, 1, 0, 1, 1, 0, 0} // 1 bit apart
	different := []uint8{0, 1, 0, 0, 1, 1, 0, 1, 0, 0, 1, 0}  // 12 bits apart

	for _, tc := range []struct {
		name  string
		other []uint8
		want  bool
	}{
		{"same capture", base, true},
		{"same person, one bit of noise", nearlySame, true},
		{"different person", different, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stores := newStores(t, TriplesPerComparison(len(base))+4, n)
			a, err := NewSharedTemplate(base, n)
			if err != nil {
				t.Fatal(err)
			}
			b, err := NewSharedTemplate(tc.other, n)
			if err != nil {
				t.Fatal(err)
			}
			res, err := SecureMatch(a, b, threshold, stores)
			if err != nil {
				t.Fatalf("SecureMatch: %v", err)
			}
			if res.Similar != tc.want {
				t.Errorf("Similar = %v, want %v (plaintext distance %d, threshold %d)",
					res.Similar, tc.want, plainHamming(base, tc.other), threshold)
			}
		})
	}
}

// TestNewSharedTemplateRejectsNonBinary pins that a caller cannot silently
// feed in a feature vector the Hamming distance is meaningless on.
func TestNewSharedTemplateRejectsNonBinary(t *testing.T) {
	if _, err := NewSharedTemplate([]uint8{0, 1, 2}, 3); err == nil {
		t.Error("a non-binary template must be rejected, not silently measured")
	}
}

// TestNoPartyCanReconstructAlone is the claim a reader should be able to check
// directly: one validator's stored row, on its own, is uniform noise.
func TestNoPartyCanReconstructAlone(t *testing.T) {
	const n = 3
	template := []uint8{1, 1, 1, 1, 1, 1, 1, 1}
	st, err := NewSharedTemplate(template, n)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		row := st.Party(i)
		allBinary := true
		for _, v := range row {
			if v > 1 {
				allBinary = false
				break
			}
		}
		if allBinary {
			t.Errorf("party %d's row is still binary — it looks like the template rather than "+
				"a uniform share, which would mean the template is stored in the clear", i)
		}
	}
}
