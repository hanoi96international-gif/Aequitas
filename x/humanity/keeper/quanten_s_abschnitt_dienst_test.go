package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Ausgabe in Abschnitten im Dienst (quanten_s_abschnitt_dienst.go): jeder
// Validator praepariert seinen Abschnitt aus seiner eigenen Messquelle, die
// Anteile reisen verschluesselt ueber den Ausgeber, die Annahmestellen laden
// ihre Stichproben selbst -- und das Einloesen laeuft ueber die
// zusammengesetzten Anteile wie immer.

// qsaMessung: eine Messquelle im Speicher -- praepariert zufaellig und misst in
// den Basen y (ohne Fehler), zaehlt die Aufrufe.
type qsaMessung struct {
	r      *rand.Rand
	aufruf atomic.Int64
	fehler bool
}

func (m *qsaMessung) abschnitt(y []int) ([]int, []int, []int, string, error) {
	m.aufruf.Add(1)
	if m.fehler {
		return nil, nil, nil, "", quantenWFehler("keine Messung zu diesen Basen")
	}
	u, t, x := make([]int, len(y)), make([]int, len(y)), make([]int, len(y))
	for i := range y {
		u[i], t[i] = m.r.Intn(2), m.r.Intn(2)
		if y[i] == u[i] {
			x[i] = t[i]
		} else {
			x[i] = m.r.Intn(2)
		}
	}
	return u, t, x, "speicher", nil
}

type qsaWelt struct {
	*qsDienste
	mess   []*qsaMessung
	y      []int
	antw   []QuantenSAbschnittAntwort
	K      string
	n      int
	zeit   int64
	wurzel [32]byte
	blatt  [][32]byte
	x      []int
}

func pubHex(k *ecdsa.PrivateKey) string { return hex.EncodeToString(crypto.FromECDSAPub(&k.PublicKey)) }

func neueQsaWelt(t *testing.T, n int) *qsaWelt {
	t.Helper()
	q := neueQsDienste(t, n, 8) // Stichproben von vorbereiten werden unten ersetzt
	a := &qsaWelt{qsDienste: q, K: q.w.p.K, n: n, zeit: q.zeit}
	r := rand.New(rand.NewSource(int64(n)))
	for x := range q.d {
		m := &qsaMessung{r: rand.New(rand.NewSource(int64(1000 + x)))}
		q.d[x].messquelle = m
		a.mess = append(a.mess, m)
	}
	for i := 0; i < n; i++ {
		a.y = append(a.y, r.Intn(2))
	}
	return a
}

// auftragAbschnitt: der Auftrag an Validator x (1-basiert), vom Ausgeber unterschrieben.
func (a *qsaWelt) auftragAbschnitt(x int) QuantenSAbschnittAnfrage {
	w := a.w
	v := len(w.val)
	von, bis, _ := quantenSAbschnitt(a.n, v, x)
	q := QuantenSAbschnittAnfrage{K: a.K, N: a.n, T: 3, Tol: qsTol, X: x, Validatoren: w.val, Basen: qsBitsText(a.y[von:bis]),
		Inhaberin: w.anna, InhaberinSchluessel: pubHex(w.keys[w.anna]), Stellen: []string{a.s.stelle[0], a.s.stelle[1]},
		StellenSchluessel: []string{pubHex(w.keys[a.s.stelle[0]]), pubHex(w.keys[a.s.stelle[1]])}, Zeit: a.zeit, Praeger: w.praeger}
	for j := range w.val {
		q.Schluessel = append(q.Schluessel, pubHex(w.valKeys[j]))
	}
	q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSAbschnittAuftragNachricht(q))
	return q
}

