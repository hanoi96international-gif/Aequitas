package keeper

import "testing"

// Ein leerer, ungueltiger oder nicht positiver Wert darf die Ratengrenze je IP
// weder anheben noch abschalten: deploy/perf-c1-setzen.sh setzt den Wert auf
// C1 leer und verlaesst sich darauf, dass der Knoten dann die Vorgabe nimmt.
func TestRPCRatengrenzeAusUmgebung(t *testing.T) {
	faelle := []struct {
		roh  string
		will int
	}{
		{"", rpcRateLimitMaxDefault},
		{"   ", rpcRateLimitMaxDefault},
		{"0", rpcRateLimitMaxDefault},
		{"-5", rpcRateLimitMaxDefault},
		{"1000abc", rpcRateLimitMaxDefault},
		{"1e9", rpcRateLimitMaxDefault},
		{"1000", 1000}, // ausdruecklich gesetzt: nur fuer Pruefstaende
	}
	for _, f := range faelle {
		t.Setenv("AEQUITAS_RPC_RATE_LIMIT_MAX", f.roh)
		if got := rpcRateLimitMaxFromEnv(); got != f.will {
			t.Errorf("AEQUITAS_RPC_RATE_LIMIT_MAX=%q: %d, will %d", f.roh, got, f.will)
		}
	}
	if rpcRateLimitMaxDefault != 200 {
		t.Fatalf("Vorgabe %d, erwartet 200 (Kommentar in perf-c1-setzen.sh nennt 200)", rpcRateLimitMaxDefault)
	}
}
