package keeper

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// Die Mindestausstattung steht an drei Stellen: im Knoten (leistungsVorgabe,
// auch fuer die Leistungsprobe neuer Validatoren), in
// deploy/validator/einrichten.sh (lehnt vor dem Start ab) und daraus erzeugt
// in der Anleitung (build/node_guide.py liest einrichten.sh). Weichen Knoten
// und Skript voneinander ab, sagt das Skript "reicht" und das Netz "nein"
// (oder umgekehrt). Dieser Test haelt beide gleich.
func TestLeistungsVorgabe_GleichWieEinrichten(t *testing.T) {
	roh, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "validator", "einrichten.sh"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(roh)
	v := leistungsVorgabe()
	m := regexp.MustCompile(`MIN_KERNE=(\d+); MIN_RAM_MB=(\d+); MIN_PLATZ_GB=(\d+)`).FindStringSubmatch(s)
	if m == nil {
		t.Fatal("einrichten.sh: MIN_KERNE-Zeile fehlt")
	}
	if k, _ := strconv.Atoi(m[1]); k != v.MinKerne {
		t.Fatalf("Kerne: einrichten.sh %d, Knoten %d", k, v.MinKerne)
	}
	p := regexp.MustCompile(`mindestens (\d+) Signaturen/s, Datenbank-Commit hoechstens (\d+) ms, (\d+) Kerne`).FindStringSubmatch(s)
	if p == nil {
		t.Fatal("einrichten.sh: Zeile mit den Leistungswerten fehlt")
	}
	if sig, _ := strconv.Atoi(p[1]); float64(sig) != v.MinSigProSek {
		t.Fatalf("Signaturen: einrichten.sh %d, Knoten %.0f", sig, v.MinSigProSek)
	}
	if ms, _ := strconv.Atoi(p[2]); float64(ms) != v.MaxCommitMs {
		t.Fatalf("Commit: einrichten.sh %d ms, Knoten %.0f ms", ms, v.MaxCommitMs)
	}
	if k, _ := strconv.Atoi(p[3]); k != v.MinKerne {
		t.Fatalf("Kerne in der Meldung: %d, Knoten %d", k, v.MinKerne)
	}
}

// Beta-Stufe: 4 Kerne / 20.000 Signaturen je Sekunde. Wer darunter liegt,
// wird nicht leiterfaehig und besteht die Probe nicht -- die Absenkung darf
// die Pruefung nicht abschalten (Grenze 0 hiesse "alles besteht").
func TestLeistungsVorgabe_BetaStufeGrenzen(t *testing.T) {
	v := leistungsVorgabe()
	if v.MinKerne != 4 || v.MinSigProSek != 20000 || v.MaxCommitMs != 5 {
		t.Fatalf("Beta-Stufe erwartet 4 Kerne, 20000/s, 5 ms: %+v", v)
	}
	for _, f := range []struct {
		name string
		b    Leistungsmessung
		soll bool
	}{
		{"genau an der Grenze", Leistungsmessung{SigProSek: 20000, CommitMs: 5, Kerne: 4}, true},
		{"ein Kern zu wenig", Leistungsmessung{SigProSek: 40000, CommitMs: 1, Kerne: 3}, false},
		{"zu langsam signiert", Leistungsmessung{SigProSek: 19999, CommitMs: 1, Kerne: 8}, false},
		{"Platte zu langsam", Leistungsmessung{SigProSek: 40000, CommitMs: 6, Kerne: 8}, false},
	} {
		ln := &leistungsnachweis{schwellen: v}
		ln.uebernehmen(f.b)
		if ok, grund := ln.erfuellt(); ok != f.soll {
			t.Errorf("%s: leiterfaehig=%v (%s), erwartet %v", f.name, ok, grund, f.soll)
		}
	}
	// Probe: n Signaturen in der Zeit fuer 20.000/s besteht, darueber nicht;
	// ein falsches Ergebnis besteht nie, egal wie schnell.
	n := probeAnzahl
	grenze := probeGrenze(n, v.MinSigProSek)
	if grenze <= 0 {
		t.Fatal("Probe-Grenze 0 -- die Probe waere abgeschaltet")
	}
	if !probeBestanden("x", "x", grenze, 0, n, v.MinSigProSek) {
		t.Error("an der Grenze nicht bestanden")
	}
	if probeBestanden("x", "x", grenze+time.Millisecond, 0, n, v.MinSigProSek) {
		t.Error("ueber der Grenze bestanden")
	}
	if probeBestanden("x", "y", time.Nanosecond, 0, n, v.MinSigProSek) {
		t.Error("falsches Ergebnis bestanden")
	}
}
