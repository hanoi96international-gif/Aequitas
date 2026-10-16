package keeper

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Siehe fromsync_stichtag.go.

func fromSyncStichtagSetzen(t *testing.T, ab int64) {
	t.Helper()
	fromSyncStrengOverride.Store(ab)
	t.Cleanup(func() { fromSyncStrengOverride.Store(0) })
}

func fromSyncRuecknahmeSetzen(t *testing.T, bis int64) {
	t.Helper()
	fromSyncStrengBisOverride.Store(bis)
	t.Cleanup(func() { fromSyncStrengBisOverride.Store(0) })
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
				if got := fromSyncAusnahmeFinalitaet(b); got != fs {
					t.Fatalf("Zeit %d FromSync=%v SelfFetched=%v: fromSyncAusnahmeFinalitaet=%v", zeit, fs, sf, got)
				}
				// Sperre mit und ohne wartende Waise: schlafend nur FromSync||SelfFetched.
				for _, waise := range []bool{false, true} {
					d := newOrphanTestDAG()
					b.Hash = "h-schlafend"
					if waise {
						d.orphans[b.Hash] = []*Block{{Hash: "kind"}}
					}
					if got := d.sperrpruefungEntfaellt(b); got != (fs || sf) {
						t.Fatalf("Zeit %d FromSync=%v SelfFetched=%v Waise=%v: sperrpruefungEntfaellt=%v", zeit, fs, sf, waise, got)
					}
				}
			}
		}
	}
	if fromSyncStrengJetzt() {
		t.Fatal("schlafend: Empfangszeit gilt als streng")
	}
	// Eine Ruecknahme ohne gesetzten Anfang aendert schlafend nichts.
	fromSyncRuecknahmeSetzen(t, nowUnix()-3600)
	if fromSyncStreng(nowUnix()) || fromSyncStreng(0) || fromSyncStrengJetzt() {
		t.Fatal("schlafend mit gesetztem Bis: streng")
	}
	fromSyncStrengBisOverride.Store(0)

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
// ohne TLS eingeschleust), wird ab dem Stichtag abgewiesen -- auch mit ALTER
// Blockzeit (Befund F2): ein Ast unter der Finalitaet hat alte Eltern und
// darf seine Zeit alt waehlen, massgeblich ist dort die Empfangszeit.
// Gegenprobe: vor dem Stichtag (Empfangszeit) kommt der alte Block wie
// bisher durch.
func TestFromSyncStichtag_FinalitaetGiltAuchBeimSync(t *testing.T) {
	ab := nowUnix() - 3600
	fromSyncStichtagSetzen(t, ab)
	finalitaet := func() *ChainState {
		return &ChainState{finalizedHeightCache: 1000, finalizedHashCache: "f", finalizedCacheLoaded: true}
	}

	neu := signTestBlockWithParent(t, 1, "deadbeef")
	neu.FromSync = true
	dag := fromSyncAnnahmeDAG(t, neu.Proposer, finalitaet())
	if !dag.isFinalityViolation(neu) {
		t.Fatal("ab dem Stichtag: FromSync-Block unter dem Checkpoint ist keine Verletzung")
	}
	if dag.AddPeerBlock(neu) {
		t.Fatal("ab dem Stichtag: FromSync-Block unter dem Checkpoint angenommen")
	}
	if dag.hasBlockInMemory(neu.Hash) {
		t.Fatal("abgewiesener Block steht im DAG")
	}

	// F2: alte Blockzeit, Empfang nach dem Stichtag.
	alt := signTestBlockWithZeit(t, 1, "deadbeef", ab-86400)
	alt.FromSync = true
	if !fromSyncAusnahme(alt) {
		t.Fatal("Vorbedingung: nach der Blockzeit allein waere der alte Block ausgenommen -- ohne die Empfangszeit kaeme er durch")
	}
	dagAlt := fromSyncAnnahmeDAG(t, alt.Proposer, finalitaet())
	if !dagAlt.isFinalityViolation(alt) {
		t.Fatal("F2: alter FromSync-Block unter der Finalitaet nach dem Stichtag ist keine Verletzung")
	}
	if dagAlt.AddPeerBlock(alt) || dagAlt.hasBlockInMemory(alt.Hash) {
		t.Fatal("F2: alter FromSync-Block unter der Finalitaet nach dem Stichtag angenommen")
	}

	// Gegenprobe: Empfang VOR dem Stichtag -- wie bisher nachladbar.
	fromSyncStrengOverride.Store(nowUnix() + 3600)
	alt2 := signTestBlockWithZeit(t, 1, "deadbeef", ab-86400)
	alt2.FromSync = true
	dagVor := fromSyncAnnahmeDAG(t, alt2.Proposer, finalitaet())
	if dagVor.isFinalityViolation(alt2) {
		t.Fatal("vor dem Stichtag muss ein alter Block vom Seed weiter nachladbar sein")
	}
	if !dagVor.AddPeerBlock(alt2) {
		t.Fatal("vor dem Stichtag wurde ein alter Block abgewiesen")
	}
}

