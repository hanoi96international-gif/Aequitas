package keeper

// Die Zahl der Plaetze im Rueckfallpfad zum Vermessen freigeben -- ohne sie
// zu aendern.
//
// # WARUM DIESER WERT UEBERPRUEFT GEHOERT
//
// parallelBatchPoolSize steht auf 4, und seine Begruendung (siehe dort) nennt
// zwei Gruende. Der erste ist nachweislich hinfaellig: gleichzeitige Chargen
// erzeugten "sorry, too many clients already" (53300), WEIL MaxOpenConns
// damals auf 20 stand. Der Verbindungspool steht inzwischen auf 100, live
// bestaetigt (db_pool.max_open). Der zweite Grund -- "4 ist die Kernzahl
// dieser Sandbox, mehr echte Parallelitaet als Kerne hilft nicht" -- galt
// fuer eine Sandbox mit vier Kernen; die Produktionsboxen haben sechs.
//
// # WARUM DAS TROTZDEM NICHT EINFACH ERHOEHT WIRD
//
// In diesem Projekt sind bereits zehn plausible Optimierungen an Messungen
// gescheitert, zuletzt das kurze Wiederholen bei belegtem Shard: es senkte
// die Rueckfallquote wie vorhergesagt von 8,6 auf 5,8 % und den Durchsatz
// von 3.274 auf 2.905 TPS. Eine Zahl zu erhoehen, weil ihre Begruendung
// veraltet ist, ist kein Beleg, dass die neue besser waere.
//
// Deshalb ein Schalter statt einer Aenderung: die Vorgabe bleibt exakt, was
// sie war, und mehrere Werte lassen sich in EINEM Deployment messen. Wirkt
// zusammen mit batcher_phasen_stats.go, das getrennt ausweist, ob die Zeit
// vor dem Platz (warten_ms) oder in der Charge (arbeit_ms) vergeht:
//
//	warten dominiert und der Durchsatz steigt   -> der Deckel band wirklich
//	warten sinkt, der Durchsatz nicht           -> wie beim Shard-Warten, verwerfen
//
// Der Wert wird beim Start gelesen (das Semaphor wird einmal angelegt), ein
// Neustart genuegt also -- kein neues Abbild.
const batcherPlaetzeEnv = "AEQUITAS_BATCHER_PLAETZE"

// batcherPlaetze liefert die Zahl der gleichzeitig verarbeiteten Chargen.
// Vorgabe unveraendert parallelBatchPoolSize.
func batcherPlaetze() int {
	return intFromEnv(batcherPlaetzeEnv, parallelBatchPoolSize)
}
