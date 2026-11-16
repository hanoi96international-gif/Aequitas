package keeper

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Variante W in der Kette (quanten_w_kette.go): ein annehmender und ein
// nachspielender Knoten laufen im Gleichschritt; jeder Missbrauch faellt bei
// beiden auf.

const qwT0 int64 = 1_900_000_000

type qwWelt struct {
	t         *testing.T
	cs, nach  *ChainState
	p         *qwPraegung
	valKeys   []*ecdsa.PrivateKey
	val       []string
	betreiber map[string]string
	keys      map[string]*ecdsa.PrivateKey // Wallet -> Schluessel
	praeger   string
	anna      string
	bert      string
}

func neueQwWelt(t *testing.T, n int) *qwWelt {
	t.Helper()
	quantenWAbOverride.Store(qwT0 - 86400)
	qwLabor(t) // Ablaeufe mit k_note = 64; sichere Groessen: TestQuantenW_SicherePruefgroesse
	alt := quantenWBetreiberVon
	t.Cleanup(func() { quantenWAbOverride.Store(0); quantenWBetreiberVon = alt })
	w := &qwWelt{t: t, cs: newTestState(), nach: newTestState(), p: qwPraegen(n, 3, 5, int64(n)),
		betreiber: map[string]string{}, keys: map[string]*ecdsa.PrivateKey{}}
	quantenWBetreiberVon = func(_ *ChainState, _ context.Context, signing string) (string, error) {
		return w.betreiber[strings.ToLower(signing)], nil
	}
	for x := 0; x < 5; x++ {
		k, a := neuerSchluessel(t)
		_, op := neuerSchluessel(t)
		w.valKeys, w.val = append(w.valKeys, k), append(w.val, a)
		w.betreiber[a] = op
		addHuman(w.cs, op, 0)
		addHuman(w.nach, op, 0)
	}
	for _, name := range []*string{&w.praeger, &w.anna, &w.bert} {
		k, a := neuerSchluessel(t)
		*name, w.keys[a] = a, k
		addHuman(w.cs, a, 100)
		addHuman(w.nach, a, 100)
	}
	return w
}

func (w *qwWelt) stand(cs *ChainState, a string) int64 {
	if acc, ok := cs.accounts.Get(a); ok {
		return acc.Balance.Micro()
	}
	return 0
}

// summe: alle Konten der Welt (Erhaltung).
func (w *qwWelt) summe(cs *ChainState) int64 {
	var s int64
	for _, a := range []string{w.praeger, w.anna, w.bert, quantenWTopfAddr, quantenWPfandAddr, ubiPoolAddr} {
		s += w.stand(cs, a)
	}
	return s
}

func (w *qwWelt) ankerHex() []string {
	var out []string
	for _, a := range w.p.anker {
		out = append(out, hex.EncodeToString(a[:]))
	}
	return out
}

func (w *qwWelt) praegenTx(zeit, deckung int64) Transaction {
	p := w.p
	wurzel := hex.EncodeToString(p.wurzel[:])
	q := &QuantenWAngaben{K: p.K, N: p.n, T: p.t, Wurzel: wurzel, Validatoren: w.val, Anker: w.ankerHex(), Deckung: deckung, Tol: quantenWTolStandard}
	for x := range w.val {
		q.Bestaetigungen = append(q.Bestaetigungen, personalSign(w.t, w.valKeys[x], quantenWBestaetigungNachricht(p.K, p.wurzel, p.n, p.t, w.val, x+1, p.anker[x], zeit, q.Tol)))
	}
	q.WalletSig = personalSign(w.t, w.keys[w.praeger], quantenWPraegenNachricht(w.praeger, p.K, p.n, p.t, wurzel, w.val, q.Anker, deckung, zeit, q.Tol))
	return Transaction{Type: quantenWPraegenTxArt, Wallet: w.praeger, DistributionAt: zeit, QuantenW: q}
}

func (w *qwWelt) anfrageTx(von string, index, k int, an string, zeit int64) Transaction {
	q := &QuantenWAngaben{K: w.p.K, Index: index, Stellen: k, An: an,
		WalletSig: personalSign(w.t, w.keys[von], quantenWAnfrageNachricht(von, w.p.K, index, k, an, zeit))}
	return Transaction{Type: quantenWAnfrageTxArt, Wallet: von, DistributionAt: zeit, QuantenW: q}
}

func (w *qwWelt) zufallTx(index int, wer ...int) Transaction {
	q := &QuantenWAngaben{K: w.p.K, Index: index}
	for x, z := range w.p.zufall(index, wer...) {
		q.Zufall = append(q.Zufall, QuantenWZufall{V: x + 1, Z: hex.EncodeToString(z[:])})
	}
	return Transaction{Type: quantenWZufallTxArt, QuantenW: q}
}

