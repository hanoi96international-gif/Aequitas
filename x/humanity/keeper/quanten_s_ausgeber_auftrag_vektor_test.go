package keeper

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// PRUEFVEKTOR: AUFTRAEGE DES AUSGEBERS UND DIE GESAMMELTEN ANTWORTEN (Weg B,
// B4b-4c-2c).
//
// Die App als Ausgeber (lib/quantenSAusgeberRunde.ts) baut je Geloster den
// Abschnitts-Auftrag (dieselbe Form wie QuantenSAbschnittAnfrage) und schickt
// ihn als Huelle; die Gelosten antworten mit ihrem eigenen Ablauf; der Ausgeber
// prueft die Antworten wie abschnitteHolen und rechnet Wurzel und die Bindungen
// der Stichproben v2 wie probenV2. testdata/quanten_s_ausgeber_auftrag_vektor.json
// entsteht in der App (lib/__tests__/quantenSAusgeberRunde.test.ts, Ende zu Ende
// mit vier Praeparatorinnen, Schluessel NUR fuer Tests). Hier prueft der Knoten
// mit SEINEM Code: der Auftrag wie der Dienst ihn liest (keine fremden Felder),
// Nachricht und Unterschrift des Ausgebers, nur verschluesselte Basen und
// Stichprobe v2, die Basen oeffnet genau die Geloste; jede Antwort (in der Form
// fuer Inhaberin und Annahmestelle) von ihrer Gelosten unterschrieben, mit dem
// Hash ihrer Basen; Wurzel, Proben und Zaehlungen wie die App sie rechnete.

type qsAusgeberAuftragVektor struct {
	Hinweis          string                   `json:"hinweis"`
	AusgeberSchl     string                   `json:"ausgeber_schluessel_nur_test"`
	PraepSchluessel  []string                 `json:"praeparatoren_schluessel_nur_test"`
	K                string                   `json:"k"`
	N                int                      `json:"n"`
	T                int                      `json:"t"`
	Tol              int                      `json:"tol"`
	Los              []string                 `json:"los"`
	BasenHashes      []string                 `json:"basen_hashes"`
	Auftrag          QuantenSAbschnittAnfrage `json:"auftrag"`
	AuftragNachricht string                   `json:"auftrag_nachricht"`
	Abschnitte       []QuantenSPaketAbschnitt `json:"abschnitte"`
	Wurzel           string                   `json:"wurzel"`
	Proben           []string                 `json:"proben"`
	Treffer          [2]int                   `json:"treffer"`
	Fehler           [2]int                   `json:"fehler"`
}

const qsAusgeberAuftragVektorPfad = "testdata/quanten_s_ausgeber_auftrag_vektor.json"

