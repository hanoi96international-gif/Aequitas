package keeper

import (
	"testing"
	"time"
)

// Die Luecke: der Vertrag prueft, dass ein Beweis GUELTIG ist -- nicht, woher
// er kommt. Groth16-Proving-Keys sind oeffentlich, dieser liegt im Repo. Wer
// sich eine bio_hash wuerfelt, lokal einen Beweis erzeugt und direkt an
// /api/register geht, praegte 1.000 AEQ. Und nochmal.

func TestSelbstErzeugterBeweisHatKeineHerkunft(t *testing.T) {
	// DER Fall. Nichts hat diesen Nullifier je bei /prove gesehen.
	if hatProveHerkunft("0xdeadbeef00000000000000000000000000000000000000000000000000000000") {
		t.Fatal("ein Nullifier, der nie durch /prove kam, darf keine Herkunft haben")
	}
}

func TestEineGeprueteAntwortHinterlaesstEineHerkunft(t *testing.T) {
	merkeProveHerkunft([]byte(`{"zkNullifier":"0xABC123","circuitVersion":3}`))
	if !hatProveHerkunft("0xabc123") {
		t.Fatal("nach einem erfolgreichen /prove muss die Herkunft stehen")
	}
	// Gross-/Kleinschreibung und 0x duerfen nicht entscheiden.
	if !hatProveHerkunft("ABC123") {
		t.Fatal("die Schreibweise darf ueber die Registrierung nicht entscheiden")
	}
}

func TestOhneNullifierWirdNichtsGemerkt(t *testing.T) {
	merkeProveHerkunft([]byte(`{"error":"bio attestation rejected"}`))
	merkeProveHerkunft([]byte(`kein JSON`))
	if hatProveHerkunft("") {
		t.Fatal("eine leere Kennung darf nie als Herkunft gelten")
	}
}

func TestEineAlteHerkunftVerfaellt(t *testing.T) {
	// Sonst waere eine einmal gepruefte Registrierung beliebig lange
	// einloesbar -- und eine abgefangene Antwort ein dauerhafter Freifahrtschein.
	proveHerkunft.Store("altfall", time.Now().Add(-proveHerkunftTTL-time.Minute))
	if hatProveHerkunft("altfall") {
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
