package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

// /api/validator-stand?wallet=0x... -- fuer den Validator-Bereich der App:
// ist diese Wallet als Validator eingetragen, mit welchem Signierschluessel,
// und wie viele Bloecke hat dieser heute (Berliner Kalendertag) und insgesamt
// gebaut.
//
// Alles hier ist oeffentlich (Register, Bindungen, Bloecke stehen ohnehin im
// Explorer); der Endpunkt liest nur und schreibt nichts.
//
// GRENZEN (AGENTS.md, Sicherheit): jede Anfrage kommt von aussen.
//   - hoechstens validatorStandSchluesselMax Schluessel je Wallet;
//   - Ergebnis je Wallet validatorStandCacheDauer zwischengespeichert, die
//     Ablage hat hoechstens validatorStandCacheMax Eintraege;
//   - hoechstens validatorStandGleichzeitig Berechnungen zugleich, sonst 503;
//   - jede Datenbankabfrage hat eine Frist (validatorStandFrist).
// Fail-closed: ist etwas nicht lesbar, kommt 503 -- nie ein "nicht
// eingetragen" oder "0 Bloecke", das nur aus einem Lesefehler stammt.

const (
	validatorStandSchluesselMax = 8
	validatorStandCacheDauer    = 20 * time.Second
	validatorStandCacheMax      = 512
	validatorStandGleichzeitig  = 4
	validatorStandFrist         = 3 * time.Second
)

type validatorStandSchluessel struct {
	Adresse string `json:"signing_address"`
	// Kette: im Validator-Register der Kette (validator_register).
	Kette bool `json:"kette"`
	// Ueberholt: der Schluessel hat spaeter einem anderen Betreiber zugestimmt.
	Ueberholt bool `json:"ueberholt"`
	// Knoten: Bindung bei diesem Knoten (validator_keys / validator_slots).
	Knoten bool `json:"knoten"`
	// Erzeuger: in der festen Erzeugerliste dieses Knotens.
	Erzeuger      bool  `json:"erzeuger"`
	BloeckeHeute  int64 `json:"bloecke_heute"`
	BloeckeGesamt int64 `json:"bloecke_gesamt"`
	// LetzterBlock: Unix-Sekunden des letzten gebauten Blocks, 0 = nie.
	LetzterBlock int64 `json:"letzter_block"`
}

type validatorStandAntwort struct {
	Wallet      string `json:"wallet"`
	Eingetragen bool   `json:"eingetragen"`
	Erzeuger    bool   `json:"erzeuger"`
	// TagBeginn: Beginn des heutigen Berliner Tages (Unix-Sekunden), ab dem
	// bloecke_heute zaehlt.
	TagBeginn  int64                      `json:"tag_beginn"`
	Schluessel []validatorStandSchluessel `json:"schluessel"`
	Stand      int64                      `json:"stand"`
}

type validatorStandEintrag struct {
	antwort validatorStandAntwort
	bis     time.Time
}

type validatorStandAblageTyp struct {
	mu      sync.Mutex
	eintrag map[string]validatorStandEintrag
}

var validatorStandAblage = &validatorStandAblageTyp{eintrag: map[string]validatorStandEintrag{}}

// validatorStandPlaetze begrenzt die gleichzeitigen Berechnungen.
var validatorStandPlaetze = make(chan struct{}, validatorStandGleichzeitig)

func (b *validatorStandAblageTyp) hole(wallet string, jetzt time.Time) (validatorStandAntwort, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.eintrag[wallet]
	if !ok || !jetzt.Before(e.bis) {
		return validatorStandAntwort{}, false
	}
	return e.antwort, true
}

// merke legt ab; ist die Ablage voll, fallen erst die abgelaufenen, und bleibt
// sie voll, wird nichts abgelegt (die Grenze gilt, die Antwort trotzdem).
func (b *validatorStandAblageTyp) merke(wallet string, a validatorStandAntwort, jetzt time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, da := b.eintrag[wallet]; !da && len(b.eintrag) >= validatorStandCacheMax {
		for k, e := range b.eintrag {
			if !jetzt.Before(e.bis) {
				delete(b.eintrag, k)
			}
		}
		if len(b.eintrag) >= validatorStandCacheMax {
			return
		}
	}
	b.eintrag[wallet] = validatorStandEintrag{antwort: a, bis: jetzt.Add(validatorStandCacheDauer)}
}

// berlinerTagBeginn: Mitternacht des Berliner Kalendertags von t.
func berlinerTagBeginn(t time.Time) time.Time {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		loc = time.FixedZone("CET", 3600)
	}
	b := t.In(loc)
	return time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, loc)
}

