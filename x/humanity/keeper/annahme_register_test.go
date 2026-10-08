package keeper

import (
	"errors"
	"testing"
	"time"
)

// Missbrauch der Annahme (Pruefung von #317): ein Knoten, dessen Schluessel
// das Register ab erzeugerSchnittAb (noch) nicht traegt -- frisch gebunden,
// in der Frist --, erzeugt nicht (nicht_im_register). Er darf dann auch
// nicht unbegrenzt annehmen: seine Auftraege kaemen in keinen Block und
// waeren beim Beginn seines Fensters zu alt fuer jede Blockzeit. Die
// Messung der Annahme-Pause beginnt darum schon mit diesem Versuch.
func TestAnnahme_PausiertOhneRegister(t *testing.T) {
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	dag, cs := newDeterminismTestDAG()
	self := "0x00000000000000000000000000000000000000c3"
	dag.selfProposer = self
	// Gebunden, aber das Fenster beginnt erst in einer Stunde (Frist).
	cs.erzeugerRegister.Store(&erzeugerStand{fenster: map[string][]zeitfenster{
		self: {{betreiber: "0xm1", von: nowUnix() + 3600, bis: 1 << 62}},
	}})
	if cs.annahmePauseGrund() != nil {
		t.Fatal("Vorbedingung: vor dem ersten Versuch nimmt der Knoten an")
	}
	produktionLetzterGrnd.Store("")
	if b := dag.ProduceBlock(); b != nil {
		t.Fatal("Block ohne Fenster im Register")
	}
	if g, _ := produktionLetzterGrnd.Load().(string); g != "nicht_im_register" {
		t.Fatalf("Grund %q, erwartet nicht_im_register", g)
	}
	// Sofort, nicht erst nach 30 s: was jetzt angenommen wuerde, waere zu
	// Beginn des Fensters zu alt fuer jede Blockzeit.
	if err := cs.annahmeBeginnen(distTestAddr(2101)); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme ohne Register nicht sofort angehalten: %v", err)
	}
	if cs.erzeugerSeit.Load() == 0 {
		t.Fatal("die Messung der Annahme-Pause hat nicht begonnen")
	}

	// Das Fenster beginnt: der Knoten erzeugt wieder (Grund nicht mehr
	// nicht_im_register), die Markierung faellt.
	cs.erzeugerRegister.Store(&erzeugerStand{fenster: map[string][]zeitfenster{
		self: {{betreiber: "0xm1", von: 0, bis: 1 << 62}},
	}})
	produktionLetzterGrnd.Store("")
	dag.ProduceBlock()
	if g, _ := produktionLetzterGrnd.Load().(string); g == "nicht_im_register" {
		t.Fatal("Vorbedingung: mit Fenster weiter nicht_im_register")
	}
	if cs.nichtImRegister.Load() {
		t.Fatal("Markierung nach bestandener Registerpruefung nicht geloescht")
	}
	// Die Messung beginnt nur einmal (CompareAndSwap): ein weiterer Versuch,
	// der keinen Block baut (hier: ein Resync laeuft), setzt sie nicht
	// zurueck -- sonst pausierte ein Knoten, der nie einen Block speichert,
	// nie.
	cs.erzeugerSeit.Store(time.Now().Unix() - admissionStallLimit() - 1)
	cs.letzterEigenerBlock.Store(0)
	dag.resyncInProgress.Store(true)
	produktionLetzterGrnd.Store("")
	dag.ProduceBlock()
	if g, _ := produktionLetzterGrnd.Load().(string); g != "resync_laeuft" {
		t.Fatalf("Vorbedingung: Grund %q, erwartet resync_laeuft", g)
	}
	if err := cs.annahmeBeginnen(distTestAddr(2102)); !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme nach %d s ohne eigenen Block nicht angehalten: %v", admissionStallLimit(), err)
	}
}
