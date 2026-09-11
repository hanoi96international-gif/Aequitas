package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// Der Takt war aus gutem Grund fest verdrahtet: am 04.07.2026 wurde er in
// einer Nacht 1s -> 2s -> 6s gedreht, waehrend die eigentlichen
// Konvergenzfehler noch offen waren, und jede Drehung sah kurz nach Besserung
// aus. Ihn wieder einstellbar zu machen holt genau diese Gefahr zurueck --
// darum die Grenzen, und darum diese Tests.

func TestBlockZeit_LeerBleibtBeiDerVorgabe(t *testing.T) {
	t.Setenv("BLOCK_TIME_MS", "")
	if got := blockZeit(); got != BLOCK_TIME_VORGABE {
		t.Fatalf("blockZeit() = %v ohne gesetzte Umgebung, erwartet %v", got, BLOCK_TIME_VORGABE)
	}
}

func TestBlockZeit_NimmtGueltigenWert(t *testing.T) {
	t.Setenv("BLOCK_TIME_MS", "500")
	if got := blockZeit(); got != 500*time.Millisecond {
		t.Fatalf("blockZeit() = %v bei BLOCK_TIME_MS=500, erwartet 500ms", got)
	}
}

// Die Grenzen sind der eigentliche Zweck. Ein Tippfehler darf nicht die Kette
// anhalten: unter 200 ms liegt der Takt in der Groessenordnung der Laufzeit
// zwischen den Boxen, und dann entstehen Bloecke schneller, als der andere sie
// sehen kann -- das Ergebnis waeren Waisen statt Transaktionen.
func TestBlockZeit_WeistUnsinnZurueckStattIhnZuUebernehmen(t *testing.T) {
	for _, fall := range []struct {
		roh   string
		warum string
	}{
		{"50", "unter der Untergrenze: schneller als die Laufzeit zwischen den Boxen"},
		{"0", "null waere ein Ticker ohne Pause"},
		{"-500", "negativ"},
		{"60000", "ueber der Obergrenze: der Totmannschalter liest so lange Luecken als Ausfall"},
		{"eine Sekunde", "keine Zahl"},
		{"1,5", "kein gueltiges Zahlenformat"},
	} {
		t.Run(fall.roh, func(t *testing.T) {
			t.Setenv("BLOCK_TIME_MS", fall.roh)
			if got := blockZeit(); got != BLOCK_TIME_VORGABE {
				t.Fatalf("blockZeit() = %v bei BLOCK_TIME_MS=%q (%s), erwartet die Vorgabe %v. "+
					"Ein unsinniger Wert muss auf die Vorgabe zurueckfallen, nicht uebernommen werden.",
					got, fall.roh, fall.warum, BLOCK_TIME_VORGABE)
			}
		})
	}
}

// Die Reihenfolge ist die halbe Aenderung: wird BLOCK_TIME erst nach
// TuneProposerBreakerForBlockTime gesetzt, skalieren Schutzschalter und
// Finalitaetsspielraum weiter auf den alten Takt -- die Kette liefe schneller,
// ihre Schwellen aber nicht mit. Das faellt in keinem Einzeltest auf, sondern
// erst live als wiederkehrendes Ausloesen des Schutzschalters.
func TestBlockZeit_WirdVorDenAbgeleitetenSchwellenGesetzt(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("main.go nicht lesbar: %v", err)
	}
	body := string(b)

	setzen := strings.Index(body, "BLOCK_TIME = blockZeit()")
	if setzen < 0 {
		t.Fatal("BLOCK_TIME wird nicht mehr aus blockZeit() gesetzt -- die Umgebungsvariable " +
			"BLOCK_TIME_MS waere damit wirkungslos, ohne dass es jemand merkt.")
	}
	for _, abgeleitet := range []string{
		"TuneProposerBreakerForBlockTime(BLOCK_TIME)",
		"TuneFinalitySlackForBlockTime(BLOCK_TIME)",
		`fmt.Printf("Block Time:`,
	} {
		if i := strings.Index(body, abgeleitet); i >= 0 && i < setzen {
			t.Errorf("%s steht vor der Zuweisung von BLOCK_TIME und sieht damit noch die Vorgabe", abgeleitet)
		}
	}
}
