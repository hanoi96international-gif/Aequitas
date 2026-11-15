package keeper

import (
	"math"
	"strings"
	"testing"
)

// B15: Form 2 bindet Kette, Wallet und Mindestbetrag. Jede Aenderung
// unterwegs (Grenze gesenkt, fremde Wallet, andere Kette, Ergebnis unter der
// Grenze, Vorbehalt mit anderer Grenze) faellt beim Nachspielen durch.
func TestAuftragForm2_BindetGrenzeWalletKette(t *testing.T) {
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	jetzt := int64(1_800_000_000)
	msg := tauschNachrichtV2(a.addr, "aeq_to_tusd", 10, 9.5, 7, jetzt)
	if !strings.HasPrefix(msg, "aequitas-tausch-v2|chain:1926|"+a.addr+"|aeq_to_tusd|10.00000000|min:9.50000000|nonce:7|ts:") {
		t.Fatalf("Nachrichtenform: %s", msg)
	}
	tausch := func(aus, min float64) Transaction {
		return Transaction{Type: "swap_aeq_tusd", Wallet: a.addr, Amount: 10, AmountOut: aus,
			Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, a, msg), Nonce: 7, Zeit: jetzt, Betrag: 10, Form: auftragForm2, Min: min}}
	}
	gut := tausch(9.6, 9.5)
	if err := pruefeAuftragsNachweis(&gut, jetzt+5); err != nil {
		t.Fatalf("gueltiger Tausch der Form 2 abgewiesen: %v", err)
	}
	faelle := map[string]Transaction{
		"Grenze unterwegs gesenkt":    tausch(9.6, 0),
		"Ergebnis unter der Grenze":   tausch(9.4, 9.5),
		"Grenze NaN":                  tausch(9.6, math.NaN()),
		"unbekannte Form":             func() Transaction { x := tausch(9.6, 9.5); x.Nachweis.Form = 3; return x }(),
		"Form 1 mit Grenze im Feld":   func() Transaction { x := tausch(9.6, 9.5); x.Nachweis.Form = 0; return x }(),
		"fremde Wallet traegt es ein": func() Transaction { x := tausch(9.6, 9.5); x.Wallet = b.addr; return x }(),
	}
	// andere Kette: dieselbe Nachricht mit anderer chain-Kennung unterschrieben
	fremdeKette := strings.Replace(msg, "chain:1926", "chain:1", 1)
	faelle["Unterschrift fuer andere Kette"] = Transaction{Type: "swap_aeq_tusd", Wallet: a.addr, Amount: 10, AmountOut: 9.6,
		Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, a, fremdeKette), Nonce: 7, Zeit: jetzt, Betrag: 10, Form: auftragForm2, Min: 9.5}}
	// Vorbehalt: die Grenze im Vorbehalt muss die unterschriebene sein
	vb := func(minOut float64) Transaction {
		x := tausch(0, 9.5)
		x.Type = "vorbehalt"
		x.Vorbehalt = &VorbehaltAngaben{Art: "swap_aeq_tusd", AEQ: 10.2, MinOut: minOut}
		return x
	}
	vbGut := vb(9.5)
	if err := pruefeAuftragsNachweis(&vbGut, jetzt+5); err != nil {
		t.Fatalf("Vorbehalt der Form 2 abgewiesen: %v", err)
	}
	faelle["Vorbehalt mit gesenkter Grenze"] = vb(0)
	for name, tx := range faelle {
		tx := tx
		if err := pruefeAuftragsNachweis(&tx, jetzt+5); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}

	// Liquiditaet: Wallet und Kette gebunden
	lm := liquiditaetNachrichtV2(a.addr, 10, 20, 4, jetzt)
	liq := Transaction{Type: "add_liquidity", Wallet: a.addr, Amount: 10, AmountOut: 20,
		Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, a, lm), Nonce: 4, Zeit: jetzt, Betrag: 10, Betrag2: 20, Form: auftragForm2}}
	if err := pruefeAuftragsNachweis(&liq, jetzt+5); err != nil {
		t.Fatalf("Liquiditaet Form 2: %v", err)
	}
	liq.Nachweis.Form = 0
	if err := pruefeAuftragsNachweis(&liq, jetzt+5); err == nil {
		t.Fatal("Unterschrift der Form 2 als Form 1 angenommen")
	}
	am := liquiditaetAbNachrichtV2(a.addr, 0.5, 5, jetzt)
	ab := Transaction{Type: "remove_liquidity", Wallet: a.addr, Amount: 0.5,
		Nachweis: &Auftragsnachweis{Sig: persoenlichSignieren(t, a, am), Nonce: 5, Zeit: jetzt, Betrag: 0.5, Form: auftragForm2}}
	if err := pruefeAuftragsNachweis(&ab, jetzt+5); err != nil {
		t.Fatalf("Abziehen Form 2: %v", err)
	}
	ab.Nachweis.Min = 1
	if err := pruefeAuftragsNachweis(&ab, jetzt+5); err == nil {
		t.Fatal("Abziehen mit fremdem Feld angenommen")
	}
}

// B15: die Annahme nimmt Form 1 nur bis zum Stichtag, unbekannte Formen nie.
func TestAuftragForm_AnnahmeStichtag(t *testing.T) {
	if err := auftragFormAnnehmbar(0, auftragForm1AnnahmeBisUnix-1); err != nil {
		t.Fatalf("Form 1 vor dem Stichtag: %v", err)
	}
	if err := auftragFormAnnehmbar(0, auftragForm1AnnahmeBisUnix); err == nil {
		t.Fatal("Form 1 nach dem Stichtag angenommen")
	}
	if err := auftragFormAnnehmbar(auftragForm2, auftragForm1AnnahmeBisUnix+1); err != nil {
		t.Fatalf("Form 2: %v", err)
	}
	for _, f := range []int{-1, 3, 99} {
		if err := auftragFormAnnehmbar(f, 0); err == nil {
			t.Errorf("Form %d angenommen", f)
		}
	}
	for _, m := range []float64{-1, math.NaN(), math.Inf(1)} {
		if gueltigeMindestgrenze(m) {
			t.Errorf("Grenze %v angenommen", m)
		}
	}
}
