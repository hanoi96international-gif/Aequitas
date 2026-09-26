package keeper

import (
	"fmt"
	"testing"
	"time"
)

// SIMULATION DER VERTEILTEN ANNAHME (Stufe 2).
//
// Die Eigenschaft: fuer KEIN Konto nehmen zwei Knoten gleichzeitig an. Sie ist
// die Verallgemeinerung dessen, was leitung_sim_test.go fuer den einen Leiter
// prueft, und genau der Zustand, der am 15.09.2026 die Kontenstaende
// auseinanderlaufen liess -- nur jetzt je Konto statt fuer das ganze Netz.
//
// Geprueft in jedem 50-ms-Schritt, fuer eine feste Stichprobe von Konten und
// die globalen Konten, mit Uhrengang, Nachrichtenverlust, Ausfaellen,
// Neustarts und Netztrennungen. Daneben gemessen: wie oft ein Konto
// ueberhaupt einen Annehmenden hat -- eine Sicherheit, die alles stilllegt,
// waere keine.

type verteiltPruefung struct {
	konten     []string
	verletzt   bool
	schritte   int
	versorgt   int // Schritte x Konten mit genau einem Annehmenden
	jeKnoten   map[string]int
	maxDoppelt string
}

func neueVerteiltPruefung(anzahl int) *verteiltPruefung {
	p := &verteiltPruefung{jeKnoten: map[string]int{}}
	for i := 0; i < anzahl; i++ {
		p.konten = append(p.konten, fmt.Sprintf("0xk%038d", i))
	}
	p.konten = append(p.konten, kontoLiquiditaetspool, ubiPoolAddr)
	return p
}

func verteiltesSimNetz(t *testing.T, anzahl int, seed int64, c LeitKonfig) *simNetz {
	c.Verteilt = true
	return neuesSimNetz(t, anzahl, seed, c)
}

func (p *verteiltPruefung) schritt(n *simNetz) {
	n.schritt()
	p.schritte++
	for _, konto := range p.konten {
		var wer []string
		for i, k := range n.knoten {
			if k.an && k.l.DarfAnnehmenFuer(konto, n.uhr(i)) {
				wer = append(wer, k.addr)
			}
		}
		if len(wer) > 1 {
			if !p.verletzt {
				n.t.Errorf("t=%s: Konto %s hat ZWEI Annehmende: %v", n.jetzt, konto, wer)
			}
			p.verletzt = true
		}
		if len(wer) == 1 {
			p.versorgt++
			p.jeKnoten[wer[0]]++
		}
	}
}

func (p *verteiltPruefung) laufe(n *simNetz, d time.Duration) {
	ende := n.jetzt + d
	for n.jetzt < ende {
		p.schritt(n)
	}
}

// Anteil der Konten mit einem Annehmenden im naechsten Schritt.
func (p *verteiltPruefung) jetztVersorgt(n *simNetz) float64 {
	gut := 0
	for _, konto := range p.konten {
		c := 0
		for i, k := range n.knoten {
			if k.an && k.l.DarfAnnehmenFuer(konto, n.uhr(i)) {
				c++
			}
		}
		if c == 1 {
			gut++
		}
	}
	return float64(gut) / float64(len(p.konten))
}

func TestVerteilt_AlleNehmenAnJederFuerSeineKonten(t *testing.T) {
	n := verteiltesSimNetz(t, 5, 1, testKonfig())
	p := neueVerteiltPruefung(200)
	p.laufe(n, 20*time.Second)
	if v := p.jetztVersorgt(n); v != 1 {
		t.Fatalf("nach dem Anlauf haben nur %.0f %% der Konten einen Annehmenden", v*100)
	}
	// Nicht einer nimmt alles an: alle fuenf haben Konten.
	vorher := map[string]int{}
	for k, v := range p.jeKnoten {
		vorher[k] = v
	}
	p.laufe(n, 5*time.Second)
	for _, k := range n.knoten {
		if p.jeKnoten[k.addr] == vorher[k.addr] {
			t.Errorf("%s nimmt fuer kein Konto an", k.addr)
		}
	}
	// Globale Konten: nur der Leiter.
	for i, k := range n.knoten {
		leiter, _ := k.l.Leiter()
		if k.l.DarfAnnehmenFuer(kontoLiquiditaetspool, n.uhr(i)) != (leiter == k.addr) {
			t.Errorf("%s: Pool-Annahme passt nicht zur Rolle (Leiter %s)", k.addr, leiter)
		}
	}
	if p.verletzt {
		t.Fatal("zwei Annehmende fuer ein Konto")
	}
}

