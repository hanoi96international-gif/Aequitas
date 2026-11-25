package keeper

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// PRUEFVEKTOR: BESTAETIGUNGS-ANFRAGE DES AUSGEBERS UND DIE BESTAETIGUNGEN DER
// GELOSTEN (Weg B, B4b-4c-2d).
//
// Die App als Ausgeber (lib/quantenSAusgeberRunde.ts) schickt jeder Gelosten
// die Bestaetigungs-Anfrage (QuantenWBestaetigenAnfrage, Art "s") mit allen
// Blaettern, Wurzel, Proben und den Buendeln der anderen an sie; die Gelosten
// pruefen mit ihrem eigenen Ablauf und bestaetigen. Der Vektor entsteht in der
// App Ende zu Ende (dieselbe Ausgabe wie quanten_s_ausgeber_auftrag_vektor.json);
// Blaetter und Buendel stehen nur als Hash darin (Groesse). Der Knoten prueft
// mit SEINEM Code: die Anfrage wie der Dienst sie liest (keine fremden
// Felder), die Nachricht des Auftrags (quantenWSAuftragNachricht mit dem Hash
// der Buendel) und die Unterschrift des Ausgebers, Wurzel und Blaetter gegen
// den Vektor aus 2c, und jede Bestaetigung (quantenWSBestaetigungNachricht)
// von genau ihrer Gelosten -- so, wie die Kette sie bei der Ausgabe prueft.

type qsAusgeberBestaetigenVektor struct {
	Hinweis        string                     `json:"hinweis"`
	AusgeberSchl   string                     `json:"ausgeber_schluessel_nur_test"`
	Los            []string                   `json:"los"`
	Wurzel         string                     `json:"wurzel"`
	BlaetterHash   string                     `json:"blaetter_hash"`
	BuendelHash    string                     `json:"buendel_hash"`
	Anfrage        QuantenWBestaetigenAnfrage `json:"anfrage"`
	Bestaetigungen []string                   `json:"bestaetigungen"`
}

func TestQuantenSAusgeberBestaetigenVektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_ausgeber_bestaetigen_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qsAusgeberBestaetigenVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	q := v.Anfrage
	nv := len(v.Los)
	if q.Art != "s" || len(q.Anteile) != 0 || len(q.Proben) != 2 || strings.Join(q.Validatoren, ",") != strings.Join(v.Los, ",") ||
		q.Wurzel != v.Wurzel || len(q.BuendelSig) != nv || q.BuendelSig[q.X-1] != "" || len(v.Bestaetigungen) != nv {
		t.Fatal("Anfrage: Form")
	}
	// Dieselbe Ausgabe wie der Vektor aus 2c: Wurzel und Blaetter.
	a := qsAusgeberAuftragVektorLesen(t)
	var blaetter []string
	for _, ab := range a.Abschnitte {
		blaetter = append(blaetter, ab.Blaetter...)
	}
	if q.K != a.K || q.N != a.N || q.T != a.T || q.Tol != a.Tol || q.Zeit != a.Auftrag.Zeit || v.Wurzel != a.Wurzel ||
		quantenWAnteileHash(blaetter) != v.BlaetterHash || q.Proben[0] != a.Proben[0] || q.Proben[1] != a.Proben[1] {
		t.Fatal("Anfrage passt nicht zur Ausgabe aus 2c")
	}
	// Nachricht und Unterschrift des Ausgebers (wie quantenWDienstAuftrag mit Buendeln).
	_, ausgeber, _ := qsAuftragSchluessel(t, v.AusgeberSchl[:2])
	m := quantenWSAuftragNachricht(q.Praeger, q.K, q.N, q.T, q.Tol, q.X, q.Wurzel, q.Validatoren, q.Proben, q.Zeit, v.BuendelHash)
	if q.Praeger != ausgeber || !kanonischeSignatur(q.PraegerSig) || verifyPersonalSign(m, q.PraegerSig, ausgeber) != nil {
		t.Fatal("Unterschrift des Ausgebers")
	}
	// Gegenprobe: anderer Hash der Buendel (vertauscht) -- nicht unterschrieben.
	if verifyPersonalSign(quantenWSAuftragNachricht(q.Praeger, q.K, q.N, q.T, q.Tol, q.X, q.Wurzel, q.Validatoren, q.Proben, q.Zeit, quantenWAnteileHash(q.BuendelSig)), q.PraegerSig, ausgeber) == nil {
		t.Fatal("andere Buendel angenommen")
	}
	// Jede Bestaetigung: von genau ihrer Gelosten, ueber genau diese Ausgabe.
	wb, _ := hex.DecodeString(v.Wurzel)
	var wurzel [32]byte
	copy(wurzel[:], wb)
	var proben [2][32]byte
	for s := 0; s < 2; s++ {
		p, ok := quantenWHex32(q.Proben[s])
		if !ok {
			t.Fatal("Probe")
		}
		proben[s] = p
	}
	for x := 1; x <= nv; x++ {
		sig := v.Bestaetigungen[x-1]
		msg := quantenWSBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, x, proben, q.Zeit, q.Tol)
		if !kanonischeSignatur(sig) || verifyPersonalSign(msg, sig, v.Los[x-1]) != nil {
			t.Fatalf("Bestaetigung %d", x)
		}
		if verifyPersonalSign(msg, sig, v.Los[x%nv]) == nil {
			t.Fatalf("Bestaetigung %d gilt fuer eine andere Geloste", x)
		}
		if x < nv && verifyPersonalSign(quantenWSBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, x+1, proben, q.Zeit, q.Tol), sig, v.Los[x-1]) == nil {
			t.Fatalf("Bestaetigung %d gilt fuer einen anderen Platz", x)
		}
	}
}
