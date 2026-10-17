package keeper

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Zeitplan der taeglichen Verteilungsrunde -- was der Planer (cmd/aequitasd)
// als "letzte Runde" liest, und wann er nach einer abgewiesenen Runde bald
// erneut prueft.
//
// # WARUM (Beobachtung 08.-16.10.2026, C1)
//
// last_ubi_at stand seit dem 02.10. still, und ubi_next_payout_secs zeigte
// fast immer einen Zeitpunkt in den naechsten 15 Minuten statt 20:00 Berlin.
// Die Toepfe waren leer (ein Mensch, pool_ubi 0.0000) -- dass nichts
// ausgezahlt wurde, war richtig. Falsch war der Planer: er las last_ubi_at
// als "letzte Runde". Das rueckt aber nur vor, wenn das Grundeinkommen etwas
// AUSZAHLT (RunDailyDistributionAtomic, `if ubiTotal > 0`). Also:
//
//   - nach jedem Neustart (jedem Deploy) galt die Runde als ueberfaellig
//     (computeFirstUBITarget: >= 24 h) und lief eine Minute spaeter -- zu
//     einer beliebigen Uhrzeit;
//   - um 20:00 wies TryLockDistribution die Runde dann ab (die Sperre war
//     erst Stunden alt), und VerteilungHeuteOffen(last_ubi_at) hielt den
//     Termin fuer offen -- fuer immer, weil last_ubi_at nie vorrueckte. Der
//     Planer pruefte alle 15 Minuten erneut, den ganzen Tag, und lief, sobald
//     die Sperre 23 h 55 min alt war: wieder zur Deploy-Uhrzeit, nie um 20:00.
//
// Jede ausgefuehrte Runde schreibt seit der Umlaufsicherung (wirtschaftAktiv)
// unbedingt last_distribution_round_at -- dieselbe Marke, die jeder Knoten
// beim Nachspielen setzt. Die ist das richtige Signal fuer "eine Runde lief".
//
// Nichts hier aendert Betraege, Empfaenger, Konsensregeln oder das, was ein
// nachspielender Knoten prueft; es aendert nur, WANN der erzeugende Knoten
// seine Runde startet -- zurueck auf den dokumentierten 20:00-Termin.

// verteilSperreFrist: so lange nach der letzten Runde (bzw. Sperre) weist
// TryLockDistribution eine neue ab. Eine Quelle fuer die Sperre und fuer die
// Frage, wann sie wieder frei ist (VerteilungBaldNachholen).
const verteilSperreFrist = 23*time.Hour + 55*time.Minute

// verteilNachholFrist: bis wie lange nach dem 20:00-Termin eine abgewiesene
// Runde noch am selben Termin nachgeholt wird (#354: gestern lief ein Retry
// erst um 20:15, heute ist die Sperre um 20:00 noch 15 Minuten zu). Wird die
// Sperre erst spaeter frei, lag die letzte Runde weit neben dem Termin; sie
// jetzt nachzuholen hielte die Runde dauerhaft auf dieser falschen Uhrzeit.
// Dann gilt der naechste Termin -- die Toepfe bleiben stehen, nichts geht
// verloren.
const verteilNachholFrist = time.Hour

// LetzteVerteilungsrunde: der Zeitpunkt der letzten AUSGEFUEHRTEN Runde --
// die spaetere von last_distribution_round_at (jede Runde) und last_ubi_at
// (nur Runden mit Auszahlung; aeltere Ketten ohne Marke). 0 = nie.
func (cs *ChainState) LetzteVerteilungsrunde() int64 {
	letzte := cs.GetLastUBIAt()
	if v := cs.getConfigValueDB("last_distribution_round_at"); v != "" {
		var t int64
		if _, err := fmt.Sscan(v, &t); err == nil && t > letzte {
			letzte = t
		}
	}
	return letzte
}

// VerteilungsSperreAb: ab wann TryLockDistribution zuletzt gesperrt hat --
// die spaetere von distribution_lock_at und last_ubi_at (die Sperre prueft
// beide). 0 = keine Sperre.
func (cs *ChainState) VerteilungsSperreAb() int64 {
	ab := cs.GetLastUBIAt()
	if v := cs.getConfigValueDB("distribution_lock_at"); v != "" {
		var t int64
		if _, err := fmt.Sscan(v, &t); err == nil && t > ab {
			ab = t
		}
	}
	return ab
}

// NaechsteVerteilungAt: was der Planer zuletzt als naechsten Versuch
// eingetragen hat (next_ubi_at). 0 = noch nichts geplant.
func (cs *ChainState) NaechsteVerteilungAt() int64 {
	var t int64
	if v := cs.getConfigValueDB("next_ubi_at"); v != "" {
		fmt.Sscan(v, &t)
	}
	return t
}

// VerteilungBaldNachholen: soll der Planer nach einer von der Sperre
// abgewiesenen Runde bald erneut pruefen (true) oder bis zum naechsten
// 20:00-Termin warten (false)? Bald nur, wenn der aktuelle Termin noch offen
// ist (VerteilungHeuteOffen, gemessen an der letzten AUSGEFUEHRTEN Runde) UND
// die Sperre spaetestens verteilNachholFrist nach diesem Termin frei wird.
func VerteilungBaldNachholen(letzteRunde, sperreAb int64, jetzt time.Time, loc *time.Location) bool {
	if !VerteilungHeuteOffen(letzteRunde, jetzt, loc) {
		return false
	}
	if sperreAb <= 0 {
		return true
	}
	frei := time.Unix(sperreAb, 0).Add(verteilSperreFrist)
	return !frei.After(verteilTerminZeit(jetzt.In(loc)).Add(verteilNachholFrist))
}

// verteilTerminZeit: der Zeitpunkt (20:00 in t's Zone) des letzten Termins
// bis t -- dieselbe Zuordnung wie verteilTermin.
func verteilTerminZeit(t time.Time) time.Time {
	if t.Hour() < 20 {
		t = t.AddDate(0, 0, -1)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 20, 0, 0, 0, t.Location())
}

// Was das Grundeinkommen in der letzten Runde dieses Prozesses ergab --
// damit "Runde lief, zahlte aber nichts" einen Grund traegt statt nur ein
// stehendes last_ubi_at.
const (
	ubiErgebnisAusgezahlt    = "ausgezahlt"
	ubiErgebnisTopfLeer      = "topf_leer"
	ubiErgebnisKeineMenschen = "keine_menschen"
	ubiErgebnisAnteilNull    = "anteil_rundet_auf_null"
)

var letztesUBIErgebnis atomic.Pointer[string]

func merkeUBIErgebnis(e string) { letztesUBIErgebnis.Store(&e) }

// LetztesUBIErgebnis: siehe oben; leer, solange keine Runde gerechnet hat.
// Nur Diagnose -- eine danach zurueckgerollte Runde meldet der Planer als
// "failed", nicht mit diesem Wert.
func LetztesUBIErgebnis() string {
	if p := letztesUBIErgebnis.Load(); p != nil {
		return *p
	}
	return ""
}
