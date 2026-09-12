package keeper

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
)

// Der Push traegt den Rumpf wieder selbst -- gepackt.
//
// GEMESSEN AM 12.09.2026 (C2-Log nach dem Lasttest, C1-Bloecke zu 4.900 bis
// 7.000 Ueberweisungen): der gestrippte Push liefert nur den Kopf, und der
// Empfaenger holt den Rumpf in einer ZWEITEN Anfrage -- synchron, im
// Push-Handler, VOR AddPeerBlock. Diese Abholung dauerte je Block 0,4 bis
// 0,7 s (Kodieren auf dem Sender, Uebertragen, Dekodieren, Wurzel pruefen,
// Speichern); das Replay selbst danach meist nur 50-200 ms. Der Partner
// braucht damit rund 1 s je Block -- genau den Takt, in dem C1 produziert.
// Kein Spielraum: unter Last faellt C2 zurueck, geht in den Aufholmodus,
// produziert nicht, und die Peer-Lag-Bremse drosselt daraufhin auch C1.
//
// Der gestrippte Push wurde eingefuehrt, weil ein voller Block als Klartext-
// JSON (1,5 MB bei 7.000 Ueberweisungen) das 3-s-Push-Fenster unter Last
// sprengte. Gepackt sind dieselben 7.000 Ueberweisungen 325 KB -- das passt
// mit weitem Abstand, und der Empfaenger hat den Rumpf sofort, ohne
// Rueckfrage. Bei 50.000 Ueberweisungen (11,6 MB Klartext) waeren es rund
// 2,5 MB gepackt: im Rechenzentrum immer noch Bruchteile einer Sekunde.
//
// Faehigkeit wird wie bei den Ruempfen nach Referenz gelernt: der Empfaenger
// setzt auf JEDE Push-Antwort den Header X-Aequitas-Gzip-Push. Ein Partner
// auf aelterem Code setzt ihn nicht und bekommt weiter, was er versteht
// (gestrippt oder voll, wie bisher). Schlaegt ein gepackter Push fehl, wird
// die Faehigkeit vergessen und derselbe Block sofort auf dem alten Weg
// nachgeschickt -- fail soft, wie beim Strippen.

const gzipPushHeader = "X-Aequitas-Gzip-Push"
const gzipPushToken = "gzip-push-v1"

// gzipPushMinBytes: darunter lohnt das Packen nicht.
const gzipPushMinBytes = 8 << 10

var gzipPushPeerCap sync.Map // peerURL -> bool

var (
	gzipPushGesendet     atomic.Int64
	gzipPushEmpfangen    atomic.Int64
	gzipPushFehlgeschl   atomic.Int64
	gzipPushBytesRoh     atomic.Int64
	gzipPushBytesGepackt atomic.Int64
)

func gzipPushPeerSupports(peerURL string) bool {
	v, ok := gzipPushPeerCap.Load(peerURL)
	if !ok {
		return false
	}
	supports, _ := v.(bool)
	return supports
}

func recordGzipPushCapability(peerURL string, supports bool) {
	prev, existed := gzipPushPeerCap.Load(peerURL)
	gzipPushPeerCap.Store(peerURL, supports)
	if had, _ := prev.(bool); !existed || had != supports {
		if supports {
			fmt.Printf("[PUSH-GZIP] ✓ %s nimmt gepackte Bloecke an -- Pushes tragen den Rumpf wieder selbst\n", peerURL)
		} else if existed {
			fmt.Printf("[PUSH-GZIP] ↩ %s nimmt keine gepackten Bloecke mehr an -- zurueck auf den alten Weg\n", peerURL)
		}
	}
}

// gzipPushPayload packt den vollen Block. false, wenn Packen nicht lohnt.
func gzipPushPayload(data []byte) ([]byte, bool) {
	if len(data) < gzipPushMinBytes {
		return nil, false
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, false
	}
	if _, err := zw.Write(data); err != nil {
		return nil, false
	}
	if err := zw.Close(); err != nil {
		return nil, false
	}
	gzipPushBytesRoh.Add(int64(len(data)))
	gzipPushBytesGepackt.Add(int64(buf.Len()))
	return buf.Bytes(), true
}

// readPushBody liest den Push-Rumpf, gepackt oder nicht. Der Deckel gilt
// fuer BEIDE Formen: fuer die Bytes auf der Leitung und fuer das Entpackte.
func readPushBody(w http.ResponseWriter, r *http.Request, maxBytes int64) (body []byte, tooLarge bool, err error) {
	if r.Header.Get("Content-Encoding") != "gzip" {
		return readBodyLimited(w, r, maxBytes)
	}
	packed, tooLarge, err := readBodyLimited(w, r, maxBytes)
	if tooLarge || err != nil {
		return nil, tooLarge, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, false, err
	}
	defer zr.Close()
	// Ein Byte mehr als erlaubt lesen: kommt es, ist das Entpackte zu gross.
	body, err = io.ReadAll(io.LimitReader(zr, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > maxBytes {
		return nil, true, nil
	}
	gzipPushEmpfangen.Add(1)
	return body, false, nil
}

// resp0AblehnungWegenRumpf erkennt die Ablehnung eines Partners, der den
// gepackten Rumpf nicht lesen konnte: "invalid block JSON" oder "read error"
// -- beides Antworten des Handlers VOR jeder inhaltlichen Pruefung.
func resp0AblehnungWegenRumpf(r blockPushResponse) bool {
	return r.Reason == "invalid block JSON" || r.Reason == "read error"
}

// GzipPushStand fuer /api/health/combined.
func GzipPushStand() map[string]interface{} {
	roh, gepackt := gzipPushBytesRoh.Load(), gzipPushBytesGepackt.Load()
	quote := 0.0
	if roh > 0 {
		quote = float64(gepackt) / float64(roh) * 100
	}
	return map[string]interface{}{
		"bedeutung": "Volle Bloecke gepackt im Push statt Kopf plus Rueckfrage. gesendet/empfangen zaehlen " +
			"gepackte Pushes; fehlgeschlagen = auf den alten Weg zurueckgefallen.",
		"gesendet":       gzipPushGesendet.Load(),
		"empfangen":      gzipPushEmpfangen.Load(),
		"fehlgeschlagen": gzipPushFehlgeschl.Load(),
		"packquote_pct":  quote,
	}
}
