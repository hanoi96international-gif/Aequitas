package keeper

import (
	"fmt"
	"math/rand"
	"testing"
)

// STUFE 2 AUF ZUSTANDSEBENE: DREI ANNEHMENDE, KEINE DIVERGENZ.
//
// zwei_produzenten_realdb_test.go zeigt den Fehler: nehmen zwei Knoten
// Belastungen DESSELBEN Kontos an, laufen die Kontenstaende auseinander,
// sobald Konten leerlaufen. Dieser Test zeigt, dass die Aufteilung aus
// zustaendigkeit.go ihn beseitigt, ohne dass nur einer annimmt:
//
//   - drei Knoten nehmen GLEICHZEITIG an, jeder nur fuer Absender, fuer die
//     er zustaendig ist; Empfaenger sind geteilt, Konten laufen leer;
//   - danach spielt jeder die Bloecke der beiden anderen nach, in
//     verschiedener Reihenfolge;
//   - alle Kontostaende und die StateRoot muessen auf allen dreien gleich
//     sein.
//
// Die Gegenprobe (jeder nimmt fuer beliebige Absender an) MUSS auseinander-
// laufen -- sonst prueft der Test nichts.

type annahmeKnoten struct {
	name  string
	dag   *BlockDAG
	cs    *ChainState
	korb  []Transaction
	hoehe int64
}

func dreiAnnahmeKnoten(t *testing.T, konten []string, start float64) []*annahmeKnoten {
	t.Helper()
	var out []*annahmeKnoten
	for _, n := range []string{"0x00000000000000000000000000000000000000a1", "0x00000000000000000000000000000000000000b2", "0x00000000000000000000000000000000000000c3"} {
		dag, cs := newDeterminismTestDAG()
		cs.mu.Lock()
		for _, k := range konten {
			cs.accounts.Set(k, &AccountState{Address: k, Balance: NewDecimal(start), LastActivityAt: nowUnix()})
		}
		cs.mu.Unlock()
		k := &annahmeKnoten{name: n, dag: dag, cs: cs}
		// Was die Annahme wirklich in den Block schreibt (mit Gebuehr,
		// BuchAt, Demurrage), nicht was der Test glaubt.
		cs.ausgangOhneDB = func(tx Transaction) { k.korb = append(k.korb, tx) }
		out = append(out, k)
	}
	return out
}

func verteilteAnnahmeLauf(t *testing.T, nurZustaendige bool, seed int64) (abweichend int, abgelehnt int) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	var konten []string
	for i := 0; i < 30; i++ {
		konten = append(konten, fmt.Sprintf("0xe%039d", i))
	}
	knoten := dreiAnnahmeKnoten(t, konten, 0.05)
	zuteilung := []string{knoten[0].name, knoten[1].name, knoten[2].name}
	const term = 7
	betraege := []float64{0.0071, 0.013, 0.0005, 0.021, 1.0 / 3.0 / 100}

	for runde := 0; runde < 12; runde++ {
		// Annahme: jeder Knoten fuer seine Absender (oder, in der Gegenprobe,
		// fuer beliebige), gleichzeitig -- keiner sieht die Annahmen der
		// anderen, bis die Bloecke kommen.
		for ki, k := range knoten {
			for n := 0; n < 30; n++ {
				von := konten[rng.Intn(len(konten))]
				an := konten[rng.Intn(len(konten))]
				if von == an {
					continue
				}
				if nurZustaendige && zustaendigFuer(von, term, zuteilung, nil, zuteilung[0]) != k.name {
					continue
				}
				if !nurZustaendige && rng.Intn(3) != ki {
					continue
				}
				betrag := betraege[rng.Intn(len(betraege))]
				tmpl := Transaction{Type: "transfer", Wallet: von, To: an, Amount: betrag,
					TxHash: fmt.Sprintf("0xva-%d-%d-%d-%d", seed, runde, ki, n)}
				if _, _, err := k.cs.TransferAtomic(von, an, betrag, tmpl); err != nil {
					abgelehnt++
				}
			}
		}
		// Bloecke bilden und bei den anderen nachspielen -- jeder in einer
		// anderen Reihenfolge.
		bloecke := make([]*Block, len(knoten))
		for ki, k := range knoten {
			k.hoehe++
			bloecke[ki] = &Block{Height: int64(runde*10 + ki + 1), Hash: fmt.Sprintf("0xva-blk-%s-%d", k.name, runde),
				Timestamp: nowUnix(), Transactions: k.korb}
			// Wie der Erzeuger beim Speichern (block.go): die Gebuehren
			// seines Blocks ins Grundeinkommen.
			k.cs.gebuehrenInsGrundeinkommen(gebuehrenSumme(k.korb))
			k.korb = nil
		}
		for ki, k := range knoten {
			reihenfolge := []int{(ki + 1) % 3, (ki + 2) % 3}
			if (runde+ki)%2 == 1 {
				reihenfolge[0], reihenfolge[1] = reihenfolge[1], reihenfolge[0]
			}
			for _, bi := range reihenfolge {
				if len(bloecke[bi].Transactions) == 0 {
					continue
				}
				if ok := k.dag.replayTransactions(bloecke[bi], true); !ok {
					t.Fatalf("Knoten %d lehnte Block von %d ab", ki, bi)
				}
			}
		}
	}
	for _, a := range append(append([]string(nil), konten...), ubiPoolAddr, validatorsPoolAddr, lpPoolAddr, treasuryPoolAddr) {
		s0 := stand(knoten[0].cs, a)
		for _, k := range knoten[1:] {
			if stand(k.cs, a) != s0 {
				abweichend++
				break
			}
		}
	}
	if abweichend == 0 {
		r0 := knoten[0].cs.StateRoot()
		for ki, k := range knoten[1:] {
			if r := k.cs.StateRoot(); r != r0 {
				t.Fatalf("Kontostaende gleich, StateRoot von Knoten %d weicht ab", ki+1)
			}
		}
	}
	return abweichend, abgelehnt
}

func TestVerteilteAnnahme_DreiAnnehmendeLaufenNichtAuseinander(t *testing.T) {
	for seed := int64(1); seed <= 2; seed++ {
		abw, abg := verteilteAnnahmeLauf(t, true, seed)
		if abg == 0 {
			t.Fatalf("seed %d: keine Ueberweisung abgelehnt -- die Konten liefen nie leer, der Test prueft nichts", seed)
		}
		if abw != 0 {
			t.Fatalf("seed %d: %d Konten weichen ab, obwohl jeder nur fuer seine Absender annahm", seed, abw)
		}
	}
}

// Gegenprobe: dieselbe Last, aber jeder Knoten nimmt fuer beliebige Absender
// an -- der Zustand vom 15.09.2026. Muss auseinanderlaufen.
func TestVerteilteAnnahme_GegenprobeOhneZustaendigkeitLaeuftAuseinander(t *testing.T) {
	gesamt := 0
	for seed := int64(1); seed <= 5 && gesamt == 0; seed++ {
		abw, _ := verteilteAnnahmeLauf(t, false, seed)
		gesamt += abw
	}
	if gesamt == 0 {
		t.Fatal("ohne Zustaendigkeit liefen die Knoten nicht auseinander -- die Last ist zu schwach, der Haupttest beweist nichts")
	}
}
