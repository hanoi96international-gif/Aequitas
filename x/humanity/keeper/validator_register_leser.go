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

	"github.com/lib/pq"
)

// Validator-Register, Schritt 3: die LESER (docs/VALIDATOR_REGISTER_KONSENS.md).
//
// Schritt 1 und 2 haben das Register auf die Kette gebracht. Gelesen wurde es
// bisher von niemandem: Strafkonto, Erzeugerliste und Leitung hingen weiter
// an Tabellen, die jeder Knoten selbst fuehrt (registered_nodes,
// validator_keys, Abgleich unter Peers). Hier stellen sie um -- jede an einem
// Zeitpunkt, der in der Kette steht (DetectedAt, Blockzeit), und vor dem
// Stichtag byte-gleich wie bisher.
//
// ZWEI STICHTAGE, BEIDE PLATZHALTER
//
//   - registerLeserAb: Strafkonto (slashing.go, strafKonto), Leitung
//     (validatorMenschVon) und -- mit geschlossener Liste -- der Abgleich
//     unter Peers (syncValidatorsFromPeer nimmt nichts mehr auf).
//   - erzeugerSchnittAb: wer Bloecke erzeugen darf. Mit AUTHORIZED_VALIDATORS
//     (geschlossen) die SCHNITTMENGE aus Liste und Register -- das Register
//     kann Erzeuger nur wegnehmen, nie hinzufuegen. Ohne Liste (offen) das
//     Register allein, statt des Abgleichs; im offenen Betrieb endet auch der
//     Abgleich erst hier (sonst nahme ein Knoten einen neuen Erzeuger an, den
//     die anderen nie erfuehren). Eigener Tag, weil er erst gesetzt werden
//     darf, wenn jeder Erzeuger der Liste gebunden ist (/api/status,
//     erzeuger_ohne_bindung) -- sonst schlosse sich C1 selbst aus.
//
// Beide mindestens eine Woche nach validatorRegisterAb: bestehende Betreiber
// binden vorher neu (Schritt 2). Erzwungen in
// TestRegisterLeser_StichtageInDerRichtigenFolge.
//
// DER VERLAUF (Sicherheitsdurchgang #303)
//
// Das Register haelt je Betreiber nur die LETZTE Bindung. Beide Leser
// brauchen aber, was ZU EINER ZEIT galt:
//   - das Strafkonto, wer den Schluessel zur Tat hielt -- mit dem Register
//     allein zahlte nach "V bindet K, B uebernimmt K, B zieht weiter" V fuer
//     B's Tat;
//   - die Erzeugerpruefung, ob ein Schluessel zur Zeit des BLOCKS erzeugen
//     durfte -- mit dem jeweils aktuellen Register wiesen Knoten, die eine
//     Uebergabe schon nachgespielt hatten, die Bloecke des alten Schluessels
//     ab, waehrend andere sie annahmen, und das Netz zerfiel dauerhaft.
// Darum der Verlauf (validator_verlauf, validator_register.go): jede gueltige
// Bindung, die je in einem Block stand. Aus ihm die Zeitraeume
// (bindungsIntervalle): eine Bindung gilt ab ihrem Zeitpunkt bis zur
// naechsten Bindung ihres Betreibers oder bis ein anderer Betreiber den
// Schluessel spaeter bindet.
//
// FRIST: Fuer die Erzeugerpruefung gilt ein Zeitraum um erzeugerFrist
// verschoben -- eine neue Bindung wirkt erst zwei Stunden nach ihrem
// Zeitpunkt, eine beendete noch zwei Stunden danach. Der Zeitpunkt liegt
// hoechstens eine Stunde vor dem Block, der die Bindung traegt
// (nachweisHoechstensAlt); jeder Knoten hat also mindestens eine Stunde, den
// Block nachzuspielen, bevor sie auf irgendeinen Block wirkt. Danach urteilen
// alle ueber denselben Block gleich, egal wann sie ihn sehen: ein Block, der
// unter dem alten Stand gilt und unter dem neuen nicht, kann nur entstehen,
// wenn sein Erzeuger die Bindung eine Stunde lang nicht nachgespielt hat.
// Eine Uebergabe ist damit nahtlos.
//
// Die Validatoren-Belohnung (Gewichte aus der Kette statt aus
// registered_nodes) ist der zweite Teil von Schritt 3.

