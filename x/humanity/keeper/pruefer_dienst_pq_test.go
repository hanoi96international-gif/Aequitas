package keeper

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Audit B2: PQ-Schluessel der Pruefer-Dienste (pruefer_dienst_pq.go).

type pdFall struct {
	f      *vrFall
	mensch prueferPQSchluessel // ML-DSA des Menschen (Zusage)
	dienst prueferPQSchluessel // ML-DSA des Dienstes
	ed     string              // Ed25519 des Dienstes
	zeit   int64
}

func neuerPdFall(t *testing.T) *pdFall {
	t.Helper()
	f := neuerVrFall(t)
	z := vrAb + 1000
	vorher := prueferDienstPQAbOverride.Load()
	prueferDienstPQAbOverride.Store(vrAb)
	t.Cleanup(func() { prueferDienstPQAbOverride.Store(vorher) })
	p := &pdFall{f: f, mensch: neuerPrueferPQ(t), dienst: neuerPrueferPQ(t), ed: neuerEd25519(t).pub, zeit: z}
	zusageEintragen(f.cs, f.m(0), p.mensch)
	return p
}

func (p *pdFall) tx(t *testing.T) Transaction {
	t.Helper()
	m := p.f.m(0)
	h, _ := sha256Hex(p.dienst.pub)
	return Transaction{Type: prueferDienstPQTxArt, Wallet: m, DistributionAt: p.zeit, DienstPQ: &PrueferDienstPQ{
		Ed: p.ed, Pub: p.dienst.pub,
		BesitzSig: p.dienst.unterschreiben(t, prueferDienstBesitzNachricht(m, p.ed)),
		MenschPub: p.mensch.pub,
		MenschSig: p.mensch.unterschreiben(t, prueferDienstFreigabeNachricht(m, p.ed, h, p.zeit)),
	}}
}

func (p *pdFall) urteil(tx Transaction, blockZeit int64) prueferDienstUrteil {
	p.f.cs.mu.Lock()
	defer p.f.cs.mu.Unlock()
	return p.f.cs.prueferDienstPQUrteilLocked(&tx, blockZeit)
}

func TestPrueferDienstPQ_GutfallUndRegister(t *testing.T) {
	p := neuerPdFall(t)
	tx := p.tx(t)
	if u := p.urteil(tx, p.zeit); u.regel != "" {
		t.Fatalf("gueltiger Eintrag abgewiesen: %s %s", u.regel, u.text)
	}
	p.f.cs.mu.Lock()
	err := p.f.cs.applyPrueferDienstPQLocked(context.Background(), &tx, p.zeit)
	p.f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	reg := p.f.cs.verifierRegisterJetzt()
	if _, ok := reg.dienstPQ(p.ed, p.f.m(0), p.zeit+erzeugerFrist-1); ok {
		t.Fatal("wirkt vor issued_at + erzeugerFrist")
	}
	if pub, ok := reg.dienstPQ(p.ed, p.f.m(0), p.zeit+erzeugerFrist); !ok || pub != p.dienst.pub {
		t.Fatal("wirkt nicht ab issued_at + erzeugerFrist")
	}
	if _, ok := reg.dienstPQ(p.ed, p.f.m(1), p.zeit+erzeugerFrist); ok {
		t.Fatal("gilt fuer einen anderen Menschen")
	}
	// Kanonisch, wieder lesbar, gleicher Hash.
	zurueck, err := verifierRegisterLesen(reg.kodiert())
	if err != nil || zurueck.pqHash != reg.pqHash || zurueck.pqHash == "" {
		t.Fatalf("Register nicht wieder lesbar: %v", err)
	}
	// Ein zweites Mal derselbe Schluessel: fest.
	if u := p.urteil(p.tx(t), p.zeit); u.regel != "pruefer_pq_fest" {
		t.Fatalf("zweiter Eintrag fuer denselben Schluessel: %q", u.regel)
	}
	// Eine Aenderung im Pruefer-Register behaelt die PQ-Eintraege.
	p.f.aufnehmen(1, p.zeit+10, 0)
	if _, ok := p.f.cs.verifierRegisterJetzt().dienstPQ(p.ed, p.f.m(0), p.zeit+erzeugerFrist); !ok {
		t.Fatal("Aufnahme eines Pruefers hat die PQ-Eintraege geloescht")
	}
}

