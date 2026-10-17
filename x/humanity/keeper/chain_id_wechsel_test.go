package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// Chain-ID-Wechsel (chain_id_wechsel.go): Gutfall, Missbrauch, und dass
// schlafend alles bleibt, wie es war.

// wechselAb setzt den Stichtag fuer einen Test.
func wechselAb(t *testing.T, ab int64) {
	t.Helper()
	chainIDWechselOverride.Store(ab)
	t.Cleanup(func() { chainIDWechselOverride.Store(0) })
}

// SCHLAFEND: der Stichtag im Code ist math.MaxInt64, und dann ist alles
// byte-gleich zu vorher -- jede Blockzeit, jede Annahmezeit, jede Antwort.
//
// Seit der Stichtag gesetzt ist (17.10.2026), prueft der Test den schlafenden
// Zustand ueber den Override -- er beschreibt das Verhalten ohne Stichtag,
// nicht den Inhalt der Konstante.
func TestChainIDWechsel_SchlafendByteGleich(t *testing.T) {
	wechselAb(t, math.MaxInt64)
	if chainIDWechselGeplant() || ChainIDWechselAbFuerStatus() != 0 {
		t.Fatal("schlafend, aber ein Wechsel gilt als geplant")
	}
	for _, z := range []int64{math.MinInt64, 0, 1, 1790452800, nowUnix(), math.MaxInt64 - 1, math.MaxInt64} {
		if got := chainIDWertFuer(z); got != 1926 {
			t.Fatalf("Zeit %d: Chain-ID %d statt 1926", z, got)
		}
		if got := chainIDHexFuer(z); got != chainIDHex {
			t.Fatalf("Zeit %d: %s statt 0x786", z, got)
		}
	}
	if got := chainConfigFuer(math.MaxUint64).ChainID; got.Int64() != 1926 {
		t.Fatalf("EVM-Chain-ID %v", got)
	}
	if got := chainConfig().ChainID; got.Int64() != 1926 {
		t.Fatalf("EVM-Chain-ID jetzt %v", got)
	}

	// RPC wie bisher.
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	if v, e := srv.dispatch("eth_chainId", nil, nil); e != nil || v != "0x786" {
		t.Fatalf("eth_chainId %v %v", v, e)
	}
	if v, e := srv.dispatch("net_version", nil, nil); e != nil || v != "1926" {
		t.Fatalf("net_version %v %v", v, e)
	}

	// Blockpruefung: dieselben Nonces fuer jede Blockzeit, auch MaxInt64,
	// und kein Zeitfenster aus der Chain-ID.
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	txs := []Transaction{signiere(t, a, b.addr, aeqWei(1), 3, 1926), signiereV7(t, a, b.addr, aeqWei(2), 4)}
	for _, z := range []int64{1, nowUnix(), math.MaxInt64} {
		l, err := pruefeUeberweisungenImBlock(txs, z)
		if err != nil || len(l) != 2 || l[0].nonce != 3 || l[1].nonce != 4 {
			t.Fatalf("Blockzeit %d: %v %+v", z, err, l)
		}
		for i := range txs {
			if _, ok := chainIDFenster(&txs[i]); ok {
				t.Fatal("schlafend, aber ein Chain-ID-Fenster")
			}
			if err := chainIDTauglich(&txs[i], z); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Die neue ID gilt schlafend nie -- auch nicht bei MaxInt64.
	neu := signiere(t, a, b.addr, aeqWei(1), 5, chainIDNeuWert)
	if _, err := pruefeUeberweisungsRoh(&neu, math.MaxInt64); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Fatalf("neue ID schlafend angenommen: %v", err)
	}
	// Blockzeit wie bisher: alles passt zu jetzt.
	jetzt := nowUnix()
	if k, z, err := blockZeitPraefix(append(txs, neu), jetzt, jetzt-5); err != nil || k != 3 || z != jetzt {
		t.Fatalf("blockZeitPraefix %d %d %v", k, z, err)
	}
	// Annahme: 1926 immer, alles andere nie.
	for _, ueb := range []bool{true, false} {
		if err := chainIDAnnahmePruefen(big.NewInt(1926), ueb, math.MaxInt64); err != nil {
			t.Fatal(err)
		}
		if err := chainIDAnnahmePruefen(big.NewInt(chainIDNeuWert), ueb, math.MaxInt64); !errors.Is(err, ErrChainIDFalsch) {
			t.Fatalf("neue ID schlafend: %v", err)
		}
	}
	// V8-Domaene wie bisher (mit 1926).
	addr := common.HexToAddress(V7_CONTRACT_ADDR)
	if v8DomainSeparator(chainIDFuer(math.MaxInt64), addr, testSalt) != v8DomainSeparator(big.NewInt(1926), addr, testSalt) {
		t.Fatal("V8-Domaene schlafend veraendert")
	}
}

// Missbrauch: eine alte 1926-Signatur nach dem Stichtag -- im Block
// abgelehnt. Vor dem Stichtag gilt sie (alte Bloecke spielen nach), die neue
// ID davor nicht.
func TestChainIDWechsel_BlockpruefungNachBlockzeit(t *testing.T) {
	const ab int64 = 1_900_000_000
	wechselAb(t, ab)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	alt := signiere(t, a, b.addr, aeqWei(1), 0, 1926)
	altV7 := signiereV7(t, a, b.addr, aeqWei(1), 1)
	neu := signiere(t, a, b.addr, aeqWei(1), 2, chainIDNeuWert)

	// Alte Bloecke: 1926 gilt bis ab-1.
	for _, z := range []int64{1790452800, ab - 1} {
		if _, err := pruefeUeberweisungenImBlock([]Transaction{alt, altV7}, z); err != nil {
			t.Fatalf("alter Block %d abgelehnt: %v", z, err)
		}
		if _, err := pruefeUeberweisungsRoh(&neu, z); !errors.Is(err, ErrUeberweisungNichtSigniert) {
			t.Fatalf("neue ID vor dem Stichtag angenommen (Block %d): %v", z, err)
		}
	}
	// Ab dem Stichtag: 1926 abgelehnt, auch V7-transfer, die neue ID gilt.
	for _, z := range []int64{ab, ab + 86400} {
		for _, tx := range []Transaction{alt, altV7} {
			if _, err := pruefeUeberweisungenImBlock([]Transaction{neu, tx}, z); !errors.Is(err, ErrUeberweisungNichtSigniert) {
				t.Fatalf("alte 1926-Signatur im Block %d angenommen: %v", z, err)
			}
		}
		if n, err := pruefeUeberweisungsRoh(&neu, z); err != nil || n != 2 {
			t.Fatalf("neue ID ab dem Stichtag abgelehnt: %v", err)
		}
	}
	// Die neue Signatur ist eine andere Rohform: ein Faelscher kann die alte
	// nicht umschreiben, ohne dass Absender oder TxHash nicht mehr passen.
	umgeschrieben := alt
	umgeschrieben.Roh = neu.Roh
	if _, err := pruefeUeberweisungsRoh(&umgeschrieben, ab); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Fatalf("fremde Rohform unter altem TxHash angenommen: %v", err)
	}
	// Eine dritte Chain-ID gilt nie.
	fremd := signiere(t, a, b.addr, aeqWei(1), 3, 1)
	for _, z := range []int64{ab - 1, ab} {
		if _, err := pruefeUeberweisungsRoh(&fremd, z); !errors.Is(err, ErrUeberweisungNichtSigniert) {
			t.Fatalf("Chain-ID 1 angenommen: %v", err)
		}
	}
}

