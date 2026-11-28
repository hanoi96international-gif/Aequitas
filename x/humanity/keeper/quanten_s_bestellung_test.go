package keeper

import (
	"context"
	"crypto/ecdsa"
	"math"
	"slices"
	"strings"
	"testing"
)

// Weg B, Bestellung, Los und Ausgabe gegen das Los (quanten_s_bestellung.go):
// ein annehmender und ein nachspielender Knoten im Gleichschritt. drand ist
// ein Testverbund mit bekanntem Schluessel (drandTestNetz), damit jede Runde
// signiert werden kann; der echte Beacon steht in einem eigenen Test.

const bwPraep = 20 // gemeldete Praeparatoren

type bWelt struct {
	*qsWelt
	praep []string
	sig   func(r uint64) string
	z     int64 // Bestellzeit
}

func neueBWelt(t *testing.T) *bWelt {
	t.Helper()
	return neueBWeltN(t, 64, 24)
}

// neueBWeltN: mit einer Praegung ueber n Stellen und Stichproben dieser Laenge
// (Probelauf, quanten_s_wegb_probelauf_test.go).
func neueBWeltN(t *testing.T, n, probe int) *bWelt {
	t.Helper()
	w := &bWelt{qsWelt: neueQsWelt(t, n, probe), z: qwT0}
	quantenSWegBAbOverride.Store(qwT0 - 30*86400)
	netz, sig := drandTestNetz(t, 0x5eed1234)
	alt := drandQuicknet
	drandQuicknet, w.sig = netz, sig
	t.Cleanup(func() { quantenSWegBAbOverride.Store(0); drandQuicknet = alt })
	reg := neuesQuantenWRegister()
	for i := 0; i < bwPraep; i++ {
		k, a := neuerSchluessel(t)
		w.keys[a] = k
		w.praep = append(w.praep, a)
		addHuman(w.cs, a, 0)
		addHuman(w.nach, a, 0)
		reg.Praep[quantenSPraepK(a)] = quantenSPraep{Konto: a, Pub: praepPubTest(w.keys[a]), Gemeldet: w.z - 2*86400}
	}
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	return w
}

// bwDeckung: die Deckung, die bestellt und ausgegeben wird (ausgabeMit).
const bwDeckung int64 = 10_000_000

func (w *bWelt) bestellenTx(von string, key *ecdsa.PrivateKey, K string, v, tt int, zeit int64) Transaction {
	return w.bestellenTxD(von, key, K, v, tt, bwDeckung, zeit)
}

func (w *bWelt) bestellenTxD(von string, key *ecdsa.PrivateKey, K string, v, tt int, deckung, zeit int64) Transaction {
	return Transaction{Type: quantenSBestellenTxArt, Wallet: von, DistributionAt: zeit, QuantenW: &QuantenWAngaben{K: K, PraepZahl: v, T: tt, Deckung: deckung,
		WalletSig: personalSign(w.t, key, quantenSBestellNachricht(von, K, v, tt, deckung, zeit))}}
}

func (w *bWelt) fristTx(K string) Transaction {
	return Transaction{Type: quantenSBestellFristTxArt, QuantenW: &QuantenWAngaben{K: K}}
}

func (w *bWelt) losenTx(K string, r uint64, sig string) Transaction {
	return Transaction{Type: quantenSLosenTxArt, QuantenW: &QuantenWAngaben{K: K, Runde: r, RundeSig: sig}}
}

// runde: die Runde, die eine Bestellung zu w.z lost.
func (w *bWelt) runde() uint64 {
	r, ok := quantenSLosRunde(w.z)
	if !ok {
		w.t.Fatal("keine Runde")
	}
	return r
}

// ausgabeMit: die S-Ausgabe von w.p.K mit diesen Praeparatoren, jeder
// bestaetigt mit seinem eigenen Schluessel (wenn nicht anders gesetzt).
func (w *bWelt) ausgabeMit(praep []string, zeit int64) Transaction {
	w.val, w.valKeys = slices.Clone(praep), nil
	for _, a := range praep {
		w.valKeys = append(w.valKeys, w.keys[a])
	}
	return w.ausgebenTx(zeit, bwDeckung)
}

func (w *bWelt) best(cs *ChainState) *quantenSBestellung {
	return cs.quantenWJetzt().Best[w.p.K]
}

// bis zum Los: bestellen (v = 5, t = 3 wie die Praegung) und losen.
func (w *bWelt) bestelltUndGelost() []string {
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	r := w.runde()
	w.annehmen(w.losenTx(w.p.K, r, w.sig(r)), w.z+quantenSLosAbstand+5)
	return w.best(w.cs).Los
}

