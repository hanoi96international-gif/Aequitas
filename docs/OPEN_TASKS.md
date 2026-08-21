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

### Why it stops at ~3,400, stated honestly

Throughput = concurrency / latency. The harness holds 380 funded sender pairs
and a transfer measures 62 ms end to end on the server, so the harness's own
ceiling is

    380 / 0.062 s = ~6,100 TPS

**Measured 3,400 — about half of that.** That gap is the important number, and
it says the harness is *not* the binding constraint at the rate actually
achieved. Working backwards, 380 / 3,400 = **112 ms of real per-transfer
latency**, against 62 ms measured inside the transfer path. So roughly 50 ms
per transfer is spent somewhere the current instrumentation does not see —
client-side queuing in the generator, RPC handling before the transfer path
starts, or response handling after it ends.

**Do not conclude "just add more senders."** More senders only help once the
harness ceiling is the thing being hit, and it is not. The first job is finding
the missing 50 ms, because it is nearly half of every transfer and no current
measurement covers it.

Both possible outcomes are actionable: if the 50 ms is on the client, the
harness needs fixing and the chain is faster than it appears; if it is in RPC
handling, that is a chain fix that no amount of load generation substitutes for.
Instrument the RPC handler end to end (arrival to response write, not just the
transfer path) and compare against the client's own observed latency. That one
measurement decides which.

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
