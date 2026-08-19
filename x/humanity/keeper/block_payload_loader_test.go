package keeper

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// EVERY loader must read BOTH stored forms of a block's transaction list.
//
// THE INCIDENT THIS PINS (2026-08-19, Contabo2 down for ~20 minutes).
//
// block_payload_codec.go can store a block's transactions gzipped in
// transactions_z, and when it does it deliberately leaves the plain
// `transactions` column EMPTY — exactly one of the two carries the payload. Its
// own header names the hazard in as many words: "Code that predates this column
// reads that as a block with NO transactions — and reads it SILENTLY, because
// the existing loader discards json.Unmarshal's error."
//
// Only ONE of the six loaders in evm_storage.go had ever been migrated to
// decodeBlockPayload (LoadBlocksSinceFromDB). The other five selected the plain
// column alone. The audit fix for the silent-loss half of that hazard
// error-checked those five — and turned a silent wrong answer into a fatal one:
// json.Unmarshal("") returns "unexpected end of JSON input", so on a box with
// compression enabled EVERY compressed row was skipped, the DAG could not be
// rebuilt from Postgres at all, and the node came up answering HTTP while stuck
// at height 0. The deploy's own verify step caught it ("height 0 -> 0 over
// 60s"), which is the only reason it was 20 minutes and not the next morning.
//
// Both directions of that mistake are the same root cause: a loader that knows
// about one storage form and not the other. This test is deliberately a
// source-level check rather than a behavioural one, because it must hold for
// loaders nobody has written yet, and because it has to run in ordinary CI —
// the compressed path only ever bites where a real Postgres holds real
// compressed rows, which is precisely where nobody is watching.
func TestBlockPayloadLoaders_EveryLoaderReadsBothStoredForms(t *testing.T) {
	src, err := os.ReadFile("evm_storage.go")
	if err != nil {
		t.Fatalf("read evm_storage.go: %v", err)
	}
	text := string(src)

	// Find every SELECT that reads the transactions column, and require the
	// compressed column in the same select list. Anchored on "signature,
	// transactions" because that is the shape all of these share; a future
	// loader that words its select list differently is caught by the second
	// half of this test instead.
	selects := regexp.MustCompile(`signature, transactions[^\n]*`).FindAllString(text, -1)
	if len(selects) == 0 {
		t.Fatal("no SELECT reading the transactions column found at all — this test needs updating")
	}
	for _, s := range selects {
		// The INSERT statements name the same two columns; they are writers,
		// not readers, and they already write both.
		if strings.Contains(s, "selected_parent, blue_score, blues, replayed, transactions_z)") {
			continue
		}
		if !strings.Contains(s, "transactions_z") {
			t.Errorf("a loader selects the plain transactions column without transactions_z:\n    %s\n"+
				"A block stored compressed leaves `transactions` EMPTY on purpose. Reading only the "+
				"plain column returns such a block as having no transactions (silent loss) or fails to "+
				"parse it (Contabo2, height 0). Add COALESCE(transactions_z, ''::bytea) and decode with "+
				"decodeBlockPayload.", strings.TrimSpace(s))
		}
	}

	// And nothing may go back to unmarshalling the plain column by hand.
	for _, forbidden := range []string{
		"json.Unmarshal([]byte(txsRaw), &b.Transactions)",
		"json.Unmarshal([]byte(txsRaw), &block.Transactions)",
	} {
		// Ignore the comments that quote the old line while explaining it.
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(line, forbidden) {
				t.Errorf("a loader unmarshals the plain transactions column directly:\n    %s\n"+
					"decodeBlockPayload is the only function that knows both stored forms — and knows "+
					"that an empty plain column means 'compressed elsewhere' or 'no transactions', "+
					"never 'corrupt'.", trimmed)
			}
		}
	}
}

