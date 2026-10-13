package keeper

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// speicherKette: Koepfe im Speicher, mit Zaehler der gelesenen Hashes.
type speicherKette struct {
	k       map[string]blockKopf
	gelesen int
	fehler  error
}

func (s *speicherKette) leser() kopfLeser {
	return func(hashes []string) (map[string]blockKopf, error) {
		if s.fehler != nil {
			return nil, s.fehler
		}
		s.gelesen += len(hashes)
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
	_, ja, voll, err := vorfahrSuchen(s.leser(), []string{ebene(11, 0)}, "nicht-da", 0)
	if err != nil || ja || voll {
		t.Fatalf("ueber der Grenze: gefunden=%v vollstaendig=%v err=%v", ja, voll, err)
	}
	if s.gelesen > vorfahrBesuchsGrenze+256*256 {
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
