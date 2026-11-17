package keeper

import (
	"encoding/json"
	"fmt"
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
