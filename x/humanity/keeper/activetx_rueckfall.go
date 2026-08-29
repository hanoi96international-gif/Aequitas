package keeper

import (
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	_, datei, zeile, ok := runtime.Caller(2)
	if !ok {
		return
	}
	if i := strings.LastIndexByte(datei, '/'); i >= 0 {
		datei = datei[i+1:]
	}
	schluessel := datei + ":" + strconv.Itoa(zeile)
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
