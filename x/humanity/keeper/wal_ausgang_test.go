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
