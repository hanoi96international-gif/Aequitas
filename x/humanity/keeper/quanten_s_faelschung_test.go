package keeper

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

// P4, Nachweis einer Faelschung (quanten_s_faelschung.go): gegen die echten
// Regeln, annehmender und nachspielender Knoten im Gleichschritt (qsWelt).
// Die Faelschung: die Inhaberin legt beiden Stellen die WAHREN Bits vor --
// so, wie es nur geht, wenn Geloste ihr Wissen geteilt haben.

const fwGelost = 3 // so viele der Validatoren haben ein Pfand als Praeparatorin

// fwSetzen: das Register beider Knoten aendern (Los, Pfaender der Gelosten).
func (w *qsWelt) fwSetzen(f func(r *quantenWRegister)) {
	w.t.Helper()
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenWJetzt().kopie()
		f(reg)
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			w.t.Fatal(err)
		}
		cs.mu.Unlock()
	}
}

func (w *qsWelt) fwGuthaben(a string, mikro int64) {
	for _, cs := range []*ChainState{w.cs, w.nach} {
		acc, ok := cs.accounts.Get(a)
		if !ok {
			acc = &AccountState{Address: a}
			cs.accounts.Set(a, acc)
		}
		acc.Balance = Decimal(mikro)
	}
}

// neueFwWelt: ausgegeben, gebunden, gewaehlt; Los = die Validatoren, die
// ersten fwGelost mit 10 AEQ Pfand (im Pfandtopf); an Stelle 0 eingeloest.
func neueFwWelt(t *testing.T, aktiv bool) (*qsWelt, int64) {
	t.Helper()
	w := neueIPfandWelt(t, aktiv)
	z := qwT0
	w.bereit(z)
	w.fwSetzen(func(r *quantenWRegister) {
		r.Lose[w.p.K] = append([]string(nil), w.val...)
		for i := 0; i < fwGelost; i++ {
			a := w.val[i]
			r.Praep[quantenSPraepK(a)] = quantenSPraep{Konto: a, Pub: praepPubTest(w.valKeys[i]), Gemeldet: z - 100, Pfand: quantenSPraepPfand}
		}
	})
	w.fwGuthaben(quantenWPfandAddr, w.stand(w.cs, quantenWPfandAddr)+fwGelost*quantenSPraepPfand)
	w.einloesen(z + 50)
	return w, w.cs.quantenWJetzt().Tote[w.p.K]
}

func (w *qsWelt) fwEnde() int64 { return w.cs.quantenWJetzt().IPfand[w.p.K].Ende }

func (w *qsWelt) fwAnmeldenTx(von string, b int, x []int, vorlageVon string) Transaction {
	bs, xs := string(rune('0'+b)), qsBits(x)
	fehl := 0
	if n := w.cs.quantenWJetzt().Nachweis[w.p.K]; n != nil {
		fehl = n.Token.Fehl
	}
	vorlage := personalSign(w.t, w.keys[vorlageVon], quantenSVorlageText(w.p.K, b, x))
	return Transaction{Type: quantenSFaelschungAnmeldenTxArt, Wallet: von, QuantenW: &QuantenWAngaben{K: w.p.K, B: bs, X: xs, Probe: w.probe[b], Vorlage: vorlage,
		WalletSig: personalSign(w.t, w.keys[von], quantenSFaelschungAnmeldenNachricht(von, w.p.K, bs, xs, w.probe[b], fehl))}}
}

// fwOffenlegenAlle: B's Stichprobe in Teilen; das letzte Ergebnis.
func (w *qsWelt) fwOffenlegenAlle(zeit int64) QuantenWAngaben {
	w.t.Helper()
	var g QuantenWAngaben
	p := w.probe[1]
	for ab := 0; ab < len(p); ab += quantenWOffenTeil {
		bis := min(ab+quantenWOffenTeil, len(p))
		tx := w.offenTx(ab, p[ab:bis])
		tx.Type = quantenSFaelschungOffenlegenTxArt
		g = w.annehmen(tx, zeit).QuantenW.gerechnet()
		if bis < len(p) && g.Urteil != "teil" {
			w.t.Fatalf("Teil %d: %+v", ab, g)
		}
	}
	return g
}

