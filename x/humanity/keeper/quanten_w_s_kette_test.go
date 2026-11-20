package keeper

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// Variante S in der Kette (quanten_w_s_kette.go): wie die W-Tests ein
// annehmender und ein nachspielender Knoten im Gleichschritt.

type qsWelt struct {
	*qwWelt
	stelle    [2]string
	probe     [2][]int
	messbasen []int // Basen, in denen die Inhaberin gemessen hat
	c         int
	x         []int // ihre Vorlage (Messwerte)
}

const qsTol = 4

func neueQsWelt(t *testing.T, n, probeLaenge int) *qsWelt {
	t.Helper()
	w := &qsWelt{qwWelt: neueQwWelt(t, n)}
	w.vorbereiten(probeLaenge)
	return w
}

// vorbereiten: Annahmestellen, Stichproben und die Messung der Inhaberin.
func (w *qsWelt) vorbereiten(probeLaenge int) {
	t, n := w.t, w.p.n
	for b := 0; b < 2; b++ {
		k, a := neuerSchluessel(t)
		w.stelle[b], w.keys[a] = a, k
	}
	r := rand.New(rand.NewSource(int64(n)*7 + int64(probeLaenge)))
	for b := 0; b < 2; b++ {
		w.probe[b] = r.Perm(n)[:probeLaenge]
		sort.Ints(w.probe[b])
	}
	w.c = 1
	for i := 0; i < n; i++ {
		m := r.Intn(2)
		w.messbasen = append(w.messbasen, m)
		if m == w.p.basen[i] {
			w.x = append(w.x, w.p.bits[i])
		} else {
			w.x = append(w.x, r.Intn(2))
		}
	}
}

func qsBits(b []int) string {
	var sb strings.Builder
	for _, v := range b {
		sb.WriteByte(byte('0' + v))
	}
	return sb.String()
}

func (w *qsWelt) probenHex() []string {
	var out []string
	for b := 0; b < 2; b++ {
		h := quantenSProbeHash(w.p.K, b, w.probe[b])
		out = append(out, hex.EncodeToString(h[:]))
	}
	return out
}

func (w *qsWelt) ausgebenTx(zeit, deckung int64) Transaction {
	p := w.p
	wurzel := hex.EncodeToString(p.wurzel[:])
	var proben [2][32]byte
	for b := 0; b < 2; b++ {
		proben[b] = quantenSProbeHash(p.K, b, w.probe[b])
	}
	q := &QuantenWAngaben{K: p.K, N: p.n, T: p.t, Wurzel: wurzel, Validatoren: w.val, Deckung: deckung, Tol: qsTol,
		Inhaber: w.anna, Annahme: []string{w.stelle[0], w.stelle[1]}, Proben: w.probenHex()}
	for x := range w.val {
		q.Bestaetigungen = append(q.Bestaetigungen, personalSign(w.t, w.valKeys[x], quantenWSBestaetigungNachricht(p.K, p.wurzel, p.n, p.t, w.val, x+1, proben, zeit, qsTol)))
	}
	q.WalletSig = personalSign(w.t, w.keys[w.praeger], quantenWSAusgabeNachricht(w.praeger, p.K, p.n, p.t, wurzel, w.val, q.Proben, q.Annahme, w.anna, deckung, zeit, qsTol))
	return Transaction{Type: quantenWSAusgebenTxArt, Wallet: w.praeger, DistributionAt: zeit, QuantenW: q}
}

// d = Messbasen XOR c: an Stelle 0 prueft die Kette d^0^c = die Messbasen.
func (w *qsWelt) bindenTx(von string) Transaction {
	d := make([]int, len(w.messbasen))
	for i, m := range w.messbasen {
		d[i] = m ^ w.c
	}
	s := qsBits(d)
	return Transaction{Type: quantenWSBindenTxArt, Wallet: von, QuantenW: &QuantenWAngaben{K: w.p.K, D: s,
		WalletSig: personalSign(w.t, w.keys[von], quantenWSBindenNachricht(von, w.p.K, s))}}
}

