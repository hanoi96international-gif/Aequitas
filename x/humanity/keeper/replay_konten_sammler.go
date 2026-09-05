package keeper

import "context"

// Ein Schreibvorgang je BLOCK statt je Buendel -- der Schritt, der die
// Datenbankkosten des Nachspielens von der Zahl der UEBERWEISUNGEN auf die
// Zahl der beruehrten KONTEN umstellt.
//
// # DIE MESSUNG, DIE DAS NOETIG MACHT
//
// Auf dem Primary unter Last (replay_phasen, 429 Bloecke, 05.09.2026) haelt
// das Nachspielen die globale Sperre 292,3 ms je Block, und 87,8 % davon sind
// zwei Posten, die beide nichts anderes tun als Konten zu schreiben:
//
//	seriell    134,79 ms   46,1 %
//	parallel   127,76 ms   43,7 %
//
// Aus diesen Zahlen laesst sich die Kostenstruktur eines Schreibvorgangs
// ableiten. Ein Buendelschreiben deckte im Mittel 166 Konten in 23,9 ms, ein
// einzelnes Konto kostete 2,46 ms. Beides zusammen ergibt rund
//
//	2,4 ms fester Aufwand je Statement + 0,13 ms je Konto
//
// Der feste Anteil ist also der teurere, sobald man mehr als ein paar
// Statements absetzt -- und genau das tat der Block: 5,35 Buendel, jedes mit
// eigenem Statement, plus je ein Statement fuer jede seriell nachgespielte
// Ueberweisung.
//
// # WARUM DAS UEBER 10k ENTSCHEIDET
//
// Bei 10.000 Ueberweisungen je Block und der bisherigen Struktur waeren es
// rund 20.000 Kontenzeilen -- die Kosten wachsen mit den Ueberweisungen. Die
// beruehrten Konten wachsen aber NICHT mit: dieselben Konten werden immer
// wieder angefasst. Ein einziges Statement je Block schreibt deshalb
// unabhaengig von der Last etwa so viele Zeilen, wie es aktive Konten gibt.
//
//	bisher   O(Ueberweisungen)   10.000 Ueberweisungen -> ~20.000 Zeilen
//	jetzt    O(Konten)           10.000 Ueberweisungen -> ~700 Zeilen
//
// Das ist der Unterschied zwischen "der Primary faellt unaufholbar zurueck"
// und "der Primary haelt mit". Live beobachtet am selben Abend: unter Last
// wuchs C1s Rueckstand auf C2 auf 180 Bloecke und stieg weiter, solange die
// Last lief.
//
// # WAS SICH NICHT AENDERT
//
// Der Schreibvorgang bleibt in derselben Datenbanktransaktion (dbTx) und
// damit in derselben Rollback-Einheit wie zuvor; er wandert nur ans Ende des
// Blocks. Die Reihenfolge spielt keine Rolle, weil er ohnehin den ENDSTAND
// jedes Kontos schreibt, und der steht im Speicher.
//
// Der StateRoot bleibt ebenfalls unberuehrt: er wird aus cs.accountSetXOR
// gebildet, den Phase 2 von applyTransferBatchParallel bereits je Konto
// fortschreibt (updateAccountLeafLocked) -- unabhaengig davon, wann die
// Zeile in die Datenbank geht.
//
// Versionen: gesammelt werden ZEIGER, nicht Kopien. saveAccountsToDBBatchCtx
// liest acc.Version beim Schreiben, also den dann aktuellen Wert. Schreibt
// der serielle Pfad dazwischen dasselbe Konto, ist dessen erhoehte Version
// hier bereits sichtbar, und die optimistische Sperre stimmt weiterhin.
type kontenSammler struct {
	konten    []*AccountState
	gesehen   map[string]bool
	aufrufe   int // wie viele Buendel zusammengelegt wurden -- fuer die Messung
	dedupiert int // wie viele Kontenberuehrungen die Dedup eingespart hat
}

func neuerKontenSammler() *kontenSammler {
	return &kontenSammler{gesehen: make(map[string]bool)}
}

// hinzufuegen nimmt die beruehrten Konten eines Buendels auf. Dieselbe
// Adresse zweimal aufzunehmen waere nicht nur verschwendet, sondern falsch:
// saveAccountsToDBBatchCtx wuerde denselben Zeiger zweimal mit derselben
// erwarteten Version schreiben, und der zweite Eintrag traefe dann eine
// Zeile, deren Version der erste bereits erhoeht hat -- ein
// Versionskonflikt, den es ohne die Dedup gar nicht gaebe.
func (s *kontenSammler) hinzufuegen(accs ...*AccountState) {
	for _, acc := range accs {
		if acc == nil {
			continue
		}
		if s.gesehen[acc.Address] {
			s.dedupiert++
			continue
		}
		s.gesehen[acc.Address] = true
		s.konten = append(s.konten, acc)
	}
	s.aufrufe++
}

func (s *kontenSammler) anzahl() int {
	if s == nil {
		return 0
	}
	return len(s.konten)
}

// schreiben setzt alle gesammelten Konten in EINEM Statement ab.
//
// Der Aufrufer MUSS dies vor dem StateRoot-Vergleich und vor dem Commit tun,
// und er MUSS es auslassen, wenn der Block ohnehin zurueckgerollt wird --
// dann ist der Speicher bereits durch restoreFromRollbackLocked zu heilen und
// ein Schreibvorgang waere nur zusaetzlicher Schaden.
func (cs *ChainState) sammlerSchreiben(ctx context.Context, s *kontenSammler) error {
	if s == nil || len(s.konten) == 0 {
		return nil
	}
	merkeSammlerSchreiben(s.aufrufe, len(s.konten), s.dedupiert)
	return cs.saveAccountsToDBBatchCtx(ctx, s.konten)
}
