package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Briefkasten (briefkasten.go): ablegen nur von Menschen, abholen und
// quittieren nur durch den Empfaenger, alles begrenzt.

const bkJetzt int64 = 1_900_000_000

type bkPerson struct {
	k    *ecdsa.PrivateKey
	addr string
}

func bkNeu(t *testing.T) bkPerson {
	k, a := neuerSchluessel(t)
	return bkPerson{k, a}
}

func bkInhalt(n int, b byte) string {
	return strings.Repeat(hex.EncodeToString([]byte{b}), n)
}

func bkAblegenSig(t *testing.T, p bkPerson, an, inhalt string, zeit int64) string {
	roh, _ := hex.DecodeString(inhalt)
	h := sha256.Sum256(roh)
	return personalSign(t, p.k, briefAblegenNachricht(p.addr, an, hex.EncodeToString(h[:]), zeit))
}

func bkCode(err error) int {
	var ab *briefAbweisung
	if errors.As(err, &ab) {
		return ab.code
	}
	if err != nil {
		return 503
	}
	return 200
}

// Ohne Datenbank: alles, was vor dem Kasten geprueft wird -- und danach
// fail-closed (503), nie "angenommen".
func TestBriefkasten_PruefungenVorDemKasten(t *testing.T) {
	cs := newTestState()
	a, b, fremd := bkNeu(t), bkNeu(t), bkNeu(t)
	addHuman(cs, a.addr, 0)
	inh := bkInhalt(100, 0xab)
	ok := bkAblegenSig(t, a, b.addr, inh, bkJetzt)
	for _, f := range []struct {
		name            string
		von, an, inhalt string
		zeit            int64
		sig             string
		code            int
	}{
		{"Empfaenger nicht kanonisch", a.addr, strings.ToUpper(b.addr), inh, bkJetzt, ok, 400},
		{"Systemadresse als Empfaenger", a.addr, ubiPoolAddr, inh, bkJetzt, bkAblegenSig(t, a, ubiPoolAddr, inh, bkJetzt), 400},
		{"leerer Inhalt", a.addr, b.addr, "", bkJetzt, ok, 400},
		{"Inhalt kein Hex", a.addr, b.addr, "zz" + inh[2:], bkJetzt, ok, 400},
		{"Inhalt gross geschrieben", a.addr, b.addr, strings.ToUpper(inh), bkJetzt, ok, 400},
		{"Inhalt zu gross", a.addr, b.addr, bkInhalt(briefInhaltMax+1, 1), bkJetzt, ok, 400},
		{"Zeit zu alt", a.addr, b.addr, inh, bkJetzt - briefZeitSpiel - 1, bkAblegenSig(t, a, b.addr, inh, bkJetzt-briefZeitSpiel-1), 400},
		{"Zeit zu weit voraus", a.addr, b.addr, inh, bkJetzt + briefZeitSpiel + 1, bkAblegenSig(t, a, b.addr, inh, bkJetzt+briefZeitSpiel+1), 400},
		{"fremder Schluessel", a.addr, b.addr, inh, bkJetzt, bkAblegenSig(t, fremd, b.addr, inh, bkJetzt), 403},
		{"Unterschrift fuer anderen Inhalt", a.addr, b.addr, inh, bkJetzt, bkAblegenSig(t, a, b.addr, bkInhalt(100, 0xac), bkJetzt), 403},
		{"Unterschrift fuer anderen Empfaenger", a.addr, b.addr, inh, bkJetzt, bkAblegenSig(t, a, fremd.addr, inh, bkJetzt), 403},
		{"gueltig, aber ohne Datenbank", a.addr, b.addr, inh, bkJetzt, ok, 503},
	} {
		_, err := cs.briefAblegen(f.von, f.an, f.inhalt, f.zeit, f.sig, bkJetzt)
		if c := bkCode(err); c != f.code {
			t.Errorf("%s: %d (%v), erwartet %d", f.name, c, err, f.code)
		}
	}
	// Abholen und quittieren: nur mit der Unterschrift des Empfaengers.
	if _, _, err := cs.briefAbholen(b.addr, 0, bkJetzt, personalSign(t, fremd.k, briefAbholenNachricht(b.addr, 0, bkJetzt)), bkJetzt); bkCode(err) != 403 {
		t.Errorf("fremdes Abholen: %v", err)
	}
	if _, err := cs.briefQuittieren(b.addr, 99, bkJetzt, personalSign(t, fremd.k, briefQuittierenNachricht(b.addr, 99, bkJetzt)), bkJetzt); bkCode(err) != 403 {
		t.Errorf("fremdes Quittieren: %v", err)
	}
	// Die Unterschrift zum Abholen ab 0 gilt nicht fuer ab 5 (nach gebunden).
	if _, _, err := cs.briefAbholen(b.addr, 5, bkJetzt, personalSign(t, b.k, briefAbholenNachricht(b.addr, 0, bkJetzt)), bkJetzt); bkCode(err) != 403 {
		t.Errorf("Abholen mit fremdem nach: %v", err)
	}
}

