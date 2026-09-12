package keeper

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
)

// DEN SPEICHER-DAG AUSDUENNEN: RUEMPFE ALTER BLOECKE RAUS, KOEPFE BLEIBEN.
//
// GEMESSEN AM 12.09.2026. Der Speicher-DAG haelt Bloecke bis finalized minus
// pruneBuffer -- rund 500 Hoehen, zwei Bloecke je Hoehe. Mit Bloecken von
// 7.000 Transaktionen sind das sieben Millionen dekodierte Transaktions-
// Strukturen im Heap: 5,3 bis 5,4 GB, exakt am GOMEMLIMIT von 5 GiB, auf
// beiden Boxen. Das Heap-Profil vom Morgen zeigte es schon bei kleineren
// Bloecken: 1,2 GB lebend, davon fast alles JSON-dekodierte Bloecke aus
// handleBlockPush und ProduceBlock. Ab dem Limit bremst der Garbage
// Collector alles, und der schwaechere Knoten faellt zurueck.
//
// WAS DIE RUEMPFE ALTER BLOECKE IM SPEICHER NOCH TUN: nichts. Replay,
// StateRoot, Index und Hash brauchen sie bei der Annahme -- einmal. Danach
// liest sie nur noch, wer den Block AUSLIEFERT (ein Peer beim Aufholen, ein
// Explorer, eine Wallet), und dafuer liegen sie in der Datenbank: als
// Batch in chain_tx_batches (bis zu dessen Byte-Budget) und dauerhaft in
// chain_blocks.transactions_z. calculateBlockHash hasht ueber TxRoot, wenn
// Transactions leer ist -- ein ausgeduennter Block hasht identisch.
//
// DIE ZWEI REGELN, DIE DAS SICHER MACHEN.
//
//  1. Ausgeduennt wird nur, was nachweislich in chain_blocks steht -- die
//     Kandidaten werden in einer Abfrage gegen die Datenbank geprueft, BEVOR
//     ihnen der Rumpf genommen wird. Ein Block, dessen Speicherung fehlschlug,
//     behaelt seinen Rumpf.
//
//  2. Jeder Getter, der einen Block aus dem Speicher zurueckgibt, laedt den
//     Rumpf eines ausgeduennten Blocks nach -- in eine KOPIE, nie in den
//     DAG-Block selbst (die Getter geben Zeiger in den lebenden Zustand
//     zurueck; siehe tx_batch_pull.go, Punkt 3). Wer also GetBlockByHash,
//     GetBlockByHeight, GetBlocksByHashesForPeer, GetBlocks oder
//     GetBlocksSince ruft, sieht einen vollstaendigen Block, wie bisher.
//
// Ein Marker im Block unterscheidet "ausgeduennt" von "war schon immer
// leer": ein Block ohne Transaktionen hat nichts nachzuladen.

const (
	// Bloecke, die weniger als so viele Hoehen unter der Spitze liegen,
	// behalten ihren Rumpf: sie koennen noch im Replay eines Nachzueglers,
	// in einer Push-Wiederholung oder in einer Neusortierung gebraucht
	// werden. 60 Hoehen sind eine Minute -- laenger dauert davon nichts.
	ausduennAbHoehen = 60
	ausduennTakt     = 10 * time.Second
)

var (
	ausduennBloecke       atomic.Int64 // wie viele Bloecke je ihren Rumpf verloren haben
	ausduennTransakt      atomic.Int64 // und wie viele Transaktionen das waren
	ausduennNachgeladen   atomic.Int64 // wie oft ein Getter nachladen musste
	ausduennNachladFehl   atomic.Int64 // und wie oft das nicht ging
	ausduennNichtInDB     atomic.Int64 // Kandidaten, die die DB nicht kannte -- behalten
	ausduennLaeufe        atomic.Int64
	ausduennLetzteDauerNs atomic.Int64
)

// StarteAusduennen laesst die Ausduennung im Takt laufen.
func (dag *BlockDAG) StarteAusduennen() {
	if dag.state == nil || dag.state.db == nil {
		return
	}
	SafeGoroutine("dagAusduennen", func() {
		t := time.NewTicker(ausduennTakt)
		defer t.Stop()
		for range t.C {
			SafeCall("dagAusduennen-tick", func() { dag.ausduennenEinmal() })
		}
	})
}

