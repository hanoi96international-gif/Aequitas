// Package mpc implements the secure multi-party computation used for
// biometric de-duplication: deciding whether a newly presented biometric
// template is "too close" to one already registered, WITHOUT any single party
// ever seeing a template.
//
// WHY THIS EXISTS
//
// The registration path this replaces derived a nullifier as Poseidon(bio) and
// enforced uniqueness by exact hash equality. That cannot work, for two
// independent reasons:
//
//  1. A cryptographic hash is an avalanche function. Two captures of the SAME
//     person never produce the same bytes, so they never produce the same
//     nullifier. Exact-equality de-duplication therefore detects nothing.
//  2. Nothing bound the hashed value to a real measurement — any number
//     produces a valid proof — so an attacker was never even forced to try.
//
// Recognising a person across two captures requires a DISTANCE, and a distance
// requires the templates themselves. Handing raw templates to a server would
// make that server able to identify everyone; handing them to a validator set
// would make every validator able to. This package is the way out: the
// template is split into additive shares, each party holds one share, and the
// distance is computed on the shares. No party can reconstruct a template, yet
// together they can answer the single question that matters — "is this person
// already registered?" — and learn nothing else.
//
// THE AEQUITAS CONSTRAINT
//
// This design is chosen so that being recognised as a human requires nothing
// but a body: no passport, no government, no device of a particular price, no
// operator who must be trusted with a face. That is not incidental, it is the
// point ("money exists because people exist"). Accordingly the protocol's
// output is deliberately a single bit — "similar to an existing registration"
// — which is a reason to REVIEW, never an automatic, permanent rejection. A
// false match must always be recoverable by the human it concerns; the
// alternative is exactly the unappealable exclusion this project treats as a
// defect.
//
// SECURITY MODEL, STATED PLAINLY
//
// Additive sharing over a prime field is information-theoretically hiding: any
// strict subset of the shares is uniformly distributed and reveals NOTHING
// about the secret, regardless of the adversary's computing power. What the
// scheme does NOT provide on its own is robustness against parties that lie
// about their computation (malicious security) — this implementation assumes
// semi-honest parties, i.e. they follow the protocol but may try to learn from
// what they see. See beaver.go for how the multiplication triples are produced
// and what trusting their dealer does and does not imply.
package mpc

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/bits"
)

// Prime is 2^61 - 1, a Mersenne prime.
//
// Chosen for three reasons: reduction modulo a Mersenne prime is a shift and
// an add rather than a division; the field is wide enough that a squared
// Euclidean distance over a full-length template cannot wrap around (which
// would silently turn a huge distance into a small one, i.e. a stranger into a
// match); and it leaves ~40 bits of headroom for the statistical masks used
// when a value must be opened.
const Prime uint64 = (1 << 61) - 1

// Element is a residue modulo Prime. Values are always kept reduced.
type Element uint64

// Add returns a + b mod Prime.
func Add(a, b Element) Element {
	s := uint64(a) + uint64(b)
	if s >= Prime {
		s -= Prime
	}
	return Element(s)
}

// Sub returns a - b mod Prime.
func Sub(a, b Element) Element {
	if a >= b {
		return a - b
	}
	return Element(uint64(a) + Prime - uint64(b))
}

// Neg returns -a mod Prime.
func Neg(a Element) Element {
	if a == 0 {
		return 0
	}
	return Element(Prime - uint64(a))
}

// Mul returns a * b mod Prime.
//
// The product of two 61-bit values does not fit in 64 bits, so this uses the
// full 128-bit product and then the Mersenne identity 2^61 ≡ 1 (mod 2^61 - 1),
// which turns reduction into "fold the high bits back onto the low bits".
func Mul(a, b Element) Element {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	// Split the 128-bit product at bit 61: value = hi*2^64 + lo
	//                                           = (hi<<3)*2^61 + lo
	// and 2^61 ≡ 1, so the high part folds down by simple addition.
	folded := (lo & Prime) + (lo >> 61) + (hi << 3)
	for folded >= Prime {
		folded -= Prime
	}
	return Element(folded)
}

// Pow returns base^exp mod Prime by square-and-multiply.
func Pow(base Element, exp uint64) Element {
	result := Element(1)
	b := base
	for exp > 0 {
		if exp&1 == 1 {
			result = Mul(result, b)
		}
		b = Mul(b, b)
		exp >>= 1
	}
	return result
}

// Inv returns the multiplicative inverse of a via Fermat's little theorem
// (a^(p-2) ≡ a^-1 for prime p). Zero has no inverse and is reported rather
// than silently returning zero, which would turn a division-by-zero bug into a
// wrong-but-plausible result.
func Inv(a Element) (Element, error) {
	if a == 0 {
		return 0, fmt.Errorf("mpc: zero has no multiplicative inverse")
	}
	return Pow(a, Prime-2), nil
}

// FromInt maps a signed integer into the field, mapping negatives to their
// additive inverse. Callers passing measurement data should keep |v| well
// below Prime; this is a mapping, not a range check.
func FromInt(v int64) Element {
	if v >= 0 {
		return Element(uint64(v) % Prime)
	}
	return Neg(Element(uint64(-v) % Prime))
}

// randomElement draws a uniform field element from crypto/rand.
//
// Rejection sampling rather than modular reduction: reducing a uniform 64-bit
// draw modulo Prime is measurably biased toward small values, and a biased
// share is a share that leaks. The rejection probability here is under 2^-3
// per draw, so the loop terminates immediately in practice.
func randomElement() (Element, error) {
	var buf [8]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, fmt.Errorf("mpc: secure randomness unavailable, refusing to produce a share: %w", err)
		}
		v := binary.BigEndian.Uint64(buf[:]) >> 3 // 61 bits
		if v < Prime {
			return Element(v), nil
		}
	}
}
