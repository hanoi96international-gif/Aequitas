package keeper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"math"
	"testing"
)

// Der einstellbare Satz des Gleichgewichts (satz_plan.go), ohne Datenbank:
// Gutfall, Wirkung im Umlauf, je Angriff ein Missbrauchstest.

const spTag int64 = 86400

var spRegeln = []string{"satz_form", "satz_grenze", "satz_zeit", "satz_wiederholt", "satz_vorlauf",
	"satz_sprung", "satz_quartal", "satz_ohne_bescheinigung", "satz_quorum"}

func spZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range spRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func spNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range spRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

type spFall struct {
	t          *testing.T
	cs         *ChainState
	betreiber  []seBetreiber
	zugelassen map[string]bool
	block      int64
}

// neuerSpFall: Gleichgewicht aktiv, Coordinatoren zugelassen, drei
// Betreiber-Menschen, ein vierter Mensch ohne Zulassung.
func neuerSpFall(t *testing.T) *spFall {
	t.Helper()
	gleichgewichtAn(t)
	vorherLeser := registerLeserOverride.Load()
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(vorherLeser) })
	f := &spFall{t: t, cs: newTestState(), zugelassen: map[string]bool{}, block: 1_700_000_000}
	for i := 0; i < 4; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		mk, mensch := neuerSchluessel(t)
		pubHex := hex.EncodeToString(pub)
		f.betreiber = append(f.betreiber, seBetreiber{priv: priv, pubHex: pubHex, bindung: CoordinatorBindung{Mensch: mensch,
			MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}})
		addHuman(f.cs, mensch, 1000)
		f.zugelassen[mensch] = i < 3
	}
	f.cs.zulassungOhneDB = func(m string, _ int64) (bool, error) { return f.zugelassen[m], nil }
	return f
}

func spUnterschrift(b seBetreiber, bp, giltAb, issued int64) Lebendigkeitsbescheinigung {
	sig := ed25519.Sign(b.priv, []byte(satzNachricht(bp, giltAb)("", issued)))
	return Lebendigkeitsbescheinigung{PublicKey: b.pubHex, Signature: hex.EncodeToString(sig),
		Mensch: b.bindung.Mensch, MenschSig: b.bindung.MenschSig, SchluesselSig: b.bindung.SchluesselSig}
}

func (f *spFall) ankuendigung(bp, giltAb, issued int64, von ...int) Transaction {
	a := &SatzAnkuendigung{Bp: bp, GiltAb: giltAb}
	for _, i := range von {
		a.Bescheinigungen = append(a.Bescheinigungen, spUnterschrift(f.betreiber[i], bp, giltAb, issued))
	}
	return Transaction{Type: satzTxArt, DistributionAt: issued, Satz: a}
}

func (f *spFall) anwenden(tx Transaction, blockZeit int64) (map[string]int64, error) {
	f.t.Helper()
	vorher := spZaehler()
	f.cs.mu.Lock()
	err := f.cs.applySatzAnkuendigungLocked(context.Background(), &tx, blockZeit)
	f.cs.mu.Unlock()
	return spNeu(vorher), err
}

func (f *spFall) satzZu(t int64) float64 {
	f.cs.mu.RLock()
	defer f.cs.mu.RUnlock()
	return f.cs.gleichgewichtSatzZuLocked(t)
}

func TestSatzPlan_LesenNurKanonisch(t *testing.T) {
	gut := satzPlan{Stufen: []satzStufe{{Ab: 100, Bp: 55}, {Ab: 200, Bp: 60}}, Zuletzt: 90}
	if p, err := satzPlanLesen(gut.kodiert()); err != nil || p.kodiert() != gut.kodiert() {
		t.Fatalf("Rundreise: %v %+v", err, p)
	}
	if p, err := satzPlanLesen(""); err != nil || !p.leer() {
		t.Fatalf("leer: %v %+v", err, p)
	}
	for _, schlecht := range []string{
		`{"stufen":[{"ab":200,"bp":60},{"ab":100,"bp":55}]}`,                               // nicht sortiert
		`{"stufen":[{"ab":100,"bp":52}]}`,                                                  // kein Schritt
		`{"stufen":[{"ab":100,"bp":150}]}`,                                                 // ueber der Spanne
		`{"stufen":[{"ab":1,"bp":50},{"ab":2,"bp":50},{"ab":3,"bp":50},{"ab":4,"bp":50}]}`, // zu viele
		`{"zuletzt":5, "stufen":[{"ab":100,"bp":55}]}`,                                     // nicht kanonisch
		`kaputt`,
	} {
		if _, err := satzPlanLesen(schlecht); err == nil {
			t.Fatalf("angenommen: %s", schlecht)
		}
	}
}

