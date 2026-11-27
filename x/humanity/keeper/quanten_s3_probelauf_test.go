package keeper

import (
	"encoding/hex"
	"math/rand"
	"testing"
)

// PROBELAUF WEG C (WEG_C.md): ein Token ueber eine Quantenleitung von der
// Vormerkung bis zum Einloesen, mit nachgebauten Sendern und Empfaenger.
//
//   - 5 Sender (= Validatoren), je ein Abschnitt der 20.000 Pulse; die Werte
//     (Basis, Bit) stehen vor dem ersten Puls in der Wurzel der Vormerkung.
//   - Empfaenger der Inhaberin: 70 % der Pulse gehen verloren, jeder
//     ankommende wird in einer ZUFAELLIGEN Basis gemessen (passt sie, das Bit
//     mit 1 % Fehler; sonst ein Zufallsbit -- mehr erfaehrt niemand aus einem
//     Photon).
//   - Melden (je Abschnitt >= gamma_det), Ziehen (drand), Ausgeben, Binden,
//     Waehlen.
//   - DIE PHYSIK: dieselbe Vorlage an der ANDEREN Stelle besteht nicht -- dort
//     prueft die Kette die Gegenbasen, und die hat die Inhaberin nie gemessen
//     (rund die Haelfte der Treffer falsch). An der gewaehlten Stelle: echt.
//
// Ein Nachbau schuetzt nichts; er zeigt nur, dass Kette und Ablauf
// zusammenpassen. Was ein echtes Geraet leistet, misst C5.

func TestQuantenS3Probelauf(t *testing.T) { s3Probelauf(t, false) }

// Gegenprobe: eine Inhaberin, die JEDES Bit kennt (was kein Photon hergibt --
// das waere eine Kopie), bestuende auch an der anderen Stelle. Die Abweisung
// oben kommt also vom fehlenden Wissen, nicht vom Testaufbau.
func TestQuantenS3Probelauf_GegenprobeAllwissend(t *testing.T) { s3Probelauf(t, true) }

func s3Probelauf(t *testing.T, allwissend bool) {
	w := &s3AWelt{s3Welt: neueS3WeltN(t, s3AN)}
	w.wurzel, w.sender = hex.EncodeToString(w.p.wurzel[:]), s3ASend
	quantenSKLeitungOverride.Store(s3AK)
	t.Cleanup(func() { quantenSKLeitungOverride.Store(0) })
	K := w.p.K

	// Empfaenger der Inhaberin.
	r := rand.New(rand.NewSource(1926))
	type messung struct{ basis, bit int }
	gemessen := map[int]messung{}
	var lam []int
	for p := 0; p < s3AN; p++ {
		if r.Float64() >= 0.30 { // verloren
			continue
		}
		b := r.Intn(2)
		x := r.Intn(2)
		if b == w.p.basen[p] || allwissend {
			x = w.p.bits[p]
			if !allwissend && r.Float64() < 0.01 {
				x ^= 1
			}
		}
		gemessen[p] = messung{b, x}
		lam = append(lam, p)
	}
	if err := quantenS3MeldungPruefenAbschnitte(s3AN, s3Gamma, lam, s3ASend); err != nil {
		t.Fatalf("Empfaenger meldet zu wenig: %v", err)
	}

	text := s3Text(t, lam)
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3AN, s3Deckung, w.z), w.z)
	w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], K, text), w.z+10)
	runde, _ := quantenS3ZiehRunde(w.z + 10)
	w.annehmen(w.ziehenTx(K, text, runde, w.sig(runde)), w.z+10+quantenS3ZiehAbstand+5)
	w.teil = append([]int(nil), w.vs(w.cs, K).Teil...)
	if len(w.teil) == 0 || len(w.teil)%s3ASend != 0 {
		t.Fatalf("Teilmenge: %d", len(w.teil))
	}

	// Stelle j des Tokens = Puls teil[j]: Basen und Messwerte der Inhaberin.
	w.messbasen, w.x = nil, nil
	for _, p := range w.teil {
		m := gemessen[p]
		w.messbasen, w.x = append(w.messbasen, m.basis), append(w.x, m.bit)
	}
	for b := 0; b < 2; b++ {
		w.probe[b] = eindeutigSortiert(r.Perm(len(w.teil))[:s3AProb])
	}

	z := w.z + quantenS3ZiehAbstand + 100
	w.annehmen(w.ausgebenS3Tx(z, w.praeger, nil), z)
	w.annehmen(w.bindenTx(w.anna), z+1)
	w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+2)

	// Die gewaehlte Stelle ist die, an der die Kette die gemessenen Basen
	// prueft (d xor b xor c = Messbasen): b = 0 (bindenTx legt d = Messbasen xor c).
	ehrlich, andere := 0, 1

	offenlegen := func(b int, zeit int64) QuantenWAngaben {
		t.Helper()
		var g QuantenWAngaben
		for ab := 0; ab < s3AProb; ab += s3ATeil {
			bis := min(ab+s3ATeil, s3AProb)
			g = w.annehmen(w.offenS3Tx(ab, w.probe[b][ab:bis], w.ehrlich, w.p.blaetter), zeit).QuantenW.gerechnet()
		}
		return g
	}

	// 1. Dieselbe Vorlage an der anderen Stelle: die Physik weist ab.
	w.annehmen(w.anmeldenTx(w.stelle[andere], andere, w.probe[andere], w.vorlage(andere, w.keys[w.anna])), z+3)
	g := offenlegen(andere, z+4)
	if allwissend {
		if g.Urteil != "echt" {
			t.Fatalf("Gegenprobe: wer jedes Bit kennt, besteht ueberall -- %+v", g)
		}
		return
	}
	if g.Urteil != "abgewiesen" {
		t.Fatalf("andere Stelle: %+v (die Physik haette abweisen muessen)", g)
	}
	tok := w.token(w.cs)
	if tok == nil || tok.Fehl != 1 || tok.Eingeloest {
		t.Fatalf("nach dem Versuch an der anderen Stelle: %+v", tok)
	}

	// 2. An der gewaehlten Stelle: echt, die Deckung geht an die Stelle.
	vorher := w.stand(w.cs, w.stelle[ehrlich])
	w.annehmen(w.anmeldenTx(w.stelle[ehrlich], ehrlich, w.probe[ehrlich], w.vorlage(ehrlich, w.keys[w.anna])), z+5)
	if g := offenlegen(ehrlich, z+6); g.Urteil != "echt" || g.WertAn != w.stelle[ehrlich] {
		t.Fatalf("gewaehlte Stelle: %+v", g)
	}
	if w.stand(w.cs, w.stelle[ehrlich]) <= vorher || w.token(w.cs) != nil {
		t.Fatal("Deckung nicht bei der Stelle oder Token nicht verbraucht")
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
		t.Fatal("Knoten weichen ab")
	}
	t.Logf("Probelauf: %d von %d Pulsen gemessen, Token mit %d Stellen aus %d Sendern; andere Stelle abgewiesen, gewaehlte echt",
		len(lam), s3AN, len(w.teil), s3ASend)
}
