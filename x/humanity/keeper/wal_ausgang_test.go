package keeper

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// docs/OFFEN.md, Punkt 9 (Pruefung von #331, Befund 1 und 2. Durchgang):
// der WAL-Wiederanlauf buchte doppelt (a) bzw. verlor Ausgangszeilen (b).

// langerFlushTakt: kein Flush von selbst -- der Test entscheidet, was wann
// nach Postgres geht. Vor newWALTestState setzen (der Arbeiter liest den
// Takt beim Start).
func langerFlushTakt(t *testing.T) {
	t.Helper()
	alt := walFlushInterval
	walFlushInterval = time.Hour
	t.Cleanup(func() { walFlushInterval = alt })
}

func zeilenMitHash(t *testing.T, cs *ChainState, hash string) int {
	t.Helper()
	var n int
	if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' = $1`, hash).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func walAbsturz(t *testing.T, cs *ChainState) {
	t.Helper()
	cs.stopWALFlushWorkerForTest()
	if err := cs.wal.Close(); err != nil {
		t.Fatal(err)
	}
}

func walUeberweisen(t *testing.T, cs *ChainState, from, to string, betrag float64, hash string) {
	t.Helper()
	if _, _, applied, err := cs.transferConcurrentWAL(from, to, betrag, Transaction{Type: "transfer", Wallet: from, To: to, Amount: betrag, TxHash: hash}); !applied || err != nil {
		t.Fatalf("Ueberweisung %s ueber das WAL: applied=%v err=%v", hash, applied, err)
	}
}

// (a) Eine serielle Speicherung schreibt den Saldo samt noch nicht
// geflushtem WAL-Satz. Vorher ohne wal_seq: der Wiederanlauf buchte die
// Abbuchung ein zweites Mal (PoC aus der Pruefung: 2104,1 -> 1203,2).
func TestWALAusgang_SerielleSpeicherungBuchtNichtDoppelt_RealDB(t *testing.T) {
	// einzeln: UPDATE mit Version; kalt: Version 0 (INSERT ... ON CONFLICT);
	// stapel: saveAccountsToDBBatch.
	for _, weg := range []string{"einzeln", "kalt", "stapel"} {
		t.Run(weg, func(t *testing.T) {
			langerFlushTakt(t)
			truncateDistTestTables(t)
			walPath := filepath.Join(t.TempDir(), "seriell.wal")
			a := newWALTestState(t, walPath)
			untergrenzeZuruecksetzen(t, a)
			s, r := distTestAddr(860), distTestAddr(861)
			seedConcurrentTestAccount(t, a, s, 3000, time.Now().Unix())
			seedConcurrentTestAccount(t, a, r, 0, time.Now().Unix())
			walUeberweisen(t, a, s, r, 900, "0xseriell")

			// Ein langsamer Pfad beruehrt S (etwa X->S 5) und speichert S.
			a.mu.Lock()
			accS, _ := a.accounts.Get(s)
			accS.Balance = accS.Balance.Add(NewDecimal(5))
			var err error
			if weg == "kalt" {
				accS.Version = 0
			}
			if weg == "stapel" {
				err = a.saveAccountsToDBBatch([]*AccountState{accS})
			} else {
				err = a.saveAccountToDB(accS)
			}
			a.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			sollS := speicherStand(t, a, s)
			if got := dbStand(t, a, s); got != sollS.Float() {
				t.Fatalf("Vorbedingung: die serielle Speicherung schreibt den Saldo samt WAL-Satz (%v, Speicher %v)", got, sollS)
			}
			if got := dbStand(t, a, r); got != 0 {
				t.Fatalf("Vorbedingung: der Satz ist nicht geflusht (Empfaenger %v)", got)
			}
			walAbsturz(t, a)

			b := newWALTestState(t, walPath)
			b.FlushWALNow()
			if got := speicherStand(t, b, s); got != sollS {
				t.Fatalf("Absender nach dem Wiederanlauf %v, erwartet %v -- die Abbuchung wurde ein zweites Mal angewandt", got, sollS)
			}
			if got := dbStand(t, b, s); got != sollS.Float() {
				t.Fatalf("Absender in Postgres %v, erwartet %v", got, sollS)
			}
			if got := dbStand(t, b, r); got != 900 {
				t.Fatalf("Empfaenger in Postgres %v, erwartet 900", got)
			}
			if n := zeilenMitHash(t, b, "0xseriell"); n != 1 {
				t.Fatalf("%d Ausgangszeilen, erwartet genau 1", n)
			}
		})
	}
}

