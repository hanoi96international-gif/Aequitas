package keeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
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
//
// RUHE HEISST: BEIDE SEITEN. Am 14.09.2026 meldeten beide Boxen
// "abweichend", waehrend nur EINE unter Last stand: C2 baute noch den
// Lastgenerator, war also selbst still -- und verglich sich mit einem C1,
// das 6.900 Ueberweisungen je Sekunde annahm. Der Zustand eines Knotens
// traegt angenommene Ueberweisungen, bevor sie in einem Block sind; der
// Vergleich mit einem beschaeftigten Partner sagt deshalb nichts. Drei
// Strikes spaeter war die Wache rot, und ein Laien-Validator mit
// AEQUITAS_DIVERGENZ_AUTORESYNC=1 haette mitten in der Last einen Resync
// begonnen. Deshalb liefert /api/debug/stateroot-components jetzt
// ruhe_seit_s mit, und der Waechter zaehlt nur, wenn auch der Seed seit
// divergenzRuhe still ist. Fehlt die Auskunft (aelterer Seed), wird nicht
// verglichen: lieber kein Urteil als ein falscher Resync.
//
// RUHE HEISST AUCH: AUSGANGSKORB LEER. 15.09.2026, 09:12 bis 09:40: sieben
// Strikes auf beiden Boxen, beide seit Minuten ohne Annahme, gleichauf --
// und account_set_xor verschieden. Kein Resync, kein Neustart, und um 13:40
// waren beide wieder gleich. Was dazwischen lag: C1 hatte im Lastlauf 1,7
// Millionen Ueberweisungen angenommen (und sofort in seinen Zustand
// uebernommen), aber nur 600.000 davon verblockt; der Rest ging ueber die
// naechsten zwanzig Minuten Block fuer Block hinaus. Solange lief C1s
// Zustand der Kette voraus, und C2 konnte ihn nur nachvollziehen, wie die
// Bloecke kamen. Ein Vergleich sagt also erst dann etwas, wenn auf BEIDEN
// Seiten nichts mehr angenommen und noch nicht verblockt ist: offen == 0.

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
	// einmalige Meldung je Prozess, wenn ein Seed ruhe_seit_s nicht kennt
	divergenzOhneAuskunftGemeldet atomic.Bool
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

// DivergenzAuskunft ist die Antwort von /api/debug/stateroot-components: die
// Zustandsbestandteile plus die eigene Ruhe, damit der Partner weiss, ob ein
// Vergleich gerade etwas sagt.
type DivergenzAuskunft struct {
	StateRootComponents
	RuheSeitS float64 `json:"ruhe_seit_s"`
	// Offen: angenommen, aber noch in keinem Block (Ausgangskorb + Speicher).
	// -1, wenn nicht bestimmbar.
	Offen int64 `json:"offen"`
}

// ruheSeit: wie lange dieser Knoten keine eigene Ueberweisung mehr
// angenommen hat.
func ruheSeit() time.Duration {
	return time.Since(time.Unix(0, letzteEigeneUeberweisungNs.Load()))
}

// divergenzVergleichbar entscheidet, ob ein Vergleich zaehlt: beide Seiten
// seit divergenzRuhe still UND beide ohne offene (angenommene, noch nicht
// verblockte) Ueberweisungen. peerRuheS/peerOffen sind nil, wenn der Seed
// die Auskunft nicht liefert -- dann zaehlt nichts.
func divergenzVergleichbar(eigeneRuhe time.Duration, eigeneOffen int64, peerRuheS *float64, peerOffen *int64) bool {
	if eigeneRuhe < divergenzRuhe || eigeneOffen != 0 {
		return false
	}
	if peerRuheS == nil || *peerRuheS < divergenzRuhe.Seconds() {
		return false
	}
	return peerOffen != nil && *peerOffen == 0
}

