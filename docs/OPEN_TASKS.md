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

### The constraint, established by controlled experiment

**Disk I/O, saturated by the node's own writes.** The decisive test: an
independent scratch-file fsync loop, sharing the device with the node and
nothing else, run twice.

| | chain idle | chain under load |
|---|---|---|
| fsyncs/s | 305 | **151** |
| p50 | 2,435 us | 3,370 us |
| p90 | 5,566 us | **11,330 us** |
| p99 | 14,736 us | **55,441 us** |
| max | 46,462 us | **535,061 us** |

A process that shares only the block device halves its own fsync rate when the
chain is loaded. That is device saturation, not a lock and not a code path.

### Ruled out by measurement — do not re-try these

| hypothesis | how it was rejected |
|---|---|
| Parallel replay too small | Real block #4391949: 297 disjoint batches covered 49,886 of 50,000 transfers. Already 99.8% parallel. |
| WAL flush batch size | Swept 4000/1000/400/150. Default won (3,264 vs 1,949/624/2,060). items_per_flush was 402 against a 4,000 cap, so it never bound. |
| WAL flush interval | Swept 5/15/40/100/250/600 ms. The 100 ms default won in BOTH directions, even though short intervals moved addrs_per_flush 348 to 134 and hold_ms 40 to 21 exactly as predicted. |
| Postgres checkpoints | max_wal_size 1GB to 8GB plus wal_compression moved sync_avg only 15,989 to 14,605 us. |
| GC / memory | 67 cycles and 159 ms of total pause across a 109 s run at 2,745 TPS: **0.15% of wall time**, on a 475 MB live heap and a 12 GB box. |
| The WAL file's own size | Same probe on a 1 MB and a 3 GB file, alternated three times: p50 1,929/1,935/1,769 vs 1,941/1,836/1,865 us. Identical. |
| Lock contention | A mutex profile put 45.90% on flushWALBatch's LockAddrs hold. Four separate changes reduced that hold as intended and moved throughput not at all. |

**A correction worth keeping**: an apparent fourfold gap between the node's
fsync (14.4 ms) and the probe's (3.4 ms) was an artefact of comparing a MEAN to
a MEDIAN. With the probe's own p90 at 11.3 ms and p99 at 55 ms, its mean lands
in the same place. There was no unexplained in-process gap.

### Fixed today, each from a measurement

1. **Blocks nobody could replay.** Block #4391949 carried 50,000 transfers;
   replaying it took 4.7 s under the exclusive lock against a 1 s block time.
   `maxTxsPerBlock` 50,000 -> 10,000, the measured replay capacity for one
   block time. Nothing is dropped, only deferred.
2. **Multi-block tick outran replay** — up to 5 full blocks per tick, engaging
   exactly when the partner is already behind. Off on both boxes.
3. **No admission control existed.** The node accepted transfers while its
   height was frozen for 15 s, growing the backlog that kept the gate shut. Now
   refuses with retryable -32005 after 30 s without a block. Confirmed live.
4. **A resync could never finish on a long-running node.** `DELETE FROM
   chain_blocks` on 4.38 M rows exceeded statement_timeout, so every attempt
   rolled back, visible only in `degraded_reason`. TRUNCATE instead.
5. **Ancestor fetches burned a node's bandwidth** — a 20 MB read cap sized
   against 2 KB blocks. Server-side byte budget plus `X-Blocks-Truncated`,
   which the client must honour or it abandons blocks the peer actually holds.
6. **A restarted node was stranded on a tip nobody knew about.** It re-offers
   its tips after 60 s without producing, so a peer can merge them.
7. **184 GB of docker build cache** across both boxes (C1 116 GB, C2 68 GB).
   C1 went 65% -> 9% full, C2 84% -> 21%. A validator that fills its disk stops
   writing its WAL and its database.
8. **The wallet-lookup index ran on the block path** — one row per transaction,
   up to 10,000 per block, and on the replay path *inside the exclusive lock*.
   Not consensus, and both call sites already discarded its error. Now
   asynchronous: `sync_avg_us` 14,374 -> 10,260 and `sync_max_us` 1,708,300 ->
   445,791.

### What the numbers say about the targets

**30,000-40,000 TPS is not reachable on these boxes.** 486 us of CPU per
transfer against six cores puts the ceiling near **12,300 TPS**, and signature
verification alone runs at 40,577/s on six cores with cgo — at 40,000 TPS that
is every core spent on signatures before any state, WAL, database or network
work. 40,000 needs roughly 24 cores.

**10,000 TPS is not reachable on this storage.** Throughput sits in a
2,000-3,000 band while the disk delivers 151 fsyncs/s under the node's own
load. The node's applied-transfer counter peaks at 6,272-7,374/s in good
seconds and collapses to single digits when an fsync stalls.

**Measurement noise is now the limit on tuning.** Nine runs today gave 2,117 /
2,164 / 3,264 / 2,990 / 3,014 / 1,816 / 1,879 / 2,745 / 2,381 — a spread of
+-40%. Single runs cannot resolve anything smaller than a ~50% change, so any
further tuning needs repeated runs and a median, not one number.

### What would actually raise it

- **Faster storage.** The single highest-value change. The device does 2 ms
  when idle and collapses under the node's own writes; NVMe-backed storage
  would move the binding constraint somewhere else entirely.
- **Less I/O per transfer.** The remaining large writer is the Postgres mirror:
  an outbox row per transfer plus account UPSERTs, on a path whose durability
  the WAL already provides. Removing or deferring that mirror is the
  architectural change that follows item 8's success.
- **More cores**, but only after the above — CPU is at about one of six.

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
