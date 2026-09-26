// fsync-messung misst, wie schnell eine Platte Daten dauerhaft macht -- in
// genau der Form, in der der WAL schreibt (x/humanity/wal): vorbelegte Datei,
// kleine Datensaetze, fdatasync nach jedem.
//
// Stufe 1.4 (docs/SKALIERUNG_DEZENTRAL.md): "Mit nur einer Platte: die
// fsync-Latenz des neuen Servers messen, bevor irgendetwas geschaetzt wird."
// Und bei zwei Platten: beide messen, bevor AEQUITAS_WAL_PATH umgestellt wird.
//
// Aufruf, auf der Box selbst, ohne dass der Knoten angehalten werden muss:
//
//	go run ./tools/fsync-messung -dir /var/lib/aequitas -dir /mnt/zweite -n 2000
//
// Ausgabe je Verzeichnis: Syncs je Sekunde, Perzentile (p50, p90, p99,
// Hoechstwert) und was das fuer den WAL heisst -- die Obergrenze ist
// Datensaetze-je-Buendel geteilt durch die Sync-Zeit.
//
// Die Messung legt je Verzeichnis eine Datei von wenigen MB an und loescht sie
// danach. Sie misst die Platte UNTER dem, was gerade sonst darauf laeuft --
// das ist gewollt: genau das sieht auch der WAL.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type verzeichnisse []string

func (v *verzeichnisse) String() string     { return strings.Join(*v, ",") }
func (v *verzeichnisse) Set(s string) error { *v = append(*v, s); return nil }

// Ergebnis einer Messung, in Mikrosekunden.
type Ergebnis struct {
	Verzeichnis            string
	Anzahl                 int
	JeSekunde              float64
	P50, P90, P99, Hoechst float64
}

func main() {
	var dirs verzeichnisse
	flag.Var(&dirs, "dir", "Verzeichnis auf der zu messenden Platte (mehrfach moeglich)")
	n := flag.Int("n", 2000, "Anzahl Syncs je Verzeichnis")
	groesse := flag.Int("satz", 256, "Bytes je Datensatz (ein WAL-Satz ist ~150-300 Bytes)")
	buendel := flag.Int("buendel", 64, "angenommene Datensaetze je Gruppen-Commit fuer die Schaetzung")
	flag.Parse()
	if len(dirs) == 0 {
		dirs = verzeichnisse{"."}
	}
	var alle []Ergebnis
	for _, d := range dirs {
		e, err := Messen(d, *n, *groesse)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", d, err)
			os.Exit(1)
		}
		alle = append(alle, e)
		fmt.Println(Bericht(e, *buendel))
	}
	if len(alle) > 1 {
		sort.Slice(alle, func(i, j int) bool { return alle[i].P99 < alle[j].P99 })
		fmt.Printf("\nFuer den WAL (AEQUITAS_WAL_PATH) am besten: %s -- niedrigstes p99 (%.0f µs).\n"+
			"Entscheidend ist p99, nicht der Median: jeder Aufrufer eines Buendels wartet auf den langsamsten Sync.\n",
			alle[0].Verzeichnis, alle[0].P99)
	}
}

// Messen schreibt n Datensaetze in eine vorbelegte Datei in dir und macht
// jeden einzeln dauerhaft.
func Messen(dir string, n, satz int) (Ergebnis, error) {
	if n < 1 || satz < 1 {
		return Ergebnis{}, fmt.Errorf("n und satz muessen positiv sein")
	}
	f, err := os.CreateTemp(dir, ".fsync-messung-*")
	if err != nil {
		return Ergebnis{}, err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()

	// Vorbelegen und EINMAL voll synchronisieren -- wie der WAL (wal.go):
	// danach aendert sich die Dateigroesse nicht mehr, und jeder weitere Sync
	// muss nur noch Daten schreiben.
	gesamt := int64(n) * int64(satz)
	if err := vorbelegen(f, gesamt); err != nil {
		return Ergebnis{}, fmt.Errorf("vorbelegen: %w", err)
	}
	if err := f.Sync(); err != nil {
		return Ergebnis{}, err
	}

	daten := make([]byte, satz)
	for i := range daten {
		daten[i] = byte(i)
	}
	dauer := make([]float64, n)
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, err := f.WriteAt(daten, int64(i)*int64(satz)); err != nil {
			return Ergebnis{}, err
		}
		t0 := time.Now()
		if err := datensync(f); err != nil {
			return Ergebnis{}, err
		}
		dauer[i] = float64(time.Since(t0).Nanoseconds()) / 1000
	}
	gesamtZeit := time.Since(start)
	abs, _ := filepath.Abs(dir)
	return Auswerten(abs, dauer, gesamtZeit), nil
}

// Auswerten: Perzentile aus einzelnen Sync-Dauern (µs).
func Auswerten(dir string, dauerUs []float64, gesamt time.Duration) Ergebnis {
	s := append([]float64(nil), dauerUs...)
	sort.Float64s(s)
	p := func(q float64) float64 {
		if len(s) == 0 {
			return 0
		}
		i := int(q*float64(len(s)-1) + 0.5)
		return s[i]
	}
	e := Ergebnis{Verzeichnis: dir, Anzahl: len(s), P50: p(0.50), P90: p(0.90), P99: p(0.99)}
	if len(s) > 0 {
		e.Hoechst = s[len(s)-1]
	}
	if gesamt > 0 {
		e.JeSekunde = float64(len(s)) / gesamt.Seconds()
	}
	return e
}

// Bericht: eine Zeile Zahlen und was sie fuer den WAL bedeuten.
func Bericht(e Ergebnis, buendel int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n  %d Syncs, %.0f/s   p50 %.0f µs   p90 %.0f µs   p99 %.0f µs   max %.0f µs\n",
		e.Verzeichnis, e.Anzahl, e.JeSekunde, e.P50, e.P90, e.P99, e.Hoechst)
	if e.P50 > 0 {
		fmt.Fprintf(&b, "  WAL-Obergrenze bei %d Saetzen je Buendel: ~%.0f Ueberweisungen/s (Median), ~%.0f/s (p99)\n",
			buendel, float64(buendel)*1e6/e.P50, float64(buendel)*1e6/nichtNull(e.P99))
	}
	switch {
	case e.P99 > 20_000:
		b.WriteString("  Einschaetzung: p99 ueber 20 ms -- diese Platte bremst den WAL. Zweite Platte oder anderer Speicher pruefen.")
	case e.P99 > 5_000:
		b.WriteString("  Einschaetzung: brauchbar, aber Ausreisser; MaxBatchWait (batch_tuning.go) an dieser Platte vermessen.")
	default:
		b.WriteString("  Einschaetzung: schnell genug; der WAL wird hier nicht die Grenze sein.")
	}
	return b.String()
}

func nichtNull(x float64) float64 {
	if x <= 0 {
		return 1
	}
	return x
}