// Der Absender einer Transaktion mit der neuen ID wird richtig
// wiederhergestellt -- fuer jede Transaktionsart; fremde IDs scheitern wie
// bisher (absender_schnell_test.go).
func TestChainIDWechsel_AbsenderNeueID(t *testing.T) {
	k, _ := crypto.GenerateKey()
	want := crypto.PubkeyToAddress(k.PublicKey)
	an := common.HexToAddress(testRecipientHex)
	cid := big.NewInt(chainIDNeuWert)
	for name, inner := range map[string]types.TxData{
		"legacy":     &types.LegacyTx{To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(0)},
		"accesslist": &types.AccessListTx{ChainID: cid, To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(1)},
		"dynamicfee": &types.DynamicFeeTx{ChainID: cid, To: &an, Value: big.NewInt(1), Gas: 21000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2)},
	} {
		tx, err := types.SignNewTx(k, types.NewCancunSigner(cid), inner)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := types.Sender(types.LatestSignerForChainID(cid), tx)
		if err != nil || ref != want {
			t.Fatalf("%s: Referenz %v %v", name, ref, err)
		}
		got, err := absenderWiederherstellen(tx)
		if err != nil || got != want {
			t.Fatalf("%s: %s %v, erwartet %s", name, got.Hex(), err, want.Hex())
		}
	}
}

