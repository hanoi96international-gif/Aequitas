package keeper

// WAECHTER BLOCKZEIT (Quantengeld Weg B, B2d; DEZENTRAL.md 18)
//
// Der Zufall eines Loses kommt aus der drand-Runde, die zur Bestellzeit plus
// quantenSLosAbstand gehoert (quanten_s_bestellung.go). Die Bestellzeit ist
// die Buchzeit, und die darf hoechstens quantenSBestellBuchSpiel vor dem
// Block liegen. Die Blockzeit selbst prueft jeder Knoten nur nach vorn
// ("nicht in der Zukunft") und gegen die Eltern -- ein einziger
// Blockerzeuger koennte sie also hinter der echten Zeit HALTEN. Dann laege die
// Runde eines Loses schon in der Vergangenheit, und wer den Block erzeugt,
// kennte das Los vor der Bestellung.
//
// Das ist von aussen sichtbar: der Takt erzeugt je BLOCK_TIME einen Block,
// auch einen leeren (block_cadence.go). Der neueste Block liegt also immer
// nur Sekunden hinter der echten Zeit. /api/health/combined meldet darum den
// Zeitstempel des neuesten Blocks (chain.neuester_block_zeit), und die Wache
// (.github/wache_pruefung.py) vergleicht ihn mit IHRER EIGENEN Uhr -- nicht
// mit der des Knotens, der sich sonst selbst entlasten koennte. Liegt er mehr
// als wacheBlockzeitRueckstand zurueck, wird die Wache rot: der Knoten haengt
// oder die Blockzeit wird zurueckgehalten.
//
// Keine Konsensregel: nur eine Auskunft. Sie wartet nie auf dag.mu (eine
// Block-Welle haelt die Schreibsperre); ist die Sperre belegt, kommt der
// zuletzt gelesene Wert.

// NeuesteBlockZeit: Zeitstempel des kanonisch neuesten Blocks (dieselbe Regel
// wie LatestBlock: BlueScore, dann Hash), ohne auf dag.mu zu warten.
func (dag *BlockDAG) NeuesteBlockZeit() int64 {
	if dag.mu.TryRLock() {
		var latest *Block
		for hash := range dag.tips {
			b := dag.blocks[hash]
			if b == nil {
				continue
			}
			if latest == nil || b.BlueScore > latest.BlueScore || (b.BlueScore == latest.BlueScore && b.Hash < latest.Hash) {
				latest = b
			}
		}
		dag.mu.RUnlock()
		if latest != nil {
			dag.neuesteBlockZeit.Store(latest.Timestamp)
		}
	}
	return dag.neuesteBlockZeit.Load()
}
