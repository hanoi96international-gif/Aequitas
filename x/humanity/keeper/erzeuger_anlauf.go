package keeper

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// ANLAUF NACH EINER NEUEN GENESIS (Neustart bei null, 15.11.2026).
//
// Seit erzeugerSchnittAb (28.10.2026) erzeugt nur, wen das Validator-Register
// zur Blockzeit traegt (validator_register_leser.go). Nach einem Neustart bei
// null ist das Register leer -- und eine Bindung (validator_bindung) braucht
// einen registrierten Menschen und einen Block, der sie traegt. Ohne Bloecke
// gibt es keinen Menschen, ohne Menschen keine Bindung, ohne Bindung keinen
// Block: die Kette steht bei Hoehe 0 (so am 15.11.2026 16:47 UTC auf C1).
//
// Darum ein Anlauf, eng begrenzt:
//
//   - nur im geschlossenen Betrieb und nur fuer Schluessel aus
//     AUTHORIZED_VALIDATORS (die Liste, deren Hash jeder Knoten vergleicht);
//     nicht fuer den eigenen Schluessel, wenn er nicht darin steht, und
//     offen (ohne Liste) nie -- dort koennte sich sonst jeder selbst
//     zulassen;
//   - ab der Genesis-Zeit hoechstens erzeugerAnlaufDauer lang;
//   - und nur bis zur ersten wirksamen Bindung des Schluessels: das
//     Anlauf-Fenster endet, wo sein erstes echtes Fenster beginnt (die
//     Bindung plus erzeugerFrist). Danach gilt allein das Register -- auch
//     fuer das Ende einer Bindung, das Register nimmt weiter weg.
//
// Das Fenster steht im gespeicherten Stand wie jedes andere; Erzeugerpruefung
// (eigene Bloecke und fremde), Komitee und Checkpoint-Komitee urteilen damit
// gleich. Es traegt den Betreiber anlaufBetreiber -- keinen Menschen: Lohn,
// Leitung, Coordinator-Zulassung und Lebensnachweis bauen ihre Fenster selbst
// aus dem Verlauf und sehen es nicht, und /api/status fuehrt den Schluessel
// weiter unter erzeuger_ohne_bindung, bis er gebunden ist.
//
// Jeder Knoten rechnet dasselbe: dieselbe Genesis, dieselbe Liste (Hash in
// /api/status), derselbe Verlauf. Ein Knoten, der die erste Bindung eines
// Schluessels noch nicht nachgespielt hat, laesst ihn im Anlauf erzeugen;
// einer, der sie kennt, ebenso bis zu ihrem Fensterbeginn -- danach decken
// sich beide (erzeugerFrist laesst eine Stunde zum Nachspielen).
const (
	erzeugerAnlaufDauer int64 = 14 * 86400
	anlaufBetreiber           = "anlauf:genesis"
)

// Nur fuer Tests (0 = genesis.json).
var anlaufGenesisOverride atomic.Int64

var anlaufGenesisEinmal = sync.OnceValue(genesisTimestamp)

func anlaufGenesis() int64 {
	if o := anlaufGenesisOverride.Load(); o != 0 {
		return o
	}
	return anlaufGenesisEinmal()
}

// anlaufFensterErgaenzen: die Fenster des Stands plus je Schluessel aus
// liste das Anlauf-Fenster [genesis, min(genesis+Dauer, erster
// Fensterbeginn)). Veraendert fenster nicht. Ueberlappt nie mit einem echten
// Fenster -- sonst hiesse der Schluessel "umstritten" (erzeugerFenster).
func anlaufFensterErgaenzen(fenster map[string][]zeitfenster, liste []string, genesis int64) map[string][]zeitfenster {
	if len(liste) == 0 || genesis <= 0 {
		return fenster
	}
	out := make(map[string][]zeitfenster, len(fenster)+len(liste))
	for k, v := range fenster {
		out[k] = v
	}
	ende := sattAdd(genesis, erzeugerAnlaufDauer)
	for _, a := range liste {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		bis := ende
		for _, f := range out[a] {
			if f.von < bis {
				bis = f.von
			}
		}
		if bis <= genesis {
			continue
		}
		neu := make([]zeitfenster, 0, len(out[a])+1)
		neu = append(neu, zeitfenster{betreiber: anlaufBetreiber, von: genesis, bis: bis})
		out[a] = append(neu, out[a]...)
	}
	return out
}

// erzeugerAnlaufListe: die Schluessel mit Anlauf -- genau AUTHORIZED_VALIDATORS.
// Offen (keine Liste): keine.
func (dag *BlockDAG) erzeugerAnlaufListe() []string {
	if len(dag.produzentenFest) == 0 {
		return nil
	}
	out := make([]string, 0, len(dag.produzentenFest))
	for a := range dag.produzentenFest {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}
