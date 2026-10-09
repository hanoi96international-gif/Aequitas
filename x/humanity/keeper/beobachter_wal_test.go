package keeper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

// Pruefung von #322, INFO-15: ein abgestuerzter Validator mit ungeflushtem
// WAL-Satz laeuft erst als Beobachter (spielt fremde Bloecke ohne den Satz
// nach), dann wieder als Validator.

// walRestAnlegen: Validator A ueberweist 42 ueber das WAL und "stuerzt ab",
// bevor der Satz in Postgres steht.
func walRestAnlegen(t *testing.T, walPath string) (from, to string) {
	t.Helper()
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	if _, err := csA.db.Exec(`DELETE FROM wal_geparkt`); err != nil {
		t.Fatal(err)
	}
	untergrenzeZuruecksetzen(t, csA)
	from, to = distTestAddr(840), distTestAddr(841)
	seedConcurrentTestAccount(t, csA, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, to, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(from, to, 42, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 42, TxHash: "0xbeobachterwalrest"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: Ueberweisung ueber das WAL: applied=%v err=%v", applied, err)
	}
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	var dbFrom float64
	csA.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, from).Scan(&dbFrom)
	if dbFrom != 1000 {
		t.Fatalf("Vorbedingung: der Satz steht schon in Postgres (%v)", dbFrom)
	}
	return from, to
}

// untergrenzeZuruecksetzen: die Untergrenze des Wiederanlaufs steht in
// chain_config, das truncateDistTestTables nicht leert -- vorher und nachher
// loeschen, sonst sieht der naechste WAL-Test sie.
func untergrenzeZuruecksetzen(t *testing.T, cs *ChainState) {
	t.Helper()
	loeschen := func() error {
		_, err := cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, walRecoveryFloorKey)
		return err
	}
	if err := loeschen(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := loeschen(); err != nil {
			t.Errorf("Untergrenze nicht geloescht: %v", err)
		}
	})
}

func dbStand(t *testing.T, cs *ChainState, addr string) float64 {
	t.Helper()
	var b float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, addr).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func speicherStand(t *testing.T, cs *ChainState, addr string) Decimal {
	t.Helper()
	acc, ok := cs.accounts.Get(addr)
	if !ok {
		t.Fatalf("Konto %s nicht geladen", addr)
	}
	return acc.Balance
}

// Der PoC: als Beobachter hat der Knoten Bloecke nachgespielt, die den
// Absender leerten (wal_seq bleibt stehen). Der Wiederanlauf als Validator
// wandte den alten Satz an: Kontostand -42,042 und eine Zeile im Ausgang.
// Jetzt: geparkt, nicht angewandt, nichts im Ausgang.
func TestWAL_WiederanlaufNachBeobachterParktUngedecktes_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "rollenwechsel.wal")
	from, to := walRestAnlegen(t, walPath)
	// Der Beobachter: ohne WAL (er liest es nicht ein).
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	csB0 := testKnoten(t, "unused-wal-test.json")
	if csB0.wal != nil {
		t.Fatal("Vorbedingung: Hilfsknoten ohne WAL")
	}
	// Was das Nachspielen fremder Bloecke als Beobachter schrieb: 42 deckt
	// den Betrag, nicht aber Betrag und Gebuehr.
	if _, err := csB0.db.Exec(`UPDATE chain_accounts SET balance = 42 WHERE lower(address) = $1`, from); err != nil {
		t.Fatal(err)
	}
	var vorher int
	csB0.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&vorher)

	for runde := 1; runde <= 2; runde++ { // zweiter Start: wieder geparkt, nicht doppelt
		csC := newWALTestState(t, walPath)
		if b := speicherStand(t, csC, from); b.Float() != 42 {
			t.Fatalf("Runde %d: Absender im Speicher %s, erwartet 42", runde, b)
		}
		if b := speicherStand(t, csC, to); !b.IsZero() {
			t.Fatalf("Runde %d: Empfaenger im Speicher %s -- Geld ohne Abbuchung", runde, b)
		}
		csC.FlushWALNow()
		if b := dbStand(t, csC, from); b != 42 {
			t.Fatalf("Runde %d: Absender in Postgres %v", runde, b)
		}
		var nachher, geparkt int
		csC.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&nachher)
		csC.db.QueryRow(`SELECT COUNT(*) FROM wal_geparkt WHERE von = $1 AND an = $2 AND betrag = 42`, from, to).Scan(&geparkt)
		if nachher != vorher || geparkt != 1 {
			t.Fatalf("Runde %d: Ausgang %d -> %d, geparkt %d", runde, vorher, nachher, geparkt)
		}
		if !strings.Contains(csC.BootstrapDegradedReason(), "wal_geparkt") {
			t.Fatalf("Runde %d: kein Hinweis fuer den Betreiber: %q", runde, csC.BootstrapDegradedReason())
		}
		csC.stopWALFlushWorkerForTest()
		csC.wal.Close()
	}
}

