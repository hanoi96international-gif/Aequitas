package keeper

import (
	"context"
	"math"
	"testing"
)

// Tests der dritten Stufe "Gleichgewicht" (wirtschaft3.go,
// nachrechnen_umlauf.go): die Regel selbst, Erzeuger und Nachspielen einer
// echten Tagesrunde, und je Angriff ein Missbrauchstest (AGENTS.md, Frage 5;
// Endfassung Abschnitt 10).

func gleichgewichtAn(t *testing.T) {
	t.Helper()
	wirtschaftAn(t)
	gleichgewichtAktivOverride.Store(1)
	t.Cleanup(func() { gleichgewichtAktivOverride.Store(0) })
}

var ggRegeln = []string{
	"umlauf_ausgelassen", "umlauf_doppelt", "umlauf_reihenfolge", "umlauf_rundenzeit",
	"umlauf_konten_unlesbar", "liegegeld", "gebuehr_nach_gleichgewicht",
}

func ggZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range ggRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func ggNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range ggRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

// Vor der Aktivierung (2100) aendert sich nichts: Menschen unter dem
// Sparfreibetrag zahlen keinen Umlauf, Ueberweisungen kosten Gebuehr.
func TestGleichgewicht_SchlaeftBis2100(t *testing.T) {
	wirtschaftAn(t)
	jetzt := int64(1_800_000_000)
	cs := newTestState()
	if b := cs.umlaufBetragRoh(wMensch1, artMensch, 1000, jetzt, sekundenJeMonat); b != 0 {
		t.Fatalf("vor dem Gleichgewicht zahlt ein Mensch mit 1.000 AEQ nichts, bekommen %v", b)
	}
	if g := cs.gebuehrMitWirtschaft(wFrei, wMensch1, artFrei, artMensch, 100, 200, jetzt); g <= 0 {
		t.Fatalf("vor dem Gleichgewicht kostet eine Ueberweisung Gebuehr, bekommen %v", g)
	}
	if gleichgewichtAktiv(gleichgewichtAktivAbUnix-1) || !gleichgewichtAktiv(gleichgewichtAktivAbUnix) {
		t.Fatal("Aktivierung muss genau an gleichgewichtAktivAbUnix liegen")
	}
	if gleichgewichtAktivAbUnix != 4102444800 {
		t.Fatal("das Gleichgewicht schlaeft bis zur Entscheidung (2100)")
	}
	wirtschaftAktivOverride.Store(math.MaxInt64)
	if gleichgewichtAktiv(gleichgewichtAktivAbUnix) {
		t.Fatal("ohne die Wirtschaftsregeln kein Gleichgewicht")
	}
}

// Modell und Kette rechnen dasselbe (Endfassung, Punkt h).
func TestWirtschaftsmodell_FormelnGleichDerKette_Gleichgewicht(t *testing.T) {
	gleichgewichtAn(t)
	g := StandardGleichgewicht()
	if g.SatzMonat != gleichgewichtSatzMonat {
		t.Fatalf("Modell %v, Kette %v", g.SatzMonat, gleichgewichtSatzMonat)
	}
	jetzt := int64(1_800_000_000)
	cs := newTestState()
	for _, stand := range []float64{0, 0.5, 100, 999.999999, 1000, 4999, 5001, 25000, 250000} {
		for _, art := range []kontoart{artMensch, artUnternehmen, artFrei} {
			if got, want := g.Umlauf(stand), cs.umlaufBetragRoh("0xa1", art, stand, jetzt, sekundenJeMonat); math.Abs(got-want) > 1e-9 {
				t.Fatalf("%v bei %.6f: Modell %.6f, Kette %.6f", art, stand, got, want)
			}
		}
		if b := cs.umlaufBetragRoh("0x00", artSystem, stand, jetzt, sekundenJeMonat); b != 0 {
			t.Fatalf("Systemadresse zahlt nichts, bekommen %v", b)
		}
	}
	for _, f := range [][2]kontoart{{artMensch, artMensch}, {artMensch, artUnternehmen}, {artUnternehmen, artUnternehmen}, {artFrei, artMensch}} {
		if gb := cs.gebuehrMitWirtschaft(wMensch1, wFirmaA, f[0], f[1], 50_000, 1e6, jetzt); gb != 0 {
			t.Fatalf("%v -> %v: keine Gebuehr, bekommen %v", f[0], f[1], gb)
		}
	}
}

