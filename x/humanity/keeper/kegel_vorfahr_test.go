package keeper

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// speicherKette: Koepfe im Speicher, mit Zaehler der gelesenen Hashes.
type speicherKette struct {
	k        map[string]blockKopf
	gelesen  int
	abfragen int
	fehler   error
}

func (s *speicherKette) leser() kopfLeser {
	return func(hashes []string) (map[string]blockKopf, error) {
		if s.fehler != nil {
			return nil, s.fehler
		}
		s.gelesen += len(hashes)
		s.abfragen++
		out := map[string]blockKopf{}
		for _, h := range hashes {
			if k, ok := s.k[h]; ok {
				out[h] = k
			}
		}
		return out, nil
	}
}

func hx(n int) string { return fmt.Sprintf("%064x", n) }

// lineareKette: Bloecke 1..n, je einer pro Sekunde ab start, jeder auf dem
// vorigen; dazu ein Seitenzweig s1..s3 auf Block 2, den die Kette nie nennt.
func lineareKette(n int, start int64) *speicherKette {
	s := &speicherKette{k: map[string]blockKopf{}}
	for i := 1; i <= n; i++ {
		var eltern []string
		if i > 1 {
			eltern = []string{hx(i - 1)}
		}
		s.k[hx(i)] = blockKopf{hoehe: int64(i), zeit: start + int64(i), eltern: eltern}
	}
	vor := hx(2)
	for i := 1; i <= 3; i++ {
		h := hx(100000 + i)
		s.k[h] = blockKopf{hoehe: int64(2 + i), zeit: start + 2 + int64(i), eltern: []string{vor}}
		vor = h
	}
	return s
}

func TestVorfahrSuchen(t *testing.T) {
	const start = int64(1_800_000_000)
	s := lineareKette(50, start)
	// Ein Vorfahr wird gefunden, mit seinem Kopf.
	k, ja, voll, err := vorfahrSuchen(s.leser(), []string{hx(50)}, hx(10), start)
	if err != nil || !ja || !voll || k.hoehe != 10 || k.zeit != start+10 {
		t.Fatalf("Vorfahr: %+v %v %v %v", k, ja, voll, err)
	}
	// Ein Block im Seitenzweig ist kein Vorfahr -- vollstaendig geprueft.
	if _, ja, voll, _ := vorfahrSuchen(s.leser(), []string{hx(50)}, hx(100002), start); ja || !voll {
		t.Fatalf("Seitenzweig als Vorfahr (%v, %v)", ja, voll)
	}
	// Ein Nachfahre ist kein Vorfahr.
	if _, ja, _, _ := vorfahrSuchen(s.leser(), []string{hx(10)}, hx(20), start); ja {
		t.Fatal("Nachfahre als Vorfahr")
	}
	// Unter der Grenze wird nicht weiter abgestiegen.
	if _, ja, voll, _ := vorfahrSuchen(s.leser(), []string{hx(50)}, hx(5), start+30); ja || !voll {
		t.Fatalf("unter der Grenze gefunden (%v, %v)", ja, voll)
	}
	// Eine Luecke: nicht gefunden UND unvollstaendig -- kein "kein Vorfahr".
	delete(s.k, hx(30))
	if _, ja, voll, _ := vorfahrSuchen(s.leser(), []string{hx(50)}, hx(10), start); ja || voll {
		t.Fatalf("mit Luecke: gefunden=%v vollstaendig=%v", ja, voll)
	}
	// Lesefehler: Fehler.
	s.fehler = errors.New("DB weg")
	if _, _, _, err := vorfahrSuchen(s.leser(), []string{hx(50)}, hx(10), start); err == nil {
		t.Fatal("Lesefehler verschluckt")
	}
}

