package keeper

// Jaehrlicher Lebensnachweis -- der Teil der Kette.
//
// WAS. Einmal im Jahr bestaetigt ein registrierter Mensch mit DEMSELBEN
// Gesicht wie bei der Anmeldung, dass er noch lebt. Der Coordinator prueft
// das (coordinator/app/lebensnachweis.py: Quorum erkennt das Gesicht, die
// getroffene Einschreibung gehoert genau dieser Wallet) und bescheinigt es
// mit seinem Ed25519-Schluessel:
//
//	aequitas-proof-of-life-v1|chain:1926|<wallet>|<issued_at>
//
// Die App reicht die Bescheinigung an POST /api/proof-of-life weiter
// ({"wallet","issued_at","signature","public_key"}, wie bei der Erneuerung).
// Die Transaktion proof_of_life traegt sie samt Bindung des Schluessels
// (Lebendigkeitsbescheinigung), und JEDER Knoten prueft sie beim Nachspielen
// selbst -- eigene Domaene: eine Erneuerungs-Bescheinigung ist kein
// Lebensnachweis und umgekehrt (bescheinigungPruefenFuer).
//
// NUR BESTAETIGEN, NIE WEGNEHMEN (docs/VERIFIER_PRINZIPIEN.md). Niemand
// verliert das Menschsein, kein Guthaben wird beruehrt. Wer ueberfaellig ist,
// bekommt NUR seinen Tagesanteil am Grundeinkommen nicht, solange er
// ueberfaellig ist -- und dieser Anteil wird nicht einbehalten: der
// Ueberfaellige steht in dieser Runde nicht in der Empfaengermenge, der Topf
// teilt sich auf alle anderen (distributeUBIPoolLocked). Nichts liegt auf
// einem Treuhandkonto, nichts verfaellt. Nach einem gueltigen Nachweis ist er
// ab der naechsten Runde wieder dabei.
//
// FRISTEN (je Konto, aus dem Kettenzustand):
//
//	am         = LebensnachweisAm (issued_at des letzten Nachweises; fuer eine
//	             Registrierung nach dem Stichtag deren Zeitpunkt), mindestens
//	             lebensnachweisUhrAb (Stichtag bzw. spaetere Zulassung der
//	             Coordinatoren) -- wer heute registriert ist, beginnt dort.
//	faellig_ab = am + 300 Tage: fruehester Nachweis. Frueher wird keiner
//	             angenommen -- das bremst auch das Abdriften der Vorlage.
//	frist      = am + 365 Tage.
//	pause_ab   = am + 395 Tage (Frist + 30 Tage Karenz).
//	pausiert   = aktiv && t >= pause_ab.
//
// AKTIV NUR, WENN NACHWEISE MOEGLICH SIND. Ein Nachweis gilt nur mit der
// Bescheinigung eines im Konsens zugelassenen Coordinators
// (bescheinigungPruefen: registerLeserAktiv + Validator-Schluessel im
// Kettenregister). registerLeserAbUnix steht heute auf MaxInt64 -- es ist
// also niemand zugelassen, und kein Nachweis koennte gelingen. Eine Pause
// ohne Moeglichkeit, sie zu beenden, hielte Geld ohne Zeitgrenze zurueck.
// Darum: aktiv = Stichtag erreicht UND registerLeserAktiv. Vorher wird
// niemand pausiert, der Endpunkt antwortet 409, eine proof_of_life im Block
// ist Leerlauf -- fail-closed fuer die Pruefung (kein Nachweis ohne
// zugelassenen Coordinator), ohne Geld festzuhalten.
//
// DIE UHR BEGINNT, WENN NACHWEISE MOEGLICH WERDEN. Kaeme die Zulassung erst
// nach der ersten Faelligkeit (Stichtag + 300 Tage), waeren beim Einschalten
// alle schon ueber pause_ab und wuerden ohne jeden Vorlauf pausiert. Darum
// zaehlt als fruehestes am nicht der Stichtag allein, sondern der spaetere von
// Stichtag und registerLeserAb (lebensnachweisUhrAb) -- jeder hat nach dem
// Einschalten der Zulassung die vollen 300/365/395 Tage. Steht
// registerLeserAb noch auf seinem Platzhalter (nach 2100), bleibt es beim
// Stichtag; aktiv ist dann ohnehin false.
//
// DETERMINISMUS. Ob jemand pausiert ist, haengt nur an der Rundenzeit
// (ubiAt, steht ab dem Stichtag in jeder ubi_distribution als
// DistributionAt) und am Kettenzustand (LebensnachweisAm, IsHuman). Der
// Erzeuger waehlt die Empfaenger; die Bloecke tragen die Betraege; jeder
// Nachspielende rechnet Empfaengerzahl, Anteil und "kein Pausierter
// bekommt" selbst nach (nachrechnen_ubi.go). LebensnachweisAm steht im
// accountLeaf nur, wenn gesetzt, und in chain_accounts/Snapshot mit
// COALESCE/omitempty: jedes Konto ohne Nachweis behaelt seinen Blattwert.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// 01.11.2026 00:00:00 UTC. Ein Datum im Code, nicht in einer
	// Umgebungsvariablen: alle Knoten muessen dieselbe Antwort geben.
	lebensnachweisAbUnix int64 = 1793491200

	lebensnachweisFaelligTage int64 = 300
	lebensnachweisFristTage   int64 = 365
	lebensnachweisPauseTage   int64 = 395 // Frist + 30 Tage Karenz

	// Der Satz, den der Coordinator unterschreibt (KETTEN_DOMAENE in
	// coordinator/app/lebensnachweis.py) -- mit der eingefrorenen Chain-ID
	// wie erneuerungsNachricht (coordinator_nachrichten.go).
	lebensnachweisDomain = "aequitas-proof-of-life-v1"

	lebensnachweisTxArt = "proof_of_life"

	// Die Annahme nimmt hoechstens 15 Minuten alte und 60 s vordatierte
	// Bescheinigungen (wie die Erneuerung).
	lebensnachweisAnnahmeAlt    int64 = 900
	lebensnachweisAnnahmeVoraus int64 = 60
)

