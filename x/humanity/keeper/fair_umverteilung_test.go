package keeper

import (
	"fmt"
	"strings"
	"testing"
)

func topfStand(cs *ChainState) (v, l, u, t float64) {
	get := func(a string) float64 {
		if acc, ok := cs.accounts.Get(a); ok {
			return acc.Balance.Float()
		}
		return 0
	}
	return get(validatorsPoolAddr), get(lpPoolAddr), get(ubiPoolAddr), get(treasuryPoolAddr)
}

// Was einem Hortenden durch Demurrage genommen wird, gehoert allen Menschen
// zu gleichen Teilen: ganz ins Grundeinkommen, nichts an Validatoren,
// Kapitalgeber oder Schatzkammer.
func TestUmverteilung_DemurrageGanzInsGrundeinkommen(t *testing.T) {
	cs := newTestState()
	cs.accounts.Set("0xidle", &AccountState{Address: "0xidle", Balance: NewDecimal(9000), IsHuman: true, LastActivityAt: nowUnix() - 400*24*3600})
	cs.humanCount = 1
	cs.pool = &PoolState{}
	cs.mu.Lock()
	acc, _ := cs.accounts.Get("0xidle")
	lost, err := cs.settleDemurrageLockedCtx(t.Context(), acc)
	cs.mu.Unlock()
	if err != nil || lost.Float() <= 0 {
		t.Fatalf("keine Demurrage: %v %v", lost, err)
	}
	v, l, u, tr := topfStand(cs)
	if v != 0 || l != 0 || tr != 0 || u != lost.Float() {
		t.Fatalf("Demurrage %.6f: Validatoren %.6f, LP %.6f, Grundeinkommen %.6f, Schatzkammer %.6f -- erwartet alles im Grundeinkommen", lost.Float(), v, l, u, tr)
	}
	// Dasselbe beim Nachspielen auf einem anderen Knoten.
	cs2 := newTestState()
	cs2.accounts.Set("0xidle", &AccountState{Address: "0xidle", Balance: NewDecimal(9000), IsHuman: true})
	cs2.pool = &PoolState{}
	cs2.mu.Lock()
	acc2, _ := cs2.accounts.Get("0xidle")
	err = cs2.applyDemurrageLossLockedCtx(t.Context(), acc2, lost.Float())
	cs2.mu.Unlock()
	if v2, l2, u2, t2 := topfStand(cs2); err != nil || v2 != 0 || l2 != 0 || t2 != 0 || u2 != lost.Float() {
		t.Fatalf("Nachspielen: %v %v %v %v %v", err, v2, l2, u2, t2)
	}
}

// Ebenso der Ueberschuss ueber der Vermoegensgrenze.
func TestUmverteilung_VermoegensgrenzeGanzInsGrundeinkommen(t *testing.T) {
	cs := newTestState()
	for i := 0; i < 30; i++ {
		addr := fmt.Sprintf("0xh%02d", i)
		cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true})
		cs.humanCount++
	}
	cs.accounts.Set("0xwhale", &AccountState{Address: "0xwhale", Balance: NewDecimal(90000), IsHuman: true})
	cs.humanCount++
	cs.pool = &PoolState{}
	cs.mu.Lock()
	acc, _ := cs.accounts.Get("0xwhale")
	err := cs.enforceWealthCapLockedCtx(t.Context(), acc)
	nachher := acc.Balance.Float()
	cs.mu.Unlock()
	if err != nil || nachher >= 90000 {
		t.Fatalf("Vermoegensgrenze griff nicht: %v %v", nachher, err)
	}
	ueberschuss := 90000 - nachher
	v, l, u, tr := topfStand(cs)
	if v != 0 || l != 0 || tr != 0 || u != ueberschuss {
		t.Fatalf("Ueberschuss %.6f: Validatoren %.6f, LP %.6f, Grundeinkommen %.6f, Schatzkammer %.6f", ueberschuss, v, l, u, tr)
	}
}

// Die Swap-Gebuehr bleibt eine Bezahlung fuer einen Dienst: 40 % Validatoren,
// 30 % Liquiditaetsgeber, 30 % Grundeinkommen, nichts in die Schatzkammer, aus
// der niemand auszahlt.
func TestUmverteilung_SwapGebuehr(t *testing.T) {
	cs := newTestState()
	cs.pool = &PoolState{}
	cs.mu.Lock()
	err := cs.distributeSwapFeeCtx(t.Context(), 10, true)
	cs.mu.Unlock()
	if v, l, u, tr := topfStand(cs); err != nil || v != 4 || l != 3 || u != 3 || tr != 0 {
		t.Fatalf("Swap-Gebuehr: %v %v %v %v %v", err, v, l, u, tr)
	}
}

