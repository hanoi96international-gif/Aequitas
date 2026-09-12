package keeper

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Die Eigenlast-Bremse: wer seinen Takt nicht schafft, macht SEINE Bloecke
// kleiner.
//
// GEMESSEN AM 12.09.2026, Lauf 11:28 UTC, Protokoll je Produktionsversuch:
//
//	C2  ProduceBlock 1,0-1,6 s je Block, davon 500-1.070 ms Warten auf die
//	    Sperre, 170-300 ms Speichern; Bloecke immer 7.000; 0,7 Bloecke/s.
//	C1  ProduceBlock 200-470 ms, jeder Takt ein Block -- und ab Minute 3
//	    auf 1.500 gedrosselt, weil C2 10-24 Hoehen zurueckhing.
//
// Die Peer-Lag-Bremse drosselt den STARKEN Knoten, wenn der schwache
// zurueckfaellt. Der schwache Knoten selbst produzierte weiter volle
// Bloecke -- und genau die kosten ihn je Block 250 ms Speichern plus Bauen
// unter derselben Sperre, auf die sein Replay wartet. Er drosselte damit
// den Partner und blieb selbst am Anschlag. Kettendurchsatz 7.200/s, bei
// Annahme von 13.000/s.
//
// Diese Bremse ist das Gegenstueck: dauert der EIGENE Produktionsversuch
// laenger als 90 % der Blockzeit, schrumpft der eigene Deckel um ein
// Fuenftel je Block; bleibt er unter der halben Blockzeit, waechst er
// wieder (ein Zwanzigstel des Maximums je Block, wie die Peer-Lag-Bremse).
// Gerechnet wird mit der GESAMTdauer inklusive Sperrwarten: das Symptom
// heisst "mein Takt passt nicht", gleich woran es liegt, und kleinere eigene
// Bloecke verkuerzen in jedem Fall den eigenen Anteil an der Sperre.
//
// Ergebnis, das zu erwarten ist: der schwache Knoten packt weniger, holt
// auf, der Rueckstand faellt, die Peer-Lag-Bremse laesst den starken los,
// und die Arbeit wandert dorthin, wo Luft ist. Der Boden ist derselbe wie
// bei der Peer-Lag-Bremse (peerLagBoden); der Deckel ist nie hoeher als der
// harte.
//
// Abschalten: AEQUITAS_EIGENLAST_BREMSE=0.

const eigenlastBremseEnv = "AEQUITAS_EIGENLAST_BREMSE"

var (
	eigenlastLetzteDauerNs atomic.Int64 // Gesamtdauer des letzten Produktionsversuchs
	eigenlastCap           atomic.Int64 // eigener Deckel, 0 = noch nicht gesetzt
	eigenlastGebremst      atomic.Int64 // Bloecke, bei denen dieser Deckel unter dem harten lag
	eigenlastGeschrumpft   atomic.Int64 // Schrumpfschritte
)

func merkeEigenlast(gesamt time.Duration) {
	eigenlastLetzteDauerNs.Store(int64(gesamt))
}

func eigenlastBremseAktiv() bool {
	if n, ok := ganzzahlAusUmgebung(eigenlastBremseEnv); ok {
		return n != 0
	}
	return true
}

// eigenlastDeckel liefert den eigenen Deckel fuer den naechsten Block.
func eigenlastDeckel(hart, boden int) int {
	if !eigenlastBremseAktiv() || hart <= 0 {
		return hart
	}
	blockZeit := time.Duration(ConfiguredBlockTimeSeconds() * float64(time.Second))
	if blockZeit <= 0 {
		return hart
	}
	dauer := time.Duration(eigenlastLetzteDauerNs.Load())
	deckel := eigenlastCap.Load()
	if deckel <= 0 || deckel > int64(hart) {
		deckel = int64(hart)
	}
	switch {
	case dauer > blockZeit*9/10:
		deckel = deckel * 8 / 10
		eigenlastGeschrumpft.Add(1)
	case dauer < blockZeit/2:
		deckel += int64(hart) / 20
	}
	if boden > 0 && deckel < int64(boden) {
		deckel = int64(boden)
	}
	if deckel > int64(hart) {
		deckel = int64(hart)
	}
	if deckel < int64(hart) {
		eigenlastGebremst.Add(1)
	}
	eigenlastCap.Store(deckel)
	return int(deckel)
}

// EigenlastBremseStand fuer /api/health/combined.
func EigenlastBremseStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "Dauert der eigene Produktionsversuch laenger als 90 % der Blockzeit, schrumpft der " +
			"eigene Blockdeckel (x0,8 je Block); unter der halben Blockzeit waechst er wieder. " +
			"Gegenstueck zur Peer-Lag-Bremse: der langsame Knoten schont sich selbst, statt nur " +
			"den schnellen zu drosseln. " + eigenlastBremseEnv + "=0 schaltet ab.",
		"aktiv":            eigenlastBremseAktiv(),
		"deckel":           eigenlastCap.Load(),
		"letzte_dauer_ms":  fmt.Sprintf("%.0f", float64(eigenlastLetzteDauerNs.Load())/1e6),
		"gebremst_bloecke": eigenlastGebremst.Load(),
		"schrumpfschritte": eigenlastGeschrumpft.Load(),
	}
}
