package keeper

import (
	"errors"
	"testing"
)

// Komitee aus dem Register (komiteeKandidaten): ab erzeugerSchnittAb waehlt
// jeder Knoten aus derselben Menge -- den Schluesseln, deren Bloecke alle
// annehmen. Vorher wie bisher aus der lokalen Liste.
func TestEpochenKomitee_AusDemRegister(t *testing.T) {
	jetzt := nowUnix()
	neu := func() *BlockDAG {
		dag := newGhostdagTestDAG()
		dag.state = newTestState()
		dag.selfProposer = "0xk1"
		// Lokal bekannt, aber nie gebunden: kam per Abgleich oder Eintragung.
		dag.authorizedValidators = map[string]bool{"0xk1": true, "0xfremd": true}
		dag.state.erzeugerRegister.Store(standAus(
			z("0xm1", "0xk1", jetzt-3*erzeugerFrist), // gebunden, Fenster offen
			z("0xm2", "0xk2", jetzt-3*erzeugerFrist), // gebunden, nicht lokal bekannt
			z("0xm3", "0xk3", jetzt),                 // gerade gebunden: Frist laeuft
		))
		return dag
	}

	// Vor dem Stichtag: wie bisher die lokale Liste.
	dag := neu()
	ec := dag.computeEpochCommittee(7)
	if ec == nil || !ec.Members["0xk1"] || !ec.Members["0xfremd"] || ec.Members["0xk2"] {
		t.Fatalf("vor dem Stichtag: %+v -- erwartet die lokale Liste", ec)
	}

	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })

	// Ab dem Stichtag: nur, wer zur Zeit jetzt im Register erzeugen darf --
	// nicht der lokal bekannte Fremde, nicht der in der Frist.
	dag = neu()
	ec = dag.computeEpochCommittee(7)
	if ec == nil || !ec.Members["0xk1"] || !ec.Members["0xk2"] || ec.Members["0xfremd"] || ec.Members["0xk3"] || ec.Size != 2 {
		t.Fatalf("ab dem Stichtag: %+v -- erwartet genau k1 und k2", ec)
	}
	// Zwei Knoten mit verschiedenen lokalen Listen, aber demselben Register:
	// dasselbe Komitee.
	ander := neu()
	ander.selfProposer = "0xk2"
	ander.authorizedValidators = map[string]bool{"0xk2": true, "0xnoch-einer": true}
	if ec2 := ander.computeEpochCommittee(7); len(ec2.Members) != len(ec.Members) || !ec2.Members["0xk1"] || !ec2.Members["0xk2"] {
		t.Fatalf("anderer Knoten, anderes Komitee: %+v gegen %+v", ec2, ec)
	}

	// Geschlossener Betrieb: die Schnittmenge mit AUTHORIZED_VALIDATORS.
	dag = neu()
	dag.produzentenFest = map[string]bool{"0xk1": true}
	if ec := dag.computeEpochCommittee(7); !ec.Members["0xk1"] || ec.Members["0xk2"] || ec.Size != 1 {
		t.Fatalf("geschlossen: %+v -- erwartet nur k1", ec)
	}
}

// Missbrauch und Fehlerfall: kein lesbarer Stand oder keiner im Register
// heisst "keiner erzeugt", nie "jeder darf" (wie beim Hochfahren) und nie
// die lokale Liste. Ein unlesbarer Stand wird nicht fuer die ganze Epoche
// gemerkt.
func TestEpochenKomitee_RegisterFailClosed(t *testing.T) {
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	jetzt := nowUnix()
	dag := newGhostdagTestDAG()
	dag.state = newTestState()
	dag.selfProposer = "0xk1"
	dag.authorizedValidators = map[string]bool{"0xk1": true}

	dag.state.erzeugerRegister.Store(&erzeugerStand{fehler: errors.New("db weg")})
	ec := dag.getEpochCommittee(42)
	if ec == nil || len(ec.Members) != 0 {
		t.Fatalf("unlesbarer Stand: %+v -- erwartet ein leeres Komitee", ec)
	}
	// Wieder lesbar: der naechste Versuch derselben Epoche rechnet neu.
	dag.state.erzeugerRegister.Store(standAus(z("0xm1", "0xk1", jetzt-3*erzeugerFrist)))
	if ec := dag.getEpochCommittee(42); ec == nil || !ec.Members["0xk1"] {
		t.Fatalf("nach dem Lesefehler: %+v -- der leere Stand wurde fuer die Epoche gemerkt", ec)
	}

	// Lesbar, aber niemand gebunden: leer, nicht nil.
	dag = newGhostdagTestDAG()
	dag.state = newTestState()
	dag.selfProposer = "0xk1"
	dag.authorizedValidators = map[string]bool{"0xk1": true}
	dag.state.erzeugerRegister.Store(standAus())
	if ec := dag.computeEpochCommittee(1); ec == nil || len(ec.Members) != 0 {
		t.Fatalf("leeres Register: %+v -- erwartet leer, nicht nil (nil hiesse: jeder erzeugt)", ec)
	}
	// Ohne Zustand ueberhaupt: ebenso.
	dag.state = nil
	if ec := dag.computeEpochCommittee(1); ec == nil || len(ec.Members) != 0 {
		t.Fatalf("ohne Zustand: %+v", ec)
	}
}