// Gutfall: bestellen, losen, ausgeben -- das Los ist das der Spezifikation
// (quantenSLos aus Abbild und Zufall), die Bestellung danach verbraucht.
func TestQuantenSBestellung_Gutfall(t *testing.T) {
	w := neueBWelt(t)
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	b := w.best(w.cs)
	if b == nil || len(b.Abbild) != bwPraep || b.Runde != 0 || b.Ausgeber != w.praeger || b.Zeit != w.z {
		t.Fatalf("Bestellung: %+v", b)
	}
	r := w.runde()
	if zr, _ := drandQuicknet.zeitDerRunde(r); zr < w.z+quantenSLosAbstand || zr-3 >= w.z+quantenSLosAbstand {
		t.Fatalf("Runde %d (Zeit %d) ist nicht die erste nach Bestellung + Abstand", r, zr)
	}
	w.annehmen(w.losenTx(w.p.K, r, w.sig(r)), w.z+quantenSLosAbstand+5)
	b = w.best(w.cs)
	zufall, _ := drandQuicknet.pruefen(r, w.sig(r))
	soll, _ := quantenSLos(b.Abbild, zufall, w.p.K, 5)
	if b.Runde != r || !slices.Equal(b.Los, soll) || len(b.Los) != 5 {
		t.Fatalf("Los: %v, erwartet %v", b.Los, soll)
	}
	w.annehmen(w.ausgabeMit(b.Los, w.z+900), w.z+900)
	if w.best(w.cs) != nil || w.cs.quantenWJetzt().Token[w.p.K] == nil {
		t.Fatal("nach der Ausgabe: Bestellung nicht verbraucht oder kein Token")
	}
	if tok := w.cs.quantenWJetzt().Token[w.p.K].token; tok.NVal != 5 || tok.T != w.p.t {
		t.Fatalf("Token: v %d, t %d", tok.NVal, tok.T)
	}
}

// Das Abbild steht in der Bestellung: Abmeldungen und neue Meldungen danach
// verschieben das Los nicht.
func TestQuantenSBestellung_AbbildFest(t *testing.T) {
	w := neueBWelt(t)
	// Bert meldet sich 100 s vor der Bestellung: nicht im Abbild (Vorlauf).
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.bert, w.keys[w.bert], w.z-100), w.z-100)
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	vorher := slices.Clone(w.best(w.cs).Abbild)
	if slices.Contains(vorher, w.bert) || len(vorher) != bwPraep {
		t.Fatalf("Abbild zur Bestellung: %d Eintraege, Bert drin: %v", len(vorher), slices.Contains(vorher, w.bert))
	}
	// Vier Praeparatoren melden sich ab, eine meldet sich neu.
	for _, a := range w.praep[:4] {
		w.annehmen(praepTx(t, quantenSPraepAbmeldenTxArt, a, w.keys[a], w.z+10), w.z+10)
	}
	w.annehmen(praepTx(t, quantenSPraepMeldenTxArt, w.anna, w.keys[w.anna], w.z+20), w.z+20)
	r := w.runde()
	w.annehmen(w.losenTx(w.p.K, r, w.sig(r)), w.z+quantenSLosAbstand+5)
	b := w.best(w.cs)
	zufall, _ := drandQuicknet.pruefen(r, w.sig(r))
	soll, _ := quantenSLos(vorher, zufall, w.p.K, 5)
	if !slices.Equal(b.Abbild, vorher) || !slices.Equal(b.Los, soll) {
		t.Fatalf("Los verschoben: %v statt %v", b.Los, soll)
	}
}

