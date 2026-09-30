package keeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func adrNr(i int) string { return fmt.Sprintf("0x%040x", i) }

func TestBindungsAblage_GutfallUndAbholen(t *testing.T) {
	b := neueBindungsAblage()
	jetzt := time.Unix(1_800_000_000, 0)
	b.merke("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", "0xsig", jetzt)
	e, ok := b.hole("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", jetzt)
	if !ok || e.Wallet != "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || e.Signatur != "0xsig" {
		t.Fatalf("abgelegte Bindung nicht (richtig) abholbar: %+v %v", e, ok)
	}
}

// Missbrauch: eine Wallet reicht fuer immer neue Signierschluessel Bindungen
// ein, um die Eintraege anderer zu verdraengen. Je Wallet bleibt nur einer.
func TestBindungsAblage_EineWalletEinEintrag(t *testing.T) {
	b := neueBindungsAblage()
	jetzt := time.Unix(1_800_000_000, 0)
	fremd := adrNr(999_999)
	b.merke(fremd, adrNr(7), "0xfremd", jetzt)
	for i := 0; i < 3*bindungsAblageMax; i++ {
		b.merke(adrNr(i+1), adrNr(42), "0xspam", jetzt.Add(time.Duration(i)*time.Second))
	}
	if len(b.nachAdr) != 2 || len(b.nachWallet) != 2 {
		t.Fatalf("eine Wallet belegt mehr als einen Eintrag: %d Adressen, %d Wallets", len(b.nachAdr), len(b.nachWallet))
	}
	if _, ok := b.hole(fremd, jetzt); !ok {
		t.Fatal("die Bindung eines anderen Menschen wurde verdraengt")
	}
	if _, ok := b.hole(adrNr(1), jetzt); ok {
		t.Fatal("die ersetzte Bindung derselben Wallet ist noch abholbar")
	}
	if e, ok := b.hole(adrNr(3*bindungsAblageMax), jetzt); !ok || e.Wallet != adrNr(42) {
		t.Fatal("die juengste Bindung der Wallet fehlt")
	}
}

// Ueberlauf: viele verschiedene Wallets. Die Ablage bleibt begrenzt, der
// aelteste Eintrag weicht.
func TestBindungsAblage_Obergrenze(t *testing.T) {
	b := neueBindungsAblage()
	jetzt := time.Unix(1_800_000_000, 0)
	for i := 0; i < bindungsAblageMax+50; i++ {
		b.merke(adrNr(i+1), adrNr(100_000+i), "0xs", jetzt.Add(time.Duration(i)*time.Second))
	}
	if len(b.nachAdr) != bindungsAblageMax || len(b.nachWallet) != bindungsAblageMax {
		t.Fatalf("Ablage nicht begrenzt: %d/%d, erwartet %d", len(b.nachAdr), len(b.nachWallet), bindungsAblageMax)
	}
	spaeter := jetzt.Add(time.Hour)
	if _, ok := b.hole(adrNr(1), spaeter); ok {
		t.Fatal("der aelteste Eintrag haette weichen muessen")
	}
	if _, ok := b.hole(adrNr(bindungsAblageMax+50), spaeter); !ok {
		t.Fatal("der juengste Eintrag fehlt")
	}
}

func TestBindungsAblage_Verfall(t *testing.T) {
	b := neueBindungsAblage()
	jetzt := time.Unix(1_800_000_000, 0)
	b.merke(adrNr(1), adrNr(2), "0xs", jetzt)
	if _, ok := b.hole(adrNr(1), jetzt.Add(bindungsAblageDauer+time.Second)); ok {
		t.Fatal("abgelaufene Bindung ist noch abholbar")
	}
	if len(b.nachAdr) != 0 || len(b.nachWallet) != 0 {
		t.Fatal("abgelaufener Eintrag bleibt im Speicher")
	}
}

func TestBindungsAblage_LeereWerteWerdenNichtAbgelegt(t *testing.T) {
	b := neueBindungsAblage()
	jetzt := time.Unix(1_800_000_000, 0)
	b.merke("", adrNr(2), "0xs", jetzt)
	b.merke(adrNr(1), "", "0xs", jetzt)
	b.merke(adrNr(1), adrNr(2), "", jetzt)
	if len(b.nachAdr) != 0 {
		t.Fatalf("unvollstaendige Bindung abgelegt: %d", len(b.nachAdr))
	}
}

func bindungAbfrage(t *testing.T, methode, url string) (*httptest.ResponseRecorder, map[string]string) {
	t.Helper()
	rec := httptest.NewRecorder()
	(&APIServer{}).handleValidatorBinding(rec, httptest.NewRequest(methode, url, nil))
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestHandleValidatorBinding(t *testing.T) {
	alt := bindungsAblage
	bindungsAblage = neueBindungsAblage()
	defer func() { bindungsAblage = alt }()

	adr := "0x1111111111111111111111111111111111111111"
	if rec, _ := bindungAbfrage(t, "GET", "/api/validator-binding?signing_address="+adr); rec.Code != http.StatusNotFound {
		t.Fatalf("ohne Bindung: HTTP %d, erwartet 404", rec.Code)
	}
	for _, schlecht := range []string{"", "0x123", "0xZZZZ111111111111111111111111111111111111", "1111111111111111111111111111111111111111aa"} {
		if rec, _ := bindungAbfrage(t, "GET", "/api/validator-binding?signing_address="+schlecht); rec.Code != http.StatusBadRequest {
			t.Fatalf("ungueltige Adresse %q: HTTP %d, erwartet 400", schlecht, rec.Code)
		}
	}
	if rec, _ := bindungAbfrage(t, "POST", "/api/validator-binding?signing_address="+adr); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: HTTP %d, erwartet 405", rec.Code)
	}

	bindungsAblage.merke(adr, "0x2222222222222222222222222222222222222222", "0xabc", time.Now())
	rec, body := bindungAbfrage(t, "GET", "/api/validator-binding?signing_address="+adr)
	if rec.Code != http.StatusOK || body["human_signature"] != "0xabc" ||
		body["human_wallet"] != "0x2222222222222222222222222222222222222222" || body["signing_address"] != adr {
		t.Fatalf("Abholung: HTTP %d, %v", rec.Code, body)
	}
}

// Faelschung: eine Einreichung mit falscher Menschen-Signatur wird abgewiesen
// und landet NICHT in der Ablage -- einrichten.sh bekaeme sonst eine
// Bindung, die der Primary beim Anmelden verwirft.
func TestRegisterValidatorKey_FaelschungKommtNichtInDieAblage(t *testing.T) {
	alt := bindungsAblage
	bindungsAblage = neueBindungsAblage()
	defer func() { bindungsAblage = alt }()

	koerper := `{"signing_address":"0x1111111111111111111111111111111111111111",` +
		`"human_wallet":"0x2222222222222222222222222222222222222222",` +
		`"human_signature":"0x` + strings.Repeat("ab", 65) + `",` +
		`"signing_key_signature":"0x` + strings.Repeat("cd", 65) + `"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/register-validator-key", strings.NewReader(koerper))
	(&APIServer{}).handleRegisterValidatorKey(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("gefaelschte Bindung: HTTP %d, erwartet 400 (%s)", rec.Code, rec.Body.String())
	}
	if len(bindungsAblage.nachAdr) != 0 {
		t.Fatal("eine gefaelschte Bindung liegt in der Ablage")
	}
}
