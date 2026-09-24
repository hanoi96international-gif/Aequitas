package keeper

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// VORLADEN: den Ausgangskorb fuer den NAECHSTEN Block laden, waehrend dieser
// noch gebaut wird.
//
// WARUM. Seit dem 22.09.2026 nimmt nur Contabo1 Ueberweisungen an
// (annahme_tor.go), und jede davon muss durch LoadPendingTxsWithLimit: SELECT
// ... FOR UPDATE SKIP LOCKED ueber bis zu 7.000 Zeilen, dann UPDATE derselben
// Zeilen, dann JSON-Dekodierung -- gegen 32 parallele WAL-Flushes, die in
// dieselbe Tabelle schreiben. ProduceBlock wartete darauf, BEVOR es ueberhaupt
// die Sperre anfragte (siehe block.go), und der Mehrfachblock-Takt baut den
// naechsten Block erst, wenn der vorige fertig ist. Das Laden lag also Block
// fuer Block voll auf dem kritischen Pfad.
//
// Jetzt startet ProduceBlock, sobald es einen VOLLEN Korb geladen hat (es gibt
// also Rueckstand), das Laden fuer den naechsten Block im Hintergrund. Das
// laeuft neben Sperre, StateRoot, Bau, Speichern und Verteilen des aktuellen
// Blocks; der naechste ProduceBlock findet seine Zeilen fertig vor.
//
// WAS GLEICH BLEIBT. Beansprucht wird genau wie bisher: LoadPendingTxsWithLimit
// setzt included_at und commitet. Jede Zeile gehoert damit genau einer Ladung,
// und keine zweite kann sie lesen. Die Reihenfolge bleibt die der
// Warteschlange: die Vorladung beginnt erst, NACHDEM die Ladung des aktuellen
// Blocks committet ist, bekommt also die naechsten Zeilen nach wal_seq.
//
// DIE DREI FAELLE, IN DENEN EINE VORLADUNG NICHT IN DEN NAECHSTEN BLOCK GEHT:
//
//  1. Der aktuelle Block wird NICHT gespeichert (ein Tor bricht ab). Dann gibt
//     ProduceBlock seine eigenen Zeilen frei -- und die Vorladung mit. Sonst
//     kaeme der naechste Block mit den SPAETEREN Ueberweisungen eines Absenders
//     vor dessen frueheren, die gerade wieder offen sind: eine Nonce-Luecke.
//  2. Der Deckel ist inzwischen kleiner (eine Bremse griff). Der Block nimmt
//     die ersten deckel Zeilen, der Rest wird freigegeben -- es ist das Ende
//     der Reihe, die Reihenfolge bleibt erhalten.
//  3. Die Vorladung ist aelter als vorladungHoechstalter. Sie wird freigegeben
//     und frisch geladen. Das haelt sie weit unter der Frist des Aufraeumers
//     (PendingLeichenAufraeumen, 10 min), der eine beanspruchte Zeile ohne
//     Block wieder oeffnet -- eine Vorladung, die er schon geoeffnet haette,
//     kaeme so nie in einen Block, und keine Zeile doppelt.
//
// Beim Herunterfahren gibt VorladungFreigeben die offene Vorladung zurueck,
// damit ein Redeploy unter Last keine Ueberweisungen bis zum Aufraeumer liegen
// laesst. Bei einem harten Absturz holt sie der Aufraeumer, wie jede Ladung,
// die ProduceBlock gerade in der Hand hielt.
//
// AEQUITAS_VORLADEN=0 schaltet es ab; dann laedt ProduceBlock wie bis zum
// 23.09.2026 jedes Mal selbst.
var vorladenAn = os.Getenv("AEQUITAS_VORLADEN") != "0"

const vorladungHoechstalter = 60 * time.Second

var (
	vorladungenGestartet atomic.Int64
	vorladungenGenutzt   atomic.Int64
	vorladungenVerworfen atomic.Int64
	vorladungenGekuerzt  atomic.Int64
	vorladungenVeraltet  atomic.Int64
	vorladungWartenNs    atomic.Int64 // Summe: wie lange ProduceBlock auf eine unfertige Vorladung wartete
)

type vorladung struct {
	fertig    chan struct{}
	txs       []Transaction
	ids       []int64
	dauer     time.Duration
	geladenUm time.Time
}

type vorlader struct {
	mu    sync.Mutex
	offen *vorladung
	aus   bool
}

