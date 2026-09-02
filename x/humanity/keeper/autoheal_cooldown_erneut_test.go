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

// Der Selbstheilungs-Vergleich darf sich nicht ausgerechnet dann abschalten,
// wenn der Knoten am kaputtesten ist.
//
// Bisher uebersprang er sich, solange der Knoten "unsettled" war (mehrere
// DAG-Tips), und ueberbrueckte das erst nach 45 Minuten. Am 02.09.2026
// sammelte C1 -- der PRIMARY -- unter Last Tips (9, 10, 11, 12 ...) und die
// Heilung meldete im Minutentakt "skipped this round ... not a settled
// state", waehrend er weiter zurueckfiel. Tip-Fragmentierung ist nicht der
// Grund, NICHT zu pruefen -- sie ist das Symptom.
func TestAutoHeal_AbgeschnittenSchlaegtUnsettled(t *testing.T) {
	jetzt := time.Now().Unix()

	t.Run("nichts angehaengt seit ueber der Schwelle: abgeschnitten", func(t *testing.T) {
		dag := &BlockDAG{}
		dag.lastHeightAdvanceAt.Store(jetzt - int64(syncStarvationThreshold.Seconds()) - 30)
		if !dag.hatSeitLangemNichtsAngehaengt() {
			t.Fatalf("ein Knoten, der seit ueber %s nichts angehaengt hat, muss als "+
				"abgeschnitten gelten -- sonst sieht die Heilung 45 Minuten lang zu",
				syncStarvationThreshold)
		}
	})

	t.Run("gerade eben angehaengt: nicht abgeschnitten", func(t *testing.T) {
		dag := &BlockDAG{}
		dag.lastHeightAdvanceAt.Store(jetzt - 5)
		if dag.hatSeitLangemNichtsAngehaengt() {
			t.Fatal("wer gerade merged, holt auf und darf nicht als abgeschnitten " +
				"gelten -- sonst wird ein gesunder Knoten aus der Kette gerissen")
		}
	})

	t.Run("frisch gestartet, nie gemergt: NICHT abgeschnitten", func(t *testing.T) {
		// Wichtig: ein Knoten, der gerade hochgekommen ist, hat noch nie
		// angehaengt. Das ist kein Beleg fuer Abgeschnittensein, und ihn
		// deswegen sofort zu resyncen waere falsch.
		dag := &BlockDAG{}
		if dag.hatSeitLangemNichtsAngehaengt() {
			t.Fatal("ein frisch gestarteter Knoten darf nicht als abgeschnitten gelten")
		}
	})
}

// DER FEHLER, DEN DER ERSTE VERSUCH HATTE: ein abgehaengter Knoten haengt
// laufend Bloecke an, die als Waisen liegenbleiben. "Zuletzt einen Block
// angehaengt" sieht dabei dauerhaft frisch aus, waehrend die Hoehe stillsteht.
// Am 02.09.2026 stand C1 so 1.400 Bloecke zurueck, Hoehe seit zwanzig Minuten
// unveraendert -- und die Selbstheilung meldete weiter "not a settled state".
func TestAutoHeal_HoeheIstDerBelegNichtDasAnhaengen(t *testing.T) {
	jetzt := time.Now().Unix()
	dag := &BlockDAG{}

	// Bloecke kommen an und werden angehaengt -- aber die Hoehe steht.
	dag.lastSuccessfulPeerSyncAt.Store(jetzt - 1) // gerade eben "gemergt"
	dag.lastHeightAdvanceAt.Store(jetzt - int64(syncStarvationThreshold.Seconds()) - 60)

	if !dag.hatSeitLangemNichtsAngehaengt() {
		t.Fatal("der Knoten haengt zwar Bloecke an, kommt aber seit ueber der Schwelle " +
			"nicht voran -- genau das ist ein abgehaengter Knoten, und genau das hat " +
			"der erste Versuch uebersehen")
	}
}

// setHeight muss den Fortschritts-Zeitstempel setzen, und NUR bei echtem
// Fortschritt.
func TestSetHeight_StempeltNurEchtenFortschritt(t *testing.T) {
	dag := &BlockDAG{}
	dag.setHeight(100)
	ersterStempel := dag.lastHeightAdvanceAt.Load()
	if ersterStempel == 0 {
		t.Fatal("setHeight hat den Fortschritts-Zeitstempel nicht gesetzt")
	}
	// Rueckwaerts oder gleich: kein Fortschritt, kein neuer Stempel.
	dag.lastHeightAdvanceAt.Store(1)
	dag.setHeight(100)
	dag.setHeight(50)
	if dag.lastHeightAdvanceAt.Load() != 1 {
		t.Fatal("setHeight stempelt auch ohne Fortschritt -- dann sieht ein " +
			"stillstehender Knoten dauerhaft gesund aus")
	}
	// Vorwaerts: neuer Stempel.
	dag.setHeight(101)
	if dag.lastHeightAdvanceAt.Load() == 1 {
		t.Fatal("setHeight stempelt echten Fortschritt nicht")
	}
}
