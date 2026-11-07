package keeper

// PRUEFER-REGISTER MIT ABSTIMMUNG (B1, docs/VERIFIER_PRINZIPIEN.md,
// "Festgelegte Entscheidungen" 18.10.2026; docs/DEZENTRALITAET.md).
//
// # WARUM
//
// Wer Coordinator (Pruefer) sein darf, entschied bis hierher allein das
// Validator-Register: wer einen Validator-Schluessel haelt, ist zugelassen
// (coordinator_zulassung.go). Eine Bindung kostet aber nichts
// (Sicherheitsdurchgang #310, H1): jeder registrierte Mensch bindet einen
// frischen Schluessel und bescheinigt zwei Stunden spaeter Erneuerungen,
// Lebensnachweise und Einsprueche. Gegen seinen Willen entziehen liess er sich
// nicht. Und wer neu dazukommen sollte, bestimmte praktisch der Betreiber.
//
// # WAS JETZT GILT (ab verifierRegisterAb)
//
// Pruefer ist, wer zur Zeit t
//
//   - einen Validator-Schluessel im Kettenregister haelt (wie bisher), UND
//   - im Pruefer-Register steht: als Gruender (verifierGruender) oder durch
//     Aufnahme mit den Stimmen von mindestens zwei Dritteln der Pruefer, die
//     zu dieser Zeit schon zugelassen sind.
//
// Die Knappheit liegt damit in den Stimmen bestehender Pruefer, nicht in
// Geld: kein Guthaben, kein Einsatz (Entscheidung 2). Jeder Mensch zaehlt
// einmal (Entscheidung 1, 6): gezaehlt werden Menschen, nie Schluessel.
//
// Eine Transaktion verifier_zulassung (Wallet = der Betroffene, issued_at in
// DistributionAt) hat eine von drei Arten:
//
//	aufnahme  Stimmen >= ceil(2n/3) der n Pruefer zur Zeit issued_at; der
//	          Betroffene ist registrierter Mensch und noch kein Pruefer
//	austritt  die eine Stimme des Betroffenen selbst -- jederzeit, ohne
//	          Bedingung (Entscheidung 4)
//	entzug    Stimmen >= ceil(2(n-1)/3) der uebrigen Pruefer (Entscheidung 5);
//	          wer allein ist, kann nur austreten
//
// Jede Stimme ist eine Bescheinigung wie beim Einspruch gegen eine Staffel
// (bescheinigungPruefenFuer: Ed25519-Schluessel des Coordinators, Bindung an
// seinen Menschen, Mensch registriert und ohne offene Staffel, Validator-
// Schluessel zur Zeit der Stimme) ueber
//
//	aequitas-verifier-zulassung-v1|chain:<id>|<art>|<betroffener>|<stimmzeit>
//
// mit eigener Stimmzeit: Pruefer stimmen nicht im selben Augenblick ab. Eine
// Stimme gilt hoechstens verifierStimmeHoechstensAlt vor issued_at und nie vor
// der letzten Aenderung beim Betroffenen -- alte Stimmen nehmen niemanden ein
// zweites Mal auf.
//
// # WIRKUNG, NIE RUECKWIRKEND
//
// Jede Aenderung wirkt ab issued_at + erzeugerFrist, nie frueher. Ein Entzug
// oder Austritt beendet die Zulassung fuer die Zukunft: was der Pruefer
// vorher bescheinigt hat, bleibt gueltig (Grundsatz 1) -- jede Pruefung fragt
// "war er zur Zeit DIESER Bescheinigung zugelassen?".
//
// # REIHENFOLGE
//
// Geschwisterbloecke spielt jeder Knoten in der Reihenfolge nach, in der sie
// ankommen. Damit das Register davon nicht abhaengt, gilt wie beim
// Validator-Register (validator_register.go): issued_at liegt hoechstens
// nachweisHoechstensAlt vor dem Block, und die Wirkung beginnt erst
// erzeugerFrist danach. Wer zur Zeit t Pruefer ist, steht also fest, bevor ein
// Block, der t betrifft, nachgespielt wird -- jeder ehrliche Knoten hat die
// Aenderung bis dahin gesehen. Zaehlt eine Abstimmung die Pruefer zu
// issued_at, stehen diese ebenso fest.
//
// # ZUSTAND
//
// Das Register steht in chain_config (verifierKonfigSchluessel), kanonisch als
// JSON, hoechstens verifierHoechstens Menschen; im Rueckroll-Snapshot, im
// Snapshot-Export und in der StateRoot -- dort nur, wenn es Eintraege gibt
// (sonst byte-gleich). Gruender stehen im Code, nicht im Zustand; ein
// Eintrag fuer einen Gruender (nach seinem Austritt) geht vor.
//
// # SCHLAFEND
//
// Vor verifierRegisterAb ist die Art unbekannt (bekannteTxArt: der Block wird
// abgewiesen), und die Zulassung bleibt wie bisher. Nichts aendert sich, bis
// der Stichtag gesetzt wird.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	verifierTxArt            = "verifier_zulassung"
	verifierDomain           = "aequitas-verifier-zulassung-v1"
	verifierKonfigSchluessel = "verifier_register"

	verifierArtAufnahme = "aufnahme"
	verifierArtAustritt = "austritt"
	verifierArtEntzug   = "entzug"

	// Hoechstens so viele Menschen im Register (Gruender zaehlen mit). Jede
	// Abstimmung zaehlt die Pruefer -- ohne Grenze wuechse das ohne Ende.
	verifierHoechstens = 256
	// Stimmen je Transaktion: zwei Drittel von verifierHoechstens, aufgerundet.
	verifierStimmenHoechstens = 171
	// Eine Stimme gilt hoechstens 14 Tage vor issued_at: Menschen stimmen
	// nicht im selben Augenblick ab, aber eine Stimme verjaehrt.
	verifierStimmeHoechstensAlt int64 = 14 * 86400
	// Ein beendeter Eintrag (kein Gruender) faellt aus dem Register, wenn
	// sein Ende so lange vor einer neuen Aenderung liegt. Laenger fragt
	// niemand mehr danach: Bescheinigungen gelten hoechstens 7 Tage
	// (erneuerungHoechstensAlt), Stimmen 14. Sonst fuellten Ehemalige das
	// Register ueber die Jahre, bis niemand mehr aufgenommen werden kann.
	verifierEndeAufbewahren int64 = 30 * 86400
	verifierKoerperMax            = 256 << 10
)

