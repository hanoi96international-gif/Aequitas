package keeper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Missbrauchstests zu K1 (Audit 2026-10-21): die Bescheinigung im Konsens.

func setBescheinigungFuerTest(t *testing.T, k *bescheinigungKonfig) {
	t.Helper()
	bescheinigungTestMu.Lock()
	bescheinigungTest = k
	bescheinigungTestMu.Unlock()
	t.Cleanup(func() {
		bescheinigungTestMu.Lock()
		bescheinigungTest = nil
		bescheinigungTestMu.Unlock()
	})
}

type bTestSchluessel struct {
	pub  string
	priv ed25519.PrivateKey
}

func neuerEd25519(t *testing.T) bTestSchluessel {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return bTestSchluessel{pub: hex.EncodeToString(pub), priv: priv}
}

func (s bTestSchluessel) unterschreibe(msg string) string {
	return hex.EncodeToString(ed25519.Sign(s.priv, []byte(msg)))
}

const (
	bTestWallet = "0xb0d1000000000000000000000000000000000001"
	bTestBio    = "987654321987654321987654321"
	bTestZeit   = int64(1_800_000_000)
)

func bTestTx(t *testing.T, bio string, sigs ...string) Transaction {
	t.Helper()
	b, _ := new(big.Int).SetString(bio, 10)
	n, err := poseidonNullifier(b)
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{Type: "register_human", Wallet: bTestWallet, RegAt: bTestZeit + 30,
		PubSignals:       []string{"1", n.String()},
		BioBescheinigung: &BioBescheinigung{Bio: bio, Zeit: bTestZeit, Modus: "test", Signaturen: sigs}}
}

func bindung(k bTestSchluessel, modus, bio, wallet string, zeit int64) string {
	return k.unterschreibe("aequitas-bio-attestation-v2|" + modus + "|" + bio + "|" + wallet + "|" + strconv.FormatInt(zeit, 10))
}

func aussage(k bTestSchluessel, modus, bio string, zeit int64) string {
	return k.unterschreibe("aequitas-bio-personhood-v2|" + modus + "|" + bio + "|" + strconv.FormatInt(zeit, 10))
}

// Poseidon wie der Circuit: Werte aus der Witness-Berechnung von
// aequitas-proof-server/biometric_v3_js/biometric_v3.wasm (22.10.2026).
func TestBescheinigung_PoseidonWieCircuit(t *testing.T) {
	for bio, erwartet := range map[string]string{
		"12345": "4267533774488295900887461483015112262021273608761099826938271132511348470966",
		"21888242871839275222246405745257275088548364400416034343698204186575808495616": "3366645945435192953002076803303112651887535928162668198103357554665518664470",
		"987654321987654321987654321": "2258162185744682068870376411042672036729508593423772695192684261112116008409",
	} {
		b, _ := new(big.Int).SetString(bio, 10)
		n, err := poseidonNullifier(b)
		if err != nil || n.String() != erwartet {
			t.Fatalf("Poseidon(%s) = %v (%v), Circuit sagt %s", bio, n, err, erwartet)
		}
	}
}

