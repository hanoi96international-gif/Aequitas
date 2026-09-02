package keeper

import (
	"os"
	"testing"
	"time"
)

// setzeEigeneHoehe stellt die "eigene Hoehe zum Zeitpunkt der Peer-Meldung"
// auf den Wert, gegen den der Test rechnen will -- ohne sie ist der
// Rueckstand definitionsgemaess 0.
func setzeEigeneHoehe(dag *BlockDAG, h int64) {
	for u := range dag.peerSyncHeight {
		dag.peerSyncEigeneHoehe[u] = h
	}
}

func frischeDAG(t *testing.T, hoehen map[string]int64, alter map[string]time.Duration) *BlockDAG {
	t.Helper()
	dag := &BlockDAG{
		peerSyncHeight:      map[string]int64{},
		peerSyncSeenAt:      map[string]time.Time{},
		peerSyncEigeneHoehe: map[string]int64{},
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
	reglerZuruecksetzen(3800)
	defer reglerZuruecksetzen(0)

	dag := frischeDAG(t, map[string]int64{"a": 1000}, nil)

	// 30 zurueck: unter dem Slack von 50 -- keine Drosselung, also der volle
	// Deckel. Bewusst maxTxsPerBlock und NICHT die letzte Blockgroesse: sonst
	// kaeme die Grenze nach einer Drosselung nie wieder hoch (siehe
	// TestPeerLagBremse_KommtNachDerDrosselungWiederHoch).
	if g := func() int { setzeEigeneHoehe(dag, 1030); return dag.blockTxCapFuerHoehe(1030) }(); g != maxTxsPerBlock {
		t.Fatalf("bei 30 Rueckstand ist die Grenze %d, erwartet den vollen Deckel %d", g, maxTxsPerBlock)
	}

	// Jetzt waechst der Rueckstand ueber die Schwelle. Der Regelkreis muss
	// SPUERBAR drosseln, nicht um drei Prozent -- genau daran ist die lineare
	// Rampe gescheitert.
	g1 := func() int { setzeEigeneHoehe(dag, 1065); return dag.blockTxCapFuerHoehe(1065) }()
	g2 := func() int { setzeEigeneHoehe(dag, 1070); return dag.blockTxCapFuerHoehe(1070) }()
	g3 := func() int { setzeEigeneHoehe(dag, 1080); return dag.blockTxCapFuerHoehe(1080) }()
	t.Logf("wachsender Rueckstand 65 -> 70 -> 80 ergibt Grenzen %d -> %d -> %d", g1, g2, g3)
	if g3 >= g2 || g2 >= g1 {
		t.Fatalf("die Grenze faellt nicht monoton: %d, %d, %d", g1, g2, g3)
	}
	if g3 > 3800*6/10 {
		t.Fatalf("nach drei wachsenden Bloecken ist die Grenze noch %d von 3800 -- "+
			"das ist zu zaghaft, genau daran ist die Rampe gescheitert", g3)
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
	if r := func() int64 { setzeEigeneHoehe(dag, 999999); return dag.groesstenFrischenRueckstand(0) }(); r != 0 {
		t.Fatalf("stummer Peer erzeugt Rueckstand %d -- er wuerde die Kette fuer immer drosseln", r)
	}
	if g := func() int { setzeEigeneHoehe(dag, 999999); return dag.blockTxCapFuerHoehe(999999) }(); g != maxTxsPerBlock {
		t.Fatalf("stummer Peer druckt die Grenze auf %d", g)
	}
}

// Ein FESTSTECKENDER Peer meldet weiter seine unveraenderte Hoehe -- der muss
// die Bremse ausloesen, nicht von ihr ausgenommen werden. Genau deshalb setzt
// advancePeerSyncHeight den Zeitstempel auch ohne Fortschritt.
func TestPeerLagBremse_FeststeckenderPeerBremstSehrWohl(t *testing.T) {
	reglerZuruecksetzen(3800)
	defer reglerZuruecksetzen(0)

	dag := frischeDAG(t,
		map[string]int64{"steht": 1000},
		map[string]time.Duration{"steht": 5 * time.Second})

	// Der Peer steht, der Rueckstand waechst mit jedem eigenen Block. Der
	// Regelkreis muss ihn bis auf den Boden herunterfahren -- nicht in einem
	// Sprung, aber verlaesslich.
	var g int
	for i := int64(1); i <= 40; i++ {
		g = func() int {
			setzeEigeneHoehe(dag, 1000+peerLagSlackVorgabe+i*20)
			return dag.blockTxCapFuerHoehe(1000 + peerLagSlackVorgabe + i*20)
		}()
	}
	if g != gesetzterBoden {
		t.Fatalf("nach 40 Bloecken mit wachsendem Rueckstand ist die Grenze %d, "+
			"erwartet der Boden %d -- ein feststeckender Peer muss verlaesslich "+
			"bis ganz herunter drosseln", g, gesetzterBoden)
	}
}

// Boden 0 schaltet die Bremse ausdruecklich ab.
func TestPeerLagBremse_BodenNullSchaltetAb(t *testing.T) {
	t.Setenv(peerLagBodenEnv, "0")
	dag := frischeDAG(t, map[string]int64{"a": 1}, nil)
	if g := func() int { setzeEigeneHoehe(dag, 999999); return dag.blockTxCapFuerHoehe(999999) }(); g != maxTxsPerBlock {
		t.Fatalf("abgeschaltet, aber Grenze %d", g)
	}
}

// Die Grenze darf NIE 0 werden -- ein Block ohne Ueberweisungen bringt den
// Rueckstaendigen zwar voran, aber die Kette darf nicht anhalten koennen,
// nur weil irgendein Peer haengt.
func TestPeerLagBremse_NieNull(t *testing.T) {
	for _, lag := range []int64{0, 1, 500, 5000, 1_000_000} {
		dag := frischeDAG(t, map[string]int64{"a": 1000}, nil)
		if g := func() int { setzeEigeneHoehe(dag, 1000+lag); return dag.blockTxCapFuerHoehe(1000 + lag) }(); g <= 0 {
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
	reglerZuruecksetzen(0)
	defer reglerZuruecksetzen(0)

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
		// Ohne Peer-Daten gilt Rueckstand 0, also keine Drosselung -- die
		// Grenze muss dem vollen Anker entsprechen.
		if g != maxTxsPerBlock {
			t.Fatalf("bei belegtem syncPeerMu ist die Grenze %d, erwartet die volle %d -- "+
				"ohne Peer-Daten wird nicht gebremst", g, maxTxsPerBlock)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blockTxCap stellt sich an syncPeerMu an")
	}
}

// reglerZuruecksetzen stellt den Zustand des Regelkreises her, damit Tests
// unabhaengig voneinander sind -- er ist bewusst zustandsbehaftet, und ohne
// das traegt ein Test die Drosselung des vorigen mit.
const gesetzterBoden = 500

func reglerZuruecksetzen(blockgroesse int64) {
	// Die Vorgabe ist AUS -- die Tests pruefen das Verhalten im
	// eingeschalteten Zustand und setzen den Boden deshalb selbst.
	os.Setenv(peerLagBodenEnv, "500")
	letzteBlockGroesse.Store(blockgroesse)
	peerLagLetzterCap.Store(0)
	vorherigerRueckstand.Store(0)
	peerLagGebremst.Store(0)
	peerLagUngebremst.Store(0)
}

// Die Bremse muss von der TATSAECHLICHEN Blockgroesse herunterfahren, nicht
// von maxTxsPerBlock.
//
// Erste Fassung nahm 10.000 als Anker. Am 02.09.2026 gemessen: die Bremse
// griff sichtbar (gebremst stieg, cap fiel 9.831 -> 9.683), aber echte
// Bloecke trugen nur ~3.800 Ueberweisungen. Eine Obergrenze ueber der echten
// Groesse bindet nicht -- C1 fiel weiter zurueck, waehrend die Anzeige
// "gebremst" meldete. Eine Bremse, die anzeigt statt zu wirken, ist schlimmer
// als keine.
func TestPeerLagBremse_ErholtSichWiederWennDerPeerAufholt(t *testing.T) {
	reglerZuruecksetzen(3800)
	peerLagLetzterCap.Store(800) // gedrosselt aus einer vorherigen Phase
	vorherigerRueckstand.Store(200)
	defer reglerZuruecksetzen(0)

	dag := frischeDAG(t, map[string]int64{"a": 1000}, nil)

	// Der Peer holt auf -- Rueckstand unter dem Slack. Die Grenze muss
	// steigen, aber NICHT sofort auf den vollen Anker springen: sonst
	// waechst der Rueckstand im naechsten Block gleich wieder.
	vorher := peerLagLetzterCap.Load()
	g := func() int { setzeEigeneHoehe(dag, 1010); return dag.blockTxCapFuerHoehe(1010) }()
	if int64(g) <= vorher {
		t.Fatalf("Grenze %d erholt sich nicht von %d", g, vorher)
	}
	if g == 3800 {
		t.Fatalf("Grenze springt sofort auf den vollen Anker -- die Erholung muss " +
			"additiv sein, sonst schwingt die Regelung")
	}
	t.Logf("Erholung: %d -> %d (Anker 3800)", vorher, g)
}

// DIE SPERRKLINKE: einmal gedrosselt, muss die Grenze wieder bis ganz nach
// oben zurueckkommen koennen.
//
// Die erste Regelkreis-Fassung nahm die letzte Blockgroesse auch als
// OBERGRENZE. Live am 02.09.2026: einmal auf 500 gedrosselt, trugen die
// Bloecke 500, damit wurde der Anker 500, und die Grenze kam nie wieder hoch.
// Der Regelkreis sperrte sich selbst ein -- Rueckstand 0, perfekte
// Synchronitaet, und dauerhaft ein Zehntel des Durchsatzes. Eine Bremse, die
// nicht mehr loslaesst, ist keine Regelung.
func TestPeerLagBremse_KommtNachDerDrosselungWiederHoch(t *testing.T) {
	reglerZuruecksetzen(0)
	defer reglerZuruecksetzen(0)

	dag := frischeDAG(t, map[string]int64{"a": 1000}, nil)

	// Erst drosseln, bis der Boden erreicht ist.
	for i := int64(1); i <= 40; i++ {
		func() int {
			setzeEigeneHoehe(dag, 1000+peerLagSlackVorgabe+i*20)
			return dag.blockTxCapFuerHoehe(1000 + peerLagSlackVorgabe + i*20)
		}()
	}
	unten := peerLagLetzterCap.Load()
	if unten != int64(gesetzterBoden) {
		t.Fatalf("nach dem Drosseln steht die Grenze auf %d, erwartet den Boden %d", unten, gesetzterBoden)
	}
	// Genau die Lage, die sich selbst einsperrte: die Bloecke tragen jetzt
	// nur noch so viel, wie die Grenze erlaubt.
	letzteBlockGroesse.Store(unten)

	// Der Peer holt auf. Die Grenze MUSS wieder bis zum Deckel steigen.
	var g int
	for i := 0; i < 300; i++ {
		g = func() int { setzeEigeneHoehe(dag, 1000); return dag.blockTxCapFuerHoehe(1000) }() // Rueckstand 0
		letzteBlockGroesse.Store(int64(g))
	}
	if g != maxTxsPerBlock {
		t.Fatalf("nach 300 Bloecken ohne Rueckstand steht die Grenze auf %d statt %d -- "+
			"die Bremse laesst nicht mehr los und kostet dauerhaft Durchsatz", g, maxTxsPerBlock)
	}
}

// DIE VORGABE IST AUS, und das muss so bleiben, bis jemand die Bremse unter
// Last beobachtet hat.
//
// Sie lag an einem Tag dreimal daneben, alle drei Male live in der
// Produktion: Selbstblock auf dag.mu, Drosselung ohne Wirkung, und
// Drosselung ohne Anlass. Alles behoben und getestet -- trotzdem gehoert ein
// Mechanismus mit dieser Vorgeschichte nicht per Vorgabe in einen laufenden
// Betrieb.
func TestPeerLagBremse_VorgabeIstAus(t *testing.T) {
	os.Unsetenv(peerLagBodenEnv)
	if peerLagBodenVorgabe != 0 {
		t.Fatalf("peerLagBodenVorgabe = %d, erwartet 0 (aus). Wer das aendert, "+
			"schaltet eine Bremse scharf, die dreimal danebenlag -- bitte erst "+
			"unter Last beobachten", peerLagBodenVorgabe)
	}
	letzteBlockGroesse.Store(3800)
	peerLagLetzterCap.Store(0)
	vorherigerRueckstand.Store(0)
	defer letzteBlockGroesse.Store(0)

	dag := frischeDAG(t, map[string]int64{"a": 1000}, nil)
	setzeEigeneHoehe(dag, 9999) // absurd grosser Rueckstand
	if g := dag.blockTxCapFuerHoehe(9999); g != maxTxsPerBlock {
		t.Fatalf("mit der Vorgabe wird gedrosselt (Grenze %d) -- sie muss aus sein", g)
	}
}
