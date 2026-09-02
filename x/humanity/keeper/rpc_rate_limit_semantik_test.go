package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Der Begrenzer bucht JE POSTEN ab, nicht je Anfrage.
//
// # WARUM DAS EIN TEST SEIN MUSS UND KEIN KOMMENTAR
//
// Genau diese Aussage stand von 2026-07-24 bis 2026-08-29 falsch im Kommentar
// zu rpcRateLimitMax: dort hiess es, ein Buendel aus 100 Ueberweisungen koste
// einen Tick, eine Quelle komme also auf 2.000 Ueberweisungen/s. Der P1-Fix
// vom 2026-07-21 hatte das laengst auf "je Posten" umgestellt -- der Kommentar
// wanderte nicht mit.
//
// Der Schaden war nicht theoretisch: am 29.08.2026 deckelte der Begrenzer eine
// ganze Messreihe auf 13-15 TPS, waehrend die Zahlen aus dem alten Kommentar
// 2.000 versprachen. Wer aus dem falschen Absatz den Wert fuer einen 10k-Lauf
// ableitet, setzt ihn hundertfach zu niedrig und schreibt der Kette zu, was
// der Begrenzer getan hat.
//
// Ein Kommentar kann still veralten. Dieser Test kann es nicht.
func TestRatenbegrenzer_BuchtJePostenNichtJeAnfrage(t *testing.T) {
	alt := rpcRateLimitMax
	rpcRateLimitMax = 10
	defer func() { rpcRateLimitMax = alt }()
	rpcRateLimit.Range(func(k, _ interface{}) bool { rpcRateLimit.Delete(k); return true })

	// Die Warteschlangen-Schranke antwortet mit DEMSELBEN Code -32005. Ohne
	// diesen Reset zaehlt der Test ihre Ablehnungen als Begrenzung mit -- beim
	// ersten Lauf hat er genau das getan und 25 statt 15 gesehen.
	inflightZuruecksetzen()

	s := &EVMRPCServer{}

	// EIN Buendel mit 25 Posten gegen ein Budget von 10.
	batch := make([]map[string]interface{}, 25)
	for i := range batch {
		batch[i] = map[string]interface{}{
			"jsonrpc": "2.0", "id": i, "method": "eth_chainId", "params": []interface{}{},
		}
	}
	roh, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(roh))
	req.RemoteAddr = "203.0.113.7:1234"
	w := httptest.NewRecorder()
	s.handleRPC(w, req)

	var antworten []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &antworten); err != nil {
		t.Fatalf("Antwort unlesbar: %v -- %s", err, w.Body.String())
	}
	if len(antworten) != 25 {
		t.Fatalf("%d Antworten, erwartet 25", len(antworten))
	}

	begrenzt := 0
	for _, a := range antworten {
		if e, ok := a["error"].(map[string]interface{}); ok {
			code, _ := e["code"].(float64)
			nachricht, _ := e["message"].(string)
			// Auf die NACHRICHT pruefen, nicht nur auf den Code: die
			// Warteschlangen-Schranke aus inflight_grenze.go benutzt
			// denselben -32005, meint aber etwas anderes.
			if int(code) == -32005 && strings.Contains(nachricht, "rate limited") {
				begrenzt++
			}
			if strings.Contains(nachricht, "in flight") {
				t.Fatalf("die Warteschlangen-Schranke hat geantwortet, nicht der Begrenzer: %q", nachricht)
			}
		}
	}

	// Je Anfrage abgebucht waere begrenzt == 0 (ein Tick von zehn).
	// Je Posten abgebucht sind die ersten 10 frei und 15 abgelehnt.
	if begrenzt == 0 {
		t.Fatal("kein Posten wurde begrenzt -- der Begrenzer bucht offenbar wieder JE ANFRAGE ab. " +
			"Das oeffnet das Schlupfloch aus dem P1-Fix vom 2026-07-21 (rpcRateLimitMax x " +
			"maxBatchSize statt rpcRateLimitMax) und macht jede daraus abgeleitete " +
			"Durchsatzrechnung um den Faktor maxBatchSize falsch")
	}
	if begrenzt != 15 {
		t.Fatalf("%d von 25 Posten begrenzt, erwartet 15 (Budget 10, danach jeder weitere). "+
			"Die Rechnung 'Zielrate x Fensterlaenge = Mindestwert' haengt genau daran", begrenzt)
	}
}

// Die Rechnung, die ein Betreiber vor einem Lastlauf anstellt, hier einmal
// festgehalten -- damit sie nicht jedes Mal neu aus einem Prosaabsatz
// abgeleitet werden muss.
func TestRatenbegrenzer_RechnungFuerEinenLastlauf(t *testing.T) {
	fensterSekunden := int(rpcRateLimitWindow.Seconds())
	if fensterSekunden != 10 {
		t.Fatalf("Fenster ist %d s -- die Rechnung unten geht von 10 aus", fensterSekunden)
	}
	faelle := []struct {
		zielTPS int
		mindest int
	}{
		{20, 200},       // die Vorgabe leistet genau das
		{1000, 10000},   // und NICHT 1000 -- der haeufige Irrtum
		{10000, 100000}, // das 10k-Ziel
	}
	for _, f := range faelle {
		got := f.zielTPS * fensterSekunden
		if got != f.mindest {
			t.Fatalf("fuer %d TPS ergibt die Rechnung %d, erwartet %d", f.zielTPS, got, f.mindest)
		}
	}
	// Und die Vorgabe deckt eben nur 20/s ab, nicht 2.000.
	if proSekunde := rpcRateLimitMaxDefault / fensterSekunden; proSekunde != 20 {
		t.Fatalf("Vorgabe deckt %d Ueberweisungen/s ab, erwartet 20 -- %s",
			proSekunde, fmt.Sprint("der alte Kommentar behauptete 2.000"))
	}
}
