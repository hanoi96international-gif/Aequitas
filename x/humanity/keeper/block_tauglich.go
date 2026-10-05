package keeper

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

// DIE BLOCKZEIT, ZU DER JEDER ANDERE KNOTEN DEN BLOCK ANNIMMT (05.10.2026).
//
// Auftraege mit Nachweis (auftrag_nachweis.go: Tausch, Liquiditaet, Faucet,
// Vormund, Lebenszeichen, Unternehmen, Vorbehalt, Validator-Bindung) tragen
// den Zeitpunkt ihrer Unterschrift, und jeder Nachspielende prueft ihn gegen
// die BLOCKZEIT: hoechstens eine Stunde alt, hoechstens fuenf Minuten voraus.
// Ein Verstoss macht den ganzen Block ungueltig. Aehnlich haengen Erneuerung,
// V8-Registrierung, Umlauf und Rundenmarke an der Blockzeit.
//
// Der Erzeuger stempelte bisher jeden Block mit seiner Uhr. Der Ausgang
// ueberlebt aber einen Neustart (pending_txs): ein Absturz in der Nacht, ein
// Neustart am Morgen, und der erste Block traegt einen Tausch von vor sieben
// Stunden mit der Zeit von jetzt. Jeder andere Knoten weist ihn ab -- und mit
// ihm jeden Block, der auf ihm aufbaut. Mit einem einzigen Erzeuger steht die
// Kette.
//
// WEGLASSEN IST KEINE LOESUNG (Sicherheitspruefung #297): der Erzeuger hat
// den Auftrag bei der Annahme schon angewendet. Laesst er ihn weg, weicht
// sein Zustand dauerhaft ab -- und ein spaeterer Auftrag, der darauf aufbaut
// (ein Tausch mit dem tUSD aus dem weggelassenen Faucet), haelt die Kette
// trotzdem an.
//
// Stattdessen waehlt der Erzeuger die Blockzeit: jetzt, wenn alle Auftraege
// des Blocks dazu passen (der Normalfall); sonst die SPAETESTE Zeit zwischen
// der Zeit der Eltern und jetzt, zu der alle passen. Der erste Block nach
// einem Absturz traegt damit die Zeit, zu der seine Auftraege angenommen
// wurden -- genau den Block, der vor dem Absturz entstanden waere. Nichts wird
// weggelassen, jeder Knoten wendet an, was der Erzeuger angewendet hat. Die
// Nachspielenden lassen eine Blockzeit bis 120 s vor den Eltern zu und
// begrenzen sie nach hinten nicht (zeitstempel_pruefung.go); der Erzeuger
// geht nie vor seine Eltern.
//
// Gibt es keine solche Zeit, entsteht kein Block (fail-closed): lieber steht
// die Erzeugung laut, als dass ein Block entsteht, den jeder abweist, oder
// ein Zustand, den nur dieser Knoten hat. Damit das nicht eintritt, haelt
// annahme_pause.go die Annahme an, solange die Erzeugung steht oder der
// Ausgang von vor dem Start noch nicht verblockt ist: die Auftraege eines
// Blocks liegen dann zeitlich nah beieinander, und ihre Fenster ueberlappen.

// blockZeitZurueck: Bloecke, deren Zeit vor jetzt lag, weil ihre Auftraege
// es verlangten. blockZeitKonflikte: Versuche ohne gemeinsame Zeit.
var (
	blockZeitZurueck     atomic.Int64
	blockZeitKonflikte   atomic.Int64
	blockZeitLetzterGrnd atomic.Value // string
	blockZeitLetzteMeld  atomic.Int64
)

// BlockZeitStand: fuer /api/health/combined.
func BlockZeitStand() map[string]interface{} {
	grund, _ := blockZeitLetzterGrnd.Load().(string)
	return map[string]interface{}{
		"bedeutung": "Blockzeit nach den Auftraegen (block_tauglich.go). zurueckdatiert: Bloecke, die die Zeit ihrer Annahme " +
			"tragen statt der Uhr (nach einem Neustart normal). konflikte > 0: es gibt Auftraege im Ausgang, die zu keiner " +
			"gemeinsamen Blockzeit passen -- dieser Knoten erzeugt dann keinen Block und nimmt nach 30 s nichts mehr an. " +
			"Abhilfe durch einen Menschen: den Knoten vom Seed neu aufsetzen (Resync) und seinen Ausgang verwerfen.",
		"zurueckdatiert":   blockZeitZurueck.Load(),
		"konflikte":        blockZeitKonflikte.Load(),
		"letzter_konflikt": grund,
	}
}

