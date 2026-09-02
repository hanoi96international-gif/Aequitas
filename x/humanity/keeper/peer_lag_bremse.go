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
//	AEQUITAS_PEER_LAG_SLACK    Rueckstand ohne Wirkung (Vorgabe 50)
//	AEQUITAS_PEER_LAG_VOLL     ab hier nur noch der Boden (Vorgabe 500)
//	AEQUITAS_PEER_LAG_BODEN    kleinste Blockgroesse (Vorgabe 500, 0 = Bremse aus)
//
// Ein unbrauchbarer Wert ergibt die Vorgabe.
// # DIE SCHWELLEN STAMMEN AUS EINER MESSUNG, NICHT AUS EINER SCHAETZUNG
//
// Erste Fassung: Slack 200, voll 2000. Beides zu locker. Im Lastlauf vom
// 02.09.2026 blieb der Rueckstand die ganze Zeit bei 60-90 -- die Bremse
// griff also nie (gebremst=0) -- und trotzdem hungerte C1 aus: nach dem Lauf
// konnte er fuenf Minuten lang keinen Block mehr anhaengen, woraufhin die
// Selbstheilung einen vollstaendigen Resync ausloeste. Ein Rueckstand, der
// nach dieser Skala "unauffaellig" war, hat den Knoten aus der Kette
// genommen.
//
// Die neuen Werte sind auf genau diesen Fall gesetzt: bei 50 beginnt die
// Drosselung, bei 500 ist sie voll. Der beobachtete Bereich 60-90 liegt damit
// deutlich IN der Drosselung statt darunter.
//
// Dass kleinere Bloecke die Gesamtarbeit nicht verringern, ist richtig und
// trotzdem kein Einwand: sie sind einzeln leichter anzuhaengen, stauen sich
// weniger zu Waisen auf, und der Rueckstau landet dort, wo er hingehoert --
// im Warteschlangen-Topf des Produzenten, wo die Annahmekontrolle ihn sieht
// und an den Aufrufer weitergibt.
const (
	peerLagSlackVorgabe = 50
	peerLagVollVorgabe  = 500
	peerLagBodenVorgabe = 500
	peerLagFrische      = 90 * time.Second

	peerLagSlackEnv = "AEQUITAS_PEER_LAG_SLACK"
	peerLagVollEnv  = "AEQUITAS_PEER_LAG_VOLL"
	peerLagBodenEnv = "AEQUITAS_PEER_LAG_BODEN"
)

// letzteBlockGroesse ist die Zahl der Ueberweisungen im zuletzt produzierten
// Block -- der Ankerpunkt, von dem aus gedrosselt wird. Siehe blockTxCap.
var letzteBlockGroesse atomic.Int64

// vorherigerRueckstand haelt den Rueckstand des letzten Blocks -- der
// Regelkreis braucht die RICHTUNG, nicht nur den Wert.
var vorherigerRueckstand atomic.Int64

// MerkeBlockGroesse wird nach jeder Blockproduktion aufgerufen.
func MerkeBlockGroesse(n int) {
	if n > 0 {
		letzteBlockGroesse.Store(int64(n))
	}
}

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
	slack := peerLagSlack()

	// Ankerpunkt ist die TATSAECHLICHE Blockgroesse, nicht maxTxsPerBlock.
	//
	// Mit 10.000 als Anker war die Bremse wirkungslos: echte Bloecke trugen
	// bei ~3.800 TPS rund 3.800 Ueberweisungen, eine Obergrenze von 9.683
	// bindet darueber nicht. Die Anzeige meldete "gebremst", C1 fiel trotzdem
	// weiter zurueck (02.09.2026).
	anker := letzteBlockGroesse.Load()
	if anker <= 0 || anker > maxTxsPerBlock {
		anker = maxTxsPerBlock
	}
	if anker < int64(boden) {
		anker = int64(boden)
	}

	vorher := peerLagLetzterCap.Load()
	if vorher <= 0 || vorher > anker {
		vorher = anker
	}
	waechst := rueckstand > vorherigerRueckstand.Load()
	vorherigerRueckstand.Store(rueckstand)

	var neu int64
	switch {
	case rueckstand > slack && waechst:
		// Ueber der Schwelle UND waechst -- kraeftig herunter. Faktor 0,7
		// halbiert in zwei Bloecken.
		neu = vorher * 7 / 10
	case rueckstand > slack:
		// Ueber der Schwelle, haelt sich: leicht weiter herunter.
		neu = vorher * 9 / 10
	default:
		// Unter der Schwelle: langsam erholen, ein Zwanzigstel je Block.
		// Additiv und nicht sprunghaft, damit die Drosselung nicht sofort
		// wieder wegfaellt und der Rueckstand erneut waechst.
		neu = vorher + anker/20
	}
	if neu < int64(boden) {
		neu = int64(boden)
	}
	if neu > anker {
		neu = anker
	}
	if neu < anker {
		peerLagGebremst.Add(1)
	} else {
		peerLagUngebremst.Add(1)
	}
	peerLagLetzterCap.Store(neu)
	return int(neu)
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
		"gebremst":            g,
		"ungebremst":          u,
		"gebremst_pct":        anteil,
		"letzter_cap":         peerLagLetzterCap.Load(),
		"letzte_blockgroesse": letzteBlockGroesse.Load(),
		"letzter_rueckstand":  peerLagLetzterLag.Load(),
		"slack":               peerLagSlack(),
		"voll":                peerLagVoll(),
		"boden":               peerLagBoden(),
		"bedeutung": "Verkleinert Bloecke, wenn ein Peer zurueckfaellt. Der Produzent wendet " +
			"Ueberweisungen ueber den WAL-Schnellpfad an, der Nachvollziehende ueber " +
			"Block-Replay -- ohne Bremse produziert er dauerhaft mehr, als jener aufnehmen " +
			"kann. gebremst_pct > 0 heisst: ein Peer haengt und wird geschont",
	}
}
