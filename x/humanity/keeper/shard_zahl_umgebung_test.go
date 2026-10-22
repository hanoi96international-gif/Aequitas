package keeper

import "testing"

// AEQUITAS_ACCOUNT_SHARDS: jeder unbrauchbare Wert ergibt die Vorgabe, auch
// einer ueber der Obergrenze. Eine Null zu viel darf den Knoten nicht beim
// Start in den Speichertod schicken. Liest nur die Umgebung, schreibt
// numAccountShards nicht.
func TestShardZahlAusUmgebung_Grenzen(t *testing.T) {
	for _, fall := range []struct {
		roh  string
		will int
	}{
		{"", numAccountShardsVorgabe},
		{"16384", 16384},
		{" 1 ", 1},
		{"1048576", 1 << 20},
		{"1048577", numAccountShardsVorgabe},
		{"2621440", numAccountShardsVorgabe},
		{"99999999999999999999", numAccountShardsVorgabe},
		{"0", numAccountShardsVorgabe},
		{"-1", numAccountShardsVorgabe},
		{"viele", numAccountShardsVorgabe},
	} {
		t.Setenv(numAccountShardsEnv, fall.roh)
		if got := shardZahlAusUmgebung(); got != fall.will {
			t.Errorf("%s=%q ergab %d, erwartet %d", numAccountShardsEnv, fall.roh, got, fall.will)
		}
	}
}