// validatorStandBerechnen liest den Stand einer Wallet. produzenten: die feste
// Erzeugerliste (klein geschrieben).
func (cs *ChainState) validatorStandBerechnen(ctx context.Context, wallet string, produzenten map[string]bool, jetzt time.Time) (validatorStandAntwort, error) {
	tag := berlinerTagBeginn(jetzt).Unix()
	aus := validatorStandAntwort{Wallet: wallet, TagBeginn: tag, Schluessel: []validatorStandSchluessel{}, Stand: jetzt.Unix()}
	if cs.db == nil {
		return aus, fmt.Errorf("keine Datenbank")
	}
	// Direkt ueber den Pool, mit Frist: dbExecCtx gaebe den ctx nicht an die
	// Abfrage weiter, und ein Lese-Endpunkt hat in keiner Transaktion etwas zu
	// suchen.
	db := cs.db
	nach := map[string]*validatorStandSchluessel{}
	reihe := []string{}
	hinzu := func(adr string) *validatorStandSchluessel {
		adr = strings.ToLower(strings.TrimSpace(adr))
		if !istHexAdresse(adr) {
			return nil
		}
		if s, ok := nach[adr]; ok {
			return s
		}
		if len(reihe) >= validatorStandSchluesselMax {
			return nil
		}
		s := &validatorStandSchluessel{Adresse: adr}
		nach[adr] = s
		reihe = append(reihe, adr)
		return s
	}

	// 1. Register der Kette: je Betreiber hoechstens eine Zeile.
	rows, err := db.QueryContext(ctx, `SELECT signing_address, ueberholt FROM validator_register WHERE operator_wallet = $1 LIMIT $2`,
		wallet, validatorStandSchluesselMax)
	if err != nil {
		return aus, fmt.Errorf("Register: %w", err)
	}
	for rows.Next() {
		var adr string
		var ueberholt bool
		if err := rows.Scan(&adr, &ueberholt); err != nil {
			rows.Close()
			return aus, fmt.Errorf("Register: %w", err)
		}
		if s := hinzu(adr); s != nil {
			s.Kette, s.Ueberholt = true, ueberholt
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return aus, fmt.Errorf("Register: %w", err)
	}
	rows.Close()

	// 2. Bindungen bei diesem Knoten.
	rows, err = db.QueryContext(ctx, `SELECT signing_address FROM validator_keys WHERE lower(human_wallet) = $1
		UNION SELECT signing_address FROM validator_slots WHERE lower(operator_wallet) = $1
		LIMIT $2`, wallet, validatorStandSchluesselMax)
	if err != nil {
		return aus, fmt.Errorf("Bindungen: %w", err)
	}
	for rows.Next() {
		var adr string
		if err := rows.Scan(&adr); err != nil {
			rows.Close()
			return aus, fmt.Errorf("Bindungen: %w", err)
		}
		if s := hinzu(adr); s != nil {
			s.Knoten = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return aus, fmt.Errorf("Bindungen: %w", err)
	}
	rows.Close()

	// 3. Bloecke je Schluessel (Index idx_chain_blocks_proposer_ts).
	if len(reihe) > 0 {
		rows, err = db.QueryContext(ctx, `SELECT lower(proposer),
				COUNT(*) FILTER (WHERE timestamp >= $2),
				COUNT(*),
				COALESCE(MAX(timestamp), 0)
			FROM chain_blocks WHERE lower(proposer) = ANY($1)
			GROUP BY lower(proposer)`, pq.Array(reihe), tag)
		if err != nil {
			return aus, fmt.Errorf("Bloecke: %w", err)
		}
		for rows.Next() {
			var adr string
			var heute, gesamt, letzter int64
			if err := rows.Scan(&adr, &heute, &gesamt, &letzter); err != nil {
				rows.Close()
				return aus, fmt.Errorf("Bloecke: %w", err)
			}
			if s, ok := nach[adr]; ok {
				s.BloeckeHeute, s.BloeckeGesamt, s.LetzterBlock = heute, gesamt, letzter
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return aus, fmt.Errorf("Bloecke: %w", err)
		}
		rows.Close()
	}

	sort.Strings(reihe)
	for _, adr := range reihe {
		s := nach[adr]
		s.Erzeuger = produzenten[adr]
		if s.Kette && !s.Ueberholt || s.Knoten {
			aus.Eingetragen = true
		}
		if s.Erzeuger && (s.Kette && !s.Ueberholt || s.Knoten) {
			aus.Erzeuger = true
		}
		aus.Schluessel = append(aus.Schluessel, *s)
	}
	return aus, nil
}

func (a *APIServer) handleValidatorStand(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("wallet")))
	if !istHexAdresse(wallet) {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}
	jetzt := time.Now()
	if v, ok := validatorStandAblage.hole(wallet, jetzt); ok {
		json.NewEncoder(w).Encode(v)
		return
	}
	select {
	case validatorStandPlaetze <- struct{}{}:
		defer func() { <-validatorStandPlaetze }()
	default:
		jsonError(w, "busy, try again in a few seconds", http.StatusServiceUnavailable)
		return
	}
	if a.blockchain == nil || a.blockchain.state == nil {
		jsonError(w, "validator state not readable", http.StatusServiceUnavailable)
		return
	}
	produzenten := map[string]bool{}
	for _, p := range a.blockchain.ProduzentenListe() {
		produzenten[strings.ToLower(p)] = true
	}
	ctx, abbruch := context.WithTimeout(r.Context(), validatorStandFrist)
	defer abbruch()
	v, err := a.blockchain.state.validatorStandBerechnen(ctx, wallet, produzenten, jetzt)
	if err != nil {
		// Grund nur ins Log: oeffentlich genuegt "nicht lesbar".
		fmt.Printf("[VALIDATOR-STAND] %s nicht lesbar: %v\n", kurzAdresse(wallet), err)
		jsonError(w, "validator state not readable, try again later", http.StatusServiceUnavailable)
		return
	}
	validatorStandAblage.merke(wallet, v, jetzt)
	json.NewEncoder(w).Encode(v)
}
