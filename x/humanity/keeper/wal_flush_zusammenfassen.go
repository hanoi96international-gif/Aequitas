package keeper

import "sync/atomic"

// Stufe 2a: im Speicherkorb-Modus je KONTO flushen, nicht je Ueberweisung.
//
// Mit Speicherkorb (speicherkorb.go) schreibt ein Flush keine Zeile in
// pending_txs mehr, nur noch die Kontostaende -- und die liest er frisch aus
// cs.accounts (Momentaufnahme unter den Shard-Sperren), nicht aus dem
// Eintrag. Ein Eintrag sagt nur noch: "diese zwei Konten muessen nach
// Postgres". Steht fuer BEIDE Konten schon ein Eintrag in der Warteschlange,
// der noch nicht entnommen ist, traegt dessen Flush auch diese Ueberweisung
// mit: entnommen wird er erst nach dem Einreihen hier, die Momentaufnahme
// folgt der Entnahme, angewendet wurde die Ueberweisung vor dem Einreihen.
// Ein zweiter Eintrag waere reine Arbeit fuer Postgres und Platz in der
// Warteschlange (Pruefstand Lauf 7: 927.000 Ablehnungen wegen WAL-Druck).
//
// Nur fuer Eintraege ohne Outbox-Zeile. Eintraege MIT Zeile tragen eine
// Ueberweisung, die sonst in keinem Block landet -- die werden nie
// zusammengefasst.
//
// Entnahme und Pruefung laufen beide unter walFlushMu: zwischen "Konto steht
// in einem wartenden Eintrag" und "ueberspringen" kann der Eintrag nicht
// entnommen werden. Scheitert ein Flush, kommen seine Eintraege zurueck in
// die Warteschlange und zaehlen wieder (walZurueckLocked) -- die
// uebersprungenen Ueberweisungen sind damit weiter gedeckt.
//
// Grenzen: walInSchlange hat hoechstens zwei Schluessel je wartendem
// Eintrag, also <= 2 * walFlushMaxQueueDepth; walUnterwegsMin einen je
// laufendem Flush (<= walFlushConcurrency, plus FlushWALNow).
//
// WAL-KUERZUNG. Die alte Rechnung (Kopf - Laenge der Warteschlange -
// Abstand) setzt einen Eintrag je Datensatz voraus. Mit Zusammenfassen ist
// die Warteschlange kuerzer als die Zahl der offenen Datensaetze -- die
// Rechnung allein waere zu grosszuegig. Deshalb begrenzt die kleinste Seq,
// die noch in der Warteschlange oder in einem laufenden Flush steht
// (walOffenMinSeq), zusaetzlich: kein Datensatz ab (kleinste offene Seq -
// Abstand) wird gekuerzt. Eine uebersprungene Ueberweisung ist durch einen
// Eintrag gedeckt, der VOR ihrer Pruefung eingereiht wurde; dessen Seq
// liegt hoechstens um die Zahl gleichzeitig Anwendender ueber ihrer --
// weit unter dem Abstand von 300.000.

// walFlushZusammengefasst: wie viele Ueberweisungen keinen eigenen Eintrag
// brauchten.
var walFlushZusammengefasst atomic.Int64

// walSchlangeZaehlenLocked: die Konten eines Eintrags stehen (d=+1) oder
// stehen nicht mehr (d=-1) in einem wartenden Eintrag. walFlushMu gehalten.
func (cs *ChainState) walSchlangeZaehlenLocked(it walFlushItem, d int) {
	if cs.walInSchlange == nil {
		cs.walInSchlange = map[string]int{}
	}
	for _, a := range [2]string{it.from, it.to} {
		n := cs.walInSchlange[a] + d
		if n <= 0 {
			delete(cs.walInSchlange, a)
		} else {
			cs.walInSchlange[a] = n
		}
	}
}

// walGedecktLocked: traegt ein wartender Eintrag diese Ueberweisung mit?
// walFlushMu gehalten.
func (cs *ChainState) walGedecktLocked(it walFlushItem) bool {
	if !it.ohneOutbox {
		return false
	}
	return cs.walInSchlange[it.from] > 0 && cs.walInSchlange[it.to] > 0
}

func batchMinSeq(batch []walFlushItem) uint64 {
	var m uint64
	for i, it := range batch {
		if i == 0 || it.seq < m {
			m = it.seq
		}
	}
	return m
}

// walGenommenLocked: ein Buendel wurde aus der Warteschlange entnommen.
// walFlushMu gehalten.
func (cs *ChainState) walGenommenLocked(batch []walFlushItem) {
	if len(batch) == 0 {
		return
	}
	for _, it := range batch {
		cs.walSchlangeZaehlenLocked(it, -1)
	}
	if cs.walUnterwegsMin == nil {
		cs.walUnterwegsMin = map[uint64]int{}
	}
	cs.walUnterwegsMin[batchMinSeq(batch)]++
}

// walAbgeschlossenLocked: der Flush des Buendels ist vorbei (geschrieben
// oder gescheitert). walFlushMu gehalten.
func (cs *ChainState) walAbgeschlossenLocked(batch []walFlushItem) {
	if len(batch) == 0 {
		return
	}
	m := batchMinSeq(batch)
	if n := cs.walUnterwegsMin[m] - 1; n <= 0 {
		delete(cs.walUnterwegsMin, m)
	} else {
		cs.walUnterwegsMin[m] = n
	}
}

// walZurueckLocked: ein gescheitertes Buendel steht wieder in der
// Warteschlange. walFlushMu gehalten.
func (cs *ChainState) walZurueckLocked(batch []walFlushItem) {
	for _, it := range batch {
		cs.walSchlangeZaehlenLocked(it, +1)
	}
	cs.walAbgeschlossenLocked(batch)
}

// walOffenMinSeq: kleinste Seq, die noch in der Warteschlange oder in einem
// laufenden Flush steht; ok=false, wenn nichts offen ist.
func (cs *ChainState) walOffenMinSeq() (min uint64, ok bool) {
	cs.walFlushMu.Lock()
	defer cs.walFlushMu.Unlock()
	for _, it := range cs.walFlushQueue {
		if !ok || it.seq < min {
			min, ok = it.seq, true
		}
	}
	for s := range cs.walUnterwegsMin {
		if !ok || s < min {
			min, ok = s, true
		}
	}
	return min, ok
}
