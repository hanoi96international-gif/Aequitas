package keeper

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// ANNAHME NUR, SOLANGE DIE AUFTRAEGE EINES BLOCKS ZEITLICH ZUSAMMENPASSEN
// (05.10.2026, zu block_tauglich.go).
//
// Ein Block traegt EINE Blockzeit, und jeder Auftrag mit Nachweis passt nur
// in ein Fenster um seine Unterschrift. Liegen im Ausgang Auftraege von vor
// einer Stunde neben Auftraegen von jetzt, gibt es keine gemeinsame Zeit --
// dann entsteht kein Block (fail-closed). Damit das bei ehrlichem Betrieb
// nicht vorkommt, haelt der annehmende Knoten die Annahme an,
//
//  1. solange der Ausgang von vor dem Start noch nicht verblockt ist: nach
//     einem Absturz traegt der erste Block die alten Auftraege mit der Zeit
//     ihrer Annahme, und erst danach kommen neue hinzu;
//  2. solange er seit admissionStallLimit() Sekunden (Vorgabe 30) keinen
//     Block gespeichert hat, obwohl er erzeugt -- dieselbe Grenze, die der
//     RPC-Weg fuer Ueberweisungen schon hat (admission_control.go), jetzt
//     fuer jeden annehmenden Pfad (annahmeBeginnen) und die Registrierung.
//
//  3. solange die aelteste offene Zeile im Ausgang aelter als
//     ausgangHoechstensAlt ist (Rueckstau-Messer, rueckstau_grenze.go) --
//     auch wenn kleine Bloecke weiterlaufen (Deckel, Bremse) und 2 deshalb
//     nicht greift.
//
// Alles ist wiederholbar ("gleich nochmal"), nicht endgueltig. Die Messung in
// 2 beginnt mit dem ersten ProduceBlock-Versuch -- auf jedem Knoten, der
// erzeugen koennte, auch einem, dessen Schluessel das Register (noch) nicht
// traegt. Ein Knoten, der dann keine Bloecke speichert (nicht im
// Erzeugerkreis, nicht im Register, Folger), nimmt nach 30 s ueber
// annahmeBeginnen nichts mehr an;
// Folger lehnt das Annahme-Tor ohnehin vorher ab, und der RPC-Weg lehnt hier
// schon immer ab (admissionRefusalReason). Wer nicht verblockt, soll nicht
// annehmen -- sein Ausgang kaeme in keinen Block.
//
// ZEILEN, DIE BEIM ABSTURZ GERADE IN EINEM BLOCK STECKTEN (Sicherheitspruefung
// #297, zweiter Durchgang, HIGH): LoadPendingTxs markiert Zeilen (included_at),
// bevor der Block gespeichert ist. Stirbt der Prozess dazwischen und startet
// binnen 10 Minuten neu, oeffnete der Aufraeumer sie erst eine Stunde spaeter
// -- angewendet, aber in keinem Block, und danach zu alt fuer jede Blockzeit.
// Wer beim Start die Erzeuger-Instanzsperre bekommt, ist der einzige Prozess
// an dieser Datenbank: er oeffnet sie sofort wieder (bis zu einem Tag alt, wie
// der Aufraeumer), und die Startsperre zaehlt sie mit. Haelt ein anderer
// Prozess die Sperre (Ueberlappung beim Neustart), zaehlt die Startsperre
// auch die markierten Zeilen ohne gespeicherten Block -- angenommen wird
// erst, wenn sie verblockt sind.

// ErrAnnahmePausiert: wiederholbar, wie ErrNichtLeiter.
var ErrAnnahmePausiert = errors.New("dieser Knoten nimmt gerade nichts an")

var annahmePausiertAbgelehnt atomic.Int64

// istWiederholbareAnnahmeAblehnung: "gleich nochmal" (-32005), nicht
// "gescheitert" -- Pause, Leiterwechsel, nur lesend.
func istWiederholbareAnnahmeAblehnung(err error) bool {
	return errors.Is(err, ErrAnnahmePausiert) || errors.Is(err, ErrNichtLeiter) || errors.Is(err, ErrNurLesend)
}

// ausgangHoechstensAlt: so alt darf die aelteste offene Zeile im Ausgang
// sein, bevor die Annahme anhaelt.
const ausgangHoechstensAlt int64 = 10 * 60

