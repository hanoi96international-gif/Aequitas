package keeper

// WEG C, C2b-2b-3c Teil 2 (forschung/quantengeld/WEG_C.md): Mehrfach-Beweis.
// Spezifikation: s_kette.s3_mehrfachbeweis / s3_mehrfach_pruefen, Pruefvektor
// testdata/quanten_s3_beweis_vektor.json (s_kette.py --vektor-s3-beweis).
//
// Ueber eine Leitung geht die Wurzel ueber ALLE N Pulse (bis 2^26). Eine
// Annahmestelle braucht nur die n Blaetter des Tokens (Stelle j = Puls
// teil[j]) und EINEN Beweis, dass sie an genau diesen Pulsen in der Wurzel
// stehen. Oeffentlich: die Blaetter sind Pruefsummen mit Salz, die geheime
// Stichprobe der Stelle verraten sie nicht (sie fragt nie einzelne Pfade an).
//
// Form: Ebene fuer Ebene, bekannte Knoten aufsteigend, je fehlendem Nachbarn
// ein Hash; das verdoppelte letzte Blatt einer ungeraden Ebene (wie
// quantenWMerkleWurzel) braucht keinen. Genau eine Form je (N, Indizes):
// keiner zu viel, keiner zu wenig. Nur Rechnung, nicht im Konsens.

import "sort"

// quantenS3BeweisMax: hoechstens so viele Hashes (n <= quantenWNMax, Tiefe <= 26
// ergibt weit weniger).
const quantenS3BeweisMax = 1 << 21

// quantenS3Mehrfachbeweis: der Beweis fuer die Blaetter an den (aufsteigenden,
// verschiedenen) Indizes. ok = false bei ungueltigen Indizes.
func quantenS3Mehrfachbeweis(blaetter [][32]byte, idx []int) ([][32]byte, bool) {
	n := len(blaetter)
	if len(idx) == 0 || idx[0] < 0 || idx[len(idx)-1] >= n {
		return nil, false
	}
	for j := 1; j < len(idx); j++ {
		if idx[j] <= idx[j-1] {
			return nil, false
		}
	}
	var beweis [][32]byte
	ebene := append([][32]byte(nil), blaetter...)
	bekannt := append([]int(nil), idx...)
	for len(ebene) > 1 {
		groesse := len(ebene)
		if groesse%2 == 1 {
			ebene = append(ebene, ebene[groesse-1])
		}
		menge := make(map[int]bool, len(bekannt))
		for _, i := range bekannt {
			menge[i] = true
		}
		var eltern []int
		for _, i := range bekannt {
			s := i ^ 1
			if menge[s] {
				if i%2 == 0 {
					eltern = append(eltern, i/2)
				}
				continue
			}
			if s < groesse {
				beweis = append(beweis, ebene[s])
			}
			eltern = append(eltern, i/2)
		}
		naechste := make([][32]byte, 0, len(ebene)/2)
		for j := 0; j < len(ebene); j += 2 {
			naechste = append(naechste, quantenWKnoten(ebene[j], ebene[j+1]))
		}
		ebene = naechste
		bekannt = eindeutigSortiert(eltern)
	}
	return beweis, true
}

func eindeutigSortiert(a []int) []int {
	sort.Ints(a)
	out := a[:0]
	for j, x := range a {
		if j == 0 || x != a[j-1] {
			out = append(out, x)
		}
	}
	return out
}

// quantenS3MehrfachPruefen: streng (genau die Hashes des Beweises, Indizes
// aufsteigend in [0, n), n <= quantenS3PulsMax). Rechenaufwand hoechstens
// O(|idx| * Tiefe).
func quantenS3MehrfachPruefen(wurzel [32]byte, n int, idx []int, blaetter, beweis [][32]byte) bool {
	if n < 1 || n > quantenS3PulsMax || len(idx) == 0 || len(idx) != len(blaetter) || len(beweis) > quantenS3BeweisMax {
		return false
	}
	if idx[0] < 0 || idx[len(idx)-1] >= n {
		return false
	}
	for j := 1; j < len(idx); j++ {
		if idx[j] <= idx[j-1] {
			return false
		}
	}
	type knoten struct {
		i int
		h [32]byte
	}
	ebene := make([]knoten, len(idx))
	for j := range idx {
		ebene[j] = knoten{idx[j], blaetter[j]}
	}
	groesse, z := n, 0
	for groesse > 1 {
		neu := make([]knoten, 0, len(ebene))
		for j := 0; j < len(ebene); j++ {
			i, h := ebene[j].i, ebene[j].h
			s := i ^ 1
			if i%2 == 0 && j+1 < len(ebene) && ebene[j+1].i == s {
				neu = append(neu, knoten{i / 2, quantenWKnoten(h, ebene[j+1].h)})
				j++
				continue
			}
			var nachbar [32]byte
			if s < groesse {
				if z >= len(beweis) {
					return false
				}
				nachbar = beweis[z]
				z++
			} else {
				nachbar = h // verdoppeltes letztes Blatt
			}
			if i%2 == 0 {
				neu = append(neu, knoten{i / 2, quantenWKnoten(h, nachbar)})
			} else {
				neu = append(neu, knoten{i / 2, quantenWKnoten(nachbar, h)})
			}
		}
		ebene, groesse = neu, (groesse+1)/2
	}
	return z == len(beweis) && len(ebene) == 1 && ebene[0].i == 0 && ebene[0].h == wurzel
}
