package keeper

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

// Oeffentlicher Zufall von drand (League of Entropy, Netz "quicknet") fuer das
// Los der Praeparatoren in Quantengeld Weg B (forschung/quantengeld/
// DEZENTRAL.md 18, B2). Wer eine Runde einreicht, legt ihre Signatur bei;
// JEDER Knoten prueft sie selbst gegen den oeffentlichen Schluessel des
// Verbunds, der hier fest steht. Der Knoten holt nichts aus dem Netz und
// glaubt keinem Server. Verzerren koennte den Zufall nur eine Mehrheit des
// Verbunds.
//
// Schema "bls-unchained-g1-rfc9380" (drand v2, crypto/schemes.go
// NewPedersenBLSUnchainedG1): Nachricht = SHA-256(Runde, 8 Byte big-endian),
// Signatur auf G1 (48 Byte komprimiert) mit RFC-9380-Hash und dem G1-DST,
// Schluessel auf G2 (96 Byte). Zufall = SHA-256(Signatur).
const (
	drandQuicknetSchluesselHex = "83cf0f2896adee7eb8b5f01fcad3912212c437e0073e911fb90022d3e760183c8c4b450b6a0a6c3ac6a5776a2d1064510d1fec758c921cc22b0e17e63aaf4bcb5ed66304de9cf809bd274ca73bab4af5a6e9c76a4bc09e76eae8991ef5ece45a"
	drandQuicknetGenesis       = int64(1692803367) // Sekunden (Unix), Runde 1
	drandQuicknetPeriode       = int64(3)          // Sekunden je Runde
)

var drandDST = []byte("BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_")

// drandNetz: ein drand-Netz, wie der Knoten es kennt.
type drandNetz struct {
	schluessel bls12381.G2Affine
	genesis    int64
	periode    int64
}

var drandQuicknet = func() drandNetz {
	n, err := drandNetzAus(drandQuicknetSchluesselHex, drandQuicknetGenesis, drandQuicknetPeriode)
	if err != nil {
		panic("drand: Schluessel von quicknet ungueltig: " + err.Error())
	}
	return n
}()

func drandNetzAus(schluesselHex string, genesis, periode int64) (drandNetz, error) {
	roh, err := hex.DecodeString(schluesselHex)
	if err != nil || len(roh) != bls12381.SizeOfG2AffineCompressed {
		return drandNetz{}, errors.New("Schluessel: 96 Byte hex")
	}
	var pk bls12381.G2Affine
	if n, err := pk.SetBytes(roh); err != nil || n != len(roh) {
		return drandNetz{}, fmt.Errorf("Schluessel: kein Punkt der Gruppe (%v)", err)
	}
	if pk.IsInfinity() {
		return drandNetz{}, errors.New("Schluessel: neutrales Element")
	}
	if genesis <= 0 || periode <= 0 {
		return drandNetz{}, errors.New("Genesis oder Periode")
	}
	return drandNetz{schluessel: pk, genesis: genesis, periode: periode}, nil
}

// zeitDerRunde: wann die Runde r erzeugt wurde (Unix-Sekunden).
func (n drandNetz) zeitDerRunde(r uint64) (int64, bool) {
	if r == 0 || r > uint64((1<<62-n.genesis)/n.periode) {
		return 0, false
	}
	return n.genesis + int64(r-1)*n.periode, true
}

// drandNachricht: was der Verbund fuer Runde r unterschreibt.
func drandNachricht(r uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], r)
	h := sha256.Sum256(b[:])
	return h[:]
}

// pruefen: die Signatur der Runde r. Rueckgabe: der Zufall (SHA-256 der
// Signatur). Jede Abweichung -- Laenge, kein Punkt, neutrales Element,
// falsche Runde, falscher Schluessel -- ist ein Fehler (fail-closed).
func (n drandNetz) pruefen(r uint64, sigHex string) ([32]byte, error) {
	var leer [32]byte
	if _, ok := n.zeitDerRunde(r); !ok {
		return leer, errors.New("drand: Runde ausserhalb")
	}
	roh, err := hex.DecodeString(sigHex)
	if err != nil || len(roh) != bls12381.SizeOfG1AffineCompressed {
		return leer, errors.New("drand: Signatur 48 Byte hex")
	}
	var sig bls12381.G1Affine
	if k, err := sig.SetBytes(roh); err != nil || k != len(roh) {
		return leer, errors.New("drand: Signatur ist kein Punkt der Gruppe")
	}
	if sig.IsInfinity() {
		return leer, errors.New("drand: Signatur ist das neutrale Element")
	}
	hm, err := bls12381.HashToG1(drandNachricht(r), drandDST)
	if err != nil {
		return leer, errors.New("drand: Hash auf die Kurve")
	}
	// e(sig, g2) == e(H(m), pk)  <=>  e(sig, g2) * e(-H(m), pk) == 1
	_, _, _, g2 := bls12381.Generators()
	var minusH bls12381.G1Affine
	minusH.Neg(&hm)
	ok, err := bls12381.PairingCheck([]bls12381.G1Affine{sig, minusH}, []bls12381.G2Affine{g2, n.schluessel})
	if err != nil || !ok {
		return leer, errors.New("drand: Signatur passt nicht zu Runde und Schluessel")
	}
	return sha256.Sum256(roh), nil
}
