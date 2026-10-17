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
