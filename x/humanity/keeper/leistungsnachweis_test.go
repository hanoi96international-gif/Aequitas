package keeper

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLeistungsnachweis_BesterWertUndSchwellen(t *testing.T) {
	ln := &leistungsnachweis{schwellen: LeistungsSchwellen{MinSigProSek: 1000, MaxCommitMs: 10, MinKerne: 2}}
	if ok, grund := ln.erfuellt(); ok || grund != "noch nicht gemessen" {
		t.Fatalf("ohne Messung: %v %q", ok, grund)
	}
	ln.uebernehmen(Leistungsmessung{SigProSek: 5000, CommitMs: 3, Kerne: 4})
	if ok, grund := ln.erfuellt(); !ok {
		t.Fatalf("gute Messung nicht erfuellt: %q", grund)
	}
	// Eine Messung unter Last darf den Nachweis nicht kosten.
	ln.uebernehmen(Leistungsmessung{SigProSek: 100, CommitMs: 400, Kerne: 4})
	if ok, grund := ln.erfuellt(); !ok {
		t.Fatalf("schlechte Messung unter Last hat den Nachweis gekostet: %q", grund)
	}
	if ln.letzte.SigProSek != 100 || ln.bester.SigProSek != 5000 || ln.bester.CommitMs != 3 {
		t.Fatalf("bester/letzte falsch: %+v %+v", ln.bester, ln.letzte)
	}

	schwach := &leistungsnachweis{schwellen: ln.schwellen}
	schwach.uebernehmen(Leistungsmessung{SigProSek: 500, CommitMs: 30, Kerne: 1})
	ok, grund := schwach.erfuellt()
	if ok {
		t.Fatal("schwacher Knoten gilt als leiterfaehig")
	}
	for _, w := range []string{"Signaturen", "Commit", "Kerne"} {
		if !strings.Contains(grund, w) {
			t.Fatalf("Grund nennt %s nicht: %q", w, grund)
		}
	}
	// Commit nicht messbar (keine Datenbank): nicht leiterfaehig.
	ohneDB := &leistungsnachweis{schwellen: ln.schwellen}
	ohneDB.uebernehmen(Leistungsmessung{SigProSek: 5000, Kerne: 4, Fehler: "keine Datenbank"})
	if ok, _ := ohneDB.erfuellt(); ok {
		t.Fatal("ohne messbaren Commit leiterfaehig")
	}
}

func TestLeistungsnachweis_BetreiberUeberstimmt(t *testing.T) {
	t.Setenv(leiterFaehigEnv, "nein")
	if leiterFaehigZwang() != "nein" {
		t.Fatal("nein nicht erkannt")
	}
	ln := &leistungsnachweis{schwellen: leistungsVorgabe(), zwang: leiterFaehigZwang()}
	ln.uebernehmen(Leistungsmessung{SigProSek: 1e9, CommitMs: 0.1, Kerne: 64})
	if ok, _ := ln.erfuellt(); ok {
		t.Fatal("nein ueberstimmt die Messung nicht")
	}
	t.Setenv(leiterFaehigEnv, "ja")
	ln = &leistungsnachweis{schwellen: leistungsVorgabe(), zwang: leiterFaehigZwang()}
	if ok, _ := ln.erfuellt(); !ok {
		t.Fatal("ja ueberstimmt die fehlende Messung nicht")
	}
	t.Setenv(leiterFaehigEnv, "vielleicht")
	if leiterFaehigZwang() != "" {
		t.Fatal("Unsinn als Zwang gewertet")
	}
}

func TestLeistungsSchwellenAusUmgebung(t *testing.T) {
	t.Setenv(leistungMinSigEnv, "12345")
	t.Setenv(leistungMaxComEnv, "7.5")
	t.Setenv(leistungMinKernEnv, "8")
	s := leistungsSchwellenAusUmgebung()
	if s.MinSigProSek != 12345 || s.MaxCommitMs != 7.5 || s.MinKerne != 8 {
		t.Fatalf("%+v", s)
	}
	t.Setenv(leistungMaxComEnv, "-1")
	t.Setenv(leistungMinSigEnv, "abc")
	s = leistungsSchwellenAusUmgebung()
	if s.MaxCommitMs != leistungsVorgabe().MaxCommitMs || s.MinSigProSek != leistungsVorgabe().MinSigProSek {
		t.Fatalf("ungueltige Werte uebernommen: %+v", s)
	}
}

func TestMesseSignaturen(t *testing.T) {
	n := messeSignaturen(200 * time.Millisecond)
	if n < 100 {
		t.Fatalf("%.0f Signaturen/s -- Messung kaputt", n)
	}
	t.Logf("%.0f Signaturen/s auf dieser Maschine", n)
}

func TestMesseCommit_RealDB(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("braucht DATABASE_URL (eine Wegwerf-Postgres)")
	}
	cs := testKnoten(t, "unused-leistung-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	ms, err := messeCommit(cs.db, 5)
	if err != nil || ms <= 0 {
		t.Fatalf("Commit-Messung: %v %v", ms, err)
	}
	m := leistungMessen(cs.db)
	if m.Fehler != "" || m.CommitMs <= 0 || m.SigProSek <= 0 || m.Kerne <= 0 {
		t.Fatalf("Messung unvollstaendig: %+v", m)
	}
	t.Logf("Messung: %+v", m)
	if _, err := messeCommit(nil, 1); err == nil {
		t.Fatal("ohne Datenbank kein Fehler")
	}
}