// Geld zwischen Kontoarten zu verschieben aendert die Summe der Abgaben
// nicht -- in der Kette, nicht nur im Modell
// (TestGleichgewicht_VerschiebenBringtNichts).
func TestGleichgewicht_VerschiebenBringtNichtsInDerKette(t *testing.T) {
	gleichgewichtAn(t)
	cs := newTestState()
	jetzt := int64(1_800_000_000)
	summe := func(mensch, firma, frei float64) float64 {
		return cs.umlaufBetragRoh(wMensch1, artMensch, mensch, jetzt, sekundenJeMonat) +
			cs.umlaufBetragRoh(wFirmaA, artUnternehmen, firma, jetzt, sekundenJeMonat) +
			cs.umlaufBetragRoh(wFrei, artFrei, frei, jetzt, sekundenJeMonat)
	}
	alles := summe(30_000, 0, 0)
	for _, v := range [][3]float64{{0, 30_000, 0}, {10_000, 19_750, 250}, {29_750, 0, 250}} {
		if s := summe(v[0], v[1], v[2]); math.Abs(s-alles) > 3e-6 {
			t.Fatalf("Aufteilung %v: %.6f AEQ, alles beim Menschen %.6f", v, s, alles)
		}
	}
}

// ggKnoten: Erzeuger und Nachspieler mit denselben Konten -- drei Menschen
// mit 1.000 AEQ (unter dem heutigen Sparfreibetrag), ein Unternehmen, eine
// freie Adresse.
func ggKnoten(t *testing.T) (erzeuger *ChainState, dag *BlockDAG, nach *ChainState) {
	t.Helper()
	gleichgewichtAn(t)
	erzeuger = newTestState()
	dag, nach = newDeterminismTestDAG()
	for _, cs := range []*ChainState{erzeuger, nach} {
		for _, a := range erhaltungMenschen {
			addHuman(cs, a, 1000)
		}
		cs.mu.Lock()
		if err := cs.applyUnternehmenEroeffnenLocked(context.Background(), wFirmaA, erhaltungMenschen[0], "Laden", "handel", nowUnix()); err != nil {
			cs.mu.Unlock()
			t.Fatal(err)
		}
		cs.mu.Unlock()
		geben(cs, wFirmaA, 3_000)
		geben(cs, wFrei, 200)
	}
	return erzeuger, dag, nach
}

// runde: eine echte Tagesrunde des Erzeugers.
func runde(t *testing.T, erzeuger *ChainState, at int64) []Transaction {
	t.Helper()
	var txs []Transaction
	erzeuger.ausgangOhneDB = func(tx Transaction) { txs = append(txs, tx) }
	if err := erzeuger.RunDailyDistributionAtomic(at); err != nil {
		t.Fatal(err)
	}
	return txs
}

func umlaufe(txs []Transaction) map[string]float64 {
	m := map[string]float64{}
	for _, tx := range txs {
		if tx.Type == "umlauf" {
			m[tx.Wallet] += tx.Amount
		}
	}
	return m
}

