package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
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

// Leiter: vor dem Stichtag traegt er nichts ein; danach setzt er den
// letzten Checkpoint und legt die Transaktion in den Ausgang -- ein
// aelterer danach wird abgewiesen.
func TestCheckpointTxEinreichen_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	f.dag.state.annehmendAusdruecklich.Store(true)
	t.Cleanup(func() { f.dag.state.annehmendAusdruecklich.Store(false) })
	if _, err := f.dag.state.db.Exec(`DELETE FROM chain_config WHERE key = $1`, letzterCheckpointSchluessel); err != nil {
		t.Fatal(err)
	}
	z1, z2 := f.zertFuer(t, f.blocks[1]), f.zertFuer(t, f.blocks[2])
	if err := f.dag.checkpointTxEinreichen(z2, z2.Zeit+200); err != nil {
		t.Fatal(err)
	}
	if v := f.dag.state.getConfigValueDB(letzterCheckpointSchluessel); v != "" {
		t.Fatalf("vor dem Stichtag gesetzt: %q", v)
	}
	checkpointTxAn(t)
	if err := f.dag.checkpointTxEinreichen(z2, z2.Zeit+200); err != nil {
		t.Fatal(err)
	}
	if v := f.dag.state.getConfigValueDB(letzterCheckpointSchluessel); v != letzterCheckpointWert(z2.KettenCheckpoint) {
		t.Fatalf("letzter: %q", v)
	}
	var n int
	if err := f.dag.state.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE tx_json LIKE '%checkpoint_zertifikat%'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("Ausgang: %d %v", n, err)
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
