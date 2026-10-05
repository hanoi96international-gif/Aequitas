package keeper

// VALIDATOR-REGISTER AUF DER KETTE, SCHRITT 1 (05.10.2026, schlafend).
//
// # WARUM
//
// Wer Validator ist, steht heute in Tabellen, die jeder Knoten selbst fuehrt
// (validator_slots, validator_keys, registered_nodes) -- befuellt ueber
// /api/peers/register und den Abgleich zwischen Knoten. Die Bindung
// (Betreiber unterschreibt "Aequitas: authorize validator <adresse>") prueft
// jeder Knoten selbst, aber WELCHE Bindungen er kennt, haengt davon ab, wen
// er erfahren hat. Strafkonto, Validatoren-Belohnung und Komitee haengen
// daran; zwei Knoten koennen verschieden rechnen. Die Unterschrift traegt
// ausserdem keinen Zeitpunkt: eine alte Bindung laesst sich wieder einspielen.
// Siehe docs/VALIDATOR_REGISTER_KONSENS.md fuer den ganzen Weg.
//
// # WAS
//
// Eine Kettentransaktion validator_bindung:
//
//	Wallet         = der Betreiber (registrierter Mensch)
//	To             = die Signieradresse seines Knotens
//	Nachweis.Zeit  = Zeitpunkt der Bindung
//	Nachweis.Sig   = Unterschrift des Betreibers
//	Nachweis.Sig2  = Unterschrift des Signierschluessels
//
// Beide unterschreiben denselben Satz (validatorBindungNachricht, mit
// Chain-ID und Zeitpunkt). Die zweite Unterschrift ist noetig: sonst koennte
// ein Mensch die Signieradresse eines FREMDEN Knotens unter seinem Namen
// eintragen (Audit 2026-09-29, H1 -- dort fuer /api/validator-self-proof
// behoben).
//
// JEDER Knoten prueft beim Nachspielen selbst:
//
//   - Stichtag, Form (beide Adressen kanonisch, beide Unterschriften da),
//     Unterschriften und Zeitfenster (hoechstens eine Stunde alt, hoechstens
//     fuenf Minuten voraus; pruefeAuftragsNachweis) -- ein Verstoss macht den
//     Block ungueltig;
//   - der Betreiber ist Mensch, und die Bindung ist NEUER als seine bisherige
//     -- sonst wird die Transaktion uebersprungen (Zustandsablehnung).
//
// # REIHENFOLGE
//
// Geschwisterbloecke im DAG spielt jeder Knoten in der Reihenfolge nach, in
// der sie ankommen. Das Register darf davon nicht abhaengen -- sonst haetten
// zwei ehrliche Knoten verschiedene Validatoren. Deshalb:
//
//   - Je Betreiber gilt die Bindung mit dem GROESSTEN Paar (Zeitpunkt,
//     Signieradresse). Ein Maximum ist in jeder Reihenfolge dasselbe; der
//     Vergleich der Adresse entscheidet nur, wenn ein Betreiber zwei
//     Bindungen mit demselben Zeitpunkt unterschrieben hat.
//   - Ob eine Signieradresse schon einem anderen Betreiber gehoert, wird beim
//     Schreiben NICHT geprueft (das hinge von der Reihenfolge ab), sondern
//     beim Lesen entschieden (validatorZuSignieradresseCtx): die Adresse
//     gehoert der Bindung mit dem spaetesten Zeitpunkt -- dorthin hat der
//     Schluessel zuletzt zugestimmt. Teilen sich zwei den spaetesten, gehoert
//     sie keinem.
//   - Uebrig bleibt "Betreiber ist Mensch": steht seine Registrierung in
//     einem Geschwisterblock der Bindung, entscheidet die Reihenfolge. Ein
//     ehrlicher Annehmender erzeugt das nie (sein Block verweist auf den
//     Block, den er nachgespielt hat -- DAG-Regel); ein boeswilliger Erzeuger
//     erreicht damit eine StateRoot-Abweichung, wie mit jeder anderen
//     Zustandsablehnung. Die Pruefung bleibt trotzdem, sonst koennte jeder
//     Erzeuger das Register mit Bindungen erfundener Schluessel fuellen.
//
// Das Register steht in validator_register; validatorSetXOR summiert die
// Eintraege wie accountSetXOR die Konten und steht in der StateRoot -- NUR,
// wenn es Eintraege gibt (sonst byte-gleich, also auch die StateRoot jedes
// bisherigen Blocks). Zurueck mit dem Block (blockRollbackSnapshot), neu
// aufgebaut beim Start (rebuildStateAccumulators), im Snapshot mitgetragen
// und beim Import selbst geprueft.
//
// # SCHLAFEND
//
// Vor validatorRegisterAb() ist jede validator_bindung ungueltig: der Block
// wird abgewiesen (bekannteTxArt), auch beim Nachspielen noch einmal, und
// ein Snapshot mit Eintraegen wird nicht importiert. Es gibt noch keinen Weg,
// eine zu erzeugen, und noch liest niemand das Register -- beides ist
// Schritt 2 und 3. Nichts aendert sich, bis der Stichtag gesetzt wird.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
)

