package mpc

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/sha3"
)

// Per-party authentication, replacing the shared token.
//
// # WHY THE SHARED TOKEN WAS WRONG
//
// The first version of this transport authenticated peers with one secret that
// every party held. That is adequate for exactly two parties who trust each
// other completely, and it breaks down the moment there is a third:
//
//   - Every party can impersonate every other. Holding the secret is the only
//     thing checked, so party 3 can submit a contribution as party 1. Since
//     contributions decide whether someone counts as a duplicate, a validator
//     could forge its peers' answers and mint accounts — the exact Sybil hole
//     the authentication existed to close.
//   - Adding or removing a validator means rotating the secret on every box at
//     once. With two boxes that is a phone call. With fifty it is not possible,
//     and until a compromised validator's secret is rotated everywhere, it can
//     still forge everything.
//   - Onboarding requires handing a secret to a stranger over a trusted
//     channel, in a project whose premise is that no such channel exists.
//
// # WHAT REPLACES IT
//
// Each party signs its own contributions; the peer list is public keys, not a
// secret. Adding a validator means publishing an address. Removing one means
// deleting it. Nobody can speak for anybody else, because nobody else has the
// key.
//
// The signature covers a digest that binds the payload to one session, one
// round and one party. Without that binding, a contribution captured in round 2
// could be replayed as round 5, or a contribution from party 0 replayed as
// party 1 — both are valid signatures over the same bytes.
//
// This package defines the interface and an ed25519 implementation for tests.
// The node implements it with the validator's existing block-signing key, so
// the MPC peer set is the validator set and the Node Binding Signature stays
// the root that ties that key to an operator.

// Authenticator signs this party's contributions and verifies its peers'.
type Authenticator interface {
	// Sign produces this party's signature over digest.
	Sign(digest []byte) ([]byte, error)

	// VerifyParty reports whether sig is a valid signature over digest by the
	// party at index. It must fail closed: an unknown party, a malformed
	// signature and a valid signature from the wrong party are all errors.
	VerifyParty(index int, digest, sig []byte) error

	// Parties is how many parties this authenticator knows keys for.
	Parties() int
}

// RoundDigest binds a payload to exactly one (session, round, party).
//
// Length-prefixed rather than concatenated: with plain concatenation a session
// "a" round 11 and a session "a1" round 1 would hash identically, and a
// contribution could be replayed across them.
func RoundDigest(session string, round, party int, payload []byte) []byte {
	h := sha3.New256()
	writeField := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	writeField([]byte("aequitas-mpc-round-v1"))
	writeField([]byte(session))

	var num [8]byte
	binary.BigEndian.PutUint64(num[:], uint64(round))
	writeField(num[:])
	binary.BigEndian.PutUint64(num[:], uint64(party))
	writeField(num[:])

	writeField(payload)
	return h.Sum(nil)
}

// Ed25519Authenticator holds this party's private key and every party's public
// key.
//
// Used by this package's tests, and usable in its own right where parties do
// not already have chain identities. The node uses the validator signing key
// instead, so that the MPC peer set is the validator set rather than a second,
// separately-managed key list that can drift out of step with it.
type Ed25519Authenticator struct {
	index   int
	private ed25519.PrivateKey
	public  []ed25519.PublicKey
}

// NewEd25519Authenticator validates the key material for one party.
func NewEd25519Authenticator(index int, priv ed25519.PrivateKey, pubs []ed25519.PublicKey) (*Ed25519Authenticator, error) {
	if len(pubs) < 2 {
		return nil, fmt.Errorf("mpc: %d parties cannot hide anything", len(pubs))
	}
	if index < 0 || index >= len(pubs) {
		return nil, fmt.Errorf("mpc: index %d is outside 0..%d", index, len(pubs)-1)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("mpc: private key is %d bytes, expected %d", len(priv), ed25519.PrivateKeySize)
	}
	for i, p := range pubs {
		if len(p) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("mpc: public key %d is %d bytes, expected %d",
				i, len(p), ed25519.PublicKeySize)
		}
	}
	// The private key must match the public key claimed for this index, or the
	// party signs contributions its peers will reject — a failure that would
	// otherwise surface only mid-registration.
	if got := priv.Public().(ed25519.PublicKey); !got.Equal(pubs[index]) {
		return nil, fmt.Errorf("mpc: the private key does not match the public key listed for " +
			"party at this index; every contribution would be rejected by the peers")
	}
	return &Ed25519Authenticator{index: index, private: priv, public: pubs}, nil
}

// Sign implements Authenticator.
func (a *Ed25519Authenticator) Sign(digest []byte) ([]byte, error) {
	return ed25519.Sign(a.private, digest), nil
}

// VerifyParty implements Authenticator.
func (a *Ed25519Authenticator) VerifyParty(index int, digest, sig []byte) error {
	if index < 0 || index >= len(a.public) {
		return fmt.Errorf("mpc: no public key for party %d", index)
	}
	if !ed25519.Verify(a.public[index], digest, sig) {
		return fmt.Errorf("mpc: signature does not verify against party %d's key — the "+
			"contribution was not produced by that validator", index)
	}
	return nil
}

// Parties implements Authenticator.
func (a *Ed25519Authenticator) Parties() int { return len(a.public) }
