package keeper

// Missbrauchstests aus der getrennten Sicherheitspruefung des Punkt-9-Fixes
// (H-1, M-1 bis M-4). Jeder war auf dem ersten Stand des Fixes rot; M-2
// bestand schon vorher.

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/wal"
	"github.com/lib/pq"
)

func walAusgangKopieren(t *testing.T, von, nach string) {
	t.Helper()
	a, err := os.Open(von)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := os.Create(nach)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(b, a); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
}

func walAusgangSpeicher(cs *ChainState, addr string) (stand float64, walSeq uint64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	acc, ok := cs.accounts.Get(addr)
	if !ok {
		return -1, 0
	}
	return acc.Balance.Float(), acc.WALSeq
}

// H-1: Speicherkorb an -> aus. Der erste Start ohne Korb sieht die Marke (bis
// dahin verblockt) und loescht sie danach. Ohne "erledigt" holte der ZWEITE
// Start fuer jeden dieser Saetze eine Ausgangszeile nach -- eine zweite
// Buchung im Block -- und die Startsperre sperrte faelschlich.
func TestWALAusgang_KorbAusZweiterNeustartKeineZweiteZeile_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "korb-aus.wal")
	truncateDistTestTables(t)
	t.Setenv("AEQUITAS_BLOCK_AUS_SPEICHER", "1")
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	if csA.korb == nil {
		t.Fatal("Vorbedingung: Speicherkorb an")
	}
	s, r := distTestAddr(880), distTestAddr(881)
	seedConcurrentTestAccount(t, csA, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	for i, h := range []string{"0xwalausgang-korb-1", "0xwalausgang-korb-2"} {
		if _, _, applied, err := csA.transferConcurrentWAL(s, r, 10, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 10, TxHash: h}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	csA.FlushWALNow()
	csA.stopWALFlushWorkerForTest()
	// Ein Block nahm beide Korb-Eintraege auf (SaveBlockMitKorb setzt die Marke).
	if err := csA.setConfigValueDB(speicherKorbMarke, "2"); err != nil {
		t.Fatal(err)
	}
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	if walAusgangZeilen(t, csA, 1)+walAusgangZeilen(t, csA, 2) != 0 {
		t.Fatal("Vorbedingung: Korb-Modus schreibt keine Zeilen")
	}

	// 1. Neustart, Korb aus: Uebergang, nichts nachzuholen, Marke weg.
	t.Setenv("AEQUITAS_BLOCK_AUS_SPEICHER", "")
	csB := newWALTestState(t, walPath)
	csB.FlushWALNow()
	if n := walAusgangZeilen(t, csB, 1) + walAusgangZeilen(t, csB, 2); n != 0 {
		t.Fatalf("1. Neustart: %d Zeilen fuer verblockte Saetze", n)
	}
	if _, da, err := csB.speicherKorbMarkeLesen(); err != nil || da {
		t.Fatalf("Vorbedingung: Marke nach dem Uebergang noch da (%v, %v)", da, err)
	}
	if e, err := csB.walAusgangErledigt(); err != nil || e != 2 {
		t.Fatalf("erledigt nach dem Uebergang: %d, %v -- erwartet 2", e, err)
	}
	csB.stopWALFlushWorkerForTest()
	csB.wal.Close()

	// Start ohne WAL: alles verblockt und in Postgres, die Sperre greift nicht.
	t.Run("Startsperre", func(t *testing.T) {
		t.Setenv("AEQUITAS_WAL_ENABLED", "")
		cs := testKnoten(t, "unused-wal-test.json")
		if err := cs.PruefeWALRest(); err != nil {
			t.Errorf("Startsperre sperrt faelschlich (alles verblockt und geflusht): %v", err)
		}
	})

	// 2. Neustart, Korb weiter aus: keine zweite Zeile.
	csC := newWALTestState(t, walPath)
	csC.FlushWALNow()
	for seq := uint64(1); seq <= 2; seq++ {
		if n := walAusgangZeilen(t, csC, seq); n != 0 {
			t.Errorf("2. Neustart: %d neue Ausgangszeile(n) fuer seq%d -- die Ueberweisung steht schon in einem Block", n, seq)
		}
	}
}

// M-3: Die Kompaktierung loescht die Vermerke unter bis sofort haltbar, die
// Umbenennung der gekuerzten Datei ist es nicht (kein fsync des
// Verzeichnisses). Liegt nach einem Stromausfall die alte Datei wieder da,
// gelten ihre Saetze dank "erledigt" trotzdem als erledigt -- keine zweite
// Zeile fuer Verblocktes.
func TestWALAusgang_KompaktierungUmbenennungVerlorenKeineZweiteZeile_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "kompakt.wal")
	sicherung := filepath.Join(t.TempDir(), "kompakt-vorher.wal")
	truncateDistTestTables(t)
	cs := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, cs)
	s, r := distTestAddr(887), distTestAddr(888)
	seedConcurrentTestAccount(t, cs, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, r, 0, time.Now().Unix())
	for i, h := range []string{"0xwalausgang-kompakt-1", "0xwalausgang-kompakt-2", "0xwalausgang-kompakt-3"} {
		if _, _, applied, err := cs.transferConcurrentWAL(s, r, 1, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 1, TxHash: h}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	cs.FlushWALNow()
	cs.stopWALFlushWorkerForTest()
	// Alle drei Zeilen gingen in einen Block (geloescht, Vermerk bleibt).
	if _, err := cs.db.Exec(`DELETE FROM pending_txs WHERE wal_seq IN (1,2,3)`); err != nil {
		t.Fatal(err)
	}
	walAusgangKopieren(t, walPath, sicherung)
	// Kompaktierung bis 3, wie walKompaktierungsRunde.
	if err := cs.wal.TruncateBefore(3); err != nil {
		t.Fatal(err)
	}
	if err := cs.walAusgangKuerzen(3); err != nil {
		t.Fatal(err)
	}
	cs.wal.Close()
	// Stromausfall: die Umbenennungen sind nicht auf der Platte.
	walAusgangKopieren(t, sicherung, walPath)
	os.Remove(walPath + ".seqhwm")

	csB := newWALTestState(t, walPath)
	csB.FlushWALNow()
	for seq := uint64(1); seq <= 3; seq++ {
		if n := walAusgangZeilen(t, csB, seq); n != 0 {
			t.Errorf("seq%d: %d neue Ausgangszeile(n) fuer eine verblockte Ueberweisung", seq, n)
		}
	}
}

