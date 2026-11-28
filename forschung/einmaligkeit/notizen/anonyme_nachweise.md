# Anonymous credentials and ZK tooling for proof-of-personhood (unlinkable, one claim per epoch). Status as of 2026-11-28

Method note: research date 2026-11-28. The session's network blocked direct fetches of docs.semaphore.pse.dev, semaphore.pse.dev, datatracker.ietf.org, docs.rs and zkmopro.org. Those hosts must be allowed in the session network settings to verify further. Facts from those hosts below come from search-engine snippets of the pages (URL given), not from full-page reads. github.com was reachable and was read directly.

## 1. Semaphore v4: design, libraries, license, audits, mobile cost, users

### Takeaway
Semaphore v4 is the most mature, MIT-licensed, audited "anonymous group membership + scoped nullifier" building block. Its components: an EdDSA (Baby Jubjub) identity, a Poseidon commitment as the Merkle leaf (LeanIMT, depth up to 32), and nullifier = hash(scope, private key). Proofs are Groth16 on BN254, with a public multi-party trusted setup run in July 2024. It maps directly onto "one UBI claim per epoch": set scope = epoch ID, and the contract stores the nullifiers it has seen. Full proofs (witness + Groth16) take well under ~1 s on recent phones with native provers. The exact numbers come from mopro and could not be fully verified.

