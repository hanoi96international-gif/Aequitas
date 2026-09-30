package keeper

import (
	"sync/atomic"
	"testing"
)

func erhaltungZaehler(regel string) int64 {
	if z, ok := nachrechnenJeRegel.Load(regel); ok {
		return z.(*atomic.Int64).Load()
	}
	return 0
}

var erhaltungMenschen = []string{
	"0xa100000000000000000000000000000000000e01",
	"0xa100000000000000000000000000000000000e02",
	"0xa100000000000000000000000000000000000e03",
}

// erhaltungKnoten: ein nachspielender Knoten mit drei Menschen und einem
// Grundeinkommens-Topf.
func erhaltungKnoten(t *testing.T, topf float64) (*BlockDAG, *ChainState) {
	t.Helper()
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	for _, a := range erhaltungMenschen {
		cs.accounts.Set(a, &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)})
	}
	cs.humanCount = int64(len(erhaltungMenschen))
	cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(topf)})
	cs.mu.Unlock()
	return dag, cs
}

func erhaltungBlock(n int, zeit int64, txs ...Transaction) *Block {
	b := testBlock(n, txs...)
	b.Timestamp = zeit
	return b
}

// Gutfall: eine echte Tagesrunde des Erzeugers, beim Nachspielen in einem
// Block und ueber zwei Bloecke verteilt -- keine Abweichung.
func TestErhaltung_EchteRundeOhneAbweichung(t *testing.T) {
	erzeuger := newTestState()
	for _, a := range erhaltungMenschen {
		erzeuger.accounts.Set(a, &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)})
	}
	erzeuger.humanCount = int64(len(erhaltungMenschen))
	erzeuger.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(100.000007)})
	var txs []Transaction
	erzeuger.ausgangOhneDB = func(t Transaction) { txs = append(txs, t) }
	at := nowUnix()
	if err := erzeuger.RunDailyDistributionAtomic(at); err != nil {
		t.Fatal(err)
	}
	if len(txs) < 4 {
		t.Fatalf("Vorbedingung: erwartet Gutschriften + Abschluss + Marke, bekommen %+v", txs)
	}

	for _, teilung := range []int{len(txs), 2} {
		vorher := erhaltungZaehler("topf_ueberzogen") + erhaltungZaehler("topf_rest") + erhaltungZaehler("runde_zu_frueh")
		dag, cs := erhaltungKnoten(t, 100.000007)
		if !dag.replayTransactions(erhaltungBlock(1, at+5, txs[:teilung]...), true) {
			t.Fatal("erster Block abgelehnt")
		}
		if teilung < len(txs) && !dag.replayTransactions(erhaltungBlock(2, at+6, txs[teilung:]...), true) {
			t.Fatal("zweiter Block abgelehnt")
		}
		nachher := erhaltungZaehler("topf_ueberzogen") + erhaltungZaehler("topf_rest") + erhaltungZaehler("runde_zu_frueh")
		if nachher != vorher {
			t.Errorf("Teilung nach %d Transaktionen: ehrliche Runde meldet %d Abweichung(en)", teilung, nachher-vorher)
		}
		if got := acct(cs, ubiPoolAddr).Balance; got != acct(erzeuger, ubiPoolAddr).Balance {
			t.Errorf("Topf beim Nachspielen %v, beim Erzeuger %v", got, acct(erzeuger, ubiPoolAddr).Balance)
		}
	}
}

// Missbrauch: ein Produzent schuettet aus einem Topf von 10 AEQ 100.000 AEQ
// an eine Wallet aus -- der Weg, mit dem das Gutachten die Geldmenge von 10
// auf 101.909 AEQ hob.
func TestErhaltung_UeberzogenerTopfWirdErkannt(t *testing.T) {
	dag, _ := erhaltungKnoten(t, 10)
	vorher := erhaltungZaehler("topf_ueberzogen")
	at := nowUnix()
	dag.replayTransactions(erhaltungBlock(1, at,
		Transaction{Type: "ubi_distribution", Wallet: erhaltungMenschen[0], Amount: 100000},
		Transaction{Type: "ubi_distribution_finalize", DistributionAt: at, Amount: 0},
	), true)
	if erhaltungZaehler("topf_ueberzogen") != vorher+1 {
		t.Fatal("Auszahlung ueber dem Topf nicht erkannt")
	}
}

