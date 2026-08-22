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
limiter, not the chain.** After the P1 security fix it charges per JSON-RPC
batch ITEM, over a 10-second window: `10,000 / 10s = 1,000 transfers/s per IP`.

A 3-minute run there: **824 TPS, 12,117 `-32005 rate limited` errors**, and only
4.3% of submitted items reaching the chain. Raised to 3,000,000 the same run
gave **3,253 TPS and none** — 98.3% of items reached the chain.

Before any throughput measurement: raise the limiter, and grep the run's
failure causes for `-32005`. **Put it back to 200 afterwards.**

### A load test against a refusing node looks exactly like a slow node

A 100-pair run started 60 seconds after a restart produced 140 transfers and a
peak of 41/s. The node was not slow — it was correctly refusing, because
admission control turns away a node that has not produced a block, and this one
took **533 seconds** from process start to its first block while the
initial-sync gate cleared.

`stability-under-load.yml` now waits for `never_produced` and `refusing` to both
clear before generating anything, and fails loudly rather than measuring
refusals. Two comparisons made before that gate existed had to be thrown away.

### Where a transfer's time goes, phase by phase

Two instruments were added, because at every stage the sum of the known parts
came out an order of magnitude below the measured whole.

**The request** (`rpc_phases`): nonce reservation was 19.59 ms of a 75.63 ms
`sendRawTransaction` — 26%. Fixed by reserving a batch's whole consecutive nonce
range in one round trip instead of 100 (`nonce_batch_reserve.go`):

    nonce_ms      19.59  ->  0.003     covered_pct 99.92, avg_run 100
    throughput     3,253 ->  3,779

**The transfer** (`transfer_phases`). Every candidate had already been measured
and cleared — `TryLockAddrs` never waits, the enqueue is a plain append, the
exclusive lock is 0.35% busy, the WAL sync averages 6.8 ms — which sums to about
7 ms against a measured 78.5 ms. The split found the missing time immediately:

| phase | before | after |
|---|---|---|
| precheck | **46.41 ms** | 22.98 ms |
| wal_append | 20.62 ms | 18.98 ms |
| enqueue | 0.46 ms | 0.24 ms |
| apply | 0.014 ms | 0.017 ms |
| lock (`TryLockAddrs`) | 0.003 ms | 0.005 ms |
| **total** | **67.5 ms** | **42.2 ms** |

**69% of a transfer was two `shardedAccounts.Get` calls.** `Get` takes the shard
mutex with a *blocking* `s.mu.Lock()`, and those are the mutexes `flushWALBatch`
holds across its whole Postgres transaction — 37 ms over 371 addresses. So every
transfer queued behind the flush two lines before reaching `TryLockAddrs`, whose
entire purpose is to bail instantly instead of waiting.

**It disguised itself as good news.** `fast_path_pct` read 99.82%, which looks
like almost no contention. It actually meant transfers *waited* for the shard
and then found it free, so the bail-out never fired. After the fix the fallback
rate rose from 1,345 to 36,817 — transfers now correctly divert to the batcher —
and the fast path dropped from 67.5 ms to 42.2 ms.

### Throughput is no longer latency-bound, and that is the finding

Cutting the fast path by 37% did **not** raise throughput: 3,779 → 3,356, inside
the run-to-run noise band of roughly 3,400–3,800. Removing 19.6 ms of nonce cost
earlier moved it only 16%. Two large, real latency reductions that throughput
did not follow means **latency is no longer what binds.**

Two candidates remain, and they are testable independently:

1. **Not enough senders.** Throughput is pairs ÷ latency, and one JSON-RPC batch
   carries transfers from a single sender, so the node never sees more
   parallelism than there are funded pairs. There are **623 funded rows = 311
   pairs**. `loadtest-widen-senders.yml` funds the 578 accounts already
   generated but empty, roughly doubling that. Built and dry-run verified;
   needs a human to dispatch with `confirm=true` because it moves AEQ.

2. **An RWMutex convoy between the flush and block production.** `flushWALBatch`
   holds `cs.mu.RLock()` for its entire 37 ms DB transaction, up to 32 at once.
   Go's RWMutex gives writers priority, so the moment block production calls
   `cs.mu.Lock()` every subsequent transfer's own `RLock()` blocks until all
   those flushes drain. `exclusive_avg_ms: 6` measures the hold, not the stall
   it induces — which is the shape of the 23 ms still left in `precheck`.

   **Do not "fix" this by releasing the flush's locks early.** That has been
   tried twice and reverted twice, both times producing
   `pq: deadlock detected (40P01)` against `saveAccountsToDBBatchCtx`. The lock
   hold is what makes a Postgres-level deadlock structurally impossible. See
   `flushWALBatch`'s own doc comment before touching it.

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

### The path to 10k, and the one step that needs you

Throughput is **pairs / latency**, and latency is no longer what binds — two
large, real reductions landed today and throughput did not follow them. The
pair count is the missing multiplier.

    623 funded rows  = 311 pairs   ->  ~3,400-3,800 TPS
    ~1,900 funded    = ~950 pairs  ->  ~10,000 TPS at the same latency

`accounts.csv` now holds **2,000 accounts** (`loadtest-add-accounts.yml`,
append-only, every one of the 1,200 originals verified preserved). Funding them
costs **2.82 AEQ of the 4.49** the load-test float holds.

The funding step moves AEQ, so it needs a human to dispatch:

```bash
gh workflow run loadtest-widen-senders.yml -f confirm=true -f seeds=120 -f fund_amount_wei=1500000000000000
gh workflow run loadtest-find-funded.yml
gh workflow run set-rpc-rate-limit-contabo2.yml -f value=3000000 -f dry_run=false
gh workflow run stability-under-load.yml -f duration=3m -f pairs=0 -f recovery_minutes=3
gh workflow run set-rpc-rate-limit-contabo2.yml -f value=200 -f dry_run=false
```

Dry-run verified: 120 seeds, 1,880 targets, budget printed.

### Two more things measured and rejected — do not retry these

**Flush concurrency 64 is worse than 32** (3,476 vs 3,779). `addrs_per_flush`
stays at ~371 whatever the worker count, so more workers only freeze more of the
address space simultaneously.

**Capping a flush by distinct addresses is much worse.** A cap of 64 addresses
took throughput to **1,490**. Smaller flushes do not amortise: the fix is in
`wal_flush_addr_cap.go`, defaults to off, and should stay off. The knob exists
so the result is reproducible, not because it helps.

### The self-heal was armed — it just could not see this failure

Contabo1 forked mid-load (a genuinely different block at height 4467699), froze,
and fell 600+ blocks behind. `autoheal-status-both.yml` confirms all four
monitors were armed and correctly configured on both boxes. The one whose
description fits exactly — "receives blocks but attaches none while behind the
primary" — never confirmed a single tick.

`attachDelta > 0` was why. A stuck node is rarely attaching *nothing*: it still
bridges the odd orphan. One attachment in a 60-second tick reset the watch, so a
node attaching one block a minute while losing sixty read as "normal (possibly
slow) operation" indefinitely. Only the 25-minute height-stall check could still
catch it.

Now a **growing gap** confirms starvation regardless of the trickle. Shrinking
and steady gaps still do not, so an ordinary catch-up after a restart is
untouched.

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
