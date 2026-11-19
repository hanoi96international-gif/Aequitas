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
// Hier die reinen Funktionen, gleich der Spezifikation (Pruefvektor
// testdata/quanten_s_probe_v2_vektor.json aus s_kette.py --vektor-probe-v2,
// CI vergleicht Byte fuer Byte), und der Stichtag. Eingehaengt sind sie im
// Dienst (quanten_s_abschnitt_dienst.go, #595), in der Kette
// (quanten_w_s_kette.go, #596) und im Werkzeug
// (quanten_s_ausgabe_abschnitte.go, --stichprobe-v2). Scharf ab dem
// Stichtag des Betreibers (unten, Konsensaenderung).

import (
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// STICHTAG DER STICHPROBE V2 (Kettenregel). Vor dem Stichtag gibt jeder
// Knoten einem S-Satz mit probe_art oder salze einen Befund -- er kommt nicht
// in die Annahme, und ein Block mit ihm scheitert beim Nachspielen. Das muss
// so sein: ein aelterer Knoten verwirft die unbekannten Felder beim Einlesen,
// kann den Blockhash (json.Marshal der Transaktionen) eines solchen Blocks
// also nicht nachrechnen und weist ihn ab. Uebergehen statt Abweisen liesse
// alte und neue Knoten auseinanderlaufen. Ab dem Stichtag nimmt die Kette
// Ausgaben mit probe_art 2 an; der bisherige Weg (probe_art 0) bleibt.
//
// Freigeschaltet vom Betreiber am 19.11.2026 ("Stichtag heute so schnell wie
// moeglich") fuer den 19.11.2026 19:30 UTC = 1795116600. Vorher: Pilot mit
// vier Knoten zweimal gruen (#598), C1 mit der schlafenden Regel seit
// Deploy #105. Frueher ging nicht: Der Deploy-Waechter
// (.github/neustart/quanten-s-probe-v2-nicht-verpasst.sh) verlangt beim
// Ausrollen mindestens 30 min Vorlauf, davor liegen CI, Merge und die
// Freigabe des Deploys. Als Zahl geschrieben, der Waechter liest sie;
// 9223372036854775807 = math.MaxInt64 = schlafend.
var (
	quantenSProbeV2AbUnix int64 = 1795116600
)

// quantenSProbeV2AbOverride: nur fuer Tests (0 = Stichtag oben).
var quantenSProbeV2AbOverride atomic.Int64

func quantenSProbeV2Ab() int64 {
	if v := quantenSProbeV2AbOverride.Load(); v != 0 {
		return v
	}
	return quantenSProbeV2AbUnix
}

func quantenSProbeV2Aktiv(t int64) bool { return t >= quantenSProbeV2Ab() }

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