// Gutfall: zwei Betreiber kuendigen 0,60 % an; vorher gilt die Vorgabe,
// ab gilt_ab der neue Satz -- auch im Umlauf.
func TestSatz_AnkuendigungWirktAbGiltAb(t *testing.T) {
	nachrechnenModus(t, true)
	f := neuerSpFall(t)
	ab := f.block + 31*spTag
	if neu, err := f.anwenden(f.ankuendigung(60, ab, f.block-60, 0, 1), f.block); err != nil || len(neu) != 0 {
		t.Fatalf("gueltige Ankuendigung: %v %v", neu, err)
	}
	if s := f.satzZu(ab - 1); s != gleichgewichtSatzMonat {
		t.Fatalf("vor gilt_ab gilt die Vorgabe, nicht %v", s)
	}
	if s := f.satzZu(ab); math.Abs(s-0.006) > 1e-12 {
		t.Fatalf("ab gilt_ab 0,6 %%, nicht %v", s)
	}
	f.cs.mu.RLock()
	vorher := f.cs.umlaufBetragRoh(wMensch1, artMensch, 1000, ab-1, sekundenJeMonat)
	nachher := f.cs.umlaufBetragRoh(wMensch1, artMensch, 1000, ab, sekundenJeMonat)
	f.cs.mu.RUnlock()
	if !fast(vorher, 5) || !fast(nachher, 6) {
		t.Fatalf("Umlauf auf 1.000 AEQ im Monat: vorher %v (5), nachher %v (6)", vorher, nachher)
	}
	if st := f.cs.SatzStand(f.block); st["angekuendigt"] == nil {
		t.Fatalf("Anzeige ohne Ankuendigung: %v", st)
	}
}

// Missbrauch: jede Grenze, jede Unterschrift. Nie angewandt, gezaehlt,
// im strengen Modus abgelehnt.
func TestSatz_Missbrauch(t *testing.T) {
	type fall struct {
		name  string
		tx    func(f *spFall) Transaction
		block func(f *spFall) int64
		regel string
	}
	ab := func(f *spFall, tage int64) int64 { return f.block + tage*spTag }
	faelle := []fall{
		{"ein Betreiber", func(f *spFall) Transaction { return f.ankuendigung(60, ab(f, 31), f.block, 0) }, nil, "satz_form"},
		{"derselbe zweimal", func(f *spFall) Transaction { return f.ankuendigung(60, ab(f, 31), f.block, 0, 0) }, nil, "satz_quorum"},
		{"nicht zugelassen", func(f *spFall) Transaction { return f.ankuendigung(60, ab(f, 31), f.block, 0, 3) }, nil, "satz_ohne_bescheinigung"},
		{"anderer Satz unterschrieben", func(f *spFall) Transaction {
			tx := f.ankuendigung(60, ab(f, 31), f.block, 0, 1)
			tx.Satz.Bp = 70
			return tx
		}, nil, "satz_ohne_bescheinigung"},
		{"anderes Datum unterschrieben", func(f *spFall) Transaction {
			tx := f.ankuendigung(60, ab(f, 31), f.block, 0, 1)
			tx.Satz.GiltAb += spTag
			return tx
		}, nil, "satz_ohne_bescheinigung"},
		{"fremde Bindung", func(f *spFall) Transaction {
			tx := f.ankuendigung(60, ab(f, 31), f.block, 0, 1)
			tx.Satz.Bescheinigungen[1].Mensch = f.betreiber[2].bindung.Mensch
			return tx
		}, nil, "satz_ohne_bescheinigung"},
		{"ueber der Spanne", func(f *spFall) Transaction { return f.ankuendigung(105, ab(f, 31), f.block, 0, 1) }, nil, "satz_grenze"},
		{"unter der Spanne", func(f *spFall) Transaction { return f.ankuendigung(20, ab(f, 31), f.block, 0, 1) }, nil, "satz_grenze"},
		{"kein Schritt", func(f *spFall) Transaction { return f.ankuendigung(52, ab(f, 31), f.block, 0, 1) }, nil, "satz_grenze"},
		{"Sprung", func(f *spFall) Transaction { return f.ankuendigung(80, ab(f, 31), f.block, 0, 1) }, nil, "satz_sprung"},
		{"zu kurz vorher", func(f *spFall) Transaction { return f.ankuendigung(60, ab(f, 29), f.block, 0, 1) }, nil, "satz_vorlauf"},
		{"zu weit voraus", func(f *spFall) Transaction { return f.ankuendigung(60, ab(f, 91), f.block, 0, 1) }, nil, "satz_vorlauf"},
		{"aus der Zukunft", func(f *spFall) Transaction { return f.ankuendigung(60, ab(f, 31), f.block+3600, 0, 1) }, nil, "satz_zeit"},
		{"zu alt", func(f *spFall) Transaction { return f.ankuendigung(60, ab(f, 31), f.block-8*spTag, 0, 1) }, nil, "satz_zeit"},
		{"ohne Ankuendigung", func(f *spFall) Transaction { return Transaction{Type: satzTxArt, DistributionAt: f.block} }, nil, "satz_form"},
	}
	for _, c := range faelle {
		t.Run(c.name, func(t *testing.T) {
			for _, streng := range []bool{false, true} {
				nachrechnenModus(t, streng)
				f := neuerSpFall(t)
				blockZeit := f.block
				if c.block != nil {
					blockZeit = c.block(f)
				}
				neu, err := f.anwenden(c.tx(f), blockZeit)
				if neu[c.regel] != 1 {
					t.Fatalf("streng=%v: %s erwartet, gemeldet %v", streng, c.regel, neu)
				}
				if streng != (err != nil) {
					t.Fatalf("streng=%v: Fehler %v", streng, err)
				}
				if !f.cs.satzPlan.leer() {
					t.Fatalf("streng=%v: ungueltige Ankuendigung angewandt: %+v", streng, f.cs.satzPlan)
				}
			}
		})
	}
}

