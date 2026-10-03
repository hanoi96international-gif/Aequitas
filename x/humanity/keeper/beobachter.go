package keeper

import (
	"os"
	"strings"
	"sync"
)

// Beobachter: ein Knoten, der nur nachspielt und nie einen Block erzeugt.
//
// WARUM. Die Nachrechen-Regeln (nachrechnen*.go, erhaltung.go) laufen nur
// beim NACHSPIELEN fremder Bloecke. Der Erzeuger spielt seine eigenen Bloecke
// nicht nach; solange C1 der einzige Knoten ist, zaehlt also niemand, ob ein
// ehrlicher Block eine Regel verletzen wuerde -- und ohne diese Zahl darf der
// strenge Modus (nachrechnenStrengAbUnix) nicht scharf werden.
//
// Ein frischer Knoten mit eigenem Schluessel erzeugt aber Bloecke, sobald er
// aufgeholt hat. Die wendet er bei sich an, C1 nimmt sie nicht (geschlossene
// Produzentenliste), und ab da spielt er C1s Bloecke auf einem anderen Stand
// nach -- jede Zahl waere wertlos. AEQUITAS_BEOBACHTER=1 schaltet das
// Erzeugen ab, sonst nichts: Nachspielen, Pruefungen und Zaehler bleiben, wie
// sie sind. Genutzt vom Workflow nachrechnen-beobachter.yml.
var (
	beobachterEinmal sync.Once
	beobachterAn     bool
)

func beobachterModus() bool {
	beobachterEinmal.Do(func() {
		v := strings.TrimSpace(strings.ToLower(os.Getenv("AEQUITAS_BEOBACHTER")))
		beobachterAn = v == "1" || v == "true" || v == "ja"
	})
	return beobachterAn
}

// BeobachterModus fuer Startmeldung und /api/status.
func BeobachterModus() bool { return beobachterModus() }
