package keeper

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
)

// Validator-Register, Schritt 3, Teil 2: die Validatoren-Belohnung aus der
// Kette (docs/VALIDATOR_REGISTER_KONSENS.md).
//
// WARUM
//
// Bis hierhin gewichtete der Erzeuger die Belohnung nach den Minuten, in
// denen ein Betreiber aus registered_nodes Bloecke gebaut hat -- gezaehlt in
// SEINER chain_blocks. registered_nodes fuehrt jeder Knoten selbst, und
// welche Bloecke einer kennt, haengt davon ab, was er gesehen hat. Kein
// anderer Knoten konnte nachrechnen; geprueft wurde nur "ist Mensch".
//
// WAS (ab registerLeserAb, an der Zeit T der Runde)
//
//   - Der ANKER: der nachgespielte Block mit dem hoechsten Blue-Score in
//     [T-10 min, T]. Er steht mit T in jeder Zahlung (Transaction.Anker,
//     DistributionAt).
//   - Der KEGEL: alle Bloecke, die vom Anker ueber Eltern-Verweise
//     erreichbar sind, bis T-24h-10min. Er haengt nur an Hashes, nicht an
//     der Reihenfolge, in der ein Knoten Bloecke gesehen hat. Rote Bloecke
//     zaehlen mit -- auch sie sind unterschriebene Anwesenheit.
//   - Je Block mit Zeit in [T-24h, T) der Betreiber, dessen Erzeugerfenster
//     den Block zuliess (dieselben Zeitraeume und dieselbe Frist wie die
//     Erzeugerpruefung, nur menschliche Betreiber). Gewicht eines Betreibers:
//     die Zahl der Minuten mit mindestens einem solchen Block (wie bisher:
//     Anwesenheit, nicht Leistung).
//   - Betrag: floor6(Topf * Gewicht / Summe) -- dieselbe Formel wie bisher.
//
// Jeder Knoten rechnet mit Anker und T selbst nach (nachrechnen_validator.go).
//
// KEINE ZAHLUNG IN DIESER RUNDE (der Topf bleibt stehen), wenn kein Anker
// da ist, der Kegel groesser als anwesenheitKegelGrenze ist, kein Block
// einem Betreiber zuzurechnen ist -- oder dem Erzeuger Geschichte im Kegel
// fehlt (nach einer Neusynchronisation). Die ersten drei sieht jeder Knoten
// gleich; im letzten Fall rechnet ein Nachspielender, dem die Geschichte
// ebenfalls fehlt, nicht nach (unsicher), statt zu raten.

// anwesenheitKegelGrenze: so viele Bloecke liest die Kegelzaehlung
// hoechstens (ein Tag bei einem Block je Sekunde sind 86.400). Variable nur
// fuer Tests.
var anwesenheitKegelGrenze = 400_000

const (
	// anwesenheitKegelPuffer: so weit unter T-24h reicht der Kegel noch --
	// Kinder duerfen bis zu 120 s vor ihren Eltern liegen.
	anwesenheitKegelPuffer int64 = 600
	// validatorAnkerHoechstensAlt: der Anker liegt hoechstens so weit vor T.
	validatorAnkerHoechstensAlt int64 = 600
	// kegelStapel: so viele Hashes je Abfrage.
	kegelStapel = 2000
)

// validatorKetteAktiv: zahlt der Erzeuger die Validatoren der Runde mit
// dieser Zeit aus der Kette?
func validatorKetteAktiv(rundenZeit int64) bool { return registerLeserAktiv(rundenZeit) }

// validatorKettePflicht: muss eine Validatoren-Zahlung in einem Block mit
// dieser Zeit die neue Form (Anker) tragen? Zehn Minuten nach dem
// Stichtag: eine Runde, die der Erzeuger kurz vor dem Stichtag berechnet
// hat, darf noch in einem Block danach stehen.
func validatorKettePflicht(blockZeit int64) bool {
	return registerLeserAktiv(blockZeit - validatorAnkerHoechstensAlt)
}

// validatorAnkerWaehlen: der nachgespielte Block mit dem hoechsten
// Blue-Score in [T-10 min, T]; "" = keiner.
func validatorAnkerWaehlen(q sqlExecutor, T int64) (string, error) {
	var h string
	err := q.QueryRow(`SELECT hash FROM chain_blocks
		WHERE timestamp <= $1 AND timestamp >= $2 AND COALESCE(replayed, true)
		ORDER BY blue_score DESC, hash ASC LIMIT 1`, T, T-validatorAnkerHoechstensAlt).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return h, err
}

// validatorAnkerZeit: die Zeit des Ankers; gefunden = false: der Knoten
// kennt ihn nicht.
func validatorAnkerZeit(q sqlExecutor, anker string) (zeit int64, gefunden bool, err error) {
	err = q.QueryRow(`SELECT timestamp FROM chain_blocks WHERE hash = $1 AND COALESCE(replayed, true)`, anker).Scan(&zeit)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return zeit, err == nil, err
}

