package keeper

import (
	"context"
	"testing"
	"time"
)

// DAS KONTROLLIERTE EXPERIMENT (docs/LAUNCH_CHECKLISTE.md, Stand 15.09. 17:30)
//
// Die Spurensuche im Lasttest-Staub hatte alles ausgeschlossen, was ein
// verlorener Block waere: beide Boxen haben dieselben Bloecke, jede
// angenommene Ueberweisung steckt in genau einem Block, jeder Block wird genau
// einmal nachgespielt, keine Rollbacks, keine uebersprungenen Ueberweisungen.
// Und trotzdem weichen einzelne Konten ab. Der Befund war deshalb: es ist ein
// Unterschied im RECHENWEG zwischen Annahme (Produzent) und Nachspielen
// (Partner) -- und der naechste Schritt ein Experiment, das ein Kontenpaar
// ueber beide Wege schickt und jeden Zwischenstand vergleicht.
//
// Das ist dieses Experiment, nur ohne Boxen: derselbe Prozess spielt beide
// Knoten. Knoten A nimmt die Ueberweisung an (der Weg, den die annehmende Box
// geht), Knoten B spielt sie als Block nach (der Weg, den die andere Box
// geht). Verglichen wird danach JEDES Feld, das ueber Geld entscheidet -- nicht
// nur der Kontostand, sondern auch die Demurrage-Uhr, die spaeter einen
// Kontostand erzeugt.
//
// # WAS ES FINDET
//
// Die Demurrage-Uhr des EMPFAENGERS. Die Regel steht zweimal im
// Produktionscode, in genau diesen Worten:
//
//	"Receiving is not the holder acting, so it does not reset the clock --
//	 only starts it if this is the first money the account has held."
//
// Drei Pfade halten sich daran (transferMutateLocked, transferWithV7FeeLocked,
// der serielle Nachspielpfad -- alle drei startClockIfUnset). Fuenf nicht:
//
//	transfer_batch_concurrent.go   der Buendler        touchActivity(toAcc)
//	transfer_concurrent.go         der Shard-Schnellpfad
//	transfer_wal.go (live)         der WAL-Schnellpfad
//	transfer_wal.go (recovery)     das Wiederherstellen aus dem WAL
//	replay_parallel.go             der parallele NACHSPIELPFAD
//
// Die Trennlinie verlaeuft also nicht zwischen Annahme und Nachspielen,
// sondern quer durch beide. Ob die Uhr eines Empfaengers zurueckgesetzt wird,
// haengt davon ab, welcher Schnellpfad auf der annehmenden Box gerade
// zustaendig war -- und auf der nachspielenden davon, ob die Ueberweisung
// zufaellig in einem buendelbaren Lauf lag. Zwei Knoten, dieselbe
// Ueberweisung, zwei verschiedene Uhren.
//
// # WARUM DAS GELD IST UND NICHT KOSMETIK
//
// LastActivityAt steht ABSICHTLICH nicht im accountLeaf (state.go), geht also
// nicht in den StateRoot ein. Die Abweichung ist damit unsichtbar -- der
// Divergenz-Waechter kann sie nicht melden, und sie bleibt stehen, bis sie
// sich in einen Kontostand verwandelt: settleDemurrageLocked rechnet den
// Verfall aus genau diesem Feld, und checkAndMoveToEscrowLocked kehrt ein
// Konto nach 2,5 Jahren Ruhe ins Escrow. Dann weichen zwei Knoten im
// Kontostand ab, ohne dass irgendein Block, irgendeine Ueberweisung oder
// irgendein Rollback dafuer verantwortlich waere.
//
// Und es ist zusaetzlich ein Leck: wer die Uhr durch Empfangen zuruecksetzen
// kann, haelt jedes beliebig grosse Vermoegen dauerhaft verfallsfrei, indem
// ihm jemand alle drei Monate ein Mikro-AEQ schickt. Genau das schliesst die
// Regel aus, an die sich fuenf von acht Pfaden nicht hielten.

// zustandsAbzug ist, was dieses Experiment je Schritt auf beiden Seiten
// mitschreibt: alles, was ueber den spaeteren Kontostand entscheidet.
type zustandsAbzug struct {
	guthabenMikro int64
	uhr           int64
}

func abzugVon(cs *ChainState, addr string) zustandsAbzug {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	acc, ok := cs.accounts.Get(addr)
	if !ok {
		return zustandsAbzug{}
	}
	return zustandsAbzug{guthabenMikro: acc.Balance.Micro(), uhr: acc.LastActivityAt}
}

