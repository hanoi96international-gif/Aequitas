package keeper

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

// Grundlagen fuer Clients (App-Neubau 2026-09-30, docs/NEUBAU_ANALYSE.md der
// App, Abschnitt 2 "Backend-Luecken").
//
// # Retry-After
//
// Die Kette antwortete auf Ueberlast mit 429 oder 503, aber nirgends mit
// Retry-After. Die App raet deshalb feste Wartezeiten -- zu kurz, und sie
// haemmert weiter; zu lang, und der Mensch wartet grundlos. retryAfterMiddleware
// setzt den Kopf bei jeder 429/503-Antwort, die ihn nicht selbst setzt. Die
// Werte sind bewusst konservativ und fest: sie verraten nichts ueber den
// Zustand der Grenze, die sie ausloest.
//
// # Netzkennung
//
// Beim Neustart der Kette bei null bleibt die Chain-ID gleich, alles andere
// (Registrierungen, Guthaben) ist weg. Eine App mit gespeicherten Staenden
// muss das erkennen koennen. netzKennung leitet sich aus Chain-ID und
// Genesis-Zeit ab (genesis.json) -- auf jedem Knoten gleich, auch auf einem,
// der per Snapshot gestartet ist und den Genesis-Block nicht haelt.
//
// Die Serverzeit liefert net/http ohnehin in jeder Antwort (Date-Kopf); die
// App gleicht ihre Uhr darueber ab (Tausch/Liquiditaet verlangen +-60 s).

const (
	retryAfter429Sekunden = 10
	retryAfter503Sekunden = 5
)

// retryAfterWriter setzt Retry-After bei 429/503, falls der Handler es nicht
// selbst getan hat.
type retryAfterWriter struct {
	http.ResponseWriter
}

func (w retryAfterWriter) WriteHeader(code int) {
	if w.Header().Get("Retry-After") == "" {
		switch code {
		case http.StatusTooManyRequests:
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter429Sekunden))
		case http.StatusServiceUnavailable:
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter503Sekunden))
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

// Flush: /api/events (SSE) prueft auf http.Flusher -- die Huelle darf das
// nicht verdecken.
func (w retryAfterWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap: http.NewResponseController (SetWriteDeadline in /api/events)
// erreicht darueber den echten Writer.
func (w retryAfterWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func retryAfterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(retryAfterWriter{w}, r)
	})
}

// netzKennung: "aequitas-<chainID>-<genesis-unix>". Einmal je Prozess
// gelesen -- /api/status wird oft abgefragt, genesis.json aendert sich nur mit
// einem Neustart der Kette (und damit des Knotens).
var netzKennung = sync.OnceValue(func() string {
	return fmt.Sprintf("aequitas-%d-%d", aequitasChainID.Int64(), genesisTimestamp())
})
