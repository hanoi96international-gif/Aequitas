package keeper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/ecies"
)

// Basen y je Validator verschluesselt (quanten_s_basen_chiffrat.go,
// DEZENTRAL.md 17 R8): der Ausgeber reicht nur Chiffrate weiter, jeder
// Validator oeffnet seines, unterschreibt den Hash, die Inhaberin erkennt
// einen Tausch.

// qsbMitschnitt: merkt sich, in welchen Basen ein Validator gemessen hat.
type qsbMitschnitt struct {
	innen quantenSMessquelle
	mu    sync.Mutex
	y     []string
}

func (m *qsbMitschnitt) abschnitt(y []int) ([]int, []int, []int, string, error) {
	m.mu.Lock()
	m.y = append(m.y, qsBitsText(y))
	m.mu.Unlock()
	return m.innen.abschnitt(y)
}

// qsbNetz: zeichnet jeden Auftrag auf, wie er den Ausgeber verlaesst (JSON).
type qsbNetz struct {
	*qsv2Netz
	auftraege []string
	ohneHash  int // dieser Validator laesst den Hash des Chiffrats weg (selbst unterschrieben)
}

func (n *qsbNetz) abschnitt(ctx context.Context, x int, req QuantenSAbschnittAnfrage) (QuantenSAbschnittAntwort, error) {
	roh, _ := json.Marshal(req)
	n.auftraege = append(n.auftraege, string(roh))
	ans, err := n.qsv2Netz.abschnitt(ctx, x, req)
	if err == nil && x == n.ohneHash {
		ans.BasenHash = ""
		ans.Sig = personalSign(n.q.w.t, n.q.w.valKeys[x-1], quantenSAbschnittAntwortNachricht(ans))
	}
	return ans, err
}

// Ein Validator unterschreibt seine Antwort ohne den Hash des Chiffrats: der
// Ausgeber gibt nichts aus.
func TestQuantenSBasenChiffrat_AusgeberPrueftHash(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w, _ := neueQsbWelt(t)
	netz := &qsbNetz{qsv2Netz: w.netz, ohneHash: 2}
	if _, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, w.dat.Bestellung, w.q.w.val, 3, netz, &w.log); err == nil ||
		!strings.Contains(err.Error(), "Validator 2: Antwort passt nicht") {
		t.Fatalf("Antwort ohne Hash angenommen: %v", err)
	}
}

// neueQsbWelt: Rollen-Welt, deren Inhaberin y an die Validatoren verschluesselt.
func neueQsbWelt(t *testing.T) (*qsrWelt, []*qsbMitschnitt) {
	t.Helper()
	w := neueQsrWelt(t)
	var val []QuantenSStelle
	for j, a := range w.q.w.val {
		val = append(val, QuantenSStelle{Adresse: a, Schluessel: pubHex(w.q.w.valKeys[j])})
	}
	dat, err := quantenSInhaberinNeu(256, len(w.q.w.val), w.dat.Bestellung.Stellen, val)
	if err != nil {
		t.Fatal(err)
	}
	w.dat = dat
	var mit []*qsbMitschnitt
	for _, d := range w.q.d {
		m := &qsbMitschnitt{innen: d.messquelle}
		d.messquelle = m
		mit = append(mit, m)
	}
	return w, mit
}