func (w *qsWelt) waehlenTx(von, c string) Transaction {
	return Transaction{Type: quantenWSWaehlenTxArt, Wallet: von, QuantenW: &QuantenWAngaben{K: w.p.K, C: c,
		WalletSig: personalSign(w.t, w.keys[von], quantenWSWahlNachricht(von, w.p.K, c))}}
}

func (w *qsWelt) vorlage(b int, unterschreiber *ecdsa.PrivateKey) string {
	return personalSign(w.t, unterschreiber, quantenSVorlageText(w.p.K, b, w.x))
}

func (w *qsWelt) anmeldenTx(von string, b int, probe []int, vorlage string) Transaction {
	bs, xs := string(rune('0'+b)), qsBits(w.x)
	return Transaction{Type: quantenWSAnmeldenTxArt, Wallet: von, QuantenW: &QuantenWAngaben{K: w.p.K, B: bs, X: xs, Probe: probe, Vorlage: vorlage,
		WalletSig: personalSign(w.t, w.keys[von], quantenWSAnmeldenNachricht(von, w.p.K, bs, xs, probe, w.fehl()))}}
}

// fehl: bisherige Fehlversuche des Tokens (die Stelle unterschreibt sie mit).
func (w *qsWelt) fehl() int {
	if t := w.token(w.cs); t != nil {
		return t.Fehl
	}
	return 0
}

func (w *qsWelt) offenTx(ab int, stellen []int) Transaction {
	q := &QuantenWAngaben{K: w.p.K, Ab: ab}
	for _, o := range w.p.offen(stellen, 1, 3, 5) {
		var of QuantenWOffen
		for _, a := range o.Anteile {
			of.Anteile = append(of.Anteile, QuantenWAnteilAngabe{X: a.X, Y: a.Y.Text(16)})
		}
		for _, h := range o.Pfad {
			of.Pfad = append(of.Pfad, hex.EncodeToString(h[:]))
		}
		q.Offen = append(q.Offen, of)
	}
	return Transaction{Type: quantenWSOffenlegenTxArt, QuantenW: q}
}

// offenlegenAlle: die ganze Stichprobe der Stelle b in Teilen; das letzte
// Ergebnis (gerechnete Felder).
func (w *qsWelt) offenlegenAlle(b int, zeit int64) QuantenWAngaben {
	w.t.Helper()
	var g QuantenWAngaben
	for ab := 0; ab < len(w.probe[b]); ab += quantenWOffenTeil {
		bis := min(ab+quantenWOffenTeil, len(w.probe[b]))
		tx := w.annehmen(w.offenTx(ab, w.probe[b][ab:bis]), zeit)
		g = tx.QuantenW.gerechnet()
		if bis < len(w.probe[b]) && g.Urteil != "teil" {
			w.t.Fatalf("Teil %d: %+v", ab, g)
		}
	}
	return g
}

func (w *qsWelt) bereit(z int64) {
	w.t.Helper()
	w.annehmen(w.ausgebenTx(z, 10_000_000), z+1)
	w.annehmen(w.bindenTx(w.anna), z+2)
	w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+3)
}

func (w *qsWelt) token(cs *ChainState) *quantenSToken {
	if e := cs.quantenWJetzt().Token[w.p.K]; e != nil {
		return e.token
	}
	return nil
}

