package keeper

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"
)

// Quantum Lightning, Kettenseite (quantenPraegen): dieselben Regeln und
// Angriffe wie forschung/quantengeld/labor.py.

const (
	qnLabor = "labor-h1491" // Spielzeugsatz des Labors, NUR im Test
	qnGross = "test-gross"  // grosser Testsatz fuer beliebige Seriennummern
)

// qnSaetze: nur fuer Tests. Die Produktion (quantenParameterSaetze) ist leer.
var qnSaetze = map[string]*big.Int{
	qnLabor: big.NewInt(1491),
	qnGross: new(big.Int).Lsh(big.NewInt(1), 8*quantenSerieMaxBytes),
}

func qnPraeg(r *quantenRegister, wer string, guthaben, wei *big.Int, serie []byte) (*quantenRegister, string, error) {
	return r.quantenPraegenMit(qnSaetze, wer, guthaben, wei, qnGross, serie)
}

func qnWert(r *quantenRegister, von string, serie []byte) *big.Int {
	return r.quantenSerieWertMit(qnSaetze, qnGross, von, serie)
}

func TestQuantenPraegen_VektorWieLabor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_praegen_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Domaene       string   `json:"domaene"`
		Satz          string   `json:"satz"`
		Ordnung       int64    `json:"ordnung"`
		Von           string   `json:"von"`
		FremdVon      string   `json:"fremd_von"`
		FremdSchl     string   `json:"fremd_schluessel"`
		AndererSatz   string   `json:"anderer_satz"`
		AndererSchl   string   `json:"anderer_satz_schluessel"`
		SerieHex      string   `json:"serie_hex"`
		Schluessel    string   `json:"schluessel"`
		SerieMaxBytes int      `json:"serie_max_bytes"`
		NullHex       string   `json:"null_hex"`
		NullSchl      string   `json:"null_schluessel"`
		GroessteHex   string   `json:"groesste_hex"`
		ZuGrossHex    []string `json:"zu_gross_hex"`
		Schlecht      []string `json:"nicht_kanonisch_hex"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	if v.Domaene != quantenDomaeneSerie || v.SerieMaxBytes != quantenSerieMaxBytes {
		t.Fatalf("Konstanten weichen vom Labor ab: %+v", v)
	}
	if v.Satz != qnLabor || qnSaetze[qnLabor].Int64() != v.Ordnung {
		t.Fatalf("Spielzeugsatz weicht vom Labor ab: %s/%d", v.Satz, v.Ordnung)
	}
	serie, _ := hex.DecodeString(v.SerieHex)
	if got := quantenSerieSchluessel(v.Satz, v.Von, serie); got != v.Schluessel {
		t.Fatalf("Schluessel %s, Labor %s", got, v.Schluessel)
	}
	if quantenSerieSchluessel(v.Satz, v.FremdVon, serie) != v.FremdSchl || v.FremdSchl == v.Schluessel {
		t.Fatal("Anmelder fliesst nicht wie im Labor in K ein")
	}
	if quantenSerieSchluessel(v.AndererSatz, v.Von, serie) != v.AndererSchl || v.AndererSchl == v.Schluessel {
		t.Fatal("Satz fliesst nicht wie im Labor in K ein")
	}
	null, _ := hex.DecodeString(v.NullHex)
	if !quantenSerieKanonisch(null) || quantenSerieSchluessel(v.Satz, v.Von, null) != v.NullSchl {
		t.Fatal("Null weicht vom Labor ab")
	}
	groesste, _ := hex.DecodeString(v.GroessteHex)
	if !quantenSerieForm(qnSaetze, v.Satz, groesste) {
		t.Fatal("h = N-1 abgewiesen")
	}
	if len(v.ZuGrossHex) == 0 || len(v.Schlecht) == 0 {
		t.Fatal("Vektor ohne Missbrauchsfaelle")
	}
	for _, x := range v.ZuGrossHex {
		b, _ := hex.DecodeString(x)
		if quantenSerieForm(qnSaetze, v.Satz, b) {
			t.Errorf("h = %s >= N angenommen", x)
		}
	}
	for _, x := range v.Schlecht {
		b, _ := hex.DecodeString(x)
		if quantenSerieKanonisch(b) {
			t.Errorf("%q gilt als kanonisch", x)
		}
	}
}

// F4: dieselbe Note unter h + N. Ohne die N-Pruefung waere das eine neue,
// gueltige Anmeldung (K ist ein anderer).
func TestQuantenPraegen_SerieUeberOrdnung(t *testing.T) {
	r := &quantenRegister{}
	n := qnSaetze[qnLabor]
	h := big.NewInt(1234)
	r, _, err := r.quantenPraegenMit(qnSaetze, qnAnna, aeq(10), aeq(5), qnLabor, h.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []*big.Int{new(big.Int).Add(h, n), new(big.Int).Set(n), new(big.Int).Mul(n, big.NewInt(7))} {
		if _, _, err := r.quantenPraegenMit(qnSaetze, qnAnna, aeq(10), aeq(1), qnLabor, x.Bytes()); err == nil {
			t.Errorf("h = %s >= N angenommen", x)
		}
		if r.quantenSerieWertMit(qnSaetze, qnLabor, qnAnna, x.Bytes()).Sign() != 0 {
			t.Errorf("h = %s >= N hat Wert", x)
		}
	}
	if _, _, err := r.quantenPraegenMit(qnSaetze, qnAnna, aeq(10), aeq(1), qnLabor, new(big.Int).Sub(n, big.NewInt(1)).Bytes()); err != nil {
		t.Fatalf("h = N-1 abgewiesen: %v", err)
	}
}

// Unbekannter oder ungueltiger Satz: abgewiesen und wertlos. Und die
// Produktion kennt heute keinen Satz -- jede Anmeldung scheitert.
func TestQuantenPraegen_SatzUnbekannt(t *testing.T) {
	r := &quantenRegister{}
	for _, satz := range []string{"", "unbekannt", "LABOR-H1491", "labor|h1491", strings.Repeat("a", 33)} {
		if _, _, err := r.quantenPraegenMit(qnSaetze, qnAnna, aeq(10), aeq(1), satz, []byte{1}); err == nil {
			t.Errorf("Satz %q angenommen", satz)
		}
	}
	if _, _, err := r.quantenPraegen(qnAnna, aeq(10), aeq(1), qnLabor, []byte{1}); err == nil {
		t.Fatal("Produktion nimmt den Spielzeugsatz an")
	}
	if r.quantenSerieWert(qnLabor, qnAnna, []byte{1}).Sign() != 0 {
		t.Fatal("Produktion gibt Wert ohne Satz")
	}
	// Kaputte Ordnung im Satz -> abgewiesen.
	kaputt := map[string]*big.Int{"null": big.NewInt(0), "nil": nil, "neg": big.NewInt(-5)}
	for satz := range kaputt {
		if _, _, err := r.quantenPraegenMit(kaputt, qnAnna, aeq(10), aeq(1), satz, []byte{0}); err == nil {
			t.Errorf("Satz %q mit kaputter Ordnung angenommen", satz)
		}
	}
}

// Waechter: jeder Satz, der je in die Produktion kommt, muss kryptografisch
// gross sein (Lightning: zweite Note mit gleicher Seriennummer ~ N Versuche).
func TestQuantenPraegen_ProduktionsSaetzeSicher(t *testing.T) {
	min := new(big.Int).Lsh(big.NewInt(1), 256)
	for satz, n := range quantenParameterSaetze {
		if !quantenSatzGueltig(satz) || n == nil || n.Cmp(min) < 0 {
			t.Errorf("Parametersatz %q ungueltig oder zu klein (N < 2^256)", satz)
		}
		if strings.HasPrefix(satz, "labor") || strings.HasPrefix(satz, "test") {
			t.Errorf("Spielzeug-/Testsatz %q in der Produktion", satz)
		}
	}
}

func TestQuantenPraegen_AnmeldenUndWert(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte{0x04, 0xd2}
	if qnWert(r, qnAnna, serie).Sign() != 0 {
		t.Fatal("nicht angemeldete Note hat Wert")
	}
	n, k, err := qnPraeg(r, qnAnna, aeq(1000), aeq(300), serie)
	if err != nil {
		t.Fatal(err)
	}
	if k != quantenSerieSchluessel(qnGross, qnAnna, serie) || qnWert(n, qnAnna, serie).Cmp(aeq(300)) != 0 {
		t.Fatal("Wert nach Anmeldung falsch")
	}
	if n.Lightning[0].Satz != qnGross {
		t.Fatal("Satz nicht vermerkt")
	}
	if qnWert(r, qnAnna, serie).Sign() != 0 || len(r.Lightning) != 0 {
		t.Fatal("altes Register veraendert")
	}
	// Unter einem anderen Satz ist dieselbe Zahl nichts wert.
	if n.quantenSerieWertMit(qnSaetze, qnLabor, qnAnna, serie).Sign() != 0 {
		t.Fatal("Wert ueber Satzgrenze hinweg")
	}
	// Erhaltung: Konto verliert 300, Register gewinnt 300.
	if n.summeGesperrt().Cmp(aeq(300)) != 0 {
		t.Fatal("Erhaltung verletzt")
	}
	if qnWert(n, qnAnna, []byte{0x04, 0xd3}).Sign() != 0 {
		t.Fatal("fremde Seriennummer hat Wert")
	}
}

func TestQuantenPraegen_Angriffe(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte("seriennummer-1")
	r, _, err := qnPraeg(r, qnAnna, aeq(1000), aeq(300), serie)
	if err != nil {
		t.Fatal(err)
	}
	faelle := []struct {
		name     string
		wer      string
		guthaben *big.Int
		wei      *big.Int
		serie    []byte
	}{
		{"doppelt anmelden", qnAnna, aeq(1000), aeq(1), serie},
		{"ohne Deckung", qnDieb, aeq(5), aeq(100), []byte("x")},
		{"Betrag null", qnDieb, aeq(5), big.NewInt(0), []byte("y")},
		{"Betrag negativ", qnDieb, aeq(5), big.NewInt(-1), []byte("z")},
		{"Betrag fehlt", qnDieb, aeq(5), nil, []byte("w")},
		{"Guthaben fehlt", qnDieb, nil, aeq(1), []byte("v")},
		{"leere Seriennummer", qnDieb, aeq(5), aeq(1), nil},
		{"Seriennummer zu lang", qnDieb, aeq(5), aeq(1), append([]byte{1}, make([]byte, quantenSerieMaxBytes)...)},
		{"Seriennummer mit fuehrender Null", qnDieb, aeq(5), aeq(1), []byte{0, 1}},
		{"Adresse nicht kanonisch", strings.ToUpper(qnDieb), aeq(5), aeq(1), []byte("u")},
	}
	for _, f := range faelle {
		if n, _, err := qnPraeg(r, f.wer, f.guthaben, f.wei, f.serie); err == nil || n != nil {
			t.Errorf("%s: angenommen", f.name)
		}
	}
	if len(r.Lightning) != 1 || r.summeGesperrt().Cmp(aeq(300)) != 0 {
		t.Fatal("abgewiesene Versuche haben das Register veraendert")
	}
	// Die laengste erlaubte Seriennummer geht.
	if _, _, err := qnPraeg(r, qnBen, aeq(1), aeq(1), append([]byte{1}, make([]byte, quantenSerieMaxBytes-1)...)); err != nil {
		t.Fatal(err)
	}
}

// Dieselbe Seriennummer in zweiter Schreibweise (fuehrende Null) darf weder
// angemeldet werden noch Wert haben.
func TestQuantenPraegen_ZweiteSchreibweise(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte{0x04, 0xd2}
	r, _, err := qnPraeg(r, qnAnna, aeq(10), aeq(5), serie)
	if err != nil {
		t.Fatal(err)
	}
	lang := append([]byte{0}, serie...)
	if _, _, err := qnPraeg(r, qnAnna, aeq(10), aeq(1), lang); err == nil {
		t.Fatal("fuehrende Null angenommen")
	}
	if qnWert(r, qnAnna, lang).Sign() != 0 {
		t.Fatal("fuehrende Null hat Wert")
	}
	// Die Null selbst ist kanonisch 0x00 und anmeldbar.
	if _, _, err := qnPraeg(r, qnBen, aeq(1), aeq(1), []byte{0}); err != nil {
		t.Fatal(err)
	}
}

// Vorlaufen: der Dieb sieht Bens Anmeldung und meldet dieselbe Seriennummer
// zuerst unter sich an. Bens Anmeldung geht trotzdem, und Bens Anmeldung
// zaehlt nur unter Ben -- unter Dieb gezeigt ist sie 1 Wei wert, nicht 50.
func TestQuantenPraegen_VorlaufenBlockiertNicht(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte("bens-note")
	r, _, err := qnPraeg(r, qnDieb, aeq(5), big.NewInt(1), serie)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = qnPraeg(r, qnBen, aeq(50), aeq(50), serie)
	if err != nil {
		t.Fatalf("Vorlaufen hat Bens Anmeldung blockiert: %v", err)
	}
	if qnWert(r, qnBen, serie).Cmp(aeq(50)) != 0 || qnWert(r, qnDieb, serie).Cmp(big.NewInt(1)) != 0 {
		t.Fatal("Werte je Anmelder vertauscht")
	}
	if qnWert(r, qnAnna, serie).Sign() != 0 {
		t.Fatal("Seriennummer unter Unbeteiligtem hat Wert")
	}
	if qnWert(r, strings.ToUpper(qnBen), serie).Sign() != 0 {
		t.Fatal("nicht kanonische Adresse hat Wert")
	}
}

// Lightning-Noten sind Einweg: weder einloesen noch zurueckholen.
func TestQuantenPraegen_EinwegKeinEinloesen(t *testing.T) {
	r := &quantenRegister{}
	r, k, err := qnPraeg(r, qnAnna, aeq(10), aeq(10), []byte("einweg"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.quantenRueckholen(qnAnna, k, 1<<40); err == nil {
		t.Fatal("Lightning-Note zurueckgeholt")
	}
	if _, _, err := r.quantenEinloesen(k, qnAnna, aeq(10), nil, nil); err == nil {
		t.Fatal("Lightning-Note eingeloest")
	}
}

// Die Grenze gilt fuer alles im Register zusammen.
func TestQuantenPraegen_RegisterVoll(t *testing.T) {
	r := &quantenRegister{Verbraucht: make([]string, quantenRegisterMax)}
	if _, _, err := qnPraeg(r, qnAnna, aeq(1), aeq(1), []byte("voll")); err == nil {
		t.Fatal("volles Register nimmt an")
	}
}

// Kanonisch sortiert, egal in welcher Reihenfolge angemeldet wird.
func TestQuantenPraegen_Kanonisch(t *testing.T) {
	r := &quantenRegister{}
	for i := 0; i < 50; i++ {
		var err error
		r, _, err = qnPraeg(r, qnAnna, aeq(100), aeq(1), []byte{byte(i + 1), 7})
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(r.Lightning); i++ {
		if r.Lightning[i-1].K >= r.Lightning[i].K {
			t.Fatal("nicht kanonisch sortiert")
		}
	}
	if r.summeGesperrt().Cmp(aeq(50)) != 0 {
		t.Fatal("Summe falsch")
	}
}
