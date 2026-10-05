package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func summe(c byte) string { return strings.Repeat(string(c), 64) }

func sp(s string) *string { return &s }

// Treuhand und Register zaehlen mit: zwei Knoten mit gleichen Konten, aber
// verschiedener Treuhand oder verschiedenem Register sind NICHT gleich. Ein
// fehlendes Feld (aelterer Seed) ist kein leeres: dieser Teil wird nicht
// verglichen.
func TestDivergenzAbweichung_TreuhandUndRegister(t *testing.T) {
	eigene := StateRootComponents{AccountSetXOR: summe('a'), EscrowSetXOR: summe('b'), ValidatorSetXOR: summe('c')}
	gleich := divergenzAuskunftFremd{AccountSetXOR: summe('a'), EscrowSetXOR: sp(summe('b')), ValidatorSetXOR: sp(summe('c'))}
	if teile, text := divergenzAbweichung(eigene, gleich); len(teile) != 0 {
		t.Fatalf("gleicher Zustand gemeldet: %s", text)
	}
	leer := StateRootComponents{AccountSetXOR: summe('a')}
	if teile, _ := divergenzAbweichung(leer, divergenzAuskunftFremd{AccountSetXOR: summe('a'), EscrowSetXOR: sp(""), ValidatorSetXOR: sp("")}); len(teile) != 0 {
		t.Fatal("leer gegen leer gemeldet")
	}
	for name, fall := range map[string]struct {
		fremd divergenzAuskunftFremd
		teil  string
	}{
		"Konten":   {divergenzAuskunftFremd{AccountSetXOR: summe('0'), EscrowSetXOR: sp(summe('b')), ValidatorSetXOR: sp(summe('c'))}, "account_set_xor"},
		"Treuhand": {divergenzAuskunftFremd{AccountSetXOR: summe('a'), EscrowSetXOR: sp(summe('0')), ValidatorSetXOR: sp(summe('c'))}, "escrow_set_xor"},
		// Neuer Seed mit LEEREM Register gegen einen Knoten mit Register: abgewichen.
		"Register leer": {divergenzAuskunftFremd{AccountSetXOR: summe('a'), EscrowSetXOR: sp(summe('b')), ValidatorSetXOR: sp("")}, "validator_set_xor"},
	} {
		teile, _ := divergenzAbweichung(eigene, fall.fremd)
		if len(teile) != 1 || teile[0] != fall.teil {
			t.Fatalf("%s: erkannt %v, erwartet %s", name, teile, fall.teil)
		}
	}
	// Alter Seed (Felder fehlen) gegen einen Knoten mit Treuhand und Register:
	// kein Urteil ueber diese Teile, also kein Strike.
	if teile, text := divergenzAbweichung(eigene, divergenzAuskunftFremd{AccountSetXOR: summe('a')}); len(teile) != 0 {
		t.Fatalf("alter Seed als Abweichung gezaehlt: %s", text)
	}
	// Leerer Knoten gegen Seed mit Treuhand: abgewichen.
	if teile, _ := divergenzAbweichung(leer, gleich); len(teile) != 2 {
		t.Fatalf("leerer Knoten gegen Seed mit Treuhand und Register: %v", teile)
	}
}

