package keeper

import (
	"testing"
	"time"
)

// Die Sperre der Selbstheilung muss drei Faelle unterscheiden -- der dritte
// hat am 02.09.2026 den PRIMARY eine halbe Stunde lang unten gehalten.
func TestAutoHealCooldown_DreiFaelle(t *testing.T) {
	jetzt := time.Now().Unix()

	t.Run("nie resynct: volle Sperre, der Aufrufer ueberspringt sie ohnehin", func(t *testing.T) {
		dag := &BlockDAG{}
		d, gekuerzt := dag.effectiveAutoHealCooldown(0)
		if d != autoHealCooldown || gekuerzt {
			t.Fatalf("bekam %s (gekuerzt=%v), erwartet die volle Sperre", d, gekuerzt)
		}
	})

	t.Run("letzter Resync brachte nichts: gekuerzt", func(t *testing.T) {
		dag := &BlockDAG{}
		dag.lastSuccessfulPeerSyncAt.Store(jetzt - 600) // Merge VOR dem Resync
		d, gekuerzt := dag.effectiveAutoHealCooldown(jetzt - 300)
		if d != autoHealFailedResyncRetry || !gekuerzt {
			t.Fatalf("bekam %s (gekuerzt=%v), erwartet die gekuerzte Sperre -- ein "+
				"Resync, der nichts angehaengt hat, muss wiederholbar sein", d, gekuerzt)
		}
	})

	t.Run("Resync wirkte, mergt aber SEIT LANGEM nichts mehr: gekuerzt", func(t *testing.T) {
		// DER FALL, DER GEFEHLT HAT. C1 wurde resynct, lief danach sauber --
		// und wurde spaeter erneut zugemauert, weil eine einzelne
		// Ueberweisung beim Nachspielen scheiterte und daraufhin ein ganzer
		// Block mit 3.525 Transaktionen verworfen wurde. Die alte Pruefung
		// sah nur "der Resync hat gemergt" und verhaengte 30 Minuten, waehrend
		// der Knoten selbst "structurally cut off" meldete und 671 Bloecke
		// zurueckstand -- als Primary.
		dag := &BlockDAG{}
		dag.lastSuccessfulPeerSyncAt.Store(jetzt - int64(syncStarvationThreshold.Seconds()) - 60)
		d, gekuerzt := dag.effectiveAutoHealCooldown(jetzt - 1800)
		if d != autoHealFailedResyncRetry || !gekuerzt {
			t.Fatalf("bekam %s (gekuerzt=%v), erwartet die gekuerzte Sperre -- ein "+
				"Knoten, der seit ueber %s nichts mehr anhaengt, ist erneut "+
				"abgehaengt, egal wie gut der letzte Resync lief",
				d, gekuerzt, syncStarvationThreshold)
		}
	})

	t.Run("Resync wirkte und der Knoten mergt weiter: volle Sperre", func(t *testing.T) {
		// Die Schutzwirkung muss bleiben: ein Knoten, der langsam aber
		// erfolgreich aufholt, darf NICHT alle drei Minuten aus der Kette
		// gerissen werden.
		dag := &BlockDAG{}
		dag.lastSuccessfulPeerSyncAt.Store(jetzt - 10) // gerade eben gemergt
		d, gekuerzt := dag.effectiveAutoHealCooldown(jetzt - 1800)
		if d != autoHealCooldown || gekuerzt {
			t.Fatalf("bekam %s (gekuerzt=%v), erwartet die VOLLE Sperre -- wer "+
				"gerade mergt, holt auf und darf nicht geresynct werden", d, gekuerzt)
		}
	})
}