// Grenze: ein breiter Kegel (viele Eltern je Block) bricht bei
// vorfahrBesuchsGrenze ab -- unvollstaendig, kein Urteil.
func TestVorfahrSuchen_Grenze(t *testing.T) {
	alt := vorfahrBesuchsGrenze
	vorfahrBesuchsGrenze = 1000
	t.Cleanup(func() { vorfahrBesuchsGrenze = alt })
	s := &speicherKette{k: map[string]blockKopf{}}
	// 12 Ebenen zu je 256 Bloecken, jeder Block verweist auf alle der Ebene darunter.
	ebene := func(e, i int) string { return fmt.Sprintf("%032x%032x", e, i) }
	for e := 0; e < 12; e++ {
		for i := 0; i < 256; i++ {
			var eltern []string
			if e > 0 {
				for j := 0; j < 256; j++ {
					eltern = append(eltern, ebene(e-1, j))
				}
			}
			s.k[ebene(e, i)] = blockKopf{hoehe: int64(e), zeit: 1_800_000_000 + int64(e), eltern: eltern}
		}
	}
	_, ja, _, err := vorfahrSuchen(s.leser(), []string{ebene(11, 0)}, "nicht-da", 0)
	if !errors.Is(err, errVorfahrZuGross) || ja {
		t.Fatalf("ueber der Grenze: gefunden=%v err=%v", ja, err)
	}
	if s.gelesen > vorfahrBesuchsGrenze {
		t.Fatalf("%d Koepfe gelesen -- Grenze %d", s.gelesen, vorfahrBesuchsGrenze)
	}
}

