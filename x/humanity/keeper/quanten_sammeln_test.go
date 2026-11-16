package keeper

import (
	"math/big"
	"testing"
)

// Sammelweg der Quantengeld-Freischaltung (quanten_sammeln.go).

func qsFrisch(t *testing.T) {
	t.Helper()
	vorher := quantenSammel
	quantenSammel = &quantenSammelstelle{offen: map[string]*quantenSammlung{}}
	t.Cleanup(func() { quantenSammel = vorher })
}

// qsFall: n Pruefer (0 Gruender, 1..n-1 aufgenommen), Quantengeld aktiv.
func qsFall(t *testing.T, n int) (*vrFall, *APIServer, int64, quantenVorschlag) {
	t.Helper()
	f := neuerVrFall(t)
	z := qgPruefer(f, n)
	qgAktiv(t)
	qsFrisch(t)
	f.cs.annehmendAusdruecklich.Store(true)
	now := z + 1000
	v := quantenVorschlag{Satz: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: now + quantenFreischaltungVorlauf}
	return f, &APIServer{state: f.cs}, now, v
}

func (f *vrFall) qsStimme(von int, v quantenVorschlag, zeit int64) VerifierStimme {
	return qgStimme(f, von, v.Satz, v.Ordnung, v.Ab, zeit)
}

func qsAbgeben(t *testing.T, f *vrFall, a *APIServer, v quantenVorschlag, s VerifierStimme, now int64) map[string]interface{} {
	t.Helper()
	if err := quantenStimmeVorpruefen(v, &s, f.cs.verifierRegisterJetzt(), now, f.cs.prueferZusageVon); err != nil {
		t.Fatalf("Vorpruefung: %v", err)
	}
	if code, err := quantenSammel.ablegen(v, s, now); err != nil {
		t.Fatalf("ablegen: %d %v", code, err)
	}
	return a.quantenSammelEinreichen(v, now)
}

// Fuenf Pruefer: zwei Drittel waeren 4, die Mindestzahl verlangt 5. Mit
// vier Stimmen wird nichts eingereicht, mit der fuenften schon.
func TestQuantenSammeln_MindestzahlReichtEin(t *testing.T) {
	f, a, now, v := qsFall(t, 5)
	for i := 0; i < 4; i++ {
		out := qsAbgeben(t, f, a, v, f.qsStimme(i, v, now-10+int64(i)), now)
		if out["eingereicht"] != false || out["stimmen"] != i+1 || out["noetig"] != 5 {
			t.Fatalf("nach %d Stimmen: %v", i+1, out)
		}
	}
	if _, da := f.cs.quantenGeldJetzt().satz(v.Satz); da {
		t.Fatal("Satz nach vier Stimmen eingetragen")
	}
	out := qsAbgeben(t, f, a, v, f.qsStimme(4, v, now-5), now)
	if out["eingereicht"] != true {
		t.Fatalf("nach fuenf Stimmen nicht eingereicht: %v", out)
	}
	if _, da := f.cs.quantenGeldJetzt().satz(v.Satz); !da {
		t.Fatal("Satz nicht eingetragen")
	}
	if len(quantenSammel.offen) != 0 {
		t.Fatal("Sammlung nach dem Einreichen nicht geleert")
	}
}

// Missbrauch: ein einziger Pruefer (wie direkt nach dem Neustart bei null)
// sammelt seine Stimme, eingereicht wird nichts.
func TestQuantenSammeln_EinPrueferReichtNichtEin(t *testing.T) {
	f, a, now, v := qsFall(t, 1)
	out := qsAbgeben(t, f, a, v, f.qsStimme(0, v, now-10), now)
	if out["eingereicht"] != false || out["noetig"] != 5 {
		t.Fatalf("ein Pruefer: %v", out)
	}
	if _, da := f.cs.quantenGeldJetzt().satz(v.Satz); da {
		t.Fatal("ein Pruefer hat einen Satz freigeschaltet")
	}
	if a.state.QuantenStand("")["noetig_stimmen"] != 5 {
		t.Fatal("Stand nennt nicht die noetigen Stimmen")
	}
}

