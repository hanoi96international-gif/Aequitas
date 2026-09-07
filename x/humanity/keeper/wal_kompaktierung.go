package keeper

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// Das WAL kleinhalten.
//
// WARUM ES DAS GIBT -- transfer_wal.go sagt es seit dem ersten Tag selbst:
//
//	"Compaction (wal.TruncateBefore is never called here). The WAL file grows
//	 without bound for the lifetime of the process. Fine for testing; NOT fine
//	 for any real deployment -- this is the single largest gap before this
//	 could ever be considered for staging, let alone production."
//
// Am 07.09.2026 ist genau das eingetreten: aequitas_transfers.wal stand bei
// 17 GB, die Platte von C2 bei 99 % (666 MB frei), und der Plattenwaechter
// lehnte deshalb Ueberweisungen ab -- der Knoten lief, antwortete und nahm
// nichts mehr an. wal.TruncateBefore war die ganze Zeit fertig implementiert
// und absturzsicher; es rief sie nur niemand.
//
// WANN KUERZEN SICHER IST. Ein Record darf verschwinden, sobald seine Wirkung
// in Postgres steht. Der Flusher arbeitet eine Warteschlange ab; ist sie
// leer, ist alles Eingereihte geschrieben. Ein Record wird aber ERST
// geschrieben und DANN eingereiht -- zwischen beidem liegt ein Fenster, in
// dem er in keiner Warteschlange steht und trotzdem noch gebraucht wird.
//
// Deshalb wird gerechnet: Stehen N Elemente in der Warteschlange und ist der
// Kopf bei S, koennen hoechstens die Records S-N+1 .. S noch offen sein --
// alles davor ist in Postgres. Ein grosszuegiger Sicherheitsabstand deckt das
// Fenster zwischen Schreiben und Einreihen.
//
// Zusaetzlich wird der Wiederanlauf-Boden beruecksichtigt: unterhalb von
// walRecoveryFloor darf ohnehin nie wieder abgespielt werden (siehe
// markWALSupersededByStateReplacement), diese Records sind also in jedem
// Fall entbehrlich.
const (
	// Wie oft geprueft wird. Die Kompaktierung schreibt die Datei neu, ist
	// also teuer -- sie soll selten laufen, nicht bei jedem Flush-Tick.
	walKompaktIntervall = 5 * time.Minute

	// Erst ab dieser Groesse ueberhaupt kuerzen. Eine kleine Datei neu zu
	// schreiben kostet mehr, als sie spart.
	walKompaktAbBytes = 512 << 20 // 512 MB
)

var (
	walKompaktLaeufe    atomic.Int64
	walKompaktBytesFrei atomic.Int64
	walKompaktLetzteSeq atomic.Uint64
	walKompaktFehler    atomic.Int64
	walKompaktLetzteMs  atomic.Int64
)

// starteWALKompaktierung laeuft neben dem Flush-Worker und haelt die Datei
// klein. Tut nichts, wenn kein WAL aktiv ist.
func (cs *ChainState) starteWALKompaktierung() {
	if cs.wal == nil {
		return
	}
	// Der Flush-Worker legt walFlushStopCh an. Ohne ihn waere der Kanal hier
	// nil, das select liefe zwar (der Ticker-Zweig wird gewaehlt), die
	// Goroutine liesse sich aber nie beenden. Der Worker wird ohnehin beim
	// ersten Transfer gebraucht.
	cs.ensureWALFlushWorkerStarted()
	SafeGoroutine("walKompaktierung", func() {
		ticker := time.NewTicker(walKompaktIntervall)
		defer ticker.Stop()
		var kandidat uint64 // in der Vorrunde gemerkte Kopfnummer
		for {
			select {
			case <-ticker.C:
				kandidat = cs.walKompaktierungsRunde(kandidat)
			case <-cs.walFlushStopCh:
				return
			}
		}
	})
}

