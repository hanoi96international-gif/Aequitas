package keeper

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

// Weg C (C1): Knoten und Spezifikation rechnen die Pruefgroesse ueber eine
// echte Leitung gleich (testdata/quanten_s_leitung_vektor.json aus
// wiesner_kette.py --vektor-leitung).
func TestQuantenSLeitung_Vektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_leitung_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Stufen  []int                       `json:"theta_stufen"`
		FreiMax int                         `json:"frei_ppm_max"`
		Tabelle map[string]map[string][]int `json:"k_leitung_tabelle"`
		Faelle  [][6]int                    `json:"k_leitung"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Stufen) != len(quantenSThetaStufen) || v.FreiMax != quantenSFreiPpmMax {
		t.Fatalf("Stufen/Grenze: %v %d", v.Stufen, v.FreiMax)
	}
	for i, st := range v.Stufen {
		if quantenSThetaStufen[i] != st {
			t.Fatalf("Stufe %d: %d statt %d", i, quantenSThetaStufen[i], st)
		}
		zeilen := v.Tabelle[strconv.Itoa(st)]
		if len(zeilen) != len(quantenWKTabelle) {
			t.Fatalf("Stufe %d: %d Toleranzen", st, len(zeilen))
		}
		for tolS, z := range zeilen {
			tol, _ := strconv.Atoi(tolS)
			var ist [9]int
			var ok bool
			if st == 0 {
				ist, ok = quantenWKTabelle[tol]
			} else {
				ist, ok = quantenSKLeitungTabelle[st][tol]
			}
			if !ok || len(z) != 9 {
				t.Fatalf("Stufe %d tol %d fehlt", st, tol)
			}
			for f := range z {
				if ist[f] != z[f] {
					t.Errorf("Tabelle[%d][%d][%d] = %d, Spezifikation %d", st, tol, f, ist[f], z[f])
				}
			}
		}
	}
	if len(quantenSKLeitungTabelle) != len(v.Stufen)-1 {
		t.Fatalf("Knoten hat %d Leitungs-Stufen", len(quantenSKLeitungTabelle))
	}
	for _, f := range v.Faelle {
		if k := quantenSKLeitung(f[0], f[1], f[2], f[3], f[4]); k != f[5] {
			t.Errorf("quantenSKLeitung%v = %d, Spezifikation %d", f[:5], k, f[5])
		}
	}
}

// Missbrauch: gemessene Werte werden nie zugunsten des Faelschers gerundet,
// und jede Eingabe ausserhalb der Grenzen ist "nicht sicher" (0).
func TestQuantenSLeitung_AufrundenUndGrenzen(t *testing.T) {
	// 2,1 Grad gilt als 5 Grad, nicht als 2.
	if quantenSKLeitung(4, 1, 1, 21, 0) != quantenSKLeitungTabelle[5][4][0] || quantenSKLeitung(4, 1, 1, 20, 0) != quantenSKLeitungTabelle[2][4][0] {
		t.Fatal("theta nicht aufgerundet")
	}
	// Ein ppm ueber einem Sechzehntel zaehlt als zwei Sechzehntel.
	if quantenSKLeitung(6, 1, 1, 50, 62_501) != quantenSKLeitungTabelle[5][6][2] || quantenSKLeitung(6, 1, 1, 50, 62_500) != quantenSKLeitungTabelle[5][6][1] {
		t.Fatal("frei nicht aufgerundet")
	}
	// Mehr Verkippung verlangt nie weniger Stellen.
	for _, tol := range []int{2, 3, 4, 5, 6, 8, 10} {
		for f := 0; f < 9; f++ {
			vorher := quantenWKTabelle[tol][f]
			for _, st := range quantenSThetaStufen[1:] {
				k := quantenSKLeitungTabelle[st][tol][f]
				if k != 0 && (vorher == 0 || k < vorher) {
					t.Fatalf("tol %d f %d: %d Grad prueft %d < %d", tol, f, st, k, vorher)
				}
				vorher = k
			}
		}
	}
	for _, f := range [][5]int{
		{4, 1, 1, -1, 0}, {4, 1, 1, 101, 0}, {4, 1, 1, 0, -1}, {4, 1, 1, 0, quantenSFreiPpmMax + 1},
		{4, 0, 1, 0, 0}, {4, 2, 1, 0, 0}, {4, 1, 0, 0, 0}, {4, 1, quantenWVMax + 1, 0, 0}, {7, 1, 1, 0, 0},
		{10, 1, 1, 30, 0}, {4, 4, 7, 100, 200_000},
	} {
		if k := quantenSKLeitung(f[0], f[1], f[2], f[3], f[4]); k != 0 {
			t.Errorf("quantenSKLeitung%v = %d, erwartet 0 (nicht sicher)", f, k)
		}
	}
	// Ohne Leitung (0 Grad, nichts frei) gilt genau die Tabelle des Chips.
	for _, f := range [][3]int{{4, 1, 1}, {4, 3, 5}, {2, 9, 16}, {6, 2, 64}} {
		if quantenSKLeitung(f[0], f[1], f[2], 0, 0) != quantenWKNote(f[0], f[1], f[2]) {
			t.Fatalf("ohne Leitung %v anders als quantenWKNote", f)
		}
	}
}
