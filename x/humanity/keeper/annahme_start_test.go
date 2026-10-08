package keeper

import (
	"errors"
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
