package keeper

import (
	"fmt"
	"testing"
)

// Treuhand als Bestand beim Nachspielen (K-2): jeder Nachspielende legt die
// Treuhand-Zeile selbst an und prueft Freigabe und Rueckholung dagegen.
// Braucht escrow_accounts, also eine echte Datenbank.

func treuhandBestandZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range []string{"treuhand_aktiv", "treuhand_lp", "treuhand_zu_frueh",
		"treuhand_doppelt", "treuhand_ohne_bestand", "treuhand_ueber_bestand", "treuhand_unlesbar"} {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

type treuhandTestKnoten struct {
	t   *testing.T
	cs  *ChainState
	dag *BlockDAG
	n   int
}

func neuerTreuhandTestKnoten(t *testing.T, name string) *treuhandTestKnoten {
	t.Helper()
	truncateDistTestTables(t)
	cs := testKnoten(t, name)
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	return &treuhandTestKnoten{t: t, cs: cs, dag: dag}
}

// inaktiverMensch: seit inaktivSeit nicht aktiv, mit Guthaben.
func (k *treuhandTestKnoten) inaktiverMensch(n int, guthaben float64, inaktivSeit int64) string {
	k.t.Helper()
	w := distTestAddr(n)
	k.cs.mu.Lock()
	defer k.cs.mu.Unlock()
	acc := &AccountState{Address: w, IsHuman: true, Balance: NewDecimal(guthaben), LastActivityAt: inaktivSeit}
	if err := k.cs.saveAccountToDB(acc); err != nil {
		k.t.Fatalf("Konto anlegen: %v", err)
	}
	k.cs.accounts.Set(w, acc)
	return w
}

func (k *treuhandTestKnoten) block(zeit int64, txs ...Transaction) bool {
	k.t.Helper()
	k.n++
	b := &Block{Height: int64(k.n), Hash: fmt.Sprintf("treuhand-bestand-%s-%d", k.t.Name(), k.n),
		Timestamp: zeit, Transactions: txs}
	return k.dag.replayTransactions(b, true)
}

// pruefe: nur nachrechnen (wie vor dem Anwenden), Zaehler-Differenz zurueck.
func (k *treuhandTestKnoten) pruefe(tx Transaction, zeit int64) map[string]int64 {
	k.t.Helper()
	vorher := treuhandBestandZaehler()
	k.cs.mu.Lock()
	err := k.cs.nachrechnenTxLocked(&tx, zeit)
	k.cs.mu.Unlock()
	if err != nil {
		k.t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
	}
	return treuhandNeu(vorher)
}

func (k *treuhandTestKnoten) bestand(w string) (float64, int64, bool) {
	k.t.Helper()
	k.cs.mu.Lock()
	defer k.cs.mu.Unlock()
	b, m, gibt, err := k.cs.treuhandBestandLocked(w)
	if err != nil {
		k.t.Fatalf("Bestand lesen: %v", err)
	}
	return b, m, gibt
}

func (k *treuhandTestKnoten) topf() float64 {
	k.cs.mu.Lock()
	defer k.cs.mu.Unlock()
	return NewDecimalFromMicro(k.cs.topfMikroLocked(ubiPoolAddr)).Float()
}

// Der Nachspielende legt die Treuhand aus SEINEM Zustand an (nicht aus dem
// Betrag im Block) und mit der Blockzeit als Frist.
func TestTreuhandBestand_VerschiebungLegtZeileAn_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-zeile-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(911, 100, t1-inactivityEscrowSeconds-10)

	// Der Block behauptet 5 AEQ -- maßgeblich ist das eigene Guthaben.
	if !k.block(t1, Transaction{Type: "escrow_move", Wallet: w, Amount: 5}) {
		t.Fatal("ehrliche Verschiebung abgelehnt")
	}
	b, m, gibt := k.bestand(w)
	if !gibt || b != 100 || m != t1 {
		t.Fatalf("Treuhand %v/%d/%v, erwartet 100 AEQ seit %d", b, m, gibt, t1)
	}
}

