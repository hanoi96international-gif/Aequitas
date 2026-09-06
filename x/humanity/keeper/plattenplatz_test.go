package keeper

import (
	"strings"
	"testing"
)

// Ein Waechter, der zu Unrecht ablehnt, ist schlimmer als keiner: er nimmt
// einem gesunden Knoten die Arbeit weg. Diese Tests pruefen deshalb vor allem
// die Faelle, in denen NICHT abgelehnt werden darf.

func plattenZustandSetzen(t *testing.T, freiMB, gesamtMB int64, pruefungen int64, fehler string) {
	t.Helper()
	alt := struct {
		frei, gesamt, pruef, warn int64
		fehler, pfad              interface{}
	}{
		plattenFreiMB.Load(), plattenGesamtMB.Load(), plattenPruefungen.Load(),
		plattenWarnungen.Load(), plattenFehler.Load(), plattenPfad.Load(),
	}
	t.Cleanup(func() {
		plattenFreiMB.Store(alt.frei)
		plattenGesamtMB.Store(alt.gesamt)
		plattenPruefungen.Store(alt.pruef)
		plattenWarnungen.Store(alt.warn)
		if alt.fehler != nil {
			plattenFehler.Store(alt.fehler)
		}
		if alt.pfad != nil {
			plattenPfad.Store(alt.pfad)
		}
	})
	plattenFreiMB.Store(freiMB)
	plattenGesamtMB.Store(gesamtMB)
	plattenPruefungen.Store(pruefungen)
	plattenFehler.Store(fehler)
	plattenPfad.Store("/test")
}

func TestPlattenplatz_VorDerErstenMessungNieAblehnen(t *testing.T) {
	// Ein Knoten, der noch nichts gemessen hat, weiss nichts -- und Unwissen
	// darf nicht wie "Platte voll" wirken. Sonst lehnt jeder frisch
	// gestartete Knoten in seiner ersten Minute alles ab.
	plattenZustandSetzen(t, 0, 0, 0, "")
	if plattenplatzKritisch() {
		t.Error("ohne jede Messung als kritisch gemeldet")
	}
}

func TestPlattenplatz_MessfehlerIstKeinAblehnungsgrund(t *testing.T) {
	// Wenn statfs scheitert (ungewoehnliches Dateisystem, Rechte, Container),
	// ist das ein Problem der MESSUNG, nicht der Platte. Ein Knoten, der
	// deswegen keine Ueberweisungen mehr annimmt, waere durch sein eigenes
	// Instrument lahmgelegt.
	plattenZustandSetzen(t, 0, 0, 5, "statfs: operation not permitted")
	if plattenplatzKritisch() {
		t.Error("ein Messfehler wurde als volle Platte gewertet")
	}
	if got := PlattenplatzStand()["messfehler"].(string); got == "" {
		t.Error("der Messfehler wird nicht ausgewiesen -- dann ist er unsichtbar")
	}
}

func TestPlattenplatz_KritischErstUnterDerGrenze(t *testing.T) {
	faelle := []struct {
		freiMB   int64
		kritisch bool
		was      string
	}{
		{100 * 1024, false, "100 GB frei"},
		{(plattenKritischGB + 1) * 1024, false, "knapp ueber der Grenze"},
		{plattenKritischGB * 1024, false, "genau auf der Grenze"},
		{plattenKritischGB*1024 - 1, true, "knapp darunter"},
		{0, true, "null frei -- der gemessene Ausfall vom 06.09.2026"},
	}
	for _, f := range faelle {
		plattenZustandSetzen(t, f.freiMB, 96*1024, 3, "")
		if got := plattenplatzKritisch(); got != f.kritisch {
			t.Errorf("%s (%d MB): kritisch=%v, erwartet %v", f.was, f.freiMB, got, f.kritisch)
		}
	}
}

func TestPlattenplatz_AblehnungNenntDenGrundUndDieZahl(t *testing.T) {
	// Der Grund muss aus der Fehlermeldung hervorgehen. Beim Ausfall vom
	// 06.09.2026 kostete genau das Stunden: der Knoten lief, antwortete, und
	// nichts sagte "Platte".
	plattenZustandSetzen(t, 512, 96*1024, 3, "")
	grund := admissionRefusalReason()
	if grund == "" {
		t.Fatal("bei voller Platte wurde nicht abgelehnt")
	}
	if !strings.Contains(grund, "disk") {
		t.Errorf("die Ablehnung nennt die Platte nicht: %q", grund)
	}
	if !strings.Contains(grund, "512") {
		t.Errorf("die Ablehnung nennt den freien Platz nicht: %q", grund)
	}
	if !strings.Contains(grund, "retry") {
		t.Errorf("die Ablehnung sagt nicht, dass sie wiederholbar ist: %q", grund)
	}
}

func TestPlattenplatz_GenugPlatzLaesstDieAnnahmeInRuhe(t *testing.T) {
	// Bei reichlich Platz darf dieser Waechter GAR NICHTS zur Ablehnung
	// beitragen -- was danach kommt (Stillstandspruefung), ist seine Sache.
	plattenZustandSetzen(t, 50*1024, 96*1024, 3, "")
	if grund := admissionRefusalReason(); strings.Contains(grund, "disk") {
		t.Errorf("bei 50 GB frei wurde wegen der Platte abgelehnt: %q", grund)
	}
	s := PlattenplatzStand()
	if s["kritisch"].(bool) {
		t.Error("50 GB frei als kritisch gemeldet")
	}
	if pct := s["belegt_pct"].(float64); pct < 47 || pct > 49 {
		t.Errorf("belegt_pct = %.1f, erwartet ~48 (50 von 96 GB frei)", pct)
	}
}
