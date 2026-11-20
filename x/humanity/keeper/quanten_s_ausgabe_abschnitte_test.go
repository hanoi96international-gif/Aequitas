package keeper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// Das Werkzeug im Abschnitts-Modus (quanten_s_ausgabe_abschnitte.go) gegen die
// echten Regeln und den echten Validator-Dienst (in-process, durch JSON):
// jeder Validator praepariert seinen Abschnitt selbst, das Werkzeug sieht nur
// Basen, Blaetter und Chiffrate.

// qsaNetz: qsNetz plus der Weg zu jedem Validator, mit Faelschungen.
type qsaNetz struct {
	*qsNetz
	fremderSchluessel  int // Validator nennt den Schluessel eines anderen
	antwortFremd       int // Antwort mit fremder Unterschrift
	oeffnungVertauscht int // Oeffnung fuer die andere Stelle
	anfragen           int // Anfragen an die Validatoren
	abholungen         int // Abschnitte fuer Inhaberin oder Stelle (fuerInhaberin)
	blattFalsch        int // dieser Validator nennt ein falsches Blatt
}

func (n *qsaNetz) schluessel(_ context.Context, x int) (string, error) {
	n.anfragen++
	if x == n.fremderSchluessel {
		return pubHex(n.q.w.valKeys[x%len(n.q.w.valKeys)]), nil
	}
	return pubHex(n.q.w.valKeys[x-1]), nil
}

func (n *qsaNetz) abschnitt(_ context.Context, x int, req QuantenSAbschnittAnfrage) (QuantenSAbschnittAntwort, error) {
	roh, _ := json.Marshal(req)
	var r QuantenSAbschnittAnfrage
	if err := json.Unmarshal(roh, &r); err != nil {
		return QuantenSAbschnittAntwort{}, err
	}
	ans, err := n.q.d[x-1].abschnittPraeparieren(r)
	if err != nil {
		return ans, err
	}
	if x == n.antwortFremd {
		k, _ := neuerSchluessel(n.q.w.t)
		ans.Sig = personalSign(n.q.w.t, k, quantenSAbschnittAntwortNachricht(ans))
	}
	roh, _ = json.Marshal(ans)
	var a QuantenSAbschnittAntwort
	return a, json.Unmarshal(roh, &a)
}

func (n *qsaNetz) fuerInhaberin(_ context.Context, x int, K string) (QuantenSPaketAbschnitt, error) {
	n.abholungen++
	a, err := n.q.d[x-1].abschnittFuerInhaberin(K)
	if err != nil {
		return a, err
	}
	if x == n.blattFalsch && len(a.Blaetter) > 0 {
		a.Blaetter[0] = strings.Repeat("ab", 32) // ein Validator luegt ueber ein Blatt (Unterschrift passt dann nicht)
	}
	roh, _ := json.Marshal(a)
	var out QuantenSPaketAbschnitt
	return out, json.Unmarshal(roh, &out)
}

func (n *qsaNetz) oeffnung(_ context.Context, x int, K string, b int) (string, error) {
	if x == n.oeffnungVertauscht {
		b = 1 - b
	}
	return n.q.d[x-1].oeffnungFuerStelle(K, b)
}

func qsaWerkzeug(t *testing.T, n int) (*qwDienste, QuantenSAusgabeAuftrag, QuantenSBasen, []*qsaMessung) {
	t.Helper()
	q, a := qsWerkzeugWelt(t, n)
	var mess []*qsaMessung
	for x, d := range q.d {
		m := &qsaMessung{r: rand.New(rand.NewSource(int64(500 + x)))}
		d.messquelle = m
		mess = append(mess, m)
	}
	r := rand.New(rand.NewSource(int64(n) * 3))
	bs := QuantenSBasen{N: n, Wo: "test"}
	v := len(q.w.val)
	for x := 1; x <= v; x++ {
		von, bis, _ := quantenSAbschnitt(n, v, x)
		y := make([]int, bis-von)
		for i := range y {
			y[i] = r.Intn(2)
		}
		bs.Basen = append(bs.Basen, qsBitsText(y))
	}
	return q, a, bs, mess
}

