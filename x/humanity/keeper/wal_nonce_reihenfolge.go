package keeper

// Nonce-Reihenfolge signierter Ueberweisungen im WAL-Pfad (Stufe 1.0).
//
// Jeder nachspielende Knoten verlangt, dass die Nonces eines Absenders in
// Blockreihenfolge nie sinken (naechsteNoncenFuerBlockLocked) -- ueber alle
// Bloecke hinweg, denn NaechsteNonce ist gemeinsamer Zustand. Bloecke werden
// aus pending_txs nach id gebaut (LoadPendingTxsWithLimit). Die Nonces eines
// Absenders muessen dort also in steigender id-Reihenfolge stehen, und zwar
// schon SICHTBAR in dieser Reihenfolge: eine Zeile mit kleinerer id, die erst
// nach einer groesseren committet wird, landet in einem spaeteren Block.
//
// Die Annahme vergibt die Nonces je Absender streng steigend (unter der
// Sperre des Absenderkontos, NaechsteNonce steigt nur). Verloren gehen kann
// die Reihenfolge an zwei Stellen, und beide schliesst diese Datei:
//
//  1. Gleichzeitige Flushes (walFlushConcurrency). Zwei Buendel mit
//     Ueberweisungen desselben Absenders koennten in beliebiger Reihenfolge
//     committen. Ein Buendel endet deshalb vor der ersten signierten
//     Ueberweisung eines Absenders, der gerade in einem anderen laufenden
//     Flush steht (walRohSchnittLocked). Unsignierte Ueberweisungen sind
//     davon nicht betroffen -- sie tragen keine Nonce.
//
//  2. Der serielle Ausweichweg. Faellt eine signierte Ueberweisung vom
//     WAL-Pfad zurueck (Shard belegt, kaltes Konto, Unternehmen ...), schreibt
//     der serielle Weg ihre Zeile SOFORT -- vor noch ungeflushten
//     WAL-Ueberweisungen desselben Absenders mit kleinerer Nonce. Deshalb wird
//     vorher geflusht, bis von diesem Absender nichts mehr offen ist
//     (walVorSeriellLeeren).

import "time"

// walRohEingereihtLocked: eine signierte Ueberweisung von from steht in der
// Warteschlange. walFlushMu gehalten.
func (cs *ChainState) walRohEingereihtLocked(it walFlushItem) {
	if it.tx.Roh == "" {
		return
	}
	if cs.walRohOffen == nil {
		cs.walRohOffen = map[string]int{}
	}
	cs.walRohOffen[it.from]++
}

// walRohSchnittLocked kuerzt ein Buendel (die ersten n Eintraege der
// Warteschlange) vor die erste signierte Ueberweisung eines Absenders, der in
// einem laufenden Flush steht. walFlushMu gehalten.
func (cs *ChainState) walRohSchnittLocked(queue []walFlushItem, n int) int {
	if len(cs.walRohUnterwegs) == 0 {
		return n
	}
	for i := 0; i < n; i++ {
		if queue[i].tx.Roh != "" && cs.walRohUnterwegs[queue[i].from] > 0 {
			return i
		}
	}
	return n
}

// walRohUnterwegsLocked markiert (d=+1) oder entlaesst (d=-1) die Absender
// signierter Ueberweisungen eines Buendels. walFlushMu gehalten.
func (cs *ChainState) walRohUnterwegsLocked(batch []walFlushItem, d int) {
	for _, it := range batch {
		if it.tx.Roh == "" {
			continue
		}
		if cs.walRohUnterwegs == nil {
			cs.walRohUnterwegs = map[string]int{}
		}
		cs.walRohUnterwegs[it.from] += d
		if cs.walRohUnterwegs[it.from] <= 0 {
			delete(cs.walRohUnterwegs, it.from)
		}
	}
}

// walRohGeschriebenLocked: das Buendel steht in Postgres. walFlushMu gehalten.
func (cs *ChainState) walRohGeschriebenLocked(batch []walFlushItem) {
	for _, it := range batch {
		if it.tx.Roh == "" {
			continue
		}
		cs.walRohOffen[it.from]--
		if cs.walRohOffen[it.from] <= 0 {
			delete(cs.walRohOffen, it.from)
		}
	}
}

func (cs *ChainState) walRohOffenFuer(from string) int {
	cs.walFlushMu.Lock()
	defer cs.walFlushMu.Unlock()
	return cs.walRohOffen[from]
}

// walVorSeriellLeeren: vor dem seriellen Weg einer signierten Ueberweisung
// alles Offene dieses Absenders nach Postgres bringen. Liefert false, wenn
// das nicht gelingt (Flush scheitert wiederholt) -- dann darf der serielle
// Weg die Ueberweisung NICHT annehmen, sonst stuende ihre Nonce vor einer
// kleineren im Block.
func (cs *ChainState) walVorSeriellLeeren(from string) bool {
	for versuch := 0; versuch < 50; versuch++ {
		if cs.walRohOffenFuer(from) == 0 {
			return true
		}
		cs.FlushWALNow()
		if versuch > 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return cs.walRohOffenFuer(from) == 0
}
