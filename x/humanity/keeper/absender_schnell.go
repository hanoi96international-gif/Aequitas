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

var (
	absenderChainID    = big.NewInt(1926)
	absenderChainIDMul = new(big.Int).Mul(absenderChainID, big.NewInt(2))
	absenderSigner     = types.NewCancunSigner(absenderChainID)
	absenderEIP155     = types.NewEIP155Signer(absenderChainID)
	big8Absender       = big.NewInt(8)
	big27Absender      = big.NewInt(27)
)

// absenderWiederherstellen: wie decodeAndRecoverSender bisher --
// types.Sender(LatestSignerForChainID(1926)) und, scheitert das,
// types.Sender(NewEIP155Signer(1926)), dessen Fehler dann gilt.
func absenderWiederherstellen(tx *types.Transaction) (common.Address, error) {
	a, err := absenderCancun(tx)
	if err == nil {
		return a, nil
	}
	return absenderEIP155Sig(tx)
}

// cancunSigner.Sender
func absenderCancun(tx *types.Transaction) (common.Address, error) {
	if tx.Type() != types.BlobTxType {
		return absenderLondon(tx)
	}
	V, R, S := tx.RawSignatureValues()
	V = new(big.Int).Add(V, big27Absender)
	if tx.ChainId().Cmp(absenderChainID) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), absenderChainID)
	}
	return absenderPlain(absenderSigner.Hash(tx), R, S, V)
}

// londonSigner.Sender
func absenderLondon(tx *types.Transaction) (common.Address, error) {
	if tx.Type() != types.DynamicFeeTxType {
		return absenderEIP2930(tx)
	}
	V, R, S := tx.RawSignatureValues()
	V = new(big.Int).Add(V, big27Absender)
	if tx.ChainId().Cmp(absenderChainID) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), absenderChainID)
	}
	return absenderPlain(absenderSigner.Hash(tx), R, S, V)
}

// eip2930Signer.Sender
func absenderEIP2930(tx *types.Transaction) (common.Address, error) {
	V, R, S := tx.RawSignatureValues()
	switch tx.Type() {
	case types.LegacyTxType:
		return absenderEIP155Sig(tx)
	case types.AccessListTxType:
		V = new(big.Int).Add(V, big27Absender)
	default:
		return common.Address{}, types.ErrTxTypeNotSupported
	}
	if tx.ChainId().Cmp(absenderChainID) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), absenderChainID)
	}
	return absenderPlain(absenderSigner.Hash(tx), R, S, V)
}

// EIP155Signer.Sender
func absenderEIP155Sig(tx *types.Transaction) (common.Address, error) {
	if tx.Type() != types.LegacyTxType {
		return common.Address{}, types.ErrTxTypeNotSupported
	}
	if !tx.Protected() {
		return absenderHomestead(tx)
	}
	if tx.ChainId().Cmp(absenderChainID) != 0 {
		return common.Address{}, fmt.Errorf("%w: have %d want %d", types.ErrInvalidChainId, tx.ChainId(), absenderChainID)
	}
	V, R, S := tx.RawSignatureValues()
	V = new(big.Int).Sub(V, absenderChainIDMul)
	V.Sub(V, big8Absender)
	return absenderPlain(absenderEIP155.Hash(tx), R, S, V)
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
