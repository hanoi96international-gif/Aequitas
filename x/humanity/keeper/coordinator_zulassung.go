package keeper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// COORDINATOREN IM KONSENS ZULASSEN UND ENTZIEHEN (Bedingung vor der Staffel,
// grant_staffel.go "WER COORDINATOR SEIN KANN").
//
// Eine Erneuerungs-Bescheinigung schaltet 800 AEQ Staffel frei. Bisher konnte
// jeder registrierte Mensch ohne offene Staffel einen Schluessel binden und
// bescheinigen -- eine Farm mit einem einzigen alten Konto haette allen ihren
// Kunstfiguren die zweite Pruefung bescheinigt.
//
// Jetzt: Coordinator darf nur sein, wer zur Zeit der Bescheinigung
// (issued_at, steht in der Transaktion) einen Validator-Schluessel im
// Kettenregister haelt -- dieselben Erzeugerfenster wie bei der
// Erzeugerpruefung (validator_register_leser.go: Verlauf der Bindungen,
// Frist, nur Menschen, umstritten: keiner). Zulassung und Entzug folgen damit
// dem Register, das jeder Knoten aus der Kette liest; wer seine Bindung
// verliert oder den Schluessel abgibt, bescheinigt nicht mehr.
//
// NOCH NICHT GENUG (Sicherheitsdurchgang #310, H1). Eine Bindung kostet
// nichts: sie verlangt nur, dass der Betreiber ein Mensch ist, keinen
// Listenplatz und keinen Einsatz -- und ein Schluessel, der nie einen Block
// erzeugt, kann auch nicht doppelt signieren. Jeder registrierte Mensch
// bindet einen frischen Schluessel und ist zwei Stunden spaeter
// Coordinator; gegen seinen Willen entziehen laesst er sich nicht. Die
// Staffel bleibt deshalb beim Platzhalter (TestStaffel_SchlaeftBisZulassungUndStreng),
// bis die Zulassung an etwas Knappes gebunden ist, das im Konsens steht.
//
// Gilt die Staffel, bevor das Register gelesen wird (registerLeserAb), ist
// niemand zugelassen -- fail-closed. TestStaffel_ZulassungVorDerStaffel
// erzwingt die Reihenfolge der Stichtage.
//
// GRENZE. Gezaehlt wird issued_at, nicht die Blockzeit -- sonst urteilten
// Annahme (Uhr) und Nachspielen (Blockzeit) an der Grenze verschieden.
// issued_at waehlt der Coordinator: eine Bescheinigung, die auf einen
// Zeitpunkt vor dem Ende seiner Bindung DATIERT ist, besteht beim
// Nachspielen bis zu 7 Tage danach (erneuerungHoechstensAlt) -- auch wenn
// er sie erst nach dem Ende unterschreibt und ein Erzeuger sie direkt in
// seinen Block legt. Die Annahme ehrlicher Knoten nimmt hoechstens 15
// Minuten alte.

// CoordinatorZulassung: haelt mensch zur Zeit t einen Validator-Schluessel?
type CoordinatorZulassung func(mensch string, t int64) (bool, error)

// coordinatorSchluesselGrenze: hoechstens so viele Schluessel je Mensch im
// Verlauf (eine Bindung je Tag, validatorBindungAbstand).
const coordinatorSchluesselGrenze = 1000

// coordinatorZugelassenIn: die Zulassung aus dem Verlauf, in q (Transaktion
// oder Verbindung). Ein Lesefehler ist ein Fehler, nie "zugelassen".
func coordinatorZugelassenIn(q sqlExecutor, mensch string, t int64) (bool, error) {
	mensch = strings.ToLower(strings.TrimSpace(mensch))
	rows, err := q.Query(`SELECT DISTINCT signing_address FROM validator_verlauf WHERE operator_wallet = $1 LIMIT $2`,
		mensch, coordinatorSchluesselGrenze+1)
	if err != nil {
		return false, fmt.Errorf("Zulassung: %w", err)
	}
	var schluessel []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return false, fmt.Errorf("Zulassung: %w", err)
		}
		schluessel = append(schluessel, strings.ToLower(s))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("Zulassung: %w", err)
	}
	if len(schluessel) > coordinatorSchluesselGrenze {
		return false, fmt.Errorf("Zulassung: mehr als %d Schluessel", coordinatorSchluesselGrenze)
	}
	if len(schluessel) == 0 {
		return false, nil
	}
	zeilen, err := verlaufLesen(q, schluessel)
	if err != nil {
		return false, fmt.Errorf("Zulassung: %w", err)
	}
	// Wie im Erzeugerstand: nur Fenster menschlicher Betreiber zaehlen, auch
	// fuer "umstritten". In EINER Abfrage (Sicherheitsdurchgang #310, M1):
	// die Betreiber sind alle, die je einen dieser Schluessel gebunden haben
	// -- je Betreiber eine Abfrage liesse sich aufblaehen. Mehr als die
	// Grenze ist ein Fehler, nicht abgeschnitten.
	betreiber := map[string]bool{}
	for _, z := range zeilen {
		betreiber[z.betreiber] = true
	}
	if len(betreiber) > coordinatorSchluesselGrenze {
		return false, fmt.Errorf("Zulassung: mehr als %d Betreiber", coordinatorSchluesselGrenze)
	}
	liste := make([]string, 0, len(betreiber))
	for b := range betreiber {
		liste = append(liste, b)
	}
	menschen := map[string]bool{}
	rows, err = q.Query(`SELECT lower(address) FROM chain_accounts WHERE lower(address) = ANY($1) AND is_human`, pq.Array(liste))
	if err != nil {
		return false, fmt.Errorf("Zulassung: %w", err)
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return false, fmt.Errorf("Zulassung: %w", err)
		}
		menschen[a] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("Zulassung: %w", err)
	}
	gesucht := map[string]bool{}
	for _, s := range schluessel {
		gesucht[s] = true
	}
	st := &erzeugerStand{fenster: fensterAusIntervallen(bindungsIntervalle(zeilen), menschen, gesucht)}
	for _, s := range schluessel {
		if st.erzeugerFenster(s, t) == mensch {
			return true, nil
		}
	}
	return false, nil
}

// coordinatorZugelassen: fuer die Annahme, ohne gehaltene Sperre, mit
// Zeitgrenze.
func (cs *ChainState) coordinatorZugelassen(mensch string, t int64) (bool, error) {
	if cs.db == nil {
		return false, fmt.Errorf("Zulassung: keine Datenbank")
	}
	ctx, abbruch := context.WithTimeout(context.Background(), 5*time.Second)
	defer abbruch()
	return coordinatorZugelassenIn(dbMitKontext{ctx: ctx, db: cs.db}, mensch, t)
}