// offeneUeberweisungen: was dieser Knoten angenommen, aber noch nicht
// verblockt hat -- die offenen Zeilen des Ausgangskorbs (Teilindex, ein
// kurzer Lauf) plus die Warteschlange im Speicher. -1, wenn die Datenbank
// nicht antwortet: dann zaehlt kein Vergleich.
func (dag *BlockDAG) offeneUeberweisungen() int64 {
	var n int64 = -1
	if dag.state != nil && dag.state.db != nil {
		if err := dag.state.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE included_at = 0`).Scan(&n); err != nil {
			return -1
		}
	} else {
		n = 0
	}
	dag.txMu.Lock()
	n += int64(len(dag.pendingTxs))
	dag.txMu.Unlock()
	return n
}

// DivergenzAuskunftFuer baut die Antwort von /api/debug/stateroot-components.
func (dag *BlockDAG) DivergenzAuskunftFuer() DivergenzAuskunft {
	return DivergenzAuskunft{
		StateRootComponents: dag.state.StateRootComponentBreakdown(),
		RuheSeitS:           ruheSeit().Seconds(),
		Offen:               dag.offeneUeberweisungen(),
	}
}

func (dag *BlockDAG) divergenzEinmalPruefen() {
	// Nur in der Ruhe: eigene Annahme seit divergenzRuhe still.
	eigeneRuhe := ruheSeit()
	if eigeneRuhe < divergenzRuhe {
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
	eigeneOffen := dag.offeneUeberweisungen()
	if eigeneOffen != 0 {
		return // Ausgangskorb nicht leer: der eigene Zustand laeuft der Kette voraus
	}
	eigene := dag.state.StateRootComponentBreakdown()
	eigeneHoehe := dag.heightSchnell.Load()
	hc := &http.Client{Timeout: 10 * time.Second}
	for _, seed := range seeds {
		// Gleichauf heisst: zum Zeitpunkt der Abfrage -- gegen die eigene
		// Hoehe von damals, nicht von jetzt (siehe peer_hoehe_echt.go).
		peerHoehe, eigeneDamals, ok := echteHoeheVonPeerMitEigener(seed)
		if !ok || peerHoehe < eigeneDamals-2 || peerHoehe > eigeneDamals+2 {
			continue // nicht gleichauf -- kein Vergleich
		}
		resp, err := hc.Get(seed + "/api/debug/stateroot-components")
		if err != nil {
			continue
		}
		var fremd struct {
			AccountSetXOR string   `json:"account_set_xor"`
			LastUBIAt     string   `json:"last_ubi_at"`
			RuheSeitS     *float64 `json:"ruhe_seit_s"`
			Offen         *int64   `json:"offen"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&fremd)
		resp.Body.Close()
		if decErr != nil || fremd.AccountSetXOR == "" {
			continue
		}
		if !divergenzVergleichbar(eigeneRuhe, eigeneOffen, fremd.RuheSeitS, fremd.Offen) {
			if fremd.RuheSeitS == nil && divergenzOhneAuskunftGemeldet.CompareAndSwap(false, true) {
				fmt.Printf("[DIVERGENZ] %s liefert keine Ruhe-Auskunft (ruhe_seit_s) -- kein Vergleich, bis der Seed aktualisiert ist\n", seed)
			}
			continue // Partner nicht in Ruhe -- der Vergleich sagt nichts
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
		// Selbstheilung -- nur wo sie erlaubt ist (siehe divergenzAutoResyncErlaubt).
		if divergenzAutoResyncErlaubt(n, os.Getenv("AEQUITAS_DIVERGENZ_AUTORESYNC"), dag.resyncBootstrapURL != "" && dag.resyncSigner != "") {
			dag.triggerAutoResync(fmt.Sprintf("Kontenstand weicht von %s ab: %d Vergleiche in Folge in der Ruhe und gleichauf (account_set_xor %s… gegen %s…) -- dieser Knoten holt den Zustand neu vom Seed", seed, n, kurzHex(eigene.AccountSetXOR), kurzHex(fremd.AccountSetXOR)))
		}
		return
	}
}

// divergenzAutoResyncErlaubt: darf eine belegte Kontostand-Abweichung einen
// Resync vom Seed ausloesen?
//
// NUR MIT AEQUITAS_DIVERGENZ_AUTORESYNC=1. Auf den Gruenderboxen bleibt es aus:
// wenn C1 und C2 voneinander abweichen, sagt der Vergleich nicht, WER recht
// hat -- das entscheidet ein Mensch (12.09.2026: per SQL-Vergleich der
// Konten, dann Resync des falschen). Ein Validator, der den Zustand ohnehin
// von den Seeds bezieht (deploy/validator), ist nie die Quelle der Wahrheit;
// fuer ihn ist "weicht ab" gleichbedeutend mit "ist falsch", und der Resync
// vom signierten Snapshot ist genau das, was ein Betreiber von Hand taete --
// nur ohne dass er es merken muss. Das ist die Selbstheilung, die ein
// Laien-Validator vor der Beta braucht.
//
// Ausserdem noetig: eine konfigurierte, signierte Quelle (BOOTSTRAP + Signer,
// gesetzt von StartDivergenceAutoHeal). Die 30-Minuten-Sperre und die
// Rueckfallpfade liegen in triggerAutoResync.
func divergenzAutoResyncErlaubt(strikes int64, env string, quelleKonfiguriert bool) bool {
	return strikes >= divergenzSchwelle && strings.TrimSpace(env) == "1" && quelleKonfiguriert
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
		"bedeutung": "Vergleich von account_set_xor mit den Seeds in der Ruhe (keine eigene Ueberweisung seit 30 s und kein offener Ausgangskorb auf BEIDEN Seiten, Hoehe gleichauf). " +
			"abweichend=true ab 3 Vergleichen in Folge mit Unterschied -- dann stimmen Kontostaende nicht ueberein, nicht nur Geschwister-Unschaerfe. " +
			"autoresync=true (AEQUITAS_DIVERGENZ_AUTORESYNC=1, fuer Validatoren, die nicht Seed sind): dann Resync vom Seed statt nur Meldung.",
		"autoresync":        strings.TrimSpace(os.Getenv("AEQUITAS_DIVERGENZ_AUTORESYNC")) == "1",
		"abweichend":        strikes >= divergenzSchwelle,
		"strikes":           strikes,
		"seit":              seit,
		"peer":              peer,
		"vergleiche":        divergenzVergleiche.Load(),
		"vergleiche_gleich": divergenzGleich.Load(),
	}
}
