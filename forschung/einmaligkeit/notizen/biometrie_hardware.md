# Biometrischer Proof-of-Personhood: Systeme, Hardware, Software (Stand Ende Nov. 2026)

Note on method: this research ran in a cloud session where world.org, whitepaper.world.org and arxiv.org could NOT be fetched directly (DNS blocked by network allowlist). Facts from those domains come from search-result summaries, not full-text reads. Mark: [V] = vendor or self-reported claim, [I] = independent measurement or third-party reporting.

## 1. World / Worldcoin: Orb, open-iris, uniqueness at scale, World ID 4.0, problems

### Takeaway
World is the only production system doing iris deduplication for a global, non-state PoP. It reports about 18 million Orb verifications (self-reported, Apr 2026). Its uniqueness claims rest on its own measurements: a whitepaper FMR of about 1e-12 versus an engineering-blog upper bound of about 4.1e-8. The matcher library (open-iris) is MIT-licensed, but no independent audit of the numbers was found. Regulators in many countries have suspended or banned it, and verified accounts have been resold.

### Cited Findings
**Scale and versions**
- [V] World ID 4.0 was unveiled at the "Lift Off" event in San Francisco on 17 Apr 2026, with Tinder, Zoom and Docusign integrations. More than 18 million people have verified with an Orb in about 160 countries, up from about 12 million a year earlier — [implicator.ai](https://www.implicator.ai/worldcoin-opens-lift-off-event-in-san-francisco-with-world-id-protocol-reveal/); [Yahoo Finance](https://finance.yahoo.com/technology/ai/articles/sam-altman-backed-world-turns-174026719.html)
- [V/I] World App user counts are reported inconsistently: "over 30 million" vs. "nearly 39 million". Reported World ID proofs range from 450M to more than 475M — [Wikipedia: World (blockchain)](https://en.wikipedia.org/wiki/World_(blockchain)); [OKX analysis](https://www.okx.com/en-us/orbit/insight/eye-tracking-verification-has-been-done-for-18-million-people-but-the-identity-sector-still-hasnt-made-money-93894229131808)
- [I] Per an OKX analysis, the Orb-verified share of World App users fell from 48.1% to 46.2%, so app growth is outpacing Orb verification — [OKX](https://www.okx.com/en-us/orbit/insight/eye-tracking-verification-has-been-done-for-18-million-people-but-the-identity-sector-still-hasnt-made-money-93894229131808)
- [V] The Orb Mini was presented at the "At Last" event on 30 Apr 2025. The plan was 7,500 Orbs across the US by end of 2025, with Orb Mini availability expected in 2026. No unit price is published — [MobileIDWorld](https://mobileidworld.com/worldcoin-launches-orb-mini-biometric-verification-device-in-us-market/); [Bitget News](https://www.bitget.com/news/detail/12560604752572)
- World raised $52.5M for the human-ID push (2026) — [Catenaa](https://catenaa.com/industries/technology/world-id-funding-ai-trust/)

**open-iris (worldcoin/open-iris)**
- License: MIT. No GitHub releases are listed. The repo has about 436 commits, 426 stars and 111 forks (as of Nov 2026). The pipeline covers segmentation, normalization/feature extraction (iris code), matching, and a score. The input is a grayscale IR eye image, and NIR capture is assumed. There is an `ORB` install extra for on-Orb inference. The repo contains `IRIS_PERFORMANCE_CARD.md` and `SEMSEG_MODEL_CARD.md`, which were not read here — [GitHub worldcoin/open-iris](https://github.com/worldcoin/open-iris)
- open-iris does not include PAD/liveness, the Orb's multispectral sensors, or the Orb's camera hardware. The README names no camera specs or wavelength — [GitHub](https://github.com/worldcoin/open-iris)

**Accuracy claims (conflicting, all [V])**
- The whitepaper puts the real-world FMR at about 1×10⁻¹² ("1 in a trillion"). It also says global uniqueness ideally needs rates around 10⁻²⁰ — [World Whitepaper "Achieving Proof of Human"](https://whitepaper.world.org/achieving-proof-of-human) (via search summary; direct fetch blocked)
- The engineering blog says no misclassified impostor pairs were observed in its test, so it reports only an upper bound of FMR ≲ 4.1×10⁻⁸. It argues billion-scale dedup needs at least FMR ≈ 10⁻⁶ per comparison — [World engineering blog: Iris recognition inference system](https://world.org/blog/engineering/iris-recognition-inference-system) (via search summary)
- [I] No independent (NIST IREX or academic) measurement of World's matcher at its claimed operating point was found.

**Dedup architecture (SMPC/AMPC)**
- Iris codes are compared by Hamming distance inside an MPC system ("AMPC"). The system reveals only a binary match/no-match result, not the distances — [World Whitepaper](https://whitepaper.world.org/achieving-proof-of-human) (via search summary)
- Academic paper: "Scaling Private Iris Code Uniqueness Checks to Millions of Users" (arXiv 2405.04463, 2024). This is the GPU-based MPC protocol for iris-code membership checks behind World's dedup. Details such as throughput and party count could not be fetched — [arXiv 2405.04463](https://arxiv.org/pdf/2405.04463)
- [I] The BioBind paper notes that World ID uses the iris for uniqueness and the face only for re-authentication — [arXiv 2611.12636](https://arxiv.org/pdf/2611.12636)

**World ID 4.0 credential levels (official dev docs)**
- Orb ("Proof of Human") is the highest assurance level, based on hardware verification — [World Developer Docs: credentials](https://docs.world.org/world-id/idkit/credentials)
- Selfie Check is medium assurance. It uses the device camera for liveness and facial similarity, and no Orb or document is required. The legacy `selfieCheckLegacy` preset is to be migrated to `selfieCheck` in 4.0. Session proofs are recommended for repeated checks — [World Docs credentials](https://docs.world.org/world-id/idkit/credentials); [session proofs](https://docs.world.org/world-id/idkit/session-proofs)
- The Passport (NFC) credential is "Proof of Attributes", aimed at regulated, age-gating and KYC-adjacent flows. It has a legacy Document fallback. Face auth matches a live selfie to the passport photo or the Orb-captured image — [World Docs](https://docs.world.org/world-id/idkit/credentials); [IDKit integrate](https://docs.world.org/world-id/idkit/integrate)
- Selfie Check is face-only. It gives no strong global uniqueness (see section 3).

**Documented problems**
- Black market (2023): BlockBeats reported a market for World IDs, with credentials sold for as little as $30. World said it found "several hundred cases" of World ID fraud but no trading of iris scans. A re-scan at the Orb lets users reclaim an account — [The Block](https://www.theblock.co/post/231433/worldcoin-iris-black-market); [Gizmodo](https://gizmodo.com/worldcoin-black-market-iris-data-identity-orb-1850454037); [Biometric Update 05/2023](https://www.biometricupdate.com/202305/worldcoin-may-have-a-biometric-data-black-market-problem)
- People in Cambodia and Kenya were allegedly recruited to sign up and then sell their World ID and WLD. This is not independently confirmed — [The Conversation](https://theconversation.com/worldcoin-is-scanning-eyeballs-to-build-a-global-id-and-finance-system-governments-are-not-impressed-210980)
- Credentials of at least 7 Orb operators turned up on the dark web (2023) — [TechCrunch](https://techcrunch.com/2023/05/12/hackers-stole-passwords-of-worldcoin-orb-operators/)
- Regulatory actions:
  - Kenya suspended World in Aug 2023 and later ordered data deletion — [Wikipedia](https://en.wikipedia.org/wiki/World_(blockchain))
  - Spain's AEPD issued a precautionary ban in March 2024, upheld by the Audiencia Nacional — [Wikipedia](https://en.wikipedia.org/wiki/World_(blockchain))
  - Portugal suspended World in 2024 (no primary document found) — [Plisio](https://plisio.net/crypto/worldcoin)
  - Indonesia revoked permits in May 2025 — [Wikipedia](https://en.wikipedia.org/wiki/World_(blockchain))
  - The Philippines NPC issued a cease-and-desist order dated 23 Sep 2025: stop Orb scans, remove the app, stop data transfers. World announced an appeal — [BitPinas](https://bitpinas.com/regulation/worldcoin-cease-desist/); [BitPinas feature](https://bitpinas.com/feature/npc-world-issue/); [Rappler](https://www.rappler.com/technology/npc-stops-tools-humanity-world-dict-bank-use/)
  - Thailand's data regulator ordered a halt to iris scanning — [ForkLog](https://forklog.com/en/thai-data-regulator-orders-world-to-stop-iris-scanning-service/)
  - World pulled Orbs from 3 countries in Dec 2023 — [Biometric Update](https://www.biometricupdate.com/202312/worldcoin-pulls-orbs-from-3-countries-claims-early-success-from-shopify-integration)

### Inferences
- The gap between FMR 1e-12 (whitepaper) and ≤4.1e-8 (blog bound) means "1 in a trillion" is likely an extrapolation, not an observed value. A protocol should plan with the conservative figure.
- The account-sale problem cannot be solved by biometrics. It concerns transfer of a valid identity after enrollment, so it needs re-authentication (face/session proofs) and economic incentives.
- open-iris covers only the matcher. A decentralized clone still needs its own NIR capture, its own PAD, and its own MPC dedup (for example, based on arXiv 2405.04463).

### Gaps
- Orb hardware: the build cost per Orb, the exact Orb versions (Orb 1, Orb "2.0"/Nvidia Jetson, Orb Mini), and which hardware files are open source. A "worldcoin/orb-hardware"-style repo is known from background knowledge, but it was not verified here because world.org and its whitepaper were not fetchable.
- The content of IRIS_PERFORMANCE_CARD.md was not read, so open-iris FNMR/FMR figures and thresholds are not in these notes.
- Number of AMPC parties, operators (universities?), throughput, and the hardware for arXiv 2405.04463 (arXiv was blocked).
- 2026 status of the bans (Kenya court case, Indonesia, Germany's BayLDA order of Dec 2024 on deletion) was not checked.

## 2. Iris recognition science: Daugman, NIST IREX, open-source toolkits, hardware prices, smartphone/visible-light iris

### Takeaway
Daugman's UAE data (about 200 billion impostor comparisons) shows an impostor Hamming distribution with about 244–249 binomial degrees of freedom and no false match below HD ≈ 0.26. Iris is therefore the only modality with an empirically grounded case for 1:N dedup at a billion scale. NIR USB iris cameras (IriTech IriShield) are cheap, around $180–190 or ₹5,300–9,100. NIST IREX 10 shows FNIR of about 0.2–1% at FPIR 0.01 for top 2026 algorithms.

### Cited Findings
- [I] Daugman (2005, UCAM-CL-TR-635): 200,027,808,750 pairwise comparisons among 632,500 different irises (UAE). Raw HD between different IrisCodes ranged from 0.265 to 0.750. At reasonable thresholds, FMR < 1 in 200 billion — [Cambridge TR-635](https://www.cl.cam.ac.uk/techreports/UCAM-CL-TR-635.pdf)
- [I] The impostor HD distribution follows a binomial with about 249 degrees of freedom, with standard deviation ≈ 0.0317. Agreement of more than 2/3 of bits (HD < 0.33) between different eyes is "extremely improbable" — [Daugman binomdata](https://www.cl.cam.ac.uk/~jgd1000/binomdata.html); [How Iris Recognition Works](https://www.cl.cam.ac.uk/~jgd1000/irisrecog.pdf)
- [I] A conflicting figure: Rozmus cites 244 degrees of freedom for the same 200-billion dataset — [arXiv 2006.16107](https://arxiv.org/pdf/2006.16107.pdf). See also Daugman, "Searching for Doppelgangers" (IET) on 1:N scaling with threshold adjustment by gallery size — [Cambridge](https://www.cl.cam.ac.uk/~jgd1000/Searching_for_Doppelgangers_IET.pdf); [Quick Primer HD](https://www.cl.cam.ac.uk/~jgd1000/QuickPrimerHD.pdf)
- [I] A 2020 paper on proofs of personhood argues that at an FAR of 1:100,000 in a billion-scale database such as Aadhaar, a new legitimate enrollee would collide with about 10,000 others. Their point: the per-comparison FMR must scale with N — [arXiv 2011.02412 (Siddarth/Ford et al.)](https://arxiv.org/pdf/2011.02412)
- [I] Empirical capacity analysis of iris systems: "Empirical Assessment of End-to-End Iris Recognition System Capacity" — [arXiv 2303.12742](https://arxiv.org/pdf/2303.12742)
- [I] Pediatric permanence: "Nine Years of Pediatric Iris Recognition: Evidence for Biometric Permanence" — [arXiv 2512.15543](https://arxiv.org/pdf/2512.15543)
- [I] NIST IREX 10 (Identification Track, open continuously) ranks submissions by FNIR at FPIR = 0.01. EER is deprecated. Developer pages from 2024–2026 show FNIR values of 0.0020, 0.0051 (±0.0007), 0.0067 and 0.0102 — [NIST IREX10](https://pages.nist.gov/IREX10); example pages for [idbio_001 (2026-03-06)](https://pages.nist.gov/IREX10/idbio_001/2026_03_06/index.html), [tech5_009 (2026-06-05)](https://pages.nist.gov/IREX10/tech5_009/2026_06_05/index.html), [neurotechnology_020 (2026-05-05)](https://pages.nist.gov/IREX10/neurotechnology_020/2026_05_05/index.html). The values could not be tied to individual vendors from the snippets.
- [V/I] IDEMIA led IREX 10 in 2020 with FNIR 0.0051 — [Biometric Update 04/2020](https://www.biometricupdate.com/202004/idemia-iris-recognition-algorithm-tops-leaderboard-in-nist-biometric-accuracy-testing). Older reports: [NIST IR 8207 (IREX IX)](https://nvlpubs.nist.gov/nistpubs/ir/2018/NIST.IR.8207.pdf); [NIST IR 7836](https://nvlpubs.nist.gov/nistpubs/ir/2012/NIST.IR.7836.pdf)
- Hardware prices for the IriTech IriShield USB MK 2120U (single-eye NIR, Aadhaar-certified):
  - $190 at Reach.dog/Fulcrum — [Reach.dog](https://reach.dog/shop/fulcrum-biometrics-inc/products/irishield-usb-mk-2120u-iris-scanner)
  - $180 for the MO 2120 module (OEM) — [Fulcrum store](https://store.fulcrumbiometrics.com/products/irishield-usb-mo-2120-module-1)
  - ₹5,300 on IndiaMART — [IndiaMART](https://m.indiamart.com/multipleitsolution-newdelhi/iris-scanner.html)
  - ₹6,550 net / ₹7,729 incl. GST (MRP ₹10,999) on Moglix — [Moglix](https://www.moglix.com/iritech-irishield-mk2120u-single-black-iris-scanner/mp/msn457mqg8nn9j)
  - Further government purchase listings on GeM — [GeM](https://org.gem.gov.in/cart/cart_global/directBuy/728057)
  - A long-range BO 2121 module also exists — [Fulcrum](https://store.fulcrumbiometrics.com/collections/all/products/irishield-usb-bo-2121-long-range-module)
  - Neurotechnology sells the IriShield with its SDK — [Neurotechnology](https://neurotechnology.com/eye-iris-scanner-iritech-irishield.html)
- [I] Statistical modelling of iris-template bits (Ising model) — [arXiv 1802.02223](https://arxiv.org/pdf/1802.02223)

### Inferences
- Rule of thumb from Daugman: for N enrolled people, the per-comparison FMR must be well below 1/N to keep false dedup collisions (legitimate people rejected as duplicates) rare. At N = 1e9 that means FMR ≤ 1e-10 to 1e-12, achievable only by lowering the HD threshold (for example below 0.26–0.30) at the cost of a higher FNMR (more missed duplicates).
- Single-eye USB scanners (about $200) are cheap enough for a decentralized enrollment network. Binocular capture (both eyes) improves the false match/miss trade-off but needs dual-iris devices or two captures.

### Gaps
- Open-source iris toolkits OSIRIS (Biosecure/Télécom SudParis), USIT (Univ. Salzburg, Wavelab) and the Notre Dame/Czajka tools (e.g. HDBIF, TripletIris): license, maturity and accuracy were not checked in this session (no tool calls left).
- Binocular scanners (IriTech IriShield BK 2121U, CMITech EF-45, etc.) and their prices were not checked.
- Smartphone iris (Samsung Galaxy S8/S9/Note, discontinued 2019 with the S10) and visible-light iris feasibility were not looked up. From background knowledge only, unverified: visible-light iris works poorly on dark (melanin-rich) irises, which is why NIR at roughly 700–900 nm is standard.

## 3. Face deduplication at scale (NIST FRTE 1:N, BioBind)

### Takeaway
At gallery sizes of 12 million, the best face algorithms reach FNIR of about 0.05–1% at FPIR 0.003 on cooperative mugshots. Face cannot carry lifelong person-level uniqueness, though: twins and look-alikes exist, appearance can be changed deliberately (BioBind: 1.8% evasion on Bosphorus), and aging adds drift. Face is suitable for re-authentication, not as the sole dedup factor.

### Cited Findings
- [I] NIST FRTE 1:N reports FNIR at a threshold with FPIR = 0.003. Updates in Aug 2026 and Sep 2026 added 7 new and 8 returning developers, including a new SenseTime entry — [Biometric Update 08/2026](https://www.biometricupdate.com/202608/nist-frte-1n-shows-face-recognition-race-shifting-beyond-accuracy); [Biometric Update 09/2026](https://www.biometricupdate.com/202609/new-nist-frte-1n-results-illustrate-facial-recognition-accuracys-multidimensionality)
- [V] IDEMIA Public Security (Aug 2026) claims first place across all mugshot identification categories, including the 12-million-identity gallery. It reports FNIR 0.05% at 1.6M and 12M, first across gallery sizes from 640K to 12M, at FPIR 0.1% (sources differ on the threshold) — [IDEMIA press](https://na.idemia.com/2026/08/13/idemia-public-security-reinforces-leadership-in-trusted-biometrics-by-achieving-top-results-in-nists-latest-face-recognition-benchmark/); [Biometric Update](https://www.biometricupdate.com/202608/idemia-ps-face-biometrics-score-high-accuracy-at-scale-in-latest-nist-frte)
- [V] NEC claims first place in the FRTE 1:N report of 13 Feb 2026 — [NEC press 09.03.2026](https://www.nec.com/en/press/202603/global_20260309_02.html)
- [V] Youverse reports a 98.97% identification rate on the 12M mugshot gallery, i.e. FNIR ≈ 1.03% — [Biometric Update 08/2026](https://www.biometricupdate.com/202608/youverse-highlights-population-scale-facial-recognition-accuracy-in-nist-evaluation)
- [I] BioBind (arXiv 2611.12636, Wang, Wu, Zhang, submitted 9 Nov 2026, v1 preprint, not peer-reviewed) binds the dedup query, gallery insertion and credential commitment to one captured biometric vector. The face prototype uses LFW features, with about 31,445 constraints and about 305k gas for the authentication circuit. On the Bosphorus database, cooperative subjects who changed their appearance evaded dedup in 1.8% of re-enrollment attempts at the operating threshold. The enrollment attempt limit is treated as a deployment parameter, because each missed duplicate is a chance at a second credential — [arXiv abs](https://arxiv.org/abs/2611.12636); [HTML](https://arxiv.org/html/2611.12636); [PDF](https://arxiv.org/pdf/2611.12636)

### Inferences
- The FRTE figures are FNIR (missed match of a known person) for cooperative, high-quality mugshots. For Sybil defense the decisive figure is the adversarial miss rate (deliberate appearance change, makeup, prosthetics), and BioBind's 1.8% is an order of magnitude worse. Repeated attempts compound it: if attempts were independent, 10 attempts would give about 17% evasion.
- At FPIR 0.003 and N = 1e8, legitimate users would routinely be flagged falsely as duplicates, so face-only dedup at a billion scale needs manual review or a second factor.

### Gaps
- No NIST FRTE primary data (FNIR tables) was read, only press and trade reports.
- Twin and look-alike rates in FRTE (NIST twins report) were not looked up.

## 4. Palm/vein: Humanity Protocol, Amazon One, Fujitsu PalmSecure, finger vein, multimodal

### Takeaway
For palm/vein there is no independent NIST-style 1:N benchmark. All accuracy figures (PalmSecure FAR 0.00008%) are vendor claims, mostly for 1:1 verification. Humanity Protocol's palm-vein and on-device-ZK claims have not been independently checked, and in Feb 2026 it pivoted away from "proof of personhood". Amazon One was shut down for retail on 3 Jun 2026 due to low adoption.

### Cited Findings
- [V] Fujitsu PalmSecure datasheet: FAR 0.00008% (8e-7), FRR 0.01% — [Fujitsu datasheet](https://www.fujitsu.com/global/Images/PalmSecure_Datasheet.pdf). A different third-party figure for PalmSecure F-Pro: FAR < 0.00001%, FRR 1.0%. That source states "No NIST-equivalent benchmark exists; treat as manufacturer claims" — [ePortID](https://www.eportid.com/feeds/blog/biometric-authentication-enterprise-security-2025)
- [I] Humanity Protocol pivoted from proof-of-personhood to biometrics-based verifiable credentials in Feb 2026, keeping palm biometrics — [Biometric Update 02/2026](https://www.biometricupdate.com/202602/humanity-protocol-pivots-from-proof-of-personhood-but-sticks-with-palm-biometrics)
- [I] Humanode analysis: Humanity Protocol claims millions of human IDs, but independent audits are not public. Off-chain parts (palm scanning, ID issuance) are operated by the HP team and its technical partners — [Humanode blog](https://blog.humanode.io/exploring-anti-sybil-approaches-humanity-protocol/)
- [V] The palm-vein "Physical Humanity Scanners" (Phase 3) are initially at crypto events and in major cities, with expansion planned through 2026. The algorithms are said to be trained on more than 500,000 biometric samples — [7BlockLabs](https://www.7blocklabs.com/blog/humanity-protocol-phase-1phase-2-technology-roadmap-web3-identity-layer-and-verifiable-credentials); [Messari](https://messari.io/report/humanity-protocol-bringing-identity-verification-onchain); [Delphi Digital](https://members.delphidigital.io/reports/humanity-protocol-report)
- [I] Amazon One: retail service discontinued on 3 Jun 2026 ("limited customer adoption"). The healthcare status is reported inconsistently, and palm data was to be deleted — [GeekWire 01/2026](https://www.geekwire.com/2026/amazon-is-ending-its-palm-id-system-for-retail-amazon-one-as-it-closes-physical-stores/); [Biometric Update 01/2026](https://www.biometricupdate.com/202601/amazon-bids-farewell-to-palm-biometrics-discontinues-amazon-one-for-retail); [IDTechWire](https://idtechwire.com/amazon-discontinues-amazon-one-palm-biometric-payment-system-due-to-limited-customer-adoption/); [AWS FAQ](https://aws.amazon.com/one/faqs)
- [V] Tencent offers palm recognition (WeChat Palm) as an alternative. It is competitor marketing — [Tencent Palm](https://palm.tencent.com/resources/top-palm-recognition-providers-2026)

### Inferences
- An FAR of 8e-7 (1:1) would give about 800 false matches per new enrollee at N = 1e9 in a 1:N search. Without demonstrable 1:N capacity, palm vein is not suitable as the sole global dedup factor, though it may serve as a second factor in a fusion.
- An HP-style closed pipeline contradicts the "decentralized, without a single company" goal.

### Gaps
- Finger vein (Hitachi VeinID) accuracy and prices, independent palm-vein 1:N studies, and multimodal fusion results (e.g. iris + face) were not researched due to the tool-call limit.
- Humanity Protocol's actual on-device ZK implementation (code, audit) was not found.

## 5. Presentation attack detection (PAD)

### Takeaway
Iris PAD is the weak point. In LivDet-Iris 2017, textured contact lenses were much harder to detect than printouts (one review: APCER 10.59% vs. 1.52%), and unknown attack types generalize poorly. ISO/IEC 30107-3 is the test standard. No open-source PAD model for iris with certified performance was found.

### Cited Findings
- [I] LivDet-Iris 2017 covered printouts and textured contact lenses. Contact lenses were harder: APCER 10.59% for lenses vs. 1.52% for printouts with the same algorithm. State-of-the-art methods show limited accuracy, especially in open-set scenarios — [Czajka & Bowyer, "PAD for Iris Recognition: An Assessment of the State of the Art" (arXiv 1804.00194)](https://arxiv.org/pdf/1804.00194); [arXiv 2002.09137 (IEEE TIFS)](https://arxiv.org/pdf/2002.09137)
- [I] Cross-spectrum PAD (NIR and visible) — [arXiv 2205.02573](https://arxiv.org/pdf/2205.02573); NIR-camera PAD — [PMC5981581](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC5981581/)
- [Unverified] A blog cites an APCER of 39.58% for unknown attacks at LivDet-Iris 2023. This was not verified against the primary report, so it should not be used without checking — (no reliable source)

### Inferences
- Off-the-shelf USB iris scanners (IriShield) without certified PAD are vulnerable to contact lenses and artefacts. A decentralized system needs supervised enrollment by an operator, multispectral or multi-frame capture, or ISO 30107-3-tested devices.
- A higher-value incentive (e.g. a basic income) raises the attacker's motivation. PAD must be evaluated against adaptive attackers, not just LivDet sets.

### Gaps
- ISO/IEC 30107-3 test labs (iBeta, BixeLab), iris device certifications, and open-source PAD models (e.g. Notre Dame "D-NetPAD", which is open) were not verified in this session.
- Face PAD (iBeta level 1/2) was not researched.

## 6. Costs per enrollment station and per person; inclusion

### Takeaway
A NIR iris sensor costs about $180–200 (hardware only). An operational station (sensor, laptop/SBC, PAD, operator) and costs per person (operator incentives) were not documented with sources. World publishes no Orb unit price.

### Cited Findings
- IriShield USB about $180–190 / ₹5,300–9,100 — see section 2 ([Reach.dog](https://reach.dog/shop/fulcrum-biometrics-inc/products/irishield-usb-mk-2120u-iris-scanner); [Moglix](https://www.moglix.com/iritech-irishield-mk2120u-single-black-iris-scanner/mp/msn457mqg8nn9j))
- No Orb or Orb Mini price was published — [MobileIDWorld](https://mobileidworld.com/worldcoin-launches-orb-mini-biometric-verification-device-in-us-market/)

### Inferences
- A rough station cost of about $300–600 (scanner plus a cheap laptop) is my own estimate and is unsourced.

### Gaps
- Per-person enrollment costs (World operator fees), the Aadhaar cost per enrollment, and inclusion data (aniridia, cataracts, nystagmus, eye injuries, missing hands for palm, failure-to-enroll rates) were not researched due to the tool-call limit. This is important for a fallback path such as social vouching or a document path.