func TestQuantenSBestellung_BestellenMissbrauch(t *testing.T) {
	w := neueBWelt(t)
	z := w.z
	fremd, fremdAdr := neuerSchluessel(t) // kein Mensch
	w.cs.ensureAccountLoadedCtx(context.Background(), fremdAdr)
	K := w.p.K
	pr := w.keys[w.praeger]
	for _, f := range []struct {
		name string
		tx   Transaction
		r    string
	}{
		{"kein Mensch", w.bestellenTx(fremdAdr, fremd, K, 5, 3, z), "quanten_s_praep_mensch"},
		{"fremder Schluessel", w.bestellenTx(w.praeger, w.keys[w.anna], K, 5, 3, z), "quanten_w_unterschrift"},
		{"alte Unterschrift", w.bestellenTx(w.praeger, pr, K, 5, 3, z-quantenWAlt-1), "quanten_w_zeit"},
		{"v zu klein", w.bestellenTx(w.praeger, pr, K, 3, 3, z), "quanten_w_form"},
		{"v zu gross", w.bestellenTx(w.praeger, pr, K, 17, 3, z), "quanten_w_form"},
		{"t groesser als v", w.bestellenTx(w.praeger, pr, K, 5, 6, z), "quanten_w_form"},
		{"t null", w.bestellenTx(w.praeger, pr, K, 5, 0, z), "quanten_w_form"},
		{"Register zu klein", w.bestellenTx(w.praeger, pr, K, 7, 3, z), "quanten_s_los_register"},
	} {
		if r := w.praepRegel(f.tx, z); r != f.r {
			t.Errorf("%s: Regel %q, erwartet %q", f.name, r, f.r)
		}
	}
	// Fremde Felder an der Bestellung (Runde, Validatoren).
	mitRunde := w.bestellenTx(w.praeger, pr, K, 5, 3, z)
	mitRunde.QuantenW.Runde = 5
	mitVal := w.bestellenTx(w.praeger, pr, K, 5, 3, z)
	mitVal.QuantenW.Validatoren = w.praep[:5]
	for _, tx := range []Transaction{mitRunde, mitVal} {
		if r := w.praepRegel(tx, z); r != "quanten_w_form" {
			t.Errorf("fremdes Feld: %q", r)
		}
	}
	// Doppelt: dasselbe K, und ein zweites K desselben Ausgebers.
	w.annehmen(w.bestellenTx(w.praeger, pr, K, 5, 3, z), z)
	if r := w.praepRegel(w.bestellenTx(w.anna, w.keys[w.anna], K, 5, 3, z+1), z+1); r != "quanten_w_doppelt" {
		t.Errorf("K zweimal bestellt: %q", r)
	}
	K2 := strings.Repeat("b", 64)
	if r := w.praepRegel(w.bestellenTx(w.praeger, pr, K2, 5, 3, z+1), z+1); r != "quanten_s_bestellung_offen" {
		t.Errorf("zweite Bestellung desselben Ausgebers: %q", r)
	}
	// Ein K, das schon als Token lebt.
	w2 := neueBWelt(t)
	w2.annehmen(w2.ausgebenTx(w2.z, 10_000_000), w2.z)
	if r := w2.praepRegel(w2.bestellenTx(w2.anna, w2.keys[w2.anna], w2.p.K, 5, 3, w2.z+1), w2.z+1); r != "quanten_w_doppelt" {
		t.Errorf("Bestellung eines ausgegebenen K: %q", r)
	}
}

// Eine W-Note mit dem K einer offenen Bestellung wird abgewiesen -- sonst
// stuende ein Register in der Kette, das kein Knoten mehr laden koennte.
func TestQuantenSBestellung_NoteMitBestelltemK(t *testing.T) {
	w := neueBWelt(t)
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	// praegenTx braucht die gebundenen Validatoren der Praegung.
	w.val, w.valKeys = nil, nil
	for x := 0; x < 5; x++ {
		k, a := neuerSchluessel(t)
		_, op := neuerSchluessel(t)
		w.valKeys, w.val = append(w.valKeys, k), append(w.val, a)
		w.betreiber[a] = op
		addHuman(w.cs, op, 0)
		addHuman(w.nach, op, 0)
	}
	if r := w.praepRegel(w.praegenTx(w.z+10, 10_000_000), w.z+10); r != "quanten_w_doppelt" {
		t.Fatalf("W-Note mit bestelltem K: %q", r)
	}
	if _, err := quantenWRegisterLesen(w.cs.quantenWJetzt().kodiert()); err != nil {
		t.Fatalf("Register nicht mehr lesbar: %v", err)
	}
}

// Abbruch und neu bestellen (Befund B2b): wer das Los nicht ausgibt, wartet
// die Frist ab, bevor er neu bestellt -- auch wenn er nie ausgibt.
func TestQuantenSBestellung_AbbruchGebremst(t *testing.T) {
	w := neueBWelt(t)
	pr := w.keys[w.praeger]
	w.bestelltUndGelost()
	K2 := strings.Repeat("c", 64)
	ende := w.z + quantenSBestellFrist
	if r := w.praepRegel(w.bestellenTx(w.praeger, pr, K2, 5, 3, ende-1), ende-1); r != "quanten_s_bestellung_offen" {
		t.Fatalf("neu bestellt vor dem Ende der Frist: %q", r)
	}
	w.annehmen(w.bestellenTx(w.praeger, pr, K2, 5, 3, ende), ende)
	if w.cs.quantenWJetzt().Best[w.p.K] != nil {
		t.Fatal("verfallene Bestellung nicht aufgeraeumt")
	}
}

