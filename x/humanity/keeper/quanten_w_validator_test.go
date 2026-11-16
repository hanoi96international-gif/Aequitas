package keeper

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// qwValWelt: die Welt von quanten_w_kette_test.go, dazu je Validator ein
// eigener Dienst mit eigenem Tresor. Die Dienste teilen sich eine Kette
// (w.cs, nachgespielt auf w.nach); Uhr, DAG-Hoehe und Finalitaet stellt der
// Test.
type qwValWelt struct {
	*qwWelt
	dienste []*quantenWValidator
	uhr     int64
	hoehe   int64
	final   int64
	best    []*QuantenWBestaetigung
	// stumm: diese Raenge (1-basiert) sind fuer andere nicht erreichbar.
	stumm map[int]bool
	// luege: diese Raenge liefern anderen falsche Anteile.
	luege map[int]bool
	// eingereicht: was die Dienste ueber den Annahmeweg geschickt haben.
	eingereicht []string
}

func neueQwValWelt(t *testing.T, n int) *qwValWelt {
	t.Helper()
	w := &qwValWelt{qwWelt: neueQwWelt(t, n), uhr: qwT0, hoehe: 100, final: 50, stumm: map[int]bool{}, luege: map[int]bool{}}
	urls := map[string]string{}
	for x, a := range w.val {
		urls[a] = fmt.Sprintf("val://%d", x+1)
	}
	for x := range w.val {
		v, err := neuerQuantenWValidator(w.cs, w.valKeys[x])
		if err != nil {
			t.Fatal(err)
		}
		v.hoehe = func() int64 { return w.hoehe }
		v.final = func() int64 { return w.final }
		v.urls = func() map[string]string { return urls }
		v.einreichen = w.einreichen
		v.holeAnteile = w.holeAnteile
		w.dienste = append(w.dienste, v)
	}
	return w
}

// einreichen: der Annahmeweg der Dienste -- dieselbe Pruefung wie
// quantenWAnnehmen, mit der Uhr des Tests, und der zweite Knoten spielt nach.
func (w *qwValWelt) einreichen(art string, q *QuantenWAngaben) error {
	kopie := *q
	tx := Transaction{Type: art, QuantenW: &kopie}
	u := w.bescheid(w.cs, tx, w.uhr)
	if u.regel != "" {
		return errors.New(u.regel + ": " + u.text)
	}
	w.annehmen(tx, w.uhr)
	w.eingereicht = append(w.eingereicht, art)
	return nil
}

func (w *qwValWelt) holeAnteile(basis, k string, index int) (*quantenWAnteileAntwort, error) {
	var rang int
	if _, err := fmt.Sscanf(basis, "val://%d", &rang); err != nil || rang < 1 || rang > len(w.dienste) {
		return nil, errors.New("unbekannte Adresse")
	}
	if w.stumm[rang] {
		return nil, errors.New("nicht erreichbar")
	}
	r, err := w.dienste[rang-1].anteileFuer(k, index, w.uhr)
	if err != nil {
		return nil, err
	}
	if w.luege[rang] {
		y, _ := new(big.Int).SetString(r.Anteile[0], 16)
		r.Anteile[0] = y.Add(y, big.NewInt(1)).Mod(y, quantenWP).Text(16)
	}
	return r, nil
}

// hinterlegung: was der Praeger dem Validator mit Rang x (1-basiert) schickt.
func (w *qwValWelt) hinterlegung(x int, zeit int64) *QuantenWHinterlegung {
	p := w.p
	h := &QuantenWHinterlegung{Wallet: w.praeger, Zeit: zeit, K: p.K, N: p.n, T: p.t,
		Wurzel: hex.EncodeToString(p.wurzel[:]), Validatoren: append([]string(nil), w.val...)}
	anteile := make([]*big.Int, p.n)
	for i := 0; i < p.n; i++ {
		anteile[i] = p.anteile[i][x-1]
		h.Anteile = append(h.Anteile, p.anteile[i][x-1].Text(16))
		h.Blaetter = append(h.Blaetter, hex.EncodeToString(p.blaetter[i][:]))
	}
	w.unterschreiben(h, x, anteile)
	return h
}

func (w *qwValWelt) unterschreiben(h *QuantenWHinterlegung, x int, anteile []*big.Int) {
	h.WalletSig = personalSign(w.t, w.keys[h.Wallet], quantenWHinterlegenNachricht(h.Wallet, h.K, h.N, h.T, h.Wurzel, h.Validatoren, x,
		quantenWInhalt(anteile, w.p.blaetter), h.Zeit))
}

// praegen: bei allen hinterlegen, mit ihren Bestaetigungen praegen.
func (w *qwValWelt) praegen(deckung int64) {
	w.t.Helper()
	zeit := w.uhr
	q := &QuantenWAngaben{K: w.p.K, N: w.p.n, T: w.p.t, Wurzel: hex.EncodeToString(w.p.wurzel[:]), Validatoren: w.val, Deckung: deckung}
	w.best = nil
	for x := range w.dienste {
		b, err := w.dienste[x].hinterlegen(w.hinterlegung(x+1, zeit), w.uhr)
		if err != nil {
			w.t.Fatalf("Validator %d bestaetigt nicht: %v", x+1, err)
		}
		if b.X != x+1 || b.Validator != w.val[x] || b.Zeit != zeit {
			w.t.Fatalf("Bestaetigung von Validator %d: %+v", x+1, b)
		}
		w.best = append(w.best, b)
		q.Anker = append(q.Anker, b.Anker)
		q.Bestaetigungen = append(q.Bestaetigungen, b.Bestaetigung)
	}
	q.WalletSig = personalSign(w.t, w.keys[w.praeger], quantenWPraegenNachricht(w.praeger, q.K, q.N, q.T, q.Wurzel, w.val, q.Anker, deckung, zeit))
	w.uhr++
	w.annehmen(Transaction{Type: quantenWPraegenTxArt, Wallet: w.praeger, DistributionAt: zeit, QuantenW: q}, w.uhr)
}