func (w *qwWelt) antwortTx(von string, index int, bits []int) Transaction {
	var sb strings.Builder
	for _, b := range bits {
		sb.WriteByte(byte('0' + b))
	}
	q := &QuantenWAngaben{K: w.p.K, Index: index, Bits: sb.String(),
		WalletSig: personalSign(w.t, w.keys[von], quantenWAntwortNachricht(von, w.p.K, index, sb.String()))}
	return Transaction{Type: quantenWAntwortTxArt, Wallet: von, QuantenW: q}
}

func (w *qwWelt) offenTx(index int, stellen []int) Transaction {
	return w.offenTeilTx(index, 0, stellen)
}

// offenTeilTx: ein Teil der Offenlegung, beginnend bei Stelle ab der Aufforderung.
func (w *qwWelt) offenTeilTx(index, ab int, stellen []int) Transaction {
	q := &QuantenWAngaben{K: w.p.K, Index: index, Ab: ab}
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
	return Transaction{Type: quantenWOffenlegenTxArt, QuantenW: q}
}

func (w *qwWelt) bescheid(cs *ChainState, tx Transaction, zeit int64) quantenWBescheid {
	w.t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	u, err := cs.quantenWBescheidLocked(context.Background(), &tx, zeit)
	if err != nil {
		w.t.Fatal(err)
	}
	return u
}

// annehmen: wie quantenWAnnehmen (gerechnete Felder setzen, anwenden), dann
// spielt der zweite Knoten denselben Block nach.
func (w *qwWelt) annehmen(tx Transaction, zeit int64) Transaction {
	w.t.Helper()
	u := w.bescheid(w.cs, tx, zeit)
	if u.regel != "" {
		w.t.Fatalf("%s abgewiesen: %s: %s", tx.Type, u.regel, u.text)
	}
	q := *tx.QuantenW
	q.Anteile, q.Pfand, q.PfandAn, q.Wert, q.WertAn, q.Urteil = u.felder.Anteile, u.felder.Pfand, u.felder.PfandAn, u.felder.Wert, u.felder.WertAn, u.felder.Urteil
	tx.QuantenW = &q
	tx.BuchAt = zeit
	w.cs.mu.Lock()
	err := w.cs.quantenWAnwendenLocked(context.Background(), u)
	w.cs.mu.Unlock()
	if err != nil {
		w.t.Fatal(err)
	}
	if err := w.nachspielen(tx, zeit+7); err != nil {
		w.t.Fatalf("Nachspielen %s: %v", tx.Type, err)
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() || w.cs.quantenWWurzel() != w.nach.quantenWWurzel() {
		w.t.Fatal("Register der beiden Knoten weichen ab")
	}
	return tx
}

func (w *qwWelt) nachspielen(tx Transaction, zeit int64) error {
	w.nach.mu.Lock()
	defer w.nach.mu.Unlock()
	if err := quantenWFeldErlaubt(&tx, zeit); err != nil {
		return err
	}
	return w.nach.applyQuantenWLocked(context.Background(), &tx, zeit)
}

func (w *qwWelt) anfrage(cs *ChainState) *quantenWAnfrage {
	return cs.quantenWJetzt().Noten[w.p.K].note.Anfrage
}

// Gutfall: praegen, pruefen, einloesen -- Buchungen, Erhaltung, beide
// Knoten gleich; die tote Note laesst sich nicht neu praegen.
func TestQuantenWKette_Gutfall(t *testing.T) {
	w := neueQwWelt(t, 200)
	summe0 := w.summe(w.cs)
	z := qwT0
	tx := w.annehmen(w.praegenTx(z, 10_000_000), z+1)
	if tx.QuantenW.Anteile != 10_000_000 || w.stand(w.cs, quantenWTopfAddr) != 10_000_000 || w.stand(w.cs, w.praeger) != 90_000_000 {
		t.Fatalf("Praegen: Anteile %d, Topf %d", tx.QuantenW.Anteile, w.stand(w.cs, quantenWTopfAddr))
	}
	if w.cs.quantenWWurzel() == "" {
		t.Fatal("Register nicht in der StateRoot")
	}
	// Pruefung durch anna (k = 16): Pfand max(1 AEQ, 10 AEQ * 16 / 200) = 1 AEQ.
	tx = w.annehmen(w.anfrageTx(w.anna, 0, 16, "", z+10), z+11)
	if tx.QuantenW.Pfand != 1_000_000 || w.stand(w.cs, quantenWPfandAddr) != 1_000_000 {
		t.Fatalf("Pfand %d", tx.QuantenW.Pfand)
	}
	w.annehmen(w.zufallTx(0, 0, 1, 2, 3), z+12)
	if w.anfrage(w.cs).Basen != nil {
		t.Fatal("Teil-Zufall hat gezogen")
	}
	w.annehmen(w.zufallTx(0, 4), z+13)
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), z+20)
	tx = w.annehmen(w.offenTx(0, a.Stellen), z+30)
	if tx.QuantenW.Urteil != "echt" || tx.QuantenW.PfandAn != w.anna || w.stand(w.cs, w.anna) != 100_000_000 || w.stand(w.cs, quantenWPfandAddr) != 0 {
		t.Fatalf("Pruefung: %+v", tx.QuantenW.gerechnet())
	}
	// Einloesen durch bert an bert: k = 64, Pfand 10 AEQ * 64 / 200 = 3,2 AEQ.
	tx = w.annehmen(w.anfrageTx(w.bert, 1, qwLK, w.bert, z+100), z+101)
	if tx.QuantenW.Pfand != 3_200_000 {
		t.Fatalf("Pfand beim Einloesen %d", tx.QuantenW.Pfand)
	}
	w.annehmen(w.zufallTx(1), z+102)
	a = w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.bert, 1, w.p.ehrlich(a.Stellen, a.Basen)), z+103)
	tx = w.annehmen(w.offenTx(1, a.Stellen), z+104)
	g := tx.QuantenW.gerechnet()
	if g.Urteil != "echt" || g.Wert != 10_000_000 || g.WertAn != w.bert || g.PfandAn != w.bert || w.stand(w.cs, w.bert) != 110_000_000 || w.stand(w.cs, quantenWTopfAddr) != 0 {
		t.Fatalf("Einloesen: %+v", g)
	}
	reg := w.cs.quantenWJetzt()
	if len(reg.Noten) != 0 || reg.Tote[w.p.K] != z {
		t.Fatal("tote Note nicht gemerkt")
	}
	if w.summe(w.cs) != summe0 || w.summe(w.nach) != summe0 {
		t.Fatalf("Erhaltung: %d / %d, vorher %d", w.summe(w.cs), w.summe(w.nach), summe0)
	}
	// Neu praegen mit denselben Ankern: abgewiesen.
	if u := w.bescheid(w.cs, w.praegenTx(z+200, 10_000_000), z+201); u.regel != "quanten_w_doppelt" {
		t.Fatalf("tote Note neu gepraegt: %+v", u)
	}
	// Nach dem Fenster vergisst das Register die tote Note -- dann gilt aber
	// auch keine alte Bestaetigung mehr.
	if u := w.bescheid(w.cs, w.praegenTx(z, 10_000_000), z+quantenWAlt+quantenWVoraus+10); u.regel != "quanten_w_zeit" {
		t.Fatalf("alte Bestaetigung nach dem Fenster: %+v", u)
	}
}