// Schlafend: kein Nachweis-Eintrag, die Arten unbekannt.
func TestQuantenSFaelschung_Schlafend(t *testing.T) {
	w, _ := neueFwWelt(t, false)
	if len(w.cs.quantenWJetzt().Nachweis) != 0 || strings.Contains(w.cs.quantenWJetzt().kodiert(), "nachweis") {
		t.Fatal("schlafend: Nachweis angelegt")
	}
	for _, art := range []string{quantenSFaelschungAnmeldenTxArt, quantenSFaelschungOffenlegenTxArt} {
		if bekannteTxArt(art, qwT0) {
			t.Errorf("%s vor dem Stichtag bekannt", art)
		}
	}
	if r := w.praepRegel(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), qwT0+100); r != "quanten_s_pfand_inaktiv" {
		t.Fatalf("schlafend: %q", r)
	}
}

// Gutfall: die Faelschung wird nachgewiesen. Stelle B bekommt das Pfand der
// Inhaberin und je Geloster 10 % ihres Pfands; Geld bleibt erhalten.
func TestQuantenSFaelschung_Nachgewiesen(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	n := w.cs.quantenWJetzt().Nachweis[w.p.K]
	if n == nil || n.An != 0 || n.Ende != w.fwEnde() || n.Token.Anmeldung != nil || n.Token.Fehl != 0 {
		t.Fatalf("Nachweis-Eintrag: %+v", n)
	}
	if w.ansicht()["nachweis_bis"] != w.fwEnde()+quantenSNachweisFrist {
		t.Fatalf("Ansicht: %v", w.ansicht())
	}
	summe := func() int64 {
		s := w.summe(w.cs) + w.stand(w.cs, w.stelle[0]) + w.stand(w.cs, w.stelle[1])
		for _, a := range w.val {
			s += w.stand(w.cs, a)
		}
		return s
	}
	s0, b0 := summe(), w.stand(w.cs, w.stelle[1])
	z := w.fwEnde() + 100
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), z)
	g := w.fwOffenlegenAlle(z + 10)
	soll := ipDeckung + fwGelost*quantenSPraepPfand/10
	if g.Urteil != quantenSUrteilFaelschung || g.Pfand != soll || g.PfandAn != w.stelle[1] || w.stand(w.cs, w.stelle[1]) != b0+soll {
		t.Fatalf("Nachweis: %+v, B %d", g, w.stand(w.cs, w.stelle[1])-b0)
	}
	reg := w.cs.quantenWJetzt()
	if reg.Nachweis[w.p.K] != nil || len(reg.IPfand) != 0 {
		t.Fatal("Nachweis oder Pfand der Inhaberin nicht weg")
	}
	for i := 0; i < fwGelost; i++ {
		if p := reg.Praep[quantenSPraepK(w.val[i])].Pfand; p != quantenSPraepPfand-quantenSPraepPfand/10 {
			t.Fatalf("Geloste %d: Pfand %d", i, p)
		}
	}
	if summe() != s0 {
		t.Fatal("Geld nicht erhalten")
	}
	if _, err := quantenWRegisterLesen(reg.kodiert()); err != nil {
		t.Fatalf("Register nach dem Nachweis: %v", err)
	}
	// Ein Nachweis je K.
	if r := w.praepRegel(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), z+20); r != "quanten_s_faelschung_unbekannt" {
		t.Fatalf("zweiter Nachweis: %q", r)
	}
	if r := w.praepRegel(ipZurueckTx(w.p.K), z+quantenSNachweisFrist+100); r != "quanten_s_pfand_unbekannt" {
		t.Fatalf("Pfand nach dem Nachweis zurueck: %q", r)
	}
}

// Ehrliche Inhaberin: ihre echte Vorlage besteht B's Stichprobe nicht (die
// Physik) -- Fehlversuch, keine Strafe; nach quantenWFehlMax ist der Eintrag weg.
func TestQuantenSFaelschung_EhrlichKeinNachweis(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	z := w.fwEnde() + 100
	b0 := w.stand(w.cs, w.stelle[1])
	for i := 0; i < quantenWFehlMax; i++ {
		zi := z + int64(i)*100
		w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.x, w.anna), zi)
		if g := w.fwOffenlegenAlle(zi + 5); g.Urteil != "abgewiesen" || g.Pfand != 0 {
			t.Fatalf("Versuch %d: %+v", i, g)
		}
	}
	reg := w.cs.quantenWJetzt()
	if reg.Nachweis[w.p.K] != nil || reg.IPfand[w.p.K].Betrag != ipDeckung || w.stand(w.cs, w.stelle[1]) != b0 {
		t.Fatal("ehrlich: Strafe oder Eintrag geblieben")
	}
	for i := 0; i < fwGelost; i++ {
		if reg.Praep[quantenSPraepK(w.val[i])].Pfand != quantenSPraepPfand {
			t.Fatal("ehrlich: Pfand einer Gelosten genommen")
		}
	}
}