// Gutfall: ausgeben, binden, waehlen, an Stelle 0 anmelden, in zwei Teilen
// offenlegen -> Wert an die Stelle; Token weg, gemerkt, Erhaltung.
func TestQuantenWSKette_Gutfall(t *testing.T) {
	w := neueQsWelt(t, 128, 80)
	summe0 := w.summe(w.cs)
	z := qwT0
	tx := w.annehmen(w.ausgebenTx(z, 10_000_000), z+1)
	if tx.QuantenW.Anteile != 10_000_000 || w.stand(w.cs, quantenWTopfAddr) != 10_000_000 || w.stand(w.cs, w.praeger) != 90_000_000 {
		t.Fatalf("Ausgabe: %+v", tx.QuantenW.gerechnet())
	}
	w.annehmen(w.bindenTx(w.anna), z+2)
	w.annehmen(w.waehlenTx(w.anna, "1"), z+3)
	w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.anna])), z+10)
	g := w.offenlegenAlle(0, z+20)
	if g.Urteil != "echt" || g.Wert != 10_000_000 || g.WertAn != w.stelle[0] || w.stand(w.cs, w.stelle[0]) != 10_000_000 || w.stand(w.cs, quantenWTopfAddr) != 0 {
		t.Fatalf("Einloesen: %+v", g)
	}
	reg := w.cs.quantenWJetzt()
	if len(reg.Token) != 0 || reg.Tote[w.p.K] != z {
		t.Fatal("eingeloester Token nicht weg oder nicht gemerkt")
	}
	if w.summe(w.cs)+w.stand(w.cs, w.stelle[0]) != summe0 {
		t.Fatal("Erhaltung")
	}
	// Dieselbe Vorlage an der zweiten Stelle: der Token ist weg.
	if u := w.bescheid(w.cs, w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna])), z+30); u.regel != "quanten_w_unbekannt" {
		t.Fatalf("zweite Stelle nach dem Einloesen: %+v", u)
	}
	if u := w.bescheid(w.cs, w.ausgebenTx(z+40, 10_000_000), z+41); u.regel != "quanten_w_doppelt" {
		t.Fatalf("eingeloesten Token neu ausgegeben: %+v", u)
	}
}

// Doppelvorlage: dieselbe Vorlage an der anderen Stelle besteht nicht (die
// Kette prueft dort die anderen Basen). Nach quantenWFehlMax falschen
// Vorlagen ist der Token tot, der Wert geht ans Grundeinkommen.
func TestQuantenWSKette_DoppelvorlageUndTot(t *testing.T) {
	w := neueQsWelt(t, 128, 40)
	z := qwT0
	w.bereit(z)
	ge0 := w.stand(w.cs, ubiPoolAddr)
	// Ein Dritter reicht eine fehlgeschlagene Anmeldung unveraendert wieder
	// ein: die Unterschrift gilt nur fuer den alten Stand der Fehlversuche.
	alt := w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna]))
	w.annehmen(alt, z+50)
	if g := w.offenlegenAlle(1, z+55); g.Urteil != "abgewiesen" {
		t.Fatalf("Doppelvorlage bestanden: %+v", g)
	}
	if u := w.bescheid(w.cs, alt, z+60); u.regel != "quanten_w_unterschrift" {
		t.Fatalf("wiederholte Anmeldung: %+v", u)
	}
	for i := 1; i < quantenWFehlMax; i++ {
		zi := z + 100*int64(i+1)
		w.annehmen(w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna])), zi)
		g := w.offenlegenAlle(1, zi+5)
		if g.Urteil != "abgewiesen" {
			t.Fatalf("Doppelvorlage %d bestanden: %+v", i, g)
		}
		tok := w.token(w.cs)
		if i < quantenWFehlMax-1 && (tok == nil || tok.Fehl != i+1 || g.Wert != 0) { // i = 1 .. FehlMax-1
			t.Fatalf("nach %d Fehlversuchen: %+v", i+1, g)
		}
		if i == quantenWFehlMax-1 && (tok != nil || g.Wert != 10_000_000 || g.WertAn != "" || w.stand(w.cs, ubiPoolAddr) != ge0+10_000_000) {
			t.Fatalf("tot: Wert nicht ans Grundeinkommen: %+v", g)
		}
	}
}