// (b) Andere Stapel schreiben beide Konten eines Satzes, dessen eigener
// Stapel offen ist (der Flush schreibt den aktuellen WALSeq je Konto). Vorher
// hielt der Wiederanlauf ihn fuer erledigt, und seine Zeile fehlte dem Netz.
func TestWALAusgang_FremderStapelVerdecktDieZeileNicht_RealDB(t *testing.T) {
	langerFlushTakt(t)
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "stapel.wal")
	a := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, a)
	s, r, x, u := distTestAddr(862), distTestAddr(863), distTestAddr(864), distTestAddr(865)
	seedConcurrentTestAccount(t, a, s, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, r, 0, time.Now().Unix())
	seedConcurrentTestAccount(t, a, x, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, a, u, 0, time.Now().Unix())
	walUeberweisen(t, a, s, r, 10, "0xstapel1")
	walUeberweisen(t, a, s, x, 20, "0xstapel2")
	walUeberweisen(t, a, x, u, 30, "0xstapel3")
	a.stopWALFlushWorkerForTest()
	a.walFlushMu.Lock()
	schlange := append([]walFlushItem(nil), a.walFlushQueue...)
	a.walFlushMu.Unlock()
	if len(schlange) != 3 {
		t.Fatalf("Vorbedingung: 3 Eintraege in der Schlange, %d", len(schlange))
	}
	// Nur die Stapel von Satz 1 und 3 committen, dann Absturz.
	for _, i := range []int{0, 2} {
		if err := a.flushWALBatch(schlange[i : i+1]); err != nil {
			t.Fatal(err)
		}
	}
	var seqS, seqX int64
	a.db.QueryRow(`SELECT wal_seq FROM chain_accounts WHERE lower(address) = $1`, s).Scan(&seqS)
	a.db.QueryRow(`SELECT wal_seq FROM chain_accounts WHERE lower(address) = $1`, x).Scan(&seqX)
	if seqS < 2 || seqX < 2 || zeilenMitHash(t, a, "0xstapel2") != 0 {
		t.Fatalf("Vorbedingung: beide Konten von Satz 2 tragen ihn (S %d, X %d), seine Zeile fehlt", seqS, seqX)
	}
	soll := map[string]Decimal{}
	for _, k := range []string{s, r, x, u} {
		soll[k] = speicherStand(t, a, k)
	}
	if err := a.wal.Close(); err != nil {
		t.Fatal(err)
	}

	b := newWALTestState(t, walPath)
	b.FlushWALNow()
	for _, h := range []string{"0xstapel1", "0xstapel2", "0xstapel3"} {
		if n := zeilenMitHash(t, b, h); n != 1 {
			t.Fatalf("%s: %d Ausgangszeilen, erwartet genau 1", h, n)
		}
	}
	for k, v := range soll {
		if got := speicherStand(t, b, k); got != v {
			t.Fatalf("Konto %s nach dem Wiederanlauf %v, erwartet %v", k, got, v)
		}
		if got := dbStand(t, b, k); got != v.Float() {
			t.Fatalf("Konto %s in Postgres %v, erwartet %v", k, got, v)
		}
	}
	// Ein dritter Start schreibt nichts doppelt.
	walAbsturz(t, b)
	c := newWALTestState(t, walPath)
	c.FlushWALNow()
	if n := zeilenMitHash(t, c, "0xstapel2"); n != 1 {
		t.Fatalf("nach dem dritten Start %d Zeilen fuer Satz 2", n)
	}
}