// Aus einem Topf sendet niemand: kein Schluessel -- auch nicht der, den es
// fuer die Topf-Adressen gibt -- kann das Grundeinkommen aller Menschen
// ueberweisen, tauschen oder in Liquiditaet stecken.
func TestTopfSendetNie(t *testing.T) {
	for _, topf := range []string{validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr} {
		cs := newTestState()
		cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(1000), TotalLPShares: NewDecimal(10)}
		cs.accounts.Set(topf, &AccountState{Address: topf, Balance: NewDecimal(500), TUsdBalance: NewDecimal(500), LPShares: NewDecimal(5)})
		dieb := "0x00000000000000000000000000000000000000dd"
		versuche := map[string]error{}
		_, _, versuche["TransferAtomic"] = cs.TransferAtomic(topf, dieb, 100, Transaction{Type: "transfer", Wallet: topf, To: dieb})
		_, _, _, versuche["TransferWithV7FeeAtomic"] = cs.TransferWithV7FeeAtomic(topf, dieb, 100, Transaction{Type: "transfer", Wallet: topf, To: dieb})
		_, _, versuche["SwapAtomic"] = cs.SwapAtomic(topf, 100, true, 0, Transaction{Type: "swap", Wallet: topf})
		_, versuche["AddLiquidityAtomic"] = cs.AddLiquidityAtomic(topf, 10, 10, Transaction{Type: "add_liquidity", Wallet: topf})
		_, _, _, versuche["RemoveLiquidityAtomic"] = cs.RemoveLiquidityAtomic(topf, 1, Transaction{Type: "remove_liquidity", Wallet: topf})
		versuche["ClaimTUsdFaucetAtomic"] = cs.ClaimTUsdFaucetAtomic(topf, Transaction{Type: "faucet", Wallet: topf})
		for weg, err := range versuche {
			if err != ErrProtokollTopf {
				t.Errorf("%s aus Topf %s: %v -- erwartet ErrProtokollTopf", weg, topf, err)
			}
		}
		if acc, _ := cs.accounts.Get(topf); acc.Balance.Float() != 500 {
			t.Errorf("Topf %s hat %v statt 500", topf, acc.Balance.Float())
		}
		// Grossschreibung hilft nicht.
		if pruefeAbsenderKeinTopf(strings.ToUpper(topf[2:])) == nil && pruefeAbsenderKeinTopf("0x"+strings.ToUpper(topf[2:])) == nil {
			t.Errorf("Topf %s in Grossbuchstaben kommt durch", topf)
		}
	}
	if pruefeAbsenderKeinTopf("0x00000000000000000000000000000000000000dd") != nil {
		t.Fatal("ein gewoehnlicher Absender wird abgelehnt")
	}
}

// Greift beim Auszahlen des Grundeinkommens die Vermoegensgrenze, fliesst der
// Ueberschuss zurueck in den UBI-Topf -- und darf dort nicht vernichtet
// werden (bis zum 24.09.2026 setzte der Abschluss den Topf auf null).
// Nachspielen auf einem zweiten Knoten ergibt denselben Topfstand.
func TestUBIRunde_UeberschussBleibtErhalten(t *testing.T) {
	baue := func() *ChainState {
		cs := newTestState()
		for i := 0; i < 30; i++ {
			addr := fmt.Sprintf("0xh%02d", i)
			cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(1000), IsHuman: true, LastActivityAt: nowUnix()})
			cs.humanCount++
		}
		cs.accounts.Set("0xwhale", &AccountState{Address: "0xwhale", Balance: NewDecimal(90000), IsHuman: true, LastActivityAt: nowUnix()})
		cs.humanCount++
		cs.accounts.Set(ubiPoolAddr, &AccountState{Address: ubiPoolAddr, Balance: NewDecimal(3100)})
		cs.pool = &PoolState{}
		return cs
	}
	cs := baue()
	assertConserved(t, cs, "UBI-Runde mit Vermoegensgrenze", func() {
		if err := cs.RunDailyDistributionAtomic(123456789); err != nil {
			t.Fatal(err)
		}
	})
	rest := acct(cs, ubiPoolAddr).Balance.Float()
	if rest <= 0 {
		t.Fatalf("Topf %v -- der Ueberschuss des Wals ist verschwunden", rest)
	}
	// Abschluss-Transaktion traegt genau diesen Stand; ein anderer Knoten, der
	// nachspielt, setzt ihn.
	cs2 := baue()
	cs2.mu.Lock()
	err := cs2.applyUBIFinalizeDeltaLocked(t.Context(), 123456789, rest)
	cs2.mu.Unlock()
	if err != nil || acct(cs2, ubiPoolAddr).Balance.Float() != rest {
		t.Fatalf("Nachspielen: %v, Topf %v statt %v", err, acct(cs2, ubiPoolAddr).Balance.Float(), rest)
	}
	cs2.mu.Lock()
	err = cs2.applyUBIFinalizeDeltaLocked(t.Context(), 123456789, -1)
	cs2.mu.Unlock()
	if err == nil {
		t.Fatal("negativer Topfstand angenommen")
	}
}
