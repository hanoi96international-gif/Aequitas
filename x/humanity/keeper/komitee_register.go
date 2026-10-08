package keeper

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// KOMITEE AUS DEM REGISTER (Validator-Register Schritt 3, schlafend bis
// erzeugerSchnittAb; docs/VALIDATOR_REGISTER_KONSENS.md).
//
// Bisher (computeEpochCommittee): die Kandidaten kommen aus
// authorizedValidators -- einer Liste, die jeder Knoten selbst fuehrt
// (Konfiguration, Abgleich unter Peers, Registrierungen bei diesem Knoten).
// Kennen zwei Knoten verschiedene Listen, waehlen sie verschiedene Komitees;
// jeder entscheidet nur fuer sich, ob er erzeugt, und die Zahl gleichzeitiger
// Erzeuger ist nicht mehr durch targetCommitteeSize begrenzt. Und eine Liste,
// die jemand mit erfundenen Adressen fuellt, drueckt echte Validatoren aus
// dem Komitee dieses Knotens.
//
// Ab erzeugerSchnittAb entscheidet ueber jeden Block das Register (die
// Erzeugerpruefung in AddPeerBlock, erzeugerErlaubt). Dann kommt auch das
// Komitee daraus -- aus derselben Menge, aus der die Erzeugerpruefung und die
// Validatoren-Belohnung (validator_lohn_kette.go) kommen:
//   - Zur Zeit t ist im Komitee, wer zu t in genau einem Erzeugerfenster
//     eines menschlichen Betreibers steht (erzeugerStand, mit erzeugerFrist)
//     UND unter diesen zu den targetCommitteeSize mit dem kleinsten
//     sha256(lower(addr)+":"+epoche) gehoert. Die Epoche zaehlt nach der
//     Zeit (t / epochLength), nicht nach der Hoehe: die Hoehe der Spitze ist
//     je Knoten verschieden, die Zeit bis auf die Uhr nicht.
//   - Die Fenster zu t stehen fest, sobald t erreicht ist: eine Bindung wirkt
//     erst erzeugerFrist nach ihrem Zeitpunkt, und bis dahin hat jeder
//     Knoten, der dem Netz folgt, sie nachgespielt. Jeder solche Knoten mit
//     demselben Stand berechnet zu t dasselbe Komitee -- hoechstens
//     targetCommitteeSize Erzeuger zugleich. (Geschlossen liest der Stand
//     die eigene Liste und den eigenen Schluessel; verschieden wird es erst
//     mit mehr als targetCommitteeSize Eintraegen. An einer Fenstergrenze
//     sind sich Knoten so lange uneinig, wie ihre Uhren auseinanderliegen.)
//   - Ein Schluesselwechsel bleibt nahtlos, solange der neue Schluessel
//     unter den ersten targetCommitteeSize Zugelassenen liegt -- mit
//     hoechstens so vielen Zugelassenen (geschlossener Betrieb) immer: er
//     steht im Komitee, sobald sein Fenster beginnt, nicht erst in der
//     naechsten Epoche (Sicherheitsdurchgang zum Komitee, MEDIUM-1 -- sonst
//     stuende ein Netz mit einem Validator nach einem Wechsel bis zum
//     Epochenende).
//
// Das Komitee bleibt eine Regel fuer den eigenen Knoten: kein Knoten weist
// einen Block ab, weil sein Erzeuger nicht im Komitee ist (das waere eine
// neue Konsensregel). K haengt nicht daran (ghostdagKBase).
//
// FEHLERFALL. Kein Stand oder ein Lesefehler: ein LEERES Komitee -- dieser
// Knoten erzeugt nicht (fail-closed, auch wenn der Stand zwischen der
// Pruefung des eigenen Schluessels und dieser wechselt).
//
// GRENZEN. Die Rangliste einer Epoche enthaelt die Schluessel des Stands,
// deren Fenster die Epoche beruehren: geschlossen (AUTHORIZED_VALIDATORS) nur
// die Liste, offen hoechstens so viele, wie verlaufGrenze Zeilen tragen.
// Berechnet wird sie beim Auffrischen des Stands fuer diese und die naechste
// Epoche (erzeugerRegisterAuffrischen, ausserhalb von dag.mu); nur fehlt sie
// dort, rechnet ProduceBlock sie einmal je Stand und Epoche selbst. Je Block
// danach: die Rangliste von vorn, bis targetCommitteeSize Schluessel
// zugelassen sind.
//
// OFFENER BETRIEB (nicht freigegeben): die Rangfolge ist vorhersagbar. Wer
// Bindungen fuer Schluessel haelt, die nie erzeugen, oder Adressen sucht, die
// in einer Epoche vorn liegen, belegt Sitze -- vor einem offenen Betrieb
// braucht die Auswahl einen Zufall, der zur Zeit der Bindung unbekannt ist.
// Und die Kosten wachsen mit dem Verlauf: bei 100.000 Schluesseln rund
// 250 ms Vorberechnung je Auffrischen (auch unter replayMu, nach Bloecken
// mit Bindungen) und bis zu 6 ms je Block, wenn weniger als
// targetCommitteeSize zugelassen sind.

