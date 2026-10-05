package keeper

import (
	"crypto/ecdsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Der Knoten unterschreibt nur, was IssuePeerChallenge erzeugt -- kein
// beliebiger Text eines Seeds (fetchAndSignPeerChallenge).

func challengeSeed(t *testing.T, challenge string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
	}))
	t.Cleanup(srv.Close)
	echt := httpSyncClient
	httpSyncClient = &http.Client{}
	t.Cleanup(func() { httpSyncClient = echt })
	return srv.URL
}

func TestPeerChallenge_EchteWirdUnterschrieben(t *testing.T) {
	key, addr := neuerSchluesselFuerChallenge(t)
	dag := newOrphanTestDAG()
	dag.peerChallenges = map[string]peerChallenge{}
	c := dag.IssuePeerChallenge(addr)
	if !echteChallenge(c) {
		t.Fatalf("IssuePeerChallenge erzeugt %q -- nicht die Form, die unterschrieben wird", c)
	}
	sig := fetchAndSignPeerChallenge(challengeSeed(t, c), addr, key)
	if sig == "" || !dag.VerifyPeerChallenge(addr, sig) {
		t.Fatal("echte Challenge nicht (gueltig) unterschrieben")
	}
}

// Missbrauch: der Seed schickt einen Satz, den die Kette als Zustimmung des
// Schluessels liest.
func TestPeerChallenge_BeliebigerTextWirdNichtUnterschrieben(t *testing.T) {
	key, addr := neuerSchluesselFuerChallenge(t)
	angreifer := "0x" + strings.Repeat("ab", 20)
	for _, c := range []string{
		"Aequitas: validator key linked to human " + angreifer,
		"Aequitas: authorize validator " + addr,
		strings.Repeat("A", 64),        // Grossbuchstaben
		strings.Repeat("a", 63),        // zu kurz
		strings.Repeat("a", 65),        // zu lang
		strings.Repeat("a", 62) + " z", // fremdes Zeichen
		"",
	} {
		if sig := fetchAndSignPeerChallenge(challengeSeed(t, c), addr, key); sig != "" {
			t.Fatalf("%q unterschrieben", c)
		}
	}
}

func neuerSchluesselFuerChallenge(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k, strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
}