// praeparieren: alle Validatoren; die Inhaberin liest ihr x, der Ausgeber baut die Wurzel.
func (a *qsaWelt) praeparieren(t *testing.T) {
	t.Helper()
	w := a.w
	a.antw, a.x, a.blatt = nil, make([]int, a.n), make([][32]byte, a.n)
	for x := 1; x <= len(w.val); x++ {
		ans, err := a.d[x-1].abschnittPraeparieren(a.auftragAbschnitt(x))
		if err != nil {
			t.Fatalf("Validator %d praepariert nicht: %v", x, err)
		}
		if verifyPersonalSign(quantenSAbschnittAntwortNachricht(ans), ans.Sig, w.val[x-1]) != nil {
			t.Fatalf("Validator %d: Antwort nicht von ihm", x)
		}
		xs, err := quantenSFuerInhaberinOeffnen(w.keys[w.anna], ans.FuerInhaberin, a.K, ans.Von, ans.Bis)
		if err != nil {
			t.Fatal(err)
		}
		copy(a.x[ans.Von:], xs)
		for i, h := range ans.Blaetter {
			a.blatt[ans.Von+i], _ = quantenWHex32(h)
		}
		a.antw = append(a.antw, ans)
	}
	a.wurzel = quantenWMerkleWurzel(a.blatt)
}

// stichprobe: Annahmestelle b laedt sich ihre Stichprobe bei allen Validatoren.
func (a *qsaWelt) stichprobe(t *testing.T, b int) []int {
	t.Helper()
	w := a.w
	alle := map[int]*big.Int{}
	for x := 1; x <= len(w.val); x++ {
		c, err := a.d[x-1].oeffnungFuerStelle(a.K, b)
		if err != nil {
			t.Fatal(err)
		}
		von, bis, _ := quantenSAbschnitt(a.n, len(w.val), x)
		m, err := quantenSOeffnungEntschluesseln(w.keys[a.s.stelle[b]], c, a.K, b, x, von, bis)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range m {
			alle[i] = v
		}
	}
	if _, err := quantenSOeffnungLaden(b, a.K, a.n, a.wurzel, a.blatt, alle, 64); err != nil {
		t.Fatalf("Stelle %d laedt nicht: %v", b, err)
	}
	var probe []int
	for i := range alle {
		probe = append(probe, i)
	}
	sort.Ints(probe)
	return probe
}

func (a *qsaWelt) bestaetigenAnfrage(x int) QuantenWBestaetigenAnfrage {
	w := a.w
	r := QuantenWBestaetigenAnfrage{K: a.K, N: a.n, T: 3, Tol: qsTol, X: x, Wurzel: hex.EncodeToString(a.wurzel[:]), Validatoren: w.val,
		Zeit: a.zeit, Art: "s", Proben: a.s.probenHex(), Praeger: w.praeger}
	for _, b := range a.blatt {
		r.Blaetter = append(r.Blaetter, hex.EncodeToString(b[:]))
	}
	for p := range w.val {
		r.Buendel = append(r.Buendel, a.antw[p].Buendel[x-1])
		r.BuendelSig = append(r.BuendelSig, a.antw[p].BuendelSig[x-1])
	}
	r.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenWDienstAuftrag(r))
	return r
}

// ausgebenAbschnitte: der ganze Weg bis zum ausgegebenen, gebundenen und gewaehlten Token.
func (a *qsaWelt) ausgebenAbschnitte(t *testing.T) {
	t.Helper()
	w, s := a.w, a.s
	a.praeparieren(t)
	s.probe[0], s.probe[1] = a.stichprobe(t, 0), a.stichprobe(t, 1)
	for _, i := range s.probe[0] {
		if j := sort.SearchInts(s.probe[1], i); j < len(s.probe[1]) && s.probe[1][j] == i {
			t.Fatalf("Stichproben nicht disjunkt (Stelle %d)", i)
		}
	}
	w.p.wurzel, w.p.blaetter = a.wurzel, a.blatt
	s.messbasen, s.x = a.y, a.x
	tx := s.ausgebenTx(a.zeit, 10_000_000)
	for x := 1; x <= len(w.val); x++ {
		anker, sig, err := a.d[x-1].bestaetigen(a.bestaetigenAnfrage(x))
		if err != nil || anker != "" {
			t.Fatalf("Validator %d bestaetigt nicht: %v", x, err)
		}
		tx.QuantenW.Bestaetigungen[x-1] = sig
	}
	w.annehmen(tx, a.zeit+1)
	w.annehmen(s.bindenTx(w.anna), a.zeit+2)
	w.annehmen(s.waehlenTx(w.anna, "1"), a.zeit+3)
}