// Gegenprobe: ist die Zeile geschrieben und schon in einem Block (aus
// pending_txs geloescht), schreibt der Wiederanlauf keine zweite -- der
// Vermerk ueberlebt die Zeile. Und unter der Untergrenze (Saetze von vor den
// Vermerken) gilt die alte Regel.
func TestWALAusgang_VerblockteZeileNichtDoppelt_RealDB(t *testing.T) {
	for _, alt := range []bool{false, true} {
		t.Run(fmt.Sprintf("ohneVermerke=%v", alt), func(t *testing.T) {
			langerFlushTakt(t)
			truncateDistTestTables(t)
			walPath := filepath.Join(t.TempDir(), "verblockt.wal")
			a := newWALTestState(t, walPath)
			untergrenzeZuruecksetzen(t, a)
			s, r := distTestAddr(866), distTestAddr(867)
			seedConcurrentTestAccount(t, a, s, 1000, time.Now().Unix())
			seedConcurrentTestAccount(t, a, r, 0, time.Now().Unix())
			walUeberweisen(t, a, s, r, 10, "0xverblockt1")
			walUeberweisen(t, a, s, r, 20, "0xverblockt2")
			a.FlushWALNow()
			if zeilenMitHash(t, a, "0xverblockt1") != 1 {
				t.Fatal("Vorbedingung: Zeile geschrieben")
			}
			var vermerke int
			a.db.QueryRow(`SELECT count(*) FROM wal_ausgang_vermerk`).Scan(&vermerke)
			if vermerke != 2 {
				t.Fatalf("Vorbedingung: 2 Vermerke, %d", vermerke)
			}
			// Der Block hat die Zeilen aufgenommen.
			if _, err := a.db.Exec(`DELETE FROM pending_txs`); err != nil {
				t.Fatal(err)
			}
			if alt {
				// Wie eine Fassung ohne Vermerke: keine Untergrenze, keine Vermerke.
				if _, err := a.db.Exec(`DELETE FROM chain_config WHERE key = $1`, walVermerkAbKey); err != nil {
					t.Fatal(err)
				}
				if _, err := a.db.Exec(`TRUNCATE wal_ausgang_vermerk`); err != nil {
					t.Fatal(err)
				}
			}
			walAbsturz(t, a)

			b := newWALTestState(t, walPath)
			b.FlushWALNow()
			for _, h := range []string{"0xverblockt1", "0xverblockt2"} {
				if n := zeilenMitHash(t, b, h); n != 0 {
					t.Fatalf("%s: %d Zeilen fuer eine schon verblockte Ueberweisung -- sie kaeme doppelt in einen Block", h, n)
				}
			}
			if alt {
				ab, da, err := b.walVermerkAbLesen()
				if err != nil || !da || ab != 3 {
					t.Fatalf("Untergrenze nach dem ersten Start mit Vermerken: %d da=%v err=%v, erwartet 3", ab, da, err)
				}
			}
		})
	}
}

