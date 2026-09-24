package keeper

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"
)

// SIMULATION DER LEITUNG.
//
// Die eine Eigenschaft, um die es geht: NIE nehmen zwei Knoten gleichzeitig
// Ueberweisungen an. Genau dieser Zustand hat am 15.09.2026 die Kontenstaende
// auseinanderlaufen lassen (zwei_produzenten_realdb_test.go). Geprueft wird sie
// in jedem Schritt, in echter Zeit, mit Uhren, die leicht falsch gehen, mit
// verlorenen und verspaeteten Nachrichten, Ausfaellen, Neustarts und
// Netztrennungen.

type simNachricht struct {
	an, von int
	m       LeitNachricht
	ankunft time.Duration
	antwort bool
}

type simKnoten struct {
	addr      string
	l         *Leitung
	an        bool
	gang      float64 // Uhrengang, z. B. 0.001 = 0,1 % zu schnell
	gespeich  LeitSpeicher
	ueberholt int
	hatBlock  func(string) bool
}

type simNetz struct {
	t         *testing.T
	r         *rand.Rand
	jetzt     time.Duration
	t0        time.Time
	knoten    []*simKnoten
	unterwegs []simNachricht
	verlust   float64
	// festeVerz: jede Nachricht braucht gleich lang -- dann laufen zwei
	// Kandidaten im Gleichschritt, wie ueber echtes HTTP im Test beobachtet.
	festeVerz bool
	gruppe    map[int]int // Netztrennung: nur gleiche Gruppe spricht
	// Einzeln gekappte Verbindungen -- NICHT transitiv: A spricht mit B, B
	// mit C, aber A nicht mit C. Genau dort entscheidet die Lease-Zusage
	// (ein Folger, der dem Leiter bestaetigt hat, stimmt fuer niemand anderen).
	gekappt map[[2]int]bool
	cfg     LeitKonfig
	start   string
	faehig  map[string]bool
	// Messwerte
	zweiLeiter bool
	leiterZeit time.Duration
	wechsel    int
	letzterLtr string
}

func (n *simNetz) uhr(i int) time.Time {
	k := n.knoten[i]
	return n.t0.Add(time.Duration(float64(n.jetzt) * (1 + k.gang)))
}

func (n *simNetz) satz() []string {
	var s []string
	for _, k := range n.knoten {
		s = append(s, k.addr)
	}
	return s
}

func (n *simNetz) baue(i int) {
	k := n.knoten[i]
	env := LeitUmgebung{
		Hoehe:      func() int64 { return int64(n.jetzt / time.Second) },
		HatBlock:   func(h string) bool { return k.hatBlock == nil || k.hatBlock(h) },
		Entleert:   func() bool { return true },
		LetzterBlk: func() string { return fmt.Sprintf("blk-%s-%d", k.addr, n.jetzt/time.Second) },
		Speichern:  func(s LeitSpeicher) { k.gespeich = s },
		Ueberholt:  func(uint64) { k.ueberholt++ },
	}
	faehig := n.faehig == nil || n.faehig[k.addr]
	k.l = NeueLeitung(k.addr, "http://"+k.addr, n.satz(), n.start, faehig, k.gespeich, n.cfg, env, n.uhr(i))
}

func neuesSimNetz(t *testing.T, anzahl int, seed int64, cfg LeitKonfig) *simNetz {
	n := &simNetz{t: t, r: rand.New(rand.NewSource(seed)), t0: time.Unix(1_790_000_000, 0), cfg: cfg, gruppe: map[int]int{}, gekappt: map[[2]int]bool{}}
	for i := 0; i < anzahl; i++ {
		n.knoten = append(n.knoten, &simKnoten{addr: fmt.Sprintf("0x%040d", i+1), an: true})
	}
	n.start = n.knoten[0].addr
	for i := range n.knoten {
		n.baue(i)
	}
	return n
}

func (n *simNetz) verbunden(a, b int) bool {
	if a > b {
		a, b = b, a
	}
	return n.knoten[a].an && n.knoten[b].an && n.gruppe[a] == n.gruppe[b] && !n.gekappt[[2]int{a, b}]
}

func (n *simNetz) sende(von int, m LeitNachricht, an int, antwort bool) {
	if n.r.Float64() < n.verlust {
		return
	}
	verz := time.Duration(5+n.r.Intn(150)) * time.Millisecond
	if n.festeVerz {
		verz = 10 * time.Millisecond
	}
	n.unterwegs = append(n.unterwegs, simNachricht{an: an, von: von, m: m, ankunft: n.jetzt + verz, antwort: antwort})
}