// Missbrauch beim Anmelden: falsche Stelle, fremde Vorlage, zu spaet.
func TestQuantenSFaelschung_AnmeldenMissbrauch(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	ende := w.fwEnde()
	z := ende + 100
	faelle := map[string]struct {
		tx   Transaction
		zeit int64
		soll string
	}{
		"Stelle A selbst":       {w.fwAnmeldenTx(w.stelle[0], 0, w.p.bits, w.anna), z, "quanten_w_s_stelle"},
		"A gibt sich als B aus": {w.fwAnmeldenTx(w.stelle[0], 1, w.p.bits, w.anna), z, "quanten_w_s_stelle"},
		"Fremder":               {w.fwAnmeldenTx(w.bert, 1, w.p.bits, w.anna), z, "quanten_w_s_stelle"},
		"Vorlage nicht von ihr": {w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.bert), z, "quanten_w_s_vorlage"},
		"nach der Frist":        {w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), ende + quantenSNachweisFrist + 1, "quanten_s_faelschung_frist"},
	}
	for name, f := range faelle {
		if r := w.praepRegel(f.tx, f.zeit); r != f.soll {
			t.Errorf("%s: %q, erwartet %q", name, r, f.soll)
		}
	}
	// Unterschrift von B ueber andere Bits als die Vorlage: abgewiesen.
	tx := w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna)
	tx.QuantenW.X = qsBits(w.x)
	if r := w.praepRegel(tx, z); r != "quanten_w_unterschrift" {
		t.Errorf("fremde Bits: %q", r)
	}
	// Am letzten Tag der Frist geht es noch.
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), ende+quantenSNachweisFrist)
	// Zurueckdatieren ueber das kurze Buchspiel hinaus: der Block ist ungueltig.
	if _, ok := quantenWBuchZeit(quantenSFaelschungAnmeldenTxArt, 1000-quantenSFaelschungBuchSpiel-1, 1000); ok {
		t.Error("Buchzeit 121 s vor dem Block angenommen")
	}
	if _, ok := quantenWBuchZeit(quantenSFaelschungOffenlegenTxArt, 1000-quantenSFaelschungBuchSpiel, 1000); !ok {
		t.Error("Buchzeit 120 s vor dem Block abgewiesen")
	}
}

// Bindung: eine offene Anmeldung haelt beide Pfaender ueber die Frist
// hinaus; verfaellt sie, ist das Pfand der Inhaberin frei (und der Eintrag
// weg). Das Los der Gelosten bleibt ueber das Aufraeumen der Toten stehen.
func TestQuantenSFaelschung_BindetPfaender(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	ende := w.fwEnde()
	letzt := ende + quantenSNachweisFrist
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), letzt)
	w.fwSetzen(func(r *quantenWRegister) {
		quantenWToteAufraeumen(r, ende+quantenWToteBehalten+1)
		e := r.Praep[quantenSPraepK(w.val[0])]
		e.Abgemeldet = ende - quantenSPraepHaltezeit
		e.Gemeldet = e.Abgemeldet - 10
		r.Praep[quantenSPraepK(w.val[0])] = e
	})
	reg := w.cs.quantenWJetzt()
	if _, tot := reg.Tote[w.p.K]; tot || len(reg.Lose[w.p.K]) == 0 {
		t.Fatal("Vorbedingung: Tote weg, Los da")
	}
	if _, err := quantenWRegisterLesen(reg.kodiert()); err != nil {
		t.Fatalf("Los fuer den Nachweis nicht lesbar: %v", err)
	}
	offen := letzt + quantenWFristOffen
	if r := w.praepRegel(ipZurueckTx(w.p.K), offen); r != "quanten_s_pfand_gebunden" {
		t.Fatalf("Pfand der Inhaberin bei offener Anmeldung: %q", r)
	}
	if r := w.praepRegel(pfZurueckTx(w.val[0]), offen); r != "quanten_s_pfand_gebunden" {
		t.Fatalf("Pfand der Gelosten bei offener Anmeldung: %q", r)
	}
	tx := w.annehmen(ipZurueckTx(w.p.K), offen+1)
	if tx.QuantenW.PfandAn != w.anna {
		t.Fatalf("nach dem Verfall: %+v", tx.QuantenW.gerechnet())
	}
	reg = w.cs.quantenWJetzt()
	if reg.Nachweis[w.p.K] != nil || reg.Lose[w.p.K] != nil {
		t.Fatal("Nachweis oder Los nach der Rueckgabe geblieben")
	}
	w.annehmen(pfZurueckTx(w.val[0]), offen+2)
}

