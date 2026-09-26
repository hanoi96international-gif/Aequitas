package keeper

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// vorbehaltsKnoten: Pool mit Reserven, X mit AEQ und tUSD.
func vorbehaltsKnoten(t *testing.T, name string) *annahmeKnoten {
	t.Helper()
	dag, cs := newDeterminismTestDAG()
	seedHumanAccounts(cs, 40, 1000)
	cs.mu.Lock()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000), TotalLPShares: NewDecimal(1000)}
	cs.accounts.Set("0xpoolstarter", &AccountState{Address: "0xpoolstarter", LPShares: NewDecimal(1000)})
	cs.accounts.Set("0xvx", &AccountState{Address: "0xvx", Balance: NewDecimal(2_000), TUsdBalance: NewDecimal(500), LastActivityAt: nowUnix(), IsHuman: true})
	cs.accounts.Set("0xvz", &AccountState{Address: "0xvz", Balance: NewDecimal(10), LastActivityAt: nowUnix()})
	cs.mu.Unlock()
	k := &annahmeKnoten{name: name, dag: dag, cs: cs}
	cs.ausgangOhneDB = func(tx Transaction) { k.korb = append(k.korb, tx) }
	return k
}

func gleicherZustand(t *testing.T, knoten ...*annahmeKnoten) {
	t.Helper()
	konten := []string{"0xvx", "0xvz", ubiPoolAddr, validatorsPoolAddr, lpPoolAddr, treasuryPoolAddr}
	for _, a := range konten {
		for _, k := range knoten[1:] {
			if s0, s := stand(knoten[0].cs, a), stand(k.cs, a); s0 != s {
				t.Errorf("%s: %s %.6f, %s %.6f", a, knoten[0].name, s0, k.name, s)
			}
		}
	}
	for _, k := range knoten[1:] {
		x0, _ := knoten[0].cs.accounts.Get("0xvx")
		x, _ := k.cs.accounts.Get("0xvx")
		if x0.TUsdBalance != x.TUsdBalance || x0.LPShares != x.LPShares {
			t.Errorf("X tUSD/LP: %s %v/%v, %s %v/%v", knoten[0].name, x0.TUsdBalance, x0.LPShares, k.name, x.TUsdBalance, x.LPShares)
		}
		if knoten[0].cs.pool.ReserveAEQ != k.cs.pool.ReserveAEQ || knoten[0].cs.pool.ReserveTUSD != k.cs.pool.ReserveTUSD {
			t.Errorf("Pool %s %v/%v, %s %v/%v", knoten[0].name, knoten[0].cs.pool.ReserveAEQ, knoten[0].cs.pool.ReserveTUSD,
				k.name, k.cs.pool.ReserveAEQ, k.cs.pool.ReserveTUSD)
		}
		if r0, r := knoten[0].cs.StateRoot(), k.cs.StateRoot(); r0 != r {
			t.Errorf("StateRoot %s %s, %s %s", knoten[0].name, r0, k.name, r)
		}
	}
}

