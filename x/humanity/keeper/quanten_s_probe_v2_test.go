package keeper

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// Stichprobe v2 (quanten_s_probe_v2.go) gegen die Spezifikation
// (s_kette.py --vektor-probe-v2) und Missbrauch beim Nachrechnen.

type qsp2Vektor struct {
	K       string `json:"K"`
	N       int    `json:"n"`
	NVal    int    `json:"n_val"`
	Stellen []struct {
		B          int      `json:"b"`
		Stichprobe []int    `json:"stichprobe"`
		Salze      []string `json:"salze"`
		Teile      []string `json:"teile"`
		Bindung    string   `json:"bindung"`
	} `json:"stellen"`
	TeilEinzeln struct {
		B       int    `json:"b"`
		J       int    `json:"j"`
		Salz    string `json:"salz"`
		Stellen []int  `json:"stellen"`
		Hash    string `json:"hash"`
	} `json:"teil_einzeln"`
}

func qsp2Hex32(t *testing.T, s string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("hex %q", s)
	}
	var out [32]byte
	copy(out[:], b)
	return out
}

func qsp2Laden(t *testing.T) qsp2Vektor {
	t.Helper()
	roh, err := os.ReadFile("testdata/quanten_s_probe_v2_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qsp2Vektor
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Stellen) != 2 {
		t.Fatal("Vektor: zwei Stellen")
	}
	return v
}

func TestQuantenSProbeV2_Vektor(t *testing.T) {
	v := qsp2Laden(t)
	e := v.TeilEinzeln
	if h := quantenSProbeTeilHash(v.K, e.B, e.J, qsp2Hex32(t, e.Salz), e.Stellen); hex.EncodeToString(h[:]) != e.Hash {
		t.Fatalf("Teil-Hash einzeln: %x", h)
	}
	// Die Reihenfolge der Stellen zaehlt nicht (sortiert wie die Spezifikation).
	rueck := []int{e.Stellen[2], e.Stellen[0], e.Stellen[1]}
	if h := quantenSProbeTeilHash(v.K, e.B, e.J, qsp2Hex32(t, e.Salz), rueck); hex.EncodeToString(h[:]) != e.Hash {
		t.Fatalf("Teil-Hash einzeln, unsortiert: %x", h)
	}
	for _, st := range v.Stellen {
		var salze, teile [][32]byte
		for j := range st.Salze {
			salze = append(salze, qsp2Hex32(t, st.Salze[j]))
			teile = append(teile, qsp2Hex32(t, st.Teile[j]))
		}
		// Je Abschnitt der Teil-Hash wie in der Spezifikation.
		for j := 1; j <= v.NVal; j++ {
			von, bis, _ := quantenSAbschnitt(v.N, v.NVal, j)
			var dort []int
			for _, p := range st.Stichprobe {
				if p >= von && p < bis {
					dort = append(dort, p)
				}
			}
			if h := quantenSProbeTeilHash(v.K, st.B, j, salze[j-1], dort); h != teile[j-1] {
				t.Fatalf("Stelle %d, Abschnitt %d: Teil-Hash %x", st.B, j, h)
			}
		}
		bindung, err := quantenSProbeHashV2(v.K, st.B, teile)
		if err != nil || hex.EncodeToString(bindung[:]) != st.Bindung {
			t.Fatalf("Stelle %d: Bindung %x (%v)", st.B, bindung, err)
		}
		// Nachrechnen wie die Kette, auch mit unsortierter Stichprobe.
		umgekehrt := append([]int(nil), st.Stichprobe...)
		for i, k := 0, len(umgekehrt)-1; i < k; i, k = i+1, k-1 {
			umgekehrt[i], umgekehrt[k] = umgekehrt[k], umgekehrt[i]
		}
		if got, err := quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, umgekehrt, salze); err != nil || got != bindung {
			t.Fatalf("Stelle %d: Nachrechnen %x (%v)", st.B, got, err)
		}
	}
}

