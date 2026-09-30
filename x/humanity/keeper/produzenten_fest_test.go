package keeper

import "testing"

// Audit 2026-09-29, K-1: ist AUTHORIZED_VALIDATORS gesetzt, wird ein
// selbst eingetragener oder von Peers gemeldeter Schluessel KEIN
// Blockproduzent -- ohne Liste bleibt das bisherige offene Verhalten.
func TestProduzentenFest_SelbsteintragungWirdKeinProduzent(t *testing.T) {
	betreiber := "0x1111111111111111111111111111111111111111"
	selbst := "0x2222222222222222222222222222222222222222"
	angreifer := "0x3333333333333333333333333333333333333333"
	dag := &BlockDAG{
		authorizedValidators: map[string]bool{betreiber: true, selbst: true},
		produzentenFest:      map[string]bool{betreiber: true},
		selfProposer:         selbst,
	}
	dag.AddAuthorizedValidator(angreifer)
	if dag.authorizedValidators[angreifer] {
		t.Fatal("ein nicht gelisteter Schluessel darf bei geschlossener Liste kein Produzent werden")
	}
	dag.mu.Lock()
	aufgenommen := dag.nimmProduzentAufLocked(angreifer)
	dag.mu.Unlock()
	if aufgenommen {
		t.Fatal("auch der Peer-Weg (sync_blocks) darf ihn nicht aufnehmen")
	}
	// Gelistete Adresse und der eigene Schluessel bleiben zugelassen.
	dag.AddAuthorizedValidator(betreiber)
	dag.AddAuthorizedValidator(selbst)
	if !dag.authorizedValidators[betreiber] || !dag.authorizedValidators[selbst] {
		t.Fatal("gelistete Adresse und eigener Schluessel muessen Produzenten bleiben")
	}
	if !dag.ProduzentenGeschlossen() {
		t.Fatal("ProduzentenGeschlossen muss true melden")
	}
}

func TestProduzentenFest_OhneListeBisherigesVerhalten(t *testing.T) {
	neu := "0x4444444444444444444444444444444444444444"
	dag := &BlockDAG{authorizedValidators: map[string]bool{}}
	dag.AddAuthorizedValidator(neu)
	if !dag.authorizedValidators[neu] {
		t.Fatal("ohne AUTHORIZED_VALIDATORS muss die Aufnahme wie bisher gelingen")
	}
	if dag.ProduzentenGeschlossen() {
		t.Fatal("ohne Liste ist nichts geschlossen")
	}
}

func TestProduzentenFest_GrossschreibungUndLeerraum(t *testing.T) {
	t.Setenv("AUTHORIZED_VALIDATORS", " 0xAAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaa ,")
	fest := loadAuthorizedValidators()
	dag := &BlockDAG{authorizedValidators: map[string]bool{}, produzentenFest: fest}
	dag.AddAuthorizedValidator("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !dag.authorizedValidators["0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] {
		t.Fatal("die Liste muss unabhaengig von Gross-/Kleinschreibung greifen")
	}
}
