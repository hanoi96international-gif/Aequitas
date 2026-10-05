package keeper

import (
	"fmt"
	"sync/atomic"
)

// WAS JEDER ANDERE KNOTEN ABWIESE, KOMMT NICHT IN DEN BLOCK (05.10.2026).
//
// Auftraege mit Nachweis (auftrag_nachweis.go: Tausch, Liquiditaet, Faucet,
// Treuhand, Vormund, Lebenszeichen, Unternehmen, Vorbehalt, Validator-
// Bindung) tragen den Zeitpunkt ihrer Unterschrift, und jeder Nachspielende
// prueft ihn gegen die BLOCKZEIT: hoechstens eine Stunde alt, hoechstens fuenf
// Minuten voraus. Ein Verstoss macht den ganzen Block ungueltig.
//
// Der Erzeuger legte bisher alles aus seinem Ausgang in den Block, ohne diese
// Frist zu pruefen. Der Ausgang ueberlebt aber einen Neustart (pending_txs):
// ein Absturz in der Nacht, ein Neustart am Morgen, und der erste Block traegt
// einen Tausch von vor sieben Stunden. Jeder andere Knoten weist ihn ab --
// und mit ihm jeden Block, der auf ihm aufbaut. Mit einem einzigen Erzeuger
// steht damit die Kette.
//
// Jetzt prueft der Erzeuger vor dem Block dieselben zeitabhaengigen Regeln wie
// jeder Nachspielende (blockTauglich) und laesst weg, was sie nicht besteht.
// Alles andere am Auftrag -- Unterschrift, Nonce, Deckung -- hat die Annahme
// schon geprueft, und es aendert sich nicht, waehrend der Auftrag im Ausgang
// liegt; nur die Zeit laeuft. Die Pruefung kostet darum nichts unter dag.mu.
//
// Weggelassen heisst: verbraucht (seine Ausgangszeile wird mit dem Block
// abgehakt, er kommt nicht wieder). Angewendet war er auf diesem Knoten aber
// schon bei der Annahme -- dessen Zustand weicht fuer die betroffenen Konten
// ab, bis zum naechsten Resync. Das ist laut (Zaehler, Log) und ein
// Kontenfehler auf einem Knoten, kein Stillstand des Netzes.
var aussortierteAuftraege atomic.Int64

// AussortierteAuftraegeStand: fuer /api/health/combined.
func AussortierteAuftraegeStand() int64 { return aussortierteAuftraege.Load() }

// blockTauglich: wuerde jeder Nachspielende diese Transaktion in einem Block
// mit dieser Blockzeit an ihrer ZEIT scheitern lassen? Nur die Regeln, die von
// der Blockzeit abhaengen.
func blockTauglich(tx *Transaction, blockZeit int64) error {
	if braucheNachweis(tx.Type) && signierteUeberweisungenPflicht(blockZeit) && tx.Nachweis != nil && hatNachweisZeit(tx.Type) {
		if z := tx.Nachweis.Zeit; z < blockZeit-nachweisHoechstensAlt || z > blockZeit+nachweisHoechstensVoraus {
			return fmt.Errorf("unterschrieben um %d, Block %d (Fenster -%ds/+%ds)", z, blockZeit, nachweisHoechstensAlt, nachweisHoechstensVoraus)
		}
	}
	// Erneuerung (nachrechnen_erneuerung.go, erneuerung_zeit): im strengen
	// Modus weist jeder Knoten eine Bescheinigung ab, die aelter als 7 Tage ist.
	if tx.Type == "liveness_renewal" && stagedGrantAktiv(blockZeit) && nachrechnenStreng(blockZeit) {
		if z := tx.DistributionAt; z <= 0 || blockZeit-z > erneuerungHoechstensAlt || z-blockZeit > erneuerungHoechstensVoraus {
			return fmt.Errorf("Erneuerung bescheinigt %d, Block %d", z, blockZeit)
		}
	}
	return nil
}

// ohneUntauglicheAuftraege: die Transaktionen fuer einen Block mit dieser
// Blockzeit, ohne die, an denen jeder andere Knoten den Block scheitern liesse.
// Die Reihenfolge der uebrigen bleibt.
func ohneUntauglicheAuftraege(txs []Transaction, blockZeit int64) []Transaction {
	out := make([]Transaction, 0, len(txs))
	for i := range txs {
		if err := blockTauglich(&txs[i], blockZeit); err != nil {
			aussortierteAuftraege.Add(1)
			fmt.Printf("[BLOCK] ✗ %s von %s nicht in den Block -- jeder andere Knoten wiese den Block ab: %v. Hier war er bei der Annahme schon angewendet; dieser Knoten weicht fuer das Konto ab, bis zum naechsten Resync.\n",
				txs[i].Type, kurzAdresse(txs[i].Wallet), err)
			continue
		}
		out = append(out, txs[i])
	}
	return out
}
