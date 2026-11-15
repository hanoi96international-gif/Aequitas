package keeper

import "strings"

// Audit 14.11.2026, B5: Lesefehler der Datenbank gilt nicht mehr als "Konto
// gibt es nicht".
//
// ensureAccountLoadedCtx kehrt bei einem echten Datenbankfehler ohne Fehler
// zurueck (seine rund 36 Aufrufer kennen keinen). Der Aufrufer legt dann ein
// frisches Konto an, und das Speichern einer Version 0 ist ein UPSERT, das
// eine vorhandene Zeile ueberschriebe. Deshalb merkt sich der Knoten die
// Adresse, und saveAccountToDBInnerCtx weist ein frisches Konto fuer sie ab,
// bis ein Lesen wieder gelingt (dann kontoLesbar).
//
// Begrenzt (AGENTS.md, Regel 2): hoechstens kontoUnlesbarHoechstens
// Adressen. Laeuft die Liste ueber -- die Datenbank ist dann offenbar ganz
// weg --, gilt jede Adresse als unlesbar, bis die Liste wieder unter die
// Haelfte faellt. Fail-closed: lieber eine Buchung abweisen als ein Konto
// ueberschreiben.
const kontoUnlesbarHoechstens = 100_000

func kontoSchluessel(addr string) string { return strings.ToLower(strings.TrimSpace(addr)) }

func (cs *ChainState) kontoUnlesbarMerken(addr string) {
	if cs.unlesbareAnzahl.Load() >= kontoUnlesbarHoechstens {
		cs.unlesbarUeberlauf.Store(true)
		return
	}
	if _, schon := cs.unlesbareKonten.LoadOrStore(kontoSchluessel(addr), struct{}{}); !schon {
		cs.unlesbareAnzahl.Add(1)
	}
}

func (cs *ChainState) kontoLesbar(addr string) {
	if _, war := cs.unlesbareKonten.LoadAndDelete(kontoSchluessel(addr)); war {
		if n := cs.unlesbareAnzahl.Add(-1); n < kontoUnlesbarHoechstens/2 {
			cs.unlesbarUeberlauf.Store(false)
		}
	}
}

func (cs *ChainState) kontoUnlesbar(addr string) bool {
	if cs.unlesbarUeberlauf.Load() {
		return true
	}
	_, da := cs.unlesbareKonten.Load(kontoSchluessel(addr))
	return da
}