// Untergrenze oder Vermerk ueber dem, was das WAL vergibt: die Datei wurde
// ersetzt oder geloescht. Der schnelle Pfad bleibt aus, der Knoten startet
// nicht (PruefeWALRest).
func TestWALAusgang_FremdeVermerkeSperren_RealDB(t *testing.T) {
	for _, fall := range []string{"untergrenze", "vermerk"} {
		t.Run(fall, func(t *testing.T) {
			truncateDistTestTables(t)
			walPath := filepath.Join(t.TempDir(), "neu.wal")
			a := newWALTestState(t, walPath)
			untergrenzeZuruecksetzen(t, a)
			walAbsturz(t, a)
			var err error
			if fall == "untergrenze" {
				err = a.setConfigValueDB(walVermerkAbKey, "1000")
			} else {
				_, err = a.db.Exec(`INSERT INTO wal_ausgang_vermerk (seq) VALUES (1)`)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("AEQUITAS_WAL_ENABLED", "1")
			t.Setenv("AEQUITAS_WAL_PATH", walPath)
			b := testKnoten(t, "unused-wal-test.json")
			if b.wal != nil {
				b.stopWALFlushWorkerForTest()
				b.wal.Close()
				t.Fatal("WAL geoeffnet, obwohl die Vermerke zu einer anderen Datei gehoeren")
			}
			if !strings.Contains(b.walWiederanlaufFehler, "ersetzt oder geloescht") {
				t.Fatalf("Grund: %q", b.walWiederanlaufFehler)
			}
			if err := b.PruefeWALRest(); err == nil {
				t.Fatal("Knoten startet trotz fremder Vermerke")
			}
		})
	}
}

// Ein Beobachter startet nicht, solange ein Satz ueber der Untergrenze keine
// Zeile hat -- auch wenn beide Salden ihn tragen. Mit Vermerk oder unter der
// Korb-Marke (schon verblockt) startet er.
func TestWALAusgang_BeobachterMitFehlenderZeile_RealDB(t *testing.T) {
	langerFlushTakt(t)
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "beobachter-zeile.wal")
	a := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, a)
	s, r := distTestAddr(868), distTestAddr(869)
	seedConcurrentTestAccount(t, a, s, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, r, 0, time.Now().Unix())
	walUeberweisen(t, a, s, r, 10, "0xbeobachterzeile")
	walAbsturz(t, a)
	// Beide Salden tragen den Satz, die Zeile fehlt (Fall b).
	if _, err := a.db.Exec(`UPDATE chain_accounts SET wal_seq = 1 WHERE lower(address) IN ($1, $2)`, s, r); err != nil {
		t.Fatal(err)
	}
	setzeBeobachterFuerTest(t, true)
	b := testKnoten(t, "unused-wal-test.json")
	if err := b.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "0 Konten, 1 Saetze ohne Ausgangszeile") {
		t.Fatalf("Beobachter mit fehlender Zeile: %v", err)
	}
	// Unter der Korb-Marke: steht schon in einem Block.
	if err := b.setConfigValueDB(speicherKorbMarke, "1"); err != nil {
		t.Fatal(err)
	}
	if err := b.PruefeWALRest(); err != nil {
		t.Fatalf("schon verblockt, trotzdem gesperrt: %v", err)
	}
	if _, err := b.db.Exec(`DELETE FROM chain_config WHERE key = $1`, speicherKorbMarke); err != nil {
		t.Fatal(err)
	}
	if err := b.PruefeWALRest(); err == nil {
		t.Fatal("ohne Marke muss die Sperre wieder greifen")
	}
	if _, err := b.db.Exec(`INSERT INTO wal_ausgang_vermerk (seq) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if err := b.PruefeWALRest(); err != nil {
		t.Fatalf("mit Vermerk gesperrt: %v", err)
	}
}

// Die Untergrenze steigt nur und nimmt die Vermerke darunter mit.
func TestWALAusgang_UntergrenzeNurAufwaerts_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "grenze.wal"))
	untergrenzeZuruecksetzen(t, cs)
	if _, err := cs.db.Exec(`INSERT INTO wal_ausgang_vermerk (seq) VALUES (5), (10), (20)`); err != nil {
		t.Fatal(err)
	}
	if err := cs.walVermerkAbAnheben(10); err != nil {
		t.Fatal(err)
	}
	if err := cs.walVermerkAbAnheben(7); err != nil {
		t.Fatal(err)
	}
	ab, da, err := cs.walVermerkAbLesen()
	if err != nil || !da || ab != 10 {
		t.Fatalf("Untergrenze %d da=%v err=%v, erwartet 10 (nie abwaerts)", ab, da, err)
	}
	var rest []string
	rows, err := cs.db.Query(`SELECT seq FROM wal_ausgang_vermerk ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s int64
		rows.Scan(&s)
		rest = append(rest, strconv.FormatInt(s, 10))
	}
	if strings.Join(rest, ",") != "10,20" {
		t.Fatalf("Vermerke nach dem Anheben: %v, erwartet 10,20", rest)
	}
	// Unlesbarer Wert: Fehler, kein Raten.
	if err := cs.setConfigValueDB(walVermerkAbKey, "zehn"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cs.walVermerkAbLesen(); err == nil {
		t.Fatal("unlesbare Untergrenze ohne Fehler gelesen")
	}
}

