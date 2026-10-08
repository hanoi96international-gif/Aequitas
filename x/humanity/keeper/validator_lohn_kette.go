package keeper

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
// Bloecke, die so alt sind, hat jeder Knoten, der dem Netz folgt; ganz
// frische koennten bei Erzeuger und Nachspielendem verschieden angekommen
// sein.
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
//   - Das Komitee (getEpochCommittee) kommt ab erzeugerSchnittAb aus
//     derselben Menge (komiteeKandidaten in block.go).

const (
	// anwesenheitRand: so lange vor der Runde endet das gezaehlte Fenster.
	anwesenheitRand int64 = 15 * 60
	// anwesenheitSchluesselGrenze: hoechstens so viele verschiedene
	// Erzeugerschluessel im Fenster. Mehr ist ein Fehler, kein Abschneiden.
	anwesenheitSchluesselGrenze = 1000
)

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

// validatorAnwesenheitAusKetteLocked: je menschlichem Betreiber die Zahl der
// Minuten im Fenster der Runde, in denen ein Schluessel, den er zur Blockzeit
// hielt, einen Block gebaut hat. Sortiert nach Wallet; Summe der Minuten.
// Unter cs.mu, in der Transaktion von ctx (Erzeuger:
// runAtomicDistributionWithOutbox, Nachspielender: replayTransactions).
func (cs *ChainState) validatorAnwesenheitAusKetteLocked(ctx context.Context, rundeAt int64) ([]betreiberMinuten, int64, error) {
	if cs.db == nil {
		return nil, 0, fmt.Errorf("keine Datenbank")
	}
	q := cs.dbExecCtx(ctx)
	seit, bis := anwesenheitsFenster(rundeAt)
	// Je Schluessel und Minute die frueheste Blockzeit -- sie entscheidet,
	// wem die Minute gehoert.
	rows, err := q.Query(`SELECT lower(proposer), timestamp / 60, MIN(timestamp) FROM chain_blocks
		WHERE timestamp >= $1 AND timestamp < $2
		GROUP BY 1, 2`, seit, bis)
	if err != nil {
		return nil, 0, fmt.Errorf("Bloecke des Fensters: %w", err)
	}
	type schluesselMinute struct {
		signing string
		minute  int64
		zeit    int64
	}
	var bloecke []schluesselMinute
	schluessel := map[string]bool{}
	for rows.Next() {
		var s schluesselMinute
		if err := rows.Scan(&s.signing, &s.minute, &s.zeit); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("Bloecke des Fensters: %w", err)
		}
		if !schluessel[s.signing] && len(schluessel) >= anwesenheitSchluesselGrenze {
			rows.Close()
			return nil, 0, fmt.Errorf("mehr als %d Erzeugerschluessel im Fenster", anwesenheitSchluesselGrenze)
		}
		schluessel[s.signing] = true
		bloecke = append(bloecke, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("Bloecke des Fensters: %w", err)
	}
	if len(bloecke) == 0 {
		return nil, 0, nil
	}
	liste := make([]string, 0, len(schluessel))
	for s := range schluessel {
		liste = append(liste, s)
	}
	sort.Strings(liste)
	zeilen, err := verlaufLesen(q, liste)
	if err != nil {
		return nil, 0, fmt.Errorf("Verlauf: %w", err)
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
	for _, s := range bloecke {
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
	return out, summe, nil
}

// anwesenheitAbschnitt: das Fenster in Abschnitte dieser Laenge; jeder muss
// Bloecke haben, damit ein Knoten die Runde nachrechnet.
const anwesenheitAbschnitt int64 = 10 * 60

// validatorFensterVollstaendigLocked: hat dieser Knoten Bloecke in jedem
// Abschnitt des Fensters? Jeder Validator baut in jedem Takt einen Block --
// ein leerer Abschnitt heisst: dieser Knoten hat ein Stueck nicht (frisch
// aus einem Snapshot, neu aufgesetzt mit Luecke), oder die Kette stand; dann
// sehen es alle gleich. Ohne Vollstaendigkeit rechnet er nicht nach.
func (cs *ChainState) validatorFensterVollstaendigLocked(ctx context.Context, rundeAt int64) (bool, error) {
	seit, bis := anwesenheitsFenster(rundeAt)
	var abschnitte int64
	if err := cs.dbExecCtx(ctx).QueryRow(`SELECT COUNT(DISTINCT (timestamp - $1) / $3) FROM chain_blocks
		WHERE timestamp >= $1 AND timestamp < $2`, seit, bis, anwesenheitAbschnitt).Scan(&abschnitte); err != nil {
		return false, err
	}
	return abschnitte >= (bis-seit)/anwesenheitAbschnitt, nil
}

// ------------------------------------------------------------ Nachrechnen

// validatorRundePruefung: Stand der laufenden Validatoren-Runde beim
// Nachspielen (wie lpRundePruefung).
type validatorRundePruefung struct {
	unsicher bool
	aktiv    bool
	rundeAt  int64
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
	if !r.aktiv {
		unsicher := r.unsicher
		*r = validatorRundePruefung{aktiv: true, unsicher: unsicher, rundeAt: tx.DistributionAt, bedacht: make(map[string]int64)}
		if !unsicher {
			ctx := context.Background()
			voll, err := cs.validatorFensterVollstaendigLocked(ctx, tx.DistributionAt)
			if err == nil && !voll {
				r.unsicher = true
			} else {
				var betreiber []betreiberMinuten
				var alle int64
				if err == nil {
					betreiber, alle, err = cs.validatorAnwesenheitAusKetteLocked(ctx, tx.DistributionAt)
				}
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

// validatorGutschrift: die Transaktion einer Validatoren-Gutschrift -- aus der
// Kette mit der Rundenzeit, nach der jeder Knoten nachrechnet.
func validatorGutschrift(s DistributionShare) Transaction {
	return Transaction{Type: "validator_distribution", Wallet: s.Wallet, Amount: s.Amount,
		FromDemurrageLost: s.DemurrageLost, DistributionAt: s.RundeAt}
}
