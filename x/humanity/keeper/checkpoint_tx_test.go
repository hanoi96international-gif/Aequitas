package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func checkpointTxAn(t *testing.T) {
	t.Helper()
	checkpointTxOverride.Store(1)
	t.Cleanup(func() { checkpointTxOverride.Store(0) })
}

// zertFuer: gueltiges Zertifikat (drei von vier) fuer Block b.
func (f *vorschlagFall) zertFuer(t *testing.T, b *Block) CheckpointZertifikat {
	t.Helper()
	c := f.vorschlag(t, b)
	z := CheckpointZertifikat{KettenCheckpoint: c}
	for _, k := range f.keys[:3] {
		u, err := checkpointUnterschreiben(c, k)
		if err != nil {
			t.Fatal(err)
		}
		z.Unterschriften = append(z.Unterschriften, u)
	}
	zertifikatOrdnen(&z)
	return z
}

func TestCheckpointTx_SchlafendUndForm(t *testing.T) {
	if checkpointTxAktiv(1 << 40) {
		t.Fatal("ohne Override aktiv")
	}
	if bekannteTxArt(checkpointTxArt, 1<<40) {
		t.Fatal("vor dem Stichtag bekannt -- ein Block damit wuerde angenommen")
	}
	checkpointTxAn(t)
	if !bekannteTxArt(checkpointTxArt, 100) {
		t.Fatal("nach dem Stichtag unbekannt")
	}
	f := neuerVorschlagFall(t, false)
	tx := neueCheckpointTx(f.zertFuer(t, f.blocks[1]))
	if err := checkpointTxForm(&tx); err != nil {
		t.Fatal(err)
	}
	mitWallet := tx
	mitWallet.Wallet = f.adr[0]
	mitBetrag := tx
	mitBetrag.Amount = 1
	andererHash := tx
	andererHash.TxHash = "0x" + strings.Repeat("0", 64)
	ohne := tx
	ohne.Checkpoint = nil
	falscheArt := tx
	falscheArt.Type = "transfer"
	for name, x := range map[string]Transaction{"Wallet": mitWallet, "Betrag": mitBetrag, "TxHash": andererHash, "ohne Zertifikat": ohne, "Art": falscheArt} {
		x := x
		if checkpointTxForm(&x) == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
}

func TestLetzterCheckpointLesen(t *testing.T) {
	c := KettenCheckpoint{Zeit: 1800000060, Hoehe: 501, Hash: cpHash(7)}
	w := letzterCheckpointWert(c)
	got, err := letzterCheckpointLesen(w)
	if err != nil || got.Zeit != c.Zeit || got.Hoehe != c.Hoehe || got.Hash != c.Hash {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := letzterCheckpointLesen(""); got != nil || err != nil {
		t.Fatal("leer")
	}
	for _, w := range []string{"1:2", "x:2:" + cpHash(1), "01:2:" + cpHash(1), "1:2:" + strings.ToUpper(cpHash(0xabcdef)), "0:2:" + cpHash(1), "1:2:" + cpHash(1) + ":x"} {
		if _, err := letzterCheckpointLesen(w); err == nil {
			t.Fatalf("%q angenommen", w)
		}
		if pruefeSnapshotKonfig(letzterCheckpointSchluessel, w) == nil {
			t.Fatalf("Snapshot mit %q angenommen", w)
		}
	}
}

// Ohne Checkpoint bleibt die StateRoot byte-gleich zur bisherigen Formel.
func TestStateRoot_OhneCheckpointUnveraendert(t *testing.T) {
	cs := &ChainState{}
	null := strings.Repeat("00", 32)
	h := sha256.Sum256([]byte("acctXOR:" + null + "|nullXOR:" + null + "|ubi:123"))
	if got := cs.stateRootLocked("123", ""); got != hex.EncodeToString(h[:]) {
		t.Fatalf("StateRoot ohne Checkpoint geaendert: %s", got)
	}
	if cs.stateRootLocked("123", "1:2:"+cpHash(1)) == cs.stateRootLocked("123", "") ||
		cs.stateRootLocked("123", "1:2:"+cpHash(1)) == cs.stateRootLocked("123", "1:2:"+cpHash(2)) {
		t.Fatal("Checkpoint geht nicht in die StateRoot ein")
	}
	found := false
	for _, k := range stateRootRelevantConfigKeys {
		found = found || k == letzterCheckpointSchluessel
	}
	if !found {
		t.Fatal("letzter Checkpoint nicht im Rueckrollen")
	}
}

// Nachspielen mit Datenbank: gueltige Kette, Reihenfolge ueberspringt,
// Faelschung macht den Block ungueltig, Luecke ist nicht pruefbar.
func TestApplyCheckpointTx_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	checkpointTxAn(t)
	ctx := context.Background()
	b0, b1, b2 := f.blocks[0], f.blocks[1], f.blocks[2]
	// tragende Bloecke: Eltern = der Checkpoint-Block selbst, Zeit danach.
	apply := func(z CheckpointZertifikat, eltern string, zeit int64, lies kopfLeser) error {
		tx := neueCheckpointTx(z)
		return f.dag.applyCheckpointTxLocked(ctx, &tx, []string{eltern}, zeit, lies)
	}
	lesen := func() string { return f.dag.state.getConfigValueDB(letzterCheckpointSchluessel) }
	if _, err := f.dag.state.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}

	// Vor dem Stichtag: ungueltig.
	checkpointTxOverride.Store(1 << 40)
	if err := apply(f.zertFuer(t, b1), b1.Hash, b1.Timestamp+200, f.koepfe.leser()); err == nil || istZustandsAblehnung(err) {
		t.Fatalf("vor dem Stichtag: %v", err)
	}
	checkpointTxOverride.Store(1)

	// Gefaelschte Unterschrift: Block ungueltig, nichts gesetzt.
	kaputt := f.zertFuer(t, b1)
	kaputt.Unterschriften[0].Sig = kaputt.Unterschriften[1].Sig
	if err := apply(kaputt, b1.Hash, b1.Timestamp+200, f.koepfe.leser()); err == nil || istZustandsAblehnung(err) {
		t.Fatalf("Faelschung: %v", err)
	}
	// Checkpoint-Block nicht in der Vergangenheit des tragenden.
	if err := apply(f.zertFuer(t, b2), b1.Hash, b2.Timestamp+200, f.koepfe.leser()); err == nil || istZustandsAblehnung(err) {
		t.Fatalf("nicht in der Vergangenheit: %v", err)
	}
	// Luecke: nicht pruefbar.
	luecke := &speicherKette{k: map[string]blockKopf{}}
	if err := apply(f.zertFuer(t, b1), b1.Hash, b1.Timestamp+200, luecke.leser()); !errors.Is(err, errCheckpointNichtPruefbar) {
		t.Fatalf("Luecke: %v", err)
	}
	if lesen() != "" {
		t.Fatalf("nach Ablehnungen gesetzt: %q", lesen())
	}

	// Gueltig: b1, dann b2 (Nachfolger).
	if err := apply(f.zertFuer(t, b1), b1.Hash, b1.Timestamp+200, f.koepfe.leser()); err != nil {
		t.Fatal(err)
	}
	if lesen() != letzterCheckpointWert(f.vorschlag(t, b1)) {
		t.Fatalf("letzter: %q", lesen())
	}
	// Aelter (b0) und derselbe nochmal: uebersprungen.
	for _, b := range []*Block{b0, b1} {
		if err := apply(f.zertFuer(t, b), b.Hash, b1.Timestamp+200, f.koepfe.leser()); !istZustandsAblehnung(err) {
			t.Fatalf("nicht juenger: %v", err)
		}
	}
	// Zu alt fuer den tragenden Block: uebersprungen (nicht ungueltig).
	if err := apply(f.zertFuer(t, b2), b2.Hash, b2.Timestamp+checkpointHoechstensAlt+1, f.koepfe.leser()); !istZustandsAblehnung(err) {
		t.Fatalf("zu alt: %v", err)
	}
	// Seitenblock (Eltern b0, nicht Nachfolger von b1): uebersprungen.
	if err := apply(f.zertFuer(t, f.seite), f.seite.Hash, f.seite.Timestamp+200, f.koepfe.leser()); !istZustandsAblehnung(err) {
		t.Fatalf("kein Nachfolger: %v", err)
	}
	if err := apply(f.zertFuer(t, b2), b2.Hash, b2.Timestamp+200, f.koepfe.leser()); err != nil {
		t.Fatal(err)
	}
	if lesen() != letzterCheckpointWert(f.vorschlag(t, b2)) {
		t.Fatalf("letzter: %q", lesen())
	}
	// Er geht in die StateRoot ein.
	if !strings.Contains(f.dag.state.StateRootComponentBreakdown().LetzterCheckpoint, b2.Hash) {
		t.Fatal("letzter Checkpoint nicht in den StateRoot-Bestandteilen")
	}
}