// Ein Block mit falscher Strafe (Betrag, Empfaenger): abgewiesen.
func TestQuantenSFaelschung_GefaelschtImBlock(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	z := w.fwEnde() + 100
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), z)
	p := w.probe[1]
	for ab := 0; ab+quantenWOffenTeil < len(p); ab += quantenWOffenTeil {
		tx := w.offenTx(ab, p[ab:ab+quantenWOffenTeil])
		tx.Type = quantenSFaelschungOffenlegenTxArt
		w.annehmen(tx, z+5)
	}
	ab := (len(p) - 1) / quantenWOffenTeil * quantenWOffenTeil
	letzt := w.offenTx(ab, p[ab:])
	letzt.Type = quantenSFaelschungOffenlegenTxArt
	soll := ipDeckung + fwGelost*quantenSPraepPfand/10
	for name, f := range map[string]func(q *QuantenWAngaben){
		"an Stelle A": func(q *QuantenWAngaben) { q.Pfand, q.PfandAn, q.Urteil = soll, w.stelle[0], quantenSUrteilFaelschung },
		"mehr":        func(q *QuantenWAngaben) { q.Pfand, q.PfandAn, q.Urteil = soll+1, w.stelle[1], quantenSUrteilFaelschung },
		"ohne Strafe": func(q *QuantenWAngaben) { q.Urteil = "abgewiesen" },
		"Gelosten frei": func(q *QuantenWAngaben) {
			q.Pfand, q.PfandAn, q.Urteil = ipDeckung, w.stelle[1], quantenSUrteilFaelschung
		},
	} {
		tx := letzt
		q := *letzt.QuantenW
		f(&q)
		tx.QuantenW, tx.BuchAt = &q, z+6
		if err := w.nachspielen(tx, z+10); err == nil {
			t.Errorf("%s im Block angenommen", name)
		}
	}
	for name, f := range map[string]func(tx *Transaction){
		"Offenlegen mit Wallet": func(tx *Transaction) { tx.Wallet = w.stelle[1] },
		"Offenlegen mit Zeit":   func(tx *Transaction) { tx.DistributionAt = z },
		"Offenlegen mit B":      func(tx *Transaction) { tx.QuantenW.B = "1" },
	} {
		tx := letzt
		q := *letzt.QuantenW
		tx.QuantenW = &q
		f(&tx)
		if r := w.praepRegel(tx, z+6); r != "quanten_w_form" && r != "quanten_w_zeit" {
			t.Errorf("%s: %q", name, r)
		}
	}
	w.annehmen(letzt, z+6)
}

