package keeper

// Schlanke Blockansicht fuer die oeffentlichen Blocklisten
// (/api/blocks ohne min_height, /api/blocks/canonical) -- der Explorer
// fragt beide alle 6 Sekunden je offenem Browser-Tab ab.
//
// # WAS GEMESSEN WURDE
//
// 27.09.2026, TPS-Messung mit Einzelueberweisungen: C1 stand ~11 Sekunden
// still. Das Heap-Profil danach: 5,0 von 5,5 GB belegt in genau diesen zwei
// Handlern -- json.Encoder baute ganze Blocklisten im Speicher auf (50 bzw.
// 30 Bloecke mit je bis zu 7.000 Ueberweisungen, seit Stufe 1 jede mit ihrer
// signierten Rohform). GOMEMLIMIT ist 8 GiB; darueber sammelt Go ohne Pause
// Speicher ein, und der Knoten steht -- Bloecke, Annahme, alles.
//
// # WAS DIE ANSICHT LIEFERT
//
// Alle Kopffelder unveraendert, dazu tx_count (die echte Zahl) und nur die
// letzten listenTxJeBlock Ueberweisungen, ohne Rohform. Mehr zeigt der
// Explorer nicht an (30 neueste insgesamt); wer einen ganzen Block braucht,
// holt ihn einzeln (/api/block?hash=). Validatoren benutzen diese Wege
// nicht -- sie synchronisieren ueber min_height, by-hash und push, die
// unveraendert voll liefern.

const listenTxJeBlock = 30

type blockListenEintrag struct {
	*Block
	// Ueberdeckt Block.Transactions (flachere Ebene gewinnt in encoding/json).
	Transactions []Transaction `json:"transactions,omitempty"`
	TxAnzahl     int           `json:"tx_count"`
	TxGekuerzt   bool          `json:"tx_gekuerzt,omitempty"`
}

func blockListenAnsicht(bloecke []*Block) []blockListenEintrag {
	out := make([]blockListenEintrag, 0, len(bloecke))
	for _, b := range bloecke {
		if b == nil {
			continue
		}
		txs := b.Transactions
		e := blockListenEintrag{Block: b, TxAnzahl: len(txs)}
		if len(txs) > listenTxJeBlock {
			txs = txs[len(txs)-listenTxJeBlock:]
			e.TxGekuerzt = true
		}
		if len(txs) > 0 {
			e.Transactions = make([]Transaction, len(txs))
			for i := range txs {
				e.Transactions[i] = txs[i]
				e.Transactions[i].Roh = ""
			}
		}
		out = append(out, e)
	}
	return out
}
