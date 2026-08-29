package keeper

import "sync/atomic"

// Warum eine Ueberweisung den Schnellpfad verlaesst.
//
// # WARUM DAS DIE NAECHSTE FRAGE IST
//
// Gemessen am 29.08.2026: 92,6 % der Ueberweisungen laufen ueber den
// shard-gesperrten Schnellpfad und warten dort 156,3 ms auf cs.mu.RLock() --
// 68 % ihrer Gesamtzeit. Ausgesperrt werden sie von den 7,4 %, die ueber die
// globale Schreibsperre gehen.
//
// Der naheliegende Schluss war, den langsamen Pfad umzubauen. Der Versuch ist
// am selben Abend gescheitert: 64 Funktionen greifen direkt auf die Shards zu,
// 247 loesen es transitiv aus, und die erste umgestellte Funktion verklemmte
// sich zwei Ebenen tief.
//
// Die Frage davor wurde nie gestellt: WARUM fallen ueberhaupt 7,4 % zurueck?
// Neun Ausstiege fuehren aus dem Schnellpfad, und sie bedeuten voellig
// Verschiedenes:
//
//   - "Shard belegt" heisst, dass zwei Ueberweisungen dasselbe Konto
//     beruehren. Im Lasttest laufen 150 Paare ueber 576 Konten -- Kollisionen
//     sind dort haeufig. Im echten Betrieb mit vielen Nutzern waeren sie
//     selten. Dann waere die gemessene Aussperrung ein Artefakt des Prueflaufs
//     und der ganze Umbau unnoetig.
//   - "Demurrage steht an" heisst, dass ein Konto abgerechnet werden muss,
//     bevor es sich bewegen darf. Das haengt am Alter des Kontos, nicht an
//     der Last -- und liesse sich vorziehen.
//   - "Warteschlange voll" heisst Rueckstau und waere ein echter Engpass.
//
// Ohne diese Aufteilung ist jede weitere Arbeit am Schnellpfad geraten. Mit
// ihr ist sie entweder begruendet oder erledigt.
//
// Ein Zaehler je Grund, keine Ausgabe im heissen Pfad -- die Aufteilung steht
// in /api/health/combined.
var (
	fbKeinWAL       atomic.Int64 // kein WAL konfiguriert
	fbWarteschlange atomic.Int64 // Flush-Warteschlange am Anschlag
	fbShardBelegt   atomic.Int64 // TryLockAddrs fand einen belegten Shard
	fbKontoFehlt    atomic.Int64 // Absender oder Empfaenger existiert nicht
	fbDemurrage     atomic.Int64 // Abrechnung steht an, gehoert dem langsamen Pfad
	fbWohlstandsCap atomic.Int64 // Empfaenger wuerde die Obergrenze reissen
	fbKodierung     atomic.Int64 // JSON-Kodierung des WAL-Satzes schlug fehl
	fbAnhangFehler  atomic.Int64 // wal.Append gab einen Fehler
)

// FallbackGruende gibt die Aufteilung fuer /api/health/combined zurueck.
func FallbackGruende() map[string]interface{} {
	werte := map[string]int64{
		"kein_wal":       fbKeinWAL.Load(),
		"warteschlange":  fbWarteschlange.Load(),
		"shard_belegt":   fbShardBelegt.Load(),
		"konto_fehlt":    fbKontoFehlt.Load(),
		"demurrage":      fbDemurrage.Load(),
		"wohlstands_cap": fbWohlstandsCap.Load(),
		"kodierung":      fbKodierung.Load(),
		"anhang_fehler":  fbAnhangFehler.Load(),
	}
	var summe int64
	for _, v := range werte {
		summe += v
	}
	aus := make(map[string]interface{}, len(werte)+2)
	for k, v := range werte {
		aus[k] = v
	}
	aus["summe"] = summe
	aus["bedeutung"] = "warum Ueberweisungen den Schnellpfad verlassen. shard_belegt ueberwiegt = " +
		"Kollisionen auf denselben Konten, im Lasttest ein Artefakt des kleinen Kontenvorrats. " +
		"demurrage ueberwiegt = Abrechnungen, die sich vorziehen liessen. warteschlange " +
		"ueberwiegt = echter Rueckstau"
	return aus
}
