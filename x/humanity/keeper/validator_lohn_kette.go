package keeper

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
)

// VALIDATOREN-BELOHNUNG AUS DER KETTE (Validator-Register Schritt 3, Teil 2;
// docs/VALIDATOR_REGISTER_KONSENS.md).
//
// Bisher (distributeValidatorsPoolLocked): Empfaenger sind die Betreiber aus
// registered_nodes, Gewicht die Minuten, in denen ein Schluessel aus
// registered_nodes einen Block gebaut hat. registered_nodes fuehrt jeder
// Knoten selbst -- der Erzeuger zahlte, wem er wollte, und kein anderer
// Knoten konnte mehr pruefen, als dass der Empfaenger ein Mensch ist.
//
// Jetzt: dieselbe Regel (gleicher Anteil je Minute Anwesenheit, mehr Bloecke
// in einer Minute zaehlen nicht mehr), aber jede Eingabe steht in der Kette:
//   - Die Bloecke stehen in chain_blocks.
//   - Wem ein Block gehoert, sagt der Verlauf der Bindungen
//     (validator_verlauf, in der StateRoot): der Betreiber, dessen
//     Erzeugerfenster den Schluessel zur Blockzeit traegt -- dieselben
//     Fenster wie bei der Erzeugerpruefung (erzeugerFrist, umstritten: keiner).
//   - Nur Menschen (chain_accounts.is_human).
// Jeder Knoten rechnet die Runde nach (nachrechnenValidatorLocked) --
// Voraussetzung, damit Validatoren ohne Liste zugelassen werden koennen.
//
// FENSTER. Gezaehlt werden die 24 Stunden bis anwesenheitRand vor der Runde.
//
// KEGEL (OFFEN 3). Gezaehlt wird nicht jeder Block, den ein Knoten im Fenster
// kennt, sondern nur der Vergangenheitskegel eines ANKERS: des nachgespielten
// Blocks mit dem hoechsten Blue-Score in [Runde-10 min, Runde]. Der Anker steht
// in jeder Gutschrift (Transaction.Anker). Der Kegel haengt nur an Hashes und
// Eltern-Verweisen -- nicht daran, welche Bloecke ein Knoten gerade gesehen
// hat (an der Finalitaetswand, frisch synchronisiert). Hat ein Knoten einen
// Block mehr oder weniger, rechnet er trotzdem gleich; fehlt ihm einer IM
// Kegel, rechnet er nicht nach (unsicher), statt falsch. Rote Bloecke zaehlen
// mit -- auch sie sind unterschriebene Anwesenheit. Den Anker waehlt der
// Erzeuger; er kann damit hoechstens die letzten 10 Minuten vor der Runde
// verschieben -- das Fenster endet schon 15 Minuten davor.
//
// SCHALTER. Erst, wenn das ganze Fenster nach erzeugerSchnittAb liegt: ab
// dann erzeugt nur, wer gebunden ist, und jeder Block hat einen Betreiber im
// Verlauf. Vorher zaehlten Bloecke ungebundener Schluessel nicht, und ihre
// Betreiber gingen leer aus. Kein eigener Stichtag.
//
// GRENZEN.
//   - Ein Knoten, dem Bloecke des Fensters fehlen (frisch aus einem Snapshot,
//     neu aufgesetzt mit Luecke: ein Abschnitt von 10 Minuten ohne Block)
//     rechnet nicht nach, sondern prueft nur, dass jeder Empfaenger ein
//     Mensch ist und keiner zweimal bekommt (unsicher, wie bei der LP-Runde
//     nach einem Neustart).
//   - Kann der Erzeuger die Anwesenheit nicht aus der Kette lesen (mehr als
//     anwesenheitSchluesselGrenze Schluessel, Verlauf zu gross), bleibt der
//     Validatoren-Topf stehen und geht in die naechste Runde -- die
//     Tagesrunde (Grundeinkommen) laeuft weiter.
//   - Hat ein Knoten einen Block des Fensters, den der Erzeuger nicht hat
//     (oder umgekehrt -- an der Finalitaetswand verschieden behandelt),
//     rechnet er anders und meldet eine Abweichung. Im strengen Modus wiese
//     er die Runde ab.
//   - Die Rundenzeit waehlt der Erzeuger, hoechstens 10 Minuten neben der
//     Blockzeit -- er verschiebt das Fenster damit um hoechstens so viel.
//   - Das Komitee kommt ab erzeugerSchnittAb aus derselben Menge
//     (komitee_register.go).