// Lesen: fail-closed.
func TestQuantenSFaelschung_RegisterLesen(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), w.fwEnde()+100)
	gut := w.cs.quantenWJetzt().form()
	if r, err := quantenWRegisterLesen(gut.kodiert()); err != nil || r.kodiert() != gut.kodiert() {
		t.Fatalf("Rundweg: %v", err)
	}
	if gut.zeilen()[quantenWZeile{quantenWArtNachweis, w.p.K}] == "" {
		t.Fatal("keine Zeile")
	}
	kaputt := map[string]func(f *quantenSNachweisForm){
		"An 2":             func(f *quantenSNachweisForm) { f.An = 2 },
		"Ende 0":           func(f *quantenSNachweisForm) { f.Ende = 0 },
		"Ende anders":      func(f *quantenSNachweisForm) { f.Ende++ },
		"K anders":         func(f *quantenSNachweisForm) { f.Token.K = strings.Repeat("e", 64) },
		"ungebunden":       func(f *quantenSNachweisForm) { f.Token.D = "" },
		"ungewaehlt":       func(f *quantenSNachweisForm) { f.Token.C = -1 },
		"fremde Inhaberin": func(f *quantenSNachweisForm) { f.Token.Inhaber = w.bert },
		"Fehl voll":        func(f *quantenSNachweisForm) { f.Token.Fehl = quantenWFehlMax },
		"Anmeldung von A":  func(f *quantenSNachweisForm) { f.An = 1 },
		"ohne Pfand dazu":  func(f *quantenSNachweisForm) { f.K = strings.Repeat("e", 64); f.Token.K = f.K },
	}
	for name, aendern := range kaputt {
		f := gut
		n := gut.Nachweis[0]
		a := *n.Token.Anmeldung
		n.Token.Anmeldung = &a
		aendern(&n)
		f.Nachweis = []quantenSNachweisForm{n}
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	f := gut
	f.Nachweis = make([]quantenSNachweisForm, quantenSIPfandHoechstens+1)
	for i := range f.Nachweis {
		f.Nachweis[i] = quantenSNachweisForm{K: fmt.Sprintf("%064x", i+1)}
	}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil || !strings.Contains(err.Error(), "hoechstens") {
		t.Errorf("ueber der Grenze: %v", err)
	}
}

// Ueber die echte Annahme: die Strafe zahlt an die vorher gesperrte Stelle B.
func TestQuantenSFaelschung_Annahme(t *testing.T) {
	w := neueIPfandWelt(t, true)
	for _, o := range []*atomic.Int64{&quantenWAbOverride, &quantenSWegBAbOverride, &quantenSLosBehaltenAbOverride, &quantenSPfaenderAbOverride} {
		o.Store(1)
	}
	z := nowUnix() - 86400
	w.bereit(z)
	w.einloesen(z + 50)
	fertig, err := w.cs.quantenWAnnehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna))
	if err != nil {
		t.Fatalf("Anmelden: %v", err)
	}
	if err := w.nachspielen(fertig, fertig.BuchAt+3); err != nil {
		t.Fatal(err)
	}
	p := w.probe[1]
	b0 := w.stand(w.cs, w.stelle[1])
	for ab := 0; ab < len(p); ab += quantenWOffenTeil {
		tx := w.offenTx(ab, p[ab:min(ab+quantenWOffenTeil, len(p))])
		tx.Type = quantenSFaelschungOffenlegenTxArt
		if fertig, err = w.cs.quantenWAnnehmen(tx); err != nil {
			t.Fatalf("Offenlegen %d: %v", ab, err)
		}
		if err := w.nachspielen(fertig, fertig.BuchAt+3); err != nil {
			t.Fatal(err)
		}
	}
	if fertig.QuantenW.Urteil != quantenSUrteilFaelschung || w.stand(w.cs, w.stelle[1]) != b0+ipDeckung {
		t.Fatalf("Annahme: %+v", fertig.QuantenW.gerechnet())
	}
}

// Tabelle gegen eine echte Datenbank: Nachweis mit offener Anmeldung als
// eigene Zeile; Neustart liest ihn zurueck (gleiche Wurzel).
func TestQuantenSFaelschung_Tabelle_RealDB(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), w.fwEnde()+100)
	reg := w.cs.quantenWJetzt()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-s-faelschung-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if err := qwtSetzen(t, cs, context.Background(), reg); err != nil {
		t.Fatal(err)
	}
	if z := qwtZeilen(t, cs)[quantenWArtNachweis+"|"+w.p.K]; z.daten == "" {
		t.Fatal("keine Zeile der Art nachweis")
	}
	neu := testKnoten(t, "unused-quanten-s-faelschung-realdb-test-2.json")
	if neu.quantenWWurzel() != cs.quantenWWurzel() || neu.quantenWJetzt().Nachweis[w.p.K].Token.Anmeldung == nil {
		t.Fatal("nach dem Neustart ein anderes Register")
	}
}