// lebensnachweisAbOverride: nur fuer Tests (0 = Konstante gilt).
var lebensnachweisAbOverride atomic.Int64

func lebensnachweisAb() int64 {
	if o := lebensnachweisAbOverride.Load(); o > 0 {
		return o
	}
	return lebensnachweisAbUnix
}

// lebensnachweisUhrMax: ein registerLeserAb dahinter ist ein Platzhalter
// (heute MaxInt64), kein Termin -- 01.01.2100.
const lebensnachweisUhrMax int64 = 4102444800

// lebensnachweisUhrAb: das frueheste am -- der spaetere von Stichtag und
// Zulassung der Coordinatoren (siehe "DIE UHR BEGINNT ..."). Eine Konstante
// des Programms, bei allen Knoten gleich.
func lebensnachweisUhrAb() int64 {
	ab := lebensnachweisAb()
	if r := registerLeserAb(); r > ab && r < lebensnachweisUhrMax {
		ab = r
	}
	return ab
}

// lebensnachweisStichtag: liegt t am oder nach dem Stichtag? Ab hier zaehlt
// eine Registrierung als erster Nachweis (lebensnachweisRegZeit).
func lebensnachweisStichtag(t int64) bool { return t >= lebensnachweisAb() }

// lebensnachweisAktiv: koennen zu t Nachweise angenommen werden -- und darf
// deshalb pausiert werden? Stichtag UND Coordinatoren im Konsens zugelassen
// (siehe "AKTIV NUR, WENN NACHWEISE MOEGLICH SIND").
func lebensnachweisAktiv(t int64) bool {
	return lebensnachweisStichtag(t) && registerLeserAktiv(t)
}

// lebensnachweisNachricht: was der Coordinator fuer einen Lebensnachweis
// unterschreibt. wallet in der einen Schreibweise (klein).
func lebensnachweisNachricht(wallet string, issuedAt int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%d", lebensnachweisDomain, eingefroreneChainID, wallet, issuedAt)
}

// lebensnachweisTransaktion: die proof_of_life mit Bescheinigung und Bindung,
// jede Unterschrift in ihrer einen Schreibweise (wie erneuerungsTransaktion).
func lebensnachweisTransaktion(wallet string, issuedAt int64, publicHex, signatureHex string, bindung CoordinatorBindung) Transaction {
	tx := erneuerungsTransaktion(wallet, issuedAt, publicHex, signatureHex, bindung)
	tx.Type = lebensnachweisTxArt
	return tx
}

