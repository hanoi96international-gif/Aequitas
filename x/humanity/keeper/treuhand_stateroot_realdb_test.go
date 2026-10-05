package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Treuhand in der StateRoot (treuhand_stateroot.go). Gegen eine echte
// Datenbank, weil die Treuhand nur in escrow_accounts steht.

// stateRootOhneTreuhand: die Formel, wie sie vor der Treuhand galt. Ohne
// Treuhand muss die Wurzel genau so bleiben -- sonst aenderte sich die
// StateRoot jedes bisherigen Blocks.
func stateRootOhneTreuhand(cs *ChainState) string {
	lastUBIAt := cs.getConfigValueDB("last_ubi_at")
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	var sb strings.Builder
	sb.WriteString("acctXOR:")
	sb.WriteString(hex.EncodeToString(cs.accountSetXOR[:]))
	if cs.pool != nil {
		fmt.Fprintf(&sb, "|pool:%d:%d:%d", cs.pool.ReserveAEQ.Micro(), cs.pool.ReserveTUSD.Micro(), cs.pool.TotalLPShares.Micro())
	}
	sb.WriteString("|nullXOR:")
	sb.WriteString(hex.EncodeToString(cs.nullifierSetXOR[:]))
	fmt.Fprintf(&sb, "|ubi:%s", lastUBIAt)
	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:])
}

func (k *treuhandTestKnoten) treuhandSumme() [32]byte {
	k.cs.mu.RLock()
	defer k.cs.mu.RUnlock()
	return k.cs.escrowSetXOR
}

// neuAufgebaut: die Summe, wie ein frisch gestarteter Knoten sie aus der
// Tabelle rechnet -- muss der laufend mitgefuehrten gleichen.
func (k *treuhandTestKnoten) neuAufgebaut() [32]byte {
	k.t.Helper()
	x, err := k.cs.treuhandSummeAusDB()
	if err != nil {
		k.t.Fatal(err)
	}
	return x
}

func TestTreuhandStateRoot_OhneTreuhandUnveraendert_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-root-leer-test.json")
	k.inaktiverMensch(931, 100, 1)
	if got, alt := k.cs.StateRoot(), stateRootOhneTreuhand(k.cs); got != alt {
		t.Fatalf("ohne Treuhand hat sich die StateRoot geaendert: %s statt %s", got, alt)
	}
	if c := k.cs.StateRootComponentBreakdown(); c.EscrowSetXOR != "" {
		t.Fatalf("ohne Treuhand meldet die Aufschluesselung eine Summe: %s", c.EscrowSetXOR)
	}
}

// Verschiebung legt das Blatt hinein, die Freigabe nimmt es heraus; die
// laufende Summe gleicht jederzeit der neu aufgebauten, und die Wurzel traegt
// sie nur, solange es Treuhand gibt.
func TestTreuhandStateRoot_VerschiebungUndFreigabe_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-root-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(932, 100, t1-inactivityEscrowSeconds-10)

	if !k.block(t1, Transaction{Type: "escrow_move", Wallet: w, Amount: 100}) {
		t.Fatal("Verschiebung abgelehnt")
	}
	if got := k.treuhandSumme(); got != treuhandBlatt(w, 100) {
		t.Fatalf("Treuhand-Summe nach der Verschiebung %x, erwartet das Blatt von %s/100", got[:6], w)
	}
	if got := k.neuAufgebaut(); got != k.treuhandSumme() {
		t.Fatalf("neu aufgebaute Summe %x, laufende %x", got[:6], k.treuhandSumme())
	}
	if got, ohne := k.cs.StateRoot(), stateRootOhneTreuhand(k.cs); got == ohne {
		t.Fatal("die StateRoot traegt die Treuhand nicht")
	}
	if c := k.cs.StateRootComponentBreakdown(); c.EscrowSetXOR == "" {
		t.Fatal("die Aufschluesselung zeigt die Treuhand nicht")
	}
	// Ein frisch gestarteter Knoten rechnet dieselbe Wurzel.
	vorher := k.cs.StateRoot()
	k.cs.mu.Lock()
	k.cs.rebuildStateAccumulators()
	k.cs.mu.Unlock()
	if got := k.cs.StateRoot(); got != vorher {
		t.Fatalf("nach dem Neuaufbau andere StateRoot: %s statt %s", got, vorher)
	}

	frei := t1 + escrowToUBISeconds
	if !k.block(frei, Transaction{Type: "escrow_release", Wallet: w, Amount: 100}) {
		t.Fatal("Freigabe abgelehnt")
	}
	if got := k.treuhandSumme(); got != ([32]byte{}) {
		t.Fatalf("nach der Freigabe noch eine Treuhand-Summe: %x", got[:6])
	}
	if got, ohne := k.cs.StateRoot(), stateRootOhneTreuhand(k.cs); got != ohne {
		t.Fatal("ohne Treuhand traegt die Wurzel noch eine Summe")
	}
}

