package keeper

import (
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// vorschlagFall: der Unterschrift-Fall plus Koepfe fuer die Vorfahren-Suche
// (b0 <- b1 <- b2, dazu ein Seitenblock im Slot von b2, der b1 nicht nennt).
type vorschlagFall struct {
	*unterschriftFall
	koepfe *speicherKette
	seite  *Block
}

func neuerVorschlagFall(t *testing.T, mitDB bool) *vorschlagFall {
	t.Helper()
	f := &vorschlagFall{unterschriftFall: neuerUnterschriftFall(t, mitDB), koepfe: &speicherKette{k: map[string]blockKopf{}}}
	for i, b := range f.blocks {
		var eltern []string
		if i > 0 {
			eltern = []string{f.blocks[i-1].Hash}
		}
		f.koepfe.k[b.Hash] = blockKopf{hoehe: b.Height, zeit: b.Timestamp, eltern: eltern}
	}
	f.seite = &Block{Hash: cpHash(7100), Height: f.blocks[2].Height, Timestamp: f.blocks[2].Timestamp + 1}
	f.dag.blocks[f.seite.Hash] = f.seite
	f.dag.replayedBlocks[f.seite.Hash] = true
	f.koepfe.k[f.seite.Hash] = blockKopf{hoehe: f.seite.Height, zeit: f.seite.Timestamp, eltern: []string{f.blocks[0].Hash}}
	return f
}

// leiterVorschlag: Vorschlag des Leiters keys[1] in Amtszeit 7.
func (f *vorschlagFall) leiterVorschlag(t *testing.T, b *Block) CheckpointVorschlag {
	t.Helper()
	v, err := checkpointVorschlagen(f.vorschlag(t, b), 7, f.keys[1])
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPruefeVorschlag(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	b := f.blocks[1]
	komitee, _ := checkpointKomitee(f.dag.state.erzeugerRegister.Load(), b.Timestamp, nil)
	leiter := f.adr[1]
	v := f.leiterVorschlag(t, b)
	if err := pruefeVorschlag(v, leiter, 7, komitee); err != nil {
		t.Fatalf("gueltiger Vorschlag: %v", err)
	}

	// Ein anderes Mitglied unterschreibt einen Vorschlag -- es ist nicht
	// der Leiter dieser Sicht.
	fremd, _ := checkpointVorschlagen(v.KettenCheckpoint, 7, f.keys[2])
	// Leiter-Feld gefaelscht: unterschrieben von keys[2], ausgegeben als Leiter.
	gefaelscht := fremd
	gefaelscht.Leiter = leiter
	// Block nach dem Unterschreiben getauscht.
	getauscht := v
	getauscht.Hash = f.blocks[2].Hash
	// Hohes s: zweite Schreibweise derselben Unterschrift.
	roh, _ := hex.DecodeString(v.Sig)
	hoch := v
	hoch.Sig = hex.EncodeToString(hohesS(roh))
	// Die Unterschrift eines Mitglieds unter den Checkpoint gilt nicht als
	// Vorschlag (eigene Domaene).
	quer := v
	mitglied, _ := checkpointUnterschreiben(v.KettenCheckpoint, f.keys[1])
	quer.Sig = mitglied.Sig
	// Ein Nicht-Mitglied als Leiter.
	k, _ := crypto.GenerateKey()
	aussen, _ := checkpointVorschlagen(v.KettenCheckpoint, 7, k)

	for name, x := range map[string]struct {
		v      CheckpointVorschlag
		leiter string
		term   uint64
	}{
		"anderes Mitglied":       {fremd, leiter, 7},
		"Leiter gefaelscht":      {gefaelscht, leiter, 7},
		"Block getauscht":        {getauscht, leiter, 7},
		"hohes s":                {hoch, leiter, 7},
		"Mitglieds-Unterschrift": {quer, leiter, 7},
		"alte Amtszeit":          {v, leiter, 8},
		"kein Leiter bekannt":    {v, "", 7},
		"Nicht-Mitglied":         {aussen, aussen.Leiter, 7},
	} {
		if err := pruefeVorschlag(x.v, x.leiter, x.term, komitee); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
}

// Ohne Datenbank: jede Ablehnung kommt vor dem Merker; ein gueltiger
// Vorschlag erreicht ihn (und scheitert dort fail-closed).
func TestCheckpointVorschlagAnnehmen_Regeln(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	b0, b2 := f.blocks[0], f.blocks[2]
	v := f.leiterVorschlag(t, b2)
	jetzt := b2.Timestamp + 200
	letzter := f.vorschlag(t, b0)
	if _, err := f.dag.checkpointVorschlagAnnehmen(v, f.adr[1], 7, &letzter, f.koepfe.leser(), jetzt); err == nil || !strings.Contains(err.Error(), "Datenbank") {
		t.Fatalf("gueltiger Vorschlag ohne DB: %v", err)
	}
	if _, err := f.dag.checkpointVorschlagAnnehmen(v, f.adr[1], 7, nil, nil, jetzt); err == nil || !strings.Contains(err.Error(), "Datenbank") {
		t.Fatalf("ohne letzten Checkpoint: %v", err)
	}

	seite := f.leiterVorschlag(t, f.seite)
	nichtVorfahr := f.vorschlag(t, f.blocks[1]) // b1 liegt nicht unter dem Seitenblock
	luecke := &speicherKette{k: map[string]blockKopf{b2.Hash: f.koepfe.k[b2.Hash]}}
	kaputt := &speicherKette{fehler: errors.New("Lesefehler")}
	fremd, _ := checkpointVorschlagen(v.KettenCheckpoint, 7, f.keys[2])
	selbst := f.vorschlag(t, b2)
	for name, x := range map[string]struct {
		v       CheckpointVorschlag
		leiter  string
		letzter *KettenCheckpoint
		lies    kopfLeser
	}{
		"nicht vom Leiter":      {fremd, f.adr[1], &letzter, f.koepfe.leser()},
		"letzter nicht Vorfahr": {seite, f.adr[1], &nichtVorfahr, f.koepfe.leser()},
		"nicht juenger":         {v, f.adr[1], &selbst, f.koepfe.leser()},
		"Luecke":                {v, f.adr[1], &letzter, luecke.leser()},
		"Lesefehler":            {v, f.adr[1], &letzter, kaputt.leser()},
		"ohne Leser":            {v, f.adr[1], &letzter, nil},
	} {
		if _, err := f.dag.checkpointVorschlagAnnehmen(x.v, x.leiter, 7, x.letzter, x.lies, jetzt); err == nil || strings.Contains(err.Error(), "Datenbank") {
			t.Fatalf("%s: %v -- erwartet eine Ablehnung vor dem Merker", name, err)
		}
	}
	f.dag.state = nil
	if _, err := f.dag.checkpointVorschlagAnnehmen(v, f.adr[1], 7, nil, nil, jetzt); err == nil {
		t.Fatal("ohne Zustand angenommen")
	}
}

// Ein gefaelschter Vorschlag verbraucht den Slot nicht: danach bekommt der
// echte Vorschlag des Leiters fuer einen anderen Block desselben Slots die
// Unterschrift. Ohne Leiter-Pruefung waere der Slot fuer den Seitenblock
// belegt.
func TestCheckpointVorschlagAnnehmen_FaelschungVerbrauchtKeinenSlot_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	b2 := f.blocks[2]
	jetzt := b2.Timestamp + 200
	echt := f.leiterVorschlag(t, b2)
	k, _ := crypto.GenerateKey()
	falsch, _ := checkpointVorschlagen(f.vorschlag(t, f.seite), 7, k)
	falsch.Leiter = f.adr[1]
	if _, err := f.dag.checkpointVorschlagAnnehmen(falsch, f.adr[1], 7, nil, nil, jetzt); err == nil {
		t.Fatal("gefaelschter Vorschlag unterschrieben")
	}
	u, err := f.dag.checkpointVorschlagAnnehmen(echt, f.adr[1], 7, nil, nil, jetzt)
	if err != nil {
		t.Fatalf("echter Vorschlag nach der Faelschung: %v", err)
	}
	if u.Von != f.adr[0] {
		t.Fatalf("Unterschrift von %s", u.Von)
	}
	// Ein zweiter echter Vorschlag des Leiters im selben Slot (Slot-Verbrauch)
	// wird abgewiesen -- der Merker gilt auch fuer den Leiter.
	zweiter := f.leiterVorschlag(t, f.seite)
	if _, err := f.dag.checkpointVorschlagAnnehmen(zweiter, f.adr[1], 7, nil, nil, jetzt); !errors.Is(err, errCheckpointSlotBelegt) {
		t.Fatalf("zweiter Block im Slot: %v", err)
	}
	if !checkpointVorschlagDoppelt(echt, zweiter) {
		t.Fatal("zwei Bloecke im Slot nicht als Beweis erkannt")
	}
}

// Mit letztem Checkpoint: ein Block, der ihn nicht in seiner Vergangenheit
// hat, bekommt keine Unterschrift -- und belegt den Slot nicht.
func TestCheckpointVorschlagAnnehmen_Kette_RealDB(t *testing.T) {
	f := neuerVorschlagFall(t, true)
	b1, b2 := f.blocks[1], f.blocks[2]
	jetzt := b2.Timestamp + 200
	letzter := f.vorschlag(t, b1)
	if _, err := f.dag.checkpointVorschlagAnnehmen(f.leiterVorschlag(t, f.seite), f.adr[1], 7, &letzter, f.koepfe.leser(), jetzt); err == nil {
		t.Fatal("Block ohne den letzten Checkpoint in der Vergangenheit unterschrieben")
	}
	if _, err := f.dag.checkpointVorschlagAnnehmen(f.leiterVorschlag(t, b2), f.adr[1], 7, &letzter, f.koepfe.leser(), jetzt); err != nil {
		t.Fatalf("Nachfolger des letzten Checkpoints: %v", err)
	}
}

func TestCheckpointVorschlagDoppelt(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	a := f.leiterVorschlag(t, f.blocks[2])
	b := f.leiterVorschlag(t, f.seite)
	if !checkpointVorschlagDoppelt(a, b) {
		t.Fatal("Beweis nicht erkannt")
	}
	andererSlot := f.leiterVorschlag(t, f.blocks[1])
	andererLeiter, _ := checkpointVorschlagen(b.KettenCheckpoint, 7, f.keys[2])
	gefaelscht := b
	gefaelscht.Sig = andererLeiter.Sig
	for name, x := range map[string]CheckpointVorschlag{
		"derselbe":       a,
		"anderer Slot":   andererSlot,
		"anderer Leiter": andererLeiter,
		"gefaelscht":     gefaelscht,
	} {
		if checkpointVorschlagDoppelt(a, x) {
			t.Fatalf("%s: als Beweis gewertet", name)
		}
	}
}

func TestCheckpointLeiterStockt(t *testing.T) {
	grenze := checkpointHoechstensAlt + checkpointLeerSlots*checkpointSlotLaenge
	if checkpointLeiterStockt(1000, 0, 1000+grenze) {
		t.Fatal("an der Grenze schon gestockt")
	}
	if !checkpointLeiterStockt(1000, 0, 1000+grenze+1) {
		t.Fatal("ueber der Grenze nicht gestockt")
	}
	// Ohne Zertifikat zaehlt der Start; ein spaeterer Start setzt neu.
	if checkpointLeiterStockt(0, 5000, 5000+grenze) || !checkpointLeiterStockt(0, 5000, 5000+grenze+1) {
		t.Fatal("ohne Zertifikat falsch gezaehlt")
	}
	if checkpointLeiterStockt(1000, 5000, 5000+grenze) {
		t.Fatal("Amtsantritt nach dem Zertifikat nicht beruecksichtigt")
	}
}

func TestCheckpointSammler(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	c := f.vorschlag(t, f.blocks[1])
	komitee, _ := checkpointKomitee(f.dag.state.erzeugerRegister.Load(), c.Zeit, nil)
	s, err := neuerCheckpointSammler(c, komitee)
	if err != nil {
		t.Fatal(err)
	}
	unter := func(i int) CheckpointUnterschrift {
		u, err := checkpointUnterschreiben(c, f.keys[i])
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	// Abgewiesen und nicht gezaehlt: fremder Absender, Unterschrift fuer
	// einen anderen Block, falscher Absender, hohes s.
	k, _ := crypto.GenerateKey()
	aussen, _ := checkpointUnterschreiben(c, k)
	andererBlock, _ := checkpointUnterschreiben(f.vorschlag(t, f.blocks[2]), f.keys[2])
	falscherVon := unter(2)
	falscherVon.Von = f.adr[3]
	roh, _ := hex.DecodeString(unter(2).Sig)
	hoch := unter(2)
	hoch.Sig = hex.EncodeToString(hohesS(roh))
	for name, u := range map[string]CheckpointUnterschrift{
		"fremd": aussen, "anderer Block": andererBlock, "falscher Absender": falscherVon, "hohes s": hoch,
	} {
		if z, err := s.hinzu(u); err == nil || z != nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	if len(s.sigs) != 0 {
		t.Fatalf("%d abgewiesene Unterschriften gespeichert", len(s.sigs))
	}
	// Dasselbe Mitglied mehrfach zaehlt einmal.
	for i := 0; i < 5; i++ {
		if z, err := s.hinzu(unter(1)); err != nil || z != nil {
			t.Fatalf("Wiederholung: %v %v", z, err)
		}
	}
	if z, err := s.hinzu(unter(2)); err != nil || z != nil {
		t.Fatalf("zwei von vier: %v %v", z, err)
	}
	z, err := s.hinzu(unter(3))
	if err != nil || z == nil {
		t.Fatalf("Quorum: %v %v", z, err)
	}
	if err := pruefeZertifikat(*z, komitee); err != nil {
		t.Fatal(err)
	}
	// Ein falsches Komitee: kein Sammler.
	if _, err := neuerCheckpointSammler(c, map[string]bool{f.adr[0]: true, f.adr[1]: true, f.adr[2]: true}); err == nil {
		t.Fatal("Sammler fuer kleines Komitee")
	}
	anders := map[string]bool{f.adr[0]: true, f.adr[1]: true, f.adr[2]: true, aussen.Von: true}
	if _, err := neuerCheckpointSammler(c, anders); err == nil {
		t.Fatal("Sammler fuer fremdes Komitee")
	}
}

// Gleichzeitig: viele Unterschriften aller Mitglieder, ein Zertifikat.
func TestCheckpointSammler_Gleichzeitig(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	c := f.vorschlag(t, f.blocks[1])
	komitee, _ := checkpointKomitee(f.dag.state.erzeugerRegister.Load(), c.Zeit, nil)
	s, err := neuerCheckpointSammler(c, komitee)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	gesehen := map[*CheckpointZertifikat]bool{}
	for r := 0; r < 8; r++ {
		for i := range f.keys {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				u, _ := checkpointUnterschreiben(c, f.keys[i])
				z, err := s.hinzu(u)
				if err != nil {
					t.Error(err)
				}
				if z != nil {
					mu.Lock()
					gesehen[z] = true
					mu.Unlock()
				}
			}(i)
		}
	}
	wg.Wait()
	if len(gesehen) != 1 {
		t.Fatalf("%d verschiedene Zertifikate", len(gesehen))
	}
	for z := range gesehen {
		if len(z.Unterschriften) != checkpointQuorum(len(komitee)) {
			t.Fatalf("%d Unterschriften", len(z.Unterschriften))
		}
	}
}