func TestQuantenWKette_PraegenMissbrauch(t *testing.T) {
	w := neueQwWelt(t, 200)
	z := qwT0
	faelle := map[string]struct {
		tx    func() Transaction
		regel string
		vor   func()
		nach  func()
	}{
		"ungebundener Validator": {tx: func() Transaction { return w.praegenTx(z, 10_000_000) }, regel: "quanten_w_validator",
			vor:  func() { w.betreiber["x"] = w.betreiber[w.val[2]]; delete(w.betreiber, w.val[2]) },
			nach: func() { w.betreiber[w.val[2]] = w.betreiber["x"]; delete(w.betreiber, "x") }},
		"zwei Validatoren, ein Mensch": {tx: func() Transaction { return w.praegenTx(z, 10_000_000) }, regel: "quanten_w_validator",
			vor:  func() { w.betreiber["x"] = w.betreiber[w.val[4]]; w.betreiber[w.val[4]] = w.betreiber[w.val[0]] },
			nach: func() { w.betreiber[w.val[4]] = w.betreiber["x"]; delete(w.betreiber, "x") }},
		"Betreiber kein Mensch": {tx: func() Transaction { return w.praegenTx(z, 10_000_000) }, regel: "quanten_w_validator",
			vor:  func() { w.betreiber["x"] = w.betreiber[w.val[1]]; _, w.betreiber[w.val[1]] = neuerSchluessel(t) },
			nach: func() { w.betreiber[w.val[1]] = w.betreiber["x"]; delete(w.betreiber, "x") }},
		"Bestaetigung eines anderen": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.QuantenW.Bestaetigungen[3] = tx.QuantenW.Bestaetigungen[2]
			return tx
		}, regel: "quanten_w_bestaetigung"},
		"Bestaetigung fuer andere Praegezeit": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.QuantenW.Bestaetigungen[0] = w.praegenTx(z-1, 10_000_000).QuantenW.Bestaetigungen[0]
			return tx
		}, regel: "quanten_w_bestaetigung"},
		"Anker ausgetauscht": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.QuantenW.Anker[1] = strings.Repeat("ab", 32)
			return tx
		}, regel: "quanten_w_unterschrift"},
		"Wallet-Unterschrift fremd": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.QuantenW.WalletSig = personalSign(t, w.keys[w.anna], "x")
			return tx
		}, regel: "quanten_w_unterschrift"},
		"Deckung ueber dem Guthaben": {tx: func() Transaction { return w.praegenTx(z, 200_000_000) }, regel: "quanten_w_deckung"},
		"Deckung unter 1 AEQ":        {tx: func() Transaction { return w.praegenTx(z, 999_999) }, regel: "quanten_w_deckung"},
		"Zeit zu alt":                {tx: func() Transaction { return w.praegenTx(z-quantenWAlt-10, 10_000_000) }, regel: "quanten_w_zeit"},
		"Zeit in der Zukunft":        {tx: func() Transaction { return w.praegenTx(z+quantenWVoraus+10, 10_000_000) }, regel: "quanten_w_zeit"},
		"fremdes Feld (Betrag)": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.Amount = 1
			return tx
		}, regel: "quanten_w_form"},
		"fremdes Feld (Ziel)": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.To = w.anna
			return tx
		}, regel: "quanten_w_form"},
		"Angabe einer anderen Art": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.QuantenW.Bits = "01"
			return tx
		}, regel: "quanten_w_form"},
		"Anteile vorgegeben": {tx: func() Transaction {
			tx := w.praegenTx(z, 10_000_000)
			tx.QuantenW.Pfand = 5
			return tx
		}, regel: "quanten_w_form"},
	}
	for name, f := range faelle {
		if f.vor != nil {
			f.vor()
		}
		if u := w.bescheid(w.cs, f.tx(), z+1); u.regel != f.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, f.regel)
		}
		if f.nach != nil {
			f.nach()
		}
	}
	// Gegenprobe, dann doppelt und voll.
	w.annehmen(w.praegenTx(z, 10_000_000), z+1)
	if u := w.bescheid(w.cs, w.praegenTx(z, 10_000_000), z+2); u.regel != "quanten_w_doppelt" {
		t.Fatalf("doppelt: %+v", u)
	}
	voll := w.cs.quantenWJetzt().kopie()
	for i := 0; len(voll.Noten) < quantenWNotenHoechstens; i++ {
		voll.Noten[strings.Repeat("0", 60)+hex.EncodeToString([]byte{byte(i >> 8), byte(i)})] = voll.Noten[w.p.K]
	}
	w.cs.quantenWReg.Store(neuerQuantenWStand(voll, ""))
	w.p = qwPraegen(200, 3, 5, 99)
	if u := w.bescheid(w.cs, w.praegenTx(z, 10_000_000), z+3); u.regel != "quanten_w_voll" {
		t.Fatalf("voll: %+v", u)
	}
}