const (
	registerLeserAbUnix   int64 = math.MaxInt64
	erzeugerSchnittAbUnix int64 = math.MaxInt64
	// registerLeserVorlauf: so lange nach validatorRegisterAb frühestens.
	registerLeserVorlauf int64 = 7 * 86400
	// erzeugerFrist: so lange nach ihrem Zeitpunkt wirkt eine Bindung (oder
	// ihr Ende) auf die Erzeugerpruefung. Der Zeitpunkt darf bis zu
	// nachweisHoechstensAlt vor dem tragenden Block liegen -- erst danach
	// bleibt jedem Knoten eine Stunde, den Block nachzuspielen.
	erzeugerFrist int64 = nachweisHoechstensAlt + 3600
	// verlaufGrenze: so viele Verlaufszeilen liest ein Leser hoechstens;
	// mehr ist ein Fehler, kein Abschneiden.
	verlaufGrenze = 100_000
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
// Erzeugerpruefung gegen das Register? Mit der Rueck-Toleranz: ein Block,
// der um bis zu zwei Minuten zurueckdatiert ist, entgeht ihr dadurch nicht
// (wie syncGeschichte).
func erzeugerSchnittAktiv(t int64) bool { return t+zeitstempelRueckToleranz >= erzeugerSchnittAb() }

// ------------------------------------------------------------ Verlauf

// bindungsZeile: eine Bindung aus dem Verlauf.
type bindungsZeile struct {
	betreiber string
	signing   string
	zeit      int64
}

// bindungsIntervall: [von, bis) -- solange galt eine Bindung. bis =
// math.MaxInt64: gilt noch.
type bindungsIntervall struct {
	betreiber string
	signing   string
	von       int64
	bis       int64
}

// bindungsIntervalle: die Zeitraeume aller Bindungen. Eine Bindung endet mit
// der naechsten ihres Betreibers (nach Zeitpunkt, dann Signieradresse --
// wie validatorNeuer) oder wenn ein ANDERER Betreiber denselben Schluessel
// spaeter bindet (dorthin hat der Schluessel zuletzt zugestimmt). Teilen
// sich zwei Betreiber denselben Zeitpunkt fuer einen Schluessel, gelten
// beide zugleich -- umstritten, erzeugerFenster liefert dann keinen. Haengt
// nur an der Menge der Zeilen, nicht an ihrer Reihenfolge.
func bindungsIntervalle(zeilen []bindungsZeile) []bindungsIntervall {
	jeBetreiber := map[string][]bindungsZeile{}
	jeSchluessel := map[string][]bindungsZeile{}
	for _, z := range zeilen {
		jeBetreiber[z.betreiber] = append(jeBetreiber[z.betreiber], z)
		jeSchluessel[z.signing] = append(jeSchluessel[z.signing], z)
	}
	for _, l := range jeBetreiber {
		sort.Slice(l, func(i, j int) bool {
			if l[i].zeit != l[j].zeit {
				return l[i].zeit < l[j].zeit
			}
			return l[i].signing < l[j].signing
		})
	}
	var out []bindungsIntervall
	for _, l := range jeBetreiber {
		for i, z := range l {
			bis := int64(math.MaxInt64)
			if i+1 < len(l) {
				bis = l[i+1].zeit
			}
			for _, a := range jeSchluessel[z.signing] {
				if a.betreiber != z.betreiber && a.zeit > z.zeit && a.zeit < bis {
					bis = a.zeit
				}
			}
			if bis > z.zeit {
				out = append(out, bindungsIntervall{z.betreiber, z.signing, z.zeit, bis})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].signing != out[j].signing {
			return out[i].signing < out[j].signing
		}
		if out[i].von != out[j].von {
			return out[i].von < out[j].von
		}
		return out[i].betreiber < out[j].betreiber
	})
	return out
}

// verlaufLesen: Zeilen aus dem Verlauf. signings = nil: alle; sonst die
// Zeilen dieser Schluessel UND alle Zeilen ihrer Betreiber (fuer das Ende
// einer Bindung durch die naechste des Betreibers). q: Transaktion oder
// Verbindung. Mehr als verlaufGrenze: Fehler.
func verlaufLesen(q sqlExecutor, signings []string) ([]bindungsZeile, error) {
	var rows *sql.Rows
	var err error
	if signings == nil {
		rows, err = q.Query(`SELECT operator_wallet, signing_address, bindung_ts FROM validator_verlauf LIMIT $1`, verlaufGrenze+1)
	} else {
		rows, err = q.Query(`SELECT operator_wallet, signing_address, bindung_ts FROM validator_verlauf
			WHERE signing_address = ANY($1)
			   OR operator_wallet IN (SELECT operator_wallet FROM validator_verlauf WHERE signing_address = ANY($1))
			LIMIT $2`, pq.Array(signings), verlaufGrenze+1)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bindungsZeile
	for rows.Next() {
		var z bindungsZeile
		if err := rows.Scan(&z.betreiber, &z.signing, &z.zeit); err != nil {
			return nil, err
		}
		z.betreiber, z.signing = strings.ToLower(z.betreiber), strings.ToLower(z.signing)
		out = append(out, z)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > verlaufGrenze {
		return nil, fmt.Errorf("Verlauf hat mehr als %d Zeilen", verlaufGrenze)
	}
	return out, nil
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

// ------------------------------------------------------------ Strafkonto

// strafKontoAusRegister: das Strafkonto ab registerLeserAb.
//
//   - Es zahlt, wer den Schluessel zuletzt VOR der Tat (DetectedAt) gebunden
//     hat -- auch wenn er danach selbst einen neuen Schluessel gebunden hat:
//     den alten kennt er weiter, und in der Frist darf der alte noch
//     erzeugen. Nur wenn das ein registrierter Mensch ist.
//   - Hat NACH der Tat ein ANDERER Betreiber den Schluessel gebunden, zahlt
//     keiner: der spaetere Halter besitzt den Schluessel (er hat ihm
//     zugestimmt) und haette den Beweis mit beliebigem Zeitpunkt selbst
//     unterschreiben und so dem frueheren anhaengen koennen. Sperre und
//     Zaehler bleiben -- sie haengen am Schluessel. Wer seinen Schluessel
//     abgibt, entgeht damit der Geldstrafe; wer ihn nur wechselt (eigene
//     neue Bindung), nicht.
//   - Kein Halter, oder zwei Betreiber mit demselben letzten Zeitpunkt
//     (umstritten): keine Geldstrafe.
//   - Ein Lesefehler ist ein Fehler -- beim Nachspielen weist er den Block
//     ab, er wird nie zu "keine Strafe".
//
// Gelesen werden die Zeilen des Schluessels selbst (nicht die Zeitraeume):
// so zaehlt auch eine Bindung, deren Zeitraum leer ist, weil ihr Betreiber
// im selben Augenblick einen anderen Schluessel gebunden hat.
//
// Bleibt von der Reihenfolge abhaengig: eine Uebergabe in einem
// Geschwisterblock der Strafe (wie "Mensch im Geschwisterblock" bei den
// Bindungen) -- ein ehrlicher Annehmender legt beides in eine Linie.
func strafKontoAusRegister(q sqlExecutor, signer string, tat int64) (string, error) {
	signer = strings.ToLower(signer)
	zeilen, err := verlaufLesen(q, []string{signer})
	if err != nil {
		return "", fmt.Errorf("Strafkonto aus dem Verlauf: %w", err)
	}
	var letzte int64
	halter := map[string]bool{}
	for _, z := range zeilen {
		if z.signing != signer || z.zeit > tat {
			continue
		}
		if len(halter) == 0 || z.zeit > letzte {
			letzte, halter = z.zeit, map[string]bool{}
		}
		if z.zeit == letzte {
			halter[z.betreiber] = true
		}
	}
	if len(halter) != 1 {
		fmt.Printf("[SLASHING] ⚠ kein eindeutiger Halter fuer %s zur Tat (%d) -- keine Geldstrafe\n", signer, tat)
		return "", nil
	}
	var wer string
	for b := range halter {
		wer = b
	}
	for _, z := range zeilen {
		if z.signing == signer && z.betreiber != wer && z.zeit > tat {
			fmt.Printf("[SLASHING] ⚠ %s wurde nach der Tat von %s gebunden -- keine Geldstrafe fuer %s\n",
				signer, kurzAdresse(z.betreiber), kurzAdresse(wer))
			return "", nil
		}
	}
	mensch, err := istMenschIn(q, wer)
	if err != nil {
		return "", fmt.Errorf("Strafkonto aus dem Verlauf: %w", err)
	}
	if !mensch {
		fmt.Printf("[SLASHING] ⚠ Halter %s von %s ist kein registrierter Mensch -- keine Geldstrafe\n", kurzAdresse(wer), signer)
		return "", nil
	}
	return wer, nil
}

// ------------------------------------------------------------ Erzeuger

// zeitfenster: [von, bis) in Blockzeit.
type zeitfenster struct {
	betreiber string
	von, bis  int64
}

// erzeugerStand: was die Erzeugerpruefung und die Leitung brauchen, im
// Speicher (beide laufen unter Sperren, ohne Datenbank).
type erzeugerStand struct {
	// fenster: je Signierschluessel die Zeiten, zu denen er erzeugen darf --
	// die Bindungszeitraeume menschlicher Betreiber, um erzeugerFrist
	// verschoben. Nach einem Lesefehler die des letzten gelesenen Stands.
	fenster map[string][]zeitfenster
	// fehler: der letzte Lesefehler; die Erzeugerpruefung schliesst dann ab.
	fehler error
	zeit   time.Time
}

// erzeugerFenster: ist addr zur Blockzeit t in genau einem Fenster? (Zwei:
// umstritten.)
func (st *erzeugerStand) erzeugerFenster(addr string, t int64) string {
	halter := ""
	for _, f := range st.fenster[addr] {
		if f.von <= t && t < f.bis {
			if halter != "" {
				return ""
			}
			halter = f.betreiber
		}
	}
	return halter
}

// erzeugerAusVerlauf: die Fenster aus dem Verlauf. fest = nil: offen, alle
// Schluessel; sonst nur diese (geschlossene Liste -- keine Grenze fuer das
// ganze Netz, die jeder Mensch mit Bindungen fuellen koennte).
func (cs *ChainState) erzeugerAusVerlauf(ctx context.Context, fest []string) (map[string][]zeitfenster, error) {
	if cs.db == nil {
		return nil, fmt.Errorf("keine Datenbank")
	}
	q := dbMitKontext{ctx: ctx, db: cs.db}
	if fest != nil && len(fest) == 0 {
		return map[string][]zeitfenster{}, nil
	}
	zeilen, err := verlaufLesen(q, fest)
	if err != nil {
		return nil, err
	}
	betreiber := map[string]bool{}
	for _, z := range zeilen {
		betreiber[z.betreiber] = true
	}
	menschen := map[string]bool{}
	if len(betreiber) > 0 {
		liste := make([]string, 0, len(betreiber))
		for b := range betreiber {
			liste = append(liste, b)
		}
		rows, err := q.Query(`SELECT lower(address) FROM chain_accounts WHERE lower(address) = ANY($1) AND is_human`, pq.Array(liste))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var a string
			if err := rows.Scan(&a); err != nil {
				rows.Close()
				return nil, err
			}
			menschen[a] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	var gesucht map[string]bool
	if fest != nil {
		gesucht = map[string]bool{}
		for _, a := range fest {
			gesucht[strings.ToLower(a)] = true
		}
	}
	return fensterAusIntervallen(bindungsIntervalle(zeilen), menschen, gesucht), nil
}

// fensterAusIntervallen: die Zeitraeume menschlicher Betreiber, um
// erzeugerFrist verschoben. gesucht = nil: alle Schluessel.
func fensterAusIntervallen(iv []bindungsIntervall, menschen, gesucht map[string]bool) map[string][]zeitfenster {
	out := map[string][]zeitfenster{}
	for _, i := range iv {
		if !menschen[i.betreiber] || (gesucht != nil && !gesucht[i.signing]) {
			continue
		}
		bis := i.bis
		if bis != math.MaxInt64 {
			bis += erzeugerFrist
		}
		out[i.signing] = append(out[i.signing], zeitfenster{betreiber: i.betreiber, von: i.von + erzeugerFrist, bis: bis})
	}
	return out
}

// dbMitKontext: *sql.DB mit Zeitgrenze als sqlExecutor -- die Leser ausserhalb
// einer Transaktion sollen nicht ohne Grenze warten (dbExecCtx liest dafuer
// cs.activeTx, das nur unter cs.mu gilt).
type dbMitKontext struct {
	ctx context.Context
	db  *sql.DB
}

func (d dbMitKontext) Exec(q string, a ...interface{}) (sql.Result, error) {
	return d.db.ExecContext(d.ctx, q, a...)
}
func (d dbMitKontext) QueryRow(q string, a ...interface{}) *sql.Row {
	return d.db.QueryRowContext(d.ctx, q, a...)
}
func (d dbMitKontext) Query(q string, a ...interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(d.ctx, q, a...)
}

// erzeugerRegisterAuffrischen: den Stand neu lesen -- nach jedem Block, der
// eine Bindung angewandt hat (replayTransactions), nach der eigenen Annahme,
// nach einem Snapshot-Import und alle 30 s (StarteErzeugerRegister). Ein
// Fehler markiert den Stand: die Erzeugerpruefung schliesst dann ab, statt
// mit einem alten Stand weiterzumachen. Die Fenster des letzten gelesenen
// Stands bleiben fuer die Leitung stehen (menschAusRegister) -- sie soll
// zwischen zwei Abfragen nicht mal den alten, mal keinen Menschen sehen.
func (cs *ChainState) erzeugerRegisterAuffrischen() {
	if cs == nil || cs.db == nil {
		return
	}
	var fest []string
	if p := cs.erzeugerFest.Load(); p != nil {
		fest = *p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := cs.erzeugerAusVerlauf(ctx, fest)
	if err != nil {
		fmt.Printf("[VALIDATOR] Erzeuger aus dem Verlauf nicht lesbar: %v\n", err)
		m = nil
		if alt := cs.erzeugerRegister.Load(); alt != nil {
			m = alt.fenster
		}
	}
	cs.erzeugerRegister.Store(&erzeugerStand{fenster: m, fehler: err, zeit: time.Now()})
}

// erzeugerAuffrischenEinmal: der Hintergrund-Leser laeuft einmal je Prozess.
var erzeugerAuffrischenEinmal sync.Once

// StarteErzeugerRegister: die geschlossene Liste merken, den Stand sofort
// lesen und danach alle 30 s -- solange einer der Stichtage keinen Tag mehr
// entfernt ist (vorher braucht ihn niemand). Vor dem HTTP-Sync starten.
func (dag *BlockDAG) StarteErzeugerRegister() {
	if dag == nil || dag.state == nil || dag.state.db == nil {
		return
	}
	erzeugerAuffrischenEinmal.Do(func() {
		if fest := dag.erzeugerFestListe(); fest != nil {
			dag.state.erzeugerFest.Store(&fest)
		}
		bald := func() bool {
			grenze := nowUnix() + 86400
			return registerLeserAb() <= grenze || erzeugerSchnittAb() <= grenze
		}
		if bald() {
			if len(dag.produzentenFest) == 0 {
				// Offen liest der Stand den ganzen Verlauf, hoechstens
				// verlaufGrenze Zeilen -- jeder Mensch kann ihn fuellen, und
				// darueber schliesst die Erzeugerpruefung fuer alle ab.
				// Fuer den Stichtag ist nur der geschlossene Betrieb
				// freigegeben (docs/VALIDATOR_REGISTER_KONSENS.md).
				fmt.Printf("[VALIDATOR] ⚠ Erzeugerpruefung aus dem Register OHNE AUTHORIZED_VALIDATORS -- nicht freigegeben, Grenze %d Verlaufszeilen\n", verlaufGrenze)
			}
			dag.state.erzeugerRegisterAuffrischen()
		}
		SafeGoroutine("erzeuger-register", func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for range t.C {
				if bald() {
					SafeCall("erzeuger-register-auffrischen", dag.state.erzeugerRegisterAuffrischen)
				}
			}
		})
	})
}

// erzeugerFestListe: die Schluessel, die der Stand liest, wenn die Liste
// geschlossen ist -- AUTHORIZED_VALIDATORS und der eigene. nil = offen.
func (dag *BlockDAG) erzeugerFestListe() []string {
	if len(dag.produzentenFest) == 0 {
		return nil
	}
	fest := make([]string, 0, len(dag.produzentenFest)+1)
	for a := range dag.produzentenFest {
		fest = append(fest, a)
	}
	if dag.selfProposer != "" && !dag.produzentenFest[dag.selfProposer] {
		fest = append(fest, dag.selfProposer)
	}
	sort.Strings(fest)
	return fest
}

// erzeugerNachRegister: darf addr zur Blockzeit t Bloecke erzeugen? Ohne
// Datenbankzugriff (Aufrufer haelt dag.mu). Kein Stand oder ein Lesefehler:
// nein (fail-closed).
func (dag *BlockDAG) erzeugerNachRegister(addr string, t int64) bool {
	if dag.state == nil {
		return false
	}
	st := dag.state.erzeugerRegister.Load()
	if st == nil || st.fehler != nil || st.erzeugerFenster(addr, t) == "" {
		return false
	}
	if len(dag.produzentenFest) > 0 {
		return dag.produzentenFest[addr] || (addr != "" && addr == dag.selfProposer)
	}
	return true
}

// erzeugerErlaubt: die Erzeugerpruefung in AddPeerBlock. Vor erzeugerSchnittAb
// wie bisher (authorizedValidators), danach das Register zur Zeit des
// Blocks (Aufrufer haelt dag.mu).
func (dag *BlockDAG) erzeugerErlaubt(proposer string, blockZeit int64) bool {
	if erzeugerSchnittAktiv(blockZeit) {
		return dag.erzeugerNachRegister(proposer, blockZeit)
	}
	return dag.authorizedValidators[proposer]
}

// abgleichBeendet: nimmt der Abgleich unter Peers nichts mehr auf? Mit
// geschlossener Liste ab registerLeserAb (die Liste entscheidet, nicht der
// Abgleich). Offen erst ab erzeugerSchnittAb -- bis dahin entscheidet der
// Abgleich, wer erzeugt, und ohne ihn nahme nur der Knoten, bei dem sich
// ein neuer Erzeuger eingetragen hat, dessen Bloecke an.
func (dag *BlockDAG) abgleichBeendet(jetzt int64) bool {
	if len(dag.produzentenFest) > 0 {
		return registerLeserAktiv(jetzt)
	}
	return erzeugerSchnittAktiv(jetzt)
}

// ErzeugerOhneBindung: Erzeuger aus AUTHORIZED_VALIDATORS, die jetzt laut
// Register (mit Frist) NICHT erzeugen duerften -- ab erzeugerSchnittAb
// erzeugten sie nicht mehr. Fuer /api/status: der Stichtag darf erst
// gesetzt werden, wenn die Liste leer ist. nil = offen (keine Liste, die
// Antwort hiesse nichts) oder der Stand ist nicht lesbar. Ohne dag.mu
// (produzentenFest wird nach dem Start nie geschrieben).
func (dag *BlockDAG) ErzeugerOhneBindung() []string {
	if dag == nil || dag.state == nil || len(dag.produzentenFest) == 0 {
		return nil
	}
	st := dag.state.erzeugerRegister.Load()
	if st == nil || st.fehler != nil {
		return nil
	}
	jetzt := nowUnix()
	out := []string{}
	for a := range dag.produzentenFest {
		if st.erzeugerFenster(a, jetzt) == "" {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// ------------------------------------------------------------ Leitung

// menschAusRegister: der Mensch hinter einem Signierschluessel fuer die
// Leitung ("ein Mensch, eine Stimme") ab registerLeserAb -- aus dem Stand im
// Speicher (die Leitung fragt unter ihrer Sperre; keine Datenbank). Der
// Betreiber, dessen Fenster jetzt gilt (dieselbe Frist wie beim Erzeugen).
// Nach einem Lesefehler aus dem letzten gelesenen Stand: so sieht die
// Leitung fuer ein Mitglied nicht ploetzlich keinen Menschen mehr und
// nimmt einen zweiten Schluessel desselben Menschen auf (vorher: je
// Schluessel eine eigene Abfrage, und ein Fehler bei einem Mitglied liess
// genau das zu). Noch nie gelesen, keiner oder umstritten: "" -- die
// Leitung nimmt ihn dann nicht auf.
func (dag *BlockDAG) menschAusRegister(signing string) string {
	if dag.state == nil {
		return ""
	}
	st := dag.state.erzeugerRegister.Load()
	if st == nil {
		return ""
	}
	return st.erzeugerFenster(strings.ToLower(signing), nowUnix())
}
