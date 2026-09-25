package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"testing"
)

// Stufe 1.3 (docs/SKALIERUNG_DEZENTRAL.md): der parallele Nachspielpfad nimmt
// auch Laeufe, in denen viele an DENSELBEN Empfaenger zahlen -- der Alltag
// eines Geschaefts. Vorher beendete jede wiederholte Adresse den Lauf.
//
// Diese Tests halten fest, was dafuer gelten muss:
//   - Absender bleiben eindeutig, ein Empfaenger darf im Lauf nie senden.
//   - Kontostaende, StateRoot und Buchfuehrung sind bitgleich mit dem
//     seriellen Pfad, auch wenn ein Empfaenger zwanzig Gutschriften bekommt.
//   - Die Wohlstandsgrenze wird am LAUFENDEN Stand geprueft: reisst erst die
//     dritte Gutschrift sie, laufen die ersten zwei im Buendel und die
//     dritte seriell, mit Kappung.

func TestCollectDisjointTransferBatch_SammelempfaengerBleibtImLauf(t *testing.T) {
	// Viele zahlen an denselben Laden: ein einziger Lauf.
	laden := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xladen", Amount: 1},
		{Type: "transfer", Wallet: "0xb", To: "0xladen", Amount: 1},
		{Type: "transfer", Wallet: "0xc", To: "0xladen", Amount: 1},
		{Type: "transfer", Wallet: "0xd", To: "0xe", Amount: 1},
		{Type: "transfer", Wallet: "0xf", To: "0xladen", Amount: 1},
	}
	if batch, _ := collectDisjointTransferBatch(laden, 0); len(batch) != len(laden) {
		t.Fatalf("Zahlungen an denselben Empfaenger muessen in einem Lauf bleiben, bekam %d von %d", len(batch), len(laden))
	}

	// Der Empfaenger zahlt im selben Lauf weiter: seine Deckung haengt dann
	// von den Gutschriften davor ab -- Ende des Laufs.
	weiter := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xladen", Amount: 1},
		{Type: "transfer", Wallet: "0xb", To: "0xladen", Amount: 1},
		{Type: "transfer", Wallet: "0xladen", To: "0xlieferant", Amount: 1},
	}
	if batch, _ := collectDisjointTransferBatch(weiter, 0); len(batch) != 2 {
		t.Fatalf("ein Empfaenger, der im Lauf sendet, muss ihn beenden, bekam %d", len(batch))
	}

	// Ein Absender, der danach empfaengt: ebenfalls Ende.
	zurueck := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xladen", Amount: 1},
		{Type: "transfer", Wallet: "0xb", To: "0xa", Amount: 1},
	}
	if batch, _ := collectDisjointTransferBatch(zurueck, 0); len(batch) != 1 {
		t.Fatalf("ein Absender, der im Lauf empfaengt, muss ihn beenden, bekam %d", len(batch))
	}

	// Derselbe Absender zweimal: Ende, wie bisher.
	doppelt := []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xladen", Amount: 1},
		{Type: "transfer", Wallet: "0xa", To: "0xladen", Amount: 1},
	}
	if batch, _ := collectDisjointTransferBatch(doppelt, 0); len(batch) != 1 {
		t.Fatalf("ein Absender darf im Lauf nur einmal vorkommen, bekam %d", len(batch))
	}
}

// sammelUeberweisungen: jeder Absender zahlt genau einmal an einen von
// wenigen Empfaengern. Absender und Empfaenger sind getrennte Mengen. Deckt
// Einkauf (viele Menschen an dasselbe Unternehmen), Lohn (mehrere
// Unternehmen an denselben Menschen), zwischen Unternehmen, Mensch an Mensch
// und freie Adressen ab.
func sammelUeberweisungen(rng *rand.Rand, w buchfuehrungsWelt, buchAt int64) (txs []Transaction, empfaenger []string) {
	empfaenger = append(append(append([]string(nil), w.firmen[:4]...), w.menschen[:4]...), w.frei[:2]...)
	absender := append(append([]string(nil), w.menschen[20:]...), w.firmen[4:]...)
	rng.Shuffle(len(absender), func(i, j int) { absender[i], absender[j] = absender[j], absender[i] })
	for _, von := range absender {
		an := empfaenger[rng.Intn(len(empfaenger))]
		betrag := float64(int64((10+rng.Float64()*400)*1e6)) / 1e6
		txs = append(txs, Transaction{Type: "transfer", Wallet: von, To: an, Amount: betrag, BuchAt: buchAt})
	}
	return txs, empfaenger
}

