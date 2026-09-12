package keeper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// push_gzip.go: der Push traegt den Rumpf gepackt statt Kopf plus Rueckfrage.

func TestGzipPush_PayloadRoundtrip(t *testing.T) {
	block := &Block{Height: 7, Hash: "abc", Transactions: make([]Transaction, 3000)}
	for i := range block.Transactions {
		block.Transactions[i] = Transaction{Type: "transfer", Wallet: "0xfrom", To: "0xto", Amount: 1}
	}
	data, _ := json.Marshal(block)
	packed, ok := gzipPushPayload(data)
	if !ok {
		t.Fatalf("ein Block mit 3.000 Ueberweisungen (%d Bytes) muss gepackt werden", len(data))
	}
	if len(packed)*4 > len(data) {
		t.Fatalf("Packen brachte fast nichts: %d -> %d Bytes", len(data), len(packed))
	}

	req := httptest.NewRequest(http.MethodPost, "/api/blocks/push", bytes.NewReader(packed))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	body, tooLarge, err := readPushBody(rec, req, int64(len(data))+1024)
	if err != nil || tooLarge {
		t.Fatalf("gepackter Rumpf muss lesbar sein: err=%v tooLarge=%v", err, tooLarge)
	}
	if !bytes.Equal(body, data) {
		t.Fatalf("entpackt != gesendet (%d vs %d Bytes)", len(body), len(data))
	}
}

func TestGzipPush_KleinerBlockBleibtUngepackt(t *testing.T) {
	data, _ := json.Marshal(&Block{Height: 1, Hash: "x"})
	if _, ok := gzipPushPayload(data); ok {
		t.Fatalf("ein leerer Block (%d Bytes) darf nicht gepackt werden", len(data))
	}
}

// Der Deckel gilt fuer das ENTPACKTE: eine Gzip-Bombe (wenige KB, die zu
// vielen MB werden) darf den Handler nicht zum Allokieren zwingen.
func TestGzipPush_DeckelGiltFuerDasEntpackte(t *testing.T) {
	data := []byte(strings.Repeat("a", 4<<20)) // 4 MB, packt auf wenige KB
	packed, ok := gzipPushPayload(data)
	if !ok {
		t.Fatal("packen")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/blocks/push", bytes.NewReader(packed))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	_, tooLarge, err := readPushBody(rec, req, 1<<20)
	if err != nil {
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
	if !tooLarge {
		t.Fatalf("4 MB entpackt bei 1 MB Deckel muss als zu gross gelten (gepackt waren es nur %d Bytes)", len(packed))
	}
}

func TestGzipPush_UngepackterPushGehtWieBisher(t *testing.T) {
	data := []byte(`{"height":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/blocks/push", bytes.NewReader(data))
	rec := httptest.NewRecorder()
	body, tooLarge, err := readPushBody(rec, req, 1<<20)
	if err != nil || tooLarge || !bytes.Equal(body, data) {
		t.Fatalf("ohne Content-Encoding muss der Rumpf unveraendert ankommen: %v %v %q", err, tooLarge, body)
	}
}

// Faehigkeit: ohne Antwort-Header kein gepackter Push -- und ein Partner, der
// den Header nicht mehr schickt, bekommt ab der naechsten Antwort wieder den
// alten Weg.
func TestGzipPush_FaehigkeitWirdAusDerAntwortGelernt(t *testing.T) {
	peer := "http://gzip-test-peer.invalid"
	if gzipPushPeerSupports(peer) {
		t.Fatal("unbekannter Partner darf nie gepackt bekommen")
	}
	recordGzipPushCapability(peer, true)
	if !gzipPushPeerSupports(peer) {
		t.Fatal("nach einer Antwort mit Header muss gepackt werden")
	}
	recordGzipPushCapability(peer, false)
	if gzipPushPeerSupports(peer) {
		t.Fatal("nach einer Antwort ohne Header muss der alte Weg gelten")
	}
}

// Der Handler muss den Header auf jeder Antwort setzen -- sonst lernt der
// Sender die Faehigkeit nie.
func TestGzipPush_AblehnungWegenRumpfErkannt(t *testing.T) {
	if !resp0AblehnungWegenRumpf(blockPushResponse{Reason: "invalid block JSON"}) {
		t.Fatal("'invalid block JSON' ist die Antwort eines Partners, der Gzip nicht liest")
	}
	if resp0AblehnungWegenRumpf(blockPushResponse{Reason: "orphaned, within grace period"}) {
		t.Fatal("eine inhaltliche Ablehnung ist kein Grund, ungepackt nachzuschieben")
	}
}
