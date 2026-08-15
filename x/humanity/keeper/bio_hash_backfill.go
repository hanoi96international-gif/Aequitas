package keeper

import (
	"fmt"
)

// backfillBioHashesOnce repopulates the bio_hashes index for registrations that
// happened while the table did not exist.
//
// Context (audit 2026-08-15): bio_hashes was read, written, counted and deleted
// by this codebase but never created — see its CREATE in initDBTables. Every
// SaveBioHash during that whole period failed and was logged as a warning, so
// creating the table now leaves it correct but EMPTY, and the duplicate-check
// it feeds (registerOnV7's GetWalletByStoredBioHash) would stay blind to every
// human who registered before today. On the live network that is all 15 of
// them.
//
// The entries are recoverable because the key is derived, not random:
// SaveBioHash stores keccak256(leftPad32(bioHash)) — exactly what
// computeBioHashKeyFromBioHash produces — and bio_registrations kept the raw
// decimal bioHash for every registration that came through the API path. So
// the same key can be recomputed here rather than guessed.
//
// Runs only when the table is empty, so this is a one-time repair and not a
// full scan of bio_registrations on every boot (that table grows with the
// human count). ON CONFLICT DO NOTHING keeps it safe if it ever does re-run.
//
// A registration whose bio_registrations row has no bio_hash (block-replay
// rows store it empty by design — see block.go's register_human case) cannot
// be recovered here and is skipped; the nullifier, which is the actual
// guarantee, covers those regardless.
//
// MEASURED ON THE LIVE NETWORK (2026-08-15, after this ran on both validators):
// it recovered nothing, and that is the correct answer rather than a failure.
// All 15 humans registered against the previous primary and reached Contabo1
// and Contabo2 as BLOCKS, so their bio_registrations rows carry the empty
// bio_hash that replay writes by design, and SaveBioHash — which only exists on
// the API registration path — never ran on either box for them. The raw
// biometric hash simply never reached these nodes; only the nullifier did, and
// that is one-way. So this index cannot be reconstructed for pre-migration
// humans by any means, here or elsewhere, and it fills from the next
// API-path registration onward.
//
// The consequence is worth stating plainly instead of papering over: for those
// 15, the chain-side bio_hashes duplicate check and the proof servers' own
// caches (7 of 15 — they are fresh databases that only saw post-migration
// registrations) are both incomplete. The authoritative check is unaffected —
// all 15 nullifiers are present on-chain, replayed, and claimed atomically with
// the registration — so a second registration attempt by any of them still
// fails. It fails later and with a less helpful message than it would with a
// warm cache, which is a UX cost, not a security one.
func (cs *ChainState) backfillBioHashesOnce() {
	if cs.db == nil {
		return
	}
	var existing int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM bio_hashes`).Scan(&existing); err != nil {
		fmt.Printf("[BIO-BACKFILL] ✗ could not read bio_hashes (does the table exist?): %v\n", err)
		return
	}
	if existing > 0 {
		return // already populated — nothing to repair
	}

	rows, err := cs.db.Query(
		`SELECT bio_hash, wallet_address FROM bio_registrations
		 WHERE bio_hash IS NOT NULL AND bio_hash <> ''`)
	if err != nil {
		fmt.Printf("[BIO-BACKFILL] ✗ could not read bio_registrations: %v\n", err)
		return
	}
	type entry struct{ key, wallet string }
	var entries []entry
	var unparsable int
	for rows.Next() {
		var bioHash, wallet string
		if err := rows.Scan(&bioHash, &wallet); err != nil {
			continue
		}
		key, keyErr := computeBioHashKeyFromBioHash(bioHash)
		if keyErr != nil {
			unparsable++
			continue
		}
		entries = append(entries, entry{key: key, wallet: wallet})
	}
	rows.Close()

	if len(entries) == 0 {
		// Always say so. An earlier version of this function returned silently
		// when there was nothing to recover AND nothing unparsable — i.e. in
		// the single most likely case, an empty bio_registrations.bio_hash
		// column — which would have left "chain_bio_hashes: 0" looking exactly
		// as unexplained as the bug this whole change exists to end. Reporting
		// a zero result is not noise; it is the difference between "repaired,
		// nothing to recover" and "did not run".
		fmt.Printf("[BIO-BACKFILL] nothing to recover: bio_registrations has no usable bio_hash values "+
			"(%d row(s) present but not a decimal integer). The index starts empty and fills from the next registration on.\n",
			unparsable)
		return
	}

	inserted := 0
	for _, e := range entries {
		res, execErr := cs.db.Exec(
			`INSERT INTO bio_hashes (hash, wallet_address) VALUES ($1, $2) ON CONFLICT (hash) DO NOTHING`,
			e.key, e.wallet)
		if execErr != nil {
			fmt.Printf("[BIO-BACKFILL] ✗ could not insert %s: %v\n", e.wallet, execErr)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	fmt.Printf("[BIO-BACKFILL] ✓ rebuilt the chain's bio_hashes index: %d entr(ies) recovered from bio_registrations (%d skipped as unparsable)\n",
		inserted, unparsable)
}
