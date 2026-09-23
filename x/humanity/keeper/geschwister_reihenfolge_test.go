package keeper

import (
	"fmt"
	"os"
	"testing"
)

// REICHT DIE WURZEL? -- ein Experiment statt eines Arguments.
//
// # DIE ANNAHME, DIE HIER GEPRUEFT WIRD
//
// Ueber die Staub-Divergenz hiess es bisher, auch von mir: sauber waere,
// wenn sich der Zustand AUSSCHLIESSLICH beim Anwenden eines Blocks aendert
// und die Annahme nur einreiht. Dann saehen alle Knoten dieselben Bloecke,
// rechneten dieselbe Folge und kaemen auf dasselbe Ergebnis.
//
// Der erste Teil stimmt. Der zweite ist eine ANNAHME, und sie haelt nur,
// wenn alle Knoten die Bloecke auch in DERSELBEN REIHENFOLGE anwenden.
//
// Genau das tun sie nicht. Bei zwei Produzenten entstehen Geschwister: C1
// und C2 bauen gleichzeitig je einen Block, keiner ist Vorfahr des anderen.
// C1 wendet seinen eigenen zuerst an und den von C2 danach; C2 umgekehrt.
// Beide haben am Ende dieselbe MENGE Bloecke -- in verschiedener Folge.
//
// Solange kein Konto leerlaeuft, ist das folgenlos. Laeuft eines leer, ist
// die Reihenfolge das, was entscheidet, welche Ueberweisung noch bezahlbar
// ist -- und der nachspielende Pfad ueberspringt die andere.
//
// # WAS DIESES EXPERIMENT MACHT
//
// Es baut die Welt NACH dem Wurzel-Umbau nach: beide Knoten nehmen gar
// nichts an, sie wenden nur Bloecke an. Kein TransferAtomic, keine
// Annahme-Mutation, nichts. Nur zwei Bloecke, zwei Reihenfolgen.
//
// Faellt der Test durch, kostet der Umbau des heissen Pfades viel und
// beseitigt diese Divergenz NICHT -- dann ist die eigentliche Bedingung
// nicht "wo wird mutiert", sondern "wer darf gleichzeitig annehmen", und
// die Sperre aus annahme_tor.go ist kein Notnagel, sondern die Antwort.
//
// Opt-in wie jeder _RealDB-Test hier, zusaetzlich AEQUITAS_DB_B.

