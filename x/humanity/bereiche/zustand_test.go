package bereiche

import (
	"math/rand"
	"testing"
)

func TestAusfuehren_KappungGehtAnsGrundeinkommen(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	a, b := neuePerson(r), neuePerson(r)
	s := 1
	z := NeuerVollZustand()
	z.SetzeKonto(a.addr, Konto{Guthaben: 5_000 * Mikro, Mensch: true})
	z.SetzeKonto(b.addr, Konto{Guthaben: 24_900 * Mikro, Mensch: true})
	u := Ueberweisung{Von: a.addr, An: b.addr, Betrag: 300 * Mikro}
	u.Signiere(a.priv)
	erg, err := Ausfuehren(z, 0, Rahmen{S: s, Grenze: 25_000 * Mikro}, Eingaben{Ueberweisungen: []Ueberweisung{u}}, Hash{1})
	if err != nil || !erg.Angewandt[0] {
		t.Fatalf("nicht angewandt: %v", err)
	}
	kb, _ := z.Konto(b.addr)
	if kb.Guthaben != 25_000*Mikro {
		t.Fatalf("B hat %d statt der Grenze", kb.Guthaben)
	}
	// Gebuehr 0,3 AEQ + Kappung 200 AEQ ans Grundeinkommen.
	if erg.Teilsumme.Gebuehren != 300*Mikro/1000+200*Mikro {
		t.Fatalf("Gebuehren %d", erg.Teilsumme.Gebuehren)
	}
}

func TestBilligPruefen_ErkenntGelogeneKoepfe(t *testing.T) {
	n := neuesSimNetz(t, 8)
	n.lauf(2, 800)
	b := 0
	blk := *n.letzter[b]
	aus, _ := Ausschuesse(n.waehlbar(), n.s, n.m, n.sammel[len(n.sammel)-2].Zufall())
	r := n.sammel[len(n.sammel)-2].Rahmen(n.s)
	// Geldmenge hochgelogen und neu unterschrieben: die Kette der Geldmengen
	// passt nicht mehr.
	gelogen := blk
	gelogen.Geldmenge += 1_000 * Mikro
	gelogen.Sig = nil
	for _, a := range aus[b] {
		gelogen.Unterschreibe(a, n.schluessel(a))
	}
	vor := BereichsBlock{Nr: blk.Nr - 1, Nach: blk.Vor,
		Geldmenge: blk.Geldmenge - (blk.Teilsumme.Eingang + blk.Teilsumme.Geschoepft + blk.Teilsumme.GEAusgezahlt - blk.Teilsumme.Ausgang - blk.Teilsumme.Gebuehren - blk.Teilsumme.Ausstiege),
		Menschen:  blk.Menschen - blk.Teilsumme.NeueMenschen}
	if err := BilligPruefen(blk, &vor, aus[b], r); err != nil {
		t.Fatalf("echter Block abgelehnt: %v", err)
	}
	if err := BilligPruefen(gelogen, &vor, aus[b], r); err == nil {
		t.Fatal("hochgelogene Geldmenge angenommen")
	}
	// Zu wenige Unterschriften.
	wenig := blk
	wenig.Sig = map[string][]byte{}
	for _, a := range aus[b][:Mehrheit(n.m)-1] {
		wenig.Unterschreibe(a, n.schluessel(a))
	}
	if err := BilligPruefen(wenig, &vor, aus[b], r); err == nil {
		t.Fatal("Block mit weniger als 2/3 angenommen")
	}
	// Kette gebrochen.
	anders := vor
	anders.Nach = Hash{9}
	if err := BilligPruefen(blk, &anders, aus[b], r); err == nil {
		t.Fatal("gebrochene Kette angenommen")
	}
}