// Missbrauch an der Annahme: 1926 nur bis zur Pause, die neue ID erst ab dem
// Stichtag, dazwischen wiederholbar keine Ueberweisung.
func TestChainIDWechsel_AnnahmeFenster(t *testing.T) {
	const ab int64 = 2_000_000_000
	wechselAb(t, ab)
	alt, neu := big.NewInt(1926), big.NewInt(chainIDNeuWert)
	pause := ab - chainIDWechselPause
	faelle := []struct {
		name         string
		cid          *big.Int
		ueberw       bool
		jetzt        int64
		ok, pausiert bool
	}{
		{"1926 vor der Pause", alt, true, pause - 1, true, false},
		{"1926 in der Pause", alt, true, pause, false, false},
		{"1926 nach dem Stichtag", alt, true, ab, false, false},
		{"neu vor der Pause", neu, true, pause - 1, false, false},
		{"neu in der Pause", neu, true, ab - 1, false, true},
		{"neu ab dem Stichtag", neu, true, ab, true, false},
		{"fremd", big.NewInt(1), true, ab, false, false},
		{"nil", nil, true, ab, false, false},
		// Vertragsaufrufe laufen nur hier in der EVM: genau die ID der Zeit.
		{"Aufruf 1926 in der Pause", alt, false, ab - 1, true, false},
		{"Aufruf 1926 ab dem Stichtag", alt, false, ab, false, false},
		{"Aufruf neu vor dem Stichtag", neu, false, ab - 1, false, false},
		{"Aufruf neu ab dem Stichtag", neu, false, ab, true, false},
	}
	for _, f := range faelle {
		err := chainIDAnnahmePruefen(f.cid, f.ueberw, f.jetzt)
		if (err == nil) != f.ok || errors.Is(err, ErrAnnahmePausiert) != f.pausiert {
			t.Errorf("%s: %v", f.name, err)
		}
	}
}

// Ueber den ganzen Annahmeweg (sendRawTransaction): nach dem Stichtag wird
// eine alte 1926-Ueberweisung abgewiesen, in der Pause wiederholbar auch die
// neue, vor dem Stichtag die neue.
func TestChainIDWechsel_SendRawTransaction(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	noteBlockProduced()
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	cs.mu.Unlock()
	nonce := uint64(0)
	sende := func(cid int64) *RPCError {
		t.Helper()
		tx := signiere(t, a, b.addr, aeqWei(1), nonce, cid)
		nonce++
		p, _ := json.Marshal(tx.Roh)
		_, rerr := srv.sendRawTransaction([]json.RawMessage{p}, nil)
		return rerr
	}
	istChainFehler := func(e *RPCError) bool {
		return e != nil && e.Code == -32000 && strings.Contains(e.Message, "chain id")
	}

	jetzt := nowUnix()
	// Nach dem Stichtag.
	wechselAb(t, jetzt-10)
	if e := sende(1926); !istChainFehler(e) {
		t.Fatalf("alte 1926-Ueberweisung nach dem Stichtag nicht abgewiesen: %v", e)
	}
	if e := sende(chainIDNeuWert); istChainFehler(e) || (e != nil && e.Code == -32005) {
		t.Fatalf("neue ID nach dem Stichtag abgewiesen: %v", e)
	}
	// In der Pause.
	chainIDWechselOverride.Store(jetzt + 100)
	if e := sende(chainIDNeuWert); e == nil || e.Code != -32005 {
		t.Fatalf("neue ID in der Pause nicht wiederholbar abgewiesen: %v", e)
	}
	if e := sende(1926); !istChainFehler(e) {
		t.Fatalf("1926 in der Pause angenommen: %v", e)
	}
	// Lange vor dem Stichtag.
	chainIDWechselOverride.Store(jetzt + 86400)
	if e := sende(chainIDNeuWert); !istChainFehler(e) {
		t.Fatalf("neue ID vor dem Stichtag angenommen: %v", e)
	}
	if e := sende(1926); istChainFehler(e) {
		t.Fatalf("1926 vor dem Stichtag abgewiesen: %v", e)
	}
}