func TestQuantenSAbschnittDienst_Gutfall(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w, s := a.w, a.s
	a.ausgebenAbschnitte(t)
	for x, m := range a.mess {
		if m.aufruf.Load() != 1 {
			t.Fatalf("Messquelle %d: %d Aufrufe", x+1, m.aufruf.Load())
		}
	}
	// Jeder Tresor haelt jetzt Anteile ALLER Stellen und ist nicht mehr wartend.
	for x, d := range a.d {
		e, _ := d.eintragVon(a.K)
		if e == nil || e.Wartend || len(e.Anteile) != a.n {
			t.Fatalf("Validator %d: Tresor unvollstaendig", x+1)
		}
	}
	// Einloesen an Stelle 0 (Messbasen = gepruefte Basen) ueber den Dienst.
	a.qsDienste.zeit = a.zeit + 10
	w.annehmen(s.anmeldenTx(s.stelle[0], 0, s.probe[0], s.vorlage(0, w.keys[w.anna])), a.qsDienste.zeit)
	a.qsDienste.zeit += 5
	a.takt()
	a.takt()
	if s.token(w.cs) != nil || w.stand(w.cs, s.stelle[0]) != 10_000_000 {
		t.Fatalf("Einloesen nicht durch: Stelle %d", w.stand(w.cs, s.stelle[0]))
	}
}

func TestQuantenSAbschnittDienst_Wiederholung(t *testing.T) {
	a := neueQsaWelt(t, 140)
	a.praeparieren(t)
	vorher := a.antw[1]
	noch, err := a.d[1].abschnittPraeparieren(a.auftragAbschnitt(2))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(noch.Blaetter, ",") != strings.Join(vorher.Blaetter, ",") || a.mess[1].aufruf.Load() != 1 {
		t.Fatal("derselbe Auftrag: andere Blaetter oder zweite Messung")
	}
	// Ein ANDERER Auftrag fuer dasselbe K: abgewiesen.
	anders := a.auftragAbschnitt(2)
	anders.Tol = 5
	anders.PraegerSig = personalSign(a.w.t, a.w.keys[a.w.praeger], quantenSAbschnittAuftragNachricht(anders))
	if _, err := a.d[1].abschnittPraeparieren(anders); err == nil {
		t.Fatal("anderer Auftrag fuer dasselbe K angenommen")
	}
}

