package keeper

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// NUR EIN KNOTEN NIMMT UEBERWEISUNGEN AN.
//
// # DER FEHLER, DEN DAS VERHINDERT
//
// Reproduziert in zwei_produzenten_realdb_test.go, mit dem Fingerabdruck der
// Boxen vom 15.09.2026: einzelne Konten weichen ab, die Summen stimmen, kein
// Block fehlt, kein Rollback, kein Flush-Fehler.
//
// Der Mechanismus. Jeder Knoten wendet seine eigenen Ueberweisungen bei der
// ANNAHME an -- also bevor ein Block ihre Reihenfolge festlegt -- und die des
// anderen beim NACHSPIELEN. Innerhalb einer Runde sieht keiner die Annahmen
// des anderen. Solange kein Konto leerlaeuft, ist das folgenlos: Addition
// kennt keine Reihenfolge. Laeuft eines leer, ist es das nicht mehr. Der
// annehmende Knoten hat gegen SEINE Sicht geprueft und angewandt; der
// nachspielende prueft ein ZWEITES Mal gegen seine eigene und ueberspringt,
// wenn es dort nicht reicht (ErrZustandLehntAb). Ab da sind sich die beiden
// ueber dieses Konto dauerhaft uneins.
//
// Die Bedingung ist also genau: DASSELBE KONTO wird auf ZWEI Knoten
// gleichzeitig belastet. Nimmt nur einer an, kann es nicht eintreten -- jede
// Belastung eines Kontos geht dann durch eine einzige Sicht, und der
// nachspielende Knoten hat, wenn er den Block anwendet, alle Vorgaenger
// bereits angewandt (DAG-Regel). Sein Kontostand an dieser Stelle ist damit
// mindestens so hoch wie der, gegen den der annehmende geprueft hat. Es gibt
// nichts zu ueberspringen.
//
// # WAS GESPERRT IST, UND WARUM ALLES DAVON
//
// "Jede Belastung" heisst jede. Der erste Anlauf sperrte nur die beiden
// Ueberweisungspfade -- und liess Swap, Liquiditaet und Faucet offen, die
// Konten UND die Tokenomics-Toepfe bei der Annahme genauso belasten und den
// Mechanismus damit unveraendert reproduziert haetten. Eine Sperre, die eine
// Bedingung "unmoeglich" nennt und dabei vier Tueren offen laesst, ist
// schlimmer als keine: sie erzeugt Vertrauen, das sie nicht traegt.
//
// Gesperrt sind deshalb alle sechs annehmenden Pfade: TransferAtomic,
// TransferWithV7FeeAtomic, SwapAtomic, AddLiquidityAtomic,
// RemoveLiquidityAtomic, ClaimTUsdFaucetAtomic.
//
// Nicht gesperrt ist das NACHSPIELEN: ein nur lesender Knoten wendet die
// Bloecke des Partners unveraendert an -- er soll ja folgen, nur nicht selbst
// annehmen. Ebenfalls nicht gesperrt ist die Registrierung: sie praegt gegen
// einen Nullifier, dessen Einmaligkeit eine Datenbanksperre traegt, und der
// Herkunftszwang bindet sie ohnehin an den Knoten, der den Beweis ausstellte.
//
// # WO DIE SPERRE SITZT
//
// Fuer den RPC-Weg zusaetzlich GANZ VORN in sendRawTransaction, vor
// ReserveNonce. Dazwischen liegt ein dauerhafter Schreibvorgang in die
// evm_nonces dieses Knotens: eine Wallet, die an den nur lesenden Knoten
// geraet, haette dort sonst eine Nonce verbrannt und bekaeme fortan eine
// Nonce zurueck, die der annehmende Knoten als "nonce too high" abweist --
// dauerhaft, denn ein ReleaseNonce gibt es nicht. Eine Sperre, die den
// Menschen schlechter stellt als gar keine Sperre, waere die schlechteste
// Art von Fix.
//
// # WARUM NICHT DIE WURZEL
//
// Sauber waere: Zustand aendert sich ausschliesslich beim Anwenden eines
// Blocks, die Annahme reiht nur ein. Das ist die uebliche Bauform einer Kette
// und macht nebenlaeufige Annahme auf beliebig vielen Knoten sicher. Es ist
// aber ein Umbau des gesamten heissen Pfades -- WAL-Schnellpfad, Buendler,
// Shard-Sperren, alles, was hier ueber Monate gemessen und optimiert wurde --
// und nichts, was man in der Woche vor einem Start unterschiebt. Diese Sperre
// ist kein Ersatz dafuer; sie macht die Bedingung unmoeglich, unter der der
// Fehler ueberhaupt entsteht, und kostet dafuer etwas Erreichbarkeit.
//
// # WAS SIE KOSTET
//
// Der Ausweichknoten traegt weiter API, RPC-Lesungen und Registrierung. Was er
// ablehnt, sind ausschliesslich Ueberweisungen. Faellt der annehmende Knoten
// aus, muss der Betreiber die Rolle umstellen -- das ist bewusst ein Handgriff
// und keine Wahl: eine automatische Uebernahme kann bei einer Netztrennung zu
// zwei Annehmenden fuehren, und das ist genau der Zustand, den diese Sperre
// beseitigt. Eine falsche Automatik waere hier schlimmer als gar keine.
//
// # VOREINSTELLUNG
//
// Ohne Konfiguration aendert sich nichts: jeder Knoten nimmt an, wie bisher.
// Ein Tor, das beim Einschalten die halbe Kette stillegen koennte, gehoert
// nicht per Vorgabe scharf -- der Betreiber schaltet es, wenn er weiss, welche
// Box welche Rolle hat. Empfohlen fuer die Beta: auf der Box, auf die die App
// zeigt, ANNAHME_ROLLE=annehmend (oder nichts), auf der anderen
// ANNAHME_ROLLE=nur_lesend.
const annahmeRolleEnv = "ANNAHME_ROLLE"

