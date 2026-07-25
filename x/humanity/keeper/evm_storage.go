package keeper

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/lib/pq"
)

// round6 rounds a float64 to 6 decimal places, eliminating floating-point
// accumulation errors that build up in ledger operations over many transactions.
// This is the application-level fix for float64 imprecision; a full integer
// refactor (microAEQ int64) remains a future architecture task.
func round6(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}

// ─── CONTRACT STORAGE ─────────────────────────────────────────────────────────

func (cs *ChainState) SaveContract(address string, bytecode []byte, deployer string) error {
	if cs.db == nil {
		return nil
	}
	address = strings.ToLower(address)
	_, err := cs.db.Exec(
		`INSERT INTO evm_contracts (address, bytecode, deployer) VALUES ($1, $2, $3)
 ON CONFLICT (address) DO UPDATE SET bytecode = $2`,
		address, hex.EncodeToString(bytecode), deployer,
	)
	if err != nil {
		fmt.Printf("[EVM] Error saving contract: %v\n", err)
	}
	return err
}

func (cs *ChainState) LoadContract(address string) ([]byte, error) {
	if cs.db == nil {
		return nil, nil
	}
	address = strings.ToLower(address)
	var bytecodeHex string
	err := cs.db.QueryRow(
		`SELECT bytecode FROM evm_contracts WHERE lower(address) = $1`, address,
	).Scan(&bytecodeHex)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	bytecodeHex = strings.TrimPrefix(strings.TrimPrefix(bytecodeHex, `\x`), "0x")
	return hex.DecodeString(bytecodeHex)
}

func (cs *ChainState) GetAllContracts() []string {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(`SELECT address FROM evm_contracts`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var addr string
		rows.Scan(&addr)
		addrs = append(addrs, addr)
	}
	return addrs
}

// ─── NONCE STORAGE ────────────────────────────────────────────────────────────

func (cs *ChainState) SaveNonce(address string, nonce uint64) error {
	if cs.db == nil {
		return nil
	}
	address = strings.ToLower(address)
	// Compare-and-swap: only advance the nonce, never decrease it.
	// Two nodes racing to reserve the same nonce would both issue
	// INSERT … nonce=$2; the second node's UPDATE fires but the
	// WHERE nonce < $2 clause rejects it, so the DB always holds
	// the highest reserved nonce.
	_, err := cs.db.Exec(
		`INSERT INTO evm_nonces (address, nonce) VALUES ($1, $2)
 ON CONFLICT (address) DO UPDATE SET nonce = $2 WHERE evm_nonces.nonce < $2`,
		address, nonce,
	)
	return err
}

