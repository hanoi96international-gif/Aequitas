package keeper

import (
	"sync/atomic"
	"time"
)

// Kleinere Bloecke, wenn ein Peer nicht mehr hinterherkommt.
//
// # DAS BEOBACHTETE VERHALTEN
//
// Am 01. und 02.09.2026 fuenfmal beobachtet: waehrend eines Lastlaufs gegen
// C2 friert C1 ein. Die Hoehe steht minutenlang, /api/health meldet
// ungesund, das Produktionstor schliesst ("Not yet 3 consecutive clean sync
// cycles"), der Abstand waechst linear auf ueber tausend Bloecke -- und dann
// haengt C1 alles in einem Sprung an (zuletzt 1.299 Bloecke auf einmal).
//
// Der Prozess stirbt dabei NICHT: RestartCount 0, ExitCode 0, kein OOM,
// Speicher bei 34 %. Von aussen sieht es trotzdem wie ein Absturz aus, und
// fuer jeden, der in dem Moment etwas von C1 will, IST es einer.
//
// # WARUM ES PASSIERT
//
// Eine Asymmetrie zwischen den beiden Wegen, auf denen eine Ueberweisung in
// den Zustand kommt:
//
//	produzierender Knoten:  WAL-Schnellpfad, shard-gesperrt, nebenlaeufig
//	nachvollziehender Knoten: Block-Replay
//
// Der Produzent kann also dauerhaft mehr in einen Block packen, als der
// andere in derselben Zeit nachvollziehen kann. evm_storage.go benennt genau
// das ueber maxTxsPerBlock: "That drains a backlog faster than any replayer
// can absorb it."
//
// Was fehlte, war die Gegenrichtung. AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING
// erlaubt einem Knoten, waehrend des eigenen Aufholens zu produzieren -- aber
// nichts bremste den Produzenten, wenn ein ANDERER ertrinkt. Der Produzent
// weiss es sogar: dag.peerSyncHeight fuehrt die Hoehe je Peer.
//
// # WIE GEBREMST WIRD
//
// Voller Block, solange alle Peers innerhalb von peerLagSlack liegen. Danach
// linear kleiner werdend bis peerLagVoll, ab dort peerLagBoden.
//
// Ein Boden statt Null, aus zwei Gruenden: die Kette muss weiterlaufen (auch
// ein Block mit wenigen Ueberweisungen bringt den Rueckstaendigen voran, weil
// er ihn ueberhaupt anhaengen kann), und ein Peer, der aus einem ganz anderen
// Grund haengt, darf die Produktion nicht anhalten koennen. peerLagBoden von
// 500 je Block bei einer Blockzeit von 1 s entspricht 500 Ueberweisungen je
// Sekunde -- weit ueber dem, was echter Betrieb heute erzeugt.
//
// # DIE FRISCHE-PRUEFUNG IST NICHT OPTIONAL
//
// peerSyncHeight ist monoton. Ein abgeschalteter Peer behaelt seine letzte
// Hoehe, und der berechnete Rueckstand waechst mit jedem eigenen Block --
// ohne Zeitstempel wuerde ein einmal entfernter Peer die Blockgroesse fuer
// immer druecken. Deshalb zaehlen nur Peers, von denen innerhalb von
// peerLagFrische etwas kam (siehe peerSyncSeenAt).
//
//	AEQUITAS_PEER_LAG_SLACK    Rueckstand ohne Wirkung (Vorgabe 200)
//	AEQUITAS_PEER_LAG_VOLL     ab hier nur noch der Boden (Vorgabe 2000)
//	AEQUITAS_PEER_LAG_BODEN    kleinste Blockgroesse (Vorgabe 500, 0 = Bremse aus)
//
// Ein unbrauchbarer Wert ergibt die Vorgabe.
const (
	peerLagSlackVorgabe = 200
	peerLagVollVorgabe  = 2000
	peerLagBodenVorgabe = 500
	peerLagFrische      = 90 * time.Second

	peerLagSlackEnv = "AEQUITAS_PEER_LAG_SLACK"
	peerLagVollEnv  = "AEQUITAS_PEER_LAG_VOLL"
	peerLagBodenEnv = "AEQUITAS_PEER_LAG_BODEN"
)

var (
	peerLagGebremst   atomic.Int64 // wie oft ein Block verkleinert wurde
	peerLagUngebremst atomic.Int64
	peerLagLetzterCap atomic.Int64
	peerLagLetzterLag atomic.Int64
)

func peerLagSlack() int64 {
	if n, ok := ganzzahlAusUmgebung(peerLagSlackEnv); ok && n >= 0 {
		return int64(n)
	}
	return peerLagSlackVorgabe
}

func peerLagVoll() int64 {
	if n, ok := ganzzahlAusUmgebung(peerLagVollEnv); ok && n > 0 {
		return int64(n)
	}
	return peerLagVollVorgabe
}

func peerLagBoden() int {
	if n, ok := ganzzahlAusUmgebung(peerLagBodenEnv); ok && n >= 0 {
		return n
	}
	return peerLagBodenVorgabe
}

