package keeper

import (
	"sort"
	"strings"
	"sync"
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
//   - und nur bis zur ersten Bindung des Schluessels: das Anlauf-Fenster
//     endet erzeugerFrist nach seiner ERSTEN Zeile im Verlauf, gleich von
//     wem -- also genau dort, wo sein erstes echtes Fenster beginnt. Auch
//     wenn der Betreiber spaeter kein Mensch mehr ist und sein Fenster
//     dadurch entfaellt, kommt der Anlauf nicht zurueck (Sicherheitsdurchgang
//     #524). Danach gilt allein das Register, das weiter wegnimmt.
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

// anlaufGenesis: die Genesis-Zeit aus genesis.json, einmal je Prozess gelesen
// (die Erzeugerpruefung laeuft je Block unter dag.mu). Ohne lesbare Datei
// die feste Zeit von genesisTimestamp -- deren Anlauf ist laengst vorbei
// (fail-closed).
var anlaufGenesis = sync.OnceValue(genesisTimestamp)

// anlaufFensterErgaenzen: die Fenster des Stands plus je Schluessel aus
// liste das Anlauf-Fenster [genesis, min(genesis+Dauer, erste Bindung +
// erzeugerFrist, erster Fensterbeginn)). erste: je Schluessel die Zeit
// seiner ersten Bindung im Verlauf (erzeugerAusVerlauf). Veraendert fenster
// nicht. Ueberlappt nie mit einem echten Fenster -- sonst hiesse der
// Schluessel "umstritten" (erzeugerFenster).
func anlaufFensterErgaenzen(fenster map[string][]zeitfenster, liste []string, genesis int64, erste map[string]int64) map[string][]zeitfenster {
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
		if z, ok := erste[a]; ok {
			if w := sattAdd(z, erzeugerFrist); w < bis {
				bis = w
			}
		}
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

// ersteBindungen: je Signierschluessel die Zeit seiner ersten Zeile im
// Verlauf.
func ersteBindungen(zeilen []bindungsZeile) map[string]int64 {
	out := make(map[string]int64, len(zeilen))
	for _, z := range zeilen {
		if t, ok := out[z.signing]; !ok || z.zeit < t {
			out[z.signing] = z.zeit
		}
	}
	return out
}