const (
	// anwesenheitRand: so lange vor der Runde endet das gezaehlte Fenster.
	anwesenheitRand int64 = 15 * 60
	// anwesenheitSchluesselGrenze: hoechstens so viele verschiedene
	// Erzeugerschluessel im Fenster. Mehr ist ein Fehler, kein Abschneiden.
	anwesenheitSchluesselGrenze = 1000
	// validatorAnkerHoechstensAlt: der Anker liegt hoechstens so weit vor
	// der Runde.
	validatorAnkerHoechstensAlt int64 = 600
	// anwesenheitKegelPuffer: so weit unter den Fensteranfang steigt der Kegel
	// noch ab -- ein Kind darf bis zu 120 s vor seinem Elternblock liegen.
	anwesenheitKegelPuffer int64 = 600
	// kegelStapel: so viele Hashes je Abfrage.
	kegelStapel = 2000
)

// anwesenheitKegelGrenze: so viele Bloecke liest die Kegelzaehlung hoechstens
// (ein Tag bei einem Block je Sekunde sind 86.400; mehrere Validatoren bauen
// je Takt mehrere). Variable nur fuer Tests.
var anwesenheitKegelGrenze = 400_000

// kegelVorratGrenze: so viele Zeilen haelt der Vorrat (Zeitraum plus
// Nachschlagen) hoechstens. Der Vorrat haengt davon ab, was DIESER Knoten
// sonst noch kennt (Seitenzweige, spaetere Bloecke) -- wird er zu gross,
// ist das kein Urteil ueber den Anker, sondern kegelLuecke.
func kegelVorratGrenze() int { return 2 * anwesenheitKegelGrenze }

// validatorLohnAusKette: wird die Runde zur Zeit rundeAt aus der Kette
// verteilt?
func validatorLohnAusKette(rundeAt int64) bool {
	if rundeAt <= 0 {
		return false
	}
	return erzeugerSchnittAktiv(sattAdd(rundeAt, -(anwesenheitsZeitraum + anwesenheitRand)))
}

// anwesenheitsFenster: [seit, bis) der Runde.
func anwesenheitsFenster(rundeAt int64) (seit, bis int64) {
	bis = rundeAt - anwesenheitRand
	return bis - anwesenheitsZeitraum, bis
}

// validatorAnteil: die eine Formel fuer Erzeuger und Nachspielende.
func validatorAnteil(minuten, alleMinuten int64, topf float64) float64 {
	return floor6(topf * float64(minuten) / float64(alleMinuten))
}

// betreiberMinuten: ein Empfaenger und seine Minuten, sortiert nach Wallet.
type betreiberMinuten struct {
	wallet  string
	minuten int64
}

// validatorAnkerWaehlen: der nachgespielte Block mit dem hoechsten Blue-Score
// in [rundeAt-10 min, rundeAt]; "" = keiner. Nur der Erzeuger waehlt; wer
// nachspielt, nimmt den Anker aus der Gutschrift.
func validatorAnkerWaehlen(q sqlExecutor, rundeAt int64) (string, error) {
	var h string
	err := q.QueryRow(`SELECT hash FROM chain_blocks
		WHERE timestamp <= $1 AND timestamp >= $2 AND COALESCE(replayed, true)
		ORDER BY blue_score DESC, hash ASC LIMIT 1`, rundeAt, rundeAt-validatorAnkerHoechstensAlt).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return h, err
}

// validatorAnkerZeit: die Zeit des Ankers; gefunden = false: dieser Knoten
// kennt ihn (noch) nicht als nachgespielten Block.
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
	// kegelLuecke: ein Block des Kegels fehlt in chain_blocks -- die
	// Geschichte ist nicht ganz da (frisch aus einem Snapshot) -- oder der
	// Vorrat dieses Knotens wurde zu gross. Kein Urteil ueber den Anker.
	kegelLuecke
	// kegelZuGross: mehr als anwesenheitKegelGrenze Bloecke.
	kegelZuGross
)