func TestVerteilt_PlanmaessigerWechselNieDoppelt(t *testing.T) {
	c := testKonfig()
	c.WechselAlle = 20 * time.Second
	n := verteiltesSimNetz(t, 5, 2, c)
	p := neueVerteiltPruefung(150)
	startTerm := n.knoten[0].l.Term()
	p.laufe(n, 3*time.Minute)
	if n.knoten[0].l.Term() < startTerm+5 {
		t.Fatalf("zu wenige Wechsel: Term %d", n.knoten[0].l.Term())
	}
	if p.verletzt {
		t.Fatal("zwei Annehmende waehrend planmaessiger Wechsel")
	}
	// Die Zuteilung wechselt mit dem Term: dasselbe Konto gehoert nicht
	// immer demselben.
	eigner := map[string]bool{}
	for w := 0; w < 6; w++ {
		p.laufe(n, 21*time.Second)
		for i, k := range n.knoten {
			if k.l.DarfAnnehmenFuer(p.konten[0], n.uhr(i)) {
				eigner[k.addr] = true
			}
		}
	}
	if len(eigner) < 2 {
		t.Errorf("Konto blieb ueber sechs Terme bei %v", eigner)
	}
	if v := float64(p.versorgt) / float64(p.schritte*len(p.konten)); v < 0.8 {
		t.Errorf("nur %.0f %% der Zeit versorgt -- die Uebergaben legen zu viel still", v*100)
	}
}

func TestVerteilt_WechselWartetAufDieBloeckeDerVorgaenger(t *testing.T) {
	c := testKonfig()
	c.WechselAlle = 15 * time.Second
	n := verteiltesSimNetz(t, 3, 3, c)
	// Knoten 2 hat die Bloecke von Knoten 1 nicht.
	n.knoten[2].hatBlock = func(h string) bool {
		return len(h) < 4 || h[:4] != "blk-" || h[4:4+len(n.knoten[1].addr)] != n.knoten[1].addr
	}
	n.baue(2)
	p := neueVerteiltPruefung(100)
	p.laufe(n, 40*time.Second)
	for i := range p.konten[:100] {
		if n.knoten[2].l.DarfAnnehmenFuer(p.konten[i], n.uhr(2)) {
			t.Fatalf("Knoten 2 nimmt an, ohne die Abschlussbloecke von Knoten 1 nachgespielt zu haben")
		}
	}
	n.knoten[2].hatBlock = nil
	p.laufe(n, 30*time.Second)
	nimmt := false
	for i := range p.konten[:100] {
		nimmt = nimmt || n.knoten[2].l.DarfAnnehmenFuer(p.konten[i], n.uhr(2))
	}
	if !nimmt {
		t.Fatal("Knoten 2 nimmt auch mit allen Bloecken nie an")
	}
	if p.verletzt {
		t.Fatal("zwei Annehmende")
	}
}

func TestVerteilt_AusfallEinesMitgliedsUebernimmtDerNaechste(t *testing.T) {
	n := verteiltesSimNetz(t, 5, 4, testKonfig())
	p := neueVerteiltPruefung(200)
	p.laufe(n, 15*time.Second)
	n.knoten[3].an = false
	p.laufe(n, 30*time.Second)
	if v := p.jetztVersorgt(n); v != 1 {
		t.Fatalf("nach dem Ausfall eines Mitglieds nur %.0f %% versorgt -- seine Konten wurden nicht uebernommen", v*100)
	}
	// Zurueck: im laufenden Term bleibt es ausgefallen, keine Doppelannahme.
	n.knoten[3].an = true
	n.baue(3)
	p.laufe(n, 30*time.Second)
	if p.verletzt {
		t.Fatal("zwei Annehmende nach Ausfall und Rueckkehr")
	}
}

func TestVerteilt_MitgliedMitLeiterAbgeschnitten(t *testing.T) {
	n := verteiltesSimNetz(t, 5, 5, testKonfig())
	p := neueVerteiltPruefung(200)
	p.laufe(n, 15*time.Second)
	// Leiter und ein Mitglied gegen die Mehrheit.
	n.gruppe[0], n.gruppe[1] = 1, 1
	p.laufe(n, 60*time.Second)
	// Die Mehrheit hat einen neuen Leiter und versorgt die Konten.
	gut := 0
	for _, konto := range p.konten {
		for i := 2; i < 5; i++ {
			if n.knoten[i].l.DarfAnnehmenFuer(konto, n.uhr(i)) {
				gut++
			}
		}
	}
	if gut < len(p.konten)*9/10 {
		t.Errorf("Mehrheitsseite versorgt nur %d von %d Konten", gut, len(p.konten))
	}
	// Die Minderheit nimmt fuer nichts an.
	for _, konto := range p.konten {
		for i := 0; i < 2; i++ {
			if n.knoten[i].l.DarfAnnehmenFuer(konto, n.uhr(i)) {
				t.Fatalf("abgeschnittener Knoten %d nimmt fuer %s an", i, konto)
			}
		}
	}
	n.gruppe = map[int]int{}
	p.laufe(n, 40*time.Second)
	if n.knoten[0].ueberholt == 0 && n.knoten[1].ueberholt == 0 {
		t.Error("kein abgeschnittener Knoten hat sich als ueberholt erkannt")
	}
	if p.verletzt {
		t.Fatal("zwei Annehmende bei Netztrennung")
	}
}