func TestPrueferDienstPQ_Missbrauch(t *testing.T) {
	p := neuerPdFall(t)
	m := p.f.m(0)
	h, _ := sha256Hex(p.dienst.pub)
	faelle := map[string]struct {
		aendern func(tx *Transaction)
		regel   string
	}{
		"fremder Mensch-Schluessel (nicht die Zusage)": {func(tx *Transaction) {
			x := neuerPrueferPQ(t)
			tx.DienstPQ.MenschPub = x.pub
			tx.DienstPQ.MenschSig = x.unterschreiben(t, prueferDienstFreigabeNachricht(m, p.ed, h, p.zeit))
		}, "pruefer_pq_zusage"},
		"Freigabe fuer anderen Dienst-Schluessel": {func(tx *Transaction) {
			x := neuerPrueferPQ(t)
			tx.DienstPQ.Pub = x.pub
			tx.DienstPQ.BesitzSig = x.unterschreiben(t, prueferDienstBesitzNachricht(m, p.ed))
		}, "pruefer_pq_freigabe"},
		"Freigabe zu anderer Zeit": {func(tx *Transaction) {
			tx.DienstPQ.MenschSig = p.mensch.unterschreiben(t, prueferDienstFreigabeNachricht(m, p.ed, h, p.zeit-1))
		}, "pruefer_pq_freigabe"},
		"Freigabe fuer anderen Ed25519-Schluessel": {func(tx *Transaction) {
			tx.DienstPQ.MenschSig = p.mensch.unterschreiben(t, prueferDienstFreigabeNachricht(m, neuerEd25519(t).pub, h, p.zeit))
		}, "pruefer_pq_freigabe"},
		"Besitz fuer anderen Menschen": {func(tx *Transaction) {
			tx.DienstPQ.BesitzSig = p.dienst.unterschreiben(t, prueferDienstBesitzNachricht(p.f.m(1), p.ed))
		}, "pruefer_pq_besitz"},
		"Besitz fremder Schluessel": {func(tx *Transaction) {
			tx.DienstPQ.BesitzSig = neuerPrueferPQ(t).unterschreiben(t, prueferDienstBesitzNachricht(m, p.ed))
		}, "pruefer_pq_besitz"},
		"kein Pruefer": {func(tx *Transaction) { tx.Wallet = p.f.m(2) }, "pruefer_pq_kein_pruefer"},
		"Grossschrift": {func(tx *Transaction) { tx.DienstPQ.Pub = strings.ToUpper(tx.DienstPQ.Pub) }, "pruefer_pq_form"},
		"Unterschrift abgeschnitten": {func(tx *Transaction) {
			tx.DienstPQ.MenschSig = tx.DienstPQ.MenschSig[:len(tx.DienstPQ.MenschSig)-2]
		}, "pruefer_pq_form"},
		"Betrag":                  {func(tx *Transaction) { tx.Amount = 1 }, "pruefer_pq_form"},
		"Zeit weit vor dem Block": {func(tx *Transaction) {}, "pruefer_pq_zeit"},
	}
	for name, fa := range faelle {
		tx := p.tx(t)
		fa.aendern(&tx)
		blockZeit := p.zeit
		if name == "Zeit weit vor dem Block" {
			blockZeit = p.zeit + nachweisHoechstensAlt + 1
		}
		if u := p.urteil(tx, blockZeit); u.regel != fa.regel {
			t.Errorf("%s: %q (%s), erwartet %q", name, u.regel, u.text, fa.regel)
		}
	}
	// Vor dem Stichtag: unbekannt.
	prueferDienstPQAbOverride.Store(p.zeit + 1)
	if u := p.urteil(p.tx(t), p.zeit); u.regel != "pruefer_pq_form" {
		t.Fatalf("vor dem Stichtag: %q", u.regel)
	}
	if bekannteTxArt(prueferDienstPQTxArt, p.zeit) {
		t.Fatal("Art vor dem Stichtag bekannt")
	}
	prueferDienstPQAbOverride.Store(vrAb)
	// Ohne Zusage.
	q := neuerPdFall(t)
	q.f.cs.umzugReg.Store(&umzugRegister{})
	if u := q.urteil(q.tx(t), q.zeit); u.regel != "pruefer_pq_zusage" {
		t.Fatalf("ohne Zusage: %q", u.regel)
	}
	// Genesis-Schluessel.
	g := neuerPdFall(t)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 1,
		Coordinator: []string{g.ed}, Bezeuger: []string{neuerEd25519(t).pub}})
	if u := g.urteil(g.tx(t), g.zeit); u.regel != "pruefer_pq_genesis" {
		t.Fatalf("Genesis-Schluessel: %q", u.regel)
	}
}