func TestQuantenSBasenChiffrat_Gutfall(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	for s := 0; s < 2; s++ {
		w, mit := neueQsbWelt(t)
		v := len(w.q.w.val)
		if w.dat.Bestellung.Basen != nil || len(w.dat.Bestellung.BasenVerschluesselt) != v || len(w.dat.Basen) != v {
			t.Fatalf("Bestellung: %+v", w.dat.Bestellung)
		}
		// Die Bestellung ist oeffentlich: kein y darin.
		best := jsonWeg(t, w.dat.Bestellung)
		roh, _ := json.Marshal(best)
		for x, y := range w.dat.Basen {
			if strings.Contains(string(roh), y) {
				t.Fatalf("y von Abschnitt %d im Klartext in der Bestellung", x+1)
			}
		}
		netz := &qsbNetz{qsv2Netz: w.netz}
		p, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, best, w.q.w.val, 3, netz, &w.log)
		if err != nil {
			t.Fatalf("Ausgabe: %v\n%s", err, w.log.String())
		}
		pj := jsonWeg(t, *p)
		// Nichts, was den Ausgeber verlaesst oder zu ihm kommt, enthaelt y.
		pr, _ := json.Marshal(pj)
		alles := strings.Join(netz.auftraege, "\n") + string(pr) + w.log.String()
		for x, y := range w.dat.Basen {
			if strings.Contains(alles, y) {
				t.Fatalf("y von Abschnitt %d beim Ausgeber", x+1)
			}
			if len(mit[x].y) != 1 || mit[x].y[0] != y {
				t.Fatalf("Validator %d hat nicht in den Basen der Inhaberin gemessen", x+1)
			}
			if pj.Abschnitte[x].BasenHash != quantenSBasenHash(w.dat.Bestellung.BasenVerschluesselt[x]) {
				t.Fatalf("Abschnitt %d: Hash des Chiffrats fehlt", x+1)
			}
		}
		if len(netz.auftraege) != v || !strings.Contains(netz.auftraege[0], `"basen":""`) {
			t.Fatalf("Auftraege: %d", len(netz.auftraege))
		}
		// Annehmen (Paket) und Abholen (vom Validator) pruefen beide den Hash.
		w.annehmen(t, pj)
		ab, err := quantenSPaketVonKette(context.Background(), w.netz, pj.K, w.q.w.val)
		if err != nil {
			t.Fatal(err)
		}
		if err := quantenSInhaberinAnnehmen(context.Background(), w.netz, w.dat, w.q.w.val, ab, &w.log); err != nil {
			t.Fatalf("Abholen: %v", err)
		}
		vl := w.einloesen(t, s)
		erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(s), vl, w.q.w.val, w.netz, &w.log)
		if err != nil || erg.Urteil != "eingeloest" || erg.Vorab != "besteht" {
			t.Fatalf("Annahme: %+v, %v\n%s", erg, err, w.log.String())
		}
	}
}

// Der Ausgeber setzt eigene Basen ein -- an denselben Validator und dieselbe
// Inhaberin verschluesselt (beides kennt er) oder im Klartext. Der Validator
// praepariert, aber die Inhaberin nimmt nicht an.
func TestQuantenSBasenChiffrat_Getauscht(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	for _, fall := range []string{"eigenes Chiffrat", "Klartext"} {
		t.Run(fall, func(t *testing.T) {
			w, _ := neueQsbWelt(t)
			v := len(w.q.w.val)
			best := w.dat.Bestellung
			eigene, err := quantenSBasenZiehen(best.N, v)
			if err != nil {
				t.Fatal(err)
			}
			if fall == "Klartext" {
				best.BasenVerschluesselt, best.Basen = nil, eigene
			} else {
				best.BasenVerschluesselt = append([]string(nil), best.BasenVerschluesselt...)
				c, err := quantenSBasenVerschluesseln(pubHex(w.q.w.valKeys[1]), w.q.w.val[1], best.N, v, 2, best.Inhaberin, eigene[1])
				if err != nil {
					t.Fatal(err)
				}
				best.BasenVerschluesselt[1] = c
			}
			p, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, best, w.q.w.val, 3, w.netz, &w.log)
			if err != nil {
				t.Fatalf("Ausgabe: %v", err)
			}
			pj := jsonWeg(t, *p)
			err = quantenSInhaberinAnnehmen(context.Background(), w.netz, w.dat, w.q.w.val, pj, &w.log)
			if err == nil || !strings.Contains(err.Error(), "getauscht") {
				t.Fatalf("getauschte Basen angenommen: %v", err)
			}
			if w.dat.K != "" {
				t.Fatal("K trotz Befund in der Datei")
			}
			// Auch ohne Paket (direkt vom Validator): derselbe Befund.
			ab, err := quantenSPaketVonKette(context.Background(), w.netz, pj.K, w.q.w.val)
			if err != nil {
				t.Fatal(err)
			}
			if err := quantenSInhaberinAnnehmen(context.Background(), w.netz, w.dat, w.q.w.val, ab, &w.log); err == nil || !strings.Contains(err.Error(), "getauscht") {
				t.Fatalf("abgeholt trotz Tausch: %v", err)
			}
		})
	}
}

