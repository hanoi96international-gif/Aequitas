package keeper

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func korbTx(n int) Transaction {
	return Transaction{Type: "transfer", TxHash: fmt.Sprintf("0xk%d", n)}
}

func korbSeqs(e []korbEintrag) []uint64 {
	out := make([]uint64, len(e))
	for i, x := range e {
		out[i] = x.seq
	}
	return out
}

func gleicheSeqs(t *testing.T, was string, got []uint64, want ...uint64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %v, erwartet %v", was, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: %v, erwartet %v", was, got, want)
		}
	}
}

// Ueberholen sich zwei Ueberweisungen auf verschiedenen Konten, gibt der
// Korb trotzdem nur ein luckenloses Praefix frei -- sonst kaeme Seq 3 in
// einen Block, Seq 2 in den naechsten, und die Marke "bis 3" verloere 2.
func TestSpeicherkorb_GibtNurLueckenlosesPraefixFrei(t *testing.T) {
	k := neuerSpeicherKorb(1)
	k.hinzu(3, korbTx(3))
	k.hinzu(1, korbTx(1))
	if got := korbSeqs(k.nehmen(10, 100)); len(got) != 1 || got[0] != 1 {
		t.Fatalf("mit Luecke bei 2: %v, erwartet nur [1]", got)
	}
	k.hinzu(2, korbTx(2))
	gleicheSeqs(t, "nach Schliessen der Luecke", korbSeqs(k.nehmen(10, 100)), 2, 3)
}

// Nur Haltbares (fsync durch) darf in einen Block -- sonst stuende im Block,
// was ein Absturz noch verlieren kann.
func TestSpeicherkorb_NurHaltbaresInDenBlock(t *testing.T) {
	k := neuerSpeicherKorb(1)
	for s := uint64(1); s <= 5; s++ {
		k.hinzu(s, korbTx(int(s)))
	}
	gleicheSeqs(t, "haltbar bis 2", korbSeqs(k.nehmen(10, 2)), 1, 2)
	gleicheSeqs(t, "Deckel 2", korbSeqs(k.nehmen(2, 100)), 3, 4)
	if k.laenge() != 1 {
		t.Fatalf("Rest: %d, erwartet 1", k.laenge())
	}
}

func TestSpeicherkorb_ZurueckLegenHaeltReihenfolge(t *testing.T) {
	k := neuerSpeicherKorb(1)
	for s := uint64(1); s <= 4; s++ {
		k.hinzu(s, korbTx(int(s)))
	}
	e := k.nehmen(3, 100)
	k.zurueckLegen(e)
	gleicheSeqs(t, "nach zurueckLegen", korbSeqs(k.nehmen(10, 100)), 1, 2, 3, 4)
}

// Missbrauch/Wiederanlauf: dieselbe Seq zweimal (z. B. WAL-Nachspielen und
// Flush-Einreihen) darf nie zweimal in einen Block.
func TestSpeicherkorb_WiederholungKommtNichtDoppeltInDenBlock(t *testing.T) {
	k := neuerSpeicherKorb(1)
	k.hinzu(1, korbTx(1))
	k.hinzu(1, korbTx(1))
	k.hinzu(3, korbTx(3))
	k.hinzu(3, korbTx(3))
	k.hinzu(2, korbTx(2))
	gleicheSeqs(t, "ohne Doppelte", korbSeqs(k.nehmen(10, 100)), 1, 2, 3)
	k.hinzu(2, korbTx(2)) // schon verblockt
	if k.laenge() != 0 {
		t.Fatalf("verblockte Seq kam zurueck in den Korb")
	}
	if k.doppelt.Load() != 3 {
		t.Fatalf("doppelt = %d, erwartet 3", k.doppelt.Load())
	}
}

