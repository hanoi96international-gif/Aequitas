package keeper

import (
	"context"
	"errors"
	"testing"
)

// Missbrauchstests zum Audit vom 21.10.2026 (AUDIT_2026-10-21.md).

const (
	auditA = "0xa0d1000000000000000000000000000000000001"
	auditB = "0xa0d1000000000000000000000000000000000002"
	auditU = "0xa0d1000000000000000000000000000000000003"
)

func auditPoolZustand() *ChainState {
	cs := newTestState()
	addHuman(cs, auditA, 1000)
	acct(cs, auditA).TUsdBalance = NewDecimal(10)
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(100), ReserveTUSD: NewDecimal(100), TotalLPShares: NewDecimal(100)}
	return cs
}

// K2: Ein Block behauptet ein Tauschergebnis ueber der Reserve. Vorher klemmte
// das Nachspielen die Reserve auf null und schrieb den vollen Betrag gut
// (gemessen: 50.000 AEQ aus 100 AEQ). Jetzt: abgelehnt, nichts veraendert.
func TestAudit_NachspielenTauschUeberReserveWirdAbgelehnt(t *testing.T) {
	for _, fall := range []struct {
		name      string
		aeqToTusd bool
		out       float64
	}{
		{"tUSD->AEQ weit ueber der Reserve", false, 50_000},
		{"tUSD->AEQ knapp ueber der Reserve", false, 100.000001},
		{"AEQ->tUSD ueber der Reserve", true, 1_000},
	} {
		t.Run(fall.name, func(t *testing.T) {
			cs := auditPoolZustand()
			vorKonto, vorTUSD := acct(cs, auditA).Balance, acct(cs, auditA).TUsdBalance
			vorPool := *cs.pool
			cs.mu.Lock()
			err := cs.applySwapDeltaLockedMitAbgabe(context.Background(), auditA, 1, fall.out, fall.aeqToTusd, 0, 1, 0)
			cs.mu.Unlock()
			if !errors.Is(err, ErrZustandLehntAb) {
				t.Fatalf("Ergebnis %.0f bei Reserve 100 nicht abgelehnt: %v", fall.out, err)
			}
			if acct(cs, auditA).Balance != vorKonto || acct(cs, auditA).TUsdBalance != vorTUSD {
				t.Fatalf("Konto veraendert trotz Ablehnung")
			}
			if *cs.pool != vorPool {
				t.Fatalf("Pool veraendert trotz Ablehnung: %+v -> %+v", vorPool, *cs.pool)
			}
		})
	}
}

// Gegenprobe zu K2: ein ehrliches Ergebnis geht weiter durch, und die Summe
// aller Konten (einschliesslich der Toepfe, in die die Tauschgebuehr geht)
// und der Reserve bleibt erhalten.
func TestAudit_NachspielenEhrlicherTauschGehtDurch(t *testing.T) {
	cs := auditPoolZustand()
	_, _, inPool := swapTeilung(1, 1)
	out := AMMSwapOut(cs.pool.ReserveTUSD, cs.pool.ReserveAEQ, inPool).Float()
	summe := func() Decimal {
		var d Decimal
		cs.accounts.Range(func(_ string, a *AccountState) bool { d = d.Add(a.Balance); return true })
		return d.Add(cs.pool.ReserveAEQ)
	}
	vorher := summe()
	cs.mu.Lock()
	err := cs.applySwapDeltaLockedMitAbgabe(context.Background(), auditA, 1, out, false, 0, 1, 0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("ehrlicher Tausch abgelehnt: %v", err)
	}
	if nachher := summe(); nachher != vorher {
		t.Fatalf("AEQ aus Konten und Reserve veraendert: %v -> %v", vorher.Float(), nachher.Float())
	}
}

// Grenzfall zu K2: ein Ergebnis von genau der Reserve (Rundung auf Mikro-AEQ
// bei sehr grossem Einsatz) ist kein Geld aus dem Nichts -- es leert die
// Reserve, mehr nicht. Abgelehnt wuerde ein ehrlicher Block zur Spaltung.
func TestAudit_NachspielenTauschGenauDieReserve(t *testing.T) {
	cs := auditPoolZustand()
	summe := func() Decimal {
		var d Decimal
		cs.accounts.Range(func(_ string, a *AccountState) bool { d = d.Add(a.Balance); return true })
		return d.Add(cs.pool.ReserveAEQ)
	}
	vorher := summe()
	cs.mu.Lock()
	err := cs.applySwapDeltaLockedMitAbgabe(context.Background(), auditA, 1, 100, false, 0, 1, 0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("Ergebnis gleich der Reserve abgelehnt: %v", err)
	}
	if !cs.pool.ReserveAEQ.IsZero() {
		t.Fatalf("Reserve nicht geleert: %v", cs.pool.ReserveAEQ.Float())
	}
	if nachher := summe(); nachher != vorher {
		t.Fatalf("AEQ aus Konten und Reserve veraendert: %v -> %v", vorher.Float(), nachher.Float())
	}
}

