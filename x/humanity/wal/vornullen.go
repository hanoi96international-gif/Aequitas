package wal

import (
	"os"
	"sync/atomic"
)

// VORNULLEN: DIE EXTENTS MUESSEN BESCHRIEBEN SEIN, NICHT NUR RESERVIERT.
//
// GEMESSEN AM 12.09.2026 UNTER LAST, beide Boxen, dieselbe Platte, dieselbe
// Minute, Schreibvorgaenge in der Groesse eines WAL-Batches (16 KB):
//
//	fallocate(0), unbeschriebene Extents   p50 3,5-5,2 ms   Mittel  7,7-10,6 ms
//	vorgenullt, beschriebene Extents       p50 0,5-0,8 ms   Mittel  1,6- 1,7 ms
//
// Die erste Zeile ist exakt das, was der WAL-Schreiber meldete (p50 4,5-5,9
// ms, Mittel 7,4-9,1). Die zweite ist sechsmal schneller.
//
// WARUM. fallocate reserviert Bloecke, markiert sie aber als "unwritten": ein
// Lesen liefert Nullen, ohne dass Nullen auf der Platte stehen. Der erste
// Schreibvorgang in so einen Bereich wandelt den Extent in "written" um -- das
// ist eine Metadatenaenderung, und fdatasync muss sie ins Journal bringen,
// sonst laese man nach einem Absturz Nullen statt der Daten. Ein Journal-Commit
// auf ext4 mit data=ordered schreibt dabei auch alle anderen schmutzigen Daten
// derselben Transaktion -- unter Last also Postgres. Bei 300 Byte je
// Schreibvorgang faellt das kaum auf (eine Seite je Sync); bei 16 KB sind es
// vier Seiten, und jede kann in einen frischen Extent-Abschnitt fallen.
//
// Steht der Bereich schon einmal beschrieben da (mit Nullen), ist der Extent
// "written", der Datensatz ueberschreibt Nullen, und fdatasync ist eine reine
// Datenspuelung ohne Journal.
//
// WARUM IM HINTERGRUND. 64 MB Nullen zu schreiben dauert eine halbe Sekunde.
// Unter w.mu waere das eine halbe Sekunde, in der kein Append durchkommt --
// alle 35 Sekunden bei 1,8 MB/s. Darum laeuft es einen Chunk VORAUS: sobald
// der Schreiber die Haelfte des aktuellen Chunks verbraucht hat, wird der
// naechste ausserhalb der Sperre reserviert, genullt und synchronisiert.
// Wenn die Kapazitaet dann gebraucht wird, ist nur noch allocEnd zu setzen.
//
// WAS PASSIERT, WENN DER SCHREIBER SCHNELLER IST ALS DAS NULLEN. Dann wartet
// ensureCapacity auf den Nuller. Das ist eine bewusste Entscheidung: der
// Nuller schreibt Nullen in genau den Bereich, den der Schreiber als
// naechstes braucht -- ein Rueckfall auf fallocate ueber diesen Bereich hinweg
// liesse den Nuller anschliessend Daten ueberschreiben. Warten kostet
// hoechstens den Rest einer halben Sekunde und tritt nur ein, wenn 32 MB
// Datensaetze schneller ankommen, als 64 MB Nullen geschrieben sind -- bei
// 1,8 MB/s gegen rund 150 MB/s praktisch nie. rueckfaelle zaehlt es.
//
// KEINE SPERRE IM NULLER. Er veroeffentlicht sein Ergebnis ueber einen
// atomaren Zeiger. Naehme er am Ende w.mu, koennte writeBatch (haelt w.mu,
// wartet auf ihn) ihn nie fertig werden lassen.

// vorgenullterBereich sagt: in dieser Datei sind die Extents bis hier
// beschrieben. Gilt nur, solange w.file dieselbe Datei ist -- nach einer
// Kompaktierung ist es eine andere, und der Eintrag verfaellt.
type vorgenullterBereich struct {
	datei *os.File
	bis   int64
}