// Der Leser der Vermerke ueber mehrere Portionen und rueckwaerts.
func TestWALAusgang_LeserUeberPortionen_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "leser.wal"))
	untergrenzeZuruecksetzen(t, cs)
	// Vermerke fuer alle geraden Seqs bis 2*walVermerkPortion+10.
	if _, err := cs.db.Exec(`INSERT INTO wal_ausgang_vermerk (seq) SELECT g*2 FROM generate_series(1, $1) g`, walVermerkPortion+5); err != nil {
		t.Fatal(err)
	}
	a := cs.neuerWALAusgang(1, false, 0)
	for seq := uint64(1); seq <= 2*walVermerkPortion+12; seq++ {
		da, err := a.erledigt(seq)
		if err != nil {
			t.Fatal(err)
		}
		if soll := seq%2 == 0 && seq <= 2*(walVermerkPortion+5); da != soll {
			t.Fatalf("Seq %d: erledigt=%v, erwartet %v", seq, da, soll)
		}
	}
	if da, err := a.erledigt(4); err != nil || !da {
		t.Fatalf("rueckwaerts gefragt: %v %v", da, err)
	}
	if da, err := a.erledigt(5); err != nil || da {
		t.Fatalf("rueckwaerts gefragt (ungerade): %v %v", da, err)
	}
}

// Pruefung #332, MEDIUM-1: eine wal_seq ueber dem WAL-Kopf (Speicherung nach
// ausgebliebener Haltbarkeit, dann Absturz) haelt den Start an -- vor dem
// Wiederanlauf. VERWERFEN loest es, ohne sich im Kreis zu drehen (LOW-4).
func TestWALAusgang_WalSeqUeberDemKopfSperrt_RealDB(t *testing.T) {
	langerFlushTakt(t)
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "kopf.wal")
	a := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, a)
	s, r := distTestAddr(870), distTestAddr(871)
	seedConcurrentTestAccount(t, a, s, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, r, 10, time.Now().Unix())
	walUeberweisen(t, a, s, r, 1, "0xkopf1")
	a.FlushWALNow()
	walAbsturz(t, a)
	// r traegt eine Seq, die das WAL noch nicht vergeben hat.
	if _, err := a.db.Exec(`UPDATE chain_accounts SET wal_seq = 7 WHERE lower(address) = $1`, r); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEQUITAS_WAL_ENABLED", "1")
	t.Setenv("AEQUITAS_WAL_PATH", walPath)
	b := testKnoten(t, "unused-wal-test.json")
	if b.wal != nil {
		b.stopWALFlushWorkerForTest()
		b.wal.Close()
		t.Fatal("WAL geoeffnet, obwohl ein Konto eine nicht vergebene Seq traegt")
	}
	if !strings.Contains(b.walWiederanlaufFehler, "wal_seq 7") {
		t.Fatalf("Grund: %q", b.walWiederanlaufFehler)
	}
	err := b.PruefeWALRest()
	if err == nil {
		t.Fatal("Knoten startet")
	}
	// Bewusst verwerfen: dieser Prozess startet nicht (Wiederanlauf
	// gescheitert), der naechste schon -- kein Kreis.
	t.Setenv("AEQUITAS_WAL_REST_VERWERFEN", "1")
	if err := b.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "bitte neu starten") {
		t.Fatalf("Verwerfen: %v", err)
	}
	t.Setenv("AEQUITAS_WAL_REST_VERWERFEN", "")
	c := testKnoten(t, "unused-wal-test.json")
	if c.wal == nil {
		t.Fatalf("nach dem Verwerfen startet der Knoten nicht: %q", c.walWiederanlaufFehler)
	}
	c.stopWALFlushWorkerForTest()
	c.wal.Close()
}

