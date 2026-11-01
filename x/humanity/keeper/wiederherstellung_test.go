package keeper

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestWiederherstellung_Sicherung ist die Wiederherstellungs-Uebung
// (wiederherstellung-ueben.yml): Die naechtliche Sicherung von C1
// (backup-ledger.yml, Artefakt) ist auf einer fremden Maschine in ein
// frisches PostgreSQL zurueckgespielt; DATABASE_URL zeigt darauf. Geprueft
// wird, was ein neuer Knoten aus genau dieser Datei machen wuerde:
//
//   - NewChainState laedt den Zustand ueber denselben Weg wie aequitasd beim
//     Start (keine Testabkuerzung);
//   - es gibt Menschen (mindestens AEQUITAS_WIEDERHERSTELLUNG_MENSCHEN);
//   - die Geldmenge geht auf (supply_reconciliation.reconciled) -- eine
//     Sicherung, in der Geld fehlt oder zu viel ist, ist keine;
//   - die StateRoot laesst sich berechnen.
//
// Die Stichtage gelten wie im Betrieb, nicht wie in den uebrigen Tests
// (TestMain stellt sie auf "schlafend"). Ohne AEQUITAS_WIEDERHERSTELLUNG=1
// laeuft der Test nicht: er braucht die zurueckgespielte Sicherung.
func TestWiederherstellung_Sicherung(t *testing.T) {
	if os.Getenv("AEQUITAS_WIEDERHERSTELLUNG") != "1" {
		t.Skip("nur in wiederherstellung-ueben.yml (braucht die zurueckgespielte Sicherung)")
	}
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL fehlt -- ohne Datenbank gibt es nichts zu pruefen")
	}
	stichtageWieImBetrieb(t)

	cs := NewChainState(filepath.Join(t.TempDir(), "wiederherstellung.json"))
	if !cs.useDB {
		t.Fatal("die zurueckgespielte Datenbank wurde nicht verwendet")
	}

	mindestens := 1
	if s := os.Getenv("AEQUITAS_WIEDERHERSTELLUNG_MENSCHEN"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			t.Fatalf("AEQUITAS_WIEDERHERSTELLUNG_MENSCHEN=%q ist keine Zahl", s)
		}
		mindestens = n
	}
	menschen := cs.TotalHumans()
	if menschen < mindestens {
		t.Fatalf("%d Menschen in der Sicherung, erwartet mindestens %d", menschen, mindestens)
	}

	cs.mu.RLock()
	konten := cs.accounts.Len()
	cs.mu.RUnlock()
	if konten == 0 {
		t.Fatal("keine Konten geladen")
	}

	abgleich := cs.SupplyReconciliation()
	if abgleich["reconciled"] != true {
		t.Fatalf("Geldmenge geht nicht auf: %v", abgleich)
	}
	gemessen, ok, grund := cs.MeasuredTotalAEQ()
	if !ok || math.IsNaN(gemessen) || gemessen <= 0 {
		t.Fatalf("Geldmenge nicht messbar: %v %s", gemessen, grund)
	}

	wurzel := cs.StateRoot()
	if wurzel == "" {
		t.Fatal("StateRoot leer")
	}
	t.Logf("WIEDERHERGESTELLT menschen=%d konten=%d geldmenge=%.6f stateroot=%s", menschen, konten, gemessen, wurzel)
}

// stichtageWieImBetrieb stellt die Stichtage, die TestMain fuer alle Tests
// schlafen legt, auf ihre Werte im Code -- ausdruecklich die Konstanten,
// nicht 0 -- und danach zurueck.
func stichtageWieImBetrieb(t *testing.T) {
	t.Helper()
	wirtschaftAktivOverride.Store(wirtschaftAktivAbUnix)
	signierteUeberweisungenOverride.Store(signierteUeberweisungenAbUnix)
	registerLeserOverride.Store(registerLeserAbUnix)
	erzeugerSchnittOverride.Store(erzeugerSchnittAbUnix)
	nachrechnenStrengOverride.Store(nachrechnenStrengAbUnix)
	t.Cleanup(func() {
		wirtschaftAktivOverride.Store(math.MaxInt64)
		signierteUeberweisungenOverride.Store(math.MaxInt64)
		registerLeserOverride.Store(math.MaxInt64)
		erzeugerSchnittOverride.Store(math.MaxInt64)
		nachrechnenStrengOverride.Store(math.MaxInt64)
	})
}
