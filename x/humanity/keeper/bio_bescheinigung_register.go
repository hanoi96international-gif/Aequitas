package keeper

// K1 MIT DEN SCHLUESSELN DER PRUEFER (B1b, docs/DEZENTRALITAET.md Schritt 2).
//
// # WARUM
//
// Bis hierher nahm die Bescheinigung im Konsens (bio_bescheinigung_konsens.go)
// nur die Schluessel aus der Genesis an. Ein neuer Pruefer -- mit zwei Dritteln
// der Stimmen aufgenommen (verifier_register.go) -- konnte weder als
// Coordinator binden noch als Vergleichsdienst bezeugen, ohne dass jemand eine
// neue Genesis-Fassung schreibt. Und eine einzige Unterschrift eines
// unbekannten Schluessels machte jede Registrierung ungueltig.
//
// # WAS AB bescheinigungRegisterAb GILT
//
// Eine Registrierung darf zu jedem Schluessel, der kein Genesis-Schluessel ist,
// seine Bindung mitbringen (Feld bindungen, im Prove-Rumpf "v3k:"):
//
//	schluessel, mensch, mensch_sig (EIP-191 "Aequitas: authorize coordinator
//	<schluessel> chain:<id>"), schluessel_sig (Ed25519 "Aequitas: coordinator
//	key for human <mensch> chain:<id>")
//
// -- dieselbe Bindung wie bei jeder Stimme und jeder Erneuerung. Jeder Knoten
// prueft sie selbst, und der Mensch muss zur Zeit der Bescheinigung im
// Pruefer-Register stehen. Ein solcher Schluessel darf binden und bezeugen.
//
// Gezaehlt werden dann MENSCHEN, nicht Schluessel: alle Genesis-Schluessel und
// alle Schluessel der Gruender sind ein Mensch (genesisMensch), jeder weitere
// Pruefer einer. Wer zwei Schluessel hat, zaehlt einmal. "Mindestens ein
// Bezeuger, der nicht gebunden hat" bleibt je Schluessel: ein Vergleichsdienst
// muss den Menschen gesehen haben.
//
// # REIHENFOLGE (warum jeder Knoten dasselbe sagt)
//
// Ob m zur Zeit t Pruefer ist, steht fest, sobald t + erzeugerFrist vorbei ist
// (verifier_register.go, REIHENFOLGE) -- mit einer Ausnahme: eine
// Wiederaufnahme ueberschreibt den alten Eintrag. Darum:
//
//   - eine Registrierung mit Pruefer-Schluesseln muss spaetestens
//     registerBindungFenster nach ihrer Bescheinigung im Block stehen, und
//   - eine Wiederaufnahme ist fruehestens verifierWiederaufnahmePause nach dem
//     Ende moeglich (verifierUrteilLocked).
//
// Damit liegt jeder Block, der eine solche Registrierung traegt, vor jeder
// Wiederaufnahme, die ihren Pruefer betreffen koennte -- weit jenseits des
// Fensters, in dem Geschwisterbloecke in verschiedener Reihenfolge ankommen.
//
// # GRENZEN
//
// Hoechstens bescheinigungMaxSignaturen Bindungen, je Schluessel eine, keine
// fuer einen Genesis-Schluessel, jede muss zu einer Unterschrift gehoeren.
// Vor dem Stichtag ist jede Bindung ein Fehler (alle Knoten sagen dasselbe).

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
)

// bescheinigungRegisterAbUnix: 08.11.2026 18:00 UTC, auf Anweisung des
// Betreibers ("alles umsetzen", 07.11.2026). Der Deploy haelt an, wenn er
// nicht mindestens 30 Minuten vorher laeuft
// (.github/neustart/b1b-stichtag-nicht-verpasst.sh).
// TestB1b_NieVorRegister erzwingt: nie vor verifierRegisterAb.
const bescheinigungRegisterAbUnix int64 = 1794160800

// bescheinigungRegisterOverride: nur fuer Tests (0 = Stichtag oben).
var bescheinigungRegisterOverride atomic.Int64

// registerBindungFenster: so lange nach ihrer Bescheinigung darf eine
// Registrierung mit Pruefer-Schluesseln im Block stehen.
const registerBindungFenster int64 = 86400

// verifierWiederaufnahmePause: so lange nach dem Ende ist keine Wiederaufnahme
// moeglich (ab bescheinigungRegisterAb).
const verifierWiederaufnahmePause = 2 * registerBindungFenster

// genesisMensch: unter diesem Namen zaehlen die Genesis-Schluessel und die
// Gruender -- ein Mensch, egal wie viele Schluessel.
const genesisMensch = "genesis"