func TestPrueferDienstPQ_HoechstensZweiJeMensch(t *testing.T) {
	p := neuerPdFall(t)
	for i := 0; i < prueferDienstPQJeMensch; i++ {
		p.ed, p.dienst = neuerEd25519(t).pub, neuerPrueferPQ(t)
		tx := p.tx(t)
		p.f.cs.mu.Lock()
		err := p.f.cs.applyPrueferDienstPQLocked(context.Background(), &tx, p.zeit)
		p.f.cs.mu.Unlock()
		if err != nil {
			t.Fatalf("Eintrag %d: %v", i, err)
		}
	}
	p.ed, p.dienst = neuerEd25519(t).pub, neuerPrueferPQ(t)
	if u := p.urteil(p.tx(t), p.zeit); u.regel != "pruefer_pq_voll" {
		t.Fatalf("dritter Dienst: %q", u.regel)
	}
}

// StateRoot: ohne PQ-Eintraege dieselbe Schreibweise wie vorher; mit ihnen
// nur der Hash -- und ein anderer Schluessel ergibt eine andere Wurzel.
func TestPrueferDienstPQ_StateRoot(t *testing.T) {
	r := (&verifierRegister{}).mitSetzen(verifierEintrag{Mensch: "0x" + strings.Repeat("1", 40), Seit: 5, Zuletzt: 5})
	if r.kodiertOhnePQ() != r.kodiert() || r.pqHash != "" {
		t.Fatal("ohne PQ nicht byte-gleich")
	}
	a := r.mitDienstPQ(prueferDienstPQ{Ed: strings.Repeat("a", 64), Mensch: "0x" + strings.Repeat("1", 40), Pub: strings.Repeat("b", pqPubHex), Wirkung: 9})
	b := r.mitDienstPQ(prueferDienstPQ{Ed: strings.Repeat("a", 64), Mensch: "0x" + strings.Repeat("1", 40), Pub: strings.Repeat("c", pqPubHex), Wirkung: 9})
	if a.pqHash == "" || a.pqHash == b.pqHash || a.kodiertOhnePQ() != r.kodiert() {
		t.Fatal("PQ-Feld nicht ueber den Hash in der Wurzel")
	}
	if len(a.kodiert()) < pqPubHex {
		t.Fatal("chain_config traegt den Schluessel nicht")
	}
	// Ein Register nur mit PQ-Eintraegen (Gruender) ist nicht leer.
	nur := (&verifierRegister{}).mitDienstPQ(prueferDienstPQ{Ed: strings.Repeat("a", 64), Mensch: "0x" + strings.Repeat("1", 40), Pub: strings.Repeat("b", pqPubHex), Wirkung: 9})
	if nur.leer() {
		t.Fatal("Register mit PQ-Eintraegen gilt als leer (fiele aus der StateRoot)")
	}
	// Strenges Lesen: unsortiert, doppelt, kaputt -> abgewiesen.
	kaputt := `{"pq":[{"ed":"` + strings.Repeat("b", 64) + `","m":"0x` + strings.Repeat("1", 40) + `","pub":"` + strings.Repeat("b", pqPubHex) + `","ab":9},{"ed":"` + strings.Repeat("a", 64) + `","m":"0x` + strings.Repeat("1", 40) + `","pub":"` + strings.Repeat("b", pqPubHex) + `","ab":9}]}`
	if _, err := verifierRegisterLesen(kaputt); err == nil {
		t.Fatal("unsortiertes PQ-Feld gelesen")
	}
}

// Der ganze Weg: eine Bescheinigung, an der ein Pruefer-Dienst (B1b) bindet,
// nach dem PQ-Stichtag. Ohne eingetragenen PQ-Schluessel ungueltig (der
// Befund B2), mit ihm gueltig -- aber nur fuer den Menschen der Bindung und
// erst ab seiner Wirkung.
type pdQuelle struct {
	f   *b1bFall
	reg *verifierRegister
}

func (q pdQuelle) mitglied(m string, t int64) bool { return q.f.istPruefer(m, t) }
func (q pdQuelle) dienstPQ(ed, mensch string, t int64) (string, bool) {
	return q.reg.dienstPQ(ed, mensch, t)
}

