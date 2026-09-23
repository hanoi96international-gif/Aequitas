package keeper

import "testing"

// testKnoten baut einen Knoten wie NewChainState und schliesst am Testende
// seine Datenbankverbindung.
//
// WARUM. NewChainState startet Hintergrundarbeiter (Pool-Flush, EVM-Spiegel,
// Quittungen), und ChainState hat keine Methode, sie anzuhalten. Ein Test, der
// seinen Knoten offen liess, schrieb deshalb nach seinem Ende weiter in die
// gemeinsame Test-Datenbank -- auch nach dem TRUNCATE des naechsten Tests.
// Belegt am 23.09.2026: der Erhaltungstest der Schnellpfade fiel in der
// CI-Gruppe in 8 von 12 Laeufen mit +0.015994 AEQ, geschrieben vom Pool-Flush
// eines fremden, laengst beendeten Tests (supply_conservation_test.go,
// eigeneDatenbank).
//
// Mit geschlossener Verbindung scheitern die verwaisten Arbeiter harmlos an
// "database is closed", statt fremde Zeilen zu schreiben.
func testKnoten(t testing.TB, datei string) *ChainState {
	t.Helper()
	cs := NewChainState(datei)
	t.Cleanup(func() {
		if cs.db != nil {
			cs.db.Close()
		}
	})
	return cs
}
