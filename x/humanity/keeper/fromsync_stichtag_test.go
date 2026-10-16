package keeper

import (
	"math"
	"strings"
	"testing"
)

// Siehe fromsync_stichtag.go.

func fromSyncStichtagSetzen(t *testing.T, ab int64) {
	t.Helper()
	fromSyncStrengOverride.Store(ab)
	t.Cleanup(func() { fromSyncStrengOverride.Store(0) })
}

// Schlafend (Konstante MaxInt64, kein Override) ist jede Entscheidung genau die
// von vorher: fromSyncAusnahme == FromSync, selbstGeholtOhneSperre ==
// FromSync || SelfFetched -- fuer jede Blockzeit, auch die Raender. Dazu die
// drei Pruefstellen selbst mit einem FromSync-Block, der vorher durchkam.
func TestFromSyncStichtag_SchlafendByteGleich(t *testing.T) {
	// Schlafend ist die Konstante (bis ein PR den Stichtag setzt); auch danach
	// prueft der Override MaxInt64 dasselbe Verhalten.
	if fromSyncStrengAbUnix == math.MaxInt64 && fromSyncStrengAb() != math.MaxInt64 {
		t.Fatal("Override aus einem anderen Test nicht zurueckgesetzt")
	}
	fromSyncStichtagSetzen(t, math.MaxInt64)
	for _, zeit := range []int64{0, 1, zeitstempelPruefungAbUnix, nowUnix(), math.MaxInt64 - 200, math.MaxInt64 - 1, math.MaxInt64} {
		if fromSyncStreng(zeit) {
			t.Fatalf("schlafend streng bei Blockzeit %d", zeit)
		}
		for _, fs := range []bool{false, true} {
			for _, sf := range []bool{false, true} {
				b := &Block{Timestamp: zeit, FromSync: fs, SelfFetched: sf}
				if got := fromSyncAusnahme(b); got != fs {
					t.Fatalf("Zeit %d FromSync=%v SelfFetched=%v: fromSyncAusnahme=%v", zeit, fs, sf, got)
				}
				if got := selbstGeholtOhneSperre(b); got != (fs || sf) {
					t.Fatalf("Zeit %d FromSync=%v SelfFetched=%v: selbstGeholtOhneSperre=%v", zeit, fs, sf, got)
				}
			}
		}
	}

	// Finalitaet: ein FromSync-Block weit unter dem Checkpoint ist wie bisher
	// keine Verletzung, derselbe ohne FromSync schon.
	dag := fromSyncFinalitaetsDAG(1000)
	unten := &Block{Height: 10, Timestamp: nowUnix(), FromSync: true}
	if dag.isFinalityViolation(unten) {
		t.Fatal("schlafend: FromSync-Block unter der Finalitaet abgewiesen -- das war vorher nicht so")
	}
	unten.FromSync = false
	if !dag.isFinalityViolation(unten) {
		t.Fatal("ohne FromSync muss die Finalitaet greifen")
	}

	// Spaet eingehaengt: FromSync wie bisher ausgenommen.
	register := validatorRegisterOverride.Load()
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(register) })
	spaet := &Block{Timestamp: nowUnix() - 7200, FromSync: true, Transactions: []Transaction{{Type: "validator_bindung"}}}
	if g := spaetEingehaengt(spaet, func() int64 { return nowUnix() }); g != "" {
		t.Fatalf("schlafend: FromSync-Block als spaet abgewiesen: %s", g)
	}
	spaet.FromSync = false
	if spaetEingehaengt(spaet, func() int64 { return nowUnix() }) == "" {
		t.Fatal("ohne FromSync muss die Spaet-Pruefung greifen")
	}
}

// Die Grenze haengt an der Blockzeit, mit der Rueck-Toleranz.
func TestFromSyncStichtag_Grenze(t *testing.T) {
	ab := nowUnix() - 3600
	fromSyncStichtagSetzen(t, ab)
	for _, c := range []struct {
		zeit   int64
		streng bool
	}{
		{ab - zeitstempelRueckToleranz - 1, false},
		{ab - zeitstempelRueckToleranz, true}, // zurueckdatiert um die Toleranz: entgeht nicht
		{ab, true},
		{math.MaxInt64, true}, // kein Ueberlauf
		{0, false},
	} {
		if got := fromSyncStreng(c.zeit); got != c.streng {
			t.Fatalf("fromSyncStreng(%d) = %v bei Stichtag %d, erwartet %v", c.zeit, got, ab, c.streng)
		}
	}
	alt := &Block{Timestamp: ab - 86400, FromSync: true, SelfFetched: true}
	if !fromSyncAusnahme(alt) || !selbstGeholtOhneSperre(alt) {
		t.Fatal("ein Block vor dem Stichtag muss unveraendert nachspielen")
	}
	neu := &Block{Timestamp: ab, FromSync: true, SelfFetched: true}
	if fromSyncAusnahme(neu) || selbstGeholtOhneSperre(neu) {
		t.Fatal("ab dem Stichtag darf FromSync/SelfFetched nichts mehr entbinden")
	}
}

