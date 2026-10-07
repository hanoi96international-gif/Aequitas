package keeper

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// L1: ein Block mit Bindung oder Beweis, dessen Zeit mehr als 30 min hinter
// der eigenen Spitze liegt, wird ab dem Stichtag abgewiesen; puenktlich, aus
// der Geschichte vom Seed, vor dem Stichtag oder ohne solche Transaktion nicht.
func TestSpaetEingehaengt_Regel(t *testing.T) {
	t.Cleanup(func() { validatorRegisterOverride.Store(0); registerLeserOverride.Store(0) })
	block := func(zeit int64, typ string, sync bool) *Block {
		return &Block{Timestamp: zeit, FromSync: sync, Transactions: []Transaction{{Type: "transfer"}, {Type: typ}}}
	}
	zeit := func(z int64) func() int64 { return func() int64 { return z } }
	const t0 = int64(1_000_000)
	spaet := t0 + spaetEingehaengtGrenze + 1
	for _, typ := range []string{"validator_bindung", "slash_equivocation"} {
		validatorRegisterOverride.Store(0)
		registerLeserOverride.Store(0)
		if g := spaetEingehaengt(block(t0, typ, false), zeit(spaet)); g != "" {
			t.Fatalf("%s vor dem Stichtag abgewiesen: %s", typ, g)
		}
		validatorRegisterOverride.Store(1)
		registerLeserOverride.Store(1)
		if g := spaetEingehaengt(block(t0, typ, false), zeit(spaet)); g == "" {
			t.Fatalf("%s: zurueckgehaltener Block angenommen", typ)
		}
		if g := spaetEingehaengt(block(t0, typ, false), zeit(t0+spaetEingehaengtGrenze)); g != "" {
			t.Fatalf("%s: genau an der Grenze abgewiesen: %s", typ, g)
		}
		if g := spaetEingehaengt(block(t0, typ, true), zeit(spaet+86400)); g != "" {
			t.Fatalf("%s: Geschichte vom Seed abgewiesen: %s", typ, g)
		}
	}
	if g := spaetEingehaengt(block(t0, "transfer", false), zeit(spaet)); g != "" {
		t.Fatalf("Block ohne Bindung abgewiesen: %s", g)
	}
}

// Die Grenze laesst jedem Knoten, der eine Zeile je hat, Zeit, bevor sie
// wirkt: Bindung hoechstens nachweisHoechstensAlt vor ihrem Block, Block
// hoechstens spaetEingehaengtGrenze spaet -- mit einer Viertelstunde Luft vor
// der Frist der Erzeugerpruefung und der Abrechnung.
func TestSpaetEingehaengt_GrenzeVorDerFrist(t *testing.T) {
	if luft := erzeugerFrist - nachweisHoechstensAlt - spaetEingehaengtGrenze; luft < 15*60 {
		t.Fatalf("nur %d s Luft vor der Frist", luft)
	}
	// Abrechnung: Bindungen bis D+W in Bloecken bis D+W+1h, Beweis in einem
	// Block bis D+W -- beide bekannt vor der Faelligkeit.
	d := int64(1_000_000)
	if strafeFaelligAb(d)-(d+strafBeweisFrisch+nachweisHoechstensAlt+spaetEingehaengtGrenze) < 15*60 {
		t.Fatal("Abrechnung faellig, bevor jede zaehlende Zeile angekommen sein muss")
	}
}

// Ueber den echten Eingang:
//   - Missbrauch: die Spitze liegt bei jetzt, ein zurueckgehaltener Block mit
//     Bindung von vor 31 min wird abgewiesen, bevor er auch nur als Waise
//     eingereiht wird.
//   - Absturz des einzigen Erzeugers: seine Spitze ist zwei Stunden alt, sein
//     erster Block danach traegt eine Bindung mit der Zeit ihrer Annahme --
//     nach der Uhr weit zurueck, gegen die Spitze puenktlich. Er kommt durch
//     (bis zur Waisenliste, Eltern fehlen); an der Uhr gemessen risse hier
//     die Kette.
func TestSpaetEingehaengt_AddPeerBlock(t *testing.T) {
	validatorRegisterOverride.Store(1)
	t.Cleanup(func() { validatorRegisterOverride.Store(0) })
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	proposer := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	lauf := func(spitze, zeit int64) (eingereiht bool) {
		dag := newUnknownProposerTestDAG()
		dag.authorizedValidators[proposer] = true
		dag.blocks["spitze"] = &Block{Hash: "spitze", Height: 1, Timestamp: spitze}
		dag.tips = map[string]bool{"spitze": true}
		fehlt := "fehlt-" + t.Name()
		b := signierterTestblock(t, key, 2, zeit, []string{fehlt}, []Transaction{{Type: "validator_bindung", Wallet: "0xm"}})
		dag.AddPeerBlock(b)
		dag.mu.RLock()
		defer dag.mu.RUnlock()
		return len(dag.orphans[fehlt]) > 0
	}
	jetzt := nowUnix()
	if !lauf(jetzt, jetzt-60) {
		t.Fatal("puenktlicher Block nicht bis zur Waisenliste gekommen -- der Test beweist nichts")
	}
	if lauf(jetzt, jetzt-spaetEingehaengtGrenze-60) {
		t.Fatal("zurueckgehaltener Block mit Bindung eingereiht")
	}
	if !lauf(jetzt-2*3600, jetzt-2*3600+30) {
		t.Fatal("erster Block des Erzeugers nach einem Absturz abgewiesen -- die Kette risse")
	}
}
