package keeper

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Zaehlt, wie oft dbExecCtx noch auf cs.activeTx zurueckfaellt.
//
// # WARUM DIESE ZAHL DARUEBER ENTSCHEIDET, OB activeTx WEG KANN
//
// Am 29.08.2026 wurde die letzte cs.dbExec()-Aufrufstelle migriert: jeder
// Schreibvorgang geht jetzt durch dbExecCtx(ctx). Das allein macht cs.activeTx
// aber noch NICHT entbehrlich, und der Unterschied ist gefaehrlich.
//
// dbExecCtx nimmt die Transaktion aus dem ctx, wenn eine drin steht. Steht
// keine drin, faellt es auf cs.activeTx zurueck -- und genau dieser Rueckfall
// haelt heute noch jeden Pfad am Leben, der innerhalb einer atomaren Operation
// laeuft, den ctx aber irgendwo unterwegs nicht weiterreicht. Wuerde man
// activeTx jetzt entfernen, wuerde so ein Pfad still auf cs.db schreiben:
// ausserhalb der Transaktion, unbeeindruckt von einem Ruecklauf, und niemand
// bemerkt es, bis eine zurueckgerollte Operation trotzdem Spuren hinterlassen
// hat.
//
// Die Frage "reicht jeder Pfad den ctx durch?" laesst sich nicht durch
// Hinsehen beantworten -- dafuer sind es zu viele, und ein uebersehener faellt
// erst im Ruecklauf auf. Sie laesst sich aber zaehlen.
//
// BEDEUTUNG DER ZAHLEN
//
//	rueckfaelle == 0 unter echter Last  -> kein Pfad verlaesst sich mehr auf
//	                                      den Rueckfall; activeTx kann weg
//	rueckfaelle > 0                     -> es gibt noch Pfade, die den ctx
//	                                      nicht durchreichen. Die Zahl sagt
//	                                      nicht welche -- dafuer ist der
//	                                      Stapelauszug im DB-GUARD-Zweig da --
//	                                      aber sie sagt, dass es sie gibt.
//
// Kostet einen atomaren Zaehler auf einem Pfad, der ohnehin gleich eine
// Datenbank anspricht. Anders als das Contention-Profil braucht das keinen
// Schalter: eine Addition je Schreibvorgang ist neben einem Netzwerk-Umlauf
// nicht messbar, und eine Zahl, die man erst einschalten muss, sieht sich
// niemand an.
var activeTxRueckfaelle atomic.Int64

// rueckfallHerkunft zaehlt die Rueckfaelle je Aufrufer (Datei:Zeile).
//
// Die blosse Anzahl beweist, dass cs.activeTx noch gebraucht wird; sie sagt
// nicht, WO. Ohne diese Karte muesste man die fehlenden Pfade suchen, statt sie
// abzulesen -- und Suchen hat in diesem Projekt schon dreimal auf die falsche
// Stelle gefuehrt.
var rueckfallHerkunft sync.Map // string -> *atomic.Int64

// rueckfallHerkunftMax deckelt die Karte. Ein unerwartet breiter Aufruferkreis
// soll Speicher nicht unbegrenzt ziehen; die haeufigsten stehen ohnehin
// zuerst drin, weil sie zuerst auftreten.
const rueckfallHerkunftMax = 64

var rueckfallHerkunftAnzahl atomic.Int64

// notiereActiveTxRueckfall wird genau dort gerufen, wo dbExecCtx die
// Transaktion NICHT aus dem ctx bekommt, sondern aus cs.activeTx.
//
// runtime.Caller(2) ueberspringt diese Funktion und dbExecCtx selbst und
// benennt damit den Aufrufer, dem der ctx fehlt -- genau den Pfad, der noch zu
// migrieren ist.
func notiereActiveTxRueckfall() {
	activeTxRueckfaelle.Add(1)
	// MEHRERE Ebenen, nicht nur eine. Der erste Anlauf zeichnete nur den
	// unmittelbaren Aufrufer von dbExecCtx auf -- und der war beide Male
	// saveAccountToDBInnerCtx bzw. saveAccountsToDBBatchCtx, also die Stelle,
	// an der es AUFFAELLT, nicht die, aus der es KOMMT. Beide sind bereits
	// ctx-gefuehrt; wer ihnen einen ctx ohne Transaktion gibt, steht weiter
	// oben. Drei Ebenen benennen den Pfad.
	var pcs [4]uintptr
	n := runtime.Callers(3, pcs[:])
	if n == 0 {
		return
	}
	rahmen := runtime.CallersFrames(pcs[:n])
	var teile []string
	for i := 0; i < 3; i++ {
		f, weiter := rahmen.Next()
		datei := f.File
		if j := strings.LastIndexByte(datei, '/'); j >= 0 {
			datei = datei[j+1:]
		}
		teile = append(teile, datei+":"+strconv.Itoa(f.Line))
		if !weiter {
			break
		}
	}
	schluessel := strings.Join(teile, " < ")
	if v, da := rueckfallHerkunft.Load(schluessel); da {
		v.(*atomic.Int64).Add(1)
		return
	}
	if rueckfallHerkunftAnzahl.Load() >= rueckfallHerkunftMax {
		return
	}
	z := new(atomic.Int64)
	z.Add(1)
	if _, geladen := rueckfallHerkunft.LoadOrStore(schluessel, z); !geladen {
		rueckfallHerkunftAnzahl.Add(1)
	}
}

