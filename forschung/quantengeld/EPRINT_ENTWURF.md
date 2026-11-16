# Decentralised private-key quantum money on a public ledger: distributed Wiesner notes and memoryless S-tokens

*Draft for IACR ePrint — Aequitas project, 16 November 2026. Not yet peer reviewed. Comments
welcome. Code, specifications and test vectors: this repository
(`forschung/quantengeld/`, `x/humanity/keeper/quanten_w*.go`, `quanten_s.go`).*

## Abstract

Private-key quantum money (Wiesner) is unforgeable by the no-cloning theorem, but it needs a
bank that knows every note's secret. We describe two constructions in which the bank is
replaced by the validators of a public ledger, so that no single party can verify or forge
alone.

- **Variant W.** A note is a string of BB84 states. The secret of each position (basis, bit,
  salt) is Shamir-shared among the note's validators and committed on the ledger under a
  Merkle root. A check is a public challenge–response:
  1. The challenge positions and bases come from pre-committed hash chains of all
     validators.
  2. The holder answers with classical bits.
  3. Only then do t validators reveal the shares of the challenged positions.
  4. Every node reconstructs, checks the commitments and judges by itself.
- **Variant S.** A memoryless S-token in the sense of Kent et al. (2022): the user measures
  immediately and keeps only classical data. The token can be redeemed at one of two fixed
  acceptance points. Each point verifies offline against its own secret, disjoint sample of
  the issuer's records, and redeems on the ledger with the user's signed presentation.

We report a parameter error we found in our own first design: a tolerance of 15 % wrong
answers is broken by the optimal phase-covariant cloner (14.64 % error per copy). We derive
integer parameter tables that bound a cloning forger by 2⁻⁴⁰, including coalitions of fewer
than t validators. We report runs on IBM hardware: a Variant W note is accepted
end-to-end without waiting, and fails after 20 µs plus teleportation. An S-token with
secure parameters (1,680 qubits) is accepted at its chosen point and rejected at the other.
Both variants are implemented in a production ledger node, disabled until an external audit.

## 1 Introduction

