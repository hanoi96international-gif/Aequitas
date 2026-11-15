package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func validatorStandAblageLeeren() {
	validatorStandAblage.mu.Lock()
	validatorStandAblage.eintrag = map[string]validatorStandEintrag{}
	validatorStandAblage.mu.Unlock()
}

func TestValidatorStand_FalscheWalletWirdAbgewiesen(t *testing.T) {
	a := &APIServer{}
	for _, q := range []string{"", "?wallet=", "?wallet=0x123", "?wallet=" + strings.Repeat("a", 42), "?wallet=0x" + strings.Repeat("g", 40)} {
		rec := httptest.NewRecorder()
		a.handleValidatorStand(rec, httptest.NewRequest(http.MethodGet, "/api/validator-stand"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%q: erwartet 400, bekam %d", q, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	a.handleValidatorStand(rec, httptest.NewRequest(http.MethodPost, "/api/validator-stand?wallet=0x"+strings.Repeat("a", 40), nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: erwartet 405, bekam %d", rec.Code)
	}
}

// Missbrauch: viele Anfragen zugleich binden nicht mehr als
// validatorStandGleichzeitig Berechnungen -- der Rest bekommt sofort 503.
func TestValidatorStand_GleichzeitigBegrenzt(t *testing.T) {
	validatorStandAblageLeeren()
	for i := 0; i < validatorStandGleichzeitig; i++ {
		validatorStandPlaetze <- struct{}{}
	}
	defer func() {
		for i := 0; i < validatorStandGleichzeitig; i++ {
			<-validatorStandPlaetze
		}
	}()
	rec := httptest.NewRecorder()
	(&APIServer{}).handleValidatorStand(rec, httptest.NewRequest(http.MethodGet, "/api/validator-stand?wallet=0x"+strings.Repeat("b", 40), nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("alle Plaetze belegt: erwartet 503, bekam %d", rec.Code)
	}
}

// Missbrauch: die Ablage waechst nicht ueber validatorStandCacheMax, auch
// wenn jemand mit immer neuen Wallets fragt.
func TestValidatorStand_AblageBegrenzt(t *testing.T) {
	validatorStandAblageLeeren()
	defer validatorStandAblageLeeren()
	jetzt := time.Now()
	for i := 0; i < validatorStandCacheMax+100; i++ {
		validatorStandAblage.merke(fmt.Sprintf("0x%040x", i), validatorStandAntwort{}, jetzt)
	}
	validatorStandAblage.mu.Lock()
	n := len(validatorStandAblage.eintrag)
	validatorStandAblage.mu.Unlock()
	if n > validatorStandCacheMax {
		t.Fatalf("Ablage hat %d Eintraege, Grenze %d", n, validatorStandCacheMax)
	}
	// Abgelaufene machen Platz.
	spaeter := jetzt.Add(validatorStandCacheDauer + time.Second)
	validatorStandAblage.merke("0x"+strings.Repeat("c", 40), validatorStandAntwort{Wallet: "neu"}, spaeter)
	if v, ok := validatorStandAblage.hole("0x"+strings.Repeat("c", 40), spaeter); !ok || v.Wallet != "neu" {
		t.Fatal("nach Ablauf wurde kein Platz frei")
	}
	if _, ok := validatorStandAblage.hole(fmt.Sprintf("0x%040x", 1), spaeter); ok {
		t.Fatal("abgelaufener Eintrag wird noch ausgeliefert")
	}
}

func TestValidatorStand_BerlinerTag(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("keine Zeitzonendaten")
	}
	for _, f := range []struct{ jetzt, beginn string }{
		{"2026-11-15T19:30:00Z", "2026-11-15T00:00:00+01:00"}, // Winterzeit
		{"2026-11-15T23:30:00Z", "2026-11-16T00:00:00+01:00"}, // nach Berliner Mitternacht
		{"2026-07-01T21:59:00Z", "2026-07-01T00:00:00+02:00"}, // Sommerzeit, 23:59 Berlin
		{"2026-07-01T22:00:00Z", "2026-07-02T00:00:00+02:00"},
	} {
		j, _ := time.Parse(time.RFC3339, f.jetzt)
		b, _ := time.Parse(time.RFC3339, f.beginn)
		if got := berlinerTagBeginn(j); !got.Equal(b) || got.Location().String() != loc.String() {
			t.Fatalf("%s: Tagesbeginn %s, erwartet %s", f.jetzt, got, b)
		}
	}
}

func validatorStandDB(t *testing.T) *ChainState {
	t.Helper()
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		t.Skip("opt-in: AEQUITAS_TPS_BENCH=1 und DATABASE_URL (wegwerfbare lokale Datenbank)")
	}
	cs := testKnoten(t, "unused-validator-stand-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	return cs
}

func TestValidatorStand_EingetragenUndBloecke_RealDB(t *testing.T) {
	cs := validatorStandDB(t)
	wallet := "0x" + strings.Repeat("7a", 20)
	kette := "0x" + strings.Repeat("7b", 20)  // Register der Kette
	knoten := "0x" + strings.Repeat("7c", 20) // nur beim Knoten gebunden
	fremd := "0x" + strings.Repeat("7d", 20)  // Bloecke eines anderen
	t.Cleanup(func() {
		cs.db.Exec(`DELETE FROM validator_register WHERE operator_wallet = $1`, wallet)
		cs.db.Exec(`DELETE FROM validator_keys WHERE human_wallet = $1`, wallet)
		cs.db.Exec(`DELETE FROM chain_blocks WHERE hash LIKE 'vstand-%'`)
	})
	if _, err := cs.db.Exec(`INSERT INTO validator_register (operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing)
		VALUES ($1, $2, 1, 'x', 'y') ON CONFLICT (operator_wallet) DO UPDATE SET signing_address = $2, ueberholt = false`, wallet, kette); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.db.Exec(`INSERT INTO validator_keys (signing_address, human_wallet) VALUES ($1, $2) ON CONFLICT DO NOTHING`, knoten, wallet); err != nil {
		t.Fatal(err)
	}
	jetzt := time.Date(2026, 11, 15, 19, 30, 0, 0, time.UTC)
	tag := berlinerTagBeginn(jetzt).Unix()
	bloecke := []struct {
		proposer string
		zeit     int64
	}{
		{kette, tag - 3600}, {kette, tag - 10}, // gestern
		{strings.ToUpper(kette[:2]) + strings.ToUpper(kette[2:]), tag + 5}, // heute, andere Schreibweise
		{kette, tag + 60}, {kette, tag + 120},
		{knoten, tag + 30},
		{fremd, tag + 40}, {fremd, tag + 50},
	}
	for i, b := range bloecke {
		if _, err := cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, $2, '[]', $3, $4)`,
			fmt.Sprintf("vstand-%d", i), 900000+i, b.proposer, b.zeit); err != nil {
			t.Fatal(err)
		}
	}

	v, err := cs.validatorStandBerechnen(context.Background(), wallet, map[string]bool{kette: true}, jetzt)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Eingetragen || !v.Erzeuger || v.TagBeginn != tag || len(v.Schluessel) != 2 {
		t.Fatalf("Stand falsch: %+v", v)
	}
	k, n := v.Schluessel[0], v.Schluessel[1]
	if k.Adresse != kette || !k.Kette || k.Knoten || !k.Erzeuger || k.BloeckeHeute != 3 || k.BloeckeGesamt != 5 || k.LetzterBlock != tag+120 {
		t.Fatalf("Ketten-Schluessel falsch: %+v", k)
	}
	if n.Adresse != knoten || n.Kette || !n.Knoten || n.Erzeuger || n.BloeckeHeute != 1 || n.BloeckeGesamt != 1 {
		t.Fatalf("Knoten-Schluessel falsch: %+v", n)
	}

	// Eine fremde Wallet sieht weder Schluessel noch Bloecke dieser Wallet.
	leer, err := cs.validatorStandBerechnen(context.Background(), "0x"+strings.Repeat("7e", 20), map[string]bool{kette: true}, jetzt)
	if err != nil {
		t.Fatal(err)
	}
	if leer.Eingetragen || leer.Erzeuger || len(leer.Schluessel) != 0 {
		t.Fatalf("fremde Wallet bekommt einen Stand: %+v", leer)
	}

	// Ueberholt zaehlt nicht als eingetragen (ohne Knotenbindung).
	cs.db.Exec(`DELETE FROM validator_keys WHERE human_wallet = $1`, wallet)
	cs.db.Exec(`UPDATE validator_register SET ueberholt = true WHERE operator_wallet = $1`, wallet)
	u, err := cs.validatorStandBerechnen(context.Background(), wallet, map[string]bool{kette: true}, jetzt)
	if err != nil {
		t.Fatal(err)
	}
	if u.Eingetragen || u.Erzeuger || len(u.Schluessel) != 1 || !u.Schluessel[0].Ueberholt {
		t.Fatalf("ueberholte Bindung gilt noch: %+v", u)
	}
}

// Fail-closed: ist die Datenbank nicht lesbar, kommt ein Fehler (503) --
// nie ein "nicht eingetragen, 0 Bloecke".
func TestValidatorStand_DatenbankFehlerSchliesstAb_RealDB(t *testing.T) {
	cs := validatorStandDB(t)
	ctx, abbruch := context.WithCancel(context.Background())
	abbruch()
	if _, err := cs.validatorStandBerechnen(ctx, "0x"+strings.Repeat("7a", 20), nil, time.Now()); err == nil {
		t.Fatal("abgebrochene Abfrage lieferte einen Stand statt eines Fehlers")
	}

	validatorStandAblageLeeren()
	defer validatorStandAblageLeeren()
	a := &APIServer{blockchain: &BlockDAG{state: cs}}
	cs.db.Close()
	rec := httptest.NewRecorder()
	a.handleValidatorStand(rec, httptest.NewRequest(http.MethodGet, "/api/validator-stand?wallet=0x"+strings.Repeat("7a", 20), nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("geschlossene Datenbank: erwartet 503, bekam %d (%s)", rec.Code, rec.Body.String())
	}
	var j map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &j)
	if _, hat := j["eingetragen"]; hat {
		t.Fatal("Fehlerantwort traegt einen Stand")
	}
}
