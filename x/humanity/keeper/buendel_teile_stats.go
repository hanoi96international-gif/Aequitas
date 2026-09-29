package keeper

import (
	"sync/atomic"
	"time"
)

// Wohin gehen die Millisekunden von applyTransferBatchParallel?
//
// Gemessen am 29.09.2026 auf C2 (nach #222): ein voller Block mit 7.000
// Ueberweisungen hielt die globale Sperre 300-2.700 ms, im schlimmsten Block
// 2.172 ms allein in `parallel`. C1 baut denselben Block in ~300 ms. Solange
// C2 langsamer nachspielt als C1 baut, faellt C2 zurueck und die
// Peer-Lag-Bremse drueckt C1 auf 1.500 je Block -- das ist die Decke der
// Ketten-TPS. `parallel` bestand aus fuenf Schritten ohne eigene Uhr; hier
// bekommt jeder eine. Angegeben je 1.000 Ueberweisungen, weil leere Bloecke
// den Mittelwert je Block sonst verwaessern.
var (
	btLadenNanos      atomic.Int64 // Phase 1: Konten warm machen (DB bei kaltem Konto)
	btKaltGeladen     atomic.Int64 // davon Konten, die nicht im Speicher lagen
	btVorrechnenNanos atomic.Int64 // Phase 1b: Deckung und Wohlstandsgrenze in Reihenfolge
	btRechnenNanos    atomic.Int64 // Phase 2: Staende, Uhren, StateRoot-Blaetter (parallel)
	btBuchNanos       atomic.Int64 // Phase 2b: nachUeberweisung je Ueberweisung
	btBuchSchreiben   atomic.Int64 // Phase 2b: Buchkonten in die Transaktion
	btAbschlussNanos  atomic.Int64 // Phase 3: Konten sammeln oder schreiben
	btUeberweisungen  atomic.Int64
	btAufrufe         atomic.Int64
)

func merkeBuendelTeil(z *atomic.Int64, seit time.Time) time.Time {
	jetzt := time.Now()
	z.Add(int64(jetzt.Sub(seit)))
	return jetzt
}

// BuendelTeileStand fuer /api/health/combined.
func BuendelTeileStand() map[string]interface{} {
	n := btUeberweisungen.Load()
	je1000 := func(z *atomic.Int64) float64 {
		if n == 0 {
			return 0
		}
		return float64(z.Load()) / float64(n) * 1000 / 1e6
	}
	return map[string]interface{}{
		"aufrufe":                   btAufrufe.Load(),
		"ueberweisungen":            n,
		"kalt_geladen":              btKaltGeladen.Load(),
		"laden_je_1000_ms":          je1000(&btLadenNanos),
		"vorrechnen_je_1000_ms":     je1000(&btVorrechnenNanos),
		"rechnen_je_1000_ms":        je1000(&btRechnenNanos),
		"buch_je_1000_ms":           je1000(&btBuchNanos),
		"buch_schreiben_je_1000_ms": je1000(&btBuchSchreiben),
		"abschluss_je_1000_ms":      je1000(&btAbschlussNanos),
		"bedeutung": "Aufteilung von replay_phasen.parallel je 1.000 nachgespielter Ueberweisungen. laden: Konten warm machen " +
			"(kalt_geladen zaehlt die DB-Umlaeufe). vorrechnen: Deckung und Wohlstandsgrenze in Blockreihenfolge. rechnen: " +
			"Staende und StateRoot-Blaetter, parallel. buch: Buchfuehrung je Ueberweisung. abschluss: Konten sammeln.",
	}
}
