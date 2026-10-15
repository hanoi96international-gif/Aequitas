package keeper

import (
	"testing"
	"time"
)

// Missbrauch/Fehlerfall (Beta-Pruefung 14.10.2026, BLOCKER): ein nur
// lesender Knoten wird nie Leiter -- sonst naehme das ganze Netz nichts an
// (annahme_tor.go). Nicht als Start-Leiter, nicht im Notbetrieb, nicht nach
// einem Neustart; schaltet ein Leiter zur Laufzeit auf nur lesend, gibt er ab.

// laufeOhneLeiter: laeuft d in Sekundenschritten und prueft dabei, dass
// Knoten i nie leitet.
func (n *simNetz) laufeOhneLeiter(t *testing.T, i int, d time.Duration) {
	t.Helper()
	for verstrichen := time.Duration(0); verstrichen < d; verstrichen += time.Second {
		n.laufe(time.Second)
		if n.knoten[i].l.rolle == leitLeiter || n.knoten[i].l.DarfAnnehmen(n.uhr(i)) {
			t.Fatalf("Knoten %d (nur lesend) leitet nach %s", i, verstrichen)
		}
	}
}

func TestLeitung_NieLeiter_StartLeiterImNotbetrieb(t *testing.T) {
	n := neuesSimNetz(t, 4, 31, testKonfig())
	n.faehig = map[string]bool{} // keiner hat den Nachweis: Notbetrieb
	n.nieLeiter = map[string]bool{n.knoten[0].addr: true}
	for i := range n.knoten {
		n.baue(i)
	}
	n.laufeOhneLeiter(t, 0, 90*time.Second)
	if i := n.annehmender(); i < 1 {
		t.Fatalf("kein anderer Leiter, obwohl der Start-Leiter nur liest (%d)", i)
	}
}

func TestLeitung_NieLeiter_NichtNachNeustart(t *testing.T) {
	n := neuesSimNetz(t, 4, 32, testKonfig())
	n.laufe(10 * time.Second)
	if i := n.annehmender(); i != 0 {
		t.Fatalf("Startleiter 0 erwartet, nimmt %d an", i)
	}
	// Knoten 0 startet neu -- jetzt nur lesend. Der gespeicherte Stand nennt
	// ihn als Leiter; er darf die Rolle nicht wieder aufnehmen.
	n.nieLeiter = map[string]bool{n.knoten[0].addr: true}
	n.baue(0)
	n.laufeOhneLeiter(t, 0, 90*time.Second)
	if i := n.annehmender(); i < 1 {
		t.Fatalf("nach dem Neustart kein anderer Leiter (%d)", i)
	}
}

func TestLeitung_NieLeiter_LaufzeitWechselGibtAb(t *testing.T) {
	n := neuesSimNetz(t, 4, 33, testKonfig())
	n.laufe(10 * time.Second)
	if i := n.annehmender(); i != 0 {
		t.Fatalf("Startleiter 0 erwartet, nimmt %d an", i)
	}
	n.nieLeiter = map[string]bool{n.knoten[0].addr: true}
	n.laufe(60 * time.Second)
	if n.knoten[0].l.rolle == leitLeiter {
		t.Fatal("Leiter gibt nach Wechsel auf nur lesend nicht ab")
	}
	if i := n.annehmender(); i < 1 {
		t.Fatalf("kein neuer Leiter nach der Abgabe (%d)", i)
	}
	n.laufeOhneLeiter(t, 0, 60*time.Second)
	if n.zweiLeiter {
		t.Fatal("zwei Leiter gleichzeitig")
	}
}

// Zwei Validatoren ohne Wahl (Pruefung von #354, MITTEL): ist der Start-
// oder gespeicherte Leiter nur lesend, muss der andere die Leitung bekommen.
func TestLeitung_NieLeiter_ZweiValidatoren(t *testing.T) {
	for _, neustart := range []bool{false, true} {
		n := neuesSimNetz(t, 2, 34, testKonfig())
		if neustart {
			n.laufe(10 * time.Second)
			if i := n.annehmender(); i != 0 {
				t.Fatalf("Startleiter 0 erwartet, nimmt %d an", i)
			}
		}
		n.nieLeiter = map[string]bool{n.knoten[0].addr: true}
		for i := range n.knoten {
			n.baue(i)
		}
		n.laufe(60 * time.Second)
		if i := n.annehmender(); i != 1 {
			t.Fatalf("neustart=%v: Knoten 1 soll annehmen, nimmt %d an", neustart, i)
		}
		if n.knoten[0].l.DarfAnnehmen(n.uhr(0)) {
			t.Fatalf("neustart=%v: nur lesender Knoten nimmt an", neustart)
		}
	}
}
