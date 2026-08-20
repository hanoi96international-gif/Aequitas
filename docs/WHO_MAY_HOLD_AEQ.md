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
| **LP shares / the AMM reserve** | **no** | **no** |

The non-human row is already correct, and deliberately so —
`enforceWealthCapLockedCtx` carries the comment "Deliberately NOT gated on
acc.IsHuman", and `settleDemurrageLockedCtx` exempts only the tokenomics pool
addresses. `TestNonHumanAccountsAreNotAnEscapeHatch` pins both, so an
`IsHuman` check added later fails a test rather than quietly opening the hatch.

The tokenomics pools are exempt on purpose: they are transit, not wealth. AEQ
lands there as fees and leaves again as UBI, LP yield and validator rewards.
Decaying money on its way to being redistributed would just be a second, hidden
fee.

## The gap this exposed: liquidity is a demurrage shelter

Demurrage is levied on `acc.Balance`. LP shares are not balance, and the AMM
reserve is not an account, so **AEQ deposited as liquidity stops decaying**.
The wealth cap has the same blind spot: it reads `acc.Balance` and ignores LP
shares entirely.

Measured on 2026-08-20, the reserve holds **596.89 AEQ — 3.9% of the entire
supply** — sitting outside both rules while every human balance decays.

The route is not subtle: deposit into the pool, wait, withdraw. Demurrage for
that period is avoided. Nothing stops it and nothing reports it.

### This is left open deliberately, and needs a decision

Three coherent answers, and the choice is an economic one rather than a
technical one:

1. **Levy demurrage on the AEQ value of LP shares.** Closes the shelter
   completely. Also taxes people for providing liquidity the exchange needs, and
   may empty the pool.
2. **Count LP-share value toward the wealth cap only.** Stops the cap being
   evaded while leaving liquidity provision undiscouraged. The demurrage shelter
   stays open.
3. **Accept it as an intended incentive** — liquidity is a service the protocol
   wants, and exemption is the payment. Then say so out loud, because at 3.9% of
   supply it is a meaningful subsidy that currently exists by accident rather
   than by decision.

Whichever is chosen, the current state should not persist by default: an
exemption nobody decided on is the same thing as a bug that has not been noticed
yet. The measurement is already published — `amm_reserve` in
`/api/health/combined` — so the size of the subsidy can be watched while the
decision is made.

## What is not covered here

946 non-human accounts hold 15.55 AEQ between them, almost certainly load-test
residue in consensus state. Under the rule above they are entirely legitimate —
they decay and they are capped — so this is a tidiness question, not a
correctness one. The amounts are dust; the row count is the annoyance.
