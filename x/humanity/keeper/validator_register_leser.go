package keeper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Validator-Register, Schritt 3: die LESER (docs/VALIDATOR_REGISTER_KONSENS.md).
//
// Schritt 1 und 2 haben das Register auf die Kette gebracht. Gelesen wurde es
// bisher von niemandem: Strafkonto, Erzeugerliste und Leitung hingen weiter
// an Tabellen, die jeder Knoten selbst fuehrt (registered_nodes,
// validator_keys, Abgleich unter Peers). Hier stellen sie um -- jede an einem
// Zeitpunkt, der in der Kette steht (DetectedAt, Blockzeit), nie an der Uhr
// des Knotens, und vor dem Stichtag byte-gleich wie bisher.
//
// ZWEI STICHTAGE, BEIDE PLATZHALTER
//
//   - registerLeserAb: Strafkonto (slashing.go, strafKonto), Leitung
//     (validatorMenschVon) und der Abgleich unter Peers
//     (syncValidatorsFromPeer nimmt keine Bindungen mehr auf).
//   - erzeugerSchnittAb: wer Bloecke erzeugen darf. Mit AUTHORIZED_VALIDATORS
//     (geschlossen) die SCHNITTMENGE aus Liste und Register -- das Register
//     kann Erzeuger nur wegnehmen, nie hinzufuegen. Ohne Liste (offen) das
//     Register allein, statt des Abgleichs. Eigener Tag, weil er erst gesetzt
//     werden darf, wenn jeder Erzeuger der Liste gebunden ist (/api/status,
//     erzeuger_ohne_bindung) -- sonst schlosse sich C1 selbst aus und die
//     Kette stuende.
//
// Beide mindestens eine Woche nach validatorRegisterAb: bestehende Betreiber
// binden vorher neu (Schritt 2). Erzwungen in
// TestRegisterLeser_StichtageInDerRichtigenFolge.
//
// Die Validatoren-Belohnung (Gewichte aus der Kette statt aus
// registered_nodes) ist der zweite Teil von Schritt 3 und noch offen.

const (
	registerLeserAbUnix   int64 = math.MaxInt64
	erzeugerSchnittAbUnix int64 = math.MaxInt64
	// registerLeserVorlauf: so lange nach validatorRegisterAb frühestens.
	registerLeserVorlauf int64 = 7 * 86400
)

// Nur fuer Tests (0 = Konstante gilt).
var (
	registerLeserOverride   atomic.Int64
	erzeugerSchnittOverride atomic.Int64
)

func registerLeserAb() int64 {
	if o := registerLeserOverride.Load(); o != 0 {
		return o
	}
	return registerLeserAbUnix
}

func erzeugerSchnittAb() int64 {
	if o := erzeugerSchnittOverride.Load(); o != 0 {
		return o
	}
	return erzeugerSchnittAbUnix
}

// registerLeserAktiv: lesen Strafkonto und Leitung zu diesem Zeitpunkt aus
// dem Register?
func registerLeserAktiv(t int64) bool { return t >= registerLeserAb() }

// erzeugerSchnittAktiv: gilt fuer einen Block mit dieser Zeit die
// Erzeugerpruefung gegen das Register?
func erzeugerSchnittAktiv(t int64) bool { return t >= erzeugerSchnittAb() }