// Fehlt eine Seq laenger als die Frist, gibt der Korb nichts mehr her
// (fail-closed) -- statt hinter der Luecke weiterzumachen.
func TestSpeicherkorb_DauerhafteLueckeSperrt(t *testing.T) {
	alt := speicherKorbLueckeFrist
	t.Cleanup(func() { speicherKorbLueckeFrist = alt })
	k := neuerSpeicherKorb(1)
	k.hinzu(1, korbTx(1))
	k.hinzu(3, korbTx(3))
	speicherKorbLueckeFrist = time.Hour
	if got := k.nehmen(10, 100); len(got) != 1 {
		t.Fatalf("innerhalb der Frist: %v", korbSeqs(got))
	}
	k.zurueckLegen([]korbEintrag{{seq: 1, tx: korbTx(1)}})
	speicherKorbLueckeFrist = 0
	time.Sleep(time.Millisecond)
	if got := k.nehmen(10, 100); got != nil {
		t.Fatalf("nach der Frist gab der Korb noch %v her", korbSeqs(got))
	}
}

func zeile(id, seq int64) langsameZeile {
	return langsameZeile{id: id, walSeq: seq, tx: Transaction{Type: "registration", TxHash: fmt.Sprintf("0xz%d", id)}}
}

func blockReihe(txs []Transaction) []string {
	out := make([]string, len(txs))
	for i, t := range txs {
		out[i] = t.TxHash
	}
	return out
}

// Langsame Zeilen stehen an ihrer Seq-Stelle (bei Gleichstand vorn), und
// nur, wenn alle kleineren Schnellpfad-Seqs mit im Block sind.
func TestBlockKorbMischen_ReihenfolgeUndVoraussetzung(t *testing.T) {
	korb := []korbEintrag{{5, korbTx(5)}, {6, korbTx(6)}, {7, korbTx(7)}}
	zeilen := []langsameZeile{zeile(1, 6), zeile(2, 9), zeile(3, 5)}
	txs, genommen, ids, zurueck, freigeben, bis := blockKorbMischen(korb, zeilen, 100, 4)
	want := []string{"0xz3", "0xk5", "0xz1", "0xk6", "0xk7"}
	got := blockReihe(txs)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Block %v, erwartet %v", got, want)
	}
	if bis != 7 || len(genommen) != 3 || len(zurueck) != 0 {
		t.Fatalf("bis=%d genommen=%d zurueck=%d", bis, len(genommen), len(zurueck))
	}
	if fmt.Sprint(ids) != "[3 1]" || fmt.Sprint(freigeben) != "[2]" {
		t.Fatalf("ids=%v freigeben=%v (Zeile mit Seq 9 > bis+1 darf nicht hinein)", ids, freigeben)
	}
}

func TestBlockKorbMischen_DeckelSchneidetUndGibtZurueck(t *testing.T) {
	korb := []korbEintrag{{5, korbTx(5)}, {6, korbTx(6)}, {7, korbTx(7)}}
	zeilen := []langsameZeile{zeile(1, 6), zeile(3, 5)}
	txs, _, ids, zurueck, freigeben, bis := blockKorbMischen(korb, zeilen, 2, 4)
	if fmt.Sprint(blockReihe(txs)) != "[0xz3 0xk5]" || bis != 5 {
		t.Fatalf("Block %v bis=%d", blockReihe(txs), bis)
	}
	gleicheSeqs(t, "zurueck", korbSeqs(zurueck), 6, 7)
	if fmt.Sprint(ids) != "[3]" || fmt.Sprint(freigeben) != "[1]" {
		t.Fatalf("ids=%v freigeben=%v", ids, freigeben)
	}
}

// Leerer Korb: eine Zeile mit Seq bis+1 gehoert dazu (sie wurde vor der
// naechsten Schnellpfad-Ueberweisung angewendet); bis+2 muss warten.
func TestBlockKorbMischen_LeererKorb(t *testing.T) {
	txs, _, ids, _, freigeben, bis := blockKorbMischen(nil, []langsameZeile{zeile(1, 11), zeile(2, 12)}, 100, 10)
	if len(txs) != 1 || fmt.Sprint(ids) != "[1]" || fmt.Sprint(freigeben) != "[2]" || bis != 10 {
		t.Fatalf("txs=%d ids=%v freigeben=%v bis=%d", len(txs), ids, freigeben, bis)
	}
}