// aeltesteOffeneZeile: created_at der aeltesten offenen Zeile in pending_txs
// (Rueckstau-Messer); 0 = keine oder nicht gemessen.
var aeltesteOffeneZeile atomic.Int64

// erzeugerInstanzSchluessel: Schluessel der Advisory-Sperre ("AEQI").
const erzeugerInstanzSchluessel int64 = 0x41455149

// ohneBlockBedingung: markiert, aber in keinem gespeicherten Block -- wie im
// Aufraeumer (aufraeumen_begrenzt.go).
const ohneBlockBedingung = `included_at > 0 AND (included_block_hash IS NULL
	OR NOT EXISTS (SELECT 1 FROM chain_blocks WHERE hash = pending_txs.included_block_hash))`

// erzeugerInstanzSperren: die Advisory-Sperre auf einer eigenen Verbindung
// nehmen und halten, solange der Prozess lebt. true = dieser Prozess ist der
// einzige an dieser Datenbank, der sie haelt.
func (cs *ChainState) erzeugerInstanzSperren() bool {
	if cs.instanzSperre != nil {
		return true
	}
	ctx, abbrechen := context.WithTimeout(context.Background(), 5*time.Second)
	defer abbrechen()
	conn, err := cs.db.Conn(ctx)
	if err != nil {
		return false
	}
	var ok bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, erzeugerInstanzSchluessel).Scan(&ok); err != nil || !ok {
		conn.Close()
		return false
	}
	cs.instanzSperre = conn
	return true
}

// erzeugerInstanzFreigeben: fuer Tests, die mehrere Knoten nacheinander an
// derselben Datenbank bauen.
func (cs *ChainState) erzeugerInstanzFreigeben() {
	if cs.instanzSperre != nil {
		cs.instanzSperre.Close()
		cs.instanzSperre = nil
	}
}

// ausgangVorStartMerken: beim Start (nach dem Aufraeumer) liegengebliebene
// Zeilen wieder oeffnen, wenn dieser Prozess der einzige ist, und die
// hoechste Ausgangszeile von vor dem Start merken. 0 = nichts offen.
func (cs *ChainState) ausgangVorStartMerken() {
	if cs == nil || cs.db == nil {
		return
	}
	bedingung := `included_at = 0`
	if cs.erzeugerInstanzSperren() {
		res, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 0, included_block_hash = NULL
			WHERE `+ohneBlockBedingung+` AND included_at >= $1`, time.Now().Add(-pendingLeicheAlter).Unix())
		if err != nil {
			// Nicht geoeffnet: dann zaehlt die Startsperre sie mit.
			fmt.Printf("[ANNAHME] Liegengebliebene Ausgangszeilen nicht geoeffnet (%v) -- die Startsperre zaehlt sie mit\n", err)
			bedingung = `(included_at = 0 OR (` + ohneBlockBedingung + `))`
		} else if n, _ := res.RowsAffected(); n > 0 {
			fmt.Printf("[ANNAHME] %d Ausgangszeile(n), die beim letzten Ende in einem ungespeicherten Block steckten, wieder geoeffnet\n", n)
		}
	} else {
		fmt.Println("[ANNAHME] Erzeuger-Instanzsperre haelt ein anderer Prozess -- dessen markierte Zeilen bleiben ihm, die Startsperre zaehlt sie mit")
		bedingung = `(included_at = 0 OR (` + ohneBlockBedingung + `))`
	}
	cs.ausgangVorStartBedingung = bedingung
	var bis int64
	if err := cs.db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM pending_txs WHERE ` + bedingung).Scan(&bis); err != nil {
		// Unbekannt heisst: so tun, als laege alles offen -- die Sperre loest
		// sich mit dem ersten gespeicherten Block, der nachsieht.
		fmt.Printf("[ANNAHME] Ausgang beim Start nicht lesbar (%v) -- Annahme wartet auf den ersten eigenen Block\n", err)
		bis = 1<<62 - 1
	}
	cs.ausgangVorStartBis.Store(bis)
	if bis > 0 {
		fmt.Printf("[ANNAHME] Ausgang von vor dem Start liegt noch offen (bis Zeile %d) -- neue Auftraege erst, wenn er verblockt ist\n", bis)
	}
}