// ActiveTxRueckfallStand gibt den Zaehler fuer /api/health/combined zurueck.
func ActiveTxRueckfallStand() map[string]interface{} {
	n := activeTxRueckfaelle.Load()
	herkunft := map[string]int64{}
	rueckfallHerkunft.Range(func(k, v interface{}) bool {
		herkunft[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return map[string]interface{}{
		"rueckfaelle": n,
		// Welche Aufrufer den ctx noch nicht durchreichen. Ohne diese Liste
		// waere die Zahl darueber eine Feststellung ohne Handlungsanweisung.
		"herkunft": herkunft,
		"bedeutung": "wie oft ein Schreibvorgang die Transaktion aus cs.activeTx nehmen musste, " +
			"weil sein ctx keine trug. 0 unter Last heisst: kein Pfad verlaesst sich mehr darauf, " +
			"cs.activeTx kann entfernt und echte Nebenlaeufigkeit eingeschaltet werden",
		"activetx_entfernbar": n == 0,
	}
}

// activeTxStrikt sagt, ob dbExecCtx sich schon so verhalten soll, wie es sich
// nach dem Entfernen von cs.activeTx verhalten wird: kein Rueckfall.
//
// Ein Schalter und keine sofortige Entfernung, weil das Entfernen der einzige
// Schritt dieser Migration ist, der sich nicht zurueckdrehen laesst. Ein
// uebersehener Pfad schriebe danach ausserhalb seiner Transaktion, ueberlebte
// jeden Ruecklauf, und nichts sagte es. Mit dem Schalter geschieht dasselbe,
// aber laut -- und er laesst sich in Sekunden wieder ausschalten.
//
// Einmal gelesen und gemerkt: der Wert aendert sich zur Laufzeit nicht, und
// diese Frage steht auf dem Zahlungspfad.
var activeTxStriktEinmal sync.Once
var activeTxStriktWert bool

func activeTxStrikt() bool {
	activeTxStriktEinmal.Do(func() {
		activeTxStriktWert = strings.TrimSpace(os.Getenv("AEQUITAS_ACTIVETX_STRICT")) == "1"
		if activeTxStriktWert {
			fmt.Println("[ACTIVETX] strikt: dbExecCtx faellt NICHT mehr auf cs.activeTx zurueck. " +
				"Ein Pfad ohne ctx-Transaktion schreibt ab jetzt eigenstaendig -- und wird gemeldet. " +
				"AEQUITAS_ACTIVETX_STRICT entfernen, um das rueckgaengig zu machen.")
		}
	})
	return activeTxStriktWert
}

// meldeStriktenRueckfall gibt den Stapelauszug aus, hoechstens einmal alle
// fuenf Sekunden. Ohne Deckel wuerde ein haeufiger Pfad das Log fluten und
// waere dadurch schwerer zu finden, nicht leichter.
var striktLetzteMeldung atomic.Int64

func meldeStriktenRueckfall() {
	jetzt := time.Now().UnixNano()
	letzte := striktLetzteMeldung.Load()
	if jetzt-letzte < int64(5*time.Second) || !striktLetzteMeldung.CompareAndSwap(letzte, jetzt) {
		return
	}
	fmt.Printf("[ACTIVETX] ✗ STRIKT: ein Schreibvorgang hatte keine Transaktion im ctx und "+
		"schreibt jetzt eigenstaendig -- er ueberlebt einen Ruecklauf. Pfad:\n%s\n", debug.Stack())
}
