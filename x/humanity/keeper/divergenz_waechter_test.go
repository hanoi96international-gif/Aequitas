package keeper

import (
	"encoding/json"
	"testing"
	"time"
)

// Selbstheilung nur mit ausdruecklicher Erlaubnis UND signierter Quelle --
// nie auf den Gruenderboxen (Env unset), nie ohne Snapshot-Quelle.
func TestDivergenzAutoResyncErlaubt(t *testing.T) {
	if divergenzAutoResyncErlaubt(divergenzSchwelle, "", true) {
		t.Fatal("ohne Env darf nichts passieren")
	}
	if divergenzAutoResyncErlaubt(divergenzSchwelle, "1", false) {
		t.Fatal("ohne Quelle darf nichts passieren")
	}
	if divergenzAutoResyncErlaubt(divergenzSchwelle-1, "1", true) {
		t.Fatal("unter der Schwelle darf nichts passieren")
	}
	if !divergenzAutoResyncErlaubt(divergenzSchwelle, " 1 ", true) {
		t.Fatal("Schwelle + Env + Quelle muss ausloesen")
	}
}

// Ein Vergleich zaehlt nur, wenn BEIDE Seiten still sind. 14.09.2026: C2 war
// still, C1 unter Volllast -- drei Strikes, Wache rot, und ein
// Laien-Validator haette mitten in der Last einen Resync begonnen.
func TestDivergenzVergleichbarNurWennBeideRuhen(t *testing.T) {
	still := divergenzRuhe.Seconds() + 1
	beschaeftigt := divergenzRuhe.Seconds() - 1
	var leer, voll int64 = 0, 1200000
	if divergenzVergleichbar(divergenzRuhe-time.Second, 0, &still, &leer) {
		t.Fatal("eigene Last: kein Vergleich")
	}
	if divergenzVergleichbar(divergenzRuhe+time.Second, 0, &beschaeftigt, &leer) {
		t.Fatal("Partner unter Last: kein Vergleich")
	}
	if divergenzVergleichbar(divergenzRuhe+time.Second, 0, nil, &leer) {
		t.Fatal("Partner ohne Auskunft: kein Vergleich -- lieber kein Urteil als ein falscher Resync")
	}
	if !divergenzVergleichbar(divergenzRuhe+time.Second, 0, &still, &leer) {
		t.Fatal("beide still, beide leer: Vergleich zaehlt")
	}
	genau := divergenzRuhe.Seconds()
	if !divergenzVergleichbar(divergenzRuhe, 0, &genau, &leer) {
		t.Fatal("genau an der Grenze zaehlt")
	}
	// 15.09.2026: 1,1 Millionen angenommene, noch nicht verblockte
	// Ueberweisungen auf einer Seite -- sieben Strikes, obwohl beide still
	// waren; zwanzig Minuten spaeter waren die Zustaende von selbst gleich.
	if divergenzVergleichbar(divergenzRuhe+time.Second, 0, &still, &voll) {
		t.Fatal("Partner mit offenem Ausgangskorb: sein Zustand laeuft der Kette voraus -- kein Vergleich")
	}
	if divergenzVergleichbar(divergenzRuhe+time.Second, 5, &still, &leer) {
		t.Fatal("eigener Ausgangskorb nicht leer: kein Vergleich")
	}
	if divergenzVergleichbar(divergenzRuhe+time.Second, 0, &still, nil) {
		t.Fatal("Partner ohne offen-Auskunft (alter Seed): kein Vergleich")
	}
	if divergenzVergleichbar(divergenzRuhe+time.Second, -1, &still, &leer) {
		t.Fatal("eigener Ausgangskorb nicht bestimmbar (-1): kein Vergleich")
	}
}

// Die Auskunft traegt die Bestandteile UND die Ruhe -- ein alter Waechter
// liest weiter account_set_xor, ein neuer zusaetzlich ruhe_seit_s.
func TestDivergenzAuskunftJSON(t *testing.T) {
	body, err := json.Marshal(DivergenzAuskunft{
		StateRootComponents: StateRootComponents{AccountSetXOR: "ab", StateRoot: "cd"},
		RuheSeitS:           42.5,
		Offen:               7,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["account_set_xor"] != "ab" || m["state_root"] != "cd" || m["ruhe_seit_s"] != 42.5 || m["offen"] != 7.0 {
		t.Fatalf("Auskunft unvollstaendig: %s", body)
	}
	var alt struct {
		AccountSetXOR string   `json:"account_set_xor"`
		RuheSeitS     *float64 `json:"ruhe_seit_s"`
	}
	if err := json.Unmarshal([]byte(`{"account_set_xor":"ab"}`), &alt); err != nil || alt.RuheSeitS != nil {
		t.Fatal("ohne Feld muss der Zeiger nil bleiben (alter Seed)")
	}
}