func TestQuantenSBestellung_LosenMissbrauch(t *testing.T) {
	w := neueBWelt(t)
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	r := w.runde()
	nach := w.z + quantenSLosAbstand + 5
	_, fremdSig := drandTestNetz(t, 0x0badbeef)
	for _, f := range []struct {
		name string
		tx   Transaction
		zeit int64
		r    string
	}{
		{"spaetere Runde, echt signiert", w.losenTx(w.p.K, r+1, w.sig(r+1)), nach, "quanten_s_los_runde"},
		{"fruehere Runde, echt signiert", w.losenTx(w.p.K, r-1, w.sig(r-1)), nach, "quanten_s_los_runde"},
		{"Signatur einer anderen Runde", w.losenTx(w.p.K, r, w.sig(r+1)), nach, "quanten_s_los_drand"},
		{"fremder Verbund", w.losenTx(w.p.K, r, fremdSig(r)), nach, "quanten_s_los_drand"},
		{"Bit gekippt", w.losenTx(w.p.K, r, kippeLetztesHex(w.sig(r))), nach, "quanten_s_los_drand"},
		{"unbekanntes K", w.losenTx(strings.Repeat("d", 64), r, w.sig(r)), nach, "quanten_w_unbekannt"},
		{"nach der Frist", w.losenTx(w.p.K, r, w.sig(r)), w.z + quantenSBestellFrist, "quanten_w_frist"},
	} {
		if g := w.praepRegel(f.tx, f.zeit); g != f.r {
			t.Errorf("%s: Regel %q, erwartet %q", f.name, g, f.r)
		}
	}
	mitWallet := w.losenTx(w.p.K, r, w.sig(r))
	mitWallet.Wallet = w.anna
	mitZeit := w.losenTx(w.p.K, r, w.sig(r))
	mitZeit.DistributionAt = nach
	for _, tx := range []Transaction{mitWallet, mitZeit} {
		if g := w.praepRegel(tx, nach); g != "quanten_w_form" && g != "quanten_w_zeit" {
			t.Errorf("Losen mit Wallet oder Zeit: %q", g)
		}
	}
	w.annehmen(w.losenTx(w.p.K, r, w.sig(r)), nach)
	if g := w.praepRegel(w.losenTx(w.p.K, r, w.sig(r)), nach+1); g != "quanten_s_los_doppelt" {
		t.Errorf("zweimal gelost: %q", g)
	}
}

func TestQuantenSBestellung_AusgabeMissbrauch(t *testing.T) {
	w := neueBWelt(t)
	z := w.z + 900
	// Vor dem Los: keine Ausgabe, auch nicht mit Weg A.
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	if r := w.praepRegel(w.ausgebenTx(z, bwDeckung), z); r != "quanten_s_los_offen" {
		t.Fatalf("Ausgabe vor dem Los: %q", r)
	}
	r := w.runde()
	w.annehmen(w.losenTx(w.p.K, r, w.sig(r)), w.z+quantenSLosAbstand+5)
	los := slices.Clone(w.best(w.cs).Los)
	var draussen string // gemeldet, aber nicht gelost
	for _, a := range w.praep {
		if !slices.Contains(los, a) {
			draussen = a
			break
		}
	}
	fremd := slices.Clone(los)
	fremd[2] = draussen
	vertauscht := slices.Clone(los)
	vertauscht[0], vertauscht[1] = vertauscht[1], vertauscht[0]
	w.ausgabeMit(los, z)
	w.valKeys[3] = w.keys[draussen]
	falscherSchluessel := w.ausgebenTx(z, bwDeckung)
	weitere := w.ausgabeMit(append(slices.Clone(los), draussen), z)
	for _, f := range []struct {
		name string
		tx   Transaction
		zeit int64
		r    string
	}{
		{"fuenf beliebige Menschen", w.ausgabeMit(w.praep[15:20], z), z, "quanten_s_los_fremd"},
		{"ein Fremder statt eines Gelosten", w.ausgabeMit(fremd, z), z, "quanten_s_los_fremd"},
		{"Reihenfolge vertauscht", w.ausgabeMit(vertauscht, z), z, "quanten_s_los_fremd"},
		{"einer fehlt", w.ausgabeMit(los[:4], z), z, "quanten_s_los_fremd"},
		{"einer zu viel", weitere, z, "quanten_s_los_fremd"},
		{"fremder Schluessel bestaetigt", falscherSchluessel, z, "quanten_w_bestaetigung"},
		{"nach der Frist", w.ausgabeMit(los, w.z+quantenSBestellFrist), w.z + quantenSBestellFrist, "quanten_w_frist"},
	} {
		if g := w.praepRegel(f.tx, f.zeit); g != f.r {
			t.Errorf("%s: Regel %q, erwartet %q", f.name, g, f.r)
		}
	}
	// Ein anderer Ausgeber mit dem richtigen Los.
	anders := w.ausgabeMit(los, z)
	q := anders.QuantenW
	q.WalletSig = personalSign(t, w.keys[w.anna], quantenWSAusgabeNachricht(w.anna, q.K, q.N, q.T, q.Wurzel, q.Validatoren, q.Proben, q.Annahme, q.Inhaber, q.Deckung, z, q.Tol))
	anders.Wallet = w.anna
	if g := w.praepRegel(anders, z); g != "quanten_s_los_ausgeber" && g != "quanten_w_form" {
		t.Errorf("fremder Ausgeber: %q", g)
	}
	w.annehmen(w.ausgabeMit(los, z), z)
}