// walAusgangNichtHaltbar bringt S in den Zustand der Befunde M-1/M-2: T0 ist
// geflusht, T1 (900) ist angenommen und im Speicher, aber noch im
// Sammelfenster des WAL-Schreibers (wartezeit). Ein serieller Pfad speichert
// S unter cs.mu; danach "stirbt" der Prozess -- crashPath ist die Datei in
// diesem Augenblick.
func walAusgangNichtHaltbar(t *testing.T, walPath, crashPath string, wartezeit time.Duration, s, r string, stapel bool) *ChainState {
	t.Helper()
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	seedConcurrentTestAccount(t, csA, s, 3000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(s, r, 10, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 10, TxHash: "0xwalausgang-haltbar-0-" + s}); !applied || err != nil {
		t.Fatalf("Vorbedingung T0: applied=%v err=%v", applied, err)
	}
	csA.FlushWALNow()
	csA.stopWALFlushWorkerForTest()
	t.Setenv("AEQUITAS_WAL_MAX_BATCH_WAIT_US", strconv.FormatInt(wartezeit.Microseconds(), 10))
	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		csA.transferConcurrentWAL(s, r, 900, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 900, TxHash: "0xwalausgang-haltbar-1-" + s})
	}()
	frist := time.Now().Add(2 * time.Second)
	for {
		if _, seq := walAusgangSpeicher(csA, s); seq == 2 {
			break
		}
		if time.Now().After(frist) {
			t.Fatal("Vorbedingung: T1 nicht im Speicher angewandt")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if csA.wal.DurableSeq() >= 2 {
		t.Fatal("Vorbedingung: T1 schon haltbar -- das Fenster ist zu")
	}
	csA.mu.Lock()
	accS, _ := csA.accounts.Get(s)
	var err error
	if stapel {
		err = csA.saveAccountsToDBBatch([]*AccountState{accS})
	} else {
		err = csA.saveAccountToDB(accS)
	}
	csA.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	walAusgangKopieren(t, walPath, crashPath)
	<-fertig
	t.Setenv("AEQUITAS_WAL_MAX_BATCH_WAIT_US", "0")
	csA.wal.Close()
	return csA
}

// M-2 (bestand schon vorher): eine serielle Speicherung schrieb den Saldo
// samt Delta eines Satzes, der noch nicht auf der Platte war. Starb der
// Prozess, fehlte der Satz im WAL, Postgres trug die Abbuchung, der
// Empfaenger nie die Gutschrift -- 900 AEQ verschwanden. Jetzt wartet die
// Speicherung auf die Haltbarkeit (hier 0,5 s Sammelfenster, unter der
// Frist von 2 s): die Ueberweisung ist ganz oder gar nicht gebucht.
func TestWALAusgang_SerielleSpeicherungWartetAufHaltbarkeit_RealDB(t *testing.T) {
	for i, stapel := range []bool{false, true} {
		name := map[bool]string{false: "einzeln", true: "Stapel"}[stapel]
		t.Run(name, func(t *testing.T) {
			walPath := filepath.Join(t.TempDir(), "haltbar.wal")
			crashPath := filepath.Join(t.TempDir(), "haltbar-absturz.wal")
			truncateDistTestTables(t)
			s, r := distTestAddr(885+2*i), distTestAddr(886+2*i)
			walAusgangNichtHaltbar(t, walPath, crashPath, 500*time.Millisecond, s, r, stapel)
			n, _, err := wal.ReplayFile(crashPath, func(wal.Entry) error { return nil })
			if err != nil || n != 2 {
				t.Fatalf("die Speicherung wartete nicht auf die Haltbarkeit: Absturzdatei traegt %d Saetze (%v), erwartet 2", n, err)
			}
			csB := newWALTestState(t, crashPath)
			sStand, _ := walAusgangSpeicher(csB, s)
			rStand, _ := walAusgangSpeicher(csB, r)
			if (sStand < 2500) != (rStand > 500) {
				t.Errorf("nach dem Absturz: Absender %v (T1 abgebucht: %v), Empfaenger %v (T1 gutgeschrieben: %v) -- halb gebucht", sStand, sStand < 2500, rStand, rStand > 500)
			}
			if rStand < 900 {
				t.Errorf("Empfaenger %v: die quittierte Ueberweisung fehlt", rStand)
			}
		})
	}
}

// M-1: Kommt die Haltbarkeit nicht in der Frist (hier 3 s Sammelfenster),
// schreibt die serielle Speicherung trotzdem -- wie der Flush. Dann kennt
// Postgres wal_seq 2, die Datei nur 1. Frueher vergab das WAL nach dem
// Neustart Seq 2 erneut: der Flush der neuen Ueberweisung uebersprang den
// Saldo (WHERE wal_seq < 2), und der naechste Wiederanlauf hielt sie fuer
// angewandt -- der Knoten wich ab. Jetzt beginnt das WAL ueber allem, was
// Postgres kennt.
func TestWALAusgang_SeqWirdNichtWiederverwendet_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "seq.wal")
	crashPath := filepath.Join(t.TempDir(), "seq-absturz.wal")
	truncateDistTestTables(t)
	s, r, q := distTestAddr(882), distTestAddr(883), distTestAddr(884)
	csA := walAusgangNichtHaltbar(t, walPath, crashPath, 3*time.Second, s, r, false)
	n, _, err := wal.ReplayFile(crashPath, func(wal.Entry) error { return nil })
	if err != nil || n != 1 {
		t.Fatalf("Vorbedingung: Absturzdatei traegt %d Saetze (%v), erwartet 1", n, err)
	}
	var dbSeq int64
	if err := csA.db.QueryRow(`SELECT wal_seq FROM chain_accounts WHERE lower(address) = $1`, s).Scan(&dbSeq); err != nil || dbSeq != 2 {
		t.Fatalf("Vorbedingung: wal_seq(S) in Postgres %d (%v), erwartet 2 (nach Ablauf der Frist geschrieben)", dbSeq, err)
	}

	csB := newWALTestState(t, crashPath)
	if p := csB.wal.PeekSeq(); p != 3 {
		t.Fatalf("das WAL vergibt als Naechstes Seq %d -- Postgres kennt schon 2, erwartet 3", p)
	}
	seedConcurrentTestAccount(t, csB, q, 0, time.Now().Unix())
	csB.ensureAccountLoadedCtx(t.Context(), s)
	if _, _, applied, err := csB.transferConcurrentWAL(s, q, 50, Transaction{Type: "transfer", Wallet: s, To: q, Amount: 50, TxHash: "0xwalausgang-seq-2"}); !applied || err != nil {
		t.Fatalf("Vorbedingung T2: applied=%v err=%v", applied, err)
	}
	csB.FlushWALNow()
	vorher, _ := walAusgangSpeicher(csB, s)
	if d := dbStand(t, csB, s); d != vorher {
		t.Errorf("nach vollstaendigem Flush: Postgres %v, Speicher %v -- der Flush uebersprang den Saldo", d, vorher)
	}
	csB.stopWALFlushWorkerForTest()
	csB.wal.Close()

	csC := newWALTestState(t, crashPath)
	if b, _ := walAusgangSpeicher(csC, s); b != vorher {
		t.Errorf("nach dem Neustart: Absender %v, vorher %v -- T2 fehlt dem Saldo", b, vorher)
	}
}