// Die Konsens-Variante des Checkpoints: Bezugszeit ist der tragende Block,
// der Checkpoint-Block muss in seiner Vergangenheit liegen.
func TestCheckpointImTragendenBlock(t *testing.T) {
	f := neuerBindungFall(t)
	start := bindungZeit - 500
	s := lineareKette(600, start) // Block 500 hat Zeit bindungZeit
	cpHash := hx(500)
	s.k[cpHash] = blockKopf{hoehe: 500, zeit: bindungZeit, eltern: []string{hx(499)}}
	z := f.zertifikat(t, 500, cpHash, bindungZeit, f.keys[:3])
	tragend := []string{hx(600)}
	tragZeit := start + 601

	if err := f.dag.checkpointImTragendenBlock(z, nil, tragend, tragZeit, s.leser()); err != nil {
		t.Fatalf("Gutfall: %v", err)
	}
	// Ohne lokalen Replay-Status: kein nachgespielter Block noetig.
	f.dag.replayedBlocks = map[string]bool{}
	if err := f.dag.checkpointImTragendenBlock(z, nil, tragend, tragZeit, s.leser()); err != nil {
		t.Fatalf("haengt am Replay-Status: %v", err)
	}

	seite := f.zertifikat(t, 4, hx(100002), start+4, f.keys[:3])
	for name, x := range map[string]struct {
		z      CheckpointZertifikat
		eltern []string
		zeit   int64
	}{
		"nicht in der Vergangenheit": {z, []string{hx(400)}, tragZeit},
		"Seitenzweig":                {seite, tragend, start + 300},
		"zu alt":                     {z, tragend, bindungZeit + checkpointHoechstensAlt + 1},
		"nach dem tragenden Block":   {z, tragend, bindungZeit - zeitstempelRueckToleranz - 1},
	} {
		if err := f.dag.checkpointImTragendenBlock(x.z, nil, x.eltern, x.zeit, s.leser()); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	// Hoehe passt nicht zum Block (vom Komitee so unterschrieben).
	falsch := f.zertifikat(t, 499, cpHash, bindungZeit, f.keys[:3])
	if f.dag.checkpointImTragendenBlock(falsch, nil, tragend, tragZeit, s.leser()) == nil {
		t.Fatal("falsche Hoehe angenommen")
	}
	// Luecke auf dem Weg: Ablehnung, nicht "kein Vorfahr".
	delete(s.k, hx(550))
	err := f.dag.checkpointImTragendenBlock(z, nil, tragend, tragZeit, s.leser())
	if err == nil || !strings.Contains(err.Error(), "unvollstaendig") {
		t.Fatalf("Luecke: %v", err)
	}
}

// Eine lange schmale Kette: hoechstens vorfahrEbenenGrenze Abfrage-Runden.
func TestVorfahrSuchen_Ebenengrenze(t *testing.T) {
	alt := vorfahrEbenenGrenze
	vorfahrEbenenGrenze = 64
	t.Cleanup(func() { vorfahrEbenenGrenze = alt })
	s := lineareKette(500, 1_800_000_000)
	_, ja, _, err := vorfahrSuchen(s.leser(), []string{hx(500)}, hx(1), 0)
	if !errors.Is(err, errVorfahrZuGross) || ja {
		t.Fatalf("lange Kette: gefunden=%v err=%v", ja, err)
	}
	if s.abfragen > vorfahrEbenenGrenze {
		t.Fatalf("%d Abfragen, Grenze %d", s.abfragen, vorfahrEbenenGrenze)
	}
	// Innerhalb der Ebenengrenze: gefunden.
	if _, ja, _, err := vorfahrSuchen(s.leser(), []string{hx(500)}, hx(500-vorfahrEbenenGrenze+1), 0); !ja || err != nil {
		t.Fatalf("innerhalb der Ebenengrenze: %v %v", ja, err)
	}
}

// Eltern-Verweise werden gezaehlt, bevor sie expandiert werden.
func TestVorfahrSuchen_Elterngrenze(t *testing.T) {
	alt := vorfahrElternGrenze
	vorfahrElternGrenze = 100
	t.Cleanup(func() { vorfahrElternGrenze = alt })
	s := &speicherKette{k: map[string]blockKopf{}}
	var eltern []string
	for i := 0; i < 200; i++ {
		eltern = append(eltern, hx(10_000+i))
	}
	s.k[hx(1)] = blockKopf{hoehe: 2, zeit: 1_800_000_000, eltern: eltern}
	if _, _, _, err := vorfahrSuchen(s.leser(), []string{hx(1)}, "nicht-da", 0); !errors.Is(err, errVorfahrZuGross) {
		t.Fatalf("Elterngrenze: %v", err)
	}
	// Mehr als peerElternGrenze direkte Eltern: sofort.
	viele := make([]string, peerElternGrenze+1)
	for i := range viele {
		viele[i] = hx(20_000 + i)
	}
	if _, _, _, err := vorfahrSuchen(s.leser(), viele, "x", 0); !errors.Is(err, errVorfahrZuGross) {
		t.Fatalf("zu viele direkte Eltern: %v", err)
	}
}

// Eine rueckdatierte Zwischenkette schneidet einen echten Vorfahr ab --
// fuer jeden Knoten gleich (Definition "ueber Bloecke mit Zeit >= grenze").
func TestVorfahrSuchen_RueckdatierteKette(t *testing.T) {
	const z = int64(1_800_000_000)
	s := &speicherKette{k: map[string]blockKopf{hx(1): {hoehe: 1, zeit: z}}}
	vor, zeit := hx(1), z
	for i := 2; i <= 8; i++ {
		zeit -= zeitstempelRueckToleranz
		s.k[hx(i)] = blockKopf{hoehe: int64(i), zeit: zeit, eltern: []string{vor}}
		vor = hx(i)
	}
	if _, ja, voll, err := vorfahrSuchen(s.leser(), []string{vor}, hx(1), z-checkpointHoechstensAlt); ja || !voll || err != nil {
		t.Fatalf("rueckdatierte Kette: gefunden=%v vollstaendig=%v err=%v", ja, voll, err)
	}
}

func TestCheckpointImTragendenBlock_Grenzen(t *testing.T) {
	f := neuerBindungFall(t)
	start := bindungZeit - 500
	s := lineareKette(600, start)
	z := f.zertifikat(t, 500, hx(500), bindungZeit, f.keys[:3])
	tragend := []string{hx(600)}
	for zeit, ok := range map[int64]bool{
		bindungZeit + checkpointHoechstensAlt:      true,
		bindungZeit + checkpointHoechstensAlt + 1:  false,
		bindungZeit - zeitstempelRueckToleranz:     true,
		bindungZeit - zeitstempelRueckToleranz - 1: false,
	} {
		if err := f.dag.checkpointImTragendenBlock(z, nil, tragend, zeit, s.leser()); (err == nil) != ok {
			t.Fatalf("tragender Block Blockzeit%+d: %v, erwartet ok=%v", zeit-bindungZeit, err, ok)
		}
	}
	// Monotonie: Wiederholung und aelterer nach neuerem.
	for _, l := range []*KettenCheckpoint{{Zeit: bindungZeit}, {Zeit: bindungZeit + 1}} {
		if f.dag.checkpointImTragendenBlock(z, l, tragend, bindungZeit+5, s.leser()) == nil {
			t.Fatalf("nach letztem %+v angenommen", *l)
		}
	}
}

// Ein gefaelschtes oder unvollstaendiges Zertifikat loest die Suche in der
// Datenbank gar nicht erst aus.
func TestCheckpointImTragendenBlock_KryptoVorSuche(t *testing.T) {
	f := neuerBindungFall(t)
	s := lineareKette(600, bindungZeit-500)
	z := f.zertifikat(t, 500, hx(500), bindungZeit, f.keys[:3])
	z.Unterschriften[0].Sig = z.Unterschriften[1].Sig // gefaelscht
	if f.dag.checkpointImTragendenBlock(z, nil, []string{hx(600)}, bindungZeit+5, s.leser()) == nil {
		t.Fatal("gefaelschte Unterschrift angenommen")
	}
	if s.abfragen != 0 {
		t.Fatalf("%d Abfragen fuer ein gefaelschtes Zertifikat", s.abfragen)
	}
}

// Was nur diesen Knoten betrifft, ist "nicht pruefbar", nicht "ungueltig";
// eine Grenze bei vollstaendigen Daten ist ein Urteil.
func TestCheckpointImTragendenBlock_NichtPruefbar(t *testing.T) {
	f := neuerBindungFall(t)
	s := lineareKette(600, bindungZeit-500)
	z := f.zertifikat(t, 500, hx(500), bindungZeit, f.keys[:3])
	tragend := []string{hx(600)}
	s.fehler = errors.New("DB weg")
	if err := f.dag.checkpointImTragendenBlock(z, nil, tragend, bindungZeit+5, s.leser()); !errors.Is(err, errCheckpointNichtPruefbar) {
		t.Fatalf("Lesefehler: %v", err)
	}
	s.fehler = nil
	delete(s.k, hx(550))
	if err := f.dag.checkpointImTragendenBlock(z, nil, tragend, bindungZeit+5, s.leser()); !errors.Is(err, errCheckpointNichtPruefbar) {
		t.Fatalf("Luecke: %v", err)
	}
	f.dag.state.erzeugerRegister.Store(nil)
	if err := f.dag.checkpointImTragendenBlock(z, nil, tragend, bindungZeit+5, s.leser()); !errors.Is(err, errCheckpointNichtPruefbar) {
		t.Fatalf("ohne Stand: %v", err)
	}
	f2 := neuerBindungFall(t)
	s2 := lineareKette(600, bindungZeit-500)
	z2 := f2.zertifikat(t, 500, hx(500), bindungZeit, f2.keys[:3])
	alt := vorfahrBesuchsGrenze
	vorfahrBesuchsGrenze = 10
	t.Cleanup(func() { vorfahrBesuchsGrenze = alt })
	err := f2.dag.checkpointImTragendenBlock(z2, nil, []string{hx(600)}, bindungZeit+5, s2.leser())
	if err == nil || errors.Is(err, errCheckpointNichtPruefbar) {
		t.Fatalf("Grenze bei vollstaendigen Daten muss ein Urteil sein: %v", err)
	}
}

// Gleiche Geschichte, gleiche Eltern-Reihenfolge: gleiches Urteil, auch an
// der Grenze (die Reihenfolge steht im Blockhash).
func TestVorfahrSuchen_GleichesUrteil(t *testing.T) {
	alt := vorfahrBesuchsGrenze
	vorfahrBesuchsGrenze = 30
	t.Cleanup(func() { vorfahrBesuchsGrenze = alt })
	for lauf := 0; lauf < 5; lauf++ {
		s := lineareKette(100, 1_800_000_000)
		_, ja, voll, err := vorfahrSuchen(s.leser(), []string{hx(100), hx(99)}, hx(75), 0)
		if !ja || !voll || err != nil {
			t.Fatalf("Lauf %d: %v %v %v", lauf, ja, voll, err)
		}
	}
}