// validatorRegisterAbUnix: Stichtag fuer validator_bindung. Platzhalter --
// den Tag setzt der Betreiber (docs/VALIDATOR_REGISTER_KONSENS.md).
const validatorRegisterAbUnix int64 = math.MaxInt64

// validatorRegisterOverride: nur fuer Tests (0 = Konstante gilt).
var validatorRegisterOverride atomic.Int64

func validatorRegisterAb() int64 {
	if o := validatorRegisterOverride.Load(); o != 0 {
		return o
	}
	return validatorRegisterAbUnix
}

// validatorRegisterAktiv: gilt validator_bindung fuer einen Block mit dieser
// Blockzeit?
func validatorRegisterAktiv(blockZeit int64) bool {
	return blockZeit >= validatorRegisterAb()
}

// validatorBindungNachricht: der Satz, den Betreiber UND Signierschluessel
// unterschreiben. Anders als jeder bisherige Satz -- eine Unterschrift fuer
// /api/peers/register ("authorize validator <adresse>") gilt hier nicht --,
// und mit der Chain-ID: eine Bindung aus einem Testnetz gilt hier nicht.
func validatorBindungNachricht(signing, betreiber string, zeit int64) string {
	return fmt.Sprintf("Aequitas: bind validator %s to operator %s chain:%d ts:%d",
		strings.ToLower(signing), strings.ToLower(betreiber), aequitasChainID.Int64(), zeit)
}

