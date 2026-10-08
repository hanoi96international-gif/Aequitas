package keeper

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Missbrauch der Annahme (Pruefung von #318): ein Beobachter erzeugt nie
// (ProduceBlock kehrt vor der Messung um). Er mass darum nie und nahm ueber
// die HTTP-Wege an, bis der Rueckstau-Messer nach 10 Minuten griff -- ein
// Ausgang, der in keinen Block kommt.
func TestAnnahme_BeobachterNimmtNichtAn(t *testing.T) {
	_, cs := newDeterminismTestDAG()
	if err := cs.annahmeBeginnen(distTestAddr(2201)); err != nil {
		t.Fatalf("Vorbedingung: ohne Beobachter nimmt der Knoten an: %v", err)
	}
	cs.annahmeEnde()
	setzeBeobachterFuerTest(t, true)
	if err := cs.annahmeBeginnen(distTestAddr(2202)); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Beobachter nimmt an: %v", err)
	}
	if n := cs.annahmenLaufend.Load(); n != 0 {
		t.Fatalf("abgelehnte Annahme haengt im Zaehler: %d", n)
	}
	if st := cs.AnnahmePauseStand(); st["pausiert"] != true {
		t.Fatalf("/api/health/combined zeigt den Beobachter nicht als pausiert: %v", st)
	}
}

// Missbrauch der Annahme (Pruefung von #318): die Messung begann erst mit
// dem ersten Erzeugungsversuch, also nach Bootstrap und Resync beim Start.
// Bis dahin nahmen die HTTP-Wege ohne Pause an. Jetzt beginnt sie beim Bau
// des Knotens (main.go), einmal.
func TestAnnahme_MessungAbStart(t *testing.T) {
	_, cs := newDeterminismTestDAG()
	if cs.erzeugerSeit.Load() != 0 {
		t.Fatal("Vorbedingung: ein frisch gebauter Knoten misst noch nicht")
	}
	vorher := time.Now().Unix()
	cs.AnnahmeMessungBeginnen()
	if s := cs.erzeugerSeit.Load(); s < vorher || s > time.Now().Unix() {
		t.Fatalf("Messbeginn %d, erwartet jetzt (%d)", s, vorher)
	}
	if err := cs.annahmeBeginnen(distTestAddr(2203)); err != nil {
		t.Fatalf("binnen der Frist nach dem Start angehalten: %v", err)
	}
	cs.annahmeEnde()

	// Kein Block seit dem Start (Bootstrap dauert): nach der Frist angehalten,
	// ohne dass ProduceBlock je lief.
	alt := time.Now().Unix() - admissionStallLimit() - 1
	cs.erzeugerSeit.Store(alt)
	if err := cs.annahmeBeginnen(distTestAddr(2204)); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme %d s nach dem Start ohne eigenen Block nicht angehalten: %v", admissionStallLimit()+1, err)
	}
	// Ein zweiter Aufruf setzt die laufende Messung nicht zurueck.
	cs.AnnahmeMessungBeginnen()
	if cs.erzeugerSeit.Load() != alt {
		t.Fatal("ein zweiter Aufruf hat die Messung neu begonnen")
	}
	// Der erste eigene Block hebt die Pause auf.
	cs.letzterEigenerBlock.Store(time.Now().Unix())
	if err := cs.annahmeBeginnen(distTestAddr(2205)); err != nil {
		t.Fatalf("nach dem ersten eigenen Block weiter angehalten: %v", err)
	}
	cs.annahmeEnde()
}

// nil-sicher wie die uebrigen Methoden des Ausgangs.
func TestAnnahme_MessungAbStartOhneZustand(t *testing.T) {
	var cs *ChainState
	cs.AnnahmeMessungBeginnen()
}

// Missbrauch (Pruefung von #322, LOW-1): ein Beobachter nimmt auch dann keine
// Registrierung an, wenn er sonst nichts annimmt (ANNAHME_ROLLE=nur_lesend,
// Folger). Vorher lief sie an der Pause vorbei bis zur EVM, und der Mensch
// bekam "Registered", ohne je in einen Block zu kommen.
func TestAnnahme_BeobachterNimmtKeineRegistrierungAn(t *testing.T) {
	cs := newTestState()
	cs.SetzeNurLesend(true)
	a := &APIServer{state: cs}
	ip := "198.51.100.231"
	ipBurst.Delete("register:" + ip)
	t.Cleanup(func() { ipBurst.Delete("register:" + ip) })
	schicke := func() string {
		body := `{"wallet":"0x00000000000000000000000000000000000000ab","pA":["1","2"],"pB":[["1","2"],["3","4"]],"pC":["1","2"],"pubSignals":["1","2"],"signature":"0x01"}`
		req := httptest.NewRequest(http.MethodPost, "/api/register", strings.NewReader(body))
		req.RemoteAddr = ip + ":4711"
		w := httptest.NewRecorder()
		a.handleRegister(w, req)
		var resp RegisterResponse
		json.NewDecoder(w.Body).Decode(&resp)
		if resp.Success {
			t.Fatal("Registrierung angenommen")
		}
		return resp.Message
	}
	// Gegenprobe: ohne Beobachter kommt die Anfrage bis zur EVM (die es im
	// Test nicht gibt) -- die Pause gilt fuer einen nur lesenden Knoten nicht.
	if msg := schicke(); !strings.Contains(msg, "EVM engine unavailable") {
		t.Fatalf("Vorbedingung: ohne Beobachter bis zur EVM, bekam %q", msg)
	}
	setzeBeobachterFuerTest(t, true)
	if msg := schicke(); !strings.Contains(msg, "Beobachter") {
		t.Fatalf("Beobachter (nur lesend) nimmt eine Registrierung an: %q", msg)
	}
}

// Ein Beobachter wird nicht Leiter (Pruefung von #322, INFO-1): er erzeugt
// nie, als Leiter hielte er die Annahme des ganzen Netzes an.
func TestLeitung_BeobachterNichtLeiterfaehig(t *testing.T) {
	leistung.mu.Lock()
	altZwang := leistung.zwang
	leistung.zwang = "ja"
	leistung.mu.Unlock()
	t.Cleanup(func() { leistung.mu.Lock(); leistung.zwang = altZwang; leistung.mu.Unlock() })
	cs := newTestState()
	if !leiterFaehig(cs) {
		t.Fatal("Vorbedingung: mit Leistungsnachweis und annehmend leiterfaehig")
	}
	setzeBeobachterFuerTest(t, true)
	if leiterFaehig(cs) {
		t.Fatal("Beobachter ist leiterfaehig")
	}
}
