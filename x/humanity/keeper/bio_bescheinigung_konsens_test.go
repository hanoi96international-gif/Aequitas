package keeper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	// Poseidon(987654321987654321987654321), von der Schaltung biometric_v3
	// gerechnet -- die Kette rechnet kein Poseidon mehr, sie prueft
	// Unterschriften ueber den Nullifier des Beweises.
	bTestNullifier = "2258162185744682068870376411042672036729508593423772695192684261112116008409"
	bTestZeit      = int64(1_800_000_000)
)

func bTestTx(t *testing.T, nullifier string, sigs ...string) Transaction {
	t.Helper()
	return Transaction{Type: "register_human", Wallet: bTestWallet, RegAt: bTestZeit + 30,
		PubSignals:       []string{"1", nullifier},
		BioBescheinigung: &BioBescheinigung{Zeit: bTestZeit, Modus: "test", Signaturen: sigs}}
}

func bindung(k bTestSchluessel, modus, nullifier, wallet string, zeit int64) string {
	return k.unterschreibe("aequitas-bio-attestation-v3|" + modus + "|" + nullifier + "|" + wallet + "|" + strconv.FormatInt(zeit, 10))
}

func aussage(k bTestSchluessel, modus, nullifier string, zeit int64) string {
	return k.unterschreibe("aequitas-bio-personhood-v3|" + modus + "|" + nullifier + "|" + strconv.FormatInt(zeit, 10))
}

// Pruefvektoren, byte-gleich zu den Gegenproben in aequitas-biometric-beta
// (coordinator/tests/test_bescheinigung_v3.py, matching-service/tests/
// test_personhood_v3.py). Weicht ein Zeichen ab, prueft keine Unterschrift.
func TestBescheinigung_NachrichtenV3(t *testing.T) {
	n := "9904028930859697121695025471312564917337032846528014134060777877259199866166" // Poseidon(123)
	if got := string(bindungNachrichtV3("test", n, "0xabcd000000000000000000000000000000000001", 1787712345)); got !=
		"aequitas-bio-attestation-v3|test|"+n+"|0xabcd000000000000000000000000000000000001|1787712345" {
		t.Fatalf("Bindung: %s", got)
	}
	if got := string(aussageNachrichtV3("test", n, 1787712345)); got != "aequitas-bio-personhood-v3|test|"+n+"|1787712345" {
		t.Fatalf("Aussage: %s", got)
	}
}

// bio (= die bio_hash des Coordinators) kommt in keinen Block: mit ihr fragt
// ein Mensch seine Einschreibung ab und loescht sie.
func TestBescheinigung_KeinBioImBlock(t *testing.T) {
	tx := bTestTx(t, bTestNullifier, "aa")
	roh, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(roh), `"bio"`) || strings.Contains(string(roh), "987654321987654321987654321") {
		t.Fatalf("bio im Block: %s", roh)
	}
	// Auch ein Erzeuger, der "bio" mitschickt, bringt es nicht in den Zustand:
	// das Feld gibt es nicht.
	var zurueck Transaction
	if err := json.Unmarshal([]byte(`{"type":"register_human","bio_bescheinigung":{"bio":"123","zeit":1,"modus":"test","sig":["aa"]}}`), &zurueck); err != nil {
		t.Fatal(err)
	}
	if neu, _ := json.Marshal(zurueck.BioBescheinigung); strings.Contains(string(neu), "bio") {
		t.Fatalf("bio uebernommen: %s", neu)
	}
}