// Gegenprobe: im gewoehnlichen Wiederanlauf (Stand gedeckt) wird der Satz
// angewandt wie bisher, nichts geparkt.
func TestWAL_WiederanlaufGedecktWendetAn_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "gewoehnlich.wal")
	from, to := walRestAnlegen(t, walPath)
	csC := newWALTestState(t, walPath)
	if b := speicherStand(t, csC, to); b.Float() != 42 {
		t.Fatalf("Empfaenger %s, erwartet 42", b)
	}
	if b := speicherStand(t, csC, from); b.Float() >= 1000-42 || b.IsNegative() {
		t.Fatalf("Absender %s", b)
	}
	var geparkt int
	csC.db.QueryRow(`SELECT COUNT(*) FROM wal_geparkt`).Scan(&geparkt)
	if geparkt != 0 {
		t.Fatalf("%d gedeckte Saetze geparkt", geparkt)
	}
}

// Ein Beobachter mit nicht abgeglichenem WAL startet nicht. Bewusstes
// Verwerfen setzt die Untergrenze; der spaetere Validator wendet den Satz
// dann nicht mehr an.
func TestBeobachter_WALRestStartetNicht_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "beobachter-rest.wal")
	from, to := walRestAnlegen(t, walPath)
	setzeBeobachterFuerTest(t, true)
	csB := testKnoten(t, "unused-wal-test.json")
	err := csB.PruefeBeobachterWAL()
	if err == nil || !strings.Contains(err.Error(), "2 Konten mit nicht abgeglichenen") {
		t.Fatalf("Beobachter mit WAL-Rest: %v", err)
	}
	// Auch wenn nur der Empfaenger den Satz noch nicht enthaelt.
	if _, err := csB.db.Exec(`UPDATE chain_accounts SET wal_seq = 1 WHERE lower(address) = $1`, from); err != nil {
		t.Fatal(err)
	}
	if err := csB.PruefeBeobachterWAL(); err == nil {
		t.Fatal("Beobachter startet, obwohl der Empfaenger den Satz nicht enthaelt")
	}
	if _, err := csB.db.Exec(`UPDATE chain_accounts SET wal_seq = 0 WHERE lower(address) = $1`, from); err != nil {
		t.Fatal(err)
	}
	// Ohne AEQUITAS_WAL_ENABLED dasselbe: die Datei ueberlebt den Schalter.
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	if err := csB.PruefeBeobachterWAL(); err == nil {
		t.Fatal("ohne AEQUITAS_WAL_ENABLED startet der Beobachter mit WAL-Rest")
	}
	// Mehr Konten als die Grenze: fail-closed.
	alt := beobachterWALHoechstensKonten
	beobachterWALHoechstensKonten = 1
	if err := csB.PruefeBeobachterWAL(); err == nil || !strings.Contains(err.Error(), "mehr als 1 Konten") {
		beobachterWALHoechstensKonten = alt
		t.Fatalf("Kontengrenze: %v", err)
	}
	beobachterWALHoechstensKonten = alt
	// Ein Tippfehler verwirft nichts.
	t.Setenv("AEQUITAS_BEOBACHTER_WAL_VERWERFEN", "ja")
	if err := csB.PruefeBeobachterWAL(); err == nil || csB.walRecoveryFloor() != 0 {
		t.Fatalf("Tippfehler hat verworfen: %v, Untergrenze %d", err, csB.walRecoveryFloor())
	}
	t.Setenv("AEQUITAS_BEOBACHTER_WAL_VERWERFEN", "1")
	if err := csB.PruefeBeobachterWAL(); err != nil {
		t.Fatalf("bewusst verworfen, startet trotzdem nicht: %v", err)
	}
	if csB.walRecoveryFloor() == 0 {
		t.Fatal("Untergrenze nicht gesetzt")
	}
	// Spaeter wieder Validator: der verworfene Satz bleibt verworfen.
	beobachterAn.Store(false)
	csC := newWALTestState(t, walPath)
	if b := speicherStand(t, csC, from); b.Float() != 1000 {
		t.Fatalf("verworfener Satz angewandt: Absender %s", b)
	}
	if b := speicherStand(t, csC, to); !b.IsZero() {
		t.Fatalf("verworfener Satz angewandt: Empfaenger %s", b)
	}
}

