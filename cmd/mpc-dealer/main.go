// Command mpc-dealer produces Beaver multiplication triples and writes one
// file per party.
//
// # WHY THIS IS A SEPARATE PROGRAM
//
// A Beaver triple is a correlated secret: party 0 holds a_0, party 1 holds a_1,
// a_0+a_1 = a, and c = a*b. They must be produced ONCE and their rows handed
// out. A party that generates its own gets a row from a different draw; the
// shapes still match, nothing objects, and every comparison silently becomes
// arithmetic noise. The node used to do exactly that, which is why loading
// replaced generating and why this program exists.
//
// # WHERE IT MUST AND MUST NOT RUN
//
// Whoever runs this knows a, b and c in the clear. That alone is harmless. But
// anyone who knows the triples AND can observe the blinded values a comparison
// opens can recover that comparison's inputs — the biometric templates.
//
// So this must run:
//
//   - NOT on a validator that takes part in the computation,
//   - NOT anywhere with network visibility of the traffic between the parties,
//   - on a machine whose copy of the output is destroyed after delivery.
//
// None of that can be enforced from inside this program. It is stated here, in
// the output, and in DEPLOY.md, because it is the whole of the remaining trust
// assumption.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

func main() {
	count := flag.Int("count", 0, "how many triples to generate per party")
	parties := flag.Int("parties", 0, "how many parties (must match the committee size)")
	outDir := flag.String("out", "", "directory to write the per-party files into")
	force := flag.Bool("force", false, "overwrite existing files (see the warning before using)")
	flag.Parse()

	if err := run(*count, *parties, *outDir, *force); err != nil {
		fmt.Fprintf(os.Stderr, "mpc-dealer: %v\n", err)
		os.Exit(1)
	}
}

func run(count, parties int, outDir string, force bool) error {
	if count <= 0 {
		return fmt.Errorf("-count must be positive")
	}
	if parties < 2 {
		return fmt.Errorf("-parties is %d; with fewer than two, one machine holds every share "+
			"and can reconstruct every template", parties)
	}
	if outDir == "" {
		return fmt.Errorf("-out is required")
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return err
	}

	paths := make([]string, parties)
	for i := range paths {
		paths[i] = filepath.Join(outDir, fmt.Sprintf("triples-party-%d.bin", i))
	}

	// Refuse to overwrite unless told twice.
	//
	// Overwriting is how triples get used twice. If a party still holds the old
	// file and its consumed-offset counter, a new file written to the same path
	// resets nothing on that party — it simply continues at the old offset into
	// different data, and the parties fall out of step. If instead the offset is
	// cleared, the party replays positions it already used, and a reused triple
	// stops blinding and leaks the difference of the two secrets it was used
	// on. Neither failure announces itself.
	if !force {
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return fmt.Errorf("%s already exists. Overwriting is how triples get reused: a "+
					"party continues at its old offset into new data, or replays offsets it has "+
					"already spent. Write to a fresh directory, or pass -force only if you are "+
					"certain every party's file and consumed-offset counter are being replaced "+
					"together", p)
			}
		}
	}

	fmt.Printf("Generating %d triples for %d parties...\n", count, parties)
	rows, err := mpc.GenerateTriples(count, parties)
	if err != nil {
		return err
	}

	// Check the correlation before anything is written. A dealer is the only
	// place this is checkable — it is the only holder of every row — and a
	// silently broken batch would surface on the parties as comparisons that
	// never succeed, with nothing pointing back to here.
	if err := verifyRows(rows); err != nil {
		return fmt.Errorf("refusing to write a batch that does not satisfy c = a*b: %w", err)
	}
	fmt.Printf("Verified: all %d triples satisfy c = a*b across %d parties.\n", count, parties)

	for i, p := range paths {
		blob := mpc.EncodeTriples(rows[i])
		if err := os.WriteFile(p, blob, 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", p, err)
		}
		sum := sha256.Sum256(blob)
		fmt.Printf("  %s  %d bytes  sha256=%s\n", p, len(blob), hex.EncodeToString(sum[:8]))
	}

	fmt.Print(`
NEXT STEPS — the security of every template depends on these:

  1. Deliver each file to EXACTLY ONE party, over a channel that keeps it
     confidential. Set MPC_TRIPLE_FILE on that party to its own file.

  2. Give no party any file but its own. Two rows for the same triple are
     enough to reconstruct it, and a triple plus the traffic between the
     parties is enough to reconstruct the templates that traffic was about.

  3. DESTROY this machine's copies once delivered, including this directory
     and any backups or snapshots that captured it.

  4. Do not run this on a validator that takes part in the computation, and
     do not run it anywhere that can observe the network between the parties.
     Nothing here can check that; it is the remaining trust assumption.

`)
	return nil
}

// verifyRows reconstructs every triple and checks c = a*b.
func verifyRows(rows [][]mpc.Triple) error {
	if len(rows) == 0 {
		return fmt.Errorf("no rows")
	}
	n := len(rows[0])
	for i, r := range rows {
		if len(r) != n {
			return fmt.Errorf("party %d has %d triples, party 0 has %d", i, len(r), n)
		}
	}
	for k := 0; k < n; k++ {
		var a, b, c mpc.Element
		for _, r := range rows {
			a = mpc.Add(a, r[k].A)
			b = mpc.Add(b, r[k].B)
			c = mpc.Add(c, r[k].C)
		}
		if mpc.Mul(a, b) != c {
			return fmt.Errorf("triple %d: a*b != c", k)
		}
	}
	return nil
}
