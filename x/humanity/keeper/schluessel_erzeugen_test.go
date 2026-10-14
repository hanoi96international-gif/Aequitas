package keeper

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
)

// Die Ausgabe von `aequitasd schluessel` sind genau zwei .env-Zeilen, die
// der Knoten so laedt.
func TestSchluesselAusgeben_LadbarUndNurZweiZeilen(t *testing.T) {
	var b bytes.Buffer
	if err := SchluesselAusgeben(&b); err != nil {
		t.Fatal(err)
	}
	zeilen := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(zeilen) != 2 || !strings.HasPrefix(zeilen[0], "RELAYER_PRIVATE_KEY=0x") || !strings.HasPrefix(zeilen[1], "NODE_KEY=") {
		t.Fatalf("Ausgabe: %d Zeilen", len(zeilen))
	}
	t.Setenv("RELAYER_PRIVATE_KEY", strings.TrimPrefix(zeilen[0], "RELAYER_PRIVATE_KEY="))
	t.Setenv("NODE_KEY", strings.TrimPrefix(zeilen[1], "NODE_KEY="))
	if _, erzeugt, err := loadOrCreateRelayerKey(); err != nil || erzeugt {
		t.Fatalf("Signierschluessel nicht geladen: %v %v", erzeugt, err)
	}
	if _, err := loadOrCreateKey(); err != nil {
		t.Fatal(err)
	}
	var c bytes.Buffer
	if err := SchluesselAusgeben(&c); err != nil || c.String() == b.String() {
		t.Fatal("zweimal derselbe Schluessel")
	}
}

// fangeAusgabe: was f nach Stdout und Stderr schreibt.
func fangeAusgabe(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	altOut, altErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	fertig := make(chan string)
	go func() { b, _ := io.ReadAll(r); fertig <- string(b) }()
	f()
	os.Stdout, os.Stderr = altOut, altErr
	w.Close()
	return <-fertig
}

// Missbrauch (Beta-Pruefung 14.10.2026): ohne gesetzte Schluessel erzeugt
// der Knoten voruebergehende -- und schreibt sie in KEINER Form ins Log.
func TestSchluessel_NieImLog(t *testing.T) {
	t.Setenv("RELAYER_PRIVATE_KEY", "")
	t.Setenv("NODE_KEY", "")
	var rk []byte
	log := fangeAusgabe(t, func() {
		k, erzeugt, err := loadOrCreateRelayerKey()
		if err != nil || !erzeugt {
			t.Errorf("kein voruebergehender Schluessel: %v", err)
			return
		}
		rk = ethcrypto.FromECDSA(k)
	})
	if len(rk) == 0 || strings.Contains(strings.ToLower(log), hex.EncodeToString(rk)) {
		t.Fatal("Signierschluessel im Log")
	}
	var nk []byte
	log = fangeAusgabe(t, func() {
		k, err := loadOrCreateKey()
		if err != nil {
			t.Error(err)
			return
		}
		nk, _ = p2pcrypto.MarshalPrivateKey(k)
	})
	if len(nk) == 0 {
		t.Fatal("kein P2P-Schluessel")
	}
	for _, teil := range []string{"SAVE THIS", "SET THIS"} {
		if strings.Contains(log, teil) {
			t.Fatalf("alte Ausgabe %q im Log", teil)
		}
	}
	// base64 des Schluessels oder ein langes Stueck davon
	if enc := encodeB64(nk); strings.Contains(log, enc[len(enc)-40:]) {
		t.Fatal("P2P-Schluessel im Log")
	}
}

func encodeB64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
