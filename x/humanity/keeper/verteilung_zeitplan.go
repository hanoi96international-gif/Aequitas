package keeper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// # MINDESTABSTAND ZWISCHEN ZWEI RUNDEN (Erzeuger)
//
// Jeder nachspielende Knoten verwirft eine Runde, deren DistributionAt
// weniger als 24 h nach seiner letzten Rundenmarke liegt
// (distributionRoundToSkip, block.go: `tx.DistributionAt - last < 24*3600`,
// fuer distribution_round_marker gegen last_distribution_round_at und fuer
// ubi_distribution_finalize gegen last_ubi_at). TryLockDistribution liess
// eine neue Runde aber schon nach 23 h 55 min zu -- eine Runde in diesem
// Fenster haette der Erzeuger angewandt und jeder andere Knoten
// uebersprungen: Marke, Umlaufzeit und (bei einer Auszahlung) Kontostaende
// liefen auseinander.
//
// Der Erzeuger startet darum nur, wenn GENAU der Wert, den er in die
// Transaktionen schreibt (ubiAt = DistributionAt), mindestens 24 h nach
// beiden Marken liegt. Die Zeitbasis ist auf beiden Seiten dieselbe: zwei vom
// Erzeuger gewaehlte ganze Sekunden, verglichen ohne Uhr des Nachspielenden
// und ohne Blockzeit. Eine Reserve ueber 24 h braucht es deshalb nicht -- und
// sie schadete: jede Runde muesste dann mindestens Reserve spaeter laufen als
// die vorige, der 20:00-Plan wanderte taeglich um die Reserve nach hinten.
// Die Rueckwaerts-Uhrstellung ist abgedeckt, weil geprueft wird, was
// geschrieben wird, nicht eine fruehere Lesung der Uhr.
//
// Nichts an distributionRoundToSkip oder am Nachspielen aendert sich.

// verteilMindestabstand: muss distributionRoundToSkip entsprechen (24*3600 s);
// TestVerteilAbstand_DeckungsgleichMitNachspielen haelt beide zusammen.
const verteilMindestabstand int64 = 24 * 3600

// verteilAbstandFrei: ab welcher DistributionAt eine neue Runde von keinem
// nachspielenden Knoten mehr verworfen wird. 0 = keine Marke, jederzeit.
func verteilAbstandFrei(letzteRunde, letzteAuszahlung int64) int64 {
	letzte := letzteRunde
	if letzteAuszahlung > letzte {
		letzte = letzteAuszahlung
	}
	if letzte <= 0 {
		return 0
	}
	return letzte + verteilMindestabstand
}

// VerteilAbstandErlaubt: darf der Erzeuger eine Runde mit DistributionAt
// ubiAt starten, ohne dass nachspielende Knoten sie verwerfen?
func VerteilAbstandErlaubt(ubiAt, frei int64) bool {
	return ubiAt >= frei
}

// VerteilungAbstandFreiAb liest beide Marken STRENG: fehlt eine Zeile, zaehlt
// sie als 0 (nie gelaufen); jeder Lesefehler und jeder nicht lesbare Wert ist
// ein Fehler -- der Aufrufer startet dann keine Runde (fail-closed). Anders
// als LetzteVerteilungsrunde, die nur den Zeitplan schaetzt.
func (cs *ChainState) VerteilungAbstandFreiAb() (int64, error) {
	if cs.db == nil {
		return 0, nil // ohne Datenbank (Einzeltests): keine Marken
	}
	return verteilAbstandFreiLesen(cs.db)
}

// zeileLeser: das Stueck von *sql.DB und *sql.Tx, das verteilAbstandFreiLesen
// braucht.
type zeileLeser interface {
	QueryRow(query string, args ...any) *sql.Row
}

// verteilAbstandFreiLesen liest beide Marken streng ueber q -- ausserhalb
// einer Runde ueber den Pool, in RunDailyDistributionAtomic ueber deren
// Transaktion (verteilAbstandInRundePruefen).
func verteilAbstandFreiLesen(q zeileLeser) (int64, error) {
	runde, auszahlung, err := verteilMarkenLesen(q)
	if err != nil {
		return 0, err
	}
	return verteilAbstandFrei(runde, auszahlung), nil
}