// komiteePunkte: die Rangzahl einer Adresse in einer Epoche (kleiner ist
// vorn) -- fuer die lokale Liste und das Register dieselbe.
func komiteePunkte(addr string, epoche int64) [32]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("%s:%d", strings.ToLower(addr), epoche)))
}

// erzeugerKomitee: das Komitee fuer einen Block, den dieser Knoten jetzt
// bauen wuerde (nextHeight). Vor erzeugerSchnittAb wie bisher
// (getEpochCommittee), danach aus dem Register. Aufrufer haelt dag.mu
// (ProduceBlock).
func (dag *BlockDAG) erzeugerKomitee(nextHeight, jetzt int64) *EpochCommittee {
	if erzeugerSchnittAktiv(jetzt) {
		return dag.komiteeAusRegister(jetzt)
	}
	return dag.getEpochCommittee(nextHeight)
}

// komiteeAusRegister: das Komitee zur Zeit jetzt aus dem Register. Nie nil:
// ohne Stand oder nach einem Lesefehler leer (niemand erzeugt).
func (dag *BlockDAG) komiteeAusRegister(jetzt int64) *EpochCommittee {
	epoche := jetzt / epochLength
	ec := &EpochCommittee{Number: epoche, Members: make(map[string]bool, targetCommitteeSize)}
	if dag.state == nil {
		return ec
	}
	st := dag.state.erzeugerRegister.Load()
	if st == nil || st.fehler != nil {
		return ec
	}
	for _, addr := range dag.komiteeRangliste(st, epoche) {
		if len(ec.Members) >= targetCommitteeSize {
			break
		}
		if st.erzeugerFenster(addr, jetzt) != "" {
			ec.Members[strings.ToLower(addr)] = true
		}
	}
	ec.Size = len(ec.Members)

	dag.epochMu.Lock()
	neueEpoche := dag.registerKomiteeEpoche != epoche
	dag.registerKomiteeEpoche = epoche
	dag.epochMu.Unlock()
	if neueEpoche {
		role := "observer"
		if ec.Members[dag.selfProposer] {
			role = "producer"
		}
		fmt.Printf("[EPOCH] Epoche %d (Zeit %d, aus dem Register): Komitee=%d Validatoren, K=%d, self=%s (%s)\n",
			epoche, jetzt, ec.Size, dag.k(), dag.selfProposer, role)
	}
	return ec
}

// komiteeRangliste: die Rangliste der Epoche -- vorberechnet am Stand, sonst
// einmal je Stand und Epoche hier berechnet und gemerkt.
func (dag *BlockDAG) komiteeRangliste(st *erzeugerStand, epoche int64) []string {
	if r, ok := st.komiteeRang[epoche]; ok {
		return r
	}
	dag.epochMu.RLock()
	if dag.registerRangStand == st && dag.registerRangEpoche == epoche {
		r := dag.registerRang
		dag.epochMu.RUnlock()
		return r
	}
	dag.epochMu.RUnlock()
	r := komiteeRangBerechnen(st, epoche)
	dag.epochMu.Lock()
	dag.registerRang, dag.registerRangStand, dag.registerRangEpoche = r, st, epoche
	dag.epochMu.Unlock()
	return r
}

// komiteeRangBerechnen: die Schluessel des Stands, deren Fenster die Epoche
// beruehren, nach komiteePunkte geordnet (gleiche Punkte: nach Adresse).
func komiteeRangBerechnen(st *erzeugerStand, epoche int64) []string {
	von, bis := epoche*epochLength, (epoche+1)*epochLength
	type eintrag struct {
		addr   string
		punkte [32]byte
	}
	var liste []eintrag
	for addr, fenster := range st.fenster {
		for _, f := range fenster {
			if f.von < bis && f.bis > von {
				liste = append(liste, eintrag{addr: addr, punkte: komiteePunkte(addr, epoche)})
				break
			}
		}
	}
	sort.Slice(liste, func(i, j int) bool {
		if c := bytes.Compare(liste[i].punkte[:], liste[j].punkte[:]); c != 0 {
			return c < 0
		}
		return liste[i].addr < liste[j].addr
	})
	out := make([]string, len(liste))
	for i, e := range liste {
		out[i] = e.addr
	}
	return out
}

// komiteeVorberechnen: die Ranglisten der Epoche von jetzt und der
// naechsten, fuer den Stand vor dem Speichern (erzeugerRegisterAuffrischen).
// Vor erzeugerSchnittAb nichts -- dann waehlt das Komitee nicht aus dem
// Register.
func komiteeVorberechnen(st *erzeugerStand, jetzt int64) map[int64][]string {
	if st == nil || st.fehler != nil || !erzeugerSchnittAktiv(jetzt+epochLength) {
		return nil
	}
	e := jetzt / epochLength
	return map[int64][]string{e: komiteeRangBerechnen(st, e), e + 1: komiteeRangBerechnen(st, e+1)}
}