// Missbrauch: der gemeldete Endstand ist hoeher als Topf minus Auszahlung --
// der Topf fuellte sich aus dem Nichts wieder auf.
func TestErhaltung_ZuHoherEndstandWirdErkannt(t *testing.T) {
	dag, _ := erhaltungKnoten(t, 90)
	vorher := erhaltungZaehler("topf_rest")
	at := nowUnix()
	var txs []Transaction
	for _, a := range erhaltungMenschen {
		txs = append(txs, Transaction{Type: "ubi_distribution", Wallet: a, Amount: 30})
	}
	txs = append(txs, Transaction{Type: "ubi_distribution_finalize", DistributionAt: at, Amount: 90})
	dag.replayTransactions(erhaltungBlock(1, at, txs...), true)
	if erhaltungZaehler("topf_rest") != vorher+1 {
		t.Fatal("Endstand 90 nach Auszahlung von 90 aus 90 nicht erkannt")
	}
}

// Missbrauch: Validatoren- und LP-Topf ueberzogen.
func TestErhaltung_ValidatorenUndLPUeberzogen(t *testing.T) {
	dag, cs := erhaltungKnoten(t, 0)
	cs.mu.Lock()
	cs.accounts.Set(validatorsPoolAddr, &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(5)})
	cs.accounts.Set(lpPoolAddr, &AccountState{Address: lpPoolAddr, Balance: NewDecimal(5)})
	cs.mu.Unlock()
	vorher := erhaltungZaehler("topf_ueberzogen")
	dag.replayTransactions(erhaltungBlock(1, nowUnix(),
		Transaction{Type: "validator_distribution", Wallet: erhaltungMenschen[0], Amount: 500},
		Transaction{Type: "validator_distribution_pool_zero"},
		Transaction{Type: "lp_distribution", Wallet: erhaltungMenschen[1], Amount: 500},
		Transaction{Type: "lp_distribution_pool_zero"},
	), true)
	if got := erhaltungZaehler("topf_ueberzogen") - vorher; got != 2 {
		t.Fatalf("erwartet 2 Ueberziehungen (Validatoren, LP), gezaehlt %d", got)
	}
}

// Missbrauch: Grundeinkommen ohne Abschluss, verteilt auf mehrere Bloecke --
// spaetestens die Rundenmarke prueft die offene Summe.
func TestErhaltung_UBIOhneAbschlussBeiMarkeErkannt(t *testing.T) {
	dag, _ := erhaltungKnoten(t, 90)
	vorher := erhaltungZaehler("topf_ueberzogen")
	for i := 1; i <= 4; i++ {
		dag.replayTransactions(erhaltungBlock(i, nowUnix(),
			Transaction{Type: "ubi_distribution", Wallet: erhaltungMenschen[0], Amount: 30},
		), true)
	}
	dag.replayTransactions(erhaltungBlock(5, nowUnix(),
		Transaction{Type: "distribution_round_marker", DistributionAt: nowUnix()},
	), true)
	if got := erhaltungZaehler("topf_ueberzogen") - vorher; got != 1 {
		t.Fatalf("erwartet eine Ueberziehung bei der Marke (120 aus 90), gezaehlt %d", got)
	}
}

// Missbrauch: Validatoren-Gutschriften ohne Abschluss ueber mehrere Bloecke
// -- jede einzelne passt, zusammen nicht; die laufende Summe faellt auf.
func TestErhaltung_ValidatorenOhneAbschlussUeberBloeckeErkannt(t *testing.T) {
	dag, cs := erhaltungKnoten(t, 0)
	cs.mu.Lock()
	cs.accounts.Set(validatorsPoolAddr, &AccountState{Address: validatorsPoolAddr, Balance: NewDecimal(90)})
	cs.mu.Unlock()
	vorher := erhaltungZaehler("topf_ueberzogen")
	for i := 1; i <= 4; i++ {
		dag.replayTransactions(erhaltungBlock(i, nowUnix(),
			Transaction{Type: "validator_distribution", Wallet: erhaltungMenschen[0], Amount: 30},
		), true)
	}
	if got := erhaltungZaehler("topf_ueberzogen") - vorher; got != 1 {
		t.Fatalf("erwartet genau eine Ueberziehung (beim vierten Block), gezaehlt %d", got)
	}
}