// Gutfall: zwei echte Runden, jede in einem Block und ueber zwei Bloecke
// verteilt -- jedes Konto belastet, beim Nachspielen keine Abweichung, auch
// im strengen Modus, und dieselben Kontostaende.
func TestGleichgewicht_EchteRundenOhneAbweichung(t *testing.T) {
	nachrechnenModus(t, true)
	for _, teilung := range []int{0, 3} {
		erzeuger, dag, nach := ggKnoten(t)
		vorher := ggZaehler()
		geprueft := umlaufRundenGeprueft.Load()
		at := nowUnix()
		for tagNr := int64(0); tagNr < 2; tagNr++ {
			rundeAt := at + tagNr*86400
			txs := runde(t, erzeuger, rundeAt)
			u := umlaufe(txs)
			for _, a := range append(append([]string{}, erhaltungMenschen...), wFirmaA, wFrei) {
				if u[a] <= 0 {
					t.Fatalf("Tag %d: %s ohne Umlauf -- jedes Konto zahlt ab dem ersten AEQ: %+v", tagNr, a, u)
				}
			}
			if want := round6(1000 * gleichgewichtSatzMonat / 30); tagNr == 0 && !fast(u[erhaltungMenschen[1]], want) {
				t.Fatalf("Mensch mit 1.000 AEQ, ein Tag: %.6f, erwartet %.6f", u[erhaltungMenschen[1]], want)
			}
			bloecke := [][]Transaction{txs}
			if teilung > 0 {
				bloecke = [][]Transaction{txs[:teilung], txs[teilung:]}
			}
			for i, b := range bloecke {
				if !dag.replayTransactions(erhaltungBlock(int(tagNr)*2+i+1, rundeAt+5, b...), true) {
					t.Fatalf("Teilung %d, Tag %d, Block %d abgelehnt", teilung, tagNr, i)
				}
			}
		}
		if neu := ggNeu(vorher); len(neu) != 0 {
			t.Fatalf("Teilung %d: ehrliche Runden melden %v", teilung, neu)
		}
		if n := umlaufRundenGeprueft.Load() - geprueft; n != 2 {
			t.Fatalf("Teilung %d: %d Runden auf Vollstaendigkeit geprueft, erwartet 2", teilung, n)
		}
		for _, a := range append(append([]string{}, erhaltungMenschen...), wFirmaA, wFrei, ubiPoolAddr) {
			if acct(nach, a).Balance != acct(erzeuger, a).Balance {
				t.Fatalf("Teilung %d: %s beim Nachspielen %v, beim Erzeuger %v", teilung, a, acct(nach, a).Balance, acct(erzeuger, a).Balance)
			}
		}
	}
}