func fromSyncFinalitaetsDAG(finalisiert int64) *BlockDAG {
	dag := newOrphanTestDAG()
	dag.state = &ChainState{}
	dag.state.finalizedHeightCache = finalisiert
	dag.state.finalizedHashCache = "f"
	dag.state.finalizedCacheLoaded = true
	return dag
}

// fromSyncAnnahmeDAG wie in TestAddPeerBlock_FromSyncNurFuerAlteGeschichte:
// ein Genesis-Elternteil, der Erzeuger ist zugelassen.
func fromSyncAnnahmeDAG(t *testing.T, proposer string, state *ChainState) *BlockDAG {
	t.Helper()
	dag := newOrphanTestDAG()
	dag.state = state
	dag.bootHeight = 0
	dag.authorizedValidators = map[string]bool{strings.ToLower(proposer): true}
	dag.warnedUnknownProposers = map[string]bool{}
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	dag.replayedBlocks = map[string]bool{}
	dag.equivocationIndex = map[string]string{}
	dag.blocks["deadbeef"] = &Block{Hash: "deadbeef", Height: 0, IsGenesis: true}
	return dag
}

// MISSBRAUCH: ein per Sync vom Seed gelieferter Block, der nur dank FromSync
// unter dem Finalitaets-Checkpoint durchkaeme (etwa ueber eine Seed-Verbindung
// ohne TLS eingeschleust), wird ab dem Stichtag abgewiesen. Ein Block von vor
// dem Stichtag kommt wie bisher durch.
func TestFromSyncStichtag_FinalitaetGiltAuchBeimSync(t *testing.T) {
	ab := nowUnix() - 3600
	fromSyncStichtagSetzen(t, ab)

	neu := signTestBlockWithParent(t, 1, "deadbeef")
	neu.FromSync = true
	state := &ChainState{finalizedHeightCache: 1000, finalizedHashCache: "f", finalizedCacheLoaded: true}
	dag := fromSyncAnnahmeDAG(t, neu.Proposer, state)
	if !dag.isFinalityViolation(neu) {
		t.Fatal("ab dem Stichtag: FromSync-Block unter dem Checkpoint ist keine Verletzung")
	}
	if dag.AddPeerBlock(neu) {
		t.Fatal("ab dem Stichtag: FromSync-Block unter dem Checkpoint angenommen")
	}
	if dag.hasBlockInMemory(neu.Hash) {
		t.Fatal("abgewiesener Block steht im DAG")
	}

	alt := signTestBlockWithZeit(t, 1, "deadbeef", ab-86400)
	alt.FromSync = true
	dagAlt := fromSyncAnnahmeDAG(t, alt.Proposer, &ChainState{finalizedHeightCache: 1000, finalizedHashCache: "f", finalizedCacheLoaded: true})
	if dagAlt.isFinalityViolation(alt) {
		t.Fatal("ein Block vor dem Stichtag muss vom Seed weiter nachladbar sein")
	}
	if !dagAlt.AddPeerBlock(alt) {
		t.Fatal("ein Block vor dem Stichtag wurde abgewiesen")
	}
}

