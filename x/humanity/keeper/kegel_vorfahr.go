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
// Pruning). Die Reihenfolge der Eltern steht im Blockhash, also auch die
// Reihenfolge der Suche und der Punkt, an dem eine Grenze greift.
//
// DEFINITION (keine Naeherung): "Vorfahr ueber Bloecke mit Zeit >= grenze".
// Abgestiegen wird nur ueber Bloecke, deren eigene Zeit mindestens grenze
// ist. Ein echter Vorfahr hinter einer rueckdatierten Zwischenkette (ein Kind
// darf je Stufe bis zu zeitstempelRueckToleranz vor seinem Elternblock
// liegen; nach sechs solchen Stufen faellt ein Zwischenblock unter die
// Grenze) zaehlt nicht -- fuer jeden Knoten gleich.
//
// Begrenzt (Pruefung von Baustein 3): hoechstens vorfahrBesuchsGrenze
// besuchte Bloecke, vorfahrElternGrenze gelesene Eltern-Verweise und
// vorfahrEbenenGrenze Ebenen (= Abfrage-Runden) -- jede Grenze VOR dem
// Lesen geprueft, nicht danach. Fehlt ein Block auf dem Weg (eigene Luecke):
// vollstaendig = false -- der Aufrufer darf dann nicht "kein Vorfahr"
// schliessen. Eine Grenze bei vollstaendigen Daten ist deterministisch
// (errVorfahrZuGross).

const vorfahrStapel = 1000

// Grenzen der Suche. Variablen nur fuer Tests.
var (
	vorfahrBesuchsGrenze = 50_000
	vorfahrElternGrenze  = 200_000
	// Ebenen: eine Abfrage-Runde je Ebene. 1024 deckt checkpointHoechstensAlt
	// bei einem Block je Sekunde mit Reserve; bei schnellerem Takt muss der
	// tragende Block einen juengeren Checkpoint nennen.
	vorfahrEbenenGrenze = 1024
)

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
// gefunden. err == errVorfahrZuGross: eine Grenze griff (deterministisch);
// sonstige Fehler: Lesefehler (nicht pruefbar).
func vorfahrSuchen(lies kopfLeser, eltern []string, ziel string, grenze int64) (kopf blockKopf, gefunden, vollstaendig bool, err error) {
	if len(eltern) > peerElternGrenze {
		return blockKopf{}, false, false, errVorfahrZuGross
	}
	gesehen := make(map[string]bool, len(eltern))
	var offen []string
	for _, p := range eltern {
		if !gesehen[p] {
			gesehen[p] = true
			offen = append(offen, p)
		}
	}
	vollstaendig = true
	elternGelesen := 0
	for ebene := 0; len(offen) > 0; ebene++ {
		if ebene >= vorfahrEbenenGrenze {
			return blockKopf{}, false, false, errVorfahrZuGross
		}
		var naechste []string
		// Stapelweise lesen und gleich verarbeiten -- keine ganze Ebene
		// im Speicher.
		for i := 0; i < len(offen); i += vorfahrStapel {
			j := i + vorfahrStapel
			if j > len(offen) {
				j = len(offen)
			}
			koepfe, err := lies(offen[i:j])
			if err != nil {
				return blockKopf{}, false, false, err
			}
			for _, h := range offen[i:j] {
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
				elternGelesen += len(k.eltern)
				if elternGelesen > vorfahrElternGrenze {
					return blockKopf{}, false, false, errVorfahrZuGross
				}
				for _, p := range k.eltern {
					if !gesehen[p] {
						if len(gesehen) >= vorfahrBesuchsGrenze {
							return blockKopf{}, false, false, errVorfahrZuGross
						}
						gesehen[p] = true
						naechste = append(naechste, p)
					}
				}
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
