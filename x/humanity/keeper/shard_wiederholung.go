package keeper

import (
	"sync/atomic"
	"time"
)

// Bei belegtem Shard kurz wiederholen, statt sofort in den Buendler zu fallen.
//
// # DIE RECHNUNG, DIE DAS AUSLOEST
//
// Gemessen am 01.09.2026 auf Contabo2, 400 disjunkte Paare:
//
//	transfer_phases.total_ms    24,2 ms   <- der Schnellpfad
//	rpc_phases.transfer_ms      90,8 ms   <- der Durchschnitt ueber ALLE
//
// Die Luecke sind die rund 9 % Rueckfaelle. Rechnet man sie heraus, kostet ein
// Rueckfall etwa 800 ms -- er wartet auf das naechste Buendel UND auf die
// globale Schreibsperre davor (warten_auf_sperre lag bei 171,7 ms). Neun
// Prozent zu 800 ms machen 76 % der Durchschnittszeit aus.
//
// # WARUM DIE SPERRE UEBERHAUPT BELEGT IST
//
// Nicht durch andere Ueberweisungen: die Rueckfallquote ist bei 50 Sendern
// (12,3 %) HOEHER als bei 400 (9,0 %), also nicht last- und damit nicht
// kollisionsgetrieben. Sie kommt von flushWALBatch, das cs.accounts.LockAddrs
// ueber seinen GANZEN Postgres-Schreibvorgang haelt: hold_avg_ms 47 bei 65
// Adressen je Flush und 32 Arbeitern. Wer in diesen 47 ms eines dieser Konten
// anfasst, faellt zurueck.
//
// Diesen Halt zu verkuerzen ist KEINE Option -- es wurde zweimal versucht und
// beide Male gab es Postgres-Deadlocks (40P01) zwischen dem UPSERT des Flushes
// und dem UPDATE von saveAccountsToDBBatchCtx. Die Begruendung steht
// ausfuehrlich ueber flushWALBatch; sie ist richtig und bleibt.
//
// # WARUM WIEDERHOLEN DER RICHTIGE HEBEL IST
//
// TryLockAddrs ist absichtlich nicht blockierend: ein Aufrufer soll sich nicht
// hinter einem langen Halt anstellen. Das stimmt fuer einen UNBEGRENZTEN Halt.
// Hier ist der Halt aber beschraenkt und bekannt (47 ms im Mittel), waehrend
// die Alternative 800 ms kostet. Sofort aufzugeben tauscht also ein paar
// Millisekunden gegen fast eine Sekunde.
//
// Wiederholt wird deshalb kurz und gedeckelt: die Vorgabe unten wartet
// hoechstens 1 ms insgesamt. Wer laenger belegt ist, faellt weiter zurueck --
// die Schutzwirkung des nicht-blockierenden Entwurfs bleibt, nur die
// Ueberreaktion auf einen kurzen Halt verschwindet.
//
// # DIE FENSTERGROESSE FOLGT AUS DER HALTEZEIT
//
// Ein erster Entwurf wartete hoechstens 1 ms. Das waere wirkungslos gewesen:
// der Blockierer haelt im Mittel 47 ms, ein 1-ms-Fenster rettet also
// hoechstens 2 % der Faelle. Faengt eine Ueberweisung den Halt an einer
// zufaelligen Stelle ab, ist die Restzeit gleichverteilt in [0, 47 ms] --
// erst ein Fenster in dieser Groessenordnung rettet die Mehrzahl.
//
// Vorgabe daher 15 x 4 ms = 60 ms. Das klingt viel und ist es nicht: die
// Alternative kostet gemessen rund 800 ms. Wer laenger belegt ist, faellt
// weiterhin zurueck.
//
// Waehrend der Pause haelt der Aufrufer NICHTS -- kein Deadlock-Risiko, nur
// eine schlafende Goroutine. Bei 3.000 TPS und 9 % Kollisionen sind das
// groessenordnungsmaessig 16 gleichzeitig schlafende Goroutinen.
//
// # GEMESSEN AM 01.09.2026 -- UND ES HAT NICHT GEHOLFEN
//
// Live auf Contabo2, 400 disjunkte Paare, Fenster 15 x 4 ms:
//
//	Rueckfallquote   8,6 %  ->  5,8 %    (ein Drittel weniger)
//	Rettungsquote            31-35 %
//	Durchsatz        3.274  ->  2.905 TPS (Mittel aus je drei Laeufen)
//
// Die Rueckfaelle sinken messbar, der Durchsatz nicht. Zwei Annahmen waren
// falsch: die Rettungsquote liegt bei einem Drittel statt bei nahezu allen,
// die Haltezeiten streuen also deutlich ueber die 47 ms Mittelwert hinaus --
// und die daraus abgeleiteten "rund 800 ms je Rueckfall" waren zu hoch
// gegriffen. Was das Warten spart, kostet es an anderer Stelle wieder.
//
// DESHALB IST DIE VORGABE 0, also AUS. Der Mechanismus bleibt samt Messung
// stehen, weil die Zahl (ein Drittel weniger Rueckfaelle) echt ist und unter
// anderen Bedingungen -- kuerzere Flush-Halte, andere Platte -- tragen
// koennte. Eingeschaltet wird er ueber die Umgebung, nicht durch Hoffnung.
//
// Das ist dieselbe Konsequenz, die dieses Projekt schon dreimal gezogen hat:
// eine plausible Optimierung, die sich nicht messen laesst, wird nicht
// Vorgabe (siehe wal_flush_addr_cap.go und batch_tuning.go).
//
//	AEQUITAS_SHARD_RETRY_VERSUCHE   Versuche (Vorgabe 0 = aus)
//	AEQUITAS_SHARD_RETRY_PAUSE_US   Pause dazwischen (Vorgabe 4000 us)
//
// Ein unbrauchbarer Wert ergibt die Vorgabe. rettungsquote in
// /api/health/combined sagt, ob ein gesetztes Fenster passt.
const (
	shardRetryVersucheVorgabe = 0
	shardRetryPauseVorgabeUs  = 4000

	shardRetryVersucheEnv = "AEQUITAS_SHARD_RETRY_VERSUCHE"
	shardRetryPauseEnv    = "AEQUITAS_SHARD_RETRY_PAUSE_US"
)

