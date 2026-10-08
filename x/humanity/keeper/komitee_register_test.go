package keeper

import (
	"errors"
	"fmt"
	"testing"
)

// komiteeStand: ein Stand mit n Schluesseln, jeder von einem eigenen
// Menschen gebunden, gueltig in [von, bis).
func komiteeStand(n int, von, bis int64) *erzeugerStand {
	st := &erzeugerStand{fenster: map[string][]zeitfenster{}, halter: map[string]string{}}
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0x%040x", 0x1000+i)
		st.fenster[addr] = []zeitfenster{{betreiber: fmt.Sprintf("0xb%039x", i), von: von, bis: bis}}
	}
	return st
}

func komiteeDAG(st *erzeugerStand, self string) *BlockDAG {
	dag := newGhostdagTestDAG()
	dag.state = newTestState()
	if st != nil {
		dag.state.erzeugerRegister.Store(st)
	}
	dag.selfProposer = self
	return dag
}

func erzeugerSchnittAb1(t *testing.T) {
	t.Helper()
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
}

// Missbrauch: ab erzeugerSchnittAb waehlt jeder Knoten dasselbe Komitee aus
// dem Register -- auch wenn einer eine lokale Liste mit 500 erfundenen
// Adressen kennt (Abgleich unter Peers, Registrierungen bei ihm) und seine
// Spitze auf einer anderen Hoehe steht. Vorher drueckte so eine Liste echte
// Validatoren aus dem Komitee dieses Knotens.
func TestKomiteeAusRegister_GleichAufJedemKnoten(t *testing.T) {
	jetzt := int64(1_900_000_000)
	st := komiteeStand(200, 0, jetzt+86400)
	gefuellt := komiteeDAG(st, "")
	gefuellt.authorizedValidators = map[string]bool{}
	for i := 0; i < 500; i++ {
		gefuellt.authorizedValidators[fmt.Sprintf("0x%040x", 0x900000+i)] = true
	}
	leer := komiteeDAG(st, "")

	// Vor dem Stichtag: wie bisher aus der lokalen Liste -- die erfundenen
	// Adressen stehen im Komitee.
	vorher := gefuellt.erzeugerKomitee(5, jetzt)
	if vorher == nil {
		t.Fatal("vor dem Stichtag kein Komitee")
	}
	erfunden := 0
	for a := range vorher.Members {
		if gefuellt.authorizedValidators[a] {
			erfunden++
		}
	}
	if erfunden != targetCommitteeSize {
		t.Fatalf("vor dem Stichtag: %d von %d aus der lokalen Liste -- der Test beweist nichts", erfunden, vorher.Size)
	}

	erzeugerSchnittAb1(t)
	a := gefuellt.erzeugerKomitee(5, jetzt)
	b := leer.erzeugerKomitee(987_654, jetzt)
	if a == nil || b == nil || a.Size != targetCommitteeSize || b.Size != targetCommitteeSize {
		t.Fatalf("Komitee aus dem Register: %+v / %+v", a, b)
	}
	if a.Number != jetzt/epochLength || b.Number != a.Number {
		t.Fatalf("Epoche nach der Zeit erwartet: %d / %d, Zeit %d", a.Number, b.Number, jetzt/epochLength)
	}
	for m := range a.Members {
		if !b.Members[m] {
			t.Fatalf("%s nur im Komitee des Knotens mit der gefuellten Liste", m)
		}
		if _, ok := st.fenster[m]; !ok {
			t.Fatalf("%s steht nicht im Register", m)
		}
	}
}

