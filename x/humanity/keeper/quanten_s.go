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
	// S2b: Einloesen in Teilen (anmelden, offenlegenTeil, frist).
	Tot       bool
	Fehl      int
	Anmeldung *quantenSAnmeldung
}

// quantenSAnmeldung: eine Annahmestelle hat den Token zum Einloesen angemeldet.
type quantenSAnmeldung struct {
	B       int
	Stelle  string
	X       []int
	Probe   []int // sortiert
	Offen   int
	Treffer int
	Fehler  int
	Zeit    int64
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

// ------------------------------------------------------------ S2b: in Teilen
//
// Wie die Spezifikation (KetteS.anmelden/offenlegen/_frist): eine Stelle
// meldet an (Vorlage-Unterschrift der Inhaberin prueft der Aufrufer), die
// Validatoren legen die Stichprobe in Teilen offen, geurteilt wird ueber alle.
// Nicht bestanden: Fehlversuch; nach quantenWFehlMax ist der Token tot (der
// Aufrufer bucht den Wert ans Grundeinkommen). Legen die Validatoren nicht in
// der Frist offen: Anmeldung weg, kein Fehlversuch.

// anmelden: hoechstens eine offene Anmeldung je Token.
func (e *quantenSToken) anmelden(K string, b int, stelle string, x, probe []int, zeit int64) error {
	if e.Eingeloest || e.Tot {
		return quantenWFehler("S-Anmelden: eingeloest oder tot")
	}
	e.frist(zeit)
	if e.Anmeldung != nil {
		return quantenWFehler("S-Anmelden: schon eine offene Anmeldung")
	}
	if e.D == nil || e.C == -1 || (b != 0 && b != 1) || !kanonischeAdresse(stelle) {
		return quantenWFehler("S-Anmelden: Bindung, Wahl oder Stelle")
	}
	if len(x) != e.N || !quantenWBitsGueltig(x) {
		return quantenWFehler("S-Anmelden: Form von x")
	}
	sortiert := append([]int(nil), probe...)
	sort.Ints(sortiert)
	for i := range sortiert {
		if sortiert[i] < 0 || sortiert[i] >= e.N || (i > 0 && sortiert[i] == sortiert[i-1]) {
			return quantenWFehler("S-Anmelden: Stichprobe")
		}
	}
	if len(sortiert) == 0 || quantenSProbeHash(K, b, sortiert) != e.Proben[b] {
		return quantenWFehler("S-Anmelden: Stichprobe passt nicht zur Ausgabe")
	}
	e.Anmeldung = &quantenSAnmeldung{B: b, Stelle: stelle, X: append([]int(nil), x...), Probe: sortiert, Zeit: zeit}
	return nil
}

// offenlegenTeil: Stellen Probe[ab : ab+len(offen)], der Reihe nach,
// hoechstens quantenWOffenTeil, ganz oder gar nicht. fertig = alle offen;
// dann bestanden (Token verbraucht) oder nicht (Fehlversuch, evtl. tot).
func (e *quantenSToken) offenlegenTeil(K string, ab int, offen []quantenWOffenStelle, zeit int64) (fertig, bestanden bool, err error) {
	e.frist(zeit)
	a := e.Anmeldung
	if a == nil {
		return false, false, quantenWFehler("S-Offenlegen: keine Anmeldung")
	}
	if ab != a.Offen || len(offen) < 1 || len(offen) > quantenWOffenTeil || ab+len(offen) > len(a.Probe) {
		return false, false, quantenWFehler("S-Offenlegen: Teil (ab %d, faellig %d, %d Stellen)", ab, a.Offen, len(offen))
	}
	bis := ab + len(offen)
	stellen := a.Probe[ab:bis]
	basen := make([]int, len(stellen))
	antwort := make([]int, len(stellen))
	for j, i := range stellen {
		basen[j] = e.D[i] ^ a.B ^ e.C
		antwort[j] = a.X[i]
	}
	tr, fe, err := quantenWAuswerten(K, e.N, e.T, e.NVal, e.Wurzel, stellen, basen, antwort, offen)
	if err != nil {
		return false, false, err
	}
	a.Offen, a.Treffer, a.Fehler = bis, a.Treffer+tr, a.Fehler+fe
	if a.Offen < len(a.Probe) {
		return false, false, nil
	}
	e.Anmeldung = nil
	if quantenWUrteil(a.Treffer, a.Fehler, e.Tol) != quantenWEcht {
		e.Fehl++
		if e.Fehl >= quantenWFehlMax {
			e.Tot = true
		}
		return true, false, nil
	}
	e.Eingeloest = true
	return true, true, nil
}

// frist: Anmeldung ohne vollstaendige Offenlegung nach quantenWFristOffen
// endet ohne Fehlversuch. true = beendet.
func (e *quantenSToken) frist(zeit int64) bool {
	if a := e.Anmeldung; a != nil && zeit > a.Zeit+quantenWFristOffen {
		e.Anmeldung = nil
		return true
	}
	return false
}
