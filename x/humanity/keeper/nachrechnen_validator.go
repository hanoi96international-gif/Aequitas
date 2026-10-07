package keeper

import (
	"context"
	"strings"
)

// Validatoren-Runde nachrechnen (Validator-Register, Schritt 3, Teil 2).
//
// Vor registerLeserAb prueft jeder Knoten nur "Empfaenger ist Mensch" -- die
// Gewichte kamen aus registered_nodes, das jeder Knoten selbst fuehrt. Ab
// dem Stichtag traegt jede Zahlung Anker und Zeit der Runde
// (validator_belohnung_kette.go); jeder Knoten rechnet Gewichte und Betraege
// selbst:
//
//   - validator_kein_mensch:     Empfaenger ist kein registrierter Mensch.
//   - validator_ohne_anker:      ab zehn Minuten nach dem Stichtag eine
//     Zahlung ohne Anker -- die alte Form ist dann nicht mehr nachrechenbar.
//   - validator_anker_zu_frueh:  Anker vor dem Stichtag.
//   - validator_zeit:            Zeit der Runde mehr als zehn Minuten neben
//     dem Block.
//   - validator_anker_unbekannt: der Anker ist kein nachgespielter Block
//     dieses Knotens. Ein ehrlicher Erzeuger nimmt einen Block, den er selbst
//     nachgespielt hat, und legt die Zahlung in einen eigenen spaeteren
//     Block -- der Anker liegt dann in dessen Vergangenheit und ist bei jedem
//     Nachspielenden schon da.
//   - validator_anker_zeit:      Anker nicht in [T-10 min, T].
//   - validator_anker_wechsel:   zwei Anker oder Zeiten in einer Runde.
//   - validator_doppelt:         derselbe Betreiber zweimal in einer Runde.
//   - validator_nicht_anwesend:  Empfaenger ohne nachgerechneten Anteil.
//   - validator_anteil:          Betrag weicht ab (1 Mikro Rundung).
//   - validator_empfaenger:      nicht jeder Betreiber bekam seinen Anteil.
//
// Fehlt dem Knoten Geschichte im Kegel, rechnet er die Betraege nicht nach
// (unsicher) -- nur, was ohne Erwartung geht. Ebenso nach einem Neustart
// mitten in der Runde (wie beim Grundeinkommen). Die Erwartung wird beim
// ersten validator_distribution der Runde einmal berechnet, wie beim
// Erzeuger aus dem Topf zu Beginn der Runde.

// validatorRundePruefung: Stand der laufenden Validatoren-Runde beim
// Nachspielen.
type validatorRundePruefung struct {
	unsicher bool
	aktiv    bool
	neu      bool // die Runde traegt einen Anker
	anker    string
	zeit     int64
	// Nur wenn neu und nicht unsicher: Betreiber -> erwarteter Betrag
	// (Mikro, > 0). Nach dem Anlegen unveraendert.
	erwartet map[string]int64
	n        int64
	bedacht  map[string]int64 // nur Menschen; Wallet -> laufende Nummer
}

func (r validatorRundePruefung) zurueck() validatorRundePruefung {
	for k, nr := range r.bedacht {
		if nr > r.n {
			delete(r.bedacht, k)
		}
	}
	return r
}

