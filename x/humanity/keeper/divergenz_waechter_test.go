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
	if divergenzVergleichbar(divergenzRuhe-time.Second, &still) {
		t.Fatal("eigene Last: kein Vergleich")
	}
	if divergenzVergleichbar(divergenzRuhe+time.Second, &beschaeftigt) {
		t.Fatal("Partner unter Last: kein Vergleich")
	}
	if divergenzVergleichbar(divergenzRuhe+time.Second, nil) {
		t.Fatal("Partner ohne Auskunft: kein Vergleich -- lieber kein Urteil als ein falscher Resync")
	}
	if !divergenzVergleichbar(divergenzRuhe+time.Second, &still) {
		t.Fatal("beide still: Vergleich zaehlt")
	}
	genau := divergenzRuhe.Seconds()
	if !divergenzVergleichbar(divergenzRuhe, &genau) {
		t.Fatal("genau an der Grenze zaehlt")
	}
}

// Die Auskunft traegt die Bestandteile UND die Ruhe -- ein alter Waechter
// liest weiter account_set_xor, ein neuer zusaetzlich ruhe_seit_s.
func TestDivergenzAuskunftJSON(t *testing.T) {
	body, err := json.Marshal(DivergenzAuskunft{
		StateRootComponents: StateRootComponents{AccountSetXOR: "ab", StateRoot: "cd"},
		RuheSeitS:           42.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["account_set_xor"] != "ab" || m["state_root"] != "cd" || m["ruhe_seit_s"] != 42.5 {
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