// Missbrauch: was ein Erzeuger an einer echten Runde aendern koennte. Jeder
// Fall wird gemeldet und im strengen Modus abgelehnt.
//
// verbergen: ein folgerichtiger Betrueger. Er laesst ein Konto nicht nur aus
// dem Block weg, sondern rechnet die ganze Runde so, als haette es nichts
// (Grundeinkommen, Topf, Abschluss passen dazu). Dann bemerkt es keine andere
// Regel -- nur die Vollstaendigkeit. Ohne verbergen schlaegt im strengen
// Modus oft zuerst eine andere Regel an (der Topf passt nicht mehr); der
// Block wird so oder so abgelehnt.
func TestGleichgewicht_Missbrauch(t *testing.T) {
	m := erhaltungMenschen
	for _, f := range []struct {
		name      string
		verbergen []string
		aendern   func(txs []Transaction) []Transaction
		regel     string
	}{
		{"Freund ausgelassen", []string{m[1]}, nil, "umlauf_ausgelassen"},
		{"Unternehmen ausgelassen", []string{wFirmaA}, nil, "umlauf_ausgelassen"},
		{"gar kein Umlauf", []string{m[0], m[1], m[2], wFirmaA, wFrei}, nil, "umlauf_ausgelassen"},
		{"nur aus dem Block gestrichen", nil, func(txs []Transaction) []Transaction {
			return ohneUmlauf(txs, m[1])
		}, "umlauf_ausgelassen"},
		{"zweimal belastet", nil, func(txs []Transaction) []Transaction {
			i := umlaufIndex(txs, m[2])
			return append(txs[:i+1:i+1], txs[i:]...)
		}, "umlauf_doppelt"},
		{"hoeher angesetzt", nil, func(txs []Transaction) []Transaction {
			txs[umlaufIndex(txs, m[0])].Amount *= 3
			return txs
		}, "liegegeld"},
		{"Unternehmen hoeher angesetzt", nil, func(txs []Transaction) []Transaction {
			txs[umlaufIndex(txs, wFirmaA)].Amount *= 3
			return txs
		}, "liegegeld"},
		{"Umlauf nach dem Grundeinkommen", nil, func(txs []Transaction) []Transaction {
			i := umlaufIndex(txs, wFrei)
			u := txs[i]
			txs = append(txs[:i:i], txs[i+1:]...)
			j := len(txs) - 1 // vor der Rundenmarke
			return append(txs[:j:j], u, txs[j])
		}, "umlauf_reihenfolge"},
		{"Rundenmarke mit anderer Zeit", nil, func(txs []Transaction) []Transaction {
			txs[len(txs)-1].DistributionAt -= 30
			return txs
		}, "umlauf_rundenzeit"},
	} {
		t.Run(f.name, func(t *testing.T) {
			for _, streng := range []bool{false, true} {
				nachrechnenModus(t, streng)
				erzeuger, dag, nach := ggKnoten(t)
				for _, a := range f.verbergen {
					acct(erzeuger, a).Balance = 0
				}
				at := nowUnix()
				txs := runde(t, erzeuger, at)
				if f.aendern != nil {
					txs = f.aendern(txs)
				}
				vorher := ggZaehler()
				ok := dag.replayTransactions(erhaltungBlock(1, at+5, txs...), true)
				neu := ggNeu(vorher)
				if (!streng || f.verbergen != nil) && neu[f.regel] == 0 {
					t.Fatalf("streng=%v: %s nicht gemeldet, gemeldet %v", streng, f.regel, neu)
				}
				if streng && ok {
					t.Fatalf("streng: Block angenommen")
				}
				if streng && acct(nach, ubiPoolAddr) != nil && acct(nach, ubiPoolAddr).Balance > 0 {
					t.Fatalf("streng: abgelehnter Block hat den Topf veraendert (%v)", acct(nach, ubiPoolAddr).Balance)
				}
				if streng && acct(nach, m[0]).Balance.Float() != 1000 {
					t.Fatalf("streng: abgelehnter Block hat Konten veraendert (%v)", acct(nach, m[0]).Balance)
				}
			}
		})
	}
}

func umlaufIndex(txs []Transaction, wallet string) int {
	for i, tx := range txs {
		if tx.Type == "umlauf" && tx.Wallet == wallet {
			return i
		}
	}
	panic("kein Umlauf fuer " + wallet)
}

func ohneUmlauf(txs []Transaction, wallet string) []Transaction {
	i := umlaufIndex(txs, wallet)
	return append(txs[:i:i], txs[i+1:]...)
}

// Ein Unternehmen ohne volles Umsatzfenster auf diesem Knoten: vor dem
// Gleichgewicht uebersprang die Pruefung es (keine Buchfuehrung, kein
// Nachrechnen), danach nicht mehr (Endfassung, Punkt c).
func TestGleichgewicht_UnternehmenOhneBuchfuehrungGeprueft(t *testing.T) {
	nachrechnenModus(t, true)
	erzeuger, dag, nach := ggKnoten(t)
	at := nowUnix()
	w := nach.wirt()
	w.mitSperre(func() { w.buchSeit = at }) // Knoten kennt kein Umsatzfenster
	var voll bool
	w.mitSperre(func() { voll = w.fensterVollLocked(at) })
	if voll {
		t.Fatal("Vorbedingung: Fenster unvollstaendig")
	}
	txs := runde(t, erzeuger, at)
	txs[umlaufIndex(txs, wFirmaA)].Amount *= 2
	vorher := ggZaehler()
	if dag.replayTransactions(erhaltungBlock(1, at+5, txs...), true) {
		t.Fatal("ueberhoehter Umlauf eines Unternehmens angenommen")
	}
	if ggNeu(vorher)["liegegeld"] == 0 {
		t.Fatal("nicht nachgerechnet")
	}
}

