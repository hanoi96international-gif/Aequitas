package keeper

import (
	"strings"
	"sync"
	"testing"
)

func rueckstauTestAnfang(t *testing.T, grenze string) {
	t.Helper()
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", grenze)
	alt := rueckstauMesserAn.Load()
	rueckstauMesserAn.Store(true)
	rueckstauAktuell.Store(0)
	rueckstauZugelassen.Store(0)
	t.Cleanup(func() {
		rueckstauMesserAn.Store(alt)
		rueckstauAktuell.Store(0)
		rueckstauZugelassen.Store(0)
	})
}

func TestRueckstau_GrenzeLehntAbUndGibtFrei(t *testing.T) {
	rueckstauTestAnfang(t, "1000")

	rueckstauAktuell.Store(999)
	if g := rueckstauGrund(); g != "" {
		t.Fatalf("unter der Grenze abgelehnt: %s", g)
	}
	if g := rueckstauPlatzNehmen(); g != "" {
		t.Fatalf("letzter Platz nicht vergeben: %s", g)
	}
	// 999 gemessen + 1 zugelassen = voll: die naechste bekommt -32005.
	if g := rueckstauPlatzNehmen(); !strings.Contains(g, "server busy") {
		t.Fatalf("ueber der Grenze angenommen: %q", g)
	}
	if r := admissionRefusalReason(); r == "" {
		t.Fatal("admissionRefusalReason laesst den Rueckstau durch")
	}
	// Die Messung sieht den Block abgetragen: wieder frei.
	rueckstauMessungUebernehmen(0, rueckstauZugelassen.Load())
	if g := rueckstauPlatzNehmen(); g != "" {
		t.Fatalf("nach dem Block weiter gesperrt: %s", g)
	}
	// Vorgabe ohne Mehrfach-Takt: EIN voller Block; 0 schaltet ab.
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "")
	t.Setenv("ENABLE_MULTI_BLOCK_TICK", "")
	blockDeckelZuletzt.Store(0)
	if rueckstauMax() != int64(blockTxHartDeckel()) {
		t.Fatalf("Vorgabe %d", rueckstauMax())
	}
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "0")
	rueckstauAktuell.Store(1 << 40)
	if g := rueckstauPlatzNehmen(); g != "" {
		t.Fatalf("abgeschaltet und trotzdem abgelehnt: %s", g)
	}
}

// Zwischen zwei Messungen darf ein Ansturm die Grenze nicht ueberrennen:
// 50.000 gleichzeitige Anfragen, Grenze 1.000 -- genau 1.000 kommen durch.
func TestRueckstau_AnsturmZwischenZweiMessungen(t *testing.T) {
	rueckstauTestAnfang(t, "1000")
	var wg sync.WaitGroup
	var mu sync.Mutex
	durch := 0
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := 0
			for i := 0; i < 1000; i++ {
				if rueckstauPlatzNehmen() == "" {
					n++
				}
			}
			mu.Lock()
			durch += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	if durch != 1000 {
		t.Fatalf("%d zugelassen, Grenze 1000", durch)
	}
}

// Haengt der Messer, schliesst sich die Annahme von selbst (fail closed),
// statt ohne Grenze weiter anzunehmen.
func TestRueckstau_OhneMessungSchliesstSich(t *testing.T) {
	rueckstauTestAnfang(t, "10")
	for i := 0; i < 10; i++ {
		if g := rueckstauPlatzNehmen(); g != "" {
			t.Fatalf("Platz %d abgelehnt: %s", i, g)
		}
	}
	if rueckstauPlatzNehmen() == "" || rueckstauGrund() == "" {
		t.Fatal("ohne neue Messung weiter angenommen")
	}
	// Was waehrend der Abfrage zugelassen wurde, bleibt gezaehlt.
	vorher := rueckstauZugelassen.Load()
	rueckstauZugelassen.Add(3) // kam waehrend der Abfrage
	rueckstauMessungUebernehmen(4, vorher)
	if s := rueckstauStand(); s != 7 {
		t.Fatalf("Stand %d, erwartet 4 gemessen + 3 waehrend der Messung", s)
	}
}

// Mit ENABLE_MULTI_BLOCK_TICK=1 baut ein Takt bis zu 1+maxExtraBlocksPerTick
// Bloecke -- aber nur, wenn der vorige voll war. Eine Grenze von einem Block
// liess nie einen zweiten vollen entstehen (27.09.2026: 55 Bloecke, keiner
// voll, 9.202 Ablehnungen). Gebremst: ein Block mit dem gebremsten Deckel.
func TestRueckstau_GrenzeJeTaktMitMehrerenBloecken(t *testing.T) {
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "")
	t.Setenv("AEQUITAS_MAX_TXS_PER_BLOCK", "7000")
	alt, altUngebremst := blockDeckelZuletzt.Load(), blockDeckelZuletztUngebremst.Load()
	t.Cleanup(func() {
		blockDeckelZuletzt.Store(alt)
		blockDeckelZuletztUngebremst.Store(altUngebremst)
	})

	t.Setenv("ENABLE_MULTI_BLOCK_TICK", "1")
	merkeBlockDeckel(7000, 7000) // ungebremst
	if got, want := rueckstauMax(), int64(7000*(1+maxExtraBlocksPerTick)); got != want {
		t.Fatalf("Mehrfach-Takt ungebremst: Grenze %d, erwartet %d", got, want)
	}

	merkeBlockDeckel(1500, 7000) // Peer-Lag-Bremse: keine Zusatzbloecke
	if got := rueckstauMax(); got != 1500 {
		t.Fatalf("Mehrfach-Takt gebremst: Grenze %d, erwartet 1500 (ein gebremster Block)", got)
	}

	t.Setenv("ENABLE_MULTI_BLOCK_TICK", "")
	merkeBlockDeckel(7000, 7000)
	if got := rueckstauMax(); got != 7000 {
		t.Fatalf("ohne Mehrfach-Takt: Grenze %d, erwartet 7000", got)
	}

	t.Setenv("ENABLE_MULTI_BLOCK_TICK", "1")
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "12345")
	if got := rueckstauMax(); got != 12345 {
		t.Fatalf("ausdruecklich gesetzt: Grenze %d, erwartet 12345", got)
	}
}
