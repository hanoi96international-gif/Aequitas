package keeper

import (
	"sort"
	"sync"
	"sync/atomic"
)

// WARUM FIEL DIESER TICK AUS.
//
// ProduceBlock hat elf Stellen, an denen es nil zurueckgibt, und der Aufrufer
// in main.go behandelt alle gleich:
//
//	if len(blocks) == 0 {
//	    return // catch-up gate — skip this tick
//	}
//
// Jede Stelle schreibt zwar eine Meldung ins Log, aber gezaehlt wird nichts.
// Am 11.09.2026 kostete das eine Messung: unter Last produzierte jeder
// Validator nur jede zweite Sekunde -- 315 Bloecke in 313 Sekunden bei zwei
// Validatoren und einem Takt von 1000 ms, wo 626 haetten entstehen muessen.
// ProduceBlock selbst brauchte dabei nur 500 bis 740 ms, passte also bequem in
// den Takt. Die Haelfte der Ticks fiel aus einem der elf Gruende aus, und aus
// dem Log war nicht abzulesen, aus welchem: unter Last laufen tausende Zeilen
// durch, und ein "skipping block production" geht darin unter.
//
// Dieselbe Lehre wie bei replay_pfad_stats.go: solange nicht gezaehlt wird,
// welcher Zweig greift, bleibt jede Erklaerung eine Vermutung. Ein Zaehler je
// Grund kostet nichts und beantwortet die Frage in einem Blick.
//
// ES WIRD NUR GEZAEHLT, NICHTS ENTSCHIEDEN. Kein Zweig aendert sein Verhalten,
// weil hier ein Zaehler steht. Die Gruende bleiben, wo sie sind -- sie werden
// nur sichtbar.

// AusfallGrund ist ein Grund samt Haeufigkeit. Ein benannter Typ auf
// Paketebene, damit ein Test die Liste pruefen kann, ohne den anonymen
// Strukturtyp Feld fuer Feld nachbauen zu muessen.
type AusfallGrund struct {
	Grund string `json:"grund"`
	Ticks int64  `json:"ticks"`
}

var (
	produktionAusfaelle   sync.Map // grund -> *atomic.Int64
	produktionAusfallSum  atomic.Int64
	produktionVersuche    atomic.Int64
	produktionErfolge     atomic.Int64
	produktionLetzterGrnd atomic.Value // string
)

// merkeProduktionsAusfall haelt fest, warum ein Tick keinen Block ergab.
// Der Grund ist ein kurzer, fester Schluessel -- keine formatierte Meldung,
// sonst entstuenden so viele Schluessel wie Ticks.
func merkeProduktionsAusfall(grund string) {
	produktionAusfallSum.Add(1)
	produktionLetzterGrnd.Store(grund)
	z, _ := produktionAusfaelle.LoadOrStore(grund, new(atomic.Int64))
	z.(*atomic.Int64).Add(1)
}

func merkeProduktionsVersuch()       { produktionVersuche.Add(1) }
func merkeProduktionsErfolg()        { produktionErfolge.Add(1) }
func produktionAusfallGesamt() int64 { return produktionAusfallSum.Load() }

// ProduktionsAusfaelle meldet, wie oft welcher Grund einen Tick gekostet hat.
func ProduktionsAusfaelle() map[string]interface{} {
	art := map[string]int64{}
	produktionAusfaelle.Range(func(k, v interface{}) bool {
		art[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	// Nach Haeufigkeit sortiert, damit der teuerste Grund oben steht und nicht
	// gesucht werden muss.
	liste := make([]AusfallGrund, 0, len(art))
	for g, n := range art {
		liste = append(liste, AusfallGrund{g, n})
	}
	sort.Slice(liste, func(i, j int) bool { return liste[i].Ticks > liste[j].Ticks })

	versuche := produktionVersuche.Load()
	erfolge := produktionErfolge.Load()
	var quote float64
	if versuche > 0 {
		quote = float64(erfolge) / float64(versuche) * 100
	}
	letzter, _ := produktionLetzterGrnd.Load().(string)

	return map[string]interface{}{
		"bedeutung": "ProduceBlock gibt an elf Stellen nil zurueck und der Aufrufer behandelt alle " +
			"gleich. Ohne diese Zaehler ist nicht zu sagen, WARUM ein Tick keinen Block ergab -- " +
			"gemessen am 11.09.2026: unter Last fiel jeder zweite Tick aus, obwohl ProduceBlock " +
			"selbst bequem in den Takt passte.",
		"versuche":      versuche,
		"erfolge":       erfolge,
		"erfolgsanteil": quote,
		"ausfaelle":     produktionAusfallSum.Load(),
		"letzter_grund": letzter,
		"nach_grund":    liste,
	}
}

// ProduktionsAusfaelleZuruecksetzen ist fuer Tests.
func ProduktionsAusfaelleZuruecksetzen() {
	produktionAusfaelle.Range(func(k, _ interface{}) bool {
		produktionAusfaelle.Delete(k)
		return true
	})
	produktionAusfallSum.Store(0)
	produktionVersuche.Store(0)
	produktionErfolge.Store(0)
	produktionLetzterGrnd.Store("")
}
