package zkgueltigkeit

import (
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/test"
)

func buendel(t *testing.T, tiefe int, auftraege []Auftrag) (*Zustand, *Kreis) {
	t.Helper()
	z, err := NeuerZustand(tiefe, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	k, err := z.Buendel(auftraege)
	if err != nil {
		t.Fatal(err)
	}
	return z, k
}

var auftraege = []Auftrag{{Von: 1, An: 2, Betrag: 300}, {Von: 2, An: 5, Betrag: 1_200}, {Von: 5, An: 1, Betrag: 50}, {Von: 7, An: 1, Betrag: 1_000}}

func TestKreis_GueltigesBuendelErfuellt(t *testing.T) {
	_, k := buendel(t, 4, auftraege)
	if err := test.IsSolved(NeuerKreis(4, len(auftraege)), k, ecc.BN254.ScalarField()); err != nil {
		t.Fatalf("gueltiges Buendel erfuellt den Kreis nicht: %v", err)
	}
}

// Jede Faelschung macht den Zeugen unerfuellbar -- also gibt es keinen Beweis.
func TestKreis_FaelschungenErfuellenNicht(t *testing.T) {
	faelle := map[string]func(k *Kreis){
		"anderer Betrag als signiert": func(k *Kreis) { k.Txs[0].Betrag = 301 },
		"falsche neue Wurzel":         func(k *Kreis) { k.NeuWurzel = big.NewInt(12345) },
		"fremde Signatur":             func(k *Kreis) { k.Txs[0].Sig = k.Txs[1].Sig },
		"Guthaben erfunden":           func(k *Kreis) { k.Txs[0].Von.Guthaben = big.NewInt(999_999) },
		"Nonce verbraucht":            func(k *Kreis) { k.Txs[0].Von.Nonce = big.NewInt(1) },
		"an sich selbst":              func(k *Kreis) { k.Txs[3].AnIndex = k.Txs[3].VonIndex },
	}
	for name, f := range faelle {
		_, k := buendel(t, 4, auftraege)
		f(k)
		if err := test.IsSolved(NeuerKreis(4, len(auftraege)), k, ecc.BN254.ScalarField()); err == nil {
			t.Errorf("%s: Faelschung erfuellt den Kreis", name)
		}
	}
	// Ueberziehung: der Zeuge rechnet mit negativem Rest -- im Koerper eine
	// riesige Zahl, die den 64-Bit-Bereich sprengt.
	_, k := buendel(t, 4, []Auftrag{{Von: 1, An: 2, Betrag: 1_000}})
	k.Txs[0].Betrag = 1_001
	if err := test.IsSolved(NeuerKreis(4, 1), k, ecc.BN254.ScalarField()); err == nil {
		t.Error("Ueberziehung erfuellt den Kreis")
	}
}

// Echter Beweis: Groth16 auf BN254 -- Setup, Beweis, Pruefung. Misst die
// Kosten (die offene Frage aus dem Konzept). Das Setup hier ist nur fuer den
// Test; im Betrieb braeuchte Groth16 eine Mehrparteien-Zeremonie (oder ein
// Verfahren ohne vertrauenswuerdiges Setup, siehe docs).
func TestKreis_BeweisUndPruefung(t *testing.T) {
	tiefe, n := 8, 4
	if os.Getenv("ZK_MESSUNG") != "" {
		tiefe, n = 16, 16
	}
	var viele []Auftrag
	for i := 0; i < n; i++ {
		viele = append(viele, Auftrag{Von: (i * 7) % (1 << tiefe), An: (i*7 + 3) % (1 << tiefe), Betrag: uint64(10 + i)})
	}
	_, k := buendel(t, tiefe, viele)

	t0 := time.Now()
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, NeuerKreis(tiefe, n))
	if err != nil {
		t.Fatal(err)
	}
	tCompile := time.Since(t0)
	t0 = time.Now()
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		t.Fatal(err)
	}
	tSetup := time.Since(t0)
	zeuge, err := frontend.NewWitness(k, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	t0 = time.Now()
	beweis, err := groth16.Prove(ccs, pk, zeuge)
	if err != nil {
		t.Fatal(err)
	}
	tBeweis := time.Since(t0)
	oeffentlich, _ := zeuge.Public()
	t0 = time.Now()
	if err := groth16.Verify(beweis, vk, oeffentlich); err != nil {
		t.Fatalf("Beweis nicht pruefbar: %v", err)
	}
	tPruef := time.Since(t0)
	t.Logf("Tiefe %d (%d Konten), %d Ueberweisungen: %d Bedingungen (%d je Ueberweisung); Compile %s, Setup %s, BEWEIS %s (%s je Ueberweisung), PRUEFUNG %s",
		tiefe, 1<<tiefe, n, ccs.GetNbConstraints(), ccs.GetNbConstraints()/n,
		tCompile.Round(time.Millisecond), tSetup.Round(time.Millisecond), tBeweis.Round(time.Millisecond),
		(tBeweis / time.Duration(n)).Round(time.Millisecond), tPruef.Round(time.Microsecond))

	// Ein Beweis fuer eine andere neue Wurzel wird abgelehnt.
	k2 := *k
	k2.NeuWurzel = big.NewInt(7)
	falsch, _ := frontend.NewWitness(&k2, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err := groth16.Verify(beweis, vk, falsch); err == nil {
		t.Fatal("Beweis passt zu einer fremden Wurzel")
	}
}