Quantum money promises cash that cannot be copied. Public-key quantum money is not known to be
secure under standard assumptions, and practical constructions need large fault-tolerant
devices (see e.g. Zhandry's lightning, and attacks on group-action candidates). Private-key
quantum money is unconditionally secure. Its weakness is organisational: the bank knows all
secrets, must be online for every verification, and can mint at will.

A public ledger already provides what the bank is needed for, except the secret:

- an always-online, replicated state;
- deposits and payouts in a native currency;
- a set of validators bound to distinct humans.

We ask how far private-key quantum money can be decentralised on such a ledger with today's
hardware. We also ask what can be shown on real devices now.

**Contributions.**
1. **Variant W:** Wiesner notes with secrets Shamir-shared among validators, committed
   per position, verified by public challenge and **reveal-after-answer**. Verification
   needs no multiparty computation: after the answer is final, t validators open the shares
   of the challenged positions, and every node judges itself (§3).
2. **Secure parameters:** a cloning analysis that corrects our own initial tolerance, and
   integer tables k(tol, f) for the number of checked positions (§5).
3. **Variant S:** S-tokens with **sample-based acceptance points**. Each point holds only a
   secret disjoint sample of the issuer's records, so a corrupted point cannot pass at
   another one. Redemption on the ledger needs the user's signature on her presentation
   (§4).
4. **Implementation and experiments:**
   - an executable specification, a node implementation that matches it byte for byte, and
     abuse tests checked by mutation (§6);
   - runs on IBM `ibm_marrakesh` and `ibm_fez` (§7).

## 2 Background

**Wiesner money and classical verification.**
- A note is |ψ⟩ = ⊗ᵢ |bᵢ, xᵢ⟩, where bᵢ ∈ {Z, X} and xᵢ ∈ {0, 1}.
- With classical verification, a verifier asks for a subset of positions in random bases.
  It accepts if the answers agree on the positions whose basis matches (hits), up to a
  tolerance.
- Pastawski et al. (PNAS 2012) show for classically verified tokens that the threshold
  fidelity is (1 + 1/√2)/2 ≈ 0.854. In other words, an error tolerance must stay below
  14.6 %.

**Cloning.**
- The optimal symmetric 1→2 phase-covariant cloner for states on a great circle (Bruß et al.
  1999) gives both copies fidelity 1/2 + 1/√8. That is an error of 14.64 % in either BB84
  basis.
- Measuring in the intermediate (Breidbart) basis gives the same error in both bases. This
  matters for S-tokens.

**S-money** (Kent 2019; Kent, Lowndes, Pitalúa-García, Rarity, npj QI 8, 28, 2022):
1. The issuer sends BB84 states.
2. The user measures each qubit at once in a random basis y and obtains x. She then
   announces d = y ⊕ z (z a secret bit) and later c = b ⊕ z, where b is the chosen
   presentation point.
3. The point b checks x on the positions where uₖ = dₖ ⊕ b ⊕ c, which are exactly the user's
   own bases.
4. Presenting at two points requires correct outcomes in both bases.

## 3 Variant W: distributed Wiesner notes

**Parties.**
- **Ledger:** every node validates every transaction.
- **Minter:** pays the cover in AEQ.
- **Validators of the note:** v of them, each bound in the ledger's validator registry to a
  different human.
- **Holder.**

**Minting.**
- For each position i the minter (or, without trust, remote state preparation at the
  validators, §3.5) fixes vᵢ = bᵢ + 2xᵢ + 4sᵢ with a 124-bit salt sᵢ. It publishes the leaf
  cᵢ = SHA-256(dom ‖ K ‖ i ‖ vᵢ) under a Merkle root.
- vᵢ is Shamir-shared (t of v) over GF(2¹²⁷−1), one polynomial per position.
- Every validator confirms (EIP-191):
  - the root, n, t, the validator list and the note's tolerance;
  - the anchor of its own hash chain of length 256 (the chain's last value);
  - the minting time.
- The note carries shares of a common pot. A circulation levy hits notes like any balance.

**A check** (anyone, against a deposit max(1 AEQ, value · k/n)):
1. **Request** for k positions. Redemption uses exactly k_note (§5). Smaller checks need
   k ≥ max(8, ⌈n/255⌉) and must leave k_note positions free plus the last hash-chain value.
2. **Randomness:** each validator reveals the next value of its hash chain once the request
   is final. When all v values are in, the seed fixes positions and bases (rejection
   sampling, Fisher–Yates). From then on these positions are consumed, whether or not
   anybody answers. Withholding a value only delays; there is no deadline. A deadline would
   let a withholder exhaust notes for free.
3. **Answer:** the requester posts classical bits within 600 s.
4. **Reveal:** once the answer is final, t validators open the shares of the challenged
   positions with their Merkle paths. This happens in parts of at most 64 positions, in
   order, each part all-or-nothing, within 1,800 s. Every node reconstructs vᵢ, checks the
   leaf and the path, and counts hits and errors.
5. **Verdict:** fewer than 4 hits means undecided (deposit returned). Otherwise the note
   passes iff errors · 100 ≤ tol · hits, in integers. A pass returns the deposit; on
   redemption the value goes to the target. A fail forfeits the deposit to the basic-income
   pool and counts a failure; three failures block checks, not redemption. A note with
   fewer than k_note free positions, or with exhausted chains, dies and its value goes to
   the basic income.

**Why reveal-after-answer is enough.**
- The holder must answer before any secret of the challenged positions is known.
- Revealed positions are consumed, and the secrets of other positions are independent
  (fresh salt, fresh polynomial).
- A wrong share is caught by the commitment: nodes never trust a validator.
- The validator service enforces *finality*. It releases a hash-chain value only when the
  request is final, and shares only when the answer is final. Otherwise a reorganisation
  could let a requester see the shares and then post a different answer.

**3.5 Minting without trust.**
- The minter prepares Bell pairs and measures nothing. Each half goes to one validator,
  who measures it in its own random basis, which fixes the note's state at that position.
- No single party knows the whole note. A coalition of m validators knows a fraction m/v
  of the positions.
- We have run this as a circuit (`note_fern`), also on real hardware (§7); over a real
  quantum link it is future work.

## 4 Variant S: memoryless tokens with sample-based acceptance points

**Issuance.**
- Records (u, t) for N qubits are Shamir-shared among validators and committed as in W.
- The issuer fixes two acceptance points (addresses) and, for each, the hash of a secret,
  disjoint sample of k positions.
- The user measures immediately, binds d and later chooses c on the ledger. Both are
  signed, once each.

**Offline acceptance.**
- Point b holds (u, t) only on its sample and checks x where uₖ = dₖ ⊕ b ⊕ c. It needs
  no network at presentation time; it must have synchronised d and c beforehand.
- If c could be stated per point, a user could make every point check her own bases. Our
  tests show this.
- A corrupted point knows only its own sample and cannot pass at the other point.

**Redemption on the ledger.**
- Only point b, as fixed at issuance, may register. It submits:
  - the user's EIP-191 signature on (K, b, H(x));
  - its sample, which must match the hash committed at issuance.
- **Why the user's signature:** without it, a point that knows its sample could redeem any
  token itself.
- **Why fixed points:** without them, the sample, visible once registered, could be copied
  from the queue by a front-runner.
- **Reveal:** validators reveal the sample in parts after the registration is final.
- **Pass:** the value goes to the point.
- **Fail:** counts a failure. The point's signature binds the failure count, so a failed
  registration cannot be replayed. After three failures the token dies (value to the
  basic income). A missed reveal deadline ends the registration without penalty.

## 5 Parameters

**Our initial parameters were broken.**
- We first used a tolerance of 15 % with at most 64 checked positions.
- The phase-covariant cloner gives each copy 14.64 % errors per hit. A cloned copy then
  passes a 64-position check with probability 0.55 (exact sum), both copies ≈ 0.3.
- Our earlier forger model (guessing unknown positions, 50 % error) had suggested
  3.8 · 10⁻⁵. The ledger was still disabled, so nothing was at risk.

**Secure tables.**
- Forger model: a coalition knows a fraction f of the positions and clones the rest. Its
  error rate per hit is c(f) = (1 − f) · 14.64 %.
- We compute, by exact binomial sums following the ledger's integer rule (minimum 4 hits,
  undecided may be retried), the smallest k (a multiple of 8) with forger acceptance
  ≤ 2⁻⁴⁰.
- The ledger takes f16 = ⌈16(t−1)/v⌉ as a conservative bound for coalitions below the
  threshold, and k_note = K[tol][f16]:

  | tol \ f16 | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 |
  |---|---|---|---|---|---|---|---|---|---|
  | 2 % | 552 | 600 | 672 | 752 | 856 | 984 | 1168 | 1408 | 1760 |
  | 3 % | 672 | 752 | 856 | 984 | 1144 | 1368 | 1680 | 2144 | 2880 |
  | 4 % | 840 | 960 | 1112 | 1304 | 1576 | 1960 | 2536 | 3472 | 5200 |
  | 5 % | 1064 | 1240 | 1464 | 1784 | 2240 | 2936 | 4096 | 6264 | – |
  | 6 % | 1360 | 1632 | 2000 | 2536 | 3360 | 4752 | 7416 | – | – |
  | 8 % | 2440 | 3144 | 4256 | 6184 | – | – | – | – | – |
  | 10 % | 5240 | 7808 | – | – | – | – | – | – | – |

  (– : not achievable below 8,192 positions; minting is rejected.)
- The node uses these integers directly. There is no floating point in consensus.
- The specification re-checks sample entries exactly: the bound holds at k and fails at
  k − 8.

**Honest acceptance.** With the error rate measured on IBM without waiting (0.7 % per
hit), tolerance 4 % and k = 840 give an honest failure probability below 10⁻⁸.

**Scope of the bound.**
- The tables bound a per-position (product) cloning attack per copy.
- Pastawski et al. prove the same threshold and a bound of the form exp(−h · D(tol ‖
  0.146)) for general attacks in their setting. That the same holds for our scheme, where
  each position is queried at most once and the copies are checked at disjoint positions,
  is plausible but is an open question for review.
- Variant S uses the same tables: the Breidbart double-presentation attack has the same
  14.64 % error.

## 6 Implementation and testing

**Executable specification** (Python, no dependencies): `wiesner_kette.py`,
`wiesner_parameter.py`, `s_kette.py`. Each runs a self-test with abuse cases:
- forged shares, wrong paths, foreign or duplicate share indices;
- answers in the wrong block, replays;
- exhausting notes, collusion;
- double presentation, corrupted points, front-running.

**Node** (Go, part of the Aequitas ledger):
- The rules live in `quanten_w.go`, `quanten_w_register.go` and `quanten_s.go`.
- They are wired into consensus in `quanten_w_kette.go` and `quanten_w_s_kette.go`: twelve
  transaction types, registers in the state root, snapshot, checkpoint and replay.
- The validator service is `quanten_w_validator.go`. It keeps an encrypted vault, confirms
  only signed orders from funded humans, releases only after finality, and submits
  reveals in parts with staggering.

**Discipline.**
- **Test vectors:** generated by the specification and checked byte for byte by the node
  in CI.
- **Mutation tests:** every new rule is checked by mutation; reverting the rule must turn
  a test red.
- **Integration:** an end-to-end test with four nodes over real HTTP and PostgreSQL.
- **Separate security pass:** before each merge. It found, among others:
  - a challenge derived from a block hash known before the request;
  - a deadline-free exhaustion attack;
  - chain exhaustion via many cheap checks (fixed by the minimum ⌈n/255⌉);
  - unauthenticated vault filling;
  - a redemption path for points without the user's signature.

## 7 Experiments

All runs are on IBM Heron devices (`ibm_marrakesh`, and `ibm_fez` where marked) through
the public Qiskit Runtime. Circuits and raw results are in the repository's workflow runs.

| What | Size | Result |
|---|---|---|
| W note, mint → challenge → answer → reveal → verdict, no waiting | 16 positions | accepted: 10 hits, 0 errors; 0.7 % error per hit over 1,000 shots, acceptance 0.998 |
| Same, 20 µs storage plus teleportation | 16 positions | rejected: 9 hits, 3 errors; 39.1 % error per hit |
| W note minted without trust (§3.5), no waiting, `ibm_fez` | 6 positions, 4 notes × 500 shots | 2.4 % error per hit, acceptance 0.90; with teleportation 12.5 %; after 20 µs 29 % |
| S-token, secure parameters (tol 4 %, k = 840 per point) | 1,680 qubits, 105 circuits × 1 shot | 0.46 % error per matching basis; chosen point accepted (438 hits, 4 errors), other point rejected (401 hits, 196 errors) |

These are demonstrations, not security experiments:
- issuer and holder are the same circuit;
- the device operator sees everything;
- the W note lives microseconds.

They show the error rates the parameters must accommodate, and that the protocol logic
works end-to-end with real measurement data. The 2.4 % of remote preparation still admits
secure parameters (tolerance 6 %, k = 1,360). The 12.5 % with teleportation does not: no
tolerance below 14.6 % meets both targets up to k = 8,192.

## 8 Limitations and open questions

- **Storage.** Transferable Wiesner notes need memories that last longer than a payment.
  Today's superconducting qubits do not. Trapped ions and solid-state memories with hours
  of coherence exist in laboratories.
- **What S-tokens are not.** They need no memory, but they are not hand-to-hand cash. They
  need a QKD-type receiver at the user, and their advantage over a ledger alone is instant
  offline acceptance plus physical unforgeability at the acceptance points.
- **Issuance over a real link (S3).** Losses, multi-photon pulses and basis misalignment θ
  change the parameters. We treat multi-photon positions among the reported ones as known
  to the forger (fraction ν/γ_det) and use the forger bound λ(θ, β) of Kent et al. instead
  of 14.64 %. Our tool reproduces their Fig. 5. Two findings:
  - Their hand-held sender is unforgeable by their Theorem 1 once bases and values come
    from the validators' randomness (β = 0). What blocked them was the bias of the device's
    random numbers.
  - The ideal-qubit table is not safe over a link: at 10° misalignment and 3 % error a
    forger would pass 1.4 % of the time.
- **Liveness.** A note stalls if one validator disappears forever. A unique threshold
  signature (t of v) would be a better randomness source.
- **Consistent shares.** Verifiable secret sharing (Feldman or Pedersen) would protect
  validators against inconsistent shares from a dishonest minter. Today this only hurts the
  minter's own note.
- **Questions for reviewers:**
  1. The general-attack version of the bound for "each position queried once".
  2. Whether f = (t−1)/v is the right coalition model.
  3. Economic griefing beyond the attacks we found.

## References

- S. Wiesner, Conjugate coding, SIGACT News 15 (1983).
- F. Pastawski, N. Y. Yao, L. Jiang, M. D. Lukin, J. I. Cirac, Unforgeable noise-tolerant
  quantum tokens, PNAS 109, 16079 (2012), arXiv:1112.5456.
- D. Bruß, M. Cinchetti, G. M. D'Ariano, C. Macchiavello, Phase-covariant quantum cloning,
  Phys. Rev. A 62, 012302 (2000), quant-ph/9909046.
- A. Molina, T. Vidick, J. Watrous, Optimal counterfeiting attacks and generalizations for
  Wiesner's quantum money, TQC 2012, arXiv:1202.4010.
- A. Kent, S-money: virtual tokens for a relativistic economy, Proc. R. Soc. A 475, 20190170
  (2019), arXiv:1806.05884.
- A. Kent, D. Lowndes, D. Pitalúa-García, J. Rarity, Practical quantum tokens without quantum
  memories and experimental tests, npj Quantum Information 8, 28 (2022), arXiv:2104.11717.
- M. Bozzio et al., Experimental investigation of practical unforgeable quantum money, npj
  Quantum Information 4, 5 (2018), arXiv:1705.01428.
- M. Zhandry, Quantum lightning never strikes the same state twice (and follow-ups on
  group actions).
