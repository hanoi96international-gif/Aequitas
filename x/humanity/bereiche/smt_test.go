package bereiche

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"testing"
)

func schl(i int) Hash { return sha256.Sum256([]byte(fmt.Sprintf("konto-%d", i))) }

func TestBaum_BeweiseUndTeilbaum(t *testing.T) {
	b := NeuerBaum()
	if b.Wurzel() != (Hash{}) {
		t.Fatal("leerer Baum hat keine Nullwurzel")
	}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 300; i++ {
		b.Setze(schl(i), sha256.Sum256([]byte{byte(r.Intn(256)), byte(i)}))
	}
	wurzel := b.Wurzel()
	// Beweis fuer vorhandene und fuer fehlende Konten.
	for _, i := range []int{0, 17, 299, 1000, 5000} {
		k := schl(i)
		got, err := WurzelAus(k, b.Wert(k), b.Beweise(k))
		if err != nil || got != wurzel {
			t.Fatalf("Beweis fuer %d passt nicht", i)
		}
		if got, _ := WurzelAus(k, sha256.Sum256([]byte("falsch")), b.Beweise(k)); got == wurzel {
			t.Fatalf("falscher Wert fuer %d passt zur Wurzel", i)
		}
	}
	// Teilbaum aus drei Blaettern: Aenderungen ergeben dieselbe Wurzel wie im
	// vollen Baum.
	ks := []Hash{schl(3), schl(40), schl(7777)}
	var ws []Hash
	var bws []Beweis
	for _, k := range ks {
		ws = append(ws, b.Wert(k))
		bws = append(bws, b.Beweise(k))
	}
	tb, err := TeilbaumAus(wurzel, ks, ws, bws)
	if err != nil {
		t.Fatal(err)
	}
	neu := []Hash{sha256.Sum256([]byte("a")), {}, sha256.Sum256([]byte("c"))}
	for i, k := range ks {
		tb.Setze(k, neu[i])
		b.Setze(k, neu[i])
	}
	if tb.Wurzel() != b.Wurzel() {
		t.Fatal("Teilbaum und voller Baum kommen zu verschiedenen Wurzeln")
	}
	// Ein Beweis gegen eine fremde Wurzel wird abgewiesen.
	if _, err := TeilbaumAus(Hash{1}, ks[:1], ws[:1], bws[:1]); err == nil {
		t.Fatal("Beweis gegen fremde Wurzel angenommen")
	}
	// Loeschen fuehrt zurueck.
	c := NeuerBaum()
	c.Setze(schl(1), Hash{9})
	c.Setze(schl(1), Hash{})
	if c.Wurzel() != (Hash{}) {
		t.Fatal("Loeschen des einzigen Blatts ergibt keinen leeren Baum")
	}
}
