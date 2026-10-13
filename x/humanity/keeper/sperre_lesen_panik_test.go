package keeper

import (
	"path/filepath"
	"testing"
)

// OFFEN 7b: readAccount ruft fn unter der Lesesperre auf. Paniert fn (ein
// Fehler in einem Aufrufer), muss die Sperre trotzdem frei werden -- sonst
// stuende jeder Schreiber, und der Knoten mit ihm.
func TestReadAccount_PanikGibtSperreFrei(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	cs := testKnoten(t, filepath.Join(t.TempDir(), "zustand.json"))
	adr := "0x00000000000000000000000000000000000000aa"
	cs.mu.Lock()
	cs.accounts.Set(adr, &AccountState{Address: adr, Balance: NewDecimal(1)})
	cs.mu.Unlock()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("fn wurde nicht aufgerufen -- der Test prueft nichts")
			}
		}()
		cs.readAccount(adr, func(*AccountState) { panic("Fehler im Aufrufer") })
	}()
	if !cs.mu.TryLock() {
		t.Fatal("cs.mu bleibt nach der Panik gesperrt")
	}
	cs.mu.Unlock()
}
