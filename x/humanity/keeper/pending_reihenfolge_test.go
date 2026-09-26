package keeper

import (
	"os"
	"strings"
	"sync/atomic"
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
	if !strings.Contains(string(f), "INSERT INTO pending_txs (tx_json, created_at, wal_seq)\nSELECT v.tx_json, $2::bigint, v.wal_seq") {
		t.Fatal("der WAL-Flush schreibt wal_seq nicht mehr")
	}
}

func TestPendingReihenfolge_SeqQuelleOhneWALIstNull(t *testing.T) {
	// FIX (Audit 19.09.2026, von -race gefunden): pendingSeqQuelle ist ein
	// PROZESSWEITER Haken, den Produktionscode aufruft (savePendingTxExec).
	// Dieser Test hat ihn mit einer Closure ueberschrieben, die eine gefangene
	// Variable ohne jede Synchronisierung hochzaehlt -- und sie nur dann
	// zurueckgesetzt, wenn vorher schon eine Quelle gesetzt WAR. Beim
	// gewoehnlichen Lauf ist sie das nicht, also blieb die Closure danach fuer
	// den Rest des Prozesses stehen. Jede nebenlaeufige Ueberweisung eines
	// SPAETEREN Tests lief dann hinein: ein Datenrennen, das -race an voellig
	// anderer Stelle meldete, und das die Seq-Reihenfolge fremder Tests
	// nebenbei verfaelschte.
	//
	// Zwei Dinge geradegezogen: immer zuruecksetzen (auch auf nil), und die
	// Testquelle selbst threadsicher machen -- ein Nachzuegler aus einem
	// anderen Test darf kein Rennen ausloesen, falls diese Reihenfolge je
	// wieder kippt.
	alt := pendingSeqQuelle.Load()
	t.Cleanup(func() { pendingSeqQuelle.Store(alt) })

	pendingSeqQuelle.Store(nil)
	if pendingSeqJetzt() != 0 {
		t.Fatal("ohne WAL muss die Seq 0 sein (ID-Reihenfolge wie bisher)")
	}
	var n atomic.Uint64
	n.Store(41)
	setzePendingSeqQuelle(func() uint64 { return n.Add(1) })
	if pendingSeqJetzt() != 42 {
		t.Fatal("die Seq-Quelle wird nicht befragt")
	}
}
