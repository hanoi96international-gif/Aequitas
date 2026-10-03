package keeper

import (
	"fmt"
	"math"
	"testing"
)

var freigabeRegeln = []string{"freigabe_zu_hoch", "freigabe_ohne_lebenszeichen", "freigabe_doppelt"}

func freigabeZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range freigabeRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func freigabeNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range freigabeRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

// freigabeAnlegen: drei Menschen mit offenem Staffel-Rest von 800 AEQ; die
// ersten beiden haben ihr Lebenszeichen erneuert.
func freigabeAnlegen(cs *ChainState) {
	for i, a := range erhaltungMenschen {
		acc := &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(200), GrantStagedRest: NewDecimal(800)}
		if i < 2 {
			acc.LivenessRenewedAt = 1
		}
		cs.accounts.Set(a, acc)
	}
	cs.humanCount = int64(len(erhaltungMenschen))
}

func freigabeAktiv(t *testing.T) {
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
}

// Gutfall: die echten Freigaben des Erzeugers, zwei Tage hintereinander.
func TestNachrechnenFreigabe_EchteRundenOhneAbweichung(t *testing.T) {
	freigabeAktiv(t)
	erzeuger := newTestState()
	freigabeAnlegen(erzeuger)
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	freigabeAnlegen(cs)
	cs.mu.Unlock()
	vorher := freigabeZaehler()
	at := nowUnix()
	for tag := int64(0); tag < 2; tag++ {
		var txs []Transaction
		erzeuger.ausgangOhneDB = func(t Transaction) { txs = append(txs, t) }
		if err := erzeuger.RunDailyDistributionAtomic(at + tag*86400); err != nil {
			t.Fatal(err)
		}
		var n int
		for _, tx := range txs {
			if tx.Type == "grant_release" {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("Vorbedingung Tag %d: zwei Freigaben erwartet, bekommen %+v", tag, txs)
		}
		if !dag.replayTransactions(erhaltungBlock(int(tag)+1, at+tag*86400+5, txs...), true) {
			t.Fatalf("Tag %d abgelehnt", tag)
		}
	}
	if neu := freigabeNeu(vorher); len(neu) != 0 {
		t.Fatalf("echte Freigaben melden %v", neu)
	}
	if acct(cs, erhaltungMenschen[0]).GrantStagedRest != acct(erzeuger, erhaltungMenschen[0]).GrantStagedRest {
		t.Fatal("Rest beim Nachspielen weicht vom Erzeuger ab")
	}
}

func freigabe(a string, betrag float64) Transaction {
	return Transaction{Type: "grant_release", Wallet: a, Amount: betrag}
}

func TestNachrechnenFreigabe_Angriffe(t *testing.T) {
	m := erhaltungMenschen
	rate := grantStaffelTagesrate()
	for _, f := range []struct {
		name  string
		txs   []Transaction
		regel string
	}{
		{"ehrlich", []Transaction{freigabe(m[0], rate), freigabe(m[1], rate)}, ""},
		{"alles auf einmal", []Transaction{freigabe(m[0], 800)}, "freigabe_zu_hoch"},
		{"ohne Lebenszeichen", []Transaction{freigabe(m[2], rate)}, "freigabe_ohne_lebenszeichen"},
		{"zweimal in einer Runde", []Transaction{freigabe(m[0], rate), freigabe(m[0], rate)}, "freigabe_doppelt"},
	} {
		t.Run(f.name, func(t *testing.T) {
			freigabeAktiv(t)
			dag, cs := nachspielKnoten(t, nil)
			cs.mu.Lock()
			freigabeAnlegen(cs)
			cs.mu.Unlock()
			vorher := freigabeZaehler()
			dag.replayTransactions(erhaltungBlock(1, nowUnix(), f.txs...), true)
			neu := freigabeNeu(vorher)
			if f.regel == "" && len(neu) != 0 {
				t.Fatalf("ehrlich meldet %v", neu)
			}
			if f.regel != "" && neu[f.regel] == 0 {
				t.Fatalf("%s nicht erkannt, gemeldet %v", f.regel, neu)
			}
		})
	}
}

// Missbrauch ueber Bloecke: dieselbe Freigabe Block fuer Block, ohne
// Rundenmarke dazwischen -- ab der zweiten gemeldet. Nach einer Marke ist die
// naechste Freigabe wieder erlaubt.
func TestNachrechnenFreigabe_UeberBloeckeUndMarke(t *testing.T) {
	freigabeAktiv(t)
	rate := grantStaffelTagesrate()
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	freigabeAnlegen(cs)
	cs.mu.Unlock()
	vorher := freigabeZaehler()
	at := nowUnix()
	for i := 1; i <= 3; i++ {
		dag.replayTransactions(erhaltungBlock(i, at, freigabe(erhaltungMenschen[0], rate)), true)
	}
	if got := freigabeNeu(vorher)["freigabe_doppelt"]; got != 2 {
		t.Fatalf("zwei Doppelte erwartet, gemeldet %d", got)
	}
	dag.replayTransactions(erhaltungBlock(4, at, Transaction{Type: "distribution_round_marker", DistributionAt: at}), true)
	dag.replayTransactions(erhaltungBlock(5, at+86400, freigabe(erhaltungMenschen[0], rate)), true)
	if got := freigabeNeu(vorher)["freigabe_doppelt"]; got != 2 {
		t.Fatalf("nach der Marke darf die naechste Freigabe nicht als doppelt gelten (gemeldet %d)", got)
	}
}

// Ein abgelehnter Block laesst seine Freigaben nicht in der Runde stehen.
func TestNachrechnenFreigabe_RueckrollenEntferntFreigabe(t *testing.T) {
	freigabeAktiv(t)
	rate := grantStaffelTagesrate()
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	freigabeAnlegen(cs)
	cs.mu.Unlock()
	vorher := freigabeZaehler()
	at := nowUnix()
	// Erst eine angewandte Freigabe, damit die Menge schon besteht und das
	// Zurueckrollen sie beschneiden muss (statt sie nur zu ersetzen).
	if !dag.replayTransactions(erhaltungBlock(9, at, freigabe(erhaltungMenschen[1], rate)), true) {
		t.Fatal("erster Block abgelehnt")
	}
	if dag.replayTransactions(erhaltungBlock(1, at, freigabe(erhaltungMenschen[0], rate),
		Transaction{Type: "ubi_distribution", Amount: 1}), true) {
		t.Fatal("Vorbedingung: der Block muss abgelehnt werden")
	}
	if !dag.replayTransactions(erhaltungBlock(2, at, freigabe(erhaltungMenschen[0], rate)), true) {
		t.Fatal("ehrlicher Block abgelehnt")
	}
	if neu := freigabeNeu(vorher); len(neu) != 0 {
		t.Fatalf("Freigabe nach Zurueckrollen meldet %v", neu)
	}
}

// Missbrauch: eine zweite Runde weniger als 24 Stunden nach der ersten wird
// beim Nachspielen als doppelt erkannt und samt ihren Gutschriften
// uebersprungen (distributionRoundToSkip). Die Staffel-Freigabe darin muss
// mit uebersprungen werden -- sonst gibt eine doppelte Runde den Rest
// schneller frei als die Tagesrate.
//
// Braucht eine echte Datenbank: die Doppelrunden-Erkennung liest den
// Zeitpunkt der letzten Runde aus chain_config. Ohne Datenbank erkennt sie
// nie eine Runde als doppelt, und der Test waere gruen, ohne etwas zu pruefen.
func TestFreigabe_DoppelteRundeGibtNichtFrei_RealDB(t *testing.T) {
	truncateDistTestTables(t) // auch das Opt-in-Tor
	cs := testKnoten(t, "unused-freigabe-doppelrunde-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	freigabeAktiv(t)
	rate := grantStaffelTagesrate()
	mensch := distTestAddr(901)
	cs.mu.Lock()
	acc := &AccountState{Address: mensch, IsHuman: true, Balance: NewDecimal(200),
		GrantStagedRest: NewDecimal(800), LivenessRenewedAt: 1}
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("Mensch anlegen: %v", err)
	}
	cs.accounts.Set(mensch, acc)
	cs.mu.Unlock()

	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	at := nowUnix()
	runde := func(n int, zeit int64) {
		t.Helper()
		b := &Block{Height: int64(n), Hash: fmt.Sprintf("freigabe-doppelrunde-%d", n), Timestamp: zeit,
			Transactions: []Transaction{
				{Type: "grant_release", Wallet: mensch, Amount: rate},
				{Type: "distribution_round_marker", DistributionAt: zeit},
			}}
		if !dag.replayTransactions(b, true) {
			t.Fatalf("Runde %d abgelehnt", n)
		}
	}
	rest := func() Decimal {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		a, _ := cs.accounts.Get(mensch)
		return a.GrantStagedRest
	}

	runde(1, at)
	nachErster := rest()
	if nachErster != NewDecimal(800-rate) {
		t.Fatalf("Vorbedingung: nach der ersten Runde Rest %v, erwartet %v", nachErster, NewDecimal(800-rate))
	}
	runde(2, at+3600) // eine Stunde spaeter: doppelte Runde
	if got := rest(); got != nachErster {
		t.Fatalf("doppelte Runde hat freigegeben: Rest %v, erwartet unveraendert %v", got, nachErster)
	}
	runde(3, at+86400) // naechster Tag: wieder eine echte Runde
	if got := rest(); got != NewDecimal(800-2*rate) {
		t.Fatalf("Runde am naechsten Tag: Rest %v, erwartet %v", got, NewDecimal(800-2*rate))
	}
}

// Dasselbe fuer das Liegegeld (umlauf): eine doppelte Runde darf es nicht
// ein zweites Mal einziehen.
func TestUmlauf_DoppelteRundeZiehtNichtZweimalEin_RealDB(t *testing.T) {
	truncateDistTestTables(t) // auch das Opt-in-Tor
	cs := testKnoten(t, "unused-umlauf-doppelrunde-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
	konto := distTestAddr(902)
	cs.mu.Lock()
	acc := &AccountState{Address: konto, IsHuman: true, Balance: NewDecimal(10000)}
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatalf("Konto anlegen: %v", err)
	}
	cs.accounts.Set(konto, acc)
	cs.mu.Unlock()

	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	at := nowUnix()
	runde := func(n int, zeit int64) {
		t.Helper()
		b := &Block{Height: int64(n), Hash: fmt.Sprintf("umlauf-doppelrunde-%d", n), Timestamp: zeit,
			Transactions: []Transaction{
				{Type: "umlauf", Wallet: konto, Amount: 10, DistributionAt: zeit},
				{Type: "distribution_round_marker", DistributionAt: zeit},
			}}
		if !dag.replayTransactions(b, true) {
			t.Fatalf("Runde %d abgelehnt", n)
		}
	}
	guthaben := func() Decimal {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		a, _ := cs.accounts.Get(konto)
		return a.Balance
	}

	runde(1, at)
	if got := guthaben(); got != NewDecimal(9990) {
		t.Fatalf("Vorbedingung: nach der ersten Runde %v, erwartet 9990", got)
	}
	runde(2, at+3600)
	if got := guthaben(); got != NewDecimal(9990) {
		t.Fatalf("doppelte Runde hat ein zweites Mal eingezogen: %v, erwartet 9990", got)
	}
}