func TestParallelesNachspielen_SammelempfaengerWieSeriell(t *testing.T) {
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	for lauf := int64(0); lauf < 20; lauf++ {
		rng := rand.New(rand.NewSource(13_2026_0926 + lauf))

		// ---- parallel: der ganze Block als EIN Buendel ----
		_, csP := newDeterminismTestDAG()
		weltP := baueBuchfuehrungsWelt(t, csP)
		txs, empfaenger := sammelUeberweisungen(rng, weltP, jetzt)

		mehrfach := map[string]int{}
		for _, tx := range txs {
			mehrfach[tx.To]++
		}
		max := 0
		for _, n := range mehrfach {
			if n > max {
				max = n
			}
		}
		if max < 3 {
			t.Fatalf("lauf %d: kein Empfaenger mit mehreren Gutschriften -- der Test prueft 1.3 nicht", lauf)
		}
		if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) != len(txs) {
			t.Fatalf("lauf %d: nur %d von %d Ueberweisungen buendelbar", lauf, len(batch), len(txs))
		}
		csP.mu.Lock()
		angewandt, err := csP.applyTransferBatchParallel(context.Background(), txs, jetzt, nil)
		csP.mu.Unlock()
		if err != nil {
			t.Fatalf("lauf %d: Buendel: %v", lauf, err)
		}
		if angewandt != len(txs) {
			t.Fatalf("lauf %d: Buendel wandte %d von %d an -- der Rest liefe seriell, der Test prueft dann nicht den parallelen Pfad", lauf, angewandt, len(txs))
		}

		// ---- seriell: dieselben Ueberweisungen einzeln ----
		_, csS := newDeterminismTestDAG()
		baueBuchfuehrungsWelt(t, csS)
		for i, tx := range txs {
			ctx := mitBuchZeit(context.Background(), buchZeitBeimNachspielen(tx.BuchAt, jetzt))
			csS.mu.Lock()
			err := csS.applyTransferDeltaLockedSammelnd(ctx, tx.Wallet, tx.To, tx.Amount, 0, 0, jetzt, nil, 0)
			csS.mu.Unlock()
			if err != nil {
				t.Fatalf("lauf %d, tx %d seriell: %v", lauf, i, err)
			}
		}

		alle := append(append(append([]string(nil), weltP.menschen...), weltP.firmen...), weltP.frei...)
		sort.Strings(alle)
		for _, a := range alle {
			if p, s := stand(csP, a), stand(csS, a); p != s {
				t.Fatalf("lauf %d: Kontostand %s parallel %.6f, seriell %.6f", lauf, a, p, s)
			}
			csP.mu.RLock()
			ap, _ := csP.accounts.Get(a)
			csP.mu.RUnlock()
			csS.mu.RLock()
			as, _ := csS.accounts.Get(a)
			csS.mu.RUnlock()
			if ap.LastActivityAt != as.LastActivityAt {
				t.Fatalf("lauf %d: Demurrage-Uhr %s parallel %d, seriell %d", lauf, a, ap.LastActivityAt, as.LastActivityAt)
			}
		}
		if p, s := csP.StateRoot(), csS.StateRoot(); p != s {
			t.Fatalf("lauf %d: StateRoot weicht ab -- das waere eine Spaltung des Netzes\n  parallel: %s\n  seriell:  %s", lauf, p, s)
		}
		bp, bs := buchSchnappschuss(t, csP, alle), buchSchnappschuss(t, csS, alle)
		if len(bs) == 0 {
			t.Fatalf("lauf %d: serielle Buchfuehrung leer -- der Test prueft nichts", lauf)
		}
		for a, want := range bs {
			if got := bp[a]; got != want {
				t.Fatalf("lauf %d: Buchkonto %s weicht ab\n  parallel: %s\n  seriell:  %s", lauf, a, got, want)
			}
		}
		for a := range bp {
			if _, ok := bs[a]; !ok {
				t.Fatalf("lauf %d: Buchkonto %s nur im parallelen Lauf", lauf, a)
			}
		}
		for _, f := range empfaenger[:4] {
			if p, s := umsatzVon(csP, f), umsatzVon(csS, f); p != s {
				t.Fatalf("lauf %d: Umsatz %s weicht ab: parallel %.6f, seriell %.6f", lauf, f, p, s)
			}
		}
	}
}

