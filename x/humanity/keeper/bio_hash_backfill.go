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
		if unparsable > 0 {
			fmt.Printf("[BIO-BACKFILL] no recoverable entries (%d row(s) had a bio_hash that is not a decimal integer)\n", unparsable)
		}
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