// Der Ausgang ueber die Grenze: was mit 1926 angenommen wurde und beim
// Stichtag noch nicht verblockt ist, kommt in einen Block mit der Zeit davor
// -- nie in einen danach (den jeder andere verwiese). Die neue ID danach.
func TestChainIDWechsel_AusgangUeberDieGrenze(t *testing.T) {
	const ab int64 = 1_950_000_000
	wechselAb(t, ab)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	alt1 := signiere(t, a, b.addr, aeqWei(1), 0, 1926)
	alt2 := signiereV7(t, a, b.addr, aeqWei(1), 1)
	neu := signiere(t, a, b.addr, aeqWei(1), 2, chainIDNeuWert)

	if err := blockTauglich(&alt1, ab); err == nil {
		t.Fatal("1926 zum Stichtag tauglich")
	}
	if err := blockTauglich(&neu, ab-1); err == nil {
		t.Fatal("neue ID vor dem Stichtag tauglich")
	}

	// Eltern vor dem Stichtag, jetzt danach: die 1926er in einen Block mit
	// ab-1, die neue bleibt fuer den naechsten.
	k, z, err := blockZeitPraefix([]Transaction{alt1, alt2, neu}, ab+30, ab-20)
	if err != nil || k != 2 || z != ab-1 {
		t.Fatalf("Praefix %d, Zeit %d, %v -- erwartet 2 bei %d", k, z, err, ab-1)
	}
	// Der Block, den jeder andere nachspielt, besteht.
	if _, err := pruefeUeberweisungenImBlock([]Transaction{alt1, alt2}, z); err != nil {
		t.Fatalf("Block mit ab-1 abgelehnt: %v", err)
	}
	k, z, err = blockZeitPraefix([]Transaction{neu}, ab+31, ab-1)
	if err != nil || k != 1 || z != ab+31 {
		t.Fatalf("neue ID: %d %d %v", k, z, err)
	}
	// Liegt schon ein Block ab dem Stichtag vor, gibt es fuer eine 1926er
	// keine Zeit mehr: kein Block (fail-closed), nie einer, den jeder abweist.
	if _, _, err := blockZeitPraefix([]Transaction{alt1}, ab+60, ab); err == nil {
		t.Fatal("1926 in einen Block nach dem Stichtag gelegt")
	}
	// Eine Rohform mit fremder ID passt in keinen Block.
	fremd := signiere(t, a, b.addr, aeqWei(1), 3, 5)
	if f, ok := chainIDFenster(&fremd); !ok || f.von <= f.bis {
		t.Fatalf("fremde ID: Fenster %+v %v", f, ok)
	}
}

// V8: der Digest aus Go gleich DOMAIN_SEPARATOR() des Vertrags, vor und nach
// dem Stichtag -- die EVM laeuft mit block.chainid = chainIDFuer(Zeit), und
// NETZ_SALT bleibt, was im Konstruktor stand.
func TestChainIDWechsel_V8DomaeneVorUndNach(t *testing.T) {
	const ab int64 = 1_960_000_000
	wechselAb(t, ab)
	_, reg := v8Schluessel(t, testKeyRegistrar)
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	p := neuerV8Pruefstand(t, []common.Address{reg}, testSalt)
	var seps []common.Hash
	for _, z := range []int64{ab - 1, ab} {
		p.evm = neueKnotenEVM(uint64(z), alice, p.sdb)
		ret, err := p.aufruf(alice, sel("3644e515")) // DOMAIN_SEPARATOR()
		want := v8DomainSeparator(chainIDFuer(z), p.vertrag, testSalt)
		if err != nil || common.BytesToHash(ret) != want {
			t.Fatalf("Zeit %d: Vertrag %x, Go %s (%v)", z, ret, want.Hex(), err)
		}
		ret, err = p.aufruf(alice, sel("67522052")) // NETZ_SALT()
		if err != nil || common.BytesToHash(ret) != testSalt {
			t.Fatalf("NETZ_SALT veraendert: %x %v", ret, err)
		}
		seps = append(seps, want)
	}
	if seps[0] == seps[1] {
		t.Fatal("Domaene vor und nach dem Stichtag gleich")
	}

	// Eine mit 1926 unterschriebene Registrierung gilt nach dem Stichtag
	// nicht mehr (Vertrag und Go), die mit der neuen ID schon.
	c, n := big.NewInt(1001), big.NewInt(2001)
	frist := ab + 3600
	sigAlt := v8Unterschrift(t, aliceKey, v8RegisterDigest(big.NewInt(1926), p.vertrag, testSalt, alice, c, n, big.NewInt(0), big.NewInt(frist)))
	if _, err := p.aufruf(reg, v8Anfrage(t, alice, c, n, frist, sigAlt, v8TestMarker)); err == nil {
		t.Fatal("Vertrag nimmt nach dem Stichtag eine 1926-Unterschrift an")
	}
	sigNeu := v8Unterschrift(t, aliceKey, v8RegisterDigest(chainIDFuer(ab), p.vertrag, testSalt, alice, c, n, big.NewInt(0), big.NewInt(frist)))
	if _, err := p.aufruf(reg, v8Anfrage(t, alice, c, n, frist, sigNeu, v8TestMarker)); err != nil {
		t.Fatalf("Vertrag lehnt die Unterschrift mit der neuen ID ab: %v", err)
	}
}