// Die Buchzeit einer Bestellung darf nur kurz vor dem Block liegen (sonst
// waere die Runde des Loses schon bekannt); andere W-Arten bleiben bei
// quantenWBuchSpiel.
func TestQuantenSBestellung_Buchzeit(t *testing.T) {
	block := qwT0
	if _, ok := quantenWBuchZeit(quantenSBestellenTxArt, block-quantenSBestellBuchSpiel, block); !ok {
		t.Fatal("Bestellung im Spiel abgewiesen")
	}
	if _, ok := quantenWBuchZeit(quantenSBestellenTxArt, block-quantenSBestellBuchSpiel-1, block); ok {
		t.Fatal("zurueckdatierte Bestellung angenommen")
	}
	if _, ok := quantenWBuchZeit(quantenSPraepMeldenTxArt, block-quantenSBestellBuchSpiel-1000, block); !ok {
		t.Fatal("Meldung mit gewoehnlichem Buchspiel abgewiesen")
	}
	w := neueBWelt(t)
	tx := w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z)
	tx.BuchAt = w.z
	if err := w.nachspielen(tx, w.z+quantenSBestellBuchSpiel+1); err == nil {
		t.Fatal("Block mit zurueckdatierter Bestellung nachgespielt")
	}
	if f := auftragsFenster(&tx, w.z); f.bis > w.z+quantenSBestellBuchSpiel {
		t.Fatalf("Blockerzeuger nimmt die Bestellung bis %d auf", f.bis)
	}
}

// Der echte quicknet-Beacon (Runde 1.000.000) lost eine Bestellung, deren
// Zeit genau auf diese Runde faellt.
func TestQuantenSBestellung_EchterBeacon(t *testing.T) {
	w := neueBWelt(t)
	drandQuicknet = func() drandNetz {
		n, _ := drandNetzAus(drandQuicknetSchluesselHex, drandQuicknetGenesis, drandQuicknetPeriode)
		return n
	}()
	zr, _ := drandQuicknet.zeitDerRunde(drandEchtRunde)
	w.z = zr - quantenSLosAbstand
	quantenWAbOverride.Store(w.z - 30*86400)
	quantenSWegBAbOverride.Store(w.z - 30*86400)
	reg := neuesQuantenWRegister()
	for _, a := range w.praep {
		reg.Praep[quantenSPraepK(a)] = quantenSPraep{Konto: a, Pub: praepPubTest(w.keys[a]), Gemeldet: w.z - 2*86400}
	}
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	if w.runde() != drandEchtRunde {
		t.Fatalf("Runde %d statt %d", w.runde(), drandEchtRunde)
	}
	w.annehmen(w.losenTx(w.p.K, drandEchtRunde, drandEchtSig), zr+5)
	if len(w.best(w.cs).Los) != 5 {
		t.Fatal("kein Los aus dem echten Beacon")
	}
}

// Register lesen: Bestellungen streng, das Los rechnet jeder Knoten nach.
func TestQuantenSBestellung_RegisterLesen(t *testing.T) {
	w := neueBWelt(t)
	w.bestelltUndGelost()
	roh := w.cs.quantenWJetzt().kodiert()
	if r, err := quantenWRegisterLesen(roh); err != nil || r.kodiert() != roh {
		t.Fatalf("Rundweg: %v", err)
	}
	gut := w.cs.quantenWJetzt().form()
	b := gut.Best[0]
	kaputt := map[string]func(f *quantenSBestellungForm){
		"Los vertauscht":    func(f *quantenSBestellungForm) { f.Los[0], f.Los[1] = f.Los[1], f.Los[0] },
		"Los fremd":         func(f *quantenSBestellungForm) { f.Los[0] = w.anna },
		"Runde daneben":     func(f *quantenSBestellungForm) { f.Runde++ },
		"Zufall ohne Runde": func(f *quantenSBestellungForm) { f.Runde = 0 },
		"Abbild unsortiert": func(f *quantenSBestellungForm) { f.Abbild[0], f.Abbild[1] = f.Abbild[1], f.Abbild[0] },
		"Abbild zu klein":   func(f *quantenSBestellungForm) { f.Abbild = f.Abbild[:3*f.V-1] },
		"v zu gross":        func(f *quantenSBestellungForm) { f.V = 17 },
		"ohne Zeit":         func(f *quantenSBestellungForm) { f.Zeit = 0 },
		"Pfand zu klein":    func(f *quantenSBestellungForm) { f.Pfand-- },
		"Deckung zu klein":  func(f *quantenSBestellungForm) { f.Deckung = 1; f.Pfand = quantenSBestellPfandFuer(1) },
		// Der Ausgeber im eigenen Abbild (ungelost, damit nur diese Regel greift).
		"Ausgeber im Abbild": func(f *quantenSBestellungForm) {
			f.Runde, f.Zufall, f.Los = 0, "", nil
			f.Abbild = append(f.Abbild, f.Ausgeber)
			slices.Sort(f.Abbild)
		},
	}
	for name, aendern := range kaputt {
		f := gut
		k := b
		k.Abbild, k.Los = slices.Clone(b.Abbild), slices.Clone(b.Los)
		aendern(&k)
		f.Best = []quantenSBestellungForm{k}
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	// Zwei Bestellungen desselben Ausgebers.
	f := gut
	k2 := b
	k2.K, k2.Runde, k2.Zufall, k2.Los = strings.Repeat("f", 64), 0, "", nil
	f.Best = []quantenSBestellungForm{b, k2}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
		t.Error("zwei Bestellungen eines Ausgebers angenommen")
	}
	if z := gut.zeilen(); z[quantenWZeile{quantenWArtBest, b.K}] == "" {
		t.Error("keine Zeile der Bestellung")
	}
}