func TestQuantenSAusgabeAbschnitte_Gutfall(t *testing.T) {
	for _, bz := range [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}} {
		t.Run(fmt.Sprintf("b%d_z%d", bz[0], bz[1]), func(t *testing.T) {
			q, a, bs, mess := qsaWerkzeug(t, 256)
			a.bz = &bz
			w := q.w
			vorher := w.stand(w.cs, w.val[0])
			var log bytes.Buffer
			erg, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, w.val, 3, &qsaNetz{qsNetz: &qsNetz{q: q}}, &log)
			if err != nil {
				t.Fatalf("%v\n%s", err, log.String())
			}
			if erg.Doppelvorlage != "abgewiesen" || erg.Vorlage != "eingeloest" || erg.B != bz[0] || erg.Stellen[erg.B] != w.val[0] {
				t.Fatalf("Ergebnis: %+v", erg)
			}
			if got := w.stand(w.cs, w.val[0]); got != vorher {
				t.Fatalf("Ausgeber: vorher %d, nachher %d", vorher, got)
			}
			if w.stand(w.cs, erg.Stellen[1-erg.B]) != 0 {
				t.Fatal("die Stelle der Doppelvorlage hat etwas bekommen")
			}
			if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
				t.Fatal("Register weichen ab")
			}
			for x, m := range mess {
				if m.aufruf.Load() != 1 {
					t.Fatalf("Validator %d: %d Messungen", x+1, m.aufruf.Load())
				}
			}
			for _, muss := range []string{"in 5 Abschnitten", "aus den Stichproben", "Doppelvorlage: abgewiesen", "ehrliche Vorlage: eingeloest", "Stellen/min"} {
				if !strings.Contains(log.String(), muss) {
					t.Fatalf("Protokoll ohne %q:\n%s", muss, log.String())
				}
			}
		})
	}
}

// Ein Validator faelscht: das Werkzeug bricht ab, bevor etwas auf die Kette geht.
func TestQuantenSAusgabeAbschnitte_Faelschung(t *testing.T) {
	for name, netz := range map[string]func(*qsNetz) *qsaNetz{
		"fremder Schluessel":  func(n *qsNetz) *qsaNetz { return &qsaNetz{qsNetz: n, fremderSchluessel: 3} },
		"fremde Antwort":      func(n *qsNetz) *qsaNetz { return &qsaNetz{qsNetz: n, antwortFremd: 2} },
		"Oeffnung vertauscht": func(n *qsNetz) *qsaNetz { return &qsaNetz{qsNetz: n, oeffnungVertauscht: 4} },
	} {
		t.Run(name, func(t *testing.T) {
			q, a, bs, _ := qsaWerkzeug(t, 256)
			if _, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz(&qsNetz{q: q}), &bytes.Buffer{}); err == nil {
				t.Fatal("Faelschung nicht bemerkt")
			}
			if len(q.w.cs.quantenWJetzt().Token) != 0 {
				t.Fatal("trotz Faelschung ausgegeben")
			}
		})
	}
}

// Basen, die nicht zu den Abschnitten passen, oder zu wenige Stellen: Abbruch
// vor jeder Messung.
func TestQuantenSAusgabeAbschnitte_BasenStreng(t *testing.T) {
	for name, f := range map[string]func(b *QuantenSBasen){
		"Abschnitt zu kurz":    func(b *QuantenSBasen) { b.Basen[1] = b.Basen[1][1:] },
		"kein Bit":             func(b *QuantenSBasen) { b.Basen[0] = "x" + b.Basen[0][1:] },
		"zu wenige Abschnitte": func(b *QuantenSBasen) { b.Basen = b.Basen[:4] },
		"zu wenige Stellen":    func(b *QuantenSBasen) { b.N, b.Basen = 20, []string{"0101", "0101", "0101", "0101", "0101"} },
	} {
		t.Run(name, func(t *testing.T) {
			q, a, bs, mess := qsaWerkzeug(t, 256)
			f(&bs)
			netz := &qsaNetz{qsNetz: &qsNetz{q: q}}
			if _, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &bytes.Buffer{}); err == nil {
				t.Fatal("angenommen")
			}
			if netz.anfragen != 0 {
				t.Fatal("das Werkzeug fragt die Validatoren, obwohl die Basen nicht passen")
			}
			for _, m := range mess {
				if m.aufruf.Load() != 0 {
					t.Fatal("trotzdem gemessen")
				}
			}
		})
	}
}