// takte: ein Takt bei jedem Dienst; wie viele Einreichungen angenommen wurden.
func (w *qwValWelt) takte() int {
	n := 0
	for _, v := range w.dienste {
		n += v.takt(w.uhr)
	}
	return n
}

// finalisieren: der finalisierte Checkpoint erreicht die DAG-Hoehe; die
// Kette waechst weiter.
func (w *qwValWelt) finalisieren() {
	w.final = w.hoehe
	w.hoehe += 60
}

func qwValStatus(err error) int {
	var ab *quantenWValAbweisung
	if errors.As(err, &ab) {
		return ab.status
	}
	return 0
}

// Gutfall: der Praeger hinterlegt bei fuenf Validatoren, praegt mit ihren
// Bestaetigungen; danach tun die Dienste alles von selbst -- und nichts, bevor
// der Anlass final ist.
func TestQuantenWValidator_Gutfall(t *testing.T) {
	w := neueQwValWelt(t, 200)
	summe0 := w.summe(w.cs)
	w.praegen(10_000_000)
	if w.takte() != 0 {
		t.Fatal("ohne Anfrage reicht ein Dienst etwas ein")
	}

	// bert loest ein.
	w.uhr += 10
	w.annehmen(w.anfrageTx(w.bert, 0, quantenWKMax, w.bert, w.uhr), w.uhr+1)
	w.uhr += 2
	// Die Anfrage ist gesehen, aber nicht final: kein Zufall.
	if n := w.takte(); n != 0 || w.anfrage(w.cs).Basen != nil {
		t.Fatalf("Zufall vor der Finalitaet der Anfrage (%d Einreichungen)", n)
	}
	w.hoehe += 10 // die Kette waechst, final ist sie noch nicht
	if n := w.takte(); n != 0 {
		t.Fatalf("Zufall vor der Finalitaet der Anfrage (%d Einreichungen)", n)
	}
	w.finalisieren()
	if n := w.takte(); n != 5 {
		t.Fatalf("%d von 5 Validatoren haben ihren Zufall eingereicht", n)
	}
	a := w.anfrage(w.cs)
	if a.Basen == nil || len(a.Stellen) != quantenWKMax {
		t.Fatal("Aufforderung steht nicht")
	}
	// Kein zweiter Zufall fuer dieselbe Anfrage, nichts ohne Antwort.
	if n := w.takte(); n != 0 {
		t.Fatalf("%d Einreichungen ohne Antwort", n)
	}
	// Anteile vor der Antwort: nein.
	if _, err := w.dienste[0].anteileFuer(w.p.K, 0, w.uhr); qwValStatus(err) != http.StatusConflict {
		t.Fatalf("Anteile vor der Antwort: %v", err)
	}

	w.uhr += 5
	w.annehmen(w.antwortTx(w.bert, 0, w.p.ehrlich(a.Stellen, a.Basen)), w.uhr)
	w.uhr++
	// Die Antwort steht, ist aber nicht final: keine Anteile, keine Offenlegung.
	if _, err := w.dienste[0].anteileFuer(w.p.K, 0, w.uhr); qwValStatus(err) != http.StatusTooEarly {
		t.Fatalf("Anteile vor der Finalitaet der Antwort: %v", err)
	}
	if n := w.takte(); n != 0 {
		t.Fatalf("Offenlegung vor der Finalitaet der Antwort (%d)", n)
	}
	w.finalisieren()
	// Reif; Rang 1 reicht sofort ein, die anderen warteten ihre Staffel ab.
	if n := w.takte(); n != 1 {
		t.Fatalf("%d Offenlegungen angenommen, erwartet genau eine", n)
	}
	if got := w.eingereicht[len(w.eingereicht)-1]; got != quantenWOffenlegenTxArt {
		t.Fatalf("zuletzt eingereicht: %s", got)
	}
	reg := w.cs.quantenWJetzt()
	if len(reg.Noten) != 0 || w.stand(w.cs, w.bert) != 110_000_000 || w.stand(w.cs, quantenWTopfAddr) != 0 || w.stand(w.cs, quantenWPfandAddr) != 0 {
		t.Fatalf("Einloesen: bert %d, Topf %d, Pfand %d", w.stand(w.cs, w.bert), w.stand(w.cs, quantenWTopfAddr), w.stand(w.cs, quantenWPfandAddr))
	}
	if w.summe(w.cs) != summe0 || w.summe(w.nach) != summe0 {
		t.Fatal("Erhaltung verletzt")
	}
	// Die Note ist tot: jeder Dienst raeumt seinen Tresor.
	w.takte()
	for x, v := range w.dienste {
		if len(v.tresor) != 0 || len(v.sicht) != 0 {
			t.Fatalf("Validator %d verwahrt nach dem Tod der Note noch %d Eintraege, %d Sichtungen", x+1, len(v.tresor), len(v.sicht))
		}
	}
}

