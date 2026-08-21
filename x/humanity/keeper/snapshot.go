package keeper

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// StateSnapshot is a complete, portable export of the Go-state needed to
// bootstrap a new node that has no PostgreSQL database of its own.
// It includes accounts, pool reserves, nullifiers, and bio-registration
// commitments — sufficient to validate new registrations, swaps, and
// transfers without access to the primary database.
// Version marks the snapshot schema version. P3-11: lets importers detect schema changes.
const SnapshotVersion = 1

type StateSnapshot struct {
	Version   int   `json:"version"`
	Timestamp int64 `json:"timestamp"`
	// Height is the producer's BlockDAG height at export time. A node that
	// bootstraps from this snapshot already reflects the cumulative effect
	// of every block up to and including this height — without recording
	// that cutoff, the importer's subsequent HTTP-SYNC catch-up (which
	// always starts from height 0, since dag.blocks is empty in memory
	// after any restart regardless of what the snapshot seeded into
	// cs.accounts) would replay those same historical blocks' transactions
	// a second time on top of the already-current imported balances —
	// silently double-applying every transfer/swap/registration that ever
	// happened before the snapshot was taken. See ImportSnapshotFromURL
	// and replayTransactions' use of "snapshot_import_height".
	Height             int64                     `json:"height"`
	Accounts           []*AccountState           `json:"accounts"`
	Pool               *PoolState                `json:"pool"`
	Nullifiers         map[string]string         `json:"nullifiers"`          // nullifier → wallet (sentinel 0x01 if wallet redacted)
	NullifiersRedacted bool                      `json:"nullifiers_redacted"` // true when wallet addresses replaced with sentinel (public tier)
	BioRegistrations   []SnapshotBioRegistration `json:"bio_registrations"`
	ChainConfig        map[string]string         `json:"chain_config,omitempty"` // critical timing keys for secondary state sync
	Signature          string                    `json:"signature,omitempty"`    // ECDSA over SHA256(JSON without this field)
}

type SnapshotBioRegistration struct {
	Commitment    string `json:"commitment"`
	WalletAddress string `json:"wallet_address"`
	BioHash       string `json:"bio_hash,omitempty"`
}

// ExportSnapshot captures the live Go-state and, if signingKey is non-nil,
// signs the JSON payload so consumers can verify authenticity. height must
// be the producer's current BlockDAG height (it's set here, before signing,
// rather than by the caller afterward, so it's covered by the signature
// like everything else in the snapshot) — see StateSnapshot.Height's
// comment for why the importer needs this cutoff.
//
// includeSensitive controls whether nullifier→wallet linkages and
// bio_registrations (commitment↔wallet↔txHash) are included.
//
// FIX (2026-06-28, SNAPSHOT_TOKEN redesign): these two fields are the only
// part of a snapshot that isn't already public-by-design (account
// balances, pool state, and config timing are all visible via the regular
// explorer API anyway). Bundling them into one bulk, anonymously-fetchable
// export is a much more convenient mass-correlation target than scraping
// the same associations one registration at a time out of historical
// blocks — that's what SNAPSHOT_TOKEN actually protects, NOT data a new
// node needs to function. A bootstrapping node doesn't need either field:
// nullifier UNIQUENESS depends on the key being present in the map — see
// IsNullifierUsed (which does an EXISTS check, not wallet != ""). The
// public snapshot uses a sentinel wallet address so importing nodes write
// a non-zero value into the EVM usedNullifiers slot (the V7 contract
// treats zero address as "not used"). bio_registrations is
// explicitly non-consensus bookkeeping (doesn't affect StateRoot). So
// includeSensitive=false still produces a fully correct, bootstrap-capable
// snapshot — just without the two fields that have no legitimate
// bootstrap use, served to anyone with no token and no admin contact
// needed (see handleSnapshot). includeSensitive=true (token required)
// keeps the original full export for authoritative resync/recovery.
func (cs *ChainState) ExportSnapshot(signingKey *ecdsa.PrivateKey, height int64, includeSensitive bool) *StateSnapshot {
	cs.mu.RLock()
	accounts := make([]*AccountState, 0, cs.accounts.Len())
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		cp := *acc
		accounts = append(accounts, &cp)
		return true
	})
	var pool PoolState
	if cs.pool != nil {
		pool = *cs.pool
	}
	nullifiers := make(map[string]string, len(cs.nullifiers))
	for k, v := range cs.nullifiers {
		if includeSensitive {
			nullifiers[k] = v
		} else {
			// FIX (P0-02): use a sentinel non-zero address instead of "".
			// Importing this snapshot stores the sentinel in the DB; the EVM
			// migration then writes a non-zero slot value so V7 treats the
			// nullifier as "used". An empty string became zero address in EVM
			// storage, which V7 reads as "not used" → double-registration attack.
			// Also ensures IsNullifierUsed's DB path (EXISTS check) remains
			// consistent — the uniqueness gate is row presence, not wallet value.
			nullifiers[k] = "0x0000000000000000000000000000000000000001"
		}
	}
	cs.mu.RUnlock()

	snap := &StateSnapshot{
		Version:            SnapshotVersion,
		Timestamp:          time.Now().Unix(),
		Height:             height,
		Accounts:           accounts,
		Pool:               &pool,
		Nullifiers:         nullifiers,
		NullifiersRedacted: !includeSensitive,
	}

	// Pull bio_registrations from DB (commitment → wallet only).
	// bio_hash is intentionally omitted from the snapshot — it is a biometric
	// identifier and must not be exported to peer nodes. A new node can verify
	// commitment uniqueness without needing the raw bio_hash.
	// Not exported at all in the public (includeSensitive=false) tier — see
	// this function's doc comment.
	if cs.db != nil && includeSensitive {
		rows, err := cs.db.Query(`SELECT commitment, wallet_address FROM bio_registrations`)
		if err == nil {
			// P2-FIX: use explicit Close() not defer — defer fires at function return
			// (after the signing step), keeping the DB connection occupied unnecessarily.
			for rows.Next() {
				var commitment, wallet string
				if scanErr := rows.Scan(&commitment, &wallet); scanErr != nil {
					fmt.Printf("[SNAPSHOT] Warning: bio_registrations scan error: %v\n", scanErr)
					continue
				}
				snap.BioRegistrations = append(snap.BioRegistrations, SnapshotBioRegistration{
					Commitment:    commitment,
					WalletAddress: wallet,
				})
			}
			rows.Close()
		}
	}

	// Export critical config values so secondary nodes have matching timing state.
	// FIX (audit 2026-06-28 recheck 4, P0-1): cs.mu.RUnlock() already ran
	// above — this read must use the plain DB-only variant, never
	// cs.dbExec()/cs.activeTx.
	snap.ChainConfig = map[string]string{}
	for _, key := range []string{"last_ubi_at", "last_validators_at", "last_lp_at", "last_treasury_at"} {
		if val := cs.getConfigValueDB(key); val != "" {
			snap.ChainConfig[key] = val
		}
	}

	if signingKey != nil {
		body, _ := json.Marshal(snap)
		hash := sha256.Sum256(body)
		sig, err := crypto.Sign(hash[:], signingKey)
		if err == nil {
			snap.Signature = hex.EncodeToString(sig)
		}
	}

	return snap
}

