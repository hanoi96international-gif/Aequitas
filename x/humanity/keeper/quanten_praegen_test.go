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

func TestQuantenPraegen_VektorWieLabor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_praegen_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Domaene       string   `json:"domaene"`
		Von           string   `json:"von"`
		FremdVon      string   `json:"fremd_von"`
		FremdSchl     string   `json:"fremd_schluessel"`
		SerieHex      string   `json:"serie_hex"`
		Schluessel    string   `json:"schluessel"`
		SerieMaxBytes int      `json:"serie_max_bytes"`
		NullHex       string   `json:"null_hex"`
		NullSchl      string   `json:"null_schluessel"`
		Schlecht      []string `json:"nicht_kanonisch_hex"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	if v.Domaene != quantenDomaeneSerie || v.SerieMaxBytes != quantenSerieMaxBytes {
		t.Fatalf("Konstanten weichen vom Labor ab: %+v", v)
	}
	serie, _ := hex.DecodeString(v.SerieHex)
	if got := quantenSerieSchluessel(v.Von, serie); got != v.Schluessel {
		t.Fatalf("Schluessel %s, Labor %s", got, v.Schluessel)
	}
	null, _ := hex.DecodeString(v.NullHex)
	if quantenSerieSchluessel(v.FremdVon, serie) != v.FremdSchl || v.FremdSchl == v.Schluessel {
		t.Fatal("Anmelder fliesst nicht wie im Labor in K ein")
	}
	if !quantenSerieKanonisch(null) || quantenSerieSchluessel(v.Von, null) != v.NullSchl {
		t.Fatal("Null weicht vom Labor ab")
	}
	if len(v.Schlecht) == 0 {
		t.Fatal("Vektor ohne nicht-kanonische Faelle")
	}
	for _, x := range v.Schlecht {
		b, _ := hex.DecodeString(x)
		if quantenSerieKanonisch(b) {
			t.Errorf("%q gilt als kanonisch", x)
		}
	}
}

// Dieselbe Seriennummer in zweiter Schreibweise (fuehrende Null) darf weder
// angemeldet werden noch Wert haben.
func TestQuantenPraegen_ZweiteSchreibweise(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte{0x04, 0xd2}
	r, _, err := r.quantenPraegen(qnAnna, aeq(10), aeq(5), serie)
	if err != nil {
		t.Fatal(err)
	}
	lang := append([]byte{0}, serie...)
	if _, _, err := r.quantenPraegen(qnAnna, aeq(10), aeq(1), lang); err == nil {
		t.Fatal("fuehrende Null angenommen")
	}
	if r.quantenSerieWert(qnAnna, lang).Sign() != 0 {
		t.Fatal("fuehrende Null hat Wert")
	}
	// Die Null selbst ist kanonisch 0x00 und anmeldbar.
	if _, _, err := r.quantenPraegen(qnBen, aeq(1), aeq(1), []byte{0}); err != nil {
		t.Fatal(err)
	}
}

func TestQuantenPraegen_AnmeldenUndWert(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte{0x04, 0xd2}
	if r.quantenSerieWert(qnAnna, serie).Sign() != 0 {
		t.Fatal("nicht angemeldete Note hat Wert")
	}
	n, k, err := r.quantenPraegen(qnAnna, aeq(1000), aeq(300), serie)
	if err != nil {
		t.Fatal(err)
	}
	if k != quantenSerieSchluessel(qnAnna, serie) || n.quantenSerieWert(qnAnna, serie).Cmp(aeq(300)) != 0 {
		t.Fatal("Wert nach Anmeldung falsch")
	}
	if r.quantenSerieWert(qnAnna, serie).Sign() != 0 || len(r.Lightning) != 0 {
		t.Fatal("altes Register veraendert")
	}
	// Erhaltung: Konto verliert 300, Register gewinnt 300.
	if n.summeGesperrt().Cmp(aeq(300)) != 0 {
		t.Fatal("Erhaltung verletzt")
	}
	// Eine andere Seriennummer ist nichts wert.
	if n.quantenSerieWert(qnAnna, []byte{0x04, 0xd3}).Sign() != 0 {
		t.Fatal("fremde Seriennummer hat Wert")
	}
}

func TestQuantenPraegen_Angriffe(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte("seriennummer-1")
	r, _, err := r.quantenPraegen(qnAnna, aeq(1000), aeq(300), serie)
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
		{"doppelt anmelden (derselbe)", qnAnna, aeq(1000), aeq(1), serie},
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
		if n, _, err := r.quantenPraegen(f.wer, f.guthaben, f.wei, f.serie); err == nil || n != nil {
			t.Errorf("%s: angenommen", f.name)
		}
	}
	if len(r.Lightning) != 1 || r.summeGesperrt().Cmp(aeq(300)) != 0 {
		t.Fatal("abgewiesene Versuche haben das Register veraendert")
	}
	// Die laengste erlaubte Seriennummer geht.
	if _, _, err := r.quantenPraegen(qnBen, aeq(1), aeq(1), append([]byte{1}, make([]byte, quantenSerieMaxBytes-1)...)); err != nil {
		t.Fatal(err)
	}
}

// Vorlaufen: der Dieb sieht Bens Anmeldung und meldet dieselbe Seriennummer
// zuerst unter sich an. Bens Anmeldung geht trotzdem, und Bens Anmeldung
// zaehlt nur unter Ben -- unter Dieb gezeigt ist sie 1 Wei wert, nicht 50.
func TestQuantenPraegen_VorlaufenBlockiertNicht(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte("bens-note")
	r, _, err := r.quantenPraegen(qnDieb, aeq(5), big.NewInt(1), serie)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = r.quantenPraegen(qnBen, aeq(50), aeq(50), serie)
	if err != nil {
		t.Fatalf("Vorlaufen hat Bens Anmeldung blockiert: %v", err)
	}
	if r.quantenSerieWert(qnBen, serie).Cmp(aeq(50)) != 0 || r.quantenSerieWert(qnDieb, serie).Cmp(big.NewInt(1)) != 0 {
		t.Fatal("Werte je Anmelder vertauscht")
	}
	if r.quantenSerieWert(qnAnna, serie).Sign() != 0 {
		t.Fatal("Seriennummer unter Unbeteiligtem hat Wert")
	}
	if r.quantenSerieWert(strings.ToUpper(qnBen), serie).Sign() != 0 {
		t.Fatal("nicht kanonische Adresse hat Wert")
	}
}

// Lightning-Noten sind Einweg: weder einloesen noch zurueckholen.
func TestQuantenPraegen_EinwegKeinEinloesen(t *testing.T) {
	r := &quantenRegister{}
	serie := []byte("einweg")
	r, k, err := r.quantenPraegen(qnAnna, aeq(10), aeq(10), serie)
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
	if _, _, err := r.quantenPraegen(qnAnna, aeq(1), aeq(1), []byte("voll")); err == nil {
		t.Fatal("volles Register nimmt an")
	}
}

// Kanonisch sortiert, egal in welcher Reihenfolge angemeldet wird.
func TestQuantenPraegen_Kanonisch(t *testing.T) {
	r := &quantenRegister{}
	for i := 0; i < 50; i++ {
		var err error
		r, _, err = r.quantenPraegen(qnAnna, aeq(100), aeq(1), []byte{byte(i + 1), 7})
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