type vornuller struct {
	laeuft  atomic.Bool
	bereich atomic.Pointer[vorgenullterBereich]
	// Was der Nuller gerade beschreibt, damit ensureCapacity diesen Bereich
	// nicht mit einem Rueckfall ueberfaehrt.
	laufendAb    atomic.Int64
	laufendDatei atomic.Pointer[os.File]
	laeufe       atomic.Int64
	fehler       atomic.Int64
	rueckfaelle  atomic.Int64
}

// nullen reserviert n Bytes ab off in f, beschreibt sie mit Nullen und
// synchronisiert. Auch die Kompaktierung (Schritt 2, ohne Sperre) nutzt es.
func nullen(f *os.File, off, n int64) error {
	if err := preallocate(f, off, n); err != nil {
		return err
	}
	const stueck = 1 << 20
	z := make([]byte, stueck)
	for geschrieben := int64(0); geschrieben < n; {
		l := int64(stueck)
		if rest := n - geschrieben; rest < l {
			l = rest
		}
		if _, err := f.WriteAt(z[:l], off+geschrieben); err != nil {
			return err
		}
		geschrieben += l
	}
	return f.Sync()
}

// nullenFn ist nullen -- als Variable, damit ein Test den Nuller kuenstlich
// langsam machen und den Fall "Schreiber ueberholt Nuller" erzwingen kann.
var nullenFn = nullen

// vornullenAnstossen startet den Nuller fuer [ab, ab+preallocChunk) in f,
// falls keiner laeuft. Der Aufrufer haelt w.mu; der Nuller selbst nie.
func (w *WAL) vornullenAnstossen(f *os.File, ab int64) {
	if !w.nuller.laeuft.CompareAndSwap(false, true) {
		return
	}
	w.nuller.laufendDatei.Store(f)
	w.nuller.laufendAb.Store(ab)
	go func() {
		defer w.nuller.laeuft.Store(false)
		w.nuller.laeufe.Add(1)
		if err := nullenFn(f, ab, int64(preallocChunk)); err != nil {
			// Typisch nach einer Kompaktierung: die Datei ist geschlossen.
			// Dann gibt es nichts zu veroeffentlichen; der naechste Anstoss
			// nimmt die neue Datei.
			w.nuller.fehler.Add(1)
			return
		}
		w.nuller.bereich.Store(&vorgenullterBereich{datei: f, bis: ab + int64(preallocChunk)})
	}()
}

// vorgenulltBereit sagt, ob fuer w.file ein vorgenullter Bereich vorliegt,
// der writeOff+n aufnimmt. Aufrufer haelt w.mu.
func (w *WAL) vorgenulltBereit(n int64) (int64, bool) {
	b := w.nuller.bereich.Load()
	if b == nil || b.datei != w.file || b.bis <= w.allocEnd || w.writeOff+n > b.bis {
		return 0, false
	}
	return b.bis, true
}

// VornullerStand fuer die Anzeige.
func (w *WAL) VornullerStand() map[string]interface{} {
	var bis int64
	if b := w.nuller.bereich.Load(); b != nil {
		bis = b.bis
	}
	return map[string]interface{}{
		"bedeutung": "Nullt den naechsten 64-MB-Chunk der WAL-Datei im Hintergrund vor. Beschriebene " +
			"Extents machen fdatasync zur reinen Datenspuelung: gemessen 12.09.2026 p50 0,5-0,8 ms " +
			"statt 3,5-5,2 ms. rueckfaelle>0 heisst: der Schreiber war schneller als das Nullen.",
		"laeufe":         w.nuller.laeufe.Load(),
		"fehler":         w.nuller.fehler.Load(),
		"rueckfaelle":    w.nuller.rueckfaelle.Load(),
		"vorgenullt_bis": bis,
		"laeuft":         w.nuller.laeuft.Load(),
	}
}
