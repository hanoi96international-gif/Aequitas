package bereiche

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// SIMULATION VIELER KNOTEN (Stufe 3).
//
// 40 Validatoren, 4 Bereiche, Ausschuesse zu 8, 120 Menschen, Epoche fuer
// Epoche: Registrierungen, Ueberweisungen innerhalb und zwischen Bereichen,
// absichtlich ungueltige (falsche Nonce, Signatur, Deckung), Ausstiege.
//
// Geprueft wird:
//   - jeder Bereichsblock besteht die billige Pruefung jedes Knotens;
//   - jede Epoche: Erhaltung der Geldmenge ueber das ganze Netz;
//   - jede Quittung wird genau einmal eingeloest;
//   - am Ende stimmt jedes Konto mit einem unabhaengig gefuehrten Hauptbuch
//     ueberein (aus den angewandten Ueberweisungen, Quittungen, Grundeinkommen
//     und Kappungen);
//   - ein gekaperter Ausschuss, der stiehlt und das Gestohlene per Ausstieg
//     zu Geld machen will, faellt per Fehlerbeweis auf, bekommt keine
//     Auszahlung, wird ausgeschlossen, und das Netz landet beim selben
//     Zustand wie ein Lauf ohne Angriff.

type simPerson struct {
	addr string
	priv ed25519.PrivateKey
}

func neuePerson(r *rand.Rand) simPerson {
	seed := make([]byte, ed25519.SeedSize)
	r.Read(seed)
	priv := ed25519.NewKeyFromSeed(seed)
	return simPerson{addr: hex.EncodeToString(priv.Public().(ed25519.PublicKey)), priv: priv}
}

type simEpocheEingaben struct {
	reg map[int][]Registrierung
	txs map[int][]Ueberweisung
}

type simNetz struct {
	t              *testing.T
	s, m           int
	validatoren    []simPerson
	byzantinisch   map[string]bool
	ausgeschlossen map[string]bool
	menschen       []simPerson
	// Zustand, den ehrliche Knoten halten (je Bereich) -- die Beobachter, die
	// Fehlerbeweise bauen koennen.
	zustaende  []*VollZustand
	letzter    []*BereichsBlock
	sammel     []Sammelblock // Index = Epoche
	offen      map[Hash]Quittung
	eingeloest map[Hash]bool
	ausgezahlt int64
	// Protokoll fuer das unabhaengige Hauptbuch.
	angewandt         []Ueberweisung
	quittungenEingang []Quittung
	// Angriff
	angriffEpoche  int
	angriffBereich int
	beute          simPerson
	entdeckt       int
	// Schnappschuesse je Epochenbeginn, fuer "Stopp beim letzten
	// bestaetigten Stand".
	snap []simSnap
	// verzug: Beobachter pruefen einen Block erst so viele Epochen spaeter
	// (0 = sofort). Dazwischen baut das Netz auf dem falschen Block weiter.
	verzug         int
	offenZuPruefen []simPruefung
	// Die Beute gibt das Gestohlene weiter -- nach dem Ruecklauf muss auch
	// das verschwinden.
	beuteWeiter bool
}

type simPruefung struct {
	epoche    int
	blk       BereichsBlock
	vor       *VollZustand
	rahmen    Rahmen
	ausschuss []string
}

type simSnap struct {
	zustaende  []*VollZustand
	letzter    []*BereichsBlock
	offen      map[Hash]Quittung
	eingeloest map[Hash]bool
	ausgezahlt int64
	angewandt  int
	qein       int
}

func (n *simNetz) sichere() simSnap {
	s := simSnap{offen: map[Hash]Quittung{}, eingeloest: map[Hash]bool{}, ausgezahlt: n.ausgezahlt,
		angewandt: len(n.angewandt), qein: len(n.quittungenEingang)}
	for i, z := range n.zustaende {
		s.zustaende = append(s.zustaende, z.Kopie())
		if n.letzter[i] != nil {
			b := *n.letzter[i]
			s.letzter = append(s.letzter, &b)
		} else {
			s.letzter = append(s.letzter, nil)
		}
	}
	for k, v := range n.offen {
		s.offen[k] = v
	}
	for k, v := range n.eingeloest {
		s.eingeloest[k] = v
	}
	return s
}