func bescheinigungRegisterAb() int64 {
	if o := bescheinigungRegisterOverride.Load(); o != 0 {
		return o
	}
	return bescheinigungRegisterAbUnix
}

func bescheinigungRegisterAktiv(t int64) bool { return t >= bescheinigungRegisterAb() }

// BescheinigungsBindung: die Bindung eines Pruefer-Schluessels an seinen
// Menschen, in der Registrierung mitgeschickt.
type BescheinigungsBindung struct {
	PublicKey     string `json:"public_key"`
	Mensch        string `json:"mensch"`
	MenschSig     string `json:"mensch_sig"`
	SchluesselSig string `json:"schluessel_sig"`
}

// leseBescheinigungsBindung: "<schluessel>.<mensch>.<mensch_sig>.<schluessel_sig>"
// in der einen Schreibweise; alles andere ist keine Bindung.
func leseBescheinigungsBindung(roh string) (BescheinigungsBindung, bool) {
	t := strings.Split(strings.TrimSpace(roh), ".")
	if len(t) != 4 {
		return BescheinigungsBindung{}, false
	}
	b := BescheinigungsBindung{PublicKey: t[0], Mensch: t[1], MenschSig: t[2], SchluesselSig: t[3]}
	if len(b.PublicKey) != 64 || !kleinHex(b.PublicKey) || !kanonischeAdresse(b.Mensch) ||
		!kanonischeSignatur(b.MenschSig) || len(b.SchluesselSig) != 128 || !kleinHex(b.SchluesselSig) {
		return BescheinigungsBindung{}, false
	}
	return b, true
}

// BindungsText: die Form im Prove-Rumpf (ohne Praefix).
func (b BescheinigungsBindung) BindungsText() string {
	return b.PublicKey + "." + b.Mensch + "." + b.MenschSig + "." + b.SchluesselSig
}

func sortierteSchluessel(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// registerSchluesselPruefen: die mitgebrachten Bindungen, geprueft; Ergebnis
// Schluessel -> Mensch (genesisMensch fuer Gruender). Die Schluessel kommen
// in pubs. Billig vor teuer: Form, Stichtag, Fenster, Doppelte, dann
// Unterschriften, dann das Register.
func registerSchluesselPruefen(b *BioBescheinigung, blockZeit int64, pruefer func(m string, t int64) bool, pubs map[string]ed25519.PublicKey) (map[string]string, error) {
	if len(b.Bindungen) == 0 {
		return nil, nil
	}
	if !bescheinigungRegisterAktiv(blockZeit) {
		return nil, errors.New("bescheinigung: Bindungen von Pruefer-Schluesseln erst ab dem Stichtag")
	}
	if pruefer == nil {
		return nil, errors.New("bescheinigung: Pruefer-Register nicht lesbar")
	}
	if len(b.Bindungen) > bescheinigungMaxSignaturen {
		return nil, fmt.Errorf("bescheinigung: %d Bindungen, hoechstens %d", len(b.Bindungen), bescheinigungMaxSignaturen)
	}
	if blockZeit > sattAdd(b.Zeit, registerBindungFenster) {
		return nil, fmt.Errorf("bescheinigung: mit Pruefer-Schluesseln hoechstens %d s nach der Bescheinigung im Block", registerBindungFenster)
	}
	aus := map[string]string{}
	for i, x := range b.Bindungen {
		if _, ok := leseBescheinigungsBindung(x.BindungsText()); !ok {
			return nil, fmt.Errorf("bescheinigung: Bindung %d nicht in ihrer einen Schreibweise", i+1)
		}
		if _, schon := pubs[x.PublicKey]; schon {
			return nil, fmt.Errorf("bescheinigung: Bindung %d fuer einen Schluessel der Genesis oder doppelt", i+1)
		}
		if !ed25519HexTauglich(x.PublicKey) {
			return nil, fmt.Errorf("bescheinigung: Bindung %d: kein tauglicher Ed25519-Schluessel", i+1)
		}
		pub, _ := hex.DecodeString(x.PublicKey)
		if !bindungGueltig(x.PublicKey, x.Mensch, x.MenschSig, x.SchluesselSig) {
			return nil, fmt.Errorf("bescheinigung: Bindung %d: Freigabe oder Besitznachweis falsch", i+1)
		}
		if !pruefer(x.Mensch, b.Zeit) {
			return nil, fmt.Errorf("bescheinigung: Bindung %d: %s ist zu %d kein Pruefer", i+1, kurzAdresse(x.Mensch), b.Zeit)
		}
		pubs[x.PublicKey] = ed25519.PublicKey(pub)
		if verifierIstGruender(x.Mensch) {
			aus[x.PublicKey] = genesisMensch
		} else {
			aus[x.PublicKey] = x.Mensch
		}
	}
	return aus, nil
}