func TestGeschwisterReihenfolge_ZweiBloeckeZweiReihenfolgen_RealDB(t *testing.T) {
	truncateDistTestTables(t) // auch das Opt-in-Tor
	dbB := os.Getenv("AEQUITAS_DB_B")
	if dbB == "" {
		t.Skip("braucht AEQUITAS_DB_B: eine zweite, entbehrliche Postgres-Datenbank")
	}

	const sender = "0xgeschwister-sender"
	// Der Sender hat 10. Block X will 8 davon, Block Y will 6. Zusammen 14 --
	// mehr als da ist. Welche durchgeht, entscheidet allein die Reihenfolge.
	const guthaben = 10.0

	bauen := func(cs *ChainState) {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		for _, a := range []string{sender, "0xempf-x", "0xempf-y"} {
			cs.accounts.Delete(a)
			bal := 0.0
			if a == sender {
				bal = guthaben
			}
			acc := &AccountState{Address: a, Balance: NewDecimal(bal), LastActivityAt: nowUnix()}
			if err := cs.saveAccountToDB(acc); err != nil {
				t.Fatalf("Konto %s anlegen: %v", a, err)
			}
		}
	}

	csA := testKnoten(t, "unused-geschwister-a.json")
	if !csA.useDB {
		t.Fatal("Knoten A hat keine Datenbank -- DATABASE_URL pruefen")
	}
	t.Setenv("DATABASE_URL", dbB)
	schemaAnlegenUndLeeren(t, dbB)
	csB := testKnoten(t, "unused-geschwister-b.json")
	if !csB.useDB {
		t.Fatal("Knoten B hat keine Datenbank -- AEQUITAS_DB_B pruefen")
	}
	bauen(csA)
	bauen(csB)

	blockX := &Block{
		Height: 1, Hash: "0xgeschwister-x", Timestamp: nowUnix(),
		Transactions: []Transaction{{Type: "transfer", Wallet: sender, To: "0xempf-x", Amount: 8}},
	}
	blockY := &Block{
		Height: 1, Hash: "0xgeschwister-y", Timestamp: nowUnix(),
		Transactions: []Transaction{{Type: "transfer", Wallet: sender, To: "0xempf-y", Amount: 6}},
	}

	dagFuer := func(cs *ChainState) *BlockDAG {
		d := newOrphanTestDAG()
		d.state = cs
		d.bootHeight = 0
		d.replayedBlocks = make(map[string]bool)
		d.replayFailures = make(map[string]replayFailureState)
		d.stateRootMismatches = map[string]int{}
		d.stateRootMismatchLastAt = map[string]int64{}
		return d
	}
	dagA, dagB := dagFuer(csA), dagFuer(csB)

	vorher := uebersprungeneUeberweisungen.Load()

	// Knoten A: erst der eigene Block, dann der des Partners.
	dagA.replayTransactions(blockX, true)
	dagA.replayTransactions(blockY, true)
	// Knoten B: umgekehrt -- und das ist der ganze Unterschied.
	dagB.replayTransactions(blockY, true)
	dagB.replayTransactions(blockX, true)

	uebersprungen := uebersprungeneUeberweisungen.Load() - vorher

	stand := func(cs *ChainState, addr string) int64 { return abzugVon(cs, addr).guthabenMikro }

	t.Logf("Sender:  A %d Mikro, B %d Mikro", stand(csA, sender), stand(csB, sender))
	t.Logf("Empf X:  A %d Mikro, B %d Mikro", stand(csA, "0xempf-x"), stand(csB, "0xempf-x"))
	t.Logf("Empf Y:  A %d Mikro, B %d Mikro", stand(csA, "0xempf-y"), stand(csB, "0xempf-y"))
	t.Logf("uebersprungen insgesamt: %d", uebersprungen)

	var abweichend []string
	for _, a := range []string{sender, "0xempf-x", "0xempf-y"} {
		if stand(csA, a) != stand(csB, a) {
			abweichend = append(abweichend, fmt.Sprintf("%s (A %d / B %d)", a, stand(csA, a), stand(csB, a)))
		}
	}
	// DIESER TEST IST GRUEN, WENN DIE DIVERGENZ EINTRITT -- und das ist kein
	// Zynismus, sondern der Zweck.
	//
	// Er haelt eine EIGENSCHAFT der heutigen Bauform fest: ausgefuehrt wird in
	// ANKUNFTSREIHENFOLGE, nicht in kanonischer DAG-Reihenfolge. Zwei
	// Geschwisterbloecke kommen bei zwei Knoten in verschiedener Folge an, und
	// sobald ein Konto leerlaeuft, entscheidet genau das ueber das Ergebnis.
	//
	// Solange das so ist, waere ein Umbau, der nur die Annahme entkernt, teuer
	// und wirkungslos gegen diese Divergenz. Wird die Ausfuehrung eines Tages
	// auf kanonische Reihenfolge umgestellt, wird dieser Test ROT -- und das
	// ist dann die richtige Nachricht: die Eigenschaft hat sich geaendert, der
	// Test gehoert umgeschrieben, und die Sperre aus annahme_tor.go koennte
	// entbehrlich werden.
	if len(abweichend) == 0 {
		t.Fatalf("Die beiden Knoten stimmen ueberein -- die Eigenschaft, die dieser Test\n" +
			"festhaelt, gilt nicht mehr. Entweder wird jetzt in kanonischer DAG-Reihenfolge\n" +
			"ausgefuehrt statt in Ankunftsreihenfolge (dann ist das eine GUTE Nachricht und\n" +
			"der Test gehoert umgeschrieben), oder der Aufbau trifft die Bedingung nicht mehr.")
	}
	if uebersprungen != 2 {
		t.Fatalf("erwartet genau zwei uebersprungene Ueberweisungen (je eine auf jedem Knoten), waren %d", uebersprungen)
	}
	t.Logf("BELEGT: zwei Knoten, KEINE Annahme, dieselben zwei Bloecke -- nur in anderer\n"+
		"Reihenfolge -- und sie sind sich ueber jedes der drei Konten uneins: %v.\n"+
		"Ein Umbau, der allein die Annahme entkernt, beseitigt das nicht.", abweichend)
}
