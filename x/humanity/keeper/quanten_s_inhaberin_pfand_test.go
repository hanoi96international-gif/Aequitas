package keeper

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
)

// P3, Pfand der Inhaberin (quanten_s_inhaberin_pfand.go): gegen die echten
// Regeln, annehmender und nachspielender Knoten im Gleichschritt (qsWelt).

const ipDeckung int64 = 10_000_000 // wie qsWelt.bereit

func neueIPfandWelt(t *testing.T, aktiv bool) *qsWelt {
	t.Helper()
	w := neueQsWelt(t, 128, 80)
	ab := int64(math.MaxInt64)
	if aktiv {
		ab = qwT0 - 30*86400
	}
	for _, o := range []*atomic.Int64{&quantenSWegBAbOverride, &quantenSLosBehaltenAbOverride} {
		o.Store(qwT0 - 30*86400)
	}
	quantenSPfaenderAbOverride.Store(ab)
	t.Cleanup(func() {
		for _, o := range []*atomic.Int64{&quantenSWegBAbOverride, &quantenSLosBehaltenAbOverride, &quantenSPfaenderAbOverride} {
			o.Store(0)
		}
	})
	qwuAktiv(t)
	w.nullifier(pfNAnna, w.anna)
	return w
}

func ipZurueckTx(K string) Transaction {
	return Transaction{Type: quantenSIPfandZurueckTxArt, QuantenW: &QuantenWAngaben{K: K}}
}

// Schlafend: die Bindung nimmt nichts, das Register traegt kein Pfand.
func TestQuantenSIPfand_Schlafend(t *testing.T) {
	w := neueIPfandWelt(t, false)
	anna := w.stand(w.cs, w.anna)
	w.bereit(qwT0)
	if w.stand(w.cs, w.anna) != anna || len(w.cs.quantenWJetzt().IPfand) != 0 || strings.Contains(w.cs.quantenWJetzt().kodiert(), "ipfand") {
		t.Fatal("schlafend: Pfand genommen")
	}
	if bekannteTxArt(quantenSIPfandZurueckTxArt, qwT0) {
		t.Error("Rueckgabe vor dem Stichtag bekannt")
	}
	if r := w.praepRegel(ipZurueckTx(w.p.K), qwT0+10); r != "quanten_s_pfand_inaktiv" {
		t.Fatalf("Rueckgabe schlafend: %q", r)
	}
}

