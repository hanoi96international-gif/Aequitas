package keeper

import (
	"encoding/json"
	"testing"
)

// eth_getTransactionCount fragt die Datenbank nur, solange die Nonce-Shard
// die Adresse nicht kennt -- und liefert dabei denselben Wert wie bisher
// (das Maximum aus Shard, Datenbank und NaechsteNonce der Kette).
func TestNonceAbfrage_BekannteAdresseOhneDatenbank(t *testing.T) {
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	const addr = "0x00000000000000000000000000000000000a0b0c"
	param := func() []json.RawMessage { return []json.RawMessage{json.RawMessage(`"` + addr + `"`)} }

	vorher := nonceDBAbfragen.Load()
	if r, err := srv.getTransactionCount(param()); err != nil || r != "0x0" {
		t.Fatalf("unbekannt: %v %v", r, err)
	}
	if nonceDBAbfragen.Load() != vorher+1 {
		t.Fatal("unbekannte Adresse muss die Datenbank fragen")
	}

	sh := srv.nonceShardFor(addr)
	sh.mu.Lock()
	sh.nonces[addr] = 7
	sh.mu.Unlock()
	vorher = nonceDBAbfragen.Load()
	if r, err := srv.getTransactionCount(param()); err != nil || r != "0x7" {
		t.Fatalf("bekannt: %v %v", r, err)
	}
	if nonceDBAbfragen.Load() != vorher {
		t.Fatal("bekannte Adresse darf die Datenbank nicht fragen")
	}

	// Die Kette ist weiter (z. B. nur lesender Knoten): der hoehere Wert gilt.
	cs.accounts.Set(addr, &AccountState{Address: addr, NaechsteNonce: 12})
	if r, _ := srv.getTransactionCount(param()); r != "0xc" {
		t.Fatalf("NaechsteNonce der Kette muss gewinnen, bekam %v", r)
	}
}
