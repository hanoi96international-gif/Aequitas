# Running secure duplicate matching on the two validators

This describes how to turn the code in this package into two machines that
compare biometric templates without either of them ever holding one.

## What this protects, and what it does not

**Protects:** no single machine can reconstruct a template. Each validator holds
one additive share — a vector of uniformly random field elements. Reconstruction
requires both. If one box is seized, compromised, or subpoenaed, the templates
on it are noise.

**Does not protect against:** anyone who can obtain both shares. With two
parties, both shares together reconstruct everything, so the whole argument rests
on no single actor controlling both.

Ownership of the two boxes is separate (C1 and C2 have different owners), which
is real: it means an attacker who compromises, steals, or seizes ONE box gets
noise. That is the most likely attack, and the split defeats it.

Operational control, however, is not yet separate, and that is what the
assumption actually depends on. Three things currently converge:

1. **Both SSH keys live in this repository's GitHub secrets.**
   `resync-both-contabos.yml` uses `CONTABO_SSH_KEY` and `CONTABO2_SSH_KEY` in
   one workflow to reach both hosts. One GitHub account therefore holds root on
   both parties.
2. **Both boxes deploy from this repository**, so a change merged to `main`
   reaches both parties. A single malicious or compromised commit is a two-party
   compromise.
3. **Both boxes are at the same provider** (Contabo), so a provider-level
   compromise or legal order is correlated across them.

None of that is an argument against the split — it is the list of what has to
move for the split to carry the weight the protocol puts on it:

- move `CONTABO2_SSH_KEY` out of this repository, into a secrets store held by
  C2's owner;
- deploy the MPC party independently of the chain node, so a node deploy cannot
  rewrite both parties' matching code;
- put any third party at a different provider.

Adding a third independently-operated party remains the single largest
improvement available, and the protocol already supports any number
(`TestThirdPartyWorksToday`).

## The two-party quorum problem

With two parties there is no majority. If one is down, or the two disagree, there
is no third vote to break the tie. The system therefore fails **closed**: no
comparison, no registration. That is the correct direction — a wrongly refused
registration can be retried, a wrongly granted second account cannot be taken
back — but it does mean **either box being down halts registration entirely**.

This is a deliberate trade, not an oversight. It should be revisited when a
third party exists.

## Configuration

Peers are identified by their **validator signing address** — the same key that
already signs blocks, and the same address the Node Binding Signature ties to an
operator wallet. There is no shared secret anywhere in this design.

| Variable | C1 (`173.249.37.118`) | C2 (`194.163.188.71`) |
|---|---|---|
| `MPC_ENABLED` | `true` | `true` |
| `MPC_PARTY_INDEX` | `0` | `1` |
| `MPC_PEERS` | `https://c1\|0x<c1-addr>,https://c2\|0x<c2-addr>` | identical |
| `RELAYER_PRIVATE_KEY` | already set | already set |

`MPC_PEERS` is in **party order** and identical on every box; only
`MPC_PARTY_INDEX` differs. Each entry is `URL\|0xSigningAddress`. The node
refuses to start MPC if its own key does not produce the address listed for its
index — otherwise every contribution it signed would be rejected by its peers,
and it would find that out only mid-registration.

### Adding a validator

**Nothing.** With `MPC_PEERS` unset the committee is derived from the chain, so a
validator becomes eligible the moment it registers as a peer — the registration
already proves ownership of its signing key, and the peer registry now records
that address alongside the URL. No file is edited, no box is restarted, and
nobody approves anybody.

    MPC_ENABLED=true
    MPC_COMMITTEE_SIZE=2        # 2..7
    RELAYER_PRIVATE_KEY=...     # already set

Membership is resolved **per registration**, not at startup, so a validator that
joins while the nodes are running is seen without a restart.

`MPC_PEERS=URL|0xAddr,...` still works and pins an explicit set. It is the
override, not the normal path — it has to be edited on every box whenever the
set changes, which is exactly the manual-config problem the validator set itself
already solved by syncing from peers.

### Why not every validator is a party

Traffic grows with the number of ordered pairs, n(n-1), because every party
publishes each round to every other. Measured on one identical comparison:
2 parties 4.0 KB, 3 parties 12.2 KB, 4 parties 24.4 KB — exactly 1x, 3x, 6x.
Scaled to a real registration, which costs 37.5 MB between two parties:

