package keeper

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"
)

// Vollstaendigkeit der Umlaufrunde (wirtschaft3.go, d). Nur ab dem
// Gleichgewicht -- davor Leerlauf, damit kein bestehender Block anders
// nachspielt.
//
// # WARUM
//
// pruefeUmlaufLocked rechnet jeden Betrag nach, der im Block STEHT. Was ein
// Erzeuger weglaesst, sieht es nicht: Er koennte Freunde (oder sich selbst)
// aus jeder Runde auslassen und haette ein Guthaben ohne Umlauf -- genau die
// Luecke, die das Gleichgewicht schliessen soll. Und ein Konto zweimal zu
// belasten, faellt bei kleinen Betraegen unter die Toleranz.
//
//   - umlauf_ausgelassen: ein Konto schuldet zur Rundenzeit mindestens
//     umlaufMindestBetrag und steht in der Runde nicht.
//   - umlauf_doppelt:     dasselbe Konto zweimal in einer Runde.
//   - umlauf_reihenfolge: ein Umlauf nach anderen Transaktionen derselben
//     Runde. Der Erzeuger rechnet den Umlauf zuerst (RunDailyDistributionAtomic).
//   - umlauf_rundenzeit:  Umlauf-Transaktionen und Rundenmarke einer Runde
//     tragen verschiedene Rundenzeiten.
//   - umlauf_konten_unlesbar: die eigene Kontenliste war nicht lesbar --
//     ohne sie kein Glaube; wie ubi_menschen_unlesbar.
//
// # WIE
//
// Der Erzeuger rechnet alle Umlaeufe einer Runde zuerst, nach Adresse
// sortiert, dann Grundeinkommen, Freigaben und den Rest, zuletzt die
// Rundenmarke. Die erste Transaktion der Runde, die kein Umlauf ist, beendet
// also den Umlaufteil. Genau dann haelt jeder Knoten fest, welche Konten
// NICHT belastet wurden und welchen Stand sie haben -- ihr Stand ist jetzt
// derselbe, mit dem der Erzeuger gerechnet hat: belastet hat sie niemand,
// und das Aufloesen von LP-Anteilen anderer Konten aendert den Wert eines
// Anteils nicht (anteilig aus beiden Reserven). An der Rundenmarke, die die
// Rundenzeit sicher traegt, wird mit derselben Funktion wie beim Erzeuger
// (umlaufBetragRoh) gerechnet. Gemeldet wird nur, was klar ueber der
// Schwelle liegt (liegegeldToleranz), damit Rundung nie einen ehrlichen
// Block anschlaegt.
//
// # GRENZEN
//
// belastet enthaelt nur Konten, die im eigenen Zustand existieren; offen nur
// Konten, die in sieben Tagen ueberhaupt die Schwelle erreichen koennten.
// Beides ist durch die Zahl der Konten begrenzt, nicht durch das, was ein
// Block hineinschreibt. Der Aufwand je Runde ist der des Erzeugers: eine
// Abfrage der Konten und ein Durchlauf (Endfassung, Abschnitt 10, Grenzen:
// ab etwa 10^5 Konten in Abschnitte teilen).
//
// # NEUSTART, RUECKROLLEN, UMSTELLUNG
//
// Wie nachrechnen_ubi.go: Nach einem Neustart ist der Anfang der laufenden
// Runde unbekannt; bis zur naechsten Rundenmarke wird nichts gemeldet.
// Rueckrollen ueber die laufende Nummer (zurueck). Ob die Runde unter das
// Gleichgewicht faellt, entscheidet ihr erster Augenblick; eine Runde, die
// die Aktivierung ueberspannt, wird nicht auf Vollstaendigkeit geprueft.
// Es wird hoechstens zu wenig gemeldet, nie zu viel.

const (
	umlaufPhaseZwischen = iota // zwischen zwei Runden (nach einer Rundenmarke)
	umlaufPhaseUmlauf          // im Umlaufteil einer Runde
	umlaufPhaseNachher         // Umlaufteil vorbei, Rest der Runde
)

type umlaufOffen struct {
	stand float64
	art   kontoart
}