// verifierRegisterAbUnix: Stichtag (math.MaxInt64 = schlafend).
// TestVerifierRegister_NieVorRegisterLeser erzwingt: nie vor registerLeserAb.
const verifierRegisterAbUnix int64 = math.MaxInt64

// verifierRegisterOverride: nur fuer Tests (0 = der Stichtag oben).
var verifierRegisterOverride atomic.Int64

// verifierGruender: die Menschen, die am Stichtag Pruefer sind -- ohne
// Abstimmung, weil es niemanden gibt, der abstimmen koennte. Heute der eine
// Betreiber (Validator-Bindung von C1, /api/validators). Jeder weitere kommt
// nur durch Abstimmung hinzu; auch ein Gruender kann austreten oder mit zwei
// Dritteln der uebrigen entzogen werden.
var verifierGruender = []string{
	"0x0be8b961cbf6564bd1931b0803d35c0659e0d016",
}

func verifierRegisterAb() int64 {
	if o := verifierRegisterOverride.Load(); o != 0 {
		return o
	}
	return verifierRegisterAbUnix
}

func verifierRegisterAktiv(t int64) bool { return t >= verifierRegisterAb() }

// VerifierZulassung: was eine verifier_zulassung traegt.
type VerifierZulassung struct {
	Art     string           `json:"art"`
	Stimmen []VerifierStimme `json:"stimmen"`
}

// VerifierStimme: eine Bescheinigung mit eigener Stimmzeit.
type VerifierStimme struct {
	Zeit          int64                      `json:"zeit"`
	Bescheinigung Lebendigkeitsbescheinigung `json:"bescheinigung"`
}

