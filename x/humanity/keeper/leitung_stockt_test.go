package keeper

import (
	"testing"
	"time"
)

// OFFEN 6: Ein Leiter, der keine eigenen Bloecke mehr erzeugt (stockt),
// haelt die Annahme des ganzen Netzes an -- er gibt ab, auch bei zwei
// Validatoren ohne ZweiWechseln. Jede Uebergabe ist die geordnete; der
// Simulator prueft in jedem Schritt, dass nie zwei annehmen.

func TestLeitung2_StockenderLeiterGibtAb(t *testing.T) {
	n := neuesSimNetz(t, 2, 21, testKonfig())
	n.laufe(5 * time.Second)
	if n.annehmender() != 0 {
		t.Fatal("Startleiter nimmt nicht an")
	}
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(20 * time.Second)
	if i := n.annehmender(); i != 1 {
		t.Fatalf("nimmt %d an, erwartet 1 -- der stockende Leiter hat nicht abgegeben", i)
	}
	// Er erzeugt wieder: bei zwei kein Wechsel zurueck ohne Grund.
	n.knoten[0].l.SetzeStockt(false)
	n.laufe(20 * time.Second)
	if i := n.annehmender(); i != 1 {
		t.Fatalf("nimmt %d an, erwartet weiter 1", i)
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Bei zwei aendert ein verlorener Leistungsnachweis allein nichts (wie
// bisher): nur das Stocken gibt ab.
func TestLeitung2_OhneNachweisAlleinKeinWechsel(t *testing.T) {
	n := neuesSimNetz(t, 2, 22, testKonfig())
	n.laufe(5 * time.Second)
	n.knoten[0].l.SetzeFaehig(false)
	n.laufe(20 * time.Second)
	if i := n.annehmender(); i != 0 {
		t.Fatalf("nimmt %d an, erwartet 0 -- bei zwei kein Wechsel nur wegen des Nachweises", i)
	}
}

// Der andere haelt keinen Nachweis: der stockende Leiter gibt trotzdem an
// ihn ab -- ein lebender Leiter ohne Nachweis ist besser als einer, der
// keine Bloecke erzeugt.
func TestLeitung2_StockenderLeiterGibtAuchAnEinenOhneNachweis(t *testing.T) {
	n := neuesSimNetz(t, 2, 23, testKonfig())
	n.faehig = map[string]bool{n.knoten[0].addr: true}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufe(5 * time.Second)
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(20 * time.Second)
	if i := n.annehmender(); i != 1 {
		t.Fatalf("nimmt %d an, erwartet 1", i)
	}
}

// Ist der andere nicht da, gibt es niemanden, an den abzugeben: der Leiter
// bleibt (und uebergibt nicht an einen Toten).
func TestLeitung2_StockenderLeiterOhneLebendenBleibt(t *testing.T) {
	n := neuesSimNetz(t, 2, 24, testKonfig())
	n.laufe(5 * time.Second)
	n.knoten[1].an = false
	n.laufe(10 * time.Second) // laenger als LeaseDauer: 1 gilt nicht mehr als lebend
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(30 * time.Second)
	if i := n.annehmender(); i != 0 {
		t.Fatalf("nimmt %d an, erwartet 0", i)
	}
	if st := n.knoten[0].l.Stand(n.uhr(0)); st["rolle"] != leitLeiter.String() || st["stockt"] != true {
		t.Fatalf("Stand %v", st)
	}
}

// Stirbt der andere genau, als der Leiter abgibt (er galt noch als lebend),
// nimmt bis zu seiner Rueckkehr keiner an -- wie beim planmaessigen Wechsel.
// Kommt er zurueck, erreicht ihn die wiederholte Uebergabe und er leitet.
func TestLeitung2_UebergabeAnGeradeGestorbenenHeiltBeiRueckkehr(t *testing.T) {
	n := neuesSimNetz(t, 2, 27, testKonfig())
	n.laufe(5 * time.Second)
	n.knoten[1].an = false
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(30 * time.Second)
	n.knoten[1].an = true
	n.baue(1)
	n.laufe(30 * time.Second)
	if i := n.annehmender(); i != 1 {
		t.Fatalf("nimmt %d an, erwartet den zurueckgekehrten 1", i)
	}
	if n.zweiLeiter {
		t.Fatal("zwei Leiter")
	}
}

// Drei Validatoren: der stockende Leiter gibt an einen leiterfaehigen ab;
// ein stockender Folger wird bei einem Ausfall nicht gewaehlt.
func TestLeitung3_StockenderWirdNichtLeiter(t *testing.T) {
	n := neuesSimNetz(t, 3, 25, testKonfig())
	n.laufe(5 * time.Second)
	n.knoten[1].l.SetzeStockt(true)
	n.knoten[0].l.SetzeStockt(true)
	n.laufe(30 * time.Second)
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
	if l.Stand(n.uhr(0))["leiterfaehig"] != false {
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
