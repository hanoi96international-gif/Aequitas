package keeper

import (
	"testing"
)

// Was der Erzeuger fuer eine Blockzeit waehlt (auftragsFenster), muss
// blockTauglich und dem Nachspielen entsprechen -- sonst entstuende ein
// Block, den jeder abweist, oder einer fehlte.
func TestStrafeAbrechnung_FensterWieNachspielen(t *testing.T) {
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	for _, ab := range []int64{1, 1_000_000, 1_000_000 + 1800, 1_000_000 + 7200} {
		registerLeserOverride.Store(ab)
		for _, d := range []int64{1_000_000 - 7200, 1_000_000, 1_000_000 + 3600} {
			for _, typ := range []string{"slash_equivocation", "slash_abrechnung"} {
				tx := &Transaction{Type: typ, Wallet: "0xs", BlockAHash: "a", BlockBHash: "b", DetectedAt: d}
				for bz := d - 600; bz <= d+5*3600; bz += 60 {
					f := auftragsFenster(tx, bz)
					imFenster := bz >= f.von && bz <= f.bis
					tauglich := blockTauglich(tx, bz) == nil
					if imFenster != tauglich {
						t.Fatalf("%s, Stichtag %d, Tat %d, Block %d: Fenster %v, tauglich %v", typ, ab, d, bz, imFenster, tauglich)
					}
					if typ == "slash_equivocation" && tauglich != (beweisFrischPruefen(d, bz) == nil) {
						t.Fatalf("Frische: Fenster und Nachspielen weichen ab (Tat %d, Block %d)", d, bz)
					}
				}
			}
		}
	}
}

// Missbrauch: ab dem Stichtag kein Beweis, der mehr als W nach der Tat in
// einem Block steht; davor wie bisher ohne Grenze.
func TestStrafeAbrechnung_FrischeNurAbStichtag(t *testing.T) {
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	registerLeserOverride.Store(10_000)
	if err := beweisFrischPruefen(100, 9_999); err != nil {
		t.Fatalf("vor dem Stichtag begrenzt: %v", err)
	}
	if err := beweisFrischPruefen(10_000-strafBeweisFrisch-1, 10_000); err == nil {
		t.Fatal("alter Beweis nach dem Stichtag angenommen")
	}
	if err := beweisFrischPruefen(10_000-strafBeweisFrisch, 10_000); err != nil {
		t.Fatalf("Beweis genau W alt abgewiesen: %v", err)
	}
	// In die Zukunft datiert (Sicherheitspruefung #306): hoechstens fuenf
	// Minuten nach dem Block.
	if err := beweisFrischPruefen(10_000+nachweisHoechstensVoraus+1, 10_000); err == nil {
		t.Fatal("in die Zukunft datierter Beweis angenommen")
	}
	if err := beweisFrischPruefen(10_000+nachweisHoechstensVoraus, 10_000); err != nil {
		t.Fatalf("Beweis fuenf Minuten voraus abgewiesen: %v", err)
	}
}

// Die Abrechnung liest Bindungen bis Tat + W. Jede solche Bindung steht in
// einem Block hoechstens nachweisHoechstensAlt nach ihrem Zeitpunkt; bis zur
// Faelligkeit hat also jeder Knoten mindestens eine Stunde, sie
// nachzuspielen.
func TestStrafeAbrechnung_FaelligkeitLaesstZeitZumNachspielen(t *testing.T) {
	d := int64(1_000_000)
	luft := strafeFaelligAb(d) - (d + strafBeweisFrisch + nachweisHoechstensAlt)
	if luft < 3600 {
		t.Fatalf("nur %d s zwischen der letzten zaehlenden Bindung und der Abrechnung", luft)
	}
	if strafBeweisMarge >= strafBeweisFrisch || strafAbrechnungMarge < zeitstempelRueckToleranz {
		t.Fatal("Margen passen nicht zu den Fristen")
	}
}

// slash_abrechnung ist vor registerLeserAb unbekannt -- ein Block damit wird
// abgewiesen wie mit einer erfundenen Art.
func TestStrafeAbrechnung_UnbekanntVorDemStichtag(t *testing.T) {
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	registerLeserOverride.Store(10_000)
	if bekannteTxArt("slash_abrechnung", 9_999) {
		t.Fatal("vor dem Stichtag bekannt")
	}
	if !bekannteTxArt("slash_abrechnung", 10_000) {
		t.Fatal("ab dem Stichtag unbekannt")
	}
}