// Schlafend: die beiden Arten vor dem Stichtag von Weg B unbekannt.
func TestQuantenSBestellung_Schlafend(t *testing.T) {
	// Schlafend ausdruecklich (der Stichtag im Code liegt vor qwT0): geprueft wird der Mechanismus.
	quantenSWegBAbOverride.Store(math.MaxInt64)
	t.Cleanup(func() { quantenSWegBAbOverride.Store(0) })
	for _, art := range []string{quantenSBestellenTxArt, quantenSLosenTxArt} {
		if bekannteTxArt(art, qwT0) || !istQuantenWTxArt(art) {
			t.Errorf("%s vor dem Stichtag bekannt oder nicht im W-Modul", art)
		}
	}
	w := neueQwWelt(t, 64)
	tx := Transaction{Type: quantenSLosenTxArt, QuantenW: &QuantenWAngaben{K: strings.Repeat("a", 64), Runde: 1, RundeSig: "00"}}
	if u := w.bescheid(w.cs, tx, qwT0); u.regel != "quanten_s_praep_inaktiv" {
		t.Fatalf("vor dem Stichtag: %q", u.regel)
	}
}

// Pfand (B2c): genommen bei der Bestellung, zurueck bei der Ausgabe, ans
// Grundeinkommen nach der Frist -- ueber die eigene Art oder beim naechsten
// Bestellen. Geld bleibt erhalten.
func TestQuantenSBestellung_Pfand(t *testing.T) {
	w := neueBWelt(t)
	pfand := quantenSBestellPfandFuer(bwDeckung)
	vorher, topf, ubi := w.stand(w.cs, w.praeger), w.stand(w.cs, quantenWPfandAddr), w.stand(w.cs, ubiPoolAddr)
	g := w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z).QuantenW.gerechnet()
	if g.Pfand != pfand || w.stand(w.cs, w.praeger) != vorher-pfand || w.stand(w.cs, quantenWPfandAddr) != topf+pfand {
		t.Fatalf("Pfand beim Bestellen: %+v", g)
	}
	r := w.runde()
	w.annehmen(w.losenTx(w.p.K, r, w.sig(r)), w.z+quantenSLosAbstand+5)
	los := slices.Clone(w.best(w.cs).Los)
	// Andere Deckung als bestellt: abgewiesen.
	w.ausgabeMit(los, w.z+900)
	anders := w.ausgebenTx(w.z+900, bwDeckung+1)
	if g := w.praepRegel(anders, w.z+900); g != "quanten_s_los_ausgeber" {
		t.Fatalf("Ausgabe mit anderer Deckung: %q", g)
	}
	g = w.annehmen(w.ausgabeMit(los, w.z+900), w.z+900).QuantenW.gerechnet()
	if g.Pfand != pfand || g.PfandAn != w.praeger || w.stand(w.cs, w.praeger) != vorher-bwDeckung ||
		w.stand(w.cs, quantenWPfandAddr) != topf || w.stand(w.nach, w.praeger) != vorher-bwDeckung {
		t.Fatalf("Pfand nach der Ausgabe nicht zurueck: %+v, Ausgeber %d", g, w.stand(w.cs, w.praeger))
	}
	if w.stand(w.cs, ubiPoolAddr) != ubi {
		t.Fatal("Grundeinkommen bei ehrlicher Ausgabe veraendert")
	}

	// Abbruch: Frist abgelaufen, Pfand ans Grundeinkommen ueber die Frist-Art.
	w2 := neueBWelt(t)
	vorher, ubi = w2.stand(w2.cs, w2.praeger), w2.stand(w2.cs, ubiPoolAddr)
	w2.annehmen(w2.bestellenTx(w2.praeger, w2.keys[w2.praeger], w2.p.K, 5, w2.p.t, w2.z), w2.z)
	ende := w2.z + quantenSBestellFrist
	if g := w2.praepRegel(w2.fristTx(w2.p.K), ende-1); g != "quanten_w_frist" {
		t.Fatalf("Frist vor dem Ende: %q", g)
	}
	if g := w2.praepRegel(w2.fristTx(strings.Repeat("e", 64)), ende); g != "quanten_w_unbekannt" {
		t.Fatalf("Frist ohne Bestellung: %q", g)
	}
	mitWallet := w2.fristTx(w2.p.K)
	mitWallet.Wallet = w2.anna
	if g := w2.praepRegel(mitWallet, ende); g != "quanten_w_form" {
		t.Fatalf("Frist mit Wallet: %q", g)
	}
	g = w2.annehmen(w2.fristTx(w2.p.K), ende).QuantenW.gerechnet()
	if g.Pfand != pfand || w2.best(w2.cs) != nil || w2.stand(w2.cs, w2.praeger) != vorher-pfand || w2.stand(w2.cs, ubiPoolAddr) != ubi+pfand {
		t.Fatalf("Frist: %+v", g)
	}

	// Abbruch ohne Frist-Art: das naechste Bestellen nimmt das alte Pfand mit.
	w3 := neueBWelt(t)
	ubi = w3.stand(w3.cs, ubiPoolAddr)
	w3.annehmen(w3.bestellenTx(w3.praeger, w3.keys[w3.praeger], w3.p.K, 5, w3.p.t, w3.z), w3.z)
	ende = w3.z + quantenSBestellFrist
	w3.annehmen(w3.bestellenTx(w3.anna, w3.keys[w3.anna], strings.Repeat("9", 64), 5, 3, ende), ende)
	if w3.stand(w3.cs, ubiPoolAddr) != ubi+pfand || w3.stand(w3.nach, ubiPoolAddr) != ubi+pfand {
		t.Fatal("verfallenes Pfand nicht beim naechsten Bestellen ans Grundeinkommen")
	}

	// Zu wenig fuer das Pfand, Deckung unter dem Mindestwert.
	w4 := neueBWelt(t)
	if g := w4.praepRegel(w4.bestellenTxD(w4.anna, w4.keys[w4.anna], w4.p.K, 5, 3, 2_000_000_000, w4.z), w4.z); g != "quanten_w_deckung" {
		t.Fatalf("Pfand ohne Guthaben: %q", g)
	}
	if g := w4.praepRegel(w4.bestellenTxD(w4.anna, w4.keys[w4.anna], w4.p.K, 5, 3, quantenMindestDeckungMik-1, w4.z), w4.z); g != "quanten_w_form" {
		t.Fatalf("Deckung unter dem Mindestwert: %q", g)
	}
	// Die Unterschrift bindet die Deckung.
	gefaelscht := w4.bestellenTx(w4.anna, w4.keys[w4.anna], w4.p.K, 5, 3, w4.z)
	gefaelscht.QuantenW.Deckung = quantenMindestDeckungMik
	if g := w4.praepRegel(gefaelscht, w4.z); g != "quanten_w_unterschrift" {
		t.Fatalf("Deckung nach der Unterschrift geaendert: %q", g)
	}
}