func (dag *BlockDAG) ausduennenEinmal() {
	start := time.Now()
	ausduennLaeufe.Add(1)

	// 1. Kandidaten sammeln -- unter der Lesesperre, ohne Datenbank.
	dag.mu.RLock()
	grenze := dag.height - ausduennAbHoehen
	var kandidaten []string
	for hash, b := range dag.blocks {
		if b.Height < grenze && !b.ausgeduennt && len(b.Transactions) > 0 && b.TxRoot != "" && !b.IsGenesis {
			kandidaten = append(kandidaten, hash)
		}
	}
	dag.mu.RUnlock()
	if len(kandidaten) == 0 {
		ausduennLetzteDauerNs.Store(int64(time.Since(start)))
		return
	}

	// 2. Gegen die Datenbank pruefen -- ohne Sperre. Nur was dort liegt, darf
	// seinen Rumpf im Speicher verlieren.
	bestaetigt, ok := ausduennBestaetigung(dag, kandidaten)
	if !ok {
		return
	}
	ausduennNichtInDB.Add(int64(len(kandidaten) - len(bestaetigt)))

	// 3. Ausduennen -- unter der Schreibsperre, kurz: nur Zeiger loeschen.
	var bloecke, txs int64
	dag.mu.Lock()
	for hash := range bestaetigt {
		b := dag.blocks[hash]
		if b == nil || b.ausgeduennt || len(b.Transactions) == 0 {
			continue
		}
		txs += int64(len(b.Transactions))
		b.Transactions = nil
		b.ausgeduennt = true
		bloecke++
	}
	dag.mu.Unlock()
	ausduennBloecke.Add(bloecke)
	ausduennTransakt.Add(txs)
	ausduennLetzteDauerNs.Store(int64(time.Since(start)))
	if bloecke > 0 {
		fmt.Printf("[DAG] 🪶 %d Bloecke unter Hoehe %d ausgeduennt (%d Transaktionen aus dem Speicher; Ruempfe bleiben in der Datenbank)\n", bloecke, grenze, txs)
	}
}

// ausduennBestaetigung fragt die Datenbank, welche der Kandidaten dort
// liegen. Als Variable, damit ein Test ohne Datenbank die Bestaetigung
// stellen kann.
var ausduennBestaetigung = func(dag *BlockDAG, kandidaten []string) (map[string]bool, bool) {
	if dag.state == nil || dag.state.db == nil {
		return nil, false
	}
	bestaetigt := map[string]bool{}
	rows, err := dag.state.db.Query(`SELECT hash FROM chain_blocks WHERE hash = ANY($1)`, pq.Array(kandidaten))
	if err != nil {
		fmt.Printf("[DAG] Ausduennen: Datenbankpruefung fehlgeschlagen, dieser Durchgang tut nichts: %v\n", err)
		return nil, false
	}
	defer rows.Close()
	for rows.Next() {
		var h string
		if rows.Scan(&h) == nil {
			bestaetigt[h] = true
		}
	}
	return bestaetigt, true
}

// hydratisiert liefert den Block mit Rumpf. Ein ausgeduennter kommt als
// KOPIE mit nachgeladenem Rumpf zurueck; der DAG-Block bleibt unberuehrt.
// Ist nichts nachzuladen (nicht ausgeduennt, oder kein Zustand), kommt der
// Block selbst zurueck -- wie bisher.
func (dag *BlockDAG) hydratisiert(b *Block) *Block {
	if b == nil || !b.ausgeduennt || dag.state == nil {
		return b
	}
	ausduennNachgeladen.Add(1)
	// Zuerst der Batch (billig, schon dekodiert im Cache oder eine Zeile),
	// dann der ganze Block aus chain_blocks.
	if txs, ok := dag.state.LoadTxBatch(b.TxRoot); ok && txs != nil {
		kopie := *b
		kopie.Transactions = txs
		kopie.ausgeduennt = false
		return &kopie
	}
	if voll := dag.state.LoadBlockFromDBByHash(b.Hash); voll != nil && len(voll.Transactions) > 0 {
		return voll
	}
	// Sollte nicht vorkommen: ausgeduennt wird nur, was in der DB liegt.
	// Lieber laut als leer ausliefern.
	ausduennNachladFehl.Add(1)
	fmt.Printf("[DAG] ⚠ Rumpf von Block %s (Hoehe %d) liess sich nicht nachladen -- wird ohne Transaktionen ausgeliefert\n", b.Hash[:min(16, len(b.Hash))], b.Height)
	return b
}

// HydratisiertAlle ist hydratisiertAlle fuer Aufrufer ausserhalb des DAG --
// die API-Seitenausgabe ueber GetBlocks(), die nur die ausgelieferte Seite
// nachlaedt.
func (dag *BlockDAG) HydratisiertAlle(bs []*Block) []*Block { return dag.hydratisiertAlle(bs) }

// hydratisiertAlle wendet hydratisiert auf eine Liste an.
func (dag *BlockDAG) hydratisiertAlle(bs []*Block) []*Block {
	for i, b := range bs {
		if b != nil && b.ausgeduennt {
			bs[i] = dag.hydratisiert(b)
		}
	}
	return bs
}

// AusduennStand fuer die Anzeige.
func AusduennStand() map[string]interface{} {
	return map[string]interface{}{
		"bedeutung": "Ruempfe alter Bloecke (aelter als " + fmt.Sprint(ausduennAbHoehen) + " Hoehen) verlassen den " +
			"Speicher-DAG; Koepfe bleiben, die Datenbank hat die Ruempfe. Getter laden nach. " +
			"nachlade_fehler muss 0 sein.",
		"laeufe":                 ausduennLaeufe.Load(),
		"bloecke":                ausduennBloecke.Load(),
		"transaktionen":          ausduennTransakt.Load(),
		"nachgeladen":            ausduennNachgeladen.Load(),
		"nachlade_fehler":        ausduennNachladFehl.Load(),
		"nicht_in_db":            ausduennNichtInDB.Load(),
		"letzte_dauer_ms":        ausduennLetzteDauerNs.Load() / 1e6,
		"ab_hoehen_unter_spitze": ausduennAbHoehen,
	}
}