// Hinterlegen: was abgewiesen wird, bevor irgendetwas unterschrieben ist.
func TestQuantenWValidator_HinterlegenMissbrauch(t *testing.T) {
	w := neueQwValWelt(t, 64)
	v := w.dienste[0]
	gut := func() *QuantenWHinterlegung { return w.hinterlegung(1, w.uhr) }
	eigene := func() []*big.Int {
		out := make([]*big.Int, w.p.n)
		for i := range out {
			out[i] = w.p.anteile[i][0]
		}
		return out
	}
	faelle := []struct {
		name   string
		status int
		bau    func() *QuantenWHinterlegung
	}{
		{"dieser Knoten nicht in der Liste", http.StatusBadRequest, func() *QuantenWHinterlegung {
			h := gut()
			_, fremd := neuerSchluessel(t)
			h.Validatoren[0] = fremd
			w.unterschreiben(h, 1, eigene())
			return h
		}},
		{"dieser Knoten zweimal in der Liste", http.StatusBadRequest, func() *QuantenWHinterlegung {
			h := gut()
			h.Validatoren[1] = h.Validatoren[0]
			w.unterschreiben(h, 1, eigene())
			return h
		}},
		{"Blaetter ergeben die Wurzel nicht", http.StatusBadRequest, func() *QuantenWHinterlegung {
			h := gut()
			h.Blaetter[3] = strings.Repeat("ab", 32)
			return h
		}},
		{"Anteil ausserhalb des Koerpers", http.StatusBadRequest, func() *QuantenWHinterlegung {
			h := gut()
			h.Anteile[0] = quantenWP.Text(16)
			return h
		}},
		{"Anteil nicht in gueltiger Form", http.StatusBadRequest, func() *QuantenWHinterlegung {
			h := gut()
			h.Anteile[0] = "0X1F"
			return h
		}},
		{"zu wenige Anteile", http.StatusBadRequest, func() *QuantenWHinterlegung {
			h := gut()
			h.Anteile = h.Anteile[:len(h.Anteile)-1]
			return h
		}},
		{"Zeit zu alt", http.StatusBadRequest, func() *QuantenWHinterlegung { return w.hinterlegung(1, w.uhr-quantenWVoraus-1) }},
		{"Zeit zu weit voraus", http.StatusBadRequest, func() *QuantenWHinterlegung { return w.hinterlegung(1, w.uhr+quantenWVoraus+1) }},
		{"Anteil nach der Unterschrift getauscht", http.StatusForbidden, func() *QuantenWHinterlegung {
			h := gut()
			h.Anteile[5] = w.p.anteile[5][1].Text(16)
			return h
		}},
		{"von einem anderen unterschrieben", http.StatusForbidden, func() *QuantenWHinterlegung {
			h := gut()
			h.WalletSig = personalSign(t, w.keys[w.anna], quantenWHinterlegenNachricht(h.Wallet, h.K, h.N, h.T, h.Wurzel, h.Validatoren, 1, quantenWInhalt(eigene(), w.p.blaetter), h.Zeit))
			return h
		}},
		{"Unterschrift fuer einen anderen Rang", http.StatusForbidden, func() *QuantenWHinterlegung {
			h := gut()
			w.unterschreiben(h, 2, eigene())
			return h
		}},
		{"ohne Unterschrift", http.StatusForbidden, func() *QuantenWHinterlegung {
			h := gut()
			h.WalletSig = ""
			return h
		}},
		{"Praeger kein Mensch", http.StatusForbidden, func() *QuantenWHinterlegung {
			k, a := neuerSchluessel(t)
			w.keys[a] = k
			h := gut()
			h.Wallet = a
			w.unterschreiben(h, 1, eigene())
			return h
		}},
		{"Praeger ohne Mindestdeckung", http.StatusForbidden, func() *QuantenWHinterlegung {
			k, a := neuerSchluessel(t)
			w.keys[a] = k
			addHuman(w.cs, a, 0.5)
			h := gut()
			h.Wallet = a
			w.unterschreiben(h, 1, eigene())
			return h
		}},
	}
	for _, f := range faelle {
		if _, err := v.hinterlegen(f.bau(), w.uhr); qwValStatus(err) != f.status {
			t.Errorf("%s: %v (Status %d), erwartet %d", f.name, err, qwValStatus(err), f.status)
		}
		if len(v.tresor) != 0 {
			t.Fatalf("%s: abgewiesen und trotzdem verwahrt", f.name)
		}
	}

	// Angenommen; dieselbe Hinterlegung noch einmal: dieselbe Bestaetigung.
	b1, err := v.hinterlegen(gut(), w.uhr)
	if err != nil {
		t.Fatal(err)
	}
	anker, _ := quantenWHex32(b1.Anker)
	msg := quantenWBestaetigungNachricht(w.p.K, w.p.wurzel, w.p.n, w.p.t, w.val, 1, anker, w.uhr)
	if pruefePersonalSignGemerkt(msg, b1.Bestaetigung, w.val[0]) != nil || !kanonischeSignatur(b1.Bestaetigung) {
		t.Fatal("die Bestaetigung prueft nicht gegen die Signieradresse des Validators")
	}
	b2, err := v.hinterlegen(gut(), w.uhr+5)
	if err != nil || *b2 != *b1 {
		t.Fatalf("Wiederholung: %v, %+v statt %+v", err, b2, b1)
	}
	// Ein ZWEITER Inhalt fuer dasselbe K: abgewiesen, der erste bleibt --
	// je K unterschreibt der Validator genau eine Wurzel und einen Anker.
	anders := qwPraegen(64, 3, 5, 77)
	h := gut()
	h.Wurzel = hex.EncodeToString(anders.wurzel[:])
	h.Blaetter = nil
	anteile := make([]*big.Int, 64)
	for i := 0; i < 64; i++ {
		anteile[i] = anders.anteile[i][0]
		h.Anteile[i] = anteile[i].Text(16)
		h.Blaetter = append(h.Blaetter, hex.EncodeToString(anders.blaetter[i][:]))
	}
	h.WalletSig = personalSign(t, w.keys[w.praeger], quantenWHinterlegenNachricht(h.Wallet, h.K, h.N, h.T, h.Wurzel, h.Validatoren, 1, quantenWInhalt(anteile, anders.blaetter), h.Zeit))
	if _, err := v.hinterlegen(h, w.uhr); qwValStatus(err) != http.StatusConflict {
		t.Fatalf("zweiter Inhalt fuer dasselbe K: %v", err)
	}
	if v.tresor[w.p.K].Wurzel != w.p.wurzel {
		t.Fatal("der zweite Inhalt hat den ersten ersetzt")
	}
	// Dieselbe Hinterlegung von einem anderen Praeger: ebenso.
	h = gut()
	h.Wallet = w.anna
	w.unterschreiben(h, 1, eigene())
	if _, err := v.hinterlegen(h, w.uhr); qwValStatus(err) != http.StatusConflict {
		t.Fatalf("fremder Praeger uebernimmt K: %v", err)
	}
}