func TestBescheinigung_GueltigUndMissbrauch(t *testing.T) {
	coord, zeuge, fremd := neuerEd25519(t), neuerEd25519(t), neuerEd25519(t)
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{coord.pub}, Bezeuger: []string{zeuge.pub}})

	gut := bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit))
	if err := pruefeBioBescheinigung(gut); err != nil {
		t.Fatalf("gueltige Bescheinigung abgelehnt: %v", err)
	}

	faelle := map[string]Transaction{
		"ohne Bescheinigung": func() Transaction { x := gut; x.BioBescheinigung = nil; return x }(),
		"Unterschriften ueber einen anderen Nullifier": func() Transaction {
			x := bTestTx(t, bTestNullifier, bindung(coord, "test", "111", bTestWallet, bTestZeit), aussage(zeuge, "test", "111", bTestZeit))
			return x
		}(),
		"alte Form v2 ueber bio": func() Transaction {
			bio := "987654321987654321987654321"
			return bTestTx(t, bTestNullifier,
				coord.unterschreibe("aequitas-bio-attestation-v2|test|"+bio+"|"+bTestWallet+"|"+strconv.FormatInt(bTestZeit, 10)),
				zeuge.unterschreibe("aequitas-bio-personhood-v2|test|"+bio+"|"+strconv.FormatInt(bTestZeit, 10)))
		}(),
		"Nullifier ausserhalb des Feldes": func() Transaction {
			gross := "21888242871839275222246405745257275088548364400416034343698204186575808495617"
			return bTestTx(t, gross, bindung(coord, "test", gross, bTestWallet, bTestZeit), aussage(zeuge, "test", gross, bTestZeit))
		}(),
		"Nullifier nicht dezimal": func() Transaction {
			return bTestTx(t, "0x1f", bindung(coord, "test", "0x1f", bTestWallet, bTestZeit), aussage(zeuge, "test", "0x1f", bTestZeit))
		}(),
		"ohne Nullifier": func() Transaction {
			x := gut
			x.PubSignals = []string{"1"}
			return x
		}(),
		"Bescheinigung einer anderen Wallet": func() Transaction {
			x := bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, "0xb0d1000000000000000000000000000000000666", bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit))
			return x
		}(),
		"fremder Schluessel bindet":      bTestTx(t, bTestNullifier, bindung(fremd, "test", bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit)),
		"Vergleichsdienst bindet selbst": bTestTx(t, bTestNullifier, bindung(zeuge, "test", bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit)),
		"nur Bindung, kein Zeuge":        bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit)),
		"Coordinator bezeugt doppelt":    bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit), aussage(coord, "test", bTestNullifier, bTestZeit)),
		"doppelte Unterschrift": bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit),
			aussage(zeuge, "test", bTestNullifier, bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit)),
		"anderer Modus": func() Transaction {
			x := bTestTx(t, bTestNullifier, bindung(coord, "real", bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge, "real", bTestNullifier, bTestZeit))
			x.BioBescheinigung.Modus = "real"
			return x
		}(),
		"Zeitpunkt umgeschrieben": func() Transaction {
			x := bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge, "test", bTestNullifier, bTestZeit))
			x.BioBescheinigung.Zeit = bTestZeit + 1
			return x
		}(),
		"zu alt fuer die Annahme": func() Transaction {
			x := gut
			x.RegAt = bTestZeit + bescheinigungHoechstensAlt + 1
			return x
		}(),
		"Nullifier mit fuehrender Null, unterschrieben so": func() Transaction {
			return bTestTx(t, "0"+bTestNullifier, bindung(coord, "test", "0"+bTestNullifier, bTestWallet, bTestZeit), aussage(zeuge, "test", "0"+bTestNullifier, bTestZeit))
		}(),
		"Unterschrift Grossbuchstaben": bTestTx(t, bTestNullifier, strings.ToUpper(bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit)), aussage(zeuge, "test", bTestNullifier, bTestZeit)),
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
	if err := pruefeBioBescheinigung(bTestTx(t, bTestNullifier, bindung(coord, "test", bTestNullifier, bTestWallet, bTestZeit))); err != nil {
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
		"Quorum ueber 17 (nie erreichbar)": func() string {
			var ks []string
			for i := 0; i < bescheinigungMaxSignaturen+1; i++ {
				ks = append(ks, `"`+neuerEd25519(t).pub+`"`)
			}
			return `{"bescheinigung":{"ab_unix":1,"modus":"test","quorum":18,"coordinator":[` + strings.Join(ks, ",") + `]}}`
		}(),
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

// Der Rumpf eines /prove: nur die v3-Eintraege zaehlen, jeder Schluessel
// genau so geschrieben. Ein zweites "BioAttestation" neben "bioAttestation"
// macht ihn mehrdeutig -- keine Bescheinigung.
func TestBescheinigung_AusProveAnfrageEindeutig(t *testing.T) {
	ok := bescheinigungAusProveAnfrage([]byte(`{"bio":"123","wallet":"0x1","bioAttestation":"cc,v3:AA, v3:bb ,dd","bioAttestationIssuedAt":1800000000}`))
	if ok == nil || ok.Zeit != 1_800_000_000 || len(ok.Signaturen) != 2 || ok.Signaturen[0] != "aa" || ok.Signaturen[1] != "bb" {
		t.Fatalf("gueltiger Rumpf nicht gelesen: %+v", ok)
	}
	for name, body := range map[string]string{
		"nur v2":                          `{"bio":"123","bioAttestation":"aa,bb","bioAttestationIssuedAt":1800000000}`,
		"doppelt verschieden geschrieben": `{"bioAttestation":"v3:aa","BioAttestation":"v3:bb","bioAttestationIssuedAt":1}`,
		"doppelt gleich":                  `{"bioAttestation":"v3:aa","bioAttestation":"v3:bb","bioAttestationIssuedAt":1}`,
		"ohne Unterschriften":             `{"bio":"123","bioAttestationIssuedAt":1800000000}`,
		"ohne Zeitpunkt":                  `{"bioAttestation":"v3:aa"}`,
		"Muell hinter dem Objekt":         `{"bioAttestation":"v3:aa","bioAttestationIssuedAt":1800000000} {}`,
		"zu viele":                        `{"bioAttestation":"` + strings.Repeat("v3:aa,", bescheinigungMaxSignaturen+1) + `","bioAttestationIssuedAt":1}`,
	} {
		if b := bescheinigungAusProveAnfrage([]byte(body)); b != nil {
			t.Errorf("%s: Bescheinigung gelesen: %+v", name, b)
		}
	}
}

// An den Proof-Server geht der Rumpf ohne die v3-Eintraege -- er kennt sie
// nicht und wiese die ganze Anfrage ab. Alles andere bleibt Byte fuer Byte.
func TestBescheinigung_ProofServerSiehtKeinV3(t *testing.T) {
	for name, fall := range map[string]struct{ rein, raus string }{
		"gemischt": {`{"bio":"123","wallet":"0xAb","bioAttestation":"aa,v3:cc,bb,v3:dd","bioAttestationIssuedAt":17,"x":[1,2]}`,
			`{"bio":"123","wallet":"0xAb","bioAttestation":"aa,bb","bioAttestationIssuedAt":17,"x":[1,2]}`},
		"Leerraum":                    {"{ \"bioAttestation\" :  \"v3:cc,aa\" , \"bio\":\"1\"}", "{ \"bioAttestation\" :  \"aa\" , \"bio\":\"1\"}"},
		"nur v3":                      {`{"bioAttestation":"v3:cc"}`, `{"bioAttestation":""}`},
		"ohne v3":                     {`{"bioAttestation":"aa,bb","bio":"1"}`, `{"bioAttestation":"aa,bb","bio":"1"}`},
		"Zeichen davor im Schluessel": {`{"a\"bioAttestation":"v3:cc","bioAttestation":"v3:dd,aa"}`, `{"a\"bioAttestation":"v3:cc","bioAttestation":"aa"}`},
		// Mehrdeutig: unveraendert weiter, der Proof-Server weist ab (fail-closed).
		"doppelt":     {`{"bioAttestation":"v3:cc","BioAttestation":"aa"}`, `{"bioAttestation":"v3:cc","BioAttestation":"aa"}`},
		"kein Objekt": {`[1]`, `[1]`},
	} {
		if got := string(proveAnfrageOhneV3([]byte(fall.rein))); got != fall.raus {
			t.Errorf("%s:\n rein %s\n raus %s\n soll %s", name, fall.rein, got, fall.raus)
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