// lebensnachweisFristen: die vier Zeitpunkte eines Kontos (Unix-Sekunden).
type lebensnachweisFristen struct {
	am, faelligAb, frist, pauseAb int64
}

func lebensnachweisFristenVon(acc *AccountState) lebensnachweisFristen {
	am := lebensnachweisUhrAb()
	if acc != nil && acc.LebensnachweisAm > am {
		am = acc.LebensnachweisAm
	}
	return lebensnachweisFristen{
		am:        am,
		faelligAb: sattAdd(am, lebensnachweisFaelligTage*86400),
		frist:     sattAdd(am, lebensnachweisFristTage*86400),
		pauseAb:   sattAdd(am, lebensnachweisPauseTage*86400),
	}
}

// lebensnachweisPausiert: bekommt dieser Mensch zur Rundenzeit t keinen
// Tagesanteil? Nur aus Kettenzustand und t -- bei jedem Knoten gleich.
func lebensnachweisPausiert(acc *AccountState, t int64) bool {
	return acc != nil && acc.IsHuman && lebensnachweisAktiv(t) && t >= lebensnachweisFristenVon(acc).pauseAb
}

// lebensnachweisPauseGrenze: pausiert ist zu t, wessen wirksames am <= dieser
// Grenze liegt (t >= am + 395 Tage).
func lebensnachweisPauseGrenze(t int64) int64 {
	return sattAdd(t, -lebensnachweisPauseTage*86400)
}

// LebensnachweisStand fuer /api/balance: alle Zeitpunkte als ganze
// Unix-Sekunden, aktiv/pausiert als bool. Fuer Nicht-Menschen nil. Vor dem
// Stichtag aktiv=false, die Zeitpunkte trotzdem berechnet.
func LebensnachweisStand(acc *AccountState, jetzt int64) map[string]interface{} {
	if acc == nil || !acc.IsHuman {
		return nil
	}
	f := lebensnachweisFristenVon(acc)
	return map[string]interface{}{
		"aktiv":      lebensnachweisAktiv(jetzt),
		"am":         f.am,
		"faellig_ab": f.faelligAb,
		"frist":      f.frist,
		"pause_ab":   f.pauseAb,
		"pausiert":   lebensnachweisPausiert(acc, jetzt),
	}
}

// LebensnachweisStandVon liest den Stand eines Kontos (auch ein kaltes).
func (cs *ChainState) LebensnachweisStandVon(address string, jetzt int64) map[string]interface{} {
	var st map[string]interface{}
	cs.readAccount(strings.ToLower(strings.TrimSpace(address)), func(acc *AccountState) {
		st = LebensnachweisStand(acc, jetzt)
	})
	return st
}

// ------------------------------------------------------------ Registrierung

// lebensnachweisRegZeit: ab wann eine Registrierung als erster Nachweis
// zaehlt -- 0 (nichts setzen), wenn die Transaktion keinen Annahmezeitpunkt
// traegt oder er vor dem Stichtag liegt. Derselbe Wert wie fuer die Staffel
// (staffelRegZeit, "EIN ZEITPUNKT" in grant_staffel.go): der Erzeuger misst
// mit seiner Uhr, jeder Nachspielende mit der Blockzeit, beide kommen auf
// RegAt. OHNE RegAt (der Spiegel-Ersatzweg in register.go, sonst tot) gaebe
// es keinen gemeinsamen Wert -- dann bleibt das Feld 0 und am ist der
// Stichtag, bei allen Knoten gleich.
func lebensnachweisRegZeit(regAt, bezug int64) int64 {
	if regAt <= 0 {
		return 0
	}
	z := staffelRegZeit(regAt, bezug)
	if !lebensnachweisStichtag(z) {
		return 0
	}
	return z
}

