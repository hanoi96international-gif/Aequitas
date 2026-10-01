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

// Missbrauch: Wird ohnehin abgewiesen, darf ein Buendel gueltig signierter
// Ueberweisungen keine einzige Signatur-Wiederherstellung kosten.
func TestWALDruck_KeineWiederherstellungBeiAbweisung(t *testing.T) {
	walDruckZuruecksetzen()
	defer walDruckZuruecksetzen()
	inflightZuruecksetzen()
	defer inflightZuruecksetzen()
	noteBlockProduced()
	walWarteschlangeStand.Store(walDruckSchwelle())

	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	var posten []string
	for i := 0; i < 8; i++ {
		raw, _ := signedRawHex(t, 0, testRecipientHex)
		posten = append(posten, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["%s"]}`, i, raw))
	}
	vorher := absenderTreffer.Load() + absenderVerfehlt.Load()
	w := httptest.NewRecorder()
	srv.handleRPC(w, httptest.NewRequest("POST", "/rpc", bytes.NewBufferString("["+strings.Join(posten, ",")+"]")))
	if nachher := absenderTreffer.Load() + absenderVerfehlt.Load(); nachher != vorher {
		t.Fatalf("%d Wiederherstellungen fuer abgewiesene Posten", nachher-vorher)
	}
	var antworten []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &antworten); err != nil || len(antworten) != 8 {
		t.Fatalf("Antwort: %v (%s)", err, w.Body.String())
	}
	for i, a := range antworten {
		e, _ := a["error"].(map[string]interface{})
		if e == nil || e["code"].(float64) != -32005 {
			t.Fatalf("Posten %d: erwartet -32005, bekam %v", i, a)
		}
	}
}

func walTestEintrag(from string, nonce int) walFlushItem {
	return walFlushItem{from: from, tx: Transaction{Roh: fmt.Sprintf("roh-%s-%d", from, nonce)}}
}

// Die Auswahl ueberspringt Absender, die gerade in einem Flush stehen, statt
// abzubrechen -- und laesst deren Eintraege in ihrer Reihenfolge stehen.
func TestWALAuswahl_UeberspringtLaufendeAbsender(t *testing.T) {
	cs := newTestState()
	cs.walFlushMu.Lock()
	defer cs.walFlushMu.Unlock()
	cs.walFlushQueue = []walFlushItem{
		walTestEintrag("x", 1), walTestEintrag("y", 1), walTestEintrag("x", 2),
		walTestEintrag("z", 1), walTestEintrag("y", 2),
	}
	cs.walRohUnterwegs = map[string]int{"x": 1}
	b := cs.walRohAuswahlLocked(100)
	if got := namen(b); got != "y1 z1 y2" {
		t.Fatalf("Buendel %q, erwartet \"y1 z1 y2\"", got)
	}
	if got := namen(cs.walFlushQueue); got != "x1 x2" {
		t.Fatalf("Rest %q, erwartet \"x1 x2\"", got)
	}
}

// Grenze: ein volles Buendel nimmt nichts mehr mit; der Rest behaelt je
// Absender seine Reihenfolge.
func TestWALAuswahl_GrenzeUndReihenfolge(t *testing.T) {
	cs := newTestState()
	cs.walFlushMu.Lock()
	defer cs.walFlushMu.Unlock()
	cs.walFlushQueue = []walFlushItem{
		walTestEintrag("x", 1), walTestEintrag("y", 1), walTestEintrag("z", 1), walTestEintrag("y", 2),
	}
	cs.walRohUnterwegs = map[string]int{"x": 1}
	b := cs.walRohAuswahlLocked(1)
	if got := namen(b); got != "y1" {
		t.Fatalf("Buendel %q", got)
	}
	if got := namen(cs.walFlushQueue); got != "x1 z1 y2" {
		t.Fatalf("Rest %q", got)
	}
}

// Missbrauch der Nebenlaeufigkeit ausgeschlossen: zwei nacheinander
// entnommene Buendel (das erste "laeuft" noch) haben nie einen Absender
// gemeinsam -- sonst koennte eine groessere Nonce vor einer kleineren
// committen.
func TestWALAuswahl_GleichzeitigeBuendelGetrennteAbsender(t *testing.T) {
	cs := newTestState()
	cs.walFlushMu.Lock()
	defer cs.walFlushMu.Unlock()
	for i := 0; i < 50; i++ {
		for _, a := range []string{"a", "b", "c", "d", "e"} {
			cs.walFlushQueue = append(cs.walFlushQueue, walTestEintrag(a, i))
		}
	}
	cs.walRohUnterwegs = map[string]int{"zz": 1} // Auswahlpfad
	erstes := cs.walRohAuswahlLocked(7)
	cs.walRohUnterwegsLocked(erstes, +1)
	zweites := cs.walRohAuswahlLocked(1000)
	in := map[string]bool{}
	for _, it := range erstes {
		in[it.from] = true
	}
	for _, it := range zweites {
		if in[it.from] {
			t.Fatalf("Absender %s in zwei gleichzeitigen Buendeln", it.from)
		}
	}
	// Je Absender: Rest beginnt genau dort, wo das erste Buendel aufhoerte.
	letzte := map[string]string{}
	for _, it := range append(append([]walFlushItem{}, erstes...), cs.walFlushQueue...) {
		if v, ok := letzte[it.from]; ok && v >= it.tx.Roh && len(v) == len(it.tx.Roh) {
			t.Fatalf("Reihenfolge von %s verletzt: %s vor %s", it.from, v, it.tx.Roh)
		}
		letzte[it.from] = it.tx.Roh
	}
}

func namen(q []walFlushItem) string {
	var s []string
	for _, it := range q {
		s = append(s, strings.TrimPrefix(strings.ReplaceAll(it.tx.Roh, "-", ""), "roh"))
	}
	return strings.Join(s, " ")
}
