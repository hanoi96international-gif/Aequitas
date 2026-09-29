package keeper

import (
	"sync/atomic"
	"testing"
	"time"
)

// Ein Instrument, das nur die erwarteten Ergebnisse beschreiben kann, ist
// keines -- das hat diese Sitzung schon einmal Stunden gekostet (siehe
// replay_pfad_stats.go). Deshalb pruefen diese Tests genau die Eigenschaft,
// auf die sich die naechste Entscheidung stuetzt: dass die benannten Phasen
// plus `rest` die gemessene Haltezeit ergeben, und dass ein grosses `rest`
// auch wirklich als solches herauskommt statt still in eine Phase zu wandern.

func TestReplayPhasen_PhasenPlusRestErgebenDenHalt(t *testing.T) {
	ReplayPhasenZuruecksetzen()
	t.Cleanup(ReplayPhasenZuruecksetzen)

	// Ein Block: 100 ms Halt, davon 20 ms benannt. Der Rest MUSS als rest
	// erscheinen -- das ist die Aussage, wegen der es das Instrument gibt.
	rpSnapshotNanos.Store(int64(5 * time.Millisecond))
	rpParallelNanos.Store(int64(8 * time.Millisecond))
	rpSeriellNanos.Store(int64(4 * time.Millisecond))
	rpStateRootNanos.Store(int64(2 * time.Millisecond))
	rpCommitNanos.Store(int64(1 * time.Millisecond))
	rpHaltNanos.Store(int64(100 * time.Millisecond))
	rpBloecke.Store(1)
	rpSeriellAufrufe.Store(2)

	s := ReplayPhasenStand()
	if got := s["halt_ms"].(float64); got != 100 {
		t.Fatalf("halt_ms = %v, erwartet 100", got)
	}
	if got := s["rest_ms"].(float64); got < 79.9 || got > 80.1 {
		t.Errorf("rest_ms = %v, erwartet 80 (100 minus die 20 benannten)", got)
	}
	if got := s["rest_anteil_pct"].(float64); got < 79.9 || got > 80.1 {
		t.Errorf("rest_anteil_pct = %v, erwartet 80", got)
	}
	// Die eine Zahl, die je Ueberweisung anfaellt: 4 ms auf 2 Aufrufe.
	if got := s["seriell_je_aufruf_ms"].(float64); got != 2 {
		t.Errorf("seriell_je_aufruf_ms = %v, erwartet 2", got)
	}
}

func TestReplayPhasen_MittelwertProBlockNichtSumme(t *testing.T) {
	ReplayPhasenZuruecksetzen()
	t.Cleanup(ReplayPhasenZuruecksetzen)

	// Vier Bloecke zu je 50 ms Halt. halt_ms muss 50 melden, nicht 200 --
	// sonst ist die Zahl nicht mit exclusive_avg_ms vergleichbar, und genau
	// dieser Vergleich ist der Zweck.
	rpHaltNanos.Store(int64(200 * time.Millisecond))
	rpBloecke.Store(4)
	rpSeriellNanos.Store(int64(40 * time.Millisecond))

	s := ReplayPhasenStand()
	if got := s["halt_ms"].(float64); got != 50 {
		t.Fatalf("halt_ms = %v, erwartet 50 (Mittel je Block, nicht Summe)", got)
	}
	if got := s["seriell_ms"].(float64); got != 10 {
		t.Errorf("seriell_ms = %v, erwartet 10", got)
	}
	if got := s["seriell_anteil_pct"].(float64); got != 20 {
		t.Errorf("seriell_anteil_pct = %v, erwartet 20", got)
	}
}

func TestReplayPhasen_LeerIstNullUndTeiltNichtDurchNull(t *testing.T) {
	ReplayPhasenZuruecksetzen()
	t.Cleanup(ReplayPhasenZuruecksetzen)

	s := ReplayPhasenStand() // darf nicht panicken
	for _, k := range []string{"halt_ms", "rest_ms", "seriell_je_aufruf_ms", "rest_anteil_pct"} {
		if got := s[k].(float64); got != 0 {
			t.Errorf("%s = %v auf leeren Zaehlern, erwartet 0", k, got)
		}
	}
	if got := s["bloecke"].(int64); got != 0 {
		t.Errorf("bloecke = %v, erwartet 0", got)
	}
}