// schritt: 50 ms echte Zeit.
func (n *simNetz) schritt() {
	n.jetzt += 50 * time.Millisecond
	// Zustellen. Antworten, die dabei entstehen, landen in n.unterwegs --
	// deshalb erst die faelligen herausnehmen, dann zustellen.
	var bleiben, faellig []simNachricht
	for _, u := range n.unterwegs {
		if u.ankunft > n.jetzt {
			bleiben = append(bleiben, u)
		} else {
			faellig = append(faellig, u)
		}
	}
	n.unterwegs = bleiben
	for _, u := range faellig {
		if !n.verbunden(u.von, u.an) {
			continue
		}
		if a := n.knoten[u.an].l.Empfange(u.m, n.uhr(u.an)); a != nil && !u.antwort {
			n.sende(u.an, *a, u.von, true)
		}
	}
	// Takt.
	for i, k := range n.knoten {
		if !k.an {
			continue
		}
		for _, m := range k.l.Takt(n.uhr(i)) {
			for j := range n.knoten {
				if j != i {
					n.sende(i, m, j, false)
				}
			}
		}
	}
	// DIE EIGENSCHAFT.
	var annehmende []string
	for i, k := range n.knoten {
		if k.an && k.l.DarfAnnehmen(n.uhr(i)) {
			annehmende = append(annehmende, k.addr)
		}
	}
	if len(annehmende) > 1 {
		n.zweiLeiter = true
		n.t.Errorf("t=%s: ZWEI nehmen gleichzeitig an: %v", n.jetzt, annehmende)
	}
	if len(annehmende) == 1 {
		n.leiterZeit += 50 * time.Millisecond
		if annehmende[0] != n.letzterLtr {
			n.wechsel++
			n.letzterLtr = annehmende[0]
		}
	}
}

func (n *simNetz) laufe(d time.Duration) {
	ende := n.jetzt + d
	for n.jetzt < ende {
		n.schritt()
	}
}

func (n *simNetz) annehmender() int {
	for i, k := range n.knoten {
		if k.an && k.l.DarfAnnehmen(n.uhr(i)) {
			return i
		}
	}
	return -1
}

func testKonfig() LeitKonfig {
	c := leitVorgabe()
	c.WechselAlle = 0
	return c
}

func TestLeitung3_StartLeiterNimmtAn(t *testing.T) {
	n := neuesSimNetz(t, 3, 1, testKonfig())
	n.laufe(5 * time.Second)
	if i := n.annehmender(); i != 0 {
		t.Fatalf("nach 5 s nimmt %d an, erwartet der Startleiter 0", i)
	}
}

func TestLeitung3_AusfallDesLeiters(t *testing.T) {
	n := neuesSimNetz(t, 3, 2, testKonfig())
	n.laufe(10 * time.Second)
	n.knoten[0].an = false
	n.laufe(60 * time.Second)
	i := n.annehmender()
	if i != 1 && i != 2 {
		t.Fatalf("60 s nach dem Ausfall nimmt %d an, erwartet 1 oder 2", i)
	}
	// Der alte kommt zurueck (Neustart aus dem Gespeicherten).
	n.knoten[0].an = true
	n.baue(0)
	n.laufe(30 * time.Second)
	if n.annehmender() == 0 {
		t.Fatal("der zurueckgekehrte alte Leiter hat die Leitung wieder an sich gerissen")
	}
}

func TestLeitung3_NetztrennungDesLeiters(t *testing.T) {
	n := neuesSimNetz(t, 3, 3, testKonfig())
	n.laufe(10 * time.Second)
	n.gruppe[0] = 1 // Leiter allein
	n.laufe(3 * time.Second)
	// Innerhalb der LeaseDauer hoert der abgeschnittene auf.
	n.laufe(n.cfg.LeaseDauer)
	if n.knoten[0].l.DarfAnnehmen(n.uhr(0)) {
		t.Fatal("der abgeschnittene Leiter nimmt nach Ablauf seiner Lease noch an")
	}
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 1 && i != 2 {
		t.Fatalf("die Mehrheit hat keinen neuen Leiter: %d", i)
	}
	n.gruppe = map[int]int{}
	n.laufe(20 * time.Second)
	if n.knoten[0].ueberholt == 0 {
		t.Fatal("der alte Leiter hat nicht bemerkt, dass er ueberholt wurde -- er muss neu synchronisieren")
	}
	if i := n.annehmender(); i == 0 || i < 0 {
		t.Fatalf("nach der Heilung nimmt %d an", i)
	}
}