| committee | traffic per registration |
|---|---|
| 2 | 37.5 MB |
| 3 | 112 MB |
| 5 | 375 MB |
| 7 | 788 MB |
| 50 | ~46 GB |

Availability moves the same way. Additive sharing is all-or-nothing: **every**
member must answer or the comparison cannot finish. At 99% per-node uptime, 2
parties are jointly available 98% of the time and 50 parties 61% — a bigger
committee is a system that more often cannot register anyone.

So the committee is a small, deterministically drawn subset. Growing the
validator set makes membership *more* decentralised — more independent operators
eligible, chosen by hash rather than by anyone's decision — without making every
registration more expensive. `MPC_COMMITTEE_SIZE` is capped at 7 for this
reason.

### Committees are history, not a current value

An enrolment's shares belong to the committee that created them. If the
committee later changes, those shares do not move — **nobody can move them**,
because moving them would mean reconstructing the template, which is the one
thing that must never happen.

So every enrolment must record its `Committee.ID`, and a comparison against it
convenes *that* committee. The consequence is real and has to be planned for: a
committee whose members are permanently gone takes its enrolments with it, and
those people can register again undetected. Committee members should therefore
be long-lived validators, and committees should change rarely.

A threshold scheme with proactive resharing would remove this constraint. The
current sharing is additive n-of-n, so it does not.

### Relationship to the Node Binding Signature

They do different jobs, and both are needed:

- **Node Binding Signature** — a one-time, human-driven proof, made in a browser
  with MetaMask, that a node is operated by the holder of a particular wallet.
  It anchors a signing key to an on-chain identity.
- **MPC round signatures** — per-message, machine-to-machine proof that *this
  payload*, in *this round*, of *this session*, came from party `i`.

A one-time binding cannot authenticate individual messages, so it cannot replace
the round signatures. The round signatures say nothing about who an operator is,
so they cannot replace the binding. The binding is what makes the round
signatures meaningful: it says which key belongs to which validator.

Replacing the shared token removed `MPC_PEER_TOKEN` entirely. It did not remove
the need for node binding — it made it more load-bearing.


### HTTPS is required

Plaintext peers are refused unless `MPC_ALLOW_INSECURE=true`, and that flag is
itself refused with any non-loopback peer. The wire carries no templates, but it
does carry the values that decide whether someone counts as new — an attacker
able to inject on that path can mint duplicate accounts.

## Verifying it is actually running

On startup each box logs one line:

```
[MPC] party 0 of 2 serving /mpc/exchange; peers=[https://... https://...]
```

If configuration is wrong, the line reads `[MPC] NOT running: <reason>` and the
endpoint is **not mounted**. The node keeps producing blocks; it simply does not
take part in duplicate checks. Check for this line after every deploy — a silent
absence means the box is not a party, and with only two parties that means
registration is halted.

