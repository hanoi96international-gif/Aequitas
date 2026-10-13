package keeper

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

type unterschriftFall struct {
	dag    *BlockDAG
	keys   []*ecdsa.PrivateKey
	adr    []string
	blocks []*Block
}

// neuerUnterschriftFall: dieser Knoten (keys[0]) und drei weitere im
// Komitee, drei nachgespielte Bloecke in aufeinanderfolgenden Slots.
func neuerUnterschriftFall(t *testing.T, mitDB bool) *unterschriftFall {
	t.Helper()
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	f := &unterschriftFall{dag: newOrphanTestDAG()}
	f.dag.replayedBlocks = map[string]bool{}
	if mitDB {
		if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
			t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
		}
		truncateDistTestTables(t)
		cs := testKnoten(t, "unused-checkpoint-unterschrift.json")
		if !cs.useDB {
			t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
		}
		if _, err := cs.db.Exec(`DROP TABLE IF EXISTS checkpoint_unterschriften`); err != nil {
			t.Fatal(err)
		}
		f.dag.state = cs
	} else {
		f.dag.state = &ChainState{}
	}
	fenster := map[string][]zeitfenster{}
	for i := 0; i < 4; i++ {
		k, _ := crypto.GenerateKey()
		a := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
		f.keys = append(f.keys, k)
		f.adr = append(f.adr, a)
		fenster[a] = []zeitfenster{{betreiber: "0xm" + a[2:8], von: bindungZeit - 86400, bis: bindungZeit + 86400}}
	}
	f.dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: fenster})
	f.dag.signingKey = f.keys[0]
	f.dag.selfProposer = f.adr[0]
	for i := 0; i < 3; i++ {
		b := &Block{Hash: cpHash(7000 + i), Height: int64(500 + i), Timestamp: bindungZeit + int64(i)*checkpointSlotLaenge}
		f.blocks = append(f.blocks, b)
		f.dag.blocks[b.Hash] = b
		f.dag.replayedBlocks[b.Hash] = true
	}
	return f
}

func (f *unterschriftFall) vorschlag(t *testing.T, b *Block) KettenCheckpoint {
	t.Helper()
	k, err := checkpointKomitee(f.dag.state.erzeugerRegister.Load(), b.Timestamp, nil)
	if err != nil {
		t.Fatal(err)
	}
	return neuerCheckpoint(b.Height, b.Hash, b.Timestamp, k)
}

// Die Regeln ohne Datenbank: alle Ablehnungen kommen vor dem Merker, und
// ohne Datenbank gibt es keine Unterschrift (fail-closed).
func TestCheckpointMitunterschreiben_Regeln(t *testing.T) {
	f := neuerUnterschriftFall(t, false)
	b := f.blocks[0]
	v := f.vorschlag(t, b)
	jetzt := b.Timestamp + 200
	if _, err := f.dag.checkpointMitunterschreiben(v, jetzt); err == nil || !strings.Contains(err.Error(), "Datenbank") {
		t.Fatalf("ohne Datenbank: %v", err)
	}
	falscheHoehe := v
	falscheHoehe.Hoehe++
	anderesKomitee := v
	anderesKomitee.Komitee = strings.Repeat("00", 32)
	unbekannt := v
	unbekannt.Hash = cpHash(9999)
	for name, x := range map[string]struct {
		v     KettenCheckpoint
		jetzt int64
	}{
		"zu frisch":       {v, b.Timestamp + zeitstempelRueckToleranz - 1},
		"zu alt":          {v, b.Timestamp + checkpointHoechstensAlt + 1},
		"falsche Hoehe":   {falscheHoehe, jetzt},
		"anderes Komitee": {anderesKomitee, jetzt},
		"unbekannt":       {unbekannt, jetzt},
	} {
		if _, err := f.dag.checkpointMitunterschreiben(x.v, x.jetzt); err == nil || strings.Contains(err.Error(), "Datenbank") {
			t.Fatalf("%s: %v -- erwartet eine Ablehnung vor dem Merker", name, err)
		}
	}
	// Nicht im Komitee.
	k, _ := crypto.GenerateKey()
	f.dag.signingKey = k
	f.dag.selfProposer = strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
	if _, err := f.dag.checkpointMitunterschreiben(v, jetzt); err == nil || !strings.Contains(err.Error(), "nicht im Komitee") {
		t.Fatalf("Nicht-Mitglied: %v", err)
	}
	// Ohne Schluessel.
	f.dag.signingKey = nil
	if _, err := f.dag.checkpointMitunterschreiben(v, jetzt); err == nil {
		t.Fatal("ohne Schluessel unterschrieben")
	}
	// Vor dem Stichtag.
	f2 := neuerUnterschriftFall(t, false)
	erzeugerSchnittOverride.Store(bindungZeit + zeitstempelRueckToleranz + 1)
	if _, err := f2.dag.checkpointMitunterschreiben(f2.vorschlag(t, f2.blocks[0]), jetzt); err == nil || !strings.Contains(err.Error(), "erzeugerSchnittAb") {
		t.Fatalf("vor dem Stichtag: %v", err)
	}
}