func TestQuantenSAbschnittDienst_AuftragMissbrauch(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w := a.w
	k2, adr2 := neuerSchluessel(t)
	unterschreiben := func(q QuantenSAbschnittAnfrage) QuantenSAbschnittAnfrage {
		q.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenSAbschnittAuftragNachricht(q))
		return q
	}
	faelle := map[string]func(q *QuantenSAbschnittAnfrage){
		"fremde Unterschrift": func(q *QuantenSAbschnittAnfrage) {
			q.PraegerSig = personalSign(w.t, k2, quantenSAbschnittAuftragNachricht(*q))
		},
		"Unterschrift alt":     func(q *QuantenSAbschnittAnfrage) { q.Basen = strings.Repeat("0", len(q.Basen)) }, // ohne neu zu unterschreiben
		"Schluessel fremd":     func(q *QuantenSAbschnittAnfrage) { q.Schluessel[2] = pubHex(k2); *q = unterschreiben(*q) },
		"falsche Nummer":       func(q *QuantenSAbschnittAnfrage) { q.X = 3; *q = unterschreiben(*q) },
		"Basen zu kurz":        func(q *QuantenSAbschnittAnfrage) { q.Basen = q.Basen[1:]; *q = unterschreiben(*q) },
		"Basen keine Bits":     func(q *QuantenSAbschnittAnfrage) { q.Basen = "2" + q.Basen[1:]; *q = unterschreiben(*q) },
		"Stellen gleich":       func(q *QuantenSAbschnittAnfrage) { q.Stellen[1] = q.Stellen[0]; *q = unterschreiben(*q) },
		"Stelle = Inhaberin":   func(q *QuantenSAbschnittAnfrage) { q.Stellen[0] = q.Inhaberin; *q = unterschreiben(*q) },
		"Stellen-Schluessel":   func(q *QuantenSAbschnittAnfrage) { q.StellenSchluessel[1] = pubHex(k2); *q = unterschreiben(*q) },
		"Inhaberin-Schluessel": func(q *QuantenSAbschnittAnfrage) { q.InhaberinSchluessel = pubHex(k2); *q = unterschreiben(*q) },
		"zu wenige Stellen":    func(q *QuantenSAbschnittAnfrage) { q.N = 100; q.Basen = q.Basen[:20]; *q = unterschreiben(*q) },
		"Zeit alt":             func(q *QuantenSAbschnittAnfrage) { q.Zeit -= quantenWAlt + 10; *q = unterschreiben(*q) },
		"Validator doppelt": func(q *QuantenSAbschnittAnfrage) {
			q.Validatoren = append([]string(nil), q.Validatoren...)
			q.Validatoren[4] = q.Validatoren[0]
			*q = unterschreiben(*q)
		},
		"Ausgeber Fremder": func(q *QuantenSAbschnittAnfrage) {
			q.Praeger = adr2
			q.PraegerSig = personalSign(w.t, k2, quantenSAbschnittAuftragNachricht(*q))
		},
	}
	for name, f := range faelle {
		q := a.auftragAbschnitt(2)
		q.Schluessel = append([]string(nil), q.Schluessel...)
		q.Stellen = append([]string(nil), q.Stellen...)
		q.StellenSchluessel = append([]string(nil), q.StellenSchluessel...)
		f(&q)
		if _, err := a.d[1].abschnittPraeparieren(q); err == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
	if a.mess[1].aufruf.Load() != 0 {
		t.Fatal("ein abgewiesener Auftrag hat die Messquelle benutzt")
	}
	// Ohne Messquelle oder ohne passende Messung: nichts abgelegt.
	a.d[0].messquelle = nil
	if _, err := a.d[0].abschnittPraeparieren(a.auftragAbschnitt(1)); err == nil {
		t.Fatal("ohne Messquelle angenommen")
	}
	a.mess[2].fehler = true
	if _, err := a.d[2].abschnittPraeparieren(a.auftragAbschnitt(3)); err == nil {
		t.Fatal("ohne Messung angenommen")
	}
	if e, _ := a.d[2].eintragVon(a.K); e != nil {
		t.Fatal("Eintrag ohne Messung abgelegt")
	}
	// Ein Ausgeber ohne Deckung.
	alt := quantenWPraegerTauglich
	quantenWPraegerTauglich = func(*ChainState, string) bool { return false }
	defer func() { quantenWPraegerTauglich = alt }()
	if _, err := a.d[3].abschnittPraeparieren(a.auftragAbschnitt(4)); err == nil {
		t.Fatal("Ausgeber ohne Deckung angenommen")
	}
}

func TestQuantenSAbschnittDienst_OeffnungMissbrauch(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w := a.w
	a.praeparieren(t)
	if _, err := a.d[0].oeffnungFuerStelle(strings.Repeat("12", 32), 0); err == nil {
		t.Fatal("Oeffnung zu unbekanntem K")
	}
	if _, err := a.d[0].oeffnungFuerStelle(a.K, 2); err == nil {
		t.Fatal("Oeffnung fuer Stelle 2")
	}
	c, err := a.d[0].oeffnungFuerStelle(a.K, 0)
	if err != nil {
		t.Fatal(err)
	}
	von, bis, _ := quantenSAbschnitt(a.n, len(w.val), 1)
	// Nur Stelle 0 kann sie lesen -- nicht Stelle 1, nicht die Inhaberin.
	for _, k := range []*ecdsa.PrivateKey{w.keys[a.s.stelle[1]], w.keys[w.anna]} {
		if _, err := quantenSOeffnungEntschluesseln(k, c, a.K, 0, 1, von, bis); err == nil {
			t.Fatal("Oeffnung von einem Fremden gelesen")
		}
	}
	// Fuer einen anderen Abschnitt gelesen: abgewiesen.
	if _, err := quantenSOeffnungEntschluesseln(w.keys[a.s.stelle[0]], c, a.K, 0, 2, von, bis); err == nil {
		t.Fatal("Oeffnung einem anderen Validator zugeschrieben")
	}
	// Die zwei Stichproben sind disjunkt und je zur Haelfte.
	p0, p1 := a.stichprobe(t, 0), a.stichprobe(t, 1)
	if len(p0)+len(p1) != a.n || len(p0) != a.n/2 {
		t.Fatalf("Stichproben %d + %d bei n = %d", len(p0), len(p1), a.n)
	}
}

