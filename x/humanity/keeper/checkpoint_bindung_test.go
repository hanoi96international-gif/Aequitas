package keeper

import (
	"crypto/ecdsa"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

const bindungZeit = int64(1_800_000_000)

type bindungFall struct {
	dag   *BlockDAG
	keys  []*ecdsa.PrivateKey
	adr   []string
	block *Block
}

// neuerBindungFall: vier Schluessel mit Fenster um bindungZeit, ein
// nachgespielter Block zu dieser Zeit.
func neuerBindungFall(t *testing.T) *bindungFall {
	t.Helper()
	f := &bindungFall{dag: newOrphanTestDAG()}
	f.dag.replayedBlocks = map[string]bool{}
	f.dag.state = &ChainState{}
	fenster := map[string][]zeitfenster{}
	for i := 0; i < 4; i++ {
		k, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		a := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
		f.keys = append(f.keys, k)
		f.adr = append(f.adr, a)
		fenster[a] = []zeitfenster{{betreiber: "0xmensch" + a[2:8], von: bindungZeit - 86400, bis: bindungZeit + 86400}}
	}
	f.dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: fenster})
	f.block = &Block{Hash: strings.Repeat("ab", 32), Height: 500, Timestamp: bindungZeit}
	f.dag.blocks[f.block.Hash] = f.block
	f.dag.replayedBlocks[f.block.Hash] = true
	return f
}

func (f *bindungFall) zertifikat(t *testing.T, hoehe int64, hash string, zeit int64, keys []*ecdsa.PrivateKey) CheckpointZertifikat {
	t.Helper()
	komitee, err := checkpointKomitee(f.dag.state.erzeugerRegister.Load(), zeit)
	if err != nil {
		t.Fatal(err)
	}
	return zertifikatVon(t, neuerCheckpoint(hoehe, hash, zeit, komitee), keys)
}

func TestCheckpointBindung_Gutfall(t *testing.T) {
	f := neuerBindungFall(t)
	z := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys[:3])
	if err := f.dag.checkpointGegenKette(z, nil, bindungZeit+5); err != nil {
		t.Fatal(err)
	}
	letzter := &KettenCheckpoint{Hoehe: 400, Zeit: bindungZeit - 100}
	if err := f.dag.checkpointGegenKette(z, letzter, bindungZeit+5); err != nil {
		t.Fatalf("nach einem aelteren Checkpoint: %v", err)
	}
}

// Missbrauch: die Zeit (und damit das Komitee) waehlt nicht der Einreicher.
func TestCheckpointBindung_Faelschungen(t *testing.T) {
	f := neuerBindungFall(t)
	jetzt := bindungZeit + 5
	echt := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys[:3])

	// Ein Block, den dieser Knoten nicht hat.
	unbekannt := f.zertifikat(t, 500, strings.Repeat("cd", 32), bindungZeit, f.keys[:3])
	// Hoehe bzw. Zeit passen nicht zum Block (vom Komitee so unterschrieben).
	falscheHoehe := f.zertifikat(t, 501, f.block.Hash, bindungZeit, f.keys[:3])
	falscheZeit := f.zertifikat(t, 500, f.block.Hash, bindungZeit-1, f.keys[:3])

	for name, x := range map[string]struct {
		z       CheckpointZertifikat
		letzter *KettenCheckpoint
		jetzt   int64
	}{
		"unbekannter Block":    {unbekannt, nil, jetzt},
		"falsche Hoehe":        {falscheHoehe, nil, jetzt},
		"falsche Zeit":         {falscheZeit, nil, jetzt},
		"zu alt":               {echt, nil, bindungZeit + checkpointHoechstensAlt + 1},
		"aus der Zukunft":      {echt, nil, bindungZeit - zeitstempelZukunftToleranz - 1},
		"nicht nach letztem":   {echt, &KettenCheckpoint{Hoehe: 500, Zeit: bindungZeit}, jetzt},
		"Hoehe nicht groesser": {echt, &KettenCheckpoint{Hoehe: 500, Zeit: bindungZeit - 10}, jetzt},
		"Zeit nicht groesser":  {echt, &KettenCheckpoint{Hoehe: 400, Zeit: bindungZeit}, jetzt},
	} {
		if err := f.dag.checkpointGegenKette(x.z, x.letzter, x.jetzt); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	// Nicht nachgespielt: abgelehnt.
	f.dag.replayedBlocks[f.block.Hash] = false
	if f.dag.checkpointGegenKette(echt, nil, jetzt) == nil {
		t.Fatal("nicht nachgespielter Block angenommen")
	}
	f.dag.replayedBlocks[f.block.Hash] = true
	if err := f.dag.checkpointGegenKette(echt, nil, jetzt); err != nil {
		t.Fatalf("Gegenprobe: %v", err)
	}
}

