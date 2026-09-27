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
	// Vorgabe: EIN voller Block; 0 schaltet ab.
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "")
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