// uhrSetzen gibt einem Konto eine Vorgeschichte: Guthaben und eine
// Demurrage-Uhr, die schon laeuft. Ein frisch angelegtes Testkonto hat
// LastActivityAt=0, und bei null sind sich ALLE acht Pfade einig (sie setzen
// die Uhr dann erstmalig). Genau deshalb ist der Unterschied bisher in einer
// gruenen Testsuite unsichtbar geblieben.
func uhrSetzen(cs *ChainState, addr string, guthaben float64, uhr int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.accounts.Set(addr, &AccountState{Address: addr, Balance: NewDecimal(guthaben), LastActivityAt: uhr})
}

// TestAnnahmeGegenNachspielen_EmpfaengerUhr ist das Experiment selbst: ein
// Kontenpaar, eine Ueberweisung, beide Wege, jedes Feld verglichen.
func TestAnnahmeGegenNachspielen_EmpfaengerUhr(t *testing.T) {
	const (
		tag    = int64(24 * 60 * 60)
		t0     = int64(1_700_000_000) // der Augenblick der Ueberweisung
		sender = "0xacct-sender"
		empf   = "0xacct-empfaenger"
	)
	// Der Empfaenger ist ein BESTEHENDES Konto: 900 AEQ (unter dem fair share
	// von 1.000, also kein Verfall faellig -- das ist die Bedingung, unter der
	// jeder Schnellpfad ueberhaupt zustaendig ist) und eine Uhr, die seit 300
	// Tagen laeuft.
	uhrVorher := t0 - 300*tag

	orig := setzeZeitQuelleFuerTest(func() time.Time { return time.Unix(t0, 0) })
	t.Cleanup(func() { setzeZeitQuelleFuerTest(orig) })

	// ---- Knoten A: ANNAHME ----
	csA := newTestState()
	uhrSetzen(csA, sender, 5000, t0)
	uhrSetzen(csA, empf, 900, uhrVorher)
	fromLost, toLost, err := csA.Transfer(sender, empf, 600)
	if err != nil {
		t.Fatalf("Annahme scheiterte: %v", err)
	}
	annahme := abzugVon(csA, empf)

	// ---- Knoten B: NACHSPIELEN ----
	// Dieselbe Ueberweisung, wie sie im Block steht -- mit exakt den
	// Demurrage-Zahlen, die die annehmende Box ermittelt hat.
	tx := Transaction{
		Type: "transfer", Wallet: sender, To: empf, Amount: 600,
		FromDemurrageLost: fromLost, ToDemurrageLost: toLost,
	}
	dagB, csB := newDeterminismTestDAG()
	uhrSetzen(csB, sender, 5000, t0)
	uhrSetzen(csB, empf, 900, uhrVorher)
	block := &Block{Height: 1, Hash: "0xexperiment", Timestamp: t0, Transactions: []Transaction{tx}}
	if ok := dagB.replayTransactions(block, true); !ok {
		t.Fatal("Nachspielen wies einen wohlgeformten Block ab")
	}
	nachspielen := abzugVon(csB, empf)

	if annahme.guthabenMikro != nachspielen.guthabenMikro {
		t.Fatalf("Guthaben des Empfaengers weicht ab: Annahme %d Mikro, Nachspielen %d Mikro",
			annahme.guthabenMikro, nachspielen.guthabenMikro)
	}
	if annahme.uhr != nachspielen.uhr {
		t.Fatalf("Demurrage-Uhr des Empfaengers weicht ab: Annahme %d, Nachspielen %d (Differenz %d Tage)\n"+
			"Dieselbe Ueberweisung, zwei Knoten, zwei Uhren. Das Feld steht nicht im StateRoot,\n"+
			"also meldet es kein Waechter -- es wird erst sichtbar, wenn settleDemurrageLocked\n"+
			"daraus zwei verschiedene Kontostaende rechnet. Die Regel, an die sich\n"+
			"transferMutateLocked, transferWithV7FeeLocked und der serielle Nachspielpfad\n"+
			"halten: Empfangen startet die Uhr, es setzt sie nie zurueck.",
			annahme.uhr, nachspielen.uhr, (nachspielen.uhr-annahme.uhr)/tag)
	}
}

