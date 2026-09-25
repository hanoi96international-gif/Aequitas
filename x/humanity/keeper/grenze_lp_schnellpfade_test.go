package keeper

import (
	"context"
	"fmt"
	"testing"
)

// Die Vermoegensgrenze zaehlt LP-Anteile mit (enforceWealthCapLockedCtx). Die
// Schnellpfade verglichen nur den Kontostand -- ein Empfaenger mit Anteilen
// im Pool wurde seriell gekappt, parallel nicht. Siehe wuerdeKappenLocked.
func TestSchnellpfade_KappenWieSeriellMitLPAnteilen(t *testing.T) {
	// 40 Menschen: Grenze 25.000. Empfaenger: 24.000 auf dem Konto und 1 von
	// 10 Anteilen an einem Pool mit 20.000 AEQ -- also 2.000 AEQ im Pool.
	// Eine Gutschrift von 500 ergibt 24.500 auf dem Konto, 26.500 Vermoegen.
	seed := func(cs *ChainState) {
		seedHumanAccounts(cs, 40, 1000)
		cs.mu.Lock()
		cs.pool = &PoolState{ReserveAEQ: NewDecimal(20_000), ReserveTUSD: NewDecimal(20_000), TotalLPShares: NewDecimal(10)}
		cs.accounts.Set("0xanleger", &AccountState{Address: "0xanleger", Balance: NewDecimal(24_000), LPShares: NewDecimal(1)})
		for i := 0; i < 3; i++ {
			a := fmt.Sprintf("0xzahlerlp%d", i)
			cs.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(1_000)})
		}
		cs.mu.Unlock()
	}
	txs := []Transaction{
		{Type: "transfer", Wallet: "0xzahlerlp0", To: "0xanleger", Amount: 500},
		{Type: "transfer", Wallet: "0xzahlerlp1", To: "0xzahlerlp2", Amount: 10},
	}

	_, csW := newDeterminismTestDAG()
	seed(csW)
	csW.mu.RLock()
	capAmt, hasCap := csW.wealthCapAmountLocked()
	acc, _ := csW.accounts.Get("0xanleger")
	kappt := csW.wuerdeKappenLocked("0xanleger", acc, 24_500, capAmt, hasCap)
	csW.mu.RUnlock()
	if !kappt {
		t.Fatal("wuerdeKappenLocked uebersieht die LP-Anteile (24.500 + 2.000 > 25.000)")
	}

	parDAG, parCS := newDeterminismTestDAG()
	seed(parCS)
	if ok := parDAG.replayTransactions(&Block{Height: 1, Hash: "0xlp-par", Timestamp: testBlockActivityTs, Transactions: txs}, true); !ok {
		t.Fatal("paralleles Nachspielen lehnte einen gueltigen Block ab")
	}
	// Seriell heisst hier wirklich seriell: auch ein Block mit einer
	// einzigen Ueberweisung liefe ueber den Buendelpfad
	// (parallelReplayMinBatch = 1).
	_, serCS := newDeterminismTestDAG()
	seed(serCS)
	for i, tx := range txs {
		serCS.mu.Lock()
		err := serCS.applyTransferDeltaLocked(context.Background(), tx.Wallet, tx.To, tx.Amount, 0, 0, testBlockActivityTs)
		serCS.mu.Unlock()
		if err != nil {
			t.Fatalf("seriell tx %d: %v", i, err)
		}
	}
	if got := stand(serCS, "0xanleger"); got >= 24_500 {
		t.Fatalf("Voraussetzung verletzt: der serielle Pfad kappte nicht (%.6f)", got)
	}
	for _, a := range []string{"0xanleger", "0xzahlerlp0", "0xzahlerlp1", "0xzahlerlp2", ubiPoolAddr} {
		if p, s := stand(parCS, a), stand(serCS, a); p != s {
			t.Errorf("%s: parallel %.6f, seriell %.6f", a, p, s)
		}
	}
	if p, s := parCS.StateRoot(), serCS.StateRoot(); p != s {
		t.Errorf("StateRoot weicht ab\n  parallel: %s\n  seriell:  %s", p, s)
	}
}