// Gutfall mit Demurrage (Sicherheitspruefung #239, Befund 1): der Erzeuger
// rechnet die Demurrage aller Menschen vorab in den Topf, beim Nachspielen
// kommt sie Gutschrift fuer Gutschrift. Keine Abweichung, gleicher Endstand.
func TestErhaltung_EchteRundeMitDemurrageOhneAbweichung(t *testing.T) {
	ruhig := nowUnix() - 400*86400
	anlegen := func(cs *ChainState) {
		for i, a := range erhaltungMenschen {
			acc := &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100)}
			if i == len(erhaltungMenschen)-1 {
				acc.Balance, acc.LastActivityAt = NewDecimal(5000), ruhig
			}
			cs.accounts.Set(a, acc)
		}
		cs.humanCount = int64(len(erhaltungMenschen))
		cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(1)})
	}
	erzeuger := newTestState()
	anlegen(erzeuger)
	var txs []Transaction
	erzeuger.ausgangOhneDB = func(t Transaction) { txs = append(txs, t) }
	at := nowUnix()
	if err := erzeuger.RunDailyDistributionAtomic(at); err != nil {
		t.Fatal(err)
	}
	var mitDemurrage bool
	for _, tx := range txs {
		mitDemurrage = mitDemurrage || tx.FromDemurrageLost > 0
	}
	if !mitDemurrage {
		t.Fatalf("Vorbedingung: die Runde muss Demurrage tragen, bekommen %+v", txs)
	}
	regeln := func() int64 {
		return erhaltungZaehler("topf_ueberzogen") + erhaltungZaehler("topf_rest") + erhaltungZaehler("demurrage_ueber_guthaben")
	}
	for _, teilung := range []int{len(txs), 1, 2} {
		vorher := regeln()
		dag, cs := nachspielKnoten(t, nil)
		cs.mu.Lock()
		anlegen(cs)
		cs.mu.Unlock()
		if !dag.replayTransactions(erhaltungBlock(1, at+5, txs[:teilung]...), true) {
			t.Fatal("erster Block abgelehnt")
		}
		if teilung < len(txs) && !dag.replayTransactions(erhaltungBlock(2, at+6, txs[teilung:]...), true) {
			t.Fatal("zweiter Block abgelehnt")
		}
		if got := regeln() - vorher; got != 0 {
			t.Errorf("Teilung nach %d: ehrliche Runde mit Demurrage meldet %d Abweichung(en)", teilung, got)
		}
		if acct(cs, ubiPoolAddr).Balance != acct(erzeuger, ubiPoolAddr).Balance {
			t.Errorf("Teilung nach %d: Topf %v, beim Erzeuger %v", teilung, acct(cs, ubiPoolAddr).Balance, acct(erzeuger, ubiPoolAddr).Balance)
		}
	}
}

// Missbrauch (Befund 2): FromDemurrageLost weit ueber dem Guthaben pumpt den
// Topf auf, aus dem danach ausgezahlt wird -- das Opfer steht im Minus.
func TestErhaltung_DemurrageUeberGuthabenErkannt(t *testing.T) {
	dag, _ := erhaltungKnoten(t, 1)
	vorher := erhaltungZaehler("demurrage_ueber_guthaben")
	dag.replayTransactions(erhaltungBlock(1, nowUnix(),
		Transaction{Type: "ubi_distribution", Wallet: erhaltungMenschen[0], Amount: 0.000001, FromDemurrageLost: 100000},
		Transaction{Type: "ubi_distribution", Wallet: erhaltungMenschen[1], Amount: 100000},
		Transaction{Type: "ubi_distribution_finalize", DistributionAt: nowUnix(), Amount: 0},
	), true)
	if erhaltungZaehler("demurrage_ueber_guthaben") != vorher+1 {
		t.Fatal("Demurrage von 100.000 AEQ auf ein Konto mit 100 nicht erkannt")
	}
}

// Ein zurueckgewiesener Block darf die laufende Summe nicht veraendern --
// sonst meldete die naechste ehrliche Runde eine falsche Ueberziehung.
func TestErhaltung_RueckrollenSetztSummeZurueck(t *testing.T) {
	dag, cs := erhaltungKnoten(t, 90)
	vorher := cs.erhaltung
	ok := dag.replayTransactions(erhaltungBlock(1, nowUnix(),
		Transaction{Type: "ubi_distribution", Wallet: erhaltungMenschen[0], Amount: 30},
		Transaction{Type: "transfer", Wallet: erhaltungMenschen[1], To: erhaltungMenschen[2], Amount: -1},
	), true)
	if ok {
		t.Fatal("Vorbedingung: der Block muss abgelehnt werden")
	}
	if cs.erhaltung != vorher {
		t.Errorf("Summe nach zurueckgewiesenem Block veraendert: %+v statt %+v", cs.erhaltung, vorher)
	}
}
