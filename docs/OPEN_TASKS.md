# Open tasks

Written 2026-08-20. This exists because "is MPC done?" could not be answered
from the repository: the code is complete and its tests are green, which looks
like done, while no validator runs any of it. Every item below states what is
actually true in production, not what the code supports.

Verify before trusting an entry. Where a claim came from a live read, the read
is named so it can be repeated.

---

## 1. MPC is built but not activated anywhere

**State:** The chain reads eight MPC settings — `MPC_ENABLED`, `MPC_PEERS`,
`MPC_PARTY_INDEX`, `MPC_TRIPLE_FILE`, `MPC_CLIENT_TOKEN`,
`MPC_COMMITTEE_SIZE`, `MPC_REQUIRED`, `MPC_ALLOW_INSECURE`. **None of them is
set on Contabo1 or Contabo2.** (Read 2026-08-20 via `wal-status-both.yml` and
`wal-contabo1.yml` mode=inspect: C1 carries 16 env keys, C2 carries 22,
and no `MPC_` key appears in either.)

The two Beaver triple files produced by `cmd/mpc-dealer` (48 MB each) are still
on the developer's machine in `mpc-triples/`. They were never transferred.

**Why it matters:** Green tests for a subsystem nothing runs is the most
expensive kind of false confidence — it reads as finished in every summary.

**To do:** place `triples-party-0.bin` on one box and `triples-party-1.bin` on
the other (never both files on one box — that single machine could then
reconstruct), set the five wiring variables per box, and confirm a committee
forms against the live nodes rather than in a test harness.

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

## 6. Throughput is limited by block propagation, not by writes

**State:** WAL is now enabled and durable on both boxes (2026-08-20). Earlier
measurements put the CPU ceiling near 12,300 TPS on six cores (486 µs per
transfer), but sustained throughput collapsed at roughly 3,600/s: pushing from
3,624 to 7,078 tx/s dropped block production from +76 to +9 blocks and DAG tips
from 7 to 1.

**Why it matters:** The target is at least 10,000 TPS. WAL removes a durability
cost, not the merge and propagation cost, so WAL alone will not reach it. The
next measurement should be of propagation, and it should be a measurement — the
last several throughput hypotheses on this project (WAL lock, batch size, fsync,
bloat, gzip) were all plausible and all wrong.

**Also still open:** Contabo1 merges but does not produce blocks, so only two
of three validators produce. Its gate ("3 clean sync cycles") is correct and
must not be loosened; it reports truthfully that C1 never fetches missing
sibling parents.

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
