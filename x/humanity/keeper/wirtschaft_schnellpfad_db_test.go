package keeper

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"testing"
)

// Der WAL-Schnellpfad mit aktiven Wirtschaftsregeln muss genau dasselbe
// ergeben wie transferMutateLocked: dieselben Gebuehren (Monatsfreibetrag),
// dieselben Ablehnungen (Grenze freier Adressen), dieselbe Buchfuehrung.
// Zwei Kontensaetze in einem Knoten, einer ueber den Schnellpfad, einer
// seriell, jede Ueberweisung auf beiden, jedes Ergebnis verglichen.

func schnellpfadKonto(t *testing.T, cs *ChainState, addr string, mensch bool, guthaben float64) {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	acc := &AccountState{Address: addr, IsHuman: mensch, Balance: NewDecimal(guthaben), LastActivityAt: nowUnix()}
	if err := cs.saveAccountToDB(acc); err != nil {
		t.Fatal(err)
	}
	cs.accounts.Set(addr, acc)
}

func schnellpfadStand(cs *ChainState, addr string) float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if acc, ok := cs.accounts.Get(addr); ok {
		return acc.Balance.Float()
	}
	return math.NaN()
}

func schnellpfadAusgegeben(cs *ChainState, addr string) float64 {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	if k := w.buch[addr]; k != nil {
		return k.Ausgegeben
	}
	return 0
}

func schnellpfadSeriell(cs *ChainState, from, to string, betrag float64, hash string) error {
	return cs.runAtomicWithOutbox([]string{from, to}, false, func(ctx context.Context) (Transaction, error) {
		if _, _, _, err := cs.transferLockedMitGebuehr(ctx, from, to, betrag); err != nil {
			return Transaction{}, err
		}
		return Transaction{Type: "transfer", Wallet: from, To: to, Amount: betrag, TxHash: hash}, nil
	})
}

func schnellpfadLeeren(t *testing.T, cs *ChainState) {
	t.Helper()
	for _, tabelle := range []string{"wirtschaft_buch", "wirtschaft_unternehmen", "wirtschaft_meta"} {
		if _, err := cs.db.Exec("DELETE FROM " + tabelle); err != nil {
			t.Fatal(err)
		}
	}
	cs.wirtschaftP.Store(neueWirtschaft())
}

func TestWirtschaftSchnellpfad_WieSeriell_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "w.wal"))
	schnellpfadLeeren(t, cs)

	// s = Schnellpfad, r = seriell (Referenz)
	type satz struct{ mensch, empf, frei string }
	s := satz{distTestAddr(901), distTestAddr(902), distTestAddr(903)}
	r := satz{distTestAddr(911), distTestAddr(912), distTestAddr(913)}
	for _, k := range []satz{s, r} {
		schnellpfadKonto(t, cs, k.mensch, true, 3000)
		schnellpfadKonto(t, cs, k.empf, true, 0)
		schnellpfadKonto(t, cs, k.frei, false, 0)
	}

	schritte := []struct {
		name          string
		von, an       func(satz) string
		betrag        float64
		fehlerErwarte bool
	}{
		{"Mensch an Mensch, im Freibetrag", func(k satz) string { return k.mensch }, func(k satz) string { return k.empf }, 600, false},
		{"Mensch an Mensch, ueber den Freibetrag", func(k satz) string { return k.mensch }, func(k satz) string { return k.empf }, 600, false},
		{"Mensch an freie Adresse", func(k satz) string { return k.mensch }, func(k satz) string { return k.frei }, 200, false},
		{"freie Adresse ueber 250", func(k satz) string { return k.mensch }, func(k satz) string { return k.frei }, 100, true},
		{"freie Adresse an Mensch", func(k satz) string { return k.frei }, func(k satz) string { return k.empf }, 50, false},
		{"zu wenig Guthaben", func(k satz) string { return k.frei }, func(k satz) string { return k.empf }, 10_000, true},
	}
	for i, st := range schritte {
		_, _, applied, errS := cs.transferConcurrentWAL(st.von(s), st.an(s), st.betrag,
			Transaction{Type: "transfer", Wallet: st.von(s), To: st.an(s), Amount: st.betrag, TxHash: fmt.Sprintf("0xschnell%d", i)})
		if !applied {
			t.Fatalf("%s: Schnellpfad trat zurueck -- er soll Menschen und freie Adressen selbst nehmen", st.name)
		}
		errR := schnellpfadSeriell(cs, st.von(r), st.an(r), st.betrag, fmt.Sprintf("0xseriell%d", i))
		if (errS != nil) != st.fehlerErwarte || (errR != nil) != st.fehlerErwarte {
			t.Fatalf("%s: Fehler Schnellpfad=%v seriell=%v, erwartet Fehler=%v", st.name, errS, errR, st.fehlerErwarte)
		}
		for _, paar := range [][2]string{{s.mensch, r.mensch}, {s.empf, r.empf}, {s.frei, r.frei}} {
			if a, b := schnellpfadStand(cs, paar[0]), schnellpfadStand(cs, paar[1]); a != b {
				t.Fatalf("%s: Stand %v (Schnellpfad) != %v (seriell)", st.name, a, b)
			}
		}
		if a, b := schnellpfadAusgegeben(cs, s.mensch), schnellpfadAusgegeben(cs, r.mensch); a != b {
			t.Fatalf("%s: Ausgegeben %v (Schnellpfad) != %v (seriell)", st.name, a, b)
		}
	}
	// Die zweite Ueberweisung lag zu 200 ueber dem Freibetrag: Gebuehr 0,2.
	if got, want := schnellpfadStand(cs, s.mensch), 3000.0-600-600-0.2-200-0.2; math.Abs(got-want) > 1e-9 {
		t.Errorf("Stand Mensch = %v, erwartet %v", got, want)
	}

	// Mit Unternehmen: serieller Weg.
	firma := distTestAddr(920)
	schnellpfadKonto(t, cs, firma, false, 0)
	eroeffne(t, cs, context.Background(), firma, s.empf)
	if _, _, applied, _ := cs.transferConcurrentWAL(s.mensch, firma, 10, Transaction{Type: "transfer", Wallet: s.mensch, To: firma, Amount: 10, TxHash: "0xfirma"}); applied {
		t.Fatal("Ueberweisung an ein Unternehmen nahm den Schnellpfad -- sie gehoert auf den seriellen Weg")
	}

	// Flush schreibt die Buchkonten mit; BuchAt steht in der Transaktion.
	cs.FlushWALNow()
	var daten string
	if err := cs.db.QueryRow(`SELECT daten FROM wirtschaft_buch WHERE address = $1`, s.mensch).Scan(&daten); err != nil {
		t.Fatalf("Buchkonto nach dem Flush nicht in der Datenbank: %v", err)
	}
	var buchAt int64
	if err := cs.db.QueryRow(`SELECT COALESCE((tx_json::json->>'buch_at')::bigint, 0) FROM pending_txs WHERE tx_json::json->>'tx_hash' = '0xschnell0'`).Scan(&buchAt); err != nil {
		t.Fatal(err)
	}
	if buchAt != 1_800_000_000 {
		t.Errorf("BuchAt = %d, erwartet den Annahmezeitpunkt 1800000000", buchAt)
	}
}

