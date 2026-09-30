package keeper

import (
	"context"
	"math"
	"testing"
)

const lpTestWallet = "0xa100000000000000000000000000000000000c02"

// lpTestZustand: ein Mensch mit guthaben AEQ und einem Anteil am Pool, der
// lpAEQ AEQ wert ist (Pool 1:1, der Mensch haelt die Haelfte der Anteile).
func lpTestZustand(guthaben, lpAEQ float64) *ChainState {
	cs := newTestState()
	cs.accounts.Set(lpTestWallet, &AccountState{
		Address: lpTestWallet, IsHuman: true,
		Balance: NewDecimal(guthaben), LPShares: NewDecimal(lpAEQ),
	})
	cs.humanCount = 1
	cs.pool = &PoolState{
		ReserveAEQ: NewDecimal(2 * lpAEQ), ReserveTUSD: NewDecimal(2 * lpAEQ),
		TotalLPShares: NewDecimal(2 * lpAEQ),
	}
	return cs
}

func umlaufRunde(t *testing.T, cs *ChainState, at int64) []Transaction {
	t.Helper()
	cs.wirt().letzterUmlauf = at - 86400
	cs.mu.Lock()
	defer cs.mu.Unlock()
	txs, err := cs.umlaufLocked(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	return txs
}

func einTagAbgabeMensch(stand float64) float64 {
	return round6(math.Max(0, stand-menschSparFreibetrag) * menschUmlaufMonat * 86400 / sekundenJeMonat)
}

// Missbrauch (C2): 25.000 AEQ, davon 10.000 im Pool. Vor dem Stichtag zahlt
// der Mensch nur auf 15.000 -- der Pool war eine Zuflucht. Ab dem Stichtag auf
// alles.
func TestUmlauf_LPIstKeineZufluchtMehr(t *testing.T) {
	wirtschaftAn(t)

	vorher := umlaufRunde(t, lpTestZustand(15000, 10000), umlaufMitLPAbUnix-3600)
	if len(vorher) != 1 || math.Abs(vorher[0].Amount-einTagAbgabeMensch(15000)) > 1e-9 {
		t.Fatalf("vor dem Stichtag: erwartet Abgabe auf 15.000 (%.6f), bekommen %+v", einTagAbgabeMensch(15000), vorher)
	}

	nachher := umlaufRunde(t, lpTestZustand(15000, 10000), umlaufMitLPAbUnix+3600)
	if len(nachher) != 1 || math.Abs(nachher[0].Amount-einTagAbgabeMensch(25000)) > 1e-9 {
		t.Fatalf("ab dem Stichtag: erwartet Abgabe auf 25.000 (%.6f), bekommen %+v", einTagAbgabeMensch(25000), nachher)
	}
	if nachher[0].Amount <= vorher[0].Amount {
		t.Fatal("die LP-Anteile senken die Abgabe weiterhin")
	}
}

// Wer ALLES im Pool haelt (Guthaben 0), stand bisher nicht einmal auf der
// Kandidatenliste. Jetzt wird die Abgabe aus den LP-Anteilen geloest -- ohne
// dass Geld entsteht oder verschwindet.
func TestUmlauf_AllesImPool_WirdAusLPEingezogen(t *testing.T) {
	wirtschaftAn(t)
	cs := lpTestZustand(0, 20000)
	at := umlaufMitLPAbUnix + 3600
	vorLP := acct(cs, lpTestWallet).LPShares

	var txs []Transaction
	assertConserved(t, cs, "Umlauf aus LP-Anteilen", func() {
		txs = umlaufRunde(t, cs, at)
	})
	if len(txs) != 1 || math.Abs(txs[0].Amount-einTagAbgabeMensch(20000)) > 1e-9 {
		t.Fatalf("erwartet eine Abgabe auf 20.000 (%.6f), bekommen %+v", einTagAbgabeMensch(20000), txs)
	}
	nachLP := acct(cs, lpTestWallet).LPShares
	if nachLP >= vorLP {
		t.Fatal("keine LP-Anteile aufgeloest")
	}
	ubi := acct(cs, ubiPoolAddr)
	if ubi == nil || math.Abs(ubi.Balance.Float()-txs[0].Amount) > 2e-6 {
		t.Fatalf("Grundeinkommen sollte %.6f bekommen, hat %v", txs[0].Amount, ubi)
	}
}

// Erzeuger und nachspielender Knoten kommen auf dieselben Zahlen: der
// Nachspielende prueft den Betrag (keine Abweichung) und loest dieselben
// Anteile auf.
func TestUmlauf_LP_ErzeugerUndNachspielerGleich(t *testing.T) {
	wirtschaftAn(t)
	at := umlaufMitLPAbUnix + 3600
	for _, fall := range []struct{ guthaben, lp float64 }{{0, 20000}, {3000, 12000}, {9000, 9000}} {
		erzeuger := lpTestZustand(fall.guthaben, fall.lp)
		nachspieler := lpTestZustand(fall.guthaben, fall.lp)
		nachspieler.wirt().letzterUmlauf = at - 86400

		txs := umlaufRunde(t, erzeuger, at)
		if len(txs) != 1 {
			t.Fatalf("%+v: erwartet eine Umlauf-Transaktion, bekommen %+v", fall, txs)
		}

		abw := liegegeldAbweichungen.Load()
		nachspieler.mu.Lock()
		if err := nachspieler.pruefeUmlaufLocked(txs[0].Wallet, txs[0].Amount, at); err != nil {
			t.Fatal(err)
		}
		if err := nachspieler.applyUmlaufDeltaLocked(context.Background(), txs[0].Wallet, txs[0].Amount, at); err != nil {
			t.Fatal(err)
		}
		nachspieler.mu.Unlock()
		if liegegeldAbweichungen.Load() != abw {
			t.Errorf("%+v: die Pruefung meldet eine Abweichung, obwohl beide gleich rechnen", fall)
		}

		e, n := acct(erzeuger, lpTestWallet), acct(nachspieler, lpTestWallet)
		if e.Balance != n.Balance || e.TUsdBalance != n.TUsdBalance || e.LPShares != n.LPShares {
			t.Errorf("%+v: Konto weicht ab: Erzeuger %v/%v/%v, Nachspieler %v/%v/%v", fall,
				e.Balance, e.TUsdBalance, e.LPShares, n.Balance, n.TUsdBalance, n.LPShares)
		}
		if *erzeuger.pool != *nachspieler.pool {
			t.Errorf("%+v: Pool weicht ab: %+v gegen %+v", fall, *erzeuger.pool, *nachspieler.pool)
		}
		if acct(erzeuger, ubiPoolAddr).Balance != acct(nachspieler, ubiPoolAddr).Balance {
			t.Errorf("%+v: Grundeinkommen weicht ab", fall)
		}
	}
}

// Ein Erzeuger, der auf den LP-Anteilen zu viel abzieht, faellt der Pruefung
// auf (im strengen Modus lehnt sie den Block ab).
func TestUmlauf_LP_UeberhoehterBetragFaelltAuf(t *testing.T) {
	wirtschaftAn(t)
	t.Setenv("AEQUITAS_LIEGEGELD_PRUEFUNG", "streng")
	at := umlaufMitLPAbUnix + 3600
	cs := lpTestZustand(0, 20000)
	cs.wirt().letzterUmlauf = at - 86400
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.pruefeUmlaufLocked(lpTestWallet, 10*einTagAbgabeMensch(20000), at); err == nil {
		t.Fatal("zehnfacher Betrag auf LP-Anteilen wurde nicht abgelehnt")
	}
	if err := cs.pruefeUmlaufLocked(lpTestWallet, einTagAbgabeMensch(20000), at); err != nil {
		t.Fatalf("richtiger Betrag abgelehnt: %v", err)
	}
}