// umlaufRundePruefung: Stand der laufenden Umlaufrunde beim Nachspielen.
// Unter cs.mu; im Rueckroll-Snapshot (Kopie in O(1), siehe zurueck).
type umlaufRundePruefung struct {
	unsicher bool // Anfang nicht beobachtet (Neustart): nichts melden
	aktiv    bool // die Runde faellt unter das Gleichgewicht
	phase    int
	at       int64 // Rundenzeit der Umlauf-Transaktionen (0 = keine gesehen)
	n        int64
	belastet map[string]int64 // Wallet -> laufende Nummer
	// offen: am Ende des Umlaufteils festgehalten, danach nur gelesen.
	// nil = nicht festgehalten (inaktiv, unsicher oder unlesbar).
	offen map[string]umlaufOffen
}

func (r umlaufRundePruefung) zurueck() umlaufRundePruefung {
	for k, nr := range r.belastet {
		if nr > r.n {
			delete(r.belastet, k)
		}
	}
	return r
}

var (
	umlaufRundenGeprueft      atomic.Int64
	umlaufRundenUebersprungen atomic.Int64
)

func umlaufRundeStand() map[string]interface{} {
	return map[string]interface{}{
		"runden_geprueft":      umlaufRundenGeprueft.Load(),
		"runden_uebersprungen": umlaufRundenUebersprungen.Load(),
	}
}

// nachrechnenUmlaufRundeLocked: jede Transaktion einer Tagesrunde, VOR dem
// Anwenden (nachrechnenTxLocked). Caller haelt cs.mu.
func (cs *ChainState) nachrechnenUmlaufRundeLocked(tx *Transaction, blockZeit int64) error {
	if !isDistributionRoundTxType(tx.Type) {
		return nil
	}
	r := &cs.umlaufRunde
	melden := func() bool { return r.aktiv && !r.unsicher }

	if tx.Type == "umlauf" {
		switch r.phase {
		case umlaufPhaseNachher:
			if melden() {
				if err := nachrechnenAbweichung("umlauf_reihenfolge", blockZeit,
					"%s: Umlauf nach anderen Transaktionen der Runde", kurzAdresse(tx.Wallet)); err != nil {
					return err
				}
			}
			// Beobachten: neu anfangen, aber ohne Vollstaendigkeit -- der
			// Anfang dieser Runde ist nicht mehr sicher.
			*r = umlaufRundePruefung{unsicher: true}
			fallthrough
		case umlaufPhaseZwischen:
			r.aktiv = gleichgewichtAktiv(tx.DistributionAt)
			r.phase = umlaufPhaseUmlauf
			r.at = tx.DistributionAt
			r.belastet = make(map[string]int64)
		case umlaufPhaseUmlauf:
			if tx.DistributionAt != r.at && melden() {
				if err := nachrechnenAbweichung("umlauf_rundenzeit", blockZeit,
					"%s: Rundenzeit %d, die Runde hat %d", kurzAdresse(tx.Wallet), tx.DistributionAt, r.at); err != nil {
					return err
				}
			}
		}
		if !r.aktiv {
			return nil
		}
		wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
		cs.ensureAccountLoadedCtx(context.Background(), wallet)
		if _, ok := cs.accounts.Get(wallet); !ok {
			return nil // applyUmlaufDeltaLocked tut nichts; nicht in die Menge
		}
		r.n++
		if _, schon := r.belastet[wallet]; schon {
			if melden() {
				return nachrechnenAbweichung("umlauf_doppelt", blockZeit,
					"%s steht zweimal in der Runde", kurzAdresse(wallet))
			}
			return nil
		}
		r.belastet[wallet] = r.n
		return nil
	}

	// Jede andere Transaktion der Runde: der Umlaufteil ist vorbei.
	if r.phase != umlaufPhaseNachher {
		if r.phase == umlaufPhaseZwischen {
			// Eine Runde ohne jeden Umlauf.
			start := blockZeit
			if tx.DistributionAt > 0 {
				start = tx.DistributionAt
			}
			r.aktiv = gleichgewichtAktiv(start)
		}
		r.phase = umlaufPhaseNachher
		if melden() {
			offen, err := cs.umlaufOffeneLocked(r.belastet, blockZeit)
			if err != nil {
				if aerr := nachrechnenAbweichung("umlauf_konten_unlesbar", blockZeit, "%v", err); aerr != nil {
					return aerr
				}
				r.unsicher = true
			} else {
				r.offen = offen
			}
		}
	}
	if tx.Type != "distribution_round_marker" {
		return nil
	}
	ende := *r
	*r = umlaufRundePruefung{} // Rundengrenze: der naechste Anfang ist beobachtet
	return cs.umlaufVollstaendigLocked(ende, tx.DistributionAt, blockZeit)
}

