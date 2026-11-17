package keeper

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lib/pq"
)

// Tagesrunde zum Nachrechnen: /api/wirtschaft/runde.
//
// # WOZU
//
// Die App erklaert eine Runde mit der Formel (Monatssatz x (fairer Anteil -
// Guthaben)). Mit Zahlen nachrechnen liess sie sich nicht: Topfgroesse, Zahl
// der Empfaenger und Anteil je Mensch gab die Kette nicht heraus. Diese
// Abfrage legt fuer eine Runde offen, was in den Bloecken steht:
//
//	Rest der Vorrunde + Abgaben dieser Runde + andere Zufluesse
//	  = Topf vor der Auszahlung = ausgezahlt + Rest nach der Runde
//	ausgezahlt = Menschen x Anteil je Mensch
//
// "Andere Zufluesse" (Swap-Gebuehren, Treuhand, Kappungen zwischen den Runden)
// stehen nicht als eigene Rundenbuchung in den Bloecken; sie sind die
// Differenz und als solche benannt.
//
// # WOHER
//
// Aus den Bloecken, beim Uebernehmen, auf JEDEM Knoten -- im selben
// Hintergrund-Schreiber wie der Kontoverlauf (tx_block_index_async.go), nie
// auf dem Blockpfad. KEIN Konsens: Wer nachpruefen will, fragt einen zweiten
// Knoten oder liest die Bloecke im Explorer (jede Zeile nennt ihren Block).
//
// # EINE ZEILE JE RUNDE, ART UND KONTO
//
// Im DAG koennen zwei Geschwisterbloecke dieselbe Runde tragen; die Kette
// spielt sie nur einmal nach (skipDistributionRound). Je Block summiert,
// zaehlte eine solche Runde doppelt. Der Schluessel (Rundenzeit, Art, Konto)
// laesst das Doppel von selbst herausfallen, und jede Summe bleibt auf ihre
// Einzelbuchungen zurueckfuehrbar.
//
// # AELTERE RUNDEN
//
// Der Schreiber beginnt mit dem Einspielen dieser Datei. Fragt jemand nach
// dem Block einer aelteren Runde (hoehe=), traegt die Abfrage genau diese
// Runde aus den Bloecken nach: nur angewendete Bloecke (replayed), die
// zusammenhaengenden Hoehen mit Buchungen derselben Rundenzeit, hoechstens
// rundeNachtragHoehen, eine Nachtragung zugleich, hinter der Lesegrenze je
// Absender.

// rundenArten: was zu einer Tagesrunde gehoert (state.go,
// RunDailyDistributionAtomic). Die Marke steht immer am Ende der Runde.
var rundenArten = map[string]bool{
	"umlauf":                    true,
	"ubi_distribution":          true,
	"ubi_distribution_finalize": true,
	"distribution_round_marker": true,
	"kappung":                   true,
	"escrow_release":            true,
}

// rundeNachtragHoehen: so viele Hoehen liest eine Abfrage hoechstens, um eine
// aeltere Runde nachzutragen (in beide Richtungen zusammen).
const rundeNachtragHoehen = 200

type rundenBuchung struct {
	zeit    int64
	art     string
	adresse string
	mikro   int64
}

// rundenMikro: AEQ in Mikro-AEQ, exakt fuer die round6-Betraege der Kette;
// Unlesbares (NaN, Inf, negativ, riesig) zaehlt nicht.
func rundenMikro(x float64) (int64, bool) {
	if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 || x > 1e12 {
		return 0, false
	}
	return int64(math.Round(x * 1e6)), true
}

// rundenBuchungen: die Buchungen einer Tagesrunde in einem Block. Eine
// Buchung ohne eigene Rundenzeit (Grundeinkommen vor lebensnachweisAktiv,
// Kappung, Treuhand) gehoert zur Runde, deren Marke, Endstand oder Abgabe im
// selben Block steht; ohne die bleibt sie draussen.
func rundenBuchungen(txs []Transaction) []rundenBuchung {
	var at int64
	for _, tx := range txs {
		switch tx.Type {
		case "distribution_round_marker", "ubi_distribution_finalize", "umlauf":
			if tx.DistributionAt > 0 {
				at = tx.DistributionAt
			}
		}
		if at > 0 {
			break
		}
	}
	var out []rundenBuchung
	for _, tx := range txs {
		if !rundenArten[tx.Type] {
			continue
		}
		z := tx.DistributionAt
		if z <= 0 {
			z = at
		}
		if z <= 0 {
			continue
		}
		m, ok := rundenMikro(tx.Amount)
		if !ok {
			continue
		}
		adr := strings.ToLower(strings.TrimSpace(tx.Wallet))
		if tx.Type == "ubi_distribution_finalize" || tx.Type == "distribution_round_marker" {
			adr = "" // einmal je Runde
		}
		out = append(out, rundenBuchung{zeit: z, art: tx.Type, adresse: adr, mikro: m})
	}
	return out
}