// lebensnachweisBeiRegistrierungLocked setzt nach einer Registrierung
// LebensnachweisAm (lebensnachweisRegZeit). Vor dem Stichtag nichts -- kein
// Blatt aendert sich. Caller haelt cs.mu.
func (cs *ChainState) lebensnachweisBeiRegistrierungLocked(ctx context.Context, address string, regAt, bezug int64) error {
	z := lebensnachweisRegZeit(regAt, bezug)
	if z == 0 {
		return nil
	}
	address = strings.ToLower(strings.TrimSpace(address))
	acc, ok := cs.accounts.Get(address)
	if !ok || !acc.IsHuman || acc.LebensnachweisAm >= z {
		return nil
	}
	acc.LebensnachweisAm = z
	return cs.saveAccountToDBCtx(ctx, acc)
}

// ------------------------------------------------------------ Nachspielen

// lebensnachweisUrteil: was eine proof_of_life zu einer Blockzeit bewirkt.
type lebensnachweisUrteil struct {
	anwenden bool   // LebensnachweisAm wird issued_at
	regel    string // Befund (nachrechnen), "" = keiner
	text     string
}

func lebensnachweisBefund(regel, format string, args ...interface{}) lebensnachweisUrteil {
	return lebensnachweisUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

// lebensnachweisZulassung: die Zulassung aus dem Kettenregister, in der
// laufenden Transaktion (dbExecCtx) -- beim Nachspielen wie bei der Annahme
// unter cs.mu, ohne zweite Verbindung.
func (cs *ChainState) lebensnachweisZulassung(ctx context.Context) CoordinatorZulassung {
	if cs.zulassungOhneDB != nil && cs.db == nil {
		return cs.zulassungOhneDB
	}
	return func(m string, t int64) (bool, error) {
		if cs.db == nil {
			return false, fmt.Errorf("keine Datenbank")
		}
		return coordinatorZugelassenIn(cs.dbExecCtx(ctx), m, t)
	}
}

// lebensnachweisUrteilLocked: darf tx zu blockUnix angewandt werden? Jeder
// Knoten prueft selbst, in dieser Reihenfolge (billig vor teuer):
//
//   - vor lebensnachweisAktiv: Leerlauf, kein Befund;
//   - Wallet in kanonischer Schreibweise (sonst laege derselbe Nachweis
//     unter mehreren Hashes in Bloecken);
//   - issued_at > 0 und hoechstens nachweisHoechstensVoraus nach dem Block;
//   - die Wallet ist ein registrierter Mensch;
//   - schon nachgewiesen (LebensnachweisAm >= issued_at): Leerlauf -- ein
//     zweites Nachspielen derselben Transaktion aendert nichts;
//   - issued_at >= faellig_ab (lebensnachweis_zu_frueh);
//   - die Bescheinigung mit eigener Domaene, Bindung in der Transaktion,
//     Coordinator nicht der Nachweisende selbst, Mensch registriert, im
//     Konsens zugelassen (bescheinigungPruefenFuer).
//
// Fehler nur, wenn die Zulassung nicht lesbar war (Infrastruktur): dann
// wird nichts entschieden -- beim Nachspielen geht der ganze Block zurueck
// und wird spaeter wiederholt, nie "ungeprueft angewandt" und nie
// "ungeprueft verworfen". Caller haelt cs.mu.
func (cs *ChainState) lebensnachweisUrteilLocked(ctx context.Context, tx *Transaction, blockUnix int64) (lebensnachweisUrteil, error) {
	if !lebensnachweisAktiv(blockUnix) {
		return lebensnachweisUrteil{}, nil
	}
	if !kanonischeAdresse(tx.Wallet) {
		return lebensnachweisBefund("lebensnachweis_ohne_bescheinigung",
			"%q: Wallet nicht in kanonischer Schreibweise", tx.Wallet), nil
	}
	wallet := tx.Wallet
	zeit := tx.DistributionAt
	if zeit <= 0 || zeit > sattAdd(blockUnix, nachweisHoechstensVoraus) {
		return lebensnachweisBefund("lebensnachweis_zeit",
			"%s: bescheinigt %d, Blockzeit %d", kurzAdresse(wallet), zeit, blockUnix), nil
	}
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok || !acc.IsHuman {
		return lebensnachweisBefund("lebensnachweis_kein_mensch",
			"%s ist kein registrierter Mensch", kurzAdresse(wallet)), nil
	}
	if acc.LebensnachweisAm >= zeit {
		return lebensnachweisUrteil{}, nil
	}
	if ab := lebensnachweisFristenVon(acc).faelligAb; zeit < ab {
		return lebensnachweisBefund("lebensnachweis_zu_frueh",
			"%s: bescheinigt %d, faellig ab %d", kurzAdresse(wallet), zeit, ab), nil
	}
	stand := func(m string) (bool, bool) {
		cs.ensureAccountLoadedCtx(ctx, m)
		a, ok := cs.accounts.Get(m)
		return ok && a.IsHuman, ok && a.GrantStagedRest > 0
	}
	if err := bescheinigungPruefenFuer(lebensnachweisNachricht, wallet, zeit, tx.Bescheinigung, stand, cs.lebensnachweisZulassung(ctx)); err != nil {
		if errors.Is(err, errZulassungNichtLesbar) {
			return lebensnachweisUrteil{}, err
		}
		return lebensnachweisBefund("lebensnachweis_ohne_bescheinigung",
			"%s: keine gueltige Bescheinigung: %v", kurzAdresse(wallet), err), nil
	}
	return lebensnachweisUrteil{anwenden: true}, nil
}

// lebensnachweisSetzenLocked: LebensnachweisAm rueckt vor, nie zurueck.
func (cs *ChainState) lebensnachweisSetzenLocked(ctx context.Context, wallet string, zeit int64) error {
	acc, ok := cs.accounts.Get(wallet)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("proof_of_life fuer %s: kein registrierter Mensch", kurzAdresse(wallet))
	}
	if acc.LebensnachweisAm >= zeit {
		return nil
	}
	acc.LebensnachweisAm = zeit
	return cs.saveAccountToDBCtx(ctx, acc)
}

