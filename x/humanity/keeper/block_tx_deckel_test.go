package keeper

import (
	"strconv"
	"testing"
)

// Der Deckel darf nur SENKEN. Ihn ueber eine Umgebungsvariable nach oben
// aufzumachen wuerde die globale Schreibsperre laenger halten -- die falsche
// Richtung, und beim naechsten Vorfall vermutet das niemand.
func TestBlockTxHartDeckel_OeffnetNieNachOben(t *testing.T) {
	for _, wert := range []string{
		strconv.Itoa(maxTxsPerBlock + 1),
		strconv.Itoa(maxTxsPerBlock * 10),
		"0", "-1", "abc", "",
	} {
		t.Setenv(maxTxsPerBlockEnv, wert)
		if got := blockTxHartDeckel(); got != maxTxsPerBlock {
			t.Fatalf("%q ergab %d, erwartet die Vorgabe %d -- der Deckel darf nur senken",
				wert, got, maxTxsPerBlock)
		}
	}
}

func TestBlockTxHartDeckel_SenktWirklich(t *testing.T) {
	t.Setenv(maxTxsPerBlockEnv, "2000")
	if got := blockTxHartDeckel(); got != 2000 {
		t.Fatalf("Deckel = %d, erwartet 2000", got)
	}
}

// Der Deckel muss auch bei ABGESCHALTETER Peer-Lag-Bremse greifen: er regelt
// nicht den Rueckstand des Partners, sondern die Haltezeit der eigenen
// Schreibsperre. Die Bremse ist seit dem 02.09.2026 per Vorgabe aus -- ein
// Deckel, der nur mit ihr zusammen wirkt, waere damit wirkungslos.
func TestBlockTxCap_DeckelGreiftAuchOhneBremse(t *testing.T) {
	t.Setenv(peerLagBodenEnv, "0") // Bremse ausdruecklich aus
	t.Setenv(maxTxsPerBlockEnv, "1500")
	dag := &BlockDAG{}
	if got := dag.blockTxCapFuerHoehe(1000); got != 1500 {
		t.Fatalf("blockTxCap = %d bei abgeschalteter Bremse, erwartet 1500 -- "+
			"sonst wirkt der Deckel genau dann nicht, wenn er gebraucht wird", got)
	}
}