// MISSBRAUCH: ein gesperrter Validator (Doppelsignatur) bekommt seinen Block
// ab dem Stichtag weder ueber FromSync noch ueber SelfFetched hinein. Mit
// echter Datenbank, damit die Gegenproben (ungesperrt angenommen, schlafend
// angenommen) zeigen, dass die Sperre der Grund der Abweisung ist.
func TestFromSyncStichtag_SperreGiltAuchBeimSync_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-fromsync-sperre.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	ab := nowUnix() - 3600

	sperren := func(addr string) {
		cs.penaltyMu.Lock()
		cs.penaltyCache = map[string]validatorPenalty{strings.ToLower(addr): {banned: true, lastOffenseAt: ab - 600}}
		cs.penaltyCacheLoaded = true
		cs.penaltyMu.Unlock()
	}
	versuch := func(fromSync, selfFetched, gesperrt bool) bool {
		b := signTestBlockWithParent(t, 1, "deadbeef")
		b.FromSync, b.SelfFetched = fromSync, selfFetched
		if gesperrt {
			sperren(b.Proposer)
			if g, _ := cs.IsValidatorSuspended(b.Proposer, b.Timestamp); !g {
				t.Fatal("Vorbedingung: der Erzeuger ist gesperrt")
			}
		} else {
			sperren("0x00000000000000000000000000000000000000ff")
		}
		dag := fromSyncAnnahmeDAG(t, b.Proposer, cs)
		ok := dag.AddPeerBlock(b)
		if ok != dag.hasBlockInMemory(b.Hash) {
			t.Fatalf("AddPeerBlock=%v, im DAG=%v", ok, dag.hasBlockInMemory(b.Hash))
		}
		return ok
	}

	// Schlafend: wie bisher entbinden FromSync und SelfFetched von der Sperre.
	fromSyncStrengOverride.Store(math.MaxInt64)
	t.Cleanup(func() { fromSyncStrengOverride.Store(0) })
	if !versuch(true, false, true) || !versuch(false, true, true) {
		t.Fatal("schlafend: FromSync/SelfFetched-Block eines Gesperrten abgewiesen -- das war vorher nicht so")
	}
	if versuch(false, false, true) {
		t.Fatal("ohne FromSync/SelfFetched muss die Sperre greifen")
	}

	// Ab dem Stichtag: gesperrt abgewiesen, auf jedem Weg; ungesperrt angenommen.
	fromSyncStrengOverride.Store(ab)
	for _, w := range []struct {
		name   string
		fs, sf bool
	}{{"FromSync", true, false}, {"SelfFetched", false, true}, {"beide", true, true}} {
		if versuch(w.fs, w.sf, true) {
			t.Fatalf("%s: Block eines gesperrten Validators ab dem Stichtag angenommen", w.name)
		}
		if !versuch(w.fs, w.sf, false) {
			t.Fatalf("%s: Gegenprobe -- ungesperrter Erzeuger abgewiesen", w.name)
		}
	}
}

// MISSBRAUCH: eine spaet eingehaengte Bindung kommt ab dem Stichtag auch ueber
// FromSync nicht mehr durch.
func TestFromSyncStichtag_SpaetEingehaengtGiltAuchBeimSync(t *testing.T) {
	register := validatorRegisterOverride.Load()
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(register) })
	ab := nowUnix() - 3*3600
	fromSyncStichtagSetzen(t, ab)

	spitze := func() int64 { return nowUnix() }
	neu := &Block{Timestamp: nowUnix() - 7200, FromSync: true, Transactions: []Transaction{{Type: "validator_bindung"}}}
	if spaetEingehaengt(neu, spitze) == "" {
		t.Fatal("ab dem Stichtag: spaet eingehaengte Bindung ueber FromSync angenommen")
	}
	alt := &Block{Timestamp: ab - 7200, FromSync: true, Transactions: []Transaction{{Type: "validator_bindung"}}}
	if g := spaetEingehaengt(alt, spitze); g != "" {
		t.Fatalf("ein Block vor dem Stichtag muss unveraendert nachspielen: %s", g)
	}
}

// Die Strafe bei Doppelsignatur haengt an !fromSyncAusnahme -- der Quelltext
// muss genau diese Bedingung tragen (die Strafe selbst laeuft ueber Datenbank
// und Ausgang und ist in slashing_*_test.go abgedeckt).
func TestFromSyncStichtag_DoppelsignaturStraftAuchBeimSync(t *testing.T) {
	body := functionBodyFromSource(t, "block.go", "func (dag *BlockDAG) AddPeerBlock(")
	if !strings.Contains(body, "isEquivocation && dag.state != nil && !fromSyncAusnahme(block) &&") {
		t.Fatal("die Strafe bei Doppelsignatur haengt nicht an fromSyncAusnahme")
	}
	if strings.Contains(body, "isEquivocation && dag.state != nil && !block.FromSync") {
		t.Fatal("alte FromSync-Ausnahme bei der Strafe noch vorhanden")
	}
	if !strings.Contains(body, "!selbstGeholtOhneSperre(block)") {
		t.Fatal("die Sperrpruefung haengt nicht an selbstGeholtOhneSperre")
	}
	fromSyncStichtagSetzen(t, nowUnix()-3600)
	if fromSyncAusnahme(&Block{Timestamp: nowUnix(), FromSync: true}) {
		t.Fatal("ab dem Stichtag entbindet FromSync die Strafe")
	}
}
