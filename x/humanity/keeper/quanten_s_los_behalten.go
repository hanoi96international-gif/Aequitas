package keeper

// LOS JE TOKEN BEHALTEN (P2a, Plan Pfaender in forschung/quantengeld/DEZENTRAL.md)
//
// Bisher vergisst die Kette nach der Ausgabe eines Weg-B-Tokens, WER gelost
// war: die Bestellung (mit dem Los) wird bei der Ausgabe geloescht, der Token
// selbst traegt nur die Zahl v (NVal). Der Faelschungsnachweis (P4) muss aber
// Tage nach dem Ende eines Tokens noch beweisen koennen, dass eine bestimmte
// Praeparatorin im Los dieses Tokens stand -- sonst trifft ihr Pfand (P2)
// niemanden. Darum ab dem Stichtag:
//
//   - bei der Ausgabe eines bestellten K: Lose[K] = das Los, in Losreihenfolge
//     (dieselbe Reihenfolge wie die Validatoren der Ausgabe; quantenSAusgabeGegenLos
//     hat die Gleichheit geprueft),
//   - der Eintrag bleibt, solange K als Token lebt oder als tot gemerkt ist
//     (Tote, mit Urteil und Zeit des Endes); quantenWToteAufraeumen loescht
//     ihn zusammen mit dem toten Eintrag. Tote bleiben quantenWToteBehalten
//     (gut 8 Tage) ab dem Ende stehen -- laenger als die Nachweis-Frist von
//     7 Tagen (quantenSNachweisFrist), die P4 selbst prueft.
//   - begrenzt: ein Eintrag je lebendem oder totem K (Token und Tote sind
//     begrenzt), je Eintrag hoechstens quantenSLosVMax Konten.
//
// Vor dem Stichtag bleibt Lose leer; das Register kodiert sich byte-gleich
// (Feld "lose" fehlt, keine Zeile der Art "los"), die StateRoot ist dieselbe.
// Gelesen wird fail-closed (quantenSLoseLesen).

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"sync/atomic"
)

const (
	// Zeile in quantenWTabelle (quanten_w_tabelle.go).
	quantenWArtLos = "los"
	// Hoechstens so viele Eintraege: je lebendem Token und je totem K einer
	// (wie in quantenWRegisterLesen: Tote bis zur Grenze plus die, die
	// danach noch sterben).
	quantenSLoseHoechstens = quantenWTokenHoechstens + quantenWToteHoechstens + quantenWNotenHoechstens + quantenWTokenHoechstens
	// Nachweis-Frist nach dem Ende eines Tokens (P4, Entscheidung des
	// Betreibers: 7 Tage). Das Register behaelt Tote laenger (s. o.).
	quantenSNachweisFrist int64 = 7 * 86400
)

// STICHTAG "LOS BEHALTEN" (Kettenregel). 9223372036854775807 = math.MaxInt64
// = schlafend. Gesetzt vom Betreiber am 26.11.2026 ("Stichtag morgen frueh",
// 08:00 MEZ): 27.11.2026 07:00 UTC. Nur P2a -- die Pfaender (P2-P4) schlafen
// weiter hinter quantenSPfaenderAbUnix, bis die App sie zeigt. Nie
// rueckwirkend: der Deploy-Waechter .github/neustart/quanten-s-los-nicht-verpasst.sh
// haelt jeden Wechsel an, der nicht mindestens 30 Minuten in der Zukunft
// liegt oder einen laufenden Stichtag aendern wuerde.
var quantenSLosBehaltenAbUnix int64 = 1795762800

// quantenSLosBehaltenAbOverride: nur fuer Tests (0 = Stichtag oben).
var quantenSLosBehaltenAbOverride atomic.Int64

func quantenSLosBehaltenAb() int64 {
	if v := quantenSLosBehaltenAbOverride.Load(); v != 0 {
		return v
	}
	return quantenSLosBehaltenAbUnix
}

// quantenSLosBehaltenAktiv: setzt Weg B voraus (ohne Weg B gibt es kein Los).
func quantenSLosBehaltenAktiv(t int64) bool {
	return quantenSWegBAktiv(t) && t >= quantenSLosBehaltenAb()
}

// quantenSLosBehaltenStatus: fuer /api/status (fehlt = schlafend).
func quantenSLosBehaltenStatus(m map[string]interface{}) {
	if ab := quantenSLosBehaltenAb(); ab != math.MaxInt64 {
		m["los_ab"] = ab
	}
}

// quantenSLosForm: ein Eintrag in kanonischer Form.
type quantenSLosForm struct {
	K   string   `json:"k"`
	Los []string `json:"los"`
}

// quantenSLoseFormen: sortiert nach K (jeder Knoten schreibt dieselben Bytes).
func quantenSLoseFormen(m map[string][]string) []quantenSLosForm {
	if len(m) == 0 {
		return nil
	}
	out := make([]quantenSLosForm, 0, len(m))
	for k, los := range m {
		out = append(out, quantenSLosForm{K: k, Los: los})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out
}

// quantenSLosGueltig: ein Los, wie es aus quantenSLos entstehen kann --
// v in quantenSLosVMin..quantenSLosVMax, kanonische, verschiedene Konten,
// keine Systemadresse.
func quantenSLosGueltig(los []string) bool {
	if len(los) < quantenSLosVMin || len(los) > quantenSLosVMax {
		return false
	}
	gesehen := make(map[string]bool, len(los))
	for _, a := range los {
		if !kanonischeAdresse(a) || istSystemAdresse(a) || gesehen[a] {
			return false
		}
		gesehen[a] = true
	}
	return true
}

// quantenSLoseLesen: fail-closed. r traegt schon Token und Tote; jeder
// Eintrag gehoert zu genau einem lebenden Token (dann passt v) oder einem
// toten K -- ein Eintrag ohne beides konnte nie entstehen.
func quantenSLoseLesen(fs []quantenSLosForm, r *quantenWRegister) (map[string][]string, error) {
	out := make(map[string][]string, len(fs))
	if len(fs) > quantenSLoseHoechstens {
		return nil, fmt.Errorf("quanten_w: %d Lose, hoechstens %d", len(fs), quantenSLoseHoechstens)
	}
	for i, f := range fs {
		if !istHash64(f.K) || (i > 0 && f.K <= fs[i-1].K) {
			return nil, fmt.Errorf("quanten_w: Los %d: K oder Ordnung", i)
		}
		if !quantenSLosGueltig(f.Los) {
			return nil, fmt.Errorf("quanten_w: Los %s: Form", f.K[:12])
		}
		if e := r.Token[f.K]; e != nil {
			if e.token.NVal != len(f.Los) {
				return nil, fmt.Errorf("quanten_w: Los %s: v passt nicht zum Token", f.K[:12])
			}
		} else if _, tot := r.Tote[f.K]; !tot && r.Nachweis[f.K] == nil {
			return nil, fmt.Errorf("quanten_w: Los %s ohne Token", f.K[:12])
		}
		out[f.K] = slices.Clone(f.Los)
	}
	return out, nil
}

// quantenSLosAnsicht: das Los in einer Ansicht (/api/quanten-w/note), wenn
// die Kette es behalten hat.
func quantenSLosAnsicht(reg *quantenWRegister, k string, v map[string]interface{}) {
	if los, ok := reg.Lose[k]; ok {
		v["los"] = slices.Clone(los)
	}
}
