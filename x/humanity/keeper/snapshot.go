package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	Version          int                       `json:"version"`
	Timestamp        int64                     `json:"timestamp"`
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
	Height           int64                     `json:"height"`
	Accounts         []*AccountState           `json:"accounts"`
	Pool             *PoolState                `json:"pool"`
	Nullifiers          map[string]string         `json:"nullifiers"`           // nullifier → wallet (sentinel 0x01 if wallet redacted)
	NullifiersRedacted  bool                      `json:"nullifiers_redacted"`  // true when wallet addresses replaced with sentinel (public tier)
	BioRegistrations    []SnapshotBioRegistration `json:"bio_registrations"`
	ChainConfig         map[string]string         `json:"chain_config,omitempty"` // critical timing keys for secondary state sync
	Signature           string                    `json:"signature,omitempty"`  // ECDSA over SHA256(JSON without this field)
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
	accounts := make([]*AccountState, 0, len(cs.accounts))
	for _, acc := range cs.accounts {
		cp := *acc
		accounts = append(accounts, &cp)
	}
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
	cs.activeTx = tx
	prevPool := cs.pool
	poolChanged := false
	for _, acc := range snap.Accounts {
		acc.Address = strings.ToLower(acc.Address)
		if existingHumans > 0 && systemAddresses[acc.Address] {
			continue
		}
		if _, exists := cs.accounts[acc.Address]; !exists {
			cs.accounts[acc.Address] = acc
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
			delete(cs.accounts, acc.Address)
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
				if err := cs.saveAccountToDB(acc); err != nil {
					return fmt.Errorf("saving account %s: %w", acc.Address, err)
				}
			}
			if err := cs.savePoolToDB(); err != nil {
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
					if err := cs.setConfigValue(key, val); err != nil {
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
				if err := cs.setConfigValue("snapshot_import_height", fmt.Sprintf("%d", snap.Height)); err != nil {
					return fmt.Errorf("setting snapshot_import_height: %w", err)
				}
			}
			return nil
		}()

		// FIX (audit 2026-06-28 recheck 4, P0-1/P0-2): cs.mu stays held from
		// before cs.activeTx was set above through this commit/rollback
		// decision — cleared and unlocked together below, never separately.
		cs.activeTx = nil
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
	backupAccounts := make(map[string]*AccountState, len(cs.accounts))
	for addr, acc := range cs.accounts {
		accCopy := *acc
		backupAccounts[addr] = &accCopy
	}
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

	cs.activeTx = tx
	cs.replaceInMemoryFromSnapshotLocked(&snap)

	fail := func(stepErr error) error {
		restoreInMemory()
		cs.activeTx = nil
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
	if _, err := tx.Exec(`DELETE FROM chain_blocks`); err != nil {
		return fail(fmt.Errorf("resync: could not clear chain_blocks: %w", err))
	}
	if _, err := tx.Exec(`DELETE FROM chain_accounts`); err != nil {
		return fail(fmt.Errorf("resync: could not clear chain_accounts: %w", err))
	}
	if _, err := tx.Exec(`DELETE FROM nullifiers`); err != nil {
		return fail(fmt.Errorf("resync: could not clear nullifiers: %w", err))
	}
	if _, err := tx.Exec(`DELETE FROM bio_registrations`); err != nil {
		return fail(fmt.Errorf("resync: could not clear bio_registrations: %w", err))
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
	for _, acc := range cs.accounts {
		// saveAccountToDB routes through cs.dbExec(), which returns
		// cs.activeTx (set above) instead of cs.db — joins this transaction.
		if err := cs.saveAccountToDB(acc); err != nil {
			return fail(fmt.Errorf("resync: could not save account %s: %w", acc.Address, err))
		}
	}
	if snap.Pool != nil {
		if err := cs.savePoolToDB(); err != nil {
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
		if err := cs.setConfigValue(key, val); err != nil {
			return fail(fmt.Errorf("resync: could not set config %q: %w", key, err))
		}
	}
	if snap.Height > 0 {
		if err := cs.setConfigValue("snapshot_import_height", fmt.Sprintf("%d", snap.Height)); err != nil {
			return fail(fmt.Errorf("resync: could not set snapshot_import_height: %w", err))
		}
		// Set max_block_height to 0, NOT snap.Height: dag.height is seeded
		// from max_block_height at startup, and doSyncOnce pages forward from
		// dag.height - syncOverlap. If we stored snap.Height here, the first
		// sync cycle would request blocks near snap.Height — but dag.blocks
		// only contains genesis (chain_blocks was just cleared above), so
		// every arriving block's parent hash would be missing and the entire
		// chain would orphan permanently. With max_block_height = 0, the node
		// restarts at dag.height = 0 and syncs ALL block headers sequentially
		// from genesis; replay is skipped for heights ≤ snapshot_import_height
		// (see replayTransactions' skipHeight check), so account state stays
		// correct. bootHeight is still set correctly via snapshot_import_height
		// in RefreshBootHeightAfterSnapshotImport.
		if err := cs.setConfigValue("max_block_height", "0"); err != nil {
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
		for _, fk := range []string{"finalized_height", "finalized_blue_score", "finalized_hash"} {
			if err := cs.setConfigValue(fk, "0"); err != nil {
				return fail(fmt.Errorf("resync: could not reset %s: %w", fk, err))
			}
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
	cs.activeTx = nil
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

// replaceInMemoryFromSnapshotLocked overwrites cs.accounts/cs.pool/cs.nullifiers
// with snap's contents. Caller must hold cs.mu.
func (cs *ChainState) replaceInMemoryFromSnapshotLocked(snap *StateSnapshot) {
	cs.accounts = make(map[string]*AccountState, len(snap.Accounts))
	for _, acc := range snap.Accounts {
		acc.Address = strings.ToLower(acc.Address)
		cs.accounts[acc.Address] = acc
	}
	if snap.Pool != nil {
		cs.pool = snap.Pool
	}
	cs.nullifiers = make(map[string]string, len(snap.Nullifiers))
	for nullifier, wallet := range snap.Nullifiers {
		cs.nullifiers[nullifier] = strings.ToLower(wallet)
	}
}