// ReserveNonce atomically advances address from expected to next.
// It returns false when another process/node already reserved the same nonce.
func (cs *ChainState) ReserveNonce(address string, expected, next uint64) (bool, error) {
	if cs.db == nil {
		return true, nil
	}
	address = strings.ToLower(address)
	if expected == 0 {
		res, err := cs.db.Exec(
			`INSERT INTO evm_nonces (address, nonce) VALUES ($1, $2)
 ON CONFLICT (address) DO NOTHING`,
			address, next,
		)
		if err != nil {
			return false, err
		}
		if rows, _ := res.RowsAffected(); rows == 1 {
			return true, nil
		}
	}
	res, err := cs.db.Exec(
		`UPDATE evm_nonces SET nonce = $3 WHERE lower(address) = $1 AND nonce = $2`,
		address, expected, next,
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (cs *ChainState) LoadNonce(address string) uint64 {
	if cs.db == nil {
		return 0
	}
	address = strings.ToLower(address)
	var nonce uint64
	cs.db.QueryRow(`SELECT nonce FROM evm_nonces WHERE lower(address) = $1`, address).Scan(&nonce)
	return nonce
}

// ─── CONTRACT STORAGE SLOTS ───────────────────────────────────────────────────

// SaveStorageSlot writes via cs.db directly — safe to call without holding
// cs.mu (used by EVM contract execution, V6 mirror, contract deploy, and
// MigrateEVMFromGoState, none of which run inside a cs.mu-held critical
// section). For callers that DO already hold cs.mu inside an atomic
// Go-state operation and need this write to join that operation's
// cs.activeTx, use saveStorageSlotLocked instead — see its own comment for
// why these can't simply be the same function (audit 2026-06-28
// Gesamtaudit, P0-1: reading cs.activeTx without holding cs.mu first is a
// real data race against any concurrent cs.mu-locked operation, the same
// class of bug already fixed for getConfigValue/tryClaimNullifierLocked
// elsewhere this session).
func (cs *ChainState) SaveStorageSlot(address, slot, value string) error {
	if cs.db == nil {
		return nil
	}
	address = strings.ToLower(address)
	_, err := cs.db.Exec(
		`INSERT INTO evm_storage (address, slot, value) VALUES ($1, $2, $3)
 ON CONFLICT (address, slot) DO UPDATE SET value = $3`,
		address, slot, value,
	)
	return err
}

// SaveStorageSlots is SaveStorageSlot's batch counterpart for many slots
// under the SAME contract address: one multi-row INSERT ... ON CONFLICT
// instead of one round trip per slot.
//
// FIX (performance audit 2026-07-06): dumpAndPersistStorage (evm_engine.go)
// used to call SaveStorageSlot individually up to ~40 times per persisting
// EVM call — every registration, every intercepted V7 transfer. Same
// "writes via cs.db directly, safe to call without holding cs.mu" contract
// as SaveStorageSlot itself (see its own comment) — EVMEngine's callers
// here don't hold cs.mu, so this must never try to join cs.activeTx the way
// saveStorageSlotLocked does.
func (cs *ChainState) SaveStorageSlots(address string, slots map[string]string) error {
	if cs.db == nil || len(slots) == 0 {
		return nil
	}
	address = strings.ToLower(address)
	// See saveAccountsToDBBatchCtx's FIX comment (state.go) for why this
	// builds a fixed-size unnest() query over 2 array parameters instead
	// of a VALUES(...) list whose text size grows with len(slots) — this
	// runs on every registration and every intercepted V7 transfer (see
	// this function's own doc comment), not just a background flush.
	// address is a single scalar $1, not a third array, since every row
	// this call writes shares the same address by construction.
	slotNames := make([]string, 0, len(slots))
	slotValues := make([]string, 0, len(slots))
	for slot, value := range slots {
		slotNames = append(slotNames, slot)
		slotValues = append(slotValues, value)
	}
	query := `INSERT INTO evm_storage (address, slot, value)
SELECT $1, s, v FROM unnest($2::text[], $3::text[]) AS t(s, v)
ON CONFLICT (address, slot) DO UPDATE SET value = EXCLUDED.value`
	_, err := cs.db.Exec(query, address, pq.Array(slotNames), pq.Array(slotValues))
	return err
}

// saveStorageSlotLocked is SaveStorageSlot's body for callers that already
// hold cs.mu inside an atomic Go-state operation (e.g. syncBalanceLocked,
// itself only ever called while cs.mu is held — see its own doc comment).
// Routes through cs.dbExec() so this write joins cs.activeTx when one is
// set, making the EVM mirror slot commit or roll back together with the
// Go-state mutation it derives from, instead of auto-committing
// independently a moment before a later step in the same operation fails.
//
// FIX (audit 2026-06-28 Gesamtaudit, P0-1): this is exactly the gap the
// audit traced through registerHumanLocked → syncHumanRegistrationLocked →
// syncBalanceLocked → SaveStorageSlot: balanceOf/isHuman could commit to
// evm_storage immediately, then a LATER step in the same registration
// (SaveNullifier, the outbox insert, or the final tx.Commit) could fail and
// roll back Go-state and the outbox — while evm_storage stayed on
// "isHuman=true" regardless, since SaveStorageSlot's plain cs.db.Exec had
// already committed it on a separate connection. eth_call/V7 dry-runs/
// wallet RPC reads from evm_storage, so this could surface as "already
// registered" against a wallet whose actual registration never completed.
// saveStorageSlotLocked is the context.Background()-calling wrapper kept for
// callers not yet migrated to thread ctx explicitly — see dbExecCtx's
// comment for the migration this is part of.
func (cs *ChainState) saveStorageSlotLocked(address, slot, value string) error {
	return cs.saveStorageSlotLockedCtx(context.Background(), address, slot, value)
}

func (cs *ChainState) saveStorageSlotLockedCtx(ctx context.Context, address, slot, value string) error {
	if cs.db == nil {
		return nil
	}
	address = strings.ToLower(address)
	_, err := cs.dbExecCtx(ctx).Exec(
		`INSERT INTO evm_storage (address, slot, value) VALUES ($1, $2, $3)
 ON CONFLICT (address, slot) DO UPDATE SET value = $3`,
		address, slot, value,
	)
	return err
}

// LoadAllStorageSlots returns every slot stored for address, used to back up
// contract state before a destructive upgrade so it can be restored on failure.
func (cs *ChainState) LoadAllStorageSlots(address string) (map[string]string, error) {
	if cs.db == nil {
		return nil, nil
	}
	address = strings.ToLower(address)
	rows, err := cs.db.Query(`SELECT slot, value FROM evm_storage WHERE lower(address) = $1`, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var slot, value string
		if err := rows.Scan(&slot, &value); err == nil {
			out[slot] = value
		}
	}
	return out, nil
}

func (cs *ChainState) LoadStorageSlot(address, slot string) (string, error) {
	if cs.db == nil {
		return "", nil
	}
	address = strings.ToLower(address)
	var value string
	err := cs.db.QueryRow(
		`SELECT value FROM evm_storage WHERE lower(address) = $1 AND slot = $2`,
		address, slot,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// ─── DUAL-LEDGER SYNC ────────────────────────────────────────────────────────

// MigrateEVMFromGoState rebuilds all V7 contract storage slots from the
// authoritative Go-state and database after an evm_storage wipe (e.g. on
// contract upgrade). Writes: totalSupply (slot 0), totalHumans (slot 1),
// balanceOf (slot 4), isHuman (slot 6), usedCommitments (slot 7),
// usedNullifiers (slot 8). Safe to call without holding cs.mu.
func (cs *ChainState) MigrateEVMFromGoState(contractAddr string) error {
	if cs.db == nil {
		return nil
	}
	contractAddr = strings.ToLower(contractAddr)
	fmt.Printf("[MIGRATE] Rebuilding EVM storage from Go-state for %s...\n", contractAddr)

	// FIX: every SaveStorageSlot call below used to discard its error. A
	// transient DB blip mid-migration would silently leave some accounts'
	// balance/isHuman/lastActivity slots written and others not, producing
	// a partially-migrated, inconsistent EVM mirror with no signal to the
	// caller — discovered later only when users report wrong balances or
	// registration status. Track the first failure and how many occurred so
	// the function can return a real error and the caller's existing
	// rollback logic (contract_deploy.go's restoreOnFailure) actually fires
	// instead of treating an incomplete migration as a success.
	var firstErr error
	failCount := 0
	save := func(addr, slot, value string) {
		if err := cs.SaveStorageSlot(addr, slot, value); err != nil {
			failCount++
			if firstErr == nil {
				firstErr = err
			}
			fmt.Printf("[MIGRATE] ERROR: SaveStorageSlot(%s, %s) failed: %v\n", addr, slot, err)
		}
	}

	var totalSupply float64
	var totalHumans int64

	cs.mu.RLock()
	cs.accounts.Range(func(addr string, acc *AccountState) bool {
		balBig := aeqToWei(acc.Balance.Float())
		addrBytes := common.HexToAddress(addr).Bytes()
		save(contractAddr, mappingSlot(addrBytes, 4).Hex(), common.BigToHash(balBig).Hex())
		totalSupply += acc.Balance.Float()
		if acc.IsHuman {
			save(contractAddr, mappingSlot(addrBytes, 6).Hex(), common.HexToHash("0x01").Hex())
			totalHumans++
			// Preserve lastActivity (slot 10) and lastDemurrage (slot 11).
			if acc.LastActivityAt > 0 {
				ts := big.NewInt(acc.LastActivityAt)
				save(contractAddr, mappingSlot(addrBytes, 10).Hex(), common.BigToHash(ts).Hex())
				save(contractAddr, mappingSlot(addrBytes, 11).Hex(), common.BigToHash(ts).Hex())
			}
			// Set ubiClaimed (slot 12) to the CURRENT ubiPerHumanAccumulated (slot 3).
			// This prevents double-claiming: after an upgrade, each human's "already claimed"
			// marker is set to the current accumulator so they can't re-claim historical UBI.
			// They can still earn new UBI from future distributions.
			// ubiPerHumanAccumulated (slot 3) will be read from EVM storage below.
			// We store a marker here; the actual slot-3 value is written after the loop.
		}
		return true
	})
	cs.mu.RUnlock()

	// totalSupply (slot 0) and totalHumans (slot 1)
	supplyWei := aeqToWei(totalSupply)
	save(contractAddr, common.BigToHash(big.NewInt(0)).Hex(), common.BigToHash(supplyWei).Hex())
	save(contractAddr, common.BigToHash(big.NewInt(1)).Hex(), common.BigToHash(big.NewInt(totalHumans)).Hex())

	// Read current ubiPerHumanAccumulated (slot 3) from DB so we can set
	// ubiClaimed = that value for every human, preventing double-claim on upgrade.
	ubiAccumSlot := common.BigToHash(big.NewInt(3)).Hex()
	ubiAccumVal, _ := cs.LoadStorageSlot(contractAddr, ubiAccumSlot)
	if ubiAccumVal == "" {
		ubiAccumVal = common.Hash{}.Hex()
	}
	// Also write slot 2 (ubiPool) and slot 3 (ubiPerHumanAccumulated) — preserve existing
	// slot 3 value; it is NOT part of Go-state so we keep what was last in EVM.

	// Set ubiClaimed (slot 12) = ubiPerHumanAccumulated for every human to prevent double-claiming.
	cs.mu.RLock()
	cs.accounts.Range(func(addr string, acc *AccountState) bool {
		if acc.IsHuman {
			addrB := common.HexToAddress(addr).Bytes()
			save(contractAddr, mappingSlot(addrB, 12).Hex(), ubiAccumVal)
		}
		return true
	})
	cs.mu.RUnlock()

	// usedNullifiers (slot 8): nullifier → wallet
	rows, err := cs.db.Query(`SELECT nullifier, wallet_address FROM nullifiers`)
	if err == nil {
		// P2-FIX: explicit Close instead of defer — defer fires at function
		// return, keeping both DB cursors open simultaneously during migration.
		for rows.Next() {
			var nullifier, wallet string
			if scanErr := rows.Scan(&nullifier, &wallet); scanErr != nil {
				fmt.Printf("[EVM] Warning: nullifier scan error in MigrateEVM: %v\n", scanErr)
				continue
			}
			// FIX (P0-02): public snapshots store nullifiers with wallet_address = ''.
			// common.HexToAddress("") is the zero address, which the V7 contract
			// interprets as "nullifier not used" — allowing double-registration.
			// Use a non-zero sentinel instead so the slot is marked occupied.
			if wallet == "" {
				wallet = "0x0000000000000000000000000000000000000001"
			}
			nullKey := common.HexToHash(strings.TrimPrefix(nullifier, "0x"))
			nullSlot := mappingSlotBytes32(nullKey, 8)
			walletHash := common.BigToHash(common.HexToAddress(wallet).Big())
			save(contractAddr, nullSlot.Hex(), walletHash.Hex())
		}
		rows.Close()
	} else {
		failCount++
		if firstErr == nil {
			firstErr = err
		}
		fmt.Printf("[MIGRATE] ERROR: nullifiers query failed: %v\n", err)
	}

	// usedCommitments (slot 7) + commitmentOf (slot 9): from bio_registrations
	rows2, err2 := cs.db.Query(`SELECT commitment, wallet_address FROM bio_registrations`)
	if err2 == nil {
		for rows2.Next() {
			var commitment, wallet string
			if err := rows2.Scan(&commitment, &wallet); err != nil {
				fmt.Printf("[MIGRATE] WARNING: bio_registrations scan error: %v — skipping row\n", err)
				continue
			}
			commitBig, ok := new(big.Int).SetString(strings.TrimPrefix(commitment, "0x"), 10)
			if !ok {
				commitBig, ok = new(big.Int).SetString(strings.TrimPrefix(commitment, "0x"), 16)
			}
			if !ok || commitBig == nil {
				continue
			}
			// usedCommitments[commitment] = true (slot 7)
			commitSlot7 := mappingSlot(common.LeftPadBytes(commitBig.Bytes(), 32), 7)
			save(contractAddr, commitSlot7.Hex(), common.HexToHash("0x01").Hex())
			// commitmentOf[wallet] = commitment (slot 9)
			if wallet != "" {
				commitSlot9 := mappingSlot(common.HexToAddress(wallet).Bytes(), 9)
				save(contractAddr, commitSlot9.Hex(), common.BigToHash(commitBig).Hex())
			}
		}
		rows2.Close()
	} else {
		failCount++
		if firstErr == nil {
			firstErr = err2
		}
		fmt.Printf("[MIGRATE] ERROR: bio_registrations query failed: %v\n", err2)
	}

	// lastActivity (slot 10) + lastDemurrage (slot 11): from chain_accounts
	cs.mu.RLock()
	cs.accounts.Range(func(addr string, acc *AccountState) bool {
		if acc.LastActivityAt == 0 {
			return true
		}
		ts := big.NewInt(acc.LastActivityAt)
		addrBytes := common.HexToAddress(addr).Bytes()
		save(contractAddr, mappingSlot(addrBytes, 10).Hex(), common.BigToHash(ts).Hex())
		save(contractAddr, mappingSlot(addrBytes, 11).Hex(), common.BigToHash(ts).Hex())
		return true
	})
	cs.mu.RUnlock()

	// Restore guardian/escrow relationship slots (5, 13-16) that were saved
	// before the storage wipe. These are not tracked in any Go-state table so
	// they can only be preserved by snapshot + restore across the upgrade.
	if restoreErr := cs.RestorePreUpgradeRelationshipSlots(contractAddr); restoreErr != nil {
		failCount++
		if firstErr == nil {
			firstErr = restoreErr
		}
		fmt.Printf("[MIGRATE] ERROR: relationship slot restore failed: %v\n", restoreErr)
	}

	if firstErr != nil {
		fmt.Printf("[MIGRATE] ✗ EVM storage rebuild INCOMPLETE: %d slot/query write(s) failed (first error: %v)\n", failCount, firstErr)
		return fmt.Errorf("migration incomplete: %d write(s) failed: %w", failCount, firstErr)
	}
	fmt.Printf("[MIGRATE] ✓ EVM storage rebuilt: %d humans, %.2f AEQ total supply\n", totalHumans, totalSupply)
	return nil
}

// upgradeRelationshipSlotsTable is the name of the temporary table used to
// preserve guardian/escrow EVM storage slots across a contract upgrade wipe.
const upgradeRelationshipSlotsTable = "evm_upgrade_relationship_slots"

// SavePreUpgradeRelationshipSlots reads EVM storage slots 5 (escrowOf) and
// 13-16 (guardianOf / pendingGuardian / guardianRequestedAt / wardCount) from
// the live evm_storage table and copies them to a temporary snapshot table.
// Call this BEFORE wiping evm_storage on a contract upgrade; then call
// RestorePreUpgradeRelationshipSlots AFTER MigrateEVMFromGoState has rebuilt
// the rest of the storage to re-inject these slots back in.
// Returns an error if the snapshot could not be completed reliably — the
// caller (contract_deploy.go) must abort the upgrade rather than proceed to
// wipe evm_storage, since a failed/partial snapshot here means guardian and
// escrow relationships would be permanently lost by the wipe with no way to
// restore them afterward.
func (cs *ChainState) SavePreUpgradeRelationshipSlots(contractAddr string) error {
	if cs.db == nil {
		return nil
	}
	contractAddr = strings.ToLower(contractAddr)
	if _, err := cs.db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		address TEXT NOT NULL,
		slot    TEXT NOT NULL,
		value   TEXT NOT NULL,
		PRIMARY KEY (address, slot)
	)`, upgradeRelationshipSlotsTable)); err != nil {
		return fmt.Errorf("create snapshot table: %w", err)
	}
	// Clear any stale snapshot from a previous upgrade cycle.
	if _, err := cs.db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE address = $1`, upgradeRelationshipSlotsTable), contractAddr); err != nil {
		return fmt.Errorf("clear stale snapshot: %w", err)
	}

	// We can't filter by "slot prefix for base slot N" efficiently in SQL because
	// the slot hash is opaque. Instead, snapshot ALL slots for this address that
	// we cannot reconstruct from Go-state (i.e., everything EXCEPT the slots
	// MigrateEVMFromGoState already writes: 0,1,4,6,7,8,9,10,11,12). We do
	// this by saving all slots and then letting MigrateEVMFromGoState overwrite
	// the ones it knows about, so only the truly-opaque slots (5,13-16) survive.
	rows, err := cs.db.Query(`SELECT slot, value FROM evm_storage WHERE lower(address) = $1`, contractAddr)
	if err != nil {
		fmt.Printf("[DEPLOY] Warning: could not snapshot relationship slots: %v\n", err)
		return fmt.Errorf("query existing slots: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var slot, value string
		if scanErr := rows.Scan(&slot, &value); scanErr != nil {
			return fmt.Errorf("scan slot row: %w", scanErr)
		}
		if _, execErr := cs.db.Exec(fmt.Sprintf(`INSERT INTO %s (address, slot, value) VALUES ($1, $2, $3)
			ON CONFLICT (address, slot) DO UPDATE SET value = $3`, upgradeRelationshipSlotsTable),
			contractAddr, slot, value); execErr != nil {
			return fmt.Errorf("save slot %s: %w", slot, execErr)
		}
		count++
	}
	fmt.Printf("[DEPLOY] Saved %d EVM storage slots for guardian/escrow preservation\n", count)
	return nil
}

// RestorePreUpgradeRelationshipSlots writes the slots that were saved by
// SavePreUpgradeRelationshipSlots back into evm_storage. Called at the end of
// MigrateEVMFromGoState so these survive the upgrade storage wipe.
// Returns an error if any saved slot could not be restored — the caller
// (MigrateEVMFromGoState) folds this into its own failure tracking so a
// partial restore is reported instead of silently leaving guardian/escrow
// relationships missing post-upgrade.
func (cs *ChainState) RestorePreUpgradeRelationshipSlots(contractAddr string) error {
	if cs.db == nil {
		return nil
	}
	contractAddr = strings.ToLower(contractAddr)
	rows, err := cs.db.Query(fmt.Sprintf(`SELECT slot, value FROM %s WHERE address = $1`, upgradeRelationshipSlotsTable), contractAddr)
	if err != nil {
		return nil // table doesn't exist yet (first-ever deploy, no prior snapshot)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var slot, value string
		if scanErr := rows.Scan(&slot, &value); scanErr != nil {
			return fmt.Errorf("scan relationship slot row: %w", scanErr)
		}
		// Use INSERT … ON CONFLICT DO NOTHING so that slots MigrateEVMFromGoState
		// already wrote (balanceOf, isHuman, etc.) are not overwritten by stale
		// pre-upgrade values — only truly-missing slots get restored.
		if _, execErr := cs.db.Exec(`INSERT INTO evm_storage (address, slot, value) VALUES ($1, $2, $3)
			ON CONFLICT (address, slot) DO NOTHING`,
			contractAddr, slot, value); execErr != nil {
			return fmt.Errorf("restore slot %s: %w", slot, execErr)
		}
		count++
	}
	if count > 0 {
		fmt.Printf("[MIGRATE] Restored %d guardian/escrow slots from pre-upgrade snapshot\n", count)
	}
	return nil
}

// SyncBalancesToEVM writes the current Go-state AEQ balance for each addr into
// the AequitasV7 contract's balanceOf storage slot (mapping at position 4),
// keeping both ledgers consistent after every Go-state change.
// FIX (P0 for throughput, 2026-07-25 — second finding from the same live CPU
// profile as maybePruneTxReceipts): this used to write to Postgres
// SYNCHRONOUSLY, once per address, on the RPC request path. sendRawTransaction
// calls it with two addresses (sender and recipient) per transfer, so every
// transfer paid two extra round trips plus two cs.mu.RLock acquisitions before
// it could answer.
//
// Its own sibling was fixed for exactly this reason and this one was missed.
// syncBalanceLocked's comment (below) already reads:
//
//	SCALING_ARCHITECTURE.md Phase 6: with a real DB, this no longer writes
//	synchronously at all -- it just records addrs as needing a refresh
//	(cheap, in-memory, own small mutex, not cs.mu) and returns; a background
//	worker (evm_mirror_flush.go) drains that set periodically ... This is
//	safe specifically BECAUSE evm_storage is already documented as a
//	display-only mirror for eth_call/MetaMask, never the authoritative
//	ledger.
//
// Every word of that applies here. The machinery is built, documented, tested
// and running; this function simply never used it.
//
// Measured, profile after the receipt-prune fix landed: database/sql.(*DB).Exec
// still at 30.21% cumulative and database/sql.withLock at 31.42%, inside a
// sendRawTransaction that accounts for 57.48% of all samples -- with the
// receipt prune gone, these two writes are what is left on that path.
//
// Also strictly MORE correct, not merely faster. The loop this replaces read
// cs.accounts.Get and mirrored a cache miss as balance ZERO, silently writing a
// wrong balance for any account not currently warm. doSyncBalanceLocked (which
// the flush worker calls) pages a cold account in via ensureAccountLoaded
// first, so the deferred path cannot make that mistake.
//
// The cost is display lag: eth_call/MetaMask can read a balance up to
// evmMirrorFlushInterval (2s) stale. That is the tradeoff this codebase already
// accepted for the locked variant, and it never touches the authoritative
// ledger, which is cs.accounts plus the durable transfer path.
func (cs *ChainState) SyncBalancesToEVM(contractAddr string, addrs ...string) {
	if cs.db == nil {
		return
	}
	cs.markEVMMirrorDirtyLocked(contractAddr, addrs...)
}

// syncHumanRegistrationLocked writes balanceOf (slot 4), isHuman (slot 6),
// lastActivity (slot 10), and lastDemurrage (slot 11) EVM slots for a newly
// registered human. Must be called only while the caller already holds cs.mu (write lock).
// syncBalanceLocked now handles all four slots, so this is a simple delegation.
func (cs *ChainState) syncHumanRegistrationLocked(contractAddr string, addr string) {
	cs.syncBalanceLocked(contractAddr, addr)
}

// syncBalanceLocked is like SyncBalancesToEVM but reads cs.accounts directly
// without acquiring cs.mu. Must be called only while the caller already holds
// cs.mu (read or write lock) — calling SyncBalancesToEVM from inside a locked
// function would deadlock on the inner RLock().
//
// SCALING_ARCHITECTURE.md Phase 6: with a real DB, this no longer writes
// synchronously at all — it just records addrs as needing a refresh (cheap,
// in-memory, own small mutex, not cs.mu) and returns; a background worker
// (evm_mirror_flush.go) drains that set periodically and performs the exact
// batched write doSyncBalanceLocked below used to do inline. This is safe
// specifically BECAUSE evm_storage is already documented as a display-only
// mirror for eth_call/MetaMask, never the authoritative ledger (that's
// cs.accounts/chain_accounts) — unlike accountSetXOR (Phase 3), nothing here
// feeds StateRoot or cross-node consensus, so a short display lag has no
// correctness implications. It's also safe with respect to the OLD
// same-transaction coupling this comment used to describe (joining
// cs.activeTx so a later rollback would undo the mirror write too): the
// flush now always reads cs.accounts fresh, AFTER whatever operation marked
// it dirty has already fully committed or rolled back in memory, so it
// converges on the correct final balance regardless of that outcome.
//
// doSyncBalanceLocked (below) is the extracted synchronous implementation,
// still used directly by the flush worker itself (which acquires cs.mu on
// its own timeline, not the original caller's).
func (cs *ChainState) syncBalanceLocked(contractAddr string, addrs ...string) {
	if cs.db == nil {
		return
	}
	cs.markEVMMirrorDirtyLocked(contractAddr, addrs...)
}

// doSyncBalanceLocked is syncBalanceLocked's original body: caller must
// already hold cs.mu (write lock — ensureAccountLoaded below can mutate
// cs.accounts for a cold address) and cs.db must be non-nil.
// Syncs slots: 4 (balanceOf), 6 (isHuman), 10 (lastActivity), 11 (lastDemurrage).
//
// FIX (audit 2026-06-28 recheck 4, P1-6): every SaveStorageSlot call here
// used to either discard its error outright (slots 6/10/11) or only log it
// (slot 4), with nothing durable recording a failure.
//
// FIX (audit 2026-06-28 Gesamtaudit, P0-1): these calls now use
// saveStorageSlotLocked, not SaveStorageSlot — this function's own
// precondition (caller already holds cs.mu) is exactly what makes that
// safe, and (when called synchronously, pre-Phase-6) is what let the EVM
// mirror slot writes join the SAME SQL transaction (cs.activeTx) as
// whatever atomic Go-state mutation was calling it. Any address whose slot
// write STILL fails (activeTx itself aborted, or a caller with cs.db set
// but no surrounding transaction) is queued the same way
// notifyProofServerWithRetryQueue queues failures (register.go), and
// RetryEVMMirrorSyncQueue (started from NewAPIServer) catches up later.
//
// FIX (performance audit 2026-07-06): this used to issue up to 4 separate
// saveStorageSlotLocked round trips per address (balanceOf, isHuman,
// lastActivity, lastDemurrage) — every daily distribution round calls this
// with every human's address, so that was up to 4×N sequential DB writes
// held under cs.mu, growing with the human count. Every slot write across
// every address in this call is now built into ONE multi-row INSERT ...
// ON CONFLICT instead. This does mean an address's slots now succeed or
// fail together with the rest of the batch, not independently — but the
// realistic failure mode here is a lost DB connection or an aborted
// activeTx (both apply to every row in the batch identically; evm_storage
// has no per-row business constraint beyond the (address,slot) upsert key
// this batching still honors), so no real-world retry behavior changes,
// only 4×N-1 unnecessary round trips are removed on the common path.
func (cs *ChainState) doSyncBalanceLocked(contractAddr string, addrs ...string) {
	contractAddr = strings.ToLower(contractAddr)

	type slotValue struct {
		slot, value string
	}
	var writes []slotValue
	lowerAddrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addr = strings.ToLower(addr)
		lowerAddrs[i] = addr
		// FIX (Monster Audit follow-up, 2026-07-12, P2): a cold addr here used
		// to read as !ok, which unconditionally wrote the EVM-mirror
		// balanceOf slot (below) as 0 regardless of the address's real Go-state
		// balance — every plain transfer passes the 4 pool addresses into this
		// function (see transferLocked/transferWithV7FeeLocked/registerHumanLocked),
		// exactly the addresses already known to go cold. Display-only (the
		// ledger of record in cs.accounts/chain_accounts is untouched, and the
		// next warm call self-corrects the slot), but wrong regardless for
		// anything reading balanceOf via eth_call/MetaMask/a dApp in the
		// meantime.
		cs.ensureAccountLoaded(addr)
		acc, ok := cs.accounts.Get(addr)
		var bal float64
		if ok {
			// P1-4: use effectiveBalance (demurrage-adjusted) so the EVM slot
			// matches the user's real spendable amount, not the stored pre-decay value.
			bal = effectiveBalance(acc).Float()
		}
		balBig := aeqToWei(bal)
		addrBytes := common.HexToAddress(addr).Bytes()
		// slot 4: balanceOf
		writes = append(writes, slotValue{mappingSlot(addrBytes, 4).Hex(), common.BigToHash(balBig).Hex()})
		if !ok {
			continue
		}
		// slot 6: isHuman
		isHumanVal := common.HexToHash("0x00")
		if acc.IsHuman {
			isHumanVal = common.HexToHash("0x01")
		}
		writes = append(writes, slotValue{mappingSlot(addrBytes, 6).Hex(), isHumanVal.Hex()})
		// slots 10 + 11: lastActivity / lastDemurrage
		if acc.LastActivityAt > 0 {
			ts := common.BigToHash(big.NewInt(acc.LastActivityAt)).Hex()
			writes = append(writes, slotValue{mappingSlot(addrBytes, 10).Hex(), ts})
			writes = append(writes, slotValue{mappingSlot(addrBytes, 11).Hex(), ts})
		}
	}
	if len(writes) == 0 {
		return
	}

	// See saveAccountsToDBBatchCtx's FIX comment (state.go) for why this
	// builds a fixed-size unnest() query over 2 array parameters instead
	// of a VALUES(...) list whose text size grows with how many addresses
	// the EVM mirror flush worker has accumulated since its last tick.
	// address (contractAddr) is a single scalar $1, not a third array,
	// since every row this call writes shares the same contract address
	// by construction (the mapping key that varies per user is encoded
	// inside each slot hash, not this column).
	slots := make([]string, len(writes))
	values := make([]string, len(writes))
	for i, w := range writes {
		slots[i] = w.slot
		values[i] = w.value
	}
	query := `INSERT INTO evm_storage (address, slot, value)
SELECT $1, s, v FROM unnest($2::text[], $3::text[]) AS t(s, v)
ON CONFLICT (address, slot) DO UPDATE SET value = EXCLUDED.value`
	_, err := cs.dbExec().Exec(query, contractAddr, pq.Array(slots), pq.Array(values))
	if err != nil {
		fmt.Printf("[EVM] Warning: could not batch-sync EVM mirror slots for %d address(es): %v\n", len(lowerAddrs), err)
		for _, addr := range lowerAddrs {
			cs.QueueEVMMirrorSync(addr, contractAddr, err.Error())
		}
		return
	}
	// THROUGHPUT (2026-07-22): skip this round trip entirely when nothing
	// is believed queued for retry — see evmMirrorQueueMaybeNonEmpty's own
	// field comment. The overwhelmingly common case (this write, just
	// above, succeeded and nothing has ever failed before) has nothing to
	// clean up here.
	if cs.evmMirrorQueueMaybeNonEmpty.Load() {
		cs.RemoveBatchFromEVMMirrorSyncQueue(lowerAddrs, contractAddr)
	}
}

// syncGuardianEscrowSlotsLocked keeps guardianOf (slot 13) and escrowOf
// (slot 5) current for addr, reading their authoritative values from the
// guardians/escrow_accounts tables (guardian.go) — the real Go-side source
// of truth for both.
//
// FIX (P2-5, beta-launch audit 2026-07-05): guardian/escrow/UBI EVM storage
// slots were only ever written once, at deploy/migration time, and never
// touched again by the real (Go-native) guardian/escrow logic — an external
// eth_call to V7.escrowOf(address)/V7.guardianOf(address) saw whatever was
// true at the last migration forever, not the live value. Deliberately NOT
// extending the much more frequently called syncBalanceLocked to also do
// this: that function runs on every transfer/swap/register (an extra 2-table
// DB read per call for fields those operations never touch would be a real,
// avoidable cost on the hottest path in this codebase). This is a separate,
// narrowly-scoped sync called only from the guardian/escrow mutation points
// themselves (SetGuardian, RecoverFromEscrow, checkAndMoveToEscrowLocked,
// releaseEscrowToUBILocked — see their call sites).
//
// Deliberately does NOT attempt pendingGuardian (slot 14), wardCount (slot
// 16), ubiPool (slot 2), or ubiClaimed (slot 12): the Go-side guardian model
// commits a guardian relationship directly (see the `guardians` table's own
// schema — a single row, no separate "proposed but not yet confirmed" state
// at all), and the Go-side UBI model distributes daily by crediting every
// human's balance immediately (distributeUBIPoolLocked, state.go) rather
// than accumulating a per-head entitlement humans separately pull later —
// neither Solidity-side concept has a Go-side equivalent value to sync
// FROM. This isn't unsynced data; it's two genuinely different accounting
// models for the same real-world behavior, and only the Go side is ever
// actually used to move real value. Integrators who need live figures for
// any of these should read /api/escrow, /api/pool, or /api/guardian-style
// endpoints (Go-state-backed), not raw eth_call on the V7 contract.
// syncGuardianEscrowSlotsLocked is the context.Background()-calling wrapper
// kept for callers not yet migrated to thread ctx explicitly — see
// dbExecCtx's comment for the migration this is part of.
func (cs *ChainState) syncGuardianEscrowSlotsLocked(contractAddr, addr string) {
	cs.syncGuardianEscrowSlotsLockedCtx(context.Background(), contractAddr, addr)
}

func (cs *ChainState) syncGuardianEscrowSlotsLockedCtx(ctx context.Context, contractAddr, addr string) {
	if cs.db == nil {
		return
	}
	contractAddr = strings.ToLower(contractAddr)
	addr = strings.ToLower(addr)
	addrBytes := common.HexToAddress(addr).Bytes()

	// FIX: use cs.dbExecCtx(ctx), not cs.db directly — several callers (e.g.
	// checkAndMoveToEscrowLocked) run this from inside an already-active,
	// not-yet-committed transaction. A direct cs.db read only sees
	// committed data (Postgres read-committed isolation), so it would miss
	// the very escrow_accounts row this exact call is meant to reflect,
	// syncing a stale (often zero) value instead.
	var guardianAddr string
	if err := cs.dbExecCtx(ctx).QueryRow(`SELECT lower(guardian_address) FROM guardians WHERE lower(wallet_address) = $1`, addr).Scan(&guardianAddr); err != nil && err != sql.ErrNoRows {
		fmt.Printf("[EVM] Warning: could not read guardian for %s: %v\n", addr, err)
	}
	guardianVal := common.Hash{}
	if guardianAddr != "" {
		guardianVal = common.BytesToHash(common.HexToAddress(guardianAddr).Bytes())
	}
	if err := cs.saveStorageSlotLockedCtx(ctx, contractAddr, mappingSlot(addrBytes, 13).Hex(), guardianVal.Hex()); err != nil {
		fmt.Printf("[EVM] Warning: could not sync guardianOf for %s: %v\n", addr, err)
	}

	var escrowAmount float64
	if err := cs.dbExecCtx(ctx).QueryRow(`SELECT amount FROM escrow_accounts WHERE wallet_address = $1`, addr).Scan(&escrowAmount); err != nil && err != sql.ErrNoRows {
		fmt.Printf("[EVM] Warning: could not read escrow for %s: %v\n", addr, err)
	}
	escrowBig := aeqToWei(escrowAmount)
	if err := cs.saveStorageSlotLockedCtx(ctx, contractAddr, mappingSlot(addrBytes, 5).Hex(), common.BigToHash(escrowBig).Hex()); err != nil {
		fmt.Printf("[EVM] Warning: could not sync escrowOf for %s: %v\n", addr, err)
	}
}

// SyncGuardianEscrowSlots is syncGuardianEscrowSlotsLocked's public wrapper
// for callers that don't already hold cs.mu (SetGuardian, RecoverFromEscrow
// — see guardian.go). Callers that already hold cs.mu (ConfirmAlive,
// checkAndMoveToEscrowLocked, releaseEscrowToUBILocked) call the Locked
// version directly instead.
func (cs *ChainState) SyncGuardianEscrowSlots(contractAddr, addr string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.syncGuardianEscrowSlotsLocked(contractAddr, addr)
}

// retryQueueMaxAttempts is the number of retry attempts after which a queue
// entry is moved to dead-letter (dead=TRUE). Dead entries are no longer picked
// up by Load* and require manual intervention (UPDATE ... SET dead=FALSE to
// requeue). Exposed via /api/health/combined dead counts.
const retryQueueMaxAttempts = 20

// QueueEVMMirrorSync persists a failed syncBalanceLocked slot write so
// RetryEVMMirrorSyncQueue can catch up later — see syncBalanceLocked's own
// comment (audit 2026-06-28 recheck 4, P1-6).
// P2-4 fix: sets next_retry_at using exponential backoff capped at 4 hours,
// and marks dead=TRUE after retryQueueMaxAttempts failures so the queue does
// not grow unbounded and dead entries are visible in the health endpoint.
func (cs *ChainState) QueueEVMMirrorSync(addr, contractAddr, lastErr string) {
	if cs.db == nil {
		return
	}
	initialNextRetry := time.Now().Unix() + 60 // 2^1 * 30 = first retry after 60s
	// FIX (P1-02): use dbExec() so this write participates in any active
	// transaction rather than bypassing it via the raw cs.db handle.
	if _, err := cs.dbExec().Exec(
		`INSERT INTO evm_mirror_sync_queue (address, contract_addr, last_error, next_retry_at, dead)
		 VALUES ($1, $2, $3, $4, FALSE)
		 ON CONFLICT (address, contract_addr) DO UPDATE SET
		   attempts      = evm_mirror_sync_queue.attempts + 1,
		   last_error    = EXCLUDED.last_error,
		   last_attempt_at = NOW(),
		   next_retry_at = (EXTRACT(EPOCH FROM NOW())::bigint
		                    + LEAST(POWER(2, evm_mirror_sync_queue.attempts + 1)::bigint * 30, 14400)),
		   dead          = (evm_mirror_sync_queue.attempts + 1) >= $5`,
		addr, contractAddr, lastErr, initialNextRetry, retryQueueMaxAttempts,
	); err != nil {
		fmt.Printf("[EVM] Warning: could not queue mirror sync retry for %s: %v\n", addr, err)
		return
	}
	// See evmMirrorQueueMaybeNonEmpty's own field comment.
	cs.evmMirrorQueueMaybeNonEmpty.Store(true)
}

// evmMirrorSyncQueueEntry is one row from evm_mirror_sync_queue.
type evmMirrorSyncQueueEntry struct {
	Address      string
	ContractAddr string
}

// CountEVMMirrorSyncQueue returns pending (non-dead) entry count, dead-letter
// count, and age in seconds of the oldest pending entry (0 if empty).
// P2-4 fix: now distinguishes pending from dead-letter so /api/health/combined
// can tell an operator whether retries are still ongoing or have permanently
// stalled and need manual intervention.
func (cs *ChainState) CountEVMMirrorSyncQueue() (count int, deadCount int, oldestAgeSecs int64) {
	if cs.db == nil {
		return 0, 0, 0
	}
	var oldest sql.NullInt64
	if err := cs.db.QueryRow(
		`SELECT COUNT(*) FILTER (WHERE NOT dead),
		        COUNT(*) FILTER (WHERE dead),
		        MIN(EXTRACT(EPOCH FROM created_at))::bigint FILTER (WHERE NOT dead)
		 FROM evm_mirror_sync_queue`,
	).Scan(&count, &deadCount, &oldest); err != nil {
		return 0, 0, 0
	}
	if oldest.Valid {
		oldestAgeSecs = time.Now().Unix() - oldest.Int64
	}
	return count, deadCount, oldestAgeSecs
}

// LoadEVMMirrorSyncQueue returns up to 200 pending retry entries whose
// next_retry_at is in the past (or NULL for pre-migration rows) and that have
// not yet hit the dead-letter limit, oldest first.
func (cs *ChainState) LoadEVMMirrorSyncQueue() []evmMirrorSyncQueueEntry {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(
		`SELECT address, contract_addr FROM evm_mirror_sync_queue
		 WHERE NOT dead
		   AND (next_retry_at IS NULL OR next_retry_at <= EXTRACT(EPOCH FROM NOW())::bigint)
		 ORDER BY created_at LIMIT 200`)
	if err != nil {
		fmt.Printf("[EVM] Warning: could not load mirror sync queue: %v\n", err)
		return nil
	}
	defer rows.Close()
	var entries []evmMirrorSyncQueueEntry
	for rows.Next() {
		var e evmMirrorSyncQueueEntry
		if err := rows.Scan(&e.Address, &e.ContractAddr); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// RemoveFromEVMMirrorSyncQueue deletes a row once its retry succeeds.
func (cs *ChainState) RemoveFromEVMMirrorSyncQueue(addr, contractAddr string) {
	if cs.db == nil {
		return
	}
	// FIX (deadlock, concurrency audit 2026-07-21): cs.dbExec() instead of
	// cs.db — see ensureAccountLoaded's FIX comment for the full
	// connection-pool self-deadlock this closes. Called from
	// syncBalanceLocked (transferLocked's own call chain) up to 6 times per
	// transfer (once per touched address), each needing its own pool
	// connection under the old cs.db.Exec while cs.mu+cs.activeTx were
	// already held — the single biggest contributor to the deadlock
	// reproduced live via the local TPS benchmark. Safe unconditionally:
	// cs.dbExec() falls back to cs.db when called standalone (e.g. from
	// RetryEVMMirrorSyncQueue's periodic background pass, outside any
	// active transaction).
	if _, err := cs.dbExec().Exec(`DELETE FROM evm_mirror_sync_queue WHERE address = $1 AND contract_addr = $2`, addr, contractAddr); err != nil {
		fmt.Printf("[EVM] Warning: could not remove mirror sync queue entry: %v\n", err)
	}
}

// RemoveBatchFromEVMMirrorSyncQueue is RemoveFromEVMMirrorSyncQueue for
// every address in addrs (same contractAddr) in ONE round trip instead of
// one per address.
//
// THROUGHPUT (2026-07-22): syncBalanceLocked calls this once per transfer
// with up to 6 addresses (sender, recipient, 4 tokenomics pools) — the
// prior one-DELETE-per-address loop meant every successful transfer paid
// up to 6 extra sequential round trips for what is, in the overwhelmingly
// common case, deleting zero rows (evm_mirror_sync_queue only ever has an
// entry for an address whose PREVIOUS mirror sync attempt failed).
// Measured as the single largest remaining per-transfer round-trip cost
// after group-commit batching (see TransferAtomic's own comment) reduced
// commit/fsync count but left this loop untouched.
func (cs *ChainState) RemoveBatchFromEVMMirrorSyncQueue(addrs []string, contractAddr string) {
	if cs.db == nil || len(addrs) == 0 {
		return
	}
	if _, err := cs.dbExec().Exec(`DELETE FROM evm_mirror_sync_queue WHERE contract_addr = $1 AND address = ANY($2)`, contractAddr, pq.Array(addrs)); err != nil {
		fmt.Printf("[EVM] Warning: could not batch-remove mirror sync queue entries: %v\n", err)
	}
}

// RetryEVMMirrorSyncQueue attempts every pending evm_mirror_sync_queue entry
// once. Intended to be called periodically (see NewAPIServer's startup
// goroutine). syncBalanceLocked itself re-queues (or clears) each entry, so
// this only needs to drive the loop.
func RetryEVMMirrorSyncQueue(cs *ChainState) {
	byContract := make(map[string][]string)
	for _, entry := range cs.LoadEVMMirrorSyncQueue() {
		byContract[entry.ContractAddr] = append(byContract[entry.ContractAddr], entry.Address)
	}
	if len(byContract) == 0 {
		// See evmMirrorQueueMaybeNonEmpty's own field comment: this
		// unconditional table query is the periodic reconciliation that
		// lets syncBalanceLocked's per-transfer skip resume after a
		// failure episode clears.
		cs.evmMirrorQueueMaybeNonEmpty.Store(false)
		return
	}
	// syncBalanceLocked only reads cs.accounts and writes to the DB (not to
	// any in-memory state), so a read lock is sufficient — see its own
	// doc comment ("read or write lock").
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for contractAddr, addrs := range byContract {
		cs.syncBalanceLocked(contractAddr, addrs...)
	}
}

// ─── EVM ENGINE HELPERS ───────────────────────────────────────────────────────

// PersistContractStorage reads storage slots from a stateDB and saves to PostgreSQL.
// Since we no longer have a persistent stateDB, this is a no-op log.
func (e *EVMEngine) PersistContractStorage(contractAddr common.Address) {
	fmt.Printf("[EVM] Contract %s active in session\n", strings.ToLower(contractAddr.Hex()))
}

// NewPersistentStateDB creates a StateDB loaded from PostgreSQL.
// Used by tests and legacy code. For production use EVMEngine.newStateDB().
func NewPersistentStateDB(cs *ChainState) (*state.StateDB, error) {
	memDB := rawdb.NewMemoryDatabase()
	sdb, err := state.New(common.Hash{}, state.NewDatabase(memDB), nil)
	if err != nil {
		return nil, err
	}

	// P2-AUDIT: Do NOT call LoadNonce per account — that issues N PostgreSQL
	// queries (one per account) and creates a DoS vector. EVM nonces for
	// sends are managed by the RPC layer; the legacy StateDB doesn't need
	// per-account nonces for call execution. Matches the fix in newStateDB.
	for _, acc := range cs.GetAllAccounts() {
		addr := common.HexToAddress(acc.Address)
		// P1-FIX: acc.Balance is a Decimal (int64 micro-units). Use .Float()
		// to get the AEQ float value before converting to wei. Using
		// int64(acc.Balance) directly would re-interpret micro-AEQ as whole-AEQ
		// and multiply by 1e18 a second time, overstating balances by 1e6×.
		sdb.SetBalance(addr, aeqToWei(acc.Balance.Float()))
	}

	for _, addrStr := range cs.GetAllContracts() {
		addr := common.HexToAddress(addrStr)
		code, err := cs.LoadContract(addrStr)
		if err != nil || len(code) == 0 {
			continue
		}
		sdb.SetCode(addr, code)
		fmt.Printf("[EVM] Loaded contract: %s (%d bytes)\n", addrStr, len(code))

		if cs.db != nil {
			rows, err := cs.db.Query(
				`SELECT slot, value FROM evm_storage WHERE address = $1`, addrStr)
			if err == nil {
				for rows.Next() {
					var slot, value string
					rows.Scan(&slot, &value)
					sdb.SetState(addr, common.HexToHash(slot), common.HexToHash(value))
				}
				rows.Close()
			}
		}
	}

	sdb.Commit(0, false)
	return sdb, nil
}

// SaveBioRegistration links a ZK proof commitment to the wallet that
// successfully registered with it. Called once, right after a
// registerWithSig transaction is confirmed successful — never speculatively.
// bioHash is also stored alongside the commitment so the app (which only
// ever knows its own bioHash, not the commitment computed on the website
// under the new flow) can poll for its registration — see
// GetWalletByBioHash below.
func (cs *ChainState) SaveBioRegistration(commitment, walletAddress, txHash, bioHash string) error {
	if commitment == "" {
		return fmt.Errorf("empty commitment rejected")
	}
	if cs.db == nil {
		return nil
	}
	walletAddress = strings.ToLower(walletAddress)
	if bioHash != "" {
		existing := cs.GetWalletByBioHash(bioHash)
		if existing != "" && strings.ToLower(existing) != walletAddress {
			return fmt.Errorf("biometric already registered to %s", existing)
		}
	}
	// P2-AUDIT: Use ON CONFLICT DO NOTHING to protect the first successful
	// registration from being overwritten by a concurrent/replay registration
	// with the same commitment. The contract itself enforces commitment uniqueness
	// on-chain; the DB row is just a mirror for polling — never the authority.
	_, err := cs.db.Exec(
		`INSERT INTO bio_registrations (commitment, wallet_address, tx_hash, bio_hash) VALUES ($1, $2, $3, $4)
 ON CONFLICT (commitment) DO NOTHING`,
		commitment, walletAddress, txHash, bioHash,
	)
	return err
}

// RegistrationDebugInfo reports, per-layer, whether wallet shows up as
// already-registered anywhere — used by the /api/admin/registration-debug
// endpoint to make "already registered" actionable: which of the several
// independent tables/slots involved in registration is actually blocking.
type RegistrationDebugInfo struct {
	ChainIsHuman          bool    `json:"chain_is_human"`
	ChainBalance          float64 `json:"chain_balance"`
	NullifierExists       bool    `json:"nullifier_exists"`
	BioRegistrationExists bool    `json:"bio_registration_exists"`
	BioHashExists         bool    `json:"bio_hash_exists"`
	EVMIsHumanSlot        bool    `json:"evm_is_human_slot"`
}

// GetRegistrationDebugInfo gathers the per-layer registration state for a
// wallet. Caller is responsible for authenticating the request — this
// function itself does no access control.
func (cs *ChainState) GetRegistrationDebugInfo(wallet string) RegistrationDebugInfo {
	wallet = strings.ToLower(wallet)
	info := RegistrationDebugInfo{
		ChainIsHuman: cs.IsHuman(wallet),
		ChainBalance: cs.GetBalance(wallet),
	}
	if cs.db == nil {
		return info
	}
	cs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM nullifiers WHERE lower(wallet_address) = $1)`, wallet).Scan(&info.NullifierExists)
	cs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM bio_registrations WHERE lower(wallet_address) = $1)`, wallet).Scan(&info.BioRegistrationExists)
	cs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM bio_hashes WHERE lower(wallet_address) = $1)`, wallet).Scan(&info.BioHashExists)
	addrBytes := common.HexToAddress(wallet).Bytes()
	isHumanSlot := mappingSlot(addrBytes, 6).Hex()
	if val, err := cs.LoadStorageSlot(strings.ToLower(V7_CONTRACT_ADDR), isHumanSlot); err == nil {
		info.EVMIsHumanSlot = common.HexToHash(val) != (common.Hash{})
	}
	return info
}

// GetWalletByCommitment looks up which wallet (if any) successfully
// registered with a given proof commitment. Returns "" if none found —
// this lets the app ask "did MY specific proof get registered?" instead of
// reading the last entry in a global, unfiltered humans list.
func (cs *ChainState) GetWalletByCommitment(commitment string) string {
	if cs.db == nil {
		return ""
	}
	var wallet string
	err := cs.db.QueryRow(`SELECT wallet_address FROM bio_registrations WHERE commitment = $1`, commitment).Scan(&wallet)
	if err != nil {
		return ""
	}
	return wallet
}

// GetWalletByBioHash looks up which wallet (if any) most recently
// completed registration for a given device biometric identity hash.
// Used by the app's post-bioHash-flow polling (startPollingByBioHash) —
// the app never computes a commitment itself under that flow, only its
// own bioHash, so this is the only key it can reliably poll by.
func (cs *ChainState) GetWalletByBioHash(bioHash string) string {
	if cs.db == nil {
		return ""
	}
	var wallet string
	err := cs.db.QueryRow(`SELECT wallet_address FROM bio_registrations WHERE bio_hash = $1 ORDER BY registered_at DESC LIMIT 1`, bioHash).Scan(&wallet)
	if err != nil {
		return ""
	}
	return wallet
}

// GetWalletByStoredBioHash looks up a wallet by the chain's OWN bio_hashes
// table (written by SaveBioHash below) — distinct from GetWalletByBioHash,
// which queries bio_registrations. The two tables can disagree (e.g. after
// a partial reset, or if a row was written to one but not the other), so
// registerOnV7 checks both as defense-in-depth rather than trusting either
// alone.
func (cs *ChainState) GetWalletByStoredBioHash(bioHash string) string {
	if cs.db == nil || bioHash == "" {
		return ""
	}
	var wallet string
	err := cs.db.QueryRow(`SELECT wallet_address FROM bio_hashes WHERE hash = $1`, bioHash).Scan(&wallet)
	if err != nil {
		return ""
	}
	return wallet
}

// SaveBioHash writes the biometric hash into the chain's OWN bio_hashes
// table after a confirmed registration. NOTE: despite the similar name and
// schema, this is NOT the same table the separate proof-server service
// checks in its /check and /prove endpoints — that service runs its own
// process with its own DATABASE_URL/Postgres instance (see
// aequitas-proof-server/bio_store.js). Clearing or populating THIS table
// has no effect on what the proof server blocks; it only affects
// GetWalletByStoredBioHash above and the chain's own bookkeeping.
//
// FIX (audit recheck2, P1 #6): the audit asked this project to pick one of
// two paths for this table — either declare it explicitly UX/diagnostic
// only, or make it atomic/consensus-relevant like the nullifier. This
// project already chose the first path, deliberately: the comment above
// (predating this fix) already establishes the REAL one-human-one-
// registration guarantee is the ZK nullifier (see TryClaimNullifier /
// RegisterHumanAtomic), checked and recorded atomically with the
// registration itself; this table is a secondary, best-effort lookup index
// for GetWalletByStoredBioHash, not itself a security boundary, and is not
// replayed from block TXs (see block.go's register_human case calling
// SaveBioRegistration, a different table, with bioHash deliberately empty —
// this table is local bookkeeping per node, not consensus state). Given
// that, returning an error here (instead of only logging) lets the one
// caller that might care — the registration RPC handler — at least know a
// write failed, without pretending a failure here should block or roll
// back the registration it's diagnostic for.
// CountChainBioHashes and CountChainNullifiers expose this node's own
// bio_hashes/nullifiers row counts — paired with the proof-server's own
// bio_hash_count (polled via syncProofServerStatus) in /api/health/combined
// so the three counts that should normally track each other (chain
// nullifiers, chain bio_hashes, proof-server bio_hashes) are visible
// separately instead of only inferred from confusing "already registered"
// reports (audit 2026-06-28 recheck 5, P2-3).
func (cs *ChainState) CountChainBioHashes() int {
	if cs.db == nil {
		return 0
	}
	var count int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM bio_hashes`).Scan(&count); err != nil {
		return 0
	}
	return count
}