// umlaufOffeneLocked: jedes Konto, das der Erzeuger haette pruefen muessen
// (umlaufKandidatenLocked), nicht belastet wurde und in sieben Tagen
// ueberhaupt die Schwelle erreichen koennte -- mit seinem Stand jetzt.
func (cs *ChainState) umlaufOffeneLocked(belastet map[string]int64, at int64) (map[string]umlaufOffen, error) {
	var q sqlExecutor
	if cs.db != nil {
		q = cs.dbExecCtx(context.Background())
	}
	adressen, err := cs.umlaufKandidatenLocked(q, at)
	if err != nil {
		return nil, err
	}
	offen := make(map[string]umlaufOffen)
	for _, a := range adressen {
		if _, ja := belastet[a]; ja {
			continue
		}
		cs.ensureAccountLoadedCtx(context.Background(), a)
		acc, ok := cs.accounts.Get(a)
		if !ok {
			continue
		}
		art := cs.kontoartVon(a, acc.IsHuman)
		stand := cs.umlaufStandLocked(acc, at)
		if cs.umlaufBetragRoh(a, art, stand, at, 7*86400) < umlaufMindestBetrag {
			continue
		}
		offen[a] = umlaufOffen{stand: stand, art: art}
	}
	return offen, nil
}

// umlaufVollstaendigLocked: an der Rundenmarke. at ist ihre Rundenzeit.
func (cs *ChainState) umlaufVollstaendigLocked(r umlaufRundePruefung, at, blockZeit int64) error {
	if !r.aktiv || r.unsicher || r.offen == nil {
		return nil
	}
	if !gleichgewichtAktiv(at) {
		umlaufRundenUebersprungen.Add(1) // die Runde ueberspannt die Aktivierung
		return nil
	}
	if r.at != 0 && r.at != at {
		return nachrechnenAbweichung("umlauf_rundenzeit", blockZeit,
			"Umlauf zur Rundenzeit %d, Rundenmarke %d", r.at, at)
	}
	// Zeitraum wie pruefeUmlaufLocked und umlaufLocked.
	w := cs.wirt()
	var vorher int64
	bekannt := true
	w.mitSperre(func() {
		switch {
		case at > w.letzterUmlauf:
			vorher = w.letzterUmlauf
		case at == w.laufAt:
			vorher = w.laufVorher
		default:
			bekannt = false
		}
	})
	if !bekannt {
		umlaufRundenUebersprungen.Add(1)
		return nil
	}
	sekunden := int64(86400)
	if vorher > 0 {
		sekunden = at - vorher
	}
	if sekunden <= 0 {
		umlaufRundenUebersprungen.Add(1)
		return nil
	}
	if sekunden > 7*86400 {
		sekunden = 7 * 86400
	}
	umlaufRundenGeprueft.Add(1)
	var fehlt []string
	for a, o := range r.offen {
		if cs.umlaufBetragRoh(a, o.art, o.stand, at, sekunden) >= umlaufMindestBetrag+liegegeldToleranz {
			fehlt = append(fehlt, a)
		}
	}
	if len(fehlt) == 0 {
		return nil
	}
	sort.Strings(fehlt)
	o := r.offen[fehlt[0]]
	return nachrechnenAbweichung("umlauf_ausgelassen", blockZeit,
		"%d Konten schulden Umlauf und fehlen in der Runde, zuerst %s (Stand %.6f AEQ, %.6f AEQ)",
		len(fehlt), kurzAdresse(fehlt[0]), o.stand, cs.umlaufBetragRoh(fehlt[0], o.art, o.stand, at, sekunden))
}
