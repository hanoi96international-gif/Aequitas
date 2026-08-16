package keeper

import (
	"fmt"
	"math/big"
	"strings"
)

// A nullifier is the one thing standing between "one human, one registration"
// and unlimited money. It has to have exactly ONE representation.
//
// It did not. Until this file, the nullifier was stored and looked up as the
// raw string the caller sent, while the V7 contract keys usedNullifiers by the
// NUMERIC value (bytes32(pubSignals[1]), AequitasV7.sol:199-203). Nothing
// normalised in between. So every one of these claimed a separate registration
// for the same biometric — measured, not theorised, each one granting another
// 1,000 AEQ through the public /api/register endpoint:
//
//	12857392037…      the honest value
//	012857392037…     one leading zero
//	0012857392037…    two
//	" 12857392037…"   a leading space
//	"12857392037… "   a trailing space, newline or tab
//	0x1F4…            the same value in hex
//	0X1F4… / 0x1f4…   either case
//	1f4…              hex without the prefix
//
// canonicalNullifier collapses all of them onto one key: the value's decimal
// string. Parsing follows registerOnV7's own encoder (register.go, "Encode
// nullifier as bytes32") so the key space and the contract agree on what a
// nullifier IS — an explicit 0x prefix or any hex letter means hex, anything
// else is the decimal pubSignals[1] the v2 circuit emits.
//
// It is deliberately strict about emptiness and garbage: a nullifier that
// cannot be parsed must never silently become a fresh key, which is exactly how
// the padded variants got in.
func canonicalNullifier(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty nullifier")
	}

	n := new(big.Int)
	hasHexPrefix := strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X")
	body := s
	if hasHexPrefix {
		body = s[2:]
	}
	if body == "" {
		return "", fmt.Errorf("nullifier %q has a hex prefix and no digits", raw)
	}

	if hasHexPrefix || containsHexLetter(body) {
		if _, ok := n.SetString(body, 16); !ok {
			return "", fmt.Errorf("nullifier %q is not a valid hex integer", raw)
		}
	} else if _, ok := n.SetString(body, 10); !ok {
		return "", fmt.Errorf("nullifier %q is not a valid decimal integer", raw)
	}

	if n.Sign() < 0 {
		return "", fmt.Errorf("nullifier %q is negative", raw)
	}
	if n.Sign() == 0 {
		// Zero is what an unset mapping slot reads as in the contract, so a
		// zero nullifier would be indistinguishable from "never used".
		return "", fmt.Errorf("nullifier %q is zero", raw)
	}
	return n.String(), nil
}

func containsHexLetter(s string) bool {
	for _, c := range s {
		if (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			return true
		}
	}
	return false
}

// nullifierMatchesProof reports whether a nullifier is the one the ZK proof
// actually commits to.
//
// The replay path verified the Groth16 proof and, separately, claimed
// tx.Nullifier — without ever checking that the two describe the same thing.
// The proof is published inside the block and is not wallet-bound (see
// AequitasV7.sol's own comment), so a copied proof plus an attacker's own
// signature and a nullifier of their choosing was accepted. Binding the two
// closes that, and it needs no new transaction field: pubSignals[1] IS the
// nullifier for the v2 circuit.
//
// pubSignals shorter than 2 entries means there is nothing to bind against —
// the caller decides what to do with that (block.go already hard-fails such a
// transaction for other reasons).
func nullifierMatchesProof(nullifier string, pubSignals []string) (bool, error) {
	if len(pubSignals) < 2 {
		return false, fmt.Errorf("proof carries no nullifier signal")
	}
	got, err := canonicalNullifier(nullifier)
	if err != nil {
		return false, err
	}
	want, err := canonicalNullifier(pubSignals[1])
	if err != nil {
		return false, fmt.Errorf("pubSignals[1] is not a usable nullifier: %w", err)
	}
	return got == want, nil
}

// nullifierBytes32 renders a nullifier as the 32-byte big-endian value the V7
// contract keys usedNullifiers by.
//
// This is also what MigrateEVMFromGoState must write. It used to call
// common.HexToHash on a DECIMAL string: every decimal digit is a valid hex
// digit, so the parse silently succeeded and produced an entirely different
// number, and the rebuilt mapping landed at a slot the contract never reads.
// Since that migration runs after every snapshot import, every resync and every
// contract upgrade, the contract-level "nullifier already used" check read
// address(0) for every real nullifier on any node that had been through one.
func nullifierBytes32(nullifier string) ([32]byte, error) {
	var out [32]byte
	canon, err := canonicalNullifier(nullifier)
	if err != nil {
		return out, err
	}
	n, ok := new(big.Int).SetString(canon, 10)
	if !ok {
		return out, fmt.Errorf("canonical nullifier %q is not decimal", canon)
	}
	b := n.Bytes()
	if len(b) > 32 {
		return out, fmt.Errorf("nullifier %q exceeds 32 bytes", nullifier)
	}
	copy(out[32-len(b):], b)
	return out, nil
}