// fetchAndValidateSnapshot downloads a StateSnapshot from peerURL, checks its
// age, and verifies its signature against expectedSignerHex if non-empty.
// Shared by ImportSnapshotFromURL (merge mode) and ResyncFromSnapshotURL
// (authoritative-replace mode) — every safety check here (SSRF-blocking
// client, age window, signature verification) must apply identically to
// both, so it lives in exactly one place instead of two copies that could
// drift apart.
func fetchAndValidateSnapshot(peerURL, expectedSignerHex string) (*StateSnapshot, error) {
	// F18-FIX: use redirect-blocking client with IP validation to prevent
	// SSRF if BOOTSTRAP_SNAPSHOT_URL is set to a private/cloud-metadata IP.
	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{DialContext: pinningDialer},
	}

	req, reqErr := http.NewRequest("GET", peerURL, nil)
	if reqErr != nil {
		return nil, fmt.Errorf("request build failed: %w", reqErr)
	}
	if token := os.Getenv("SNAPSHOT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("snapshot server returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	var snap StateSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	now := time.Now().Unix()
	if snap.Timestamp > now+60 {
		return nil, fmt.Errorf("snapshot timestamp is in the future (%d seconds ahead)", snap.Timestamp-now)
	}
	maxAge := int64(86400) // 24 hours default
	if v := os.Getenv("SNAPSHOT_MAX_AGE_SECONDS"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &maxAge); n != 1 || err != nil {
			maxAge = 86400
		}
	}
	if now-snap.Timestamp > maxAge {
		return nil, fmt.Errorf("snapshot is too old (%d seconds, max %d) — set SNAPSHOT_MAX_AGE_SECONDS to override", now-snap.Timestamp, maxAge)
	}

	// Signature verification is mandatory when BOOTSTRAP_SIGNER is configured.
	if expectedSignerHex != "" {
		if snap.Signature == "" {
			return nil, fmt.Errorf("snapshot has no signature but BOOTSTRAP_SIGNER is set — import rejected")
		}
		sigCopy := snap.Signature
		snap.Signature = ""
		unsigned, _ := json.Marshal(snap)
		snap.Signature = sigCopy

		hash := sha256.Sum256(unsigned)
		sigBytes, err := hex.DecodeString(snap.Signature)
		if err != nil || len(sigBytes) != 65 {
			return nil, fmt.Errorf("invalid snapshot signature format")
		}
		pubBytes, err := crypto.Ecrecover(hash[:], sigBytes)
		if err != nil {
			return nil, fmt.Errorf("snapshot signature recovery failed: %w", err)
		}
		pubKey, err := crypto.UnmarshalPubkey(pubBytes)
		if err != nil {
			return nil, fmt.Errorf("snapshot public key invalid: %w", err)
		}
		recovered := strings.ToLower(crypto.PubkeyToAddress(*pubKey).Hex())
		expected := strings.ToLower(expectedSignerHex)
		if recovered != expected {
			return nil, fmt.Errorf("snapshot signed by %s, expected %s", recovered, expected)
		}
		fmt.Printf("[SNAPSHOT] ✓ Signature verified (signer: %s)\n", recovered)
	}
	return &snap, nil
}