// applyLebensnachweisLocked: proof_of_life beim Nachspielen. Ein ungueltiger
// Nachweis wird nie angewandt -- in keinem Modus (fail-closed); er wird als
// Befund gezaehlt und lehnt im strengen Modus den Block ab. Caller haelt
// cs.mu.
func (cs *ChainState) applyLebensnachweisLocked(ctx context.Context, tx *Transaction, blockUnix int64) error {
	u, err := cs.lebensnachweisUrteilLocked(ctx, tx, blockUnix)
	if err != nil {
		return err
	}
	if u.regel != "" {
		return nachrechnenAbweichung(u.regel, blockUnix, "%s", u.text)
	}
	if !u.anwenden {
		return nil
	}
	return cs.lebensnachweisSetzenLocked(ctx, tx.Wallet, tx.DistributionAt)
}

// nachrechnenLebensnachweisLocked: was nur gemeldet, nicht entschieden wird
// -- eine Bescheinigung, die laenger als 7 Tage bis in den Block brauchte
// (wie erneuerung_zeit). Sie bringt nichts: LebensnachweisAm wird ihr
// eigenes issued_at, und vor faellig_ab gilt sie nicht. Abgelehnt erst im
// strengen Modus; dann haelt block_tauglich.go den Erzeuger im Fenster.
func (cs *ChainState) nachrechnenLebensnachweisLocked(tx *Transaction, blockZeit int64) error {
	if !lebensnachweisAktiv(blockZeit) {
		return nil
	}
	if tx.DistributionAt > 0 && blockZeit-tx.DistributionAt > erneuerungHoechstensAlt {
		return nachrechnenAbweichung("lebensnachweis_zeit", blockZeit,
			"%s: bescheinigt %d, Blockzeit %d (aelter als 7 Tage)", kurzAdresse(tx.Wallet), tx.DistributionAt, blockZeit)
	}
	return nil
}

// ------------------------------------------------------------ Grundeinkommen

// lebensnachweisEmpfaengerLocked: die Menschen, die zur Rundenzeit t ihren
// Tagesanteil bekommen -- alle ausser den Pausierten, in derselben
// Reihenfolge. Vor lebensnachweisAktiv(t) alle. Caller haelt cs.mu, die
// Konten sind geladen.
func (cs *ChainState) lebensnachweisEmpfaengerLocked(menschen []string, t int64) (empfaenger []string, pausiert int) {
	if !lebensnachweisAktiv(t) {
		return menschen, 0
	}
	empfaenger = make([]string, 0, len(menschen))
	for _, a := range menschen {
		if acc, ok := cs.accounts.Get(a); ok && lebensnachweisPausiert(acc, t) {
			pausiert++
			continue
		}
		empfaenger = append(empfaenger, a)
	}
	return empfaenger, pausiert
}

