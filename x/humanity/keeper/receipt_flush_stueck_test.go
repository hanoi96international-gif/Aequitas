package keeper

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	_ "github.com/lib/pq"
)

// Der Vorfall vom 12.09.2026 auf C2: EIN Puffer-INSERT mit 1.525.615 Zeilen
// lief in das 5-s-Zeitlimit, alles wurde wieder eingereiht, und ab da
// scheiterte jeder Anlauf gleich -- 398-mal in 90 Minuten, 384 MB Heap je
// Anlauf. Diese Tests halten fest, was das abstellt: Stuecke, und bei einem
// Fehler bleibt nur das Ungeschriebene liegen.

func neuerReceiptStateMitScheinDB(t *testing.T) *ChainState {
	t.Helper()
	// sql.Open verbindet nicht; ein Handle reicht, damit flushTxReceipts
	// nicht am nil-db vorbeigeht. Die Nahtstelle unten faengt jeden Schreibversuch ab.
	db, err := sql.Open("postgres", "postgres://nirgends/nichts?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &ChainState{db: db}
}

func TestReceiptFlush_SchreibtInStueckenUndBehaeltNurDasUngeschriebene(t *testing.T) {
	cs := neuerReceiptStateMitScheinDB(t)
	for i := 0; i < 12_000; i++ {
		cs.receiptBufMu.Lock()
		if cs.receiptBuf == nil {
			cs.receiptBuf = map[string]pendingReceipt{}
		}
		cs.receiptBuf[fmt.Sprintf("0x%06d", i)] = pendingReceipt{txHash: fmt.Sprintf("0x%06d", i), status: "0x1"}
		cs.receiptBufMu.Unlock()
	}

	var aufrufe []int
	gesamt := 0
	alt := receiptSchreibeFn
	receiptSchreibeFn = func(cs *ChainState, rows []pendingReceipt) error {
		aufrufe = append(aufrufe, len(rows))
		gesamt++
		if gesamt == 2 { // genau der zweite Schreibversuch scheitert
			return errors.New("canceling statement due to statement timeout")
		}
		return nil
	}
	defer func() { receiptSchreibeFn = alt }()

	fehlerVorher := receiptFlushFehler.Load()
	cs.flushTxReceipts()

	if len(aufrufe) != 2 || aufrufe[0] != receiptFlushChunk || aufrufe[1] != receiptFlushChunk {
		t.Fatalf("erwartet zwei Stuecke je %d (das zweite scheitert), bekommen %v", receiptFlushChunk, aufrufe)
	}
	cs.receiptBufMu.Lock()
	rest := len(cs.receiptBuf)
	cs.receiptBufMu.Unlock()
	if rest != 12_000-receiptFlushChunk {
		t.Fatalf("nach dem Fehler muessen genau die %d ungeschriebenen Quittungen im Puffer liegen, nicht %d", 12_000-receiptFlushChunk, rest)
	}
	if receiptFlushFehler.Load() != fehlerVorher+1 {
		t.Fatalf("ein gescheitertes Stueck muss als ein Fehler zaehlen")
	}

	// Naechster Anlauf: der Rest geht in zwei Stuecken durch, der Puffer ist leer.
	aufrufe = nil
	cs.flushTxReceipts()
	if len(aufrufe) != 2 || aufrufe[0]+aufrufe[1] != 12_000-receiptFlushChunk {
		t.Fatalf("der Rest muss in Stuecken nachgeschrieben werden, bekommen %v", aufrufe)
	}
	cs.receiptBufMu.Lock()
	rest = len(cs.receiptBuf)
	cs.receiptBufMu.Unlock()
	if rest != 0 {
		t.Fatalf("Puffer muss leer sein, haelt %d", rest)
	}
}

func TestReceiptBuffer_DeckelVerdraengtStattZuWachsen(t *testing.T) {
	cs := &ChainState{}
	alt := receiptBufMax
	receiptBufMax = 100
	defer func() { receiptBufMax = alt }()

	vorher := receiptVerworfen.Load()
	for i := 0; i < 250; i++ {
		cs.bufferTxReceipt(pendingReceipt{txHash: fmt.Sprintf("0x%04d", i), status: "0x1"})
	}
	cs.receiptBufMu.Lock()
	n := len(cs.receiptBuf)
	_, letzteDa := cs.receiptBuf["0x0249"]
	cs.receiptBufMu.Unlock()
	if n != 100 {
		t.Fatalf("der Puffer darf den Deckel nicht ueberschreiten: %d statt 100", n)
	}
	if !letzteDa {
		t.Fatalf("die juengste Quittung muss im Puffer sein -- verdraengt wird Altes, nicht Neues")
	}
	if receiptVerworfen.Load()-vorher != 150 {
		t.Fatalf("150 Verdraengungen erwartet, gezaehlt %d", receiptVerworfen.Load()-vorher)
	}
}