func (n *simNetz) zurueck(s simSnap) {
	n.zustaende, n.letzter = nil, nil
	for i := range s.zustaende {
		n.zustaende = append(n.zustaende, s.zustaende[i].Kopie())
		if s.letzter[i] != nil {
			b := *s.letzter[i]
			n.letzter = append(n.letzter, &b)
		} else {
			n.letzter = append(n.letzter, nil)
		}
	}
	n.offen, n.eingeloest = map[Hash]Quittung{}, map[Hash]bool{}
	for k, v := range s.offen {
		n.offen[k] = v
	}
	for k, v := range s.eingeloest {
		n.eingeloest[k] = v
	}
	n.ausgezahlt = s.ausgezahlt
	n.angewandt = n.angewandt[:s.angewandt]
	n.quittungenEingang = n.quittungenEingang[:s.qein]
}

func neuesSimNetz(t *testing.T, seed int64) *simNetz {
	r := rand.New(rand.NewSource(seed))
	n := &simNetz{t: t, s: 4, m: 8, byzantinisch: map[string]bool{}, ausgeschlossen: map[string]bool{},
		offen: map[Hash]Quittung{}, eingeloest: map[Hash]bool{}, angriffEpoche: -1}
	for i := 0; i < 40; i++ {
		n.validatoren = append(n.validatoren, neuePerson(r))
	}
	for i := 0; i < 120; i++ {
		n.menschen = append(n.menschen, neuePerson(r))
	}
	n.beute = neuePerson(r)
	for i := 0; i < n.s; i++ {
		n.zustaende = append(n.zustaende, NeuerVollZustand())
		n.letzter = append(n.letzter, nil)
	}
	n.sammel = []Sammelblock{{}}
	n.snap = []simSnap{{}} // Index = Epoche; 0 ungenutzt
	return n
}

func (n *simNetz) waehlbar() []string {
	var v []string
	for _, p := range n.validatoren {
		if !n.ausgeschlossen[p.addr] {
			v = append(v, p.addr)
		}
	}
	return v
}

func (n *simNetz) schluessel(addr string) ed25519.PrivateKey {
	for _, p := range n.validatoren {
		if p.addr == addr {
			return p.priv
		}
	}
	return nil
}

// eingabenFuer: zufaellige, zum Teil absichtlich ungueltige Auftraege.
func (n *simNetz) eingabenFuer(e int, r *rand.Rand) simEpocheEingaben {
	in := simEpocheEingaben{reg: map[int][]Registrierung{}, txs: map[int][]Ueberweisung{}}
	if e == 1 {
		for _, p := range n.menschen {
			b := BereichVon(p.addr, n.s)
			in.reg[b] = append(in.reg[b], Registrierung{Addr: p.addr})
		}
		in.reg[BereichVon(n.beute.addr, n.s)] = append(in.reg[BereichVon(n.beute.addr, n.s)], Registrierung{Addr: n.beute.addr})
		return in
	}
	// Noncen aus dem Zustand der ehrlichen Knoten -- der Absender kennt sie.
	nonce := map[string]uint64{}
	if n.beuteWeiter && e == n.angriffEpoche+1 {
		// Die Beute gibt 400 AEQ weiter -- auf dem falschen Stand gedeckt,
		// auf dem richtigen nicht (sie hatte nur den Grundbetrag, 1.000 AEQ,
		// und gibt 1.200 aus).
		b := BereichVon(n.beute.addr, n.s)
		k, _ := n.zustaende[b].Konto(n.beute.addr)
		u := Ueberweisung{Von: n.beute.addr, An: n.menschen[0].addr, Betrag: 1_200 * Mikro, Nonce: k.Nonce}
		u.Signiere(n.beute.priv)
		in.txs[b] = append(in.txs[b], u)
	}
	for i := 0; i < 180; i++ {
		von := n.menschen[r.Intn(len(n.menschen))]
		an := n.menschen[r.Intn(len(n.menschen))]
		if von.addr == an.addr {
			continue
		}
		b := BereichVon(von.addr, n.s)
		if _, ok := nonce[von.addr]; !ok {
			k, _ := n.zustaende[b].Konto(von.addr)
			nonce[von.addr] = k.Nonce
		}
		u := Ueberweisung{Von: von.addr, An: an.addr, Betrag: int64(1+r.Intn(300)) * Mikro / 10, Nonce: nonce[von.addr]}
		switch r.Intn(20) {
		case 0:
			u.Nonce += 5 // falsche Nonce
		case 1:
			u.Betrag = 50_000 * Mikro // nicht gedeckt
		case 2:
			u.Ausstieg, u.An = true, ""
		}
		u.Signiere(von.priv)
		if r.Intn(30) == 0 {
			u.Sig[0] ^= 0xff // falsche Signatur
		} else if u.Nonce == nonce[von.addr] {
			nonce[von.addr]++ // wird (vermutlich) angewandt
		}
		in.txs[b] = append(in.txs[b], u)
	}
	return in
}