// Mit Datenbank: die Unterschrift ist gueltig, derselbe Vorschlag gibt
// dieselbe Antwort, ein anderer Block im selben Slot oder ein aelterer Slot
// nach einem juengeren wird abgewiesen -- auch nach einem "Neustart" (neuer
// DAG an derselben Datenbank).
func TestCheckpointMitunterschreiben_KeinDoppelsignieren_RealDB(t *testing.T) {
	f := neuerUnterschriftFall(t, true)
	b0, b1 := f.blocks[0], f.blocks[1]
	v1 := f.vorschlag(t, b1)
	jetzt := b1.Timestamp + 200

	u, err := f.dag.checkpointMitunterschreiben(v1, jetzt)
	if err != nil {
		t.Fatal(err)
	}
	// Die Unterschrift traegt zusammen mit zwei weiteren ein Zertifikat.
	z := CheckpointZertifikat{KettenCheckpoint: v1, Unterschriften: []CheckpointUnterschrift{u}}
	for _, k := range f.keys[1:3] {
		w, err := checkpointUnterschreiben(v1, k)
		if err != nil {
			t.Fatal(err)
		}
		z.Unterschriften = append(z.Unterschriften, w)
	}
	zertifikatOrdnen(&z)
	if err := pruefeZertifikat(z, map[string]bool{f.adr[0]: true, f.adr[1]: true, f.adr[2]: true, f.adr[3]: true}); err != nil {
		t.Fatalf("Zertifikat mit der eigenen Unterschrift: %v", err)
	}
	// Derselbe Vorschlag: wieder eine Unterschrift.
	if _, err := f.dag.checkpointMitunterschreiben(v1, jetzt+1); err != nil {
		t.Fatalf("derselbe Vorschlag erneut: %v", err)
	}
	// Ein anderer Block im selben Slot: nie.
	anders := &Block{Hash: cpHash(8000), Height: b1.Height, Timestamp: b1.Timestamp + 1}
	f.dag.blocks[anders.Hash] = anders
	f.dag.replayedBlocks[anders.Hash] = true
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, anders), jetzt); !errors.Is(err, errCheckpointSlotBelegt) {
		t.Fatalf("zweiter Block im selben Slot: %v", err)
	}
	// Aelterer Slot nach juengerem: nie.
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, b0), b0.Timestamp+200); err == nil {
		t.Fatal("aelterer Slot nach juengerem unterschrieben")
	}
	// "Neustart": neuer DAG, dieselbe Datenbank -- der Merker gilt weiter.
	neu := newOrphanTestDAG()
	neu.state, neu.signingKey, neu.selfProposer = f.dag.state, f.dag.signingKey, f.dag.selfProposer
	neu.replayedBlocks = map[string]bool{anders.Hash: true}
	neu.blocks[anders.Hash] = anders
	if _, err := neu.checkpointMitunterschreiben(f.vorschlag(t, anders), jetzt); !errors.Is(err, errCheckpointSlotBelegt) {
		t.Fatalf("nach Neustart zweiter Block im selben Slot: %v", err)
	}
	// Ein juengerer Slot geht.
	b2 := f.blocks[2]
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, b2), b2.Timestamp+200); err != nil {
		t.Fatalf("juengerer Slot: %v", err)
	}
}

// Gleichzeitige Anfragen fuer verschiedene Bloecke im selben Slot: hoechstens
// eine bekommt eine Unterschrift.
func TestCheckpointMitunterschreiben_Gleichzeitig_RealDB(t *testing.T) {
	f := neuerUnterschriftFall(t, true)
	b := f.blocks[1]
	var vs []KettenCheckpoint
	for i := 0; i < 8; i++ {
		x := &Block{Hash: cpHash(9000 + i), Height: b.Height, Timestamp: b.Timestamp + int64(i%30)}
		f.dag.blocks[x.Hash] = x
		f.dag.replayedBlocks[x.Hash] = true
		vs = append(vs, f.vorschlag(t, x))
	}
	ergebnis := make(chan error, len(vs))
	for _, v := range vs {
		v := v
		go func() {
			_, err := f.dag.checkpointMitunterschreiben(v, b.Timestamp+200)
			ergebnis <- err
		}()
	}
	ok := 0
	for range vs {
		if <-ergebnis == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d Unterschriften fuer verschiedene Bloecke im selben Slot", ok)
	}
}