The endpoint rejects unauthenticated requests, so a bare probe returning `401`
is the healthy answer:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<box>/mpc/exchange
```

`401` = mounted and protected (the probe carries no signature). `404` = not
mounted, check the startup log.

The startup line names the address this node signs as, so a copy-paste error in
`MPC_PEERS` shows up without waiting for a registration to fail.

## What one registration costs

Measured over two HTTP servers, 1200 candidates at 512 features
(`TestWorldScaleRegistrationOverTheNetwork`):

| | |
|---|---|
| multiplications | 614,400 |
| network rounds | 19 |
| bytes on the wire | 37.5 MB |
| online compute | ~0.4 s |

The round count is flat in the candidate count, so link **latency** costs
`19 × RTT` regardless of population — about 0.4 s on a 20 ms link.

**Bandwidth is the real limit.** 37.5 MB per registration means a 1 Gbit/s link
between the pair sustains roughly 3 registrations/second (~250k/day); at
100 Mbit/s it is closer to 0.3/second. Past that, the LSH keyspace has to be
sharded across several validator pairs so registrations spread over independent
links. That sharding is **not implemented**.

## Triples must be verified, and the dealer must stay off the wire

The offline phase produces Beaver triples. A triple with `c != a*b` makes the
multiplication consuming it produce a wrong value **silently** — no error, no
log line — and enough wrong triples flip a duplicate check to "new". That is a
Sybil attack mounted entirely from the offline phase, without seeing a single
template.

`Session.SacrificeVerify` closes it. Triples are checked in bulk before use, at
the cost of destroying one triple per checked triple, so budget
`TriplesForVerifiedWork(n) == 2n`. Verification is not optional: an unverified
triple is one the dealer could have forged.

What verification does **not** fix: a dealer knows `a` and `b`, so a dealer that
*also* sees the openings of a multiplication recovers that multiplication's
inputs — the templates. The triples are perfectly valid; there is nothing to
detect. The only defence is keeping the dealer off the wire:

- the dealer **must not** run on either validator,
- it **must not** have network access to the exchange traffic between them,
- it **must** discard the plaintext triples after distribution.

So the dealer's power is reduced from "can silently decide who is a duplicate,
and read every template" to "can read templates only by also controlling the
network between the parties". That is a real reduction. It is not the same as
removing the dealer.

### Why the dealer is still there

Removing it means generating triples between the parties themselves, via
oblivious transfer or homomorphic encryption. The cost is the reason it has not
been done here, and the numbers are worth knowing before anyone plans it:

| | |
|---|---|
| triples per registration | 1,228,800 (1200 candidates x 2 x 512 features) |
| with verification | ~2.46M raw triples |
| OT-based generation (Gilboa, 61-bit field) | ~61 OTs per multiplication |
| offline traffic per registration | **~1 GB**, against 37.5 MB online |

Roughly thirty times the online cost, in a phase that can at least be
precomputed during idle time. It is also a substantial piece of
security-critical cryptography — OT extension has subtle requirements — and
writing it unreviewed would replace a documented weakness with an undocumented
one. It needs a vetted implementation, not a session's work.

## A third party works today

Nothing in the protocol assumes two parties; `TestThirdPartyWorksToday` runs the
whole comparison over three and four independent HTTP servers and checks they
agree. Adding an independent operator is an operational task, not a
cryptographic one.

It is also the largest available improvement, because with two parties both
shares together reconstruct everything and both boxes are administered by one
person. With three, no two shares reveal anything.

The cost is bandwidth, which grows with the number of party pairs — measured on
the same comparison: 2 parties 4.0 KB, 3 parties 12.2 KB, 4 parties 24.4 KB. A
third party roughly triples the traffic.

To add one: extend `MPC_PEERS` to three URLs on every box, give each its own
`MPC_PARTY_INDEX`, and share the same token.

## Calibration is a procedure, not a guess

`Calibrate` and `RecommendThreshold` turn labelled captures into the threshold
that decides who is refused. Feed it `LabelledPair` values built from real
captures through the real sketch pipeline — not embeddings, since the
comparison runs on sketches and the binarisation loses something that has to be
measured.

The two errors are not symmetric and the code takes a side:

- **FAR** — two different people measured as the same. The second is told they
  are already registered, with no document to appeal to. This is the
  irreversible error.
- **FRR** — the same person measured as different. They register twice: a Sybil,
  which can be found and removed later.

`RecommendThreshold` therefore optimises against a **FAR budget** and reports
the FRR that comes with it, never the reverse. It refuses to recommend anything
from fewer than 1000 pairs per class: a FAR measured as zero on thirty pairs
means nothing was observed, not that nobody is locked out — and that number
would be quoted as fact for years.

Use `EffectiveDuplicateCatchRate` to combine the threshold with the index
recall. A 99% recall index in front of a comparison that misses a third of
duplicates catches two thirds, not 99%.

## Turning it on for registration

The comparison only counts once registration consults it. Two endpoints and one
switch:

| Variable | Meaning |
|---|---|
| `MPC_CLIENT_TOKEN` | authenticates the capture pipeline to `/mpc/check` and `/mpc/enroll` |
| `MPC_REQUIRED` | `true` makes `/api/register` refuse without a passed check |

`POST /mpc/check` — the capture pipeline sends **each party its own row** and the
bucket keys. Every party runs the comparison against the enrolments it holds and
records the verdict under the session id. `POST /api/register` then carries
`mpc_session`, and the verdict is consumed there.

The client never asserts its own result. It could otherwise simply post "not a
duplicate" and the whole subsystem would be decorative. A verdict is spent once
and expires after 10 minutes — a stale pass was measured against an older
enrolment set, so anyone who registered in between was never compared.

`POST /mpc/enroll` stores the new person's row after a successful registration.

### The direction of failure is deliberate

Approving someone already registered mints a second account for one person and
is effectively irreversible. Refusing this attempt costs a retry.

So a check that cannot run — peer down, committee not formed, triples exhausted
— **refuses the attempt and says to try again**. It never approves by default,
and it never records a permanent rejection: nobody is locked out because a
validator was rebooting. Operational failures are worded so they do not accuse
the person of anything; only an actual match says "already registered".

### `MPC_CLIENT_TOKEN` is a shared secret, and that is correct here

Between validators a shared token was wrong, because it lets any party
impersonate any other and those parties decide who is a duplicate. This is a
service credential for one caller, which impersonates nobody: the most it grants
is the ability to submit captures, which is what the caller exists to do. Do not
"make it consistent" with the peer authentication — that would undo the point.

### What is NOT in this repo

Splitting a capture into per-party rows happens where the biometric still exists
in the clear — on the device or in the matching service — and then each row goes
to exactly one party. After that split no single machine can reconstruct the
template, which is the entire security argument. That code lives in the app and
matching-service repositories; this repo only ever sees one row.

## Activation, in order

Every step is a precondition for the next. Skipping one does not degrade the
system gracefully — it produces something that looks like it works.

**1. Give every party an HTTPS endpoint.** Checked 2026-08-19: neither box
serves TLS on its IP, and only `aequitas.digital` (in front of C1) has a
certificate. C2 has no HTTPS endpoint at all, so no committee can form today.
`NewHTTPTransport` refuses plaintext peers on purpose — anyone able to inject on
that path can force a "no match" and mint a second account for someone already
registered. There is precedent for the setup in
`.github/workflows/contabo1-serve-domain.yml`.

**2. Let the peers re-register**, so the registry records each one's signing
address alongside its URL. Committee membership is derived from that pairing;
before it exists, `MPCCandidates` returns too few entries and
`SelectCommittee` refuses.

**3. Run the dealer, somewhere that is neither party:**

```
go run ./cmd/mpc-dealer -count 5000000 -parties 2 -out /secure/triples
```

Deliver each file to exactly one party, set `MPC_TRIPLE_FILE` there, and destroy
the dealer's copies. Sizing: one registration against 1200 candidates needs
`TriplesForVerifiedWork(1200 x 2 x 512)`, roughly 2.5M triples, so 5M covers two
registrations. This is the part that does not yet scale — see the open gaps.

**4. Turn on the parties**, without the gate:

```
MPC_ENABLED=true
MPC_COMMITTEE_SIZE=2
MPC_CLIENT_TOKEN=<shared with the capture pipeline>
MPC_TRIPLE_FILE=/secure/triples/triples-party-N.bin
```

Registration is unaffected while `MPC_REQUIRED` is unset. Watch that
`/mpc/check` returns verdicts and that the two parties agree.

**5. Calibrate**, with at least 1000 labelled pairs per class from real captures
through the real sketch pipeline. `RecommendThreshold` refuses smaller samples.

**6. Only then set `MPC_REQUIRED=true`.** Before calibration the threshold is a
guess, and turning the gate on would refuse real people on a number nobody has
measured.

## Open gaps

Still open, and deliberately so:

- **The dealer exists.** Verified and constrained as above, not removed. Needs a
  vetted OT or HE implementation; see the cost table.
- **Two parties means the operators can collude.** The protocol supports more
  today; what is missing is an independent operator.
- **No sharding**, so throughput is capped by one link's bandwidth at roughly 3
  registrations/second on 1 Gbit/s.
- **Registration is not gated by default.** `MPC_REQUIRED` is off until the
  threshold is calibrated; turning it on before that would refuse real people
  on a guessed number.
- **The threshold is not yet calibrated.** The harness is ready and tested
  against known ground truth; it has not been run on real captures, because
  there are none yet. Until it is, the deployed threshold is still a guess.
- **Semi-honest security model.** Verification catches forged triples, but a
  party that deviates from the protocol in other ways is not detected. Malicious
  security needs authenticated shares (MACs, as in SPDZ).