// Der Validator oeffnet nur, was an IHN, fuer DIESEN Abschnitt und DIESE
// Inhaberin verschluesselt ist -- und erst nach der Unterschrift des
// Ausgebers. Abgewiesen wird ohne Messung.
func TestQuantenSBasenChiffrat_Auftrag(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w := a.w
	v := len(w.val)
	chiffrat := func(x, fuerX int, inhaberin string, basen string) string {
		t.Helper()
		c, err := quantenSBasenVerschluesseln(pubHex(w.valKeys[fuerX-1]), w.val[fuerX-1], a.n, v, x, inhaberin, basen)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	auftrag := func(x int, aendern func(q *QuantenSAbschnittAnfrage)) QuantenSAbschnittAnfrage {
		q := a.auftragAbschnitt(x)
		aendern(&q)
		q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSAbschnittAuftragNachricht(q))
		return q
	}
	// Gutfall: Validator 2 misst in genau dem y aus dem Chiffrat und
	// unterschreibt dessen Hash.
	y2 := a.basenVon(2)
	gut := chiffrat(2, 2, w.anna, y2)
	mit := &qsbMitschnitt{innen: a.d[1].messquelle}
	a.d[1].messquelle = mit
	ans, err := a.d[1].abschnittPraeparieren(auftrag(2, func(q *QuantenSAbschnittAnfrage) { q.Basen, q.BasenVerschluesselt = "", gut }))
	if err != nil {
		t.Fatal(err)
	}
	if len(mit.y) != 1 || mit.y[0] != y2 || ans.BasenHash != quantenSBasenHash(gut) ||
		verifyPersonalSign(quantenSAbschnittAntwortNachricht(ans), ans.Sig, w.val[1]) != nil {
		t.Fatalf("Gutfall: gemessen %v, Hash %q", mit.y, ans.BasenHash)
	}
	// Der Hash ist unterschrieben: ohne ihn passt die Unterschrift nicht.
	ohne := ans
	ohne.BasenHash = ""
	if verifyPersonalSign(quantenSAbschnittAntwortNachricht(ohne), ans.Sig, w.val[1]) == nil {
		t.Fatal("Hash des Chiffrats nicht unterschrieben")
	}
	// Wiederholung desselben Auftrags: dieselbe Antwort, keine zweite Messung.
	nochmal, err := a.d[1].abschnittPraeparieren(auftrag(2, func(q *QuantenSAbschnittAnfrage) { q.Basen, q.BasenVerschluesselt = "", gut }))
	if err != nil || nochmal.BasenHash != ans.BasenHash || len(mit.y) != 1 {
		t.Fatalf("Wiederholung: %v, %d Messungen", err, len(mit.y))
	}
	// (Abholen nach der Ausgabe traegt den Hash ebenfalls: TestQuantenSBasenChiffrat_Gutfall.)

	y3 := a.basenVon(3)
	falle := map[string]struct {
		f    func(q *QuantenSAbschnittAnfrage)
		ohne bool // ohne neue Unterschrift
		muss string
	}{
		"an einen anderen Validator": {f: func(q *QuantenSAbschnittAnfrage) { q.Basen, q.BasenVerschluesselt = "", chiffrat(3, 4, w.anna, y3) }, muss: "nicht fuer diesen Validator"},
		"anderer Abschnitt":          {f: func(q *QuantenSAbschnittAnfrage) { q.Basen, q.BasenVerschluesselt = "", chiffrat(2, 3, w.anna, y2) }, muss: "anderen Abschnitt"},
		"andere Inhaberin": {f: func(q *QuantenSAbschnittAnfrage) {
			q.Basen, q.BasenVerschluesselt = "", chiffrat(3, 3, w.praeger, y3)
		}, muss: "andere Inhaberin"},
		"Klartext und Chiffrat": {f: func(q *QuantenSAbschnittAnfrage) { q.BasenVerschluesselt = chiffrat(3, 3, w.anna, y3) }, muss: "zugleich Klartext"},
		"Grossbuchstaben": {f: func(q *QuantenSAbschnittAnfrage) {
			q.Basen, q.BasenVerschluesselt = "", strings.ToUpper(chiffrat(3, 3, w.anna, y3))
		}, muss: "Form"},
		"ungerade Laenge": {f: func(q *QuantenSAbschnittAnfrage) { q.Basen, q.BasenVerschluesselt = "", chiffrat(3, 3, w.anna, y3)[1:] }, muss: "Form"},
		"zu gross": {f: func(q *QuantenSAbschnittAnfrage) {
			q.Basen, q.BasenVerschluesselt = "", strings.Repeat("ab", quantenSBasenChiffratMax/2+1)
		}, muss: "Form"},
		"veraendert": {f: func(q *QuantenSAbschnittAnfrage) {
			q.Basen, q.BasenVerschluesselt = "", kippeHex(chiffrat(3, 3, w.anna, y3))
		}, muss: "nicht fuer diesen Validator"},
		"Muell, nicht unterschrieben": {f: func(q *QuantenSAbschnittAnfrage) { q.Basen, q.BasenVerschluesselt = "", strings.Repeat("ab", 200) }, ohne: true, muss: "nicht vom Ausgeber unterschrieben"},
		"Chiffrat getauscht, alte Unterschrift": {f: func(q *QuantenSAbschnittAnfrage) {
			q.Basen, q.BasenVerschluesselt = "", chiffrat(3, 3, w.anna, y3)
			q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSAbschnittAuftragNachricht(*q))
			q.BasenVerschluesselt = chiffrat(3, 3, w.anna, strings.Repeat("1", len(y3)))
		}, ohne: true, muss: "nicht vom Ausgeber unterschrieben"},
	}
	for name, fall := range falle {
		t.Run(name, func(t *testing.T) {
			vorher := a.mess[2].aufruf.Load()
			var q QuantenSAbschnittAnfrage
			if fall.ohne {
				q = a.auftragAbschnitt(3)
				fall.f(&q)
			} else {
				q = auftrag(3, fall.f)
			}
			_, err := a.d[2].abschnittPraeparieren(q)
			if err == nil || !strings.Contains(err.Error(), fall.muss) {
				t.Fatalf("angenommen oder falscher Grund: %v (verlangt %q)", err, fall.muss)
			}
			if a.mess[2].aufruf.Load() != vorher {
				t.Fatal("trotz Abweisung gemessen")
			}
		})
	}
}

