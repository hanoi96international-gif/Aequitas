package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// QG3 gegen eine echte Datenbank: jede Note eine Zeile, die Saetze in
// chain_config; Neustart, Rueckroll, manipulierte Zeilen, Snapshot und
// Resync.

func qnZeilen(t *testing.T, cs *ChainState) int {
	t.Helper()
	var n int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM ` + quantenNotenTabelle).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func qnNote(k string, anteile int64) quantenNoteEintrag {
	return quantenNoteEintrag{K: strings.Repeat(k, 64), Satz: "isogenie-8192", Von: qnAnna, Anteile: anteile}
}

func TestQuantenNoten_Tabelle_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-noten-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	ctx := context.Background()
	satz := quantenSatzEintrag{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: qgAb + 100, Seit: qgAb}

	cs.mu.Lock()
	err := cs.quantenSatzSpeichernLocked(ctx, cs.quantenGeldJetzt().mitSatz(satz))
	for i, k := range []string{"c", "a", "b"} {
		if err != nil {
			break
		}
		e := qnNote(k, int64(1_000_000*(i+1)))
		err = cs.quantenNoteSpeichernLocked(ctx, cs.quantenGeldJetzt().mitNote(e), e)
	}
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if n := qnZeilen(t, cs); n != 3 {
		t.Fatalf("%d Zeilen, erwartet 3", n)
	}
	if k := cs.getConfigValueDB(quantenGeldKonfigSchluessel); strings.Contains(k, "noten") || !strings.Contains(k, "isogenie-8192") {
		t.Fatalf("chain_config traegt mehr als die Saetze: %s", k)
	}
	wurzel := cs.quantenGeldJetzt().stateRootTeil()
	if wurzel != "|quanten:8608fdde0b6534899b5d380736eb70175382a3290497f5bbd1165911d3a7514a" {
		t.Fatalf("StateRoot-Teil %s -- nicht derselbe wie vor QG3", wurzel)
	}

	// Missbrauch: dieselbe Note noch einmal -- Fehler, nichts ueberschrieben,
	// der Stand im Speicher bleibt.
	vorher := cs.quantenGeldJetzt()
	doppelt := qnNote("a", 999)
	cs.mu.Lock()
	err = cs.quantenNoteSpeichernLocked(ctx, vorher.mitNote(doppelt), doppelt)
	cs.mu.Unlock()
	if err == nil || cs.quantenGeldJetzt() != vorher {
		t.Fatalf("doppelte Note: %v", err)
	}
	var anteile int64
	if err := cs.db.QueryRow(`SELECT anteile FROM `+quantenNotenTabelle+` WHERE k = $1`, doppelt.K).Scan(&anteile); err != nil || anteile != 2_000_000 {
		t.Fatalf("Zeile ueberschrieben: %d %v", anteile, err)
	}

	// Neustart: dasselbe Register, dieselbe Wurzel.
	neu := testKnoten(t, "unused-quanten-noten-realdb-test-2.json")
	if got := neu.quantenGeldJetzt(); got.stateRootTeil() != wurzel || len(got.Noten) != 3 || got.Noten[0].K != strings.Repeat("a", 64) {
		t.Fatalf("nach dem Neustart: %+v", got)
	}

	// Rueckroll: eine Note in einer Transaktion, die zurueckgeht, steht nicht
	// in der Tabelle.
	tx, err := cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	e := qnNote("d", 5)
	cs.mu.Lock()
	err = cs.quantenNoteSpeichernLocked(withTx(ctx, tx), vorher.mitNote(e), e)
	cs.quantenGeld.Store(vorher) // wie blockRollbackSnapshot
	cs.mu.Unlock()
	tx.Rollback()
	if err != nil || qnZeilen(t, cs) != 3 {
		t.Fatalf("nach dem Rueckroll: %v, %d Zeilen", err, qnZeilen(t, cs))
	}

	// Einloesen: genau eine Zeile weniger; dieselbe Note ein zweites Mal
	// entfernen ist ein Fehler.
	cs.mu.Lock()
	err = cs.quantenNoteEntfernenLocked(ctx, vorher.ohneNote(strings.Repeat("b", 64)), strings.Repeat("b", 64))
	zweimal := cs.quantenNoteEntfernenLocked(ctx, cs.quantenGeldJetzt().ohneNote(strings.Repeat("b", 64)), strings.Repeat("b", 64))
	cs.mu.Unlock()
	if err != nil || zweimal == nil || qnZeilen(t, cs) != 2 {
		t.Fatalf("Einloesen: %v / zweites Mal %v / %d Zeilen", err, zweimal, qnZeilen(t, cs))
	}
	nachEinloesen := cs.quantenGeldJetzt().stateRootTeil()

	// Snapshot traegt die Noten; ein Resync schreibt die Tabelle ersetzend
	// (eine fremde Zeile verschwindet) und kommt zur selben Wurzel.
	snap := cs.ExportSnapshot(nil, 0, true)
	if snap == nil || len(snap.QuantenNoten) != 2 || !strings.Contains(snap.ChainConfig[quantenGeldKonfigSchluessel], "isogenie-8192") {
		t.Fatalf("Snapshot: %+v", snap)
	}
	if _, err := cs.db.Exec(`INSERT INTO `+quantenNotenTabelle+` (k, satz, von, anteile) VALUES ($1, 'isogenie-8192', $2, 9)`, strings.Repeat("e", 64), qnAnna); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALLOW_RESYNC_REGRESSION", "true")
	g := &resyncSnapshot{snap: snap, peerURL: "http://peer.invalid/api/snapshot", signer: resyncTestSigner}
	if err := cs.wendeResyncSnapshotAn(g); err != nil {
		t.Fatalf("Resync: %v", err)
	}
	if qnZeilen(t, cs) != 2 || cs.quantenGeldJetzt().stateRootTeil() != nachEinloesen {
		t.Fatalf("nach dem Resync: %d Zeilen, Wurzel %s", qnZeilen(t, cs), cs.quantenGeldJetzt().stateRootTeil())
	}

	// Missbrauch: ein Snapshot mit einer Note zu einem unbekannten Satz.
	// Der Resync scheitert, die Datenbank bleibt, und die Register im
	// Speicher kehren zurueck -- auch die, die vor dem Quantengeld schon
	// neu geladen waren (hier das Pruefer-Register).
	bekannterPruefer := &verifierRegister{Eintraege: []verifierEintrag{{Mensch: qnAnna, Seit: 1}}}
	cs.verifierReg.Store(bekannterPruefer)
	quantenVorher := cs.quantenGeld.Load()
	boese := *snap
	boese.Timestamp = time.Now().Unix()
	boese.QuantenNoten = append([]quantenNoteEintrag(nil), snap.QuantenNoten...)
	boese.QuantenNoten[0].Satz = "unbekannt-1"
	err = cs.wendeResyncSnapshotAn(&resyncSnapshot{snap: &boese, peerURL: g.peerURL, signer: resyncTestSigner})
	if err == nil {
		t.Fatal("Resync mit Note zu unbekanntem Satz angenommen")
	}
	if cs.verifierReg.Load() != bekannterPruefer || cs.quantenGeld.Load() != quantenVorher {
		t.Fatal("Register im Speicher nach dem gescheiterten Resync nicht zurueck")
	}
	if qnZeilen(t, cs) != 2 {
		t.Fatalf("gescheiterter Resync hat geschrieben: %d Zeilen", qnZeilen(t, cs))
	}

	// Missbrauch: manipulierte Zeilen und das alte Format halten den Knoten an.
	laden := func() error {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		return cs.quantenGeldLaden(ctx)
	}
	for _, f := range []struct {
		name, sql string
	}{
		{"K kein Hash", `INSERT INTO quanten_noten (k, satz, von, anteile) VALUES ('xyz', 'isogenie-8192', '` + qnAnna + `', 1)`},
		{"unbekannter Satz", `INSERT INTO quanten_noten (k, satz, von, anteile) VALUES ('` + strings.Repeat("f", 64) + `', 'anderer-satz', '` + qnAnna + `', 1)`},
		{"null Anteile", `INSERT INTO quanten_noten (k, satz, von, anteile) VALUES ('` + strings.Repeat("f", 64) + `', 'isogenie-8192', '` + qnAnna + `', 0)`},
		{"Von keine Adresse", `INSERT INTO quanten_noten (k, satz, von, anteile) VALUES ('` + strings.Repeat("f", 64) + `', 'isogenie-8192', 'niemand', 1)`},
	} {
		if _, err := cs.db.Exec(f.sql); err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		if err := laden(); err == nil {
			t.Errorf("%s: geladen", f.name)
		}
		if _, err := cs.db.Exec(`DELETE FROM quanten_noten WHERE k NOT IN ($1, $2)`, strings.Repeat("a", 64), strings.Repeat("c", 64)); err != nil {
			t.Fatal(err)
		}
	}
	if err := laden(); err != nil {
		t.Fatalf("aufgeraeumt, trotzdem nicht ladbar: %v", err)
	}
	alt := cs.quantenGeldJetzt().kodiert() // Saetze UND Noten -- Format vor QG3
	if err := cs.setConfigValueDB(quantenGeldKonfigSchluessel, alt); err != nil {
		t.Fatal(err)
	}
	if err := laden(); err == nil || !strings.Contains(err.Error(), "QG3") {
		t.Fatalf("altes Format geladen: %v", err)
	}
}

// Import in einen laufenden Knoten: Noten nur, wenn er noch keine hat.
func TestQuantenNoten_ImportNurInLeereTabelle_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-quanten-noten-import-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	noten := []quantenNoteEintrag{qnNote("a", 1), qnNote("b", 2)}
	if err := quantenNotenImportieren(cs.db, noten, false); err != nil || qnZeilen(t, cs) != 2 {
		t.Fatalf("in leere Tabelle: %v, %d Zeilen", err, qnZeilen(t, cs))
	}
	if err := quantenNotenImportieren(cs.db, []quantenNoteEintrag{qnNote("c", 3)}, false); err != nil || qnZeilen(t, cs) != 2 {
		t.Fatalf("in volle Tabelle: %v, %d Zeilen", err, qnZeilen(t, cs))
	}
	if err := quantenNotenImportieren(cs.db, []quantenNoteEintrag{qnNote("c", 3)}, true); err != nil || qnZeilen(t, cs) != 1 {
		t.Fatalf("ersetzend: %v, %d Zeilen", err, qnZeilen(t, cs))
	}
	// Viele Noten in Bloecken.
	viele := make([]quantenNoteEintrag, 1203)
	for i := range viele {
		h := sha256.Sum256([]byte{byte(i >> 8), byte(i)})
		viele[i] = quantenNoteEintrag{K: hex.EncodeToString(h[:]), Satz: "isogenie-8192", Von: qnAnna, Anteile: 1}
	}
	if err := quantenNotenImportieren(cs.db, viele, true); err != nil || qnZeilen(t, cs) != 1203 {
		t.Fatalf("1203 Noten: %v, %d Zeilen", err, qnZeilen(t, cs))
	}
}
