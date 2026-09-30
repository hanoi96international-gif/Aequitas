package keeper

import (
	"path/filepath"
	"testing"
	"time"
)

// Befund 30.09.2026 (CI, sporadisch): der WAL-Flush schrieb Saldo, wal_seq
// und Nonce, aber nicht last_activity_at. Wurde vor einem Absturz geflusht,
// lud der Neustart die ALTE Aktivitaetszeit, und recoverFromWAL wendete die
// Ueberweisung nicht noch einmal an -- die Uhr des Absenders stand zurueck.
// Hier wird der Flush erzwungen, damit der Fall jedes Mal eintritt.
func TestTransferConcurrentWAL_FlushVorAbsturz_BehaeltAktivitaet(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "test.wal")
	truncateDistTestTables(t)

	tTransfer := time.Now().Unix() - 1000
	csA := newWALTestState(t, walPath)
	from, to := distTestAddr(892), distTestAddr(893)
	seedConcurrentTestAccount(t, csA, from, 1000, tTransfer-100)
	seedConcurrentTestAccount(t, csA, to, 0, tTransfer-100)

	alt := setzeZeitQuelleFuerTest(func() time.Time { return time.Unix(tTransfer, 0) })
	_, _, applied, err := csA.transferConcurrentWAL(from, to, 100, Transaction{
		Type: "transfer", Wallet: from, To: to, Amount: 100, TxHash: "0xflushaktivitaet",
	})
	setzeZeitQuelleFuerTest(alt)
	if !applied || err != nil {
		t.Fatalf("transfer: applied=%v err=%v", applied, err)
	}

	// Flush erzwingen, dann "Absturz".
	csA.stopWALFlushWorkerForTest()
	csA.flushWALQueue()
	if err := csA.wal.Close(); err != nil {
		t.Fatalf("closing WAL: %v", err)
	}

	csB := newWALTestState(t, walPath)
	csB.mu.RLock()
	acc, ok := csB.accounts.Get(from)
	csB.mu.RUnlock()
	if !ok {
		t.Fatal("Absender nach Neustart nicht geladen")
	}
	if acc.LastActivityAt != tTransfer {
		t.Fatalf("Absender LastActivityAt nach Flush+Neustart = %d, erwartet %d (Zeit der Ueberweisung) -- der Flush hat die Aktivitaet nicht geschrieben",
			acc.LastActivityAt, tTransfer)
	}
	// 1000 - 100 - Gebuehr (0,1 %, sobald die Wirtschaftsregeln gelten).
	if got := acc.Balance.Float(); got > 900 || got < 899.8 {
		t.Fatalf("Saldo nach Neustart = %v, erwartet 900 abzueglich hoechstens 0,2 Gebuehr", got)
	}
}