func kippeHex(c string) string {
	b, _ := hex.DecodeString(c)
	b[len(b)/2] ^= 1
	return hex.EncodeToString(b)
}

// Der Inhalt des Chiffrats wird streng gelesen: Domaene, Kette, Empfaenger,
// genau diese Felder, nur Bits in der Laenge des Abschnitts.
func TestQuantenSBasenChiffrat_Inhalt(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w := a.w
	v := len(w.val)
	y := a.basenVon(1)
	pub, err := quantenSSchluesselPruefen(pubHex(w.valKeys[0]), w.val[0])
	if err != nil {
		t.Fatal(err)
	}
	roh := func(m map[string]interface{}) string {
		b, _ := json.Marshal(m)
		c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), b, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(c)
	}
	basis := func() map[string]interface{} {
		return map[string]interface{}{"dom": quantenSDomBasen, "chain": eingefroreneChainID, "an": w.val[0], "n": a.n, "v": v, "x": 1, "inhaberin": w.anna, "basen": y}
	}
	if got, err := quantenSBasenOeffnen(w.valKeys[0], w.val[0], roh(basis()), a.n, v, 1, w.anna); err != nil || got != y {
		t.Fatalf("Gutfall: %v", err)
	}
	falle := map[string]func(m map[string]interface{}){
		"Domaene":          func(m map[string]interface{}) { m["dom"] = "aequitas-quanten-s-buendel-v1" },
		"Kette":            func(m map[string]interface{}) { m["chain"] = eingefroreneChainID + 1 },
		"Empfaenger":       func(m map[string]interface{}) { m["an"] = w.val[1] },
		"n":                func(m map[string]interface{}) { m["n"] = a.n + 2 },
		"v":                func(m map[string]interface{}) { m["v"] = v - 1 },
		"Feld zu viel":     func(m map[string]interface{}) { m["k"] = a.K },
		"Feld fehlt":       func(m map[string]interface{}) { delete(m, "inhaberin") },
		"Basen kurz":       func(m map[string]interface{}) { m["basen"] = y[1:] },
		"Basen keine Bits": func(m map[string]interface{}) { m["basen"] = strings.Repeat("2", len(y)) },
	}
	for name, f := range falle {
		m := basis()
		f(m)
		if _, err := quantenSBasenOeffnen(w.valKeys[0], w.val[0], roh(m), a.n, v, 1, w.anna); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	// Daten nach dem JSON.
	b, _ := json.Marshal(basis())
	c, _ := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), append(b, []byte(`{}`)...), nil, nil)
	if _, err := quantenSBasenOeffnen(w.valKeys[0], w.val[0], hex.EncodeToString(c), a.n, v, 1, w.anna); err == nil {
		t.Error("Daten nach dem Inhalt angenommen")
	}
	// Verschluesseln nur an einen Schluessel, der zur Adresse gehoert.
	if _, err := quantenSBasenVerschluesseln(pubHex(w.valKeys[1]), w.val[0], a.n, v, 1, w.anna, y); err == nil {
		t.Error("an einen fremden Schluessel verschluesselt")
	}
	if _, err := quantenSBasenVerschluesseln(pubHex(w.valKeys[0]), w.val[0], a.n, v, 1, w.anna, y+"0"); err == nil {
		t.Error("Basen falscher Laenge verschluesselt")
	}
}