// epoche: eine Epoche ablaufen lassen. Gibt false zurueck, wenn ein
// Fehlerbeweis einen Ruecklauf ausgeloest hat (die Epoche wird dann vom
// Aufrufer neu gerechnet).
func (n *simNetz) epoche(e int, in simEpocheEingaben) (bool, int) {
	t := n.t
	// Verzoegerte Beobachter: Bloecke von vor `verzug` Epochen nachrechnen.
	var rest []simPruefung
	for _, p := range n.offenZuPruefen {
		if p.epoche > e-n.verzug {
			rest = append(rest, p)
			continue
		}
		soll, _ := Ausfuehren(p.vor.Kopie(), p.blk.Bereich, p.rahmen, p.blk.Eingaben, p.blk.EingabenID())
		if soll.Wurzel == p.blk.Nach {
			continue
		}
		fb := ErstelleFehlerbeweis(p.vor, p.blk, p.rahmen)
		if falsch, err := PruefeFehlerbeweis(fb, p.rahmen); err != nil || !falsch {
			t.Fatalf("Fehlerbeweis nicht anerkannt: %v", err)
		}
		n.entdeckt++
		for a := range p.blk.Sig {
			n.ausgeschlossen[a] = true
		}
		// Stopp beim letzten bestaetigten Stand: vor der Epoche des falschen
		// Blocks. Alles danach wird neu gerechnet.
		n.zurueck(n.snap[p.epoche])
		n.sammel = n.sammel[:p.epoche]
		n.offenZuPruefen = nil
		return false, p.epoche
	}
	n.offenZuPruefen = rest
	vorSB := n.sammel[e-1]
	r := vorSB.Rahmen(n.s)
	ausschuesse, err := Ausschuesse(n.waehlbar(), n.s, n.m, vorSB.Zufall())
	if err != nil {
		t.Fatal(err)
	}
	// Eingehende Quittungen: alle offenen aus frueheren Epochen, je Ziel.
	eingang := map[int][]Quittung{}
	var ids []Hash
	for id := range n.offen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return string(ids[i][:]) < string(ids[j][:]) })
	for _, id := range ids {
		q := n.offen[id]
		eingang[q.NachBereich] = append(eingang[q.NachBereich], q)
	}

	var bloecke []BereichsBlock
	var neueQ []Quittung
	for b := 0; b < n.s; b++ {
		blk := BereichsBlock{Bereich: b, Epoche: uint64(e), Rahmen: r,
			Eingaben: Eingaben{Eingang: eingang[b], Registrierung: in.reg[b], Ueberweisungen: in.txs[b]}}
		if n.letzter[b] != nil {
			blk.Nr, blk.Vor = n.letzter[b].Nr+1, n.letzter[b].Nach
		} else {
			blk.Vor = n.zustaende[b].Wurzel()
		}
		// Der Ausschuss rechnet (ehrlich: auf dem Zustand der Ehrlichen).
		arbeit := n.zustaende[b].Kopie()
		erg, err := Ausfuehren(arbeit, b, r, blk.Eingaben, blk.EingabenID())
		if err != nil {
			t.Fatalf("Epoche %d Bereich %d: %v", e, b, err)
		}
		// Gekapert ist ein Ausschuss nur, wenn mindestens 2/3 seiner Mitglieder
		// byzantinisch sind -- nach dem Ausschluss ist das nicht mehr der Fall.
		byz := 0
		for _, a := range ausschuesse[b] {
			if n.byzantinisch[a] {
				byz++
			}
		}
		gekapert := e == n.angriffEpoche && b == n.angriffBereich && byz >= Mehrheit(n.m)
		if gekapert {
			// Diebstahl: 500 AEQ vom reichsten Menschen des Bereichs an die
			// Beute -- Geldmenge bleibt gleich, die billige Pruefung merkt nichts.
			var opfer string
			var best int64
			for a, k := range arbeit.konten {
				if k.Mensch && a != n.beute.addr && k.Guthaben > best {
					opfer, best = a, k.Guthaben
				}
			}
			ko, _ := arbeit.Konto(opfer)
			ko.Guthaben -= 500 * Mikro
			arbeit.SetzeKonto(opfer, ko)
			kb, _ := arbeit.Konto(n.beute.addr)
			if BereichVon(n.beute.addr, n.s) != b {
				t.Fatal("Testaufbau: Beute muss im gekaperten Bereich liegen")
			}
			kb.Guthaben += 500 * Mikro
			arbeit.SetzeKonto(n.beute.addr, kb)
			erg.Wurzel = arbeit.Wurzel()
		}
		blk.Nach, blk.Ausgang, blk.Teilsumme = erg.Wurzel, erg.Ausgang, erg.Teilsumme
		g, m := arbeit.Geldmenge()
		blk.Geldmenge, blk.Menschen = g, m
		for _, a := range ausschuesse[b] {
			if !gekapert || n.byzantinisch[a] {
				blk.Unterschreibe(a, n.schluessel(a))
			}
		}
		var vorher *BereichsBlock
		if n.letzter[b] != nil {
			vorher = n.letzter[b]
		}
		if err := BilligPruefen(blk, vorher, ausschuesse[b], r); err != nil {
			t.Fatalf("Epoche %d: billige Pruefung: %v", e, err)
		}

		// Beobachter: ein ehrlicher Knoten, der den Bereich haelt, rechnet
		// nach. Falsch? Fehlerbeweis an alle. Mit Verzug erst spaeter --
		// dann entscheidet bei Ausstiegen allein der Pruefausschuss.
		pruef := n.zustaende[b].Kopie()
		soll, _ := Ausfuehren(pruef, b, r, blk.Eingaben, blk.EingabenID())
		if n.verzug > 0 {
			n.offenZuPruefen = append(n.offenZuPruefen, simPruefung{epoche: e, blk: blk, vor: n.zustaende[b].Kopie(), rahmen: r, ausschuss: ausschuesse[b]})
		}
		if n.verzug == 0 && soll.Wurzel != blk.Nach {
			fb := ErstelleFehlerbeweis(n.zustaende[b], blk, r)
			falsch, err := PruefeFehlerbeweis(fb, r) // jeder Knoten prueft den Beweis selbst
			if err != nil || !falsch {
				t.Fatalf("Epoche %d: Fehlerbeweis nicht anerkannt: %v", e, err)
			}
			n.entdeckt++
			// Ausschuss aus -- alle, die unterschrieben haben.
			for a := range blk.Sig {
				n.ausgeschlossen[a] = true
			}
			// Der Pruefausschuss (Ausstiege!) haette nie bestaetigt:
			pa := Pruefausschuss(n.waehlbar(), ausschuesse[b], n.m, vorSB.Zufall(), b)
			for _, a := range pa {
				if !n.byzantinisch[a] {
					continue // ehrliche Pruefer rechnen nach und unterschreiben nicht
				}
				blk.PruefUnterschrift(a, n.schluessel(a))
			}
			if blk.GeprueftVon(pa) {
				t.Fatal("gefaelschter Block vom Pruefausschuss bestaetigt -- Ausstieg waere ausgezahlt worden")
			}
			// Stopp beim letzten bestaetigten Stand: Ruecklauf auf den
			// Beginn dieser Epoche, neu rechnen (Aufrufer).
			n.zurueck(n.snap[e])
			n.sammel = n.sammel[:e]
			return false, e
		}
		// Ausstiege: erst nach dem Pruefausschuss.
		if blk.Teilsumme.Ausstiege > 0 {
			pa := Pruefausschuss(n.waehlbar(), ausschuesse[b], n.m, vorSB.Zufall(), b)
			for _, a := range pa {
				blk.PruefUnterschrift(a, n.schluessel(a))
			}
			// Der Pruefausschuss rechnet selbst nach (ehrlich: auf dem
			// Zustand der Ehrlichen) und unterschreibt nur einen richtigen
			// Block -- gerade wenn die Beobachter noch nicht so weit sind.
			if soll.Wurzel != blk.Nach {
				t.Fatalf("Epoche %d: falscher Block mit Ausstieg -- der Pruefausschuss haelt die Auszahlung an", e)
			}
			if !blk.GeprueftVon(pa) {
				t.Fatalf("ehrlicher Block nicht vom Pruefausschuss bestaetigt")
			}
			n.ausgezahlt += blk.Teilsumme.Ausstiege
		}
		// Uebernehmen.
		n.zustaende[b] = arbeit
		kopie := blk
		n.letzter[b] = &kopie
		bloecke = append(bloecke, blk)
		for _, q := range blk.Eingaben.Eingang {
			if n.eingeloest[q.ID] {
				t.Fatalf("Quittung %x doppelt eingeloest", q.ID[:4])
			}
			n.eingeloest[q.ID] = true
			delete(n.offen, q.ID)
			n.quittungenEingang = append(n.quittungenEingang, q)
		}
		neueQ = append(neueQ, blk.Ausgang...)
		for i, u := range blk.Eingaben.Ueberweisungen {
			if erg.Angewandt[i] {
				n.angewandt = append(n.angewandt, u)
			}
		}
	}
	for _, q := range neueQ {
		n.offen[q.ID] = q
	}
	var offen int64
	for _, q := range n.offen {
		offen += q.Betrag
	}
	var geld []int64
	var menschen []int
	for _, z := range n.zustaende {
		g, m := z.Geldmenge()
		geld = append(geld, g)
		menschen = append(menschen, m)
	}
	sb := NaechsterSammelblock(vorSB, geld, menschen, bloecke, offen)
	if !sb.Erhalten() {
		t.Fatalf("Epoche %d: Geldmenge nicht erhalten: %+v", e, sb)
	}
	if sb.Ausgestiegen != n.ausgezahlt {
		t.Fatalf("Epoche %d: ausgezahlt %d, laut Sammelblock %d", e, n.ausgezahlt, sb.Ausgestiegen)
	}
	n.sammel = append(n.sammel, sb)
	return true, 0
}

