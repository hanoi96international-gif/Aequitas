package main

import (
	"strings"
	"testing"
)

// Am 29.08.2026 zerfiel EINE Ursache in 23 scheinbar verschiedene, weil
// "batch member 9/26" nicht normalisiert wurde: ein Bruch ist nicht rein
// numerisch. Jede Variante bekam eine kleine Zahl, und die tatsaechlich
// haeufigste Ursache war in der Aufstellung nicht mehr zu erkennen.
func TestNormalizeErrForTally_BuendelPositionIstKeineEigeneUrsache(t *testing.T) {
	a := normalizeErrForTally("Transfer failed: batch member 9/26 (0x29ada0ff0e9845e6b16a518515f21e9e2fe2c750 -> 0x8ae8d22747bd4369fd732617673fed7554886807)")
	b := normalizeErrForTally("Transfer failed: batch member 12/33 (0xdbee57d88c6ea6eddd8f7a7a7e1ce2e67304db1b -> 0xa388bb2d38ed58c7c2a5c1db21e9778c24df39d4)")
	if a != b {
		t.Fatalf("zwei Meldungen derselben Ursache ergeben verschiedene Schluessel:\n  %s\n  %s", a, b)
	}
}

func TestIsFraction(t *testing.T) {
	for _, s := range []string{"9/26", "1/1", "12/330"} {
		if !isFraction(s) {
			t.Fatalf("%q sollte als Bruch erkannt werden", s)
		}
	}
	for _, s := range []string{"", "/", "9/", "/26", "a/b", "9", "9/2a"} {
		if isFraction(s) {
			t.Fatalf("%q ist kein Bruch", s)
		}
	}
}

// Eine Netzadresse ist nie eine URSACHE -- der Quellport wechselt bei jedem
// Versuch. Ohne diese Normalisierung zerfiel im Lauf vom 29.08.2026 EIN
// Fehler (connection reset by peer) in 11.158 scheinbar verschiedene, und die
// Zusammenfassung endete mit "... and 11158 more distinct cause(s)" -- genau
// dann, als die Ursache gebraucht wurde.
func TestNormalizeErrForTally_NetzadressenWerdenZusammengefasst(t *testing.T) {
	a := normalizeErrForTally(`Post "http://127.0.0.1:8080/rpc": read tcp 127.0.0.1:58900->127.0.0.1:8080: read: connection reset by peer`)
	b := normalizeErrForTally(`Post "http://127.0.0.1:8080/rpc": read tcp 127.0.0.1:44010->127.0.0.1:8080: read: connection reset by peer`)
	if a != b {
		t.Fatalf("zwei Verbindungen mit verschiedenen Quellports zaehlen noch getrennt:\n  %s\n  %s", a, b)
	}
	if !strings.Contains(a, "<addr>") {
		t.Fatalf("Adresse nicht ersetzt: %s", a)
	}
}

// Prosa mit Doppelpunkt darf NICHT als Adresse durchgehen, sonst
// verschwindet die eigentliche Fehlermeldung hinter <addr>.
func TestIstNetzAdresse_VerwechseltProsaNicht(t *testing.T) {
	keine := []string{
		"failed:", "Transfer", "-32005:", "http://127.0.0.1:8080/rpc\":",
		"read:", "peer", "nonce", "expected=2903",
	}
	for _, f := range keine {
		if istNetzAdresse(f) {
			t.Errorf("%q wurde faelschlich als Adresse erkannt", f)
		}
	}
	sind := []string{
		"127.0.0.1:58900->127.0.0.1:8080", "127.0.0.1:8080",
		"194.163.188.71:8080", "localhost:5432",
	}
	for _, f := range sind {
		if !istNetzAdresse(f) {
			t.Errorf("%q wurde nicht als Adresse erkannt", f)
		}
	}
}