// Das Komitee kommt aus dem Register zur Blockzeit: wer zu dieser Zeit kein
// Fenster hat, zaehlt nicht -- auch nicht, wenn er unterschreibt. Und eine
// Zeit, zu der das Komitee klein ist, bestaetigt nichts.
func TestCheckpointBindung_KomiteeZurBlockzeit(t *testing.T) {
	f := neuerBindungFall(t)
	st := f.dag.state.erzeugerRegister.Load()
	// Der vierte Schluessel hat sein Fenster erst nach der Blockzeit.
	st.fenster[f.adr[3]] = []zeitfenster{{betreiber: "0xspaet", von: bindungZeit + 1, bis: bindungZeit + 86400}}
	komitee, err := checkpointKomitee(st, bindungZeit)
	if err != nil {
		t.Fatal(err)
	}
	if len(komitee) != 3 || komitee[f.adr[3]] {
		t.Fatalf("Komitee zur Blockzeit: %v", komitee)
	}
	// Drei Mitglieder < checkpointMindestKomitee: auch volle Zustimmung
	// bestaetigt nichts.
	z := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys[:3])
	if f.dag.checkpointGegenKette(z, nil, bindungZeit+5) == nil {
		t.Fatal("kleines Komitee hat bestaetigt")
	}
}

// Ein Knoten mit anderem Register-Stand (ein Mitglied mehr) lehnt ab, statt
// still anders zu urteilen.
func TestCheckpointBindung_AndererStandLehntAb(t *testing.T) {
	f := neuerBindungFall(t)
	z := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys)
	k, _ := crypto.GenerateKey()
	extra := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
	st := f.dag.state.erzeugerRegister.Load()
	st.fenster[extra] = []zeitfenster{{betreiber: "0xextra", von: bindungZeit - 10, bis: bindungZeit + 10}}
	if f.dag.checkpointGegenKette(z, nil, bindungZeit+5) == nil {
		t.Fatal("anderer Stand hat bestaetigt")
	}
}

// Kein Stand oder ein Lesefehler: Ablehnung.
func TestCheckpointBindung_OhneStand(t *testing.T) {
	f := neuerBindungFall(t)
	z := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys[:3])
	st := f.dag.state.erzeugerRegister.Load()
	f.dag.state.erzeugerRegister.Store(&erzeugerStand{fenster: st.fenster, fehler: errors.New("Lesefehler")})
	if f.dag.checkpointGegenKette(z, nil, bindungZeit+5) == nil {
		t.Fatal("mit Lesefehler bestaetigt")
	}
	f.dag.state.erzeugerRegister.Store(nil)
	if f.dag.checkpointGegenKette(z, nil, bindungZeit+5) == nil {
		t.Fatal("ohne Stand bestaetigt")
	}
	if _, err := checkpointKomitee(nil, bindungZeit); err == nil {
		t.Fatal("checkpointKomitee ohne Stand")
	}
}

// Hoechstens targetCommitteeSize Mitglieder, auch bei mehr Fenstern.
func TestCheckpointBindung_KomiteeBegrenzt(t *testing.T) {
	fenster := map[string][]zeitfenster{}
	for i := 0; i < targetCommitteeSize+50; i++ {
		k, _ := crypto.GenerateKey()
		fenster[strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())] = []zeitfenster{{betreiber: "0xm", von: 0, bis: bindungZeit * 2}}
	}
	komitee, err := checkpointKomitee(&erzeugerStand{fenster: fenster}, bindungZeit)
	if err != nil || len(komitee) != targetCommitteeSize {
		t.Fatalf("%d Mitglieder, %v", len(komitee), err)
	}
}