// walKompaktierungsRunde fuehrt eine Runde aus und gibt die Kopfnummer
// zurueck, die in der naechsten Runde als Untergrenze dient (0 = keine).
// Ausgelagert, damit die Regel testbar ist, ohne fuenf Minuten zu warten.
//
// WARUM NICHT "WARTESCHLANGE LEER". Der erste Anlauf am 07.09.2026 kuerzte
// nur, wenn die Flush-Warteschlange zwei Runden in Folge leer war. Live
// gemessen lief er NIE: unter Dauerlast ist die Warteschlange nie leer, also
// blieb laeufe=0 und die Datei bei 17 GB -- die Bedingung war zwar sicher,
// aber genau dann wirkungslos, wenn sie gebraucht wird.
//
// Jetzt wird gerechnet statt gewartet. Jedes Element der Warteschlange gehoert
// zu genau einem Record, und eingereiht wird in derselben Reihenfolge, in der
// geschrieben wird. Stehen also N Elemente aus und ist der Kopf bei S, dann
// koennen hoechstens die Records S-N+1 .. S noch offen sein; alles davor ist
// nachweislich in Postgres. Ein grosszuegiger Sicherheitsabstand kommt hinzu,
// weil zwischen dem Schreiben eines Records und seinem Einreihen ein kurzes
// Fenster liegt, in dem er in keiner Warteschlange steht.
func (cs *ChainState) walKompaktierungsRunde(_ uint64) uint64 {
	if cs.wal == nil {
		return 0
	}
	kopf := cs.wal.HeadSeq()
	offen := uint64(cs.WALFlushQueueDepth())
	// Der Abstand deckt das Schreib-/Einreih-Fenster und jede Unschaerfe in
	// der Tiefenmessung. Bei rund 5.000 Ueberweisungen je Sekunde entspricht
	// er gut einer Minute Vorlauf -- weit mehr, als ein Flush je braucht.
	const abstand = uint64(300000)
	if kopf <= offen+abstand {
		return kopf // noch zu jung zum Kuerzen
	}
	bis := kopf - offen - abstand
	if boden := cs.walRecoveryFloor(); boden > bis {
		// Unterhalb des Wiederanlauf-Bodens darf ohnehin nie wieder abgespielt
		// werden, dort ist Kuerzen immer erlaubt.
		bis = boden
	}
	vorher := walDateiGroesse(cs.wal.Path())
	if vorher < walKompaktAbBytes {
		return kopf
	}
	start := time.Now()
	if err := cs.wal.TruncateBefore(bis); err != nil {
		walKompaktFehler.Add(1)
		fmt.Printf("[WAL] ⚠ Kompaktierung bis Seq %d fehlgeschlagen: %v — die Datei bleibt, wie sie war\n", bis, err)
		return kopf
	}
	dauer := time.Since(start)
	nachher := walDateiGroesse(cs.wal.Path())
	walKompaktLaeufe.Add(1)
	walKompaktLetzteSeq.Store(bis)
	walKompaktLetzteMs.Store(dauer.Milliseconds())
	if vorher > nachher {
		walKompaktBytesFrei.Add(vorher - nachher)
	}
	fmt.Printf("[WAL] ✓ Kompaktiert bis Seq %d in %s: %d MB → %d MB (Kopf %d, %d offen)\n",
		bis, dauer.Round(time.Millisecond), vorher>>20, nachher>>20, kopf, offen)
	return kopf
}

func walDateiGroesse(pfad string) int64 {
	if pfad == "" {
		return 0
	}
	fi, err := os.Stat(pfad)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// WALKompaktierungsStand meldet, ob und wie oft gekuerzt wurde.
func WALKompaktierungsStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "Ohne Kompaktierung waechst das WAL unbegrenzt. Am 07.09.2026 stand es " +
			"bei 17 GB und die Platte bei 99 %, worauf der Knoten Ueberweisungen ablehnte. " +
			"laeufe=0 bei grosser Datei heisst, dass die Zwei-Runden-Bedingung nie erfuellt " +
			"war -- dann steht dauerhaft etwas in der Flush-Warteschlange.",
		"laeufe":            walKompaktLaeufe.Load(),
		"freigegeben_mb":    walKompaktBytesFrei.Load() >> 20,
		"letzte_seq":        walKompaktLetzteSeq.Load(),
		"letzte_dauer_ms":   walKompaktLetzteMs.Load(),
		"fehler":            walKompaktFehler.Load(),
		"intervall_minuten": int64(walKompaktIntervall / time.Minute),
		"ab_groesse_mb":     int64(walKompaktAbBytes >> 20),
	}
}
