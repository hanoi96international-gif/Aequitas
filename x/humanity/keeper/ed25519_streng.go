package keeper

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"strings"

	"filippo.io/edwards25519"
)

// Strenge Ed25519-Pruefung fuer Schluessel, die von aussen kommen
// (Sicherheitspruefung #300, MEDIUM-2).
//
// WARUM
//
// crypto/ed25519.Verify prueft ohne Kofaktor und nimmt JEDEN oeffentlichen
// Schluessel, der sich als Punkt lesen laesst -- auch einen kleiner Ordnung.
// Mit dem neutralen Element (0100..00) als Schluessel gilt die Unterschrift
// R = Basispunkt, s = 1 fuer JEDE Nachricht: [1]B = B + [k]·0. Wer so einen
// Schluessel eintraegt, hat eine Universalunterschrift -- fuer die
// Besitznachweise und, seit die Bescheinigung ihre Bindung selbst traegt
// (grant_staffel.go), fuer jede Erneuerung.
//
// WAS
//
// Ein Schluessel taugt nur, wenn er
//   - kanonisch kodiert ist (dieselben 32 Bytes kommen beim Zurueckschreiben
//     heraus -- SetBytes nimmt auch nicht-kanonische y),
//   - nicht kleiner Ordnung ist ([8]A ist nicht das neutrale Element) und
//   - in der Untergruppe der Primordnung liegt ([L]A ist das neutrale
//     Element; sonst gaelten fuer denselben Schluessel je nach Pruefer
//     verschiedene Unterschriften).
//
// Jeder echte Schluessel ([a]B mit geklemmtem a) erfuellt alle drei. Die
// Unterschrift selbst muss klein geschriebenes Hex ohne 0x sein -- genau eine
// Schreibweise, damit dieselbe Bescheinigung nicht unter mehreren
// Transaktions-Hashes in Bloecke kommt. s < L prueft crypto/ed25519 selbst.
//
// Die Bibliothek steht schon im Modulgraphen (indirekt), sie kommt nicht neu
// hinzu.

// ed25519OrdnungMinusEins: L-1, little-endian. [L-1]A + A = [L]A.
var ed25519OrdnungMinusEins = func() *edwards25519.Scalar {
	b, _ := hex.DecodeString("ecd3f55c1a631258d69cf7a2def9de1400000000000000000000000000000010")
	s, err := edwards25519.NewScalar().SetCanonicalBytes(b)
	if err != nil {
		panic("ed25519OrdnungMinusEins: " + err.Error())
	}
	return s
}()

// ed25519SchluesselTauglich: kanonisch, nicht kleiner Ordnung, Primordnung.
func ed25519SchluesselTauglich(pub []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	p, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil || !bytes.Equal(p.Bytes(), pub) {
		return false
	}
	null := edwards25519.NewIdentityPoint()
	if new(edwards25519.Point).MultByCofactor(p).Equal(null) == 1 {
		return false
	}
	l := new(edwards25519.Point).ScalarMult(ed25519OrdnungMinusEins, p)
	return l.Add(l, p).Equal(null) == 1
}

// kleinHex: nur 0-9 und a-f.
func kleinHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// ed25519SigNormal: die eine Schreibweise -- klein, ohne 0x, ohne Rand.
// Alles andere (falsche Laenge, Fremdzeichen) bleibt und faellt in
// ed25519PruefenStreng durch.
func ed25519SigNormal(sig string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(sig)), "0x")
}

// ed25519PruefenStreng: Schluessel (64 Hex klein) taugt, Unterschrift ist
// 128 Hex klein, und sie passt zur Nachricht.
func ed25519PruefenStreng(pubHex, sigHex string, msg []byte) bool {
	if len(pubHex) != 2*ed25519.PublicKeySize || !kleinHex(pubHex) ||
		len(sigHex) != 2*ed25519.SignatureSize || !kleinHex(sigHex) {
		return false
	}
	pub, err := hex.DecodeString(pubHex)
	if err != nil || !ed25519SchluesselTauglich(pub) {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}
