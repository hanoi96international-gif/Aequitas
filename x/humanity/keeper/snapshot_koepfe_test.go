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
	if _, err := cs.db.Exec(`INSERT INTO chain_blocks (hash, height, parent_hashes, proposer, timestamp) VALUES ($1, 7, '["`+cpHash(70)+`"]', 'x', 1000)`, cpHash(71)); err != nil {
		t.Fatal(err)
	}
	if k, err := cs.snapshotKoepfeExport(1100); err != nil || k != nil {
		t.Fatalf("vor dem Stichtag exportiert: %v %v", k, err)
	}
	checkpointTxAn(t)
	k, err := cs.snapshotKoepfeExport(1100)
	if err != nil || len(k) != 1 || k[0].Hash != cpHash(71) || k[0].Eltern[0] != cpHash(70) {
		t.Fatalf("Export: %+v %v", k, err)
	}
	fremd := []SnapshotKopf{{Hash: cpHash(80), Hoehe: 5, Zeit: 900, Eltern: []string{cpHash(79)}}}
	tx, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshotKoepfeImportieren(tx, fremd, 1100); err != nil {
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
	snapshotKoepfeImportieren(tx, []SnapshotKopf{{Hash: cpHash(81), Hoehe: 5, Zeit: 900}}, 1100)
	tx.Commit()
	if koepfe, _ := kopfLeserDB(cs.db)([]string{cpHash(81)}); len(koepfe) != 0 {
		t.Fatal("vor dem Stichtag importiert")
	}
}