func TestWALKuerzenBis_NieUeberDieKorbMarke(t *testing.T) {
	// Ohne Korb wie bisher.
	if got := walKuerzenBis(1_000_000, 100, 300_000, 0, false, 0, 0, false); got != 699_900 {
		t.Fatalf("ohne Korb: %d", got)
	}
	// Mit Korb: nie ueber Marke+1, auch wenn Boden oder Abstand mehr erlaubten.
	if got := walKuerzenBis(1_000_000, 100, 300_000, 900_000, true, 500_000, 0, false); got != 500_001 {
		t.Fatalf("mit Korb: %d, erwartet 500001", got)
	}
	if got := walKuerzenBis(1_000_000, 100, 300_000, 0, true, 900_000, 0, false); got != 699_900 {
		t.Fatalf("Marke weit vorn: %d", got)
	}
}

// --- Mit echter Datenbank und echtem WAL (CI-Gruppe "wal") ---------------

func korbTestState(t *testing.T, walPath string, an bool) *ChainState {
	t.Helper()
	if an {
		t.Setenv(speicherKorbEnv, "1")
	} else {
		t.Setenv(speicherKorbEnv, "0")
	}
	return newWALTestState(t, walPath)
}

func korbUeberweisen(t *testing.T, cs *ChainState, from, to string, n int, praefix string) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, _, applied, err := cs.transferConcurrentWAL(from, to, 1, Transaction{Type: "transfer", Wallet: from, To: to, Amount: 1, TxHash: fmt.Sprintf("%s%d", praefix, i)})
		if !applied || err != nil {
			t.Fatalf("Ueberweisung %d: applied=%v err=%v", i, applied, err)
		}
	}
	if !cs.wal.WaitDurable(cs.wal.PeekSeq()-1, 5*time.Second) {
		t.Fatal("WAL wurde nicht haltbar")
	}
}

func korbAbsturz(t *testing.T, cs *ChainState) {
	t.Helper()
	cs.stopWALFlushWorkerForTest()
	if err := cs.wal.Close(); err != nil {
		t.Fatalf("WAL schliessen: %v", err)
	}
}

func TestWALSpeicherkorb_LadeOffeneZeilenOrdnetNachWALSeq(t *testing.T) {
	truncateDistTestTables(t)
	cs := korbTestState(t, filepath.Join(t.TempDir(), "t.wal"), false)
	// IDs in Flush-Reihenfolge, wal_seq in Anwendungsreihenfolge -- verschieden.
	for _, seq := range []int64{30, 10, 20} {
		if _, err := cs.db.Exec(`INSERT INTO pending_txs (tx_json, created_at, wal_seq) VALUES ($1, 0, $2)`,
			fmt.Sprintf(`{"type":"transfer","tx_hash":"0xs%d"}`, seq), seq); err != nil {
			t.Fatal(err)
		}
	}
	txs, _ := cs.LoadPendingTxsWithLimit(10)
	if fmt.Sprint(blockReihe(txs)) != "[0xs10 0xs20 0xs30]" {
		t.Fatalf("Reihenfolge %v -- der Block muss die Anwendungsreihenfolge (wal_seq) tragen, nicht die Flush-Reihenfolge (id)", blockReihe(txs))
	}
}