func TestQuantenWSKette_AusgebenMissbrauch(t *testing.T) {
	w := neueQsWelt(t, 64, 24)
	z := qwT0
	for name, f := range map[string]struct {
		tx    func() Transaction
		regel string
	}{
		"Bestaetigung fehlt": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Bestaetigungen[2] = tx.QuantenW.Bestaetigungen[1]
			return tx
		}, "quanten_w_bestaetigung"},
		"Stichprobe ausgetauscht": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Proben[0] = strings.Repeat("ab", 32)
			return tx
		}, "quanten_w_unterschrift"},
		"Stelle ausgetauscht": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Annahme[1] = w.bert
			return tx
		}, "quanten_w_unterschrift"},
		"zweimal dieselbe Stelle": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Annahme[1] = tx.QuantenW.Annahme[0]
			return tx
		}, "quanten_w_form"},
		"Inhaberin als Stelle": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Annahme[0] = w.anna
			return tx
		}, "quanten_w_form"},
		"Topf als Stelle": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Annahme[0] = quantenWTopfAddr
			return tx
		}, "quanten_w_form"},
		"eine Stichprobe": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Proben = tx.QuantenW.Proben[:1]
			return tx
		}, "quanten_w_form"},
		"Toleranz ausserhalb der Tabelle": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Tol = 7
			return tx
		}, "quanten_w_form"},
		"Deckung unter 1 AEQ":    {func() Transaction { return w.ausgebenTx(z, 999_999) }, "quanten_w_deckung"},
		"Deckung ueber Guthaben": {func() Transaction { return w.ausgebenTx(z, 200_000_000) }, "quanten_w_deckung"},
		"Zeit zu alt":            {func() Transaction { return w.ausgebenTx(z-quantenWAlt-10, 10_000_000) }, "quanten_w_zeit"},
		"Nummer gesetzt": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Index = 1
			return tx
		}, "quanten_w_form"},
		"fremde W-Angabe": {func() Transaction {
			tx := w.ausgebenTx(z, 10_000_000)
			tx.QuantenW.Anker = []string{strings.Repeat("ab", 32)}
			return tx
		}, "quanten_w_form"},
	} {
		if u := w.bescheid(w.cs, f.tx(), z+1); u.regel != f.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, f.regel)
		}
	}
	// Validator ohne Bindung: wie bei W.
	op := w.betreiber[w.val[3]]
	delete(w.betreiber, w.val[3])
	if u := w.bescheid(w.cs, w.ausgebenTx(z, 10_000_000), z+1); u.regel != "quanten_w_validator" {
		t.Fatalf("ungebundener Validator: %+v", u)
	}
	w.betreiber[w.val[3]] = op
	w.annehmen(w.ausgebenTx(z, 10_000_000), z+1)
	if u := w.bescheid(w.cs, w.ausgebenTx(z, 10_000_000), z+2); u.regel != "quanten_w_doppelt" {
		t.Fatalf("doppelt: %+v", u)
	}
	// Dieselbe K als W-Note: belegt.
	if u := w.bescheid(w.cs, w.praegenTx(z, 10_000_000), z+2); u.regel != "quanten_w_doppelt" {
		t.Fatalf("K als W-Note: %+v", u)
	}
}