// Missbrauch: eine Ueberweisung mit Gebuehr nach der Aktivierung -- im
// seriellen Pfad abgelehnt, und der schnelle Pfad gibt sie dorthin ab.
func TestGleichgewicht_GebuehrImBlock(t *testing.T) {
	nachrechnenModus(t, true)
	_, dag, nach := ggKnoten(t)
	at := nowUnix()
	tx := Transaction{Type: "transfer", Wallet: erhaltungMenschen[0], To: erhaltungMenschen[1], Amount: 100, Gebuehr: 0.1, BuchAt: at}
	vorher := ggZaehler()
	if dag.replayTransactions(erhaltungBlock(1, at+5, tx), true) {
		t.Fatal("Ueberweisung mit Gebuehr angenommen")
	}
	if ggNeu(vorher)["gebuehr_nach_gleichgewicht"] == 0 {
		t.Fatal("Gebuehr nicht gemeldet")
	}
	if b := acct(nach, erhaltungMenschen[0]).Balance.Float(); b != 1000 {
		t.Fatalf("abgelehnter Block hat gebucht: %v", b)
	}
	ohne := tx
	ohne.Gebuehr = 0
	lauf := []Transaction{ohne, ohne, tx, ohne}
	if b := nach.lohnGebuehrPraefix(lauf, at+5); len(b) != 2 {
		t.Fatalf("schneller Pfad: Lauf muss vor der Gebuehr enden, Laenge %d", len(b))
	}
	gleichgewichtAktivOverride.Store(math.MaxInt64)
	if b := nach.lohnGebuehrPraefix(lauf, at+5); len(b) != 4 {
		t.Fatalf("vor dem Gleichgewicht bleibt die Gebuehr eines Menschen im schnellen Pfad, Laenge %d", len(b))
	}
}

// Vor der Aktivierung meldet die Vollstaendigkeit nichts: dieselbe
// ausgelassene Runde wie oben, aber mit den heutigen Regeln.
func TestGleichgewicht_VollstaendigkeitSchlaeft(t *testing.T) {
	nachrechnenModus(t, true)
	erzeuger, dag, _ := ggKnoten(t)
	gleichgewichtAktivOverride.Store(math.MaxInt64)
	for _, a := range erhaltungMenschen {
		acct(erzeuger, a).Balance = NewDecimal(8_000) // ueber dem Sparfreibetrag
	}
	at := nowUnix()
	txs := runde(t, erzeuger, at)
	if len(umlaufe(txs)) == 0 {
		t.Fatal("Vorbedingung: heutige Regeln erzeugen Umlauf")
	}
	vorher := ggZaehler()
	dag.replayTransactions(erhaltungBlock(1, at+5, ohneUmlauf(txs, erhaltungMenschen[1])...), true)
	if n := ggNeu(vorher)["umlauf_ausgelassen"]; n != 0 {
		t.Fatalf("vor dem Gleichgewicht gemeldet: %d", n)
	}
}

// Nach einem Neustart ist der Anfang der laufenden Runde unbekannt: nichts
// melden (hoechstens zu wenig, nie zu viel). Die naechste Runde ist wieder
// beobachtet.
func TestGleichgewicht_NeustartMeldetNichts(t *testing.T) {
	nachrechnenModus(t, false)
	erzeuger, dag, nach := ggKnoten(t)
	nach.umlaufRunde.unsicher = true
	at := nowUnix()
	vorher := ggZaehler()
	dag.replayTransactions(erhaltungBlock(1, at+5, ohneUmlauf(runde(t, erzeuger, at), erhaltungMenschen[1])...), true)
	if n := ggNeu(vorher)["umlauf_ausgelassen"]; n != 0 {
		t.Fatalf("nach dem Neustart gemeldet: %d", n)
	}
	at += 86400
	dag.replayTransactions(erhaltungBlock(2, at+5, ohneUmlauf(runde(t, erzeuger, at), erhaltungMenschen[2])...), true)
	if n := ggNeu(vorher)["umlauf_ausgelassen"]; n != 1 {
		t.Fatalf("die naechste Runde ist beobachtet: %d Meldungen", n)
	}
}