// Absturz VOR dem Speichern eines Blocks: alles Angenommene ist nach dem
// Neustart wieder im Korb, in derselben Reihenfolge, und nichts davon steht
// in pending_txs (sonst kaeme es doppelt in einen Block).
func TestWALSpeicherkorb_AbsturzVorDemBlock_NichtsVerloren(t *testing.T) {
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "t.wal")
	a := korbTestState(t, walPath, true)
	if a.korb == nil {
		t.Fatal("Korb nicht an")
	}
	from, to := distTestAddr(901), distTestAddr(902)
	seedConcurrentTestAccount(t, a, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, to, 0, time.Now().Unix())
	korbUeberweisen(t, a, from, to, 3, "0xvor")
	a.FlushWALNow()
	korbAbsturz(t, a)

	b := korbTestState(t, walPath, true)
	if b.korb == nil || b.korb.laenge() != 3 {
		t.Fatalf("nach dem Neustart %v im Korb, erwartet 3", b.SpeicherKorbStand())
	}
	txs, _, genommen, bis, _ := b.korbFuerBlock(100)
	b.korbFreigeben()
	if fmt.Sprint(blockReihe(txs)) != "[0xvor0 0xvor1 0xvor2]" || bis != genommen[len(genommen)-1].seq {
		t.Fatalf("Block %v bis=%d", blockReihe(txs), bis)
	}
	var zeilen int
	b.db.QueryRow(`SELECT count(*) FROM pending_txs`).Scan(&zeilen)
	if zeilen != 0 {
		t.Fatalf("%d Zeilen in pending_txs -- Korb-Ueberweisungen kaemen doppelt in einen Block", zeilen)
	}
}

// Absturz NACH dem Speichern: die Marke steht mit dem Block, das Verblockte
// kommt nicht wieder.
func TestWALSpeicherkorb_NachDemBlock_NichtDoppelt(t *testing.T) {
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "t.wal")
	a := korbTestState(t, walPath, true)
	from, to := distTestAddr(903), distTestAddr(904)
	seedConcurrentTestAccount(t, a, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, to, 0, time.Now().Unix())
	korbUeberweisen(t, a, from, to, 3, "0xnach")
	txs, ids, _, bis, gehalten := a.korbFuerBlock(100)
	if !gehalten {
		t.Fatal("Korb nicht gehalten")
	}
	if len(txs) != 3 {
		t.Fatalf("Block mit %d statt 3", len(txs))
	}
	blk := &Block{Hash: fmt.Sprintf("korbtest-%d", time.Now().UnixNano()), Height: 1, Transactions: txs, Timestamp: time.Now().Unix()}
	if err := a.SaveBlockMitKorb(blk, ids, bis); err != nil {
		t.Fatal(err)
	}
	a.korbBis.Store(bis)
	a.korbFreigeben()
	a.FlushWALNow()
	korbAbsturz(t, a)

	b := korbTestState(t, walPath, true)
	if b.korb.laenge() != 0 || b.korbBis.Load() != bis {
		t.Fatalf("nach dem Neustart: %v, erwartet leer und bis=%d", b.SpeicherKorbStand(), bis)
	}
	korbUeberweisen(t, b, from, to, 1, "0xneu")
	txs, _, _, _, _ = b.korbFuerBlock(100)
	if fmt.Sprint(blockReihe(txs)) != "[0xneu0]" {
		t.Fatalf("naechster Block %v -- nur die neue Ueberweisung", blockReihe(txs))
	}
}

// Ausschalten: was nur im Korb stand, kommt nach pending_txs -- genau
// einmal, auch bei wiederholtem Start.
func TestWALSpeicherkorb_AusschaltenHoltZeilenNach(t *testing.T) {
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "t.wal")
	a := korbTestState(t, walPath, true)
	from, to := distTestAddr(905), distTestAddr(906)
	seedConcurrentTestAccount(t, a, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, to, 0, time.Now().Unix())
	korbUeberweisen(t, a, from, to, 2, "0xaus")
	a.FlushWALNow() // Kontostaende in Postgres, keine Zeilen (Korb)
	korbAbsturz(t, a)

	b := korbTestState(t, walPath, false)
	if b.korb != nil {
		t.Fatal("Korb sollte aus sein")
	}
	b.FlushWALNow()
	zaehle := func(cs *ChainState) int {
		var n int
		cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' LIKE '0xaus%'`).Scan(&n)
		return n
	}
	if n := zaehle(b); n != 2 {
		t.Fatalf("%d Zeilen nach dem Ausschalten, erwartet 2", n)
	}
	if _, da, _ := b.speicherKorbMarkeLesen(); da {
		t.Fatal("Korb-Marke steht noch")
	}
	korbAbsturz(t, b)
	c := korbTestState(t, walPath, false)
	c.FlushWALNow()
	if n := zaehle(c); n != 2 {
		t.Fatalf("nach erneutem Start %d Zeilen, erwartet weiter 2", n)
	}
}

