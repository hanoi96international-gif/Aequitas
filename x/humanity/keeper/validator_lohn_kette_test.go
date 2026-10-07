package keeper

import "testing"

// Das Fenster: die 24 Stunden bis 15 Minuten vor der Runde -- mit festen
// Zahlen, nicht aus denselben Konstanten nachgerechnet.
func TestValidatorLohnAusKette_Fenster(t *testing.T) {
	seit, bis := anwesenheitsFenster(1_000_000)
	if seit != 1_000_000-86400-900 || bis != 1_000_000-900 {
		t.Fatalf("Fenster [%d, %d)", seit, bis)
	}
}

// Umgeschaltet wird erst, wenn das ganze Fenster nach erzeugerSchnittAb
// liegt (Rueck-Toleranz der Blockzeit eingerechnet); nie ohne Rundenzeit.
func TestValidatorLohnAusKette_Schalter(t *testing.T) {
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	const ab = int64(10_000_000)
	erzeugerSchnittOverride.Store(ab)
	grenze := ab + 86400 + 900 - zeitstempelRueckToleranz
	if validatorLohnAusKette(grenze - 1) {
		t.Fatal("umgeschaltet, obwohl das Fenster vor dem Stichtag beginnt")
	}
	if !validatorLohnAusKette(grenze) {
		t.Fatal("nicht umgeschaltet, obwohl das Fenster nach dem Stichtag liegt")
	}
	if validatorLohnAusKette(0) {
		t.Fatal("ohne Rundenzeit umgeschaltet")
	}
	erzeugerSchnittOverride.Store(0) // Platzhalter
	if validatorLohnAusKette(nowUnix()) {
		t.Fatal("vor dem Stichtag (Platzhalter) umgeschaltet")
	}
}

// Die Gutschrift traegt die Rundenzeit -- ohne sie koennte kein Knoten
// nachrechnen (validator_ohne_runde).
func TestValidatorLohnAusKette_GutschriftTraegtRunde(t *testing.T) {
	tx := validatorGutschrift(DistributionShare{Wallet: "0xa", Amount: 1.5, DemurrageLost: 0.1, RundeAt: 777})
	if tx.Type != "validator_distribution" || tx.Wallet != "0xa" || tx.Amount != 1.5 || tx.FromDemurrageLost != 0.1 || tx.DistributionAt != 777 {
		t.Fatalf("Gutschrift %+v", tx)
	}
	if tx := validatorGutschrift(DistributionShare{Wallet: "0xa", Amount: 1}); tx.DistributionAt != 0 {
		t.Fatalf("alte Regel mit Rundenzeit: %+v", tx)
	}
}
