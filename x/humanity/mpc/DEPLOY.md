# Running secure duplicate matching on the two validators

This describes how to turn the code in this package into two machines that
compare biometric templates without either of them ever holding one.

## What this protects, and what it does not

**Protects:** no single machine can reconstruct a template. Each validator holds
one additive share — a vector of uniformly random field elements. Reconstruction
requires both. If one box is seized, compromised, or subpoenaed, the templates
on it are noise.

**Does not protect against:** the two operators colluding. With two parties,
both shares together reconstruct everything. Two parties is the minimum that
provides any protection at all, not a comfortable margin. The security argument
is only as strong as the independence of the two operators, and today both boxes
are administered by the same person — so the current deployment protects against
an attacker who takes one box, and not against the operator.

Adding a third independently-operated party is the single largest improvement
available, and the protocol already supports any number.

## The two-party quorum problem

With two parties there is no majority. If one is down, or the two disagree, there
is no third vote to break the tie. The system therefore fails **closed**: no
comparison, no registration. That is the correct direction — a wrongly refused
registration can be retried, a wrongly granted second account cannot be taken
back — but it does mean **either box being down halts registration entirely**.

This is a deliberate trade, not an oversight. It should be revisited when a
third party exists.

## Configuration

Both boxes need all of these. They must agree on the peer list and its order.

| Variable | C1 (`173.249.37.118`) | C2 (`194.163.188.71`) |
|---|---|---|
| `MPC_ENABLED` | `true` | `true` |
| `MPC_PARTY_INDEX` | `0` | `1` |
| `MPC_PEERS` | `https://<c1>,https://<c2>` | `https://<c1>,https://<c2>` |
| `MPC_PEER_TOKEN` | same secret on both | same secret on both |

`MPC_PEERS` is in **party order**: entry 0 is party 0's base URL. Both boxes get
the identical list; only `MPC_PARTY_INDEX` differs.

### The token

`MPC_PEER_TOKEN` is the only thing preventing a stranger from contributing to a
comparison and steering it to answer "this person is new". At least 32
characters, generated with a CSPRNG, never committed. Generate it on the box:

```bash
openssl rand -hex 32
```

Set the same value on both boxes. Rotating it requires restarting both;
mid-flight comparisons will fail closed, which is the intended behaviour.

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

`401` = mounted and protected. `404` = not mounted, check the startup log.

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

## Open gaps

Still open, and deliberately so:

- **The dealer exists.** Verified and constrained as above, not removed. Needs a
  vetted OT or HE implementation; see the cost table.
- **Two parties means the operators can collude.** The protocol supports more
  today; what is missing is an independent operator.
- **No sharding**, so throughput is capped by one link's bandwidth at roughly 3
  registrations/second on 1 Gbit/s.
- **The threshold is not yet calibrated.** The harness is ready and tested
  against known ground truth; it has not been run on real captures, because
  there are none yet. Until it is, the deployed threshold is still a guess.
- **Semi-honest security model.** Verification catches forged triples, but a
  party that deviates from the protocol in other ways is not detected. Malicious
  security needs authenticated shares (MACs, as in SPDZ).
