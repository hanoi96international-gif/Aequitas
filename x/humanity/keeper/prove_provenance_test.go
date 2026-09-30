package keeper

import (
	"testing"
	"time"
)

// Die Luecke: der Vertrag prueft, dass ein Beweis GUELTIG ist -- nicht, woher
// er kommt. Groth16-Proving-Keys sind oeffentlich, dieser liegt im Repo. Wer
// sich eine bio_hash wuerfelt, lokal einen Beweis erzeugt und direkt an
// /api/register geht, praegte 1.000 AEQ. Und nochmal.

const (
	walletAlice   = "0xa11ce00000000000000000000000000000000001"
	walletMallory = "0x3a11020000000000000000000000000000000666"
)

func proveAnfrage(wallet string) []byte {
	return []byte(`{"wallet":"` + wallet + `","bio":"1","salt":"2"}`)
}

func TestSelbstErzeugterBeweisHatKeineHerkunft(t *testing.T) {
	// DER Fall. Nichts hat diesen Nullifier je bei /prove gesehen.
	if hatProveHerkunft("0xdeadbeef00000000000000000000000000000000000000000000000000000000", walletAlice) {
		t.Fatal("ein Nullifier, der nie durch /prove kam, darf keine Herkunft haben")
	}
}

func TestEineGeprueteAntwortHinterlaesstEineHerkunft(t *testing.T) {
	merkeProveHerkunft(proveAnfrage(walletAlice), []byte(`{"zkNullifier":"0xABC123","circuitVersion":3}`))
	if !hatProveHerkunft("0xabc123", walletAlice) {
		t.Fatal("nach einem erfolgreichen /prove muss die Herkunft stehen")
	}
	// Gross-/Kleinschreibung und 0x duerfen nicht entscheiden -- weder beim
	// Nullifier noch bei der Wallet.
	if !hatProveHerkunft("ABC123", "0xA11CE00000000000000000000000000000000001") {
		t.Fatal("die Schreibweise darf ueber die Registrierung nicht entscheiden")
	}
}

func TestMissbrauch_AbgefangenerBeweisFuerDieEigeneWallet(t *testing.T) {
	// Alice besteht /prove. Mallory faengt die Antwort ab, bevor Alice sie
	// benutzt, und reicht sie mit SEINER Signatur fuer SEINE Wallet ein. Der
	// Nullifier hat Herkunft, der Beweis ist gueltig, die Signatur passt zu
	// Mallory -- nur die Herkunft gehoert zu Alice.
	merkeProveHerkunft(proveAnfrage(walletAlice), []byte(`{"zkNullifier":"0xF00D01","circuitVersion":3}`))
	if hatProveHerkunft("0xf00d01", walletMallory) {
		t.Fatal("die Herkunft eines Beweises darf nur fuer die Wallet gelten, fuer die /prove ihn erzeugt hat")
	}
	if !hatProveHerkunft("0xf00d01", walletAlice) {
		t.Fatal("fuer Alice selbst muss die Herkunft weiter gelten")
	}
}

func TestMissbrauch_OhneOderMitKaputterWalletWirdNichtsGemerkt(t *testing.T) {
	// Ohne Wallet in der Anfrage kennt der Knoten nicht, fuer wen der Beweis
	// ist -- dann lieber keine Herkunft (Registrierung scheitert, umkehrbar).
	for i, anfrage := range []string{`{}`, `{"wallet":""}`, `{"wallet":"0x123"}`, `{"wallet":42}`, `kein JSON`} {
		n := []string{"0xB0001", "0xB0002", "0xB0003", "0xB0004", "0xB0005"}[i]
		merkeProveHerkunft([]byte(anfrage), []byte(`{"zkNullifier":"`+n+`","circuitVersion":3}`))
		for _, w := range []string{walletAlice, "", "0x123"} {
			if hatProveHerkunft(n, w) {
				t.Fatalf("Anfrage %s: ohne gueltige Wallet darf keine Herkunft entstehen", anfrage)
			}
		}
	}
}