func TestQuantenWKette_AblaufMissbrauch(t *testing.T) {
	w := neueQwWelt(t, 200)
	z := qwT0
	w.annehmen(w.praegenTx(z, 10_000_000), z+1)
	for name, f := range map[string]struct {
		tx    Transaction
		regel string
	}{
		"Anfrage fremd unterschrieben": {func() Transaction {
			tx := w.anfrageTx(w.anna, 0, 16, "", z+5)
			tx.Wallet = w.bert
			return tx
		}(), "quanten_w_unterschrift"},
		"Anfrage mit falscher Nummer": {w.anfrageTx(w.anna, 1, 16, "", z+5), "quanten_w_form"},
		"Einloesen an einen Topf":     {w.anfrageTx(w.anna, 0, qwLK, quantenWTopfAddr, z+5), "quanten_w_form"},
		"Einloesen mit k = 8":         {w.anfrageTx(w.anna, 0, 8, w.anna, z+5), "quanten_w_regel"},
		"Pruefung frisst die Reserve": {w.anfrageTx(w.anna, 0, 137, "", z+5), "quanten_w_regel"},
		"Zufall vor der Anfrage":      {w.zufallTx(0), "quanten_w_regel"},
		"Antwort ohne Anfrage":        {w.antwortTx(w.anna, 0, make([]int, 16)), "quanten_w_form"},
		"Frist ohne Anfrage":          {Transaction{Type: quantenWFristTxArt, QuantenW: &QuantenWAngaben{K: w.p.K}}, "quanten_w_form"},
		"unbekannte Note":             {Transaction{Type: quantenWFristTxArt, QuantenW: &QuantenWAngaben{K: strings.Repeat("c", 64)}}, "quanten_w_unbekannt"},
	} {
		if u := w.bescheid(w.cs, f.tx, z+6); u.regel != f.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, f.regel)
		}
	}
	// Pfand nicht gedeckt: bert verschenkt fast alles.
	w.cs.accounts.Set(w.bert, &AccountState{Address: w.bert, Balance: Decimal(999_999), IsHuman: true})
	if u := w.bescheid(w.cs, w.anfrageTx(w.bert, 0, 16, "", z+5), z+6); u.regel != "quanten_w_deckung" {
		t.Fatalf("Pfand ungedeckt: %+v", u)
	}
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", z+10), z+11)
	falsch := w.zufallTx(0, 2)
	falsch.QuantenW.Zufall[0].Z = strings.Repeat("ab", 32)
	doppelt := w.zufallTx(0, 1)
	doppelt.QuantenW.Zufall = append(doppelt.QuantenW.Zufall, doppelt.QuantenW.Zufall[0])
	for name, f := range map[string]struct {
		tx    Transaction
		regel string
	}{
		"falscher Zufall":            {falsch, "quanten_w_regel"},
		"Zufall doppelt":             {doppelt, "quanten_w_form"},
		"Zufall mit Wallet":          {func() Transaction { tx := w.zufallTx(0); tx.Wallet = w.anna; return tx }(), "quanten_w_form"},
		"zweite Anfrage":             {w.anfrageTx(w.bert, 1, 16, "", z+12), "quanten_w_regel"},
		"Antwort vor dem Zufall":     {w.antwortTx(w.anna, 0, make([]int, 16)), "quanten_w_regel"},
		"Frist vor dem Zufall":       {Transaction{Type: quantenWFristTxArt, QuantenW: &QuantenWAngaben{K: w.p.K}}, "quanten_w_frist"},
		"Offenlegen vor der Antwort": {w.offenTx(0, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}), "quanten_w_regel"},
	} {
		if u := w.bescheid(w.cs, f.tx, z+13); u.regel != f.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, f.regel)
		}
	}
	w.annehmen(w.zufallTx(0), z+14)
	a := w.anfrage(w.cs)
	if u := w.bescheid(w.cs, w.antwortTx(w.bert, 0, w.p.ehrlich(a.Stellen, a.Basen)), z+15); u.regel != "quanten_w_regel" {
		t.Fatalf("fremde Antwort: %+v", u)
	}
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), z+15)
	kaputt := w.offenTx(0, a.Stellen)
	kaputt.QuantenW.Offen[0].Anteile[0].Y = "1"
	if u := w.bescheid(w.cs, kaputt, z+16); u.regel != "quanten_w_regel" {
		t.Fatalf("falscher Anteil: %+v", u)
	}
	// Die Validatoren legen nicht offen: Frist -> Pfand zurueck an anna.
	if u := w.bescheid(w.cs, Transaction{Type: quantenWFristTxArt, QuantenW: &QuantenWAngaben{K: w.p.K}}, z+15+quantenWFristOffen); u.regel != "quanten_w_frist" {
		t.Fatalf("Frist zu frueh: %+v", u)
	}
	tx := w.annehmen(Transaction{Type: quantenWFristTxArt, QuantenW: &QuantenWAngaben{K: w.p.K}}, z+15+quantenWFristOffen+1)
	if g := tx.QuantenW.gerechnet(); g.Urteil != "frist" || g.PfandAn != w.anna || g.Pfand != 1_000_000 {
		t.Fatalf("Frist: %+v", g)
	}
	if u := w.bescheid(w.cs, w.offenTx(0, a.Stellen), z+15+quantenWFristOffen+2); u.regel != "quanten_w_form" {
		t.Fatalf("Offenlegen nach der Frist: %+v", u)
	}
}

