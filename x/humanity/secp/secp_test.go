package secp

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	gethsecp "github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// Diese Testdatei linkt beide Kopien von libsecp256k1 in ein Programm --
// faehrt sie an, gibt es kein doppeltes Symbol (umbenennen.h).

func zufall(t testing.TB, n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func gleich(t *testing.T, name string, nachricht, sig []byte) {
	t.Helper()
	alt, altErr := crypto.Ecrecover(nachricht, sig)
	neu, neuErr := Wiederherstellen(nachricht, sig)
	if (altErr == nil) != (neuErr == nil) {
		t.Fatalf("%s: go-ethereum err=%v, hier err=%v (sig %x)", name, altErr, neuErr, sig)
	}
	if !bytes.Equal(alt, neu) {
		t.Fatalf("%s: verschiedene Schluessel\n go-ethereum %x\n hier        %x", name, alt, neu)
	}
}

// Gutfall: echte Signaturen ergeben bitgleich denselben Schluessel.
func TestWiederherstellen_WieGoEthereum(t *testing.T) {
	for i := 0; i < 2000; i++ {
		k, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		h := zufall(t, 32)
		sig, err := crypto.Sign(h, k)
		if err != nil {
			t.Fatal(err)
		}
		gleich(t, "echt", h, sig)
		pub, err := Wiederherstellen(h, sig)
		if err != nil || !bytes.Equal(pub, crypto.FromECDSAPub(&k.PublicKey)) {
			t.Fatalf("falscher Schluessel: %x, %v", pub, err)
		}
	}
}

// Missbrauch: Faelschungen und kaputte Eingaben -- dieselbe Entscheidung
// wie go-ethereum, nie ein Schluessel, wo dort ein Fehler steht.
func TestWiederherstellen_MissbrauchWieGoEthereum(t *testing.T) {
	n := gethsecp.S256().Params().N
	k, _ := crypto.GenerateKey()
	h := zufall(t, 32)
	echt, _ := crypto.Sign(h, k)

	setze := func(sig []byte, von int, x *big.Int) {
		b := x.Bytes()
		if len(b) > 32 {
			b = b[len(b)-32:]
		}
		for i := 0; i < 32; i++ {
			sig[von+i] = 0
		}
		copy(sig[von+32-len(b):von+32], b)
	}
	variante := func(f func(sig []byte)) []byte {
		s := append([]byte(nil), echt...)
		f(s)
		return s
	}
	faelle := map[string][]byte{
		"r=0":           variante(func(s []byte) { setze(s, 0, big.NewInt(0)) }),
		"s=0":           variante(func(s []byte) { setze(s, 32, big.NewInt(0)) }),
		"r=n":           variante(func(s []byte) { setze(s, 0, n) }),
		"s=n":           variante(func(s []byte) { setze(s, 32, n) }),
		"r=n+1":         variante(func(s []byte) { setze(s, 0, new(big.Int).Add(n, big.NewInt(1))) }),
		"r=2^256-1":     variante(func(s []byte) { setze(s, 0, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))) }),
		"s hoch":        variante(func(s []byte) { setze(s, 32, new(big.Int).Sub(n, new(big.Int).SetBytes(s[32:64]))) }),
		"v=1-v":         variante(func(s []byte) { s[64] ^= 1 }),
		"v=2":           variante(func(s []byte) { s[64] = 2 }),
		"v=3":           variante(func(s []byte) { s[64] = 3 }),
		"v=4":           variante(func(s []byte) { s[64] = 4 }),
		"v=27":          variante(func(s []byte) { s[64] = 27 }),
		"bit gekippt r": variante(func(s []byte) { s[5] ^= 0x10 }),
		"bit gekippt s": variante(func(s []byte) { s[40] ^= 0x01 }),
	}
	for name, sig := range faelle {
		gleich(t, name, h, sig)
	}
	for i := 0; i < 3000; i++ {
		sig := zufall(t, 65)
		sig[64] %= 4
		gleich(t, "zufall", h, sig)
	}
	for _, l := range []int{0, 31, 33} {
		if _, err := Wiederherstellen(make([]byte, l), echt); err == nil {
			t.Fatalf("Nachricht mit %d Byte angenommen", l)
		}
	}
	for _, l := range []int{0, 64, 66} {
		if _, err := Wiederherstellen(h, make([]byte, l)); err == nil {
			t.Fatalf("Signatur mit %d Byte angenommen", l)
		}
	}
}

func BenchmarkWiederherstellen(b *testing.B) {
	k, _ := crypto.GenerateKey()
	h := zufall(b, 32)
	sig, _ := crypto.Sign(h, k)
	b.Run("go-ethereum", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = crypto.Ecrecover(h, sig)
			}
		})
	})
	b.Run("hier", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = Wiederherstellen(h, sig)
			}
		})
	})
}

// Ein Kontext fuer alle: gleichzeitige Aufrufe liefern weiter richtige
// Schluessel (unter -race in CI).
func TestWiederherstellen_Gleichzeitig(t *testing.T) {
	type fall struct{ h, sig, pub []byte }
	faelle := make([]fall, 16)
	for i := range faelle {
		k, _ := crypto.GenerateKey()
		h := zufall(t, 32)
		sig, _ := crypto.Sign(h, k)
		faelle[i] = fall{h, sig, crypto.FromECDSAPub(&k.PublicKey)}
	}
	fehler := make(chan string, 8)
	fertig := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func(g int) {
			defer func() { fertig <- struct{}{} }()
			for i := 0; i < 200; i++ {
				f := faelle[(g+i)%len(faelle)]
				if pub, err := Wiederherstellen(f.h, f.sig); err != nil || !bytes.Equal(pub, f.pub) {
					fehler <- "falscher Schluessel"
					return
				}
			}
		}(g)
	}
	for g := 0; g < 8; g++ {
		<-fertig
	}
	select {
	case e := <-fehler:
		t.Fatal(e)
	default:
	}
}
