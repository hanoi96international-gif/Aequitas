# Open tasks

Written 2026-08-20. This exists because "is MPC done?" could not be answered
from the repository: the code is complete and its tests are green, which looks
like done, while no validator runs any of it. Every item below states what is
actually true in production, not what the code supports.

Verify before trusting an entry. Where a claim came from a live read, the read
is named so it can be repeated.

---

## 1. MPC is live on both validators (was: built but not activated)

**Done 2026-08-20, in this order:**

1. `mpc-triples-distribute.yml` generated both dealer rows in one CI run and
   delivered exactly one to each box — Contabo1 party 0, Contabo2 party 1, by
   ascending signing address, checksums verified, each box checked for the
   ABSENCE of the other's row.
2. `mpc-activate.yml` set `MPC_ENABLED`, `MPC_COMMITTEE_SIZE=2` and
   `MPC_TRIPLE_FILE`, mounted the row at `/data/mpc`, and recreated both
   containers — sequenced, never both at once, since two validators restarting
   together stops the chain.
3. `mpc-client-token.yml` set one shared `MPC_CLIENT_TOKEN` on both.

Both nodes now log, after restart:

    [MPC] serving /mpc/exchange as 0x…; committee of 2 drawn from the chain,
    membership resolved per registration

**Discovering mode was chosen deliberately.** `MPC_PEERS` is left empty, so the
committee comes from the chain's peer registry and a new validator becomes
eligible by advertising `mpc_ready` — no edit on any existing box. The
alternative writes every peer out by hand with a per-node party index, which
makes adding a validator an O(n) manual change across machines that are not all
owned by one person.

**Credential exposure, 2026-08-20.** The activation workflow confirmed its work
with `grep -E '^MPC_' /root/.aequitas.env`, and `MPC_CLIENT_TOKEN` shares that
prefix — so the first token was printed in clear text into a workflow log
readable by anyone with repository access. It was rotated and the output now
prints `=<set>`. Worth generalising: a prefix filter written to confirm
configuration will match a secret that shares the prefix.

**Still open here:** `MPC_REQUIRED` is deliberately unset, so the gate is
advisory — see item 2. And the caller has not been wired: the matching service
needs the same token before it can submit anything. Read it off a box with
`grep '^MPC_CLIENT_TOKEN=' /root/.aequitas.env`.

---

## 2. The matching threshold has never been calibrated

**State:** `MinPairsForCalibration = 1000` labelled pairs
(`x/humanity/mpc/calibrate.go`). No calibration has been run against real
captures, so `RecommendThreshold`'s FAR budget has no measured input.

**Why it matters:** An uncalibrated threshold either rejects real people or
accepts duplicates, and which one it does is currently unknown rather than
chosen. `MPC_REQUIRED=true` must not be set before this — enforcing a gate
whose threshold is a guess is worse than leaving the gate advisory, because
the failure lands on applicants who cannot appeal.

---

## 3. Two parties, one control plane

**State:** Contabo1 belongs to the user, Contabo2 to their spouse, so ownership
is genuinely separate. But both are deployed from the same GitHub repository,
through one workflow, using two SSH secrets held in the same secret store, and
both machines are at the same provider.

**Why it matters:** MPC's guarantee is that no single party sees a whole
template. Two parties whose deploy path converges on one set of credentials is
one compromise away from being one party. This is a real limit on what the
current setup can honestly claim, not a theoretical one.

**To do:** a third party operated by someone else, on infrastructure neither
household controls. Until then, describe the guarantee as what it is.

---

## 4. Every biometric validator stores a whole plaintext template

**State:** `matching-service/app/storage.py` persists `face_embedding` as
`embedding.astype(np.float64).tobytes()` — the complete ArcFace vector,
unencrypted. `main.py` loads every stored enrolment and compares plaintext
locally. `app/no_whole_templates.py` was written to make this structurally
impossible but is **not yet imported by anything**.