// Nicht transitive Trennung: A (Leiter) sieht B, B sieht C, A sieht C nicht.
// C bekommt keine Leases und will waehlen. B hat A gerade bestaetigt und darf
// C nicht waehlen, solange As Lease laeuft -- sonst naehmen A (mit Bs
// Bestaetigung) und C (mit Bs Stimme) gleichzeitig an.
func TestLeitung3_NichtTransitiveTrennung(t *testing.T) {
	n := neuesSimNetz(t, 3, 11, testKonfig())
	n.laufe(10 * time.Second)
	n.gekappt[[2]int{0, 2}] = true
	n.laufe(2 * time.Minute) // prueft in jedem Schritt: nie zwei
	if n.annehmender() < 0 {
		t.Fatal("bei nicht transitiver Trennung kein Leiter")
	}
}

// Gleichschritt: nach dem Ausfall treten die beiden uebrigen gleichzeitig an
// und teilen sich die Stimmen. Ohne gestaffelten Neuanlauf wiederholte sich
// das endlos (ueber HTTP gemessen: Term 6, kein Leiter).
func TestLeitung3_GeteilteStimmenLoesenSichAuf(t *testing.T) {
	for seed := int64(20); seed < 26; seed++ {
		n := neuesSimNetz(t, 3, seed, testKonfig())
		n.festeVerz = true
		n.laufe(10 * time.Second)
		n.knoten[0].an = false
		n.laufe(90 * time.Second)
		if i := n.annehmender(); i != 1 && i != 2 {
			t.Fatalf("seed %d: 90 s nach dem Ausfall kein Leiter (Gleichschritt)", seed)
		}
	}
}

func TestLeitung3_PlanmaessigerWechsel(t *testing.T) {
	c := testKonfig()
	c.WechselAlle = 30 * time.Second
	n := neuesSimNetz(t, 3, 4, c)
	n.laufe(5 * time.Minute)
	if n.wechsel < 5 {
		t.Fatalf("in 5 min nur %d Wechsel, erwartet etwa 10", n.wechsel)
	}
	for _, k := range n.knoten {
		if k.ueberholt > 0 {
			t.Fatalf("%s wurde bei planmaessigem Wechsel als ueberholt gemeldet", k.addr)
		}
	}
	if n.leiterZeit < 4*time.Minute {
		t.Fatalf("nur %s von 5 min mit Leiter -- der Wechsel dauert zu lange", n.leiterZeit)
	}
}

func TestLeitung3_WechselWartetAufDenBlock(t *testing.T) {
	c := testKonfig()
	c.WechselAlle = 20 * time.Second
	n := neuesSimNetz(t, 3, 5, c)
	// Der Naechste hat den letzten Block des alten noch nicht.
	for i := range n.knoten {
		k := n.knoten[i]
		k.hatBlock = func(string) bool { return false }
	}
	n.laufe(40 * time.Second)
	// Nach Uebergabe darf niemand annehmen, solange der Block fehlt ...
	var nach time.Duration
	for s := 0; s < 200; s++ {
		n.schritt()
		if n.annehmender() >= 0 {
			nach += 50 * time.Millisecond
		}
	}
	// ... bis die Mehrheit einen neuen waehlt (der alte hat aufgehoert und
	// schickt keine Leases mehr). Das ist der Ausweg, wenn der Neue haengt.
	for i := range n.knoten {
		n.knoten[i].hatBlock = nil
	}
	n.laufe(60 * time.Second)
	if n.annehmender() < 0 {
		t.Fatal("nach dem Haengen des Nachfolgers kommt kein Leiter mehr zustande")
	}
}

func TestLeitung_NichtFaehigerWirdNieLeiter(t *testing.T) {
	c := testKonfig()
	n := neuesSimNetz(t, 3, 6, c)
	n.faehig = map[string]bool{n.knoten[0].addr: true, n.knoten[1].addr: true}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(10 * time.Second)
	n.knoten[0].an = false
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 1 {
		t.Fatalf("nach Ausfall nimmt %d an, erwartet 1 (2 ist nicht leiterfaehig)", i)
	}
	n.knoten[1].an = false
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i == 2 {
		t.Fatal("ein nicht leiterfaehiger Knoten hat die Leitung uebernommen")
	}
}

// Keiner haelt den Leistungsnachweis: die Kette darf daran nicht stehen.
func TestLeitung3_NotbetriebKeinerFaehig(t *testing.T) {
	n := neuesSimNetz(t, 3, 11, testKonfig())
	n.faehig = map[string]bool{}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(10 * time.Second)
	if n.annehmender() != 0 {
		t.Fatalf("Startleiter ohne Nachweis nimmt nicht an (%d)", n.annehmender())
	}
	n.knoten[0].an = false
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 1 && i != 2 {
		t.Fatalf("Notbetrieb: nach Ausfall kein Leiter (%d)", i)
	}
}

