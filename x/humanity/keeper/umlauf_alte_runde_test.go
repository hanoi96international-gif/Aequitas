package keeper

import "testing"

// Missbrauch (heutige Regeln, Sicherheitspruefung 25.10.2026): ein Umlauf mit
// einer Rundenzeit vor der letzten Runde wurde weder nachgerechnet noch
// begrenzt -- ein Erzeuger nahm damit jedem Konto das ganze Guthaben. Jetzt
// gemeldet (umlauf_alte_runde) und ab dem gemeinsamen Stichtag abgelehnt,
// entschieden nach der Blockzeit, nicht nach der gefaelschten Rundenzeit.
func TestUmlauf_AlteRundeNimmtKeinGeld(t *testing.T) {
	wirtschaftAn(t)
	for _, streng := range []bool{false, true} {
		nachrechnenModus(t, streng)
		dag, nach := newDeterminismTestDAG()
		addHuman(nach, wMensch1, 8000)
		geben(nach, wFrei, 200)
		at := nowUnix()
		w := nach.wirt()
		w.mitSperre(func() { w.letzterUmlauf, w.laufAt, w.laufVorher = at, at, at-86400 })
		vorher := erhaltungZaehler("umlauf_alte_runde")
		raub := []Transaction{
			{Type: "umlauf", Wallet: wMensch1, Amount: 8000, DistributionAt: at - 86400},
			{Type: "umlauf", Wallet: wFrei, Amount: 200, DistributionAt: at - 7*86400},
		}
		ok := dag.replayTransactions(erhaltungBlock(1, at+5, raub...), true)
		n := erhaltungZaehler("umlauf_alte_runde") - vorher
		if !streng {
			if n != 2 {
				t.Fatalf("beobachten: beide alten Runden zaehlen, gezaehlt %d", n)
			}
			continue
		}
		if ok || n != 1 {
			t.Fatalf("streng: Block angenommen=%v, gemeldet %d", ok, n)
		}
		if b := acct(nach, wMensch1).Balance.Float(); b != 8000 {
			t.Fatalf("streng: Guthaben genommen, steht %v", b)
		}
		if b := acct(nach, wFrei).Balance.Float(); b != 200 {
			t.Fatalf("streng: Guthaben genommen, steht %v", b)
		}
	}
	// Gleich alt (Neustart mitten in der Runde: letzterUmlauf = at, laufAt
	// unbekannt) bleibt wie bisher ungeprueft und wird nicht gemeldet.
	nachrechnenModus(t, true)
	cs := newTestState()
	addHuman(cs, wMensch1, 8000)
	at := nowUnix()
	w := cs.wirt()
	w.mitSperre(func() { w.letzterUmlauf, w.laufAt = at, 0 })
	cs.mu.Lock()
	err := cs.pruefeUmlaufLocked(wMensch1, 1, at, at+5)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("Neustart mitten in der Runde darf nicht ablehnen: %v", err)
	}
}