// zeitFenster: die Blockzeiten [von, bis], zu denen eine Transaktion die
// zeitabhaengigen Regeln der Nachspielenden besteht. Leer, wenn von > bis.
type zeitFenster struct{ von, bis int64 }

func offenesFenster() zeitFenster { return zeitFenster{math.MinInt64, math.MaxInt64} }

func (f *zeitFenster) eng(von, bis int64) {
	if von > f.von {
		f.von = von
	}
	if bis < f.bis {
		f.bis = bis
	}
}

func sattAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	if b < 0 && a < math.MinInt64-b {
		return math.MinInt64
	}
	return a + b
}

// auftragsFenster: das Zeitfenster einer Transaktion -- mit den Regeln, die
// zur Zeit jetzt gelten. Eine Regel, die erst spaeter gilt, kann den Bereich
// vor jetzt nicht betreffen; eine, die vor jetzt noch nicht galt, wird hier
// streng angesetzt (blockZeitFuer prueft die gewaehlte Zeit danach genau).
func auftragsFenster(tx *Transaction, jetzt int64) zeitFenster {
	f := offenesFenster()
	switch {
	case tx.Type == "validator_bindung":
		// validator_register.go: immer geprueft (nicht an die Signaturpflicht
		// gebunden), und erst ab dem Stichtag.
		f.eng(validatorRegisterAb(), math.MaxInt64)
		if tx.Nachweis == nil {
			return zeitFenster{1, 0}
		}
		z := tx.Nachweis.Zeit
		f.eng(sattAdd(z, -nachweisHoechstensVoraus), sattAdd(z, nachweisHoechstensAlt))
	case braucheNachweis(tx.Type) && signierteUeberweisungenPflicht(jetzt):
		if tx.Nachweis == nil {
			// Ohne Nachweis nur vor der Pflicht (Rest aus der Zeit davor).
			f.eng(math.MinInt64, signierteUeberweisungenAb()-1)
		} else if hatNachweisZeit(tx.Type) {
			z := tx.Nachweis.Zeit
			f.eng(sattAdd(z, -nachweisHoechstensVoraus), sattAdd(z, nachweisHoechstensAlt))
		}
	}
	switch tx.Type {
	case "liveness_renewal":
		// nachrechnen_erneuerung.go (erneuerung_zeit): nur im strengen Modus.
		if stagedGrantAktiv(jetzt) && nachrechnenStreng(jetzt) {
			if tx.DistributionAt <= 0 {
				return zeitFenster{1, 0}
			}
			f.eng(sattAdd(tx.DistributionAt, -erneuerungHoechstensVoraus), sattAdd(tx.DistributionAt, erneuerungHoechstensAlt))
		}
	case "register_human":
		// pruefeRegistrierungV8: die Annahmezeit liegt hoechstens 5 min nach
		// dem Block.
		if vertragV8() && tx.RegAt > 0 {
			f.eng(sattAdd(tx.RegAt, -v8NachspielKarenz), math.MaxInt64)
		}
	case "umlauf", "distribution_round_marker":
		// wirtschaft.go/umlauf_lp.go: die Rundenzeit liegt hoechstens 60 s
		// nach dem Block.
		if jetzt >= rundenZeitStrengAbUnix || (tx.Type == "umlauf" && tx.DistributionAt >= umlaufMitLPAbUnix) {
			f.eng(sattAdd(tx.DistributionAt, -60), math.MaxInt64)
		}
		// nachrechnen.go (rundenmarke): im strengen Modus hoechstens 10 min
		// neben dem Block.
		if tx.Type == "distribution_round_marker" && nachrechnenStreng(jetzt) {
			f.eng(sattAdd(tx.DistributionAt, -600), sattAdd(tx.DistributionAt, 600))
		}
	}
	return f
}

