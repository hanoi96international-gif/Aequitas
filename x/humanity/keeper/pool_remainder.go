package keeper

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Der Rest einer Ausschuettung gehoert in den naechsten Topf, nicht ins Nichts.
//
// # DER BEFUND
//
// Alle drei Ausschuettungen -- UBI, Validatorenanteil, LP-Anteil -- rechnen den
// Anteil mit floor6 und leeren danach den GANZEN Topf:
//
//	share := floor6(total / n)
//	...
//	poolAcc.Balance = NewDecimal(0)
//
// Was zwischen "n mal abgerundeter Anteil" und "voller Topf" liegt, verschwand
// damit jede Runde. Der Wachhund der Versorgung meldet das als -0,000014 AEQ,
// und ein dauerhaft roter Wachhund ist keiner mehr: er wuerde eine echte
// Geldschoepfung genauso melden, und niemand saehe hin.
//
// floor6 war und bleibt richtig. round6 hat vorher GESCHOEPFT -- die Summe der
// gerundeten Anteile ueberstieg den Topf, der gleich danach genullt wurde. Der
// Fehler lag nie beim Abrunden, sondern beim Nullen.
//
// # WARUM AN EINEM ZEITSTEMPEL, NICHT AN EINER HOEHE
//
// Der Topfstand steht im StateRoot. Wuerde ein Knoten den Rest stehen lassen,
// waehrend ein anderer noch nullt, liefen die StateRoots auseinander -- und das
// hat dieses Projekt schon mehrfach Tage gekostet. Der Zeitpunkt des Deploys
// darf ueber Konsens nicht entscheiden.
//
// Eine Blockhoehe waere der uebliche Anker, ist an diesen Stellen aber nicht
// verfuegbar. Verfuegbar ist etwas Gleichwertiges: RunDailyDistributionAtomic
// bekommt ubiAt, EINEN ausdruecklichen Zeitstempel, den der produzierende
// Knoten in die Transaktion schreibt und jeder andere beim Nachspielen
// unveraendert uebernimmt (siehe ApplyUBIFinalizeDelta). Er ist damit genauso
// deterministisch wie eine Hoehe -- und, anders als time.Now(), fuer alle
// derselbe.
//
// # AUSROLLEN
//
// Voreinstellung AUS. Erst auf ALLEN Knoten ausrollen, dann ueberall dieselbe
// Sekunde setzen, die sicher in der Zukunft liegt. Ohne die Variable verhaelt
// sich der Knoten exakt wie bisher -- die umkehrbare Richtung.
var (
	restUebertragAb       int64
	restUebertragEinmal   sync.Once
	restUebertragGemeldet sync.Once
)

// restUebertragUmgebung ist der Name der Umgebungsvariablen (Unix-Sekunden).
const restUebertragUmgebung = "POOL_REMAINDER_CARRY_FROM_UNIX"

func restUebertragSchwelle() int64 {
	restUebertragEinmal.Do(func() {
		roh := strings.TrimSpace(os.Getenv(restUebertragUmgebung))
		if roh == "" {
			return
		}
		t, err := strconv.ParseInt(roh, 10, 64)
		if err != nil || t <= 0 {
			fmt.Printf("[POOL] %s=%q ist kein Zeitstempel -- Restuebertrag bleibt aus\n",
				restUebertragUmgebung, roh)
			return
		}
		restUebertragAb = t
		fmt.Printf("[POOL] Restuebertrag ab Zeitstempel %d\n", t)
	})
	return restUebertragAb
}

// RestUebertragAktiv sagt, ob bei DIESER Ausschuettung der Rest im Topf bleibt.
//
// verteiltAm ist der geteilte Zeitstempel der Runde, nicht die Uhr dieses
// Rechners. Ist er 0 -- etwa auf einem der toten Aufrufwege --, bleibt es beim
// alten Verhalten.
func RestUebertragAktiv(verteiltAm int64) bool {
	schwelle := restUebertragSchwelle()
	if schwelle <= 0 || verteiltAm <= 0 {
		return false
	}
	if verteiltAm < schwelle {
		restUebertragGemeldet.Do(func() {
			fmt.Printf("[POOL] Restuebertrag ist gesetzt (ab %d), diese Runde liegt davor. "+
				"Bis dahin verfaellt der Rest wie bisher.\n", schwelle)
		})
		return false
	}
	return true
}

// neuerTopfstand liefert, was nach einer Ausschuettung im Topf bleiben soll.
//
// Vor der Schwelle: null, wie bisher. Danach: was nicht ausgezahlt wurde.
//
// Der Rest wird NICHT gefloort, und das ist der Punkt.
//
// Der erste Entwurf tat es -- aus Sorge, die Differenz zweier Gleitkommazahlen
// koenne eine Ziffer jenseits der sechsten Stelle tragen und die Knoten
// auseinanderlaufen lassen. Ein Test hat das sofort widerlegt und den Fehler
// gezeigt: 0,000007 minus 0,000006 ergibt in Gleitkomma ein Haar WENIGER als
// 0,000001, und floor6 macht daraus null. Der Rest verschwand also weiterhin,
// obwohl der Uebertrag eingeschaltet war -- der Schutz hat genau den Schaden
// angerichtet, den er verhindern sollte.
//
// Die Sorge war auch unbegruendet. Jeder Knoten rechnet dieselbe Differenz aus
// demselben replizierten Topfstand und demselben deterministisch gefloorten
// Anteil; IEEE-754 liefert dafuer ueberall dasselbe Bitmuster. Deterministisch
// war es die ganze Zeit, nur eben falsch gerundet.
//
// Negativ kann der Rest nicht werden: die Anteile sind abgerundet, ihre Summe
// ist hoechstens der Topf. Die Klammer steht trotzdem da, weil eine
// Vorzeichenumkehr hier Geld schoepfen wuerde, und das darf nie von einer
// Annahme abhaengen.
func neuerTopfstand(gesamt, ausgezahlt float64, verteiltAm int64) float64 {
	if !RestUebertragAktiv(verteiltAm) {
		return 0
	}
	rest := gesamt - ausgezahlt
	if rest <= 0 {
		return 0
	}
	return rest
}