func TestReplayPhasen_MerkeAddiertUndZaehlt(t *testing.T) {
	ReplayPhasenZuruecksetzen()
	t.Cleanup(ReplayPhasenZuruecksetzen)

	var z atomic.Int64
	start := time.Now().Add(-30 * time.Millisecond)
	merkeReplayPhase(&z, start)
	if z.Load() < int64(30*time.Millisecond) {
		t.Errorf("merkeReplayPhase hat %v addiert, erwartet mindestens 30ms", time.Duration(z.Load()))
	}

	merkeReplaySeriellZeit(time.Now().Add(-5 * time.Millisecond))
	merkeReplaySeriellZeit(time.Now().Add(-5 * time.Millisecond))
	if got := rpSeriellAufrufe.Load(); got != 2 {
		t.Errorf("seriell_aufrufe = %d, erwartet 2", got)
	}

	merkeReplayBlock(70 * time.Millisecond)
	if got := rpBloecke.Load(); got != 1 {
		t.Errorf("bloecke = %d, erwartet 1", got)
	}
	if got := rpHaltNanos.Load(); got != int64(70*time.Millisecond) {
		t.Errorf("halt = %v, erwartet 70ms", time.Duration(got))
	}
}

// Der teuerste Halt muss vollstaendig erhalten bleiben -- Hoehe, Groesse und
// alle Phasen. Ein Mittelwert verschluckt genau den Ausreisser, der die
// Blockproduktion anhaelt (gemessen 06.09.2026: Mittel 89 ms, schlimmster
// 35.991 ms), und ohne diese Aufschluesselung bliebe die Ursache Vermutung.
func TestReplaySchlimmster_HaeltDenTeuerstenHaltVollstaendig(t *testing.T) {
	for _, z := range []*atomic.Int64{&rpMaxHaltNanos, &rpMaxHoehe, &rpMaxTxAnzahl,
		&rpMaxSnapshotNs, &rpMaxBeginNs, &rpMaxParallelNs, &rpMaxSeriellNs,
		&rpMaxSammlerNs, &rpMaxStateRootNs, &rpMaxCommitNs} {
		z.Store(0)
	}
	ms := time.Millisecond

	// Ein billiger Block, dann ein teurer, dann wieder ein billiger.
	merkeReplayBlockDetail(50*ms, 100, 10, replayBlockPhasen{snapshot: ms, begin: ms, parallel: 10 * ms, seriell: 5 * ms, sammler: ms, stateroot: ms, commit: ms})
	merkeReplayBlockDetail(9000*ms, 4242, 8888, replayBlockPhasen{snapshot: 2 * ms, begin: 3 * ms, parallel: 1000 * ms, seriell: 7000 * ms, sammler: 20 * ms, stateroot: 5 * ms, commit: 10 * ms})
	merkeReplayBlockDetail(60*ms, 300, 12, replayBlockPhasen{snapshot: ms, begin: ms, parallel: 12 * ms, seriell: 6 * ms, sammler: ms, stateroot: ms, commit: ms})

	s := ReplaySchlimmsterStand()
	if got := s["halt_ms"].(float64); got != 9000 {
		t.Fatalf("halt_ms = %v, erwartet 9000 (der teuerste, nicht der letzte)", got)
	}
	if got := s["hoehe"].(int64); got != 4242 {
		t.Errorf("hoehe = %v, erwartet 4242 -- ohne sie ist der Block nicht auffindbar", got)
	}
	if got := s["transaktionen"].(int64); got != 8888 {
		t.Errorf("transaktionen = %v, erwartet 8888 -- die Zahl entscheidet, ob der Block ungewoehnlich gross war", got)
	}
	if got := s["seriell_ms"].(float64); got != 7000 {
		t.Errorf("seriell_ms = %v, erwartet 7000 -- die Phase, die die Sekunden verbraucht hat", got)
	}
	if got := s["parallel_ms"].(float64); got != 1000 {
		t.Errorf("parallel_ms = %v, erwartet 1000", got)
	}
	// 9000 - (2+3+1000+7000+20+5+10) = 960
	if got := s["rest_ms"].(float64); got < 959 || got > 961 {
		t.Errorf("rest_ms = %v, erwartet 960", got)
	}
}