// ImportSnapshotFromURL downloads a StateSnapshot from peerURL and merges it
// into local state. The import is additive: existing accounts, nullifiers
// and bio-registrations are not overwritten, so it is safe to call on
// partially-populated state (e.g. after a crash mid-import) without
// regressing balances. Verifies the snapshot signature against
// expectedSignerHex if non-empty.
//
// FIX (audit recheck2, P1 #9): this merge-only behavior is exactly right for
// a fresh node bootstrapping (nothing to lose by only ever adding), but is
// NOT a resync — a node with divergent/incorrect local state keeps every
// wrong value, since merge only fills gaps, never corrects existing entries.
// See ResyncFromSnapshotURL below for the authoritative-replace counterpart
// this audit asked for; this function's own merge behavior is unchanged.
func (cs *ChainState) ImportSnapshotFromURL(peerURL, expectedSignerHex string) error {
	local := cs.TotalHumans()
	if local > 0 {
		fmt.Printf("[SNAPSHOT] Merging into existing state (%d humans local) — adding missing entries\n", local)
	}
	snapPtr, err := fetchAndValidateSnapshot(peerURL, expectedSignerHex)
	if err != nil {
		return err
	}
	snap := *snapPtr

	// Apply in-memory under lock, then persist outside lock to avoid deadlock.
	// Existing accounts are NOT overwritten — only missing ones are added.
	// This makes the import safe to call on partially-populated state without
	// regressing balances that have advanced via UBI/demurrage since the snapshot.
	var accountsToPersist []*AccountState
	// FIX 12: Skip system pool addresses when MERGING into an already-running
	// node -- a stale snapshot must not override a pool balance that has
	// already moved on locally (e.g. from fees credited since the snapshot
	// was taken). This guard must NOT apply on a genuine fresh bootstrap
	// (existingHumans == 0): there is nothing to "regress" on an empty node,
	// and skipping these four addresses unconditionally meant a freshly
	// bootstrapped node's validators/LP/UBI/treasury pool balances stayed at
	// zero forever while the primary's kept accumulating real fees --
	// guaranteeing a permanent StateRoot mismatch against the primary that
	// looked exactly like ongoing divergence but was actually just this
	// import never having happened. Mirrors the existing existingHumans==0
	// gate already used for cs.pool a few lines below.
	systemAddresses := map[string]bool{
		validatorsPoolAddr: true,
		lpPoolAddr:         true,
		ubiPoolAddr:        true,
		treasuryPoolAddr:   true,
	}
	// FIX 2: Read human count BEFORE acquiring the write lock. TotalHumans()
	// acquires cs.mu.RLock() internally — calling it while holding cs.mu.Lock()
	// would deadlock (write lock is not re-entrant in sync.RWMutex).
	existingHumans := cs.TotalHumans()

	// FIX (audit 2026-06-28 recheck 4, P0-1/P0-2): cs.db.Begin() must happen
	// BEFORE cs.mu.Lock() (same reasoning as runAtomicWithOutbox — a
	// blocking DB call must never run while cs.mu is held), and cs.mu must
	// then stay held continuously from before cs.activeTx is set through
	// the commit/rollback decision. The previous version released cs.mu
	// right after the in-memory mutation, then set cs.activeTx afterwards,
	// with no lock held at all — meaning cs.activeTx was being read AND
	// written outside of cs.mu's protection (a real data race against any
	// concurrent atomic operation doing the same), and the newly-merged
	// in-memory state was visible to other goroutines before this
	// transaction's commit was even attempted.
	var tx *sql.Tx
	if cs.db != nil {
		var err error
		tx, err = cs.db.Begin()
		if err != nil {
			return fmt.Errorf("snapshot import: could not begin transaction: %w", err)
		}
	}

	cs.mu.Lock()
	cs.setActiveTx(tx)
	// See processTransferBatch's own comment for why capturing cs.activeTx
	// into ctx here (cs.mu held throughout) is safe.
	ctx := withTx(context.Background(), tx)
	prevPool := cs.pool
	poolChanged := false
	for _, acc := range snap.Accounts {
		acc.Address = strings.ToLower(acc.Address)
		if existingHumans > 0 && systemAddresses[acc.Address] {
			continue
		}
		if _, exists := cs.accounts.Get(acc.Address); !exists {
			cs.accounts.Set(acc.Address, acc)
			accountsToPersist = append(accountsToPersist, acc)
		}
	}
	// FIX 11: Only import pool state on genuine cold start (no humans registered).
	// Prevents a stale snapshot from overwriting an active pool that temporarily has
	// zero reserves (e.g., after all liquidity was removed).
	if snap.Pool != nil && existingHumans == 0 && (cs.pool == nil || (cs.pool.ReserveAEQ.Float() == 0 && cs.pool.ReserveTUSD.Float() == 0)) {
		cs.pool = snap.Pool
		poolChanged = true
	}
	for nullifier, wallet := range snap.Nullifiers {
		if _, exists := cs.nullifiers[nullifier]; !exists {
			cs.nullifiers[nullifier] = wallet
		}
	}

	revertInMemory := func() {
		for _, acc := range accountsToPersist {
			cs.accounts.Delete(acc.Address)
		}
		for nullifier := range snap.Nullifiers {
			if wallet, exists := cs.nullifiers[nullifier]; exists && strings.EqualFold(wallet, snap.Nullifiers[nullifier]) {
				delete(cs.nullifiers, nullifier)
			}
		}
		if poolChanged {
			cs.pool = prevPool
		}
	}

	if cs.db != nil {
		// FIX (audit 2026-06-28 full recheck, P1-2): every DB write below used
		// to be fire-and-forget (errors discarded, function always returned
		// nil). In-memory state (cs.accounts/cs.nullifiers/cs.pool) was
		// already mutated above unconditionally — a DB error here silently
		// left memory ahead of the database: the merge would *look*
		// successful to the caller, but a restart (which reloads from the DB,
		// not from memory) would lose exactly the entries that failed to
		// persist, with no error ever surfaced. Now every write joins one
		// real transaction; any failure rolls the DB back AND undoes the
		// in-memory additions made above, so this function's outcome is
		// truthful: either the merge fully applied in both places, or it
		// didn't apply at all and ImportSnapshotFromURL returns an error.
		persistErr := func() error {
			for _, acc := range accountsToPersist {
				if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
					return fmt.Errorf("saving account %s: %w", acc.Address, err)
				}
			}
			if err := cs.savePoolToDBCtx(ctx); err != nil {
				return fmt.Errorf("saving pool: %w", err)
			}
			for nullifier, wallet := range snap.Nullifiers {
				if _, err := tx.Exec(
					`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
					nullifier, strings.ToLower(wallet),
				); err != nil {
					return fmt.Errorf("saving nullifier %s: %w", nullifier, err)
				}
			}
			// FIX (audit recheck3, P1 — "Snapshot/Resync verliert Chain-seitige
			// bio_hashes"): br.BioHash is ALWAYS empty here — ExportSnapshot
			// deliberately never populates it (see its own comment: biometric
			// data must not leave the exporting node). The `if br.BioHash != ""`
			// guard this used to have was therefore permanently dead code,
			// silently implying bio_hashes import was supported when it never
			// ran. bio_hashes is local-node bookkeeping, not part of what this
			// snapshot format actually carries — removed instead of pretending
			// otherwise.
			for _, br := range snap.BioRegistrations {
				if _, err := tx.Exec(
					`INSERT INTO bio_registrations (commitment, wallet_address, bio_hash) VALUES ($1, $2, $3)
					 ON CONFLICT (commitment) DO NOTHING`,
					br.Commitment, strings.ToLower(br.WalletAddress), br.BioHash,
				); err != nil {
					return fmt.Errorf("saving bio_registration %s: %w", br.Commitment, err)
				}
			}
			// Import chain_config timing values. Do NOT overwrite if already set —
			// the primary's live value takes precedence over the snapshot's snapshot-time value.
			for key, val := range snap.ChainConfig {
				if existing := cs.getConfigValue(key); existing == "" {
					if err := cs.setConfigValueCtx(ctx, key, val); err != nil {
						return fmt.Errorf("setting config %s: %w", key, err)
					}
				}
			}
			// FIX (double-apply): on a genuine fresh bootstrap, record the height
			// this snapshot was taken at. cs.accounts above already reflects the
			// cumulative effect of every block up to and including this height —
			// without this marker, the subsequent HTTP-SYNC catch-up (which
			// always starts from height 0, since dag.blocks is empty in memory
			// after any restart) would replay those same blocks' transactions a
			// second time on top of the already-current imported balances.
			// replayTransactions checks this value and skips applying deltas for
			// any block at or below it. See StateSnapshot.Height's comment.
			if existingHumans == 0 && snap.Height > 0 {
				if err := cs.setConfigValueCtx(ctx, "snapshot_import_height", fmt.Sprintf("%d", snap.Height)); err != nil {
					return fmt.Errorf("setting snapshot_import_height: %w", err)
				}
			}
			return nil
		}()

		// FIX (audit 2026-06-28 recheck 4, P0-1/P0-2): cs.mu stays held from
		// before cs.activeTx was set above through this commit/rollback
		// decision — cleared and unlocked together below, never separately.
		cs.setActiveTx(nil)
		if persistErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				fmt.Printf("[SNAPSHOT] CRITICAL: rollback after failed merge-persist also failed: %v\n", rbErr)
			}
			revertInMemory()
			cs.mu.Unlock()
			return fmt.Errorf("snapshot import: merge-persist failed, rolled back: %w", persistErr)
		}
		if err := tx.Commit(); err != nil {
			revertInMemory()
			cs.mu.Unlock()
			return fmt.Errorf("snapshot import: merge-persist commit failed: %w", err)
		}
	}
	// Rebuild the state-root accumulators from the freshly merged DB: the bulk
	// snapshot INSERTs above bypass saveAccountToDB's incremental hook, so
	// accountSetXOR/nullifierSetXOR must be reseeded from the full tables before
	// the next StateRoot is computed. Still under cs.mu.
	cs.rebuildStateAccumulators()
	// Every WAL record written before this import is now superseded: the
	// snapshot just replaced the account state those records would apply to,
	// and chain_accounts.wal_seq (recoverFromWAL's idempotency marker) was
	// replaced along with it. See markWALSupersededByStateReplacement for the
	// live corruption this prevents.
	cs.markWALSupersededByStateReplacement()
	cs.mu.Unlock()

	// FIX (audit 2026-06-28 recheck 5, P1-3): this used to only log the
	// error and still return nil — a fresh or merging node could report a
	// successful import while the EVM mirror (everything eth_call and the
	// V7 contract's own storage reads) silently diverged from the Go-state
	// this function just correctly imported. Mirrors ResyncFromSnapshotURL's
	// own fix for the same gap (audit recheck3, P0 #1): the Go-state DB
	// transaction above already committed by this point (EVM is a derived
	// mirror, not the source of truth, so it can't reasonably join that
	// same SQL transaction) — but the caller must be told this didn't
	// fully succeed instead of seeing a bare "✓ Applied" message.
	if err := cs.MigrateEVMFromGoState(V7_CONTRACT_ADDR); err != nil {
		return fmt.Errorf("snapshot import: Go-state committed successfully, but EVM mirror migration failed (EVM state may now be inconsistent — restart to retry the mirror step): %w", err)
	}

	fmt.Printf("[SNAPSHOT] ✓ Applied %d accounts, %d nullifiers, %d bio-registrations\n",
		len(snap.Accounts), len(snap.Nullifiers), len(snap.BioRegistrations))
	return nil
}

// ResyncFromSnapshotURL is ImportSnapshotFromURL's authoritative-replace
// counterpart (audit recheck2, P1 #9): instead of merging (adding only
// missing entries, never correcting existing ones — see
// ImportSnapshotFromURL's own comment), it REPLACES local accounts, pool,
// nullifiers, bio-registrations, and every StateRoot-relevant chain_config
// key with exactly what the snapshot contains. (NOT bio_hashes — see the
// "DELETE FROM bio_hashes" removal below, audit recheck3 P1: that table is
// never part of what a snapshot exports in the first place, so this
// function leaves it alone rather than wiping it without any way to
// restore it.) This is the correct operation for a node KNOWN to have
// diverged — merge mode cannot
// fix that, by construction, since it only ever adds.
//
// Two safety properties apply here that merge mode doesn't need, precisely
// because this can discard local data:
//  1. expectedSignerHex is MANDATORY, with no "proceed unsigned" fallback —
//     resync without a verified signer would let anyone who can reach this
//     URL overwrite this node's entire state with arbitrary data.
//  2. The call site (main.go) gates this behind an explicit
//     RESYNC_FROM_SNAPSHOT=true env var — this must be a deliberate operator
//     action, never triggered by ordinary node startup.
//
// Combine with RESET_DB_STATE=true and a restart for the cleanest result —
// the live BlockDAG's in-memory tips/orphans aren't touched here (this runs
// before block production starts, same timing as ImportSnapshotFromURL),
// but max_block_height/snapshot_import_height ARE set to the snapshot's
// height so the restart's bootHeight calculation (block.go) picks it up
// correctly — this is the same signed-snapshot recovery this session
// already used twice in production for CD20/the VPS via a full DB wipe;
// this gives operators a way to do it without one.
func (cs *ChainState) ResyncFromSnapshotURL(peerURL, expectedSignerHex string) error {
	if expectedSignerHex == "" {
		return fmt.Errorf("RESYNC_FROM_SNAPSHOT requires BOOTSTRAP_SIGNER set — refusing to replace local state from an unverified source")
	}
	snapPtr, err := fetchAndValidateSnapshot(peerURL, expectedSignerHex)
	if err != nil {
		return err
	}
	snap := *snapPtr

	// FIX (audit 2026-08-15, after a live near-miss): refuse a snapshot that
	// would move this node BACKWARDS by more than a rollout's worth of blocks.
	//
	// Resync exists to rescue a node that has fallen behind, so a legitimate
	// snapshot sits at or ahead of local height. The opposite direction is the
	// dangerous one, and it was live: Contabo1 — the authoritative chain, past
	// 3.8 million blocks — had BOOTSTRAP_SNAPSHOT_URL pointing at Contabo2, and
	// its own log carries a boot where it tried to replace its state from there.
	// That evening Contabo2 was serving height 1. The attempt failed on an HTTP
	// 429, and the only other thing in its way was that BOOTSTRAP_SIGNER matched
	// neither node's actual signing key. Two accidents, both outside this code,
	// were all that stood between a restart and 3.8 million blocks being replaced
	// by a chain of one.
	//
	// snap.Height is the producer's own height and is covered by the signature
	// (see ExportSnapshot), so it cannot be forged apart from the payload. The
	// tolerance is deliberately generous — a snapshot is already slightly stale
	// when it is applied, and a busy node advances during the fetch — but it is
	// finite, which is the entire point.
	if localHeight := cs.localChainHeightForResync(); localHeight > 0 &&
		snap.Height+resyncBackwardsTolerance < localHeight {
		if os.Getenv("ALLOW_RESYNC_REGRESSION") != "true" {
			return fmt.Errorf("%s", resyncRegressionMessage(peerURL, snap.Height, localHeight))
		}
		fmt.Printf("[RESYNC] ⚠ ALLOW_RESYNC_REGRESSION=true — accepting a snapshot %d blocks behind local height %d\n",
			localHeight-snap.Height, localHeight)
	}

	fmt.Printf("[RESYNC] ⚠ Authoritative resync: REPLACING local state with snapshot from %s (signed by %s)\n", peerURL, expectedSignerHex)

	if cs.db == nil {
		// No DB — nothing to make atomic with; replace in-memory only.
		cs.mu.Lock()
		cs.replaceInMemoryFromSnapshotLocked(&snap)
		cs.mu.Unlock()
		fmt.Printf("[RESYNC] ✓ Replaced local state with %d accounts, %d nullifiers, %d bio-registrations from snapshot\n",
			len(snap.Accounts), len(snap.Nullifiers), len(snap.BioRegistrations))
		return nil
	}

	// FIX (audit recheck3, P0 #1): this used to replace in-memory state
	// FIRST, unconditionally, then run several separate, individually
	// auto-committing cs.db.Exec calls (DELETE FROM chain_accounts, DELETE
	// FROM nullifiers, ... re-INSERT each). A crash, DB error, or dropped
	// connection between any two of those statements left the database
	// partially cleared and partially repopulated — and by that point
	// in-memory state had ALREADY been fully replaced, so memory and DB
	// disagreed in two different ways simultaneously. Resync is supposed to
	// be the tool that RESCUES a diverged node; a resync that can itself
	// produce a worse, half-replaced state defeats its own purpose. Now:
	// one real DB transaction for every write below, in-memory state backed
	// up before being touched and restored verbatim on any failure (DB
	// error OR commit failure), so this function's only two outcomes are
	// "fully replaced, in DB and memory together" or "nothing changed at
	// all" — never a partial state in either place.
	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("resync: could not begin transaction: %w", err)
	}

	cs.mu.Lock()
	backupAccounts := cs.accounts.Clone()
	var backupPool *PoolState
	if cs.pool != nil {
		poolCopy := *cs.pool
		backupPool = &poolCopy
	}
	backupNullifiers := make(map[string]string, len(cs.nullifiers))
	for k, v := range cs.nullifiers {
		backupNullifiers[k] = v
	}
	restoreInMemory := func() {
		cs.accounts = backupAccounts
		cs.pool = backupPool
		cs.nullifiers = backupNullifiers
	}

	cs.setActiveTx(tx)
	// See processTransferBatch's own comment for why capturing cs.activeTx
	// into ctx here (cs.mu held throughout) is safe.
	ctx := withTx(context.Background(), tx)
	cs.replaceInMemoryFromSnapshotLocked(&snap)

	fail := func(stepErr error) error {
		restoreInMemory()
		cs.setActiveTx(nil)
		cs.mu.Unlock()
		tx.Rollback()
		return stepErr
	}

	// Replace, not merge: clear every table this snapshot covers before
	// re-inserting its contents, so stale local rows that no longer exist
	// in the (authoritative) snapshot don't survive — all within tx now.
	//
	// chain_blocks is also cleared so the restart's LoadBlocksFromDB starts
	// empty: we need all block headers re-synced from genesis (see
	// max_block_height = "0" below) so dag.blocks is fully populated before
	// Railway's new blocks arrive. Without this, stale headers (from a
	// previous run) would set dag.height to the old max, and doSyncOnce
	// would start from there — leaving a gap whose parent hashes don't
	// exist in dag.blocks, causing every subsequent block to orphan.
	// TRUNCATE, not DELETE, and with the statement timeout lifted for this
	// transaction only.
	//
	// WHY: on 2026-08-21 Contabo1 diverged and could not resync. Every attempt
	// -- boot-time and in-process auto-heal alike -- died at this exact step:
	//
	//   resync failed: resync: could not clear chain_blocks:
	//   pq: canceling statement due to statement timeout (57014)
	//
	// The node held 4.38 million blocks. `DELETE FROM chain_blocks` walks every
	// row and writes an equal volume of WAL, so it cannot finish inside
	// statement_timeout. TRUNCATE unlinks the file instead: constant time, no
	// per-row work.
	//
	// This is a trap that GROWS WITH UPTIME. A young node resyncs fine; the
	// same node months later cannot, and the failure is quiet -- it keeps
	// answering /api/status and looks healthy while being permanently unable to
	// rejoin the chain it diverged from. The only place the reason surfaced was
	// degraded_reason in /api/health/combined.
	//
	// One statement for all four: TRUNCATE takes an ACCESS EXCLUSIVE lock and
	// refuses a table that another references by foreign key unless they go
	// together. That is fine here -- this transaction replaces all of them.
	//
	// SET LOCAL reverts when the transaction ends, so the timeout protecting
	// every other query on this connection is untouched.
	if _, err := tx.Exec(`SET LOCAL statement_timeout = 0`); err != nil {
		return fail(fmt.Errorf("resync: could not lift the statement timeout: %w", err))
	}
	if _, err := tx.Exec(`TRUNCATE chain_blocks, chain_accounts, nullifiers, bio_registrations`); err != nil {
		return fail(fmt.Errorf("resync: could not clear chain_blocks/chain_accounts/nullifiers/bio_registrations: %w", err))
	}
	// FIX (audit recheck3, P1 — "Snapshot/Resync verliert Chain-seitige
	// bio_hashes"): this used to DELETE FROM bio_hashes here and then only
	// reinsert rows where br.BioHash != "" — which is NEVER true, since
	// ExportSnapshot deliberately never populates BioHash (privacy — see
	// its own comment). The net effect was an unconditional, one-way wipe
	// of this node's entire bio_hashes table on every resync, with no way
	// for THIS mechanism to ever restore it, while still being labeled
	// "authoritative" replace — exactly the dishonesty the audit flagged:
	// a resync can't authoritatively restore data it never had a copy of
	// in the first place. bio_hashes is local-node bookkeeping (see
	// SaveBioHash's comment: a secondary, best-effort lookup index, not a
	// security boundary, not consensus state) — left untouched here,
	// consistent with it never being part of what this snapshot exports.
	var saveErr error
	cs.accounts.Range(func(_ string, acc *AccountState) bool {
		// saveAccountToDBCtx routes through cs.dbExecCtx(ctx), which returns
		// tx (captured above) — joins this transaction.
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			saveErr = fmt.Errorf("resync: could not save account %s: %w", acc.Address, err)
			return false
		}
		return true
	})
	if saveErr != nil {
		return fail(saveErr)
	}
	if snap.Pool != nil {
		if err := cs.savePoolToDBCtx(ctx); err != nil {
			return fail(fmt.Errorf("resync: could not save pool: %w", err))
		}
	}
	for nullifier, wallet := range snap.Nullifiers {
		if _, err := tx.Exec(
			`INSERT INTO nullifiers (nullifier, wallet_address) VALUES ($1, $2)`,
			nullifier, strings.ToLower(wallet),
		); err != nil {
			return fail(fmt.Errorf("resync: could not insert nullifier: %w", err))
		}
	}
	for _, br := range snap.BioRegistrations {
		if _, err := tx.Exec(
			`INSERT INTO bio_registrations (commitment, wallet_address, bio_hash) VALUES ($1, $2, $3) ON CONFLICT (commitment) DO NOTHING`,
			br.Commitment, strings.ToLower(br.WalletAddress), br.BioHash,
		); err != nil {
			return fail(fmt.Errorf("resync: could not insert bio_registration: %w", err))
		}
	}
	// Authoritative: every StateRoot-relevant config key takes the
	// snapshot's value unconditionally, unlike ImportSnapshotFromURL's
	// merge mode which never overwrites an existing key. setConfigValue now
	// routes through cs.dbExec() too (audit recheck3, P0 #2), so this joins
	// the same transaction instead of auto-committing separately.
	for key, val := range snap.ChainConfig {
		if err := cs.setConfigValueCtx(ctx, key, val); err != nil {
			return fail(fmt.Errorf("resync: could not set config %q: %w", key, err))
		}
	}
	if snap.Height > 0 {
		if err := cs.setConfigValueCtx(ctx, "snapshot_import_height", fmt.Sprintf("%d", snap.Height)); err != nil {
			return fail(fmt.Errorf("resync: could not set snapshot_import_height: %w", err))
		}
		// Set max_block_height to 0, NOT snap.Height, as the DEFAULT here:
		// dag.height is seeded from max_block_height at startup, and
		// doSyncOnce pages forward from dag.height - syncOverlap. If we
		// stored snap.Height unconditionally, the first sync cycle would
		// request blocks near snap.Height — but dag.blocks only contains
		// genesis at this point (chain_blocks was just cleared above), so
		// every arriving block's parent hash would be missing and the entire
		// chain would orphan permanently -- UNLESS a verified real block gets
		// seeded at that height first. See SeedTrustedCheckpoint (called by
		// main.go right after this function returns, before
		// RefreshBootHeightAfterSnapshotImport reads max_block_height back
		// out): on success it overwrites max_block_height with the real,
		// independently-verified checkpoint height and seeds dag.blocks with
		// the actual block, making a 0-height genesis walk unnecessary. This
		// function always writes "0" first as the safe default that a
		// checkpoint-seed failure (peer unreachable, verification failed,
		// etc.) can never leave the node worse off than today's baseline.
		// With max_block_height = 0, the node
		// restarts at dag.height = 0 and syncs ALL block headers sequentially
		// from genesis; replay is skipped for heights ≤ snapshot_import_height
		// (see replayTransactions' skipHeight check), so account state stays
		// correct. bootHeight is still set correctly via snapshot_import_height
		// in RefreshBootHeightAfterSnapshotImport.
		if err := cs.setConfigValueCtx(ctx, "max_block_height", "0"); err != nil {
			return fail(fmt.Errorf("resync: could not reset max_block_height: %w", err))
		}
		// Reset the finalized checkpoint too (P0, 2026-07-02 bootstrap-deadlock):
		// chain_blocks was just wiped and max_block_height set to 0 for a clean
		// genesis-up re-sync. A stale finalized_height left over from BEFORE the
		// wipe (confirmed live on Contabo: finalized_height=67293 with ZERO rows
		// in chain_blocks) is incoherent — the node believes it has finalized a
		// height it can no longer produce a single block below, and the catch-up
		// deadlocks: everything below the stale checkpoint is treated as
		// already-final history that never re-arrives, so the DAG can never be
		// rebuilt from genesis and every block above orphans forever. This is
		// exactly why the FIRST resync of a node works but a SECOND one (after
		// finalization has advanced) hangs. Re-finalization happens naturally via
		// maybeAdvanceFinalizedCheckpoint as the node re-syncs the canonical chain.
		//
		// FIX (P0, 2026-07-03 night): this used to write "0" via the plain
		// setConfigValue calls below directly — which only ever touch chain_config
		// in the DB, never GetFinalizedCheckpoint's in-memory cache
		// (finalizedHeightCache/finalizedCacheLoaded, added by the same night's
		// earlier cadence fix). NewBlockchain's own startup incoherence check
		// (see its call to GetFinalizedCheckpoint) populates that cache from
		// whatever was in the DB BEFORE this resync ever runs — and once
		// finalizedCacheLoaded is true, GetFinalizedCheckpoint NEVER reads the DB
		// again for the rest of this process's life, regardless of what any other
		// code path writes there. Confirmed live: a full resync correctly wiped
		// chain_blocks and dag.height back to 0, but isFinalityViolation kept
		// rejecting every incoming block as "below finalized checkpoint 172871" —
		// the exact stale pre-resync value, permanently deadlocking catch-up
		// again, the identical failure mode this whole reset exists to prevent,
		// just relocated from the DB layer to the cache layer that didn't exist
		// when this code was first written. ResetFinalizedCheckpoint resets both
		// atomically; use it instead of reimplementing half of it here.
		if err := cs.ResetFinalizedCheckpoint(); err != nil {
			return fail(fmt.Errorf("resync: could not reset finalized checkpoint: %w", err))
		}
	}

	// FIX (audit 2026-06-28 recheck 4, P0-2): cs.mu used to be released here,
	// BEFORE tx.Commit() ran below — making the freshly-replaced in-memory
	// state visible to every other goroutine while the DB transaction
	// itself was still pending. A concurrent caller could read the new
	// memory state and write against cs.db (cs.activeTx was already nil)
	// before this commit's outcome was even known; if the commit then
	// failed and triggered a revert, that concurrent write would be
	// clobbered without ever being told its assumptions were invalidated.
	// cs.mu now stays held through the commit decision itself.
	cs.setActiveTx(nil)
	if err := tx.Commit(); err != nil {
		// Commit itself failed — none of the above actually persisted, so
		// in-memory (already replaced above) must be reverted too, or this
		// node would believe it resynced while the DB never did.
		restoreInMemory()
		cs.mu.Unlock()
		return fmt.Errorf("resync: commit failed, state fully reverted: %w", err)
	}
	// Reseed the state-root accumulators from the authoritative post-resync DB:
	// the bulk replace above bypassed the incremental hooks, so accountSetXOR/
	// nullifierSetXOR must be recomputed before the next StateRoot. This is what
	// makes a resynced node's root match the healthy peer it copied — the whole
	// point of the resync. Still under cs.mu.
	cs.rebuildStateAccumulators()
	// THE resync path this fix exists for — see
	// markWALSupersededByStateReplacement: without this, the next boot
	// re-applied the entire WAL history on top of the freshly resynced state
	// (confirmed live on Contabo2: 153,429 records, 74 accounts driven
	// negative, permanent StateRoot divergence).
	cs.markWALSupersededByStateReplacement()
	cs.mu.Unlock()

	// FIX (audit recheck3, P0 #1): used to only log this and return success
	// regardless — a resync could report "done" while the EVM mirror, which
	// every eth_* RPC call and the V7 contract's own view of balances reads
	// from, silently diverged from the Go-state the rest of this function
	// just correctly resynced. The Go-state DB transaction above already
	// committed by this point (EVM is a derived mirror, not the source of
	// truth, so it can't reasonably be folded into the same SQL
	// transaction) — but the caller must still be told this didn't fully
	// succeed instead of seeing a bare "✓ Replaced local state" message.
	if err := cs.MigrateEVMFromGoState(V7_CONTRACT_ADDR); err != nil {
		return fmt.Errorf("resync: Go-state committed successfully, but EVM mirror migration failed (EVM state may now be inconsistent — re-run resync to retry the mirror step): %w", err)
	}

	fmt.Printf("[RESYNC] ✓ Replaced local state with %d accounts, %d nullifiers, %d bio-registrations from snapshot\n",
		len(snap.Accounts), len(snap.Nullifiers), len(snap.BioRegistrations))
	return nil
}

// fetchAndVerifyBlockFromPeer fetches the single block at height from
// baseURL's /api/block endpoint and cryptographically verifies it exactly
// like AddPeerBlock does for any other incoming block: recomputed hash must
// match the claimed hash, and the ECDSA signature must recover to the
// claimed proposer. This gives the same trust guarantee as accepting the
// block over normal peer gossip — no new "blind trust" is introduced by
// fetching it out-of-band via a plain GET instead.
func fetchAndVerifyBlockFromPeer(baseURL string, height int64) (*Block, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{DialContext: pinningDialer},
	}
	resp, err := client.Get(fmt.Sprintf("%s/api/block?height=%d", strings.TrimRight(baseURL, "/"), height))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("peer returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	var block Block
	if err := json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}
	if err := verifyFetchedBlock(&block, height); err != nil {
		return nil, err
	}
	return &block, nil
}

// verifyFetchedBlock is fetchAndVerifyBlockFromPeer's cryptographic-checks
// half, split out as a pure function (no network I/O) so it's directly unit
// testable without needing pinningDialer to allow a loopback test server —
// see block_ghostdag_lookup_test.go's TestVerifyFetchedBlock* cases.
func verifyFetchedBlock(block *Block, height int64) error {
	if block.Height != height {
		return fmt.Errorf("peer returned block at height %d, expected %d", block.Height, height)
	}
	if expected := calculateBlockHash(block); expected != block.Hash {
		return fmt.Errorf("hash mismatch: claimed %s, computed %s", block.Hash, expected)
	}
	if block.Signature == "" {
		return fmt.Errorf("block has no signature")
	}
	sigBytes, err := hex.DecodeString(block.Signature)
	if err != nil || len(sigBytes) != 65 {
		return fmt.Errorf("malformed signature")
	}
	hashBytes, err := hex.DecodeString(block.Hash)
	if err != nil {
		return fmt.Errorf("malformed hash: %w", err)
	}
	pubkeyBytes, err := crypto.Ecrecover(hashBytes, sigBytes)
	if err != nil {
		return fmt.Errorf("signature recovery failed: %w", err)
	}
	pubkey, err := crypto.UnmarshalPubkey(pubkeyBytes)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}
	recoveredAddr := strings.ToLower(crypto.PubkeyToAddress(*pubkey).Hex())
	if recoveredAddr != strings.ToLower(block.Proposer) {
		return fmt.Errorf("signature mismatch: signer %s, proposer %s", recoveredAddr, block.Proposer)
	}
	return nil
}

// filterAndVerifySiblings is fetchAndVerifySiblingsAtHeight's pure
// filtering+verification half, split out (mirroring verifyFetchedBlock's own
// split from fetchAndVerifyBlockFromPeer) so it's directly unit testable
// without needing pinningDialer to allow a loopback test server. Keeps only
// candidates at exactly height, skips the genesis block, and drops any
// candidate that fails cryptographic verification — a bad/malformed sibling
// from a misbehaving peer is silently skipped, not fatal to the others.
func filterAndVerifySiblings(candidates []*Block, height int64) []*Block {
	var verified []*Block
	for _, b := range candidates {
		if b.Height != height || b.IsGenesis {
			continue
		}
		if err := verifyFetchedBlock(b, height); err != nil {
			continue // best-effort — see fetchAndVerifySiblingsAtHeight's own comment
		}
		verified = append(verified, b)
	}
	return verified
}

// fetchAndVerifySiblingsAtHeight fetches every block at exactly height
// (not just the canonical one) from baseURL and independently verifies
// each — see SeedTrustedCheckpoint's own FIX comment for why this exists.
// Uses /api/blocks?min_height=height-1, which returns every DAG sibling in
// range (unlike /api/block?height=, which resolves to a single canonical
// choice via GetBlockByHeight), then filters to exactly this height. A
// per-block verification failure is skipped, not fatal to the whole call —
// the canonical block (verified separately by fetchAndVerifyBlockFromPeer)
// is what actually matters for the checkpoint itself; siblings are a
// best-effort completeness improvement.
func fetchAndVerifySiblingsAtHeight(baseURL string, height int64) ([]*Block, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{DialContext: pinningDialer},
	}
	resp, err := client.Get(fmt.Sprintf("%s/api/blocks?min_height=%d&limit=500", strings.TrimRight(baseURL, "/"), height-1))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("peer returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	var candidates []*Block
	if err := json.Unmarshal(body, &candidates); err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}
	verified := filterAndVerifySiblings(candidates, height)
	return verified, nil
}

// SeedTrustedCheckpoint is the durable-resync speedup: instead of the
// genesis-only reset RefreshBootHeightAfterSnapshotImport otherwise performs
// after a RESYNC_FROM_SNAPSHOT, this fetches and independently verifies the
// REAL block at the just-imported snapshot's height from primaryURL, and if
// it checks out, persists it as the new trusted root -- so the resync only
// needs to replay the handful of blocks produced since, not the entire
// chain since genesis.
//
// FIX (durable fix, 2026-07-03): before this, EVERY auto-heal/manual resync
// re-walked the full historical block-header chain from height 0, no matter
// how far the chain had actually progressed (140,000+ blocks tonight) --
// slow, and prone to exactly the historical-orphan-wall problem this whole
// session kept re-encountering (permanently-lost early blocks needing
// ALLOW_RUNTIME_ORPHAN_BRIDGE + a 15-minute abandon timer per hash). The
// chain already maintains a hard finality checkpoint that every honest node
// treats as a trust boundary no reorg may cross -- RESYNC just never used
// it, unconditionally rebuilding from genesis regardless. This seeds that
// same trust boundary directly: unlike a synthetic-checkpoint stub (a
// height/hash-only placeholder with BlueScore forced to 0, carrying no
// verified content), the checkpoint block seeded here is the REAL,
// signature-verified block -- there is nothing "unverified" about it, so it
// never counts toward UnverifiedSyntheticCheckpointCount and never gates
// production the way a stub above bootHeight would.
//
// Called after a successful ResyncFromSnapshotURL, before
// RefreshBootHeightAfterSnapshotImport reads max_block_height back out.
// Returns false (with no partial side effects beyond logging) on any
// failure -- the caller falls back to the existing, slower-but-always-safe
// genesis walk automatically, since max_block_height stays at the "0" value
// ResyncFromSnapshotURL already wrote.
func (dag *BlockDAG) SeedTrustedCheckpoint(primaryURL string) bool {
	if primaryURL == "" || dag.state == nil || dag.state.db == nil {
		return false
	}
	heightStr := dag.state.getConfigValueDB("snapshot_import_height")
	var height int64
	if _, err := fmt.Sscanf(heightStr, "%d", &height); err != nil || height <= 0 {
		return false
	}
	block, err := fetchAndVerifyBlockFromPeer(primaryURL, height)
	if err != nil {
		fmt.Printf("[RESYNC] ⚠ Could not seed a trusted checkpoint at height %d (%v) — falling back to a full genesis resync\n", height, err)
		return false
	}
	if err := dag.state.SaveBlockToDB(block, true); err != nil {
		fmt.Printf("[RESYNC] ⚠ Verified checkpoint block at height %d but could not persist it (%v) — falling back to a full genesis resync\n", height, err)
		return false
	}
	if err := dag.state.setConfigValueDB("max_block_height", fmt.Sprintf("%d", block.Height)); err != nil {
		fmt.Printf("[RESYNC] ⚠ Verified and saved checkpoint block at height %d but could not update max_block_height (%v) — falling back to a full genesis resync\n", height, err)
		return false
	}
	dag.state.SetFinalizedCheckpoint(block.Hash, block.Height, block.BlueScore)
	fmt.Printf("[RESYNC] ✓ Seeded trusted checkpoint at height %d (%s..., blue_score %d) from %s — resync will replay forward from here instead of genesis\n",
		block.Height, block.Hash[:min(16, len(block.Hash))], block.BlueScore, primaryURL)

	// FIX (P0, 2026-07-04 — true root cause of a night of persistent
	// non-convergence, found live via a byte-for-byte reproduction against
	// the real primary): only the CANONICAL block at the checkpoint height
	// was ever seeded. GHOSTDAG routinely has multiple concurrently-produced
	// blocks (siblings) at the same height, and a later block's merge set
	// can legitimately reference ANY of them as a parent — not just the one
	// GetBlockByHeight picks as canonical. Confirmed live: the very first
	// blocks fetched above a fresh checkpoint referenced a sibling at the
	// checkpoint's OWN height that was never seeded, so ghostdagBlockLookup
	// could never resolve it — permanently orphaning that block and
	// everything built on top, no matter how many sync passes ran, because
	// deepScan's floor deliberately never re-fetches anything at or below
	// the checkpoint (see deepScanFloor's own comment for why that's
	// otherwise correct and necessary). Best-effort: a failure here doesn't
	// invalidate the checkpoint itself, just means fewer siblings are
	// available to resolve against — strictly a completeness improvement
	// over seeding only the canonical block.
	//
	// FIX (2026-07-07 — closes the residual gap this comment already flagged
	// above): querying only primaryURL misses a sibling that simply hadn't
	// propagated to primaryURL YET at the exact moment of this resync — an
	// ordinary cross-provider timing race, not a defect in primaryURL. Once
	// isFinalityViolation's hard wall (finalityHeightSlack=50 blocks) moves
	// past this height, a sibling missed here can never be recovered —
	// maybeAdvanceFinalizedCheckpoint only ever advances forward, and
	// AddPeerBlock hardens it on every accepted peer block with no equivalent
	// "corroborated by more than one source" gate. Widening the sibling fetch
	// to every OTHER already-configured/known seed — seedURLs() (
	// PRIMARY_NODE_URL + PRIMARY_NODE_URLS, the existing dynamic
	// bootstrap-resilience mechanism StartPeerDiscovery already relies on;
	// deliberately NOT the legacy static PEER_NODES list) plus whatever
	// GlobalPeerRegistry already knows about — costs nothing on the common
	// single-seed deployment (both collapse back to just primaryURL) and
	// closes the gap wherever more than one seed/peer is actually reachable.
	sources := []string{primaryURL}
	seenSource := map[string]bool{strings.TrimRight(primaryURL, "/"): true}
	addSource := func(u string) {
		u = strings.TrimRight(u, "/")
		if u == "" || seenSource[u] {
			return
		}
		seenSource[u] = true
		sources = append(sources, u)
	}
	selfURL := strings.TrimRight(NormalizeNodeURL(os.Getenv("SELF_URL")), "/")
	for _, u := range seedURLs(selfURL) {
		addSource(u)
	}
	for _, u := range GlobalPeerRegistry.ActivePeers(selfURL) {
		addSource(u)
	}

	seenSiblingHash := map[string]bool{block.Hash: true}
	saved := 0
	anyErr := false
	for _, src := range sources {
		siblings, sibErr := fetchAndVerifySiblingsAtHeight(src, height)
		if sibErr != nil {
			anyErr = true
			fmt.Printf("[RESYNC] ⚠ Could not fetch sibling blocks at checkpoint height %d from %s (%v)\n", height, src, sibErr)
			continue
		}
		for _, sib := range siblings {
			if seenSiblingHash[sib.Hash] {
				continue // already saved (the canonical block, or a sibling another source also returned)
			}
			seenSiblingHash[sib.Hash] = true
			if err := dag.state.SaveBlockToDB(sib, true); err != nil {
				fmt.Printf("[RESYNC] ⚠ Could not persist sibling block %s... at checkpoint height %d (%v)\n", sib.Hash[:min(16, len(sib.Hash))], height, err)
				continue
			}
			saved++
		}
	}
	if saved > 0 {
		fmt.Printf("[RESYNC] ✓ Also seeded %d sibling block(s) at checkpoint height %d (across %d source(s)) — later blocks referencing them as a merge parent can now resolve\n", saved, height, len(sources))
	} else if anyErr {
		fmt.Printf("[RESYNC] ⚠ No siblings recovered at checkpoint height %d from any of %d source(s) — checkpoint still valid, but a later block referencing a non-canonical sibling here may orphan permanently\n", height, len(sources))
	}
	return true
}

// replaceInMemoryFromSnapshotLocked overwrites cs.accounts/cs.pool/cs.nullifiers
// with snap's contents. Caller must hold cs.mu.
func (cs *ChainState) replaceInMemoryFromSnapshotLocked(snap *StateSnapshot) {
	fresh := newShardedAccounts()
	for _, acc := range snap.Accounts {
		acc.Address = strings.ToLower(acc.Address)
		fresh.Set(acc.Address, acc)
	}
	cs.accounts = fresh
	if snap.Pool != nil {
		cs.pool = snap.Pool
	}
	cs.nullifiers = make(map[string]string, len(snap.Nullifiers))
	for nullifier, wallet := range snap.Nullifiers {
		cs.nullifiers[nullifier] = strings.ToLower(wallet)
	}
}

// resyncBackwardsTolerance is how far behind local height a resync snapshot may
// be before it is refused outright.
//
// A snapshot is always a little stale by the time it is validated and applied,
// and a node under load keeps advancing during the fetch, so a small backwards
// step is normal and harmless — those blocks come back from peers immediately.
// 1,000 blocks is roughly seventeen minutes at BLOCK_TIME=1s: far more slack
// than any real fetch needs, and still nowhere near the millions that separate a
// healthy validator from an empty one.
const resyncBackwardsTolerance = 1000

// localChainHeightForResync reports this node's own chain height from durable
// state, for the regression guard above.
//
// Deliberately read from the DB rather than the BlockDAG: this runs during
// startup resync, before the DAG is loaded, and ChainState has no handle on it
// anyway. max_block_height is the same counter NewBlockchain uses to establish
// bootHeight, so it is exactly "what this node believes it already has".
// Returns 0 when it cannot be determined, which disables the guard rather than
// blocking a legitimate first-time bootstrap on a node that has no height yet.
func (cs *ChainState) localChainHeightForResync() int64 {
	if cs.db == nil {
		return 0
	}
	raw := cs.getConfigValueDB("max_block_height")
	if raw == "" {
		return 0
	}
	h, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || h < 0 {
		return 0
	}
	return h
}

// resyncRegressionMessage is the refusal an operator actually reads. Split out
// so its content can be pinned by a test: meeting this at 3am, the numbers and
// the deliberate override are the whole point of the message.
func resyncRegressionMessage(peerURL string, snapHeight, localHeight int64) string {
	behind := localHeight - snapHeight
	return fmt.Sprintf(
		"refusing resync: the snapshot from %s is at height %d, %d blocks BEHIND this node's %d — "+
			"resync replaces local state wholesale, so this would discard %d blocks of real history. "+
			"If that rollback is intended, set ALLOW_RESYNC_REGRESSION=true",
		peerURL, snapHeight, behind, localHeight, behind)
}