func TestQuantenSAbschnittDienst_BestaetigenMissbrauch(t *testing.T) {
	a := neueQsaWelt(t, 140)
	w, s := a.w, a.s
	a.praeparieren(t)
	s.probe[0], s.probe[1] = a.stichprobe(t, 0), a.stichprobe(t, 1)
	k2, _ := neuerSchluessel(t)
	neu := func(r QuantenWBestaetigenAnfrage) QuantenWBestaetigenAnfrage {
		r.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenWDienstAuftrag(r))
		return r
	}
	faelle := map[string]func(r *QuantenWBestaetigenAnfrage){
		"Buendel vertauscht": func(r *QuantenWBestaetigenAnfrage) {
			r.Buendel[0], r.BuendelSig[0] = a.antw[0].Buendel[3], a.antw[0].BuendelSig[3] // fuer Validator 4 bestimmt
			*r = neu(*r)
		},
		"Buendel fremd unterschrieben": func(r *QuantenWBestaetigenAnfrage) {
			r.BuendelSig[0] = personalSign(w.t, k2, quantenSBuendelNachricht(a.K, 1, 2, r.Buendel[0]))
			*r = neu(*r)
		},
		"Buendel an sich selbst": func(r *QuantenWBestaetigenAnfrage) { r.Buendel[1] = r.Buendel[0]; *r = neu(*r) },
		"Buendel fehlt": func(r *QuantenWBestaetigenAnfrage) {
			r.Buendel, r.BuendelSig = r.Buendel[:4], r.BuendelSig[:4]
			*r = neu(*r)
		},
		"nicht beauftragt": func(r *QuantenWBestaetigenAnfrage) { r.PraegerSig = personalSign(w.t, k2, quantenWDienstAuftrag(*r)) },
		"Auftrag ueber andere Buendel": func(r *QuantenWBestaetigenAnfrage) {
			*r = neu(*r)
			r.Buendel[2], r.BuendelSig[2] = a.antw[2].Buendel[0], a.antw[2].BuendelSig[0]
		},
		"Wurzel falsch": func(r *QuantenWBestaetigenAnfrage) { r.Wurzel = strings.Repeat("0", 64); *r = neu(*r) },
		"eigenes Blatt anders": func(r *QuantenWBestaetigenAnfrage) {
			bl := append([][32]byte(nil), a.blatt...)
			von, _, _ := quantenSAbschnitt(a.n, 5, 2)
			bl[von][0] ^= 1
			r.Blaetter = nil
			for _, b := range bl {
				r.Blaetter = append(r.Blaetter, hex.EncodeToString(b[:]))
			}
			ww := quantenWMerkleWurzel(bl)
			r.Wurzel = hex.EncodeToString(ww[:])
			*r = neu(*r)
		},
		"Anteile dazu": func(r *QuantenWBestaetigenAnfrage) { r.Anteile = []string{"1"}; *r = neu(*r) },
		"anderes Tol":  func(r *QuantenWBestaetigenAnfrage) { r.Tol = 5; *r = neu(*r) },
	}
	for name, f := range faelle {
		r := a.bestaetigenAnfrage(2)
		r.Buendel = append([]string(nil), r.Buendel...)
		r.BuendelSig = append([]string(nil), r.BuendelSig...)
		f(&r)
		if _, _, err := a.d[1].bestaetigen(r); err == nil {
			t.Fatalf("%s angenommen", name)
		}
		if e, _ := a.d[1].eintragVon(a.K); e == nil || !e.Wartend {
			t.Fatalf("%s: wartender Eintrag veraendert", name)
		}
	}
	// Eine Bestaetigung ohne Buendel (Ausgabe mit einem Auftrag) fuer ein
	// wartendes K: abgewiesen.
	if _, _, err := a.d[1].bestaetigen(a.anfrageS(2)); err == nil {
		t.Fatal("Bestaetigung mit Anteilen fuer ein wartendes K angenommen")
	}
	// Gutfall, dann dieselbe Anfrage noch einmal: dieselbe Bestaetigung.
	_, s1, err := a.d[1].bestaetigen(a.bestaetigenAnfrage(2))
	if err != nil {
		t.Fatal(err)
	}
	_, s2, err := a.d[1].bestaetigen(a.bestaetigenAnfrage(2))
	if err != nil || s1 != s2 {
		t.Fatalf("Wiederholung: %v", err)
	}
}