// kanonischeAdresse: 0x + 40 Hex-Ziffern, klein. Eine Bindung hat genau eine
// Schreibweise -- sonst staende dieselbe Adresse zweimal im Register.
func kanonischeAdresse(a string) bool {
	if len(a) != 42 || a[0] != '0' || a[1] != 'x' {
		return false
	}
	for i := 2; i < 42; i++ {
		c := a[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// validatorBindungForm: zustandslos. Ein Verstoss macht den Block ungueltig.
func validatorBindungForm(tx *Transaction) error {
	if !kanonischeAdresse(tx.Wallet) {
		return fmt.Errorf("validator_bindung: Betreiber %q ist keine kanonische Adresse", tx.Wallet)
	}
	if !kanonischeAdresse(tx.To) {
		return fmt.Errorf("validator_bindung: Signieradresse %q ist keine kanonische Adresse", tx.To)
	}
	n := tx.Nachweis
	if n == nil || n.Zeit <= 0 || n.Sig == "" || n.Sig2 == "" {
		return fmt.Errorf("validator_bindung: Nachweis unvollstaendig (Zeit und beide Unterschriften noetig)")
	}
	return nil
}

// validatorNeuer: ist (zeit, signing) groesser als (bisherZeit,
// bisherSigning)? Die eine Ordnung, nach der je Betreiber die Bindung gilt.
func validatorNeuer(zeit int64, signing string, bisherZeit int64, bisherSigning string) bool {
	if zeit != bisherZeit {
		return zeit > bisherZeit
	}
	return signing > bisherSigning
}

// validatorBlatt: ein Eintrag in der Summe. Der Zeitpunkt steht darin -- er
// entscheidet, welche Bindung gilt, und ist fuer alle gleich (aus dem Block,
// nicht aus einer Uhr).
func validatorBlatt(betreiber, signing string, zeit int64) [32]byte {
	b := []byte("validator:" + strings.ToLower(betreiber) + ":" + strings.ToLower(signing) + ":")
	b = strconv.AppendInt(b, zeit, 10)
	return sha256.Sum256(b)
}

// InitValidatorRegisterTable: aus initDB. Kleinschreibung garantiert der
// Schreibweg (kanonischeAdresse), also genuegen einfache Schluessel. Die
// Signieradresse ist bewusst NICHT eindeutig (siehe REIHENFOLGE oben).
func (cs *ChainState) InitValidatorRegisterTable() error {
	if cs.db == nil {
		return nil
	}
	if _, err := cs.db.Exec(`CREATE TABLE IF NOT EXISTS validator_register (
		operator_wallet TEXT PRIMARY KEY,
		signing_address TEXT NOT NULL,
		bindung_ts      BIGINT NOT NULL,
		sig_operator    TEXT NOT NULL,
		sig_signing     TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("validator_register anlegen: %w", err)
	}
	if _, err := cs.db.Exec(`CREATE INDEX IF NOT EXISTS validator_register_signing ON validator_register (signing_address)`); err != nil {
		return fmt.Errorf("validator_register Index: %w", err)
	}
	return nil
}

// Wie oft eine validator_bindung uebersprungen wurde. Eigener Zaehler: die
// Zahl der uebersprungenen Ueberweisungen ist ein Alarm fuer abweichende
// Kontostaende (zustand_ablehnung.go), und eine wiederholte Bindung -- die
// jeder einspielen kann, der sie gesehen hat -- darf ihn nicht ausloesen.
var uebersprungeneBindungen atomic.Int64

// validatorZustand: Zustandsablehnung -- die Transaktion wird
// uebersprungen, der Block bleibt gueltig.
func validatorZustand(format string, args ...interface{}) error {
	return fmt.Errorf("validator_bindung: "+format+": %w", append(args, ErrZustandLehntAb)...)
}

// applyValidatorBindungLocked: Nachspielen. cs.mu gehalten, ctx traegt die
// Transaktion des Blocks. Rueckgabe:
//   - nil: angewendet;
//   - ErrZustandLehntAb (istZustandsAblehnung): uebersprungen;
//   - jeder andere Fehler: der Block ist ungueltig (Form, Unterschrift,
//     Stichtag) oder der Zustand nicht lesbar -- beides rollt den Block
//     zurueck, nie wird ein Lesefehler zu "uebersprungen".
func (cs *ChainState) applyValidatorBindungLocked(ctx context.Context, tx *Transaction, blockZeit int64) error {
	if !validatorRegisterAktiv(blockZeit) {
		return fmt.Errorf("validator_bindung vor dem Stichtag (Block %d, ab %d)", blockZeit, validatorRegisterAb())
	}
	if err := validatorBindungForm(tx); err != nil {
		return err
	}
	// Auch wenn pruefeAuftraegeImBlock den Nachweis schon geprueft hat: hier
	// noch einmal, damit die Pruefung nicht an signierteUeberweisungenPflicht
	// haengt. Bereits gepruefte Unterschriften kommen aus dem Speicher.
	if err := pruefeAuftragsNachweis(tx, blockZeit); err != nil {
		return err
	}
	if cs.db == nil {
		return fmt.Errorf("validator_bindung braucht eine Datenbank")
	}
	betreiber, signing, zeit := tx.Wallet, tx.To, tx.Nachweis.Zeit

	if unbekannt := cs.ladeKontenCtx(ctx, []string{betreiber}); unbekannt[betreiber] {
		return fmt.Errorf("validator_bindung: Konto %s nicht lesbar", kurzAdresse(betreiber))
	}
	if acc, ok := cs.accounts.Get(betreiber); !ok || !acc.IsHuman {
		return validatorZustand("Betreiber %s ist kein registrierter Mensch", kurzAdresse(betreiber))
	}

	db := cs.dbExecCtx(ctx)
	var bisherSigning string
	var bisherZeit int64
	bisher := true
	switch err := db.QueryRow(`SELECT signing_address, bindung_ts FROM validator_register WHERE operator_wallet = $1`,
		betreiber).Scan(&bisherSigning, &bisherZeit); {
	case errors.Is(err, sql.ErrNoRows):
		bisher = false
	case err != nil:
		return fmt.Errorf("validator_bindung: Register lesen: %w", err)
	}
	if bisher && !validatorNeuer(zeit, signing, bisherZeit, bisherSigning) {
		return validatorZustand("%s hat schon eine Bindung von %d -- diese (%d) ist nicht neuer", kurzAdresse(betreiber), bisherZeit, zeit)
	}

	if _, err := db.Exec(
		`INSERT INTO validator_register (operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (operator_wallet) DO UPDATE SET signing_address = $2, bindung_ts = $3, sig_operator = $4, sig_signing = $5`,
		betreiber, signing, zeit, tx.Nachweis.Sig, tx.Nachweis.Sig2,
	); err != nil {
		return fmt.Errorf("validator_bindung: Register schreiben: %w", err)
	}
	if bisher {
		xorInto(&cs.validatorSetXOR, validatorBlatt(betreiber, bisherSigning, bisherZeit))
	}
	xorInto(&cs.validatorSetXOR, validatorBlatt(betreiber, signing, zeit))
	return nil
}

// validatorZuSignieradresseCtx: welcher Betreiber gilt fuer signing? Die
// Bindung mit dem spaetesten Zeitpunkt; teilen sich zwei den spaetesten,
// keiner. "" = keiner. Fuer Schritt 3 (Strafkonto, Belohnung, Komitee) --
// die Leser pruefen dazu, dass der Betreiber Mensch ist.
func (cs *ChainState) validatorZuSignieradresseCtx(ctx context.Context, signing string) (string, error) {
	if cs.db == nil {
		return "", nil
	}
	rows, err := cs.dbExecCtx(ctx).Query(
		`SELECT operator_wallet, bindung_ts FROM validator_register WHERE signing_address = $1
		 ORDER BY bindung_ts DESC, operator_wallet LIMIT 2`, strings.ToLower(signing))
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var betreiber []string
	var zeiten []int64
	for rows.Next() {
		var b string
		var z int64
		if err := rows.Scan(&b, &z); err != nil {
			return "", err
		}
		betreiber, zeiten = append(betreiber, b), append(zeiten, z)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch {
	case len(betreiber) == 0:
		return "", nil
	case len(betreiber) == 2 && zeiten[0] == zeiten[1]:
		return "", nil // umstritten
	}
	return betreiber[0], nil
}

// validatorSummeAusDB: die Summe aller Eintraege, fuer
// rebuildStateAccumulators. Ohne Tabelle null.
func (cs *ChainState) validatorSummeAusDB() ([32]byte, error) {
	var x [32]byte
	eintraege, err := cs.validatorRegisterLesen()
	if err != nil {
		return x, err
	}
	for _, e := range eintraege {
		xorInto(&x, validatorBlatt(e.Betreiber, e.Signing, e.Zeit))
	}
	return x, nil
}

// SnapshotValidator: ein Eintrag des Registers im Snapshot, mit beiden
// Unterschriften -- der importierende Knoten prueft sie selbst.
type SnapshotValidator struct {
	Betreiber   string `json:"operator"`
	Signing     string `json:"signing"`
	Zeit        int64  `json:"ts"`
	SigOperator string `json:"sig_operator"`
	SigSigning  string `json:"sig_signing"`
}

// validatorRegisterLesen: alle Eintraege, nach Betreiber geordnet (der
// Snapshot ist signiert; die Reihenfolge muss fest sein).
func (cs *ChainState) validatorRegisterLesen() ([]SnapshotValidator, error) {
	if cs.db == nil {
		return nil, nil
	}
	var gibt bool
	if err := cs.db.QueryRow(`SELECT to_regclass('validator_register') IS NOT NULL`).Scan(&gibt); err != nil || !gibt {
		return nil, err
	}
	rows, err := cs.db.Query(`SELECT operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing
		FROM validator_register ORDER BY operator_wallet`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SnapshotValidator
	for rows.Next() {
		var e SnapshotValidator
		if err := rows.Scan(&e.Betreiber, &e.Signing, &e.Zeit, &e.SigOperator, &e.SigSigning); err != nil {
			return nil, fmt.Errorf("validator_register lesen: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// pruefeSnapshotValidatoren: der importierende Knoten glaubt dem Snapshot
// die Eintraege nicht, er prueft Form, beide Unterschriften und den
// Stichtag selbst. Das Zeitfenster gilt hier nicht (ein Eintrag ist so alt
// wie seine Bindung), aber keine Bindung kann aelter sein als eine Stunde
// vor dem Stichtag. Ein einziger falscher Eintrag verwirft den Import. Ohne
// cs.mu aufrufen -- die Wiederherstellung der Schluessel kostet Zeit.
func pruefeSnapshotValidatoren(liste []SnapshotValidator) error {
	fruehestens := validatorRegisterAb() - nachweisHoechstensAlt
	betreiber := make(map[string]bool, len(liste))
	for i, e := range liste {
		tx := Transaction{Type: "validator_bindung", Wallet: e.Betreiber, To: e.Signing,
			Nachweis: &Auftragsnachweis{Zeit: e.Zeit, Sig: e.SigOperator, Sig2: e.SigSigning}}
		if err := validatorBindungForm(&tx); err != nil {
			return fmt.Errorf("Validator %d: %w", i, err)
		}
		if e.Zeit < fruehestens {
			return fmt.Errorf("Validator %d: Bindung von %d liegt vor dem Stichtag", i, e.Zeit)
		}
		if betreiber[e.Betreiber] {
			return fmt.Errorf("Validator %d: Betreiber %s doppelt", i, kurzAdresse(e.Betreiber))
		}
		betreiber[e.Betreiber] = true
		msg := validatorBindungNachricht(e.Signing, e.Betreiber, e.Zeit)
		if err := pruefePersonalSignGemerkt(msg, e.SigOperator, e.Betreiber); err != nil {
			return fmt.Errorf("Validator %d: Unterschrift des Betreibers: %w", i, err)
		}
		if err := pruefePersonalSignGemerkt(msg, e.SigSigning, e.Signing); err != nil {
			return fmt.Errorf("Validator %d: Unterschrift des Signierschluessels: %w", i, err)
		}
	}
	return nil
}

// validatorImportZiel: die *sql.Tx des Imports.
type validatorImportZiel interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// validatorenImportieren: in die Transaktion des Imports. ersetzen = Resync
// (das Register gilt, wie es im Snapshot steht); sonst ergaenzend nach
// derselben Regel wie beim Nachspielen -- je Betreiber die neuere Bindung
// (COLLATE "C": byteweise wie validatorNeuer, nicht nach Sprachregeln).
// Prueft die Eintraege noch einmal (dann aus dem Speicher der Unterschriften).
func validatorenImportieren(tx validatorImportZiel, liste []SnapshotValidator, ersetzen bool) error {
	if err := pruefeSnapshotValidatoren(liste); err != nil {
		return err
	}
	if ersetzen {
		if _, err := tx.Exec(`TRUNCATE validator_register`); err != nil {
			return fmt.Errorf("validator_register leeren: %w", err)
		}
	}
	for _, e := range liste {
		if _, err := tx.Exec(
			`INSERT INTO validator_register (operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (operator_wallet) DO UPDATE SET signing_address = EXCLUDED.signing_address,
			   bindung_ts = EXCLUDED.bindung_ts, sig_operator = EXCLUDED.sig_operator, sig_signing = EXCLUDED.sig_signing
			 WHERE (EXCLUDED.bindung_ts, EXCLUDED.signing_address COLLATE "C") > (validator_register.bindung_ts, validator_register.signing_address COLLATE "C")`,
			e.Betreiber, e.Signing, e.Zeit, e.SigOperator, e.SigSigning,
		); err != nil {
			return fmt.Errorf("Validator %s: %w", kurzAdresse(e.Betreiber), err)
		}
	}
	return nil
}