// Grenzen: je Praeger, gesamt, und eine gepraegte Note gibt K nicht mehr her.
func TestQuantenWValidator_Grenzen(t *testing.T) {
	w := neueQwValWelt(t, 64)
	v := w.dienste[0]
	fuer := func(seed int64, wallet string) *QuantenWHinterlegung {
		p := qwPraegen(64, 3, 5, seed)
		h := &QuantenWHinterlegung{Wallet: wallet, Zeit: w.uhr, K: p.K, N: p.n, T: p.t, Wurzel: hex.EncodeToString(p.wurzel[:]), Validatoren: w.val}
		anteile := make([]*big.Int, p.n)
		for i := 0; i < p.n; i++ {
			anteile[i] = p.anteile[i][0]
			h.Anteile = append(h.Anteile, anteile[i].Text(16))
			h.Blaetter = append(h.Blaetter, hex.EncodeToString(p.blaetter[i][:]))
		}
		h.WalletSig = personalSign(t, w.keys[wallet], quantenWHinterlegenNachricht(wallet, p.K, p.n, p.t, h.Wurzel, w.val, 1, quantenWInhalt(anteile, p.blaetter), w.uhr))
		return h
	}
	for i := 0; i < quantenWTresorJeWallet; i++ {
		if _, err := v.hinterlegen(fuer(int64(1000+i), w.anna), w.uhr); err != nil {
			t.Fatalf("Hinterlegung %d: %v", i+1, err)
		}
	}
	if _, err := v.hinterlegen(fuer(1999, w.anna), w.uhr); qwValStatus(err) != http.StatusTooManyRequests {
		t.Fatalf("mehr als %d ungepraegte Hinterlegungen eines Praegers: %v", quantenWTresorJeWallet, err)
	}
	// Ein anderer Praeger ist davon nicht betroffen.
	if _, err := v.hinterlegen(fuer(2000, w.bert), w.uhr); err != nil {
		t.Fatalf("anderer Praeger: %v", err)
	}
	// Tresor voll: fail-closed.
	for i := len(v.tresor); i < quantenWTresorHoechstens; i++ {
		v.tresor[fmt.Sprintf("%064x", i)] = &quantenWTresorEintrag{K: fmt.Sprintf("%064x", i), Praeger: "x", Zeit: w.uhr}
	}
	if _, err := v.hinterlegen(fuer(3000, w.bert), w.uhr); qwValStatus(err) != http.StatusServiceUnavailable {
		t.Fatalf("Tresor voll: %v", err)
	}
	// Ungepraegt und das Praegefenster vorbei: der Takt raeumt.
	if v.takt(w.uhr+quantenWAlt+quantenWVoraus+quantenWBuchSpiel) != 0 || len(v.tresor) != quantenWTresorHoechstens {
		t.Fatal("im Praegefenster geraeumt -- die Bestaetigung galt noch")
	}
	v.takt(w.uhr + quantenWAlt + quantenWVoraus + quantenWBuchSpiel + 1)
	if len(v.tresor) != 0 {
		t.Fatalf("nach dem Praegefenster noch %d Eintraege", len(v.tresor))
	}

	// Eine gepraegte Note: ihr K nimmt kein Validator noch einmal an, auch
	// der nicht, der seinen Eintrag verloren hat.
	w.praegen(10_000_000)
	delete(v.tresor, w.p.K)
	if _, err := v.hinterlegen(w.hinterlegung(1, w.uhr), w.uhr); qwValStatus(err) != http.StatusConflict {
		t.Fatalf("Hinterlegung fuer eine lebende Note: %v", err)
	}
}