// Beim Nachspielen: pruefeRegistrierungV8 rechnet die Domaene mit
// chainIDFuer(RegAt) -- alte Registrierungen bleiben gueltig, eine alte
// Unterschrift mit einer Annahmezeit nach dem Stichtag nicht.
func TestChainIDWechsel_V8NachspielenNachRegAt(t *testing.T) {
	const ab int64 = 1_970_000_000
	wechselAb(t, ab)
	aliceKey, alice := v8Schluessel(t, testKeyAlice)
	c, n := big.NewInt(1001), big.NewInt(2001)
	tx := func(regAt int64, chainID *big.Int) Transaction {
		frist := regAt + 3600
		d := v8RegisterDigest(chainID, common.HexToAddress(V7_CONTRACT_ADDR), v8NetzSalt(), alice, c, n, big.NewInt(0), big.NewInt(frist))
		return Transaction{
			Type: "register_human", Wallet: strings.ToLower(alice.Hex()), PubSignals: []string{c.String(), n.String()},
			RegAt: regAt, RegFrist: frist, RegSignatur: "0x" + common.Bytes2Hex(v8Unterschrift(t, aliceKey, d)),
		}
	}
	if err := pruefeRegistrierungV8(tx(ab-100, big.NewInt(1926)), ab+500); err != nil {
		t.Fatalf("alte Registrierung (vor dem Stichtag angenommen) abgelehnt: %v", err)
	}
	if err := pruefeRegistrierungV8(tx(ab, big.NewInt(1926)), ab+10); err == nil {
		t.Fatal("1926-Unterschrift mit Annahme nach dem Stichtag angenommen")
	}
	if err := pruefeRegistrierungV8(tx(ab-1, big.NewInt(chainIDNeuWert)), ab+10); err == nil {
		t.Fatal("Unterschrift mit neuer ID vor dem Stichtag angenommen")
	}
	if err := pruefeRegistrierungV8(tx(ab, big.NewInt(chainIDNeuWert)), ab+10); err != nil {
		t.Fatalf("neue ID nach dem Stichtag abgelehnt: %v", err)
	}
}

// EINGEFROREN: Netzkennung, NETZ_SALT, die chain:1926-Saetze und die
// Checkpoint-Domaene bleiben, auch wenn der Wechsel gilt.
func TestChainIDWechsel_EingefrorenesBleibt(t *testing.T) {
	wechselAb(t, 1) // der Wechsel gilt fuer jede Zeit
	if chainIDJetzt() != chainIDNeuWert {
		t.Fatal("Override greift nicht")
	}
	want := fmt.Sprintf("aequitas-1926-%d", genesisTimestamp())
	if got := netzKennung(); got != want {
		t.Fatalf("netzKennung %q, erwartet %q", got, want)
	}
	if v8NetzSalt() != crypto.Keccak256Hash([]byte(want)) {
		t.Fatal("v8NetzSalt veraendert -- NETZ_SALT im Vertrag ist immutable")
	}
	for name, satz := range map[string]string{
		"besitz":     coordinatorBesitzNachricht("0xabc"),
		"freigabe":   coordinatorFreigabeNachricht("0xdef"),
		"erneuerung": erneuerungsNachricht("0xabc", 5),
		"validator":  validatorBindungNachricht("0x1", "0x2", 7),
	} {
		if !strings.Contains(satz, "chain:1926") || strings.Contains(satz, "19260930") {
			t.Errorf("%s: %q", name, satz)
		}
	}
	if checkpointChainID != 1926 || chainIDHex != "0x786" {
		t.Fatalf("Checkpoint-Chain-ID %d / %s", checkpointChainID, chainIDHex)
	}
	// Und im Quelltext: die eingefrorenen Stellen beziehen sich nicht auf
	// den Stichtag. Wer das aendert, aendert NETZ_SALT oder macht
	// gespeicherte Freigaben ungueltig.
	for _, datei := range []string{"api_app_grundlagen.go", "coordinator_nachrichten.go", "validator_register.go", "checkpoint_zertifikat.go"} {
		q, err := os.ReadFile(datei)
		if err != nil {
			t.Fatal(err)
		}
		for _, verboten := range []string{"chainIDFuer", "chainIDWertFuer", "chainIDJetzt", "chainIDNeuWert", "chainIDWechsel"} {
			if strings.Contains(string(q), verboten) {
				t.Errorf("%s bezieht sich auf %s -- muss eingefroren bleiben", datei, verboten)
			}
		}
	}
}