// Die Grenze reisst erst mit der dritten Gutschrift an denselben Empfaenger.
// Jede einzelne liegt darunter -- eine Pruefung am Stand VOR dem Buendel (wie
// vor 1.3 ausreichend) liesse alle drei durch und der Empfaenger laege
// ungekappt ueber der Grenze.
func TestParallelesNachspielen_SammelempfaengerWohlstandsgrenzeLaufend(t *testing.T) {
	// 40 Menschen => voller Faktor 25, Durchschnitt fest 1000: Grenze 25.000.
	const menschen = 40
	seed := func(cs *ChainState) {
		seedHumanAccounts(cs, menschen, 1000)
		cs.mu.Lock()
		cs.accounts.Set("0xsammler", &AccountState{Address: "0xsammler", Balance: NewDecimal(24_000)})
		for i := 0; i < 4; i++ {
			a := fmt.Sprintf("0xzahler%d", i)
			cs.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(1_000)})
		}
		cs.mu.Unlock()
	}
	txs := []Transaction{
		{Type: "transfer", Wallet: "0xzahler0", To: "0xsammler", Amount: 400}, // 24.400
		{Type: "transfer", Wallet: "0xzahler1", To: "0xsammler", Amount: 400}, // 24.800
		{Type: "transfer", Wallet: "0xzahler2", To: "0xsammler", Amount: 400}, // 25.200 > Grenze
		{Type: "transfer", Wallet: "0xzahler3", To: "0xsammler", Amount: 400},
	}

	// Das Buendel darf nur das gesunde Praefix nehmen.
	_, csB := newDeterminismTestDAG()
	seed(csB)
	csB.mu.Lock()
	angewandt, err := csB.applyTransferBatchParallel(context.Background(), txs, testBlockActivityTs, nil)
	csB.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if angewandt != 2 {
		t.Fatalf("Buendel wandte %d an, erwartet 2 (die dritte Gutschrift reisst die Grenze)", angewandt)
	}

	// Ganzer Block ueber replayTransactions gegen eine Ueberweisung je Block.
	parDAG, parCS := newDeterminismTestDAG()
	seed(parCS)
	if ok := parDAG.replayTransactions(&Block{Height: 1, Hash: "0xsammel-par", Timestamp: testBlockActivityTs, Transactions: txs}, true); !ok {
		t.Fatal("paralleles Nachspielen lehnte einen gueltigen Block ab")
	}
	serDAG, serCS := newDeterminismTestDAG()
	seed(serCS)
	for i, tx := range txs {
		b := &Block{Height: int64(i + 1), Hash: fmt.Sprintf("0xsammel-ser-%d", i), Timestamp: testBlockActivityTs, Transactions: []Transaction{tx}}
		if ok := serDAG.replayTransactions(b, true); !ok {
			t.Fatalf("serielles Nachspielen lehnte tx %d ab", i)
		}
	}

	if got := stand(serCS, "0xsammler"); got != 25_000 {
		t.Fatalf("Voraussetzung verletzt: der serielle Pfad kappte nicht (%.6f)", got)
	}
	for _, a := range []string{"0xsammler", "0xzahler0", "0xzahler1", "0xzahler2", "0xzahler3",
		validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		if p, s := stand(parCS, a), stand(serCS, a); p != s {
			t.Errorf("%s: parallel %.6f, seriell %.6f", a, p, s)
		}
	}
	if p, s := parCS.StateRoot(), serCS.StateRoot(); p != s {
		t.Errorf("StateRoot weicht ab\n  parallel: %s\n  seriell:  %s", p, s)
	}
}

