package keeper

import (
	"net"
	"net/http/httptest"
	"testing"
	"time"
)

func freiFuer(ip string) bool {
	r := httptest.NewRequest("POST", "/rpc", nil)
	r.RemoteAddr = ip + ":4711"
	return rpcRateLimitFrei(r)
}

// Missbrauch (Pruefung von #319, MEDIUM-2): von der Ratenbegrenzung
// freigestellt werden nur die Adressen der Mitglieder des eigenen Satzes --
// nicht die, die irgendein zugelassener Validator ankuendigt, nie Loopback,
// und wer seine Adresse wechselt oder den Satz verlaesst, faellt heraus. Die
// Adressen aus AEQUITAS_RPC_RATE_LIMIT_FREI bleiben.
func TestFreiliste_NurSatzKeinLoopbackNeuAufgebaut(t *testing.T) {
	altListe, altUmgebung := rpcRateLimitFreiListe.Load(), rpcRateLimitFreiUmgebung
	t.Cleanup(func() { rpcRateLimitFreiListe.Store(altListe); rpcRateLimitFreiUmgebung = altUmgebung })
	rpcRateLimitFreiUmgebung = map[string]bool{"192.0.2.200": true} // Lastgenerator aus der Umgebung
	rpcRateLimitFreiValidatoren(nil)

	ich := "0x0000000000000000000000000000000000000003"
	mitglied := "0x0000000000000000000000000000000000000001"
	lokal := "0x0000000000000000000000000000000000000002"
	fremd := "0x0000000000000000000000000000000000000009" // zugelassen, nicht im Satz
	l := NeueLeitung(ich, "", []string{ich, mitglied, lokal}, mitglied, true, LeitSpeicher{Term: 1, Leiter: mitglied}, testKonfig(), LeitUmgebung{}, time.Now())
	l.SetzeURL(mitglied, "http://203.0.113.5:8080")
	l.SetzeURL(lokal, "http://127.0.0.1:8080")
	// Der Angriff: ein zugelassener Validator ausserhalb des Satzes kuendigt
	// in einer Hallo-Nachricht eine Adresse an (die eines Proxys).
	l.Empfange(LeitNachricht{Art: leitArtHallo, Von: fremd, URL: "http://100.64.0.2:8080"}, time.Now())
	if l.URLs()[fremd] == "" {
		t.Fatal("Vorbedingung: die angekuendigte Adresse ist gespeichert")
	}
	validatorIPsFrei(l)
	for ip, soll := range map[string]bool{
		"203.0.113.5": true,  // Mitglied
		"127.0.0.1":   false, // Loopback, auch von einem Mitglied
		"100.64.0.2":  false, // nicht im Satz
		"192.0.2.200": true,  // aus der Umgebung
	} {
		if freiFuer(ip) != soll {
			t.Fatalf("%s freigestellt = %v, erwartet %v", ip, !soll, soll)
		}
	}
	// Das Mitglied wechselt die Adresse: die alte faellt heraus.
	l.SetzeURL(mitglied, "http://203.0.113.6:8080")
	validatorIPsFrei(l)
	if freiFuer("203.0.113.5") || !freiFuer("203.0.113.6") || !freiFuer("192.0.2.200") {
		t.Fatal("die Liste wurde nicht neu aufgebaut")
	}
}

func TestFreistellbar(t *testing.T) {
	for ip, soll := range map[string]bool{
		"203.0.113.5": true, "10.0.0.7": true, "100.64.1.2": true, "2001:db8::1": true,
		"127.0.0.1": false, "::1": false, "0.0.0.0": false, "::": false,
		"169.254.1.1": false, "fe80::1": false, "224.0.0.1": false, "ff02::1": false,
	} {
		if got := freistellbar(net.ParseIP(ip)); got != soll {
			t.Fatalf("freistellbar(%s) = %v, erwartet %v", ip, got, soll)
		}
	}
	if freistellbar(nil) {
		t.Fatal("freistellbar(nil)")
	}
}