// M1: Betraege, die auf null Mikro-AEQ runden, werden bei der Annahme
// abgewiesen -- vorher galten sie als erfolgreiche, gebuehrenfreie
// Ueberweisungen, die nichts bewegen.
func TestAudit_UeberweisungUnterEinemMikroWirdAbgewiesen(t *testing.T) {
	cs := newTestState()
	addHuman(cs, auditA, 1000)
	addHuman(cs, auditB, 1000)
	for _, betrag := range []float64{1e-18, 1e-9, 4.9e-7} {
		_, _, err := cs.TransferAtomic(auditA, auditB, betrag, Transaction{Type: "transfer", Wallet: auditA, To: auditB, Amount: betrag})
		if !errors.Is(err, ErrBetragUnterMikro) {
			t.Fatalf("Betrag %g angenommen: %v", betrag, err)
		}
	}
	if acct(cs, auditA).Balance.Float() != 1000 || acct(cs, auditB).Balance.Float() != 1000 {
		t.Fatalf("Kontostand veraendert")
	}
	// Gegenprobe: ein Mikro-AEQ ist die kleinste gueltige Ueberweisung.
	if _, _, err := cs.TransferAtomic(auditA, auditB, 1e-6, Transaction{Type: "transfer", Wallet: auditA, To: auditB, Amount: 1e-6}); err != nil {
		t.Fatalf("ein Mikro-AEQ abgewiesen: %v", err)
	}
}

// K2 (Gebuehr): ein Block traegt eine Gebuehr ueber 0,1 % des Betrags. Im
// strengen Modus abgelehnt, im Beobachtungsmodus gezaehlt.
func TestAudit_NachrechnenUeberhoehteGebuehr(t *testing.T) {
	wirtschaftAn(t)
	cs := newTestState()
	addHuman(cs, auditA, 1000)
	addHuman(cs, auditB, 1000)
	ueberhoeht := Transaction{Type: "transfer", Wallet: auditA, To: auditB, Amount: 100, Gebuehr: 900}
	ehrlich := Transaction{Type: "transfer", Wallet: auditA, To: auditB, Amount: 100, Gebuehr: grundGebuehr(100)}

	nachrechnenModus(t, true)
	if err := cs.nachrechnenTxLocked(&ueberhoeht, 1); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("Gebuehr 900 auf 100 AEQ nicht abgelehnt: %v", err)
	}
	if err := cs.nachrechnenTxLocked(&ehrlich, 1); err != nil {
		t.Fatalf("ehrliche Gebuehr abgelehnt: %v", err)
	}
	unter := Transaction{Type: "transfer", Wallet: auditA, To: auditB, Amount: 1e-9}
	if err := cs.nachrechnenTxLocked(&unter, 1); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("Betrag unter einem Mikro-AEQ nicht abgelehnt: %v", err)
	}

	nachrechnenModus(t, false)
	vorher := nachrechnenAbweichungen.Load()
	if err := cs.nachrechnenTxLocked(&ueberhoeht, 1); err != nil {
		t.Fatalf("Beobachtungsmodus darf nicht ablehnen: %v", err)
	}
	if nachrechnenAbweichungen.Load() != vorher+1 {
		t.Fatalf("Abweichung nicht gezaehlt")
	}
}

// K2 (Gebuehr): ein Unternehmen zahlt auf Loehne an Menschen nichts. Eine
// Gebuehr darauf ist ein Abzug, den niemand unterschrieben hat.
func TestAudit_NachrechnenLohnOhneGebuehr(t *testing.T) {
	wirtschaftAn(t)
	nachrechnenModus(t, true)
	cs := newTestState()
	addHuman(cs, auditB, 1000)
	cs.accounts.Set(auditU, &AccountState{Address: auditU, Balance: NewDecimal(5000)})
	cs.wirt().unternehmen[auditU] = &unternehmenEintrag{Adresse: auditU, Verantwortliche: []string{auditB}, EroeffnetAm: 1}
	lohn := Transaction{Type: "transfer", Wallet: auditU, To: auditB, Amount: 1000, Gebuehr: 1}
	if err := cs.nachrechnenTxLocked(&lohn, 1); !errors.Is(err, ErrZustandLehntAb) {
		t.Fatalf("Gebuehr auf Lohn nicht abgelehnt: %v", err)
	}
	lohn.Gebuehr = 0
	if err := cs.nachrechnenTxLocked(&lohn, 1); err != nil {
		t.Fatalf("Lohn ohne Gebuehr abgelehnt: %v", err)
	}
}

// Der parallele Nachspielpfad ueberspringt nachrechnenTxLocked. Was die
// Pruefung beanstanden wuerde, muss deshalb in den seriellen Pfad fallen --
// sonst genuegte es, eine ueberhoehte Gebuehr in einen Lauf zu legen.
func TestAudit_ParallelerLaufNimmtKeineUeberhoehteGebuehr(t *testing.T) {
	txs := []Transaction{
		{Type: "transfer", Wallet: auditA, To: auditB, Amount: 10, Gebuehr: grundGebuehr(10)},
		{Type: "transfer", Wallet: auditB, To: auditU, Amount: 10, Gebuehr: 5},
	}
	if batch, _ := collectDisjointTransferBatch(txs, 0); len(batch) != 1 {
		t.Fatalf("Lauf mit ueberhoehter Gebuehr: %d statt 1 Ueberweisung", len(batch))
	}
	if batch, _ := collectDisjointTransferBatch(txs, 1); len(batch) != 0 {
		t.Fatalf("ueberhoehte Gebuehr landete im parallelen Lauf")
	}
	unter := []Transaction{{Type: "transfer", Wallet: auditA, To: auditB, Amount: 1e-9}}
	if batch, _ := collectDisjointTransferBatch(unter, 0); len(batch) != 0 {
		t.Fatalf("Betrag unter einem Mikro-AEQ landete im parallelen Lauf")
	}
}

// Der gemeinsame Stichtag ist gesetzt (nicht mehr unendlich) -- wer ihn
// verschiebt, muss diesen Test aendern und das begruenden.
func TestAudit_NachrechnenStichtagGesetzt(t *testing.T) {
	if nachrechnenStrengAbUnix != 1793491200 {
		t.Fatalf("nachrechnenStrengAbUnix = %d, erwartet 2026-11-01T00:00:00Z (1793491200)", nachrechnenStrengAbUnix)
	}
}
