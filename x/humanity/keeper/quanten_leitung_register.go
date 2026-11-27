package keeper

// WEG C, Schritt C2a (forschung/quantengeld/WEG_C.md): das Register der
// Quantenleitungen. Entscheidung des Betreibers (27.11.2026): die Messwerte
// eines Geraetepaars (Sender der Praeparatorinnen, Empfaenger der Inhaberin)
// traegt man mit zwei Dritteln der Pruefer ein, mindestens
// quantenFreischaltungMindestPruefer -- dieselbe Abstimmung wie die
// Freischaltung eines Quantengeld-Satzes (quanten_geld.go). Kein Einzelner
// kann die Werte druecken: zu niedrige Werte machten jedem Faelscher den Weg
// leichter (C1, quanten_s_leitung.go).
//
// # FORM
//
// Eine Freischaltung (quanten_freischaltung) mit einem Namen "leitung-..." und
// OHNE Gruppenordnung, dafuer mit
//   - theta: gemessene Verkippung der Basen in Zehntelgrad (0..100),
//   - frei:  Anteil der Mehrphotonen-Pulse unter den gemeldeten, ppm
//            (0..quantenSFreiPpmMax).
// Die Stimmen unterschreiben eine EIGENE Nachricht (Domaene
// aequitas-quanten-leitung-v1): eine Stimme fuer einen Satz gilt nie fuer eine
// Leitung und umgekehrt. Vorlauf, Fristen, Quorum, PQ-Teil: wie beim Satz.
//
// # REGISTER
//
// Eigene Liste im Quantengeld-Register (Leitungen), nie unter den Saetzen: eine
// Note darf eine Leitung nie als Satz nennen. Leer bleibt alles byte-gleich
// (Kodierung, StateRoot, chain_config).
//
// # VOR DEM STICHTAG
//
// Vor quantenLeitungAbUnix ist eine solche Freischaltung so ungueltig wie
// bisher (Gruppenordnung fehlt); nichts aendert sich. Nach dem Stichtag darf
// kein SATZ mehr "leitung-..." heissen.
//
// # NOCH NICHT
//
// Ein Token, der eine Leitung nennt und mit ihrer Pruefgroesse rechnet, kommt
// mit C2b. Bis dahin ist eine eingetragene Leitung nur ein Eintrag.

import (
	"fmt"
	"strings"
	"sync/atomic"
)

const (
	quantenLeitungPraefix      = "leitung-"
	quantenDomaeneLeitung      = "aequitas-quanten-leitung-v1"
	quantenLeitungenHoechstens = 8
)

// quantenLeitungAbUnix: Stichtag der Form (Kettenregel).
// 9223372036854775807 = math.MaxInt64 = schlafend. Der Deploy-Waechter .github/neustart/quanten-leitung-nicht-verpasst.sh
// haelt jeden Wechsel an, der nicht mindestens 30 Minuten in der Zukunft liegt
// oder einen laufenden Stichtag aendern wuerde.
var quantenLeitungAbUnix int64 = 9223372036854775807

// quantenLeitungAbOverride: nur fuer Tests (0 = Stichtag oben).
var quantenLeitungAbOverride atomic.Int64

func quantenLeitungAb() int64 {
	if v := quantenLeitungAbOverride.Load(); v != 0 {
		return v
	}
	return quantenLeitungAbUnix
}

func quantenLeitungAktiv(t int64) bool { return quantenGeldAktiv(t) && t >= quantenLeitungAb() }

// quantenLeitungEintrag: die gemessenen Werte eines Geraetepaars.
type quantenLeitungEintrag struct {
	Name         string `json:"name"`
	ThetaZehntel int    `json:"theta"`
	FreiPpm      int    `json:"frei"`
	Ab           int64  `json:"ab"`
	Seit         int64  `json:"seit"` // issued_at der Freischaltung
}

func istLeitungsName(name string) bool { return strings.HasPrefix(name, quantenLeitungPraefix) }

// quantenLeitungWerteGueltig: im Bereich, den die Pruefgroesse kennt (C1).
func quantenLeitungWerteGueltig(theta, frei int) bool {
	return theta >= 0 && theta <= quantenSThetaZehntelMax && frei >= 0 && frei <= quantenSFreiPpmMax
}

// quantenLeitungNachricht: was ein Pruefer fuer genau diese Werte unterschreibt.
func quantenLeitungNachricht(name string, theta, frei int, ab int64) func(wallet string, stimmzeit int64) string {
	return func(_ string, stimmzeit int64) string {
		return fmt.Sprintf("%s|chain:%d|%s|theta:%d|frei:%d|%d|%d", quantenDomaeneLeitung, eingefroreneChainID, name, theta, frei, ab, stimmzeit)
	}
}

func (r *quantenGeldRegister) leitung(name string) (quantenLeitungEintrag, bool) {
	for _, l := range r.Leitungen {
		if l.Name == name {
			return l, true
		}
	}
	return quantenLeitungEintrag{}, false
}

// leitungZu: die Werte einer Leitung, die zur Zeit t gilt (fuer C2b).
func (r *quantenGeldRegister) leitungZu(name string, t int64) (theta, frei int, ok bool) {
	l, da := r.leitung(name)
	if !da || t < l.Ab {
		return 0, 0, false
	}
	return l.ThetaZehntel, l.FreiPpm, true
}

func (r *quantenGeldRegister) mitLeitung(l quantenLeitungEintrag) *quantenGeldRegister {
	k := r.kopie()
	if _, da := k.leitung(l.Name); da {
		return k
	}
	k.Leitungen = append(k.Leitungen, l)
	for i := len(k.Leitungen) - 1; i > 0 && k.Leitungen[i].Name < k.Leitungen[i-1].Name; i-- {
		k.Leitungen[i], k.Leitungen[i-1] = k.Leitungen[i-1], k.Leitungen[i]
	}
	return k
}

// quantenLeitungenPruefen: Teil von quantenGeldRegister.pruefen (streng, auch
// fuer ein Register von aussen).
func (r *quantenGeldRegister) quantenLeitungenPruefen() error {
	if len(r.Leitungen) > quantenLeitungenHoechstens {
		return fmt.Errorf("Quantengeld-Register: %d Leitungen, hoechstens %d", len(r.Leitungen), quantenLeitungenHoechstens)
	}
	for i, l := range r.Leitungen {
		if !istLeitungsName(l.Name) || !quantenSatzGueltig(l.Name) || !quantenLeitungWerteGueltig(l.ThetaZehntel, l.FreiPpm) ||
			l.Ab <= 0 || l.Seit <= 0 || (i > 0 && l.Name <= r.Leitungen[i-1].Name) {
			return fmt.Errorf("Quantengeld-Register: Leitung %d ungueltig", i)
		}
		if _, da := r.satz(l.Name); da {
			return fmt.Errorf("Quantengeld-Register: Leitung %d heisst wie ein Satz", i)
		}
	}
	return nil
}

// quantenLeitungStand: fuer /api/quanten/stand.
func (r *quantenGeldRegister) quantenLeitungStand() []map[string]interface{} {
	aus := make([]map[string]interface{}, 0, len(r.Leitungen))
	for _, l := range r.Leitungen {
		aus = append(aus, map[string]interface{}{"name": l.Name, "theta_zehntel": l.ThetaZehntel, "frei_ppm": l.FreiPpm, "ab": l.Ab, "beschlossen": l.Seit})
	}
	return aus
}
