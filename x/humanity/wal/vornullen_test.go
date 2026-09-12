package wal

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fuellen schreibt nebenlaeufig, damit der Gruppen-Commit die fsyncs buendelt.
func fuellen(t *testing.T, w *WAL, anzahl int, nutzlast []byte) {
	t.Helper()
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < anzahl/32; i++ {
				if _, err := w.Append(nutzlast); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func warteBisNullerFertig(w *WAL) {
	for i := 0; i < 20000 && w.nuller.laeuft.Load(); i++ {
		time.Sleep(time.Millisecond)
	}
}

// Nach der Haelfte des ersten Chunks muss der Nuller den zweiten vorbereitet
// haben, und die Uebernahme darf keinen Datensatz kosten -- auch nicht die
// ueber der Chunkgrenze.
func TestVornullen_ChunkWirdUebernommenUndNichtsGehtVerloren(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// 40 MB: ueber der Haelfte des 64-MB-Chunks, unter seinem Ende.
	nutzlast := bytes.Repeat([]byte("x"), 4096)
	fuellen(t, w, 10000, nutzlast)
	warteBisNullerFertig(w)

	st := w.VornullerStand()
	if st["laeufe"].(int64) < 1 {
		t.Fatal("der Nuller ist nach der Haelfte des ersten Chunks nicht angelaufen")
	}
	if b := w.nuller.bereich.Load(); b == nil || b.bis != 2*int64(preallocChunk) {
		t.Fatalf("vorgenullter Bereich = %+v, erwartet bis %d (Ende des zweiten Chunks)", b, 2*preallocChunk)
	}

	// Ueber die Grenze schreiben: weitere 40 MB.
	fuellen(t, w, 10000, nutzlast)
	if st := w.VornullerStand(); st["rueckfaelle"].(int64) != 0 {
		t.Errorf("rueckfaelle = %v, erwartet 0: der Nuller war laengst fertig, es gab nichts zu warten", st["rueckfaelle"])
	}
	w.mu.Lock()
	allocEnd := w.allocEnd
	w.mu.Unlock()
	if allocEnd < 2*int64(preallocChunk) {
		t.Errorf("allocEnd = %d, der vorgenullte zweite Chunk wurde nicht uebernommen", allocEnd)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	n, truncated, err := ReplayFile(path, func(e Entry) error {
		if !bytes.Equal(e.Payload, nutzlast) {
			return fmt.Errorf("Seq %d: Nutzlast beschaedigt (%d Bytes)", e.Seq, len(e.Payload))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("das Replay meldet einen abgeschnittenen Schwanz -- ein Datensatz ist halb")
	}
	if n != 2*(10000/32)*32 {
		t.Errorf("%d Datensaetze im Replay, erwartet %d", n, 2*(10000/32)*32)
	}
}

// DER GEFAEHRLICHE FALL. Der Nuller schreibt Nullen in [allocEnd, +64MB).
// Kommt der Schreiber dort an, bevor der Nuller fertig ist, darf er NICHT
// mit fallocate ueber diesen Bereich hinweg reservieren und hineinschreiben --
// der Nuller wuerde anschliessend seine Datensaetze mit Nullen ueberschreiben.
// Er muss warten. Erzwungen mit einem kuenstlich langsamen Nuller.
func TestVornullen_SchreiberWartetStattInDenNullerZuSchreiben(t *testing.T) {
	alt := nullenFn
	t.Cleanup(func() { nullenFn = alt })
	nullenFn = func(f *os.File, off, n int64) error {
		time.Sleep(1500 * time.Millisecond) // laenger, als das Auffuellen braucht
		return nullen(f, off, n)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	nutzlast := bytes.Repeat([]byte("y"), 8192)
	// 48 MB: Nuller startet bei 32 MB und schlaeft; die restlichen 16 MB
	// kommen deutlich vor Ablauf der 1,5 s -- dann ist der Chunk mit 64 MB
	// voll, und ensureCapacity trifft den laufenden Nuller.
	fuellen(t, w, 6000, nutzlast)
	// Und weiter ueber die Grenze.
	start := time.Now()
	fuellen(t, w, 4000, nutzlast)
	dauer := time.Since(start)

	st := w.VornullerStand()
	if st["rueckfaelle"].(int64) < 1 {
		t.Fatalf("rueckfaelle = %v: der Schreiber hat NICHT gewartet -- entweder hat er ueber den "+
			"laufenden Nuller hinweg reserviert (Datenverlust droht), oder der Test hat den Fall "+
			"nicht erzwungen (Dauer %s)", st["rueckfaelle"], dauer)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	n, truncated, err := ReplayFile(path, func(e Entry) error {
		if !bytes.Equal(e.Payload, nutzlast) {
			return fmt.Errorf("Seq %d: Nutzlast beschaedigt -- der Nuller hat Datensaetze ueberschrieben", e.Seq)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("abgeschnittener Schwanz")
	}
	erwartet := (6000/32)*32 + (4000/32)*32
	if n != erwartet {
		t.Errorf("%d Datensaetze, erwartet %d -- Datensaetze sind unter Nullen verschwunden", n, erwartet)
	}
}

// Nach der Kompaktierung ist die neue Datei vorgenullt, nicht nur reserviert
// -- und der Nuller-Eintrag der alten Datei darf nicht fuer die neue gelten.
func TestVornullen_KompaktierungNulltVorUndAlterEintragVerfaellt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	nutzlast := bytes.Repeat([]byte("z"), 4096)
	fuellen(t, w, 10000, nutzlast) // 40 MB, Nuller laeuft an
	warteBisNullerFertig(w)
	alteDatei := w.file
	if err := w.TruncateBefore(5000); err != nil {
		t.Fatal(err)
	}
	if w.file == alteDatei {
		t.Fatal("die Kompaktierung hat die Datei nicht getauscht")
	}
	if _, bereit := func() (int64, bool) { w.mu.Lock(); defer w.mu.Unlock(); return w.vorgenulltBereit(1) }(); bereit {
		t.Error("der vorgenullte Bereich der ALTEN Datei gilt noch fuer die neue")
	}
	// Die neue Datei muss hinter den Daten beschriebene Nullen haben, nicht
	// nur reservierten Platz: Groesse = Daten + Chunk, und dort stehen Nullen.
	fi, _ := os.Stat(path)
	w.mu.Lock()
	writeOff, allocEnd := w.writeOff, w.allocEnd
	w.mu.Unlock()
	if fi.Size() < allocEnd || allocEnd != writeOff+int64(preallocChunk) {
		t.Errorf("Groesse %d, writeOff %d, allocEnd %d: die neue Datei ist nicht um einen Chunk vorbelegt", fi.Size(), writeOff, allocEnd)
	}
	fuellen(t, w, 3000, nutzlast)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	n, _, err := ReplayFile(path, func(e Entry) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n != (10000/32)*32-4999+(3000/32)*32 {
		t.Errorf("%d Datensaetze nach Kompaktierung+Anhaengen, erwartet %d", n, (10000/32)*32-4999+(3000/32)*32)
	}
}
