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
	seit := cs.erzeugerSeit.Load()
	if seit == 0 {
		t.Fatal("die Messung der Annahme-Pause hat nicht begonnen -- dieser Knoten naehme unbegrenzt an")
	}
	// Nach admissionStallLimit ohne eigenen Block: Annahme angehalten.
	cs.erzeugerSeit.Store(time.Now().Unix() - admissionStallLimit() - 1)
	if err := cs.annahmePausiert(); err == nil || !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme nach %d s ohne eigenen Block nicht angehalten: %v", admissionStallLimit(), err)
	}
}
