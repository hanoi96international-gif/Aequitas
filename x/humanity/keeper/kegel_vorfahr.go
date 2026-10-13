package keeper

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// LIEGT EIN BLOCK IN DER VERGANGENHEIT EINES ANDEREN? (OFFEN 10, Stufe C,
// Baustein 3)
//
// Ob ein Block ein Vorfahr eines anderen ist, haengt nur an Hashes und
// Eltern-Verweisen -- jeder Knoten mit derselben Geschichte urteilt gleich,
// anders als "von diesem Knoten nachgespielt" (Replay-Status, Neustart,
// Pruning). Abgestiegen wird von den Eltern des tragenden Blocks bis zur
// Zeit grenze; was darunter liegt, wird nicht weiter verfolgt (ein Kind darf
// hoechstens zeitstempelRueckToleranz vor seinem Elternblock liegen).
//
// Begrenzt: hoechstens vorfahrBesuchsGrenze besuchte Bloecke, Nachschlagen in
// Stapeln. Fehlt ein Block auf dem Weg (eigene Luecke) oder wird die Grenze
// erreicht: vollstaendig = false -- der Aufrufer darf dann nicht "kein
// Vorfahr" schliessen.

const vorfahrStapel = 1000

// vorfahrBesuchsGrenze: hoechstens so viele besuchte Bloecke. Variable nur
// fuer Tests.
var vorfahrBesuchsGrenze = 50_000

// blockKopf: was die Vorfahren-Suche je Block braucht.
type blockKopf struct {
	hoehe  int64
	zeit   int64
	eltern []string
}

// kopfLeser: die Koepfe zu den Hashes; fehlende fehlen in der Antwort.
type kopfLeser func(hashes []string) (map[string]blockKopf, error)

var errVorfahrZuGross = errors.New("Vorfahren-Suche ueber der Grenze")

// vorfahrSuchen: liegt ziel im Vergangenheitskegel von eltern (ziel selbst
// eingeschlossen, wenn es unter eltern ist)? kopf: der Kopf von ziel, wenn
// gefunden.
func vorfahrSuchen(lies kopfLeser, eltern []string, ziel string, grenze int64) (kopf blockKopf, gefunden, vollstaendig bool, err error) {
	gesehen := make(map[string]bool, len(eltern))
	var offen []string
	for _, p := range eltern {
		if !gesehen[p] {
			gesehen[p] = true
			offen = append(offen, p)
		}
	}
	vollstaendig = true
	for len(offen) > 0 {
		koepfe := make(map[string]blockKopf, len(offen))
		for i := 0; i < len(offen); i += vorfahrStapel {
			j := i + vorfahrStapel
			if j > len(offen) {
				j = len(offen)
			}
			teil, err := lies(offen[i:j])
			if err != nil {
				return blockKopf{}, false, false, err
			}
			for h, k := range teil {
				koepfe[h] = k
			}
		}
		var naechste []string
		for _, h := range offen {
			k, ok := koepfe[h]
			if !ok {
				vollstaendig = false // eigene Luecke; die anderen Wege weiter
				continue
			}
			if h == ziel {
				return k, true, true, nil
			}
			if k.zeit < grenze {
				continue
			}
			for _, p := range k.eltern {
				if !gesehen[p] {
					gesehen[p] = true
					naechste = append(naechste, p)
				}
			}
			if len(gesehen) > vorfahrBesuchsGrenze {
				return blockKopf{}, false, false, nil
			}
		}
		offen = naechste
	}
	return blockKopf{}, false, vollstaendig, nil
}

// kopfLeserDB: die Koepfe aus chain_blocks.
func kopfLeserDB(q sqlExecutor) kopfLeser {
	return func(hashes []string) (map[string]blockKopf, error) {
		rows, err := q.Query(`SELECT hash, height, timestamp, parent_hashes FROM chain_blocks WHERE hash = ANY($1)`, pq.Array(hashes))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make(map[string]blockKopf, len(hashes))
		for rows.Next() {
			var h, eltern string
			var k blockKopf
			if err := rows.Scan(&h, &k.hoehe, &k.zeit, &eltern); err != nil {
				return nil, err
			}
			if err := json.Unmarshal([]byte(eltern), &k.eltern); err != nil {
				return nil, fmt.Errorf("Eltern von %s nicht lesbar: %w", kurzAdresse(h), err)
			}
			if len(k.eltern) > peerElternGrenze {
				return nil, fmt.Errorf("Block %s mit %d Eltern", kurzAdresse(h), len(k.eltern))
			}
			out[h] = k
		}
		return out, rows.Err()
	}
}