func (cs *ChainState) CountChainNullifiers() int {
	if cs.db == nil {
		cs.mu.RLock()
		defer cs.mu.RUnlock()
		return len(cs.nullifiers)
	}
	var count int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM nullifiers`).Scan(&count); err != nil {
		return 0
	}
	return count
}

func (cs *ChainState) SaveBioHash(bioHash, walletAddress string) error {
	if cs.db == nil || bioHash == "" {
		return nil
	}
	walletAddress = strings.ToLower(walletAddress)
	_, err := cs.db.Exec(
		`INSERT INTO bio_hashes (hash, wallet_address) VALUES ($1, $2) ON CONFLICT (hash) DO NOTHING`,
		bioHash, walletAddress,
	)
	if err != nil {
		fmt.Printf("[REGISTER] Warning: could not sync bio_hashes: %v\n", err)
		return fmt.Errorf("could not sync bio_hashes for %s: %w", walletAddress, err)
	}
	return nil
}

// QueueProofServerSync persists a failed notifyProofServer attempt so
// RetryProofServerSyncQueue can catch up later instead of the sync gap
// being permanent — see proof_server_sync_queue's own table comment
// (state.go) for why this exists (audit 2026-06-28 recheck 4, P1-5).
// P2-4 fix: ON CONFLICT now also sets next_retry_at using exponential
// backoff (capped at 4h) and marks dead=TRUE after retryQueueMaxAttempts
// failures so permanently-unreachable proof-servers don't grow the queue
// unbounded and dead entries surface in /api/health/combined.
func (cs *ChainState) QueueProofServerSync(bioHashKey, wallet, lastErr string) {
	if cs.db == nil || bioHashKey == "" {
		return
	}
	initialNextRetry := time.Now().Unix() + 60 // 2^1 * 30 = first retry after 60s
	if _, err := cs.db.Exec(
		`INSERT INTO proof_server_sync_queue (bio_hash_key, wallet_address, last_error, next_retry_at, dead)
		 VALUES ($1, $2, $3, $4, FALSE)
		 ON CONFLICT (bio_hash_key) DO UPDATE SET
		   attempts      = proof_server_sync_queue.attempts + 1,
		   last_error    = EXCLUDED.last_error,
		   last_attempt_at = NOW(),
		   next_retry_at = (EXTRACT(EPOCH FROM NOW())::bigint
		                    + LEAST(POWER(2, proof_server_sync_queue.attempts + 1)::bigint * 30, 14400)),
		   dead          = (proof_server_sync_queue.attempts + 1) >= $5`,
		bioHashKey, strings.ToLower(wallet), lastErr, initialNextRetry, retryQueueMaxAttempts,
	); err != nil {
		fmt.Printf("[REGISTER] Warning: could not queue proof-server sync retry for %s: %v\n", wallet, err)
	}
}

// proofServerSyncQueueEntry is one row from proof_server_sync_queue.
type proofServerSyncQueueEntry struct {
	BioHashKey string
	Wallet     string
	Attempts   int
}

// CountProofServerSyncQueue returns pending (non-dead) entry count, dead-letter
// count, and age in seconds of the oldest pending entry (0 if empty).
// P2-4 fix: distinguishes pending from dead-letter; see CountEVMMirrorSyncQueue.
func (cs *ChainState) CountProofServerSyncQueue() (count int, deadCount int, oldestAgeSecs int64) {
	if cs.db == nil {
		return 0, 0, 0
	}
	var oldest sql.NullInt64
	if err := cs.db.QueryRow(
		`SELECT COUNT(*) FILTER (WHERE NOT dead),
		        COUNT(*) FILTER (WHERE dead),
		        MIN(EXTRACT(EPOCH FROM created_at))::bigint FILTER (WHERE NOT dead)
		 FROM proof_server_sync_queue`,
	).Scan(&count, &deadCount, &oldest); err != nil {
		return 0, 0, 0
	}
	if oldest.Valid {
		oldestAgeSecs = time.Now().Unix() - oldest.Int64
	}
	return count, deadCount, oldestAgeSecs
}

// LoadProofServerSyncQueue returns up to 50 pending retry entries whose
// next_retry_at is in the past (or NULL for pre-migration rows) and that have
// not yet hit the dead-letter limit, oldest first.
func (cs *ChainState) LoadProofServerSyncQueue() []proofServerSyncQueueEntry {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(
		`SELECT bio_hash_key, wallet_address, attempts FROM proof_server_sync_queue
		 WHERE NOT dead
		   AND (next_retry_at IS NULL OR next_retry_at <= EXTRACT(EPOCH FROM NOW())::bigint)
		 ORDER BY created_at LIMIT 50`)
	if err != nil {
		fmt.Printf("[REGISTER] Warning: could not load proof-server sync queue: %v\n", err)
		return nil
	}
	defer rows.Close()
	var entries []proofServerSyncQueueEntry
	for rows.Next() {
		var e proofServerSyncQueueEntry
		if err := rows.Scan(&e.BioHashKey, &e.Wallet, &e.Attempts); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// RemoveFromProofServerSyncQueue deletes a row once its retry succeeds.
func (cs *ChainState) RemoveFromProofServerSyncQueue(bioHashKey string) {
	if cs.db == nil {
		return
	}
	if _, err := cs.db.Exec(`DELETE FROM proof_server_sync_queue WHERE bio_hash_key = $1`, bioHashKey); err != nil {
		fmt.Printf("[REGISTER] Warning: could not remove proof-server sync queue entry: %v\n", err)
	}
}

// ─── NULLIFIERS ───────────────────────────────────────────────────────────────
//
// A nullifier is a one-way derivation of the biometric secret:
//   nullifier = SHA256(bioHash + ":aequitas-ubi-v1")
//
// It is computed by the client and stored on-chain after a successful
// registration. Because the same biometric always produces the same bioHash
// (on the same device), it always produces the same nullifier — so a second
// registration attempt reveals an already-used nullifier and is rejected,
// even if the user switches wallets. The server never sees the raw bioHash
// in this step, only its SHA256 derivative. In a future ZK upgrade the
// nullifier will be generated inside the Groth16 circuit itself (Semaphore
// style), removing even the SHA256 link.

func (cs *ChainState) IsNullifierUsed(nullifier string) bool {
	// nullifiersMu, not cs.mu: see that field's own comment -- cs.nullifiers
	// is a plain Go map, unsafe for concurrent access from multiple
	// RLock-holding goroutines (the concurrent-registration fast path,
	// register_concurrent.go) even to different keys, and this is the only
	// field this specific read touches.
	cs.nullifiersMu.Lock()
	_, inMem := cs.nullifiers[nullifier]
	cs.nullifiersMu.Unlock()
	if inMem {
		return true
	}
	if cs.db == nil {
		return false
	}
	// FIX (P0-02): use EXISTS so nullifiers imported with empty wallet
	// (public snapshots, where wallet_address = '') are correctly treated as
	// used. The old wallet != "" check returned false for those rows.
	var exists bool
	err := cs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM nullifiers WHERE nullifier = $1)`, nullifier).Scan(&exists)
	return err == nil && exists
}

