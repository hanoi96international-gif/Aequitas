package keeper

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// PRUEFVEKTOR: AUSGABE GEGEN DAS LOS AUS DER APP (Weg B, B4b-4c-2e).
//
// Die App als Ausgeber (lib/quantenSAusgeberRunde.ts, ausgeben) reicht
// quanten_w_s_ausgeben ein: Validatoren = die Gelosten in Losreihenfolge, ihre
// Bestaetigungen, Deckung wie bestellt, Inhaberin, Annahmestellen, Proben v2,
// Zeit des Rahmens. Der Koerper entsteht in der App Ende zu Ende (dieselbe
// Ausgabe wie die Vektoren aus 2c und 2d). Hier prueft der Knoten mit SEINEM
// Code: den Koerper wie handleQuantenW (keine fremden Felder, nur die Felder
// dieser Art), die Unterschrift der Wallet (quantenWSAusgabeNachrichtArt),
// Wurzel und Proben wie in 2c/2d, und quantenSAusgabeGegenLos gegen eine
// Bestellung mit genau diesem Los -- samt Gegenproben (anderes Los, andere
// Deckung, fremde Wallet, vertauschte Bestaetigungen, verfallen, ungelost).

type qsAusgeberAusgebenVektor struct {
	Hinweis         string `json:"hinweis"`
	AusgeberSchl    string `json:"ausgeber_schluessel_nur_test"`
	BestellZeit     int64  `json:"bestell_zeit"`
	KoerperAusgeben string `json:"koerper_ausgeben"`
}

func TestQuantenSAusgeberAusgebenVektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_ausgeber_ausgeben_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qsAusgeberAusgebenVektor
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	tx := qsKoerperLesen(t, quantenWSAusgebenTxArt, v.KoerperAusgeben)
	q := *tx.QuantenW
	if !quantenWNurEigeneFelder(&tx) {
		t.Fatal("Koerper: Felder, die nicht zu quanten_w_s_ausgeben gehoeren")
	}
	_, ausgeber, _ := qsAuftragSchluessel(t, v.AusgeberSchl[:2])
	if tx.Wallet != ausgeber || q.ProbeArt != 2 || len(q.Annahme) != 2 || len(q.Proben) != 2 {
		t.Fatal("Koerper: Wallet, Stichprobe oder Stellen")
	}
	m := quantenWSAusgabeNachrichtArt(tx.Wallet, q.K, q.N, q.T, q.Wurzel, q.Validatoren, q.Proben, q.Annahme, q.Inhaber, q.Deckung, tx.DistributionAt, q.Tol, q.ProbeArt)
	if !kanonischeSignatur(q.WalletSig) || verifyPersonalSign(m, q.WalletSig, ausgeber) != nil {
		t.Fatal("Unterschrift der Wallet")
	}
	// Dieselbe Ausgabe wie 2c und 2d.
	a := qsAusgeberAuftragVektorLesen(t)
	if q.K != a.K || q.N != a.N || q.T != a.T || q.Tol != a.Tol || q.Wurzel != a.Wurzel || q.Proben[0] != a.Proben[0] || q.Proben[1] != a.Proben[1] ||
		tx.DistributionAt != a.Auftrag.Zeit || q.Inhaber != a.Auftrag.Inhaberin || q.Annahme[0] != a.Auftrag.Stellen[0] || q.Annahme[1] != a.Auftrag.Stellen[1] {
		t.Fatal("Ausgabe passt nicht zu 2c")
	}
	// Gegen das Los, mit der Pruefung der Kette.
	wurzel, ok := quantenWHex32(q.Wurzel)
	if !ok {
		t.Fatal("Wurzel")
	}
	var proben [2][32]byte
	for s := 0; s < 2; s++ {
		if proben[s], ok = quantenWHex32(q.Proben[s]); !ok {
			t.Fatal("Probe")
		}
	}
	nachricht := func(x int) string {
		return quantenWSBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, x+1, proben, tx.DistributionAt, q.Tol)
	}
	los := append([]string(nil), a.Los...)
	bestellung := func() *quantenSBestellung {
		return &quantenSBestellung{Ausgeber: ausgeber, V: len(los), T: q.T, Zeit: v.BestellZeit, Deckung: q.Deckung, Runde: 1, Los: append([]string(nil), los...)}
	}
	if b := quantenSAusgabeGegenLos(bestellung(), tx.Wallet, &q, tx.DistributionAt, nachricht); b.regel != "" {
		t.Fatalf("Ausgabe gegen das Los abgewiesen: %s %s", b.regel, b.text)
	}
	gegen := map[string]func(b *quantenSBestellung, w *string, q2 *QuantenWAngaben, zeit *int64){
		"quanten_s_los_fremd": func(b *quantenSBestellung, _ *string, _ *QuantenWAngaben, _ *int64) {
			b.Los[0], b.Los[1] = b.Los[1], b.Los[0]
		},
		"quanten_s_los_ausgeber":        func(b *quantenSBestellung, _ *string, _ *QuantenWAngaben, _ *int64) { b.Deckung++ },
		"quanten_s_los_ausgeber/wallet": func(_ *quantenSBestellung, w *string, _ *QuantenWAngaben, _ *int64) { *w = a.Los[0] },
		"quanten_w_bestaetigung": func(_ *quantenSBestellung, _ *string, q2 *QuantenWAngaben, _ *int64) {
			q2.Bestaetigungen[0], q2.Bestaetigungen[1] = q2.Bestaetigungen[1], q2.Bestaetigungen[0]
		},
		"quanten_w_frist": func(_ *quantenSBestellung, _ *string, _ *QuantenWAngaben, zeit *int64) {
			*zeit = v.BestellZeit + quantenSBestellFrist + 1
		},
		"quanten_s_los_offen": func(b *quantenSBestellung, _ *string, _ *QuantenWAngaben, _ *int64) { b.Runde = 0 },
	}
	for name, aendern := range gegen {
		b, w, q2, zeit := bestellung(), tx.Wallet, q, tx.DistributionAt
		q2.Bestaetigungen = append([]string(nil), q.Bestaetigungen...)
		aendern(b, &w, &q2, &zeit)
		if got := quantenSAusgabeGegenLos(b, w, &q2, zeit, nachricht).regel; got != strings.Split(name, "/")[0] {
			t.Fatalf("Gegenprobe %s: Regel %q", name, got)
		}
	}
}
