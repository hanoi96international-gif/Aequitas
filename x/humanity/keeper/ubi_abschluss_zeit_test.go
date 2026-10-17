package keeper

import (
	"errors"
	"testing"
)

// Missbrauch (Sicherheitspruefung #391, info): ubi_distribution_finalize setzt
// last_ubi_at auf seinen Zeitpunkt. Ein boeswilliger Erzeuger konnte ihn
// beliebig setzen (negativ, weit in der Zukunft) und damit Doppelrunden-
// Erkennung und Mindestabstand aller Knoten verschieben. Jetzt gilt dieselbe
// Grenze wie fuer die Rundenmarke: hoechstens 10 min neben dem Block.
// Beobachtet vor dem strengen Nachrechnen, abgelehnt ab ihm; der Erzeuger
// legt so eine Transaktion im strengen Modus gar nicht erst in einen Block.
func TestNachrechnen_UBIAbschlussZeitBegrenzt(t *testing.T) {
	block := int64(1_800_000_000)
	zaehler := func() int64 { return erhaltungZaehler("ubi_abschluss_zeit") }
	pruefe := func(at int64) error {
		cs := nachrechnenTestState()
		tx := Transaction{Type: "ubi_distribution_finalize", DistributionAt: at}
		cs.mu.Lock()
		defer cs.mu.Unlock()
		return cs.nachrechnenTxLocked(&tx, block)
	}

	// Beobachten (vor dem strengen Modus): gezaehlt, nicht abgelehnt.
	for _, at := range []int64{-1, 0, block - 601, block + 601, block + 400*86400} {
		vorher := zaehler()
		if err := pruefe(at); errors.Is(err, ErrZustandLehntAb) {
			t.Fatalf("at=%d: vor dem strengen Modus abgelehnt: %v", at, err)
		}
		if zaehler()-vorher != 1 {
			t.Fatalf("at=%d: nicht als ubi_abschluss_zeit gezaehlt", at)
		}
	}
	// Gutfall: an der Grenze kein Befund.
	for _, at := range []int64{block, block - 600, block + 600} {
		vorher := zaehler()
		_ = pruefe(at)
		if zaehler() != vorher {
			t.Fatalf("at=%d: ehrlicher Abschluss gezaehlt", at)
		}
	}

	// Streng: abgelehnt -- im Nachrechnen und schon beim Erzeuger.
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })
	for _, at := range []int64{-1, block - 601, block + 601} {
		if err := pruefe(at); !errors.Is(err, ErrZustandLehntAb) {
			t.Fatalf("at=%d: im strengen Modus nicht abgelehnt: %v", at, err)
		}
		tx := Transaction{Type: "ubi_distribution_finalize", DistributionAt: at}
		if blockTauglich(&tx, block) == nil {
			t.Fatalf("at=%d: Erzeuger haette den Abschluss in den Block gelegt", at)
		}
	}
	tx := Transaction{Type: "ubi_distribution_finalize", DistributionAt: block - 30}
	if err := blockTauglich(&tx, block); err != nil {
		t.Fatalf("ehrlicher Abschluss nicht blocktauglich: %v", err)
	}
}

// Ehrlicher Weg auf Ausgangsebene (Pruefung von e956700a, niedrig): Abschluss
// und Rundenmarke tragen denselben Zeitpunkt. Im strengen Modus kommen beide
// in denselben Block, solange die Elternzeit hoechstens 10 min nach der Runde
// liegt -- nie einer ohne den anderen. Danach ein definierter Fehler (k == 0),
// kein Weglassen.
func TestBlockZeitPraefix_UBIAbschlussUndMarkeZusammen(t *testing.T) {
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })
	at := rundenZeitStrengAbUnix + 30*86400
	ausgang := []Transaction{
		{Type: "ubi_distribution", Wallet: "0xa", Amount: 1},
		{Type: "ubi_distribution_finalize", DistributionAt: at},
		{Type: "distribution_round_marker", DistributionAt: at},
	}
	for _, d := range []int64{0, 5, 300, 599} {
		k, zeit, err := blockZeitPraefix(ausgang, at+d+1, at+d)
		if err != nil || k != len(ausgang) {
			t.Fatalf("Elternzeit at+%d: k=%d zeit=%d err=%v, want alle %d in einem Block", d, k, zeit, err, len(ausgang))
		}
		if zeit-at > 600 || at-zeit > 600 {
			t.Fatalf("Elternzeit at+%d: Blockzeit %d ausserhalb der 10 min", d, zeit)
		}
	}
	// Elternzeit schon ueber at+600: kein Block, nichts weggelassen.
	k, _, err := blockZeitPraefix(ausgang, at+700, at+650)
	if err == nil {
		// Teilen ist nur vor dem Abschluss erlaubt -- nie Abschluss ohne Marke.
		if k == 2 {
			t.Fatal("Abschluss ohne Rundenmarke in einen Block gelegt")
		}
		if k > 1 {
			t.Fatalf("k=%d: Runde ausserhalb der 10 min in einen Block gelegt", k)
		}
	}
}
