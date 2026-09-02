package keeper

import (
	"testing"
	"time"
)

func frischeDAG(t *testing.T, hoehen map[string]int64, alter map[string]time.Duration) *BlockDAG {
	t.Helper()
	dag := &BlockDAG{
		peerSyncHeight: map[string]int64{},
		peerSyncSeenAt: map[string]time.Time{},
	}
	for u, h := range hoehen {
		dag.peerSyncHeight[u] = h
		a, da := alter[u]
		if !da {
			a = 0
		}
		dag.peerSyncSeenAt[u] = time.Now().Add(-a)
	}
	return dag
}

// Ein Peer, der weit zurueckliegt, muss die Blockgroesse druecken -- das ist
// der ganze Zweck. Ohne die Bremse produziert dieser Knoten weiter Bloecke,
// die jener nicht nachvollziehen kann, und der steht dann minutenlang.
func TestPeerLagBremse_DrosseltBeiRueckstand(t *testing.T) {
	dag := frischeDAG(t, map[string]int64{"a": 1000}, nil)
	voll := dag.groesstenFrischenRueckstand(1100) // 100 zurueck, im Slack
	if voll != 100 {
		t.Fatalf("Rueckstand %d, erwartet 100", voll)
	}
	if g := dag.blockTxCapFuerHoehe(1100); g != maxTxsPerBlock {
		t.Fatalf("bei 100 Rueckstand ist die Grenze %d, erwartet voll (%d)", g, maxTxsPerBlock)
	}
	// Weit zurueck: auf den Boden.
	if g := dag.blockTxCapFuerHoehe(1000 + peerLagVollVorgabe + 500); g != peerLagBodenVorgabe {
		t.Fatalf("weit zurueck: Grenze %d, erwartet Boden %d", g, peerLagBodenVorgabe)
	}
	// Dazwischen: kleiner als voll, groesser als der Boden.
	mitte := dag.blockTxCapFuerHoehe(1000 + (peerLagSlackVorgabe+peerLagVollVorgabe)/2)
	if mitte >= maxTxsPerBlock || mitte <= peerLagBodenVorgabe {
		t.Fatalf("in der Mitte: Grenze %d, erwartet zwischen %d und %d",
			mitte, peerLagBodenVorgabe, maxTxsPerBlock)
	}
}

// DER GEFAEHRLICHE FALL: ein Peer, von dem seit Langem nichts kam, darf die
// Kette NICHT dauerhaft bremsen. peerSyncHeight ist monoton -- ein
// abgeschalteter Peer behaelt seine letzte Hoehe, und der Rueckstand waechst
// sonst mit jedem eigenen Block weiter, fuer immer.
func TestPeerLagBremse_StummerPeerBremstNicht(t *testing.T) {
	dag := frischeDAG(t,
		map[string]int64{"weg": 1000},
		map[string]time.Duration{"weg": peerLagFrische + time.Minute})
	if r := dag.groesstenFrischenRueckstand(999999); r != 0 {
		t.Fatalf("stummer Peer erzeugt Rueckstand %d -- er wuerde die Kette fuer immer drosseln", r)
	}
	if g := dag.blockTxCapFuerHoehe(999999); g != maxTxsPerBlock {
		t.Fatalf("stummer Peer druckt die Grenze auf %d", g)
	}
}

// Ein FESTSTECKENDER Peer meldet weiter seine unveraenderte Hoehe -- der muss
// die Bremse ausloesen, nicht von ihr ausgenommen werden. Genau deshalb setzt
// advancePeerSyncHeight den Zeitstempel auch ohne Fortschritt.
func TestPeerLagBremse_FeststeckenderPeerBremstSehrWohl(t *testing.T) {
	dag := frischeDAG(t,
		map[string]int64{"steht": 1000},
		map[string]time.Duration{"steht": 5 * time.Second})
	if g := dag.blockTxCapFuerHoehe(1000 + peerLagVollVorgabe + 100); g != peerLagBodenVorgabe {
		t.Fatalf("feststeckender Peer: Grenze %d, erwartet Boden %d -- genau dieser Fall "+
			"soll gebremst werden", g, peerLagBodenVorgabe)
	}
}