// LOW-4: ebenso fremde Vermerke -- VERWERFEN loest auch sie.
func TestWALAusgang_VerwerfenLoestFremdeVermerke_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "fremd.wal")
	a := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, a)
	walAbsturz(t, a)
	if _, err := a.db.Exec(`INSERT INTO wal_ausgang_vermerk (seq) VALUES (5)`); err != nil {
		t.Fatal(err)
	}
	if err := a.setConfigValueDB(walVermerkAbKey, "9"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEQUITAS_WAL_ENABLED", "1")
	t.Setenv("AEQUITAS_WAL_PATH", walPath)
	b := testKnoten(t, "unused-wal-test.json")
	if b.wal != nil {
		t.Fatal("WAL geoeffnet trotz fremder Vermerke")
	}
	t.Setenv("AEQUITAS_WAL_REST_VERWERFEN", "0")
	if err := b.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "bitte neu starten") {
		t.Fatalf("Verwerfen: %v", err)
	}
	t.Setenv("AEQUITAS_WAL_REST_VERWERFEN", "")
	c := testKnoten(t, "unused-wal-test.json")
	if c.wal == nil {
		t.Fatalf("nach dem Verwerfen startet der Knoten nicht: %q", c.walWiederanlaufFehler)
	}
	c.stopWALFlushWorkerForTest()
	c.wal.Close()
}

// LOW-4: RESET_DB_STATE leert die Vermerke mit.
func TestWALAusgang_ResetLeertVermerke(t *testing.T) {
	body := functionBodyFromSource(t, "state.go", "func (cs *ChainState) resetDBStateForBootstrap(")
	if !strings.Contains(body, `"wal_ausgang_vermerk",`) {
		t.Fatal("resetDBStateForBootstrap leert wal_ausgang_vermerk nicht -- nach dem Reset sperrten alte Vermerke den Start")
	}
}

// MEDIUM-2 und die Haltbarkeit selbst: eine Speicherung wartet auf die
// Haltbarkeit ihrer WAL-Saetze; bleibt sie aus, schliesst der schnelle Pfad,
// und danach wartet keine Speicherung mehr. Ein Stapel wartet einmal.
func TestWALAusgang_SpeicherungWartetAufHaltbarkeit_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "haltbar.wal"))
	untergrenzeZuruecksetzen(t, cs)
	altFrist := walSeqHaltbarFrist
	walSeqHaltbarFrist = 150 * time.Millisecond
	walSchnellpfadDefekt.Store(false)
	t.Cleanup(func() {
		walSeqHaltbarFrist = altFrist
		walSchnellpfadDefekt.Store(false)
	})
	// Ein Stapel aus drei Konten, deren Saetze nie haltbar werden (Seq
	// ueber dem Kopf): einmal warten, nicht dreimal.
	var konten []*AccountState
	for i := 0; i < 3; i++ {
		addr := distTestAddr(872 + i)
		acc := seedConcurrentTestAccount(t, cs, addr, 10, time.Now().Unix())
		acc.WALSeq = cs.wal.PeekSeq() + 100
		konten = append(konten, acc)
	}
	cs.mu.Lock()
	t0 := time.Now()
	err := cs.saveAccountsToDBBatch(konten)
	dauer := time.Since(t0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if dauer < walSeqHaltbarFrist {
		t.Fatalf("Stapel wartete %v -- er muss auf die Haltbarkeit warten (Frist %v)", dauer, walSeqHaltbarFrist)
	}
	if dauer >= 2*walSeqHaltbarFrist {
		t.Fatalf("Stapel wartete %v -- einmal fuer den Stapel, nicht je Konto", dauer)
	}
	if !walSchnellpfadDefekt.Load() {
		t.Fatal("ausgebliebene Haltbarkeit schliesst den schnellen Pfad nicht")
	}
	// Danach wartet keine Speicherung mehr.
	cs.mu.Lock()
	t0 = time.Now()
	err = cs.saveAccountToDB(konten[0])
	dauer = time.Since(t0)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if dauer >= walSeqHaltbarFrist {
		t.Fatalf("nach geschlossenem Pfad wartete die Speicherung %v", dauer)
	}
}