var (
	rundenGeschrieben atomic.Int64
	rundenFehler      atomic.Int64
	rundeNachtragMu   sync.Mutex
)

func (cs *ChainState) ensureRundenTabelle() {
	if cs.db == nil {
		return
	}
	cs.rundenOnce.Do(func() {
		cs.db.Exec(`CREATE TABLE IF NOT EXISTS wirtschaft_runden_buchungen (
			zeit         BIGINT NOT NULL,
			art          TEXT NOT NULL,
			adresse      TEXT NOT NULL DEFAULT '',
			mikro        BIGINT NOT NULL,
			block_height BIGINT NOT NULL,
			PRIMARY KEY (zeit, art, adresse)
		)`)
		cs.db.Exec(`CREATE INDEX IF NOT EXISTS wirtschaft_runden_buchungen_hoehe
			ON wirtschaft_runden_buchungen (block_height)`)
	})
}

// schreibeRundenBuchungen: die Rundenbuchungen eines uebernommenen Blocks,
// eine Anweisung, idempotent (ON CONFLICT DO NOTHING).
func (cs *ChainState) schreibeRundenBuchungen(height int64, txs []Transaction) error {
	if cs.db == nil || len(txs) == 0 {
		return nil
	}
	return cs.rundenEinfuegen(height, rundenBuchungen(txs))
}

func (cs *ChainState) rundenEinfuegen(height int64, bu []rundenBuchung) error {
	if len(bu) == 0 {
		return nil
	}
	cs.ensureRundenTabelle()
	n := len(bu)
	zeit, mikro := make([]int64, n), make([]int64, n)
	art, adr := make([]string, n), make([]string, n)
	for i, b := range bu {
		zeit[i], art[i], adr[i], mikro[i] = b.zeit, b.art, b.adresse, b.mikro
	}
	// cs.db direkt, nie dbExec(): siehe IndexBlockTransactions.
	_, err := cs.db.Exec(`INSERT INTO wirtschaft_runden_buchungen (zeit, art, adresse, mikro, block_height)
		SELECT z, t, a, m, $5 FROM unnest($1::bigint[], $2::text[], $3::text[], $4::bigint[]) AS v(z, t, a, m)
		ON CONFLICT DO NOTHING`,
		pq.Array(zeit), pq.Array(art), pq.Array(adr), pq.Array(mikro), height)
	if err != nil {
		rundenFehler.Add(1)
		return fmt.Errorf("Rundenbuchungen Block #%d (%d Zeilen): %w", height, n, err)
	}
	rundenGeschrieben.Add(int64(n))
	return nil
}

// rundeSumme: eine Art einer Runde, zusammengefasst.
type rundeSumme struct {
	anzahl, summe, kleinster, groesster int64
	vonBlock, bisBlock                  int64
}

func mikroAEQ(m int64) float64 { return float64(m) / 1e6 }

// rundeAuswerten: die Antwort zu einer Runde aus ihren Summen und dem
// Endstand der Vorrunde (vorRest nil: unbekannt). Reine Rechnung, getestet.
func rundeAuswerten(zeit int64, s map[string]rundeSumme, vorZeit int64, vorRest *int64) map[string]interface{} {
	ab, ubi := s["umlauf"], s["ubi_distribution"]
	fin, finDa := s["ubi_distribution_finalize"]
	_, markeDa := s["distribution_round_marker"]
	var von, bis int64
	for _, x := range s {
		if von == 0 || x.vonBlock < von {
			von = x.vonBlock
		}
		if x.bisBlock > bis {
			bis = x.bisBlock
		}
	}
	antwort := map[string]interface{}{
		"zeit":    zeit,
		"bloecke": map[string]int64{"von": von, "bis": bis},
		// Die Marke steht am Ende jeder Runde: ohne sie fehlt ein Teil.
		"vollstaendig": markeDa && (ubi.anzahl == 0 || finDa),
		"abgaben": map[string]interface{}{
			"konten": ab.anzahl, "summe": mikroAEQ(ab.summe), "summe_mikro": ab.summe,
		},
		"grundeinkommen": map[string]interface{}{
			"menschen": ubi.anzahl, "summe": mikroAEQ(ubi.summe), "summe_mikro": ubi.summe,
			"je_mensch_kleinster": mikroAEQ(ubi.kleinster), "je_mensch_groesster": mikroAEQ(ubi.groesster),
		},
		"kappungen":          map[string]interface{}{"anzahl": s["kappung"].anzahl, "summe": mikroAEQ(s["kappung"].summe)},
		"treuhand_freigaben": map[string]interface{}{"anzahl": s["escrow_release"].anzahl, "summe": mikroAEQ(s["escrow_release"].summe)},
		"rechnung": "rest_vorrunde + abgaben + andere_zufluesse = topf_vor_auszahlung = grundeinkommen + rest_nach; " +
			"grundeinkommen = menschen x je_mensch. andere_zufluesse (Swap-Gebuehren, Treuhand, Kappungen zwischen den Runden) ist die Differenz.",
		"hinweis": "Abgeleitet aus den Bloecken, kein Konsens; jeder Knoten schreibt es selbst. Gegenpruefen: dieselbe Abfrage bei einem zweiten Knoten oder die Bloecke von bis im Explorer.",
	}
	if ubi.anzahl > 0 && ubi.kleinster == ubi.groesster {
		antwort["probe_menschen_mal_anteil"] = map[string]interface{}{
			"menschen_mal_anteil_mikro": ubi.anzahl * ubi.kleinster,
			"ausgezahlt_mikro":          ubi.summe,
			"gleich":                    ubi.anzahl*ubi.kleinster == ubi.summe,
		}
	}
	if finDa {
		antwort["rest_nach"] = mikroAEQ(fin.summe)
		antwort["topf_vor_auszahlung"] = mikroAEQ(ubi.summe + fin.summe)
		if vorRest != nil {
			antwort["vorrunde"] = vorZeit
			antwort["rest_vorrunde"] = mikroAEQ(*vorRest)
			antwort["andere_zufluesse"] = mikroAEQ(ubi.summe + fin.summe - *vorRest - ab.summe)
		}
	}
	return antwort
}