// eigenerBlockGespeichert: nach jedem gespeicherten eigenen Block.
func (cs *ChainState) eigenerBlockGespeichert() {
	if cs == nil {
		return
	}
	cs.letzterEigenerBlock.Store(time.Now().Unix())
	bis := cs.ausgangVorStartBis.Load()
	if bis == 0 {
		return
	}
	if cs.db == nil {
		cs.ausgangVorStartBis.Store(0)
		return
	}
	bedingung := cs.ausgangVorStartBedingung
	if bedingung == "" {
		bedingung = `included_at = 0`
	}
	// Im Speicherkorb-Modus wartet eine Zeile mit wal_seq > korbBis+1 auf
	// Schnellpfad-Ueberweisungen mit kleinerer Seq (blockKorbMischen). Kommen
	// keine (abgeschnittener WAL), haelt sie die Sperre nicht fest -- sonst
	// kaeme nie eine, und nichts ginge weiter.
	korbBedingung := ""
	if cs.korb != nil {
		korbBedingung = fmt.Sprintf(" AND wal_seq <= %d", cs.korbBis.Load()+1)
	}
	var offen bool
	if err := cs.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pending_txs WHERE `+bedingung+` AND id <= $1`+korbBedingung+`)`, bis).Scan(&offen); err != nil {
		return // beim naechsten Block wieder
	}
	if !offen && cs.ausgangVorStartBis.CompareAndSwap(bis, 0) {
		fmt.Println("[ANNAHME] ✓ Ausgang von vor dem Start ist verblockt -- Annahme offen")
	}
}

// annahmePausiert: nil, wenn angenommen werden darf; zaehlt Ablehnungen.
func (cs *ChainState) annahmePausiert() error {
	err := cs.annahmePauseGrund()
	if err != nil {
		annahmePausiertAbgelehnt.Add(1)
	}
	return err
}

// annahmePauseGrund: wie annahmePausiert, ohne zu zaehlen.
func (cs *ChainState) annahmePauseGrund() error {
	if cs.ausgangVorStartBis.Load() != 0 {
		return fmt.Errorf("%w: der Ausgang von vor dem Neustart wird noch verblockt -- bitte in wenigen Sekunden erneut versuchen", ErrAnnahmePausiert)
	}
	if a := aeltesteOffeneZeile.Load(); a > 0 {
		if d := time.Now().Unix() - a; d > ausgangHoechstensAlt {
			return fmt.Errorf("%w: der Ausgang haengt (aelteste offene Zeile %d s alt) -- bitte in Kuerze erneut versuchen", ErrAnnahmePausiert, d)
		}
	}
	// Der eigene Schluessel steht (noch) nicht im Register -- frisch
	// gebunden, in der Frist von zwei Stunden: bis dahin entsteht hier kein
	// Block, und was jetzt angenommen wuerde, waere zu Beginn des Fensters zu
	// alt fuer jede Blockzeit. Sofort anhalten, nicht erst nach 30 s
	// (Pruefung von #318).
	if cs.nichtImRegister.Load() {
		return fmt.Errorf("%w: der Schluessel dieses Knotens darf laut Register (noch) keine Bloecke erzeugen -- bitte einen anderen Knoten nutzen oder spaeter erneut versuchen", ErrAnnahmePausiert)
	}
	seit := cs.erzeugerSeit.Load()
	if seit == 0 {
		return nil // erzeugt nicht (oder noch nie versucht)
	}
	if l := cs.letzterEigenerBlock.Load(); l > seit {
		seit = l
	}
	if d := time.Now().Unix() - seit; d >= admissionStallLimit() {
		return fmt.Errorf("%w: seit %d s kein eigener Block -- bitte in Kuerze erneut versuchen", ErrAnnahmePausiert, d)
	}
	return nil
}

// AnnahmePauseStand: fuer /api/health/combined.
func (cs *ChainState) AnnahmePauseStand() map[string]interface{} {
	grund := ""
	if err := cs.annahmePauseGrund(); err != nil {
		grund = err.Error()
	}
	return map[string]interface{}{
		"bedeutung": "Annahme haelt an, solange der Ausgang von vor dem Start nicht verblockt ist oder seit " +
			fmt.Sprint(admissionStallLimit()) + " s kein eigener Block entstand (annahme_pause.go) -- sonst passten die Auftraege eines Blocks zu keiner gemeinsamen Blockzeit.",
		"pausiert":  grund != "",
		"grund":     grund,
		"abgelehnt": annahmePausiertAbgelehnt.Load(),
		"vor_start": cs.ausgangVorStartBis.Load() != 0,
	}
}
