package keeper

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// qwTeil: ein Teil der Offenlegung mit `stellen` Stellen, je t Anteilen
// (Y mit 32 Hexzeichen, die groesste erlaubte Form) und `tiefe` Pfadknoten.
func qwTeil(stellen, t, tiefe int) []QuantenWOffen {
	var out []QuantenWOffen
	for i := 0; i < stellen; i++ {
		o := QuantenWOffen{}
		for x := 1; x <= t; x++ {
			o.Anteile = append(o.Anteile, QuantenWAnteilAngabe{X: x, Y: strings.Repeat(fmt.Sprintf("%x", (i+x)%16), 32)})
		}
		for j := 0; j < tiefe; j++ {
			o.Pfad = append(o.Pfad, "0x"+strings.Repeat(fmt.Sprintf("%x", (i+j)%16), 64))
		}
		out = append(out, o)
	}
	return out
}

// qwRumpf: der Rumpf, mit dem der Dienst einen Teil einreicht (wie
// quantenSHTTP.einreichen / handleQuantenW).
func qwRumpf(t *testing.T, offen []QuantenWOffen) int {
	t.Helper()
	roh, err := json.Marshal(map[string]interface{}{"wallet": "0x" + strings.Repeat("a", 40), "zeit": int64(1794943210),
		"quanten_w": QuantenWAngaben{K: strings.Repeat("f", 64), Index: 3, Ab: 16383, Offen: offen}})
	if err != nil {
		t.Fatal(err)
	}
	return len(roh)
}

// Befund 17.11.2026 (Pilot mit 5.200 Stellen je Annahmestelle, 3 von 4): ein
// Teil mit 64 Stellen und 14 Pfadknoten ist groesser als die Weiterleitung
// an den Leiter (zumLeiterRumpfHoechstens) -- 413, Offenlegung bei 0.
func TestQuantenWTeil_UngekuerztZuGross(t *testing.T) {
	if n := qwRumpf(t, qwTeil(quantenWOffenTeil, 3, 14)); n <= zumLeiterRumpfHoechstens {
		t.Fatalf("Gegenprobe: ungekuerzter Teil %d Bytes passt -- der Befund waere nicht nachgestellt", n)
	}
}

// Jeder erlaubte Teil, bis zum groessten (64 Stellen, t = 16, Tiefe bei
// quantenWNMax), passt nach dem Kuerzen durch die Weiterleitung; gekuerzt
// wird nur von hinten (Praefix, Reihenfolge bleibt), mindestens eine Stelle.
func TestQuantenWTeilKuerzen_PasstImmer(t *testing.T) {
	for _, tc := range []struct{ stellen, t, tiefe int }{
		{quantenWOffenTeil, quantenWTMax, 14}, {quantenWOffenTeil, 3, 14}, {quantenWOffenTeil, 1, 11},
		{1, quantenWTMax, 14}, {7, 5, 13}, {quantenWOffenTeil, 2, 0},
	} {
		voll := qwTeil(tc.stellen, tc.t, tc.tiefe)
		k := quantenWTeilKuerzen(voll)
		if len(k) < 1 || len(k) > len(voll) {
			t.Fatalf("%+v: %d Stellen nach dem Kuerzen", tc, len(k))
		}
		for i := range k {
			if (tc.tiefe > 0 && k[i].Pfad[0] != voll[i].Pfad[0]) || k[i].Anteile[0] != voll[i].Anteile[0] {
				t.Fatalf("%+v: kein Praefix an Stelle %d", tc, i)
			}
		}
		if n := qwRumpf(t, k); n > zumLeiterRumpfHoechstens {
			t.Fatalf("%+v: Rumpf %d Bytes > %d", tc, n, zumLeiterRumpfHoechstens)
		}
		if n := qwRumpf(t, k); n > quantenWKoerperMax {
			t.Fatalf("%+v: Rumpf %d Bytes > quantenWKoerperMax", tc, n)
		}
	}
	// Kleine Teile bleiben unveraendert (kein unnoetiges Kuerzen).
	if k := quantenWTeilKuerzen(qwTeil(quantenWOffenTeil, 1, 11)); len(k) != quantenWOffenTeil {
		t.Fatalf("Teil mit Tiefe 11 und t = 1 unnoetig gekuerzt: %d", len(k))
	}
}

