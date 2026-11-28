package keeper

import "testing"

// PROBELAUF WEG B (docs/PILOT_WEG_B.md): Quantengeld, das viele Menschen mit
// ihrem Handy praeparieren, von der Bestellung bis zum Einloesen.
//
//   - 20 gemeldete Praeparatorinnen; die Kette lost mit drand v = 5 aus, die
//     Schwelle ist t = 3. Geloste x praepariert Abschnitt x des Tokens
//     (quantenSAbschnitt) und gibt nur Anteile weiter: weniger als t Geloste
//     kennen nur ihre eigenen Abschnitte, t setzen jeden Wert zusammen.
//   - Die Inhaberin misst in ihren Basen; Bindung und Wahl auf der Kette.
//   - DOPPELVORLAGE: dieselbe Messung an der anderen Stelle -- abgewiesen.
//   - ABSPRACHE UNTER DER SCHWELLE: die Inhaberin im Bund mit t-1 = 2
//     Gelosten kennt deren Abschnitte genau und raet den Rest -- abgewiesen.
//   - GEGENPROBE: im Bund mit t = 3 Gelosten (sie setzen alle Werte zusammen)
//     bestuende sie. Genau die Schwelle ist die Grenze.
//   - Danach loest die ehrliche Vorlage an der gewaehlten Stelle ein.
//
// Das ist der Schutz von Weg B: kein Einzelner -- auch nicht der Betreiber oder
// eine Geloste -- kann faelschen, nur t im Bund. Physik (Weg C) ist es nicht.

const (
	bwProbeN     = 400 // Stellen des Tokens
	bwProbeStich = 80  // Stellen je Stichprobe
)

func TestQuantenSWegBProbelauf(t *testing.T) {
	for _, fall := range []struct {
		name   string
		imBund int    // Geloste im Bund mit der Inhaberin
		urteil string // an der anderen Stelle
		danach bool   // ehrlich an der gewaehlten Stelle einloesen
	}{
		{"Doppelvorlage ohne Bund", 0, "abgewiesen", true},
		{"Absprache unter der Schwelle (t-1 = 2)", 2, "abgewiesen", true},
		{"Gegenprobe: Absprache mit t = 3", 3, "echt", false},
	} {
		t.Run(fall.name, func(t *testing.T) {
			w := neueBWeltN(t, bwProbeN, bwProbeStich)
			los := w.bestelltUndGelost()
			if len(los) != 5 {
				t.Fatalf("Los: %v", los)
			}
			z := w.z + 900
			w.annehmen(w.ausgabeMit(los, z), z)
			w.annehmen(w.bindenTx(w.anna), z+1)
			w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+2)

			// Gewaehlte Stelle 0: dort prueft die Kette die gemessenen Basen.
			// Die andere Stelle prueft die Gegenbasen. Was die Inhaberin dort
			// vorlegt: ihre Messung, an den Stellen der Gelosten im Bund aber
			// den echten Wert (die kennt der Bund).
			ehrlich := append([]int(nil), w.x...)
			gefaelscht := append([]int(nil), w.x...)
			bekannt := 0
			if fall.imBund >= w.p.t { // t setzen jeden Wert aus den Anteilen zusammen
				copy(gefaelscht, w.p.bits)
				bekannt = bwProbeN
			} else {
				for x := 1; x <= fall.imBund; x++ {
					von, bis, ok := quantenSAbschnitt(bwProbeN, 5, x)
					if !ok {
						t.Fatal("Abschnitt")
					}
					copy(gefaelscht[von:bis], w.p.bits[von:bis])
					bekannt += bis - von
				}
			}

			w.x = gefaelscht
			w.annehmen(w.anmeldenTx(w.stelle[1], 1, w.probe[1], w.vorlage(1, w.keys[w.anna])), z+3)
			g := w.offenlegenAlle(1, z+4)
			if g.Urteil != fall.urteil {
				t.Fatalf("andere Stelle (%d von %d Stellen bekannt): %+v, erwartet %s", bekannt, bwProbeN, g, fall.urteil)
			}
			if !fall.danach {
				return
			}
			w.x = ehrlich
			vorher := w.stand(w.cs, w.stelle[0])
			w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.anna])), z+5)
			if g := w.offenlegenAlle(0, z+6); g.Urteil != "echt" || g.WertAn != w.stelle[0] {
				t.Fatalf("gewaehlte Stelle: %+v", g)
			}
			if w.stand(w.cs, w.stelle[0]) <= vorher || w.token(w.cs) != nil {
				t.Fatal("Deckung nicht bei der Stelle oder Token nicht verbraucht")
			}
			if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
				t.Fatal("Knoten weichen ab")
			}
			t.Logf("%s: andere Stelle %s (%d von %d Stellen bekannt), gewaehlte echt", fall.name, g.Urteil, bekannt, bwProbeN)
		})
	}
}
