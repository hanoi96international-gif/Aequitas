package keeper

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// Mitgliedschaft ohne Handliste (leitung.go, "Wer Mitglied ist"). In jedem
// 50-ms-Schritt prueft simNetz, dass nie zwei gleichzeitig annehmen.

func satzTestKonfig() LeitKonfig {
	c := testKonfig()
	c.EntfernenNach = 60 * time.Second
	c.AufnahmeFrist = 10 * time.Second
	c.AufnahmeSperre = 30 * time.Second
	return c
}

// satzVon: Satz eines Knotens als Text (fuer Vergleiche und Meldungen).
func (n *simNetz) satzVon(i int) string {
	l := n.knoten[i].l
	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Sprintf("v%d/%s", l.satzVersion, strings.Join(l.satz, ","))
}

func (n *simNetz) mitglieder(i int) []string {
	l := n.knoten[i].l
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.satz...)
}

// einigImSatz: alle laufenden Knoten haben denselben Satz, und er besteht
// genau aus ihnen.
func (n *simNetz) einigImSatz() (bool, string) {
	var laufend []string
	for _, k := range n.knoten {
		if k.an {
			laufend = append(laufend, k.addr)
		}
	}
	sort.Strings(laufend)
	want := strings.Join(laufend, ",")
	var ist []string
	ok := true
	for i, k := range n.knoten {
		if !k.an {
			continue
		}
		m := strings.Join(n.mitglieder(i), ",")
		ist = append(ist, n.satzVon(i))
		if m != want {
			ok = false
		}
	}
	return ok, strings.Join(ist, " | ")
}