func TestOhneNullifierWirdNichtsGemerkt(t *testing.T) {
	merkeProveHerkunft(proveAnfrage(walletAlice), []byte(`{"error":"bio attestation rejected"}`))
	merkeProveHerkunft(proveAnfrage(walletAlice), []byte(`kein JSON`))
	if hatProveHerkunft("", walletAlice) {
		t.Fatal("eine leere Kennung darf nie als Herkunft gelten")
	}
}

func TestEineAlteHerkunftVerfaellt(t *testing.T) {
	// Sonst waere eine einmal gepruefte Registrierung beliebig lange
	// einloesbar -- und eine abgefangene Antwort ein dauerhafter Freifahrtschein.
	proveHerkunft.Store(herkunftsSchluessel("0xa1f0"), herkunft{zeit: time.Now().Add(-proveHerkunftTTL - time.Minute), wallet: walletAlice})
	if hatProveHerkunft("0xa1f0", walletAlice) {
		t.Fatal("eine abgelaufene Herkunft darf nicht mehr zaehlen")
	}
}

func TestDieVoreinstellungVerlangtHerkunft(t *testing.T) {
	// Ein Tor, das gebaut aber nicht eingeschaltet ist, schuetzt niemanden --
	// und dieses steht vor der Stelle, an der Geld entsteht.
	t.Setenv("REQUIRE_PROVE_PROVENANCE", "")
	if !proveHerkunftVerlangt() {
		t.Fatal("ohne ausdrueckliches Abschalten muss die Herkunft verlangt werden")
	}
	t.Setenv("REQUIRE_PROVE_PROVENANCE", "false")
	if proveHerkunftVerlangt() {
		t.Fatal("ausdrueckliches Abschalten muss moeglich bleiben")
	}
}

func TestMissbrauch_ZweiWalletsInEinerAnfrage(t *testing.T) {
	// Sicherheitspruefung 30.09.2026: Go liest "Wallet" wie "wallet" (der
	// letzte gewinnt), der Proof-Server liest nur "wallet". Mit einer
	// mitgelesenen Anfrage des Opfers haette ein Angreifer den Beweis fuer
	// die Wallet des Opfers erzeugen lassen und die Herkunft auf seine eigene
	// gebucht.
	angriffe := []string{
		`{"wallet":"` + walletAlice + `","bio":"1","Wallet":"` + walletMallory + `"}`,
		`{"wallet":"` + walletAlice + `","WALLET":"` + walletMallory + `"}`,
		`{"Wallet":"` + walletMallory + `"}`,
		`{"wallet":"` + walletAlice + `","wallet":"` + walletMallory + `"}`,
		`{"wallet":"` + walletAlice + `"} {"wallet":"` + walletMallory + `"}`,
		`[{"wallet":"` + walletAlice + `"}]`,
	}
	for i, anfrage := range angriffe {
		n := "0xC" + string(rune('0'+i)) + "01"
		merkeProveHerkunft([]byte(anfrage), []byte(`{"zkNullifier":"`+n+`","circuitVersion":3}`))
		if hatProveHerkunft(n, walletMallory) || hatProveHerkunft(n, walletAlice) {
			t.Errorf("mehrdeutige Anfrage %s: darf keine Herkunft hinterlassen", anfrage)
		}
		if _, ok := eindeutigeWallet([]byte(anfrage)); ok {
			t.Errorf("mehrdeutige Anfrage %s: muss am Proxy abgewiesen werden", anfrage)
		}
	}
	// Die ehrliche Anfrage geht weiter, auch mit weiteren Feldern.
	w, ok := eindeutigeWallet([]byte(`{"bio":"1","salt":"2","wallet":"0xA11CE00000000000000000000000000000000001","bioAttestation":"x"}`))
	if !ok || w != walletAlice {
		t.Fatalf("ehrliche Anfrage abgewiesen: %q %v", w, ok)
	}
}