// verifierNachricht: was jeder Abstimmende unterschreibt.
func verifierNachricht(art, betroffener string) func(wallet string, issuedAt int64) string {
	b := strings.ToLower(strings.TrimSpace(betroffener))
	return func(_ string, stimmzeit int64) string {
		return fmt.Sprintf("%s|chain:%d|%s|%s|%d", verifierDomain, eingefroreneChainID, art, b, stimmzeit)
	}
}

// ------------------------------------------------------------ Register

// verifierEintrag: die letzte Zeitspanne eines Menschen. Bis 0 = offen.
// Zuletzt: issued_at der letzten angewandten Aenderung (gegen Wiederholung).
type verifierEintrag struct {
	Mensch  string `json:"m"`
	Seit    int64  `json:"seit"`
	Bis     int64  `json:"bis,omitempty"`
	Zuletzt int64  `json:"z"`
}

// verifierRegister: Eintraege nach Mensch sortiert. Unveraenderlich: eine
// Aenderung baut ein neues (cs.verifierReg ist ein atomarer Zeiger, lesbar
// ohne cs.mu).
type verifierRegister struct {
	Eintraege []verifierEintrag `json:"e,omitempty"`
}

func (r *verifierRegister) leer() bool { return r == nil || len(r.Eintraege) == 0 }

