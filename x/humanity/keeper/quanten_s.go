package keeper

// QUANTENGELD VARIANTE S -- STUFE S2a: DIE REGELN (schlafend, nicht eingehaengt)
//
// Quanten-Token OHNE Quantenspeicher (S-money nach Kent u. a., npj QI 2022;
// ANALYSE.md 5b). Die Nutzerin misst die Qubits sofort und haelt danach nur
// Klartext x. Auf der Kette: Ausgabe (Wurzel der Pruefsummen wie Variante W,
// je Annahmestelle die Bindung ihrer geheimen Stichprobe, Deckung), Bindung d
// (maskierte Basen) und Wahl c, je einmal. Eine Annahmestelle nimmt ohne Netz
// an und loest spaeter auf der Kette ein: mit der Unterschrift der Nutzerin
// unter die Vorlage, ihrer Stichprobe und den offengelegten Anteilen dieser
// Stellen. Jeder Knoten prueft selbst; die Deckung geht genau einmal an die
// Stelle. An zwei Stellen durchzukommen verbietet die Physik (Breidbart 14,6 %
// gegen Toleranz 4 %, wiesner_parameter.py).
//
// Spezifikation: forschung/quantengeld/s_kette.py (KetteS); der Pruefvektor
// testdata/quanten_s_vektor.json kommt aus ihr. Unterschriften prueft die
// Einbindung (S2b) VOR diesen Regeln; Geld bewegt sie danach.

import (
	"crypto/sha256"
	"sort"
	"strconv"
	"strings"
)

var (
	quantenSDomProbe   = []byte("aequitas-quanten-s-probe-v1|")
	quantenSDomVorlage = "aequitas-quanten-s-vorlage-v1|"
)

// quantenSProbeHash: bindet die Stichprobe der Stelle b schon bei der Ausgabe.
func quantenSProbeHash(K string, b int, probe []int) [32]byte {
	s := append([]int(nil), probe...)
	sort.Ints(s)
	teile := make([]string, len(s))
	for i, v := range s {
		teile[i] = strconv.Itoa(v)
	}
	return quantenWHash(quantenSDomProbe, []byte(K), []byte("|"), []byte(strconv.Itoa(b)), []byte("|"), []byte(strings.Join(teile, ",")))
}

// quantenSVorlageText: was die Nutzerin bei der Vorlage unterschreibt.
func quantenSVorlageText(K string, b int, x []int) string {
	raw := make([]byte, len(x))
	for i, v := range x {
		raw[i] = byte(v)
	}
	h := sha256.Sum256(raw)
	return quantenSDomVorlage + K + "|" + strconv.Itoa(b) + "|" + hexString(h[:])
}

func hexString(b []byte) string {
	const ziffern = "0123456789abcdef"
	out := make([]byte, 2*len(b))
	for i, c := range b {
		out[2*i], out[2*i+1] = ziffern[c>>4], ziffern[c&15]
	}
	return string(out)
}

// quantenSToken: was die Kette ueber einen S-Token weiss.
type quantenSToken struct {
	N          int
	Inhaber    string
	Wurzel     [32]byte
	Deckung    int64
	Proben     [2][32]byte
	T, NVal    int
	Tol        int
	D          []int // nil bis zur Bindung
	C          int   // -1 bis zur Wahl
	Eingeloest bool
}

func neuerQuantenSToken(n int, inhaber string, wurzel [32]byte, deckung int64, proben [2][32]byte, t, nVal, tol int) (*quantenSToken, error) {
	if n < 2 || n > quantenWNMax || t < 1 || t > quantenWTMax || nVal < t || nVal > quantenWVMax || deckung <= 0 || !kanonischeAdresse(inhaber) {
		return nil, quantenWFehler("S-Ausgabe: Groessen")
	}
	if _, ok := quantenWKTabelle[tol]; !ok {
		return nil, quantenWFehler("S-Ausgabe: Toleranz %d", tol)
	}
	return &quantenSToken{N: n, Inhaber: inhaber, Wurzel: wurzel, Deckung: deckung, Proben: proben, T: t, NVal: nVal, Tol: tol, C: -1}, nil
}

// binden: nur die Inhaberin, einmal, n Bits.
func (e *quantenSToken) binden(von string, d []int) error {
	if von != e.Inhaber || e.D != nil {
		return quantenWFehler("S-Bindung: fremd oder schon gebunden")
	}
	if len(d) != e.N || !quantenWBitsGueltig(d) {
		return quantenWFehler("S-Bindung: Form")
	}
	e.D = append([]int(nil), d...)
	return nil
}

// waehlen: nur die Inhaberin, nach der Bindung, einmal.
func (e *quantenSToken) waehlen(von string, c int) error {
	if von != e.Inhaber || e.D == nil || e.C != -1 {
		return quantenWFehler("S-Wahl: fremd, ungebunden oder schon gewaehlt")
	}
	if c != 0 && c != 1 {
		return quantenWFehler("S-Wahl: Form")
	}
	e.C = c
	return nil
}

// einloesen: die Annahmestelle b mit der Vorlage x (Unterschrift der
// Inhaberin prueft der Aufrufer VORHER ueber quantenSVorlageText), ihrer
// Stichprobe und den offengelegten Anteilen (je Stelle der Probe, sortiert).
// Bestanden: der Token ist verbraucht, die Deckung geht an die Stelle.
func (e *quantenSToken) einloesen(K string, b int, x, probe []int, offen []quantenWOffenStelle) (treffer, fehler int, err error) {
	if e.Eingeloest {
		return 0, 0, quantenWFehler("S-Einloesen: schon eingeloest")
	}
	if e.D == nil || e.C == -1 || (b != 0 && b != 1) {
		return 0, 0, quantenWFehler("S-Einloesen: Bindung, Wahl oder Stelle")
	}
	if len(x) != e.N || !quantenWBitsGueltig(x) {
		return 0, 0, quantenWFehler("S-Einloesen: Form von x")
	}
	sortiert := append([]int(nil), probe...)
	sort.Ints(sortiert)
	for i := range sortiert {
		if sortiert[i] < 0 || sortiert[i] >= e.N || (i > 0 && sortiert[i] == sortiert[i-1]) {
			return 0, 0, quantenWFehler("S-Einloesen: Stichprobe")
		}
	}
	if quantenSProbeHash(K, b, sortiert) != e.Proben[b] || len(offen) != len(sortiert) {
		return 0, 0, quantenWFehler("S-Einloesen: Stichprobe passt nicht zur Ausgabe")
	}
	// Basis, die an Stelle b geprueft wird: d XOR b XOR c (fuer die ehrliche
	// Inhaberin genau ihre Messbasen); quantenWAuswerten zaehlt Treffer
	// (Basis passt) und Fehler (Wert falsch) und prueft Anteile, Pruefsumme, Pfad.
	basen := make([]int, len(sortiert))
	antwort := make([]int, len(sortiert))
	for j, i := range sortiert {
		basen[j] = e.D[i] ^ b ^ e.C
		antwort[j] = x[i]
	}
	treffer, fehler, err = quantenWAuswerten(K, e.N, e.T, e.NVal, e.Wurzel, sortiert, basen, antwort, offen)
	if err != nil {
		return 0, 0, err
	}
	if quantenWUrteil(treffer, fehler, e.Tol) != quantenWEcht {
		return treffer, fehler, quantenWFehler("S-Einloesen: Pruefung nicht bestanden (%d Fehler bei %d Treffern)", fehler, treffer)
	}
	e.Eingeloest = true
	return treffer, fehler, nil
}