// Gutfall: die Bindung nimmt D; nach dem Ende und der Frist geht es zurueck;
// Geld bleibt erhalten; das Pfand ueberlebt das Aufraeumen der Toten.
func TestQuantenSIPfand_Gutfall(t *testing.T) {
	for _, tot := range []bool{false, true} {
		w := neueIPfandWelt(t, true)
		z := qwT0
		summe := func() int64 { return w.summe(w.cs) + w.stand(w.cs, w.stelle[0]) + w.stand(w.cs, w.stelle[1]) }
		summe0 := summe()
		anna0 := w.stand(w.cs, w.anna)
		w.annehmen(w.ausgebenTx(z, ipDeckung), z+1)
		tx := w.annehmen(w.bindenTx(w.anna), z+2)
		p := w.cs.quantenWJetzt().IPfand[w.p.K]
		if tx.QuantenW.Pfand != ipDeckung || p.Betrag != ipDeckung || p.Konto != w.anna || p.N != pfNAnna || p.Ende != 0 ||
			w.stand(w.cs, w.anna) != anna0-ipDeckung {
			t.Fatalf("Bindung: %+v, Anna %d", p, w.stand(w.cs, w.anna))
		}
		if w.ansicht()["pfand_inhaberin"] != ipDeckung {
			t.Fatalf("Ansicht: %v", w.ansicht())
		}
		if r := w.praepRegel(ipZurueckTx(w.p.K), z+5); r != "quanten_s_pfand_gebunden" {
			t.Fatalf("laufend zurueck: %q", r)
		}
		w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+3)
		if tot {
			w.totMachen(z + 50)
		} else {
			w.einloesen(z + 50)
		}
		ende := w.cs.quantenWJetzt().IPfand[w.p.K].Ende
		if ende == 0 || ende != w.cs.quantenWJetzt().Enden[w.p.K].Ende {
			t.Fatalf("Ende nicht festgehalten: %d", ende)
		}
		if r := w.praepRegel(ipZurueckTx(w.p.K), ende+quantenSNachweisFrist); r != "quanten_s_pfand_frist" {
			t.Fatalf("am letzten Tag der Frist: %q", r)
		}
		// Die Toten werden aufgeraeumt -- das Pfand bleibt, das Register lesbar.
		for _, cs := range []*ChainState{w.cs, w.nach} {
			reg := cs.quantenWJetzt().kopie()
			quantenWToteAufraeumen(reg, ende+quantenWToteBehalten+1)
			if _, da := reg.Tote[w.p.K]; da {
				t.Fatal("Vorbedingung: nicht aufgeraeumt")
			}
			if _, err := quantenWRegisterLesen(reg.kodiert()); err != nil || reg.IPfand[w.p.K].Betrag != ipDeckung {
				t.Fatalf("nach dem Aufraeumen: %v", err)
			}
			cs.mu.Lock()
			if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
				t.Fatal(err)
			}
			cs.mu.Unlock()
		}
		vorher := w.stand(w.cs, w.anna)
		zz := ende + quantenWToteBehalten + 2
		tx = w.annehmen(ipZurueckTx(w.p.K), zz)
		if tx.QuantenW.PfandAn != w.anna || w.stand(w.cs, w.anna) != vorher+ipDeckung || len(w.cs.quantenWJetzt().IPfand) != 0 {
			t.Fatalf("Rueckgabe: %+v", tx.QuantenW.gerechnet())
		}
		if summe() != summe0 {
			t.Fatal("Geld nicht erhalten")
		}
		if r := w.praepRegel(ipZurueckTx(w.p.K), zz+1); r != "quanten_s_pfand_unbekannt" {
			t.Fatalf("zweite Rueckgabe: %q", r)
		}
	}
}

// Ohne Deckung fuer das Pfand: keine Bindung.
func TestQuantenSIPfand_OhneDeckung(t *testing.T) {
	w := neueIPfandWelt(t, true)
	w.annehmen(w.ausgebenTx(qwT0, ipDeckung), qwT0+1)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		a, _ := cs.accounts.Get(w.anna)
		a.Balance = Decimal(ipDeckung - 1)
	}
	if r := w.praepRegel(w.bindenTx(w.anna), qwT0+2); r != "quanten_w_deckung" {
		t.Fatalf("ohne Deckung: %q", r)
	}
}

// Umzug: das Pfand geht an die neue Wallet; Mensch ohne Wallet: nichts.
func TestQuantenSIPfand_Umzug(t *testing.T) {
	w := neueIPfandWelt(t, true)
	z := qwT0
	w.bereit(z)
	w.einloesen(z + 50)
	ende := w.cs.quantenWJetzt().IPfand[w.p.K].Ende
	_, neu := neuerSchluessel(t)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		addHuman(cs, neu, 0)
	}
	w.nullifier(pfNAnna, neu)
	tx := w.annehmen(ipZurueckTx(w.p.K), ende+quantenSNachweisFrist+1)
	if tx.QuantenW.PfandAn != neu || w.stand(w.cs, neu) != ipDeckung {
		t.Fatalf("nach dem Umzug an %s", tx.QuantenW.PfandAn)
	}
	w2 := neueIPfandWelt(t, true)
	w2.bereit(z)
	w2.einloesen(z + 50)
	w2.nullifier(pfNAnna, "")
	if r := w2.praepRegel(ipZurueckTx(w2.p.K), ende+quantenSNachweisFrist+1); r != "quanten_s_pfand_empfaenger" {
		t.Fatalf("Mensch ohne Wallet: %q", r)
	}
}

