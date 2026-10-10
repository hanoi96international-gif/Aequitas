package keeper

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Punkt 9 in docs/OFFEN.md (Pruefung von #331, Befund 1, und ein verwandter
// Fall): der WAL-Wiederanlauf entschied allein ueber wal_seq je Konto. Das
// heisst "Saldo enthaelt den Satz", aber nicht "Ausgangszeile geschrieben".

func walAusgangZeilen(t *testing.T, cs *ChainState, seq uint64) int {
	t.Helper()
	var n int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE wal_seq = $1`, int64(seq)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// (a) Eine WAL-Ueberweisung S->R ist angenommen, noch nicht geflusht. Ein
// serieller Pfad speichert S (Saldo samt WAL-Abbuchung), dann Absturz. Der
// Wiederanlauf buchte die Abbuchung ein zweites Mal ab (PoC der Pruefung:
// 2104,1 -> 1203,2). Erwartet: S einmal belastet, R gutgeschrieben, genau
// eine Ausgangszeile.
func TestWALAusgang_SerielleSpeicherungKeineDoppelbuchung_RealDB(t *testing.T) {
	// Beide Wege von saveAccountToDBCtx: optimistisches UPDATE (Version
	// bekannt) und INSERT ... ON CONFLICT (Version 0, etwa ein frisch
	// aufgebautes AccountState fuer eine bestehende Zeile).
	for _, einfuegen := range []bool{false, true} {
		t.Run(map[bool]string{false: "update", true: "einfuegen"}[einfuegen], func(t *testing.T) {
			serielleSpeicherung(t, einfuegen)
		})
	}
}

func serielleSpeicherung(t *testing.T, einfuegen bool) {
	walPath := filepath.Join(t.TempDir(), "seriell.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r := distTestAddr(850), distTestAddr(851)
	seedConcurrentTestAccount(t, csA, s, 3000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(s, r, 900, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 900, TxHash: "0xwalausgang-seriell"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: applied=%v err=%v", applied, err)
	}
	csA.stopWALFlushWorkerForTest()
	erwartetS := speicherStand(t, csA, s)
	// Ein serieller Pfad (unter cs.mu.Lock) schreibt S nach Postgres.
	csA.mu.Lock()
	acc, _ := csA.accounts.Get(s)
	if einfuegen {
		acc.Version = 0
	}
	if err := csA.saveAccountToDB(acc); err != nil {
		csA.mu.Unlock()
		t.Fatal(err)
	}
	csA.mu.Unlock()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	if d := dbStand(t, csA, s); d != erwartetS.Float() {
		t.Fatalf("Vorbedingung: serielle Speicherung schrieb %v, erwartet %s", d, erwartetS)
	}

	csB := newWALTestState(t, walPath)
	if b := speicherStand(t, csB, s); b != erwartetS {
		t.Fatalf("Absender nach dem Wiederanlauf %s, erwartet %s (doppelt abgebucht?)", b, erwartetS)
	}
	if b := speicherStand(t, csB, r); b.Float() != 900 {
		t.Fatalf("Empfaenger %s, erwartet 900", b)
	}
	csB.FlushWALNow()
	if n := walAusgangZeilen(t, csB, 1); n != 1 {
		t.Fatalf("%d Ausgangszeilen fuer Satz 1, erwartet 1", n)
	}
}

// (b) Zwei WAL-Ueberweisungen S->R (Satz 1 und 2). Ein Flush-Stapel traegt
// nur Satz 1, schreibt aber den aktuellen Stand beider Konten (wal_seq 2).
// Absturz vor dem naechsten Stapel: der Wiederanlauf hielt Satz 2 fuer
// angewandt und schrieb keine Ausgangszeile -- der Transfer fehlte dem Netz.
func TestWALAusgang_AbsturzZwischenStapelnKeineFehlendeZeile_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "stapel.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r := distTestAddr(852), distTestAddr(853)
	seedConcurrentTestAccount(t, csA, s, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	for i, h := range []string{"0xwalausgang-stapel1", "0xwalausgang-stapel2"} {
		if _, _, applied, err := csA.transferConcurrentWAL(s, r, 10, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 10, TxHash: h}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	csA.stopWALFlushWorkerForTest()
	csA.walFlushMu.Lock()
	if len(csA.walFlushQueue) != 2 {
		csA.walFlushMu.Unlock()
		t.Fatalf("Vorbedingung: %d Eintraege in der Warteschlange, erwartet 2", len(csA.walFlushQueue))
	}
	erster := csA.walFlushQueue[0]
	csA.walFlushMu.Unlock()
	if erster.seq != 1 {
		t.Fatalf("Vorbedingung: erster Eintrag Satz %d", erster.seq)
	}
	if err := csA.flushWALBatch([]walFlushItem{erster}); err != nil {
		t.Fatal(err)
	}
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	if walAusgangZeilen(t, csA, 1) != 1 || walAusgangZeilen(t, csA, 2) != 0 {
		t.Fatal("Vorbedingung: nur Satz 1 hat eine Ausgangszeile")
	}

	csB := newWALTestState(t, walPath)
	csB.FlushWALNow()
	if n := walAusgangZeilen(t, csB, 2); n != 1 {
		t.Fatalf("%d Ausgangszeilen fuer Satz 2 nach dem Wiederanlauf, erwartet 1 (Transfer fehlt dem Netz)", n)
	}
	if n := walAusgangZeilen(t, csB, 1); n != 1 {
		t.Fatalf("%d Ausgangszeilen fuer Satz 1, erwartet 1 (doppelt?)", n)
	}
	if b := speicherStand(t, csB, s); b.Float() >= 1000-20 {
		t.Fatalf("Absender %s", b)
	}
}

// Wie (a), aber der serielle Pfad speichert BEIDE Konten. Seit sie wal_seq
// mitschreiben, halten beide den Satz fuer angewandt -- die Ausgangszeile
// muss trotzdem nachgeholt werden (sie stand nie in Postgres).
func TestWALAusgang_BeideKontenSeriellZeileWirdNachgeholt_RealDB(t *testing.T) {
	for _, stapel := range []bool{false, true} {
		t.Run(map[bool]string{false: "einzeln", true: "stapel"}[stapel], func(t *testing.T) {
			beideKontenSeriell(t, stapel)
		})
	}
}

// beideKontenSeriell: einzeln ueber saveAccountToDB oder zusammen ueber
// saveAccountsToDBBatch -- beide schreiben wal_seq mit.
func beideKontenSeriell(t *testing.T, stapel bool) {
	walPath := filepath.Join(t.TempDir(), "seriell-beide.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r := distTestAddr(854), distTestAddr(855)
	seedConcurrentTestAccount(t, csA, s, 3000, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(s, r, 900, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 900, TxHash: "0xwalausgang-beide"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: applied=%v err=%v", applied, err)
	}
	csA.stopWALFlushWorkerForTest()
	erwartetS, erwartetR := speicherStand(t, csA, s), speicherStand(t, csA, r)
	csA.mu.Lock()
	if stapel {
		accS, _ := csA.accounts.Get(s)
		accR, _ := csA.accounts.Get(r)
		if err := csA.saveAccountsToDBBatch([]*AccountState{accS, accR}); err != nil {
			csA.mu.Unlock()
			t.Fatal(err)
		}
	} else {
		for _, a := range []string{s, r} {
			acc, _ := csA.accounts.Get(a)
			if err := csA.saveAccountToDB(acc); err != nil {
				csA.mu.Unlock()
				t.Fatal(err)
			}
		}
	}
	csA.mu.Unlock()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}

	csB := newWALTestState(t, walPath)
	if b := speicherStand(t, csB, s); b != erwartetS {
		t.Fatalf("Absender %s, erwartet %s", b, erwartetS)
	}
	if b := speicherStand(t, csB, r); b != erwartetR {
		t.Fatalf("Empfaenger %s, erwartet %s", b, erwartetR)
	}
	csB.FlushWALNow()
	if n := walAusgangZeilen(t, csB, 1); n != 1 {
		t.Fatalf("%d Ausgangszeilen fuer Satz 1, erwartet 1 (Transfer fehlt dem Netz)", n)
	}
}

// Der Fall aus dem 2. Pruefdurchgang von #331: seq1 S->R, seq2 S->X, seq3
// X->U. Nur die Stapel von seq1 und seq3 committen -- sie schreiben S und X
// mit dem aktuellen WALSeq und decken seq2 auf beiden Seiten. Absturz. seq2
// fehlte dem Netz. Erwartet: jede Seq genau eine Ausgangszeile, auch nach
// einem zweiten Wiederanlauf.
func TestWALAusgang_FremdeStapelDeckenSatz_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "fremde-stapel.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r, x, u := distTestAddr(856), distTestAddr(857), distTestAddr(858), distTestAddr(859)
	seedConcurrentTestAccount(t, csA, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, x, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, u, 0, time.Now().Unix())
	for i, tr := range [][2]string{{s, r}, {s, x}, {x, u}} {
		if _, _, applied, err := csA.transferConcurrentWAL(tr[0], tr[1], 10, Transaction{Type: "transfer", Wallet: tr[0], To: tr[1], Amount: 10, TxHash: "0xwalausgang-fremd-" + string(rune('1'+i))}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	csA.stopWALFlushWorkerForTest()
	csA.walFlushMu.Lock()
	q := append([]walFlushItem(nil), csA.walFlushQueue...)
	csA.walFlushMu.Unlock()
	if len(q) != 3 || q[0].seq != 1 || q[2].seq != 3 {
		t.Fatalf("Vorbedingung: Warteschlange %v", q)
	}
	for _, it := range []walFlushItem{q[0], q[2]} {
		if err := csA.flushWALBatch([]walFlushItem{it}); err != nil {
			t.Fatal(err)
		}
	}
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	if walAusgangZeilen(t, csA, 2) != 0 {
		t.Fatal("Vorbedingung: seq2 ohne Ausgangszeile")
	}
	for runde := 1; runde <= 2; runde++ {
		cs := newWALTestState(t, walPath)
		cs.FlushWALNow()
		for seq := uint64(1); seq <= 3; seq++ {
			if n := walAusgangZeilen(t, cs, seq); n != 1 {
				t.Fatalf("Runde %d: %d Ausgangszeilen fuer seq%d, erwartet 1", runde, n, seq)
			}
		}
		cs.stopWALFlushWorkerForTest()
		cs.wal.Close()
	}
}

// Der Flush vermerkt jede Ausgangszeile, und gekuerzt wird nur unter der
// Grenze (Kompaktierung, Untergrenze).
func TestWALAusgang_VermerkUndKuerzen_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "vermerk.wal")
	truncateDistTestTables(t)
	cs := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, cs)
	s, r := distTestAddr(860), distTestAddr(861)
	seedConcurrentTestAccount(t, cs, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, cs, r, 0, time.Now().Unix())
	for i := 0; i < 3; i++ {
		if _, _, applied, err := cs.transferConcurrentWAL(s, r, 1, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 1, TxHash: "0xwalausgang-vermerk-" + string(rune('1'+i))}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	cs.FlushWALNow()
	m, err := cs.walAusgangVermerke(0)
	if err != nil || len(m) != 3 || !m.hat(1) || !m.hat(3) || m.hat(4) {
		t.Fatalf("Vermerke nach dem Flush: %v, %v", m, err)
	}
	if err := cs.walAusgangKuerzen(3); err != nil {
		t.Fatal(err)
	}
	if m, _ := cs.walAusgangVermerke(0); len(m) != 1 || !m.hat(3) {
		t.Fatalf("nach dem Kuerzen unter 3: %v", m)
	}
	// Grenze: mehr Vermerke als erlaubt -> der Wiederanlauf scheitert
	// (fail-closed), statt den Speicher zu sprengen.
	alt := walAusgangHoechstens
	walAusgangHoechstens = 0
	if _, err := cs.walAusgangVermerke(0); err == nil {
		t.Fatal("Grenze der Vermerke greift nicht")
	}
	walAusgangHoechstens = alt
	// Untergrenze (Zustandsersatz): alles bis zum Kopf faellt weg.
	cs.markWALSupersededByStateReplacement()
	if m, _ := cs.walAusgangVermerke(0); len(m) != 0 {
		t.Fatalf("nach dem Setzen der Untergrenze noch %v", m)
	}
}

// Uebergang: der erste Wiederanlauf mit dieser Fassung (keine Grenze in
// chain_config) wendet fuer alle Saetze der Datei die alte Regel an -- sie
// haben keinen Vermerk, auch wenn ihre Zeile laengst in einem Block steht --
// und setzt die Grenze auf den hoechsten Satz. Ein Satz, dessen Zeile laut
// alter Regel steht (beide Konten geflusht), bekommt keine zweite.
func TestWALAusgang_UebergangAlteSaetzeKeineZweiteZeile_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "uebergang.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r := distTestAddr(862), distTestAddr(863)
	seedConcurrentTestAccount(t, csA, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(s, r, 5, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 5, TxHash: "0xwalausgang-alt"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: applied=%v err=%v", applied, err)
	}
	csA.FlushWALNow()
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	// So saehe es nach einer frueheren Fassung aus: Zeile verblockt
	// (geloescht), kein Vermerk, keine Grenze.
	for _, q := range []string{`DELETE FROM pending_txs WHERE wal_seq = 1`, `DELETE FROM wal_ausgang`, `DELETE FROM chain_config WHERE key = '` + walAusgangAbKey + `'`} {
		if _, err := csA.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	csB := newWALTestState(t, walPath)
	csB.FlushWALNow()
	if n := walAusgangZeilen(t, csB, 1); n != 0 {
		t.Fatalf("alter, verblockter Satz bekam %d neue Ausgangszeile(n)", n)
	}
	if ab, da, err := csB.walAusgangAb(); err != nil || !da || ab != 1 {
		t.Fatalf("Uebergangsgrenze %d (gesetzt %v, %v), erwartet 1", ab, da, err)
	}
}

// Pruefung von #331, 3. Durchgang, INFO-2: scheitert der Wiederanlauf (hier
// am zweiten Satz), schreibt kein Flush-Arbeiter den schon eingespielten
// ersten nach Postgres -- der Arbeiter startet erst nach gelungenem Init.
func TestWALAusgang_GescheiterterWiederanlaufSchreibtNichts_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "gescheitert-teil.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r, q := distTestAddr(864), distTestAddr(865), distTestAddr(866)
	seedConcurrentTestAccount(t, csA, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, q, 0, time.Now().Unix())
	for i, z := range []string{r, q} {
		if _, _, applied, err := csA.transferConcurrentWAL(s, z, 10, Transaction{Type: "transfer", Wallet: s, To: z, Amount: 10, TxHash: "0xwalausgang-teil-" + string(rune('1'+i))}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := csA.db.Exec(`DELETE FROM chain_accounts WHERE lower(address) = $1`, q); err != nil {
		t.Fatal(err)
	}
	cs := testKnoten(t, "unused-wal-test.json") // AEQUITAS_WAL_* noch gesetzt
	t.Cleanup(cs.stopWALFlushWorkerForTest)
	if cs.wal != nil || cs.walWiederanlaufFehler == "" {
		t.Fatalf("Vorbedingung: Wiederanlauf gescheitert (wal=%v, Fehler %q)", cs.wal != nil, cs.walWiederanlaufFehler)
	}
	time.Sleep(400 * time.Millisecond) // laenger als ein Flush-Takt
	if d := dbStand(t, cs, s); d != 100 {
		t.Fatalf("halb eingespielter Rest nach Postgres geschrieben: Absender %v, erwartet 100", d)
	}
	if n := walAusgangZeilen(t, cs, 1); n != 0 {
		t.Fatalf("%d Ausgangszeilen nach gescheitertem Wiederanlauf", n)
	}
	if err := cs.PruefeWALRest(); err == nil {
		t.Fatal("gescheiterter Wiederanlauf: Knoten startet")
	}
}

// Die Startsperre zaehlt einen Satz ohne Ausgangszeile als offen, auch wenn
// beide Salden ihn enthalten (Fall b) -- ein Beobachter holte die Zeile nie
// nach. Bis zur Korb-Marke steht er in einem Block und zaehlt nicht.
func TestWALAusgang_SperreZaehltSatzOhneZeile_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "sperre-ohne-zeile.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r := distTestAddr(867), distTestAddr(868)
	seedConcurrentTestAccount(t, csA, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	for i := 0; i < 2; i++ {
		if _, _, applied, err := csA.transferConcurrentWAL(s, r, 10, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 10, TxHash: "0xwalausgang-sperre-" + string(rune('1'+i))}); !applied || err != nil {
			t.Fatalf("Vorbedingung %d: applied=%v err=%v", i, applied, err)
		}
	}
	csA.stopWALFlushWorkerForTest()
	csA.walFlushMu.Lock()
	erster := csA.walFlushQueue[0]
	csA.walFlushMu.Unlock()
	if err := csA.flushWALBatch([]walFlushItem{erster}); err != nil {
		t.Fatal(err)
	}
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	setzeBeobachterFuerTest(t, true)
	t.Setenv("AEQUITAS_WAL_ENABLED", "")
	cs := testKnoten(t, "unused-wal-test.json")
	konten, ohneZeile, _, err := cs.walRestPruefen(walPath)
	if err != nil || konten != 0 || ohneZeile != 1 {
		t.Fatalf("Rest: %d Konten, %d ohne Zeile, %v -- erwartet 0 und 1", konten, ohneZeile, err)
	}
	if err := cs.PruefeWALRest(); err == nil || !strings.Contains(err.Error(), "1 Saetze ohne Ausgangszeile") {
		t.Fatalf("Beobachter mit Satz ohne Ausgangszeile: %v", err)
	}
	if err := cs.setConfigValueDB(speicherKorbMarke, "2"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, speicherKorbMarke) })
	if err := cs.PruefeWALRest(); err != nil {
		t.Fatalf("bis zur Korb-Marke verblockt, trotzdem gesperrt: %v", err)
	}
}

// Nach gelungenem Wiederanlauf startet der Flush-Arbeiter von selbst und
// schreibt den eingespielten Rest -- ohne manuellen Flush und ohne neue
// Ueberweisung, die ihn sonst anstiesse.
func TestWALAusgang_ArbeiterStartetNachGelungenemWiederanlauf_RealDB(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "arbeiter-start.wal")
	truncateDistTestTables(t)
	csA := newWALTestState(t, walPath)
	untergrenzeZuruecksetzen(t, csA)
	s, r := distTestAddr(869), distTestAddr(870)
	seedConcurrentTestAccount(t, csA, s, 100, time.Now().Unix())
	seedConcurrentTestAccount(t, csA, r, 0, time.Now().Unix())
	if _, _, applied, err := csA.transferConcurrentWAL(s, r, 10, Transaction{Type: "transfer", Wallet: s, To: r, Amount: 10, TxHash: "0xwalausgang-arbeiter"}); !applied || err != nil {
		t.Fatalf("Vorbedingung: applied=%v err=%v", applied, err)
	}
	csA.stopWALFlushWorkerForTest()
	if err := csA.wal.Close(); err != nil {
		t.Fatal(err)
	}
	cs := newWALTestState(t, walPath)
	if cs.walWiederanlaufLaeuft.Load() {
		t.Fatal("Merker des Wiederanlaufs nach Erfolg noch gesetzt")
	}
	frist := time.Now().Add(3 * time.Second)
	for dbStand(t, cs, r) != 10 || walAusgangZeilen(t, cs, 1) != 1 {
		if time.Now().After(frist) {
			t.Fatalf("eingespielter Rest nach 3 s nicht geschrieben: Empfaenger %v, Zeilen %d", dbStand(t, cs, r), walAusgangZeilen(t, cs, 1))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
