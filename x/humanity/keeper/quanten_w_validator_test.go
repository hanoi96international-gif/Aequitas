package keeper

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Validator-Dienst (W2d): Tresor, Bestaetigung, Anteile erst nach der
// Antwort, Hintergrund (Zufall, Offenlegen in Teilen ueber Nachbarn, Fristen).

type qwDienste struct {
	w      *qwWelt
	d      []*quantenWDienst
	srv    []*httptest.Server
	zeit   int64
	mu     sync.Mutex
	falsch map[int]bool // Validator (1-basiert) liefert falsche Anteile
	// DAG-Hoehe und finalisierte Hoehe, wie die Dienste sie sehen. Beide 0:
	// alles ist sofort final (die Tests ohne Finalitaet); der Test der
	// Finalitaet stellt sie.
	hoehe, final atomic.Int64
}

// einreichen: wie die API -- Bescheid, anwenden, der zweite Knoten spielt nach.
func (q *qwDienste) einreichen(art string, a QuantenWAngaben) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	tx := Transaction{Type: art, QuantenW: &a}
	u := q.w.bescheid(q.w.cs, tx, q.zeit)
	if u.regel != "" {
		return fmt.Errorf("%s: %s", u.regel, u.text)
	}
	g := *tx.QuantenW
	g.Anteile, g.Pfand, g.PfandAn, g.Wert, g.WertAn, g.Urteil = u.felder.Anteile, u.felder.Pfand, u.felder.PfandAn, u.felder.Wert, u.felder.WertAn, u.felder.Urteil
	tx.QuantenW, tx.BuchAt = &g, q.zeit
	q.w.cs.mu.Lock()
	err := q.w.cs.quantenWAnwendenLocked(context.Background(), u)
	q.w.cs.mu.Unlock()
	if err != nil {
		return err
	}
	return q.w.nachspielen(tx, q.zeit+7)
}

func neueQwDienste(t *testing.T, n int) *qwDienste {
	t.Helper()
	q := &qwDienste{w: neueQwWelt(t, n), zeit: qwT0, falsch: map[int]bool{}}
	for x := range q.w.val {
		d, err := neuerQuantenWDienst(q.w.cs, q.w.valKeys[x], &quantenWSpeicherAblage{}, nil, q.einreichen)
		if err != nil {
			t.Fatal(err)
		}
		d.jetzt = func() int64 { return q.zeit }
		d.hoehe, d.final = q.hoehe.Load, q.final.Load
		q.d = append(q.d, d)
		xx := x + 1
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			index, _ := strconv.Atoi(r.URL.Query().Get("index"))
			ab, _ := strconv.Atoi(r.URL.Query().Get("ab"))
			out, err := d.anteile(r.URL.Query().Get("k"), index, ab)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusConflict)
				return
			}
			if q.falsch[xx] && len(out.Y) > 0 {
				out.Y[0] = "1" // falscher Anteil (vom Validator selbst unterschrieben)
			}
			if err := d.anteilUnterschreiben(&out); err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(rw).Encode(out)
		}))
		t.Cleanup(srv.Close)
		q.srv = append(q.srv, srv)
	}
	for x, d := range q.d {
		for y, s := range q.srv {
			if y != x {
				d.peers = append(d.peers, s.URL)
			}
		}
	}
	return q
}

func (q *qwDienste) anfrageBestaetigen(x int) QuantenWBestaetigenAnfrage {
	p := q.w.p
	r := QuantenWBestaetigenAnfrage{K: p.K, N: p.n, T: p.t, Tol: quantenWTolStandard, X: x, Wurzel: hex.EncodeToString(p.wurzel[:]),
		Validatoren: q.w.val, Zeit: q.zeit}
	for i := 0; i < p.n; i++ {
		r.Anteile = append(r.Anteile, p.anteile[i][x-1].Text(16))
		r.Blaetter = append(r.Blaetter, hex.EncodeToString(p.blaetter[i][:]))
	}
	return q.auftrag(r, q.w.praeger)
}

// auftrag: der Praeger (wer) unterschreibt den Auftrag.
func (q *qwDienste) auftrag(r QuantenWBestaetigenAnfrage, wer string) QuantenWBestaetigenAnfrage {
	r.Praeger = wer
	r.PraegerSig = personalSign(q.w.t, q.w.keys[wer], quantenWAuftragNachricht(wer, r.K, r.N, r.T, r.Tol, r.X, r.Wurzel, r.Validatoren, r.Zeit, quantenWAnteileHash(r.Anteile)))
	return r
}