func qsAusgeberAuftragVektorLesen(t *testing.T) qsAusgeberAuftragVektor {
	t.Helper()
	roh, err := os.ReadFile(qsAusgeberAuftragVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var v qsAusgeberAuftragVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestQuantenSAusgeberAuftragVektor(t *testing.T) {
	v := qsAusgeberAuftragVektorLesen(t)
	q := v.Auftrag
	nv := len(v.Los)
	if nv != len(v.PraepSchluessel) || nv != len(v.BasenHashes) || nv != len(v.Abschnitte) || nv < quantenSLosVMin {
		t.Fatalf("Vektor: %d Geloste", nv)
	}
	// 1. Der Auftrag, wie der Dienst ihn liest: genau diese Felder.
	roh, _ := json.Marshal(q)
	var q2 QuantenSAbschnittAnfrage
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q2); err != nil {
		t.Fatal(err)
	}
	if q.K != v.K || q.N != v.N || q.T != v.T || q.Tol != v.Tol || strings.Join(q.Validatoren, ",") != strings.Join(v.Los, ",") {
		t.Fatal("Auftrag passt nicht zur Bestellung")
	}
	if q.Basen != "" || q.BasenVerschluesselt == "" || q.ProbeArt != 2 {
		t.Fatal("Weg B: nur verschluesselte Basen und Stichprobe v2")
	}
	if quantenWKNote(q.Tol, q.T, nv) == 0 {
		t.Fatal("Toleranz oder Schwelle ohne Pruefgroesse")
	}
	if got := quantenSAbschnittAuftragNachricht(q); got != v.AuftragNachricht {
		t.Fatalf("Nachricht:\n%s\nstatt\n%s", got, v.AuftragNachricht)
	}
	_, ausgeber, _ := qsAuftragSchluessel(t, v.AusgeberSchl[:2])
	if q.Praeger != ausgeber || !kanonischeSignatur(q.PraegerSig) || verifyPersonalSign(v.AuftragNachricht, q.PraegerSig, ausgeber) != nil {
		t.Fatal("Unterschrift des Ausgebers")
	}
	for j, a := range q.Validatoren {
		if _, err := quantenSSchluesselPruefen(q.Schluessel[j], a); err != nil {
			t.Fatalf("Schluessel der Gelosten %d: %v", j+1, err)
		}
	}
	// Die Basen oeffnet genau die Geloste X, mit dem Hash, den alle Antworten nennen.
	kX, aX, _ := qsAuftragSchluessel(t, v.PraepSchluessel[q.X-1][:2])
	if aX != q.Validatoren[q.X-1] {
		t.Fatal("Schluessel der Gelosten X")
	}
	von, bis, _ := quantenSAbschnitt(q.N, nv, q.X)
	if y, err := quantenSBasenOeffnen(kX, aX, q.BasenVerschluesselt, q.N, q.Validatoren, q.X, q.Inhaberin); err != nil || len(y) != bis-von {
		t.Fatalf("Basen oeffnen: %v", err)
	}
	if quantenSBasenHash(q.BasenVerschluesselt) != v.BasenHashes[q.X-1] {
		t.Fatal("Hash der Basen X")
	}
	kAnders, aAnders, _ := qsAuftragSchluessel(t, v.PraepSchluessel[q.X%nv][:2])
	if _, err := quantenSBasenOeffnen(kAnders, aAnders, q.BasenVerschluesselt, q.N, q.Validatoren, q.X, q.Inhaberin); err == nil {
		t.Fatal("eine andere Geloste oeffnet die Basen")
	}
	// 2. Jede Antwort: von ihrer Gelosten unterschrieben, mit dem Hash ihrer
	// Basen, Stichprobe v2 in moeglichen Grenzen.
	var blaetter [][32]byte
	var teile [2][][32]byte
	var treffer, fehler [2]int
	for x := 1; x <= nv; x++ {
		ab := v.Abschnitte[x-1]
		if err := quantenSPaketAbschnittPruefen(ab, v.K, v.N, nv, x, v.Los[x-1]); err != nil {
			t.Fatal(err)
		}
		if ab.BasenHash != v.BasenHashes[x-1] {
			t.Fatalf("Antwort %d: nicht mit den Basen ihres Auftrags", x)
		}
		vonX, bisX, _ := quantenSAbschnitt(v.N, nv, x)
		if err := quantenSAntwortV2Pruefen(QuantenSAbschnittAntwort{Teile: ab.Teile, Zaehlung: ab.Zaehlung}, bisX-vonX); err != nil {
			t.Fatalf("Antwort %d: %v", x, err)
		}
		for _, h := range ab.Blaetter {
			bl, ok := quantenWHex32(h)
			if !ok {
				t.Fatalf("Antwort %d: Blatt", x)
			}
			blaetter = append(blaetter, bl)
		}
		for s := 0; s < 2; s++ {
			h, _ := quantenWHex32(ab.Teile[s])
			teile[s] = append(teile[s], h)
			treffer[s] += ab.Zaehlung[s][0]
			fehler[s] += ab.Zaehlung[s][1]
		}
		// Ein anderer Abschnitt mit dieser Unterschrift faellt durch.
		if x == 1 {
			falsch := ab
			falsch.Blaetter = append([]string{ab.Blaetter[1]}, ab.Blaetter[1:]...)
			if quantenSPaketAbschnittPruefen(falsch, v.K, v.N, nv, x, v.Los[x-1]) == nil {
				t.Fatal("veraendertes Blatt angenommen")
			}
			if quantenSPaketAbschnittPruefen(ab, v.K, v.N, nv, x, v.Los[1]) == nil {
				t.Fatal("Antwort einer anderen Gelosten angenommen")
			}
		}
	}
	// 3. Wurzel, Proben und Zaehlungen wie die App.
	w := quantenWMerkleWurzel(blaetter)
	if len(blaetter) != v.N || hex.EncodeToString(w[:]) != v.Wurzel {
		t.Fatal("Wurzel")
	}
	if len(v.Proben) != 2 {
		t.Fatal("Proben")
	}
	for s := 0; s < 2; s++ {
		p, err := quantenSProbeHashV2(v.K, s, teile[s])
		if err != nil || hex.EncodeToString(p[:]) != v.Proben[s] {
			t.Fatalf("Probe %d: %v", s, err)
		}
	}
	if treffer != v.Treffer || fehler != v.Fehler {
		t.Fatalf("Zaehlungen %v/%v statt %v/%v", fehler, treffer, v.Fehler, v.Treffer)
	}
	if err := quantenSEhrlichBesteht(treffer, fehler, v.Tol); err != nil {
		t.Fatal(err)
	}
}