// Missbrauch gegen den Bestand: zu viel, zu frueh, ohne Treuhand, doppelt.
// Gutfall: Freigabe nach Frist in Hoehe des Bestands, danach ist die Zeile
// weg und eine zweite Freigabe faellt auf.
func TestTreuhandBestand_FreigabeGegenBestand_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-freigabe-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(912, 100, t1-inactivityEscrowSeconds-10)
	fremd := k.inaktiverMensch(913, 50, t1-inactivityEscrowSeconds-10)
	if !k.block(t1, Transaction{Type: "escrow_move", Wallet: w, Amount: 100}) {
		t.Fatal("Verschiebung abgelehnt")
	}
	frei := t1 + escrowToUBISeconds

	if got := k.pruefe(Transaction{Type: "escrow_move", Wallet: w}, t1+86400); got["treuhand_doppelt"] != 1 {
		t.Fatalf("zweite Verschiebung derselben Wallet nicht erkannt: %v", got)
	}
	if got := k.pruefe(Transaction{Type: "escrow_release", Wallet: w, Amount: 150}, frei); got["treuhand_ueber_bestand"] != 1 {
		t.Fatalf("Freigabe ueber dem Bestand nicht erkannt: %v", got)
	}
	if got := k.pruefe(Transaction{Type: "escrow_release", Wallet: w, Amount: 100}, frei-2*treuhandUhrSpielraum); got["treuhand_zu_frueh"] != 1 {
		t.Fatalf("Freigabe vor der Frist nicht erkannt: %v", got)
	}
	if got := k.pruefe(Transaction{Type: "escrow_release", Wallet: fremd, Amount: 50}, frei); got["treuhand_ohne_bestand"] != 1 {
		t.Fatalf("Freigabe ohne Treuhand nicht erkannt: %v", got)
	}
	if got := k.pruefe(Transaction{Type: "escrow_release", Amount: 50}, frei); got["treuhand_ohne_bestand"] != 1 {
		t.Fatalf("Freigabe ohne Wallet nicht erkannt: %v", got)
	}
	if got := k.pruefe(Transaction{Type: "escrow_recover", Wallet: w, Amount: 101}, frei); got["treuhand_ueber_bestand"] != 1 {
		t.Fatalf("Rueckholung ueber dem Bestand nicht erkannt: %v", got)
	}

	// Gutfall.
	if got := k.pruefe(Transaction{Type: "escrow_release", Wallet: w, Amount: 100}, frei); len(got) != 0 {
		t.Fatalf("ehrliche Freigabe gemeldet: %v", got)
	}
	vorher := k.topf()
	if !k.block(frei, Transaction{Type: "escrow_release", Wallet: w, Amount: 100}) {
		t.Fatal("ehrliche Freigabe abgelehnt")
	}
	if got := k.topf() - vorher; got != 100 {
		t.Fatalf("Topf um %v gewachsen, erwartet 100", got)
	}
	if _, _, gibt := k.bestand(w); gibt {
		t.Fatal("Treuhand-Zeile nach der Freigabe noch da")
	}
	if got := k.pruefe(Transaction{Type: "escrow_release", Wallet: w, Amount: 100}, frei+86400); got["treuhand_ohne_bestand"] != 1 {
		t.Fatalf("zweite Freigabe derselben Treuhand nicht erkannt: %v", got)
	}
}

// Rueckholung entfernt die Zeile wie beim Erzeuger.
func TestTreuhandBestand_RueckholungEntferntZeile_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-rueckholung-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(914, 40, t1-inactivityEscrowSeconds-10)
	if !k.block(t1, Transaction{Type: "escrow_move", Wallet: w, Amount: 40}) {
		t.Fatal("Verschiebung abgelehnt")
	}
	if got := k.pruefe(Transaction{Type: "escrow_recover", Wallet: w, Amount: 40}, t1+3600); len(got) != 0 {
		t.Fatalf("ehrliche Rueckholung gemeldet: %v", got)
	}
	if !k.block(t1+3600, Transaction{Type: "escrow_recover", Wallet: w, Amount: 40}) {
		t.Fatal("Rueckholung abgelehnt")
	}
	if _, _, gibt := k.bestand(w); gibt {
		t.Fatal("Treuhand-Zeile nach der Rueckholung noch da")
	}
	if got := k.pruefe(Transaction{Type: "escrow_recover", Wallet: w, Amount: 40}, t1+7200); got["treuhand_ohne_bestand"] != 1 {
		t.Fatalf("zweite Rueckholung nicht erkannt: %v", got)
	}
}

// Ein zurueckgewiesener Block hinterlaesst keine Treuhand-Zeile: sie liegt
// in derselben Transaktion wie der Rest des Blocks.
func TestTreuhandBestand_ZurueckgewiesenerBlockOhneZeile_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-rueckrollen-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(915, 30, t1-inactivityEscrowSeconds-10)
	ok := k.block(t1,
		Transaction{Type: "escrow_move", Wallet: w, Amount: 30},
		Transaction{Type: "escrow_move", Wallet: distTestAddr(999999), Amount: 1}, // unbekanntes Konto: harter Fehler
	)
	if ok {
		t.Fatal("Vorbedingung: der Block muss scheitern")
	}
	if _, _, gibt := k.bestand(w); gibt {
		t.Fatal("zurueckgewiesener Block hat eine Treuhand-Zeile hinterlassen")
	}
}

// Im strengen Modus lehnt jeder Knoten eine Freigabe ueber dem eigenen
// Bestand ab -- das Geld entsteht nicht.
func TestTreuhandBestand_StrengLehntUeberBestandAb_RealDB(t *testing.T) {
	k := neuerTreuhandTestKnoten(t, "unused-treuhand-streng-test.json")
	t1 := fruehesterKettenstartUnix + inactivityEscrowSeconds + 86400
	w := k.inaktiverMensch(916, 100, t1-inactivityEscrowSeconds-10)
	if !k.block(t1, Transaction{Type: "escrow_move", Wallet: w, Amount: 100}) {
		t.Fatal("Verschiebung abgelehnt")
	}
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })

	frei := t1 + escrowToUBISeconds
	vorher := k.topf()
	if k.block(frei, Transaction{Type: "escrow_release", Wallet: w, Amount: 1_000_000}) {
		t.Fatal("Freigabe ueber dem Bestand angenommen")
	}
	if got := k.topf(); got != vorher {
		t.Fatalf("Topf veraendert: %v -> %v", vorher, got)
	}
	if b, _, gibt := k.bestand(w); !gibt || b != 100 {
		t.Fatalf("Treuhand nach abgelehntem Block %v/%v, erwartet 100", b, gibt)
	}
	if !k.block(frei, Transaction{Type: "escrow_release", Wallet: w, Amount: 100}) {
		t.Fatal("ehrliche Freigabe im strengen Modus abgelehnt")
	}
}
