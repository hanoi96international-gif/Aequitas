package keeper

import (
	"os"
	"testing"
	"time"
)

// Breite Zufallspruefung der verteilten Annahme: 3, 4, 5 und 7 Validatoren,
// je 30 Seeds, verschiedene Wechselzeiten und Verlustraten -- 120 Laeufe,
// etwa zwei Minuten. Opt-in (VERTEILT_BREIT=1), weil zu lang fuer jeden Lauf;
// vor jeder Aenderung an leitung_verteilt.go laufen lassen.
func TestVerteiltBreit(t *testing.T) {
	if os.Getenv("VERTEILT_BREIT") == "" {
		t.Skip("opt-in: VERTEILT_BREIT=1 (120 Simulationslaeufe, ~2 min)")
	}
	for _, knoten := range []int{3, 4, 5, 7} {
		for seed := int64(1000); seed < 1030; seed++ {
			c := testKonfig()
			c.WechselAlle = time.Duration(15+seed%4*20) * time.Second
			n := verteiltesSimNetz(t, knoten, seed*int64(knoten), c)
			n.verlust = 0.05 + float64(seed%3)*0.05
			for i, k := range n.knoten {
				k.gang = (n.r.Float64()*2 - 1) * 0.002
				n.baue(i)
			}
			p := neueVerteiltPruefung(40)
			for runde := 0; runde < 25; runde++ {
				switch n.r.Intn(6) {
				case 0, 5:
					n.knoten[n.r.Intn(knoten)].an = false
				case 1:
					i := n.r.Intn(knoten)
					if !n.knoten[i].an {
						n.knoten[i].an = true
						n.baue(i)
					}
				case 2:
					n.gruppe = map[int]int{}
					for i := range n.knoten {
						n.gruppe[i] = n.r.Intn(2)
					}
				case 3:
					n.gruppe = map[int]int{}
					n.gekappt = map[[2]int]bool{}
					for k := 0; k < 2; k++ {
						a, b := n.r.Intn(knoten), n.r.Intn(knoten)
						if a != b {
							if a > b {
								a, b = b, a
							}
							n.gekappt[[2]int{a, b}] = true
						}
					}
				}
				p.laufe(n, time.Duration(3+n.r.Intn(30))*time.Second)
				if p.verletzt {
					t.Fatalf("knoten %d seed %d runde %d", knoten, seed, runde)
				}
			}
			for i, k := range n.knoten {
				if !k.an {
					k.an = true
					n.baue(i)
				}
			}
			n.gruppe = map[int]int{}
			n.gekappt = map[[2]int]bool{}
			p.laufe(n, time.Minute)
			p.versorgt, p.schritte = 0, 0
			p.laufe(n, time.Minute)
			if v := float64(p.versorgt) / float64(p.schritte*len(p.konten)); v < 0.75 {
				t.Errorf("knoten %d seed %d: nach Heilung %.0f %% versorgt", knoten, seed, v*100)
				t.FailNow()
			}
		}
	}
}
