package keeper

import (
	"os"
	"strings"
	"testing"
)

// Der Block muss die Anwendungsreihenfolge tragen -- siehe
// pending_reihenfolge.go fuer die Abweichung, die die ID-Reihenfolge
// erzeugt hat (12.09.2026: 14 von 79 Konten eines Buckets auf C1 und C2
// verschieden, 100 uebersprungene Ueberweisungen in zwei Minuten Last).

func TestPendingReihenfolge_ProduceBlockLiestNachWALSeq(t *testing.T) {
	b, err := os.ReadFile("evm_storage.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "WHERE included_at = 0 ORDER BY wal_seq, id LIMIT $1") {
		t.Fatal("LoadPendingTxs ordnet nicht mehr nach wal_seq -- die Bloecke tragen dann wieder die Flush-Reihenfolge, und die Validatoren laufen auseinander")
	}
	f, err := os.ReadFile("transfer_wal.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(f), "INSERT INTO pending_txs (tx_json, created_at, wal_seq) VALUES") {
		t.Fatal("der WAL-Flush schreibt wal_seq nicht mehr")
	}
}

func TestPendingReihenfolge_SeqQuelleOhneWALIstNull(t *testing.T) {
	alt := pendingSeqQuelle.Load()
	pendingSeqQuelle.Store(nil)
	defer func() {
		if alt != nil {
			pendingSeqQuelle.Store(alt)
		}
	}()
	if pendingSeqJetzt() != 0 {
		t.Fatal("ohne WAL muss die Seq 0 sein (ID-Reihenfolge wie bisher)")
	}
	n := uint64(41)
	setzePendingSeqQuelle(func() uint64 { n++; return n })
	if pendingSeqJetzt() != 42 {
		t.Fatal("die Seq-Quelle wird nicht befragt")
	}
}