// Ein Block mit falschen gerechneten Feldern ist falsch, und nichts wird
// angewandt. W-Angaben an anderen Arten und Ueberweisungen an die W-Toepfe
// traegt kein Block.
func TestQuantenWKette_BlockFalsch(t *testing.T) {
	w := neueQwWelt(t, 200)
	z := qwT0
	gut := w.praegenTx(z, 10_000_000)
	u := w.bescheid(w.cs, gut, z+1)
	for name, aendern := range map[string]func(q *QuantenWAngaben){
		"keine Anteile":   func(q *QuantenWAngaben) {},
		"Anteile + 1":     func(q *QuantenWAngaben) { q.Anteile = u.felder.Anteile + 1 },
		"Urteil erfunden": func(q *QuantenWAngaben) { q.Anteile, q.Urteil = u.felder.Anteile, "echt" },
		"Pfand erfunden":  func(q *QuantenWAngaben) { q.Anteile, q.Pfand = u.felder.Anteile, 1 },
	} {
		tx := w.praegenTx(z, 10_000_000)
		aendern(tx.QuantenW)
		if err := w.nachspielen(tx, z+1); err == nil {
			t.Errorf("%s: Block angenommen", name)
		}
		if w.nach.quantenWWurzel() != "" || w.stand(w.nach, quantenWTopfAddr) != 0 {
			t.Fatalf("%s: abgewiesener Block hat etwas angewandt", name)
		}
	}
	w.annehmen(gut, z+1)
	tx := w.annehmen(w.anfrageTx(w.anna, 0, 16, "", z+10), z+11)
	_ = tx
	w.annehmen(w.zufallTx(0), z+12)
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, make([]int, 16)), z+13) // Unsinn
	// Offenlegen: der Erzeuger behauptet "echt" und gibt das Pfand zurueck.
	off := w.offenTx(0, a.Stellen)
	ub := w.bescheid(w.nach, off, z+14)
	if ub.regel != "" {
		t.Fatalf("Offenlegen: %+v", ub)
	}
	falsch := off
	q := *off.QuantenW
	q.Pfand, q.PfandAn, q.Urteil = 1_000_000, w.anna, "echt"
	falsch.QuantenW = &q
	if ub.felder.Urteil != "echt" {
		if err := w.nachspielen(falsch, z+14); err == nil {
			t.Fatal("geschoentes Urteil angenommen")
		}
	}
	// W-Angaben an einer Ueberweisung; Ueberweisungen an die W-Toepfe.
	for name, tx := range map[string]Transaction{
		"W-Angaben an Ueberweisung": {Type: "transfer", Wallet: w.anna, To: w.bert, Amount: 1, QuantenW: &QuantenWAngaben{K: w.p.K}},
		"an den W-Topf":             {Type: "transfer", Wallet: w.anna, To: quantenWTopfAddr, Amount: 1},
		"an den Pfand-Topf":         {Type: "transfer", Wallet: w.anna, To: quantenWPfandAddr, Amount: 1},
		"W-Art ohne Angaben":        {Type: quantenWFristTxArt},
	} {
		if quantenWFeldErlaubt(&tx, z+20) == nil {
			t.Errorf("%s: im Block erlaubt", name)
		}
	}
	for _, to := range []string{quantenWTopfAddr, quantenWPfandAddr} {
		if pruefeTopfNurMitNote(to, Transaction{}) == nil {
			t.Errorf("Ueberweisung an %s angenommen", to)
		}
	}
}