// Absturz zwischen Annahme und Flush: Ausgegeben wird genau einmal gebucht --
// auch wenn die Erholung zweimal laeuft.
func TestWirtschaftSchnellpfad_Absturz_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	wirtschaftAn(t)
	uhr(t, 1_800_000_000)
	walPfad := filepath.Join(t.TempDir(), "w.wal")
	csA := newWALTestState(t, walPfad)
	schnellpfadLeeren(t, csA)
	mensch, empf := distTestAddr(931), distTestAddr(932)
	schnellpfadKonto(t, csA, mensch, true, 3000)
	schnellpfadKonto(t, csA, empf, true, 0)

	ueberweise := func(cs *ChainState, betrag float64, hash string) {
		t.Helper()
		if _, _, applied, err := cs.transferConcurrentWAL(mensch, empf, betrag, Transaction{Type: "transfer", Wallet: mensch, To: empf, Amount: betrag, TxHash: hash}); !applied || err != nil {
			t.Fatalf("applied=%v err=%v", applied, err)
		}
	}
	ueberweise(csA, 600, "0xabs1")
	csA.FlushWALNow() // die erste steht in Postgres, samt Buch
	ueberweise(csA, 300, "0xabs2")
	ueberweise(csA, 200, "0xabs3") // diese beiden nicht
	standVorher := schnellpfadStand(csA, mensch)
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}

	csB := newWALTestState(t, walPfad)
	if got := schnellpfadAusgegeben(csB, mensch); got != 1100 {
		t.Fatalf("Ausgegeben nach der Erholung = %v, erwartet 1100 (600 + 300 + 200, keins doppelt)", got)
	}
	if got := schnellpfadStand(csB, mensch); got != standVorher {
		t.Fatalf("Stand nach der Erholung = %v, erwartet %v", got, standVorher)
	}
	csB.FlushWALNow()
	csB.stopWALFlushWorkerForTest()
	if err := csB.wal.Close(); err != nil {
		t.Fatal(err)
	}

	// Zweite Erholung nach dem Flush: nichts darf erneut zaehlen.
	csC := newWALTestState(t, walPfad)
	if got := schnellpfadAusgegeben(csC, mensch); got != 1100 {
		t.Fatalf("Ausgegeben nach zweiter Erholung = %v, erwartet weiter 1100", got)
	}
	// Und die naechste Ueberweisung rechnet mit dem richtigen Freibetrag:
	// 1100 schon ausgegeben, also volle Gebuehr auf 100.
	vorher := schnellpfadStand(csC, mensch)
	ueberweise(csC, 100, "0xabs4")
	if got, want := vorher-schnellpfadStand(csC, mensch), 100.1; math.Abs(got-want) > 1e-9 {
		t.Fatalf("abgebucht %v, erwartet %v (Freibetrag verbraucht)", got, want)
	}
}