// Anteile: nur fuer die laufende, beantwortete, finale Anfrage -- und nach
// einer anderen Antwort beginnt das Warten neu.
func TestQuantenWValidator_AnteileErstNachFinalerAntwort(t *testing.T) {
	w := neueQwValWelt(t, 200)
	w.praegen(10_000_000)
	v := w.dienste[2]
	if _, err := v.anteileFuer(strings.Repeat("0", 64), 0, w.uhr); qwValStatus(err) != http.StatusNotFound {
		t.Fatalf("fremde Note: %v", err)
	}
	if _, err := v.anteileFuer(w.p.K, 0, w.uhr); qwValStatus(err) != http.StatusConflict {
		t.Fatalf("ohne Anfrage: %v", err)
	}
	w.uhr += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", w.uhr), w.uhr+1)
	w.uhr += 2
	w.takte()
	w.finalisieren()
	w.takte()
	a := w.anfrage(w.cs)
	if a.Basen == nil {
		t.Fatal("kein Zufall")
	}
	if _, err := v.anteileFuer(w.p.K, 0, w.uhr); qwValStatus(err) != http.StatusConflict {
		t.Fatalf("vor der Antwort: %v", err)
	}
	// Der Stand vor der Antwort, um sie "zuruecknehmen" zu koennen (wie eine
	// Umordnung, die den Block mit der Antwort verwirft).
	vorCs, vorNach := w.cs.quantenWStandJetzt(), w.nach.quantenWStandJetzt()
	falsch := make([]int, len(a.Stellen)) // geraten
	w.uhr++
	w.annehmen(w.antwortTx(w.anna, 0, falsch), w.uhr)
	if _, err := v.anteileFuer(w.p.K, 0, w.uhr); qwValStatus(err) != http.StatusTooEarly {
		t.Fatalf("Antwort nicht final: %v", err)
	}
	if _, err := v.anteileFuer(w.p.K, 1, w.uhr); qwValStatus(err) != http.StatusConflict {
		t.Fatalf("andere Anfrage: %v", err)
	}
	// Die Finalitaet rueckt bis knapp unter die Sichtung: weiter nein.
	w.final = w.hoehe - 1
	if _, err := v.anteileFuer(w.p.K, 0, w.uhr); qwValStatus(err) != http.StatusTooEarly {
		t.Fatalf("Finalitaet eine Hoehe unter der Sichtung: %v", err)
	}
	// DER ANGRIFF: die Antwort faellt aus der Kette, eine andere kommt.
	// Die Sichtung der ersten zaehlt fuer die zweite nicht.
	w.cs.mu.Lock()
	w.cs.quantenWReg.Store(vorCs)
	w.cs.mu.Unlock()
	w.nach.mu.Lock()
	w.nach.quantenWReg.Store(vorNach)
	w.nach.mu.Unlock()
	w.hoehe += 5
	w.final = w.hoehe - 1 // die Hoehe der ERSTEN Sichtung ist jetzt final
	a = w.anfrage(w.cs)
	w.uhr++
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), w.uhr)
	if _, err := v.anteileFuer(w.p.K, 0, w.uhr); qwValStatus(err) != http.StatusTooEarly {
		t.Fatalf("nach dem Tausch der Antwort sofort Anteile: %v", err)
	}
	w.final = w.hoehe
	r, err := v.anteileFuer(w.p.K, 0, w.uhr)
	if err != nil {
		t.Fatal(err)
	}
	if r.X != 3 || len(r.Anteile) != 16 || len(r.Pfade) != 16 {
		t.Fatalf("Antwort: %+v", r)
	}
	for i, s := range a.Stellen {
		if r.Stellen[i] != s || r.Anteile[i] != w.p.anteile[s][2].Text(16) {
			t.Fatalf("Stelle %d: falscher Anteil", s)
		}
		var pfad [][32]byte
		for _, p := range r.Pfade[i] {
			h, _ := quantenWHex32(p)
			pfad = append(pfad, h)
		}
		if !quantenWMerklePruefen(w.p.wurzel, w.p.blaetter[s], s, pfad, w.p.n) {
			t.Fatalf("Stelle %d: Pfad prueft nicht", s)
		}
	}
	// Nur die Stellen der Anfrage, nichts sonst von der Note.
	if len(r.Anteile) >= w.p.n {
		t.Fatal("mehr als die Stellen der Anfrage herausgegeben")
	}
}

// Zufall: nur der Wert fuer die laufende Anfrage, und er passt zum Anker.
func TestQuantenWValidator_ZufallNurFuerDieLaufendeAnfrage(t *testing.T) {
	w := neueQwValWelt(t, 200)
	w.praegen(10_000_000)
	e := w.dienste[0].tresor[w.p.K]
	anker := e.anker()
	for _, j := range []int{0, 1, 7, quantenWZufallL - 1} {
		z, ok := e.kettenWert(j)
		if !ok || quantenWKetteVor(z, j+1) != anker {
			t.Fatalf("Wert %d der Hash-Kette passt nicht zum Anker", j)
		}
	}
	if _, ok := e.kettenWert(quantenWZufallL); ok {
		t.Fatal("Wert jenseits der Kette")
	}
	if _, ok := e.kettenWert(-1); ok {
		t.Fatal("Wert vor der Kette")
	}
	// Zwei Validatoren stumm (ihr Takt laeuft nicht): die Anfrage bleibt
	// offen -- die anderen legen genau einen Wert offen, nicht mehr.
	w.uhr += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", w.uhr), w.uhr+1)
	w.uhr += 2
	w.takte()
	w.finalisieren()
	n := 0
	for _, v := range w.dienste[:3] {
		n += v.takt(w.uhr)
	}
	if n != 3 || w.anfrage(w.cs).Basen != nil {
		t.Fatalf("%d Werte, Aufforderung steht: %v", n, w.anfrage(w.cs).Basen != nil)
	}
	for i := 0; i < 3; i++ {
		w.uhr += 5
		for _, v := range w.dienste[:3] {
			if v.takt(w.uhr) != 0 {
				t.Fatal("ein Validator reicht seinen Zufall zweimal ein")
			}
		}
	}
	note := w.cs.quantenWJetzt().Noten[w.p.K].note
	for x := 0; x < 3; x++ {
		if note.ZIndex[x] != 0 {
			t.Fatalf("Validator %d steht bei Stelle %d seiner Kette", x+1, note.ZIndex[x])
		}
	}
}