// Schlafend: vor dem Stichtag unbekannt, im Block verboten, 409, und das
// leere Register aendert keine Wurzel.
func TestQuantenWKette_Schlafend(t *testing.T) {
	quantenWAbOverride.Store(0)
	if quantenWAb() != math_MaxInt64() {
		t.Fatal("Stichtag nicht schlafend")
	}
	jetzt := nowUnix()
	for _, art := range []string{quantenWPraegenTxArt, quantenWAnfrageTxArt, quantenWZufallTxArt, quantenWAntwortTxArt, quantenWOffenlegenTxArt, quantenWFristTxArt} {
		if bekannteTxArt(art, jetzt) {
			t.Errorf("%s vor dem Stichtag bekannt", art)
		}
		tx := Transaction{Type: art, QuantenW: &QuantenWAngaben{K: strings.Repeat("a", 64)}}
		if quantenWFeldErlaubt(&tx, jetzt) == nil {
			t.Errorf("%s vor dem Stichtag im Block erlaubt", art)
		}
		a := &APIServer{state: newTestState()}
		r := httptest.NewRequest(http.MethodPost, "/api/quanten-w/x", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		a.handleQuantenW(art)(rec, r)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: Endpunkt %d statt 409", art, rec.Code)
		}
	}
	cs := newTestState()
	if cs.quantenWWurzel() != "" {
		t.Fatal("leeres Register in der StateRoot")
	}
	// Ueberweisungen an die W-Toepfe vor dem Stichtag: Block wie bisher
	// (die Annahme weist sie trotzdem ab, pruefeTopfNurMitNote).
	tx := Transaction{Type: "transfer", Wallet: "0x" + strings.Repeat("1", 40), To: quantenWTopfAddr, Amount: 1}
	if quantenWFeldErlaubt(&tx, jetzt) != nil {
		t.Fatal("Altbloecke mit Ueberweisung an die Adresse werden ungueltig")
	}
}

func math_MaxInt64() int64 { return 1<<63 - 1 }

// Toepfe: der W-Topf zahlt die Umlaufabgabe (Noten sinken anteilig), der
// Pfand-Topf nicht; beide sind Protokoll-Toepfe.
func TestQuantenWKette_Toepfe(t *testing.T) {
	if !istNotenTopf(quantenWTopfAddr) || istNotenTopf(quantenWPfandAddr) || !umlaufZahlt(quantenWTopfAddr) || umlaufZahlt(quantenWPfandAddr) {
		t.Fatal("Umlaufabgabe der W-Toepfe falsch")
	}
	if !isTokenomicsPoolAddress(quantenWTopfAddr) || !isTokenomicsPoolAddress(quantenWPfandAddr) || quantenWTopfAddr == quantenWPfandAddr {
		t.Fatal("W-Toepfe keine Protokoll-Toepfe")
	}
	if cs := newTestState(); cs.umlaufArt(quantenWTopfAddr, false) != artFrei {
		t.Fatal("W-Topf zahlt nicht wie ein freies Konto")
	}
}

