package keeper

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

// Mehrfach-Beweis (quanten_s3_beweis.go) gleich wie die Spezifikation
// (s_kette.py --vektor-s3-beweis), und jede Verfaelschung abgewiesen.

func TestQuantenS3Beweis_Vektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s3_beweis_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Faelle []struct {
			N        int      `json:"n"`
			Idx      []int    `json:"idx"`
			Blaetter []string `json:"blaetter"`
			Wurzel   string   `json:"wurzel"`
			Beweis   []string `json:"beweis"`
		} `json:"faelle"`
	}
	if err := json.Unmarshal(roh, &v); err != nil || len(v.Faelle) < 8 {
		t.Fatalf("Vektor: %v", err)
	}
	h32 := func(s string) [32]byte {
		var o [32]byte
		b, _ := hex.DecodeString(s)
		copy(o[:], b)
		return o
	}
	for _, f := range v.Faelle {
		var bl, bw [][32]byte
		for _, s := range f.Blaetter {
			bl = append(bl, h32(s))
		}
		for _, s := range f.Beweis {
			bw = append(bw, h32(s))
		}
		if !quantenS3MehrfachPruefen(h32(f.Wurzel), f.N, f.Idx, bl, bw) {
			t.Fatalf("n %d, %d Blaetter: Beweis der Spezifikation abgewiesen", f.N, len(f.Idx))
		}
	}
}

func s3BeweisBlaetter(n int, seed int64) [][32]byte {
	r := rand.New(rand.NewSource(seed))
	bl := make([][32]byte, n)
	for i := range bl {
		bl[i] = quantenWPruefsumme("7a7a", i, new(big.Int).Rand(r, new(big.Int).Lsh(big.NewInt(1), 126)))
	}
	return bl
}

func TestQuantenS3Beweis_RundlaufUndAngriffe(t *testing.T) {
	for _, n := range []int{1, 2, 3, 6, 7, 1000, 1025} {
		bl := s3BeweisBlaetter(n, int64(n))
		w := quantenWMerkleWurzel(bl)
		r := rand.New(rand.NewSource(int64(n) * 3))
		for _, idx := range [][]int{{0}, {n - 1}, eindeutigSortiert(r.Perm(n)[:min(n, 9)]), func() []int {
			a := make([]int, n)
			for i := range a {
				a[i] = i
			}
			return a
		}()} {
			bw, ok := quantenS3Mehrfachbeweis(bl, idx)
			var b []([32]byte)
			for _, i := range idx {
				b = append(b, bl[i])
			}
			if !ok || !quantenS3MehrfachPruefen(w, n, idx, b, bw) {
				t.Fatalf("n %d, %d Blaetter: Rundlauf", n, len(idx))
			}
			if len(idx) == 1 && len(bw) > len(quantenWMerklePfad(bl, idx[0])) {
				t.Fatal("ein Blatt: laenger als der einfache Pfad")
			}
		}
	}
	bl := s3BeweisBlaetter(1000, 99)
	w := quantenWMerkleWurzel(bl)
	idx := []int{3, 500, 999}
	bw, _ := quantenS3Mehrfachbeweis(bl, idx)
	gut := [][32]byte{bl[3], bl[500], bl[999]}
	falsch := map[string]func() bool{
		"ein Hash zu wenig": func() bool { return quantenS3MehrfachPruefen(w, 1000, idx, gut, bw[:len(bw)-1]) },
		"ein Hash zu viel": func() bool {
			return quantenS3MehrfachPruefen(w, 1000, idx, gut, append(append([][32]byte{}, bw...), bw[0]))
		},
		"Hash vertauscht": func() bool {
			b := append([][32]byte{}, bw...)
			b[0][0] ^= 1
			return quantenS3MehrfachPruefen(w, 1000, idx, gut, b)
		},
		"anderer Index": func() bool { return quantenS3MehrfachPruefen(w, 1000, []int{3, 501, 999}, gut, bw) },
		"Indizes nicht aufsteigend": func() bool {
			return quantenS3MehrfachPruefen(w, 1000, []int{500, 3, 999}, [][32]byte{gut[1], gut[0], gut[2]}, bw)
		},
		"Index doppelt":                 func() bool { return quantenS3MehrfachPruefen(w, 1000, []int{3, 3, 999}, gut, bw) },
		"anderes N":                     func() bool { return quantenS3MehrfachPruefen(w, 1001, idx, gut, bw) },
		"N ueber der Grenze":            func() bool { return quantenS3MehrfachPruefen(w, quantenS3PulsMax+1, idx, gut, bw) },
		"falsches Blatt":                func() bool { return quantenS3MehrfachPruefen(w, 1000, idx, [][32]byte{bl[4], gut[1], gut[2]}, bw) },
		"Index ausserhalb":              func() bool { return quantenS3MehrfachPruefen(w, 1000, []int{3, 500, 1000}, gut, bw) },
		"Blaetter und Indizes ungleich": func() bool { return quantenS3MehrfachPruefen(w, 1000, idx, gut[:2], bw) },
		"keine Indizes":                 func() bool { return quantenS3MehrfachPruefen(w, 1000, nil, nil, bw) },
	}
	for name, f := range falsch {
		if f() {
			t.Fatalf("%s: angenommen", name)
		}
	}
	if _, ok := quantenS3Mehrfachbeweis(bl, []int{5, 5}); ok {
		t.Fatal("Beweis fuer doppelte Indizes gebaut")
	}
	// Genau eine Form: der Beweis haengt nur von Blaettern und Indizes ab.
	bw2, _ := quantenS3Mehrfachbeweis(bl, idx)
	if !reflect.DeepEqual(bw, bw2) {
		t.Fatal("Beweis nicht eindeutig")
	}
}
