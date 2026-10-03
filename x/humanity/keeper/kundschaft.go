package keeper

// Zwei Zahlen, die zeigen statt versprechen (docs/WIRTSCHAFT_REIFEPRUEFUNG.md):
//
//  1. ECHTE KUNDSCHAFT je Unternehmen: wie viele VERSCHIEDENE verifizierte
//     Menschen in den letzten 90 Tagen dort bezahlt haben (je Zahlung
//     mindestens 1 AEQ; die eigenen Verantwortlichen zaehlen nicht). Das ist
//     die Verifizierungsschicht 2: Weil jeder Mensch nur einmal existiert,
//     braucht jeder gefaelschte Kunde einen echten Menschen. Sie bringt kein
//     Geld, nur eine Anzeige im Register.
//
//  2. GRUNDEINKOMMEN der letzten 30 Tage: wie viel ein Mensch bekommen hat,
//     der die ganzen 30 Tage dabei war. Alle bekommen je Runde gleich viel;
//     die groesste Summe je Mensch im Fenster ist darum genau dieser Wert,
//     unabhaengig davon, wie sich eine Runde auf Bloecke verteilt.
//
// QUELLE ist chain_konto_verlauf (kontoverlauf.go): KEIN Konsens, beginnt mit
// dem Einspielen des Verlaufs, wird im Hintergrund geschrieben. Die Zahlen
// sind Anzeige, keine Regel haengt an ihnen.
//
// BEGRENZT: je Zahl eine einzige Sammelabfrage mit fester Wartezeit,
// zwischengespeichert fuer kundschaftCacheDauer; laeuft schon eine Rechnung,
// bekommt jede weitere Anfrage den alten Wert, statt eine zweite zu starten.
// Fehler: der alte Wert bleibt, sonst "unbekannt" (null), nie eine erfundene 0.

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

const (
	kundschaftFensterTage    = 90
	kundschaftMindestBetrag  = 1.0
	kundschaftCacheDauer     = 10 * time.Minute
	kundschaftWartezeit      = 5 * time.Second
	grundeinkommenFensterTag = 30
)

type kundschaftStand struct {
	mu    sync.Mutex // nur eine Rechnung zugleich (TryLock)
	lesen sync.RWMutex
	je    map[string]int
	ubi30 float64
	ubiOK bool
	// Weitergabe je Unternehmen (90 Tage), siehe rechneWeitergabe.
	weiter    map[string]weitergabe
	gerechnet time.Time
}

// weitergabe: wie viel vom Eingenommenen ein Unternehmen im Netz weitergibt
// (UNTERNEHMEN_KONZEPT.md 10.2 Nr. 6, "Ruf statt Rabatt"). Ein Laden, der
// AEQ sofort in Stable tauscht, hat eine hohe Ausstiegsquote; einer, der
// Lieferanten und Loehne in AEQ zahlt, eine hohe Weitergabe. Anzeige, kein
// Konsens, keine Regel haengt daran.
type weitergabe struct {
	Ein      float64 `json:"eingang_90_tage"`
	Weiter   float64 `json:"weitergegeben_90_tage"`
	Ausstieg float64 `json:"getauscht_90_tage"`
}

// quoten: Anteile am Eingang, auf 0..1 begrenzt (Weitergabe kann aus
// frueherem Guthaben groesser sein als der Eingang). nil ohne Eingang.
func (g weitergabe) quoten() map[string]interface{} {
	if g.Ein <= 0 {
		return map[string]interface{}{"weitergabe": nil, "ausstieg": nil, "eingang_90_tage": 0}
	}
	q := func(x float64) float64 { return round6(math.Min(1, math.Max(0, x/g.Ein))) }
	return map[string]interface{}{"weitergabe": q(g.Weiter), "ausstieg": q(g.Ausstieg), "eingang_90_tage": round6(g.Ein)}
}

var kundschaftCache kundschaftStand

// kundschaftUndGrundeinkommen liefert die zwischengespeicherten Zahlen und
// rechnet sie neu, wenn sie aelter als kundschaftCacheDauer sind.
func (cs *ChainState) kundschaftUndGrundeinkommen() (map[string]int, float64, bool) {
	k := &kundschaftCache
	k.lesen.RLock()
	frisch := !k.gerechnet.IsZero() && time.Since(k.gerechnet) < kundschaftCacheDauer
	je, ubi, ok := k.je, k.ubi30, k.ubiOK
	k.lesen.RUnlock()
	if frisch || cs == nil || cs.db == nil {
		return je, ubi, ok
	}
	if !k.mu.TryLock() {
		return je, ubi, ok // es rechnet schon jemand
	}
	defer k.mu.Unlock()

	neuJe, errK := cs.rechneKundschaft(time.Now())
	neuUbi, errU := cs.rechneGrundeinkommen30(time.Now())
	neuWeiter, errW := cs.rechneWeitergabe(time.Now())
	k.lesen.Lock()
	if errK == nil {
		k.je = neuJe
	} else {
		fmt.Printf("[KUNDSCHAFT] nicht berechnet (alter Wert bleibt): %v\n", errK)
	}
	if errW == nil {
		k.weiter = neuWeiter
	} else {
		fmt.Printf("[WEITERGABE] nicht berechnet (alter Wert bleibt): %v\n", errW)
	}
	if errU == nil {
		k.ubi30, k.ubiOK = neuUbi, true
	} else {
		fmt.Printf("[GRUNDEINKOMMEN] nicht berechnet (alter Wert bleibt): %v\n", errU)
	}
	k.gerechnet = time.Now()
	je, ubi, ok = k.je, k.ubi30, k.ubiOK
	k.lesen.Unlock()
	return je, ubi, ok
}

