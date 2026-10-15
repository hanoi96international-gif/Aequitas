package keeper

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Missbrauch (Beta-Pruefung 15.10.2026): /api/blocks/push hat feste
// Obergrenzen gleichzeitiger Anfragen, global und je IP.
func TestBlockPush_Obergrenzen(t *testing.T) {
	var frei []func()
	for i := 0; i < blockPushGleichzeitigJeIP; i++ {
		f, ok := blockPushPlatz("198.51.100.1")
		if !ok {
			t.Fatalf("Platz %d fuer eine IP verweigert", i)
		}
		frei = append(frei, f)
	}
	if _, ok := blockPushPlatz("198.51.100.1"); ok {
		t.Fatal("eine IP bekommt mehr als ihre Grenze")
	}
	for i := 0; len(frei) < blockPushGleichzeitig; i++ {
		f, ok := blockPushPlatz("198.51.100." + string(rune('a'+i)))
		if !ok {
			t.Fatalf("globaler Platz %d verweigert", len(frei))
		}
		frei = append(frei, f)
	}
	if _, ok := blockPushPlatz("203.0.113.77"); ok {
		t.Fatal("mehr als die globale Grenze")
	}
	for _, f := range frei {
		f()
	}
	if f, ok := blockPushPlatz("203.0.113.77"); !ok {
		t.Fatal("nach der Freigabe kein Platz")
	} else {
		f()
	}
	blockPushJeIPMu.Lock()
	n := len(blockPushJeIP)
	blockPushJeIPMu.Unlock()
	if n != 0 {
		t.Fatalf("Je-IP-Eintraege bleiben liegen: %d", n)
	}
}

// Ungueltige Ruempfe zaehlen im Flutschutz: eine IP, die nur Muell schickt,
// sperrt sich selbst (vorher zaehlte erst das Ergebnis von AddPeerBlock).
func TestBlockPush_MuellZaehltImFlutschutz(t *testing.T) {
	ip := "198.51.100.250"
	// Eigener Ausgangsstand: andere Tests fuellen den (begrenzten) Breaker.
	blockPushBreaker.Clear()
	t.Cleanup(func() { blockPushBreaker.Clear() })
	a := &APIServer{}
	gesperrt := false
	for i := 0; i < blockPushBreakerThreshold+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/blocks/push", bytes.NewReader([]byte("{kein json")))
		req.RemoteAddr = ip + ":4711"
		rec := httptest.NewRecorder()
		a.handleBlockPush(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			gesperrt = true
			break
		}
	}
	if !gesperrt {
		t.Fatal("eine IP mit ungueltigen Ruempfen wird nie gesperrt")
	}
}

// Ein gestrippter Kopf mit erfundenem Hash oder fremder Signatur loest keinen
// Rumpf-Abruf aus; ein echt signierter Kopf eines (noch) unbekannten
// Erzeugers wird vollstaendig angefordert.
func TestBlockPush_KopfVorAbruf(t *testing.T) {
	k, _ := crypto.GenerateKey()
	addr := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
	b := &Block{Height: 5, Timestamp: 1_800_000_000, ParentHashes: []string{strings.Repeat("a", 64)}, Proposer: addr, TxRoot: strings.Repeat("b", 64)}
	b.Hash = calculateBlockHash(b)
	h, _ := hex.DecodeString(b.Hash)
	sig, err := crypto.Sign(h, k)
	if err != nil {
		t.Fatal(err)
	}
	b.Signature = hex.EncodeToString(sig)
	dag := &BlockDAG{authorizedValidators: map[string]bool{}}
	if u := dag.kopfVorAbruf(b); u != kopfUnbekannt {
		t.Fatalf("echt signiert, unbekannter Erzeuger: %v", u)
	}
	dag.authorizedValidators[addr] = true
	if u := dag.kopfVorAbruf(b); u != kopfGueltig {
		t.Fatalf("echt signiert, erlaubter Erzeuger: %v", u)
	}
	falsch := *b
	falsch.TxRoot = strings.Repeat("c", 64) // erfundene Wurzel, Hash passt nicht mehr
	if u := dag.kopfVorAbruf(&falsch); u != kopfGefaelscht {
		t.Fatalf("erfundene tx_root: %v", u)
	}
	fremd := *b
	fremd.Proposer = "0x" + strings.Repeat("1", 40)
	fremd.Hash = calculateBlockHash(&fremd)
	if u := dag.kopfVorAbruf(&fremd); u != kopfGefaelscht {
		t.Fatalf("Signatur eines anderen: %v", u)
	}
}