func (n *simNetz) lauf(epochen int, seed int64) {
	r := rand.New(rand.NewSource(seed))
	var alle []simEpocheEingaben
	leer := simEpocheEingaben{reg: map[int][]Registrierung{}, txs: map[int][]Ueberweisung{}}
	// Nachlauf: verzug+1 Epochen ohne neue Auftraege, damit alle Quittungen
	// ankommen und jeder Block geprueft ist.
	ende := epochen + 1 + n.verzug
	for e := 1; e <= ende; {
		if e > len(alle) {
			if e <= epochen {
				alle = append(alle, n.eingabenFuer(e, r))
			} else {
				alle = append(alle, leer)
			}
		}
		n.snap = append(n.snap[:e], n.sichere())
		ok, zurueck := n.epoche(e, alle[e-1])
		if !ok {
			e = zurueck
			continue
		}
		e++
	}
	if len(n.offen) != 0 {
		n.t.Fatalf("%d Quittungen nie eingeloest", len(n.offen))
	}
}

// hauptbuchPruefen: jedes Konto aus den angewandten Auftraegen nachrechnen.
func (n *simNetz) hauptbuchPruefen() {
	t := n.t
	buch := map[string]int64{}
	for _, p := range append(append([]simPerson(nil), n.menschen...), n.beute) {
		buch[p.addr] = Grundbetrag
	}
	for _, u := range n.angewandt {
		buch[u.Von] -= u.Betrag + u.Betrag*gebuehrBps/10_000
		if !u.Ausstieg && BereichVon(u.An, n.s) == BereichVon(u.Von, n.s) {
			buch[u.An] += u.Betrag
		}
	}
	for _, q := range n.quittungenEingang {
		buch[q.An] += q.Betrag
	}
	// Grundeinkommen: was jedes Konto bis zu seinem GEStand bekommen hat.
	// Kappungen: in dieser Simulation liegt niemand ueber der Grenze.
	for addr, soll := range buch {
		k, _ := n.zustaende[BereichVon(addr, n.s)].Konto(addr)
		if k.Guthaben-k.GEStand != soll {
			t.Fatalf("Konto %s: Zustand %d (ohne Grundeinkommen %d), Hauptbuch %d", addr[:8], k.Guthaben, k.Guthaben-k.GEStand, soll)
		}
	}
}

