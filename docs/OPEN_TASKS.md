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

## 6. Throughput and stability under load

### Where it ended up

| | session start | now |
|---|---|---|
| Sustained throughput (median) | ~2,381 TPS | **3,376 TPS** (runs: 3,914 / 3,376 / 3,263) |
| Transfer latency | not measured | 55-62 ms, of which WAL sync is ~6 ms |
| WAL fsync | 14,374 us avg, 1,708,300 us max | **5,606 us avg** |
| Failed transfers in a run | tens of thousands | **788 of 390,800 (0.2%)** |
| After a heavy run | Contabo1 stuck, needed an operator resync | **converged after all three consecutive runs** |

**Stability is the part that now works.** Three heavy runs back to back, and the
two nodes ended byte-identical every time. That was the failure that dominated
the day: a node fell behind, could not rejoin, and needed a resync.

**10,000 TPS was not reached**, and the honest reason is below.

### The measurement was capped by our own rate limiter

**Every throughput number taken with the limiter at 10,000 was measuring the
limiter, not the chain.** After the P1 security fix the limiter charges per
JSON-RPC batch ITEM, and the window is 10 seconds:

    10,000 items / 10s = 1,000 transfers/s per IP

A 3-minute run at that setting: **824 TPS, and 12,117 `-32005 rate limited`
errors** as the dominant failure cause. Only 4.3% of submitted batch items ever
reached the chain.

Raised to 3,000,000 (300k/s, cannot bind) the same run gave **3,253 TPS,
peak 7,151/s, and zero rate-limit errors** — 98.3% of items reached the chain.

Before any throughput measurement on this project: raise the limiter, and check
the run's failure causes for `-32005`. **Put it back to 200 afterwards** —
it is a protection on a public endpoint.

### Where a transfer's time actually goes

Measured with `rpc_phases` (rpc_phase_stats.go), limiter out of the way:

| phase | ms | share |
|---|---|---|
| whole handler, per batch item | 74.75 | — |
| `sendRawTransaction` | 75.63 | 100% |
| ├ nonce reservation | 19.59 | **26%** |
| ├ `TransferAtomic` | 52.12 | **69%** |
| └ everything else | 3.92 | 5% |

**`rpc_total_per_item` 74.75 ≈ `send_tx` 75.63, so the node owns essentially
all of it — there is no hidden client-side gap.** That answers the question the
instrument was built for, and it rules out the "fix the harness" branch.

### The next target, and why it is not a quick patch

**Nonce reservation is 26% of every transfer.** The batcher's own wait is
capped at 1ms (`nonceBatchMaxWait`), so the 19.6ms is the Postgres round trip
under load — and the per-sender shard lock is held across it, while a batch's
100 transfers from one sender are necessarily serial.

The in-memory nonce map already provides ordering while the process is alive;
the database compare-and-swap is there for restart durability. Making it
asynchronous would remove the 19.6ms but opens a replay window across a crash.
That is a real durability trade-off and needs its own design pass, not a patch.

Arithmetic for the target: at 380 senders, 10,000 TPS needs 38ms per transfer.
`TransferAtomic` alone is 52ms, so **the nonce fix is necessary but not
sufficient** — reaching 10k needs either TransferAtomic reduced as well, or
substantially more concurrent senders.

---

### Ten hypotheses measured and rejected — do not retry these

| hypothesis | how it was rejected |
|---|---|
| Parallel replay too small | Real block #4391949: 297 disjoint batches covered 49,886 of 50,000 transfers. Already 99.8% parallel. |
| WAL flush batch size | Swept 4000/1000/400/150; default won. items_per_flush was 402 against a 4,000 cap, so it never bound. |
| WAL flush interval | Swept 5/15/40/100/250/600 ms. The 100 ms default won in BOTH directions. |
| Postgres checkpoints | max_wal_size 1GB->8GB plus wal_compression moved sync_avg only 15,989 -> 14,605 us. |
| GC / memory | 67 cycles, 159 ms total pause across 109 s at 2,745 TPS: 0.15% of wall time, 475 MB live heap. |
| The WAL file's own size | Identical fsync on a 1 MB and a 3 GB file, alternated three times. |
| The exclusive state lock | `exclusive_busy_pct` measured **0.67%**. Block production is not blocking transfers. |
| Transfers falling off the fast path | `fast_path_pct` measured **99.90%**. Almost none fall back to the batcher. |
| The disk being slow | It does 426 fsyncs/s at a p50 of 1,976 us when idle. |
| Append blocked by the writer's mutex | Real and fixed, but worth only ~55 avg_batch -> 56. |