// kegelBlock: ein Block im Vergangenheitskegel.
type kegelBlock struct {
	proposer string
	zeit     int64
}

// kegelStand: wie vollstaendig der Kegel gelesen wurde.
type kegelStand int

const (
	kegelVollstaendig kegelStand = iota
	// kegelLuecke: ein Block fehlt in chain_blocks -- die Geschichte ist
	// nicht ganz da.
	kegelLuecke
	// kegelZuGross: mehr als anwesenheitKegelGrenze Bloecke.
	kegelZuGross
)

// bloeckeImKegel: die Bloecke im Vergangenheitskegel des Ankers (der Anker
// selbst eingeschlossen) mit Zeit in [seit, bis). Abgestiegen wird bis
// seit - anwesenheitKegelPuffer.
func bloeckeImKegel(q sqlExecutor, anker string, seit, bis int64) ([]kegelBlock, kegelStand, error) {
	untergrenze := seit - anwesenheitKegelPuffer
	gesehen := map[string]bool{anker: true}
	offen := []string{anker}
	var out []kegelBlock
	for len(offen) > 0 {
		n := len(offen)
		if n > kegelStapel {
			n = kegelStapel
		}
		stapel := offen[:n]
		offen = offen[n:]
		rows, err := q.Query(`SELECT hash, proposer, timestamp, parent_hashes FROM chain_blocks WHERE hash = ANY($1)`, pq.Array(stapel))
		if err != nil {
			return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
		}
		gefunden := 0
		for rows.Next() {
			var h, proposer, eltern string
			var ts int64
			if err := rows.Scan(&h, &proposer, &ts, &eltern); err != nil {
				rows.Close()
				return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
			}
			gefunden++
			if ts >= seit && ts < bis {
				out = append(out, kegelBlock{proposer: strings.ToLower(proposer), zeit: ts})
			}
			if ts < untergrenze {
				continue
			}
			var ph []string
			if err := json.Unmarshal([]byte(eltern), &ph); err != nil {
				rows.Close()
				return nil, kegelLuecke, fmt.Errorf("Kegel: Eltern von %s nicht lesbar: %w", kurzAdresse(h), err)
			}
			for _, e := range ph {
				if !gesehen[e] {
					gesehen[e] = true
					offen = append(offen, e)
				}
			}
			if len(gesehen) > anwesenheitKegelGrenze {
				rows.Close()
				return nil, kegelZuGross, nil
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
		}
		if gefunden < len(stapel) {
			return nil, kegelLuecke, nil
		}
	}
	return out, kegelVollstaendig, nil
}

// validatorGewichte: je Betreiber die Minuten in [T-24h, T) mit mindestens
// einem Block im Kegel des Ankers, zugerechnet dem Betreiber, dessen
// Erzeugerfenster den Block zuliess. q: die Transaktion des Erzeugers bzw.
// des Nachspielenden. Bei kegelLuecke oder kegelZuGross keine Gewichte.
func validatorGewichte(q sqlExecutor, anker string, T int64) (map[string]int64, kegelStand, error) {
	bloecke, stand, err := bloeckeImKegel(q, anker, T-anwesenheitsZeitraum, T)
	if err != nil || stand != kegelVollstaendig {
		return nil, stand, err
	}
	schluessel := map[string]bool{}
	for _, b := range bloecke {
		schluessel[b.proposer] = true
	}
	liste := make([]string, 0, len(schluessel))
	for s := range schluessel {
		liste = append(liste, s)
	}
	sort.Strings(liste)
	gewichte := map[string]int64{}
	if len(liste) == 0 {
		return gewichte, kegelVollstaendig, nil
	}
	fenster, err := fensterAusVerlauf(q, liste)
	if err != nil {
		return nil, kegelLuecke, fmt.Errorf("Erzeugerfenster: %w", err)
	}
	st := &erzeugerStand{fenster: fenster}
	minuten := map[string]map[int64]bool{}
	for _, b := range bloecke {
		op := st.erzeugerFenster(b.proposer, b.zeit)
		if op == "" {
			continue
		}
		if minuten[op] == nil {
			minuten[op] = map[int64]bool{}
		}
		minuten[op][b.zeit/60] = true
	}
	for op, m := range minuten {
		gewichte[op] = int64(len(m))
	}
	return gewichte, kegelVollstaendig, nil
}

// validatorAnteil: die eine Formel fuer den Anteil eines Betreibers.
func validatorAnteil(topf float64, gewicht, summe int64) float64 {
	return floor6(topf * float64(gewicht) / float64(summe))
}

// gewichteSumme: die Summe der Gewichte.
func gewichteSumme(g map[string]int64) int64 {
	var s int64
	for _, w := range g {
		s += w
	}
	return s
}
