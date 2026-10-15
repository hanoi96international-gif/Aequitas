package keeper

import (
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

// Sicherheitspruefung #359 (HOCH): Blob-Transaktionen (Typ 3) mit Sidecar.

// blobUeberweisung: eine signierte Typ-3-Ueberweisung von a an b, ohne
// Sidecar (sc == nil) oder mit Sidecar der Version sc (0 Blobs).
func blobUeberweisung(t *testing.T, a testSchluessel, an string, nonce uint64, sc *byte) (Transaction, *types.Transaction) {
	t.Helper()
	ziel := common.HexToAddress(an)
	stx, err := types.SignNewTx(a.key, types.NewCancunSigner(big.NewInt(1926)), &types.BlobTx{
		ChainID: uint256.NewInt(1926), Nonce: nonce, To: ziel, Value: uint256.NewInt(1e18), Gas: 21000,
		GasTipCap: uint256.NewInt(1), GasFeeCap: uint256.NewInt(2), BlobFeeCap: uint256.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sc != nil {
		stx = stx.WithBlobTxSidecar(types.NewBlobTxSidecar(*sc, nil, nil, nil))
	}
	raw, err := stx.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return Transaction{Type: "transfer", Wallet: a.addr, To: strings.ToLower(ziel.Hex()),
		Amount: betragAusWei(big.NewInt(1e18)), TxHash: stx.Hash().Hex(), Roh: "0x" + common.Bytes2Hex(raw)}, stx
}

// MISSBRAUCH: das Netzformat v1 ([tx, 0x01, blobs, commitments,
// cellProofs]) las v1.13 nicht. Liest es dieser Knoten, bestaetigt er eine
// Ueberweisung, die jeder alte Validator verwirft -- Spaltung durch jeden
// Nutzer. Es muss genau wie unter v1.13 unlesbar sein, beim Einlesen, bei
// der Absenderwiederherstellung und beim Nachspielen.
func TestAnnahmeTyp_SidecarV1UnlesbarWieV113(t *testing.T) {
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	v1 := types.BlobSidecarVersion1
	ueb, _ := blobUeberweisung(t, a, b.addr, 0, &v1)
	if _, err := decodeRohTransaktion(ueb.Roh); err == nil {
		t.Fatal("Sidecar v1 eingelesen -- v1.13 konnte es nicht")
	}
	if _, _, _, err := decodeAndRecoverSender(ueb.Roh); err == nil {
		t.Fatal("Sidecar v1: Absender wiederhergestellt")
	}
	if _, err := pruefeUeberweisungsRoh(&ueb); err == nil {
		t.Fatal("Sidecar v1 beim Nachspielen angenommen -- alte Validatoren verwerfen den Block")
	}

	// Was v1.13 las, bleibt lesbar: alte Bloecke spielen gleich nach.
	v0 := types.BlobSidecarVersion0
	for _, sc := range []*byte{nil, &v0} {
		ueb, _ := blobUeberweisung(t, a, b.addr, 0, sc)
		if _, err := decodeRohTransaktion(ueb.Roh); err != nil {
			t.Fatalf("Sidecar %v: v1.13 las es, jetzt %v", sc, err)
		}
	}
}

// MISSBRAUCH: Typ 3 (mit und ohne Sidecar) und Typ 4 nimmt kein Knoten
// mehr an -- weder einzeln noch im Buendel, und keine Nonce wird dafuer
// verbraucht.
func TestAnnahmeTyp_BlobUndSetCodeAbgelehnt(t *testing.T) {
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	noteBlockProduced()
	cs := newTestState()
	srv := NewEVMRPCServer(&BlockDAG{state: cs}, cs)
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	cs.mu.Unlock()

	v0 := types.BlobSidecarVersion0
	var rohe []string
	for _, sc := range []*byte{nil, &v0} {
		ueb, _ := blobUeberweisung(t, a, b.addr, 0, sc)
		rohe = append(rohe, ueb.Roh)
	}
	for i, roh := range rohe {
		p, _ := json.Marshal(roh)
		_, rerr := srv.sendRawTransaction([]json.RawMessage{p}, nil)
		if rerr == nil || !strings.Contains(rerr.Message, types.ErrTxTypeNotSupported.Error()) {
			t.Fatalf("Typ 3 (%d) angenommen: %v", i, rerr)
		}
	}
	if n := srv.nonceShardFor(a.addr).nonces[a.addr]; n != 0 {
		t.Fatalf("Nonce fuer abgelehnten Typ 3 verbraucht: %d", n)
	}

	// Buendel: fortlaufende Typ-3-Noncen werden nicht vorab reserviert.
	var buendel []*precomputedSendTx
	for n := uint64(0); n < 3; n++ {
		_, stx := blobUeberweisung(t, a, b.addr, n, nil)
		buendel = append(buendel, &precomputedSendTx{tx: stx, sender: a.addr})
	}
	srv.preReserveBatchNonces(buendel, allIndexes(len(buendel)))
	for i, p := range buendel {
		if p.nonceReserved {
			t.Fatalf("Eintrag %d: Nonce fuer Typ 3 reserviert", i)
		}
		if _, rerr := srv.sendRawTransaction(nil, p); rerr == nil {
			t.Fatalf("Eintrag %d: Typ 3 im Buendel angenommen", i)
		}
	}

	// Gegenprobe: Typ 2 desselben Absenders geht durch das Typ-Tor.
	ok := signiere(t, a, b.addr, aeqWei(1), 0, 1926)
	p, _ := json.Marshal(ok.Roh)
	if _, rerr := srv.sendRawTransaction([]json.RawMessage{p}, nil); rerr != nil && strings.Contains(rerr.Message, types.ErrTxTypeNotSupported.Error()) {
		t.Fatalf("Typ 2 am Typ-Tor abgewiesen: %v", rerr)
	}
}

func TestAnnahmeTyp_NurNullEinsZwei(t *testing.T) {
	for typ, want := range map[byte]bool{
		types.LegacyTxType: true, types.AccessListTxType: true, types.DynamicFeeTxType: true,
		types.BlobTxType: false, types.SetCodeTxType: false,
	} {
		var inner types.TxData
		switch typ {
		case types.LegacyTxType:
			inner = &types.LegacyTx{}
		case types.AccessListTxType:
			inner = &types.AccessListTx{}
		case types.DynamicFeeTxType:
			inner = &types.DynamicFeeTx{}
		case types.BlobTxType:
			inner = &types.BlobTx{}
		case types.SetCodeTxType:
			inner = &types.SetCodeTx{}
		}
		if got := annahmeTypErlaubt(types.NewTx(inner)); got != want {
			t.Errorf("Typ %d: %v statt %v", typ, got, want)
		}
	}
}