// Register: Kodierung hin und zurueck gleich; was so nie entstehen konnte,
// wird beim Lesen (Start, Snapshot, Checkpoint) abgewiesen.
func TestQuantenWKette_RegisterLesen(t *testing.T) {
	w := neueQwWelt(t, 200)
	z := qwT0
	w.annehmen(w.praegenTx(z, 10_000_000), z+1)
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", z+10), z+11)
	w.annehmen(w.zufallTx(0), z+12)
	k := w.cs.quantenWJetzt().kodiert()
	r, err := quantenWRegisterLesen(k)
	if err != nil || r.kodiert() != k {
		t.Fatalf("hin und zurueck: %v", err)
	}
	if pruefeSnapshotKonfig(quantenWKonfigSchluessel, k) != nil {
		t.Fatal("Snapshot-Pruefung weist gutes Register ab")
	}
	var roh map[string]interface{}
	json.Unmarshal([]byte(k), &roh)
	note := func(m map[string]interface{}) map[string]interface{} {
		return m["noten"].([]interface{})[0].(map[string]interface{})
	}
	for name, aendern := range map[string]func(m map[string]interface{}){
		"unbekanntes Feld":         func(m map[string]interface{}) { m["x"] = 1 },
		"Sperre ohne Fehlversuche": func(m map[string]interface{}) { note(m)["gesperrt"] = true },
		"Zufall ueber der Anfrage": func(m map[string]interface{}) { note(m)["z_index"].([]interface{})[0] = 5.0 },
		"Anfrage mit anderer Nummer": func(m map[string]interface{}) {
			note(m)["anfrage"].(map[string]interface{})["Index"] = 3.0
		},
		"Bitfeld zu lang": func(m map[string]interface{}) { note(m)["benutzt"] = note(m)["benutzt"].(string) + "00" },
		"Bitfeld gross":   func(m map[string]interface{}) { note(m)["benutzt"] = strings.ToUpper(note(m)["benutzt"].(string)) },
		"Bitfeld leer kodiert": func(m map[string]interface{}) {
			note(m)["benutzt"] = strings.Repeat("0", len(note(m)["benutzt"].(string)))
		},
		"k_note falsch":               func(m map[string]interface{}) { note(m)["k_note"] = 8.0 },
		"Toleranz alt (15)":           func(m map[string]interface{}) { note(m)["tol"] = 15.0 },
		"Toleranz fehlt":              func(m map[string]interface{}) { note(m)["tol"] = 0.0 },
		"offen ohne Antwort":          func(m map[string]interface{}) { note(m)["anfrage"].(map[string]interface{})["Offen"] = 3.0 },
		"Treffer ohne offene Stellen": func(m map[string]interface{}) { note(m)["anfrage"].(map[string]interface{})["Treffer"] = 1.0 },
		"Fehler ueber den Treffern":   func(m map[string]interface{}) { note(m)["anfrage"].(map[string]interface{})["Fehler"] = 1.0 },
		"Validator doppelt":           func(m map[string]interface{}) { v := note(m)["validatoren"].([]interface{}); v[1] = v[0] },
		"keine Anteile":               func(m map[string]interface{}) { note(m)["anteile"] = 0.0 },
		"Pfand unter 1 AEQ":           func(m map[string]interface{}) { note(m)["anfrage"].(map[string]interface{})["Pfand"] = 5.0 },
		"Basen ohne Stellen":          func(m map[string]interface{}) { note(m)["anfrage"].(map[string]interface{})["Stellen"] = nil },
		"tote Note doppelt": func(m map[string]interface{}) {
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
		if pruefeSnapshotKonfig(quantenWKonfigSchluessel, string(b)) == nil {
			t.Errorf("%s: Snapshot angenommen", name)
		}
	}
	if _, err := quantenWRegisterLesen(`{}`); err == nil {
		t.Error("leeres Register nicht leer kodiert: gelesen")
	}
}

// BuchAt: Annahme und Nachspielen rechnen mit derselben Zeit (der Annahme),
// auch wenn der Block spaeter kommt -- sonst wichen Register und StateRoot
// ab. Ein BuchAt, das nicht zum Block passt, macht den Block falsch.
func TestQuantenWKette_BuchZeit(t *testing.T) {
	w := neueQwWelt(t, 200)
	z := qwT0
	w.annehmen(w.praegenTx(z, 10_000_000), z+1)
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", z+10), z+11)
	// Zufall angenommen um z+12, Block erst eine Stunde spaeter: die Zufallszeit
	// im Register ist bei beiden Knoten z+12.
	u := w.bescheid(w.cs, w.zufallTx(0), z+12)
	tx := w.zufallTx(0)
	tx.BuchAt = z + 12
	for name, bt := range map[string]struct{ buch, block int64 }{
		"ohne BuchAt":           {0, z + 12},
		"BuchAt nach dem Block": {z + 12, z + 12 - 61},
		"BuchAt zu alt":         {z + 12, z + 12 + quantenWBuchSpiel + 1},
	} {
		tx.BuchAt = bt.buch
		if err := w.nachspielen(tx, bt.block); err == nil {
			t.Errorf("%s: Block angenommen", name)
		}
	}
	w.cs.mu.Lock()
	w.cs.quantenWAnwendenLocked(context.Background(), u)
	w.cs.mu.Unlock()
	tx.BuchAt = z + 12
	if err := w.nachspielen(tx, z+3600); err != nil {
		t.Fatalf("Block eine Stunde spaeter: %v", err)
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() || w.anfrage(w.nach).Gezogen != z+12 {
		t.Fatal("Register weichen ab: Nachspielende rechnen nicht mit BuchAt")
	}
	// Eine Antwort nach der Frist laesst sich nicht in einen spaeteren Block
	// mit passender Zeit schmuggeln: BuchAt nach der Frist ist abgewiesen.
	a := w.anfrage(w.cs)
	spaet := w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen))
	spaet.BuchAt = z + 12 + quantenWFristAntwort + 1
	if err := w.nachspielen(spaet, spaet.BuchAt); err == nil {
		t.Fatal("Antwort nach der Frist nachgespielt")
	}
	// block_tauglich: nur Bloecke, zu denen BuchAt passt.
	ok := w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen))
	ok.BuchAt = z + 20
	if fenster := auftragsFenster(&ok, z+20); fenster.von > z+20 || fenster.bis < z+20+quantenWBuchSpiel-1 || fenster.von < z+20-60 {
		t.Fatalf("Zeitfenster %+v", fenster)
	}
}

