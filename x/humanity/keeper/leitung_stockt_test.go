package keeper

import (
	"testing"
	"time"
)

// OFFEN 6: Ein Leiter, der keine eigenen Bloecke mehr erzeugt (stockt),
// haelt die Annahme des ganzen Netzes an -- er gibt ab, sobald er laenger
// als StockenFrist stockt. Jede Uebergabe ist die geordnete; der Simulator
// prueft in jedem Schritt, dass nie zwei annehmen.

func stockKonfig() LeitKonfig {
	c := testKonfig()
	c.StockenFrist = 20 * time.Second
	return c
}

func TestLeitung3_StockenderLeiterGibtNachDerFristAb(t *testing.T) {
	n := neuesSimNetz(t, 3, 21, stockKonfig())
	n.laufe(5 * time.Second)
	if n.annehmender() != 0 {
		t.Fatal("Startleiter nimmt nicht an")
	}
	n.knoten[0].l.SetzeStockt(true)
	// Seit dem Antritt sind erst 5 s vergangen: noch keine Abgabe.
	n.laufe(10 * time.Second)
	if i := n.annehmender(); i != 0 && n.knoten[0].l.Stand(n.uhr(0))["uebergabe_laeuft"] == true {
		t.Fatalf("vor der Frist abgegeben (%d)", i)
	}
	n.laufe(30 * time.Second)
	if i := n.annehmender(); i != 1 && i != 2 {
		t.Fatalf("nimmt %d an -- der stockende Leiter hat nicht abgegeben", i)
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Ein neuer Leiter bekommt seine Frist: stocken alle, wandert die Leitung
// hoechstens einmal je Frist weiter (Pruefung des Leiter-PR, Befund 6) --
// und sobald sie wieder erzeugen, nimmt genau einer an.
func TestLeitung3_AlleStockenKeinFlattern(t *testing.T) {
	n := neuesSimNetz(t, 3, 22, stockKonfig())
	n.laufe(5 * time.Second)
	for _, k := range n.knoten {
		k.l.SetzeStockt(true)
	}
	vorher := n.wechsel
	n.laufe(60 * time.Second)
	if w := n.wechsel - vorher; w > 4 {
		t.Fatalf("%d Wechsel in 60 s bei einer Frist von 20 s", w)
	}
	for _, k := range n.knoten {
		k.l.SetzeStockt(false)
	}
	n.laufe(60 * time.Second)
	if n.annehmender() < 0 {
		t.Fatal("nach dem Ende des Stockens kein Leiter")
	}
}

// Befund 2: der Leiter stockt, sein Ausgang ist nicht leer (genau die
// Zeilen werden nicht verblockt), er erholt sich und sendet die Uebergabe
// erst dann -- an einen ohne Nachweis. Der nimmt sie an (und gibt an den
// leiterfaehigen zurueck); niemand haengt.
func TestLeitung3_UebergabeNachErholungHaengtNicht(t *testing.T) {
	n := neuesSimNetz(t, 3, 23, stockKonfig())
	n.faehig = map[string]bool{n.knoten[0].addr: true}
	leer := false
	n.knoten[0].entleert = func() bool { return leer }
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(5 * time.Second)
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(30 * time.Second) // abschliessen gesetzt, wartet auf Entleert
	n.knoten[0].l.SetzeStockt(false)
	leer = true
	// Schnell -- die geordnete Uebergabe, keine Wahl nach Ablauf der
	// FolgerFrist (die heilte zu dritt auch einen haengenden Abgebenden).
	n.laufe(6 * time.Second)
	if n.annehmender() < 0 {
		t.Fatal("kurz nach der Uebergabe kein Leiter -- der Empfaenger hat abgelehnt")
	}
	n.laufe(60 * time.Second)
	if n.annehmender() < 0 {
		t.Fatal("nach der Uebergabe kein Leiter")
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Keiner der anderen haelt den Nachweis: der stockende Leiter gibt trotzdem
// ab -- ein lebender Leiter ohne Nachweis ist besser als einer, der keine
// Bloecke erzeugt.
func TestLeitung3_StockenderGibtAuchAnEinenOhneNachweis(t *testing.T) {
	n := neuesSimNetz(t, 3, 27, stockKonfig())
	n.faehig = map[string]bool{n.knoten[0].addr: true}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(5 * time.Second)
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(40 * time.Second)
	if i := n.annehmender(); i != 1 && i != 2 {
		t.Fatalf("nimmt %d an, erwartet 1 oder 2", i)
	}
}

// Bei zwei Validatoren ohne ZweiWechseln gibt ein stockender Leiter nicht
// ab: ohne Wahl heilt nichts eine verlorene Uebergabe (Befunde 1, 3, 4).
func TestLeitung2_OhneZweiWechselnKeineAbgabe(t *testing.T) {
	n := neuesSimNetz(t, 2, 24, stockKonfig())
	n.laufe(5 * time.Second)
	n.knoten[0].l.SetzeStockt(true)
	n.knoten[0].l.SetzeFaehig(false)
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 0 {
		t.Fatalf("nimmt %d an, erwartet 0", i)
	}
}

// Befund 1: zwei Validatoren mit ZweiWechseln, beide stocken. Die Frist
// verhindert das Zurueckgeben im selben Takt; erholen sich beide, nimmt
// wieder genau einer an.
func TestLeitung2_BeideStockenDannErholung(t *testing.T) {
	for seed := int64(30); seed < 50; seed++ {
		c := stockKonfig()
		c.ZweiWechseln = true
		n := neuesSimNetz(t, 2, seed, c)
		n.laufe(5 * time.Second)
		n.knoten[0].l.SetzeStockt(true)
		n.knoten[1].l.SetzeStockt(true)
		n.laufe(90 * time.Second)
		n.knoten[0].l.SetzeStockt(false)
		n.knoten[1].l.SetzeStockt(false)
		n.laufe(60 * time.Second)
		if n.annehmender() < 0 {
			t.Fatalf("seed %d: nach der Erholung kein Leiter", seed)
		}
	}
}

// Ein stockender Folger wird bei einem Ausfall nicht gewaehlt.
func TestLeitung3_StockenderWirdNichtLeiter(t *testing.T) {
	n := neuesSimNetz(t, 3, 25, stockKonfig())
	n.laufe(5 * time.Second)
	n.knoten[1].l.SetzeStockt(true)
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 2 {
		t.Fatalf("nimmt %d an, erwartet 2 (einziger, der nicht stockt)", i)
	}
	n.knoten[0].l.SetzeStockt(false)
	n.laufe(5 * time.Second)
	n.knoten[2].an = false
	n.laufe(60 * time.Second)
	if i := n.annehmender(); i != 0 {
		t.Fatalf("nach Ausfall von 2 nimmt %d an, erwartet 0 (1 stockt)", i)
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Zufall: Stocken und Nachweis wechseln, dazu Ausfaelle, Trennungen,
// Verlust. Nie zwei -- und nach der Heilung (alle da, keiner stockt) gibt
// es wieder einen Leiter (Lebendigkeit, Pruefung des Leiter-PR).
func TestLeitung_ZufallMitStocken(t *testing.T) {
	for _, anzahl := range []int{3, 4} {
		for seed := int64(200); seed < 220; seed++ {
			c := stockKonfig()
			c.WechselAlle = 45 * time.Second
			n := neuesSimNetz(t, anzahl, seed, c)
			n.verlust = 0.1
			for runde := 0; runde < 30; runde++ {
				i := n.r.Intn(anzahl)
				switch n.r.Intn(5) {
				case 0:
					n.knoten[i].l.SetzeStockt(true)
				case 1:
					n.knoten[i].l.SetzeStockt(false)
				case 2:
					n.knoten[i].an = false
				case 3:
					if !n.knoten[i].an {
						n.knoten[i].an = true
						n.baue(i)
					}
				case 4:
					n.gruppe = map[int]int{}
					for j := range n.knoten {
						n.gruppe[j] = n.r.Intn(2)
					}
				}
				n.laufe(time.Duration(5+n.r.Intn(20)) * time.Second)
			}
			n.gruppe = map[int]int{}
			for i, k := range n.knoten {
				if !k.an {
					k.an = true
					n.baue(i)
				}
				k.l.SetzeStockt(false)
			}
			n.laufe(3 * time.Minute)
			if n.annehmender() < 0 {
				t.Fatalf("%d Knoten, seed %d: nach der Heilung kein Leiter", anzahl, seed)
			}
			if n.zweiLeiter {
				t.Fatalf("%d Knoten, seed %d: zwei Leiter", anzahl, seed)
			}
		}
	}
}

// Die Messung: erst ab 2*admissionStallLimit() ohne eigenen Block, nie,
// solange der Knoten nicht erzeugt; sofort, wenn das Register ihn nicht
// traegt.
func TestEigeneBloeckeStocken(t *testing.T) {
	cs := &ChainState{}
	jetzt := int64(1_800_000_000)
	grenze := 2 * admissionStallLimit()
	if cs.eigeneBloeckeStocken(jetzt) {
		t.Fatal("erzeugt nicht -- stockt nicht")
	}
	cs.erzeugerSeit.Store(jetzt - grenze + 1)
	if cs.eigeneBloeckeStocken(jetzt) {
		t.Fatal("vor der Grenze gestockt")
	}
	cs.erzeugerSeit.Store(jetzt - grenze)
	if !cs.eigeneBloeckeStocken(jetzt) {
		t.Fatal("an der Grenze nicht gestockt")
	}
	cs.letzterEigenerBlock.Store(jetzt - 5)
	if cs.eigeneBloeckeStocken(jetzt) {
		t.Fatal("eigener Block vor 5 s -- stockt nicht")
	}
	cs.nichtImRegister.Store(true)
	if !cs.eigeneBloeckeStocken(jetzt) {
		t.Fatal("nicht im Register -- stockt")
	}
	var nichts *ChainState
	if nichts.eigeneBloeckeStocken(jetzt) {
		t.Fatal("nil")
	}
}

// Stocken und Nachweis sind getrennt: wer stockt, kuendigt sich als nicht
// leiterfaehig an, und sobald er wieder erzeugt, zaehlt sein Nachweis.
func TestLeitung_StocktUndNachweisGetrennt(t *testing.T) {
	n := neuesSimNetz(t, 1, 26, testKonfig())
	l := n.knoten[0].l
	l.SetzeStockt(true)
	if l.Stand(n.uhr(0))["leiterfaehig"] != false || l.Stand(n.uhr(0))["stockt"] != true {
		t.Fatal("stockt, aber leiterfaehig")
	}
	l.SetzeFaehig(true) // neue Messung des Nachweises hebt das Stocken nicht auf
	if l.Stand(n.uhr(0))["leiterfaehig"] != false {
		t.Fatal("Nachweis hob das Stocken auf")
	}
	l.SetzeStockt(false)
	if l.Stand(n.uhr(0))["leiterfaehig"] != true {
		t.Fatal("nach dem Stocken nicht wieder leiterfaehig")
	}
	l.SetzeFaehig(false)
	if l.Stand(n.uhr(0))["leiterfaehig"] != false {
		t.Fatal("ohne Nachweis leiterfaehig")
	}
}