func TestReplaySchlimmster_EinBilligerBlockUeberschreibtNicht(t *testing.T) {
	for _, z := range []*atomic.Int64{&rpMaxHaltNanos, &rpMaxHoehe, &rpMaxTxAnzahl, &rpMaxSeriellNs} {
		z.Store(0)
	}
	ms := time.Millisecond
	merkeReplayBlockDetail(5000*ms, 77, 999, replayBlockPhasen{snapshot: ms, begin: ms, parallel: ms, seriell: 4000 * ms, sammler: ms, stateroot: ms, commit: ms})
	merkeReplayBlockDetail(10*ms, 78, 1, replayBlockPhasen{snapshot: ms, begin: ms, parallel: ms, seriell: ms, sammler: ms, stateroot: ms, commit: ms})

	s := ReplaySchlimmsterStand()
	if got := s["hoehe"].(int64); got != 77 {
		t.Errorf("hoehe = %v, erwartet 77 -- ein billiger Block hat den teuersten ueberschrieben", got)
	}
	if got := s["seriell_ms"].(float64); got != 4000 {
		t.Errorf("seriell_ms = %v, erwartet 4000", got)
	}
}

// Signatur, Nonce, Schleifenrest und Nachlauf lagen bis zum 29.09.2026 in
// rest. Jetzt muessen sie dort herausgerechnet sein -- sonst zeigt die
// naechste Messung wieder nur eine unerklaerte Zahl.
func TestReplayPhasen_ZusatzphasenVerlassenRest(t *testing.T) {
	ReplayPhasenZuruecksetzen()
	rpMaxHaltNanos.Store(0)
	ms := time.Millisecond
	p := replayBlockPhasen{snapshot: ms, begin: ms, parallel: 10 * ms, seriell: 5 * ms, sammler: ms, stateroot: ms, commit: ms,
		signatur: 3000 * ms, nonce: 2000 * ms, schleifeUebrig: 500 * ms, nachlauf: 100 * ms}
	halt := 6000 * ms
	merkeReplayBlock(halt)
	merkeReplayZusatzphasen(p)
	// Die sieben alten Phasen laufen im Betrieb ueber merkeReplayPhase.
	for z, d := range map[*atomic.Int64]time.Duration{&rpSnapshotNanos: p.snapshot, &rpBeginNanos: p.begin,
		&rpParallelNanos: p.parallel, &rpSeriellNanos: p.seriell, &rpSammlerNanos: p.sammler,
		&rpStateRootNanos: p.stateroot, &rpCommitNanos: p.commit} {
		z.Add(int64(d))
	}
	merkeReplayBlockDetail(halt, 5, 5520, p)

	// 6000 - (1+1+10+5+1+1+1 + 3000+2000+500+100) = 380
	for name, s := range map[string]map[string]interface{}{"mittel": ReplayPhasenStand(), "schlimmster": ReplaySchlimmsterStand()} {
		if got := s["rest_ms"].(float64); got < 379.9 || got > 380.1 {
			t.Errorf("%s: rest_ms = %v, erwartet 380", name, got)
		}
		if got := s["signatur_ms"].(float64); got != 3000 {
			t.Errorf("%s: signatur_ms = %v, erwartet 3000", name, got)
		}
		if got := s["nonce_ms"].(float64); got != 2000 {
			t.Errorf("%s: nonce_ms = %v, erwartet 2000", name, got)
		}
	}
	ReplayPhasenZuruecksetzen()
}