// Zwei Blockbauer gleichzeitig: der zweite bekommt nichts aus dem Korb.
func TestSpeicherkorb_NurEinBlockbauerGleichzeitig(t *testing.T) {
	cs := &ChainState{korb: neuerSpeicherKorb(1)}
	cs.korb.hinzu(1, korbTx(1))
	cs.korb.hinzu(2, korbTx(2))
	// Ohne WAL ist nichts haltbar -- hier nur die Sperre pruefen.
	_, _, _, _, erster := cs.korbFuerBlock(10)
	_, _, _, _, zweiter := cs.korbFuerBlock(10)
	if !erster || zweiter {
		t.Fatalf("erster=%v zweiter=%v -- der zweite darf den Korb nicht bekommen", erster, zweiter)
	}
	cs.korbFreigeben()
	if _, _, _, _, dritter := cs.korbFuerBlock(10); !dritter {
		t.Fatal("nach der Freigabe muss der Korb wieder zu haben sein")
	}
}

// Ausschalten, nachdem Ueberweisungen schon VERBLOCKT, ihre Kontostaende
// aber noch nicht in Postgres sind: sie duerfen keine Zeile in pending_txs
// bekommen -- sonst kaemen sie ein zweites Mal in einen Block.
func TestWALSpeicherkorb_AusschaltenNachBlockOhneFlush_NichtDoppelt(t *testing.T) {
	truncateDistTestTables(t)
	walPath := filepath.Join(t.TempDir(), "t.wal")
	a := korbTestState(t, walPath, true)
	from, to := distTestAddr(907), distTestAddr(908)
	seedConcurrentTestAccount(t, a, from, 1000, time.Now().Unix())
	seedConcurrentTestAccount(t, a, to, 0, time.Now().Unix())
	korbUeberweisen(t, a, from, to, 2, "0xverblockt")
	txs, ids, _, bis, _ := a.korbFuerBlock(100)
	blk := &Block{Hash: fmt.Sprintf("korbtest-%d", time.Now().UnixNano()), Height: 1, Transactions: txs, Timestamp: time.Now().Unix()}
	if err := a.SaveBlockMitKorb(blk, ids, bis); err != nil {
		t.Fatal(err)
	}
	// KEIN Flush: die Kontostaende stehen nur im WAL.
	korbAbsturz(t, a)

	b := korbTestState(t, walPath, false)
	b.FlushWALNow()
	var n int
	b.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE tx_json::json->>'tx_hash' LIKE '0xverblockt%'`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d Zeilen fuer schon verblockte Ueberweisungen -- sie kaemen doppelt in einen Block", n)
	}
	var saldo float64
	b.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, to).Scan(&saldo)
	if saldo != 2 {
		t.Fatalf("Empfaenger in Postgres %v, erwartet 2 (Kontostaende muessen trotzdem nachgezogen werden)", saldo)
	}
}

func TestAdressenAufteilen_DisjunktUndStabil(t *testing.T) {
	var adr []string
	for i := 0; i < 500; i++ {
		adr = append(adr, distTestAddr(2000+i))
	}
	g := adressenAufteilen(adr, 4)
	gesehen := map[string]int{}
	for i, grp := range g {
		for _, a := range grp {
			if _, schon := gesehen[a]; schon {
				t.Fatalf("%s in zwei Gruppen", a)
			}
			gesehen[a] = i
		}
	}
	if len(gesehen) != len(adr) {
		t.Fatalf("%d von %d Adressen verteilt", len(gesehen), len(adr))
	}
	g2 := adressenAufteilen(adr, 4)
	for i := range g {
		if fmt.Sprint(g[i]) != fmt.Sprint(g2[i]) {
			t.Fatal("Aufteilung nicht stabil")
		}
	}
}

func TestWALFlushTeileFuer_NurOhneOutbox(t *testing.T) {
	alt := walFlushTeileWert
	t.Cleanup(func() { walFlushTeileWert = alt })
	walFlushTeileWert = 4
	ohne := []walFlushItem{{ohneOutbox: true}, {ohneOutbox: true}}
	mit := []walFlushItem{{ohneOutbox: true}, {ohneOutbox: false}}
	if walFlushTeileFuer(ohne, 1000) != 4 {
		t.Fatal("ohne Outbox und genug Adressen: 4 Teile erwartet")
	}
	if walFlushTeileFuer(mit, 1000) != 1 {
		t.Fatal("mit Outbox-Zeile darf nicht geteilt werden (doppelte Zeilen bei Wiederholung)")
	}
	if walFlushTeileFuer(ohne, 10) != 1 {
		t.Fatal("wenige Adressen: nicht teilen")
	}
}

// Aufgeteilter Flush gegen echtes Postgres: alle Kontostaende stimmen mit dem
// Speicher ueberein, auch nach einer Wiederholung desselben Buendels.
func TestWALSpeicherkorb_AufgeteilterFlushSchreibtAlles(t *testing.T) {
	truncateDistTestTables(t)
	alt := walFlushTeileWert
	t.Cleanup(func() { walFlushTeileWert = alt })
	walFlushTeileWert = 4
	cs := korbTestState(t, filepath.Join(t.TempDir(), "t.wal"), true)
	// Ohne Hintergrund-Flush: sonst nimmt der Ticker waehrend des Einzahlens
	// Teile weg, und FlushWALNow bekommt ein Buendel unter der Teilungsgrenze.
	cs.stopWALFlushWorkerForTest()
	const paare = 60
	for i := 0; i < paare; i++ {
		seedConcurrentTestAccount(t, cs, distTestAddr(3000+2*i), 100, time.Now().Unix())
		seedConcurrentTestAccount(t, cs, distTestAddr(3001+2*i), 0, time.Now().Unix())
	}
	for i := 0; i < paare; i++ {
		korbUeberweisen(t, cs, distTestAddr(3000+2*i), distTestAddr(3001+2*i), 2, fmt.Sprintf("0xteil%d-", i))
	}
	vorher := walFlushTeileLaeufe.Load()
	cs.FlushWALNow()
	if walFlushTeileLaeufe.Load() == vorher {
		t.Fatal("Flush lief nicht aufgeteilt")
	}
	pruefe := func() {
		for i := 0; i < 2*paare; i++ {
			a := distTestAddr(3000 + i)
			acc, _ := cs.accounts.Get(a)
			var saldo float64
			var seq int64
			if err := cs.db.QueryRow(`SELECT balance, wal_seq FROM chain_accounts WHERE lower(address) = $1`, a).Scan(&saldo, &seq); err != nil {
				t.Fatalf("%s: %v", a, err)
			}
			if saldo != acc.Balance.Float() || uint64(seq) != acc.WALSeq {
				t.Fatalf("%s: Postgres %v/%d, Speicher %v/%d", a, saldo, seq, acc.Balance.Float(), acc.WALSeq)
			}
		}
	}
	pruefe()
	// Wiederholung (wie nach einem gescheiterten Teil): nichts aendert sich.
	var items []walFlushItem
	for i := 0; i < paare; i++ {
		items = append(items, walFlushItem{from: distTestAddr(3000 + 2*i), to: distTestAddr(3001 + 2*i), ohneOutbox: true})
	}
	if err := cs.flushWALBatch(items); err != nil {
		t.Fatal(err)
	}
	pruefe()
}