// Sammeln: was ein anderer Validator liefert, wird nicht geglaubt; zu wenige
// erreichbare ergeben keine Offenlegung.
func TestQuantenWValidator_SammelnMisstrautAnderen(t *testing.T) {
	bis := func(t *testing.T) *qwValWelt {
		w := neueQwValWelt(t, 200)
		w.praegen(10_000_000)
		w.uhr += 10
		w.annehmen(w.anfrageTx(w.anna, 0, 16, "", w.uhr), w.uhr+1)
		w.uhr += 2
		w.takte()
		w.finalisieren()
		w.takte()
		a := w.anfrage(w.cs)
		w.uhr++
		w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), w.uhr)
		w.uhr++
		w.takte()
		w.finalisieren()
		// Bei allen reif, bevor einer sammelt: sonst bekaeme der erste von
		// den anderen "noch nicht final".
		for _, v := range w.dienste {
			v.anteileFuer(w.p.K, 0, w.uhr)
		}
		return w
	}
	t.Run("ein Validator luegt", func(t *testing.T) {
		w := bis(t)
		w.luege[2] = true
		if n := w.dienste[0].takt(w.uhr); n != 1 {
			t.Fatalf("mit einem luegenden Validator keine Offenlegung (%d)", n)
		}
		if w.cs.quantenWJetzt().Noten[w.p.K].note.Anfrage != nil || w.stand(w.cs, w.anna) != 100_000_000 {
			t.Fatal("die Pruefung der echten Note ist nicht als echt geendet")
		}
	})
	t.Run("zwei luegen, drei ehrliche reichen", func(t *testing.T) {
		w := bis(t)
		w.luege[2], w.luege[3] = true, true
		if n := w.dienste[0].takt(w.uhr); n != 1 {
			t.Fatalf("mit drei ehrlichen von fuenf (t = 3) keine Offenlegung (%d)", n)
		}
	})
	t.Run("drei luegen: nichts wird eingereicht", func(t *testing.T) {
		w := bis(t)
		w.luege[2], w.luege[3], w.luege[4] = true, true, true
		vorher := len(w.eingereicht)
		if n := w.dienste[0].takt(w.uhr); n != 0 || len(w.eingereicht) != vorher {
			t.Fatal("eine Offenlegung mit falschen Anteilen wurde eingereicht")
		}
		if w.anfrage(w.cs) == nil {
			t.Fatal("die Anfrage ist beendet")
		}
	})
	t.Run("zu wenige erreichbar", func(t *testing.T) {
		w := bis(t)
		w.stumm[2], w.stumm[3], w.stumm[4], w.stumm[5] = true, true, true, true
		if n := w.dienste[0].takt(w.uhr); n != 0 {
			t.Fatal("Offenlegung mit einem von drei noetigen Validatoren")
		}
		// Einer kommt zurueck: immer noch zu wenige. Zwei: genug.
		w.stumm[2] = false
		if n := w.dienste[0].takt(w.uhr); n != 0 {
			t.Fatal("Offenlegung mit zwei von drei noetigen Validatoren")
		}
		w.stumm[3] = false
		if n := w.dienste[0].takt(w.uhr); n != 1 {
			t.Fatal("mit drei Validatoren keine Offenlegung")
		}
	})
	t.Run("Staffel nach Rang", func(t *testing.T) {
		w := bis(t)
		// Rang 3 wartet zwei Staffeln, wenn Rang 1 und 2 ausfallen.
		v := w.dienste[2]
		if n := v.takt(w.uhr + 2*quantenWValStaffel - 1); n != 0 {
			t.Fatal("Rang 3 reicht vor seiner Staffel ein")
		}
		w.uhr += 2 * quantenWValStaffel
		if n := v.takt(w.uhr); n != 1 {
			t.Fatal("Rang 3 reicht nach seiner Staffel nicht ein")
		}
	})
}

// Frist: bleibt die Antwort aus, beendet ein Dienst die Anfrage -- erst nach
// der Frist.
func TestQuantenWValidator_Frist(t *testing.T) {
	w := neueQwValWelt(t, 200)
	w.praegen(10_000_000)
	w.uhr += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", w.uhr), w.uhr+1)
	w.uhr += 2
	w.takte()
	w.finalisieren()
	w.takte()
	a := w.anfrage(w.cs)
	w.uhr = a.Gezogen + quantenWFristAntwort
	if n := w.takte(); n != 0 {
		t.Fatal("Frist vor ihrem Ablauf eingereicht")
	}
	w.uhr++
	if n := w.dienste[0].takt(w.uhr); n != 1 || w.anfrage(w.cs) != nil {
		t.Fatalf("Frist nach ihrem Ablauf: %d", n)
	}
	note := w.cs.quantenWJetzt().Noten[w.p.K].note
	if note.Fehl != 1 || w.stand(w.cs, w.anna) != 99_000_000 {
		t.Fatalf("ohne Antwort: Fehl %d, anna %d", note.Fehl, w.stand(w.cs, w.anna))
	}
}