func TestBescheinigung_GueltigUndMissbrauch(t *testing.T) {
	coord, zeuge, fremd := neuerEd25519(t), neuerEd25519(t), neuerEd25519(t)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{coord.pub}, Bezeuger: []string{zeuge.pub}})

	gut := bTestTx(t, bTestBio, bindung(coord, "test", bTestBio, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestBio, bTestZeit))
	if err := pruefeBioBescheinigung(gut); err != nil {
		t.Fatalf("gueltige Bescheinigung abgelehnt: %v", err)
	}

	faelle := map[string]Transaction{
		"ohne Bescheinigung": func() Transaction { x := gut; x.BioBescheinigung = nil; return x }(),
		"erfundenes bio (Nullifier passt nicht)": func() Transaction {
			x := bTestTx(t, "111", bindung(coord, "test", bTestBio, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestBio, bTestZeit))
			x.PubSignals = gut.PubSignals
			return x
		}(),
		"Bescheinigung einer anderen Wallet": func() Transaction {
			x := bTestTx(t, bTestBio, bindung(coord, "test", bTestBio, "0xb0d1000000000000000000000000000000000666", bTestZeit), aussage(zeuge, "test", bTestBio, bTestZeit))
			return x
		}(),
		"fremder Schluessel bindet":      bTestTx(t, bTestBio, bindung(fremd, "test", bTestBio, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestBio, bTestZeit)),
		"Vergleichsdienst bindet selbst": bTestTx(t, bTestBio, bindung(zeuge, "test", bTestBio, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestBio, bTestZeit)),
		"nur Bindung, kein Zeuge":        bTestTx(t, bTestBio, bindung(coord, "test", bTestBio, bTestWallet, bTestZeit)),
		"Coordinator bezeugt doppelt":    bTestTx(t, bTestBio, bindung(coord, "test", bTestBio, bTestWallet, bTestZeit), aussage(coord, "test", bTestBio, bTestZeit)),
		"doppelte Unterschrift": bTestTx(t, bTestBio, bindung(coord, "test", bTestBio, bTestWallet, bTestZeit),
			aussage(zeuge, "test", bTestBio, bTestZeit), aussage(zeuge, "test", bTestBio, bTestZeit)),
		"anderer Modus": func() Transaction {
			x := bTestTx(t, bTestBio, bindung(coord, "real", bTestBio, bTestWallet, bTestZeit), aussage(zeuge, "real", bTestBio, bTestZeit))
			x.BioBescheinigung.Modus = "real"
			return x
		}(),
		"Zeitpunkt umgeschrieben": func() Transaction {
			x := bTestTx(t, bTestBio, bindung(coord, "test", bTestBio, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestBio, bTestZeit))
			x.BioBescheinigung.Zeit = bTestZeit + 1
			return x
		}(),
		"zu alt fuer die Annahme": func() Transaction {
			x := gut
			x.RegAt = bTestZeit + bescheinigungHoechstensAlt + 1
			return x
		}(),
		"bio mit fuehrender Null": func() Transaction {
			x := bTestTx(t, bTestBio, bindung(coord, "test", "0"+bTestBio, bTestWallet, bTestZeit), aussage(zeuge, "test", "0"+bTestBio, bTestZeit))
			x.BioBescheinigung.Bio = "0" + bTestBio
			return x
		}(),
		"Unterschrift Grossbuchstaben": bTestTx(t, bTestBio, strings.ToUpper(bindung(coord, "test", bTestBio, bTestWallet, bTestZeit)), aussage(zeuge, "test", bTestBio, bTestZeit)),
	}
	for name, tx := range faelle {
		if err := pruefeBioBescheinigung(tx); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
}

// Ohne eigene Bezeugerliste gilt die Coordinator-Liste auch fuer die Aussage
// -- dann zaehlt EIN Schluessel einmal, nie zweimal.
func TestBescheinigung_EinzelbetriebZaehltSchluesselNichtSignaturen(t *testing.T) {
	coord := neuerEd25519(t)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 1, Coordinator: []string{coord.pub}})
	if err := pruefeBioBescheinigung(bTestTx(t, bTestBio, bindung(coord, "test", bTestBio, bTestWallet, bTestZeit))); err != nil {
		t.Fatalf("Quorum 1 mit einem Coordinator abgelehnt: %v", err)
	}
	k := bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2, Coordinator: []string{coord.pub}}
	if k.pruefen() == nil {
		t.Fatal("Quorum 2 mit nur einem Schluessel als Konfiguration angenommen")
	}
}