// RPC und Status melden ab dem Stichtag die neue ID.
func TestChainIDWechsel_RPCMeldetNeueID(t *testing.T) {
	wechselAb(t, nowUnix()-1)
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	if v, e := srv.dispatch("eth_chainId", nil, nil); e != nil || v != "0x125e602" {
		t.Fatalf("eth_chainId %v %v", v, e)
	}
	if v, e := srv.dispatch("net_version", nil, nil); e != nil || v != "19260930" {
		t.Fatalf("net_version %v %v", v, e)
	}
	st := statusNotstand(1)
	if st["chain_evm_id"] != int(chainIDNeuWert) || st["chain_id_wechsel_ab"] != chainIDWechselAb() {
		t.Fatalf("Status %v", st)
	}
	if chainConfig().ChainID.Int64() != chainIDNeuWert {
		t.Fatal("EVM-Chain-ID nicht umgestellt")
	}
}

// Die schnelle Lesung der Chain-ID (Blockzeitwahl) stimmt mit der vollen
// Dekodierung ueberein -- fuer jede Art, beide IDs, fremde, ungeschuetzte und
// kaputte Rohformen.
func TestChainIDWechsel_RohChainIDWieDekodiert(t *testing.T) {
	k, _ := crypto.GenerateKey()
	an := common.HexToAddress(testRecipientHex)
	var rohe []string
	for _, kette := range []int64{1926, chainIDNeuWert, 1, 0x7fffffff, 1 << 40} {
		cid := big.NewInt(kette)
		for _, inner := range []types.TxData{
			&types.LegacyTx{To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(0), Data: []byte{1, 2, 3}},
			&types.AccessListTx{ChainID: cid, To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(1)},
			&types.DynamicFeeTx{ChainID: cid, To: &an, Value: big.NewInt(1), Gas: 21000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2)},
		} {
			tx, err := types.SignNewTx(k, types.NewCancunSigner(cid), inner)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := tx.MarshalBinary()
			rohe = append(rohe, "0x"+common.Bytes2Hex(raw))
		}
	}
	unge, _ := types.SignNewTx(k, types.HomesteadSigner{}, &types.LegacyTx{To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(0)})
	raw, _ := unge.MarshalBinary()
	rohe = append(rohe, "0x"+common.Bytes2Hex(raw), "0x", "0xzz", "0x02", "0x02c0", "0xc0", "0x0102")
	for _, r := range rohe {
		got, gotOK := rohChainID(r)
		var want int64
		wantOK := false
		if t2, err := decodeRohTransaktion(r); err == nil && t2.Protected() && t2.ChainId().IsInt64() {
			want, wantOK = t2.ChainId().Int64(), true
		}
		if got != want || gotOK != wantOK {
			t.Errorf("%.20s: schnell (%d, %v), dekodiert (%d, %v)", r, got, gotOK, want, wantOK)
		}
	}
}

// umhuellt: dieselbe signierte Transaktion als RLP-String verpackt -- die
// Form, die decodeRohTransaktion ueber den rlp.DecodeBytes-Rueckfall auch
// annimmt.
func umhuellt(t *testing.T, roh string) string {
	t.Helper()
	tx, err := decodeRohTransaktion(roh)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rlp.EncodeToBytes(tx)
	if err != nil {
		t.Fatal(err)
	}
	if b[0] < 0x80 || b[0] >= 0xc0 {
		t.Fatalf("keine umhuellte Form: %x", b[0])
	}
	return "0x" + common.Bytes2Hex(b)
}