func TestQuantenSProbeV2_Missbrauch(t *testing.T) {
	v := qsp2Laden(t)
	st, andere := v.Stellen[1], v.Stellen[0]
	var salze, salzeAndere [][32]byte
	for j := range st.Salze {
		salze = append(salze, qsp2Hex32(t, st.Salze[j]))
		salzeAndere = append(salzeAndere, qsp2Hex32(t, andere.Salze[j]))
	}
	soll := qsp2Hex32(t, st.Bindung)
	falschesSalz := append([][32]byte(nil), salze...)
	falschesSalz[2][0] ^= 1
	mehr := append(append([]int(nil), st.Stichprobe...), andere.Stichprobe[0])
	// Rechnet die Kette eine andere Bindung (oder einen Fehler), weist sie ab.
	for name, f := range map[string]func() ([32]byte, error){
		"Teilmenge": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe[1:], salze)
		},
		"eine Stelle mehr": func() ([32]byte, error) { return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, mehr, salze) },
		"falsches Salz": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe, falschesSalz)
		},
		"Salz der anderen Stelle": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe, salzeAndere)
		},
		"Stichprobe der anderen Stelle": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, andere.Stichprobe, salzeAndere)
		},
		"andere Stelle b": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, 0, v.N, v.NVal, st.Stichprobe, salze)
		},
		"anderes K": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K[1:]+"0", st.B, v.N, v.NVal, st.Stichprobe, salze)
		},
		"Salze vertauscht": func() ([32]byte, error) {
			s2 := append([][32]byte(nil), salze...)
			s2[0], s2[1] = s2[1], s2[0]
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe, s2)
		},
		"ein Salz zu wenig": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe, salze[:3])
		},
		"Stelle doppelt": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, append(append([]int(nil), st.Stichprobe...), st.Stichprobe[0]), salze)
		},
		"Stelle ausserhalb": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, append(append([]int(nil), st.Stichprobe...), v.N), salze)
		},
		"Stelle negativ": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, append([]int{-1}, st.Stichprobe...), salze)
		},
		"Stelle 2": func() ([32]byte, error) { return quantenSProbeV2Pruefen(v.K, 2, v.N, v.NVal, st.Stichprobe, salze) },
		// n steht beim Token fest (K bindet ihn); ein anderes n mit anderen
		// Abschnittsgrenzen ordnet die Stellen anders zu.
		"andere Abschnittsgrenzen": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, 2*v.N, v.NVal, st.Stichprobe, salze)
		},
		"keine Validatoren": func() ([32]byte, error) { return quantenSProbeV2Pruefen(v.K, st.B, v.N, 0, st.Stichprobe, nil) },
	} {
		if got, err := f(); err == nil && got == soll {
			t.Fatalf("%s: dieselbe Bindung", name)
		}
	}
	if _, err := quantenSProbeHashV2(v.K, 1, nil); err == nil {
		t.Fatal("Bindung ohne Teil-Hashes")
	}
	// Diese Faelle muessen ausdruecklich abgewiesen werden (Fehler, nicht nur
	// eine andere Bindung).
	zuViel := append(append([][32]byte(nil), salze...), salze[0])
	for name, f := range map[string]func() ([32]byte, error){
		"Stelle 2":  func() ([32]byte, error) { return quantenSProbeV2Pruefen(v.K, 2, v.N, v.NVal, st.Stichprobe, salze) },
		"Stelle -1": func() ([32]byte, error) { return quantenSProbeV2Pruefen(v.K, -1, v.N, v.NVal, st.Stichprobe, salze) },
		"ein Salz zu wenig": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe, salze[:3])
		},
		"ein Salz zu viel": func() ([32]byte, error) { return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe, zuViel) },
		"Stelle doppelt": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, append(append([]int(nil), st.Stichprobe...), st.Stichprobe[0]), salze)
		},
		"Stelle ausserhalb": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, append(append([]int(nil), st.Stichprobe...), v.N), salze)
		},
		"Stelle negativ": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, append([]int{-1}, st.Stichprobe...), salze)
		},
		"keine Validatoren": func() ([32]byte, error) { return quantenSProbeV2Pruefen(v.K, st.B, v.N, 0, st.Stichprobe, nil) },
		"zu viele Validatoren": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, 1000, quantenWVMax+1, nil, make([][32]byte, quantenWVMax+1))
		},
		"n zu gross": func() ([32]byte, error) {
			return quantenSProbeV2Pruefen(v.K, st.B, quantenWNMax+1, v.NVal, st.Stichprobe, salze)
		},
	} {
		if _, err := f(); err == nil {
			t.Fatalf("%s: nicht abgewiesen", name)
		}
	}
	// Gegenprobe: richtig gibt die Bindung.
	if got, err := quantenSProbeV2Pruefen(v.K, st.B, v.N, v.NVal, st.Stichprobe, salze); err != nil || got != soll {
		t.Fatalf("Gutfall: %x %v", got, err)
	}
}
