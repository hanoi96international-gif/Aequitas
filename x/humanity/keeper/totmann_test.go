package keeper

import "testing"

// Der Schalter beendet den Prozess. Was er meldet, muss deshalb stimmen --
// und die Schwellen muessen so stehen, dass ein aufholender Knoten sie nie
// erreicht.
func TestTotmann_SchwellenSindKonservativ(t *testing.T) {
	s := TotmannStand()
	// Gemessen wurden unter Volllast hoechstens 29 Sekunden Stillstand. Alles
	// unter einer Minute wuerde im Normalbetrieb ausloesen.
	if got := s["stillstand_sekunden"].(int64); got < 120 {
		t.Errorf("stillstand_sekunden = %d -- zu kurz, das loest im Normalbetrieb aus", got)
	}
	// Ein Partner, der nur ein paar Bloecke voraus ist, beweist nichts.
	if got := s["mindest_abstand"].(int64); got < 20 {
		t.Errorf("mindest_abstand = %d -- zu klein, normale Streuung wuerde ausloesen", got)
	}
	if s["ausgeloest"].(bool) {
		t.Error("ausgeloest ist im Test wahr")
	}
}

// Ohne Partner darf der Schalter nichts tun -- sonst beendete sich ein
// alleinstehender Knoten selbst.
func TestTotmann_OhnePartnerPassiertNichts(t *testing.T) {
	dag := &BlockDAG{}
	dag.StarteTotmannSchalter("") // darf nicht panicken und nichts starten
	if TotmannStand()["ausgeloest"].(bool) {
		t.Error("ohne Partner ausgeloest")
	}
}