// MISSBRAUCH (Sicherheitspruefung #360, Befund 1): eine 1926-Ueberweisung,
// als RLP-String verpackt eingereicht, bekommt dasselbe Chain-ID-Fenster wie
// in kanonischer Form -- der Erzeuger legt sie nie in einen Block ab dem
// Stichtag, den jeder andere Knoten abweisen wuerde.
func TestChainIDWechsel_UmhuellteFormHatFenster(t *testing.T) {
	ab := nowUnix() + 3600
	wechselAb(t, ab)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	for _, kette := range []int64{1926, chainIDNeuWert} {
		tx := signiere(t, a, b.addr, aeqWei(1), 1, kette)
		tx.Roh = umhuellt(t, tx.Roh)
		if cid, ok := rohChainID(tx.Roh); !ok || cid != kette {
			t.Fatalf("umhuellt: (%d, %v) statt %d", cid, ok, kette)
		}
		f, ok := chainIDFenster(&tx)
		if !ok {
			t.Fatalf("Chain-ID %d umhuellt: kein Fenster", kette)
		}
		nachspiel := func(z int64) error { _, err := pruefeUeberweisungsRoh(&tx, z); return err }
		// Erzeuger (Fenster) und Nachspielender sind sich an beiden Seiten
		// des Stichtags einig.
		for _, z := range []int64{ab - 1, ab} {
			imFenster := z >= f.von && z <= f.bis
			if (chainIDTauglich(&tx, z) == nil) != imFenster {
				t.Fatalf("Chain-ID %d, Zeit %d: Tauglich passt nicht zum Fenster", kette, z)
			}
			if (nachspiel(z) == nil) != imFenster {
				t.Fatalf("Chain-ID %d, Zeit %d: Erzeuger und Nachspielender uneins (%v)", kette, z, nachspiel(z))
			}
		}
	}
	// Ungeschuetzt und Muell umhuellt bleiben ohne Chain-ID.
	for _, r := range []string{"0x80", "0x81ff", "0x8102"} {
		if _, ok := rohChainID(r); ok {
			t.Fatalf("%s: Chain-ID gelesen", r)
		}
	}
}

// MISSBRAUCH (Sicherheitspruefung #360, Befund 2): nach dem Stichtag reicht
// jemand gesammelte alte 1926-Ueberweisungen eines Opfers als Buendel mit
// fortlaufenden Noncen ein. Sie werden abgelehnt -- und duerfen dabei keine
// Nonce des Opfers verbrauchen.
func TestChainIDWechsel_BuendelVerbrenntKeineNoncen(t *testing.T) {
	wechselAb(t, nowUnix()-100)
	s := newReserveTestServer(t)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	var buendel []*precomputedSendTx
	for n := uint64(0); n < 3; n++ {
		roh := signiere(t, a, b.addr, aeqWei(1), n, 1926).Roh
		tx, err := decodeRohTransaktion(roh)
		if err != nil {
			t.Fatal(err)
		}
		buendel = append(buendel, &precomputedSendTx{tx: tx, sender: a.addr})
	}
	s.preReserveBatchNonces(buendel, allIndexes(len(buendel)))
	for i, p := range buendel {
		if p.nonceReserved {
			t.Fatalf("Eintrag %d: Nonce fuer eine abgelehnte 1926-Ueberweisung reserviert", i)
		}
	}
	if got := s.nonceShardFor(a.addr).nonces[a.addr]; got != 0 {
		t.Fatalf("Nonce des Opfers auf %d verschoben", got)
	}

	// Gutfall: dieselben Ueberweisungen mit der neuen ID werden reserviert.
	var neu []*precomputedSendTx
	for n := uint64(0); n < 3; n++ {
		tx, _ := decodeRohTransaktion(signiere(t, a, b.addr, aeqWei(1), n, chainIDNeuWert).Roh)
		neu = append(neu, &precomputedSendTx{tx: tx, sender: a.addr})
	}
	s.preReserveBatchNonces(neu, allIndexes(len(neu)))
	for i, p := range neu {
		if !p.nonceReserved {
			t.Fatalf("Eintrag %d mit neuer ID nicht reserviert", i)
		}
	}
}