func TestVerteilt_Zufall(t *testing.T) {
	for seed := int64(300); seed < 312; seed++ {
		c := testKonfig()
		c.WechselAlle = 40 * time.Second
		n := verteiltesSimNetz(t, 5, seed, c)
		n.verlust = 0.1
		for i, k := range n.knoten {
			k.gang = (n.r.Float64()*2 - 1) * 0.002
			n.baue(i)
		}
		p := neueVerteiltPruefung(60)
		for runde := 0; runde < 30; runde++ {
			switch n.r.Intn(5) {
			case 0:
				n.knoten[n.r.Intn(5)].an = false
			case 1:
				i := n.r.Intn(5)
				if !n.knoten[i].an {
					n.knoten[i].an = true
					n.baue(i)
				}
			case 2:
				n.gruppe = map[int]int{}
				for i := range n.knoten {
					n.gruppe[i] = n.r.Intn(2)
				}
			case 3:
				n.gruppe = map[int]int{}
				n.gekappt = map[[2]int]bool{}
				a, b := n.r.Intn(5), n.r.Intn(5)
				if a != b {
					if a > b {
						a, b = b, a
					}
					n.gekappt[[2]int{a, b}] = true
				}
			default:
			}
			p.laufe(n, time.Duration(5+n.r.Intn(30))*time.Second)
			if p.verletzt {
				t.Fatalf("seed %d, Runde %d: zwei Annehmende", seed, runde)
			}
		}
		for i, k := range n.knoten {
			if !k.an {
				k.an = true
				n.baue(i)
			}
		}
		n.gruppe = map[int]int{}
		n.gekappt = map[[2]int]bool{}
		// Anteil ueber eine Minute, nicht Momentaufnahme: jede planmaessige
		// Uebergabe laesst fuer ~3 s niemanden annehmen (Abschluss, dann die
		// erste von einer Mehrheit bestaetigte Lease des Neuen). Bei
		// WechselAlle = 40 s sind das zwei Luecken je Minute; im Betrieb
		// (10 min) ein halbes Prozent.
		p.laufe(n, time.Minute)
		p.versorgt, p.schritte = 0, 0
		p.laufe(n, time.Minute)
		if v := float64(p.versorgt) / float64(p.schritte*len(p.konten)); v < 0.75 {
			t.Errorf("seed %d: nach Heilung nur %.0f %% der Zeit versorgt", seed, v*100)
		}
	}
}

// Bei zwei Validatoren gibt es keine verteilte Annahme -- wie bisher nimmt
// genau einer fuer alle Konten an.
func TestVerteilt_ZweiValidatorenBleibenBeimEinen(t *testing.T) {
	n := verteiltesSimNetz(t, 2, 6, testKonfig())
	p := neueVerteiltPruefung(50)
	p.laufe(n, 10*time.Second)
	for _, konto := range p.konten {
		if n.knoten[1].l.DarfAnnehmenFuer(konto, n.uhr(1)) || !n.knoten[0].l.DarfAnnehmenFuer(konto, n.uhr(0)) {
			t.Fatalf("zwei Validatoren: Konto %s nicht beim Startleiter", konto)
		}
	}
}

// Die Aktivierung mitten im Term greift erst mit dem naechsten: bis dahin
// nimmt der Leiter weiter fuer alle an, und kein Mitglied nimmt an, bevor es
// den letzten Block des bisher einzigen Annehmenden hat.
func TestVerteilt_AktivierungGreiftMitDemNaechstenTerm(t *testing.T) {
	c := testKonfig()
	c.WechselAlle = 30 * time.Second
	n := neuesSimNetz(t, 5, 7, c) // Verteilt aus
	p := neueVerteiltPruefung(100)
	p.laufe(n, 10*time.Second)
	leiter := n.annehmender()
	if leiter < 0 {
		t.Fatal("kein Leiter")
	}
	verteilteAnnahmeOverride.Store(1) // ab jetzt aktiviert
	t.Cleanup(func() { verteilteAnnahmeOverride.Store(0) })
	term := n.knoten[leiter].l.Term()
	p.laufe(n, 2*time.Second)
	if n.knoten[leiter].l.Term() != term {
		t.Fatal("Term wechselte zu frueh -- der Test prueft dann nichts")
	}
	for _, konto := range p.konten {
		for i, k := range n.knoten {
			if i != leiter && k.l.DarfAnnehmenFuer(konto, n.uhr(i)) {
				t.Fatalf("Mitglied %d nimmt im laufenden Term fuer %s an", i, konto)
			}
		}
		if !n.knoten[leiter].l.DarfAnnehmenFuer(konto, n.uhr(leiter)) {
			t.Fatalf("Leiter nimmt nach der Aktivierung im selben Term nicht mehr fuer %s an", konto)
		}
	}
	p.laufe(n, 90*time.Second)
	verteiltJetzt := 0
	for i, k := range n.knoten {
		if k.l.Stand(n.uhr(i))["verteilt"].(map[string]interface{})["an"] == true {
			verteiltJetzt++
		}
	}
	if verteiltJetzt < 4 {
		t.Fatalf("nach mehreren Wechseln nehmen nur %d Knoten verteilt an", verteiltJetzt)
	}
	if p.verletzt {
		t.Fatal("zwei Annehmende beim Uebergang")
	}
}