// bloeckeImKegel: die Bloecke im Vergangenheitskegel des Ankers (der Anker
// eingeschlossen) mit Zeit in [seit, bis). Abgestiegen wird bis
// seit - anwesenheitKegelPuffer; was darunter liegt, wird nicht weiter
// verfolgt. Haengt nur an Hashes -- jeder Knoten mit derselben Geschichte
// liest denselben Kegel.
//
// EINE Abfrage fuer den Zeitraum, der Abstieg im Speicher: Ebene fuer Ebene
// je eine Abfrage hiesse bei einer Kette mit meist einem Elternblock rund
// 86.400 Abfragen fuer einen Tag -- unter cs.mu. Eltern ausserhalb des
// Zeitraums (ein Kind darf vor seinem Elternblock liegen) werden einzeln
// nachgeschlagen. Fehlt ein Block ganz: kegelLuecke.
func bloeckeImKegel(q sqlExecutor, anker string, seit, bis int64) ([]kegelBlock, kegelStand, error) {
	untergrenze := seit - anwesenheitKegelPuffer
	ankerZeit, gefunden, err := validatorAnkerZeit(q, anker)
	if err != nil {
		return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
	}
	if !gefunden {
		return nil, kegelLuecke, nil
	}
	type eintrag struct {
		proposer string
		zeit     int64
		eltern   string
	}
	bloecke := map[string]eintrag{}
	lies := func(rows *sql.Rows) error {
		defer rows.Close()
		for rows.Next() {
			var h string
			var e eintrag
			if err := rows.Scan(&h, &e.proposer, &e.zeit, &e.eltern); err != nil {
				return err
			}
			bloecke[h] = e
			if len(bloecke) > kegelVorratGrenze() {
				return errKegelVorratZuGross
			}
		}
		return rows.Err()
	}
	rows, err := q.Query(`SELECT hash, proposer, timestamp, parent_hashes FROM chain_blocks
		WHERE timestamp >= $1 AND timestamp <= $2 LIMIT $3`, untergrenze, ankerZeit+anwesenheitKegelPuffer, kegelVorratGrenze()+1)
	if err != nil {
		return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
	}
	if err := lies(rows); err != nil {
		if errors.Is(err, errKegelVorratZuGross) {
			return nil, kegelLuecke, nil
		}
		return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
	}
	gesehen := map[string]bool{anker: true}
	offen := []string{anker}
	var out []kegelBlock
	for len(offen) > 0 {
		// Was nicht im Zeitraum lag: einzeln nachschlagen.
		var fehlt []string
		for _, h := range offen {
			if _, ok := bloecke[h]; !ok {
				fehlt = append(fehlt, h)
			}
		}
		for i := 0; i < len(fehlt); i += kegelStapel {
			j := i + kegelStapel
			if j > len(fehlt) {
				j = len(fehlt)
			}
			rows, err := q.Query(`SELECT hash, proposer, timestamp, parent_hashes FROM chain_blocks WHERE hash = ANY($1)`, pq.Array(fehlt[i:j]))
			if err != nil {
				return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
			}
			if err := lies(rows); err != nil {
				if errors.Is(err, errKegelVorratZuGross) {
					return nil, kegelLuecke, nil
				}
				return nil, kegelLuecke, fmt.Errorf("Kegel lesen: %w", err)
			}
		}
		var naechste []string
		for _, h := range offen {
			e, ok := bloecke[h]
			if !ok {
				return nil, kegelLuecke, nil // die Geschichte ist nicht ganz da
			}
			if e.zeit >= seit && e.zeit < bis {
				out = append(out, kegelBlock{proposer: strings.ToLower(e.proposer), zeit: e.zeit})
			}
			if e.zeit < untergrenze {
				continue
			}
			var ph []string
			if err := json.Unmarshal([]byte(e.eltern), &ph); err != nil {
				return nil, kegelLuecke, fmt.Errorf("Kegel: Eltern von %s nicht lesbar: %w", kurzAdresse(h), err)
			}
			for _, p := range ph {
				if !gesehen[p] {
					gesehen[p] = true
					naechste = append(naechste, p)
				}
			}
			if len(gesehen) > anwesenheitKegelGrenze {
				return nil, kegelZuGross, nil
			}
		}
		offen = naechste
	}
	return out, kegelVollstaendig, nil
}

