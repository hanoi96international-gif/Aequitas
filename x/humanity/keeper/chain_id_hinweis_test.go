package keeper

import (
	"math"
	"strings"
	"testing"
)

// Der Hinweis erscheint nur mit gesetztem Stichtag, bis 14 Tage danach, genau
// einmal hinter <main>, und nennt Datum und neue ID.
func TestChainIDHinweis_NurMitStichtag(t *testing.T) {
	wechselAb(t, math.MaxInt64)
	if h := mitChainIDHinweis(landingHTML, nowUnix()); h != landingHTML {
		t.Fatal("schlafend: Startseite veraendert")
	}

	ab := int64(1793059200) // 27.10.2026 00:00 UTC
	wechselAb(t, ab)
	seite := mitChainIDHinweis(landingHTML, ab-7*86400)
	if strings.Count(seite, "Chain-ID-Wechsel am 27.10.2026, 00:00 UTC") != 1 {
		t.Fatal("Hinweis fehlt oder doppelt vor dem Stichtag")
	}
	if !strings.Contains(seite, "<main>\n<div role=\"status\"") || !strings.Contains(seite, "19260930") {
		t.Fatal("Hinweis nicht hinter <main> oder ohne neue ID")
	}
	if mitChainIDHinweis(landingHTML, ab+14*86400-1) == landingHTML {
		t.Fatal("Hinweis fehlt kurz vor Ende des Nachlaufs")
	}
	if mitChainIDHinweis(landingHTML, ab+14*86400) != landingHTML {
		t.Fatal("Hinweis steht nach dem Nachlauf noch")
	}
	if strings.Count(landingHTML, "<main>") != 1 {
		t.Fatal("Startseite hat nicht genau ein <main>")
	}
}
