package keeper

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Weg C, mehrere Sender (C2b-2b-4): Abschnitte der Pulse, Meldung und
// Teilmenge je Abschnitt, Ausgabe nur mit genau so vielen Validatoren.

// Gleich wie die Spezifikation: s_kette.py --vektor-s3-sender.
func TestQuantenS3Sender_Vektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s3_sender_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		K      string `json:"k"`
		NPuls  int    `json:"n_puls"`
		Lambda []int  `json:"lambda"`
		Zufall string `json:"zufall"`
		Faelle []struct {
			V          int      `json:"v"`
			NMax       int      `json:"n_max"`
			Abschnitte [][2]int `json:"abschnitte"`
			Teil       []int    `json:"teil"`
		} `json:"faelle"`
		Meld []struct {
			V      int   `json:"v"`
			Ppm    int   `json:"gamma_det_ppm"`
			Lambda []int `json:"lambda"`
			OK     bool  `json:"ok"`
		} `json:"meldungen"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	z, _ := hex.DecodeString(v.Zufall)
	if len(v.Faelle) < 6 || len(v.Meld) < 5 {
		t.Fatal("Vektor zu kurz")
	}
	for _, f := range v.Faelle {
		for x := 1; x <= f.V; x++ {
			von, bis, ok := quantenS3AbschnittPulse(v.NPuls, f.V, x)
			if !ok || [2]int{von, bis} != f.Abschnitte[x-1] {
				t.Fatalf("v %d, Abschnitt %d: %d..%d, Spezifikation %v", f.V, x, von, bis, f.Abschnitte[x-1])
			}
		}
		teil, err := quantenS3TeilmengeAbschnitte(v.K, v.Lambda, z, 5, 6, v.NPuls, f.V, f.NMax)
		if err != nil || !reflect.DeepEqual(teil, f.Teil) {
			t.Fatalf("v %d, n_max %d: Teilmenge weicht ab (%v)", f.V, f.NMax, err)
		}
		if f.V == 1 {
			alt, _ := quantenS3Teilmenge(v.K, v.Lambda, z, 5, 6, f.NMax)
			if !reflect.DeepEqual(alt, teil) {
				t.Fatal("ein Sender: nicht wie die bisherige Teilmenge")
			}
		}
	}
	for i, m := range v.Meld {
		if err := quantenS3MeldungPruefenAbschnitte(v.NPuls, m.Ppm, m.Lambda, m.V); (err == nil) != m.OK {
			t.Fatalf("Meldung %d (v %d, %d ppm): %v, Spezifikation ok=%v", i, m.V, m.Ppm, err, m.OK)
		}
	}
}

// Der Angriff: die Inhaberin meldet fast nur Pulse der Sender im Bund. Mit
// drei Sendern abgewiesen; ohne Abschnitte (ein Sender) zaehlte nur die Summe.
func TestQuantenS3Sender_SchiefeMeldung(t *testing.T) {
	const n = 3000
	alle := s3LambdaJe(3, n, 3, 300) // je Abschnitt 300
	var schief []int
	mitte := 0
	for _, p := range alle {
		if p < 1000 || p >= 2000 {
			schief = append(schief, p)
		} else if mitte < 5 {
			schief, mitte = append(schief, p), mitte+1
		}
	}
	if err := quantenS3MeldungPruefenAbschnitte(n, 150_000, schief, 3); err == nil {
		t.Fatal("Abschnitt eines ehrlichen Senders kaum gemeldet: angenommen")
	}
	if err := quantenS3MeldungPruefenAbschnitte(n, 150_000, schief, 1); err != nil {
		t.Fatalf("mit einem Sender zaehlt die Summe: %v", err)
	}
	z := make([]byte, 32)
	teil, err := quantenS3TeilmengeAbschnitte(strings.Repeat("ab", 32), schief, z, 1, 2, n, 3, quantenWNMax)
	if err != nil || len(teil) != 15 {
		t.Fatalf("Teilmenge richtet sich nach dem kleinsten Abschnitt: %d %v", len(teil), err)
	}
	for x := 1; x <= 3; x++ {
		von, bis, _ := quantenS3AbschnittPulse(n, 3, x)
		for _, p := range teil[(x-1)*5 : x*5] {
			if p < von || p >= bis {
				t.Fatalf("Stelle aus Abschnitt %d ausserhalb", x)
			}
		}
	}
	if _, err := quantenS3TeilmengeAbschnitte(strings.Repeat("ab", 32), schief[:200], z, 1, 2, n, 3, quantenWNMax); err == nil {
		t.Fatal("Abschnitt ohne gemeldeten Puls: Teilmenge gezogen")
	}
	if _, _, ok := quantenS3AbschnittPulse(2, 3, 1); ok {
		t.Fatal("mehr Sender als Pulse")
	}
}

// Ueber die Kette: Vormerken mit drei Sendern, schiefe Meldung abgewiesen,
// ehrliche Meldung, Ziehen je Abschnitt; Form und Unterschrift der Zahl.
func TestQuantenS3Sender_Kette(t *testing.T) {
	w := neueS3Welt(t)
	w.sender = 3
	K := w.p.K
	pk := w.keys[w.praeger]
	w.abgewiesen("null Sender", func() Transaction {
		w.sender = 0
		defer func() { w.sender = 3 }()
		return w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, w.z)
	}(), w.z, "quanten_w_form")
	w.abgewiesen("zu viele Sender", func() Transaction {
		w.sender = quantenWVMax + 1
		defer func() { w.sender = 3 }()
		return w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, w.z)
	}(), w.z, "quanten_w_form")
	w.abgewiesen("mehr Sender als Pulse", w.vormerkenTx(w.praeger, pk, K, w.anna, 2, s3Deckung, w.z), w.z, "quanten_w_form")
	anders := w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, w.z)
	anders.QuantenW.PraepZahl = 2 // Unterschrift galt 3 Sendern
	w.abgewiesen("Unterschrift fuer andere Zahl der Sender", anders, w.z, "quanten_w_unterschrift")
	w.annehmen(w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, w.z), w.z)
	if v := w.vs(w.cs, K); v.Sender != 3 {
		t.Fatalf("Sender in der Vormerkung: %d", v.Sender)
	}

	ehrlich := s3LambdaJe(21, s3N, 3, 7_000) // je Abschnitt 7.000 (gamma 20 % verlangt 6.667)
	var schief []int
	for _, p := range ehrlich {
		if von, bis, _ := quantenS3AbschnittPulse(s3N, 3, 2); p < von || p >= bis {
			schief = append(schief, p)
		}
	}
	for _, p := range s3LambdaJe(23, s3N, 3, 9_000) { // mehr aus Abschnitt 1: Summe ueber gamma, Abschnitt 2 leer
		if p < s3N/3 {
			schief = append(schief, p)
		}
	}
	schief = s3Eindeutig(schief)
	if len(schief)*1_000_000 < s3Gamma*s3N {
		t.Fatalf("Testaufbau: schiefe Meldung muss die Summe erfuellen (%d)", len(schief))
	}
	w.abgewiesen("schiefe Meldung (Sender 2 fehlt)", w.meldenTx(w.anna, w.keys[w.anna], K, s3Text(t, schief)), w.z+10, "quanten_s3_meldung")
	text := s3Text(t, ehrlich)
	w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], K, text), w.z+10)
	r, _ := quantenS3ZiehRunde(w.z + 10)
	w.annehmen(w.ziehenTx(K, text, r, w.sig(r)), w.z+10+quantenS3ZiehAbstand+5)
	v := w.vs(w.cs, K)
	soll, _ := quantenS3TeilmengeAbschnitte(K, ehrlich, v.Zufall[:], v.MeldungZeit, v.MeldungZeit+1, s3N, 3, quantenWNMax)
	if !reflect.DeepEqual(v.Teil, soll) || len(v.Teil) != 3*(quantenWNMax/3) {
		t.Fatalf("Teilmenge je Abschnitt: %d Stellen", len(v.Teil))
	}
	if a := quantenS3Ansicht(K, v); a["sender"] != 3 {
		t.Fatalf("Ansicht: %v", a["sender"])
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
		t.Fatal("Knoten weichen ab")
	}

	// Register streng: Zahl der Sender und Form der Teilmenge.
	gut := w.cs.quantenWJetzt().form()
	if r, err := quantenWRegisterLesen(gut.kodiert()); err != nil || r.form().kodiert() != gut.kodiert() {
		t.Fatalf("Rundlauf des Registers: %v", err)
	}
	pruefen := func(name string, f func(fs *quantenS3VormerkungForm)) {
		t.Helper()
		k := gut
		k.S3 = append([]quantenS3VormerkungForm{}, gut.S3...)
		f(&k.S3[0])
		if _, err := quantenWRegisterLesen(k.kodiert()); err == nil {
			t.Fatalf("%s: gelesen", name)
		}
	}
	pruefen("Sender 0", func(fs *quantenS3VormerkungForm) { fs.Sender = 0 })
	pruefen("Sender zu viele", func(fs *quantenS3VormerkungForm) { fs.Sender = quantenWVMax + 1 })
	pruefen("andere Zahl der Sender als die Teilmenge", func(fs *quantenS3VormerkungForm) { fs.Sender = 2 })
	pruefen("Teilmenge nicht je Abschnitt gleich", func(fs *quantenS3VormerkungForm) {
		teil, _ := quantenS3Dekodieren(fs.Teil)
		teil[0] = teil[len(teil)-1] - 1 // ein Puls aus Abschnitt 1 nach Abschnitt 3
		teil = s3Eindeutig(teil)
		fs.Teil, _ = quantenS3Kodieren(teil)
	})
	pruefen("ein Puls zu viel im letzten Abschnitt", func(fs *quantenS3VormerkungForm) {
		teil, _ := quantenS3Dekodieren(fs.Teil)
		drin := map[int]bool{}
		for _, p := range teil {
			drin[p] = true
		}
		for _, p := range ehrlich {
			if von, _, _ := quantenS3AbschnittPulse(s3N, 3, 3); p >= von && !drin[p] {
				teil = s3Eindeutig(append(teil, p))
				break
			}
		}
		fs.Teil, _ = quantenS3Kodieren(teil)
	})
	pruefen("je Abschnitt mehr als N_MAX / Sender", func(fs *quantenS3VormerkungForm) {
		je := quantenWNMax/3 + 1
		var teil []int
		for x := 1; x <= 3; x++ {
			von, bis, _ := quantenS3AbschnittPulse(s3N, 3, x)
			zahl := 0
			for _, p := range ehrlich {
				if p >= von && p < bis && zahl < je {
					teil, zahl = append(teil, p), zahl+1
				}
			}
		}
		fs.Teil, _ = quantenS3Kodieren(teil)
	})
	pruefen("ein Puls zu wenig", func(fs *quantenS3VormerkungForm) {
		teil, _ := quantenS3Dekodieren(fs.Teil)
		fs.Teil, _ = quantenS3Kodieren(teil[1:])
	})
}

// Ausgabe: genau so viele Validatoren wie Sender (Validator x = Sender x).
func TestQuantenS3Sender_AusgabeZahl(t *testing.T) {
	w := neueS3AWelt(t)
	z := w.z + quantenS3ZiehAbstand + 100
	w.abgewiesen("vier Validatoren fuer fuenf Sender", w.ausgebenS3Tx(z, w.praeger, func(q *QuantenWAngaben) { q.Validatoren = q.Validatoren[:4] }), z, "quanten_s3_ausgabe")
	w.annehmen(w.ausgebenS3Tx(z, w.praeger, nil), z)
	a := w.cs.quantenWSTokenAnsichtLocked(t.Context(), w.cs.quantenWJetzt(), w.p.K)
	if a["sender"] != s3ASend {
		t.Fatalf("Ansicht des Tokens: sender %v", a["sender"])
	}
}

func s3Eindeutig(a []int) []int {
	m := map[int]bool{}
	var out []int
	for _, p := range a {
		if !m[p] {
			m[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}
