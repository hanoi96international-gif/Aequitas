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
	inMap, rueckstand, _ := cs.receiptPufferStand()
	if inMap != 0 || rueckstand != 12_000-receiptFlushChunk {
		t.Fatalf("nach dem Fehler muessen genau die %d ungeschriebenen Quittungen als Rueckstand liegen (Map leer), nicht map=%d rueckstand=%d", 12_000-receiptFlushChunk, inMap, rueckstand)
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
	inMap, rueckstand, inArbeit := cs.receiptPufferStand()
	if inMap+rueckstand+inArbeit != 0 {
		t.Fatalf("Puffer muss leer sein, haelt map=%d rueckstand=%d in_arbeit=%d", inMap, rueckstand, inArbeit)
	}
}

// 14.09.2026: der Rueckstand darf beim naechsten Anlauf keine neue Map
// kosten, muss VOR den neuen Quittungen geschrieben werden, und eine
// Quittung, die inzwischen neuer hereinkam, gewinnt -- ohne in einer
// Anweisung doppelt aufzutauchen.
func TestReceiptFlush_RueckstandZuerstOhneDoppel(t *testing.T) {
	cs := neuerReceiptStateMitScheinDB(t)
	alt := receiptSchreibeFn
	defer func() { receiptSchreibeFn = alt }()

	for i := 0; i < 3; i++ {
		cs.bufferTxReceipt(pendingReceipt{txHash: fmt.Sprintf("0x%02d", i), status: "0x1"})
	}
	receiptSchreibeFn = func(cs *ChainState, rows []pendingReceipt) error { return errors.New("timeout") }
	cs.flushTxReceipts()
	if _, r, _ := cs.receiptPufferStand(); r != 3 {
		t.Fatalf("drei Quittungen muessen als Rueckstand liegen, nicht %d", r)
	}
	// Eine davon kommt neuer herein (Status geaendert), dazu eine vierte.
	cs.bufferTxReceipt(pendingReceipt{txHash: "0x01", status: "0x0"})
	cs.bufferTxReceipt(pendingReceipt{txHash: "0x03", status: "0x1"})
	if r, ok := cs.lookupBufferedReceipt("0x00"); !ok || r.status != "0x1" {
		t.Fatal("eine Quittung im Rueckstand muss auffindbar bleiben")
	}
	if r, ok := cs.lookupBufferedReceipt("0x01"); !ok || r.status != "0x0" {
		t.Fatal("die neuere Fassung in der Map muss gewinnen")
	}

	var geschrieben []pendingReceipt
	receiptSchreibeFn = func(cs *ChainState, rows []pendingReceipt) error {
		geschrieben = append(geschrieben, rows...)
		return nil
	}
	cs.flushTxReceipts()
	if len(geschrieben) != 4 {
		t.Fatalf("vier verschiedene Quittungen erwartet, geschrieben %d", len(geschrieben))
	}
	gesehen := map[string]string{}
	for _, r := range geschrieben {
		if _, doppelt := gesehen[r.txHash]; doppelt {
			t.Fatalf("Quittung %s zweimal in einem Anlauf -- ON CONFLICT DO UPDATE wuerde das abweisen", r.txHash)
		}
		gesehen[r.txHash] = r.status
	}
	// Der Rueckstand stammt aus einer Map, seine innere Reihenfolge ist
	// zufaellig -- nur "Rueckstand vor Neuem" ist die Zusage.
	vorne := map[string]bool{geschrieben[0].txHash: true, geschrieben[1].txHash: true}
	if !vorne["0x00"] || !vorne["0x02"] {
		t.Fatalf("Rueckstand (ohne die ueberholte 0x01) muss zuerst kommen, Reihenfolge: %v", geschrieben)
	}
	if gesehen["0x01"] != "0x0" {
		t.Fatal("die neuere Fassung von 0x01 muss geschrieben werden")
	}
	if m, r, a := cs.receiptPufferStand(); m+r+a != 0 {
		t.Fatalf("Puffer muss leer sein: map=%d rueckstand=%d in_arbeit=%d", m, r, a)
	}
}

// Der Deckel zaehlt Map UND Rueckstand; verdraengt wird das Aelteste, also
// zuerst aus dem Rueckstand.
func TestReceiptBuffer_DeckelZaehltRueckstandMit(t *testing.T) {
	cs := &ChainState{}
	alt := receiptBufMax
	receiptBufMax = 4
	defer func() { receiptBufMax = alt }()
	cs.receiptRest = []pendingReceipt{{txHash: "0xa0"}, {txHash: "0xa1"}, {txHash: "0xa2"}}
	cs.bufferTxReceipt(pendingReceipt{txHash: "0xb0"}) // 4: voll, nichts verdraengt
	cs.bufferTxReceipt(pendingReceipt{txHash: "0xb1"}) // verdraengt 0xa0
	m, r, _ := cs.receiptPufferStand()
	if m != 2 || r != 2 {
		t.Fatalf("erwartet map=2 rueckstand=2, bekommen map=%d rueckstand=%d", m, r)
	}
	if _, ok := cs.lookupBufferedReceipt("0xa0"); ok {
		t.Fatal("die aelteste Quittung des Rueckstands muss verdraengt sein")
	}
	if _, ok := cs.lookupBufferedReceipt("0xa1"); !ok {
		t.Fatal("die zweitaelteste muss noch da sein")
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
