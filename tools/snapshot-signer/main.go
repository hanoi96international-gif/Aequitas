// snapshot-signer recovers which address signed a node's /api/snapshot.
//
// WHY THIS EXISTS. A resync refuses to proceed unless BOOTSTRAP_SIGNER matches
// the address that actually signed the snapshot -- deliberately, since that
// check is the only thing standing between a node and accepting arbitrary
// state. The failure is silent from the operator's side: the node simply
// refuses and stays broken, which looks identical to the resync not having run
// at all. recover-contabo1-resync.yml records exactly this: its predecessor
// pointed at a decommissioned signer and failed closed.
//
// So the signer must be RECOVERED from the live snapshot, never assumed from a
// validator address or copied from another box's config. This does that by
// repeating the node's own verification (fetchAndValidateSnapshot in
// x/humanity/keeper/snapshot.go): blank the signature, marshal, sha256,
// ecrecover.
//
//	go run ./tools/snapshot-signer http://173.249.37.118:8080
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: snapshot-signer <node-base-url>")
		os.Exit(2)
	}
	base := strings.TrimRight(os.Args[1], "/")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(base + "/api/snapshot")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}

	// The node's rate limiter answers with a JSON error object rather than a
	// snapshot, which would otherwise unmarshal into an empty struct and
	// "recover" a meaningless address.
	if strings.Contains(string(body), `"error"`) && len(body) < 500 {
		fmt.Fprintf(os.Stderr, "node answered with an error, not a snapshot: %s\n", strings.TrimSpace(string(body)))
		os.Exit(1)
	}

	var snap keeper.StateSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)
		os.Exit(1)
	}
	if snap.Signature == "" {
		fmt.Fprintln(os.Stderr, "snapshot carries no signature — a resync against it would be refused")
		os.Exit(1)
	}

	// Exactly what the node does. The struct must be this package's own so the
	// marshalled field order matches byte for byte; a re-encoded generic map
	// would hash differently and recover a wrong address.
	sig := snap.Signature
	snap.Signature = ""
	unsigned, _ := json.Marshal(snap)
	snap.Signature = sig

	hash := sha256.Sum256(unsigned)
	sigBytes, err := hex.DecodeString(sig)
	if err != nil || len(sigBytes) != 65 {
		fmt.Fprintf(os.Stderr, "signature is not 65 raw bytes of hex (%d bytes, err %v)\n", len(sigBytes), err)
		os.Exit(1)
	}
	pubBytes, err := crypto.Ecrecover(hash[:], sigBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ecrecover: %v\n", err)
		os.Exit(1)
	}
	pub, err := crypto.UnmarshalPubkey(pubBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pubkey: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("node          %s\n", base)
	fmt.Printf("height        %d\n", snap.Height)
	fmt.Printf("BOOTSTRAP_SIGNER=%s\n", strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()))
}