// M-1, Variante: auch ein Vermerk kann eine Seq tragen, die nie haltbar
// wurde (der Flush schreibt nach Ablauf der Frist trotzdem). Das WAL beginnt
// auch ueber dem hoechsten Vermerk -- sonst hiesse ein alter Vermerk fuer
// eine wiederverwendete Seq "Zeile schon geschrieben".
func TestWALAusgang_SeqUeberVermerken_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "seq-vermerk.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r := distTestAddr(889), distTestAddr(890)
	seedConcurrentTestAccount(t, csA, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(s, r, 1, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 1, TxHash: "0xwalausgang-seq-vermerk"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: applied=%v err=%v", applied, err)
	}
	csA.FlushWALNow()
	csA.stopWALFlushWorkerForTest()
	csA.wal.Close()
	if _, err := csA.db.Exec(walAusgangVermerkSQL, pq.Array([]int64{9})); err != nil {
		t.Fatal(err)
	}
	csB := newWALTestState(t, walPath)
	if p := csB.wal.PeekSeq(); p != 10 {
		t.Fatalf("das WAL vergibt als Naechstes Seq %d -- ein Vermerk fuer 9 steht schon, erwartet 10", p)
	}
	csB.stopWALFlushWorkerForTest()
	csB.wal.Close()
	// Ebenso ueber jeder Grenze: eine neue Seq darunter galte als ersetzt,
	// erledigt oder verblockt.
	for i, key := range []string{walRecoveryFloorKey, walAusgangErledigtKey, walAusgangAbKey, speicherKorbMarke} {
		t.Run(key, func(t *testing.T) {
			grenze := uint64(20 + 10*i)
			if err := csB.setConfigValueDB(key, strconv.FormatUint(grenze, 10)); err != nil {
				t.Fatal(err)
			}
			cs := newWALTestState(t, walPath)
			if key == speicherKorbMarke {
				// Korb aus, Marke da: der Start schliesst den Uebergang ab
				// (Marke weg, erledigt gehoben) -- vorher wird geoeffnet.
				if e, err := cs.walAusgangErledigt(); err != nil || e < grenze {
					t.Fatalf("erledigt nach dem Korb-Uebergang: %d, %v", e, err)
				}
			}
			if p := cs.wal.PeekSeq(); p <= grenze {
				t.Fatalf("das WAL vergibt als Naechstes Seq %d -- %s steht auf %d", p, key, grenze)
			}
			cs.stopWALFlushWorkerForTest()
			cs.wal.Close()
		})
	}
}