### What actually moved it, each from a measurement

1. **`maxTxsPerBlock` 50,000 -> 10,000.** A 50,000-transfer block took 4.7 s to
   replay against a 1 s block time, so the replaying node fell behind by 4.7x
   per block.
2. **Multi-block tick off.** It emits up to 5 full blocks per tick, and engages
   exactly when the partner is already behind.
3. **Admission control.** A node that has not produced for 30 s refuses with a
   retryable -32005 instead of growing the backlog that keeps its gate shut.
4. **Resync made possible at all.** `DELETE FROM chain_blocks` on 4.38M rows
   exceeded statement_timeout, so every resync silently rolled back. TRUNCATE.
5. **Ancestor fetch.** A 20 MB read cap sized against 2 KB blocks; server-side
   byte budget plus `X-Blocks-Truncated`.
6. **Tip re-announce** for a restarted node holding a tip no peer knows about.
7. **184 GB of docker build cache** reclaimed (C1 65%->9% full, C2 84%->21%).
8. **The wallet index off the block path** — up to 10,000 rows per block, and
   on the replay path inside the exclusive lock.
9. **WAL preallocation + fdatasync.** Measured 352 -> 1,429 syncs/s in
   isolation; neither half works alone.
10. **Flush concurrency 4 -> 32.** hold_max 557 ms -> 162 ms. The flush holds
    account locks across its Postgres transaction, and transfers wait behind
    it — most of a transfer's 60 ms.

### The stability failure, root-caused and fixed

A heavy run left Contabo2 frozen at its exact starting height while still
applying 4,349 transfers a second, and a restart did not help. Two separate
defects, both now fixed:

**1. Admission control had a hole exactly where it was needed.**
`productionStalledFor` returned zero whenever the node had never produced a
block — so a node that had not produced *since starting* was permanently
exempt. Contabo2's stats read `last_block_produced_unix: 0`, `refusing: false`,
`stalled_seconds: 0` while it was frozen. It accepted 104,060 transfers it
could never include. Fixed by anchoring the stall origin to process start: a
node still gets the full stall limit to produce its first block, and no longer
gets an unbounded pass after that.

**2. Why it could not rejoin, measured rather than assumed.** Six checks, five
clean: the data was reachable from the box in 13ms, the chain was linked (block
4425914's parent IS Contabo2's own tip), blocks were 717 bytes not oversized,
the box rejected nothing, no proposer was banned, and it was 470 blocks behind
against a far-ahead cap of 5000.

The real cause: it held orphan blocks from the load run, reloaded from its own
Postgres at boot, waiting on parent `b99fa340...` — a hash that exists nowhere
on Contabo1. They can never resolve, and while they sit there they count as
unresolved deferrals, pinning `clean_sync_streak` at 0 and holding the
production gate shut.

**The deepScan actively made it worse.** Its sweep started at the right height,
failed to merge, and concluded the common ancestor must lie further BACK — so
it walked its floor down (4425913 → 4425755 → 4425600), spending the bounded
sweep budget re-walking history the box already had while the real gap ahead
grew a block a second. Those two causes need opposite responses and the code
cannot yet tell them apart; that distinction is still open.

Recovered with `recover-contabo2-resync.yml`. Both nodes now produce and sit
byte-identical. `tools/snapshot-signer` was written so BOOTSTRAP_SIGNER is
recovered from the live snapshot rather than assumed — a wrong value makes a
resync fail closed, which looks exactly like it never ran.

### What is still open

The remaining latency is spread across many small costs with none dominant:
~60 ms per transfer, ~6 ms of it the WAL. CPU sits at about one core of six, so
the machine is not the limit either. Getting past this needs either the load
harness extended to several hundred more funded senders — which is the only way
to even observe 10k — or a structural reduction in what one transfer waits on.

**30,000-40,000 TPS remains arithmetically out of reach on six cores**: 486 us
of CPU per transfer puts the ceiling near 12,300 TPS, and signature
verification alone runs at 40,577/s on six cores with cgo.

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