// TestAnnahmeGegenNachspielen_UhrWirdZuGeld zeigt, was die abweichende Uhr
// spaeter kostet: derselbe Kontostand, hundert Tage weiter, auf beiden Knoten
// verschieden. Das ist die Bruecke von "unsichtbares Feld" zu "die Boxen sind
// sich ueber einen Kontostand uneins".
func TestAnnahmeGegenNachspielen_UhrWirdZuGeld(t *testing.T) {
	const tag = int64(24 * 60 * 60)
	const t0 = int64(1_700_000_000)

	orig := setzeZeitQuelleFuerTest(func() time.Time { return time.Unix(t0, 0) })
	t.Cleanup(func() { setzeZeitQuelleFuerTest(orig) })

	// Zwei Konten mit identischem Guthaben, aber den zwei Uhren, die die
	// beiden Wege oben hinterlassen: die eine lief schon 300 Tage weiter, die
	// andere wurde beim Empfangen zurueckgesetzt.
	cs := newTestState()
	uhrSetzen(cs, "0xacct-alt", 1500, t0-300*tag) // Uhr nicht zurueckgesetzt (Annahme, langsamer Pfad)
	uhrSetzen(cs, "0xacct-neu", 1500, t0)         // Uhr zurueckgesetzt (Buendler / paralleles Nachspielen)

	setzeZeitQuelleFuerTest(func() time.Time { return time.Unix(t0+100*tag, 0) })
	altAcc, _ := cs.accounts.Get("0xacct-alt")
	neuAcc, _ := cs.accounts.Get("0xacct-neu")
	alt := effectiveBalance(altAcc)
	neu := effectiveBalance(neuAcc)

	if alt == neu {
		t.Fatalf("Aufbau taugt nicht: beide Uhren ergeben denselben Kontostand (%v) -- "+
			"dann misst dieses Experiment nichts", alt)
	}
	// Belegt und benannt, damit die Groessenordnung in der Ausgabe steht.
	t.Logf("derselbe Kontostand, hundert Tage weiter: Uhr gelaufen %.6f AEQ, Uhr zurueckgesetzt %.6f AEQ (Differenz %.6f)",
		alt.Float(), neu.Float(), neu.Float()-alt.Float())
}

// TestNachspielen_SeriellUndParallelGleicheEmpfaengerUhr haelt fest, dass die
// zwei Nachspiel-Implementierungen sich nicht unterscheiden duerfen.
//
// Beide werden hier DIREKT gerufen, nicht ueber die Blockschleife: seit
// parallelReplayMinBatch auf 1 steht, nimmt auch ein Block mit einer einzigen
// Ueberweisung den Buendelpfad, und "eine je Block" ist damit kein Weg mehr,
// den seriellen Pfad zu erzwingen. Der direkte Aufruf vergleicht ohnehin
// genau das, worum es geht: zwei Implementierungen derselben Wirkung.
func TestNachspielen_SeriellUndParallelGleicheEmpfaengerUhr(t *testing.T) {
	const tag = int64(24 * 60 * 60)
	const t0 = int64(1_700_000_000)
	const sender, empf = "0xacct-sender", "0xacct-empfaenger"

	orig := setzeZeitQuelleFuerTest(func() time.Time { return time.Unix(t0, 0) })
	t.Cleanup(func() { setzeZeitQuelleFuerTest(orig) })

	uhrVorher := t0 - 300*tag
	bauen := func() *ChainState {
		cs := newTestState()
		uhrSetzen(cs, sender, 5000, t0)
		uhrSetzen(cs, empf, 900, uhrVorher)
		return cs
	}

	csP := bauen()
	batch := []Transaction{{Type: "transfer", Wallet: sender, To: empf, Amount: 600}}
	csP.mu.Lock()
	angewandt, err := csP.applyTransferBatchParallel(context.Background(), batch, t0, nil)
	csP.mu.Unlock()
	if err != nil || angewandt != 1 {
		t.Fatalf("Buendelpfad wandte nichts an: angewandt=%d err=%v", angewandt, err)
	}

	csS := bauen()
	csS.mu.Lock()
	err = csS.applyTransferDeltaLockedSammelnd(context.Background(), sender, empf, 600, 0, 0, t0, nil, 0)
	csS.mu.Unlock()
	if err != nil {
		t.Fatalf("serieller Pfad scheiterte: %v", err)
	}

	p, s := abzugVon(csP, empf), abzugVon(csS, empf)
	if p != s {
		t.Fatalf("Empfaenger %s: parallel %+v, seriell %+v\n"+
			"Ob gebuendelt wird, entscheidet die Blockzusammensetzung -- nicht der Zustand.",
			empf, p, s)
	}
}