// Gegen ein echtes Postgres: mehrere Gutschriften an dasselbe Unternehmen in
// EINEM Buendel. Jedes Konto und jedes Buchkonto darf in der gebuendelten
// Schreibanweisung nur einmal vorkommen (ein INSERT ... ON CONFLICT, das
// dieselbe Zeile zweimal trifft, bricht in Postgres ab), und danach muessen
// Datenbank und Speicher uebereinstimmen.
func TestParallelesNachspielen_SammelempfaengerInDB_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL must point at a disposable local Postgres database")
	}
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	jetzt := nowUnix()

	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-replay-parallel-sammel-realdb-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection (cs.useDB == false) — check DATABASE_URL")
	}
	for _, q := range []string{`TRUNCATE wirtschaft_buch`, `TRUNCATE wirtschaft_unternehmen`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	cs.wirt().mu.Lock()
	cs.wirt().buch = map[string]*buchKonto{}
	cs.wirt().unternehmen = map[string]*unternehmenEintrag{}
	cs.wirt().mu.Unlock()

	ctx := context.Background()
	var firmen, alle []string
	cs.mu.Lock()
	for i := 0; i < 3; i++ {
		firma := distTestAddr(1100 + i)
		inhaber := distTestAddr(1200 + i)
		for _, a := range []*AccountState{
			{Address: inhaber, Balance: NewDecimal(2000), IsHuman: true},
			{Address: firma, Balance: NewDecimal(3000)},
		} {
			cs.accounts.Set(a.Address, a)
			if err := cs.saveAccountToDB(a); err != nil {
				cs.mu.Unlock()
				t.Fatalf("seed %s: %v", a.Address, err)
			}
		}
		if err := cs.applyUnternehmenEroeffnenLocked(ctx, firma, inhaber, fmt.Sprintf("Laden %d", i), "handel", jetzt); err != nil {
			cs.mu.Unlock()
			t.Fatalf("eroeffnen %s: %v", firma, err)
		}
		firmen = append(firmen, firma)
	}
	var txs []Transaction
	for i := 0; i < 12; i++ {
		mensch := distTestAddr(1300 + i)
		a := &AccountState{Address: mensch, Balance: NewDecimal(2000), IsHuman: true}
		cs.accounts.Set(mensch, a)
		if err := cs.saveAccountToDB(a); err != nil {
			cs.mu.Unlock()
			t.Fatalf("seed %s: %v", mensch, err)
		}
		txs = append(txs, Transaction{Type: "transfer", Wallet: mensch, To: firmen[i%3], Amount: 50 + float64(i), BuchAt: jetzt})
		alle = append(alle, mensch)
	}
	cs.mu.Unlock()
	alle = append(alle, firmen...)

	if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) != len(txs) {
		t.Fatalf("nur %d von %d Ueberweisungen buendelbar -- der Test prueft dann nicht den parallelen Pfad", len(batch), len(txs))
	}

	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	block := &Block{Height: 1, Hash: "replay-parallel-sammel-realdb-1", Timestamp: jetzt, Transactions: txs}
	if ok := dag.replayTransactions(block, false); !ok {
		t.Fatal("replayTransactions lehnte einen gueltigen Block ab")
	}

	for i, f := range firmen {
		want := 3000.0
		for j := i; j < 12; j += 3 {
			want += 50 + float64(j)
		}
		if got := stand(cs, f); got != want {
			t.Fatalf("%s im Speicher: %.6f, erwartet %.6f", f, got, want)
		}
	}
	speicher := buchSchnappschuss(t, cs, alle)
	if len(speicher) != len(alle) {
		t.Fatalf("Buchfuehrung im Speicher: %d von %d Konten", len(speicher), len(alle))
	}
	for _, a := range alle {
		var balance float64
		if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, a).Scan(&balance); err != nil {
			t.Fatalf("accounts %s: %v", a, err)
		}
		if got := stand(cs, a); balance != got {
			t.Fatalf("Konto %s: Datenbank %.6f, Speicher %.6f", a, balance, got)
		}
		var daten string
		if err := cs.db.QueryRow(`SELECT daten FROM wirtschaft_buch WHERE address = $1`, a).Scan(&daten); err != nil {
			t.Fatalf("wirtschaft_buch %s: %v", a, err)
		}
		var db, mem map[string]interface{}
		if err := json.Unmarshal([]byte(daten), &db); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(speicher[a]), &mem); err != nil {
			t.Fatal(err)
		}
		dbJ, _ := json.Marshal(db)
		memJ, _ := json.Marshal(mem)
		if string(dbJ) != string(memJ) {
			t.Fatalf("Buchkonto %s: Datenbank und Speicher weichen ab\n  db:       %s\n  speicher: %s", a, dbJ, memJ)
		}
	}
}
