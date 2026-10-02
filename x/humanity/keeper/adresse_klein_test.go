package keeper

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// Dasselbe Ergebnis wie der alte Weg -- fuer jede Adresse, auch fuer solche
// mit Ziffern und Buchstaben an jeder Stelle (sonst wichen Kontoschluessel ab).
func TestAdresseKlein_GleichWieToLowerHex(t *testing.T) {
	faelle := []common.Address{{}, common.HexToAddress("0xFFfFfFffFFfffFFfFFfFFFFFffFFFffffFfFFFfF"), common.HexToAddress("0x52908400098527886E0F7030069857D2E4169EE7")}
	for i := 0; i < 2000; i++ {
		var a common.Address
		if _, err := rand.Read(a[:]); err != nil {
			t.Fatal(err)
		}
		faelle = append(faelle, a)
	}
	for _, a := range faelle {
		if got, want := adresseKlein(a), strings.ToLower(a.Hex()); got != want {
			t.Fatalf("%x: %q statt %q", a[:], got, want)
		}
	}
}

func BenchmarkAdresseKlein(b *testing.B) {
	a := common.HexToAddress("0x52908400098527886E0F7030069857D2E4169EE7")
	b.Run("neu", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = adresseKlein(a)
		}
	})
	b.Run("alt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = strings.ToLower(a.Hex())
		}
	})
}