// praegen: alle Validatoren bestaetigen ueber den Dienst, dann die Kette.
func (q *qwDienste) praegen(t *testing.T) {
	t.Helper()
	p, w := q.w.p, q.w
	wurzel := hex.EncodeToString(p.wurzel[:])
	a := &QuantenWAngaben{K: p.K, N: p.n, T: p.t, Wurzel: wurzel, Validatoren: w.val, Deckung: 10_000_000, Tol: quantenWTolStandard}
	for x := range w.val {
		anker, sig, err := q.d[x].bestaetigen(q.anfrageBestaetigen(x + 1))
		if err != nil {
			t.Fatalf("Validator %d bestaetigt nicht: %v", x+1, err)
		}
		a.Anker, a.Bestaetigungen = append(a.Anker, anker), append(a.Bestaetigungen, sig)
	}
	a.WalletSig = personalSign(t, w.keys[w.praeger], quantenWPraegenNachricht(w.praeger, p.K, p.n, p.t, wurzel, w.val, a.Anker, a.Deckung, q.zeit, a.Tol))
	w.annehmen(Transaction{Type: quantenWPraegenTxArt, Wallet: w.praeger, DistributionAt: q.zeit, QuantenW: a}, q.zeit+1)
}

func (q *qwDienste) takt() {
	for _, d := range q.d {
		d.takt(context.Background())
	}
}