var errKegelVorratZuGross = errors.New("Kegel-Vorrat zu gross")

// validatorAnwesenheitAusKetteLocked: je menschlichem Betreiber die Zahl der
// Minuten im Fenster der Runde, in denen ein Schluessel, den er zur Blockzeit
// hielt, einen Block IM KEGEL DES ANKERS gebaut hat. Sortiert nach Wallet;
// Summe der Minuten. Bei kegelLuecke oder kegelZuGross keine Minuten.
// Unter cs.mu, in der Transaktion von ctx (Erzeuger:
// runAtomicDistributionWithOutbox, Nachspielender: replayTransactions).
func (cs *ChainState) validatorAnwesenheitAusKetteLocked(ctx context.Context, rundeAt int64, anker string) ([]betreiberMinuten, int64, kegelStand, error) {
	if cs.db == nil {
		return nil, 0, kegelLuecke, fmt.Errorf("keine Datenbank")
	}
	q := cs.dbExecCtx(ctx)
	seit, bis := anwesenheitsFenster(rundeAt)
	kegel, stand, err := bloeckeImKegel(q, anker, seit, bis)
	if err != nil || stand != kegelVollstaendig {
		return nil, 0, stand, err
	}
	// Je Schluessel und Minute die frueheste Blockzeit -- sie entscheidet,
	// wem die Minute gehoert.
	type schluesselMinute struct {
		signing string
		minute  int64
		zeit    int64
	}
	type schluesselUndMinute struct {
		signing string
		minute  int64
	}
	frueheste := map[schluesselUndMinute]*schluesselMinute{}
	schluessel := map[string]bool{}
	for _, b := range kegel {
		if !schluessel[b.proposer] && len(schluessel) >= anwesenheitSchluesselGrenze {
			return nil, 0, kegelVollstaendig, fmt.Errorf("mehr als %d Erzeugerschluessel im Fenster", anwesenheitSchluesselGrenze)
		}
		schluessel[b.proposer] = true
		k := schluesselUndMinute{b.proposer, b.zeit / 60}
		if v := frueheste[k]; v == nil || b.zeit < v.zeit {
			frueheste[k] = &schluesselMinute{signing: b.proposer, minute: b.zeit / 60, zeit: b.zeit}
		}
	}
	if len(frueheste) == 0 {
		return nil, 0, kegelVollstaendig, nil
	}
	liste := make([]string, 0, len(schluessel))
	for s := range schluessel {
		liste = append(liste, s)
	}
	sort.Strings(liste)
	zeilen, err := verlaufLesen(q, liste)
	if err != nil {
		return nil, 0, kegelVollstaendig, fmt.Errorf("Verlauf: %w", err)
	}
	betreiber := map[string]bool{}
	for _, z := range zeilen {
		betreiber[z.betreiber] = true
	}
	namen := make([]string, 0, len(betreiber))
	for b := range betreiber {
		namen = append(namen, b)
	}
	cs.ensureAccountsLoadedCtx(ctx, namen)
	menschen := map[string]bool{}
	for _, b := range namen {
		if acc, ok := cs.accounts.Get(b); ok && acc.IsHuman {
			menschen[b] = true
		}
	}
	st := &erzeugerStand{fenster: fensterAusIntervallen(bindungsIntervalle(zeilen), menschen, schluessel)}
	minuten := map[string]map[int64]bool{}
	for _, s := range frueheste {
		wer := st.erzeugerFenster(s.signing, s.zeit)
		if wer == "" {
			continue
		}
		if minuten[wer] == nil {
			minuten[wer] = map[int64]bool{}
		}
		minuten[wer][s.minute] = true
	}
	out := make([]betreiberMinuten, 0, len(minuten))
	var summe int64
	for w, m := range minuten {
		out = append(out, betreiberMinuten{wallet: w, minuten: int64(len(m))})
		summe += int64(len(m))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].wallet < out[j].wallet })
	return out, summe, kegelVollstaendig, nil
}

// ------------------------------------------------------------ Nachrechnen

