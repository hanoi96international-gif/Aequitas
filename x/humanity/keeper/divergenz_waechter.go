package keeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// Der Divergenz-Waechter: vergleicht in der Ruhe die Konten-Summe mit den
// Seeds. Meldet, heilt nicht.
//
// WARUM DER STATEROOT-VERGLEICH IM BLOCK NICHT REICHT. Der StateRoot eines
// Blocks ist der Nachzustand des Produzenten INKLUSIVE seines Mempools --
// zwei Validatoren, die an derselben Hoehe produzieren, ergeben immer
// verschiedene Roots (stateroot_abweichungen.geschwisterbedingt). Darauf
// einen Resync zu bauen, gab am 12.09.2026 dreimal Fehlalarm unter Last
// (TRUNCATE chain_accounts mitten im Lauf), und der Resync wurde
// abgeschaltet. Seitdem merkte niemand mehr, dass C1 und C2 wirklich
// auseinanderliefen: 14 von 79 Lasttest-Konten eines Buckets verschieden,
// jeder Lauf machte es schlimmer (pending_reihenfolge.go).
//
// WAS STATTDESSEN TRAEGT: in der Ruhe -- keine eigene Ueberweisung seit
// 30 s, Hoehe gleichauf mit dem Seed -- muss account_set_xor auf beiden
// Seiten gleich sein. Ist er es dreimal hintereinander nicht, ist das keine
// Unschaerfe mehr, sondern ein Unterschied im Kontostand. Der Waechter
// schreibt es ins Log und in /api/health/combined (divergenz); den Resync
// entscheidet ein Mensch (resync-contabo1-only.yml / -contabo2-only.yml),
// bis der Weg dorthin wieder ohne Fehlalarm ist.

const (
	divergenzTakt     = 60 * time.Second
	divergenzRuhe     = 30 * time.Second
	divergenzSchwelle = 3
)

var (
	divergenzStrikes    atomic.Int64
	divergenzSeitUnix   atomic.Int64
	divergenzPeer       atomic.Value // string
	divergenzVergleiche atomic.Int64
	divergenzGleich     atomic.Int64
	divergenzLetzteMeld atomic.Int64
)

func (dag *BlockDAG) StarteDivergenzWaechter() {
	if dag.state == nil {
		return
	}
	SafeGoroutine("divergenzWaechter", func() {
		t := time.NewTicker(divergenzTakt)
		defer t.Stop()
		for range t.C {
			SafeCall("divergenzWaechter-tick", func() { dag.divergenzEinmalPruefen() })
		}
	})
}

func (dag *BlockDAG) divergenzEinmalPruefen() {
	// Nur in der Ruhe: eigene Annahme seit divergenzRuhe still.
	if time.Since(time.Unix(0, letzteEigeneUeberweisungNs.Load())) < divergenzRuhe {
		return
	}
	dag.syncPeerMu.Lock()
	seeds := make([]string, 0, len(dag.trustedSeeds))
	for s := range dag.trustedSeeds {
		seeds = append(seeds, s)
	}
	dag.syncPeerMu.Unlock()
	if len(seeds) == 0 {
		return
	}
	eigene := dag.state.StateRootComponentBreakdown()
	eigeneHoehe := dag.heightSchnell.Load()
	hc := &http.Client{Timeout: 10 * time.Second}
	for _, seed := range seeds {
		peerHoehe, ok := echteHoeheVonPeer(seed)
		if !ok || peerHoehe < eigeneHoehe-2 || peerHoehe > eigeneHoehe+2 {
			continue // nicht gleichauf -- kein Vergleich
		}
		resp, err := hc.Get(seed + "/api/debug/stateroot-components")
		if err != nil {
			continue
		}
		var fremd struct {
			AccountSetXOR string `json:"account_set_xor"`
			LastUBIAt     string `json:"last_ubi_at"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&fremd)
		resp.Body.Close()
		if decErr != nil || fremd.AccountSetXOR == "" {
			continue
		}
		divergenzVergleiche.Add(1)
		if fremd.AccountSetXOR == eigene.AccountSetXOR {
			divergenzGleich.Add(1)
			if divergenzStrikes.Swap(0) >= divergenzSchwelle {
				fmt.Printf("[DIVERGENZ] ✓ Kontenstand wieder gleich mit %s\n", seed)
			}
			divergenzSeitUnix.Store(0)
			return
		}
		n := divergenzStrikes.Add(1)
		if n == 1 {
			divergenzSeitUnix.Store(time.Now().Unix())
		}
		divergenzPeer.Store(seed)
		if n >= divergenzSchwelle && time.Now().Unix()-divergenzLetzteMeld.Load() >= 600 {
			divergenzLetzteMeld.Store(time.Now().Unix())
			fmt.Printf("[DIVERGENZ] ✗ Kontenstand weicht von %s ab (account_set_xor %s… gegen %s…), %d Vergleiche in Folge, beide in Ruhe und gleichauf (Hoehe %d/%d). Das ist kein Geschwister-Effekt. Abhilfe: Resync eines Knotens vom anderen (resync-contabo1-only.yml / -contabo2-only.yml) -- in der Ruhe, nie unter Last.\n",
				seed, kurzHex(eigene.AccountSetXOR), kurzHex(fremd.AccountSetXOR), n, eigeneHoehe, peerHoehe)
		}
		return
	}
}

func kurzHex(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// DivergenzStand fuer /api/health/combined.
func DivergenzStand() map[string]interface{} {
	peer, _ := divergenzPeer.Load().(string)
	seit := ""
	if t := divergenzSeitUnix.Load(); t > 0 {
		seit = time.Unix(t, 0).UTC().Format(time.RFC3339)
	}
	strikes := divergenzStrikes.Load()
	return map[string]interface{}{
		"bedeutung": "Vergleich von account_set_xor mit den Seeds in der Ruhe (keine eigene Ueberweisung seit 30 s, Hoehe gleichauf). " +
			"abweichend=true ab 3 Vergleichen in Folge mit Unterschied -- dann stimmen Kontostaende nicht ueberein, nicht nur Geschwister-Unschaerfe. " +
			"Heilt nicht selbst; Resync eines Knotens vom anderen in der Ruhe.",
		"abweichend":        strikes >= divergenzSchwelle,
		"strikes":           strikes,
		"seit":              seit,
		"peer":              peer,
		"vergleiche":        divergenzVergleiche.Load(),
		"vergleiche_gleich": divergenzGleich.Load(),
	}
}