func TestSimulation_VieleKnotenEhrlich(t *testing.T) {
	n := neuesSimNetz(t, 1)
	n.lauf(10, 100)
	n.hauptbuchPruefen()
	letzter := n.sammel[len(n.sammel)-1]
	if letzter.GEKumuliert == 0 || n.ausgezahlt == 0 || len(n.quittungenEingang) == 0 {
		t.Fatalf("Simulation zu schwach: GE %d, Ausstiege %d, Quittungen %d", letzter.GEKumuliert, n.ausgezahlt, len(n.quittungenEingang))
	}
	if len(n.angewandt) < 500 {
		t.Fatalf("nur %d angewandte Ueberweisungen", len(n.angewandt))
	}
}

// Gekaperter Ausschuss: 6 von 8 Mitgliedern eines Bereichs sind
// byzantinisch (mehr als 2/3). Er stiehlt in Epoche 5. Erwartet: entdeckt,
// ausgeschlossen, keine Auszahlung, und am Ende derselbe Zustand wie ohne
// Angriff.
func TestSimulation_GekaperterAusschussFaelltAuf(t *testing.T) {
	ehrlich := neuesSimNetz(t, 2)
	ehrlich.lauf(8, 200)

	n := neuesSimNetz(t, 2)
	n.angriffEpoche = 5
	n.angriffBereich = BereichVon(n.beute.addr, n.s)
	// Die Losung der Epoche 5 ist erst bekannt, wenn Epoche 4 durch ist. Der
	// Angreifer "kontrolliert" im Test einfach 6 der 8 dann Gelosten.
	vorlauf := neuesSimNetz(t, 2)
	vorlauf.lauf(4, 200)
	aus, _ := Ausschuesse(vorlauf.waehlbar(), n.s, n.m, vorlauf.sammel[4].Zufall())
	for _, a := range aus[n.angriffBereich][:6] {
		n.byzantinisch[a] = true
	}
	n.lauf(8, 200)

	if n.entdeckt != 1 {
		t.Fatalf("Angriff %d-mal entdeckt statt einmal", n.entdeckt)
	}
	if len(n.ausgeschlossen) != 6 {
		t.Fatalf("%d ausgeschlossen statt der 6 Unterzeichner", len(n.ausgeschlossen))
	}
	for a := range n.ausgeschlossen {
		if !n.byzantinisch[a] {
			t.Fatalf("ehrlicher Validator %s ausgeschlossen", a[:8])
		}
	}
	// Derselbe Zustand wie ohne Angriff: Kontostaende und Auszahlungen. (Die
	// Wurzeln der Bloecke koennen abweichen, weil nach dem Ausschluss anders
	// gelost wird -- die Konten nicht.)
	for b := 0; b < n.s; b++ {
		if n.zustaende[b].Wurzel() != ehrlich.zustaende[b].Wurzel() {
			t.Errorf("Bereich %d: Zustand weicht vom ehrlichen Lauf ab", b)
		}
	}
	if n.ausgezahlt != ehrlich.ausgezahlt {
		t.Fatalf("ausgezahlt %d statt %d", n.ausgezahlt, ehrlich.ausgezahlt)
	}
	kb, _ := n.zustaende[n.angriffBereich].Konto(n.beute.addr)
	ke, _ := ehrlich.zustaende[n.angriffBereich].Konto(n.beute.addr)
	if kb.Guthaben != ke.Guthaben {
		t.Fatalf("Beute %d statt %d", kb.Guthaben, ke.Guthaben)
	}
	n.hauptbuchPruefen()
}

