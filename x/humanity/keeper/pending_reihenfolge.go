package keeper

import "sync/atomic"

// DIE REIHENFOLGE IM BLOCK IST DIE REIHENFOLGE DER ANWENDUNG. SONST LAUFEN
// DIE VALIDATOREN AUSEINANDER.
//
// GEMESSEN AM 12.09.2026. C1 und C2 hatten verschiedene account_set_xor:
// von 79 Lasttest-Konten eines Buckets standen 14 auf beiden Boxen mit
// verschiedenem Guthaben (0,000008 gegen 0,001058 AEQ), Menschen und alle
// Konten ueber 0,5 AEQ gleich. Und die Abweichung wuchs mit jedem Lauf: in
// zwei Minuten Last uebersprang C1 beim Nachspielen von C2s Bloecken 100
// Ueberweisungen ("insufficient balance ... after demurrage"), weil sein
// Stand des Absenders schon abwich. Ein Knoten, der eine Ueberweisung
// ueberspringt, weicht ab -- und weicht danach immer weiter ab.
//
// WOHER DER ERSTE UNTERSCHIED KOMMT. Der Produzent wendet eine Ueberweisung
// bei der Annahme an (WAL-Schnellpfad, unter der Shard-Sperre der beiden
// Konten) und schreibt sie spaeter -- ueber 32 nebenlaeufige Flush-Arbeiter
// -- als Zeile in pending_txs. ProduceBlock las die Zeilen ORDER BY id. Die
// ID ist die Reihenfolge des FLUSH, nicht die der Anwendung. Zwei
// Ueberweisungen desselben Kontos (X->Y, dann W->X, dann X->Z, in dieser
// Reihenfolge angewendet und alle gueltig) koennen im Block als X->Y, X->Z,
// W->X stehen. Der Nachspielende sieht X->Z dann vor der Gutschrift, lehnt
// ab, ueberspringt -- und hat ab jetzt einen anderen Kontostand als der
// Produzent. Bei Konten nahe null (Lasttest-Staub, aber auch jeder Mensch
// mit fast leerem Konto) passiert das staendig.
//
// DIE REIHENFOLGE, DIE STIMMT: die WAL-Seq. Sie wird in AppendAsync unter
// derselben Shard-Sperre vergeben, unter der die Ueberweisung angewendet
// wurde; zwei Ueberweisungen, die ein Konto teilen, halten dieselbe Sperre
// und bekommen ihre Seqs in Anwendungsreihenfolge. Sie steht jetzt in
// pending_txs.wal_seq, und ProduceBlock liest ORDER BY wal_seq, id.
//
// Zeilen, die nicht ueber den Schnellpfad kommen (Registrierung, Swap,
// Slashing, Demurrage-Ueberweisungen unter der exklusiven Sperre), bekommen
// die Seq, die der WAL ALS NAECHSTES vergeben wuerde: alles Schnellpfad-
// Angewendete davor hat eine kleinere, alles danach eine groessere oder
// gleiche (dann entscheidet die ID). Ohne WAL sind alle Seqs 0 und die ID
// ordnet wie bisher.
//
// Was das NICHT loest: eine Abweichung, die schon da ist (siehe die Zahlen
// oben) -- die verschwindet nur durch einen Resync eines Knotens vom
// anderen. Und das Ueberspringen selbst bleibt die Notbremse, damit ein
// abweichender Knoten nicht steht; nur soll sie nicht mehr im Normalbetrieb
// greifen.

var pendingSeqQuelle atomic.Pointer[func() uint64]

func setzePendingSeqQuelle(f func() uint64) {
	pendingSeqQuelle.Store(&f)
}

// pendingSeqJetzt liefert die naechste WAL-Seq -- oder 0 ohne WAL.
func pendingSeqJetzt() int64 {
	f := pendingSeqQuelle.Load()
	if f == nil {
		return 0
	}
	return int64((*f)())
}