// starten beginnt eine Vorladung, sofern keine offen ist. laden laeuft in
// einer eigenen Goroutine, nicht im produceBlockPool: der hat genau zwei
// Arbeiter fuer genau die zwei Auftraege, die ProduceBlock abwartet.
func (v *vorlader) starten(laden func() ([]Transaction, []int64)) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.aus || v.offen != nil {
		return false
	}
	vl := &vorladung{fertig: make(chan struct{})}
	v.offen = vl
	vorladungenGestartet.Add(1)
	SafeGoroutine("vorladen-ausgangskorb", func() {
		defer close(vl.fertig)
		start := time.Now()
		vl.txs, vl.ids = laden()
		vl.dauer = time.Since(start)
		vl.geladenUm = time.Now()
	})
	return true
}

// nehmen gibt die offene Vorladung heraus (fertig oder nicht) und leert den
// Platz. Nil, wenn keine offen ist.
func (v *vorlader) nehmen() *vorladung {
	v.mu.Lock()
	defer v.mu.Unlock()
	vl := v.offen
	v.offen = nil
	return vl
}

// verwerfen wartet eine offene Vorladung ab und gibt ihre Zeilen frei.
func (v *vorlader) verwerfen(freigeben func([]int64)) {
	vl := v.nehmen()
	if vl == nil {
		return
	}
	<-vl.fertig
	vorladungenVerworfen.Add(1)
	if len(vl.ids) > 0 {
		freigeben(vl.ids)
	}
}

// abschalten verhindert weitere Vorladungen und gibt die offene frei.
func (v *vorlader) abschalten(freigeben func([]int64)) {
	v.mu.Lock()
	v.aus = true
	v.mu.Unlock()
	v.verwerfen(freigeben)
}

// einloesen macht aus einer Vorladung die Ladung fuer DIESEN Block: abwarten,
// Alter und Deckel pruefen. Die Faelle 2 und 3 oben.
func (vl *vorladung) einloesen(deckel int, freigeben func([]int64), frischLaden func(int) ([]Transaction, []int64)) ([]Transaction, []int64) {
	warteStart := time.Now()
	<-vl.fertig
	vorladungWartenNs.Add(int64(time.Since(warteStart)))
	txs, ids := vl.txs, vl.ids
	if len(ids) > 0 && time.Since(vl.geladenUm) > vorladungHoechstalter {
		vorladungenVeraltet.Add(1)
		fmt.Printf("[BLOCK] Vorladung von %d Ueberweisungen ist %s alt -- freigegeben, frisch geladen\n",
			len(ids), time.Since(vl.geladenUm).Round(time.Second))
		freigeben(ids)
		return frischLaden(deckel)
	}
	if deckel > 0 && len(ids) > deckel && len(txs) == len(ids) {
		vorladungenGekuerzt.Add(1)
		freigeben(ids[deckel:])
		txs, ids = txs[:deckel], ids[:deckel]
	}
	vorladungenGenutzt.Add(1)
	return txs, ids
}

// VorladungFreigeben gibt beim Herunterfahren die offene Vorladung zurueck und
// verhindert weitere. Siehe oben.
func (dag *BlockDAG) VorladungFreigeben() {
	if dag == nil || dag.state == nil {
		return
	}
	dag.vorlauf.abschalten(dag.state.PendingTxIDsFreigeben)
}

// vorladenStand fuer /api/health/combined.
func vorladenStand() map[string]interface{} {
	genutzt := vorladungenGenutzt.Load()
	mittelWarten := 0.0
	if genutzt > 0 {
		mittelWarten = float64(vorladungWartenNs.Load()) / float64(genutzt) / 1e6
	}
	return map[string]interface{}{
		"an":               vorladenAn,
		"gestartet":        vorladungenGestartet.Load(),
		"genutzt":          genutzt,
		"verworfen":        vorladungenVerworfen.Load(),
		"gekuerzt":         vorladungenGekuerzt.Load(),
		"veraltet":         vorladungenVeraltet.Load(),
		"mittel_warten_ms": mittelWarten,
		"bedeutung": "Ausgangskorb fuer den naechsten Block, geladen waehrend der aktuelle gebaut wird. " +
			"genutzt = Bloecke, die ihre Zeilen fertig vorfanden; mittel_warten_ms = wie lange sie im Schnitt " +
			"noch auf das Laden warteten. verworfen = der Block davor wurde nicht gespeichert, die Zeilen " +
			"gingen zurueck in die Warteschlange.",
	}
}
