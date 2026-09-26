package keeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/lib/pq"
)

// Kontoverlauf: welche Buchungen ein Konto betrafen -- fuer die App
// ("Verlauf" im Wallet) und jeden, der sein Konto nachvollziehen will.
//
// # WOHER
//
// Aus den Bloecken, beim Uebernehmen, auf JEDEM Knoten -- im selben
// Hintergrund-Schreiber wie chain_tx_block_index (tx_block_index_async.go),
// also nie auf dem Blockpfad. Faellt ein Schreibversuch aus, traegt der
// Nachtraeger den Block spaeter nach; die Zeilen sind idempotent (ON CONFLICT
// DO NOTHING).
//
// # WAS NICHT
//
// Nur Konten von Menschen und Unternehmen bekommen Zeilen. Freie Adressen --
// darunter die Lasttest-Konten, die C1 mit tausenden Ueberweisungen je
// Sekunde fluten -- nicht: sonst verdreifachte jeder Lastlauf die
// Schreibmenge, und die Platte ist gemessen der Engpass des Durchsatzes.
// Systemkonten (Toepfe) ebenso nicht.
//
// Der Verlauf ist KEIN Konsens. Er beginnt mit dem Einspielen dieser Datei;
// aeltere Buchungen stehen weiter in den Bloecken (Explorer).

// verlaufArten: welche Transaktionsarten ein Mensch im Verlauf sieht. Marker
// und Topf-Buchhaltung (distribution_round_marker, *_pool_zero,
// ubi_distribution_finalize, pool_correction) gehoeren nicht dazu.
var verlaufArten = map[string]bool{
	"transfer": true, "ubi_distribution": true, "register_human": true,
	"grant_release": true, "umlauf": true, "kappung": true,
	"validator_distribution": true, "lp_distribution": true,
	"swap": true, "swap_aeq_tusd": true, "add_liquidity": true, "remove_liquidity": true,
	"faucet": true, "escrow_release": true, "escrow_recover": true,
}

type verlaufZeile struct {
	adresse     string
	txIndex     int32
	seite       int16 // 0 = Absender/Betroffener, 1 = Empfaenger einer Ueberweisung
	txHash      string
	art         string
	gegenpartei string
	betrag      float64
	gebuehr     float64
	zeit        int64
}

// verlaufZeilen leitet die Zeilen eines Blocks ab. relevant sagt, ob eine
// Adresse einen Verlauf fuehrt. Reine Rechnung, ohne Datenbank.
func verlaufZeilen(txs []Transaction, blockZeit int64, relevant func(string) bool) []verlaufZeile {
	var out []verlaufZeile
	for i, tx := range txs {
		if !verlaufArten[tx.Type] {
			continue
		}
		zeit := blockZeit
		if tx.BuchAt > 0 {
			zeit = tx.BuchAt
		}
		von := strings.ToLower(strings.TrimSpace(tx.Wallet))
		an := strings.ToLower(strings.TrimSpace(tx.To))
		hash := strings.ToLower(strings.TrimSpace(tx.TxHash))
		if von != "" && relevant(von) {
			out = append(out, verlaufZeile{adresse: von, txIndex: int32(i), seite: 0, txHash: hash, art: tx.Type,
				gegenpartei: an, betrag: tx.Amount, gebuehr: tx.Gebuehr, zeit: zeit})
		}
		if tx.Type == "transfer" && an != "" && an != von && relevant(an) {
			out = append(out, verlaufZeile{adresse: an, txIndex: int32(i), seite: 1, txHash: hash, art: tx.Type,
				gegenpartei: von, betrag: tx.Amount, zeit: zeit})
		}
	}
	return out
}

var (
	verlaufGeschrieben atomic.Int64
	verlaufFehler      atomic.Int64
)

func (cs *ChainState) ensureKontoVerlaufTable() {
	if cs.db == nil {
		return
	}
	cs.kontoVerlaufOnce.Do(func() {
		cs.db.Exec(`CREATE TABLE IF NOT EXISTS chain_konto_verlauf (
			adresse      TEXT NOT NULL,
			block_height BIGINT NOT NULL,
			tx_index     INT NOT NULL,
			seite        SMALLINT NOT NULL,
			tx_hash      TEXT NOT NULL DEFAULT '',
			art          TEXT NOT NULL,
			gegenpartei  TEXT NOT NULL DEFAULT '',
			betrag       DOUBLE PRECISION NOT NULL DEFAULT 0,
			gebuehr      DOUBLE PRECISION NOT NULL DEFAULT 0,
			zeit         BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (adresse, block_height, tx_index, seite)
		)`)
	})
}

// verlaufRelevant: fuehrt diese Adresse einen Verlauf? Menschen und
// Unternehmen ja, freie Adressen und Toepfe nicht.
func (cs *ChainState) verlaufRelevant(addr string) bool {
	if istSystemAdresse(addr) {
		return false
	}
	var mensch bool
	cs.mu.RLock()
	if acc, ok := cs.accounts.Get(addr); ok {
		mensch = acc.IsHuman
	}
	cs.mu.RUnlock()
	if mensch {
		return true
	}
	return cs.wirt().istUnternehmen(addr)
}

