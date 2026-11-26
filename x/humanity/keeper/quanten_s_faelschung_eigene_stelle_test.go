package keeper

import "testing"

// Missbrauch (Pruefung vom 26.11.2026): Stelle B gehoert der Inhaberin. Sie
// kennt nur, was jede ehrliche Praeparatorin ihr als Oeffnung schickt --
// Basis und Wert der Stellen ihrer EIGENEN Stichprobe -- und baut daraus eine
// Vorlage, die B's Stichprobe besteht, ohne eine einzige unehrliche Geloste.
// Der Nachweis gelingt (das laesst sich nicht verhindern; darum beweist er
// kein Leck der Gelosten), kostet die Gelosten aber nichts: B bekommt nur das
// Pfand der Inhaberin, also Geld zwischen den Beteiligten.
func TestQuantenSFaelschung_EigeneStelleB(t *testing.T) {
	w, _ := neueFwWelt(t, true)
	n := w.cs.quantenWJetzt().Nachweis[w.p.K]
	if n == nil {
		t.Fatal("kein Nachweis-Eintrag")
	}
	tok, err := quantenWSTokenLesen(n.Token)
	if err != nil {
		t.Fatal(err)
	}
	// x' = die ehrliche Messung der Inhaberin, nur auf B's Stichprobe aus der
	// Oeffnung korrigiert -- kein Wissen ueber andere Stellen.
	x := append([]int(nil), w.x...)
	geaendert := 0
	for _, i := range w.probe[1] {
		if tok.token.D[i]^1^tok.token.C == w.p.basen[i] && x[i] != w.p.bits[i] {
			x[i] = w.p.bits[i]
			geaendert++
		}
	}
	if geaendert == 0 {
		t.Fatal("Testaufbau: die ehrliche Vorlage bestuende B's Stichprobe schon")
	}
	pfaender := map[string]int64{}
	for i := 0; i < fwGelost; i++ {
		pfaender[w.val[i]] = w.cs.quantenWJetzt().Praep[quantenSPraepK(w.val[i])].Pfand
	}
	b0, topf0 := w.stand(w.cs, w.stelle[1]), w.stand(w.cs, quantenWPfandAddr)
	z := w.fwEnde() + 100
	w.annehmen(w.fwAnmeldenTx(w.stelle[1], 1, x, w.anna), z)
	g := w.fwOffenlegenAlle(z + 10)
	if g.Urteil != quantenSUrteilFaelschung || g.Pfand != ipPfand || g.PfandAn != w.stelle[1] {
		t.Fatalf("Nachweis: %+v", g)
	}
	if d := w.stand(w.cs, w.stelle[1]) - b0; d != ipPfand {
		t.Fatalf("B bekam %d, nur das Pfand der Inhaberin (%d) waere richtig", d, ipPfand)
	}
	if d := topf0 - w.stand(w.cs, quantenWPfandAddr); d != ipPfand {
		t.Fatalf("Pfandtopf gab %d heraus, erwartet %d", d, ipPfand)
	}
	reg := w.cs.quantenWJetzt()
	for konto, vorher := range pfaender {
		if p := reg.Praep[quantenSPraepK(konto)].Pfand; p != vorher {
			t.Fatalf("Geloste %s verlor Pfand: %d -> %d", kurzAdresse(konto), vorher, p)
		}
	}
}