// Missbrauch: ein Block mit einer Verschiebung scheitert -- die Summe geht mit
// zurueck. Ein danach ehrlicher Block legt sie genau einmal an.
func TestTreuhandStateRoot_ZurueckgewiesenerBlock_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-root-rueck-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(933, 30, t1-inactivityEscrowSeconds-10)
	vorher := k.cs.StateRoot()
	if k.block(t1,
		Transaction{Type: "escrow_move", Wallet: w, Amount: 30},
		Transaction{Type: "escrow_move", Wallet: distTestAddr(999998), Amount: 1}, // unbekanntes Konto: harter Fehler
	) {
		t.Fatal("Vorbedingung: der Block muss scheitern")
	}
	if got := k.treuhandSumme(); got != ([32]byte{}) {
		t.Fatalf("zurueckgewiesener Block hat eine Treuhand-Summe hinterlassen: %x", got[:6])
	}
	if got := k.cs.StateRoot(); got != vorher {
		t.Fatalf("StateRoot nach dem Zurueckrollen %s statt %s", got, vorher)
	}
	if !k.block(t1, Transaction{Type: "escrow_move", Wallet: w, Amount: 30}) {
		t.Fatal("ehrlicher Block abgelehnt")
	}
	if got := k.treuhandSumme(); got != treuhandBlatt(w, 30) {
		t.Fatalf("Treuhand-Summe nach dem ehrlichen Block %x", got[:6])
	}
	// Dieselbe Verschiebung noch einmal: die Zeile bleibt, die Summe auch.
	k.block(t1+86400, Transaction{Type: "escrow_move", Wallet: w, Amount: 30})
	if got := k.treuhandSumme(); got != treuhandBlatt(w, 30) {
		t.Fatalf("zweite Verschiebung hat die Summe veraendert: %x", got[:6])
	}
}

// Der Snapshot traegt die Treuhand mit; ohne Treuhand bleibt sein JSON ohne
// das Feld (Signatur byte-gleich).
func TestTreuhandStateRoot_SnapshotTraegtTreuhand_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-root-snap-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(934, 40, t1-inactivityEscrowSeconds-10)
	leer, _ := json.Marshal(k.cs.ExportSnapshot(nil, 1, false))
	if strings.Contains(string(leer), `"treuhand"`) {
		t.Fatal("Snapshot ohne Treuhand traegt das Feld")
	}
	if !k.block(t1, Transaction{Type: "escrow_move", Wallet: w, Amount: 40}) {
		t.Fatal("Verschiebung abgelehnt")
	}
	snap := k.cs.ExportSnapshot(nil, 2, false)
	if len(snap.Treuhand) != 1 || snap.Treuhand[0].Wallet != w || snap.Treuhand[0].Amount != 40 || snap.Treuhand[0].MovedAt != t1 {
		t.Fatalf("Treuhand im Snapshot: %+v", snap.Treuhand)
	}
}

// Das Blatt: Betrag auf Mikro-AEQ, Wallet ohne Gross-/Kleinschreibung; die
// Frist steht nicht darin (Erzeuger und Nachspielende setzen sie verschieden).
func TestTreuhandBlatt(t *testing.T) {
	if treuhandBlatt("0xAB", 1) != treuhandBlatt("0xab", 1) {
		t.Fatal("Blatt haengt von der Schreibweise der Wallet ab")
	}
	if treuhandBlatt("0xab", 1) == treuhandBlatt("0xab", 1.000001) {
		t.Fatal("ein Mikro-AEQ Unterschied aendert das Blatt nicht")
	}
	if treuhandBlatt("0xab", 1) == treuhandBlatt("0xac", 1) {
		t.Fatal("andere Wallet, gleiches Blatt")
	}
}