// Ein Block mit falschem Pfand bei der Bindung oder fremdem Empfaenger der
// Rueckgabe wird abgewiesen; Form der Rueckgabe streng.
func TestQuantenSIPfand_GefaelschtImBlock(t *testing.T) {
	w := neueIPfandWelt(t, true)
	z := qwT0
	w.annehmen(w.ausgebenTx(z, ipDeckung), z+1)
	binden := w.bindenTx(w.anna)
	for _, pfand := range []int64{0, ipDeckung - 1, ipDeckung + 1} {
		tx := binden
		q := *tx.QuantenW
		q.Pfand = pfand
		tx.QuantenW, tx.BuchAt = &q, z+2
		if err := w.nachspielen(tx, z+9); err == nil {
			t.Errorf("Bindung mit Pfand %d im Block angenommen", pfand)
		}
	}
	w.annehmen(binden, z+2)
	w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+3)
	w.einloesen(z + 50)
	zz := w.cs.quantenWJetzt().IPfand[w.p.K].Ende + quantenSNachweisFrist + 1
	gut := ipZurueckTx(w.p.K)
	for name, f := range map[string]func(q *QuantenWAngaben){
		"fremder Empfaenger": func(q *QuantenWAngaben) { q.Pfand, q.PfandAn = ipDeckung, w.bert },
		"mehr Pfand":         func(q *QuantenWAngaben) { q.Pfand, q.PfandAn = ipDeckung+1, w.anna },
	} {
		tx := gut
		q := *gut.QuantenW
		f(&q)
		tx.QuantenW, tx.BuchAt = &q, zz
		if err := w.nachspielen(tx, zz+7); err == nil {
			t.Errorf("%s im Block angenommen", name)
		}
	}
	for name, f := range map[string]func(tx *Transaction){
		"mit Wallet": func(tx *Transaction) { tx.Wallet = w.anna },
		"mit Zeit":   func(tx *Transaction) { tx.DistributionAt = zz },
		"mit Index":  func(tx *Transaction) { tx.QuantenW.Index = 1 },
		"mit D":      func(tx *Transaction) { tx.QuantenW.D = "00" },
	} {
		tx := gut
		q := *gut.QuantenW
		tx.QuantenW = &q
		f(&tx)
		if r := w.praepRegel(tx, zz); r != "quanten_w_form" && r != "quanten_w_zeit" {
			t.Errorf("%s: %q", name, r)
		}
	}
	w.annehmen(gut, zz)
}

// Voll: keine Bindung mehr, solange die Grenze erreicht ist.
func TestQuantenSIPfand_Voll(t *testing.T) {
	w := neueIPfandWelt(t, true)
	w.annehmen(w.ausgebenTx(qwT0, ipDeckung), qwT0+1)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenWJetzt().kopie()
		for i := 0; len(reg.IPfand) < quantenSIPfandHoechstens; i++ {
			reg.IPfand[fmt.Sprintf("%064x", i+1)] = quantenSIPfand{Betrag: 1, Konto: w.bert, Ende: 5}
		}
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	if r := w.praepRegel(w.bindenTx(w.anna), qwT0+2); r != "quanten_w_voll" {
		t.Fatalf("volles Register: %q", r)
	}
}

// Dasselbe K neu ausgeben, solange das alte Pfand liegt: abgewiesen.
func TestQuantenSIPfand_KNichtWiederverwenden(t *testing.T) {
	w := neueIPfandWelt(t, true)
	z := qwT0
	w.bereit(z)
	w.einloesen(z + 50)
	ende := w.cs.quantenWJetzt().IPfand[w.p.K].Ende
	spaet := ende + quantenWToteBehalten + 100
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenWJetzt().kopie()
		quantenWToteAufraeumen(reg, spaet)
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	if r := w.praepRegel(w.ausgebenTx(spaet, ipDeckung), spaet+1); r != "quanten_w_doppelt" {
		t.Fatalf("K mit liegendem Pfand neu ausgegeben: %q", r)
	}
}

