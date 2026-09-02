# Who may hold AEQ, and under what rules

Settled 2026-08-20, from the live ledger rather than from intent.

## The question as asked, and why the answer is not a number

"How many non-registered humans may own AEQ?" invites a cap — some allowance of
non-human accounts. A cap would be both unenforceable and beside the point.
Anyone can generate addresses, so a limit on their *number* limits nothing; and
the harm a non-human holder could do has nothing to do with how many there are.

The binding question is different: **is every AEQ subject to the same economic
rules, wherever it sits?** If yes, a non-human holder is just a wallet, and the
count is irrelevant. If no, non-human holding is an escape hatch, and one
account is already too many.

So the rule is stated as a property, not a quota.

## The rule

> **Anyone may hold AEQ. Every holding is subject to demurrage and the wealth
> cap, whether or not a registered human is behind it. The only exemptions are
> the protocol's own plumbing, and each one is named here.**

That is what makes the count uninteresting. A non-human wallet that decays and
is capped exactly like a human's is not an evasion — it is a second wallet, a
merchant, a multisig, a contract holding funds on someone's behalf. Forbidding
those would make the currency unusable for commerce without protecting anything.

## Where it holds today, measured

| holder | demurrage | wealth cap |
|---|---|---|
| human account balance | yes | yes |
| **non-human account balance** | **yes** | **yes** |
| tokenomics pools (ubi, lp, validators, treasury) | exempt | exempt |
| **LP shares / the AMM reserve** | yes since 2026-08-20, but see the correction below — demurrage reaches no active account anyway | **yes** (since 2026-08-20) |

Both the non-human and the LP rows are enforced, and deliberately so —
`enforceWealthCapLockedCtx` carries the comment "Deliberately NOT gated on
acc.IsHuman", and `settleDemurrageLockedCtx` exempts only the tokenomics pool
addresses. `TestNonHumanAccountsAreNotAnEscapeHatch` pins both, so an
`IsHuman` check added later fails a test rather than quietly opening the hatch.

The tokenomics pools are exempt on purpose: they are transit, not wealth. AEQ
lands there as fees and leaves again as UBI, LP yield and validator rewards.
Decaying money on its way to being redistributed would just be a second, hidden
fee.

## The gap this exposed, and how it was settled

Demurrage was levied on `acc.Balance`, and the wealth cap read `acc.Balance`.
LP shares are not balance and the AMM reserve is not an account, so **AEQ
deposited as liquidity escaped both rules**. Deposit, wait, withdraw: the decay
for that period was avoided, and the cap never saw the holding at all. Measured
on 2026-08-20 the reserve held **596.89 AEQ — 3.9% of the entire supply** —
outside both rules while every ordinary balance decayed.

### The decision: both rules apply to LP value

The tempting exemption is the demurrage one, on the argument that pooled
liquidity "is not idle" and demurrage exists to punish idleness. That argument
does not survive contact with what the protocol is for.

**Liquidity providers are already paid for the service**, out of swap fees, via
`distributeLPPoolLocked`. Exempting them from demurrage on top was a *second*
payment — one nobody voted for, that scaled with wealth, and that was available
only to people who knew the trick. A rule that binds whoever has not discovered
the workaround is unfairness by sophistication, which is the precise opposite of
a protocol premised on everyone standing equal.

So:

- **Demurrage** is levied on balance *plus* the AEQ value of LP shares.
  `effectiveBalance` is not duplicated — it runs against a copy whose balance is
  the whole holding — so the grace period, the fair-share floor and the rate stay
  in exactly one place.
- **The wealth cap** counts the same total.
- Both take the amount owed from balance first, and reach into liquidity only
  for the remainder. Someone whose balance already covers it keeps their
  position untouched.

`lpValueLockedAEQ` is the one definition of "AEQ held as LP shares", and
`releaseLPForAEQ` is the one way to turn shares back into balance. It delegates
to `liquidateLPSharesForEscrowLocked`, which already burns the shares and
credits the account — the first version of the helper repeated that and credited
twice, which `TestRemoveLiquidityDeltaLocked_MirrorsPrimaryWealthCap` caught.

`TestLiquidityIsNoLongerAShelter` pins both halves.

### Correction: what this actually changes, and what it does not

The claim first written here — "anyone holding wealth as liquidity now decays
and is capped like everyone else, and that is a real reduction" — was only half
right, and the wrong half was stated most confidently.

**Demurrage does not fire for anyone receiving UBI, LP or not.**
`distributeUBIPoolLocked` credits every human and then calls `touchActivity`
(state.go, in the per-human credit loop), and `distributeLPPoolLocked` does the
same for every LP holder. The demurrage grace period is 90 days. A daily
distribution resets that clock for everyone, every day, so it never elapses.

So the "liquidity was a demurrage shelter worth 3.9% of supply" framing
overstated it. There was no shelter to speak of, because the thing it sheltered
from does not reach active participants at all.

What each half of the change is actually worth:

- **The wealth cap on LP value: real, and it bites immediately.** The cap has no
  idleness condition — it is `avg x multiplier`, checked on every enforcement.
  LP shares escaping it was a genuine hole: park wealth in the pool and the cap
  never saw it. That is closed, and it is the half worth having.
- **Demurrage on LP value: inert in practice.** It is still correct — a position
  that genuinely goes dormant (no fee revenue, so no LP distribution touching
  the holder) should not be exempt — but it is a completion of the rule, not the
  significant lever it was described as.

### The larger question this uncovered

Demurrage is documented as a wealth decay: WHITEPAPER.md, README.md and the
landing page all describe `(balance - fair_share) x 0.5% / month` on the excess.
But it only begins after 90 days of inactivity, and the daily UBI credit resets
that for every human. In practice it is therefore an **abandoned-account sweep**,
not a decay on hoarded wealth.

Both are defensible designs. What is not defensible is the documentation
describing one and the code implementing the other, because everyone reasoning
about the protocol's incentives is reading the first. That gap belongs on the
list of things to settle, and it is bigger than the LP question that surfaced it.

## What is not covered here

946 non-human accounts hold 15.55 AEQ between them, almost certainly load-test
residue in consensus state. Under the rule above they are entirely legitimate —
they decay and they are capped — so this is a tidiness question, not a
correctness one. The amounts are dust; the row count is the annoyance.
