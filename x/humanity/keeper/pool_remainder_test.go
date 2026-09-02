package keeper

import (
	"fmt"
	"sync"
	"testing"
)

// Der Rest einer Ausschuettung darf weder entstehen noch verschwinden.
//
// TestSupplyConservation_UBIDistribution_RemaindersThatUsedToMint deckt die
// eine Richtung ab: floor6 statt round6 verhindert, dass die Summe der Anteile
// den Topf uebersteigt. Die andere Richtung war nie geprueft -- und sie war
// jahrelang falsch: der Topf wurde vollstaendig genullt, obwohl nur die
// gefloorten Anteile ausgezahlt waren. Der Wachhund der Versorgung meldete das
// als -0,000014 AEQ, ohne dass jemand die Ursache kannte.

// verteilterTopf richtet einen Zustand mit n Menschen und einem Topf ein und
// verteilt einmal.
func verteilterTopf(t *testing.T, topf float64, menschen int, verteiltAm int64) (vorher, nachher, topfDanach float64) {
	t.Helper()
	cs := newTestState()
	for i := 0; i < menschen; i++ {
		addr := fmt.Sprintf("0xq%02d", i)
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true})
		cs.humanCount++
	}
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(topf)})
	cs.pool = &PoolState{}

	vorher = totalAEQ(cs)
	cs.mu.Lock()
	_, err := cs.distributeUBIPoolLocked(t.Context(), verteiltAm)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("Topf %.6f auf %d Menschen: %v", topf, menschen, err)
	}
	nachher = totalAEQ(cs)
	if acc, ok := cs.accounts.Get(ubiPoolAddr); ok {
		topfDanach = acc.Balance.Float()
	}
	return
}

// Ohne Schwelle bleibt alles beim Alten -- das ist die Zusicherung, auf die
// sich das Ausrollen stuetzt: ein Knoten mit dem neuen Code, aber ohne gesetzte
// Variable, rechnet exakt wie einer mit dem alten. Ohne das waere jeder Deploy
// eine Kettenspaltung.
func TestRestUebertrag_OhneSchwelleUnveraendert(t *testing.T) {
	t.Setenv(restUebertragUmgebung, "")
	// sync.Once laesst sich nicht zuruecksetzen -- eine frische zuweisen.
	restUebertragEinmal = sync.Once{}

	for _, tc := range []struct {
		topf     float64
		menschen int
	}{{0.000007, 2}, {0.000025, 10}, {1.0000005, 2}} {
		vorher, nachher, topfDanach := verteilterTopf(t, tc.topf, tc.menschen, 1800000000)
		if topfDanach != 0 {
			t.Errorf("Topf %.7f auf %d: ohne Schwelle muss der Topf genullt werden, steht aber auf %.9f",
				tc.topf, tc.menschen, topfDanach)
		}
		if nachher-vorher > 1e-9 {
			t.Errorf("Topf %.7f auf %d: es wurde geschoepft (%+.9f)", tc.topf, tc.menschen, nachher-vorher)
		}
	}
}

// Mit Schwelle ist die Versorgung exakt erhalten: was nicht ausgezahlt wurde,
// steht im Topf, statt zu verschwinden.
func TestRestUebertrag_ErhaeltDieVersorgungExakt(t *testing.T) {
	const schwelle = 1700000000
	t.Setenv(restUebertragUmgebung, fmt.Sprint(schwelle))
	// sync.Once laesst sich nicht zuruecksetzen -- eine frische zuweisen.
	restUebertragEinmal = sync.Once{}
	restUebertragAb = 0

	for _, tc := range []struct {
		topf     float64
		menschen int
	}{
		{0.000007, 2},   // 3,5 Mikro je Kopf -> 3 ausgezahlt, 1 Rest
		{0.000025, 10},  // 2,5 -> 2 je Kopf, 5 Rest
		{1.0000005, 2},  // halbes Mikro Rest
		{0.000011, 2},   // 5,5 -> 5, 1 Rest
		{10.0000009, 3}, // krummer Rest auf drei
	} {
		vorher, nachher, topfDanach := verteilterTopf(t, tc.topf, tc.menschen, schwelle+1)
		if abweichung := nachher - vorher; abweichung > 1e-9 || abweichung < -1e-9 {
			t.Errorf("Topf %.7f auf %d Menschen: Versorgung um %+.9f AEQ veraendert "+
				"(Topf danach %.9f) -- mit Restuebertrag muss sie exakt gleich bleiben",
				tc.topf, tc.menschen, abweichung, topfDanach)
		}
	}
}

// Der Rest darf nie negativ werden: das waere Geldschoepfung ueber den Umweg
// eines Topfes, der mehr enthaelt, als hineingelegt wurde.
func TestRestUebertrag_NieNegativ(t *testing.T) {
	for _, f := range []struct{ gesamt, ausgezahlt float64 }{
		{1, 1}, {1, 1.0000001}, {0, 0}, {0.000001, 0.000002},
	} {
		if got := neuerTopfstand(f.gesamt, f.ausgezahlt, 0); got < 0 {
			t.Errorf("gesamt %.7f, ausgezahlt %.7f -> %.9f (negativ)", f.gesamt, f.ausgezahlt, got)
		}
	}
}

// Eine Runde VOR der Schwelle bleibt beim alten Verhalten, auch wenn die
// Variable gesetzt ist. Sonst haette der Zeitpunkt des Deploys doch wieder
// ueber den Zustand entschieden.
func TestRestUebertrag_VorDerSchwelleAltesVerhalten(t *testing.T) {
	const schwelle = 1700000000
	t.Setenv(restUebertragUmgebung, fmt.Sprint(schwelle))
	// sync.Once laesst sich nicht zuruecksetzen -- eine frische zuweisen.
	restUebertragEinmal = sync.Once{}
	restUebertragAb = 0

	_, _, topfDanach := verteilterTopf(t, 0.000007, 2, schwelle-1)
	if topfDanach != 0 {
		t.Errorf("vor der Schwelle muss genullt werden, Topf steht auf %.9f", topfDanach)
	}
}