// Vormerken mit Chiffrat: der Validator merkt y aus dem Chiffrat vor (in
// seinem Verzeichnis), weist fremde Chiffrate und halbe Angaben ab.
func TestQuantenSBasenChiffrat_Vormerken(t *testing.T) {
	a, mv := qsvWelt(t)
	w := a.w
	d := a.d[1]
	v := len(w.val)
	y := a.basenVon(2)
	vm := func(c, inhaberin, basen string) QuantenSVormerkung {
		q := QuantenSVormerkung{N: a.n, X: 2, Validatoren: w.val, Basen: basen, BasenVerschluesselt: c, Inhaberin: inhaberin, Zeit: a.zeit, Praeger: w.praeger}
		q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSVormerkNachricht(q))
		return q
	}
	c, err := quantenSBasenVerschluesseln(pubHex(w.valKeys[1]), w.val[1], a.n, v, 2, w.anna, y)
	if err != nil {
		t.Fatal(err)
	}
	fremd, _ := quantenSBasenVerschluesseln(pubHex(w.valKeys[2]), w.val[2], a.n, v, 2, w.anna, y)
	andere, _ := quantenSBasenVerschluesseln(pubHex(w.valKeys[1]), w.val[1], a.n, v, 2, w.praeger, y)
	for name, q := range map[string]QuantenSVormerkung{
		"an einen anderen Validator": vm(fremd, w.anna, ""),
		"andere Inhaberin":           vm(andere, w.anna, ""),
		"ohne Inhaberin":             vm(c, "", ""),
		"Inhaberin ohne Chiffrat":    vm("", w.anna, y),
		"Klartext und Chiffrat":      vm(c, w.anna, y),
	} {
		if _, err := d.vormerken(q); err == nil {
			t.Errorf("%s: vorgemerkt", name)
		}
	}
	if b, _ := qsvVormerkungen(t, mv); len(b) != 0 {
		t.Fatalf("trotz Abweisung vorgemerkt: %v", b)
	}
	// Inhaberin getauscht nach der Unterschrift: nicht vom Ausgeber.
	q := vm(c, w.anna, "")
	q.Inhaberin = w.praeger
	if _, err := d.vormerken(q); err == nil || !strings.Contains(err.Error(), "nicht vom Ausgeber") {
		t.Fatalf("Inhaberin nach der Unterschrift getauscht: %v", err)
	}
	ans, err := d.vormerken(vm(c, w.anna, ""))
	if err != nil || ans.Status != quantenSVorgemerkt {
		t.Fatalf("Gutfall: %+v, %v", ans, err)
	}
	if b, _ := qsvVormerkungen(t, mv); len(b) != 1 || b[0] != strings.TrimSuffix(quantenSMessName(y), ".json")+quantenSVormerkEndung {
		t.Fatalf("Vormerkung nicht unter den Basen aus dem Chiffrat: %v", b)
	}
}