// Was ein Seed liefert, geht durch divergenzAuskunftLesen. Missbrauch: zu
// gross, keine Summe, Steuerzeichen -- jeweils kein Urteil (Fehler), nie ein
// Strike und nie ungeprueft ins Log.
func TestDivergenzAuskunftLesen_FeindlicheAntworten(t *testing.T) {
	gut := `{"account_set_xor":"` + summe('a') + `","escrow_set_xor":"","validator_set_xor":"` + summe('c') + `","ruhe_seit_s":40,"offen":0}`
	f, err := divergenzAuskunftLesen(strings.NewReader(gut))
	if err != nil || f.EscrowSetXOR == nil || *f.EscrowSetXOR != "" || f.ValidatorSetXOR == nil || *f.ValidatorSetXOR != summe('c') {
		t.Fatalf("gueltige Auskunft: %+v, %v", f, err)
	}
	alt, err := divergenzAuskunftLesen(strings.NewReader(`{"account_set_xor":"` + summe('a') + `"}`))
	if err != nil || alt.EscrowSetXOR != nil || alt.ValidatorSetXOR != nil {
		t.Fatalf("alter Seed: Felder muessen nil bleiben: %+v, %v", alt, err)
	}
	for name, body := range map[string]string{
		"zu gross":       `{"account_set_xor":"` + summe('a') + `","last_ubi_at":"` + strings.Repeat("x", divergenzAuskunftGrenze) + `"}`,
		"keine Summe":    `{"account_set_xor":"` + summe('a') + `","escrow_set_xor":"zz"}`,
		"Gross-Hex":      `{"account_set_xor":"` + strings.ToUpper(summe('a')) + `"}`,
		"Steuerzeichen":  `{"account_set_xor":"` + summe('a') + `","validator_set_xor":"\n[FAKE] ok\u001b[` + strings.Repeat("a", 50) + `"}`,
		"Konten fehlen":  `{"escrow_set_xor":"` + summe('b') + `"}`,
		"Konten zu kurz": `{"account_set_xor":"abc"}`,
		"kein JSON":      `<html>`,
	} {
		if _, err := divergenzAuskunftLesen(strings.NewReader(body)); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
}

// Abruf: nur Status 200, keine Weiterleitung (sonst lenkte ein Seed den
// Knoten auf jede Adresse), Groesse begrenzt auch bei endlosem Strom.
func TestDivergenzAuskunftHolen_StatusUndWeiterleitung(t *testing.T) {
	var umgeleitetAufgerufen bool
	ziel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		umgeleitetAufgerufen = true
		w.Write([]byte(`{"account_set_xor":"` + summe('a') + `"}`))
	}))
	defer ziel.Close()
	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/umleitung":
			http.Redirect(w, r, ziel.URL+"/x", http.StatusFound)
		case "/fehler":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"account_set_xor":"` + summe('a') + `"}`))
		case "/endlos":
			w.Write([]byte(`{"account_set_xor":"` + summe('a') + `","last_ubi_at":"`))
			chunk := []byte(strings.Repeat("x", 4096))
			for i := 0; i < 1<<12; i++ { // 16 MB, wenn niemand abbricht
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
		default:
			w.Write([]byte(`{"account_set_xor":"` + summe('a') + `"}`))
		}
	}))
	defer seed.Close()
	hc := divergenzKlient()
	if _, err := divergenzAuskunftHolen(hc, seed.URL+"/gut"); err != nil {
		t.Fatalf("gueltige Auskunft: %v", err)
	}
	if _, err := divergenzAuskunftHolen(hc, seed.URL+"/umleitung"); err == nil || umgeleitetAufgerufen {
		t.Fatalf("Weiterleitung gefolgt (aufgerufen=%v, err=%v)", umgeleitetAufgerufen, err)
	}
	if _, err := divergenzAuskunftHolen(hc, seed.URL+"/fehler"); err == nil {
		t.Fatal("Status 500 angenommen")
	}
	if _, err := divergenzAuskunftHolen(hc, seed.URL+"/endlos"); err == nil {
		t.Fatal("endlose Auskunft angenommen")
	}
}

// Ein Resync, der die Abweichung nicht behebt, loest keinen zweiten aus --
// erst wenn ein Vergleich wieder gleich ausging (Reset) oder andere Teile
// abweichen.
func TestDivergenzResyncEntscheiden_KeineSchleife(t *testing.T) {
	divergenzResyncTeile.Store("")
	t.Cleanup(func() { divergenzResyncTeile.Store(""); divergenzStrikes.Store(0) })
	if divergenzResyncEntscheiden(false, []string{"escrow_set_xor"}) {
		t.Fatal("ohne Erlaubnis ausgeloest")
	}
	divergenzStrikes.Store(divergenzSchwelle)
	if !divergenzResyncEntscheiden(true, []string{"escrow_set_xor"}) {
		t.Fatal("erste belegte Abweichung: Resync erwartet")
	}
	if divergenzStrikes.Load() != 0 {
		t.Fatal("nach dem Ausloesen beginnt die Zaehlung nicht neu")
	}
	if divergenzResyncEntscheiden(true, []string{"escrow_set_xor"}) {
		t.Fatal("dieselbe Abweichung nach dem Resync: kein zweiter")
	}
	if !divergenzResyncEntscheiden(true, []string{"account_set_xor"}) {
		t.Fatal("andere Teile: Resync erwartet")
	}
	divergenzResyncTeile.Store("") // ein gleicher Vergleich setzt zurueck
	if !divergenzResyncEntscheiden(true, []string{"account_set_xor"}) {
		t.Fatal("nach einem gleichen Vergleich: Resync wieder erlaubt")
	}
}

// Die Auskunft traegt Treuhand und Register IMMER, auch leer -- sonst
// unterschiede ein neuer Waechter "leer" nicht von "aelterer Seed".
func TestStateRootComponents_FelderImmerDa(t *testing.T) {
	body, err := json.Marshal(StateRootComponents{AccountSetXOR: summe('a')})
	if err != nil {
		t.Fatal(err)
	}
	for _, feld := range []string{`"escrow_set_xor":""`, `"validator_set_xor":""`} {
		if !strings.Contains(string(body), feld) {
			t.Fatalf("%s fehlt in %s", feld, body)
		}
	}
}
