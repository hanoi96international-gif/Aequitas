package keeper

import "testing"

func treuhandZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range []string{"treuhand_aktiv", "treuhand_lp", "treuhand_zu_frueh"} {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func treuhandNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for r, v := range vorher {
		if d := erhaltungZaehler(r) - v; d != 0 {
			m[r] = d
		}
	}
	return m
}

func treuhandPruefe(t *testing.T, cs *ChainState, tx Transaction, zeit int64) map[string]int64 {
	t.Helper()
	vorher := treuhandZaehler()
	cs.mu.Lock()
	err := cs.nachrechnenTxLocked(&tx, zeit)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
	}
	return treuhandNeu(vorher)
}

// Missbrauch: ein Produzent setzt das Guthaben eines AKTIVEN Menschen per
// escrow_move auf null. Ein wirklich 2,5 Jahre inaktives Konto bleibt ohne
// Meldung.
func TestNachrechnenTreuhand_Verschiebung(t *testing.T) {
	jetzt := int64(1_900_000_000)
	cs := newTestState()
	cs.mu.Lock()
	cs.accounts.Set("0xaktiv", &AccountState{Address: "0xaktiv", IsHuman: true, Balance: NewDecimal(500), LastActivityAt: jetzt - 86400})
	cs.accounts.Set("0xruhig", &AccountState{Address: "0xruhig", IsHuman: true, Balance: NewDecimal(500), LPShares: NewDecimal(2),
		LastActivityAt: jetzt - inactivityEscrowSeconds - 86400})
	cs.accounts.Set("0xfrei", &AccountState{Address: "0xfrei", Balance: NewDecimal(500), LastActivityAt: jetzt - inactivityEscrowSeconds - 86400})
	cs.mu.Unlock()

	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_move", Wallet: "0xruhig", LPShares: 2}, jetzt); len(neu) != 0 {
		t.Fatalf("echte Verschiebung eines inaktiven Kontos meldet %v", neu)
	}
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_move", Wallet: "0xaktiv"}, jetzt); neu["treuhand_aktiv"] != 1 {
		t.Fatalf("Verschiebung eines aktiven Kontos nicht erkannt: %v", neu)
	}
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_move", Wallet: "0xfrei"}, jetzt); neu["treuhand_aktiv"] != 1 {
		t.Fatalf("Verschiebung eines Nicht-Menschen nicht erkannt: %v", neu)
	}
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_move", Wallet: "0xunbekannt"}, jetzt); neu["treuhand_aktiv"] != 1 {
		t.Fatalf("Verschiebung eines unbekannten Kontos nicht erkannt: %v", neu)
	}
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_move", Wallet: "0xruhig", LPShares: 50}, jetzt); neu["treuhand_lp"] != 1 {
		t.Fatalf("LP-Anteile ueber dem Bestand nicht erkannt: %v", neu)
	}
}

// Missbrauch: Freigabe in den Topf bzw. Rueckholung, bevor es eine
// ehrliche Treuhand ueberhaupt geben kann -- neues Geld.
func TestNachrechnenTreuhand_FreigabeZuFrueh(t *testing.T) {
	cs := newTestState()
	heute := int64(1_791_000_000) // Oktober 2026
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_release", Amount: 1e6}, heute); neu["treuhand_zu_frueh"] != 1 {
		t.Fatalf("escrow_release heute nicht erkannt: %v", neu)
	}
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_recover", Wallet: "0xa", Amount: 1e6}, heute); neu["treuhand_zu_frueh"] != 1 {
		t.Fatalf("escrow_recover heute nicht erkannt: %v", neu)
	}
	spaeter := fruehesterKettenstartUnix + inactivityEscrowSeconds + escrowToUBISeconds + 86400
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_release", Amount: 5}, spaeter); len(neu) != 0 {
		t.Fatalf("nach der ersten moeglichen Freigabe darf nichts anschlagen: %v", neu)
	}
	if neu := treuhandPruefe(t, cs, Transaction{Type: "escrow_recover", Wallet: "0xa", Amount: 5}, spaeter); len(neu) != 0 {
		t.Fatalf("Rueckholung nach der Frist meldet %v", neu)
	}
}
