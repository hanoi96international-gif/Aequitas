package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func walDruckZuruecksetzen() {
	walWarteschlangeStand.Store(0)
	walWarteschlangeAbgelehnt.Store(0)
}

// Unter der Schwelle nimmt der Knoten an, ab der Schwelle weist er ab.
func TestWALDruck_SchwelleGreift(t *testing.T) {
	walDruckZuruecksetzen()
	defer walDruckZuruecksetzen()
	noteBlockProduced()
	walWarteschlangeStand.Store(walDruckSchwelle() - 1)
	if g := walDruckGrund(); g != "" {
		t.Fatalf("unter der Schwelle abgewiesen: %q", g)
	}
	walWarteschlangeStand.Store(walDruckSchwelle())
	if g := admissionRefusalReason(); !strings.Contains(g, "still being written") {
		t.Fatalf("an der Schwelle nicht abgewiesen: %q", g)
	}
}

// Missbrauch / Fehlerfall: unter WAL-Druck darf ein Buendel gueltig
// signierter Ueberweisungen KEINE Nonce reservieren -- sonst bekaeme die
// Wallet beim Wiederholen "nonce too low" (eine Freigabe gibt es nicht).
// Jeder Posten antwortet -32005.
func TestWALDruck_KeineNonceReservierungBeiAbweisung(t *testing.T) {
	walDruckZuruecksetzen()
	defer walDruckZuruecksetzen()
	inflightZuruecksetzen()
	defer inflightZuruecksetzen()
	noteBlockProduced()
	walWarteschlangeStand.Store(walDruckSchwelle())

	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	var posten []string
	var absender []string
	for i := 0; i < 6; i++ {
		raw, s := signedRawHex(t, 0, testRecipientHex)
		absender = append(absender, s)
		posten = append(posten, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["%s"]}`, i, raw))
	}
	w := httptest.NewRecorder()
	srv.handleRPC(w, httptest.NewRequest("POST", "/rpc", bytes.NewBufferString("["+strings.Join(posten, ",")+"]")))
	var antworten []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &antworten); err != nil {
		t.Fatalf("keine JSON-Liste: %v (%s)", err, w.Body.String())
	}
	for i, a := range antworten {
		e, _ := a["error"].(map[string]interface{})
		if e == nil || e["code"].(float64) != -32005 {
			t.Fatalf("Posten %d: erwartet -32005, bekam %v", i, a)
		}
	}
	for _, s := range absender {
		sh := srv.nonceShardFor(s)
		sh.mu.Lock()
		n := sh.nonces[s]
		sh.mu.Unlock()
		if n != 0 {
			t.Fatalf("Nonce von %s reserviert (%d), obwohl abgewiesen", s, n)
		}
	}
}

// Ein laufender voller Flush wird nicht ein zweites Mal angehaengt.
func TestWALDruck_KeinZweiterVollerFlushHintendran(t *testing.T) {
	cs := newTestState()
	cs.walFlushNowMu.Lock()
	if cs.versucheFlushWALNow() {
		cs.walFlushNowMu.Unlock()
		t.Fatal("versucheFlushWALNow lief, obwohl schon ein voller Flush lief")
	}
	cs.walFlushNowMu.Unlock()
	if !cs.versucheFlushWALNow() {
		t.Fatal("ohne laufenden Flush muss versucheFlushWALNow laufen")
	}
}

// Fehlerfall: wird die Warteschlange verworfen (ueberholter Leiter), darf der
// Druckzaehler nicht auf dem alten Stand stehen bleiben -- sonst wiese die
// Annahme dauerhaft ab, obwohl nichts mehr wartet.
func TestWALDruck_VerwerfenSetztZaehlerZurueck(t *testing.T) {
	walDruckZuruecksetzen()
	defer walDruckZuruecksetzen()
	cs := newTestState()
	cs.walFlushMu.Lock()
	cs.walFlushQueue = make([]walFlushItem, 3)
	cs.walFlushMu.Unlock()
	walWarteschlangeStand.Store(walDruckSchwelle())
	if wal, _ := cs.leitungUnverteiltVerwerfen(); wal != 3 {
		t.Fatalf("verworfen %d, erwartet 3", wal)
	}
	if n := walWarteschlangeStand.Load(); n != 0 {
		t.Fatalf("Druckzaehler nach dem Verwerfen %d, erwartet 0", n)
	}
}