func (r *verifierRegister) kodiert() string {
	if r == nil {
		return `{}`
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func (r *verifierRegister) eintrag(m string) (verifierEintrag, bool) {
	if r == nil {
		return verifierEintrag{}, false
	}
	i := sort.Search(len(r.Eintraege), func(i int) bool { return r.Eintraege[i].Mensch >= m })
	if i < len(r.Eintraege) && r.Eintraege[i].Mensch == m {
		return r.Eintraege[i], true
	}
	return verifierEintrag{}, false
}

func verifierIstGruender(m string) bool {
	for _, g := range verifierGruender {
		if g == m {
			return true
		}
	}
	return false
}

// mitglied: ist m zur Zeit t Pruefer? Vor dem Stichtag niemand (die Frage
// stellt dann niemand: verifierZugelassen antwortet vorher "ja").
func (r *verifierRegister) mitglied(m string, t int64) bool {
	m = strings.ToLower(strings.TrimSpace(m))
	if !verifierRegisterAktiv(t) {
		return false
	}
	if e, ok := r.eintrag(m); ok {
		return e.Seit <= t && (e.Bis == 0 || t < e.Bis)
	}
	return verifierIstGruender(m)
}

// mitgliederZu: alle Pruefer zur Zeit t, sortiert.
func (r *verifierRegister) mitgliederZu(t int64) []string {
	if !verifierRegisterAktiv(t) {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	if r != nil {
		for _, e := range r.Eintraege {
			seen[e.Mensch] = true
			if e.Seit <= t && (e.Bis == 0 || t < e.Bis) {
				out = append(out, e.Mensch)
			}
		}
	}
	for _, g := range verifierGruender {
		if !seen[g] {
			out = append(out, g)
		}
	}
	sort.Strings(out)
	return out
}

// verifierVeraltet: faellt x bei einer Aenderung zu zeit aus dem Register?
// Nur beendete Eintraege, nie ein Gruender (ohne Eintrag waere er wieder
// Pruefer).
func verifierVeraltet(x verifierEintrag, zeit int64) bool {
	return x.Bis != 0 && x.Bis < zeit-verifierEndeAufbewahren && !verifierIstGruender(x.Mensch)
}

// mitSetzen: ein neues Register mit e (ersetzt den Eintrag desselben
// Menschen); veraltete Eintraege (verifierVeraltet zu e.Zuletzt) fallen weg.
func (r *verifierRegister) mitSetzen(e verifierEintrag) *verifierRegister {
	neu := &verifierRegister{}
	if r != nil {
		for _, x := range r.Eintraege {
			if x.Mensch != e.Mensch && !verifierVeraltet(x, e.Zuletzt) {
				neu.Eintraege = append(neu.Eintraege, x)
			}
		}
	}
	neu.Eintraege = append(neu.Eintraege, e)
	sort.Slice(neu.Eintraege, func(i, j int) bool { return neu.Eintraege[i].Mensch < neu.Eintraege[j].Mensch })
	return neu
}

// verifierRegisterLesen: der Wert aus chain_config oder einem Snapshot.
// Streng: was keine Kette erzeugt haben kann, wird abgewiesen.
func verifierRegisterLesen(s string) (*verifierRegister, error) {
	if s == "" {
		return &verifierRegister{}, nil
	}
	var r verifierRegister
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("Pruefer-Register nicht lesbar: %w", err)
	}
	if len(r.Eintraege) > verifierHoechstens {
		return nil, fmt.Errorf("Pruefer-Register: %d Eintraege, hoechstens %d", len(r.Eintraege), verifierHoechstens)
	}
	for i, e := range r.Eintraege {
		if !kanonischeAdresse(e.Mensch) || e.Seit <= 0 || e.Bis < 0 || (e.Bis != 0 && e.Bis < e.Seit) || e.Zuletzt <= 0 ||
			(i > 0 && e.Mensch <= r.Eintraege[i-1].Mensch) {
			return nil, fmt.Errorf("Pruefer-Register: Eintrag %d ungueltig", i)
		}
	}
	if r.kodiert() != s {
		return nil, errors.New("Pruefer-Register nicht in kanonischer Schreibweise")
	}
	return &r, nil
}

// verifierRegisterJetzt: das Register, ohne Sperre lesbar.
func (cs *ChainState) verifierRegisterJetzt() *verifierRegister {
	if r := cs.verifierReg.Load(); r != nil {
		return r
	}
	return &verifierRegister{}
}

// verifierRegisterLaden: beim Start und nach einem Snapshot-Import. Ein
// unlesbarer Wert haelt den Knoten an, statt andere Pruefer zu zaehlen als
// alle anderen.
func (cs *ChainState) verifierRegisterLaden(ctx context.Context) error {
	if cs.db == nil {
		return nil
	}
	r, err := verifierRegisterLesen(cs.getConfigValueCtx(ctx, verifierKonfigSchluessel))
	if err != nil {
		return err
	}
	cs.verifierReg.Store(r)
	return nil
}

// verifierZugelassen: die zweite Bedingung der Coordinator-Zulassung. Vor dem
// Stichtag gilt sie als erfuellt (wie bisher zaehlt nur der
// Validator-Schluessel).
func (cs *ChainState) verifierZugelassen(m string, t int64) bool {
	if !verifierRegisterAktiv(t) {
		return true
	}
	return cs.verifierRegisterJetzt().mitglied(m, t)
}

// mitPruefern: eine Zulassung, die zusaetzlich das Pruefer-Register verlangt.
// Die Datenbank (Validator-Schluessel) zuerst: sie meldet Lesefehler.
func (cs *ChainState) mitPruefern(z CoordinatorZulassung) CoordinatorZulassung {
	return func(m string, t int64) (bool, error) {
		ok, err := z(m, t)
		if err != nil || !ok {
			return ok, err
		}
		return cs.verifierZugelassen(strings.ToLower(strings.TrimSpace(m)), t), nil
	}
}

// ------------------------------------------------------------ Urteil

type verifierUrteil struct {
	anwenden bool
	eintrag  verifierEintrag
	regel    string
	text     string
}

func verifierBefund(regel, format string, args ...interface{}) verifierUrteil {
	return verifierUrteil{regel: regel, text: fmt.Sprintf(format, args...)}
}

// verifierSchwelle: Stimmen, die bei n Stimmberechtigten noetig sind --
// zwei Drittel, aufgerundet, mindestens eine.
func verifierSchwelle(n int) int {
	s := (2*n + 2) / 3
	if s < 1 {
		s = 1
	}
	return s
}

// verifierUrteilLocked: darf tx zu blockUnix angewandt werden? Billig vor
// teuer; Fehler nur, wenn die Zulassung nicht lesbar war (dann geht der Block
// zurueck). Caller haelt cs.mu.
func (cs *ChainState) verifierUrteilLocked(ctx context.Context, tx *Transaction, blockUnix int64) (verifierUrteil, error) {
	if !verifierRegisterAktiv(blockUnix) {
		return verifierUrteil{}, nil
	}
	z := tx.Verifier
	if z == nil {
		return verifierBefund("verifier_form", "keine Zulassung in der Transaktion"), nil
	}
	betroffener := tx.Wallet
	if !kanonischeAdresse(betroffener) {
		return verifierBefund("verifier_form", "Betroffener nicht in kanonischer Schreibweise"), nil
	}
	if z.Art != verifierArtAufnahme && z.Art != verifierArtAustritt && z.Art != verifierArtEntzug {
		return verifierBefund("verifier_form", "Art %q unbekannt", z.Art), nil
	}
	if n := len(z.Stimmen); n < 1 || n > verifierStimmenHoechstens || (z.Art == verifierArtAustritt && n != 1) {
		return verifierBefund("verifier_form", "%d Stimmen fuer %s", n, z.Art), nil
	}
	zeit := tx.DistributionAt
	// Wie die Validator-Bindung: hoechstens nachweisHoechstensAlt vor dem
	// Block -- sonst haenge das Register an der Nachspielreihenfolge (Kopf).
	if zeit <= 0 || zeit < blockUnix-nachweisHoechstensAlt || zeit > sattAdd(blockUnix, nachweisHoechstensVoraus) {
		return verifierBefund("verifier_zeit", "bescheinigt %d, Blockzeit %d", zeit, blockUnix), nil
	}
	if !verifierRegisterAktiv(zeit) {
		return verifierBefund("verifier_zeit", "bescheinigt %d vor dem Stichtag", zeit), nil
	}
	reg := cs.verifierRegisterJetzt()
	vorher, hatEintrag := reg.eintrag(betroffener)
	if hatEintrag && zeit <= vorher.Zuletzt {
		return verifierBefund("verifier_wiederholt", "bescheinigt %d, letzte Aenderung %d", zeit, vorher.Zuletzt), nil
	}
	wirkung := sattAdd(zeit, erzeugerFrist)
	// Stand zu issued_at UND zur Wirkung: wer zur Wirkung noch (oder schon)
	// Pruefer ist, wird nicht noch einmal aufgenommen; wer es zu issued_at
	// nicht ist, tritt nicht aus.
	istJetzt := reg.mitglied(betroffener, zeit)
	istDann := reg.mitglied(betroffener, wirkung)
	switch z.Art {
	case verifierArtAufnahme:
		if istJetzt || istDann {
			return verifierBefund("verifier_schon", "%s ist schon Pruefer", kurzAdresse(betroffener)), nil
		}
		if !hatEintrag {
			bleiben := 0
			for _, x := range reg.Eintraege {
				if !verifierVeraltet(x, zeit) {
					bleiben++
				}
			}
			if bleiben >= verifierHoechstens {
				return verifierBefund("verifier_voll", "%d Eintraege im Register, hoechstens %d", bleiben, verifierHoechstens), nil
			}
		}
	default:
		if !istJetzt || !istDann {
			return verifierBefund("verifier_kein_mitglied", "%s ist kein Pruefer", kurzAdresse(betroffener)), nil
		}
	}
	// Der Betroffene ist ein registrierter Mensch (Entscheidung 1). Erst hier,
	// nach den billigen Pruefungen -- das kann die Datenbank kosten.
	cs.ensureAccountLoadedCtx(ctx, betroffener)
	if acc, ok := cs.accounts.Get(betroffener); !ok || !acc.IsHuman {
		if z.Art == verifierArtAufnahme {
			return verifierBefund("verifier_kein_mensch", "%s ist kein registrierter Mensch", kurzAdresse(betroffener)), nil
		}
	}

	stimmberechtigt := map[string]bool{}
	for _, m := range reg.mitgliederZu(zeit) {
		stimmberechtigt[m] = true
	}
	noetig := 1
	switch z.Art {
	case verifierArtAufnahme:
		noetig = verifierSchwelle(len(stimmberechtigt))
	case verifierArtEntzug:
		delete(stimmberechtigt, betroffener)
		if len(stimmberechtigt) == 0 {
			return verifierBefund("verifier_quorum", "niemand ausser %s ist Pruefer -- nur Austritt moeglich", kurzAdresse(betroffener)), nil
		}
		noetig = verifierSchwelle(len(stimmberechtigt))
	}

	stand := func(m string) (bool, bool) {
		cs.ensureAccountLoadedCtx(ctx, m)
		acc, ok := cs.accounts.Get(m)
		return ok && acc.IsHuman, ok && acc.GrantStagedRest > 0
	}
	zulassung := cs.lebensnachweisZulassung(ctx)
	nachricht := verifierNachricht(z.Art, betroffener)
	// Beim Austritt unterschreibt der Betroffene selbst; bescheinigungPruefenFuer
	// lehnt "sich selbst bescheinigen" fuer die Wallet ab, die es bekommt --
	// hier steht dort darum eine Adresse, die kein Mensch ist.
	fuer := betroffener
	if z.Art == verifierArtAustritt {
		fuer = ubiPoolAddr
	}
	stimmen := map[string]bool{}
	for i := range z.Stimmen {
		s := &z.Stimmen[i]
		if s.Zeit <= 0 || s.Zeit > sattAdd(zeit, nachweisHoechstensVoraus) || zeit-s.Zeit > verifierStimmeHoechstensAlt {
			return verifierBefund("verifier_stimme_zeit", "Stimme %d von %d, bescheinigt %d", i+1, s.Zeit, zeit), nil
		}
		if hatEintrag && s.Zeit <= vorher.Zuletzt {
			return verifierBefund("verifier_stimme_zeit", "Stimme %d von %d liegt vor der letzten Aenderung %d", i+1, s.Zeit, vorher.Zuletzt), nil
		}
		// Wer nicht stimmberechtigt ist, faellt VOR jeder Unterschrift und
		// jedem Datenbankzugriff heraus (billig vor teuer): die Angabe des
		// Menschen ist hier nur eine Behauptung, geprueft wird sie unten.
		if m := strings.ToLower(strings.TrimSpace(s.Bescheinigung.Mensch)); z.Art == verifierArtAustritt && m != betroffener {
			return verifierBefund("verifier_stimme", "Austritt nur durch %s selbst", kurzAdresse(betroffener)), nil
		} else if z.Art != verifierArtAustritt && !stimmberechtigt[m] {
			return verifierBefund("verifier_stimme", "Stimme %d: %s ist zu %d nicht stimmberechtigt", i+1, kurzAdresse(m), zeit), nil
		}
		if err := bescheinigungPruefenFuer(nachricht, fuer, s.Zeit, &s.Bescheinigung, stand, zulassung); err != nil {
			if errors.Is(err, errZulassungNichtLesbar) {
				return verifierUrteil{}, err
			}
			return verifierBefund("verifier_stimme", "Stimme %d: %v", i+1, err), nil
		}
		// Stimmberechtigt (oben) ist, wer zu issued_at Pruefer ist -- nicht der
		// Betroffene, nicht ein Mensch, der erst spaeter dazukommt. Gezaehlt
		// wird der gepruefte Mensch der Bescheinigung.
		stimmen[s.Bescheinigung.Mensch] = true
	}
	if len(stimmen) < noetig {
		return verifierBefund("verifier_quorum", "%d verschiedene Pruefer, noetig %d von %d", len(stimmen), noetig, len(stimmberechtigt)), nil
	}

	e := verifierEintrag{Mensch: betroffener, Zuletzt: zeit}
	switch z.Art {
	case verifierArtAufnahme:
		e.Seit = wirkung
	default:
		e.Seit = vorher.Seit
		if !hatEintrag {
			// Gruender ohne Eintrag: Pruefer seit dem Stichtag.
			e.Seit = verifierRegisterAb()
		}
		e.Bis = wirkung
	}
	return verifierUrteil{anwenden: true, eintrag: e}, nil
}

// verifierSetzenLocked: das neue Register nach einer gueltigen Aenderung.
func (cs *ChainState) verifierSetzenLocked(ctx context.Context, e verifierEintrag) error {
	neu := cs.verifierRegisterJetzt().mitSetzen(e)
	if cs.db != nil {
		if err := cs.setConfigValueCtx(ctx, verifierKonfigSchluessel, neu.kodiert()); err != nil {
			return fmt.Errorf("verifier_zulassung: Register speichern: %w", err)
		}
	}
	cs.verifierReg.Store(neu)
	return nil
}

// applyVerifierZulassungLocked: beim Nachspielen. Ungueltig: nie angewandt,
// gezaehlt, im strengen Modus Fehler. Caller haelt cs.mu.
func (cs *ChainState) applyVerifierZulassungLocked(ctx context.Context, tx *Transaction, blockUnix int64) error {
	u, err := cs.verifierUrteilLocked(ctx, tx, blockUnix)
	if err != nil {
		return err
	}
	if u.regel != "" {
		return nachrechnenAbweichung(u.regel, blockUnix, "%s", u.text)
	}
	if !u.anwenden {
		return nil
	}
	return cs.verifierSetzenLocked(ctx, u.eintrag)
}

// VerifierStand: fuer /api/verifier.
func (cs *ChainState) VerifierStand(jetzt int64) map[string]interface{} {
	reg := cs.verifierRegisterJetzt()
	m := map[string]interface{}{
		"aktiv":       verifierRegisterAktiv(jetzt),
		"gruender":    verifierGruender,
		"hoechstens":  verifierHoechstens,
		"frist_s":     erzeugerFrist,
		"schwelle":    "zwei Drittel der Pruefer zur Zeit der Abstimmung, aufgerundet",
		"stimme_tage": verifierStimmeHoechstensAlt / 86400,
	}
	if verifierRegisterAb() != math.MaxInt64 {
		m["ab"] = verifierRegisterAb()
	}
	if verifierRegisterAktiv(jetzt) {
		mit := reg.mitgliederZu(jetzt)
		m["pruefer"] = mit
		m["aufnahme_braucht"] = verifierSchwelle(len(mit))
	}
	if !reg.leer() {
		m["eintraege"] = reg.Eintraege
	}
	return m
}

// ------------------------------------------------------------ API

// verifierVonPraefix: Zaehler weitergeleiteter Anfragen (nachweisGrenze).
// Weitergeleitet wird diese Art nicht (weiterleitungUnterschreiben): die
// Stimmen sprengen weiterleitungKoerperMax schon bei wenigen Pruefern, und
// die Grenze wird nicht gelockert. Eingereicht wird beim Leiter.
const verifierVonPraefix = "verifier-von:"

var verifierGrenzArt = nachweisGrenzArt{
	aktiv:      verifierRegisterAktiv,
	inaktiv:    "verifier register not active yet",
	direkt:     "verifier:",
	pruefung:   "verifier-pruefung:",
	von:        verifierVonPraefix,
	zuVieleTxt: "too many verifier requests from this address -- please retry in a minute",
}

func (a *APIServer) verifierGrenze(next http.HandlerFunc) http.HandlerFunc {
	return a.nachweisGrenze(verifierGrenzArt, next)
}

type verifierAbgelehnt struct{ text string }

func (e *verifierAbgelehnt) Error() string { return e.text }

var errVerifierOhneWirkung = errors.New("ohne Wirkung")

// handleVerifier: GET den Stand des Pruefer-Registers; POST eine Aufnahme,
// einen Austritt oder einen Entzug:
//
//	{"art", "wallet", "issued_at", "stimmen": [{"zeit", "bescheinigung":
//	  {"public_key", "signature", "mensch", "mensch_sig", "schluessel_sig"}}]}
//
// Die Bescheinigung traegt ihre Bindung selbst (wie im Block); geprueft wird
// alles von verifierUrteilLocked, bei der Annahme wie beim Nachspielen.
// Angenommen nur beim Leiter (das Register gehoert keinem Konto).
func (a *APIServer) handleVerifier(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	now := time.Now().Unix()
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(a.state.VerifierStand(now))
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "GET or POST only", http.StatusMethodNotAllowed)
		return
	}
	if !verifierRegisterAktiv(now) {
		jsonError(w, "verifier register not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Art      string           `json:"art"`
		Wallet   string           `json:"wallet"`
		IssuedAt int64            `json:"issued_at"`
		Stimmen  []VerifierStimme `json:"stimmen"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, verifierKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if n := len(req.Stimmen); n < 1 || n > verifierStimmenHoechstens {
		jsonError(w, fmt.Sprintf("stimmen: 1 to %d required", verifierStimmenHoechstens), http.StatusBadRequest)
		return
	}
	if req.IssuedAt <= 0 || now-req.IssuedAt > nachweisHoechstensAlt || req.IssuedAt-now > nachweisHoechstensVoraus {
		jsonError(w, "issued_at expired or from the future", http.StatusBadRequest)
		return
	}
	// Unterschriften VOR jedem Zugriff auf Zustand oder Datenbank: ein
	// Haufen Muell soll nichts kosten ausser Ed25519 (Kopf, billig vor teuer).
	nachricht := verifierNachricht(req.Art, req.Wallet)
	for i := range req.Stimmen {
		s := &req.Stimmen[i]
		s.Bescheinigung.PublicKey = strings.ToLower(strings.TrimSpace(s.Bescheinigung.PublicKey))
		s.Bescheinigung.Signature = ed25519SigNormal(s.Bescheinigung.Signature)
		s.Bescheinigung.Mensch = strings.ToLower(strings.TrimSpace(s.Bescheinigung.Mensch))
		s.Bescheinigung.MenschSig = kanonischeSignaturVersuch(s.Bescheinigung.MenschSig)
		s.Bescheinigung.SchluesselSig = ed25519SigNormal(s.Bescheinigung.SchluesselSig)
		if !ed25519PruefenStreng(s.Bescheinigung.PublicKey, s.Bescheinigung.Signature, []byte(nachricht("", s.Zeit))) {
			jsonError(w, fmt.Sprintf("invalid vote %d: does not match art, wallet and time", i+1), http.StatusForbidden)
			return
		}
	}
	tx := Transaction{Type: verifierTxArt, Wallet: strings.ToLower(strings.TrimSpace(req.Wallet)), DistributionAt: req.IssuedAt,
		Verifier: &VerifierZulassung{Art: req.Art, Stimmen: req.Stimmen}}
	if err := a.state.annahmeBeginnenLeiter(); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer a.state.annahmeEnde()
	if err := a.state.runAtomicWithOutbox(nil, false, func(ctx context.Context) (Transaction, error) {
		u, err := a.state.verifierUrteilLocked(ctx, &tx, now)
		if err != nil {
			return Transaction{}, err
		}
		if u.regel != "" {
			return Transaction{}, &verifierAbgelehnt{text: u.regel + ": " + u.text}
		}
		if !u.anwenden {
			return Transaction{}, errVerifierOhneWirkung
		}
		if err := a.state.verifierSetzenLocked(ctx, u.eintrag); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	}); err != nil {
		var abgelehnt *verifierAbgelehnt
		switch {
		case errors.As(err, &abgelehnt):
			jsonError(w, "verifier change rejected: "+abgelehnt.text, http.StatusConflict)
		case errors.Is(err, errVerifierOhneWirkung):
			jsonError(w, "verifier change has no effect", http.StatusConflict)
		default:
			fmt.Printf("[API] verifier change failed: %v\n", err)
			jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		}
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "verifier": a.state.VerifierStand(now)})
}