var errRundeUnbekannt = errors.New("keine Runde zu dieser Angabe")

// rundeZeitZuHoehe: die Rundenzeit der Buchungen in Block hoehe; traegt eine
// aeltere Runde bei Bedarf aus den Bloecken nach.
func (cs *ChainState) rundeZeitZuHoehe(hoehe int64) (int64, error) {
	cs.ensureRundenTabelle()
	var z int64
	err := cs.db.QueryRow(`SELECT zeit FROM wirtschaft_runden_buchungen WHERE block_height = $1 LIMIT 1`, hoehe).Scan(&z)
	if err == nil {
		return z, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return cs.rundeNachtragen(hoehe)
}

// rundeNachtragen: eine aeltere Runde aus den Bloecken -- begrenzt (Hoehen,
// eine zugleich), idempotent.
func (cs *ChainState) rundeNachtragen(hoehe int64) (int64, error) {
	if !rundeNachtragMu.TryLock() {
		return 0, errRundeBeschaeftigt
	}
	defer rundeNachtragMu.Unlock()
	gelesen := 0
	lies := func(h int64) []rundenBuchung {
		gelesen++
		// Nur Bloecke, die dieser Knoten ANGEWENDET hat (replayed): ein
		// gespeicherter, aber nicht nachgespielter Block -- etwa von einem
		// Peer, dessen Pruefung scheiterte -- darf keine Rundenzahlen liefern
		// (und mit "erste Zeile gewinnt" die echten verdecken).
		angewendet := cs.angewendeteHashesAufHoehe(h)
		var bu []rundenBuchung
		for _, b := range cs.LoadBlocksAtHeightFromDB(h) {
			if b == nil || !angewendet[b.Hash] {
				continue
			}
			bu = append(bu, rundenBuchungen(b.Transactions)...)
		}
		return bu
	}
	nurZeit := func(bu []rundenBuchung, z int64) []rundenBuchung {
		var out []rundenBuchung
		for _, b := range bu {
			if b.zeit == z {
				out = append(out, b)
			}
		}
		return out
	}
	erst := lies(hoehe)
	if len(erst) == 0 {
		return 0, errRundeUnbekannt
	}
	z := erst[0].zeit
	if err := cs.rundenEinfuegen(hoehe, nurZeit(erst, z)); err != nil {
		return 0, err
	}
	for _, schritt := range []int64{-1, 1} {
		for h := hoehe + schritt; h > 0 && gelesen < rundeNachtragHoehen; h += schritt {
			bu := nurZeit(lies(h), z)
			if len(bu) == 0 {
				break
			}
			if err := cs.rundenEinfuegen(h, bu); err != nil {
				return 0, err
			}
		}
	}
	return z, nil
}

var errRundeBeschaeftigt = errors.New("eine andere Runde wird gerade nachgetragen")

// angewendeteHashesAufHoehe: die Bloecke einer Hoehe, die dieser Knoten
// angewendet hat (chain_blocks.replayed, siehe ensureReplayedColumn).
func (cs *ChainState) angewendeteHashesAufHoehe(h int64) map[string]bool {
	out := map[string]bool{}
	if cs.db == nil {
		return out
	}
	cs.ensureReplayedColumn()
	rows, err := cs.db.Query(`SELECT hash FROM chain_blocks WHERE height = $1 AND replayed`, h)
	if err != nil {
		return out // fail-closed: nichts nachtragen
	}
	defer rows.Close()
	for rows.Next() {
		var x string
		if rows.Scan(&x) == nil {
			out[x] = true
		}
	}
	return out
}

// RundeStand: die Antwort fuer eine Runde (zeit > 0) oder die juengste (0).
func (cs *ChainState) RundeStand(zeit int64) (map[string]interface{}, error) {
	if cs.db == nil {
		return nil, errors.New("keine Datenbank")
	}
	cs.ensureRundenTabelle()
	if zeit <= 0 {
		var z sql.NullInt64
		if err := cs.db.QueryRow(`SELECT MAX(zeit) FROM wirtschaft_runden_buchungen WHERE art = 'distribution_round_marker'`).Scan(&z); err != nil {
			return nil, err
		}
		if !z.Valid {
			return nil, errRundeUnbekannt
		}
		zeit = z.Int64
	}
	rows, err := cs.db.Query(`SELECT art, COUNT(*), COALESCE(SUM(mikro), 0), COALESCE(MIN(mikro), 0), COALESCE(MAX(mikro), 0),
			MIN(block_height), MAX(block_height)
		FROM wirtschaft_runden_buchungen WHERE zeit = $1 GROUP BY art`, zeit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	s := map[string]rundeSumme{}
	for rows.Next() {
		var art string
		var x rundeSumme
		if err := rows.Scan(&art, &x.anzahl, &x.summe, &x.kleinster, &x.groesster, &x.vonBlock, &x.bisBlock); err != nil {
			return nil, err
		}
		s[art] = x
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(s) == 0 {
		return nil, errRundeUnbekannt
	}
	// Endstand der unmittelbar vorigen Runde: die juengste Marke davor. Hat
	// sie keinen Endstand (Topf leer, nichts ausgezahlt), ist der Rest
	// unbekannt -- dann keine Differenz.
	var vorZeit int64
	var vorRest *int64
	var vz sql.NullInt64
	if err := cs.db.QueryRow(`SELECT MAX(zeit) FROM wirtschaft_runden_buchungen
		WHERE art = 'distribution_round_marker' AND zeit < $1`, zeit).Scan(&vz); err == nil && vz.Valid {
		vorZeit = vz.Int64
		var r int64
		if err := cs.db.QueryRow(`SELECT mikro FROM wirtschaft_runden_buchungen
			WHERE zeit = $1 AND art = 'ubi_distribution_finalize' AND adresse = ''`, vorZeit).Scan(&r); err == nil {
			vorRest = &r
		}
	}
	return rundeAuswerten(zeit, s, vorZeit, vorRest), nil
}

// rundeZahl: eine positive ganze Zahl aus der Anfrage (leer: 0).
func rundeZahl(r *http.Request, name string) (int64, bool) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return 0, true
	}
	if len(v) > 12 {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// handleWirtschaftRunde: GET /api/wirtschaft/runde?hoehe=N (Block einer
// Buchung der Runde, etwa aus dem Kontoverlauf) oder ?zeit=T (Rundenzeit);
// ohne beides die juengste Runde.
func (a *APIServer) handleWirtschaftRunde(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	hoehe, ok1 := rundeZahl(r, "hoehe")
	zeit, ok2 := rundeZahl(r, "zeit")
	if !ok1 || !ok2 || (hoehe > 0 && zeit > 0) {
		jsonError(w, "hoehe oder zeit: eine positive ganze Zahl, nicht beides", http.StatusBadRequest)
		return
	}
	cs := a.state
	if cs == nil || cs.db == nil {
		jsonError(w, "nicht verfuegbar", http.StatusServiceUnavailable)
		return
	}
	if hoehe > 0 {
		z, err := cs.rundeZeitZuHoehe(hoehe)
		switch {
		case errors.Is(err, errRundeUnbekannt):
			jsonError(w, "in diesem Block steht keine Buchung einer Tagesrunde", http.StatusNotFound)
			return
		case errors.Is(err, errRundeBeschaeftigt):
			jsonError(w, err.Error()+"; bitte gleich noch einmal", http.StatusServiceUnavailable)
			return
		case err != nil:
			jsonError(w, "Abfrage fehlgeschlagen", http.StatusInternalServerError)
			return
		}
		zeit = z
	}
	antwort, err := cs.RundeStand(zeit)
	switch {
	case errors.Is(err, errRundeUnbekannt):
		jsonError(w, "keine Runde zu dieser Zeit (aeltere Runden: mit hoehe= abfragen)", http.StatusNotFound)
		return
	case err != nil:
		jsonError(w, "Abfrage fehlgeschlagen", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(antwort)
}