// Boden 0 schaltet die Bremse ausdruecklich ab.
func TestPeerLagBremse_BodenNullSchaltetAb(t *testing.T) {
	t.Setenv(peerLagBodenEnv, "0")
	dag := frischeDAG(t, map[string]int64{"a": 1}, nil)
	if g := dag.blockTxCapFuerHoehe(999999); g != maxTxsPerBlock {
		t.Fatalf("abgeschaltet, aber Grenze %d", g)
	}
}

// Die Grenze darf NIE 0 werden -- ein Block ohne Ueberweisungen bringt den
// Rueckstaendigen zwar voran, aber die Kette darf nicht anhalten koennen,
// nur weil irgendein Peer haengt.
func TestPeerLagBremse_NieNull(t *testing.T) {
	for _, lag := range []int64{0, 1, 500, 5000, 1_000_000} {
		dag := frischeDAG(t, map[string]int64{"a": 1000}, nil)
		if g := dag.blockTxCapFuerHoehe(1000 + lag); g <= 0 {
			t.Fatalf("bei Rueckstand %d ist die Grenze %d -- die Kette wuerde anhalten", lag, g)
		}
	}
}

// blockTxCap laeuft in der Blockproduktion, und die haelt dag.mu EXKLUSIV.
// Es darf deshalb keine Sperre nehmen, die dabei blockieren kann.
//
// Die erste Fassung tat genau das -- sie rief dag.Height(), das dag.mu.RLock()
// nimmt. Go's RWMutex ist nicht reentrant: eine Goroutine mit der
// Schreibsperre bekommt die Lesesperre nie. C1 hing damit am 02.09.2026 in der
// Sekunde fest, in der er Produzent wurde: Container oben, Logs stehen nach
// "[EPOCH] ... (producer)", keine HTTP-Antwort, Load 0.00.
func TestPeerLagBremse_BlockiertNichtUnterGehaltenerDagSperre(t *testing.T) {
	dag := &BlockDAG{
		peerSyncHeight: map[string]int64{"a": 1000},
		peerSyncSeenAt: map[string]time.Time{"a": time.Now()},
		blocks:         map[string]*Block{},
		tips:           map[string]bool{},
	}
	dag.setHeight(5000)

	// Genau die Lage der Blockproduktion: dag.mu exklusiv gehalten.
	dag.mu.Lock()
	defer dag.mu.Unlock()

	fertig := make(chan int, 1)
	go func() { fertig <- dag.blockTxCap() }()

	select {
	case g := <-fertig:
		if g <= 0 {
			t.Fatalf("Grenze %d -- die Kette wuerde anhalten", g)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blockTxCap blockiert, waehrend dag.mu gehalten wird. In der " +
			"Blockproduktion ist das ein Selbstblock: der Knoten haengt sich auf, " +
			"sobald er Produzent wird")
	}
}

// Dasselbe fuer syncPeerMu: auch die darf die Produktion nicht anhalten.
func TestPeerLagBremse_BlockiertNichtBeiBelegtemSyncPeerMu(t *testing.T) {
	dag := &BlockDAG{
		peerSyncHeight: map[string]int64{"a": 1000},
		peerSyncSeenAt: map[string]time.Time{"a": time.Now()},
	}
	dag.setHeight(5000)
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()

	fertig := make(chan int, 1)
	go func() { fertig <- dag.blockTxCap() }()
	select {
	case g := <-fertig:
		if g != maxTxsPerBlock {
			t.Fatalf("bei belegtem syncPeerMu ist die Grenze %d, erwartet die volle %d -- "+
				"ohne Peer-Daten wird nicht gebremst", g, maxTxsPerBlock)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blockTxCap stellt sich an syncPeerMu an")
	}
}
