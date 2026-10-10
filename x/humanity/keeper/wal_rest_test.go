package keeper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

// Pruefung von #322, INFO-15, und von #331: ein Knoten mit ungeflushtem
// WAL-Satz, der den Rest nicht einspielt (Beobachter, WAL aus, Wiederanlauf
// gescheitert), darf nicht starten.

// walRestAnlegen: Validator A ueberweist 42 ueber das WAL und "stuerzt ab",
// bevor der Satz in Postgres steht.
func walRestAnlegen(t *testing.T, walPath string) (from, to string) {
	t.Helper()
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
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

// Gegenprobe: der gewoehnliche Wiederanlauf als Validator spielt den Rest ein
// und startet.
func TestWALRest_ValidatorSpieltEinUndStartet_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "gewoehnlich.wal")
	from, to := walRestAnlegen(t, walPath)
	csC := newWALTestState(t, walPath)
	if b := speicherStand(t, csC, to); b.Float() != 42 {
		t.Fatalf("Empfaenger %s, erwartet 42", b)
	}
	if b := speicherStand(t, csC, from); b.Float() >= 1000-42 || b.IsNegative() {
		t.Fatalf("Absender %s", b)
	}
	if err := csC.PruefeWALRest(); err != nil {
		t.Fatalf("Validator mit eingespieltem Rest aufgehalten: %v", err)
	}
}

// Ein Beobachter mit nicht abgeglichenem WAL startet nicht. Bewusstes
// Verwerfen gilt nur fuer genau den gemeldeten Stand (Kopf-seq) und setzt die
// Untergrenze; der spaetere Validator wendet den Satz dann nicht mehr an, und
// eine stehengebliebene Variable verwirft keinen neuen Rest (Pruefung #331,
// Befund 5).
func TestWALRest_BeobachterStartetNicht_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "beobachter-rest.wal")
	from, to := walRestAnlegen(t, walPath)
	setzeBeobachterFuerTest(t, true)
	csB := testKnoten(t, "unused-wal-test.json")
	err := csB.PruefeWALRest()
	if err == nil || !strings.Contains(err.Error(), "2 Konten, bis seq 1") || !strings.Contains(err.Error(), "Beobachter") ||
		!strings.Contains(err.Error(), "AEQUITAS_WAL_REST_VERWERFEN=1") {
		t.Fatalf("Beobachter mit WAL-Rest: %v", err)
	}
	// Auch wenn nur der Empfaenger den Satz noch nicht enthaelt.
	if _, err := csB.db.Exec(`UPDATE chain_accounts SET wal_seq = 1 WHERE lower(address) = $1`, from); err != nil {
		t.Fatal(err)
	}
	if err := csB.PruefeWALRest(); err == nil {
		t.Fatal("Beobachter startet, obwohl der Empfaenger den Satz nicht enthaelt")
	}
	if _, err := csB.db.Exec(`UPDATE chain_accounts SET wal_seq = 0 WHERE lower(address) = $1`, from); err != nil {
		t.Fatal(err)
	}
	// Ohne AEQUITAS_WAL_ENABLED dasselbe: die Datei ueberlebt den Schalter.
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	if err := csB.PruefeWALRest(); err == nil {
		t.Fatal("ohne AEQUITAS_WAL_ENABLED startet der Beobachter mit WAL-Rest")
	}
	// Mehr Konten als die Grenze: fail-closed.
	alt := walRestHoechstensKonten
	walRestHoechstensKonten = 1
	if err := csB.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "mehr als 1 Konten") {
		walRestHoechstensKonten = alt
		t.Fatalf("Kontengrenze: %v", err)
	}
	walRestHoechstensKonten = alt
	// Ein falscher Stand verwirft nichts.
	for _, falsch := range []string{"ja", "true", "2"} {
		t.Setenv("AEQUITAS_WAL_REST_VERWERFEN", falsch)
		if err := csB.PruefeWALRest(); err == nil || csB.walRecoveryFloor() != 0 {
			t.Fatalf("AEQUITAS_WAL_REST_VERWERFEN=%s hat verworfen: %v, Untergrenze %d", falsch, err, csB.walRecoveryFloor())
		}
	}
	t.Setenv("AEQUITAS_WAL_REST_VERWERFEN", "1")
	if err := csB.PruefeWALRest(); err != nil {
		t.Fatalf("bewusst verworfen, startet trotzdem nicht: %v", err)
	}
	if csB.walRecoveryFloor() != 1 {
		t.Fatalf("Untergrenze %d, erwartet 1", csB.walRecoveryFloor())
	}
	// Spaeter wieder Validator: der verworfene Satz bleibt verworfen.
	beobachterAn.Store(false)
	t.Setenv("AEQUITAS_WAL_ENABLED", "1")
	csC := newWALTestState(t, walPath)
	if b := speicherStand(t, csC, from); b.Float() != 1000 {
		t.Fatalf("verworfener Satz angewandt: Absender %s", b)
	}
	if b := speicherStand(t, csC, to); !b.IsZero() {
		t.Fatalf("verworfener Satz angewandt: Empfaenger %s", b)
	}
	// Ein neuer Rest (seq 2) -- die stehengebliebene Variable (=1) verwirft ihn
	// nicht.
	if _, _, applied, err := csC.transferConcurrentWAL(from, to, 7, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 7, TxHash: "0xwalrestneu"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: zweite Ueberweisung: applied=%v err=%v", applied, err)
	}
	csC.stopWALFlushWorkerForTest()
	if err := csC.wal.Close(); err != nil {
		t.Fatal(err)
	}
	setzeBeobachterFuerTest(t, true)
	csD := testKnoten(t, "unused-wal-test.json")
	if err := csD.PruefeWALRest(); err == nil || csD.walRecoveryFloor() != 1 {
		t.Fatalf("stehengebliebene Variable hat einen neuen Rest verworfen: %v, Untergrenze %d", err, csD.walRecoveryFloor())
	}
}