// Wiederholung und Quartal: eine angenommene Ankuendigung laesst sich nicht
// noch einmal einspielen, und nach einer Aenderung ist die naechste erst
// ein Quartal spaeter moeglich.
func TestSatz_WiederholungUndQuartal(t *testing.T) {
	nachrechnenModus(t, false)
	f := neuerSpFall(t)
	ab1 := f.block + 31*spTag
	tx := f.ankuendigung(60, ab1, f.block, 0, 1)
	if neu, _ := f.anwenden(tx, f.block); len(neu) != 0 {
		t.Fatalf("erste: %v", neu)
	}
	if neu, _ := f.anwenden(tx, f.block+60); neu["satz_wiederholt"] != 1 {
		t.Fatalf("Wiederholung nicht erkannt: %v", neu)
	}
	// Die Aenderung gilt; einen Tag danach die naechste.
	b2 := ab1 + spTag
	if neu, _ := f.anwenden(f.ankuendigung(65, b2+40*spTag, b2, 0, 2), b2); neu["satz_quartal"] != 1 {
		t.Fatalf("zweite Aenderung im Quartal nicht erkannt: %v", neu)
	}
	if neu, _ := f.anwenden(f.ankuendigung(65, ab1+91*spTag, b2+60, 0, 2), b2+60); len(neu) != 0 {
		t.Fatalf("nach einem Quartal erlaubt: %v", neu)
	}
	if s := f.satzZu(ab1 + 91*spTag); math.Abs(s-0.0065) > 1e-12 {
		t.Fatalf("0,65 %% erwartet, %v", s)
	}
}

// Eine neue Ankuendigung ersetzt eine offene; der geltende Satz nimmt sie
// zurueck -- und zaehlt nicht als Aenderung.
func TestSatz_ErsetzenUndZuruecknehmen(t *testing.T) {
	nachrechnenModus(t, true)
	f := neuerSpFall(t)
	ab := f.block + 40*spTag
	if _, err := f.anwenden(f.ankuendigung(60, ab, f.block, 0, 1), f.block); err != nil {
		t.Fatal(err)
	}
	if _, err := f.anwenden(f.ankuendigung(55, ab+spTag, f.block+60, 1, 2), f.block+60); err != nil {
		t.Fatal(err)
	}
	if s := f.satzZu(ab + spTag); math.Abs(s-0.0055) > 1e-12 || len(f.cs.satzPlan.Stufen) != 1 {
		t.Fatalf("ersetzt: %v %+v", s, f.cs.satzPlan)
	}
	if _, err := f.anwenden(f.ankuendigung(50, ab, f.block+120, 0, 2), f.block+120); err != nil {
		t.Fatal(err)
	}
	if len(f.cs.satzPlan.Stufen) != 0 || f.satzZu(ab+spTag) != gleichgewichtSatzMonat {
		t.Fatalf("zurueckgenommen: %+v", f.cs.satzPlan)
	}
}