// MISSBRAUCH (Befund F2): ein Block mit GEFAELSCHTEM Erzeuger (nicht
// zugelassen) und einer Blockzeit vor dem 30.09.2026 kommt ueber
// syncGeschichte an der Erzeugerpruefung vorbei. Ab dem Stichtag (Empfang)
// faellt er an der Finalitaet, sobald er unter ihr liegt. Gegenprobe:
// schlafend kommt derselbe Block durch (so war es vorher) -- die Abweisung
// kommt also von der neuen Regel.
func TestFromSyncStichtag_SyncGeschichteGefaelschterErzeuger(t *testing.T) {
	zeit := zeitstempelPruefungAbUnix - 86400
	if !syncGeschichte(zeit) {
		t.Fatal("Vorbedingung: die Blockzeit liegt in der Sync-Geschichte")
	}
	versuch := func() bool {
		b := signTestBlockWithZeit(t, 1, "deadbeef", zeit)
		b.FromSync, b.SelfFetched = true, true
		// zugelassen ist ein ANDERER Schluessel -- der Erzeuger ist gefaelscht.
		dag := fromSyncAnnahmeDAG(t, "0x00000000000000000000000000000000000000aa",
			&ChainState{finalizedHeightCache: 1000, finalizedHashCache: "f", finalizedCacheLoaded: true})
		if dag.erzeugerErlaubt(strings.ToLower(b.Proposer), b.Timestamp) {
			t.Fatal("Vorbedingung: der Erzeuger ist nicht zugelassen")
		}
		ok := dag.AddPeerBlock(b)
		if ok != dag.hasBlockInMemory(b.Hash) {
			t.Fatalf("AddPeerBlock=%v, im DAG=%v", ok, dag.hasBlockInMemory(b.Hash))
		}
		return ok
	}
	fromSyncStichtagSetzen(t, math.MaxInt64)
	if !versuch() {
		t.Fatal("schlafend: syncGeschichte-Block abgewiesen -- das war vorher nicht so")
	}
	fromSyncStrengOverride.Store(nowUnix() - 3600)
	if versuch() {
		t.Fatal("F2: Block mit gefaelschtem Erzeuger und alter Blockzeit unter der Finalitaet nach dem Stichtag angenommen")
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
	if !strings.Contains(body, "!dag.sperrpruefungEntfaellt(block)") {
		t.Fatal("die Sperrpruefung haengt nicht an sperrpruefungEntfaellt")
	}
	fin := functionBodyFromSource(t, "finality.go", "func (dag *BlockDAG) isFinalityViolation(")
	if !strings.Contains(fin, "fromSyncAusnahmeFinalitaet(block)") {
		t.Fatal("die Finalitaet haengt nicht an fromSyncAusnahmeFinalitaet (Empfangszeit, F2)")
	}
	fromSyncStichtagSetzen(t, nowUnix()-3600)
	if fromSyncAusnahme(&Block{Timestamp: nowUnix(), FromSync: true}) {
		t.Fatal("ab dem Stichtag entbindet FromSync die Strafe")
	}
}

// Befund F1: ab dem Stichtag entbindet FromSync/SelfFetched von der Sperre
// nur noch, wenn eine Waise auf genau diesen Block wartet. Ein Block von
// aussen (Push, weder FromSync noch SelfFetched) nie.
func TestFromSyncStichtag_SperreNurOhneWartendeWaise(t *testing.T) {
	fromSyncStichtagSetzen(t, nowUnix()-3600)
	for _, w := range []struct {
		name   string
		fs, sf bool
	}{{"FromSync", true, false}, {"SelfFetched", false, true}, {"beide", true, true}, {"Push", false, false}} {
		dag := newOrphanTestDAG()
		b := &Block{Hash: "v-block", Timestamp: nowUnix(), FromSync: w.fs, SelfFetched: w.sf}
		if dag.sperrpruefungEntfaellt(b) {
			t.Fatalf("%s: ohne wartende Waise entfaellt die Sperre", w.name)
		}
		dag.queueOrphan(b.Hash, &Block{Hash: "kind", Height: 2, ParentHashes: []string{b.Hash}, Proposer: "0xehrlich"})
		if got, want := dag.sperrpruefungEntfaellt(b), w.fs || w.sf; got != want {
			t.Fatalf("%s: mit wartender Waise sperrpruefungEntfaellt=%v, erwartet %v", w.name, got, want)
		}
	}
}

// MISSBRAUCH / VERFUEGBARKEIT (Befund F1), mit echter Datenbank: V hat doppelt
// signiert und ist gesperrt (lastOffenseAt = Blockzeit der Tat,
// suspendedUntil in 14 Tagen). Ein nachholender Knoten bekommt ab dem
// Stichtag V's Folgeblock v1 und den ehrlichen Nachfahren h von W.
//
//   - v1 allein: abgewiesen (keine Waise wartet).
//   - V's eigenes Kind v2 auf v1: an der Sperre abgewiesen, wird KEINE Waise
//     -- V kann sich die Ausnahme nicht selbst verschaffen.
//   - h von W: wird Waise auf v1. Der gezielt nachgeholte v1 (SelfFetched)
//     wird angenommen, h danach aufgeloest: der Knoten bleibt nicht stehen.
//   - Gegenprobe: derselbe v1 als Push (weder FromSync noch SelfFetched)
//     bleibt trotz wartender Waise gesperrt.
func TestFromSyncStichtag_NachholenderBleibtNichtStehen_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-fromsync-waise.json")
	if !cs.useDB {
		t.Fatal("keine Datenbank")
	}
	fromSyncStichtagSetzen(t, nowUnix()-3600)

	keyV, _ := crypto.GenerateKey()
	keyW, _ := crypto.GenerateKey()
	v := strings.ToLower(crypto.PubkeyToAddress(keyV.PublicKey).Hex())
	w := strings.ToLower(crypto.PubkeyToAddress(keyW.PublicKey).Hex())
	for _, q := range []string{`DELETE FROM validator_penalties`, `DELETE FROM equivocation_evidence`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cs.db.Exec(`INSERT INTO validator_penalties (signing_address, offense_count, suspended_until, banned, last_offense_at) VALUES ($1, 1, $2, FALSE, $3)`,
		v, nowUnix()+equivocationFirstOffenseSuspensionDays*86400, nowUnix()-600); err != nil {
		t.Fatal(err)
	}
	cs.invalidatePenaltyCache()

	neuerDAG := func() *BlockDAG {
		dag := fromSyncAnnahmeDAG(t, v, cs)
		dag.authorizedValidators[w] = true
		return dag
	}
	geholt := func(b *Block) *Block {
		c := *b
		c.FromSync, c.SelfFetched = true, true
		return &c
	}

	v1 := signTestBlockFromKey(t, keyV, 1, "deadbeef")
	if g, _ := cs.IsValidatorSuspended(v, v1.Timestamp); !g {
		t.Fatal("Vorbedingung: V ist fuer v1 gesperrt")
	}
	v2 := signTestBlockFromKey(t, keyV, 2, v1.Hash)
	h := signTestBlockFromKey(t, keyW, 2, v1.Hash)

	dag := neuerDAG()
	if dag.AddPeerBlock(geholt(v1)) {
		t.Fatal("v1 ohne wartende Waise angenommen -- die Sperre muss ab dem Stichtag greifen")
	}
	if dag.AddPeerBlock(geholt(v2)) || dag.hasAwaitingOrphan(v1.Hash) {
		t.Fatal("V's eigenes Kind hat eine Waise auf v1 erzeugt -- V verschafft sich die Ausnahme selbst")
	}
	if dag.AddPeerBlock(geholt(v1)) {
		t.Fatal("v1 nach V's eigenem Kind angenommen")
	}

	// Gegenprobe: Push bleibt gesperrt, auch mit wartender Waise.
	pushDAG := neuerDAG()
	if pushDAG.AddPeerBlock(geholt(h)) || !pushDAG.hasAwaitingOrphan(v1.Hash) {
		t.Fatal("Vorbedingung: h wartet als Waise auf v1")
	}
	push := *v1
	push.FromSync, push.SelfFetched = false, false
	if pushDAG.AddPeerBlock(&push) {
		t.Fatal("v1 als Push trotz Sperre angenommen")
	}

	// Der Nachholende: h wird Waise, v1 gezielt nachgeholt, h aufgeloest.
	if dag.AddPeerBlock(geholt(h)) {
		t.Fatal("h ohne Elternblock angenommen")
	}
	if !dag.hasAwaitingOrphan(v1.Hash) {
		t.Fatal("h wartet nicht als Waise auf v1")
	}
	if !dag.AddPeerBlock(geholt(v1)) {
		t.Fatal("F1: v1 trotz wartender ehrlicher Waise abgewiesen -- der Nachholende bleibt stehen")
	}
	frist := time.Now().Add(10 * time.Second)
	for !dag.hasBlockInMemory(h.Hash) {
		if time.Now().After(frist) {
			t.Fatal("F1: der ehrliche Nachfahre h wurde nach v1 nicht eingehaengt")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Befund F3: Ruecknahme ueber einen zweiten Stichtag Bis. Bloecke mit
// Blockzeit in [Ab, Bis) bleiben fuer immer streng (auch wenn ein Knoten sie
// erst nach Bis nachholt), ab Bis gilt wieder die alte Ausnahme. Toleranzen
// zugunsten der Pruefung an beiden Raendern; ein Bis <= Ab wird nicht
// beachtet (fail-closed).
func TestFromSyncStichtag_Ruecknahme(t *testing.T) {
	ab := nowUnix() - 10*86400
	bis := nowUnix() - 86400
	fromSyncStichtagSetzen(t, ab)
	fromSyncRuecknahmeSetzen(t, bis)
	for _, c := range []struct {
		zeit   int64
		streng bool
	}{
		{ab - zeitstempelRueckToleranz - 1, false},
		{ab - zeitstempelRueckToleranz, true},
		{(ab + bis) / 2, true},
		{bis, true}, // vorausdatiert um die Zukunfts-Toleranz: entgeht nicht
		{bis + zeitstempelZukunftToleranz - 1, true},
		{bis + zeitstempelZukunftToleranz, false},
		{math.MaxInt64, false},
	} {
		if got := fromSyncStreng(c.zeit); got != c.streng {
			t.Fatalf("fromSyncStreng(%d) = %v bei [%d, %d), erwartet %v", c.zeit, got, ab, bis, c.streng)
		}
	}
	// Empfang nach Bis: die Finalitaet ist wieder ausgenommen -- aber nur fuer
	// Bloecke ausserhalb des Fensters. Ein Block aus dem Fenster bleibt streng
	// (deterministische Geschichte, kein rueckwirkender Wechsel).
	if fromSyncStrengJetzt() {
		t.Fatal("Empfang nach Bis gilt noch als streng")
	}
	imFenster := &Block{Timestamp: (ab + bis) / 2, FromSync: true, SelfFetched: true}
	if fromSyncAusnahme(imFenster) || fromSyncAusnahmeFinalitaet(imFenster) || selbstGeholtOhneSperre(imFenster) {
		t.Fatal("F3: ein Block aus [Ab, Bis) ist nach der Ruecknahme wieder ausgenommen -- die Regel aendert sich rueckwirkend")
	}
	danach := &Block{Timestamp: nowUnix(), FromSync: true, SelfFetched: true}
	if !fromSyncAusnahme(danach) || !fromSyncAusnahmeFinalitaet(danach) || !selbstGeholtOhneSperre(danach) {
		t.Fatal("nach Bis gilt die alte Ausnahme nicht wieder")
	}

	// Gegenprobe zu "Ab zurueck auf MaxInt64" (der alte Rueckweg der Doku):
	// dann waere derselbe Block wieder ausgenommen -- genau die Divergenz.
	fromSyncStrengOverride.Store(math.MaxInt64)
	if !fromSyncAusnahme(imFenster) {
		t.Fatal("Gegenprobe: mit Ab = MaxInt64 waere der Block aus dem Fenster ausgenommen")
	}
	fromSyncStrengOverride.Store(ab)

	// Empfang im Fenster: streng.
	fromSyncStrengBisOverride.Store(nowUnix() + 3600)
	if !fromSyncStrengJetzt() {
		t.Fatal("Empfang in [Ab, Bis) gilt nicht als streng")
	}

	// Bis <= Ab: nicht beachtet, der Stichtag bleibt scharf.
	for _, falsch := range []int64{ab, ab - 1, 1} {
		fromSyncStrengBisOverride.Store(falsch)
		if !fromSyncStreng(nowUnix()) || !fromSyncStrengJetzt() {
			t.Fatalf("Bis = %d <= Ab = %d schaltet den Stichtag ab", falsch, ab)
		}
	}
}

// Die Konstanten selbst: Bis nur nach Ab (sonst schaltete die Ruecknahme
// still nichts zurueck -- fromSyncStreng ignoriert sie dann).
func TestFromSyncStichtag_KonstantenGeordnet(t *testing.T) {
	if fromSyncStrengBisUnix != math.MaxInt64 && fromSyncStrengBisUnix <= fromSyncStrengAbUnix {
		t.Fatalf("fromSyncStrengBisUnix (%d) muss nach fromSyncStrengAbUnix (%d) liegen", fromSyncStrengBisUnix, fromSyncStrengAbUnix)
	}
	if fromSyncStrengBisUnix != math.MaxInt64 && fromSyncStrengAbUnix == math.MaxInt64 {
		t.Fatal("Bis ohne Ab gesetzt")
	}
}

// Befund F5, mit echter Datenbank: ein frisch synchronisierender Knoten sieht
// ab dem Stichtag eine Doppelsignatur aus der Geschichte (FromSync) und
// erkennt sie selbst -- die Strafe steht aber schon in der Kette (ein anderer
// Erkennender hat sie in einen Block gelegt, und der Knoten hat ihn
// nachgespielt). Ergebnis: kein zweites Vergehen, kein zweiter Abzug, keine
// laengere Sperre. Dicht macht das UNIQUE (block_a_hash, block_b_hash) mit
// ON CONFLICT DO NOTHING in vermerkeDoppelsignatur. Gegenprobe: ein ANDERES
// Paar zaehlt sehr wohl -- der Test sieht also Vergehen.
func TestFromSyncStichtag_StrafeSchonInDerKette_RealDB(t *testing.T) {
	k := neuerStrafKnoten(t)
	fromSyncStichtagSetzen(t, nowUnix()-7*86400)
	if !k.block(k.doppelsignatur(nowUnix()-5*86400, "aa", "bb")) {
		t.Fatal("erstes Vergehen abgelehnt")
	}
	zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")
	if !k.block(zweites) {
		t.Fatal("Block mit der Strafe abgelehnt")
	}
	var bisVorher int64
	if err := k.cs.db.QueryRow(`SELECT suspended_until FROM validator_penalties WHERE signing_address = $1`, k.signer).Scan(&bisVorher); err != nil {
		t.Fatal(err)
	}
	vorher := k.guthaben()
	if v := k.vergehen(); v != 2 {
		t.Fatalf("Vorbedingung: %d Vergehen statt 2", v)
	}

	// Die eigene Erkennung beim Nachholen, in beiden Hash-Reihenfolgen.
	for _, paar := range [][2]string{{zweites.BlockAHash, zweites.BlockBHash}, {zweites.BlockBHash, zweites.BlockAHash}} {
		if _, betrag, err := k.cs.DoppelsignaturErkannt(k.signer, paar[0], paar[1], zweites.DetectedAt, zweites.Doppelbeweis); err != nil || betrag != 0 {
			t.Fatalf("Erkennung eines schon vermerkten Paars: %.6f AEQ, %v", betrag, err)
		}
	}
	// Und eine zweite Strafe-Transaktion desselben Paars in einem Block.
	if !k.block(zweites) {
		t.Fatal("Block mit doppelter Strafe-Transaktion abgelehnt")
	}
	var bisNachher int64
	k.cs.db.QueryRow(`SELECT suspended_until FROM validator_penalties WHERE signing_address = $1`, k.signer).Scan(&bisNachher)
	if v := k.vergehen(); v != 2 {
		t.Fatalf("F5: %d Vergehen statt 2 -- die Strafe aus der Kette wurde ein zweites Mal gezaehlt", v)
	}
	if n := k.beweise(zweites); n != 1 {
		t.Fatalf("F5: %d Beweise fuer ein Paar", n)
	}
	if got := k.guthaben(); got != vorher {
		t.Fatalf("F5: Guthaben %.6f statt %.6f -- zweiter Abzug", got, vorher)
	}
	if bisNachher != bisVorher {
		t.Fatalf("F5: Sperre verlaengert (%d statt %d)", bisNachher, bisVorher)
	}

	// Gegenprobe: ein neues Paar zaehlt (drittes Vergehen: dauerhaft).
	drittes := k.doppelsignatur(nowUnix()-1800, "ee", "ff")
	if !k.block(drittes) {
		t.Fatal("Block mit neuem Paar abgelehnt")
	}
	if v := k.vergehen(); v != 3 {
		t.Fatalf("Gegenprobe: neues Paar ergab %d Vergehen statt 3", v)
	}
}
