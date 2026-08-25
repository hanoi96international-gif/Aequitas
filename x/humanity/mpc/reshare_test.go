package mpc

import (
	"testing"
)

// Jeder Test hier prüft eine Eigenschaft, ohne die das Neuteilen schlimmer
// wäre als gar keines: eine kaputte Neuteilung zerstört Einschreibungen
// STILL, und still zerstörte Einschreibungen sehen aus wie Menschen, die es
// noch nie gab.

func summe(rows []PartyTemplate) []Element {
	if len(rows) == 0 {
		return nil
	}
	acc := make([]Element, len(rows[0]))
	for _, r := range rows {
		for j := range r {
			acc[j] = Add(acc[j], r[j])
		}
	}
	return acc
}

func gleich(a, b []Element) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func testVorlage(t *testing.T, bits int, parteien int) ([]PartyTemplate, []Element) {
	t.Helper()
	sketch := make([]uint8, bits)
	for i := range sketch {
		sketch[i] = uint8((i*7 + 3) % 2)
	}
	rows, err := SplitTemplateForParties(sketch, parteien)
	if err != nil {
		t.Fatal(err)
	}
	return rows, summe(rows)
}

// DIE Eigenschaft: nach der Neuteilung ergibt die Summe dasselbe Geheimnis.
func TestNeuteilungErhaeltDasGeheimnis(t *testing.T) {
	for _, f := range []struct{ alt, neu int }{
		{2, 2}, {2, 3}, {3, 2}, {2, 5}, {5, 3}, {4, 4},
	} {
		alt, geheim := testVorlage(t, 64, f.alt)
		neu, err := ReshareAll(alt, f.neu)
		if err != nil {
			t.Fatalf("%d -> %d: %v", f.alt, f.neu, err)
		}
		if len(neu) != f.neu {
			t.Fatalf("%d -> %d: %d neue Anteile", f.alt, f.neu, len(neu))
		}
		if !gleich(summe(neu), geheim) {
			t.Errorf("%d -> %d: das Geheimnis hat sich veraendert", f.alt, f.neu)
		}
	}
}