func TestQuantenWSKette_AblaufMissbrauch(t *testing.T) {
	w := neueQsWelt(t, 64, 24)
	z := qwT0
	w.annehmen(w.ausgebenTx(z, 10_000_000), z+1)
	vorlage := w.vorlage(0, w.keys[w.anna])
	for name, f := range map[string]struct {
		tx    Transaction
		regel string
	}{
		"Bindung durch Fremde":   {w.bindenTx(w.bert), "quanten_w_regel"},
		"Wahl vor der Bindung":   {w.waehlenTx(w.anna, "1"), "quanten_w_regel"},
		"Anmelden ungebunden":    {w.anmeldenTx(w.stelle[0], 0, w.probe[0], vorlage), "quanten_w_regel"},
		"Bindung fremd signiert": {func() Transaction { tx := w.bindenTx(w.anna); tx.Wallet = w.bert; return tx }(), "quanten_w_unterschrift"},
		"Bindung zu kurz": {func() Transaction {
			tx := w.bindenTx(w.anna)
			tx.QuantenW.D = tx.QuantenW.D[1:]
			tx.QuantenW.WalletSig = personalSign(t, w.keys[w.anna], quantenWSBindenNachricht(w.anna, w.p.K, tx.QuantenW.D))
			return tx
		}(), "quanten_w_form"},
	} {
		if u := w.bescheid(w.cs, f.tx, z+5); u.regel != f.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, f.regel)
		}
	}
	w.annehmen(w.bindenTx(w.anna), z+6)
	if u := w.bescheid(w.cs, w.bindenTx(w.anna), z+7); u.regel != "quanten_w_regel" {
		t.Fatalf("zweite Bindung: %+v", u)
	}
	if u := w.bescheid(w.cs, w.waehlenTx(w.anna, "2"), z+7); u.regel != "quanten_w_form" {
		t.Fatalf("Wahl 2: %+v", u)
	}
	w.annehmen(w.waehlenTx(w.anna, "1"), z+8)
	if u := w.bescheid(w.cs, w.waehlenTx(w.anna, "0"), z+9); u.regel != "quanten_w_regel" {
		t.Fatalf("zweite Wahl: %+v", u)
	}
	for name, f := range map[string]struct {
		tx    Transaction
		regel string
	}{
		// Vorauslaufen: ein Dritter kopiert Vorlage und Stichprobe der Stelle 0.
		"Dritter meldet mit kopierter Stichprobe an": {w.anmeldenTx(w.bert, 0, w.probe[0], vorlage), "quanten_w_s_stelle"},
		"Stelle 1 meldet als Stelle 0 an":            {w.anmeldenTx(w.stelle[1], 0, w.probe[0], vorlage), "quanten_w_s_stelle"},
		"Vorlage nicht von der Inhaberin":            {w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.bert])), "quanten_w_s_vorlage"},
		"Vorlage einer anderen Stelle":               {w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(1, w.keys[w.anna])), "quanten_w_s_vorlage"},
		"Stichprobe der anderen Stelle":              {w.anmeldenTx(w.stelle[0], 0, w.probe[1], vorlage), "quanten_w_regel"},
		"Stichprobe gekuerzt":                        {w.anmeldenTx(w.stelle[0], 0, w.probe[0][1:], vorlage), "quanten_w_regel"},
		"Stelle 2": {func() Transaction {
			tx := w.anmeldenTx(w.stelle[0], 0, w.probe[0], vorlage)
			tx.QuantenW.B = "2"
			return tx
		}(), "quanten_w_form"},
		"Anmeldung fremd signiert": {func() Transaction {
			tx := w.anmeldenTx(w.stelle[0], 0, w.probe[0], vorlage)
			tx.QuantenW.WalletSig = personalSign(t, w.keys[w.stelle[1]], "x")
			return tx
		}(), "quanten_w_unterschrift"},
		"Offenlegen ohne Anmeldung": {w.offenTx(0, w.probe[0][:8]), "quanten_w_form"},
		"Frist ohne Anmeldung":      {Transaction{Type: quantenWSFristTxArt, QuantenW: &QuantenWAngaben{K: w.p.K}}, "quanten_w_frist"},
	} {
		if u := w.bescheid(w.cs, f.tx, z+10); u.regel != f.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, f.regel)
		}
	}
	w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], vorlage), z+10)
	if u := w.bescheid(w.cs, w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna])), z+11); u.regel != "quanten_w_regel" {
		t.Fatalf("zweite offene Anmeldung: %+v", u)
	}
	kaputt := w.offenTx(0, w.probe[0][:8])
	kaputt.QuantenW.Offen[0].Anteile[0].Y = "1"
	mitWallet := w.offenTx(0, w.probe[0][:8])
	mitWallet.Wallet = w.bert
	for name, f := range map[string]struct {
		tx    Transaction
		regel string
	}{
		"falscher Anteil":       {kaputt, "quanten_w_regel"},
		"falscher Beginn":       {w.offenTx(1, w.probe[0][1:9]), "quanten_w_regel"},
		"Offenlegen mit Wallet": {mitWallet, "quanten_w_form"},
		"Frist zu frueh":        {Transaction{Type: quantenWSFristTxArt, QuantenW: &QuantenWAngaben{K: w.p.K}}, "quanten_w_frist"},
	} {
		if u := w.bescheid(w.cs, f.tx, z+12); u.regel != f.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, f.regel)
		}
	}
	// Ein Teil, dann legen die Validatoren nicht weiter offen: Frist ohne
	// Fehlversuch, die Stelle darf neu anmelden.
	w.annehmen(w.offenTx(0, w.probe[0][:8]), z+12)
	tx := w.annehmen(Transaction{Type: quantenWSFristTxArt, QuantenW: &QuantenWAngaben{K: w.p.K}}, z+10+quantenWFristOffen+1)
	if g := tx.QuantenW.gerechnet(); g.Urteil != "frist" || w.token(w.cs).Anmeldung != nil || w.token(w.cs).Fehl != 0 {
		t.Fatalf("Frist: %+v", g)
	}
	w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], vorlage), z+quantenWFristOffen+100)
	// Der Block nennt ein geschoentes Urteil: abgewiesen, nichts angewandt.
	letzt := w.offenTx(0, w.probe[0])
	u := w.bescheid(w.nach, letzt, z+quantenWFristOffen+101)
	if u.regel != "" || u.felder.Urteil != "echt" {
		t.Fatalf("ehrliche Offenlegung: %+v", u)
	}
	falsch := letzt
	q := *letzt.QuantenW
	q.Urteil, q.Wert, q.WertAn = "echt", u.felder.Wert, w.bert
	falsch.QuantenW = &q
	falsch.BuchAt = z + quantenWFristOffen + 101
	if err := w.nachspielen(falsch, falsch.BuchAt); err == nil {
		t.Fatal("Wert an einen Fremden nachgespielt")
	}
	if w.token(w.nach) == nil || w.stand(w.nach, w.bert) != 100_000_000 {
		t.Fatal("abgewiesener Block hat etwas angewandt")
	}
}