// Bestellung: genau eines (Klartext oder Chiffrate), Datei der Inhaberin mit
// eigenen Basen; das Werkzeug, das alle Rollen spielt, nimmt keine Chiffrate.
func TestQuantenSBasenChiffrat_Bestellung(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w, _ := neueQsbWelt(t)
	v := len(w.q.w.val)
	b := w.dat.Bestellung
	if err := b.pruefen(v); err != nil {
		t.Fatal(err)
	}
	beide := b
	beide.Basen = w.dat.Basen
	if err := beide.pruefen(v); err == nil {
		t.Error("Bestellung mit Klartext und Chiffraten angenommen")
	}
	kurz := b
	kurz.BasenVerschluesselt = b.BasenVerschluesselt[1:]
	if err := kurz.pruefen(v); err == nil {
		t.Error("Bestellung mit zu wenigen Chiffraten angenommen")
	}
	kaputt := b
	kaputt.BasenVerschluesselt = append([]string{"XYZ"}, b.BasenVerschluesselt[1:]...)
	if err := kaputt.pruefen(v); err == nil {
		t.Error("kaputtes Chiffrat angenommen")
	}
	// Die Datei braucht ihre eigenen Basen und die Validatoren.
	ohne := *w.dat
	ohne.Basen = nil
	if _, err := ohne.y(); err == nil {
		t.Error("Datei ohne eigene Basen")
	}
	klar := *w.dat
	klar.Bestellung.BasenVerschluesselt, klar.Bestellung.Basen = nil, w.dat.Basen
	if _, err := klar.y(); err == nil {
		t.Error("Basen doppelt (Klartext-Bestellung und Datei)")
	}
	// Andere Validatoren angegeben als die, an die verschluesselt ist.
	p := jsonWeg(t, *mustAusgeben(t, w))
	vertauscht := append([]string{w.q.w.val[1], w.q.w.val[0]}, w.q.w.val[2:]...)
	p.Validatoren = vertauscht
	if err := quantenSInhaberinAnnehmen(context.Background(), w.netz, w.dat, vertauscht, p, &w.log); err == nil || !strings.Contains(err.Error(), "an andere Validatoren verschluesselt") {
		t.Errorf("andere Validatoren als bei der Verschluesselung: %v", err)
	}
	// Werkzeug-Weg (spielt die Inhaberin): keine Chiffrate.
	if _, err := quantenSAusgabeLaufAbschnitte(context.Background(), w.a, b.basenFuerRunde(), w.q.w.val, 3, w.netz, &w.log); err == nil || !strings.Contains(err.Error(), "nur bei getrennten Rollen") {
		t.Errorf("Werkzeug-Weg mit verschluesselten Basen: %v", err)
	}
	// Aus einer Datei kommen keine Chiffrate in QuantenSBasen.
	var bs QuantenSBasen
	if err := json.Unmarshal([]byte(`{"n":2,"basen":["0"],"wo":"","job":"","Verschluesselt":["ab"]}`), &bs); err != nil || bs.Verschluesselt != nil {
		t.Errorf("Chiffrate aus JSON: %v %v", bs.Verschluesselt, err)
	}
}

func mustAusgeben(t *testing.T, w *qsrWelt) *QuantenSPaket {
	t.Helper()
	p, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, w.dat.Bestellung, w.q.w.val, 3, w.netz, &w.log)
	if err != nil {
		t.Fatalf("Ausgabe: %v\n%s", err, w.log.String())
	}
	return p
}

// Der groesste Abschnitt (n = quantenWNMax, ein Validator) passt als Chiffrat
// in die Form und mit dem ganzen Auftrag in den Koerper des Endpunkts.
func TestQuantenSBasenChiffrat_Groesste(t *testing.T) {
	k, adr := neuerSchluessel(t)
	_, inh := neuerSchluessel(t)
	y := strings.Repeat("01", quantenWNMax/2)
	c, err := quantenSBasenVerschluesseln(pubHex(k), adr, quantenWNMax, 1, 1, inh, y)
	if err != nil {
		t.Fatal(err)
	}
	if !quantenSBasenChiffratForm(c) {
		t.Fatalf("groesstes Chiffrat (%d Zeichen) ausserhalb der Form (hoechstens %d)", len(c), quantenSBasenChiffratMax)
	}
	if got, err := quantenSBasenOeffnen(k, adr, c, quantenWNMax, 1, 1, inh); err != nil || got != y {
		t.Fatalf("groesstes Chiffrat nicht lesbar: %v", err)
	}
	q := QuantenSAbschnittAnfrage{K: strings.Repeat("ab", 32), N: quantenWNMax, T: 1, Tol: 2, X: 1, Validatoren: []string{adr}, Schluessel: []string{pubHex(k)},
		BasenVerschluesselt: c, Inhaberin: inh, InhaberinSchluessel: pubHex(k), Stellen: []string{adr, adr}, StellenSchluessel: []string{pubHex(k), pubHex(k)},
		Zeit: 1, Praeger: adr, PraegerSig: "0x" + strings.Repeat("ab", 65), ProbeArt: 2}
	if roh, _ := json.Marshal(q); len(roh) > quantenSAbschnittKoerperMax {
		t.Fatalf("Auftrag mit groesstem Chiffrat: %d Byte, Endpunkt nimmt %d", len(roh), quantenSAbschnittKoerperMax)
	}
}