// Das Komitee darf wachsen UND schrumpfen -- sonst waere ein Validator, der
// geht, ein dauerhafter Verlust an Groesse.
func TestKomiteeDarfWachsenUndSchrumpfen(t *testing.T) {
	alt, geheim := testVorlage(t, 32, 3)
	gewachsen, err := ReshareAll(alt, 7)
	if err != nil {
		t.Fatal(err)
	}
	zurueck, err := ReshareAll(gewachsen, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !gleich(summe(zurueck), geheim) {
		t.Error("nach Wachsen und Schrumpfen stimmt das Geheimnis nicht mehr")
	}
}

// Kein neuer Anteil darf einem alten gleichen. Waere er es, haette die neue
// Partei genau das erfahren, was die alte wusste.
func TestNeueAnteileSindNichtDieAlten(t *testing.T) {
	alt, _ := testVorlage(t, 128, 2)
	neu, err := ReshareAll(alt, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := range neu {
		for j := range alt {
			if gleich(neu[i], alt[j]) {
				t.Errorf("neuer Anteil %d ist mit altem Anteil %d identisch", i, j)
			}
		}
	}
}

// Ein einzelner neuer Anteil darf das Geheimnis nicht sein. Das ist die
// Zusicherung, fuer die es diese ganze Bauart gibt.
func TestEinzelnerAnteilIstNichtDasGeheimnis(t *testing.T) {
	alt, geheim := testVorlage(t, 64, 2)
	neu, err := ReshareAll(alt, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range neu {
		if gleich([]Element(r), geheim) {
			t.Errorf("Anteil %d IST das Geheimnis", i)
		}
	}
}

// Zwei Neuteilungen desselben Anteils muessen verschieden ausfallen. Waeren
// sie gleich, waere die Zufaelligkeit nicht frisch, und wer zwei Runden
// beobachtet, koennte sie gegeneinander rechnen.
func TestJedeNeuteilungIstFrisch(t *testing.T) {
	alt, _ := testVorlage(t, 64, 2)
	a, err := ReshareAll(alt, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReshareAll(alt, 2)
	if err != nil {
		t.Fatal(err)
	}
	if gleich(a[0], b[0]) {
		t.Error("zwei Neuteilungen ergaben denselben Anteil -- die Zufaelligkeit ist nicht frisch")
	}
}

// DER WICHTIGSTE TEST: ein fehlender Beitrag muss ABBRECHEN.
//
// Wuerde er stillschweigend hingenommen, ergaebe die Summe einen Anteil, der
// zu nichts mehr passt. Jeder Vergleich gegen diese Einschreibung lieferte
// danach "kein Duplikat" -- der Mensch waere geloescht, ohne dass es jemand
// bemerkt.
func TestFehlenderBeitragBrichtAb(t *testing.T) {
	alt, _ := testVorlage(t, 32, 3)
	teile := make([]PartyTemplate, 0, 3)
	for _, row := range alt {
		st, err := SplitRowForReshare(row, 2)
		if err != nil {
			t.Fatal(err)
		}
		teile = append(teile, st[0])
	}
	if _, err := CombineReshares(teile[:2], 3); err == nil {
		t.Fatal("zwei von drei Beitraegen wurden angenommen -- die Einschreibung waere " +
			"unbrauchbar geworden, ohne dass es auffiele")
	}
	if _, err := CombineReshares(teile, 3); err != nil {
		t.Fatalf("drei von drei wurden abgelehnt: %v", err)
	}
}

// Teilstuecke verschiedener Laenge stammen nicht von derselben Einschreibung.
func TestUneinheitlicheLaengeBrichtAb(t *testing.T) {
	kurz := PartyTemplate{1, 2, 3}
	lang := PartyTemplate{1, 2, 3, 4}
	if _, err := CombineReshares([]PartyTemplate{kurz, lang}, 2); err == nil {
		t.Error("Teilstuecke unterschiedlicher Laenge wurden zusammengefasst")
	}
}

// Unter zwei Parteien ist nichts verborgen -- in beide Richtungen.
func TestUnterZweiParteienWirdAbgelehnt(t *testing.T) {
	row := PartyTemplate{1, 2, 3}
	if _, err := SplitRowForReshare(row, 1); err == nil {
		t.Error("Neuteilung auf eine Partei wurde angenommen")
	}
	if _, err := SplitRowForReshare(row, 0); err == nil {
		t.Error("Neuteilung auf null Parteien wurde angenommen")
	}
	if _, err := CombineReshares([]PartyTemplate{row}, 1); err == nil {
		t.Error("ein einzelner Beitrag wurde angenommen")
	}
	if _, err := ReshareAll([]PartyTemplate{row}, 2); err == nil {
		t.Error("Neuteilung AUS einer einzelnen Partei wurde angenommen")
	}
}

func TestLeererAnteilWirdAbgelehnt(t *testing.T) {
	if _, err := SplitRowForReshare(PartyTemplate{}, 2); err == nil {
		t.Error("ein leerer Anteil wurde geteilt")
	}
}

// Die Kette aus vielen Wechseln darf sich nicht aufsummieren -- ein
// Rundungs- oder Modulofehler waere nach zwanzig Wechseln sichtbar.
func TestVieleWechselHintereinander(t *testing.T) {
	rows, geheim := testVorlage(t, 48, 2)
	groessen := []int{3, 2, 5, 4, 2, 6, 3, 2}
	for runde := 0; runde < 20; runde++ {
		var err error
		rows, err = ReshareAll(rows, groessen[runde%len(groessen)])
		if err != nil {
			t.Fatalf("Runde %d: %v", runde, err)
		}
	}
	if !gleich(summe(rows), geheim) {
		t.Error("nach 20 Komiteewechseln stimmt das Geheimnis nicht mehr")
	}
}
