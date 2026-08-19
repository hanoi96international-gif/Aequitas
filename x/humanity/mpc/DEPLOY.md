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

## Open gaps

These are known and deliberate, not oversights:

- **Beaver triples come from a trusted dealer.** The offline phase currently
  generates triples centrally. A production deployment needs them generated
  without a dealer (OT- or HE-based), or the dealer can forge comparisons.
  This is the largest remaining correctness gap.
- **Two parties means the operators can collude**, as above.
- **No sharding**, so throughput is capped by one link's bandwidth.
- **Thresholds are not calibrated against real captures.** The FAR/FRR numbers
  that decide who is wrongly refused come from synthetic vectors so far.
