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

// Lesen eines beendeten Pfands (der Token ist weg): dann tragen allein die
// Formpruefungen. Und ein Pfand zu einem lebenden, aber ungebundenen Token.
func TestQuantenSIPfand_RegisterLesenBeendet(t *testing.T) {
	w := neueIPfandWelt(t, true)
	w.bereit(qwT0)
	w.einloesen(qwT0 + 50)
	gut := w.cs.quantenWJetzt().form()
	if len(gut.IPfand) != 1 || gut.IPfand[0].Ende == 0 || len(gut.Token) != 0 {
		t.Fatalf("Vorbedingung: %+v", gut.IPfand)
	}
	for name, aendern := range map[string]func(f *quantenSIPfandForm){
		"Betrag 0":        func(f *quantenSIPfandForm) { f.Betrag = 0 },
		"Betrag negativ":  func(f *quantenSIPfandForm) { f.Betrag = -1 },
		"Konto kaputt":    func(f *quantenSIPfandForm) { f.Konto = "0x12" },
		"Konto gross":     func(f *quantenSIPfandForm) { f.Konto = strings.ToUpper(f.Konto) },
		"Systemadresse":   func(f *quantenSIPfandForm) { f.Konto = ubiPoolAddr },
		"Ende negativ":    func(f *quantenSIPfandForm) { f.Ende = -1 },
		"N nicht dezimal": func(f *quantenSIPfandForm) { f.N = "abc" },
	} {
		f := gut
		p := gut.IPfand[0]
		aendern(&p)
		f.IPfand = []quantenSIPfandForm{p}
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Errorf("beendet, %s: angenommen", name)
		}
	}
	// Ungebunden: der Token lebt, ist aber nicht gebunden -- dazu kann es
	// kein laufendes Pfand geben.
	w2 := neueIPfandWelt(t, true)
	w2.annehmen(w2.ausgebenTx(qwT0, ipDeckung), qwT0+1)
	f := w2.cs.quantenWJetzt().form()
	f.IPfand = []quantenSIPfandForm{{K: w2.p.K, Betrag: ipDeckung, Konto: w2.anna}}
	if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
		t.Error("Pfand zu einem ungebundenen Token angenommen")
	}
}

// Nur noch ein Pfand im Register (Token und Tote weg): das Register ist
// nicht leer -- sonst fiele das Pfand aus Kodierung und StateRoot.
func TestQuantenSIPfand_NurPfandNichtLeer(t *testing.T) {
	_, konto := neuerSchluessel(t)
	r := neuesQuantenWRegister()
	r.IPfand[strings.Repeat("a", 64)] = quantenSIPfand{Betrag: ipDeckung, Konto: konto, Ende: 5}
	if r.leer() || r.kodiert() == "" {
		t.Fatal("Register mit Pfand gilt als leer")
	}
	g, err := quantenWRegisterLesen(r.kodiert())
	if err != nil || g.IPfand[strings.Repeat("a", 64)].Betrag != ipDeckung {
		t.Fatalf("Rundweg: %v", err)
	}
	if neuerQuantenWStand(r, r.kodiert()).wurzel == "" {
		t.Fatal("Pfand nicht in der StateRoot")
	}
}

// Tabelle gegen eine echte Datenbank: das Pfand als eigene Zeile, auch wenn
// Token und Tote weg sind; Neustart liest es zurueck.
func TestQuantenSIPfand_Tabelle_RealDB(t *testing.T) {
	w := neueIPfandWelt(t, true)
	w.bereit(qwT0)
	w.einloesen(qwT0 + 50)
	reg := w.cs.quantenWJetzt().kopie()
	quantenWToteAufraeumen(reg, reg.IPfand[w.p.K].Ende+quantenWToteBehalten+1)
	if len(reg.Tote) != 0 || len(reg.Token) != 0 || len(reg.IPfand) != 1 {
		t.Fatalf("Vorbedingung: %d Tote, %d Token, %d Pfaender", len(reg.Tote), len(reg.Token), len(reg.IPfand))
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-s-ipfand-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if err := qwtSetzen(t, cs, context.Background(), reg); err != nil {
		t.Fatal(err)
	}
	if z := qwtZeilen(t, cs)[quantenWArtIPfand+"|"+w.p.K]; z.daten == "" {
		t.Fatal("keine Zeile der Art ipfand")
	}
	neu := testKnoten(t, "unused-quanten-s-ipfand-realdb-test-2.json")
	if neu.quantenWWurzel() != cs.quantenWWurzel() || neu.quantenWJetzt().IPfand[w.p.K].Betrag != ipDeckung {
		t.Fatal("nach dem Neustart ein anderes Register")
	}
}

// ipGeberTx: die Bindung, das Pfand gibt von (mit eigener Unterschrift).
func (w *qsWelt) ipGeberTx(von string, sigVon string, K, inhaber string, betrag int64) Transaction {
	tx := w.bindenTx(w.anna)
	tx.QuantenW.PfandVon = von
	tx.QuantenW.PfandSig = personalSign(w.t, w.keys[sigVon], quantenSIPfandNachricht(von, K, inhaber, betrag))
	return tx
}

// Pfandgeber: die Menschen-Wallet gibt das Pfand fuer den frischen Schluessel
// der Inhaberin; es gehoert ihr (Konto, Nullifier) und geht an sie zurueck.
func TestQuantenSIPfand_Pfandgeber(t *testing.T) {
	w := neueIPfandWelt(t, true)
	z := qwT0
	w.nullifier(pfNBert, w.bert)
	w.annehmen(w.ausgebenTx(z, ipDeckung), z+1)
	anna0, bert0 := w.stand(w.cs, w.anna), w.stand(w.cs, w.bert)
	tx := w.annehmen(w.ipGeberTx(w.bert, w.bert, w.p.K, w.anna, ipDeckung), z+2)
	p := w.cs.quantenWJetzt().IPfand[w.p.K]
	if tx.QuantenW.Pfand != ipDeckung || p.Konto != w.bert || p.N != pfNBert || w.stand(w.cs, w.bert) != bert0-ipDeckung || w.stand(w.cs, w.anna) != anna0 {
		t.Fatalf("Pfandgeber: %+v, Bert %d, Anna %d", p, w.stand(w.cs, w.bert)-bert0, w.stand(w.cs, w.anna)-anna0)
	}
	if _, err := quantenWRegisterLesen(w.cs.quantenWJetzt().kodiert()); err != nil {
		t.Fatalf("Register mit Pfandgeber: %v", err)
	}
	w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+3)
	w.einloesen(z + 50)
	tx = w.annehmen(ipZurueckTx(w.p.K), w.fwEnde()+quantenSNachweisFrist+1)
	if tx.QuantenW.PfandAn != w.bert || w.stand(w.cs, w.bert) != bert0 {
		t.Fatalf("Rueckgabe an %s", tx.QuantenW.PfandAn)
	}
}