// Abgeglichen (geflusht) oder ohne Datei startet der Beobachter; ein
// Validator wird nie aufgehalten.
func TestBeobachter_WALAbgeglichenStartet_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "abgeglichen.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	from, to := distTestAddr(842), distTestAddr(843)
	seedConcurrentTestAccount(t, csA, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, to, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(from, to, 42, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 42, TxHash: "0xbeobachterwalok"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: applied=%v err=%v", applied, err)
	}
	// Ein Validator mit Rest startet immer (er gleicht ihn ab).
	if err := csA.PruefeBeobachterWAL(); err != nil {
		t.Fatalf("Validator aufgehalten: %v", err)
	}
	csA.FlushWALNow()
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	if dbStand(t, csA, to) != 42 {
		t.Fatal("Vorbedingung: Satz nicht geflusht")
	}
	setzeBeobachterFuerTest(t, true)
	csB := testKnoten(t, "unused-wal-test.json")
	if err := csB.PruefeBeobachterWAL(); err != nil {
		t.Fatalf("abgeglichenes WAL haelt den Beobachter auf: %v", err)
	}
	t.Setenv("AEQUITAS_WAL_PATH", filepath.Join(t.TempDir(), "gibt-es-nicht.wal"))
	if err := csB.PruefeBeobachterWAL(); err != nil {
		t.Fatalf("ohne WAL-Datei: %v", err)
	}
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("Pruefung hat die Datei angefasst: %v", err)
	}
}

// Laesst sich ein ungedeckter Satz nicht parken, scheitert der Wiederanlauf
// (der Schnellpfad bleibt aus) -- er wird weder angewandt noch still
// verloren.
func TestWAL_ParkenScheitertHaeltWiederanlauf_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "parken-scheitert.wal")
	from, to := walRestAnlegen(t, walPath)
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	cs0 := testKnoten(t, "unused-wal-test.json")
	if _, err := cs0.db.Exec(`UPDATE chain_accounts SET balance = 0 WHERE lower(address) = $1`, from); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE OR REPLACE FUNCTION wal_geparkt_sperre() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'gesperrt (Test)'; END $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS wal_geparkt_sperre ON wal_geparkt`,
		`CREATE TRIGGER wal_geparkt_sperre BEFORE INSERT ON wal_geparkt FOR EACH ROW EXECUTE FUNCTION wal_geparkt_sperre()`,
	} {
		if _, err := cs0.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if _, err := cs0.db.Exec(`DROP TRIGGER IF EXISTS wal_geparkt_sperre ON wal_geparkt`); err != nil {
			t.Errorf("Trigger nicht entfernt: %v", err)
		}
	})
	t.Setenv("AEQUITAS_WAL_ENABLED", "1")
	t.Setenv("AEQUITAS_WAL_PATH", walPath)
	cs := testKnoten(t, "unused-wal-test.json")
	if cs.wal != nil {
		cs.stopWALFlushWorkerForTest()
		cs.wal.Close()
		t.Fatal("Wiederanlauf trotz gescheitertem Parken gelungen")
	}
	if b := speicherStand(t, cs, from); !b.IsZero() {
		t.Fatalf("Absender %s", b)
	}
	if b := speicherStand(t, cs, to); !b.IsZero() {
		t.Fatalf("Empfaenger %s", b)
	}
}

// Zwei Saetze desselben Kontos, nur der erste steht in Postgres (wal_seq 1):
// der zweite ist offen -- massgeblich ist der hoechste Satz je Konto.
func TestBeobachter_WALZweiterSatzOffen_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "zwei-saetze.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	from, to := distTestAddr(844), distTestAddr(845)
	seedConcurrentTestAccount(t, csA, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, to, 0, time.Now().Unix())
	for i, h := range []string{"0xzweisaetze1", "0xzweisaetze2"} {
		if _, _, applied, err := csA.transferConcurrentWAL(from, to, 1, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 1, TxHash: h}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := csA.db.Exec(`UPDATE chain_accounts SET wal_seq = 1 WHERE lower(address) = ANY($1)`, pq.Array([]string{from, to})); err != nil {
		t.Fatal(err)
	}
	setzeBeobachterFuerTest(t, true)
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	csB := testKnoten(t, "unused-wal-test.json")
	if err := csB.PruefeBeobachterWAL(); err == nil || !strings.Contains(err.Error(), "2 Konten") {
		t.Fatalf("zweiter Satz offen, Beobachter: %v", err)
	}
}