// Der Plan bleibt begrenzt, wie viele Aenderungen es auch gibt.
func TestSatz_PlanBegrenzt(t *testing.T) {
	nachrechnenModus(t, true)
	f := neuerSpFall(t)
	block, bp := f.block, int64(50)
	for i := 0; i < 8; i++ {
		bp += 5
		if bp > 75 {
			bp = 55
		}
		if _, err := f.anwenden(f.ankuendigung(bp, block+31*spTag, block, i%2, 2), block); err != nil {
			t.Fatalf("Runde %d: %v", i, err)
		}
		if n := len(f.cs.satzPlan.Stufen); n > satzPlanStufenMax {
			t.Fatalf("Runde %d: %d Stufen", i, n)
		}
		block += 92 * spTag
	}
}

// Vor dem Gleichgewicht: Leerlauf, unbekannte Art.
func TestSatz_SchlaeftVorDemGleichgewicht(t *testing.T) {
	nachrechnenModus(t, true)
	f := neuerSpFall(t)
	gleichgewichtAktivOverride.Store(math.MaxInt64)
	tx := f.ankuendigung(60, f.block+31*spTag, f.block, 0, 1)
	if neu, err := f.anwenden(tx, f.block); err != nil || len(neu) != 0 || !f.cs.satzPlan.leer() {
		t.Fatalf("vor dem Gleichgewicht: %v %v %+v", neu, err, f.cs.satzPlan)
	}
	if bekannteTxArt(satzTxArt, f.block) {
		t.Fatal("die Art muss vor dem Gleichgewicht unbekannt sein")
	}
}

// StateRoot: ohne Plan byte-gleich, mit Plan anders; ein Rueckrollen stellt
// den Plan wieder her.
func TestSatz_StateRootUndRueckrollen(t *testing.T) {
	nachrechnenModus(t, true)
	f := neuerSpFall(t)
	f.cs.mu.Lock()
	ohne := f.cs.stateRootLocked("", "")
	snap := f.cs.snapshotForRollbackLocked(nil, true, nil)
	f.cs.mu.Unlock()
	if _, err := f.anwenden(f.ankuendigung(60, f.block+31*spTag, f.block, 0, 1), f.block); err != nil {
		t.Fatal(err)
	}
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	mit := f.cs.stateRootLocked("", "")
	if mit == ohne {
		t.Fatal("der Plan muss in der StateRoot stehen")
	}
	if err := f.cs.restoreFromRollbackLockedCtx(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	if !f.cs.satzPlan.leer() || f.cs.stateRootLocked("", "") != ohne {
		t.Fatalf("zurueckgerollt: %+v", f.cs.satzPlan)
	}
}

// Zwei Knoten, eine echte Runde: der Umlauf rechnet mit dem Satz, der zur
// Rundenzeit gilt -- beim Erzeuger und bei jedem Nachspielenden. Ein Knoten,
// der einen anderen Plan haette, meldete jeden Betrag.
func TestSatz_UmlaufNachrechnenMitSatz(t *testing.T) {
	nachrechnenModus(t, true)
	erzeuger, dag, nach := ggKnoten(t)
	at := nowUnix()
	plan := satzPlan{Stufen: []satzStufe{{Ab: at - spTag, Bp: 75}}, Zuletzt: at - 40*spTag}
	erzeuger.satzPlan, nach.satzPlan = plan, plan
	txs := runde(t, erzeuger, at)
	if want := round6(1000 * 0.0075 / 30); !fast(umlaufe(txs)[erhaltungMenschen[1]], want) {
		t.Fatalf("Umlauf mit 0,75 %%: %v, erwartet %v", umlaufe(txs)[erhaltungMenschen[1]], want)
	}
	vorher := ggZaehler()
	if !dag.replayTransactions(erhaltungBlock(1, at+5, txs...), true) {
		t.Fatal("ehrliche Runde abgelehnt")
	}
	if neu := ggNeu(vorher); len(neu) != 0 {
		t.Fatalf("ehrliche Runde meldet %v", neu)
	}
	// Ein Knoten ohne den Plan rechnet mit 0,5 % und weist die Runde ab.
	erzeuger2, dag2, _ := ggKnoten(t)
	erzeuger2.satzPlan = plan
	if dag2.replayTransactions(erhaltungBlock(1, at+5, runde(t, erzeuger2, at)...), true) {
		t.Fatal("ein Knoten mit anderem Satz darf die Runde nicht annehmen")
	}
}
