# Social / ceremony / web-of-trust proof of personhood without state ID (status: late Nov 2026)

Method note: In this session most hosts could not be fetched directly: arxiv.org, forum.polkadot.network, t.me, book.encointer.org, duniter.org, ssrn, harvard, bford.info and frontiersin were all blocked. Claims about papers therefore come from search-engine excerpts of the primary pages, and the cited URL is that primary page. Repository facts (license, last commit, commit counts, parameters in code) were checked directly with `git clone --bare --filter=blob:none` and raw.githubusercontent.com on 2026-11-28; those are first-hand. GitHub stars and forks could NOT be retrieved because the GitHub API and HTML were blocked, so they appear under Gaps. Unless marked "independent", usage numbers are project self-claims or third-party estimates.

## 1. Pseudonym parties (Ford & Strauss 2008; Borge et al. 2017; Ford 2020)

### Takeaway
Pseudonym parties are the cleanest *theoretical* basis for "one body, one token per period" without ID. Their security rests on physical simultaneity, not on a social graph. No production-grade implementation survives: the EPFL/DEDIS "PoP" prototype was a research artefact, and Encointer is the only live descendant. Known weaknesses are malicious or colluding organisers, scaling across federated parties, coercion and vote-buying, and exclusion of people who cannot attend.

### Cited Findings
- Ford & Strauss, "An Offline Foundation for Online Accountable Pseudonyms", SocialNets '08, April 2008, ACM DOI 10.1145/1435497.1435503, pp. 31–36. It combines in-person social events ("parties") with a pseudonymous sign-on service to enforce accountability. — [ACM DL](https://dl.acm.org/doi/abs/10.1145/1435497.1435503); [PDF MIT PDOS](https://pdos.csail.mit.edu/papers/accountable-pseudonyms-socialnets08.pdf)
- Ford 2020 describes the mechanism as "periodic real-world events where people wishing to wield a vote online gather to demonstrate their genuine personhood publicly, each obtaining one-per-person digital tokens usable for voting and other purposes during the next time period". Its security rests on the fact that real people "have only one physical body each that can be in only one place at a time". — [Ford, arXiv 2011.02412](https://arxiv.org/abs/2011.02412)
- Ford 2020 names open problems: securely scaling to large or global federations of pseudonym parties "without falling prey to digital fakery attacks by malicious organizers", timing-based inclusion, and coercion-resistant voting tokens. It also names exclusion: prisoners and residents of authoritarian surveillance states may be unable to organise or attend. — [arXiv 2011.02412](https://arxiv.org/pdf/2011.02412)
- The "Ghost Trilemma" SoK: holding organisers of each pseudonym party accountable to attendees and other organisers is hard (deepfakes), and coordinating gatherings across time zones is hard. — [SoK: The Ghost Trilemma, arXiv 2308.02202](https://arxiv.org/html/2308.02202.pdf)
- Borge, Kokoris-Kogias, Jovanovic, Gasser, Khoffi, Ford, "Proof-of-Personhood: Redemocratizing Permissionless Cryptocurrencies", IEEE S&B Workshop (EuroS&PW), Paris, April 2017. It combines pseudonym parties with linkable ring signatures and collective signing to issue anonymous "PoP-tokens", and proposes "PoPCoin" with PoP-based consensus. — [EPFL Infoscience](https://infoscience.epfl.ch/record/301744); [bford.info abstract](https://bford.info/pub/dec/pop-abs/)
- Bryan Ford gave a talk on coercion resistance on 2025-01-06, which shows ongoing work on the coercion and vote-buying weakness. — [bford.info talk PDF](https://bford.info/talk/2025-01-06-coercion-resistance/coercion-resistance.pdf)
- The EPFL master report (Hoffmann, 11 June 2021, supervised by Ford, Merino and Zhang) analysed how an adversary could profit from Encointer, the live pseudonym-party-like system, using simplified simulations. — [Encointer-hosted PDF](https://encointer.org/wp-content/uploads/2022/04/report-2021-1-hoffmann_21_pop_security_encointer.pdf)

### Inferences
- (a) Simultaneous double registration: strong in principle. One body cannot attend two synchronous parties. This breaks if parties are not globally synchronous, or if an organiser fakes attendees.
- (b) Restart with a fresh identity: tokens are per period by design, so there is no "lifetime" identity. Every period everyone is new, which makes restarting meaningless. This conflicts with "keep one registration for life" unless the token is linked across periods, for example by linkable ring signatures with a stable tag, as Borge et al. and Encointer's reputation rings do.
- (c) Renting or selling: tokens are transferable bearer credentials unless coercion-resistant designs are used. This is the main open problem (Ford's coercion work).
- (d) Collusion: a corrupt organiser can mint tokens for absent people. Federated, mutually auditing organisers, video or witness records, and random assignment of attendees (Encointer) are the proposed mitigations.

### Gaps
- No currently maintained open-source "pseudonym party" implementation was found besides Encointer. The status of the DEDIS PoP code (dedis/cothority "pop" service) was not verified because GitHub could not be browsed.
- The full text of Ford & Strauss 2008 could not be fetched to check exactly how it specifies global simultaneity.

## 2. Encointer

### Takeaway
Encointer is the only live, open-source, physical-co-presence PoP with UBI ("community income"). It uses randomly assigned meetups of 3–12/15 people at a synchronised time, every 10 days on mainnet, as a Kusama system or common-good parachain. Code is actively maintained in 2026 (pallets commits through 2026-11-23; anonymous "reputation rings" with Bandersnatch ring-VRF; "unlinkable PoP contexts" in Feb 2026). Adoption is tiny: a handful of communities, the most active in Africa, and Zurich's Leu at "low traction". The Encointer Association stopped subsidising communities in 2025/26. By design, reputation expires after 5 ceremonies, so there is no lifetime identity, and restarting with a new account is explicitly accepted.

### Cited Findings
**Mechanism (project docs and code):**
- Three phases: Registering (longest), Assigning (24 h; the chain closes registration and picks a random seed for meetup assignment), Attesting (48 h; meetups worldwide around local noon, participants mutually attest and send attestations on-chain, then rewards can be claimed). — [Encointer Book, ceremony cycle (raw source)](https://raw.githubusercontent.com/encointer/book/master/src/protocol-ceremony-cycle.md)
- "On mainnet, the ceremonies currently happen in intervals of 10d", to be lengthened later. — same source.
- Reputation lifetime is "currently set to 5 ceremonies on mainnet". Rationale: early adopters should not be favoured, and losing a phone has limited impact: "You just start all over with a new account". Reputation ratio "3/5 will prove that you are not maintaining 2 or more accounts". — [Encointer Book, reputation (raw)](https://raw.githubusercontent.com/encointer/book/master/src/protocol-reputation.md)
- User roles: Newbies are not guaranteed a slot and are capped per meetup because "our threat model assumes an honest majority". Endorsement by bootstrappers guarantees assignment. Reputables are guaranteed while they attend within the lifetime. No-show "spends" reputation and drops the user back to newbie. Bootstrappers have "immortal" status, are always guaranteed, and may endorse newbies they "have met physically". — same source.
- Threat-model doc rules: a meetup has ≥3 and ≤12 assigned participants. Only 1/4 may have zero reputation, which "directly impacts the maximum adoption rate. A population of 1M will take at least log(1M/12)/log(4/3) ≤ 40 ceremonies". To get income, a participant's claim must be signed by ≥1/3 of registered participants. The model assumes an honest majority of reputable participants. It names attacks: flooding with no-show newbies, "evil.corp" sharing key pairs to undermine randomisation, plus aggression, detention and abduction at publicly known locations. — [Encointer Book, threat model (raw)](https://raw.githubusercontent.com/encointer/book/master/src/protocol-threat-model.md)
- Code check (2026-11-28): the encointer-node runtime sets `MeetupSizeTarget = 15`, `MeetupMinSize = 3`, `MeetupNewbieLimitDivider = 2`, and the ceremonies pallet has `endorse_newcomer`, `attest_attendees` and `register_participant`. This is the solo/dev runtime. The Kusama production runtime lives in the Fellowship runtimes repo and was not checked. The doc's "≤12, 1/4 newbies" may be outdated. — [encointer-node runtime/src/lib.rs](https://raw.githubusercontent.com/encointer/encointer-node/master/runtime/src/lib.rs); [encointer/pallets ceremonies](https://raw.githubusercontent.com/encointer/pallets/master/ceremonies/src/lib.rs)
- The council (7 members of the Encointer Association) can set phase durations, register communities and locations, change nominal income and demurrage, set `max_speed_mps` and `min_solar_trip_time_s` (the physical-speed security parameters for location distance), set reputation lifetime and endorsement tickets, and purge a community, wiping balances and reputation. Protocol upgrades need Kusama governance. — [Encointer Book, governance (raw)](https://raw.githubusercontent.com/encointer/book/master/src/decentralization-governance.md); [Kusama parachain page (raw)](https://raw.githubusercontent.com/encointer/book/master/src/parachain-kusama.md)
- Reputation rings (anonymous PoP): each participant registers a Bandersnatch key. After each ceremony a ring of attendees is computed per (community, ceremony, level), where level N/5 means attended ≥N of the last 5. A ring-VRF signature yields a deterministic, context-specific pseudonym that prevents double claims and is unlinkable across contexts. k-anonymity equals ring size, with MaxRingSize 255 and sub-rings of 128–255. "Small communities have weak anonymity." — [Encointer Book, reputation rings (raw)](https://raw.githubusercontent.com/encointer/book/master/src/protocol-reputation-rings.md)
- Ford 2020 on Encointer: synchronised randomised groups mean that a person at one site cannot be at another. Larger groups exponentially reduce the attacker's advantage from fully controlled groups, but residual risk remains. Attendees might be socially engineered into signing more than one pseudonym per person. The trust root is the Swiss association. — [arXiv 2011.02412](https://arxiv.org/pdf/2011.02412)
- Original paper: Brenzikofer, "Encointer – Local Community Cryptocurrencies with Universal Basic Income" (arXiv 1912.12141). Letting participants choose locations would allow collusion, hence random assignment. — [arXiv 1912.12141](https://arxiv.org/pdf/1912.12141)
- ETH Zurich master's thesis (Guicciardi, 2022) on Encointer scalability states that "it is impossible to formally prove its security". — [ETH Research Collection](https://research-collection.ethz.ch/handle/20.500.11850/525222); [PDF](https://pub.tik.ee.ethz.ch/students/2021-HS/MA-2021-42.pdf)

**Status and usage (self-claims and secondary):**
- Encointer page (Jan 2025): Leu (Zurich), the first community, "now has low traction but is kept alive by a bunch of supporters". Community income was earlier raised from 22 to 44 LEU per ceremony (pegged to CHF), and attendance grew. — [Encointer: Our Communities](https://encointer.org/our-communities/); [Charting Leu](https://encointer.org/charting-the-development-of-encointers-longest-established-community-currency/)
- The Leu site claims "over 200 active members" (undated, self-claim). — [leu.zuerich](https://leu.zuerich/)
- "Most active communities are located in Africa". The Nyota (NYT) community in Dar es Salaam is the 4th community there, and Nyota documentary videos were posted 7 May 2026. — [Encointer communities](https://encointer.org/our-communities/); [A Star for Dar es Salaam](https://encointer.org/a-star-for-dar-es-salaam/)
- Polkadot Forum post (about mid-2025, via search excerpt): the Encointer Association decided to stop subsidising its communities. It keeps running the network, but communities lack funding or a reserve. Donation route: KSM or USDC to communities directly. Meetups are every 10 days. — [Polkadot Forum #17685](https://forum.polkadot.network/t/donate-ksm-or-usdc-p-to-encointer-communities-directly-no-intermediaries-no-custody/17685) (not fetched directly; excerpt only)
- Encointer runs as a Kusama common-good parachain whose slot is granted by governance. — [Encointer Book](https://raw.githubusercontent.com/encointer/book/master/src/parachain-kusama.md). parachains.info shows an "end date 7 Jan 2026" for the lease, which is likely irrelevant for a system chain and possibly stale. — [parachains.info](https://parachains.info/details/encointer)
- Related to Polkadot "Individuality": the People chain shipped people, members and coinage pallets in Fellowship runtime 2.5. DIM2 was "the next release" as of 15 Sep 2026. Full people are recognised via decentralised individuality mechanisms (DIMs), and light people via a governance-approved attester. I found no confirmation that Encointer is a DIM. — [Polkadot Forum, Fellowship OpenDev call 15 Sep 2026](https://forum.polkadot.network/t/polkadot-technical-fellowship-opendev-call-proceedings-15-september-2026/18684)

**Repositories (checked first-hand 2026-11-28):**
- encointer/pallets: GPL-3.0. Last commit 2026-11-23 ("bump to polkadot-sdk stable2609-2"). 40 commits since 2025-11-28, 4 since 2026-06-01. Recent fixes: "fix unbound purges" (2026-10-01), "fix unbound iteration > pop-bound" (2026-09-23). — [github.com/encointer/pallets](https://github.com/encointer/pallets)
- encointer/encointer-node: Apache-2.0. Last commit 2026-02-22, tag 1.21.3. Feb 2026 "support unlinkable pop contexts (#426)". — [github.com/encointer/encointer-node](https://github.com/encointer/encointer-node)
- encointer/book (docs): last commit 2026-04-08. — [github.com/encointer/book](https://github.com/encointer/book)

### Inferences
- (a) Double registration in the same ceremony: prevented by one assignment per account per ceremony. Two accounts of one person would be assigned to different (random) meetups at the same time, which is physically impossible unless the person colludes with others at both meetups (signing a ghost). The 1/3-signature rule plus random assignment makes this probabilistic, not absolute.
- (b) Restart after leaving: explicitly allowed. Reputation expires after 5 ceremonies and "start over with a new account" is the intended recovery path. So "one lifetime registration" is NOT guaranteed. Only "at most one *active* account per ceremony" is guaranteed.
- (c) Renting or selling: the account key can be handed to someone else who attends in person. Co-presence proves "a human attended", not "the same human as last time". Ring-VRF pseudonyms make purchased reputation hard to detect.
- (d) Collusion: bounded by random assignment and newbie caps. Small communities (few meetups) are much weaker, because a colluding group can make up most of a meetup. Bootstrappers are a trusted, immortal root.
- Decentralisation: medium-low. A 7-member Association council controls schedules, communities and purges. Protocol upgrades go through Kusama governance.
- Inclusion: requires a smartphone and physical attendance every ~10 days around noon. Strong exclusion of mobility-impaired, shift workers and rural people unless a location is near.
- Cost: low monetary cost (tiny KSM fee), high time cost.
- For an EU UBI: the open-source pallets (GPL-3.0) and ring-VRF reputation rings are reusable building blocks, but the protocol needs a lifetime-identity layer that it currently does not try to offer.

### Gaps
- No independent 2026 count of communities, attendees per ceremony or total reputables. The Encointer explorer and Subscan could not be fetched.
- The conclusions of Hoffmann's 2021 security report (profit figures) were not obtained.
- Kusama production-runtime values (meetup size, newbie divider, endorsement tickets) were not verified.
- GitHub stars and forks were not retrievable.

## 3. Idena

### Takeaway
Idena (mainnet Aug 2019) uses synchronised global FLIP ceremonies (a reverse-Turing, AI-hard puzzle under a strict time limit) plus invites. Empirically it was captured by "puppeteers": Ohlhaver, Nikulin & Berman found that by May 2022, 23 entities (<0.6% of distinct entities) controlled ≥~40% of accounts and ~48% of rewards. Idena responded by moving to identity staking with quadratic or sublinear reward weighting (IIP-5, July 2022), which moves it toward proof of stake. In 2026 the network is in maintenance mode: idena-go got 2 commits in Dec 2025 after none since mid-2023, and Human Passport retired its Idena stamp in Jan 2026.

### Cited Findings
- "Compressed to 0: The Silent Strings of Proof of Personhood", Ohlhaver, Nikulin, Berman. SSRN 2024, Harvard Ash Center PDF (June 2024), Stanford Journal of Blockchain Law & Policy (Jan 2025). It covers Idena from the Aug 2019 launch to the May 2022 crisis. — [SSRN 4749892](https://papers.ssrn.com/sol3/papers.cfm?abstract_id=4749892); [Harvard Ash PDF](https://ash.harvard.edu/wp-content/uploads/2024/06/proof-of-personhood_ohlhaver.pdf); [Stanford JBLP](https://stanford-jblp.pubpub.org/pub/compressed-to-0-proof-personhood)
  - 23 entities (<0.6% of distinct entities) controlled at least ~40% of accounts and ~48% of rewards (May 2022). The top 31 pools reduce to 23 entities once financial ties are merged. Three entities held ~19% of accounts and ~24% of rewards. Inference rests on evidence of third-party key access, which gives pool operators total control.
  - Solo accounts fell from 62% to 27% (June 2021 to May 2022). Large pools were 61% of accounts and took 70% of rewards.
  - Identity staking had already discouraged account *sales*, so "puppeteering" (buying participants' time and keys) became the dominant attack. The authors conclude an off-chain oligopoly absorbed the on-chain egalitarian system.
  - Caveat: authors' own inference. The ~40% figure depends on their definition of control. Search excerpts note it is "contested" under stricter definitions.
- IIP-5 "Mining rewards based on Quadratic staking" (final, July 2022). Motivation: "with the growth of the network, the number of nodes does not increase, but falls", attributed to pools concentrating mining rewards. — [IIP-5](https://docs.idena.io/docs/iip/iip-5); [GitHub discussion #81](https://github.com/idena-network/idena-docs/discussions/81); [Medium: Quadratic Staking — Idena's Ideological Turning Point](https://medium.com/idena/quadratic-staking-idenas-ideological-turning-point-f4cfcf0746f4)
- FAQ: if your identity is "killed" you lose your stake (20% of all rewards are auto-locked). Validation must be renewed every epoch. A missed validation leads to "Zombie" status. Invitees can join only the next validation. — [Idena FAQ](https://www.idena.io/faq)
- Strict global time limit: one person cannot solve more than one FLIP set at once. Weakness: reverse Turing tests stop bots but not humans who pass multiple times or are hired. — [Siddarth et al. 2020, arXiv 2008.05300](https://arxiv.org/abs/2008.05300)
- Human Passport retired the Idena stamp in its January 2026 reweight. — [Human Passport support: Idena Stamp (Retired)](https://support.passport.human.tech/stamps/how-to-verify/idena-stamp)
- Validation numbers seen in the Idena Telegram announcements (undated in excerpts): 8,136 and 7,885 identities validated in older ceremonies, and 112–115 validated per session in what appears to be a recent period. These undated self-reports cannot be put on a timeline. — [Idena Announcements (Telegram)](https://t.me/s/IdenaAnnouncements)
- Repo check (2026-11-28): idena-network/idena-go is LGPL-3.0. Last commit 2025-12-22 ("Ignore invalid contract transactions to prevent blockchain stalls", tag v1.1.2). The previous commit was 2023-08-04. idena-desktop's last commit was 2023-08-04, and idena-docs' was 2024-09-04. — [github.com/idena-network/idena-go](https://github.com/idena-network/idena-go)

### Inferences
- (a) Simultaneous double registration: mitigated by the global synchronous ceremony, but a hired human or a pool operator running many keys with many hired solvers defeats it at scale.
- (b) Restart: possible. A killed or lapsed identity can be replaced by a new invite. The only cost is the lost stake.
- (c) Renting: empirically the dominant failure (puppeteering, about 40% of accounts).
- (d) Collusion: pools are collusion. Online-only ceremonies with no physical co-presence let a single operator scale.
- Lesson for UBI: any online-only PoP with transferable keys and monetary reward drifts toward rent extraction. Moving to staking abandons one-person-one-share.

### Gaps
- No reliable dated 2025–2026 count of validated identities. The Idena explorer and API were unreachable.

## 4. Duniter / Ğ1 web of trust

### Takeaway
Ğ1 (live since 2017) is the longest-running non-biometric, state-free UBI currency. Membership needs 5 certifications from members plus a distance rule: 80% of "referee" members must be reachable within 5 steps. Membership runs 1 year and certifications 2 years. Duniter v2 (Rust, Polkadot-SDK/Substrate) went live for Ğ1 on 7–8 March 2026, migrating balances, members and certifications. Third-party sources give about 6,000+ members. Isenmann (arXiv 2512.07897, Dec 2025) models Sybil attacks and argues that graph rules alone block *large* attacks on the real Ğ1 graph. However, motivated attackers can maintain Sybils by renewing certifications, and slow certification rates throttle growth. Code is AGPL-3.0 and very active (128 commits in the last 12 months).

### Cited Findings
- Isenmann, "Analysis of the Sybil defense of Duniter-based cryptocurrencies", arXiv 2512.07897 (physics.soc-ph), submitted 5 Dec 2025. It studies the maximum Sybil-attack size against a simplified Duniter WoT model, as a function of attacker count and parameters. Headline: the system cannot stop every huge attack in general, but "can block attacks of large size in a real Duniter currency using graph-theoretic rules alone". Ğ1 parameters are modelled as d=5, δ=5, Δ=100, α=0.8. The basic attack creates as many fake identities as possible. Tests ran on the real Ğ1 WoT of size 2,590 (April 2020) with 100,000 random sets of 5 attackers. — [arXiv 2512.07897 (abs)](https://arxiv.org/abs/2512.07897); [HTML](https://arxiv.org/html/2512.07897) (not fetched directly; abstract and excerpts only)
  - Note: Isenmann's homepage lists the paper as FRCSS 2021, which may mean an earlier conference version. — [lucas-isenmann.github.io](https://lucas-isenmann.github.io/)
- Exact Duniter v2 parameters (first-hand, `runtime/shared/parameters.rs`, duniter-v2s master, 2026-11-28):
  - `WotMinCertForMembership = 5` (sigQty), `WotMinCertForCreateIdtyRight = 5`, `MinReceivedCertToBeAbleToIssueCert = 5`
  - `MaxByIssuer = 100` (sigStock), `CertPeriod = 5 days` (sigPeriod), `ValidityPeriod = 2 years` (sigValidity)
  - `MembershipPeriod = 1 year` (msValidity), `MembershipRenewalPeriod = 2 months` (earliest renewal interval)
  - Distance rule: `MaxRefereeDistance = 5` (stepMax), `MinAccessibleReferees = 80%` (xpercent), `EvaluationPeriod`, `DistanceRetryPeriod = 2 days`. In v2 the distance computation is done by an off-chain distance oracle (docs/user/distance.md).
  - Identity: `ConfirmPeriod = 2 months`, `IdtyCreationPeriod = 5 days`, `ChangeOwnerKeyPeriod = 6 months`, `AutorevocationPeriod = 1 year`, `DeletionPeriod = 10 years`
  - UBI: `UdCreationPeriod = 1 day`, `UdReevalPeriod = 1/2 year`, `SquareMoneyGrowthRate = Perbill 2_382_304` (c² ≈ 0.00238 per half-year, i.e. c≈4.88%/half-year). The v2 genesis sets `ud: 1148` (= 11.48 Ğ1/day).
  - Smith (validator) sub-WoT: `SmithWotMinCertForMembership = 3`, `SmithMaxByIssuer = 12`, 3-month inactivity limit. The genesis lists 15 smiths and an 8-member technical committee.
  — [duniter-v2s parameters.rs](https://github.com/duniter/duniter-v2s/blob/master/runtime/shared/parameters.rs); [resources/g1.yaml](https://github.com/duniter/duniter-v2s/blob/master/resources/g1.yaml)
- Duniter's wiki gives stepMax = 5 and xpercent = 80% for Ğ1, and the WoT size is capped by a formula tied to stepMax. — [Duniter: Introducing the web of trust](https://duniter.org/wiki/web-of-trust/introducing-web-of-trust/); [Deep dive](https://duniter.org/en/deep-dive-into-the-web-of-trust)
- v2 migration: the final network launched 7 March 2026. The last v1 block (#913,638) was imported into v2 genesis. The v2 is a full Rust rewrite on Polkadot-SDK. A wiki commit says production on 8 March 2026. — [Duniter blog: Duniter v2](https://duniter.org/blog/duniter-v2/); [git.duniter.org commit](https://git.duniter.org/websites/duniter-website-en-v2/-/commit/b5f28100ba10939df4666615568a8f960422b52b); [Forum: La Ğ1v2 est là](https://forum.duniter.org/t/la-g1v2-est-la/13647)
- Size: "6,000+ members" (third-party rating site Money2069, not independent counting). The older Duniter-blog figure of ~4,971 is outdated. — [Money2069 Ğ1](https://money2069.org/projects/g1-duniter); [Duniter blog](https://duniter.org/blog/duniter-substrate/)
- A v2.3.0 security update for Ğ1 and ĞTest was published 22 Sep 2026. — [Duniter forum v2s category](https://forum.duniter.org/c/dev/duniter-v2s/57)
- Ford 2020: although certifications expire, "an attacker with some motivation can still maintain many Sybil identities while renewing each Sybil's certifications often enough". — [arXiv 2011.02412](https://arxiv.org/pdf/2011.02412)
- Siddarth et al. 2020: Duniter's slow certification rate "exhibits good Sybil-protection properties but significantly restricts the network growth". Joining requires 5 vouches. — [Frontiers in Blockchain 2020](https://www.frontiersin.org/journals/blockchain/articles/10.3389/fbloc.2020.590171/full)
- Repo check (2026-11-28): duniter/duniter-v2s (GitHub mirror of git.duniter.org) is AGPL-3.0. Last commit 2026-11-14 (ParityDB deadlock fix, indexer handoff). 128 commits since 2025-11-28, 58 since 2026-06-01. Latest tags gtest-1000-2.3.3 and 2.3.2. — [github.com/duniter/duniter-v2s](https://github.com/duniter/duniter-v2s)

### Inferences
- (a) Simultaneous double registration: not prevented by co-presence. It is prevented socially, since certifiers promise to know the person (the Ğ1 licence requires knowing the person and that they have no other account). A person with two friend circles can get two identities. The licence is a norm, not a mechanism.
- (b) Restart: the old identity is revoked after membership lapse (1 year) and deleted after 10 years. A new identity needs 5 fresh certifications. This is socially detectable but not cryptographically prevented.
- (c) Renting: keys are transferable, and the UBI (11.48 Ğ1/day) flows to whoever holds the key.
- (d) Collusion: 5 colluding members (each able to issue 100 certs at 1 per 5 days) can grow a Sybil region, limited by the distance rule (80% of referees within 5 hops) and the issuance rate. Isenmann shows the bound is meaningful on the real graph.
- Decentralisation: high (no foundation root; smith WoT for validators). Anonymity: low (public pseudonymous graph). Inclusion: depends on knowing 5 members, which is hard outside France or Belgium.
- For an EU UBI: the best-documented, longest-running, open (AGPL) WoT-UBI codebase on Substrate. It is a natural reference for parameters. It is small (~6k), slow-growing, and has no binding "one human" proof beyond social norms.

### Gaps
- The full numeric results of Isenmann (maximum Sybil sizes for various attacker counts) were not obtained because arXiv was blocked.
- No independent 2026 member count. The Duniter indexer and explorer were not reachable.
- No documented large-scale Sybil incident on Ğ1 was found (absence of evidence, not evidence of absence).

## 5. BrightID, Circles, Proof of Humanity, Human Passport

### Takeaway
- **BrightID**: social-graph uniqueness (SybilRank, GroupSybilRank, then the "Aura" expert-rating layer). Development has largely stalled: BrightID-Node last committed 2024-03 and the app 2025-10. Aura stayed in beta. The project self-claims 100k+ verified users (Jul 2025).
- **Circles**: v2 launched May 2025 on Gnosis Chain. Each human mints 1 CRC/hour (24/day) with demurrage, and trust edges define which personal currencies are accepted. Sybil defence is only the trust graph plus a 96-CRC invitation cost. Giza (Sept 2023) found ~120k accounts but only ~2.5k active in v1.
- **Proof of Humanity**: registration with photo and video, vouching by registered humans, a deposit, and challenges judged by Kleros court. v1 had ~19k registrations (2021). v2 runs on Gnosis and Ethereum with cross-chain IDs, and its web UI is very actively developed in 2026.
- **Human Passport** (formerly Gitcoin Passport, acquired by Holonym/human.tech): score aggregation of "stamps" plus an ML "Models API" over on-chain activity. It is useful for airdrop filtering but is probabilistic and not a uniqueness guarantee. Its license is proprietary-style.

### Cited Findings
**BrightID**
- Aura runs weighted SybilRank over an inner graph of Aura participants (honesty ratings × "energy"). It had an initial beta and was "undergoing major changes based on the feedback". — [BrightID forum: Aura](https://forum.brightid.org/t/aura-a-new-verification-for-brightid/393); [Paragraph @adamstallard](https://paragraph.com/@adamstallard/i67wuYjmb5w2GW8yduaO)
- BrightID uses GroupSybilRank, a modification of SybilRank based on group similarity. — [Humanode blog](https://blog.humanode.io/revolutionizing-identity-verification-an-introduction-to-proof-of-personhood-pop-protocols/)
- The gitbook (~2 years old) lists "Meets" and "Bitu" as live and Aura under development. — [BrightID docs: Verifications](https://brightid.gitbook.io/brightid/verifications)
- An Octant proposal (2024) aimed to grow Aura-verified people from 1,000 to 1,000,000 and planned Aura beta 2 for Q1 2024. — [Octant discuss](https://discuss.octant.app/t/brightid-decentralized-uniqueness-protocol/79/1); [Octant newsletter](https://octant.substack.com/p/introducing-brightid)
- KAIST student report (2025): "as of July 2025, over 100,000 verified unique users and 15 dApps". This is secondary and repeats self-claims. — [Web3@KAIST 2025](https://web3classdao.github.io/kaist2025/reports/brightid/)
- Gitcoin governance discussion of lessons from Aura. — [Gitcoin gov](https://gov.gitcoin.co/t/what-can-we-learn-from-brightids-aura-sybil-defense-software/11159)
- Repo check (2026-11-28): BrightID/BrightID-Node is ISC-licensed, last commit 2024-03-18, 0 commits in the last 12 months. BrightID/BrightID (app) is ISC, last commit 2025-10-20 (v5.0.2), 0 commits since 2025-11-28. — [github.com/BrightID/BrightID-Node](https://github.com/BrightID/BrightID-Node); [github.com/BrightID/BrightID](https://github.com/BrightID/BrightID)

**Circles**
- v2 launched 21 May 2025 on Gnosis Chain. Third-party rating site estimates ~10,000 active v2 users. — [Money2069 Circles](https://money2069.org/projects/circles)
- Each account may mint 1 CRC/hour. An invitation costs the inviter 96 CRC, of which the invitee gets 48 CRC welcome bonus. Groups mint a shared CRC backed by members' coins. — [Circles docs: invitations](https://docs.aboutcircles.com/circles-sdk/invitations-and-referrals); [Circles docs](https://docs.aboutcircles.com/); [Circles forum: Contract v2](https://forum.aboutcircles.com/t/circles-contract-v2/562/3)
- Whitepaper: trust is directed and weighted, coins pass along transitive trust paths, and "trust can also always be revoked if an account turns out to be a Sybil". — [CirclesUBI whitepaper](https://github.com/CirclesUBI/whitepaper)
- Giza × Circles "Circles Network Data Analysis Report" (Sept 2023): unsupervised ML on an unfiltered database of ~120,000 accounts and a filtered set of ~2,500 active users. It used small labelled sets of known fake accounts from the Circles team. Its goal was algorithmic trust scoring against Sybils. — [Giza report](https://gizatech.xyz/collection/circles-network-data-analysis-report)
- Frontiers (2024) case study (Berlin pilot): ~200,000 accounts total. The web-of-trust and pathfinder mechanism "is very complex and it does not work in practice" (authors' assessment). — [Frontiers in Blockchain 2024, 10.3389/fbloc.2024.1362939](https://www.frontiersin.org/journals/blockchain/articles/10.3389/fbloc.2024.1362939/full)
- Impact study of a Circles UBI pilot (2025). — [arXiv 2504.02714](https://arxiv.org/html/2504.02714v1)
- The trust graph is public on-chain, which is a privacy concern. — [community-circles proposal](https://github.com/benjaminbollen/community-circles)
- Repo check (2026-11-28): aboutcircles/circles-contracts-v2 is AGPL-3.0, last commit 2026-03-05, 2 commits in 12 months (tag rc-v1.0.0-beta). aboutcircles/circles-sdk has its last commit 2026-03-05 (v0.29.2), and no license file was found at repo root. Legacy CirclesUBI/circles-contracts' last commit was 2023-07-03. — [github.com/aboutcircles/circles-contracts-v2](https://github.com/aboutcircles/circles-contracts-v2); [circles-sdk](https://github.com/aboutcircles/circles-sdk)

**Proof of Humanity (Kleros)**
- Registration: name, photo and short video. A registered human must vouch, and at least 1 vouch is needed. One vouch counts for only one pending profile at a time. The full deposit is required, then the profile goes "In Review". An unchallenged profile gets the deposit back. A challenge goes to Kleros court and loses the deposit if it fails. — [Kleros docs: PoH](https://docs.kleros.io/products/proof-of-humanity); [PoH 2.0 tutorial](https://kleros.gitbook.io/docs/products/proof-of-humanity/proof-of-humanity-tutorial)
- Voucher penalty: if a submission is rejected for Sybil or identity theft, all who vouched are removed from the registry and lose their vouching deposit. This does not apply to ordinary removals. — [PoH FAQ](https://proof-of-humanity.gitbook.io/docs/proof-of-humanity-faq); [Kleros PoH FAQ](https://kleros.gitbook.io/docs/products/proof-of-humanity/poh-faq)
- HIP-55 "Explicit Sybil Resistance": proposed treating Sybil challenges as duplicates so vouchers are removed. It noted that legacy profiles registered before the "sybil epoch" could not be cleanly removed, and that Sybils can withdraw accrued UBI. — [PoH governance HIP-55](https://gov.proofofhumanity.id/t/phase-3-hip-55-explicit-sybil-resistance/2424)
- Graph-analysis proposal: Sybil rings appear as small tight clusters at the periphery of the vouch graph, but "by the time such accounts are identified and removed, the damage may already be done". — [PoH gov: graph analysis](https://gov.proofofhumanity.id/t/using-graph-analysis-to-prevent-sybil-attacks/3103)
- v1 (2021) registered "almost 19,000 users" (self-claim). v2 deployed on Gnosis Chain to cut fees, with a single home chain per humanity and cross-chain state via the AMB bridge (up to 24 h delay). — [PoH gov: v2 update](https://gov.proofofhumanity.id/t/proof-of-humanity-v2-update/2062); [Kleros docs](https://docs.kleros.io/products/proof-of-humanity)
- 2026: a PNK airdrop for the first 10,000 humans (1,200 PNK, doubled if staked in the Humanity Court). Kleros dev updates were published Jan and Feb 2026. — [Kleros Dev Update Jan 2026](https://blog.kleros.io/kleros-development-update-january-2026/); [Feb 2026](https://blog.kleros.io/kleros-development-update-february-2026/)
- Kleros governance tension: forum thread "Kleros at a Crossroads: Governance, Execution and Accountability". — [Kleros forum](https://forum.kleros.io/t/kleros-at-a-crossroads-governance-execution-and-accountability/1477)
- Repo check (2026-11-28): Proof-Of-Humanity/proof-of-humanity-v2-web has its last commit 2026-09-15, 611 commits in 12 months and 248 since June 2026, so it is very active. proof-of-humanity-v2-contracts has its last commit 2026-05-07 (CrossChainProofOfHumanity integration docs), and its only tag is v1.0.0-preaudit1. No LICENSE file was found at repo root for either. — [v2-contracts](https://github.com/Proof-Of-Humanity/proof-of-humanity-v2-contracts); [v2-web](https://github.com/Proof-Of-Humanity/proof-of-humanity-v2-web)

**Human Passport (formerly Gitcoin Passport)**
- Stamps (linked accounts and credentials) form a composite "Unique Humanity Score". The Models API scores any EVM address with ML on on-chain activity (Aggregate, Arbitrum, Base, Optimism, Polygon models). A score of −1 means insufficient data. The vendor claims 96% human detection accuracy, not independently verified. — [Models API docs](https://docs.passport.human.tech/building-with-passport/models/introduction); [Holonym developer docs](https://developer.holonym.id/tech/human-passport)
- Used for model-based Sybil detection in Gitcoin GG23. — [Passport blog GG23](https://passport.human.tech/blog/human-passport-x-gitcoin-grants-defending-gg23-with-model-based-sybil-detection)
- Acquired by Holonym Foundation (Dec 2024 or Feb 2025; sources differ). Q1 2026: data oracle on Newton Protocol, "GuestList" stamp. Q2 2026: NFC passport verification via ZKPassport (i.e. state-ID-based, which is outside the "state-free" scope). — [Holonym Q1 2026](https://wallet.human.tech/blog/holonym-foundation-quarterly-update-q1-2026); [Holonym Q2 2026](https://wallet.human.tech/blog/holonym-foundation-quarterly-update-q2-2026)
- Repo check (2026-11-28): passportxyz/passport has its last commit 2026-11-04 (v1.0.104), 86 commits in 12 months. passportxyz/passport-scorer has its last commit 2026-11-25 (v0.0.110). The LICENSE file reads "Copyright (C) 2024 Passport XYZ Inc", i.e. not an OSI license per the file header (full text not reviewed). — [github.com/passportxyz/passport](https://github.com/passportxyz/passport); [passport-scorer](https://github.com/passportxyz/passport-scorer)

### Inferences
- BrightID: (a)/(b) a person can build a second identity with a second social circle. Social-graph scores catch only poorly connected Sybil clusters. (c) Rentable. (d) Collusion of a group of real humans co-verifying fakes is the canonical failure of graph methods (see §6). The project looks near-dormant.
- Circles: there is no uniqueness check at all beyond human trust decisions. Sybils mint 24 CRC/day each. Damage is contained only in the sense that Sybil CRC is not accepted by those who do not trust it. The Giza ratio of ~2% active accounts shows how much dead or Sybil mass accumulates. Circles is a currency-acceptance network, not a PoP.
- PoH: (a) duplicates are caught only if someone notices and challenges, and video lets humans compare faces (quasi-biometric by humans, an EU data-protection issue because faces and videos are published). (b) Re-registration after removal requires a new video, so it is detectable by face comparison only by humans. (c) Renting the registered address is possible. (d) Vouching rings are bounded by the voucher penalty and deposits. Its security depends on an active, paid challenger and juror economy.
- Human Passport: a probabilistic scoring service, centralised, with no lifetime uniqueness. Not suitable as a basis for UBI entitlement. It is suitable only as a supplementary signal.

### Gaps
- No current (2026) independent count of PoH v2 registered humans (Dune not reachable). No 2026 BrightID or Aura status statement found. Circles v2 numbers are third-party estimates only.
- Licenses of the PoH v2 repos and the circles-sdk could not be determined (no root LICENSE).
- No documented, sourced specific PoH Sybil-ring incident (numbers removed) was found. Only governance discussions.

## 6. Social-graph Sybil-defence research (SybilGuard, SybilLimit, SybilRank, SoK) and limitations

### Takeaway
All classic graph defences assume (1) a fast-mixing honest region and (2) few "attack edges" between honest and Sybil regions. Both assumptions are empirically weak: real social graphs are not fast mixing and have community structure, and in weak-trust networks attack edges are plentiful. At best these methods bound the number of Sybils per attack edge, roughly O(log n) for SybilGuard/SybilLimit-style schemes. They cannot guarantee one-human-one-account, and they fail against colluding real humans or bought edges.

### Cited Findings
- SybilGuard assumes a fast-mixing non-Sybil region, where after O(log n) hops a random walk's endpoint distribution reaches stationarity. SybilLimit shares this. Fast mixing and trust are the two common assumptions of all social-network Sybil defences. — [Viswanath et al., "An Analysis of Social Network-Based Sybil Defenses", SIGCOMM 2010](http://ccr.sigcomm.org/online/files/p363.pdf); [Wang et al. survey, arXiv 1312.6349](https://arxiv.org/html/1312.6349.pdf)
- Viswanath et al.: these schemes effectively detect *communities*. As modularity increases, performance "falls close to random". — [SIGCOMM CCR p363](http://ccr.sigcomm.org/online/files/p363.pdf); [MPI-SWS design-space paper](https://people.mpi-sws.org/~gummadi/papers/sybil_defense.pdf)
- Mohaisen et al.: measured mixing time "is much larger than that anticipated", i.e. social networks are generally not fast mixing (cited in the survey). — [Wang et al. arXiv 1312.6349](https://arxiv.org/html/1312.6349.pdf)
- RenRen (the largest Chinese OSN) violates the few-attack-edges assumption. In weak-trust networks, attack edges exceed what previous works assume. — [SybilFuse, arXiv 1803.06772](https://arxiv.org/pdf/1803.06772)
- Expander-like properties are a stronger, unvalidated assumption. — [Zhang et al., PMLR v45](http://proceedings.mlr.press/v45/Zhang15b.pdf)
- Alvisi, Clement, Epasto, Lattanzi, Panconesi, "SoK: The Evolution of Sybil Defense via Social Networks" (IEEE S&P 2013), as cited: most random-walk-based defences are vulnerable to a family of simple Sybil attacks. It proposes local community detection around trusted seeds instead. — [Communities, Random Walks, and Social Sybil Defense (ResearchGate)](https://www.researchgate.net/publication/265595277_Communities_Random_Walks_and_Social_Sybil_Defense)
- SybilRank (Cao et al., NSDI 2012) uses early-terminated power iteration from trusted seeds and ranks nodes by degree-normalised trust. Deployed at Tuenti. — [SybilRank paper (MIT course copy)](https://css.csail.mit.edu/6.566/2018/readings/sybilrank.pdf)
- Survey of graph-based fake-account detection (2025). — [arXiv 2507.06541](https://arxiv.org/pdf/2507.06541)

### Inferences
- For a lifetime "one human = one registration" guarantee, graph scoring can only be a supplementary signal (rank-and-review). It cannot be the primary gate. Its known failure modes are exactly (c) and (d): bought edges and colluding humans.
- Seed-based local methods (SybilRank, Alvisi's local community approach) fit "local group / neighbourhood" designs, since trust seeds can be local ceremony attendees.

### Gaps
- Primary texts of SybilGuard, SybilLimit and the Alvisi SoK were not fetched. Quantitative per-attack-edge bounds were taken from memory of the literature and are not cited here; the report should cite the primary papers.

## 7. Cross-cutting: security matrix, decentralisation, anonymity, inclusion, cost; research on combining ceremonies with dispute resolution or local groups

### Takeaway
No surveyed social or ceremony system guarantees lifetime uniqueness. Co-presence systems (pseudonym parties, Encointer) prevent *simultaneous* double participation, but by design they allow restart and cannot stop key rental. Web-of-trust systems (Ğ1, BrightID, Circles) rely on social norms and graph bounds. Challenge systems (PoH) rely on human face comparison and paid adjudicators. Online ceremonies (Idena) collapsed into puppeteer oligopoly. I found no published, implemented design that forces two suspected identities to appear simultaneously as a dispute mechanism. Kleros-style challenges plus Encointer-style synchronised physical meetups are the closest components, which suggests a research gap. New 2026 preprints exist but could not be verified beyond abstracts.

### Cited Findings
- Siddarth, Ivliev, Siri, Berman, "Who Watches the Watchmen? A Review of Subjective Approaches for Sybil-resistance in Proof of Personhood Protocols", Frontiers in Blockchain 2020, arXiv 2008.05300. It gives a formal PoP definition and a taxonomy (objective third-party claims vs subjective approaches) and reviews implementations. Conflict of interest: Ivliev is active in Idena, and Siri and Berman are linked to Democracy Earth. — [Frontiers](https://www.frontiersin.org/journals/blockchain/articles/10.3389/fbloc.2020.590171/full); [arXiv 2008.05300](https://arxiv.org/abs/2008.05300)
- The review notes that "some systems have recourse to adjudicate cases of faulty or duplicate users" (PoH/Kleros), and that reverse Turing tests fail against human-generated attacks. — [arXiv 2008.05300v5](https://arxiv.org/pdf/2008.05300v5)
- Ford 2020 evaluates inclusion, equality, security and privacy of pseudonym parties vs other PoPs (biometrics, social graphs, ID). — [arXiv 2011.02412](https://arxiv.org/abs/2011.02412)
- "Compressed to 0" argues that every PoP must address off-chain "puppeteering", whether through physical presence, coercion resistance or other means. — [Harvard Ash PDF](https://ash.harvard.edu/wp-content/uploads/2024/06/proof-of-personhood_ohlhaver.pdf)
- New 2026 preprints (titles and abstracts only, not reviewed):
  - "Clear Human Web: Where a Person Vouches for Every Client" (Porcelli, arXiv 2611.06286, 4 Nov 2026): a wallet holding ID data gets limited blind-signed tokens. It cites Idena puppeteering (23 entities, ~40%). It is ID-based, so outside the state-free scope. — [arXiv 2611.06286](https://arxiv.org/abs/2611.06286)
  - "Binding Humans to Keys: A Secure Decentralized Proof of Personhood Protocol" (Zaghdoudi, Bu, Hayek, Lafourcade, Potop-Butucaru; IACR ePrint 2026/2246, Sept 2026): S-GYID protocol against identity-theft and "ghost key" attacks by colluding authorities. It is biometric-based, so outside scope, but relevant for key-binding. — [ePrint 2026/2246](https://eprint.iacr.org/2026/2246)
  - "Beyond Identity Creation: Bilinear Sybil Cost in Identities and Time" (arXiv 2611.06268) and "Sustained Participation as a Security Resource: The Bounded Participation Channel" (arXiv 2609.35300). These surfaced in search; their titles suggest Sybil-cost models based on ongoing participation (relevant to periodic ceremonies), but the content was not verified. — [arXiv 2611.06268](https://arxiv.org/html/2611.06268v1); [arXiv 2609.35300](https://arxiv.org/pdf/2609.35300)
- Polkadot "Individuality" (People chain): distinguishes "full people" recognised by decentralised individuality mechanisms (DIMs) from "light people" recognised by a governance-approved attester, who get smaller benefits. Uses ring-based membership (members pallet on the People chain because ring ops are block-space heavy). DIM2 was not shipped as of Sep 2026. A tiered model like this matches the "local group plus escalation" idea. — [Polkadot Forum 15 Sep 2026](https://forum.polkadot.network/t/polkadot-technical-fellowship-opendev-call-proceedings-15-september-2026/18684); [proofofpersonhood.how](https://www.proofofpersonhood.how/)

### Inferences
Summary matrix (my synthesis from the cited mechanisms):

| System | (a) simultaneous double reg. | (b) fresh restart after leaving | (c) rent/sell | (d) collusion | Decentral. | Anonymity | Inclusion / cost |
|---|---|---|---|---|---|---|---|
| Pseudonym parties | strong (one body, synchronous) | n/a: per-period tokens; restart is the norm | weak (bearer tokens) | organiser fraud; federation needed | high (if federated) | high (unlinkable tokens) | must attend; time cost |
| Encointer | strong-probabilistic (random meetups, 1/3 sigs) | explicitly allowed (5-ceremony lifetime) | weak | bounded by randomness and newbie cap; weak in small communities | medium-low (Association council, bootstrappers) | good (ring-VRF, k≤255) | every 10 days, noon, smartphone |
| Idena | medium (global time limit) | allowed (new invite; lose stake) | failed empirically (~40% puppeteered) | pools = collusion | medium | pseudonymous | online synchronous session each epoch; no travel |
| Ğ1 / Duniter | social only (licence) | social only; new 5 certs | weak | bounded by distance rule and cert rate (Isenmann) | high | low (public graph) | need 5 local members; slow |
| BrightID / Aura | graph scoring | graph scoring | weak | canonical failure | medium (node operators) | low-medium | social connections; project near-dormant |
| Circles v2 | none (trust is local acceptance) | none | weak | trust graph only | high (contracts) | low (public trust graph) | invite cost 96 CRC |
| PoH v2 | human face comparison plus challenges | human face comparison | weak (address transferable) | voucher penalty plus deposits | medium (Kleros jurors) | very low (public face video, GDPR problem) | deposit plus video; crypto wallet |
| Human Passport | probabilistic ML or stamps | probabilistic | weak | n/a | low (company) | medium | easy; not a uniqueness guarantee |

- Design implications for an EU UBI ("one human for life, without state ID"):
  1. Co-presence is the only state-free primitive with a hard physical argument against (a).
  2. Lifetime continuity (b) needs a *persistent* link across ceremonies, such as linkable ring signatures or a stable VRF pseudonym per person, plus a social or adjudication layer to detect re-entry under a new key. No surveyed system does this well.
  3. Rental (c) is unsolved everywhere; Idena is the empirical warning. Mitigations are frequent co-presence (raising the time cost of renting), coercion-resistant credentials (Ford), and reward design that does not pay puppeteers (sublinearity broke one-person-one-share in Idena).
  4. A "challenge → simultaneous co-presence" dispute (two suspected duplicates must appear at the same time at different, randomly assigned places) combines Encointer's physical argument with Kleros-style challenges. I found no published implementation or formal analysis, so it is a research gap and novel territory.
- Reusable open code: Duniter-v2s (AGPL-3.0, Substrate WoT, distance oracle, UD), Encointer pallets (GPL-3.0, ceremonies, ring-VRF reputation rings), Circles v2 contracts (AGPL-3.0, Solidity, EVM), PoH v2 contracts (Solidity; license unclear). Licenses matter: the GPL and AGPL copyleft requirements would apply to a derivative chain.

### Gaps
- I found no peer-reviewed study of "forced simultaneous appearance" dispute resolution or formal "neighbourhood or local group" uniqueness designs beyond Encointer's community model and Polkadot's DIM tiers. A targeted search of bford.info and the DEDIS publications was not possible (host blocked).
- The full texts of the new 2026 preprints (arXiv 2611.06268, 2609.35300) were not read, so their relevance is unverified.
- GitHub star and fork counts for all repos could not be retrieved (GitHub API and HTML blocked by the session proxy). Only commit activity and licenses were verified.
- Hosts to allow for a follow-up pass: arxiv.org, forum.polkadot.network, book.encointer.org, encointer.org, duniter.org, forum.duniter.org, t.me, api.idena.io / scan.idena.io, gizatech.xyz, frontiersin.org, ash.harvard.edu, bford.info, dune.com, api.github.com.