**Mitigating fact, confirmed 2026-08-20:** `real_enrollments` requires both
`ALLOW_REAL_BIOMETRIC_DATA=true` and `LEGAL_SIGNOFF_DATE`, and neither is set.
No real person's embedding is stored today — only test data.

**Why it matters:** An embedding is not a hash. It can be matched against any
other database, and published inversion attacks reconstruct an approximate face
from one. Adding validators for redundancy currently multiplies complete copies
of everyone's biometric, which is the exact concentration MPC exists to remove.
Deploying the biometric validators to C1 and C2 was deliberately **not** done
for this reason.

**To do:** integrate `no_whole_templates.py` on the storage path and move the
comparison to MPC. Note the deliberate design in that module: it fails loudly
rather than dropping the embedding, because a service that stores nothing and
compares against nothing answers "not a duplicate" for every person alive while
looking perfectly healthy.

---

## 5. Attestation gates nothing

**State:** `BIO_ATTESTATION_MODE=off` on both boxes and
`COORDINATOR_PUBLIC_KEYS` is empty.

**Why it matters:** With attestation off, any caller can choose its own bio
scalar and receive a valid nullifier. The Play Integrity work is live and
unused.

**To do:** decide the mode, populate the coordinator keys, and re-check that
registration still succeeds for a genuine capture before enforcing.

---

## 6. Throughput: Contabo1 is now level with Contabo2; the ceiling needs one measurement

**Target:** at least 10,000 TPS.

### Signature verification is NOT the blocker at 10k — the number already existed

`bench-signature-cgo-contabo2.yml` ran successfully on 2026-07-27 and its
result was never read. On the production path (the Dockerfile sets
`CGO_ENABLED=1`, so libsecp256k1, not the Go implementation):

| path | 6 cores | cores needed at 10,000 TPS | at 50,000 |
|---|---|---|---|
| pure Go (`CGO_ENABLED=0`) | 18,829 rec/s | 3.2 | 15.9 |
| **cgo (what production runs)** | **40,577 rec/s** | **1.5** | 7.4 |

So signature verification costs about 1.5 of 6 cores at the 10k target. It only
becomes the binding constraint near 50k. Any note still saying "the cgo number
is missing" is out of date.

### Four caps removed on Contabo1, all measured first

Contabo1 had **none** of Contabo2's throughput work. Every item below was found
by reading live state, not by reasoning about code:

1. **Connection pool saturated at idle** — default `AEQUITAS_DB_MAX_CONNS` of 20
   with 18 in use while the chain was idle, against `max_connections` 100, on a
   box now running eight containers. Throughput is bounded by the slowest
   REPLAYING node, so this capped the network regardless of Contabo2. Now pool
   100 against 250; idle usage 13. Matches the 87,534 pool waits measured
   earlier.
2. **No WAL at all** — no flags, no host mount, and a `deploy.sh` whose
   `docker run` carried no `-v`. Fixed, and the deploy script patched too:
   without that the next code deploy would have removed it silently.
3. **No block-payload compression.** This is the one aimed squarely at the
   collapse. Its own header: `SaveBlockWithPendingTxsAtomic` holds `dag.mu`
   while writing, and under load "block production fell to 9 blocks in 85
   seconds where 85 were due", with `AddPeerBlock` locked out for the same
   window — which is what piles up orphans and collapses the DAG to one tip.
   Measured 3.5x at full-block size with compression time counted.
   **Verified live afterwards**, not just assumed from the flag: both boxes'
   most recent `chain_blocks` rows now have `transactions` empty and
   `transactions_z` populated.
4. **No `ENABLE_MULTI_BLOCK_TICK`, no `AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING`.**
   The latter keeps a node producing while it works off a backlog instead of
   mistaking one for a fork — precisely the state a node is in under load.

**Corrected:** a previous entry said Contabo1 merges but never produces blocks.
True on 2026-07-28, not true now — 31 of the last 60 blocks were Contabo1's
against Contabo2's 29.

### Measured 2026-08-21: ~2,150 TPS, and the chain is the limit