// Rueckrollen: Eintraege eines zurueckgewiesenen Blocks verschwinden aus der
// Menge der Belasteten.
func TestGleichgewicht_RundeRollt(t *testing.T) {
	r := umlaufRundePruefung{phase: umlaufPhaseUmlauf, belastet: map[string]int64{"a": 1}, n: 1}
	snap := r
	r.n++
	r.belastet["b"] = r.n
	zurueck := snap.zurueck()
	if _, da := zurueck.belastet["b"]; da || len(zurueck.belastet) != 1 || zurueck.n != 1 {
		t.Fatalf("zurueckgerollt: %+v", zurueck)
	}
}

// Mit echter Datenbank: ab dem Gleichgewicht kommen die Kandidaten aus der
// Datenbank UND dem Speicher -- ein Konto, dessen erstes Geld der
// WAL-Schnellpfad noch nicht geschrieben hat, faellt nicht heraus. Vorher
// bleibt die Auswahl, wie sie war (nur Datenbank, Menschen erst ueber dem
// Sparfreibetrag).
func TestGleichgewicht_KandidatenAusDatenbankUndSpeicher_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-gleichgewicht-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	gleichgewichtAn(t)
	ctx := context.Background()
	mensch, firma, nurSpeicher, leer := distTestAddr(3101), distTestAddr(3102), distTestAddr(3103), distTestAddr(3104)
	cs.mu.Lock()
	for _, acc := range []*AccountState{
		{Address: mensch, IsHuman: true, Balance: NewDecimal(1000)},
		{Address: firma, Balance: NewDecimal(50)},
		{Address: leer, Balance: 0},
	} {
		cs.accounts.Set(acc.Address, acc)
		if err := cs.saveAccountToDB(acc); err != nil {
			cs.mu.Unlock()
			t.Fatal(err)
		}
	}
	cs.accounts.Set(nurSpeicher, &AccountState{Address: nurSpeicher, Balance: NewDecimal(30)})
	at := nowUnix()
	// Die Testdatenbank teilen sich alle Tests: ein frueherer kann einen
	// spaeteren letzten Umlauf hinterlassen haben. Ein Tag Abstand, fest.
	w := cs.wirt()
	w.mitSperre(func() { w.letzterUmlauf = at - 86400 })
	enthaelt := func(liste []string) map[string]bool {
		m := map[string]bool{}
		for _, a := range liste {
			m[a] = true
		}
		return m
	}
	gg, err := cs.umlaufKandidatenLocked(cs.dbExecCtx(ctx), at)
	if err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	gleichgewichtAktivOverride.Store(math.MaxInt64)
	heute, err := cs.umlaufKandidatenLocked(cs.dbExecCtx(ctx), at)
	gleichgewichtAktivOverride.Store(1)
	if err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	txs, err := cs.umlaufLocked(ctx, at)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if k := enthaelt(gg); !k[mensch] || !k[firma] || !k[nurSpeicher] || k[leer] {
		t.Fatalf("Gleichgewicht: Kandidaten %v", gg)
	}
	if k := enthaelt(heute); k[mensch] || !k[firma] || k[nurSpeicher] {
		t.Fatalf("heutige Auswahl veraendert: %v", heute)
	}
	belastet := umlaufe(txs)
	for _, a := range []string{mensch, firma, nurSpeicher} {
		if belastet[a] <= 0 {
			t.Fatalf("%s ohne Umlauf: %+v", a, belastet)
		}
	}
}
