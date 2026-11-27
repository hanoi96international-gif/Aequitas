package keeper

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// PRUEFVEKTOR: MELDEN UND ZIEHEN AUS DER APP (Weg C, C2b-2b-3c).
//
// Die App als Inhaberin (lib/quantenS3Inhaberin.ts) baut die Meldung aus der
// Messung ihres Empfaengers (je Abschnitt eines Senders mindestens gamma_det),
// kodiert und unterschreibt sie und reicht Melden und Ziehen ein.
// testdata/quanten_s3_inhaberin_vektor.json entsteht in der App
// (lib/__tests__/quantenS3Inhaberin.test.ts, Schluessel NUR fuer Tests); hier
// nimmt der Knoten genau diese Koerper mit SEINEM Code an -- Ziehen mit dem
// echten quicknet-Beacon -- und zieht dieselbe Teilmenge.

type qs3InhaberinVektor struct {
	Hinweis       string `json:"hinweis"`
	Schluessel    string `json:"schluessel_nur_test"`
	Wallet        string `json:"wallet"`
	K             string `json:"k"`
	NPuls         int    `json:"n_puls"`
	Sender        int    `json:"sender"`
	Gamma         int    `json:"gamma"`
	Meldezeit     int64  `json:"meldezeit"`
	KoerperMelden string `json:"koerper_melden"`
	Runde         uint64 `json:"runde"`
	RundeSig      string `json:"runde_sig"`
	KoerperZiehen string `json:"koerper_ziehen"`
	Teil          []int  `json:"teil"`
}

func TestQuantenS3InhaberinVektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s3_inhaberin_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qs3InhaberinVektor
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	key, err := crypto.HexToECDSA(v.Schluessel)
	if err != nil || quantenSAdresse(key) != v.Wallet {
		t.Fatal("Schluessel gehoert nicht zur Wallet")
	}
	echt := drandQuicknet
	w := neueS3WeltN(t, 64)
	drandQuicknet = echt // Ziehen mit dem echten Beacon (Cleanup setzt ihn wieder)
	// Die Zeit des Beacons (2023) liegt vor den echten Stichtagen: alle auf 1.
	geldVorher := quantenGeldAbOverride.Load()
	t.Cleanup(func() { quantenGeldAbOverride.Store(geldVorher) })
	for _, o := range []*atomic.Int64{&quantenGeldAbOverride, &quantenWAbOverride, &quantenLeitungAbOverride, &quantenS3AbOverride} {
		o.Store(1)
	}
	w.leitungSetzen(20, 50_000, v.Gamma, 1)
	w.sender, w.z = v.Sender, v.Meldezeit-100

	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], v.K, v.Wallet, v.NPuls, s3Deckung, w.z), w.z)
	melden := qsKoerperLesen(t, quantenS3MeldenTxArt, v.KoerperMelden)
	w.annehmen(melden, v.Meldezeit)
	vm := w.vs(w.cs, v.K)
	if r, _ := quantenS3ZiehRunde(vm.MeldungZeit); vm.Gemeldet == 0 || vm.MeldungZeit != v.Meldezeit || r != v.Runde {
		t.Fatalf("Meldung: %+v", vm)
	}
	// Dieselbe Meldung noch einmal: abgewiesen (genau einmal).
	w.abgewiesen("zweite Meldung", qsKoerperLesen(t, quantenS3MeldenTxArt, v.KoerperMelden), v.Meldezeit+1, "quanten_s3_doppelt")

	ziehen := qsKoerperLesen(t, quantenS3ZiehenTxArt, v.KoerperZiehen)
	if ziehen.QuantenW.Runde != v.Runde || ziehen.QuantenW.RundeSig != v.RundeSig {
		t.Fatal("Runde im Koerper")
	}
	w.annehmen(ziehen, v.Meldezeit+quantenS3ZiehAbstand+5)
	vm = w.vs(w.cs, v.K)
	if !reflect.DeepEqual(vm.Teil, v.Teil) {
		t.Fatalf("Teilmenge: Knoten %d Pulse, App %d", len(vm.Teil), len(v.Teil))
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
		t.Fatal("Knoten weichen ab")
	}
}
