package keeper

import (
	"strings"
	"testing"
)

// Diese Tests decken die Entscheidungslogik des Sybil-Reports ab, nicht seine
// SQL-Ausfuehrung: die Testsuite hier konstruiert ChainState direkt und hat
// keine Postgres-Instanz, gegen die die Queries laufen koennten. Getestet wird
// deshalb genau der Teil, der im Betrieb nachjustiert wird und bei dem ein
// Fehler still bliebe — Schwellen und Einstufung.

func TestBurstThreshold(t *testing.T) {
	tests := []struct {
		name   string
		humans int
		want   int
	}{
		// Der Boden von 5 ist der eigentliche Zweck: ohne ihn waere die
		// Schwelle bei kleiner Population 0 und jede einzelne Registrierung
		// wuerde als Spitze gemeldet. Bei den aktuell 6 registrierten Menschen
		// in Produktion ist genau das der relevante Fall.
		{"leeres Netz", 0, 5},
		{"Produktionsstand heute", 6, 5},
		{"knapp unter dem Umschlagpunkt", 49, 5},
		{"Umschlagpunkt: 10 % erreichen den Boden", 50, 5},
		{"darueber waechst die Schwelle mit", 100, 10},
		{"grosses Netz", 12_345, 1234},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := burstThreshold(tc.humans); got != tc.want {
				t.Errorf("burstThreshold(%d) = %d, erwartet %d", tc.humans, got, tc.want)
			}
		})
	}
}

func TestBurstThresholdIsMonotonic(t *testing.T) {
	// Eine wachsende Population darf die Schwelle nie senken — sonst wuerde
	// ein Netz durch Wachstum plotzlich empfindlicher statt toleranter.
	prev := burstThreshold(0)
	for humans := 1; humans <= 5000; humans += 7 {
		got := burstThreshold(humans)
		if got < prev {
			t.Fatalf("Schwelle faellt bei %d Menschen: %d nach %d", humans, got, prev)
		}
		prev = got
	}
}

func TestBurstSeverity(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		threshold int
		want      SybilSeverity
	}{
		{"genau auf der Schwelle", 5, 5, SybilWarn},
		{"knapp unter dem Dreifachen", 14, 5, SybilWarn},
		{"genau das Dreifache", 15, 5, SybilAlert},
		{"weit darueber", 500, 5, SybilAlert},
		// Schutz gegen Division/Multiplikation mit 0: eine Schwelle von 0
		// darf nicht dazu fuehren, dass jedes Ergebnis Alarm ist.
		{"Schwelle 0 eskaliert nicht", 3, 0, SybilWarn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := burstSeverity(tc.total, tc.threshold); got != tc.want {
				t.Errorf("burstSeverity(%d, %d) = %q, erwartet %q",
					tc.total, tc.threshold, got, tc.want)
			}
		})
	}
}

func TestDormantSeverity(t *testing.T) {
	tests := []struct {
		name    string
		dormant int
		humans  int
		want    SybilSeverity
	}{
		// Unter 20 Konten bleibt es immer bei "info", auch wenn rechnerisch
		// 100 % dormant sind. Bei 6 Menschen ist eine Quote schlicht kein
		// Signal, und ein Alarm waere reines Rauschen.
		{"kleine Population, alles dormant", 6, 6, SybilInfo},
		{"knapp unter der Mindestpopulation", 19, 19, SybilInfo},
		{"ab 20 greift die Quote", 20, 20, SybilAlert},
		{"80 % ist Alarm", 16, 20, SybilAlert},
		{"knapp unter 80 % ist Warnung", 15, 20, SybilWarn},
		{"50 % ist Warnung", 10, 20, SybilWarn},
		{"unter 50 % ist info", 9, 20, SybilInfo},
		{"gesundes Netz", 5, 1000, SybilInfo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dormantSeverity(tc.dormant, tc.humans); got != tc.want {
				t.Errorf("dormantSeverity(%d, %d) = %q, erwartet %q",
					tc.dormant, tc.humans, got, tc.want)
			}
		})
	}
}

func TestDormantSeverityNoDivisionByZero(t *testing.T) {
	// humans == 0 darf nicht in eine Division durch null laufen. Der Fall ist
	// im Report zwar durch ein frueheres Return abgefangen, aber die Funktion
	// muss fuer sich genommen sicher sein.
	if got := dormantSeverity(0, 0); got != SybilInfo {
		t.Errorf("dormantSeverity(0, 0) = %q, erwartet %q", got, SybilInfo)
	}
}

func TestBuildSybilReportWithoutDatabase(t *testing.T) {
	// Ein Knoten ohne Datenbank darf beim Report nicht panicken, sondern muss
	// den Grund benennen. Das ist der Pfad, den ein frisch gestarteter oder
	// fehlkonfigurierter Knoten nimmt.
	cs := &ChainState{}
	rep := cs.BuildSybilReport(0)

	if len(rep.Errors) == 0 {
		t.Fatal("Report ohne Datenbank muss einen Fehler melden")
	}
	if len(rep.Signals) != 0 {
		t.Errorf("Report ohne Datenbank darf keine Signale liefern, hat %d", len(rep.Signals))
	}
	// windowHours <= 0 muss auf den Default fallen, sonst wuerde eine 0 als
	// Intervall an make_interval durchgereicht und stillschweigend nie etwas
	// finden.
	if rep.WindowHours != 24 {
		t.Errorf("windowHours = %d, erwartet den Default 24", rep.WindowHours)
	}
	if rep.GeneratedAt.IsZero() {
		t.Error("GeneratedAt wurde nicht gesetzt")
	}
}

func TestBuildSybilReportKeepsExplicitWindow(t *testing.T) {
	cs := &ChainState{}
	if rep := cs.BuildSybilReport(72); rep.WindowHours != 72 {
		t.Errorf("windowHours = %d, erwartet 72", rep.WindowHours)
	}
}

func TestSybilReportString(t *testing.T) {
	rep := SybilReport{
		WindowHours: 24,
		TotalHumans: 6,
		Signals: []SybilSignal{
			{Kind: "registration_burst", Severity: SybilAlert, Description: "Spitze"},
		},
		Errors: []string{"untouched_grant: Verbindung verloren"},
	}
	out := rep.String()

	for _, want := range []string{"ALERT", "registration_burst", "Spitze", "FEHLER", "Verbindung verloren"} {
		if !strings.Contains(out, want) {
			t.Errorf("String() enthaelt %q nicht:\n%s", want, out)
		}
	}
}

func TestSybilReportStringWithoutSignals(t *testing.T) {
	// Ein leerer Report muss das ausdruecklich sagen. Eine leere Ausgabe waere
	// von "Report nicht gelaufen" nicht zu unterscheiden.
	out := SybilReport{WindowHours: 24}.String()
	if !strings.Contains(out, "keine Signale") {
		t.Errorf("leerer Report muss das benennen:\n%s", out)
	}
}