// maxInMemNullifiers caps the in-memory nullifier cache to ~50 MB at 1M entries.
// P3-7: above this threshold new nullifiers are only written to DB; lookups
// fall through to the DB automatically via IsNullifierUsed.
const maxInMemNullifiers = 500_000

// TryClaimNullifier atomically inserts the nullifier and returns true if it
// was newly inserted (this caller owns the registration), false if it already
// existed (another goroutine or a previous replay already claimed it).
// Using a DB-level INSERT … ON CONFLICT eliminates the TOCTOU window between
// IsNullifierUsed and SaveNullifier — no separate mutex required.
func (cs *ChainState) TryClaimNullifier(nullifier, walletAddress string) (bool, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	return cs.tryClaimNullifierLocked(context.Background(), nullifier, walletAddress)
}

// tryClaimNullifierLocked is TryClaimNullifier's body, for callers that
// already hold cs.mu (audit recheck3, P0/P1: replayTransactions needs this
// so it can hold cs.mu continuously across snapshot/deltas/StateRoot-check
// instead of releasing and reacquiring it once per call — see
// replayTransactions' own comment for the race that isolation closes).
//
// FIX (audit 2026-06-28 recheck 5, P1-1): this used to write via cs.db
// directly instead of cs.dbExec(), and returned only a bool. Both
// mattered: when called from inside replayTransactions (which sets
// cs.activeTx before running any TX), the INSERT committed immediately
// and permanently, completely independent of the surrounding replay
// transaction — if a LATER TX in that same block then hard-failed or the
// block's StateRoot mismatched, the whole block got rolled back, but this
// nullifier row had already auto-committed and stayed in the DB,
// potentially leaving a human permanently unable to register ("already
// registered" with no real registration behind it) if the compensating
// releaseNullifierLocked call ever failed too. Now routes through
// cs.dbExec(), so inside replay this INSERT joins dbTx and is
// automatically discarded by the same ROLLBACK that undoes everything
// else in a rejected block — no separate compensation needed for that
// path specifically (releaseNullifierLocked's own DELETE remains the
// real compensation mechanism for callers outside any active
// transaction, e.g. the mirror-path fallback in register.go).
// Also now returns an error so a genuine DB failure during the claim is
// never silently treated as "already used" by a caller checking just
// the bool — see replayTransactions' own fix at its call site.
func (cs *ChainState) tryClaimNullifierLocked(ctx context.Context, nullifier, walletAddress string) (bool, error) {
	if nullifier == "" {
		return false, nil
	}
	walletAddress = strings.ToLower(walletAddress)
	if cs.db == nil {
		if _, exists := cs.nullifiers[nullifier]; exists {
			return false, nil
		}
		cs.nullifiers[nullifier] = walletAddress
		xorInto(&cs.nullifierSetXOR, nullifierLeaf(nullifier)) // fold new key into state-root accumulator
		return true, nil
	}
	res, err := cs.dbExecCtx(ctx).Exec(
		`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ($1, $2) ON CONFLICT (nullifier) DO NOTHING`,
		nullifier, walletAddress,
	)
	if err != nil {
		return false, fmt.Errorf("could not claim nullifier: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil // already existed
	}
	// Insert genuinely happened — fold the key into the nullifier accumulator
	// (see ChainState.nullifierSetXOR). Done regardless of the in-memory cap
	// below, because the accumulator commits to EVERY nullifier, not just the
	// cached subset.
	xorInto(&cs.nullifierSetXOR, nullifierLeaf(nullifier))
	// Insert succeeded — update in-memory cache.
	if len(cs.nullifiers) < maxInMemNullifiers {
		cs.nullifiers[nullifier] = walletAddress
	}
	return true, nil
}

// SaveNullifier records nullifier as used. Caller must already hold cs.mu
// (it mutates cs.nullifiers directly, like the other "Locked"-style
// helpers in this file) — see RegisterHumanAtomic's closure for the
// expected call site.
//
// FIX (audit recheck 2, P1 #7/#10): this used to be void and use cs.db
// directly, called as a separate, non-atomic step AFTER
// RegisterHumanAtomic's transaction had already committed (register.go).
// A failure here — or a crash between the two calls — left Go-state and
// the outbox correct while StateRoot (which hashes the sorted set of
// nullifier keys) had no record of this nullifier, a permanent
// inconsistency no later retry could fix (the registration itself had
// already succeeded). Now returns an error and uses cs.dbExecCtx(ctx), so
// when called from inside RegisterHumanAtomic's fn() closure (which builds
// ctx from cs.activeTx for that call), this write commits or rolls back
// together with the account mutation and the outbox insert as one DB
// transaction. See dbExecCtx's own comment for the migration this is part
// of.
func (cs *ChainState) SaveNullifier(ctx context.Context, nullifier, walletAddress string) error {
	if nullifier == "" {
		return nil
	}
	walletAddress = strings.ToLower(walletAddress)
	if cs.db == nil {
		// No-DB mode: the map is authoritative. Fold into the accumulator only
		// when the key is genuinely new so repeated saves can't double-count.
		// nullifiersMu: see that field's own comment.
		cs.nullifiersMu.Lock()
		if _, exists := cs.nullifiers[nullifier]; !exists {
			cs.nullifiers[nullifier] = walletAddress
			xorInto(&cs.nullifierSetXOR, nullifierLeaf(nullifier))
		}
		cs.nullifiersMu.Unlock()
		return nil
	}
	res, err := cs.dbExecCtx(ctx).Exec(
		`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ($1, $2) ON CONFLICT (nullifier) DO NOTHING`,
		nullifier, walletAddress,
	)
	if err != nil {
		return fmt.Errorf("could not persist nullifier: %w", err)
	}
	// Fold into the nullifier accumulator only on a genuine insert (RowsAffected
	// > 0), never on a conflict no-op — otherwise a nullifier already counted by
	// tryClaimNullifierLocked would be XORed in a second time and cancel itself.
	// nullifiersMu guards this mutation (see that field's own comment) --
	// uncontended/free for every existing cs.mu.Lock()-based caller
	// (RegisterHumanAtomic), the actual synchronization for
	// registerHumanConcurrent (register_concurrent.go), which only holds
	// cs.mu.RLock().
	if n, _ := res.RowsAffected(); n > 0 {
		cs.nullifiersMu.Lock()
		xorInto(&cs.nullifierSetXOR, nullifierLeaf(nullifier))
		cs.nullifiersMu.Unlock()
	} else {
		// SECURITY (P0, launch audit 2026-07-03): RowsAffected==0 means a row
		// for this nullifier already existed. SaveNullifier's callers
		// (RegisterHumanAtomic, registerHumanConcurrent) reach this line
		// after already confirming walletAddress itself isn't registered
		// yet — so an existing row here can only belong to a DIFFERENT
		// wallet. Returning nil in that case used to let the caller's
		// outbox transaction commit anyway: a second wallet walks away with
		// a second 1,000 AEQ grant sharing the first wallet's nullifier,
		// while the nullifiers table quietly keeps pointing at the first
		// wallet only (double-mint of the same biometric). Fail closed so
		// the caller rolls back the whole registration instead of silently
		// dropping just the nullifier row.
		var existingWallet string
		if scanErr := cs.dbExecCtx(ctx).QueryRow(
			`SELECT wallet_address FROM nullifiers WHERE nullifier = $1`, nullifier,
		).Scan(&existingWallet); scanErr != nil {
			return fmt.Errorf("nullifier conflict but could not verify existing owner: %w", scanErr)
		}
		if existingWallet != walletAddress {
			return fmt.Errorf("nullifier already used by a different wallet")
		}
	}
	cs.nullifiersMu.Lock()
	if len(cs.nullifiers) < maxInMemNullifiers {
		cs.nullifiers[nullifier] = walletAddress
	}
	cs.nullifiersMu.Unlock()
	return nil
}

// ReleaseNullifier undoes a TryClaimNullifier claim. Used when a
// registration that successfully claimed a nullifier later fails for an
// unrelated reason (invalid signature, write error, etc.) — without this,
// the nullifier would be permanently consumed and the legitimate human
// behind it could never register again with a fresh attempt.
func (cs *ChainState) ReleaseNullifier(nullifier string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// cs.mu-only path, never runs inside runAtomicWithOutbox — see
	// RegisterHuman's comment.
	cs.releaseNullifierLocked(context.Background(), nullifier)
}

// releaseNullifierLocked is ReleaseNullifier's body, for callers that
// already hold cs.mu — see tryClaimNullifierLocked's comment. Block replay
// (block.go) also calls this directly with context.Background(): it sets
// dag.state.activeTx itself before this runs, and dbExecCtx falls back to
// that field when ctx carries no transaction, so behavior there is
// unchanged — see registerHumanLocked's comment for the same reasoning.
func (cs *ChainState) releaseNullifierLocked(ctx context.Context, nullifier string) {
	if nullifier == "" {
		return
	}
	_, wasCached := cs.nullifiers[nullifier]
	delete(cs.nullifiers, nullifier)
	if cs.db == nil {
		// No-DB mode: the map was authoritative, so only reverse the accumulator
		// if this key was actually present (XOR is self-inverse — XORing out a
		// key that was never in would wrongly fold it IN).
		if wasCached {
			xorInto(&cs.nullifierSetXOR, nullifierLeaf(nullifier))
		}
		return
	}
	// FIX (audit 2026-06-28 recheck 5, P1-1): routes through cs.dbExecCtx(ctx)
	// like tryClaimNullifierLocked now does — when called from inside
	// replayTransactions this DELETE joins the same dbTx as the claim it's
	// undoing (so it's redundant-but-harmless there, since a ROLLBACK
	// would discard the claim anyway), and stays the real, separate
	// compensating action for callers outside any active transaction
	// (e.g. the mirror-path fallback in register.go).
	res, err := cs.dbExecCtx(ctx).Exec(`DELETE FROM nullifiers WHERE nullifier = $1`, nullifier)
	if err != nil {
		fmt.Printf("[NULLIFIER] Warning: could not release nullifier %s: %v\n", nullifier, err)
		return
	}
	// Reverse the accumulator only when a row was actually deleted, so a
	// release of an already-absent nullifier can't fold a phantom key in.
	if n, _ := res.RowsAffected(); n > 0 {
		xorInto(&cs.nullifierSetXOR, nullifierLeaf(nullifier))
	}
}

func (cs *ChainState) GetWalletByNullifier(nullifier string) string {
	cs.mu.RLock()
	w, ok := cs.nullifiers[nullifier]
	cs.mu.RUnlock()
	if ok {
		return w
	}
	if cs.db == nil {
		return ""
	}
	var wallet string
	cs.db.QueryRow(`SELECT wallet_address FROM nullifiers WHERE nullifier = $1`, nullifier).Scan(&wallet)
	return wallet
}

// ─── PRICE HISTORY ───────────────────────────────────────────────────────────

// priceHistoryRetentionDays bounds both how long price_snapshots rows are
// kept and the maximum window GetPriceHistory will ever query (see its
// minutesClamp below, derived from this same constant so the two can't
// drift apart the way the chart's old 30-day retention vs. its frontend's
// 10-day preload window had). 366 days comfortably covers a "1y" chart
// interval with a day of slack, and gives "all" a real, non-trivial window
// instead of silently capping at whatever the retention used to be (30 days
// — launch audit 2026-07-03: too short for any of 1mo/3mo/1y/all to show
// meaningful data at all, regardless of what the frontend requested).
const priceHistoryRetentionDays = 366

func (cs *ChainState) InitPriceSnapshotsTable() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS price_snapshots (
		id           SERIAL PRIMARY KEY,
		price        DOUBLE PRECISION NOT NULL,
		reserve_aeq  DOUBLE PRECISION NOT NULL,
		reserve_tusd DOUBLE PRECISION NOT NULL,
		captured_at  TIMESTAMP DEFAULT NOW()
	)`)
	// FIX (launch audit 2026-07-03): every query against this table filters
	// and sorts on captured_at (see GetPriceHistory) with no index backing
	// it — a full table scan + sort on every /api/price-history call, and
	// this table has no per-row cap (SavePriceSnapshot runs on a fixed
	// timer, not bounded by row count), so cost only grows with uptime.
	cs.db.Exec(`CREATE INDEX IF NOT EXISTS idx_price_snapshots_captured_at ON price_snapshots (captured_at)`)
	cs.purgeOldPriceSnapshots()

	// FIX (P2, launch audit 2026-07-03): this purge used to run exactly
	// once, at process startup — a node that stays up for a long time (the
	// entire point of running a validator) accumulated snapshots forever
	// past the retention window between restarts, with the query-side clamp
	// providing no relief since it only bounds what's SERVED, not what's
	// stored. Re-run it periodically for the lifetime of the process.
	SafeGoroutine("purgeOldPriceSnapshots-ticker", func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			// FIX (P0-3, beta-launch audit 2026-07-05): recover per-tick — see safeCall's comment.
			SafeCall("purgeOldPriceSnapshots-tick", cs.purgeOldPriceSnapshots)
		}
	})
}

// purgeOldPriceSnapshots deletes price_snapshots rows older than the
// retention window. Safe to call anytime (no lock needed — cs.db has its
// own internal connection pooling/locking).
//
// Uses ($1 * INTERVAL '1 day') rather than string-formatting the day count
// into the query, matching GetPriceHistory's own P1-11 pattern — even
// though priceHistoryRetentionDays is a compile-time constant here, not
// user input, staying consistent avoids ever normalizing string-built
// interval clauses in this file.
func (cs *ChainState) purgeOldPriceSnapshots() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`DELETE FROM price_snapshots WHERE captured_at < NOW() - ($1 * INTERVAL '1 day')`, priceHistoryRetentionDays)
}

// SavePriceSnapshot records the current AEQ/tUSD price. Must be safe to call
// concurrently — copies pool values under RLock before the DB write so a
// concurrent swap cannot modify cs.pool while we're reading it.
func (cs *ChainState) SavePriceSnapshot() {
	if cs.db == nil {
		return
	}
	cs.mu.RLock()
	if cs.pool == nil || cs.pool.ReserveAEQ <= 0 || cs.pool.ReserveTUSD <= 0 {
		cs.mu.RUnlock()
		return
	}
	price := cs.pool.ReserveTUSD.Float() / cs.pool.ReserveAEQ.Float()
	aeq := cs.pool.ReserveAEQ.Float()
	tusd := cs.pool.ReserveTUSD.Float()
	cs.mu.RUnlock()
	// FIX (2026-07-05 — chart intervals silently going stale): this insert's
	// error was previously discarded entirely. Confirmed live: Primary's
	// price_snapshots simply stopped gaining new rows for 14+ hours (last
	// row from 2026-07-04 11:25) while Contabo 1's own snapshots kept
	// saving fine over the same window — the exact asymmetry a silently
	// swallowed per-node DB error would produce, and one this codebase had
	// no way to ever surface. Log so a recurrence is visible instead of
	// only showing up as "the chart looks flat/stale" days later.
	if _, err := cs.db.Exec(`INSERT INTO price_snapshots (price, reserve_aeq, reserve_tusd) VALUES ($1, $2, $3)`,
		price, aeq, tusd); err != nil {
		fmt.Printf("[PRICE] ✗ SavePriceSnapshot insert failed: %v\n", err)
	}
}

// GetPriceHistory returns price snapshots from the last `minutes` minutes,
// limited to `limit` points, oldest first (ready for a chart to plot
// directly). Returns [{t, p, aeq, tusd}, ...].
// minutes is clamped to 1-(priceHistoryRetentionDays*1440), limit to 1-5000.
func (cs *ChainState) GetPriceHistory(minutes, limit int) []map[string]interface{} {
	if cs.db == nil {
		return nil
	}
	if minutes < 1 {
		minutes = 1
	}
	// FIX (launch audit 2026-07-03): was hard-capped at 43200 (30 days),
	// making 1mo/3mo/1y/all chart intervals architecturally impossible no
	// matter what the frontend asked for. Clamped to the same window the
	// data is actually retained for (see priceHistoryRetentionDays) instead
	// of an arbitrary shorter constant.
	maxMinutes := priceHistoryRetentionDays * 24 * 60
	if minutes > maxMinutes {
		minutes = maxMinutes
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 5000 {
		limit = 5000
	}
	// P1-11: use ($1 * INTERVAL '1 minute') instead of string concat to
	// prevent any future SQL-injection if $1 type changes to string.
	//
	// FIX (launch audit 2026-07-03): the old query was
	// "ORDER BY captured_at ASC LIMIT $2" directly — when a window contains
	// more rows than the limit, ASC+LIMIT keeps the OLDEST rows in the
	// window and silently drops everything more recent, which is exactly
	// backwards for a live price chart (the whole point of "last N minutes"
	// is to see up to now). Select the newest `limit` rows first (DESC),
	// then re-sort ASC in the outer query so the result is still
	// oldest-first for the chart, same as before this fix.
	rows, err := cs.db.Query(`
		SELECT * FROM (
			SELECT EXTRACT(EPOCH FROM captured_at)::BIGINT AS ts, price, reserve_aeq, reserve_tusd, captured_at
			FROM price_snapshots
			WHERE captured_at >= NOW() - ($1 * INTERVAL '1 minute')
			ORDER BY captured_at DESC
			LIMIT $2
		) recent
		ORDER BY captured_at ASC`, minutes, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var ts int64
		var price, aeq, tusd float64
		var capturedAt time.Time // outer ORDER BY column only, unused otherwise
		rows.Scan(&ts, &price, &aeq, &tusd, &capturedAt)
		result = append(result, map[string]interface{}{
			"t": ts * 1000, // milliseconds for JS Date
			"p": price,
			"a": aeq,
			"u": tusd,
		})
	}
	return result
}

// ─── GINI HISTORY ────────────────────────────────────────────────────────────

func (cs *ChainState) InitGiniSnapshotsTable() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS gini_snapshots (
		id          SERIAL PRIMARY KEY,
		gini        DOUBLE PRECISION NOT NULL,
		humans      INT NOT NULL,
		captured_at TIMESTAMP DEFAULT NOW()
	)`)
}

// SaveGiniSnapshot persists the current Gini coefficient. Called after each
// UBI distribution so the history chart has real data points over time.
// Must NOT be called while cs.mu is held — CalcGini acquires RLock internally.
func (cs *ChainState) SaveGiniSnapshot() {
	if cs.db == nil {
		return
	}
	gini := cs.CalcGini()      // acquires RLock
	humans := cs.TotalHumans() // acquires RLock
	cs.db.Exec(`INSERT INTO gini_snapshots (gini, humans) VALUES ($1, $2)`, gini, humans)
}

// SaveGiniSnapshotValues saves a pre-computed Gini/humans pair without
// acquiring any lock. Call this from inside a locked function by passing
// values already read under the lock, to avoid lock-reentrancy deadlocks.
func (cs *ChainState) SaveGiniSnapshotValues(gini float64, humans int) {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`INSERT INTO gini_snapshots (gini, humans) VALUES ($1, $2)`, gini, humans)
}

