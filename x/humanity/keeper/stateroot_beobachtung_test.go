package keeper

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Missbrauchstests zu stateroot_beobachtung.go: ein Produzent mit gefaelschtem
// StateRoot muss je Produzent sichtbar werden, und kein Schwall von
// Produzenten darf die Zaehler unbegrenzt wachsen lassen.

// ruhigerKnoten: ein nachspielender Knoten, der gerade nichts annimmt --
// sonst gilt jede Abweichung als "lokal unscharf" (geschwisterbedingt).
func ruhigerKnoten(t *testing.T) (*BlockDAG, *ChainState) {
	t.Helper()
	alt := letzteEigeneUeberweisungNs.Load()
	letzteEigeneUeberweisungNs.Store(0)
	t.Cleanup(func() { letzteEigeneUeberweisungNs.Store(alt) })
	return newDeterminismTestDAG()
}

func srBlock(h int, produzent, root string) *Block {
	b := testBlock(h)
	b.Hash = fmt.Sprintf("0xsr-%d-%s", h, produzent)
	b.Proposer = produzent
	b.StateRoot = root
	return b
}

func srStandVon(t *testing.T, dag *BlockDAG, produzent string) (stateRootProduzentStand, bool) {
	t.Helper()
	dag.srBeobachtung.mu.Lock()
	defer dag.srBeobachtung.mu.Unlock()
	s, ok := dag.srBeobachtung.je[produzent]
	if !ok {
		return stateRootProduzentStand{}, false
	}
	return *s, true
}

// Angriff: ein berechtigter Produzent signiert einen Block mit falschem Root.
// Der Block wird (bewusst, siehe Dateikopf) weiter angenommen -- aber die
// Abweichung steht jetzt je Produzent als "echt" in der Anzeige, mit Hoehe
// und Root-Praefixen, und ein ehrlicher Produzent daneben bleibt sauber.
func TestStateRootBeobachtung_GefaelschterRootWirdJeProduzentSichtbar(t *testing.T) {
	dag, cs := ruhigerKnoten(t)

	const boese = "0xB0E5E00000000000000000000000000000000001"
	falsch := strings.Repeat("deadbeef", 8)
	if ok := dag.replayTransactions(srBlock(1, boese, falsch), true); !ok {
		t.Fatal("Block mit falschem Root abgewiesen -- das Verhalten der Annahme darf sich hier nicht aendern")
	}
	s, ok := srStandVon(t, dag, strings.ToLower(boese))
	if !ok {
		t.Fatalf("abweichender Produzent fehlt in der Anzeige: %+v", dag.StateRootJeProduzent())
	}
	if s.Vergleiche != 1 || s.Abweichend != 1 || s.Echt != 1 || s.Geschwister != 0 || s.Gleich != 0 {
		t.Fatalf("Zaehler falsch: %+v", s)
	}
	if s.LetzteAbweichungHoehe != 1 || s.LetzteAbweichungUnix == 0 {
		t.Fatalf("letzte Abweichung nicht festgehalten: %+v", s)
	}
	if s.LetzterRootBehauptet != "deadbeefdeadbeef" || len(s.LetzterRootLokal) != stateRootPraefixLaenge {
		t.Fatalf("Root-Praefixe falsch: %+v", s)
	}
	if got := dag.EchteStateRootAbweichungen(); got != 1 {
		t.Fatalf("EchteStateRootAbweichungen = %d, erwartet 1", got)
	}

	// Ehrlicher Produzent mit dem richtigen Root: zaehlt als gleich.
	cs.mu.Lock()
	richtig := cs.stateRootLocked(cs.getConfigValue("last_ubi_at"), cs.getConfigValue(letzterCheckpointSchluessel))
	cs.mu.Unlock()
	const ehrlich = "0xe000000000000000000000000000000000000002"
	if ok := dag.replayTransactions(srBlock(2, ehrlich, richtig), true); !ok {
		t.Fatal("ehrlicher Block abgewiesen")
	}
	e, ok := srStandVon(t, dag, ehrlich)
	if !ok || e.Gleich != 1 || e.Abweichend != 0 {
		t.Fatalf("ehrlicher Produzent falsch gezaehlt: %+v (da=%v)", e, ok)
	}

	// Kumuliert: ein passender Block des boesen Produzenten setzt die
	// Anzeige NICHT zurueck (die alten Zaehler schon -- sie messen Folgen).
	cs.mu.Lock()
	richtig = cs.stateRootLocked(cs.getConfigValue("last_ubi_at"), cs.getConfigValue(letzterCheckpointSchluessel))
	cs.mu.Unlock()
	if ok := dag.replayTransactions(srBlock(3, boese, richtig), true); !ok {
		t.Fatal("Block abgewiesen")
	}
	s, _ = srStandVon(t, dag, strings.ToLower(boese))
	if s.Vergleiche != 2 || s.Abweichend != 1 || s.Gleich != 1 {
		t.Fatalf("Anzeige nicht kumuliert: %+v", s)
	}
	if got := dag.EchteStateRootAbweichungen(); got != 0 {
		t.Fatalf("Folgezaehler nach Uebereinstimmung nicht zurueckgesetzt: %d", got)
	}

	// Gross-/Kleinvarianten derselben Adresse sind EIN Eintrag.
	dag.srBeobachtung.erfasse(strings.ToUpper(boese), 9, false, false, falsch, falsch, 1)
	s, _ = srStandVon(t, dag, strings.ToLower(boese))
	if s.Abweichend != 2 {
		t.Fatalf("Grossschreibung erzeugte einen eigenen Eintrag: %+v", dag.StateRootJeProduzent())
	}
}

