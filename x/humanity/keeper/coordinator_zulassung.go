package keeper

import (
	"context"
	"fmt"
	"strings"
	"time"
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
// dem Register, das jeder Knoten aus der Kette liest: wer als Validator
// gebunden ist, haftet fuer Doppelsignaturen und ist (im geschlossenen
// Betrieb) auf der Liste; wer seine Bindung verliert, bescheinigt nicht mehr.
//
// Gilt die Staffel, bevor das Register gelesen wird (registerLeserAb), ist
// niemand zugelassen -- fail-closed. TestStaffel_ZulassungVorDerStaffel
// erzwingt die Reihenfolge der Stichtage.
//
// GRENZE. Gezaehlt wird issued_at, nicht die Blockzeit -- sonst urteilten
// Annahme (Uhr) und Nachspielen (Blockzeit) an der Grenze verschieden. Eine
// Bescheinigung, die ein Coordinator vor dem Ende seiner Bindung ausgestellt
// hat, bleibt bis zu 7 Tage gueltig (erneuerungHoechstensAlt); bei der
// Annahme hoechstens 15 Minuten.

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
	// fuer "umstritten".
	menschen := map[string]bool{}
	for _, z := range zeilen {
		if _, gesehen := menschen[z.betreiber]; gesehen {
			continue
		}
		m, err := istMenschIn(q, z.betreiber)
		if err != nil {
			return false, fmt.Errorf("Zulassung: %w", err)
		}
		menschen[z.betreiber] = m
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