// Tresor: das Siegel gibt Anteile und Geheimnis nur mit dem Schluessel und
// nur fuer dieses K her.
func TestQuantenWValidator_Siegel(t *testing.T) {
	w := neueQwValWelt(t, 64)
	w.praegen(10_000_000)
	v := w.dienste[0]
	e := v.tresor[w.p.K]
	siegel, err := v.tresorVersiegeln(e)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < e.N; i++ {
		var b [16]byte
		e.Anteile[i].FillBytes(b[:])
		if strings.Contains(string(siegel), string(b[:])) {
			t.Fatal("ein Anteil steht im Klartext im Siegel")
		}
	}
	if strings.Contains(string(siegel), string(e.Geheim[:])) {
		t.Fatal("das Geheimnis der Hash-Kette steht im Klartext im Siegel")
	}
	offen := &quantenWTresorEintrag{K: e.K, N: e.N}
	if err := v.tresorOeffnen(offen, siegel); err != nil {
		t.Fatal(err)
	}
	if offen.Geheim != e.Geheim || offen.Anteile[7].Cmp(e.Anteile[7]) != 0 {
		t.Fatal("geoeffnet ist nicht, was versiegelt wurde")
	}
	if err := w.dienste[1].tresorOeffnen(&quantenWTresorEintrag{K: e.K, N: e.N}, siegel); err == nil {
		t.Fatal("ein anderer Schluessel oeffnet das Siegel")
	}
	if err := v.tresorOeffnen(&quantenWTresorEintrag{K: strings.Repeat("1", 64), N: e.N}, siegel); err == nil {
		t.Fatal("das Siegel laesst sich unter ein anderes K legen")
	}
	kaputt := append([]byte(nil), siegel...)
	kaputt[len(kaputt)-1] ^= 1
	if err := v.tresorOeffnen(&quantenWTresorEintrag{K: e.K, N: e.N}, kaputt); err == nil {
		t.Fatal("ein veraendertes Siegel oeffnet")
	}
	if err := v.tresorOeffnen(&quantenWTresorEintrag{K: e.K, N: e.N}, siegel[:5]); err == nil {
		t.Fatal("ein abgeschnittenes Siegel oeffnet")
	}
}

