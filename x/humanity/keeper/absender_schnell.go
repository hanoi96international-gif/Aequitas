package keeper

// ABSENDER EINER ROHTRANSAKTION -- MIT DER SCHNELLEREN KURVENRECHNUNG.
//
// Gemessen am 02.10.2026 (BenchmarkAnnahmeBuendelDB, Postgres + WAL +
// Speicherkorb wie auf C1): die Wiederherstellung des Absenders ist mit
// 88 von 165 us der groesste Posten je angenommener Ueberweisung, und jeder
// Validator zahlt sie beim Nachspielen noch einmal (signierte_ueberweisung.go).
// x/humanity/secp rechnet dasselbe mit libsecp256k1 v0.6.0 rund 30 % schneller.
//
// types.Sender laesst sich die Kurvenrechnung nicht unterschieben. Deshalb
// steht hier die Logik von go-ethereum v1.13.0
// (core/types/transaction_signing.go: cancunSigner -> londonSigner ->
// eip2930Signer -> EIP155Signer -> HomesteadSigner, recoverPlain) Zeile fuer
// Zeile nachgebaut -- mit denselben Pruefungen in derselben Reihenfolge und
// denselben Fehlern. Nur crypto.Ecrecover ist ersetzt. Die Hashes rechnet
// weiter go-ethereum (Signer.Hash).
//
// Beim Update auf go-ethereum v1.17.8 bewusst beibehalten: der Knoten
// entscheidet weiter nach den Regeln von v1.13.0. Transaktionen vom Typ 4
// (EIP-7702), die v1.17 einlesen kann, lehnt er wie bisher ab
// (ErrTxTypeNotSupported); konsens_evm_golden_test.go haelt das fest.
//
// Ob das gleich bleibt, prueft absender_schnell_test.go gegen types.Sender:
// alle Transaktionsarten, fremde Chain-IDs, ungeschuetzte, hohes S, kaputte
// V-Werte, Zufallssignaturen. Weicht etwas ab, ist der Test rot -- eine
// Abweichung hiesse, dass Annahme und Nachspielen anders entscheiden als
// bisher.

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/secp"
)

// absenderKette: die Signer einer Chain-ID.
type absenderKette struct {
	id     *big.Int
	idMul  *big.Int
	signer types.Signer
	eip155 types.Signer
}

func neueAbsenderKette(id int64) *absenderKette {
	cid := big.NewInt(id)
	return &absenderKette{
		id:     cid,
		idMul:  new(big.Int).Mul(cid, big.NewInt(2)),
		signer: types.NewCancunSigner(cid),
		eip155: types.NewEIP155Signer(cid),
	}
}

var (
	// Chain-ID-Wechsel (chain_id_wechsel.go): Signer fuer beide IDs. Welche
	// zur Blockzeit GILT, entscheidet allein der Stichtag -- in
	// pruefeUeberweisungsRoh und an der Annahme, nie hier.
	absenderAlt   = neueAbsenderKette(chainIDAltWert)
	absenderNeu   = neueAbsenderKette(chainIDNeuWert)
	big8Absender  = big.NewInt(8)
	big27Absender = big.NewInt(27)
)

// absenderKetteFuer: die neue ID nur, wenn die Transaktion sie traegt; jede
// andere (1926, fremde, ungeschuetzte) laeuft wie bisher gegen 1926 -- mit
// denselben Fehlern wie bisher.
func absenderKetteFuer(tx *types.Transaction) *absenderKette {
	if istChainID(tx.ChainId(), chainIDNeuWert) {
		return absenderNeu
	}
	return absenderAlt
}

// absenderWiederherstellen: wie decodeAndRecoverSender bisher --
// types.Sender(LatestSignerForChainID(id)) und, scheitert das,
// types.Sender(NewEIP155Signer(id)), dessen Fehler dann gilt. id ist 1926
// oder, nur fuer eine damit signierte Transaktion, die neue ID.
func absenderWiederherstellen(tx *types.Transaction) (common.Address, error) {
	k := absenderKetteFuer(tx)
	a, err := k.absenderCancun(tx)
	if err == nil {
		return a, nil
	}
	return k.absenderEIP155Sig(tx)
}

// cancunSigner.Sender
func (k *absenderKette) absenderCancun(tx *types.Transaction) (common.Address, error) {
	if tx.Type() != types.BlobTxType {
		return k.absenderLondon(tx)
	}
	V, R, S := tx.RawSignatureValues()
	V = new(big.Int).Add(V, big27Absender)
	if tx.ChainId().Cmp(k.id) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), k.id)
	}
	return absenderPlain(k.signer.Hash(tx), R, S, V)
}

// londonSigner.Sender
func (k *absenderKette) absenderLondon(tx *types.Transaction) (common.Address, error) {
	if tx.Type() != types.DynamicFeeTxType {
		return k.absenderEIP2930(tx)
	}
	V, R, S := tx.RawSignatureValues()
	V = new(big.Int).Add(V, big27Absender)
	if tx.ChainId().Cmp(k.id) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), k.id)
	}
	return absenderPlain(k.signer.Hash(tx), R, S, V)
}

// eip2930Signer.Sender
func (k *absenderKette) absenderEIP2930(tx *types.Transaction) (common.Address, error) {
	V, R, S := tx.RawSignatureValues()
	switch tx.Type() {
	case types.LegacyTxType:
		return k.absenderEIP155Sig(tx)
	case types.AccessListTxType:
		V = new(big.Int).Add(V, big27Absender)
	default:
		return common.Address{}, types.ErrTxTypeNotSupported
	}
	if tx.ChainId().Cmp(k.id) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), k.id)
	}
	return absenderPlain(k.signer.Hash(tx), R, S, V)
}

// EIP155Signer.Sender
func (k *absenderKette) absenderEIP155Sig(tx *types.Transaction) (common.Address, error) {
	if tx.Type() != types.LegacyTxType {
		return common.Address{}, types.ErrTxTypeNotSupported
	}
	if !tx.Protected() {
		return absenderHomestead(tx)
	}
	if tx.ChainId().Cmp(k.id) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), k.id)
	}
	V, R, S := tx.RawSignatureValues()
	V = new(big.Int).Sub(V, k.idMul)
	V.Sub(V, big8Absender)
	return absenderPlain(k.eip155.Hash(tx), R, S, V)
}

// HomesteadSigner.Sender
func absenderHomestead(tx *types.Transaction) (common.Address, error) {
	if tx.Type() != types.LegacyTxType {
		return common.Address{}, types.ErrTxTypeNotSupported
	}
	v, r, s := tx.RawSignatureValues()
	return absenderPlain(types.HomesteadSigner{}.Hash(tx), r, s, v)
}

// recoverPlain(sighash, R, S, Vb, homestead=true), Kurve aus secp.
func absenderPlain(sighash common.Hash, R, S, Vb *big.Int) (common.Address, error) {
	if Vb.BitLen() > 8 {
		return common.Address{}, types.ErrInvalidSig
	}
	V := byte(Vb.Uint64() - 27)
	if !crypto.ValidateSignatureValues(V, R, S, true) {
		return common.Address{}, types.ErrInvalidSig
	}
	r, s := R.Bytes(), S.Bytes()
	sig := make([]byte, crypto.SignatureLength)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = V
	pub, err := secp.Wiederherstellen(sighash[:], sig)
	if err != nil {
		return common.Address{}, err
	}
	if len(pub) == 0 || pub[0] != 4 {
		return common.Address{}, errors.New("invalid public key")
	}
	var addr common.Address
	copy(addr[:], crypto.Keccak256(pub[1:])[12:])
	return addr, nil
}