// Pruefung #331, Befund 3: ein Validator mit ausgeschaltetem WAL spielt den
// Rest nicht ein -- der Absender stuende bei 1000 und koennte die schon
// ueberwiesenen 42 ein zweites Mal ausgeben. Er startet nicht.
func TestWALRest_ValidatorOhneWALStartetNicht_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "ohne-wal.wal")
	from, _ := walRestAnlegen(t, walPath)
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	cs := testKnoten(t, "unused-wal-test.json")
	if cs.wal != nil {
		t.Fatal("Vorbedingung: WAL aus")
	}
	if b := speicherStand(t, cs, from); b.Float() != 1000 {
		t.Fatalf("Vorbedingung: Rest nicht eingespielt, Absender %s", b)
	}
	if err := cs.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "ausgeschaltet") {
		t.Fatalf("Validator ohne WAL mit Rest: %v", err)
	}
}

// Pruefung #331, Befund 3: scheitert der Wiederanlauf (hier: der Empfaenger
// fehlt in chain_accounts), lief der Knoten bisher ohne den Rest weiter.
// Jetzt startet er nicht.
func TestWALRest_WiederanlaufGescheitertStartetNicht_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "gescheitert.wal")
	_, to := walRestAnlegen(t, walPath)
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	cs0 := testKnoten(t, "unused-wal-test.json")
	if _, err := cs0.db.Exec(`DELETE FROM chain_accounts WHERE lower(address) = $1`, to); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEQUITAS_WAL_ENABLED", "1")
	t.Setenv("AEQUITAS_WAL_PATH", walPath)
	cs := testKnoten(t, "unused-wal-test.json")
	if cs.wal != nil {
		cs.stopWALFlushWorkerForTest()
		cs.wal.Close()
		t.Fatal("Vorbedingung: Wiederanlauf gelungen")
	}
	if err := cs.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "gescheitert") {
		t.Fatalf("gescheiterter Wiederanlauf mit Rest: %v", err)
	}
}

// Abgeglichen (geflusht) oder ohne Datei startet der Beobachter; ein
// Validator mit offenem WAL wird nie aufgehalten.
func TestWALRest_AbgeglichenStartet_RealDB(t *testing.T) {
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
	if err := csA.PruefeWALRest(); err != nil {
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
	if err := csB.PruefeWALRest(); err != nil {
		t.Fatalf("abgeglichenes WAL haelt den Beobachter auf: %v", err)
	}
	t.Setenv("AEQUITAS_WAL_PATH", filepath.Join(t.TempDir(), "gibt-es-nicht.wal"))
	if err := csB.PruefeWALRest(); err != nil {
		t.Fatalf("ohne WAL-Datei: %v", err)
	}
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("Pruefung hat die Datei angefasst: %v", err)
	}
}

// Zwei Saetze desselben Kontos, nur der erste steht in Postgres (wal_seq 1):
// der zweite ist offen -- massgeblich ist der hoechste Satz je Konto.
func TestWALRest_ZweiterSatzOffen_RealDB(t *testing.T) {
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
	if err := csB.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "2 Konten") {
		t.Fatalf("zweiter Satz offen, Beobachter: %v", err)
	}
}