// M-4: Grenze, "erledigt", Untergrenze und Korb-Marke werden fail-closed
// gelesen. getConfigValueDB machte aus einem Lesefehler "nicht gesetzt":
// alte Regel fuer alle Saetze, die echte Grenze danach ueberschrieben, die
// Startsperre frei. Ohne Postgres: eine geschlossene Verbindung.
func TestWALAusgang_GrenzenLesefehlerFailClosed(t *testing.T) {
	defekt, err := sql.Open("postgres", "postgres://x@127.0.0.1:1/x?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defekt.Close()
	cs := &ChainState{db: defekt}
	if ab, da, err := cs.walAusgangAb(); err == nil {
		t.Errorf("walAusgangAb verschluckt den Lesefehler: (%d, %v, nil)", ab, da)
	}
	if e, err := cs.walAusgangErledigt(); err == nil {
		t.Errorf("walAusgangErledigt verschluckt den Lesefehler: (%d, nil)", e)
	}
	if b, err := cs.walRecoveryFloorLesen(); err == nil {
		t.Errorf("walRecoveryFloorLesen verschluckt den Lesefehler: (%d, nil)", b)
	}
	if m, da, err := cs.speicherKorbMarkeLesen(); err == nil {
		t.Errorf("speicherKorbMarkeLesen verschluckt den Lesefehler: (%d, %v, nil)", m, da)
	}
	if h, err := cs.walSeqPostgresHoechste(); err == nil {
		t.Errorf("walSeqPostgresHoechste verschluckt den Lesefehler: (%d, nil)", h)
	}
	if _, _, _, err := cs.walRestPruefen(writeLeerWAL(t)); err == nil {
		t.Error("walRestPruefen verschluckt den Lesefehler")
	}
	// Ohne Datenbank (cs.db nil) ist "nicht gesetzt" richtig.
	ohne := &ChainState{}
	if _, da, err := ohne.configZahlLesen(walAusgangAbKey); err != nil || da {
		t.Errorf("ohne Datenbank: (%v, %v)", da, err)
	}
}

// writeLeerWAL legt eine WAL-Datei mit einem Satz an (walRestPruefen liest
// die Grenzen nur, wenn die Datei existiert).
func writeLeerWAL(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "leer.wal")
	w, err := wal.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Append([]byte(`{"from":"a","to":"b"}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}
