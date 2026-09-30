package keeper

// LP-Anteile zaehlen zur Umlaufabgabe (Wirtschaftspruefung 29.09.2026, C2).
//
// Seit der Aktivierung der Wirtschaftsregeln ersetzt die Umlaufabgabe die
// alte Demurrage. Die Demurrage zaehlte LP-Anteile mit (Fix vom 20.08.), die
// Umlaufabgabe las nur acc.Balance. Wer 10.000 AEQ samt passendem tUSD in den
// Pool legte, halbierte damit seine Abgabe und bekam obendrein 30 % der
// Swap-Gebuehren -- der wirksamste Weg um die Regel herum.
//
// Ab umlaufMitLPAbUnix ist die Grundlage der Abgabe Guthaben + Wert der
// LP-Anteile, wie bei der Vermoegensgrenze (lpValueLockedAEQ). Fehlt beim
// Einzug Guthaben, loest applyUmlaufDeltaLocked den Rest aus den LP-Anteilen
// (releaseLPForAEQ). Beides rechnet jeder Knoten selbst aus demselben
// Pool-Stand; im Block steht wie bisher nur der Betrag.
//
// Konsens: Erzeuger (umlaufLocked), Pruefung (pruefeUmlaufLocked) und
// Einzug (applyUmlaufDeltaLocked) entscheiden ueber den Zeitpunkt der Runde
// (DistributionAt), nicht ueber die eigene Uhr. Aeltere Runden spielen
// unveraendert nach.

// umlaufMitLPAbUnix: 07.10.2026 00:00 UTC -- eine Woche nach dem Einbau.
const umlaufMitLPAbUnix int64 = 1791331200

// umlaufLPZeitVorgezogen: eine Umlauf-Transaktion beansprucht die neue Regel
// (Rundenzeit ab umlaufMitLPAbUnix), liegt aber nach ihrem eigenen Block. Der
// Erzeuger waehlt die Rundenzeit, bevor er den Block baut -- ehrlich kommt
// das nie vor, auch nicht in der Geschichte (vor dem Stichtag liegt jede
// Rundenzeit davor). Ohne diese Grenze konnte ein Erzeuger die Regel mit einer
// vordatierten Rundenzeit vorziehen und LP-Anteile aufloesen lassen
// (Sicherheitspruefung #238, M1). Eine Minute Uhrenspiel ist erlaubt.
func umlaufLPZeitVorgezogen(at, blockZeit int64) bool {
	return at >= umlaufMitLPAbUnix && at > blockZeit+60
}

// umlaufStandLocked: der Stand, auf den die Umlaufabgabe zur Runde at
// berechnet wird. Caller haelt cs.mu (lesend genuegt).
func (cs *ChainState) umlaufStandLocked(acc *AccountState, at int64) float64 {
	if acc == nil {
		return 0
	}
	s := acc.Balance.Float()
	if at >= umlaufMitLPAbUnix {
		s += cs.lpValueLockedAEQ(acc)
	}
	return s
}
