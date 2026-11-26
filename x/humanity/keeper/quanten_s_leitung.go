package keeper

// WEG C, Schritt C1 (forschung/quantengeld/WEG_C.md): Pruefgroesse eines
// S-Tokens, dessen Photonen die Inhaberin ueber eine echte Quantenleitung
// SELBST misst. Erst dann schuetzt die Physik den Token vor dem Verdoppeln:
// ein Photon laesst sich nicht kopieren, wer es misst, erfaehrt nur eine Basis.
//
// Zwei Unterschiede zum Chip (ANALYSE.md 5c, Kent u. a. 2022):
//   - verkippte Basen (theta): der beste Faelscher irrt nur mit lambda(theta)
//     statt 14,64 % je Treffer;
//   - Mehrphotonen-Pulse: ein Anteil frei_ppm der gemeldeten Stellen gilt als
//     dem Faelscher bekannt, zusaetzlich zur Absprache (t-1)/v.
//
// Die Tabelle rechnet die Spezifikation exakt (wiesner_kette.py K_LEITUNG,
// Selbsttest prueft jede Zelle): ein Faelscher besteht hoechstens mit 2^-40.
// Gemessene Werte werden immer AUFGERUNDET (theta auf die naechste Stufe,
// frei auf Sechzehntel); jede Eingabe ausserhalb der Grenzen ergibt 0 (nicht
// sicher, fail-closed). Ganze Zahlen, kein Gleitkomma.
//
// NOCH NICHT IM KONSENS: nur die Rechnung. Eine Token-Art "Leitung", die sie
// benutzt, kommt mit C2 hinter einem eigenen Stichtag. Pruefvektor:
// testdata/quanten_s_leitung_vektor.json.

const (
	quantenSThetaZehntelMax = 100     // 10,0 Grad
	quantenSFreiPpmMax      = 250_000 // hoechstens ein Viertel frei (Kents Handsender: 22 %)
)

// quantenSThetaStufen: Grad, aufsteigend; Stufe 0 ist quantenWKTabelle.
var quantenSThetaStufen = [...]int{0, 2, 5, 10}

// quantenSKLeitungTabelle[theta][tol][f16] fuer theta > 0 (Stufe 0 = quantenWKTabelle).
var quantenSKLeitungTabelle = map[int]map[int][9]int{
	2: {
		2:  {624, 688, 768, 864, 984, 1144, 1360, 1656, 2096},
		3:  {784, 880, 1008, 1160, 1368, 1648, 2048, 2656, 3656},
		4:  {1000, 1152, 1344, 1600, 1952, 2472, 3272, 4640, 7320},
		5:  {1304, 1536, 1848, 2288, 2936, 3960, 5776, 0, 0},
		6:  {1736, 2112, 2640, 3440, 4736, 7088, 0, 0, 0},
		8:  {3448, 4624, 6616, 0, 0, 0, 0, 0, 0},
		10: {0, 0, 0, 0, 0, 0, 0, 0, 0},
	},
	5: {
		2:  {768, 856, 960, 1088, 1256, 1472, 1768, 2184, 2832},
		3:  {1016, 1152, 1320, 1544, 1848, 2272, 2896, 3888, 5632},
		4:  {1360, 1584, 1880, 2280, 2864, 3752, 5232, 8000, 0},
		5:  {1872, 2256, 2784, 3576, 4816, 6984, 0, 0, 0},
		6:  {2680, 3376, 4432, 6160, 0, 0, 0, 0, 0},
		8:  {6800, 0, 0, 0, 0, 0, 0, 0, 0},
		10: {0, 0, 0, 0, 0, 0, 0, 0, 0},
	},
	10: {
		2:  {1184, 1336, 1512, 1752, 2056, 2472, 3064, 3960, 5432},
		3:  {1728, 2000, 2360, 2856, 3560, 4616, 6344, 0, 0},
		4:  {2624, 3176, 3960, 5136, 7032, 0, 0, 0, 0},
		5:  {4288, 5544, 7536, 0, 0, 0, 0, 0, 0},
		6:  {7904, 0, 0, 0, 0, 0, 0, 0, 0},
		8:  {0, 0, 0, 0, 0, 0, 0, 0, 0},
		10: {0, 0, 0, 0, 0, 0, 0, 0, 0},
	},
}

// quantenSThetaStufe: die kleinste Stufe >= dem gemessenen Winkel; -1 = ausserhalb.
func quantenSThetaStufe(thetaZehntel int) int {
	if thetaZehntel < 0 || thetaZehntel > quantenSThetaZehntelMax {
		return -1
	}
	for _, st := range quantenSThetaStufen {
		if st*10 >= thetaZehntel {
			return st
		}
	}
	return -1
}

// quantenSF16Leitung: ceil(16 ((t-1)/v + freiPpm/10^6)); v >= 1, freiPpm >= 0.
func quantenSF16Leitung(t, v, freiPpm int) int {
	z := 16*(t-1)*1_000_000 + 16*freiPpm*v
	n := v * 1_000_000
	return (z + n - 1) / n
}

// quantenSKLeitung: Pruefgroesse je Annahmestelle; 0 = nicht sicher moeglich.
func quantenSKLeitung(tol, t, v, thetaZehntel, freiPpm int) int {
	if v < 1 || t < 1 || t > v || v > quantenWVMax || freiPpm < 0 || freiPpm > quantenSFreiPpmMax {
		return 0
	}
	st := quantenSThetaStufe(thetaZehntel)
	if st < 0 {
		return 0
	}
	z, ok := quantenWKTabelle[tol]
	if st > 0 {
		z, ok = quantenSKLeitungTabelle[st][tol]
	}
	f := quantenSF16Leitung(t, v, freiPpm)
	if !ok || f < 0 || f >= len(z) {
		return 0
	}
	return z[f]
}