// Ein Fehlerbeweis gegen einen RICHTIGEN Block wird nicht anerkannt, und ein
// Beweis mit fehlenden Konten taugt nichts.
func TestFehlerbeweis_NurEchteFehler(t *testing.T) {
	n := neuesSimNetz(t, 3)
	n.lauf(3, 300)
	b := 0
	blk := n.letzter[b]
	// n.letzter[b] ist der Block der Epoche 4 (Nachlauf); sein Vorzustand
	// ist der Stand am Beginn von Epoche 4.
	vor := n.snap[4].zustaende[b]
	r := n.sammel[3].Rahmen(n.s)
	fb := ErstelleFehlerbeweis(vor, *blk, r)
	if falsch, err := PruefeFehlerbeweis(fb, r); err != nil || falsch {
		t.Fatalf("richtiger Block als falsch erkannt: %v %v", falsch, err)
	}
	if len(fb.Vor.Konten) > 1 {
		fb.Vor.Konten = fb.Vor.Konten[1:]
		if _, err := PruefeFehlerbeweis(fb, r); err == nil {
			t.Fatal("Beweis mit fehlendem Konto angenommen")
		}
	}
}

func TestLosung_UndAktivierung(t *testing.T) {
	if Aktiv(31, 1, 16) || Aktiv(40, 4, 16) || !Aktiv(64, 4, 16) || !Aktiv(32, 2, 16) {
		t.Fatal("Aktivierungsregel falsch")
	}
	n := neuesSimNetz(t, 4)
	v := n.waehlbar()
	a1, _ := Ausschuesse(v, 4, 8, Hash{1})
	a2, _ := Ausschuesse(v, 4, 8, Hash{1})
	a3, _ := Ausschuesse(v, 4, 8, Hash{2})
	if fmt.Sprint(a1) != fmt.Sprint(a2) || fmt.Sprint(a1) == fmt.Sprint(a3) {
		t.Fatal("Losung nicht deterministisch oder nicht zufallsabhaengig")
	}
	gesehen := map[string]bool{}
	for _, aus := range a1 {
		for _, a := range aus {
			if gesehen[a] {
				t.Fatal("Validator in zwei Ausschuessen")
			}
			gesehen[a] = true
		}
	}
	pa := Pruefausschuss(v, a1[0], 8, Hash{1}, 0)
	for _, a := range pa {
		for _, b := range a1[0] {
			if a == b {
				t.Fatal("Pruefausschuss enthaelt ein Mitglied des eigenen Ausschusses")
			}
		}
	}
	if _, err := Ausschuesse(v[:20], 4, 8, Hash{}); err == nil {
		t.Fatal("zu wenige Validatoren angenommen")
	}
}

