package keeper

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Das Los im Knoten ist Byte fuer Byte das der Spezifikation
// (python3 -I forschung/quantengeld/s_weg_b.py --vektor).
func TestQuantenSLos_GleichWieSpezifikation(t *testing.T) {
	roh, err := os.ReadFile(filepath.Join("testdata", "quanten_s_los_vektor.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Abbild []string `json:"abbild"`
		Zufall string   `json:"zufall"`
		Faelle []struct {
			K   string   `json:"K"`
			V   int      `json:"v"`
			Los []string `json:"los"`
		} `json:"faelle"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	zb, _ := hex.DecodeString(v.Zufall)
	var z [32]byte
	copy(z[:], zb)
	if len(v.Faelle) != 3 {
		t.Fatalf("Vektor: %d Faelle", len(v.Faelle))
	}
	for _, f := range v.Faelle {
		los, err := quantenSLos(v.Abbild, z, f.K, f.V)
		if err != nil || !reflect.DeepEqual(los, f.Los) {
			t.Fatalf("v = %d: Knoten %v (%v), Spezifikation %v", f.V, los, err, f.Los)
		}
	}
	// Anderer Zufall, anderes Los; Reihenfolge des Abbilds egal.
	z2 := z
	z2[0] ^= 1
	if a, _ := quantenSLos(v.Abbild, z2, v.Faelle[1].K, 7); reflect.DeepEqual(a, v.Faelle[1].Los) {
		t.Fatal("Los haengt nicht vom Zufall ab")
	}
	umgedreht := make([]string, len(v.Abbild))
	for i, k := range v.Abbild {
		umgedreht[len(v.Abbild)-1-i] = k
	}
	if a, _ := quantenSLos(umgedreht, z, v.Faelle[1].K, 7); !reflect.DeepEqual(a, v.Faelle[1].Los) {
		t.Fatal("Los haengt von der Reihenfolge des Abbilds ab")
	}
	// Fail-closed.
	for name, f := range map[string]func() error{
		"v zu klein":        func() error { _, e := quantenSLos(v.Abbild, z, "k", quantenSLosVMin-1); return e },
		"v zu gross":        func() error { _, e := quantenSLos(v.Abbild, z, "k", quantenSLosVMax+1); return e },
		"Register zu klein": func() error { _, e := quantenSLos(v.Abbild[:20], z, "k", 7); return e },
		"Konto doppelt": func() error {
			_, e := quantenSLos(append(append([]string{}, v.Abbild...), v.Abbild[0]), z, "k", 4)
			return e
		},
		"Konto nicht kanonisch": func() error {
			_, e := quantenSLos(append(append([]string{}, v.Abbild[:39]...), "0xABC"), z, "k", 4)
			return e
		},
	} {
		if f() == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
}