// Vor dem Stichtag der Pfaender gebunden (kein Pfand der Inhaberin), danach
// eingeloest: kein Nachweis -- er brachte B nichts, und ohne Pfand waere das
// Register nicht mehr lesbar.
func TestQuantenSFaelschung_OhnePfandKeinNachweis(t *testing.T) {
	w := neueIPfandWelt(t, false)
	z := qwT0
	w.bereit(z)
	quantenSPfaenderAbOverride.Store(z + 10)
	w.einloesen(z + 50)
	reg := w.cs.quantenWJetzt()
	if len(reg.Nachweis) != 0 || len(reg.IPfand) != 0 {
		t.Fatalf("ohne Pfand: %d Nachweise", len(reg.Nachweis))
	}
	if _, err := quantenWRegisterLesen(reg.kodiert()); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// Fehlversuche anderer vor dem Einloesen nehmen B keinen Versuch: der
// Nachweis-Eintrag beginnt bei 0.
func TestQuantenSFaelschung_FehlversucheNeu(t *testing.T) {
	w := neueIPfandWelt(t, true)
	z := qwT0
	w.bereit(z)
	w.annehmen(w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna])), z+20)
	if g := w.offenlegenAlle(1, z+25); g.Urteil != "abgewiesen" || w.token(w.cs).Fehl != 1 {
		t.Fatalf("Vorbedingung Fehlversuch: %+v", g)
	}
	w.einloesen(z + 50)
	if n := w.cs.quantenWJetzt().Nachweis[w.p.K]; n == nil || n.Token.Fehl != 0 {
		t.Fatalf("Nachweis nach einem Fehlversuch: %+v", n)
	}
}

// An Stelle 1 eingeloest: der Nachweis gehoert Stelle 0.
func TestQuantenSFaelschung_EingeloestAnStelle1(t *testing.T) {
	w := neueIPfandWelt(t, true)
	z := qwT0
	w.bereit(z)
	w.x = append([]int(nil), w.p.bits...) // besteht an jeder Stelle (wie eine Kopie)
	w.annehmen(w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna])), z+50)
	if g := w.offenlegenAlle(1, z+60); g.Urteil != "echt" || g.WertAn != w.stelle[1] {
		t.Fatalf("Einloesen an 1: %+v", g)
	}
	n := w.cs.quantenWJetzt().Nachweis[w.p.K]
	if n == nil || n.An != 1 {
		t.Fatalf("Nachweis: %+v", n)
	}
	if r := w.praepRegel(w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna), z+100); r != "quanten_w_s_stelle" {
		t.Fatalf("A=1 meldet selbst an: %q", r)
	}
	w.annehmen(w.fwAnmeldenTx(w.stelle[0], 0, w.p.bits, w.anna), z+100)
}

// Lesen ohne offene Anmeldung: dann tragen die Pruefungen des Eintrags selbst.
func TestQuantenSFaelschung_RegisterLesenOhneAnmeldung(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	gut := w.cs.quantenWJetzt().form()
	if len(gut.Nachweis) != 1 || gut.Nachweis[0].Token.Anmeldung != nil {
		t.Fatal("Vorbedingung")
	}
	for name, aendern := range map[string]func(f *quantenSNachweisForm){
		"An 2":       func(f *quantenSNachweisForm) { f.An = 2 },
		"An -1":      func(f *quantenSNachweisForm) { f.An = -1 },
		"K anders":   func(f *quantenSNachweisForm) { f.Token.K = strings.Repeat("e", 64) },
		"ungewaehlt": func(f *quantenSNachweisForm) { f.Token.C = -1 },
		"Fehl voll":  func(f *quantenSNachweisForm) { f.Token.Fehl = quantenWFehlMax },
	} {
		f := gut
		n := gut.Nachweis[0]
		aendern(&n)
		f.Nachweis = []quantenSNachweisForm{n}
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	// Form des Anmeldens: ohne Wallet, mit Zeit -- abgewiesen.
	for name, f := range map[string]func(tx *Transaction){
		"ohne Wallet": func(tx *Transaction) { tx.Wallet = "" },
		"mit Zeit":    func(tx *Transaction) { tx.DistributionAt = qwT0 },
	} {
		tx := w.fwAnmeldenTx(w.stelle[1], 1, w.p.bits, w.anna)
		f(&tx)
		if r := w.praepRegel(tx, w.fwEnde()+10); r == "" {
			t.Errorf("Anmelden %s angenommen", name)
		}
	}
}