// Schlafend: vor dem Stichtag tut der Takt nichts, und die Endpunkte
// antworten 409 -- auch mit laufendem Dienst.
func TestQuantenWValidator_Schlafend(t *testing.T) {
	w := neueQwValWelt(t, 200)
	w.praegen(10_000_000)
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", w.uhr+5), w.uhr+6)
	w.cs.quantenWVal.Store(w.dienste[0])
	a := &APIServer{state: w.cs}
	quantenWAbOverride.Store(0) // die Konstante gilt: MaxInt64
	w.finalisieren()
	if w.takte() != 0 || w.anfrage(w.cs).Basen != nil {
		t.Fatal("der Takt arbeitet vor dem Stichtag")
	}
	rec := httptest.NewRecorder()
	a.handleQuantenWHinterlegen(rec, httptest.NewRequest(http.MethodPost, "/api/quanten-w/validator/hinterlegen", strings.NewReader(`{}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("hinterlegen vor dem Stichtag: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.handleQuantenWAnteile(rec, httptest.NewRequest(http.MethodGet, "/api/quanten-w/validator/anteile?k="+w.p.K+"&index=0", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("anteile vor dem Stichtag: %d", rec.Code)
	}
}

// Endpunkte: Form, Groesse, Methode, Knoten ohne Dienst.
func TestQuantenWValidator_Endpunkte(t *testing.T) {
	w := neueQwValWelt(t, 64)
	quantenWAbOverride.Store(1) // jetzt aktiv, nach der echten Uhr
	ohne := &APIServer{state: newTestState()}
	rec := httptest.NewRecorder()
	ohne.handleQuantenWHinterlegen(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Knoten ohne Dienst: %d", rec.Code)
	}
	w.cs.quantenWVal.Store(w.dienste[0])
	a := &APIServer{state: w.cs}
	post := func(body string) int {
		rec := httptest.NewRecorder()
		a.handleQuantenWHinterlegen(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)))
		return rec.Code
	}
	if c := post(`{"unbekannt":1}`); c != http.StatusBadRequest {
		t.Fatalf("unbekanntes Feld: %d", c)
	}
	if c := post(`{"wallet":"x"}`); c != http.StatusBadRequest {
		t.Fatalf("leere Hinterlegung: %d", c)
	}
	if c := post(`{"k":"` + strings.Repeat("a", quantenWHinterlegenMax) + `"}`); c != http.StatusRequestEntityTooLarge && c != http.StatusBadRequest {
		t.Fatalf("zu grosser Koerper: %d", c)
	}
	rec = httptest.NewRecorder()
	a.handleQuantenWHinterlegen(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET auf hinterlegen: %d", rec.Code)
	}
	get := func(q string) int {
		rec := httptest.NewRecorder()
		a.handleQuantenWAnteile(rec, httptest.NewRequest(http.MethodGet, "/x?"+q, nil))
		return rec.Code
	}
	for _, q := range []string{"k=abc&index=0", "k=" + w.p.K + "&index=-1", "k=" + w.p.K + "&index=x", "k=" + w.p.K + "&index=256", "k=" + strings.ToUpper(w.p.K) + "&index=0"} {
		want := http.StatusBadRequest
		if strings.HasPrefix(q, "k="+strings.ToUpper(w.p.K)) {
			want = http.StatusNotFound // gross geschrieben wird klein gelesen: unbekannte Note
		}
		if c := get(q); c != want {
			t.Fatalf("anteile?%s: %d, erwartet %d", q, c, want)
		}
	}
}

// Der Tresor in der echten Datenbank: er uebersteht den Neustart (dieselbe
// Hinterlegung ergibt danach dieselbe Bestaetigung -- derselbe Anker, also
// dasselbe Geheimnis), liegt nicht im Klartext, oeffnet sich keinem anderen
// Schluessel, und die Datenbank selbst weist einen zweiten Eintrag fuer
// dasselbe K ab.
func TestQuantenWTresor_UeberlebtNeustart_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := testKnoten(t, "unused-quanten-w-tresor-realdb-test.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank -- DATABASE_URL pruefen")
	}
	if _, err := cs.db.Exec(`DELETE FROM quanten_w_tresor`); err != nil {
		t.Fatal(err)
	}
	quantenWAbOverride.Store(qwT0 - 86400)
	t.Cleanup(func() { quantenWAbOverride.Store(0) })
	k1, a1 := neuerSchluessel(t)
	k2, _ := neuerSchluessel(t)
	kp, praeger := neuerSchluessel(t)
	cs.mu.Lock()
	addHuman(cs, praeger, 100)
	cs.mu.Unlock()
	val := []string{a1}
	for i := 0; i < 4; i++ {
		_, a := neuerSchluessel(t)
		val = append(val, a)
	}
	p := qwPraegen(64, 3, 5, 4242)
	h := &QuantenWHinterlegung{Wallet: praeger, Zeit: qwT0, K: p.K, N: p.n, T: p.t, Wurzel: hex.EncodeToString(p.wurzel[:]), Validatoren: val}
	anteile := make([]*big.Int, p.n)
	for i := 0; i < p.n; i++ {
		anteile[i] = p.anteile[i][0]
		h.Anteile = append(h.Anteile, anteile[i].Text(16))
		h.Blaetter = append(h.Blaetter, hex.EncodeToString(p.blaetter[i][:]))
	}
	h.WalletSig = personalSign(t, kp, quantenWHinterlegenNachricht(praeger, p.K, p.n, p.t, h.Wurzel, val, 1, quantenWInhalt(anteile, p.blaetter), qwT0))

	v1, err := neuerQuantenWValidator(cs, k1)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := v1.hinterlegen(h, qwT0)
	if err != nil {
		t.Fatal(err)
	}
	var siegel []byte
	if err := cs.db.QueryRow(`SELECT siegel FROM quanten_w_tresor WHERE k = $1`, p.K).Scan(&siegel); err != nil {
		t.Fatalf("keine Zeile im Tresor: %v", err)
	}
	for i := 0; i < p.n; i++ {
		var b [16]byte
		anteile[i].FillBytes(b[:])
		if strings.Contains(string(siegel), string(b[:])) {
			t.Fatal("ein Anteil steht im Klartext in der Datenbank")
		}
	}
	// Neustart: derselbe Schluessel, neue Instanz.
	v2, err := neuerQuantenWValidator(cs, k1)
	if err != nil {
		t.Fatal(err)
	}
	e := v2.tresor[p.K]
	if e == nil || e.Geheim != v1.tresor[p.K].Geheim || e.Anteile[9].Cmp(anteile[9]) != 0 || e.Blaetter[9] != p.blaetter[9] || e.X != 1 || e.Praeger != praeger {
		t.Fatal("nach dem Neustart fehlt der Eintrag oder er ist ein anderer")
	}
	b2, err := v2.hinterlegen(h, qwT0+3)
	if err != nil || *b2 != *b1 {
		t.Fatalf("nach dem Neustart eine andere Bestaetigung: %v", err)
	}
	// Ein anderer Schluessel auf derselben Datenbank: kein Fehler, aber
	// auch kein Eintrag -- er kann das Siegel nicht oeffnen.
	v3, err := neuerQuantenWValidator(cs, k2)
	if err != nil {
		t.Fatal(err)
	}
	if len(v3.tresor) != 0 {
		t.Fatal("ein anderer Schluessel liest den Tresor")
	}
	// Die Datenbank weist einen zweiten Eintrag fuer dasselbe K ab.
	if err := v1.tresorSpeichern(v1.tresor[p.K]); err == nil {
		t.Fatal("zweite Zeile fuer dasselbe K geschrieben")
	}
	v2.mu.Lock()
	v2.tresorLoeschenLocked(p.K, "Test")
	v2.mu.Unlock()
	var zeilen int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM quanten_w_tresor`).Scan(&zeilen); err != nil || zeilen != 0 {
		t.Fatalf("nach dem Loeschen %d Zeilen (%v)", zeilen, err)
	}
}