// schreibeKontoVerlauf: die Zeilen eines uebernommenen Blocks, eine Anweisung.
func (cs *ChainState) schreibeKontoVerlauf(height int64, blockZeit int64, txs []Transaction) error {
	if cs.db == nil || len(txs) == 0 {
		return nil
	}
	zeilen := verlaufZeilen(txs, blockZeit, cs.verlaufRelevant)
	if len(zeilen) == 0 {
		return nil
	}
	cs.ensureKontoVerlaufTable()
	n := len(zeilen)
	adr, hash, art, gegen := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	idx, seite := make([]int64, n), make([]int64, n)
	betrag, gebuehr := make([]float64, n), make([]float64, n)
	zeit := make([]int64, n)
	for i, z := range zeilen {
		adr[i], hash[i], art[i], gegen[i] = z.adresse, z.txHash, z.art, z.gegenpartei
		idx[i], seite[i] = int64(z.txIndex), int64(z.seite)
		betrag[i], gebuehr[i], zeit[i] = z.betrag, z.gebuehr, z.zeit
	}
	// cs.db direkt, nie dbExec(): siehe IndexBlockTransactions.
	_, err := cs.db.Exec(`INSERT INTO chain_konto_verlauf
		(adresse, block_height, tx_index, seite, tx_hash, art, gegenpartei, betrag, gebuehr, zeit)
		SELECT a, $2, i, s, h, t, g, b, f, z
		FROM unnest($1::text[], $3::int[], $4::smallint[], $5::text[], $6::text[], $7::text[],
		            $8::double precision[], $9::double precision[], $10::bigint[]) AS v(a, i, s, h, t, g, b, f, z)
		ON CONFLICT DO NOTHING`,
		pq.Array(adr), height, pq.Array(idx), pq.Array(seite), pq.Array(hash), pq.Array(art), pq.Array(gegen),
		pq.Array(betrag), pq.Array(gebuehr), pq.Array(zeit))
	if err != nil {
		verlaufFehler.Add(1)
		return fmt.Errorf("Kontoverlauf Block #%d (%d Zeilen): %w", height, n, err)
	}
	verlaufGeschrieben.Add(int64(n))
	return nil
}

// VerlaufEintrag: eine Zeile, wie /api/verlauf sie liefert.
type VerlaufEintrag struct {
	Hoehe       int64   `json:"hoehe"`
	TxIndex     int     `json:"tx_index"`
	Richtung    string  `json:"richtung"` // "aus", "ein" oder "neutral"
	Art         string  `json:"art"`
	Gegenpartei string  `json:"gegenpartei,omitempty"`
	Betrag      float64 `json:"betrag"`
	Gebuehr     float64 `json:"gebuehr,omitempty"`
	Zeit        int64   `json:"zeit"`
	TxHash      string  `json:"tx_hash,omitempty"`
}

// KontoVerlauf: die juengsten Eintraege eines Kontos, neueste zuerst; vor
// begrenzt auf Bloecke unterhalb dieser Hoehe (Blaettern).
func (cs *ChainState) KontoVerlauf(adresse string, vor int64, limit int) ([]VerlaufEintrag, error) {
	out := []VerlaufEintrag{}
	if cs.db == nil {
		return out, nil
	}
	cs.ensureKontoVerlaufTable()
	rows, err := cs.db.Query(`SELECT block_height, tx_index, seite, art, gegenpartei, betrag, gebuehr, zeit, tx_hash
		FROM chain_konto_verlauf
		WHERE adresse = $1 AND ($2 = 0 OR block_height < $2)
		ORDER BY block_height DESC, tx_index DESC, seite DESC
		LIMIT $3`, strings.ToLower(adresse), vor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e VerlaufEintrag
		var seite int
		if err := rows.Scan(&e.Hoehe, &e.TxIndex, &seite, &e.Art, &e.Gegenpartei, &e.Betrag, &e.Gebuehr, &e.Zeit, &e.TxHash); err != nil {
			return nil, err
		}
		e.Richtung = verlaufRichtung(e.Art, seite)
		out = append(out, e)
	}
	return out, rows.Err()
}

// verlaufRichtung: kommt Geld herein oder geht es hinaus?
func verlaufRichtung(art string, seite int) string {
	switch art {
	case "transfer":
		if seite == 1 {
			return "ein"
		}
		return "aus"
	case "umlauf", "kappung":
		return "aus"
	case "swap", "swap_aeq_tusd", "add_liquidity", "remove_liquidity", "faucet":
		// Tausch und Liquiditaet bewegen AEQ und tUSD zugleich, der Faucet
		// nur tUSD: kein Vorzeichen fuer das AEQ-Guthaben.
		return "neutral"
	}
	return "ein"
}

func (a *APIServer) handleKontoVerlauf(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	addr, ok := normAdresse(r.URL.Query().Get("adresse"))
	if !ok {
		jsonError(w, "adresse must be a 0x address", http.StatusBadRequest)
		return
	}
	vor, _ := strconv.ParseInt(r.URL.Query().Get("vor"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	eintraege, err := a.state.KontoVerlauf(addr, vor, limit)
	if err != nil {
		jsonError(w, "verlauf nicht lesbar", http.StatusServiceUnavailable)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"adresse": addr, "eintraege": eintraege})
}

// KontoVerlaufStand fuer /api/health/combined.
func KontoVerlaufStand() map[string]interface{} {
	return map[string]interface{}{"zeilen_geschrieben": verlaufGeschrieben.Load(), "fehler": verlaufFehler.Load()}
}