// Gutfall: Praegen ueber den Dienst, Pruefung und Einloesen -- Zufall und
// Offenlegung (in Teilen, ueber die Nachbarn) macht der Hintergrund.
func TestQuantenWDienst_Gutfall(t *testing.T) {
	q := neueQwDienste(t, 200)
	w := q.w
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt() // jeder Validator reicht seinen Zufall ein
	a := w.anfrage(w.cs)
	if a == nil || a.Basen == nil {
		t.Fatal("Zufall nicht vollstaendig")
	}
	// Vor der Antwort gibt kein Validator Anteile heraus.
	for _, d := range q.d {
		if _, err := d.anteile(w.p.K, 0, 0); err == nil {
			t.Fatal("Anteile vor der Antwort herausgegeben")
		}
	}
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	q.zeit += 2
	q.takt() // der zustaendige Validator sammelt und legt offen
	if w.anfrage(w.cs) != nil {
		t.Fatalf("Pruefung nicht abgeschlossen: %+v", w.anfrage(w.cs))
	}
	if w.stand(w.cs, w.anna) != 100_000_000 {
		t.Fatal("Pfand nicht zurueck")
	}
	// Einloesen: k_note = 64 -> ein Teil; falsche Nachbarn werden umgangen.
	q.falsch[2] = true
	q.zeit += 50
	w.annehmen(w.anfrageTx(w.bert, 1, qwLK, w.bert, q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	a = w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.bert, 1, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	q.zeit += 2
	q.takt()
	if _, lebt := w.cs.quantenWJetzt().Noten[w.p.K]; lebt || w.stand(w.cs, w.bert) != 110_000_000 {
		t.Fatalf("Einloesen nicht durch: bert %d", w.stand(w.cs, w.bert))
	}
	// Die tote Note verschwindet aus dem Tresor.
	q.takt()
	for _, d := range q.d {
		if len(d.eintraege) != 0 {
			t.Fatal("tote Note im Tresor")
		}
	}
}

// Offenlegen in Teilen ueber den Hintergrund (k = 150 > 64: drei Teile).
func TestQuantenWDienst_Teile(t *testing.T) {
	q := neueQwDienste(t, 400)
	w := q.w
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 150, "", q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	// Drei Teile in EINEM Takt: der zustaendige Validator reicht sie ohne
	// Pause nacheinander ein (hoechstens quantenWTeileJeTakt).
	q.zeit += 2
	q.takt()
	if w.anfrage(w.cs) != nil || w.stand(w.cs, w.anna) != 100_000_000 {
		t.Fatalf("drei Teile nicht durch: %+v", w.anfrage(w.cs))
	}
}

// Missbrauch der Bestaetigung: alles, was nicht passt, wird abgewiesen und
// nichts abgelegt.
func TestQuantenWDienst_BestaetigenMissbrauch(t *testing.T) {
	q := neueQwDienste(t, 200)
	d := q.d[0]
	gut := q.anfrageBestaetigen(1)
	for name, aendern := range map[string]func(r *QuantenWBestaetigenAnfrage){
		"fremde Nummer":   func(r *QuantenWBestaetigenAnfrage) { r.X = 2 },
		"Nummer 0":        func(r *QuantenWBestaetigenAnfrage) { r.X = 0 },
		"Blatt falsch":    func(r *QuantenWBestaetigenAnfrage) { r.Blaetter[5] = strings.Repeat("0", 64) },
		"Wurzel fremd":    func(r *QuantenWBestaetigenAnfrage) { r.Wurzel = strings.Repeat("1", 64) },
		"Anteil fehlt":    func(r *QuantenWBestaetigenAnfrage) { r.Anteile = r.Anteile[1:] },
		"Anteil zu gross": func(r *QuantenWBestaetigenAnfrage) { r.Anteile[0] = quantenWP.Text(16) },
		"Anteil gross":    func(r *QuantenWBestaetigenAnfrage) { r.Anteile[0] = strings.ToUpper(r.Anteile[0]) + "A" },
		"Zeit alt":        func(r *QuantenWBestaetigenAnfrage) { r.Zeit = q.zeit - quantenWAlt - 1 },
		"Zeit Zukunft":    func(r *QuantenWBestaetigenAnfrage) { r.Zeit = q.zeit + quantenWVoraus + 1 },
		"Toleranz 15":     func(r *QuantenWBestaetigenAnfrage) { r.Tol = 15 },
		"K kaputt":        func(r *QuantenWBestaetigenAnfrage) { r.K = "xyz" },
		"Validator doppelt": func(r *QuantenWBestaetigenAnfrage) {
			v := append([]string(nil), r.Validatoren...)
			v[1] = v[0]
			r.Validatoren = v
		},
		"Validator gross": func(r *QuantenWBestaetigenAnfrage) {
			v := append([]string(nil), r.Validatoren...)
			v[2] = strings.ToUpper(v[2])
			r.Validatoren = v
		},
		"n zu gross":                func(r *QuantenWBestaetigenAnfrage) { r.N = quantenWNMax + 1 },
		"Auftrag ohne Unterschrift": func(r *QuantenWBestaetigenAnfrage) { r.PraegerSig = "" },
		"Auftrag fremd unterschrieben": func(r *QuantenWBestaetigenAnfrage) {
			*r = q.auftrag(*r, q.w.anna)
			r.Praeger = q.w.praeger
		},
		"Auftrag fuer andere Toleranz": func(r *QuantenWBestaetigenAnfrage) { s := *r; s.Tol = 2; *r = q.auftrag(s, q.w.praeger); r.Tol = 4 },
		"Praeger kein Mensch": func(r *QuantenWBestaetigenAnfrage) {
			k, a := neuerSchluessel(t)
			q.w.keys[a] = k
			*r = q.auftrag(*r, a)
		},
	} {
		r := gut
		r.Anteile = append([]string(nil), gut.Anteile...)
		r.Blaetter = append([]string(nil), gut.Blaetter...)
		aendern(&r)
		if _, _, err := d.bestaetigen(r); err == nil {
			t.Errorf("%s: bestaetigt", name)
		}
	}
	if len(d.eintraege) != 0 {
		t.Fatal("abgewiesene Bestaetigung abgelegt")
	}
	// Abgefangener Auftrag mit anderen Anteilen: die Unterschrift des
	// Praegers deckt seit v2 den Hash der Anteile.
	umgeschrieben := gut
	umgeschrieben.Anteile = append([]string{"1"}, gut.Anteile[1:]...)
	if _, _, err := d.bestaetigen(umgeschrieben); err == nil {
		t.Fatal("Auftrag mit ausgetauschten Anteilen bestaetigt")
	}
	anker1, sig1, err := d.bestaetigen(gut)
	if err != nil {
		t.Fatalf("Gegenprobe: %v", err)
	}
	// Derselbe Auftrag noch einmal (Antwort verloren): dieselbe Bestaetigung.
	anker2, sig2, err := d.bestaetigen(gut)
	if err != nil || anker2 != anker1 || sig2 != sig1 {
		t.Fatalf("Wiederholung: %v, Anker gleich %v", err, anker2 == anker1)
	}
	// Ein ANDERER Auftrag fuer dasselbe K: abgewiesen, der Tresor bleibt.
	anders := gut
	anders.Zeit = gut.Zeit + 1
	anders = q.auftrag(anders, q.w.praeger)
	if _, _, err := d.bestaetigen(anders); err == nil || !strings.Contains(err.Error(), "anderer Auftrag") {
		t.Fatalf("zweiter Auftrag fuer dasselbe K: %v", err)
	}
	if len(d.eintraege) != 1 {
		t.Fatal("Tresor hat mehr als einen Eintrag")
	}
	// Ein Praeger ohne Deckung belegt keinen Platz.
	alt := quantenWPraegerTauglich
	quantenWPraegerTauglich = func(*ChainState, string) bool { return false }
	r2 := q.anfrageBestaetigen(1)
	r2.K = strings.Repeat("a", 64)
	if _, _, err := d.bestaetigen(q.auftrag(r2, q.w.praeger)); err == nil {
		t.Fatal("Praeger ohne Deckung bestaetigt")
	}
	quantenWPraegerTauglich = alt
	// Je Praeger hoechstens quantenWOffeneJePraeger nicht gepraegte Bestaetigungen.
	for i := 1; i < quantenWOffeneJePraeger; i++ {
		ri := q.anfrageBestaetigen(1)
		ri.K = fmt.Sprintf("%064x", 1000+i)
		if _, _, err := d.bestaetigen(q.auftrag(ri, q.w.praeger)); err != nil {
			t.Fatalf("Bestaetigung %d: %v", i, err)
		}
	}
	rz := q.anfrageBestaetigen(1)
	rz.K = fmt.Sprintf("%064x", 2000)
	if _, _, err := d.bestaetigen(q.auftrag(rz, q.w.praeger)); err == nil {
		t.Fatal("mehr als quantenWOffeneJePraeger offene Bestaetigungen eines Praegers")
	}
	if _, _, err := d.bestaetigen(q.auftrag(rz, q.w.anna)); err != nil {
		t.Fatalf("anderer Praeger abgewiesen: %v", err)
	}
	// Tresor voll.
	d.eintraege = map[string]*quantenWTresorEintrag{}
	for i := 0; i < quantenWTresorHoechstens; i++ {
		d.eintraege[fmt.Sprintf("%064x", i)] = &quantenWTresorEintrag{}
	}
	if _, _, err := d.bestaetigen(gut); err == nil {
		t.Fatal("Tresor ueber der Grenze")
	}
}

// Anteile: nur mit offener Anfrage dieser Nummer und erst nach der Antwort.
func TestQuantenWDienst_AnteileMissbrauch(t *testing.T) {
	q := neueQwDienste(t, 200)
	w := q.w
	d := q.d[0]
	if _, err := d.anteile(w.p.K, 0, 0); err == nil {
		t.Fatal("Anteile einer nicht bestaetigten Note")
	}
	q.praegen(t)
	if _, err := d.anteile(w.p.K, 0, 0); err == nil {
		t.Fatal("Anteile ohne Anfrage")
	}
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", q.zeit), q.zeit+1)
	if _, err := d.anteile(w.p.K, 0, 0); err == nil {
		t.Fatal("Anteile vor dem Zufall")
	}
	q.zeit += 2
	q.takt()
	if _, err := d.anteile(w.p.K, 0, 0); err == nil {
		t.Fatal("Anteile vor der Antwort")
	}
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	for name, f := range map[string]func() error{
		"andere Nummer": func() error { _, err := d.anteile(w.p.K, 1, 0); return err },
		"ab negativ":    func() error { _, err := d.anteile(w.p.K, 0, -1); return err },
		"ab am Ende":    func() error { _, err := d.anteile(w.p.K, 0, 16); return err },
		"fremde Note":   func() error { _, err := d.anteile(strings.Repeat("e", 64), 0, 0); return err },
	} {
		if f() == nil {
			t.Errorf("%s: herausgegeben", name)
		}
	}
	out, err := d.anteile(w.p.K, 0, 0)
	if err != nil || len(out.Stellen) != 16 || out.X != 1 {
		t.Fatalf("Gegenprobe: %v %+v", err, out)
	}
	for j, i := range out.Stellen {
		if out.Y[j] != w.p.anteile[i][0].Text(16) || len(out.Pfade[j]) != quantenWMerkleTiefe(w.p.n) {
			t.Fatal("falscher Anteil oder Pfad herausgegeben")
		}
	}
}

// Tresor: verschluesselt abgelegt, nach dem Neustart wieder da; ein fremder
// Schluessel oder ein veraenderter Eintrag -- der Dienst startet nicht.
func TestQuantenWDienst_Tresor(t *testing.T) {
	q := neueQwDienste(t, 200)
	ablage := &quantenWSpeicherAblage{}
	d, err := neuerQuantenWDienst(q.w.cs, q.w.valKeys[0], ablage, nil, q.einreichen)
	if err != nil {
		t.Fatal(err)
	}
	d.jetzt = func() int64 { return q.zeit }
	gut := q.anfrageBestaetigen(1)
	if _, _, err := d.bestaetigen(gut); err != nil {
		t.Fatal(err)
	}
	roh, _ := ablage.alle()
	for _, b := range roh {
		for _, geheim := range []string{gut.Anteile[0], gut.Anteile[1], d.eintraege[gut.K].Geheim} {
			if strings.Contains(string(b), geheim) || strings.Contains(hex.EncodeToString(b), geheim) {
				t.Fatal("Klartext im Tresor")
			}
		}
	}
	d2, err := neuerQuantenWDienst(q.w.cs, q.w.valKeys[0], ablage, nil, q.einreichen)
	if err != nil || d2.eintraege[gut.K] == nil || d2.eintraege[gut.K].Geheim != d.eintraege[gut.K].Geheim {
		t.Fatalf("Neustart: %v", err)
	}
	if _, err := neuerQuantenWDienst(q.w.cs, q.w.valKeys[1], ablage, nil, q.einreichen); err == nil {
		t.Fatal("fremder Schluessel oeffnet den Tresor")
	}
	for k, b := range roh {
		b[len(b)-1] ^= 1
		ablage.setzen(k, b)
	}
	if _, err := neuerQuantenWDienst(q.w.cs, q.w.valKeys[0], ablage, nil, q.einreichen); err == nil {
		t.Fatal("veraenderter Eintrag geoeffnet")
	}
	// Ein Eintrag unter fremdem K (Zusatzdaten) oeffnet sich nicht.
	for _, b := range roh {
		b[len(b)-1] ^= 1
		ablage.m = map[string][]byte{strings.Repeat("f", 64): b}
	}
	if _, err := neuerQuantenWDienst(q.w.cs, q.w.valKeys[0], ablage, nil, q.einreichen); err == nil {
		t.Fatal("Eintrag unter fremdem K geoeffnet")
	}
}

// Fristen reicht der Hintergrund selbst ein; schlafend tut er nichts.
func TestQuantenWDienst_FristUndSchlaf(t *testing.T) {
	q := neueQwDienste(t, 200)
	w := q.w
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	q.zeit += quantenWFristAntwort + 1 // anna antwortet nicht
	q.d[3].takt(context.Background())
	if w.anfrage(w.cs) != nil || w.cs.quantenWJetzt().Noten[w.p.K].note.Fehl != 1 {
		t.Fatal("Frist nicht eingereicht")
	}
	// Schlafend: keine Einreichung.
	w.annehmen(w.anfrageTx(w.anna, 1, 16, "", q.zeit), q.zeit+1)
	quantenWAbOverride.Store(1 << 62)
	q.takt()
	quantenWAbOverride.Store(qwT0 - 86400)
	if a := w.anfrage(w.cs); a == nil || a.Basen != nil {
		t.Fatal("schlafender Dienst hat Zufall eingereicht")
	}
}

// Pfade aus den einmal gebauten Ebenen = Pfade je Stelle.
func TestQuantenWDienst_PfadeAusEbenen(t *testing.T) {
	for _, n := range []int{1, 2, 3, 7, 64, 200, 1000} {
		var bl [][32]byte
		for i := 0; i < n; i++ {
			bl = append(bl, quantenWHash([]byte{byte(i), byte(i >> 8)}))
		}
		eb := quantenWMerkleEbenen(bl)
		for i := 0; i < n; i++ {
			if fmt.Sprint(quantenWPfadAusEbenen(eb, i)) != fmt.Sprint(quantenWMerklePfad(bl, i)) {
				t.Fatalf("n %d, Stelle %d: Pfade verschieden", n, i)
			}
		}
	}
}

func TestQuantenWDienst_Auswahlen(t *testing.T) {
	if got := quantenWAuswahlen(4, 3, 10); fmt.Sprint(got) != "[[0 1 2] [0 1 3] [0 2 3]]" {
		t.Fatalf("Auswahlen %v", got)
	}
	if len(quantenWAuswahlen(64, 3, quantenWSammelVersuche)) != quantenWSammelVersuche || quantenWAuswahlen(2, 3, 10) != nil {
		t.Fatal("Grenzen der Auswahl")
	}
	a := QuantenWAnteilAusgabe{X: 1, Stellen: []int{1}, Y: []string{"1"}, Pfade: [][]string{{}}}
	b := QuantenWAnteilAusgabe{X: 1, Stellen: []int{1}, Y: []string{"2"}, Pfade: [][]string{{}}}
	c := QuantenWAnteilAusgabe{X: 2, Stellen: []int{2}, Y: []string{"2"}, Pfade: [][]string{{}}}
	if quantenWSammeln([]QuantenWAnteilAusgabe{a, b}, []int{0, 1}, 2) != nil || quantenWSammeln([]QuantenWAnteilAusgabe{a, c}, []int{0, 1}, 2) != nil {
		t.Fatal("doppeltes x oder andere Stellen gesammelt")
	}
}

// Nichts vor der Finalitaet: den Wert der Hash-Kette gibt es erst, wenn die
// Anfrage final ist, die Anteile erst, wenn die Antwort final ist -- und eine
// ANDERE Antwort an derselben Stelle wartet neu.
func TestQuantenWDienst_NichtsVorDerFinalitaet(t *testing.T) {
	q := neueQwDienste(t, 200)
	w := q.w
	q.hoehe.Store(100)
	q.final.Store(50)
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", q.zeit), q.zeit+1)
	q.zeit += 2

	// Die Anfrage steht, ist aber nicht final: kein Validator legt seinen
	// Wert offen -- auch nicht, wenn die Kette weiterwaechst.
	q.takt()
	q.hoehe.Store(120)
	q.final.Store(99)
	q.takt()
	note := w.cs.quantenWJetzt().Noten[w.p.K].note
	for x, zi := range note.ZIndex {
		if zi >= 0 {
			t.Fatalf("Validator %d hat seinen Zufall vor der Finalitaet der Anfrage offengelegt", x+1)
		}
	}
	q.final.Store(100) // die Hoehe der ersten Sichtung ist final
	q.takt()
	a := w.anfrage(w.cs)
	if a == nil || a.Basen == nil {
		t.Fatal("nach der Finalitaet der Anfrage kein Zufall")
	}

	// Der Stand vor der Antwort, um sie "zuruecknehmen" zu koennen (wie eine
	// Umordnung, die ihren Block verwirft).
	vorCs, vorNach := w.cs.quantenWStandJetzt(), w.nach.quantenWStandJetzt()
	falsch := make([]int, len(a.Stellen)) // geraten
	w.annehmen(w.antwortTx(w.anna, 0, falsch), q.zeit+1)
	q.zeit += 2
	for x, d := range q.d {
		if _, err := d.anteile(w.p.K, 0, 0); err == nil {
			t.Fatalf("Validator %d gibt Anteile heraus, bevor die Antwort final ist", x+1)
		}
	}
	// Kein Validator legt offen (er bekaeme von sich selbst keine Anteile).
	q.takt()
	if w.anfrage(w.cs) == nil || w.anfrage(w.cs).Offen != 0 {
		t.Fatal("Offenlegung vor der Finalitaet der Antwort")
	}
	q.final.Store(119) // eine Hoehe unter der Sichtung der Antwort
	if _, err := q.d[0].anteile(w.p.K, 0, 0); err == nil {
		t.Fatal("Anteile eine Hoehe vor der Finalitaet")
	}

	// DER ANGRIFF: die geratene Antwort faellt aus der Kette, die Hoehe ihrer
	// Sichtung wird final, und der Anfragende reicht eine andere ein. Die
	// Sichtung der ersten zaehlt fuer die zweite nicht.
	w.cs.quantenWReg.Store(vorCs)
	w.nach.quantenWReg.Store(vorNach)
	q.hoehe.Store(130)
	q.final.Store(120)
	a = w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	q.zeit += 2
	for x, d := range q.d {
		if _, err := d.anteile(w.p.K, 0, 0); err == nil {
			t.Fatalf("Validator %d gibt nach dem Tausch der Antwort sofort Anteile heraus", x+1)
		}
	}
	q.final.Store(130)
	if _, err := q.d[0].anteile(w.p.K, 0, 0); err != nil {
		t.Fatalf("nach der Finalitaet der Antwort keine Anteile: %v", err)
	}
	q.takt()
	if w.anfrage(w.cs) != nil || w.stand(w.cs, w.anna) != 100_000_000 {
		t.Fatal("die Pruefung der echten Note ist nicht als echt geendet")
	}
	// Der Takt vergisst Sichtungen, deren Stand es nicht mehr gibt.
	q.takt()
	for x, d := range q.d {
		if len(d.sicht) != 0 {
			t.Fatalf("Validator %d merkt sich noch %d Sichtungen", x+1, len(d.sicht))
		}
	}

	// Ohne Quelle fuer Hoehe und Finalitaet (ein Dienst, den niemand
	// angeschlossen hat) wird nichts reif.
	d, err := neuerQuantenWDienst(w.cs, w.valKeys[0], &quantenWSpeicherAblage{}, nil, q.einreichen)
	if err != nil {
		t.Fatal(err)
	}
	if d.reif("x") || d.reif("x") {
		t.Fatal("ein Dienst ohne Finalitaetsquelle haelt etwas fuer final")
	}
	// Die Karte der Sichtungen ist begrenzt; voll = nichts Neues wird reif.
	voll := q.d[1]
	for i := 0; i < quantenWSichtHoechstens; i++ {
		voll.sicht[strconv.Itoa(i)] = 0
	}
	if voll.reif("neu") {
		t.Fatal("bei voller Karte wird ein neuer Stand reif")
	}
	if len(voll.sicht) != quantenWSichtHoechstens {
		t.Fatal("die Karte der Sichtungen waechst ueber ihre Grenze")
	}
}

// Drei Pruefungen, die in der Gegenprobe (Mutation) kein Test bemerkte:
// der Auftrag fuer die Nummer eines anderen Validators, der volle Tresor und
// der Takt vor dem Stichtag.
func TestQuantenWDienst_NummerTresorStichtag(t *testing.T) {
	q := neueQwDienste(t, 200)
	w := q.w
	// Ein gueltig unterschriebener Auftrag fuer Nummer 2 bei Validator 1:
	// er wuerde fremde Anteile verwahren und unter seinem Schluessel fuer
	// eine Nummer bestaetigen, die nicht seine ist.
	if _, _, err := q.d[0].bestaetigen(q.anfrageBestaetigen(2)); err == nil {
		t.Fatal("Validator 1 bestaetigt den Auftrag fuer Nummer 2")
	}
	if len(q.d[0].eintraege) != 0 {
		t.Fatal("abgewiesen und trotzdem verwahrt")
	}
	// Die Praegezeit ausserhalb des Fensters, vom Praeger gueltig
	// unterschrieben: eine Bestaetigung, die fuer kein Praegen mehr gilt
	// (zu alt) oder die Uhr vorwegnimmt, belegte nur den Tresor.
	for name, zeit := range map[string]int64{"zu alt": q.zeit - quantenWAlt - 1, "zu weit voraus": q.zeit + quantenWVoraus + 1} {
		r := q.anfrageBestaetigen(1)
		r.Zeit = zeit
		if _, _, err := q.d[0].bestaetigen(q.auftrag(r, w.praeger)); err == nil {
			t.Fatalf("Praegezeit %s bestaetigt", name)
		}
	}
	for name, zeit := range map[string]int64{"gerade noch alt": q.zeit - quantenWAlt, "gerade noch voraus": q.zeit + quantenWVoraus} {
		r := q.anfrageBestaetigen(1)
		r.Zeit = zeit
		if _, _, err := q.d[0].bestaetigen(q.auftrag(r, w.praeger)); err != nil {
			t.Fatalf("Praegezeit %s abgewiesen: %v", name, err)
		}
		delete(q.d[0].eintraege, r.K)
	}
	// Voller Tresor: fail-closed, auch fuer einen sonst gueltigen Auftrag.
	voll := q.d[1]
	for i := 0; i < quantenWTresorHoechstens; i++ {
		k := fmt.Sprintf("%064x", i+1)
		voll.eintraege[k] = &quantenWTresorEintrag{K: k, Praeger: "x", Zeit: q.zeit}
	}
	if _, _, err := voll.bestaetigen(q.anfrageBestaetigen(2)); err == nil || !strings.Contains(err.Error(), "Tresor voll") {
		t.Fatalf("voller Tresor nimmt noch an: %v", err)
	}
	for k := range voll.eintraege {
		delete(voll.eintraege, k)
	}

	// Vor dem Stichtag tut der Takt nichts, auch wenn eine Anfrage wartet.
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", q.zeit), q.zeit+1)
	q.zeit += 2
	// Gezaehlt wird, was der Dienst einreichen WILL: die Kette wiese es vor
	// dem Stichtag ohnehin ab, der Takt soll es gar nicht erst versuchen.
	versuche := 0
	for _, d := range q.d {
		echt := d.einreichen
		d.einreichen = func(art string, a QuantenWAngaben) error { versuche++; return echt(art, a) }
	}
	quantenWAbOverride.Store(q.zeit + 3600) // der Stichtag liegt noch vor uns
	q.takt()
	if a := w.anfrage(w.cs); a == nil || a.Basen != nil || versuche != 0 {
		t.Fatalf("der Takt arbeitet vor dem Stichtag (%d Einreichungen versucht)", versuche)
	}
	quantenWAbOverride.Store(qwT0 - 86400)
	q.takt()
	if a := w.anfrage(w.cs); a == nil || a.Basen == nil {
		t.Fatal("nach dem Stichtag kein Zufall")
	}
}

// Staffel: nur der zustaendige Validator reicht sofort ein; die anderen in
// fester Reihenfolge -- Rang r (Abstand zum Zustaendigen) erst, wenn der Teil
// r * quantenWStaffelWarten lang offen ist (Zustaendiger faellt aus). Wer
// eingesprungen ist, macht den Versuch ohne weitere Wartezeit zu Ende.
func TestQuantenWDienst_Staffel(t *testing.T) {
	bereit := func(t *testing.T) (*qwDienste, int) {
		q := neueQwDienste(t, 400)
		w := q.w
		q.praegen(t)
		q.zeit += 10
		w.annehmen(w.anfrageTx(w.anna, 0, 150, "", q.zeit), q.zeit+1) // 150 Stellen: drei Teile
		q.zeit += 2
		q.takt()
		a := w.anfrage(w.cs)
		w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
		q.zeit += 2
		return q, quantenWZustaendig(w.p.K, 0, len(w.val))
	}
	t.Run("Rang 1 springt ein und macht weiter", func(t *testing.T) {
		q, z := bereit(t)
		w := q.w
		v := len(q.d)
		rang1, rang2 := q.d[z%v], q.d[(z+1)%v]
		rang1.takt(context.Background())
		rang2.takt(context.Background())
		if w.anfrage(w.cs).Offen != 0 {
			t.Fatal("ein nicht zustaendiger Validator reicht sofort ein")
		}
		q.zeit += quantenWStaffelWarten - 1
		rang1.takt(context.Background())
		if w.anfrage(w.cs).Offen != 0 {
			t.Fatal("eingesprungen vor der Wartezeit")
		}
		q.zeit++
		rang2.takt(context.Background())
		if w.anfrage(w.cs).Offen != 0 {
			t.Fatal("Rang 2 springt zugleich mit Rang 1 ein")
		}
		// Rang 1 legt alle drei Teile in einem Takt offen -- ohne fuer jeden
		// Teil die Staffel neu abzuwarten.
		rang1.takt(context.Background())
		if w.anfrage(w.cs) != nil {
			t.Fatalf("Rang 1 hat nach dem Einspringen nicht weitergemacht (Offen %d)", w.anfrage(w.cs).Offen)
		}
		// Wartezeiten und Einspringen vergisst der naechste Takt.
		q.takt()
		for x, d := range q.d {
			if len(d.faellig) != 0 || len(d.eingesprungen) != 0 {
				t.Fatalf("Validator %d merkt sich noch %d offene Teile, %d Versuche", x+1, len(d.faellig), len(d.eingesprungen))
			}
		}
	})
	t.Run("dranbleiben ueber Takte", func(t *testing.T) {
		q, z := bereit(t)
		w := q.w
		rang1 := q.d[z%len(q.d)]
		echt := rang1.einreichen
		teile := 0
		// Nur der erste Teil geht in diesem Takt durch (die anderen weist der
		// Leiter gerade ab, etwa wegen der Burst-Grenze).
		rang1.einreichen = func(art string, x QuantenWAngaben) error {
			if art == quantenWOffenlegenTxArt {
				if teile++; teile > 1 {
					return errors.New("Status 429")
				}
			}
			return echt(art, x)
		}
		rang1.takt(context.Background())
		q.zeit += quantenWStaffelWarten
		rang1.takt(context.Background())
		if o := w.anfrage(w.cs).Offen; o != quantenWOffenTeil {
			t.Fatalf("Rang 1 nicht eingesprungen (Offen %d)", o)
		}
		// Naechster Takt, eine Sekunde spaeter: Rang 1 macht ohne neue
		// Wartezeit weiter.
		rang1.einreichen = echt
		q.zeit++
		rang1.takt(context.Background())
		if w.anfrage(w.cs) != nil {
			t.Fatal("Rang 1 wartet nach dem Einspringen fuer den naechsten Teil wieder die Staffel ab")
		}
	})
	t.Run("dranbleiben, waehrend der Stand nachzieht", func(t *testing.T) {
		q, z := bereit(t)
		w := q.w
		rang1 := q.d[z%len(q.d)]
		echt := rang1.einreichen
		var unterwegs []QuantenWAngaben
		// Angenommen, aber noch nicht im Stand (Weiterleitung zum Leiter).
		rang1.einreichen = func(art string, x QuantenWAngaben) error {
			if art == quantenWOffenlegenTxArt {
				unterwegs = append(unterwegs, x)
				return nil
			}
			return echt(art, x)
		}
		rang1.takt(context.Background())
		q.zeit += quantenWStaffelWarten
		rang1.takt(context.Background()) // springt ein, Teil unterwegs
		q.zeit++
		rang1.takt(context.Background()) // wartet nur auf den Stand
		if len(unterwegs) != 1 || len(rang1.eingesprungen) != 1 {
			t.Fatalf("%d Teile unterwegs, Einspringen gemerkt: %d", len(unterwegs), len(rang1.eingesprungen))
		}
		// Der Block kommt; danach geht es ohne neue Wartezeit weiter.
		if err := echt(quantenWOffenlegenTxArt, unterwegs[0]); err != nil {
			t.Fatal(err)
		}
		rang1.einreichen = echt
		q.zeit++
		rang1.takt(context.Background())
		if w.anfrage(w.cs) != nil {
			t.Fatal("nach dem Warten auf den Stand wieder die Staffel abgewartet")
		}
	})
	t.Run("Rang 2 erst nach doppelter Wartezeit", func(t *testing.T) {
		q, z := bereit(t)
		w := q.w
		rang2 := q.d[(z+1)%len(q.d)]
		rang2.takt(context.Background())
		q.zeit += 2*quantenWStaffelWarten - 1
		rang2.takt(context.Background())
		if w.anfrage(w.cs).Offen != 0 {
			t.Fatal("Rang 2 springt vor der doppelten Wartezeit ein")
		}
		q.zeit++
		rang2.takt(context.Background())
		if w.anfrage(w.cs) != nil {
			t.Fatal("Rang 2 springt nicht ein, obwohl Zustaendiger und Rang 1 fehlen")
		}
	})
}

// Nicht nochmal einreichen: Steht ein eingereichter Teil noch nicht im
// eigenen Stand (der Zustaendige ist nicht der Leiter, der Block kommt erst),
// reicht der Dienst ihn weder im selben Takt noch im naechsten erneut ein --
// erst nach quantenWStaffelWarten (falls er unterwegs verloren ging).
// Befund 17.11.2026: Vorher holte jede Runde die Anteile neu und reichte
// denselben Teil wieder ein, gegen die Burst-Grenzen.
func TestQuantenWDienst_TeilNichtDoppelt(t *testing.T) {
	q := neueQwDienste(t, 400)
	w := q.w
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 150, "", q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	q.zeit += 2
	d := q.d[quantenWZustaendig(w.p.K, 0, len(w.val))-1]
	echt := d.einreichen
	gesendet := 0
	// Angenommen, aber (noch) nicht im Stand: wie die Weiterleitung zum Leiter.
	d.einreichen = func(art string, x QuantenWAngaben) error {
		if art == quantenWOffenlegenTxArt {
			gesendet++
			return nil
		}
		return echt(art, x)
	}
	d.takt(context.Background())
	if gesendet != 1 {
		t.Fatalf("im selben Takt %d-mal eingereicht, erwartet 1", gesendet)
	}
	q.zeit += quantenWStaffelWarten - 1
	d.takt(context.Background())
	if gesendet != 1 {
		t.Fatalf("vor der Wartezeit erneut eingereicht (%d)", gesendet)
	}
	q.zeit++
	d.takt(context.Background())
	if gesendet != 2 {
		t.Fatalf("nach der Wartezeit nicht erneut eingereicht (%d)", gesendet)
	}
	// Kommt der Stand nach, geht es ohne Wartezeit weiter.
	d.einreichen = echt
	q.zeit += quantenWStaffelWarten
	d.takt(context.Background())
	if w.anfrage(w.cs) != nil {
		t.Fatal("der Zustaendige hat die Offenlegung nicht abgeschlossen")
	}
	q.takt()
	if len(d.gesendet) != 0 {
		t.Fatalf("%d eingereichte Teile noch gemerkt", len(d.gesendet))
	}
}
