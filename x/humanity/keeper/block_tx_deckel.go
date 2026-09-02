package keeper

import (
	"os"
	"strconv"
	"sync/atomic"
)

// Die Blockgroesse live begrenzen -- ohne Deploy je Messpunkt.
//
// # WARUM DAS DER HEBEL IST
//
// replayTransactions haelt cs.mu.Lock() -- die GLOBALE Schreibsperre -- fuer
// einen GANZEN Block. Nicht je Ueberweisung, sondern einmal fuer alle
// Transaktionen darin. Die Haltezeit waechst also linear mit der Blockgroesse.
//
// Go's RWMutex sperrt ankommende Leser aus, sobald ein Schreiber ANSTEHT.
// Damit gilt: je groesser der Block, desto laenger steht jeder Leser. Und die
// Leser sind hier nicht Nebensache -- 92,6 % aller Ueberweisungen laufen ueber
// den shard-gesperrten Schnellpfad und brauchen cs.mu.RLock(). Gemessen am
// 29.08.2026 warteten sie dort 156,3 ms, 68 % ihrer Gesamtzeit.
//
// Das erklaert, warum der Stillstand NUR UNTER LAST auftritt. Im Leerlauf
// tragen die Bloecke ein paar Dutzend Transaktionen, der Halt ist kurz,
// niemand merkt etwas. Unter Last tragen sie Tausende, der Halt wird lang --
// und dann verhungern nacheinander: der Schnellpfad (keine Ueberweisung kommt
// durch), /api/status (antwortet nicht), und schliesslich der Partnerknoten,
// der daraus schliesst, dieser Knoten sei tot.
//
// Der Knoten stuerzt dabei nicht ab. Er ist ausgesperrt von seiner eigenen
// Sperre. Von aussen sieht beides gleich aus -- und genau deshalb wurde
// monatelang nach einem Absturz gesucht, den es nie gab.
//
// # WARUM ES SCHON EINMAL GEWIRKT HAT
//
// Am 21.08.2026 wurde maxTxsPerBlock von 50.000 auf 10.000 gesenkt. Der
// Durchsatz stieg von 2.117 auf 3.264 TPS -- um 54 %, durch KLEINERE Bloecke.
// Das ist derselbe Mechanismus: kuerzerer Halt, weniger ausgesperrte Leser.
// Die Messung ist der Beleg, dass die Richtung stimmt; sie sagt nur nicht, wo
// das Optimum liegt. 10.000 war kein gemessener Bestwert, sondern der erste
// Versuch, der besser war als der Vorgaengerwert.
//
// # WARUM ALS SCHALTER
//
// maxTxsPerBlock ist eine Konstante. Jeder Messpunkt kostet damit einen
// Deploy -- und ein Deploy startet den Knoten neu, was auf dem Primary genau
// die Unruhe erzeugt, die hier untersucht werden soll. Als Umgebungsvariable
// kostet ein Messpunkt Sekunden und keinen Neustart des Netzes.
//
//	AEQUITAS_MAX_TXS_PER_BLOCK   Obergrenze je Block (Vorgabe maxTxsPerBlock)
//
// Ein unbrauchbarer oder zu grosser Wert ergibt die Vorgabe: der Deckel darf
// nur SENKEN. Ihn nach oben aufzumachen wuerde die Sperre laenger halten --
// die falsche Richtung, und ueber eine Umgebungsvariable eine, die niemand
// beim naechsten Vorfall vermutet.
const maxTxsPerBlockEnv = "AEQUITAS_MAX_TXS_PER_BLOCK"

// blockTxHartDeckel liefert die geltende Obergrenze.
//
// Bei jedem Block gelesen, nicht zwischengespeichert: ein os.Getenv kostet
// einige hundert Nanosekunden gegen einen Halt der globalen Sperre von
// Millisekunden -- und dafuer wirkt eine Aenderung am laufenden Knoten.
func blockTxHartDeckel() int {
	raw := os.Getenv(maxTxsPerBlockEnv)
	if raw == "" {
		return maxTxsPerBlock
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > maxTxsPerBlock {
		return maxTxsPerBlock
	}
	return n
}

// Wie oft der harte Deckel tatsaechlich gegriffen hat -- ein Schalter, dessen
// Wirkung man nicht sehen kann, wird beim naechsten Vorfall falsch erklaert.
var blockTxDeckelGriff atomic.Int64

// BlockTxDeckelStand zeigt den geltenden Wert in /api/health/combined.
func BlockTxDeckelStand() map[string]interface{} {
	deckel := blockTxHartDeckel()
	return map[string]interface{}{
		"deckel":               deckel,
		"vorgabe":              maxTxsPerBlock,
		"abweicht_von_vorgabe": deckel != maxTxsPerBlock,
		"gegriffen":            blockTxDeckelGriff.Load(),
		"bedeutung": "Obergrenze der Transaktionen je Block. replayTransactions haelt die " +
			"globale Schreibsperre fuer einen ganzen Block, also waechst die Haltezeit -- und " +
			"damit die Aussperrung der 92,6 % Schnellpfad-Leser -- linear mit diesem Wert. " +
			"50.000 -> 10.000 brachte am 21.08.2026 +54 % Durchsatz",
	}
}