The seed rows were empty, but /api/snapshot showed 722 rows of accounts.csv
still holding 4.49 AEQ between them, and a load-test transfer moves 0.00001
AEQ. So the fund phase — and any transfer out of a real account — was
unnecessary: `loadtest-find-funded.yml` writes `accounts-funded.csv` (funded
rows only, richest first) and the run drives from that.

| sender pairs | TPS |
|---|---|
| 100 | 1,847 |
| 358 | 2,164 (repeat of an earlier 2,117) |

**3.6x the senders bought 1.17x the throughput.** That plateau is the answer to
the question the earlier notes left open: this is the CHAIN's ceiling, not the
load generator measuring itself. Roughly 2,150 TPS against a 10,000 target.

Two things had to be cleared before the number meant anything:

- **The RPC rate limiter, not the chain, capped the first run.** 200 requests
  per 10s per IP; 7,960 rejected batches x 100 transfers = essentially every
  failure. `set-rpc-rate-limit-contabo2.yml` exists for exactly this. Raised
  for the measurement and **restored to the shipped 200 afterwards**.
- Some senders ran dry mid-run, which shows up as "insufficient balance"
  rather than as a throughput fact.

CPU is not the constraint: at 486 us per transfer, 2,150 TPS is about one core
of six. Signature verification needs ~1.5 cores at 10k. So the remaining
suspect is serialisation — the global write lock — which is what
`diagnose-tps-ceiling-contabo2.yml` was built to confirm through goroutine
dumps. It now accepts `accounts_csv` so it can run without funding; its
analysis step still fails and is the next thing to fix.

### The throughput run exposed a bug that took a validator down

Contabo2 restarted, fell ~500 blocks behind, and stopped advancing. Every
cycle:

    Could not batch-fetch 412 missing ancestor(s): unexpected end of JSON input

`fetchBlocksByHashes` read peer responses under a 20 MB cap and passed whatever
arrived to `json.Unmarshal`. `maxBlocksByHashPerRequest` is 500 because api.go
sized it against blocks of "~2 KB each ... at 500 hashes is ~1 MB". True of
near-empty blocks; false once blocks carry thousands of transfers, which is
precisely what a throughput run produces. The read truncated, the parse failed,
and the message named JSON rather than size — so nothing pointed at the batch.
The node looked healthy the whole time: /api/status answered, peers connected,
one repeated line.

Fixed in `fb2176f`: the read goes one byte past the cap so a filled response is
distinguishable from a near-miss and returns `errResponseTooLarge`, and
`splitOnOversize` halves the batch until it fits. Four tests pin it. Raising
the cap was rejected — an unbounded read from a peer is a memory DoS, and no
constant can know what blocks weigh, which is how the 500 was chosen and how it
failed.

**Confirmed working in production while recovering from this**: Contabo2's WAL
replayed 8,206,534 records and correctly fenced off every one as superseded by
a state replacement, reapplying none. That is `wal_recovery_floor_seq` doing
its job on a live node — the fix for the corruption that once produced 74
negative accounts.

---

## 7. Supply is 305.278008 AEQ above what registrations account for

**State:** Both benign explanations were ruled out live — the counter has not
drifted and no humans deregistered — so the AEQ was created, on the liquidity
or swap path. Five instances of the underlying bug class (account and pool are
two non-atomic writes) were found and fixed, and
`x/humanity/keeper/write_order_test.go` now fails if a new path appears that
does not declare which side loses AEQ. A daily drift watch runs against the
`305.278008` baseline.

**Why it matters:** The fixes stop it recurring; they do not explain the
existing gap or remove it.

---

## 8. Trusted dealer is reduced, not removed

**State:** Sacrificing (`x/humanity/mpc/sacrifice.go`) catches forged triples,
so a dishonest dealer is detected rather than trusted. Removing the dealer
entirely costs roughly 1 GB per registration, which the bandwidth budget does
not currently allow (~3 registrations/s at 1 Gbit/s).

**Why it matters:** Worth recording as a known, priced trade-off rather than
rediscovering it.
