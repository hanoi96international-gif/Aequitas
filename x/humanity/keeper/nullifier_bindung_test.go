package keeper

import (
	"math/big"
	"testing"
)

// Der Angriff vom 29.09.2026 (Audit H1): ein ehrlicher /prove liefert N; eine
// zweite Anmeldung nennt "0x"+N, traegt aber einen selbst erzeugten Beweis mit
// eigenem pubSignals[1]. Beides muss scheitern: die Herkunft von N gilt nicht
// fuer "0x"+N, und ein angegebener Nullifier, der nicht der des Beweises ist,
// wird abgewiesen.
const ehrlicherNullifier = "17579322874185201837465019283746501928374650192837465019283746501928374"

func TestRegistrierung_NullifierMussDerDesBeweisesSein(t *testing.T) {
	fremd := "4242424242424242424242424242424242424242424242424242424242424242424242"
	if _, err := registrierungsNullifier("0x"+ehrlicherNullifier, []string{"1", fremd}); err == nil {
		t.Fatal(`"0x"+N mit fremdem Beweis wurde angenommen -- ein Mensch, zwei Konten`)
	}
	if _, err := registrierungsNullifier(ehrlicherNullifier, []string{"1", fremd}); err == nil {
		t.Fatal("N mit fremdem Beweis wurde angenommen")
	}
	if _, err := registrierungsNullifier(ehrlicherNullifier, []string{"1"}); err == nil {
		t.Fatal("ein Beweis ohne pubSignals[1] wurde angenommen")
	}
	got, err := registrierungsNullifier(ehrlicherNullifier, []string{"1", ehrlicherNullifier})
	if err != nil || got != ehrlicherNullifier {
		t.Fatalf("der ehrliche Fall scheiterte: %q, %v", got, err)
	}
	// Dieselbe ZAHL in Hex-Schreibweise ist derselbe Nullifier -- kanonisch
	// kommt immer die Dezimalform heraus.
	n, _ := new(big.Int).SetString(ehrlicherNullifier, 10)
	got, err = registrierungsNullifier("0x"+n.Text(16), []string{"1", ehrlicherNullifier})
	if err != nil || got != ehrlicherNullifier {
		t.Fatalf("Hex-Schreibweise derselben Zahl: %q, %v", got, err)
	}
}

func TestHerkunft_GiltFuerDieZahlNichtFuerDieSchreibweise(t *testing.T) {
	const w = "0x1111111111111111111111111111111111111111"
	merkeProveHerkunft([]byte(`{"wallet":"`+w+`"}`), []byte(`{"zkNullifier":"`+ehrlicherNullifier+`","circuitVersion":3}`))
	if !hatProveHerkunft(ehrlicherNullifier, w) {
		t.Fatal("nach einem erfolgreichen /prove muss die Herkunft stehen")
	}
	if hatProveHerkunft("0x"+ehrlicherNullifier, w) {
		t.Fatal(`"0x"+N ist eine andere Zahl und darf die Herkunft von N nicht erben`)
	}
	n, _ := new(big.Int).SetString(ehrlicherNullifier, 10)
	if !hatProveHerkunft("0x"+n.Text(16), w) {
		t.Fatal("dieselbe Zahl in Hex-Schreibweise muss die Herkunft haben")
	}
	if hatProveHerkunft("kein-nullifier", w) {
		t.Fatal("Unlesbares darf nie als Herkunft gelten")
	}
}

func TestNullifierBytes32_UeberlangeIstFehler(t *testing.T) {
	// 78 Dezimalziffern als Hex gelesen: mehr als 32 Byte. Frueher wurde das
	// still zu null -- und der Vertrag nahm null als "keine Angabe".
	if _, err := nullifierBytes32("0x" + ehrlicherNullifier + "12345678"); err == nil {
		t.Fatal("ein Nullifier ueber 32 Byte muss ein Fehler sein, nicht null")
	}
}