// Grenze je Ausgeber: mehr offene Auftraege als quantenWOffeneJePraeger werden
// abgewiesen, ohne die Messquelle zu benutzen.
func TestQuantenSAbschnittDienst_GrenzeJeAusgeber(t *testing.T) {
	a := neueQsaWelt(t, 140)
	d := a.d[0]
	for j := 0; j < quantenWOffeneJePraeger; j++ {
		a.K = strings.Repeat(string("abcdef0123"[j]), 64)
		if _, err := d.abschnittPraeparieren(a.auftragAbschnitt(1)); err != nil {
			t.Fatalf("Auftrag %d: %v", j+1, err)
		}
	}
	vorher := a.mess[0].aufruf.Load()
	a.K = strings.Repeat("9", 64)
	if _, err := d.abschnittPraeparieren(a.auftragAbschnitt(1)); err == nil || !strings.Contains(err.Error(), "offene") {
		t.Fatalf("Auftrag ueber der Grenze: %v", err)
	}
	if a.mess[0].aufruf.Load() != vorher {
		t.Fatal("Auftrag ueber der Grenze hat gemessen")
	}
}

func TestQuantenSMessDatei(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mess.json")
	inhalt := quantenSMessDateiInhalt{Abschnitte: []quantenSMessDatensatz{
		{U: "0101", T: "0011", Y: "0110", X: "0010", Wo: "fake", Job: "j1"},
		{U: "1111", T: "0000", Y: "0000", X: "1010", Wo: "fake", Job: "j2"},
	}}
	roh, _ := json.Marshal(inhalt)
	if err := os.WriteFile(p, roh, 0o600); err != nil {
		t.Fatal(err)
	}
	m := &quantenSMessDatei{pfad: p}
	u, tt, x, _, err := m.abschnitt([]int{0, 1, 1, 0})
	if err != nil || qsBitsText(u) != "0101" || qsBitsText(tt) != "0011" || qsBitsText(x) != "0010" {
		t.Fatalf("Gutfall: %v", err)
	}
	// Genau einmal: derselbe Abschnitt ist weg, der andere bleibt.
	if _, _, _, _, err := m.abschnitt([]int{0, 1, 1, 0}); err == nil {
		t.Fatal("Abschnitt zweimal verwendet")
	}
	if _, _, _, _, err := m.abschnitt([]int{0, 0, 0, 0}); err != nil {
		t.Fatalf("zweiter Abschnitt: %v", err)
	}
	for name, s := range map[string]string{
		"unbekanntes Feld": `{"abschnitte":[{"u":"01","t":"01","y":"01","x":"01","mehr":1}]}`,
		"kaputte Bits":     `{"abschnitte":[{"u":"0a","t":"01","y":"01","x":"01"}]}`,
		"kein JSON":        `{`,
	} {
		os.WriteFile(p, []byte(s), 0o600)
		if _, _, _, _, err := m.abschnitt([]int{0, 1}); err == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
	os.Remove(p)
	if _, _, _, _, err := m.abschnitt([]int{0, 1}); err == nil {
		t.Fatal("fehlende Datei angenommen")
	}
}