// countUBIEmpfaengerLocked: wie viele Menschen zur Rundenzeit t ihren
// Tagesanteil bekommen (nachrechnen_ubi.go). Eine Zaehlung, keine Liste --
// wie countHumanAccountsLocked. Dieselbe Bedingung wie
// lebensnachweisPausiert, in SQL: wirksames am (mindestens lebensnachweisUhrAb) >
// t - 395 Tage.
func (cs *ChainState) countUBIEmpfaengerLocked(ctx context.Context, t int64) (int64, error) {
	if !lebensnachweisAktiv(t) {
		return cs.countHumanAccountsLocked(ctx)
	}
	if cs.db == nil {
		var n int64
		cs.accounts.Range(func(_ string, acc *AccountState) bool {
			if acc.IsHuman && !lebensnachweisPausiert(acc, t) {
				n++
			}
			return true
		})
		return n, nil
	}
	var n int64
	if err := cs.dbExecCtx(ctx).QueryRow(
		`SELECT count(*) FROM chain_accounts WHERE is_human = true AND GREATEST(COALESCE(lebensnachweis_am, 0), $2) > $1`,
		lebensnachweisPauseGrenze(t), lebensnachweisUhrAb()).Scan(&n); err != nil {
		return 0, fmt.Errorf("count UBI recipients: %w", err)
	}
	return n, nil
}

// ------------------------------------------------------------ API

var lebensnachweisGrenzArt = nachweisGrenzArt{
	aktiv:      lebensnachweisAktiv,
	inaktiv:    "proof of life not active yet",
	direkt:     "proof-of-life:",
	pruefung:   "proof-of-life-pruefung:",
	von:        lebensnachweisVonPraefix,
	zuVieleTxt: "too many proof-of-life requests from this address -- please retry in a minute",
}

// lebensnachweisGrenze: dieselbe Grenze wie fuer die Erneuerung
// (erneuerungsGrenze: burstErneuerungJeIP je Minute und Absender, gezaehlt
// VOR der Weiterleitung, weitergeleitete mit Nachweis unter dem Absender des
// Folgers), mit EIGENEN Zaehlern -- Erneuerungen verbrauchen nicht das
// Budget der Lebensnachweise und umgekehrt. Vor lebensnachweisAktiv 409.
func (a *APIServer) lebensnachweisGrenze(next http.HandlerFunc) http.HandlerFunc {
	return a.nachweisGrenze(lebensnachweisGrenzArt, next)
}

// errSchonNachgewiesen: dieser oder ein spaeterer Nachweis ist schon
// angewandt -- nichts in den Ausgang.
var errSchonNachgewiesen = errors.New("schon nachgewiesen")

// lebensnachweisAbgelehnt: die Pruefung unter der Sperre hat den Nachweis
// abgewiesen (z. B. zwei gleichzeitige Anfragen).
type lebensnachweisAbgelehnt struct{ text string }

func (e *lebensnachweisAbgelehnt) Error() string { return e.text }