// LOW-6 (M4): ein nachgeladenes Konto kommt mit WALSeq 0 in den Speicher;
// seine Speicherung darf wal_seq in Postgres nicht senken -- sonst buchte der
// naechste Wiederanlauf doppelt.
func TestWALAusgang_NachgeladenesKontoSenktWalSeqNicht_RealDB(t *testing.T) {
	for _, kalt := range []bool{false, true} {
		t.Run(fmt.Sprintf("kalt=%v", kalt), func(t *testing.T) {
			langerFlushTakt(t)
			truncateDistTestTables(t)
			cs := newWALTestState(t, filepath.Join(t.TempDir(), "nachgeladen.wal"))
			untergrenzeZuruecksetzen(t, cs)
			s, r := distTestAddr(876), distTestAddr(877)
			seedConcurrentTestAccount(t, cs, s, 100, time.Now().Unix())
			seedConcurrentTestAccount(t, cs, r, 0, time.Now().Unix())
			walUeberweisen(t, cs, s, r, 1, "0xnachgeladen")
			cs.FlushWALNow()
			cs.mu.Lock()
			acc, _ := cs.accounts.Get(s)
			acc.WALSeq = 0 // wie frisch aus Postgres geladen
			if kalt {
				acc.Version = 0
			}
			err := cs.saveAccountToDB(acc)
			cs.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			var seq int64
			cs.db.QueryRow(`SELECT wal_seq FROM chain_accounts WHERE lower(address) = $1`, s).Scan(&seq)
			if seq != 1 {
				t.Fatalf("wal_seq nach der Speicherung %d, erwartet 1 (nie abwaerts)", seq)
			}
		})
	}
}

// LOW-6 (M6): eine nachgeholte Zeile kommt in einen Block; ein weiterer Start
// schreibt sie nicht noch einmal (der Vermerk entstand mit ihr).
func TestWALAusgang_NachgeholtVerblocktNichtDoppelt_RealDB(t *testing.T) {
	langerFlushTakt(t)
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "nachgeholt.wal")
	a := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, a)
	s, r := distTestAddr(878), distTestAddr(879)
	seedConcurrentTestAccount(t, a, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, a, r, 0, time.Now().Unix())
	walUeberweisen(t, a, s, r, 1, "0xnachgeholt")
	walAbsturz(t, a)
	// Beide Salden tragen den Satz, die Zeile fehlt.
	if _, err := a.db.Exec(`UPDATE chain_accounts SET wal_seq = 1, balance = balance + CASE WHEN lower(address) = $2 THEN 1 ELSE -1.001 END WHERE lower(address) IN ($1, $2)`, s, r); err != nil {
		t.Fatal(err)
	}
	b := newWALTestState(t, walPath)
	b.FlushWALNow()
	if n := zeilenMitHash(t, b, "0xnachgeholt"); n != 1 {
		t.Fatalf("Vorbedingung: Zeile nachgeholt (%d)", n)
	}
	if _, err := b.db.Exec(`DELETE FROM pending_txs`); err != nil { // verblockt
		t.Fatal(err)
	}
	walAbsturz(t, b)
	c := newWALTestState(t, walPath)
	c.FlushWALNow()
	if n := zeilenMitHash(t, c, "0xnachgeholt"); n != 0 {
		t.Fatalf("%d Zeilen nach dem Verblocken -- die nachgeholte Ueberweisung kaeme doppelt in einen Block", n)
	}
}

