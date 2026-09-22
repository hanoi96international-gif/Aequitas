//go:build !race

package keeper

// unterRace sagt, ob dieses Binary mit -race gebaut wurde.
//
// WOFUER. Ein paar Tests hier messen NEBENLAEUFIGKEIT: teilen sich
// gleichzeitige Ueberweisungen einen Gruppen-Commit, oder wartet jede
// einzeln auf ihren fsync? Die Antwort haengt davon ab, wie viele davon sich
// zeitlich ueberlappen -- und genau das zerstoert der Race-Detektor. Er
// verlangsamt jeden Speicherzugriff um ein Vielfaches und verschiebt die
// Ablaufplanung der Goroutinen, sodass weniger gleichzeitig im Fenster
// liegen. Die Messung misst dann den Detektor, nicht die Sache.
//
// Belegt in CI: derselbe Test, der im gewoehnlichen Lauf durchgeht, meldete
// im -race-Lauf 0,81 Syncs je Ueberweisung gegen die geforderten 0,75 --
// und hielt damit JEDEN CI-Lauf dieses Zweiges rot (Laeufe 834 bis 841).
//
// Die scharfe Zusicherung bleibt, wo sie etwas aussagt: im gewoehnlichen
// Testlauf, der bei jedem Push mitlaeuft. Unter -race wird dieselbe Zahl
// ERMITTELT und PROTOKOLLIERT, nur nicht mehr als Fehler gewertet -- dort
// zaehlt, was -race wirklich prueft: dass dabei kein Datenrennen auftritt.
const unterRace = false