// Ein Validator startet allein; vier weitere melden sich und werden einer
// nach dem anderen aufgenommen. Danach waehlt die Mehrheit bei Ausfall.
func TestSatz_WachsenVonEinsAufFuenf(t *testing.T) {
	n := neuesSimNetz(t, 5, 21, satzTestKonfig())
	n.genesis = []string{n.knoten[0].addr}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(60 * time.Second)
	if ok, ist := n.einigImSatz(); !ok {
		t.Fatalf("nicht alle aufgenommen: %s", ist)
	}
	if n.annehmender() != 0 {
		t.Fatalf("nimmt %d an, erwartet 0", n.annehmender())
	}
	// Jetzt fuenf: Ausfall des Leiters -> Wahl.
	n.knoten[0].an = false
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i < 1 {
		t.Fatalf("nach Ausfall des Genesis-Validators kein Leiter (%d)", i)
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Wer lange nichts hoeren laesst, wird entfernt; wer zurueckkommt, wieder
// aufgenommen.
func TestSatz_AusgefallenerRausUndWiederRein(t *testing.T) {
	n := neuesSimNetz(t, 3, 22, satzTestKonfig())
	n.laufe(10 * time.Second)
	n.knoten[2].an = false
	n.laufe(3 * time.Minute)
	if ok, ist := n.einigImSatz(); !ok {
		t.Fatalf("Ausgefallener nicht entfernt: %s", ist)
	}
	if n.annehmender() < 0 {
		t.Fatal("nach dem Entfernen nimmt niemand an")
	}
	// Der Leiter faellt jetzt AUCH aus: bei zwei gibt es keine Uebernahme
	// (das ist die bekannte Grenze) -- aber der Uebriggebliebene darf nicht
	// allein anfangen.
	// Erst: der Entfernte kommt zurueck, mit seinem alten (Dreier-)Satz.
	n.knoten[2].an = true
	n.baue(2)
	n.laufe(60 * time.Second)
	if ok, ist := n.einigImSatz(); !ok {
		t.Fatalf("Zurueckgekehrter nicht wieder aufgenommen: %s", ist)
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Ein Neuer meldet sich und faellt aus, bevor er bestaetigt: der Leiter nimmt
// die Aufnahme zurueck und nimmt die ganze Zeit an (vorher allein).
func TestSatz_AufnahmeZurueckgenommen(t *testing.T) {
	n := neuesSimNetz(t, 2, 23, satzTestKonfig())
	n.genesis = []string{n.knoten[0].addr}
	for i := range n.knoten {
		n.baue(i)
	}
	// Knoten 1 meldet sich, wird aufgenommen, faellt sofort aus.
	for schritt := 0; schritt < 400; schritt++ {
		n.schritt()
		if len(n.mitglieder(0)) == 2 {
			n.knoten[1].an = false
			break
		}
	}
	if len(n.mitglieder(0)) != 2 {
		t.Fatal("Knoten 1 wurde nie aufgenommen")
	}
	vorher := n.leiterZeit
	n.laufe(30 * time.Second)
	if got := n.leiterZeit - vorher; got < 29*time.Second {
		t.Fatalf("waehrend der offenen Aufnahme nur %s angenommen (von 30 s)", got)
	}
	if m := n.mitglieder(0); len(m) != 1 {
		t.Fatalf("Aufnahme nicht zurueckgenommen: %v", m)
	}
	// Er kommt wieder: nach der Sperre wieder aufgenommen.
	n.knoten[1].an = true
	n.baue(1)
	n.laufe(60 * time.Second)
	if ok, ist := n.einigImSatz(); !ok {
		t.Fatalf("nach der Sperre nicht aufgenommen: %s", ist)
	}
}

// Von drei auf zwei, waehrend der Entfernte nur vom Leiter abgeschnitten ist
// (nicht von den anderen): er darf mit dem alten Dreier-Satz keine Mehrheit
// finden, waehrend der Leiter zu zweit annimmt.
func TestSatz_EntfernterFindetKeineMehrheit(t *testing.T) {
	n := neuesSimNetz(t, 3, 24, satzTestKonfig())
	n.laufe(10 * time.Second)
	if n.annehmender() != 0 {
		t.Fatal("Start")
	}
	n.gekappt[[2]int{0, 2}] = true
	n.laufe(4 * time.Minute)
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
	if n.annehmender() < 0 {
		t.Fatal("niemand nimmt an")
	}
}

// Nicht registriert = wird nie aufgenommen und bekommt kein Gehoer.
func TestSatz_NurZugelassene(t *testing.T) {
	n := neuesSimNetz(t, 3, 25, satzTestKonfig())
	n.genesis = []string{n.knoten[0].addr}
	n.zugelassen = map[string]bool{n.knoten[0].addr: true, n.knoten[1].addr: true}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(60 * time.Second)
	m := n.mitglieder(0)
	if len(m) != 2 || m[0] == n.knoten[2].addr || m[1] == n.knoten[2].addr {
		t.Fatalf("Satz %v -- erwartet genau 0 und 1", m)
	}
}

// Zufall: Beitritte, Ausfaelle, Neustarts, Trennungen, Verlust, Uhrengang --
// mit wechselnder Mitgliedschaft. Nie zwei Annehmende; am Ende, alles heil,
// genau einer, und alle im selben Satz.
func TestSatz_Zufall(t *testing.T) {
	var aenderungen uint64
	for seed := int64(300); seed < 330; seed++ {
		c := satzTestKonfig()
		c.WechselAlle = 45 * time.Second
		n := neuesSimNetz(t, 6, seed, c)
		n.verlust = 0.1
		n.genesis = []string{n.knoten[0].addr, n.knoten[1].addr, n.knoten[2].addr}
		for i, k := range n.knoten {
			k.gang = (n.r.Float64()*2 - 1) * 0.002
			if i >= 3 {
				k.an = false // treten spaeter bei
			}
			n.baue(i)
		}
		for runde := 0; runde < 40; runde++ {
			switch n.r.Intn(6) {
			case 0:
				n.knoten[n.r.Intn(6)].an = false
			case 1, 5:
				i := n.r.Intn(6)
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
			case 4:
				for k := 0; k < 3; k++ {
					a, b := n.r.Intn(6), n.r.Intn(6)
					if a > b {
						a, b = b, a
					}
					if a != b {
						n.gekappt[[2]int{a, b}] = true
					}
				}
			}
			n.laufe(time.Duration(5+n.r.Intn(60)) * time.Second)
		}
		for i, k := range n.knoten {
			if !k.an {
				k.an = true
				n.baue(i)
			}
		}
		n.gruppe = map[int]int{}
		n.gekappt = map[[2]int]bool{}
		n.laufe(4 * time.Minute)
		if n.zweiLeiter {
			t.Fatalf("seed %d: zwei Leiter gleichzeitig", seed)
		}
		if n.annehmender() < 0 {
			t.Errorf("seed %d: nach Heilung kein Leiter; Saetze: %s", seed, func() string { _, s := n.einigImSatz(); return s }())
			continue
		}
		if ok, ist := n.einigImSatz(); !ok {
			t.Errorf("seed %d: nach Heilung nicht einig: %s", seed, ist)
		}
		aenderungen += n.hoechsteSatzVersion()
	}
	// Sonst prueft der Test nichts ueber die Mitgliedschaft.
	if aenderungen < 100 {
		t.Fatalf("nur %d Satzaenderungen in 30 Laeufen", aenderungen)
	}
	t.Logf("%d Satzaenderungen in 30 Laeufen", aenderungen)
}

func (n *simNetz) hoechsteSatzVersion() uint64 {
	var v uint64
	for _, k := range n.knoten {
		k.l.mu.Lock()
		if k.l.satzVersion > v {
			v = k.l.satzVersion
		}
		k.l.mu.Unlock()
	}
	return v
}

// Zwei auf einmal aufzunehmen waere unsicher: aus {0,1,2} wird {0,1,2,3,4},
// dessen Mehrheit {0,3,4} keinen Knoten mit der alten Mehrheit {1,2} teilt.
// Genau diese Trennung, waehrend 3 und 4 sich melden.
func TestSatz_TrennungWaehrendAufnahme(t *testing.T) {
	for seed := int64(40); seed < 50; seed++ {
		n := neuesSimNetz(t, 5, seed, satzTestKonfig())
		n.genesis = []string{n.knoten[0].addr, n.knoten[1].addr, n.knoten[2].addr}
		for i, k := range n.knoten {
			k.an = i < 3
			n.baue(i)
		}
		n.laufe(10 * time.Second)
		if n.annehmender() != 0 {
			t.Fatal("Start")
		}
		n.gruppe = map[int]int{0: 0, 3: 0, 4: 0, 1: 1, 2: 1}
		n.knoten[3].an, n.knoten[4].an = true, true
		n.laufe(90 * time.Second)
		if n.zweiLeiter {
			t.Fatalf("seed %d: zwei Leiter", seed)
		}
		// Heilen: alle fuenf einig, einer nimmt an.
		n.gruppe = map[int]int{}
		n.laufe(90 * time.Second)
		if ok, ist := n.einigImSatz(); !ok || n.annehmender() < 0 {
			t.Fatalf("seed %d: nach Heilung %v %s", seed, n.annehmender(), ist)
		}
	}
}

// Von drei auf zwei: der Leiter entfernt den Ausgefallenen -- und genau
// danach reisst auch die Verbindung zum Dritten im Bunde, bevor der den
// neuen Satz hat. Der hat noch den Dreier-Satz und waehlt mit dem
// "Ausgefallenen" (der nur vom Leiter abgeschnitten war) einen neuen Leiter.
// Zu zweit ohne Wahl darf der alte Leiter deshalb erst ohne Bestaetigung
// annehmen, wenn feststeht, dass der andere den Zweier-Satz hat.
func TestSatz_DreiAufZweiUnterTrennung(t *testing.T) {
	for seed := int64(60); seed < 70; seed++ {
		c := satzTestKonfig()
		// Wie in Betrieb: die Ruecknahme kommt NACH einer moeglichen Wahl --
		// sie darf die Sicherheit nicht tragen.
		c.AufnahmeFrist = leitVorgabe().AufnahmeFrist
		n := neuesSimNetz(t, 3, seed, c)
		n.laufe(10 * time.Second)
		if n.annehmender() != 0 {
			t.Fatal("Start")
		}
		// 2 ist fuer beide still (sonst entfernt ihn niemand -- 1 hoert ihn
		// ja noch) ...
		n.gekappt[[2]int{0, 2}] = true
		n.gekappt[[2]int{1, 2}] = true
		for schritt := 0; schritt < 5000 && len(n.mitglieder(0)) == 3; schritt++ {
			n.schritt()
		}
		if len(n.mitglieder(0)) != 2 {
			t.Fatalf("seed %d: nicht entfernt", seed)
		}
		// ... und genau jetzt kommt 2 zu 1 zurueck, waehrend 0 von 1
		// abreisst, bevor 1 den Zweier-Satz hat.
		delete(n.gekappt, [2]int{1, 2})
		n.gekappt[[2]int{0, 1}] = true
		n.laufe(60 * time.Second)
		if n.zweiLeiter {
			t.Fatalf("seed %d: zwei Leiter", seed)
		}
	}
}

// Nur vom Leiter abgeschnitten ist nicht ausgefallen: solange ein Folger ihn
// hoert, wird niemand entfernt.
func TestSatz_LebendeWerdenNichtEntfernt(t *testing.T) {
	n := neuesSimNetz(t, 3, 26, satzTestKonfig())
	n.laufe(10 * time.Second)
	n.gekappt[[2]int{0, 2}] = true
	// In JEDEM Schritt: der Leiter schlaegt es nicht einmal vor (die Folger
	// wuerden es ablehnen -- das ist die zweite Schicht, nicht die erste).
	for schritt := 0; schritt < 4*60*20; schritt++ {
		n.schritt()
		if m := n.mitglieder(0); len(m) != 3 {
			t.Fatalf("t=%s: Leiter hat einen lebenden Validator entfernt: %v", n.jetzt, m)
		}
	}
	for i := range n.knoten {
		if m := n.mitglieder(i); len(m) != 3 {
			t.Fatalf("%d: %v -- ein lebender Validator wurde entfernt", i, m)
		}
	}
	if n.zweiLeiter || n.annehmender() < 0 {
		t.Fatalf("zwei Leiter %v / Annehmender %d", n.zweiLeiter, n.annehmender())
	}
}

// Ein Mensch, eine Stimme: zwei Schluessel desselben Menschen -- nur einer
// wird Mitglied.
func TestSatz_EinMenschEineStimme(t *testing.T) {
	n := neuesSimNetz(t, 4, 27, satzTestKonfig())
	n.genesis = []string{n.knoten[0].addr}
	mensch := map[string]string{}
	for i, k := range n.knoten {
		mensch[k.addr] = fmt.Sprintf("mensch-%d", i)
	}
	mensch[n.knoten[3].addr] = mensch[n.knoten[2].addr] // derselbe Mensch
	for i := range n.knoten {
		n.baue(i)
		n.knoten[i].l.env.Mensch = func(a string) string { return mensch[a] }
	}
	// In JEDEM Schritt: auch der Leiter schlaegt nie beide vor (die Folger
	// lehnten es ab -- zweite Schicht).
	for schritt := 0; schritt < 2*60*20; schritt++ {
		n.schritt()
		zwei, drei := false, false
		for _, a := range n.mitglieder(0) {
			zwei = zwei || a == n.knoten[2].addr
			drei = drei || a == n.knoten[3].addr
		}
		if zwei && drei {
			t.Fatalf("t=%s: Leiter hat denselben Menschen zweimal im Satz", n.jetzt)
		}
	}
	m := n.mitglieder(0)
	if len(m) != 3 {
		t.Fatalf("Satz %v -- erwartet 0, 1 und genau einer von 2/3", m)
	}
	zwei, drei := false, false
	for _, a := range m {
		zwei = zwei || a == n.knoten[2].addr
		drei = drei || a == n.knoten[3].addr
	}
	if zwei == drei {
		t.Fatalf("Satz %v -- derselbe Mensch zweimal oder gar nicht", m)
	}
}

// Ein Leiter, der luegt: er wirft einen lebenden Validator hinaus und nimmt
// einen nicht registrierten auf. Die Folger uebernehmen das nicht; die
// Aenderung wird nie bestaetigt, der Leiter nimmt sie zurueck.
func TestSatz_LuegenderLeiter(t *testing.T) {
	n := neuesSimNetz(t, 5, 28, satzTestKonfig())
	n.genesis = []string{n.knoten[0].addr, n.knoten[1].addr, n.knoten[2].addr}
	// 3 ist nicht registriert; 4 ist registriert, gehoert aber demselben
	// Menschen wie 1.
	n.zugelassen = map[string]bool{n.knoten[0].addr: true, n.knoten[1].addr: true, n.knoten[2].addr: true, n.knoten[4].addr: true}
	mensch := map[string]string{}
	for i, k := range n.knoten {
		mensch[k.addr] = fmt.Sprintf("mensch-%d", i)
	}
	mensch[n.knoten[4].addr] = mensch[n.knoten[1].addr]
	for i := range n.knoten {
		n.baue(i)
		n.knoten[i].l.env.Mensch = func(a string) string { return mensch[a] }
	}
	n.knoten[3].an = false
	n.knoten[4].an = false
	n.laufe(20 * time.Second)
	if n.annehmender() != 0 {
		t.Fatal("Start")
	}
	for _, versuch := range []struct {
		name string
		satz []string
	}{
		{"wirft den lebenden 2 hinaus", []string{n.knoten[0].addr, n.knoten[1].addr}},
		{"nimmt den nicht registrierten 3 auf", []string{n.knoten[0].addr, n.knoten[1].addr, n.knoten[2].addr, n.knoten[3].addr}},
		{"nimmt 4 auf, einen zweiten Schluessel des Menschen hinter 1", []string{n.knoten[0].addr, n.knoten[1].addr, n.knoten[2].addr, n.knoten[4].addr}},
	} {
		l := n.knoten[0].l
		l.mu.Lock()
		l.aendere(versuch.satz, "Luege: "+versuch.name, n.uhr(0))
		l.mu.Unlock()
		n.laufe(5 * time.Second)
		for i := 1; i <= 2; i++ {
			if got := strings.Join(n.mitglieder(i), ","); got != strings.Join(n.satz()[:3], ",") {
				t.Fatalf("%s: Folger %d hat uebernommen: %s", versuch.name, i, got)
			}
		}
		n.laufe(30 * time.Second)
		if m := n.mitglieder(0); len(m) != 3 {
			t.Fatalf("%s: Leiter hat nicht zurueckgenommen: %v", versuch.name, m)
		}
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}