func cpHash(n int) string { return fmt.Sprintf("%064x", n) }

// Verlorener Merker (Tabelle geloescht, neue Datenbank): die Startsperre
// verhindert trotzdem eine zweite Unterschrift fuer denselben Slot -- der
// neue Prozess unterschreibt nur Bloecke nach seinem Start.
func TestCheckpointMitunterschreiben_StartsperreNachMerkerverlust_RealDB(t *testing.T) {
	f := neuerUnterschriftFall(t, true)
	b1 := f.blocks[1]
	jetzt := b1.Timestamp + 200
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, b1), jetzt); err != nil {
		t.Fatal(err)
	}
	// Neustart mit verlorenem Merker.
	if _, err := f.dag.state.db.Exec(`DROP TABLE checkpoint_unterschriften`); err != nil {
		t.Fatal(err)
	}
	checkpointTabelleDa.Delete(f.dag.state.db)
	alt := checkpointProzessStart.Load()
	checkpointProzessStart.Store(jetzt)
	t.Cleanup(func() { checkpointProzessStart.Store(alt) })
	anders := &Block{Hash: cpHash(8100), Height: b1.Height, Timestamp: b1.Timestamp + 2}
	f.dag.blocks[anders.Hash] = anders
	f.dag.replayedBlocks[anders.Hash] = true
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, anders), jetzt+1); err == nil {
		t.Fatal("nach verlorenem Merker zweiter Block im selben Slot unterschrieben")
	}
	// Ein Block nach dem Start geht wieder.
	spaeter := &Block{Hash: cpHash(8200), Height: 900, Timestamp: jetzt + 10}
	f.dag.blocks[spaeter.Hash] = spaeter
	f.dag.replayedBlocks[spaeter.Hash] = true
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, spaeter), spaeter.Timestamp+200); err != nil {
		t.Fatalf("Block nach dem Start: %v", err)
	}
}

// Eine Uhr, die hinter die juengste unterschriebene Blockzeit zurueckspringt,
// unterschreibt nichts.
func TestCheckpointMitunterschreiben_UhrZurueck_RealDB(t *testing.T) {
	f := neuerUnterschriftFall(t, true)
	b2 := f.blocks[2]
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, b2), b2.Timestamp+200); err != nil {
		t.Fatal(err)
	}
	// Ein spaeterer Slot, aber die Uhr steht vor der gemerkten Blockzeit.
	x := &Block{Hash: cpHash(8300), Height: 950, Timestamp: b2.Timestamp + checkpointSlotLaenge}
	f.dag.blocks[x.Hash] = x
	f.dag.replayedBlocks[x.Hash] = true
	if _, err := f.dag.checkpointMitunterschreiben(f.vorschlag(t, x), b2.Timestamp-1); err == nil {
		t.Fatal("mit zurueckgesprungener Uhr unterschrieben")
	}
}

// Weitere Ablehnungen ohne Datenbank: kleines Komitee, kein oder fehlerhafter
// Register-Stand, Block aus der Zukunft, Block vor dem Prozessstart.
func TestCheckpointMitunterschreiben_WeitereRegeln(t *testing.T) {
	f := neuerUnterschriftFall(t, false)
	b := f.blocks[0]
	v := f.vorschlag(t, b)
	jetzt := b.Timestamp + 200
	pruefe := func(name, teil string) {
		t.Helper()
		if _, err := f.dag.checkpointMitunterschreiben(v, jetzt); err == nil || !strings.Contains(err.Error(), teil) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := f.dag.checkpointMitunterschreiben(v, b.Timestamp-10); err == nil {
		t.Fatal("Block aus der Zukunft unterschrieben")
	}
	alt := checkpointProzessStart.Load()
	checkpointProzessStart.Store(b.Timestamp)
	pruefe("vor dem Prozessstart", "Startsperre")
	checkpointProzessStart.Store(alt)
	st := f.dag.state.erzeugerRegister.Load()
	f.dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: st.fenster, fehler: errors.New("Lesefehler")})
	pruefe("Lesefehler", "Komitee nicht bestimmbar")
	f.dag.state.erzeugerRegister.Store(nil)
	pruefe("kein Stand", "Komitee nicht bestimmbar")
	klein := map[string][]zeitfenster{}
	for _, a := range f.adr[:3] {
		klein[a] = st.fenster[a]
	}
	f.dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: klein})
	pruefe("Komitee mit 3", "bestaetigt nichts")
}
