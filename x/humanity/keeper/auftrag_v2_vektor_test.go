package keeper

import (
	"os"
	"strings"
	"testing"
)

// Feste Vektoren fuer Nachrichtenform 2. Dieselben Zeichenketten stehen in
// der App (Aequitas-App, lib/__tests__/auftrag.test.ts) und muessen dort
// identisch herauskommen -- sonst scheitert jede Unterschrift am Knoten.
// Der Betrag 0.00195313 ist 2^-9 = 0.001953125 auf 8 Stellen gerundet: roh
// geschickt ergaebe Go "%.8f" 0.00195312, JavaScript toFixed(8) 0.00195313.
func TestAuftragForm2_FesteVektoren(t *testing.T) {
	const w = "0xAbCdEf0123456789aBcDeF0123456789AbCdEf01"
	faelle := []struct{ got, want string }{
		{tauschNachrichtV2(w, "aeq_to_tusd", 12.5, 12.37500001, 42, 1794748500),
			"aequitas-tausch-v2|chain:1926|0xabcdef0123456789abcdef0123456789abcdef01|aeq_to_tusd|12.50000000|min:12.37500001|nonce:42|ts:1794748500"},
		{tauschNachrichtV2(w, "tusd_to_aeq", 0.00195313, 0, 1, 1794748501),
			"aequitas-tausch-v2|chain:1926|0xabcdef0123456789abcdef0123456789abcdef01|tusd_to_aeq|0.00195313|min:0.00000000|nonce:1|ts:1794748501"},
		{liquiditaetNachrichtV2(w, 100, 99.8003, 3, 1794748500),
			"aequitas-liquiditaet-v2|chain:1926|0xabcdef0123456789abcdef0123456789abcdef01|100.00000000|99.80030000|nonce:3|ts:1794748500"},
		{liquiditaetAbNachrichtV2(w, 0.12345678, 4, 1794748500),
			"aequitas-liquiditaet-ab-v2|chain:1926|0xabcdef0123456789abcdef0123456789abcdef01|0.12345678|nonce:4|ts:1794748500"},
	}
	for i, f := range faelle {
		if f.got != f.want {
			t.Errorf("Vektor %d:\n got %s\nwant %s", i, f.got, f.want)
		}
	}
}

// Website und Explorer unterschreiben Tausch und Liquiditaet in Form 2 und
// schicken form: 2 mit -- die alte Nachricht nimmt die Annahme ab dem
// 01.12.2026 nicht mehr an. Hier: die alte Form steht nirgends mehr, die
// neue samt form: 2 ist da (die Vektoren oben wurden mit node gegen beide
// Dateien nachgerechnet).
func TestAuftragForm2_WebseiteUnterschreibtForm2(t *testing.T) {
	for _, f := range []string{"assets/explorer.js", "assets/dapp.js"} {
		roh, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(roh)
		for _, alt := range []string{"'Aequitas Swap: '", "'Aequitas Add Liquidity: '", "'Aequitas Remove Liquidity: '", "'Aequitas: recover escrow '"} {
			if strings.Contains(s, alt) {
				t.Errorf("%s unterschreibt noch Form 1 (%s)", f, alt)
			}
		}
		for _, neu := range []string{"'aequitas-tausch-v2|chain:1926|'", "form: AUFTRAG_FORM", "const AUFTRAG_FORM = 2;"} {
			if !strings.Contains(s, neu) {
				t.Errorf("%s: %s fehlt", f, neu)
			}
		}
	}
}