// GetGiniHistory returns the last n Gini snapshots in chronological order.
// Returns a slice of maps with keys: idx (0-100), gini (0-1), humans, timestamp.
func (cs *ChainState) GetGiniHistory(n int) []map[string]interface{} {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(
		`SELECT gini, humans, EXTRACT(EPOCH FROM captured_at)::BIGINT
		 FROM gini_snapshots ORDER BY captured_at DESC LIMIT $1`, n)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var gini float64
		var humans int
		var ts int64
		rows.Scan(&gini, &humans, &ts)
		result = append(result, map[string]interface{}{
			"idx":       gini * 100,
			"gini":      gini,
			"humans":    humans,
			"timestamp": ts,
		})
	}
	// Reverse to get chronological order (we queried DESC).
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// ─── SWAP NONCES ─────────────────────────────────────────────────────────────
//
// Each wallet has a monotonically increasing nonce for swap/liquidity actions.
// The nonce is included in the signed message, so a captured signature cannot
// be replayed — the nonce check atomically rejects any second use.

// ─── VALIDATOR KEY REGISTRY ──────────────────────────────────────────────────
//
// Replaces the shared PEER_SECRET model with individual, human-authorized
// validator keys. Each node operator signs their signing key with their
// registered human wallet, creating a 1:1 link: "this human authorizes
// this signing key to produce blocks on their behalf."
//
// A compromised node key can be revoked individually without affecting any
// other validator. Authorization is tied to on-chain human identity.

func (cs *ChainState) InitValidatorKeysTable() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS validator_keys (
		signing_address TEXT PRIMARY KEY,
		human_wallet    TEXT NOT NULL UNIQUE,
		registered_at   TIMESTAMP DEFAULT NOW()
	)`)
	// Add UNIQUE on human_wallet if the table already existed without it.
	// Remove any existing duplicates first so the index creation succeeds.
	cs.db.Exec(`DELETE FROM validator_keys vk1
		USING validator_keys vk2
		WHERE vk1.registered_at < vk2.registered_at
		  AND vk1.human_wallet = vk2.human_wallet`)
	if _, err := cs.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_validator_keys_human_wallet
		ON validator_keys (human_wallet)`); err != nil {
		// N2 fix: mark node degraded instead of silently continuing. A missing
		// UNIQUE constraint on human_wallet means one wallet can hold multiple
		// signing keys and reward distribution can be counted twice. This is
		// surfaced in /api/health/combined via SetBootstrapDegraded.
		reason := fmt.Sprintf("validator_keys UNIQUE(human_wallet) index could not be created — duplicate validator key bindings possible, reward distribution may be incorrect: %v", err)
		Log.Error(reason)
		cs.SetBootstrapDegraded(reason)
	}
}

// RegisterValidatorKey links a node signing address to a registered human
// wallet, authorizing that signing key to propose blocks. The human_wallet
// must be a registered human; the signature must be a valid personal_sign
// of "Aequitas: authorize validator key {signing_address}".
func (cs *ChainState) RegisterValidatorKey(signingAddress, humanWallet string) error {
	if cs.db == nil {
		return fmt.Errorf("no database")
	}
	signingAddress = strings.ToLower(strings.TrimSpace(signingAddress))
	humanWallet = strings.ToLower(strings.TrimSpace(humanWallet))
	if !cs.IsHuman(humanWallet) {
		return fmt.Errorf("human_wallet %s is not a registered human", humanWallet)
	}
	_, err := cs.db.Exec(
		`INSERT INTO validator_keys (signing_address, human_wallet) VALUES ($1, $2)
		 ON CONFLICT (signing_address) DO UPDATE SET human_wallet = $2, registered_at = NOW()`,
		signingAddress, humanWallet)
	return err
}

// LoadValidatorKeysIntoDAG reads all registered validator signing addresses
// from the DB and adds them to the DAG's authorized validators set.
// Called at startup so keys registered before the node restarted are effective.
//
// FIX (audit recheck2, P1 #8): this used to read ONLY validator_keys —
// validator_slots (the wallet-bound, signature-verified binding BindValidatorSlot
// writes, the mechanism this project's Sybil-resistance redesign actually
// relies on) was never reloaded here. A validator authorized purely through
// the BindValidatorSlot/handlePeerRegister flow (AddAuthorizedValidator
// called in-memory at bind time, never via RegisterValidatorKey) lost its
// block-signing authorization on every single restart — it would have to
// re-bind (re-sign and re-submit) before it could propose another block,
// even though its binding was still valid and present in validator_slots
// the whole time. Now loads both tables; either one authorizing a signing
// address is sufficient, matching handlePeerRegister's own "PEER_SECRET OR
// signature" acceptance logic.
func (cs *ChainState) LoadValidatorKeysIntoDAG(dag interface{ AddAuthorizedValidator(string) }) {
	if cs.db == nil {
		return
	}
	// P2-08: log errors instead of silently returning a partial result.
	rows, err := cs.db.Query(`SELECT signing_address FROM validator_keys`)
	if err != nil {
		fmt.Printf("[VALIDATORS] ⚠ LoadValidatorKeysIntoDAG: validator_keys query failed: %v\n", err)
	} else {
		for rows.Next() {
			var addr string
			rows.Scan(&addr)
			dag.AddAuthorizedValidator(strings.ToLower(strings.TrimSpace(addr)))
		}
		rows.Close()
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS validator_slots (
operator_wallet TEXT PRIMARY KEY,
signing_address TEXT NOT NULL,
claimed_at TIMESTAMP DEFAULT NOW()
)`)
	slotRows, err := cs.db.Query(`SELECT signing_address FROM validator_slots`)
	if err != nil {
		fmt.Printf("[VALIDATORS] ⚠ LoadValidatorKeysIntoDAG: validator_slots query failed: %v\n", err)
		return
	}
	defer slotRows.Close()
	for slotRows.Next() {
		var addr string
		slotRows.Scan(&addr)
		dag.AddAuthorizedValidator(strings.ToLower(strings.TrimSpace(addr)))
	}
}

// GetAllRegisteredValidatorAddresses returns all distinct signing addresses
// from both validator_keys and validator_slots, sorted deterministically.
// Used by the epoch-committee selector in computeEpochCommittee.
func (cs *ChainState) GetAllRegisteredValidatorAddresses() []string {
	if cs.db == nil {
		return nil
	}
	seen := make(map[string]bool)
	var addrs []string
	for _, q := range []string{
		`SELECT lower(signing_address) FROM validator_keys`,
		`SELECT lower(signing_address) FROM validator_slots`,
	} {
		rows, err := cs.db.Query(q)
		if err != nil {
			continue
		}
		for rows.Next() {
			var addr string
			rows.Scan(&addr)
			addr = strings.TrimSpace(addr)
			if addr != "" && !seen[addr] {
				seen[addr] = true
				addrs = append(addrs, addr)
			}
		}
		rows.Close()
	}
	sort.Strings(addrs) // deterministic ordering for committee selection
	return addrs
}

// GetValidatorKeyPairsForSync returns (signing_address, human_wallet) pairs
// from both validator_keys and validator_slots, deduplicated by signing_address.
// Used by /api/validators so receiving peers can verify the human_wallet is
// a registered human before trusting the signing key (P1-04 audit fix).
func (cs *ChainState) GetValidatorKeyPairsForSync() []ValidatorKeyPair {
	if cs.db == nil {
		return nil
	}
	seen := make(map[string]bool)
	var pairs []ValidatorKeyPair

	// P2-08: log DB errors instead of silently returning a partial result.
	rows, err := cs.db.Query(`SELECT signing_address, human_wallet FROM validator_keys ORDER BY registered_at`)
	if err != nil {
		fmt.Printf("[VALIDATORS] ⚠ GetValidatorKeyPairsForSync: validator_keys query failed: %v\n", err)
	} else {
		for rows.Next() {
			var addr, wallet string
			rows.Scan(&addr, &wallet)
			addr = strings.ToLower(strings.TrimSpace(addr))
			wallet = strings.ToLower(strings.TrimSpace(wallet))
			if addr != "" && !seen[addr] {
				seen[addr] = true
				pairs = append(pairs, ValidatorKeyPair{SigningAddress: addr, HumanWallet: wallet})
			}
		}
		rows.Close()
	}

	// P1-03: include binding_signature (may be absent on older rows — COALESCE
	// to empty string for backward compatibility).
	slotRows, err := cs.db.Query(`SELECT signing_address, operator_wallet, COALESCE(binding_signature,'') FROM validator_slots`)
	if err != nil {
		fmt.Printf("[VALIDATORS] ⚠ GetValidatorKeyPairsForSync: validator_slots query failed: %v\n", err)
	} else {
		for slotRows.Next() {
			var addr, wallet, bindingSig string
			slotRows.Scan(&addr, &wallet, &bindingSig)
			addr = strings.ToLower(strings.TrimSpace(addr))
			wallet = strings.ToLower(strings.TrimSpace(wallet))
			if addr != "" && !seen[addr] {
				seen[addr] = true
				pairs = append(pairs, ValidatorKeyPair{SigningAddress: addr, HumanWallet: wallet, OperatorBindingSignature: bindingSig})
			}
		}
		slotRows.Close()
	}

	return pairs
}

func (cs *ChainState) GetValidatorKeys() []map[string]string {
	if cs.db == nil {
		return nil
	}
	rows, err := cs.db.Query(`SELECT signing_address, human_wallet FROM validator_keys ORDER BY registered_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []map[string]string
	for rows.Next() {
		var addr, wallet string
		rows.Scan(&addr, &wallet)
		result = append(result, map[string]string{"signing_address": addr, "human_wallet": wallet})
	}
	return result
}

// ValidateNodeOperatorWallet returns an error string if the wallet is not a
// registered human. The calling code must STOP registration if this returns
// non-empty — rewards go only to verified humans, no exceptions.
func (cs *ChainState) ValidateNodeOperatorWallet(wallet string) string {
	if !cs.IsHuman(strings.ToLower(wallet)) {
		return "wallet " + wallet + " is NOT a registered human — register via the Android app first before running a node"
	}
	return ""
}

// InitSwapNoncesTable creates the swap_nonces table if it doesn't exist.
func (cs *ChainState) InitSwapNoncesTable() {
	if cs.db == nil {
		return
	}
	cs.db.Exec(`CREATE TABLE IF NOT EXISTS swap_nonces (
		wallet_address TEXT PRIMARY KEY,
		next_nonce     BIGINT NOT NULL DEFAULT 0
	)`)
}

// GetSwapNonce returns the next nonce a wallet should sign with.
// Returns 0 for wallets that have never performed a swap.
func (cs *ChainState) GetSwapNonce(wallet string) int64 {
	if cs.db == nil {
		return 0
	}
	wallet = strings.ToLower(wallet)
	var nonce int64
	cs.db.QueryRow(`SELECT next_nonce FROM swap_nonces WHERE wallet_address = $1`, wallet).Scan(&nonce)
	return nonce
}

// RestoreSwapNonce decrements the nonce back to its pre-swap value when a
// swap fails after the nonce was already consumed. Safe to call: if the nonce
// has already advanced past nonce+1 (extremely unlikely concurrent case) the
// UPDATE finds no rows and the decrement is skipped — user must re-sign.
func (cs *ChainState) RestoreSwapNonce(wallet string, nonce int64) {
	if cs.db == nil {
		return
	}
	wallet = strings.ToLower(wallet)
	cs.db.Exec(`UPDATE swap_nonces SET next_nonce = $2 WHERE wallet_address = $1 AND next_nonce = $2 + 1`,
		wallet, nonce)
}

// ConsumeSwapNonce atomically verifies that nonce matches the expected value
// and increments it. Returns an error if the nonce doesn't match (replay or
// wrong value). Must be called only after the signature has been verified.
func (cs *ChainState) ConsumeSwapNonce(wallet string, nonce int64) error {
	if cs.db == nil {
		return nil // no DB — skip in development
	}
	wallet = strings.ToLower(wallet)
	var result interface{ RowsAffected() (int64, error) }
	var err error
	if nonce == 0 {
		// First-ever swap for this wallet, OR a retry after RestoreSwapNonce
		// put next_nonce back to 0 (the first attempt consumed the nonce but
		// failed afterward, e.g. insufficient balance in SwapAtomic). Plain
		// "ON CONFLICT DO NOTHING" only covers the true-first-ever case — if
		// the row already exists, the INSERT is always a no-op regardless of
		// the row's actual value, which permanently locked out any wallet
		// whose first swap ever failed after the nonce was consumed (every
		// later nonce:0 retry hit "already used" forever, even though the
		// row's next_nonce had correctly been restored to 0). The WHERE
		// clause makes this behave like the nonce!=0 branch below: it only
		// succeeds if the row is actually sitting at 0 right now.
		result, err = cs.db.Exec(
			`INSERT INTO swap_nonces (wallet_address, next_nonce) VALUES ($1, 1)
			 ON CONFLICT (wallet_address) DO UPDATE SET next_nonce = 1
			 WHERE swap_nonces.next_nonce = 0`, wallet)
	} else {
		// Subsequent swap — increment only if current value matches.
		result, err = cs.db.Exec(
			`UPDATE swap_nonces SET next_nonce = next_nonce + 1
			 WHERE wallet_address = $1 AND next_nonce = $2`, wallet, nonce)
	}
	if err != nil {
		return fmt.Errorf("nonce db error: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("nonce %d already used or invalid — replay rejected", nonce)
	}
	return nil
}

// ─── EVM TX RECEIPTS (persistent — survives node restart) ────────────────────

// SaveTxReceipt persists an EVM transaction receipt to the database so MetaMask
// can retrieve it after a node restart. Without this, restarts cleared the
// in-memory txStatus map and MetaMask would show successful transactions as
// "Senden fehlgeschlagen" (failed) because receipts returned null.
// SaveTxReceipt persists an EVM transaction receipt. contractAddr is the
// deployed contract's address for a deployment TX, or "" for everything else
// — passing it through means getTransactionReceipt can still report
// "contractAddress" correctly after a node restart, when it falls back to
// this DB-persisted row instead of the in-memory-only deployedContracts map.
func (cs *ChainState) SaveTxReceipt(txHash, fromAddr, toAddr, status, contractAddr string) {
	if cs.db == nil {
		return
	}
	// Buffered, not written: see receipt_flush.go for the profile that made
	// this the last per-transfer Postgres round trip on the request path, and
	// for why deferring it is safe (getTransactionReceipt answers from the
	// in-memory maps first, and GetTxReceipt below checks the buffer before
	// the database). created_at is captured HERE, not at flush time, so the
	// stored timestamp keeps meaning "when the transaction happened".
	cs.bufferTxReceipt(pendingReceipt{
		txHash:       strings.ToLower(txHash),
		fromAddr:     strings.ToLower(fromAddr),
		toAddr:       strings.ToLower(toAddr),
		status:       status,
		contractAddr: strings.ToLower(contractAddr),
		createdAt:    time.Now().Unix(),
	})
	cs.maybePruneTxReceipts()
}

// receiptPruneInterval is the minimum gap between receipt-table prunes.
//
// FIX (P0 for throughput, 2026-07-25 — found by profiling Contabo2 while it
// was under load, not by reading the code): the prune below used to run
// INLINE, on the request path, for EVERY transaction:
//
//	DELETE FROM evm_tx_receipts WHERE tx_hash NOT IN (
//	    SELECT tx_hash FROM evm_tx_receipts ORDER BY created_at DESC LIMIT 10000)
//
// That is a full-table anti-join with a sort. Running it per transaction means
// Postgres re-sorted the entire receipts table on every single transfer, to
// delete rows that were already deleted the previous time — 150 times a second
// at the throughput actually observed.
//
// The 30s CPU profile taken during a live load run, cumulative:
//
//	sendRawTransaction        10.18s  55.63%
//	  database/sql.(*DB).Exec  5.02s  27.43%   <- two Execs per transfer
//	    lib/pq.(*conn).Exec    4.72s  25.79%
//	  types.Sender/Ecrecover   3.21s  17.54%
//	database/sql.withLock      5.25s  28.69%   <- pooled-connection serialisation
//
// and total samples were 18.30s over 30.10s wall, i.e. ~0.61 cores. The node
// was not computing, it was waiting: two synchronous Postgres round trips per
// transfer, one of them expensive, with every request queueing for a pooled
// connection behind them.
//
// Correctness is unchanged — the cap is still 10,000 rows. Only the CADENCE
// changes: the table is allowed to drift a little above the cap between
// prunes, which costs a bounded amount of disk and nothing else, instead of
// paying a sort-the-world query per transaction to hold it exactly. Nothing
// reads evm_tx_receipts expecting a precise row count; GetTxReceipt looks up
// one tx_hash.
const receiptPruneInterval = 60 * time.Second

// receiptPruneKeep is how many receipts to retain — unchanged from the inline
// version this replaced.
const receiptPruneKeep = 10000

var (
	receiptPruneLastAt  atomic.Int64 // unix seconds
	receiptPruneRunning atomic.Bool
)

// maybePruneTxReceipts runs the receipt prune at most once per
// receiptPruneInterval, in the background, and never more than one at a time.
//
// Background rather than inline because the caller is an RPC request handler:
// making a transfer's latency depend on how long a housekeeping DELETE takes
// is what the profile above caught. Single-flight because a slow prune must
// not have a second one queued behind it — that would reproduce the pile-up
// this fix exists to remove, just at a coarser interval.
func (cs *ChainState) maybePruneTxReceipts() {
	// Guarded here as well as in SaveTxReceipt: relying on SafeGoroutine to
	// recover a nil-db panic works, but it writes a full stack trace into the
	// log for a condition that is entirely ordinary in tests and in a node
	// started without a database.
	if cs.db == nil {
		return
	}
	now := time.Now().Unix()
	last := receiptPruneLastAt.Load()
	if now-last < int64(receiptPruneInterval.Seconds()) {
		return
	}
	// CompareAndSwap, not a plain Store: several concurrent transfers reach
	// this line in the same second, and exactly one of them should win.
	if !receiptPruneLastAt.CompareAndSwap(last, now) {
		return
	}
	if !receiptPruneRunning.CompareAndSwap(false, true) {
		return
	}
	SafeGoroutine("pruneTxReceipts", func() {
		defer receiptPruneRunning.Store(false)
		if _, err := cs.db.Exec(`DELETE FROM evm_tx_receipts WHERE tx_hash NOT IN (
			SELECT tx_hash FROM evm_tx_receipts ORDER BY created_at DESC LIMIT $1
		)`, receiptPruneKeep); err != nil {
			fmt.Printf("[EVM] receipt prune failed: %v — retrying at the next interval\n", err)
		}
	})
}

// GetTxReceipt looks up a persisted receipt. Returns (fromAddr, toAddr, status, contractAddr, found).
// Called by getTransactionReceipt/getTransactionByHash when the txHash is not in the in-memory cache.
func (cs *ChainState) GetTxReceipt(txHash string) (fromAddr, toAddr, status, contractAddr string, found bool) {
	if cs.db == nil {
		return "", "", "", "", false
	}
	// Check the not-yet-flushed buffer first. Without this, a receipt written
	// less than receiptFlushInterval ago would read as "not found" from here
	// even though it exists -- turning an internal batching detail into a
	// visible inconsistency for any caller that reaches this fallback.
	if r, ok := cs.lookupBufferedReceipt(txHash); ok {
		return r.fromAddr, r.toAddr, r.status, r.contractAddr, true
	}
	err := cs.db.QueryRow(
		`SELECT from_addr, COALESCE(to_addr, ''), status, COALESCE(contract_addr, '') FROM evm_tx_receipts WHERE tx_hash = $1`,
		strings.ToLower(txHash),
	).Scan(&fromAddr, &toAddr, &status, &contractAddr)
	if err == sql.ErrNoRows || err != nil {
		return "", "", "", "", false
	}
	return fromAddr, toAddr, status, contractAddr, true
}

// ─── PENDING TXs (persistent — survive node restart) ─────────────────────────

// SavePendingTx writes a Transaction to the DB so it survives node restarts.
// ProduceBlock calls LoadPendingTxs/ClearPendingTxs to drain these and
// include them in the next block, ensuring secondary nodes receive every
// state change.
// FIX: now returns error. By the time any caller invokes this, the
// underlying state change has already been applied and committed locally —
// there is nothing left to roll back. A failure here means no other node
// will ever learn about that change (permanent divergence), so callers must
// at minimum surface this loudly rather than silently continue. Returning
// the error lets each caller decide how (most just log an [ALERT] today,
// and fall back to the in-memory-only AddTransaction queue as a
// best-effort second chance — see those call sites).
//
// FIX (durability): retries with a short backoff before giving up. The
// realistic failure mode here is a transient DB hiccup — if the connection
// were durably down, the state mutation that already happened just before
// this call (in the same DB) would itself have failed too, so SavePendingTx
// failing in isolation right after a successful state write is almost
// always a brief blip. Retrying in-process closes that gap automatically
// instead of requiring it to surface as a permanent divergence every time.
func (cs *ChainState) SavePendingTx(tx Transaction) error {
	if cs.db == nil {
		return fmt.Errorf("no DB configured — pending TX outbox unavailable")
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := savePendingTxExec(cs.db, tx); err != nil {
			lastErr = err
			fmt.Printf("[TX] SavePendingTx db error (attempt %d/3): %v\n", attempt, err)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
			}
			continue
		}
		return nil
	}
	return lastErr
}

// savePendingTxExec inserts tx via the given executor (cs.db or an active
// transaction) with no retry — retrying individual statements inside an
// already-failed SQL transaction doesn't make sense (Postgres aborts the
// whole transaction on the first error until rolled back), so retry policy
// belongs to the caller that owns the executor's lifetime: SavePendingTx
// retries because it owns cs.db directly; runAtomicWithOutbox does not,
// because a failure here means the whole atomic operation rolls back.
func savePendingTxExec(ex sqlExecutor, tx Transaction) error {
	data, err := json.Marshal(tx)
	if err != nil {
		fmt.Printf("[TX] SavePendingTx marshal error: %v\n", err)
		return err
	}
	_, err = ex.Exec(`INSERT INTO pending_txs (tx_json, created_at) VALUES ($1, $2)`, string(data), time.Now().Unix())
	return err
}

// savePendingTxsBatchExec is savePendingTxExec's multi-row counterpart —
// inserts every tx in txs via ONE round trip instead of one per tx, same
// motivation and technique as flushWALBatch's own outbox INSERT
// (transfer_wal.go) and saveAccountsToDBBatchCtx's multi-row VALUES list
// above. Used by processTransferBatch so a batch of N group-committed
// transfers pays for one outbox round trip instead of N. A no-op for an
// empty slice (a batch of exactly 1 member has nothing left for this to
// insert — its sole outbox row is runAtomicWithOutbox's own single-
// Transaction insert, unchanged).
func savePendingTxsBatchExec(ex sqlExecutor, txs []Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	// See saveAccountsToDBBatchCtx's FIX comment (state.go) for why this
	// builds a fixed-size unnest() query over 2 array parameters instead
	// of a VALUES(...) list whose text size (and lib/pq's own parse cost)
	// grows linearly with len(txs).
	txJSON := make([]string, len(txs))
	createdAts := make([]int64, len(txs))
	now := time.Now().Unix()
	for i, tx := range txs {
		data, err := json.Marshal(tx)
		if err != nil {
			fmt.Printf("[TX] SavePendingTx marshal error: %v\n", err)
			return err
		}
		txJSON[i] = string(data)
		createdAts[i] = now
	}
	_, err := ex.Exec(`INSERT INTO pending_txs (tx_json, created_at) SELECT * FROM unnest($1::text[], $2::bigint[])`, pq.Array(txJSON), pq.Array(createdAts))
	return err
}

// LoadPendingTxs reads all not-yet-included DB-pending TXs and atomically
// marks them included, in the SAME query (UPDATE ... RETURNING) — not a
// separate SELECT now and a DELETE later via ClearPendingTxs. Call
// ClearPendingTxs with the returned ids afterward once the caller has
// durably incorporated these TXs (e.g. into a produced block); that DELETE
// is now just table hygiene, not the only thing preventing reuse — see
// this function's own FIX comment below for why that distinction matters.
// Note: pending_txs is only written by the primary node's EVM RPC layer.
// Secondary nodes have separate DBs and their pending_txs table is always empty,
// so calling this on a secondary is safe — it just returns nil immediately.
//
// FIX (audit 2026-06-28 recheck 5, P1-2): this used to be a plain SELECT,
// relying entirely on ClearPendingTxs's later DELETE to prevent the same
// row from being loaded twice. If that DELETE failed (DB hiccup AFTER the
// block carrying these TXs was already built and broadcast — exactly the
// audit recheck 4 P1-1 fix's own warning, "these TX(s) may be duplicated
// into a future block"), the row stayed eligible and the next
// ProduceBlock call loaded it again, including the same TX in a SECOND
// block. register_human is protected from this by its nullifier
// uniqueness check, but transfer/swap/liquidity/faucet/escrow have no such
// guard — any peer that replayed both blocks would apply that TX's delta
// twice, a real double-credit/debit. Marking included_at here means a row
// can never be selected by this query again regardless of whether the
// later DELETE ever succeeds — a failed delete now only leaves a harmless,
// already-included row behind, not a duplicate-processing risk.
// maxTxsPerBlock bounds how many pending transactions a single ProduceBlock
// call can bundle into one block — see SCALING_ARCHITECTURE.md Phase 9.
// LoadPendingTxs used to have no LIMIT at all: every currently-pending TX,
// however many, went into ONE block every BLOCK_TIME tick. Measured directly
// (TestBlockCostAtScale, AEQUITAS_BLOCK_SIZE_BENCH=1): at 50,000 TXs in one
// block, calculateBlockHash (producer AND every verifying peer) + the full
// block's json.Unmarshal (every receiving peer) alone already cost ~275ms
// combined — a meaningful fraction of a 1-2s BLOCK_TIME before any P2P
// transfer time or transaction-replay cost is even counted; at 100,000 it
// was ~530ms. Left unbounded, sustained high-TPS load would eventually
// produce single blocks large enough to make ProduceBlock/AddPeerBlock's own
// serialization overhead compete with the actual transaction throughput
// this whole scaling project exists to increase.
//
// Raised from 20,000 to 50,000 (matching the stated TPS target directly)
// once two things were confirmed: (1) TestBlockCostAtScale already measured
// 50,000 TXs/block at ~275ms combined hash+unmarshal cost — comfortably
// inside a 1-2s BLOCK_TIME window, the same safety margin the original
// 20,000 figure was picked for; (2) the P2P block-receive path
// (handleBlockStream, see p2p.go's maxBlockStreamBytes) had a stale 512 KB
// read cap that silently truncated (and therefore silently DROPPED) any
// block over roughly ~2,200 transactions — already reachable at the OLD
// 20,000 cap, not merely a future risk. Raising this constant without that
// fix would have made an already-live bug's blast radius worse for no
// benefit; fixed first, in the same change.
//
// This still does NOT by itself deliver 50,000 TPS sustained: at
// BLOCK_TIME's current ~1-2s cadence, one block still caps throughput at
// roughly 25,000-50,000 TPS through block relay specifically — an upper
// bound this constant now matches, not a guarantee the storage layer
// (phases 1-6, i.e. everything up to and including the shard-locked
// transfer path) can actually fill a block that large every tick. See
// SCALING_ARCHITECTURE.md's Phase 9 framing: multiple blocks per tick
// and/or a shorter BLOCK_TIME remain a separate, larger cadence decision,
// deliberately not part of this change (that's a multi-node consensus-
// timing question, not a transport/storage-layer one — see this repo's
// documented history of real GHOSTDAG forks from exactly that class of
// change, and needs real multi-node staging validation this constant
// bump does not).
//
// Any pending TX beyond this cap simply stays included_at=0 (this query's
// WHERE clause) and is picked up by the NEXT LoadPendingTxs call — no TX is
// ever dropped, only deferred, exactly the same FIFO backlog-draining
// property a real bounded queue needs.
const maxTxsPerBlock = 50000

func (cs *ChainState) LoadPendingTxs() ([]Transaction, []int64) {
	if cs.db == nil {
		return nil, nil
	}
	rows, err := cs.db.Query(
		`UPDATE pending_txs SET included_at = $1
		 WHERE id IN (SELECT id FROM pending_txs WHERE included_at = 0 ORDER BY id LIMIT $2)
		 RETURNING id, tx_json`,
		time.Now().Unix(), maxTxsPerBlock,
	)
	if err != nil {
		fmt.Printf("[TX] LoadPendingTxs error: %v\n", err)
		return nil, nil
	}
	// FIX (BRUTAL-P2-03): do NOT issue DML (INSERT/DELETE on pending_txs) while
	// the UPDATE...RETURNING cursor is still open — the same connection holds
	// row locks from the RETURNING scan, and issuing further DML on those rows
	// inside the same result-set iteration is undefined/blocking behaviour.
	// Collect corrupt rows in a slice, close the cursor explicitly, then
	// dead-letter them in a separate pass. The defer below is kept as a
	// safety net for the early-return error paths above.
	defer rows.Close()
	type idTx struct {
		id  int64
		tx  Transaction
	}
	type badRow struct {
		id     int64
		errMsg string
	}
	var loaded []idTx
	var corrupt []badRow
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			continue
		}
		var tx Transaction
		if err := json.Unmarshal([]byte(raw), &tx); err != nil {
			corrupt = append(corrupt, badRow{id: id, errMsg: err.Error()})
			continue
		}
		loaded = append(loaded, idTx{id: id, tx: tx})
	}
	// Close the cursor before any DML so we no longer hold locks on the rows.
	rows.Close()
	for _, br := range corrupt {
		fmt.Printf("[TX] LoadPendingTxs unmarshal error for id=%d — moving to dead-letter queue: %v\n", br.id, br.errMsg)
		// FIX (Brutal Audit 2026-06-28, P3-06; confirmed still present
		// 2026-06-29): both DML statements here used to discard their
		// errors. A corrupt row's whole point of being routed here is that
		// it can never be replayed as a normal TX again (its included_at
		// was already claimed by the UPDATE...RETURNING above) — if the
		// INSERT into the dead-letter table fails, the row's content is
		// gone forever the moment the DELETE below still runs anyway, with
		// no record anywhere of what it contained or why. If the INSERT
		// succeeds but the DELETE fails, the row stays in pending_txs
		// forever with included_at already set (non-zero), permanently
		// invisible to the next LoadPendingTxs call (which only selects
		// included_at = 0) — silently "lost in place" rather than dead-
		// lettered, with no log line distinguishing that from a clean
		// dead-letter. Insert first; only delete from pending_txs if the
		// insert actually succeeded, so a corrupt row's content is never
		// destroyed without first being durably preserved somewhere.
		if _, err := cs.db.Exec(
			`INSERT INTO pending_txs_dead_letter (id, tx_json, created_at, failed_at, fail_reason)
			 SELECT id, tx_json, created_at, $1, $2 FROM pending_txs WHERE id = $3
			 ON CONFLICT (id) DO NOTHING`,
			time.Now().Unix(), br.errMsg, br.id,
		); err != nil {
			fmt.Printf("[TX] ⚠ ALERT: could not dead-letter corrupt pending_tx id=%d — leaving it in pending_txs (included_at already claimed, so it will NOT be retried; investigate manually): %v\n", br.id, err)
			continue
		}
		if _, err := cs.db.Exec(`DELETE FROM pending_txs WHERE id = $1`, br.id); err != nil {
			fmt.Printf("[TX] ⚠ ALERT: dead-lettered pending_tx id=%d but could not delete it from pending_txs — row now exists in BOTH tables with included_at already claimed (harmless duplicate record, but investigate): %v\n", br.id, err)
		}
	}
	// UPDATE ... RETURNING does not guarantee output order matches the
	// subquery's ORDER BY — restore insertion order explicitly, since
	// block.Transactions order is part of the block hash and replay
	// processes TXs in order.
	sort.Slice(loaded, func(i, j int) bool { return loaded[i].id < loaded[j].id })
	txs := make([]Transaction, 0, len(loaded))
	ids := make([]int64, 0, len(loaded))
	for _, lt := range loaded {
		txs = append(txs, lt.tx)
		ids = append(ids, lt.id)
	}
	return txs, ids
}