// handleProofOfLife nimmt einen Lebensnachweis entgegen. Ablauf wie
// handleLivenessRenewal: Form, Zeitfenster, Unterschrift (vor jeder
// Datenbank), Bindung aus dem Register dieses Knotens in die Transaktion,
// dieselbe Pruefung wie jeder Nachspielende, Annahme-Tor, Ausgang atomar mit
// dem Zustand. Dieselbe oder eine aeltere Bescheinigung noch einmal: 200 mit
// schon_nachgewiesen, nichts in den Ausgang.
func (a *APIServer) handleProofOfLife(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now().Unix()
	if !lebensnachweisAktiv(now) {
		jsonError(w, "proof of life not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Wallet    string `json:"wallet"`
		IssuedAt  int64  `json:"issued_at"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	if !kanonischeAdresse(wallet) {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}
	if req.IssuedAt <= 0 || now-req.IssuedAt > lebensnachweisAnnahmeAlt || req.IssuedAt-now > lebensnachweisAnnahmeVoraus {
		jsonError(w, "attestation expired or from the future", http.StatusBadRequest)
		return
	}
	// Die Unterschrift haengt nur an Schluessel und Satz: vor jeder
	// Registerabfrage -- eine Muell-Unterschrift kostet keine Datenbank.
	if !ed25519PruefenStreng(strings.ToLower(strings.TrimSpace(req.PublicKey)), ed25519SigNormal(req.Signature),
		[]byte(lebensnachweisNachricht(wallet, req.IssuedAt))) {
		jsonError(w, "invalid proof-of-life attestation: Bescheinigung passt nicht zu Wallet und Zeitpunkt (oder ist keine Ed25519-Unterschrift in kanonischer Schreibweise)", http.StatusForbidden)
		return
	}
	bindung, ok, err := a.state.coordinatorBindungLesen(r.Context(), req.PublicKey)
	if err != nil {
		fmt.Printf("[API] proof of life: coordinator register not readable for %q: %v\n", kurzAdresse(wallet), err)
		jsonError(w, "coordinator register temporarily unavailable, please retry shortly", http.StatusServiceUnavailable)
		return
	}
	if !ok {
		jsonError(w, "proof-of-life attestation not signed by a registered coordinator (or its registration predates stored signatures -- register the coordinator key again)", http.StatusForbidden)
		return
	}
	tx := lebensnachweisTransaktion(wallet, req.IssuedAt, req.PublicKey, req.Signature, bindung)
	if err := bescheinigungPruefenFuer(lebensnachweisNachricht, wallet, req.IssuedAt, tx.Bescheinigung, a.state.coordinatorMenschStand, a.state.coordinatorZugelassen); err != nil {
		if annahmeFehlerIntern(err) {
			fmt.Printf("[API] proof of life failed for %s: %v\n", kurzAdresse(wallet), err)
			jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
			return
		}
		jsonError(w, "invalid proof-of-life attestation: "+err.Error(), http.StatusForbidden)
		return
	}
	var istMensch bool
	var am int64
	var f lebensnachweisFristen
	a.state.readAccount(wallet, func(acc *AccountState) {
		istMensch, am, f = acc.IsHuman, acc.LebensnachweisAm, lebensnachweisFristenVon(acc)
	})
	if !istMensch {
		jsonError(w, "not a registered human", http.StatusConflict)
		return
	}
	if req.IssuedAt <= am {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "wallet": wallet, "schon_nachgewiesen": true})
		return
	}
	// Faellig gilt fuer die Bescheinigung selbst -- so prueft es jeder
	// Nachspielende (lebensnachweis_zu_frueh).
	if now < f.faelligAb || req.IssuedAt < f.faelligAb {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "proof of life not due yet", "faellig_ab": f.faelligAb})
		return
	}
	// Durchs Annahme-Tor: nur der Zustaendige des Kontos nimmt an (Folger
	// leiten weiter, zumLeiter).
	if err := a.state.annahmeBeginnen(wallet); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer a.state.annahmeEnde()
	if err := a.state.runAtomicWithOutbox([]string{wallet}, false, func(ctx context.Context) (Transaction, error) {
		// Unter der Sperre noch einmal genau die Pruefung jedes
		// Nachspielenden: was hier angewandt wird, wendet jeder an. Zwei
		// gleichzeitige Anfragen kommen nicht beide durch.
		u, err := a.state.lebensnachweisUrteilLocked(ctx, &tx, now)
		if err != nil {
			return Transaction{}, err
		}
		if u.regel != "" {
			return Transaction{}, &lebensnachweisAbgelehnt{text: u.regel + ": " + u.text}
		}
		if !u.anwenden {
			return Transaction{}, errSchonNachgewiesen
		}
		if err := a.state.lebensnachweisSetzenLocked(ctx, wallet, req.IssuedAt); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	}); err != nil {
		var abgelehnt *lebensnachweisAbgelehnt
		switch {
		case errors.Is(err, errSchonNachgewiesen):
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "wallet": wallet, "schon_nachgewiesen": true})
		case errors.As(err, &abgelehnt):
			jsonError(w, "proof of life rejected: "+abgelehnt.text, http.StatusConflict)
		default:
			fmt.Printf("[API] proof of life failed for %s: %v\n", kurzAdresse(wallet), err)
			jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		}
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true, "wallet": wallet, "lebensnachweis_am": req.IssuedAt,
		"lebensnachweis": a.state.LebensnachweisStandVon(wallet, time.Now().Unix()),
	})
}