// validatorRundePruefung: Stand der laufenden Validatoren-Runde beim
// Nachspielen (wie lpRundePruefung).
type validatorRundePruefung struct {
	unsicher bool
	aktiv    bool
	rundeAt  int64
	anker    string
	// Nur wenn aktiv und nicht unsicher: Wallet -> erwarteter Betrag (Mikro).
	erwartet map[string]int64
	n        int64
	bedacht  map[string]int64
}

func (r validatorRundePruefung) zurueck() validatorRundePruefung {
	for k, nr := range r.bedacht {
		if nr > r.n {
			delete(r.bedacht, k)
		}
	}
	return r
}

// nachrechnenValidatorLocked: eine Validatoren-Gutschrift, VOR dem Anwenden.
func (cs *ChainState) nachrechnenValidatorLocked(tx *Transaction, blockZeit int64) error {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	// Was jeder Knoten immer pruefen kann: nur an Menschen.
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	if acc, ok := cs.accounts.Get(wallet); !ok || !acc.IsHuman {
		return nachrechnenAbweichung("validator_kein_mensch", blockZeit,
			"%s ist kein registrierter Mensch (%.6f AEQ)", kurzAdresse(wallet), tx.Amount)
	}
	// Ohne Rundenzeit: die alte Regel. Ab der Umstellung muss sie dabei sein.
	if tx.DistributionAt == 0 {
		if validatorLohnAusKette(blockZeit - 600) {
			return nachrechnenAbweichung("validator_ohne_runde", blockZeit,
				"%s: Gutschrift ohne Rundenzeit", kurzAdresse(wallet))
		}
		return nil
	}
	if d := tx.DistributionAt - blockZeit; d > 600 || d < -600 {
		return nachrechnenAbweichung("validator_runde", blockZeit,
			"distribution_at=%d, Blockzeit %d (Abstand %ds)", tx.DistributionAt, blockZeit, d)
	}
	if !validatorLohnAusKette(tx.DistributionAt) {
		return nachrechnenAbweichung("validator_runde", blockZeit,
			"Rundenzeit %d vor der Umstellung", tx.DistributionAt)
	}
	// Ab der Umstellung traegt jede Gutschrift ihren Anker (OFFEN 3).
	if tx.Anker == "" {
		return nachrechnenAbweichung("validator_ohne_anker", blockZeit,
			"%s: Gutschrift ohne Anker", kurzAdresse(wallet))
	}
	r := &cs.validatorRunde
	if r.aktiv && r.rundeAt != tx.DistributionAt {
		if err := nachrechnenAbweichung("validator_runde", blockZeit,
			"zwei Rundenzeiten in einer Runde (%d, %d)", r.rundeAt, tx.DistributionAt); err != nil {
			return err
		}
		if err := cs.nachrechnenValidatorAbschlussLocked(blockZeit); err != nil {
			return err
		}
	}
	if r.aktiv && r.anker != tx.Anker {
		return nachrechnenAbweichung("validator_anker", blockZeit,
			"zwei Anker in einer Runde (%s, %s)", kurzAdresse(r.anker), kurzAdresse(tx.Anker))
	}
	if !r.aktiv {
		unsicher := r.unsicher
		*r = validatorRundePruefung{aktiv: true, unsicher: unsicher, rundeAt: tx.DistributionAt, anker: tx.Anker, bedacht: make(map[string]int64)}
		if !unsicher {
			ctx := context.Background()
			q := cs.dbExecCtx(ctx)
			ankerZeit, kenntAnker, err := validatorAnkerZeit(q, tx.Anker)
			var betreiber []betreiberMinuten
			var alle int64
			stand := kegelVollstaendig
			switch {
			case err != nil:
			case !kenntAnker:
				// Diesem Knoten fehlt der Anker (noch nicht nachgespielt,
				// frisch synchronisiert): er rechnet nicht nach, statt zu raten.
				stand = kegelLuecke
			case ankerZeit > tx.DistributionAt || ankerZeit < tx.DistributionAt-validatorAnkerHoechstensAlt:
				return nachrechnenAbweichung("validator_anker", blockZeit,
					"Anker %s (Zeit %d) liegt nicht in [%d, %d]", kurzAdresse(tx.Anker), ankerZeit,
					tx.DistributionAt-validatorAnkerHoechstensAlt, tx.DistributionAt)
			default:
				betreiber, alle, stand, err = cs.validatorAnwesenheitAusKetteLocked(ctx, tx.DistributionAt, tx.Anker)
			}
			if err == nil && stand == kegelZuGross {
				// Der Erzeuger zahlt bei einem zu grossen Kegel nicht aus --
				// derselbe Anker ergibt bei jedem denselben Kegel.
				return nachrechnenAbweichung("validator_kegel", blockZeit,
					"Kegel des Ankers %s groesser als %d Bloecke", kurzAdresse(tx.Anker), anwesenheitKegelGrenze)
			}
			if err == nil && stand == kegelLuecke {
				r.unsicher = true
			} else {
				if err != nil {
					r.unsicher = true
					if aerr := nachrechnenAbweichung("validator_unlesbar", blockZeit, "%v", err); aerr != nil {
						return aerr
					}
				} else {
					topf := NewDecimalFromMicro(cs.topfMikroLocked(validatorsPoolAddr)).Float()
					r.erwartet = make(map[string]int64, len(betreiber))
					for _, b := range betreiber {
						if anteil := validatorAnteil(b.minuten, alle, topf); anteil > 0 {
							r.erwartet[b.wallet] = NewDecimal(anteil).Micro()
						}
					}
				}
			}
		}
	}
	r.n++
	var soll int64
	if !r.unsicher {
		var ok bool
		if soll, ok = r.erwartet[wallet]; !ok {
			return nachrechnenAbweichung("validator_kein_betreiber", blockZeit,
				"%s hat im Fenster keinen Block gebaut (%.6f AEQ)", kurzAdresse(wallet), tx.Amount)
		}
	}
	if _, schon := r.bedacht[wallet]; schon {
		if err := nachrechnenAbweichung("validator_doppelt", blockZeit,
			"%s bekommt in dieser Runde zum zweiten Mal", kurzAdresse(wallet)); err != nil {
			return err
		}
	} else {
		// Nur Betreiber aus der Erwartung (bzw. ohne Erwartung: Menschen)
		// kommen in die Menge -- sie ist durch den Zustand begrenzt.
		r.bedacht[wallet] = r.n
	}
	if r.unsicher {
		return nil
	}
	if d := NewDecimal(tx.Amount).Micro() - soll; d > 1 || d < -1 {
		return nachrechnenAbweichung("validator_anteil", blockZeit,
			"%s: %.6f AEQ, nachgerechnet %.6f", kurzAdresse(wallet), tx.Amount, NewDecimalFromMicro(soll).Float())
	}
	return nil
}

