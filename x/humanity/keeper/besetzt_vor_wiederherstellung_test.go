package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// Gemessen am 01.10.2026 (C1-Pruefstand, CPU-Profil): rpcKonten stellte fuer
// die Weiterleitungsentscheidung jede Signatur wieder her -- auch fuer die
// 74 % der Posten, die gleich danach als "too much work in flight" abgewiesen
// wurden. Missbrauch: ein Angreifer schickt bei voller Schranke Buendel
// gueltig signierter Ueberweisungen; jede kostete eine secp256k1-Rechnung.
// Erwartet: alle Posten -32005, und KEINE Wiederherstellung.
func TestBesetzt_KeineWiederherstellungBeiVollerSchranke(t *testing.T) {
	for _, mitLeitung := range []bool{false, true} {
		t.Run(fmt.Sprintf("leitung=%v", mitLeitung), func(t *testing.T) {
			t.Setenv(inflightGrenzeEnv, "100")
			inflightZuruecksetzen()
			defer inflightZuruecksetzen()
			inflightAktuell.Store(100) // voll

			cs := newTestState()
			if mitLeitung {
				cs.leitung.Store(&Leitung{})
				defer cs.leitung.Store(nil)
			}
			srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)

			const n = 10
			posten := make([]string, 0, n)
			for i := 0; i < n; i++ {
				raw, _ := signedRawHex(t, 0, testRecipientHex)
				posten = append(posten, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["%s"]}`, i, raw))
			}
			vorher := absenderTreffer.Load() + absenderVerfehlt.Load()
			req := httptest.NewRequest("POST", "/rpc", bytes.NewBufferString("["+strings.Join(posten, ",")+"]"))
			w := httptest.NewRecorder()
			srv.handleRPC(w, req)

			var antworten []map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &antworten); err != nil {
				t.Fatalf("keine JSON-Liste: %v (%s)", err, w.Body.String())
			}
			if len(antworten) != n {
				t.Fatalf("%d Antworten, erwartet %d", len(antworten), n)
			}
			for i, a := range antworten {
				e, _ := a["error"].(map[string]interface{})
				if e == nil || e["code"].(float64) != -32005 {
					t.Fatalf("Posten %d: erwartet -32005, bekam %v", i, a)
				}
			}
			if nachher := absenderTreffer.Load() + absenderVerfehlt.Load(); nachher != vorher {
				t.Fatalf("bei voller Schranke %d Signaturen wiederhergestellt -- Ablehnung muss vorher kommen", nachher-vorher)
			}
			if inflightAktuell.Load() != 100 {
				t.Fatalf("Schranke veraendert: %d", inflightAktuell.Load())
			}
		})
	}
}
