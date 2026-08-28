package keeper

import (
	"fmt"
	"sync/atomic"
)

// Warum der Zweitindex hinter der Zahl der Menschen zurueckbleibt -- sichtbar
// statt raetselhaft.
//
// DIE GESCHICHTE
//
// chain_bio_hashes stand am 12.07.2026 auf 0 bei 6 Menschen, am 15.08. auf 0
// bei 15, am 28.08. auf 3 bei 18. Dreimal wurde untersucht, woran das liegt,
// und zweimal blieb es offen. Der Grund ist nicht ein Fehler, sondern eine
// Summe von Wegen, auf denen nichts passiert -- und keiner davon hinterliess
// eine Spur:
//
//  1. Kommt eine Registrierung ohne bio_hash_key an, wird der Eintrag
//     uebersprungen. Ohne Log, ohne Zaehler. Der haeufigste Fall.
//  2. Schlaegt der Schreibvorgang fehl, gibt es eine Warnzeile und sonst
//     nichts -- waehrend die Zustellung an die Proof-Server fuer denselben
//     Wert eine Wiederholungsschlange hat. Genau diese Asymmetrie hat die
//     beiden Zwischenspeicher auseinanderlaufen lassen: 3 auf der Kette, 7 auf
//     den Proof-Servern, ohne eine einzige Ueberschneidung.
//  3. Eine Registrierung, die als BLOCK ankommt statt ueber die Schnittstelle,
//     traegt den rohen Hash gar nicht bei sich -- absichtlich, siehe
//     block.go's register_human. Ein Knoten, der eine Registrierung nur
//     nachspielt, kann den Eintrag also nicht haben. Das ist keine Panne,
//     sondern der Preis dafuer, dass biometrisch abgeleitete Kennungen nicht
//     in der Blockhistorie jedes Knotens landen.
//
// WAS DAS BEDEUTET, UND WAS NICHT
//
// Der Zweitindex ist ein Zwischenspeicher, keine Zusicherung. Die eigentliche
// Abwehr sind die Nullifier: sie stehen vollstaendig auf der Kette, werden
// mitgespielt und zusammen mit der Registrierung in einem Zug beansprucht. Ein
// zweiter Versuch scheitert also auch mit leerem Index -- nur spaeter und mit
// einer weniger hilfreichen Meldung. Das kostet Bedienkomfort, nicht
// Sicherheit.
//
// Deshalb wird hier nichts erzwungen, sondern gezaehlt. Wer die Luecke das
// naechste Mal sieht, soll sie erklaert bekommen statt sie zu untersuchen.

var (
	bioIndexOhneSchluessel atomic.Int64 // Registrierung kam ohne bio_hash_key
	bioIndexSchreibfehler  atomic.Int64 // Schreibvorgang schlug fehl
	bioIndexGeschrieben    atomic.Int64 // erfolgreich eingetragen
)

// bioIndexKeinSchluessel haelt fest, dass eine Registrierung nichts zum
// Eintragen mitbrachte.
//
// Frueher war das der stille Zweig eines if. Ein Weg, auf dem nichts passiert
// und nichts protokolliert wird, ist nicht zu unterscheiden von einem, der
// funktioniert -- und das war er drei Untersuchungen lang auch nicht.
func bioIndexKeinSchluessel(wallet string) {
	bioIndexOhneSchluessel.Add(1)
	fmt.Printf("[BIO-INDEX] %s ohne bio_hash_key registriert -- der Zweitindex bleibt fuer "+
		"diesen Menschen leer. Der Nullifier deckt ihn trotzdem ab; nur die Duplikatsmeldung "+
		"kommt spaeter und weniger deutlich.\n", wallet)
}

func bioIndexFehler(wallet string, err error) {
	bioIndexSchreibfehler.Add(1)
	fmt.Printf("[BIO-INDEX] Eintrag fuer %s fehlgeschlagen: %v -- anders als die Zustellung an "+
		"die Proof-Server wird das NICHT wiederholt. Genau diese Asymmetrie hat die beiden "+
		"Zwischenspeicher auseinanderlaufen lassen.\n", wallet, err)
}

func bioIndexErfolg() {
	bioIndexGeschrieben.Add(1)
}

// BioIndexZustand erklaert den Abstand zwischen Menschen und Zweitindex.
func (cs *ChainState) BioIndexZustand() map[string]interface{} {
	eintraege := cs.CountChainBioHashes()
	menschen := cs.CountChainNullifiers()

	z := map[string]interface{}{
		"eintraege":                eintraege,
		"menschen":                 menschen,
		"ohne_schluessel_erhalten": bioIndexOhneSchluessel.Load(),
		"schreibfehler":            bioIndexSchreibfehler.Load(),
		"geschrieben":              bioIndexGeschrieben.Load(),
	}
	if eintraege < menschen {
		z["erklaerung"] = "Der Zweitindex ist ein Zwischenspeicher, keine Zusicherung. " +
			"Er bleibt leer fuer Registrierungen, die als Block ankommen statt ueber die " +
			"Schnittstelle -- der rohe Hash reist dort absichtlich nicht mit. Die Abwehr " +
			"sind die Nullifier, und die sind vollstaendig. Ein Abstand hier kostet eine " +
			"weniger deutliche Duplikatsmeldung, keine Sicherheit."
	}
	return z
}