func TestQuantenSammeln_Missbrauch(t *testing.T) {
	f, _, now, v := qsFall(t, 5)
	reg := f.cs.verifierRegisterJetzt()
	pruefe := func(name string, v quantenVorschlag, s VerifierStimme) {
		t.Helper()
		if err := quantenStimmeVorpruefen(v, &s, reg, now, f.cs.prueferZusageVon); err == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	gut := f.qsStimme(0, v, now-10)
	if err := quantenStimmeVorpruefen(v, &gut, reg, now, f.cs.prueferZusageVon); err != nil {
		t.Fatalf("gute Stimme abgewiesen: %v", err)
	}
	// Stimme fuer einen anderen Vorschlag, umgeschrieben auf diesen.
	anderesAb := v
	anderesAb.Ab++
	pruefe("Stimme fuer anderes ab", anderesAb, f.qsStimme(0, v, now-10))
	andereOrdnung := v
	andereOrdnung.Ordnung = new(big.Int).Add(qgSatzN, big.NewInt(1)).String()
	pruefe("Stimme fuer andere Ordnung", andereOrdnung, f.qsStimme(0, v, now-10))
	// Kein Pruefer (Mensch 5 hat Schluessel, ist aber nicht aufgenommen).
	pruefe("Nicht-Pruefer", v, f.qsStimme(5, v, now-10))
	// Spielzeug und Labor.
	klein := v
	klein.Ordnung = "1491"
	pruefe("Spielzeuggruppe", klein, f.qsStimme(0, klein, now-10))
	labor := v
	labor.Satz = "labor-h1491"
	pruefe("Laborsatz", labor, f.qsStimme(0, labor, now-10))
	// Ohne Vorlauf.
	frueh := v
	frueh.Ab = now + 60
	pruefe("ohne Vorlauf", frueh, f.qsStimme(0, frueh, now-10))
	// Zeiten.
	pruefe("Stimme zu alt", v, f.qsStimme(0, v, now-verifierStimmeHoechstensAlt-1))
	pruefe("Stimme aus der Zukunft", v, f.qsStimme(0, v, now+nachweisHoechstensVoraus+1))
	// Fremder Mensch in einer echten Bescheinigung.
	fremd := f.qsStimme(0, v, now-10)
	fremd.Bescheinigung.Mensch = f.m(1)
	pruefe("fremder Mensch", v, fremd)
	// Nach dem PQ-Stichtag ohne PQ-Teil.
	prueferPQStichtag(t, now-100)
	pruefe("ohne PQ-Teil nach dem Stichtag", v, f.qsStimme(0, v, now-10))
}

func TestQuantenSammeln_Grenzen(t *testing.T) {
	f, _, now, v := qsFall(t, 3)
	vorschlag := func(i int) quantenVorschlag {
		x := v
		x.Ab = v.Ab + int64(i)
		return x
	}
	// Je Pruefer hoechstens quantenSammlungenJePruefer Vorschlaege.
	for i := 0; i < quantenSammlungenJePruefer; i++ {
		if _, err := quantenSammel.ablegen(vorschlag(i), f.qsStimme(0, vorschlag(i), now-10), now); err != nil {
			t.Fatalf("Vorschlag %d: %v", i, err)
		}
	}
	if _, err := quantenSammel.ablegen(vorschlag(9), f.qsStimme(0, vorschlag(9), now-10), now); err == nil {
		t.Fatal("ein Pruefer belegt mehr Vorschlaege als erlaubt")
	}
	// Insgesamt hoechstens quantenSammlungenHoechstens.
	for i := quantenSammlungenJePruefer; i < quantenSammlungenHoechstens; i++ {
		if _, err := quantenSammel.ablegen(vorschlag(i), f.qsStimme(1, vorschlag(i), now-10), now); err != nil {
			t.Fatalf("Vorschlag %d: %v", i, err)
		}
	}
	if _, err := quantenSammel.ablegen(vorschlag(20), f.qsStimme(2, vorschlag(20), now-10), now); err == nil {
		t.Fatal("mehr offene Vorschlaege als erlaubt")
	}
	if len(quantenSammel.offen) > quantenSammlungenHoechstens {
		t.Fatalf("%d offene Vorschlaege", len(quantenSammel.offen))
	}
	// Verfallen: alles weg.
	quantenSammel.mu.Lock()
	quantenSammel.aufraeumenLocked(now + verifierStimmeHoechstensAlt + 100)
	n := len(quantenSammel.offen)
	quantenSammel.mu.Unlock()
	if n != 0 {
		t.Fatalf("verfallene Vorschlaege bleiben: %d", n)
	}
	// Dieselbe Stimme zweimal: eine Stimme je Mensch.
	qsFrisch(t)
	quantenSammel.ablegen(v, f.qsStimme(0, v, now-10), now)
	quantenSammel.ablegen(v, f.qsStimme(0, v, now-5), now)
	if s := quantenSammel.offen[v.schluessel()]; s == nil || len(s.stimmen) != 1 || s.stimmen[f.m(0)].Zeit != now-5 {
		t.Fatal("ein Mensch zaehlt nicht einmal, oder die juengere Stimme ersetzt nicht")
	}
}

// Eine Stimme, die erst im Annahme-Tor scheitert (offene Staffel -- nur mit
// dem Kontostand pruefbar), faellt heraus; mit den uebrigen geht es weiter.
// Sechs Pruefer: ohne die schlechte Stimme fehlt eine, die sechste reicht ein.
func TestQuantenSammeln_SchlechteStimmeFaelltHeraus(t *testing.T) {
	f, a, now, v := qsFall(t, 6)
	f.cs.mu.Lock()
	acc, _ := f.cs.accounts.Get(f.m(1))
	acc.GrantStagedRest = NewDecimal(5)
	f.cs.mu.Unlock()
	for _, i := range []int{1, 0, 2, 3, 4} {
		s := f.qsStimme(i, v, now-10)
		if _, err := quantenSammel.ablegen(v, s, now); err != nil {
			t.Fatal(err)
		}
	}
	if out := a.quantenSammelEinreichen(v, now); out["eingereicht"] != false {
		t.Fatalf("mit der schlechten Stimme eingereicht: %v", out)
	}
	if s := quantenSammel.offen[v.schluessel()]; s == nil || len(s.stimmen) != 4 {
		t.Fatal("die schlechte Stimme ist nicht herausgefallen")
	}
	out := qsAbgeben(t, f, a, v, f.qsStimme(5, v, now-5), now)
	if out["eingereicht"] != true {
		t.Fatalf("mit den guten Stimmen nicht eingereicht: %v", out)
	}
}
