package keeper

import (
	"net/http/httptest"
	"testing"
)

func TestRpcRateLimitFrei_NurDieTCPAdresseZaehlt(t *testing.T) {
	t.Setenv("AEQUITAS_RPC_RATE_LIMIT_FREI", " 194.163.188.71 , kein-ip, ")
	liste := rpcRateLimitFreiAusUmgebung()
	if len(liste) != 1 || !liste["194.163.188.71"] {
		t.Fatalf("Liste = %v, erwartet genau die eine gueltige Adresse", liste)
	}

	partner := httptest.NewRequest("POST", "/rpc", nil)
	partner.RemoteAddr = "194.163.188.71:51234"
	if !rpcRateLimitFreiFuer(liste, partner) {
		t.Fatal("die Partnerbox muss freigestellt sein")
	}

	fremd := httptest.NewRequest("POST", "/rpc", nil)
	fremd.RemoteAddr = "203.0.113.9:40000"
	if rpcRateLimitFreiFuer(liste, fremd) {
		t.Fatal("eine fremde Adresse darf nicht freigestellt sein")
	}

	// Der Angriff, den die Beschraenkung auf RemoteAddr abwehrt: jemand aus
	// dem Netz gibt sich per Kopf als Partner aus -- direkt oder ueber den
	// eigenen Proxy (dann ist RemoteAddr privat und clientIP() glaubt dem Kopf).
	for _, remote := range []string{"203.0.113.9:40000", "127.0.0.1:40000"} {
		gefaelscht := httptest.NewRequest("POST", "/rpc", nil)
		gefaelscht.RemoteAddr = remote
		gefaelscht.Header.Set("X-Forwarded-For", "194.163.188.71")
		if rpcRateLimitFreiFuer(liste, gefaelscht) {
			t.Fatalf("X-Forwarded-For darf nie freistellen (RemoteAddr %s)", remote)
		}
	}
}

func TestRpcRateLimitFrei_LeerStelltNiemandenFrei(t *testing.T) {
	t.Setenv("AEQUITAS_RPC_RATE_LIMIT_FREI", "")
	liste := rpcRateLimitFreiAusUmgebung()
	r := httptest.NewRequest("POST", "/rpc", nil)
	r.RemoteAddr = "194.163.188.71:1"
	if rpcRateLimitFreiFuer(liste, r) {
		t.Fatal("ohne Liste ist niemand freigestellt")
	}
}