// Der gekuerzte Teil ist der LAENGSTE Anfang, der ins Budget passt -- eine
// Stelle mehr passt nicht mehr. Befund 18.11.2026: halbieren verschenkte bei
// 3 von 4 (Tiefe 14) gut ein Drittel jeder Einreichung, und Einreichungen
// sind durch die Burst-Grenze je Absender der Engpass.
func TestQuantenWTeilKuerzen_Laengster(t *testing.T) {
	budget := func(o []QuantenWOffen) int {
		roh, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		return len(roh)
	}
	for _, tc := range []struct{ stellen, t, tiefe int }{
		{quantenWOffenTeil, quantenWTMax, 14}, {quantenWOffenTeil, 3, 14}, {quantenWOffenTeil, 4, 13},
		{quantenWOffenTeil, 8, 14}, {40, 3, 14},
	} {
		voll := qwTeil(tc.stellen, tc.t, tc.tiefe)
		k := quantenWTeilKuerzen(voll)
		if budget(k) > quantenWTeilRumpfBudget {
			t.Fatalf("%+v: %d Stellen passen nicht", tc, len(k))
		}
		if len(k) < len(voll) && budget(voll[:len(k)+1]) <= quantenWTeilRumpfBudget {
			t.Fatalf("%+v: %d Stellen -- eine mehr haette gepasst", tc, len(k))
		}
		t.Logf("%+v: %d von %d Stellen je Einreichung", tc, len(k), len(voll))
	}
	// Bei 3 von 4 (Tiefe 14) traegt eine Einreichung deutlich mehr als die
	// Haelfte -- der Gewinn, um den es geht.
	if k := quantenWTeilKuerzen(qwTeil(quantenWOffenTeil, 3, 14)); len(k) <= quantenWOffenTeil/2 {
		t.Fatalf("3 von 4, Tiefe 14: nur %d Stellen (halbieren gaebe %d)", len(k), quantenWOffenTeil/2)
	}
}

// Ein Rest wird nur gemerkt, wenn eine Einreichung nur fuer ihn mindestens so
// viel traegt wie ein frisch zugeschnittener Teil.
func TestQuantenWRestLohnt(t *testing.T) {
	for _, tc := range []struct {
		teil, rest int
		lohnt      bool
	}{{47, 17, false}, {32, 32, true}, {20, 44, true}, {64, 0, false}, {1, 0, false}, {10, 11, true}} {
		if quantenWRestLohnt(tc.teil, tc.rest) != tc.lohnt {
			t.Fatalf("Teil %d, Rest %d: lohnt %v erwartet", tc.teil, tc.rest, tc.lohnt)
		}
	}
}

// Der Rest eines gekuerzten Teils wird nur fuer GENAU den naechsten Teil
// desselben Versuchs mit denselben Positionen wiederverwendet; alles andere
// (anderer Versuch, andere Startstelle, andere Stichprobe nach einer neuen
// Anmeldung, unbekannte Note) faellt durch -- dann wird wie sonst geholt,
// und die Kette prueft ohnehin jeden Teil selbst.
func TestQuantenWRest_NurPassend(t *testing.T) {
	K := strings.Repeat("a", 64)
	d := &quantenWDienst{eintraege: map[string]*quantenWTresorEintrag{K: {}}}
	stellen := []int{10, 12, 15, 20}
	offen := qwTeil(4, 3, 2)

	d.restMerken("fremd", 0, 32, stellen, offen) // unbekannte Note: nichts merken
	if _, ok := d.restNehmen("fremd", 0, 32, stellen); ok {
		t.Fatal("Rest fuer eine Note, die nicht im Tresor ist")
	}
	for _, f := range []struct {
		name      string
		index, ab int
		st        []int
	}{
		{"anderer Versuch", 1, 32, stellen},
		{"andere Startstelle", 0, 64, stellen},
		{"andere Positionen", 0, 32, []int{10, 13, 15, 20}},
		{"keine Positionen", 0, 32, nil},
	} {
		d.restMerken(K, 0, 32, stellen, offen)
		if _, ok := d.restNehmen(K, f.index, f.ab, f.st); ok {
			t.Fatalf("%s: Rest wiederverwendet", f.name)
		}
		if _, ok := d.rest[K]; ok {
			t.Fatalf("%s: unpassender Rest nicht vergessen", f.name)
		}
	}
	// Passt: die eigene Seite ist laenger (64 Stellen ab ab) -- verglichen
	// wird der gemeinsame Anfang, zurueck kommt nur der gemerkte Rest.
	d.restMerken(K, 0, 32, stellen, offen)
	r, ok := d.restNehmen(K, 0, 32, append(slices.Clone(stellen), 30, 31))
	if !ok || len(r.offen) != 4 || r.offen[0].Pfad[0] != offen[0].Pfad[0] {
		t.Fatalf("passender Rest nicht genommen: %v %d", ok, len(r.offen))
	}
	// Leerer Rest und Vergessen der Note raeumen auf.
	d.restMerken(K, 0, 36, nil, nil)
	if _, ok := d.rest[K]; ok {
		t.Fatal("leerer Rest gemerkt")
	}
	d.restMerken(K, 0, 32, stellen, offen)
	d.ablage = &quantenWSpeicherAblage{m: map[string][]byte{}}
	d.vergessen(K)
	if _, ok := d.rest[K]; ok {
		t.Fatal("Rest nach dem Vergessen der Note noch da")
	}
}