// MEDIUM-3: Zeile ohne Vermerk (Rueckschritt auf eine Fassung ohne
// Vermerke), schon verblockt: steht sie im Block-Index, kein zweites Mal.
func TestWALAusgang_VerblocktOhneVermerkLautIndex_RealDB(t *testing.T) {
	langerFlushTakt(t)
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "rueckschritt.wal")
	a := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, a)
	s, r := distTestAddr(880), distTestAddr(881)
	seedConcurrentTestAccount(t, a, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, a, r, 0, time.Now().Unix())
	walUeberweisen(t, a, s, r, 1, "0xrueckschritt")
	a.FlushWALNow()
	walAbsturz(t, a)
	// Wie die alte Fassung: Zeile ohne Vermerk, verblockt und indiziert.
	a.ensureTxBlockIndexTable()
	for _, q := range []string{`TRUNCATE wal_ausgang_vermerk`, `DELETE FROM pending_txs`, `DELETE FROM chain_tx_block_index WHERE tx_hash = '0xrueckschritt'`} {
		if _, err := a.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.IndexBlockTransactions(1, "blk-rueckschritt", []Transaction{{Type: "transfer", TxHash: "0xrueckschritt"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.db.Exec(`DELETE FROM chain_tx_block_index WHERE tx_hash = '0xrueckschritt'`) })
	b := newWALTestState(t, walPath)
	b.FlushWALNow()
	if n := zeilenMitHash(t, b, "0xrueckschritt"); n != 0 {
		t.Fatalf("%d Zeilen fuer eine laut Index verblockte Ueberweisung", n)
	}
	var v int
	b.db.QueryRow(`SELECT count(*) FROM wal_ausgang_vermerk WHERE seq = 1`).Scan(&v)
	if v != 1 {
		t.Fatal("Vermerk nicht nachgetragen")
	}
}

// LOW-6 (M1/M2): die Kompaktierungsrunde hebt die Untergrenze auf genau die
// Kuerzungsgrenze -- nicht hoeher (sonst gaelte fuer offene Saetze die alte
// Regel) und auch, wenn die Datei zu klein zum Kuerzen ist.
func TestWALAusgang_KompaktierungHebtUntergrenze_RealDB(t *testing.T) {
	langerFlushTakt(t)
	truncateDistTestTables(t)
	cs := newWALTestState(t, filepath.Join(t.TempDir(), "kompakt.wal"))
	untergrenzeZuruecksetzen(t, cs)
	alt := walKompaktAbstand
	walKompaktAbstand = 1
	t.Cleanup(func() { walKompaktAbstand = alt })
	s, r := distTestAddr(882), distTestAddr(883)
	seedConcurrentTestAccount(t, cs, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, r, 0, time.Now().Unix())
	for i := 0; i < 4; i++ {
		walUeberweisen(t, cs, s, r, 1, fmt.Sprintf("0xkompakt%d", i))
	}
	cs.FlushWALNow()
	kopf := cs.wal.HeadSeq()
	soll := walKuerzenBis(kopf, 0, walKompaktAbstand, cs.walRecoveryFloor(), false, 0, 0, false)
	if soll == 0 || soll >= kopf {
		t.Fatalf("Vorbedingung: Grenze %d bei Kopf %d", soll, kopf)
	}
	cs.walKompaktierungsRunde(0)
	ab, da, err := cs.walVermerkAbLesen()
	if err != nil || !da || ab != soll {
		t.Fatalf("Untergrenze nach der Runde %d (da=%v err=%v), erwartet %d", ab, da, err, soll)
	}
	var unter int
	cs.db.QueryRow(`SELECT count(*) FROM wal_ausgang_vermerk WHERE seq < $1`, int64(soll)).Scan(&unter)
	if unter != 0 {
		t.Fatalf("%d Vermerke unter der Grenze nicht geloescht", unter)
	}
}
