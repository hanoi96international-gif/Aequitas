const PS = '/api'; // proof calls proxied via /api/prove on this node (avoids browser CORS)
const CID = '0x786';
const V7_CONTRACT = '0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78';
const WC_PROJECT_ID = '32365adb6cf733a385948841ad399cf9';
let waddr = '', proofData = null, curLang = 'en';
let wcProvider = null; // WalletConnect EthereumProvider instance, once connected via connectWalletConnect()

// The EIP-1193 provider actually in use for signing — WalletConnect if that's
// how the user connected, otherwise the injected MetaMask provider. Both
// expose the same request()/on() surface, so call sites don't need to care
// which one they're talking to.
function activeProvider() { return wcProvider || window.ethereum; }

const T = {
en:{
  'x-consensus-ghostdag-knightdag':'◆ Consensus: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Contract Code',
  'x-demurrage-is-a-holding-cost':'Demurrage is a holding cost on money — a negative interest rate that makes hoarding expensive and circulation attractive. It has historical precedent: the Wörgl experiment (Austria, 1932) used a demurrage currency and reduced local unemployment by 25% within one year. The Central Bank of Austria shut it down precisely because it worked too well and threatened the banking monopoly. The Chiemgauer (Germany, 2003) operates on the same principle and has circulated successfully for over 20 years. Aequitas implements continuous demurrage at 0.5% per month, applied only after a 3-month grace period of inactivity.',
  'x-network-consensus':'→ Network / Consensus',
  'x-node-decentralization-roadmap':'Node Decentralization Roadmap',
  'x-open-source-chain-logic':'Open Source Chain Logic',
  'x-phase-0-now':'Phase 0 (now):',
  'x-phase-1-100-humans':'Phase 1 (100+ humans):',
  'x-phase-2-1-000-humans':'Phase 2 (1,000+ humans):',
  'x-phase-3-10-000-humans':'Phase 3 (10,000+ humans):',
  'x-protocol-mechanisms':'Protocol Mechanisms',
  'x-what-happens-to-aeq-when':'What happens to AEQ when people die or become permanently incapacitated? In Bitcoin and most cryptocurrencies, lost wallets mean permanently lost supply — millions of BTC are estimated to be inaccessible forever. Aequitas solves this through a multi-stage inactivity recovery system: if a wallet shows no activity for an extended period, its balance is gradually returned to the community through the UBI pool, ensuring the total effective supply remains meaningful.',
  'x-what-if-someone-is-hospitalized':'What if someone is hospitalized, incarcerated, or otherwise unable to access their device for months? The Guardian system allows a trusted person — another verified human — to confirm that the wallet owner is still alive, preventing their AEQ from being moved to escrow. The Guardian has strictly zero financial access: they can only call a single function that resets the inactivity clock. They cannot move, spend, or access any funds under any circumstances.',
  'bv-bind':'🔗 Generate Node Binding Signature',
  'bv-check-d':'The second call lists every verifier and compares them: whether all hold the same number of enrollments, whether anyone is missing a seed, and whether the keys agree. If your entry shows a divergence, it is better to find out here than during someone’s registration.',
  'bv-check-t':'Checking That It Works',
  'bv-desc':'A block-producing node secures the <strong style="color:var(--text)">ledger</strong>. A bio verifier secures something else: the promise that <strong style="color:var(--neon)">each human registers only once</strong>. These are separate roles — you can run either, or both on the same machine.',
  'bv-guide-sub':'Step by step &middot; No cryptography knowledge required &middot; About 30 minutes, most of it downloading',
  'bv-honest-d':'This part is in beta and the limits are real. The joint comparison consumes one-time cryptographic material, and one delivery currently covers a few dozen registrations before more is needed — so the confidential path proves itself at small scale first, not at millions. The work also grows with the number of people enrolled. We publish these numbers rather than round them off: a system that asks for your face has no business being vague about what it can and cannot do yet.',
  'bv-honest-t':'Where this stands today — plainly',
  'bv-need-1':'<strong style="color:var(--text)">A registered Aequitas human account.</strong> Same rule as for block production, and for the same reason: one human, one key. Without it, a single person could quietly become a whole committee.',
  'bv-need-2':'<strong style="color:var(--text)">A small Linux server with Docker.</strong> 2 GB RAM is enough. No GPU — the comparison is arithmetic on 64-byte values. The machine already running your node is fine.',
  'bv-need-3':'<strong style="color:var(--text)">A domain name with HTTPS.</strong> Other committee members must reach you. A subdomain of something you already own is enough.',
  'bv-need-4':'<strong style="color:var(--text)">To stay online.</strong> Every member of a committee must answer for a registration to finish. A verifier that is often away slows people down instead of protecting them.',
  'bv-need-t':'Before You Start — What You Need',
  'bv-s1-note':'Keep the private half on your server and nowhere else. The public half is meant to be shared — it is how others verify that you attested something. <strong style="color:var(--text)">Your own projection seed matters:</strong> because every verifier uses a different one, a stolen database from one verifier cannot be compared against another’s. Lose the seed and your stored shares become meaningless, so back it up somewhere you control.',
  'bv-s1-t':'Step 1 — Generate Your Own Keys',
  'bv-s1-warn-d':'Two verifiers holding the same secret count as one, and the committee would be smaller than it looks. Nobody — including us — should ever send you a key.',
  'bv-s1-warn-t':'Generate them yourself. Never accept keys from anyone.',
  'bv-s2-d':'Put the values from Step 1 into a file readable only by you. One value per line, no quotes.',
  'bv-s2-note':'<strong style="color:var(--gold)">Leave ALLOW_REAL_BIOMETRIC_DATA on false</strong> until you have read the data-protection notes. With it off, your verifier joins the network and takes part in test enrollments without ever storing data from a real person. That is the right way to start, and there is no hurry to change it.',
  'bv-s2-t':'Step 2 — Write the Configuration File',
  'bv-s3-note':'A healthy answer reports <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> and <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. The first is the claim that no whole template is stored, in a form you can check yourself rather than take on faith. Check it now and again later — it is your own guarantee as much as anyone else’s.',
  'bv-s3-t':'Step 3 — Start the Verifier',
  'bv-s4-d':'Other committee members reach you over the public internet, so the port must not be exposed unencrypted. Caddy obtains a certificate on its own.',
  'bv-s4-t':'Step 4 — Put HTTPS in Front',
  'bv-s5-d':'Block producers bind their signing key to a registered human wallet: the wallet signs <strong style="color:var(--text)">Aequitas: authorize validator &lt;address&gt;</strong> and the chain refuses the slot without it. The button below produces exactly that signature — for the validator role. <strong style="color:var(--text)">A verifier key has no such binding yet.</strong> Its public half is collected out of band (Step 6) and added to the list every proof server checks. Nothing on the chain ties it to a person. Until that exists, a committee counts machines, not people, and one operator could hold several. We would rather say so here than let the number look stronger than it is.',
  'bv-s5-t':'Step 5 — What Binds a Key to a Human (and What Does Not Yet)',
  'bv-s6-d':'Send the <strong style="color:var(--text)">public</strong> half from Step 1, together with your HTTPS address, to the group. It is added to the list every proof server checks against, and from then on your attestations count toward the quorum. Nothing secret leaves your machine in this step — that is the point of the split: the private half stays with you forever, and the public half is useless without it.',
  'bv-s6-t':'Step 6 — Publish Your Public Key',
  'bv-status-d':'The verifier source is <strong style="color:var(--text)">not public yet</strong>, so the steps below cannot be completed by everyone today. They are published now because the design should be checkable before it is deployed, not after. If you want to run one, ask in the Telegram group linked on the front page. Opening this repository is what turns the guide below from a plan into an invitation, and it is the next thing we owe you.',
  'bv-status-t':'Status: closed beta — read this before you start',
  'bv-title':'Or Become a Bio Verifier — the Role That Makes Uniqueness Decentralized',
  'bv-what-d':'A face is never sent to you. Your machine stores one <strong style="color:var(--text)">additive share</strong> of a 64-byte sketch: on its own it is indistinguishable from random noise, and no computation you can run recovers a face from it. Comparisons happen jointly with the other members of your committee, and none of you learns anything but the answer — <em>duplicate: yes or no</em>. That is not a promise about our good intentions; it is a property of the arithmetic.',
  'bv-what-t':'What you would hold — and what you would never see',
  'bv-why-d':'A registration is accepted only once <strong style="color:var(--text)">several different verifiers</strong> have attested it. One stolen key is not enough — an attacker needs a whole committee. And because <strong style="color:var(--neon)">one human may hold exactly one validator key</strong>, buying a committee means being that many people. With 100 verifiers, an attacker controlling 10 of them has under a 1-in-1,000 chance of owning a full committee of three. Every person who joins makes that number smaller. This is the one place where the count of participants <em>is</em> the security. <strong style="color:var(--text)">This arithmetic assumes one human per verifier key.</strong> For block production the chain enforces that; for verifier keys it does not yet (Step 5). Until it does, the number above is an upper bound on the security, not a measurement of it.',
  'bv-why-t':'Why every additional verifier makes the network harder to corrupt',
  'x-0-1-split-40-30':'0.1% · split 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 humans. Sliding wealth cap 5x &#8594; 25x. Foundation building.',
  'x-0-8211-2-years':'0 &#8211; 2 years',
  'x-0-perfect-equality':'0 = perfect equality',
  'x-1-000-aeq-minted':'+1,000 AEQ minted',
  'x-1-000-aeq-per-human':'1,000 AEQ per human',
  'x-1-000-aeq-will-be':'1,000 AEQ will be credited automatically',
  'x-10-000-8211-1m-humans':'10,000 &#8211; 1M humans. Min 10 nodes. Fully decentralized.',
  'x-100-8211-10-000-humans':'100 &#8211; 10,000 humans. Fixed cap 25x. Open node joining.',
  'x-100-maximum-concentration':'100 = maximum concentration',
  'x-1m-humans-global-ubi-at':'1M+ humans. Global UBI at scale. Gini target &lt;0.30.',
  'x-9679-liquidity-lp-30':'&#9679; Liquidity LP 30%',
  'x-9679-treasury-10':'&#9679; Treasury 10%',
  'x-9679-ubi-pool-20':'&#9679; UBI Pool 20%',
  'x-9679-validators-40':'&#9679; Validators 40%',
  'x-active-validators':'Active Validators',
  'x-add-aequitas-chain-to-metamask':'Add Aequitas Chain to MetaMask to view your AEQ balance, send transactions, and interact with the V7 contract directly from your browser or mobile wallet.',
  'x-admin-keys-or-governance-votes':'admin keys or governance votes',
  'x-aeq-activity':'AEQ ACTIVITY',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'Aequitas BlockDAG — nothing wasted',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Aequitas Chain (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas implements this mathematically. Every verified human receives exactly 1,000 AEQ &#8212; billionaire or subsistence farmer, no exceptions. Four redistribution mechanisms ensure inequality cannot accumulate indefinitely. The Gini coefficient is tracked on-chain in real time.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — Proof of Humanity Chain',
  'x-android-apk-direct-download':'Android APK · direct download',
  'x-architecture':'Architecture',
  'x-automatic-on-chain':'automatic on-chain',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (Directed Acyclic Graph)',
  'x-blockdag-parallel-production':'BlockDAG · Parallel production',
  'x-blockdag-proof-of-humanity':'BlockDAG + Proof of Humanity',
  'x-blue-score':'"blue score"',
  'x-both-blocks-are-kept-ghostdag':'Both blocks are kept — GHOSTDAG merges the concurrent one in and still counts it toward the canonical order.',
  'x-canonical-winner':'canonical winner',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Comparable to the USA (0.41) or France (0.32). Within the range of most developed economies. Redistribution mechanisms actively flattening the curve.',
  'x-confirm-ward-is-alive':'✓ CONFIRM WARD IS ALIVE',
  'x-core-technology':'Core Technology',
  'x-daily-ubi-returns-to-all':'daily UBI returns to all verified humans',
  'x-demurrage-0-5-mo':'Demurrage (0.5%/mo)',
  'x-device-bound-zk-proof-one':'Device-bound ZK proof · one registration per device',
  'x-diagonal-line-perfect-equality':'diagonal line = perfect equality',
  'x-disconnect-wallet':'⊘ DISCONNECT WALLET',
  'x-distinct-proposers-recent-blocks':'Distinct proposers, recent blocks',
  'x-distribution':'📈 Distribution',
  'x-elliptic-curve':'Elliptic Curve',
  'x-entire-distribution':'entire distribution',
  'x-evm-compatible':'EVM Compatible',
  'x-fill-ghostdag-verdict-thin-ring':'Fill = GHOSTDAG verdict · thin ring = proposer · one column per height. Hover any block for details.',
  'x-generate-node-binding-signature':'🔗 Generate Node Binding Signature',
  'x-run-a-coordinator':'🚪 Run a Coordinator',
  'co-title':'Or Run a Coordinator — the Door Every Human Walks Through',
  'co-desc':'The coordinator is where a person arrives: it issues the challenge, fans the capture out to the verifiers, counts their votes, and issues the attestation the chain mints on. For a long time exactly one of them existed — which meant every registration in the network passed through a single machine. Not because anything was missing, but because nobody had run a second one.',
  'co-status-t':'Status: closed beta — the same caveat as the verifier',
  'co-status-d':'The coordinator lives in the same repository as the verifier, and that repository is <strong style="color:var(--text)">not public yet</strong>. So the steps below cannot be completed by everyone today. They are published anyway, for the same reason: a design should be checkable before it is deployed, not after.',
  'co-power-t':'What a coordinator can do — and what it cannot',
  'co-power-d':'It <strong style="color:var(--text)">cannot invent a human</strong>. No bio_hash exists until several different verifiers have attested it, and a coordinator holds none of their keys. What it can do is bind an <strong style="color:var(--text)">existing</strong> bio_hash to a wallet — so a dishonest one could redirect an allocation to an address of its choosing. That is a real power, it grows with every coordinator added, and anyone weighing whether to trust one should know the difference.',
  'co-safe-t':'Why a second coordinator is safe at all',
  'co-safe-d':'It was not always. Until August 2026 the promise <strong style="color:var(--text)">one human, one registration</strong> hung on a Redis lock inside the coordinator — and two independent coordinators share no Redis, so two simultaneous registrations of the same person would both have gone through. Now <strong style="color:var(--text)">every verifier checks for itself</strong>, before its own write, whether that face is already enrolled. The guarantee no longer depends on any shared service or shared secret, so a coordinator can join or fall away without changing it.',
  'co-need-t':'What you need',
  'co-need-d':'A registered Aequitas account — the same rule as for producing blocks and for verifying: one human, one key. A server with Docker and a public HTTPS address, because browsers hand no camera to an insecure page. And two keys of your own, which you generate yourself and which never leave your machine: one that signs your attestations, one that maps wallet addresses to markers.',
  'co-keys-t':'Never accept a key from anyone — including us',
  'co-keys-d':'Two coordinators sharing one signing key are not two coordinators; they are one with two addresses, and the quorum that is supposed to protect people would look satisfied without being so. Generate both keys on your own machine, with your own randomness, and let neither of them off it.',
  'co-auth-t':'Authorizing your key — no permission required',
  'co-auth-d':'Until your key is authorized, verifiers refuse everything it signs. Authorizing it takes two proofs and no approval from anyone: your wallet signs that a registered human stands behind this key, and your coordinator proves on its own host that the key is really its own. Use the button above to produce the first; your coordinator produces the second by itself. Until August 2026 you also needed a shared secret from us — which meant that secret <em>was</em> the permission. It is gone.',
  'co-pernode-t':'The registry is per-node, and that is deliberate',
  'co-pernode-d':'An authorization written to one node does not travel to the others — there is no transaction for it and no gossip. A replicated trust list would be exactly the central authority this system is built without: every operator decides for themselves whose attestations their node accepts. The cost is that your authorization has to be sent to each node that should honour it. The signature itself is portable, so you sign once and send it everywhere; a node you skip will simply keep refusing you.',
  'co-law-t':'What you learn about other people — and what follows from it',
  'co-law-d':'The capture passes through you; you hand it on and keep nothing. But you alone hold the mapping between wallet address and marker for the people who register through you — which is why your marker key must stay yours: shared, any operator could compute the marker for any public address and look up whose face belongs to it. It also means you become the <strong style="color:var(--text)">data controller</strong> for those people under GDPR. Not us. Access, erasure and objection requests reach you, and that is not a formality.',
  'co-limit-t':'The one limitation this creates',
  'co-limit-d':'Erasure by wallet address only works at the coordinator where the enrolment was made: your marker hangs on your key, and another coordinator derives a different one for the same address. A "not found" from elsewhere therefore means "not registered here", not "not registered" — and the answer says so. The route through a person\'s own bio_hash, the one that belongs to them and needs no operator at all, works at every coordinator, because that identifier stays the same.',
  'x-authorize-coordinator-key':'🔑 Authorize a Coordinator Key',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — one true order out of a tangled graph',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Gini coefficient',
  'x-gini-coefficient-0-1':'Gini coefficient (0–1)',
  'x-gini-index-history':'Gini Index History',
  'x-gini-target-scandinavian-level':'Gini target (Scandinavian level)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'Groth16 ZKP (Zero-Knowledge)',
  'x-guardian-system-8212-human-failsafe':'Guardian System &#8212; Human Failsafe for Lost Wallets',
  'x-hash-wallet':'Hash / Wallet',
  'x-healthier-than-most-nations-on':'Healthier than most nations on Earth. Comparable to Scandinavia (0.27) and Germany (0.31). Wealth cap and demurrage successfully maintaining fair distribution.',
  'x-higher-than-most-european-nations':'Higher than most European nations — comparable to Brazil (0.53) or Russia. Protocol redistribution at elevated intensity.',
  'x-honest-limitation':'Honest limitation:',
  'x-how-it-works':'How It Works',
  'x-how-to-read-this-chart':'How to read this chart:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'humans could register',
  'x-imagine-a-world-where-every':'"Imagine a world where every person on Earth &#8212; regardless of where they were born, what language they speak, or how much money their parents had &#8212; receives a guaranteed daily income simply for being human. Not as charity. As a mathematical right, enforced by code that no government or corporation can override."',
  'x-inactive-escrow':'Inactive escrow',
  'x-inactivity-timeline':'Inactivity Timeline',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (post-quantum safe)',
  'x-key-protections':'Key protections:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — Aequitas\'s own upgrade beyond fixed-K GHOSTDAG',
  'x-knightdag-secured':'· KnightDAG-secured',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Like Scandinavia (~0.27)',
  'x-liquidity-pool-30':'Liquidity Pool (30%)',
  'x-loading-blocks':'Loading blocks...',
  'x-loading-topology':'Loading topology…',
  'x-loading-transactions':'Loading transactions...',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Lorenz Curve — AEQ Distribution Across Humans',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: if AEQ balance shows 0 after registration, go to Settings → Networks → delete Aequitas Chain → re-add via this website',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: if AEQ shows 0 after adding, delete the network and re-add it using the button above.',
  'x-money-exists-because-people-exist':'Money exists because people exist. Therefore every person should have an equal share of money simply by virtue of being human.',
  'x-money-exists-because-people-exist-2':'"Money exists because people exist. Nothing more, nothing less."',
  'x-most-unequal-currency-ever':'Most unequal currency ever',
  'x-multi-validator-network':'Multi-validator network',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: not yet significant',
  'x-no-snapshots-yet-first-one':'No snapshots yet — first one saved after the next UBI distribution.',
  'x-no-stake-blockchain':'No-Stake Blockchain',
  'x-node-operator-guide-pdf':'📄 Node Operator Guide (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET must be a registered Aequitas human',
  'x-one-human-one-wallet-1':'One Human = One Wallet = 1,000 AEQ',
  'x-p2p-protocol':'P2P Protocol',
  'x-paid-out-daily':'paid out daily',
  'x-permanent-on-chain':'Permanent · On-chain',
  'x-phase-roadmap-8212-the-path':'Phase Roadmap &#8212; The Path to Global Scale',
  'x-phase-transitions-are-automatic-8212':'Phase transitions are automatic &#8212; triggered by human count thresholds, enforced by the smart contract. No governance vote, no admin key.',
  'x-planned-post-beta':'Planned (post-beta)',
  'x-postgresql-persistent':'PostgreSQL (persistent)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Provide AEQ / tUSD liquidity to earn 30% of all swap fees, distributed daily.',
  'x-recorded-after-each-ubi-distribution':'Recorded after each UBI distribution. Shows how equality evolves as the network grows. Lower is better — target is Gini below 0.30.',
  'x-redistribution':'REDISTRIBUTION',
  'x-run-a-node':'⚙️ Run a Node',
  'x-run-a-verifier':'⚙️ Run a Verifier',
  'x-set-guardian':'🛡 SET GUARDIAN',
  'x-swap-fees-0-1':'Swap fees (0.1%)',
  'x-sybil-resistance-8212-current-state':'Sybil Resistance &#8212; Current State, Honestly',
  'x-the-4-redistribution-mechanisms':'The 4 Redistribution Mechanisms',
  'x-the-core-innovation':'The Core Innovation',
  'x-the-matching-threshold-has-not':'The matching threshold has not yet been calibrated against real captures',
  'x-the-vision-8212-a-global':'The Vision &#8212; A Global Basic Income Protocol',
  'x-the-year-is-2009-satoshi':'The year is 2009. Satoshi Nakamoto releases Bitcoin. For the first time, value can transfer between any two people without a bank. A genuine revolution. But something goes wrong almost immediately.',
  'x-this-is-not-a-0815':'This is not a 0815 blockchain with one block at a time. Aequitas runs a real BlockDAG, ordered by GHOSTDAG — and since 2026, secured by KnightDAG, Aequitas\'s own adaptive evolution of it. This is the mechanism every balance, every UBI payout, and every wealth-cap enforcement ultimately depends on for a single, agreed-upon history.',
  'x-today-beta':'Today (beta)',
  'x-today-this-verifies-one-device':'Today this verifies one device, not yet one unique person',
  'x-traditional-blockchain-wasted-work':'Traditional blockchain — wasted work',
  'x-treasury-10':'Treasury (10%)',
  'x-trusted-verified-human':'trusted verified human',
  'x-two-validators-produce-at-once':'Two validators produce at once → one wins, one is discarded — wasted work, and it caps how fast the network can safely go.',
  'x-ubi-pool-20':'UBI Pool (20%)',
  'x-validators-pool-40':'Validators Pool (40%)',
  'x-view-source-on-github':'🐙 View Source on GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'Wealth Cap Multiplier — Bootstrap Slider',
  'x-wealth-cap-overflow':'Wealth cap overflow',
  'x-wealth-distribution-analysis':'Wealth Distribution Analysis',
  'x-what-happens-when-someone-is':'What happens when someone is hospitalized, incarcerated, or dies? In most crypto systems, lost wallets mean lost coins forever. Aequitas has a three-layer inactivity recovery system.',
  'x-what-is-a-guardian':'What is a Guardian?',
  'x-what-is-and-is-not':'What is and is not private:',
  'x-what-would-a-cryptocurrency-look':'"What would a cryptocurrency look like if designed from first principles to be fair to every human being?"',
  'x-why-a-normal-blockchain-isn':'Why a normal blockchain isn\'t enough',
  'x-worse-than-any-country-on':'Worse than any country on Earth (South Africa record: 0.63). Approaching Bitcoin (0.85). Protocol at maximum intervention — wealth cap and redistribution at full force.',
  'x-year-2-180d':'Year 2 +180d',
  'x-zk-device-key-proof':'ZK Device-Key Proof',
  'swap-price-flat':'No trades in this period — the price has not moved. The chart is working; the market is quiet.',
  'mpc-optin-title':'Optional — help check for duplicate registrations (prepared, not yet in service)',
  'mpc-optin-desc':'Prepared, but not yet in service. Later your node will be able to help verify that nobody registers twice without ever seeing anyone\'s biometric data: each participating party holds only a mathematical share of every enrolled template — noise on its own — and they compare a new capture together, so no single machine can reconstruct anything. Today this path decides nothing. The duplicate check does not run over it, and the committee is a fixed list rather than drawn automatically, so setting the three variables below changes nothing about registrations for now.',
  'mpc-optin-note':'The share file contains one-time randomness that only your node may hold — never copy it to another machine and never commit it anywhere. It currently has to come from the operator, which is the remaining central dependency. You do not need a new key: your node identifies itself to the other members with the same signing key it already uses for blocks.',
  'logo-sub':'PROOF OF HUMANITY','live':'LIVE',
  'reg-title':'🔐 Register as a Verified Human',
  'reg-sub':'Join the Aequitas network and receive your 1,000 AEQ Universal Basic Income grant. Registration is one-time, permanent, and completely gasless. No personal data is ever stored.',
  'app-title':'REGISTRATION VIA ANDROID APP',
  'app-text':'Registration captures your face and a short liveness sequence with the phone camera. Independent matching services check that a living person is present and that this face is not already registered; they must agree by quorum. A Groth16 ZK proof then carries the result to the chain without revealing anything about you. Your <strong style="color:var(--gold)">1,000 AEQ credited automatically</strong> upon verification. <strong style="color:var(--gold)">Note:</strong> the matching threshold has not yet been calibrated against real captures — see the FAQ below.',
  's1t':'Face capture','s1d':'The app records your face and a short liveness sequence and sends them to independent matching services. They check that a living person is present and compare the face against everyone already registered. The images are discarded after processing.',
  's2t':'ZK Proof Generation','s2d':'A Groth16 ZK proof commits your bio_hash into commitment = keccak256(bioHash‖wallet) without revealing it. The nullifier is derived from that hash, so the same face cannot count twice — see the FAQ below.',
  's3t':'Connect Wallet','s3d':'The app opens MetaMask on this page · connect your Ethereum wallet · the proof is cryptographically bound to your wallet address',
  's4t':'1,000 AEQ Granted','s4d':'Registration confirmed on Aequitas BlockDAG within 1 second · 1,000 AEQ credited instantly · your identity is permanently recorded as a verified human',
  'priv-bar':'🔒 Face check by quorum · Groth16 ZKP · Images discarded after checking · One registration per person',
  'conn-wallet':'CONNECTED WALLET','proof-recv':'⚡ ZK PROOF RECEIVED','proof-hint':'Connect wallet to register',
  'btn-conn':'🦊 CONNECT METAMASK','btn-reg':'🔐 REGISTER ON-CHAIN',
  'btn-wc':'🔗 CONNECT WALLETCONNECT',
  'reg-log-hint':'// Open Aequitas Android App to generate your proof, then return here...',
  'reg-details':'Registration Details','k-network':'Network','k-chainid':'Chain ID','k-grant':'UBI Grant',
  'k-fee':'Gas Fee','free':'FREE — completely gasless','k-limit':'Registrations','k-limit-v':'Once per person · permanent · immutable',
  'k-bio':'Face','never-stored':'Images discarded after checking — no validator holds a whole template',
  'k-proof':'Proof System','k-conf':'Confirmation','k-conf-v':'Within 1 second (1 block)',
  'k-sybil':'Sybil Protection','k-sybil-v':'One identity per person · face-bound, threshold not yet calibrated',
  's-height':'Block Height',
  's-humans':'Verified Humans',
  's-supply':'Total Supply','s-supply-sub':'Always = Humans × 1,000 AEQ',
  's-uptime':'Uptime',
  'k-chain':'Chain Name','k-symbol':'Symbol','k-btime':'Block Time',
  'k-cons':'Consensus','k-storage':'Storage','k-dec':'Decimals',
  'btn-add-mm':'+ ADD AEQUITAS NETWORK',
  'humans-title':'Verified Humans on Aequitas Chain',
  'h-what':'What is a Verified Human?','h-what-t':'A Verified Human is a wallet address proven to belong to someone whose face is not already registered. Independent matching services must agree by quorum before it counts, and only a Groth16 ZK proof reaches the chain — no image and no template. <strong style="color:var(--gold)">Until 2026-08-23 this verified one device rather than one person; that is no longer the case.</strong>',
  'h-zkp':'Zero-Knowledge Proof System','h-zkp-t':'Aequitas uses Groth16 on BN128 — same curve as Ethereum and Zcash. Proof: ~200 bytes. Verification: ~10ms. commitment = keccak256(deviceKey‖wallet). The nullifier is bound to this device: losing your phone does not let you create a second identity on it, but a different device can still register separately. No key material is ever revealed or stored server-side.',
  'h-sybil':'Sybil Resistance — Current State','h-sybil-t':'The nullifier is derived from the bio_hash of your face, so the same face cannot be registered twice — across devices as well, which a device key never could. What it rests on is a matching threshold that has not yet been calibrated against real captures: the cryptography is exact, the biometrics underneath it is a measurement whose error rate has not been quantified.',
  'h-global':'Global Financial Inclusion','h-global-t':'No bank account, no credit card, no prior cryptocurrency required. Just an Android smartphone with a camera. Aequitas is designed to be accessible to every human on Earth.',
  'h-bio-hw':'Identity Verification Roadmap','h-bio-hw-t':'Today (beta): a face check across independent matching services that must agree by quorum. Its threshold has not yet been calibrated against real captures — that needs about 1000 impostor pairs before any number is quoted. Planned: that calibration, and a duplicate check in which no service holds a whole template.',
  'reg-humans':'Registered Humans','h-desc':'Every address below belongs to someone whose face was checked against every existing registration by independent services, proven with a ZK proof, and credited exactly 1,000 AEQ. The registry is permanent, immutable and on-chain. See the FAQ for what the matching threshold does and does not guarantee today.',
  'no-humans':'No humans registered yet.\n\nDownload the Aequitas Android App and be the first human on the chain!',
  'reg-stats':'Registry Stats','total-humans':'Total Humans',
  'idx-title':'Aequitas Index — Real-Time Economic Equality Score',
  'idx-desc':'The Aequitas Index is derived from the <strong style="color:var(--teal)">Gini coefficient</strong> — the international standard for measuring wealth inequality, adopted by the World Bank, OECD, and UN. It captures the complete balance distribution across every verified human simultaneously. <strong style="color:var(--neon)">0 = perfect equality</strong> (every wallet holds the same AEQ). <strong style="color:var(--red)">100 = total concentration</strong> (one wallet holds all AEQ). Bitcoin Gini ≈ 0.85 (Index 85) · South Africa (world record) ≈ 0.63 · Scandinavia ≈ 0.27 · Aequitas long-term target: Gini below 0.30 — comparable to the most equal developed economies, enforced by the wealth cap and redistribution pools.',
  'gini-what-title':'What is the Gini Coefficient?',
  'gini-what-text':'Developed by Italian statistician Corrado Gini (1912). Measures wealth distribution by comparing actual balances against a hypothetical perfectly equal baseline — visualized as the Lorenz curve. Scale: 0 (everyone holds the same) to 1 (one person holds everything). Used by World Bank, OECD, UN to compare countries. Reference values: Bitcoin ≈ 0.85 · South Africa (world record) ≈ 0.63 · USA ≈ 0.41 · Germany ≈ 0.31 · Scandinavia ≈ 0.27 · Aequitas long-term target: Gini below 0.30 — comparable to Scandinavian countries, enforced by wealth cap (bootstrap: 5×→25× per human).',
  'gini-calc-title':'How is the Aequitas Index calculated?',
  'gini-calc-text':'All AEQ balances of verified humans are collected. The formula computes the mean absolute difference between every possible pair of balances, normalized by population squared (n²) and the mean balance (x̄). Result 0–1 multiplied by 100 = Aequitas Index. Updated on-chain after every registration, monthly demurrage run, pool payout, and wealth cap event — via keeper calling updateGini().',
  'gini-why-title':'Why Gini — and not a simpler metric?',
  'gini-why-text':'A simple richest-vs-poorest ratio is easy to game: 10,000 wallets could show a low spread but 90% of AEQ concentrated in 100 hands — Gini detects this, a ratio does not. The coefficient captures the complete distribution across all verified humans in one auditable number. Aequitas publishes this on-chain — transparent, tamper-evident, globally verifiable. It is the primary signal for automatic phase transitions, wealth cap calibration, and redistribution intensity. No human can override the index reading or the mechanisms it triggers.',
  'curr-idx':'Current Index','bar-0':'0 — Perfect Equality','bar-100':'100 — Max Inequality','wcap-lbl':'Current Wealth Cap:','wcap-mult':'Multiplier:','wcap-avg':'Fair share:',
  'gini':'Gini Coefficient','gini-desc':'0 = equal · 1 = unequal',
  'supply-desc':'Always = Humans × 1,000 AEQ',
  'phase':'Protocol Phase','phase-desc':'Auto-advances by human count',
  'humans-desc':'Face-verified registrations',
  'pools-title':'Redistribution Pools',
  'pools-desc':'Every swap fee, demurrage charge, and wealth cap overflow is automatically split across four pools. No manual intervention — the protocol handles all redistribution through code alone. All pools pay out daily.',
  'vel-pool':'Validators Pool','vel-pool-desc':'40% of all fees → node operators who secure the network',
  'liq-pool':'Liquidity Pool','liq-pool-desc':'30% of all fees → liquidity providers, proportional to LP shares',
  'ubi-pool':'UBI Pool','ubi-pool-desc':'20% of all fees → all verified humans equally, every 24 hours',
  'treasury':'Treasury','treasury-desc':'10% of all fees → protocol development and maintenance',
  'phases-title':'Protocol Phases',
  'phases-desc':'The wealth cap uses a bootstrap multiplier during Phase 0: max(5, min(N, 25))× the fair share (1,000 AEQ per human). With 1–4 humans: 5× the fair share. Each new human adds 1×. At 25+ humans: locks permanently at 25×. Phase 1+ maintains 25× fixed. All transitions trigger automatically by human count — no governance, no admin key.',
  'p0':'Bootstrap · &lt;100 humans · Wealth Cap: max(5,min(N,25))× fair share · Slides 5×→25× until 25th human · Currently active',
  'p1':'Growth · 100–10,000 humans · Wealth Cap: 25× the fair share = 25,000 AEQ',
  'p2':'Stability · 10,000–1M humans · Wealth Cap: 25× the fair share = 25,000 AEQ',
  'p3':'Maturity · 1M+ humans · Wealth Cap: 25× the fair share = 25,000 AEQ',
  'wealth-cap-explain':'The Wealth Cap in Phase 0 (Bootstrap) uses max(5, min(N, 25))× average AEQ balance, where N = registered humans. 1–4 humans: cap = 5× the fair share. Each new human adds 1×. 25+ humans: locked permanently at 25×. The cap always scales with the live average balance.',
  'demurrage-title':'Demurrage — Incentive to Circulate',
  'demurrage-desc':'Aequitas implements a demurrage mechanism inspired by historical complementary currencies. Idle AEQ balances slowly lose value to discourage hoarding and incentivize economic participation.',
  'dem-rate-k':'Decay Rate','dem-rate-v':'0.5% per month (continuous, not stepped)',
  'dem-grace-k':'Grace Period','dem-grace-v':'3 months of inactivity before decay begins',
  'dem-reset-k':'Clock Reset','dem-reset-v':'Any transfer, swap, or liquidity action resets the timer to zero',
  'dem-dest-k':'Decayed AEQ goes to','dem-dest-v':'Redistribution pools (40/30/20/10 split)',
  'dem-warn-k':'Warning System','dem-warn-v':'14-day notice (shown once) + 7-day repeated reminder at each login',
  'story-title':'The Story of Aequitas — Why This Exists',
  'nodes-title':'Active Nodes — Current Network Topology',
  'nodes-desc':'The Aequitas network currently operates on multiple geographically distributed nodes (live count above). All of them participate in block production, state synchronization, and API serving. They communicate peer-to-peer via libp2p and synchronize block state via HTTP. Each node runs its own PostgreSQL database for persistent state. The network is designed to support additional nodes — any operator can join.',
  'run-node-title':'Run Your Own Node — Help Secure the Network',
  'run-node-desc':'Every registered human can run an Aequitas node — no stake, no application, no permission from us. One human, one validator key: a node whose NODE_OPERATOR_WALLET is not a registered human is refused with HTTP 403, because otherwise one person could quietly become the whole validator set. Nodes participate in block production, validate the human registry, and synchronize the BlockDAG. Node operators earn a share of protocol fees via the Validators Pool (40% of all swap fees, distributed daily).',
  'bootstrap-title':'Connect a New Node','bootstrap-desc':'To run your own Aequitas node you configure no entry point at all — the validator addresses are compiled in. Your node registers automatically, syncs the full chain state, and begins participating in block production. Set PRIMARY_NODE_URL only if you deliberately want to pin one specific entry point.',
  'tech-title':'Technical Specifications','mm-config':'MetaMask Configuration',
  'k-lang':'Language','k-src':'Source','evm-yes':'Yes — JSON-RPC /rpc · MetaMask compatible',
  'proto-label':'Aequitas V7 Protocol — Technical Documentation',
  'ca-title':'Contract Addresses',
  'ca-text':'Chain: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Main): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 defines the rules of the Aequitas economy and holds the registry that makes them enforceable: every nullifier ever claimed, every registration, the wealth cap and the demurrage formula. It is immutable — no admin key, no upgrade proxy, no governance vote can change a line of it. What settles a live transfer, though, is the chain layer: the node intercepts the ERC-20 call before it reaches the EVM and applies it to its own ledger, which is what makes transfers sub-second and gasless. The contract is the rulebook and the registry; the chain is the engine that runs them, and its source is public.<br><br>The BioVerifier contract receives Groth16 zero-knowledge proofs generated entirely on the user\'s Android device. It verifies mathematically on-chain in ~10 ms that the submitted nullifier was correctly derived from a secret the registrant holds, and the chain refuses any nullifier it has already seen — without ever learning their name, identity, or biometric data. That is what rules out a second registration from the same identity source; whether that source is a person or a device depends on whether biometric mode is active. This is what makes gasless, investment-free registration possible: the proof is the only thing that ever leaves the device.<br><br>That combination is what is actually new: the rules and the one-human-one-registration registry sit in a contract nobody — not the operator, not a company, not a government — can rewrite, and the code that carries them out is open source and reproducible from this repository. All of it can be checked by anyone. What still requires trust is the operation of the nodes themselves, and the honest way to reduce that is more independent validators, not a stronger sentence here.',
  'poa-title':'1. PROOF OF ALIVE','poa-text':'<p>What happens to AEQ when people die or disappear? In Bitcoin, millions of BTC are permanently lost. In Aequitas, if someone is inactive for an extended period, their AEQ eventually returns to the community through the UBI pool.</p>',
  'poa-box':'Year 0-2: Normal usage<br>Year 2: Warning 1 — Guardian can respond<br>Year 2 + 60 days: Warning 2<br>Year 2 + 120 days: Warning 3<br>Year 2 + 180 days (2.5 years total, day 910): AEQ goes to PERSONAL ESCROW<br>Year 4 (day ~1460): If still inactive — returns to UBI Pool',
  'guard-title':'2. GUARDIAN SYSTEM','guard-text':'<p>What if someone cannot access their device for months? A trusted Guardian — another verified human — can confirm they are still alive, without any transaction rights.</p>',
  'guard-box':'1 Guardian per human (must be another verified human)<br>Guardian can ONLY call confirmAlive() — zero transaction rights<br>Guardian CANNOT move funds or transfer AEQ<br>Max 3 wards · 7-day timelock · No circular relationships allowed',
  'dem-title':'3. DEMURRAGE — Anti-Hoarding Mechanism',
  'dem-box':'Charged only on the portion above your fair share — a balance at or below it never decays<br>Rate: 0.5%/month after 3 months grace period<br>Clock resets on any transfer, swap, or liquidity action<br>Decayed AEQ redistributed to pools (not burned)',
  'dem-text':'<p>Historical precedent: The Wörgl experiment (Austria, 1932) used a demurrage currency and reduced unemployment by 25% in one year. The Chiemgauer (Germany, 2003) has operated successfully for over 20 years using a similar mechanism.</p>',
  'cap-title':'4. WEALTH CAP — Mathematical Fairness','cap-box':'Bootstrap cap: max(5,min(N,25))× current average AEQ balance<br>1–4 humans: 5× · +1× per human · 25+: 25× permanently<br>Excess AEQ instantly redistributed · No manual intervention',
  'ubi-title':'5. UNIVERSAL BASIC INCOME','ubi-box':'Sources: Swap fees (20%) · Wealth cap overflow · Demurrage · Inactive escrow<br><br>Daily: UBI Pool divided equally among all registered humans. Pool resets to zero after each distribution and refills continuously.',
  'inf-title':'6. NO ALGORITHMIC INFLATION','inf-box':'The ONLY event that creates new AEQ: a new verified human registers<br><br>Total Supply = Verified Humans × 1,000 AEQ — always, exactly.',
    'btn-download-app':'DOWNLOAD AEQUITAS APP',
  'usp-headline':'For the first time in history — everyone starts equal',
  'usp-sub':'If you own an Android smartphone, you qualify. No bank, no crypto background, no investment needed.',
  'usp-c1-title':'0.00 Start Investment','usp-c1-desc':'Registration is completely gasless. No ETH, no MATIC, no credit card. The protocol pays all fees on your behalf.',
  'usp-c2-title':'1,000 AEQ for every human','usp-c2-desc':'Billionaire or subsistence farmer — everyone gets exactly 1,000 AEQ. Not more, not less. Equal start, guaranteed by math.',
  'usp-c3-title':'Accessible to all','usp-c3-desc':'No bank account, no credit card, no government ID, no extra hardware to buy — just the camera already in your Android phone.',
  'usp-c4-title':'Daily UBI forever','usp-c4-desc':'Once registered, you receive a daily share of UBI payouts automatically — every day, no action required.',
  'ubi-hero-title':'UNIVERSAL BASIC INCOME POOL','ubi-hero-sub':'Accumulating — next payout distributed equally to all verified humans in:',
  'ubi-hero-desc':'Split equally among all verified humans · paid every 24h · pool resets to zero after each payout · no minimum balance required',
  'ubi-bal-lbl':'current pool balance','ubi-how-fills':'HOW THE UBI POOL FILLS UP',
  'ubi-see-above':'see countdown above','ubi-timer-above':'⏰ countdown displayed above',
  'ubi-src-swap':'20% Swap Fees','ubi-src-swap-d':'Every AEQ↔tUSD swap contributes 20% of its 0.1% fee here. More trading activity = faster pool fill.',
  'ubi-src-dem':'variable Demurrage','ubi-src-dem-d':'Idle AEQ (3+ months inactive) decays at 0.5%/month. The decayed amount enters the 40/30/20/10 split — 20% goes to UBI.',
  'ubi-src-cap':'variable Wealth Cap Overflow','ubi-src-cap-d':'Wallets exceeding 25× the fair share (25,000 AEQ) have the excess confiscated instantly. 20% flows to UBI immediately.',
  'ubi-pool-desc':'20% of swap fees + demurrage + wealth cap overflow → divided equally among all verified humans every 24 hours. Even with zero trading, demurrage and wealth cap ensure the pool always fills.',
  'pool-t-timer':'Accumulates — no timer',
  'pools4-header':'ALL FOUR REDISTRIBUTION POOLS',
  'swap-title':'🔄 Swap AEQ ↔ tUSD',
  'swap-sub':'Exchange AEQ for tUSD (a simulated test-dollar) through the native liquidity pool. A 0.1% fee applies only to swaps — ordinary AEQ transfers between people remain completely free.',
  'swap-faucet-desc':'Claim 1,000 tUSD once to pair with your AEQ — for your first liquidity deposit.',
  'swap-btn-faucet':'CLAIM TEST tUSD (once)','swap-btn-go':'🔄 SWAP',
  'swap-rate-lbl':'Rate','swap-fee-bps':'Fee','swap-out-lbl':'You receive approx.','swap-impact-lbl':'Price Impact',
  'swap-depth-lbl':'Pool Depth','swap-pool-aeq':'Pool AEQ','swap-pool-tusd':'Pool tUSD','swap-pool-price':'Price',
  'swap-pool-title':'AMM Liquidity Pool','swap-no-liquidity':'No liquidity yet','swap-details-hdr':'Swap Details',
  'swap-lp-title':'Your LP Position','swap-lp-share':'Pool Share','swap-lp-withdrawable':'Withdrawable',
  'swap-lp-youget':'You get approx.','swap-lp-pct-label':'of pool','swap-lps':'LP Shares',
  'swap-your-aeq':'Your AEQ','swap-your-tusd':'Your tUSD',
  'swap-addliq-title':'Add Liquidity','swap-addliq-desc':'Deposit AEQ and tUSD to earn 30% of all swap fees proportional to your share.',
  'swap-btn-addliq':'+ ADD LIQUIDITY','swap-btn-removeliq':'− REMOVE LIQUIDITY',
  'swap-fee-est':'Estimated fee','swap-log-hint':'// Connect wallet to swap AEQ ↔ tUSD...',
  'swap-ubi':'20% UBI','swap-validators':'40% Validators','swap-treasury':'10% Treasury',
  'amm-title':'How the AMM works','amm-text':'Automated Market Maker using the x·y=k formula. Price is determined by pool ratio. Deeper pools = lower price impact per swap.',
  'swap-pools-addr-title':'Pool Addresses','swap-priv-bar':'🔒 Non-custodial · AMM x·y=k · 0.1% fee · Instant settlement · No slippage protection needed at small sizes',
  'v7-intro-title':'What is AequitasV7?',
  'v7-intro-text':'AequitasV7 is the central smart contract of the Aequitas protocol. It defines the economic rules — the registration grant, the wealth cap, demurrage, the UBI accounting — and holds the nullifier registry that makes "one human, one registration" enforceable. It is deployed immutably on Aequitas Chain (ID 1926). Live transfers are settled by the chain layer against that rulebook, which is what makes them sub-second and gasless.',
'swap-sell-label':'Sell','swap-receive-label':'Receive',
  'guard-title':'🛡 Guardian System','guard-my-lbl':'My Guardian','guard-none':'None',
  'guard-set-lbl':'Set / Change Guardian','guard-set-hint':'Must be a registered Aequitas human · 7-day timelock · Guardian can only confirm your liveness, not access funds · Max 3 wards per guardian',
  'guard-confirm-lbl':'Confirm Alive (As Guardian)','guard-confirm-hint':'If your ward cannot access their wallet, confirm their liveness to prevent their funds moving to escrow after 910 days of inactivity.','guard-recover-btn':'🔓 RECOVER FROM ESCROW',
  'faq-title':'❓ FAQ','faq-q1':'Is my biometric data safe?','faq-a1':'Your face is captured and sent to independent matching services — that is the only way "one person, one account" can be checked at all. The images are processed and then discarded; they are not stored. What is kept is a mathematical template: encrypted, and split into shares across separately operated validators, so no validator ever holds a whole one. One honest limit, stated rather than hidden: the service that runs the comparison does still hold templates, because comparing needs them.',
  'faq-q1b':'Does registration prove I am a unique real person?','faq-a1b':'Better than a device key ever could, and not yet provable as a number. The face is compared against every existing registration by independent services that must agree, so the same person on a second phone is caught — which a device key never could. What is not yet established is the error rate: the matching threshold has not been calibrated against real captures, and that needs about 1000 impostor pairs before anyone quotes a figure.',
  'faq-q2':'Can I register with a different wallet later?','faq-a2':'No. A registration is permanently bound to one wallet address. That is deliberate: the nullifier derived from your face is spent once, so registering again to a different wallet would be a second identity for the same person.',
  'faq-q3':'What happens if I lose my phone?','faq-a3':'Your AEQ remains in your wallet — it is tied to your private key, not your phone. You can still access your wallet via MetaMask with your seed phrase. Wallet recovery is independent of the device-key registration.',
  'path-title':'Choose Your Path','path-human-title':'I am a Human','path-human-desc':'I want to register, receive 1,000 AEQ, and join the basic income network.','path-human-steps':'1. Download the Aequitas Android App<br>2. Unlock with your device\'s screen-lock (fingerprint/face/PIN)<br>3. Connect MetaMask<br>4. Receive 1,000 AEQ instantly',
  'path-node-title':'I am a Node Operator','path-node-desc':'I want to run a full node, participate in block production, and earn from the 40% validator pool.','path-node-steps':'1. Register as a human (required)<br>2. No entry point to configure — the validator addresses are built in<br>3. Deploy on Contabo/Hetzner/any VPS<br>4. Earn daily from validator pool',
  'path-dev-title':'I am a Developer','path-dev-desc':'I want to build on Aequitas, integrate the API, or contribute to the protocol.','path-dev-steps':'1. EVM-compatible JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Metrics: /metrics (Prometheus)',
  'story-flow-title':'AEQ Token Flow Diagram','story-topo-title':'Network Topology — Current State',
  'swap-price-title':'AEQ / tUSD — Live Price','swap-price-desc':'Real-time price derived from pool reserves (x·y=k). Updates every 8 seconds as new pool data arrives.','swap-price-empty':'No pool data yet — add liquidity to see the price chart.',
  'node-guide-lang-note':'This inline guide is in English. A translated PDF is available in your language using the button above.',
  'k-zkp':'ZKP System','k-hash':'Hash System','k-sybil-prot':'Sybil Protection',
  'soc-title':'💬 Social Media','soc-sub':'Announcements, the state of the chain, and the awkward questions &mdash; in public, on both.',
  'soc-x-desc':'Announcements, and what the chain is actually doing. Short form.','soc-tg-desc':'The open group: questions, node operators, and help getting registered.',
  's-validators':'Active Validators',
  'expl-heading':'Block Explorer',
},
de:{
  'x-consensus-ghostdag-knightdag':'◆ Konsens: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Vertragscode',
  'x-demurrage-is-a-holding-cost':'Schwundgeld ist eine Haltegebühr auf Geld — ein negativer Zins, der Horten teuer und Umlauf attraktiv macht. Dafür gibt es Vorbilder: Das Wörgler Experiment (Österreich, 1932) nutzte eine Schwundwährung und senkte die örtliche Arbeitslosigkeit binnen eines Jahres um 25 %. Die Österreichische Nationalbank unterband es gerade deshalb, weil es zu gut funktionierte und das Bankenmonopol bedrohte. Der Chiemgauer (Deutschland, 2003) arbeitet nach demselben Grundsatz und läuft seit über 20 Jahren erfolgreich. Aequitas setzt fortlaufendes Schwundgeld von 0,5 % im Monat um, angewandt erst nach drei Monaten Untätigkeit.',
  'x-network-consensus':'→ Netz / Konsens',
  'x-node-decentralization-roadmap':'Fahrplan zur Node-Dezentralisierung',
  'x-open-source-chain-logic':'Offener Quelltext der Kettenlogik',
  'x-phase-0-now':'Phase 0 (jetzt):',
  'x-phase-1-100-humans':'Phase 1 (ab 100 Menschen):',
  'x-phase-2-1-000-humans':'Phase 2 (ab 1.000 Menschen):',
  'x-phase-3-10-000-humans':'Phase 3 (ab 10.000 Menschen):',
  'x-protocol-mechanisms':'Protokoll-Mechanismen',
  'x-what-happens-to-aeq-when':'Was geschieht mit AEQ, wenn Menschen sterben oder dauerhaft handlungsunfähig werden? Bei Bitcoin und den meisten Kryptowährungen bedeuten verlorene Wallets dauerhaft verlorenes Geld — Schätzungen zufolge sind Millionen BTC für immer unerreichbar. Aequitas löst das über eine mehrstufige Wiederherstellung bei Untätigkeit: Zeigt eine Wallet über längere Zeit keine Aktivität, fließt ihr Guthaben schrittweise über den Grundeinkommens-Topf an die Gemeinschaft zurück, damit die tatsächlich umlaufende Menge aussagekräftig bleibt.',
  'x-what-if-someone-is-hospitalized':'Was, wenn jemand im Krankenhaus liegt, inhaftiert ist oder monatelang nicht an sein Gerät kommt? Die Vertrauensperson — ein anderer bestätigter Mensch — kann bestätigen, dass die Inhaberin noch lebt, und verhindert so, dass ihr AEQ in die Treuhand wandert. Die Vertrauensperson hat ausdrücklich keinerlei Zugriff auf Geld: Sie kann genau eine Funktion aufrufen, die die Untätigkeitsuhr zurücksetzt. Geld bewegen, ausgeben oder darauf zugreifen kann sie unter keinen Umständen.',
  'bv-bind':'🔗 Bindungs-Signatur erzeugen',
  'bv-check-d':'Der zweite Aufruf listet jeden Verifier auf und vergleicht sie: ob alle gleich viele Einschreibungen halten, ob irgendwo ein Seed fehlt und ob die Schlüssel übereinstimmen. Zeigt dein Eintrag eine Abweichung, ist es besser, das hier zu erfahren als mitten in der Registrierung eines Menschen.',
  'bv-check-t':'Nachprüfen, dass es läuft',
  'bv-desc':'Ein blockproduzierender Node sichert das <strong style="color:var(--text)">Kassenbuch</strong>. Ein Bio-Verifier sichert etwas anderes: die Zusage, dass sich <strong style="color:var(--neon)">jeder Mensch nur einmal anmeldet</strong>. Das sind getrennte Rollen — du kannst eine davon betreiben oder beide auf derselben Maschine.',
  'bv-guide-sub':'Schritt für Schritt &middot; Keine Kryptografie-Kenntnisse nötig &middot; Etwa 30 Minuten, das meiste davon Herunterladen',
  'bv-honest-d':'Dieser Teil ist Beta, und die Grenzen sind echt. Der gemeinsame Vergleich verbraucht kryptografisches Einwegmaterial, und eine Lieferung reicht derzeit für einige Dutzend Registrierungen, bevor Nachschub nötig ist — der vertrauliche Weg bewährt sich also zuerst im Kleinen, nicht bei Millionen. Der Aufwand wächst außerdem mit der Zahl der Eingeschriebenen. Wir veröffentlichen diese Zahlen, statt sie zu runden: ein System, das nach deinem Gesicht fragt, hat kein Recht darauf, unklar zu bleiben, was es kann und was noch nicht.',
  'bv-honest-t':'Wo das heute steht — unverblümt',
  'bv-need-1':'<strong style="color:var(--text)">Ein registriertes Aequitas-Konto.</strong> Dieselbe Regel wie beim Blockbauen, und aus demselben Grund: ein Mensch, ein Schlüssel. Ohne sie könnte eine einzelne Person unbemerkt ein ganzes Komitee werden.',
  'bv-need-2':'<strong style="color:var(--text)">Ein kleiner Linux-Server mit Docker.</strong> 2 GB Arbeitsspeicher genügen. Keine Grafikkarte — verglichen wird mit Arithmetik auf 64 Byte. Die Maschine, auf der schon dein Node läuft, reicht aus.',
  'bv-need-3':'<strong style="color:var(--text)">Ein Domainname mit HTTPS.</strong> Die anderen Komiteemitglieder müssen dich erreichen. Eine Unterdomain von etwas, das dir ohnehin gehört, genügt.',
  'bv-need-4':'<strong style="color:var(--text)">Erreichbar bleiben.</strong> Für eine Registrierung muss jedes Mitglied eines Komitees antworten. Ein Verifier, der oft weg ist, bremst Menschen, statt sie zu schützen.',
  'bv-need-t':'Bevor du anfängst — was du brauchst',
  'bv-s1-note':'Die private Hälfte bleibt auf deinem Server und sonst nirgends. Die öffentliche ist zum Weitergeben gedacht — mit ihr prüfen andere, dass du etwas bezeugt hast. <strong style="color:var(--text)">Dein eigener Projektions-Seed zählt:</strong> weil jeder Verifier einen anderen benutzt, lässt sich eine gestohlene Datenbank des einen nicht gegen die eines anderen halten. Geht der Seed verloren, werden deine gespeicherten Anteile bedeutungslos — sichere ihn also an einem Ort, den du kontrollierst.',
  'bv-s1-t':'Schritt 1 — Eigene Schlüssel erzeugen',
  'bv-s1-warn-d':'Zwei Verifier mit demselben Geheimnis zählen als einer, und das Komitee wäre kleiner, als es aussieht. Niemand — auch wir nicht — sollte dir je einen Schlüssel schicken.',
  'bv-s1-warn-t':'Erzeuge sie selbst. Nimm niemals Schlüssel von jemandem an.',
  'bv-s2-d':'Trage die Werte aus Schritt 1 in eine Datei ein, die nur du lesen kannst. Ein Wert je Zeile, ohne Anführungszeichen.',
  'bv-s2-note':'<strong style="color:var(--gold)">Lass ALLOW_REAL_BIOMETRIC_DATA auf false</strong>, bis du die Datenschutzhinweise gelesen hast. So nimmt dein Verifier am Netz und an Test-Einschreibungen teil, ohne je Daten eines echten Menschen zu speichern. Das ist der richtige Anfang, und es eilt nicht, daran etwas zu ändern.',
  'bv-s2-t':'Schritt 2 — Die Konfigurationsdatei schreiben',
  'bv-s3-note':'Eine gesunde Antwort meldet <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> und <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. Das Erste ist die Zusage, dass keine vollständige Vorlage gespeichert wird — in einer Form, die du selbst nachprüfen kannst, statt sie zu glauben. Prüfe es jetzt und später wieder; es ist deine eigene Absicherung so gut wie die aller anderen.',
  'bv-s3-t':'Schritt 3 — Den Verifier starten',
  'bv-s4-d':'Die anderen Komiteemitglieder erreichen dich über das offene Internet, der Port darf also nicht unverschlüsselt offenliegen. Caddy holt sich das Zertifikat von selbst.',
  'bv-s4-t':'Schritt 4 — HTTPS davorsetzen',
  'bv-s5-d':'Blockproduzenten binden ihren Signierschlüssel an eine registrierte Wallet: die Wallet unterschreibt <strong style="color:var(--text)">Aequitas: authorize validator &lt;Adresse&gt;</strong>, und ohne das verweigert die Kette den Platz. Der Knopf unten erzeugt genau diese Signatur — für die Validator-Rolle. <strong style="color:var(--text)">Für einen Verifier-Schlüssel gibt es diese Bindung noch nicht.</strong> Seine öffentliche Hälfte wird außerhalb der Kette eingesammelt (Schritt 6) und in die Liste aufgenommen, gegen die jeder Proof-Server prüft. Nichts auf der Kette knüpft ihn an einen Menschen. Solange das fehlt, zählt ein Komitee Maschinen und keine Menschen, und ein Betreiber könnte mehrere halten. Das sagen wir lieber hier, als die Zahl stärker aussehen zu lassen, als sie ist.',
  'bv-s5-t':'Schritt 5 — Was einen Schlüssel an einen Menschen bindet (und was noch nicht)',
  'bv-s6-d':'Schicke die <strong style="color:var(--text)">öffentliche</strong> Hälfte aus Schritt 1 zusammen mit deiner HTTPS-Adresse an die Gruppe. Sie kommt in die Liste, gegen die jeder Proof-Server prüft, und von da an zählen deine Bezeugungen zum Quorum. In diesem Schritt verlässt nichts Geheimes deine Maschine — genau dafür gibt es die Teilung: die private Hälfte bleibt für immer bei dir, und die öffentliche ist ohne sie nutzlos.',
  'bv-s6-t':'Schritt 6 — Deinen öffentlichen Schlüssel bekanntgeben',
  'bv-status-d':'Der Quelltext des Verifiers ist <strong style="color:var(--text)">noch nicht öffentlich</strong>, die Schritte unten kann heute also nicht jeder gehen. Sie stehen trotzdem schon hier, weil ein Entwurf prüfbar sein sollte, bevor er läuft, nicht danach. Wer einen betreiben möchte, meldet sich in der Telegram-Gruppe von der Startseite. Dieses Repository zu öffnen ist das, was den Leitfaden von einem Plan zu einer Einladung macht — und es ist das Nächste, was wir euch schulden.',
  'bv-status-t':'Stand: geschlossene Beta — bitte vor dem Anfangen lesen',
  'bv-title':'Oder Bio-Verifier werden — die Rolle, die Einmaligkeit dezentral macht',
  'bv-what-d':'Ein Gesicht wird dir nie geschickt. Deine Maschine speichert einen <strong style="color:var(--text)">additiven Anteil</strong> eines 64-Byte-Auszugs: für sich allein ist er von Zufallsrauschen nicht zu unterscheiden, und keine Rechnung, die du darauf anwenden kannst, holt daraus ein Gesicht zurück. Verglichen wird gemeinsam mit den anderen Mitgliedern deines Komitees, und keiner von euch erfährt etwas außer der Antwort — <em>Duplikat: ja oder nein</em>. Das ist keine Zusage über unsere guten Absichten, sondern eine Eigenschaft der Rechnung.',
  'bv-what-t':'Was du halten würdest — und was du nie zu sehen bekommst',
  'bv-why-d':'Eine Registrierung wird erst angenommen, wenn <strong style="color:var(--text)">mehrere verschiedene Verifier</strong> sie bezeugt haben. Ein gestohlener Schlüssel genügt also nicht — ein Angreifer braucht ein ganzes Komitee. Und weil <strong style="color:var(--neon)">ein Mensch genau einen Validator-Schlüssel halten darf</strong>, heißt ein Komitee zu kaufen: so viele Menschen zu sein. Bei 100 Verifiern hat jemand, der 10 davon kontrolliert, weniger als eine Chance von 1 zu 1.000, ein ganzes Dreier-Komitee zu besitzen. Jede Person, die dazukommt, verkleinert diese Zahl. Das ist die eine Stelle, an der die Zahl der Teilnehmer die Sicherheit <em>ist</em>. <strong style="color:var(--text)">Diese Rechnung setzt einen Menschen je Verifier-Schlüssel voraus.</strong> Bei der Blockproduktion erzwingt die Kette das; für Verifier-Schlüssel noch nicht (Schritt 5). Bis dahin ist die Zahl oben eine Obergrenze der Sicherheit, keine Messung.',
  'bv-why-t':'Warum jeder weitere Verifier das Netz schwerer zu unterwandern macht',
  'x-0-1-split-40-30':'0,1 % · Aufteilung 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 Menschen. Gleitende Vermögensobergrenze 5x &#8594; 25x. Aufbauphase.',
  'x-0-8211-2-years':'0 &#8211; 2 Jahre',
  'x-0-perfect-equality':'0 = vollkommene Gleichheit',
  'x-1-000-aeq-minted':'+1.000 AEQ geschöpft',
  'x-1-000-aeq-per-human':'1.000 AEQ je Mensch',
  'x-1-000-aeq-will-be':'1.000 AEQ werden automatisch gutgeschrieben',
  'x-10-000-8211-1m-humans':'10.000 &#8211; 1 Mio. Menschen. Mindestens 10 Nodes. Vollständig dezentral.',
  'x-100-8211-10-000-humans':'100 &#8211; 10.000 Menschen. Feste Obergrenze 25x. Offener Node-Zugang.',
  'x-100-maximum-concentration':'100 = maximale Konzentration',
  'x-1m-humans-global-ubi-at':'Über 1 Mio. Menschen. Globales Grundeinkommen im großen Maßstab. Gini-Ziel &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Liquidität LP 30 %',
  'x-9679-treasury-10':'&#9679; Rücklage 10 %',
  'x-9679-ubi-pool-20':'&#9679; Grundeinkommens-Topf 20 %',
  'x-9679-validators-40':'&#9679; Validatoren 40 %',
  'x-active-validators':'Aktive Validatoren',
  'x-add-aequitas-chain-to-metamask':'Füge die Aequitas-Chain zu MetaMask hinzu, um dein AEQ-Guthaben zu sehen, Überweisungen zu senden und direkt aus Browser oder Handy-Wallet mit dem V7-Vertrag zu arbeiten.',
  'x-admin-keys-or-governance-votes':'Admin-Schlüssel oder Abstimmungen',
  'x-aeq-activity':'AEQ-AKTIVITÄT',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'Aequitas-BlockDAG — nichts wird verworfen',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Aequitas-Chain (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas setzt das mathematisch um. Jeder bestätigte Mensch erhält genau 1.000 AEQ &#8212; Milliardär oder Kleinbauer, ohne Ausnahme. Vier Umverteilungsmechanismen sorgen dafür, dass Ungleichheit sich nicht unbegrenzt aufbauen kann. Der Gini-Koeffizient wird in Echtzeit auf der Kette geführt.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — Kette des Menschlichkeitsnachweises',
  'x-android-apk-direct-download':'Android-APK · Direkter Download',
  'x-architecture':'Aufbau',
  'x-automatic-on-chain':'automatisch auf der Kette',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (gerichteter azyklischer Graph)',
  'x-blockdag-parallel-production':'BlockDAG · Parallele Produktion',
  'x-blockdag-proof-of-humanity':'BlockDAG + Menschlichkeitsnachweis',
  'x-blue-score':'„blauer Wert"',
  'x-both-blocks-are-kept-ghostdag':'Beide Blöcke bleiben erhalten — GHOSTDAG führt den gleichzeitigen mit ein und zählt ihn weiterhin für die verbindliche Reihenfolge.',
  'x-canonical-winner':'verbindlicher Gewinner',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Vergleichbar mit den USA (0,41) oder Frankreich (0,32). Im Bereich der meisten entwickelten Volkswirtschaften. Die Umverteilung flacht die Kurve spürbar ab.',
  'x-confirm-ward-is-alive':'✓ BESTÄTIGEN, DASS DIE PERSON LEBT',
  'x-core-technology':'Kerntechnik',
  'x-daily-ubi-returns-to-all':'tägliches Grundeinkommen fließt an alle bestätigten Menschen zurück',
  'x-demurrage-0-5-mo':'Schwundgeld (0,5 %/Monat)',
  'x-device-bound-zk-proof-one':'Gerätegebundener ZK-Beweis · eine Registrierung je Gerät',
  'x-diagonal-line-perfect-equality':'Diagonale = vollkommene Gleichheit',
  'x-disconnect-wallet':'⊘ WALLET TRENNEN',
  'x-distinct-proposers-recent-blocks':'Verschiedene Erzeuger, jüngste Blöcke',
  'x-distribution':'📈 Verteilung',
  'x-elliptic-curve':'Elliptische Kurve',
  'x-entire-distribution':'gesamte Verteilung',
  'x-evm-compatible':'EVM-kompatibel',
  'x-fill-ghostdag-verdict-thin-ring':'Füllung = GHOSTDAG-Urteil · dünner Ring = Erzeuger · eine Spalte je Höhe. Für Einzelheiten über einen Block fahren.',
  'x-generate-node-binding-signature':'🔗 Bindungs-Signatur erzeugen',
  'x-run-a-coordinator':'🚪 Coordinator betreiben',
  'co-title':'Oder einen Coordinator betreiben — die Tür, durch die jeder Mensch geht',
  'co-desc':'Der Coordinator ist die Stelle, an der ein Mensch ankommt: er gibt die Aufgabe aus, fächert die Aufnahme an die Verifier aus, zählt deren Stimmen und stellt die Bescheinigung aus, auf die die Kette prägt. Lange gab es genau einen — also lief jede Registrierung im Netz über eine einzige Maschine. Nicht, weil etwas gefehlt hätte, sondern weil niemand einen zweiten betrieb.',
  'co-status-t':'Status: geschlossene Beta — derselbe Vorbehalt wie beim Verifier',
  'co-status-d':'Der Coordinator liegt im selben Repository wie der Verifier, und dieses Repository ist <strong style="color:var(--text)">noch nicht öffentlich</strong>. Die Schritte unten kann heute also nicht jeder ausführen. Sie stehen trotzdem hier, aus demselben Grund: ein Entwurf soll prüfbar sein, bevor er ausgeliefert wird, nicht danach.',
  'co-power-t':'Was ein Coordinator kann — und was nicht',
  'co-power-d':'Er <strong style="color:var(--text)">kann keinen Menschen erfinden</strong>. Eine bio_hash entsteht erst, wenn mehrere verschiedene Verifier sie bezeugt haben, und deren Schlüssel hat er nicht. Was er kann: eine <strong style="color:var(--text)">bestehende</strong> bio_hash an eine Wallet binden — ein unehrlicher könnte eine Zuteilung also auf eine Adresse seiner Wahl umlenken. Das ist eine echte Befugnis, sie wächst mit jedem zusätzlichen Coordinator, und wer über Vertrauen entscheidet, sollte den Unterschied kennen.',
  'co-safe-t':'Warum ein zweiter Coordinator überhaupt gefahrlos ist',
  'co-safe-d':'Er war es nicht immer. Bis August 2026 hing die Zusage <strong style="color:var(--text)">ein Mensch, eine Registrierung</strong> an einer Redis-Sperre im Coordinator — und zwei unabhängige Coordinatoren teilen kein Redis, zwei gleichzeitige Registrierungen desselben Menschen wären beide durchgekommen. Heute prüft <strong style="color:var(--text)">jeder Verifier selbst</strong>, vor seinem eigenen Schreibvorgang, ob dieses Gesicht schon eingeschrieben ist. Die Zusage hängt an keinem gemeinsamen Dienst und keinem gemeinsamen Geheimnis mehr; ein Coordinator kann hinzukommen oder wegfallen, ohne dass sich daran etwas ändert.',
  'co-need-t':'Was du brauchst',
  'co-need-d':'Ein registriertes Aequitas-Konto — dieselbe Regel wie beim Blockbauen und beim Verifizieren: ein Mensch, ein Schlüssel. Einen Server mit Docker und einer öffentlichen HTTPS-Adresse, denn an eine unsichere Seite gibt kein Browser die Kamera heraus. Und zwei eigene Schlüssel, die du selbst erzeugst und die deine Maschine nie verlassen: einer unterschreibt deine Bescheinigungen, einer bildet Wallet-Adressen auf Marker ab.',
  'co-keys-t':'Nimm niemals einen Schlüssel von jemandem an — auch nicht von uns',
  'co-keys-d':'Zwei Coordinatoren mit demselben Signierschlüssel sind keine zwei Coordinatoren, sondern einer mit zwei Adressen — und das Quorum, das Menschen schützen soll, sähe erfüllt aus, ohne es zu sein. Erzeuge beide Schlüssel auf deiner eigenen Maschine, mit deinem eigenen Zufall, und lass keinen davon herunter.',
  'co-auth-t':'Deinen Schlüssel autorisieren — ohne Genehmigung',
  'co-auth-d':'Solange dein Schlüssel nicht autorisiert ist, weisen die Verifier alles ab, was er unterschreibt. Die Autorisierung braucht zwei Nachweise und niemandes Zustimmung: deine Wallet unterschreibt, dass ein registrierter Mensch hinter diesem Schlüssel steht, und dein Coordinator weist auf seinem eigenen Rechner nach, dass der Schlüssel wirklich seiner ist. Den ersten erzeugst du mit dem Knopf oben; den zweiten erzeugt dein Coordinator selbst. Bis August 2026 brauchtest du zusätzlich ein geteiltes Geheimnis von uns — womit dieses Geheimnis die Genehmigung <em>war</em>. Es ist weg.',
  'co-pernode-t':'Das Register ist knotenlokal, und das ist Absicht',
  'co-pernode-d':'Eine Autorisierung, die auf einem Knoten steht, wandert nicht zu den anderen — es gibt dafür weder Transaktion noch Gossip. Eine replizierte Vertrauensliste wäre genau die zentrale Instanz, ohne die dieses System gebaut ist: jeder Betreiber entscheidet selbst, wessen Bescheinigungen sein Knoten annimmt. Der Preis ist, dass deine Autorisierung an jeden Knoten muss, der sie gelten lassen soll. Die Unterschrift selbst ist übertragbar — einmal unterschreiben, überallhin senden; ein übersprungener Knoten weist dich einfach weiter ab.',
  'co-law-t':'Was du über andere Menschen erfährst — und was daraus folgt',
  'co-law-d':'Die Aufnahme läuft durch dich hindurch; du reichst sie weiter und behältst nichts. Aber du allein kennst die Zuordnung von Wallet-Adresse und Marker für die Menschen, die über dich registrieren — darum muss dein Marker-Schlüssel deiner bleiben: geteilt könnte jeder Betreiber zu jeder öffentlichen Adresse den Marker ausrechnen und nachschlagen, wessen Gesicht dazugehört. Es heißt außerdem, dass du für diese Menschen <strong style="color:var(--text)">datenschutzrechtlich Verantwortlicher</strong> wirst. Nicht wir. Auskunft, Löschung und Widerspruch laufen bei dir auf, und das ist keine Formalie.',
  'co-limit-t':'Die eine Einschränkung, die daraus folgt',
  'co-limit-d':'Löschung über die Wallet-Adresse findet nur der Coordinator, bei dem die Einschreibung entstanden ist: dein Marker hängt an deinem Schlüssel, ein anderer Coordinator leitet für dieselbe Adresse einen anderen ab. Ein „nicht gefunden" von woanders heißt deshalb „nicht hier registriert", nicht „nicht registriert" — und die Antwort sagt das auch. Der Weg über die eigene bio_hash, derjenige, der dem Menschen selbst gehört und keinen Betreiber braucht, funktioniert bei jedem Coordinator, weil diese Kennung dieselbe bleibt.',
  'x-authorize-coordinator-key':'🔑 Coordinator-Schlüssel autorisieren',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — eine verbindliche Reihenfolge aus einem verworrenen Graphen',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Gini-Koeffizient',
  'x-gini-coefficient-0-1':'Gini-Koeffizient (0–1)',
  'x-gini-index-history':'Verlauf des Gini-Index',
  'x-gini-target-scandinavian-level':'Gini-Ziel (skandinavisches Niveau)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'Groth16-ZKP (Nullwissen)',
  'x-guardian-system-8212-human-failsafe':'Vertrauensperson &#8212; menschliche Absicherung für verlorene Wallets',
  'x-hash-wallet':'Hash / Wallet',
  'x-healthier-than-most-nations-on':'Gesünder als die meisten Länder der Erde. Vergleichbar mit Skandinavien (0,27) und Deutschland (0,31). Vermögensobergrenze und Schwundgeld halten die Verteilung erfolgreich fair.',
  'x-higher-than-most-european-nations':'Höher als in den meisten europäischen Ländern — vergleichbar mit Brasilien (0,53) oder Russland. Die Umverteilung läuft mit erhöhter Stärke.',
  'x-honest-limitation':'Ehrliche Einschränkung:',
  'x-how-it-works':'Wie es funktioniert',
  'x-how-to-read-this-chart':'Wie diese Grafik zu lesen ist:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'Menschen könnten sich registrieren',
  'x-imagine-a-world-where-every':'„Stell dir eine Welt vor, in der jeder Mensch auf der Erde &#8212; gleich wo er geboren wurde, welche Sprache er spricht oder wie viel Geld seine Eltern hatten &#8212; ein gesichertes tägliches Einkommen erhält, einfach weil er ein Mensch ist. Nicht als Almosen. Als mathematisches Recht, durchgesetzt von Code, den keine Regierung und kein Konzern übergehen kann."',
  'x-inactive-escrow':'Treuhand bei Untätigkeit',
  'x-inactivity-timeline':'Zeitlauf bei Untätigkeit',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (quantensicher)',
  'x-key-protections':'Wesentliche Schutzvorkehrungen:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — Aequitas’ eigene Weiterentwicklung über GHOSTDAG mit festem K hinaus',
  'x-knightdag-secured':'· KnightDAG-gesichert',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Wie Skandinavien (~0,27)',
  'x-liquidity-pool-30':'Liquiditäts-Topf (30 %)',
  'x-loading-blocks':'Blöcke werden geladen …',
  'x-loading-topology':'Netzstruktur wird geladen …',
  'x-loading-transactions':'Überweisungen werden geladen …',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Lorenz-Kurve — AEQ-Verteilung über alle Menschen',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: zeigt das AEQ-Guthaben nach der Registrierung 0, dann unter Einstellungen → Netzwerke die Aequitas-Chain löschen und über diese Website neu hinzufügen',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: zeigt AEQ nach dem Hinzufügen 0, dann das Netzwerk löschen und mit dem Knopf oben neu hinzufügen.',
  'x-money-exists-because-people-exist':'Geld gibt es, weil es Menschen gibt. Also sollte jeder Mensch einen gleichen Anteil daran haben, allein weil er ein Mensch ist.',
  'x-money-exists-because-people-exist-2':'„Geld gibt es, weil es Menschen gibt. Nicht mehr und nicht weniger."',
  'x-most-unequal-currency-ever':'Ungleichste Währung aller Zeiten',
  'x-multi-validator-network':'Netz aus mehreren Validatoren',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: noch nicht aussagekräftig',
  'x-no-snapshots-yet-first-one':'Noch keine Aufzeichnungen — die erste entsteht nach der nächsten Grundeinkommens-Ausschüttung.',
  'x-no-stake-blockchain':'Blockchain ohne Einsatz',
  'x-node-operator-guide-pdf':'📄 Node-Betreiber-Leitfaden (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET muss ein registrierter Aequitas-Mensch sein',
  'x-one-human-one-wallet-1':'Ein Mensch = eine Wallet = 1.000 AEQ',
  'x-p2p-protocol':'P2P-Protokoll',
  'x-paid-out-daily':'täglich ausgezahlt',
  'x-permanent-on-chain':'Dauerhaft · auf der Kette',
  'x-phase-roadmap-8212-the-path':'Phasenplan &#8212; der Weg zum globalen Maßstab',
  'x-phase-transitions-are-automatic-8212':'Die Phasenübergänge erfolgen automatisch &#8212; ausgelöst durch Schwellen bei der Zahl der Menschen, durchgesetzt vom Vertrag. Keine Abstimmung, kein Admin-Schlüssel.',
  'x-planned-post-beta':'Geplant (nach der Beta)',
  'x-postgresql-persistent':'PostgreSQL (dauerhaft)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Stelle AEQ/tUSD-Liquidität bereit und erhalte 30 % aller Tauschgebühren, täglich ausgeschüttet.',
  'x-recorded-after-each-ubi-distribution':'Wird nach jeder Grundeinkommens-Ausschüttung festgehalten. Zeigt, wie sich die Gleichheit mit wachsendem Netz entwickelt. Niedriger ist besser — Ziel ist ein Gini unter 0,30.',
  'x-redistribution':'UMVERTEILUNG',
  'x-run-a-node':'⚙️ Node betreiben',
  'x-run-a-verifier':'⚙️ Verifier betreiben',
  'x-set-guardian':'🛡 VERTRAUENSPERSON FESTLEGEN',
  'x-swap-fees-0-1':'Tauschgebühren (0,1 %)',
  'x-sybil-resistance-8212-current-state':'Sybil-Abwehr &#8212; der ehrliche Stand',
  'x-the-4-redistribution-mechanisms':'Die vier Umverteilungsmechanismen',
  'x-the-core-innovation':'Der Kerngedanke',
  'x-the-matching-threshold-has-not':'Die Erkennungsschwelle ist noch nicht an echten Aufnahmen geeicht',
  'x-the-vision-8212-a-global':'Die Idee &#8212; ein weltweites Grundeinkommens-Protokoll',
  'x-the-year-is-2009-satoshi':'Wir schreiben 2009. Satoshi Nakamoto veröffentlicht Bitcoin. Zum ersten Mal kann Wert zwischen zwei beliebigen Menschen wandern, ohne Bank. Eine echte Revolution. Doch fast sofort läuft etwas schief.',
  'x-this-is-not-a-0815':'Das ist keine 08/15-Blockchain mit einem Block nach dem anderen. Aequitas betreibt einen echten BlockDAG, geordnet von GHOSTDAG — und seit 2026 gesichert durch KnightDAG, Aequitas’ eigene anpassungsfähige Weiterentwicklung davon. Von diesem Verfahren hängt am Ende jedes Guthaben, jede Auszahlung und jede Vermögensobergrenze ab, damit es eine einzige, unstrittige Geschichte gibt.',
  'x-today-beta':'Heute (Beta)',
  'x-today-this-verifies-one-device':'Heute prüft das ein Gerät, noch nicht einen einzelnen Menschen',
  'x-traditional-blockchain-wasted-work':'Herkömmliche Blockchain — verworfene Arbeit',
  'x-treasury-10':'Rücklage (10 %)',
  'x-trusted-verified-human':'vertrauenswürdiger, bestätigter Mensch',
  'x-two-validators-produce-at-once':'Zwei Validatoren erzeugen gleichzeitig → einer gewinnt, einer wird verworfen — verlorene Arbeit, und es deckelt, wie schnell das Netz sicher laufen kann.',
  'x-ubi-pool-20':'Grundeinkommens-Topf (20 %)',
  'x-validators-pool-40':'Validatoren-Topf (40 %)',
  'x-view-source-on-github':'🐙 Quelltext auf GitHub ansehen',
  'x-wealth-cap-multiplier-bootstrap-slider':'Faktor der Vermögensobergrenze — gleitend in der Aufbauphase',
  'x-wealth-cap-overflow':'Überlauf der Vermögensobergrenze',
  'x-wealth-distribution-analysis':'Auswertung der Vermögensverteilung',
  'x-what-happens-when-someone-is':'Was geschieht, wenn jemand ins Krankenhaus kommt, inhaftiert wird oder stirbt? In den meisten Krypto-Systemen sind verlorene Wallets für immer verloren. Aequitas hat eine dreistufige Wiederherstellung bei Untätigkeit.',
  'x-what-is-a-guardian':'Was ist eine Vertrauensperson?',
  'x-what-is-and-is-not':'Was privat ist und was nicht:',
  'x-what-would-a-cryptocurrency-look':'„Wie sähe eine Kryptowährung aus, wenn man sie von Grund auf so entwürfe, dass sie zu jedem Menschen fair ist?"',
  'x-why-a-normal-blockchain-isn':'Warum eine gewöhnliche Blockchain nicht genügt',
  'x-worse-than-any-country-on':'Schlechter als jedes Land der Erde (Südafrika-Rekord: 0,63). Nähert sich Bitcoin (0,85). Das Protokoll greift mit voller Stärke ein — Vermögensobergrenze und Umverteilung am Anschlag.',
  'x-year-2-180d':'Jahr 2 +180 T',
  'x-zk-device-key-proof':'ZK-Beweis über den Geräteschlüssel',
  'swap-price-flat':'Keine Geschäfte in diesem Zeitraum — der Preis hat sich nicht bewegt. Der Chart funktioniert; der Markt ist ruhig.',
  'mpc-optin-title':'Optional — Duplikatspruefung unterstuetzen (vorbereitet, noch nicht aktiv)',
  'mpc-optin-desc':'Vorbereitet, aber noch nicht im Einsatz. Dein Node kann spaeter mithelfen zu pruefen, dass sich niemand zweimal registriert, ohne je biometrische Daten zu sehen: jede beteiligte Partei haelt nur einen mathematischen Anteil jeder Vorlage — fuer sich genommen Rauschen — und sie vergleichen eine neue Aufnahme gemeinsam, sodass keine einzelne Maschine etwas rekonstruieren kann. Heute entscheidet dieser Weg nichts. Die Duplikatspruefung laeuft nicht darueber, und das Komitee ist eine feste Liste statt automatisch gezogen; wer die drei Variablen unten setzt, aendert an Registrierungen vorerst nichts.',
  'mpc-optin-note':'Die Anteilsdatei enthaelt Einmal-Zufall, den nur dein Node halten darf — niemals auf eine andere Maschine kopieren und nirgends einchecken. Sie muss derzeit vom Betreiber kommen; das ist die verbleibende zentrale Abhaengigkeit. Einen neuen Schluessel brauchst du nicht: dein Node weist sich den anderen mit demselben Signierschluessel aus, den er ohnehin fuer Bloecke benutzt.',
  'logo-sub':'MENSCHLICHKEITSNACHWEIS','live':'LIVE',
  'reg-title':'🔐 Als verifizierter Mensch registrieren',
  'reg-sub':'Tritt dem Aequitas-Netzwerk bei und erhalte dein Universelles Grundeinkommen von 1.000 AEQ. Einmalig, permanent und vollständig gebührenfrei. Keine persönlichen Daten werden jemals gespeichert.',
  'app-title':'REGISTRIERUNG NUR ÜBER ANDROID-APP',
  'app-text':'Bei der Registrierung nimmt die Kamera dein Gesicht und eine kurze Lebendigkeits-Sequenz auf. Unabhängige Vergleichsdienste prüfen, dass ein lebender Mensch davorsitzt und dass dieses Gesicht nicht bereits registriert ist; sie müssen mehrheitlich zustimmen. Ein Groth16-ZK-Beweis trägt das Ergebnis dann auf die Kette, ohne etwas über dich preiszugeben. Deine <strong style="color:var(--gold)">1.000 AEQ werden nach der Verifikation automatisch gutgeschrieben</strong>. <strong style="color:var(--gold)">Hinweis:</strong> die Vergleichsschwelle ist noch nicht an echten Aufnahmen kalibriert — siehe FAQ unten.',
  's1t':'Gesichtsaufnahme','s1d':'Die App nimmt dein Gesicht und eine kurze Lebendigkeits-Sequenz auf und schickt beides an unabhängige Vergleichsdienste. Die prüfen, dass ein lebender Mensch davorsitzt, und vergleichen das Gesicht mit allen bereits Registrierten. Die Bilder werden nach der Verarbeitung verworfen.',
  's2t':'ZK-Beweis-Erzeugung','s2d':'Ein Groth16-ZK-Beweis bindet deinen bio_hash in commitment = keccak256(bioHash‖wallet) ein, ohne ihn preiszugeben. Der Nullifier wird aus diesem Hash abgeleitet, dasselbe Gesicht kann also nicht zweimal zählen — siehe FAQ unten.',
  's3t':'Wallet verbinden','s3d':'Die App öffnet MetaMask auf dieser Seite · verbinde deine Ethereum-Wallet · der Beweis ist kryptografisch an deine Wallet-Adresse gebunden',
  's4t':'1.000 AEQ gutgeschrieben','s4d':'Registrierung auf Aequitas BlockDAG innerhalb von 1 Sekunde bestätigt · 1.000 AEQ sofort gutgeschrieben · deine Identität ist dauerhaft als verifizierter Mensch gespeichert',
  'priv-bar':'🔒 Gesichtsprüfung im Quorum · Groth16 ZKP · Bilder nach der Prüfung verworfen · Eine Registrierung pro Mensch',
  'conn-wallet':'VERBUNDENE WALLET','proof-recv':'⚡ ZK-BEWEIS EMPFANGEN','proof-hint':'Wallet verbinden um zu registrieren',
  'btn-conn':'🦊 METAMASK VERBINDEN','btn-reg':'🔐 ON-CHAIN REGISTRIEREN',
  'btn-wc':'🔗 WALLETCONNECT VERBINDEN',
  'reg-log-hint':'// Öffne die Aequitas Android App um deinen Beweis zu erstellen, dann kehre hierher zurück...',
  'reg-details':'Registrierungsdetails','k-network':'Netzwerk','k-chainid':'Chain-ID','k-grant':'UBI-Zuteilung',
  'k-fee':'Gasgebühr','free':'KOSTENLOS — vollständig gebührenfrei','k-limit':'Registrierungen','k-limit-v':'Einmal pro Mensch · permanent · unveränderlich',
  'k-bio':'Gesicht','never-stored':'Bilder werden nach der Prüfung verworfen — kein Validator hält eine ganze Vorlage',
  'k-proof':'Beweissystem','k-conf':'Bestätigung','k-conf-v':'Innerhalb von 1 Sekunde (1 Block)',
  'k-sybil':'Sybil-Schutz','k-sybil-v':'Eine Identität pro Mensch · gesichtsgebunden, Schwelle noch nicht kalibriert',
  's-height':'Blockhöhe',
  's-humans':'Verifizierte Menschen',
  's-supply':'Gesamtmenge','s-supply-sub':'Immer = Menschen × 1.000 AEQ',
  's-uptime':'Laufzeit',
  'k-chain':'Chain-Name','k-symbol':'Symbol','k-btime':'Blockzeit',
  'k-cons':'Konsens','k-storage':'Speicher','k-dec':'Dezimalstellen',
  'btn-add-mm':'+ AEQUITAS-NETZWERK HINZUFÜGEN',
  'humans-title':'Verifizierte Menschen auf der Aequitas Chain',
  'h-what':'Was ist ein verifizierter Mensch?','h-what-t':'Ein verifizierter Mensch ist eine Wallet-Adresse, für die nachgewiesen ist, dass sie zu jemandem gehört, dessen Gesicht nicht bereits registriert ist. Unabhängige Vergleichsdienste müssen mehrheitlich zustimmen, und auf die Kette gelangt nur ein Groth16-ZK-Beweis — kein Bild und keine Vorlage. <strong style="color:var(--gold)">Bis zum 23.08.2026 wurde damit ein Gerät verifiziert, nicht ein Mensch; das ist nicht mehr so.</strong>',
  'h-zkp':'Zero-Knowledge-Beweissystem','h-zkp-t':'Aequitas verwendet Groth16 auf BN128 — dieselbe Kurve wie Ethereum und Zcash. ~200 Bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). Der Nullifier ist an dieses Gerät gebunden: Telefonverlust erzeugt auf diesem Gerät keine zweite Identität, ein anderes Gerät kann sich aber weiterhin separat registrieren.',
  'h-sybil':'Sybil-Resistenz — Aktueller Stand','h-sybil-t':'Der Nullifier wird aus dem bio_hash deines Gesichts abgeleitet, dasselbe Gesicht kann also nicht zweimal registriert werden — auch nicht über Geräte hinweg, was ein Geräteschlüssel nie konnte. Worauf das ruht, ist eine Vergleichsschwelle, die noch nicht an echten Aufnahmen kalibriert wurde: die Kryptografie ist exakt, die Biometrie darunter eine Messung mit unbezifferter Fehlerrate.',
  'h-global':'Globale finanzielle Inklusion','h-global-t':'Kein Bankkonto, keine Kreditkarte, keine Vorerfahrung mit Kryptowährungen nötig. Nur ein Android-Smartphone mit Kamera. Aequitas ist so entworfen, dass es für jeden Menschen auf der Erde zugänglich ist.',
  'h-bio-hw':'Identitätsverifikation — Roadmap','h-bio-hw-t':'Heute (Beta): eine Gesichtsprüfung ueber unabhängige Vergleichsdienste, die mehrheitlich zustimmen müssen. Ihre Schwelle ist noch nicht an echten Aufnahmen kalibriert — dafür braucht es rund 1000 Impostor-Paare, bevor irgendeine Zahl genannt wird. Geplant: genau diese Kalibrierung und eine Duplikatsprüfung, bei der kein Dienst eine ganze Vorlage hält.',
  'reg-humans':'Registrierte Menschen','h-desc':'Jede Adresse unten gehört zu einem Menschen, dessen Gesicht von unabhängigen Diensten gegen alle bestehenden Registrierungen geprüft, per ZK-Beweis belegt und mit genau 1.000 AEQ gutgeschrieben wurde. Das Register ist permanent, unveränderlich und on-chain. Was die Vergleichsschwelle heute garantiert und was nicht, steht in den FAQ.',
  'no-humans':'Noch keine Menschen registriert.\n\nLade die Aequitas Android App herunter und sei der erste Mensch auf der Chain!',
  'reg-stats':'Registrierungsstatistiken','total-humans':'Gesamtmenschen',
  'idx-title':'Aequitas-Index — Echtzeit-Wirtschaftsgleichheits-Score',
  'idx-desc':'Der Aequitas-Index wird aus dem <strong style="color:var(--teal)">Gini-Koeffizienten</strong> abgeleitet — dem internationalen Standard zur Messung wirtschaftlicher Ungleichheit, genutzt von Weltbank, OECD und UN. Er erfasst die vollständige Bilanzverteilung aller verifizierten Menschen gleichzeitig. <strong style="color:var(--neon)">0 = perfekte Gleichheit</strong> (jede Wallet hält gleich viel AEQ). <strong style="color:var(--red)">100 = totale Konzentration</strong> (eine Wallet hält alles). Bitcoin-Gini ≈ 0,85 (Index 85) · Südafrika (Weltrekord) ≈ 0,63 · Skandinavien ≈ 0,27 · Aequitas-Langzeitziel: Gini unter 0,30 (Index unter 30) — vergleichbar mit den gleichheitsstärksten Industrieländern, automatisch durchgesetzt durch den Vermögensobergrenze-Mechanismus.',
  'gini-what-title':'Was ist der Gini-Koeffizient?',
  'gini-what-text':'Entwickelt vom italienischen Statistiker Corrado Gini (1912). Misst die Vermögensverteilung durch Vergleich mit einer perfekt gleichen Verteilung — visualisiert als Lorenz-Kurve. Skala: 0 (alle halten gleich viel) bis 1 (eine Person hält alles). Genutzt von Weltbank, OECD, UN. Referenzwerte: Bitcoin ≈ 0,85 · Südafrika (Weltrekord) ≈ 0,63 · USA ≈ 0,41 · Deutschland ≈ 0,31 · Schweden ≈ 0,27 · Aequitas-Langzeitziel: Gini unter 0,30 — vergleichbar mit Skandinavien und Deutschland, durchgesetzt durch den Vermögensdeckel (Bootstrap: gleitender Deckel 5×→25× pro Mensch).',
  'gini-calc-title':'Wie wird der Aequitas-Index berechnet?',
  'gini-calc-text':'Alle AEQ-Salden verifizierter Menschen werden erfasst. Die Formel berechnet die mittlere absolute Differenz zwischen allen Saldo-Paaren, normiert durch Bevölkerungsgröße im Quadrat (n²) und Durchschnittssaldo (x̄). Ergebnis 0–1 multipliziert mit 100 = Aequitas-Index. Aktualisiert On-Chain nach jeder Registrierung, jedem monatlichen Demurrage-Lauf, jeder Pool-Ausschüttung und jedem Vermögensobergrenze-Ereignis — via Keeper-Aufruf updateGini().',
  'gini-why-title':'Warum Gini — und nicht eine einfachere Kennzahl?',
  'gini-why-text':'Ein "Reich-Arm-Verhältnis" ist leicht manipulierbar: 10.000 Wallets könnten eine geringe Spanne zeigen, aber 90% des AEQ in 100 Händen halten — Gini erkennt das, ein Verhältnis nicht. Der Koeffizient erfasst die vollständige Verteilung aller verifizierten Menschen in einer einzigen prüfbaren Zahl. Aequitas veröffentlicht diese On-Chain — transparent, manipulationssicher, weltweit verifizierbar. Sie ist das Hauptsignal für automatische Phasenübergänge, Vermögensobergrenze-Kalibrierung und Umverteilungsintensität. Kein Mensch kann den Index-Wert oder die von ihm ausgelösten Mechanismen überschreiben.',
  'curr-idx':'Aktueller Index','bar-0':'0 — Perfekte Gleichheit','bar-100':'100 — Max. Ungleichheit','wcap-lbl':'Aktuelle Vermögensobergrenze:','wcap-mult':'Multiplikator:','wcap-avg':'Fair Share:',
  'gini':'Gini-Koeffizient','gini-desc':'0 = gleich · 1 = ungleich',
  'supply-desc':'Immer = Menschen × 1.000 AEQ',
  'phase':'Protokollphase','phase-desc':'Automatisch nach Menschenanzahl',
  'humans-desc':'Gesichtsgeprüfte Registrierungen',
  'pools-title':'Umverteilungspools',
  'pools-desc':'Jede Swap-Gebühr, Demurrage-Belastung und Vermögensobergrenze-Überschuss wird automatisch auf vier Pools aufgeteilt. Keine manuelle Eingriffe. Alle Pools zahlen täglich aus.',
  'vel-pool':'Validatoren-Pool','vel-pool-desc':'40% aller Gebühren → Node-Betreiber die das Netzwerk sichern',
  'liq-pool':'Liquiditäts-Pool','liq-pool-desc':'30% aller Gebühren → Liquiditätsanbieter, proportional zu LP-Anteilen',
  'ubi-pool':'UBI-Pool','ubi-pool-desc':'20% aller Gebühren → alle verifizierten Menschen gleichmäßig, alle 24 Stunden',
  'treasury':'Schatzkammer','treasury-desc':'10% aller Gebühren → Protokollentwicklung und -wartung',
  'phases-title':'Protokollphasen',
  'phases-desc':'In Phase 0 verwendet die Vermögensobergrenze einen Bootstrap-Multiplikator: max(5, min(N, 25))× Durchschnittsguthaben. Mit 1–4 Menschen: 5× Durchschnitt. Jeder neue Mensch erhöht um 1×. Ab 25+ Menschen: dauerhaft auf 25× fixiert. Phase 1+ behält 25× fest. Alle Übergänge erfolgen automatisch — kein Governance-Vote, kein Admin-Key.',
  'p0':'Bootstrap · &lt;100 Menschen · Vermögensobergrenze: max(5,min(N,25))× Durchschnitt · Gleitet 5×→25× bis zum 25. Menschen · Derzeit aktiv',
  'p1':'Wachstum · 100–10.000 Menschen · Vermögensobergrenze: 25× Fair Share = 25.000 AEQ',
  'p2':'Stabilität · 10.000–1M Menschen · Vermögensobergrenze: 25× Fair Share = 25.000 AEQ',
  'p3':'Reife · 1M+ Menschen · Vermögensobergrenze: 25× Fair Share = 25.000 AEQ',
  'wealth-cap-explain':'Die Vermögensobergrenze in Phase 0 (Bootstrap) verwendet max(5, min(N, 25))× Durchschnittsguthaben, wobei N = registrierte Menschen. 1–4 Menschen: 5× Durchschnitt. Jeder neue Mensch erhöht um 1×. Ab 25+ Menschen: dauerhaft 25×. Die Obergrenze skaliert stets mit dem Live-Durchschnittsguthaben.',
  'demurrage-title':'Demurrage — Anreiz zum Zirkulieren',
  'demurrage-desc':'Aequitas implementiert einen Demurrage-Mechanismus inspiriert von historischen Komplementärwährungen. Inaktive AEQ-Guthaben verlieren langsam an Wert um Hortung zu entmutigen.',
  'dem-rate-k':'Verfallsrate','dem-rate-v':'0,5% pro Monat (kontinuierlich, nicht gestuft)',
  'dem-grace-k':'Schonfrist','dem-grace-v':'3 Monate Inaktivität bevor der Verfall beginnt',
  'dem-reset-k':'Uhr-Reset','dem-reset-v':'Jede Überweisung, Swap oder Liquiditätsaktion setzt den Timer zurück',
  'dem-dest-k':'Verfallenes AEQ geht an','dem-dest-v':'Umverteilungspools (40/30/20/10 Aufteilung)',
  'dem-warn-k':'Warnsystem','dem-warn-v':'14-Tage-Hinweis (einmal) + 7-Tage-Wiederholung bei jedem Login',
  'story-title':'Die Geschichte von Aequitas — Warum es das gibt',
  'nodes-title':'Aktive Nodes — Aktuelle Netzwerktopologie',
  'nodes-desc':'Das Aequitas-Netzwerk betreibt derzeit mehrere geografisch verteilte Nodes (aktuelle Anzahl oben). Alle nehmen an Blockproduktion, Statussynchronisation und API-Bereitstellung teil. Sie kommunizieren per libp2p und synchronisieren Blockzustände via HTTP. Das Netzwerk ist für zusätzliche Nodes ausgelegt — jeder Betreiber kann beitreten.',
  'run-node-title':'Eigenen Node betreiben — Das Netzwerk sichern',
  'run-node-desc':'Jeder registrierte Mensch kann einen Aequitas-Node betreiben — kein Stake, keine Bewerbung, keine Genehmigung von uns. Ein Mensch, ein Validator-Schlüssel: ein Node, dessen NODE_OPERATOR_WALLET kein registrierter Mensch ist, wird mit HTTP 403 abgewiesen — sonst könnte eine einzelne Person unbemerkt der ganze Validatorensatz werden. Nodes nehmen an der Blockproduktion teil und validieren die Menschenregistrierung. Node-Betreiber erhalten täglich einen Anteil der Protokollgebühren über den Validators-Pool (40% aller Swap-Gebühren).',
  'bootstrap-title':'Neuen Node verbinden','bootstrap-desc':'Für einen eigenen Aequitas-Node ist kein Einstiegspunkt zu konfigurieren — die Validator-Adressen sind fest eingebaut. Dein Node registriert sich automatisch, synchronisiert den vollständigen Chain-Zustand und nimmt an der Blockproduktion teil. PRIMARY_NODE_URL nur setzen, wenn du bewusst einen bestimmten Einstiegspunkt festlegen willst.',
  'tech-title':'Technische Spezifikationen','mm-config':'MetaMask-Konfiguration',
  'k-lang':'Sprache','k-src':'Quellcode','evm-yes':'Ja — JSON-RPC /rpc · MetaMask-kompatibel',
  'proto-label':'Aequitas V7 Protokoll — Technische Dokumentation',
  'ca-title':'Contract- & Netzwerk-Adressen','ca-text':'Chain: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier (Groth16 On-Chain-Verifier): 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Haupt-Contract): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 legt die Regeln der Aequitas-Wirtschaft fest und führt das Register, das sie durchsetzbar macht: jeden je beanspruchten Nullifier, jede Registrierung, die Vermögensobergrenze und die Demurrage-Formel. Der Contract ist unveränderlich — kein Admin-Schlüssel, kein Upgrade-Proxy, keine Governance-Abstimmung kann eine Zeile davon ändern. Ausgeführt wird ein echter Transfer allerdings vom Chain-Layer: der Knoten fängt den ERC-20-Aufruf ab, bevor er die EVM erreicht, und verbucht ihn in seinem eigenen Hauptbuch — genau das macht Transfers sekundenschnell und gasfrei. Der Contract ist das Regelwerk und das Register; die Chain ist die Maschine, die beides ausführt, und ihr Quelltext ist offen.<br><br>Der BioVerifier-Contract empfängt Groth16-Zero-Knowledge-Beweise die vollständig auf dem Android-Gerät des Nutzers erzeugt werden. Er verifiziert mathematisch on-chain in ~10 ms dass der eingereichte Nullifier korrekt aus einem Geheimnis des Registrierenden abgeleitet wurde, und die Kette weist jeden bereits gesehenen Nullifier ab — ohne jemals seinen Namen, seine Identität oder seine biometrischen Daten zu erfahren. Das schließt eine zweite Anmeldung aus derselben Identitätsquelle aus; ob diese Quelle ein Mensch oder ein Gerät ist, hängt davon ab, ob der Biometrie-Modus aktiv ist. Das ist es was die gasfreie, investitionsfreie Registrierung möglich macht: Der Beweis ist das Einzige was das Gerät je verlässt.<br><br>Genau diese Kombination ist das Neue: Die Regeln und das Register für „ein Mensch, eine Registrierung\" liegen in einem Contract, den niemand umschreiben kann — weder der Betreiber noch ein Unternehmen noch eine Regierung — und der Code, der sie ausführt, ist offen und aus diesem Repository reproduzierbar. Alles davon ist von jedem überprüfbar. Vertrauen erfordert weiterhin der Betrieb der Knoten selbst, und der ehrliche Weg, das zu verringern, sind mehr unabhängige Validatoren — nicht ein stärkerer Satz an dieser Stelle.',
  'h-what':'Was ist ein verifizierter Mensch?','h-what-t':'Ein verifizierter Mensch ist eine Wallet-Adresse, für die nachgewiesen ist, dass sie zu jemandem gehört, dessen Gesicht nicht bereits registriert ist. Unabhängige Vergleichsdienste müssen mehrheitlich zustimmen, und auf die Kette gelangt nur ein Groth16-ZK-Beweis — kein Bild und keine Vorlage. <strong style="color:var(--gold)">Bis zum 23.08.2026 wurde damit ein Gerät verifiziert, nicht ein Mensch; das ist nicht mehr so.</strong>',
  'h-zkp':'Zero-Knowledge-Proof-System','h-zkp-t':'Aequitas verwendet Groth16 auf BN128 — dieselbe Kurve wie Ethereum und Zcash. Beweisgröße: ~200 Byte. Verifikationszeit: ~10ms. commitment = keccak256(deviceKey‖wallet). Der Nullifier ist an dieses Gerät gebunden: Telefonverlust erzeugt keine zweite Identität auf diesem Gerät, ein anderes Gerät kann sich aber weiterhin separat registrieren. Kein Schlüsselmaterial wird je serverseitig offengelegt oder gespeichert.',
  'h-sybil':'Sybil-Resistenz — Aktueller Stand','h-sybil-t':'Der Nullifier wird aus dem bio_hash deines Gesichts abgeleitet, dasselbe Gesicht kann also nicht zweimal registriert werden — auch nicht über Geräte hinweg, was ein Geräteschlüssel nie konnte. Worauf das ruht, ist eine Vergleichsschwelle, die noch nicht an echten Aufnahmen kalibriert wurde: die Kryptografie ist exakt, die Biometrie darunter eine Messung mit unbezifferter Fehlerrate.',
  'h-global':'Globale finanzielle Inklusion','h-global-t':'Kein Bankkonto, keine Kreditkarte, keine Vorerfahrung mit Kryptowährungen nötig. Nur ein Android-Smartphone mit Kamera. Aequitas ist so entworfen, dass es für jeden Menschen auf der Erde zugänglich ist.',
  'h-bio-hw':'Identitätsverifikation — Roadmap','h-bio-hw-t':'Heute (Beta): eine Gesichtsprüfung ueber unabhängige Vergleichsdienste, die mehrheitlich zustimmen müssen. Ihre Schwelle ist noch nicht an echten Aufnahmen kalibriert — dafür braucht es rund 1000 Impostor-Paare, bevor irgendeine Zahl genannt wird. Geplant: genau diese Kalibrierung und eine Duplikatsprüfung, bei der kein Dienst eine ganze Vorlage hält.',
  'poa-title':'1. LEBENSNACHWEIS — Inaktive Guthaben-Rückgewinnung','poa-text':'<p>Was passiert mit AEQ wenn Menschen sterben oder dauerhaft handlungsunfähig werden? Bei Bitcoin und den meisten Kryptowährungen bedeuten verlorene Wallets dauerhaft verlorene Menge. Aequitas löst dies durch ein mehrstufiges Inaktivitäts-Rückgewinnungssystem: Wenn eine Wallet über einen längeren Zeitraum keine Aktivität zeigt, wird ihr Guthaben schrittweise über den UBI-Pool zur Gemeinschaft zurückgeführt.</p>',
  'poa-box':'Jahr 0–2: Normale Nutzung — keine Einschränkungen<br>Jahr 2: Warnung 1 — Guardian kann im Namen antworten<br>Jahr 2 + 60 Tage: Warnung 2 — steigende Dringlichkeit<br>Jahr 2 + 120 Tage: Warnung 3 — letzte Benachrichtigung<br>Jahr 2 + 180 Tage (insgesamt 2,5 Jahre, Tag 910): AEQ in persönliches TREUHANDKONTO verschoben (noch rückgewinnbar)<br>Jahr 4 (Tag ~1460): Bei weiter Inaktivität — Treuhand an UBI-Pool freigegeben',
  'guard-title':'2. GUARDIAN-SYSTEM — Menschliche Absicherung','guard-text':'<p>Was wenn jemand hospitalisiert, inhaftiert oder anderweitig monatelang nicht in der Lage ist auf sein Gerät zuzugreifen? Das Guardian-System erlaubt einer vertrauenswürdigen Person — einem anderen verifizierten Menschen — zu bestätigen dass der Wallet-Inhaber noch lebt, wodurch verhindert wird dass sein AEQ ins Treuhandkonto verschoben wird. Der Guardian hat strikt null finanziellen Zugang: Er kann nur eine einzige Funktion aufrufen die den Inaktivitätstimer zurücksetzt. Er kann unter keinen Umständen Gelder verschieben, ausgeben oder darauf zugreifen.</p>',
  'guard-box':'1 Guardian pro Mensch · muss ein verifizierter Mensch auf Aequitas sein<br>Guardian kann NUR confirmAlive() aufrufen — null Transaktionsrechte<br>Guardian KANN KEINE Gelder verschieben, AEQ übertragen oder auf die Wallet zugreifen<br>Maximal 3 Schutzbefohlene pro Guardian (verhindert Zentralisierung des Vertrauens)<br>7-Tage-Zeitsperre bei Guardian-Zuweisung (verhindert erzwungene Zuweisung)<br>Keine zirkulären Guardian-Beziehungen erlaubt',
  'dem-title':'3. DEMURRAGE — Anti-Hortungs-Mechanismus',
  'dem-box':'Wird nur auf den Anteil über deinem fairen Anteil erhoben — ein Guthaben auf oder unter dieser Grenze verfällt nie<br>Rate: 0,5% pro Monat nach 3 Monaten Inaktivität (kontinuierlich, nicht gestuft)<br>Uhr setzt sich automatisch zurück bei jeder Überweisung, Swap oder Liquiditätsaktion<br>Verfallenes AEQ wird an die vier Pools umverteilt — niemals vernichtet<br>14-Tage-Warnung einmalig angezeigt · 7-Tage-Warnung bei jeder aktiven Sitzung wiederholt',
  'dem-text':'<p>Demurrage ist ein Haltungskosten auf Geld — ein negativer Zinssatz der Horten teuer und Zirkulation attraktiv macht. Historisches Beispiel: Das Wörgl-Experiment (Österreich, 1932) verwendete eine Demurrage-Währung und reduzierte die lokale Arbeitslosigkeit innerhalb eines Jahres um 25%. Die Österreichische Zentralbank stellte es genau deshalb ein weil es zu gut funktionierte. Der Chiemgauer (Deutschland, 2003) arbeitet nach demselben Prinzip und zirkuliert seit über 20 Jahren erfolgreich.</p>',
  'cap-title':'4. VERMÖGENSOBERGRENZE — Mathematische Fairness-Durchsetzung','cap-box':'Bootstrap-Deckel: max(5,min(N,25))× aktuelles Durchschnittsguthaben<br>1–4 Menschen: 5× · +1× pro Mensch · 25+: dauerhaft 25×<br>Gilt für ALLE Adressen außer den 4 Protokoll-Pool-Adressen<br>Überschuss-AEQ sofort weitergeleitet · Keine manuellen Eingriffe',
  'ubi-title':'5. UNIVERSELLES GRUNDEINKOMMEN — Tägliche Umverteilung','ubi-box':'Quellen des UBI-Pool-Einkommens:<br>· 20% aller Swap-Gebühren aus dem AEQ↔tUSD AMM-Pool<br>· Überschuss aus der Vermögensobergrenze-Durchsetzung<br>· Demurrage-Gebühren von inaktiven Konten<br>· Inaktive Treuhand nach 4 Jahren freigegeben<br><br>Ausschüttung: Alle 24 Stunden wird der gesamte UBI-Pool-Saldo gleichmäßig unter allen registrierten verifizierten Menschen aufgeteilt. Der Pool setzt sich auf null zurück und beginnt sofort wieder aus der laufenden Protokollaktivität aufzufüllen.',
  'inf-title':'6. KEINE ALGORITHMISCHE INFLATION — Feste Mengenformel','inf-box':'Das EINZIGE Ereignis das neues AEQ schafft: ein neuer verifizierter Mensch registriert sich.<br><br>Gesamtmenge = Verifizierte Menschen × 1.000 AEQ<br><br>Dies ist keine Richtlinie — es wird durch das Protokoll erzwungen. Kein Admin kann zusätzliches AEQ prägen, kein Governance-Votum kann die Ausgabe ändern, keine Gründer-Zuteilung wurde vorab gemint. AEQ ist die einzige Kryptowährung bei der die Gesamtmenge ausschließlich durch die Anzahl verifizierter lebender Menschen bestimmt wird.',
  'btn-download-app':'AEQUITAS APP HERUNTERLADEN',
  'swap-title':'🔄 Tausche AEQ ↔ tUSD',
  'swap-sub':'Tausche AEQ gegen tUSD (ein simulierter Test-Dollar) über den nativen Liquiditäts-Pool. 0,1% Gebühr gilt nur für Swaps — gewöhnliche AEQ-Transfers zwischen Menschen bleiben vollständig kostenlos.',
  'swap-priv-bar':'🔒 Nur 0,1% Swap-Gebühr · AEQ-zu-AEQ-Transfers kostenlos · tUSD ist eine Testwährung ohne realen Wert',
  'swap-your-aeq':'Dein AEQ','swap-your-tusd':'Dein tUSD',
  'swap-fee-est':'Protokollgebühr (0,1%)','swap-details-hdr':'Swap-Details',
  'swap-out-lbl':'Du erhältst (ca.)','swap-impact-lbl':'Preisauswirkung','swap-rate-lbl':'Wechselkurs',
  'swap-btn-go':'🔄 TAUSCHEN',
  'swap-log-hint':'// Wallet verbinden um zu tauschen...',
  'swap-no-liquidity':'Noch kein tUSD?','swap-faucet-desc':'Registrierte Menschen können einmalig Test-tUSD beanspruchen',
  'swap-btn-faucet':'💧 TEST-tUSD BEANSPRUCHEN',
  'swap-addliq-title':'Liquidität bereitstellen','swap-addliq-desc':'Sei der Erste der einzahlt — dein Verhältnis legt den Startpreis fest.',
  'swap-btn-addliq':'💧 LIQUIDITÄT HINZUFÜGEN',
  'swap-lp-title':'Deine LP-Position','swap-lp-share':'Pool-Anteil','swap-lp-withdrawable':'Auszahlbar',
  'swap-lp-pct-label':'% deiner Position','swap-lp-youget':'Du erhältst','swap-btn-removeliq':'🔥 LIQUIDITÄT ENTFERNEN',
  'swap-pool-title':'AEQ / tUSD — Pool-Status',
  'swap-pool-aeq':'AEQ-Reserve','swap-pool-tusd':'tUSD-Reserve','swap-pool-price':'Spot-Preis',
  'swap-depth-lbl':'Pool-Zusammensetzung',
  'amm-title':'x × y = k — Konstantprodukt-AMM',
  'amm-text':'Wenn du AEQ gegen tUSD tauschst, wächst die AEQ-Reserve und die tUSD-Reserve schrumpft — ihr Produkt bleibt immer gleich k. Jeder Swap bewegt den Preis. Größere Swaps relativ zur Pool-Größe führen zu größerer Preisauswirkung. Die 0,1% Gebühr wird vor Anwendung der Formel abgezogen — so verdient der Pool an jedem Trade.',
  'swap-fee-bps':'Swap-Gebühr',
  'swap-pools-addr-title':'Tokenomics-Pool-Adressen',
  'swap-validators':'Validatoren (40%)','swap-lps':'Liquiditätsanbieter (30%)','swap-ubi':'UBI-Pool (20%)','swap-treasury':'Schatzkammer (10%)',
  'ubi-hero-title':'UNIVERSELLES GRUNDEINKOMMEN — UBI-POOL',
  'ubi-hero-sub':'Akkumuliert — nächste Ausschüttung gleichmäßig an alle verifizierten Menschen in:',
  'ubi-bal-lbl':'aktuelles Pool-Guthaben',
  'ubi-hero-desc':'Gleichmäßig unter allen verifizierten Menschen aufgeteilt · alle 24h ausgezahlt · Pool setzt auf null zurück · kein Mindestguthaben nötig',
  'ubi-how-fills':'Wie der UBI-Pool sich füllt',
  'ubi-src-swap':'Swap-Gebühren','ubi-src-swap-d':'Jeder AEQ↔tUSD-Swap trägt 20% seiner 0,1% Gebühr bei. Mehr Handelsaktivität = schnelleres Auffüllen.',
  'ubi-src-dem':'Demurrage','ubi-src-dem-d':'Inaktives AEQ (3+ Monate) verfällt mit 0,5%/Monat. Der verfallene Betrag geht in die 40/30/20/10-Aufteilung — 20% an UBI.',
  'ubi-src-cap':'Vermögensobergrenze-Überschuss','ubi-src-cap-d':'Wallets die den Vermögensdeckel (max(5,min(N,25))× Durchschnitt) überschreiten werden sofort gekappt. 20% fließt direkt an UBI.',
  'pools4-header':'Alle vier Umverteilungs-Pools',
  'vel-pool-desc':'Node-Betreiber die Blöcke produzieren, ZK-Registrierungen validieren und den BlockDAG sichern. Täglich ausgezahlt proportional zur Blockproduktion.',
  'liq-pool-desc':'Anbieter von AEQ/tUSD-Liquidität erhalten 30% aller Gebühren proportional zu ihrem LP-Anteil. Tiefere Liquidität = geringere Preisauswirkung für alle Nutzer.',
  'ubi-pool-desc':'20% der Swap-Gebühren + Demurrage + Vermögensobergrenze-Überschuss → gleichmäßig unter allen verifizierten Menschen alle 24 Stunden. Auch ohne Trading füllt sich der Pool durch Demurrage und Vermögensobergrenze.',
  'treasury-desc':'Protokollentwicklung, Infrastruktur, Sicherheitsprüfungen und zukünftige Upgrades. Vollständige On-Chain-Transparenz.',
  'ubi-see-above':'siehe Countdown oben','ubi-timer-above':'⏰ Countdown oben angezeigt','pool-t-timer':'Akkumuliert — kein Timer',
  'usp-headline':'Zum ersten Mal in der Geschichte — alle starten gleich',
  'usp-sub':'Ein Android-Smartphone genügt. Kein Bankkonto, keine Kreditkarte, keine Vorkenntnisse, keine Investition.',
  'usp-c1-title':'0,00 € Startinvestition','usp-c1-desc':'Die Registrierung ist vollständig gebührenfrei. Kein ETH, kein BNB, keine Kreditkarte. Das Protokoll übernimmt alle Transaktionskosten — du startest bei null.',
  'usp-c2-title':'1.000 AEQ für jeden Menschen','usp-c2-desc':'Millionär oder Subsistenzlandwirt — jeder erhält exakt 1.000 AEQ. Nicht mehr, nicht weniger. Gleicher Start, mathematisch garantiert.',
  'usp-c3-title':'Für alle zugänglich','usp-c3-desc':'Kein Bankkonto, keine Kreditkarte, kein Ausweis, keine zusätzliche Hardware — nur die Kamera, die dein Android-Telefon ohnehin hat.',
  'usp-c4-title':'Täglich UBI empfangen','usp-c4-desc':'Nach der Registrierung erhältst du automatisch täglich einen Anteil der UBI-Ausschüttung — jeden Tag, ohne Aktion, solange du AEQ hältst.',
  'v7-intro-title':'Was ist AequitasV7?',
  'v7-intro-text':'AequitasV7 ist der zentrale Smart Contract des Aequitas-Protokolls. "V7" steht für die 7. Hauptversion des Fairness-Contracts — das Ergebnis iterativer Designverbesserung. Er ist unveränderlich auf der Aequitas Chain (Chain ID 1926) deployed und regelt jeden Aspekt des Protokolls: Menschenregistrierung, ZK-Beweisverifizierung, Guthabenverwaltung, Vermögensobergrenze, UBI-Ausschüttung, Swap-Gebühren und alle Governance-Parameter. Kein Admin kann den Contract upgraden oder ersetzen — er ist das unveränderliche Gesetz der Aequitas-Wirtschaft.',
  'swap-sell-label':'Verkaufen','swap-receive-label':'Erhalten',
  'guard-title':'🛡 Guardian-System','guard-my-lbl':'Mein Guardian','guard-none':'Keiner',
  'guard-set-lbl':'Guardian festlegen / ändern','guard-set-hint':'Muss ein registrierter Aequitas-Mensch sein · 7-Tage-Zeitsperre · Guardian kann nur deine Lebendigkeit bestätigen, nicht auf Guthaben zugreifen · Max. 3 Schützlinge pro Guardian',
  'guard-confirm-lbl':'Lebendig bestätigen (Als Guardian)','guard-confirm-hint':'Falls dein Schützling keinen Zugang zu seiner Wallet hat, bestätige seine Lebendigkeit, um zu verhindern, dass Gelder nach 910 Tagen Inaktivität ins Escrow überführt werden.','guard-recover-btn':'🔓 AUS ESCROW ZURÜCKFORDERN',
  'faq-title':'❓ FAQ','faq-q1':'Sind meine biometrischen Daten sicher?','faq-a1':'Dein Gesicht wird aufgenommen und an unabhängige Vergleichsdienste geschickt — nur so lässt sich „ein Mensch, ein Konto" überhaupt prüfen. Die Bilder werden verarbeitet und danach verworfen, nicht gespeichert. Aufbewahrt wird eine mathematische Vorlage: verschlüsselt und in Anteile geteilt auf getrennt betriebene Validatoren, sodass kein Validator je eine ganze hält. Eine ehrliche Grenze, benannt statt verschwiegen: der Dienst, der den Vergleich rechnet, hält weiterhin Vorlagen, weil Vergleichen sie braucht.',
  'faq-q1b':'Beweist die Registrierung, dass ich ein einzigartiger echter Mensch bin?','faq-a1b':'Besser, als ein Geräteschlüssel es je konnte — und noch nicht als Zahl belegbar. Das Gesicht wird von unabhängigen Diensten, die übereinstimmen müssen, gegen alle bestehenden Registrierungen verglichen; derselbe Mensch auf einem zweiten Telefon wird also erkannt, was ein Geräteschlüssel nie konnte. Offen ist die Fehlerrate: die Vergleichsschwelle ist nicht an echten Aufnahmen kalibriert, und dafür braucht es rund 1000 Impostor-Paare, bevor jemand eine Zahl nennt.',
  'faq-q2':'Kann ich mich später mit einer anderen Wallet registrieren?','faq-a2':'Nein. Eine Registrierung ist dauerhaft an eine Wallet-Adresse gebunden. Das ist Absicht: der aus deinem Gesicht abgeleitete Nullifier wird einmal verbraucht — eine erneute Registrierung auf eine andere Wallet wäre eine zweite Identität für denselben Menschen.',
  'faq-q3':'Was passiert, wenn ich mein Handy verliere?','faq-a3':'Deine AEQ bleiben in deiner Wallet — sie sind mit deinem privaten Schlüssel verknüpft, nicht mit deinem Handy. Du kannst weiterhin über MetaMask mit deiner Seed-Phrase auf deine Wallet zugreifen. Die Wallet-Wiederherstellung ist unabhängig von der biometrischen Registrierung.',
  'path-title':'Wähle deinen Weg','path-human-title':'Ich bin ein Mensch','path-human-desc':'Ich möchte mich registrieren, 1.000 AEQ erhalten und dem Grundeinkommensnetzwerk beitreten.','path-human-steps':'1. Aequitas Android App herunterladen<br>2. Mit der Bildschirmsperre deines Geräts entsperren (Fingerabdruck/Gesicht/PIN)<br>3. MetaMask verbinden<br>4. Sofort 1.000 AEQ erhalten',
  'path-node-title':'Ich bin ein Node-Betreiber','path-node-desc':'Ich möchte einen vollständigen Node betreiben, an der Blockproduktion teilnehmen und aus dem 40%-Validator-Pool verdienen.','path-node-steps':'1. Als Mensch registrieren (erforderlich)<br>2. Kein Einstiegspunkt zu konfigurieren — die Validator-Adressen sind eingebaut<br>3. Auf Contabo/Hetzner/beliebigem VPS deployen<br>4. Täglich aus dem Validator-Pool verdienen',
  'path-dev-title':'Ich bin ein Entwickler','path-dev-desc':'Ich möchte auf Aequitas aufbauen, die API integrieren oder zum Protokoll beitragen.','path-dev-steps':'1. EVM-kompatibler JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* Endpunkte<br>4. Metriken: /metrics (Prometheus)',
  'story-flow-title':'AEQ Token-Flussdiagramm','story-topo-title':'Netzwerktopologie — Aktueller Zustand',
  'swap-price-title':'AEQ / tUSD — Live-Preis','swap-price-desc':'Echtzeit-Preis aus Pool-Reserven (x·y=k). Aktualisiert alle 8 Sekunden mit neuen Pool-Daten.','swap-price-empty':'Noch keine Pool-Daten — Liquidität hinzufügen, um das Preisdiagramm zu sehen.',
  'node-guide-lang-note':'Diese Anleitung ist auf Englisch. Eine übersetzte PDF-Version ist in deiner Sprache über den Button oben verfügbar.',
  'k-zkp':'ZKP-System','k-hash':'Hash-System','k-sybil-prot':'Sybil-Schutz',
  'soc-title':'💬 Soziale Medien','soc-sub':'Ankündigungen, der Zustand der Chain und die unbequemen Fragen &mdash; öffentlich, auf beiden.',
  'soc-x-desc':'Ankündigungen und was die Chain tatsächlich tut. Kurzform.','soc-tg-desc':'Die offene Gruppe: Fragen, Node-Betreiber und Hilfe bei der Registrierung.',
  's-validators':'Aktive Validatoren',
  'expl-heading':'Block-Explorer',
},
es:{
  'x-consensus-ghostdag-knightdag':'◆ Consenso: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Código del contrato',
  'x-demurrage-is-a-holding-cost':'La oxidación monetaria es un coste por retener dinero — un tipo de interés negativo que encarece el atesoramiento y hace atractiva la circulación. Tiene precedentes: el experimento de Wörgl (Austria, 1932) usó una moneda oxidable y redujo el paro local un 25 % en un año. El Banco Nacional de Austria lo cerró precisamente porque funcionaba demasiado bien y amenazaba el monopolio bancario. El Chiemgauer (Alemania, 2003) se basa en el mismo principio y circula con éxito desde hace más de 20 años. Aequitas aplica una oxidación continua del 0,5 % mensual, solo tras tres meses de inactividad.',
  'x-network-consensus':'→ Red / consenso',
  'x-node-decentralization-roadmap':'Hoja de ruta de descentralización de nodos',
  'x-open-source-chain-logic':'Lógica de la cadena en código abierto',
  'x-phase-0-now':'Fase 0 (ahora):',
  'x-phase-1-100-humans':'Fase 1 (más de 100 personas):',
  'x-phase-2-1-000-humans':'Fase 2 (más de 1.000 personas):',
  'x-phase-3-10-000-humans':'Fase 3 (más de 10.000 personas):',
  'x-protocol-mechanisms':'Mecanismos del protocolo',
  'x-what-happens-to-aeq-when':'¿Qué pasa con los AEQ cuando alguien muere o queda permanentemente incapacitado? En Bitcoin y la mayoría de criptomonedas, un monedero perdido significa oferta perdida para siempre — se estima que millones de BTC son inaccesibles de forma definitiva. Aequitas lo resuelve con una recuperación por inactividad en varias etapas: si un monedero no muestra actividad durante un periodo largo, su saldo vuelve poco a poco a la comunidad a través del fondo de renta básica, para que la oferta realmente en circulación siga teniendo sentido.',
  'x-what-if-someone-is-hospitalized':'¿Y si alguien está hospitalizado, encarcelado o sin acceso a su dispositivo durante meses? La persona de confianza — otra persona verificada — puede confirmar que la titular sigue viva, evitando que sus AEQ pasen a depósito. Esa persona no tiene absolutamente ningún acceso financiero: solo puede llamar a una única función que reinicia el reloj de inactividad. En ninguna circunstancia puede mover, gastar ni consultar fondos.',
  'bv-bind':'🔗 Generar firma de vinculación',
  'bv-check-d':'La segunda llamada enumera cada verificador y los compara: si todos tienen el mismo número de registros, si a alguno le falta una semilla y si las claves coinciden. Si tu entrada muestra una divergencia, es mejor enterarse aquí que en mitad del registro de alguien.',
  'bv-check-t':'Comprobar que funciona',
  'bv-desc':'Un nodo que produce bloques asegura el <strong style="color:var(--text)">libro contable</strong>. Un verificador biométrico asegura otra cosa: la promesa de que <strong style="color:var(--neon)">cada persona se registra una sola vez</strong>. Son papeles distintos: puedes ejercer uno, o ambos en la misma máquina.',
  'bv-guide-sub':'Paso a paso &middot; No hace falta saber criptografía &middot; Unos 30 minutos, la mayoría descargando',
  'bv-honest-d':'Esta parte está en beta y los límites son reales. La comparación conjunta consume material criptográfico de un solo uso, y una entrega cubre por ahora unas pocas decenas de registros antes de necesitar más: la vía confidencial se demuestra primero a pequeña escala, no en millones. El trabajo crece además con el número de personas inscritas. Publicamos estas cifras en lugar de redondearlas: un sistema que pide tu rostro no tiene derecho a ser vago sobre lo que puede y lo que todavía no.',
  'bv-honest-t':'Dónde está esto hoy — sin rodeos',
  'bv-need-1':'<strong style="color:var(--text)">Una cuenta de Aequitas registrada.</strong> La misma regla que para producir bloques, y por el mismo motivo: una persona, una clave. Sin ella, una sola persona podría convertirse sin ruido en un comité entero.',
  'bv-need-2':'<strong style="color:var(--text)">Un servidor Linux pequeño con Docker.</strong> Bastan 2 GB de memoria. Sin tarjeta gráfica: la comparación es aritmética sobre 64 bytes. La máquina donde ya corre tu nodo sirve.',
  'bv-need-3':'<strong style="color:var(--text)">Un dominio con HTTPS.</strong> Los demás miembros del comité deben poder alcanzarte. Un subdominio de algo que ya tengas es suficiente.',
  'bv-need-4':'<strong style="color:var(--text)">Mantenerte en línea.</strong> Cada miembro de un comité debe responder para que un registro termine. Un verificador ausente a menudo frena a la gente en vez de protegerla.',
  'bv-need-t':'Antes de empezar — lo que necesitas',
  'bv-s1-note':'Guarda la mitad privada en tu servidor y en ningún otro sitio. La mitad pública está pensada para compartirse: es como los demás comprueban que has atestiguado algo. <strong style="color:var(--text)">Tu propia semilla de proyección importa:</strong> como cada verificador usa una distinta, una base de datos robada a uno no puede contrastarse con la de otro. Si pierdes la semilla, tus partes almacenadas dejan de significar nada; guárdala en un lugar que controles.',
  'bv-s1-t':'Paso 1 — Genera tus propias claves',
  'bv-s1-warn-d':'Dos verificadores con el mismo secreto cuentan como uno, y el comité sería más pequeño de lo que parece. Nadie —tampoco nosotros— debería enviarte jamás una clave.',
  'bv-s1-warn-t':'Genéralas tú mismo. No aceptes nunca claves de nadie.',
  'bv-s2-d':'Pon los valores del paso 1 en un archivo que solo tú puedas leer. Un valor por línea, sin comillas.',
  'bv-s2-note':'<strong style="color:var(--gold)">Deja ALLOW_REAL_BIOMETRIC_DATA en false</strong> hasta que hayas leído las notas de protección de datos. Con eso desactivado, tu verificador se une a la red y participa en registros de prueba sin almacenar jamás datos de una persona real. Es la forma correcta de empezar, y no hay prisa por cambiarlo.',
  'bv-s2-t':'Paso 2 — Escribe el archivo de configuración',
  'bv-s3-note':'Una respuesta sana indica <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> y <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. Lo primero es la afirmación de que no se guarda ninguna plantilla completa, en una forma que puedes comprobar tú mismo en lugar de creerla. Compruébalo ahora y también más adelante: es tu propia garantía tanto como la de los demás.',
  'bv-s3-t':'Paso 3 — Arranca el verificador',
  'bv-s4-d':'Los demás miembros del comité te alcanzan por la internet pública, así que el puerto no debe quedar expuesto sin cifrar. Caddy obtiene el certificado por su cuenta.',
  'bv-s4-t':'Paso 4 — Pon HTTPS delante',
  'bv-s5-d':'Los productores de bloques vinculan su clave a una cuenta humana registrada: la cartera firma <strong style="color:var(--text)">Aequitas: authorize validator &lt;dirección&gt;</strong> y sin eso la cadena rechaza la plaza. El botón de abajo produce esa firma — para el rol de validador. <strong style="color:var(--text)">Una clave de verificador aún no tiene ese vínculo.</strong> Su mitad pública se recoge fuera de la cadena (paso 6) y se añade a la lista que comprueba cada servidor de pruebas. Nada en la cadena la ata a una persona. Mientras eso falte, un comité cuenta máquinas, no personas, y un operador podría tener varias. Preferimos decirlo aquí a que la cifra parezca más fuerte de lo que es.',
  'bv-s5-t':'Paso 5 — Qué vincula una clave a una persona (y qué todavía no)',
  'bv-s6-d':'Envía al grupo la mitad <strong style="color:var(--text)">pública</strong> del paso 1 junto con tu dirección HTTPS. Se añade a la lista que consulta cada servidor de pruebas, y desde entonces tus atestaciones cuentan para el quórum. En este paso no sale nada secreto de tu máquina: ese es el sentido de la separación: la mitad privada se queda contigo para siempre, y la pública no vale nada sin ella.',
  'bv-s6-t':'Paso 6 — Publica tu clave pública',
  'bv-status-d':'El código del verificador <strong style="color:var(--text)">aún no es público</strong>, así que hoy no todo el mundo puede completar los pasos de abajo. Se publican ahora porque un diseño debe poder revisarse antes de desplegarse, no después. Si quieres poner uno en marcha, pregunta en el grupo de Telegram enlazado en la portada. Abrir este repositorio es lo que convertirá esta guía de un plan en una invitación, y es lo siguiente que os debemos.',
  'bv-status-t':'Estado: beta cerrada — léelo antes de empezar',
  'bv-title':'O conviértete en verificador biométrico — el papel que descentraliza la unicidad',
  'bv-what-d':'Nunca se te envía un rostro. Tu máquina guarda una <strong style="color:var(--text)">parte aditiva</strong> de un extracto de 64 bytes: por sí sola es indistinguible del ruido aleatorio, y ningún cálculo a tu alcance recupera un rostro a partir de ella. Las comparaciones se hacen conjuntamente con los demás miembros de tu comité, y ninguno aprende nada salvo la respuesta — <em>duplicado: sí o no</em>. No es una promesa sobre nuestras buenas intenciones; es una propiedad de la aritmética.',
  'bv-what-t':'Qué tendrías — y qué no verías nunca',
  'bv-why-d':'Un registro solo se acepta cuando <strong style="color:var(--text)">varios verificadores distintos</strong> lo han atestiguado. Una clave robada no basta: un atacante necesita un comité entero. Y como <strong style="color:var(--neon)">una persona solo puede tener una clave de validador</strong>, comprar un comité significa ser esas tantas personas. Con 100 verificadores, quien controle 10 tiene menos de una posibilidad entre 1.000 de poseer un comité completo de tres. Cada persona que se suma reduce ese número. Este es el único lugar donde el número de participantes <em>es</em> la seguridad. <strong style="color:var(--text)">Este cálculo supone una persona por clave de verificador.</strong> Para la producción de bloques la cadena lo exige; para las claves de verificador todavía no (paso 5). Hasta entonces, la cifra de arriba es un límite superior de la seguridad, no una medición.',
  'bv-why-t':'Por qué cada verificador adicional hace la red más difícil de corromper',
  'x-0-1-split-40-30':'0,1 % · reparto 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 personas. Tope de riqueza móvil 5x &#8594; 25x. Fase de cimientos.',
  'x-0-8211-2-years':'0 &#8211; 2 años',
  'x-0-perfect-equality':'0 = igualdad perfecta',
  'x-1-000-aeq-minted':'+1.000 AEQ emitidos',
  'x-1-000-aeq-per-human':'1.000 AEQ por persona',
  'x-1-000-aeq-will-be':'Se acreditarán 1.000 AEQ automáticamente',
  'x-10-000-8211-1m-humans':'10.000 &#8211; 1 M de personas. Mínimo 10 nodos. Totalmente descentralizado.',
  'x-100-8211-10-000-humans':'100 &#8211; 10.000 personas. Tope fijo 25x. Incorporación libre de nodos.',
  'x-100-maximum-concentration':'100 = concentración máxima',
  'x-1m-humans-global-ubi-at':'Más de 1 M de personas. Renta básica mundial a gran escala. Objetivo Gini &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Liquidez LP 30 %',
  'x-9679-treasury-10':'&#9679; Tesorería 10 %',
  'x-9679-ubi-pool-20':'&#9679; Fondo de renta básica 20 %',
  'x-9679-validators-40':'&#9679; Validadores 40 %',
  'x-active-validators':'Validadores activos',
  'x-add-aequitas-chain-to-metamask':'Añade la cadena Aequitas a MetaMask para ver tu saldo AEQ, enviar transacciones e interactuar con el contrato V7 desde el navegador o el monedero móvil.',
  'x-admin-keys-or-governance-votes':'claves de administración o votaciones de gobernanza',
  'x-aeq-activity':'ACTIVIDAD AEQ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'BlockDAG de Aequitas — nada se desperdicia',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Cadena Aequitas (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas lo implementa matemáticamente. Cada persona verificada recibe exactamente 1.000 AEQ &#8212; multimillonario o campesino de subsistencia, sin excepciones. Cuatro mecanismos de redistribución impiden que la desigualdad se acumule indefinidamente. El coeficiente de Gini se registra en la cadena en tiempo real.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — cadena de prueba de humanidad',
  'x-android-apk-direct-download':'APK de Android · descarga directa',
  'x-architecture':'Arquitectura',
  'x-automatic-on-chain':'automático, en la cadena',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (grafo acíclico dirigido)',
  'x-blockdag-parallel-production':'BlockDAG · producción paralela',
  'x-blockdag-proof-of-humanity':'BlockDAG + prueba de humanidad',
  'x-blue-score':'«puntuación azul»',
  'x-both-blocks-are-kept-ghostdag':'Se conservan ambos bloques — GHOSTDAG incorpora el bloque simultáneo y lo cuenta en el orden canónico.',
  'x-canonical-winner':'ganador canónico',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Comparable a EE. UU. (0,41) o Francia (0,32). Dentro del rango de la mayoría de economías desarrolladas. La redistribución aplana activamente la curva.',
  'x-confirm-ward-is-alive':'✓ CONFIRMAR QUE LA PERSONA VIVE',
  'x-core-technology':'Tecnología central',
  'x-daily-ubi-returns-to-all':'la renta básica diaria vuelve a todas las personas verificadas',
  'x-demurrage-0-5-mo':'Oxidación monetaria (0,5 %/mes)',
  'x-device-bound-zk-proof-one':'Prueba ZK ligada al dispositivo · un registro por dispositivo',
  'x-diagonal-line-perfect-equality':'diagonal = igualdad perfecta',
  'x-disconnect-wallet':'⊘ DESCONECTAR MONEDERO',
  'x-distinct-proposers-recent-blocks':'Proponentes distintos, bloques recientes',
  'x-distribution':'📈 Distribución',
  'x-elliptic-curve':'Curva elíptica',
  'x-entire-distribution':'distribución completa',
  'x-evm-compatible':'Compatible con EVM',
  'x-fill-ghostdag-verdict-thin-ring':'Relleno = veredicto GHOSTDAG · anillo fino = proponente · una columna por altura. Pasa el ratón por un bloque para ver detalles.',
  'x-generate-node-binding-signature':'🔗 Generar firma de vinculación',
  'x-run-a-coordinator':'🚪 Ejecutar un coordinador',
  'co-title':'O ejecuta un coordinador: la puerta por la que pasa cada persona',
  'co-desc':'El coordinador es donde llega una persona: emite el desafío, reparte la captura entre los verificadores, cuenta sus votos y emite la certificación sobre la que acuña la cadena. Durante mucho tiempo existió exactamente uno, de modo que cada registro de la red pasaba por una sola máquina. No porque faltara algo, sino porque nadie había puesto en marcha un segundo.',
  'co-status-t':'Estado: beta cerrada — la misma advertencia que para el verificador',
  'co-status-d':'El coordinador vive en el mismo repositorio que el verificador, y ese repositorio <strong style="color:var(--text)">aún no es público</strong>. Por eso hoy no todo el mundo puede completar los pasos siguientes. Se publican igualmente, por la misma razón: un diseño debe poder comprobarse antes de desplegarse, no después.',
  'co-power-t':'Lo que un coordinador puede hacer — y lo que no',
  'co-power-d':'<strong style="color:var(--text)">No puede inventar a una persona</strong>. Ningún bio_hash existe hasta que varios verificadores distintos lo han certificado, y el coordinador no tiene ninguna de sus claves. Lo que sí puede es vincular un bio_hash <strong style="color:var(--text)">existente</strong> a una cartera, de modo que uno deshonesto podría desviar una asignación a la dirección que quisiera. Es un poder real, crece con cada coordinador añadido, y quien decida si confiar debería conocer la diferencia.',
  'co-safe-t':'Por qué un segundo coordinador es seguro',
  'co-safe-d':'No siempre lo fue. Hasta agosto de 2026 la promesa <strong style="color:var(--text)">una persona, un registro</strong> dependía de un bloqueo Redis dentro del coordinador, y dos coordinadores independientes no comparten Redis: dos registros simultáneos de la misma persona habrían pasado ambos. Ahora <strong style="color:var(--text)">cada verificador comprueba por sí mismo</strong>, antes de su propia escritura, si ese rostro ya está inscrito. La garantía ya no depende de ningún servicio ni secreto compartido, así que un coordinador puede sumarse o desaparecer sin alterarla.',
  'co-need-t':'Lo que necesitas',
  'co-need-d':'Una cuenta Aequitas registrada — la misma regla que para producir bloques y para verificar: una persona, una clave. Un servidor con Docker y una dirección HTTPS pública, porque ningún navegador entrega la cámara a una página insegura. Y dos claves propias que generes tú y que nunca salgan de tu máquina: una firma tus certificaciones, otra asigna direcciones de cartera a marcadores.',
  'co-keys-t':'Nunca aceptes una clave de nadie — tampoco de nosotros',
  'co-keys-d':'Dos coordinadores con la misma clave de firma no son dos coordinadores: son uno con dos direcciones, y el quórum que debe proteger a las personas parecería cumplido sin estarlo. Genera ambas claves en tu propia máquina, con tu propia aleatoriedad, y que ninguna salga de ahí.',
  'co-auth-t':'Autorizar tu clave — sin permiso de nadie',
  'co-auth-d':'Mientras tu clave no esté autorizada, los verificadores rechazan todo lo que firme. Autorizarla exige dos pruebas y la aprobación de nadie: tu cartera firma que detrás de esta clave hay una persona registrada, y tu coordinador demuestra en su propio servidor que la clave es realmente suya. La primera la produces con el botón de arriba; la segunda la produce tu coordinador solo. Hasta agosto de 2026 además necesitabas un secreto compartido nuestro, con lo cual ese secreto <em>era</em> el permiso. Ya no existe.',
  'co-pernode-t':'El registro es por nodo, y es deliberado',
  'co-pernode-d':'Una autorización escrita en un nodo no viaja a los demás: no hay transacción para ello ni difusión. Una lista de confianza replicada sería exactamente la autoridad central sin la que está construido este sistema: cada operador decide por sí mismo qué certificaciones acepta su nodo. El coste es que tu autorización debe enviarse a cada nodo que deba honrarla. La firma es transferible: firmas una vez y la envías a todas partes; un nodo que omitas seguirá rechazándote.',
  'co-law-t':'Lo que aprendes sobre otras personas — y lo que se deriva de ello',
  'co-law-d':'La captura pasa por ti; la entregas y no guardas nada. Pero solo tú tienes la correspondencia entre dirección de cartera y marcador para quienes se registran a través de ti, y por eso tu clave de marcador debe seguir siendo tuya: compartida, cualquier operador podría calcular el marcador de cualquier dirección pública y averiguar de quién es el rostro. También significa que te conviertes en <strong style="color:var(--text)">responsable del tratamiento</strong> de esas personas según el RGPD. No nosotros. Las solicitudes de acceso, supresión y oposición llegan a ti, y eso no es una formalidad.',
  'co-limit-t':'La única limitación que esto crea',
  'co-limit-d':'La supresión por dirección de cartera solo funciona en el coordinador donde se hizo la inscripción: tu marcador depende de tu clave, y otro coordinador deriva uno distinto para la misma dirección. Un «no encontrado» desde otro sitio significa «no registrado aquí», no «no registrado» — y la respuesta lo dice. La vía a través del propio bio_hash, la que pertenece a la persona y no necesita operador alguno, funciona en cualquier coordinador, porque ese identificador no cambia.',
  'x-authorize-coordinator-key':'🔑 Autorizar clave de coordinador',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — un orden único a partir de un grafo enmarañado',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Coeficiente de Gini',
  'x-gini-coefficient-0-1':'Coeficiente de Gini (0–1)',
  'x-gini-index-history':'Histórico del índice de Gini',
  'x-gini-target-scandinavian-level':'Objetivo Gini (nivel escandinavo)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'ZKP Groth16 (conocimiento cero)',
  'x-guardian-system-8212-human-failsafe':'Persona de confianza &#8212; salvaguarda humana para monederos perdidos',
  'x-hash-wallet':'Hash / monedero',
  'x-healthier-than-most-nations-on':'Más sano que la mayoría de países del mundo. Comparable a Escandinavia (0,27) y Alemania (0,31). El tope de riqueza y la oxidación mantienen una distribución justa.',
  'x-higher-than-most-european-nations':'Más alto que en la mayoría de países europeos — comparable a Brasil (0,53) o Rusia. La redistribución del protocolo actúa con intensidad elevada.',
  'x-honest-limitation':'Limitación reconocida:',
  'x-how-it-works':'Cómo funciona',
  'x-how-to-read-this-chart':'Cómo leer este gráfico:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'personas podrían registrarse',
  'x-imagine-a-world-where-every':'«Imagina un mundo en el que cada persona de la Tierra &#8212; sin importar dónde nació, qué idioma habla o cuánto dinero tenían sus padres &#8212; recibe una renta diaria garantizada simplemente por ser humana. No como caridad. Como un derecho matemático, aplicado por un código que ningún gobierno ni empresa puede anular.»',
  'x-inactive-escrow':'Depósito por inactividad',
  'x-inactivity-timeline':'Cronología de inactividad',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (resistente a lo cuántico)',
  'x-key-protections':'Protecciones esenciales:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — la evolución propia de Aequitas más allá de un GHOSTDAG de K fija',
  'x-knightdag-secured':'· asegurado por KnightDAG',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Como Escandinavia (~0,27)',
  'x-liquidity-pool-30':'Fondo de liquidez (30 %)',
  'x-loading-blocks':'Cargando bloques…',
  'x-loading-topology':'Cargando topología…',
  'x-loading-transactions':'Cargando transacciones…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Curva de Lorenz — distribución de AEQ entre las personas',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: si el saldo AEQ muestra 0 tras el registro, ve a Ajustes → Redes → elimina la cadena Aequitas → vuelve a añadirla desde esta web',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: si AEQ muestra 0 tras añadirla, elimina la red y vuelve a añadirla con el botón de arriba.',
  'x-money-exists-because-people-exist':'El dinero existe porque existen las personas. Por tanto, cada persona debería tener una parte igual, por el mero hecho de ser humana.',
  'x-money-exists-because-people-exist-2':'«El dinero existe porque existen las personas. Ni más ni menos.»',
  'x-most-unequal-currency-ever':'La moneda más desigual jamás vista',
  'x-multi-validator-network':'Red con varios validadores',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: aún no significativo',
  'x-no-snapshots-yet-first-one':'Aún no hay registros — el primero se guardará tras el próximo reparto.',
  'x-no-stake-blockchain':'Blockchain sin participación en juego',
  'x-node-operator-guide-pdf':'📄 Guía del operador de nodo (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET debe ser una persona registrada en Aequitas',
  'x-one-human-one-wallet-1':'Una persona = un monedero = 1.000 AEQ',
  'x-p2p-protocol':'Protocolo P2P',
  'x-paid-out-daily':'pagado a diario',
  'x-permanent-on-chain':'Permanente · en la cadena',
  'x-phase-roadmap-8212-the-path':'Hoja de ruta por fases &#8212; el camino hacia la escala mundial',
  'x-phase-transitions-are-automatic-8212':'Los cambios de fase son automáticos &#8212; los activan umbrales de población y los aplica el contrato. Sin votaciones ni clave de administración.',
  'x-planned-post-beta':'Previsto (tras la beta)',
  'x-postgresql-persistent':'PostgreSQL (persistente)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Aporta liquidez AEQ / tUSD para ganar el 30 % de todas las comisiones de intercambio, repartidas a diario.',
  'x-recorded-after-each-ubi-distribution':'Se registra tras cada reparto de renta básica. Muestra cómo evoluciona la igualdad a medida que crece la red. Cuanto más bajo, mejor — el objetivo es un Gini por debajo de 0,30.',
  'x-redistribution':'REDISTRIBUCIÓN',
  'x-run-a-node':'⚙️ Ejecutar un nodo',
  'x-run-a-verifier':'⚙️ Ejecutar un verificador',
  'x-set-guardian':'🛡 DESIGNAR PERSONA DE CONFIANZA',
  'x-swap-fees-0-1':'Comisiones de intercambio (0,1 %)',
  'x-sybil-resistance-8212-current-state':'Resistencia Sybil &#8212; el estado actual, con franqueza',
  'x-the-4-redistribution-mechanisms':'Los cuatro mecanismos de redistribución',
  'x-the-core-innovation':'La idea central',
  'x-the-matching-threshold-has-not':'El umbral de coincidencia aún no se ha calibrado con capturas reales',
  'x-the-vision-8212-a-global':'La visión &#8212; un protocolo mundial de renta básica',
  'x-the-year-is-2009-satoshi':'Corre el año 2009. Satoshi Nakamoto publica Bitcoin. Por primera vez, el valor puede pasar entre dos personas sin un banco. Una revolución auténtica. Pero casi de inmediato algo se tuerce.',
  'x-this-is-not-a-0815':'Esto no es una blockchain corriente de un bloque cada vez. Aequitas ejecuta un BlockDAG real, ordenado por GHOSTDAG — y desde 2026 asegurado por KnightDAG, su propia evolución adaptativa. De este mecanismo dependen en última instancia cada saldo, cada pago y cada tope de riqueza, para que exista una única historia acordada.',
  'x-today-beta':'Hoy (beta)',
  'x-today-this-verifies-one-device':'Hoy esto verifica un dispositivo, todavía no una persona única',
  'x-traditional-blockchain-wasted-work':'Blockchain tradicional — trabajo desperdiciado',
  'x-treasury-10':'Tesorería (10 %)',
  'x-trusted-verified-human':'persona verificada y de confianza',
  'x-two-validators-produce-at-once':'Dos validadores producen a la vez → uno gana, el otro se descarta — trabajo perdido, y limita la velocidad a la que la red puede avanzar con seguridad.',
  'x-ubi-pool-20':'Fondo de renta básica (20 %)',
  'x-validators-pool-40':'Fondo de validadores (40 %)',
  'x-view-source-on-github':'🐙 Ver el código en GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'Multiplicador del tope de riqueza — deslizador de arranque',
  'x-wealth-cap-overflow':'Excedente del tope de riqueza',
  'x-wealth-distribution-analysis':'Análisis de la distribución de la riqueza',
  'x-what-happens-when-someone-is':'¿Qué ocurre si alguien es hospitalizado, encarcelado o fallece? En la mayoría de sistemas cripto, un monedero perdido lo está para siempre. Aequitas cuenta con una recuperación por inactividad en tres capas.',
  'x-what-is-a-guardian':'¿Qué es una persona de confianza?',
  'x-what-is-and-is-not':'Qué es privado y qué no lo es:',
  'x-what-would-a-cryptocurrency-look':'«¿Cómo sería una criptomoneda diseñada desde el principio para ser justa con todo ser humano?»',
  'x-why-a-normal-blockchain-isn':'Por qué no basta con una blockchain normal',
  'x-worse-than-any-country-on':'Peor que cualquier país del mundo (récord de Sudáfrica: 0,63). Se acerca a Bitcoin (0,85). El protocolo interviene al máximo — tope y redistribución a plena potencia.',
  'x-year-2-180d':'Año 2 +180 d',
  'x-zk-device-key-proof':'Prueba ZK de la clave del dispositivo',
  'swap-price-flat':'Sin operaciones en este periodo — el precio no se ha movido. El gráfico funciona; el mercado está tranquilo.',
  'mpc-optin-title':'Opcional — ayudar a detectar registros duplicados (preparado, aun no en servicio)',
  'mpc-optin-desc':'Preparado, pero aún no en servicio. Más adelante tu nodo podrá ayudar a comprobar que nadie se registra dos veces sin ver jamás datos biométricos: cada parte participante guarda solo una porción matemática de cada plantilla — ruido por sí sola — y comparan juntas una captura nueva, de modo que ninguna máquina puede reconstruir nada. Hoy este camino no decide nada. La comprobación de duplicados no pasa por él, y el comité es una lista fija en lugar de sortearse automáticamente, así que fijar las tres variables no cambia nada por ahora.',
  'mpc-optin-note':'El archivo de porciones contiene aleatoriedad de un solo uso que solo tu nodo puede guardar — nunca lo copies a otra máquina ni lo subas a ningún repositorio. Por ahora debe proporcionarlo el operador, y esa es la dependencia central que queda. No necesitas una clave nueva: tu nodo se identifica con la misma clave de firma que ya usa para los bloques.',
  'logo-sub':'PRUEBA DE HUMANIDAD','live':'EN VIVO',
  'reg-title':'🔐 Regístrate como Humano Verificado',
  'reg-sub':'Únete a la red Aequitas y recibe tu subsidio de Renta Básica Universal de 1,000 AEQ. Único, permanente y completamente gratuito. Ningún dato personal es almacenado.',
  'app-title':'REGISTRO SOLO VÍA APP ANDROID',
  'app-text':'Al registrarte, la cámara capta tu rostro y una breve secuencia de vitalidad. Servicios de comparación independientes comprueban que hay una persona viva y que ese rostro no está ya registrado; deben coincidir por quórum. Una prueba ZK Groth16 lleva luego el resultado a la cadena sin revelar nada sobre ti. Tus <strong style="color:var(--gold)">1.000 AEQ se acreditan automáticamente</strong> tras la verificación. <strong style="color:var(--gold)">Nota:</strong> el umbral de comparación aún no está calibrado con capturas reales — ver las preguntas frecuentes.',
  's1t':'Captura facial','s1d':'La app graba tu rostro y una breve secuencia de vitalidad y los envía a servicios de comparación independientes. Estos comprueban que hay una persona viva delante y comparan el rostro con todos los ya registrados. Las imágenes se descartan tras el procesamiento.',
  's2t':'Generación de Prueba ZK','s2d':'Una prueba ZK Groth16 compromete tu bio_hash en commitment = keccak256(bioHash‖wallet) sin revelarlo. El nullifier se deriva de ese hash, así que el mismo rostro no puede contar dos veces — ver las preguntas frecuentes.',
  's3t':'Conectar Wallet','s3d':'La app abre MetaMask en esta página · conecta tu wallet Ethereum · la prueba está criptográficamente vinculada a tu dirección',
  's4t':'1,000 AEQ Acreditados','s4d':'Registro confirmado en el BlockDAG de Aequitas en 1 segundo · 1,000 AEQ acreditados instantáneamente · tu identidad queda permanentemente registrada',
  'priv-bar':'🔒 Verificación facial por quórum · Groth16 ZKP · Imágenes descartadas tras la comprobación · Un registro por persona',
  'conn-wallet':'WALLET CONECTADA','proof-recv':'⚡ PRUEBA ZK RECIBIDA','proof-hint':'Conecta wallet para registrar',
  'btn-conn':'🦊 CONECTAR METAMASK','btn-reg':'🔐 REGISTRAR ON-CHAIN',
  'btn-wc':'🔗 CONECTAR WALLETCONNECT',
  'reg-log-hint':'// Abre la App Android Aequitas para generar tu prueba, luego regresa aquí...',
  'reg-details':'Detalles del Registro','k-network':'Red','k-chainid':'ID de Cadena','k-grant':'Subsidio UBI',
  'k-fee':'Tarifa de Gas','free':'GRATIS — completamente sin gas','k-limit':'Registros','k-limit-v':'Una vez por persona · permanente · inmutable',
  'k-bio':'Rostro','never-stored':'Las imágenes se descartan tras la comprobación — ningún validador tiene una plantilla completa',
  'k-proof':'Sistema de Prueba','k-conf':'Confirmación','k-conf-v':'En 1 segundo (1 bloque)',
  'k-sybil':'Protección Sybil','k-sybil-v':'Una identidad por persona · ligada al rostro, umbral aún sin calibrar',
  's-height':'Altura de Bloque',
  's-humans':'Humanos Verificados',
  's-supply':'Suministro Total','s-supply-sub':'Siempre = Humanos × 1,000 AEQ',
  's-uptime':'Tiempo Activo',
  'k-chain':'Nombre de Cadena','k-symbol':'Símbolo','k-btime':'Tiempo de Bloque',
  'k-cons':'Consenso','k-storage':'Almacenamiento','k-dec':'Decimales',
  'btn-add-mm':'+ AGREGAR RED AEQUITAS',
  'humans-title':'Humanos Verificados en Aequitas Chain',
  'h-what':'¿Qué es un Humano Verificado?','h-what-t':'Un Humano Verificado es una dirección wallet para la que se ha demostrado que pertenece a alguien cuyo rostro no está ya registrado. Servicios de comparación independientes deben coincidir por quórum, y a la cadena solo llega una prueba ZK Groth16 — ninguna imagen y ninguna plantilla. <strong style="color:var(--gold)">Hasta el 23-08-2026 esto verificaba un dispositivo y no una persona; ya no es así.</strong>',
  'h-zkp':'Sistema de Prueba ZK','h-zkp-t':'Aequitas usa Groth16 en BN128 — misma curva que Ethereum y Zcash. ~200 bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). El nullifier está vinculado a este dispositivo: perder tu teléfono no crea una segunda identidad en él, pero otro dispositivo puede seguir registrándose por separado. Ningún material de clave se revela ni almacena en el servidor.',
  'h-sybil':'Resistencia Sybil — Estado Actual','h-sybil-t':'El nullifier se deriva del bio_hash de tu rostro, así que el mismo rostro no puede registrarse dos veces — tampoco entre dispositivos, algo que una clave de dispositivo nunca pudo. Sobre lo que descansa es un umbral de comparación aún sin calibrar con capturas reales: la criptografía es exacta, la biometría debajo es una medición cuya tasa de error no está cuantificada.',
  'h-global':'Inclusión Financiera Global','h-global-t':'Sin cuenta bancaria, sin tarjeta de crédito, sin experiencia previa en criptomonedas. Solo un smartphone Android con cámara. Aequitas está diseñado para ser accesible a todas las personas del planeta.',
  'h-bio-hw':'Hoja de Ruta de Verificación de Identidad','h-bio-hw-t':'Hoy (beta): una comprobación facial entre servicios de comparación independientes que deben coincidir por quórum. Su umbral aún no está calibrado con capturas reales — hacen falta unos 1000 pares de impostores antes de citar cifra alguna. Previsto: esa calibración y una comprobación de duplicados en la que ningún servicio conserve una plantilla completa.',
  'reg-humans':'Humanos Registrados','h-desc':'Cada dirección de abajo pertenece a alguien cuyo rostro fue comprobado por servicios independientes frente a todos los registros existentes, demostrado con una prueba ZK y acreditado con exactamente 1.000 AEQ. El registro es permanente, inmutable y on-chain. Lo que el umbral garantiza hoy y lo que no, está en las preguntas frecuentes.',
  'no-humans':'No hay humanos registrados aún.\n\n¡Descarga la App Android Aequitas y sé el primero!',
  'reg-stats':'Estadísticas del Registro','total-humans':'Total de Humanos',
  'idx-title':'Índice Aequitas — Puntuación de Igualdad Económica en Tiempo Real',
  'idx-desc':'El Índice Aequitas mide la desigualdad económica de todos los humanos verificados en tiempo real. Se calcula desde el coeficiente Gini de la distribución de saldos on-chain. 0 = igualdad perfecta. 100 = desigualdad máxima.',
  'gini-what-title':'¿Qué es el Coeficiente de Gini?','gini-what-text':'Desarrollado por el estadístico italiano Corrado Gini (1912). Mide la distribución de la riqueza comparando los saldos reales con una línea base hipotéticamente igualitaria — visualizado como la curva de Lorenz. Escala: 0 (todos tienen lo mismo) a 1 (una persona lo tiene todo). Usado por el Banco Mundial, la OCDE y la ONU para comparar países. Valores de referencia: Bitcoin ≈ 0,85 · Sudáfrica (récord mundial) ≈ 0,63 · EE.UU. ≈ 0,41 · Alemania ≈ 0,31 · Escandinavia ≈ 0,27 · Objetivo a largo plazo de Aequitas: Gini por debajo de 0,30 — comparable a los países escandinavos, impuesto por el límite de riqueza.',
  'gini-calc-title':'¿Cómo se calcula el Índice Aequitas?','gini-calc-text':'Se recopilan todos los saldos de AEQ de humanos verificados. La fórmula calcula la diferencia absoluta media entre cada par posible de saldos, normalizada por la población al cuadrado (n²) y el saldo medio (x̄). El resultado 0–1 multiplicado por 100 = Índice Aequitas. Se actualiza on-chain tras cada registro, demurrage mensual, pago de pool y evento de límite de riqueza.',
  'gini-why-title':'¿Por qué Gini — y no una métrica más simple?','gini-why-text':'Una simple relación rico-pobre es fácil de manipular: 10.000 wallets podrían mostrar una dispersión baja pero con el 90% del AEQ concentrado en 100 manos — Gini detecta esto, una relación simple no. El coeficiente captura la distribución completa entre todos los humanos verificados en un único número auditable. Aequitas publica esto on-chain — transparente, a prueba de manipulaciones, verificable globalmente. Es la señal principal para las transiciones automáticas de fase, la calibración del límite de riqueza y la intensidad de redistribución.',
  'curr-idx':'Índice Actual','bar-0':'0 — Igualdad Perfecta','bar-100':'100 — Máx. Desigualdad','wcap-lbl':'Límite de Riqueza Actual:','wcap-mult':'Multiplicador:','wcap-avg':'Parte justa:',
  'gini':'Coeficiente Gini','gini-desc':'0 = igual · 1 = desigual',
  'supply-desc':'Siempre = Humanos × 1,000 AEQ',
  'phase':'Fase del Protocolo','phase-desc':'Avanza automáticamente por recuento humano',
  'humans-desc':'Registros verificados por rostro',
  'pools-title':'Pools de Redistribución',
  'pools-desc':'Cada tarifa de swap, cargo de demurrage y desbordamiento del límite de riqueza se divide automáticamente entre cuatro pools. Sin intervención manual. Todos los pools pagan diariamente.',
  'vel-pool':'Pool Validadores','vel-pool-desc':'40% de todas las tarifas → operadores de nodos que aseguran la red',
  'liq-pool':'Pool Liquidez','liq-pool-desc':'30% de todas las tarifas → proveedores de liquidez, proporcional a participaciones LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% de todas las tarifas → todos los humanos verificados por igual, cada 24 horas',
  'treasury':'Tesorería','treasury-desc':'10% de todas las tarifas → desarrollo y mantenimiento del protocolo',
  'phases-title':'Fases del Protocolo',
  'phases-desc':'En Fase 0, el límite de riqueza usa un multiplicador de arranque: max(5, min(N, 25))× saldo promedio. Con 1–4 humanos: 5× promedio. Cada nuevo humano añade 1×. A 25+ humanos: fijado permanentemente en 25×. Fase 1+ mantiene 25× fijo. Todas las transiciones son automáticas — sin voto de gobernanza, sin clave de administrador.',
  'p0':'Bootstrap · &lt;100 humanos · Límite de Riqueza: max(5,min(N,25))× promedio · Deslizamiento 5×→25× hasta el 25.º humano · Actualmente activo',
  'p1':'Crecimiento · 100–10,000 humanos · Límite de Riqueza: 25× la parte justa = 25.000 AEQ',
  'p2':'Estabilidad · 10,000–1M humanos · Límite de Riqueza: 25× la parte justa = 25.000 AEQ',
  'p3':'Madurez · 1M+ humanos · Límite de Riqueza: 25× la parte justa = 25.000 AEQ',
  'wealth-cap-explain':'El Límite de Riqueza en Fase 0 (Bootstrap) usa max(5, min(N, 25))× saldo promedio, donde N = humanos registrados. 1–4 humanos: 5× promedio. Cada nuevo humano añade 1×. 25+ humanos: bloqueado en 25× permanentemente. El límite siempre se escala con el saldo promedio actual.',
  'btn-download-app':'DESCARGAR APP AEQUITAS',
  'swap-title':'🔄 Intercambiar AEQ ↔ tUSD','swap-sub':'Intercambia AEQ por tUSD (un dólar de prueba simulado) a través del pool de liquidez nativo. Se aplica una comisión del 0,1% solo a los intercambios — las transferencias ordinarias de AEQ entre personas permanecen completamente gratuitas.',
  'swap-priv-bar':'🔒 Solo 0,1% de comisión de swap · Transferencias AEQ a AEQ gratuitas · tUSD es una moneda de prueba sin valor real',
  'swap-your-aeq':'Tu AEQ','swap-your-tusd':'Tu tUSD',
  'swap-fee-est':'Comisión de protocolo (0,1%)','swap-details-hdr':'Detalles del Swap',
  'swap-out-lbl':'Recibes (est.)','swap-impact-lbl':'Impacto en precio','swap-rate-lbl':'Tipo de cambio',
  'swap-depth-lbl':'Composición del Pool','amm-title':'x × y = k — AMM de Producto Constante',
  'amm-text':'Cuando intercambias AEQ por tUSD, la reserva de AEQ crece y la de tUSD decrece — su producto siempre permanece igual a k. Swaps más grandes causan mayor impacto en precio. La comisión del 0,1% se descuenta antes de aplicar la fórmula.',
  'swap-btn-go':'🔄 INTERCAMBIAR',
  'swap-log-hint':'// Conecta tu wallet para intercambiar...',
  'swap-no-liquidity':'¿Sin tUSD todavía?','swap-faucet-desc':'Los humanos registrados pueden reclamar tUSD de prueba una vez','swap-btn-faucet':'💧 RECLAMAR tUSD DE PRUEBA',
  'swap-addliq-title':'Proporcionar Liquidez','swap-addliq-desc':'Sé el primero en depositar — tu ratio establece el precio inicial.','swap-btn-addliq':'💧 AGREGAR LIQUIDEZ',
  'swap-lp-title':'Tu Posición LP','swap-lp-share':'Participación del Pool','swap-lp-withdrawable':'Retirable',
  'swap-lp-pct-label':'% de tu posición','swap-lp-youget':'Recibirás','swap-btn-removeliq':'🔥 RETIRAR LIQUIDEZ',
  'swap-pool-title':'AEQ / tUSD — Estado del Pool',
  'swap-pool-aeq':'Reserva AEQ','swap-pool-tusd':'Reserva tUSD','swap-pool-price':'Precio Spot',
  'swap-fee-bps':'Comisión de Swap',
  'swap-pools-addr-title':'Direcciones de Pools Tokenomics',
  'swap-validators':'Validadores (40%)','swap-lps':'Proveedores de Liquidez (30%)','swap-ubi':'Pool UBI (20%)','swap-treasury':'Tesorería (10%)',
  'ubi-hero-title':'RENTA BÁSICA UNIVERSAL — POOL UBI',
  'ubi-hero-sub':'Acumulando — próximo pago distribuido por igual a todos los humanos verificados en:',
  'ubi-bal-lbl':'saldo actual del pool','ubi-hero-desc':'Dividido por igual entre todos · pagado cada 24h · el pool se reinicia a cero · sin saldo mínimo requerido',
  'ubi-how-fills':'Cómo se llena el Pool UBI',
  'ubi-src-swap':'Comisiones de Swap','ubi-src-swap-d':'Cada swap AEQ↔tUSD contribuye el 20% de su comisión de 0,1%. Más actividad = llenado más rápido.',
  'ubi-src-dem':'Demurrage','ubi-src-dem-d':'AEQ inactivo (3+ meses) decae al 0,5%/mes. El 20% del importe decaído va al UBI.',
  'ubi-src-cap':'Desbordamiento del Límite','ubi-src-cap-d':'Wallets que superan el límite de riqueza (max(5,min(N,25))× promedio) son confiscadas al instante. El 20% fluye al UBI.',
  'pools4-header':'Los cuatro pools de redistribución',
  'ubi-see-above':'ver countdown arriba','ubi-timer-above':'⏰ countdown mostrado arriba','pool-t-timer':'Acumula — sin temporizador',
  'usp-headline':'Por primera vez en la historia — todos empiezan igual',
  'usp-sub':'Si tienes un smartphone Android, calificas. Sin banco, sin conocimientos cripto, sin inversión.',
  'usp-c1-title':'0,00 Inversión Inicial','usp-c1-desc':'El registro es completamente sin gas. Sin ETH, sin MATIC, sin tarjeta de crédito. El protocolo paga todas las comisiones.',
  'usp-c2-title':'1.000 AEQ para cada humano','usp-c2-desc':'Millonario o agricultor — todos reciben exactamente 1.000 AEQ. Inicio igual, garantizado matemáticamente.',
  'usp-c3-title':'Accesible para todos','usp-c3-desc':'Sin cuenta bancaria, sin tarjeta de crédito, sin documento de identidad, sin hardware adicional — solo la cámara que ya tiene tu teléfono Android.',
  'usp-c4-title':'UBI diario para siempre','usp-c4-desc':'Tras registrarte recibes automáticamente una parte diaria de los pagos UBI — cada día, sin ninguna acción requerida.',
  'v7-intro-title':'¿Qué es AequitasV7?',
  'v7-intro-text':'AequitasV7 es el contrato inteligente central del protocolo Aequitas. "V7" es la 7ª versión mayor del contrato de equidad. Es inmutable en Aequitas Chain (ID 1926) y gestiona todo: registro humano, verificación ZK, gestión de saldos, límite de riqueza, distribución UBI, comisiones de swap. Ningún administrador puede actualizarlo. Los seis mecanismos forman un sistema autorreforzante: el demurrage alimenta el UBI, el desbordamiento del límite suma al UBI, las comisiones se distribuyen entre los cuatro pools simultáneamente.',
  'demurrage-title':'Demurrage — Incentivo para Circular',
  'demurrage-desc':'Aequitas implementa un mecanismo de demurrage inspirado en monedas complementarias históricas. Los saldos AEQ inactivos pierden valor lentamente para desalentar el acaparamiento.',
  'dem-rate-k':'Tasa de Decaimiento','dem-rate-v':'0.5% por mes (continuo, no escalonado)',
  'dem-grace-k':'Período de Gracia','dem-grace-v':'3 meses de inactividad antes de que comience el decaimiento',
  'dem-reset-k':'Reinicio del Reloj','dem-reset-v':'Cualquier transferencia, swap o acción de liquidez reinicia el temporizador',
  'dem-dest-k':'AEQ decaído va a','dem-dest-v':'Pools de redistribución (división 40/30/20/10)',
  'dem-warn-k':'Sistema de Advertencia','dem-warn-v':'Aviso de 14 días (una vez) + recordatorio de 7 días repetido en cada inicio',
  'story-title':'La Historia de Aequitas',
  'nodes-title':'Nodos Activos — Topología Actual de la Red',
  'nodes-desc':'La red Aequitas opera actualmente en múltiples nodos distribuidos geográficamente (número actual arriba). Todos participan en la producción de bloques, sincronización de estado y servicio de API. Se comunican peer-to-peer via libp2p y sincronizan el estado de bloques via HTTP. La red está diseñada para soportar nodos adicionales.',
  'run-node-title':'Ejecuta Tu Propio Nodo — Ayuda a Asegurar la Red',
  'run-node-desc':'Cualquier persona registrada puede ejecutar un nodo de Aequitas — sin stake, sin solicitud, sin permiso nuestro. Una persona, una clave de validador: un nodo cuyo NODE_OPERATOR_WALLET no sea una persona registrada se rechaza con HTTP 403, porque si no una sola persona podría convertirse en todo el conjunto de validadores. Los nodos participan en la producción de bloques y validan el registro humano. Los operadores de nodos ganan una parte de las comisiones del protocolo via el Pool de Validadores (40% de todas las comisiones de swap, distribuidas diariamente).',
  'bootstrap-title':'Conectar un Nuevo Nodo','bootstrap-desc':'Para ejecutar tu propio nodo no configuras ningún punto de entrada — las direcciones de validador vienen integradas. Tu nodo se registra solo y sincroniza automáticamente el estado completo de la cadena. Establece PRIMARY_NODE_URL solo si quieres fijar deliberadamente un punto de entrada concreto.',
  'tech-title':'Especificaciones Técnicas','mm-config':'Configuración MetaMask',
  'k-lang':'Idioma','k-src':'Código Fuente','evm-yes':'Sí — JSON-RPC /rpc · Compatible con MetaMask',
  'proto-label':'Protocolo Aequitas V7 — Documentación Técnica',
  'ca-title':'Contratos y Direcciones de Red','ca-text':'Cadena: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier (verificador Groth16 on-chain): 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (contrato principal): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 define las reglas de la economía Aequitas y mantiene el registro que las hace exigibles: cada nullifier reclamado, cada registro, el límite de riqueza y la fórmula de demurrage. Es inmutable — no hay clave de administrador, ni proxy de actualización, ni votación de gobernanza que pueda cambiar una línea. Lo que liquida una transferencia real, sin embargo, es la capa de cadena: el nodo intercepta la llamada ERC-20 antes de que llegue a la EVM y la aplica a su propio libro mayor, que es lo que hace las transferencias instantáneas y sin gas. El contrato es el reglamento y el registro; la cadena es el motor que los ejecuta, y su código es público.<br><br>El contrato BioVerifier recibe pruebas de conocimiento cero Groth16 generadas completamente en el dispositivo Android del usuario. Verifica matemáticamente on-chain en ~10 ms que el nullifier enviado se derivó correctamente de un secreto que posee el registrante, y la cadena rechaza cualquier nullifier que ya haya visto — sin conocer jamás su nombre, identidad o datos biométricos. Eso descarta un segundo registro desde la misma fuente de identidad; si esa fuente es una persona o un dispositivo depende de si el modo biométrico está activo. Esto es lo que hace posible el registro sin gas y sin inversión: la prueba es lo único que sale del dispositivo.<br><br>Esa combinación es lo verdaderamente nuevo: las reglas y el registro de «un humano, un registro» viven en un contrato que nadie —ni el operador, ni una empresa, ni un gobierno— puede reescribir, y el código que las ejecuta es abierto y reproducible desde este repositorio. Todo ello es verificable por cualquiera. Lo que sigue requiriendo confianza es la operación de los propios nodos, y la forma honesta de reducirla son más validadores independientes, no una frase más rotunda aquí.',
  'h-what':'¿Qué es un Humano Verificado?','h-what-t':'Un Humano Verificado es una dirección wallet para la que se ha demostrado que pertenece a alguien cuyo rostro no está ya registrado. Servicios de comparación independientes deben coincidir por quórum, y a la cadena solo llega una prueba ZK Groth16 — ninguna imagen y ninguna plantilla. <strong style="color:var(--gold)">Hasta el 23-08-2026 esto verificaba un dispositivo y no una persona; ya no es así.</strong>',
  'h-zkp':'Sistema de Prueba ZK','h-zkp-t':'Aequitas usa Groth16 en BN128 — misma curva que Ethereum y Zcash. ~200 bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). El nullifier está vinculado a este dispositivo: perder tu teléfono no crea una segunda identidad en él, pero otro dispositivo puede seguir registrándose por separado. Ningún material de clave se revela ni almacena en el servidor.',
  'h-sybil':'Resistencia Sybil — Estado Actual','h-sybil-t':'El nullifier se deriva del bio_hash de tu rostro, así que el mismo rostro no puede registrarse dos veces — tampoco entre dispositivos, algo que una clave de dispositivo nunca pudo. Sobre lo que descansa es un umbral de comparación aún sin calibrar con capturas reales: la criptografía es exacta, la biometría debajo es una medición cuya tasa de error no está cuantificada.',
  'h-global':'Inclusión Financiera Global','h-global-t':'Sin cuenta bancaria, sin tarjeta de crédito, sin experiencia previa en criptomonedas. Solo un smartphone Android con cámara. Aequitas está diseñado para ser accesible a todas las personas del planeta.',
  'h-bio-hw':'Hoja de Ruta de Verificación de Identidad','h-bio-hw-t':'Hoy (beta): una comprobación facial entre servicios de comparación independientes que deben coincidir por quórum. Su umbral aún no está calibrado con capturas reales — hacen falta unos 1000 pares de impostores antes de citar cifra alguna. Previsto: esa calibración y una comprobación de duplicados en la que ningún servicio conserve una plantilla completa.',
  'poa-title':'1. PRUEBA DE VIDA — Recuperación de Saldos Inactivos','poa-text':'<p>¿Qué pasa con AEQ cuando las personas mueren o quedan permanentemente incapacitadas? En Bitcoin, las wallets perdidas significan suministro perdido permanentemente. Aequitas soluciona esto mediante un sistema de recuperación por inactividad de múltiples etapas: si una wallet no muestra actividad durante un período prolongado, su saldo se devuelve gradualmente a la comunidad a través del pool UBI.</p>',
  'poa-box':'Año 0–2: Uso normal — sin restricciones<br>Año 2: Aviso 1 — el Guardian puede responder en nombre<br>Año 2+60d: Aviso 2 — urgencia creciente<br>Año 2+120d: Aviso 3 — aviso final<br>Año 2+180d: AEQ movido a CUSTODIA personal (aún recuperable)<br>Año 4: Si aún inactivo — CUSTODIA liberada al Pool UBI',
  'guard-title':'2. SISTEMA GUARDIAN — Salvaguarda Humana','guard-text':'<p>¿Y si alguien está hospitalizado, encarcelado o de algún modo incapaz de acceder a su dispositivo por meses? El sistema Guardian permite a una persona de confianza — otro humano verificado — confirmar que el propietario de la wallet sigue vivo. El Guardian tiene estrictamente cero acceso financiero: solo puede llamar una función que reinicia el temporizador de inactividad.</p>',
  'guard-box':'1 Guardian por humano · debe ser un humano verificado en Aequitas<br>Guardian SOLO puede llamar confirmAlive() — cero derechos de transacción<br>Guardian NO PUEDE mover fondos, transferir AEQ ni acceder a la wallet<br>Máximo 3 tutelados por Guardian (evita centralización de confianza)<br>Bloqueo de 7 días en asignación de Guardian (evita asignación forzada)<br>No se permiten relaciones Guardian circulares',
  'dem-title':'3. DEMURRAGE — Mecanismo Anti-Acaparamiento',
  'dem-box':'Se cobra solo sobre la parte por encima de tu parte justa — un saldo igual o inferior nunca decae<br>Tasa: 0,5% por mes después de 3 meses de inactividad (continuo, no escalonado)<br>El reloj se reinicia automáticamente con cualquier transferencia, swap o acción de liquidez<br>AEQ decaído redistribuido a los cuatro pools — nunca destruido<br>Aviso de 14 días mostrado una vez · aviso de 7 días repetido en cada sesión activa',
  'dem-text':'<p>El demurrage es un costo de tenencia sobre el dinero — una tasa de interés negativa que hace costoso acumular y atractivo circular. El experimento de Wörgl (Austria, 1932) usó una moneda con demurrage y redujo el desempleo local un 25% en un año. El Banco Central de Austria lo cerró precisamente porque funcionó demasiado bien. El Chiemgauer (Alemania, 2003) opera según el mismo principio con éxito desde hace más de 20 años.</p>',
  'cap-title':'4. LÍMITE DE RIQUEZA — Aplicación de Justicia Matemática','cap-box':'Límite bootstrap: max(5,min(N,25))× saldo promedio actual<br>1–4 humanos: 5× · +1× por humano · 25+: 25× permanente<br>Se aplica a TODAS las direcciones excepto las 4 pools del protocolo<br>Exceso AEQ redistribuido instantáneamente · Sin intervención manual',
  'ubi-title':'5. RENTA BÁSICA UNIVERSAL — Redistribución Diaria','ubi-box':'Fuentes de ingresos del Pool UBI:<br>· 20% de todas las comisiones de swap del pool AMM AEQ↔tUSD<br>· Desbordamiento de la aplicación del límite de riqueza<br>· Cargos de demurrage de cuentas inactivas<br>· Custodia inactiva liberada después de 4 años<br><br>Distribución: Cada 24 horas, todo el saldo del pool UBI se divide igualmente entre todos los humanos verificados registrados. El pool se reinicia a cero y comienza a llenarse inmediatamente de la actividad continua del protocolo.',
  'inf-title':'6. SIN INFLACIÓN ALGORÍTMICA — Fórmula de Suministro Fijo','inf-box':'El ÚNICO evento que crea nuevo AEQ: un nuevo humano verificado se registra.<br><br>Suministro Total = Humanos Verificados × 1.000 AEQ<br><br>Esto no es una política — es aplicado por el protocolo. Ningún administrador puede acuñar AEQ adicional, ningún voto de gobernanza puede cambiar la emisión. AEQ es la única criptomoneda donde el suministro total está determinado únicamente por el número de humanos vivos verificados.',
  'swap-sell-label':'Vender','swap-receive-label':'Recibir',
  'guard-title':'🛡 Sistema Guardian','guard-my-lbl':'Mi Guardian','guard-none':'Ninguno',
  'guard-set-lbl':'Establecer / Cambiar Guardian','guard-set-hint':'Debe ser un humano registrado en Aequitas · Bloqueo de 7 días · El Guardian solo puede confirmar tu vitalidad, no acceder a fondos · Máximo 3 protegidos por Guardian',
  'guard-confirm-lbl':'Confirmar Vivo (Como Guardian)','guard-confirm-hint':'Si tu protegido no puede acceder a su wallet, confirma su vitalidad para evitar que sus fondos vayan al escrow después de 910 días de inactividad.','guard-recover-btn':'🔓 RECUPERAR DEL ESCROW',
  'faq-title':'❓ Preguntas Frecuentes','faq-q1':'¿Están seguros mis datos biométricos?','faq-a1':'Tu rostro se captura y se envía a servicios de comparación independientes: es la única forma de comprobar «una persona, una cuenta». Las imágenes se procesan y luego se descartan; no se almacenan. Lo que se conserva es una plantilla matemática: cifrada y dividida en partes entre validadores operados por separado, de modo que ningún validador tiene nunca una completa. Un límite honesto, dicho y no ocultado: el servicio que ejecuta la comparación sí conserva plantillas, porque compararlas las requiere.',
  'faq-q1b':'¿El registro demuestra que soy una persona real y única?','faq-a1b':'Mejor de lo que una clave de dispositivo pudo nunca, y todavía no demostrable como cifra. El rostro se compara con todos los registros existentes mediante servicios independientes que deben coincidir, así que la misma persona en un segundo teléfono sí se detecta, cosa que una clave de dispositivo nunca logró. Lo que falta es la tasa de error: el umbral no está calibrado con capturas reales, y eso requiere unos 1000 pares de impostores.',
  'faq-q2':'¿Puedo registrarme con una wallet diferente más adelante?','faq-a2':'No. Un registro queda ligado de forma permanente a una dirección wallet. Es intencionado: el nullifier derivado de tu rostro se gasta una sola vez, así que registrarse de nuevo con otra wallet sería una segunda identidad para la misma persona.',
  'faq-q3':'¿Qué pasa si pierdo mi teléfono?','faq-a3':'Tu AEQ permanece en tu wallet — está vinculado a tu clave privada, no a tu teléfono. Puedes acceder a tu wallet a través de MetaMask con tu frase semilla. La recuperación de la wallet es independiente del registro biométrico.',
  'path-title':'Elige Tu Camino','path-human-title':'Soy un Humano','path-human-desc':'Quiero registrarme, recibir 1.000 AEQ y unirme a la red de ingreso básico.','path-human-steps':'1. Descargar la App Android de Aequitas<br>2. Desbloquear con el bloqueo de pantalla de tu dispositivo (huella/rostro/PIN)<br>3. Conectar MetaMask<br>4. Recibir 1.000 AEQ al instante',
  'path-node-title':'Soy un Operador de Nodo','path-node-desc':'Quiero ejecutar un nodo completo, participar en la producción de bloques y ganar del pool de validadores del 40%.','path-node-steps':'1. Registrarse como humano (obligatorio)<br>2. Sin punto de entrada que configurar — las direcciones de validador vienen integradas<br>3. Desplegar en Contabo/Hetzner/cualquier VPS<br>4. Ganar diariamente del pool de validadores',
  'path-dev-title':'Soy un Desarrollador','path-dev-desc':'Quiero construir sobre Aequitas, integrar la API o contribuir al protocolo.','path-dev-steps':'1. JSON-RPC compatible con EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Métricas: /metrics (Prometheus)',
  'story-flow-title':'Diagrama de Flujo del Token AEQ','story-topo-title':'Topología de Red — Estado Actual',
  'swap-price-title':'AEQ / tUSD — Precio en Vivo','swap-price-desc':'Precio en tiempo real derivado de las reservas del pool (x·y=k). Se actualiza cada 8 segundos con nuevos datos del pool.','swap-price-empty':'Sin datos del pool aún — añade liquidez para ver el gráfico de precios.',
  'node-guide-lang-note':'Esta guía está en inglés. Una traducción en PDF está disponible en tu idioma con el botón de arriba.',
  'k-zkp':'Sistema ZKP','k-hash':'Sistema Hash','k-sybil-prot':'Protección Sybil',
  'soc-title':'💬 Redes Sociales','soc-sub':'Anuncios, el estado de la cadena y las preguntas incómodas &mdash; en público, en ambas.',
  'soc-x-desc':'Anuncios y lo que la cadena está haciendo realmente. Formato breve.','soc-tg-desc':'El grupo abierto: preguntas, operadores de nodos y ayuda para registrarse.',
  's-validators':'Validadores Activos',
  'expl-heading':'Explorador de Bloques',
},
ru:{
  'x-consensus-ghostdag-knightdag':'◆ Консенсус: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Код контракта',
  'x-demurrage-is-a-holding-cost':'Плата за простой — это издержка на хранение денег, отрицательный процент, который делает накопление дорогим, а обращение привлекательным. Есть исторический пример: эксперимент в Вёргле (Австрия, 1932) использовал такую валюту и снизил местную безработицу на 25 % за год. Национальный банк Австрии прекратил его именно потому, что он работал слишком хорошо и угрожал банковской монополии. Chiemgauer (Германия, 2003) построен на том же принципе и успешно обращается больше 20 лет. Aequitas применяет непрерывную плату 0,5 % в месяц, только после трёх месяцев бездействия.',
  'x-network-consensus':'→ Сеть / консенсус',
  'x-node-decentralization-roadmap':'План децентрализации узлов',
  'x-open-source-chain-logic':'Открытый код логики сети',
  'x-phase-0-now':'Этап 0 (сейчас):',
  'x-phase-1-100-humans':'Этап 1 (от 100 человек):',
  'x-phase-2-1-000-humans':'Этап 2 (от 1 000 человек):',
  'x-phase-3-10-000-humans':'Этап 3 (от 10 000 человек):',
  'x-protocol-mechanisms':'Механизмы протокола',
  'x-what-happens-to-aeq-when':'Что происходит с AEQ, когда человек умирает или навсегда теряет дееспособность? В Bitcoin и большинстве криптовалют утраченный кошелёк означает навсегда утраченное предложение — по оценкам, миллионы BTC недоступны безвозвратно. Aequitas решает это многоступенчатым восстановлением при бездействии: если кошелёк долго не проявляет активности, его баланс постепенно возвращается сообществу через фонд базового дохода, чтобы реально обращающееся предложение сохраняло смысл.',
  'x-what-if-someone-is-hospitalized':'А если человек в больнице, в заключении или месяцами не имеет доступа к устройству? Доверенное лицо — другой подтверждённый человек — может подтвердить, что владелец кошелька жив, и тем самым не дать его AEQ уйти на депонирование. Финансового доступа у него нет вовсе: он может вызвать лишь одну функцию, обнуляющую счётчик бездействия. Ни при каких обстоятельствах он не может переводить, тратить или просматривать средства.',
  'bv-bind':'🔗 Создать подпись привязки',
  'bv-check-d':'Второй вызов перечисляет всех верификаторов и сравнивает их: у всех ли одинаковое число записей, не потерялось ли где-то зерно и совпадают ли ключи. Если ваша запись показывает расхождение, лучше узнать об этом здесь, чем посреди чьей-то регистрации.',
  'bv-check-t':'Проверка, что всё работает',
  'bv-desc':'Узел, производящий блоки, защищает <strong style="color:var(--text)">реестр</strong>. Биометрический верификатор защищает другое: обещание, что <strong style="color:var(--neon)">каждый человек регистрируется лишь один раз</strong>. Это разные роли — можно взять одну или обе на одной машине.',
  'bv-guide-sub':'Шаг за шагом &middot; Знание криптографии не требуется &middot; Около 30 минут, большей частью загрузка',
  'bv-honest-d':'Эта часть в бете, и ограничения настоящие. Совместное сравнение расходует одноразовый криптографический материал, и одной поставки пока хватает на несколько десятков регистраций — то есть конфиденциальный путь сперва доказывает себя в малом, а не на миллионах. Работа растёт и с числом зарегистрированных. Мы публикуем эти цифры, а не округляем их: система, которая просит ваше лицо, не вправе быть расплывчатой в том, что она умеет и чего пока нет.',
  'bv-honest-t':'Как обстоит дело сегодня — без прикрас',
  'bv-need-1':'<strong style="color:var(--text)">Зарегистрированный аккаунт Aequitas.</strong> То же правило, что и для производства блоков, и по той же причине: один человек — один ключ. Без этого один человек мог бы незаметно стать целым комитетом.',
  'bv-need-2':'<strong style="color:var(--text)">Небольшой сервер Linux с Docker.</strong> Хватит 2 ГБ памяти. Видеокарта не нужна — сравнение это арифметика над 64 байтами. Подойдёт та же машина, где уже работает ваш узел.',
  'bv-need-3':'<strong style="color:var(--text)">Доменное имя с HTTPS.</strong> Другие члены комитета должны до вас достучаться. Достаточно поддомена того, чем вы уже владеете.',
  'bv-need-4':'<strong style="color:var(--text)">Оставаться на связи.</strong> Чтобы регистрация завершилась, ответить должен каждый член комитета. Верификатор, которого часто нет, тормозит людей вместо того, чтобы их защищать.',
  'bv-need-t':'Прежде чем начать — что понадобится',
  'bv-s1-note':'Приватную половину держите на своём сервере и больше нигде. Открытая предназначена для передачи — по ней другие проверяют, что вы что-то засвидетельствовали. <strong style="color:var(--text)">Ваше собственное зерно проекции важно:</strong> поскольку у каждого верификатора оно своё, украденная у одного база не сопоставима с базой другого. Потеряете зерно — сохранённые доли утратят смысл, поэтому держите резервную копию там, где распоряжаетесь вы.',
  'bv-s1-t':'Шаг 1 — Создайте собственные ключи',
  'bv-s1-warn-d':'Два верификатора с одним и тем же секретом считаются одним, и комитет окажется меньше, чем выглядит. Никто — включая нас — не должен присылать вам ключ.',
  'bv-s1-warn-t':'Создайте их сами. Никогда не принимайте ключи ни от кого.',
  'bv-s2-d':'Поместите значения из шага 1 в файл, читаемый только вами. По одному значению в строке, без кавычек.',
  'bv-s2-note':'<strong style="color:var(--gold)">Оставьте ALLOW_REAL_BIOMETRIC_DATA в false</strong>, пока не прочтёте заметки о защите данных. При выключенном значении ваш верификатор входит в сеть и участвует в тестовых регистрациях, ни разу не сохраняя данные настоящего человека. Это правильный старт, и спешить с изменением незачем.',
  'bv-s2-t':'Шаг 2 — Напишите файл настроек',
  'bv-s3-note':'Здоровый ответ сообщает <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> и <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. Первое — это утверждение, что целый шаблон нигде не хранится, в форме, которую вы можете проверить сами, а не принять на веру. Проверьте сейчас и потом ещё раз: это ваша гарантия не меньше, чем чужая.',
  'bv-s3-t':'Шаг 3 — Запустите верификатор',
  'bv-s4-d':'Другие члены комитета достигают вас через открытый интернет, поэтому порт не должен быть доступен без шифрования. Caddy получает сертификат самостоятельно.',
  'bv-s4-t':'Шаг 4 — Поставьте впереди HTTPS',
  'bv-s5-d':'Производители блоков привязывают свой ключ к зарегистрированному кошельку: кошелёк подписывает <strong style="color:var(--text)">Aequitas: authorize validator &lt;адрес&gt;</strong>, без этого цепочка не даёт слот. Кнопка ниже создаёт именно эту подпись — для роли валидатора. <strong style="color:var(--text)">У ключа верификатора такой привязки пока нет.</strong> Его публичная половина собирается вне цепочки (шаг 6) и добавляется в список, который проверяет каждый proof-сервер. Ничто в цепочке не связывает его с человеком. Пока этого нет, комитет считает машины, а не людей, и один оператор может держать несколько. Мы предпочитаем сказать это здесь, чем позволить числу выглядеть сильнее, чем оно есть.',
  'bv-s5-t':'Шаг 5 — Что связывает ключ с человеком (и что пока нет)',
  'bv-s6-d':'Отправьте в группу <strong style="color:var(--text)">открытую</strong> половину из шага 1 вместе со своим HTTPS-адресом. Её добавят в список, по которому сверяется каждый сервер доказательств, и с этого момента ваши свидетельства идут в кворум. На этом шаге с вашей машины не уходит ничего секретного — в этом и смысл разделения: приватная половина остаётся у вас навсегда, а открытая без неё бесполезна.',
  'bv-s6-t':'Шаг 6 — Опубликуйте открытый ключ',
  'bv-status-d':'Исходный код верификатора <strong style="color:var(--text)">пока не открыт</strong>, поэтому сегодня выполнить шаги ниже может не каждый. Мы публикуем их всё равно: замысел должен поддаваться проверке до запуска, а не после. Если хотите поднять свой — спросите в группе Telegram со стартовой страницы. Именно открытие этого репозитория превратит руководство из плана в приглашение, и это следующее, что мы вам должны.',
  'bv-status-t':'Статус: закрытая бета — прочтите перед началом',
  'bv-title':'Или станьте биометрическим верификатором — роль, которая делает уникальность децентрализованной',
  'bv-what-d':'Лицо вам никогда не отправляют. Ваша машина хранит одну <strong style="color:var(--text)">аддитивную долю</strong> 64-байтовой выжимки: сама по себе она неотличима от случайного шума, и никакое доступное вам вычисление не восстановит из неё лицо. Сравнение идёт совместно с другими членами вашего комитета, и никто из вас не узнаёт ничего, кроме ответа — <em>дубликат: да или нет</em>. Это не обещание о наших добрых намерениях, а свойство арифметики.',
  'bv-what-t':'Что бы вы хранили — и чего никогда не увидели бы',
  'bv-why-d':'Регистрация принимается лишь после того, как её засвидетельствовали <strong style="color:var(--text)">несколько разных верификаторов</strong>. Одного украденного ключа недостаточно — нападающему нужен целый комитет. А поскольку <strong style="color:var(--neon)">один человек может держать ровно один ключ валидатора</strong>, купить комитет — значит быть столькими людьми. При 100 верификаторах у того, кто контролирует 10, шанс владеть полным комитетом из трёх — меньше одного к 1000. Каждый присоединившийся уменьшает это число. Это единственное место, где количество участников <em>и есть</em> безопасность. <strong style="color:var(--text)">Этот расчёт предполагает одного человека на ключ верификатора.</strong> Для производства блоков цепочка это обеспечивает; для ключей верификатора пока нет (шаг 5). До тех пор число выше — верхняя граница безопасности, а не её измерение.',
  'bv-why-t':'Почему каждый новый верификатор делает сеть труднее для подрыва',
  'x-0-1-split-40-30':'0,1 % · распределение 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 человек. Скользящий потолок богатства 5x &#8594; 25x. Этап становления.',
  'x-0-8211-2-years':'0 &#8211; 2 года',
  'x-0-perfect-equality':'0 = полное равенство',
  'x-1-000-aeq-minted':'+1 000 AEQ выпущено',
  'x-1-000-aeq-per-human':'1 000 AEQ на человека',
  'x-1-000-aeq-will-be':'1 000 AEQ будут зачислены автоматически',
  'x-10-000-8211-1m-humans':'10 000 &#8211; 1 млн человек. Не менее 10 узлов. Полностью децентрализовано.',
  'x-100-8211-10-000-humans':'100 &#8211; 10 000 человек. Фиксированный потолок 25x. Свободное подключение узлов.',
  'x-100-maximum-concentration':'100 = предельная концентрация',
  'x-1m-humans-global-ubi-at':'Более 1 млн человек. Всемирный базовый доход в большом масштабе. Цель по Джини &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Ликвидность LP 30 %',
  'x-9679-treasury-10':'&#9679; Резерв 10 %',
  'x-9679-ubi-pool-20':'&#9679; Фонд базового дохода 20 %',
  'x-9679-validators-40':'&#9679; Валидаторы 40 %',
  'x-active-validators':'Активные валидаторы',
  'x-add-aequitas-chain-to-metamask':'Добавьте сеть Aequitas в MetaMask, чтобы видеть баланс AEQ, отправлять переводы и работать с контрактом V7 прямо из браузера или мобильного кошелька.',
  'x-admin-keys-or-governance-votes':'административные ключи или голосования',
  'x-aeq-activity':'АКТИВНОСТЬ AEQ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'BlockDAG Aequitas — ничего не пропадает зря',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Сеть Aequitas (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas воплощает это математически. Каждый подтверждённый человек получает ровно 1 000 AEQ &#8212; миллиардер или крестьянин, без исключений. Четыре механизма перераспределения не дают неравенству накапливаться бесконечно. Коэффициент Джини ведётся в сети в реальном времени.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — сеть доказательства человечности',
  'x-android-apk-direct-download':'APK для Android · прямая загрузка',
  'x-architecture':'Устройство',
  'x-automatic-on-chain':'автоматически, в сети',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (направленный ациклический граф)',
  'x-blockdag-parallel-production':'BlockDAG · параллельное производство',
  'x-blockdag-proof-of-humanity':'BlockDAG + доказательство человечности',
  'x-blue-score':'«синий счёт»',
  'x-both-blocks-are-kept-ghostdag':'Оба блока сохраняются — GHOSTDAG включает одновременный и продолжает учитывать его в каноническом порядке.',
  'x-canonical-winner':'канонический победитель',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Сопоставимо с США (0,41) или Францией (0,32). В пределах большинства развитых экономик. Перераспределение заметно сглаживает кривую.',
  'x-confirm-ward-is-alive':'✓ ПОДТВЕРДИТЬ, ЧТО ЧЕЛОВЕК ЖИВ',
  'x-core-technology':'Основа',
  'x-daily-ubi-returns-to-all':'ежедневный базовый доход возвращается всем подтверждённым людям',
  'x-demurrage-0-5-mo':'Плата за простой (0,5 %/мес.)',
  'x-device-bound-zk-proof-one':'ZK-доказательство, привязанное к устройству · одна регистрация на устройство',
  'x-diagonal-line-perfect-equality':'диагональ = полное равенство',
  'x-disconnect-wallet':'⊘ ОТКЛЮЧИТЬ КОШЕЛЁК',
  'x-distinct-proposers-recent-blocks':'Разные создатели, недавние блоки',
  'x-distribution':'📈 Распределение',
  'x-elliptic-curve':'Эллиптическая кривая',
  'x-entire-distribution':'всё распределение',
  'x-evm-compatible':'Совместимо с EVM',
  'x-fill-ghostdag-verdict-thin-ring':'Заливка = вердикт GHOSTDAG · тонкое кольцо = создатель · один столбец на высоту. Наведите курсор на блок для подробностей.',
  'x-generate-node-binding-signature':'🔗 Создать подпись привязки',
  'x-run-a-coordinator':'🚪 Запустить координатор',
  'co-title':'Или запустите координатор — дверь, через которую проходит каждый человек',
  'co-desc':'Координатор — это место, куда приходит человек: он выдаёт задание, распределяет снимок по верификаторам, считает их голоса и выдаёт свидетельство, на основании которого цепь выпускает средства. Долгое время существовал ровно один, а значит каждая регистрация в сети проходила через одну машину. Не потому, что чего-то не хватало, а потому, что никто не запустил второй.',
  'co-status-t':'Статус: закрытая бета — та же оговорка, что и для верификатора',
  'co-status-d':'Координатор находится в том же репозитории, что и верификатор, а этот репозиторий <strong style="color:var(--text)">пока не открыт</strong>. Поэтому сегодня не каждый сможет выполнить шаги ниже. Они опубликованы всё равно, по той же причине: замысел должен поддаваться проверке до развёртывания, а не после.',
  'co-power-t':'Что координатор может — и чего не может',
  'co-power-d':'Он <strong style="color:var(--text)">не может выдумать человека</strong>. Никакой bio_hash не существует, пока его не засвидетельствовали несколько разных верификаторов, а их ключей у координатора нет. Что он может — привязать <strong style="color:var(--text)">существующий</strong> bio_hash к кошельку: нечестный мог бы перенаправить начисление на выбранный им адрес. Это настоящее полномочие, оно растёт с каждым новым координатором, и тот, кто решает вопрос доверия, должен понимать разницу.',
  'co-safe-t':'Почему второй координатор вообще безопасен',
  'co-safe-d':'Так было не всегда. До августа 2026 года обещание <strong style="color:var(--text)">один человек — одна регистрация</strong> держалось на блокировке Redis внутри координатора, а два независимых координатора не делят Redis: две одновременные регистрации одного человека прошли бы обе. Теперь <strong style="color:var(--text)">каждый верификатор проверяет сам</strong>, перед собственной записью, не внесено ли уже это лицо. Гарантия больше не зависит ни от общей службы, ни от общего секрета, поэтому координатор может добавиться или исчезнуть, ничего не меняя.',
  'co-need-t':'Что вам понадобится',
  'co-need-d':'Зарегистрированный аккаунт Aequitas — то же правило, что при производстве блоков и при верификации: один человек, один ключ. Сервер с Docker и публичным HTTPS-адресом, потому что небезопасной странице браузер камеру не отдаст. И два собственных ключа, которые вы создаёте сами и которые никогда не покидают вашу машину: один подписывает ваши свидетельства, другой отображает адреса кошельков в маркеры.',
  'co-keys-t':'Никогда не принимайте ключ ни от кого — в том числе от нас',
  'co-keys-d':'Два координатора с одним ключом подписи — это не два координатора, а один с двумя адресами, и кворум, который должен защищать людей, выглядел бы выполненным, не будучи таковым. Создайте оба ключа на своей машине, своим источником случайности, и не выпускайте их наружу.',
  'co-auth-t':'Авторизация ключа — разрешение не требуется',
  'co-auth-d':'Пока ваш ключ не авторизован, верификаторы отклоняют всё, что он подписывает. Для авторизации нужны два доказательства и ничьё одобрение: ваш кошелёк подписывает, что за этим ключом стоит зарегистрированный человек, а координатор на своём сервере доказывает, что ключ действительно его. Первое вы получаете кнопкой выше, второе координатор создаёт сам. До августа 2026 года требовался ещё и общий секрет от нас — и именно он <em>был</em> разрешением. Его больше нет.',
  'co-pernode-t':'Реестр локален для узла, и это сделано намеренно',
  'co-pernode-d':'Авторизация, записанная на одном узле, не переходит на другие: для неё нет ни транзакции, ни рассылки. Реплицируемый список доверия был бы именно той центральной инстанцией, без которой построена эта система: каждый оператор сам решает, чьи свидетельства принимает его узел. Плата за это — вашу авторизацию нужно отправить каждому узлу, который должен её признать. Сама подпись переносима: подписываете один раз и рассылаете всюду; пропущенный узел просто продолжит вам отказывать.',
  'co-law-t':'Что вы узнаёте о других людях — и что из этого следует',
  'co-law-d':'Снимок проходит через вас; вы передаёте его дальше и ничего не сохраняете. Но только вы храните соответствие между адресом кошелька и маркером для тех, кто регистрируется через вас — поэтому ваш ключ маркера должен остаться вашим: при совместном использовании любой оператор мог бы вычислить маркер для любого публичного адреса и узнать, чьё это лицо. Это также значит, что вы становитесь <strong style="color:var(--text)">оператором персональных данных</strong> для этих людей. Не мы. Запросы на доступ, удаление и возражение приходят к вам, и это не формальность.',
  'co-limit-t':'Единственное ограничение, которое отсюда следует',
  'co-limit-d':'Удаление по адресу кошелька работает только у того координатора, где была сделана запись: ваш маркер зависит от вашего ключа, а другой координатор выведет для того же адреса другой. Поэтому «не найдено» из другого места означает «здесь не зарегистрирован», а не «не зарегистрирован» — и ответ так и говорит. Путь через собственный bio_hash, тот, что принадлежит самому человеку и не требует оператора, работает у любого координатора, потому что этот идентификатор не меняется.',
  'x-authorize-coordinator-key':'🔑 Авторизовать ключ координатора',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — единый верный порядок из запутанного графа',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Коэффициент Джини',
  'x-gini-coefficient-0-1':'Коэффициент Джини (0–1)',
  'x-gini-index-history':'История индекса Джини',
  'x-gini-target-scandinavian-level':'Цель по Джини (скандинавский уровень)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'Groth16 ZKP (нулевое разглашение)',
  'x-guardian-system-8212-human-failsafe':'Доверенное лицо &#8212; человеческая подстраховка для утраченных кошельков',
  'x-hash-wallet':'Хеш / кошелёк',
  'x-healthier-than-most-nations-on':'Здоровее, чем в большинстве стран мира. Сопоставимо со Скандинавией (0,27) и Германией (0,31). Потолок богатства и плата за простой удерживают справедливое распределение.',
  'x-higher-than-most-european-nations':'Выше, чем в большинстве европейских стран — сопоставимо с Бразилией (0,53) или Россией. Перераспределение работает с повышенной силой.',
  'x-honest-limitation':'Признанное ограничение:',
  'x-how-it-works':'Как это работает',
  'x-how-to-read-this-chart':'Как читать этот график:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'человек могли бы зарегистрироваться',
  'x-imagine-a-world-where-every':'«Представьте мир, где каждый человек на Земле &#8212; независимо от того, где он родился, на каком языке говорит и сколько денег было у его родителей &#8212; получает гарантированный ежедневный доход просто потому, что он человек. Не как подаяние. Как математическое право, соблюдаемое кодом, который не может отменить ни правительство, ни корпорация.»',
  'x-inactive-escrow':'Депонирование при бездействии',
  'x-inactivity-timeline':'Сроки бездействия',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (устойчив к квантовым атакам)',
  'x-key-protections':'Основные меры защиты:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — собственное развитие Aequitas за пределы GHOSTDAG с фиксированным K',
  'x-knightdag-secured':'· защищено KnightDAG',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Как в Скандинавии (~0,27)',
  'x-liquidity-pool-30':'Пул ликвидности (30 %)',
  'x-loading-blocks':'Загрузка блоков…',
  'x-loading-topology':'Загрузка топологии…',
  'x-loading-transactions':'Загрузка переводов…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Кривая Лоренца — распределение AEQ между людьми',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: если после регистрации баланс AEQ показывает 0, откройте Настройки → Сети → удалите сеть Aequitas → добавьте её заново через этот сайт',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: если после добавления AEQ показывает 0, удалите сеть и добавьте её снова кнопкой выше.',
  'x-money-exists-because-people-exist':'Деньги существуют потому, что существуют люди. Значит, каждый человек должен иметь равную долю — уже потому, что он человек.',
  'x-money-exists-because-people-exist-2':'«Деньги существуют потому, что существуют люди. Не больше и не меньше.»',
  'x-most-unequal-currency-ever':'Самая неравная валюта в истории',
  'x-multi-validator-network':'Сеть из нескольких валидаторов',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: пока незначимо',
  'x-no-snapshots-yet-first-one':'Записей пока нет — первая сохранится после следующего распределения.',
  'x-no-stake-blockchain':'Блокчейн без залога',
  'x-node-operator-guide-pdf':'📄 Руководство оператора узла (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET должен быть зарегистрированным человеком Aequitas',
  'x-one-human-one-wallet-1':'Один человек = один кошелёк = 1 000 AEQ',
  'x-p2p-protocol':'Протокол P2P',
  'x-paid-out-daily':'выплачивается ежедневно',
  'x-permanent-on-chain':'Постоянно · в сети',
  'x-phase-roadmap-8212-the-path':'План по этапам &#8212; путь к мировому масштабу',
  'x-phase-transitions-are-automatic-8212':'Переходы между этапами происходят автоматически &#8212; их запускают пороги числа людей, а исполняет контракт. Без голосований и административных ключей.',
  'x-planned-post-beta':'Запланировано (после беты)',
  'x-postgresql-persistent':'PostgreSQL (постоянное хранение)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Внесите ликвидность AEQ / tUSD и получайте 30 % всех комиссий за обмен, распределяемых ежедневно.',
  'x-recorded-after-each-ubi-distribution':'Записывается после каждого распределения базового дохода. Показывает, как меняется равенство по мере роста сети. Чем ниже, тем лучше — цель: Джини ниже 0,30.',
  'x-redistribution':'ПЕРЕРАСПРЕДЕЛЕНИЕ',
  'x-run-a-node':'⚙️ Запустить узел',
  'x-run-a-verifier':'⚙️ Запустить верификатор',
  'x-set-guardian':'🛡 НАЗНАЧИТЬ ДОВЕРЕННОЕ ЛИЦО',
  'x-swap-fees-0-1':'Комиссии за обмен (0,1 %)',
  'x-sybil-resistance-8212-current-state':'Защита от Sybil &#8212; как обстоит дело на самом деле',
  'x-the-4-redistribution-mechanisms':'Четыре механизма перераспределения',
  'x-the-core-innovation':'Главная мысль',
  'x-the-matching-threshold-has-not':'Порог совпадения ещё не откалиброван на реальных снимках',
  'x-the-vision-8212-a-global':'Замысел &#8212; всемирный протокол базового дохода',
  'x-the-year-is-2009-satoshi':'Идёт 2009 год. Сатоши Накамото выпускает Bitcoin. Впервые ценность может перейти от одного человека к другому без банка. Настоящая революция. Но почти сразу что-то идёт не так.',
  'x-this-is-not-a-0815':'Это не рядовой блокчейн, выдающий по одному блоку за раз. Aequitas работает на настоящем BlockDAG, упорядоченном GHOSTDAG — а с 2026 года защищённом KnightDAG, собственным адаптивным развитием. Именно от этого механизма в конечном счёте зависят каждый баланс, каждая выплата и каждый потолок богатства, чтобы существовала одна согласованная история.',
  'x-today-beta':'Сегодня (бета)',
  'x-today-this-verifies-one-device':'Сегодня это подтверждает устройство, а не отдельного человека',
  'x-traditional-blockchain-wasted-work':'Обычный блокчейн — потраченная впустую работа',
  'x-treasury-10':'Резерв (10 %)',
  'x-trusted-verified-human':'проверенный человек, которому доверяют',
  'x-two-validators-produce-at-once':'Два валидатора создают блок одновременно → один побеждает, другой отбрасывается — работа потеряна, и это ограничивает скорость, с которой сеть может безопасно работать.',
  'x-ubi-pool-20':'Фонд базового дохода (20 %)',
  'x-validators-pool-40':'Фонд валидаторов (40 %)',
  'x-view-source-on-github':'🐙 Посмотреть код на GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'Множитель потолка богатства — ползунок на этапе становления',
  'x-wealth-cap-overflow':'Превышение потолка богатства',
  'x-wealth-distribution-analysis':'Разбор распределения богатства',
  'x-what-happens-when-someone-is':'Что происходит, если человек попал в больницу, в заключение или умер? В большинстве криптосистем утраченный кошелёк утрачен навсегда. В Aequitas есть трёхслойное восстановление при бездействии.',
  'x-what-is-a-guardian':'Кто такое доверенное лицо?',
  'x-what-is-and-is-not':'Что является личным, а что нет:',
  'x-what-would-a-cryptocurrency-look':'«Как выглядела бы криптовалюта, задуманная с самого начала быть справедливой к каждому человеку?»',
  'x-why-a-normal-blockchain-isn':'Почему обычного блокчейна недостаточно',
  'x-worse-than-any-country-on':'Хуже, чем в любой стране мира (рекорд ЮАР: 0,63). Приближается к Bitcoin (0,85). Протокол вмешивается по максимуму — потолок и перераспределение на полную мощность.',
  'x-year-2-180d':'2-й год +180 дн.',
  'x-zk-device-key-proof':'ZK-доказательство ключа устройства',
  'swap-price-flat':'За этот период сделок не было — цена не менялась. График работает, просто рынок спокоен.',
  'mpc-optin-title':'Дополнительно — помощь в проверке повторных регистраций (подготовлено, ещё не работает)',
  'mpc-optin-desc':'Подготовлено, но пока не работает. Позже ваш узел сможет помогать проверять, что никто не регистрируется дважды, ни разу не видя биометрических данных: каждая сторона хранит лишь математическую долю каждого шаблона — сама по себе шум — и они сравнивают новый снимок вместе, так что ни одна машина не может ничего восстановить. Сегодня этот путь ничего не решает. Проверка на дубликаты через него не идёт, а комитет задан фиксированным списком, а не выбирается автоматически, поэтому три переменные пока ничего не меняют.',
  'mpc-optin-note':'Файл долей содержит одноразовую случайность, которую может хранить только ваш узел — никогда не копируйте его на другую машину и не помещайте в репозиторий. Пока его должен выдать оператор — это оставшаяся централизованная зависимость. Новый ключ не нужен: узел представляется остальным тем же ключом подписи, которым уже подписывает блоки.',
  'logo-sub':'ДОКАЗАТЕЛЬСТВО ЧЕЛОВЕЧНОСТИ','live':'ОНЛАЙН',
  'reg-title':'🔐 Зарегистрируйтесь как Верифицированный Человек',
  'reg-sub':'Присоединитесь к сети Aequitas и получите 1 000 AEQ в качестве Универсального Базового Дохода. Однократно, постоянно и полностью бесплатно. Никакие личные данные никогда не сохраняются.',
  'app-title':'РЕГИСТРАЦИЯ ТОЛЬКО ЧЕРЕЗ ANDROID-ПРИЛОЖЕНИЕ',
  'app-text':'При регистрации камера снимает ваше лицо и короткую последовательность для проверки живости. Независимые службы сравнения проверяют, что перед камерой живой человек и что это лицо ещё не зарегистрировано; они должны согласиться кворумом. Затем доказательство Groth16 переносит результат в цепочку, не раскрывая о вас ничего. Ваши <strong style="color:var(--gold)">1000 AEQ начисляются автоматически</strong> после проверки. <strong style="color:var(--gold)">Примечание:</strong> порог сравнения ещё не откалиброван на реальных снимках — см. FAQ ниже.',
  's1t':'Съёмка лица','s1d':'Приложение снимает ваше лицо и короткую последовательность для проверки живости и отправляет их независимым службам сравнения. Те проверяют, что перед камерой живой человек, и сравнивают лицо со всеми уже зарегистрированными. После обработки изображения удаляются.',
  's2t':'Создание ZK-Доказательства','s2d':'Доказательство Groth16 фиксирует ваш bio_hash в commitment = keccak256(bioHash‖wallet), не раскрывая его. Нуллификатор выводится из этого хеша, поэтому одно и то же лицо не может засчитаться дважды — см. FAQ ниже.',
  's3t':'Подключение Кошелька','s3d':'Приложение открывает MetaMask на этой странице · подключите кошелёк Ethereum · доказательство криптографически привязано к вашему адресу',
  's4t':'1 000 AEQ Зачислены','s4d':'Регистрация подтверждена на BlockDAG Aequitas за 1 секунду · 1 000 AEQ зачислены мгновенно · личность навсегда записана как верифицированный человек',
  'priv-bar':'🔒 Проверка лица кворумом · Groth16 ZKP · Изображения удаляются после проверки · Одна регистрация на человека',
  'conn-wallet':'ПОДКЛЮЧЁННЫЙ КОШЕЛЁК','proof-recv':'⚡ ZK-ДОКАЗАТЕЛЬСТВО ПОЛУЧЕНО','proof-hint':'Подключите кошелёк для регистрации',
  'btn-conn':'🦊 ПОДКЛЮЧИТЬ METAMASK','btn-reg':'🔐 ЗАРЕГИСТРИРОВАТЬ ОН-ЧЕЙН',
  'btn-wc':'🔗 ПОДКЛЮЧИТЬ WALLETCONNECT',
  'reg-log-hint':'// Откройте Android-приложение Aequitas для создания доказательства, затем вернитесь сюда...',
  'reg-details':'Детали Регистрации','k-network':'Сеть','k-chainid':'ID Цепи','k-grant':'Субсидия UBI',
  'k-fee':'Комиссия Gas','free':'БЕСПЛАТНО — полностью без комиссий','k-limit':'Регистрации','k-limit-v':'Один раз на человека · навсегда · неизменно',
  'k-bio':'Лицо','never-stored':'Изображения удаляются после проверки — ни один валидатор не хранит целый шаблон',
  'k-proof':'Система Доказательств','k-conf':'Подтверждение','k-conf-v':'В течение 1 секунды (1 блок)',
  'k-sybil':'Защита от Сибилл','k-sybil-v':'Одна личность на человека · привязка к лицу, порог ещё не откалиброван',
  's-height':'Высота Блока',
  's-humans':'Верифицированные Люди',
  's-supply':'Общий Объём','s-supply-sub':'Всегда = Люди × 1 000 AEQ',
  's-uptime':'Время Работы',
  'k-chain':'Имя Цепи','k-symbol':'Символ','k-btime':'Время Блока',
  'k-cons':'Консенсус','k-storage':'Хранилище','k-dec':'Десятичные',
  'btn-add-mm':'+ ДОБАВИТЬ СЕТЬ AEQUITAS',
  'humans-title':'Верифицированные Люди в Aequitas Chain',
  'h-what':'Что такое Верифицированный Человек?','h-what-t':'Верифицированный Человек — это адрес кошелька, для которого доказано, что он принадлежит человеку, чьё лицо ещё не зарегистрировано. Независимые службы сравнения должны согласиться кворумом, а в цепочку попадает только доказательство Groth16 — ни изображения, ни шаблона. <strong style="color:var(--gold)">До 23.08.2026 это подтверждало устройство, а не человека; больше это не так.</strong>',
  'h-zkp':'Система ZK-Доказательств','h-zkp-t':'Aequitas использует Groth16 на BN128 — та же кривая, что Ethereum и Zcash. ~200 байт, ~10мс. commitment = keccak256(deviceKey‖wallet). Nullifier привязан к этому устройству: потеря телефона не создаёт вторую идентичность на нём, но другое устройство может зарегистрироваться отдельно. Материал ключа никогда не раскрывается и не хранится на сервере.',
  'h-sybil':'Устойчивость к Sybil — Текущее Состояние','h-sybil-t':'Нуллификатор выводится из bio_hash вашего лица, поэтому одно и то же лицо нельзя зарегистрировать дважды — в том числе с разных устройств, чего ключ устройства никогда не мог. Опирается это на порог сравнения, ещё не откалиброванный на реальных снимках: криптография точна, биометрия под ней — измерение с неизвестной частотой ошибок.',
  'h-global':'Глобальная Финансовая Инклюзия','h-global-t':'Не нужны ни банковский счёт, ни кредитная карта, ни опыт с криптовалютами. Достаточно Android-смартфона с камерой. Aequitas задуман так, чтобы быть доступным каждому человеку на Земле.',
  'h-bio-hw':'Дорожная Карта Верификации Личности','h-bio-hw-t':'Сегодня (бета): проверка лица независимыми службами сравнения, которые должны согласиться кворумом. Её порог ещё не откалиброван на реальных снимках — для этого нужно около 1000 импостор-пар, прежде чем называть какое-либо число. Планируется: эта калибровка и проверка дубликатов, при которой ни одна служба не хранит целый шаблон.',
  'reg-humans':'Зарегистрированные Люди','h-desc':'Каждый адрес ниже принадлежит человеку, чьё лицо независимые службы сверили со всеми существующими регистрациями, подтвердили ZK-доказательством и зачислили ровно 1000 AEQ. Реестр постоянный, неизменяемый и в цепочке. Что порог сегодня гарантирует, а что нет — в FAQ.',
  'no-humans':'Люди ещё не зарегистрированы.\n\nСкачайте Android-приложение Aequitas и будьте первым!',
  'reg-stats':'Статистика Реестра','total-humans':'Всего Людей',
  'idx-title':'Индекс Aequitas — Оценка Экономического Равенства в Реальном Времени',
  'idx-desc':'Индекс Aequitas измеряет экономическое неравенство всех верифицированных людей в реальном времени. Рассчитывается из коэффициента Джини распределения балансов он-чейн. 0 = идеальное равенство. 100 = максимальное неравенство.',
  'curr-idx':'Текущий Индекс','bar-0':'0 — Идеальное Равенство','bar-100':'100 — Макс. Неравенство','wcap-lbl':'Текущий Потолок Богатства:','wcap-mult':'Множитель:','wcap-avg':'Справедливая доля:',
  'gini':'Коэффициент Джини','gini-desc':'0 = равно · 1 = неравно',
  'supply-desc':'Всегда = Люди × 1 000 AEQ',
  'phase':'Фаза Протокола','phase-desc':'Автоматически по количеству людей',
  'humans-desc':'Регистрации с проверкой лица',
  'pools-title':'Пулы Перераспределения',
  'pools-desc':'Каждая комиссия свопа, плата за демередж и превышение лимита богатства автоматически делится между четырьмя пулами. Все пулы выплачивают ежедневно.',
  'vel-pool':'Пул Валидаторов','vel-pool-desc':'40% всех комиссий → операторы нод, обеспечивающие сеть',
  'liq-pool':'Пул Ликвидности','liq-pool-desc':'30% всех комиссий → поставщики ликвидности, пропорционально LP-долям',
  'ubi-pool':'Пул UBI','ubi-pool-desc':'20% всех комиссий → все верифицированные люди поровну, каждые 24 часа',
  'treasury':'Казначейство','treasury-desc':'10% всех комиссий → разработка и обслуживание протокола',
  'phases-title':'Фазы Протокола',
  'phases-desc':'В Фазе 0 (Bootstrap) применяется скользящий множитель: max(5, min(N, 25))× средний баланс. При 1–4 людях: 5× средний. Каждый новый человек прибавляет 1×. При 25+ людях: фиксируется навсегда на 25×. Фаза 1+ сохраняет 25× фиксированным. Переходы автоматические — без голосования, без административных ключей.',
  'p0':'Bootstrap · &lt;100 людей · Лимит богатства: max(5,min(N,25))× средний · Скользит 5×→25× до 25-го человека · Сейчас активен',
  'p1':'Рост · 100–10 000 людей · Лимит богатства: 25× справедливой доли = 25 000 AEQ',
  'p2':'Стабильность · 10 000–1М людей · Лимит богатства: 25× справедливой доли = 25 000 AEQ',
  'p3':'Зрелость · 1М+ людей · Лимит богатства: 25× справедливой доли = 25 000 AEQ',
  'wealth-cap-explain':'В Фазе 0 (Bootstrap) Лимит Богатства = max(5, min(N, 25))× средний баланс AEQ, где N = количество зарегистрированных людей. 1–4 человека: 5× средний. Каждый новый человек прибавляет 1×. 25+ людей: фиксируется навсегда на 25×. Лимит всегда привязан к актуальному среднему балансу.',
  'demurrage-title':'Демередж — Стимул к Обращению',
  'demurrage-desc':'Aequitas реализует механизм демереджа, вдохновлённый историческими дополнительными валютами. Бездействующие балансы AEQ постепенно теряют стоимость для предотвращения накопления.',
  'dem-rate-k':'Скорость Распада','dem-rate-v':'0,5% в месяц (непрерывно)',
  'dem-grace-k':'Льготный Период','dem-grace-v':'3 месяца бездействия до начала распада',
  'dem-reset-k':'Сброс Таймера','dem-reset-v':'Любой перевод, своп или операция с ликвидностью сбрасывает таймер',
  'dem-dest-k':'Распавшийся AEQ идёт в','dem-dest-v':'Пулы перераспределения (40/30/20/10)',
  'dem-warn-k':'Система Предупреждений','dem-warn-v':'14-дневное уведомление (один раз) + 7-дневное повторение при каждом входе',
  'story-title':'История Aequitas — Почему это существует',
  'nodes-title':'Активные Ноды — Текущая Топология Сети','nodes-desc':'Сеть Aequitas работает на нескольких географически распределённых нодах (текущее число выше). Все они участвуют в производстве блоков и синхронизации. Сеть рассчитана на дополнительные ноды.',
  'run-node-title':'Запустите Свою Ноду — Помогите Защитить Сеть',
  'run-node-desc':'Любой зарегистрированный человек может запустить узел Aequitas — без стейка, без заявки, без нашего разрешения. Один человек — один ключ валидатора: узел, чей NODE_OPERATOR_WALLET не является зарегистрированным человеком, отклоняется с HTTP 403, иначе один человек мог бы незаметно стать всем набором валидаторов. Операторы нод получают 40% всех комиссий свопа ежедневно через Пул Валидаторов.',
  'bootstrap-title':'Подключить Новую Ноду','bootstrap-desc':'Настраивать точку входа не нужно — адреса валидаторов встроены. Нода регистрируется сама, автоматически синхронизируется и начинает производство блоков. PRIMARY_NODE_URL — только если вы намеренно хотите закрепить конкретную точку входа.',
  'tech-title':'Технические Характеристики','mm-config':'Конфигурация MetaMask',
  'k-lang':'Язык','k-src':'Исходный Код','evm-yes':'Да — JSON-RPC /rpc · Совместимо с MetaMask',
  'proto-label':'Протокол Aequitas V7 — Техническая Документация',
  'ca-title':'Адреса Контрактов','ca-text':'Цепь: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 задаёт правила экономики Aequitas и ведёт реестр, который делает их исполнимыми: каждый когда-либо заявленный nullifier, каждую регистрацию, ограничение богатства и формулу демереджа. Контракт неизменяем — ни ключа администратора, ни прокси обновления, ни голосования по управлению, способного изменить хотя бы строку. Однако реальный перевод исполняет слой цепи: узел перехватывает вызов ERC-20 до того, как он достигнет EVM, и применяет его к собственной книге учёта — именно это делает переводы мгновенными и без комиссии за газ. Контракт — это свод правил и реестр; цепь — механизм, который их исполняет, и её исходный код открыт.<br><br>Контракт BioVerifier получает доказательства с нулевым разглашением Groth16 сгенерированные полностью на Android-устройстве пользователя. Он математически проверяет on-chain примерно за 10 мс что отправленный nullifier был корректно выведен из секрета, которым владеет регистрант, и цепь отклоняет любой уже виденный nullifier — не узнавая никогда его имени, личности или биометрических данных. Именно это исключает повторную регистрацию из того же источника личности; является ли этот источник человеком или устройством, зависит от того, включён ли биометрический режим. Именно это делает возможной безгазовую регистрацию без инвестиций: доказательство — единственное что когда-либо покидает устройство.<br><br>Именно это сочетание и является новым: правила и реестр «один человек — одна регистрация» находятся в контракте, который никто не может переписать — ни оператор, ни компания, ни государство, — а исполняющий их код открыт и воспроизводим из этого репозитория. Всё это может проверить любой. Доверия по-прежнему требует работа самих узлов, и честный способ уменьшить его — больше независимых валидаторов, а не более сильная фраза здесь.',
  'poa-title':'1. ДОКАЗАТЕЛЬСТВО ЖИЗНИ — Восстановление Неактивных Балансов','poa-text':'<p>Что происходит с AEQ когда люди умирают или становятся недееспособными? В Bitcoin потерянные кошельки означают навсегда потерянный объём. Aequitas решает это через многоуровневую систему: если кошелёк не проявляет активности в течение длительного периода, его баланс постепенно возвращается сообществу через пул UBI.</p>',
  'poa-box':'Год 0–2: Обычное использование — без ограничений<br>Год 2: Предупреждение 1 — Guardian может ответить от имени<br>Год 2+60д: Предупреждение 2 — нарастающая срочность<br>Год 2+120д: Предупреждение 3 — последнее уведомление<br>Год 2+180д: AEQ перемещён в личный ЭСКРОУ (ещё восстановимо)<br>Год 4: При сохранении бездействия — ЭСКРОУ в Пул UBI',
  'guard-title':'2. СИСТЕМА GUARDIAN — Человеческая Защита','guard-text':'<p>Что если кто-то госпитализирован или иначе не может получить доступ к устройству месяцами? Система Guardian позволяет доверенному лицу — другому верифицированному человеку — подтвердить что владелец кошелька жив. Guardian имеет строго нулевой финансовый доступ: он может только сбросить таймер бездействия.</p>',
  'guard-box':'1 Guardian на человека · должен быть верифицированным человеком в Aequitas<br>Guardian может ТОЛЬКО вызывать confirmAlive() — ноль прав транзакций<br>Guardian НЕ МОЖЕТ перемещать средства, переводить AEQ или получать доступ к кошельку<br>Максимум 3 подопечных · Блокировка 7 дней при назначении · Без круговых отношений',
  'dem-title':'3. ДЕМЕРЕДЖ — Механизм Против Накопления',
  'dem-box':'Взимается только с части сверх вашей справедливой доли — баланс на уровне доли или ниже никогда не уменьшается<br>Ставка: 0,5%/месяц после 3 месяцев бездействия (непрерывно, не ступенчато)<br>Таймер сбрасывается при любом переводе, свопе или операции с ликвидностью<br>Decayed AEQ перераспределяется в пулы — никогда не сжигается',
  'dem-text':'<p>Демередж — стоимость хранения денег. Эксперимент Вёрглена (Австрия, 1932) сократил местную безработицу на 25% за год. Chiemgauer (Германия, 2003) работает по тому же принципу уже более 20 лет.</p>',
  'cap-title':'4. ЛИМИТ БОГАТСТВА — Математическое Обеспечение Справедливости','cap-box':'Bootstrap-лимит: max(5,min(N,25))× текущий средний баланс<br>1–4 людей: 5× · +1× за человека · 25+: 25× навсегда<br>Применяется ко всем адресам кроме 4 протокольных пулов<br>Избыток AEQ мгновенно перераспределяется · Без ручного вмешательства',
  'ubi-title':'5. УНИВЕРСАЛЬНЫЙ БАЗОВЫЙ ДОХОД — Ежедневное Перераспределение','ubi-box':'Источники: Комиссии свопов (20%) · Превышение лимита богатства · Демередж · Эскроу после 4 лет<br><br>Ежедневно: весь пул UBI делится поровну между всеми зарегистрированными людьми. Пул сбрасывается и сразу наполняется снова.',
  'inf-title':'6. НИКАКОЙ АЛГОРИТМИЧЕСКОЙ ИНФЛЯЦИИ — Фиксированная Формула','inf-box':'ЕДИНСТВЕННОЕ событие создающее новый AEQ: регистрируется новый верифицированный человек.<br><br>Общий Объём = Верифицированные Люди × 1 000 AEQ<br><br>Это не политика — обеспечивается протоколом. AEQ — единственная криптовалюта где объём определяется исключительно числом верифицированных живых людей.',
  'phases-desc':'В Фазе 0 лимит богатства использует скользящий Bootstrap-множитель: max(5, min(N, 25))× средний баланс. При 1–4 людях: 5× средний. Каждый новый человек прибавляет 1×. При 25+ людях: фиксируется навсегда на 25×. Фаза 1+ сохраняет 25× фиксированным. Переходы автоматические — без голосования, без административных ключей.',
  'p0':'Bootstrap · &lt;100 людей · Лимит богатства: max(5,min(N,25))× средний · Скользит 5×→25× до 25-го человека · Сейчас активен',
  'p1':'Рост · 100–10 000 людей · Лимит богатства: 25× справедливой доли = 25 000 AEQ',
  'p2':'Стабильность · 10 000–1M людей · Лимит богатства: 25× (планируемое снижение: 10×)',
  'p3':'Зрелость · 1M+ людей · Лимит богатства: 25× (планируемое снижение: 5×)',
  'wealth-cap-explain':'Лимит богатства в настоящее время установлен на 25× среднего баланса AEQ всех верифицированных людей. Это фиксированная константа в живом коде Go. Поскольку значение всегда относительно текущего среднего, лимит автоматически масштабируется по мере роста сети.',
  'btn-download-app':'СКАЧАТЬ ПРИЛОЖЕНИЕ AEQUITAS',
  'swap-title':'🔄 Обмен AEQ ↔ tUSD','swap-sub':'Обменивайте AEQ на tUSD (симулированный тестовый доллар) через нативный пул ликвидности. Комиссия 0,1% применяется только к свопам — обычные переводы AEQ между людьми остаются полностью бесплатными.',
  'swap-priv-bar':'🔒 Только 0,1% комиссия свопа · Переводы AEQ-AEQ бесплатны · tUSD — тестовая валюта без реальной стоимости',
  'swap-your-aeq':'Ваш AEQ','swap-your-tusd':'Ваш tUSD',
  'swap-fee-est':'Комиссия протокола (0,1%)','swap-details-hdr':'Детали Свопа',
  'swap-out-lbl':'Вы получите (прим.)','swap-impact-lbl':'Влияние на цену','swap-rate-lbl':'Обменный курс',
  'swap-depth-lbl':'Состав Пула','amm-title':'x × y = k — AMM с Постоянным Произведением',
  'amm-text':'Когда вы обмениваете AEQ на tUSD, резерв AEQ растёт, а резерв tUSD уменьшается — их произведение всегда равно k. Более крупные свопы вызывают большее влияние на цену. Комиссия 0,1% вычитается до применения формулы.',
  'swap-btn-go':'🔄 ОБМЕНЯТЬ',
  'swap-log-hint':'// Подключите кошелёк для обмена...',
  'swap-no-liquidity':'Нет tUSD?','swap-faucet-desc':'Зарегистрированные люди могут получить тестовый tUSD один раз','swap-btn-faucet':'💧 ПОЛУЧИТЬ ТЕСТОВЫЙ tUSD',
  'swap-addliq-title':'Предоставить Ликвидность','swap-addliq-desc':'Будьте первым кто внесёт — ваше соотношение устанавливает начальную цену.','swap-btn-addliq':'💧 ДОБАВИТЬ ЛИКВИДНОСТЬ',
  'swap-lp-title':'Ваша LP-Позиция','swap-lp-share':'Доля в Пуле','swap-lp-withdrawable':'Доступно к выводу',
  'swap-lp-pct-label':'% вашей позиции','swap-lp-youget':'Вы получите','swap-btn-removeliq':'🔥 ВЫВЕСТИ ЛИКВИДНОСТЬ',
  'swap-pool-title':'AEQ / tUSD — Статус Пула',
  'swap-pool-aeq':'Резерв AEQ','swap-pool-tusd':'Резерв tUSD','swap-pool-price':'Спотовая Цена',
  'swap-fee-bps':'Комиссия Свопа',
  'swap-pools-addr-title':'Адреса Пулов Токеномики',
  'swap-validators':'Валидаторы (40%)','swap-lps':'Провайдеры Ликвидности (30%)','swap-ubi':'Пул UBI (20%)','swap-treasury':'Казначейство (10%)',
  'ubi-hero-title':'УНИВЕРСАЛЬНЫЙ БАЗОВЫЙ ДОХОД — ПУЛ UBI',
  'ubi-hero-sub':'Накапливается — следующая выплата поровну всем верифицированным людям через:',
  'ubi-bal-lbl':'текущий баланс пула','ubi-hero-desc':'Делится поровну между всеми · выплачивается каждые 24ч · пул обнуляется после выплаты · минимальный баланс не требуется',
  'ubi-how-fills':'Как заполняется Пул UBI',
  'ubi-src-swap':'Комиссии Свопов','ubi-src-swap-d':'Каждый своп AEQ↔tUSD вносит 20% своей комиссии 0,1%. Больше торговли = быстрее заполнение.',
  'ubi-src-dem':'Демередж','ubi-src-dem-d':'Неактивный AEQ (3+ месяца) убывает со скоростью 0,5%/месяц. 20% убывшей суммы идёт в UBI.',
  'ubi-src-cap':'Превышение Лимита Богатства','ubi-src-cap-d':'Кошельки превышающие лимит (max(5,min(N,25))× средний) конфискуются мгновенно. 20% поступает в UBI немедленно.',
  'pools4-header':'Все четыре пула перераспределения',
  'ubi-see-above':'см. обратный отсчёт выше','ubi-timer-above':'⏰ обратный отсчёт показан выше','pool-t-timer':'Накапливается — без таймера',
  'usp-headline':'Впервые в истории — все начинают на равных',
  'usp-sub':'Если у вас есть Android-смартфон — вы квалифицируетесь. Без банка, без знаний крипто, без инвестиций.',
  'usp-c1-title':'0,00 стартовых инвестиций','usp-c1-desc':'Регистрация полностью без газа. Без ETH, без MATIC, без кредитной карты. Протокол оплачивает все транзакционные сборы.',
  'usp-c2-title':'1 000 AEQ для каждого человека','usp-c2-desc':'Миллиардер или фермер — все получают ровно 1 000 AEQ. Не больше, не меньше. Равный старт, гарантированный математически.',
  'usp-c3-title':'Доступно для всех','usp-c3-desc':'Без банковского счёта, без кредитной карты, без удостоверения личности, без дополнительного оборудования — только камера, которая уже есть в вашем Android-телефоне.',
  'usp-c4-title':'Ежедневный UBI навсегда','usp-c4-desc':'После регистрации вы автоматически получаете ежедневную долю выплат UBI — каждый день, без каких-либо действий.',
  'v7-intro-title':'Что такое AequitasV7?',
  'v7-intro-text':'AequitasV7 — центральный смарт-контракт протокола Aequitas. "V7" — 7-я основная версия контракта справедливости. Развёрнут неизменяемым образом в Aequitas Chain (ID 1926) и управляет всем: регистрация людей, верификация ZK, управление балансами, лимит богатства, распределение UBI, комиссии свопов. Ни один администратор не может обновить его. Шесть механизмов образуют самоусиливающуюся систему.',
  'swap-sell-label':'Продать','swap-receive-label':'Получить',
  'gini-what-title':'Что такое коэффициент Джини?','gini-what-text':'Разработан итальянским статистиком Коррадо Джини (1912). Измеряет распределение богатства, сравнивая фактические балансы с гипотетически равным базовым уровнем. Шкала: 0 (у всех одинаково) до 1 (у одного всё). Используется Всемирным банком, ОЭСР, ООН для сравнения стран. Справочные значения: Bitcoin ≈ 0,85 · ЮАР (мировой рекорд) ≈ 0,63 · США ≈ 0,41 · Германия ≈ 0,31 · Скандинавия ≈ 0,27 · Долгосрочная цель Aequitas: Джини ниже 0,30.','gini-calc-title':'Как рассчитывается Индекс Aequitas','gini-calc-text':'Собираются все балансы AEQ. Формула вычисляет среднее абсолютное отклонение нормализованное на n2. Результат 0-1 x 100 = Индекс.','gini-why-title':'Почему Gini','gini-why-text':'Gini учитывает полное распределение среди всех людей в одном числе.',
  'guard-title':'🛡 Система Хранителя','guard-my-lbl':'Мой Хранитель','guard-none':'Нет',
  'guard-set-lbl':'Установить / Изменить Хранителя','guard-set-hint':'Должен быть зарегистрированным человеком Aequitas · Блокировка на 7 дней · Хранитель может только подтвердить вашу активность, не имея доступа к средствам · Макс. 3 подопечных на хранителя',
  'guard-confirm-lbl':'Подтвердить Активность (Как Хранитель)','guard-confirm-hint':'Если ваш подопечный не может получить доступ к кошельку, подтвердите его активность, чтобы предотвратить перевод средств на эскроу после 910 дней бездействия.','guard-recover-btn':'🔓 ВЕРНУТЬ ИЗ ЭСКРОУ',
  'faq-title':'❓ Вопросы и Ответы','faq-q1':'Мои биометрические данные в безопасности?','faq-a1':'Ваше лицо снимается и отправляется независимым службам сравнения — только так вообще можно проверить «один человек — один счёт». Изображения обрабатываются и затем удаляются, они не хранятся. Хранится математический шаблон: зашифрованный и разделённый на доли между отдельно управляемыми валидаторами, так что ни один валидатор никогда не держит целый. Честное ограничение, названное, а не скрытое: служба, выполняющая сравнение, шаблоны всё же хранит, потому что сравнение их требует.',
  'faq-q1b':'Доказывает ли регистрация, что я уникальный реальный человек?','faq-a1b':'Лучше, чем когда-либо мог ключ устройства, и пока не выражается числом. Лицо сравнивается со всеми существующими регистрациями независимыми службами, которые должны согласиться, поэтому тот же человек со второго телефона будет замечен — чего ключ устройства никогда не мог. Не установлена частота ошибок: порог не откалиброван на реальных снимках, для этого нужно около 1000 импостор-пар.',
  'faq-q2':'Могу ли я зарегистрироваться с другим кошельком позже?','faq-a2':'Нет. Регистрация навсегда привязана к одному адресу кошелька. Это сделано намеренно: нуллификатор, выведенный из вашего лица, тратится один раз, поэтому повторная регистрация на другой кошелёк была бы второй личностью того же человека.',
  'faq-q3':'Что произойдёт, если я потеряю телефон?','faq-a3':'Ваши AEQ остаются в кошельке — они привязаны к вашему приватному ключу, а не к телефону. Доступ к кошельку возможен через MetaMask с помощью сид-фразы. Восстановление кошелька не зависит от биометрической регистрации.',
  'path-title':'Выберите Свой Путь','path-human-title':'Я Человек','path-human-desc':'Хочу зарегистрироваться, получить 1 000 AEQ и присоединиться к сети базового дохода.','path-human-steps':'1. Скачать Android-приложение Aequitas<br>2. Разблокировать экраном блокировки устройства (отпечаток/лицо/PIN)<br>3. Подключить MetaMask<br>4. Получить 1 000 AEQ мгновенно',
  'path-node-title':'Я Оператор Ноды','path-node-desc':'Хочу запустить полную ноду, участвовать в производстве блоков и зарабатывать из 40%-ного пула валидаторов.','path-node-steps':'1. Зарегистрироваться как человек (обязательно)<br>2. Ничего не настраивать — адреса валидаторов встроены<br>3. Развернуть на Contabo/Hetzner/любом VPS<br>4. Ежедневно зарабатывать из пула валидаторов',
  'path-dev-title':'Я Разработчик','path-dev-desc':'Хочу создавать на базе Aequitas, интегрировать API или вносить вклад в протокол.','path-dev-steps':'1. EVM-совместимый JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* эндпоинты<br>4. Метрики: /metrics (Prometheus)',
  'story-flow-title':'Схема движения токена AEQ','story-topo-title':'Топология Сети — Текущее Состояние',
  'swap-price-title':'AEQ / tUSD — Живая Цена','swap-price-desc':'Цена в реальном времени из резервов пула (x·y=k). Обновляется каждые 8 секунд с новыми данными пула.','swap-price-empty':'Данных пула ещё нет — добавьте ликвидность для просмотра графика цены.',
  'node-guide-lang-note':'Это руководство на английском. Перевод доступен в PDF на вашем языке — используйте кнопку выше.',
  'k-zkp':'ZKP-Система','k-hash':'Хеш-Система','k-sybil-prot':'Защита от Sybil',
  'soc-title':'💬 Социальные сети','soc-sub':'Объявления, состояние сети и неудобные вопросы &mdash; публично, в обеих.',
  'soc-x-desc':'Объявления и то, чем сеть занята на самом деле. Коротко.','soc-tg-desc':'Открытая группа: вопросы, операторы узлов и помощь с регистрацией.',
  's-validators':'Активные валидаторы',
  'expl-heading':'Обозреватель блоков',
},
zh:{
  'x-consensus-ghostdag-knightdag':'◆ 共识：GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'合约代码',
  'x-demurrage-is-a-holding-cost':'滞留费是持有货币的成本 —— 一种负利率，使囤积变贵、流通变得有吸引力。历史上有先例：沃格尔实验（奥地利，1932）采用带滞留费的货币，一年内使当地失业率下降 25 %。奥地利国家银行终止了它，恰恰因为它成效太好，威胁到银行的垄断。Chiemgauer（德国，2003）基于同样的原理，已成功流通二十多年。Aequitas 实行每月 0.5 % 的连续滞留费，且仅在三个月不活跃之后才开始计收。',
  'x-network-consensus':'→ 网络 / 共识',
  'x-node-decentralization-roadmap':'节点去中心化路线图',
  'x-open-source-chain-logic':'开源的链上逻辑',
  'x-phase-0-now':'第 0 阶段（现在）：',
  'x-phase-1-100-humans':'第 1 阶段（100 人以上）：',
  'x-phase-2-1-000-humans':'第 2 阶段（1,000 人以上）：',
  'x-phase-3-10-000-humans':'第 3 阶段（10,000 人以上）：',
  'x-protocol-mechanisms':'协议机制',
  'x-what-happens-to-aeq-when':'当人去世或永久失能时，AEQ 会怎样？在比特币和多数加密货币中，丢失的钱包意味着供给永久消失 —— 据估计有数百万枚 BTC 永远无法取回。Aequitas 通过多级不活跃恢复机制解决这一问题：若某个钱包长期没有任何动静，其余额会经由基本收入池逐步回到社群，使真正流通的供给保持意义。',
  'x-what-if-someone-is-hospitalized':'如果有人住院、入狱，或数月无法使用自己的设备呢？受托人机制允许另一位已验证的真人确认钱包持有者仍然在世，从而避免其 AEQ 被转入托管。这位受托人完全没有任何资金权限：他只能调用一个函数，用来重置不活跃计时。在任何情况下都无法转移、花费或查看资金。',
  'bv-bind':'🔗 生成绑定签名',
  'bv-check-d':'第二个调用会列出每一位验证者并加以比较：是否都持有相同数量的登记、是否有谁缺少种子、密钥是否一致。如果你的条目显示出偏差，在这里发现总好过在别人注册到一半时发现。',
  'bv-check-t':'确认它确实在工作',
  'bv-desc':'出块节点保护的是<strong style="color:var(--text)">账本</strong>。生物验证者保护的是另一件事：<strong style="color:var(--neon)">每个人只注册一次</strong>这个承诺。二者是不同的角色 —— 你可以只做其一，也可以在同一台机器上兼任。',
  'bv-guide-sub':'逐步说明 &middot; 无需密码学知识 &middot; 约 30 分钟，大部分时间在下载',
  'bv-honest-d':'这一部分仍在测试阶段，限制是真实存在的。联合比对会消耗一次性的密码学材料，目前一次供给大约只够几十次注册，之后就需要补充 —— 也就是说，这条保密路径先在小规模上自证，而不是在数百万人上。工作量还会随登记人数增长。我们如实公布这些数字而不去取整：一个要求你交出面容的系统，没有资格在"能做什么、还不能做什么"上含糊其辞。',
  'bv-honest-t':'今天的实际情况 —— 直说',
  'bv-need-1':'<strong style="color:var(--text)">一个已注册的 Aequitas 账户。</strong>与出块的规则相同，理由也相同：一人一钥。若无此限制，一个人便可悄悄成为整个委员会。',
  'bv-need-2':'<strong style="color:var(--text)">一台装有 Docker 的小型 Linux 服务器。</strong>2 GB 内存即可。无需显卡 —— 比对只是 64 字节上的算术。你已在运行节点的那台机器就够用。',
  'bv-need-3':'<strong style="color:var(--text)">一个带 HTTPS 的域名。</strong>其他委员会成员必须能连上你。用你已有域名的一个子域即可。',
  'bv-need-4':'<strong style="color:var(--text)">保持在线。</strong>一次注册要完成，委员会的每位成员都必须应答。经常离线的验证者是在拖慢别人，而不是保护别人。',
  'bv-need-t':'开始之前 —— 你需要什么',
  'bv-s1-note':'私钥那一半只留在你的服务器上，别处都不要。公钥那一半本就是用来分享的 —— 别人凭它验证你确实作了证。<strong style="color:var(--text)">你自己的投影种子很重要：</strong>因为每个验证者用的都不同，从一个验证者那里窃得的数据库无法与另一个的相互比对。种子一旦丢失，你保存的份额便失去意义，所以请备份到你自己掌控的地方。',
  'bv-s1-t':'第 1 步 —— 生成你自己的密钥',
  'bv-s1-warn-d':'两个持有同一份秘密的验证者只算一个，委员会会比看上去更小。任何人 —— 包括我们 —— 都不应该给你发送密钥。',
  'bv-s1-warn-t':'自己生成。绝不要接受任何人给的密钥。',
  'bv-s2-d':'把第 1 步的值放进一个只有你能读的文件。每行一个值，不加引号。',
  'bv-s2-note':'<strong style="color:var(--gold)">在读完数据保护说明之前，请让 ALLOW_REAL_BIOMETRIC_DATA 保持 false</strong>。关闭时，你的验证者照样加入网络并参与测试注册，却从不保存任何真实个人的数据。这是正确的起步方式，也不必急着改。',
  'bv-s2-t':'第 2 步 —— 写好配置文件',
  'bv-s3-note':'健康的回应会报告 <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> 和 <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>。前者是"不保存任何完整模板"这一说法的凭据，而且是你能亲自核对的形式，不必靠相信。现在查一次，过些时候再查一次 —— 这既是别人的保障，也是你自己的。',
  'bv-s3-t':'第 3 步 —— 启动验证者',
  'bv-s4-d':'其他委员会成员通过公共网络连上你，因此端口不能未加密暴露。Caddy 会自行申请证书。',
  'bv-s4-t':'第 4 步 —— 在前面加上 HTTPS',
  'bv-s5-d':'区块生产者把签名密钥绑定到已注册的人类钱包：钱包签署 <strong style="color:var(--text)">Aequitas: authorize validator &lt;地址&gt;</strong>，没有它链会拒绝该席位。下面的按钮生成的正是这个签名——用于验证者角色。<strong style="color:var(--text)">验证员密钥还没有这样的绑定。</strong>它的公开一半在链外收集（步骤 6），加入每台证明服务器核对的清单。链上没有任何东西把它与一个人联系起来。在此之前，委员会数的是机器而不是人，一名运营者可能持有多个。我们宁愿在这里说清楚，也不愿让数字看起来比实际更强。',
  'bv-s5-t':'步骤 5 — 什么把密钥绑定到一个人（以及什么还没有）',
  'bv-s6-d':'把第 1 步中<strong style="color:var(--text)">公开</strong>的那一半连同你的 HTTPS 地址发到群里。它会被加入每台证明服务器核对的名单，从此你的作证便计入法定人数。这一步没有任何秘密离开你的机器 —— 这正是拆分的意义：私钥永远留在你手里，公钥离了它便毫无用处。',
  'bv-s6-t':'第 6 步 —— 公布你的公钥',
  'bv-status-d':'验证者的源码<strong style="color:var(--text)">尚未公开</strong>，因此今天并非人人都能走完下面的步骤。我们仍然先行发布，因为一个设计应当在上线之前就可被检验，而不是之后。若你想运行一个，请到首页链接的 Telegram 群里询问。开放这个代码库，才能把下面的指南从一份计划变成一份邀请 —— 这是我们接下来欠你们的。',
  'bv-status-t':'状态：封闭测试 —— 开始前请先读这一段',
  'bv-title':'或成为生物验证者 —— 让唯一性去中心化的角色',
  'bv-what-d':'人脸永远不会发送给你。你的机器保存的是 64 字节摘要的一份<strong style="color:var(--text)">加法份额</strong>：单独来看它与随机噪声无从分辨，你能运行的任何计算都无法从中还原出一张脸。比对由你所在委员会的成员共同完成，你们中没有任何人知道答案以外的信息 —— <em>是否重复：是或否</em>。这不是关于我们善意的承诺，而是算术本身的性质。',
  'bv-what-t':'你会持有什么 —— 以及你永远看不到什么',
  'bv-why-d':'一次注册只有在<strong style="color:var(--text)">多位不同验证者</strong>共同作证后才被接受。因此一把被盗的密钥并不够 —— 攻击者需要整个委员会。而由于<strong style="color:var(--neon)">一个人只能持有一把验证者密钥</strong>，买下一个委员会就意味着要成为那么多个人。在 100 位验证者中，控制其中 10 位的人拥有一个完整三人委员会的概率低于千分之一。每多一个人加入，这个数字就更小。这是参与者数量<em>本身即是</em>安全性的唯一之处。 <strong style="color:var(--text)">这个算法假设每个验证员密钥对应一个人。</strong>对区块生产，链会强制这一点；对验证员密钥还不会（步骤 5）。在此之前，上面的数字是安全性的上限，而不是对它的度量。',
  'bv-why-t':'为什么每多一位验证者，网络就更难被腐蚀',
  'x-0-1-split-40-30':'0.1 % · 按 40/30/20/10 分配',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 人。财富上限从 5 倍滑动至 &#8594; 25 倍。奠基阶段。',
  'x-0-8211-2-years':'0 &#8211; 2 年',
  'x-0-perfect-equality':'0 = 完全平等',
  'x-1-000-aeq-minted':'+1,000 AEQ 已铸造',
  'x-1-000-aeq-per-human':'每人 1,000 AEQ',
  'x-1-000-aeq-will-be':'1,000 AEQ 将自动入账',
  'x-10-000-8211-1m-humans':'1 万 &#8211; 100 万人。至少 10 个节点。完全去中心化。',
  'x-100-8211-10-000-humans':'100 &#8211; 1 万人。固定上限 25 倍。节点自由加入。',
  'x-100-maximum-concentration':'100 = 集中度最高',
  'x-1m-humans-global-ubi-at':'超过 100 万人。大规模的全球基本收入。基尼目标 &lt;0.30。',
  'x-9679-liquidity-lp-30':'&#9679; 流动性 LP 30 %',
  'x-9679-treasury-10':'&#9679; 储备金 10 %',
  'x-9679-ubi-pool-20':'&#9679; 基本收入池 20 %',
  'x-9679-validators-40':'&#9679; 验证者 40 %',
  'x-active-validators':'活跃验证者',
  'x-add-aequitas-chain-to-metamask':'把 Aequitas 链添加到 MetaMask，即可查看 AEQ 余额、发送交易，并直接从浏览器或手机钱包与 V7 合约交互。',
  'x-admin-keys-or-governance-votes':'管理员密钥或治理投票',
  'x-aeq-activity':'AEQ 活动',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'Aequitas BlockDAG —— 没有一分力气白费',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Aequitas 链（BlockDAG）',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas 以数学方式落实这一点。每位通过验证的人都恰好获得 1,000 AEQ —— 无论是亿万富翁还是自给自足的农民，没有例外。四种再分配机制确保不平等无法无限累积。基尼系数实时记录在链上。',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas —— 人性证明链',
  'x-android-apk-direct-download':'Android APK · 直接下载',
  'x-architecture':'架构',
  'x-automatic-on-chain':'链上自动执行',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG（有向无环图）',
  'x-blockdag-parallel-production':'BlockDAG · 并行出块',
  'x-blockdag-proof-of-humanity':'BlockDAG + 人性证明',
  'x-blue-score':'「蓝色分数」',
  'x-both-blocks-are-kept-ghostdag':'两个区块都保留 —— GHOSTDAG 会把同时产生的那个并入，并仍计入规范顺序。',
  'x-canonical-winner':'规范胜出者',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'与美国（0.41）或法国（0.32）相当，处于多数发达经济体的区间内。再分配正在切实拉平曲线。',
  'x-confirm-ward-is-alive':'✓ 确认此人仍在世',
  'x-core-technology':'核心技术',
  'x-daily-ubi-returns-to-all':'每日基本收入回到所有已验证的人手中',
  'x-demurrage-0-5-mo':'滞留费（每月 0.5 %）',
  'x-device-bound-zk-proof-one':'绑定设备的 ZK 证明 · 每台设备一次注册',
  'x-diagonal-line-perfect-equality':'对角线 = 完全平等',
  'x-disconnect-wallet':'⊘ 断开钱包',
  'x-distinct-proposers-recent-blocks':'不同的出块者，最近的区块',
  'x-distribution':'📈 分配',
  'x-elliptic-curve':'椭圆曲线',
  'x-entire-distribution':'整体分布',
  'x-evm-compatible':'兼容 EVM',
  'x-fill-ghostdag-verdict-thin-ring':'填充 = GHOSTDAG 裁定 · 细环 = 出块者 · 每个高度一列。将鼠标移到区块上可看详情。',
  'x-generate-node-binding-signature':'🔗 生成绑定签名',
  'x-run-a-coordinator':'🚪 运行协调器',
  'co-title':'或者运行一个协调器 —— 每个人都要经过的门',
  'co-desc':'协调器是人到达的地方：它发出挑战，把采集分发给各验证者，统计他们的投票，并签发链据以铸币的证明。很长一段时间只有一个协调器存在——这意味着网络中的每一次注册都经过同一台机器。不是因为缺少什么，而是因为没有人运行第二个。',
  'co-status-t':'状态：封闭测试 —— 与验证者相同的说明',
  'co-status-d':'协调器与验证者位于同一个代码库，而该代码库<strong style="color:var(--text)">尚未公开</strong>。因此今天并非所有人都能完成下面的步骤。它们仍然发布出来，理由相同：设计应当在部署之前可被检验，而不是之后。',
  'co-power-t':'协调器能做什么 —— 不能做什么',
  'co-power-d':'它<strong style="color:var(--text)">无法凭空造出一个人</strong>。在多个不同的验证者作证之前，bio_hash 根本不存在，而协调器不持有他们任何一把密钥。它能做的是把一个<strong style="color:var(--text)">已存在的</strong> bio_hash 绑定到某个钱包——因此不诚实的协调器可以把分配转移到自己选定的地址。这是一项真实的权力，随着每增加一个协调器而增长，任何权衡是否信任的人都应当明白这个区别。',
  'co-safe-t':'为什么第二个协调器是安全的',
  'co-safe-d':'以前并非如此。到 2026 年 8 月为止，<strong style="color:var(--text)">一人一次注册</strong>的承诺依赖协调器内部的一把 Redis 锁——而两个独立的协调器不共享 Redis，同一个人的两次同时注册都会通过。现在<strong style="color:var(--text)">每个验证者都自行检查</strong>，在自己写入之前，这张脸是否已经登记。该保证不再依赖任何共享服务或共享密钥，因此协调器可以加入或退出而不改变它。',
  'co-need-t':'你需要什么',
  'co-need-d':'一个已注册的 Aequitas 账户——与出块和验证同样的规则：一人，一密钥。一台装有 Docker 并具备公共 HTTPS 地址的服务器，因为浏览器不会把摄像头交给不安全的页面。以及两把你自己生成、永不离开你机器的密钥：一把为你的证明签名，一把把钱包地址映射为标记。',
  'co-keys-t':'绝不要接受任何人给的密钥 —— 包括我们',
  'co-keys-d':'两个协调器共用一把签名密钥就不是两个协调器，而是一个拥有两个地址的协调器，本应保护人们的法定人数看起来达成了，实际并没有。在你自己的机器上、用你自己的随机源生成这两把密钥，并且一把也不要让它离开。',
  'co-auth-t':'授权你的密钥 —— 无需任何许可',
  'co-auth-d':'在你的密钥获得授权之前，验证者会拒绝它签署的一切。授权需要两项证明，不需要任何人的批准：你的钱包签名证明这把密钥背后站着一个已注册的人，你的协调器在自己的主机上证明这把密钥确实属于它。第一项用上面的按钮生成；第二项由协调器自行生成。到 2026 年 8 月为止你还需要我们提供的一个共享密钥——而那个密钥<em>就是</em>许可。它已经取消了。',
  'co-pernode-t':'该登记表按节点分别保存，这是刻意的',
  'co-pernode-d':'写入某一个节点的授权不会传到其他节点——既没有为此设计的交易，也没有广播。一份被复制的信任清单，恰恰就是这个系统刻意不设的中央权威：每位运营者自行决定其节点接受谁的证明。代价是，你的授权必须发送到每一个应当认可它的节点。签名本身是可转移的：签一次，发往各处；被跳过的节点只会继续拒绝你。',
  'co-law-t':'你会知道关于他人的什么 —— 以及由此产生什么',
  'co-law-d':'采集经你转手；你把它传下去，什么也不留。但对于经由你注册的人，只有你掌握钱包地址与标记之间的对应关系——这正是你的标记密钥必须只属于你的原因：一旦共享，任何运营者都能为任意公开地址算出标记，并查出那是谁的脸。这也意味着，依据 GDPR，你成为这些人的<strong style="color:var(--text)">数据控制者</strong>。不是我们。查阅、删除和反对的请求会到你这里，这不是形式。',
  'co-limit-t':'由此产生的唯一限制',
  'co-limit-d':'按钱包地址删除只在登记发生的那个协调器上有效：你的标记取决于你的密钥，另一个协调器会为同一地址推导出不同的标记。因此来自别处的「未找到」意味着「未在此处注册」，而不是「未注册」——回复中也这样说明。通过本人 bio_hash 的途径，那条属于本人、不需要任何运营者的途径，在每一个协调器都有效，因为这个标识始终不变。',
  'x-authorize-coordinator-key':'🔑 授权协调器密钥',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG（2018）—— 从纠缠的图中得出唯一确定的顺序',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'基尼系数',
  'x-gini-coefficient-0-1':'基尼系数（0–1）',
  'x-gini-index-history':'基尼指数历史',
  'x-gini-target-scandinavian-level':'基尼目标（北欧水平）',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'Groth16 ZKP（零知识）',
  'x-guardian-system-8212-human-failsafe':'受托人 —— 为丢失钱包设立的人工保障',
  'x-hash-wallet':'哈希 / 钱包',
  'x-healthier-than-most-nations-on':'比世界上多数国家更健康。与北欧（0.27）和德国（0.31）相当。财富上限与滞留费成功维持了公平分配。',
  'x-higher-than-most-european-nations':'高于多数欧洲国家 —— 与巴西（0.53）或俄罗斯相当。协议的再分配正以较高强度运行。',
  'x-honest-limitation':'坦承的局限：',
  'x-how-it-works':'运作方式',
  'x-how-to-read-this-chart':'如何看懂这张图：',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'人可以注册',
  'x-imagine-a-world-where-every':'「设想这样一个世界：地球上的每个人 —— 无论出生在哪里、说什么语言、父母有多少钱 —— 都仅仅因为身为人类而获得一份有保障的日收入。不是施舍，而是一项数学权利，由任何政府或企业都无法推翻的代码来执行。」',
  'x-inactive-escrow':'不活跃托管',
  'x-inactivity-timeline':'不活跃时间线',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256（抗量子）',
  'x-key-protections':'主要保护措施：',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG（2026）—— Aequitas 自行发展、超越固定 K 值 GHOSTDAG 的版本',
  'x-knightdag-secured':'· 由 KnightDAG 保障',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'相当于北欧（约 0.27）',
  'x-liquidity-pool-30':'流动性池（30 %）',
  'x-loading-blocks':'正在加载区块…',
  'x-loading-topology':'正在加载网络结构…',
  'x-loading-transactions':'正在加载交易…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'洛伦兹曲线 —— AEQ 在人群中的分布',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask 手机版：若注册后 AEQ 余额显示为 0，请前往 设置 → 网络 → 删除 Aequitas 链 → 通过本网站重新添加',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask 手机版：若添加后 AEQ 显示为 0，请删除该网络并用上方按钮重新添加。',
  'x-money-exists-because-people-exist':'钱之所以存在，是因为人存在。因此，每个人都应仅仅因为身为人类而拥有平等的一份。',
  'x-money-exists-because-people-exist-2':'「钱之所以存在，是因为人存在。不多，也不少。」',
  'x-most-unequal-currency-ever':'有史以来最不平等的货币',
  'x-multi-validator-network':'多验证者网络',
  'x-n-lt-10-not-yet':'⚠ N&lt;10：尚不具统计意义',
  'x-no-snapshots-yet-first-one':'尚无记录 —— 第一条将在下次发放后保存。',
  'x-no-stake-blockchain':'无质押区块链',
  'x-node-operator-guide-pdf':'📄 节点运营者指南（PDF）',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET 必须是已注册的 Aequitas 真人',
  'x-one-human-one-wallet-1':'一个人 = 一个钱包 = 1,000 AEQ',
  'x-p2p-protocol':'P2P 协议',
  'x-paid-out-daily':'每日发放',
  'x-permanent-on-chain':'永久 · 链上',
  'x-phase-roadmap-8212-the-path':'阶段路线图 —— 通往全球规模之路',
  'x-phase-transitions-are-automatic-8212':'阶段切换是自动的 —— 由人数阈值触发，由合约执行。没有治理投票，也没有管理员密钥。',
  'x-planned-post-beta':'计划中（测试版之后）',
  'x-postgresql-persistent':'PostgreSQL（持久化）',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'提供 AEQ / tUSD 流动性，即可获得全部兑换手续费的 30 %，每日发放。',
  'x-recorded-after-each-ubi-distribution':'每次基本收入发放后记录一次。显示网络成长时平等程度如何变化。数值越低越好 —— 目标是基尼低于 0.30。',
  'x-redistribution':'再分配',
  'x-run-a-node':'⚙️ 运行节点',
  'x-run-a-verifier':'⚙️ 运行验证者',
  'x-set-guardian':'🛡 设定受托人',
  'x-swap-fees-0-1':'兑换手续费（0.1 %）',
  'x-sybil-resistance-8212-current-state':'女巫攻击防护 —— 如实说明现状',
  'x-the-4-redistribution-mechanisms':'四种再分配机制',
  'x-the-core-innovation':'核心构想',
  'x-the-matching-threshold-has-not':'比对阈值尚未用真实采集样本校准',
  'x-the-vision-8212-a-global':'愿景 —— 一个全球基本收入协议',
  'x-the-year-is-2009-satoshi':'那是 2009 年。中本聪发布了比特币。价值第一次能够在任意两人之间流转，而无需银行。一场真正的革命。但几乎立刻，有些事情出了偏差。',
  'x-this-is-not-a-0815':'这不是一条一次只出一个块的普通区块链。Aequitas 运行的是真正的 BlockDAG，由 GHOSTDAG 排序 —— 并自 2026 年起由 KnightDAG 保障，那是 Aequitas 自行发展的自适应版本。每一笔余额、每一次发放、每一次财富上限执行，最终都依赖这套机制来得到唯一且公认的历史。',
  'x-today-beta':'今天（测试版）',
  'x-today-this-verifies-one-device':'目前这验证的是一台设备，还不是一个独一无二的人',
  'x-traditional-blockchain-wasted-work':'传统区块链 —— 白费的工作',
  'x-treasury-10':'储备金（10 %）',
  'x-trusted-verified-human':'可信的已验证真人',
  'x-two-validators-produce-at-once':'两个验证者同时出块 → 一个胜出，一个被丢弃 —— 白费的工作，而且限制了网络能够安全达到的速度。',
  'x-ubi-pool-20':'基本收入池（20 %）',
  'x-validators-pool-40':'验证者池（40 %）',
  'x-view-source-on-github':'🐙 在 GitHub 查看源码',
  'x-wealth-cap-multiplier-bootstrap-slider':'财富上限倍数 —— 起步阶段滑杆',
  'x-wealth-cap-overflow':'超出财富上限的部分',
  'x-wealth-distribution-analysis':'财富分布分析',
  'x-what-happens-when-someone-is':'如果有人住院、入狱或去世，会怎样？在多数加密系统中，钱包一旦丢失便永远丢失。Aequitas 设有三层不活跃恢复机制。',
  'x-what-is-a-guardian':'什么是受托人？',
  'x-what-is-and-is-not':'哪些是私密的，哪些不是：',
  'x-what-would-a-cryptocurrency-look':'「如果一种加密货币从最初就以对每个人公平为原则来设计，它会是什么样子？」',
  'x-why-a-normal-blockchain-isn':'为什么普通区块链不够用',
  'x-worse-than-any-country-on':'比地球上任何国家都糟（南非纪录：0.63），正在逼近比特币（0.85）。协议已处于最大干预 —— 财富上限与再分配全力运转。',
  'x-year-2-180d':'第 2 年 +180 天',
  'x-zk-device-key-proof':'设备密钥的 ZK 证明',
  'swap-price-flat':'此时间段内没有成交 —— 价格没有变动。图表是正常的，只是市场很安静。',
  'mpc-optin-title':'可选 — 协助检查重复注册（已准备，尚未启用）',
  'mpc-optin-desc':'已准备，但尚未启用。今后你的节点可以协助核验没有人重复注册，且从不接触任何生物特征数据：每个参与方只保存每份模板的一个数学份额（单独看只是噪声），共同比对新采集，因此没有任何一台机器能还原出内容。目前这条路径不做任何决定：重复检查并不经过它，委员会也是固定名单而非自动抽取，所以设置下面三个变量暂时不会改变注册流程。',
  'mpc-optin-note':'份额文件包含仅你的节点可持有的一次性随机数——切勿复制到其他机器，也不要提交到任何仓库。目前它必须由运营方提供，这是尚存的中心化依赖。你不需要新密钥：节点用它签名区块时已在使用的同一把密钥向其他成员表明身份。',
  'logo-sub':'人类证明','live':'实时',
  'reg-title':'🔐 注册成为经过验证的人类',
  'reg-sub':'加入Aequitas网络并获得1,000 AEQ的普遍基本收入补贴。一次性、永久性且完全免费。永远不会存储任何个人数据。',
  'app-title':'仅通过安卓应用注册',
  'app-text':'注册时，摄像头会拍摄你的面部和一小段活体检测序列。独立的比对服务核验镜头前是活人，且该面部尚未注册；它们必须达成法定多数一致。随后 Groth16 零知识证明将结果带上链，而不泄露你的任何信息。验证通过后，你的 <strong style="color:var(--gold)">1,000 AEQ 将自动入账</strong>。<strong style="color:var(--gold)">注意：</strong>比对阈值尚未用真实采集数据校准 — 详见下方常见问题。',
  's1t':'面部采集','s1d':'应用会录制你的面部和一小段活体检测序列，并发送给独立的比对服务。它们核验镜头前是活人，并将该面部与所有已注册者比对。图像在处理后即被丢弃。',
  's2t':'ZK证明生成','s2d':'Groth16 零知识证明将你的 bio_hash 提交为 commitment = keccak256(bioHash‖wallet)，而不泄露它。nullifier 由该哈希派生，因此同一张面部无法被计入两次 — 详见下方常见问题。',
  's3t':'连接钱包','s3d':'应用在此页面打开MetaMask · 连接您的以太坊钱包 · 证明与您的地址密码绑定',
  's4t':'获得1,000 AEQ','s4d':'在6秒内在Aequitas BlockDAG上确认注册 · 立即记入1,000 AEQ · 身份永久记录为经过验证的人类',
  'priv-bar':'🔒 面部核验由法定多数完成 · Groth16 ZKP · 图像核验后即丢弃 · 每人一次注册',
  'conn-wallet':'已连接钱包','proof-recv':'⚡ 已收到ZK证明','proof-hint':'连接钱包以注册',
  'btn-conn':'🦊 连接 METAMASK','btn-reg':'🔐 链上注册',
  'btn-wc':'🔗 连接 WALLETCONNECT',
  'reg-log-hint':'// 打开Aequitas安卓应用生成您的证明，然后返回此处...',
  'reg-details':'注册详情','k-network':'网络','k-chainid':'链ID','k-grant':'UBI补贴',
  'k-fee':'Gas费','free':'免费——完全无Gas','k-limit':'注册','k-limit-v':'每人一次 · 永久 · 不可更改',
  'k-bio':'面部','never-stored':'图像在核验后即被丢弃 — 没有任何验证节点持有完整模板',
  'k-proof':'证明系统','k-conf':'确认','k-conf-v':'1秒内（1个区块）',
  'k-sybil':'女巫攻击防护','k-sybil-v':'每人一个身份 · 与面部绑定，阈值尚未校准',
  's-height':'区块高度',
  's-humans':'已验证人类',
  's-supply':'总供应量','s-supply-sub':'始终 = 人类 × 1,000 AEQ',
  's-uptime':'运行时间',
  'k-chain':'链名称','k-symbol':'符号','k-btime':'区块时间',
  'k-cons':'共识','k-storage':'存储','k-dec':'小数位',
  'btn-add-mm':'+ 添加AEQUITAS网络',
  'humans-title':'Aequitas链上的已验证人类',
  'h-what':'什么是已验证人类？','h-what-t':'已验证人类是指经证明属于某位其面部尚未注册者的钱包地址。独立的比对服务必须达成法定多数一致，且只有 Groth16 零知识证明会上链 — 不含任何图像或模板。<strong style="color:var(--gold)">在 2026-08-23 之前，这验证的是一台设备而非一个人；现已不同。</strong>',
  'h-zkp':'零知识证明系统','h-zkp-t':'Aequitas在BN128上使用Groth16——与Ethereum和Zcash相同的曲线。约200字节，约10毫秒。commitment = keccak256(deviceKey‖wallet)。Nullifier绑定到此设备：丢失手机不会在该设备上创建第二身份，但另一台设备仍可单独注册。密钥材料从不在服务器端泄露或存储。',
  'h-sybil':'女巫攻击抵御——当前状态','h-sybil-t':'nullifier 由你面部的 bio_hash 派生，因此同一张面部无法注册两次 — 跨设备同样如此，这是设备密钥从来做不到的。它所依赖的是一个尚未用真实采集数据校准的比对阈值：密码学是精确的，其下的生物特征识别是一项误差率尚未量化的测量。',
  'h-global':'全球金融包容','h-global-t':'无需银行账户、无需信用卡、无需任何加密货币经验。只要一部带摄像头的安卓智能手机。Aequitas 的设计目标是让地球上的每个人都能使用。',
  'h-bio-hw':'身份验证路线图','h-bio-hw-t':'当前（测试版）：由独立比对服务完成的面部核验，须达成法定多数一致。其阈值尚未用真实采集数据校准 — 在给出任何数字之前需要约 1000 组冒名配对。计划中：完成该校准，以及一种没有任何服务持有完整模板的重复检测。',
  'reg-humans':'已注册人类','h-desc':'下面每个地址都属于这样一个人：其面部由独立服务与所有既有注册进行了核验，以零知识证明加以证实，并获得了恰好 1,000 AEQ。该登记册是永久、不可更改且上链的。比对阈值今天能保证什么、不能保证什么，见常见问题。',
  'no-humans':'尚未注册人类。\n\n下载Aequitas安卓应用，成为链上第一个人类！',
  'reg-stats':'注册统计','total-humans':'总人数',
  'idx-title':'Aequitas指数——实时经济平等评分',
  'idx-desc':'Aequitas指数实时衡量所有经过验证的人类的经济不平等。从链上余额分布的基尼系数导出。0 = 完全平等。100 = 最大不平等。',
  'curr-idx':'当前指数','bar-0':'0 — 完全平等','bar-100':'100 — 最大不平等','wcap-lbl':'当前财富上限：','wcap-mult':'倍数：','wcap-avg':'公平份额：',
  'gini':'基尼系数','gini-desc':'0 = 平等 · 1 = 不平等',
  'supply-desc':'始终 = 人类 × 1,000 AEQ',
  'phase':'协议阶段','phase-desc':'按人类数量自动推进',
  'humans-desc':'经面部核验的注册',
  'pools-title':'再分配池',
  'pools-desc':'每笔兑换费用、滞期费和财富上限溢出自动在四个池之间分配。无需人工干预。所有池每日分配。',
  'vel-pool':'验证者池','vel-pool-desc':'所有费用的40% → 保障网络安全的节点运营商',
  'liq-pool':'流动性池','liq-pool-desc':'所有费用的30% → 流动性提供者，按LP份额比例',
  'ubi-pool':'UBI池','ubi-pool-desc':'所有费用的20% → 所有经过验证的人类均等，每24小时',
  'treasury':'国库','treasury-desc':'所有费用的10% → 协议开发和维护',
  'phases-title':'协议阶段',
  'phases-desc':'阶段转换由人类数量自动触发——无需投票、治理或管理员密钥。',
  'p0':'启动 · &lt;100人类 · 财富上限：50×平均余额 · 当前活跃',
  'p1':'增长 · 100–10,000人类 · 财富上限：20×公平份额（每人 1,000 AEQ）',
  'p2':'稳定 · 10,000–100万人类 · 财富上限：10×平均余额',
  'p3':'成熟 · 100万+人类 · 财富上限：3×平均余额 · 最大再分配',
  'wealth-cap-explain':'财富上限设定为所有经过验证的人类当前平均余额的倍数——而非固定数字。随着网络增长自动调整。',
  'demurrage-title':'滞期费——流通激励',
  'demurrage-desc':'Aequitas实施受历史互补货币启发的滞期费机制。闲置AEQ余额缓慢贬值以阻止囤积。',
  'dem-rate-k':'衰减率','dem-rate-v':'每月0.5%（连续，非阶梯式）',
  'dem-grace-k':'宽限期','dem-grace-v':'衰减开始前3个月不活动',
  'dem-reset-k':'计时器重置','dem-reset-v':'任何转账、兑换或流动性操作重置计时器',
  'dem-dest-k':'衰减的AEQ去往','dem-dest-v':'再分配池（40/30/20/10分配）',
  'dem-warn-k':'警告系统','dem-warn-v':'14天通知（一次）+ 每次登录7天重复提醒',
  'story-title':'Aequitas的故事——为何而生',
  'nodes-title':'活跃节点 — 当前网络拓扑','nodes-desc':'Aequitas网络目前在多个地理分布的节点上运行（当前数量见上方）。所有节点均参与区块生产、状态同步和API服务。通过libp2p点对点通信，通过HTTP同步区块状态。网络设计支持更多节点——任何运营商均可加入。',
  'run-node-title':'运行您自己的节点 — 帮助保护网络',
  'run-node-desc':'每个已注册的人都可以运行 Aequitas 节点——无需质押、无需申请、无需我们批准。一人一个验证者密钥：NODE_OPERATOR_WALLET 不是已注册人类的节点会被以 HTTP 403 拒绝，否则一个人可能悄悄成为整个验证者集合。。节点参与区块生产并验证人类注册表。节点运营商通过验证者池（每日分配的所有互换费用的40%）赚取协议费用份额。',
  'run-node-title':'运行您自己的节点 — 帮助保护网络',
  'run-node-desc':'每个已注册的人都可以运行 Aequitas 节点——无需质押、无需申请、无需我们批准。一人一个验证者密钥：NODE_OPERATOR_WALLET 不是已注册人类的节点会被以 HTTP 403 拒绝，否则一个人可能悄悄成为整个验证者集合。。节点参与区块生产并验证人类注册表。节点运营商通过验证者池（每日分配的所有互换费用的40%）赚取协议费用份额。',
  'bootstrap-title':'运行自己的节点','bootstrap-desc':'任何人都可以通过运行节点加入Aequitas网络。下载节点指南获取分步说明。',
  'tech-title':'技术规格','mm-config':'MetaMask配置',
  'k-lang':'语言','k-src':'源代码','evm-yes':'是 — JSON-RPC /rpc · MetaMask兼容',
  'proto-label':'Aequitas V7协议——技术文档',
  'ca-title':'合约地址','ca-text':'链：Aequitas Chain（链ID：1926 · 0x786）<br>RPC：__RPC__<br><br>BioVerifier：0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7：0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 定义了 Aequitas 经济的规则，并持有使这些规则可执行的登记簿：每一个被使用过的 nullifier、每一次注册、财富上限与需求损耗公式。合约不可变更——没有管理员密钥、没有升级代理、没有治理投票能改动其中一行。但真正结算一笔转账的是链层：节点在 ERC-20 调用抵达 EVM 之前将其拦截，并记入自己的账本，这正是转账能做到亚秒级且无 gas 的原因。合约是规则书与登记簿；链是执行它们的引擎，其源代码公开。<br><br>BioVerifier合约接收完全在用户Android设备上生成的Groth16零知识证明。它在约10毫秒内在链上数学验证所提交的 nullifier 是由注册者持有的秘密正确推导而来，并且链会拒绝任何已经见过的 nullifier——而不会泄露他们的姓名、身份或生物特征数据。这排除了同一身份来源的第二次注册；该来源是人还是设备，取决于生物识别模式是否启用。这使得无gas、无需投资的注册成为可能：证明是唯一离开设备的东西。<br><br>真正新的正是这个组合：规则与「一人一次注册」的登记簿位于一份任何人都无法改写的合约中——运营者、公司、政府都不能——而执行它们的代码是开源的，可从本仓库复现。这一切任何人都能核验。仍然需要信任的是节点本身的运行，而减少这种信任的诚实方式是更多独立验证者，而不是在此处写一句更强硬的话。',
  'poa-title':'1. 生存证明 — 非活跃余额恢复','poa-text':'<p>当人们死亡或永久失去行为能力时AEQ会怎样？在比特币中，丢失的钱包意味着永久丢失的供应量。Aequitas通过多阶段非活跃恢复系统解决这个问题：如果一个钱包长时间没有活动，其余额会逐渐通过UBI池返回社区。</p>',
  'poa-box':'第0–2年：正常使用 — 无限制<br>第2年：警告1 — 监护人可以代表回应<br>第2年+60天：警告2 — 紧迫性增加<br>第2年+120天：警告3 — 最终通知<br>第2年+180天：AEQ移至个人托管（仍可恢复）<br>第4年：如果仍不活跃 — 托管释放至UBI池',
  'guard-title':'2. 监护人系统 — 人类安全保障','guard-text':'<p>如果有人住院或因其他原因数月无法访问其设备怎么办？监护人系统允许可信任的人——另一个经过验证的人类——确认钱包所有者仍然活着。监护人拥有严格为零的财务访问权限：只能调用重置非活跃计时器的单一函数。在任何情况下都不能移动、花费或访问资金。</p>',
  'guard-box':'每人1个监护人 · 必须是Aequitas上的经过验证的人类<br>监护人只能调用confirmAlive() — 零交易权限<br>监护人不能移动资金、转移AEQ或访问钱包<br>每个监护人最多3名受监护人 · 分配7天时间锁 · 不允许循环关系',
  'dem-title':'3. 滞期费 — 防囤积机制',
  'dem-box':'仅对超出你公平份额的部分收取 — 等于或低于该份额的余额永不衰减<br>费率：3个月非活跃后每月0.5%（连续，非分步）<br>任何转账、互换或流动性操作会自动重置计时器<br>衰减的AEQ重新分配到四个池中 — 从不销毁<br>14天通知显示一次 · 每次活跃会话重复7天提醒',
  'dem-text':'<p>滞期费是货币的持有成本——一种使囤积变得昂贵、流通变得有吸引力的负利率。沃尔格实验（奥地利，1932年）使用滞期费货币在一年内将当地失业率降低了25%。奥地利中央银行正因为它运作得太好而关闭了它。Chiemgauer（德国，2003年）按照相同原则成功运营了20多年。</p>',
  'cap-title':'4. 财富上限 — 数学公平执行','cap-box':'启动上限：max(5,min(N,25))× 平均AEQ余额<br>1–4人：5×（5,000 AEQ）· 每增1人加1× · 25+人：25×（25,000 AEQ）永久<br>适用于除4个协议池外的所有地址<br>超额AEQ立即重新分配 · 无需手动干预',
  'ubi-title':'5. 普遍基本收入 — 每日再分配','ubi-box':'UBI池收入来源：<br>· AEQ↔tUSD AMM池所有互换费用的20%<br>· 财富上限执行的溢出<br>· 非活跃账户的滞期费<br>· 4年后释放的非活跃托管<br><br>分配：每24小时，整个UBI池余额在所有注册的经过验证的人类中平均分配。池重置为零并立即开始从持续的协议活动中重新填充。',
  'inf-title':'6. 无算法通胀 — 固定供应公式','inf-box':'创建新AEQ的唯一事件：新的经过验证的人类注册。<br><br>总供应量 = 经过验证的人类 × 1,000 AEQ<br><br>这不是政策——它由协议执行。没有管理员可以铸造额外的AEQ，没有治理投票可以改变发行，没有预挖矿的创始人分配。AEQ是唯一一种总供应量完全由经过验证的活人数量决定的加密货币。',
  'phases-desc':'阶段边界定义网络增长里程碑。启动阶段（&lt;25名注册人类）财富上限使用滑动乘数：max(5,min(N,25))×平均余额 — 1–4人时为5×，每增加1人加1×，25+人时达到完整25×。防止早期参与者在真正参与形成前集中财富。',
  'p0':'引导期 · 不足100人 · 上限：max(5,min(N,25))×平均 · 滑动5×→25×直至25人 · 当前激活',
  'p1':'增长期 · 100–10,000人 · 财富上限：25×公平份额 = 25,000 AEQ',
  'p2':'稳定期 · 10,000–1M人 · 财富上限：25×公平份额 = 25,000 AEQ',
  'p3':'成熟期 · 1M+人 · 财富上限：25×公平份额 = 25,000 AEQ',
  'wealth-cap-explain':'财富上限在启动阶段动态调整：max(5, min(N, 25)) × 平均余额，N为已注册人类数。1–4人时：5×（5,000 AEQ）。每新增1人多1×。25+人时：永久25×（25,000 AEQ）。防止早期采用者在真实参与形成前过度积累。始终相对于当前平均余额。',
  'btn-download-app':'下载 AEQUITAS 应用',
  'swap-title':'🔄 兑换 AEQ ↔ tUSD','swap-sub':'通过原生流动性池将AEQ兑换为tUSD（模拟测试美元）。0.1%手续费仅适用于兑换 — 人与人之间的普通AEQ转账完全免费。',
  'swap-priv-bar':'🔒 仅0.1%兑换费 · AEQ到AEQ转账免费 · tUSD是无实际价值的测试货币',
  'swap-your-aeq':'你的 AEQ','swap-your-tusd':'你的 tUSD',
  'swap-fee-est':'协议手续费 (0.1%)','swap-details-hdr':'兑换详情',
  'swap-out-lbl':'你获得（估算）','swap-impact-lbl':'价格影响','swap-rate-lbl':'汇率',
  'swap-depth-lbl':'池子构成','amm-title':'x × y = k — 恒定乘积 AMM',
  'amm-text':'当你用AEQ兑换tUSD时，AEQ储备增加，tUSD储备减少——它们的乘积始终等于k。更大的兑换造成更大的价格影响。0.1%手续费在应用公式前从输入中扣除。',
  'swap-btn-go':'🔄 兑换',
  'swap-log-hint':'// 连接钱包以兑换...',
  'swap-no-liquidity':'还没有 tUSD？','swap-faucet-desc':'已注册的人类可以申领一次测试 tUSD','swap-btn-faucet':'💧 申领测试 tUSD',
  'swap-addliq-title':'提供流动性','swap-addliq-desc':'成为第一个存款者 — 你的比例设定起始价格。','swap-btn-addliq':'💧 添加流动性',
  'swap-lp-title':'你的 LP 仓位','swap-lp-share':'池子份额','swap-lp-withdrawable':'可提取',
  'swap-lp-pct-label':'% 你的仓位','swap-lp-youget':'你将收到','swap-btn-removeliq':'🔥 移除流动性',
  'swap-pool-title':'AEQ / tUSD — 池子状态',
  'swap-pool-aeq':'AEQ 储备','swap-pool-tusd':'tUSD 储备','swap-pool-price':'现货价格',
  'swap-fee-bps':'兑换手续费',
  'swap-pools-addr-title':'代币经济池地址',
  'swap-validators':'验证者 (40%)','swap-lps':'流动性提供者 (30%)','swap-ubi':'UBI 池 (20%)','swap-treasury':'国库 (10%)',
  'ubi-hero-title':'普遍基本收入 — UBI 池',
  'ubi-hero-sub':'累积中 — 下次平等分配给所有验证人类：',
  'ubi-bal-lbl':'当前池余额','ubi-hero-desc':'在所有验证人类中平等分配 · 每24小时支付 · 支付后池归零 · 无最低余额要求',
  'ubi-how-fills':'UBI 池如何填充',
  'ubi-src-swap':'兑换手续费','ubi-src-swap-d':'每次AEQ↔tUSD兑换贡献其0.1%手续费的20%。更多交易 = 更快填充。',
  'ubi-src-dem':'滞期费','ubi-src-dem-d':'不活跃AEQ（3+个月）以0.5%/月衰减。衰减金额的20%进入UBI。',
  'ubi-src-cap':'财富上限溢出','ubi-src-cap-d':'超过max(5,min(N,25))×平均余额的钱包立即被没收超额部分。20%立即流入UBI。',
  'pools4-header':'所有四个再分配池',
  'ubi-see-above':'见上方倒计时','ubi-timer-above':'⏰ 倒计时显示在上方','pool-t-timer':'累积中 — 无计时器',
  'usp-headline':'历史上首次 — 所有人在平等条件下起步',
  'usp-sub':'只需拥有一部Android智能手机即可参与。无需银行账户，无需加密货币知识，无需任何投资。',
  'usp-c1-title':'0元启动投资','usp-c1-desc':'注册完全免gas。无需ETH、无需MATIC、无需信用卡。协议代您支付所有交易费用。',
  'usp-c2-title':'每人1,000 AEQ','usp-c2-desc':'亿万富翁还是贫困农民——每人恰好获得1,000 AEQ。不多不少。平等起点，数学保证。',
  'usp-c3-title':'人人可参与','usp-c3-desc':'无需银行账户、无需信用卡、无需身份证件、无需额外硬件 — 只要你的安卓手机自带的摄像头。',
  'usp-c4-title':'永久每日UBI','usp-c4-desc':'注册后，您每天自动获得UBI支付份额——每天，无需任何操作。',
  'v7-intro-title':'什么是 AequitasV7？',
  'v7-intro-text':'AequitasV7是Aequitas协议的核心智能合约。"V7"指公平合约的第7个主要版本。它不可更改地部署在Aequitas Chain（链ID 1926）上，处理所有方面：人类注册、ZK证明验证、余额管理、财富上限、UBI分配、兑换手续费。没有管理员可以升级它。六个机制形成自我强化系统。',
  'swap-sell-label':'卖出','swap-receive-label':'接收',
  'gini-what-title':'什么是基尼系数？','gini-what-text':'由意大利统计学家科拉多·基尼于1912年提出。通过将实际余额与假设的完全平等基线进行比较来衡量财富分配。范围：0（人人均等）到1（一人独占）。世界银行、经合组织、联合国用于比较各国。参考值：比特币≈0.85 · 南非（世界纪录）≈0.63 · 美国≈0.41 · 德国≈0.31 · 北欧≈0.27 · Aequitas长期目标：基尼系数低于0.30。','gini-calc-title':'如何计算Aequitas指数','gini-calc-text':'收集所有AEQ余额。公式计算每对余额之间的平均绝对差，结果0-1乘以100=Aequitas指数。','gini-why-title':'为什么选择基尼系数','gini-why-text':'基尼系数捕捉所有已验证人类的完整分布。Aequitas将此数据发布在链上。',
  'guard-title':'🛡 守护者系统','guard-my-lbl':'我的守护者','guard-none':'无',
  'guard-set-lbl':'设置 / 更改守护者','guard-set-hint':'必须是已注册的Aequitas人类 · 7天时间锁 · 守护者只能确认您的活跃状态，不能访问资金 · 每位守护者最多3名被保护者',
  'guard-confirm-lbl':'确认存活（作为守护者）','guard-confirm-hint':'如果您的被保护者无法访问其钱包，请确认其活跃状态，以防止其资金在910天不活跃后转入托管。','guard-recover-btn':'🔓 从托管中恢复',
  'faq-title':'❓ 常见问题','faq-q1':'我的生物特征数据安全吗？','faq-a1':'系统会拍摄你的面部并发送给独立的比对服务——只有这样才能真正核验「一人一账户」。图像经处理后即被丢弃，不会存储。保留的是一份数学模板：经过加密，并拆分成份额分布在独立运营的验证节点上，因此没有任何验证节点持有完整的模板。一个如实说明而非隐瞒的界限：执行比对的服务本身仍然持有模板，因为比对需要它们。',
  'faq-q1b':'注册能证明我是独特的真实人类吗？','faq-a1b':'比设备密钥所能做到的更好，但尚不能用数字证明。面部会由必须达成一致的独立服务与所有既有注册进行比对，因此同一个人换第二部手机也会被识破 — 这是设备密钥从来做不到的。尚未确定的是错误率：比对阈值未用真实采集数据校准，这需要约 1000 组冒名配对。',
  'faq-q2':'我以后可以用不同的钱包注册吗？','faq-a2':'不能。一次注册永久绑定到一个钱包地址。这是有意为之：由你的面部派生的 nullifier 只能使用一次，因此换一个钱包再次注册就等于同一个人拥有第二个身份。',
  'faq-q3':'如果我丢失手机会怎样？','faq-a3':'您的AEQ保留在您的钱包中——它与您的私钥绑定，而非手机。您仍然可以通过MetaMask使用助记词访问钱包。钱包恢复与生物特征注册无关。',
  'path-title':'选择您的路径','path-human-title':'我是人类','path-human-desc':'我想注册、获得1,000 AEQ并加入基本收入网络。','path-human-steps':'1. 下载Aequitas安卓应用<br>2. 用设备锁屏解锁（指纹/面部/PIN）<br>3. 连接MetaMask<br>4. 立即获得1,000 AEQ',
  'path-node-title':'我是节点运营商','path-node-desc':'我想运行完整节点，参与区块生产，并从40%验证者池中获利。','path-node-steps':'1. 注册为人类（必须）<br>2. 无需配置入口 — 验证者地址已内置<br>3. 部署在Contabo/Hetzner/任意VPS<br>4. 每日从验证者池获利',
  'path-dev-title':'我是开发者','path-dev-desc':'我想在Aequitas上构建，集成API，或为协议做贡献。','path-dev-steps':'1. EVM兼容JSON-RPC<br>2. 链ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* 端点<br>4. 指标: /metrics (Prometheus)',
  'story-flow-title':'AEQ代币流向图','story-topo-title':'网络拓扑——当前状态',
  'swap-price-title':'AEQ / tUSD — 实时价格','swap-price-desc':'从池储备（x·y=k）实时派生的价格。每8秒更新一次。','swap-price-empty':'暂无池数据——添加流动性以查看价格图表。',
  'node-guide-lang-note':'此内联指南为英文。您语言的翻译PDF可通过上方按钮获取。',
  'k-zkp':'ZKP系统','k-hash':'哈希系统','k-sybil-prot':'女巫攻击防护',
  'soc-title':'💬 社交媒体','soc-sub':'公告、链的真实状态，以及那些不好回答的问题 &mdash; 两个平台，都公开。',
  'soc-x-desc':'公告，以及链实际在做什么。短内容。','soc-tg-desc':'公开群组：提问、节点运营者，以及注册方面的帮助。',
  's-validators':'活跃验证者',
  'expl-heading':'区块浏览器',
},
id:{
  'x-consensus-ghostdag-knightdag':'◆ Konsensus: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Kode kontrak',
  'x-demurrage-is-a-holding-cost':'Uang menyusut adalah biaya menyimpan uang — bunga negatif yang membuat penimbunan mahal dan peredaran menarik. Ada preseden sejarahnya: percobaan Wörgl (Austria, 1932) memakai mata uang menyusut dan menurunkan pengangguran setempat 25 % dalam setahun. Bank Nasional Austria menghentikannya justru karena terlalu berhasil dan mengancam monopoli perbankan. Chiemgauer (Jerman, 2003) berjalan atas asas yang sama dan beredar dengan sukses lebih dari 20 tahun. Aequitas menerapkan penyusutan berkelanjutan 0,5 % per bulan, hanya setelah tiga bulan tidak aktif.',
  'x-network-consensus':'→ Jaringan / konsensus',
  'x-node-decentralization-roadmap':'Peta jalan desentralisasi node',
  'x-open-source-chain-logic':'Logika rantai sumber terbuka',
  'x-phase-0-now':'Tahap 0 (sekarang):',
  'x-phase-1-100-humans':'Tahap 1 (100+ orang):',
  'x-phase-2-1-000-humans':'Tahap 2 (1.000+ orang):',
  'x-phase-3-10-000-humans':'Tahap 3 (10.000+ orang):',
  'x-protocol-mechanisms':'Mekanisme protokol',
  'x-what-happens-to-aeq-when':'Apa yang terjadi pada AEQ ketika seseorang meninggal atau menjadi tak berdaya secara permanen? Di Bitcoin dan kebanyakan mata uang kripto, dompet yang hilang berarti pasokan yang hilang selamanya — diperkirakan jutaan BTC tak terjangkau untuk selamanya. Aequitas mengatasinya dengan pemulihan ketidakaktifan bertahap: bila sebuah dompet tak menunjukkan aktivitas dalam waktu lama, saldonya berangsur kembali ke masyarakat lewat dana pendapatan dasar, sehingga pasokan yang benar-benar beredar tetap bermakna.',
  'x-what-if-someone-is-hospitalized':'Bagaimana bila seseorang dirawat di rumah sakit, dipenjara, atau berbulan-bulan tak bisa mengakses perangkatnya? Sistem orang kepercayaan memungkinkan orang terverifikasi lain memastikan bahwa pemilik dompet masih hidup, sehingga AEQ-nya tidak berpindah ke penitipan. Orang itu sama sekali tidak punya akses keuangan: ia hanya dapat memanggil satu fungsi yang mengatur ulang penghitung ketidakaktifan. Dalam keadaan apa pun ia tak dapat memindahkan, membelanjakan, atau mengakses dana.',
  'bv-bind':'🔗 Buat tanda tangan pengikatan',
  'bv-check-d':'Panggilan kedua mendaftar setiap verifier dan membandingkannya: apakah semuanya memegang jumlah pendaftaran yang sama, apakah ada yang kehilangan benih, dan apakah kuncinya cocok. Bila entrimu menunjukkan selisih, lebih baik mengetahuinya di sini daripada di tengah pendaftaran seseorang.',
  'bv-check-t':'Memastikan semuanya berjalan',
  'bv-desc':'Node penghasil blok mengamankan <strong style="color:var(--text)">buku besar</strong>. Verifier biometrik mengamankan hal lain: janji bahwa <strong style="color:var(--neon)">setiap orang mendaftar hanya sekali</strong>. Ini peran terpisah — kamu bisa menjalankan salah satu, atau keduanya di mesin yang sama.',
  'bv-guide-sub':'Langkah demi langkah &middot; Tak perlu paham kriptografi &middot; Sekitar 30 menit, sebagian besar mengunduh',
  'bv-honest-d':'Bagian ini masih beta dan batasannya nyata. Perbandingan bersama memakai bahan kriptografis sekali pakai, dan satu pasokan saat ini menutup beberapa puluh pendaftaran sebelum perlu tambahan — jadi jalur rahasia ini membuktikan diri dulu dalam skala kecil, bukan jutaan. Bebannya juga tumbuh seiring jumlah orang yang terdaftar. Kami menerbitkan angka-angka ini alih-alih membulatkannya: sistem yang meminta wajahmu tak berhak bersikap kabur tentang apa yang bisa dan belum bisa dilakukannya.',
  'bv-honest-t':'Di mana posisinya hari ini — terus terang',
  'bv-need-1':'<strong style="color:var(--text)">Akun Aequitas yang terdaftar.</strong> Aturan yang sama seperti produksi blok, dengan alasan yang sama: satu orang, satu kunci. Tanpa itu, satu orang bisa diam-diam menjadi satu komite utuh.',
  'bv-need-2':'<strong style="color:var(--text)">Server Linux kecil dengan Docker.</strong> Memori 2 GB sudah cukup. Tanpa kartu grafis — perbandingannya aritmetika atas 64 byte. Mesin yang sudah menjalankan node-mu sudah memadai.',
  'bv-need-3':'<strong style="color:var(--text)">Nama domain dengan HTTPS.</strong> Anggota komite lain harus bisa menghubungimu. Subdomain dari sesuatu yang sudah kamu miliki sudah cukup.',
  'bv-need-4':'<strong style="color:var(--text)">Tetap daring.</strong> Setiap anggota komite harus menjawab agar sebuah pendaftaran selesai. Verifier yang sering absen memperlambat orang, bukan melindungi mereka.',
  'bv-need-t':'Sebelum mulai — apa yang kamu perlukan',
  'bv-s1-note':'Simpan bagian privat di servermu dan tidak di tempat lain. Bagian publik memang untuk dibagikan — begitulah orang lain memastikan kamu menyaksikan sesuatu. <strong style="color:var(--text)">Benih proyeksimu sendiri penting:</strong> karena tiap verifier memakai yang berbeda, basis data curian dari satu verifier tak bisa diadu dengan milik yang lain. Kehilangan benih membuat bagian yang kamu simpan kehilangan makna, jadi cadangkan di tempat yang kamu kendalikan.',
  'bv-s1-t':'Langkah 1 — Buat kuncimu sendiri',
  'bv-s1-warn-d':'Dua verifier yang memegang rahasia yang sama dihitung satu, dan komitenya jadi lebih kecil daripada tampaknya. Tak seorang pun — termasuk kami — boleh mengirimimu kunci.',
  'bv-s1-warn-t':'Buat sendiri. Jangan pernah menerima kunci dari siapa pun.',
  'bv-s2-d':'Masukkan nilai dari Langkah 1 ke berkas yang hanya bisa kamu baca. Satu nilai per baris, tanpa tanda kutip.',
  'bv-s2-note':'<strong style="color:var(--gold)">Biarkan ALLOW_REAL_BIOMETRIC_DATA bernilai false</strong> sampai kamu membaca catatan perlindungan data. Dengan itu nonaktif, verifier-mu bergabung ke jaringan dan ikut dalam pendaftaran uji tanpa pernah menyimpan data orang sungguhan. Itu cara memulai yang benar, dan tak perlu buru-buru mengubahnya.',
  'bv-s2-t':'Langkah 2 — Tulis berkas konfigurasi',
  'bv-s3-note':'Jawaban yang sehat melaporkan <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> dan <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. Yang pertama adalah klaim bahwa tak ada templat utuh yang disimpan, dalam bentuk yang bisa kamu periksa sendiri alih-alih dipercaya begitu saja. Periksa sekarang dan periksa lagi nanti — itu jaminanmu sendiri sebanyak jaminan orang lain.',
  'bv-s3-t':'Langkah 3 — Jalankan verifier',
  'bv-s4-d':'Anggota komite lain menghubungimu lewat internet publik, jadi porta tidak boleh terbuka tanpa enkripsi. Caddy mengambil sertifikat sendiri.',
  'bv-s4-t':'Langkah 4 — Pasang HTTPS di depan',
  'bv-s5-d':'Produsen blok mengikat kunci penandatanganannya ke dompet manusia terdaftar: dompet menandatangani <strong style="color:var(--text)">Aequitas: authorize validator &lt;alamat&gt;</strong>, dan tanpa itu rantai menolak slotnya. Tombol di bawah menghasilkan tanda tangan itu — untuk peran validator. <strong style="color:var(--text)">Kunci verifier belum punya ikatan seperti itu.</strong> Bagian publiknya dikumpulkan di luar rantai (Langkah 6) dan dimasukkan ke daftar yang diperiksa setiap proof server. Tidak ada di rantai yang mengikatnya ke seorang manusia. Selama itu belum ada, sebuah komite menghitung mesin, bukan manusia, dan satu operator bisa memegang beberapa. Lebih baik kami katakan di sini daripada membiarkan angkanya tampak lebih kuat.',
  'bv-s5-t':'Langkah 5 — Apa yang mengikat kunci ke seorang manusia (dan apa yang belum)',
  'bv-s6-d':'Kirimkan bagian <strong style="color:var(--text)">publik</strong> dari Langkah 1 beserta alamat HTTPS-mu ke grup. Ia ditambahkan ke daftar yang diperiksa setiap server bukti, dan sejak itu kesaksianmu dihitung untuk kuorum. Pada langkah ini tak ada rahasia yang meninggalkan mesinmu — itulah inti pemisahannya: bagian privat tetap padamu selamanya, dan bagian publik tak berguna tanpanya.',
  'bv-s6-t':'Langkah 6 — Umumkan kunci publikmu',
  'bv-status-d':'Kode sumber verifier <strong style="color:var(--text)">belum publik</strong>, jadi langkah-langkah di bawah belum bisa dijalankan semua orang hari ini. Kami tetap menerbitkannya karena sebuah rancangan seharusnya bisa diperiksa sebelum dijalankan, bukan sesudahnya. Kalau kamu ingin menjalankan satu, tanyakan di grup Telegram yang tertaut di halaman depan. Membuka repositori inilah yang akan mengubah panduan ini dari rencana menjadi undangan, dan itu hal berikutnya yang kami utang kepada kalian.',
  'bv-status-t':'Status: beta tertutup — baca sebelum mulai',
  'bv-title':'Atau jadilah verifier biometrik — peran yang membuat keunikan terdesentralisasi',
  'bv-what-d':'Wajah tidak pernah dikirim kepadamu. Mesinmu menyimpan satu <strong style="color:var(--text)">bagian aditif</strong> dari ringkasan 64 byte: sendirian ia tak bisa dibedakan dari derau acak, dan tak ada perhitungan yang bisa kamu jalankan untuk memulihkan wajah darinya. Perbandingan dilakukan bersama anggota komite lainnya, dan tak satu pun dari kalian mengetahui apa pun selain jawabannya — <em>duplikat: ya atau tidak</em>. Ini bukan janji tentang niat baik kami; ini sifat dari aritmetikanya.',
  'bv-what-t':'Apa yang akan kamu simpan — dan apa yang tak pernah kamu lihat',
  'bv-why-d':'Sebuah pendaftaran hanya diterima setelah <strong style="color:var(--text)">beberapa verifier yang berbeda</strong> menyaksikannya. Satu kunci curian tidak cukup — penyerang butuh satu komite penuh. Dan karena <strong style="color:var(--neon)">satu orang hanya boleh memegang satu kunci validator</strong>, membeli sebuah komite berarti menjadi sebanyak itu orang. Dengan 100 verifier, seseorang yang menguasai 10 punya peluang kurang dari 1 banding 1.000 untuk memiliki komite bertiga secara penuh. Setiap orang yang bergabung memperkecil angka itu. Inilah satu-satunya tempat di mana jumlah peserta <em>adalah</em> keamanannya. <strong style="color:var(--text)">Perhitungan ini mengandaikan satu manusia per kunci verifier.</strong> Untuk produksi blok rantai menegakkannya; untuk kunci verifier belum (Langkah 5). Sampai itu ada, angka di atas adalah batas atas keamanan, bukan pengukurannya.',
  'bv-why-t':'Mengapa setiap verifier tambahan membuat jaringan lebih sulit dirusak',
  'x-0-1-split-40-30':'0,1 % · pembagian 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 orang. Batas kekayaan bergeser 5x &#8594; 25x. Tahap fondasi.',
  'x-0-8211-2-years':'0 &#8211; 2 tahun',
  'x-0-perfect-equality':'0 = kesetaraan sempurna',
  'x-1-000-aeq-minted':'+1.000 AEQ diterbitkan',
  'x-1-000-aeq-per-human':'1.000 AEQ per orang',
  'x-1-000-aeq-will-be':'1.000 AEQ akan dikreditkan otomatis',
  'x-10-000-8211-1m-humans':'10.000 &#8211; 1 juta orang. Minimal 10 node. Sepenuhnya terdesentralisasi.',
  'x-100-8211-10-000-humans':'100 &#8211; 10.000 orang. Batas tetap 25x. Node bebas bergabung.',
  'x-100-maximum-concentration':'100 = pemusatan maksimum',
  'x-1m-humans-global-ubi-at':'Lebih dari 1 juta orang. Pendapatan dasar global berskala besar. Target Gini &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Likuiditas LP 30 %',
  'x-9679-treasury-10':'&#9679; Kas 10 %',
  'x-9679-ubi-pool-20':'&#9679; Dana pendapatan dasar 20 %',
  'x-9679-validators-40':'&#9679; Validator 40 %',
  'x-active-validators':'Validator aktif',
  'x-add-aequitas-chain-to-metamask':'Tambahkan rantai Aequitas ke MetaMask untuk melihat saldo AEQ, mengirim transaksi, dan berinteraksi dengan kontrak V7 langsung dari peramban atau dompet ponselmu.',
  'x-admin-keys-or-governance-votes':'kunci admin atau pemungutan suara tata kelola',
  'x-aeq-activity':'AKTIVITAS AEQ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'BlockDAG Aequitas — tidak ada yang terbuang',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Rantai Aequitas (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas mewujudkannya secara matematis. Setiap orang yang terverifikasi menerima tepat 1.000 AEQ &#8212; miliarder maupun petani subsisten, tanpa kecuali. Empat mekanisme redistribusi mencegah ketimpangan menumpuk tanpa batas. Koefisien Gini dicatat di rantai secara waktu nyata.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — rantai bukti kemanusiaan',
  'x-android-apk-direct-download':'APK Android · unduhan langsung',
  'x-architecture':'Arsitektur',
  'x-automatic-on-chain':'otomatis, di rantai',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (graf berarah tanpa siklus)',
  'x-blockdag-parallel-production':'BlockDAG · produksi paralel',
  'x-blockdag-proof-of-humanity':'BlockDAG + bukti kemanusiaan',
  'x-blue-score':'«skor biru»',
  'x-both-blocks-are-kept-ghostdag':'Kedua blok tetap disimpan — GHOSTDAG menyertakan yang bersamaan dan tetap menghitungnya dalam urutan kanonik.',
  'x-canonical-winner':'pemenang kanonik',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Setara dengan AS (0,41) atau Prancis (0,32). Masih dalam kisaran kebanyakan ekonomi maju. Redistribusi aktif meratakan kurvanya.',
  'x-confirm-ward-is-alive':'✓ KONFIRMASI ORANG TERSEBUT MASIH HIDUP',
  'x-core-technology':'Teknologi inti',
  'x-daily-ubi-returns-to-all':'pendapatan dasar harian kembali ke semua orang terverifikasi',
  'x-demurrage-0-5-mo':'Uang menyusut (0,5 %/bulan)',
  'x-device-bound-zk-proof-one':'Bukti ZK terikat perangkat · satu pendaftaran per perangkat',
  'x-diagonal-line-perfect-equality':'garis diagonal = kesetaraan sempurna',
  'x-disconnect-wallet':'⊘ PUTUSKAN DOMPET',
  'x-distinct-proposers-recent-blocks':'Pengusul berbeda, blok terbaru',
  'x-distribution':'📈 Distribusi',
  'x-elliptic-curve':'Kurva eliptik',
  'x-entire-distribution':'seluruh distribusi',
  'x-evm-compatible':'Kompatibel dengan EVM',
  'x-fill-ghostdag-verdict-thin-ring':'Isian = putusan GHOSTDAG · cincin tipis = pengusul · satu kolom per ketinggian. Arahkan kursor ke blok untuk rinciannya.',
  'x-generate-node-binding-signature':'🔗 Buat tanda tangan pengikatan',
  'x-run-a-coordinator':'🚪 Jalankan koordinator',
  'co-title':'Atau jalankan koordinator — pintu yang dilewati setiap manusia',
  'co-desc':'Koordinator adalah tempat seseorang tiba: ia mengeluarkan tantangan, menyebarkan hasil tangkapan ke para verifier, menghitung suara mereka, dan menerbitkan atestasi yang menjadi dasar rantai mencetak. Lama sekali hanya ada satu — artinya setiap pendaftaran di jaringan melewati satu mesin. Bukan karena ada yang kurang, melainkan karena belum ada yang menjalankan yang kedua.',
  'co-status-t':'Status: beta tertutup — peringatan yang sama seperti verifier',
  'co-status-d':'Koordinator berada di repositori yang sama dengan verifier, dan repositori itu <strong style="color:var(--text)">belum publik</strong>. Karena itu langkah-langkah di bawah belum bisa diselesaikan semua orang hari ini. Tetap diterbitkan, dengan alasan yang sama: sebuah rancangan harus dapat diperiksa sebelum digelar, bukan sesudahnya.',
  'co-power-t':'Apa yang bisa dilakukan koordinator — dan apa yang tidak',
  'co-power-d':'Ia <strong style="color:var(--text)">tidak bisa menciptakan manusia</strong>. Tidak ada bio_hash sebelum beberapa verifier berbeda mengatestasinya, dan koordinator tidak memegang satu pun kunci mereka. Yang bisa ia lakukan adalah mengikat bio_hash yang <strong style="color:var(--text)">sudah ada</strong> ke sebuah dompet — sehingga yang tidak jujur dapat mengalihkan alokasi ke alamat pilihannya. Itu kuasa nyata, ia tumbuh dengan setiap koordinator tambahan, dan siapa pun yang menimbang kepercayaan perlu tahu bedanya.',
  'co-safe-t':'Mengapa koordinator kedua aman sama sekali',
  'co-safe-d':'Dulu tidak. Sampai Agustus 2026 janji <strong style="color:var(--text)">satu manusia, satu pendaftaran</strong> bergantung pada kunci Redis di dalam koordinator — dan dua koordinator independen tidak berbagi Redis, sehingga dua pendaftaran serentak orang yang sama akan lolos keduanya. Kini <strong style="color:var(--text)">setiap verifier memeriksa sendiri</strong>, sebelum penulisannya sendiri, apakah wajah itu sudah terdaftar. Jaminan itu tidak lagi bergantung pada layanan atau rahasia bersama, jadi koordinator bisa bergabung atau hilang tanpa mengubahnya.',
  'co-need-t':'Yang kamu butuhkan',
  'co-need-d':'Akun Aequitas terdaftar — aturan yang sama seperti memproduksi blok dan memverifikasi: satu manusia, satu kunci. Server dengan Docker dan alamat HTTPS publik, karena peramban tidak menyerahkan kamera ke halaman yang tidak aman. Dan dua kunci milikmu sendiri, yang kamu bangkitkan sendiri dan tak pernah meninggalkan mesinmu: satu menandatangani atestasimu, satu memetakan alamat dompet ke penanda.',
  'co-keys-t':'Jangan pernah menerima kunci dari siapa pun — termasuk kami',
  'co-keys-d':'Dua koordinator dengan satu kunci penanda tangan bukanlah dua koordinator; itu satu dengan dua alamat, dan kuorum yang seharusnya melindungi orang akan tampak terpenuhi padahal tidak. Bangkitkan kedua kunci di mesinmu sendiri, dengan keacakanmu sendiri, dan jangan biarkan satu pun keluar.',
  'co-auth-t':'Mengesahkan kuncimu — tanpa izin siapa pun',
  'co-auth-d':'Selama kuncimu belum disahkan, verifier menolak semua yang ditandatanganinya. Pengesahan butuh dua bukti dan persetujuan siapa pun tidak diperlukan: dompetmu menandatangani bahwa ada manusia terdaftar di balik kunci ini, dan koordinatormu membuktikan di hostnya sendiri bahwa kunci itu memang miliknya. Yang pertama kamu hasilkan dengan tombol di atas; yang kedua dihasilkan koordinatormu sendiri. Sampai Agustus 2026 kamu juga butuh rahasia bersama dari kami — yang berarti rahasia itu <em>adalah</em> izinnya. Itu sudah tiada.',
  'co-pernode-t':'Daftar ini per-node, dan itu disengaja',
  'co-pernode-d':'Pengesahan yang ditulis ke satu node tidak berpindah ke yang lain — tidak ada transaksi untuk itu dan tidak ada gosip. Daftar kepercayaan yang direplikasi justru akan menjadi otoritas pusat yang sengaja tidak ada di sistem ini: setiap operator memutuskan sendiri atestasi siapa yang diterima nodenya. Biayanya, pengesahanmu harus dikirim ke setiap node yang harus menghormatinya. Tanda tangannya sendiri dapat dipindahkan: tanda tangani sekali, kirim ke mana-mana; node yang kamu lewati akan terus menolakmu.',
  'co-law-t':'Apa yang kamu ketahui tentang orang lain — dan konsekuensinya',
  'co-law-d':'Tangkapan itu lewat padamu; kamu meneruskannya dan tidak menyimpan apa pun. Tetapi hanya kamu yang memegang pemetaan antara alamat dompet dan penanda bagi orang-orang yang mendaftar melaluimu — itulah sebabnya kunci penandamu harus tetap milikmu: jika dibagi, operator mana pun dapat menghitung penanda untuk alamat publik mana pun dan menelusuri wajah siapa itu. Artinya juga kamu menjadi <strong style="color:var(--text)">pengendali data</strong> bagi orang-orang itu menurut GDPR. Bukan kami. Permintaan akses, penghapusan dan keberatan sampai kepadamu, dan itu bukan formalitas.',
  'co-limit-t':'Satu batasan yang timbul darinya',
  'co-limit-d':'Penghapusan berdasarkan alamat dompet hanya berhasil di koordinator tempat pendaftaran dibuat: penandamu bergantung pada kuncimu, dan koordinator lain menurunkan penanda berbeda untuk alamat yang sama. Maka "tidak ditemukan" dari tempat lain berarti "tidak terdaftar di sini", bukan "tidak terdaftar" — dan jawabannya menyatakan itu. Jalur melalui bio_hash sendiri, yang menjadi milik orang itu dan tidak butuh operator sama sekali, bekerja di setiap koordinator, karena pengenal itu tetap sama.',
  'x-authorize-coordinator-key':'🔑 Otorisasi kunci koordinator',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — satu urutan sah dari graf yang kusut',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Koefisien Gini',
  'x-gini-coefficient-0-1':'Koefisien Gini (0–1)',
  'x-gini-index-history':'Riwayat indeks Gini',
  'x-gini-target-scandinavian-level':'Target Gini (tingkat Skandinavia)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'ZKP Groth16 (tanpa pengetahuan)',
  'x-guardian-system-8212-human-failsafe':'Orang kepercayaan &#8212; pengaman manusiawi untuk dompet yang hilang',
  'x-hash-wallet':'Hash / dompet',
  'x-healthier-than-most-nations-on':'Lebih sehat daripada kebanyakan negara di dunia. Setara Skandinavia (0,27) dan Jerman (0,31). Batas kekayaan dan uang menyusut menjaga distribusi tetap adil.',
  'x-higher-than-most-european-nations':'Lebih tinggi daripada kebanyakan negara Eropa — setara Brasil (0,53) atau Rusia. Redistribusi protokol bekerja pada intensitas tinggi.',
  'x-honest-limitation':'Keterbatasan yang diakui:',
  'x-how-it-works':'Cara kerjanya',
  'x-how-to-read-this-chart':'Cara membaca grafik ini:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'orang dapat mendaftar',
  'x-imagine-a-world-where-every':'«Bayangkan dunia tempat setiap orang di Bumi &#8212; tak peduli di mana ia lahir, bahasa apa yang ia gunakan, atau seberapa banyak uang orang tuanya &#8212; menerima pendapatan harian yang terjamin semata-mata karena ia manusia. Bukan sebagai sedekah. Sebagai hak matematis, ditegakkan oleh kode yang tak bisa dibatalkan pemerintah atau perusahaan mana pun.»',
  'x-inactive-escrow':'Penitipan karena ketidakaktifan',
  'x-inactivity-timeline':'Lini masa ketidakaktifan',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (tahan pasca-kuantum)',
  'x-key-protections':'Perlindungan utama:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — pengembangan Aequitas sendiri melampaui GHOSTDAG ber-K tetap',
  'x-knightdag-secured':'· diamankan KnightDAG',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Seperti Skandinavia (~0,27)',
  'x-liquidity-pool-30':'Kolam likuiditas (30 %)',
  'x-loading-blocks':'Memuat blok…',
  'x-loading-topology':'Memuat topologi…',
  'x-loading-transactions':'Memuat transaksi…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Kurva Lorenz — distribusi AEQ di antara orang-orang',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: jika saldo AEQ tampak 0 setelah pendaftaran, buka Pengaturan → Jaringan → hapus rantai Aequitas → tambahkan lagi lewat situs ini',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: jika AEQ tampak 0 setelah ditambahkan, hapus jaringannya dan tambahkan lagi dengan tombol di atas.',
  'x-money-exists-because-people-exist':'Uang ada karena manusia ada. Maka setiap orang seharusnya memiliki bagian yang sama, semata karena ia manusia.',
  'x-money-exists-because-people-exist-2':'«Uang ada karena manusia ada. Tidak lebih, tidak kurang.»',
  'x-most-unequal-currency-ever':'Mata uang paling timpang sepanjang masa',
  'x-multi-validator-network':'Jaringan dengan banyak validator',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: belum bermakna',
  'x-no-snapshots-yet-first-one':'Belum ada catatan — yang pertama disimpan setelah pembagian berikutnya.',
  'x-no-stake-blockchain':'Blockchain tanpa jaminan',
  'x-node-operator-guide-pdf':'📄 Panduan pengelola node (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET harus merupakan orang terdaftar di Aequitas',
  'x-one-human-one-wallet-1':'Satu orang = satu dompet = 1.000 AEQ',
  'x-p2p-protocol':'Protokol P2P',
  'x-paid-out-daily':'dibayarkan setiap hari',
  'x-permanent-on-chain':'Permanen · di rantai',
  'x-phase-roadmap-8212-the-path':'Peta jalan bertahap &#8212; jalan menuju skala global',
  'x-phase-transitions-are-automatic-8212':'Perpindahan tahap berlangsung otomatis &#8212; dipicu ambang jumlah orang, ditegakkan oleh kontrak. Tanpa pemungutan suara, tanpa kunci admin.',
  'x-planned-post-beta':'Direncanakan (setelah beta)',
  'x-postgresql-persistent':'PostgreSQL (persisten)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Sediakan likuiditas AEQ / tUSD untuk memperoleh 30 % dari seluruh biaya penukaran, dibagikan setiap hari.',
  'x-recorded-after-each-ubi-distribution':'Dicatat setelah setiap pembagian pendapatan dasar. Menunjukkan bagaimana kesetaraan berkembang seiring jaringan tumbuh. Makin rendah makin baik — targetnya Gini di bawah 0,30.',
  'x-redistribution':'REDISTRIBUSI',
  'x-run-a-node':'⚙️ Jalankan node',
  'x-run-a-verifier':'⚙️ Jalankan verifier',
  'x-set-guardian':'🛡 TETAPKAN ORANG KEPERCAYAAN',
  'x-swap-fees-0-1':'Biaya penukaran (0,1 %)',
  'x-sybil-resistance-8212-current-state':'Ketahanan Sybil &#8212; keadaan sekarang, sejujurnya',
  'x-the-4-redistribution-mechanisms':'Empat mekanisme redistribusi',
  'x-the-core-innovation':'Gagasan intinya',
  'x-the-matching-threshold-has-not':'Ambang kecocokan belum dikalibrasi terhadap pengambilan gambar nyata',
  'x-the-vision-8212-a-global':'Visinya &#8212; protokol pendapatan dasar sedunia',
  'x-the-year-is-2009-satoshi':'Tahun 2009. Satoshi Nakamoto merilis Bitcoin. Untuk pertama kalinya nilai dapat berpindah antara dua orang tanpa bank. Sebuah revolusi sejati. Tetapi hampir seketika ada yang melenceng.',
  'x-this-is-not-a-0815':'Ini bukan blockchain biasa yang menghasilkan satu blok pada satu waktu. Aequitas menjalankan BlockDAG sungguhan, diurutkan oleh GHOSTDAG — dan sejak 2026 diamankan oleh KnightDAG, pengembangan adaptif miliknya sendiri. Pada mekanisme inilah setiap saldo, setiap pembayaran, dan setiap batas kekayaan akhirnya bergantung, agar ada satu riwayat yang disepakati.',
  'x-today-beta':'Hari ini (beta)',
  'x-today-this-verifies-one-device':'Hari ini ini memastikan satu perangkat, belum satu orang yang unik',
  'x-traditional-blockchain-wasted-work':'Blockchain tradisional — kerja yang terbuang',
  'x-treasury-10':'Kas (10 %)',
  'x-trusted-verified-human':'orang terverifikasi yang tepercaya',
  'x-two-validators-produce-at-once':'Dua validator memproduksi bersamaan → satu menang, satu dibuang — kerja yang hilang, dan itu membatasi seberapa cepat jaringan dapat melaju dengan aman.',
  'x-ubi-pool-20':'Dana pendapatan dasar (20 %)',
  'x-validators-pool-40':'Dana validator (40 %)',
  'x-view-source-on-github':'🐙 Lihat kode di GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'Pengali batas kekayaan — penggeser tahap awal',
  'x-wealth-cap-overflow':'Kelebihan di atas batas kekayaan',
  'x-wealth-distribution-analysis':'Analisis distribusi kekayaan',
  'x-what-happens-when-someone-is':'Apa yang terjadi bila seseorang dirawat di rumah sakit, dipenjara, atau meninggal? Di sebagian besar sistem kripto, dompet yang hilang hilang selamanya. Aequitas punya pemulihan ketidakaktifan berlapis tiga.',
  'x-what-is-a-guardian':'Apa itu orang kepercayaan?',
  'x-what-is-and-is-not':'Apa yang bersifat pribadi dan apa yang tidak:',
  'x-what-would-a-cryptocurrency-look':'«Seperti apa mata uang kripto yang dirancang sejak awal agar adil bagi setiap manusia?»',
  'x-why-a-normal-blockchain-isn':'Mengapa blockchain biasa tidak cukup',
  'x-worse-than-any-country-on':'Lebih buruk daripada negara mana pun di dunia (rekor Afrika Selatan: 0,63). Mendekati Bitcoin (0,85). Protokol bekerja pada intervensi maksimum — batas kekayaan dan redistribusi sepenuh tenaga.',
  'x-year-2-180d':'Tahun 2 +180 h',
  'x-zk-device-key-proof':'Bukti ZK kunci perangkat',
  'swap-price-flat':'Tidak ada transaksi pada periode ini — harga tidak bergerak. Grafiknya berfungsi; pasarnya yang sepi.',
  'mpc-optin-title':'Opsional — membantu memeriksa pendaftaran ganda (disiapkan, belum aktif)',
  'mpc-optin-desc':'Sudah disiapkan, tetapi belum aktif. Nanti node-mu dapat membantu memverifikasi bahwa tidak ada yang mendaftar dua kali tanpa pernah melihat data biometrik siapa pun: setiap pihak hanya memegang satu bagian matematis dari tiap templat — sekadar derau bila berdiri sendiri — dan mereka membandingkan tangkapan baru bersama-sama, sehingga tidak ada satu mesin pun yang bisa merekonstruksi apa pun. Saat ini jalur ini tidak memutuskan apa pun: pemeriksaan duplikat tidak melewatinya, dan komitenya adalah daftar tetap, bukan diundi otomatis.',
  'mpc-optin-note':'Berkas bagian berisi keacakan sekali pakai yang hanya boleh dipegang node-mu — jangan pernah menyalinnya ke mesin lain atau memasukkannya ke repositori. Saat ini berkas itu harus berasal dari operator, dan itulah ketergantungan terpusat yang tersisa. Kamu tidak perlu kunci baru: node-mu mengenalkan diri dengan kunci penanda tangan yang sudah dipakai untuk blok.',
  'logo-sub':'BUKTI KEMANUSIAAN','live':'LANGSUNG',
  'reg-title':'🔐 Daftar sebagai Manusia Terverifikasi',
  'reg-sub':'Bergabunglah dengan jaringan Aequitas dan terima hibah Pendapatan Dasar Universal sebesar 1.000 AEQ. Satu kali, permanen, dan sepenuhnya gratis. Tidak ada data pribadi yang pernah disimpan.',
  'app-title':'PENDAFTARAN HANYA MELALUI APLIKASI ANDROID',
  'app-text':'Saat pendaftaran, kamera merekam wajah Anda dan urutan pendeteksian keaslian singkat. Layanan pencocokan independen memeriksa bahwa ada orang hidup dan bahwa wajah ini belum terdaftar; mereka harus sepakat secara kuorum. Bukti ZK Groth16 kemudian membawa hasilnya ke rantai tanpa mengungkapkan apa pun tentang Anda. <strong style="color:var(--gold)">1.000 AEQ Anda dikreditkan otomatis</strong> setelah verifikasi. <strong style="color:var(--gold)">Catatan:</strong> ambang pencocokan belum dikalibrasi dengan rekaman nyata — lihat FAQ di bawah.',
  's1t':'Perekaman wajah','s1d':'Aplikasi merekam wajah Anda dan urutan pendeteksian keaslian singkat, lalu mengirimkannya ke layanan pencocokan independen. Mereka memeriksa bahwa ada orang hidup di depan kamera dan membandingkan wajah dengan semua yang sudah terdaftar. Gambar dibuang setelah diproses.',
  's2t':'Pembuatan Bukti ZK','s2d':'Bukti ZK Groth16 mengikat bio_hash Anda ke commitment = keccak256(bioHash‖wallet) tanpa mengungkapkannya. Nullifier diturunkan dari hash itu, sehingga wajah yang sama tidak dapat dihitung dua kali — lihat FAQ di bawah.',
  's3t':'Hubungkan Dompet','s3d':'Aplikasi membuka MetaMask di halaman ini · hubungkan dompet Ethereum Anda · bukti terikat secara kriptografis ke alamat Anda',
  's4t':'1.000 AEQ Dikreditkan','s4d':'Pendaftaran dikonfirmasi di BlockDAG Aequitas dalam 6 detik · 1.000 AEQ dikreditkan seketika · identitas Anda dicatat permanen sebagai manusia terverifikasi',
  'priv-bar':'🔒 Pemeriksaan wajah oleh kuorum · Groth16 ZKP · Gambar dibuang setelah pemeriksaan · Satu pendaftaran per orang',
  'conn-wallet':'DOMPET TERHUBUNG','proof-recv':'⚡ BUKTI ZK DITERIMA','proof-hint':'Hubungkan dompet untuk mendaftar',
  'btn-conn':'🦊 HUBUNGKAN METAMASK','btn-reg':'🔐 DAFTAR ON-CHAIN',
  'btn-wc':'🔗 HUBUNGKAN WALLETCONNECT',
  'reg-log-hint':'// Buka Aplikasi Android Aequitas untuk membuat bukti Anda, lalu kembali ke sini...',
  'reg-details':'Detail Pendaftaran','k-network':'Jaringan','k-chainid':'ID Rantai','k-grant':'Hibah UBI',
  'k-fee':'Biaya Gas','free':'GRATIS — sepenuhnya tanpa gas','k-limit':'Pendaftaran','k-limit-v':'Sekali per orang · permanen · tidak dapat diubah',
  'k-bio':'Wajah','never-stored':'Gambar dibuang setelah pemeriksaan — tidak ada validator yang memegang templat utuh',
  'k-proof':'Sistem Bukti','k-conf':'Konfirmasi','k-conf-v':'Dalam 1 detik (1 blok)',
  'k-sybil':'Perlindungan Sybil','k-sybil-v':'Satu identitas per orang · terikat wajah, ambang belum dikalibrasi',
  's-height':'Tinggi Blok',
  's-humans':'Manusia Terverifikasi',
  's-supply':'Total Pasokan','s-supply-sub':'Selalu = Manusia × 1.000 AEQ',
  's-uptime':'Waktu Aktif',
  'k-chain':'Nama Rantai','k-symbol':'Simbol','k-btime':'Waktu Blok',
  'k-cons':'Konsensus','k-storage':'Penyimpanan','k-dec':'Desimal',
  'btn-add-mm':'+ TAMBAHKAN JARINGAN AEQUITAS',
  'humans-title':'Manusia Terverifikasi di Aequitas Chain',
  'h-what':'Apa itu Manusia Terverifikasi?','h-what-t':'Manusia Terverifikasi adalah alamat dompet yang terbukti milik seseorang yang wajahnya belum terdaftar. Layanan pencocokan independen harus sepakat secara kuorum, dan hanya bukti ZK Groth16 yang sampai ke rantai — tanpa gambar dan tanpa templat. <strong style="color:var(--gold)">Sampai 2026-08-23 ini memverifikasi satu perangkat, bukan satu orang; kini tidak lagi.</strong>',
  'h-zkp':'Sistem Bukti ZK','h-zkp-t':'Aequitas menggunakan Groth16 pada BN128 — kurva yang sama dengan Ethereum dan Zcash. ~200 byte, ~10ms. commitment = keccak256(deviceKey‖wallet). Nullifier terikat ke perangkat ini: kehilangan ponsel tidak membuat identitas kedua di perangkat itu, tetapi perangkat lain masih dapat mendaftar secara terpisah. Materi kunci tidak pernah diungkapkan atau disimpan di server.',
  'h-sybil':'Ketahanan Sybil — Kondisi Saat Ini','h-sybil-t':'Nullifier diturunkan dari bio_hash wajah Anda, sehingga wajah yang sama tidak dapat didaftarkan dua kali — juga lintas perangkat, yang tidak pernah bisa dilakukan kunci perangkat. Yang menjadi dasarnya adalah ambang pencocokan yang belum dikalibrasi dengan rekaman nyata: kriptografinya persis, biometrik di bawahnya adalah pengukuran yang laju kesalahannya belum terukur.',
  'h-global':'Inklusi Keuangan Global','h-global-t':'Tidak perlu rekening bank, kartu kredit, atau pengalaman kripto sebelumnya. Cukup ponsel Android dengan kamera. Aequitas dirancang agar dapat diakses setiap manusia di Bumi.',
  'h-bio-hw':'Peta Jalan Verifikasi Identitas','h-bio-hw-t':'Saat ini (beta): pemeriksaan wajah oleh layanan pencocokan independen yang harus sepakat secara kuorum. Ambangnya belum dikalibrasi dengan rekaman nyata — perlu sekitar 1000 pasangan impostor sebelum ada angka yang disebut. Direncanakan: kalibrasi tersebut, dan pemeriksaan duplikat di mana tidak ada layanan yang memegang templat utuh.',
  'reg-humans':'Manusia Terdaftar','h-desc':'Setiap alamat di bawah milik seseorang yang wajahnya diperiksa oleh layanan independen terhadap semua pendaftaran yang ada, dibuktikan dengan bukti ZK, dan dikreditkan tepat 1.000 AEQ. Registri bersifat permanen, tidak dapat diubah, dan on-chain. Apa yang dijamin dan tidak dijamin ambang saat ini ada di FAQ.',
  'no-humans':'Belum ada manusia terdaftar.\n\nUnduh Aplikasi Android Aequitas dan jadilah yang pertama!',
  'reg-stats':'Statistik Registri','total-humans':'Total Manusia',
  'idx-title':'Indeks Aequitas — Skor Kesetaraan Ekonomi Real-Time',
  'idx-desc':'Indeks Aequitas mengukur ketidaksetaraan ekonomi semua manusia terverifikasi secara real-time. Diturunkan dari koefisien Gini distribusi saldo on-chain. 0 = kesetaraan sempurna. 100 = ketidaksetaraan maksimum.',
  'curr-idx':'Indeks Saat Ini','bar-0':'0 — Kesetaraan Sempurna','bar-100':'100 — Maks. Ketidaksetaraan','wcap-lbl':'Batas Kekayaan Saat Ini:','wcap-mult':'Pengganda:','wcap-avg':'Bagian adil:',
  'gini':'Koefisien Gini','gini-desc':'0 = setara · 1 = tidak setara',
  'supply-desc':'Selalu = Manusia × 1.000 AEQ',
  'phase':'Fase Protokol','phase-desc':'Otomatis berdasarkan jumlah manusia',
  'humans-desc':'Pendaftaran terverifikasi wajah',
  'pools-title':'Pool Redistribusi',
  'pools-desc':'Setiap biaya swap, biaya demurrage, dan kelebihan batas kekayaan secara otomatis dibagi ke empat pool. Tanpa intervensi manual. Semua pool membayar setiap hari.',
  'vel-pool':'Pool Validator','vel-pool-desc':'40% semua biaya → operator node yang mengamankan jaringan',
  'liq-pool':'Pool Likuiditas','liq-pool-desc':'30% semua biaya → penyedia likuiditas, proporsional dengan saham LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% semua biaya → semua manusia terverifikasi secara merata, setiap 24 jam',
  'treasury':'Perbendaharaan','treasury-desc':'10% semua biaya → pengembangan dan pemeliharaan protokol',
  'phases-title':'Fase Protokol',
  'demurrage-title':'Demurrage — Insentif untuk Bersirkulasi',
  'demurrage-desc':'Aequitas mengimplementasikan mekanisme demurrage yang terinspirasi dari mata uang komplementer historis. Saldo AEQ yang tidak aktif perlahan kehilangan nilai untuk mencegah penimbunan.',
  'dem-rate-k':'Tingkat Peluruhan','dem-rate-v':'0,5% per bulan (berkelanjutan, tidak bertahap)',
  'dem-grace-k':'Masa Tenggang','dem-grace-v':'3 bulan tidak aktif sebelum peluruhan dimulai',
  'dem-reset-k':'Reset Timer','dem-reset-v':'Setiap transfer, swap, atau tindakan likuiditas mereset timer',
  'dem-dest-k':'AEQ yang meluruh pergi ke','dem-dest-v':'Pool redistribusi (pembagian 40/30/20/10)',
  'dem-warn-k':'Sistem Peringatan','dem-warn-v':'Pemberitahuan 14 hari (sekali) + pengingat 7 hari berulang setiap login',
  'story-title':'Kisah Aequitas — Mengapa Ini Ada',
  'nodes-title':'Node Aktif — Topologi Jaringan Saat Ini','nodes-desc':'Jaringan Aequitas saat ini beroperasi pada beberapa node yang tersebar secara geografis (jumlah saat ini di atas). Semuanya berpartisipasi dalam produksi blok, sinkronisasi status, dan layanan API. Jaringan dirancang untuk mendukung node tambahan — operator mana pun dapat bergabung.',
  'run-node-title':'Jalankan Node Anda Sendiri — Bantu Amankan Jaringan',
  'run-node-desc':'Setiap manusia terdaftar dapat menjalankan node Aequitas — tanpa stake, tanpa lamaran, tanpa izin dari kami. Satu manusia, satu kunci validator: node yang NODE_OPERATOR_WALLET-nya bukan manusia terdaftar ditolak dengan HTTP 403, sebab jika tidak satu orang bisa diam-diam menjadi seluruh himpunan validator. Node berpartisipasi dalam produksi blok dan memvalidasi registri manusia. Operator node mendapatkan bagian biaya protokol melalui Pool Validator (40% semua biaya swap, didistribusikan setiap hari).',
  'bootstrap-title':'Jalankan Node Anda Sendiri','bootstrap-desc':'Siapa pun dapat bergabung dengan jaringan Aequitas dengan menjalankan node. Unduh panduan node untuk instruksi langkah demi langkah.',
  'tech-title':'Spesifikasi Teknis','mm-config':'Konfigurasi MetaMask',
  'k-lang':'Bahasa','k-src':'Kode Sumber','evm-yes':'Ya — JSON-RPC /rpc · Kompatibel MetaMask',
  'proto-label':'Protokol Aequitas V7 — Dokumentasi Teknis',
  'ca-title':'Alamat Kontrak','ca-text':'Rantai: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 menetapkan aturan ekonomi Aequitas dan memegang daftar yang membuatnya dapat ditegakkan: setiap nullifier yang pernah diklaim, setiap registrasi, batas kekayaan, dan formula demurrage. Kontraknya tidak dapat diubah — tidak ada kunci admin, tidak ada proxy peningkatan, tidak ada pemungutan suara tata kelola yang bisa mengubah satu baris pun. Namun yang menyelesaikan transfer nyata adalah lapisan rantai: node mencegat panggilan ERC-20 sebelum mencapai EVM dan menerapkannya pada buku besarnya sendiri — itulah yang membuat transfer berlangsung di bawah satu detik dan tanpa gas. Kontrak adalah buku aturan dan daftarnya; rantai adalah mesin yang menjalankannya, dan sumbernya terbuka.<br><br>Kontrak BioVerifier menerima bukti zero-knowledge Groth16 yang dihasilkan sepenuhnya di perangkat Android pengguna. Ia memverifikasi secara matematis on-chain dalam ~10 ms bahwa nullifier yang dikirim diturunkan dengan benar dari rahasia yang dimiliki pendaftar, dan rantai menolak setiap nullifier yang pernah dilihatnya — tanpa pernah mengetahui nama, identitas, atau data biometrik mereka. Itu menutup kemungkinan pendaftaran kedua dari sumber identitas yang sama; apakah sumber itu seorang manusia atau sebuah perangkat bergantung pada apakah mode biometrik aktif. Inilah yang membuat registrasi tanpa gas dan tanpa investasi menjadi mungkin: bukti adalah satu-satunya hal yang pernah meninggalkan perangkat.<br><br>Kombinasi itulah yang benar-benar baru: aturan dan daftar «satu manusia, satu registrasi» berada dalam kontrak yang tak seorang pun — bukan operator, bukan perusahaan, bukan pemerintah — dapat menulis ulang, dan kode yang menjalankannya bersifat terbuka serta dapat direproduksi dari repositori ini. Semuanya dapat diperiksa siapa saja. Yang masih memerlukan kepercayaan adalah pengoperasian node itu sendiri, dan cara jujur untuk menguranginya adalah lebih banyak validator independen, bukan kalimat yang lebih tegas di sini.',
  'poa-title':'1. BUKTI KEHIDUPAN — Pemulihan Saldo Tidak Aktif','poa-text':'<p>Apa yang terjadi dengan AEQ ketika orang meninggal atau menjadi tidak mampu secara permanen? Di Bitcoin, dompet yang hilang berarti pasokan yang hilang selamanya. Aequitas menyelesaikan ini melalui sistem pemulihan ketidakaktifan multi-tahap: jika dompet tidak menunjukkan aktivitas untuk jangka waktu yang lama, saldonya secara bertahap dikembalikan ke komunitas melalui pool UBI.</p>',
  'poa-box':'Tahun 0–2: Penggunaan normal — tanpa batasan<br>Tahun 2: Peringatan 1 — Guardian dapat merespons atas nama<br>Tahun 2+60h: Peringatan 2 — urgensi meningkat<br>Tahun 2+120h: Peringatan 3 — pemberitahuan terakhir<br>Tahun 2+180h: AEQ dipindahkan ke ESCROW pribadi (masih dapat dipulihkan)<br>Tahun 4: Jika masih tidak aktif — ESCROW dirilis ke Pool UBI',
  'guard-title':'2. SISTEM GUARDIAN — Perlindungan Manusia','guard-text':'<p>Bagaimana jika seseorang dirawat di rumah sakit atau tidak dapat mengakses perangkatnya selama berbulan-bulan? Sistem Guardian memungkinkan orang terpercaya — manusia terverifikasi lainnya — mengonfirmasi bahwa pemilik dompet masih hidup. Guardian memiliki nol akses keuangan: hanya dapat memanggil satu fungsi yang mereset timer ketidakaktifan. Tidak dapat memindahkan, membelanjakan, atau mengakses dana dalam keadaan apapun.</p>',
  'guard-box':'1 Guardian per manusia · harus manusia terverifikasi di Aequitas<br>Guardian HANYA dapat memanggil confirmAlive() — nol hak transaksi<br>Guardian TIDAK DAPAT memindahkan dana, mentransfer AEQ, atau mengakses dompet<br>Maksimal 3 wali per Guardian · Kunci waktu 7 hari · Tanpa hubungan melingkar',
  'dem-title':'3. DEMURRAGE — Mekanisme Anti-Penimbunan',
  'dem-box':'Hanya dikenakan pada bagian di atas bagian wajar Anda — saldo sama atau di bawahnya tidak pernah meluruh<br>Tingkat: 0,5%/bulan setelah 3 bulan ketidakaktifan (berkelanjutan, tidak bertahap)<br>Timer direset secara otomatis dengan transfer, swap, atau tindakan likuiditas apapun<br>AEQ yang meluruh didistribusikan ulang ke empat pool — tidak pernah dibakar<br>Pemberitahuan 14 hari ditampilkan sekali · 7 hari diulang di setiap sesi aktif',
  'dem-text':'<p>Demurrage adalah biaya kepemilikan uang — suku bunga negatif yang membuat penimbunan mahal dan sirkulasi menarik. Eksperimen Wörgl (Austria, 1932) mengurangi pengangguran lokal 25% dalam satu tahun. Bank Sentral Austria menutupnya justru karena bekerja terlalu baik. Chiemgauer (Jerman, 2003) beroperasi dengan prinsip yang sama dengan sukses selama lebih dari 20 tahun.</p>',
  'cap-title':'4. BATAS KEKAYAAN — Penerapan Keadilan Matematis','cap-box':'Batas bootstrap: max(5,min(N,25))× saldo rata-rata saat ini<br>1–4 manusia: 5× · +1× per manusia · 25+: 25× permanen<br>Berlaku untuk SEMUA alamat kecuali 4 pool protokol<br>Kelebihan AEQ langsung didistribusikan ulang · Tanpa intervensi manual',
  'ubi-title':'5. PENDAPATAN DASAR UNIVERSAL — Redistribusi Harian','ubi-box':'Sumber pendapatan Pool UBI:<br>· 20% semua biaya swap dari pool AMM AEQ↔tUSD<br>· Overflow dari penerapan batas kekayaan<br>· Biaya demurrage dari akun tidak aktif<br>· Escrow tidak aktif dirilis setelah 4 tahun<br><br>Distribusi: Setiap 24 jam, seluruh saldo pool UBI dibagi rata di antara semua manusia terverifikasi yang terdaftar. Pool direset ke nol dan segera mulai diisi ulang dari aktivitas protokol yang berkelanjutan.',
  'inf-title':'6. TANPA INFLASI ALGORITMIK — Formula Pasokan Tetap','inf-box':'SATU-SATUNYA peristiwa yang menciptakan AEQ baru: manusia terverifikasi baru mendaftar.<br><br>Total Pasokan = Manusia Terverifikasi × 1.000 AEQ<br><br>Ini bukan kebijakan — ini diterapkan oleh protokol. Tidak ada admin yang dapat mencetak AEQ tambahan, tidak ada suara tata kelola yang dapat mengubah penerbitan. AEQ adalah satu-satunya cryptocurrency di mana total pasokan ditentukan semata-mata oleh jumlah manusia hidup yang terverifikasi.',
  'phases-desc':'Pada Fase 0, batas kekayaan menggunakan pengganda bootstrap: max(5, min(N, 25))× saldo rata-rata. Dengan 1–4 manusia: 5× rata-rata. Setiap manusia baru menambah 1×. Pada 25+ manusia: terkunci permanen di 25×. Fase 1+ mempertahankan 25× tetap. Semua transisi otomatis — tanpa pemungutan suara, tanpa kunci admin.',
  'p0':'Bootstrap · &lt;100 manusia · Batas Kekayaan: max(5,min(N,25))× rata-rata · Meluncur 5×→25× hingga manusia ke-25 · Saat ini aktif',
  'p1':'Pertumbuhan · 100–10.000 manusia · Batas Kekayaan: 25× bagian adil = 25.000 AEQ',
  'p2':'Stabilitas · 10.000–1M manusia · Batas Kekayaan: 25× bagian adil = 25.000 AEQ',
  'p3':'Kematangan · 1M+ manusia · Batas Kekayaan: 25× bagian adil = 25.000 AEQ',
  'wealth-cap-explain':'Batas Kekayaan pada Fase 0 (Bootstrap) menggunakan max(5, min(N, 25))× saldo AEQ rata-rata, di mana N = manusia terdaftar. 1–4 manusia: 5× rata-rata. Setiap manusia baru menambah 1×. 25+ manusia: terkunci permanen di 25×. Batas selalu mengikuti saldo rata-rata saat ini.',
  'btn-download-app':'UNDUH APLIKASI AEQUITAS',
  'swap-title':'🔄 Tukar AEQ ↔ tUSD','swap-sub':'Tukarkan AEQ dengan tUSD (dolar uji simulasi) melalui pool likuiditas asli. Biaya 0,1% hanya berlaku untuk pertukaran — transfer AEQ biasa antar orang tetap sepenuhnya gratis.',
  'swap-priv-bar':'🔒 Hanya 0,1% biaya swap · Transfer AEQ-ke-AEQ gratis · tUSD adalah mata uang uji tanpa nilai nyata',
  'swap-your-aeq':'AEQ Anda','swap-your-tusd':'tUSD Anda',
  'swap-fee-est':'Biaya protokol (0,1%)','swap-details-hdr':'Detail Pertukaran',
  'swap-out-lbl':'Anda terima (est.)','swap-impact-lbl':'Dampak harga','swap-rate-lbl':'Nilai tukar',
  'swap-depth-lbl':'Komposisi Pool','amm-title':'x × y = k — AMM Produk Konstan',
  'amm-text':'Saat Anda menukar AEQ dengan tUSD, cadangan AEQ bertambah dan cadangan tUSD berkurang — produknya selalu sama dengan k. Pertukaran lebih besar menyebabkan dampak harga lebih besar. Biaya 0,1% dipotong sebelum rumus diterapkan.',
  'swap-btn-go':'🔄 TUKAR',
  'swap-log-hint':'// Hubungkan dompet untuk menukar...',
  'swap-no-liquidity':'Belum punya tUSD?','swap-faucet-desc':'Manusia terdaftar dapat klaim tUSD uji sekali','swap-btn-faucet':'💧 KLAIM tUSD UJI',
  'swap-addliq-title':'Sediakan Likuiditas','swap-addliq-desc':'Jadilah yang pertama menyetor — rasio Anda menetapkan harga awal.','swap-btn-addliq':'💧 TAMBAH LIKUIDITAS',
  'swap-lp-title':'Posisi LP Anda','swap-lp-share':'Bagian Pool','swap-lp-withdrawable':'Dapat Ditarik',
  'swap-lp-pct-label':'% posisi Anda','swap-lp-youget':'Anda akan terima','swap-btn-removeliq':'🔥 HAPUS LIKUIDITAS',
  'swap-pool-title':'AEQ / tUSD — Status Pool',
  'swap-pool-aeq':'Cadangan AEQ','swap-pool-tusd':'Cadangan tUSD','swap-pool-price':'Harga Spot',
  'swap-fee-bps':'Biaya Swap',
  'swap-pools-addr-title':'Alamat Pool Tokenomik',
  'swap-validators':'Validator (40%)','swap-lps':'Penyedia Likuiditas (30%)','swap-ubi':'Pool UBI (20%)','swap-treasury':'Perbendaharaan (10%)',
  'ubi-hero-title':'PENDAPATAN DASAR UNIVERSAL — POOL UBI',
  'ubi-hero-sub':'Mengumpulkan — pembayaran berikutnya dibagikan merata ke semua manusia terverifikasi dalam:',
  'ubi-bal-lbl':'saldo pool saat ini','ubi-hero-desc':'Dibagi merata di antara semua · dibayar setiap 24j · pool direset ke nol · tidak perlu saldo minimum',
  'ubi-how-fills':'Bagaimana Pool UBI terisi',
  'ubi-src-swap':'Biaya Swap','ubi-src-swap-d':'Setiap swap AEQ↔tUSD berkontribusi 20% dari biaya 0,1%-nya. Lebih banyak trading = pengisian lebih cepat.',
  'ubi-src-dem':'Demurrage','ubi-src-dem-d':'AEQ tidak aktif (3+ bulan) berkurang 0,5%/bulan. 20% jumlah yang berkurang masuk ke UBI.',
  'ubi-src-cap':'Overflow Batas Kekayaan','ubi-src-cap-d':'Dompet yang melebihi batas kekayaan (max(5,min(N,25))× rata-rata) langsung disita kelebihannya. 20% mengalir ke UBI segera.',
  'pools4-header':'Keempat pool redistribusi',
  'ubi-see-above':'lihat hitung mundur di atas','ubi-timer-above':'⏰ hitung mundur ditampilkan di atas','pool-t-timer':'Mengumpulkan — tanpa timer',
  'usp-headline':'Untuk pertama kalinya dalam sejarah — semua memulai dengan setara',
  'usp-sub':'Jika Anda memiliki smartphone Android, Anda memenuhi syarat. Tanpa bank, tanpa pengetahuan kripto, tanpa investasi.',
  'usp-c1-title':'Investasi Awal 0,00','usp-c1-desc':'Pendaftaran sepenuhnya tanpa gas. Tanpa ETH, tanpa MATIC, tanpa kartu kredit. Protokol membayar semua biaya atas nama Anda.',
  'usp-c2-title':'1.000 AEQ untuk setiap manusia','usp-c2-desc':'Miliarder atau petani subsisten — semua mendapat tepat 1.000 AEQ. Tidak lebih, tidak kurang. Start setara, dijamin matematika.',
  'usp-c3-title':'Dapat diakses semua orang','usp-c3-desc':'Tanpa rekening bank, tanpa kartu kredit, tanpa kartu identitas, tanpa perangkat keras tambahan — cukup kamera yang sudah ada di ponsel Android Anda.',
  'usp-c4-title':'UBI harian selamanya','usp-c4-desc':'Setelah terdaftar, Anda secara otomatis menerima bagian harian dari pembayaran UBI — setiap hari, tanpa tindakan apa pun.',
  'v7-intro-title':'Apa itu AequitasV7?',
  'v7-intro-text':'AequitasV7 adalah kontrak pintar inti dari protokol Aequitas. "V7" mengacu pada versi utama ke-7 dari kontrak keadilan. Dikerahkan secara tidak dapat diubah di Aequitas Chain (ID 1926) dan menangani setiap aspek: pendaftaran manusia, verifikasi ZK, manajemen saldo, batas kekayaan, distribusi UBI, biaya swap. Tidak ada admin yang dapat memperbaruinya. Keenam mekanisme membentuk sistem yang saling memperkuat.',
  'swap-sell-label':'Jual','swap-receive-label':'Terima',
  'gini-what-title':'Apa itu Koefisien Gini?','gini-what-text':'Dikembangkan oleh ahli statistik Italia Corrado Gini (1912). Mengukur distribusi kekayaan dengan membandingkan saldo aktual dengan basis yang secara hipotetis sepenuhnya setara. Skala: 0 (semua sama) hingga 1 (satu orang menguasai semua). Digunakan oleh Bank Dunia, OECD, PBB untuk membandingkan negara. Nilai referensi: Bitcoin ≈ 0,85 · Afrika Selatan (rekor dunia) ≈ 0,63 · AS ≈ 0,41 · Jerman ≈ 0,31 · Skandinavia ≈ 0,27 · Target jangka panjang Aequitas: Gini di bawah 0,30.','gini-calc-title':'Bagaimana Indeks Aequitas dihitung','gini-calc-text':'Semua saldo AEQ dikumpulkan. Rumus menghitung perbedaan absolut rata-rata dinormalisasi dengan n2. Hasil 0-1 dikali 100 = Indeks Aequitas.','gini-why-title':'Mengapa Gini','gini-why-text':'Koefisien Gini menangkap distribusi lengkap semua manusia terverifikasi.',
  'guard-title':'🛡 Sistem Guardian','guard-my-lbl':'Guardian Saya','guard-none':'Tidak Ada',
  'guard-set-lbl':'Tetapkan / Ubah Guardian','guard-set-hint':'Harus manusia Aequitas yang terdaftar · Kunci waktu 7 hari · Guardian hanya bisa mengkonfirmasi kelayakan hidup Anda, tidak mengakses dana · Maks. 3 wali per guardian',
  'guard-confirm-lbl':'Konfirmasi Masih Hidup (Sebagai Guardian)','guard-confirm-hint':'Jika wali Anda tidak dapat mengakses wallet mereka, konfirmasi kelayakan hidup mereka untuk mencegah dana mereka berpindah ke escrow setelah 910 hari tidak aktif.','guard-recover-btn':'🔓 PULIHKAN DARI ESCROW',
  'faq-title':'❓ Pertanyaan Umum','faq-q1':'Apakah data biometrik saya aman?','faq-a1':'Wajah Anda direkam dan dikirim ke layanan pencocokan independen — hanya begitulah "satu orang, satu akun" dapat diperiksa sama sekali. Gambar diproses lalu dibuang; tidak disimpan. Yang disimpan adalah templat matematis: terenkripsi dan dipecah menjadi bagian-bagian di antara validator yang dioperasikan terpisah, sehingga tidak ada validator yang pernah memegang templat utuh. Satu batas yang jujur, disebutkan dan tidak disembunyikan: layanan yang menjalankan pembandingan tetap menyimpan templat, karena membandingkan membutuhkannya.',
  'faq-q1b':'Apakah pendaftaran membuktikan saya orang unik yang nyata?','faq-a1b':'Lebih baik daripada yang pernah bisa dilakukan kunci perangkat, dan belum dapat dibuktikan sebagai angka. Wajah dibandingkan dengan semua pendaftaran yang ada oleh layanan independen yang harus sepakat, sehingga orang yang sama di ponsel kedua tetap tertangkap — hal yang tidak pernah bisa dilakukan kunci perangkat. Yang belum ditetapkan adalah laju kesalahan: ambang belum dikalibrasi dengan rekaman nyata, dan itu perlu sekitar 1000 pasangan impostor.',
  'faq-q2':'Bisakah saya mendaftar dengan wallet berbeda nanti?','faq-a2':'Tidak. Sebuah pendaftaran terikat permanen pada satu alamat dompet. Ini disengaja: nullifier yang diturunkan dari wajah Anda hanya terpakai sekali, jadi mendaftar lagi ke dompet lain berarti identitas kedua bagi orang yang sama.',
  'faq-q3':'Apa yang terjadi jika saya kehilangan ponsel?','faq-a3':'AEQ Anda tetap di wallet — terikat ke kunci privat Anda, bukan ponsel. Anda masih bisa mengakses wallet melalui MetaMask dengan frasa benih. Pemulihan wallet tidak bergantung pada pendaftaran biometrik.',
  'path-title':'Pilih Jalur Anda','path-human-title':'Saya adalah Manusia','path-human-desc':'Saya ingin mendaftar, menerima 1.000 AEQ, dan bergabung dengan jaringan penghasilan dasar.','path-human-steps':'1. Unduh Aplikasi Android Aequitas<br>2. Buka kunci dengan kunci layar perangkat Anda (sidik jari/wajah/PIN)<br>3. Hubungkan MetaMask<br>4. Terima 1.000 AEQ seketika',
  'path-node-title':'Saya adalah Operator Node','path-node-desc':'Saya ingin menjalankan node penuh, berpartisipasi dalam produksi blok, dan menghasilkan dari pool validator 40%.','path-node-steps':'1. Daftar sebagai manusia (wajib)<br>2. No entry point to configure — the validator addresses are built in<br>3. Deploy di Contabo/Hetzner/VPS mana pun<br>4. Hasilkan harian dari pool validator',
  'path-dev-title':'Saya adalah Pengembang','path-dev-desc':'Saya ingin membangun di Aequitas, mengintegrasikan API, atau berkontribusi pada protokol.','path-dev-steps':'1. JSON-RPC kompatibel EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Metrik: /metrics (Prometheus)',
  'story-flow-title':'Diagram Aliran Token AEQ','story-topo-title':'Topologi Jaringan — Status Saat Ini',
  'swap-price-title':'AEQ / tUSD — Harga Live','swap-price-desc':'Harga real-time dari cadangan pool (x·y=k). Diperbarui setiap 8 detik dengan data pool terbaru.','swap-price-empty':'Belum ada data pool — tambahkan likuiditas untuk melihat grafik harga.',
  'node-guide-lang-note':'Panduan inline ini dalam bahasa Inggris. PDF terjemahan tersedia dalam bahasa Anda menggunakan tombol di atas.',
  'k-zkp':'Sistem ZKP','k-hash':'Sistem Hash','k-sybil-prot':'Perlindungan Sybil',
  'soc-title':'💬 Media Sosial','soc-sub':'Pengumuman, keadaan rantai, dan pertanyaan yang canggung &mdash; terbuka, di keduanya.',
  'soc-x-desc':'Pengumuman, dan apa yang sebenarnya dilakukan rantai ini. Bentuk singkat.','soc-tg-desc':'Grup terbuka: pertanyaan, operator node, dan bantuan untuk mendaftar.',
  's-validators':'Validator Aktif',
  'expl-heading':'Penjelajah Blok',
},
it:{
  'x-consensus-ghostdag-knightdag':'◆ Consenso: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Codice del contratto',
  'x-demurrage-is-a-holding-cost':'La moneta deperibile comporta un costo di detenzione — un tasso d’interesse negativo che rende costoso accumulare e conveniente far circolare. Esistono precedenti: l’esperimento di Wörgl (Austria, 1932) usò una moneta deperibile e ridusse la disoccupazione locale del 25 % in un anno. La Banca nazionale austriaca lo fece cessare proprio perché funzionava troppo bene e minacciava il monopolio bancario. Il Chiemgauer (Germania, 2003) segue lo stesso principio e circola con successo da oltre 20 anni. Aequitas applica un deperimento continuo dello 0,5 % al mese, solo dopo tre mesi di inattività.',
  'x-network-consensus':'→ Rete / consenso',
  'x-node-decentralization-roadmap':'Percorso di decentralizzazione dei nodi',
  'x-open-source-chain-logic':'Logica della catena a sorgente aperta',
  'x-phase-0-now':'Fase 0 (ora):',
  'x-phase-1-100-humans':'Fase 1 (oltre 100 persone):',
  'x-phase-2-1-000-humans':'Fase 2 (oltre 1.000 persone):',
  'x-phase-3-10-000-humans':'Fase 3 (oltre 10.000 persone):',
  'x-protocol-mechanisms':'Meccanismi del protocollo',
  'x-what-happens-to-aeq-when':'Che ne è degli AEQ quando una persona muore o diventa permanentemente incapace? In Bitcoin e nella maggior parte delle criptovalute un portafoglio perduto significa offerta perduta per sempre — si stima che milioni di BTC siano irraggiungibili in via definitiva. Aequitas lo risolve con un recupero per inattività a più stadi: se un portafoglio non mostra attività per un lungo periodo, il suo saldo torna gradualmente alla comunità attraverso il fondo del reddito di base, così che l’offerta realmente in circolazione conservi un senso.',
  'x-what-if-someone-is-hospitalized':'E se qualcuno è ricoverato, detenuto o non può accedere al proprio dispositivo per mesi? La persona di fiducia — un altro essere umano verificato — può confermare che la titolare è ancora in vita, impedendo che i suoi AEQ finiscano in deposito. Questa persona non ha alcun accesso finanziario: può chiamare una sola funzione, che azzera l’orologio dell’inattività. In nessun caso può spostare, spendere o consultare fondi.',
  'bv-bind':'🔗 Genera la firma di collegamento',
  'bv-check-d':'La seconda chiamata elenca ogni verificatore e li confronta: se tutti hanno lo stesso numero di registrazioni, se a qualcuno manca un seme e se le chiavi coincidono. Se la tua voce mostra uno scarto, è meglio scoprirlo qui che durante la registrazione di qualcuno.',
  'bv-check-t':'Verificare che funzioni',
  'bv-desc':'Un nodo che produce blocchi mette al sicuro il <strong style="color:var(--text)">registro</strong>. Un verificatore biometrico mette al sicuro altro: la promessa che <strong style="color:var(--neon)">ogni persona si registri una sola volta</strong>. Sono ruoli distinti: puoi svolgerne uno, o entrambi sulla stessa macchina.',
  'bv-guide-sub':'Passo dopo passo &middot; Nessuna conoscenza di crittografia richiesta &middot; Circa 30 minuti, per lo più di download',
  'bv-honest-d':'Questa parte è in beta e i limiti sono reali. Il confronto congiunto consuma materiale crittografico monouso, e una fornitura copre per ora poche decine di registrazioni prima che ne serva altro: la via riservata si dimostra prima su piccola scala, non su milioni. Il lavoro cresce inoltre con il numero di persone iscritte. Pubblichiamo queste cifre invece di arrotondarle: un sistema che chiede il tuo volto non ha alcun diritto di restare vago su ciò che sa fare e ciò che ancora non sa.',
  'bv-honest-t':'A che punto siamo oggi — senza giri di parole',
  'bv-need-1':'<strong style="color:var(--text)">Un account Aequitas registrato.</strong> Stessa regola della produzione di blocchi, e per lo stesso motivo: una persona, una chiave. Senza, una sola persona potrebbe diventare in silenzio un comitato intero.',
  'bv-need-2':'<strong style="color:var(--text)">Un piccolo server Linux con Docker.</strong> Bastano 2 GB di memoria. Nessuna scheda grafica: il confronto è aritmetica su 64 byte. La macchina che già ospita il tuo nodo va bene.',
  'bv-need-3':'<strong style="color:var(--text)">Un dominio con HTTPS.</strong> Gli altri membri del comitato devono poterti raggiungere. Basta un sottodominio di qualcosa che possiedi già.',
  'bv-need-4':'<strong style="color:var(--text)">Restare raggiungibile.</strong> Ogni membro di un comitato deve rispondere perché una registrazione si concluda. Un verificatore spesso assente rallenta le persone invece di proteggerle.',
  'bv-need-t':'Prima di iniziare — che cosa serve',
  'bv-s1-note':'Tieni la metà privata sul tuo server e da nessun’altra parte. La metà pubblica è fatta per essere condivisa: è così che gli altri verificano che hai attestato qualcosa. <strong style="color:var(--text)">Il tuo seme di proiezione conta:</strong> poiché ogni verificatore ne usa uno diverso, un database rubato a uno non può essere confrontato con quello di un altro. Se perdi il seme, le tue quote memorizzate perdono senso: conservane una copia in un luogo che controlli.',
  'bv-s1-t':'Passo 1 — Genera le tue chiavi',
  'bv-s1-warn-d':'Due verificatori con lo stesso segreto contano come uno, e il comitato sarebbe più piccolo di quanto sembri. Nessuno — noi compresi — dovrebbe mai inviarti una chiave.',
  'bv-s1-warn-t':'Generale tu stesso. Non accettare mai chiavi da nessuno.',
  'bv-s2-d':'Metti i valori del passo 1 in un file leggibile solo da te. Un valore per riga, senza virgolette.',
  'bv-s2-note':'<strong style="color:var(--gold)">Lascia ALLOW_REAL_BIOMETRIC_DATA su false</strong> finché non hai letto le note sulla protezione dei dati. Così il tuo verificatore entra in rete e partecipa alle registrazioni di prova senza mai conservare dati di una persona reale. È il modo giusto di cominciare, e non c’è fretta di cambiarlo.',
  'bv-s2-t':'Passo 2 — Scrivi il file di configurazione',
  'bv-s3-note':'Una risposta sana riporta <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> e <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. La prima è l’affermazione che nessun modello completo viene conservato, in una forma che puoi verificare tu stesso invece di crederci. Controllala ora e di nuovo più avanti: è una garanzia tua quanto degli altri.',
  'bv-s3-t':'Passo 3 — Avvia il verificatore',
  'bv-s4-d':'Gli altri membri del comitato ti raggiungono dalla rete pubblica, quindi la porta non deve restare esposta in chiaro. Caddy ottiene il certificato da solo.',
  'bv-s4-t':'Passo 4 — Metti HTTPS davanti',
  'bv-s5-d':'I produttori di blocchi legano la loro chiave a un portafoglio umano registrato: il portafoglio firma <strong style="color:var(--text)">Aequitas: authorize validator &lt;indirizzo&gt;</strong> e senza questo la catena rifiuta il posto. Il pulsante qui sotto produce esattamente quella firma — per il ruolo di validatore. <strong style="color:var(--text)">Una chiave di verificatore non ha ancora questo legame.</strong> La sua metà pubblica viene raccolta fuori catena (passo 6) e aggiunta all\'elenco che ogni proof server controlla. Nulla sulla catena la lega a una persona. Finché manca, un comitato conta macchine e non persone, e un operatore potrebbe averne diverse. Preferiamo dirlo qui piuttosto che far sembrare il numero più forte di quanto sia.',
  'bv-s5-t':'Passo 5 — Cosa lega una chiave a una persona (e cosa non ancora)',
  'bv-s6-d':'Invia al gruppo la metà <strong style="color:var(--text)">pubblica</strong> del passo 1 insieme al tuo indirizzo HTTPS. Viene aggiunta all’elenco che ogni server di prova consulta, e da quel momento le tue attestazioni contano per il quorum. In questo passo nulla di segreto lascia la tua macchina: è il senso della separazione — la metà privata resta con te per sempre, e quella pubblica senza di essa non vale nulla.',
  'bv-s6-t':'Passo 6 — Pubblica la tua chiave pubblica',
  'bv-status-d':'Il codice del verificatore <strong style="color:var(--text)">non è ancora pubblico</strong>, quindi oggi non tutti possono completare i passi qui sotto. Li pubblichiamo comunque perché un progetto dovrebbe poter essere verificato prima di essere messo in funzione, non dopo. Se vuoi gestirne uno, chiedi nel gruppo Telegram indicato in home page. Aprire questo repository è ciò che trasformerà questa guida da progetto a invito, ed è la prossima cosa che vi dobbiamo.',
  'bv-status-t':'Stato: beta chiusa — da leggere prima di iniziare',
  'bv-title':'Oppure diventa verificatore biometrico — il ruolo che decentralizza l’unicità',
  'bv-what-d':'Nessun volto ti viene inviato. La tua macchina conserva una <strong style="color:var(--text)">quota additiva</strong> di un estratto di 64 byte: da sola è indistinguibile dal rumore casuale, e nessun calcolo alla tua portata ne ricava un volto. I confronti avvengono insieme agli altri membri del tuo comitato, e nessuno di voi apprende nulla oltre alla risposta — <em>duplicato: sì o no</em>. Non è una promessa sulle nostre buone intenzioni; è una proprietà dell’aritmetica.',
  'bv-what-t':'Che cosa avresti — e che cosa non vedresti mai',
  'bv-why-d':'Una registrazione viene accettata solo quando <strong style="color:var(--text)">più verificatori diversi</strong> l’hanno attestata. Una chiave rubata non basta: serve un intero comitato. E poiché <strong style="color:var(--neon)">una persona può detenere esattamente una chiave di validatore</strong>, comprare un comitato significa essere altrettante persone. Con 100 verificatori, chi ne controlla 10 ha meno di una possibilità su 1.000 di possedere un comitato completo di tre. Ogni persona che si unisce riduce quel numero. È l’unico punto in cui il numero dei partecipanti <em>è</em> la sicurezza. <strong style="color:var(--text)">Questo calcolo presuppone una persona per chiave di verificatore.</strong> Per la produzione di blocchi la catena lo impone; per le chiavi di verificatore non ancora (passo 5). Fino ad allora il numero sopra è un limite superiore della sicurezza, non una misura.',
  'bv-why-t':'Perché ogni verificatore in più rende la rete più difficile da corrompere',
  'x-0-1-split-40-30':'0,1 % · ripartizione 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 persone. Tetto patrimoniale mobile 5x &#8594; 25x. Fase di costruzione.',
  'x-0-8211-2-years':'0 &#8211; 2 anni',
  'x-0-perfect-equality':'0 = uguaglianza perfetta',
  'x-1-000-aeq-minted':'+1.000 AEQ emessi',
  'x-1-000-aeq-per-human':'1.000 AEQ a persona',
  'x-1-000-aeq-will-be':'1.000 AEQ verranno accreditati automaticamente',
  'x-10-000-8211-1m-humans':'10.000 &#8211; 1 mln di persone. Almeno 10 nodi. Del tutto decentralizzato.',
  'x-100-8211-10-000-humans':'100 &#8211; 10.000 persone. Tetto fisso 25x. Adesione libera dei nodi.',
  'x-100-maximum-concentration':'100 = concentrazione massima',
  'x-1m-humans-global-ubi-at':'Oltre 1 mln di persone. Reddito di base mondiale su larga scala. Obiettivo Gini &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Liquidità LP 30 %',
  'x-9679-treasury-10':'&#9679; Riserva 10 %',
  'x-9679-ubi-pool-20':'&#9679; Fondo reddito di base 20 %',
  'x-9679-validators-40':'&#9679; Validatori 40 %',
  'x-active-validators':'Validatori attivi',
  'x-add-aequitas-chain-to-metamask':'Aggiungi la catena Aequitas a MetaMask per vedere il saldo AEQ, inviare transazioni e interagire con il contratto V7 dal browser o dal portafoglio mobile.',
  'x-admin-keys-or-governance-votes':'chiavi di amministrazione o voti di governance',
  'x-aeq-activity':'ATTIVITÀ AEQ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'BlockDAG di Aequitas — nulla va sprecato',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Catena Aequitas (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas lo realizza matematicamente. Ogni persona verificata riceve esattamente 1.000 AEQ &#8212; miliardario o contadino di sussistenza, senza eccezioni. Quattro meccanismi di redistribuzione impediscono che la disuguaglianza si accumuli indefinitamente. Il coefficiente di Gini è registrato sulla catena in tempo reale.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — catena della prova di umanità',
  'x-android-apk-direct-download':'APK Android · download diretto',
  'x-architecture':'Architettura',
  'x-automatic-on-chain':'automatico, sulla catena',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (grafo aciclico orientato)',
  'x-blockdag-parallel-production':'BlockDAG · produzione parallela',
  'x-blockdag-proof-of-humanity':'BlockDAG + prova di umanità',
  'x-blue-score':'«punteggio blu»',
  'x-both-blocks-are-kept-ghostdag':'Entrambi i blocchi restano — GHOSTDAG integra quello concorrente e continua a contarlo nell’ordine canonico.',
  'x-canonical-winner':'vincitore canonico',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Paragonabile agli USA (0,41) o alla Francia (0,32). Nell’intervallo della maggior parte delle economie sviluppate. La redistribuzione appiattisce attivamente la curva.',
  'x-confirm-ward-is-alive':'✓ CONFERMARE CHE LA PERSONA È IN VITA',
  'x-core-technology':'Tecnologia di base',
  'x-daily-ubi-returns-to-all':'il reddito di base quotidiano torna a tutte le persone verificate',
  'x-demurrage-0-5-mo':'Moneta deperibile (0,5 %/mese)',
  'x-device-bound-zk-proof-one':'Prova ZK legata al dispositivo · una registrazione per dispositivo',
  'x-diagonal-line-perfect-equality':'diagonale = uguaglianza perfetta',
  'x-disconnect-wallet':'⊘ SCOLLEGA IL PORTAFOGLIO',
  'x-distinct-proposers-recent-blocks':'Proponenti distinti, blocchi recenti',
  'x-distribution':'📈 Distribuzione',
  'x-elliptic-curve':'Curva ellittica',
  'x-entire-distribution':'intera distribuzione',
  'x-evm-compatible':'Compatibile con EVM',
  'x-fill-ghostdag-verdict-thin-ring':'Riempimento = verdetto GHOSTDAG · anello sottile = proponente · una colonna per altezza. Passa sopra un blocco per i dettagli.',
  'x-generate-node-binding-signature':'🔗 Genera la firma di collegamento',
  'x-run-a-coordinator':'🚪 Gestisci un coordinatore',
  'co-title':'Oppure gestisci un coordinatore — la porta che ogni persona attraversa',
  'co-desc':'Il coordinatore è il punto in cui una persona arriva: emette la sfida, distribuisce l\'acquisizione ai verificatori, conta i loro voti ed emette l\'attestazione su cui la catena conia. Per molto tempo ne è esistito esattamente uno, il che significa che ogni registrazione della rete passava per una sola macchina. Non perché mancasse qualcosa, ma perché nessuno ne aveva avviato un secondo.',
  'co-status-t':'Stato: beta chiusa — la stessa avvertenza del verificatore',
  'co-status-d':'Il coordinatore vive nello stesso repository del verificatore, e quel repository <strong style="color:var(--text)">non è ancora pubblico</strong>. Perciò oggi non tutti possono completare i passaggi seguenti. Vengono pubblicati lo stesso, per la stessa ragione: un progetto deve poter essere verificato prima di essere messo in produzione, non dopo.',
  'co-power-t':'Cosa può fare un coordinatore — e cosa no',
  'co-power-d':'<strong style="color:var(--text)">Non può inventare una persona</strong>. Nessun bio_hash esiste finché più verificatori diversi non l\'hanno attestato, e il coordinatore non possiede nessuna delle loro chiavi. Ciò che può fare è legare un bio_hash <strong style="color:var(--text)">esistente</strong> a un portafoglio: uno disonesto potrebbe quindi dirottare un\'assegnazione verso un indirizzo a sua scelta. È un potere reale, cresce con ogni coordinatore aggiunto, e chi valuta se fidarsi dovrebbe conoscere la differenza.',
  'co-safe-t':'Perché un secondo coordinatore è sicuro',
  'co-safe-d':'Non lo è sempre stato. Fino ad agosto 2026 la promessa <strong style="color:var(--text)">una persona, una registrazione</strong> dipendeva da un lock Redis dentro il coordinatore — e due coordinatori indipendenti non condividono Redis: due registrazioni simultanee della stessa persona sarebbero passate entrambe. Ora <strong style="color:var(--text)">ogni verificatore controlla da sé</strong>, prima della propria scrittura, se quel volto è già iscritto. La garanzia non dipende più da alcun servizio né segreto condiviso, quindi un coordinatore può aggiungersi o venir meno senza alterarla.',
  'co-need-t':'Cosa ti serve',
  'co-need-d':'Un account Aequitas registrato — la stessa regola che vale per produrre blocchi e per verificare: una persona, una chiave. Un server con Docker e un indirizzo HTTPS pubblico, perché nessun browser consegna la fotocamera a una pagina non sicura. E due chiavi tue, che generi tu e che non lasciano mai la tua macchina: una firma le tue attestazioni, l\'altra mappa gli indirizzi dei portafogli in marcatori.',
  'co-keys-t':'Non accettare mai una chiave da nessuno — nemmeno da noi',
  'co-keys-d':'Due coordinatori con la stessa chiave di firma non sono due coordinatori: sono uno con due indirizzi, e il quorum che dovrebbe proteggere le persone sembrerebbe raggiunto senza esserlo. Genera entrambe le chiavi sulla tua macchina, con la tua casualità, e non lasciarne uscire nessuna.',
  'co-auth-t':'Autorizzare la tua chiave — senza permessi',
  'co-auth-d':'Finché la tua chiave non è autorizzata, i verificatori rifiutano tutto ciò che firma. Autorizzarla richiede due prove e l\'approvazione di nessuno: il tuo portafoglio firma che dietro questa chiave c\'è una persona registrata, e il tuo coordinatore dimostra sul proprio host che la chiave è davvero sua. La prima la produci con il pulsante qui sopra; la seconda la produce il coordinatore da solo. Fino ad agosto 2026 serviva anche un segreto condiviso da noi — e quel segreto <em>era</em> il permesso. Non c\'è più.',
  'co-pernode-t':'Il registro è per nodo, ed è voluto',
  'co-pernode-d':'Un\'autorizzazione scritta su un nodo non viaggia verso gli altri: non esiste una transazione per farlo né alcun gossip. Una lista di fiducia replicata sarebbe esattamente l\'autorità centrale senza la quale questo sistema è costruito: ogni operatore decide da sé quali attestazioni il suo nodo accetta. Il costo è che la tua autorizzazione va inviata a ogni nodo che debba riconoscerla. La firma è trasferibile: firmi una volta e la mandi ovunque; un nodo saltato continuerà semplicemente a rifiutarti.',
  'co-law-t':'Cosa vieni a sapere sugli altri — e cosa ne consegue',
  'co-law-d':'L\'acquisizione passa attraverso di te; la inoltri e non trattieni nulla. Ma sei l\'unico a possedere la corrispondenza tra indirizzo del portafoglio e marcatore per chi si registra tramite te — ed è per questo che la tua chiave dei marcatori deve restare tua: se condivisa, qualsiasi operatore potrebbe calcolare il marcatore per qualsiasi indirizzo pubblico e risalire a chi appartiene quel volto. Significa anche che diventi <strong style="color:var(--text)">titolare del trattamento</strong> per quelle persone secondo il GDPR. Non noi. Richieste di accesso, cancellazione e opposizione arrivano a te, e non è una formalità.',
  'co-limit-t':'L\'unica limitazione che ne deriva',
  'co-limit-d':'La cancellazione tramite indirizzo del portafoglio funziona solo presso il coordinatore dove l\'iscrizione è avvenuta: il tuo marcatore dipende dalla tua chiave, e un altro coordinatore ne deriva uno diverso per lo stesso indirizzo. Un «non trovato» da altrove significa quindi «non registrato qui», non «non registrato» — e la risposta lo dice. La via attraverso il proprio bio_hash, quella che appartiene alla persona stessa e non richiede alcun operatore, funziona presso ogni coordinatore, perché quell\'identificativo resta lo stesso.',
  'x-authorize-coordinator-key':'🔑 Autorizza la chiave del coordinatore',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — un ordine unico ricavato da un grafo intricato',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Coefficiente di Gini',
  'x-gini-coefficient-0-1':'Coefficiente di Gini (0–1)',
  'x-gini-index-history':'Storico dell’indice di Gini',
  'x-gini-target-scandinavian-level':'Obiettivo Gini (livello scandinavo)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'ZKP Groth16 (conoscenza zero)',
  'x-guardian-system-8212-human-failsafe':'Persona di fiducia &#8212; garanzia umana per i portafogli perduti',
  'x-hash-wallet':'Hash / portafoglio',
  'x-healthier-than-most-nations-on':'Più sano della maggior parte dei paesi del mondo. Paragonabile alla Scandinavia (0,27) e alla Germania (0,31). Tetto patrimoniale e moneta deperibile mantengono una distribuzione equa.',
  'x-higher-than-most-european-nations':'Più alto della maggior parte dei paesi europei — paragonabile al Brasile (0,53) o alla Russia. La redistribuzione del protocollo agisce con intensità elevata.',
  'x-honest-limitation':'Limite dichiarato:',
  'x-how-it-works':'Come funziona',
  'x-how-to-read-this-chart':'Come leggere questo grafico:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'persone potrebbero registrarsi',
  'x-imagine-a-world-where-every':'«Immagina un mondo in cui ogni persona sulla Terra &#8212; a prescindere da dove è nata, che lingua parla o quanto denaro avevano i suoi genitori &#8212; riceve un reddito quotidiano garantito solo perché è un essere umano. Non come carità. Come un diritto matematico, applicato da un codice che nessun governo o azienda può scavalcare.»',
  'x-inactive-escrow':'Deposito per inattività',
  'x-inactivity-timeline':'Cronologia dell’inattività',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (resistente al quantistico)',
  'x-key-protections':'Protezioni essenziali:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — l’evoluzione propria di Aequitas oltre un GHOSTDAG a K fissa',
  'x-knightdag-secured':'· protetto da KnightDAG',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Come la Scandinavia (~0,27)',
  'x-liquidity-pool-30':'Fondo di liquidità (30 %)',
  'x-loading-blocks':'Caricamento dei blocchi…',
  'x-loading-topology':'Caricamento della topologia…',
  'x-loading-transactions':'Caricamento delle transazioni…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Curva di Lorenz — distribuzione degli AEQ tra le persone',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: se dopo la registrazione il saldo AEQ risulta 0, vai in Impostazioni → Reti → elimina la catena Aequitas → riaggiungila da questo sito',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: se dopo l’aggiunta AEQ risulta 0, elimina la rete e riaggiungila con il pulsante qui sopra.',
  'x-money-exists-because-people-exist':'Il denaro esiste perché esistono le persone. Quindi ogni persona dovrebbe averne una quota uguale, per il solo fatto di essere umana.',
  'x-money-exists-because-people-exist-2':'«Il denaro esiste perché esistono le persone. Né più né meno.»',
  'x-most-unequal-currency-ever':'La valuta più diseguale di sempre',
  'x-multi-validator-network':'Rete con più validatori',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: non ancora significativo',
  'x-no-snapshots-yet-first-one':'Ancora nessuna rilevazione — la prima verrà salvata dopo la prossima distribuzione.',
  'x-no-stake-blockchain':'Blockchain senza puntata',
  'x-node-operator-guide-pdf':'📄 Guida per gestori di nodo (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET deve essere una persona registrata su Aequitas',
  'x-one-human-one-wallet-1':'Una persona = un portafoglio = 1.000 AEQ',
  'x-p2p-protocol':'Protocollo P2P',
  'x-paid-out-daily':'erogato ogni giorno',
  'x-permanent-on-chain':'Permanente · sulla catena',
  'x-phase-roadmap-8212-the-path':'Tabella di marcia per fasi &#8212; la strada verso la scala mondiale',
  'x-phase-transitions-are-automatic-8212':'I passaggi di fase sono automatici &#8212; attivati da soglie di popolazione e applicati dal contratto. Nessun voto, nessuna chiave di amministrazione.',
  'x-planned-post-beta':'Previsto (dopo la beta)',
  'x-postgresql-persistent':'PostgreSQL (persistente)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Fornisci liquidità AEQ / tUSD per guadagnare il 30 % di tutte le commissioni di scambio, distribuite ogni giorno.',
  'x-recorded-after-each-ubi-distribution':'Registrato dopo ogni distribuzione del reddito di base. Mostra come evolve l’uguaglianza mentre la rete cresce. Più basso è meglio — l’obiettivo è un Gini sotto 0,30.',
  'x-redistribution':'REDISTRIBUZIONE',
  'x-run-a-node':'⚙️ Gestisci un nodo',
  'x-run-a-verifier':'⚙️ Esegui un verificatore',
  'x-set-guardian':'🛡 DESIGNA UNA PERSONA DI FIDUCIA',
  'x-swap-fees-0-1':'Commissioni di scambio (0,1 %)',
  'x-sybil-resistance-8212-current-state':'Resistenza Sybil &#8212; lo stato attuale, con franchezza',
  'x-the-4-redistribution-mechanisms':'I quattro meccanismi di redistribuzione',
  'x-the-core-innovation':'L’idea centrale',
  'x-the-matching-threshold-has-not':'La soglia di corrispondenza non è ancora stata tarata su acquisizioni reali',
  'x-the-vision-8212-a-global':'La visione &#8212; un protocollo mondiale di reddito di base',
  'x-the-year-is-2009-satoshi':'È il 2009. Satoshi Nakamoto pubblica Bitcoin. Per la prima volta il valore può passare tra due persone senza una banca. Una vera rivoluzione. Ma quasi subito qualcosa va storto.',
  'x-this-is-not-a-0815':'Non è una blockchain qualunque, con un blocco per volta. Aequitas fa girare un vero BlockDAG, ordinato da GHOSTDAG — e dal 2026 protetto da KnightDAG, la sua evoluzione adattiva. Da questo meccanismo dipendono in ultima istanza ogni saldo, ogni erogazione e ogni tetto patrimoniale, perché esista una storia unica e condivisa.',
  'x-today-beta':'Oggi (beta)',
  'x-today-this-verifies-one-device':'Oggi questo verifica un dispositivo, non ancora una singola persona',
  'x-traditional-blockchain-wasted-work':'Blockchain tradizionale — lavoro sprecato',
  'x-treasury-10':'Riserva (10 %)',
  'x-trusted-verified-human':'persona verificata e fidata',
  'x-two-validators-produce-at-once':'Due validatori producono insieme → uno vince, l’altro viene scartato — lavoro perso, e limita la velocità con cui la rete può procedere in sicurezza.',
  'x-ubi-pool-20':'Fondo reddito di base (20 %)',
  'x-validators-pool-40':'Fondo dei validatori (40 %)',
  'x-view-source-on-github':'🐙 Vedi il codice su GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'Moltiplicatore del tetto patrimoniale — cursore di avvio',
  'x-wealth-cap-overflow':'Eccedenza oltre il tetto patrimoniale',
  'x-wealth-distribution-analysis':'Analisi della distribuzione della ricchezza',
  'x-what-happens-when-someone-is':'Che succede se qualcuno finisce in ospedale, in carcere o muore? Nella maggior parte dei sistemi cripto un portafoglio perduto è perduto per sempre. Aequitas ha un recupero per inattività su tre livelli.',
  'x-what-is-a-guardian':'Che cos’è una persona di fiducia?',
  'x-what-is-and-is-not':'Che cosa è privato e che cosa no:',
  'x-what-would-a-cryptocurrency-look':'«Come sarebbe una criptovaluta progettata fin dall’inizio per essere giusta con ogni essere umano?»',
  'x-why-a-normal-blockchain-isn':'Perché una blockchain normale non basta',
  'x-worse-than-any-country-on':'Peggio di qualsiasi paese al mondo (record del Sudafrica: 0,63). Si avvicina a Bitcoin (0,85). Il protocollo interviene al massimo — tetto e redistribuzione a piena forza.',
  'x-year-2-180d':'Anno 2 +180 g',
  'x-zk-device-key-proof':'Prova ZK della chiave del dispositivo',
  'swap-price-flat':'Nessuno scambio in questo periodo — il prezzo non si è mosso. Il grafico funziona; è il mercato a essere fermo.',
  'mpc-optin-title':'Opzionale — aiutare a rilevare registrazioni doppie (predisposto, non ancora attivo)',
  'mpc-optin-desc':'Predisposto, ma non ancora in servizio. In futuro il tuo nodo potrà aiutare a verificare che nessuno si registri due volte senza mai vedere dati biometrici: ogni parte conserva solo una quota matematica di ciascun modello — da sola è rumore — e confrontano insieme una nuova acquisizione, così nessuna singola macchina può ricostruire alcunché. Oggi questo percorso non decide nulla: il controllo dei duplicati non passa di qui e il comitato è un elenco fisso anziché estratto automaticamente.',
  'mpc-optin-note':'Il file delle quote contiene casualità monouso che solo il tuo nodo può custodire — non copiarlo mai su un\'altra macchina né inserirlo in un repository. Al momento deve arrivare dall\'operatore, ed è la dipendenza centrale che resta. Non serve una chiave nuova: il nodo si identifica con la stessa chiave di firma che usa già per i blocchi.',
  'logo-sub':'PROVA DI UMANITÀ','live':'LIVE',
  'reg-title':'🔐 Registrati come Umano Verificato',
  'reg-sub':'Unisciti alla rete Aequitas e ricevi il tuo sussidio di Reddito Universale di Base di 1.000 AEQ. Una tantum, permanente e completamente gratuito. Nessun dato personale viene mai memorizzato.',
  'app-title':'REGISTRAZIONE SOLO VIA APP ANDROID',
  'app-text':'Alla registrazione la fotocamera acquisisce il tuo volto e una breve sequenza di vitalità. Servizi di confronto indipendenti verificano che ci sia una persona viva e che quel volto non sia già registrato; devono concordare per quorum. Una prova ZK Groth16 porta poi il risultato sulla catena senza rivelare nulla di te. I tuoi <strong style="color:var(--gold)">1.000 AEQ vengono accreditati automaticamente</strong> dopo la verifica. <strong style="color:var(--gold)">Nota:</strong> la soglia di confronto non è ancora calibrata su acquisizioni reali — vedi le FAQ qui sotto.',
  's1t':'Acquisizione del volto','s1d':'L\'app registra il tuo volto e una breve sequenza di vitalità e li invia a servizi di confronto indipendenti. Questi verificano che davanti ci sia una persona viva e confrontano il volto con tutti i già registrati. Le immagini vengono scartate dopo l\'elaborazione.',
  's2t':'Generazione Prova ZK','s2d':'Una prova ZK Groth16 impegna il tuo bio_hash in commitment = keccak256(bioHash‖wallet) senza rivelarlo. Il nullifier deriva da quell\'hash, quindi lo stesso volto non può contare due volte — vedi le FAQ qui sotto.',
  's3t':'Connetti Wallet','s3d':'L\'app apre MetaMask su questa pagina · connetti il tuo wallet Ethereum · la prova è crittograficamente legata al tuo indirizzo',
  's4t':'1.000 AEQ Accreditati','s4d':'Registrazione confermata su Aequitas BlockDAG entro 1 secondo · 1.000 AEQ accreditati istantaneamente · la tua identità è registrata permanentemente come umano verificato',
  'priv-bar':'🔒 Verifica del volto per quorum · Groth16 ZKP · Immagini scartate dopo la verifica · Una registrazione per persona',
  'conn-wallet':'WALLET CONNESSO','proof-recv':'⚡ PROVA ZK RICEVUTA','proof-hint':'Connetti wallet per registrarti',
  'btn-conn':'🦊 CONNETTI METAMASK','btn-reg':'🔐 REGISTRA ON-CHAIN',
  'btn-wc':'🔗 CONNETTI WALLETCONNECT',
  'reg-log-hint':'// Apri l\'App Android Aequitas per generare la tua prova, poi torna qui...',
  'reg-details':'Dettagli Registrazione','k-network':'Rete','k-chainid':'ID Catena','k-grant':'Sussidio UBI',
  'k-fee':'Commissione Gas','free':'GRATUITO — completamente senza gas','k-limit':'Registrazioni','k-limit-v':'Una volta per persona · permanente · immutabile',
  'k-bio':'Volto','never-stored':'Le immagini vengono scartate dopo la verifica — nessun validatore possiede un modello intero',
  'k-proof':'Sistema di Prova','k-conf':'Conferma','k-conf-v':'Entro 1 secondo (1 blocco)',
  'k-sybil':'Protezione Sybil','k-sybil-v':'Un\'identità per persona · legata al volto, soglia non ancora calibrata',
  's-height':'Altezza Blocco',
  's-humans':'Umani Verificati',
  's-supply':'Offerta Totale','s-supply-sub':'Sempre = Umani × 1.000 AEQ',
  's-uptime':'Uptime',
  'k-chain':'Nome Catena','k-symbol':'Simbolo','k-btime':'Tempo Blocco',
  'k-cons':'Consenso','k-storage':'Archiviazione','k-dec':'Decimali',
  'btn-add-mm':'+ AGGIUNGI RETE AEQUITAS',
  'humans-title':'Umani Verificati su Aequitas Chain',
  'h-what':'Cos\'è un Umano Verificato?','h-what-t':'Un Umano Verificato è un indirizzo wallet per cui è dimostrato che appartiene a qualcuno il cui volto non è già registrato. Servizi di confronto indipendenti devono concordare per quorum e sulla catena arriva solo una prova ZK Groth16 — nessuna immagine e nessun modello. <strong style="color:var(--gold)">Fino al 23-08-2026 questo verificava un dispositivo e non una persona; ora non più.</strong>',
  'h-zkp':'Sistema di Prova a Conoscenza Zero','h-zkp-t':'Aequitas usa Groth16 su BN128 — stessa curva di Ethereum e Zcash. ~200 byte, ~10ms. commitment = keccak256(deviceKey‖wallet). Il nullifier è legato a questo dispositivo: perdere il telefono non crea una seconda identità su di esso, ma un altro dispositivo può comunque registrarsi separatamente. Nessun materiale della chiave viene mai rivelato o memorizzato lato server.',
  'h-sybil':'Resistenza Sybil — Stato Attuale','h-sybil-t':'Il nullifier deriva dal bio_hash del tuo volto, quindi lo stesso volto non può essere registrato due volte — nemmeno tra dispositivi diversi, cosa che una chiave di dispositivo non ha mai potuto. Ciò su cui poggia è una soglia di confronto non ancora calibrata su acquisizioni reali: la crittografia è esatta, la biometria sottostante è una misura con un tasso di errore non quantificato.',
  'h-global':'Inclusione Finanziaria Globale','h-global-t':'Nessun conto bancario, nessuna carta di credito, nessuna esperienza pregressa con le criptovalute. Basta uno smartphone Android con fotocamera. Aequitas è pensato per essere accessibile a ogni essere umano sulla Terra.',
  'h-bio-hw':'Roadmap di Verifica dell\'Identità','h-bio-hw-t':'Oggi (beta): una verifica del volto tra servizi di confronto indipendenti che devono concordare per quorum. La sua soglia non è ancora calibrata su acquisizioni reali — servono circa 1000 coppie impostore prima di citare qualsiasi numero. In programma: quella calibrazione e un controllo duplicati in cui nessun servizio possiede un modello intero.',
  'reg-humans':'Umani Registrati','h-desc':'Ogni indirizzo qui sotto appartiene a una persona il cui volto è stato confrontato da servizi indipendenti con tutte le registrazioni esistenti, dimostrato con una prova ZK e accreditato di esattamente 1.000 AEQ. Il registro è permanente, immutabile e on-chain. Cosa garantisce oggi la soglia e cosa no è nelle FAQ.',
  'no-humans':'Nessun umano registrato ancora.\n\nScarica l\'App Android Aequitas e sii il primo umano sulla chain!',
  'reg-stats':'Statistiche Registro','total-humans':'Totale Umani',
  'idx-title':'Indice Aequitas — Punteggio di Uguaglianza Economica in Tempo Reale',
  'idx-desc':'L\'Indice Aequitas misura la disuguaglianza economica tra tutti gli umani verificati in tempo reale. È derivato dal coefficiente Gini della distribuzione dei saldi on-chain. 0 = perfetta uguaglianza. 100 = massima disuguaglianza. Il protocollo attiva automaticamente i meccanismi di redistribuzione quando l\'indice sale.',
  'curr-idx':'Indice Attuale','bar-0':'0 — Perfetta Uguaglianza','bar-100':'100 — Massima Disuguaglianza','wcap-lbl':'Tetto Patrimoniale Attuale:','wcap-mult':'Moltiplicatore:','wcap-avg':'Quota equa:',
  'gini':'Coefficiente Gini','gini-desc':'0 = uguale · 1 = disuguale',
  'supply-desc':'Sempre = Umani × 1.000 AEQ',
  'phase':'Fase Protocollo','phase-desc':'Avanza automaticamente per numero di umani',
  'humans-desc':'Registrazioni verificate col volto',
  'pools-title':'Pool di Redistribuzione',
  'pools-desc':'Ogni commissione di swap, addebito di demurrage e overflow del limite di ricchezza viene automaticamente suddiviso tra quattro pool. Nessun intervento manuale — il protocollo gestisce tutta la redistribuzione solo attraverso il codice. Tutti i pool pagano quotidianamente.',
  'vel-pool':'Pool Validatori','vel-pool-desc':'40% di tutte le commissioni → operatori node che proteggono la rete',
  'liq-pool':'Pool Liquidità','liq-pool-desc':'30% di tutte le commissioni → fornitori di liquidità, proporzionale alle quote LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% di tutte le commissioni → tutti gli umani verificati equamente, ogni 24 ore',
  'treasury':'Tesoreria','treasury-desc':'10% di tutte le commissioni → sviluppo e manutenzione del protocollo',
  'phases-title':'Fasi del Protocollo',
  'demurrage-title':'Demurrage — Incentivo a Circolare',
  'demurrage-desc':'Aequitas implementa un meccanismo di demurrage ispirato alle valute complementari storiche. I saldi AEQ inattivi perdono lentamente valore per scoraggiare l\'accumulo e incentivare la partecipazione economica.',
  'dem-rate-k':'Tasso di Decadimento','dem-rate-v':'0,5% al mese (continuo, non a gradini)',
  'dem-grace-k':'Periodo di Grazia','dem-grace-v':'3 mesi di inattività prima che inizi il decadimento',
  'dem-reset-k':'Reset Timer','dem-reset-v':'Qualsiasi trasferimento, swap o azione di liquidità azzera il timer',
  'dem-dest-k':'AEQ decaduto va a','dem-dest-v':'Pool di redistribuzione (suddivisione 40/30/20/10)',
  'dem-warn-k':'Sistema di Avviso','dem-warn-v':'Avviso di 14 giorni (una volta) + promemoria di 7 giorni ripetuto ad ogni accesso',
  'story-title':'La Storia di Aequitas — Perché Esiste',
  'nodes-title':'Node Attivi — Topologia Attuale della Rete',
  'nodes-desc':'La rete Aequitas opera attualmente su più node distribuiti geograficamente (numero attuale sopra). Tutti partecipano alla produzione di blocchi, sincronizzazione dello stato e servizio API. Comunicano peer-to-peer via libp2p e sincronizzano lo stato dei blocchi via HTTP. La rete è progettata per supportare node aggiuntivi.',
  'run-node-title':'Esegui il Tuo Node — Aiuta a Proteggere la Rete',
  'run-node-desc':'Ogni persona registrata può eseguire un node Aequitas — senza stake, senza domanda, senza permesso da parte nostra. Una persona, una chiave di validatore: un node il cui NODE_OPERATOR_WALLET non è una persona registrata viene rifiutato con HTTP 403, altrimenti una sola persona potrebbe diventare l\'intero insieme dei validatori. I node partecipano alla produzione di blocchi e validano il registro umano. Gli operatori di node guadagnano una quota delle commissioni del protocollo tramite il Pool Validatori (40% di tutte le commissioni di swap, distribuite quotidianamente).',
  'bootstrap-title':'Connettere un Nuovo Node','bootstrap-desc':'Per eseguire il tuo node non configuri alcun punto di ingresso — gli indirizzi dei validatori sono integrati. Il tuo node si registra da solo e si sincronizza automaticamente con lo stato completo della chain. Imposta PRIMARY_NODE_URL solo se vuoi deliberatamente fissare un punto di ingresso specifico.',
  'tech-title':'Specifiche Tecniche','mm-config':'Configurazione MetaMask',
  'k-lang':'Lingua','k-src':'Codice Sorgente','evm-yes':'Sì — JSON-RPC /rpc · Compatibile MetaMask',
  'proto-label':'Protocollo Aequitas V7 — Documentazione Tecnica',
  'ca-title':'Indirizzi Contratto','ca-text':'Chain: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Principale): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 definisce le regole dell\'economia Aequitas e custodisce il registro che le rende applicabili: ogni nullifier mai rivendicato, ogni registrazione, il tetto patrimoniale e la formula di demurrage. È immutabile — nessuna chiave di amministrazione, nessun proxy di aggiornamento, nessun voto di governance può cambiarne una riga. A regolare un trasferimento reale è però il livello della catena: il nodo intercetta la chiamata ERC-20 prima che raggiunga l\'EVM e la applica al proprio libro mastro, ed è questo a rendere i trasferimenti immediati e senza gas. Il contratto è il regolamento e il registro; la catena è il motore che li esegue, e il suo codice è pubblico.<br><br>Il contratto BioVerifier riceve prove a conoscenza zero Groth16 generate interamente sul dispositivo Android dell\'utente. Verifica matematicamente on-chain in ~10 ms che il nullifier inviato è stato derivato correttamente da un segreto in possesso del registrante, e la catena rifiuta qualsiasi nullifier già visto — senza mai conoscere il suo nome, identità o dati biometrici. Questo esclude una seconda registrazione dalla stessa fonte di identità; se tale fonte sia una persona o un dispositivo dipende da quale modalità biometrica sia attiva. Questo è ciò che rende possibile la registrazione senza gas e senza investimenti: la prova è l\'unica cosa che lascia mai il dispositivo.<br><br>È proprio questa combinazione a essere nuova: le regole e il registro «un essere umano, una registrazione» risiedono in un contratto che nessuno — né l\'operatore, né un\'azienda, né un governo — può riscrivere, e il codice che le esegue è aperto e riproducibile da questo repository. Tutto è verificabile da chiunque. Ciò che richiede ancora fiducia è il funzionamento dei nodi stessi, e il modo onesto di ridurla sono più validatori indipendenti, non una frase più forte qui.',
  'poa-title':'1. PROVA DI VITA — Recupero Saldi Inattivi','poa-text':'<p>Cosa succede all\'AEQ quando le persone muoiono o diventano permanentemente incapaci? In Bitcoin, i portafogli persi significano fornitura persa permanentemente. Aequitas risolve questo con un sistema di recupero dell\'inattività a più fasi: se un portafoglio non mostra attività per un periodo prolungato, il suo saldo viene gradualmente restituito alla comunità attraverso il pool UBI.</p>',
  'poa-box':'Anno 0–2: Uso normale — nessuna restrizione<br>Anno 2: Avviso 1 — il Guardian può rispondere a nome<br>Anno 2+60g: Avviso 2 — urgenza crescente<br>Anno 2+120g: Avviso 3 — avviso finale<br>Anno 2+180g: AEQ spostato in ESCROW personale (ancora recuperabile)<br>Anno 4: Se ancora inattivo — ESCROW rilasciato al Pool UBI',
  'guard-title':'2. SISTEMA GUARDIAN — Protezione Umana','guard-text':'<p>E se qualcuno è ricoverato in ospedale o non riesce ad accedere al proprio dispositivo per mesi? Il sistema Guardian permette a una persona di fiducia — un altro umano verificato — di confermare che il proprietario del portafoglio è ancora vivo. Il Guardian ha accesso finanziario strettamente nullo: può solo chiamare una singola funzione che reimposta il timer di inattività. Non può spostare, spendere o accedere ai fondi in nessuna circostanza.</p>',
  'guard-box':'1 Guardian per umano · deve essere un umano verificato su Aequitas<br>Il Guardian può SOLO chiamare confirmAlive() — zero diritti di transazione<br>Il Guardian NON PUÒ spostare fondi, trasferire AEQ o accedere al portafoglio<br>Massimo 3 tutelati per Guardian · Blocco di 7 giorni all\'assegnazione · Nessuna relazione circolare',
  'dem-title':'3. DEMURRAGE — Meccanismo Anti-Accumulo',
  'dem-box':'Applicato solo sulla parte oltre la tua quota equa — un saldo pari o inferiore non decade mai<br>Tasso: 0,5%/mese dopo 3 mesi di inattività (continuo, non a gradini)<br>Il timer si azzera automaticamente con qualsiasi trasferimento, swap o azione di liquidità<br>AEQ decaduto ridistribuito ai quattro pool — mai bruciato<br>Avviso di 14 giorni mostrato una volta · 7 giorni ripetuto in ogni sessione attiva',
  'dem-text':'<p>Il demurrage è un costo di detenzione sul denaro — un tasso di interesse negativo che rende costoso accumulare e attraente la circolazione. L\'esperimento di Wörgl (Austria, 1932) usò una valuta con demurrage e ridusse la disoccupazione locale del 25% in un anno. La Banca Centrale austriaca lo chiuse proprio perché funzionava troppo bene. Il Chiemgauer (Germania, 2003) opera con lo stesso principio con successo da oltre 20 anni.</p>',
  'cap-title':'4. LIMITE DI RICCHEZZA — Applicazione dell\'Equità Matematica','cap-box':'Bootstrap: max(5,min(N,25))× saldo AEQ medio<br>1–4 umani: 5× (5.000 AEQ) · Cresce 1× per umano · 25+: 25× (25.000 AEQ) permanente<br>Si applica a TUTTI gli indirizzi tranne i 4 pool del protocollo<br>L\'eccesso di AEQ viene immediatamente ridistribuito · Nessun intervento manuale',
  'ubi-title':'5. REDDITO UNIVERSALE DI BASE — Ridistribuzione Giornaliera','ubi-box':'Fonti di reddito del Pool UBI:<br>· 20% di tutte le commissioni di swap del pool AMM AEQ↔tUSD<br>· Overflow dall\'applicazione del limite di ricchezza<br>· Addebiti di demurrage da account inattivi<br>· Escrow inattivo rilasciato dopo 4 anni<br><br>Distribuzione: Ogni 24 ore, l\'intero saldo del pool UBI viene diviso equamente tra tutti gli umani verificati registrati. Il pool si azzera e inizia immediatamente a riempirsi di nuovo dall\'attività continua del protocollo.',
  'inf-title':'6. NESSUNA INFLAZIONE ALGORITMICA — Formula di Fornitura Fissa','inf-box':'L\'UNICO evento che crea nuovo AEQ: un nuovo umano verificato si registra.<br><br>Offerta Totale = Umani Verificati × 1.000 AEQ<br><br>Questo non è una politica — è applicato dal protocollo. Nessun amministratore può coniare AEQ aggiuntivo, nessun voto di governance può modificare l\'emissione. AEQ è l\'unica criptovaluta in cui l\'offerta totale è determinata esclusivamente dal numero di esseri umani vivi verificati.',
  'phases-desc':'In Fase 0 (Bootstrap) il limite di ricchezza usa un moltiplicatore scorrevole: max(5, min(N, 25))× saldo medio. Con 1–4 umani: 5× media. Ogni nuovo umano aggiunge 1×. A 25+ umani: bloccato permanentemente a 25×. Fase 1+ mantiene 25× fisso. Tutte le transizioni sono automatiche — nessun voto, nessuna chiave admin.',
  'p0':'Bootstrap · &lt;100 umani · Limite di Ricchezza: max(5,min(N,25))× media · Scorre 5×→25× fino al 25° umano · Attualmente attivo',
  'p1':'Crescita · 100–10.000 umani · Limite di Ricchezza: 25× la quota equa = 25.000 AEQ',
  'p2':'Stabilità · 10.000–1M umani · Limite di Ricchezza: 25× la quota equa = 25.000 AEQ',
  'p3':'Maturità · 1M+ umani · Limite di Ricchezza: 25× la quota equa = 25.000 AEQ',
  'wealth-cap-explain':'Il Limite di Ricchezza in Fase 0 (Bootstrap) usa max(5, min(N, 25))× saldo AEQ medio, dove N = umani registrati. 1–4 umani: 5× media. Ogni nuovo umano aggiunge 1×. 25+ umani: bloccato permanentemente a 25×. Il limite si adatta sempre al saldo medio corrente.',
  'btn-download-app':'SCARICA L\'APP AEQUITAS',
  'swap-title':'🔄 Scambia AEQ ↔ tUSD','swap-sub':'Scambia AEQ con tUSD (un dollaro di test simulato) attraverso il pool di liquidità nativo. Una commissione dello 0,1% si applica solo agli scambi — i normali trasferimenti AEQ tra persone rimangono completamente gratuiti.',
  'swap-priv-bar':'🔒 Solo 0,1% commissione swap · Trasferimenti AEQ-AEQ gratuiti · tUSD è una valuta di test senza valore reale',
  'swap-your-aeq':'Il tuo AEQ','swap-your-tusd':'Il tuo tUSD',
  'swap-fee-est':'Commissione protocollo (0,1%)','swap-details-hdr':'Dettagli Scambio',
  'swap-out-lbl':'Ricevi (est.)','swap-impact-lbl':'Impatto sul prezzo','swap-rate-lbl':'Tasso di cambio',
  'swap-depth-lbl':'Composizione del Pool','amm-title':'x × y = k — AMM a Prodotto Costante',
  'amm-text':'Quando scambi AEQ con tUSD, la riserva AEQ cresce e quella tUSD diminuisce — il loro prodotto rimane sempre uguale a k. Scambi più grandi causano un maggiore impatto sul prezzo. La commissione dello 0,1% viene detratta prima di applicare la formula.',
  'swap-btn-go':'🔄 SCAMBIA',
  'swap-log-hint':'// Collega il wallet per scambiare...',
  'swap-no-liquidity':'Nessun tUSD ancora?','swap-faucet-desc':'Gli umani registrati possono richiedere tUSD di test una volta','swap-btn-faucet':'💧 RICHIEDI tUSD DI TEST',
  'swap-addliq-title':'Fornire Liquidità','swap-addliq-desc':'Sii il primo a depositare — il tuo rapporto imposta il prezzo iniziale.','swap-btn-addliq':'💧 AGGIUNGI LIQUIDITÀ',
  'swap-lp-title':'La tua Posizione LP','swap-lp-share':'Quota del Pool','swap-lp-withdrawable':'Prelevabile',
  'swap-lp-pct-label':'% della tua posizione','swap-lp-youget':'Riceverai','swap-btn-removeliq':'🔥 RIMUOVI LIQUIDITÀ',
  'swap-pool-title':'AEQ / tUSD — Stato del Pool',
  'swap-pool-aeq':'Riserva AEQ','swap-pool-tusd':'Riserva tUSD','swap-pool-price':'Prezzo Spot',
  'swap-fee-bps':'Commissione Swap',
  'swap-pools-addr-title':'Indirizzi Pool Tokenomics',
  'swap-validators':'Validatori (40%)','swap-lps':'Fornitori di Liquidità (30%)','swap-ubi':'Pool UBI (20%)','swap-treasury':'Tesoreria (10%)',
  'ubi-hero-title':'REDDITO UNIVERSALE DI BASE — POOL UBI',
  'ubi-hero-sub':'Accumulando — prossimo pagamento distribuito equamente a tutti gli umani verificati in:',
  'ubi-bal-lbl':'saldo attuale del pool','ubi-hero-desc':'Diviso equamente tra tutti · pagato ogni 24h · il pool si azzera dopo ogni pagamento · nessun saldo minimo richiesto',
  'ubi-how-fills':'Come si riempie il Pool UBI',
  'ubi-src-swap':'Commissioni Swap','ubi-src-swap-d':'Ogni swap AEQ↔tUSD contribuisce il 20% della sua commissione dello 0,1%. Più trading = riempimento più rapido.',
  'ubi-src-dem':'Demurrage','ubi-src-dem-d':'AEQ inattivo (3+ mesi) decade dello 0,5%/mese. Il 20% dell\'importo decaduto va all\'UBI.',
  'ubi-src-cap':'Overflow Limite di Ricchezza','ubi-src-cap-d':'I wallet che superano max(5,min(N,25))× il saldo medio hanno l\'eccesso confiscato istantaneamente. Il 20% fluisce all\'UBI.',
  'pools4-header':'Tutti e quattro i pool di redistribuzione',
  'ubi-see-above':'vedi conto alla rovescia sopra','ubi-timer-above':'⏰ conto alla rovescia mostrato sopra','pool-t-timer':'Accumula — nessun timer',
  'usp-headline':'Per la prima volta nella storia — tutti iniziano alla pari',
  'usp-sub':'Se possiedi uno smartphone Android, sei idoneo. Senza banca, senza conoscenze crypto, senza investimento.',
  'usp-c1-title':'0,00 Investimento Iniziale','usp-c1-desc':'La registrazione è completamente senza gas. Senza ETH, senza MATIC, senza carta di credito. Il protocollo paga tutte le commissioni per te.',
  'usp-c2-title':'1.000 AEQ per ogni umano','usp-c2-desc':'Miliardario o agricoltore di sussistenza — tutti ricevono esattamente 1.000 AEQ. Non di più, non di meno. Inizio uguale, garantito dalla matematica.',
  'usp-c3-title':'Accessibile a tutti','usp-c3-desc':'Nessun conto bancario, nessuna carta di credito, nessun documento, nessun hardware aggiuntivo — solo la fotocamera che il tuo telefono Android ha già.',
  'usp-c4-title':'UBI quotidiano per sempre','usp-c4-desc':'Una volta registrato, ricevi automaticamente una quota giornaliera dei pagamenti UBI — ogni giorno, senza alcuna azione richiesta.',
  'v7-intro-title':'Cos\'è AequitasV7?',
  'v7-intro-text':'AequitasV7 è il contratto intelligente centrale del protocollo Aequitas. "V7" si riferisce alla 7ª versione principale del contratto di equità. È distribuito immutabilmente su Aequitas Chain (ID 1926) e gestisce ogni aspetto: registrazione umana, verifica ZK, gestione saldi, limite di ricchezza, distribuzione UBI, commissioni swap. Nessun amministratore può aggiornarlo. I sei meccanismi formano un sistema auto-rinforzante.',
  'swap-sell-label':'Vendi','swap-receive-label':'Ricevi',
  'gini-what-title':'Cos e il Coefficiente di Gini?','gini-what-text':'Sviluppato dallo statistico italiano Corrado Gini (1912). Misura la distribuzione della ricchezza confrontando i saldi reali con una linea di base ipoteticamente perfettamente equa. Scala: 0 (tutti hanno lo stesso) a 1 (una persona ha tutto). Utilizzato da Banca Mondiale, OCSE, ONU per confrontare i paesi. Valori di riferimento: Bitcoin ≈ 0,85 · Sudafrica (record mondiale) ≈ 0,63 · USA ≈ 0,41 · Germania ≈ 0,31 · Scandinavia ≈ 0,27 · Obiettivo a lungo termine di Aequitas: Gini sotto 0,30.','gini-calc-title':'Come si calcola l indice','gini-calc-text':'Vengono raccolti tutti i saldi AEQ. La formula calcola la differenza assoluta media normalizzata per n2. Risultato 0-1 x 100 = Indice Aequitas.','gini-why-title':'Perche Gini','gini-why-text':'Il coefficiente Gini cattura la distribuzione completa in un numero verificabile.',
  'guard-title':'🛡 Sistema Guardian','guard-my-lbl':'Il mio Guardian','guard-none':'Nessuno',
  'guard-set-lbl':'Imposta / Cambia Guardian','guard-set-hint':'Deve essere un umano registrato su Aequitas · Blocco temporale di 7 giorni · Il Guardian può solo confermare la tua vitalità, non accedere ai fondi · Max 3 assistiti per Guardian',
  'guard-confirm-lbl':'Conferma in Vita (Come Guardian)','guard-confirm-hint':'Se il tuo assistito non riesce ad accedere al proprio wallet, conferma la sua vitalità per evitare che i fondi vengano trasferiti in escrow dopo 910 giorni di inattività.','guard-recover-btn':'🔓 RECUPERA DALL\'ESCROW',
  'faq-title':'❓ FAQ','faq-q1':'I miei dati biometrici sono al sicuro?','faq-a1':'Il tuo volto viene acquisito e inviato a servizi di confronto indipendenti: è l\'unico modo per verificare davvero «una persona, un account». Le immagini vengono elaborate e poi scartate; non vengono conservate. Ciò che resta è un modello matematico: cifrato e diviso in quote tra validatori gestiti separatamente, così nessun validatore ne possiede mai uno intero. Un limite onesto, detto e non nascosto: il servizio che esegue il confronto conserva comunque i modelli, perché confrontarli li richiede.',
  'faq-q1b':'La registrazione dimostra che sono una persona reale e unica?','faq-a1b':'Meglio di quanto una chiave di dispositivo abbia mai potuto, e non ancora dimostrabile come numero. Il volto viene confrontato con tutte le registrazioni esistenti da servizi indipendenti che devono concordare, quindi la stessa persona su un secondo telefono viene individuata — cosa che una chiave di dispositivo non ha mai potuto. Manca il tasso di errore: la soglia non è calibrata su acquisizioni reali, e servono circa 1000 coppie impostore.',
  'faq-q2':'Posso registrarmi con un wallet diverso in seguito?','faq-a2':'No. Una registrazione è legata in modo permanente a un indirizzo wallet. È voluto: il nullifier derivato dal tuo volto si consuma una sola volta, quindi registrarsi di nuovo con un altro wallet sarebbe una seconda identità per la stessa persona.',
  'faq-q3':'Cosa succede se perdo il telefono?','faq-a3':'I tuoi AEQ rimangono nel wallet — sono collegati alla tua chiave privata, non al telefono. Puoi comunque accedere al wallet tramite MetaMask con la frase seed. Il recupero del wallet è indipendente dalla registrazione biometrica.',
  'path-title':'Scegli il Tuo Percorso','path-human-title':'Sono un Umano','path-human-desc':'Voglio registrarmi, ricevere 1.000 AEQ e unirmi alla rete di reddito di base.','path-human-steps':'1. Scarica l\'App Android Aequitas<br>2. Sblocca con il blocco schermo del tuo dispositivo (impronta/volto/PIN)<br>3. Connetti MetaMask<br>4. Ricevi 1.000 AEQ istantaneamente',
  'path-node-title':'Sono un Operatore di Node','path-node-desc':'Voglio eseguire un node completo, partecipare alla produzione di blocchi e guadagnare dal pool validatori del 40%.','path-node-steps':'1. Registrarsi come umano (obbligatorio)<br>2. Nessun punto di ingresso da configurare — gli indirizzi dei validatori sono integrati<br>3. Distribuire su Contabo/Hetzner/qualsiasi VPS<br>4. Guadagnare giornalmente dal pool validatori',
  'path-dev-title':'Sono uno Sviluppatore','path-dev-desc':'Voglio costruire su Aequitas, integrare l\'API o contribuire al protocollo.','path-dev-steps':'1. JSON-RPC compatibile EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoint<br>4. Metriche: /metrics (Prometheus)',
  'story-flow-title':'Diagramma di Flusso Token AEQ','story-topo-title':'Topologia di Rete — Stato Attuale',
  'swap-price-title':'AEQ / tUSD — Prezzo Live','swap-price-desc':'Prezzo in tempo reale derivato dalle riserve del pool (x·y=k). Si aggiorna ogni 8 secondi con nuovi dati.','swap-price-empty':'Nessun dato del pool ancora — aggiungi liquidità per vedere il grafico dei prezzi.',
  'node-guide-lang-note':'Questa guida inline è in inglese. Un PDF tradotto nella tua lingua è disponibile tramite il pulsante sopra.',
  'k-zkp':'Sistema ZKP','k-hash':'Sistema Hash','k-sybil-prot':'Protezione Sybil',
  'soc-title':'💬 Social Media','soc-sub':'Annunci, lo stato della catena e le domande scomode &mdash; in pubblico, su entrambi.',
  'soc-x-desc':'Annunci, e cosa sta facendo davvero la catena. Formato breve.','soc-tg-desc':'Il gruppo aperto: domande, operatori di nodi e aiuto per registrarsi.',
  's-validators':'Validatori Attivi',
  'expl-heading':'Esplora blocchi',
},
tr:{
  'x-consensus-ghostdag-knightdag':'◆ Uzlaşı: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Sözleşme kodu',
  'x-demurrage-is-a-holding-cost':'Eriyen para, parayı elde tutmanın bir maliyetidir — istiflemeyi pahalı, dolaşımı çekici kılan negatif faiz. Tarihsel örneği vardır: Wörgl denemesi (Avusturya, 1932) eriyen bir para kullandı ve yerel işsizliği bir yıl içinde %25 azalttı. Avusturya Ulusal Bankası tam da fazla iyi işlediği ve bankacılık tekelini tehdit ettiği için buna son verdi. Chiemgauer (Almanya, 2003) aynı ilkeye dayanır ve 20 yılı aşkın süredir başarıyla dolaşır. Aequitas ayda %0,5 sürekli erime uygular, yalnızca üç aylık hareketsizlikten sonra.',
  'x-network-consensus':'→ Ağ / uzlaşı',
  'x-node-decentralization-roadmap':'Düğüm merkezsizleşme yol haritası',
  'x-open-source-chain-logic':'Açık kaynaklı zincir mantığı',
  'x-phase-0-now':'Aşama 0 (şimdi):',
  'x-phase-1-100-humans':'Aşama 1 (100+ kişi):',
  'x-phase-2-1-000-humans':'Aşama 2 (1.000+ kişi):',
  'x-phase-3-10-000-humans':'Aşama 3 (10.000+ kişi):',
  'x-protocol-mechanisms':'Protokol düzenekleri',
  'x-what-happens-to-aeq-when':'Biri öldüğünde ya da kalıcı olarak iş göremez hâle geldiğinde AEQ’ya ne olur? Bitcoin’de ve çoğu kripto parada kaybolan cüzdan, sonsuza dek kaybolmuş arz demektir — milyonlarca BTC’nin kalıcı olarak erişilemez olduğu tahmin ediliyor. Aequitas bunu çok aşamalı bir hareketsizlik kurtarmasıyla çözer: bir cüzdan uzun süre hareket göstermezse bakiyesi temel gelir havuzu üzerinden yavaş yavaş topluluğa döner, böylece gerçekten dolaşımdaki arz anlamını korur.',
  'x-what-if-someone-is-hospitalized':'Peki biri hastanede yatıyorsa, cezaevindeyse ya da aylarca cihazına erişemiyorsa? Güvenilen kişi düzeneği, bir başka doğrulanmış insanın cüzdan sahibinin hâlâ hayatta olduğunu doğrulamasına izin verir ve AEQ’sunun emanete geçmesini önler. Bu kişinin hiçbir mali erişimi yoktur: yalnızca hareketsizlik saatini sıfırlayan tek bir işlevi çağırabilir. Hiçbir koşulda para taşıyamaz, harcayamaz ya da paraya erişemez.',
  'bv-bind':'🔗 Bağlama imzası oluştur',
  'bv-check-d':'İkinci çağrı her doğrulayıcıyı listeler ve karşılaştırır: hepsinde aynı sayıda kayıt var mı, birinde tohum eksik mi, anahtarlar uyuşuyor mu. Girdinde bir sapma görünüyorsa, bunu birinin kaydı sırasında değil burada öğrenmek daha iyidir.',
  'bv-check-t':'Çalıştığını doğrulamak',
  'bv-desc':'Blok üreten bir düğüm <strong style="color:var(--text)">defteri</strong> güvence altına alır. Biyometrik doğrulayıcı başka bir şeyi: <strong style="color:var(--neon)">her insanın yalnızca bir kez kaydolduğu</strong> sözünü. Bunlar ayrı roller — birini ya da ikisini aynı makinede yürütebilirsin.',
  'bv-guide-sub':'Adım adım &middot; Kriptografi bilgisi gerekmez &middot; Yaklaşık 30 dakika, çoğu indirme',
  'bv-honest-d':'Bu bölüm beta aşamasında ve sınırlar gerçek. Ortak karşılaştırma tek kullanımlık kriptografik malzeme tüketir ve bir teslimat şu an daha fazlası gerekmeden birkaç düzine kaydı karşılar — yani gizli yol önce küçük ölçekte kendini kanıtlar, milyonlarda değil. İş yükü ayrıca kayıtlı kişi sayısıyla birlikte büyür. Bu sayıları yuvarlamak yerine yayımlıyoruz: yüzünü isteyen bir sistemin, neyi yapıp neyi henüz yapamadığı konusunda belirsiz kalmaya hakkı yoktur.',
  'bv-honest-t':'Bugün durum ne — açıkça',
  'bv-need-1':'<strong style="color:var(--text)">Kayıtlı bir Aequitas hesabı.</strong> Blok üretimiyle aynı kural, aynı nedenle: bir insan, bir anahtar. Bu olmadan tek bir kişi sessizce bütün bir komite olabilirdi.',
  'bv-need-2':'<strong style="color:var(--text)">Docker kurulu küçük bir Linux sunucusu.</strong> 2 GB bellek yeter. Ekran kartı gerekmez — karşılaştırma 64 bayt üzerinde aritmetiktir. Düğümünün zaten çalıştığı makine uygundur.',
  'bv-need-3':'<strong style="color:var(--text)">HTTPS’li bir alan adı.</strong> Diğer komite üyeleri sana ulaşabilmeli. Zaten sahip olduğun bir şeyin alt alan adı yeterlidir.',
  'bv-need-4':'<strong style="color:var(--text)">Çevrimiçi kalmak.</strong> Bir kaydın tamamlanması için komitenin her üyesi yanıt vermeli. Sık sık kapalı olan bir doğrulayıcı insanları korumak yerine yavaşlatır.',
  'bv-need-t':'Başlamadan önce — neye ihtiyacın var',
  'bv-s1-note':'Özel yarıyı sunucunda tut, başka hiçbir yerde. Açık yarı paylaşılmak içindir — başkaları bir şeyi onayladığını böyle doğrular. <strong style="color:var(--text)">Kendi izdüşüm tohumun önemlidir:</strong> her doğrulayıcı farklı birini kullandığından, birinden çalınan veritabanı bir başkasınınkiyle karşılaştırılamaz. Tohumu kaybedersen sakladığın paylar anlamını yitirir; onu denetlediğin bir yerde yedekle.',
  'bv-s1-t':'Adım 1 — Kendi anahtarlarını üret',
  'bv-s1-warn-d':'Aynı sırrı taşıyan iki doğrulayıcı tek sayılır ve komite göründüğünden küçük olur. Hiç kimse — biz dahil — sana asla bir anahtar göndermemeli.',
  'bv-s1-warn-t':'Onları kendin üret. Kimseden asla anahtar kabul etme.',
  'bv-s2-d':'Adım 1’deki değerleri yalnızca senin okuyabileceğin bir dosyaya koy. Satır başına bir değer, tırnaksız.',
  'bv-s2-note':'<strong style="color:var(--gold)">Veri koruma notlarını okuyana kadar ALLOW_REAL_BIOMETRIC_DATA değerini false bırak</strong>. Kapalıyken doğrulayıcın ağa katılır ve gerçek bir kişinin verisini hiç saklamadan test kayıtlarına katılır. Başlamanın doğru yolu budur ve bunu değiştirmek için acele yok.',
  'bv-s2-t':'Adım 2 — Yapılandırma dosyasını yaz',
  'bv-s3-note':'Sağlıklı bir yanıt <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> ve <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span> bildirir. Birincisi, hiçbir tam şablonun saklanmadığı iddiasıdır — inanmak yerine kendin doğrulayabileceğin bir biçimde. Şimdi bak, sonra tekrar bak: bu, başkalarınınki kadar senin de güvencendir.',
  'bv-s3-t':'Adım 3 — Doğrulayıcıyı başlat',
  'bv-s4-d':'Diğer komite üyeleri sana açık internet üzerinden ulaşır, bu yüzden bağlantı noktası şifresiz açıkta kalmamalı. Caddy sertifikayı kendisi alır.',
  'bv-s4-t':'Adım 4 — Önüne HTTPS koy',
  'bv-s5-d':'Blok üreticileri imza anahtarını kayıtlı bir insan cüzdanına bağlar: cüzdan <strong style="color:var(--text)">Aequitas: authorize validator &lt;adres&gt;</strong> imzalar, bu olmadan zincir yuvayı reddeder. Aşağıdaki düğme tam olarak bu imzayı üretir — doğrulayıcı rolü için. <strong style="color:var(--text)">Bir verifier anahtarının böyle bir bağı henüz yok.</strong> Açık yarısı zincir dışında toplanır (Adım 6) ve her proof sunucusunun denetlediği listeye eklenir. Zincirde onu bir insana bağlayan hiçbir şey yok. Bu olmadıkça bir komite makineleri sayar, insanları değil, ve bir işletmeci birden fazlasını tutabilir. Bunu burada söylemeyi, sayının olduğundan güçlü görünmesine yeğliyoruz.',
  'bv-s5-t':'Adım 5 — Bir anahtarı bir insana ne bağlar (ve ne henüz bağlamaz)',
  'bv-s6-d':'Adım 1’deki <strong style="color:var(--text)">açık</strong> yarıyı HTTPS adresinle birlikte gruba gönder. Her kanıt sunucusunun baktığı listeye eklenir ve o andan itibaren onayların yeter sayıya katılır. Bu adımda makinenden gizli hiçbir şey çıkmaz — ayrımın anlamı budur: özel yarı sonsuza dek sende kalır, açık yarı onsuz işe yaramaz.',
  'bv-s6-t':'Adım 6 — Açık anahtarını duyur',
  'bv-status-d':'Doğrulayıcının kaynak kodu <strong style="color:var(--text)">henüz açık değil</strong>, bu yüzden aşağıdaki adımları bugün herkes tamamlayamaz. Yine de yayımlıyoruz, çünkü bir tasarım devreye alınmadan önce denetlenebilmeli, sonrasında değil. Bir tane çalıştırmak istersen ana sayfadaki Telegram grubunda sor. Bu depoyu açmak, bu kılavuzu bir plandan bir davete dönüştürecek şeydir ve size borçlu olduğumuz bir sonraki adımdır.',
  'bv-status-t':'Durum: kapalı beta — başlamadan önce oku',
  'bv-title':'Ya da biyometrik doğrulayıcı ol — tekliği merkezsizleştiren rol',
  'bv-what-d':'Sana hiçbir zaman bir yüz gönderilmez. Makinen, 64 baytlık bir özetin <strong style="color:var(--text)">toplamsal payını</strong> saklar: tek başına rastgele gürültüden ayırt edilemez ve elindeki hiçbir hesap ondan bir yüz geri getirmez. Karşılaştırmalar komitenin diğer üyeleriyle birlikte yapılır ve hiçbiriniz cevaptan başka bir şey öğrenmez — <em>kopya: evet ya da hayır</em>. Bu, iyi niyetimize dair bir söz değil; aritmetiğin bir özelliğidir.',
  'bv-what-t':'Neyi tutardın — ve neyi asla görmezdin',
  'bv-why-d':'Bir kayıt ancak <strong style="color:var(--text)">birden fazla farklı doğrulayıcı</strong> onayladığında kabul edilir. Çalınan tek bir anahtar yetmez — saldırganın tüm bir komiteye ihtiyacı vardır. Ve <strong style="color:var(--neon)">bir insan tam olarak bir doğrulayıcı anahtarı tutabildiği</strong> için, bir komite satın almak o kadar insan olmak demektir. 100 doğrulayıcı varken 10’unu elinde tutan birinin üç kişilik bir komitenin tamamına sahip olma şansı 1.000’de 1’in altındadır. Katılan her kişi bu sayıyı küçültür. Katılımcı sayısının doğrudan güvenlik <em>olduğu</em> tek yer burasıdır. <strong style="color:var(--text)">Bu hesap her verifier anahtarı için bir insan varsayar.</strong> Blok üretiminde zincir bunu zorunlu kılar; verifier anahtarları için henüz değil (Adım 5). O zamana kadar yukarıdaki sayı güvenliğin üst sınırıdır, ölçümü değil.',
  'bv-why-t':'Her yeni doğrulayıcı ağı neden bozmayı zorlaştırır',
  'x-0-1-split-40-30':'%0,1 · dağılım 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 kişi. Kayan servet tavanı 5x &#8594; 25x. Temel atma aşaması.',
  'x-0-8211-2-years':'0 &#8211; 2 yıl',
  'x-0-perfect-equality':'0 = tam eşitlik',
  'x-1-000-aeq-minted':'+1.000 AEQ basıldı',
  'x-1-000-aeq-per-human':'kişi başına 1.000 AEQ',
  'x-1-000-aeq-will-be':'1.000 AEQ otomatik olarak hesabına geçer',
  'x-10-000-8211-1m-humans':'10.000 &#8211; 1 milyon kişi. En az 10 düğüm. Tümüyle merkezsiz.',
  'x-100-8211-10-000-humans':'100 &#8211; 10.000 kişi. Sabit tavan 25x. Düğümlere açık katılım.',
  'x-100-maximum-concentration':'100 = en yüksek yoğunlaşma',
  'x-1m-humans-global-ubi-at':'1 milyondan çok kişi. Büyük ölçekte küresel temel gelir. Gini hedefi &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Likidite LP %30',
  'x-9679-treasury-10':'&#9679; Hazine %10',
  'x-9679-ubi-pool-20':'&#9679; Temel gelir havuzu %20',
  'x-9679-validators-40':'&#9679; Doğrulayıcılar %40',
  'x-active-validators':'Etkin doğrulayıcılar',
  'x-add-aequitas-chain-to-metamask':'AEQ bakiyeni görmek, işlem göndermek ve V7 sözleşmesiyle tarayıcından ya da mobil cüzdanından doğrudan çalışmak için Aequitas zincirini MetaMask’e ekle.',
  'x-admin-keys-or-governance-votes':'yönetici anahtarları ya da yönetişim oyları',
  'x-aeq-activity':'AEQ HAREKETLERİ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'Aequitas BlockDAG — hiçbir şey boşa gitmez',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Aequitas zinciri (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas bunu matematiksel olarak uygular. Doğrulanan her kişi tam olarak 1.000 AEQ alır &#8212; milyarder ya da geçimlik çiftçi, istisnasız. Dört yeniden dağıtım düzeneği eşitsizliğin sınırsız birikmesini engeller. Gini katsayısı zincir üzerinde gerçek zamanlı tutulur.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — insanlık kanıtı zinciri',
  'x-android-apk-direct-download':'Android APK · doğrudan indirme',
  'x-architecture':'Mimari',
  'x-automatic-on-chain':'zincir üzerinde otomatik',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (yönlü çevrimsiz çizge)',
  'x-blockdag-parallel-production':'BlockDAG · paralel üretim',
  'x-blockdag-proof-of-humanity':'BlockDAG + insanlık kanıtı',
  'x-blue-score':'«mavi puan»',
  'x-both-blocks-are-kept-ghostdag':'İki blok da korunur — GHOSTDAG eşzamanlı olanı da katar ve kanonik sıralamada saymayı sürdürür.',
  'x-canonical-winner':'kanonik kazanan',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'ABD (0,41) ya da Fransa (0,32) düzeyinde. Gelişmiş ekonomilerin çoğunun aralığında. Yeniden dağıtım eğriyi etkin biçimde düzleştiriyor.',
  'x-confirm-ward-is-alive':'✓ KİŞİNİN HAYATTA OLDUĞUNU ONAYLA',
  'x-core-technology':'Temel teknoloji',
  'x-daily-ubi-returns-to-all':'günlük temel gelir tüm doğrulanmış kişilere geri döner',
  'x-demurrage-0-5-mo':'Eriyen para (aylık %0,5)',
  'x-device-bound-zk-proof-one':'Cihaza bağlı ZK kanıtı · cihaz başına tek kayıt',
  'x-diagonal-line-perfect-equality':'köşegen = tam eşitlik',
  'x-disconnect-wallet':'⊘ CÜZDAN BAĞLANTISINI KES',
  'x-distinct-proposers-recent-blocks':'Farklı üreticiler, son bloklar',
  'x-distribution':'📈 Dağılım',
  'x-elliptic-curve':'Eliptik eğri',
  'x-entire-distribution':'tüm dağılım',
  'x-evm-compatible':'EVM uyumlu',
  'x-fill-ghostdag-verdict-thin-ring':'Dolgu = GHOSTDAG kararı · ince halka = üretici · her yükseklik için bir sütun. Ayrıntılar için bloğun üzerine gel.',
  'x-generate-node-binding-signature':'🔗 Bağlama imzası oluştur',
  'x-run-a-coordinator':'🚪 Koordinatör çalıştır',
  'co-title':'Ya da bir koordinatör çalıştırın — her insanın geçtiği kapı',
  'co-desc':'Koordinatör, bir kişinin vardığı yerdir: meydan okumayı verir, kaydı doğrulayıcılara dağıtır, oylarını sayar ve zincirin üzerine basım yaptığı belgeyi düzenler. Uzun süre tam olarak bir tane vardı — yani ağdaki her kayıt tek bir makineden geçiyordu. Eksik bir şey olduğundan değil, kimse ikincisini çalıştırmadığından.',
  'co-status-t':'Durum: kapalı beta — doğrulayıcıyla aynı uyarı',
  'co-status-d':'Koordinatör, doğrulayıcıyla aynı depoda bulunuyor ve bu depo <strong style="color:var(--text)">henüz herkese açık değil</strong>. Bu yüzden aşağıdaki adımları bugün herkes tamamlayamaz. Yine de yayımlanıyorlar, aynı nedenle: bir tasarım devreye alınmadan önce denetlenebilmeli, sonrasında değil.',
  'co-power-t':'Bir koordinatör ne yapabilir — ve ne yapamaz',
  'co-power-d':'<strong style="color:var(--text)">Bir insan uyduramaz</strong>. Birden çok farklı doğrulayıcı tanıklık etmeden hiçbir bio_hash var olmaz ve koordinatör onların hiçbir anahtarını tutmaz. Yapabileceği şey, <strong style="color:var(--text)">mevcut</strong> bir bio_hash\'i bir cüzdana bağlamaktır — dürüst olmayan biri tahsisi seçtiği bir adrese yönlendirebilir. Bu gerçek bir yetkidir, eklenen her koordinatörle büyür ve güvenip güvenmemeyi tartan herkesin bu farkı bilmesi gerekir.',
  'co-safe-t':'İkinci bir koordinatör neden güvenli',
  'co-safe-d':'Her zaman değildi. Ağustos 2026\'ya kadar <strong style="color:var(--text)">bir insan, bir kayıt</strong> sözü koordinatörün içindeki bir Redis kilidine bağlıydı — iki bağımsız koordinatör Redis paylaşmaz, yani aynı kişinin eşzamanlı iki kaydı da geçerdi. Artık <strong style="color:var(--text)">her doğrulayıcı kendi yazımından önce kendisi denetliyor</strong>, o yüzün zaten kayıtlı olup olmadığını. Güvence artık ortak bir hizmete veya ortak bir sırra bağlı değil; bir koordinatör katılabilir ya da düşebilir, bu değişmez.',
  'co-need-t':'Neye ihtiyacınız var',
  'co-need-d':'Kayıtlı bir Aequitas hesabı — blok üretmek ve doğrulamakla aynı kural: bir insan, bir anahtar. Docker kurulu ve herkese açık HTTPS adresi olan bir sunucu, çünkü tarayıcılar güvensiz bir sayfaya kamera vermez. Ve kendi ürettiğiniz, makinenizden hiç çıkmayan iki anahtar: biri belgelerinizi imzalar, biri cüzdan adreslerini işaretlere eşler.',
  'co-keys-t':'Hiç kimseden anahtar kabul etmeyin — bizden de',
  'co-keys-d':'Aynı imza anahtarını paylaşan iki koordinatör iki koordinatör değildir; iki adresi olan bir tanedir ve insanları koruması gereken yetersayı, gerçekte sağlanmamışken sağlanmış görünür. Her iki anahtarı da kendi makinenizde, kendi rastgeleliğinizle üretin ve hiçbirini dışarı bırakmayın.',
  'co-auth-t':'Anahtarınızı yetkilendirmek — izne gerek yok',
  'co-auth-d':'Anahtarınız yetkilendirilene kadar doğrulayıcılar onun imzaladığı her şeyi reddeder. Yetkilendirme iki kanıt ister ve kimsenin onayını istemez: cüzdanınız bu anahtarın arkasında kayıtlı bir insanın durduğunu imzalar, koordinatörünüz de kendi sunucusunda anahtarın gerçekten kendisine ait olduğunu kanıtlar. Birincisini yukarıdaki düğmeyle üretirsiniz; ikincisini koordinatörünüz kendisi üretir. Ağustos 2026\'ya kadar ayrıca bizden ortak bir sır gerekiyordu — ki o sır iznin ta kendisi <em>idi</em>. Artık yok.',
  'co-pernode-t':'Kayıt defteri düğüm başınadır ve bu bilinçlidir',
  'co-pernode-d':'Bir düğüme yazılan yetkilendirme diğerlerine geçmez — bunun için ne bir işlem vardır ne de yayım. Çoğaltılan bir güven listesi, tam da bu sistemin bilerek kurmadığı merkezî otorite olurdu: her işletmeci, düğümünün kimin belgelerini kabul edeceğine kendisi karar verir. Bedeli, yetkilendirmenizin onu tanıması gereken her düğüme gönderilmesidir. İmzanın kendisi taşınabilir: bir kez imzalar, her yere gönderirsiniz; atladığınız düğüm sizi reddetmeye devam eder.',
  'co-law-t':'Başkaları hakkında ne öğrenirsiniz — ve bundan ne çıkar',
  'co-law-d':'Kayıt sizden geçer; iletirsiniz ve hiçbir şey saklamazsınız. Ama sizin üzerinizden kaydolan kişiler için cüzdan adresi ile işaret arasındaki eşleşmeyi yalnız siz tutarsınız — işaret anahtarınızın size ait kalması bu yüzden şarttır: paylaşılırsa herhangi bir işletmeci herhangi bir açık adres için işareti hesaplayıp o yüzün kime ait olduğunu bulabilir. Bu ayrıca GDPR uyarınca o kişiler için <strong style="color:var(--text)">veri sorumlusu</strong> olduğunuz anlamına gelir. Biz değil. Erişim, silme ve itiraz talepleri size ulaşır ve bu bir formalite değildir.',
  'co-limit-t':'Bundan doğan tek sınırlama',
  'co-limit-d':'Cüzdan adresiyle silme yalnızca kaydın yapıldığı koordinatörde çalışır: işaretiniz anahtarınıza bağlıdır ve başka bir koordinatör aynı adres için başka bir işaret türetir. Bu yüzden başka bir yerden gelen "bulunamadı", "kayıtlı değil" değil "burada kayıtlı değil" demektir — yanıt da bunu söyler. Kişinin kendi bio_hash\'i üzerinden giden yol, kendisine ait olan ve hiçbir işletmeciye ihtiyaç duymayan yol, her koordinatörde çalışır, çünkü o tanımlayıcı aynı kalır.',
  'x-authorize-coordinator-key':'🔑 Koordinatör anahtarını yetkilendir',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — karışık bir çizgeden tek bir geçerli sıra',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Gini katsayısı',
  'x-gini-coefficient-0-1':'Gini katsayısı (0–1)',
  'x-gini-index-history':'Gini endeksi geçmişi',
  'x-gini-target-scandinavian-level':'Gini hedefi (İskandinav düzeyi)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'Groth16 ZKP (sıfır bilgi)',
  'x-guardian-system-8212-human-failsafe':'Güvenilen kişi &#8212; kayıp cüzdanlar için insani güvence',
  'x-hash-wallet':'Özet / cüzdan',
  'x-healthier-than-most-nations-on':'Dünyadaki çoğu ülkeden daha sağlıklı. İskandinavya (0,27) ve Almanya (0,31) düzeyinde. Servet tavanı ve eriyen para adil dağılımı koruyor.',
  'x-higher-than-most-european-nations':'Çoğu Avrupa ülkesinden yüksek — Brezilya (0,53) ya da Rusya düzeyinde. Protokolün yeniden dağıtımı yüksek şiddette çalışıyor.',
  'x-honest-limitation':'Açıkça belirtilen sınır:',
  'x-how-it-works':'Nasıl çalışır',
  'x-how-to-read-this-chart':'Bu grafik nasıl okunur:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'kişi kaydolabilir',
  'x-imagine-a-world-where-every':'«Yeryüzündeki her insanın &#8212; nerede doğduğuna, hangi dili konuştuğuna ya da ailesinin ne kadar parası olduğuna bakılmaksızın &#8212; yalnızca insan olduğu için güvenceli bir günlük gelir aldığı bir dünya düşün. Sadaka olarak değil. Hiçbir hükümetin ya da şirketin geçersiz kılamayacağı bir kodun uyguladığı matematiksel bir hak olarak.»',
  'x-inactive-escrow':'Hareketsizlik emaneti',
  'x-inactivity-timeline':'Hareketsizlik zaman çizelgesi',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (kuantum sonrasına dayanıklı)',
  'x-key-protections':'Temel korumalar:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — sabit K’li GHOSTDAG’ın ötesinde Aequitas’ın kendi geliştirmesi',
  'x-knightdag-secured':'· KnightDAG korumalı',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'İskandinavya gibi (~0,27)',
  'x-liquidity-pool-30':'Likidite havuzu (%30)',
  'x-loading-blocks':'Bloklar yükleniyor…',
  'x-loading-topology':'Ağ yapısı yükleniyor…',
  'x-loading-transactions':'İşlemler yükleniyor…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Lorenz eğrisi — AEQ’nun kişiler arasındaki dağılımı',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: kayıttan sonra AEQ bakiyesi 0 görünüyorsa Ayarlar → Ağlar → Aequitas zincirini sil → bu siteden yeniden ekle',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: ekledikten sonra AEQ 0 görünüyorsa ağı sil ve yukarıdaki düğmeyle yeniden ekle.',
  'x-money-exists-because-people-exist':'Para, insanlar var olduğu için vardır. Öyleyse her insan, yalnızca insan olduğu için ondan eşit bir paya sahip olmalıdır.',
  'x-money-exists-because-people-exist-2':'«Para, insanlar var olduğu için vardır. Ne fazlası ne eksiği.»',
  'x-most-unequal-currency-ever':'Gelmiş geçmiş en eşitsiz para',
  'x-multi-validator-network':'Çok doğrulayıcılı ağ',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: henüz anlamlı değil',
  'x-no-snapshots-yet-first-one':'Henüz kayıt yok — ilki bir sonraki dağıtımdan sonra saklanacak.',
  'x-no-stake-blockchain':'Teminatsız blok zinciri',
  'x-node-operator-guide-pdf':'📄 Düğüm işletmecisi kılavuzu (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET kayıtlı bir Aequitas insanı olmalıdır',
  'x-one-human-one-wallet-1':'Bir insan = bir cüzdan = 1.000 AEQ',
  'x-p2p-protocol':'P2P protokolü',
  'x-paid-out-daily':'her gün ödenir',
  'x-permanent-on-chain':'Kalıcı · zincir üzerinde',
  'x-phase-roadmap-8212-the-path':'Aşama planı &#8212; küresel ölçeğe giden yol',
  'x-phase-transitions-are-automatic-8212':'Aşama geçişleri otomatiktir &#8212; kişi sayısı eşiklerince tetiklenir, sözleşmece uygulanır. Oylama yok, yönetici anahtarı yok.',
  'x-planned-post-beta':'Planlanan (beta sonrası)',
  'x-postgresql-persistent':'PostgreSQL (kalıcı)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'AEQ / tUSD likiditesi sağla ve her gün dağıtılan tüm takas ücretlerinin %30’unu kazan.',
  'x-recorded-after-each-ubi-distribution':'Her temel gelir dağıtımından sonra kaydedilir. Ağ büyürken eşitliğin nasıl geliştiğini gösterir. Düşük olması iyidir — hedef 0,30’un altında bir Gini.',
  'x-redistribution':'YENİDEN DAĞITIM',
  'x-run-a-node':'⚙️ Düğüm işlet',
  'x-run-a-verifier':'⚙️ Doğrulayıcı çalıştır',
  'x-set-guardian':'🛡 GÜVENİLEN KİŞİYİ BELİRLE',
  'x-swap-fees-0-1':'Takas ücretleri (%0,1)',
  'x-sybil-resistance-8212-current-state':'Sybil direnci &#8212; bugünkü durum, dürüstçe',
  'x-the-4-redistribution-mechanisms':'Dört yeniden dağıtım düzeneği',
  'x-the-core-innovation':'Temel fikir',
  'x-the-matching-threshold-has-not':'Eşleşme eşiği henüz gerçek çekimlerle ayarlanmadı',
  'x-the-vision-8212-a-global':'Tasavvur &#8212; küresel bir temel gelir protokolü',
  'x-the-year-is-2009-satoshi':'Yıl 2009. Satoshi Nakamoto Bitcoin’i yayımlar. İlk kez değer, banka olmadan iki kişi arasında geçebilir. Gerçek bir devrim. Ama neredeyse hemen bir şeyler ters gider.',
  'x-this-is-not-a-0815':'Bu, her seferinde tek blok üreten sıradan bir blok zinciri değil. Aequitas gerçek bir BlockDAG çalıştırır, GHOSTDAG ile sıralanan — ve 2026’dan beri KnightDAG ile, yani kendi uyarlanabilir geliştirmesiyle korunan. Her bakiye, her ödeme ve her servet tavanı, tek ve üzerinde uzlaşılmış bir tarih için sonunda bu düzeneğe dayanır.',
  'x-today-beta':'Bugün (beta)',
  'x-today-this-verifies-one-device':'Bugün bu bir cihazı doğruluyor, henüz tek bir insanı değil',
  'x-traditional-blockchain-wasted-work':'Geleneksel blok zinciri — boşa giden emek',
  'x-treasury-10':'Hazine (%10)',
  'x-trusted-verified-human':'doğrulanmış, güvenilir kişi',
  'x-two-validators-produce-at-once':'İki doğrulayıcı aynı anda üretir → biri kazanır, diğeri atılır — boşa giden emek, ve ağın güvenle ne kadar hızlanabileceğini sınırlar.',
  'x-ubi-pool-20':'Temel gelir havuzu (%20)',
  'x-validators-pool-40':'Doğrulayıcı havuzu (%40)',
  'x-view-source-on-github':'🐙 Kaynağı GitHub’da gör',
  'x-wealth-cap-multiplier-bootstrap-slider':'Servet tavanı çarpanı — başlangıç kaydırıcısı',
  'x-wealth-cap-overflow':'Servet tavanı taşması',
  'x-wealth-distribution-analysis':'Servet dağılımı çözümlemesi',
  'x-what-happens-when-someone-is':'Biri hastaneye kaldırılırsa, hapse girerse ya da ölürse ne olur? Çoğu kripto sisteminde kayıp cüzdan sonsuza dek kayıptır. Aequitas’ta üç katmanlı bir hareketsizlik kurtarması var.',
  'x-what-is-a-guardian':'Güvenilen kişi nedir?',
  'x-what-is-and-is-not':'Neyin özel olduğu ve neyin olmadığı:',
  'x-what-would-a-cryptocurrency-look':'«Her insana adil olacak biçimde en baştan tasarlanmış bir kripto para nasıl görünürdü?»',
  'x-why-a-normal-blockchain-isn':'Sıradan bir blok zinciri neden yetmez',
  'x-worse-than-any-country-on':'Dünyadaki her ülkeden kötü (Güney Afrika rekoru: 0,63). Bitcoin’e (0,85) yaklaşıyor. Protokol en yüksek müdahalede — tavan ve yeniden dağıtım tam güçte.',
  'x-year-2-180d':'2. yıl +180 g',
  'x-zk-device-key-proof':'Cihaz anahtarının ZK kanıtı',
  'swap-price-flat':'Bu dönemde işlem yok — fiyat hiç hareket etmedi. Grafik çalışıyor; piyasa sakin.',
  'mpc-optin-title':'İsteğe bağlı — mükerrer kayıt denetimine yardım (hazır, henüz devrede değil)',
  'mpc-optin-desc':'Hazırlandı, ancak henüz devrede değil. İleride düğümün, kimsenin biyometrik verisini hiç görmeden mükerrer kayıt olmadığını doğrulamaya yardım edebilecek: her taraf yalnızca her şablonun matematiksel bir payını tutar — tek başına gürültüdür — ve yeni bir kaydı birlikte karşılaştırırlar, böylece tek bir makine hiçbir şeyi geri oluşturamaz. Bugün bu yol hiçbir şeye karar vermiyor: mükerrer denetimi buradan geçmiyor ve komite otomatik çekilmek yerine sabit bir liste.',
  'mpc-optin-note':'Pay dosyası yalnızca senin düğümünün tutabileceği tek kullanımlık rastgelelik içerir — başka bir makineye asla kopyalama ve hiçbir yere ekleme. Şu anda operatörden gelmesi gerekiyor; kalan merkezi bağımlılık budur. Yeni bir anahtara ihtiyacın yok: düğümün, blokları imzalarken kullandığı anahtarla kendini tanıtır.',
  'logo-sub':'İNSANLIK KANITI','live':'CANLI',
  'reg-title':'🔐 Doğrulanmış İnsan Olarak Kayıt Ol',
  'reg-sub':'Aequitas ağına katıl ve 1.000 AEQ Evrensel Temel Gelir hibeni al. Tek seferlik, kalıcı ve tamamen ücretsiz. Hiçbir kişisel veri asla saklanmaz.',
  'app-title':'KAYIT YALNIZCA ANDROİD UYGULAMASI İLE',
  'app-text':'Kayıt sırasında kamera yüzünüzü ve kısa bir canlılık dizisini kaydeder. Bağımsız eşleştirme servisleri canlı bir insanın bulunduğunu ve bu yüzün henüz kayıtlı olmadığını denetler; yeter sayıyla anlaşmaları gerekir. Ardından bir Groth16 ZK kanıtı sonucu, sizinle ilgili hiçbir şeyi açığa çıkarmadan zincire taşır. <strong style="color:var(--gold)">1.000 AEQ</strong> doğrulamadan sonra otomatik olarak hesabınıza geçer. <strong style="color:var(--gold)">Not:</strong> eşleştirme eşiği henüz gerçek kayıtlarla kalibre edilmedi — aşağıdaki SSS bölümüne bakın.',
  's1t':'Yüz kaydı','s1d':'Uygulama yüzünüzü ve kısa bir canlılık dizisini kaydeder ve bağımsız eşleştirme servislerine gönderir. Bunlar karşıda canlı bir insan olduğunu denetler ve yüzü kayıtlı herkesle karşılaştırır. Görüntüler işlendikten sonra atılır.',
  's2t':'ZK Kanıtı Oluşturma','s2d':'Bir Groth16 ZK kanıtı bio_hash değerinizi commitment = keccak256(bioHash‖wallet) içine, açığa çıkarmadan bağlar. Nullifier bu özetten türetilir; aynı yüz iki kez sayılamaz — aşağıdaki SSS bölümüne bakın.',
  's3t':'Cüzdan Bağla','s3d':'Uygulama bu sayfada MetaMask\'ı açar · Ethereum cüzdanını bağla · kanıt kriptografik olarak adresine bağlanır',
  's4t':'1.000 AEQ Yatırıldı','s4d':'Kayıt 6 saniye içinde Aequitas BlockDAG\'da onaylandı · 1.000 AEQ anında yatırıldı · kimliğin kalıcı olarak doğrulanmış insan olarak kaydedildi',
  'priv-bar':'🔒 Yüz denetimi yeter sayıyla · Groth16 ZKP · Görüntüler denetimden sonra atılır · Kişi başına bir kayıt',
  'conn-wallet':'BAĞLI CÜZDAN','proof-recv':'⚡ ZK KANITI ALINDI','proof-hint':'Kayıt için cüzdan bağla',
  'btn-conn':'🦊 METAMASK BAĞLA','btn-reg':'🔐 ZİNCİRE KAYIT OL',
  'btn-wc':'🔗 WALLETCONNECT BAĞLA',
  'reg-log-hint':'// Kanıtını oluşturmak için Aequitas Android Uygulamasını aç, ardından buraya dön...',
  'reg-details':'Kayıt Detayları','k-network':'Ağ','k-chainid':'Zincir ID','k-grant':'UBI Hibesi',
  'k-fee':'Gas Ücreti','free':'ÜCRETSİZ — tamamen gas\'sız','k-limit':'Kayıtlar','k-limit-v':'Kişi başına bir kez · kalıcı · değiştirilemez',
  'k-bio':'Yüz','never-stored':'Görüntüler denetimden sonra atılır — hiçbir doğrulayıcı tam şablon tutmaz',
  'k-proof':'Kanıt Sistemi','k-conf':'Onay','k-conf-v':'1 saniye içinde (1 blok)',
  'k-sybil':'Sybil Koruması','k-sybil-v':'Kişi başına tek kimlik · yüze bağlı, eşik henüz kalibre edilmedi',
  's-height':'Blok Yüksekliği',
  's-humans':'Doğrulanmış İnsanlar',
  's-supply':'Toplam Arz','s-supply-sub':'Her zaman = İnsanlar × 1.000 AEQ',
  's-uptime':'Çalışma Süresi',
  'k-chain':'Zincir Adı','k-symbol':'Sembol','k-btime':'Blok Süresi',
  'k-cons':'Konsensüs','k-storage':'Depolama','k-dec':'Ondalık',
  'btn-add-mm':'+ AEQUITAS AĞINI EKLE',
  'humans-title':'Aequitas Zincirindeki Doğrulanmış İnsanlar',
  'h-what':'Doğrulanmış İnsan Nedir?','h-what-t':'Doğrulanmış İnsan, yüzü henüz kayıtlı olmayan birine ait olduğu kanıtlanmış bir cüzdan adresidir. Bağımsız eşleştirme servislerinin yeter sayıyla anlaşması gerekir ve zincire yalnızca bir Groth16 ZK kanıtı ulaşır — hiçbir görüntü ve hiçbir şablon. <strong style="color:var(--gold)">23.08.2026 tarihine kadar bu, bir insanı değil bir cihazı doğruluyordu; artık öyle değil.</strong>',
  'h-zkp':'Sıfır Bilgi Kanıtı Sistemi','h-zkp-t':'Aequitas, BN128 üzerinde Groth16 kullanır — Ethereum ve Zcash ile aynı eğri. ~200 bayt, ~10ms. commitment = keccak256(deviceKey‖wallet). Nullifier bu cihaza bağlıdır: telefonu kaybetmek bu cihazda ikinci bir kimlik oluşturmaz, ancak başka bir cihaz yine de ayrı olarak kayıt olabilir. Anahtar materyali sunucu tarafında asla ifşa edilmez veya saklanmaz.',
  'h-sybil':'Sybil Direnci — Mevcut Durum','h-sybil-t':'Nullifier, yüzünüzün bio_hash değerinden türetilir; aynı yüz iki kez kaydedilemez — cihazlar arasında da, ki bunu bir cihaz anahtarı hiçbir zaman yapamazdı. Dayandığı şey, gerçek kayıtlarla henüz kalibre edilmemiş bir eşleştirme eşiğidir: kriptografi kesindir, altındaki biyometri ise hata oranı ölçülmemiş bir ölçümdür.',
  'h-global':'Küresel Finansal Kapsayıcılık','h-global-t':'Banka hesabı, kredi kartı veya kripto deneyimi gerekmez. Kamerası olan bir Android telefon yeterlidir. Aequitas, Dünya üzerindeki her insan için erişilebilir olacak şekilde tasarlanmıştır.',
  'h-bio-hw':'Kimlik Doğrulama Yol Haritası','h-bio-hw-t':'Bugün (beta): yeter sayıyla anlaşması gereken bağımsız eşleştirme servisleri üzerinden bir yüz denetimi. Eşiği gerçek kayıtlarla henüz kalibre edilmedi — herhangi bir sayı verilmeden önce yaklaşık 1000 sahte çift gerekir. Planlanan: bu kalibrasyon ve hiçbir servisin tam şablon tutmadığı bir kopya denetimi.',
  'reg-humans':'Kayıtlı İnsanlar','h-desc':'Aşağıdaki her adres, yüzü bağımsız servislerce mevcut tüm kayıtlara karşı denetlenmiş, ZK kanıtıyla belgelenmiş ve tam olarak 1.000 AEQ yatırılmış bir kişiye aittir. Kayıt defteri kalıcı, değiştirilemez ve zincir üzerindedir. Eşiğin bugün neyi garanti edip etmediği SSS bölümündedir.',
  'no-humans':'Henüz kayıtlı insan yok.\n\nAequitas Android Uygulamasını indir ve zincirdeki ilk insan ol!',
  'reg-stats':'Kayıt İstatistikleri','total-humans':'Toplam İnsan',
  'idx-title':'Aequitas Endeksi — Gerçek Zamanlı Ekonomik Eşitlik Puanı',
  'idx-desc':'Aequitas Endeksi, tüm doğrulanmış insanların ekonomik eşitsizliğini gerçek zamanlı olarak ölçer. Zincir üzerindeki bakiye dağılımının <strong style="color:var(--teal)">Gini katsayısından</strong> türetilir. <strong style="color:var(--neon)">0 = mükemmel eşitlik</strong>. <strong style="color:var(--red)">100 = maksimum eşitsizlik</strong>. Bitcoin Gini ≈ 0,85 · Güney Afrika ≈ 0,63 · İskandinavya ≈ 0,27 · Aequitas hedefi: Gini 0,30\'in altında.',
  'gini-what-title':'Gini Katsayısı Nedir?',
  'gini-what-text':'İtalyan istatistikçi Corrado Gini tarafından 1912\'de geliştirilmiştir. Lorenz eğrisi ile görselleştirilen gerçek dağılımı mükemmel eşit dağılımla karşılaştırarak servet dağılımını ölçer. Ölçek: 0 (herkes aynı miktarı tutar) ile 1 (bir kişi her şeyi tutar). Dünya Bankası, OECD ve BM tarafından kullanılır.',
  'gini-calc-title':'Aequitas Endeksi Nasıl Hesaplanır?',
  'gini-calc-text':'Tüm doğrulanmış insanların AEQ bakiyeleri toplanır. Formül, tüm bakiye çiftleri arasındaki ortalama mutlak farkı, nüfus karesi (n²) ve ortalama bakiye (x̄) ile normalleştirilmiş olarak hesaplar. Sonuç 0–1 ile 100 ile çarpılır = Aequitas Endeksi.',
  'gini-why-title':'Neden Gini — Daha Basit Bir Metrik Değil?',
  'gini-why-text':'Basit bir zengin-fakir oranı kolayca manipüle edilebilir: 10.000 cüzdan düşük bir spread gösterebilir ama AEQ\'nun %90\'ı 100 elde konsantre olabilir — Gini bunu tespit eder, bir oran etmez. Katsayı, tüm doğrulanmış insanlar arasındaki tam dağılımı tek bir denetlenebilir sayıda yakalar.',
  'curr-idx':'Mevcut Endeks','bar-0':'0 — Mükemmel Eşitlik','bar-100':'100 — Maks. Eşitsizlik',
  'wcap-lbl':'Mevcut Servet Tavanı:','wcap-mult':'Çarpan:','wcap-avg':'Adil pay:',
  'gini':'Gini Katsayısı','gini-desc':'0 = eşit · 1 = eşitsiz',
  'supply-desc':'Her zaman = İnsanlar × 1.000 AEQ',
  'phase':'Protokol Aşaması','phase-desc':'İnsan sayısına göre otomatik ilerler',
  'humans-desc':'Yüzle doğrulanmış kayıtlar',
  'pools-title':'Yeniden Dağıtım Havuzları',
  'pools-desc':'Her takas ücreti, gecikme ücreti ve servet tavanı taşması otomatik olarak dört havuza bölünür. Manuel müdahale yok. Tüm havuzlar günlük ödeme yapar.',
  'vel-pool':'Doğrulayıcı Havuzu','vel-pool-desc':'Tüm ücretlerin %40\'ı → ağı güvence altına alan node operatörleri',
  'liq-pool':'Likidite Havuzu','liq-pool-desc':'Tüm ücretlerin %30\'u → LP paylarıyla orantılı likidite sağlayıcıları',
  'ubi-pool':'UBI Havuzu','ubi-pool-desc':'Tüm ücretlerin %20\'si → her 24 saatte tüm doğrulanmış insanlar eşit olarak',
  'treasury':'Hazine','treasury-desc':'Tüm ücretlerin %10\'u → protokol geliştirme ve bakımı',
  'phases-title':'Protokol Aşamaları',
  'phases-desc':'Aşama 0\'da servet tavanı bir bootstrap çarpanı kullanır: max(5, min(N, 25))× ortalama bakiye. 1–4 insanla: 5× ortalama. Her yeni insan 1× ekler. 25+ insanda: kalıcı olarak 25×\'e sabitlenir. Aşama 1+ 25×\'i sabit tutar. Tüm geçişler otomatiktir — yönetişim oyu yok, yönetici anahtarı yok.',
  'p0':'Bootstrap · &lt;100 insan · Servet Tavanı: max(5,min(N,25))× ort. · 5×→25× arası kayar · Şu anda aktif',
  'p1':'Büyüme · 100–10.000 insan · Servet Tavanı: 25× adil pay = 25.000 AEQ',
  'p2':'Kararlılık · 10.000–1M insan · Servet Tavanı: 25× adil pay = 25.000 AEQ',
  'p3':'Olgunluk · 1M+ insan · Servet Tavanı: 25× adil pay = 25.000 AEQ',
  'wealth-cap-explain':'Aşama 0\'daki (Bootstrap) Servet Tavanı max(5, min(N, 25))× ortalama AEQ bakiyesi kullanır; burada N = kayıtlı insan sayısı. 1–4 insan: 5× ortalama. Her yeni insan 1× ekler. 25+ insan: kalıcı olarak 25×. Tavan her zaman mevcut ortalama bakiyeyle ölçeklenir.',
  'demurrage-title':'Gecikme Ücreti — Dolaşım Teşviki',
  'demurrage-desc':'Aequitas, tarihi tamamlayıcı para birimlerinden ilham alan bir gecikme ücreti mekanizması uygular. Atıl AEQ bakiyeleri, biriktirmeyi caydırmak için yavaşça değer kaybeder.',
  'dem-rate-k':'Bozunma Hızı','dem-rate-v':'Ayda %0,5 (sürekli, kademeli değil)',
  'dem-grace-k':'İzin Süresi','dem-grace-v':'Bozunma başlamadan önce 3 aylık hareketsizlik',
  'dem-reset-k':'Saat Sıfırlama','dem-reset-v':'Herhangi bir transfer, takas veya likidite işlemi zamanlayıcıyı sıfırlar',
  'dem-dest-k':'Bozunan AEQ şuraya gider','dem-dest-v':'Yeniden dağıtım havuzları (40/30/20/10 bölünmesi)',
  'dem-warn-k':'Uyarı Sistemi','dem-warn-v':'14 günlük bildirim (bir kez) + her girişte 7 günlük tekrarlayan hatırlatma',
  'story-title':'Aequitas\'ın Hikayesi — Neden Var Olduğu',
  'nodes-title':'Aktif Node\'lar — Mevcut Ağ Topolojisi',
  'nodes-desc':'Aequitas ağı şu anda birden fazla coğrafi olarak dağıtılmış node üzerinde çalışıyor (güncel sayı yukarıda). Hepsi blok üretimine, durum senkronizasyonuna ve API hizmetine katılıyor. libp2p aracılığıyla eşler arası iletişim kuruyor ve HTTP aracılığıyla blok durumunu senkronize ediyorlar. Ağ ek node\'ları desteklemek üzere tasarlanmıştır.',
  'run-node-title':'Kendi Node\'unu Çalıştır — Ağı Güvence Altına Almaya Yardım Et',
  'run-node-desc':'Kayıtlı her insan bir Aequitas node\'u çalıştırabilir — teminat yok, başvuru yok, bizden izin yok. Bir insan, bir doğrulayıcı anahtarı: NODE_OPERATOR_WALLET\'ı kayıtlı bir insan olmayan bir node HTTP 403 ile reddedilir; aksi hâlde tek bir kişi sessizce tüm doğrulayıcı kümesi olabilirdi. Node\'lar blok üretimine katılır ve insan kaydını doğrular. Node operatörleri, Doğrulayıcı Havuzu aracılığıyla protokol ücretlerinden pay kazanır (tüm takas ücretlerinin %40\'ı, günlük dağıtılır).',
  'bootstrap-title':'Yeni Node Bağla','bootstrap-desc':'Yapılandırılacak bir giriş noktası yok — doğrulayıcı adresleri yerleşiktir. Node kendini otomatik kaydeder, tam zincir durumunu senkronize eder ve blok üretimine katılır. PRIMARY_NODE_URL yalnızca belirli bir giriş noktasını bilinçli olarak sabitlemek istiyorsanız gereklidir.',
  'tech-title':'Teknik Özellikler','mm-config':'MetaMask Yapılandırması',
  'k-lang':'Dil','k-src':'Kaynak Kodu','evm-yes':'Evet — JSON-RPC /rpc · MetaMask uyumlu',
  'proto-label':'Aequitas V7 Protokolü — Teknik Dokümantasyon',
  'ca-title':'Sözleşme Adresleri','ca-text':'Zincir: Aequitas Chain (Zincir ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Ana): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7, Aequitas ekonomisinin kurallarını tanımlar ve bunları uygulanabilir kılan kaydı tutar: kullanılmış her nullifier, her kayıt, servet üst sınırı ve demurrage formülü. Sözleşme değiştirilemez — hiçbir yönetici anahtarı, yükseltme proxy\'si ya da yönetişim oylaması tek bir satırını bile değiştiremez. Ancak gerçek bir transferi çözen katman zincirdir: düğüm, ERC-20 çağrısını EVM\'ye ulaşmadan yakalar ve kendi defterine işler — transferleri saniyenin altında ve gazsız yapan da budur. Sözleşme kural kitabı ve kayıttır; zincir onları çalıştıran motordur ve kaynağı açıktır.',
  'poa-title':'1. HAYAT KANITI — Hareketsiz Bakiye Kurtarma','poa-text':'<p>İnsanlar ölünce veya kalıcı olarak yetersiz hale gelince AEQ\'ya ne olur? Bitcoin\'de kaybedilen cüzdanlar, kalıcı olarak kaybedilen arz anlamına gelir. Aequitas bunu çok aşamalı bir hareketsizlik kurtarma sistemiyle çözer.</p>',
  'poa-box':'Yıl 0–2: Normal kullanım — kısıtlama yok<br>Yıl 2: Uyarı 1 — Vasi adına yanıt verebilir<br>Yıl 2+60g: Uyarı 2 — artan aciliyet<br>Yıl 2+120g: Uyarı 3 — son bildirim<br>Yıl 2+180g: AEQ kişisel EMANET\'e taşındı (hâlâ kurtarılabilir)<br>Yıl 4: Hâlâ hareketsizse — EMANET UBI Havuzuna serbest bırakıldı',
  'guard-title':'2. VASİ SİSTEMİ — İnsani Güvence','guard-text':'<p>Ya biri hastanede ya da başka bir nedenle aylarca cihazına erişemiyorsa? Vasi sistemi, güvenilen bir kişinin — başka bir doğrulanmış insanın — cüzdan sahibinin hâlâ hayatta olduğunu onaylamasına izin verir. Vasinin kesinlikle sıfır finansal erişimi vardır: yalnızca hareketsizlik zamanlayıcısını sıfırlayan tek bir işlevi çağırabilir.</p>',
  'guard-box':'İnsan başına 1 Vasi · Aequitas\'ta doğrulanmış insan olmalı<br>Vasi YALNIZCA confirmAlive() çağırabilir — sıfır işlem hakkı<br>Vasi fon taşıyamaz, AEQ transfer edemez veya cüzdana erişemez<br>Vasi başına en fazla 3 korunan · 7 günlük kilit · Döngüsel ilişkiye izin yok',
  'dem-title':'3. GECİKME ÜCRETİ — Biriktirme Karşıtı Mekanizma',
  'dem-box':'Yalnızca adil payınızın üzerindeki kısımdan alınır — bu paya eşit veya altındaki bakiye asla azalmaz<br>Hız: 3 aylık hareketsizlikten sonra ayda %0,5 (sürekli, kademeli değil)<br>Herhangi bir transfer, takas veya likidite işlemi zamanlayıcıyı otomatik olarak sıfırlar<br>Bozunan AEQ dört havuza yeniden dağıtılır — asla yakılmaz<br>14 günlük uyarı bir kez gösterilir · 7 günlük uyarı her aktif oturumda tekrarlanır',
  'dem-text':'<p>Gecikme ücreti, para üzerindeki bir tutma maliyetidir — biriktirmeyi pahalı, dolaşımı çekici kılan negatif bir faiz oranı. Wörgl Deneyi (Avusturya, 1932), gecikme ücretli bir para birimi kullandı ve bir yılda yerel işsizliği %25 azalttı.</p>',
  'cap-title':'4. SERVET TAVANI — Matematiksel Adalet Uygulaması','cap-box':'Bootstrap tavanı: max(5,min(N,25))× mevcut ortalama AEQ bakiyesi<br>1–4 insan: 5× · insan başına +1× · 25+: kalıcı 25×<br>4 protokol havuzu adresi dışındaki TÜM adresler için geçerli<br>Fazla AEQ anında yeniden dağıtılır · Manuel müdahale yok',
  'ubi-title':'5. EVRENSEL TEMEL GELİR — Günlük Yeniden Dağıtım','ubi-box':'UBI Havuzu Gelir Kaynakları:<br>· AEQ↔tUSD AMM havuzundan tüm takas ücretlerinin %20\'si<br>· Servet tavanı uygulamasından taşma<br>· Hareketsiz hesaplardan gecikme ücretleri<br>· 4 yıl sonra serbest bırakılan hareketsiz emanet<br><br>Dağıtım: Her 24 saatte bir, tüm UBI Havuzu bakiyesi tüm kayıtlı doğrulanmış insanlar arasında eşit olarak bölünür.',
  'inf-title':'6. ALGORİTMİK ENFLASYON YOK — Sabit Arz Formülü','inf-box':'Yeni AEQ yaratan TEK olay: yeni bir doğrulanmış insan kaydolur.<br><br>Toplam Arz = Doğrulanmış İnsanlar × 1.000 AEQ<br><br>Bu bir politika değil — protokol tarafından zorlanır. Hiçbir yönetici ek AEQ basamaz.',
  'btn-download-app':'AEQUİTAS UYGULAMASINI İNDİR',
  'swap-title':'🔄 AEQ ↔ tUSD Takas Et','swap-sub':'Yerel likidite havuzu üzerinden AEQ\'yu tUSD (simüle edilmiş test doları) ile takas et. %0,1 ücret yalnızca takaslar için geçerlidir — insanlar arasındaki normal AEQ transferleri tamamen ücretsiz kalır.',
  'swap-priv-bar':'🔒 Yalnızca %0,1 takas ücreti · AEQ\'dan AEQ\'ya transferler ücretsiz · tUSD gerçek değeri olmayan test para birimidir',
  'swap-your-aeq':'Senin AEQ','swap-your-tusd':'Senin tUSD',
  'swap-fee-est':'Protokol ücreti (%0,1)','swap-details-hdr':'Takas Detayları',
  'swap-out-lbl':'Alacaksın (tahmini)','swap-impact-lbl':'Fiyat etkisi','swap-rate-lbl':'Döviz kuru',
  'swap-depth-lbl':'Havuz Bileşimi','amm-title':'x × y = k — Sabit Çarpım AMM',
  'amm-text':'AEQ\'yu tUSD karşılığında takas ettiğinde, AEQ rezervi büyür ve tUSD rezervi küçülür — çarpımları her zaman k\'ya eşit kalır. Daha büyük takaslar daha fazla fiyat etkisine neden olur. %0,1 ücreti formül uygulanmadan önce düşülür.',
  'swap-btn-go':'🔄 TAKAS ET',
  'swap-log-hint':'// Takas yapmak için cüzdan bağla...',
  'swap-no-liquidity':'Henüz tUSD yok mu?','swap-faucet-desc':'Kayıtlı insanlar bir kez test tUSD talep edebilir','swap-btn-faucet':'💧 TEST tUSD TALEP ET',
  'swap-addliq-title':'Likidite Sağla','swap-addliq-desc':'İlk yatıran ol — oranın başlangıç fiyatını belirler.','swap-btn-addliq':'💧 LİKİDİTE EKLE',
  'swap-lp-title':'LP Pozisyonun','swap-lp-share':'Havuz Payı','swap-lp-withdrawable':'Çekilebilir',
  'swap-lp-pct-label':'% pozisyonun','swap-lp-youget':'Alacaksın','swap-btn-removeliq':'🔥 LİKİDİTE KALDIR',
  'swap-pool-title':'AEQ / tUSD — Havuz Durumu',
  'swap-pool-aeq':'AEQ Rezervi','swap-pool-tusd':'tUSD Rezervi','swap-pool-price':'Spot Fiyat',
  'swap-fee-bps':'Takas Ücreti',
  'swap-pools-addr-title':'Tokenomik Havuz Adresleri',
  'swap-validators':'Doğrulayıcılar (%40)','swap-lps':'Likidite Sağlayıcıları (%30)','swap-ubi':'UBI Havuzu (%20)','swap-treasury':'Hazine (%10)',
  'ubi-hero-title':'EVRENSEL TEMEL GELİR — UBI HAVUZU',
  'ubi-hero-sub':'Biriktirilmekte — bir sonraki ödeme tüm doğrulanmış insanlara eşit olarak dağıtılıyor:',
  'ubi-bal-lbl':'mevcut havuz bakiyesi','ubi-hero-desc':'Tümüne eşit bölünür · her 24 saatte ödenir · havuz sıfırlanır · minimum bakiye gerekmez',
  'ubi-how-fills':'UBI Havuzu Nasıl Dolar',
  'ubi-src-swap':'Takas Ücretleri','ubi-src-swap-d':'Her AEQ↔tUSD takası, %0,1 ücretinin %20\'sini katkıda bulunur. Daha fazla işlem = daha hızlı dolma.',
  'ubi-src-dem':'Gecikme Ücreti','ubi-src-dem-d':'Hareketsiz AEQ (3+ ay) ayda %0,5 bozunur. Bozunan miktarın %20\'si UBI\'ya gider.',
  'ubi-src-cap':'Servet Tavanı Taşması','ubi-src-cap-d':'Servet tavanını (max(5,min(N,25))× ortalama) aşan cüzdanlar anında kesilir. %20\'si UBI\'ya akar.',
  'pools4-header':'Dört yeniden dağıtım havuzunun tamamı',
  'ubi-see-above':'yukarıdaki geri sayımı gör','ubi-timer-above':'⏰ geri sayım yukarıda gösterildi','pool-t-timer':'Birikiyor — zamanlayıcı yok',
  'usp-headline':'Tarihte ilk kez — herkes eşit başlıyor',
  'usp-sub':'Android akıllı telefonun varsa katılabilirsin. Banka yok, kripto bilgisi yok, yatırım yok.',
  'usp-c1-title':'0,00 Başlangıç Yatırımı','usp-c1-desc':'Kayıt tamamen gas\'sız. ETH, MATIC veya kredi kartı gerekmez. Protokol tüm işlem maliyetlerini öder.',
  'usp-c2-title':'Her insan için 1.000 AEQ','usp-c2-desc':'Milyarder ya da geçimlik çiftçi — herkes tam olarak 1.000 AEQ alır. Fazlası değil, azı değil. Eşit başlangıç, matematiksel garanti.',
  'usp-c3-title':'Herkese erişilebilir','usp-c3-desc':'Banka hesabı yok, kredi kartı yok, kimlik belgesi yok, satın alınacak ek donanım yok — yalnızca Android telefonunuzda zaten bulunan kamera.',
  'usp-c4-title':'Sonsuza kadar günlük UBI','usp-c4-desc':'Kaydolduktan sonra, her gün otomatik olarak UBI ödemelerinden pay alırsın — her gün, hiçbir işlem gerektirmez.',
  'v7-intro-title':'AequitasV7 Nedir?',
  'v7-intro-text':'AequitasV7, Aequitas protokolünün merkezi akıllı sözleşmesidir. "V7", adalet sözleşmesinin 7. ana sürümüdür. Aequitas Chain\'de (Zincir ID 1926) değiştirilemez şekilde dağıtılmıştır ve her şeyi yönetir: insan kaydı, ZK doğrulaması, bakiye yönetimi, servet tavanı, UBI dağıtımı, takas ücretleri. Hiçbir yönetici onu güncelleyemez. Altı mekanizma kendi kendini güçlendiren bir sistem oluşturur.',
  'swap-sell-label':'Sat','swap-receive-label':'Al',
  'guard-title':'🛡 Koruyucu Sistemi','guard-my-lbl':'Koruyucum','guard-none':'Yok',
  'guard-set-lbl':'Koruyucu Belirle / Değiştir','guard-set-hint':'Kayıtlı bir Aequitas insanı olmalıdır · 7 günlük zaman kilidi · Koruyucu yalnızca canlılığınızı onaylayabilir, fonlara erişemez · Koruyucu başına maks. 3 korunan',
  'guard-confirm-lbl':'Hayatta Olduğunu Onayla (Koruyucu Olarak)','guard-confirm-hint':'Korunanınız cüzdanına erişemiyorsa, 910 günlük hareketsizlik sonrasında fonlarının emanete geçmesini önlemek için canlılığını onaylayın.','guard-recover-btn':'🔓 EMANETTEN GERİ AL',
  'faq-title':'❓ Sık Sorulan Sorular','faq-q1':'Biyometrik verilerim güvende mi?','faq-a1':'Yüzünüz kaydedilir ve bağımsız eşleştirme servislerine gönderilir — "bir insan, bir hesap" ancak böyle denetlenebilir. Görüntüler işlenir ve ardından atılır; saklanmaz. Saklanan şey matematiksel bir şablondur: şifrelenmiş ve ayrı işletilen doğrulayıcılar arasında paylara bölünmüş, böylece hiçbir doğrulayıcı hiçbir zaman tam bir şablon tutmaz. Dürüst bir sınır, gizlenmeden söylenmiş: karşılaştırmayı yürüten servis şablonları yine de tutar, çünkü karşılaştırma onları gerektirir.',
  'faq-q1b':'Kayıt, benzersiz gerçek bir insan olduğumu kanıtlıyor mu?','faq-a1b':'Bir cihaz anahtarının yapabildiğinden daha iyi, ancak henüz bir sayı olarak kanıtlanabilir değil. Yüz, anlaşmak zorunda olan bağımsız servislerce mevcut tüm kayıtlarla karşılaştırılır; böylece aynı kişi ikinci bir telefonda da yakalanır — bunu bir cihaz anahtarı hiçbir zaman yapamazdı. Belirlenmemiş olan hata oranıdır: eşik gerçek kayıtlarla kalibre edilmedi ve bunun için yaklaşık 1000 sahte çift gerekir.',
  'faq-q2':'Daha sonra farklı bir cüzdanla kayıt olabilir miyim?','faq-a2':'Hayır. Bir kayıt kalıcı olarak tek bir cüzdan adresine bağlıdır. Bu bilinçlidir: yüzünüzden türetilen nullifier yalnızca bir kez harcanır, dolayısıyla başka bir cüzdanla yeniden kayıt aynı kişi için ikinci bir kimlik olurdu.',
  'faq-q3':'Telefonumu kaybedersem ne olur?','faq-a3':'AEQ\'leriniz cüzdanınızda kalır — özel anahtarınıza bağlıdır, telefonunuza değil. MetaMask\'ı tohum ifadenizle kullanarak cüzdanınıza erişmeye devam edebilirsiniz. Cüzdan kurtarma, biyometrik kayıttan bağımsızdır.',
  'path-title':'Yolunuzu Seçin','path-human-title':'Ben bir İnsanım','path-human-desc':'Kayıt olmak, 1.000 AEQ almak ve temel gelir ağına katılmak istiyorum.','path-human-steps':'1. Aequitas Android Uygulamasını indir<br>2. Cihazının ekran kilidiyle kilidini aç (parmak izi/yüz/PIN)<br>3. MetaMask\'ı bağla<br>4. Anında 1.000 AEQ al',
  'path-node-title':'Ben bir Node Operatörüyüm','path-node-desc':'Tam bir node çalıştırmak, blok üretimine katılmak ve %40 doğrulayıcı havuzundan kazanmak istiyorum.','path-node-steps':'1. İnsan olarak kayıt ol (zorunlu)<br>2. Yapılandırılacak giriş noktası yok — doğrulayıcı adresleri yerleşiktir<br>3. Contabo/Hetzner/VPS\'de dağıt<br>4. Doğrulayıcı havuzundan günlük kazan',
  'path-dev-title':'Ben bir Geliştiriciyim','path-dev-desc':'Aequitas üzerinde inşa etmek, API\'yi entegre etmek veya protokole katkıda bulunmak istiyorum.','path-dev-steps':'1. EVM uyumlu JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* uç noktaları<br>4. Metrikler: /metrics (Prometheus)',
  'story-flow-title':'AEQ Token Akış Şeması','story-topo-title':'Ağ Topolojisi — Mevcut Durum',
  'swap-price-title':'AEQ / tUSD — Canlı Fiyat','swap-price-desc':'Havuz rezervlerinden gerçek zamanlı fiyat (x·y=k). Her 8 saniyede yeni havuz verileriyle güncellenir.','swap-price-empty':'Henüz havuz verisi yok — fiyat grafiğini görmek için likidite ekleyin.',
  'node-guide-lang-note':'Bu kılavuz İngilizce\'dir. Dilinizde çevrilmiş PDF yukarıdaki düğmeyle mevcuttur.',
  'k-zkp':'ZKP Sistemi','k-hash':'Hash Sistemi','k-sybil-prot':'Sybil Koruması',
  'soc-title':'💬 Sosyal Medya','soc-sub':'Duyurular, zincirin durumu ve zor sorular &mdash; herkese açık, her ikisinde de.',
  'soc-x-desc':'Duyurular ve zincirin gerçekte ne yaptığı. Kısa biçim.','soc-tg-desc':'Açık grup: sorular, node işletenler ve kayıt olma konusunda yardım.',
  's-validators':'Aktif Doğrulayıcılar',
  'expl-heading':'Blok Gezgini',
},
fr:{
  'x-consensus-ghostdag-knightdag':'◆ Consensus : GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Code du contrat',
  'x-demurrage-is-a-holding-cost':'La fonte monétaire est un coût de détention — un taux d’intérêt négatif qui rend la thésaurisation coûteuse et la circulation attrayante. Il existe des précédents : l’expérience de Wörgl (Autriche, 1932) utilisa une monnaie fondante et réduisit le chômage local de 25 % en un an. La Banque nationale d’Autriche y mit fin précisément parce que cela marchait trop bien et menaçait le monopole bancaire. Le Chiemgauer (Allemagne, 2003) repose sur le même principe et circule avec succès depuis plus de 20 ans. Aequitas applique une fonte continue de 0,5 % par mois, seulement après trois mois d’inactivité.',
  'x-network-consensus':'→ Réseau / consensus',
  'x-node-decentralization-roadmap':'Feuille de route de la décentralisation des nœuds',
  'x-open-source-chain-logic':'Logique de chaîne en source ouverte',
  'x-phase-0-now':'Phase 0 (maintenant) :',
  'x-phase-1-100-humans':'Phase 1 (100 personnes et plus) :',
  'x-phase-2-1-000-humans':'Phase 2 (1 000 personnes et plus) :',
  'x-phase-3-10-000-humans':'Phase 3 (10 000 personnes et plus) :',
  'x-protocol-mechanisms':'Mécanismes du protocole',
  'x-what-happens-to-aeq-when':'Qu’advient-il des AEQ lorsqu’une personne meurt ou devient définitivement incapable ? Dans Bitcoin et la plupart des cryptomonnaies, un portefeuille perdu signifie une offre perdue à jamais — on estime que des millions de BTC sont inaccessibles pour toujours. Aequitas résout cela par une récupération d’inactivité à plusieurs étages : si un portefeuille ne montre aucune activité pendant une longue période, son solde revient progressivement à la communauté via le fonds de revenu de base, afin que l’offre réellement en circulation garde un sens.',
  'x-what-if-someone-is-hospitalized':'Et si quelqu’un est hospitalisé, incarcéré ou privé de son appareil pendant des mois ? Le dispositif de personne de confiance permet à une autre personne vérifiée de confirmer que la titulaire est toujours en vie, empêchant que ses AEQ ne partent en séquestre. Cette personne n’a strictement aucun accès financier : elle ne peut appeler qu’une seule fonction, qui remet à zéro l’horloge d’inactivité. Elle ne peut en aucun cas déplacer, dépenser ni consulter des fonds.',
  'bv-bind':'🔗 Générer la signature de liaison',
  'bv-check-d':'Le second appel énumère tous les vérificateurs et les compare : s’ils détiennent le même nombre d’inscriptions, s’il manque une graine quelque part, et si les clés concordent. Si votre entrée montre un écart, mieux vaut l’apprendre ici qu’au milieu de l’inscription de quelqu’un.',
  'bv-check-t':'Vérifier que cela fonctionne',
  'bv-desc':'Un nœud qui produit des blocs sécurise le <strong style="color:var(--text)">registre</strong>. Un vérificateur biométrique sécurise autre chose : la promesse que <strong style="color:var(--neon)">chaque personne ne s’inscrit qu’une seule fois</strong>. Ce sont deux rôles distincts — vous pouvez tenir l’un, ou les deux sur la même machine.',
  'bv-guide-sub':'Pas à pas &middot; Aucune connaissance en cryptographie requise &middot; Environ 30 minutes, surtout du téléchargement',
  'bv-honest-d':'Cette partie est en bêta et les limites sont réelles. La comparaison conjointe consomme du matériel cryptographique à usage unique, et une livraison couvre pour l’instant quelques dizaines d’inscriptions avant qu’il n’en faille davantage — la voie confidentielle fait donc ses preuves à petite échelle d’abord, pas sur des millions. Le travail croît aussi avec le nombre de personnes inscrites. Nous publions ces chiffres plutôt que de les arrondir : un système qui réclame votre visage n’a pas le droit de rester vague sur ce qu’il sait faire et ce qu’il ne sait pas encore.',
  'bv-honest-t':'Où en est-on aujourd’hui — sans détour',
  'bv-need-1':'<strong style="color:var(--text)">Un compte Aequitas enregistré.</strong> Même règle que pour la production de blocs, et pour la même raison : une personne, une clé. Sans cela, une seule personne pourrait devenir discrètement un comité entier.',
  'bv-need-2':'<strong style="color:var(--text)">Un petit serveur Linux avec Docker.</strong> 2 Go de mémoire suffisent. Pas de carte graphique — la comparaison est de l’arithmétique sur 64 octets. La machine qui fait déjà tourner votre nœud convient.',
  'bv-need-3':'<strong style="color:var(--text)">Un nom de domaine avec HTTPS.</strong> Les autres membres du comité doivent pouvoir vous joindre. Un sous-domaine de quelque chose que vous possédez déjà suffit.',
  'bv-need-4':'<strong style="color:var(--text)">Rester joignable.</strong> Chaque membre d’un comité doit répondre pour qu’une inscription aboutisse. Un vérificateur souvent absent ralentit les gens au lieu de les protéger.',
  'bv-need-t':'Avant de commencer — ce qu’il vous faut',
  'bv-s1-note':'Gardez la moitié privée sur votre serveur et nulle part ailleurs. La moitié publique est faite pour être partagée — c’est ainsi que d’autres vérifient que vous avez attesté quelque chose. <strong style="color:var(--text)">Votre propre graine de projection compte :</strong> comme chaque vérificateur en utilise une différente, une base volée chez l’un ne peut pas être confrontée à celle d’un autre. Perdez la graine et vos parts stockées perdent tout sens : sauvegardez-la dans un endroit que vous maîtrisez.',
  'bv-s1-t':'Étape 1 — Générez vos propres clés',
  'bv-s1-warn-d':'Deux vérificateurs partageant le même secret comptent pour un seul, et le comité serait plus petit qu’il n’y paraît. Personne — nous y compris — ne devrait jamais vous envoyer une clé.',
  'bv-s1-warn-t':'Générez-les vous-même. N’acceptez jamais de clés de qui que ce soit.',
  'bv-s2-d':'Placez les valeurs de l’étape 1 dans un fichier que vous seul pouvez lire. Une valeur par ligne, sans guillemets.',
  'bv-s2-note':'<strong style="color:var(--gold)">Laissez ALLOW_REAL_BIOMETRIC_DATA sur false</strong> tant que vous n’avez pas lu les notes de protection des données. Ainsi, votre vérificateur rejoint le réseau et participe aux inscriptions de test sans jamais conserver les données d’une personne réelle. C’est la bonne façon de commencer, et rien ne presse pour changer cela.',
  'bv-s2-t':'Étape 2 — Écrivez le fichier de configuration',
  'bv-s3-note':'Une réponse saine indique <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> et <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. La première est l’affirmation qu’aucun gabarit complet n’est conservé, sous une forme que vous pouvez vérifier vous-même plutôt que croire sur parole. Vérifiez-la maintenant, puis de nouveau plus tard — c’est votre garantie autant que celle des autres.',
  'bv-s3-t':'Étape 3 — Démarrez le vérificateur',
  'bv-s4-d':'Les autres membres du comité vous joignent par l’internet public : le port ne doit donc pas être exposé en clair. Caddy obtient un certificat tout seul.',
  'bv-s4-t':'Étape 4 — Placez HTTPS devant',
  'bv-s5-d':'Les producteurs de blocs lient leur clé de signature à un portefeuille humain enregistré : le portefeuille signe <strong style="color:var(--text)">Aequitas: authorize validator &lt;adresse&gt;</strong>, et sans cela la chaîne refuse la place. Le bouton ci-dessous produit exactement cette signature — pour le rôle de validateur. <strong style="color:var(--text)">Une clé de vérificateur n’a pas encore ce lien.</strong> Sa moitié publique est collectée hors chaîne (étape 6) et ajoutée à la liste que chaque serveur de preuve vérifie. Rien sur la chaîne ne la rattache à une personne. Tant que cela manque, un comité compte des machines et non des personnes, et un opérateur pourrait en tenir plusieurs. Nous préférons le dire ici plutôt que laisser le chiffre paraître plus solide qu’il ne l’est.',
  'bv-s5-t':'Étape 5 — Ce qui lie une clé à une personne (et ce qui ne le fait pas encore)',
  'bv-s6-d':'Envoyez au groupe la moitié <strong style="color:var(--text)">publique</strong> de l’étape 1, avec votre adresse HTTPS. Elle est ajoutée à la liste que chaque serveur de preuve consulte, et dès lors vos attestations comptent pour le quorum. Rien de secret ne quitte votre machine à cette étape — c’est tout l’intérêt de la séparation : la moitié privée reste chez vous pour toujours, et la moitié publique ne vaut rien sans elle.',
  'bv-s6-t':'Étape 6 — Publiez votre clé publique',
  'bv-status-d':'Le code du vérificateur <strong style="color:var(--text)">n’est pas encore public</strong>, les étapes ci-dessous ne sont donc pas réalisables par tout le monde aujourd’hui. Elles sont publiées maintenant parce qu’une conception doit pouvoir être vérifiée avant d’être déployée, pas après. Si vous souhaitez en faire tourner un, demandez dans le groupe Telegram indiqué en page d’accueil. Ouvrir ce dépôt est ce qui transformera ce guide d’un projet en une invitation, et c’est la prochaine chose que nous vous devons.',
  'bv-status-t':'État : bêta fermée — à lire avant de commencer',
  'bv-title':'Ou devenez vérificateur biométrique — le rôle qui décentralise l’unicité',
  'bv-what-d':'Aucun visage ne vous est envoyé. Votre machine conserve une <strong style="color:var(--text)">part additive</strong> d’une empreinte de 64 octets : seule, elle est indiscernable d’un bruit aléatoire, et aucun calcul à votre portée n’en fait ressortir un visage. Les comparaisons se font conjointement avec les autres membres de votre comité, et aucun de vous n’apprend rien d’autre que la réponse — <em>doublon : oui ou non</em>. Ce n’est pas une promesse sur nos bonnes intentions ; c’est une propriété du calcul.',
  'bv-what-t':'Ce que vous détiendriez — et ce que vous ne verriez jamais',
  'bv-why-d':'Une inscription n’est acceptée qu’une fois attestée par <strong style="color:var(--text)">plusieurs vérificateurs différents</strong>. Une clé volée ne suffit donc pas — il faut tout un comité. Et comme <strong style="color:var(--neon)">une personne ne peut détenir qu’une seule clé de validateur</strong>, acheter un comité revient à être autant de personnes. Avec 100 vérificateurs, quelqu’un qui en contrôle 10 a moins d’une chance sur 1 000 de posséder un comité entier de trois. Chaque personne qui rejoint réduit ce nombre. C’est le seul endroit où le nombre de participants <em>est</em> la sécurité. <strong style="color:var(--text)">Ce calcul suppose une personne par clé de vérificateur.</strong> Pour la production de blocs, la chaîne l’impose ; pour les clés de vérificateur, pas encore (étape 5). D’ici là, le chiffre ci-dessus est une borne supérieure de la sécurité, pas une mesure.',
  'bv-why-t':'Pourquoi chaque vérificateur supplémentaire rend le réseau plus difficile à corrompre',
  'x-0-1-split-40-30':'0,1 % · répartition 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 personnes. Plafond de richesse glissant 5x &#8594; 25x. Phase de fondation.',
  'x-0-8211-2-years':'0 &#8211; 2 ans',
  'x-0-perfect-equality':'0 = égalité parfaite',
  'x-1-000-aeq-minted':'+1 000 AEQ émis',
  'x-1-000-aeq-per-human':'1 000 AEQ par personne',
  'x-1-000-aeq-will-be':'1 000 AEQ seront crédités automatiquement',
  'x-10-000-8211-1m-humans':'10 000 &#8211; 1 M de personnes. 10 nœuds minimum. Entièrement décentralisé.',
  'x-100-8211-10-000-humans':'100 &#8211; 10 000 personnes. Plafond fixe 25x. Adhésion libre des nœuds.',
  'x-100-maximum-concentration':'100 = concentration maximale',
  'x-1m-humans-global-ubi-at':'Plus d’1 M de personnes. Revenu de base mondial à grande échelle. Objectif Gini &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Liquidité LP 30 %',
  'x-9679-treasury-10':'&#9679; Trésorerie 10 %',
  'x-9679-ubi-pool-20':'&#9679; Fonds de revenu de base 20 %',
  'x-9679-validators-40':'&#9679; Validateurs 40 %',
  'x-active-validators':'Validateurs actifs',
  'x-add-aequitas-chain-to-metamask':'Ajoutez la chaîne Aequitas à MetaMask pour voir votre solde AEQ, envoyer des transactions et interagir avec le contrat V7 depuis votre navigateur ou votre portefeuille mobile.',
  'x-admin-keys-or-governance-votes':'clés d’administration ou votes de gouvernance',
  'x-aeq-activity':'ACTIVITÉ AEQ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'BlockDAG Aequitas — rien n’est gaspillé',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Chaîne Aequitas (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas met cela en œuvre mathématiquement. Chaque personne vérifiée reçoit exactement 1 000 AEQ &#8212; milliardaire ou paysan de subsistance, sans exception. Quatre mécanismes de redistribution empêchent l’inégalité de s’accumuler indéfiniment. Le coefficient de Gini est suivi en temps réel sur la chaîne.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — chaîne de preuve d’humanité',
  'x-android-apk-direct-download':'APK Android · téléchargement direct',
  'x-architecture':'Architecture',
  'x-automatic-on-chain':'automatique, sur la chaîne',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (graphe orienté acyclique)',
  'x-blockdag-parallel-production':'BlockDAG · production parallèle',
  'x-blockdag-proof-of-humanity':'BlockDAG + preuve d’humanité',
  'x-blue-score':'« score bleu »',
  'x-both-blocks-are-kept-ghostdag':'Les deux blocs sont conservés — GHOSTDAG intègre le bloc concurrent et le compte dans l’ordre canonique.',
  'x-canonical-winner':'gagnant canonique',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Comparable aux États-Unis (0,41) ou à la France (0,32). Dans la fourchette de la plupart des économies développées. La redistribution aplatit activement la courbe.',
  'x-confirm-ward-is-alive':'✓ CONFIRMER QUE LA PERSONNE EST EN VIE',
  'x-core-technology':'Technologie centrale',
  'x-daily-ubi-returns-to-all':'le revenu de base quotidien revient à toutes les personnes vérifiées',
  'x-demurrage-0-5-mo':'Fonte monétaire (0,5 %/mois)',
  'x-device-bound-zk-proof-one':'Preuve ZK liée à l’appareil · une inscription par appareil',
  'x-diagonal-line-perfect-equality':'diagonale = égalité parfaite',
  'x-disconnect-wallet':'⊘ DÉCONNECTER LE PORTEFEUILLE',
  'x-distinct-proposers-recent-blocks':'Producteurs distincts, blocs récents',
  'x-distribution':'📈 Répartition',
  'x-elliptic-curve':'Courbe elliptique',
  'x-entire-distribution':'répartition entière',
  'x-evm-compatible':'Compatible EVM',
  'x-fill-ghostdag-verdict-thin-ring':'Remplissage = verdict GHOSTDAG · anneau fin = producteur · une colonne par hauteur. Survolez un bloc pour les détails.',
  'x-generate-node-binding-signature':'🔗 Générer la signature de liaison',
  'x-run-a-coordinator':'🚪 Exploiter un coordinateur',
  'co-title':'Ou exploitez un coordinateur — la porte que franchit chaque personne',
  'co-desc':'Le coordinateur est l\'endroit où une personne arrive : il émet le défi, répartit la capture entre les vérificateurs, compte leurs voix et délivre l\'attestation sur laquelle la chaîne émet. Pendant longtemps il n\'en existait qu\'un seul — donc chaque inscription du réseau passait par une seule machine. Non parce qu\'il manquait quelque chose, mais parce que personne n\'en avait lancé un deuxième.',
  'co-status-t':'Statut : bêta fermée — la même réserve que pour le vérificateur',
  'co-status-d':'Le coordinateur se trouve dans le même dépôt que le vérificateur, et ce dépôt <strong style="color:var(--text)">n\'est pas encore public</strong>. Tout le monde ne peut donc pas accomplir aujourd\'hui les étapes ci-dessous. Elles sont publiées quand même, pour la même raison : une conception doit pouvoir être vérifiée avant d\'être déployée, pas après.',
  'co-power-t':'Ce qu\'un coordinateur peut faire — et ce qu\'il ne peut pas',
  'co-power-d':'Il <strong style="color:var(--text)">ne peut pas inventer un être humain</strong>. Aucun bio_hash n\'existe tant que plusieurs vérificateurs différents ne l\'ont pas attesté, et le coordinateur ne détient aucune de leurs clés. Ce qu\'il peut faire, c\'est lier un bio_hash <strong style="color:var(--text)">existant</strong> à un portefeuille — un coordinateur malhonnête pourrait donc détourner une attribution vers une adresse de son choix. C\'est un pouvoir réel, il croît avec chaque coordinateur ajouté, et quiconque évalue s\'il faut faire confiance devrait connaître la différence.',
  'co-safe-t':'Pourquoi un deuxième coordinateur est sans danger',
  'co-safe-d':'Ce ne fut pas toujours le cas. Jusqu\'en août 2026, la promesse <strong style="color:var(--text)">un humain, une inscription</strong> reposait sur un verrou Redis à l\'intérieur du coordinateur — et deux coordinateurs indépendants ne partagent aucun Redis : deux inscriptions simultanées de la même personne seraient toutes deux passées. Désormais <strong style="color:var(--text)">chaque vérificateur contrôle lui-même</strong>, avant sa propre écriture, si ce visage est déjà inscrit. La garantie ne dépend plus d\'aucun service ni secret partagé ; un coordinateur peut donc se joindre ou disparaître sans rien y changer.',
  'co-need-t':'Ce qu\'il vous faut',
  'co-need-d':'Un compte Aequitas enregistré — la même règle que pour produire des blocs et pour vérifier : un humain, une clé. Un serveur avec Docker et une adresse HTTPS publique, car aucun navigateur ne confie la caméra à une page non sécurisée. Et deux clés à vous, que vous générez vous-même et qui ne quittent jamais votre machine : l\'une signe vos attestations, l\'autre associe les adresses de portefeuille à des marqueurs.',
  'co-keys-t':'N\'acceptez jamais une clé de quiconque — nous compris',
  'co-keys-d':'Deux coordinateurs partageant une même clé de signature ne sont pas deux coordinateurs : c\'est un seul avec deux adresses, et le quorum censé protéger les personnes semblerait atteint sans l\'être. Générez les deux clés sur votre propre machine, avec votre propre aléa, et n\'en laissez sortir aucune.',
  'co-auth-t':'Autoriser votre clé — sans permission',
  'co-auth-d':'Tant que votre clé n\'est pas autorisée, les vérificateurs refusent tout ce qu\'elle signe. L\'autorisation demande deux preuves et l\'accord de personne : votre portefeuille signe qu\'un humain enregistré se tient derrière cette clé, et votre coordinateur prouve sur son propre serveur que la clé est bien la sienne. La première se produit avec le bouton ci-dessus ; la seconde, votre coordinateur la produit seul. Jusqu\'en août 2026 il fallait en plus un secret partagé venant de nous — et ce secret <em>était</em> la permission. Il a disparu.',
  'co-pernode-t':'Le registre est propre à chaque nœud, et c\'est délibéré',
  'co-pernode-d':'Une autorisation inscrite sur un nœud ne voyage pas vers les autres — il n\'existe ni transaction ni diffusion pour cela. Une liste de confiance répliquée serait exactement l\'autorité centrale sans laquelle ce système est bâti : chaque opérateur décide lui-même quelles attestations son nœud accepte. Le coût, c\'est que votre autorisation doit être envoyée à chaque nœud censé l\'honorer. La signature, elle, est transférable : vous signez une fois et l\'envoyez partout ; un nœud oublié continuera simplement de vous refuser.',
  'co-law-t':'Ce que vous apprenez sur autrui — et ce qui en découle',
  'co-law-d':'La capture passe par vous ; vous la transmettez et n\'en gardez rien. Mais vous seul détenez la correspondance entre adresse de portefeuille et marqueur pour les personnes qui s\'inscrivent chez vous — d\'où la nécessité que votre clé de marqueur reste la vôtre : partagée, n\'importe quel opérateur pourrait calculer le marqueur de n\'importe quelle adresse publique et retrouver à qui appartient ce visage. Cela signifie aussi que vous devenez le <strong style="color:var(--text)">responsable du traitement</strong> pour ces personnes au sens du RGPD. Pas nous. Les demandes d\'accès, d\'effacement et d\'opposition vous parviennent, et ce n\'est pas une formalité.',
  'co-limit-t':'La seule limitation qui en résulte',
  'co-limit-d':'L\'effacement par adresse de portefeuille ne fonctionne qu\'auprès du coordinateur où l\'inscription a eu lieu : votre marqueur dépend de votre clé, et un autre coordinateur en dérive un différent pour la même adresse. Un « introuvable » venu d\'ailleurs signifie donc « pas enregistré ici », et non « pas enregistré » — et la réponse le dit. La voie passant par le bio_hash de la personne, celle qui lui appartient et ne requiert aucun opérateur, fonctionne auprès de tout coordinateur, car cet identifiant reste le même.',
  'x-authorize-coordinator-key':'🔑 Autoriser une clé de coordinateur',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — un ordre unique tiré d’un graphe enchevêtré',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Coefficient de Gini',
  'x-gini-coefficient-0-1':'Coefficient de Gini (0–1)',
  'x-gini-index-history':'Historique de l’indice de Gini',
  'x-gini-target-scandinavian-level':'Objectif Gini (niveau scandinave)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'ZKP Groth16 (divulgation nulle)',
  'x-guardian-system-8212-human-failsafe':'Personne de confiance &#8212; sécurité humaine pour les portefeuilles perdus',
  'x-hash-wallet':'Empreinte / portefeuille',
  'x-healthier-than-most-nations-on':'Plus sain que la plupart des pays du monde. Comparable à la Scandinavie (0,27) et à l’Allemagne (0,31). Le plafond de richesse et la fonte monétaire maintiennent une répartition équitable.',
  'x-higher-than-most-european-nations':'Plus élevé que dans la plupart des pays européens — comparable au Brésil (0,53) ou à la Russie. La redistribution du protocole tourne à intensité accrue.',
  'x-honest-limitation':'Limite assumée :',
  'x-how-it-works':'Comment cela fonctionne',
  'x-how-to-read-this-chart':'Comment lire ce graphique :',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'personnes pourraient s’inscrire',
  'x-imagine-a-world-where-every':'« Imaginez un monde où chaque personne sur Terre &#8212; peu importe où elle est née, quelle langue elle parle ou combien d’argent avaient ses parents &#8212; reçoit un revenu quotidien garanti simplement parce qu’elle est humaine. Non par charité. Comme un droit mathématique, appliqué par un code qu’aucun gouvernement ni aucune entreprise ne peut contourner. »',
  'x-inactive-escrow':'Séquestre pour inactivité',
  'x-inactivity-timeline':'Calendrier d’inactivité',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (résistant au quantique)',
  'x-key-protections':'Protections essentielles :',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — l’évolution propre à Aequitas au-delà d’un GHOSTDAG à K fixe',
  'x-knightdag-secured':'· sécurisé par KnightDAG',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Comme la Scandinavie (~0,27)',
  'x-liquidity-pool-30':'Pool de liquidité (30 %)',
  'x-loading-blocks':'Chargement des blocs…',
  'x-loading-topology':'Chargement de la topologie…',
  'x-loading-transactions':'Chargement des transactions…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Courbe de Lorenz — répartition des AEQ entre les personnes',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile : si le solde AEQ affiche 0 après l’inscription, allez dans Paramètres → Réseaux → supprimez la chaîne Aequitas → rajoutez-la via ce site',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile : si AEQ affiche 0 après l’ajout, supprimez le réseau et rajoutez-le avec le bouton ci-dessus.',
  'x-money-exists-because-people-exist':'L’argent existe parce que les gens existent. Chaque personne devrait donc en avoir une part égale, du simple fait d’être humaine.',
  'x-money-exists-because-people-exist-2':'« L’argent existe parce que les gens existent. Rien de plus, rien de moins. »',
  'x-most-unequal-currency-ever':'Monnaie la plus inégalitaire jamais vue',
  'x-multi-validator-network':'Réseau à plusieurs validateurs',
  'x-n-lt-10-not-yet':'⚠ N&lt;10 : pas encore significatif',
  'x-no-snapshots-yet-first-one':'Aucun relevé pour l’instant — le premier sera enregistré après la prochaine distribution.',
  'x-no-stake-blockchain':'Blockchain sans mise',
  'x-node-operator-guide-pdf':'📄 Guide de l’opérateur de nœud (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET doit être une personne Aequitas enregistrée',
  'x-one-human-one-wallet-1':'Une personne = un portefeuille = 1 000 AEQ',
  'x-p2p-protocol':'Protocole P2P',
  'x-paid-out-daily':'versé quotidiennement',
  'x-permanent-on-chain':'Permanent · sur la chaîne',
  'x-phase-roadmap-8212-the-path':'Feuille de route par phases &#8212; le chemin vers l’échelle mondiale',
  'x-phase-transitions-are-automatic-8212':'Les passages de phase sont automatiques &#8212; déclenchés par des seuils de population, appliqués par le contrat. Aucun vote, aucune clé d’administration.',
  'x-planned-post-beta':'Prévu (après la bêta)',
  'x-postgresql-persistent':'PostgreSQL (persistant)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Apportez de la liquidité AEQ / tUSD pour percevoir 30 % de tous les frais d’échange, distribués quotidiennement.',
  'x-recorded-after-each-ubi-distribution':'Enregistré après chaque distribution du revenu de base. Montre l’évolution de l’égalité à mesure que le réseau grandit. Plus bas est mieux — l’objectif est un Gini inférieur à 0,30.',
  'x-redistribution':'REDISTRIBUTION',
  'x-run-a-node':'⚙️ Faire tourner un nœud',
  'x-run-a-verifier':'⚙️ Faire tourner un vérificateur',
  'x-set-guardian':'🛡 DÉSIGNER UNE PERSONNE DE CONFIANCE',
  'x-swap-fees-0-1':'Frais d’échange (0,1 %)',
  'x-sybil-resistance-8212-current-state':'Résistance Sybil &#8212; l’état actuel, honnêtement',
  'x-the-4-redistribution-mechanisms':'Les quatre mécanismes de redistribution',
  'x-the-core-innovation':'L’idée centrale',
  'x-the-matching-threshold-has-not':'Le seuil de correspondance n’a pas encore été étalonné sur des captures réelles',
  'x-the-vision-8212-a-global':'La vision &#8212; un protocole mondial de revenu de base',
  'x-the-year-is-2009-satoshi':'Nous sommes en 2009. Satoshi Nakamoto publie Bitcoin. Pour la première fois, de la valeur peut circuler entre deux personnes sans banque. Une véritable révolution. Mais presque aussitôt, quelque chose tourne mal.',
  'x-this-is-not-a-0815':'Ce n’est pas une blockchain ordinaire produisant un bloc à la fois. Aequitas fait tourner un véritable BlockDAG, ordonné par GHOSTDAG — et depuis 2026 sécurisé par KnightDAG, sa propre évolution adaptative. C’est le mécanisme dont dépendent en dernier ressort chaque solde, chaque versement et chaque plafond, pour qu’il existe une histoire unique et admise par tous.',
  'x-today-beta':'Aujourd’hui (bêta)',
  'x-today-this-verifies-one-device':'Aujourd’hui cela vérifie un appareil, pas encore une personne unique',
  'x-traditional-blockchain-wasted-work':'Blockchain classique — travail gaspillé',
  'x-treasury-10':'Trésorerie (10 %)',
  'x-trusted-verified-human':'personne vérifiée et de confiance',
  'x-two-validators-produce-at-once':'Deux validateurs produisent en même temps → l’un gagne, l’autre est écarté — du travail perdu, et cela limite la vitesse à laquelle le réseau peut avancer sans risque.',
  'x-ubi-pool-20':'Fonds de revenu de base (20 %)',
  'x-validators-pool-40':'Fonds des validateurs (40 %)',
  'x-view-source-on-github':'🐙 Voir le code sur GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'Multiplicateur du plafond de richesse — curseur d’amorçage',
  'x-wealth-cap-overflow':'Dépassement du plafond de richesse',
  'x-wealth-distribution-analysis':'Analyse de la répartition des richesses',
  'x-what-happens-when-someone-is':'Que se passe-t-il si quelqu’un est hospitalisé, incarcéré ou décède ? Dans la plupart des systèmes crypto, un portefeuille perdu l’est pour toujours. Aequitas dispose d’un dispositif de récupération à trois niveaux.',
  'x-what-is-a-guardian':'Qu’est-ce qu’une personne de confiance ?',
  'x-what-is-and-is-not':'Ce qui est privé et ce qui ne l’est pas :',
  'x-what-would-a-cryptocurrency-look':'« À quoi ressemblerait une cryptomonnaie conçue dès l’origine pour être juste envers chaque être humain ? »',
  'x-why-a-normal-blockchain-isn':'Pourquoi une blockchain ordinaire ne suffit pas',
  'x-worse-than-any-country-on':'Pire que n’importe quel pays au monde (record sud-africain : 0,63). Approche du Bitcoin (0,85). Le protocole intervient au maximum — plafond et redistribution à pleine puissance.',
  'x-year-2-180d':'An 2 +180 j',
  'x-zk-device-key-proof':'Preuve ZK de la clé d’appareil',
  'swap-price-flat':'Aucune transaction sur cette période — le prix n’a pas bougé. Le graphique fonctionne ; c’est le marché qui est calme.',
  'mpc-optin-title':'Optionnel — aider à détecter les inscriptions en double (prêt, pas encore en service)',
  'mpc-optin-desc':'Préparé, mais pas encore en service. Plus tard, votre nœud pourra aider à vérifier que personne ne s\'inscrit deux fois sans jamais voir de données biométriques : chaque partie ne détient qu\'une part mathématique de chaque gabarit — du bruit à elle seule — et elles comparent ensemble une nouvelle capture, si bien qu\'aucune machine ne peut rien reconstruire. Aujourd\'hui ce chemin ne décide rien : la vérification des doublons n\'y passe pas, et le comité est une liste fixe plutôt qu\'un tirage automatique.',
  'mpc-optin-note':'Le fichier de parts contient un aléa à usage unique que seul votre nœud peut détenir — ne le copiez jamais sur une autre machine et ne le versionnez nulle part. Il doit actuellement venir de l\'opérateur, ce qui reste la dépendance centrale. Vous n\'avez pas besoin d\'une nouvelle clé : votre nœud s\'identifie avec la clé de signature qu\'il utilise déjà pour les blocs.',
  'logo-sub':'PREUVE D\'HUMANITÉ','live':'EN DIRECT',
  'reg-title':'🔐 S\'inscrire en tant qu\'humain vérifié',
  'reg-sub':'Rejoignez le réseau Aequitas et recevez 1 000 AEQ de Revenu de Base Universel. L\'inscription est unique, permanente et totalement sans frais. Aucune donnée personnelle n\'est stockée.',
  'app-title':'INSCRIPTION VIA L\'APPLICATION ANDROID',
  'app-text':'À l\'inscription, la caméra capture ton visage et une courte séquence de vivacité. Des services de comparaison indépendants vérifient qu\'une personne vivante est présente et que ce visage n\'est pas déjà enregistré ; ils doivent s\'accorder par quorum. Une preuve ZK Groth16 porte ensuite le résultat sur la chaîne sans rien révéler de toi. Tes <strong style="color:var(--gold)">1 000 AEQ sont crédités automatiquement</strong> après vérification. <strong style="color:var(--gold)">Note :</strong> le seuil de comparaison n\'est pas encore calibré sur des captures réelles — voir la FAQ ci-dessous.',
  's1t':'Capture du visage','s1d':'L\'application enregistre ton visage et une courte séquence de vivacité, puis les envoie à des services de comparaison indépendants. Ceux-ci vérifient qu\'une personne vivante est présente et comparent le visage à toutes les personnes déjà enregistrées. Les images sont écartées après traitement.',
  's2t':'Génération de Preuve ZK','s2d':'Une preuve ZK Groth16 engage ton bio_hash dans commitment = keccak256(bioHash‖wallet) sans le révéler. Le nullifier est dérivé de ce hachage, donc le même visage ne peut pas compter deux fois — voir la FAQ ci-dessous.',
  's3t':'Connecter le Portefeuille','s3d':'L\'app ouvre MetaMask · connectez votre portefeuille Ethereum · la preuve est liée cryptographiquement à votre adresse',
  's4t':'1 000 AEQ Accordés','s4d':'Inscription confirmée sur le BlockDAG en 1 seconde · 1 000 AEQ crédités instantanément · identité enregistrée en permanence',
  'priv-bar':'🔒 Vérification du visage par quorum · Groth16 ZKP · Images écartées après vérification · Un enregistrement par personne',
  'conn-wallet':'PORTEFEUILLE CONNECTÉ','proof-recv':'⚡ PREUVE ZK REÇUE','proof-hint':'Connecter un portefeuille pour s\'inscrire',
  'btn-conn':'🦊 CONNECTER METAMASK','btn-reg':'🔐 INSCRIPTION ON-CHAIN',
  'btn-wc':'🔗 CONNECTER WALLETCONNECT',
  'reg-log-hint':'// Ouvrir l\'app Android Aequitas pour générer votre preuve, puis revenir ici...',
  'reg-details':'Détails d\'inscription','k-network':'Réseau','k-chainid':'ID de chaîne','k-grant':'Allocation UBI',
  'k-fee':'Frais de gaz','free':'GRATUIT — totalement sans frais','k-limit':'Inscriptions','k-limit-v':'Une fois par personne · permanent · immuable',
  'k-bio':'Visage','never-stored':'Les images sont écartées après la vérification — aucun validateur ne détient un gabarit entier',
  'k-proof':'Système de preuve','k-conf':'Confirmation','k-conf-v':'En 1 seconde (1 bloc)',
  'k-sybil':'Protection Sybil','k-sybil-v':'Une identité par personne · liée au visage, seuil pas encore calibré',
  's-height':'Hauteur de bloc',
  's-humans':'Humains vérifiés',
  's-supply':'Offre totale','s-supply-sub':'Toujours = Humains × 1 000 AEQ',
  's-uptime':'Disponibilité',
  'k-chain':'Nom de chaîne','k-symbol':'Symbole','k-btime':'Temps de bloc',
  'k-cons':'Consensus','k-storage':'Stockage','k-dec':'Décimales',
  'btn-add-mm':'+ AJOUTER LE RÉSEAU AEQUITAS',
  'humans-title':'Humains vérifiés sur Aequitas Chain',
  'h-what':'Qu\'est-ce qu\'un humain vérifié ?','h-what-t':'Un Humain vérifié est une adresse de portefeuille dont il est prouvé qu\'elle appartient à quelqu\'un dont le visage n\'est pas déjà enregistré. Des services de comparaison indépendants doivent s\'accorder par quorum, et seule une preuve ZK Groth16 atteint la chaîne — aucune image et aucun gabarit. <strong style="color:var(--gold)">Jusqu\'au 23-08-2026, cela vérifiait un appareil et non une personne ; ce n\'est plus le cas.</strong>',
  'h-zkp':'Système de preuve ZK','h-zkp-t':'Aequitas utilise Groth16 sur BN128 — même courbe qu\'Ethereum et Zcash. ~200 octets, ~10ms. commitment = keccak256(deviceKey‖wallet). Le nullifier est lié à cet appareil : perdre le téléphone ne crée pas une seconde identité sur cet appareil, mais un autre appareil peut toujours s\'inscrire séparément. Le matériel de la clé n\'est jamais révélé ni stocké côté serveur.',
  'h-sybil':'Résistance Sybil — État Actuel','h-sybil-t':'Le nullifier est dérivé du bio_hash de ton visage, donc le même visage ne peut pas être enregistré deux fois — y compris d\'un appareil à l\'autre, ce qu\'une clé d\'appareil n\'a jamais pu faire. Ce sur quoi cela repose est un seuil de comparaison pas encore calibré sur des captures réelles : la cryptographie est exacte, la biométrie en dessous est une mesure dont le taux d\'erreur n\'est pas chiffré.',
  'h-global':'Inclusion financière mondiale','h-global-t':'Pas besoin de compte bancaire, de carte de crédit ni d\'expérience préalable des cryptomonnaies. Juste un smartphone Android avec une caméra. Aequitas est conçu pour être accessible à chaque être humain sur Terre.',
  'h-bio-hw':'Feuille de Route de Vérification d\'Identité','h-bio-hw-t':'Aujourd\'hui (bêta) : une vérification du visage par des services de comparaison indépendants qui doivent s\'accorder par quorum. Son seuil n\'est pas encore calibré sur des captures réelles — il faut environ 1000 paires d\'imposteurs avant d\'avancer un chiffre. Prévu : cette calibration, et un contrôle de doublons où aucun service ne détient un gabarit entier.',
  'reg-humans':'Humains inscrits','h-desc':'Chaque adresse ci-dessous appartient à une personne dont le visage a été comparé par des services indépendants à tous les enregistrements existants, prouvé par une preuve ZK, et créditée d\'exactement 1 000 AEQ. Le registre est permanent, immuable et on-chain. Ce que le seuil garantit aujourd\'hui, et ce qu\'il ne garantit pas, est dans la FAQ.',
  'no-humans':'Aucun humain inscrit pour l\'instant.\n\nTéléchargez l\'application Android Aequitas et soyez le premier !',
  'reg-stats':'Statistiques du registre','total-humans':'Total d\'humains',
  'idx-title':'Index Aequitas — Score d\'égalité économique en temps réel',
  'idx-desc':'L\'Index Aequitas est dérivé du <strong style="color:var(--teal)">coefficient de Gini</strong> — la norme internationale pour mesurer les inégalités (Banque mondiale, OCDE, ONU). <strong style="color:var(--neon)">0 = égalité parfaite</strong>. <strong style="color:var(--red)">100 = concentration totale</strong>. Objectif : Gini sous 0,30.',
  'gini-what-title':'Qu\'est-ce que le coefficient de Gini ?',
  'gini-what-text':'Développé par Corrado Gini (1912). Mesure la distribution des richesses. Échelle : 0 (tous égaux) à 1 (une personne détient tout). Utilisé par la Banque mondiale, l\'OCDE, l\'ONU.',
  'gini-calc-title':'Comment l\'Index est-il calculé ?',
  'gini-calc-text':'Tous les soldes AEQ collectés. Différence absolue moyenne entre toutes les paires, normalisée par n² et le solde moyen. Résultat × 100 = Index Aequitas.',
  'gini-why-title':'Pourquoi le Gini ?',
  'gini-why-text':'Un simple ratio riche/pauvre est manipulable. Le Gini capture la distribution complète en un seul chiffre auditable, publié on-chain — transparent et vérifiable mondialement.',
  'curr-idx':'Index actuel','bar-0':'0 — Égalité parfaite','bar-100':'100 — Inégalité max','wcap-lbl':'Plafond de richesse :','wcap-mult':'Multiplicateur :','wcap-avg':'Part équitable :',
  'gini':'Coefficient de Gini','gini-desc':'0 = égal · 1 = inégal',
  'supply-desc':'Toujours = Humains × 1 000 AEQ',
  'phase':'Phase du protocole','phase-desc':'Avance automatiquement par nombre d\'humains',
  'humans-desc':'Enregistrements vérifiés par le visage',
  'pools-title':'Pools de redistribution',
  'pools-desc':'Chaque frais de swap, demurrage et dépassement du plafond est divisé entre quatre pools. Tous versent quotidiennement.',
  'vel-pool':'Pool des validateurs','vel-pool-desc':'40% de tous les frais → opérateurs de nœuds qui sécurisent le réseau',
  'liq-pool':'Pool de liquidité','liq-pool-desc':'30% de tous les frais → fournisseurs de liquidité, proportionnellement aux parts LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% de tous les frais → tous les humains vérifiés également, toutes les 24 heures',
  'treasury':'Trésorerie','treasury-desc':'10% de tous les frais → développement et maintenance du protocole',
  'phases-title':'Phases du protocole',
  'phases-desc':'Plafond bootstrap Phase 0 : max(5, min(N, 25))× solde moyen. 1–4 humains : 5×. Chaque humain ajoute 1×. 25+ humains : verrouillé à 25×. Transitions automatiques.',
  'p0':'Bootstrap · &lt;100 humains · Plafond : max(5,min(N,25))× moyen · 5×→25× · Actuellement actif',
  'p1':'Croissance · 100–10 000 humains · Plafond : 25× la part équitable = 25 000 AEQ',
  'p2':'Stabilité · 10 000–1M humains · Plafond : 25× la part équitable = 25 000 AEQ',
  'p3':'Maturité · 1M+ humains · Plafond : 25× la part équitable = 25 000 AEQ',
  'wealth-cap-explain':'Plafond Phase 0 : max(5, min(N, 25))× solde moyen. 1–4 humains : 5×. Chaque humain +1×. 25+ : verrouillé à 25×.',
  'demurrage-title':'Demurrage — Incitation à la circulation',
  'demurrage-desc':'Les soldes AEQ inactifs perdent lentement de la valeur pour décourager l\'accumulation.',
  'dem-rate-k':'Taux de décroissance','dem-rate-v':'0,5 % par mois (continu)',
  'dem-grace-k':'Période de grâce','dem-grace-v':'3 mois d\'inactivité avant début de décroissance',
  'dem-reset-k':'Réinitialisation','dem-reset-v':'Tout transfert, swap ou action de liquidité remet le compteur à zéro',
  'dem-dest-k':'L\'AEQ décroissant va vers','dem-dest-v':'Pools de redistribution (40/30/20/10)',
  'dem-warn-k':'Système d\'avertissement','dem-warn-v':'Avis 14 jours (une fois) + rappel 7 jours à chaque connexion',
  'story-title':'L\'histoire d\'Aequitas',
  'nodes-title':'Nœuds actifs — Topologie réseau actuelle',
  'nodes-desc':'Le réseau Aequitas fonctionne sur plusieurs nœuds géographiquement distribués (nombre actuel ci-dessus) participant à la production de blocs, synchronisation d\'état et service API. Nœuds supplémentaires bienvenus.',
  'run-node-title':'Exécuter votre propre nœud','run-node-desc':'Toute personne enregistrée peut faire tourner un nœud Aequitas — sans mise, sans candidature, sans autorisation de notre part. Une personne, une clé de validateur : un nœud dont le NODE_OPERATOR_WALLET n\'est pas une personne enregistrée est refusé avec HTTP 403, sinon une seule personne pourrait devenir discrètement l\'ensemble des validateurs. Opérateurs gagnent 40% des frais de swap distribués quotidiennement.',
  'bootstrap-title':'Connecter un nouveau nœud','bootstrap-desc':'Aucun point d\'entrée à configurer — les adresses des validateurs sont intégrées. Votre nœud s\'enregistre seul et synchronise automatiquement l\'état complet de la chaîne. Définissez PRIMARY_NODE_URL uniquement si vous voulez délibérément fixer un point d\'entrée précis.',
  'tech-title':'Spécifications techniques','mm-config':'Configuration MetaMask',
  'k-lang':'Langue','k-src':'Source','evm-yes':'Oui — JSON-RPC /rpc · Compatible MetaMask',
  'proto-label':'Protocole Aequitas V7 — Documentation technique',
  'ca-title':'Adresses des contrats',
  'ca-text':'Chaîne : Aequitas Chain (Chain ID : 1926 · 0x786)<br>RPC : __RPC__<br><br>BioVerifier : 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 : 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 définit les règles de l\'économie Aequitas et tient le registre qui les rend applicables : chaque nullifier revendiqué, chaque enregistrement, le plafond de patrimoine et la formule de fonte. Le contrat est immuable — aucune clé d\'administration, aucun proxy de mise à jour, aucun vote de gouvernance ne peut en changer une ligne. Ce qui règle un transfert réel, en revanche, c\'est la couche chaîne : le nœud intercepte l\'appel ERC-20 avant qu\'il n\'atteigne l\'EVM et l\'applique à son propre grand livre — c\'est ce qui rend les transferts instantanés et sans frais de gaz. Le contrat est le règlement et le registre ; la chaîne est le moteur qui les exécute, et son code est public.',
  'poa-title':'1. PREUVE DE VIE','poa-text':'<p>Quand les gens décèdent, leurs AEQ retournent progressivement à la communauté via le pool UBI plutôt que d\'être perdus comme dans Bitcoin.</p>',
  'poa-box':'Années 0–2 : Utilisation normale<br>Année 2 : Avertissement 1 — Gardien peut répondre<br>Année 2+60j : Avertissement 2<br>Année 2+120j : Avertissement 3<br>Année 2+180j : AEQ en séquestre personnel<br>Année 4 : Si inactif — retourne au Pool UBI',
  'guard-title':'2. SYSTÈME DE GARDIEN','guard-text':'<p>Un Gardien de confiance (autre humain vérifié) peut confirmer qu\'une personne est encore en vie, sans aucun droit de transaction.</p>',
  'guard-box':'1 Gardien par humain · doit être humain vérifié Aequitas<br>Gardien peut UNIQUEMENT appeler confirmAlive() · zéro droit financier<br>Gardien NE PEUT PAS déplacer des fonds · Max 3 protégés · Timelock 7j',
  'dem-title':'3. DEMURRAGE — Anti-accumulation',
  'dem-box':'Prélevé uniquement sur la part au-dessus de votre part équitable — un solde égal ou inférieur ne décroît jamais<br>Taux : 0,5%/mois après 3 mois de grâce<br>Réinitialisation à chaque transfert, swap ou action de liquidité<br>AEQ décroissant redistribué dans les pools (non brûlé)',
  'dem-text':'<p>Précédent : Wörgl (Autriche, 1932) — réduction du chômage de 25% en un an. Chiemgauer (Allemagne, 2003) — fonctionne depuis plus de 20 ans.</p>',
  'cap-title':'4. PLAFOND DE RICHESSE','cap-box':'Plafond : max(5,min(N,25))× solde moyen<br>1–4 humains : 5× · +1× par humain · 25+ : 25× permanent<br>Excès immédiatement redistribué · Aucune intervention manuelle',
  'ubi-title':'5. REVENU DE BASE UNIVERSEL','ubi-box':'Sources : Frais de swap (20%) · Dépassement du plafond · Demurrage<br><br>Quotidien : Pool UBI divisé également entre tous les humains. Pool remis à zéro après chaque distribution.',
  'inf-title':'6. PAS D\'INFLATION ALGORITHMIQUE','inf-box':'Seul événement créant de l\'AEQ : un nouvel humain vérifié s\'inscrit.<br><br>Offre totale = Humains vérifiés × 1 000 AEQ — toujours, exactement.',
  'btn-download-app':'TÉLÉCHARGER AEQUITAS',
  'swap-title':'🔄 Échanger AEQ ↔ tUSD','swap-sub':'Échangez AEQ contre tUSD (dollar test) via le pool de liquidité natif. Frais 0,1% uniquement pour les swaps — transferts AEQ ordinaires totalement gratuits.',
  'swap-priv-bar':'🔒 Seulement 0,1% de frais · Transferts AEQ→AEQ gratuits · tUSD est une monnaie test sans valeur réelle',
  'swap-your-aeq':'Votre AEQ','swap-your-tusd':'Votre tUSD',
  'swap-fee-est':'Frais de protocole (0,1%)','swap-details-hdr':'Détails de l\'échange',
  'swap-out-lbl':'Vous recevez (est.)','swap-impact-lbl':'Impact sur le prix','swap-rate-lbl':'Taux de change',
  'swap-depth-lbl':'Composition du Pool','amm-title':'x × y = k — AMM à produit constant',
  'amm-text':'Lors d\'un swap, les réserves AEQ augmentent et les réserves tUSD diminuent — produit toujours égal à k. Swaps plus grands = plus grand impact sur le prix.',
  'swap-btn-go':'🔄 ÉCHANGER',
  'swap-log-hint':'// Connecter un portefeuille pour échanger...',
  'swap-no-liquidity':'Pas encore de tUSD ?','swap-faucet-desc':'Humains inscrits peuvent réclamer du tUSD test une fois','swap-btn-faucet':'💧 RÉCLAMER tUSD TEST',
  'swap-addliq-title':'Fournir de la liquidité','swap-addliq-desc':'Soyez le premier à déposer — votre ratio fixe le prix initial.','swap-btn-addliq':'💧 AJOUTER LIQUIDITÉ',
  'swap-lp-title':'Votre position LP','swap-lp-share':'Part du Pool','swap-lp-withdrawable':'Retirable',
  'swap-lp-pct-label':'% de votre position','swap-lp-youget':'Vous recevrez','swap-btn-removeliq':'🔥 RETIRER LIQUIDITÉ',
  'swap-pool-title':'AEQ / tUSD — Statut du Pool',
  'swap-pool-aeq':'Réserve AEQ','swap-pool-tusd':'Réserve tUSD','swap-pool-price':'Prix Spot',
  'swap-fee-bps':'Frais de Swap',
  'swap-pools-addr-title':'Adresses des Pools Tokenomiques',
  'swap-validators':'Validateurs (40%)','swap-lps':'Fournisseurs de Liquidité (30%)','swap-ubi':'Pool UBI (20%)','swap-treasury':'Trésorerie (10%)',
  'ubi-hero-title':'REVENU DE BASE UNIVERSEL — POOL UBI',
  'ubi-hero-sub':'Accumulation — prochain paiement distribué à tous les humains vérifiés dans :',
  'ubi-bal-lbl':'solde actuel du pool','ubi-hero-desc':'Divisé également · payé toutes les 24h · pool remis à zéro · solde minimum non requis',
  'ubi-how-fills':'Comment le Pool UBI se remplit',
  'ubi-src-swap':'Frais de Swap','ubi-src-swap-d':'Chaque swap AEQ↔tUSD contribue 20% de ses frais. Plus d\'échanges = remplissage plus rapide.',
  'ubi-src-dem':'Demurrage','ubi-src-dem-d':'AEQ inactif (3+ mois) décroît 0,5%/mois. 20% du décroissant va à l\'UBI.',
  'ubi-src-cap':'Dépassement du Plafond','ubi-src-cap-d':'Portefeuilles dépassant le plafond immédiatement rognés. 20% afflue vers l\'UBI.',
  'pools4-header':'Les quatre pools de redistribution',
  'ubi-see-above':'voir compte à rebours ci-dessus','ubi-timer-above':'⏰ compte à rebours affiché ci-dessus','pool-t-timer':'Accumulation — pas de minuterie',
  'usp-headline':'Pour la première fois dans l\'histoire — tout le monde commence à égalité',
  'usp-sub':'Si vous avez un smartphone Android, vous êtes éligible. Pas de banque, pas de crypto, pas d\'investissement.',
  'usp-c1-title':'0 € d\'investissement initial','usp-c1-desc':'Inscription totalement sans frais. Pas d\'ETH ni de carte bancaire. Le protocole paie tous les frais.',
  'usp-c2-title':'1 000 AEQ pour chaque humain','usp-c2-desc':'Milliardaire ou agriculteur — tous reçoivent exactement 1 000 AEQ. Égalité garantie mathématiquement.',
  'usp-c3-title':'Accessible à tous','usp-c3-desc':'Pas de compte bancaire, pas de carte de crédit, pas de pièce d\'identité, aucun matériel supplémentaire à acheter — juste la caméra déjà présente dans ton téléphone Android.',
  'usp-c4-title':'UBI quotidien pour toujours','usp-c4-desc':'Une fois inscrit, votre part des paiements UBI arrive automatiquement chaque jour — sans aucune action.',
  'v7-intro-title':'Qu\'est-ce qu\'AequitasV7 ?',
  'v7-intro-text':'AequitasV7 est le contrat intelligent central d\'Aequitas. Déployé de manière immuable sur Aequitas Chain (ID 1926). Gère tout : inscription humaine, vérification ZK, soldes, plafond de richesse, UBI, frais de swap. Aucun administrateur ne peut le modifier.',
  'swap-sell-label':'Vendre','swap-receive-label':'Recevoir',
  'guard-title':'🛡 Système de Gardien','guard-my-lbl':'Mon Gardien','guard-none':'Aucun',
  'guard-set-lbl':'Définir / Changer de Gardien','guard-set-hint':'Doit être un humain enregistré sur Aequitas · Verrou temporel de 7 jours · Le gardien peut uniquement confirmer votre vitalité, pas accéder aux fonds · Max 3 protégés par gardien',
  'guard-confirm-lbl':'Confirmer en Vie (En tant que Gardien)','guard-confirm-hint':'Si votre protégé ne peut pas accéder à son portefeuille, confirmez sa vitalité pour éviter que ses fonds soient transférés en séquestre après 910 jours d\'inactivité.','guard-recover-btn':'🔓 RÉCUPÉRER DU SÉQUESTRE',
  'faq-title':'❓ FAQ','faq-q1':'Mes données biométriques sont-elles sécurisées ?','faq-a1':'Ton visage est capturé et envoyé à des services de comparaison indépendants — c\'est la seule façon de vérifier réellement « une personne, un compte ». Les images sont traitées puis écartées ; elles ne sont pas conservées. Ce qui est conservé est un gabarit mathématique : chiffré et découpé en parts réparties entre des validateurs exploités séparément, de sorte qu\'aucun validateur n\'en détient jamais un entier. Une limite honnête, dite et non masquée : le service qui effectue la comparaison conserve tout de même des gabarits, car comparer les exige.',
  'faq-q1b':'L\'inscription prouve-t-elle que je suis une personne réelle et unique ?','faq-a1b':'Mieux qu\'une clé d\'appareil ne l\'a jamais pu, et pas encore démontrable en chiffres. Le visage est comparé à tous les enregistrements existants par des services indépendants qui doivent s\'accorder : la même personne sur un second téléphone est donc repérée, ce qu\'une clé d\'appareil n\'a jamais su faire. Ce qui manque, c\'est le taux d\'erreur : le seuil n\'est pas calibré sur des captures réelles, et cela demande environ 1000 paires d\'imposteurs.',
  'faq-q2':'Puis-je m\'inscrire avec un portefeuille différent plus tard ?','faq-a2':'Non. Une inscription est liée de façon permanente à une seule adresse de portefeuille. C\'est voulu : le nullifier dérivé de ton visage n\'est dépensé qu\'une fois, donc se réinscrire avec un autre portefeuille serait une seconde identité pour la même personne.',
  'faq-q3':'Que se passe-t-il si je perds mon téléphone ?','faq-a3':'Vos AEQ restent dans votre portefeuille — ils sont liés à votre clé privée, pas à votre téléphone. Vous pouvez toujours accéder à votre portefeuille via MetaMask avec votre phrase de récupération. La récupération du portefeuille est indépendante de l\'inscription biométrique.',
  'path-title':'Choisissez Votre Voie','path-human-title':'Je suis un Humain','path-human-desc':'Je veux m\'inscrire, recevoir 1 000 AEQ et rejoindre le réseau de revenu de base.','path-human-steps':'1. Télécharger l\'app Android Aequitas<br>2. Déverrouiller avec le verrouillage d\'écran de votre appareil (empreinte/visage/code)<br>3. Connecter MetaMask<br>4. Recevoir 1 000 AEQ instantanément',
  'path-node-title':'Je suis un Opérateur de Nœud','path-node-desc':'Je veux exécuter un nœud complet, participer à la production de blocs et gagner du pool de validateurs à 40%.','path-node-steps':'1. S\'inscrire en tant qu\'humain (obligatoire)<br>2. Aucun point d\'entrée à configurer — les adresses des validateurs sont intégrées<br>3. Déployer sur Contabo/Hetzner/tout VPS<br>4. Gagner quotidiennement du pool de validateurs',
  'path-dev-title':'Je suis un Développeur','path-dev-desc':'Je veux construire sur Aequitas, intégrer l\'API ou contribuer au protocole.','path-dev-steps':'1. JSON-RPC compatible EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Métriques: /metrics (Prometheus)',
  'story-flow-title':'Diagramme de Flux du Token AEQ','story-topo-title':'Topologie Réseau — État Actuel',
  'swap-price-title':'AEQ / tUSD — Prix en Direct','swap-price-desc':'Prix en temps réel dérivé des réserves du pool (x·y=k). Mis à jour toutes les 8 secondes.','swap-price-empty':'Pas encore de données de pool — ajoutez de la liquidité pour voir le graphique de prix.',
  'node-guide-lang-note':'Ce guide en ligne est en anglais. Un PDF traduit dans votre langue est disponible via le bouton ci-dessus.',
  'k-zkp':'Système ZKP','k-hash':'Système de Hachage','k-sybil-prot':'Protection Sybil',
  'soc-title':'💬 Réseaux sociaux','soc-sub':'Les annonces, l\'état de la chaîne et les questions qui dérangent &mdash; en public, sur les deux.',
  'soc-x-desc':'Les annonces, et ce que la chaîne fait vraiment. Format court.','soc-tg-desc':'Le groupe ouvert : questions, opérateurs de nœuds et aide à l\'inscription.',
  's-validators':'Validateurs actifs',
  'expl-heading':'Explorateur de blocs',
},
pt:{
  'x-consensus-ghostdag-knightdag':'◆ Consenso: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'Código do contrato',
  'x-demurrage-is-a-holding-cost':'A desvalorização programada é um custo por reter dinheiro — um juro negativo que torna a acumulação cara e a circulação atrativa. Há precedentes: a experiência de Wörgl (Áustria, 1932) usou uma moeda com desvalorização e reduziu o desemprego local em 25 % num ano. O Banco Nacional da Áustria acabou com ela precisamente por funcionar bem de mais e ameaçar o monopólio bancário. O Chiemgauer (Alemanha, 2003) assenta no mesmo princípio e circula com êxito há mais de 20 anos. A Aequitas aplica uma desvalorização contínua de 0,5 % ao mês, só após três meses de inatividade.',
  'x-network-consensus':'→ Rede / consenso',
  'x-node-decentralization-roadmap':'Roteiro de descentralização dos nós',
  'x-open-source-chain-logic':'Lógica da cadeia em código aberto',
  'x-phase-0-now':'Fase 0 (agora):',
  'x-phase-1-100-humans':'Fase 1 (mais de 100 pessoas):',
  'x-phase-2-1-000-humans':'Fase 2 (mais de 1.000 pessoas):',
  'x-phase-3-10-000-humans':'Fase 3 (mais de 10.000 pessoas):',
  'x-protocol-mechanisms':'Mecanismos do protocolo',
  'x-what-happens-to-aeq-when':'O que acontece aos AEQ quando alguém morre ou fica permanentemente incapacitado? No Bitcoin e na maioria das criptomoedas, uma carteira perdida significa oferta perdida para sempre — estima-se que milhões de BTC estejam inacessíveis para sempre. A Aequitas resolve isto com uma recuperação por inatividade em várias fases: se uma carteira não mostrar atividade durante muito tempo, o seu saldo regressa gradualmente à comunidade através do fundo de rendimento básico, para que a oferta realmente em circulação continue a fazer sentido.',
  'x-what-if-someone-is-hospitalized':'E se alguém for hospitalizado, preso ou ficar meses sem acesso ao aparelho? A pessoa de confiança — outro ser humano verificado — pode confirmar que a titular continua viva, impedindo que os seus AEQ passem para depósito. Essa pessoa não tem qualquer acesso financeiro: só pode chamar uma única função, que reinicia o relógio da inatividade. Em circunstância alguma pode mover, gastar ou consultar fundos.',
  'bv-bind':'🔗 Gerar assinatura de ligação',
  'bv-check-d':'A segunda chamada lista todos os verificadores e compara-os: se todos têm o mesmo número de registos, se falta uma semente a algum e se as chaves coincidem. Se a tua entrada mostrar divergência, é melhor saber aqui do que a meio do registo de alguém.',
  'bv-check-t':'Confirmar que funciona',
  'bv-desc':'Um nó que produz blocos protege o <strong style="color:var(--text)">livro de registos</strong>. Um verificador biométrico protege outra coisa: a promessa de que <strong style="color:var(--neon)">cada pessoa se regista apenas uma vez</strong>. São papéis distintos — podes ter um, ou ambos na mesma máquina.',
  'bv-guide-sub':'Passo a passo &middot; Não é preciso saber criptografia &middot; Cerca de 30 minutos, a maioria a descarregar',
  'bv-honest-d':'Esta parte está em beta e os limites são reais. A comparação conjunta consome material criptográfico de uso único, e uma entrega cobre por agora algumas dezenas de registos antes de ser preciso mais — a via confidencial prova-se primeiro em pequena escala, não em milhões. O trabalho cresce também com o número de pessoas inscritas. Publicamos estes números em vez de os arredondar: um sistema que pede o teu rosto não tem o direito de ser vago sobre o que consegue e o que ainda não.',
  'bv-honest-t':'Onde isto está hoje — sem rodeios',
  'bv-need-1':'<strong style="color:var(--text)">Uma conta Aequitas registada.</strong> A mesma regra da produção de blocos, e pelo mesmo motivo: uma pessoa, uma chave. Sem ela, uma só pessoa poderia tornar-se em silêncio num comité inteiro.',
  'bv-need-2':'<strong style="color:var(--text)">Um pequeno servidor Linux com Docker.</strong> Bastam 2 GB de memória. Sem placa gráfica: a comparação é aritmética sobre 64 bytes. A máquina onde já corre o teu nó serve.',
  'bv-need-3':'<strong style="color:var(--text)">Um domínio com HTTPS.</strong> Os outros membros do comité têm de te alcançar. Basta um subdomínio de algo que já tenhas.',
  'bv-need-4':'<strong style="color:var(--text)">Manteres-te acessível.</strong> Cada membro de um comité tem de responder para que um registo termine. Um verificador muitas vezes ausente atrasa as pessoas em vez de as proteger.',
  'bv-need-t':'Antes de começar — o que precisas',
  'bv-s1-note':'Guarda a metade privada no teu servidor e em mais lado nenhum. A metade pública é para partilhar: é assim que outros confirmam que atestaste algo. <strong style="color:var(--text)">A tua própria semente de projeção conta:</strong> como cada verificador usa uma diferente, uma base roubada a um não pode ser cruzada com a de outro. Se perderes a semente, as tuas parcelas guardadas deixam de significar algo — guarda-a num sítio que controles.',
  'bv-s1-t':'Passo 1 — Gera as tuas próprias chaves',
  'bv-s1-warn-d':'Dois verificadores com o mesmo segredo contam como um, e o comité seria menor do que parece. Ninguém — nós incluídos — deveria alguma vez enviar-te uma chave.',
  'bv-s1-warn-t':'Gera-as tu. Nunca aceites chaves de ninguém.',
  'bv-s2-d':'Coloca os valores do passo 1 num ficheiro que só tu possas ler. Um valor por linha, sem aspas.',
  'bv-s2-note':'<strong style="color:var(--gold)">Deixa ALLOW_REAL_BIOMETRIC_DATA em false</strong> até teres lido as notas de proteção de dados. Assim, o teu verificador entra na rede e participa em registos de teste sem nunca guardar dados de uma pessoa real. É a forma certa de começar, e não há pressa para mudar.',
  'bv-s2-t':'Passo 2 — Escreve o ficheiro de configuração',
  'bv-s3-note':'Uma resposta saudável indica <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> e <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. A primeira é a afirmação de que nenhum modelo completo é guardado, numa forma que podes verificar tu mesmo em vez de acreditar. Confirma agora e outra vez mais tarde — é a tua garantia tanto como a dos outros.',
  'bv-s3-t':'Passo 3 — Arranca o verificador',
  'bv-s4-d':'Os outros membros do comité alcançam-te pela internet pública, por isso a porta não pode ficar exposta sem cifra. O Caddy obtém o certificado sozinho.',
  'bv-s4-t':'Passo 4 — Põe HTTPS à frente',
  'bv-s5-d':'Os produtores de blocos ligam a sua chave a uma carteira humana registada: a carteira assina <strong style="color:var(--text)">Aequitas: authorize validator &lt;endereço&gt;</strong> e sem isso a cadeia recusa o lugar. O botão abaixo produz exatamente essa assinatura — para o papel de validador. <strong style="color:var(--text)">Uma chave de verificador ainda não tem essa ligação.</strong> A sua metade pública é recolhida fora da cadeia (passo 6) e adicionada à lista que cada servidor de prova verifica. Nada na cadeia a liga a uma pessoa. Enquanto isso faltar, um comité conta máquinas e não pessoas, e um operador poderia ter vários. Preferimos dizê-lo aqui a deixar o número parecer mais forte do que é.',
  'bv-s5-t':'Passo 5 — O que liga uma chave a uma pessoa (e o que ainda não)',
  'bv-s6-d':'Envia ao grupo a metade <strong style="color:var(--text)">pública</strong> do passo 1 juntamente com o teu endereço HTTPS. É acrescentada à lista que cada servidor de provas consulta e, a partir daí, as tuas atestações contam para o quórum. Neste passo nada de secreto sai da tua máquina — é esse o sentido da separação: a metade privada fica contigo para sempre, e a pública sem ela não vale nada.',
  'bv-s6-t':'Passo 6 — Publica a tua chave pública',
  'bv-status-d':'O código do verificador <strong style="color:var(--text)">ainda não é público</strong>, por isso nem todos conseguem hoje cumprir os passos abaixo. São publicados na mesma porque um desenho deve poder ser verificado antes de entrar em funcionamento, não depois. Se quiseres manter um, pergunta no grupo de Telegram ligado na página inicial. Abrir este repositório é o que transformará este guia de um plano numa convite, e é a próxima coisa que vos devemos.',
  'bv-status-t':'Estado: beta fechada — lê antes de começar',
  'bv-title':'Ou torna-te verificador biométrico — o papel que descentraliza a unicidade',
  'bv-what-d':'Nenhum rosto te é enviado. A tua máquina guarda uma <strong style="color:var(--text)">parcela aditiva</strong> de um extrato de 64 bytes: sozinha é indistinguível de ruído aleatório, e nenhum cálculo ao teu alcance recupera dela um rosto. As comparações fazem-se em conjunto com os outros membros do teu comité, e nenhum de vocês fica a saber mais do que a resposta — <em>duplicado: sim ou não</em>. Não é uma promessa sobre as nossas boas intenções; é uma propriedade da aritmética.',
  'bv-what-t':'O que terias — e o que nunca verias',
  'bv-why-d':'Um registo só é aceite quando <strong style="color:var(--text)">vários verificadores diferentes</strong> o atestaram. Uma chave roubada não chega — é preciso um comité inteiro. E como <strong style="color:var(--neon)">uma pessoa só pode ter uma chave de validador</strong>, comprar um comité significa ser essas tantas pessoas. Com 100 verificadores, quem controlar 10 tem menos de uma hipótese em 1.000 de possuir um comité completo de três. Cada pessoa que adere reduz esse número. É o único ponto em que o número de participantes <em>é</em> a segurança. <strong style="color:var(--text)">Este cálculo pressupõe uma pessoa por chave de verificador.</strong> Para a produção de blocos a cadeia impõe-no; para chaves de verificador ainda não (passo 5). Até lá, o número acima é um limite superior da segurança, não uma medição.',
  'bv-why-t':'Porque cada verificador adicional torna a rede mais difícil de corromper',
  'x-0-1-split-40-30':'0,1 % · repartição 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 pessoas. Teto de riqueza móvel 5x &#8594; 25x. Fase de alicerces.',
  'x-0-8211-2-years':'0 &#8211; 2 anos',
  'x-0-perfect-equality':'0 = igualdade perfeita',
  'x-1-000-aeq-minted':'+1.000 AEQ emitidos',
  'x-1-000-aeq-per-human':'1.000 AEQ por pessoa',
  'x-1-000-aeq-will-be':'Serão creditados 1.000 AEQ automaticamente',
  'x-10-000-8211-1m-humans':'10.000 &#8211; 1 M de pessoas. Mínimo de 10 nós. Totalmente descentralizado.',
  'x-100-8211-10-000-humans':'100 &#8211; 10.000 pessoas. Teto fixo 25x. Adesão livre de nós.',
  'x-100-maximum-concentration':'100 = concentração máxima',
  'x-1m-humans-global-ubi-at':'Mais de 1 M de pessoas. Rendimento básico mundial em larga escala. Meta Gini &lt;0,30.',
  'x-9679-liquidity-lp-30':'&#9679; Liquidez LP 30 %',
  'x-9679-treasury-10':'&#9679; Reserva 10 %',
  'x-9679-ubi-pool-20':'&#9679; Fundo de rendimento básico 20 %',
  'x-9679-validators-40':'&#9679; Validadores 40 %',
  'x-active-validators':'Validadores ativos',
  'x-add-aequitas-chain-to-metamask':'Adiciona a cadeia Aequitas ao MetaMask para veres o teu saldo AEQ, enviares transações e interagires com o contrato V7 a partir do navegador ou da carteira no telemóvel.',
  'x-admin-keys-or-governance-votes':'chaves de administração ou votações de governação',
  'x-aeq-activity':'ATIVIDADE AEQ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'BlockDAG da Aequitas — nada é desperdiçado',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Cadeia Aequitas (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'A Aequitas concretiza isto matematicamente. Cada pessoa verificada recebe exatamente 1.000 AEQ &#8212; multimilionário ou agricultor de subsistência, sem exceções. Quatro mecanismos de redistribuição impedem que a desigualdade se acumule indefinidamente. O coeficiente de Gini é registado na cadeia em tempo real.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — cadeia de prova de humanidade',
  'x-android-apk-direct-download':'APK Android · descarga direta',
  'x-architecture':'Arquitetura',
  'x-automatic-on-chain':'automático, na cadeia',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (grafo acíclico dirigido)',
  'x-blockdag-parallel-production':'BlockDAG · produção paralela',
  'x-blockdag-proof-of-humanity':'BlockDAG + prova de humanidade',
  'x-blue-score':'«pontuação azul»',
  'x-both-blocks-are-kept-ghostdag':'Ambos os blocos ficam — o GHOSTDAG integra o concorrente e continua a contá-lo na ordem canónica.',
  'x-canonical-winner':'vencedor canónico',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'Comparável aos EUA (0,41) ou à França (0,32). Dentro do intervalo da maioria das economias desenvolvidas. A redistribuição achata ativamente a curva.',
  'x-confirm-ward-is-alive':'✓ CONFIRMAR QUE A PESSOA ESTÁ VIVA',
  'x-core-technology':'Tecnologia central',
  'x-daily-ubi-returns-to-all':'o rendimento básico diário volta a todas as pessoas verificadas',
  'x-demurrage-0-5-mo':'Moeda com desvalorização (0,5 %/mês)',
  'x-device-bound-zk-proof-one':'Prova ZK ligada ao aparelho · um registo por aparelho',
  'x-diagonal-line-perfect-equality':'diagonal = igualdade perfeita',
  'x-disconnect-wallet':'⊘ DESLIGAR A CARTEIRA',
  'x-distinct-proposers-recent-blocks':'Proponentes distintos, blocos recentes',
  'x-distribution':'📈 Distribuição',
  'x-elliptic-curve':'Curva elíptica',
  'x-entire-distribution':'distribuição inteira',
  'x-evm-compatible':'Compatível com EVM',
  'x-fill-ghostdag-verdict-thin-ring':'Preenchimento = veredito GHOSTDAG · anel fino = proponente · uma coluna por altura. Passa o rato sobre um bloco para ver os detalhes.',
  'x-generate-node-binding-signature':'🔗 Gerar assinatura de ligação',
  'x-run-a-coordinator':'🚪 Executar um coordenador',
  'co-title':'Ou execute um coordenador — a porta por onde passa cada pessoa',
  'co-desc':'O coordenador é onde uma pessoa chega: emite o desafio, distribui a captura pelos verificadores, conta os seus votos e emite a atestação sobre a qual a cadeia cunha. Durante muito tempo existiu exatamente um — o que significava que cada registo da rede passava por uma única máquina. Não por faltar alguma coisa, mas porque ninguém tinha posto um segundo a funcionar.',
  'co-status-t':'Estado: beta fechada — a mesma ressalva do verificador',
  'co-status-d':'O coordenador vive no mesmo repositório que o verificador, e esse repositório <strong style="color:var(--text)">ainda não é público</strong>. Por isso os passos abaixo não podem hoje ser cumpridos por toda a gente. São publicados na mesma, pela mesma razão: um desenho deve poder ser verificado antes de entrar em produção, não depois.',
  'co-power-t':'O que um coordenador pode fazer — e o que não pode',
  'co-power-d':'<strong style="color:var(--text)">Não pode inventar uma pessoa</strong>. Nenhum bio_hash existe enquanto vários verificadores diferentes não o tiverem atestado, e o coordenador não detém nenhuma das suas chaves. O que pode fazer é ligar um bio_hash <strong style="color:var(--text)">existente</strong> a uma carteira — um coordenador desonesto poderia assim desviar uma atribuição para um endereço à sua escolha. É um poder real, cresce com cada coordenador acrescentado, e quem pondera confiar deve conhecer a diferença.',
  'co-safe-t':'Porque é que um segundo coordenador é seguro',
  'co-safe-d':'Nem sempre foi. Até agosto de 2026 a promessa <strong style="color:var(--text)">uma pessoa, um registo</strong> assentava num bloqueio Redis dentro do coordenador — e dois coordenadores independentes não partilham Redis: dois registos simultâneos da mesma pessoa teriam passado ambos. Agora <strong style="color:var(--text)">cada verificador verifica por si</strong>, antes da sua própria escrita, se aquele rosto já está inscrito. A garantia deixou de depender de qualquer serviço ou segredo partilhado, pelo que um coordenador pode juntar-se ou desaparecer sem a alterar.',
  'co-need-t':'Do que precisa',
  'co-need-d':'Uma conta Aequitas registada — a mesma regra que para produzir blocos e para verificar: uma pessoa, uma chave. Um servidor com Docker e um endereço HTTPS público, porque nenhum navegador entrega a câmara a uma página insegura. E duas chaves suas, que gera você mesmo e que nunca saem da sua máquina: uma assina as suas atestações, outra mapeia endereços de carteira para marcadores.',
  'co-keys-t':'Nunca aceite uma chave de ninguém — nem de nós',
  'co-keys-d':'Dois coordenadores com a mesma chave de assinatura não são dois coordenadores: são um com dois endereços, e o quórum que deveria proteger as pessoas pareceria cumprido sem o estar. Gere ambas as chaves na sua própria máquina, com a sua própria aleatoriedade, e não deixe sair nenhuma.',
  'co-auth-t':'Autorizar a sua chave — sem precisar de permissão',
  'co-auth-d':'Enquanto a sua chave não estiver autorizada, os verificadores recusam tudo o que ela assinar. Autorizá-la exige duas provas e a aprovação de ninguém: a sua carteira assina que há uma pessoa registada por trás desta chave, e o seu coordenador prova no seu próprio servidor que a chave é mesmo dele. A primeira produz-se com o botão acima; a segunda o coordenador produz sozinho. Até agosto de 2026 era ainda necessário um segredo partilhado nosso — e esse segredo <em>era</em> a permissão. Deixou de existir.',
  'co-pernode-t':'O registo é por nó, e isso é deliberado',
  'co-pernode-d':'Uma autorização escrita num nó não viaja para os outros — não existe transação para isso nem difusão. Uma lista de confiança replicada seria exatamente a autoridade central sem a qual este sistema foi construído: cada operador decide por si que atestações o seu nó aceita. O custo é que a sua autorização tem de ser enviada a cada nó que a deva honrar. A assinatura em si é transferível: assina uma vez e envia para todo o lado; um nó que salte continuará simplesmente a recusá-lo.',
  'co-law-t':'O que fica a saber sobre outras pessoas — e o que daí decorre',
  'co-law-d':'A captura passa por si; entrega-a e não guarda nada. Mas só você detém a correspondência entre endereço de carteira e marcador para quem se regista através de si — é por isso que a sua chave de marcador tem de continuar sua: partilhada, qualquer operador poderia calcular o marcador de qualquer endereço público e descobrir de quem é aquele rosto. Significa também que passa a ser <strong style="color:var(--text)">responsável pelo tratamento</strong> dessas pessoas ao abrigo do RGPD. Não nós. Pedidos de acesso, apagamento e oposição chegam a si, e isso não é uma formalidade.',
  'co-limit-t':'A única limitação que isto cria',
  'co-limit-d':'O apagamento por endereço de carteira só funciona no coordenador onde a inscrição foi feita: o seu marcador depende da sua chave, e outro coordenador deriva um diferente para o mesmo endereço. Um «não encontrado» vindo de outro lado significa portanto «não registado aqui», não «não registado» — e a resposta di-lo. A via através do próprio bio_hash, aquela que pertence à pessoa e não precisa de operador nenhum, funciona em qualquer coordenador, porque esse identificador mantém-se o mesmo.',
  'x-authorize-coordinator-key':'🔑 Autorizar chave do coordenador',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — uma ordem única a partir de um grafo emaranhado',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'Coeficiente de Gini',
  'x-gini-coefficient-0-1':'Coeficiente de Gini (0–1)',
  'x-gini-index-history':'Histórico do índice de Gini',
  'x-gini-target-scandinavian-level':'Meta Gini (nível escandinavo)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'ZKP Groth16 (conhecimento nulo)',
  'x-guardian-system-8212-human-failsafe':'Pessoa de confiança &#8212; salvaguarda humana para carteiras perdidas',
  'x-hash-wallet':'Hash / carteira',
  'x-healthier-than-most-nations-on':'Mais saudável do que a maioria dos países do mundo. Comparável à Escandinávia (0,27) e à Alemanha (0,31). O teto de riqueza e a desvalorização mantêm uma distribuição justa.',
  'x-higher-than-most-european-nations':'Mais alto do que na maioria dos países europeus — comparável ao Brasil (0,53) ou à Rússia. A redistribuição do protocolo atua com intensidade elevada.',
  'x-honest-limitation':'Limitação assumida:',
  'x-how-it-works':'Como funciona',
  'x-how-to-read-this-chart':'Como ler este gráfico:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'pessoas poderiam registar-se',
  'x-imagine-a-world-where-every':'«Imagina um mundo em que cada pessoa da Terra &#8212; independentemente de onde nasceu, que língua fala ou de quanto dinheiro os pais tinham &#8212; recebe um rendimento diário garantido apenas por ser humana. Não como caridade. Como um direito matemático, imposto por código que nenhum governo ou empresa pode contornar.»',
  'x-inactive-escrow':'Depósito por inatividade',
  'x-inactivity-timeline':'Cronologia da inatividade',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (resistente ao quântico)',
  'x-key-protections':'Proteções essenciais:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — a evolução própria da Aequitas para além de um GHOSTDAG de K fixo',
  'x-knightdag-secured':'· protegido por KnightDAG',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'Como a Escandinávia (~0,27)',
  'x-liquidity-pool-30':'Fundo de liquidez (30 %)',
  'x-loading-blocks':'A carregar blocos…',
  'x-loading-topology':'A carregar a topologia…',
  'x-loading-transactions':'A carregar transações…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'Curva de Lorenz — distribuição de AEQ pelas pessoas',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask Mobile: se o saldo AEQ mostrar 0 após o registo, vai a Definições → Redes → apaga a cadeia Aequitas → volta a adicioná-la por este site',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask Mobile: se o AEQ mostrar 0 depois de adicionares, apaga a rede e volta a adicioná-la com o botão acima.',
  'x-money-exists-because-people-exist':'O dinheiro existe porque as pessoas existem. Por isso, cada pessoa deveria ter uma parte igual, só por ser humana.',
  'x-money-exists-because-people-exist-2':'«O dinheiro existe porque as pessoas existem. Nada mais, nada menos.»',
  'x-most-unequal-currency-ever':'A moeda mais desigual de sempre',
  'x-multi-validator-network':'Rede com vários validadores',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: ainda não significativo',
  'x-no-snapshots-yet-first-one':'Ainda sem registos — o primeiro será guardado após a próxima distribuição.',
  'x-no-stake-blockchain':'Blockchain sem aposta',
  'x-node-operator-guide-pdf':'📄 Guia do operador de nó (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET tem de ser uma pessoa registada na Aequitas',
  'x-one-human-one-wallet-1':'Uma pessoa = uma carteira = 1.000 AEQ',
  'x-p2p-protocol':'Protocolo P2P',
  'x-paid-out-daily':'pago diariamente',
  'x-permanent-on-chain':'Permanente · na cadeia',
  'x-phase-roadmap-8212-the-path':'Plano por fases &#8212; o caminho para a escala mundial',
  'x-phase-transitions-are-automatic-8212':'As mudanças de fase são automáticas &#8212; desencadeadas por limiares de população e impostas pelo contrato. Sem votações, sem chave de administração.',
  'x-planned-post-beta':'Previsto (depois da beta)',
  'x-postgresql-persistent':'PostgreSQL (persistente)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'Fornece liquidez AEQ / tUSD para ganhares 30 % de todas as comissões de troca, distribuídas diariamente.',
  'x-recorded-after-each-ubi-distribution':'Registado após cada distribuição do rendimento básico. Mostra como a igualdade evolui à medida que a rede cresce. Quanto mais baixo, melhor — a meta é um Gini abaixo de 0,30.',
  'x-redistribution':'REDISTRIBUIÇÃO',
  'x-run-a-node':'⚙️ Manter um nó',
  'x-run-a-verifier':'⚙️ Executar um verificador',
  'x-set-guardian':'🛡 DEFINIR PESSOA DE CONFIANÇA',
  'x-swap-fees-0-1':'Comissões de troca (0,1 %)',
  'x-sybil-resistance-8212-current-state':'Resistência Sybil &#8212; o estado atual, com franqueza',
  'x-the-4-redistribution-mechanisms':'Os quatro mecanismos de redistribuição',
  'x-the-core-innovation':'A ideia central',
  'x-the-matching-threshold-has-not':'O limiar de correspondência ainda não foi calibrado com capturas reais',
  'x-the-vision-8212-a-global':'A visão &#8212; um protocolo mundial de rendimento básico',
  'x-the-year-is-2009-satoshi':'Estamos em 2009. Satoshi Nakamoto publica o Bitcoin. Pela primeira vez, valor pode passar entre duas pessoas sem um banco. Uma revolução verdadeira. Mas quase de imediato algo corre mal.',
  'x-this-is-not-a-0815':'Isto não é uma blockchain vulgar, com um bloco de cada vez. A Aequitas corre um BlockDAG a sério, ordenado pelo GHOSTDAG — e desde 2026 protegido pelo KnightDAG, a sua própria evolução adaptativa. É deste mecanismo que dependem, em última análise, cada saldo, cada pagamento e cada teto de riqueza, para que exista uma única história acordada.',
  'x-today-beta':'Hoje (beta)',
  'x-today-this-verifies-one-device':'Hoje isto verifica um aparelho, ainda não uma pessoa única',
  'x-traditional-blockchain-wasted-work':'Blockchain tradicional — trabalho desperdiçado',
  'x-treasury-10':'Reserva (10 %)',
  'x-trusted-verified-human':'pessoa verificada e de confiança',
  'x-two-validators-produce-at-once':'Dois validadores produzem ao mesmo tempo → um ganha, o outro é descartado — trabalho perdido, e limita a velocidade a que a rede pode avançar com segurança.',
  'x-ubi-pool-20':'Fundo de rendimento básico (20 %)',
  'x-validators-pool-40':'Fundo dos validadores (40 %)',
  'x-view-source-on-github':'🐙 Ver o código no GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'Multiplicador do teto de riqueza — cursor de arranque',
  'x-wealth-cap-overflow':'Excedente do teto de riqueza',
  'x-wealth-distribution-analysis':'Análise da distribuição da riqueza',
  'x-what-happens-when-someone-is':'O que acontece se alguém for hospitalizado, preso ou morrer? Na maioria dos sistemas cripto, uma carteira perdida fica perdida para sempre. A Aequitas tem uma recuperação por inatividade em três camadas.',
  'x-what-is-a-guardian':'O que é uma pessoa de confiança?',
  'x-what-is-and-is-not':'O que é privado e o que não é:',
  'x-what-would-a-cryptocurrency-look':'«Como seria uma criptomoeda desenhada de raiz para ser justa com todo o ser humano?»',
  'x-why-a-normal-blockchain-isn':'Porque é que uma blockchain normal não chega',
  'x-worse-than-any-country-on':'Pior do que qualquer país do mundo (recorde da África do Sul: 0,63). A aproximar-se do Bitcoin (0,85). O protocolo intervém no máximo — teto e redistribuição em plena força.',
  'x-year-2-180d':'Ano 2 +180 d',
  'x-zk-device-key-proof':'Prova ZK da chave do aparelho',
  'swap-price-flat':'Sem negócios neste período — o preço não se moveu. O gráfico funciona; o mercado é que está parado.',
  'mpc-optin-title':'Opcional — ajudar a verificar registos duplicados (preparado, ainda não ativo)',
  'mpc-optin-desc':'Preparado, mas ainda não em serviço. Mais tarde o teu nó poderá ajudar a verificar que ninguém se regista duas vezes sem nunca ver dados biométricos: cada parte guarda apenas uma parcela matemática de cada modelo — ruído por si só — e comparam em conjunto uma nova captura, pelo que nenhuma máquina consegue reconstruir nada. Hoje este caminho não decide nada: a verificação de duplicados não passa por aqui e o comité é uma lista fixa em vez de sorteado automaticamente.',
  'mpc-optin-note':'O ficheiro de parcelas contém aleatoriedade de uso único que só o teu nó pode guardar — nunca o copies para outra máquina nem o submetas a um repositório. De momento tem de vir do operador, e essa é a dependência central que resta. Não precisas de uma chave nova: o teu nó identifica-se com a mesma chave de assinatura que já usa para os blocos.',
  'logo-sub':'PROVA DE HUMANIDADE','live':'AO VIVO',
  'reg-title':'🔐 Registrar como Humano Verificado',
  'reg-sub':'Junte-se à rede Aequitas e receba 1.000 AEQ de Renda Básica Universal. Registro único, permanente e completamente sem taxas. Nenhum dado pessoal é armazenado.',
  'app-title':'REGISTRO VIA APLICATIVO ANDROID',
  'app-text':'No registo, a câmara capta o seu rosto e uma curta sequência de vivacidade. Serviços de comparação independentes verificam que está uma pessoa viva e que esse rosto ainda não está registado; têm de concordar por quórum. Uma prova ZK Groth16 leva depois o resultado para a cadeia sem revelar nada sobre si. Os seus <strong style="color:var(--gold)">1.000 AEQ são creditados automaticamente</strong> após a verificação. <strong style="color:var(--gold)">Nota:</strong> o limiar de comparação ainda não foi calibrado com captações reais — ver as perguntas frequentes abaixo.',
  's1t':'Captação do rosto','s1d':'A aplicação grava o seu rosto e uma curta sequência de vivacidade e envia-os a serviços de comparação independentes. Estes verificam que está uma pessoa viva à frente e comparam o rosto com todos os já registados. As imagens são descartadas após o processamento.',
  's2t':'Geração de Prova ZK','s2d':'Uma prova ZK Groth16 compromete o seu bio_hash em commitment = keccak256(bioHash‖wallet) sem o revelar. O nullifier deriva desse hash, portanto o mesmo rosto não pode contar duas vezes — ver as perguntas frequentes abaixo.',
  's3t':'Conectar Carteira','s3d':'O app abre MetaMask · conecte sua carteira Ethereum · prova ligada criptograficamente ao seu endereço',
  's4t':'1.000 AEQ Concedidos','s4d':'Registro confirmado no BlockDAG em 1 segundo · 1.000 AEQ creditados instantaneamente · identidade registrada permanentemente',
  'priv-bar':'🔒 Verificação do rosto por quórum · Groth16 ZKP · Imagens descartadas após a verificação · Um registo por pessoa',
  'conn-wallet':'CARTEIRA CONECTADA','proof-recv':'⚡ PROVA ZK RECEBIDA','proof-hint':'Conectar carteira para registrar',
  'btn-conn':'🦊 CONECTAR METAMASK','btn-reg':'🔐 REGISTRAR ON-CHAIN',
  'btn-wc':'🔗 CONECTAR WALLETCONNECT',
  'reg-log-hint':'// Abra o App Android Aequitas para gerar sua prova, depois retorne aqui...',
  'reg-details':'Detalhes do Registro','k-network':'Rede','k-chainid':'ID da Cadeia','k-grant':'Concessão UBI',
  'k-fee':'Taxa de Gás','free':'GRATUITO — completamente sem taxas','k-limit':'Registros','k-limit-v':'Uma vez por pessoa · permanente · imutável',
  'k-bio':'Rosto','never-stored':'As imagens são descartadas após a verificação — nenhum validador detém um modelo inteiro',
  'k-proof':'Sistema de Prova','k-conf':'Confirmação','k-conf-v':'Em 1 segundo (1 bloco)',
  'k-sybil':'Proteção Sybil','k-sybil-v':'Uma identidade por pessoa · ligada ao rosto, limiar ainda não calibrado',
  's-height':'Altura do Bloco',
  's-humans':'Humanos Verificados',
  's-supply':'Oferta Total','s-supply-sub':'Sempre = Humanos × 1.000 AEQ',
  's-uptime':'Disponibilidade',
  'k-chain':'Nome da Cadeia','k-symbol':'Símbolo','k-btime':'Tempo de Bloco',
  'k-cons':'Consenso','k-storage':'Armazenamento','k-dec':'Decimais',
  'btn-add-mm':'+ ADICIONAR REDE AEQUITAS',
  'humans-title':'Humanos Verificados na Aequitas Chain',
  'h-what':'O que é um Humano Verificado?','h-what-t':'Um Humano Verificado é um endereço de carteira para o qual está provado que pertence a alguém cujo rosto ainda não está registado. Serviços de comparação independentes têm de concordar por quórum, e à cadeia chega apenas uma prova ZK Groth16 — nenhuma imagem e nenhum modelo. <strong style="color:var(--gold)">Até 23-08-2026 isto verificava um dispositivo e não uma pessoa; já não é assim.</strong>',
  'h-zkp':'Sistema de Prova ZK','h-zkp-t':'Aequitas usa Groth16 sobre BN128 — a mesma curva do Ethereum e Zcash. ~200 bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). O nullifier está vinculado a este dispositivo: perder o telemóvel não cria uma segunda identidade neste dispositivo, mas outro dispositivo ainda pode registar-se separadamente. O material da chave nunca é revelado ou armazenado no servidor.',
  'h-sybil':'Resistência Sybil — Estado Atual','h-sybil-t':'O nullifier deriva do bio_hash do seu rosto, por isso o mesmo rosto não pode ser registado duas vezes — também entre dispositivos, algo que uma chave de dispositivo nunca conseguiu. Aquilo em que assenta é um limiar de comparação ainda não calibrado com captações reais: a criptografia é exata, a biometria por baixo é uma medição cuja taxa de erro não foi quantificada.',
  'h-global':'Inclusão Financeira Global','h-global-t':'Sem conta bancária, sem cartão de crédito, sem experiência prévia em criptomoedas. Basta um smartphone Android com câmara. A Aequitas foi concebida para ser acessível a todos os seres humanos do planeta.',
  'h-bio-hw':'Roteiro de Verificação de Identidade','h-bio-hw-t':'Hoje (beta): uma verificação do rosto por serviços de comparação independentes que têm de concordar por quórum. O seu limiar ainda não foi calibrado com captações reais — são precisos cerca de 1000 pares de impostores antes de citar qualquer número. Previsto: essa calibração e uma verificação de duplicados em que nenhum serviço detém um modelo inteiro.',
  'reg-humans':'Humanos Registrados','h-desc':'Cada endereço abaixo pertence a alguém cujo rosto foi comparado por serviços independentes com todos os registos existentes, provado com uma prova ZK e creditado com exatamente 1.000 AEQ. O registo é permanente, imutável e on-chain. O que o limiar garante hoje e o que não garante está nas perguntas frequentes.',
  'no-humans':'Nenhum humano registrado ainda.\n\nBaixe o App Android Aequitas e seja o primeiro humano na cadeia!',
  'reg-stats':'Estatísticas do Registro','total-humans':'Total de Humanos',
  'idx-title':'Índice Aequitas — Pontuação de Igualdade Econômica em Tempo Real',
  'idx-desc':'O Índice Aequitas é derivado do <strong style="color:var(--teal)">coeficiente de Gini</strong> (Banco Mundial, OCDE, ONU). <strong style="color:var(--neon)">0 = igualdade perfeita</strong>. <strong style="color:var(--red)">100 = concentração total</strong>. Meta: Gini abaixo de 0,30.',
  'gini-what-title':'O que é o Coeficiente de Gini?',
  'gini-what-text':'Desenvolvido por Corrado Gini (1912). Mede a distribuição de riqueza. Escala: 0 (todos iguais) a 1 (uma pessoa detém tudo). Banco Mundial, OCDE, ONU.',
  'gini-calc-title':'Como o Índice é calculado?',
  'gini-calc-text':'Todos os saldos AEQ coletados. Diferença absoluta média entre todos os pares, normalizada por n² e saldo médio. Resultado × 100 = Índice Aequitas.',
  'gini-why-title':'Por que Gini?',
  'gini-why-text':'Um simples ratio rico/pobre é manipulável. O Gini captura a distribuição completa em um único número auditável, publicado on-chain — transparente e verificável globalmente.',
  'curr-idx':'Índice Atual','bar-0':'0 — Igualdade Perfeita','bar-100':'100 — Desigualdade Máx.','wcap-lbl':'Teto de Riqueza Atual:','wcap-mult':'Multiplicador:','wcap-avg':'Parte justa:',
  'gini':'Coeficiente de Gini','gini-desc':'0 = igual · 1 = desigual',
  'supply-desc':'Sempre = Humanos × 1.000 AEQ',
  'phase':'Fase do Protocolo','phase-desc':'Avança automaticamente pelo número de humanos',
  'humans-desc':'Registos verificados pelo rosto',
  'pools-title':'Pools de Redistribuição',
  'pools-desc':'Cada taxa de swap, demurrage e excesso do teto é dividido entre quatro pools. Todos pagam diariamente.',
  'vel-pool':'Pool de Validadores','vel-pool-desc':'40% de todas as taxas → operadores de nodes que protegem a rede',
  'liq-pool':'Pool de Liquidez','liq-pool-desc':'30% de todas as taxas → provedores de liquidez, proporcional às cotas LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% de todas as taxas → todos os humanos verificados igualmente, a cada 24 horas',
  'treasury':'Tesouro','treasury-desc':'10% de todas as taxas → desenvolvimento e manutenção do protocolo',
  'phases-title':'Fases do Protocolo',
  'phases-desc':'Teto bootstrap Fase 0: max(5, min(N, 25))× saldo médio. 1–4 humanos: 5×. Cada humano +1×. 25+ humanos: travado em 25×. Transições automáticas.',
  'p0':'Bootstrap · &lt;100 humanos · Teto: max(5,min(N,25))× médio · 5×→25× · Ativo agora',
  'p1':'Crescimento · 100–10.000 humanos · Teto: 25× a parte justa = 25.000 AEQ',
  'p2':'Estabilidade · 10.000–1M humanos · Teto: 25× a parte justa = 25.000 AEQ',
  'p3':'Maturidade · 1M+ humanos · Teto: 25× a parte justa = 25.000 AEQ',
  'wealth-cap-explain':'Teto Fase 0: max(5, min(N, 25))× saldo médio. 1–4 humanos: 5×. Cada humano +1×. 25+: travado em 25×.',
  'demurrage-title':'Demurrage — Incentivo para Circular',
  'demurrage-desc':'Saldos AEQ inativos perdem lentamente valor para desencorajar acumulação.',
  'dem-rate-k':'Taxa de Decaimento','dem-rate-v':'0,5% por mês (contínuo)',
  'dem-grace-k':'Período de Graça','dem-grace-v':'3 meses de inatividade antes do decaimento começar',
  'dem-reset-k':'Reinicialização','dem-reset-v':'Qualquer transferência, swap ou liquidez reinicia o contador',
  'dem-dest-k':'AEQ decaído vai para','dem-dest-v':'Pools de redistribuição (40/30/20/10)',
  'dem-warn-k':'Sistema de Aviso','dem-warn-v':'Aviso 14 dias (uma vez) + lembrete 7 dias repetido em cada login',
  'story-title':'A História da Aequitas',
  'nodes-title':'Nodes Ativos — Topologia de Rede Atual',
  'nodes-desc':'A rede Aequitas opera atualmente em múltiplos nodes distribuídos geograficamente (número atual acima), participando da produção de blocos, sincronização e API. Nodes adicionais são bem-vindos.',
  'run-node-title':'Execute seu Próprio Node','run-node-desc':'Qualquer pessoa registada pode executar um node Aequitas — sem stake, sem candidatura, sem permissão nossa. Uma pessoa, uma chave de validador: um node cujo NODE_OPERATOR_WALLET não seja uma pessoa registada é recusado com HTTP 403, caso contrário uma só pessoa poderia tornar-se todo o conjunto de validadores. Operadores ganham 40% das taxas de swap distribuídas diariamente.',
  'bootstrap-title':'Conectar um Novo Node','bootstrap-desc':'Não há ponto de entrada a configurar — os endereços dos validadores estão integrados. O seu node regista-se sozinho e sincroniza automaticamente o estado completo da cadeia. Defina PRIMARY_NODE_URL apenas se quiser fixar deliberadamente um ponto de entrada específico.',
  'tech-title':'Especificações Técnicas','mm-config':'Configuração MetaMask',
  'k-lang':'Idioma','k-src':'Fonte','evm-yes':'Sim — JSON-RPC /rpc · Compatível MetaMask',
  'proto-label':'Protocolo Aequitas V7 — Documentação Técnica',
  'ca-title':'Endereços dos Contratos',
  'ca-text':'Cadeia: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 define as regras da economia Aequitas e mantém o registo que as torna exigíveis: cada nullifier alguma vez reclamado, cada registo, o limite de riqueza e a fórmula de demurrage. O contrato é imutável — nenhuma chave de administrador, nenhum proxy de atualização, nenhum voto de governação pode mudar uma linha. Mas quem liquida uma transferência real é a camada da cadeia: o nó interceta a chamada ERC-20 antes de chegar à EVM e aplica-a ao seu próprio livro-razão — é isso que torna as transferências instantâneas e sem gás. O contrato é o regulamento e o registo; a cadeia é o motor que os executa, e o seu código é público.',
  'poa-title':'1. PROVA DE VIDA','poa-text':'<p>AEQ de pessoas falecidas retorna gradualmente à comunidade via pool UBI, em vez de ser perdido para sempre como no Bitcoin.</p>',
  'poa-box':'Anos 0–2: Uso normal<br>Ano 2: Aviso 1 — Guardião pode responder<br>Ano 2+60d: Aviso 2<br>Ano 2+120d: Aviso 3<br>Ano 2+180d: AEQ em custódia pessoal<br>Ano 4: Se inativo — retorna ao Pool UBI',
  'guard-title':'2. SISTEMA DE GUARDIÃO','guard-text':'<p>Um Guardião de confiança (outro humano verificado) pode confirmar que alguém está vivo, sem nenhum direito de transação.</p>',
  'guard-box':'1 Guardião por humano · deve ser humano verificado Aequitas<br>Guardião pode APENAS chamar confirmAlive() · zero direitos financeiros<br>Guardião NÃO PODE mover fundos · Máx. 3 protegidos · Timelock 7d',
  'dem-title':'3. DEMURRAGE — Anti-Acumulação',
  'dem-box':'Cobrado apenas sobre a parte acima da sua parte justa — um saldo igual ou inferior nunca decai<br>Taxa: 0,5%/mês após 3 meses de graça<br>Reinicialização a cada transferência, swap ou liquidez<br>AEQ decaído redistribuído nos pools (não queimado)',
  'dem-text':'<p>Precedente: Wörgl (Áustria, 1932) — desemprego reduziu 25% em um ano. Chiemgauer (Alemanha, 2003) — opera com sucesso há mais de 20 anos.</p>',
  'cap-title':'4. TETO DE RIQUEZA','cap-box':'Teto: max(5,min(N,25))× saldo médio AEQ<br>1–4 humanos: 5× · +1× por humano · 25+: 25× permanente<br>Excesso redistribuído imediatamente · Sem intervenção manual',
  'ubi-title':'5. RENDA BÁSICA UNIVERSAL','ubi-box':'Fontes: Taxas de swap (20%) · Excesso do teto · Demurrage<br><br>Diário: Pool UBI dividido igualmente entre todos os humanos. Pool zera após cada distribuição.',
  'inf-title':'6. SEM INFLAÇÃO ALGORÍTMICA','inf-box':'Único evento criando AEQ: novo humano verificado se registra.<br><br>Oferta Total = Humanos Verificados × 1.000 AEQ — sempre, exatamente.',
  'btn-download-app':'BAIXAR AEQUITAS',
  'swap-title':'🔄 Trocar AEQ ↔ tUSD','swap-sub':'Troque AEQ por tUSD (dólar de teste) via pool de liquidez nativo. Taxa 0,1% apenas para swaps — transferências AEQ comuns completamente gratuitas.',
  'swap-priv-bar':'🔒 Apenas 0,1% de taxa · Transferências AEQ→AEQ gratuitas · tUSD é moeda de teste sem valor real',
  'swap-your-aeq':'Seu AEQ','swap-your-tusd':'Seu tUSD',
  'swap-fee-est':'Taxa de protocolo (0,1%)','swap-details-hdr':'Detalhes da Troca',
  'swap-out-lbl':'Você recebe (est.)','swap-impact-lbl':'Impacto no preço','swap-rate-lbl':'Taxa de câmbio',
  'swap-depth-lbl':'Composição do Pool','amm-title':'x × y = k — AMM de Produto Constante',
  'amm-text':'No swap, reservas AEQ aumentam e reservas tUSD diminuem — produto sempre igual a k. Swaps maiores causam maior impacto no preço.',
  'swap-btn-go':'🔄 TROCAR',
  'swap-log-hint':'// Conectar carteira para trocar...',
  'swap-no-liquidity':'Ainda sem tUSD?','swap-faucet-desc':'Humanos registrados podem reivindicar tUSD de teste uma vez','swap-btn-faucet':'💧 REIVINDICAR tUSD TESTE',
  'swap-addliq-title':'Fornecer Liquidez','swap-addliq-desc':'Seja o primeiro a depositar — sua proporção define o preço inicial.','swap-btn-addliq':'💧 ADICIONAR LIQUIDEZ',
  'swap-lp-title':'Sua Posição LP','swap-lp-share':'Cota do Pool','swap-lp-withdrawable':'Retirável',
  'swap-lp-pct-label':'% da sua posição','swap-lp-youget':'Você receberá','swap-btn-removeliq':'🔥 REMOVER LIQUIDEZ',
  'swap-pool-title':'AEQ / tUSD — Status do Pool',
  'swap-pool-aeq':'Reserva AEQ','swap-pool-tusd':'Reserva tUSD','swap-pool-price':'Preço Spot',
  'swap-fee-bps':'Taxa de Swap',
  'swap-pools-addr-title':'Endereços dos Pools Tokenômicos',
  'swap-validators':'Validadores (40%)','swap-lps':'Provedores de Liquidez (30%)','swap-ubi':'Pool UBI (20%)','swap-treasury':'Tesouro (10%)',
  'ubi-hero-title':'RENDA BÁSICA UNIVERSAL — POOL UBI',
  'ubi-hero-sub':'Acumulando — próximo pagamento distribuído a todos os humanos verificados em:',
  'ubi-bal-lbl':'saldo atual do pool','ubi-hero-desc':'Dividido igualmente · pago a cada 24h · pool zerado · saldo mínimo não necessário',
  'ubi-how-fills':'Como o Pool UBI se enche',
  'ubi-src-swap':'Taxas de Swap','ubi-src-swap-d':'Cada swap AEQ↔tUSD contribui 20% de suas taxas. Mais trading = enchimento mais rápido.',
  'ubi-src-dem':'Demurrage','ubi-src-dem-d':'AEQ inativo (3+ meses) decai 0,5%/mês. 20% do decaído vai para UBI.',
  'ubi-src-cap':'Excesso do Teto','ubi-src-cap-d':'Carteiras que excedem o teto são imediatamente cortadas. 20% flui para UBI.',
  'pools4-header':'Os quatro pools de redistribuição',
  'ubi-see-above':'ver contagem regressiva acima','ubi-timer-above':'⏰ contagem regressiva exibida acima','pool-t-timer':'Acumulando — sem temporizador',
  'usp-headline':'Pela primeira vez na história — todos começam em igualdade',
  'usp-sub':'Com um smartphone Android você é elegível. Sem banco, sem crypto, sem investimento.',
  'usp-c1-title':'R$ 0,00 de Investimento Inicial','usp-c1-desc':'Registro completamente sem taxas. Sem ETH, MATIC ou cartão. O protocolo paga todos os custos.',
  'usp-c2-title':'1.000 AEQ para cada humano','usp-c2-desc':'Bilionário ou agricultor — todos recebem exatamente 1.000 AEQ. Igualdade garantida matematicamente.',
  'usp-c3-title':'Acessível a todos','usp-c3-desc':'Sem conta bancária, sem cartão de crédito, sem documento de identidade, sem hardware adicional — apenas a câmara que o seu telemóvel Android já tem.',
  'usp-c4-title':'UBI diário para sempre','usp-c4-desc':'Após registrado, sua parte do UBI chega automaticamente todos os dias — sem nenhuma ação.',
  'v7-intro-title':'O que é AequitasV7?',
  'v7-intro-text':'AequitasV7 é o contrato inteligente central do protocolo Aequitas. Implantado de forma imutável na Aequitas Chain (ID 1926). Gerencia tudo: registro humano, verificação ZK, saldos, teto de riqueza, UBI, taxas de swap. Nenhum administrador pode modificá-lo.',
  'swap-sell-label':'Vender','swap-receive-label':'Receber',
  'guard-title':'🛡 Sistema Guardian','guard-my-lbl':'Meu Guardian','guard-none':'Nenhum',
  'guard-set-lbl':'Definir / Alterar Guardian','guard-set-hint':'Deve ser um humano registado na Aequitas · Bloqueio temporal de 7 dias · O Guardian só pode confirmar a sua vitalidade, não aceder a fundos · Máx. 3 protegidos por Guardian',
  'guard-confirm-lbl':'Confirmar Vivo (Como Guardian)','guard-confirm-hint':'Se o seu protegido não conseguir aceder à sua carteira, confirme a sua vitalidade para evitar que os fundos sejam transferidos para custódia após 910 dias de inatividade.','guard-recover-btn':'🔓 RECUPERAR DA CUSTÓDIA',
  'faq-title':'❓ Perguntas Frequentes','faq-q1':'Os meus dados biométricos estão seguros?','faq-a1':'O seu rosto é captado e enviado a serviços de comparação independentes — só assim é possível verificar «uma pessoa, uma conta». As imagens são processadas e depois descartadas; não são armazenadas. O que fica guardado é um modelo matemático: cifrado e dividido em partes por validadores operados separadamente, de modo que nenhum validador detém alguma vez um inteiro. Um limite honesto, dito e não escondido: o serviço que executa a comparação continua a guardar modelos, porque comparar exige-os.',
  'faq-q1b':'O registo prova que sou uma pessoa real e única?','faq-a1b':'Melhor do que uma chave de dispositivo alguma vez conseguiu, e ainda não demonstrável como número. O rosto é comparado com todos os registos existentes por serviços independentes que têm de concordar, por isso a mesma pessoa num segundo telemóvel é apanhada — algo que uma chave de dispositivo nunca conseguiu. Falta a taxa de erro: o limiar não está calibrado com captações reais, e isso exige cerca de 1000 pares de impostores.',
  'faq-q2':'Posso registar-me com uma carteira diferente mais tarde?','faq-a2':'Não. Um registo fica permanentemente ligado a um endereço de carteira. É intencional: o nullifier derivado do seu rosto é gasto uma única vez, por isso registar-se de novo noutra carteira seria uma segunda identidade para a mesma pessoa.',
  'faq-q3':'O que acontece se perder o telemóvel?','faq-a3':'Os seus AEQ permanecem na carteira — estão vinculados à sua chave privada, não ao telemóvel. Ainda pode aceder à carteira via MetaMask com a frase de recuperação. A recuperação da carteira é independente do registo biométrico.',
  'path-title':'Escolha o Seu Caminho','path-human-title':'Sou um Humano','path-human-desc':'Quero registar-me, receber 1.000 AEQ e juntar-me à rede de rendimento básico.','path-human-steps':'1. Descarregar a app Android Aequitas<br>2. Desbloquear com o bloqueio de ecrã do seu dispositivo (impressão digital/rosto/PIN)<br>3. Conectar MetaMask<br>4. Receber 1.000 AEQ instantaneamente',
  'path-node-title':'Sou um Operador de Node','path-node-desc':'Quero executar um node completo, participar na produção de blocos e ganhar do pool de validadores de 40%.','path-node-steps':'1. Registar como humano (obrigatório)<br>2. Nenhum ponto de entrada a configurar — os endereços dos validadores estão integrados<br>3. Implementar em Contabo/Hetzner/qualquer VPS<br>4. Ganhar diariamente do pool de validadores',
  'path-dev-title':'Sou um Desenvolvedor','path-dev-desc':'Quero construir no Aequitas, integrar a API ou contribuir para o protocolo.','path-dev-steps':'1. JSON-RPC compatível com EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Métricas: /metrics (Prometheus)',
  'story-flow-title':'Diagrama de Fluxo do Token AEQ','story-topo-title':'Topologia de Rede — Estado Atual',
  'swap-price-title':'AEQ / tUSD — Preço ao Vivo','swap-price-desc':'Preço em tempo real derivado das reservas do pool (x·y=k). Atualizado a cada 8 segundos.','swap-price-empty':'Sem dados do pool ainda — adicione liquidez para ver o gráfico de preços.',
  'node-guide-lang-note':'Este guia inline está em inglês. Um PDF traduzido na sua língua está disponível através do botão acima.',
  'k-zkp':'Sistema ZKP','k-hash':'Sistema Hash','k-sybil-prot':'Proteção Sybil',
  'soc-title':'💬 Redes Sociais','soc-sub':'Anúncios, o estado da cadeia e as perguntas incômodas &mdash; em público, em ambas.',
  'soc-x-desc':'Anúncios, e o que a cadeia está realmente fazendo. Formato curto.','soc-tg-desc':'O grupo aberto: perguntas, operadores de nós e ajuda para se registrar.',
  's-validators':'Validadores Ativos',
  'expl-heading':'Explorador de Blocos',
},
ar:{
  'x-consensus-ghostdag-knightdag':'◆ التوافق: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'شِفرة العقد',
  'x-demurrage-is-a-holding-cost':'رسم الركود كلفةٌ على الاحتفاظ بالمال — فائدة سالبة تجعل الاكتناز مكلفاً والتداول مُغرياً. وله سابقة تاريخية: تجربة فيرغل (النمسا، 1932) استعملت عملة من هذا النوع فخفَّضت البطالة المحلية 25 % خلال سنة. وأوقفها البنك الوطني النمساوي تحديداً لأنها نجحت أكثر من اللازم وهدَّدت احتكار المصارف. ويقوم الـ Chiemgauer (ألمانيا، 2003) على المبدأ نفسه ويتداول بنجاح منذ أكثر من عشرين عاماً. تطبِّق Aequitas رسماً مستمراً قدره 0.5 % شهرياً، ولا يبدأ إلا بعد ثلاثة أشهر من الخمول.',
  'x-network-consensus':'← الشبكة / التوافق',
  'x-node-decentralization-roadmap':'خارطة لامركزية العُقد',
  'x-open-source-chain-logic':'منطق السلسلة مفتوح المصدر',
  'x-phase-0-now':'المرحلة 0 (الآن):',
  'x-phase-1-100-humans':'المرحلة 1 (أكثر من 100 شخص):',
  'x-phase-2-1-000-humans':'المرحلة 2 (أكثر من 1,000 شخص):',
  'x-phase-3-10-000-humans':'المرحلة 3 (أكثر من 10,000 شخص):',
  'x-protocol-mechanisms':'آليات البروتوكول',
  'x-what-happens-to-aeq-when':'ماذا يحدث لـ AEQ حين يموت المرء أو يفقد أهليته نهائياً؟ في بيتكوين ومعظم العملات المشفَّرة، المحفظة المفقودة تعني معروضاً مفقوداً إلى الأبد — إذ يُقدَّر أن ملايين البيتكوين تعذَّر الوصول إليها للأبد. تعالج Aequitas ذلك عبر استرداد متعدّد المراحل عند الخمول: إذا لم تُظهر محفظة أي نشاط لمدة طويلة، يعود رصيدها تدريجياً إلى الجماعة عبر صندوق الدخل الأساسي، بحيث يظل المعروض المتداول فعلياً ذا معنى.',
  'x-what-if-someone-is-hospitalized':'وماذا لو دخل أحدهم المستشفى أو السجن أو تعذَّر عليه الوصول إلى جهازه شهوراً؟ يتيح نظام الشخص المؤتمَن لإنسان آخر مُتحقَّق منه أن يؤكِّد أن صاحب المحفظة ما زال حياً، فلا تنتقل عملاته إلى الحجز. وهذا الشخص لا يملك أي صلاحية مالية على الإطلاق: كل ما يستطيعه هو استدعاء دالة واحدة تُصفِّر مؤقّت الخمول. ولا يمكنه بأي حال تحريك الأموال أو إنفاقها أو الاطلاع عليها.',
  'bv-bind':'🔗 إنشاء توقيع الربط',
  'bv-check-d':'الاستدعاء الثاني يسرد كل مُحقِّق ويقارن بينهم: هل يحملون العدد نفسه من التسجيلات، هل ينقص أحدهم بذرة، وهل تتوافق المفاتيح. فإن أظهر مدخلك تبايناً، فمعرفة ذلك هنا خير من معرفته في منتصف تسجيل أحدهم.',
  'bv-check-t':'التأكد من أنه يعمل',
  'bv-desc':'العُقدة التي تُنتج الكتل تحمي <strong style="color:var(--text)">السجل</strong>. أما المُحقِّق الحيوي فيحمي شيئاً آخر: الوعد بأن <strong style="color:var(--neon)">كل إنسان يسجّل مرة واحدة فقط</strong>. هذان دوران منفصلان — يمكنك القيام بأحدهما أو بكليهما على الجهاز نفسه.',
  'bv-guide-sub':'خطوة بخطوة &middot; لا تلزم معرفة بالتعمية &middot; نحو 30 دقيقة، معظمها تنزيل',
  'bv-honest-d':'هذا الجزء في طور التجربة وحدوده حقيقية. المقارنة المشتركة تستهلك مادة تعمية تُستعمل مرة واحدة، ودفعة واحدة تكفي حالياً لبضع عشرات من التسجيلات قبل أن يلزم المزيد — أي أن المسار السرّي يثبت نفسه أولاً على نطاق صغير لا على الملايين. كما يزداد العمل بازدياد عدد المسجَّلين. ننشر هذه الأرقام بدل تدويرها: نظام يطلب وجهك لا يحق له أن يكون غامضاً بشأن ما يقدر عليه وما لا يقدر عليه بعد.',
  'bv-honest-t':'أين نقف اليوم — بصراحة',
  'bv-need-1':'<strong style="color:var(--text)">حساب Aequitas مسجَّل.</strong> القاعدة نفسها كما في إنتاج الكتل، وللسبب نفسه: إنسان واحد، مفتاح واحد. من دونها يستطيع شخص واحد أن يصير لجنة كاملة في صمت.',
  'bv-need-2':'<strong style="color:var(--text)">خادم لينكس صغير عليه Docker.</strong> تكفي ذاكرة 2 غيغابايت. لا حاجة لبطاقة رسوميات — المقارنة حساب على 64 بايت. الجهاز الذي يشغّل عقدتك أصلاً يفي بالغرض.',
  'bv-need-3':'<strong style="color:var(--text)">اسم نطاق مع HTTPS.</strong> يجب أن يصل إليك بقية أعضاء اللجنة. يكفي نطاق فرعي لشيء تملكه بالفعل.',
  'bv-need-4':'<strong style="color:var(--text)">أن تبقى متصلاً.</strong> لكي يكتمل تسجيل ما، على كل عضو في اللجنة أن يجيب. المُحقِّق كثير الغياب يُبطئ الناس بدل أن يحميهم.',
  'bv-need-t':'قبل أن تبدأ — ما تحتاج إليه',
  'bv-s1-note':'احتفظ بالنصف الخاص على خادمك ولا مكان سواه. أما النصف العام فمُعدٌّ للمشاركة — به يتحقق الآخرون من أنك شهدت بشيء. <strong style="color:var(--text)">بذرة الإسقاط الخاصة بك مهمة:</strong> لأن كل مُحقِّق يستعمل بذرة مختلفة، فقاعدة بيانات مسروقة من أحدهم لا تُقارَن بقاعدة آخر. وإن ضاعت البذرة فقدت حصصك المخزَّنة معناها، فاحفظ نسخة في مكان تتحكم به.',
  'bv-s1-t':'الخطوة 1 — أنشئ مفاتيحك بنفسك',
  'bv-s1-warn-d':'مُحقِّقان يحملان السرّ نفسه يُحسبان واحداً، وتصبح اللجنة أصغر مما تبدو. لا أحد — ونحن منهم — ينبغي أن يرسل إليك مفتاحاً.',
  'bv-s1-warn-t':'أنشئها بنفسك. ولا تقبل مفاتيح من أحد أبداً.',
  'bv-s2-d':'ضع قيم الخطوة 1 في ملف لا يقرؤه سواك. قيمة واحدة في كل سطر، بلا علامات اقتباس.',
  'bv-s2-note':'<strong style="color:var(--gold)">اترك ALLOW_REAL_BIOMETRIC_DATA على false</strong> حتى تقرأ ملاحظات حماية البيانات. وهي مُعطَّلة، ينضم مُحقِّقك إلى الشبكة ويشارك في تسجيلات اختبارية دون أن يخزّن يوماً بيانات شخص حقيقي. هذه هي البداية الصحيحة، ولا عجلة في تغييرها.',
  'bv-s2-t':'الخطوة 2 — اكتب ملف الإعدادات',
  'bv-s3-note':'الإجابة السليمة تُبلغ عن <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> و<span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span>. الأولى هي الادعاء بأن لا قالب كامل يُخزَّن، في صيغة تستطيع فحصها بنفسك بدل تصديقها. افحصها الآن ثم افحصها لاحقاً — فهي ضمانك أنت بقدر ما هي ضمان غيرك.',
  'bv-s3-t':'الخطوة 3 — شغِّل المُحقِّق',
  'bv-s4-d':'يصل إليك بقية أعضاء اللجنة عبر الإنترنت العام، فلا يجوز أن يبقى المنفذ مكشوفاً بلا تعمية. وCaddy يحصل على الشهادة من تلقاء نفسه.',
  'bv-s4-t':'الخطوة 4 — ضع HTTPS في المقدمة',
  'bv-s5-d':'يربط منتجو الكتل مفتاح توقيعهم بمحفظة إنسان مسجَّلة: توقّع المحفظة <strong style="color:var(--text)">Aequitas: authorize validator &lt;العنوان&gt;</strong>، وبدون ذلك ترفض السلسلة المقعد. الزر أدناه ينتج هذا التوقيع تحديداً — لدور المُصادِق. <strong style="color:var(--text)">مفتاح المُحقِّق ليس له هذا الربط بعد.</strong> يُجمَع نصفه العام خارج السلسلة (الخطوة 6) ويُضاف إلى القائمة التي يفحصها كل خادم إثبات. لا شيء على السلسلة يربطه بشخص. وإلى أن يوجد ذلك، تَعُدّ اللجنة آلات لا أشخاصاً، وقد يملك مشغّل واحد عدة مفاتيح. نفضّل قول ذلك هنا على أن يبدو الرقم أقوى مما هو عليه.',
  'bv-s5-t':'الخطوة 5 — ما الذي يربط مفتاحاً بإنسان (وما الذي لا يربطه بعد)',
  'bv-s6-d':'أرسل إلى المجموعة النصف <strong style="color:var(--text)">العام</strong> من الخطوة 1 مع عنوانك على HTTPS. يُضاف إلى القائمة التي يراجعها كل خادم برهان، ومن ثمّ تُحتسب شهاداتك ضمن النصاب. لا يغادر جهازك في هذه الخطوة أي سرّ — وهذا هو مغزى الفصل: النصف الخاص يبقى معك إلى الأبد، والنصف العام لا قيمة له بدونه.',
  'bv-s6-t':'الخطوة 6 — انشر مفتاحك العام',
  'bv-status-d':'شِفرة المُحقِّق <strong style="color:var(--text)">ليست علنية بعد</strong>، لذا لا يستطيع الجميع اليوم إتمام الخطوات أدناه. ننشرها مع ذلك لأن التصميم ينبغي أن يكون قابلاً للفحص قبل التشغيل لا بعده. إن أردت تشغيل واحد، اسأل في مجموعة تيليجرام المرتبطة في الصفحة الرئيسية. فتحُ هذا المستودع هو ما سيحوّل هذا الدليل من خطة إلى دعوة، وهو ما ندين لكم به تالياً.',
  'bv-status-t':'الحالة: نسخة تجريبية مغلقة — اقرأ هذا قبل أن تبدأ',
  'bv-title':'أو كن مُحقِّقاً حيوياً — الدور الذي يجعل التفرّد لامركزياً',
  'bv-what-d':'لا يُرسَل إليك وجه قط. جهازك يخزّن <strong style="color:var(--text)">حصة جمعية</strong> من خلاصة بحجم 64 بايت: وحدها لا تُميَّز عن ضجيج عشوائي، ولا حساب في متناولك يستعيد منها وجهاً. تجري المقارنات بالاشتراك مع بقية أعضاء لجنتك، ولا يعرف أحدكم شيئاً سوى الجواب — <em>مكرَّر: نعم أم لا</em>. هذا ليس وعداً بحسن نوايانا، بل خاصية في الحساب نفسه.',
  'bv-what-t':'ما الذي ستحتفظ به — وما الذي لن تراه أبداً',
  'bv-why-d':'لا يُقبل التسجيل إلا بعد أن يشهد به <strong style="color:var(--text)">عدة مُحقِّقين مختلفين</strong>. فمفتاح واحد مسروق لا يكفي — يحتاج المهاجم إلى لجنة كاملة. ولأن <strong style="color:var(--neon)">الإنسان الواحد لا يملك إلا مفتاح مُحقِّق واحداً</strong>، فشراء لجنة يعني أن تكون ذلك العدد من البشر. مع 100 مُحقِّق، من يسيطر على 10 منهم فرصته أقل من واحد في الألف لامتلاك لجنة ثلاثية كاملة. كل شخص ينضم يُصغِّر هذا الرقم. هذا هو الموضع الوحيد الذي يكون فيه عدد المشاركين <em>هو</em> الأمان. <strong style="color:var(--text)">يفترض هذا الحساب إنساناً واحداً لكل مفتاح مُحقِّق.</strong> في إنتاج الكتل تفرض السلسلة ذلك؛ أما لمفاتيح المُحقِّقين فلا تفرضه بعد (الخطوة 5). وحتى ذلك الحين، الرقم أعلاه حدٌّ أعلى للأمان لا قياسٌ له.',
  'bv-why-t':'لماذا يجعل كل مُحقِّق إضافي إفسادَ الشبكة أصعب',
  'x-0-1-split-40-30':'0.1 % · توزيع 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 شخص. سقف ثروة متحرك 5x &#8594; 25x. مرحلة التأسيس.',
  'x-0-8211-2-years':'0 &#8211; سنتان',
  'x-0-perfect-equality':'0 = مساواة تامة',
  'x-1-000-aeq-minted':'+1,000 AEQ مُصدَرة',
  'x-1-000-aeq-per-human':'1,000 AEQ لكل شخص',
  'x-1-000-aeq-will-be':'ستُضاف 1,000 AEQ تلقائياً',
  'x-10-000-8211-1m-humans':'10,000 &#8211; مليون شخص. عشر عُقد على الأقل. لامركزية كاملة.',
  'x-100-8211-10-000-humans':'100 &#8211; 10,000 شخص. سقف ثابت 25x. انضمام مفتوح للعُقد.',
  'x-100-maximum-concentration':'100 = أقصى تركّز',
  'x-1m-humans-global-ubi-at':'أكثر من مليون شخص. دخل أساسي عالمي على نطاق واسع. هدف جيني &lt;0.30.',
  'x-9679-liquidity-lp-30':'&#9679; السيولة LP ‏30 %',
  'x-9679-treasury-10':'&#9679; الاحتياطي 10 %',
  'x-9679-ubi-pool-20':'&#9679; صندوق الدخل الأساسي 20 %',
  'x-9679-validators-40':'&#9679; المُحقِّقون 40 %',
  'x-active-validators':'المُحقِّقون النشطون',
  'x-add-aequitas-chain-to-metamask':'أضِف سلسلة Aequitas إلى MetaMask لترى رصيدك من AEQ، وترسل المعاملات، وتتعامل مع عقد V7 مباشرةً من متصفحك أو محفظتك على الهاتف.',
  'x-admin-keys-or-governance-votes':'مفاتيح إدارية أو تصويتات حوكمة',
  'x-aeq-activity':'حركة AEQ',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'BlockDAG في Aequitas — لا شيء يذهب هدراً',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'سلسلة Aequitas ‏(BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'تُنفِّذ Aequitas ذلك رياضياً. كل شخص مُتحقَّق منه يتلقى 1,000 AEQ بالضبط &#8212; مليارديراً كان أم مزارع كفاف، بلا استثناء. أربع آليات لإعادة التوزيع تمنع تراكم عدم المساواة بلا حدود. ويُسجَّل معامل جيني على السلسلة لحظةً بلحظة.',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — سلسلة إثبات الإنسانية',
  'x-android-apk-direct-download':'‏APK لأندرويد · تنزيل مباشر',
  'x-architecture':'البنية',
  'x-automatic-on-chain':'تلقائي على السلسلة',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'‏BlockDAG (رسم بياني موجَّه بلا دورات)',
  'x-blockdag-parallel-production':'‏BlockDAG · إنتاج متوازٍ',
  'x-blockdag-proof-of-humanity':'‏BlockDAG + إثبات الإنسانية',
  'x-blue-score':'«الدرجة الزرقاء»',
  'x-both-blocks-are-kept-ghostdag':'يُحتفَظ بالكتلتين معاً — إذ يُدمِج GHOSTDAG الكتلة المتزامنة ويظل يحتسبها ضمن الترتيب المعتمد.',
  'x-canonical-winner':'الفائز المعتمد',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'مماثل للولايات المتحدة (0.41) أو فرنسا (0.32). ضمن نطاق معظم الاقتصادات المتقدمة. وإعادة التوزيع تُسطِّح المنحنى فعلياً.',
  'x-confirm-ward-is-alive':'✓ تأكيد أن الشخص على قيد الحياة',
  'x-core-technology':'التقنية الأساسية',
  'x-daily-ubi-returns-to-all':'الدخل الأساسي اليومي يعود إلى كل الأشخاص المُتحقَّق منهم',
  'x-demurrage-0-5-mo':'رسم الركود (0.5 % شهرياً)',
  'x-device-bound-zk-proof-one':'إثبات ZK مرتبط بالجهاز · تسجيل واحد لكل جهاز',
  'x-diagonal-line-perfect-equality':'الخط القُطري = مساواة تامة',
  'x-disconnect-wallet':'⊘ فصل المحفظة',
  'x-distinct-proposers-recent-blocks':'مُنتِجون مختلفون، كتل حديثة',
  'x-distribution':'📈 التوزيع',
  'x-elliptic-curve':'منحنى إهليلجي',
  'x-entire-distribution':'التوزيع بأكمله',
  'x-evm-compatible':'متوافق مع EVM',
  'x-fill-ghostdag-verdict-thin-ring':'التعبئة = حكم GHOSTDAG · الحلقة الرفيعة = المُنتِج · عمود لكل ارتفاع. مرِّر المؤشر فوق أي كتلة للتفاصيل.',
  'x-generate-node-binding-signature':'🔗 إنشاء توقيع الربط',
  'x-run-a-coordinator':'🚪 تشغيل منسّق',
  'co-title':'أو شغّل منسّقًا — الباب الذي يمرّ منه كل إنسان',
  'co-desc':'المنسّق هو المكان الذي يصل إليه الإنسان: يصدر التحدي، ويوزّع اللقطة على المدققين، ويحصي أصواتهم، ويصدر الشهادة التي تسكّ السلسلة بناءً عليها. لمدة طويلة كان هناك منسّق واحد بالضبط — أي أن كل تسجيل في الشبكة كان يمرّ عبر جهاز واحد. لا لأن شيئًا كان ناقصًا، بل لأن أحدًا لم يشغّل ثانيًا.',
  'co-status-t':'الحالة: نسخة تجريبية مغلقة — التحفّظ نفسه كما في المدقق',
  'co-status-d':'يوجد المنسّق في المستودع نفسه الذي يوجد فيه المدقق، وهذا المستودع <strong style="color:var(--text)">ليس عامًّا بعد</strong>. لذلك لا يستطيع الجميع اليوم إتمام الخطوات أدناه. وهي منشورة رغم ذلك، للسبب نفسه: ينبغي أن يكون التصميم قابلًا للفحص قبل النشر لا بعده.',
  'co-power-t':'ما يستطيعه المنسّق — وما لا يستطيعه',
  'co-power-d':'<strong style="color:var(--text)">لا يستطيع اختلاق إنسان</strong>. لا يوجد bio_hash قبل أن يشهد عليه عدة مدققين مختلفين، والمنسّق لا يملك أيًّا من مفاتيحهم. ما يستطيعه هو ربط bio_hash <strong style="color:var(--text)">قائم</strong> بمحفظة — فقد يحوّل منسّق غير أمين مخصّصًا إلى عنوان يختاره. هذه سلطة حقيقية، تكبر مع كل منسّق يُضاف، ومن يوازن مسألة الثقة ينبغي أن يعرف الفرق.',
  'co-safe-t':'لماذا يكون منسّق ثانٍ آمنًا أصلًا',
  'co-safe-d':'لم يكن كذلك دائمًا. حتى آب/أغسطس 2026 كان وعد <strong style="color:var(--text)">إنسان واحد، تسجيل واحد</strong> معلّقًا بقفل Redis داخل المنسّق — ومنسّقان مستقلان لا يتشاركان Redis، فكان تسجيلان متزامنان للشخص نفسه يمرّان كلاهما. أما الآن <strong style="color:var(--text)">فكل مدقق يفحص بنفسه</strong>، قبل كتابته الخاصة، ما إذا كان هذا الوجه مسجَّلًا. لم يعد الضمان معتمدًا على خدمة مشتركة ولا سرّ مشترك، فيمكن لمنسّق أن ينضم أو يزول دون أن يتغير شيء.',
  'co-need-t':'ما تحتاج إليه',
  'co-need-d':'حساب Aequitas مسجَّل — القاعدة نفسها كما في إنتاج الكتل والتدقيق: إنسان واحد، مفتاح واحد. خادم عليه Docker وعنوان HTTPS عام، لأن المتصفحات لا تسلّم الكاميرا لصفحة غير آمنة. ومفتاحان خاصان بك تولّدهما بنفسك ولا يغادران جهازك أبدًا: أحدهما يوقّع شهاداتك، والآخر يحوّل عناوين المحافظ إلى علامات.',
  'co-keys-t':'لا تقبل مفتاحًا من أحد أبدًا — بمن فينا نحن',
  'co-keys-d':'منسّقان يتشاركان مفتاح توقيع واحد ليسا منسّقين اثنين، بل واحدًا بعنوانين، ويبدو النصاب الذي يُفترض أن يحمي الناس مستوفىً دون أن يكون كذلك. ولّد المفتاحين على جهازك، بعشوائيتك أنت، ولا تدع أيًّا منهما يخرج.',
  'co-auth-t':'تفويض مفتاحك — دون إذن من أحد',
  'co-auth-d':'ما دام مفتاحك غير مفوَّض، يرفض المدققون كل ما يوقّعه. يتطلب التفويض إثباتين ولا يتطلب موافقة أحد: محفظتك توقّع بأن وراء هذا المفتاح إنسانًا مسجَّلًا، ومنسّقك يثبت على خادمه أن المفتاح ملكه فعلًا. الأول تُنتجه بالزر أعلاه، والثاني ينتجه منسّقك وحده. حتى آب/أغسطس 2026 كنت تحتاج أيضًا إلى سرّ مشترك منّا — وهذا السرّ <em>كان</em> هو الإذن. لقد زال.',
  'co-pernode-t':'السجل خاص بكل عقدة، وهذا مقصود',
  'co-pernode-d':'التفويض المكتوب على عقدة لا ينتقل إلى غيرها — لا توجد معاملة لذلك ولا بثّ. قائمة ثقة مُستنسخة ستكون بالضبط السلطة المركزية التي بُني هذا النظام بدونها: كل مشغّل يقرر بنفسه شهادات مَن تقبلها عقدته. الثمن أن تفويضك يجب أن يُرسل إلى كل عقدة يُراد أن تعترف به. أما التوقيع نفسه فقابل للنقل: توقّع مرة واحدة وترسله إلى كل مكان؛ والعقدة التي تتخطاها ستظل ترفضك.',
  'co-law-t':'ما تعرفه عن الآخرين — وما يترتب عليه',
  'co-law-d':'تمرّ اللقطة من خلالك؛ تمرّرها ولا تحتفظ بشيء. لكنك وحدك تملك الربط بين عنوان المحفظة والعلامة لمن يسجّلون عبرك — ولهذا يجب أن يبقى مفتاح العلامات لك وحدك: لو شورك، لاستطاع أي مشغّل أن يحسب العلامة لأي عنوان عام ويعرف صاحب ذلك الوجه. ويعني ذلك أيضًا أنك تصبح <strong style="color:var(--text)">المتحكم بالبيانات</strong> لهؤلاء الأشخاص بموجب اللائحة العامة لحماية البيانات. لا نحن. طلبات الاطلاع والمحو والاعتراض تصل إليك، وليست هذه شكليات.',
  'co-limit-t':'القيد الوحيد الناتج عن ذلك',
  'co-limit-d':'المحو بعنوان المحفظة لا ينجح إلا لدى المنسّق الذي تمّ عنده التسجيل: علامتك معلّقة بمفتاحك، ومنسّق آخر يشتق علامة مختلفة للعنوان نفسه. لذلك فإن «غير موجود» من مكان آخر تعني «غير مسجَّل هنا» لا «غير مسجَّل» — والجواب يقول ذلك صراحةً. أما الطريق عبر bio_hash الخاص بالشخص، ذاك الذي يملكه هو ولا يحتاج إلى مشغّل، فيعمل لدى كل منسّق، لأن هذا المعرّف يبقى نفسه.',
  'x-authorize-coordinator-key':'🔑 تفويض مفتاح المنسّق',
  'x-ghostdag-2018-one-true-order':'‏GHOSTDAG ‏(2018) — ترتيب واحد صحيح من رسم بياني متشابك',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'معامل جيني',
  'x-gini-coefficient-0-1':'معامل جيني ‏(0–1)',
  'x-gini-index-history':'تاريخ مؤشر جيني',
  'x-gini-target-scandinavian-level':'هدف جيني (المستوى الإسكندنافي)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'‏Groth16 ZKP (معرفة صفرية)',
  'x-guardian-system-8212-human-failsafe':'الشخص المؤتمَن &#8212; ضمانة بشرية للمحافظ المفقودة',
  'x-hash-wallet':'البصمة / المحفظة',
  'x-healthier-than-most-nations-on':'أفضل حالاً من معظم دول الأرض. مماثل لإسكندنافيا (0.27) وألمانيا (0.31). سقف الثروة ورسم الركود يحافظان على توزيع عادل.',
  'x-higher-than-most-european-nations':'أعلى من معظم الدول الأوروبية — مماثل للبرازيل (0.53) أو روسيا. إعادة التوزيع تعمل بشدة مرتفعة.',
  'x-honest-limitation':'قيدٌ نُقرّ به:',
  'x-how-it-works':'كيف يعمل',
  'x-how-to-read-this-chart':'كيف تقرأ هذا الرسم:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'شخصاً يمكنهم التسجيل',
  'x-imagine-a-world-where-every':'«تخيَّل عالماً يتلقى فيه كل إنسان على الأرض &#8212; بغضّ النظر عن مكان ولادته أو لغته أو ثروة والديه &#8212; دخلاً يومياً مضموناً لمجرد كونه إنساناً. لا صدقةً، بل حقاً رياضياً تفرضه شِفرة لا تستطيع حكومة ولا شركة تجاوزها.»',
  'x-inactive-escrow':'حجز عند الخمول',
  'x-inactivity-timeline':'الجدول الزمني للخمول',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'‏keccak256 (آمن بعد الكَمّ)',
  'x-key-protections':'الحمايات الأساسية:',
  'x-knightdag-2026-aequitas-s-own':'◆ ‏KNIGHTDAG ‏(2026) — تطوير Aequitas الخاص متجاوزاً GHOSTDAG ذا الـ K الثابت',
  'x-knightdag-secured':'· محميّ بـ KnightDAG',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'مثل إسكندنافيا (~0.27)',
  'x-liquidity-pool-30':'مجمع السيولة (30 %)',
  'x-loading-blocks':'جارٍ تحميل الكتل…',
  'x-loading-topology':'جارٍ تحميل بنية الشبكة…',
  'x-loading-transactions':'جارٍ تحميل المعاملات…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'منحنى لورنز — توزيع AEQ بين الأشخاص',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask للهاتف: إذا ظهر رصيد AEQ صفراً بعد التسجيل، فاذهب إلى الإعدادات ← الشبكات ← احذف سلسلة Aequitas ← ثم أضِفها من جديد عبر هذا الموقع',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask للهاتف: إذا ظهر AEQ صفراً بعد الإضافة، فاحذف الشبكة وأضِفها ثانيةً بالزر أعلاه.',
  'x-money-exists-because-people-exist':'المال موجود لأن الناس موجودون. إذن ينبغي أن يكون لكل إنسان نصيب متساوٍ منه، لمجرد كونه إنساناً.',
  'x-money-exists-because-people-exist-2':'«المال موجود لأن الناس موجودون. لا أكثر ولا أقل.»',
  'x-most-unequal-currency-ever':'أكثر عملة تفاوتاً على الإطلاق',
  'x-multi-validator-network':'شبكة بعدة مُحقِّقين',
  'x-n-lt-10-not-yet':'⚠ ‏N&lt;10: غير ذي دلالة بعد',
  'x-no-snapshots-yet-first-one':'لا سجلات بعد — سيُحفَظ الأول بعد التوزيع القادم.',
  'x-no-stake-blockchain':'سلسلة كتل بلا رهان',
  'x-node-operator-guide-pdf':'📄 دليل مُشغِّل العُقدة ‏(PDF)',
  'x-node-operator-wallet-must-be':'يجب أن يكون NODE_OPERATOR_WALLET شخصاً مسجَّلاً في Aequitas',
  'x-one-human-one-wallet-1':'إنسان واحد = محفظة واحدة = 1,000 AEQ',
  'x-p2p-protocol':'بروتوكول P2P',
  'x-paid-out-daily':'يُصرَف يومياً',
  'x-permanent-on-chain':'دائم · على السلسلة',
  'x-phase-roadmap-8212-the-path':'خارطة المراحل &#8212; الطريق إلى النطاق العالمي',
  'x-phase-transitions-are-automatic-8212':'الانتقال بين المراحل تلقائي &#8212; تُطلِقه عتبات عدد الأشخاص ويُنفِّذه العقد. بلا تصويت وبلا مفتاح إداري.',
  'x-planned-post-beta':'مُخطَّط (بعد النسخة التجريبية)',
  'x-postgresql-persistent':'‏PostgreSQL (تخزين دائم)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'وفِّر سيولة AEQ / tUSD لتكسب 30 % من كل رسوم التبادل، تُوزَّع يومياً.',
  'x-recorded-after-each-ubi-distribution':'يُسجَّل بعد كل توزيع للدخل الأساسي، ويُظهر كيف تتطور المساواة مع نمو الشبكة. كلما انخفض كان أفضل — والهدف جيني دون 0.30.',
  'x-redistribution':'إعادة التوزيع',
  'x-run-a-node':'⚙️ شغِّل عُقدة',
  'x-run-a-verifier':'⚙️ تشغيل مُحقِّق',
  'x-set-guardian':'🛡 تعيين شخص مؤتمَن',
  'x-swap-fees-0-1':'رسوم التبادل ‏(0.1 %)',
  'x-sybil-resistance-8212-current-state':'مقاومة سيبيل &#8212; الوضع الحالي بصراحة',
  'x-the-4-redistribution-mechanisms':'آليات إعادة التوزيع الأربع',
  'x-the-core-innovation':'الفكرة الجوهرية',
  'x-the-matching-threshold-has-not':'لم تُعايَر عتبة المطابقة بعد على صور حقيقية',
  'x-the-vision-8212-a-global':'الرؤية &#8212; بروتوكول عالمي للدخل الأساسي',
  'x-the-year-is-2009-satoshi':'نحن في عام 2009. يُطلِق ساتوشي ناكاموتو بيتكوين. ولأول مرة تنتقل القيمة بين أي شخصين دون مصرف. ثورة حقيقية. لكن سرعان ما يختل شيء ما.',
  'x-this-is-not-a-0815':'ليست هذه سلسلة كتل عادية تُنتِج كتلة واحدة في كل مرة. تُشغِّل Aequitas سلسلة BlockDAG حقيقية، يرتّبها GHOSTDAG — ومنذ 2026 يحميها KnightDAG، وهو تطويرها التكيفي الخاص. على هذه الآلية يعتمد في نهاية المطاف كل رصيد وكل دفعة وكل سقف ثروة، كي يوجد تاريخ واحد متفق عليه.',
  'x-today-beta':'اليوم (تجريبي)',
  'x-today-this-verifies-one-device':'اليوم يتحقق هذا من جهاز، لا من شخص واحد بعينه بعد',
  'x-traditional-blockchain-wasted-work':'سلسلة الكتل التقليدية — جهد ضائع',
  'x-treasury-10':'الاحتياطي ‏(10 %)',
  'x-trusted-verified-human':'شخص مُتحقَّق منه وموثوق',
  'x-two-validators-produce-at-once':'يُنتِج مُحقِّقان في الوقت نفسه ← يفوز أحدهما ويُطرَح الآخر — جهد ضائع، وهو يحدّ من السرعة التي يمكن للشبكة بلوغها بأمان.',
  'x-ubi-pool-20':'صندوق الدخل الأساسي ‏(20 %)',
  'x-validators-pool-40':'صندوق المُحقِّقين ‏(40 %)',
  'x-view-source-on-github':'🐙 اطّلع على الشِفرة في GitHub',
  'x-wealth-cap-multiplier-bootstrap-slider':'مُضاعِف سقف الثروة — مؤشر مرحلة الانطلاق',
  'x-wealth-cap-overflow':'الفائض عن سقف الثروة',
  'x-wealth-distribution-analysis':'تحليل توزيع الثروة',
  'x-what-happens-when-someone-is':'ماذا يحدث إذا دخل أحدهم المستشفى أو السجن أو تُوفِّي؟ في معظم أنظمة العملات المشفَّرة، المحفظة المفقودة مفقودة إلى الأبد. أما Aequitas فلديها استرداد عند الخمول من ثلاث طبقات.',
  'x-what-is-a-guardian':'ما الشخص المؤتمَن؟',
  'x-what-is-and-is-not':'ما هو خاص وما ليس كذلك:',
  'x-what-would-a-cryptocurrency-look':'«كيف تبدو عملة مشفَّرة صُمِّمت منذ البداية لتكون عادلة مع كل إنسان؟»',
  'x-why-a-normal-blockchain-isn':'لماذا لا تكفي سلسلة كتل عادية',
  'x-worse-than-any-country-on':'أسوأ من أي دولة على الأرض (رقم جنوب أفريقيا القياسي: 0.63). ويقترب من بيتكوين (0.85). البروتوكول عند أقصى تدخّل — السقف وإعادة التوزيع بكامل قوتهما.',
  'x-year-2-180d':'السنة 2 +180 يوماً',
  'x-zk-device-key-proof':'إثبات ZK لمفتاح الجهاز',
  'swap-price-flat':'لا صفقات في هذه الفترة — لم يتحرك السعر. الرسم البياني يعمل، لكن السوق هادئ.',
  'mpc-optin-title':'اختياري — المساعدة في كشف التسجيلات المكررة (جاهز، لم يُفعّل بعد)',
  'mpc-optin-desc':'جاهز، لكنه لم يُفعّل بعد. لاحقاً سيتمكن نظيرك من المساعدة في التحقق من عدم تسجيل أي شخص مرتين دون أن يرى أي بيانات حيوية: كل طرف مشارك يحتفظ بحصة رياضية واحدة فقط من كل قالب — وهي بمفردها مجرد ضجيج — ويقارنون لقطة جديدة معاً، فلا تستطيع أي آلة منفردة إعادة بناء شيء. أما اليوم فهذا المسار لا يقرر شيئاً: فحص التكرار لا يمر عبره، واللجنة قائمة ثابتة وليست مسحوبة تلقائياً.',
  'mpc-optin-note':'يحتوي ملف الحصص على عشوائية تُستخدم مرة واحدة ولا يجوز أن يحتفظ بها سوى نظيرك — لا تنسخه أبداً إلى جهاز آخر ولا تودعه في أي مستودع. يجب حالياً أن يأتي من المشغّل، وهذه هي التبعية المركزية المتبقية. ولا تحتاج إلى مفتاح جديد: يعرّف نظيرك نفسه بالمفتاح ذاته الذي يوقّع به الكتل أصلاً.',
  'logo-sub':'إثبات الإنسانية','live':'مباشر',
  'reg-title':'🔐 التسجيل كإنسان موثق',
  'reg-sub':'انضم إلى شبكة Aequitas واحصل على منحة دخل أساسي شامل تبلغ 1,000 AEQ. التسجيل لمرة واحدة، دائم، ومجاني تماماً. لا يتم تخزين أي بيانات شخصية.',
  'app-title':'التسجيل عبر تطبيق أندرويد',
  'app-text':'عند التسجيل تلتقط الكاميرا وجهك ومقطعاً قصيراً للتحقق من الحيوية. تتحقق خدمات مطابقة مستقلة من وجود شخص حي ومن أن هذا الوجه غير مسجَّل بالفعل؛ ويجب أن تتفق بالنصاب. ثم ينقل إثبات Groth16 النتيجة إلى السلسلة دون كشف أي شيء عنك. تُضاف <strong style="color:var(--gold)">1000 AEQ تلقائياً</strong> بعد التحقق. <strong style="color:var(--gold)">ملاحظة:</strong> لم تُعايَر عتبة المطابقة بعد على عمليات التقاط حقيقية — انظر الأسئلة الشائعة أدناه.',
  's1t':'التقاط الوجه','s1d':'يسجّل التطبيق وجهك ومقطعاً قصيراً للتحقق من الحيوية ويرسلهما إلى خدمات مطابقة مستقلة. تتحقق هذه الخدمات من وجود شخص حي أمام الكاميرا وتقارن الوجه بجميع المسجّلين. تُتلَف الصور بعد المعالجة.',
  's2t':'توليد دليل ZK','s2d':'يلتزم إثبات Groth16 بقيمة bio_hash الخاصة بك ضمن commitment = keccak256(bioHash‖wallet) دون كشفها. يُشتق المُبطِل من هذا الهاش، لذا لا يمكن احتساب الوجه نفسه مرتين — انظر الأسئلة الشائعة أدناه.',
  's3t':'ربط المحفظة','s3d':'يفتح التطبيق MetaMask · ارتبط بمحفظة Ethereum · الدليل مرتبط تشفيرياً بعنوان محفظتك',
  's4t':'تم منح 1,000 AEQ','s4d':'تم تأكيد التسجيل على BlockDAG خلال 6 ثوانٍ · اعتماد 1,000 AEQ فوراً · هويتك مسجلة بشكل دائم',
  'priv-bar':'🔒 التحقق من الوجه بالنصاب · Groth16 ZKP · تُتلَف الصور بعد التحقق · تسجيل واحد لكل شخص',
  'conn-wallet':'المحفظة المتصلة','proof-recv':'⚡ تم استلام دليل ZK','proof-hint':'ربط محفظة للتسجيل',
  'btn-conn':'🦊 ربط METAMASK','btn-reg':'🔐 التسجيل ON-CHAIN',
  'btn-wc':'🔗 ربط WALLETCONNECT',
  'reg-log-hint':'// افتح تطبيق Aequitas Android لتوليد دليلك، ثم عد هنا...',
  'reg-details':'تفاصيل التسجيل','k-network':'الشبكة','k-chainid':'معرّف السلسلة','k-grant':'منحة UBI',
  'k-fee':'رسوم الغاز','free':'مجاني — بدون رسوم تماماً','k-limit':'التسجيلات','k-limit-v':'مرة واحدة لكل شخص · دائم · غير قابل للتغيير',
  'k-bio':'الوجه','never-stored':'تُتلَف الصور بعد التحقق — ولا يملك أي مُصادِق قالباً كاملاً',
  'k-proof':'نظام الأدلة','k-conf':'التأكيد','k-conf-v':'خلال ثانية واحدة (كتلة واحدة)',
  'k-sybil':'حماية Sybil','k-sybil-v':'هوية واحدة لكل شخص · مرتبطة بالوجه، والعتبة لم تُعايَر بعد',
  's-height':'ارتفاع الكتلة',
  's-humans':'البشر الموثقون',
  's-supply':'إجمالي العرض','s-supply-sub':'دائماً = البشر × 1,000 AEQ',
  's-uptime':'وقت التشغيل',
  'k-chain':'اسم السلسلة','k-symbol':'الرمز','k-btime':'وقت الكتلة',
  'k-cons':'التوافق','k-storage':'التخزين','k-dec':'الأرقام العشرية',
  'btn-add-mm':'+ إضافة شبكة AEQUITAS',
  'humans-title':'البشر الموثقون على Aequitas Chain',
  'h-what':'ما هو الإنسان الموثق؟','h-what-t':'الإنسان الموثق هو عنوان محفظة ثبت أنه يعود لشخص لم يُسجَّل وجهه من قبل. يجب أن تتفق خدمات مطابقة مستقلة بالنصاب، ولا يصل إلى السلسلة سوى إثبات Groth16 — دون أي صورة ودون أي قالب. <strong style="color:var(--gold)">حتى 2026-08-23 كان هذا يوثّق جهازاً لا شخصاً؛ ولم يعد الأمر كذلك.</strong>',
  'h-zkp':'نظام أدلة ZK','h-zkp-t':'تستخدم Aequitas بروتوكول Groth16 على المنحنى BN128 — نفس المنحنى المستخدم في Ethereum وZcash. ~200 بايت، ~10ms. commitment = keccak256(deviceKey‖wallet). يرتبط الـ nullifier بهذا الجهاز: فقدان الهاتف لا يُنشئ هوية ثانية على هذا الجهاز، لكن جهازاً آخر لا يزال بإمكانه التسجيل بشكل منفصل. لا يُكشَف عن مادة المفتاح أو تُخزَّن على الخادم أبداً.',
  'h-sybil':'مقاومة Sybil — الوضع الحالي','h-sybil-t':'يُشتق المُبطِل من bio_hash الخاص بوجهك، لذا لا يمكن تسجيل الوجه نفسه مرتين — ولا حتى عبر أجهزة مختلفة، وهو ما لم يستطعه مفتاح الجهاز قط. وما يستند إليه ذلك هو عتبة مطابقة لم تُعايَر بعد على عمليات التقاط حقيقية: التشفير دقيق، أما القياس الحيوي تحته فقياس لم تُحدَّد نسبة خطئه.',
  'h-global':'الشمول المالي العالمي','h-global-t':'لا حاجة لحساب مصرفي ولا بطاقة ائتمان ولا خبرة سابقة بالعملات المشفرة. يكفي هاتف أندرويد بكاميرا. صُمّمت Aequitas لتكون في متناول كل إنسان على الأرض.',
  'h-bio-hw':'خارطة طريق التحقق من الهوية','h-bio-hw-t':'اليوم (إصدار تجريبي): تحقق من الوجه عبر خدمات مطابقة مستقلة يجب أن تتفق بالنصاب. لم تُعايَر عتبتها بعد على عمليات التقاط حقيقية — يلزم نحو 1000 زوج منتحل قبل ذكر أي رقم. المخطط: هذه المعايرة، وفحص تكرار لا تحتفظ فيه أي خدمة بقالب كامل.',
  'reg-humans':'البشر المسجلون','h-desc':'كل عنوان أدناه يعود لشخص تم التحقق من وجهه عبر خدمات مستقلة مقابل جميع التسجيلات القائمة، وأُثبت بإثبات ZK، وقُيّد له 1000 AEQ بالضبط. السجل دائم وغير قابل للتغيير وعلى السلسلة. وما تضمنه العتبة اليوم وما لا تضمنه مذكور في الأسئلة الشائعة.',
  'no-humans':'لا يوجد بشر مسجلون بعد.\n\nحمّل تطبيق Aequitas Android وكن أول إنسان على السلسلة!',
  'reg-stats':'إحصائيات السجل','total-humans':'إجمالي البشر',
  'idx-title':'مؤشر Aequitas — درجة المساواة الاقتصادية في الوقت الفعلي',
  'idx-desc':'مؤشر Aequitas مشتق من <strong style="color:var(--teal)">معامل جيني</strong> — المعيار الدولي لقياس عدم المساواة (البنك الدولي، OECD، الأمم المتحدة). <strong style="color:var(--neon)">0 = مساواة مثالية</strong>. <strong style="color:var(--red)">100 = تركيز كامل</strong>. الهدف: جيني أقل من 0.30.',
  'gini-what-title':'ما هو معامل جيني؟',
  'gini-what-text':'طوّره كورادو جيني (1912). يقيس توزيع الثروة. المقياس: 0 (الجميع متساوون) إلى 1 (شخص واحد يملك كل شيء). يُستخدم من قِبل البنك الدولي وOECD والأمم المتحدة.',
  'gini-calc-title':'كيف يتم حساب مؤشر Aequitas؟','gini-calc-text':'يتم جمع جميع أرصدة AEQ للبشر المعتمدين. تحسب الصيغة الفرق المطلق المتوسط بين كل زوج من الأرصدة، مقسومًا على مربع عدد السكان (n²) والرصيد المتوسط. النتيجة 0-1 مضروبة في 100 = مؤشر Aequitas.',
  'gini-why-title':'لماذا جيني — ولا مقياس أبسط؟','gini-why-text':'نسبة الأغنى-الأفقر بسيطة وسهلة التحايل عليها — معامل جيني يكتشف ذلك. يلتقط المعامل التوزيع الكامل بين جميع البشر المعتمدين في رقم واحد قابل للتدقيق. تنشر Aequitas هذا على السلسلة — شفاف وقابل للتحقق عالميًا.',
  'curr-idx':'المؤشر الحالي','bar-0':'0 — مساواة مثالية','bar-100':'100 — أقصى عدم مساواة','wcap-lbl':'سقف الثروة الحالي:','wcap-mult':'المضاعف:','wcap-avg':'الحصة العادلة:',
  'phases-desc':'في المرحلة 0 (التأسيس)، يستخدم سقف الثروة مضاعفًا متحركًا: max(5, min(N, 25)) × متوسط الرصيد. مع 1-4 بشر: 5× المتوسط. كل إنسان جديد يضيف 1×. عند 25+ إنسانًا: يُثبَّت بشكل دائم عند 25×. تحدث جميع الانتقالات تلقائيًا بحسب عدد البشر — بدون تصويت إداري، بدون مفتاح إداري.',
  'wealth-cap-explain':'سقف الثروة في المرحلة 0 (التأسيس) يستخدم max(5, min(N, 25)) × متوسط رصيد AEQ، حيث N = عدد البشر المسجلين. 1-4 بشر: السقف = 5× المتوسط. كل إنسان جديد يضيف 1×. عند 25+: يُثبَّت دائمًا عند 25×. يتكيف السقف دائمًا مع متوسط الرصيد الحالي.',
  'p0':'التأسيس · أقل من 100 إنسان · سقف الثروة: max(5,min(N,25))× المتوسط · ينزلق من 5× إلى 25× حتى الإنسان الخامس والعشرين · نشط حاليًا',
  'p1':'النمو · 100–10,000 إنسان · سقف الثروة: 25× الحصة العادلة = 25,000 AEQ',
  'p2':'الاستقرار · 10,000–1M إنسان · سقف الثروة: 25× الحصة العادلة = 25,000 AEQ',
  'p3':'النضج · 1M+ إنسان · سقف الثروة: 25× الحصة العادلة = 25,000 AEQ',
  'gini':'معامل جيني','gini-desc':'0 = متساوٍ · 1 = غير متساوٍ',
  'supply-desc':'دائماً = البشر × 1,000 AEQ',
  'phase':'مرحلة البروتوكول','phase-desc':'يتقدم تلقائياً بعدد البشر',
  'humans-desc':'تسجيلات موثّقة بالوجه',
  'pools-title':'مجمعات إعادة التوزيع',
  'pools-desc':'كل رسوم المبادلة والتلاشي والفائض من سقف الثروة يُقسَّم تلقائياً بين أربعة مجمعات. جميعها تدفع يومياً.',
  'vel-pool':'مجمع المدققين','vel-pool-desc':'40% من جميع الرسوم ← مشغّلو العقد الذين يؤمّنون الشبكة',
  'liq-pool':'مجمع السيولة','liq-pool-desc':'30% من جميع الرسوم ← مزودو السيولة، بنسبة حصص LP',
  'ubi-pool':'مجمع UBI','ubi-pool-desc':'20% من جميع الرسوم ← جميع البشر الموثقين بالتساوي، كل 24 ساعة',
  'treasury':'الخزينة','treasury-desc':'10% من جميع الرسوم ← تطوير البروتوكول وصيانته',
  'phases-title':'مراحل البروتوكول',
  'demurrage-title':'التلاشي — حافز للتداول',
  'demurrage-desc':'أرصدة AEQ غير النشطة تفقد قيمتها ببطء لثني الاكتناز وتحفيز المشاركة الاقتصادية.',
  'dem-rate-k':'معدل التلاشي','dem-rate-v':'0.5% شهرياً (مستمر)',
  'dem-grace-k':'فترة السماح','dem-grace-v':'3 أشهر من الخمول قبل بدء التلاشي',
  'dem-reset-k':'إعادة التعيين','dem-reset-v':'أي تحويل أو مبادلة أو إجراء سيولة يعيد العداد إلى الصفر',
  'dem-dest-k':'AEQ المتلاشي يذهب إلى','dem-dest-v':'مجمعات إعادة التوزيع (40/30/20/10)',
  'dem-warn-k':'نظام التحذير','dem-warn-v':'إشعار 14 يوماً (مرة واحدة) + تذكير 7 أيام عند كل تسجيل دخول',
  'story-title':'قصة Aequitas',
  'nodes-title':'العقد النشطة — طوبولوجيا الشبكة الحالية',
  'nodes-desc':'تعمل شبكة Aequitas على عدة عقد موزعة جغرافياً (العدد الحالي أعلاه)، تشارك في إنتاج الكتل والمزامنة وخدمة API.',
  'run-node-title':'قم بتشغيل عقدتك الخاصة','run-node-desc':'يمكن لكل إنسان مسجَّل تشغيل عقدة Aequitas — بلا حصة، بلا طلب، بلا إذن منّا. إنسان واحد، مفتاح مُصادِق واحد: العقدة التي لا يكون NODE_OPERATOR_WALLET الخاص بها إنساناً مسجَّلاً تُرفض بـ HTTP 403، وإلا لأمكن لشخص واحد أن يصبح بهدوء مجموعة المُصادِقين كلها. المشغّلون يكسبون 40% من رسوم المبادلة يومياً.',
  'bootstrap-title':'ربط عقدة جديدة','bootstrap-desc':'لا توجد نقطة دخول لإعدادها — عناوين المتحققين مدمجة في البرنامج. تسجّل عقدتك نفسها تلقائياً وتزامن حالة السلسلة كاملة وتشارك في إنتاج الكتل. عيّن PRIMARY_NODE_URL فقط إذا أردت عمداً تثبيت نقطة دخول محددة.',
  'tech-title':'المواصفات التقنية','mm-config':'إعداد MetaMask',
  'k-lang':'اللغة','k-src':'المصدر','evm-yes':'نعم — JSON-RPC /rpc · متوافق مع MetaMask',
  'proto-label':'بروتوكول Aequitas V7 — وثائق تقنية',
  'ca-title':'عناوين العقود',
  'ca-text':'السلسلة: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'يحدّد AequitasV7 قواعد اقتصاد Aequitas ويحتفظ بالسجل الذي يجعلها قابلة للإنفاذ: كل nullifier استُخدم، وكل تسجيل، وسقف الثروة، ومعادلة الاندثار. العقد غير قابل للتغيير — لا مفتاح إدارة ولا وكيل ترقية ولا تصويت حوكمة يستطيع تغيير سطر واحد منه. أما ما ينفّذ التحويل الفعلي فهو طبقة السلسلة: تعترض العقدة نداء ERC-20 قبل وصوله إلى EVM وتقيّده في دفترها الخاص، وهذا ما يجعل التحويلات فورية وبلا رسوم غاز. العقد هو كتاب القواعد والسجل؛ والسلسلة هي المحرّك الذي ينفّذهما، وشيفرتها مفتوحة.',
  'poa-title':'1. إثبات الحياة','poa-text':'<p>عند وفاة الأشخاص، تعود AEQ الخاصة بهم تدريجياً إلى المجتمع عبر مجمع UBI بدلاً من ضياعها للأبد.</p>',
  'poa-box':'السنوات 0–2: استخدام طبيعي<br>السنة 2: تحذير 1 — الحارس يمكنه الرد<br>السنة 2+60 يوم: تحذير 2<br>السنة 2+120 يوم: تحذير 3<br>السنة 2+180 يوم: AEQ في ضمان شخصي<br>السنة 4: إذا لا يزال خاملاً — يعود لمجمع UBI',
  'guard-title':'2. نظام الحارس','guard-text':'<p>حارس موثوق (إنسان موثق آخر) يمكنه تأكيد أن شخصاً ما لا يزال حياً، دون أي حقوق مالية.</p>',
  'guard-box':'حارس واحد لكل إنسان · يجب أن يكون إنساناً موثقاً<br>الحارس يمكنه فقط استدعاء confirmAlive() · صفر حقوق مالية<br>الحارس لا يمكنه تحريك الأموال · الحد الأقصى 3 · Timelock 7 أيام',
  'dem-title':'3. التلاشي — آلية مكافحة الاكتناز',
  'dem-box':'يُفرض فقط على الجزء الذي يتجاوز حصتك العادلة — الرصيد المساوي لها أو الأقل لا يتناقص أبدًا<br>المعدل: 0.5%/شهر بعد 3 أشهر سماح<br>إعادة تعيين عند أي تحويل أو مبادلة أو سيولة<br>AEQ المتلاشي يُعاد توزيعه في المجمعات (لا يُحرق)',
  'dem-text':'<p>سابقة تاريخية: تجربة Wörgl (النمسا، 1932) — خفض البطالة 25% في عام واحد. Chiemgauer (ألمانيا، 2003) — يعمل بنجاح منذ أكثر من 20 عاماً.</p>',
  'cap-title':'4. سقف الثروة','cap-box':'السقف: max(5,min(N,25))× متوسط الرصيد<br>1–4 بشر: 5× · +1× لكل إنسان · 25+: 25× دائم<br>الفائض يُعاد توزيعه فوراً · بدون تدخل يدوي',
  'ubi-title':'5. الدخل الأساسي الشامل','ubi-box':'المصادر: رسوم المبادلة (20%) · فائض السقف · التلاشي<br><br>يومياً: مجمع UBI مقسّم بالتساوي بين جميع البشر المسجلين. يُعاد ضبط المجمع بعد كل توزيع.',
  'inf-title':'6. لا تضخم خوارزمي','inf-box':'الحدث الوحيد الذي ينشئ AEQ جديداً: تسجيل إنسان موثق جديد.<br><br>إجمالي العرض = البشر الموثقون × 1,000 AEQ — دائماً، بالضبط.',
  'btn-download-app':'تحميل تطبيق AEQUITAS',
  'swap-title':'🔄 تبادل AEQ ↔ tUSD','swap-sub':'تبادل AEQ مع tUSD (دولار اختبار محاكى) عبر مجمع السيولة الأصلي. رسوم 0.1% فقط للمبادلات — التحويلات العادية مجانية تماماً.',
  'swap-priv-bar':'🔒 رسوم 0.1% فقط · تحويلات AEQ→AEQ مجانية · tUSD عملة اختبار بدون قيمة حقيقية',
  'swap-your-aeq':'AEQ لديك','swap-your-tusd':'tUSD لديك',
  'swap-fee-est':'رسوم البروتوكول (0.1%)','swap-details-hdr':'تفاصيل التبادل',
  'swap-out-lbl':'ستحصل على (تقريباً)','swap-impact-lbl':'تأثير السعر','swap-rate-lbl':'سعر الصرف',
  'swap-depth-lbl':'تكوين المجمع','amm-title':'x × y = k — AMM ذو الجداء الثابت',
  'amm-text':'عند التبادل، تزداد احتياطيات AEQ وتنخفض احتياطيات tUSD — جداؤها يبقى دائماً مساوياً لـ k. التبادلات الكبيرة تسبب تأثيراً أكبر على السعر.',
  'swap-btn-go':'🔄 تبادل',
  'swap-log-hint':'// ربط محفظة للتبادل...',
  'swap-no-liquidity':'لا يوجد tUSD بعد?','swap-faucet-desc':'البشر المسجلون يمكنهم المطالبة بـ tUSD اختبار مرة واحدة','swap-btn-faucet':'💧 المطالبة بـ tUSD الاختبار',
  'swap-addliq-title':'توفير السيولة','swap-addliq-desc':'كن أول من يودع — نسبتك تحدد السعر الأولي.','swap-btn-addliq':'💧 إضافة سيولة',
  'swap-lp-title':'مركز LP الخاص بك','swap-lp-share':'حصة المجمع','swap-lp-withdrawable':'قابل للسحب',
  'swap-lp-pct-label':'% من مركزك','swap-lp-youget':'ستحصل على','swap-btn-removeliq':'🔥 سحب السيولة',
  'swap-pool-title':'AEQ / tUSD — حالة المجمع',
  'swap-pool-aeq':'احتياطي AEQ','swap-pool-tusd':'احتياطي tUSD','swap-pool-price':'السعر الفوري',
  'swap-fee-bps':'رسوم المبادلة',
  'swap-pools-addr-title':'عناوين مجمعات التوكينوميكس',
  'swap-validators':'المدققون (40%)','swap-lps':'مزودو السيولة (30%)','swap-ubi':'مجمع UBI (20%)','swap-treasury':'الخزينة (10%)',
  'ubi-hero-title':'الدخل الأساسي الشامل — مجمع UBI',
  'ubi-hero-sub':'يتراكم — الدفعة التالية توزَّع بالتساوي على جميع البشر الموثقين خلال:',
  'ubi-bal-lbl':'رصيد المجمع الحالي','ubi-hero-desc':'مقسَّم بالتساوي · يُدفع كل 24 ساعة · يُصفَّر المجمع · لا يشترط رصيد أدنى',
  'ubi-how-fills':'كيف يمتلئ مجمع UBI',
  'ubi-src-swap':'رسوم المبادلة','ubi-src-swap-d':'كل مبادلة AEQ↔tUSD تساهم بـ 20% من رسومها. المزيد من التداول = امتلاء أسرع.',
  'ubi-src-dem':'التلاشي','ubi-src-dem-d':'AEQ الخامل (3+ أشهر) يتلاشى 0.5%/شهر. 20% من المتلاشي يذهب لـ UBI.',
  'ubi-src-cap':'فائض السقف','ubi-src-cap-d':'المحافظ التي تتجاوز السقف تُقلَّص فوراً. 20% يتدفق إلى UBI.',
  'pools4-header':'المجمعات الأربعة لإعادة التوزيع',
  'ubi-see-above':'انظر العد التنازلي أعلاه','ubi-timer-above':'⏰ العد التنازلي معروض أعلاه','pool-t-timer':'يتراكم — لا عداد',
  'usp-headline':'لأول مرة في التاريخ — الجميع يبدأ على قدم المساواة',
  'usp-sub':'إذا كان لديك هاتف أندرويد فأنت مؤهل. بدون بنك، بدون معرفة بالعملات المشفرة، بدون استثمار.',
  'usp-c1-title':'استثمار أولي 0','usp-c1-desc':'التسجيل مجاني تماماً. لا ETH ولا بطاقة بنكية. البروتوكول يدفع جميع رسوم المعاملات.',
  'usp-c2-title':'1,000 AEQ لكل إنسان','usp-c2-desc':'مليارديرًا كان أم مزارعاً — الجميع يحصل على 1,000 AEQ بالضبط. مساواة مضمونة رياضياً.',
  'usp-c3-title':'متاح للجميع','usp-c3-desc':'بدون حساب مصرفي، بدون بطاقة ائتمان، بدون هوية رسمية، بدون أجهزة إضافية — فقط الكاميرا الموجودة أصلاً في هاتفك الأندرويد.',
  'usp-c4-title':'UBI يومي إلى الأبد','usp-c4-desc':'بعد التسجيل، تصل حصتك من UBI تلقائياً كل يوم — دون أي إجراء.',
  'v7-intro-title':'ما هو AequitasV7؟',
  'v7-intro-text':'AequitasV7 هو العقد الذكي المركزي لبروتوكول Aequitas. مُنشر بشكل غير قابل للتغيير على Aequitas Chain (ID 1926). يدير كل شيء: التسجيل البشري، التحقق ZK، الأرصدة، سقف الثروة، UBI، رسوم المبادلة. لا يمكن لأي مدير تعديله.',
  'swap-sell-label':'بيع','swap-receive-label':'استلام',
  'guard-title':'🛡 نظام الوصي','guard-my-lbl':'وصيّي','guard-none':'لا يوجد',
  'guard-set-lbl':'تعيين / تغيير الوصي','guard-set-hint':'يجب أن يكون إنساناً مسجلاً في Aequitas · قفل زمني لمدة 7 أيام · الوصي يستطيع فقط تأكيد حياتك، لا الوصول إلى الأموال · الحد الأقصى 3 محميين لكل وصي',
  'guard-confirm-lbl':'تأكيد الحياة (بصفة وصي)','guard-confirm-hint':'إذا لم يستطع محميّك الوصول إلى محفظته، أكّد حياته لمنع نقل أمواله إلى الضمان بعد 910 يوماً من الخمول.','guard-recover-btn':'🔓 استرداد من الضمان',
  'faq-title':'❓ الأسئلة الشائعة','faq-q1':'هل بياناتي البيومترية آمنة؟','faq-a1':'يتم التقاط وجهك وإرساله إلى خدمات مطابقة مستقلة — وهذه هي الطريقة الوحيدة للتحقق فعلياً من «شخص واحد، حساب واحد». تُعالَج الصور ثم تُتلَف؛ ولا يتم تخزينها. ما يُحتفَظ به هو قالب رياضي: مشفَّر ومقسَّم إلى حصص موزَّعة على مُصادِقين يُشغَّلون بشكل منفصل، بحيث لا يملك أي مُصادِق قالباً كاملاً أبداً. حد صريح، مذكور لا مُخفى: الخدمة التي تنفّذ المقارنة ما زالت تحتفظ بالقوالب، لأن المقارنة تتطلبها.',
  'faq-q1b':'هل يثبت التسجيل أنني شخص حقيقي وفريد؟','faq-a1b':'أفضل مما استطاعه مفتاح الجهاز يوماً، ولا يمكن إثباته بعد كرقم. يُقارن الوجه بجميع التسجيلات القائمة عبر خدمات مستقلة يجب أن تتفق، لذا يُكتشف الشخص نفسه على هاتف ثانٍ — وهو ما لم يستطعه مفتاح الجهاز قط. ما لم يُحدَّد بعد هو نسبة الخطأ: لم تُعايَر العتبة على عمليات التقاط حقيقية، ويلزم لذلك نحو 1000 زوج منتحل.',
  'faq-q2':'هل يمكنني التسجيل بمحفظة مختلفة لاحقاً؟','faq-a2':'لا. يرتبط التسجيل بشكل دائم بعنوان محفظة واحد. وهذا مقصود: المُبطِل المشتق من وجهك يُستهلك مرة واحدة، لذا فإن التسجيل مجدداً بمحفظة أخرى سيكون هوية ثانية للشخص نفسه.',
  'faq-q3':'ماذا يحدث إذا فقدت هاتفي؟','faq-a3':'يبقى AEQ الخاص بك في محفظتك — مرتبط بمفتاحك الخاص، وليس بهاتفك. لا يزال بإمكانك الوصول إلى محفظتك عبر MetaMask باستخدام عبارة الاسترداد. استرداد المحفظة مستقل عن التسجيل البيومتري.',
  'path-title':'اختر مسارك','path-human-title':'أنا إنسان','path-human-desc':'أريد التسجيل وتلقي 1,000 AEQ والانضمام إلى شبكة الدخل الأساسي.','path-human-steps':'1. تحميل تطبيق Aequitas Android<br>2. فتح القفل بقفل شاشة جهازك (بصمة/وجه/رمز PIN)<br>3. ربط MetaMask<br>4. استلام 1,000 AEQ فوراً',
  'path-node-title':'أنا مشغّل عقدة','path-node-desc':'أريد تشغيل عقدة كاملة والمشاركة في إنتاج الكتل والكسب من مجموعة المتحققين 40%.','path-node-steps':'1. التسجيل كإنسان (مطلوب)<br>2. لا يوجد نقطة دخول لإعدادها — عناوين المتحققين مدمجة<br>3. النشر على Contabo/Hetzner/أي VPS<br>4. الكسب اليومي من مجموعة المتحققين',
  'path-dev-title':'أنا مطوّر','path-dev-desc':'أريد البناء على Aequitas أو دمج API أو المساهمة في البروتوكول.','path-dev-steps':'1. JSON-RPC متوافق مع EVM<br>2. معرّف السلسلة: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* نقاط النهاية<br>4. المقاييس: /metrics (Prometheus)',
  'story-flow-title':'مخطط تدفق رمز AEQ','story-topo-title':'طوبولوجيا الشبكة — الحالة الراهنة',
  'swap-price-title':'AEQ / tUSD — السعر المباشر','swap-price-desc':'سعر فوري مشتق من احتياطيات المجموعة (x·y=k). يتحدث كل 8 ثوانٍ.','swap-price-empty':'لا توجد بيانات مجموعة بعد — أضف سيولة لرؤية مخطط السعر.',
  'node-guide-lang-note':'هذا الدليل المضمّن باللغة الإنجليزية. ملف PDF مترجم بلغتك متاح عبر الزر أعلاه.',
  'k-zkp':'نظام ZKP','k-hash':'نظام التجزئة','k-sybil-prot':'حماية سيبل',
  'soc-title':'💬 وسائل التواصل الاجتماعي','soc-sub':'الإعلانات، وحالة السلسلة، والأسئلة المحرجة &mdash; علنًا، على كليهما.',
  'soc-x-desc':'الإعلانات، وما تفعله السلسلة فعليًا. بصيغة مختصرة.','soc-tg-desc':'المجموعة المفتوحة: الأسئلة، ومشغّلو العقد، والمساعدة في التسجيل.',
  's-validators':'المدققون النشطون',
  'expl-heading':'مستكشف الكتل',
},
hi:{
  'x-consensus-ghostdag-knightdag':'◆ सर्वसम्मति: GHOSTDAG + KNIGHTDAG',
  'x-contract-code':'अनुबंध कोड',
  'x-demurrage-is-a-holding-cost':'ठहराव शुल्क धन रखने की लागत है — एक ऋणात्मक ब्याज जो संचय को महँगा और प्रचलन को आकर्षक बनाता है। इसका ऐतिहासिक उदाहरण है: वर्गल प्रयोग (ऑस्ट्रिया, 1932) में ऐसी ही मुद्रा प्रयुक्त हुई और एक वर्ष में स्थानीय बेरोज़गारी 25 % घट गई। ऑस्ट्रिया के राष्ट्रीय बैंक ने इसे ठीक इसलिए बंद किया क्योंकि यह कहीं अधिक सफल था और बैंकिंग एकाधिकार के लिए ख़तरा बन गया था। Chiemgauer (जर्मनी, 2003) इसी सिद्धांत पर चलता है और बीस वर्षों से सफलतापूर्वक प्रचलन में है। Aequitas प्रति माह 0.5 % का सतत ठहराव शुल्क लागू करता है, वह भी केवल तीन माह की निष्क्रियता के बाद।',
  'x-network-consensus':'→ नेटवर्क / सर्वसम्मति',
  'x-node-decentralization-roadmap':'नोड विकेंद्रीकरण की योजना',
  'x-open-source-chain-logic':'शृंखला-तर्क का खुला स्रोत',
  'x-phase-0-now':'चरण 0 (अभी):',
  'x-phase-1-100-humans':'चरण 1 (100+ लोग):',
  'x-phase-2-1-000-humans':'चरण 2 (1,000+ लोग):',
  'x-phase-3-10-000-humans':'चरण 3 (10,000+ लोग):',
  'x-protocol-mechanisms':'प्रोटोकॉल के तंत्र',
  'x-what-happens-to-aeq-when':'जब कोई व्यक्ति मर जाए या स्थायी रूप से असमर्थ हो जाए, तो AEQ का क्या होता है? बिटकॉइन और अधिकांश क्रिप्टोमुद्राओं में खोया बटुआ हमेशा के लिए खोई आपूर्ति है — अनुमान है कि लाखों BTC सदा के लिए अगम्य हैं। Aequitas इसे बहु-चरणीय निष्क्रियता-पुनर्प्राप्ति से हल करता है: यदि कोई बटुआ लंबे समय तक कोई गतिविधि न दिखाए, तो उसका शेष धीरे-धीरे मूल आय कोष के माध्यम से समुदाय को लौट जाता है, जिससे वास्तव में प्रचलित आपूर्ति सार्थक बनी रहे।',
  'x-what-if-someone-is-hospitalized':'और यदि कोई अस्पताल में हो, जेल में हो, या महीनों तक अपने उपकरण तक न पहुँच सके? विश्वासपात्र प्रणाली किसी अन्य सत्यापित व्यक्ति को यह पुष्टि करने देती है कि बटुए का स्वामी अब भी जीवित है, जिससे उसके AEQ अमानत में नहीं जाते। उस व्यक्ति के पास कोई वित्तीय पहुँच बिल्कुल नहीं होती: वह केवल एक फलन बुला सकता है जो निष्क्रियता की घड़ी को शून्य कर देता है। किसी भी परिस्थिति में वह धन हटा, खर्च या देख नहीं सकता।',
  'bv-bind':'🔗 बाइंडिंग हस्ताक्षर बनाएँ',
  'bv-check-d':'दूसरा आह्वान हर सत्यापक को सूचीबद्ध कर उनकी तुलना करता है: क्या सभी के पास पंजीकरणों की समान संख्या है, क्या किसी के पास बीज नहीं है, और क्या कुंजियाँ मेल खाती हैं। यदि आपकी प्रविष्टि में अंतर दिखे, तो उसे यहाँ जान लेना किसी के पंजीकरण के बीच जानने से बेहतर है।',
  'bv-check-t':'जाँचना कि यह काम कर रहा है',
  'bv-desc':'ब्लॉक बनाने वाला नोड <strong style="color:var(--text)">बहीखाते</strong> की रक्षा करता है। बायो-सत्यापक कुछ और सुरक्षित करता है: यह वादा कि <strong style="color:var(--neon)">हर व्यक्ति केवल एक बार पंजीकरण करे</strong>। ये अलग भूमिकाएँ हैं — आप एक चला सकते हैं, या दोनों एक ही मशीन पर।',
  'bv-guide-sub':'चरण-दर-चरण &middot; क्रिप्टोग्राफी का ज्ञान आवश्यक नहीं &middot; लगभग 30 मिनट, अधिकांश डाउनलोड में',
  'bv-honest-d':'यह हिस्सा बीटा में है और सीमाएँ वास्तविक हैं। संयुक्त तुलना एक बार उपयोग होने वाली क्रिप्टोग्राफ़िक सामग्री खर्च करती है, और एक आपूर्ति अभी कुछ दर्जन पंजीकरणों तक ही चलती है — यानी गोपनीय मार्ग पहले छोटे पैमाने पर स्वयं को सिद्ध करता है, लाखों पर नहीं। काम पंजीकृत लोगों की संख्या के साथ भी बढ़ता है। हम ये आँकड़े गोल करने के बजाय प्रकाशित करते हैं: जो व्यवस्था आपका चेहरा माँगती है, उसे यह बताने में अस्पष्ट रहने का अधिकार नहीं कि वह क्या कर सकती है और क्या अभी नहीं।',
  'bv-honest-t':'आज स्थिति क्या है — साफ़-साफ़',
  'bv-need-1':'<strong style="color:var(--text)">एक पंजीकृत Aequitas खाता।</strong> वही नियम जो ब्लॉक बनाने के लिए है, और उसी कारण से: एक व्यक्ति, एक कुंजी। इसके बिना एक ही व्यक्ति चुपचाप पूरी समिति बन सकता था।',
  'bv-need-2':'<strong style="color:var(--text)">Docker वाला एक छोटा लिनक्स सर्वर।</strong> 2 GB मेमोरी पर्याप्त है। ग्राफ़िक्स कार्ड की ज़रूरत नहीं — तुलना 64 बाइट पर अंकगणित भर है। जिस मशीन पर आपका नोड पहले से चल रहा है, वही चलेगी।',
  'bv-need-3':'<strong style="color:var(--text)">HTTPS वाला एक डोमेन नाम।</strong> समिति के अन्य सदस्यों को आप तक पहुँचना होगा। किसी ऐसी चीज़ का उपडोमेन जो पहले से आपकी है, काफी है।',
  'bv-need-4':'<strong style="color:var(--text)">ऑनलाइन बने रहना।</strong> किसी पंजीकरण के पूरा होने के लिए समिति के हर सदस्य को उत्तर देना होता है। बार-बार अनुपलब्ध रहने वाला सत्यापक लोगों की रक्षा करने के बजाय उन्हें धीमा करता है।',
  'bv-need-t':'शुरू करने से पहले — आपको क्या चाहिए',
  'bv-s1-note':'निजी आधा हिस्सा अपने सर्वर पर रखें और कहीं नहीं। सार्वजनिक आधा साझा करने के लिए ही है — इसी से दूसरे जाँचते हैं कि आपने कुछ प्रमाणित किया। <strong style="color:var(--text)">आपका अपना प्रक्षेपण बीज मायने रखता है:</strong> चूँकि हर सत्यापक अलग बीज उपयोग करता है, एक से चुराया गया डेटाबेस दूसरे के विरुद्ध नहीं मिलाया जा सकता। बीज खो जाए तो आपके संचित हिस्से अर्थहीन हो जाते हैं, इसलिए उसका बैकअप वहीं रखें जिस पर आपका नियंत्रण हो।',
  'bv-s1-t':'चरण 1 — अपनी कुंजियाँ स्वयं बनाएँ',
  'bv-s1-warn-d':'एक ही रहस्य रखने वाले दो सत्यापक एक ही गिने जाते हैं, और समिति दिखने से छोटी हो जाती है। कोई भी — हम भी नहीं — आपको कभी कुंजी न भेजे।',
  'bv-s1-warn-t':'इन्हें स्वयं बनाएँ। किसी से भी कभी कुंजी स्वीकार न करें।',
  'bv-s2-d':'चरण 1 के मान ऐसी फ़ाइल में डालें जिसे केवल आप पढ़ सकें। प्रति पंक्ति एक मान, बिना उद्धरण चिह्नों के।',
  'bv-s2-note':'<strong style="color:var(--gold)">जब तक आप डेटा-सुरक्षा टिप्पणियाँ न पढ़ लें, ALLOW_REAL_BIOMETRIC_DATA को false ही रहने दें</strong>। बंद रहने पर आपका सत्यापक नेटवर्क से जुड़ता है और परीक्षण पंजीकरणों में भाग लेता है, बिना किसी वास्तविक व्यक्ति का डेटा कभी संचित किए। शुरुआत का यही सही तरीका है, और इसे बदलने की कोई जल्दी नहीं।',
  'bv-s2-t':'चरण 2 — कॉन्फ़िगरेशन फ़ाइल लिखें',
  'bv-s3-note':'स्वस्थ उत्तर <span style="font-family:var(--font-mono);color:var(--neon)">plaintext_templates: 0</span> और <span style="font-family:var(--font-mono);color:var(--neon)">sketch_seed_configured: true</span> बताता है। पहला यह दावा है कि कोई पूरा टेम्पलेट संचित नहीं होता — ऐसे रूप में जिसे आप मान लेने के बजाय स्वयं जाँच सकते हैं। अभी जाँचें और बाद में फिर जाँचें — यह जितनी दूसरों की गारंटी है, उतनी ही आपकी अपनी।',
  'bv-s3-t':'चरण 3 — सत्यापक चालू करें',
  'bv-s4-d':'समिति के अन्य सदस्य आप तक सार्वजनिक इंटरनेट से पहुँचते हैं, इसलिए पोर्ट बिना एन्क्रिप्शन खुला नहीं रहना चाहिए। Caddy प्रमाणपत्र स्वयं ले लेता है।',
  'bv-s4-t':'चरण 4 — आगे HTTPS लगाएँ',
  'bv-s5-d':'ब्लॉक उत्पादक अपनी हस्ताक्षर कुंजी को एक पंजीकृत मानव वॉलेट से बाँधते हैं: वॉलेट <strong style="color:var(--text)">Aequitas: authorize validator &lt;पता&gt;</strong> पर हस्ताक्षर करता है, और उसके बिना श्रृंखला स्थान देने से इनकार करती है। नीचे का बटन ठीक वही हस्ताक्षर बनाता है — सत्यापनकर्ता भूमिका के लिए। <strong style="color:var(--text)">बायो-सत्यापक कुंजी के लिए ऐसा बंधन अभी नहीं है।</strong> उसका सार्वजनिक आधा हिस्सा श्रृंखला के बाहर एकत्र किया जाता है (चरण 6) और उस सूची में जोड़ा जाता है जिसे हर प्रूफ सर्वर जाँचता है। श्रृंखला पर कुछ भी उसे किसी व्यक्ति से नहीं जोड़ता। जब तक यह नहीं है, समिति मशीनें गिनती है, लोग नहीं, और एक संचालक कई रख सकता है। हम यह यहाँ कहना पसंद करते हैं बजाय इसके कि संख्या वास्तविकता से मज़बूत दिखे।',
  'bv-s5-t':'चरण 5 — कोई कुंजी किसी व्यक्ति से किससे बंधती है (और किससे अभी नहीं)',
  'bv-s6-d':'चरण 1 का <strong style="color:var(--text)">सार्वजनिक</strong> आधा हिस्सा अपने HTTPS पते के साथ समूह को भेजें। वह उस सूची में जुड़ जाता है जिसे हर प्रमाण सर्वर जाँचता है, और उसके बाद आपकी गवाहियाँ कोरम में गिनी जाती हैं। इस चरण में आपकी मशीन से कुछ भी गुप्त बाहर नहीं जाता — विभाजन का यही अर्थ है: निजी आधा सदा आपके पास रहता है, और सार्वजनिक आधा उसके बिना बेकार है।',
  'bv-s6-t':'चरण 6 — अपनी सार्वजनिक कुंजी प्रकाशित करें',
  'bv-status-d':'सत्यापक का स्रोत कोड <strong style="color:var(--text)">अभी सार्वजनिक नहीं है</strong>, इसलिए नीचे दिए चरण आज हर कोई पूरा नहीं कर सकता। फिर भी हम इन्हें प्रकाशित कर रहे हैं, क्योंकि किसी रचना की जाँच तैनाती से पहले होनी चाहिए, बाद में नहीं। यदि आप एक चलाना चाहते हैं, तो मुखपृष्ठ पर दिए टेलीग्राम समूह में पूछें। इस भंडार को खोलना ही इस मार्गदर्शिका को योजना से निमंत्रण में बदलेगा, और यही अगली चीज़ है जो हम आप पर उधार हैं।',
  'bv-status-t':'स्थिति: बंद बीटा — शुरू करने से पहले पढ़ें',
  'bv-title':'या बायो-सत्यापक बनें — वह भूमिका जो विशिष्टता को विकेंद्रित करती है',
  'bv-what-d':'आपको कभी कोई चेहरा नहीं भेजा जाता। आपकी मशीन 64 बाइट के सारांश का एक <strong style="color:var(--text)">योगात्मक हिस्सा</strong> रखती है: अकेले में वह यादृच्छिक शोर से अलग नहीं पहचाना जा सकता, और आपके पास उपलब्ध कोई गणना उससे चेहरा वापस नहीं ला सकती। तुलना आपकी समिति के अन्य सदस्यों के साथ मिलकर होती है, और आप में से कोई भी उत्तर के सिवा कुछ नहीं जानता — <em>प्रतिलिपि: हाँ या नहीं</em>। यह हमारी नीयत का वादा नहीं, यह गणित का गुण है।',
  'bv-what-t':'आप क्या रखेंगे — और क्या कभी नहीं देखेंगे',
  'bv-why-d':'कोई पंजीकरण तभी स्वीकार होता है जब <strong style="color:var(--text)">कई अलग-अलग सत्यापक</strong> उसकी गवाही दे चुके हों। इसलिए एक चोरी हुई कुंजी काफी नहीं — हमलावर को पूरी समिति चाहिए। और चूँकि <strong style="color:var(--neon)">एक व्यक्ति ठीक एक ही सत्यापक कुंजी रख सकता है</strong>, समिति खरीदने का अर्थ है उतने लोग होना। 100 सत्यापकों में से 10 पर नियंत्रण रखने वाले के पास तीन की पूरी समिति पाने का अवसर 1000 में 1 से भी कम है। हर जुड़ने वाला व्यक्ति इस संख्या को घटाता है। यही एकमात्र जगह है जहाँ भागीदारों की संख्या <em>ही</em> सुरक्षा है। <strong style="color:var(--text)">यह गणना प्रति सत्यापक कुंजी एक व्यक्ति मानती है।</strong> ब्लॉक उत्पादन के लिए श्रृंखला इसे लागू करती है; सत्यापक कुंजियों के लिए अभी नहीं (चरण 5)। तब तक ऊपर की संख्या सुरक्षा की ऊपरी सीमा है, उसका माप नहीं।',
  'bv-why-t':'हर अतिरिक्त सत्यापक नेटवर्क को भ्रष्ट करना कठिन क्यों बनाता है',
  'x-0-1-split-40-30':'0.1 % · बँटवारा 40/30/20/10',
  'x-0-8211-100-humans-sliding':'0 &#8211; 100 लोग। सरकती संपत्ति सीमा 5x &#8594; 25x। नींव का चरण।',
  'x-0-8211-2-years':'0 &#8211; 2 वर्ष',
  'x-0-perfect-equality':'0 = पूर्ण समानता',
  'x-1-000-aeq-minted':'+1,000 AEQ जारी',
  'x-1-000-aeq-per-human':'प्रति व्यक्ति 1,000 AEQ',
  'x-1-000-aeq-will-be':'1,000 AEQ अपने आप जमा हो जाएँगे',
  'x-10-000-8211-1m-humans':'10,000 &#8211; 10 लाख लोग। कम से कम 10 नोड। पूरी तरह विकेंद्रित।',
  'x-100-8211-10-000-humans':'100 &#8211; 10,000 लोग। स्थिर सीमा 25x। नोड के लिए खुला प्रवेश।',
  'x-100-maximum-concentration':'100 = अधिकतम संकेंद्रण',
  'x-1m-humans-global-ubi-at':'10 लाख से अधिक लोग। बड़े पैमाने पर वैश्विक मूल आय। जिनी लक्ष्य &lt;0.30।',
  'x-9679-liquidity-lp-30':'&#9679; तरलता LP 30 %',
  'x-9679-treasury-10':'&#9679; कोष 10 %',
  'x-9679-ubi-pool-20':'&#9679; मूल आय कोष 20 %',
  'x-9679-validators-40':'&#9679; सत्यापक 40 %',
  'x-active-validators':'सक्रिय सत्यापक',
  'x-add-aequitas-chain-to-metamask':'अपना AEQ शेष देखने, लेन-देन भेजने और ब्राउज़र या मोबाइल बटुए से सीधे V7 अनुबंध के साथ काम करने के लिए Aequitas शृंखला को MetaMask में जोड़ें।',
  'x-admin-keys-or-governance-votes':'प्रशासनिक कुंजियाँ या शासन-मतदान',
  'x-aeq-activity':'AEQ गतिविधि',
  'x-aeq-reserve-tusd-reserve-k':'AEQ_reserve × tUSD_reserve = k (constant)',
  'x-aequitas-blockdag-nothing-wasted':'Aequitas BlockDAG — कुछ भी व्यर्थ नहीं जाता',
  'x-aequitas-chain':'Aequitas Chain',
  'x-aequitas-chain-blockdag':'Aequitas शृंखला (BlockDAG)',
  'x-aequitas-implements-this-mathematically-ever':'Aequitas इसे गणितीय रूप से लागू करता है। हर सत्यापित व्यक्ति को ठीक 1,000 AEQ मिलते हैं &#8212; अरबपति हो या निर्वाह-किसान, कोई अपवाद नहीं। चार पुनर्वितरण तंत्र असमानता को अनंत रूप से जमा नहीं होने देते। जिनी गुणांक शृंखला पर वास्तविक समय में दर्ज होता है।',
  'x-aequitas-index-g-100':'Aequitas Index = G × 100',
  'x-aequitas-now':'Aequitas Now',
  'x-aequitas-proof-of-humanity-chain':'Aequitas — मानवता प्रमाण शृंखला',
  'x-android-apk-direct-download':'Android APK · सीधा डाउनलोड',
  'x-architecture':'संरचना',
  'x-automatic-on-chain':'शृंखला पर स्वतः',
  'x-bitcoin-gini':'Bitcoin Gini',
  'x-blockdag-directed-acyclic-graph':'BlockDAG (दिशात्मक अचक्रीय ग्राफ़)',
  'x-blockdag-parallel-production':'BlockDAG · समानांतर उत्पादन',
  'x-blockdag-proof-of-humanity':'BlockDAG + मानवता प्रमाण',
  'x-blue-score':'«नीला अंक»',
  'x-both-blocks-are-kept-ghostdag':'दोनों ब्लॉक रखे जाते हैं — GHOSTDAG समवर्ती ब्लॉक को भी समेट लेता है और उसे मानक क्रम में गिनता रहता है।',
  'x-canonical-winner':'मानक विजेता',
  'x-commitment-keccak256-devicekey-wallet':'commitment = keccak256(deviceKey ‖ wallet)',
  'x-comparable-to-the-usa-0':'अमेरिका (0.41) या फ़्रांस (0.32) के बराबर। अधिकांश विकसित अर्थव्यवस्थाओं की सीमा में। पुनर्वितरण वक्र को सक्रिय रूप से समतल कर रहा है।',
  'x-confirm-ward-is-alive':'✓ पुष्टि करें कि व्यक्ति जीवित है',
  'x-core-technology':'मूल तकनीक',
  'x-daily-ubi-returns-to-all':'दैनिक मूल आय सभी सत्यापित लोगों को लौटती है',
  'x-demurrage-0-5-mo':'ठहराव शुल्क (0.5 %/माह)',
  'x-device-bound-zk-proof-one':'उपकरण से बँधा ZK प्रमाण · प्रति उपकरण एक पंजीकरण',
  'x-diagonal-line-perfect-equality':'विकर्ण रेखा = पूर्ण समानता',
  'x-disconnect-wallet':'⊘ बटुआ अलग करें',
  'x-distinct-proposers-recent-blocks':'भिन्न प्रस्तावक, हाल के ब्लॉक',
  'x-distribution':'📈 वितरण',
  'x-elliptic-curve':'दीर्घवृत्तीय वक्र',
  'x-entire-distribution':'सम्पूर्ण वितरण',
  'x-evm-compatible':'EVM संगत',
  'x-fill-ghostdag-verdict-thin-ring':'भराव = GHOSTDAG का निर्णय · पतला वलय = प्रस्तावक · प्रति ऊँचाई एक स्तंभ। विवरण के लिए किसी ब्लॉक पर कर्सर ले जाएँ।',
  'x-generate-node-binding-signature':'🔗 बाइंडिंग हस्ताक्षर बनाएँ',
  'x-run-a-coordinator':'🚪 समन्वयक चलाएँ',
  'co-title':'या एक समन्वयक चलाएँ — वह द्वार जिससे हर मनुष्य गुज़रता है',
  'co-desc':'समन्वयक वह जगह है जहाँ व्यक्ति पहुँचता है: वह चुनौती जारी करता है, कैप्चर को सत्यापकों में बाँटता है, उनके मत गिनता है, और वह प्रमाणन जारी करता है जिस पर शृंखला मुद्रण करती है। लंबे समय तक ठीक एक ही था — यानी नेटवर्क का हर पंजीकरण एक ही मशीन से गुज़रता था। इसलिए नहीं कि कुछ कमी थी, बल्कि इसलिए कि किसी ने दूसरा नहीं चलाया।',
  'co-status-t':'स्थिति: बंद बीटा — सत्यापक जैसी ही चेतावनी',
  'co-status-d':'समन्वयक उसी रिपॉज़िटरी में है जिसमें सत्यापक, और वह रिपॉज़िटरी <strong style="color:var(--text)">अभी सार्वजनिक नहीं है</strong>। इसलिए नीचे दिए चरण आज हर कोई पूरा नहीं कर सकता। फिर भी ये प्रकाशित हैं, उसी कारण से: किसी रचना की जाँच तैनाती से पहले संभव होनी चाहिए, बाद में नहीं।',
  'co-power-t':'समन्वयक क्या कर सकता है — और क्या नहीं',
  'co-power-d':'वह <strong style="color:var(--text)">कोई मनुष्य गढ़ नहीं सकता</strong>। जब तक कई अलग-अलग सत्यापक प्रमाणित न कर दें, कोई bio_hash बनता ही नहीं, और समन्वयक के पास उनकी एक भी कुंजी नहीं होती। वह जो कर सकता है, वह है किसी <strong style="color:var(--text)">मौजूदा</strong> bio_hash को किसी बटुए से बाँधना — यानी बेईमान समन्वयक आवंटन को अपनी पसंद के पते पर मोड़ सकता है। यह वास्तविक अधिकार है, हर नए समन्वयक के साथ बढ़ता है, और जो भी भरोसे पर विचार करे उसे यह अंतर जानना चाहिए।',
  'co-safe-t':'दूसरा समन्वयक सुरक्षित क्यों है',
  'co-safe-d':'हमेशा नहीं था। अगस्त 2026 तक <strong style="color:var(--text)">एक मनुष्य, एक पंजीकरण</strong> का वादा समन्वयक के भीतर के एक Redis लॉक पर टिका था — और दो स्वतंत्र समन्वयक Redis साझा नहीं करते, इसलिए एक ही व्यक्ति के दो एक साथ पंजीकरण दोनों निकल जाते। अब <strong style="color:var(--text)">हर सत्यापक स्वयं जाँचता है</strong>, अपने लिखने से पहले, कि वह चेहरा पहले से दर्ज है या नहीं। यह गारंटी अब किसी साझा सेवा या साझा रहस्य पर निर्भर नहीं, इसलिए कोई समन्वयक जुड़ या हट सकता है और इसमें कुछ नहीं बदलता।',
  'co-need-t':'आपको क्या चाहिए',
  'co-need-d':'एक पंजीकृत Aequitas खाता — वही नियम जो ब्लॉक बनाने और सत्यापन पर लागू है: एक मनुष्य, एक कुंजी। Docker वाला एक सर्वर और एक सार्वजनिक HTTPS पता, क्योंकि ब्राउज़र असुरक्षित पृष्ठ को कैमरा नहीं सौंपते। और अपनी दो कुंजियाँ, जो आप स्वयं बनाते हैं और जो आपकी मशीन कभी नहीं छोड़तीं: एक आपके प्रमाणनों पर हस्ताक्षर करती है, दूसरी बटुआ-पतों को चिह्नों में बदलती है।',
  'co-keys-t':'किसी से कुंजी कभी न लें — हमसे भी नहीं',
  'co-keys-d':'एक ही हस्ताक्षर-कुंजी साझा करने वाले दो समन्वयक दो समन्वयक नहीं हैं; वे दो पतों वाला एक हैं, और जो कोरम लोगों की रक्षा करने वाला है वह पूरा दिखेगा जबकि होगा नहीं। दोनों कुंजियाँ अपनी मशीन पर, अपनी यादृच्छिकता से बनाएँ, और किसी को भी बाहर न जाने दें।',
  'co-auth-t':'अपनी कुंजी अधिकृत करना — किसी अनुमति की ज़रूरत नहीं',
  'co-auth-d':'जब तक आपकी कुंजी अधिकृत नहीं, सत्यापक उसके हस्ताक्षरित हर चीज़ को अस्वीकार करते हैं। अधिकृत करने के लिए दो प्रमाण चाहिए और किसी की स्वीकृति नहीं: आपका बटुआ हस्ताक्षर करता है कि इस कुंजी के पीछे एक पंजीकृत मनुष्य है, और आपका समन्वयक अपने ही होस्ट पर सिद्ध करता है कि कुंजी सचमुच उसी की है। पहला ऊपर के बटन से बनाइए; दूसरा आपका समन्वयक स्वयं बनाता है। अगस्त 2026 तक हमसे एक साझा रहस्य भी चाहिए था — और वही रहस्य दरअसल अनुमति <em>था</em>। वह हट चुका है।',
  'co-pernode-t':'यह रजिस्टर हर नोड का अपना है, और यह जानबूझकर है',
  'co-pernode-d':'एक नोड पर लिखा अधिकार दूसरों तक नहीं जाता — इसके लिए न कोई लेनदेन है, न प्रसारण। प्रतिकृत विश्वास-सूची ठीक वही केंद्रीय सत्ता होती जिसके बिना यह प्रणाली बनी है: हर संचालक स्वयं तय करता है कि उसका नोड किसके प्रमाणन स्वीकारे। इसकी कीमत यह है कि आपका अधिकार हर उस नोड को भेजना होगा जिसे उसे मानना है। हस्ताक्षर स्वयं स्थानांतरणीय है: एक बार हस्ताक्षर कीजिए और सब जगह भेजिए; छोड़ा हुआ नोड बस आपको अस्वीकार करता रहेगा।',
  'co-law-t':'आप दूसरों के बारे में क्या जानते हैं — और उससे क्या निकलता है',
  'co-law-d':'कैप्चर आपसे होकर गुज़रता है; आप उसे आगे देते हैं और कुछ नहीं रखते। पर जो लोग आपके ज़रिये पंजीकरण करते हैं, उनके लिए बटुआ-पते और चिह्न के बीच का संबंध केवल आपके पास है — इसीलिए आपकी चिह्न-कुंजी आपकी ही रहनी चाहिए: साझा होने पर कोई भी संचालक किसी भी सार्वजनिक पते के लिए चिह्न निकालकर देख सकता है कि वह चेहरा किसका है। इसका अर्थ यह भी है कि GDPR के तहत आप उन लोगों के लिए <strong style="color:var(--text)">डेटा नियंत्रक</strong> बन जाते हैं। हम नहीं। पहुँच, मिटाने और आपत्ति के अनुरोध आप तक पहुँचेंगे, और यह औपचारिकता नहीं है।',
  'co-limit-t':'इससे उपजी एकमात्र सीमा',
  'co-limit-d':'बटुआ-पते से मिटाना केवल उसी समन्वयक पर काम करता है जहाँ पंजीकरण हुआ था: आपका चिह्न आपकी कुंजी पर टिका है, और दूसरा समन्वयक उसी पते के लिए अलग चिह्न निकालेगा। इसलिए कहीं और से आया «नहीं मिला» का अर्थ है «यहाँ पंजीकृत नहीं», न कि «पंजीकृत नहीं» — और उत्तर यही कहता भी है। व्यक्ति के अपने bio_hash वाला रास्ता, जो उसी का है और जिसे किसी संचालक की ज़रूरत नहीं, हर समन्वयक पर काम करता है, क्योंकि वह पहचानकर्ता वही रहता है.',
  'x-authorize-coordinator-key':'🔑 समन्वयक कुंजी अधिकृत करें',
  'x-ghostdag-2018-one-true-order':'GHOSTDAG (2018) — उलझे ग्राफ़ से एक सही क्रम',
  'x-ghostdag-knightdag':'GHOSTDAG + KnightDAG',
  'x-gini-coefficient':'जिनी गुणांक',
  'x-gini-coefficient-0-1':'जिनी गुणांक (0–1)',
  'x-gini-index-history':'जिनी सूचकांक का इतिहास',
  'x-gini-target-scandinavian-level':'जिनी लक्ष्य (स्कैंडिनेवियाई स्तर)',
  'x-github-open-source':'GitHub — Open Source',
  'x-go-1-24-chain-node':'Go 1.24 (chain) · Node.js (proof server)',
  'x-groth16-bn128':'Groth16 / BN128',
  'x-groth16-snarkjs-circom':'Groth16 / snarkjs / circom',
  'x-groth16-zkp-zero-knowledge':'Groth16 ZKP (शून्य-ज्ञान)',
  'x-guardian-system-8212-human-failsafe':'विश्वासपात्र &#8212; खोए बटुओं के लिए मानवीय सुरक्षा',
  'x-hash-wallet':'हैश / बटुआ',
  'x-healthier-than-most-nations-on':'पृथ्वी के अधिकांश देशों से बेहतर। स्कैंडिनेविया (0.27) और जर्मनी (0.31) के बराबर। संपत्ति सीमा और ठहराव शुल्क निष्पक्ष वितरण बनाए रखते हैं।',
  'x-higher-than-most-european-nations':'अधिकांश यूरोपीय देशों से अधिक — ब्राज़ील (0.53) या रूस के बराबर। प्रोटोकॉल का पुनर्वितरण बढ़ी हुई तीव्रता पर।',
  'x-honest-limitation':'स्वीकृत सीमा:',
  'x-how-it-works':'यह कैसे काम करता है',
  'x-how-to-read-this-chart':'यह चार्ट कैसे पढ़ें:',
  'x-https-aequitas-digital-rpc':'https://aequitas.digital/rpc',
  'x-humans-could-register':'लोग पंजीकरण कर सकते हैं',
  'x-imagine-a-world-where-every':'«ऐसी दुनिया की कल्पना कीजिए जहाँ पृथ्वी का हर व्यक्ति &#8212; चाहे वह कहीं भी जन्मा हो, कोई भी भाषा बोलता हो, या उसके माता-पिता के पास कितना भी धन रहा हो &#8212; केवल मनुष्य होने के नाते एक सुनिश्चित दैनिक आय पाता है। दान के रूप में नहीं। एक गणितीय अधिकार के रूप में, जिसे ऐसा कोड लागू करता है जिसे कोई सरकार या निगम रद्द नहीं कर सकता।»',
  'x-inactive-escrow':'निष्क्रियता पर अमानत',
  'x-inactivity-timeline':'निष्क्रियता की समय-रेखा',
  'x-ip4-173-249-37-118':'/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm',
  'x-ip4-194-163-188-71':'/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN',
  'x-keccak256-post-quantum-safe':'keccak256 (क्वांटम-पश्चात सुरक्षित)',
  'x-key-protections':'मुख्य सुरक्षाएँ:',
  'x-knightdag-2026-aequitas-s-own':'◆ KNIGHTDAG (2026) — स्थिर K वाले GHOSTDAG से आगे Aequitas का अपना विकास',
  'x-knightdag-secured':'· KnightDAG द्वारा सुरक्षित',
  'x-libp2p-bootstrap-addresses':'LIBP2P BOOTSTRAP ADDRESSES',
  'x-libp2p-go-implementation':'libp2p (Go implementation)',
  'x-like-scandinavia-0-27':'स्कैंडिनेविया जैसा (~0.27)',
  'x-liquidity-pool-30':'तरलता कोष (30 %)',
  'x-loading-blocks':'ब्लॉक लोड हो रहे हैं…',
  'x-loading-topology':'नेटवर्क संरचना लोड हो रही है…',
  'x-loading-transactions':'लेन-देन लोड हो रहे हैं…',
  'x-lorenz-curve':'Lorenz Curve',
  'x-lorenz-curve-aeq-distribution-across':'लॉरेंज़ वक्र — लोगों के बीच AEQ का वितरण',
  'x-max-5-min-n-25':'max(5, min(N, 25))× average AEQ balance',
  'x-metamask-mobile-if-aeq-balance':'📱 MetaMask मोबाइल: यदि पंजीकरण के बाद AEQ शेष 0 दिखे, तो सेटिंग्स → नेटवर्क → Aequitas शृंखला हटाएँ → इसी वेबसाइट से दोबारा जोड़ें',
  'x-metamask-mobile-if-aeq-shows':'📱 MetaMask मोबाइल: यदि जोड़ने के बाद AEQ 0 दिखे, तो नेटवर्क हटाकर ऊपर वाले बटन से दोबारा जोड़ें।',
  'x-money-exists-because-people-exist':'पैसा इसलिए है क्योंकि लोग हैं। इसलिए हर व्यक्ति को उसमें बराबर हिस्सा मिलना चाहिए, केवल मनुष्य होने के नाते।',
  'x-money-exists-because-people-exist-2':'«पैसा इसलिए है क्योंकि लोग हैं। न इससे अधिक, न कम।»',
  'x-most-unequal-currency-ever':'अब तक की सबसे असमान मुद्रा',
  'x-multi-validator-network':'कई सत्यापकों वाला नेटवर्क',
  'x-n-lt-10-not-yet':'⚠ N&lt;10: अभी सार्थक नहीं',
  'x-no-snapshots-yet-first-one':'अभी कोई अभिलेख नहीं — पहला अगले वितरण के बाद सहेजा जाएगा।',
  'x-no-stake-blockchain':'बिना दाँव वाली ब्लॉकचेन',
  'x-node-operator-guide-pdf':'📄 नोड संचालक मार्गदर्शिका (PDF)',
  'x-node-operator-wallet-must-be':'NODE_OPERATOR_WALLET का Aequitas में पंजीकृत व्यक्ति होना आवश्यक है',
  'x-one-human-one-wallet-1':'एक व्यक्ति = एक बटुआ = 1,000 AEQ',
  'x-p2p-protocol':'P2P प्रोटोकॉल',
  'x-paid-out-daily':'प्रतिदिन भुगतान',
  'x-permanent-on-chain':'स्थायी · शृंखला पर',
  'x-phase-roadmap-8212-the-path':'चरण-योजना &#8212; वैश्विक पैमाने तक का रास्ता',
  'x-phase-transitions-are-automatic-8212':'चरण-परिवर्तन स्वतः होते हैं &#8212; लोगों की संख्या की सीमाएँ इन्हें शुरू करती हैं और अनुबंध इन्हें लागू करता है। न मतदान, न प्रशासनिक कुंजी।',
  'x-planned-post-beta':'नियोजित (बीटा के बाद)',
  'x-postgresql-persistent':'PostgreSQL (स्थायी)',
  'x-protocol-v7':'📜 Protocol V7',
  'x-provide-aeq-tusd-liquidity-to':'AEQ / tUSD तरलता दें और सभी अदला-बदली शुल्कों का 30 % कमाएँ, जो प्रतिदिन बाँटा जाता है।',
  'x-recorded-after-each-ubi-distribution':'हर मूल आय वितरण के बाद दर्ज होता है। दिखाता है कि नेटवर्क बढ़ने के साथ समानता कैसे विकसित होती है। जितना कम उतना बेहतर — लक्ष्य 0.30 से नीचे का जिनी।',
  'x-redistribution':'पुनर्वितरण',
  'x-run-a-node':'⚙️ नोड चलाएँ',
  'x-run-a-verifier':'⚙️ सत्यापक चलाएँ',
  'x-set-guardian':'🛡 विश्वासपात्र नियुक्त करें',
  'x-swap-fees-0-1':'अदला-बदली शुल्क (0.1 %)',
  'x-sybil-resistance-8212-current-state':'सिबिल प्रतिरोध &#8212; वर्तमान स्थिति, ईमानदारी से',
  'x-the-4-redistribution-mechanisms':'चार पुनर्वितरण तंत्र',
  'x-the-core-innovation':'मूल विचार',
  'x-the-matching-threshold-has-not':'मिलान की सीमा अभी वास्तविक चित्रों पर अंशांकित नहीं हुई है',
  'x-the-vision-8212-a-global':'दृष्टि &#8212; एक वैश्विक मूल आय प्रोटोकॉल',
  'x-the-year-is-2009-satoshi':'वर्ष 2009 है। सतोशी नाकामोतो बिटकॉइन जारी करते हैं। पहली बार मूल्य किन्हीं दो लोगों के बीच बिना बैंक के जा सकता है। एक सच्ची क्रांति। पर लगभग तुरंत ही कुछ गड़बड़ हो जाता है।',
  'x-this-is-not-a-0815':'यह एक-एक ब्लॉक बनाने वाली कोई साधारण ब्लॉकचेन नहीं है। Aequitas एक वास्तविक BlockDAG चलाता है, जिसे GHOSTDAG क्रमबद्ध करता है — और 2026 से KnightDAG सुरक्षित करता है, जो इसका अपना अनुकूली विकास है। हर शेष, हर भुगतान और हर संपत्ति-सीमा अंततः इसी तंत्र पर निर्भर है, ताकि एक ही, सर्वसम्मत इतिहास बने।',
  'x-today-beta':'आज (बीटा)',
  'x-today-this-verifies-one-device':'आज यह एक उपकरण की पुष्टि करता है, अभी एक अद्वितीय व्यक्ति की नहीं',
  'x-traditional-blockchain-wasted-work':'पारंपरिक ब्लॉकचेन — व्यर्थ गया श्रम',
  'x-treasury-10':'कोष (10 %)',
  'x-trusted-verified-human':'भरोसेमंद, सत्यापित व्यक्ति',
  'x-two-validators-produce-at-once':'दो सत्यापक एक साथ ब्लॉक बनाते हैं → एक जीतता है, दूसरा हटा दिया जाता है — श्रम व्यर्थ, और यह सीमित करता है कि नेटवर्क कितनी तेज़ी से सुरक्षित रूप से चल सकता है।',
  'x-ubi-pool-20':'मूल आय कोष (20 %)',
  'x-validators-pool-40':'सत्यापक कोष (40 %)',
  'x-view-source-on-github':'🐙 GitHub पर स्रोत देखें',
  'x-wealth-cap-multiplier-bootstrap-slider':'संपत्ति सीमा गुणक — आरंभिक चरण का स्लाइडर',
  'x-wealth-cap-overflow':'संपत्ति सीमा से अधिक की राशि',
  'x-wealth-distribution-analysis':'संपत्ति वितरण का विश्लेषण',
  'x-what-happens-when-someone-is':'यदि कोई अस्पताल में भर्ती हो, जेल चला जाए या उसकी मृत्यु हो जाए तो क्या होता है? अधिकांश क्रिप्टो प्रणालियों में खोया बटुआ हमेशा के लिए खो जाता है। Aequitas में निष्क्रियता के लिए तीन-स्तरीय पुनर्प्राप्ति है।',
  'x-what-is-a-guardian':'विश्वासपात्र क्या है?',
  'x-what-is-and-is-not':'क्या निजी है और क्या नहीं:',
  'x-what-would-a-cryptocurrency-look':'«ऐसी क्रिप्टोमुद्रा कैसी होती जो आरंभ से ही हर मनुष्य के प्रति न्यायसंगत होने के लिए बनाई गई हो?»',
  'x-why-a-normal-blockchain-isn':'साधारण ब्लॉकचेन क्यों पर्याप्त नहीं',
  'x-worse-than-any-country-on':'पृथ्वी के किसी भी देश से बदतर (दक्षिण अफ़्रीका का रिकॉर्ड: 0.63)। बिटकॉइन (0.85) के निकट। प्रोटोकॉल अधिकतम हस्तक्षेप पर — संपत्ति सीमा और पुनर्वितरण पूरी शक्ति से।',
  'x-year-2-180d':'वर्ष 2 +180 दिन',
  'x-zk-device-key-proof':'उपकरण कुंजी का ZK प्रमाण',
  'swap-price-flat':'इस अवधि में कोई सौदा नहीं — कीमत हिली ही नहीं। चार्ट ठीक काम कर रहा है; बाज़ार शांत है।',
  'mpc-optin-title':'वैकल्पिक — दोहरे पंजीकरण की जाँच में सहायता (तैयार, अभी सक्रिय नहीं)',
  'mpc-optin-desc':'तैयार है, पर अभी सेवा में नहीं। आगे चलकर आपका नोड यह जाँचने में मदद कर सकेगा कि कोई दो बार पंजीकरण न करे, बिना किसी का बायोमेट्रिक डेटा देखे: हर भागीदार केवल प्रत्येक टेम्पलेट का एक गणितीय हिस्सा रखता है — अकेले में वह मात्र शोर है — और वे मिलकर नई कैप्चर की तुलना करते हैं, इसलिए कोई एक मशीन कुछ भी पुनर्निर्मित नहीं कर सकती। आज यह रास्ता कुछ तय नहीं करता: दोहराव की जाँच इससे होकर नहीं जाती, और समिति स्वतः चुनी जाने के बजाय एक निश्चित सूची है।',
  'mpc-optin-note':'हिस्सा-फ़ाइल में एक-बार-प्रयोग की यादृच्छिकता होती है जिसे केवल आपका नोड रख सकता है — इसे कभी किसी दूसरी मशीन पर न कॉपी करें और कहीं कमिट न करें। फ़िलहाल यह संचालक से ही आनी चाहिए, और यही शेष केंद्रीकृत निर्भरता है। आपको नई कुंजी की ज़रूरत नहीं: आपका नोड उसी हस्ताक्षर-कुंजी से पहचान देता है जो वह ब्लॉकों के लिए पहले से उपयोग करता है।',
  'logo-sub':'मानवता का प्रमाण','live':'लाइव',
  'reg-title':'🔐 सत्यापित मानव के रूप में रजिस्टर करें',
  'reg-sub':'Aequitas नेटवर्क से जुड़ें और 1,000 AEQ का यूनिवर्सल बेसिक इनकम अनुदान प्राप्त करें। रजिस्ट्रेशन एक बार, स्थायी और पूरी तरह निःशुल्क है। कोई व्यक्तिगत डेटा संग्रहीत नहीं किया जाता।',
  'app-title':'एंड्रॉयड ऐप के माध्यम से रजिस्ट्रेशन',
  'app-text':'पंजीकरण के समय कैमरा आपका चेहरा और एक छोटा जीवंतता अनुक्रम कैप्चर करता है। स्वतंत्र मिलान सेवाएँ जाँचती हैं कि कोई जीवित व्यक्ति मौजूद है और यह चेहरा पहले से पंजीकृत नहीं है; उन्हें कोरम से सहमत होना होता है। फिर एक Groth16 ZK प्रमाण परिणाम को चेन तक ले जाता है, आपके बारे में कुछ भी उजागर किए बिना। सत्यापन के बाद आपके <strong style="color:var(--gold)">1,000 AEQ स्वतः जमा हो जाते हैं</strong>। <strong style="color:var(--gold)">ध्यान दें:</strong> मिलान सीमा अभी वास्तविक कैप्चर पर कैलिब्रेट नहीं हुई है — नीचे FAQ देखें।',
  's1t':'चेहरे की कैप्चर','s1d':'ऐप आपका चेहरा और एक छोटा जीवंतता अनुक्रम रिकॉर्ड करता है और उन्हें स्वतंत्र मिलान सेवाओं को भेजता है। वे जाँचते हैं कि सामने कोई जीवित व्यक्ति है और चेहरे की तुलना पहले से पंजीकृत सभी लोगों से करते हैं। प्रोसेसिंग के बाद छवियाँ हटा दी जाती हैं।',
  's2t':'ZK प्रमाण जनरेशन','s2d':'एक Groth16 ZK प्रमाण आपके bio_hash को commitment = keccak256(bioHash‖wallet) में बिना उजागर किए प्रतिबद्ध करता है। nullifier उसी हैश से व्युत्पन्न होता है, इसलिए वही चेहरा दो बार नहीं गिना जा सकता — नीचे FAQ देखें।',
  's3t':'वॉलेट कनेक्ट करें','s3d':'ऐप इस पेज पर MetaMask खोलती है · अपना Ethereum वॉलेट कनेक्ट करें · प्रमाण आपके वॉलेट पते से क्रिप्टोग्राफिक रूप से जुड़ा है',
  's4t':'1,000 AEQ प्रदान','s4d':'Aequitas BlockDAG पर 6 सेकंड में रजिस्ट्रेशन की पुष्टि · 1,000 AEQ तुरंत जमा · आपकी पहचान स्थायी रूप से दर्ज',
  'priv-bar':'🔒 कोरम द्वारा चेहरे की जाँच · Groth16 ZKP · जाँच के बाद छवियाँ हटाई गईं · प्रति व्यक्ति एक पंजीकरण',
  'conn-wallet':'कनेक्टेड वॉलेट','proof-recv':'⚡ ZK प्रमाण प्राप्त','proof-hint':'रजिस्टर करने के लिए वॉलेट कनेक्ट करें',
  'btn-conn':'🦊 METAMASK कनेक्ट करें','btn-reg':'🔐 ON-CHAIN रजिस्टर करें',
  'btn-wc':'🔗 WALLETCONNECT कनेक्ट करें',
  'reg-log-hint':'// अपना प्रमाण उत्पन्न करने के लिए Aequitas Android App खोलें, फिर यहाँ वापस आएं...',
  'reg-details':'रजिस्ट्रेशन विवरण','k-network':'नेटवर्क','k-chainid':'चेन ID','k-grant':'UBI अनुदान',
  'k-fee':'गैस शुल्क','free':'निःशुल्क — पूरी तरह गैसलेस','k-limit':'रजिस्ट्रेशन','k-limit-v':'प्रति व्यक्ति एक बार · स्थायी · अपरिवर्तनीय',
  'k-bio':'चेहरा','never-stored':'जाँच के बाद छवियाँ हटा दी जाती हैं — किसी सत्यापनकर्ता के पास पूरा टेम्पलेट नहीं होता',
  'k-proof':'प्रमाण प्रणाली','k-conf':'पुष्टि','k-conf-v':'1 सेकंड के भीतर (1 ब्लॉक)',
  'k-sybil':'Sybil सुरक्षा','k-sybil-v':'प्रति व्यक्ति एक पहचान · चेहरे से बँधी, सीमा अभी कैलिब्रेट नहीं',
  's-height':'ब्लॉक हाइट',
  's-humans':'सत्यापित मनुष्य',
  's-supply':'कुल आपूर्ति','s-supply-sub':'हमेशा = मनुष्य × 1,000 AEQ',
  's-uptime':'अपटाइम',
  'k-chain':'चेन नाम','k-symbol':'प्रतीक','k-btime':'ब्लॉक समय',
  'k-cons':'सहमति','k-storage':'स्टोरेज','k-dec':'दशमलव',
  'btn-add-mm':'+ AEQUITAS नेटवर्क जोड़ें',
  'humans-title':'Aequitas Chain पर सत्यापित मनुष्य',
  'h-what':'सत्यापित मानव क्या है?','h-what-t':'एक सत्यापित मानव एक वॉलेट पता है जिसके बारे में सिद्ध है कि वह ऐसे व्यक्ति का है जिसका चेहरा पहले से पंजीकृत नहीं है। स्वतंत्र मिलान सेवाओं को कोरम से सहमत होना होता है, और चेन तक केवल एक Groth16 ZK प्रमाण पहुँचता है — कोई छवि नहीं और कोई टेम्पलेट नहीं। <strong style="color:var(--gold)">23-08-2026 तक यह एक उपकरण को सत्यापित करता था, एक व्यक्ति को नहीं; अब ऐसा नहीं है।</strong>',
  'h-zkp':'ZK प्रमाण प्रणाली','h-zkp-t':'Aequitas BN128 पर Groth16 उपयोग करता है — Ethereum और Zcash जैसा ही वक्र। ~200 बाइट, ~10ms। commitment = keccak256(deviceKey‖wallet)। Nullifier इस डिवाइस से बंधा है: फोन खोने से इस डिवाइस पर दूसरी पहचान नहीं बनती, लेकिन कोई अन्य डिवाइस अभी भी अलग से पंजीकरण कर सकता है। कुंजी सामग्री कभी सर्वर साइड पर प्रकट या संग्रहीत नहीं होती।',
  'h-sybil':'Sybil प्रतिरोध — वर्तमान स्थिति','h-sybil-t':'nullifier आपके चेहरे के bio_hash से व्युत्पन्न होता है, इसलिए वही चेहरा दो बार पंजीकृत नहीं हो सकता — उपकरणों के आर-पार भी, जो एक डिवाइस कुंजी कभी नहीं कर सकी। यह जिस पर टिका है वह एक मिलान सीमा है जो अभी वास्तविक कैप्चर पर कैलिब्रेट नहीं हुई: क्रिप्टोग्राफी सटीक है, उसके नीचे की बायोमेट्रिक्स एक माप है जिसकी त्रुटि दर अभी अज्ञात है।',
  'h-global':'वैश्विक वित्तीय समावेशन','h-global-t':'किसी बैंक खाते, क्रेडिट कार्ड या पूर्व क्रिप्टो अनुभव की आवश्यकता नहीं। बस कैमरे वाला एक एंड्रॉइड स्मार्टफोन। Aequitas को इस तरह बनाया गया है कि यह पृथ्वी के हर मनुष्य के लिए सुलभ हो।',
  'h-bio-hw':'पहचान सत्यापन रोडमैप','h-bio-hw-t':'आज (बीटा): स्वतंत्र मिलान सेवाओं के माध्यम से चेहरे की जाँच, जिन्हें कोरम से सहमत होना होता है। इसकी सीमा अभी वास्तविक कैप्चर पर कैलिब्रेट नहीं है — कोई भी संख्या बताने से पहले लगभग 1000 इम्पोस्टर जोड़े चाहिए। नियोजित: वही कैलिब्रेशन, और एक डुप्लिकेट जाँच जिसमें कोई सेवा पूरा टेम्पलेट न रखे।',
  'reg-humans':'रजिस्टर्ड मनुष्य','h-desc':'नीचे का हर पता ऐसे व्यक्ति का है जिसके चेहरे की स्वतंत्र सेवाओं ने सभी मौजूदा पंजीकरणों से तुलना की, ZK प्रमाण से सिद्ध किया, और ठीक 1,000 AEQ जमा किए। रजिस्ट्री स्थायी, अपरिवर्तनीय और ऑन-चेन है। सीमा आज क्या सुनिश्चित करती है और क्या नहीं, यह FAQ में है।',
  'no-humans':'अभी तक कोई मानव रजिस्टर्ड नहीं।\n\nAequitas Android App डाउनलोड करें और चेन पर पहले मानव बनें!',
  'reg-stats':'रजिस्ट्री आँकड़े','total-humans':'कुल मनुष्य',
  'idx-title':'Aequitas इंडेक्स — रियल-टाइम आर्थिक समानता स्कोर',
  'idx-desc':'Aequitas इंडेक्स <strong style="color:var(--teal)">जिनी गुणांक</strong> से लिया गया है — विश्व बैंक, OECD और UN द्वारा अपनाया गया अंतरराष्ट्रीय मानक। <strong style="color:var(--neon)">0 = पूर्ण समानता</strong>। <strong style="color:var(--red)">100 = अधिकतम एकाग्रता</strong>। लक्ष्य: जिनी 0.30 से कम।',
  'gini-what-title':'जिनी गुणांक क्या है?',
  'gini-what-text':'इतालवी सांख्यिकीविद् कोर्राडो जिनी (1912) द्वारा विकसित। धन वितरण मापता है। पैमाना: 0 (सब समान) से 1 (एक व्यक्ति के पास सब कुछ)। विश्व बैंक, OECD, UN उपयोग करते हैं।',
  'gini-calc-title':'Aequitas इंडेक्स की गणना कैसे होती है?','gini-calc-text':'सभी सत्यापित मानवों के AEQ बैलेंस एकत्र किए जाते हैं। फॉर्मूला हर संभावित जोड़ी के बैलेंस के बीच माध्य निरपेक्ष अंतर की गणना करता है, जनसंख्या वर्ग (n²) और माध्य बैलेंस से सामान्यीकृत। परिणाम 0–1 को 100 से गुणा = Aequitas इंडेक्स।',
  'gini-why-title':'जिनी ही क्यों — कोई सरल मेट्रिक नहीं?','gini-why-text':'सरल अमीर-गरीब अनुपात में हेरफेर आसान है — जिनी इसे पकड़ लेता है। यह गुणांक सभी सत्यापित मानवों के बीच पूर्ण वितरण को एक ऑडिट योग्य संख्या में दर्शाता है। Aequitas इसे ऑन-चेन प्रकाशित करता है — पारदर्शी, विश्व स्तर पर सत्यापन योग्य।',
  'curr-idx':'वर्तमान इंडेक्स','bar-0':'0 — पूर्ण समानता','bar-100':'100 — अधिकतम असमानता','wcap-lbl':'वर्तमान धन सीमा:','wcap-mult':'गुणक:','wcap-avg':'उचित हिस्सा:',
  'phases-desc':'चरण 0 (बूटस्ट्रैप) में धन सीमा एक स्लाइडिंग गुणक का उपयोग करती है: max(5, min(N, 25)) × औसत बैलेंस। 1–4 मनुष्यों के साथ: 5× औसत। हर नया मनुष्य 1× जोड़ता है। 25+ मनुष्यों पर: स्थायी रूप से 25× पर स्थिर। सभी बदलाव मनुष्यों की संख्या से स्वचालित रूप से होते हैं — कोई गवर्नेंस वोट नहीं, कोई एडमिन कुंजी नहीं।',
  'wealth-cap-explain':'चरण 0 (बूटस्ट्रैप) में धन सीमा max(5, min(N, 25)) × औसत AEQ बैलेंस का उपयोग करती है, जहाँ N = पंजीकृत मनुष्य। 1–4 मनुष्य: सीमा = 5× औसत। हर नया मनुष्य 1× जोड़ता है। 25+ पर: स्थायी रूप से 25× पर स्थिर। सीमा हमेशा वर्तमान औसत बैलेंस के साथ बदलती है।',
  'p0':'बूटस्ट्रैप · 100 से कम मनुष्य · धन सीमा: max(5,min(N,25))× औसत · 25वें मनुष्य तक 5× से 25× तक बढ़ता है · वर्तमान में सक्रिय',
  'p1':'विकास · 100–10,000 मनुष्य · धन सीमा: 25× उचित हिस्सा = 25,000 AEQ',
  'p2':'स्थिरता · 10,000–1M मनुष्य · धन सीमा: 25× उचित हिस्सा = 25,000 AEQ',
  'p3':'परिपक्वता · 1M+ मनुष्य · धन सीमा: 25× उचित हिस्सा = 25,000 AEQ',
  'gini':'जिनी गुणांक','gini-desc':'0 = समान · 1 = असमान',
  'supply-desc':'हमेशा = मनुष्य × 1,000 AEQ',
  'phase':'प्रोटोकॉल चरण','phase-desc':'मानवों की संख्या से स्वचालित रूप से आगे बढ़ता है',
  'humans-desc':'चेहरे से सत्यापित पंजीकरण',
  'pools-title':'पुनर्वितरण पूल',
  'pools-desc':'प्रत्येक स्वैप शुल्क, डेमरेज और धन सीमा अधिशेष स्वचालित रूप से चार पूलों में विभाजित होता है। सभी पूल दैनिक भुगतान करते हैं।',
  'vel-pool':'वैलिडेटर पूल','vel-pool-desc':'सभी शुल्कों का 40% → नोड ऑपरेटर जो नेटवर्क सुरक्षित करते हैं',
  'liq-pool':'लिक्विडिटी पूल','liq-pool-desc':'सभी शुल्कों का 30% → लिक्विडिटी प्रदाता, LP शेयर के अनुपात में',
  'ubi-pool':'UBI पूल','ubi-pool-desc':'सभी शुल्कों का 20% → सभी सत्यापित मनुष्यों को समान रूप से, हर 24 घंटे',
  'treasury':'ट्रेजरी','treasury-desc':'सभी शुल्कों का 10% → प्रोटोकॉल विकास और रखरखाव',
  'phases-title':'प्रोटोकॉल चरण',
  'demurrage-title':'डेमरेज — परिसंचरण के लिए प्रोत्साहन',
  'demurrage-desc':'निष्क्रिय AEQ बैलेंस धीरे-धीरे मूल्य खोते हैं ताकि संचय को हतोत्साहित किया जा सके।',
  'dem-rate-k':'क्षय दर','dem-rate-v':'0.5% प्रति माह (निरंतर)',
  'dem-grace-k':'ग्रेस पीरियड','dem-grace-v':'क्षय शुरू होने से पहले 3 महीने की निष्क्रियता',
  'dem-reset-k':'रीसेट','dem-reset-v':'कोई भी ट्रांसफर, स्वैप या लिक्विडिटी एक्शन टाइमर शून्य करता है',
  'dem-dest-k':'क्षयित AEQ जाता है','dem-dest-v':'पुनर्वितरण पूल में (40/30/20/10 विभाजन)',
  'dem-warn-k':'चेतावनी प्रणाली','dem-warn-v':'14 दिन की सूचना (एक बार) + हर लॉगिन पर 7 दिन का अनुस्मारक',
  'story-title':'Aequitas की कहानी',
  'nodes-title':'सक्रिय नोड्स — वर्तमान नेटवर्क टोपोलॉजी',
  'nodes-desc':'Aequitas नेटवर्क वर्तमान में कई भौगोलिक रूप से वितरित नोड्स पर चलता है (वर्तमान संख्या ऊपर)। सभी ब्लॉक उत्पादन, स्टेट सिंक्रोनाइज़ेशन और API सेवा में भाग लेते हैं।',
  'run-node-title':'अपना नोड चलाएं','run-node-desc':'हर पंजीकृत व्यक्ति एक Aequitas नोड चला सकता है — कोई स्टेक नहीं, कोई आवेदन नहीं, हमारी कोई अनुमति नहीं। एक व्यक्ति, एक वैलिडेटर कुंजी: जिस नोड का NODE_OPERATOR_WALLET पंजीकृत व्यक्ति नहीं है, उसे HTTP 403 के साथ अस्वीकार किया जाता है, अन्यथा एक ही व्यक्ति चुपचाप पूरा वैलिडेटर समूह बन सकता था। ऑपरेटर दैनिक वितरित स्वैप शुल्क का 40% कमाते हैं।',
  'bootstrap-title':'नया नोड कनेक्ट करें','bootstrap-desc':'कोई एंट्री पॉइंट कॉन्फ़िगर नहीं करना है — वैलिडेटर पते अंतर्निहित हैं। आपका नोड स्वयं रजिस्टर होता है, पूरी चेन स्थिति सिंक करता है और ब्लॉक उत्पादन में भाग लेता है। PRIMARY_NODE_URL केवल तभी सेट करें जब आप जानबूझकर एक विशिष्ट एंट्री पॉइंट तय करना चाहते हों।',
  'tech-title':'तकनीकी विशिष्टताएं','mm-config':'MetaMask कॉन्फ़िगरेशन',
  'k-lang':'भाषा','k-src':'स्रोत','evm-yes':'हाँ — JSON-RPC /rpc · MetaMask संगत',
  'proto-label':'Aequitas V7 प्रोटोकॉल — तकनीकी दस्तावेज़ीकरण',
  'ca-title':'अनुबंध पते',
  'ca-text':'चेन: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: __RPC__<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 Aequitas अर्थव्यवस्था के नियम तय करता है और वह रजिस्ट्री रखता है जो उन्हें लागू करने योग्य बनाती है: हर इस्तेमाल हुआ nullifier, हर पंजीकरण, संपत्ति सीमा और डिमरेज सूत्र। कॉन्ट्रैक्ट अपरिवर्तनीय है — कोई एडमिन की, अपग्रेड प्रॉक्सी या गवर्नेंस वोट उसकी एक पंक्ति भी नहीं बदल सकता। लेकिन असली ट्रांसफर को निपटाती है चेन लेयर: नोड ERC-20 कॉल को EVM तक पहुँचने से पहले रोकता है और अपने ही बहीखाते में दर्ज करता है — यही ट्रांसफर को एक सेकंड से कम और गैस-मुक्त बनाता है। कॉन्ट्रैक्ट नियम-पुस्तिका और रजिस्ट्री है; चेन वह इंजन है जो उन्हें चलाता है, और उसका स्रोत सार्वजनिक है।',
  'poa-title':'1. जीवन का प्रमाण','poa-text':'<p>जब लोग मरते हैं, उनका AEQ धीरे-धीरे UBI पूल के माध्यम से समुदाय को वापस जाता है, बजाय Bitcoin की तरह हमेशा के लिए खोने के।</p>',
  'poa-box':'वर्ष 0–2: सामान्य उपयोग<br>वर्ष 2: चेतावनी 1 — Guardian जवाब दे सकता है<br>वर्ष 2+60 दिन: चेतावनी 2<br>वर्ष 2+120 दिन: चेतावनी 3<br>वर्ष 2+180 दिन: AEQ व्यक्तिगत एस्क्रो में<br>वर्ष 4: निष्क्रिय रहने पर — UBI पूल में वापस',
  'guard-title':'2. गार्जियन सिस्टम','guard-text':'<p>एक विश्वसनीय Guardian (दूसरा सत्यापित मानव) पुष्टि कर सकता है कि कोई अभी भी जीवित है, बिना किसी वित्तीय अधिकार के।</p>',
  'guard-box':'प्रति मानव 1 Guardian · दूसरा सत्यापित मानव होना चाहिए<br>Guardian केवल confirmAlive() कॉल कर सकता है · शून्य वित्तीय अधिकार<br>Guardian धन नहीं हिला सकता · अधिकतम 3 · Timelock 7 दिन',
  'dem-title':'3. डेमरेज — संचय-विरोधी तंत्र',
  'dem-box':'केवल आपके उचित हिस्से से ऊपर के भाग पर लगाया जाता है — उससे कम या बराबर शेष कभी क्षय नहीं होता<br>दर: 3 महीने की छूट के बाद 0.5%/माह<br>किसी भी ट्रांसफर, स्वैप या लिक्विडिटी पर रीसेट<br>क्षयित AEQ पूलों में पुनर्वितरित (जला नहीं जाता)',
  'dem-text':'<p>ऐतिहासिक उदाहरण: Wörgl प्रयोग (ऑस्ट्रिया, 1932) — एक वर्ष में बेरोजगारी 25% कम। Chiemgauer (जर्मनी, 2003) — 20+ वर्षों से सफलतापूर्वक चल रहा है।</p>',
  'cap-title':'4. धन सीमा — गणितीय निष्पक्षता','cap-box':'सीमा: max(5,min(N,25))× औसत AEQ बैलेंस<br>1–4 मनुष्य: 5× · प्रति मानव +1× · 25+: 25× स्थायी<br>अतिरिक्त AEQ तुरंत पुनर्वितरित · कोई हस्तक्षेप नहीं',
  'ubi-title':'5. यूनिवर्सल बेसिक इनकम','ubi-box':'स्रोत: स्वैप शुल्क (20%) · सीमा अधिशेष · डेमरेज<br><br>दैनिक: UBI पूल सभी पंजीकृत मनुष्यों में समान रूप से विभाजित। प्रत्येक वितरण के बाद पूल शून्य हो जाता है।',
  'inf-title':'6. कोई एल्गोरिदमिक मुद्रास्फीति नहीं','inf-box':'केवल एक घटना नया AEQ बनाती है: नया सत्यापित मानव पंजीकृत होता है।<br><br>कुल आपूर्ति = सत्यापित मनुष्य × 1,000 AEQ — हमेशा, बिल्कुल।',
  'btn-download-app':'AEQUITAS ऐप डाउनलोड करें',
  'swap-title':'🔄 AEQ ↔ tUSD स्वैप करें','swap-sub':'नेटिव लिक्विडिटी पूल के माध्यम से AEQ को tUSD (सिमुलेटेड टेस्ट डॉलर) से बदलें। स्वैप के लिए केवल 0.1% शुल्क — सामान्य AEQ ट्रांसफर पूरी तरह निःशुल्क।',
  'swap-priv-bar':'🔒 केवल 0.1% स्वैप शुल्क · AEQ→AEQ ट्रांसफर निःशुल्क · tUSD कोई वास्तविक मूल्य के बिना टेस्ट मुद्रा है',
  'swap-your-aeq':'आपका AEQ','swap-your-tusd':'आपका tUSD',
  'swap-fee-est':'प्रोटोकॉल शुल्क (0.1%)','swap-details-hdr':'स्वैप विवरण',
  'swap-out-lbl':'आप प्राप्त करेंगे (अनुमानित)','swap-impact-lbl':'मूल्य प्रभाव','swap-rate-lbl':'विनिमय दर',
  'swap-depth-lbl':'पूल संरचना','amm-title':'x × y = k — कॉन्स्टेंट प्रोडक्ट AMM',
  'amm-text':'AEQ स्वैप करते समय AEQ रिजर्व बढ़ता है और tUSD रिजर्व घटता है — उनका गुणनफल हमेशा k के बराबर रहता है। बड़े स्वैप से मूल्य पर अधिक प्रभाव।',
  'swap-btn-go':'🔄 स्वैप करें',
  'swap-log-hint':'// स्वैप करने के लिए वॉलेट कनेक्ट करें...',
  'swap-no-liquidity':'अभी tUSD नहीं है?','swap-faucet-desc':'पंजीकृत मनुष्य एक बार टेस्ट tUSD का दावा कर सकते हैं','swap-btn-faucet':'💧 टेस्ट tUSD का दावा करें',
  'swap-addliq-title':'लिक्विडिटी प्रदान करें','swap-addliq-desc':'पहले डिपॉजिट करें — आपका अनुपात प्रारंभिक मूल्य तय करता है।','swap-btn-addliq':'💧 लिक्विडिटी जोड़ें',
  'swap-lp-title':'आपकी LP स्थिति','swap-lp-share':'पूल हिस्सा','swap-lp-withdrawable':'निकालने योग्य',
  'swap-lp-pct-label':'आपकी स्थिति का %','swap-lp-youget':'आप प्राप्त करेंगे','swap-btn-removeliq':'🔥 लिक्विडिटी हटाएं',
  'swap-pool-title':'AEQ / tUSD — पूल स्थिति',
  'swap-pool-aeq':'AEQ रिजर्व','swap-pool-tusd':'tUSD रिजर्व','swap-pool-price':'स्पॉट मूल्य',
  'swap-fee-bps':'स्वैप शुल्क',
  'swap-pools-addr-title':'टोकनोमिक्स पूल पते',
  'swap-validators':'वैलिडेटर (40%)','swap-lps':'लिक्विडिटी प्रदाता (30%)','swap-ubi':'UBI पूल (20%)','swap-treasury':'ट्रेजरी (10%)',
  'ubi-hero-title':'यूनिवर्सल बेसिक इनकम — UBI पूल',
  'ubi-hero-sub':'जमा हो रहा है — अगला भुगतान सभी सत्यापित मनुष्यों को समान रूप से वितरित:',
  'ubi-bal-lbl':'वर्तमान पूल बैलेंस','ubi-hero-desc':'समान रूप से विभाजित · हर 24 घंटे भुगतान · पूल शून्य होता है · न्यूनतम बैलेंस की जरूरत नहीं',
  'ubi-how-fills':'UBI पूल कैसे भरता है',
  'ubi-src-swap':'स्वैप शुल्क','ubi-src-swap-d':'प्रत्येक AEQ↔tUSD स्वैप अपने 0.1% शुल्क का 20% योगदान देता है।',
  'ubi-src-dem':'डेमरेज','ubi-src-dem-d':'निष्क्रिय AEQ (3+ माह) 0.5%/माह क्षय होता है। क्षयित राशि का 20% UBI में जाता है।',
  'ubi-src-cap':'सीमा अधिशेष','ubi-src-cap-d':'सीमा से अधिक वॉलेट तुरंत कटते हैं। 20% UBI में प्रवाहित होता है।',
  'pools4-header':'चारों पुनर्वितरण पूल',
  'ubi-see-above':'ऊपर काउंटडाउन देखें','ubi-timer-above':'⏰ काउंटडाउन ऊपर दिखाया गया','pool-t-timer':'जमा हो रहा है — कोई टाइमर नहीं',
  'usp-headline':'इतिहास में पहली बार — सब एक समान से शुरू करते हैं',
  'usp-sub':'अगर आपके पास Android स्मार्टफोन है तो आप पात्र हैं। बिना बैंक, बिना क्रिप्टो ज्ञान, बिना निवेश।',
  'usp-c1-title':'₹0 प्रारंभिक निवेश','usp-c1-desc':'रजिस्ट्रेशन पूरी तरह निःशुल्क। कोई ETH, MATIC या क्रेडिट कार्ड नहीं। प्रोटोकॉल सभी लागत वहन करता है।',
  'usp-c2-title':'प्रत्येक मानव के लिए 1,000 AEQ','usp-c2-desc':'अरबपति हो या किसान — सभी को बिल्कुल 1,000 AEQ मिलता है। गणितीय गारंटी के साथ समान शुरुआत।',
  'usp-c3-title':'सभी के लिए सुलभ','usp-c3-desc':'कोई बैंक खाता नहीं, कोई क्रेडिट कार्ड नहीं, कोई सरकारी पहचान नहीं, खरीदने के लिए कोई अतिरिक्त हार्डवेयर नहीं — बस वह कैमरा जो आपके एंड्रॉइड फोन में पहले से है।',
  'usp-c4-title':'हमेशा के लिए दैनिक UBI','usp-c4-desc':'पंजीकरण के बाद, आपका UBI हिस्सा हर दिन स्वचालित रूप से आता है — बिना किसी कार्रवाई के।',
  'v7-intro-title':'AequitasV7 क्या है?',
  'v7-intro-text':'AequitasV7, Aequitas प्रोटोकॉल का केंद्रीय स्मार्ट अनुबंध है। Aequitas Chain (ID 1926) पर अपरिवर्तनीय रूप से तैनात। सब कुछ प्रबंधित करता है: मानव पंजीकरण, ZK सत्यापन, बैलेंस प्रबंधन, धन सीमा, UBI वितरण, स्वैप शुल्क। कोई व्यवस्थापक इसे अपडेट नहीं कर सकता।',
  'swap-sell-label':'बेचें','swap-receive-label':'प्राप्त करें',
  'guard-title':'🛡 गार्जियन सिस्टम','guard-my-lbl':'मेरा गार्जियन','guard-none':'कोई नहीं',
  'guard-set-lbl':'गार्जियन सेट / बदलें','guard-set-hint':'Aequitas का पंजीकृत मानव होना आवश्यक · 7-दिन का टाइम लॉक · गार्जियन केवल आपकी जीवितता की पुष्टि कर सकता है, फंड तक नहीं पहुंच सकता · प्रति गार्जियन अधिकतम 3 वार्ड',
  'guard-confirm-lbl':'जीवित होने की पुष्टि करें (गार्जियन के रूप में)','guard-confirm-hint':'यदि आपका वार्ड अपने वॉलेट तक नहीं पहुंच सकता, तो 910 दिनों की निष्क्रियता के बाद उनके फंड एस्क्रो में जाने से रोकने के लिए उनकी जीवितता की पुष्टि करें।','guard-recover-btn':'🔓 एस्क्रो से वापस लें',
  'faq-title':'❓ सामान्य प्रश्न','faq-q1':'क्या मेरा बायोमेट्रिक डेटा सुरक्षित है?','faq-a1':'आपका चेहरा कैप्चर किया जाता है और स्वतंत्र मिलान सेवाओं को भेजा जाता है — «एक व्यक्ति, एक खाता» की जाँच केवल इसी तरह संभव है। छवियाँ संसाधित होकर फिर हटा दी जाती हैं; उन्हें संग्रहीत नहीं किया जाता। जो रखा जाता है वह एक गणितीय टेम्पलेट है: एन्क्रिप्टेड, और अलग-अलग संचालित सत्यापनकर्ताओं के बीच हिस्सों में बँटा हुआ, ताकि किसी भी सत्यापनकर्ता के पास कभी पूरा टेम्पलेट न हो। एक ईमानदार सीमा, छिपाई नहीं बल्कि बताई गई: तुलना चलाने वाली सेवा अब भी टेम्पलेट रखती है, क्योंकि तुलना के लिए वे ज़रूरी हैं।',
  'faq-q1b':'क्या पंजीकरण साबित करता है कि मैं एक अद्वितीय वास्तविक व्यक्ति हूं?','faq-a1b':'एक डिवाइस कुंजी जो कभी कर सकी उससे बेहतर, और अभी संख्या के रूप में सिद्ध नहीं। चेहरे की तुलना स्वतंत्र सेवाओं द्वारा सभी मौजूदा पंजीकरणों से की जाती है जिन्हें सहमत होना होता है, इसलिए दूसरे फोन पर वही व्यक्ति पकड़ा जाता है — जो एक डिवाइस कुंजी कभी नहीं कर सकी। जो तय नहीं है वह त्रुटि दर है: सीमा वास्तविक कैप्चर पर कैलिब्रेट नहीं है, और उसके लिए लगभग 1000 इम्पोस्टर जोड़े चाहिए।',
  'faq-q2':'क्या मैं बाद में अलग वॉलेट से रजिस्टर कर सकता/सकती हूं?','faq-a2':'नहीं। एक पंजीकरण स्थायी रूप से एक वॉलेट पते से बँधा होता है। यह जानबूझकर है: आपके चेहरे से व्युत्पन्न nullifier एक ही बार खर्च होता है, इसलिए किसी दूसरे वॉलेट से दोबारा पंजीकरण उसी व्यक्ति की दूसरी पहचान होगी।',
  'faq-q3':'अगर मैं अपना फोन खो दूं तो क्या होगा?','faq-a3':'आपके AEQ आपके वॉलेट में रहते हैं — वे आपकी प्राइवेट कुंजी से जुड़े हैं, फोन से नहीं। आप अभी भी अपने सीड फ्रेज से MetaMask के जरिए वॉलेट एक्सेस कर सकते हैं। वॉलेट रिकवरी बायोमेट्रिक पंजीकरण से स्वतंत्र है।',
  'path-title':'अपना रास्ता चुनें','path-human-title':'मैं एक मानव हूं','path-human-desc':'मैं पंजीकरण करना, 1,000 AEQ प्राप्त करना और बेसिक इनकम नेटवर्क में शामिल होना चाहता/चाहती हूं।','path-human-steps':'1. Aequitas Android ऐप डाउनलोड करें<br>2. अपने डिवाइस के स्क्रीन-लॉक से अनलॉक करें (फिंगरप्रिंट/फेस/PIN)<br>3. MetaMask कनेक्ट करें<br>4. तुरंत 1,000 AEQ प्राप्त करें',
  'path-node-title':'मैं एक नोड ऑपरेटर हूं','path-node-desc':'मैं पूर्ण नोड चलाना, ब्लॉक उत्पादन में भाग लेना और 40% वैलिडेटर पूल से कमाना चाहता/चाहती हूं।','path-node-steps':'1. मानव के रूप में रजिस्टर करें (अनिवार्य)<br>2. कोई एंट्री पॉइंट कॉन्फ़िगर नहीं करना — वैलिडेटर पते अंतर्निहित हैं<br>3. Contabo/Hetzner/किसी भी VPS पर डिप्लॉय करें<br>4. वैलिडेटर पूल से दैनिक कमाएं',
  'path-dev-title':'मैं एक डेवलपर हूं','path-dev-desc':'मैं Aequitas पर निर्माण करना, API को एकीकृत करना या प्रोटोकॉल में योगदान देना चाहता/चाहती हूं।','path-dev-steps':'1. EVM-संगत JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* एंडपॉइंट<br>4. मेट्रिक्स: /metrics (Prometheus)',
  'story-flow-title':'AEQ टोकन प्रवाह आरेख','story-topo-title':'नेटवर्क टोपोलॉजी — वर्तमान स्थिति',
  'swap-price-title':'AEQ / tUSD — लाइव मूल्य','swap-price-desc':'पूल रिज़र्व से रियल-टाइम मूल्य (x·y=k)। हर 8 सेकंड में नए पूल डेटा के साथ अपडेट।','swap-price-empty':'अभी पूल डेटा नहीं — मूल्य चार्ट देखने के लिए लिक्विडिटी जोड़ें।',
  'node-guide-lang-note':'यह इनलाइन गाइड अंग्रेज़ी में है। आपकी भाषा में PDF ऊपर के बटन से उपलब्ध है।',
  'k-zkp':'ZKP सिस्टम','k-hash':'हैश सिस्टम','k-sybil-prot':'Sybil सुरक्षा',
  'soc-title':'💬 सोशल मीडिया','soc-sub':'घोषणाएँ, चेन की स्थिति, और असहज सवाल &mdash; सार्वजनिक रूप से, दोनों पर।',
  'soc-x-desc':'घोषणाएँ, और चेन असल में क्या कर रही है। संक्षिप्त रूप।','soc-tg-desc':'खुला समूह: सवाल, नोड संचालक, और रजिस्टर करने में मदद।',
  's-validators':'सक्रिय वैलिडेटर',
  'expl-heading':'ब्लॉक एक्सप्लोरर',
}
};

function showStab(parentId, stabId, el) {
  const parent = document.getElementById(parentId);
  parent.querySelectorAll('.stab-panel').forEach(p => p.classList.remove('active'));
  parent.querySelectorAll('.stab').forEach(s => s.classList.remove('active'));
  document.getElementById(stabId).classList.add('active');
  el.classList.add('active');
  if (stabId === 'eqi-score') { setTimeout(function(){ drawGiniHistoryChart(); }, 30); }
  if (stabId === 'eqi-lorenz') { setTimeout(drawLorenzCurve, 30); }
  if (stabId === 'eqi-economy') { setTimeout(drawWcapSlideChart, 30); }
  // Push sub-route URL
  // FIX (2026-07-21): this map must have one entry per real stab-panel id
  // (see the <div id="..." class="stab-panel"> list in explorer.html) or a
  // reload silently loses the sub-tab — a panel missing here isn't visibly
  // broken while clicking around (showStab above already switched the
  // visible panel), it only surfaces on refresh, when activateTabFromPath
  // has no URL slug to restore and falls back to the tab's first panel.
  // 'eqi-charts' never matched any real id (the panel is 'eqi-story');
  // 'net-consensus'/'net-story' were missing outright — both silently
  // broken sub-tabs, reported live as "refresh jumps back to Overview or
  // Run a Node" for the Network tab specifically.
  const tabSlugMap = {'tab-register':'register','tab-explorer':'explorer','tab-index':'index','tab-network':'network','tab-exchange':'exchange'};
  const stabSlugMap = {'sep-blocks':'blocks','sep-humans':'humans','eqi-score':'score','eqi-lorenz':'distribution','eqi-economy':'economy','eqi-story':'story','net-overview':'overview','net-consensus':'consensus','net-story':'story','net-runnode':'node','net-protocol':'protocol','exch-swap':'swap','exch-liquidity':'liquidity'};
  const tabSlug = tabSlugMap[parentId];
  const stabSlug = stabSlugMap[stabId];
  if (tabSlug && stabSlug) history.pushState(null, '', '/' + tabSlug + '/' + stabSlug);
  syncActiveAria();
}

function showTab(name, el) {
  document.querySelectorAll('.tab-content').forEach(function(t) {
    t.classList.remove('active');
    t.style.display = ''; // clear any server-injected inline style
  });
  document.querySelectorAll('.tab').forEach(function(t) { t.classList.remove('active'); });
  const tabContent = document.getElementById('tab-' + name);
  if (!tabContent) return;
  tabContent.classList.add('active');
  el.classList.add('active');
  // Always activate first stab-panel when switching tabs
  var panels2 = tabContent.querySelectorAll('.stab-panel');
  var stabs2 = tabContent.querySelectorAll('.stab');
  panels2.forEach(function(p){p.classList.remove('active');});
  stabs2.forEach(function(s){s.classList.remove('active');});
  if (panels2.length) panels2[0].classList.add('active');
  if (stabs2.length) stabs2[0].classList.add('active');
  if (name === 'exchange') { loadPoolStatus(); preloadPriceHistory(); }
  history.pushState(null, '', '/' + name);
  syncActiveAria();
}

function goTab(name, stabId) {
  let el = null;
  document.querySelectorAll('.tab').forEach(t => {
    if ((t.getAttribute('data-args') || '').includes('"' + name + '"')) el = t;
  });
  if (el) showTab(name, el);
  if (stabId) {
    const stabEl = document.querySelector(`#tab-${name} .stab[data-args*='"${stabId}"']`);
    if (stabEl) showStab('tab-' + name, stabId, stabEl);
  }
}

// The chosen language has to outlive the page view. Every tab in the bar is a
// real URL served as its own document (/register, /network, ...), and the
// landing page at / is a separate document again — so "switching language"
// only ever applied to whatever was on screen at that second, and the next
// click put everything back to English. Storing the choice is what makes the
// selector mean anything at all.
const LANG_KEY = 'aeq_lang';

function setLang(lang) {
  if (!T[lang]) return;
  curLang = lang;
  const sel = document.getElementById('lang-sel');
  if (sel) sel.value = lang;
  document.documentElement.dir = lang === 'ar' ? 'rtl' : 'ltr';
  document.documentElement.lang = lang;
  try { localStorage.setItem(LANG_KEY, lang); } catch (e) { /* private mode — language then lasts this page view only */ }
  const t = T[lang];
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    // Translation strings may contain safe HTML (bold, emphasis) — this is intentional.
    // Never allow user-supplied content to flow into the T object.
    if (t[key] !== undefined) el.innerHTML = t[key];
  });
  // FIX (BRUTAL-P3-11): update the node-guide PDF download link to the
  // language-specific PDF when available. Falls back to English for languages
  // without a translated PDF (RU, ZH, AR, VI, HI, etc.).
  const pdfBtn = document.getElementById('node-guide-pdf-btn');
  const pdfLangs = {en:1,de:1,es:1,fr:1,id:1,it:1,pt:1,tr:1};
  if (pdfBtn) {
    const pdfLang = pdfLangs[lang] ? lang : 'en';
    pdfBtn.href = '/download/node-guide-' + pdfLang + '.pdf';
    pdfBtn.download = 'Aequitas_Node_Guide_' + pdfLang.toUpperCase() + '.pdf';
  }
  const langBanner = document.getElementById('node-guide-lang-banner');
  if (langBanner) langBanner.style.display = (pdfLangs[lang] && lang !== 'en') ? 'block' : 'none';
  // Re-apply the live block-time value into any translated string that
  // still has the __BT__ placeholder (see applyBlockTime's own comment) —
  // setLang() just overwrote innerHTML from the raw T[lang] dictionary
  // entry, which always contains the literal token, not a resolved value.
  applyBlockTime(lastKnownBlockTime);
  applyRpcUrl(); // same reasoning, see applyRpcUrl's own comment — no API data needed, so no guard to trip on
}

// lastKnownBlockTime + applyBlockTime: __BT__ is a placeholder embedded in
// every locale's blocks-desc string (and k-btime-val's plain text) instead
// of a hardcoded number — see this file's own 2026-07-05 audit fix history:
// BLOCK_TIME changed 4 times in one night and a hardcoded "~6 seconds"
// stayed wrong in 9 of 12 languages for hours because nothing ever
// re-checked it. Always reflects /api/status's own block_time field
// (loadStatus, called every 6s) instead of a number baked in at whatever
// moment this page happened to be written — this can never go stale again
// regardless of how many more times BLOCK_TIME changes.
let lastKnownBlockTime = null;
const BLOCKTIME_PHRASE = {
  en: n => '~' + n + ' second' + (n === 1 ? '' : 's'),
  de: n => '~' + n + ' Sekunde' + (n === 1 ? '' : 'n'),
  es: n => '~' + n + ' segundo' + (n === 1 ? '' : 's'),
  pt: n => '~' + n + ' segundo' + (n === 1 ? '' : 's'),
  fr: n => '~' + n + ' seconde' + (n === 1 ? '' : 's'),
  it: n => '~' + n + ' second' + (n === 1 ? 'o' : 'i'),
  ru: n => '~' + n + ' секунд' + (n === 1 ? 'а' : (n >= 2 && n <= 4 ? 'ы' : '')),
  tr: n => '~' + n + ' saniye',
  id: n => '~' + n + ' detik',
  zh: n => '约' + n + '秒',
  ar: n => '~' + n + ' ثانية',
  hi: n => '~' + n + ' सेकंड',
};
function applyBlockTime(blockTimeSeconds) {
  if (blockTimeSeconds === undefined || blockTimeSeconds === null) return;
  lastKnownBlockTime = blockTimeSeconds;
  const f = BLOCKTIME_PHRASE[curLang] || BLOCKTIME_PHRASE.en;
  const phrase = f(blockTimeSeconds);
  document.querySelectorAll('[data-i18n]').forEach(function(el) {
    if (el.innerHTML.indexOf('__BT__') !== -1) {
      el.innerHTML = el.innerHTML.split('__BT__').join(phrase);
    }
  });
  const specEl = document.getElementById('k-btime-val');
  if (specEl) specEl.textContent = phrase + ' average';
}

// FIX (audit 2026-08-16): the Protocol V7 docs card's RPC address was
// hardcoded per-locale to https://aequitas.digital/rpc, which doesn't
// resolve to this network until the 2026-08-18 domain switch — before that,
// every visitor reading the docs card saw an RPC endpoint that doesn't work.
// __RPC__ is the same placeholder mechanism as __BT__ above, but substituted
// in its OWN pass rather than folded into applyBlockTime: that function
// returns early whenever lastKnownBlockTime is still null (true on first
// load, before any /api/status response has arrived), which would have left
// __RPC__ sitting there as literal, visible text — unlike the block-time
// phrase, this value needs no API data at all, so it must not share that
// guard.
function applyRpcUrl() {
  const rpcUrl = window.location.origin + '/rpc';
  document.querySelectorAll('[data-i18n]').forEach(function(el) {
    if (el.innerHTML.indexOf('__RPC__') !== -1) {
      el.innerHTML = el.innerHTML.split('__RPC__').join(rpcUrl);
    }
  });
}

function fmt(n) {
  if (n === undefined || n === null) return '—';
  if (typeof n === 'number') return n.toLocaleString();
  return n;
}

function timeAgo(ts) {
  const d = Math.floor(Date.now() / 1000) - ts;
  if (d < 60) return d + 's ago';
  if (d < 3600) return Math.floor(d / 60) + 'm ago';
  return Math.floor(d / 3600) + 'h ago';
}

function short(h, s, e) {
  s = s || 8; e = e || 6;
  return h ? h.slice(0, s) + '...' + h.slice(-e) : '—';
}

function avatarColor(a) {
  const c = ['#4FC3F7', '#00E676', '#FFB300', '#CE93D8', '#EF5350', '#4DD0E1'];
  return c[parseInt((a || '0x00').slice(2, 4), 16) % c.length];
}

async function addToMetaMask() {
  if (!window.ethereum) { addLog('🦊 MetaMask not found — <a href="https://metamask.io/download/" target="_blank" style="color:var(--gold)">install MetaMask</a> to use this feature.', 'warn', true); return; }
  try {
    await window.ethereum.request({
      method: 'wallet_addEthereumChain',
      params: [{
        chainId: CID,
        chainName: 'Aequitas Chain',
        // AEQ is declared here as the chain's native currency (like ETH on
        // Ethereum) — MetaMask shows this automatically in the main
        // account balance display once eth_getBalance returns real
        // values, no further setup needed. We previously ALSO called
        // wallet_watchAsset below to add AEQ again as a separate ERC20
        // custom token. That meant AEQ showed up twice in MetaMask: once
        // correctly as the native balance, and once as an ERC20 entry
        // whose balance came from the V7 contract's balanceOf() mapping
        // instead — two numbers for "your AEQ" that could drift apart
        // (e.g. after a native transfer, only the native number changes,
        // while the ERC20 entry still shows the contract's value). Now
        // that registration and transfers write to the native balance,
        // the ERC20 entry no longer reflects the real, current state and
        // has been removed.
        nativeCurrency: { name: 'AEQ', symbol: 'AEQ', decimals: 18 },
        // FIX (audit 2026-08-16): hardcoded to https://aequitas.digital,
        // which does not resolve to this network until the 2026-08-18
        // domain switch (docs/LAUNCH_2026-08-18.md) — today it resolves to a
        // different, near-empty chain. Every visitor clicking "Add to
        // MetaMask" before that date got a wallet configured against a host
        // that answers with someone else's chain. window.location.origin is
        // whatever host actually served this page — the temporary IP today,
        // the real domain from 2026-08-18 onward — so this is correct on
        // both sides of the switch with no revert needed.
        rpcUrls: [window.location.origin + '/rpc'],
        blockExplorerUrls: [window.location.origin]
      }]
    });
  } catch (e) { console.error('MetaMask error:', e); }
  // FIX (P2, beta-launch audit 2026-07-05): wallet_addEthereumChain only
  // switches the active network when the chain is being added for the
  // FIRST time — a returning user who already added Aequitas Chain in a
  // previous session but is currently active on a different network (the
  // common case: they used MetaMask for something else since) got no
  // prompt at all, then hit confusing failures deep into registration/swap
  // flows instead of a clear, immediate "switch network" prompt. Explicitly
  // request the switch regardless of whether the add above was a no-op.
  try {
    await window.ethereum.request({
      method: 'wallet_switchEthereumChain',
      params: [{ chainId: CID }]
    });
  } catch (e) { console.error('MetaMask network switch error:', e); }
}

// UBI countdown timer — counts down to the next daily distribution.
// secsRemaining comes from the server (uptime modulo 86400 subtracted from 86400).
// Once it reaches zero it resets to 24h and keeps ticking, since the
// distribution just ran and the next one is 24h away again.
let ubiTimerInterval = null;
function startUBITimer(secsRemaining) {
  if (ubiTimerInterval) clearInterval(ubiTimerInterval);
  let secs = secsRemaining;
  const els = [
    document.getElementById('ubi-timer'),
    document.getElementById('validators-timer'),
    document.getElementById('lp-timer'),
  ].filter(Boolean);
  if (!els.length) return;

  const fmt = s => {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    return String(h).padStart(2,'0') + 'h ' + String(m).padStart(2,'0') + 'm ' + String(sec).padStart(2,'0') + 's';
  };

  els.forEach(el => el.textContent = fmt(secs));
  ubiTimerInterval = setInterval(() => {
    secs--;
    if (secs <= 0) {
      secs = 86400;
      els.forEach(el => { el.style.color = 'var(--green)'; });
      setTimeout(() => { els.forEach(el => { el.style.color = ''; }); }, 3000);
      // Refresh only pool balances after distribution — do NOT call loadStatus()
      // here because loadStatus() calls startUBITimer() which would restart the
      // timer from 0 again if next_ubi_at hasn't been written yet → 2s reset loop.
      setTimeout(() => { if (typeof loadPoolStatus === 'function') loadPoolStatus(); }, 3000);
    }
    els.forEach(el => el.textContent = fmt(secs));
  }, 1000);
}

// loadTopology renders the "Active Nodes" grid and the network topology
// SVG from live data (/api/status + /api/peers) instead of the 3 hardcoded
// node boxes/SVG rects this page used to ship with — beta-launch audit
// 2026-07-05. The project's own design explicitly invites any registered
// human to run an additional validator node (see nodes-desc), so hardcoding
// exactly 3 specific node names/URLs meant every new node required another
// manual edit here, and the page would keep claiming "three nodes" forever
// even after a 4th joined. Peer roles beyond "is this node itself the
// primary" aren't derivable from a single fetch (ActivePeers returns bare
// URLs, not each peer's own is_primary flag), so non-self nodes are
// labeled generically as "Node" rather than guessing a role we can't verify.
let loadTopologySeq = 0;
async function loadTopology() {
  const grid = document.getElementById('node-grid');
  const badge = document.getElementById('node-count-badge');
  const svg = document.getElementById('topology-svg');
  if (!grid && !badge && !svg) return;
  const mySeq = ++loadTopologySeq;
  try {
    const [statusRes, peersRes] = await Promise.all([
      fetch('/api/status').then(function(r) { return r.json(); }).catch(function() { return {}; }),
      fetch('/api/peers').then(function(r) { return r.json(); }).catch(function() { return { peers: [] }; }),
    ]);
    if (mySeq !== loadTopologySeq) return;

    const selfUrl = window.location.origin;
    const norm = function(u) { return (u || '').replace(/\/+$/, ''); };
    const peerUrls = Array.isArray(peersRes.peers) ? peersRes.peers : [];
    const dedupedPeers = peerUrls.filter(function(u) { return u && norm(u) !== norm(selfUrl); });
    const nodes = [{ url: selfUrl, isPrimary: !!statusRes.is_primary, self: true, roleKnown: typeof statusRes.is_primary === 'boolean', gitCommit: statusRes.git_commit }]
      .concat(dedupedPeers.map(function(u) { return { url: u, isPrimary: false, self: false, roleKnown: false, gitCommit: null }; }));

    if (badge) badge.textContent = '(' + nodes.length + ')';

    // FIX (2026-07-21): fetch each PEER's own /api/status too, not just this
    // node's — /api/status sets Access-Control-Allow-Origin: * specifically
    // so this cross-origin read works. The one real question this whole
    // session kept running into ("is every node actually on the same
    // commit?") used to require checking each node's CI/deploy history by
    // hand; now it's answerable at a glance right here. Best-effort: an
    // HTTPS page fetching a peer that's only reachable over plain HTTP (a
    // bare-IP node with no TLS cert) gets blocked by the browser as mixed
    // content — gitCommit just stays null for that peer, rendered as "—"
    // rather than breaking the rest of the grid.
    // FIX (2026-08-16, reported live): this used to fetch each peer's
    // /api/status straight from the browser, and every one of those reads was
    // refused by this page's OWN Content-Security-Policy —
    //   "Connecting to 'http://173.249.37.118:8080/api/status' violates the
    //    following Content Security Policy directive: connect-src 'self' …"
    // So the primary showed up on a secondary's Network tab as a node whose
    // role could not be verified, and the commit column was permanently "—".
    // Widening connect-src to arbitrary peer origins would be the wrong trade,
    // and would not help anyway once this page is served over HTTPS on
    // 2026-08-18 while the validators answer on plain HTTP (mixed content).
    // The node has no such restriction and already talks to its peers, so it
    // asks on our behalf: one same-origin call, /api/peers/status.
    const peerInfo = {};
    try {
      const ps = await fetch('/api/peers/status');
      if (ps.ok) {
        const pj = await ps.json();
        (pj.peers || []).forEach(function(p) { peerInfo[norm(p.url)] = p; });
      }
    } catch (e) { /* leave every peer unverified rather than guessing */ }
    if (mySeq !== loadTopologySeq) return;

    nodes.filter(function(n) { return !n.self; }).forEach(function(n) {
      const info = peerInfo[norm(n.url)];
      if (info && info.reachable) {
        n.gitCommit = info.git_commit && info.git_commit !== 'unknown' ? info.git_commit : null;
        n.roleKnown = typeof info.is_primary === 'boolean';
        n.isPrimary = !!info.is_primary;
      } else {
        n.gitCommit = null;
        n.roleKnown = false;
      }
    });
    if (mySeq !== loadTopologySeq) return;

    if (grid) {
      const cols = Math.max(1, Math.min(nodes.length, 4));
      grid.style.gridTemplateColumns = 'repeat(' + cols + ', 1fr)';
      const commits = nodes.map(function(n) { return n.gitCommit; }).filter(Boolean);
      const allSame = commits.length > 1 && commits.every(function(c) { return c === commits[0]; });
      grid.innerHTML = nodes.map(function(n) {
        let host;
        try { host = new URL(n.url).host; } catch (e) { host = n.url; }
        // Unreachable peer: say so, never assert a role that was never read.
        const role = n.roleKnown ? (n.isPrimary ? 'Primary' : 'Secondary') : 'Node (role unverified)';
        const tag = n.self ? ' (this node)' : '';
        // FIX (2026-08-16, reported live): every card used to be tagged
        // "⚠ differs from other nodes" whenever the builds were not all
        // identical — so with two nodes on two commits, BOTH were marked as
        // the odd one out, which cannot be true of both at once. A short
        // SHA carries no ordering either, so the page cannot tell which node
        // is behind; claiming per-node fault was inventing information it
        // does not have. Each card now simply states its own build, and the
        // fleet-level observation is made ONCE, below the grid.
        const commitLine = '<div class="ndesc" style="color:var(--muted)" title="Build commit this node is running">'
          + (n.gitCommit ? ('commit ' + sanitize(n.gitCommit)) : 'commit — (not reported by this node)')
          + '</div>';
        return '<div class="nbox">' +
          '<div class="nstat"><span class="ndot"></span>' + sanitize(role + tag) + '</div>' +
          '<div class="nurl">' + sanitize(host) + '</div>' +
          '<div class="ndesc">API &middot; Block producer &middot; P2P peer &middot; Own PostgreSQL state</div>' +
          commitLine +
          '</div>';
      }).join('');

      // One fleet-level statement, made once. Two nodes on two builds is the
      // normal look of a rollout in progress — each box is deployed in turn —
      // so this reports the fact without calling it a fault. It only becomes
      // one if it stays that way, and a human reading this line can tell the
      // difference; the page cannot, because a short SHA has no ordering.
      const fleetNote = document.getElementById('fleet-build-note');
      if (fleetNote) {
        if (commits.length > 1 && !allSame) {
          const distinct = [];
          commits.forEach(function(c) { if (distinct.indexOf(c) === -1) distinct.push(c); });
          fleetNote.textContent = distinct.length + ' different builds across ' + commits.length +
            ' nodes (' + distinct.join(', ') + ') — expected while a deploy rolls from one box to the next.';
          fleetNote.style.display = '';
        } else {
          fleetNote.style.display = 'none';
        }
      }
    }

    if (svg) renderTopologySVG(svg, nodes);
  } catch (e) {
    if (mySeq !== loadTopologySeq) return;
    if (badge) badge.textContent = '';
  }
}

// renderTopologySVG lays out nodes.length boxes in a single row (the
// same visual style the old hardcoded 3-box version used), each connected
// by a dashed line to the central P2P/libp2p hub, which in turn connects up
// to a static MetaMask/Users box. Pure string-built SVG (no interactivity
// needed here, unlike the DAG view's createElementNS-based approach).
function renderTopologySVG(svg, nodes) {
  const W = 500, H = 230;
  const n = Math.max(nodes.length, 1);
  const gap = 8;
  const boxW = Math.max(70, (W - gap * (n + 1)) / n);
  const boxH = 70;
  const boxY = 150;
  const hubX = W / 2, hubY = 97;
  const colors = ['#047857', '#0891B2', '#6B46C1', '#B45309', '#BE185D', '#4338CA'];

  let svgHtml = '<rect width="' + W + '" height="' + H + '" fill="rgba(255,255,255,0.03)" rx="10"/>' +
    '<rect x="' + (W / 2 - 75) + '" y="10" width="150" height="38" rx="8" fill="rgba(146,64,14,0.06)" stroke="#92400E" stroke-width="1"/>' +
    '<text x="' + hubX + '" y="27" text-anchor="middle" font-size="9" font-weight="700" fill="rgba(240,180,41,0.9)">MetaMask / Users</text>' +
    '<text x="' + hubX + '" y="41" text-anchor="middle" font-size="7.5" fill="rgba(136,146,164,0.9)">JSON-RPC &#183; Chain ID 1926</text>' +
    '<ellipse cx="' + hubX + '" cy="' + hubY + '" rx="70" ry="28" fill="rgba(107,70,193,0.08)" stroke="rgba(107,70,193,0.3)" stroke-width="1" stroke-dasharray="4,3"/>' +
    '<text x="' + hubX + '" y="' + (hubY - 4) + '" text-anchor="middle" font-size="9" fill="rgba(155,114,246,0.9)">P2P libp2p</text>' +
    '<text x="' + hubX + '" y="' + (hubY + 10) + '" text-anchor="middle" font-size="8" fill="rgba(136,146,164,0.9)">BlockDAG sync &#183; GHOSTDAG merge</text>' +
    '<line x1="' + hubX + '" y1="48" x2="' + hubX + '" y2="69" stroke="#6B46C1" stroke-width="1.5" stroke-dasharray="4,3"/>';

  nodes.forEach(function(node, i) {
    const x = gap + i * (boxW + gap);
    const cx = x + boxW / 2;
    const color = colors[i % colors.length];
    let host;
    try { host = new URL(node.url).host; } catch (e) { host = node.url; }
    const roleLine = (node.roleKnown ? (node.isPrimary ? 'Primary' : 'Secondary') : 'role unverified') + (node.self ? ' (this node)' : '');
    svgHtml += '<rect x="' + x + '" y="' + boxY + '" width="' + boxW + '" height="' + boxH + '" rx="8" fill="' + color + '22" stroke="' + color + '" stroke-width="1.5"/>' +
      '<text x="' + cx + '" y="' + (boxY + 22) + '" text-anchor="middle" font-size="9" font-weight="700" fill="' + color + '">' + sanitize(host.length > 22 ? host.slice(0, 20) + '…' : host) + '</text>' +
      '<text x="' + cx + '" y="' + (boxY + 36) + '" text-anchor="middle" font-size="7.5" fill="rgba(136,146,164,0.9)">' + sanitize(roleLine) + '</text>' +
      '<text x="' + cx + '" y="' + (boxY + 51) + '" text-anchor="middle" font-size="7" fill="' + color + '">&#9679; API &#183; own PostgreSQL</text>' +
      '<text x="' + cx + '" y="' + (boxY + 61) + '" text-anchor="middle" font-size="7" fill="' + color + '">&#9679; Block producer</text>' +
      '<line x1="' + cx + '" y1="' + boxY + '" x2="' + hubX + '" y2="' + (hubY + 28) + '" stroke="#6B46C1" stroke-width="1.5" stroke-dasharray="4,3"/>';
  });

  svgHtml += '<text x="' + hubX + '" y="228" text-anchor="middle" font-size="7" fill="rgba(136,146,164,0.7)">any additional registered human can run a validator node — no application, no shared secret</text>';

  svg.innerHTML = svgHtml;
}

// loadHealth polls /api/health/combined and reflects the node's real
// sync/consensus health on the header badge — surfacing the same signals
// (degraded state, synthetic-checkpoint trust mode, StateRoot mismatches)
// an operator would otherwise only see in server logs.
// FIX (P2, beta-launch audit 2026-07-05): every setInterval-driven poller in
// this file (loadStatus/loadBlocks/loadHumans/loadHealth/loadValidatorLabels/
// loadPoolStatus) had no request-sequencing guard — if a slow response from
// tick N resolved after tick N+1's already landed, tick N's .then() would
// unconditionally overwrite the DOM with older data, visibly reverting
// stats/blocks/health to a stale snapshot for one interval. Each poller now
// grabs a sequence number before its fetch and bails before touching the DOM
// if a newer call for the same poller has already started.
let loadHealthSeq = 0;
async function loadHealth() {
  const badge = document.getElementById('health-badge');
  if (!badge) return;
  const mySeq = ++loadHealthSeq;
  try {
    const r = await fetch('/api/health/combined');
    const d = await r.json();
    if (mySeq !== loadHealthSeq) return;
    const c = d.chain || {};
    const status = c.status || 'healthy';
    badge.classList.remove('badge-health-healthy', 'badge-health-warn', 'badge-health-unhealthy');
    badge.classList.add('badge-health-' + status);
    const label = status === 'healthy' ? '● GHOSTDAG' : (status === 'warn' ? '● GHOSTDAG ⚠' : '● GHOSTDAG ✕');
    badge.textContent = label;
    const notes = Array.isArray(c.notes) && c.notes.length ? c.notes.join(' · ') : 'All systems nominal — height ' + fmt(c.height) + ', ' + fmt(c.dag_tips_count) + ' active tip(s)';
    badge.title = notes;
  } catch (e) {
    if (mySeq !== loadHealthSeq) return;
    badge.classList.remove('badge-health-healthy', 'badge-health-warn');
    badge.classList.add('badge-health-unhealthy');
    badge.textContent = '● GHOSTDAG ✕';
    badge.title = 'Could not reach /api/health/combined';
  }
}

let loadStatusSeq = 0;
async function loadStatus() {
  const mySeq = ++loadStatusSeq;
  try {
    const d = await (await fetch('/api/status')).json();
    if (mySeq !== loadStatusSeq) return;
    // Cache the true chain height so the block list's "N blocks" stat can
    // show it directly instead of a deduped count of whatever page of
    // blocks happens to be locally fetched (see loadBlocks).
    latestChainHeight = d.height;
    if (typeof d.knightdag_activation_height === 'number') {
      knightdagActivationHeight = d.knightdag_activation_height;
    }
    document.getElementById('s-height').textContent = fmt(d.height);
    // FIX (2026-07-05): this used to be a hardcoded "~6s" in the HTML —
    // BLOCK_TIME changed several times the same night without this text
    // ever being updated to match, silently going stale on every future
    // change too. Always reflect the server's own reported cadence
    // (block_time, kept in sync with cmd/aequitasd's BLOCK_TIME constant —
    // see api.go) instead of a number baked in at whatever moment this
    // page happened to be written.
    const heightSubEl = document.getElementById('s-height-sub');
    if (heightSubEl && d.block_time !== undefined) {
      heightSubEl.textContent = '~' + d.block_time + 's · BlockDAG · Parallel production';
    }
    // Also resolves every __BT__ placeholder (blocks-desc across all
    // locales, k-btime-val in the Technical Specifications table) — see
    // applyBlockTime's own comment.
    applyBlockTime(d.block_time);
    document.getElementById('s-humans').textContent = fmt(d.total_humans);
    document.getElementById('s-supply').textContent = d.total_supply || '—';
    document.getElementById('s-index').textContent = fmt(d.index);
    const up = d.uptime || 0;
    document.getElementById('s-uptime').textContent = Math.floor(up/3600) + 'h ' + Math.floor((up%3600)/60) + 'm';
    document.getElementById('idx-score').textContent = fmt(d.index);
    document.getElementById('idx-gini').textContent = typeof d.gini === 'number' ? d.gini.toFixed(4) : '—';
    const gniWarn = document.getElementById('gini-n-warn');
    if (gniWarn) gniWarn.style.display = (d.total_humans < 10) ? 'block' : 'none';
    document.getElementById('idx-supply2').textContent = d.total_supply || '—';
    document.getElementById('idx-phase').textContent = fmt(d.phase);
    document.getElementById('idx-humans2').textContent = fmt(d.total_humans);
    document.getElementById('stat-humans').textContent = fmt(d.total_humans);
    document.getElementById('stat-supply').textContent = d.total_supply || '—';

    // Pool balances — show 0.0000 instead of — when pool is empty
    const fmtPool = v => (v || '0.0000') + ' AEQ';
    document.getElementById('pool-v').textContent = fmtPool(d.pool_validators);
    document.getElementById('pool-l').textContent = fmtPool(d.pool_lp);
    document.getElementById('pool-u').textContent = fmtPool(d.pool_ubi);
    document.getElementById('pool-t').textContent = fmtPool(d.pool_treasury);

    // UBI countdown timer + fill bar
    // Only (re)start the timer when the server returns a positive value.
    // When ubi_next_payout_secs === 0 (IS_PRIMARY_NODE not set, or next_ubi_at
    // not yet written to DB), leave the running timer alone — restarting from 0
    // causes a reset loop because loadStatus fires every 6s.
    if (d.ubi_next_payout_secs > 0) {
      startUBITimer(d.ubi_next_payout_secs);
    }
    const fillSecs = d.ubi_next_payout_secs || 0;
    const fillPct = Math.min(100, Math.max(0, (86400 - fillSecs) / 86400 * 100));
    const fillBar = document.getElementById('ubi-fill-bar');
    if (fillBar) fillBar.style.width = fillPct.toFixed(1) + '%';

    if (d.index !== undefined) {
      document.getElementById('idx-bar').style.width = Math.min(d.index, 100) + '%';
      const phases = ['Phase 0: Bootstrap — sliding wealth cap 5×→25× (active)', 'Phase 1: Growth — expanding human registry (cap: 25×)', 'Phase 2: Stability — redistribution active (cap: 25×)', 'Phase 3: Maturity — full decentralization (cap: 25×)'];
      document.getElementById('idx-phase-desc').textContent = phases[d.phase || 0] || 'Phase ' + (d.phase || 0);
    }
  } catch (e) {}
  // Populate live wealth-cap widget (non-blocking)
  try {
    const wc = await (await fetch('/api/wealth-cap')).json();
    if (mySeq !== loadStatusSeq) return;
    const capEl = document.getElementById('live-cap-aeq');
    const multEl = document.getElementById('live-cap-mult');
    const avgEl = document.getElementById('live-cap-avg');
    // Cap display at total supply — when only 1000 AEQ exist the theoretical
    // cap (5 × 1000 = 5000) is unreachable and confusing to show.
    // FIX 1: strip comma separators before parseFloat (e.g. "1,234.56" → 1234.56)
    const supplyText = (document.getElementById('s-supply') || {}).textContent || '0';
    const totalSupplyNum = parseFloat(supplyText.replace(/,/g, '')) || 0;
    // FIX 2: guard against NaN (e.g. when s-supply still shows "—")
    const displayCap = (wc.cap_aeq !== undefined && totalSupplyNum > 0 && !isNaN(totalSupplyNum))
      ? Math.min(wc.cap_aeq, totalSupplyNum)
      : wc.cap_aeq;
    if (capEl && displayCap !== undefined && !isNaN(displayCap)) capEl.textContent = displayCap.toFixed(2);
    else if (capEl && wc.cap_aeq !== undefined) capEl.textContent = wc.cap_aeq.toFixed(2);
    if (multEl && wc.multiplier !== undefined) multEl.textContent = wc.multiplier.toFixed(0) + '×';
    if (avgEl && wc.average_aeq !== undefined) avgEl.textContent = wc.average_aeq.toFixed(2);
  } catch(_) {}
}

async function drawGiniHistoryChart() {
  const canvas = document.getElementById('gini-history-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0, 0, W, H);
  try {
    const d = await (await fetch('/api/gini/history')).json();
    // NOT reversed. The server already hands these over oldest-first:
    // GetGiniHistory queries `ORDER BY captured_at DESC` to take the newest N,
    // then reverses the rows itself ("Reverse to get chronological order").
    // Reversing again here put the NEWEST snapshot at x=0 and the OLDEST at the
    // right-hand edge, so the curve ran backwards in time — a chain growing
    // steadily less equal was drawn as one steadily improving — and
    // history[length-1], the point the latest-value label names, was the very
    // first snapshot ever taken. Nothing on the canvas dates the x axis, so the
    // only visible symptom was the trend pointing the wrong way.
    const history = (d.history || []).slice();
    const emptyEl = document.getElementById('gini-history-empty');
    if (!history.length) {
      if (emptyEl) { emptyEl.style.display = 'block'; canvas.style.display = 'none'; } return;
    }
    if (emptyEl) { emptyEl.style.display = 'none'; canvas.style.display = 'block'; }
    // Single data point — draw a gauge/meter visualization
    if (history.length === 1) {
      var g0 = history[0].gini || (history[0].idx/100); // 0-1 scale
      // Background
      ctx.fillStyle='rgba(8,10,22,0.7)'; ctx.fillRect(0,0,W,H);
      // Horizontal bar gauge
      var bx=40, by=H/2-18, bw=W-80, bh=28, r=6;
      // Track
      ctx.fillStyle='rgba(255,255,255,0.06)';
      ctx.beginPath(); ctx.roundRect(bx,by,bw,bh,r); ctx.fill();
      // Zone colors: green 0-0.30, amber 0.30-0.70, red 0.70-1.0 (Gini 0–1 scale)
      var zones=[[0,0.30,'rgba(0,255,100,0.5)'],[0.30,0.70,'rgba(245,158,11,0.5)'],[0.70,1.0,'rgba(239,68,68,0.5)']];
      zones.forEach(function(z){
        var x1=bx+bw*z[0], x2=bx+bw*z[1];
        ctx.fillStyle=z[2]; ctx.fillRect(x1,by,x2-x1,bh);
      });
      // Fill up to current value
      var fill=bw*g0/1.0;
      var grd=ctx.createLinearGradient(bx,0,bx+fill,0);
      grd.addColorStop(0,'rgba(0,255,200,0.9)'); grd.addColorStop(0.5,'rgba(245,158,11,0.9)'); grd.addColorStop(1,'rgba(239,68,68,0.9)');
      ctx.fillStyle=grd; ctx.beginPath(); ctx.roundRect(bx,by,fill,bh,r); ctx.fill();
      // Target marker at 0.30 (Gini target)
      var tx=bx+bw*0.30;
      ctx.strokeStyle='rgba(0,255,209,0.9)'; ctx.lineWidth=2;
      ctx.beginPath(); ctx.moveTo(tx,by-6); ctx.lineTo(tx,by+bh+6); ctx.stroke();
      ctx.fillStyle='rgba(0,255,209,0.9)'; ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='center';
      ctx.fillText('0.30', tx, by-10);
      // Pointer
      var px=bx+bw*g0/1.0;
      ctx.fillStyle='#fff'; ctx.beginPath(); ctx.moveTo(px,by-2); ctx.lineTo(px-5,by-10); ctx.lineTo(px+5,by-10); ctx.fill();
      // Labels: 0, 0.30, 0.70, 1.0 (Gini 0–1 scale)
      [[0,'0'],[0.30,'0.30'],[0.70,'0.70'],[1,'1.0']].forEach(function(l){
        ctx.fillStyle='rgba(200,168,76,0.5)'; ctx.font='9px JetBrains Mono,monospace'; ctx.textAlign='center';
        ctx.fillText(l[1], bx+bw*l[0], by+bh+14);
      });
      // Big value
      ctx.fillStyle='rgba(200,168,76,0.95)'; ctx.font='bold 28px JetBrains Mono,monospace'; ctx.textAlign='center';
      ctx.fillText('Gini: ' + g0.toFixed(4), W/2, by-26);
      // Description (g0 is 0–1 Gini scale, target is < 0.30)
      var label;
      if(g0<0.30) label='Below target — excellent equality';
      else if(g0<0.70) label='Above target — redistribution active';
      else label='Critical — protocol at maximum intervention';
      ctx.font='11px Inter,sans-serif'; ctx.fillStyle='rgba(200,200,200,0.6)';
      ctx.fillText(label, W/2, by+bh+28);
      ctx.font='10px Inter,sans-serif'; ctx.fillStyle='rgba(0,255,209,0.5)';
      ctx.fillText('History chart grows after each daily UBI distribution', W/2, H-10);
      return;
    }
    const pad = {l:48,r:24,t:36,b:32};
    const cW = W-pad.l-pad.r, cH = H-pad.t-pad.b;
    const toX = (i) => pad.l + cW*i/Math.max(history.length-1,1);
    const toY = (v) => pad.t + cH*(1-v/100);
    // danger zone (>70) subtle red tint
    const dg = ctx.createLinearGradient(0,toY(100),0,toY(70));
    dg.addColorStop(0,'rgba(248,113,113,0.06)'); dg.addColorStop(1,'rgba(248,113,113,0)');
    ctx.fillStyle=dg; ctx.fillRect(pad.l,toY(100),cW,toY(70)-toY(100));
    // grid lines
    for (let i=0;i<=4;i++) {
      const v=i*25, y=toY(v);
      ctx.strokeStyle = v===0?'rgba(139,92,246,0.2)':'rgba(139,92,246,0.08)';
      ctx.lineWidth = v===0?1.5:1;
      ctx.beginPath(); ctx.moveTo(pad.l,y); ctx.lineTo(W-pad.r,y); ctx.stroke();
      ctx.fillStyle='rgba(200,168,76,0.75)'; ctx.font='10px JetBrains Mono,monospace'; ctx.textAlign='right';
      ctx.fillText(v+'', pad.l-6, y+4);
    }
    // target 0.30 line (idx = gini*100, so toY(30) = Gini 0.30)
    const targetY = toY(30);
    ctx.save(); ctx.shadowColor='rgba(0,255,209,0.7)'; ctx.shadowBlur=5;
    ctx.strokeStyle='rgba(0,255,209,0.55)'; ctx.lineWidth=1.5; ctx.setLineDash([6,5]);
    ctx.beginPath(); ctx.moveTo(pad.l,targetY); ctx.lineTo(W-pad.r,targetY); ctx.stroke();
    ctx.setLineDash([]); ctx.restore();
    // Names both scales. This axis is the Index (0-100); the surrounding copy
    // and the whole project talk in Gini (0-1). Printing only "0.30" beside an
    // Index axis is what let the point label below be read on the wrong scale.
    //
    // Anchored LEFT: the latest-value label sits at the right, beside the final
    // data point, so putting both on the same side made them overlap each other
    // and the curve on a phone-width canvas.
    ctx.fillStyle='rgba(4,120,87,0.85)'; ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='left';
    var tgtLong='TARGET  INDEX 30 = GINI 0.30', tgtShort='TARGET 30';
    var tgtTxt = ctx.measureText(tgtLong).width <= (W-pad.l-pad.r)*0.62 ? tgtLong : tgtShort;
    ctx.fillText(tgtTxt, pad.l+2, targetY-5);
    // bezier path helper
    var pathBez = function(pts) {
      ctx.moveTo(toX(0), toY(pts[0].idx));
      if (pts.length<3) { for(var k=1;k<pts.length;k++) ctx.lineTo(toX(k),toY(pts[k].idx)); return; }
      for (var k=1;k<pts.length-1;k++) {
        var mx=(toX(k)+toX(k+1))/2, my=(toY(pts[k].idx)+toY(pts[k+1].idx))/2;
        ctx.quadraticCurveTo(toX(k),toY(pts[k].idx),mx,my);
      }
      ctx.lineTo(toX(pts.length-1), toY(pts[pts.length-1].idx));
    };
    // gradient fill
    var fg=ctx.createLinearGradient(0,pad.t,0,H-pad.b);
    fg.addColorStop(0,'rgba(200,168,76,0.28)'); fg.addColorStop(0.7,'rgba(200,168,76,0.07)'); fg.addColorStop(1,'rgba(200,168,76,0.01)');
    ctx.beginPath(); pathBez(history);
    ctx.lineTo(toX(history.length-1),H-pad.b); ctx.lineTo(toX(0),H-pad.b); ctx.closePath();
    ctx.fillStyle=fg; ctx.fill();
    // glowing line
    ctx.save(); ctx.shadowColor='rgba(200,168,76,0.6)'; ctx.shadowBlur=10;
    ctx.strokeStyle='#C9A84C'; ctx.lineWidth=2.5;
    ctx.beginPath(); pathBez(history); ctx.stroke(); ctx.restore();
    // dots
    history.forEach(function(pt,i){
      var x=toX(i), y=toY(pt.idx);
      ctx.save(); ctx.shadowColor='rgba(200,168,76,0.9)'; ctx.shadowBlur=12;
      ctx.beginPath(); ctx.arc(x,y,4.5,0,2*Math.PI); ctx.fillStyle='#C9A84C'; ctx.fill(); ctx.restore();
      ctx.beginPath(); ctx.arc(x,y,2,0,2*Math.PI); ctx.fillStyle='#fff'; ctx.fill();
    });
    // latest value label
    // FIX: this printed lpt.idx — the Aequitas Index, the 0-100 value this
    // whole chart is plotted on — under the label "Gini", the 0-1 value.
    // On screen that read "Gini: 9.581" directly beside "TARGET 0.30",
    // i.e. thirty-two times over target, when the chain was actually at
    // Gini 0.096, comfortably under it. The chart was stating the exact
    // opposite of the one number this project is built on. Both scales are
    // now shown, so neither can be read as the other.
    var lpt=history[history.length-1], lx=toX(history.length-1), ly=toY(lpt.idx);
    var lgini = (typeof lpt.gini === 'number') ? lpt.gini : lpt.idx/100;
    ctx.fillStyle='rgba(200,168,76,0.95)'; ctx.font='bold 11px JetBrains Mono,monospace';
    // Drop the parenthetical Gini on a canvas too narrow to hold it rather
    // than letting it run over the curve.
    var lLong='Index '+lpt.idx.toFixed(1)+'  (Gini '+lgini.toFixed(3)+')';
    var lShort='Index '+lpt.idx.toFixed(1);
    var llabel = ctx.measureText(lLong).width <= (W-pad.l-pad.r)*0.55 ? lLong : lShort;
    var lw = ctx.measureText(llabel).width;
    // Right-align once the label would otherwise run off the canvas, and clamp
    // so it can never start left of the plot area.
    var lright = lx + 8 + lw > W - pad.r;
    ctx.textAlign = lright ? 'right' : 'left';
    var lxPos = lright ? Math.max(lx-8, pad.l+lw) : lx+8;
    // The target line is a fixed horizontal at Index 30. When the latest point
    // sits close to it the label collides with the dashes, so flip below.
    var lyPos = (Math.abs(ly-targetY) < 18) ? ly+18 : ly-9;
    ctx.fillText(llabel, lxPos, lyPos);
    // title
    // The long form does not fit a phone-width canvas — it was being cut
    // mid-sentence at the right edge ("... 100 ="). Canvas does not wrap or
    // ellipsize, so pick the form that fits.
    ctx.fillStyle='rgba(107,70,193,0.55)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='left';
    var tLong = 'GINI INDEX HISTORY  —  0 = perfect equality  ·  100 = max inequality';
    var tShort = 'GINI INDEX HISTORY  —  0 = equal  ·  100 = unequal';
    var avail = W - pad.l - pad.r;
    var title = ctx.measureText(tLong).width <= avail ? tLong
              : (ctx.measureText(tShort).width <= avail ? tShort : 'GINI INDEX HISTORY');
    ctx.fillText(title, pad.l, 20);
  } catch(e) {}
}

// Guards the one self-scheduled redraw after a 429 (see the fetch below), so a
// visitor flipping between sub-tabs cannot stack timers.
var drawLorenzRetryPending = false;
// FIX (2026-08-15): drawLorenzCurve is reached from five places — sub-tab
// clicks, direct-URL activation, the IntersectionObserver, the retry above and
// resize — and it awaits a network call in the middle while drawing to a single
// shared canvas. Two overlapping runs therefore interleaved: one cleared the
// canvas and went to the network, the other finished drawing the curve, and the
// first then printed its placeholder ON TOP of that finished chart. That is
// visible in the report this fix came from: a fully drawn Lorenz curve with
// 'Need 2+ registered humans' floating in the middle of it. Same sequence guard
// loadHumans and loadTopology already use — the newest run wins, older ones
// return without touching the canvas.
var drawLorenzSeq = 0;
async function drawLorenzCurve() {
  var mySeq = ++drawLorenzSeq;
  var canvas = document.getElementById('lorenz-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  var W = canvas.width;
  // Mobile: legend goes below chart → taller canvas; desktop: legend right
  var isMobile = W < 480;
  // mLegH = space below the chart area reserved for the 2-column legend (8 items = 4 rows × 26 + 20 padding)
  var mLegH = isMobile ? 130 : 0;
  // H = chart drawing height; canvas.height = H + legend space (mobile) or just H (desktop)
  var H = isMobile ? 420 : 460;
  canvas.height = H + mLegH;
  var ctx = canvas.getContext('2d');
  ctx.clearRect(0, 0, W, canvas.height);
  ctx.fillStyle = '#070B16'; ctx.fillRect(0, 0, W, canvas.height);

  // Mobile layout: no right panel, legend drawn below chart
  // Desktop layout: 252px right legend panel, 82px top header
  var legendW = isMobile ? 0 : 252;
  var pad = isMobile
    ? {l:36, r:8,  t:54, b:44}   // mobile: full-width chart
    : {l:62, r:legendW, t:82, b:62}; // desktop
  var cW = W - pad.l - pad.r;
  var cH = H - pad.t - pad.b;
  function px(f) { return pad.l + cW * f; }
  function py(f) { return pad.t + cH * (1 - f); }
  function rr(x,y,w,h,r) { if(ctx.roundRect)ctx.roundRect(x,y,w,h,r); else ctx.rect(x,y,w,h); }

  try {
    // NOTE (fresh Monster Audit 2026-07-12, P2): no limit param here means
    // the server's default cap applies (500, address-sorted — see
    // handleHumans in api.go). Below that population this curve is exact;
    // beyond it, it's drawn from a fixed subset (the first 500 addresses
    // alphabetically) rather than the true population, which is not a
    // representative sample. Not worth a client-side pagination loop for a
    // decorative curve — the authoritative Score-tab Gini is computed
    // server-side over every account, not from this fetch, so the number
    // that actually matters stays correct regardless; only this
    // supplementary visual would start drawing an approximation once
    // registrations pass 500.
    // FIX (2026-08-15, reported live with a screenshot): this read the JSON
    // without looking at the HTTP status — the identical defect loadHumans had
    // fixed on 2026-07-27, left standing at this second call site. /api/humans
    // allows one request per 3s per IP, and THIS PAGE CALLS IT TWICE: the
    // 10s-poll loadHumans and this curve. Opening the Distribution tab lands
    // both inside one window, so the loser gets a 429 whose body has no
    // "humans" key, `humans` becomes [], and the canvas states "Need 2+
    // registered humans" — a claim that the chain has almost no humans, on a
    // node whose /api/status right above it reads 15. Reproduced against
    // production: two calls 0.3s apart return 200 then 429.
    var resp = await fetch('/api/humans');
    if (mySeq !== drawLorenzSeq) return;
    if (!resp.ok) {
      ctx.fillStyle = 'rgba(155,114,246,0.6)'; ctx.font = '13px Inter'; ctx.textAlign = 'center';
      ctx.fillText(resp.status === 429
        ? 'Too many requests just now — redrawing in a moment…'
        : 'Could not load the distribution (HTTP ' + resp.status + ').', W / 2, H / 2);
      // A 429 clears in three seconds. Come back once on our own rather than
      // leaving a wrong-looking chart until the visitor happens to switch tabs.
      if (resp.status === 429 && !drawLorenzRetryPending) {
        drawLorenzRetryPending = true;
        setTimeout(function () { drawLorenzRetryPending = false; drawLorenzCurve(); }, 3500);
      }
      return;
    }
    var d = await resp.json();
    if (mySeq !== drawLorenzSeq) return;
    var humans = d.humans || [];
    if (humans.length < 2) {
      ctx.fillStyle='rgba(155,114,246,0.6)'; ctx.font='13px Inter'; ctx.textAlign='center';
      ctx.fillText('Need 2+ registered humans', W/2, H/2); return;
    }

    // Use total AEQ wealth (liquid + LP value), the SAME number the server's
    // Gini uses (humanAEQWealthLocked / total_value_aeq). Reading h.balance here
    // counted LP providers as holding 0, making this curve disagree with the
    // Score Gini (0.72 vs 0.15). Fall back to balance for older API responses.
    var bals = humans.map(function(h){ return parseFloat(h.total_value_aeq != null ? h.total_value_aeq : h.balance)||0; }).sort(function(a,b){return a-b;});
    var n = bals.length, total = bals.reduce(function(s,b){return s+b;},0);

    var lorenz = [{x:0,y:0}]; var cum=0;
    for(var i=0;i<n;i++){cum+=bals[i];lorenz.push({x:(i+1)/n,y:total>0?cum/total:(i+1)/n});}

    var area=0;
    for(var i=1;i<lorenz.length;i++){area+=(lorenz[i].x-lorenz[i-1].x)*(lorenz[i].y+lorenz[i-1].y)/2;}
    var gini=Math.max(0,1-2*area);
    // FIX (2026-08-15, reported live): this used to multiply by n/(n-1), added
    // deliberately to match what the server published. The server's own copy of
    // that factor was removed the same day: it is the correction for estimating
    // a large population's Gini from a SAMPLE, and this chain samples nothing —
    // it knows every registered human, so these balances are the whole
    // population. Keeping it here after the server dropped it recreated exactly
    // the discrepancy it was written to remove, in the opposite direction: the
    // curve printed 0.1324 while /api/status beside it reported 0.1236 — the
    // same 15/14 at the 15 humans then live, and a full 2x at n=2. The Lorenz
    // area below IS the population Gini; nothing further is applied to it.

    var gEl=document.getElementById('lorenz-gini-val');
    if(gEl){gEl.textContent=gini.toFixed(4);gEl.style.color=gini<0.30?'#34D399':'#F0B429';}

    // Interpolate at exactly x=0.5 between the two bracketing Lorenz points
    // (nearest-point snap was biased by data density near 50%).
    var aqY50 = (function(){
      for(var i=1;i<lorenz.length;i++){
        if(lorenz[i].x>=0.5){
          var t=(0.5-lorenz[i-1].x)/(lorenz[i].x-lorenz[i-1].x);
          return lorenz[i-1].y+t*(lorenz[i].y-lorenz[i-1].y);
        }
      }
      return lorenz[lorenz.length-1].y;
    })();
    var gC = gini<0.30?'#34D399':'#F0B429';

    // ── HEADER ─────────────────────────────────────────────────────────────
    if(isMobile) {
      // Mobile: compact single-line header + one info bar
      ctx.fillStyle='rgba(232,237,245,0.85)'; ctx.font='bold 10px Inter'; ctx.textAlign='left';
      ctx.fillText('LORENZ CURVE', pad.l, 13);
      ctx.fillStyle='rgba(136,146,164,0.55)'; ctx.font='8px Inter';
      ctx.fillText('Diagonal = perfect equality. Below = inequality.', pad.l, 25);
      // Single compact bar: Aequitas vs World
      var barW = W - pad.l - pad.r - 2;
      ctx.fillStyle='rgba(7,11,22,0.97)'; ctx.strokeStyle=gC; ctx.lineWidth=1;
      ctx.beginPath(); rr(pad.l, 30, barW, 20, 4); ctx.fill(); ctx.stroke();
      ctx.font='bold 9px JetBrains Mono'; ctx.textAlign='left';
      ctx.fillStyle=gC; ctx.fillText('Aequitas: '+gini.toFixed(4), pad.l+8, 43);
      ctx.fillStyle='rgba(167,139,250,0.85)'; ctx.fillText('| World avg: 0.38', pad.l+100, 43);
    } else {
      // Desktop: full title + two info boxes
      ctx.fillStyle='rgba(232,237,245,0.88)'; ctx.font='bold 11px Inter'; ctx.textAlign='left';
      ctx.fillText('LORENZ CURVE — WEALTH DISTRIBUTION', pad.l, 14);
      ctx.fillStyle='rgba(136,146,164,0.6)'; ctx.font='8.5px Inter';
      ctx.fillText('Diagonal = perfect equality.  Curves bowing down = more inequality.  Shaded area = size of inequality gap.', pad.l, 27);
      var bw=Math.min(180, Math.floor((cW - 12) / 2)), bh=40;
      ctx.fillStyle='rgba(7,11,22,0.97)'; ctx.strokeStyle=gC; ctx.lineWidth=1.5;
      ctx.beginPath(); rr(pad.l, 34, bw, bh, 5); ctx.fill(); ctx.stroke();
      ctx.fillStyle='rgba(136,146,164,0.6)'; ctx.font='7px JetBrains Mono'; ctx.textAlign='center';
      ctx.fillText('AEQUITAS GINI COEFFICIENT', pad.l+bw/2, 46);
      ctx.fillStyle=gC; ctx.font='bold 17px JetBrains Mono';
      ctx.fillText(gini.toFixed(4), pad.l+58, 65);
      ctx.fillStyle='rgba(200,200,200,0.65)'; ctx.font='9px JetBrains Mono'; ctx.textAlign='left';
      ctx.fillText('= '+gini.toFixed(4), pad.l+105, 65);
      var b2x=pad.l+bw+12;
      ctx.fillStyle='rgba(7,11,22,0.97)'; ctx.strokeStyle='rgba(167,139,250,0.7)'; ctx.lineWidth=1.5;
      ctx.beginPath(); rr(b2x, 34, bw, bh, 5); ctx.fill(); ctx.stroke();
      ctx.fillStyle='rgba(136,146,164,0.6)'; ctx.font='7px JetBrains Mono'; ctx.textAlign='center';
      ctx.fillText('WORLD AVERAGE GINI 2024', b2x+bw/2, 46);
      ctx.fillStyle='rgba(167,139,250,0.9)'; ctx.font='bold 17px JetBrains Mono';
      ctx.fillText('38.0%', b2x+58, 65);
      ctx.fillStyle='rgba(200,200,200,0.65)'; ctx.font='9px JetBrains Mono'; ctx.textAlign='left';
      ctx.fillText('= 0.380', b2x+108, 65);
    }

    // ── GRID ──────────────────────────────────────────────────────────────
    ctx.strokeStyle='rgba(255,255,255,0.04)'; ctx.lineWidth=1;
    for(var i=1;i<4;i++){
      ctx.beginPath();ctx.moveTo(pad.l,py(i/4));ctx.lineTo(pad.l+cW,py(i/4));ctx.stroke();
      ctx.beginPath();ctx.moveTo(px(i/4),pad.t);ctx.lineTo(px(i/4),pad.t+cH);ctx.stroke();
    }

    // ── AXIS ──────────────────────────────────────────────────────────────
    var axFontSz = isMobile ? 8 : 10;
    ctx.fillStyle='rgba(136,146,164,0.7)'; ctx.font=axFontSz+'px JetBrains Mono';
    // On mobile only show 0%, 50%, 100% to save space
    var tl = isMobile ? ['0%','50%','100%'] : ['0%','25%','50%','75%','100%'];
    var tlIdx = isMobile ? [0,2,4] : [0,1,2,3,4];
    for(var i=0;i<tl.length;i++){
      ctx.textAlign='center'; ctx.fillText(tl[i],px(tlIdx[i]/4),pad.t+cH+16);
      ctx.textAlign='right';  ctx.fillText(tl[i],pad.l-(isMobile?4:6),py(tlIdx[i]/4)+4);
    }
    if(!isMobile) {
      ctx.save();ctx.translate(12,pad.t+cH/2);ctx.rotate(-Math.PI/2);
      ctx.fillStyle='rgba(155,114,246,0.7)';ctx.font='10px Inter';ctx.textAlign='center';
      ctx.fillText('Cumulative % of AEQ wealth',0,0);ctx.restore();
      ctx.fillStyle='rgba(155,114,246,0.6)';ctx.font='10px Inter';ctx.textAlign='center';
      ctx.fillText('% of Population (poorest left → richest right)',px(0.5),pad.t+cH+36);
    } else {
      ctx.fillStyle='rgba(155,114,246,0.5)';ctx.font='8px Inter';ctx.textAlign='center';
      ctx.fillText('Population % →',px(0.5),pad.t+cH+30);
    }

    // ── 50% GUIDE LINE ─────────────────────────────────────────────────────
    ctx.beginPath();ctx.moveTo(px(0.5),pad.t);ctx.lineTo(px(0.5),pad.t+cH);
    ctx.strokeStyle='rgba(255,255,255,0.09)';ctx.lineWidth=1;ctx.setLineDash([4,3]);ctx.stroke();ctx.setLineDash([]);
    ctx.fillStyle='rgba(136,146,164,0.45)';ctx.font='8px JetBrains Mono';ctx.textAlign='center';
    ctx.fillText('50% mark',px(0.5),pad.t-4);

    // ── REFERENCE COUNTRIES (most unequal first → fills stack correctly) ───
    var refs = [
      {label:'South Africa', g:0.63, lc:'#F87171', fc:'rgba(239,68,68,0.18)', tag:'Extreme inequality'},
      {label:'Brazil',       g:0.53, lc:'#FB923C', fc:'rgba(251,146,60,0.14)', tag:'High inequality'},
      {label:'USA',          g:0.41, lc:'#FCD34D', fc:'rgba(252,211,77,0.11)', tag:'Moderate'},
      {label:'World Avg',    g:0.38, lc:'#A78BFA', fc:'rgba(167,139,250,0.09)', tag:'Global average'},
      {label:'Germany',      g:0.31, lc:'#34D399', fc:'rgba(52,211,153,0.08)', tag:'Low inequality'},
      {label:'Scandinavia',  g:0.27, lc:'#60A5FA', fc:'rgba(96,165,250,0.07)', tag:'Very low — target'}
    ];

    refs.forEach(function(ref){
      var rpts=[];
      for(var j=0;j<=120;j++){var xf=j/120;rpts.push({x:xf,y:Math.pow(xf,1+2*ref.g)});}
      ctx.beginPath();ctx.moveTo(px(0),py(0));
      rpts.forEach(function(p){ctx.lineTo(px(p.x),py(p.y));});
      for(var j=120;j>=0;j--){ctx.lineTo(px(j/120),py(j/120));}
      ctx.closePath();ctx.fillStyle=ref.fc;ctx.fill();

      ctx.beginPath();
      rpts.forEach(function(p,i){if(i===0)ctx.moveTo(px(p.x),py(p.y));else ctx.lineTo(px(p.x),py(p.y));});
      ctx.strokeStyle=ref.lc;
      ctx.lineWidth=ref.label==='World Avg'?1.9:1.2;
      ctx.setLineDash(ref.label==='World Avg'?[7,3]:[5,4]);ctx.stroke();ctx.setLineDash([]);
    });

    // ── EQUALITY DIAGONAL ──────────────────────────────────────────────────
    var diag=ctx.createLinearGradient(px(0),py(0),px(1),py(1));
    diag.addColorStop(0,'rgba(155,114,246,0.9)');diag.addColorStop(1,'rgba(34,211,238,0.9)');
    ctx.beginPath();ctx.moveTo(px(0),py(0));ctx.lineTo(px(1),py(1));
    ctx.strokeStyle=diag;ctx.lineWidth=2;ctx.setLineDash([8,5]);ctx.stroke();ctx.setLineDash([]);

    // ── AEQUITAS CURVE ─────────────────────────────────────────────────────
    ctx.beginPath();
    lorenz.forEach(function(p,i){if(i===0)ctx.moveTo(px(p.x),py(p.y));else ctx.lineTo(px(p.x),py(p.y));});
    for(var j=lorenz.length-1;j>=0;j--){ctx.lineTo(px(lorenz[j].x),py(lorenz[j].x));}
    ctx.closePath();
    var aqFill=ctx.createLinearGradient(0,py(0.5),0,py(0));
    aqFill.addColorStop(0,'rgba(240,180,41,0.48)');aqFill.addColorStop(1,'rgba(240,180,41,0.04)');
    ctx.fillStyle=aqFill;ctx.fill();

    ctx.beginPath();
    lorenz.forEach(function(p,i){if(i===0)ctx.moveTo(px(p.x),py(p.y));else ctx.lineTo(px(p.x),py(p.y));});
    ctx.save();ctx.shadowColor='rgba(240,180,41,0.8)';ctx.shadowBlur=12;
    ctx.strokeStyle='#F0B429';ctx.lineWidth=3;ctx.stroke();ctx.restore();
    lorenz.slice(1).forEach(function(p){
      ctx.beginPath();ctx.arc(px(p.x),py(p.y),4,0,2*Math.PI);
      ctx.fillStyle='#F0B429';ctx.fill();
      ctx.strokeStyle='rgba(0,0,0,0.6)';ctx.lineWidth=1;ctx.stroke();
    });

    // ── LEGEND ────────────────────────────────────────────────────────────
    var legendItems = [
      {label:'Aequitas',         gStr:gini.toFixed(4), color:'#F0B429', bold:true},
      {label:'Perfect Equality', gStr:'0.00',          color:'rgba(155,114,246,0.9)', bold:false}
    ];
    refs.slice().sort(function(a,b){return a.g-b.g;}).forEach(function(ref){
      legendItems.push({label:ref.label, gStr:ref.g.toFixed(2), color:ref.lc, bold:false});
    });

    // Dots at x=50% in chart (both mobile and desktop)
    legendItems.forEach(function(item){
      var dotY;
      if(item.bold) { dotY = py(aqY50); }
      else if(item.label==='Perfect Equality') { dotY = py(0.5); }
      else {
        var rm = refs.filter(function(r){return r.label===item.label;})[0];
        dotY = rm ? py(Math.pow(0.5,1+2*rm.g)) : null;
      }
      if(dotY != null) {
        ctx.beginPath(); ctx.arc(px(0.5), dotY, item.bold?5:3, 0, 2*Math.PI);
        ctx.fillStyle=item.color; ctx.fill();
        if(item.bold){ctx.strokeStyle='rgba(0,0,0,0.7)';ctx.lineWidth=1;ctx.stroke();}
      }
    });

    if(isMobile) {
      // ── MOBILE LEGEND: compact 2-column grid below chart ──────────────
      var legTop = pad.t + cH + 44;
      var colW = Math.floor((W - pad.l - pad.r) / 2);
      var rowH = 26;
      legendItems.forEach(function(item, idx){
        var col = idx % 2, row = Math.floor(idx / 2);
        var lx2 = pad.l + col * colW;
        var ly2 = legTop + row * rowH;
        // color dot
        ctx.beginPath(); ctx.arc(lx2+6, ly2+7, 5, 0, 2*Math.PI);
        ctx.fillStyle = item.color; ctx.fill();
        // label
        ctx.fillStyle = item.bold ? item.color : 'rgba(232,237,245,0.85)';
        ctx.font = (item.bold ? 'bold ' : '') + '9px Inter';
        ctx.textAlign='left';
        ctx.fillText(item.label, lx2+16, ly2+8);
        // gini
        ctx.fillStyle = 'rgba(136,146,164,0.7)';
        ctx.font = '8.5px JetBrains Mono';
        ctx.fillText('G='+item.gStr, lx2+16, ly2+19);
      });
    } else {
      // ── DESKTOP LEGEND: stacked right panel ───────────────────────────
      var lx = pad.l + cW + 14;
      var lw = pad.r - 20;
      var itemH = Math.min(40, cH / legendItems.length);
      var totalH = itemH * legendItems.length;
      var startY = pad.t + (cH - totalH) / 2 + itemH / 2;
      legendItems.forEach(function(item, idx){
        var cy = startY + idx * itemH;
        ctx.globalAlpha = item.bold ? 1.0 : 0.85;
        ctx.fillStyle = item.color;
        ctx.fillRect(lx, cy - Math.min(itemH*0.38,14), 3, Math.min(itemH*0.76,28));
        ctx.globalAlpha = 1.0;
        ctx.fillStyle = item.color;
        ctx.font = (item.bold?'bold ':'')+' 11px Inter'; ctx.textAlign='left';
        ctx.fillText(item.label, lx+9, cy-2);
        ctx.fillStyle = item.bold ? item.color : 'rgba(232,237,245,0.88)';
        ctx.font = (item.bold?'bold ':'')+' 11.5px JetBrains Mono';
        ctx.fillText('G='+item.gStr, lx+9, cy+11);
        if(itemH>=32){
          ctx.fillStyle='rgba(136,146,164,0.5)'; ctx.font='8px Inter';
          var rm2 = refs.filter(function(r){return r.label===item.label;})[0];
          var owns = item.bold ? '50% own '+(aqY50*100).toFixed(1)+'%'
            : item.label==='Perfect Equality' ? '50% own 50%'
            : rm2 ? '50% own '+Math.round(Math.pow(0.5,1+2*rm2.g)*100)+'%' : '';
          if(owns) ctx.fillText(owns, lx+9, cy+22);
        }
      });
    }

    // ── BOTTOM NOTE ────────────────────────────────────────────────────────
    var noteY = pad.t + cH + 50;
    if(noteY < H - 4) {
      ctx.fillStyle = gini<0.10 ? 'rgba(52,211,153,0.8)' : 'rgba(136,146,164,0.5)';
      ctx.font = (gini<0.10?'bold ':'') + 'italic 8.5px Inter'; ctx.textAlign='center';
      var noteText = gini<0.10
        ? 'Aequitas Gini '+gini.toFixed(4)+' — 4.5x below world average (0.38) — near-perfect equality!'
        : 'Aequitas target: Gini < 0.30  ·  World average: 0.38  •  World average: 38%';
      ctx.fillText(noteText, px(0.5), noteY);
    }

  } catch(e){ console.error('Lorenz error:',e); }
}


function drawWcapSlideChart() {
  const canvas = document.getElementById('wcap-slide-chart');
  if (!canvas || !canvas.offsetParent) return;
  canvas.width = canvas.offsetWidth;
  const ctx = canvas.getContext('2d');
  const W = canvas.width, H = canvas.height;
  ctx.clearRect(0,0,W,H);
  const pad = {l:44,r:20,t:36,b:32};
  const cW = W-pad.l-pad.r, cH = H-pad.t-pad.b;
  const maxN = 28;
  const bw = cW/maxN;
  // horizontal reference lines
  [5,10,15,20,25].forEach(function(v){
    var y=H-pad.b-cH*(v/25);
    ctx.strokeStyle=v===25?'rgba(0,255,209,0.2)':'rgba(139,92,246,0.08)'; ctx.lineWidth=1;
    ctx.beginPath(); ctx.moveTo(pad.l,y); ctx.lineTo(W-pad.r,y); ctx.stroke();
    ctx.fillStyle='rgba(200,168,76,0.7)'; ctx.font='10px JetBrains Mono,monospace'; ctx.textAlign='right';
    ctx.fillText(v+'x', pad.l-5, y+4);
  });
  // bars
  for (var n=1;n<=maxN;n++) {
    var mult=Math.max(5,Math.min(n,25));
    var bh=cH*(mult/25), bx=pad.l+(n-1)*bw+1, bw2=bw-2;
    var y=H-pad.b-bh, r=Math.min(3,bw2/2);
    var barGrad;
    if (n>25) { barGrad='rgba(255,255,255,0.06)'; }
    else if (n===25) { var g=ctx.createLinearGradient(0,y,0,H-pad.b); g.addColorStop(0,'rgba(0,255,209,0.8)'); g.addColorStop(1,'rgba(0,255,209,0.25)'); barGrad=g; }
    else if (n>=20) { var g2=ctx.createLinearGradient(0,y,0,H-pad.b); g2.addColorStop(0,'rgba(200,168,76,0.85)'); g2.addColorStop(1,'rgba(200,168,76,0.28)'); barGrad=g2; }
    else { var g3=ctx.createLinearGradient(0,y,0,H-pad.b); g3.addColorStop(0,'rgba(200,168,76,0.6)'); g3.addColorStop(1,'rgba(200,168,76,0.18)'); barGrad=g3; }
    // rounded top bar
    ctx.beginPath();
    ctx.moveTo(bx+r,y); ctx.lineTo(bx+bw2-r,y);
    ctx.arcTo(bx+bw2,y,bx+bw2,y+r,r);
    ctx.lineTo(bx+bw2,H-pad.b); ctx.lineTo(bx,H-pad.b); ctx.lineTo(bx,y+r);
    ctx.arcTo(bx,y,bx+r,y,r); ctx.closePath();
    if (n===25){ctx.save();ctx.shadowColor='rgba(0,255,209,0.55)';ctx.shadowBlur=8;}
    ctx.fillStyle=barGrad; ctx.fill();
    if (n===25) ctx.restore();
    // labels at key N values
    if (n===1||n===5||n===10||n===15||n===20||n===25) {
      ctx.fillStyle=n===25?'rgba(0,255,209,0.9)':'rgba(200,168,76,0.85)';
      ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='center';
      ctx.fillText(mult+'x', bx+bw2/2, y-4);
      ctx.fillStyle='rgba(255,255,255,0.4)'; ctx.font='8px JetBrains Mono,monospace';
      ctx.fillText('N='+n, bx+bw2/2, H-pad.b+13);
    }
  }
  // lock line at N=25
  var lockY=H-pad.b-cH;
  ctx.save(); ctx.shadowColor='rgba(0,255,209,0.5)'; ctx.shadowBlur=5;
  ctx.strokeStyle='rgba(0,255,209,0.55)'; ctx.lineWidth=1.5; ctx.setLineDash([5,4]);
  ctx.beginPath(); ctx.moveTo(pad.l+(25-1)*bw,lockY); ctx.lineTo(W-pad.r,lockY); ctx.stroke();
  ctx.setLineDash([]); ctx.restore();
  ctx.fillStyle='rgba(0,255,209,0.8)'; ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='left';
  ctx.fillText('LOCKED AT 25x', pad.l+25*bw+4, lockY-4);
  // title
  ctx.fillStyle='rgba(200,168,76,0.35)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='left';
  ctx.fillText('WEALTH CAP  —  BOOTSTRAP MULTIPLIER  ·  max(5, min(N, 25))×', pad.l, 20);
}

// ── PRICE CHART (TradingView Lightweight Charts, DexScreener-style) ──────────
// The interval buttons are the CANDLE TIMEFRAME (like DexScreener), not a
// time-window filter. Each button = one candle's duration in ms; the chart
// shows the FULL history at that resolution and is scrollable/zoomable.
var lwChart = null, lwCandleSeries = null, lwVolSeries = null;

// buildOHLC aggregates every price point into candles of tfMs each — no window
// filter, so all recorded history is shown at the selected resolution.
function buildOHLC(pts, tfMs) {
  var data = pts.filter(function(p){return p.p>0;});
  if (!data.length) return [];
  var buckets = {};
  var vorige = null;
  data.forEach(function(pt) {
    var b = Math.floor(pt.t/tfMs)*tfMs;
    // ECHTES VOLUMEN, nicht die Zahl der Abfragen.
    //
    // Hier stand frueher vol:1 und vol++ -- gezaehlt wurden also die
    // Preis-Schnappschuesse. Ein Balken, der "Volumen" heisst und in
    // Wahrheit sagt, wie oft der Browser gepollt hat, ist keine
    // Ungenauigkeit, sondern eine falsche Auskunft: er waechst, wenn
    // NICHTS passiert, und ein Chart, der wie DexScreener aussieht, wird
    // auch so gelesen.
    //
    // In einem x*y=k-Pool aendert nur ein Tausch die Reserven. Der Betrag
    // der Aenderung der tUSD-Reserve zwischen zwei Schnappschuessen ist
    // also der gehandelte Wert. Ohne Handel ist er null -- und ein leerer
    // Volumenbereich ist die richtige Auskunft, wenn nicht gehandelt wurde.
    var gehandelt = 0;
    if (vorige && typeof pt.u === 'number' && typeof vorige.u === 'number') {
      gehandelt = Math.abs(pt.u - vorige.u);
    }
    if (!buckets[b]) {
      buckets[b] = {time:Math.floor(b/1000), open:pt.p, high:pt.p, low:pt.p, close:pt.p, vol:gehandelt};
    } else {
      buckets[b].high = Math.max(buckets[b].high, pt.p);
      buckets[b].low  = Math.min(buckets[b].low,  pt.p);
      buckets[b].close = pt.p;
      buckets[b].vol += gehandelt;
    }
    vorige = pt;
  });
  return Object.values(buckets).sort(function(a,b){return a.time-b.time;});
}

function initPriceChart() {
  var el = document.getElementById('price-chart');
  if (!el || lwChart || !window.LightweightCharts) return;
  lwChart = LightweightCharts.createChart(el, {
    width:el.clientWidth, height:260,
    layout:{background:{color:'#0A0C16'},textColor:'#8892A4',fontSize:11,fontFamily:'JetBrains Mono,monospace'},
    grid:{vertLines:{color:'rgba(255,255,255,0.03)'},horzLines:{color:'rgba(255,255,255,0.03)'}},
    crosshair:{mode:LightweightCharts.CrosshairMode.Normal,
      vertLine:{color:'rgba(155,114,246,0.55)',width:1,style:0,labelBackgroundColor:'#9B72F6'},
      horzLine:{color:'rgba(155,114,246,0.55)',width:1,style:0,labelBackgroundColor:'#9B72F6'}},
    rightPriceScale:{borderColor:'rgba(255,255,255,0.07)'},
    timeScale:{borderColor:'rgba(255,255,255,0.07)',timeVisible:true,secondsVisible:false,rightOffset:4},
    handleScroll:true, handleScale:true,
  });
  lwCandleSeries = lwChart.addCandlestickSeries({
    upColor:'#34D399',downColor:'#F87171',
    borderUpColor:'#34D399',borderDownColor:'#F87171',
    wickUpColor:'#34D399',wickDownColor:'#F87171',
    priceFormat:{type:'price',precision:6,minMove:0.000001},
  });
  lwVolSeries = lwChart.addHistogramSeries({
    priceFormat:{type:'volume'},priceScaleId:'vol',
    scaleMargins:{top:0.78,bottom:0},
  });
  try { lwChart.priceScale('vol').applyOptions({scaleMargins:{top:0.78,bottom:0},autoScale:false}); } catch(_){}
  new ResizeObserver(function(e){ if(lwChart&&e[0]) lwChart.applyOptions({width:e[0].contentRect.width}); }).observe(el);
}

function updatePriceDisplay(candles) {
  var pEl=document.getElementById('price-current'), cEl=document.getElementById('price-change');
  if (!pEl) return;
  if (!candles.length) { pEl.textContent='—'; return; }
  var last=candles[candles.length-1], first=candles[0];
  var chg = first.open>0 ? (last.close-first.open)/first.open*100 : 0;
  pEl.textContent = last.close.toFixed(6)+' tUSD';
  if (cEl) { cEl.textContent=(chg>=0?'▲ +':'▼ ')+Math.abs(chg).toFixed(2)+'%'; cEl.style.color=chg>=0?'var(--neon)':'var(--red)'; }
}

function drawPriceChart() {
  var el=document.getElementById('price-chart');
  if (!el||!el.offsetParent) return;
  if (!lwChart) initPriceChart();
  if (!lwChart) return;
  var candles = buildOHLC(priceHistory, chartIntervalMs);
  updatePriceDisplay(candles);
  if (!candles.length) {
    if (lwCandleSeries) lwCandleSeries.setData([]);
    if (lwVolSeries) lwVolSeries.setData([]);
    return;
  }
  // FLACHER MARKT: ehrlich aussehen statt kaputt.
  //
  // Hat sich der Preis nie bewegt -- am 26.08.2026 gemessen: 5.022 Punkte
  // ueber 42 Stunden, min = max, 100 % Doji --, dann skaliert die
  // Bibliothek die Achse auf die verbleibende Fliesskomma-Streuung. Das
  // ergibt eine Beschriftung wie 25,901354 bis 25,901366: sechs
  // Nachkommastellen Nichts, die aussehen, als sei der Chart defekt.
  //
  // Er ist es nicht -- es wurde schlicht nicht gehandelt. Also wird die
  // Achse aufgespannt (+/- 2 %) und der Zustand benannt, statt Rauschen zu
  // vergroessern. Ein Chart, der Stillstand als Bewegung zeichnet, luegt in
  // die andere Richtung.
  var alleGleich = candles.every(function(c){ return c.high === c.low && c.open === c.close; });
  var hinweis = document.getElementById('price-chart-flat');
  if (alleGleich && candles.length) {
    var kurs = candles[candles.length-1].close;
    lwCandleSeries.applyOptions({ autoscaleInfoProvider: function() {
      return { priceRange: { minValue: kurs * 0.98, maxValue: kurs * 1.02 } };
    }});
    if (hinweis) hinweis.style.display = 'block';
  } else {
    lwCandleSeries.applyOptions({ autoscaleInfoProvider: null });
    if (hinweis) hinweis.style.display = 'none';
  }

  lwCandleSeries.setData(candles.map(function(c){return {time:c.time,open:c.open,high:c.high,low:c.low,close:c.close};}));
  lwVolSeries.setData(candles.map(function(c){return {time:c.time,value:c.vol,color:c.close>=c.open?'rgba(52,211,153,0.22)':'rgba(248,113,113,0.22)'};}));
  // DexScreener behaviour: on a timeframe switch (or first draw) snap to the
  // most recent ~120 candles, scrollable back for older history. Live 8s
  // updates leave the view alone so the user's scroll position is preserved
  // (LightweightCharts auto-follows only when already at the right edge).
  if (chartRefitPending) {
    chartRefitPending = false;
    var N = 120;
    if (candles.length > N) {
      lwChart.timeScale().setVisibleLogicalRange({from: candles.length - N, to: candles.length + 1});
    } else {
      lwChart.timeScale().fitContent();
    }
  }
}

let allBlocks = [];
let latestChainHeight = 0;
// latestTipBlueScore backs openBlock()'s confirmation-confidence estimate —
// see its own comment. Updated on every loadBlocks() poll from the newest
// canonical block, exactly like latestChainHeight above.
let latestTipBlueScore = 0;
let dagAutoScrolled = false;
let validatorLabels = {};

// GHOSTDAG-verdict state shared between renderDagView (which computes it
// fresh every poll) and openBlock's detail modal (which has no reason to
// recompute the same sets just to answer "was this block blue or red?" for
// whichever block a mobile user — no hover, so no DAG-view tooltip — just
// tapped open). Always reflects the most recent render.
let lastBlueSet = new Set();
let lastReferencedSet = new Set();
let lastCanonicalHashSet = new Set();

// previousDagHashes tracks which block hashes were already on screen at the
// last renderDagView call, so a live poll can tell "genuinely new since a
// moment ago" apart from "already here" — only the former gets the
// dag-node-new entrance animation; re-rendering everything on every poll
// (the previous behavior) would otherwise replay it for the whole DAG each
// time.
let previousDagHashes = new Set();

// blueRatioHistory is a short rolling window of "% of the visible merge
// window GHOSTDAG counted as blue" samples, one per renderDagView call —
// real data already being computed for dag-verdict-stat, just kept around
// long enough to draw a trend instead of a single snapshot number.
const BLUE_RATIO_HISTORY_MAX = 40;
let blueRatioHistory = [];

// sparklineSVG builds a minimal inline SVG sparkline (no axes, no libs) —
// values in [0,1], most-recent last. Stroke color comes from the
// .dag-spark path CSS rule (explorer.css), not an inline attribute — SVG
// presentation attributes don't reliably resolve CSS custom properties
// across every browser the way an actual stylesheet rule does.
function sparklineSVG(values, width, height) {
  if (!values.length) return '';
  if (values.length === 1) values = [values[0], values[0]];
  const stepX = width / (values.length - 1);
  const pts = values.map(function(v, i) {
    const x = i * stepX;
    const y = height - v * height;
    return x.toFixed(1) + ',' + y.toFixed(1);
  });
  return '<svg class="dag-spark" width="' + width + '" height="' + height + '" viewBox="0 0 ' + width + ' ' + height + '">' +
    '<path d="M' + pts.join(' L') + '"/></svg>';
}

// loadValidatorLabels fetches the registration-order ordinal for every
// known signing address (see GetValidatorOrdinals' own comment, state.go,
// for why this is registration order, not a hardcoded per-node name).
// Called once at page load and refreshed alongside loadBlocks — cheap,
// and a newly-joined validator's first label should show up without a
// page reload.
// FIX (P3-e, audit 2026-07-06): mySeq guard, matching every sibling poller
// (loadStatus/loadHealth/loadBlocks/loadHumans/etc.) — without it, an
// overtaken (out-of-order-arriving) response could overwrite validatorLabels
// with stale data after a newer request already resolved.
let loadValidatorLabelsSeq = 0;
async function loadValidatorLabels() {
  const mySeq = ++loadValidatorLabelsSeq;
  try {
    const d = await (await fetch('/api/validator-labels')).json();
    if (mySeq !== loadValidatorLabelsSeq) return;
    validatorLabels = d.labels || {};
  } catch (e) {}
}

// validatorLabel returns the ready-to-display label for a known signing
// address (e.g. "Primary", "Validator #2" — computed server-side by
// handleValidatorLabels, see its own comment for why: VALIDATOR_LABELS
// overrides take precedence over the per-node GetValidatorOrdinals fallback
// there), or null if this address has no label yet (labels haven't loaded,
// or a very-just-joined validator nothing here knows about yet).
function validatorLabel(address) {
  return validatorLabels[(address || '').toLowerCase()] || null;
}

// DAG_MAX_HEIGHTS/DAG_MAX_ROWS bound the rendered window so a wild
// concurrent-production burst (many siblings at one height) can't blow up
// the SVG into something unreadable or slow to render — same reasoning as
// maxParentsPerBlock/maxMergeVisits on the backend (block.go).
const DAG_MAX_HEIGHTS = 24;
const DAG_MAX_ROWS = 5;
const DAG_COL_W = 56;
const DAG_ROW_H = 34;
const DAG_PAD = 26;

// knightdagActivationHeight is read from /api/status ("knightdag_activation_height",
// the backend's own authoritative KNIGHTDAG_ACTIVATION_HEIGHT — see that
// field's own comment in api.go). ONLY used for the informational "activates
// at #X" header text below; it is never guessed or hardcoded here, and it is
// NEVER used to decide whether an individual block gets the KnightDAG
// diamond — that decision always reads block.k_eff directly (set by the
// backend's own computeGHOSTDAGState for that exact block), so a block's
// marker can never disagree with what the chain that produced it actually
// did, regardless of what this node's local copy of the activation height
// says. Starts at Infinity (nothing "activates" until the real value loads)
// so a page freshly opened before the first /api/status response can't
// flash an incorrect "already active" or "activates at 0" message.
let knightdagActivationHeight = Infinity;

// renderDagView draws the actual GHOSTDAG parent/merge structure: one
// column per block height, one row per concurrent block at that height
// (deduplicated across every sibling — not just the canonical winner), and
// a curved edge for every real parent_hashes reference that lands on
// another block still in the visible window. This is what makes the DAG
// (not a linear chain) visible — a merge block literally has multiple
// incoming edges converging on it.
//
// Beyond structure, this renders GHOSTDAG's actual verdict wherever it can
// be determined from data already fetched: the selected-parent edge (the
// real chain link a block's blue_score is built on) is drawn thick and
// glowing purple, distinct from ordinary merge edges. Every node gets a
// colored ring in GHOSTDAG's own vocabulary — blue (counted toward
// blue_score) or red (excluded — too much concurrency in its anticone) —
// derived by unioning every visible block's own `blues` list against every
// hash any visible block references as a parent. A block whose classifying
// child hasn't arrived in the visible window yet (normally just the newest
// column) is left "pending" rather than guessed. Every block carrying
// k_eff (backend-computed — see Block.KEff) gets a gold diamond frame:
// GHOSTDAG's fixed per-epoch K didn't apply to it — it inferred its own
// from the DAG around it (see the Network → Consensus tab).
function renderDagView(rawBlocks, canonicalHashSet) {
  const svg = document.getElementById('dag-svg');
  const wrap = document.getElementById('dag-wrap');
  const tip = document.getElementById('dag-tip');
  if (!svg || !rawBlocks || !rawBlocks.length) return;
  const svgNS = 'http://www.w3.org/2000/svg';

  const byHash = {};
  rawBlocks.forEach(function(b) { if (b && b.hash) byHash[b.hash] = b; });

  const blueSet = new Set();
  const referencedSet = new Set();
  rawBlocks.forEach(function(b) {
    (b.blues || []).forEach(function(h) { blueSet.add(h); });
    (b.parent_hashes || []).forEach(function(h) { referencedSet.add(h); });
  });
  lastBlueSet = blueSet;
  lastReferencedSet = referencedSet;
  lastCanonicalHashSet = canonicalHashSet;

  let heights = Array.from(new Set(Object.keys(byHash).map(function(h) { return byHash[h].height; })));
  heights.sort(function(a, b) { return a - b; });
  if (heights.length > DAG_MAX_HEIGHTS) heights = heights.slice(heights.length - DAG_MAX_HEIGHTS);
  const colOf = {};
  heights.forEach(function(h, i) { colOf[h] = i; });

  const byHeight = {};
  Object.keys(byHash).forEach(function(hash) {
    const b = byHash[hash];
    if (!(b.height in colOf)) return;
    (byHeight[b.height] = byHeight[b.height] || []).push(b);
  });

  const nodePos = {};
  let maxRows = 1;
  heights.forEach(function(h) {
    let blocksAtH = (byHeight[h] || []).slice();
    blocksAtH.sort(function(a, b) {
      const aC = canonicalHashSet.has(a.hash) ? 0 : 1;
      const bC = canonicalHashSet.has(b.hash) ? 0 : 1;
      if (aC !== bC) return aC - bC;
      return (b.blue_score || 0) - (a.blue_score || 0);
    });
    if (blocksAtH.length > DAG_MAX_ROWS) blocksAtH = blocksAtH.slice(0, DAG_MAX_ROWS);
    maxRows = Math.max(maxRows, blocksAtH.length);
    const col = colOf[h];
    blocksAtH.forEach(function(b, row) {
      nodePos[b.hash] = {
        x: DAG_PAD + col * DAG_COL_W,
        y: DAG_PAD + row * DAG_ROW_H,
        block: b,
        canonical: canonicalHashSet.has(b.hash)
      };
    });
  });

  const width = DAG_PAD * 2 + Math.max(0, heights.length - 1) * DAG_COL_W;
  const height = DAG_PAD * 2 + Math.max(0, maxRows - 1) * DAG_ROW_H;
  svg.setAttribute('viewBox', '0 0 ' + width + ' ' + height);
  svg.setAttribute('width', width);
  svg.setAttribute('height', height);
  while (svg.firstChild) svg.removeChild(svg.firstChild);

  // Glow filter for the selected-parent path and the canonical chain's own
  // nodes — this is the chain the whole network actually agreed on, so it
  // should read as the one thread everything else is measured against.
  const defs = document.createElementNS(svgNS, 'defs');
  defs.innerHTML = '<filter id="dagGlow" x="-80%" y="-80%" width="260%" height="260%">' +
    '<feGaussianBlur stdDeviation="2" result="blur"/>' +
    '<feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>' +
    '</filter>';
  svg.appendChild(defs);

  const edgeGroup = document.createElementNS(svgNS, 'g');
  Object.keys(nodePos).forEach(function(hash) {
    const n = nodePos[hash];
    (n.block.parent_hashes || []).forEach(function(ph) {
      const p = nodePos[ph];
      if (!p) return;
      const isSelected = !!n.block.selected_parent && ph === n.block.selected_parent;
      const midX = (n.x + p.x) / 2;
      const path = document.createElementNS(svgNS, 'path');
      path.setAttribute('d', 'M' + p.x + ',' + p.y + ' C ' + midX + ',' + p.y + ' ' + midX + ',' + n.y + ' ' + n.x + ',' + n.y);
      path.setAttribute('class', isSelected ? 'dag-edge dag-edge-selected' : 'dag-edge');
      if (isSelected) path.setAttribute('filter', 'url(#dagGlow)');
      edgeGroup.appendChild(path);
    });
  });
  svg.appendChild(edgeGroup);

  const nodeGroup = document.createElementNS(svgNS, 'g');
  Object.keys(nodePos).forEach(function(hash) {
    const n = nodePos[hash];
    // GHOSTDAG's own vocabulary drives the ring color: on the accepted
    // selected-parent chain, explicitly blue-classified by some visible
    // block, explicitly excluded (red) by some visible block, or not yet
    // classified by anything in the window (pending — normally just the
    // newest column).
    const status = n.canonical ? 'selected' : (blueSet.has(hash) ? 'blue' : (referencedSet.has(hash) ? 'red' : 'pending'));
    // Authoritative: k_eff is only ever set by the backend for a block whose
    // OWN computeGHOSTDAGState actually ran the adaptive-K path — see
    // knightdagActivationHeight's own comment above for why this must never
    // be re-derived from a height comparison client-side.
    const isKnight = n.block.k_eff != null;
    const isNew = !previousDagHashes.has(hash);
    const g = document.createElementNS(svgNS, 'g');
    g.setAttribute('class', 'dag-node dag-node-' + status + (n.canonical ? '' : ' dag-sibling') + (isNew ? ' dag-node-new' : ''));
    g.setAttribute('transform', 'translate(' + n.x + ',' + n.y + ')');
    // KnightDAG frame goes in FIRST so the verdict-colored circle sits on
    // top of it — a gold diamond outline around the whole node, not a tiny
    // satellite glyph nobody could see at this scale.
    if (isKnight) {
      const half = n.canonical ? 7.4 : 6;
      const frame = document.createElementNS(svgNS, 'rect');
      frame.setAttribute('x', -half);
      frame.setAttribute('y', -half);
      frame.setAttribute('width', half * 2);
      frame.setAttribute('height', half * 2);
      frame.setAttribute('transform', 'rotate(45)');
      frame.setAttribute('class', 'dag-knight-frame');
      g.appendChild(frame);
    }
    const circle = document.createElementNS(svgNS, 'circle');
    circle.setAttribute('r', n.canonical ? 7 : 5.5);
    // Encoding swap (launch feedback): the FILL now carries GHOSTDAG's own
    // verdict (via the dag-node-<status> CSS class — purple chain, blue
    // counted, red excluded, dark pending), because the verdict is the whole
    // point of this view. Proposer identity moves to the thin ring, where it
    // stays readable without drowning out the consensus story the way the
    // old bright per-proposer fills did.
    circle.style.stroke = avatarColor(n.block.proposer || '0x00');
    if (n.canonical) circle.setAttribute('filter', 'url(#dagGlow)');
    g.appendChild(circle);
    g.addEventListener('click', function() { openBlock(hash); });
    g.addEventListener('mousemove', function(ev) {
      if (!tip) return;
      tip.style.display = 'block';
      tip.style.left = (ev.clientX + 14) + 'px';
      tip.style.top = (ev.clientY + 14) + 'px';
      while (tip.firstChild) tip.removeChild(tip.firstChild);
      const statusLabel = {
        selected: '★ selected parent chain',
        blue: '● GHOSTDAG blue (counted)',
        red: '● GHOSTDAG red (excluded, still merged)',
        pending: '○ not yet classified'
      }[status];
      [
        '#' + n.block.height + ' · ' + statusLabel,
        'proposer: ' + short(n.block.proposer || '', 8, 4) + (validatorLabel(n.block.proposer) ? ' (' + validatorLabel(n.block.proposer) + ')' : ''),
        'blue_score: ' + (n.block.blue_score != null ? n.block.blue_score : '—'),
        'parents: ' + ((n.block.parent_hashes || []).length)
      ].concat(isKnight ? [
        (n.block.k_eff != null)
          ? '◆ KnightDAG: inferred k=' + n.block.k_eff + ' (adaptive — smallest k covering a merge-set majority)'
          : '◆ KnightDAG: adaptive K active for this block'
      ] : [])
      .forEach(function(line) {
        const div = document.createElement('div');
        div.textContent = line;
        tip.appendChild(div);
      });
    });
    g.addEventListener('mouseleave', function() { if (tip) tip.style.display = 'none'; });
    nodeGroup.appendChild(g);
  });
  svg.appendChild(nodeGroup);
  previousDagHashes = new Set(Object.keys(nodePos));

  // KnightDAG activation boundary: when the handover height falls INSIDE the
  // visible window, draw a dashed gold line between the last pure-GHOSTDAG
  // column and the first adaptive-K column — the exact place the consensus
  // rule changes. A window entirely on one side draws nothing (a permanent
  // line would be noise, and the header status below already says which
  // regime the chain is in).
  let firstKnightCol = -1;
  for (let ci = 0; ci < heights.length; ci++) {
    // Authoritative per-column check: does ANY block at this height carry
    // k_eff? (Same reasoning as isKnight above — never a height guess.)
    const blocksHere = byHeight[heights[ci]] || [];
    if (blocksHere.some(function(b) { return b.k_eff != null; })) { firstKnightCol = ci; break; }
  }
  if (firstKnightCol > 0) {
    const bx = DAG_PAD + firstKnightCol * DAG_COL_W - DAG_COL_W / 2;
    const line = document.createElementNS(svgNS, 'line');
    line.setAttribute('x1', bx); line.setAttribute('x2', bx);
    line.setAttribute('y1', 4); line.setAttribute('y2', height - 4);
    line.setAttribute('class', 'dag-knight-boundary');
    svg.appendChild(line);
    const lbl = document.createElementNS(svgNS, 'text');
    lbl.setAttribute('x', bx + 5);
    lbl.setAttribute('y', 11);
    lbl.setAttribute('class', 'dag-knight-boundary-label');
    lbl.textContent = '◆ KNIGHTDAG →';
    svg.appendChild(lbl);
  }
  const knightStatusEl = document.getElementById('dag-knight-status');
  if (knightStatusEl && heights.length) {
    // Authoritative: "active" means at least one VISIBLE block actually
    // carries k_eff, not a height guess — see isKnight's own comment above.
    const ks = [];
    Object.keys(nodePos).forEach(function(h2) {
      const ke = nodePos[h2].block.k_eff;
      if (ke != null) ks.push(ke);
    });
    if (ks.length) {
      // Median inferred k across the visible window — the one number that
      // shows the adaptive layer actually working.
      ks.sort(function(a, b) { return a - b; });
      knightStatusEl.textContent = '· ◆ KnightDAG active · median k=' + ks[Math.floor(ks.length / 2)];
    } else if (isFinite(knightdagActivationHeight)) {
      // knightdagActivationHeight (from /api/status) is only used for this
      // informational text — never to decide a per-block marker.
      knightStatusEl.textContent = '· KnightDAG activates at #' + knightdagActivationHeight.toLocaleString();
    } else {
      knightStatusEl.textContent = '';
    }
  }

  const countEl = document.getElementById('dag-node-count');
  if (countEl) countEl.textContent = Object.keys(nodePos).length + ' blocks · ' + heights.length + ' heights';

  // Live blue/red split, computed the same way the node rings are colored —
  // a real, honest number (not a fabricated K_eff) that shows GHOSTDAG's
  // classification actually happening across the visible window. The
  // sparkline plots blueRatioHistory — the same ratio sampled on every
  // poll — so the panel shows a trend, not just a snapshot.
  const verdictEl = document.getElementById('dag-verdict-stat');
  if (verdictEl) {
    let blues = 0, reds = 0;
    Object.keys(nodePos).forEach(function(hash) {
      if (nodePos[hash].canonical) return;
      if (blueSet.has(hash)) blues++;
      else if (referencedSet.has(hash)) reds++;
    });
    const total = blues + reds;
    if (total > 0) {
      const ratio = blues / total;
      blueRatioHistory.push(ratio);
      if (blueRatioHistory.length > BLUE_RATIO_HISTORY_MAX) blueRatioHistory.shift();
      verdictEl.innerHTML = '<span style="color:var(--dag-blue)">' + blues + ' blue</span> / <span style="color:var(--dag-red)">' + reds + ' red</span> (' + Math.round(100 * ratio) + '% counted)'
        + sparklineSVG(blueRatioHistory, 56, 16);
    } else {
      verdictEl.innerHTML = '';
    }
  }

  // Snap to the newest column on first render only — later live refreshes
  // must not fight a user who scrolled back to look at older history.
  if (wrap && !dagAutoScrolled) {
    dagAutoScrolled = true;
    wrap.scrollLeft = wrap.scrollWidth;
  }
}

let loadBlocksSeq = 0;
async function loadBlocks() {
  const mySeq = ++loadBlocksSeq;
  try {
    // FIX (durable fix, 2026-07-04 — "the explorer must show the same thing
    // on every node"): the table's rows now come from /api/blocks/canonical,
    // which walks the authoritative SelectedParent chain server-side (the
    // exact same logic GetBlockByHeight uses for cross-node divergence
    // checks) instead of guessing a "winner" per height client-side from
    // whatever raw siblings this node's own in-memory window happened to
    // hold at request time. That guess was confirmed live to disagree
    // node-to-node (different proposer, ~23,000-point blue_score gap at
    // neighbouring heights between the primary and Contabo 1) even when the
    // real canonical chain already agreed — see handleCanonicalBlocks'
    // comment (api.go). /api/blocks is still fetched, but now only to count
    // siblings-per-height for the "⟁N parallel blocks" badge and to back
    // search/detail lookups in allBlocks, which intentionally need every
    // sibling, not just the canonical one.
    const [canonicalBlocks, rawBlocks] = await Promise.all([
      fetch('/api/blocks/canonical?limit=30').then(function(r) { return r.json(); }),
      fetch('/api/blocks').then(function(r) { return r.json(); })
    ]);
    if (mySeq !== loadBlocksSeq) return;
    const list = document.getElementById('blocks-list');
    const txList = document.getElementById('txns-list');
    if (!canonicalBlocks || !canonicalBlocks.length) {
      if (list) list.innerHTML = '<tr><td colspan="5" class="exp-empty">No blocks yet</td></tr>';
      if (txList) txList.innerHTML = '<tr><td colspan="4" class="exp-empty">No transactions yet</td></tr>';
      return;
    }
    allBlocks = rawBlocks || [];
    // siblingsAt[h] = total count of blocks at that height (for DAG sibling display) —
    // purely cosmetic now, no longer used to pick which block represents the height.
    const siblingsAt = {};
    const txCountAt = {};
    (rawBlocks || []).forEach(function(b) {
      siblingsAt[b.height] = (siblingsAt[b.height] || 0) + 1;
      txCountAt[b.height] = (txCountAt[b.height] || 0) + ((b.transactions || []).length);
    });
    // DAG view needs every sibling (rawBlocks) plus the authoritative
    // canonical set (to mark which one is the real selected-parent chain) —
    // merge canonical blocks into allBlocks too, since a canonical block can
    // in principle fall just outside /api/blocks' last-50 raw window.
    const canonicalHashSet = new Set();
    (canonicalBlocks || []).forEach(function(b) {
      canonicalHashSet.add(b.hash);
      if (!allBlocks.some(function(x) { return x.hash === b.hash; })) allBlocks.push(b);
    });
    renderDagView(allBlocks, canonicalHashSet);
    // Active Validators: distinct proposers seen across the raw sibling
    // window (not just the canonical chain, which only ever shows ONE
    // winner per height) — this is the real, live GHOSTDAG parallelism
    // number, computed from data already being fetched rather than a
    // hardcoded network-size claim that goes stale the moment a validator
    // joins or drops (see s-height-sub's own 2026-07-05 fix for the exact
    // failure mode a static number here would repeat).
    const validatorsEl = document.getElementById('s-validators');
    if (validatorsEl && rawBlocks && rawBlocks.length) {
      const distinctProposers = new Set(rawBlocks.map(function(b) { return (b.proposer || '').toLowerCase(); }));
      validatorsEl.textContent = distinctProposers.size;
    }
    const dedupedBlocks = canonicalBlocks.slice().sort(function(a, b) { return b.height - a.height; });
    if (dedupedBlocks.length && dedupedBlocks[0].blue_score != null) {
      latestTipBlueScore = dedupedBlocks[0].blue_score;
    }
    // FIX: this used to show dedupedBlocks.length — the deduped count of
    // whatever page of blocks was just fetched (capped at 50), not the
    // true chain height. Once the chain passed 50 blocks that number
    // stopped meaning anything ("47 blocks" forever while the real height
    // climbed into the thousands). Prefer the real height from loadStatus().
    document.getElementById('block-count').textContent =
      (latestChainHeight || dedupedBlocks.length) + ' blocks';
    // Populate block table rows (latest 30)
    if (list) {
      list.innerHTML = dedupedBlocks.slice(0, 30).map(function(b) {
        const merge = b.parent_hashes && b.parent_hashes.length > 1;
        // Count transactions across EVERY block at this height, not just the
        // canonical one. Same reason the transaction list below reads siblings:
        // GHOSTDAG merges them, so a transfer in a sibling really was included
        // at this height — showing "0" while the list underneath displays that
        // very transfer is how a user concludes their transaction vanished.
        const txCount = txCountAt[b.height] != null ? txCountAt[b.height] : (b.transactions || []).length;
        const vLabel = validatorLabel(b.proposer);
        const proposer = b.proposer ? short(b.proposer, 6, 4) : '—';
        const sibCount = siblingsAt[b.height] || 1;
        const sibBadge = sibCount > 1 ? ' <span style="font-size:0.48rem;color:var(--gold);vertical-align:middle" title="' + sibCount + ' parallel blocks at this height (GHOSTDAG DAG)">⟁' + sibCount + '</span>' : '';
        const typeBadge = merge
          ? '<span class="exp-badge exp-badge-merge">MERGE</span>'
          : '<span class="exp-badge exp-badge-std">STD</span>';
        const txBadge = txCount > 0
          ? '<span class="exp-badge exp-badge-tx">' + txCount + '</span>'
          : '<span class="exp-muted">0</span>';
        const blueScore = b.blue_score != null ? sanitize(String(b.blue_score)) : '<span class="exp-muted">—</span>';
        return '<tr class="exp-tr" data-act="openBlock" data-args="' + attrEscape(JSON.stringify([b.hash])) + '">' +
          '<td style="color:var(--purple);font-weight:700">#' + sanitize(String(b.height)) + sibBadge + '</td>' +
          '<td class="exp-muted" style="font-size:0.6rem">' + sanitize(timeAgo(b.timestamp)) + '</td>' +
          '<td>' + txBadge + '</td>' +
          '<td class="exp-addr" style="font-size:0.6rem">' + (vLabel ? ('<strong>' + sanitize(vLabel) + '</strong> <span class="exp-muted">(' + sanitize(proposer) + ')</span>') : sanitize(proposer)) + '</td>' +
          '<td>' + typeBadge + '</td>' +
          '<td style="color:var(--teal);font-size:0.6rem">' + blueScore + '</td>' +
          '</tr>';
      }).join('');
    }
    // Collect all transactions across blocks, newest first.
    //
    // FIX (2026-07-27, reported live): this walked dedupedBlocks — the
    // CANONICAL chain — and therefore missed every transaction that landed in a
    // merged sibling. In a BlockDAG several validators produce at the same
    // height; GHOSTDAG selects one as canonical and MERGES the rest, so a
    // transaction in a sibling is fully valid, executed and reflected in state.
    // It simply is not in the selected-parent block.
    //
    // Confirmed on the live chain: a user's 20 AEQ transfer sat in one of three
    // blocks at height 2023173. eth_getTransactionReceipt returned it correctly
    // and chain_tx_block_index recorded it — but the explorer showed "No
    // transactions yet", because the canonical block at that height was one of
    // the two empty siblings. From the user's side a successful transfer had
    // simply vanished.
    //
    // rawBlocks (/api/blocks) already carries every sibling; it was being
    // fetched only to count them for the parallel-blocks badge. Reading the
    // transaction list from it costs nothing extra and shows what actually
    // happened on the chain.
    if (txList) {
      const allTxs = [];
      const txSource = (rawBlocks && rawBlocks.length) ? rawBlocks : dedupedBlocks;
      const seenTx = new Set();
      txSource.slice().sort(function(a, b) { return b.height - a.height; }).forEach(function(b) {
        (b.transactions || []).forEach(function(tx) {
          // The same transaction can legitimately appear in more than one
          // sibling at a height — show it once, attributed to the first block
          // seen, rather than listing an apparent duplicate transfer.
          const key = tx.tx_hash || (b.hash + '|' + (tx.wallet || '') + '|' + (tx.to || '') + '|' + (tx.amount || ''));
          if (seenTx.has(key)) return;
          seenTx.add(key);
          allTxs.push({ tx: tx, blockHeight: b.height, blockHash: b.hash });
        });
      });
      const txCountEl = document.getElementById('tx-count');
      if (txCountEl) txCountEl.textContent = allTxs.length + ' txns';
      if (allTxs.length === 0) {
        txList.innerHTML = '<tr><td colspan="4" class="exp-empty">No transactions yet</td></tr>';
      } else {
        txList.innerHTML = allTxs.slice(0, 30).map(function(item) {
          const tx = item.tx;
          const ref = tx.tx_hash || tx.wallet || '—';
          const shortRef = ref.length > 14 ? ref.slice(0, 8) + '…' + ref.slice(-4) : ref;
          const amt = (tx.amount && parseFloat(tx.amount) > 0) ? sanitize((+tx.amount).toFixed(4)) + ' AEQ' : '<span class="exp-muted">—</span>';
          const typeColor = tx.type === 'register_human' ? 'var(--gold)' : tx.type === 'transfer' ? 'var(--teal)' : tx.type === 'swap' ? 'var(--purple)' : 'var(--muted)';
          return '<tr class="exp-tr" data-act="openBlock" data-args="' + attrEscape(JSON.stringify([item.blockHash])) + '">' +
            '<td style="color:var(--teal);font-size:0.59rem">' + sanitize(shortRef) + '</td>' +
            '<td style="color:var(--purple);font-weight:700">#' + sanitize(String(item.blockHeight)) + '</td>' +
            '<td style="color:' + typeColor + ';font-size:0.59rem">' + sanitize(tx.type || '—') + '</td>' +
            '<td style="font-size:0.62rem">' + amt + '</td>' +
            '</tr>';
        }).join('');
      }
    }
  } catch (e) {}
}

function expSearchFail() {
  const msgEl = document.getElementById('exp-search-input');
  if (msgEl) { msgEl.style.borderColor = 'var(--red)'; setTimeout(function() { msgEl.style.borderColor = ''; }, 1500); }
}

// FIX: this used to only search allBlocks — whatever ~50 most recent blocks
// happened to be cached client-side from the live list. The chain has no
// upper bound on height, so searching for anything older than the last 50
// blocks silently found nothing, with no indication that the block might
// simply not have been fetched yet (as opposed to not existing at all).
// Now falls back to a direct server lookup (/api/block?height=/?hash=) for
// anything not already cached, so search works for the entire chain
// history without the live list itself needing to grow.
async function expSearch() {
  const q = ((document.getElementById('exp-search-input') || {}).value || '').trim();
  if (!q) return;
  const byNum = allBlocks.find(function(b) { return String(b.height) === q; });
  if (byNum) { openBlock(byNum.hash); return; }
  const ql = q.toLowerCase();
  const byHash = allBlocks.find(function(b) { return b.hash && b.hash.toLowerCase().startsWith(ql); });
  if (byHash) { openBlock(byHash.hash); return; }

  try {
    const isHeight = /^\d+$/.test(q);
    const url = isHeight ? ('/api/block?height=' + encodeURIComponent(q)) : ('/api/block?hash=' + encodeURIComponent(q));
    const resp = await fetch(url);
    if (!resp.ok) { expSearchFail(); return; }
    const block = await resp.json();
    if (!block || !block.hash) { expSearchFail(); return; }
    allBlocks.push(block);
    openBlock(block.hash);
  } catch (e) {
    expSearchFail();
  }
}

function openBlock(hash) {
  const b = allBlocks.find(function(x) { return x.hash === hash; });
  if (!b) return;
  document.getElementById('bdc-title').textContent = 'Block #' + b.height;
  const ts = new Date(b.timestamp * 1000);
  const isMerge = b.parent_hashes && b.parent_hashes.length > 1;
  // All peer-supplied block fields go through sanitize() before innerHTML
  // to prevent XSS — an authorized validator can sign arbitrary content.
  const parentList = (b.parent_hashes || []).map(function(h) {
    const pb = allBlocks.find(function(x) { return x.hash === h; });
    const pProp = pb && pb.proposer ? ' <span style="color:var(--purple);font-size:0.5rem">(' + sanitize(short(pb.proposer, 6, 4)) + ')</span>' : '';
    return '<div style="margin-bottom:3px;font-size:0.54rem;word-break:break-all">' + sanitize(h) + pProp + '</div>';
  }).join('') || '<span style="color:var(--muted)">None (genesis)</span>';
  const txs = b.transactions || [];
  let html = '';
  html += '<div class="bdc-row"><div class="bdc-k">Height</div><div class="bdc-v">'
    + '<span style="color:var(--purple);font-weight:700">#' + sanitize(String(b.height)) + '</span>'
    + (b.is_genesis ? ' <span class="bm">GENESIS</span>' : '')
    + (isMerge ? ' <span class="bm">MERGE</span>' : '') + '</div></div>';
  html += '<div class="bdc-row"><div class="bdc-k">Timestamp</div><div class="bdc-v">'
    + sanitize(ts.toUTCString()) + ' <span style="color:var(--muted)">(' + sanitize(timeAgo(b.timestamp)) + ')</span></div></div>';
  html += '<div class="bdc-row"><div class="bdc-k">Transactions</div><div class="bdc-v"><span style="color:var(--neon);font-weight:700">'
    + txs.length + '</span> in this block</div></div>';
  html += '<div class="bdc-row"><div class="bdc-k">Humans in Chain</div><div class="bdc-v">' + sanitize(String(b.humans || 0)) + '</div></div>';
  html += '<div class="bdc-row"><div class="bdc-k">Type</div><div class="bdc-v">'
    + (isMerge
      ? '<span class="exp-badge exp-badge-merge">MERGE BLOCK</span> &mdash; ' + sanitize(String(b.parent_hashes.length)) + ' parents merged'
      : '<span class="exp-badge exp-badge-std">STANDARD</span> &mdash; 1 parent') + '</div></div>';
  if (b.blue_score != null) {
    html += '<div class="bdc-row"><div class="bdc-k">GHOSTDAG Blue Score</div><div class="bdc-v" style="color:var(--teal);font-weight:700">'
      + sanitize(String(b.blue_score)) + ' <span style="color:var(--muted);font-weight:400;font-size:0.55rem">canonical ordering key</span></div></div>';
  }
  // GHOSTDAG verdict + KnightDAG flag — the DAG view's hover tooltip shows
  // this too, but hover doesn't exist on touch devices; this block detail
  // modal (reachable by tap, same as desktop click) is the one place every
  // device can see it. Reuses the exact same classification the DAG view
  // just rendered (lastBlueSet/lastReferencedSet/lastCanonicalHashSet),
  // not a re-derivation that could disagree with what's on screen.
  const verdict = lastCanonicalHashSet.has(b.hash) ? 'selected'
    : lastBlueSet.has(b.hash) ? 'blue'
    : lastReferencedSet.has(b.hash) ? 'red' : 'pending';
  const verdictColor = { selected: 'var(--purple)', blue: 'var(--dag-blue)', red: 'var(--dag-red)', pending: 'var(--muted)' }[verdict];
  const verdictText = {
    selected: '★ selected parent chain',
    blue: '● GHOSTDAG blue — counted toward blue_score',
    red: '● GHOSTDAG red — excluded, still merged into the DAG',
    pending: '○ not yet classified (its classifying block hasn\'t arrived)'
  }[verdict];
  html += '<div class="bdc-row"><div class="bdc-k">GHOSTDAG Verdict</div><div class="bdc-v" style="color:' + verdictColor + ';font-weight:700">' + verdictText + '</div></div>';
  if (b.k_eff != null) {
    html += '<div class="bdc-row"><div class="bdc-k">KnightDAG</div><div class="bdc-v" style="color:var(--gold);font-weight:700">'
      + sanitize('◆ inferred k=' + b.k_eff + ' (adaptive)') + '</div></div>';
  }
  // Confirmation confidence: a real number derived from GHOSTDAG's own
  // blue-weight accumulation (DAGKNIGHT's actual contribution — confidence
  // grows with confirmed weight, not with a fixed block-count heuristic),
  // computed entirely client-side from data already on screen. Depth =
  // how much blue_score has piled up on the canonical chain since this
  // block. Only meaningful for a block GHOSTDAG has actually accepted
  // (selected or blue); a pending/red block never gains confidence this way.
  if (b.blue_score != null && latestTipBlueScore > 0 && (verdict === 'selected' || verdict === 'blue')) {
    const depth = Math.max(0, latestTipBlueScore - b.blue_score);
    let confLabel, confColor;
    if (depth < 1) { confLabel = 'just landed'; confColor = 'var(--muted)'; }
    else if (depth < 3) { confLabel = 'low — still settling'; confColor = 'var(--gold)'; }
    else if (depth < 10) { confLabel = 'medium'; confColor = 'var(--teal)'; }
    else { confLabel = 'high — deeply buried under blue weight'; confColor = 'var(--dag-blue)'; }
    html += '<div class="bdc-row"><div class="bdc-k">Confirmation Confidence</div><div class="bdc-v" style="color:' + confColor + ';font-weight:700">'
      + sanitize(confLabel) + ' <span style="color:var(--muted);font-weight:400;font-size:0.55rem">(' + depth + ' blue_score behind tip)</span></div></div>';
  }
  const bLabel = validatorLabel(b.proposer);
  html += '<div class="bdc-row"><div class="bdc-k">Proposer</div><div class="bdc-v" style="color:var(--teal);word-break:break-all;font-size:0.54rem">'
    + (bLabel ? ('<strong>' + sanitize(bLabel) + '</strong> &mdash; ') : '') + sanitize(b.proposer || '—') + '</div></div>';
  html += '<div class="bdc-row"><div class="bdc-k">Block Hash</div><div class="bdc-v" style="font-size:0.52rem;word-break:break-all">'
    + sanitize(b.hash || '') + '</div></div>';
  if (b.state_root) {
    html += '<div class="bdc-row"><div class="bdc-k">State Root</div><div class="bdc-v" style="font-size:0.52rem;word-break:break-all">'
      + sanitize(b.state_root) + '</div></div>';
  }
  html += '<div class="bdc-row"><div class="bdc-k">Parent Hash(es)</div><div class="bdc-v">' + parentList + '</div></div>';
  if (txs.length > 0) {
    html += '<div class="bdc-tx-hdr">Transactions (' + txs.length + ')</div>';
    txs.forEach(function(tx) {
      const typeColor = tx.type === 'register_human' ? 'var(--gold)' : tx.type === 'transfer' ? 'var(--teal)' : tx.type === 'swap' ? 'var(--purple)' : 'var(--neon)';
      html += '<div class="bdc-tx">'
        + '<div style="display:flex;justify-content:space-between;margin-bottom:5px">'
        + '<span style="color:' + typeColor + ';font-weight:700;font-family:var(--font-body);font-size:0.65rem">' + sanitize(tx.type || '?') + '</span>'
        + (tx.amount && parseFloat(tx.amount) > 0 ? '<span style="color:var(--neon)">' + sanitize((+tx.amount).toFixed(6)) + ' AEQ</span>' : '')
        + '</div>'
        + (tx.wallet ? '<div style="color:var(--muted)">WALLET: <span style="color:var(--text)">' + sanitize(tx.wallet) + '</span></div>' : '')
        + (tx.to ? '<div style="color:var(--muted)">TO: <span style="color:var(--teal)">' + sanitize(tx.to) + '</span></div>' : '')
        + (tx.tx_hash ? '<div style="color:var(--muted)">TX: <span style="color:var(--purple)">' + sanitize(tx.tx_hash) + '</span></div>' : '')
        + '</div>';
    });
  } else {
    html += '<div class="bdc-row"><div class="bdc-k">Transactions</div><div class="bdc-v" style="color:var(--muted)">Empty block</div></div>';
  }
  document.getElementById('bdc-content').innerHTML = html;
  document.getElementById('block-detail-overlay').classList.add('open');
  document.body.style.overflow = 'hidden';
}

function closeBlock() {
  document.getElementById('block-detail-overlay').classList.remove('open');
  document.body.style.overflow = '';
}

// ── GUARDIAN SYSTEM ──────────────────────────────────────────────────────────
function guardianLog(msg, type) {
  const el = document.getElementById('guardian-log');
  if (!el) return;
  el.textContent = msg;
  el.style.color = type === 'ok' ? 'var(--neon)' : type === 'err' ? 'var(--red)' : 'var(--muted)';
}

async function loadGuardianStatus() {
  if (!waddr) return;
  // Always show the panel for registered humans — regardless of whether a
  // guardian is already set (404 from /api/guardian = no guardian yet = show "None")
  const panel = document.getElementById('guardian-panel');
  if (panel) panel.style.display = 'block';
  try {
    const resp = await fetch('/api/guardian?wallet=' + waddr);
    const addrEl = document.getElementById('guardian-addr-display');
    const noneStr = (T[curLang] && T[curLang]['guard-none']) || 'None';
    if (resp.ok) {
      const d = await resp.json();
      if (addrEl) addrEl.textContent = d.guardian || noneStr;
    } else {
      if (addrEl) addrEl.textContent = noneStr;
    }
  } catch(_) {
    const addrEl = document.getElementById('guardian-addr-display');
    if (addrEl) addrEl.textContent = (T[curLang] && T[curLang]['guard-none']) || 'None';
  }
  try {
    const resp = await fetch('/api/escrow?wallet=' + waddr);
    if (resp.ok) {
      const d = await resp.json();
      const warn = document.getElementById('escrow-warning');
      const amtEl = document.getElementById('escrow-amount-display');
      if (warn) warn.style.display = 'block';
      if (amtEl) amtEl.textContent = (d.amount || 0).toFixed(4) + ' AEQ';
    }
  } catch(_) {}
}

async function doSetGuardian() {
  if (!waddr || !activeProvider()) { guardianLog('Connect wallet first.', 'err'); return; }
  const guardian = (document.getElementById('guardian-input').value || '').trim().toLowerCase();
  if (!guardian.startsWith('0x') || guardian.length !== 42) {
    guardianLog('Enter a valid guardian address (0x... 42 chars).', 'err'); return;
  }
  try {
    guardianLog('Sign in your wallet to set guardian...', 'info');
    const msg = 'Aequitas: set guardian ' + guardian;
    const sig = await activeProvider().request({ method: 'personal_sign', params: [msg, waddr] });
    const resp = await fetch('/api/set-guardian', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet: waddr, guardian, signature: sig })
    });
    const d = await resp.json();
    if (d.guardian) {
      guardianLog('✓ Guardian set: ' + sanitize(d.guardian), 'ok');
      const addrEl = document.getElementById('guardian-addr-display');
      if (addrEl) addrEl.textContent = d.guardian;
      // FIX 5: clear input after successful set
      const inputEl = document.getElementById('guardian-input');
      if (inputEl) inputEl.value = '';
    } else {
      guardianLog('✗ ' + sanitize(d.error || 'Failed'), 'err');
    }
  } catch(e) { guardianLog('✗ ' + sanitize(e.message), 'err'); }
}

async function doGuardianConfirmAlive() {
  if (!waddr || !activeProvider()) { guardianLog('Connect wallet first.', 'err'); return; }
  const ward = (document.getElementById('ward-input').value || '').trim().toLowerCase();
  if (!ward.startsWith('0x') || ward.length !== 42) {
    guardianLog('Enter a valid ward address (0x... 42 chars).', 'err'); return;
  }
  // FIX 9: prevent self-confirmation — user cannot confirm themselves as alive
  if (ward === waddr.toLowerCase()) {
    guardianLog('You cannot confirm yourself — enter your ward\'s address.', 'err');
    return;
  }
  try {
    guardianLog('Sign in your wallet as guardian...', 'info');
    const msg = 'Aequitas: confirm alive ' + ward;
    const sig = await activeProvider().request({ method: 'personal_sign', params: [msg, waddr] });
    const resp = await fetch('/api/confirm-alive', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet: ward, guardian: waddr, signature: sig })
    });
    const d = await resp.json();
    if (d.success) {
      guardianLog('✓ Activity confirmed for ' + sanitize(ward), 'ok');
    } else {
      guardianLog('✗ ' + sanitize(d.error || 'Failed'), 'err');
    }
  } catch(e) { guardianLog('✗ ' + sanitize(e.message), 'err'); }
}

async function doRecoverEscrow() {
  if (!waddr || !activeProvider()) { guardianLog('Connect wallet first.', 'err'); return; }
  try {
    guardianLog('Sign in your wallet to recover escrow...', 'info');
    const msg = 'Aequitas: recover escrow ' + waddr;
    const sig = await activeProvider().request({ method: 'personal_sign', params: [msg, waddr] });
    const resp = await fetch('/api/recover-escrow', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet: waddr, signature: sig })
    });
    const d = await resp.json();
    if (d.success) {
      guardianLog('✓ Escrow recovered! New balance: ' + sanitize(String((d.new_balance || 0).toFixed(4))) + ' AEQ', 'ok');
      const warn = document.getElementById('escrow-warning');
      if (warn) warn.style.display = 'none';
    } else {
      guardianLog('✗ ' + sanitize(d.error || 'Failed'), 'err');
    }
  } catch(e) { guardianLog('✗ ' + sanitize(e.message), 'err'); }
}

let loadHumansSeq = 0;
async function loadHumans() {
  const mySeq = ++loadHumansSeq;
  try {
    // FIX (2026-07-27, reported live): this read the JSON without ever looking
    // at the HTTP status. /api/humans is rate limited to one request per 3s per
    // IP, and a 429 body is {"error":"rate limited, try again shortly"} — no
    // "humans" key. The check below then concluded the registry was empty and
    // rendered "No humans registered yet", while the very same page showed
    // "Total Humans 14" from /api/status right beside it.
    //
    // A transient rate limit was therefore making the strongest possible
    // negative claim about the chain: that nobody has ever registered. Reload
    // the page twice quickly, or keep a second tab open, and that is what a
    // visitor saw.
    const resp = await fetch('/api/humans');
    if (mySeq !== loadHumansSeq) return;
    const listEl = document.getElementById('humans-list');
    if (!resp.ok) {
      // Say what actually happened and keep whatever is already on screen
      // rather than replacing it with a claim about the registry.
      // Only replace a placeholder, never a list that is already showing real
      // entries — a transient 429 must not wipe a correct registry off screen.
      if (listEl && (!listEl.children.length || listEl.querySelector('.empty'))) {
        listEl.innerHTML = '<div class="empty">' +
          (resp.status === 429
            ? 'Too many requests just now — the list will refresh in a moment.'
            : 'Could not load the registry (HTTP ' + resp.status + '). Retrying shortly.') +
          '</div>';
      }
      return;
    }
    const d = await resp.json();
    if (mySeq !== loadHumansSeq) return;
    document.getElementById('h-count').textContent = fmt(d.total);
    const list = listEl;
    if (!d.humans || !d.humans.length) { list.innerHTML = '<div class="empty">No humans registered yet.<br><br>Download the Aequitas Android App and be the first!</div>'; return; }
    list.innerHTML = d.humans.map(h => {
      const color = avatarColor(h.address || '0x00');
      const init = (h.address || '??').slice(2, 4).toUpperCase();
      // Show TOTAL AEQ wealth (liquid + LP), not just the liquid balance — a
      // human who added all their AEQ as liquidity has balance 0 but is not
      // broke. When part of it is in the pool, note it so the number is clear.
      const total = (h.total_value_aeq != null) ? h.total_value_aeq : h.balance;
      const lpVal = h.lp_value_aeq || 0;
      const lpNote = lpVal > 0.000001
        ? '<span style="font-size:0.7em;color:var(--muted);font-weight:500"> · incl. ' + sanitize(fmt(lpVal)) + ' in LP</span>'
        : '';
      return '<div class="hi"><div class="hav" style="background:' + color + '20;color:' + color + ';border-color:' + color + '50">' + init + '</div><div style="flex:1;min-width:0"><div class="hbal">' + sanitize(fmt(total)) + ' AEQ' + lpNote + '</div><div class="hadr">' + sanitize(h.address || '—') + '</div></div><div class="hbdg">HUMAN</div></div>';
    }).join('');
  } catch (e) {}
}

// ── SWAP TAB ─────────────────────────────────────────────────────────────
let swapWaddr = null;
let swapDirection = 'aeq_to_tusd';
let currentPoolAEQ = 0;
let currentPoolTUSD = 0;
let myAEQBalance = 0;
let myTUSDBalance = 0;
var priceHistory = [];
var chartIntervalMs = 900000; // default candle timeframe: 15m (DexScreener-style)
var chartRefitPending = true;  // snap view to recent candles on next draw (tf change / first load)
var priceHistoryLoaded = false;
// Widest history window fetched so far (minutes), so switching to a range
// button already covered by what's loaded doesn't refetch needlessly.
// Matches preloadPriceHistory's own initial fetch window below.
var chartRangeMinutesLoaded = 14400;
// Max points kept client-side before the oldest are dropped. Long-range
// buttons (1y/all) can legitimately pull thousands of real DB points across
// priceHistoryRetentionDays (see evm_storage.go) — the old cap of 1000
// eroded exactly that preloaded long history a few points at a time as live
// 8s polling appended new ones during a long-open tab (launch audit
// 2026-07-03). 20000 comfortably covers a year of snapshots even under
// heavy swap activity while still bounding worst-case memory/redraw cost.
var priceHistoryMaxPoints = 20000;

// CHART_RANGES backs the long-range buttons (3d/1w/2w/1M/3M/1Y/ALL): unlike
// the short-timeframe buttons above (pure candle-size selectors over
// whatever's already in priceHistory), each of these also widens the actual
// fetched window via loadPriceRange, since the default preload only covers
// 10 days — nowhere near enough for "1y" or "all" to show real data. candleMs
// is chosen so the resulting candle COUNT stays chart-readable at that span
// (e.g. ~365 daily candles for 1y, not 525,600 one-minute candles).
var CHART_RANGES = {
  '3d':  { candleMs: 14400000,        minutes: 3   * 1440 },
  '1w':  { candleMs: 86400000,        minutes: 7   * 1440 },
  '2w':  { candleMs: 86400000,        minutes: 14  * 1440 },
  '1mo': { candleMs: 86400000,        minutes: 30  * 1440 },
  '3mo': { candleMs: 4 * 86400000,    minutes: 90  * 1440 },
  '1y':  { candleMs: 7 * 86400000,    minutes: 366 * 1440 },
  // "all" = as much as the server retains (priceHistoryRetentionDays in
  // evm_storage.go, 366 days) — there is no unbounded "since genesis" option
  // once a retention policy exists, so this asks for exactly that ceiling
  // rather than an arbitrarily large number that would just get clamped.
  'all': { candleMs: 7 * 86400000,    minutes: 366 * 1440 },
};
var ALL_CHART_BTN_IDS = ['ci-1m','ci-5m','ci-15m','ci-1h','ci-4h','ci-1d',
  'ci-3d','ci-1w','ci-2w','ci-1mo','ci-3mo','ci-1y','ci-all'];

function highlightChartBtn(activeId) {
  ALL_CHART_BTN_IDS.forEach(function(id) {
    var el = document.getElementById(id);
    if (el) el.className = 'ci-btn' + (id === activeId ? ' ci-active' : '');
  });
}

// Preload price history from DB so interval buttons show real historical
// data. Fetches the last 10 days of price snapshots saved after each
// swap/liquidity (enough for every short-timeframe button; long-range
// buttons widen this further on demand via loadPriceRange).
async function preloadPriceHistory() {
  if (priceHistoryLoaded) return;
  try {
    var d = await (await fetch('/api/price-history?minutes=' + chartRangeMinutesLoaded + '&limit=5000')).json();
    var hist = d.history || [];
    if (hist.length > 0) {
      // Merge DB history with any in-memory points, de-duplicate by timestamp
      var existing = new Set(priceHistory.map(function(p){ return p.t; }));
      hist.forEach(function(pt) {
        if (!existing.has(pt.t)) priceHistory.push({t: pt.t, p: pt.p, u: pt.u});
      });
      priceHistory.sort(function(a,b){ return a.t - b.t; });
      priceHistoryLoaded = true;
      drawPriceChart();
    }
  } catch(_) {}
}

// loadPriceRange widens priceHistory to cover at least minutesBack, fetching
// from the server only if not already covered by a previous load. Shared by
// setChartRange (explicit long-range buttons) below.
async function loadPriceRange(minutesBack) {
  if (minutesBack <= chartRangeMinutesLoaded) return;
  try {
    var d = await (await fetch('/api/price-history?minutes=' + minutesBack + '&limit=5000')).json();
    var hist = d.history || [];
    var existing = new Set(priceHistory.map(function(p){ return p.t; }));
    hist.forEach(function(pt) {
      if (!existing.has(pt.t)) priceHistory.push({t: pt.t, p: pt.p, u: pt.u});
    });
    priceHistory.sort(function(a,b){ return a.t - b.t; });
    chartRangeMinutesLoaded = minutesBack;
  } catch(_) {}
}

function setChartInterval(ms) {
  chartIntervalMs = ms;
  chartRefitPending = true; // snap to recent candles for the newly-selected timeframe
  var btnIds = ['ci-1m','ci-5m','ci-15m','ci-1h','ci-4h','ci-1d'];
  var btnVals = [60000,300000,900000,3600000,14400000,86400000];
  var activeId = null;
  for (var bi = 0; bi < btnIds.length; bi++) {
    if (btnVals[bi] === ms) { activeId = btnIds[bi]; break; }
  }
  highlightChartBtn(activeId);
  drawPriceChart();
}

// setChartRange handles the long-range buttons (3d/1w/2w/1M/3M/1Y/ALL): sets
// an appropriately coarser candle size for the span, widens priceHistory to
// cover it (fetching more from the server if needed), then redraws.
async function setChartRange(key) {
  var r = CHART_RANGES[key];
  if (!r) return;
  chartIntervalMs = r.candleMs;
  chartRefitPending = true;
  highlightChartBtn('ci-' + key);
  await loadPriceRange(r.minutes);
  drawPriceChart();
}

function swapLog(msg, type, allowHTML) {
  const el = document.getElementById('swap-log');
  if (!el) return;
  const row = document.createElement('div');
  const span = document.createElement('span');
  span.className = (type || 'info');
  if (allowHTML) {
    // only for explicit HTML content (e.g. MetaMask deep-links) — never pass server messages here
    span.innerHTML = msg;
  } else {
    span.textContent = msg; // default: treat as plain text
  }
  row.appendChild(span);
  el.appendChild(row);
  el.scrollTop = el.scrollHeight;
}

function sanitize(s) {
  const d = document.createElement('div');
  d.textContent = String(s);
  return d.innerHTML;
}

let loadPoolStatusSeq = 0;
async function loadPoolStatus() {
  const mySeq = ++loadPoolStatusSeq;
  try {
    const d = await (await fetch('/api/pool')).json();
    if (mySeq !== loadPoolStatusSeq) return;
    currentPoolAEQ = d.reserve_aeq;
    currentPoolTUSD = d.reserve_tusd;
    document.getElementById('pool-reserve-aeq').textContent = fmt(d.reserve_aeq) + ' AEQ';
    document.getElementById('pool-reserve-tusd').textContent = fmt(d.reserve_tusd) + ' tUSD';
    document.getElementById('pool-price').textContent = d.reserve_aeq > 0
      ? ('1 AEQ ≈ ' + d.price_aeq_in_tusd.toFixed(4) + ' tUSD')
      : 'No liquidity yet';
    const total = (d.reserve_aeq || 0) + (d.reserve_tusd || 0);
    if (total > 0) {
      const aeqPct = (d.reserve_aeq / total * 100).toFixed(1);
      const depthFill = document.getElementById('depth-aeq-fill');
      const aeqPctEl = document.getElementById('depth-aeq-pct');
      const tusdPctEl = document.getElementById('depth-tusd-pct');
      if (depthFill) depthFill.style.width = aeqPct + '%';
      if (aeqPctEl) aeqPctEl.textContent = aeqPct + '%';
      if (tusdPctEl) tusdPctEl.textContent = (100 - parseFloat(aeqPct)).toFixed(1) + '%';
    }
    const desc = document.getElementById('swap-addliq-desc');
    if (desc) {
      desc.textContent = d.reserve_aeq > 0
        ? ('Pool ratio: 1 AEQ ≈ ' + d.price_aeq_in_tusd.toFixed(4) + ' tUSD — match this ratio when depositing')
        : 'Be the first to deposit — your ratio sets the starting price.';
    }
    if (d.reserve_aeq > 0 && d.price_aeq_in_tusd > 0) {
      priceHistory.push({ t: Date.now(), p: d.price_aeq_in_tusd, u: d.reserve_tusd });
      if (priceHistory.length > priceHistoryMaxPoints) priceHistory.shift();
      drawPriceChart();
    }
    updateFeeEstimate();
  } catch (e) {}
}

function setSwapDirection(dir) {
  swapDirection = dir;
  const fromIcon = document.getElementById('swap-from-icon');
  const fromSym = document.getElementById('swap-from-sym');
  const toIcon = document.getElementById('swap-to-icon');
  const toSym = document.getElementById('swap-to-sym');
  if (dir === 'aeq_to_tusd') {
    if (fromIcon) fromIcon.textContent = '🔶'; if (fromSym) fromSym.textContent = 'AEQ';
    if (toIcon) toIcon.textContent = '💵'; if (toSym) toSym.textContent = 'tUSD';
  } else {
    if (fromIcon) fromIcon.textContent = '💵'; if (fromSym) fromSym.textContent = 'tUSD';
    if (toIcon) toIcon.textContent = '🔶'; if (toSym) toSym.textContent = 'AEQ';
  }
  // Sync balance labels in the from/to panels
  const fromBal = document.getElementById('swap-from-bal');
  const toBal = document.getElementById('swap-to-bal');
  if (fromBal) fromBal.textContent = dir === 'aeq_to_tusd' ? (fmt(myAEQBalance) + ' AEQ') : (fmt(myTUSDBalance) + ' tUSD');
  if (toBal) toBal.textContent = dir === 'aeq_to_tusd' ? (fmt(myTUSDBalance) + ' tUSD') : (fmt(myAEQBalance) + ' AEQ');
  updateFeeEstimate();
}

function reverseSwapDir() {
  setSwapDirection(swapDirection === 'aeq_to_tusd' ? 'tusd_to_aeq' : 'aeq_to_tusd');
  document.getElementById('swap-amount').value = '';
  updateFeeEstimate();
}

// Mirrors the same constant-product math the server uses (see swapLocked
// in state.go), so the UI can warn BEFORE asking for a signature instead
// of after a wasted MetaMask popup. This is just for live feedback —
// the server still re-validates for real when the swap actually submits,
// since the pool could change between typing and submitting.
function estimateSwapOutput(amountIn, aeqToTusd) {
  if (amountIn <= 0 || currentPoolAEQ <= 0 || currentPoolTUSD <= 0) return null;
  const fee = amountIn * 0.001;
  const amountInAfterFee = amountIn - fee;
  let amountOut, reserveOut;
  if (aeqToTusd) {
    amountOut = (currentPoolTUSD * amountInAfterFee) / (currentPoolAEQ + amountInAfterFee);
    reserveOut = currentPoolTUSD;
  } else {
    amountOut = (currentPoolAEQ * amountInAfterFee) / (currentPoolTUSD + amountInAfterFee);
    reserveOut = currentPoolAEQ;
  }
  return { amountOut, fee, tooLarge: amountOut >= reserveOut };
}

function updateFeeEstimate() {
  const amt = parseFloat(document.getElementById('swap-amount').value || '0');
  const aeqToTusd = swapDirection === 'aeq_to_tusd';
  const unit = aeqToTusd ? 'AEQ' : 'tUSD';
  const outUnit = aeqToTusd ? 'tUSD' : 'AEQ';
  const fee = amt * 0.001;
  const feeEl = document.getElementById('swap-fee-est');
  if (feeEl) feeEl.textContent = fee > 0 ? (fee.toFixed(6) + ' ' + unit) : '—';

  const panel = document.getElementById('swap-details-panel');
  const goBtn = document.getElementById('swap-btn-go');
  const warnEl = document.getElementById('swap-warn');

  if (amt <= 0) {
    if (panel) panel.style.display = 'none';
    warnEl.style.display = 'none';
    const od = document.getElementById('swap-out-est-dex'); if (od) od.textContent = '—';
    if (swapWaddr) goBtn.disabled = false;
    return;
  }
  if (currentPoolAEQ <= 0 || currentPoolTUSD <= 0) {
    if (panel) panel.style.display = 'none';
    warnEl.textContent = '⚠ Pool has no liquidity yet — deposit some below before swapping.';
    warnEl.style.display = 'block';
    if (swapWaddr) goBtn.disabled = true;
    return;
  }
  const est = estimateSwapOutput(amt, aeqToTusd);
  if (est && est.tooLarge) {
    if (panel) panel.style.display = 'none';
    // Binary-search the largest input that stays safely under the
    // reserve, so the warning can suggest a concrete number instead of
    // just saying "too much" — 99% of the output reserve as a safety
    // margin, since the pool could shift slightly before this submits.
    let lo = 0, hi = amt;
    for (let i = 0; i < 30; i++) {
      const mid = (lo + hi) / 2;
      const midEst = estimateSwapOutput(mid, aeqToTusd);
      if (midEst && midEst.amountOut < (aeqToTusd ? currentPoolTUSD : currentPoolAEQ) * 0.99) lo = mid;
      else hi = mid;
    }
    warnEl.textContent = '⚠ Too large for current pool liquidity. Try up to ~' + lo.toFixed(4) + ' ' + unit + '.';
    warnEl.style.display = 'block';
    if (swapWaddr) goBtn.disabled = true;
  } else if (est) {
    // Show swap details panel with price impact calculation
    if (panel) {
      panel.style.display = 'block';
      const outEl = document.getElementById('swap-out-est');
      const outDex = document.getElementById('swap-out-est-dex');
      const outStr = est.amountOut.toFixed(6) + ' ' + outUnit;
      if (outEl) outEl.textContent = outStr;
      if (outDex) outDex.textContent = outStr;
      // Price impact = how far execution price deviates from spot price
      const spotPrice = aeqToTusd ? (currentPoolTUSD / currentPoolAEQ) : (currentPoolAEQ / currentPoolTUSD);
      const amtAfterFee = amt - est.fee;
      const execPrice = amtAfterFee > 0 ? est.amountOut / amtAfterFee : 0;
      const impact = spotPrice > 0 ? Math.max(0, (1 - execPrice / spotPrice) * 100) : 0;
      const impEl = document.getElementById('swap-price-impact');
      if (impEl) {
        impEl.textContent = impact.toFixed(2) + '%';
        impEl.style.color = impact < 1 ? 'var(--neon)' : impact < 3 ? 'var(--gold)' : 'var(--red)';
      }
      const rateEl = document.getElementById('swap-rate-display');
      if (rateEl) rateEl.textContent = aeqToTusd
        ? ('1 AEQ = ' + (amtAfterFee > 0 ? est.amountOut / amtAfterFee : 0).toFixed(4) + ' tUSD')
        : ('1 tUSD = ' + (amtAfterFee > 0 ? est.amountOut / amtAfterFee : 0).toFixed(4) + ' AEQ');
      if (impact >= 5) {
        warnEl.textContent = '⚠ High price impact (' + impact.toFixed(2) + '%). Consider a smaller amount.';
        warnEl.style.display = 'block';
      } else {
        warnEl.style.display = 'none';
      }
    } else {
      warnEl.textContent = 'You will receive ≈ ' + est.amountOut.toFixed(6) + ' ' + outUnit;
      warnEl.style.display = 'block';
    }
    if (swapWaddr) goBtn.disabled = false;
  }
}

async function connectSwapWallet() {
  if (!window.ethereum) {
    const _isMobS = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);
    if (_isMobS) { const _dl = 'https://metamask.app.link/dapp/' + window.location.host; swapLog('🦊 Mobile: <a href="' + _dl + '" style="color:var(--gold)">In MetaMask App öffnen</a>', 'warn', true); } else { swapLog('🦊 MetaMask not found — <a href="https://metamask.io/download/" target="_blank" style="color:var(--gold)">install MetaMask</a>', 'warn', true); }
    return;
  }
  try {
    await addToMetaMask();
    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
    swapWaddr = accounts[0];
    waddr = swapWaddr;
    localStorage.setItem('aeq_wallet', swapWaddr);
    document.getElementById('swap-wbox').style.display = 'block';
    document.getElementById('swap-wadr').textContent = swapWaddr;
    const btn = document.getElementById('swap-btn-conn');
    btn.textContent = swapWaddr.slice(0, 10) + '...' + swapWaddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    const swapDBtn = document.getElementById('swap-btn-disconnect');
    if (swapDBtn) swapDBtn.style.display = 'block';
    const swapWcBtn = document.getElementById('swap-btn-wc');
    if (swapWcBtn) swapWcBtn.style.display = 'none';
    // Sync register tab wallet display
    const regBox = document.getElementById('wbox');
    const regAdr = document.getElementById('wadr');
    const regBtn = document.getElementById('btn-conn');
    const regDBtn = document.getElementById('btn-disconnect');
    const regWcBtn = document.getElementById('btn-wc');
    if (regBox) regBox.style.display = 'block';
    if (regAdr) regAdr.textContent = swapWaddr;
    if (regBtn) { regBtn.textContent = swapWaddr.slice(0, 10) + '...' + swapWaddr.slice(-4); regBtn.style.background = 'var(--green)'; regBtn.style.color = '#050A14'; }
    if (regDBtn) regDBtn.style.display = 'block';
    if (regWcBtn) regWcBtn.style.display = 'none';
    await refreshSwapBalances();
    await loadLPPosition();
    document.getElementById('swap-btn-go').disabled = false;
    document.getElementById('swap-btn-faucet').disabled = false;
    document.getElementById('swap-btn-addliq').disabled = false;
    setSwapDirection('aeq_to_tusd');
    // FIX 4: load guardian status for registered humans connecting via Exchange tab
    try {
      const balResp = await fetch('/api/balance?wallet=' + accounts[0]);
      const balData = await balResp.json();
      if (balData.is_human) loadGuardianStatus();
    } catch(_) {}
  } catch (e) {
    swapLog('Connection failed: ' + sanitize(e.message), 'err');
  }
}

async function refreshSwapBalances() {
  if (!swapWaddr) return;
  try {
    const br = await fetch('/api/balance?wallet=' + swapWaddr);
    const bd = await br.json();
    myAEQBalance = bd.balance || 0;
    myTUSDBalance = bd.tusd_balance || 0;
    document.getElementById('swap-bal-aeq').textContent = fmt(bd.balance) + ' AEQ';
    document.getElementById('swap-bal-tusd').textContent = fmt(bd.tusd_balance) + ' tUSD';
    // Update DEX from/to panel balance labels
    const fromBal = document.getElementById('swap-from-bal');
    const toBal = document.getElementById('swap-to-bal');
    if (fromBal) fromBal.textContent = swapDirection === 'aeq_to_tusd' ? (fmt(myAEQBalance) + ' AEQ') : (fmt(myTUSDBalance) + ' tUSD');
    if (toBal) toBal.textContent = swapDirection === 'aeq_to_tusd' ? (fmt(myTUSDBalance) + ' tUSD') : (fmt(myAEQBalance) + ' AEQ');
    showDemurrageNotice(bd);
  } catch (e) {}
}

// Surfaces the demurrage warning at "login" time (i.e. whenever the
// wallet connects/refreshes its balance) per the two-stage design: a
// one-time notice once the account enters the 14-day window (the server
// tracks whether this has already fired and won't repeat it), and a
// notice on every check once inside the final 7 days before decay
// actually starts. Once decay is active, a different, ongoing message
// is shown instead of either warning.
function showDemurrageNotice(bd) {
  const box = document.getElementById('demurrage-notice');
  if (!box) return;
  if (bd.demurrage_active) {
    box.style.display = 'block';
    box.textContent = '⏳ Part of your idle AEQ balance is now slowly decaying (0.5%/month) because it hasn\'t been used in over 3 months. Send, swap, or deposit any amount to reset the clock.';
  } else if (bd.show_7_day_notice) {
    box.style.display = 'block';
    box.textContent = '⏳ Your AEQ balance will start decaying in ' + (bd.demurrage_days_until_start || 0).toFixed(1) + ' days. Tip: transfer or swap to reset your activity timer.';
  } else if (bd.show_14_day_notice) {
    box.style.display = 'block';
    box.textContent = '💡 Heads up: if this balance stays untouched, it will start slowly decaying in about 2 weeks. Any transfer, swap, or deposit resets the countdown.';
  } else {
    box.style.display = 'none';
  }
}

// Fills the AddLiquidity input for side ('aeq' or 'tusd') with pct of
// the user's own balance for that currency (0.25/0.5/0.75/1 = 25/50/75/
// 100%). Triggers the existing ratio-matching logic afterward so the
// OTHER field auto-fills too, exactly as if the user had typed it
// themselves — same behavior, just one click instead of a calculator.
function setPctAmount(side, pct) {
  if (side === 'aeq') {
    const floored = Math.floor(myAEQBalance * pct * 1e6) / 1e6;
    document.getElementById('addliq-aeq').value = floored > 0 ? floored : '';
    updateLiquidityRatio('aeq');
  } else {
    const floored = Math.floor(myTUSDBalance * pct * 1e6) / 1e6;
    document.getElementById('addliq-tusd').value = floored > 0 ? floored : '';
    updateLiquidityRatio('tusd');
  }
}

function setSwapPct(pct) {
  const bal = swapDirection === 'aeq_to_tusd' ? myAEQBalance : myTUSDBalance;
  const amt = bal * pct;
  document.getElementById('swap-amount').value = amt > 0 ? amt.toFixed(6) : '';
  updateFeeEstimate();
}

// Signs a fixed, human-readable message describing exactly what's being
// authorized — the wallet owner sees this in MetaMask's signing prompt
// before approving, and the server checks the signature matches both the
// claimed wallet AND this exact message (see verifyPersonalSign in swap.go).
async function signMessage(message) {
  return await activeProvider().request({
    method: 'personal_sign',
    params: [message, swapWaddr]
  });
}

async function doSwap() {
  if (!swapWaddr) return;
  const amount = parseFloat(document.getElementById('swap-amount').value || '0');
  if (amount <= 0) { swapLog('Enter a valid amount', 'err'); return; }
  // FIX 7: guard against sub-precision amounts rounding to zero
  const preciseAmount = parseFloat(amount.toFixed(8));
  if (preciseAmount <= 0) { swapLog('Amount too small (minimum precision: 0.00000001)', 'err'); document.getElementById('swap-btn-go').disabled = false; return; }

  document.getElementById('swap-btn-go').disabled = true;
  try {
    const nonceResp = await fetch('/api/nonce?wallet=' + swapWaddr);
    const { nonce } = await nonceResp.json();
    const timestamp = Math.floor(Date.now() / 1000);
    const message = 'Aequitas Swap: ' + swapDirection + ' ' + preciseAmount.toFixed(8) + ' nonce:' + nonce + ' ts:' + timestamp;
    swapLog('Sign the message in MetaMask to confirm this swap...', 'info');
    const signature = await signMessage(message);

    // Slippage protection: the server supports an optional min_amount_out
    // floor (swap.go), but the UI never sent it, so swaps executed with zero
    // protection against the pool moving between quote and submission.
    // 1% tolerance below the last on-screen estimate.
    const aeqToTusd = swapDirection === 'aeq_to_tusd';
    const est = estimateSwapOutput(preciseAmount, aeqToTusd);
    const minAmountOut = est && !est.tooLarge ? est.amountOut * 0.99 : 0;

    const resp = await fetch('/api/swap', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet: swapWaddr, direction: swapDirection, amount: preciseAmount, nonce, timestamp, signature, min_amount_out: minAmountOut })
    });
    const data = await resp.json();
    if (data.success) {
      swapLog('✓ Swapped! Received ' + data.amount_out.toFixed(6) + ' ' + (swapDirection === 'aeq_to_tusd' ? 'tUSD' : 'AEQ'), 'ok');
      document.getElementById('swap-bal-aeq').textContent = fmt(data.new_aeq_balance) + ' AEQ';
      document.getElementById('swap-bal-tusd').textContent = fmt(data.new_tusd_balance) + ' tUSD';
      loadPoolStatus();
    } else {
      swapLog('✗ Swap failed: ' + sanitize(data.message), 'err');
    }
  } catch (e) {
    swapLog('✗ Error: ' + sanitize(e.message), 'err');
  }
  document.getElementById('swap-btn-go').disabled = false;
}

async function claimFaucet() {
  if (!swapWaddr) return;
  document.getElementById('swap-btn-faucet').disabled = true;
  try {
    const faucetTs = Math.floor(Date.now() / 1000);
    const message = 'Aequitas tUSD Faucet Claim: ' + swapWaddr.toLowerCase() + ' ts:' + faucetTs;
    swapLog('Sign the message in MetaMask to claim test-tUSD...', 'info');
    const signature = await signMessage(message);

    const resp = await fetch('/api/faucet', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet: swapWaddr, signature, timestamp: faucetTs })
    });
    const data = await resp.json();
    if (data.success) {
      swapLog('✓ Claimed ' + data.granted + ' test-tUSD', 'ok');
      await refreshSwapBalances();
    } else {
      swapLog('✗ Faucet claim failed: ' + sanitize(data.message), 'err');
      document.getElementById('swap-btn-faucet').disabled = false;
    }
  } catch (e) {
    swapLog('✗ Error: ' + sanitize(e.message), 'err');
    document.getElementById('swap-btn-faucet').disabled = false;
  }
}

// When the pool already has liquidity, typing one amount auto-fills the
// other at the pool's current ratio — matches what AddLiquidity itself
// requires (within 1% tolerance), so users don't have to calculate it
// by hand and then get rejected for a slightly-off ratio.
function updateLiquidityRatio(changed) {
  if (currentPoolAEQ <= 0 || currentPoolTUSD <= 0) return;
  const aeqInput = document.getElementById('addliq-aeq');
  const tusdInput = document.getElementById('addliq-tusd');
  if (changed === 'aeq') {
    const aeq = parseFloat(aeqInput.value || '0');
    if (aeq > 0) tusdInput.value = Math.floor(aeq * (currentPoolTUSD / currentPoolAEQ) * 1e6) / 1e6;
  } else {
    const tusd = parseFloat(tusdInput.value || '0');
    if (tusd > 0) aeqInput.value = Math.floor(tusd * (currentPoolAEQ / currentPoolTUSD) * 1e6) / 1e6;
  }
}

async function doAddLiquidity() {
  if (!swapWaddr) return;
  const amountAEQ = parseFloat(document.getElementById('addliq-aeq').value || '0');
  const amountTUSD = parseFloat(document.getElementById('addliq-tusd').value || '0');
  if (amountAEQ <= 0 || amountTUSD <= 0) { swapLog('Enter both AEQ and tUSD amounts', 'err'); return; }

  document.getElementById('swap-btn-addliq').disabled = true;
  try {
    const nonceRespL = await fetch('/api/nonce?wallet=' + swapWaddr);
    const { nonce: nonce_l } = await nonceRespL.json();
    const timestamp = Math.floor(Date.now() / 1000);
    const message = 'Aequitas Add Liquidity: ' + amountAEQ.toFixed(8) + ' AEQ + ' + amountTUSD.toFixed(8) + ' tUSD nonce:' + nonce_l + ' ts:' + timestamp;
    swapLog('Sign the message in MetaMask to confirm this deposit...', 'info');
    const signature = await signMessage(message);

    const resp = await fetch('/api/add-liquidity', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet: swapWaddr, amount_aeq: amountAEQ, amount_tusd: amountTUSD, nonce: nonce_l, timestamp, signature })
    });
    const data = await resp.json();
    if (data.success) {
      swapLog('✓ Liquidity added: ' + amountAEQ + ' AEQ + ' + amountTUSD + ' tUSD', 'ok');
      document.getElementById('addliq-aeq').value = '';
      document.getElementById('addliq-tusd').value = '';
      await refreshSwapBalances();
      await loadPoolStatus();
      await loadLPPosition();
    } else {
      swapLog('✗ Add liquidity failed: ' + sanitize(data.message), 'err');
    }
  } catch (e) {
    swapLog('✗ Error: ' + sanitize(e.message), 'err');
  }
  document.getElementById('swap-btn-addliq').disabled = false;
}

// ── LP POSITION / REMOVE LIQUIDITY ──────────────────────────────────────
let myLPShares = 0;
let myFullWithdrawableAEQ = 0;
let myFullWithdrawableTUSD = 0;

async function loadLPPosition() {
  if (!swapWaddr) return;
  try {
    const d = await (await fetch('/api/lp-position?wallet=' + swapWaddr)).json();
    myLPShares = d.shares || 0;
    myFullWithdrawableAEQ = d.withdrawable_aeq || 0;
    myFullWithdrawableTUSD = d.withdrawable_tusd || 0;
    const box = document.getElementById('lp-position-box');
    if (myLPShares > 0) {
      box.style.display = 'block';
      document.getElementById('lp-share-pct').textContent = d.pool_share_pct.toFixed(4) + '%';
      document.getElementById('lp-withdrawable').textContent =
        d.withdrawable_aeq.toFixed(4) + ' AEQ + ' + d.withdrawable_tusd.toFixed(4) + ' tUSD';
      updateRemovePreview();
    } else {
      box.style.display = 'none';
    }
  } catch (e) {}
}

// Recomputes "you will receive" from the currently selected removePct —
// called whenever removePct changes, whether from a percentage button or
// the manual input field, so both paths stay in sync with the same preview.
function updateRemovePreview() {
  var aeq = myFullWithdrawableAEQ * removePct;
  var tusd = myFullWithdrawableTUSD * removePct;
  var preview = aeq.toFixed(4) + ' AEQ + ' + tusd.toFixed(4) + ' tUSD';
  document.getElementById('lp-remove-preview').textContent = preview;
  // Also update the prominent inline preview
  var inline = document.getElementById('lp-remove-inline');
  if (inline) {
    inline.style.display = removePct > 0 ? 'block' : 'none';
    var aeqEl = document.getElementById('lp-inline-aeq');
    var tusdEl = document.getElementById('lp-inline-tusd');
    if (aeqEl) aeqEl.textContent = fmt(aeq);
    if (tusdEl) tusdEl.textContent = fmt(tusd);
  }
}

// Manual percentage input — lets someone type e.g. "37.5" instead of only
// having the 25/50/75/100 quick buttons. Clears the active button
// highlighting since a manual value generally won't match one exactly.
function setRemovePctManual(value) {
  const pct = parseFloat(value || '0');
  if (pct < 0 || pct > 100 || isNaN(pct)) return;
  removePct = pct / 100;
  document.querySelectorAll('#lp-position-box .pctbtn').forEach(b => { b.style.background = ''; b.style.color = ''; });
  updateRemovePreview();
}

// Stores the chosen withdrawal fraction (set by the 25/50/75/MAX buttons)
// so doRemoveLiquidity knows how many shares to burn without needing a
// raw share-count input field — most people think in "withdraw half my
// position", not in the underlying share units.
let removePct = 1;
function setRemovePct(pct, btn) {
  removePct = pct;
  document.querySelectorAll('#lp-position-box .pctbtn').forEach(b => { b.style.background = ''; b.style.color = ''; });
  if (btn) { btn.style.background = 'var(--gold)'; btn.style.color = '#050A14'; }
  document.getElementById('remove-pct-input').value = (pct * 100).toString();
  updateRemovePreview();
}

async function doRemoveLiquidity() {
  if (!swapWaddr || myLPShares <= 0) return;
  const sharesToBurn = myLPShares * removePct;

  document.getElementById('swap-btn-removeliq').disabled = true;
  try {
    const nonceRespR = await fetch('/api/nonce?wallet=' + swapWaddr);
    const { nonce: nonce_r } = await nonceRespR.json();
    const timestamp = Math.floor(Date.now() / 1000);
    const message = 'Aequitas Remove Liquidity: ' + sharesToBurn.toFixed(8) + ' shares nonce:' + nonce_r + ' ts:' + timestamp;
    swapLog('Sign the message in MetaMask to confirm this withdrawal...', 'info');
    const signature = await signMessage(message);

    const resp = await fetch('/api/remove-liquidity', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wallet: swapWaddr, shares: sharesToBurn, nonce: nonce_r, timestamp, signature })
    });
    const data = await resp.json();
    if (data.success) {
      swapLog('✓ Removed liquidity: received ' + data.amount_aeq.toFixed(4) + ' AEQ + ' + data.amount_tusd.toFixed(4) + ' tUSD', 'ok');
      await refreshSwapBalances();
      await loadPoolStatus();
      await loadLPPosition();
    } else {
      swapLog('✗ Remove liquidity failed: ' + sanitize(data.message), 'err');
    }
  } catch (e) {
    swapLog('✗ Error: ' + sanitize(e.message), 'err');
  }
  document.getElementById('swap-btn-removeliq').disabled = false;
}

function activateTabFromPath(path) {
  const tabNames = ['register','explorer','index','network','exchange'];
  const parts = (path || '').replace(/^\//, '').split('/');
  let name = parts[0];
  const stabSlug = parts[1] || '';
  // Backwards-compat: /swap -> /exchange
  if (name === 'swap') name = 'exchange';
  if (!name || !tabNames.includes(name)) name = 'register'; // default to register tab for root path
  let tabEl = null;
  document.querySelectorAll('.tab').forEach(t => {
    if ((t.getAttribute('data-args') || '').includes('"' + name + '"')) tabEl = t;
  });
  if (!tabEl) return;
  document.querySelectorAll('.tab-content').forEach(function(t) {
    t.classList.remove('active');
    t.style.display = ''; // clear any server-injected inline style
  });
  document.querySelectorAll('.tab').forEach(function(t) { t.classList.remove('active'); });
  const tabContent = document.getElementById('tab-' + name);
  if (!tabContent) return;
  tabContent.classList.add('active');
  tabEl.classList.add('active');
  // Activate stab-panel: use URL slug if present, otherwise first panel.
  // Must mirror showStab's stabSlugMap above (own FIX comment there) —
  // 'consensus'/'story' were missing for network, which is what made a
  // reload on those two sub-tabs fall through to the first panel instead.
  const stabMap = {
    explorer:  {blocks:'sep-blocks', humans:'sep-humans'},
    index:     {score:'eqi-score', distribution:'eqi-lorenz', economy:'eqi-economy', story:'eqi-story'},
    network:   {overview:'net-overview', consensus:'net-consensus', story:'net-story', node:'net-runnode', protocol:'net-protocol'},
    exchange:  {swap:'exch-swap', liquidity:'exch-liquidity'}
  };
  const panels = tabContent.querySelectorAll('.stab-panel');
  const stabs  = tabContent.querySelectorAll('.stab');
  if (panels.length) {
    panels.forEach(p => p.classList.remove('active'));
    stabs.forEach(s => s.classList.remove('active'));
    const targetId = stabSlug && stabMap[name] && stabMap[name][stabSlug];
    const targetEl = targetId ? document.getElementById(targetId) : panels[0];
    if (targetEl) targetEl.classList.add('active');
    // Activate matching stab button
    const stabBtn = targetId
      ? tabContent.querySelector(`.stab[data-args*='"${targetId}"']`)
      : stabs[0];
    if (stabBtn) stabBtn.classList.add('active');
    else if (stabs[0]) stabs[0].classList.add('active');
  }
  if (name === 'exchange') { loadPoolStatus(); preloadPriceHistory(); }
  if (name === 'index') {
    setTimeout(function() {
      const active = tabContent.querySelector('.stab-panel.active');
      if (!active) return;
      // FIX (2026-07-21): this only ever special-cased eqi-score and
      // eqi-economy — a direct load/reload landing on eqi-lorenz
      // (Distribution) via activateTabFromPath (as opposed to a click,
      // which goes through showStab and already handles it, line ~1961)
      // never called drawLorenzCurve, so the chart stayed on its "Need 2+
      // registered humans" placeholder forever regardless of actual human
      // count. Exactly the class of bug the Consensus/Story sub-tab
      // routing fix above this function was written to close — reload
      // landing directly on a sub-tab is the one path click-driven
      // handlers don't cover.
      if (active.id === 'eqi-score') { drawGiniHistoryChart(); drawLorenzCurve(); }
      else if (active.id === 'eqi-lorenz') drawLorenzCurve();
      else if (active.id === 'eqi-economy') drawWcapSlideChart();
    }, 50);
  }
}

// Activate the correct tab immediately — runs synchronously before first paint
// because this script tag is at the bottom of <body> (HTML already parsed).
activateTabFromPath(window.location.pathname);

document.addEventListener('DOMContentLoaded', function() {
  // Offer the browser extension when there IS one.
  //
  // FIX (2026-07-27, reported live): both "CONNECT METAMASK" buttons ship with
  // style="display:none" in explorer.html and nothing in this file ever
  // unhid them. Only the WalletConnect button was reachable, so a visitor with
  // the MetaMask extension installed and already connected to this very site
  // was still pushed into the QR-code flow to pair a phone. The extension was
  // never "not detected" — the path to it simply was not offered.
  //
  // window.ethereum is the injected provider every extension wallet exposes;
  // if it is absent (a plain browser, or mobile without an in-app browser)
  // nothing changes and WalletConnect remains the only option, which is
  // correct for those cases.
  if (window.ethereum) {
    ['btn-conn', 'swap-btn-conn'].forEach(function(id) {
      const b = document.getElementById(id);
      if (b) b.style.display = '';
    });
    // WalletConnect stays available — pairing a phone wallet is a legitimate
    // choice — but it no longer looks like the only one.
    ['btn-wc', 'swap-btn-wc'].forEach(function(id) {
      const b = document.getElementById(id);
      if (b) b.style.background = 'transparent', b.style.border = '1px solid rgba(59,153,252,0.5)', b.style.color = '#3b99fc';
    });
  }
  const amtInput = document.getElementById('swap-amount');
  if (amtInput) amtInput.addEventListener('input', updateFeeEstimate);
  const addliqAeq = document.getElementById('addliq-aeq');
  if (addliqAeq) addliqAeq.addEventListener('input', function() { updateLiquidityRatio('aeq'); });
  const addliqTusd = document.getElementById('addliq-tusd');
  if (addliqTusd) addliqTusd.addEventListener('input', function() { updateLiquidityRatio('tusd'); });
  const removePctInput = document.getElementById('remove-pct-input');
  if (removePctInput) removePctInput.addEventListener('input', function() { setRemovePctManual(this.value); });
  const langSel = document.getElementById('lang-sel');
  if (langSel) langSel.addEventListener('change', function() { setLang(this.value); });
  // IMMER anwenden, auch fuer Englisch -- und das ist der Kern.
  //
  // Vorher lief setLang() nur, wenn schon eine Sprache gespeichert war. Ein
  // Erstbesucher bekam also nie das Woerterbuch zu sehen, sondern den rohen
  // Text aus dem HTML. Der ist eine ZWEITE Kopie jeder Zeichenkette, und
  // Kopien driften: am 26.08.2026 gemessen wichen 138 von 462 Stellen vom
  // gepflegten englischen Woerterbuch ab.
  //
  // Das war nicht kosmetisch. Die Abweichungen stammten fast alle aus der
  // alten, geraetegebundenen Beschreibung und sagten dem Besucher das
  // Gegenteil dessen, was geschieht:
  //
  //   "One identity per device, not yet body-bound"  -> tatsaechlich
  //                                                     gesichtsgebunden
  //   "Once per device"                              -> einmal pro MENSCH
  //   "Data never leaves device"                     -> Gesichtspruefung per
  //                                                     Quorum, Bilder danach
  //                                                     verworfen
  //
  // Jemandem zu sagen, sein Koerper werde nicht geprueft, waehrend eine
  // Gesichtspruefung zwingend ist (BIO_ATTESTATION_MODE=required auf beiden
  // Proof-Servern), ist keine veraltete Marketingzeile, sondern eine falsche
  // Grundlage fuer seine Einwilligung.
  //
  // Das Woerterbuch ist die gepflegte Quelle -- dort landet jede Uebersetzung
  // und jede Korrektur. Es unbedingt anzuwenden macht die Drift wirkungslos,
  // statt sie einmalig hinterherzuraeumen.
  //
  // Gefahrlos: genau dieser Pfad laeuft heute schon bei jedem wiederkehrenden
  // Besucher, und setLang() zieht dynamische Werte danach selbst wieder nach
  // (applyBlockTime, applyRpcUrl).
  var storedLang = null;
  try { storedLang = localStorage.getItem(LANG_KEY); } catch (e) { /* private mode */ }
  setLang(storedLang && T[storedLang] ? storedLang : 'en');
  // Mirror whatever the markup shipped as active before anyone clicks.
  syncActiveAria();
  const expSearchInput = document.getElementById('exp-search-input');
  if (expSearchInput) expSearchInput.addEventListener('keydown', function(ev) { if (ev.key === 'Enter') expSearch(); });
  // Resolve __RPC__ once at load. setLang() also calls this, but setLang only
  // runs when the user CHANGES language — a visitor who never touches the
  // selector would otherwise read the literal token "__RPC__" in the docs
  // card. (__BT__ has no equivalent line because its value genuinely isn't
  // known until the first /api/status response arrives; the RPC URL is known
  // immediately.)
  applyRpcUrl();
});

// Back/forward navigation: restore the tab that matches the URL
window.addEventListener('popstate', () => activateTabFromPath(window.location.pathname));

// ── DELEGATED CLICK HANDLING ─────────────────────────────────────────────────
// FIX (Monster Audit 2026-07-12, P2): CSP hardening removes 'unsafe-inline'
// from script-src, so none of this page's onclick="..." attributes can
// execute anymore (inline event handler attributes are governed by
// script-src same as <script> tags). Every click target now carries
// data-act (+ optional data-args, a JSON array) instead; this single
// delegated listener resolves data-act through an explicit allow-list
// (CLICK_ACTIONS) rather than a dynamic window[name] lookup, so a data-act
// value can never be used to invoke an arbitrary global. The clicked
// element and the raw event are appended after the JSON args on every call
// — most handlers ignore the extras, a few (showTab, showStab, copyAddr,
// setRemovePct, closeBlockOnBackdropClick) rely on the element exactly as
// the old onclick="...,this" pattern did.
function showTabAndPriceChart(name, el) { showTab(name, el); setTimeout(drawPriceChart, 50); }
function showStabAndPriceChart(parentId, stabId, el) { showStab(parentId, stabId, el); setTimeout(drawPriceChart, 50); }
function showStabAndLorenzChart(parentId, stabId, el) { showStab(parentId, stabId, el); setTimeout(drawLorenzCurve, 60); }
function closeBlockOnBackdropClick(el, ev) { if (ev.target === el) closeBlock(); }

const CLICK_ACTIONS = {
  showTab: showTab,
  showTabAndPriceChart: showTabAndPriceChart,
  showStab: showStab,
  showStabAndPriceChart: showStabAndPriceChart,
  showStabAndLorenzChart: showStabAndLorenzChart,
  goTab: goTab,
  copyAddr: copyAddr,
  connectWallet: connectWallet,
  connectWalletConnect: connectWalletConnect,
  disconnectWallet: disconnectWallet,
  doRegister: doRegister,
  doRecoverEscrow: doRecoverEscrow,
  doSetGuardian: doSetGuardian,
  doGuardianConfirmAlive: doGuardianConfirmAlive,
  expSearch: expSearch,
  openBlock: openBlock,
  closeBlock: closeBlock,
  closeBlockOnBackdropClick: closeBlockOnBackdropClick,
  setChartInterval: setChartInterval,
  setChartRange: setChartRange,
  reverseSwapDir: reverseSwapDir,
  setSwapPct: setSwapPct,
  connectSwapWallet: connectSwapWallet,
  doSwap: doSwap,
  claimFaucet: claimFaucet,
  setPctAmount: setPctAmount,
  doAddLiquidity: doAddLiquidity,
  setRemovePct: setRemovePct,
  doRemoveLiquidity: doRemoveLiquidity,
  addToMetaMask: addToMetaMask,
  registerValidatorKey: registerValidatorKey,
};

// Activation is deliberately not click-only. 28 of the elements carrying
// data-act are <div>s — every tab in the section bar, every sub-tab, the
// explorer's shortcut cards — and a div takes neither focus nor Enter on its
// own. Until this handler existed, a visitor navigating by keyboard could
// reach the page and then go nowhere: the entire section bar was unreachable.
// The markup gives those divs tabindex/role; this gives them the key.
function dispatchAction(ev, el) {
  const fn = CLICK_ACTIONS[el.getAttribute('data-act')];
  if (typeof fn !== 'function') return;
  let args = [];
  const raw = el.getAttribute('data-args');
  if (raw) {
    try { args = JSON.parse(raw); } catch (e) { args = []; }
  }
  fn.apply(null, args.concat([el, ev]));
}

document.addEventListener('click', function (ev) {
  const el = ev.target.closest('[data-act]');
  if (el) dispatchAction(ev, el);
});

document.addEventListener('keydown', function (ev) {
  if (ev.key !== 'Enter' && ev.key !== ' ' && ev.key !== 'Spacebar') return;
  const el = ev.target.closest('[data-act]');
  if (!el) return;
  // Native controls already do this themselves; intercepting them would
  // double-fire the action and, on <button>, swallow the browser's own
  // handling.
  const tag = el.tagName;
  if (tag === 'BUTTON' || tag === 'A' || tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') return;
  // Space scrolls the page by default, which is exactly what must not happen
  // when the thing under the cursor is being activated.
  ev.preventDefault();
  dispatchAction(ev, el);
});

// The .active class is the single source of truth for which section is
// showing; this mirrors it into the accessibility tree. These elements change
// the URL via pushState, so aria-current="page" describes them accurately —
// they navigate, they do not merely toggle.
function syncActiveAria() {
  document.querySelectorAll('.tab[data-act], .stab[data-act]').forEach(function (el) {
    if (el.classList.contains('active')) el.setAttribute('aria-current', 'page');
    else el.removeAttribute('aria-current');
  });
}

// HTML-attribute-safe escaping for values spliced into data-args="..." when
// building table rows via innerHTML (block hashes are validator-controlled,
// not raw user input, but this is the same defense-in-depth this whole CSP
// pass is about — don't rely on "it's not user input today" staying true).
function attrEscape(s) {
  return String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function checkProofParams() {
  const p = new URLSearchParams(window.location.search);
  const proofId = p.get('proofId');
  const proof = p.get('proof');
  // FIX (Monster Audit 2026-07-12, P1): this function used to also accept a
  // raw ?bioHash=<value> query param (a hand-off from the now-retired
  // AequitasBio app's MetaMask deep link) and feed it straight into the
  // wallet-connect-then-prove flow. A raw bioHash in a URL lands in browser
  // history, the MetaMask dapp-link, reverse-proxy logs, and referrers —
  // exactly the kind of durable identity-linkable leak this endpoint's own
  // POST-only /api/check-registration-by-biohash handler was already hardened
  // against (see that handler's comment). No current client constructs this
  // link (AequitasBio is retired; aequitas-app registers fully in-app via
  // /api/prove, no browser hop at all; aequitas-dapp.html never built this
  // link either) — removed rather than kept as unused attack surface.
  if (proofId) {
    if (!/^[a-zA-Z0-9_-]{1,64}$/.test(proofId)) {
      console.warn('Invalid proof ID format');
      return;
    }
    fetch('/api/prove/get/' + proofId).then(r => r.json()).then(pd => {
      proofData = pd;
      document.getElementById('pbox').style.display = 'block';
      document.getElementById('pval').textContent = 'Proof ID: ' + proofId + ' — Connect wallet to register';
      document.querySelectorAll('.tab')[0].click();
      setTimeout(() => connectWallet(), 600);
    }).catch(e => console.error(e));
  } else if (proof) {
    if (proof.length > 10000) {
      console.warn('Proof param too large');
      return;
    }
    try {
      proofData = JSON.parse(decodeURIComponent(proof));
      document.getElementById('pbox').style.display = 'block';
      document.getElementById('pval').textContent = 'Proof received — Connect wallet to register';
      document.querySelectorAll('.tab')[0].click();
      setTimeout(() => connectWallet(), 600);
    } catch (e) {}
  }
}

async function connectWallet() {
  if (!window.ethereum) {
    const _isMobW = /iPhone|iPad|iPod|Android/i.test(navigator.userAgent);
    if (_isMobW) { const _dl = 'https://metamask.app.link/dapp/' + window.location.host; addLog('🦊 Mobile: <a href="' + _dl + '" style="color:var(--gold)">In MetaMask App öffnen</a>', 'warn', true); } else { addLog('🦊 MetaMask not found — <a href="https://metamask.io/download/" target="_blank" style="color:var(--gold)">install MetaMask</a>', 'warn', true); }
    return;
  }
  try {
    await addToMetaMask();
    const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
    waddr = accounts[0];
    swapWaddr = waddr;
    localStorage.setItem('aeq_wallet', waddr);
    document.getElementById('wbox').style.display = 'block';
    document.getElementById('wadr').textContent = waddr;
    const btn = document.getElementById('btn-conn');
    btn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    const dBtn = document.getElementById('btn-disconnect');
    if (dBtn) dBtn.style.display = 'block';
    const wcBtn = document.getElementById('btn-wc');
    if (wcBtn) wcBtn.style.display = 'none';
    // Sync swap tab wallet display
    const swapBox = document.getElementById('swap-wbox');
    const swapAdr = document.getElementById('swap-wadr');
    const swapBtn = document.getElementById('swap-btn-conn');
    const swapDBtn = document.getElementById('swap-btn-disconnect');
    const swapWcBtn = document.getElementById('swap-btn-wc');
    if (swapBox) swapBox.style.display = 'block';
    if (swapAdr) swapAdr.textContent = waddr;
    if (swapBtn) { swapBtn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4); swapBtn.style.background = 'var(--green)'; swapBtn.style.color = '#050A14'; }
    if (swapDBtn) swapDBtn.style.display = 'block';
    if (swapWcBtn) swapWcBtn.style.display = 'none';
    try {
      const br = await fetch('/api/balance?wallet=' + waddr);
      const bd = await br.json();
      if (bd.is_human) {
        addLog('Already registered! Balance: ' + sanitize(String(bd.balance || 0)) + ' AEQ', 'ok');
        document.getElementById('btn-reg').disabled = true;
        document.getElementById('btn-reg').textContent = 'ALREADY REGISTERED';
      } else if (proofData) {
        document.getElementById('btn-reg').disabled = false;
        document.getElementById('btn-reg').textContent = 'PROOF READY — CLICK TO REGISTER';
      } else {
        document.getElementById('btn-reg').disabled = true;
      }
    } catch (e) {
      document.getElementById('btn-reg').disabled = !proofData;
    }
  } catch (e) {
    addLog('Connection failed: ' + sanitize(e.message), 'err');
  }
}

function copyAddr(id, btn) {
  const addr = document.getElementById(id).textContent;
  if (!addr || addr === '—') return;
  navigator.clipboard.writeText(addr).then(() => {
    const orig = btn.textContent;
    btn.textContent = '✓ Copied';
    setTimeout(() => { btn.textContent = orig; }, 1500);
  });
}

function addLog(msg, type, allowHTML) {
  const el = document.getElementById('rlog');
  if (!el) return;
  const row = document.createElement('div');
  const span = document.createElement('span');
  span.className = (type || 'info');
  if (allowHTML) {
    // only for explicit HTML content (e.g. MetaMask deep-links) — never pass server messages here
    span.innerHTML = msg;
  } else {
    span.textContent = msg; // default: treat as plain text
  }
  row.appendChild(span);
  el.appendChild(row);
  el.scrollTop = el.scrollHeight;
}

// Logs to both tabs' log boxes — connectWalletConnect() below can be
// triggered from either the Register or the Swap page, and whichever one
// isn't currently active still has its log box in the DOM (SPA tabs are
// hidden, not removed), just not visible to the user right now.
function walletLog(msg, type, allowHTML) {
  addLog(msg, type, allowHTML);
  swapLog(msg, type, allowHTML);
}

// Lazily creates (once) and returns the WalletConnect EthereumProvider —
// same EIP-1193 request()/on() surface as window.ethereum, so the existing
// personal_sign call sites just need a provider reference, not a rewrite.
// optionalChains (not chains) is used for the Aequitas chain id: making it
// a hard-required namespace would make many general-purpose mobile wallets
// reject the pairing outright since they don't pre-recognize chain 1926.
async function getWalletConnectProvider() {
  if (wcProvider) return wcProvider;
  if (!window.WalletConnectEthereumProvider) {
    throw new Error('WalletConnect script failed to load — check your connection and reload the page.');
  }
  wcProvider = await window.WalletConnectEthereumProvider.init({
    projectId: WC_PROJECT_ID,
    optionalChains: [1926],
    // FIX (audit 2026-08-16): same fix as addToMetaMask above — see its
    // comment. window.location.origin is the node this page is actually
    // talking to, which is what WalletConnect should read chain state from
    // too.
    rpcMap: { 1926: window.location.origin + '/rpc' },
    showQrModal: true,
    metadata: {
      name: 'Aequitas',
      description: 'Proof of Humanity Chain',
      url: window.location.origin,
      icons: []
    }
  });
  wcProvider.on('accountsChanged', handleAccountsChanged);
  wcProvider.on('chainChanged', function() { window.location.reload(); });
  wcProvider.on('disconnect', function() { disconnectWallet(); });
  return wcProvider;
}

// WalletConnect counterpart to connectWallet()/connectSwapWallet(): shown on
// both the Register and Swap pages as an alternative to the MetaMask
// extension (mobile wallets, hardware wallets via a companion app, etc).
// Unlike those two, this single handler always syncs both tabs' UI and swap
// state, since either button can be the very first thing the user clicks.
async function connectWalletConnect() {
  try {
    const provider = await getWalletConnectProvider();
    const accounts = await provider.enable();
    if (!accounts || !accounts[0]) throw new Error('No account returned by wallet');
    waddr = accounts[0];
    swapWaddr = waddr;
    localStorage.setItem('aeq_wallet', waddr);
    localStorage.setItem('aeq_wallet_provider', 'walletconnect');

    document.getElementById('wbox').style.display = 'block';
    document.getElementById('wadr').textContent = waddr;
    const btn = document.getElementById('btn-conn');
    btn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4);
    btn.style.background = 'var(--green)';
    btn.style.color = '#050A14';
    const dBtn = document.getElementById('btn-disconnect');
    if (dBtn) dBtn.style.display = 'block';
    const wcBtn = document.getElementById('btn-wc');
    if (wcBtn) wcBtn.style.display = 'none';

    const swapBox = document.getElementById('swap-wbox');
    const swapAdr = document.getElementById('swap-wadr');
    const swapBtn = document.getElementById('swap-btn-conn');
    const swapDBtn = document.getElementById('swap-btn-disconnect');
    const swapWcBtn = document.getElementById('swap-btn-wc');
    if (swapBox) swapBox.style.display = 'block';
    if (swapAdr) swapAdr.textContent = waddr;
    if (swapBtn) { swapBtn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4); swapBtn.style.background = 'var(--green)'; swapBtn.style.color = '#050A14'; }
    if (swapDBtn) swapDBtn.style.display = 'block';
    if (swapWcBtn) swapWcBtn.style.display = 'none';

    try {
      const br = await fetch('/api/balance?wallet=' + waddr);
      const bd = await br.json();
      if (bd.is_human) {
        walletLog('Already registered! Balance: ' + sanitize(String(bd.balance || 0)) + ' AEQ', 'ok');
        document.getElementById('btn-reg').disabled = true;
        document.getElementById('btn-reg').textContent = 'ALREADY REGISTERED';
        loadGuardianStatus();
      } else if (proofData) {
        document.getElementById('btn-reg').disabled = false;
        document.getElementById('btn-reg').textContent = 'PROOF READY — CLICK TO REGISTER';
      } else {
        document.getElementById('btn-reg').disabled = true;
      }
    } catch (e) {
      document.getElementById('btn-reg').disabled = !proofData;
    }

    await refreshSwapBalances();
    await loadLPPosition();
    document.getElementById('swap-btn-go').disabled = false;
    document.getElementById('swap-btn-faucet').disabled = false;
    document.getElementById('swap-btn-addliq').disabled = false;
    setSwapDirection('aeq_to_tusd');
  } catch (e) {
    // Closing the QR modal, or clicking Connect a second time while a pairing
    // is still pending, makes the library reject the FIRST (now-aborted)
    // promise with "Connection request reset"/"Proposal expired"/a user-
    // rejection — none of these are real failures the user needs a red error
    // for; they just cancelled or restarted. Show those as a neutral hint and
    // drop the stale provider so the next click starts a clean pairing,
    // instead of the alarming "connection failed" the raw message produced.
    const msg = (e && e.message ? e.message : String(e)) || '';
    const benign = /reset|expired|rejected|closed|cancell?ed|user (?:denied|declined)|modal/i.test(msg);
    if (benign) {
      wcProvider = null; // force a fresh EthereumProvider.init() on retry
      walletLog('WalletConnect cancelled — tap CONNECT WALLETCONNECT again to retry.', 'info');
    } else {
      walletLog('WalletConnect connection failed: ' + sanitize(msg || 'unknown error'), 'err');
    }
  }
}

async function doRegister() {
  if (!waddr || !proofData) return;
  try {
    addLog('Preparing signature...', 'info');
    document.getElementById('btn-reg').disabled = true;

    // commitment is pubSignals[0] — must match exactly what the contract reads
    const commitment = proofData.pubSignals[0];

    // Nullifier must be the ZK-circuit-derived value (pubSignals[1] from the
    // v2 circuit) — it is the only value cryptographically attested by the
    // proof. The chain contract (v7.6+) hard-rejects any registration whose
    // nullifier isn't this ZK-bound value, so a client-side SHA256-derived
    // "v1" fallback would always fail server-side anyway; it used to exist
    // here and silently waste a MetaMask signature round-trip before failing.
    if (!proofData.zkNullifier) {
      addLog('Error: proof server did not return a ZK-bound nullifier (circuit v3 is required) — try generating the proof again', 'err');
      document.getElementById('btn-reg').disabled = false;
      return;
    }
    // FIX (fresh Monster Audit 2026-07-12, P1-5): register.go hard-requires
    // circuitVersion === 3 (v7.6+, ZK-bound nullifier only). This used to
    // fall back to `proofData.circuitVersion || 2` when the field was
    // missing/falsy and send that — 2 is not 3, so the backend would always
    // reject it anyway, just later and less clearly (after the MetaMask
    // signature round-trip below) than catching it here alongside the
    // zkNullifier check above.
    if (proofData.circuitVersion !== 3) {
      addLog('Error: proof server returned circuit v' + (proofData.circuitVersion ?? 'unknown') + ', but v3 is required — try generating the proof again', 'err');
      document.getElementById('btn-reg').disabled = false;
      return;
    }
    const zkN = BigInt(proofData.zkNullifier);
    const nullifier = zkN.toString(16).padStart(64, '0');
    addLog('Using ZK-bound nullifier (circuit v3)', 'info');

    // Build the EXACT same hash the contract computes:
    // keccak256(abi.encodePacked(block.chainid, address(this), "register", commitment, nullifier))
    const messageHash = ethers.solidityPackedKeccak256(
      ['uint256', 'address', 'string', 'uint256', 'bytes32'],
      [1926, V7_CONTRACT, 'register', commitment, '0x' + nullifier]
    );

    addLog('Please sign the message in your wallet to prove this wallet is yours (no gas, no cost)...', 'info');
    // personal_sign automatically adds the "\x19Ethereum Signed Message:\n32" prefix
    const signature = await activeProvider().request({
      method: 'personal_sign',
      params: [messageHash, waddr]
    });

    addLog('Registering on Aequitas V7...', 'info');
    const r = await fetch('/api/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        wallet: waddr,
        pA: proofData.pA, pB: proofData.pB, pC: proofData.pC, pubSignals: proofData.pubSignals,
        signature: signature,
        bioHash: '',
        bioHashKey: proofData.bioHashKey || '',
        nullifier: nullifier,
        circuitVersion: proofData.circuitVersion,
        zkNullifier: proofData.zkNullifier || null
      })
    });
    const d = await r.json();
    if (!d.success) { addLog('Error: ' + sanitize(d.message || ''), 'err'); document.getElementById('btn-reg').disabled = false; return; }
    addLog('Registered! ' + sanitize(d.message || ''), 'ok');
    loadGuardianStatus();
    setTimeout(() => { window.location.href = '/registered?wallet=' + waddr; }, 1500);
  } catch (e) { addLog('Error: ' + sanitize(e.message), 'err'); document.getElementById('btn-reg').disabled = false; }
}

// FIX (P2, beta-launch audit 2026-07-05): this handler used to update only
// waddr and the Register tab. swapWaddr (the address every swap/
// liquidity/faucet action in the Exchange tab actually signs and submits
// with — see signMessage()) was never reassigned, so switching the active
// account directly in MetaMask's own UI left the two tabs pointing at two
// different wallets with no visible indication anything had changed. It
// also silently did nothing when every account was disconnected (a === []),
// leaving stale "connected" UI displayed indefinitely. Now mirrors the same
// dual-tab sync connectWallet()/connectSwapWallet() already do, and fully
// resets via disconnectWallet() when nothing is connected anymore.
// Named (not inline) so both window.ethereum and the WalletConnect provider
// (see getWalletConnectProvider()) can register the same handler.
function handleAccountsChanged(a) {
  const newAddr = a[0] || '';
  if (!newAddr) { disconnectWallet(); return; }
  waddr = newAddr;
  swapWaddr = newAddr;
  localStorage.setItem('aeq_wallet', newAddr);

  document.getElementById('wbox').style.display = 'block';
  document.getElementById('wadr').textContent = waddr;
  const btn = document.getElementById('btn-conn');
  btn.textContent = waddr.slice(0, 10) + '...' + waddr.slice(-4);
  btn.style.background = 'var(--green)';
  btn.style.color = '#050A14';
  const dBtn = document.getElementById('btn-disconnect');
  if (dBtn) dBtn.style.display = 'block';
  const wcBtn = document.getElementById('btn-wc');
  if (wcBtn) wcBtn.style.display = 'none';

  const swapBox = document.getElementById('swap-wbox');
  const swapAdr = document.getElementById('swap-wadr');
  const swapBtn = document.getElementById('swap-btn-conn');
  const swapDBtn = document.getElementById('swap-btn-disconnect');
  const swapWcBtn = document.getElementById('swap-btn-wc');
  if (swapBox) swapBox.style.display = 'block';
  if (swapAdr) swapAdr.textContent = swapWaddr;
  if (swapBtn) { swapBtn.textContent = swapWaddr.slice(0, 10) + '...' + swapWaddr.slice(-4); swapBtn.style.background = 'var(--green)'; swapBtn.style.color = '#050A14'; }
  if (swapDBtn) swapDBtn.style.display = 'block';
  if (swapWcBtn) swapWcBtn.style.display = 'none';
  if (typeof refreshSwapBalances === 'function') refreshSwapBalances().catch(function() {});
  if (typeof loadLPPosition === 'function') loadLPPosition().catch(function() {});

  fetch('/api/balance?wallet=' + waddr).then(function(r) { return r.json(); }).then(function(bd) {
    if (bd.is_human) {
      document.getElementById('btn-reg').disabled = true;
      document.getElementById('btn-reg').textContent = 'ALREADY REGISTERED';
      addLog('Already registered! Balance: ' + sanitize(String(bd.balance || 0)) + ' AEQ', 'ok');
      loadGuardianStatus();
    } else {
      document.getElementById('btn-reg').disabled = !proofData;
      if (proofData) document.getElementById('btn-reg').textContent = 'PROOF READY — CLICK TO REGISTER';
    }
  }).catch(function() { document.getElementById('btn-reg').disabled = !proofData; });
}
window.ethereum && window.ethereum.on('accountsChanged', handleAccountsChanged);

// FIX (P2, beta-launch audit 2026-07-05): no chainChanged listener existed
// at all — a user connected while MetaMask was on any other network saw
// every part of the UI behave as if connected normally (no warning, no
// forced switch), then hit confusing failures deep into a registration or
// swap flow instead of a clear, immediate explanation. Reloading on
// chainChanged is MetaMask's own documented recommendation: it guarantees
// every piece of page state (wallet address, balances, chain-dependent
// constants) is re-derived fresh against whatever network is now active,
// rather than trying to patch each of them in place.
window.ethereum && window.ethereum.on('chainChanged', function() {
  window.location.reload();
});

function disconnectWallet() {
  const wasWalletConnect = localStorage.getItem('aeq_wallet_provider') === 'walletconnect';
  waddr = '';
  swapWaddr = '';
  localStorage.removeItem('aeq_wallet');
  localStorage.removeItem('aeq_wallet_provider');
  // Reset register tab
  const wbox = document.getElementById('wbox');
  const wadr = document.getElementById('wadr');
  const bConn = document.getElementById('btn-conn');
  const bDisc = document.getElementById('btn-disconnect');
  const bReg = document.getElementById('btn-reg');
  const bWc = document.getElementById('btn-wc');
  if (wbox) wbox.style.display = 'none';
  if (wadr) wadr.textContent = '—';
  if (bConn) { bConn.textContent = '🦊 CONNECT METAMASK'; bConn.style.background = ''; bConn.style.color = ''; }
  if (bDisc) bDisc.style.display = 'none';
  if (bReg) { bReg.disabled = true; bReg.textContent = 'REGISTER ON-CHAIN'; }
  if (bWc) bWc.style.display = '';
  // Reset swap tab
  const swapBox = document.getElementById('swap-wbox');
  const swapAdr = document.getElementById('swap-wadr');
  const swapConn = document.getElementById('swap-btn-conn');
  const swapDisc = document.getElementById('swap-btn-disconnect');
  const swapGo = document.getElementById('swap-btn-go');
  const swapWc = document.getElementById('swap-btn-wc');
  if (swapBox) swapBox.style.display = 'none';
  if (swapAdr) swapAdr.textContent = '—';
  if (swapConn) { swapConn.textContent = '🦊 CONNECT METAMASK'; swapConn.style.background = ''; swapConn.style.color = ''; }
  if (swapDisc) swapDisc.style.display = 'none';
  if (swapGo) swapGo.disabled = true;
  if (swapWc) swapWc.style.display = '';
  if (wasWalletConnect && wcProvider) {
    wcProvider.disconnect().catch(function() {});
    wcProvider = null;
  }
  addLog(wasWalletConnect ? '✓ Wallet disconnected.' : '✓ Wallet disconnected locally. To fully revoke, open MetaMask → Connected Sites.', 'info');
}

// Shared by both restore paths below (MetaMask's silent eth_accounts check
// and WalletConnect's persisted-session check) — applies an already-known,
// already-authorized address to both tabs' UI without prompting the user.
async function applyRestoredWallet(addr) {
  waddr = addr;
  swapWaddr = addr;
  // Restore register tab UI
  const wbox = document.getElementById('wbox');
  const wadr = document.getElementById('wadr');
  const bConn = document.getElementById('btn-conn');
  const bDisc = document.getElementById('btn-disconnect');
  if (wbox) wbox.style.display = 'block';
  if (wadr) { wadr.textContent = addr; wadr.title = addr; }
  if (bConn) { bConn.textContent = addr.slice(0,10)+'...'+addr.slice(-4); bConn.style.background='var(--green)'; bConn.style.color='#050A14'; }
  if (bDisc) bDisc.style.display = 'block';
  // Restore swap tab UI
  const swapBox = document.getElementById('swap-wbox');
  const swapAdr = document.getElementById('swap-wadr');
  const swapConn = document.getElementById('swap-btn-conn');
  const swapDBtn = document.getElementById('swap-btn-disconnect');
  if (swapBox) swapBox.style.display = 'block';
  if (swapAdr) { swapAdr.textContent = addr; swapAdr.title = addr; }
  if (swapConn) { swapConn.textContent = addr.slice(0,10)+'...'+addr.slice(-4); swapConn.style.background='var(--green)'; swapConn.style.color='#050A14'; }
  if (swapDBtn) swapDBtn.style.display = 'block';
  const goBtn = document.getElementById('swap-btn-go');
  const faucetBtn = document.getElementById('swap-btn-faucet');
  const addliqBtn = document.getElementById('swap-btn-addliq');
  if (goBtn) goBtn.disabled = false;
  if (faucetBtn) faucetBtn.disabled = false;
  if (addliqBtn) addliqBtn.disabled = false;
  setSwapDirection('aeq_to_tusd');
  refreshSwapBalances();
  loadLPPosition();
  // Check registration status silently — no popup
  try {
    const br = await fetch('/api/balance?wallet=' + addr);
    const bd = await br.json();
    if (bd.is_human) {
      const bReg = document.getElementById('btn-reg');
      if (bReg) { bReg.disabled = true; bReg.textContent = 'ALREADY REGISTERED ✓'; }
      addLog('✓ Wallet restored. Balance: ' + (bd.balance || 0).toFixed(4) + ' AEQ · Already registered.', 'ok');
      loadGuardianStatus();
    }
  } catch(_) {}
}

async function restoreWalletFromStorage() {
  const saved = localStorage.getItem('aeq_wallet');
  if (!saved) return;
  if (localStorage.getItem('aeq_wallet_provider') === 'walletconnect') {
    try {
      const provider = await getWalletConnectProvider();
      const restored = provider.session && provider.accounts && provider.accounts[0];
      if (restored && provider.accounts[0].toLowerCase() === saved.toLowerCase()) {
        await applyRestoredWallet(provider.accounts[0]);
        const wcBtn = document.getElementById('btn-wc');
        if (wcBtn) wcBtn.style.display = 'none';
        const swapWcBtn = document.getElementById('swap-btn-wc');
        if (swapWcBtn) swapWcBtn.style.display = 'none';
      } else {
        localStorage.removeItem('aeq_wallet');
        localStorage.removeItem('aeq_wallet_provider');
      }
    } catch (e) {}
    return;
  }
  if (!window.ethereum) return;
  try {
    const accounts = await window.ethereum.request({ method: 'eth_accounts' });
    if (accounts && accounts[0] && accounts[0].toLowerCase() === saved.toLowerCase()) {
      await applyRestoredWallet(accounts[0]);
    } else {
      localStorage.removeItem('aeq_wallet');
    }
  } catch(e) {}
}

checkProofParams();
restoreWalletFromStorage();
loadValidatorLabels();
loadStatus();
loadHealth();
loadBlocks();
loadHumans();
loadTopology();
// FIX (Monster Audit 2026-07-12, P3): these 7 intervals used to poll
// unconditionally, including in background/minimized tabs — a dashboard
// left open overnight in an unfocused tab was hitting the server exactly
// as hard as one someone was actively watching. pollWhenVisible() skips
// the fetch entirely while the tab is hidden (Page Visibility API); the
// visibilitychange listener below does one immediate refresh of
// everything on return so data isn't stale for up to a full interval
// period after re-focusing.
function pollWhenVisible(fn) {
  return function() { if (!document.hidden) fn(); };
}
// FIX (fresh Monster Audit 2026-07-12, P2): loadHumans (tab-explorer's
// h-count/humans-list) and loadTopology (tab-network) were polling every
// 10s/60s even while some OTHER tab was active — pollWhenVisible only
// covers the whole-browser-tab-hidden case, not "this in-page tab isn't
// the one showing this data." The default landing tab is tab-register,
// which shows neither, so a visitor who never clicks Explorer/Network
// still generated the full steady-state polling load for both. Only
// gating the RECURRING poll here, not the one-shot calls a few lines up —
// those still fire unconditionally on page load so every tab's data is
// already populated (not blank) the moment someone switches to it.
function pollWhenTabActive(tabContentId, fn) {
  return function() {
    if (!document.hidden && document.getElementById(tabContentId).classList.contains('active')) fn();
  };
}
setInterval(pollWhenVisible(loadStatus), 6000);
setInterval(pollWhenVisible(loadHealth), 30000);
setInterval(pollWhenVisible(loadBlocks), 6000);
setInterval(pollWhenTabActive('tab-explorer', loadHumans), 10000);
setInterval(pollWhenVisible(loadValidatorLabels), 60000);
setInterval(pollWhenVisible(loadPoolStatus), 8000);
setInterval(pollWhenTabActive('tab-network', loadTopology), 60000);
document.addEventListener('visibilitychange', function() {
  if (!document.hidden) {
    loadStatus(); loadHealth(); loadBlocks(); loadHumans();
    loadValidatorLabels(); loadPoolStatus(); loadTopology();
  }
});

// Live push (scaling roadmap 2026-07-21): /api/events (SSE) wakes these same
// refreshers the instant a new block lands, instead of waiting for the next
// setInterval tick above — real block-to-screen latency drops from "up to
// 6s" to "as fast as the network round trip". Deliberately NOT a replacement
// for the polling above, which stays exactly as-is: EventSource reconnects
// on its own after a drop, but if it's ever unavailable (proxy strips SSE,
// older browser, transient failure) the existing interval polling still
// covers everything on its own, unaffected — this is a latency improvement
// layered on top of an already-correct fallback, not a new single point of
// failure.
(function() {
  if (typeof EventSource === 'undefined') return;
  let es = null;
  let reconnectDelay = 2000;
  function connect() {
    try { es = new EventSource('/api/events'); } catch (e) { return; }
    es.addEventListener('block', function() {
      if (document.hidden) return; // hidden-tab refreshers already skip via pollWhenVisible; skip the SSE-triggered call too
      reconnectDelay = 2000; // a working message resets backoff
      loadStatus(); loadBlocks();
      if (document.getElementById('tab-explorer').classList.contains('active')) loadHumans();
      loadPoolStatus();
    });
    es.onerror = function() {
      // EventSource retries on its own for ordinary drops; only tear down
      // and back off manually if the browser gave up (readyState CLOSED),
      // e.g. after repeated failures or a server that doesn't support SSE.
      if (es.readyState === EventSource.CLOSED) {
        es.close();
        setTimeout(connect, reconnectDelay);
        reconnectDelay = Math.min(reconnectDelay * 2, 30000);
      }
    };
  }
  connect();
})();

// Observe each canvas individually so charts redraw when they become visible.
// We observe the canvas containers, not document.body (which fires on every
// DOM change and would cause constant redraws killing performance).
(function() {
  if (typeof ResizeObserver === 'undefined') return;
  function observeCanvas(canvasId, drawFn) {
    var canvas = document.getElementById(canvasId);
    if (!canvas) return;
    var ro = new ResizeObserver(function(entries) {
      for (var e of entries) {
        if (e.contentRect.width > 0) drawFn();
      }
    });
    ro.observe(canvas);
  }
  observeCanvas('gini-history-chart', drawGiniHistoryChart);
  observeCanvas('lorenz-chart', drawLorenzCurve);
  observeCanvas('price-chart', drawPriceChart);
})();

async function registerValidatorKey() {
  var statusEl = document.getElementById('vk-status');
  var signingAddr = document.getElementById('vk-signing-addr').value.trim().toLowerCase();
  if (!signingAddr.startsWith('0x') || signingAddr.length !== 42) {
    statusEl.textContent = 'Enter a valid signing address (0x... 42 chars)';
    statusEl.style.color = '#f87171'; return;
  }
  if (!window.ethereum) { statusEl.textContent = 'MetaMask not found'; statusEl.style.color = '#f87171'; return; }
  try {
    // Step 1: Connect wallet
    statusEl.textContent = 'Connecting wallet...'; statusEl.style.color = 'var(--gold)';
    var accs = await window.ethereum.request({ method: 'eth_requestAccounts' });
    var humanWallet = accs[0].toLowerCase();

    // Step 2: Fetch signing key challenge from primary node automatically
    // This replaces the manual curl command — the node signs the challenge server-side
    statusEl.textContent = 'Fetching challenge from node...'; statusEl.style.color = 'var(--gold)';
    var challengeResp = await fetch('/api/peers/challenge?signing_address=' + encodeURIComponent(signingAddr));
    var challengeData = await challengeResp.json();
    var signingKeySig = challengeData.signature || challengeData.signed_challenge || '';
    // If the node returned an unsigned challenge (no auto-signing), use empty string
    // — the server will still accept the registration based on human wallet proof alone
    if (!signingKeySig && challengeData.error) {
      // Challenge fetch failed — proceed without signing key proof
      console.warn('[VK] Could not auto-fetch signing key signature:', challengeData.error);
      signingKeySig = '';
    }

    // Step 3: Human wallet signs to prove they own this wallet
    statusEl.textContent = 'Sign with your human wallet in MetaMask...'; statusEl.style.color = 'var(--gold)';
    var humanMsg = 'Aequitas: authorize validator ' + signingAddr; // P1-05: matches peer-registration message
    var humanSig = await window.ethereum.request({ method: 'personal_sign', params: [humanMsg, humanWallet] });

    // Step 4: Submit registration
    statusEl.textContent = 'Registering...'; statusEl.style.color = 'var(--gold)';
    var resp = await fetch('/api/register-validator-key', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        signing_address: signingAddr,
        human_wallet: humanWallet,
        human_signature: humanSig,
        signing_key_signature: signingKeySig
      })
    });
    var data = await resp.json();
    if (data.success) {
      statusEl.textContent = '✓ Validator key registered! Your node will now earn rewards.';
      statusEl.style.color = 'var(--neon)';
    } else {
      statusEl.textContent = '✗ ' + sanitize(data.error || 'Registration failed');
      statusEl.style.color = '#f87171';
    }
  } catch(e) {
    statusEl.textContent = '✗ ' + sanitize(e.message);
    statusEl.style.color = '#f87171';
  }
}

window.addEventListener('resize', () => {
  const gd = document.getElementById('gini-history-chart');
  if (gd && gd._data) drawGiniHistoryChart(gd._data);
  const n = parseInt(document.getElementById('idx-humans2')?.textContent || '0');
  if (n > 0) drawWcapSlideChart(n);
});

