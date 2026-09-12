package wal

import (
	"testing"
	"time"
)

// Die Perzentile muessen die Frage beantworten, wegen der es sie gibt: liegt
// das Mittel hoch, weil jeder Sync langsam ist, oder weil wenige sehr langsam
// sind? Der Test baut genau den zweiten Fall und prueft, dass er als solcher
// herauskommt.
func TestSyncVerteilung_ErkenntSeltenenAusreisserImMittel(t *testing.T) {
	syncRing.mu.Lock()
	syncRing.pos, syncRing.voll = 0, false
	syncRing.mu.Unlock()

	// 99 Syncs zu 0,6 ms, einer zu 500 ms: Mittel rund 5,6 ms.
	for i := 0; i < 99; i++ {
		merkeSyncDauer(600 * time.Microsecond)
	}
	merkeSyncDauer(500 * time.Millisecond)

	v := SyncVerteilung()
	if got := v["p50_us"].(int64); got != 600 {
		t.Errorf("p50 = %d us, erwartet 600 -- der Median darf den Ausreisser nicht sehen", got)
	}
	if got := v["mittel_us"].(int64); got < 5000 {
		t.Errorf("mittel = %d us, erwartet rund 5600 -- der Ausreisser muss das Mittel heben", got)
	}
	if got := v["anteil_mittel_aus_top1_pct"].(float64); got < 85 {
		t.Errorf("anteil_mittel_aus_top1_pct = %.0f, erwartet ueber 85: fast das ganze Mittel "+
			"stammt aus dem einen Ausreisser, und genau das soll die Zahl zeigen", got)
	}
	if got := v["max_us"].(int64); got != 500000 {
		t.Errorf("max = %d us, erwartet 500000", got)
	}
}

func TestSyncVerteilung_LeerTeiltNichtDurchNull(t *testing.T) {
	syncRing.mu.Lock()
	syncRing.pos, syncRing.voll = 0, false
	syncRing.mu.Unlock()
	if got := SyncVerteilung()["anzahl"].(int); got != 0 {
		t.Errorf("anzahl = %d auf leerem Ring", got)
	}
}