// MarkPendingTxsIncluded records which block included a set of pending TX rows.
// Called BEFORE SaveBlockToDB (see block.go) so ResetStaleIncludedPendingTxs
// can distinguish crash-before-save (block absent → requeue OK) from
// crash-after-save (block present → leave alone).
// FIX (BRUTAL-P2-04): now returns error so callers can react — previously
// errors were only logged and the caller had no signal that the write failed.
func (cs *ChainState) MarkPendingTxsIncluded(ids []int64, blockHash string) error {
	if cs.db == nil || len(ids) == 0 {
		return nil
	}
	if _, err := cs.db.Exec(
		`UPDATE pending_txs SET included_block_hash = $1 WHERE id = ANY($2)`,
		blockHash, ids,
	); err != nil {
		fmt.Printf("[TX] MarkPendingTxsIncluded error: %v\n", err)
		return err
	}
	return nil
}

// ResetStaleIncludedPendingTxs reverts included_at back to 0 for any row
// that's been "included" for longer than maxAge and never cleared —
// recovery for the crash window between LoadPendingTxs (marks included)
// and ClearPendingTxs (deletes rows). Only resets rows whose
// included_block_hash is either NULL (crash before SaveBlockToDB) or
// references a block NOT in chain_blocks (block never durably saved) —
// rows linked to a saved block are left alone to avoid re-including TXs
// that were already processed.
func (cs *ChainState) ResetStaleIncludedPendingTxs(maxAge time.Duration) {
	if cs.db == nil {
		return
	}
	cutoff := time.Now().Add(-maxAge).Unix()
	res, err := cs.db.Exec(
		`UPDATE pending_txs SET included_at = 0, included_block_hash = NULL
		 WHERE included_at > 0 AND included_at < $1
		   AND (included_block_hash IS NULL
		        OR NOT EXISTS (SELECT 1 FROM chain_blocks WHERE hash = pending_txs.included_block_hash))`,
		cutoff,
	)
	if err != nil {
		fmt.Printf("[TX] ResetStaleIncludedPendingTxs error: %v\n", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		fmt.Printf("[TX] Reset %d stale-included pending_txs row(s) for retry (likely a crash before broadcast)\n", n)
	}
}

// ── REGISTRATION RECOVERY (BRUTAL-P1-01 / P1-02) ───────────────────────────
//
// Flow (true state-machine, as required by audit P1-02):
//
//  1. SaveRegistrationIntent(wallet, nullifier, pendingTx)
//     → inserts into registration_recovery with evm_tx_hash = '' (pre-EVM sentinel)
//     → returns the row id so the caller can update / mark it later
//
//  2. sendRawTransaction (EVM submit)
//     → on failure: DeleteRegistrationIntent(id)
//     → on success: UpdateRegistrationIntentEVMTxHash(id, txHash)
//
//  3. RegisterHumanAtomic (Go-state + outbox)
//     → on success: MarkRegistrationIntentRecovered(id)
//     → on failure (3 retries): leave the record; background retry picks it up
//
// Background RetryRegistrationRecoveries:
//   • evm_tx_hash = '' : pre-EVM intent — EVM was never confirmed. Try
//     RegisterHumanAtomic anyway; if the wallet was registered by block replay
//     from another node, "already registered" closes the record. If not,
//     leave pending — the user must re-submit the registration.
//   • evm_tx_hash != '' : post-EVM recovery — retry RegisterHumanAtomic only.
//
// This closes the critical window where EVM commits but the process crashes
// before either RegisterHumanAtomic or SaveRegistrationRecovery is called,
// which previously left the registration invisible to all secondary nodes.

// SaveRegistrationIntent writes a pre-EVM intent record. evm_tx_hash is stored
// as '' until the EVM transaction is confirmed. Returns the new row id.
func (cs *ChainState) SaveRegistrationIntent(wallet, nullifier string, pendingTx Transaction) (int64, error) {
	if cs.db == nil {
		return 0, fmt.Errorf("db not available")
	}
	pendingJSON, _ := json.Marshal(pendingTx)
	var id int64
	err := cs.db.QueryRow(
		`INSERT INTO registration_recovery
		 (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		 VALUES ($1, '', $2, $3, $4)
		 RETURNING id`,
		strings.ToLower(wallet), nullifier, string(pendingJSON), time.Now().Unix(),
	).Scan(&id)
	return id, err
}

// UpdateRegistrationIntentEVMTxHash updates the intent row after EVM success.
func (cs *ChainState) UpdateRegistrationIntentEVMTxHash(id int64, txHash string) error {
	if cs.db == nil {
		return nil
	}
	_, err := cs.db.Exec(`UPDATE registration_recovery SET evm_tx_hash = $1 WHERE id = $2`, txHash, id)
	return err
}

// DeleteRegistrationIntent removes a pre-EVM intent when EVM submission fails —
// the registration never happened so there is nothing to recover.
func (cs *ChainState) DeleteRegistrationIntent(id int64) {
	if cs.db == nil {
		return
	}
	if _, err := cs.db.Exec(`DELETE FROM registration_recovery WHERE id = $1 AND evm_tx_hash = ''`, id); err != nil {
		fmt.Printf("[RECOVERY] ⚠ Could not delete pre-EVM intent id=%d: %v\n", id, err)
	}
}

// MarkRegistrationIntentRecovered closes a registration_recovery record after
// Go-state has been successfully updated.
func (cs *ChainState) MarkRegistrationIntentRecovered(id int64) {
	if cs.db == nil {
		return
	}
	if _, err := cs.db.Exec(`UPDATE registration_recovery SET recovered_at = $1 WHERE id = $2`, time.Now().Unix(), id); err != nil {
		fmt.Printf("[RECOVERY] ⚠ Could not mark registration intent id=%d as recovered: %v\n", id, err)
	}
}

// SaveRegistrationRecovery writes a recovery record for a registration whose
// EVM transaction succeeded but whose Go-state sync failed. Returns an error
// only if writing to the DB itself fails (the original regErr is passed in
// separately by the caller for logging/degraded messaging).
func (cs *ChainState) SaveRegistrationRecovery(wallet, evmTxHash, nullifier string, pendingTx Transaction) error {
	if cs.db == nil {
		return fmt.Errorf("db not available")
	}
	pendingJSON, err := json.Marshal(pendingTx)
	if err != nil {
		pendingJSON = []byte("{}")
	}
	_, dbErr := cs.db.Exec(
		`INSERT INTO registration_recovery
		 (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		strings.ToLower(wallet), evmTxHash, nullifier, string(pendingJSON), time.Now().Unix(),
	)
	return dbErr
}

// RecordSyntheticCheckpointEvent persists a durable audit trail entry every
// time this node bridges a permanently-missing parent with a
// synthetic-checkpoint stub (audit 2026-06-30 monster audit, P1-05). source
// identifies which code path inserted the stub ("startup-bridge" for
// BridgeHistoricalGap, "runtime-orphan-bridge" for queueOrphan's TTL
// branch) so a later DB query can distinguish "trusted a known historical
// gap once at boot" from "kept trusting new gaps during normal operation."
// Best-effort: a failure here must never block the bridge itself (the stub
// already exists in memory regardless) — only logged.
func (cs *ChainState) RecordSyntheticCheckpointEvent(stubHash string, stubHeight int64, source string) {
	if cs.db == nil {
		return
	}
	if _, err := cs.db.Exec(
		`INSERT INTO synthetic_checkpoint_events (stub_hash, stub_height, source, created_at) VALUES ($1, $2, $3, $4)`,
		stubHash, stubHeight, source, time.Now().Unix(),
	); err != nil {
		fmt.Printf("[AUDIT] ⚠ Could not persist synthetic-checkpoint event for %s: %v\n", stubHash, err)
	}
}

// CountUnrecoveredRegistrations returns the number of registration_recovery
// rows that have not yet been successfully replayed (recovered_at IS NULL).
func (cs *ChainState) CountUnrecoveredRegistrations() int {
	if cs.db == nil {
		return 0
	}
	var n int
	cs.db.QueryRow(`SELECT COUNT(*) FROM registration_recovery WHERE recovered_at IS NULL`).Scan(&n)
	return n
}

// RetryRegistrationRecoveries attempts RegisterHumanAtomic for every
// unrecovered record, marks the record recovered on success, and returns the
// number of records newly recovered in this pass.
func (cs *ChainState) RetryRegistrationRecoveries() int {
	if cs.db == nil {
		return 0
	}
	rows, err := cs.db.Query(`
		SELECT id, wallet, evm_tx_hash, nullifier, pending_tx_json
		FROM registration_recovery
		WHERE recovered_at IS NULL
		ORDER BY created_at ASC`)
	if err != nil {
		fmt.Printf("[RECOVERY] RetryRegistrationRecoveries query failed: %v\n", err)
		return 0
	}
	type rec struct {
		id          int64
		wallet      string
		evmTxHash   string
		nullifier   string
		pendingJSON string
	}
	var records []rec
	for rows.Next() {
		var r rec
		if scanErr := rows.Scan(&r.id, &r.wallet, &r.evmTxHash, &r.nullifier, &r.pendingJSON); scanErr == nil {
			records = append(records, r)
		}
	}
	rows.Close()

	recovered := 0
	for _, r := range records {
		// Pre-EVM intent (evm_tx_hash='') — EVM was never confirmed for this record.
		// This happens when the process crashed between SaveRegistrationIntent and
		// sendRawTransaction.  We can't re-submit the EVM tx from here (no signing
		// key available in ChainState), so try RegisterHumanAtomic:
		// • if the wallet was registered via block replay from another node →
		//   "already registered" → mark recovered (the registration did happen)
		// • if not yet registered → leave pending (user must re-submit via /register)
		if r.evmTxHash == "" {
			var regErr error
			var pendingTx Transaction
			if r.pendingJSON != "" {
				if err := json.Unmarshal([]byte(r.pendingJSON), &pendingTx); err != nil {
					fmt.Printf("[RECOVERY] ⚠ Pre-EVM intent id=%d (wallet %s) has corrupt pending_tx_json — leaving pending for manual review\n", r.id, r.wallet)
					continue
				}
			}
			if pendingTx.Nullifier != "" {
				regErr = cs.RegisterHumanAtomic(r.wallet, pendingTx)
			} else {
				regErr = cs.RegisterHuman(r.wallet)
			}
			if regErr == nil || strings.Contains(regErr.Error(), "already registered") {
				if _, err := cs.db.Exec(`UPDATE registration_recovery SET recovered_at=$1, last_error='pre-evm intent: resolved via block replay or RegisterHumanAtomic' WHERE id=$2`,
					time.Now().Unix(), r.id); err != nil {
					fmt.Printf("[RECOVERY] ⚠ Could not mark pre-EVM intent id=%d recovered: %v\n", r.id, err)
				}
				recovered++
				fmt.Printf("[RECOVERY] ✓ Pre-EVM intent for %s resolved\n", r.wallet)
			} else {
				fmt.Printf("[RECOVERY] ℹ Pre-EVM intent id=%d (wallet %s) not yet recoverable: %v — user should re-submit registration\n", r.id, r.wallet, regErr)
				if _, err := cs.db.Exec(`UPDATE registration_recovery SET last_error=$1 WHERE id=$2`, "pre-evm intent: "+regErr.Error(), r.id); err != nil {
					fmt.Printf("[RECOVERY] ⚠ Could not update last_error for pre-EVM intent id=%d: %v\n", r.id, err)
				}
			}
			continue
		}

		if _, err := cs.db.Exec(`UPDATE registration_recovery SET attempt_count=attempt_count+1, last_attempt_at=$1 WHERE id=$2`,
			time.Now().Unix(), r.id); err != nil {
			fmt.Printf("[RECOVERY] ⚠ Could not update attempt_count for recovery id=%d (wallet %s): %v\n", r.id, r.wallet, err)
		}

		// FIX (Brutal Audit P2-05): a corrupt pending_tx_json used to be
		// silently swallowed (json.Unmarshal error suppressed with
		// //nolint:errcheck), leaving pendingTx at its zero value — which
		// then fell through to the weaker cs.RegisterHuman(r.wallet) path
		// (no nullifier, no outbox TX) as if this record had simply never
		// had pending_tx_json in the first place. That's a real
		// "already registered" / outbox-less registration risk hiding
		// behind data corruption, not a legitimate missing-field case.
		// Genuinely empty (pre-existing records from before this column
		// existed) is fine and expected; a non-empty value that fails to
		// parse is corruption and must be flagged loudly, not silently
		// downgraded to the weaker recovery path.
		var pendingTx Transaction
		if r.pendingJSON != "" {
			if err := json.Unmarshal([]byte(r.pendingJSON), &pendingTx); err != nil {
				fmt.Printf("[RECOVERY] ✗ Corrupt pending_tx_json for recovery id=%d (wallet %s): %v — skipping this attempt, NOT falling back to RegisterHuman without outbox data\n", r.id, r.wallet, err)
				if _, dbErr := cs.db.Exec(`UPDATE registration_recovery SET last_error=$1 WHERE id=$2`,
					fmt.Sprintf("corrupt pending_tx_json: %v", err), r.id); dbErr != nil {
					fmt.Printf("[RECOVERY] ⚠ Could not record corruption error for recovery id=%d: %v\n", r.id, dbErr)
				}
				continue
			}
		}

		var regErr error
		if pendingTx.Nullifier != "" {
			regErr = cs.RegisterHumanAtomic(r.wallet, pendingTx)
		} else {
			regErr = cs.RegisterHuman(r.wallet)
		}

		if regErr != nil {
			alreadyDone := strings.Contains(regErr.Error(), "already registered")
			if alreadyDone {
				// Go-state already has this wallet as human (perhaps recovered
				// by a previous attempt or by block replay from another node).
				if _, err := cs.db.Exec(`UPDATE registration_recovery SET recovered_at=$1, last_error='already registered in go-state — treated as recovered' WHERE id=$2`,
					time.Now().Unix(), r.id); err != nil {
					fmt.Printf("[RECOVERY] ⚠ Could not mark recovery id=%d as recovered (wallet %s already registered in Go-state — recovery WILL be retried again next cycle): %v\n", r.id, r.wallet, err)
				}
				recovered++
				fmt.Printf("[RECOVERY] ✓ Registration for %s already present in Go-state — marked recovered\n", r.wallet)
			} else {
				if _, err := cs.db.Exec(`UPDATE registration_recovery SET last_error=$1 WHERE id=$2`, regErr.Error(), r.id); err != nil {
					fmt.Printf("[RECOVERY] ⚠ Could not record retry error for recovery id=%d (wallet %s): %v\n", r.id, r.wallet, err)
				}
				fmt.Printf("[RECOVERY] ✗ Retry failed for %s: %v\n", r.wallet, regErr)
			}
		} else {
			if _, err := cs.db.Exec(`UPDATE registration_recovery SET recovered_at=$1, last_error=NULL WHERE id=$2`,
				time.Now().Unix(), r.id); err != nil {
				fmt.Printf("[RECOVERY] ⚠ Could not mark recovery id=%d as recovered (wallet %s WAS successfully registered — recovery WILL be retried again next cycle): %v\n", r.id, r.wallet, err)
			}
			recovered++
			fmt.Printf("[RECOVERY] ✓ Successfully recovered Go-state registration for wallet %s\n", r.wallet)
			cs.SyncBalancesToEVM(V7_CONTRACT_ADDR, r.wallet)
		}
	}

	// Clear the degraded flag once no unrecovered records remain.
	if recovered > 0 && cs.CountUnrecoveredRegistrations() == 0 {
		cur := cs.BootstrapDegradedReason()
		if strings.Contains(cur, "registration_recovery") {
			cs.SetBootstrapDegraded("")
		}
	}
	return recovered
}

// ClearPendingTxs deletes the given pending_txs rows by id. Call only after
// the corresponding TXs are durably incorporated elsewhere (e.g. in a
// produced block) — see LoadPendingTxs.
//
// FIX (audit 2026-06-28 recheck 4, P1-1): this used to discard every Exec
// error silently and return nothing. If a delete failed, the caller had no
// way to know — the next ProduceBlock's LoadPendingTxs would load that same
// row again and include its TX in a SECOND block. Any peer that replays
// both blocks would apply that TX's delta twice: a real double-credit/debit,
// not just stale outbox bookkeeping. Now retries each delete a few times
// (the same transient-DB-blip tolerance SavePendingTx already has) and
// returns an aggregated error so the caller can at least alert loudly —
// the block this round already produced can't be un-broadcast at this
// point, so there's no rollback to do here, but the operator needs to know
// duplicate-TX risk now exists for this round's rows.
func (cs *ChainState) ClearPendingTxs(ids []int64) error {
	if cs.db == nil {
		return nil
	}
	var firstErr error
	for _, id := range ids {
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			if _, err := cs.db.Exec(`DELETE FROM pending_txs WHERE id = $1`, id); err != nil {
				lastErr = err
				if attempt < 3 {
					time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
				}
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			fmt.Printf("[TX] ClearPendingTxs: could not delete pending_txs id=%d after retries: %v\n", id, lastErr)
			if firstErr == nil {
				firstErr = fmt.Errorf("could not delete pending_txs id=%d: %w", id, lastErr)
			}
		}
	}
	return firstErr
}

// LoadAndClearPendingTxs is kept for any external callers that don't need
// the durability ordering LoadPendingTxs/ClearPendingTxs provides.
//
// FIX: ProduceBlock used to call this directly, which deletes the DB rows
// BEFORE the block carrying these TXs is actually constructed. A crash in
// that window (between this delete committing and the rest of ProduceBlock
// finishing) permanently loses the TX from the outbox with no block ever
// having included it — the primary's own local state already has the
// change (it was applied synchronously when first processed), but no other
// node ever learns about it: a permanent, silent divergence. ProduceBlock
// now calls LoadPendingTxs/ClearPendingTxs directly instead, clearing only
// after the block is fully built.
func (cs *ChainState) LoadAndClearPendingTxs() []Transaction {
	txs, ids := cs.LoadPendingTxs()
	if err := cs.ClearPendingTxs(ids); err != nil {
		fmt.Printf("[TX] LoadAndClearPendingTxs: %v\n", err)
	}
	return txs
}

// SaveBlockToDB persists a block header durably to chain_blocks — see the
// table's own FIX comment (state.go) for why this exists: dag.blocks is
// purely in-memory and resets to genesis on every restart, so without this
// a node that produces or accepts a block and then crashes before any peer
// also has it permanently loses that block, even though the account-state
// effects of its TXs were already committed earlier (at mutation time).
// ON CONFLICT DO NOTHING: a block can legitimately be saved twice (e.g. a
// node that both produced a block and later re-receives it from a peer);
// the row already reflects the same immutable content keyed by hash.
// ensureGHOSTDAGColumns adds the three GHOSTDAG columns to chain_blocks if
// they don't exist yet (one-time migration; idempotent via IF NOT EXISTS).
func (cs *ChainState) ensureGHOSTDAGColumns() {
	if cs.db == nil {
		return
	}
	// Run the ALTER TABLEs at most once per process — see ghostdagColumnsOnce's
	// comment for why calling them on every block save was a major latency and
	// lock-contention source over a remote DB. The columns are immutable once
	// added, so a single successful pass is sufficient for the process lifetime.
	cs.ghostdagColumnsOnce.Do(func() {
		cs.db.Exec(`ALTER TABLE chain_blocks ADD COLUMN IF NOT EXISTS selected_parent TEXT DEFAULT ''`)
		cs.db.Exec(`ALTER TABLE chain_blocks ADD COLUMN IF NOT EXISTS blue_score BIGINT DEFAULT 0`)
		cs.db.Exec(`ALTER TABLE chain_blocks ADD COLUMN IF NOT EXISTS blues TEXT DEFAULT '[]'`)
		// FIX (2026-07-04 — Contabo 2 catch-up-vs-real-time-production
		// starvation at faster BLOCK_TIME): LoadBlocksSinceFromDB (the query
		// that answers every peer's /api/blocks?min_height= page request —
		// i.e. every deepScan/BridgeHistoricalGap catch-up call a struggling
		// node makes) filters WHERE height >= $1 and orders by height ASC,
		// blue_score DESC, hash ASC. The plain height-only index
		// (idx_chain_blocks_height) lets Postgres find the right rows but
		// still forces an explicit sort over every matching row before LIMIT
		// can apply, since blue_score/hash aren't part of that index — cheap
		// at 6s BLOCK_TIME (few rows per height, calls infrequent), but each
		// such call is now far more frequent (every ~1s poll) and the table
		// has grown into the hundreds of thousands of rows. The SERVING
		// node (Primary/Contabo 1) pays this sort cost synchronously inside
		// the HTTP response the REQUESTING node (Contabo 2, mid catch-up) is
		// blocked waiting on — slow responses here directly eat into the
		// tighter real-time window a faster BLOCK_TIME leaves for catching
		// up before the requester's own next production tick fires.
		cs.db.Exec(`CREATE INDEX IF NOT EXISTS idx_chain_blocks_height_bluescore_hash ON chain_blocks (height, blue_score DESC, hash ASC)`)
	})
}

// ensureReplayedColumn adds chain_blocks.replayed if it doesn't exist yet.
//
// FIX (self-deadlock incident, 2026-07-11 — see verifyZKProof's own comment
// in block.go for the deadlock itself): SaveBlockToDB persists a block's
// header BEFORE replayTransactions actually applies its transactions to
// chain_accounts (P2-05's deliberate save-before-replay ordering — see that
// comment). The startup loader used to mark EVERY block found in
// chain_blocks as already-replayed unconditionally, on the assumption that
// "header saved" implies "effects committed" — true for a clean
// success-or-rollback replay, but NOT true if the process is killed
// mid-replay (as SIGQUIT does: no deferred cleanup runs). Confirmed live:
// a register_human block's header made it into chain_blocks on Contabo1
// while the goroutine replaying it was deadlocked inside verifyZKProof;
// after restart, the block was silently treated as fully applied and that
// human was permanently missing from chain_accounts, with total_humans
// stuck one below Primary's and no error anywhere.
//
// DEFAULT TRUE on the ALTER TABLE (existing rows are presumed already
// correctly applied — true for the overwhelming majority of chain
// history); SaveBlockToDB explicitly inserts NEW rows as false, only
// flipped to true by MarkBlockReplayed once a replay actually commits. The
// startup loader (NewBlockchain) now checks this per-block instead of
// blindly trusting chain_blocks membership, and re-drives replay for any
// block still marked false — see its own comment.
func (cs *ChainState) ensureReplayedColumn() {
	if cs.db == nil {
		return
	}
	cs.replayedColumnOnce.Do(func() {
		cs.db.Exec(`ALTER TABLE chain_blocks ADD COLUMN IF NOT EXISTS replayed BOOLEAN NOT NULL DEFAULT true`)
	})
}

// MarkBlockReplayed flips chain_blocks.replayed to true for hash. Called via
// cs.dbExecCtx(ctx) so it joins the SAME dbTx as the account mutations replay
// just made (replayTransactions calls this right before
// commitOrRollback(true), passing context.Background() — dag.state.activeTx
// is still set to dbTx at that point, and dbExecCtx falls back to it, see
// registerHumanLocked's comment) — atomic with the actual state effects, so
// a crash before commit leaves BOTH the account changes and this flag
// rolled back together, and a crash after commit leaves BOTH durable
// together. Never true without the corresponding account effects also
// being true.
func (cs *ChainState) MarkBlockReplayed(ctx context.Context, hash string) error {
	if cs.db == nil {
		return nil
	}
	cs.ensureReplayedColumn()
	_, err := cs.dbExecCtx(ctx).Exec(`UPDATE chain_blocks SET replayed = true WHERE hash = $1`, hash)
	return err
}

// SaveBlockToDB persists a block header, with replayed marking whether its
// transactions' effects are already known-durable in chain_accounts:
//   - AddPeerBlock (block.go) passes false — the header is saved BEFORE
//     replayTransactions runs (P2-05's deliberate ordering), so it isn't
//     true yet; MarkBlockReplayed flips it once replay actually commits.
//   - The checkpoint/resync seeding call sites (snapshot.go) pass true —
//     a checkpoint's account state is imported wholesale as a trusted
//     snapshot, never individually replayed transaction-by-transaction, so
//     there is nothing for a later repair pass to redo.
//
// See ensureReplayedColumn's comment for the live incident this distinction
// closes.
func (cs *ChainState) SaveBlockToDB(block *Block, replayed bool) error {
	if cs.db == nil {
		return nil
	}
	cs.ensureGHOSTDAGColumns()
	cs.ensureReplayedColumn()
	parentHashesJSON, err := json.Marshal(block.ParentHashes)
	if err != nil {
		return fmt.Errorf("marshal parent_hashes: %w", err)
	}
	txsJSON, err := json.Marshal(block.Transactions)
	if err != nil {
		return fmt.Errorf("marshal transactions: %w", err)
	}
	bluesJSON, err := json.Marshal(block.Blues)
	if err != nil {
		bluesJSON = []byte("[]")
	}
	// cs.db directly, NOT cs.dbExec() (P0, 2026-07-25 — the exact path the new
	// [DB-GUARD] caught in production within minutes of shipping):
	//
	//   [DB-GUARD] dbExec from goroutine 2481 while goroutine 143 holds the
	//   active transaction
	//     → SaveBlockToDB (evm_storage.go) → AddPeerBlock → handleBlockPush
	//
	// A block arriving over HTTP push saves its header on its own goroutine
	// while some other goroutine is midway through a replay transaction.
	// dbExec()'s cs.activeTx fallback handed it that foreign *sql.Tx, putting
	// two goroutines on one Postgres connection — which desyncs the wire
	// protocol (`pq: unexpected Parse response "(D) DataRow"`) and cost two
	// consensus blocks tonight.
	//
	// Every caller of this function is a standalone write that must not join
	// anyone's transaction: AddPeerBlock deliberately saves the header BEFORE
	// replay opens its own (and deletes it again if replay fails, see P0-02),
	// and SeedTrustedCheckpoint runs after the resync transaction has already
	// been cleared. So the pool is not merely safe here, it is correct.
	_, err = cs.db.Exec(
		`INSERT INTO chain_blocks
		   (hash, height, parent_hashes, proposer, timestamp, humans, state_root,
		    signature, transactions, selected_parent, blue_score, blues, replayed)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 ON CONFLICT (hash) DO NOTHING`,
		block.Hash, block.Height, string(parentHashesJSON), block.Proposer, block.Timestamp,
		block.Humans, block.StateRoot, block.Signature, string(txsJSON),
		block.SelectedParent, block.BlueScore, string(bluesJSON), replayed,
	)
	return err
}

// DeleteBlockFromDB removes a block header from chain_blocks.
// Called when AddPeerBlock's replay fails after the header was pre-saved,
// so a restart never marks an unapplied block as replayed (P0-02).
func (cs *ChainState) DeleteBlockFromDB(hash string) error {
	if cs.db == nil {
		return nil
	}
	_, err := cs.dbExec().Exec(`DELETE FROM chain_blocks WHERE hash=$1`, hash)
	return err
}

// SaveGHOSTDAGState updates only the GHOSTDAG columns for an existing block.
// Used by the startup migration and after local GHOSTDAG compute in AddPeerBlock (P1-03).
func (cs *ChainState) SaveGHOSTDAGState(block *Block) error {
	if cs.db == nil {
		return nil
	}
	cs.ensureGHOSTDAGColumns()
	bluesJSON, err := json.Marshal(block.Blues)
	if err != nil {
		bluesJSON = []byte("[]")
	}
	_, err = cs.dbExec().Exec(
		`UPDATE chain_blocks SET selected_parent=$1, blue_score=$2, blues=$3 WHERE hash=$4`,
		block.SelectedParent, block.BlueScore, string(bluesJSON), block.Hash,
	)
	return err
}

// SaveGHOSTDAGStateBatch persists GHOSTDAG columns for a slice of blocks in a
// single DB transaction — used by the startup migration to collapse O(n)
// round-trips into O(n/batchSize) commits (~100 UPDATEs each).
func (cs *ChainState) SaveGHOSTDAGStateBatch(blocks []*Block) error {
	if cs.db == nil || len(blocks) == 0 {
		return nil
	}
	cs.ensureGHOSTDAGColumns()
	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`UPDATE chain_blocks SET selected_parent=$1, blue_score=$2, blues=$3 WHERE hash=$4`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, block := range blocks {
		bluesJSON, jerr := json.Marshal(block.Blues)
		if jerr != nil {
			bluesJSON = []byte("[]")
		}
		if _, err := stmt.Exec(block.SelectedParent, block.BlueScore, string(bluesJSON), block.Hash); err != nil {
			tx.Rollback()
			return fmt.Errorf("batch ghostdag save block #%d: %w", block.Height, err)
		}
	}
	return tx.Commit()
}

// SaveBlockWithPendingTxsAtomic saves the block and clears the given pending-TX
// rows in a single DB transaction so the two operations either both commit or
// both roll back.  This closes the narrow window where SaveBlockToDB succeeds
// but ClearPendingTxs fails — previously that left rows with included_at set
// but not deleted; the already-processed TXs could theoretically be loaded
// again on the next ProduceBlock call.
//
// The call also stamps included_block_hash on the rows inside the same
// transaction, which ResetStaleIncludedPendingTxs uses to decide whether to
// requeue a row: "block present in chain_blocks AND included_block_hash matches"
// → leave alone; "block absent" → requeue.
func (cs *ChainState) SaveBlockWithPendingTxsAtomic(block *Block, ids []int64) error {
	if cs.db == nil {
		return nil
	}
	parentHashesJSON, err := json.Marshal(block.ParentHashes)
	if err != nil {
		return fmt.Errorf("marshal parent_hashes: %w", err)
	}
	txsJSON, err := json.Marshal(block.Transactions)
	if err != nil {
		return fmt.Errorf("marshal transactions: %w", err)
	}

	bluesJSONFast, _ := json.Marshal(block.Blues)
	if bluesJSONFast == nil {
		bluesJSONFast = []byte("[]")
	}

	// FAST PATH (2026-07-02 cadence fix): the overwhelmingly common case is a
	// block with no pending-TX rows to reconcile (0 tx/block in steady state).
	// The atomic Begin/INSERT/Commit below exists ONLY to keep the block INSERT
	// and the pending_txs UPDATE+DELETE in one transaction — with no ids there
	// is nothing to make atomic, so the explicit transaction is pure overhead:
	// 3 network round trips (BEGIN, INSERT, COMMIT) instead of 1. Over a remote
	// DB reached via a cross-project public proxy (~380ms/round-trip, confirmed
	// live on the primary) that turned every block save into ~1.14s held under
	// dag.mu — the direct cause of the sustained multi-second cadence and the
	// resulting failure to merge with peers. A single autocommit INSERT is one
	// round trip (~380ms) and is exactly as durable here (a lone INSERT is its
	// own implicit transaction). The transactional path is still used verbatim
	// whenever there ARE pending-TX ids to reconcile atomically.
	// replayed = true, EXPLICITLY (2026-07-25 night): this function persists
	// this node's OWN freshly-produced block, whose transaction effects were
	// already applied to chain_accounts synchronously at RPC time, before the
	// block was even assembled — there is nothing for a later repair pass to
	// redo. This previously relied on the column's schema DEFAULT true, which
	// works but leaves the intent invisible and one schema-default change away
	// from every self-produced block silently joining the boot-repair backlog
	// (the exact backlog class that kept Primary unreachable for ~35 minutes).
	cs.ensureReplayedColumn()
	if len(ids) == 0 {
		if _, err := cs.db.Exec(
			`INSERT INTO chain_blocks
			   (hash, height, parent_hashes, proposer, timestamp, humans, state_root,
			    signature, transactions, selected_parent, blue_score, blues, replayed)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true)
			 ON CONFLICT (hash) DO NOTHING`,
			block.Hash, block.Height, string(parentHashesJSON), block.Proposer, block.Timestamp,
			block.Humans, block.StateRoot, block.Signature, string(txsJSON),
			block.SelectedParent, block.BlueScore, string(bluesJSONFast),
		); err != nil {
			return fmt.Errorf("save block (fast path): %w", err)
		}
		return nil
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	rollback := func() {
		if rbErr := tx.Rollback(); rbErr != nil && rbErr.Error() != "sql: transaction has already been committed or rolled back" {
			fmt.Printf("[TX] SaveBlockWithPendingTxsAtomic rollback error: %v\n", rbErr)
		}
	}

	// FIX (P0 performance, 2026-07-24 — measured, not assumed): this used to
	// run `UPDATE pending_txs SET included_block_hash = $1 WHERE id = ANY($2)`
	// here, on the same ids the DELETE below removes inside this same
	// transaction. That stamp was never observable by anything: on COMMIT the
	// rows are gone, and on ROLLBACK the UPDATE is undone with them. Both
	// statements sit under the identical len(ids) > 0 guard, so there is no
	// path where the UPDATE lands without the DELETE following it.
	//
	// It was not free, though. In Postgres an UPDATE writes an entirely new
	// row version (MVCC) and touches every index on the table, so this
	// rewrote all 50,000 rows microseconds before deleting them.
	// TestSaveBlockCostAtScale (block_save_cost_bench_test.go) measured its
	// share of a full-block save directly, and it dominated everything else:
	//
	//   txs      UPDATE   INSERT   DELETE   COMMIT   save-total
	//   1,000      6ms      3ms      2ms      4ms       14ms   (UPDATE 42%)
	//   10,000    48ms     27ms     17ms     15ms      108ms   (UPDATE 45%)
	//   50,000   753ms    103ms     43ms     13ms      912ms   (UPDATE 83%)
	//
	// — superlinear in row count, and 912ms lines up with the ~822ms
	// STAGING_RUNBOOK.md measured for this function on a full block, which is
	// the bulk of the ~1.8s ProduceBlock that currently caps block relay at
	// roughly one full block per 2s (~25,000 TPS, not the 50,000 target).
	// Dropping it takes this function from ~912ms to ~159ms on that block.
	//
	// Consequence for crash recovery: none. ResetStaleIncludedPendingTxs only
	// ever sees rows that still EXIST with included_at > 0 — i.e. exactly the
	// crash-before-save case, where LoadPendingTxs' own committed UPDATE set
	// included_at but this transaction never ran. Those rows have
	// included_block_hash NULL both before and after this change, so its
	// "IS NULL → requeue" branch behaves identically. included_block_hash is
	// now vestigial (nothing writes it; MarkPendingTxsIncluded, its only other
	// writer, has no callers left), deliberately left in place rather than
	// dropped in the same change as a performance fix.
	bluesJSON, _ := json.Marshal(block.Blues)
	if bluesJSON == nil {
		bluesJSON = []byte("[]")
	}
	if _, err := tx.Exec(
		`INSERT INTO chain_blocks
		   (hash, height, parent_hashes, proposer, timestamp, humans, state_root,
		    signature, transactions, selected_parent, blue_score, blues, replayed)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true)
		 ON CONFLICT (hash) DO NOTHING`,
		block.Hash, block.Height, string(parentHashesJSON), block.Proposer, block.Timestamp,
		block.Humans, block.StateRoot, block.Signature, string(txsJSON),
		block.SelectedParent, block.BlueScore, string(bluesJSON),
	); err != nil {
		rollback()
		return fmt.Errorf("save block: %w", err)
	}

	if len(ids) > 0 {
		if _, err := tx.Exec(
			`DELETE FROM pending_txs WHERE id = ANY($1)`, ids,
		); err != nil {
			rollback()
			return fmt.Errorf("clear pending txs: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		rollback()
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// LoadBlocksFromDB reconstructs every durably-saved block (see SaveBlockToDB)
// for seeding dag.blocks/dag.tips/dag.height on startup, so a node's own
// previously produced or accepted blocks survive a restart without needing
// any peer to still have them. Returns blocks keyed by hash; the caller
// derives tips (any hash never referenced as another loaded block's parent)
// and height (the max Height among them) itself, since BlockDAG owns that
// state, not ChainState.
//
// FIX (2026-06-28, production incident — same root cause class as
// loadFromDB's): this used to return nil silently on any query error,
// indistinguishable from "this node genuinely has zero durably-saved
// blocks" (a real, normal case for a brand-new node). The caller
// (NewBlockchain, block.go) only restores dag.height/dag.blocks/dag.tips
// when len(loaded) > 0 — so a transient query failure on a node with a
// FULL chain_blocks table silently left the in-memory DAG at genesis,
// height 0, forcing a full peer resync of its entire own history on every
// restart that hit the hiccup. Now retries once, and returns an explicit
// error (instead of a nil map a real "zero rows" case can't be told apart
// from) if it still fails, so the caller can refuse to start rather than
// silently behave as if a node with real history had none.
// getMaxBlockHeightDB returns the max_block_height stored in chain_config,
// or 0 if not set. Used by NewBlockchain to determine the startup load window
// before dag.height is established.
func (cs *ChainState) getMaxBlockHeightDB() int64 {
	v := cs.getConfigValueDB("max_block_height")
	var h int64
	fmt.Sscanf(v, "%d", &h)
	return h
}

// LoadBlocksFromDB loads blocks from chain_blocks into a hash→Block map.
// minHeight filters to height >= minHeight; pass 0 to load all blocks.
// On large chains, callers should pass (tipHeight - startupLoadWindow) to
// bound startup RAM — bootHeight guards against re-replay of older blocks
// even when they are absent from dag.blocks.
func (cs *ChainState) LoadBlocksFromDB(minHeight int64) (map[string]*Block, error) {
	if cs.db == nil {
		return nil, nil
	}
	// Ensure GHOSTDAG/replayed columns exist before reading them (idempotent migration).
	cs.ensureGHOSTDAGColumns()
	cs.ensureReplayedColumn()
	baseQuery := `SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions,
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]'),
	                 COALESCE(replayed,true)
	          FROM chain_blocks`
	var rows *sql.Rows
	var err error
	if minHeight > 0 {
		rows, err = cs.db.Query(baseQuery+" WHERE height >= $1", minHeight)
	} else {
		rows, err = cs.db.Query(baseQuery)
	}
	if err != nil {
		fmt.Printf("[BLOCK] LoadBlocksFromDB query error (attempt 1): %v — retrying once\n", err)
		time.Sleep(2 * time.Second)
		if minHeight > 0 {
			rows, err = cs.db.Query(baseQuery+" WHERE height >= $1", minHeight)
		} else {
			rows, err = cs.db.Query(baseQuery)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("LoadBlocksFromDB query failed after retry: %w", err)
	}
	defer rows.Close()
	blocks := make(map[string]*Block)
	for rows.Next() {
		var b Block
		var parentHashesRaw, txsRaw, bluesRaw string
		if err := rows.Scan(
			&b.Hash, &b.Height, &parentHashesRaw, &b.Proposer, &b.Timestamp,
			&b.Humans, &b.StateRoot, &b.Signature, &txsRaw,
			&b.SelectedParent, &b.BlueScore, &bluesRaw, &b.Replayed,
		); err != nil {
			fmt.Printf("[BLOCK] LoadBlocksFromDB scan error: %v\n", err)
			continue
		}
		if err := json.Unmarshal([]byte(parentHashesRaw), &b.ParentHashes); err != nil {
			fmt.Printf("[BLOCK] LoadBlocksFromDB parent_hashes unmarshal error for %s: %v\n", b.Hash, err)
			continue
		}
		if err := json.Unmarshal([]byte(txsRaw), &b.Transactions); err != nil {
			fmt.Printf("[BLOCK] LoadBlocksFromDB transactions unmarshal error for %s: %v\n", b.Hash, err)
			continue
		}
		if bluesRaw != "" && bluesRaw != "[]" && bluesRaw != "null" {
			if err := json.Unmarshal([]byte(bluesRaw), &b.Blues); err != nil {
				b.Blues = nil // will be recomputed in migration pass
			}
		}
		blocks[b.Hash] = &b
	}
	return blocks, nil
}

// LoadUnreplayedBlocksFromDB returns every chain_blocks row with
// replayed = false, with NO height bound — unlike LoadBlocksFromDB's
// startupLoadWindow-bounded load (which exists purely to cap startup RAM on
// a large chain), a block needing repair can be arbitrarily far behind the
// current tip (confirmed live: Contabo1's one broken block was ~8,500
// blocks behind by the time this was diagnosed) and would otherwise never
// even be fetched, let alone repaired. This should only ever return a
// handful of rows in practice — replayed only stays false when a process is
// killed mid-replay (see ensureReplayedColumn's comment) — so an unbounded
// scan is fine; the startup loader's RAM concern doesn't apply to a query
// that's normally empty.
func (cs *ChainState) LoadUnreplayedBlocksFromDB() ([]*Block, error) {
	if cs.db == nil {
		return nil, nil
	}
	cs.ensureGHOSTDAGColumns()
	cs.ensureReplayedColumn()
	rows, err := cs.db.Query(`SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions,
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]')
	          FROM chain_blocks WHERE replayed = false`)
	if err != nil {
		return nil, fmt.Errorf("LoadUnreplayedBlocksFromDB query failed: %w", err)
	}
	defer rows.Close()
	var blocks []*Block
	for rows.Next() {
		var b Block
		var parentHashesRaw, txsRaw, bluesRaw string
		if err := rows.Scan(
			&b.Hash, &b.Height, &parentHashesRaw, &b.Proposer, &b.Timestamp,
			&b.Humans, &b.StateRoot, &b.Signature, &txsRaw,
			&b.SelectedParent, &b.BlueScore, &bluesRaw,
		); err != nil {
			fmt.Printf("[BLOCK] LoadUnreplayedBlocksFromDB scan error: %v\n", err)
			continue
		}
		if err := json.Unmarshal([]byte(parentHashesRaw), &b.ParentHashes); err != nil {
			fmt.Printf("[BLOCK] LoadUnreplayedBlocksFromDB parent_hashes unmarshal error for %s: %v\n", b.Hash, err)
			continue
		}
		if err := json.Unmarshal([]byte(txsRaw), &b.Transactions); err != nil {
			fmt.Printf("[BLOCK] LoadUnreplayedBlocksFromDB transactions unmarshal error for %s: %v\n", b.Hash, err)
			continue
		}
		if bluesRaw != "" && bluesRaw != "[]" && bluesRaw != "null" {
			json.Unmarshal([]byte(bluesRaw), &b.Blues)
		}
		b.Replayed = false
		blocks = append(blocks, &b)
	}
	return blocks, nil
}

// LoadBlockFromDBByHeight loads a single block header at the given height
// directly from chain_blocks, bypassing dag.blocks entirely — same fallback
// role as LoadBlockFromDBByHash (see its comment), for callers keyed by
// height instead of hash (GetBlockByHeight, as its own last-resort fallback
// when its primary path — canonicalBlockAtHeightLocked's SelectedParent
// walk from the best in-memory tip — can't run at all, e.g. dag.tips is
// empty right after a restart). Multiple validators can routinely produce a
// sibling at the same height; this prefers the highest BlueScore (ties
// broken by lowest hash), matching canonicalBlockAtHeightLocked's own
// tie-break as closely as a single-row, no-graph-walk DB query can. This is
// still only a heuristic, NOT the same guarantee as the real walk (it
// can't know which sibling is actually reachable via SelectedParent links
// from the network's true best tip without that tip as a starting point) —
// acceptable here specifically because it is the last-resort path, not the
// primary one. Excludes synthetic-checkpoint stubs the same way
// GetBlockByHeight does. Returns nil if no real (non-stub) block exists at
// this height.
func (cs *ChainState) LoadBlockFromDBByHeight(height int64) *Block {
	if cs.db == nil {
		return nil
	}
	cs.ensureGHOSTDAGColumns()
	rows, err := cs.db.Query(`SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions,
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]')
	          FROM chain_blocks WHERE height = $1`, height)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var best *Block
	for rows.Next() {
		var b Block
		var parentHashesRaw, txsRaw, bluesRaw string
		if err := rows.Scan(
			&b.Hash, &b.Height, &parentHashesRaw, &b.Proposer, &b.Timestamp,
			&b.Humans, &b.StateRoot, &b.Signature, &txsRaw,
			&b.SelectedParent, &b.BlueScore, &bluesRaw,
		); err != nil {
			continue
		}
		if b.Proposer == "synthetic-checkpoint" {
			continue
		}
		_ = json.Unmarshal([]byte(parentHashesRaw), &b.ParentHashes)
		_ = json.Unmarshal([]byte(txsRaw), &b.Transactions)
		if bluesRaw != "" && bluesRaw != "[]" && bluesRaw != "null" {
			_ = json.Unmarshal([]byte(bluesRaw), &b.Blues)
		}
		if best == nil || b.BlueScore > best.BlueScore || (b.BlueScore == best.BlueScore && b.Hash < best.Hash) {
			bCopy := b
			best = &bCopy
		}
	}
	return best
}

// HasBlockFromProposerAtHeight reports whether chain_blocks already has a
// row for the given (proposer, height) pair. Used by ProduceBlock's
// post-boot double-production guard (see that call site's own comment) —
// unlike LoadBlockFromDBByHeight, which picks the single highest-BlueScore
// block at a height regardless of proposer (the right tool for canonical
// lookups, the wrong one here), this checks specifically for THIS
// validator's own row, since two different validators legitimately sharing
// a height is normal GHOSTDAG operation, not the case this guards against.
func (cs *ChainState) HasBlockFromProposerAtHeight(proposer string, height int64) bool {
	if cs.db == nil {
		return false
	}
	var exists bool
	cs.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM chain_blocks WHERE height = $1 AND lower(proposer) = lower($2))`,
		height, proposer,
	).Scan(&exists)
	return exists
}