// Die Hoehe des Pfands: 10 % aufgerundet, mindestens quantenWPfand, ohne
// Ueberlauf; und genug gegen Abbruch und neu bestellen: im Mittel kostet ein
// Los mit t Komplizen Pfand / p mehr als die bestellte Deckung.
func TestQuantenSBestellung_PfandHoehe(t *testing.T) {
	for _, f := range []struct{ d, p int64 }{
		{1, quantenWPfand}, {quantenMindestDeckungMik, quantenWPfand}, {10_000_000, 1_000_000}, {10_000_001, 1_000_001},
		{123_456_789, 12_345_679}, {math.MaxInt64, math.MaxInt64/100*10 + 1},
	} {
		if g := quantenSBestellPfandFuer(f.d); g != f.p {
			t.Errorf("Pfand fuer %d: %d, erwartet %d", f.d, g, f.p)
		}
	}
	// p je Los hoechstens ~3,1 % (DEZENTRAL 18, Tabelle): Pfand / p >= 3 D.
	const pHoechstens = 0.031
	for _, d := range []int64{quantenMindestDeckungMik, 10_000_000, 1_000_000_000} {
		if kosten := float64(quantenSBestellPfandFuer(d)) / pHoechstens; kosten < 3*float64(d) {
			t.Errorf("Deckung %d: Abbruch kostet im Mittel nur %.0f", d, kosten)
		}
	}
}