// nachrechnenValidatorAbschlussLocked: bei validator_distribution_pool_zero
// oder spaetestens der Rundenmarke.
func (cs *ChainState) nachrechnenValidatorAbschlussLocked(blockZeit int64) error {
	r := cs.validatorRunde
	cs.validatorRunde = validatorRundePruefung{}
	if !r.aktiv || r.unsicher || r.erwartet == nil {
		return nil
	}
	fehlt := 0
	for w := range r.erwartet {
		if _, ok := r.bedacht[w]; !ok {
			fehlt++
		}
	}
	if fehlt > 0 {
		return nachrechnenAbweichung("validator_empfaenger", blockZeit,
			"%d von %d Betreibern ohne Gutschrift", fehlt, len(r.erwartet))
	}
	return nil
}

// rundeAusKette: die Rundenzeit fuer die Gutschrift, nur wenn aus der Kette.
func rundeAusKette(ausKette bool, rundeAt int64) int64 {
	if ausKette {
		return rundeAt
	}
	return 0
}

// ankerAusKette: der Anker fuer die Gutschrift, nur wenn aus der Kette.
func ankerAusKette(ausKette bool, anker string) string {
	if ausKette {
		return anker
	}
	return ""
}

// validatorGutschrift: die Transaktion einer Validatoren-Gutschrift -- aus der
// Kette mit der Rundenzeit, nach der jeder Knoten nachrechnet.
func validatorGutschrift(s DistributionShare) Transaction {
	return Transaction{Type: "validator_distribution", Wallet: s.Wallet, Amount: s.Amount,
		FromDemurrageLost: s.DemurrageLost, DistributionAt: s.RundeAt, Anker: s.Anker}
}