func TestBescheinigung_KonfigurationFailClosed(t *testing.T) {
	klein := "0100000000000000000000000000000000000000000000000000000000000000" // neutrales Element
	gut := neuerEd25519(t).pub
	for name, json := range map[string]string{
		"Schluessel kleiner Ordnung": `{"bescheinigung":{"ab_unix":1,"modus":"test","quorum":1,"coordinator":["` + klein + `"]}}`,
		"kein Coordinator":           `{"bescheinigung":{"ab_unix":1,"modus":"test","quorum":1,"coordinator":[]}}`,
		"Modus unbekannt":            `{"bescheinigung":{"ab_unix":1,"modus":"echt","quorum":1,"coordinator":["` + gut + `"]}}`,
		"Quorum 0":                   `{"bescheinigung":{"ab_unix":1,"modus":"test","quorum":0,"coordinator":["` + gut + `"]}}`,
		"Tippfehler im Feld":         `{"bescheinigung":{"ab_unix":1,"modus":"test","quorum":1,"coordinatoren":["` + gut + `"]}}`,
		"Grossbuchstaben":            `{"bescheinigung":{"ab_unix":1,"modus":"test","quorum":1,"coordinator":["` + strings.ToUpper(gut) + `"]}}`,
	} {
		if _, err := ladeBescheinigungKonfig([]byte(json)); err == nil {
			t.Errorf("%s: Konfiguration angenommen", name)
		}
	}
	k, err := ladeBescheinigungKonfig([]byte(`{"chain_id":"aequitas-1"}`))
	if err != nil || k.AbUnix != 0 {
		t.Fatalf("ohne Eintrag muss die Regel schlafen: %+v, %v", k, err)
	}
	if bescheinigungAktiv(4_000_000_000) && k.AbUnix == 0 && bescheinigungAbOverride.Load() == 0 {
		t.Fatal("schlafende Regel aktiv")
	}
}

// Die Repo-Genesis muss lesbar bleiben (sonst startet kein Knoten).
func TestBescheinigung_RepoGenesisLesbar(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ladeBescheinigungKonfig(data); err != nil {
		t.Fatalf("genesis.json: %v", err)
	}
}

// Der Rumpf eines /prove: jeder Schluessel genau so geschrieben. Ein
// zweites "Bio" neben "bio" macht ihn mehrdeutig -- keine Bescheinigung.
func TestBescheinigung_AusProveAnfrageEindeutig(t *testing.T) {
	ok := bescheinigungAusProveAnfrage([]byte(`{"bio":"123","wallet":"0x1","bioAttestation":"AA,bb","bioAttestationIssuedAt":1800000000}`))
	if ok == nil || ok.Bio != "123" || ok.Zeit != 1_800_000_000 || len(ok.Signaturen) != 2 || ok.Signaturen[0] != "aa" {
		t.Fatalf("gueltiger Rumpf nicht gelesen: %+v", ok)
	}
	for name, body := range map[string]string{
		"doppelt verschieden geschrieben": `{"bio":"123","Bio":"999","bioAttestation":"aa","bioAttestationIssuedAt":1}`,
		"doppelt gleich":                  `{"bio":"123","bio":"999","bioAttestation":"aa","bioAttestationIssuedAt":1}`,
		"ohne Unterschriften":             `{"bio":"123","bioAttestationIssuedAt":1800000000}`,
		"bio als Zahl":                    `{"bio":123,"bioAttestation":"aa","bioAttestationIssuedAt":1800000000}`,
		"Muell hinter dem Objekt":         `{"bio":"123","bioAttestation":"aa","bioAttestationIssuedAt":1800000000} {}`,
	} {
		if b := bescheinigungAusProveAnfrage([]byte(body)); b != nil {
			t.Errorf("%s: Bescheinigung gelesen: %+v", name, b)
		}
	}
}

// Ab dem Stichtag legt der Erzeuger keine Registrierung ohne Bescheinigung
// mehr in einen Block.
func TestBescheinigung_BlockOhneBescheinigungNichtTauglich(t *testing.T) {
	bescheinigungAbOverride.Store(1_000)
	t.Cleanup(func() { bescheinigungAbOverride.Store(0) })
	tx := Transaction{Type: "register_human", Wallet: bTestWallet}
	if err := blockTauglich(&tx, 999); err != nil {
		t.Fatalf("vor dem Stichtag abgewiesen: %v", err)
	}
	if err := blockTauglich(&tx, 1_000); err == nil {
		t.Fatal("ab dem Stichtag ohne Bescheinigung tauglich")
	}
}