// Missbrauch: ein Blockerzeuger traegt bei Bestellen, Ausgabe oder Frist ein
// anderes Pfand ein, als die Regel ergibt (zu klein: der Ausgeber zahlt
// weniger; zu gross oder an einen anderen: Geld wandert falsch). Jeder
// Nachspielende rechnet das Pfand selbst (quantenSBestellPfandFuer) und weist
// den Block ab; das richtige Pfand nimmt er an.
func TestQuantenSBestellung_PfandGefaelschtImBlock(t *testing.T) {
	w := neueBWelt(t)
	// Nur rechnen (nicht anwenden): der Nachspielende sieht jede Variante frisch.
	tx := w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z)
	u := w.bescheid(w.cs, tx, w.z)
	if u.regel != "" {
		t.Fatalf("Bestellen abgewiesen: %s", u.text)
	}
	gq := *tx.QuantenW
	gq.Pfand, gq.PfandAn, gq.Wert, gq.WertAn = u.felder.Pfand, u.felder.PfandAn, u.felder.Wert, u.felder.WertAn
	tx.QuantenW, tx.BuchAt = &gq, w.z
	pfand := quantenSBestellPfandFuer(bwDeckung)
	if tx.QuantenW.Pfand != pfand {
		t.Fatalf("Pfand %d, Regel %d", tx.QuantenW.Pfand, pfand)
	}
	for name, f := range map[string]func(q *QuantenWAngaben){
		"zu klein":    func(q *QuantenWAngaben) { q.Pfand = pfand - 1 },
		"null":        func(q *QuantenWAngaben) { q.Pfand = 0 },
		"zu gross":    func(q *QuantenWAngaben) { q.Pfand = pfand + 1 },
		"an jemanden": func(q *QuantenWAngaben) { q.PfandAn = w.praep[0] },
		"Wert dazu":   func(q *QuantenWAngaben) { q.Wert = 1 },
	} {
		g := tx
		q := *tx.QuantenW
		f(&q)
		g.QuantenW = &q
		err := w.nachspielen(g, w.z)
		if err == nil || !(strings.Contains(err.Error(), "Block nennt") || strings.Contains(err.Error(), "quanten_w_form")) {
			t.Fatalf("%s: Block mit gefaelschtem Pfand nicht wegen des Pfands abgewiesen: %v", name, err)
		}
	}
	if err := w.nachspielen(tx, w.z); err != nil {
		t.Fatalf("richtiges Pfand abgewiesen: %v", err)
	}
	if b := w.best(w.nach); b == nil || b.Pfand != pfand {
		t.Fatal("Nachspielender hat das Pfand nicht wie die Regel")
	}
}

// Der Ausgeber praepariert nie fuer die eigene Ausgabe (25.11.2026): auch als
// gemeldeter Praeparator faellt er aus dem Abbild seiner Bestellung -- und
// zaehlt nicht fuer die Mindestgroesse des Registers.
func TestQuantenSBestellung_AusgeberNieImEigenenLos(t *testing.T) {
	w := neueBWelt(t)
	// Der Ausgeber ist (wie die anderen) seit zwei Tagen gemeldet -- direkt im Register beider Knoten.
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.mu.Lock()
		reg := cs.quantenWJetzt().kopie()
		reg.Praep[quantenSPraepK(w.praeger)] = quantenSPraep{Konto: w.praeger, Pub: praepPubTest(w.keys[w.praeger]), Gemeldet: w.z - 2*86400}
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	w.cs.mu.Lock()
	im := w.cs.quantenSPraepAbbildLocked(context.Background(), w.cs.quantenWJetzt(), w.z)
	w.cs.mu.Unlock()
	if !slices.Contains(im, w.praeger) || len(im) != bwPraep+1 {
		t.Fatalf("Vorbedingung: der Ausgeber ist gemeldet und im Register-Abbild (%d)", len(im))
	}
	// v = 7: 3*7 = 21 = 20 andere + der Ausgeber -- ohne ihn zu klein.
	if r := w.praepRegel(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 7, 4, w.z), w.z); r != "quanten_s_los_register" {
		t.Fatalf("v = 7 mit dem Ausgeber als einzigem Fehlenden: Regel %q", r)
	}
	// v = 5: das Abbild ohne ihn, das Los nie mit ihm.
	w.annehmen(w.bestellenTx(w.praeger, w.keys[w.praeger], w.p.K, 5, w.p.t, w.z), w.z)
	b := w.best(w.cs)
	if slices.Contains(b.Abbild, w.praeger) || len(b.Abbild) != bwPraep {
		t.Fatalf("Abbild mit dem Ausgeber oder falsch gross: %d", len(b.Abbild))
	}
	if nb := w.best(w.nach); nb == nil || !slices.Equal(nb.Abbild, b.Abbild) {
		t.Fatal("nachspielender Knoten rechnet ein anderes Abbild")
	}
	r := w.runde()
	w.annehmen(w.losenTx(w.p.K, r, w.sig(r)), w.z+quantenSLosAbstand+5)
	if slices.Contains(w.best(w.cs).Los, w.praeger) {
		t.Fatal("Ausgeber im eigenen Los")
	}
}
