package keeper

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Single source of truth for all currently-active contract addresses on the
// Aequitas Chain. Every other file (main.go, api.go, api_html.go's frontend,
// register.go) should reference these constants instead of hardcoding the
// address again — hardcoding is what caused three different "V6"/"V7"
// addresses and two different bio-verifier addresses to drift apart in the
// codebase and the user-facing explorer at the same time.
const V6_CONTRACT_ADDR = "0xA76cA3bf34F2Ae5dFA0608696627e42b81180488"
const V7_CONTRACT_ADDR = "0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78"
const BIO_VERIFIER_ADDR = "0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2"
const V5_SEPOLIA_LEGACY_ADDR = "0x4f147d5B3388AF07993CC4fC548502A78Af0B8b5" // Sepolia testnet — historical only, no longer in active use

// MirrorV6Registration mirrors a V6 registration to PostgreSQL.
//
// FIX (P3-m, audit 2026-07-06): this has 0 call sites anywhere in the
// codebase — new V6 registrations are fully disabled (V7 is the only live
// registration path; see V7_CONTRACT_ADDR above and register.go). Kept,
// not deleted, only because RestoreV6FromMirror below still legitimately
// runs on every boot — but that function REPLAYS rows a past call to THIS
// one already wrote to PostgreSQL back when V6 registration was still
// live; it does not depend on this function ever running again. Don't
// read RestoreV6FromMirror's continued activity as evidence this one is
// too — it isn't.
func (e *EVMEngine) MirrorV6Registration(wallet, commitment string) {
	e.chainState.SaveV6Human(wallet, commitment)
	e.chainState.SaveV6Commitment(commitment, wallet)

	grant := new(big.Int).Mul(big.NewInt(1000), weiPerAEQ)
	e.chainState.SaveV6Balance(wallet, hex.EncodeToString(grant.Bytes()))

	humans := e.chainState.GetAllV6Humans()
	e.chainState.SaveV6State("totalHumans", fmt.Sprintf("%x", len(humans)))

	fmt.Printf("[V6] Mirrored registration: %s\n", wallet)
}

// RestoreV6FromMirror restores V6 EVM state from PostgreSQL to evm_storage table
// so that CallContract can read it via newStateDB()
func (e *EVMEngine) RestoreV6FromMirror() {
	contractAddr := common.HexToAddress(V6_CONTRACT_ADDR)
	addrStr := strings.ToLower(contractAddr.Hex())

	humans := e.chainState.GetAllV6Humans()
	if len(humans) == 0 {
		return
	}

	fmt.Printf("[V6] Restoring %d registrations to storage...\n", len(humans))

	for _, human := range humans {
		walletAddr := common.HexToAddress(human["address"])
		commitment := human["commitment"]

		// isHuman[wallet] = true (slot 3)
		isHumanSlot := mappingSlot(walletAddr.Bytes(), 3)
		e.chainState.SaveStorageSlot(addrStr, isHumanSlot.Hex(), common.HexToHash("0x01").Hex())

		// commitmentOf[wallet] (slot 14)
		if commitment != "" {
			commitmentSlot := mappingSlot(walletAddr.Bytes(), 14)
			commitBig := new(big.Int)
			commitBig.SetString(commitment, 16)
			e.chainState.SaveStorageSlot(addrStr, commitmentSlot.Hex(), common.BigToHash(commitBig).Hex())

			// usedCommitments[commitment] = true (slot 2)
			commitHash := common.BigToHash(commitBig)
			usedSlot := mappingSlotBytes32(commitHash, 2)
			e.chainState.SaveStorageSlot(addrStr, usedSlot.Hex(), common.HexToHash("0x01").Hex())
		}

		// balanceOf[wallet] (slot 1)
		balWeiHex := e.chainState.LoadV6Balance(human["address"])
		if balWeiHex != "" {
			balBig := new(big.Int)
			balBig.SetString(balWeiHex, 16)
			balSlot := mappingSlot(walletAddr.Bytes(), 1)
			e.chainState.SaveStorageSlot(addrStr, balSlot.Hex(), common.BigToHash(balBig).Hex())
		}
	}

	// totalHumans (slot 9)
	totalHumansHex := e.chainState.LoadV6State("totalHumans")
	if totalHumansHex != "" {
		n := new(big.Int)
		n.SetString(totalHumansHex, 16)
		slot9 := common.BigToHash(big.NewInt(9))
		e.chainState.SaveStorageSlot(addrStr, slot9.Hex(), common.BigToHash(n).Hex())
	}

	fmt.Printf("[V6] ✓ Storage restored for %d humans\n", len(humans))
}

func mappingSlot(key []byte, slot int64) common.Hash {
	// FIX 14: Guard against keys longer than 32 bytes — keep the rightmost 32
	// bytes (matching Solidity's left-padding semantics for address/uint keys).
	if len(key) > 32 {
		key = key[len(key)-32:]
	}
	paddedKey := make([]byte, 32)
	copy(paddedKey[32-len(key):], key)
	slotBytes := common.BigToHash(big.NewInt(slot)).Bytes()
	// FIX 14: Use a fresh allocation to avoid aliasing when paddedKey or
	// slotBytes share an underlying array with other variables.
	data := make([]byte, 64)
	copy(data[:32], paddedKey)
	copy(data[32:], slotBytes)
	return common.BytesToHash(crypto.Keccak256(data))
}

func mappingSlotBytes32(key common.Hash, slot int64) common.Hash {
	slotBytes := common.BigToHash(big.NewInt(slot)).Bytes()
	// FIX 14: Use a fresh allocation instead of append to avoid aliasing — if
	// key.Bytes() returns a slice backed by key's array, appending to it could
	// corrupt the original key value in callers that reuse the hash.
	data := make([]byte, 64)
	copy(data[:32], key.Bytes())
	copy(data[32:], slotBytes)
	return common.BytesToHash(crypto.Keccak256(data))
}