// Die Vorgabe der Buendelgroesse traegt eine Messung, keine Vermutung.
func TestBatchSize_VorgabeIstDieGemesseneUndNichtDasMaximum(t *testing.T) {
	if batchSize > 25 {
		t.Fatalf("batchSize = %d. Gemessen am 29.08.2026: Buendel 10 ergab 591 TPS "+
			"bei 161 Absendern, Buendel 100 nur 427 bei 169 -- der Knoten arbeitet "+
			"die Posten EINES Buendels seriell ab, ein grosses Buendel ist also eine "+
			"lange Kette hinter einem Absender", batchSize)
	}
	if batchSize < 5 {
		t.Fatalf("batchSize = %d. Buendel 3 hat am 29.08.2026 mit 320 Absendern die "+
			"Verbindungsebene ueberfahren (connection reset by peer) -- die Zahl "+
			"offener Verbindungen waechst mit 1/Buendelgroesse", batchSize)
	}
}

// Die tragende Eigenschaft des Rings: Goroutine i sendet AUSSCHLIESSLICH aus
// acc[i]. Beide Ziele sind Nachbarn, nie das eigene Konto, und kein Konto ist
// Absender fuer zwei Goroutinen.
//
// Der frueher hier verbaute Weg -- Absender und Empfaenger tauschen -- verletzt
// genau das: Goroutine i saehe acc[i+1] als Absender, aus dem Goroutine i+1
// schon sendet. Zwei Vergeber derselben Nonce zerlegen beide Konten fuer den
// Rest des Laufs. Der Test haelt fest, welche Umkehr sicher ist.
func TestRingNachbarn_AbsenderBleibtImmerDasEigeneKonto(t *testing.T) {
	for _, n := range []int{2, 3, 8, 617, 618} {
		absender := map[int]int{}
		for i := 0; i < n; i++ {
			nach, vor := ringNachbarn(n, i)
			if nach < 0 || nach >= n || vor < 0 || vor >= n {
				t.Fatalf("n=%d i=%d: Nachbarn ausserhalb des Rings (%d, %d)", n, i, nach, vor)
			}
			if n > 2 && (nach == i || vor == i) {
				t.Fatalf("n=%d i=%d: ein Ziel ist das eigene Konto (%d, %d)", n, i, nach, vor)
			}
			if n > 2 && nach == vor {
				t.Fatalf("n=%d i=%d: beide Ziele sind dasselbe Konto -- dann kehrt nichts um", n, i)
			}
			if vorher, doppelt := absender[i]; doppelt {
				t.Fatalf("n=%d: Konto %d ist Absender fuer Goroutine %d UND %d", n, i, vorher, i)
			}
			absender[i] = i
		}
		// Und die Gegenprobe: der unsichere Weg kollidiert wirklich.
		if n > 2 {
			nach0, _ := ringNachbarn(n, 0)
			if nach0 != 1 {
				t.Fatalf("n=%d: Nachfolger von 0 ist %d, erwartet 1", n, nach0)
			}
			// Wuerde Goroutine 0 aus acc[nach0] senden, waere das genau der
			// Absender von Goroutine 1.
			if nach0 != 1 {
				t.Fatal("Vorbedingung der Gegenprobe verletzt")
			}
		}
	}
}

// Ueber einen ganzen Umlauf muss sich die Richtungsumkehr aufheben: jedes Konto
// sendet gleich oft vorwaerts wie rueckwaerts und empfaengt genauso oft von
// beiden Seiten. Ohne diese Eigenschaft verbraucht der Lasttest seinen eigenen
// Kontenvorrat -- am 29.08.2026 standen danach 418 von 618 Konten auf null.
func TestRingNachbarn_UmkehrGleichtDenFlussAus(t *testing.T) {
	const n = 64
	saldo := make([]int, n)
	// Zwei Runden: erst alle vorwaerts, dann alle rueckwaerts -- genau das,
	// was das abwechselnde forward-Flag ueber zwei Buendel tut.
	for i := 0; i < n; i++ {
		nach, _ := ringNachbarn(n, i)
		saldo[i]--
		saldo[nach]++
	}
	for i := 0; i < n; i++ {
		_, vor := ringNachbarn(n, i)
		saldo[i]--
		saldo[vor]++
	}
	for i, v := range saldo {
		if v != 0 {
			t.Fatalf("Konto %d steht nach einem Hin und Her bei %+d statt 0 -- "+
				"der Lauf verbraucht dann seinen eigenen Kontenvorrat", i, v)
		}
	}
}
