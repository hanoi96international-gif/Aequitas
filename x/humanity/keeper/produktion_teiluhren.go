package keeper

import (
	"sync/atomic"
	"time"
)

// Teiluhren innerhalb der Blockphasen.
//
// Pruefstand Lauf 12 (02.10.2026, Speicherkorb an): Bloecke mit 7.000
// Ueberweisungen brauchen meist 100-300 ms, einzelne aber 0,5-1,5 s -- mal
// in "laden", mal in "db_paar", mal in "speichern". Postgres lief dabei auf
// 0,49 Kernen. Die Phasen sagen nur WO, nicht WORAN: Abfrage, Warten auf
// cs.mu (ein Schreiber hinter laufenden Lesern sperrt alle neuen Leser) oder
// Warten auf einen Pool-Arbeiter. Jede Uhr hier misst genau einen Schritt.
//
// Feste Liste, keine Schluessel von aussen; Kosten: zwei atomare Additionen
// und ein CAS je Messung.

type teilUhr struct {
	summe  atomic.Int64
	anzahl atomic.Int64
	max    atomic.Int64
}

func (u *teilUhr) seit(start time.Time) {
	d := int64(time.Since(start))
	u.summe.Add(d)
	u.anzahl.Add(1)
	for {
		alt := u.max.Load()
		if d <= alt || u.max.CompareAndSwap(alt, d) {
			return
		}
	}
}

func (u *teilUhr) stand() map[string]float64 {
	n := u.anzahl.Load()
	m := 0.0
	if n > 0 {
		m = float64(u.summe.Load()) / float64(n) / 1e6
	}
	return map[string]float64{"mittel_ms": m, "max_ms": float64(u.max.Load()) / 1e6, "anzahl": float64(n)}
}

var (
	tuStateRootDB      teilUhr // StateRoot: last_ubi_at aus Postgres
	tuStateRootSperre  teilUhr // StateRoot: Warten auf cs.mu.RLock
	tuStateRootHash    teilUhr // StateRoot: der Hash selbst
	tuLadenPoolWarten  teilUhr // Lader: von submit bis zum Start im Pool
	tuKorbOffeneZeilen teilUhr // Speicherkorb: ladeOffeneZeilen (Postgres)
	tuSpeichernArgs    teilUhr // Block-Zeile: JSON + Komprimierung
	tuSpeichernDB      teilUhr // Block-Zeile: Begin..Commit
	tuGebuehrSperre    teilUhr // Gebuehren ans Grundeinkommen: Warten auf cs.mu.Lock
	tuFlushRLock       teilUhr // WAL-Flush: Warten auf cs.mu.RLock
)

// ProduktionsTeiluhrenStand fuer /api/produktion.
func ProduktionsTeiluhrenStand() map[string]interface{} {
	return map[string]interface{}{
		"stateroot_db":       tuStateRootDB.stand(),
		"stateroot_sperre":   tuStateRootSperre.stand(),
		"stateroot_hash":     tuStateRootHash.stand(),
		"laden_pool_warten":  tuLadenPoolWarten.stand(),
		"korb_offene_zeilen": tuKorbOffeneZeilen.stand(),
		"speichern_args":     tuSpeichernArgs.stand(),
		"speichern_db":       tuSpeichernDB.stand(),
		"gebuehr_sperre":     tuGebuehrSperre.stand(),
		"flush_rlock":        tuFlushRLock.stand(),
	}
}