var (
	// Wie oft eine Wiederholung den Rueckfall abgewendet hat -- ohne diese
	// Zahl liesse sich nicht sagen, ob der Schalter etwas tut.
	shardRetryGerettet   atomic.Int64
	shardRetryVergeblich atomic.Int64
)

func shardRetryVersuche() int {
	if n, ok := ganzzahlAusUmgebung(shardRetryVersucheEnv); ok && n >= 0 {
		return n
	}
	return shardRetryVersucheVorgabe
}

func shardRetryPause() time.Duration {
	if n, ok := ganzzahlAusUmgebung(shardRetryPauseEnv); ok && n > 0 {
		return time.Duration(n) * time.Microsecond
	}
	return shardRetryPauseVorgabeUs * time.Microsecond
}

// sperreMitKurzerWiederholung versucht TryLockAddrs und wiederholt kurz.
//
// Gibt (unlock, true) zurueck, wenn die Shards erworben wurden. Der Aufrufer
// behandelt false genau wie bisher: Rueckfall auf den Buendler.
func (cs *ChainState) sperreMitKurzerWiederholung(addrs ...string) (func(), bool) {
	unlock, ok := cs.accounts.TryLockAddrs(addrs...)
	if ok {
		return unlock, true
	}
	versuche := shardRetryVersuche()
	if versuche <= 0 {
		shardRetryVergeblich.Add(1)
		return nil, false
	}
	pause := shardRetryPause()
	for i := 0; i < versuche; i++ {
		time.Sleep(pause)
		if unlock, ok = cs.accounts.TryLockAddrs(addrs...); ok {
			shardRetryGerettet.Add(1)
			return unlock, true
		}
	}
	shardRetryVergeblich.Add(1)
	return nil, false
}

// ShardRetryStand zeigt die Wirkung in /api/health/combined.
func ShardRetryStand() map[string]interface{} {
	g := shardRetryGerettet.Load()
	v := shardRetryVergeblich.Load()
	gesamt := g + v
	quote := 0.0
	if gesamt > 0 {
		quote = float64(g) / float64(gesamt) * 100
	}
	return map[string]interface{}{
		"gerettet":      g,
		"vergeblich":    v,
		"rettungsquote": quote,
		"versuche":      shardRetryVersuche(),
		"pause_us":      shardRetryPause().Microseconds(),
		"bedeutung": "Wie oft ein kurzes Wiederholen den Rueckfall auf den Buendler abgewendet hat. " +
			"Ein Rueckfall kostet gemessen rund 800 ms, das Wiederholen hoechstens 1 ms. " +
			"rettungsquote nahe 0 hiesse: die Shards sind laenger belegt als das Fenster",
	}
}
