package keeper

import (
	"strings"
	"testing"
)

func TestPruefeSnapshotKoepfe(t *testing.T) {
	gut := []SnapshotKopf{{Hash: cpHash(1), Hoehe: 1, Zeit: 100, Eltern: []string{cpHash(0)}}}
	if err := pruefeSnapshotKoepfe(gut); err != nil {
		t.Fatal(err)
	}
	vieleEltern := make([]string, peerElternGrenze+1)
	for i := range vieleEltern {
		vieleEltern[i] = cpHash(i)
	}
	zuViele := make([]SnapshotKopf, snapshotKopfGrenze+1)
	for i := range zuViele {
		zuViele[i] = gut[0]
	}
	for name, k := range map[string][]SnapshotKopf{
		"zu viele":          zuViele,
		"leerer Hash":       {{Hash: "", Hoehe: 1, Zeit: 1}},
		"langer Hash":       {{Hash: strings.Repeat("a", snapshotHashLaenge+1), Hoehe: 1, Zeit: 1}},
		"ohne Zeit":         {{Hash: cpHash(1), Hoehe: 1}},
		"viele Eltern":      {{Hash: cpHash(1), Hoehe: 1, Zeit: 1, Eltern: vieleEltern}},
		"leerer Elternteil": {{Hash: cpHash(1), Hoehe: 1, Zeit: 1, Eltern: []string{""}}},
	} {
		if pruefeSnapshotKoepfe(k) == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
}

// Ab dem Stichtag: Export traegt die juengsten Koepfe, Import legt sie in
// kopf_vorrat, und die Vorfahren-Suche findet einen Block, der nur dort
// steht. Vor dem Stichtag: nichts exportiert, nichts importiert.
func TestSnapshotKoepfe_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	cs := f.dag.state
	if _, err := cs.db.Exec(`DELETE FROM kopf_vorrat`); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, cpHash(71)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, cpHash(71)) })
	if _, err := cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, 7, '["`+cpHash(70)+`"]', 'x', $2)`, cpHash(71), kopfJetzt-10); err != nil {
		t.Fatal(err)
	}
	if k, err := cs.snapshotKoepfeExport(kopfJetzt); err != nil || k != nil {
		t.Fatalf("vor dem Stichtag exportiert: %v %v", k, err)
	}
	checkpointTxAn(t)
	k, err := cs.snapshotKoepfeExport(kopfJetzt)
	if err != nil || len(k) != 1 || k[0].Hash != cpHash(71) || k[0].Eltern[0] != cpHash(70) {
		t.Fatalf("Export: %+v %v", k, err)
	}
	fremd := []SnapshotKopf{{Hash: cpHash(80), Hoehe: 5, Zeit: kopfJetzt - 20, Eltern: []string{cpHash(79)}}}
	tx, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshotKoepfeImportieren(tx, fremd, kopfJetzt, false); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	koepfe, err := kopfLeserDB(cs.db)([]string{cpHash(80), cpHash(71), cpHash(99)})
	if err != nil || len(koepfe) != 2 || koepfe[cpHash(80)].eltern[0] != cpHash(79) || koepfe[cpHash(71)].hoehe != 7 {
		t.Fatalf("Suche: %+v %v", koepfe, err)
	}
	// Vor dem Stichtag kein Import.
	checkpointTxOverride.Store(1 << 40)
	tx, _ = cs.db.Begin()
	snapshotKoepfeImportieren(tx, []SnapshotKopf{{Hash: cpHash(81), Hoehe: 5, Zeit: kopfJetzt - 20}}, kopfJetzt, false)
	tx.Commit()
	if koepfe, _ := kopfLeserDB(cs.db)([]string{cpHash(81)}); len(koepfe) != 0 {
		t.Fatal("vor dem Stichtag importiert")
	}
}

// kopfJetzt: weit nach allen Bloecken anderer Tests -- das Fenster des
// Exports enthaelt nur die eigenen.
const kopfJetzt = int64(5_000_000_000)

// chain_blocks hat Vorrang vor kopf_vorrat; ein Resync ersetzt den Vorrat;
// ohne Tabelle liest die Suche nur chain_blocks.
func TestSnapshotKoepfe_VorrangUndAufraeumen_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	cs := f.dag.state
	checkpointTxAn(t)
	for _, q := range []string{`DELETE FROM kopf_vorrat`, `DELETE FROM chain_blocks WHERE hash = '` + cpHash(91) + `'`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, cpHash(91)) })
	if _, err := cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, 9, '["`+cpHash(90)+`"]', 'x', $2)`, cpHash(91), kopfJetzt-10); err != nil {
		t.Fatal(err)
	}
	importiere := func(k []SnapshotKopf, ersetzen bool) {
		tx, _ := cs.db.Begin()
		if err := snapshotKoepfeImportieren(tx, k, kopfJetzt, ersetzen); err != nil {
			t.Fatal(err)
		}
		tx.Commit()
	}
	// Falscher Kopf fuer einen Block, den chain_blocks hat: zaehlt nicht.
	importiere([]SnapshotKopf{{Hash: cpHash(91), Hoehe: 9, Zeit: kopfJetzt - 10, Eltern: []string{cpHash(55)}}, {Hash: cpHash(92), Hoehe: 3, Zeit: kopfJetzt - 10}}, false)
	k, err := kopfLeserDB(cs.db)([]string{cpHash(91)})
	if err != nil || k[cpHash(91)].eltern[0] != cpHash(90) {
		t.Fatalf("Vorrang: %+v %v", k, err)
	}
	// Resync ersetzt den Vorrat.
	importiere(nil, true)
	if k, _ := kopfLeserDB(cs.db)([]string{cpHash(92)}); len(k) != 0 {
		t.Fatal("Resync hat den alten Vorrat behalten")
	}
	// Ohne Tabelle (Anlegen gescheitert): nur chain_blocks, kein Fehler.
	kopfVorratDa.Store(false)
	t.Cleanup(func() { kopfVorratDa.Store(true) })
	if k, err := kopfLeserDB(cs.db)([]string{cpHash(91)}); err != nil || len(k) != 1 {
		t.Fatalf("ohne kopf_vorrat: %+v %v", k, err)
	}
}