// Startleiter ohne Nachweis gibt an einen leiterfaehigen ab; verliert der
// seinen, gibt er weiter.
func TestLeitung3_OhneNachweisGibtAb(t *testing.T) {
	n := neuesSimNetz(t, 3, 12, testKonfig())
	n.faehig = map[string]bool{n.knoten[2].addr: true}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(20 * time.Second)
	if i := n.annehmender(); i != 2 {
		t.Fatalf("nimmt %d an, erwartet 2 (einziger leiterfaehiger)", i)
	}
	n.knoten[2].l.SetzeFaehig(false)
	n.knoten[1].l.SetzeFaehig(true)
	n.laufe(20 * time.Second)
	if i := n.annehmender(); i != 1 {
		t.Fatalf("nimmt %d an, erwartet 1 nach Verlust des Nachweises", i)
	}
	// Einziger leiterfaehiger faellt aus: Notbetrieb statt Stillstand.
	n.knoten[1].an = false
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 0 && i != 2 {
		t.Fatalf("kein Leiter, nachdem der einzige leiterfaehige ausfiel (%d)", i)
	}
	// Er kommt zurueck: die Leitung geht wieder an ihn.
	n.knoten[1].an = true
	n.baue(1)
	n.knoten[1].l.SetzeFaehig(true)
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 1 {
		t.Fatalf("nimmt %d an, erwartet den zurueckgekehrten leiterfaehigen 1", i)
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Wie TestLeitung_Zufall, dazu wechselnder Leistungsnachweis. Am Ende,
// alles heil: genau einer nimmt an, und er haelt den Nachweis.
func TestLeitung_ZufallMitNachweis(t *testing.T) {
	for seed := int64(200); seed < 220; seed++ {
		c := testKonfig()
		c.WechselAlle = 45 * time.Second
		n := neuesSimNetz(t, 5, seed, c)
		n.verlust = 0.1
		n.faehig = map[string]bool{}
		for i, k := range n.knoten {
			n.faehig[k.addr] = n.r.Intn(2) == 0
			k.gang = (n.r.Float64()*2 - 1) * 0.002
			n.baue(i)
		}
		for runde := 0; runde < 40; runde++ {
			switch n.r.Intn(6) {
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
			case 4: // Nachweis wechselt
				k := n.knoten[n.r.Intn(5)]
				n.faehig[k.addr] = !n.faehig[k.addr]
				k.l.SetzeFaehig(n.faehig[k.addr])
			default:
			}
			n.laufe(time.Duration(5+n.r.Intn(40)) * time.Second)
		}
		for i, k := range n.knoten {
			if !k.an {
				k.an = true
				n.baue(i)
			}
		}
		n.gruppe = map[int]int{}
		n.gekappt = map[[2]int]bool{}
		n.laufe(2 * time.Minute)
		i := n.annehmender()
		if i < 0 {
			t.Errorf("seed %d: nach Heilung kein Leiter", seed)
			continue
		}
		einer := false
		for _, f := range n.faehig {
			einer = einer || f
		}
		if einer && !n.faehig[n.knoten[i].addr] {
			t.Errorf("seed %d: Leiter %d ohne Nachweis, obwohl ein leiterfaehiger lebt", seed, i)
		}
		if n.zweiLeiter {
			t.Fatalf("seed %d: zwei Leiter gleichzeitig", seed)
		}
	}
}

func TestLeitung2_KeineAutomatischeUebernahme(t *testing.T) {
	n := neuesSimNetz(t, 2, 7, testKonfig())
	n.laufe(5 * time.Second)
	if n.annehmender() != 0 {
		t.Fatal("bei zwei nimmt der Startleiter nicht an")
	}
	// Folger faellt aus: der Leiter macht weiter (sonst stuende das Netz,
	// sobald der ANDERE ausfaellt).
	n.knoten[1].an = false
	n.laufe(60 * time.Second)
	if n.annehmender() != 0 {
		t.Fatal("bei zwei hat der Leiter aufgehoert, weil der Folger ausfiel")
	}
	n.knoten[1].an = true
	n.baue(1)
	n.laufe(10 * time.Second)
	// Leiter faellt aus: der andere uebernimmt NICHT (tot oder getrennt ist
	// von aussen nicht zu unterscheiden).
	n.knoten[0].an = false
	n.laufe(2 * time.Minute)
	if n.annehmender() == 1 {
		t.Fatal("bei zwei hat der Folger automatisch uebernommen -- bei einer Netztrennung waeren das zwei Leiter")
	}
}

func TestLeitung2_FrischerKnotenReisstNichtsAnSich(t *testing.T) {
	n := neuesSimNetz(t, 2, 8, testKonfig())
	// Knoten 0 leitet laengst Term 7; Knoten 1 kommt mit geloeschter
	// Datenbank und haelt sich laut Startleiter-Einstellung fuer Leiter.
	n.knoten[0].gespeich = LeitSpeicher{Term: 7, Leiter: n.knoten[0].addr, WarLeiter: true}
	n.start = n.knoten[1].addr
	n.baue(0)
	n.baue(1)
	for s := 0; s < 400; s++ {
		n.schritt() // prueft in jedem Schritt: nie zwei
	}
	if n.annehmender() != 0 {
		t.Fatalf("nimmt %d an, erwartet der laufende Leiter 0", n.annehmender())
	}
}

func TestLeitung2_WechselWennEingeschaltetUndVerlust(t *testing.T) {
	c := testKonfig()
	c.WechselAlle = 20 * time.Second
	c.ZweiWechseln = true
	n := neuesSimNetz(t, 2, 9, c)
	n.verlust = 0.3 // auch die Uebergabe geht mal verloren
	n.laufe(3 * time.Minute)
	if n.wechsel < 4 {
		t.Fatalf("nur %d Wechsel bei zwei Validatoren", n.wechsel)
	}
}

// Zufall ueber viele Laeufe: Ausfaelle, Neustarts, Netztrennungen, Verlust,
// Uhrengang. Die Eigenschaft muss IMMER gelten; ausserdem muss es die meiste
// Zeit einen Leiter geben, wenn eine Mehrheit verbunden ist.
func TestLeitung_Zufall(t *testing.T) {
	for seed := int64(100); seed < 130; seed++ {
		c := testKonfig()
		c.WechselAlle = 45 * time.Second
		n := neuesSimNetz(t, 5, seed, c)
		n.verlust = 0.1
		for i, k := range n.knoten {
			k.gang = (n.r.Float64()*2 - 1) * 0.002 // +-0,2 %
			n.baue(i)
		}
		for runde := 0; runde < 40; runde++ {
			switch n.r.Intn(6) {
			case 0: // Ausfall
				n.knoten[n.r.Intn(5)].an = false
			case 1: // Neustart aus dem Gespeicherten
				i := n.r.Intn(5)
				if !n.knoten[i].an {
					n.knoten[i].an = true
					n.baue(i)
				}
			case 2: // Netztrennung
				n.gruppe = map[int]int{}
				for i := range n.knoten {
					n.gruppe[i] = n.r.Intn(2)
				}
			case 3: // Heilung
				n.gruppe = map[int]int{}
				n.gekappt = map[[2]int]bool{}
			case 4: // einzelne Verbindungen kappen (nicht transitiv)
				for k := 0; k < 3; k++ {
					a, b := n.r.Intn(5), n.r.Intn(5)
					if a > b {
						a, b = b, a
					}
					if a != b {
						n.gekappt[[2]int{a, b}] = true
					}
				}
			default:
			}
			n.laufe(time.Duration(5+n.r.Intn(40)) * time.Second)
		}
		// Am Ende alles heil: es muss wieder genau einen geben.
		for i, k := range n.knoten {
			if !k.an {
				k.an = true
				n.baue(i)
			}
		}
		n.gruppe = map[int]int{}
		n.gekappt = map[[2]int]bool{}
		n.laufe(2 * time.Minute)
		if n.annehmender() < 0 {
			t.Errorf("seed %d: nach vollstaendiger Heilung kein Leiter", seed)
		}
		if n.zweiLeiter {
			t.Fatalf("seed %d: zwei Leiter gleichzeitig", seed)
		}
	}
}

func TestMehrheitUndNachfolger(t *testing.T) {
	l := NeueLeitung("0xb", "", []string{"0xc", "0xa", "0xb"}, "0xa", true, LeitSpeicher{}, testKonfig(), LeitUmgebung{}, time.Now())
	l.faehig = map[string]bool{"0xa": true, "0xb": true, "0xc": true}
	if l.mehrheit() != 2 {
		t.Fatalf("Mehrheit von 3 = %d", l.mehrheit())
	}
	got := []string{l.nachfolger("0xa", false), l.nachfolger("0xb", false), l.nachfolger("0xc", false)}
	want := []string{"0xb", "0xc", "0xa"}
	sort.Strings(got)
	sort.Strings(want)
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("nachfolger: %v", got)
		}
	}
}
