package keeper

import (
	"errors"
	"math"
	"testing"
)

func nachrechnenTestState() *ChainState {
	cs := newTestState()
	cs.mu.Lock()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100_000), ReserveTUSD: NewDecimal(100_000), TotalLPShares: NewDecimal(1000)}
	cs.accounts.Set("0xmensch", &AccountState{Address: "0xmensch", IsHuman: true, Balance: NewDecimal(10)})
	cs.accounts.Set("0xfrei", &AccountState{Address: "0xfrei", Balance: NewDecimal(10)})
	cs.mu.Unlock()
	return cs
}

// pruefe laeuft im Beobachtungsmodus (blockZeit < Stichtag) und liefert, ob
// die Regel angeschlagen hat -- gemessen am Zaehler, nicht am Fehler.
func pruefe(t *testing.T, cs *ChainState, tx Transaction) bool {
	t.Helper()
	vorher := nachrechnenAbweichungen.Load()
	cs.mu.Lock()
	err := cs.nachrechnenTxLocked(&tx, 1790800000)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
	}
	return nachrechnenAbweichungen.Load() > vorher
}

func erwarteterTausch(cs *ChainState, betrag float64) float64 {
	nachGebuehr := betrag - betrag*float64(swapFeeBps)/10000.0
	return AMMSwapOut(cs.pool.ReserveAEQ, cs.pool.ReserveTUSD, NewDecimal(nachGebuehr)).Float()
}

func TestNachrechnen_GefaelschtesTauschergebnis(t *testing.T) {
	cs := nachrechnenTestState()
	ehrlich := erwarteterTausch(cs, 100)
	if pruefe(t, cs, Transaction{Type: "swap_aeq_tusd", Wallet: "0xfrei", Amount: 100, AmountOut: ehrlich}) {
		t.Fatal("ehrliches Tauschergebnis darf nicht anschlagen")
	}
	// Angriff: AmountOut aus dem Nichts.
	if !pruefe(t, cs, Transaction{Type: "swap_aeq_tusd", Wallet: "0xfrei", Amount: 100, AmountOut: 1e9}) {
		t.Fatal("AmountOut=1e9 muss anschlagen")
	}
	// Angriff andersherum: der Nutzer bekommt fast nichts.
	if !pruefe(t, cs, Transaction{Type: "swap_tusd_aeq", Wallet: "0xfrei", Amount: 100, AmountOut: 0.000001}) {
		t.Fatal("zu kleines AmountOut muss anschlagen")
	}
}

func TestNachrechnen_AufgeblaehteLPAnteile(t *testing.T) {
	cs := nachrechnenTestState()
	ehrlich := (100.0 / 100_000) * 1000
	if pruefe(t, cs, Transaction{Type: "add_liquidity", Wallet: "0xfrei", Amount: 100, AmountOut: 100, LPShares: ehrlich}) {
		t.Fatal("ehrliche LP-Anteile duerfen nicht anschlagen")
	}
	if !pruefe(t, cs, Transaction{Type: "add_liquidity", Wallet: "0xfrei", Amount: 1, AmountOut: 1, LPShares: 1e12}) {
		t.Fatal("LPShares=1e12 fuer 1 AEQ muss anschlagen")
	}
}

func TestNachrechnen_Faucet(t *testing.T) {
	cs := nachrechnenTestState()
	if pruefe(t, cs, Transaction{Type: "faucet", Wallet: "0xmensch", Amount: tusdFaucetAmount}) {
		t.Fatal("ehrlicher Faucet an einen Menschen darf nicht anschlagen")
	}
	if !pruefe(t, cs, Transaction{Type: "faucet", Wallet: "0xmensch", Amount: 1e9}) {
		t.Fatal("Faucet-Betrag 1e9 muss anschlagen")
	}
	if !pruefe(t, cs, Transaction{Type: "faucet", Wallet: "0xfrei", Amount: tusdFaucetAmount}) {
		t.Fatal("Faucet an eine Adresse ohne Menschenstatus muss anschlagen")
	}
}

func TestNachrechnen_UngueltigeBetraege(t *testing.T) {
	cs := nachrechnenTestState()
	for _, tx := range []Transaction{
		{Type: "transfer", Wallet: "0xfrei", To: "0xmensch", Amount: 1, ToDemurrageLost: -5},
		{Type: "transfer", Wallet: "0xfrei", To: "0xmensch", Amount: math.NaN()},
		{Type: "ubi_distribution", Wallet: "0xmensch", Amount: math.Inf(1)},
		{Type: "transfer", Wallet: "0xfrei", To: "0xmensch", Amount: 1, Gebuehr: -1},
	} {
		if !pruefe(t, cs, tx) {
			t.Fatalf("%+v muss anschlagen", tx)
		}
	}
}

func TestNachrechnen_AlteUBIFormUndRundenmarke(t *testing.T) {
	cs := nachrechnenTestState()
	if !pruefe(t, cs, Transaction{Type: "ubi_distribution", AmountPerHuman: 1e6}) {
		t.Fatal("alte UBI-Form mit Betrag fuer alle muss anschlagen")
	}
	if pruefe(t, cs, Transaction{Type: "distribution_round_marker", DistributionAt: 1790800000 - 30}) {
		t.Fatal("Rundenmarke nahe der Blockzeit darf nicht anschlagen")
	}
	if !pruefe(t, cs, Transaction{Type: "distribution_round_marker", DistributionAt: 1790800000 + 86400}) {
		t.Fatal("Rundenmarke einen Tag in der Zukunft muss anschlagen")
	}
}

// Im strengen Modus lehnt dieselbe Abweichung den Block ab.
func TestNachrechnen_StrengLehntAb(t *testing.T) {
	cs := nachrechnenTestState()
	tx := Transaction{Type: "faucet", Wallet: "0xmensch", Amount: 1e9}
	cs.mu.Lock()
	err := cs.nachrechnenTxLocked(&tx, math.MaxInt64)
	cs.mu.Unlock()
	if !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("streng: erwartet ErrZustandLehntAb, bekam %v", err)
	}
	if nachrechnenStreng(1790800000) {
		t.Fatal("heute muss noch der Beobachtungsmodus gelten")
	}
}
