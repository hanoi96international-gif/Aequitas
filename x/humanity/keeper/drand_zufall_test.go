package keeper

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

// Ein Testverbund mit bekanntem Schluessel: signieren wie drand quicknet
// (sk * H(SHA-256(Runde)) auf G1, Schluessel sk * g2), pruefen mit pruefen.
func drandTestNetz(t *testing.T, sk int64) (drandNetz, func(r uint64) string) {
	t.Helper()
	_, _, _, g2 := bls12381.Generators()
	var pk bls12381.G2Affine
	pk.ScalarMultiplication(&g2, big.NewInt(sk))
	b := pk.Bytes()
	n, err := drandNetzAus(hex.EncodeToString(b[:]), drandQuicknetGenesis, drandQuicknetPeriode)
	if err != nil {
		t.Fatal(err)
	}
	signiere := func(r uint64) string {
		hm, err := bls12381.HashToG1(drandNachricht(r), drandDST)
		if err != nil {
			t.Fatal(err)
		}
		var s bls12381.G1Affine
		s.ScalarMultiplication(&hm, big.NewInt(sk))
		sb := s.Bytes()
		return hex.EncodeToString(sb[:])
	}
	return n, signiere
}

func TestDrand_QuicknetSchluesselGueltig(t *testing.T) {
	if drandQuicknet.schluessel.IsInfinity() || !drandQuicknet.schluessel.IsInSubGroup() {
		t.Fatal("quicknet-Schluessel kein gueltiger Punkt")
	}
	if z, ok := drandQuicknet.zeitDerRunde(1); !ok || z != drandQuicknetGenesis {
		t.Fatalf("Runde 1: %d", z)
	}
	if z, _ := drandQuicknet.zeitDerRunde(1001); z != drandQuicknetGenesis+3000 {
		t.Fatalf("Runde 1001: %d", z)
	}
}

func TestDrand_GutfallUndMissbrauch(t *testing.T) {
	n, sig := drandTestNetz(t, 0x5eed1234)
	z, err := n.pruefen(7, sig(7))
	if err != nil {
		t.Fatalf("Gutfall: %v", err)
	}
	if z2, _ := n.pruefen(7, sig(7)); z2 != z {
		t.Fatal("Zufall nicht deterministisch")
	}
	fremd, fsig := drandTestNetz(t, 0x0badbeef)
	_ = fremd
	nullHex := strings.Repeat("00", 48)
	var unendlich bls12381.G1Affine
	ub := unendlich.Bytes()
	for _, f := range []struct {
		name string
		r    uint64
		s    string
	}{
		{"Signatur einer anderen Runde", 8, sig(7)},
		{"Signatur eines anderen Schluessels", 7, fsig(7)},
		{"Runde 0", 0, sig(0)},
		{"kein Hex", 7, "zz" + sig(7)[2:]},
		{"zu kurz", 7, sig(7)[:94]},
		{"zu lang", 7, sig(7) + "00"},
		{"lauter Nullen", 7, nullHex},
		{"neutrales Element", 7, hex.EncodeToString(ub[:])},
		{"ein Bit geaendert", 7, kippeLetztesHex(sig(7))},
	} {
		if _, err := n.pruefen(f.r, f.s); err == nil {
			t.Errorf("%s: angenommen", f.name)
		}
	}
	if _, err := drandNetzAus(strings.Repeat("00", 96), 1, 3); err == nil {
		t.Error("Schluessel aus Nullen angenommen")
	}
	if _, err := drandNetzAus(drandQuicknetSchluesselHex[:190], 1, 3); err == nil {
		t.Error("zu kurzer Schluessel angenommen")
	}
}

func kippeLetztesHex(s string) string {
	b, _ := hex.DecodeString(s)
	b[len(b)-1] ^= 1
	return hex.EncodeToString(b)
}