func TestPrueferDienstPQ_BescheinigungNachDemStichtag(t *testing.T) {
	coord, zeuge := neuerPQ(t), neuerPQ(t)
	f := neuerB1bFall(t, 1)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 1,
		Coordinator: []string{coord.ed.pub}, Bezeuger: []string{zeuge.ed.pub},
		PQAbUnix: bTestZeit, PQ: map[string]string{coord.ed.pub: coord.pub, zeuge.ed.pub: zeuge.pub}})
	p := neuerB1bPruefer(t)
	f.pruefer[p.mensch] = true
	dienst := neuerPrueferPQ(t)
	pqp := pqTestSchluessel{ed: p.k, pq: dienst.sk, pub: dienst.pub}
	tx := mitBindungen(bTestTx(t, bTestNullifier,
		bindung(p.k, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(zeuge.ed, "test", bTestNullifier, bTestZeit)), p.bindung(t))
	tx.BioBescheinigung.PQ = []string{
		pqp.pqUnterschrift(t, bindungsText("test", bTestNullifier, bTestWallet, bTestZeit)),
		zeuge.pqUnterschrift(t, aussageText("test", bTestNullifier, bTestZeit)),
	}
	leer := pdQuelle{f: f, reg: &verifierRegister{}}
	if err := pruefeBioBescheinigungMit(tx, bTestZeit+60, leer); err == nil || !strings.Contains(err.Error(), "keinen PQ-Schluessel") {
		t.Fatalf("ohne eingetragenen Dienst-Schluessel: %v (der Befund B2)", err)
	}
	mit := pdQuelle{f: f, reg: (&verifierRegister{}).mitDienstPQ(prueferDienstPQ{Ed: p.k.pub, Mensch: p.mensch, Pub: dienst.pub, Wirkung: bTestZeit})}
	if err := pruefeBioBescheinigungMit(tx, bTestZeit+60, mit); err != nil {
		t.Fatalf("mit eingetragenem Dienst-Schluessel abgewiesen: %v", err)
	}
	spaet := pdQuelle{f: f, reg: (&verifierRegister{}).mitDienstPQ(prueferDienstPQ{Ed: p.k.pub, Mensch: p.mensch, Pub: dienst.pub, Wirkung: bTestZeit + 1})}
	if err := pruefeBioBescheinigungMit(tx, bTestZeit+60, spaet); err == nil {
		t.Fatal("Dienst-Schluessel zaehlt vor seiner Wirkung")
	}
	fremd := pdQuelle{f: f, reg: (&verifierRegister{}).mitDienstPQ(prueferDienstPQ{Ed: p.k.pub, Mensch: "0x" + strings.Repeat("9", 40), Pub: dienst.pub, Wirkung: bTestZeit})}
	if err := pruefeBioBescheinigungMit(tx, bTestZeit+60, fremd); err == nil {
		t.Fatal("Dienst-Schluessel eines anderen Menschen zaehlt")
	}
	anderer := neuerPrueferPQ(t)
	falsch := pdQuelle{f: f, reg: (&verifierRegister{}).mitDienstPQ(prueferDienstPQ{Ed: p.k.pub, Mensch: p.mensch, Pub: anderer.pub, Wirkung: bTestZeit})}
	if err := pruefeBioBescheinigungMit(tx, bTestZeit+60, falsch); err == nil {
		t.Fatal("PQ-Unterschrift passt zu einem anderen eingetragenen Schluessel")
	}
}

// Der Besitznachweis aus dem Dienst (aequitas-biometric-beta,
// pq_unterschrift.besitz, Python cryptography): derselbe Satz, und der Knoten
// nimmt genau diese Unterschrift an.
func TestPrueferDienstPQ_VektorAusDemDienst(t *testing.T) {
	roh, err := os.ReadFile("testdata/pruefer_dienst_besitz_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Mensch, Ed, Nachricht, Pub, Sig string }
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	if got := prueferDienstBesitzNachricht("0xAbCdEf0123456789aBcDeF0123456789AbCdEf01", strings.ToUpper(v.Ed)); got != v.Nachricht {
		t.Fatalf("Satz:\n got %s\nwant %s", got, v.Nachricht)
	}
	if !mldsaPruefen(v.Pub, v.Sig, prueferDienstBesitzNachricht(v.Mensch, v.Ed)) {
		t.Fatal("Besitznachweis des Dienstes abgewiesen")
	}
	if mldsaPruefen(v.Pub, v.Sig, prueferDienstBesitzNachricht("0x"+strings.Repeat("1", 40), v.Ed)) {
		t.Fatal("Besitznachweis gilt fuer einen anderen Menschen")
	}
	if got := prueferDienstFreigabeNachricht(v.Mensch, v.Ed, strings.Repeat("e", 64), 1800000000); got !=
		"aequitas-pruefer-dienst-pq-v1|chain:1926|"+v.Mensch+"|"+v.Ed+"|"+strings.Repeat("e", 64)+"|1800000000" {
		t.Fatalf("Freigabe-Satz: %s", got)
	}
}