// Missbrauch: fremde, falsche oder fehlende Unterschrift, Systemadresse, die
// Inhaberin selbst, zu wenig Deckung, vor dem Stichtag.
func TestQuantenSIPfand_PfandgeberMissbrauch(t *testing.T) {
	w := neueIPfandWelt(t, true)
	z := qwT0
	w.annehmen(w.ausgebenTx(z, ipDeckung), z+1)
	faelle := map[string]struct {
		tx   Transaction
		soll string
	}{
		"fremde Unterschrift": {w.ipGeberTx(w.bert, w.praeger, w.p.K, w.anna, ipDeckung), "quanten_w_unterschrift"},
		"anderes K":           {w.ipGeberTx(w.bert, w.bert, strings.Repeat("e", 64), w.anna, ipDeckung), "quanten_w_unterschrift"},
		"andere Inhaberin":    {w.ipGeberTx(w.bert, w.bert, w.p.K, w.praeger, ipDeckung), "quanten_w_unterschrift"},
		"anderer Betrag":      {w.ipGeberTx(w.bert, w.bert, w.p.K, w.anna, ipDeckung-1), "quanten_w_unterschrift"},
		"Inhaberin selbst":    {w.ipGeberTx(w.anna, w.anna, w.p.K, w.anna, ipDeckung), "quanten_w_form"},
	}
	ohneSig := w.ipGeberTx(w.bert, w.bert, w.p.K, w.anna, ipDeckung)
	ohneSig.QuantenW.PfandSig = ""
	faelle["ohne Unterschrift"] = struct {
		tx   Transaction
		soll string
	}{ohneSig, "quanten_w_unterschrift"}
	sys := w.ipGeberTx(w.bert, w.bert, w.p.K, w.anna, ipDeckung)
	sys.QuantenW.PfandVon = ubiPoolAddr
	faelle["Systemadresse"] = struct {
		tx   Transaction
		soll string
	}{sys, "quanten_w_form"}
	nurSig := w.ipGeberTx(w.bert, w.bert, w.p.K, w.anna, ipDeckung)
	nurSig.QuantenW.PfandVon = ""
	faelle["Unterschrift ohne Pfandgeber"] = struct {
		tx   Transaction
		soll string
	}{nurSig, "quanten_w_form"}
	for name, f := range faelle {
		if r := w.praepRegel(f.tx, z+2); r != f.soll {
			t.Errorf("%s: %q, erwartet %q", name, r, f.soll)
		}
	}
	w.fwGuthaben(w.bert, ipDeckung-1)
	if r := w.praepRegel(w.ipGeberTx(w.bert, w.bert, w.p.K, w.anna, ipDeckung), z+2); r != "quanten_w_deckung" {
		t.Errorf("Pfandgeber ohne Deckung: %q", r)
	}
	// Vor dem Stichtag: Felder des Pfandgebers abgewiesen, nicht uebergangen.
	w2 := neueIPfandWelt(t, false)
	w2.annehmen(w2.ausgebenTx(z, ipDeckung), z+1)
	if r := w2.praepRegel(w2.ipGeberTx(w2.bert, w2.bert, w2.p.K, w2.anna, ipDeckung), z+2); r != "quanten_w_form" {
		t.Errorf("Pfandgeber vor dem Stichtag: %q", r)
	}
}

// Die Ansicht des Tokens nennt die Deckung (Betrag, den der Pfandgeber unterschreibt).
func TestQuantenSIPfand_AnsichtDeckung(t *testing.T) {
	w := neueIPfandWelt(t, false)
	w.annehmen(w.ausgebenTx(qwT0, ipDeckung), qwT0+1)
	if d, _ := w.ansicht()["deckung"].(int64); d != ipDeckung {
		t.Fatalf("deckung in der Ansicht: %v", w.ansicht()["deckung"])
	}
}