// verteilMarkenLesen liest last_distribution_round_at und last_ubi_at
// STRENG: fehlt eine Zeile oder ist sie leer, zaehlt sie als 0 (nie
// gelaufen); jeder Lesefehler und jeder nicht ganzzahlige oder negative Wert
// ist ein Fehler. Erzeuger (Mindestabstand) und Nachspielen
// (distributionRoundToSkip) lesen beide hierueber -- dieselbe Lesart auf
// beiden Seiten.
func verteilMarkenLesen(q zeileLeser) (runde, auszahlung int64, err error) {
	lies := func(key string) (int64, error) {
		var v string
		err := q.QueryRow(`SELECT value FROM chain_config WHERE key = $1`, key).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		if err != nil {
			return 0, fmt.Errorf("%s nicht lesbar: %w", key, err)
		}
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, nil
		}
		t, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s hat keinen gueltigen Zeitpunkt (%q)", key, v)
		}
		// Negativ = nie gelaufen (0), genau wie vorher beim Nachspielen
		// (distributionRoundToSkip verlangt > 0). Kein Fehler: ein negativer
		// Wert ist von der Kette selbst schreibbar (DistributionAt einer
		// ubi_distribution_finalize wird nicht begrenzt). Waere er ein
		// Fehler, stuende jeder Knoten vor dem naechsten Block mit einer
		// Ausschuettung (Sicherheitspruefung, Befund mittel). Unlesbar ist
		// nur, was die Kette nie schreibt (sie schreibt immer %d).
		if t < 0 {
			return 0, nil
		}
		return t, nil
	}
	if runde, err = lies("last_distribution_round_at"); err != nil {
		return 0, 0, err
	}
	if auszahlung, err = lies("last_ubi_at"); err != nil {
		return 0, 0, err
	}
	return runde, auszahlung, nil
}

// ErrVerteilAbstand: RunDailyDistributionAtomic hat die Runde abgelehnt, weil
// sie unter cs.mu den Mindestabstand nicht einhielt.
var ErrVerteilAbstand = errors.New("Mindestabstand zur letzten Runde nicht eingehalten")

// verteilAbstandInRundePruefen: dieselbe Pruefung wie im Planer, aber
// INNERHALB der Verteilungstransaktion und unter cs.mu (Sicherheitspruefung
// des Zeitplan-PRs, Befund mittel). Der Planer prueft ohne Sperre; zwischen
// seiner Pruefung und der Runde kann ein Peer-Block mit der Runde eines
// anderen Erzeugers nachgespielt werden (Leiterwechsel in derselben Minute).
// Ohne diese zweite Pruefung liefe die Runde trotzdem, und jeder andere
// Knoten verwuerfe sie (distributionRoundToSkip) -- Marken, Umlauf und
// Kontostaende liefen auseinander. Nachgespielt wird nur unter cs.mu, also
// sieht diese Lesung jede bis hierher angewandte Marke. Fail-closed:
// Lesefehler oder zu kurzer Abstand -> Fehler, die Runde wird ganz
// zurueckgerollt.
func (cs *ChainState) verteilAbstandInRundePruefen(ctx context.Context, ubiAt int64) error {
	if cs.db == nil {
		return nil // ohne Datenbank (Einzeltests): keine Marken
	}
	frei, err := verteilAbstandFreiLesen(cs.dbExecCtx(ctx))
	if err != nil {
		return fmt.Errorf("Mindestabstand nicht pruefbar: %w", err)
	}
	if !VerteilAbstandErlaubt(ubiAt, frei) {
		return fmt.Errorf("%w (Runde %d, frei ab %d)", ErrVerteilAbstand, ubiAt, frei)
	}
	return nil
}

// verteilAbstandNachlauf: so viel spaeter als 20:00 darf eine Runde wegen
// des Mindestabstands hoechstens laufen. Lief die vorige Runde Sekunden nach
// 20:00 (Planerlatenz), laeuft die naechste ebenso viele Sekunden nach 20:00
// -- der Versatz waechst nicht, er ist das Maximum der bisherigen Latenzen.
// Lief sie mehr als das spaeter (Retry wegen Sync, Sommerzeit-Umstellung mit
// 23-h-Tag), faellt der eine Termin aus und der Plan setzt wieder auf 20:00
// auf, statt die Verspaetung dauerhaft mitzunehmen.
const verteilAbstandNachlauf = 15 * time.Minute

// VerteilAbstandWiederholung: wann der Planer nach einer am Mindestabstand
// gescheiterten Runde erneut versucht -- genau zum freien Zeitpunkt, wenn der
// hoechstens verteilAbstandNachlauf nach dem aktuellen 20:00-Termin liegt;
// sonst ok=false und der Aufrufer nimmt den naechsten Termin.
func VerteilAbstandWiederholung(frei int64, jetzt time.Time, loc *time.Location) (time.Time, bool) {
	ziel := time.Unix(frei, 0)
	if ziel.After(verteilTerminZeit(jetzt.In(loc)).Add(verteilAbstandNachlauf)) {
		return time.Time{}, false
	}
	if !ziel.After(jetzt) {
		ziel = jetzt
	}
	return ziel, true
}