// Angriff: ein Schwall neuer Produzentenadressen (offene Produzentenliste)
// mit falschen UND mit richtigen Roots. Keine Map darf ueber
// stateRootProduzentenMax wachsen, bestehende Eintraege bleiben, der Rest
// landet im Sammeltopf.
func TestStateRootBeobachtung_SchwallVonProduzentenBleibtBegrenzt(t *testing.T) {
	dag, cs := ruhigerKnoten(t)
	const n = 3 * stateRootProduzentenMax
	falsch := strings.Repeat("ab", 32)
	for i := 0; i < n; i++ {
		p := fmt.Sprintf("0x%040x", i+1)
		if ok := dag.replayTransactions(srBlock(i+1, p, falsch), true); !ok {
			t.Fatalf("Block %d abgewiesen", i)
		}
	}
	dag.stateRootMismatchesMu.Lock()
	for name, m := range map[string]map[string]int{
		"stateRootMismatches": dag.stateRootMismatches, "stateRootEcht": dag.stateRootEcht,
		"stateRootGeschwister": dag.stateRootGeschwister,
	} {
		if len(m) > stateRootProduzentenMax {
			t.Errorf("%s hat %d Eintraege, Grenze %d", name, len(m), stateRootProduzentenMax)
		}
	}
	if len(dag.stateRootMismatchLastAt) > stateRootProduzentenMax || len(dag.stateRootEchtLastAt) > stateRootProduzentenMax {
		t.Errorf("Zeitmarken-Maps unbegrenzt: %d/%d", len(dag.stateRootMismatchLastAt), len(dag.stateRootEchtLastAt))
	}
	dag.stateRootMismatchesMu.Unlock()

	dag.srBeobachtung.mu.Lock()
	je, verdichtet, uebrige := len(dag.srBeobachtung.je), dag.srBeobachtung.verdichtet, dag.srBeobachtung.uebrige
	dag.srBeobachtung.mu.Unlock()
	if je != stateRootProduzentenMax {
		t.Fatalf("Anzeige hat %d Eintraege, erwartet genau %d", je, stateRootProduzentenMax)
	}
	if verdichtet != n-stateRootProduzentenMax || uebrige.Abweichend != n-stateRootProduzentenMax {
		t.Fatalf("Sammeltopf falsch: verdichtet=%d uebrige=%+v", verdichtet, uebrige)
	}
	// Ein frueher Produzent zaehlt nach dem Schwall weiter in SEINEM Eintrag.
	erster := fmt.Sprintf("0x%040x", 1)
	if ok := dag.replayTransactions(srBlock(n+1, erster, falsch), true); !ok {
		t.Fatal("Block abgewiesen")
	}
	if s, ok := srStandVon(t, dag, erster); !ok || s.Abweichend != 2 {
		t.Fatalf("bestehender Eintrag nach dem Schwall verdraengt oder nicht weitergezaehlt: %+v (da=%v)", s, ok)
	}
	if out := dag.StateRootJeProduzent(); len(out["produzenten"].([]map[string]interface{})) > stateRootProduzentenMax {
		t.Fatal("Ausgabe unbegrenzt")
	}

	// Passende Roots von vielen neuen Produzenten legen in den alten
	// Folgezaehlern KEINE Eintraege an (vorher: "= 0" fuer jeden).
	dag2, cs2 := ruhigerKnoten(t)
	_ = cs
	for i := 0; i < n; i++ {
		cs2.mu.Lock()
		r := cs2.stateRootLocked(cs2.getConfigValue("last_ubi_at"), cs2.getConfigValue(letzterCheckpointSchluessel))
		cs2.mu.Unlock()
		p := fmt.Sprintf("0x%040x", 1000+i)
		if ok := dag2.replayTransactions(srBlock(i+1, p, r), true); !ok {
			t.Fatalf("Block %d abgewiesen", i)
		}
	}
	dag2.stateRootMismatchesMu.Lock()
	defer dag2.stateRootMismatchesMu.Unlock()
	if len(dag2.stateRootMismatches)+len(dag2.stateRootEcht)+len(dag2.stateRootGeschwister) != 0 {
		t.Fatalf("Uebereinstimmungen legten Eintraege an: %d/%d/%d",
			len(dag2.stateRootMismatches), len(dag2.stateRootEcht), len(dag2.stateRootGeschwister))
	}
}

