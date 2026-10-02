package keeper

import (
	"crypto/ecdsa"
	"math/big"
	mrand "math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

// absenderBisher: genau das, was decodeAndRecoverSender bis zum 02.10.2026
// tat. Referenz fuer absenderWiederherstellen.
func absenderBisher(t *types.Transaction) (common.Address, error) {
	a, err := types.Sender(types.LatestSignerForChainID(big.NewInt(1926)), t)
	if err != nil {
		a, err = types.Sender(types.NewEIP155Signer(big.NewInt(1926)), t)
	}
	return a, err
}

// vergleicheAbsender meldet, ob ein Absender herauskam (fuer die Zaehlung,
// dass der Test auch Erfolge prueft und nicht nur Fehler).
func vergleicheAbsender(t *testing.T, name string, neu func() *types.Transaction) bool {
	t.Helper()
	// Jeweils ein frisches Objekt: types.Sender merkt sich den Absender in
	// der Transaktion.
	altA, altErr := absenderBisher(neu())
	neuA, neuErr := absenderWiederherstellen(neu())
	if (altErr == nil) != (neuErr == nil) || altA != neuA {
		t.Fatalf("%s: bisher (%s, %v), jetzt (%s, %v)", name, altA.Hex(), altErr, neuA.Hex(), neuErr)
	}
	if altErr != nil && altErr.Error() != neuErr.Error() {
		t.Fatalf("%s: Fehlertext bisher %q, jetzt %q", name, altErr, neuErr)
	}
	return altErr == nil
}

type absenderFall struct {
	name string
	tx   func() *types.Transaction
}

// Gutfall und Missbrauch: jede Transaktionsart, richtig und falsch signiert.
func TestAbsenderWiederherstellen_WieTypesSender(t *testing.T) {
	r := mrand.New(mrand.NewSource(1))
	an := common.HexToAddress(testRecipientHex)
	schl := make([]*ecdsa.PrivateKey, 8)
	for i := range schl {
		schl[i], _ = crypto.GenerateKey()
	}
	var faelle []absenderFall
	signiert := func(name string, inner types.TxData, signer types.Signer, k *ecdsa.PrivateKey) {
		tx, err := types.SignNewTx(k, signer, inner)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		raw, _ := tx.MarshalBinary()
		faelle = append(faelle, absenderFall{name, func() *types.Transaction {
			x := new(types.Transaction)
			if err := x.UnmarshalBinary(raw); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return x
		}})
	}
	for _, kette := range []int64{1926, 1, 1925, 3852} {
		cid := big.NewInt(kette)
		for i, k := range schl {
			n := uint64(i)
			signiert("legacy-eip155", &types.LegacyTx{Nonce: n, To: &an, Value: big.NewInt(1e15), Gas: 21000, GasPrice: big.NewInt(0)}, types.NewEIP155Signer(cid), k)
			signiert("accesslist", &types.AccessListTx{ChainID: cid, Nonce: n, To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(1)}, types.NewCancunSigner(cid), k)
			signiert("dynamicfee", &types.DynamicFeeTx{ChainID: cid, Nonce: n, To: &an, Value: big.NewInt(1), Gas: 21000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2)}, types.NewCancunSigner(cid), k)
			signiert("blob", &types.BlobTx{ChainID: uint256.NewInt(uint64(kette)), Nonce: n, To: an, Value: uint256.NewInt(1), Gas: 21000, GasTipCap: uint256.NewInt(1), GasFeeCap: uint256.NewInt(2), BlobFeeCap: uint256.NewInt(1), BlobHashes: []common.Hash{{1}}}, types.NewCancunSigner(cid), k)
		}
	}
	for i, k := range schl {
		signiert("legacy-ungeschuetzt", &types.LegacyTx{Nonce: uint64(i), To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(0)}, types.HomesteadSigner{}, k)
	}
	erfolge := 0
	for _, f := range faelle {
		if vergleicheAbsender(t, f.name, f.tx) {
			erfolge++
		}
	}
	// Richtig signiert und Kette 1926: 8 Schluessel x 4 Arten, dazu 8
	// ungeschuetzte. Alles andere (fremde Ketten) muss scheitern.
	if erfolge != 8*4+8 {
		t.Fatalf("%d echte Absender, erwartet %d", erfolge, 8*4+8)
	}

	// Frei gewaehlte V, R, S: kaputte Recovery-IDs, R/S an und ueber den
	// Grenzen, hohes S, Zufall.
	n := crypto.S256().Params().N
	halb := new(big.Int).Rsh(n, 1)
	werteRS := []*big.Int{
		big.NewInt(0), big.NewInt(1), halb, new(big.Int).Add(halb, big.NewInt(1)),
		new(big.Int).Sub(n, big.NewInt(1)), n, new(big.Int).Add(n, big.NewInt(1)),
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)),
	}
	zufallRS := func() *big.Int {
		b := make([]byte, 32)
		r.Read(b)
		return new(big.Int).SetBytes(b)
	}
	werteV := []*big.Int{
		big.NewInt(0), big.NewInt(1), big.NewInt(2), big.NewInt(26), big.NewInt(27), big.NewInt(28), big.NewInt(29),
		big.NewInt(35), big.NewInt(36), big.NewInt(3886), big.NewInt(3887), big.NewInt(3888), big.NewInt(3889),
		big.NewInt(255), big.NewInt(256), new(big.Int).Lsh(big.NewInt(1), 64), new(big.Int).Lsh(big.NewInt(1), 200),
	}
	// Zu einer echten Signatur: Hash-gebunden, damit die Kurve auch Erfolge
	// liefert und nicht nur Fehler.
	k := schl[0]
	echtLegacy, _ := types.SignNewTx(k, types.NewEIP155Signer(big.NewInt(1926)), &types.LegacyTx{To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(0)})
	_, echtR, echtS := echtLegacy.RawSignatureValues()
	werteRS = append(werteRS, echtR, echtS, new(big.Int).Sub(n, echtS))

	freiErfolge, freiFehler := 0, 0
	// Je Art die V-Werte, die die Pruefung bis zur Kurve passieren -- sonst
	// endet fast jeder Fall schon vor der Wiederherstellung.
	gueltigesV := [][]*big.Int{
		{big.NewInt(27), big.NewInt(28), big.NewInt(3887), big.NewInt(3888)},
		{big.NewInt(0), big.NewInt(1)}, {big.NewInt(0), big.NewInt(1)}, {big.NewInt(0), big.NewInt(1)},
	}
	for runde := 0; runde < 2000; runde++ {
		var R, S *big.Int
		if runde%4 == 0 {
			R, S = zufallRS(), zufallRS()
		} else {
			R, S = werteRS[r.Intn(len(werteRS))], werteRS[r.Intn(len(werteRS))]
		}
		for art := 0; art < 4; art++ {
			V := werteV[r.Intn(len(werteV))]
			if r.Intn(10) < 7 {
				V = gueltigesV[art][r.Intn(len(gueltigesV[art]))]
			}
			R, S, V := new(big.Int).Set(R), new(big.Int).Set(S), new(big.Int).Set(V)
			var inner func() types.TxData
			switch art {
			case 0:
				inner = func() types.TxData {
					return &types.LegacyTx{To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(0), V: new(big.Int).Set(V), R: new(big.Int).Set(R), S: new(big.Int).Set(S)}
				}
			case 1:
				inner = func() types.TxData {
					return &types.AccessListTx{ChainID: big.NewInt(1926), To: &an, Value: big.NewInt(1), Gas: 21000, GasPrice: big.NewInt(1), V: new(big.Int).Set(V), R: new(big.Int).Set(R), S: new(big.Int).Set(S)}
				}
			case 2:
				inner = func() types.TxData {
					return &types.DynamicFeeTx{ChainID: big.NewInt(1926), To: &an, Value: big.NewInt(1), Gas: 21000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), V: new(big.Int).Set(V), R: new(big.Int).Set(R), S: new(big.Int).Set(S)}
				}
			case 3:
				if V.BitLen() > 256 || R.BitLen() > 256 || S.BitLen() > 256 {
					continue
				}
				inner = func() types.TxData {
					v, _ := uint256.FromBig(V)
					rr, _ := uint256.FromBig(R)
					ss, _ := uint256.FromBig(S)
					return &types.BlobTx{ChainID: uint256.NewInt(1926), To: an, Value: uint256.NewInt(1), Gas: 21000, GasTipCap: uint256.NewInt(1), GasFeeCap: uint256.NewInt(2), BlobFeeCap: uint256.NewInt(1), BlobHashes: []common.Hash{{1}}, V: v, R: rr, S: ss}
				}
			}
			if vergleicheAbsender(t, "frei", func() *types.Transaction { return types.NewTx(inner()) }) {
				freiErfolge++
			} else {
				freiFehler++
			}
		}
	}
	// Freie Werte ergeben oft einen (falschen, aber gueltigen) Punkt -- beide
	// Ausgaenge muessen vorkommen, sonst prueft die Schleife nur eine Seite.
	if freiErfolge < 500 || freiFehler < 500 {
		t.Fatalf("frei: %d Erfolge, %d Fehler -- zu einseitig", freiErfolge, freiFehler)
	}
	t.Logf("frei: %d Erfolge, %d Fehler", freiErfolge, freiFehler)
}

// Ueber den ganzen Weg: decodeAndRecoverSender liefert fuer eine echte
// Ueberweisung denselben Absender wie bisher.
func TestDecodeAndRecoverSender_NutztSchnellenWeg(t *testing.T) {
	raw, want := signedRawHex(t, 3, testRecipientHex)
	tx, got, _, err := decodeAndRecoverSender(raw)
	if err != nil || got != want {
		t.Fatalf("Absender %s, erwartet %s (%v)", got, want, err)
	}
	alt, err := absenderBisher(tx)
	if err != nil || adresseKlein(alt) != want {
		t.Fatalf("Referenz: %s, %v", alt.Hex(), err)
	}
}
