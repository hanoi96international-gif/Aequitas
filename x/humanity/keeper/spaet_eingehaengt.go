package keeper

import "fmt"

// L1 (zweiter Sicherheitsdurchgang zu #303): KEIN SPAET EINGEHAENGTER BLOCK
// MIT BINDUNG ODER BEWEIS.
//
// Die Erzeugerpruefung (validator_register_leser.go) und die Abrechnung der
// Geldstrafe (strafe_abrechnung.go) verlassen sich darauf, dass jeder Knoten
// eine Zeile kennt, bevor sie wirkt:
//   - Ein Urteil ueber einen Block zur Zeit t haengt nur an Bindungen mit
//     Zeitpunkt <= t - erzeugerFrist (2 h). Die Bindung steht in einem Block
//     hoechstens nachweisHoechstensAlt (1 h) nach ihrem Zeitpunkt -- es
//     bleibt eine Stunde, sie nachzuspielen.
//   - Die Abrechnung liest Bindungen bis DetectedAt + W und den Beweis, der
//     hoechstens W nach DetectedAt in einem Block steht; sie gilt erst ab
//     DetectedAt + W + erzeugerFrist.
//
// Das setzt voraus, dass der Block mit der Zeile PUENKTLICH ankommt. Ein
// Erzeuger, der ihn zurueckhaelt und Stunden spaeter mit einem frischen
// Block einhaengt, der ihn als Elternteil nennt, umginge die
// Finalitaetswand (isFinalityViolation laesst einen nachgeholten Vorfahren
// durch, auf den ein Nachfolger wartet, und gilt nicht, solange die
// Finalitaet ruht). Dann wirkte seine Zeile rueckwirkend: Knoten, die
// Bloecke schon ohne sie beurteilt hatten, urteilten anders als Knoten, die
// spaeter nachladen.
//
// Darum: ab dem Stichtag nimmt ein Knoten einen Block mit validator_bindung
// oder slash_equivocation nicht an, wenn seine Blockzeit mehr als
// spaetEingehaengtGrenze (30 min) hinter der neuesten Blockzeit liegt, die
// der Knoten schon angenommen hat (seine Spitzen).
//
// WARUM DIE SPITZEN UND NICHT DIE UHR. Schaden entsteht nur, wenn ein Block Y
// mit t_Y >= Zeitpunkt + Frist schon beurteilt wurde, bevor die Zeile kam.
// Dann liegt die eigene Spitze bei mindestens t_Y >= Blockzeit + 1 h, und
// der spaete Block wird abgewiesen. Gemessen an der Uhr dagegen wiese jeder
// Knoten den ersten Block des einzigen Erzeugers nach einem Absturz ab: der
// traegt eine Bindung aus dem Ausgang mit der Zeit ihrer Annahme
// (block_tauglich.go), Stunden vor jetzt -- und die Kette risse. Nach einem
// Absturz ohne andere Erzeuger steht die Spitze aber auf dem Stand davor,
// und der Block ist nicht spaet. Ein nachholender Knoten hat ebenso alte
// Spitzen und weist nichts ab, was in seine Geschichte gehoert.
//
// Ausgenommen ist die Geschichte vom vertrauten Seed (FromSync): ein
// nachholender Knoten bekommt sie dort in der Reihenfolge der Kette, und ein
// zurueckgehaltener Block steht in ihr nie, weil der Seed ihn selbst
// abgewiesen hat.
//
// GRENZEN.
//   - Die Spitzen sind je Knoten leicht verschieden. Gibt ein Erzeuger den
//     Block genau um die Grenze frei, koennen Knoten ihn verschieden
//     behandeln -- wie an der Finalitaetswand. Er kann damit nur seinen
//     eigenen Block verlieren, keine Zeile rueckwirkend einschmuggeln.
//   - Mit mehreren Erzeugern: faellt einer aus und erzeugen die anderen
//     weiter, wird sein erster Block nach dem Neustart abgewiesen, wenn er
//     eine Bindung aus der Zeit vor dem Absturz traegt. Er muss dann vom
//     Seed neu aufsetzen -- angenommen haette seine Zeile rueckwirkend
//     gewirkt.
//   - Ebenso nach einer Trennung: ein ERZEUGER, der ueber 30 Minuten
//     abgeschnitten war und weiter eigene Bloecke gemacht hat, weist danach
//     einen Block mit Bindung oder Beweis von der anderen Seite ab und heilt
//     nicht mehr von selbst (die ruhende Finalitaet haette es sonst
//     erlaubt). Er setzt vom Seed neu auf. Ein Knoten, der nicht erzeugt,
//     ist nicht betroffen: seine Spitzen stehen waehrend der Trennung.
//   - Ein ehrlicher Block, der ueber 30 Minuten als Waise wartet, waehrend
//     die Spitzen weiterlaufen, kommt nur noch ueber die Geschichte vom Seed.
const spaetEingehaengtGrenze int64 = 30 * 60

// spaetEingehaengt: Grund, warum der Block zu spaet kommt, oder "".
// spitze liefert die neueste Blockzeit unter den Spitzen dieses Knotens
// (0: keine); gefragt nur, wenn der Block eine solche Transaktion traegt.
func spaetEingehaengt(block *Block, spitze func() int64) string {
	if block.FromSync {
		return ""
	}
	for i := range block.Transactions {
		switch typ := block.Transactions[i].Type; {
		case typ == "validator_bindung" && validatorRegisterAktiv(block.Timestamp),
			typ == "slash_equivocation" && registerLeserAktiv(block.Timestamp):
			spitze := spitze()
			if spitze <= sattAdd(block.Timestamp, spaetEingehaengtGrenze) {
				return ""
			}
			return fmt.Sprintf("%s in einem Block von %d, %ds hinter der eigenen Spitze (hoechstens %ds)",
				typ, block.Timestamp, spitze-block.Timestamp, spaetEingehaengtGrenze)
		}
	}
	return ""
}

// neuesteSpitzenzeitLocked: die groesste Blockzeit unter den Spitzen. Unter
// dag.mu. Die Zahl der Spitzen ist klein (parallele Erzeuger).
func (dag *BlockDAG) neuesteSpitzenzeitLocked() int64 {
	var neueste int64
	for h := range dag.tips {
		if b := dag.blocks[h]; b != nil && b.Timestamp > neueste {
			neueste = b.Timestamp
		}
	}
	return neueste
}
