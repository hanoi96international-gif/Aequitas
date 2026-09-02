package mpc

import "fmt"

// Anteile an ein neues Komitee weiterreichen, ohne die Vorlage je
// zusammenzusetzen.
//
// # DAS PROBLEM, DAS DAS HIER LÖST
//
// Eine Einschreibung gehört dem Komitee, das sie erzeugt hat. committee.go
// sagt die Folge unverblümt: ein Komitee, dessen Mitglieder dauerhaft fort
// sind, nimmt seine Einschreibungen mit — "those people can register again
// undetected".
//
// Bei einem offenen Validatorensatz ist das kein Randfall. Betreiber kommen
// und gehen; jedes erloschene Komitee ist ein Loch in der Einmaligkeit, und
// die Löcher summieren sich über Jahre. Ohne einen Ausweg steht man vor der
// Wahl zwischen offener Teilnahme und verlässlicher Einmaligkeit.
//
// # WARUM ES TROTZDEM GEHT
//
// Der Kommentar in committee.go schließt zu viel aus. Er sagt, die Anteile
// zu bewegen hieße, die Vorlage zu rekonstruieren. Für naives Verschieben
// stimmt das. Für additive Anteile gibt es aber einen Weg, bei dem die
// Vorlage nirgends entsteht:
//
//	Alt:  s = r₁ + r₂ + … + rₙ        Partei i hält rᵢ
//
//	1. Jede alte Partei i zerlegt IHREN EIGENEN Anteil weiter:
//	      rᵢ = rᵢ₁ + rᵢ₂ + … + rᵢₘ
//	   und schickt rᵢⱼ an die neue Partei j.
//
//	2. Jede neue Partei j summiert, was sie bekommen hat:
//	      r'ⱼ = Σᵢ rᵢⱼ
//
//	Dann gilt  Σⱼ r'ⱼ = Σⱼ Σᵢ rᵢⱼ = Σᵢ rᵢ = s.
//
// Niemand hat dabei mehr gesehen als vorher: die alte Partei kennt nur ihren
// eigenen Anteil, die neue nur eine Summe fremder Teilstücke. Die Vorlage
// existiert in keinem Schritt.
//
// # DIE GRENZE, DIE BLEIBT
//
// Das Neuteilen braucht das ALTE Komitee lebendig und mitwirkend. Es ist
// Vorsorge, keine Rettung: sind die Mitglieder schon fort, hilft es nicht
// mehr. Deshalb muss es laufen, BEVOR jemand geht — bei jedem
// Komiteewechsel, nicht erst, wenn etwas fehlt.
//
// # WAS DIE GRÖSSE ANGEHT
//
// m darf sich von n unterscheiden: das Komitee darf wachsen und schrumpfen.
// Nur unter zwei darf es nicht fallen, denn dann hält eine Partei alles.

// SplitRowForReshare zerlegt den Anteil EINER alten Partei in m Teilstücke,
// je eines für jede neue Partei.
//
// Läuft lokal bei der alten Partei. Sie gibt ihren Anteil nicht heraus,
// sondern nur Teilstücke davon, und jedes einzelne ist für sich zufällig.
func SplitRowForReshare(row PartyTemplate, m int) ([]PartyTemplate, error) {
	if len(row) == 0 {
		return nil, fmt.Errorf("mpc: leerer Anteil — es gibt nichts weiterzureichen")
	}
	if m < 2 {
		return nil, fmt.Errorf("mpc: Neuteilung auf %d Parteien abgelehnt — "+
			"eine einzelne Partei hielte damit die ganze Vorlage; mindestens 2", m)
	}
	teile, err := SplitVector([]Element(row), m)
	if err != nil {
		return nil, err
	}
	out := make([]PartyTemplate, m)
	for i := range teile {
		out[i] = PartyTemplate(teile[i])
	}
	return out, nil
}

// CombineReshares summiert die Teilstücke, die EINE neue Partei von allen
// alten Parteien erhalten hat, zu ihrem neuen Anteil.
//
// erwarteteBeitraege ist die Zahl der alten Parteien. Sie wird verlangt und
// geprüft, statt sie aus der Länge zu schließen — und das ist der wichtigste
// Teil dieser Funktion.
//
// Fehlt ein Beitrag, ergäbe die Summe einen Anteil, der zu nichts mehr passt.
// Die Einschreibung wäre danach unbrauchbar, und zwar STILL: der Vergleich
// liefe weiter, nur verglichen mit Rauschen, und jede Antwort lautete "kein
// Duplikat". Ein unvollständiges Neuteilen sieht also genau wie ein
// erfolgreiches aus — bis Menschen sich zweimal registrieren.
//
// Deshalb ist Unvollständigkeit hier ein Fehler und keine Näherung.
func CombineReshares(teile []PartyTemplate, erwarteteBeitraege int) (PartyTemplate, error) {
	if erwarteteBeitraege < 2 {
		return nil, fmt.Errorf("mpc: %d beitragende Parteien sind zu wenig — "+
			"unter zwei war die Vorlage nie geteilt", erwarteteBeitraege)
	}
	if len(teile) != erwarteteBeitraege {
		return nil, fmt.Errorf("mpc: %d Beiträge erhalten, %d erwartet — die Neuteilung wird "+
			"abgebrochen. Ein fehlender Beitrag ergäbe einen Anteil, der zu nichts passt, und "+
			"die Einschreibung wäre danach unbrauchbar, ohne dass es auffiele: jeder Vergleich "+
			"gegen sie lieferte 'kein Duplikat'",
			len(teile), erwarteteBeitraege)
	}
	laenge := len(teile[0])
	if laenge == 0 {
		return nil, fmt.Errorf("mpc: leeres Teilstück erhalten")
	}
	neu := make(PartyTemplate, laenge)
	for i, teil := range teile {
		if len(teil) != laenge {
			return nil, fmt.Errorf("mpc: Teilstück %d hat %d Merkmale, das erste %d — "+
				"sie stammen nicht von derselben Einschreibung", i, len(teil), laenge)
		}
		for j := range teil {
			neu[j] = Add(neu[j], teil[j])
		}
	}
	return neu, nil
}

// ReshareAll führt die Neuteilung für ein ganzes Komitee durch.
//
// NUR FÜR TESTS UND FÜR EINEN EINZELNEN VERTRAUENSWÜRDIGEN AUFRUFER, der
// ohnehin alle Anteile hält. Im Betrieb läuft Schritt 1 bei jeder alten
// Partei getrennt und Schritt 2 bei jeder neuen — genau darum geht es. Diese
// Funktion existiert, damit die Rechnung als Ganzes prüfbar ist.
func ReshareAll(alt []PartyTemplate, m int) ([]PartyTemplate, error) {
	if len(alt) < 2 {
		return nil, fmt.Errorf("mpc: %d alte Parteien — unter zwei war nichts verborgen", len(alt))
	}
	// beitraege[j][i] = Teilstück von alter Partei i an neue Partei j
	beitraege := make([][]PartyTemplate, m)
	for j := range beitraege {
		beitraege[j] = make([]PartyTemplate, len(alt))
	}
	for i, row := range alt {
		teile, err := SplitRowForReshare(row, m)
		if err != nil {
			return nil, fmt.Errorf("alte Partei %d: %w", i, err)
		}
		for j := range teile {
			beitraege[j][i] = teile[j]
		}
	}
	neu := make([]PartyTemplate, m)
	for j := range neu {
		r, err := CombineReshares(beitraege[j], len(alt))
		if err != nil {
			return nil, fmt.Errorf("neue Partei %d: %w", j, err)
		}
		neu[j] = r
	}
	return neu, nil
}