// Kandidat ist, wer zum BEGINN der Epoche in genau einem Erzeugerfenster
// steht: frisch gebunden erst in der naechsten Epoche, abgelaufen nicht,
// umstritten nicht.
func TestKomiteeAusRegister_FensterZumEpochenbeginn(t *testing.T) {
	erzeugerSchnittAb1(t)
	beginn := int64(1_900_000_800) / epochLength * epochLength
	jetzt := beginn + 1800
	st := &erzeugerStand{fenster: map[string][]zeitfenster{
		"0xaa": {{betreiber: "0xm1", von: 0, bis: beginn + 86400}},
		// mitten in der Epoche gebunden (Frist abgelaufen): erst naechste
		"0xbb": {{betreiber: "0xm2", von: beginn + 60, bis: beginn + 86400}},
		// vor Beginn der Epoche abgelaufen
		"0xcc": {{betreiber: "0xm3", von: 0, bis: beginn}},
		// umstritten: zwei Fenster zugleich
		"0xdd": {{betreiber: "0xm4", von: 0, bis: beginn + 86400}, {betreiber: "0xm5", von: 0, bis: beginn + 86400}},
		// endet mitten in der Epoche: Kandidat (ProduceBlock prueft danach
		// selbst, nicht_im_register)
		"0xee": {{betreiber: "0xm6", von: 0, bis: beginn + 60}},
	}}
	dag := komiteeDAG(st, "0xaa")
	ec := dag.erzeugerKomitee(1, jetzt)
	if ec == nil {
		t.Fatal("kein Komitee")
	}
	soll := map[string]bool{"0xaa": true, "0xee": true}
	if len(ec.Members) != len(soll) {
		t.Fatalf("Komitee %v, erwartet %v", ec.Members, soll)
	}
	for m := range soll {
		if !ec.Members[m] {
			t.Fatalf("Komitee %v, erwartet %v", ec.Members, soll)
		}
	}
	// Naechste Epoche: 0xbb dabei, 0xee nicht mehr.
	nachher := dag.erzeugerKomitee(1, beginn+epochLength+5)
	if nachher == nil || !nachher.Members["0xbb"] || nachher.Members["0xee"] || nachher.Members["0xcc"] || nachher.Members["0xdd"] {
		t.Fatalf("naechste Epoche: %+v", nachher)
	}
}

// Fail-closed: ohne Stand oder nach einem Lesefehler kein Komitee -- und
// erzeugt wird trotzdem nicht, weil ProduceBlock den eigenen Schluessel
// vorher gegen denselben Stand prueft.
func TestKomiteeAusRegister_OhneStandKeinErzeugen(t *testing.T) {
	erzeugerSchnittAb1(t)
	jetzt := int64(1_900_000_000)
	ohne := komiteeDAG(nil, "0x0000000000000000000000000000000000001000")
	if ec := ohne.erzeugerKomitee(1, jetzt); ec != nil {
		t.Fatalf("ohne Stand: %+v", ec)
	}
	if ohne.erzeugerNachRegister(ohne.selfProposer, jetzt) {
		t.Fatal("ohne Stand darf dieser Knoten erzeugen")
	}
	st := komiteeStand(3, 0, jetzt+86400)
	kaputt := komiteeDAG(&erzeugerStand{fenster: st.fenster, fehler: errors.New("db weg")}, "0x0000000000000000000000000000000000001000")
	if ec := kaputt.erzeugerKomitee(1, jetzt); ec != nil {
		t.Fatalf("nach einem Lesefehler: %+v", ec)
	}
	if kaputt.erzeugerNachRegister(kaputt.selfProposer, jetzt) {
		t.Fatal("nach einem Lesefehler darf dieser Knoten erzeugen")
	}
	// Ein Komitee aus einem frueheren Stand gilt nach dem Lesefehler nicht
	// weiter.
	gut := komiteeDAG(st, "0x0000000000000000000000000000000000001000")
	if ec := gut.erzeugerKomitee(1, jetzt); ec == nil || ec.Size != 3 {
		t.Fatalf("mit Stand: %+v", ec)
	}
	gut.state.erzeugerRegister.Store(&erzeugerStand{fenster: st.fenster, fehler: errors.New("db weg")})
	if ec := gut.erzeugerKomitee(1, jetzt); ec != nil {
		t.Fatalf("Komitee aus dem alten Stand nach einem Lesefehler: %+v", ec)
	}
}

// Ein neu gelesener Stand gilt sofort, auch in derselben Epoche (ein
// nachgeholter Block mit einer alten Bindung); derselbe Stand rechnet nicht
// neu.
func TestKomiteeAusRegister_NeuerStandGilt(t *testing.T) {
	erzeugerSchnittAb1(t)
	jetzt := int64(1_900_000_000)
	st := komiteeStand(3, 0, jetzt+86400)
	dag := komiteeDAG(st, "")
	erst := dag.erzeugerKomitee(1, jetzt)
	if erst == nil || erst.Size != 3 {
		t.Fatalf("erst: %+v", erst)
	}
	if nochmal := dag.erzeugerKomitee(1, jetzt+10); nochmal != erst {
		t.Fatal("derselbe Stand, dieselbe Epoche: neu berechnet")
	}
	neu := komiteeStand(4, 0, jetzt+86400)
	dag.state.erzeugerRegister.Store(neu)
	if ec := dag.erzeugerKomitee(1, jetzt+20); ec == nil || ec.Size != 4 {
		t.Fatalf("neuer Stand nicht uebernommen: %+v", ec)
	}
}