// Root-Praefixe und Adressen kommen aus fremden Bloecken: nur Hex, feste Laenge.
func TestStateRootBeobachtung_FremdeWerteWerdenGekappt(t *testing.T) {
	if got := stateRootPraefix("<script>alert(1)</script>" + strings.Repeat("f", 100)); got != "cae1cfffffffffff" {
		t.Fatalf("Praefix nicht bereinigt: %q", got)
	}
	if got := stateRootSchluessel(strings.Repeat("A", 500)); len(got) != stateRootAdresseMax || got != strings.Repeat("a", stateRootAdresseMax) {
		t.Fatalf("Adresse nicht gekappt: %q", got)
	}
}

// Die Anzeige muss in /api/health/combined stehen, sonst sieht sie keiner.
func TestStateRootBeobachtung_StehtImGesundheitsEndpunkt(t *testing.T) {
	body := quelleLesen(t, "api.go")
	i := strings.Index(body, "func (a *APIServer) handleCombinedHealth(")
	if i < 0 {
		t.Fatal("handleCombinedHealth nicht gefunden")
	}
	rumpf := body[i:]
	if j := strings.Index(rumpf[1:], "\nfunc "); j > 0 {
		rumpf = rumpf[:j]
	}
	if !strings.Contains(rumpf, `"stateroot_je_produzent": a.blockchain.StateRootJeProduzent()`) {
		t.Fatal("stateroot_je_produzent fehlt in /api/health/combined")
	}
}

// Nachspielen schreibt, /api/health/combined liest -- nebenlaeufig. Unter
// go test -race darf das nicht melden.
func TestStateRootBeobachtung_NebenlaeufigLesenUndSchreiben(t *testing.T) {
	var o stateRootBeobachtung
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			o.erfasse(fmt.Sprintf("0x%d", i%300), int64(i), i%3 == 0, i%2 == 0, "ab", "cd", int64(i))
		}
	}()
	for i := 0; i < 200; i++ {
		_ = o.stand()
	}
	<-done
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.je) != stateRootProduzentenMax {
		t.Fatalf("%d Eintraege, erwartet %d", len(o.je), stateRootProduzentenMax)
	}
}

