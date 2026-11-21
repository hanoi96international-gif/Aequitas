package keeper

// QUANTENGELD W UND S -- EINDEUTIGES URTEIL (DEZENTRAL.md 17, R10)
//
// BEFUND: Endet ein Token (eingeloest oder tot nach quantenWFehlMax falschen
// Vorlagen), legt die Kette ihn in beiden Faellen in Tote ab, und die
// oeffentliche Ansicht sagt fuer beide nur {"tot": true}. Das Urteil stand
// nur in der entscheidenden Transaktion (urteil "echt" mit wert_an der Stelle
// oder "abgewiesen" mit dem Wert ans Grundeinkommen). Die Werkzeuge
// schlossen deshalb aus der Zahl der Fehlversuche: nach 0 oder 1 kann ein
// verschwundener Token nur eingeloest sein -- beim letzten Versuch (2) nicht
// mehr, und dort meldeten sie gar nicht erst an. Ausserdem merkt sich die
// Kette einen Token nur, wenn er hoechstens quantenWToteBehalten nach der
// Ausgabe endet; spaeter verschwand er spurlos (dieselbe Ansicht wie "nie
// ausgegeben").
//
// AB DEM STICHTAG schreibt jeder Knoten beim Ende selbst, aus dem Block, den
// er nachrechnet: Urteil ("eingeloest" oder "tot") und Zeit des Endes, im
// Eintrag unter Tote -- immer, auch nach langer Laufzeit. Aufgeraeumt wird
// er quantenWToteBehalten nach dem spaeteren von Ausgabe und Ende. Vor dem
// Stichtag bleibt alles byte-gleich (die Felder fehlen, die StateRoot ist
// dieselbe). Konsensaenderung: ein aelterer Knoten rechnete nach dem
// Stichtag eine andere StateRoot -- darum der Stichtag und der Deploy-Waechter
// .github/neustart/quanten-urteil-nicht-verpasst.sh.

import (
	"math"
	"sync/atomic"
)

// STICHTAG DES EINDEUTIGEN URTEILS (Kettenregel). Als Zahl geschrieben, der
// Waechter liest sie; 9223372036854775807 = math.MaxInt64 = schlafend. Den
// Tag setzt der Betreiber, wenn alle Validatoren diese Fassung haben.
var (
	quantenWUrteilAbUnix int64 = 9223372036854775807
)

// quantenWUrteilAbOverride: nur fuer Tests (0 = Stichtag oben).
var quantenWUrteilAbOverride atomic.Int64

func quantenWUrteilAb() int64 {
	if v := quantenWUrteilAbOverride.Load(); v != 0 {
		return v
	}
	return quantenWUrteilAbUnix
}

func quantenWUrteilAktiv(t int64) bool { return t >= quantenWUrteilAb() }

const (
	quantenWUrteilEingeloest = "eingeloest"
	quantenWUrteilTot        = "tot"
)

// quantenWEnde: Urteil und Zeit des Endes eines Tokens oder einer Note.
type quantenWEnde struct {
	Ende   int64
	Urteil string
}

// toteMerken: K endet im Block mit blockZeit (zeit = Praege- bzw.
// Ausgabezeit). Vor dem Stichtag wie bisher: nur gemerkt, wenn die Ausgabe
// hoechstens quantenWToteBehalten zurueckliegt, ohne Urteil.
func (r *quantenWRegister) toteMerken(k string, zeit, blockZeit int64, urteil string) {
	if quantenWUrteilAktiv(blockZeit) {
		r.Tote[k] = zeit
		r.Enden[k] = quantenWEnde{Ende: blockZeit, Urteil: urteil}
		return
	}
	if zeit >= blockZeit-quantenWToteBehalten {
		r.Tote[k] = zeit
	}
}

// quantenWUrteilGueltig: was im Register stehen darf (Lesen, fail-closed).
func quantenWUrteilGueltig(ende int64, urteil string) bool {
	switch urteil {
	case "":
		return ende == 0
	case quantenWUrteilEingeloest, quantenWUrteilTot:
		return ende > 0 && ende < math.MaxInt64
	}
	return false
}

// quantenWUrteilStatus: fuer /api/status (fehlt = schlafend).
func quantenWUrteilStatus(m map[string]interface{}) {
	if ab := quantenWUrteilAb(); ab != math.MaxInt64 {
		m["urteil_ab"] = ab
	}
}
