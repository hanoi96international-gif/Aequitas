package keeper

// Wirtschaftsregeln im WAL-Schnellpfad (26.09.2026).
//
// Bis hierhin traten ab der Aktivierung (wirtschaftAktiv) alle schnellen
// Pfade zurueck, damit die Regeln an genau einer Stelle stehen
// (transferMutateLocked). Das kostete den Durchsatz: jede Ueberweisung lief
// seriell. Die Regeln selbst sind aber billig und lokal -- jede braucht nur
// Absender und Empfaenger:
//
//   - Gebuehr: Monatsfreibetrag des Absenders (gebuehrMitWirtschaft)
//   - Grenze freier Adressen: Stand des Empfaengers (pruefeEmpfaengerWirtschaft)
//   - Buchfuehrung: Zaehler von Absender und Empfaenger (nachUeberweisung)
//
// Der Schnellpfad uebernimmt jetzt die haeufigen Faelle: Menschen und freie
// Adressen untereinander. Dort bucht nachUeberweisung nur zweierlei -- beide
// Buchkonten auf den laufenden Monat bringen und beim Menschen als Absender
// Ausgegeben erhoehen. Ist ein Unternehmen oder ein Protokoll-Topf beteiligt
// (Umsatz, Lohn, Rueckzahlung, Quartalszaehler), geht die Ueberweisung
// weiter den seriellen Weg: dort haengen Buchungen beider Seiten voneinander
// ab, und das hier waere die zweite Stelle, die sie kennen muesste.
//
// Was in den Block kommt, ist dasselbe wie im seriellen Pfad: Gebuehr nach
// denselben Regeln, BuchAt = Annahmezeitpunkt. Nachspielende Knoten sehen
// keinen Unterschied, deshalb braucht es keinen gemeinsamen Stichtag.
//
// ABSTURZ. Die Buchfuehrung wird wie die Kontostaende im WAL-Flush in
// derselben Datenbank-Transaktion geschrieben (flushWALBatch). Nach einem
// Absturz spielt recoverFromWAL die ungeflushten Saetze nach; damit Ausgegeben
// nicht doppelt zaehlt, traegt das Buchkonto die WAL-Folgenummer der letzten
// Ueberweisung, die es enthaelt (buchKonto.WS) -- dasselbe Verfahren wie
// AccountState.WALSeq fuer den Kontostand.

// wirtschaftSchnellArten: Kontoarten, wenn der Schnellpfad die Ueberweisung
// mit aktiven Wirtschaftsregeln nehmen darf; ok=false heisst zurueck auf den
// seriellen Weg.
func (cs *ChainState) wirtschaftSchnellArten(from, to string, fromMensch, toMensch bool) (fromArt, toArt kontoart, ok bool) {
	fromArt = cs.kontoartVon(from, fromMensch)
	toArt = cs.kontoartVon(to, toMensch)
	if fromArt == artUnternehmen || toArt == artUnternehmen || fromArt == artSystem || toArt == artSystem {
		return fromArt, toArt, false
	}
	return fromArt, toArt, true
}

// buchSchnell: genau das, was nachUeberweisung fuer Menschen und freie
// Adressen bucht. seq: WAL-Folgenummer dieser Ueberweisung.
func (cs *ChainState) buchSchnell(from, to string, fromArt kontoart, amount float64, at int64, seq uint64) {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	fk := w.kontoLocked(from, at)
	w.kontoLocked(to, at)
	if fromArt == artMensch {
		fk.Ausgegeben += amount
	}
	if seq > fk.WS {
		fk.WS = seq
	}
}

// buchWiederherstellen: buchSchnell beim Nachspielen des WAL nach einem
// Absturz. Enthaelt das gespeicherte Buchkonto des Absenders diese
// Ueberweisung schon (WS >= seq), bleibt es unveraendert. Monate werden hier
// nie zurueckgestellt: ein Buchkonto, das schon im Folgemonat steht, hat
// spaetere Saetze -- und damit diesen -- bereits enthalten.
func (cs *ChainState) buchWiederherstellen(from, to string, fromMensch bool, amount float64, at int64, seq uint64) (gebucht bool) {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	m := monatVon(at)
	vorwaerts := func(addr string) *buchKonto {
		if k := w.buch[addr]; k != nil && k.Monat > m {
			return k
		}
		return w.kontoLocked(addr, at)
	}
	vorwaerts(to)
	if k := w.buch[from]; k != nil && k.WS >= seq {
		return false
	}
	fk := vorwaerts(from)
	if fromMensch {
		fk.Ausgegeben += amount
	}
	fk.WS = seq
	return true
}