// Lesen: fail-closed.
func TestQuantenSIPfand_RegisterLesen(t *testing.T) {
	w := neueIPfandWelt(t, true)
	w.bereit(qwT0)
	gut := w.cs.quantenWJetzt().form()
	if r, err := quantenWRegisterLesen(gut.kodiert()); err != nil || r.kodiert() != gut.kodiert() {
		t.Fatalf("Rundweg: %v", err)
	}
	if gut.zeilen()[quantenWZeile{quantenWArtIPfand, w.p.K}] == "" {
		t.Fatal("keine Zeile")
	}
	kaputt := map[string]func(f *quantenSIPfandForm){
		"Betrag != D":   func(f *quantenSIPfandForm) { f.Betrag-- },
		"Betrag 0":      func(f *quantenSIPfandForm) { f.Betrag = 0 },
		"fremdes Konto": func(f *quantenSIPfandForm) { f.Konto = w.bert },
		"Konto kaputt":  func(f *quantenSIPfandForm) { f.Konto = "0x12" },
		"Systemadresse": func(f *quantenSIPfandForm) { f.Konto = ubiPoolAddr },
		"N kaputt":      func(f *quantenSIPfandForm) { f.N = "x" },
		"Ende negativ":  func(f *quantenSIPfandForm) { f.Ende = -1 },
		"beendet, lebt": func(f *quantenSIPfandForm) { f.Ende = 5 },
		"K ohne Token":  func(f *quantenSIPfandForm) { f.K = strings.Repeat("e", 64) },
		"K kein Hash":   func(f *quantenSIPfandForm) { f.K = "abc" },
		"ungebunden":    func(f *quantenSIPfandForm) {},
	}
	for name, aendern := range kaputt {
		f := gut
		p := gut.IPfand[0]
		aendern(&p)
		f.IPfand = []quantenSIPfandForm{p}
		if name == "ungebunden" {
			f.Token = append([]quantenWSTokenForm(nil), gut.Token...)
			f.Token[0].D = ""
		}
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	f := gut
	f.IPfand = []quantenSIPfandForm{gut.IPfand[0], gut.IPfand[0]}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
		t.Error("doppelt angenommen")
	}
	f = gut
	f.IPfand = make([]quantenSIPfandForm, quantenSIPfandHoechstens+1)
	for i := range f.IPfand {
		f.IPfand[i] = quantenSIPfandForm{K: fmt.Sprintf("%064x", i+1), Betrag: 1, Konto: w.bert, Ende: 1}
	}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil || !strings.Contains(err.Error(), "hoechstens") {
		t.Errorf("ueber der Grenze: %v", err)
	}
}

// Ueber die echte Annahme: die Rueckgabe sperrt den Empfaenger vorher.
func TestQuantenSIPfand_Annahme(t *testing.T) {
	w := neueIPfandWelt(t, true)
	for _, o := range []*atomic.Int64{&quantenWAbOverride, &quantenSWegBAbOverride, &quantenSLosBehaltenAbOverride, &quantenSPfaenderAbOverride} {
		o.Store(1)
	}
	// Mit der echten Uhr: ausgegeben und eingeloest vor 20 Tagen.
	z := nowUnix() - 20*86400
	w.bereit(z)
	w.einloesen(z + 50)
	_, neu := neuerSchluessel(t)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		addHuman(cs, neu, 0)
	}
	w.nullifier(pfNAnna, neu)
	fertig, err := w.cs.quantenWAnnehmen(ipZurueckTx(w.p.K))
	if err != nil || fertig.QuantenW.PfandAn != neu || w.stand(w.cs, neu) != ipDeckung {
		t.Fatalf("Annahme: %v %+v", err, fertig.QuantenW)
	}
	if err := w.nachspielen(fertig, fertig.BuchAt+3); err != nil {
		t.Fatalf("Nachspielen: %v", err)
	}
}
