package keeper

import (
	"testing"
	"time"
)

// Zehn Leute an einem Tisch im selben WLAN sind EINE Adresse -- sie duerfen
// sich nicht gegenseitig aussperren. Der Elfte im selben Fenster wartet.
func TestBurstErlaubtGruppenHinterEinerAdresse(t *testing.T) {
	key := "test:" + t.Name()
	for i := 0; i < 10; i++ {
		if !burstErlaubt(key, 10, time.Minute) {
			t.Fatalf("Anfrage %d muss durch", i+1)
		}
	}
	if burstErlaubt(key, 10, time.Minute) {
		t.Fatal("die elfte im Fenster muss warten")
	}
	// Ein anderer Schluessel ist unbeeindruckt.
	if !burstErlaubt(key+":andere", 10, time.Minute) {
		t.Fatal("andere Adresse blockiert")
	}
}

func TestBurstFensterGleitet(t *testing.T) {
	key := "test:" + t.Name()
	if !burstErlaubt(key, 1, 30*time.Millisecond) || burstErlaubt(key, 1, 30*time.Millisecond) {
		t.Fatal("erste durch, zweite sofort nicht")
	}
	time.Sleep(40 * time.Millisecond)
	if !burstErlaubt(key, 1, 30*time.Millisecond) {
		t.Fatal("nach dem Fenster wieder frei")
	}
	time.Sleep(5 * time.Millisecond)
	ipBurstAufraeumen(time.Millisecond)
	if _, ok := ipBurst.Load(key); ok {
		t.Fatal("Aufraeumen muss verfallene Schluessel entfernen")
	}
}