// Die Annahme stempelt BuchAt und rechnet selbst (gerechnete Felder aus der
// Einreichung zaehlen nicht).
func TestQuantenWKette_Annahme(t *testing.T) {
	w := neueQwWelt(t, 200)
	quantenWAbOverride.Store(1)
	jetzt := nowUnix()
	tx := w.praegenTx(jetzt, 10_000_000)
	tx.QuantenW.Anteile = 1
	fertig, err := w.cs.quantenWAnnehmen(tx)
	if err != nil {
		t.Fatalf("Praegen: %v", err)
	}
	if fertig.BuchAt < jetzt || fertig.TxHash == "" || fertig.QuantenW.Anteile != 10_000_000 || w.cs.quantenWJetzt().Noten[w.p.K].Anteile != 10_000_000 {
		t.Fatalf("Annahme: BuchAt %d, Anteile %d", fertig.BuchAt, fertig.QuantenW.Anteile)
	}
	if err := w.nachspielen(fertig, fertig.BuchAt+5); err != nil {
		t.Fatalf("Nachspielen der angenommenen Transaktion: %v", err)
	}
	if _, err := w.cs.quantenWAnnehmen(w.praegenTx(jetzt, 10_000_000)); err == nil || !strings.Contains(err.Error(), "quanten_w_doppelt") {
		t.Fatalf("zweimal angenommen: %v", err)
	}
}

// Ein ungebundener Validator faellt an der Bindung auf (nicht erst daran, dass
// "" kein Mensch ist).
func TestQuantenWKette_UngebundenerValidator(t *testing.T) {
	w := neueQwWelt(t, 200)
	op := w.betreiber[w.val[2]]
	delete(w.betreiber, w.val[2])
	u := w.bescheid(w.cs, w.praegenTx(qwT0, 10_000_000), qwT0+1)
	if u.regel != "quanten_w_validator" || !strings.Contains(u.text, "kein gebundener Validator") {
		t.Fatalf("ungebundener Validator: %q %q", u.regel, u.text)
	}
	w.betreiber[w.val[2]] = op
}

// Tote Note neu praegen mit zurueckdatiertem BuchAt: ein spaeterer Block hat
// die Liste der Toten schon aufgeraeumt, der Erzeuger datiert BuchAt um fast
// 7 Tage zurueck, damit die alte Bestaetigung (Praegezeit z) wieder ins
// Fenster passt. Die Note muss trotzdem gemerkt sein.
func TestQuantenWKette_ToteGegenZurueckdatieren(t *testing.T) {
	w := neueQwWelt(t, 72)
	z := qwT0
	alt := w.p
	altPraegen := w.praegenTx(z, 5_000_000)
	w.annehmen(altPraegen, z+1)
	// Erschoepfen (Stoerer): eine Pruefung, ein falsches Einloesen.
	w.annehmen(w.anfrageTx(w.anna, 0, 8, "", z+10), z+11)
	w.annehmen(w.zufallTx(0), z+12)
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), z+13)
	w.annehmen(w.offenTx(0, a.Stellen), z+14)
	w.annehmen(w.anfrageTx(w.bert, 1, qwLK, w.bert, z+20), z+21)
	w.annehmen(w.zufallTx(1), z+22)
	w.annehmen(w.antwortTx(w.bert, 1, make([]int, qwLK)), z+23)
	a = w.anfrage(w.cs)
	if tx := w.annehmen(w.offenTx(1, a.Stellen), z+24); tx.QuantenW.Urteil == "echt" {
		t.Fatal("Nullantwort bestand -- Saat des Tests aendern (deterministisch)")
	}
	if len(w.cs.quantenWJetzt().Noten) != 0 {
		t.Fatal("Note nicht erschoepft")
	}
	// Spaeterer Block praegt eine andere Note -- und raeumt die Toten auf, die
	// nicht mehr gemerkt werden muessen.
	spaeter := z + quantenWAlt + quantenWVoraus + quantenWBuchSpiel/2
	w.p = qwPraegen(200, 3, 5, 4711)
	w.annehmen(w.praegenTx(spaeter, 5_000_000), spaeter)
	if _, da := w.cs.quantenWJetzt().Tote[alt.K]; !da {
		t.Fatal("tote Note zu frueh vergessen")
	}
	// Erzeuger datiert BuchAt zurueck (im erlaubten Spiel vor dem Block).
	neu := altPraegen
	neu.BuchAt = z + 100
	q := *neu.QuantenW
	q.Anteile = 5_000_000
	neu.QuantenW = &q
	if err := w.nachspielen(neu, spaeter+60); err == nil || !strings.Contains(err.Error(), "quanten_w_doppelt") {
		t.Fatalf("tote Note mit zurueckdatiertem BuchAt neu gepraegt: %v", err)
	}
}
