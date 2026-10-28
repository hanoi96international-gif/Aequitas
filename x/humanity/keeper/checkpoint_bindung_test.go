package keeper

import (
	"crypto/ecdsa"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

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
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(math.MaxInt64) })
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
	komitee, err := checkpointKomitee(f.dag.state.erzeugerRegister.Load(), zeit, nil)
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
		"unbekannter Block":   {unbekannt, nil, jetzt},
		"falsche Hoehe":       {falscheHoehe, nil, jetzt},
		"falsche Zeit":        {falscheZeit, nil, jetzt},
		"zu alt":              {echt, nil, bindungZeit + checkpointHoechstensAlt + 1},
		"aus der Zukunft":     {echt, nil, bindungZeit - zeitstempelZukunftToleranz - 1},
		"Wiederholung":        {echt, &KettenCheckpoint{Hoehe: 500, Zeit: bindungZeit}, jetzt},
		"Zeit nicht groesser": {echt, &KettenCheckpoint{Hoehe: 400, Zeit: bindungZeit}, jetzt},
		"vor dem letzten":     {echt, &KettenCheckpoint{Hoehe: 400, Zeit: bindungZeit + 1}, jetzt},
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
	komitee, err := checkpointKomitee(st, bindungZeit, nil)
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
	alt := f.dag.state.erzeugerRegister.Load()
	fenster := map[string][]zeitfenster{extra: {{betreiber: "0xextra", von: bindungZeit - 10, bis: bindungZeit + 10}}}
	for a, w := range alt.fenster {
		fenster[a] = w
	}
	// Ein echter zweiter Stand, mit vorberechneter Rangliste.
	f.dag.state.erzeugerRegister.Store(erzeugerStandBauen(fenster, nil, nil, time.Now(), bindungZeit))
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
	if _, err := checkpointKomitee(nil, bindungZeit, nil); err == nil {
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
	komitee, err := checkpointKomitee(&erzeugerStand{fenster: fenster}, bindungZeit, nil)
	if err != nil || len(komitee) != targetCommitteeSize {
		t.Fatalf("%d Mitglieder, %v", len(komitee), err)
	}
}

// Zeitgrenzen exakt: 600 s alt und 120 s Zukunft gehen noch, eine Sekunde
// mehr nicht.
func TestCheckpointBindung_Zeitgrenzen(t *testing.T) {
	f := neuerBindungFall(t)
	z := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys[:3])
	for jetzt, ok := range map[int64]bool{
		bindungZeit + checkpointHoechstensAlt:        true,
		bindungZeit + checkpointHoechstensAlt + 1:    false,
		bindungZeit - zeitstempelZukunftToleranz:     true,
		bindungZeit - zeitstempelZukunftToleranz - 1: false,
	} {
		if err := f.dag.checkpointGegenKette(z, nil, jetzt); (err == nil) != ok {
			t.Fatalf("jetzt = Blockzeit%+d: %v, erwartet ok=%v", jetzt-bindungZeit, err, ok)
		}
	}
}

// Eine ueberhoehte Hoehe im letzten Checkpoint sperrt nichts: Monotonie
// nur ueber die Zeit.
func TestCheckpointBindung_HoeheSperrtNicht(t *testing.T) {
	f := neuerBindungFall(t)
	z := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys[:3])
	if err := f.dag.checkpointGegenKette(z, &KettenCheckpoint{Hoehe: 1 << 40, Zeit: bindungZeit - 10}, bindungZeit+5); err != nil {
		t.Fatalf("ueberhoehte Hoehe sperrt: %v", err)
	}
}

// Vor erzeugerSchnittAb kommt das Komitee nicht aus dem Register: nichts
// wird bestaetigt.
func TestCheckpointBindung_VorDemStichtag(t *testing.T) {
	f := neuerBindungFall(t)
	z := f.zertifikat(t, 500, f.block.Hash, bindungZeit, f.keys[:3])
	erzeugerSchnittOverride.Store(bindungZeit + zeitstempelRueckToleranz + 1)
	if f.dag.checkpointGegenKette(z, nil, bindungZeit+5) == nil {
		t.Fatal("vor dem Stichtag bestaetigt")
	}
}

// Geschlossener Betrieb: der eigene Schluessel, den der Stand zusaetzlich
// liest, zaehlt nicht -- sonst haette jeder Knoten ausserhalb der Liste ein
// eigenes Komitee und lehnte jedes Zertifikat ab.
func TestCheckpointBindung_EigenerSchluesselNichtImKomitee(t *testing.T) {
	f := neuerBindungFall(t)
	k, _ := crypto.GenerateKey()
	selbst := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
	st := f.dag.state.erzeugerRegister.Load()
	fenster := map[string][]zeitfenster{selbst: {{betreiber: "0xselbst", von: bindungZeit - 10, bis: bindungZeit + 10}}}
	for a, w := range st.fenster {
		fenster[a] = w
	}
	f.dag.state.erzeugerRegister.Store(erzeugerStandBauen(fenster, nil, nil, time.Now(), bindungZeit))
	f.dag.selfProposer = selbst
	f.dag.produzentenFest = map[string]bool{}
	for _, a := range f.adr {
		f.dag.produzentenFest[a] = true
	}
	// Das Zertifikat der Listen-Mitglieder (wie es jeder Knoten mit dieser
	// Liste sieht) wird angenommen.
	komitee := map[string]bool{}
	for _, a := range f.adr {
		komitee[a] = true
	}
	z := zertifikatVon(t, neuerCheckpoint(500, f.block.Hash, bindungZeit, komitee), f.keys[:3])
	if err := f.dag.checkpointGegenKette(z, nil, bindungZeit+5); err != nil {
		t.Fatalf("eigener Schluessel verfaelscht das Komitee: %v", err)
	}
}

// Fenstergrenzen: t == von ist Mitglied, t == bis nicht; ein umstrittener
// Schluessel (zwei Fenster zugleich) zaehlt nicht.
func TestCheckpointBindung_Fenstergrenzen(t *testing.T) {
	a, b, c := "0x"+strings.Repeat("a", 40), "0x"+strings.Repeat("b", 40), "0x"+strings.Repeat("c", 40)
	st := &erzeugerStand{fenster: map[string][]zeitfenster{
		a: {{betreiber: "0xm1", von: bindungZeit, bis: bindungZeit + 10}},
		b: {{betreiber: "0xm2", von: bindungZeit - 10, bis: bindungZeit}},
		c: {{betreiber: "0xm3", von: 0, bis: bindungZeit * 2}, {betreiber: "0xm4", von: 0, bis: bindungZeit * 2}},
	}}
	k, err := checkpointKomitee(st, bindungZeit, nil)
	if err != nil || !k[a] || k[b] || k[c] || len(k) != 1 {
		t.Fatalf("Komitee %v, %v -- erwartet nur a", k, err)
	}
}

// Zu wenige oder zu viele Unterschriften: abgelehnt, bevor ein Block
// nachgeschlagen wird (ein unbekannter Hash kaeme sonst als "nicht
// nachgespielt" zurueck).
func TestCheckpointBindung_ZahlVorNachschlag(t *testing.T) {
	f := neuerBindungFall(t)
	z := f.zertifikat(t, 500, strings.Repeat("cd", 32), bindungZeit, f.keys[:2])
	err := f.dag.checkpointGegenKette(z, nil, bindungZeit+5)
	if err == nil || !strings.Contains(err.Error(), "Unterschriften") {
		t.Fatalf("zu wenige Unterschriften nicht vor dem Nachschlag abgewiesen: %v", err)
	}
}