// MISSBRAUCH (Pruefung von #383): 64 Produzenten weichen je einmal ab und
// passen danach wieder. Ein NEUER Produzent, der danach dauerhaft abweicht,
// muss in den Signalen (echt/gesamt, [ALERT], Selbstheilung) zaehlen --
// stehengelassene 0-Eintraege durften die Plaetze nicht fuer immer belegen.
func TestStateRootBeobachtung_VolleMapsVerdeckenKeinenNeuenAbweichler(t *testing.T) {
	dag, cs := ruhigerKnoten(t)
	falsch := strings.Repeat("cd", 32)
	richtig := func() string {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		return cs.stateRootLocked(cs.getConfigValue("last_ubi_at"), cs.getConfigValue(letzterCheckpointSchluessel))
	}
	h := 0
	for i := 0; i < stateRootProduzentenMax; i++ {
		p := fmt.Sprintf("0x%040x", 5000+i)
		h++
		dag.replayTransactions(srBlock(h, p, falsch), true)
		h++
		dag.replayTransactions(srBlock(h, p, richtig()), true)
	}
	neu := fmt.Sprintf("0x%040x", 9999)
	for i := 0; i < 20; i++ {
		h++
		dag.replayTransactions(srBlock(h, neu, falsch), true)
	}
	if e, g := dag.EchteStateRootAbweichungen(), dag.TotalStateRootMismatches(); e != 20 || g != 20 {
		t.Fatalf("neuer Abweichler nach 64 erholten Produzenten: echt=%d gesamt=%d, erwartet 20/20", e, g)
	}
}

// MISSBRAUCH: 64 Produzenten weichen ab und hoeren dann auf (Eintraege
// werden alt). Ein neuer Abweichler bekommt trotzdem einen Platz -- alte
// Eintraege zaehlt kein Leser mehr, sie werden bei voller Map geraeumt.
// Gegenprobe: solange die 64 frisch sind, bleibt die Grenze hart.
func TestStateRootBeobachtung_AlteEintraegeMachenPlatz(t *testing.T) {
	dag, _ := ruhigerKnoten(t)
	falsch := strings.Repeat("ef", 32)
	for i := 0; i < stateRootProduzentenMax; i++ {
		dag.replayTransactions(srBlock(i+1, fmt.Sprintf("0x%040x", 7000+i), falsch), true)
	}
	neu := fmt.Sprintf("0x%040x", 8888)
	// Gegenprobe: alle 64 frisch -> kein Platz, Grenze haelt.
	dag.replayTransactions(srBlock(100, neu, falsch), true)
	dag.stateRootMismatchesMu.Lock()
	_, drin := dag.stateRootEcht[neu]
	n := len(dag.stateRootEcht)
	// Die 64 altern: Zeitmarken vor das Fenster legen.
	alt := time.Now().Add(-2 * stateRootMismatchActiveWindow).Unix()
	for p := range dag.stateRootMismatchLastAt {
		dag.stateRootMismatchLastAt[p] = alt
	}
	for p := range dag.stateRootEchtLastAt {
		dag.stateRootEchtLastAt[p] = alt
	}
	dag.stateRootMismatchesMu.Unlock()
	if drin || n > stateRootProduzentenMax {
		t.Fatalf("Grenze bei frischen Eintraegen nicht hart: drin=%v n=%d", drin, n)
	}
	dag.replayTransactions(srBlock(101, neu, falsch), true)
	if e := dag.EchteStateRootAbweichungen(); e != 1 {
		t.Fatalf("neuer Abweichler nach gealterten Eintraegen nicht gezaehlt: echt=%d", e)
	}
	dag.stateRootMismatchesMu.Lock()
	defer dag.stateRootMismatchesMu.Unlock()
	if len(dag.stateRootEcht) > stateRootProduzentenMax || len(dag.stateRootMismatches) > stateRootProduzentenMax {
		t.Fatalf("Grenze ueberschritten: %d/%d", len(dag.stateRootEcht), len(dag.stateRootMismatches))
	}
}