// blockTauglich: besteht die Transaktion in einem Block mit dieser Blockzeit
// die zeitabhaengigen Regeln jedes Nachspielenden? Genau, mit den Regeln, die
// zu blockZeit gelten.
func blockTauglich(tx *Transaction, blockZeit int64) error {
	if tx.Type == "validator_bindung" {
		if !validatorRegisterAktiv(blockZeit) {
			return fmt.Errorf("validator_bindung vor dem Stichtag (Block %d)", blockZeit)
		}
		if tx.Nachweis == nil {
			return fmt.Errorf("validator_bindung ohne Nachweis")
		}
		if z := tx.Nachweis.Zeit; z < blockZeit-nachweisHoechstensAlt || z > blockZeit+nachweisHoechstensVoraus {
			return fmt.Errorf("validator_bindung unterschrieben um %d, Block %d", z, blockZeit)
		}
	} else if braucheNachweis(tx.Type) && signierteUeberweisungenPflicht(blockZeit) {
		if tx.Nachweis == nil {
			return fmt.Errorf("%s ohne Nachweis, Block %d", tx.Type, blockZeit)
		}
		if hatNachweisZeit(tx.Type) {
			if z := tx.Nachweis.Zeit; z < blockZeit-nachweisHoechstensAlt || z > blockZeit+nachweisHoechstensVoraus {
				return fmt.Errorf("%s unterschrieben um %d, Block %d (Fenster -%ds/+%ds)", tx.Type, z, blockZeit, nachweisHoechstensAlt, nachweisHoechstensVoraus)
			}
		}
	}
	switch tx.Type {
	case "liveness_renewal":
		if stagedGrantAktiv(blockZeit) && nachrechnenStreng(blockZeit) {
			if z := tx.DistributionAt; z <= 0 || blockZeit-z > erneuerungHoechstensAlt || z-blockZeit > erneuerungHoechstensVoraus {
				return fmt.Errorf("Erneuerung bescheinigt %d, Block %d", z, blockZeit)
			}
		}
	case "register_human":
		if vertragV8() && tx.RegAt > blockZeit+v8NachspielKarenz {
			return fmt.Errorf("Registrierung angenommen um %d, Block %d", tx.RegAt, blockZeit)
		}
	case "umlauf":
		if rundenZeitNachBlock(tx.DistributionAt, blockZeit) || umlaufLPZeitVorgezogen(tx.DistributionAt, blockZeit) {
			return fmt.Errorf("Umlauf zur Runde %d, Block %d", tx.DistributionAt, blockZeit)
		}
	case "distribution_round_marker":
		if rundenZeitNachBlock(tx.DistributionAt, blockZeit) {
			return fmt.Errorf("Rundenmarke %d, Block %d", tx.DistributionAt, blockZeit)
		}
		if d := tx.DistributionAt - blockZeit; nachrechnenStreng(blockZeit) && (d > 600 || d < -600) {
			return fmt.Errorf("Rundenmarke %d, Block %d (strenger Modus)", tx.DistributionAt, blockZeit)
		}
	}
	return nil
}

// blockZeitFuer: die Blockzeit fuer einen Block mit diesen Transaktionen.
// jetzt, wenn alle dazu passen; sonst die spaeteste Zeit in [elternZeit,
// jetzt], zu der alle passen. Fehler: es gibt keine -- dann entsteht kein
// Block.
func blockZeitFuer(txs []Transaction, jetzt, elternZeit int64) (int64, error) {
	alleJetzt := true
	for i := range txs {
		if blockTauglich(&txs[i], jetzt) != nil {
			alleJetzt = false
			break
		}
	}
	if alleJetzt {
		return jetzt, nil
	}
	f := offenesFenster()
	f.eng(elternZeit, jetzt)
	engster := -1
	for i := range txs {
		g := auftragsFenster(&txs[i], jetzt)
		vorher := f
		f.eng(g.von, g.bis)
		if f.von > f.bis {
			engster = i
			f = vorher
			break
		}
	}
	if engster >= 0 {
		tx := &txs[engster]
		return 0, fmt.Errorf("%s von %s passt zu keiner gemeinsamen Blockzeit zwischen %d und %d (Eltern %d, jetzt %d): %v",
			tx.Type, kurzAdresse(tx.Wallet), f.von, f.bis, elternZeit, jetzt, blockTauglich(tx, jetzt))
	}
	t := f.bis
	// Genau pruefen, mit den Regeln, die zu t gelten.
	for i := range txs {
		if err := blockTauglich(&txs[i], t); err != nil {
			return 0, fmt.Errorf("Blockzeit %d (Eltern %d, jetzt %d) besteht nicht: %v", t, elternZeit, jetzt, err)
		}
	}
	return t, nil
}

// blockZeitKonfliktMelden: zaehlt und meldet (hoechstens einmal je Minute).
func blockZeitKonfliktMelden(err error) {
	blockZeitKonflikte.Add(1)
	blockZeitLetzterGrnd.Store(err.Error())
	jetzt := time.Now().Unix()
	if l := blockZeitLetzteMeld.Load(); jetzt-l >= 60 && blockZeitLetzteMeld.CompareAndSwap(l, jetzt) {
		fmt.Printf("[BLOCK] ✗ Kein Block: %v. Ein Block mit diesen Auftraegen wuerde von jedem anderen Knoten abgewiesen, "+
			"Weglassen liesse diesen Knoten abweichen. Die Annahme haelt nach 30 s an (annahme_pause.go). Abhilfe: Resync vom Seed, Ausgang verwerfen.\n", err)
	}
}
