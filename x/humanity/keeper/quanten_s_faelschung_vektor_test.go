package keeper

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"
)

// PRUEFVEKTOR: NACHWEIS EINER FAELSCHUNG AUS DER APP (P5c).
//
// Die App als Stelle B (lib/quantenSStelleNachweis.ts) reicht
// quanten_w_s_faelschung_anmelden und die Teile von
// quanten_w_s_faelschung_offenlegen ein. Hier prueft der Knoten die Koerper
// mit SEINEM Code: wie handleQuantenW gelesen (keine fremden Felder, nur die
// Felder dieser Art), die Nachricht (quantenSFaelschungAnmeldenNachricht mit
// den Fehlversuchen) und die Unterschrift der Stelle, die Vorlage der
// Inhaberin (quantenSVorlageText), je Teil die Werte (quantenSWerteLesen: keine
// Anteile) und ihre Pfade gegen die Wurzel (quantenSWerteAuswerten) -- die
// Vorlage besteht die Stichprobe. Gegenproben: ein Wert falsch, ein Pfad
// falsch, ein anderer Zaehler der Fehlversuche, Anteile dazu.

type qsFaelschungVektor struct {
	K         string   `json:"k"`
	N         int      `json:"n"`
	B         int      `json:"b"`
	C         int      `json:"c"`
	D         string   `json:"d"`
	Tol       int      `json:"tol"`
	Wallet    string   `json:"wallet"`
	Inhaberin string   `json:"inhaberin"`
	X         string   `json:"x"`
	Probe     []int    `json:"probe"`
	Vorlage   string   `json:"vorlage"`
	Fehl      int      `json:"fehl"`
	Wurzel    string   `json:"wurzel"`
	Nachricht string   `json:"nachricht"`
	WalletSig string   `json:"wallet_sig"`
	Anmelden  string   `json:"koerper_anmelden"`
	Offenlege []string `json:"koerper_offenlegen"`
}

type fwKoerper struct {
	Wallet   string          `json:"wallet"`
	Zeit     int64           `json:"zeit"`
	QuantenW QuantenWAngaben `json:"quanten_w"`
}

func fwKoerperLesen(t *testing.T, s string) fwKoerper {
	t.Helper()
	var k fwKoerper
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&k); err != nil {
		t.Fatalf("Koerper wie handleQuantenW: %v", err)
	}
	return k
}

func TestQuantenSFaelschungVektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_faelschung_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qsFaelschungVektor
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	x, ok := quantenWSBitsLesen(v.X, v.N)
	if !ok || len(v.D) != v.N {
		t.Fatal("x oder d")
	}
	bs := string(rune('0' + v.B))

	// Anmelden: Koerper, Nachricht, Unterschrift der Stelle, Vorlage der Inhaberin.
	a := fwKoerperLesen(t, v.Anmelden)
	tx := Transaction{Type: quantenSFaelschungAnmeldenTxArt, Wallet: a.Wallet, DistributionAt: a.Zeit, QuantenW: &a.QuantenW}
	if !quantenWNurEigeneFelder(&tx) || a.Wallet != v.Wallet || a.QuantenW.K != v.K || a.QuantenW.B != bs || a.QuantenW.X != v.X {
		t.Fatalf("Anmelden: Felder %+v", a)
	}
	nachricht := quantenSFaelschungAnmeldenNachricht(v.Wallet, v.K, bs, v.X, a.QuantenW.Probe, v.Fehl)
	if nachricht != v.Nachricht {
		t.Fatalf("Nachricht:\n  App    %s\n  Knoten %s", v.Nachricht, nachricht)
	}
	if err := pruefePersonalSignGemerkt(nachricht, a.QuantenW.WalletSig, v.Wallet); err != nil {
		t.Fatalf("Unterschrift der Stelle: %v", err)
	}
	if pruefePersonalSignGemerkt(quantenSFaelschungAnmeldenNachricht(v.Wallet, v.K, bs, v.X, a.QuantenW.Probe, v.Fehl+1), a.QuantenW.WalletSig, v.Wallet) == nil {
		t.Fatal("Gegenprobe: Unterschrift gilt auch mit anderem Zaehler der Fehlversuche")
	}
	if err := pruefePersonalSignGemerkt(quantenSVorlageText(v.K, v.B, x), a.QuantenW.Vorlage, v.Inhaberin); err != nil {
		t.Fatalf("Vorlage der Inhaberin: %v", err)
	}

	// Offenlegen: Teile in Folge, nur Werte, jede Stelle gegen die Wurzel.
	wurzel, ok := quantenWHex32(v.Wurzel)
	if !ok {
		t.Fatal("Wurzel")
	}
	gesamtT, gesamtF, ab := 0, 0, 0
	for n, s := range v.Offenlege {
		o := fwKoerperLesen(t, s)
		tx := Transaction{Type: quantenSFaelschungOffenlegenTxArt, Wallet: o.Wallet, DistributionAt: o.Zeit, QuantenW: &o.QuantenW}
		if !quantenWNurEigeneFelder(&tx) || o.QuantenW.K != v.K || o.QuantenW.Ab != ab || len(o.QuantenW.Offen) == 0 || len(o.QuantenW.Offen) > quantenWOffenTeil {
			t.Fatalf("Teil %d: Felder (ab %d, erwartet %d)", n, o.QuantenW.Ab, ab)
		}
		werte, ok := quantenSWerteLesen(o.QuantenW.Offen)
		if !ok {
			t.Fatalf("Teil %d: Werte nicht lesbar", n)
		}
		stellen := v.Probe[ab : ab+len(werte)]
		basen := make([]int, len(stellen))
		antwort := make([]int, len(stellen))
		for j, i := range stellen {
			basen[j] = int(v.D[i]-'0') ^ v.B ^ v.C
			antwort[j] = x[i]
		}
		tr, fe, err := quantenSWerteAuswerten(v.K, v.N, wurzel, stellen, basen, antwort, werte)
		if err != nil {
			t.Fatalf("Teil %d: %v", n, err)
		}
		gesamtT, gesamtF = gesamtT+tr, gesamtF+fe
		if n == 0 {
			// Gegenproben am ersten Teil.
			falsch := append([]quantenSWertStelle(nil), werte...)
			falsch[0] = quantenSWertStelle{W: new(big.Int).Xor(werte[0].W, big.NewInt(2)), Pfad: werte[0].Pfad}
			if _, _, err := quantenSWerteAuswerten(v.K, v.N, wurzel, stellen, basen, antwort, falsch); err == nil {
				t.Fatal("Gegenprobe: falscher Wert angenommen")
			}
			pfad := append([][32]byte(nil), werte[0].Pfad...)
			pfad[0][0] ^= 1
			falsch[0] = quantenSWertStelle{W: werte[0].W, Pfad: pfad}
			if _, _, err := quantenSWerteAuswerten(v.K, v.N, wurzel, stellen, basen, antwort, falsch); err == nil {
				t.Fatal("Gegenprobe: falscher Pfad angenommen")
			}
			mitAnteil := append([]QuantenWOffen(nil), o.QuantenW.Offen...)
			mitAnteil[0].Anteile = []QuantenWAnteilAngabe{{X: 1, Y: "1"}}
			if _, ok := quantenSWerteLesen(mitAnteil); ok {
				t.Fatal("Gegenprobe: Wert mit Anteilen angenommen")
			}
			if len(o.QuantenW.Offen[0].Pfad) == 0 || len(hex.EncodeToString(werte[0].Pfad[0][:])) != 64 {
				t.Fatal("Pfad leer")
			}
		}
		ab += len(werte)
	}
	if ab != len(v.Probe) {
		t.Fatalf("offengelegt %d von %d", ab, len(v.Probe))
	}
	if quantenWUrteil(gesamtT, gesamtF, v.Tol) != quantenWEcht {
		t.Fatalf("die Vorlage besteht die Stichprobe nicht (%d Treffer, %d Fehler)", gesamtT, gesamtF)
	}
}
