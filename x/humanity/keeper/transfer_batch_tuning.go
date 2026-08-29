package keeper

import (
	"os"
	"strconv"
	"time"
)

// Die zwei Groessen des Buendlers zum Vermessen freigeben -- ohne sie zu
// aendern.
//
// # WARUM GERADE DIESE ZWEI
//
// Am 29.08.2026 wurde erstmals gemessen, wofuer die globale Sperre in
// runAtomicWithOutbox gehalten wird: 468,8 ms Warten gegen 50,3 ms Halten,
// bei nur 44 Vorgaengen fuer 50.000 Ueberweisungen. Der Halt selbst ist
// unauffaellig (~45 us je Ueberweisung); das Warten ist es.
//
// Am selben Abend hat der Sweep des WAL-Adressdeckels die naheliegende
// Gegenmassnahme widerlegt: den Halt zu VERKLEINERN machte alles schlechter
// (868 -> 319 TPS), weil aus einem langen Halt viele kurze wurden. Die Lehre,
// die evm_storage.go schon traegt, gilt also auch hier: "The cost was never
// the holding; it was being a writer at all." Go's RWMutex sperrt ankommende
// Leser aus, sobald ein Schreiber ANSTEHT.
//
// Daraus folgt die entgegengesetzte Richtung: nicht kleiner, sondern
// SELTENER. Weniger, dafuer groessere Buendel bedeuten weniger
// Sperrvorgaenge. Genau das steuern transferBatchMaxSize (heute 1000) und
// transferBatchMaxWait (heute 1 ms) -- und die gemessenen ~1.100
// Ueberweisungen je Vorgang zeigen, dass die Buendel bereits gegen den
// Groessendeckel laufen.
//
// # WARUM ALS SCHALTER UND NICHT ALS NEUER WERT
//
// Der bestehende Kommentar zu transferBatchMaxSize begruendet die 1000 mit
// einer Abwaegung, die nicht erfunden ist: ein groesserer Deckel kann unter
// einem echten Ausbruch einen mehrhundert-Millisekunden-Halt erzeugen, und
// eine einzelne Anfrage, die waehrenddessen ankommt, wartet ihn ab. Ob der
// Tausch sich lohnt, ist eine Messfrage, keine Meinungsfrage -- und drei
// plausible Optimierungen dieses Projekts haben sich schon als wirkungslos
// oder schaedlich erwiesen.
//
// Deshalb: Vorgabe unveraendert, Aenderung je Box ueber Umgebungsvariablen,
// Rueckweg in Sekunden. Dieselbe Bauart wie AEQUITAS_WAL_FLUSH_MAX_ADDRS --
// die hat gestern die falsche Richtung ausgeschlossen, ohne dass jemand Code
// aendern musste.
//
//	AEQUITAS_TRANSFER_BATCH_MAX_SIZE   Stueck je Buendel (Vorgabe 1000)
//	AEQUITAS_TRANSFER_BATCH_MAX_WAIT_US  Sammelfenster in Mikrosekunden
//	                                     (Vorgabe 1000, also 1 ms)
//
// Ein unbrauchbarer Wert ergibt die Vorgabe -- aus demselben Grund, aus dem
// walFlushMaxAddrs sich bei einem Tippfehler nicht selbst abschaltet.
const (
	transferBatchMaxSizeEnv = "AEQUITAS_TRANSFER_BATCH_MAX_SIZE"
	transferBatchMaxWaitEnv = "AEQUITAS_TRANSFER_BATCH_MAX_WAIT_US"
)

// transferBatchSize liefert den Groessendeckel fuer ein Buendel.
//
// Bei jedem Buendel gelesen, nicht zwischengespeichert: das sind einige
// hundert Nanosekunden gegen eine Sammelphase von einer Millisekunde und
// einen anschliessenden Postgres-Umlauf -- und dafuer laesst sich der Wert an
// einem laufenden Knoten aendern, was den Unterschied zwischen einem Sweep
// und einem Deploy-Zyklus je Messpunkt ausmacht.
func transferBatchSize() int {
	if n, ok := ganzzahlAusUmgebung(transferBatchMaxSizeEnv); ok && n > 0 {
		return n
	}
	return transferBatchMaxSize
}

// transferBatchWait liefert das Sammelfenster.
func transferBatchWait() time.Duration {
	if n, ok := ganzzahlAusUmgebung(transferBatchMaxWaitEnv); ok && n > 0 {
		return time.Duration(n) * time.Microsecond
	}
	return transferBatchMaxWait
}

func ganzzahlAusUmgebung(name string) (int, bool) {
	raw := os.Getenv(name)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

// TransferBatchAbstimmung zeigt die geltenden Werte in
// /api/health/combined -- ein Schalter, den man nicht sehen kann, wird beim
// naechsten Deploy vergessen und beim uebernaechsten falsch erklaert.
func TransferBatchAbstimmung() map[string]interface{} {
	groesse := transferBatchSize()
	warten := transferBatchWait()
	return map[string]interface{}{
		"max_size":             groesse,
		"max_wait_us":          warten.Microseconds(),
		"abweicht_von_vorgabe": groesse != transferBatchMaxSize || warten != transferBatchMaxWait,
		"bedeutung": "Groesse und Sammelfenster des Transferbuendlers. Groessere Buendel " +
			"bedeuten WENIGER Sperrvorgaenge -- die Richtung, die der WAL-Sweep vom 29.08. " +
			"nahelegt (kleinere Halte waren dort messbar schlechter)",
	}
}