### Cited Findings
- Identity: an EdDSA public/private key pair plus a commitment. The EdDSA is built on Baby Jubjub and Poseidon, and the commitment is the Poseidon hash of the public key. — [Semaphore glossary (search snippet)](https://docs.semaphore.pse.dev/glossary)
- The nullifier is "the hash of the scope and private key of the user's Semaphore identity". — [Semaphore glossary](https://docs.semaphore.pse.dev/glossary)
- A group is a Merkle tree whose leaves are identity commitments. It uses LeanIMT, an optimized binary incremental Merkle tree with Poseidon nodes. — [Semaphore glossary](https://docs.semaphore.pse.dev/glossary)
- v4 supports trees up to depth 32 (secondary write-up). — [semaphore under the hood (blog)](https://n0zom1z0.github.io/posts/semaphore-under-the-hood/)
- The proof system is Groth16 over BN254, with a separate circuit key per depth. Older semaphore-rs uses `identityNullifier`/`identityTrapdoor` (the v3-style secret split). — [semaphore-rs protocol source (docs.rs)](https://docs.rs/semaphore-rs/latest/src/semaphore_rs/protocol/mod.rs.html)
- Trusted setup: an MPC Phase-2 ceremony for the V4 circuits with 400+ participants in July 2024. — [Semaphore docs llms.txt (search snippet)](https://docs.semaphore.pse.dev/llms.txt); [trusted-setup page](https://docs.semaphore.pse.dev/trusted-setup)
- Audit: the "Semaphore 4.0.0 Audit" PDF covers circuits, contracts and libraries and is dated March 2024 (per search summary). Findings it lists include a missing admin access check and a constraint on the Baby Jubjub secret scalar. — [Semaphore_4.0.0_Audit.pdf](https://semaphore.pse.dev/Semaphore_4.0.0_Audit.pdf). Veridise also lists a Semaphore engagement. — [Veridise](https://veridise.com/?p=10962)
- The project site says "Semaphore V4 has been released". — [semaphore.pse.dev](https://semaphore.pse.dev/); Devcon 7 (Nov 2024) talk "Semaphore V4" — [Devcon archive](https://archive.devcon.org/devcon-7/semaphore-v4)
- Monorepo license: MIT. JS packages: `@semaphore-protocol/core`, `identity`, `group`, `proof`, `data`, `hardhat`, `cli`, `utils`. Solidity: `@semaphore-protocol/contracts`. Circuits are written in Circom (`packages/circuits`). (GitHub page read 2026-11-28; the Releases section was not rendered.) — [GitHub semaphore-protocol/semaphore](https://github.com/semaphore-protocol/semaphore)
- Rust: `semaphore-rs` describes itself as "Semaphore v4 implemented in Rust", but its releases page says "No releases published" (maturity signal). — [GitHub semaphore-rs](https://github.com/semaphore-protocol/semaphore-rs)
- A Noir port exists: hashcloak/semaphore-noir, benchmarked in mopro. — [mopro performance page (older build)](https://30fcbdc3.mopro.pages.dev/docs/performance)
- Mobile figures (mopro docs; different versions and devices, so indicative only):
  - Semaphore-32 witness generation on iPhone 16 Pro: ~166 ms (witnesscalc), ~277 ms (circom-witnesscalc), ~2.1 s (wasmer). — [mopro performance](https://e96d117a.mopro.pages.dev/docs/performance)
  - Semaphore-32 proof generation on Samsung S23 Ultra: ~793 ms (mopro arkworks) vs ~1,045 ms (snarkjs). — [mopro performance (older)](https://30fcbdc3.mopro.pages.dev/docs/performance)
  - semaphore-noir rows show 828 ms / 3,990 ms and 436 ms / 4,638 ms. The column headers were unclear in the extraction, so do not quote these as a mopro-vs-browser comparison. — same source
- Native mobile proving is described as up to ~10x faster than the same prover running in WASM. — [Wavect 2026](https://wavect.io/blog/zero-knowledge-proofs-production-2026/)
- World ID historically used Semaphore: walletkit-core (World's wallet library) depends on semaphore-rs to compute nullifier hashes, and in 0.15.1 that dependency sits behind an optional feature (the legacy/v3 path). — [walletkit-core 0.16.1 source](https://docs.rs/crate/walletkit-core/0.16.1/source/src/v3/world_id.rs); [walletkit-core 0.15.1](https://docs.rs/crate/walletkit-core/0.15.1/source/src/v3/proof.rs)

### Inferences
- For a daily UBI: scope = hash(contract, day_index). The contract checks `!used[nullifier]` and a valid Merkle root (keep a bounded root history), then pays. The same person on another day produces an unlinkable nullifier. Semaphore v4 also has no identity-trapdoor split, so the private key is the single secret. Losing it means losing the identity. Key rotation means a new commitment, which the issuer must re-admit without allowing two live leaves for one person.
- The Groth16 setup is a shared, already-performed ceremony. Reusing the official artifacts avoids running your own ceremony, provided the circuit is unchanged.
- An anonymity-set caveat: the group's size and its leaf-insertion timing leak information. Payouts to a fresh address right after enrolment can be linked by timing.

### Gaps
- The exact v4 circuit nullifier expression could not be read (docs host blocked). Verify in `packages/circuits/src/semaphore.circom`.
- No dated latest npm release (e.g. 4.x patch level, 2025/2026) was confirmed. No post-March-2024 re-audit was found.
- No end-to-end (witness + prove) Semaphore v4 time on a mid/low-end Android was found.

## 2. World ID / IDKit and the World ID 4.0 protocol internals; self-hosting or forking

### Takeaway
World ID 4.0 (2026) moved from plain Semaphore nullifiers to OPRF-derived nullifiers. A threshold set of OPRF nodes, keyed per relying party (RP) through an on-chain `OprfKeyRegistry` on World Chain, jointly computes the nullifier input. As a result neither the user's secret alone nor a single node determines it, and RP requests must be signed. The protocol repo (Circom circuits, Solidity, Rust crates, gateway/indexer/oprf-node services) is MIT-licensed and says third-party or self-hosted services are allowed. So a fork is technically possible, but the uniqueness anchor (Orb enrolment) and the OPRF key holders belong to World's ecosystem.

### Cited Findings
- The `worldcoin/world-id-protocol` repo holds "Core protocol infrastructure and developer documentation for World ID". License: MIT. Directories: `circom/`, `contracts/`, `crates/`, `services/` (gateway, indexer, oprf-node), `docs/`, `audits/`. Core contracts and the OPRF Key Registry are deployed on World Chain (chain ID 480). The reference indexer/gateway "isn't required"; you may "host your own services or use third-party services". It "undergoes continuous audits". About 901 commits on main (read 2026-11-28). — [GitHub worldcoin/world-id-protocol](https://github.com/worldcoin/world-id-protocol)
- The `world-id-proof` crate (MIT, repo github:worldcoin:protocol) generates nullifiers "using the OPRF Nodes" via a distributed protocol. The required threshold of node responses is set by the on-chain `OprfKeyRegistry`. — [world-id-proof 0.5.4](https://docs.rs/world-id-proof/0.5.4/world_id_proof/nullifier); [nullifier.rs 0.4.2](https://docs.rs/world-id-proof/0.4.2/src/world_id_proof/nullifier.rs.html)
- OPRF domains in `world-id-primitives`: `Nullifier`, `Session`, `CredentialBlindingFactor`. Nodes use the same `oprfKeyId` per RP with different domain separators. — [world-id-primitives 0.8.0 oprf.rs](https://docs.rs/crate/world-id-primitives/0.8.0/source/src/oprf.rs); [0.9.0](https://docs.rs/crate/world-id-primitives/0.9.0/source/src/oprf.rs)
- Action prefixes: nullifier actions start with byte 0x00, session actions with 0x01, which keeps uniqueness nullifiers separate from session IDs. A session lets an RP recognise the same World ID across interactions. Session nullifiers "do NOT guarantee uniqueness" the way a nullifier does. — [world-id-primitives session.rs 0.9.0](https://docs.rs/crate/world-id-primitives/0.9.0/source/src/session.rs); [walletkit-core v3 world_id.rs](https://docs.rs/crate/walletkit-core/0.16.1/source/src/v3/world_id.rs)
- Requests carry `rp_id` plus an RP signature. Errors include `timestamp_too_old` and `invalid_action_for_nullifier`. The signature can be omitted if the RP is a "WIP101 conform contract". — [world-id-primitives SessionId docs](https://docs.rs/world-id-primitives/0.13.0/world_id_primitives/struct.SessionId.html); [world-id-authenticator 0.9.0 prove.rs](https://docs.rs/crate/world-id-authenticator/0.9.0/source/src/prove.rs)
- The legacy definition: the "nullifier hash is a unique pseudo-random number for the particular identity and context". — [walletkit-core](https://docs.rs/crate/walletkit-core/0.16.1/source/src/v3/world_id.rs)
- World ID 4.0 launch (2026) press: key rotation "detaches keys from identity", multi-party entropy makes "every interaction unlinkable", existing credentials stay valid, and Zoom and DocuSign are integrating. Single-use nullifiers produce a one-time token specific to the consuming service. — [Computerworld](https://www.computerworld.com/article/4160511/world-id-expands-its-proof-of-human-vision-for-the-ai-era.html); [idtechwire](https://idtechwire.com/world-ships-world-id-4-0-as-zoom-and-docusign-adopt-proof-of-human/); [MobileIDWorld](https://www.mobileidworld.com/world-ships-world-id-4-0-app-with-zoom-docusign-integrations/)
- Least Authority published audits of TACEO's OPRF circuits, in both Circom and Noir versions (TACEO appears to be the threshold-OPRF/MPC provider; only the titles were seen). — [Least Authority: TACEO OPRF Circom](https://leastauthority.com/taceo-oprf-circom-circuits-combined-final-audit-report); [TACEO OPRF Noir](https://leastauthority.com/taceo-oprf-noir-circuits-final-audit-report); [Least Authority tag: Tools for Humanity](https://leastauthority.com/blog/tag/tools-for-humanity/)
- IDKit is described as the integration SDK. Its configuration options were not found. — [MEXC news (low-quality aggregator)](https://www.mexc.com/news/1036843)

### Inferences
- What the OPRF design adds over Semaphore: (a) RPs need a registered key, so nullifier generation is permissioned and rate-limitable by World. (b) If a user's secret leaks, an attacker still cannot precompute nullifiers offline without the node threshold. (c) The trust cost is a set of OPRF operators who must not collude.
- A fork is feasible because of the MIT code, but it means running your own OPRF node set, registry and issuer. Without that, the Semaphore-style "nullifier = H(scope, sk)" is simpler and needs no online third party at proof time.

### Gaps
- No official world.org developer doc page was retrieved (exact nullifier formula, TACEO threshold parameters, proof system behind `circom/`: Groth16 assumed, not confirmed).
- IDKit's license and whether it can target a non-World verifier: not verified.

## 3. Anonymous credential schemes and libraries (BBS/BBS+, CL/AnonCreds, U-Prove, Idemix, pseudonyms per RP, EUDI ARF as reference)

### Takeaway
BBS is the leading pairing-based anonymous credential scheme in standardisation (IRTF CFRG). It has companion drafts for Blind BBS and per-verifier linkability (pseudonyms), all still Internet-Drafts in 2026, not RFCs. There are mature Apache-2.0 Rust libraries (zkryptium, AnonCreds/CL, Dock's crypto). In the EUDI context, Google pushes Longfellow-ZK (a SNARK over existing ECDSA/mdoc), arguing that BBS conflicts with device binding and with the EU's post-quantum preference. BBS/CL alone give "one pseudonym per verifier", not "one per verifier per epoch". The latter needs the pseudonym input to include the epoch, which the per-verifier draft's context-ID mechanism can carry.

### Cited Findings
- The `draft-irtf-cfrg-bbs-per-verifier-linkability` -00 (Jan 2025) defines a pseudonym that "will remain constant each time a Prover presents a BBS proof to the same Verifier". -01 is from March 2025. No 2026 revision was found. — [datatracker -00](https://datatracker.ietf.org/doc/draft-irtf-cfrg-bbs-per-verifier-linkability/00/); [ietf.org -01 txt](https://www.ietf.org/archive/id/draft-irtf-cfrg-bbs-per-verifier-linkability-01.txt)
- `draft-irtf-cfrg-bbs-blind-signatures`: -00 Jan 2025. The history lists -03 around 2026-06-26 (uploaded by Greg Bernstein). The -02 page shows "Expired, last updated 2026-03-16". The metadata conflicts, and -03 contents are unverified. — [history](https://datatracker.ietf.org/doc/draft-irtf-cfrg-bbs-blind-signatures/history/); [-02](https://datatracker.ietf.org/doc/draft-irtf-cfrg-bbs-blind-signatures/02/)
- The base `draft-irtf-cfrg-bbs-signatures`: zkryptium targets draft-10. The current revision number and state could not be verified (datatracker blocked). — [zkryptium docs.rs](https://docs.rs/crate/zkryptium/0.6.2)
- zkryptium (Rust, Apache-2.0) implements BBS+ per IRTF draft-10, plus blind BBS and per-verifier linkability. — [zkryptium 0.6.1](https://docs.rs/crate/zkryptium/0.6.1); [lib.rs](https://docs.rs/zkryptium/0.6.1/src/zkryptium/lib.rs.html)
- anoncreds-rs (Hyperledger): Apache-2.0, the reference implementation of AnonCreds v1.0 (CL signatures). The spec uses the LF Community Specification License v1.0. anoncreds-v2-rs also exists. — [anoncreds-rs mirror](https://gitee.com/hyperledger/anoncreds-rs); [anoncreds-spec](https://github.com/hyperledger/anoncreds-spec); [anoncreds-v2-rs](https://github.com/hyperledger/anoncreds-v2-rs)
- Dock's `docknetwork/crypto` (Rust core + WASM wrappers) was chosen in a 27 Jan 2026 evaluation by the eid-privacy project for its functionality and architecture. Its license was not confirmed. — [eid-privacy WP2 blog 2026-01-27](https://eid-privacy.github.io/wp2/2026/01/27/docknetwork-crypto-library.html)
- RWC 2025 talk (Anja Lehmann): ARF 1.7 (March 2025) uses salted-hash selective disclosure, which makes presentations linkable. Two fixes are proposed: signatures with built-in ZK (BBS, CL, PS) or general zkSNARKs. — [RWC 2025 slides](https://iacr.org/submit/files/slides/2025/rwc/rwc2025/95/95_slides.pdf)
- Mayrhofer, Lehmann, shelat (Google-affiliated), arXiv 2510.05419: EUDI pseudonyms, positioned for account creation and recovery with passkeys for routine login. It names "rate limited pseudonyms" as a goal against bot farms. — [arXiv 2510.05419](https://arxiv.org/html/2510.05419v1)
- Longfellow-ZK: a Google C++ library proving over existing ECDSA/SHA-256 mdoc credentials (no new issuance), deployed in Google Wallet (Bumble age check), with an ISRG Rust port (zk-cred-longfellow). It is submitted to the IETF as draft-google-cfrg-libzk. Its authors argue that device binding and legacy support rule out BBS-family schemes and that the EU, Apple and Google want PQ-friendly ZK. — [IETF 125 CFRG slides](https://datatracker.ietf.org/meeting/125/materials/slides-125-cfrg-longfellow-zk-00); [IETF 124 CFRG slides](https://datatracker.ietf.org/meeting/124/materials/slides-124-cfrg-longfellow-zk-01); [IETF 124 dispatch](https://datatracker.ietf.org/meeting/124/materials/slides-124-dispatch-longfellow-zero-knowledge-credentials-00). Counterpoint, Sporny (Digital Bazaar): BBS allows dynamic selective disclosure without a circuit per combination. — [W3C CCG list, June 2025](https://lists.w3.org/Archives/Public/public-credentials/2025Jun/0094.html)
- shelat at W3C CCG (Apr 2025): BBS has inherent unlinkability limits because the structure of revealed messages leaks. — [CCG minutes 2025-04-29](https://lists.w3.org/Archives/Public/public-credentials/2025Apr/0067.html)

### Inferences
- For a UBI chain: a BBS credential from a personhood issuer plus a per-verifier pseudonym where verifier_id = "ubi-contract||epoch" gives one pseudonym per epoch, the same property as a Semaphore nullifier. But BBS verification needs BLS12-381 pairings. On EVM that means the EIP-2537 precompiles; in a custom Go chain, a native verifier. Semaphore/Groth16 has cheaper and more established on-chain verification.
- BBS/CL issuers see the credential at issuance, but blind issuance (Blind BBS) hides the holder secret (prover_nym). This is the property needed so the issuer cannot compute the user's pseudonyms.

### Gaps
- No primary info gathered on U-Prove (Microsoft) or Idemix (IBM/Hyperledger Fabric) in 2026. Both are generally considered legacy/maintenance; not verified here.
- MATTR's BBS libraries (status/license 2026) were not checked.
- The exact final EUDI ARF version adopting a ZK mechanism was not found.

## 4. zk-SNARK/STARK stacks and mobile proving

### Takeaway
For phone-side proofs of membership plus nullifier, the realistic options in 2026 are Circom/Groth16 (smallest proofs, cheapest on-chain verification, needs a circuit-specific trusted setup) and Noir/Barretenberg UltraHonk (universal setup, good mobile memory profile). mopro wraps Circom, Halo2 and Noir for iOS and Android. General zkVMs (RISC Zero, SP1) need 2–10 GB of RAM and are not suitable for on-phone proving. gnark (Go) is relevant for the verifier side in a Go chain, but no mobile numbers were gathered.

### Cited Findings
- mopro wraps Circom, Halo2 and Noir provers for iOS and Android. Native is up to 10x faster than WASM in the browser. SP1 Hypercube is listed as production-ready (Succinct claim: 99.7% of Ethereum blocks in under 12 s on 16 RTX 5090s, Nov 2025). RISC Zero is mature for GPU proving. The article flags vendor numbers as unverified. — [Wavect, "ZK proofs in production 2026"](https://wavect.io/blog/zero-knowledge-proofs-production-2026/)
- Many zkVMs need 2–10 GB of RAM to prove. Baseline Android RAM is ~3.82 GB and baseline iPhone ~5.7 GB. Barretenberg verification is ~33 ms; RISC Zero ~12.5 ms. — [Client-side proving report Q1 (hackmd)](https://hackmd.io/@clientsideproving/report-q1)
- Payy left Halo2 for Noir + Barretenberg (UltraHonk) for mobile proving speed, small proofs and low memory, and judged RISC Zero-style STARKs a non-starter on mobile (vendor blog, Aztec). — [Aztec blog](https://aztec.network/blog/just-write-if-why-payy-left-halo2-for-noir)
- Large-scale mobile client-side proving benchmark: "FibRace" (arXiv 2510.14693). — [arXiv 2510.14693](https://arxiv.org/pdf/2510.14693)
- mopro: Keccak256 proving ~11.0 s on iPhone 15 Pro and ~23.9 s on Pixel 6 Pro, while RSA crashed on iPhone 15 Pro. That is a heavy circuit, unlike Semaphore. — [mopro performance](https://e96d117a.mopro.pages.dev/docs/performance)
- An SP1 bug disclosure triggered a debate about ZK security transparency. — [Blockworks](https://blockworks.co/news/succinct-sp1-bug)
- Trusted setup: Semaphore V4 Groth16 uses an MPC ceremony (July 2024, 400+ participants). — [docs llms.txt](https://docs.semaphore.pse.dev/llms.txt)

### Inferences
- A Semaphore-class circuit (Merkle depth ≤32 + Poseidon + EdDSA) is small, so it proves in about 1 s on mid-range phones with native Groth16 (arkworks/rapidsnark). On low-end Android, budget 2–4 s (extrapolated from the S23U and witness figures, not measured).
- PLONK/Honk-family (Noir) avoids per-circuit ceremonies. Groth16 has the cheapest verification (~3 pairings, ~200-300k gas on EVM; general knowledge, not sourced here).

### Gaps
- Noir 1.0 stable release status and a Barretenberg UltraHonk audit (2026) were not confirmed.
- No gnark/Halo2 mobile numbers or licenses were gathered. gnark is Apache-2.0 per general knowledge (unverified here).

## 5. Personhood Credentials papers (Adler et al. 2024; Ide & Sharma 2025)

### Takeaway
Adler et al. (arXiv 2408.07892) argue for multiple trusted issuers with a capped number of credentials per person, unlinkable use across services, and per-credential rate limits. arXiv 2502.16375 is a user study (Ide & Sharma, Feb 2025) whose participants favour periodic biometric re-verification and time-bound credentials. That supports annual liveness with expiring credentials.

### Cited Findings
- Adler et al.: multiple issuers "promotes choice" and lowers the single-issuer risk while still "limiting the total number of credentials". PHCs enable "per-credential rate limits on activities". — [arXiv 2408.07892 v2 (HTML)](https://arxiv.org/html/2408.07892v2); [PDF](https://arxiv.org/pdf/2408.07892); [MIT Media Lab coverage](https://www.media.mit.edu/articles/how-personhood-credentials-could-help-prove-you-re-a-human-online/)
- Ide & Sharma (submitted 22 Feb 2025): a human-centered design study of PHCs. Its design concepts include periodic biometric verification, time-bound credentials, visually interactive human checks, and government supervision of issuance. It contrasts centralized and decentralized issuance. — [arXiv 2502.16375](https://arxiv.org/html/2502.16375v1)
- Rate-limited pseudonyms against "troll bots" with many accounts. — [arXiv 2510.05419](https://arxiv.org/html/2510.05419v1)

### Inferences
- With multiple issuers, a person enrolled at two issuers could claim twice. That requires either a cross-issuer dedup (which links data) or a cap of one issuer per person, which is a policy problem rather than a cryptographic one.

### Gaps
- The full text of 2408.07892 v3 recommendations (exact list) was not re-read in this session.

## 6. RLN, MACI, unlinkable-but-sybil-resistant designs, revocation and expiry without linking

### Takeaway
RLN extends Semaphore: per epoch, a user may emit at most N signals, and exceeding the limit reveals the secret through Shamir shares, which enables slashing. MACI (v3, audited by HashCloak in 2025) adds collusion resistance for voting via a coordinator, which UBI claiming does not need. For annual liveness without linking: rotate groups (a new Merkle tree per year, re-enrol after the liveness check, old root expires), or prove non-membership in a revocation list or an expiry attribute inside ZK. Accumulator-based non-revocation proofs exist but have stale-witness costs.

### Cited Findings
- RLN overview (PSE). — [PSE blog: Rate-Limiting Nullifier](https://pse.dev/blog/rate-limiting-nullifier-rln); RLN paper — [arXiv 2207.00117](https://arxiv.org/pdf/2207.00117.pdf)
- RLN Circom circuit audits: Veridise (1–12 May 2023) and yAudit/Electisec (June 2023, fixes merged). No 2025 RLN-v2 audit was found. — [Veridise RLN audit](https://veridise.com/audits-archive/company/ethereum-pse/rate-limiting-nullifier-2023-09-01/); [Electisec 2023-06 RLN](https://dev-reports-electisec.netlify.app/pdf/2023-06-RLN.pdf); [thogiti audit write-up](https://thogiti.github.io/2023/06/14/Rate-Limiting-Nullifier-RLN-Protocol-Circom-Circuits-Security-Audit.html)
- zerokit (Rust RLN, used by nwaku/Waku v2) and the `rln` crate. — [GitHub vacp2p/zerokit](https://github.com/vacp2p/zerokit); [rln 0.8.0](https://docs.rs/crate/rln/0.8.0); RLN v3 design note — [vac.dev](https://vac.dev/rlog/rln-v3)
- MACI v3: HashCloak review of contracts, Circom circuits and the TS core. The report text dates it 20 Feb, 3 Mar and 17 Mar 2025, while the MACI site lists "2026/03". The dates are inconsistent. — [HashCloak report PDF](https://maci.pse.dev/assets/files/20260317_Hashcloak_audit_report-f9fdba01db9fcc7092adbfaf27297836.pdf); [MACI security/audit page](https://maci.pse.dev/docs/security/audit)
- Revocation: ZK non-membership in a blacklist, or accumulator inclusion. — [arXiv 2509.11934](https://arxiv.org/html/2509.11934v1). An ePrint 2026/1824 paper on accumulator revocation for digital identity discusses epoch length and what a holder leaks when fetching partial revocation state. — [ePrint 2026/1824](https://eprint.iacr.org/2026/1824.pdf). A NIST WPEC 2026 talk covers revocations that make witnesses stale, and delegation with constant online cost. — [NIST WPEC 2026 3b2](https://csrc.nist.gov/presentations/2026/wpec2026-3b2). DIF non-revocation token. — [identity.foundation](https://identity.foundation/revocation/non-revocation-token/)
- "Proving non-expiration without revealing the correlatable issuance or expiration date is arguably one of the easiest parts". The same paper uses a Merkle-root revocation registry as public input. — [arXiv 2301.00823](https://arxiv.org/pdf/2301.00823)
- zk-creds: rate limiting via a pseudorandom token per (epoch, counter), and clone resistance by deanonymizing over-use. — [zk-creds slides](https://www.cs.purdue.edu/homes/white570/media/zk-creds%20(UIUC).pdf)

### Inferences
- Simplest liveness design for a Semaphore stack: keep one group per validity year. On the yearly liveness check the issuer inserts a fresh commitment into the new year's group, and the contract accepts only roots of the current group. Nothing links the year-N and year-N+1 commitments on-chain if the user picks a fresh identity. The issuer still knows "this person enrolled again" off-chain, which is unavoidable if the issuer dedups.
- Revocation (e.g. a deceased person or a duplicate found later) without linking needs either a leaf-removal plus root update (LeanIMT supports update and remove), which reveals which leaf but not its past nullifiers, or a ZK non-membership proof.

### Gaps
- No proof-of-personhood-specific comparison of these revocation approaches was found.

## 7. Post-quantum anonymous credentials

### Takeaway
PQ anonymous credentials are still at research stage in late 2026. They are lattice-based, with credential proofs around 50–80 KB (Argo et al.), and there are no production libraries. Hash/IOP-based SNARKs (STARKs, Ligero/Longfellow-style) are the practical PQ-leaning route. Every production stack above (Semaphore/Groth16 BN254, BBS BLS12-381, CL/RSA) is not post-quantum secure.

### Cited Findings
- Argo et al., ePrint 2024/131 (CCS 2024), "Practical Post-Quantum Signatures for Privacy": with a newer preimage sampler (Asiacrypt 2025, ePrint 2024/1952) the credential proof shrank from 79.58 KB to 53.98 KB (Aug 2025 update). — [ePrint 2024/131](https://eprint.iacr.org/2024/131)
- Jeudy & Sanders, ePrint 2026/1920, compact lattice anonymous credentials from tighter approximate range proofs: "current quantum-safe solutions based on standard assumptions are still lagging behind" ad-hoc ones. — [ePrint 2026/1920](https://eprint.iacr.org/2026/1920)
- Lattice EPID with efficient revocation (ePrint 2025/1225); log-size lattice set-membership NIZK (ePrint 2026/1885). — [2025/1225](https://eprint.iacr.org/2025/1225); [2026/1885](https://eprint.iacr.org/2026/1885)
- IBM: "A framework for practical anonymous credentials from lattices". — [IBM Research](https://research.ibm.com/publications/a-framework-for-practical-anonymous-credentials-from-lattices). Older lattice AC paper — [ePrint 2022/509](https://ia.cr/2022/509)
- Longfellow-ZK authors argue that the EU, Apple and Google want PQ-friendly ZK rather than new ECC schemes. — [IETF 125 CFRG slides](https://datatracker.ietf.org/meeting/125/materials/slides-125-cfrg-longfellow-zk-00)
- Related 2026 preprints (titles only seen): "ZK-ACE: Identity-Centric ZK Authorization for Post-Quantum Blockchain Systems" — [arXiv 2603.07974](https://arxiv.org/pdf/2603.07974); "BioBind: Binding Biometric Deduplication to Zero-Knowledge Authentication" — [arXiv 2611.12636](https://arxiv.org/pdf/2611.12636)

### Inferences
- Pragmatic stance: anonymity in Semaphore/Groth16 is statistically or perfectly zero-knowledge, so past proofs stay anonymous even against a future quantum adversary. What breaks is soundness: forging proofs, and recovering the EdDSA private key from the public commitment input. So the PQ risk is future forgery, and therefore theft of UBI, rather than retroactive deanonymisation. Mitigation: plan a migration to a hash-based SNARK (e.g. STARK/WHIR-based) once mobile-practical. (This rests on known Groth16 properties; no source was fetched for it here.)

### Gaps
- No hash-based anonymous credential library with mobile benchmarks was found. The WHIR-based client-side note exists — [hackmd whir-based](https://hackmd.io/@clientsideproving/whir-based) — but was not read.

## 8. Self.xyz, ZKPassport, Rarimo: nullifier design reference only

### Takeaway
All three use the same pattern: a per-document deterministic nullifier blocks duplicate registration, and per-app/scope action nullifiers are derived from the user secret and the app scope. Self and Rarimo use Circom; ZKPassport uses Noir. State documents are out of scope as an anchor here; these are listed only as design references.

### Cited Findings
- Self: a deterministic nullifier per passport, and the contract rejects registration if the nullifier was already seen (zkSecurity audit). The action nullifier is derived from the user secret and the app scope. Circuits are written in Circom. — [zkSecurity audit of Self](https://reports.zksecurity.xyz/reports/celo-self-audit/); [Google Cloud Self case study](https://services.google.com/fh/files/misc/self_case_study.pdf)
- Rarimo: commitment = Poseidon(nullifier, secret), with the nullifier hash public. Halborn audited the circuits and the findings were fixed. — [rarimo passport-zk-circuits README](https://github.com/rarimo/passport-zk-circuits/blob/main/README.md); [Halborn audit](https://www.halborn.com/audits/rarimo/passport-zk-circuits)
- ZKPassport: NFC + Noir circuits. Its identifier is domain- and scope-scoped. — [IPTF private identity](https://iptf.ethereum.org/use-cases/private-identity/); [GitHub zkpassport](https://github.com/zkpassport); overview — [Safe Research](https://safefoundation.org/blog/safe-research-zk-passport-where-are-we-now)

### Inferences
- The two-level pattern (registration nullifier from the uniqueness source, action nullifier = H(secret, scope)) transfers directly. In a biometric-dedup system, the "registration nullifier" role is played by the dedup step at the issuer, and on-chain only the Semaphore commitment plus scoped nullifiers appear.

### Gaps
- No side-by-side comparison of the three projects' scope encodings was found.