// LoadBlockFromDBByHash loads a single block header by hash directly from
// chain_blocks, bypassing dag.blocks entirely. Used as a fallback when a
// block needed for a computation (e.g. the finality checkpoint's
// selected-parent walk, see maybeAdvanceFinalizedCheckpoint) has been
// evicted from the in-memory DAG by pruneOldDAGBlocks but — per that
// function's own guarantee — is still durably retained here. Returns nil
// (not an error) if the hash genuinely doesn't exist, matching
// GetBlockByHash's contract.
func (cs *ChainState) LoadBlockFromDBByHash(hash string) *Block {
	if cs.db == nil {
		return nil
	}
	cs.ensureGHOSTDAGColumns()
	row := cs.db.QueryRow(`SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions,
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]')
	          FROM chain_blocks WHERE hash = $1`, hash)
	var b Block
	var parentHashesRaw, txsRaw, bluesRaw string
	if err := row.Scan(
		&b.Hash, &b.Height, &parentHashesRaw, &b.Proposer, &b.Timestamp,
		&b.Humans, &b.StateRoot, &b.Signature, &txsRaw,
		&b.SelectedParent, &b.BlueScore, &bluesRaw,
	); err != nil {
		return nil
	}
	_ = json.Unmarshal([]byte(parentHashesRaw), &b.ParentHashes)
	_ = json.Unmarshal([]byte(txsRaw), &b.Transactions)
	if bluesRaw != "" && bluesRaw != "[]" && bluesRaw != "null" {
		_ = json.Unmarshal([]byte(bluesRaw), &b.Blues)
	}
	return &b
}

