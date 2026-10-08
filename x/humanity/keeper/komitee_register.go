package keeper

import (
	"fmt"
	"sort"
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
//   - Kandidaten sind die Schluessel, die zum BEGINN der Epoche in genau
//     einem Erzeugerfenster eines menschlichen Betreibers stehen
//     (erzeugerStand, mit erzeugerFrist). Eine Bindung wirkt erst zwei
//     Stunden nach ihrem Zeitpunkt; zum Beginn einer Epoche, die schon
//     laeuft, hat also jeder Knoten, der dem Netz folgt, dieselben Fenster.
//   - Die Epoche zaehlt nach der Zeit (jetzt / epochLength), nicht nach der
//     Hoehe: die Hoehe der Spitze ist je Knoten verschieden, die Zeit
//     bis auf die Uhr nicht.
//   - Ausgewaehlt wird wie bisher (komiteeWaehlen).
// Jeder Knoten, der dem Netz folgt, berechnet so zur selben Epoche dasselbe
// Komitee -- hoechstens targetCommitteeSize Erzeuger je Epoche.
//
// Das Komitee bleibt eine Regel fuer den eigenen Knoten: kein Knoten weist
// einen Block ab, weil sein Erzeuger nicht im Komitee ist (das waere eine
// neue Konsensregel). K haengt nicht daran (ghostdagKBase).
//
// Ein neu gebundener Schluessel wartet damit bis zur naechsten Epoche (bis zu
// einer Stunde nach dem Ende seiner Frist); ein Schluessel, dessen Fenster in
// der Epoche endet, erzeugt ab dann nicht mehr -- das prueft ProduceBlock
// vorher (nicht_im_register).
//
// FEHLERFALL. Kein Stand oder ein Lesefehler: kein Komitee (nil). Erzeugt
// wird dann trotzdem nicht -- ProduceBlock prueft vorher den eigenen
// Schluessel gegen denselben Stand (erzeugerNachRegister), und der schliesst
// ab.
//
// GRENZEN. Die Kandidaten sind die Schluessel des Stands: geschlossen
// (AUTHORIZED_VALIDATORS) nur die Liste, offen hoechstens so viele, wie
// verlaufGrenze Zeilen tragen. Neu berechnet wird je Epoche und je neu
// gelesenem Stand (hoechstens alle 30 s, erzeugerRegisterAuffrischen), unter
// dag.mu: einmal sha256 je Kandidat und eine Sortierung.

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

// komiteeAusRegister: das Komitee der Epoche, in der jetzt liegt, aus dem
// Register. nil: kein Stand, Lesefehler oder keine Kandidaten.
func (dag *BlockDAG) komiteeAusRegister(jetzt int64) *EpochCommittee {
	if dag.state == nil {
		return nil
	}
	st := dag.state.erzeugerRegister.Load()
	if st == nil || st.fehler != nil {
		return nil
	}
	epoche := jetzt / epochLength

	dag.epochMu.RLock()
	if dag.registerKomiteeStand == st && dag.registerKomitee != nil && dag.registerKomitee.Number == epoche {
		ec := dag.registerKomitee
		dag.epochMu.RUnlock()
		return ec
	}
	dag.epochMu.RUnlock()

	ec := komiteeWaehlen(komiteeKandidaten(st, epoche*epochLength), epoche)
	if ec == nil {
		return nil
	}

	dag.epochMu.Lock()
	neueEpoche := dag.registerKomitee == nil || dag.registerKomitee.Number != epoche
	dag.registerKomitee, dag.registerKomiteeStand = ec, st
	dag.epochMu.Unlock()
	if neueEpoche {
		role := "observer"
		if ec.Members[dag.selfProposer] {
			role = "producer"
		}
		fmt.Printf("[EPOCH] Epoche %d (Zeit %d, aus dem Register): Komitee=%d Validatoren, K=%d, self=%s (%s)\n",
			epoche, epoche*epochLength, ec.Size, dag.k(), dag.selfProposer, role)
	}
	return ec
}

// komiteeKandidaten: die Schluessel, die zur Zeit t in genau einem
// Erzeugerfenster stehen (umstritten: keiner), sortiert.
func komiteeKandidaten(st *erzeugerStand, t int64) []string {
	out := make([]string, 0, len(st.fenster))
	for addr := range st.fenster {
		if st.erzeugerFenster(addr, t) != "" {
			out = append(out, addr)
		}
	}
	sort.Strings(out)
	return out
}
