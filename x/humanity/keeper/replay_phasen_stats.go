package keeper

import (
	"sync/atomic"
	"time"
)

// Wohin gehen die 447 ms, die das Nachspielen die globale Sperre haelt?
//
// # WARUM DIESE ZAHL JETZT ALLES ENTSCHEIDET
//
// Gemessen am 05.09.2026, beide Boxen gleichzeitig unter Last:
//
//	                     C1 (Primary)   C2 (nimmt die Last an)
//	exclusive_busy_pct        53,00 %                 12,33 %
//	exclusive_avg_ms              447                    154
//	exclusive_max_ms           19.043                 11.458
//	replay seriell             87.293                 11.142
//
// Der Primary ist ueber die HAELFTE der Zeit vollstaendig eingefroren, mit
// Einzelhalten bis 19 Sekunden. In diesem Fenster laeuft nichts: keine
// Ueberweisung, keine Blockproduktion, keine Auskunft. Das ist zugleich
//
//   - die Durchsatzdecke: Go's RWMutex sperrt ankommende Leser aus, sobald
//     ein Schreiber ANSTEHT. Der Schnellpfad wartet dadurch 43,7 ms von
//     76,9 ms je Ueberweisung auf cs.mu.RLock(), und der WAL-Gruppencommit
//     zerfaellt in einen Saegezahn -- max_batch 500 wird erreicht, avg_batch
//     ist 39,7. Daraus 39,7/8,0 ms ~ 4.963 Saetze/s, was die gemessenen
//     2.400-3.900 TPS genau einrahmt.
//
//   - das Absturzrisiko: 485 Ueberweisungen je Block kosten C1 447 ms, also
//     922 us je Ueberweisung. Bei 10k TPS waere die Haltezeit groesser als
//     die Blockzeit -- der Primary fiele unaufholbar zurueck, statt langsamer
//     nur noch gar nicht mehr zu antworten.
//
// # WARUM RATEN HIER SCHON DREIMAL FALSCH WAR
//
// Fuer diesen Engpass sind bereits gescheitert: paralleles Replay (ist
// bereits zu 89-93 % parallel), Flush-Batchgroesse, Flush-Intervall,
// Postgres-Checkpoints, GC, WAL-Dateigroesse, Sperr-Kontention und das
// Verlaengern des Gruppencommit-Fensters. Was jedes Mal geholfen hat, war
// eine Phasenaufteilung, die sich zum Ganzen aufsummiert -- dann ist die
// Antwort eine Subtraktion und keine Hypothese.
//
// Genau das ist hier gebaut: die benannten Phasen plus `rest` ergeben die
// gemessene Haltezeit. Ein grosses `rest` ist die wichtigste Auskunft, die
// dieses Instrument geben kann -- es hiesse, die Kosten liegen in Code, den
// keine der Phasen abdeckt.
//
// Kosten: sechs atomare Additionen je Block, keine je Ueberweisung.
var (
	rpSnapshotNanos  atomic.Int64 // snapshotForRollbackLocked
	rpParallelNanos  atomic.Int64 // applyTransferBatchParallel
	rpSeriellNanos   atomic.Int64 // applyTransferDeltaLocked, eine Ueberweisung
	rpStateRootNanos atomic.Int64 // stateRootLocked + Vergleich
	rpCommitNanos    atomic.Int64 // commitOrRollback -- die DB-Transaktion
	rpHaltNanos      atomic.Int64 // die ganze Haltezeit, als Bezugsgroesse
	rpBloecke        atomic.Int64
	rpSeriellAufrufe atomic.Int64
)

func merkeReplayPhase(z *atomic.Int64, seit time.Time) {
	z.Add(int64(time.Since(seit)))
}

// merkeReplaySeriellZeit zaehlt die Aufrufe mit, weil der serielle Pfad der
// einzige ist, dessen Kosten JE UEBERWEISUNG anfallen -- bei ihm ist der
// Mittelwert je Aufruf die Zahl, die ueber den naechsten Schritt entscheidet.
func merkeReplaySeriellZeit(seit time.Time) {
	rpSeriellNanos.Add(int64(time.Since(seit)))
	rpSeriellAufrufe.Add(1)
}

func merkeReplayBlock(halt time.Duration) {
	rpHaltNanos.Add(int64(halt))
	rpBloecke.Add(1)
}

// ReplayPhasenStand zeigt die Aufteilung in /api/health/combined.
func ReplayPhasenStand() map[string]interface{} {
	n := rpBloecke.Load()
	msJe := func(z *atomic.Int64) float64 {
		if n == 0 {
			return 0
		}
		return float64(z.Load()) / float64(n) / 1e6
	}
	halt := msJe(&rpHaltNanos)
	benannt := msJe(&rpSnapshotNanos) + msJe(&rpParallelNanos) + msJe(&rpSeriellNanos) +
		msJe(&rpStateRootNanos) + msJe(&rpCommitNanos)
	seriellJeAufruf := float64(0)
	if a := rpSeriellAufrufe.Load(); a > 0 {
		seriellJeAufruf = float64(rpSeriellNanos.Load()) / float64(a) / 1e6
	}
	anteil := func(ms float64) float64 {
		if halt <= 0 {
			return 0
		}
		return ms / halt * 100
	}
	return map[string]interface{}{
		"bloecke":              n,
		"halt_ms":              halt,
		"snapshot_ms":          msJe(&rpSnapshotNanos),
		"parallel_ms":          msJe(&rpParallelNanos),
		"seriell_ms":           msJe(&rpSeriellNanos),
		"seriell_je_aufruf_ms": seriellJeAufruf,
		"seriell_aufrufe":      rpSeriellAufrufe.Load(),
		"stateroot_ms":         msJe(&rpStateRootNanos),
		"commit_ms":            msJe(&rpCommitNanos),
		"rest_ms":              halt - benannt,
		"rest_anteil_pct":      anteil(halt - benannt),
		"seriell_anteil_pct":   anteil(msJe(&rpSeriellNanos)),
		"stateroot_anteil_pct": anteil(msJe(&rpStateRootNanos)),
		"bedeutung": "Aufteilung der globalen Sperre beim Nachspielen. Die benannten Phasen plus rest ergeben halt_ms. " +
			"Ein grosses rest heisst: die Kosten liegen ausserhalb der gemessenen Phasen. " +
			"seriell_je_aufruf_ms ist die einzige Zahl, die JE UEBERWEISUNG anfaellt -- steht sie im Millisekundenbereich, " +
			"ist es ein Datenbank-Umlauf unter der globalen Sperre, und das ist der Hebel.",
	}
}

// ReplayPhasenZuruecksetzen macht die Zaehler zwischen zwei Messlaeufen
// vergleichbar. Nur fuer Tests und den Betriebs-Endpunkt.
func ReplayPhasenZuruecksetzen() {
	for _, z := range []*atomic.Int64{
		&rpSnapshotNanos, &rpParallelNanos, &rpSeriellNanos,
		&rpStateRootNanos, &rpCommitNanos, &rpHaltNanos,
		&rpBloecke, &rpSeriellAufrufe,
	} {
		z.Store(0)
	}
}