// groesstenFrischenRueckstand liefert den groessten Rueckstand unter den
// Peers, von denen kuerzlich etwas kam. 0 heisst: niemand haengt.
func (dag *BlockDAG) groesstenFrischenRueckstand(eigeneHoehe int64) int64 {
	// TryLock, nicht Lock. Diese Funktion laeuft unter dag.mu; eine zweite
	// Sperre dort zu erwerben waere eine Reihenfolge-Annahme, die niemand
	// aufgeschrieben hat. Ist syncPeerMu gerade belegt, wird eben nicht
	// gebremst -- ein voller Block ist die harmlosere Antwort als ein
	// Knoten, der steht.
	if !dag.syncPeerMu.TryLock() {
		return 0
	}
	defer dag.syncPeerMu.Unlock()
	jetzt := time.Now()
	var groesster int64
	for url, hoehe := range dag.peerSyncHeight {
		gesehen, da := dag.peerSyncSeenAt[url]
		if !da || jetzt.Sub(gesehen) > peerLagFrische {
			continue // stumm -- siehe Kommentar oben, sonst bremst er ewig
		}
		if r := eigeneHoehe - hoehe; r > groesster {
			groesster = r
		}
	}
	return groesster
}

// blockTxCap liefert, wie viele Ueberweisungen der naechste Block hoechstens
// tragen darf.
func (dag *BlockDAG) blockTxCap() int {
	// heightSchnell, NICHT Height().
	//
	// Diese Funktion laeuft in der Blockproduktion, und die haelt dag.mu
	// EXKLUSIV (block.go, dag.mu.Lock() mit defer Unlock). Height() nimmt
	// dag.mu.RLock() -- und eine Goroutine, die die Schreibsperre haelt, kann
	// die Lesesperre nicht bekommen. Go's RWMutex ist nicht reentrant.
	//
	// Die erste Fassung machte genau das und hat C1 am 02.09.2026 sofort
	// aufgehaengt: Container oben, Logs stehen nach "[EPOCH] ... (producer)",
	// keine HTTP-Antwort, Load 0.00. Kein Burst, kein Absturz -- ein
	// Selbstblock in der Sekunde, in der der Knoten Produzent wurde.
	return dag.blockTxCapFuerHoehe(dag.heightSchnell.Load())
}

// blockTxCapFuerHoehe ist blockTxCap mit ausdruecklich uebergebener eigener
// Hoehe -- damit der Test die Rechnung pruefen kann, ohne eine ganze DAG
// aufzubauen.
func (dag *BlockDAG) blockTxCapFuerHoehe(eigeneHoehe int64) int {
	boden := peerLagBoden()
	if boden <= 0 {
		peerLagUngebremst.Add(1)
		return maxTxsPerBlock // ausdruecklich abgeschaltet
	}
	rueckstand := dag.groesstenFrischenRueckstand(eigeneHoehe)
	peerLagLetzterLag.Store(rueckstand)

	slack, voll := peerLagSlack(), peerLagVoll()
	if rueckstand <= slack || voll <= slack {
		peerLagUngebremst.Add(1)
		peerLagLetzterCap.Store(maxTxsPerBlock)
		return maxTxsPerBlock
	}
	if rueckstand >= voll {
		peerLagGebremst.Add(1)
		peerLagLetzterCap.Store(int64(boden))
		return boden
	}
	// Linear zwischen slack und voll herunterfahren.
	anteil := float64(voll-rueckstand) / float64(voll-slack)
	grenze := boden + int(float64(maxTxsPerBlock-boden)*anteil)
	if grenze < boden {
		grenze = boden
	}
	peerLagGebremst.Add(1)
	peerLagLetzterCap.Store(int64(grenze))
	return grenze
}

// PeerLagBremseStand zeigt die Wirkung in /api/health/combined.
func PeerLagBremseStand() map[string]interface{} {
	g, u := peerLagGebremst.Load(), peerLagUngebremst.Load()
	gesamt := g + u
	anteil := 0.0
	if gesamt > 0 {
		anteil = float64(g) / float64(gesamt) * 100
	}
	return map[string]interface{}{
		"gebremst":           g,
		"ungebremst":         u,
		"gebremst_pct":       anteil,
		"letzter_cap":        peerLagLetzterCap.Load(),
		"letzter_rueckstand": peerLagLetzterLag.Load(),
		"slack":              peerLagSlack(),
		"voll":               peerLagVoll(),
		"boden":              peerLagBoden(),
		"bedeutung": "Verkleinert Bloecke, wenn ein Peer zurueckfaellt. Der Produzent wendet " +
			"Ueberweisungen ueber den WAL-Schnellpfad an, der Nachvollziehende ueber " +
			"Block-Replay -- ohne Bremse produziert er dauerhaft mehr, als jener aufnehmen " +
			"kann. gebremst_pct > 0 heisst: ein Peer haengt und wird geschont",
	}
}