func TestBriefkasten_Endpunkt(t *testing.T) {
	a := &APIServer{state: newTestState()}
	for _, f := range []struct {
		methode, body string
		code          int
	}{
		{http.MethodGet, ``, http.StatusMethodNotAllowed},
		{http.MethodPost, `{"an":"x","unbekannt":1}`, http.StatusBadRequest},
		{http.MethodPost, `{"inhalt":"` + strings.Repeat("a", briefKoerperMax+10) + `"}`, http.StatusBadRequest},
		{http.MethodPost, `{"von":"0x1","an":"0x2","inhalt":"ab","zeit":1,"sig":"0x"}`, http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(f.methode, "/api/briefkasten/ablegen", strings.NewReader(f.body))
		r.RemoteAddr = "198.51.100.7:1234"
		a.handleBriefkasten("ablegen")(rec, r)
		if rec.Code != f.code {
			t.Errorf("%s %.40s: %d, erwartet %d", f.methode, f.body, rec.Code, f.code)
		}
	}
	ipBurst.Delete("briefkasten:198.51.100.7")
}

// Gegen eine echte Datenbank: der ganze Weg und alle Grenzen.
func TestBriefkasten_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-briefkasten-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if _, err := cs.db.Exec(`DELETE FROM briefkasten`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM briefkasten`) })
	a, b, c, kein := bkNeu(t), bkNeu(t), bkNeu(t), bkNeu(t)
	for _, p := range []bkPerson{a, b, c} {
		addHuman(cs, p.addr, 0)
	}
	ablegen := func(von bkPerson, an, inhalt string, zeit int64) (int64, error) {
		return cs.briefAblegen(von.addr, an, inhalt, zeit, bkAblegenSig(t, von, an, inhalt, zeit), bkJetzt)
	}
	abholen := func(p bkPerson, nach int64) ([]briefAusgabe, bool, error) {
		return cs.briefAbholen(p.addr, nach, bkJetzt, personalSign(t, p.k, briefAbholenNachricht(p.addr, nach, bkJetzt)), bkJetzt)
	}

	// Ablegen, abholen, quittieren. Der erste Brief geht an c: quittiert b
	// spaeter bis zu einer hoeheren Nummer, muss er bleiben.
	idC, err := ablegen(a, c.addr, bkInhalt(8, 7), bkJetzt-1)
	if err != nil {
		t.Fatal(err)
	}
	id1, err := ablegen(a, b.addr, bkInhalt(1000, 1), bkJetzt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ablegen(a, b.addr, bkInhalt(1000, 1), bkJetzt); bkCode(err) != http.StatusConflict {
		t.Fatalf("dieselbe Ablage zweimal: %v", err)
	}
	if _, err := ablegen(kein, b.addr, bkInhalt(10, 1), bkJetzt); bkCode(err) != http.StatusForbidden {
		t.Fatalf("Ablage eines Nicht-Menschen: %v", err)
	}
	id2, _ := ablegen(c, b.addr, bkInhalt(10, 2), bkJetzt+1)
	briefe, mehr, err := abholen(b, 0)
	if err != nil || mehr || len(briefe) != 2 || briefe[0].ID != id1 || briefe[0].Von != a.addr || briefe[0].Inhalt != bkInhalt(1000, 1) || briefe[1].ID != id2 {
		t.Fatalf("abholen: %+v, %v, %v", briefe, mehr, err)
	}
	if briefe, _, _ := abholen(c, 0); len(briefe) != 1 || briefe[0].ID != idC {
		t.Fatal("ein anderer Empfaenger sieht fremde Briefe")
	}
	if briefe, _, _ := abholen(b, id1); len(briefe) != 1 || briefe[0].ID != id2 {
		t.Fatal("abholen ab nach")
	}
	if n, err := cs.briefQuittieren(b.addr, id1, bkJetzt, personalSign(t, b.k, briefQuittierenNachricht(b.addr, id1, bkJetzt)), bkJetzt); err != nil || n != 1 {
		t.Fatalf("quittieren: %d, %v", n, err)
	}
	if briefe, _, _ := abholen(b, 0); len(briefe) != 1 || briefe[0].ID != id2 {
		t.Fatal("quittierter Brief noch da")
	}
	if briefe, _, _ := abholen(c, 0); len(briefe) != 1 || briefe[0].ID != idC {
		t.Fatal("quittieren hat einen fremden Brief geloescht")
	}

	// Blaettern: hoechstens briefAbholenHoechstens je Aufruf.
	for i := 0; i < briefAbholenHoechstens+3; i++ {
		if _, err := ablegen(a, c.addr, bkInhalt(16, byte(i)), bkJetzt+int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if briefe, mehr, _ := abholen(c, 0); len(briefe) != briefAbholenHoechstens || !mehr {
		t.Fatalf("blaettern: %d, mehr %v", len(briefe), mehr)
	}

	// Verfall: ein alter Brief wird beim naechsten Ablegen geloescht und nie ausgeliefert.
	if _, err := cs.db.Exec(`INSERT INTO briefkasten (schluessel, von, an, inhalt, groesse, zeit, abgelegt) VALUES ('alt', $1, $2, '\x01', 1, 1, $3)`,
		a.addr, b.addr, bkJetzt-briefBehalten-1); err != nil {
		t.Fatal(err)
	}
	if briefe, _, _ := abholen(b, 0); len(briefe) != 1 {
		t.Fatalf("verfallener Brief ausgeliefert: %d", len(briefe))
	}
	ablegen(c, a.addr, bkInhalt(4, 9), bkJetzt)
	var n int
	cs.db.QueryRow(`SELECT count(*) FROM briefkasten WHERE schluessel = 'alt'`).Scan(&n)
	if n != 0 {
		t.Fatal("verfallener Brief nicht geloescht")
	}

	// Kontingente: Empfaenger voll, Absender voll, Kasten voll.
	fuellen := func(von, an string, anzahl, groesse int) {
		t.Helper()
		if _, err := cs.db.Exec(`INSERT INTO briefkasten (schluessel, von, an, inhalt, groesse, zeit, abgelegt)
			SELECT 'f-' || $1 || '-' || $2 || '-' || g, $1, $2, '\x00', $4, 1, $5 FROM generate_series(1, $3) g`,
			von, an, anzahl, groesse, bkJetzt); err != nil {
			t.Fatal(err)
		}
	}
	leeren := func() { cs.db.Exec(`DELETE FROM briefkasten WHERE schluessel LIKE 'f-%'`) }
	d := bkNeu(t)
	addHuman(cs, d.addr, 0)
	fuellen(c.addr, d.addr, briefJeEmpfaenger, 1)
	if _, err := ablegen(a, d.addr, bkInhalt(4, 1), bkJetzt); bkCode(err) != http.StatusTooManyRequests {
		t.Fatalf("Empfaenger voll: %v", err)
	}
	leeren()
	fuellen(d.addr, c.addr, briefJeAbsender, 1)
	if _, err := ablegen(d, b.addr, bkInhalt(4, 1), bkJetzt); bkCode(err) != http.StatusTooManyRequests {
		t.Fatalf("Absender voll (Anzahl): %v", err)
	}
	leeren()
	fuellen(d.addr, c.addr, 1, briefBytesJeAbsender-3)
	if _, err := ablegen(d, b.addr, bkInhalt(4, 1), bkJetzt); bkCode(err) != http.StatusTooManyRequests {
		t.Fatalf("Absender voll (Bytes): %v", err)
	}
	leeren()
	fuellen(c.addr, a.addr, 1, briefBytesGesamt)
	if _, err := ablegen(d, b.addr, bkInhalt(4, 1), bkJetzt); bkCode(err) != http.StatusServiceUnavailable {
		t.Fatalf("Kasten voll: %v", err)
	}
	leeren()
	if _, err := ablegen(d, b.addr, bkInhalt(4, 1), bkJetzt); err != nil {
		t.Fatalf("nach dem Leeren: %v", err)
	}
}