// A ist fuer X zustaendig, B ist Leiter (Pool). A nimmt den Vorbehalt an und
// gleichzeitig eine Ausgabe von X; B fuehrt den Tausch aus; C spielt in
// anderer Reihenfolge nach. Alle gleich, X hat seinen tUSD, der Einsatz war
// waehrenddessen nicht ausgebbar.
func TestVorbehalt_TauschInZweiSchrittenDreiKnotenGleich(t *testing.T) {
	a, b, c := vorbehaltsKnoten(t, "a"), vorbehaltsKnoten(t, "b"), vorbehaltsKnoten(t, "c")

	tmpl := Transaction{Type: "swap_aeq_tusd", Wallet: "0xvx", Amount: 1_000, TxHash: "0xtausch1"}
	id, err := a.cs.VorbehaltAtomic(tmpl, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := stand(a.cs, "0xvx"); got != 2_000-1_020 {
		t.Fatalf("X nach dem Vorbehalt %.6f, erwartet 980 (1.000 + hoechstens 2 %% Abgabe vorgemerkt)", got)
	}
	// Der vorgemerkte Einsatz ist nicht ausgebbar.
	if _, _, err := a.cs.TransferAtomic("0xvx", "0xvz", 1_500, Transaction{Type: "transfer", Wallet: "0xvx", To: "0xvz", Amount: 1_500, TxHash: "0xa2"}); err == nil {
		t.Fatal("X konnte den vorgemerkten Einsatz ausgeben")
	}
	if _, _, err := a.cs.TransferAtomic("0xvx", "0xvz", 100, Transaction{Type: "transfer", Wallet: "0xvx", To: "0xvz", Amount: 100, TxHash: "0xa3"}); err != nil {
		t.Fatal(err)
	}
	blkA := a.block(t, 1)

	if !b.dag.replayTransactions(blkA, true) {
		t.Fatal("B lehnte A's Block ab")
	}
	if n := b.cs.VorbehalteAbarbeiten(); n != 1 {
		t.Fatalf("B fuehrte %d Vorbehalte aus statt 1", n)
	}
	blkB := b.block(t, 2)
	if n := b.cs.VorbehalteAbarbeiten(); n != 0 {
		t.Fatalf("B fuehrte ein zweites Mal aus (%d)", n)
	}
	if !a.dag.replayTransactions(blkB, true) {
		t.Fatal("A lehnte B's Block ab")
	}
	for _, blk := range []*Block{blkA, blkB} {
		if !c.dag.replayTransactions(blk, true) {
			t.Fatalf("C lehnte %s ab", blk.Hash)
		}
	}
	gleicherZustand(t, a, b, c)
	x, _ := a.cs.accounts.Get("0xvx")
	if x.TUsdBalance.Float() <= 500 {
		t.Fatalf("X hat keinen tUSD erhalten: %v", x.TUsdBalance)
	}
	v, _ := a.cs.accounts.Get(vorbehaltsKonto(id))
	if v == nil || !v.Balance.IsZero() {
		t.Fatalf("Vorbehaltskonto nicht leer: %+v", v)
	}
	// Dieselbe Ausfuehrung noch einmal nachgespielt: aendert nichts.
	vorher := stand(c.cs, "0xvx")
	blkB2 := &Block{Height: 3, Hash: "0xvb-wieder", Timestamp: nowUnix(), Transactions: blkB.Transactions}
	c.dag.replayTransactions(blkB2, true)
	if got := stand(c.cs, "0xvx"); got != vorher {
		t.Fatalf("wiederholte Ausfuehrung zahlte erneut: %.6f -> %.6f", vorher, got)
	}
}

func TestVorbehalt_ScheiternErstattetVollstaendig(t *testing.T) {
	a, b := vorbehaltsKnoten(t, "a"), vorbehaltsKnoten(t, "b")
	// Mindestbetrag, den der Pool nie hergibt.
	if _, err := a.cs.VorbehaltAtomic(Transaction{Type: "swap_aeq_tusd", Wallet: "0xvx", Amount: 1_000, TxHash: "0xt2"}, 1e9); err != nil {
		t.Fatal(err)
	}
	blkA := a.block(t, 1)
	if !b.dag.replayTransactions(blkA, true) {
		t.Fatal("B lehnte ab")
	}
	b.cs.VorbehalteAbarbeiten()
	blkB := b.block(t, 2)
	var aus *Transaction
	for i := range blkB.Transactions {
		if blkB.Transactions[i].Type == "vorbehalt_ausfuehrung" {
			aus = &blkB.Transactions[i]
		}
	}
	if aus == nil || !aus.Vorbehalt.Erstattet {
		t.Fatalf("keine Erstattung: %+v", blkB.Transactions)
	}
	if !a.dag.replayTransactions(blkB, true) {
		t.Fatal("A lehnte B's Block ab")
	}
	if got := stand(a.cs, "0xvx"); got != 2_000 {
		t.Fatalf("nach der Erstattung hat X %.6f statt 2.000", got)
	}
	gleicherZustand(t, a, b)
}

func TestVorbehalt_LiquiditaetHinUndZurueck(t *testing.T) {
	a, b := vorbehaltsKnoten(t, "a"), vorbehaltsKnoten(t, "b")
	if _, err := a.cs.VorbehaltAtomic(Transaction{Type: "add_liquidity", Wallet: "0xvx", Amount: 100, AmountOut: 100, TxHash: "0xl1"}, 0); err != nil {
		t.Fatal(err)
	}
	blk1 := a.block(t, 1)
	b.dag.replayTransactions(blk1, true)
	b.cs.VorbehalteAbarbeiten()
	blk2 := b.block(t, 2)
	a.dag.replayTransactions(blk2, true)
	x, _ := a.cs.accounts.Get("0xvx")
	anteile := x.LPShares.Float()
	if anteile <= 0 {
		t.Fatalf("keine Anteile nach dem Einzahlen")
	}
	if _, err := a.cs.VorbehaltAtomic(Transaction{Type: "remove_liquidity", Wallet: "0xvx", Amount: anteile, TxHash: "0xl2"}, 0); err != nil {
		t.Fatal(err)
	}
	blk3 := a.block(t, 3)
	b.dag.replayTransactions(blk3, true)
	b.cs.VorbehalteAbarbeiten()
	blk4 := b.block(t, 4)
	a.dag.replayTransactions(blk4, true)
	x, _ = a.cs.accounts.Get("0xvx")
	if !x.LPShares.IsZero() {
		t.Fatalf("Anteile nach dem Abziehen: %v", x.LPShares)
	}
	gleicherZustand(t, a, b)
}

// Mit Pflicht (Stufe 1.0): der Vorbehalt traegt den Nachweis des inneren
// Auftrags; eine Faelschung wird abgelehnt.
func TestVorbehalt_NachweisDesInnerenAuftrags(t *testing.T) {
	k := neuerTestSchluessel(t)
	jetzt := nowUnix()
	inner := signierterTausch(t, k, 10, 0, 3, jetzt)
	vb := inner
	vb.Type = "vorbehalt"
	aeq, _, _ := vorbehaltsEinsatz("swap_aeq_tusd", 10, 0)
	vb.Vorbehalt = &VorbehaltAngaben{Art: "swap_aeq_tusd", AEQ: aeq}
	if err := pruefeAuftragsNachweis(&vb, jetzt); err != nil {
		t.Fatalf("gueltiger Vorbehalt abgelehnt: %v", err)
	}
	f := vb
	f.Wallet = strings.ToLower("0x000000000000000000000000000000000000dead")
	if err := pruefeAuftragsNachweis(&f, jetzt); err == nil {
		t.Fatal("Vorbehalt mit fremdem Auftraggeber angenommen")
	}
	l := vb
	l.Vorbehalt = &VorbehaltAngaben{Art: "add_liquidity", AEQ: aeq}
	if err := pruefeAuftragsNachweis(&l, jetzt); err == nil {
		t.Fatal("Vorbehalt mit anderer Art als unterschrieben angenommen")
	}
	_ = fmt.Sprintf
}

// Gegen Postgres: der offene Vorbehalt steht in vorbehalte_offen (dieselbe
// Transaktion wie der Vorbehalt), ueberlebt einen Neustart und verschwindet
// mit der Ausfuehrung.
func TestVorbehalt_UeberlebtNeustart_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-vorbehalt-realdb-test.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank -- DATABASE_URL pruefen")
	}
	if _, err := cs.db.Exec(`DELETE FROM vorbehalte_offen`); err != nil {
		t.Fatal(err)
	}
	x := distTestAddr(1500)
	cs.mu.Lock()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000), TotalLPShares: NewDecimal(1000)}
	acc := &AccountState{Address: x, Balance: NewDecimal(2_000), LastActivityAt: nowUnix()}
	cs.accounts.Set(x, acc)
	if err := cs.saveAccountToDB(acc); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	if err := cs.savePoolToDBCtx(context.Background()); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	cs.mu.Unlock()

	id, err := cs.VorbehaltAtomic(Transaction{Type: "swap_aeq_tusd", Wallet: x, Amount: 100, TxHash: "0xvbdb1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	cs.db.QueryRow(`SELECT COUNT(*) FROM vorbehalte_offen WHERE konto = $1`, vorbehaltsKonto(id)).Scan(&n)
	if n != 1 {
		t.Fatalf("Vorbehalt nicht in vorbehalte_offen (%d)", n)
	}
	// "Neustart": der Speicher vergisst, die Tabelle nicht.
	cs.vorbehalte = nil
	if got := cs.VorbehalteEinlesen(); got != 1 {
		t.Fatalf("%d offene Vorbehalte eingelesen statt 1", got)
	}
	if got := cs.VorbehalteAbarbeiten(); got != 1 {
		t.Fatalf("%d ausgefuehrt statt 1", got)
	}
	cs.db.QueryRow(`SELECT COUNT(*) FROM vorbehalte_offen`).Scan(&n)
	if n != 0 {
		t.Fatalf("nach der Ausfuehrung noch %d offene Vorbehalte in der Tabelle", n)
	}
	var tusd float64
	if err := cs.db.QueryRow(`SELECT tusd_balance FROM chain_accounts WHERE lower(address) = $1`, x).Scan(&tusd); err != nil || tusd <= 0 {
		t.Fatalf("tUSD nicht in der Datenbank angekommen: %v, %v", tusd, err)
	}
}