// Der Betrug wird erst zwei Epochen spaeter gefunden; dazwischen hat die
// Beute das Gestohlene schon weitergegeben. Erwartet: Stopp beim letzten
// bestaetigten Stand, die Zeit danach neu gerechnet -- und am Ende derselbe
// Zustand wie ohne Angriff (die Weitergabe scheitert dort an der Deckung).
func TestSimulation_SpaetEntdeckterBetrugWirdZurueckgerechnet(t *testing.T) {
	ehrlich := neuesSimNetz(t, 5)
	ehrlich.verzug = 2
	ehrlich.angriffEpoche = 5
	ehrlich.beuteWeiter = true
	ehrlich.angriffBereich = BereichVon(ehrlich.beute.addr, ehrlich.s)
	ehrlich.lauf(9, 500)
	if ehrlich.entdeckt != 0 {
		t.Fatal("ehrlicher Lauf fand einen Betrug")
	}

	vorlauf := neuesSimNetz(t, 5)
	vorlauf.verzug = 2
	vorlauf.lauf(4, 500)
	n := neuesSimNetz(t, 5)
	n.verzug = 2
	n.angriffEpoche = 5
	n.beuteWeiter = true
	n.angriffBereich = BereichVon(n.beute.addr, n.s)
	aus, _ := Ausschuesse(vorlauf.waehlbar(), n.s, n.m, vorlauf.sammel[4].Zufall())
	for _, a := range aus[n.angriffBereich][:6] {
		n.byzantinisch[a] = true
	}
	n.lauf(9, 500)
	if n.entdeckt != 1 {
		t.Fatalf("%d Entdeckungen statt 1", n.entdeckt)
	}
	for b := 0; b < n.s; b++ {
		if n.zustaende[b].Wurzel() != ehrlich.zustaende[b].Wurzel() {
			t.Errorf("Bereich %d weicht nach dem Neurechnen vom ehrlichen Lauf ab", b)
		}
	}
	if n.ausgezahlt != ehrlich.ausgezahlt {
		t.Fatalf("ausgezahlt %d statt %d", n.ausgezahlt, ehrlich.ausgezahlt)
	}
	n.hauptbuchPruefen()
}
