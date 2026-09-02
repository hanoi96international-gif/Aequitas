package keeper

import (
	"os"
	"strings"
	"testing"
)

// A resync must not be defeated by how long the node has been running.
//
// On 2026-08-21 Contabo1 diverged and could not get back. Every resync attempt
// -- the boot-time one and the in-process auto-heal alike -- died at the same
// step, and the only place it surfaced was degraded_reason:
//
//	resync failed: resync: could not clear chain_blocks:
//	pq: canceling statement due to statement timeout (57014)
//
// `DELETE FROM chain_blocks` walks every row and writes an equal volume of WAL.
// At 4.38 million blocks it cannot finish inside statement_timeout, so the
// transaction rolled back and the node stayed diverged -- while answering
// /api/status and looking healthy the whole time.
//
// The property that matters is not "TRUNCATE is used" for its own sake: it is
// that clearing these tables costs the same whether the node is a day old or a
// year old. A row-by-row DELETE does not have that property and a fresh
// database will never reveal it, which is why this reads the source instead of
// exercising the path -- reproducing it needs millions of rows and a real
// Postgres, and by the time it reproduces, a validator is already down.
func TestResyncClearsBlocksInConstantTime(t *testing.T) {
	src, err := os.ReadFile("snapshot.go")
	if err != nil {
		t.Fatalf("could not read snapshot.go: %v", err)
	}
	content := strings.ReplaceAll(string(src), "\r\n", "\n")

	start := strings.Index(content, "func (cs *ChainState) ResyncFromSnapshotURL")
	if start < 0 {
		start = 0
	}
	body := content[start:]

	// VERSCHAERFT 02.09.2026: die geschuetzte Eigenschaft ist nicht "TRUNCATE
	// steht da", sondern "das Leeren kostet gleich viel, ob der Knoten einen
	// Tag oder ein Jahr alt ist". Ein UNBEGRENZTES DELETE verletzt das. Ein
	// DELETE mit WHERE auf den abweichenden Schwanz oberhalb der
	// Snapshot-Hoehe nicht: dessen Groesse haengt an der Divergenztiefe,
	// nicht am Alter des Knotens.
	//
	// Warum diese Unterscheidung noetig wurde: chain_blocks mitzuleeren hat am
	// 02.09.2026 die eigentliche Instabilitaet verursacht. Nach mehreren
	// Resyncs hielten die Knoten noch 580 beziehungsweise 21 Bloecke; ein
	// zurueckgefallener Peer bekam auf jede Anfrage nach einem Elternblock
	// {"error":"block not found"} und konnte nie wieder aufholen. Die
	// Historie MUSS also erhalten bleiben -- aber weiterhin in konstanter
	// Zeit geleert werden, wo geleert wird.
	for _, table := range []string{"chain_accounts", "nullifiers", "bio_registrations"} {
		if strings.Contains(body, "Exec(`DELETE FROM "+table) {
			t.Errorf("the resync path clears %s with DELETE.\n"+
				"  That is O(rows) and cannot finish inside statement_timeout on a node with "+
				"millions of rows -- the transaction rolls back, the node stays diverged, and "+
				"it keeps reporting healthy while being unable to rejoin the chain.\n"+
				"  Use TRUNCATE, which unlinks the file in constant time.", table)
		}
	}

	// chain_blocks darf geloescht werden -- aber nur BEGRENZT.
	if strings.Contains(body, "Exec(`DELETE FROM chain_blocks`") ||
		strings.Contains(body, "Exec(`DELETE FROM chain_blocks `") {
		t.Error("the resync clears chain_blocks with an UNBOUNDED DELETE. That is O(rows) " +
			"and dies in statement_timeout on an old node. Delete only the diverged tail " +
			"(WHERE height > the snapshot height) or TRUNCATE.")
	}
	if strings.Contains(body, "Exec(`DELETE FROM chain_blocks WHERE height >") {
		// Der gewollte Fall. Er muss an die Snapshot-Hoehe gebunden sein --
		// ohne sie gaebe es keinen sicheren Schnitt, und ein Platzhalter
		// waere ein unbegrenztes DELETE in Verkleidung.
		if !strings.Contains(body, "snap.Height > 0") {
			t.Error("chain_blocks wird begrenzt geloescht, aber ohne Pruefung auf eine " +
				"gueltige Snapshot-Hoehe. Ohne sie gibt es keinen sicheren Schnitt.")
		}
	} else if !strings.Contains(body, "TRUNCATE chain_blocks") {
		t.Error("der Resync raeumt chain_blocks weder begrenzt noch per TRUNCATE. Wenn das " +
			"woanders hin gewandert ist, diesen Test umhaengen statt loeschen: der Fehler, " +
			"den er abfaengt, zeigt sich erst, wenn ein lang laufender Validator resyncen " +
			"muss -- also genau dann, wenn niemand Zeit zum Diagnostizieren hat.")
	}

	// Und die Historie muss erhalten bleiben: ein TRUNCATE von chain_blocks
	// zusammen mit den Zustandstabellen nimmt dem Knoten die Faehigkeit,
	// einem zurueckgefallenen Peer zu helfen.
	if strings.Contains(body, "TRUNCATE chain_blocks, chain_accounts") {
		t.Error("chain_blocks wird wieder zusammen mit den Zustandstabellen geleert. " +
			"Genau das hat am 02.09.2026 die Instabilitaet verursacht: die Knoten hielten " +
			"danach 580 bzw. 21 Bloecke, ein zurueckgefallener Peer bekam auf jede Anfrage " +
			"nach einem Elternblock \"block not found\" und konnte nie wieder aufholen.")
	}

	if !strings.Contains(body, "SET LOCAL statement_timeout = 0") {
		t.Error("the resync transaction no longer lifts statement_timeout. TRUNCATE alone is " +
			"fast, but the rest of this transaction re-inserts the whole snapshot, and the " +
			"timeout that killed the DELETE applies to those statements too. SET LOCAL reverts " +
			"at the end of the transaction, so nothing else on the connection is affected.")
	}
}
