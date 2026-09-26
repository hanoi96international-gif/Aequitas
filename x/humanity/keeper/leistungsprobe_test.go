package keeper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestLeistungsprobe_RechnungUndBewertung(t *testing.T) {
	a, err := leistungsprobeRechnen([]byte("seed-a-0123456789"), 200)
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := leistungsprobeRechnen([]byte("seed-a-0123456789"), 200)
	b, _ := leistungsprobeRechnen([]byte("seed-b-0123456789"), 200)
	if a != a2 || a == b || len(a) != 64 {
		t.Fatalf("nicht deterministisch oder nicht vom Zufallswert abhaengig: %s %s %s", a, a2, b)
	}
	if _, err := leistungsprobeRechnen([]byte("x"), 0); err == nil {
		t.Fatal("Anzahl 0 angenommen")
	}
	// 4000 in 0,3 s bei 20.000/s erlaubt (Grenze 0,4 s); 0,5 s nicht; falsches Ergebnis nie.
	if !probeBestanden(a, a, 350*time.Millisecond, 50*time.Millisecond, 4000, 20000) {
		t.Error("schnelle, richtige Probe nicht bestanden")
	}
	if probeBestanden(a, a, 550*time.Millisecond, 50*time.Millisecond, 4000, 20000) {
		t.Error("zu langsame Probe bestanden")
	}
	if probeBestanden(a, b, time.Millisecond, 0, 4000, 20000) {
		t.Error("falsches Ergebnis bestanden")
	}
}

func TestLeistungsprobe_EndpunktNurFuerMitglieder(t *testing.T) {
	pruefer, _ := crypto.GenerateKey()
	fremd, _ := crypto.GenerateKey()
	pAddr := strings.ToLower(crypto.PubkeyToAddress(pruefer.PublicKey).Hex())
	ich := "0x00000000000000000000000000000000000000f1"
	cs := newTestState()
	cs.leitung.Store(NeueLeitung(ich, "", []string{ich, pAddr, "0x00000000000000000000000000000000000000f3"}, ich, true, LeitSpeicher{}, testKonfig(), LeitUmgebung{}, time.Now()))
	api := &APIServer{state: cs}
	srv := httptest.NewServer(http.HandlerFunc(api.handleLeistungsprobe))
	defer srv.Close()
	proben = &probenStand{ergebnis: map[string]probeErgebnis{}, naechste: map[string]time.Time{}, zuletztAn: map[string]time.Time{}}

	senden := func(key interface{}, n int, seed string) (*http.Response, string) {
		dag := &BlockDAG{signingKey: pruefer}
		von := pAddr
		if key != nil {
			dag.signingKey = fremd
			von = strings.ToLower(crypto.PubkeyToAddress(fremd.PublicKey).Hex())
		}
		m := LeitNachricht{Art: leitArtProbe, Von: von, ZeitMs: time.Now().UnixMilli(), BlockHash: seed, Hoehe: int64(n)}
		dag.signiereLeitNachricht(&m)
		b, _ := json.Marshal(m)
		resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		var antw struct{ Ergebnis string }
		json.NewDecoder(resp.Body).Decode(&antw)
		resp.Body.Close()
		return resp, antw.Ergebnis
	}
	seed := "00112233445566778899aabbccddeeff"
	resp, erg := senden(nil, probeAnzahl, seed)
	sb := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	erwartet, _ := leistungsprobeRechnen(sb, probeAnzahl)
	if resp.StatusCode != 200 || erg != erwartet {
		t.Fatalf("Mitglied: Status %d, Ergebnis stimmt: %v", resp.StatusCode, erg == erwartet)
	}
	if resp, _ := senden(nil, probeAnzahl, seed); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("zweite Probe binnen 30 s angenommen: %d", resp.StatusCode)
	}
	if resp, _ := senden(fremd, probeAnzahl, seed); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Nicht-Mitglied angenommen: %d", resp.StatusCode)
	}
}

// Wer die Probe nicht besteht, bekommt im naechsten Term keine Konten -- und
// die Konten sind trotzdem alle versorgt.
func TestLeistungsprobe_DurchgefallenBekommtKeineKonten(t *testing.T) {
	c := testKonfig()
	c.WechselAlle = 20 * time.Second
	n := verteiltesSimNetz(t, 5, 9, c)
	durchgefallen := n.knoten[3].addr
	for i := range n.knoten {
		n.baue(i)
		n.knoten[i].l.env.Zuteilbar = func(a string) bool { return a != durchgefallen }
	}
	p := neueVerteiltPruefung(150)
	p.laufe(n, 70*time.Second)
	for _, konto := range p.konten {
		if n.knoten[3].l.DarfAnnehmenFuer(konto, n.uhr(3)) {
			t.Fatalf("durchgefallenes Mitglied nimmt fuer %s an", konto)
		}
	}
	for i, k := range n.knoten {
		if k.l.Stand(n.uhr(i))["verteilt"].(map[string]interface{})["an"] == true {
			if z := k.l.vt.zuteilung; len(z) != 4 {
				t.Fatalf("Zuteilung hat %d Mitglieder statt 4: %v", len(z), z)
			}
		}
	}
	p.versorgt, p.schritte = 0, 0
	p.laufe(n, 30*time.Second)
	if v := float64(p.versorgt) / float64(p.schritte*len(p.konten)); v < 0.8 {
		t.Fatalf("nur %.0f %% versorgt", v*100)
	}
	if p.verletzt {
		t.Fatal("zwei Annehmende")
	}
}