// nachrechnenValidatorLocked: eine Validatoren-Gutschrift, VOR dem Anwenden.
func (cs *ChainState) nachrechnenValidatorLocked(tx *Transaction, blockZeit int64) error {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	// Was jeder Knoten immer gleich weiss: nur registrierte Menschen.
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	if acc, ok := cs.accounts.Get(wallet); !ok || !acc.IsHuman {
		return nachrechnenAbweichung("validator_kein_mensch", blockZeit,
			"%s ist kein registrierter Mensch (%.6f AEQ)", kurzAdresse(wallet), tx.Amount)
	}
	neu := tx.Anker != "" || tx.DistributionAt != 0
	if !neu {
		if validatorKettePflicht(blockZeit) {
			return nachrechnenAbweichung("validator_ohne_anker", blockZeit,
				"%s: %.6f AEQ ohne Anker", kurzAdresse(wallet), tx.Amount)
		}
		return nil
	}
	if !validatorKetteAktiv(tx.DistributionAt) {
		return nachrechnenAbweichung("validator_anker_zu_frueh", blockZeit,
			"Runde %d vor dem Stichtag", tx.DistributionAt)
	}
	if d := tx.DistributionAt - blockZeit; d > validatorAnkerHoechstensAlt || d < -validatorAnkerHoechstensAlt {
		return nachrechnenAbweichung("validator_zeit", blockZeit,
			"Runde %d, Blockzeit %d (Abstand %ds)", tx.DistributionAt, blockZeit, d)
	}
	r := &cs.validatorRunde
	if !r.aktiv {
		unsicher := r.unsicher
		*r = validatorRundePruefung{aktiv: true, neu: true, unsicher: unsicher, anker: tx.Anker, zeit: tx.DistributionAt,
			bedacht: make(map[string]int64)}
		if !unsicher {
			if err := cs.validatorErwartungLocked(r, blockZeit); err != nil {
				return err
			}
		}
	} else if !r.neu || tx.Anker != r.anker || tx.DistributionAt != r.zeit {
		return nachrechnenAbweichung("validator_anker_wechsel", blockZeit,
			"Anker %s/%d, in dieser Runde bisher %s/%d", kurzAdresse(tx.Anker), tx.DistributionAt, kurzAdresse(r.anker), r.zeit)
	}
	r.n++
	if _, schon := r.bedacht[wallet]; schon {
		if err := nachrechnenAbweichung("validator_doppelt", blockZeit,
			"%s bekommt in dieser Runde zum zweiten Mal", kurzAdresse(wallet)); err != nil {
			return err
		}
	} else {
		r.bedacht[wallet] = r.n
	}
	if r.unsicher {
		return nil // ohne Geschichte keine Erwartung
	}
	soll, ok := r.erwartet[wallet]
	if !ok {
		return nachrechnenAbweichung("validator_nicht_anwesend", blockZeit,
			"%s hat keinen nachgerechneten Anteil (%.6f AEQ)", kurzAdresse(wallet), tx.Amount)
	}
	if d := NewDecimal(tx.Amount).Micro() - soll; d > 1 || d < -1 {
		return nachrechnenAbweichung("validator_anteil", blockZeit,
			"%s: %.6f AEQ, nachgerechnet %.6f", kurzAdresse(wallet), tx.Amount, NewDecimalFromMicro(soll).Float())
	}
	return nil
}

// validatorErwartungLocked: Anker pruefen und die Erwartung der Runde
// berechnen. Fehlt Geschichte, wird die Runde unsicher. Ein Lesefehler ist
// ein Fehler (in der Transaktion des Blocks: der Block geht zurueck).
func (cs *ChainState) validatorErwartungLocked(r *validatorRundePruefung, blockZeit int64) error {
	if cs.db == nil {
		r.unsicher = true
		return nil
	}
	q := cs.dbExecCtx(context.Background())
	ankerZeit, gefunden, err := validatorAnkerZeit(q, r.anker)
	if err != nil {
		return err
	}
	if !gefunden {
		r.unsicher = true
		return nachrechnenAbweichung("validator_anker_unbekannt", blockZeit, "Anker %s", kurzAdresse(r.anker))
	}
	if ankerZeit > r.zeit || ankerZeit < r.zeit-validatorAnkerHoechstensAlt {
		r.unsicher = true
		return nachrechnenAbweichung("validator_anker_zeit", blockZeit,
			"Anker %s von %d, Runde %d", kurzAdresse(r.anker), ankerZeit, r.zeit)
	}
	gewichte, stand, err := validatorGewichte(q, r.anker, r.zeit)
	if err != nil {
		return err
	}
	if stand == kegelLuecke {
		r.unsicher = true
		return nil
	}
	r.erwartet = map[string]int64{}
	summe := gewichteSumme(gewichte)
	if stand == kegelZuGross || summe == 0 {
		return nil // keine Zahlung erwartet
	}
	topf := NewDecimalFromMicro(cs.topfMikroLocked(validatorsPoolAddr)).Float()
	for op, w := range gewichte {
		if m := NewDecimal(validatorAnteil(topf, w, summe)).Micro(); m > 0 {
			r.erwartet[op] = m
		}
	}
	return nil
}

// nachrechnenValidatorAbschlussLocked: bei validator_distribution_pool_zero
// oder spaetestens der Rundenmarke.
func (cs *ChainState) nachrechnenValidatorAbschlussLocked(blockZeit int64) error {
	r := cs.validatorRunde
	cs.validatorRunde = validatorRundePruefung{}
	if !r.aktiv || !r.neu || r.unsicher || r.erwartet == nil {
		return nil
	}
	fehlt := 0
	for w := range r.erwartet {
		if _, ok := r.bedacht[w]; !ok {
			fehlt++
		}
	}
	if fehlt > 0 {
		return nachrechnenAbweichung("validator_empfaenger", blockZeit,
			"%d von %d Betreibern ohne Gutschrift", fehlt, len(r.erwartet))
	}
	return nil
}