// dbSinceFetchWindow is how many extra rows LoadBlocksSinceFromDB fetches
// beyond the caller's requested limit, to comfortably cover same-height
// sibling blocks sitting after an after_hash cursor within the same page.
const dbSinceFetchWindow = 512

// LoadBlocksSinceFromDB loads a canonically-ordered (height ASC, blue_score
// DESC, hash ASC) window of blocks directly from chain_blocks, starting at
// height >= minHeight, bypassing dag.blocks entirely. Fetches a window wider
// than `limit` (dbSinceFetchWindow) so selectBlocksSince has enough rows on
// hand to apply the exact same min_height/after_hash cursor semantics the
// in-memory path uses (including same-height siblings after a cursor — see
// selectBlocksSince's comment) before trimming to `limit`.
//
// FIX (P0, merge-reliability audit 2026-07-03): see GetBlocksSince's own
// comment for the incident this fixes — pruneOldDAGBlocks evicts old blocks
// from dag.blocks, and until this function existed nothing backing
// /api/blocks?min_height= could ever serve a range below that eviction
// point, even though chain_blocks (here) has always retained it in full.
func (cs *ChainState) LoadBlocksSinceFromDB(minHeight int64, afterHash string, limit int) ([]*Block, error) {
	if cs.db == nil {
		return nil, nil
	}
	cs.ensureGHOSTDAGColumns()
	fetchLimit := limit + dbSinceFetchWindow
	rows, err := cs.db.Query(`SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions,
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]')
	          FROM chain_blocks
	          WHERE height >= $1 AND proposer != 'synthetic-checkpoint'
	          ORDER BY height ASC, blue_score DESC, hash ASC
	          LIMIT $2`, minHeight, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("LoadBlocksSinceFromDB query failed: %w", err)
	}
	defer rows.Close()
	var blocks []*Block
	for rows.Next() {
		var b Block
		var parentHashesRaw, txsRaw, bluesRaw string
		if err := rows.Scan(
			&b.Hash, &b.Height, &parentHashesRaw, &b.Proposer, &b.Timestamp,
			&b.Humans, &b.StateRoot, &b.Signature, &txsRaw,
			&b.SelectedParent, &b.BlueScore, &bluesRaw,
		); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(parentHashesRaw), &b.ParentHashes)
		_ = json.Unmarshal([]byte(txsRaw), &b.Transactions)
		if bluesRaw != "" && bluesRaw != "[]" && bluesRaw != "null" {
			_ = json.Unmarshal([]byte(bluesRaw), &b.Blues)
		}
		blocks = append(blocks, &b)
	}
	return selectBlocksSince(blocks, minHeight, afterHash, limit), nil
}

// LoadBlocksByHashesFromDB loads blocks by exact hash directly from
// chain_blocks, bypassing dag.blocks. Used as a DB fallback for
// GetBlocksByHashesForPeer once pruneOldDAGBlocks has evicted the requested
// hashes from memory — silently omits any hash not found, matching
// GetBlocksByHashesForPeer's own contract (the caller checks which hashes
// are still missing afterward).
func (cs *ChainState) LoadBlocksByHashesFromDB(hashes []string) ([]*Block, error) {
	if cs.db == nil || len(hashes) == 0 {
		return nil, nil
	}
	cs.ensureGHOSTDAGColumns()
	rows, err := cs.db.Query(`SELECT hash, height, parent_hashes, proposer, timestamp, humans, state_root,
	                 signature, transactions,
	                 COALESCE(selected_parent,''), COALESCE(blue_score,0), COALESCE(blues,'[]')
	          FROM chain_blocks
	          WHERE hash = ANY($1) AND proposer != 'synthetic-checkpoint'`, pq.Array(hashes))
	if err != nil {
		return nil, fmt.Errorf("LoadBlocksByHashesFromDB query failed: %w", err)
	}
	defer rows.Close()
	var blocks []*Block
	for rows.Next() {
		var b Block
		var parentHashesRaw, txsRaw, bluesRaw string
		if err := rows.Scan(
			&b.Hash, &b.Height, &parentHashesRaw, &b.Proposer, &b.Timestamp,
			&b.Humans, &b.StateRoot, &b.Signature, &txsRaw,
			&b.SelectedParent, &b.BlueScore, &bluesRaw,
		); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(parentHashesRaw), &b.ParentHashes)
		_ = json.Unmarshal([]byte(txsRaw), &b.Transactions)
		if bluesRaw != "" && bluesRaw != "[]" && bluesRaw != "null" {
			_ = json.Unmarshal([]byte(bluesRaw), &b.Blues)
		}
		blocks = append(blocks, &b)
	}
	return blocks, nil
}
