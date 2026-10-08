package keeper

import (
	"context"
	"encoding/hex"
	"testing"
	"time"
)

// Missbrauch (Pruefung #314, LOW-3, und von #319): die Registerabfrage der
// Erneuerung wartet hoechstens 5 s auf eine Verbindung -- eine ausgeschoepfte
// Datenbank haelt keine oeffentliche Anfrage unbegrenzt fest. Fail-closed:
// ohne Antwort keine Bindung.
func TestCoordinatorBindungLokal_Zeitgrenze_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pubHex := hex.EncodeToString(f.pub)
	if _, ok := f.cs.CoordinatorBindungLokal(pubHex); !ok {
		t.Fatal("Vorbedingung: der Schluessel ist eingetragen")
	}
	vorher := f.cs.db.Stats().MaxOpenConnections
	f.cs.db.SetMaxOpenConns(1)
	t.Cleanup(func() { f.cs.db.SetMaxOpenConns(vorher) })
	belegt, err := f.cs.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer belegt.Close()
	fertig := make(chan bool, 1)
	go func() {
		_, ok := f.cs.CoordinatorBindungLokal(pubHex)
		fertig <- ok
	}()
	select {
	case ok := <-fertig:
		if ok {
			t.Fatal("Bindung gelesen, obwohl keine Verbindung frei war")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("die Registerabfrage wartet ohne Zeitgrenze auf eine Verbindung")
	}
}