// The property the loaders depend on, stated as a test so it cannot drift:
// an empty plain column is NOT an error. This is the exact call every migrated
// loader now makes for a compressed row before it reaches the gzip branch, and
// the one whose old behaviour (json.Unmarshal("") -> error) took the node down.
func TestBlockPayloadLoaders_EmptyPlainColumnIsNotCorruption(t *testing.T) {
	txs, err := decodeBlockPayload("", nil)
	if err != nil {
		t.Fatalf("an empty payload must decode as 'no transactions', not as an error: %v", err)
	}
	if len(txs) != 0 {
		t.Fatalf("empty payload decoded to %d transactions", len(txs))
	}

	// And the compressed form must survive the same call with the plain column
	// empty — the shape every row written with compression on actually has.
	want := []Transaction{
		{Type: "transfer", Wallet: "0xaa", To: "0xbb", Amount: 12.5},
		{Type: "register_human", Wallet: "0xcc"},
	}
	raw := mustMarshalTxs(t, want)
	z, err := compressBlockPayload(raw)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	got, err := decodeBlockPayload("", z)
	if err != nil {
		t.Fatalf("a compressed payload with an empty plain column must decode: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("decoded %d transactions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Wallet != want[i].Wallet || got[i].Amount != want[i].Amount {
			t.Errorf("transaction %d round-tripped as %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The end-to-end version, against a real Postgres: write a block the way a node
// with compression enabled writes it, then read it back through every loader
// that can return a single block and assert the transactions are still there.
//
// This is the test that reproduces the incident directly. It needs a real
// database because the bug lives in the SQL select list, which no in-memory
// fake exercises.
func TestBlockPayloadLoaders_CompressedBlockRoundTripsThroughEveryLoader_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)

	// Write the way a compression-enabled node writes.
	t.Setenv("AEQUITAS_COMPRESS_BLOCK_PAYLOAD", "1")
	if !blockPayloadCompressionEnabled() {
		t.Fatal("compression did not switch on — the rest of this test would prove nothing")
	}

	state := NewChainState("unused-payload-loader-test.json")
	if !state.useDB {
		t.Fatal("expected a live PostgreSQL connection (check DATABASE_URL)")
	}
	ensureRealDBSchema(t, state.db)

	height := int64(910000000 + os.Getpid()%1000)
	hash := fmt.Sprintf("%064x", height)
	blk := &Block{
		Hash:      hash,
		Height:    height,
		Proposer:  "0x00000000000000000000000000000000000c0de1",
		Timestamp: 1787097600,
		Humans:    3,
		StateRoot: "payload-loader-test",
		Signature: "sig",
		Transactions: []Transaction{
			{Type: "transfer", Wallet: "0xaa", To: "0xbb", Amount: 7.25},
			{Type: "transfer", Wallet: "0xbb", To: "0xcc", Amount: 0.5},
		},
		ParentHashes: []string{},
		Blues:        []string{},
	}
	t.Cleanup(func() {
		state.db.Exec(`DELETE FROM chain_blocks WHERE hash = $1`, hash)
	})

	if err := state.SaveBlockToDB(blk, true); err != nil {
		t.Fatalf("SaveBlockToDB: %v", err)
	}

	// Confirm the row really is in the compressed shape — otherwise this test
	// would pass for the wrong reason on a build where compression is off.
	var plain string
	var z []byte
	if err := state.db.QueryRow(
		`SELECT transactions, COALESCE(transactions_z,''::bytea) FROM chain_blocks WHERE hash = $1`, hash,
	).Scan(&plain, &z); err != nil {
		t.Fatalf("read the row back: %v", err)
	}
	if plain != "" {
		t.Fatalf("the plain column holds %q — the row is not in the compressed shape this test exists for", plain)
	}
	if len(z) == 0 {
		t.Fatal("transactions_z is empty — nothing was stored compressed")
	}

	check := func(name string, got *Block) {
		t.Helper()
		if got == nil {
			t.Errorf("%s returned nil for a compressed block — this is the Contabo2 failure: "+
				"the DAG cannot be rebuilt from Postgres and the node comes up stuck at height 0", name)
			return
		}
		if len(got.Transactions) != len(blk.Transactions) {
			t.Errorf("%s returned %d transactions, want %d — a compressed payload was read as an "+
				"empty block, which still hashes correctly through its TxRoot, so nothing downstream "+
				"would object either", name, len(got.Transactions), len(blk.Transactions))
			return
		}
		for i, want := range blk.Transactions {
			if got.Transactions[i].Amount != want.Amount || got.Transactions[i].Wallet != want.Wallet {
				t.Errorf("%s transaction %d came back as %+v, want %+v", name, i, got.Transactions[i], want)
			}
		}
	}

	check("LoadBlockFromDBByHash", state.LoadBlockFromDBByHash(hash))
	check("LoadBlockFromDBByHeight", state.LoadBlockFromDBByHeight(height))

	byHash, err := state.LoadBlocksByHashesFromDB([]string{hash})
	if err != nil {
		t.Errorf("LoadBlocksByHashesFromDB: %v", err)
	} else if len(byHash) != 1 {
		t.Errorf("LoadBlocksByHashesFromDB returned %d blocks for one existing hash, want 1 — "+
			"a compressed row was skipped as unreadable", len(byHash))
	} else {
		check("LoadBlocksByHashesFromDB", byHash[0])
	}

	loaded, err := state.LoadBlocksFromDB(height)
	if err != nil {
		t.Errorf("LoadBlocksFromDB: %v", err)
	} else if got := loaded[hash]; got == nil {
		t.Error("LoadBlocksFromDB (the STARTUP loader) dropped the compressed block entirely — " +
			"this is precisely why Contabo2 answered HTTP at height 0")
	} else {
		check("LoadBlocksFromDB", got)
	}

	// minHeight is EXCLUSIVE here (selectBlocksSince: `b.Height > minHeight`),
	// unlike LoadBlocksFromDB's inclusive bound — hence height-1.
	since, err := state.LoadBlocksSinceFromDB(height-1, "", 10)
	if err != nil {
		t.Errorf("LoadBlocksSinceFromDB: %v", err)
	} else {
		var found *Block
		for _, b := range since {
			if b.Hash == hash {
				found = b
			}
		}
		check("LoadBlocksSinceFromDB", found)
	}
}
