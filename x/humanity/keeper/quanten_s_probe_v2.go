package keeper

// QUANTENGELD VARIANTE S -- STICHPROBE V2: ERST NACH DER BINDUNG (Bausteine)
//
// Befund 5i (forschung/quantengeld/DEZENTRAL.md 16): Eine Annahmestelle, die
// vor der Bindung mit der Inhaberin zusammenarbeitet, laesst sie an beiden
// Stellen durchkommen -- es reicht, zu verraten, welche Stellen ihre sind.
// Ausweg (s_kette.py 5j): Bei der Ausgabe kennt niemand eine Stelle der
// Stichproben. Jeder Validator meldet je Annahmestelle nur einen GESALZENEN
// Teil-Hash seines Abschnitts; die Kette bindet die Stichprobe aus den
// Teil-Hashes (Bindung v2) und rechnet sie beim Anmelden mit den Salzen nach,
// die die Validatoren erst nach Bindung und Wahl herausgeben.
//
// Hier nur die reinen Funktionen, gleich der Spezifikation (Pruefvektor
// testdata/quanten_s_probe_v2_vektor.json aus s_kette.py --vektor-probe-v2,
// CI vergleicht Byte fuer Byte). Noch nicht in Kette, Dienst oder Werkzeug
// eingehaengt: die Kettenregel ist eine Konsensaenderung mit eigenem Stichtag
// und braucht die Freigabe des Betreibers.

import (
	"sort"
	"strconv"
	"strings"
)

var (
	quantenSDomProbeTeil = []byte("aequitas-quanten-s-probe-teil-v1|")
	quantenSDomProbeV2   = []byte("aequitas-quanten-s-probe-v2|")
)

// quantenSProbeTeilHash: Teil-Hash des Abschnitts j (1-basiert) fuer
// Annahmestelle b, gesalzen (sonst liessen sich kleine Abschnitte erraten).
func quantenSProbeTeilHash(K string, b, j int, salz [32]byte, stellen []int) [32]byte {
	s := append([]int(nil), stellen...)
	sort.Ints(s)
	teile := make([]string, len(s))
	for i, v := range s {
		teile[i] = strconv.Itoa(v)
	}
	return quantenWHash(quantenSDomProbeTeil, []byte(K), []byte("|"), []byte(strconv.Itoa(b)), []byte("|"),
		[]byte(strconv.Itoa(j)), []byte("|"), salz[:], []byte("|"), []byte(strings.Join(teile, ",")))
}

// quantenSProbeHashV2: die Bindung der Stichprobe von Stelle b aus den
// Teil-Hashes aller Abschnitte, der Reihe nach.
func quantenSProbeHashV2(K string, b int, teile [][32]byte) ([32]byte, error) {
	if len(teile) == 0 {
		return [32]byte{}, quantenWFehler("Stichprobe v2: keine Teil-Hashes")
	}
	roh := make([]byte, 0, 32*len(teile))
	for _, t := range teile {
		roh = append(roh, t[:]...)
	}
	return quantenWHash(quantenSDomProbeV2, []byte(K), []byte("|"), []byte(strconv.Itoa(b)), []byte("|"), roh), nil
}

// quantenSProbeV2Pruefen: was die Kette beim Anmelden nachrechnet. Die
// Stichprobe muss eindeutig sein und in 0..n-1 liegen (fail-closed, statt
// Stellen ausserhalb still zu verwerfen); sie wird nach den Abschnitten
// (quantenSAbschnitt) geteilt, je Abschnitt mit seinem Salz gehasht.
func quantenSProbeV2Pruefen(K string, b, n, v int, probe []int, salze [][32]byte) ([32]byte, error) {
	if b != 0 && b != 1 {
		return [32]byte{}, quantenWFehler("Stichprobe v2: Stelle")
	}
	if v < 1 || v > quantenWVMax || n < v || n > quantenWNMax || len(salze) != v {
		return [32]byte{}, quantenWFehler("Stichprobe v2: ein Salz je Abschnitt")
	}
	s := append([]int(nil), probe...)
	sort.Ints(s)
	for i, p := range s {
		if p < 0 || p >= n || (i > 0 && s[i-1] == p) {
			return [32]byte{}, quantenWFehler("Stichprobe v2: Stelle doppelt oder ausserhalb")
		}
	}
	teile := make([][32]byte, v)
	k := 0
	for j := 1; j <= v; j++ {
		von, bis, ok := quantenSAbschnitt(n, v, j)
		if !ok {
			return [32]byte{}, quantenWFehler("Stichprobe v2: Abschnitte")
		}
		start := k
		for k < len(s) && s[k] < bis {
			k++
		}
		if start < k && s[start] < von {
			return [32]byte{}, quantenWFehler("Stichprobe v2: Abschnitte")
		}
		teile[j-1] = quantenSProbeTeilHash(K, b, j, salze[j-1], s[start:k])
	}
	return quantenSProbeHashV2(K, b, teile)
}