// validatorBetreiberBis: welcher Betreiber hielt den Schluessel signing zum
// Zeitpunkt bis? Die Bindung an signing mit dem spaetesten Zeitpunkt <= bis
// -- auch eine inzwischen ueberholte: gezahlt hat, wer den Schluessel zur Tat
// hielt, nicht wer ihn spaeter uebernahm. Teilen sich zwei diesen Zeitpunkt,
// keiner (umstritten). "" = keiner. q ist dieselbe Transaktion wie der
// Vermerk -- jeder Knoten liest denselben Kettenzustand.
//
// Grenze: das Register haelt je Betreiber nur die letzte Bindung. Hat der
// Betreiber selbst den Schluessel gewechselt, bevor die Strafe ankommt, steht
// seine alte Bindung nicht mehr da, und es gibt keine Strafe -- wie heute bei
// einem fehlenden Eintrag. Sperre und Vergehen haengen weiter an der
// Signieradresse.
func validatorBetreiberBis(q sqlExecutor, signing string, bis int64) (string, error) {
	rows, err := q.Query(`SELECT operator_wallet, bindung_ts FROM validator_register
		WHERE signing_address = $1 AND bindung_ts <= $2
		ORDER BY bindung_ts DESC, operator_wallet LIMIT 2`, strings.ToLower(signing), bis)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	type zeile struct {
		betreiber string
		zeit      int64
	}
	var z []zeile
	for rows.Next() {
		var r zeile
		if err := rows.Scan(&r.betreiber, &r.zeit); err != nil {
			return "", err
		}
		z = append(z, r)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(z) == 0 || (len(z) == 2 && z[0].zeit == z[1].zeit) {
		return "", nil
	}
	return z[0].betreiber, nil
}

// istMenschIn: chain_accounts.is_human in derselben Transaktion -- so sieht
// auch ein Mensch, der im selben Block registriert wurde, wie jeder andere
// Knoten aus.
func istMenschIn(q sqlExecutor, adresse string) (bool, error) {
	var mensch bool
	switch err := q.QueryRow(`SELECT is_human FROM chain_accounts WHERE lower(address) = $1`, strings.ToLower(adresse)).Scan(&mensch); {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	}
	return mensch, nil
}

// strafKontoAusRegister: das Strafkonto ab registerLeserAb -- der Betreiber,
// der den Schluessel zur Zeit der Erkennung hielt, und nur, wenn er ein
// registrierter Mensch ist. Kein Betreiber, umstritten oder kein Mensch:
// keine Geldstrafe (Sperre und Zaehler bleiben). Ein Lesefehler ist ein
// Fehler -- beim Nachspielen weist er den Block ab, er wird nie zu "keine
// Strafe".
func strafKontoAusRegister(q sqlExecutor, signer string, detectedAt int64) (string, error) {
	betreiber, err := validatorBetreiberBis(q, signer, detectedAt)
	if err != nil {
		return "", fmt.Errorf("Strafkonto aus dem Register: %w", err)
	}
	if betreiber == "" {
		fmt.Printf("[SLASHING] ⚠ kein eindeutiger Betreiber fuer %s im Register (bis %d) -- keine Geldstrafe\n", signer, detectedAt)
		return "", nil
	}
	mensch, err := istMenschIn(q, betreiber)
	if err != nil {
		return "", fmt.Errorf("Strafkonto aus dem Register: %w", err)
	}
	if !mensch {
		fmt.Printf("[SLASHING] ⚠ Betreiber %s von %s ist kein registrierter Mensch -- keine Geldstrafe\n", kurzAdresse(betreiber), signer)
		return "", nil
	}
	return betreiber, nil
}

// ------------------------------------------------------------ Erzeuger

// erzeugerRegisterGrenze: hoechstens so viele Betreiber liest der Knoten fuer
// die Erzeugerpruefung. Mehr ist ein Fehler (fail-closed), kein Abschneiden
// -- abgeschnitten saehe jeder Knoten eine andere Menge.
const erzeugerRegisterGrenze = 10000

// erzeugerStand: die Signieradressen, die laut Register erzeugen duerfen --
// nicht ueberholt, nicht umstritten, Betreiber ist Mensch.
type erzeugerStand struct {
	adressen map[string]bool
	fehler   error
	zeit     time.Time
}

// validatorErzeugerAusRegister liest die Menge. Unter keiner Sperre.
func (cs *ChainState) validatorErzeugerAusRegister(ctx context.Context) (map[string]bool, error) {
	if cs.db == nil {
		return nil, fmt.Errorf("keine Datenbank")
	}
	rows, err := cs.db.QueryContext(ctx, `SELECT r.signing_address, COALESCE(a.is_human, false)
		FROM validator_register r
		LEFT JOIN chain_accounts a ON lower(a.address) = r.operator_wallet
		WHERE NOT r.ueberholt
		  AND NOT EXISTS (SELECT 1 FROM validator_register o
		                  WHERE o.signing_address = r.signing_address AND o.operator_wallet <> r.operator_wallet AND NOT o.ueberholt)
		ORDER BY r.signing_address LIMIT $1`, erzeugerRegisterGrenze+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	n := 0
	for rows.Next() {
		var s string
		var mensch bool
		if err := rows.Scan(&s, &mensch); err != nil {
			return nil, err
		}
		n++
		if mensch {
			out[strings.ToLower(s)] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if n > erzeugerRegisterGrenze {
		return nil, fmt.Errorf("Register hat mehr als %d Bindungen", erzeugerRegisterGrenze)
	}
	return out, nil
}

// erzeugerRegisterAuffrischen: den Stand neu lesen. Nach jedem Block mit
// validator_bindung (replayTransactions), nach der eigenen Annahme und alle
// 30 s (StarteErzeugerRegister). Ein Fehler ersetzt den Stand -- die
// Pruefung schliesst dann ab, statt mit einem alten Stand weiterzumachen.
func (cs *ChainState) erzeugerRegisterAuffrischen() {
	if cs == nil || cs.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := cs.validatorErzeugerAusRegister(ctx)
	if err != nil {
		fmt.Printf("[VALIDATOR] Erzeuger aus dem Register nicht lesbar: %v\n", err)
	}
	cs.erzeugerRegister.Store(&erzeugerStand{adressen: m, fehler: err, zeit: time.Now()})
}

// erzeugerAuffrischenEinmal: der Hintergrund-Leser laeuft einmal je Prozess.
var erzeugerAuffrischenEinmal sync.Once

// StarteErzeugerRegister: liest den Stand sofort und dann alle 30 s.
func (dag *BlockDAG) StarteErzeugerRegister() {
	if dag == nil || dag.state == nil || dag.state.db == nil {
		return
	}
	erzeugerAuffrischenEinmal.Do(func() {
		dag.state.erzeugerRegisterAuffrischen()
		SafeGoroutine("erzeuger-register", func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for range t.C {
				dag.state.erzeugerRegisterAuffrischen()
			}
		})
	})
}

// erzeugerNachRegister: darf addr ab erzeugerSchnittAb Bloecke erzeugen?
// Ohne Datenbankzugriff (Aufrufer haelt dag.mu). Kein Stand oder ein
// Lesefehler: nein (fail-closed).
func (dag *BlockDAG) erzeugerNachRegister(addr string) bool {
	if dag.state == nil {
		return false
	}
	st := dag.state.erzeugerRegister.Load()
	if st == nil || st.fehler != nil || !st.adressen[addr] {
		return false
	}
	if len(dag.produzentenFest) > 0 {
		return dag.produzentenFest[addr] || (addr != "" && addr == dag.selfProposer)
	}
	return true
}

// erzeugerErlaubt: die Erzeugerpruefung in AddPeerBlock. Vor erzeugerSchnittAb
// wie bisher (authorizedValidators), danach das Register (Aufrufer haelt
// dag.mu).
func (dag *BlockDAG) erzeugerErlaubt(proposer string, blockZeit int64) bool {
	if erzeugerSchnittAktiv(blockZeit) {
		return dag.erzeugerNachRegister(proposer)
	}
	return dag.authorizedValidators[proposer]
}

// ErzeugerOhneBindung: Erzeuger aus AUTHORIZED_VALIDATORS, die das Register
// (noch) nicht traegt -- ab erzeugerSchnittAb erzeugten sie nicht mehr. Fuer
// /api/status: der Stichtag darf erst gesetzt werden, wenn die Liste leer ist.
// nil = der Stand ist nicht lesbar. Ohne dag.mu (produzentenFest wird nach dem
// Start nie geschrieben).
func (dag *BlockDAG) ErzeugerOhneBindung() []string {
	if dag == nil || dag.state == nil {
		return nil
	}
	st := dag.state.erzeugerRegister.Load()
	if st == nil || st.fehler != nil {
		return nil
	}
	out := []string{}
	for a := range dag.produzentenFest {
		if !st.adressen[a] {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// ------------------------------------------------------------ Leitung

// menschAusRegister: der Mensch hinter einem Signierschluessel fuer die
// Leitung ("ein Mensch, eine Stimme") ab registerLeserAb -- die nicht
// ueberholte, unumstrittene Bindung, und der Betreiber ist Mensch. 30 s
// gemerkt, hoechstens 1024 Eintraege.
func (dag *BlockDAG) menschAusRegister(signing string) string {
	if v, ok := dag.registerMenschen.Load(signing); ok {
		e := v.(registerMensch)
		if time.Since(e.zeit) < 30*time.Second {
			return e.mensch
		}
	}
	if dag.state == nil || dag.state.db == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	b, err := dag.state.validatorZuSignieradresseCtx(ctx, signing)
	if err != nil {
		return ""
	}
	if b != "" {
		mensch, err := istMenschIn(dag.state.db, b)
		if err != nil {
			return ""
		}
		if !mensch {
			b = ""
		}
	}
	if dag.registerMenschenZahl.Add(1) > 1024 {
		dag.registerMenschen.Range(func(k, _ interface{}) bool { dag.registerMenschen.Delete(k); return true })
		dag.registerMenschenZahl.Store(1)
	}
	dag.registerMenschen.Store(signing, registerMensch{mensch: b, zeit: time.Now()})
	return b
}

type registerMensch struct {
	mensch string
	zeit   time.Time
}

// blockHatValidatorBindung: aendert der Block das Register?
func blockHatValidatorBindung(b *Block) bool {
	if b == nil {
		return false
	}
	for i := range b.Transactions {
		if b.Transactions[i].Type == "validator_bindung" {
			return true
		}
	}
	return false
}