// rechneKundschaft: eine Abfrage fuer alle offenen Unternehmen.
func (cs *ChainState) rechneKundschaft(jetzt time.Time) (map[string]int, error) {
	liste := cs.unternehmenFuerSnapshot()
	var adressen, ausgenommen []string
	for _, e := range liste {
		if !e.offen() {
			continue
		}
		adressen = append(adressen, e.Adresse)
		for _, v := range e.Verantwortliche {
			ausgenommen = append(ausgenommen, e.Adresse+":"+strings.ToLower(v))
		}
	}
	out := make(map[string]int, len(adressen))
	if len(adressen) == 0 {
		return out, nil
	}
	// Die Tabelle entsteht sonst erst beim ersten Schreiben; ein frischer
	// Knoten (Beobachter, neuer Validator) las bis dahin ins Leere.
	cs.ensureKontoVerlaufTable()
	ctx, cancel := context.WithTimeout(context.Background(), kundschaftWartezeit)
	defer cancel()
	ab := jetzt.Add(-kundschaftFensterTage * 24 * time.Hour).Unix()
	rows, err := cs.db.QueryContext(ctx, `
		SELECT v.adresse, COUNT(DISTINCT v.gegenpartei)
		FROM chain_konto_verlauf v
		JOIN chain_accounts a ON a.address = v.gegenpartei AND a.is_human
		WHERE v.adresse = ANY($1) AND v.seite = 1 AND v.art = 'transfer'
		  AND v.zeit >= $2 AND v.betrag >= $3
		  AND (v.adresse || ':' || v.gegenpartei) <> ALL($4)
		GROUP BY v.adresse`,
		pq.Array(adressen), ab, kundschaftMindestBetrag, pq.Array(ausgenommen))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var adr string
		var n int
		if err := rows.Scan(&adr, &n); err != nil {
			return nil, err
		}
		out[adr] = n
	}
	return out, rows.Err()
}

// weitergabeVon: zwischengespeicherte Weitergabe (nach kundschaftUndGrundeinkommen).
func weitergabeVon(adresse string) (weitergabe, bool) {
	k := &kundschaftCache
	k.lesen.RLock()
	defer k.lesen.RUnlock()
	if k.weiter == nil {
		return weitergabe{}, false
	}
	return k.weiter[adresse], true
}

// rechneWeitergabe: eine Abfrage fuer alle offenen Unternehmen, 90 Tage.
// Eingang: Ueberweisungen an das Unternehmen. Weitergabe: Ueberweisungen des
// Unternehmens an andere (Loehne, Lieferanten, Entnahmen -- alles bleibt im
// Netz). Ausstieg: Tausch AEQ -> Stable.
func (cs *ChainState) rechneWeitergabe(jetzt time.Time) (map[string]weitergabe, error) {
	var adressen []string
	for _, e := range cs.unternehmenFuerSnapshot() {
		if e.offen() {
			adressen = append(adressen, e.Adresse)
		}
	}
	out := make(map[string]weitergabe, len(adressen))
	if len(adressen) == 0 {
		return out, nil
	}
	// Die Tabelle entsteht sonst erst beim ersten Schreiben; ein frischer
	// Knoten (Beobachter, neuer Validator) las bis dahin ins Leere.
	cs.ensureKontoVerlaufTable()
	ctx, cancel := context.WithTimeout(context.Background(), kundschaftWartezeit)
	defer cancel()
	ab := jetzt.Add(-kundschaftFensterTage * 24 * time.Hour).Unix()
	rows, err := cs.db.QueryContext(ctx, `
		SELECT adresse,
		       COALESCE(SUM(betrag) FILTER (WHERE art = 'transfer' AND seite = 1), 0),
		       COALESCE(SUM(betrag) FILTER (WHERE art = 'transfer' AND seite = 0), 0),
		       COALESCE(SUM(betrag) FILTER (WHERE art = 'swap_aeq_tusd' AND seite = 0), 0)
		FROM chain_konto_verlauf
		WHERE adresse = ANY($1) AND zeit >= $2
		GROUP BY adresse`, pq.Array(adressen), ab)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var adr string
		var g weitergabe
		if err := rows.Scan(&adr, &g.Ein, &g.Weiter, &g.Ausstieg); err != nil {
			return nil, err
		}
		out[adr] = g
	}
	return out, rows.Err()
}

// rechneGrundeinkommen30: groesste Summe an Grundeinkommen, die ein Mensch in
// den letzten 30 Tagen bekommen hat.
func (cs *ChainState) rechneGrundeinkommen30(jetzt time.Time) (float64, error) {
	// Die Tabelle entsteht sonst erst beim ersten Schreiben; ein frischer
	// Knoten (Beobachter, neuer Validator) las bis dahin ins Leere.
	cs.ensureKontoVerlaufTable()
	ctx, cancel := context.WithTimeout(context.Background(), kundschaftWartezeit)
	defer cancel()
	ab := jetzt.Add(-grundeinkommenFensterTag * 24 * time.Hour).Unix()
	var s float64
	err := cs.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(s), 0) FROM (
			SELECT adresse, SUM(betrag) AS s FROM chain_konto_verlauf
			WHERE art = 'ubi_distribution' AND zeit >= $1
			GROUP BY adresse) x`, ab).Scan(&s)
	return s, err
}

// kundschaftCacheLeeren: nur fuer Tests.
func kundschaftCacheLeeren() {
	k := &kundschaftCache
	k.lesen.Lock()
	k.je, k.ubi30, k.ubiOK, k.weiter, k.gerechnet = nil, 0, false, nil, time.Time{}
	k.lesen.Unlock()
}