// abgelehnteUeberweisungen zaehlt, was diese Sperre abgewiesen hat -- sichtbar
// in /api/health/combined. Eine Sperre, die niemand zaehlt, merkt man erst,
// wenn sich jemand beschwert.
var abgelehnteUeberweisungen atomic.Int64

// Die Rolle haengt am KNOTEN, nicht am Prozess.
//
// Gelesen wird die Umgebungsvariable einmal, beim Bau des Knotens; danach
// steht die Rolle in cs.nurLesend. Das ist nicht nur sauberer -- es ist die
// Voraussetzung dafuer, dass zwei Knoten mit VERSCHIEDENEN Rollen in einem
// Prozess gebaut werden koennen, und genau das braucht der Nachweis, dass
// diese Sperre die Divergenz beseitigt (zwei_produzenten_realdb_test.go).
// Eine Sperre, deren Wirkung man nicht messen kann, ist eine Behauptung.

// annahmeRolleAusUmgebung liest die Vorgabe fuer einen neu gebauten Knoten.
func annahmeRolleAusUmgebung() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(annahmeRolleEnv)), "nur_lesend")
}

// nimmtUeberweisungenAn sagt, ob dieser Knoten Ueberweisungen annehmen darf.
//
// Mit rotierendem Leiter (leitung.go) nur, solange er der Leiter ist und
// seine Lease traegt. nur_lesend bleibt eine harte Sperre darueber: ein so
// eingestellter Knoten nimmt nie an, auch nicht als gewaehlter Leiter.
func (cs *ChainState) nimmtUeberweisungenAn() bool {
	if cs.nurLesend.Load() {
		return false
	}
	if l := cs.leitung.Load(); l != nil {
		return l.DarfAnnehmen(time.Now())
	}
	return true
}

