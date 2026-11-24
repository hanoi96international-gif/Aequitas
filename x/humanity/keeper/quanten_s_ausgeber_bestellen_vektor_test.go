package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR: BESTELLEN UND LOSEN AUS DER APP (Weg B, B4b-4c-2a).
//
// Die App als Ausgeber (lib/quantenSAusgeber.ts) bildet die Nachricht der
// Bestellung, das Pfand und die Runde des Loses selbst und reicht Bestellen
// und Losen bei POST /api/quanten-w/s_bestellen bzw. s_losen ein.
// testdata/quanten_s_ausgeber_bestellen_vektor.json entsteht in der App
// (lib/__tests__/quantenSAusgeber.test.ts, Schluessel NUR fuer Tests); hier
// prueft der Knoten ihn mit SEINEM Code: die Koerper wie handleQuantenW
// gelesen (keine fremden Felder), Nachricht, Unterschrift, Pfand- und
// Rundentabelle, und Bestellen und Losen mit dem echten quicknet-Beacon auf
// einer Testkette angenommen.

type qsAusgeberVektor struct {
	Hinweis          string     `json:"hinweis"`
	Schluessel       string     `json:"schluessel_nur_test"`
	Wallet           string     `json:"wallet"`
	K                string     `json:"k"`
	V                int        `json:"v"`
	T                int        `json:"t"`
	Deckung          int64      `json:"deckung"`
	Zeit             int64      `json:"zeit"`
	Nachricht        string     `json:"nachricht"`
	WalletSig        string     `json:"wallet_sig"`
	Pfand            int64      `json:"pfand"`
	KoerperBestellen string     `json:"koerper_bestellen"`
	Runde            uint64     `json:"runde"`
	RundeSig         string     `json:"runde_sig"`
	KoerperLosen     string     `json:"koerper_losen"`
	PfandTabelle     [][2]int64 `json:"pfand_tabelle"`
	RundenTabelle    [][2]int64 `json:"runden_tabelle"`
}

// qsKoerperLesen: wie handleQuantenW (DisallowUnknownFields, K klein).
func qsKoerperLesen(t *testing.T, art, roh string) Transaction {
	t.Helper()
	var req struct {
		Wallet   string          `json:"wallet"`
		Zeit     int64           `json:"zeit"`
		QuantenW QuantenWAngaben `json:"quanten_w"`
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		t.Fatalf("%s: Koerper: %v", art, err)
	}
	q := req.QuantenW
	q.K = strings.ToLower(strings.TrimSpace(q.K))
	return Transaction{Type: art, Wallet: strings.ToLower(strings.TrimSpace(req.Wallet)), DistributionAt: req.Zeit, QuantenW: &q}
}

func TestQuantenSAusgeberBestellenVektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_ausgeber_bestellen_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qsAusgeberVektor
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	key, err := crypto.HexToECDSA(v.Schluessel)
	if err != nil {
		t.Fatal(err)
	}
	if quantenSAdresse(key) != v.Wallet {
		t.Fatal("Schluessel gehoert nicht zur Wallet")
	}
	if m := quantenSBestellNachricht(v.Wallet, v.K, v.V, v.T, v.Deckung, v.Zeit); m != v.Nachricht {
		t.Fatalf("Nachricht:\n App    %s\n Knoten %s", v.Nachricht, m)
	}
	if !kanonischeSignatur(v.WalletSig) || verifyPersonalSign(v.Nachricht, v.WalletSig, v.Wallet) != nil {
		t.Fatal("Unterschrift der App nimmt der Knoten nicht")
	}
	if p := quantenSBestellPfandFuer(v.Deckung); p != v.Pfand {
		t.Fatalf("Pfand: App %d, Knoten %d", v.Pfand, p)
	}
	for _, e := range v.PfandTabelle {
		if p := quantenSBestellPfandFuer(e[0]); p != e[1] {
			t.Fatalf("Pfand fuer %d: App %d, Knoten %d", e[0], e[1], p)
		}
	}
	for _, e := range v.RundenTabelle {
		if r, ok := quantenSLosRunde(e[0]); !ok || int64(r) != e[1] {
			t.Fatalf("Runde fuer Bestellzeit %d: App %d, Knoten %d (%v)", e[0], e[1], r, ok)
		}
	}
	if r, ok := quantenSLosRunde(v.Zeit); !ok || r != v.Runde || v.Runde != drandEchtRunde || v.RundeSig != drandEchtSig {
		t.Fatalf("Runde %d (App %d) oder Beacon nicht der echte", r, v.Runde)
	}

	// Die Koerper genau wie der Handler.
	best := qsKoerperLesen(t, quantenSBestellenTxArt, v.KoerperBestellen)
	los := qsKoerperLesen(t, quantenSLosenTxArt, v.KoerperLosen)
	if best.Wallet != v.Wallet || best.DistributionAt != v.Zeit || best.QuantenW.K != v.K || best.QuantenW.PraepZahl != v.V ||
		best.QuantenW.T != v.T || best.QuantenW.Deckung != v.Deckung || best.QuantenW.WalletSig != v.WalletSig {
		t.Fatal("Koerper der Bestellung weicht vom Vektor ab")
	}
	for _, tx := range []Transaction{best, los} {
		if !quantenWNurEigeneFelder(&tx) {
			t.Fatalf("%s: fremde Felder", tx.Type)
		}
		if !quantenWZeitGueltig(&tx, v.Zeit) {
			t.Fatalf("%s: Zeit", tx.Type)
		}
	}

	// Auf einer Testkette mit dem echten quicknet: Bestellen und Losen gehen durch.
	w := neueBWelt(t)
	drandQuicknet = func() drandNetz {
		n, _ := drandNetzAus(drandQuicknetSchluesselHex, drandQuicknetGenesis, drandQuicknetPeriode)
		return n
	}()
	w.z = v.Zeit
	quantenWAbOverride.Store(w.z - 30*86400)
	quantenSWegBAbOverride.Store(w.z - 30*86400)
	reg := neuesQuantenWRegister()
	for _, a := range w.praep {
		reg.Praep[quantenSPraepK(a)] = quantenSPraep{Konto: a, Pub: praepPubTest(w.keys[a]), Gemeldet: w.z - 2*86400}
	}
	for _, cs := range []*ChainState{w.cs, w.nach} {
		addHuman(cs, v.Wallet, 100)
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	w.annehmen(best, v.Zeit)
	b := w.cs.quantenWJetzt().Best[v.K]
	if b == nil || b.Pfand != v.Pfand || b.Ausgeber != v.Wallet || b.V != v.V || b.T != v.T {
		t.Fatal("Bestellung der App nicht wie erwartet im Register")
	}
	w.annehmen(los, v.Zeit+quantenSLosAbstand+5)
	if b := w.cs.quantenWJetzt().Best[v.K]; b == nil || len(b.Los) != v.V || b.Runde != v.Runde {
		t.Fatal("Losen der App nicht angenommen")
	}

	// Missbrauch: veraenderte Bestellung (andere Deckung) traegt die Unterschrift nicht.
	falsch := best
	q := *best.QuantenW
	q.Deckung++
	falsch.QuantenW = &q
	if u := w.bescheid(w.cs, falsch, v.Zeit); u.regel == "" {
		t.Fatal("veraenderte Deckung angenommen")
	}
}