// Register: Token hin und zurueck gleich; Faelschungen beim Lesen abgewiesen.
func TestQuantenWSKette_RegisterLesen(t *testing.T) {
	w := neueQsWelt(t, 64, 24)
	z := qwT0
	w.bereit(z)
	w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.anna])), z+10)
	w.annehmen(w.offenTx(0, w.probe[0][:8]), z+11)
	k := w.cs.quantenWJetzt().kodiert()
	r, err := quantenWRegisterLesen(k)
	if err != nil || r.kodiert() != k {
		t.Fatalf("hin und zurueck: %v", err)
	}
	tok := func(m map[string]interface{}) map[string]interface{} {
		return m["token"].([]interface{})[0].(map[string]interface{})
	}
	for name, aendern := range map[string]func(m map[string]interface{}){
		"Anmeldung einer fremden Stelle": func(m map[string]interface{}) {
			tok(m)["anmeldung"].(map[string]interface{})["stelle"] = w.bert
		},
		"Stichprobe passt nicht": func(m map[string]interface{}) {
			// gueltig sortiert und im Bereich, aber die Stichprobe der anderen Stelle
			var andere []interface{}
			for _, v := range w.probe[1] {
				andere = append(andere, float64(v))
			}
			tok(m)["anmeldung"].(map[string]interface{})["probe"] = andere
		},
		"Bindung zu kurz":       func(m map[string]interface{}) { tok(m)["d"] = "00" },
		"Wahl ohne Bindung":     func(m map[string]interface{}) { delete(tok(m), "d") },
		"tot, aber lebendig":    func(m map[string]interface{}) { tok(m)["fehl"] = float64(quantenWFehlMax) },
		"Stellen gleich":        func(m map[string]interface{}) { s := tok(m)["stellen"].([]interface{}); s[1] = s[0] },
		"Teil ueber der Probe":  func(m map[string]interface{}) { tok(m)["anmeldung"].(map[string]interface{})["offen"] = float64(24) },
		"Fehler ueber Treffern": func(m map[string]interface{}) { tok(m)["anmeldung"].(map[string]interface{})["fehler"] = float64(9) },
		"Token auch als tote Note": func(m map[string]interface{}) {
			m["tote"] = []interface{}{map[string]interface{}{"k": w.p.K, "zeit": 1.0}}
		},
	} {
		var m map[string]interface{}
		json.Unmarshal([]byte(k), &m)
		aendern(m)
		b, _ := json.Marshal(m)
		if _, err := quantenWRegisterLesen(string(b)); err == nil {
			t.Errorf("%s: gelesen", name)
		}
	}
	// GET zeigt Token und offene Anmeldung.
	a := &APIServer{state: w.cs}
	rec := httptest.NewRecorder()
	a.handleQuantenWNote(rec, httptest.NewRequest(http.MethodGet, "/api/quanten-w/note?k="+w.p.K, nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"art":"s"`) || !strings.Contains(rec.Body.String(), `"anmeldung"`) {
		t.Fatalf("Ansicht: %d %s", rec.Code, rec.Body.String())
	}
	// Die Ansicht nennt, bei wie vielen Validatoren das Geheimnis liegt
	// (v) und wie viele davon noetig sind (t) -- genau wie im Register.
	var ans struct{ T, V int }
	if err := json.Unmarshal(rec.Body.Bytes(), &ans); err != nil {
		t.Fatal(err)
	}
	tk := w.cs.quantenWJetzt().Token[w.p.K].token
	if ans.V != tk.NVal || ans.T != tk.T || ans.V < ans.T || ans.V < 1 {
		t.Fatalf("Ansicht t=%d v=%d, Register t=%d v=%d", ans.T, ans.V, tk.T, tk.NVal)
	}
	// Ein Token im bisherigen Weg nennt keine Art der Stichprobe.
	if strings.Contains(rec.Body.String(), "probe_art") {
		t.Fatalf("Ansicht eines bisherigen Tokens nennt probe_art: %s", rec.Body.String())
	}
}

// Schlafend: die S-Arten sind vor dem Stichtag unbekannt und im Block verboten.
func TestQuantenWSKette_Schlafend(t *testing.T) {
	// Stichtag eine Stunde in der Zukunft: geprueft wird die Zeit davor,
	// unabhaengig vom Stichtag im Hauptnetz und von der Uhr.
	quantenWAbOverride.Store(nowUnix() + 3600)
	t.Cleanup(func() { quantenWAbOverride.Store(0) })
	jetzt := nowUnix()
	for _, art := range []string{quantenWSAusgebenTxArt, quantenWSBindenTxArt, quantenWSWaehlenTxArt, quantenWSAnmeldenTxArt, quantenWSOffenlegenTxArt, quantenWSFristTxArt} {
		if bekannteTxArt(art, jetzt) || !istQuantenWTxArt(art) {
			t.Errorf("%s vor dem Stichtag bekannt oder nicht im W-Modul", art)
		}
		tx := Transaction{Type: art, QuantenW: &QuantenWAngaben{K: strings.Repeat("a", 64)}}
		if quantenWFeldErlaubt(&tx, jetzt) == nil {
			t.Errorf("%s vor dem Stichtag im Block erlaubt", art)
		}
		a := &APIServer{state: newTestState()}
		rec := httptest.NewRecorder()
		a.handleQuantenW(art)(rec, httptest.NewRequest(http.MethodPost, "/api/quanten-w/x", strings.NewReader(`{}`)))
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: Endpunkt %d statt 409", art, rec.Code)
		}
		if !bekannteTxArt(art, quantenWAb()) {
			t.Errorf("%s am Stichtag unbekannt", art)
		}
	}
	var _ = context.Background
}

// Ueber den echten Annahmeweg (quantenWAnnehmen, mit Kontensperre) bis zur
// Auszahlung an die Stelle; der zweite Knoten spielt jeden Schritt nach.
func TestQuantenWSKette_AnnahmeBisAuszahlung(t *testing.T) {
	w := neueQsWelt(t, 64, 24)
	quantenWAbOverride.Store(1)
	jetzt := nowUnix()
	schritt := func(tx Transaction) QuantenWAngaben {
		t.Helper()
		fertig, err := w.cs.quantenWAnnehmen(tx)
		if err != nil {
			t.Fatalf("%s: %v", tx.Type, err)
		}
		if err := w.nachspielen(fertig, fertig.BuchAt+3); err != nil {
			t.Fatalf("Nachspielen %s: %v", tx.Type, err)
		}
		return fertig.QuantenW.gerechnet()
	}
	schritt(w.ausgebenTx(jetzt, 10_000_000))
	schritt(w.bindenTx(w.anna))
	schritt(w.waehlenTx(w.anna, "1"))
	schritt(w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.anna])))
	g := schritt(w.offenTx(0, w.probe[0]))
	if g.Urteil != "echt" || g.WertAn != w.stelle[0] || w.stand(w.cs, w.stelle[0]) != 10_000_000 || w.stand(w.nach, w.stelle[0]) != 10_000_000 {
		t.Fatalf("Auszahlung ueber die Annahme: %+v", g)
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
		t.Fatal("Register weichen ab")
	}
}