// annahmeBeginnen ist das Tor fuer die sechs annehmenden Pfade, MIT
// Zaehlung: erst zaehlen, dann pruefen. Schliesst die Leitung das Tor fuer
// eine Uebergabe, sieht sie in annahmenLaufend jede Annahme, die schon
// durch ist -- und jede spaetere prueft danach und kehrt um. Erst bei 0
// (und leerem WAL und Ausgangskorb) wird uebergeben.
func (cs *ChainState) annahmeBeginnen() error {
	cs.annahmenLaufend.Add(1)
	if err := cs.pruefeAnnahmeTor(); err != nil {
		cs.annahmenLaufend.Add(-1)
		return err
	}
	return nil
}

func (cs *ChainState) annahmeEnde() { cs.annahmenLaufend.Add(-1) }

// SetzeNurLesend stellt die Rolle zur Laufzeit um -- fuer den Fall, dass der
// annehmende Knoten ausfaellt und ein Mensch die Rolle umhaengt, und fuer die
// Tests, die zwei Knoten mit verschiedenen Rollen in einem Prozess bauen.
func (cs *ChainState) SetzeNurLesend(nurLesend bool) {
	cs.nurLesend.Store(nurLesend)
}

// ErrNurLesend ist die Antwort auf eine Ueberweisung an einen Knoten, der
// dafuer nicht zustaendig ist. Der Text nennt den Grund und den Ausweg, weil
// er bei einem Menschen in der App landet.
var ErrNurLesend = fmt.Errorf(
	"dieser Knoten nimmt keine Ueberweisungen an (%s=nur_lesend) -- er traegt Lesungen und "+
		"Registrierung weiter; fuer Ueberweisungen den annehmenden Knoten verwenden", annahmeRolleEnv)

// pruefeAnnahmeTor gibt ErrNurLesend zurueck, wenn dieser Knoten nicht
// annehmen darf, und zaehlt die Ablehnung.
func (cs *ChainState) pruefeAnnahmeTor() error {
	if cs.nimmtUeberweisungenAn() {
		return nil
	}
	abgelehnteUeberweisungen.Add(1)
	if !cs.nurLesend.Load() && cs.leitung.Load() != nil {
		return ErrNichtLeiter
	}
	return ErrNurLesend
}

// ErrNichtLeiter: mit rotierendem Leiter nimmt gerade ein anderer an. Die
// Anfrage wird normalerweise weitergeleitet; diese Antwort kommt nur, wenn
// das nicht ging (Leiterwechsel laeuft, Leiter kurz nicht erreichbar).
// -32005 in RPC: wiederholbar.
var ErrNichtLeiter = fmt.Errorf("dieser Knoten ist gerade nicht der Leiter und konnte nicht weiterleiten " +
	"(Leiterwechsel laeuft) -- bitte in wenigen Sekunden erneut versuchen")

// AnnahmeTorStand zeigt die Rolle in /api/health/combined.
func (cs *ChainState) AnnahmeTorStand() map[string]interface{} {
	return map[string]interface{}{
		"nimmt_an":          cs.nimmtUeberweisungenAn(),
		"abgelehnt":         abgelehnteUeberweisungen.Load(),
		"umgebungsvariable": annahmeRolleEnv,
		"bedeutung": "Nimmt dieser Knoten Ueberweisungen an? Nehmen ZWEI Knoten gleichzeitig " +
			"Ueberweisungen desselben Kontos an, koennen ihre Kontenstaende dauerhaft " +
			"auseinanderlaufen, sobald ein Konto leerlaeuft -- belegt in " +
			"zwei_produzenten_realdb_test.go. Mit " + annahmeRolleEnv + "=nur_lesend traegt " +
			"dieser Knoten Lesungen und Registrierung weiter und lehnt nur Ueberweisungen ab. " +
			"Faellt der annehmende Knoten aus, stellt ein Mensch die Rolle um -- eine " +
			"automatische Uebernahme koennte bei einer Netztrennung zwei Annehmende erzeugen, " +
			"also genau den Zustand, den die Sperre beseitigt.",
	}
}