// Leiter: vor dem Stichtag traegt er nichts ein; danach legt er die
// Transaktion in den Ausgang, OHNE den Zustand zu aendern -- der gilt erst
// mit dem eigenen Block, nach der Regel des Nachspielens (Stichtag-Sperre
// 3): zu alt fuer den eigenen Block heisst unveraendert, wie bei jedem
// anderen.
func TestCheckpointTxEinreichen_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	f.dag.state.annehmendAusdruecklich.Store(true)
	t.Cleanup(func() { f.dag.state.annehmendAusdruecklich.Store(false) })
	alt := checkpointReplayLeser
	checkpointReplayLeser = func(sqlExecutor) kopfLeser { return f.koepfe.leser() }
	t.Cleanup(func() { checkpointReplayLeser = alt })
	if _, err := f.dag.state.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	lesen := func() string { return f.dag.state.getConfigValueDB(letzterCheckpointSchluessel) }
	z1, z2 := f.zertFuer(t, f.blocks[1]), f.zertFuer(t, f.blocks[2])
	if err := f.dag.checkpointTxEinreichen(z2, z2.Zeit+200); err != nil {
		t.Fatal(err)
	}
	var n int
	if f.dag.state.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE tx_json LIKE '%checkpoint_zertifikat%'`).Scan(&n); n != 0 {
		t.Fatalf("vor dem Stichtag im Ausgang: %d", n)
	}
	checkpointTxAn(t)
	if err := f.dag.checkpointTxEinreichen(z2, z2.Zeit+200); err != nil {
		t.Fatal(err)
	}
	if lesen() != "" {
		t.Fatalf("beim Einreichen gesetzt: %q", lesen())
	}
	if err := f.dag.state.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE tx_json LIKE '%checkpoint_zertifikat%'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("Ausgang: %d %v", n, err)
	}
	tx := neueCheckpointTx(z2)
	// Eigener Block zu spaet: unveraendert (wie jeder Nachspielende).
	zuSpaet := &Block{Height: 960, Timestamp: z2.Zeit + checkpointHoechstensAlt + 1, ParentHashes: []string{z2.Hash}, Transactions: []Transaction{tx}}
	v := f.dag.checkpointVorschau(zuSpaet.Transactions, zuSpaet.ParentHashes, zuSpaet.Timestamp)
	if v.geaendert || v.unklar || v.index != 0 {
		t.Fatalf("zu alter eigener Block: %+v", v)
	}
	if v.geaendert {
		t.Fatal("zu alt und trotzdem geaendert")
	}
	if lesen() != "" {
		t.Fatalf("zu alter eigener Block gesetzt: %q", lesen())
	}
	// Eigener Block rechtzeitig: gesetzt.
	recht := &Block{Height: 961, Timestamp: z2.Zeit + 200, ParentHashes: []string{z2.Hash}, Transactions: []Transaction{{Type: "transfer"}, tx}}
	v = f.dag.checkpointVorschau(recht.Transactions, recht.ParentHashes, recht.Timestamp)
	if !v.geaendert || v.index != 1 {
		t.Fatalf("Vorschau: %+v", v)
	}
	recht.Hash = "cp-einreichen-" + strings.Repeat("a", 50)
	recht.letzterCheckpointNeu = v.neu
	t.Cleanup(func() { f.dag.state.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, recht.Hash) })
	if err := f.dag.state.SaveBlockWithPendingTxsAtomic(recht, nil); err != nil {
		t.Fatal(err)
	}
	if lesen() != letzterCheckpointWert(z2.KettenCheckpoint) {
		t.Fatalf("eigener Block: %q", lesen())
	}
	if err := f.dag.checkpointTxEinreichen(z1, z1.Zeit+200); !istZustandsAblehnung(err) {
		t.Fatalf("aelterer: %v", err)
	}
}

// Der Erzeuger bleibt mit einer alten Checkpoint-Transaktion nicht stehen:
// sie passt zu jeder Blockzeit ab dem Stichtag.
func TestCheckpointTx_BlockTauglichOhneAlter(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	tx := neueCheckpointTx(f.zertFuer(t, f.blocks[1]))
	if err := blockTauglich(&tx, f.blocks[1].Timestamp+10*86400); err == nil {
		t.Fatal("vor dem Stichtag tauglich")
	}
	checkpointTxAn(t)
	if err := blockTauglich(&tx, f.blocks[1].Timestamp+10*86400); err != nil {
		t.Fatalf("alte Checkpoint-Transaktion haelt die Erzeugung an: %v", err)
	}
	if _, _, err := blockZeitPraefix([]Transaction{tx}, f.blocks[1].Timestamp+10*86400, f.blocks[1].Timestamp); err != nil {
		t.Fatal(err)
	}
}

// Durch replayTransactions: gueltig gesetzt, zwei je Block oder eine
// Faelschung machen den Block ungueltig (nichts gesetzt), zu alt wird
// uebersprungen (Block gueltig).
func TestCheckpointTx_Nachspielen_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	checkpointTxAn(t)
	d := f.dag
	d.bootHeight = 0
	d.replayFailures = make(map[string]replayFailureState)
	d.stateRootMismatches = map[string]int{}
	d.stateRootMismatchLastAt = map[string]int64{}
	alt := checkpointReplayLeser
	checkpointReplayLeser = func(sqlExecutor) kopfLeser { return f.koepfe.leser() }
	t.Cleanup(func() { checkpointReplayLeser = alt })
	if _, err := d.state.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	b1, b2 := f.blocks[1], f.blocks[2]
	n := 0
	block := func(eltern string, zeit int64, txs ...Transaction) bool {
		n++
		b := &Block{Height: int64(900 + n), Hash: cpHash(9100 + n), Timestamp: zeit, ParentHashes: []string{eltern}, Transactions: txs}
		return d.replayTransactions(b, true)
	}
	lesen := func() string { return d.state.getConfigValueDB(letzterCheckpointSchluessel) }
	t1 := neueCheckpointTx(f.zertFuer(t, b1))
	kaputt := f.zertFuer(t, b1)
	kaputt.Unterschriften[0].Sig = kaputt.Unterschriften[1].Sig
	tk := neueCheckpointTx(kaputt)
	if block(b1.Hash, b1.Timestamp+200, tk) {
		t.Fatal("Faelschung nachgespielt")
	}
	if block(b1.Hash, b1.Timestamp+200, t1, t1) {
		t.Fatal("zwei Checkpoints in einem Block nachgespielt")
	}
	if lesen() != "" {
		t.Fatalf("nach abgewiesenen Bloecken gesetzt: %q", lesen())
	}
	if !block(b1.Hash, b1.Timestamp+200, t1) || lesen() != letzterCheckpointWert(t1.Checkpoint.KettenCheckpoint) {
		t.Fatalf("gueltiger Checkpoint: %q", lesen())
	}
	t2 := neueCheckpointTx(f.zertFuer(t, b2))
	if !block(b2.Hash, b2.Timestamp+checkpointHoechstensAlt+1, t2) || lesen() != letzterCheckpointWert(t1.Checkpoint.KettenCheckpoint) {
		t.Fatalf("zu alt: Block muss gueltig bleiben, letzter unveraendert: %q", lesen())
	}
}

// Der Snapshot traegt den letzten Checkpoint (sonst haette ein frisch
// aufgesetzter Knoten eine andere StateRoot).
func TestCheckpointTx_SnapshotTraegtLetzten_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	cs := f.dag.state
	if _, err := cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	if snap := cs.ExportSnapshot(nil, 1, false); snap != nil {
		if _, da := snap.ChainConfig[letzterCheckpointSchluessel]; da {
			t.Fatal("Snapshot ohne Checkpoint traegt den Schluessel")
		}
	}
	w := letzterCheckpointWert(f.vorschlag(t, f.blocks[1]))
	if err := cs.setConfigValueDB(letzterCheckpointSchluessel, w); err != nil {
		t.Fatal(err)
	}
	snap := cs.ExportSnapshot(nil, 2, false)
	if snap == nil || snap.ChainConfig[letzterCheckpointSchluessel] != w {
		t.Fatalf("Snapshot ohne letzten Checkpoint: %+v", snap)
	}
}

// F1: das Feld Checkpoint an einer anderen Transaktion macht den Block
// ungueltig -- auch vor dem Stichtag (Ballast, verschiedene Urteile).
func TestCheckpointFeld_NurAnCheckpointTx_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	z := f.zertFuer(t, f.blocks[1])
	tr := Transaction{Type: "transfer", Wallet: f.adr[0], To: f.adr[1], Amount: 1, TxHash: "0x01", Checkpoint: &z}
	if checkpointFeldErlaubt(&tr) == nil {
		t.Fatal("Checkpoint an transfer erlaubt")
	}
	ok := neueCheckpointTx(z)
	if checkpointFeldErlaubt(&ok) != nil || checkpointFeldErlaubt(&Transaction{Type: "transfer"}) != nil {
		t.Fatal("erlaubte Formen abgewiesen")
	}
	d := f.dag
	d.bootHeight = 0
	d.replayFailures = make(map[string]replayFailureState)
	d.stateRootMismatches = map[string]int{}
	d.stateRootMismatchLastAt = map[string]int64{}
	b := &Block{Height: 950, Hash: cpHash(9500), Timestamp: f.blocks[1].Timestamp + 200, ParentHashes: []string{f.blocks[1].Hash}, Transactions: []Transaction{tr}}
	if d.replayTransactions(b, true) {
		t.Fatal("Block mit Checkpoint an einer Ueberweisung nachgespielt")
	}
}

// F5: hoechstens ein checkpoint_zertifikat je Block -- der zweite geht in
// den naechsten.
func TestCheckpointTx_HoechstensEinerJeBlock(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	checkpointTxAn(t)
	a := neueCheckpointTx(f.zertFuer(t, f.blocks[1]))
	b := neueCheckpointTx(f.zertFuer(t, f.blocks[2]))
	jetzt := f.blocks[2].Timestamp + 200
	n, _, err := blockZeitPraefix([]Transaction{a, b}, jetzt, jetzt-1)
	if err != nil || n != 1 {
		t.Fatalf("zwei Checkpoints: %d %v", n, err)
	}
	if checkpointTxGrenze([]Transaction{{Type: "transfer"}, a, {Type: "transfer"}, b, a}) != 3 {
		t.Fatal("Grenze falsch")
	}
}

// F4: liegt der letzte Checkpoint zu weit zurueck fuer die Suche, gilt der
// neue trotzdem (Zeit-Monotonie) -- sonst rueckte der Wert nie mehr vor.
func TestApplyCheckpointTx_LetzterWeitZurueck_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	checkpointTxAn(t)
	ctx := context.Background()
	b0, b2 := f.blocks[0], f.blocks[2]
	if err := f.dag.state.setConfigValueDB(letzterCheckpointSchluessel, letzterCheckpointWert(f.vorschlag(t, b0))); err != nil {
		t.Fatal(err)
	}
	alt := vorfahrEbenenGrenze
	vorfahrEbenenGrenze = 1 // b2 -> b1 -> b0: zwei Ebenen
	t.Cleanup(func() { vorfahrEbenenGrenze = alt })
	tx := neueCheckpointTx(f.zertFuer(t, b2))
	if err := f.dag.applyCheckpointTxLocked(ctx, &tx, []string{b2.Hash}, b2.Timestamp+200, f.koepfe.leser()); err != nil {
		t.Fatalf("weit zurueck: %v", err)
	}
	if v := f.dag.state.getConfigValueDB(letzterCheckpointSchluessel); v != letzterCheckpointWert(tx.Checkpoint.KettenCheckpoint) {
		t.Fatalf("letzter: %q", v)
	}
}

// F8: vor dem Stichtag uebernimmt ein Snapshot keinen letzten Checkpoint.
func TestSnapshotKonfigUebernehmen(t *testing.T) {
	if snapshotKonfigUebernehmen(letzterCheckpointSchluessel, 100) {
		t.Fatal("vor dem Stichtag uebernommen")
	}
	if !snapshotKonfigUebernehmen("last_ubi_at", 100) {
		t.Fatal("anderer Schluessel nicht uebernommen")
	}
	checkpointTxAn(t)
	if !snapshotKonfigUebernehmen(letzterCheckpointSchluessel, 100) {
		t.Fatal("nach dem Stichtag nicht uebernommen")
	}
}

// Stichtag-Sperre 1: ab dem Stichtag startet ein Knoten mit Liste nur mit
// festgeschriebenem Hash und mit der eigenen Adresse darin; ohne Hash ist
// eine Checkpoint-Transaktion nicht pruefbar.
func TestCheckpointTx_ListeFestgeschrieben(t *testing.T) {
	a, b := "0x"+strings.Repeat("1", 40), "0x"+strings.Repeat("2", 40)
	fest := map[string]bool{a: true, b: true}
	h := validatorListeHash(fest)
	t.Cleanup(func() { validatorListeFestgeschrieben.Store(false) })
	if err := validatorListePruefenUndMerken(fest, a, "", 100); err != nil || validatorListeFestgeschrieben.Load() {
		t.Fatalf("vor dem Stichtag ohne Hash: %v", err)
	}
	checkpointTxAn(t)
	if validatorListePruefenUndMerken(fest, a, "", 100) == nil {
		t.Fatal("ab dem Stichtag ohne Hash gestartet")
	}
	if validatorListePruefenUndMerken(fest, "0x"+strings.Repeat("3", 40), h, 100) == nil {
		t.Fatal("ab dem Stichtag ohne eigene Adresse gestartet")
	}
	if err := validatorListePruefenUndMerken(fest, a, h, 100); err != nil || !validatorListeFestgeschrieben.Load() {
		t.Fatalf("festgeschrieben: %v", err)
	}
	if err := validatorListePruefenUndMerken(nil, a, "", 100); err != nil {
		t.Fatalf("offener Betrieb ohne Liste: %v", err)
	}

	f := neuerVorschlagFall(t, true)
	f.dag.produzentenFest = map[string]bool{}
	for _, x := range f.adr {
		f.dag.produzentenFest[x] = true
	}
	tx := neueCheckpointTx(f.zertFuer(t, f.blocks[1]))
	validatorListeFestgeschrieben.Store(false)
	if err := f.dag.applyCheckpointTxLocked(context.Background(), &tx, []string{f.blocks[1].Hash}, f.blocks[1].Timestamp+200, f.koepfe.leser()); !errors.Is(err, errCheckpointNichtPruefbar) {
		t.Fatalf("Liste ohne Hash: %v", err)
	}
	validatorListeFestgeschrieben.Store(true)
	if _, err := f.dag.state.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	if err := f.dag.applyCheckpointTxLocked(context.Background(), &tx, []string{f.blocks[1].Hash}, f.blocks[1].Timestamp+200, f.koepfe.leser()); err != nil {
		t.Fatalf("Liste festgeschrieben: %v", err)
	}
}

// Die StateRoot des Erzeugers fuer einen Checkpoint-Block ist die, die jeder
// Nachspielende danach hat -- auch wenn die Transaktion uebersprungen wird.
func TestCheckpointTx_ErzeugerRootGleichNachspielen_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	checkpointTxAn(t)
	alt := checkpointReplayLeser
	checkpointReplayLeser = func(sqlExecutor) kopfLeser { return f.koepfe.leser() }
	t.Cleanup(func() { checkpointReplayLeser = alt })
	cs := f.dag.state
	if _, err := cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	b1 := f.blocks[1]
	tx := neueCheckpointTx(f.zertFuer(t, b1))
	vorher := cs.StateRoot()
	// Zu alt: keine Aenderung, die Root bleibt.
	if v := f.dag.checkpointVorschau([]Transaction{tx}, []string{b1.Hash}, b1.Timestamp+checkpointHoechstensAlt+1); v.geaendert || v.unklar {
		t.Fatalf("Vorschau fuer einen zu alten Checkpoint: %+v", v)
	}
	v := f.dag.checkpointVorschau([]Transaction{{Type: "transfer"}, tx}, []string{b1.Hash}, b1.Timestamp+200)
	if !v.geaendert || v.neu != letzterCheckpointWert(tx.Checkpoint.KettenCheckpoint) {
		t.Fatalf("Vorschau: %+v", v)
	}
	if cs.StateRoot() != vorher {
		t.Fatal("Vorschau hat den Zustand geaendert")
	}
	erzeuger := cs.stateRootMitCheckpoint(v.neu)
	eigen := &Block{Hash: "cp-erzeuger-" + strings.Repeat("b", 52), Height: 970, Timestamp: b1.Timestamp + 200, ParentHashes: []string{b1.Hash}, Transactions: []Transaction{tx}, letzterCheckpointNeu: v.neu}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, eigen.Hash) })
	if err := cs.SaveBlockWithPendingTxsAtomic(eigen, nil); err != nil {
		t.Fatal(err)
	}
	if nach := cs.StateRoot(); nach != erzeuger || nach == vorher {
		t.Fatalf("Erzeuger %s, nach dem Anwenden %s", erzeuger, nach)
	}
}

// Missbrauch/Fehler: ist der Checkpoint hier gerade nicht pruefbar (Kopf
// nicht lesbar, Frist abgelaufen), meldet die Vorschau "unklar" -- die
// Transaktion kommt nicht in den Block, statt dass andere anwenden, was der
// Erzeuger nicht weiss. Scheitert das Setzen nach der Vorschau, haelt der
// Knoten die Produktion an.
func TestCheckpointTx_VorschauUnklarUndSetzenScheitert_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	checkpointTxAn(t)
	alt := checkpointReplayLeser
	t.Cleanup(func() { checkpointReplayLeser = alt })
	cs := f.dag.state
	if _, err := cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	b1 := f.blocks[1]
	tx := neueCheckpointTx(f.zertFuer(t, b1))
	txs := []Transaction{{Type: "transfer"}, tx}

	// Kopf nicht lesbar: unklar, mit der Stelle der Transaktion.
	checkpointReplayLeser = func(sqlExecutor) kopfLeser {
		return func([]string) (map[string]blockKopf, error) { return nil, errors.New("DB weg") }
	}
	if v := f.dag.checkpointVorschau(txs, []string{b1.Hash}, b1.Timestamp+200); !v.unklar || v.geaendert || v.index != 1 {
		t.Fatalf("Lesefehler: %+v", v)
	}
	// Frist abgelaufen: unklar, auch wenn der Leser gesund ist.
	checkpointReplayLeser = func(sqlExecutor) kopfLeser { return f.koepfe.leser() }
	altFrist := checkpointVorschauFrist
	checkpointVorschauFrist = -time.Second
	v := f.dag.checkpointVorschau(txs, []string{b1.Hash}, b1.Timestamp+200)
	checkpointVorschauFrist = altFrist
	if !v.unklar || v.geaendert {
		t.Fatalf("Frist: %+v", v)
	}

	// Gesund: geaendert. Dann haelt ein anderer die Zeile -- das Speichern
	// scheitert an lock_timeout, und zwar Block UND Wert zusammen: kein
	// Zustand, in dem der Block da ist und der Wert fehlt (auch nicht nach
	// einem Neustart).
	v = f.dag.checkpointVorschau(txs, []string{b1.Hash}, b1.Timestamp+200)
	if !v.geaendert {
		t.Fatalf("gesund: %+v", v)
	}
	if err := cs.setConfigValueCtx(context.Background(), letzterCheckpointSchluessel, "0:0:"+strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel)
	})
	eigen := &Block{Hash: "cp-gesperrt-" + strings.Repeat("c", 52), Height: 971, Timestamp: b1.Timestamp + 200, ParentHashes: []string{b1.Hash}, Transactions: txs, letzterCheckpointNeu: v.neu}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, eigen.Hash) })
	sperre, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sperre.Exec(`SELECT 1 FROM chain_config WHERE key = $1 FOR UPDATE`, letzterCheckpointSchluessel); err != nil {
		sperre.Rollback()
		t.Fatal(err)
	}
	for name, speichern := range map[string]func() error{
		"atomar": func() error { return cs.SaveBlockWithPendingTxsAtomic(eigen, nil) },
		"korb":   func() error { return cs.SaveBlockMitKorb(eigen, nil, 0) },
	} {
		if err := speichern(); err == nil {
			sperre.Rollback()
			t.Fatalf("%s: trotz Sperre gespeichert", name)
		}
		var n int
		if err := cs.db.QueryRow(`SELECT COUNT(*) FROM chain_blocks WHERE hash = $1`, eigen.Hash).Scan(&n); err != nil || n != 0 {
			sperre.Rollback()
			t.Fatalf("%s: Block ohne Wert gespeichert (%d, %v)", name, n, err)
		}
	}
	sperre.Rollback()
	if got := cs.getConfigValueDB(letzterCheckpointSchluessel); got == v.neu {
		t.Fatal("trotz Sperre gesetzt")
	}
	// Ohne Sperre: Block und Wert zusammen.
	if err := cs.SaveBlockWithPendingTxsAtomic(eigen, nil); err != nil {
		t.Fatal(err)
	}
	if got := cs.getConfigValueDB(letzterCheckpointSchluessel); got != v.neu {
		t.Fatalf("mit dem Block: %q", got)
	}
}
