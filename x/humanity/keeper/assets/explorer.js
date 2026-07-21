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
  'logo-sub':'PROOF OF HUMANITY','live':'LIVE',
  'tab-register':'🔐 Register','tab-explorer':'🔍 Explorer','tab-humans':'👥 Humans','tab-index':'📊 Index','tab-network':'🌐 Network','tab-protocol':'📜 Protocol V7','tab-swap':'🔄 Swap',
  'reg-title':'🔐 Register as a Verified Human',
  'reg-sub':'Join the Aequitas network and receive your 1,000 AEQ Universal Basic Income grant. Registration is one-time, permanent, and completely gasless. No personal data is ever stored.',
  'app-title':'REGISTRATION VIA ANDROID APP',
  'app-text':'Registration generates a cryptographic key inside your phone\'s secure hardware (Secure Enclave / StrongBox), gated behind your device\'s own screen-unlock — no separate sensor, no raw biometric data is ever produced, processed, or transmitted. A Groth16 ZK proof proves you hold that device key without revealing it. Your <strong style="color:var(--gold)">1,000 AEQ credited automatically</strong> upon verification. <strong style="color:var(--gold)">Note:</strong> this currently proves control of one device, not biological uniqueness across devices — see the FAQ below.',
  's1t':'Device Key','s1d':'Your phone\'s secure hardware generates a private key behind your existing screen-unlock (fingerprint/face/PIN, whichever you already use to unlock your phone). No separate sensor kit, no raw biometric data ever leaves the device.',
  's2t':'ZK Proof Generation','s2d':'A Groth16 ZK proof commits your device key into commitment = keccak256(deviceKey‖wallet) without revealing the key itself. The nullifier is bound to this device, not to your body — see the FAQ below.',
  's3t':'Connect Wallet','s3d':'The app opens MetaMask on this page · connect your Ethereum wallet · the proof is cryptographically bound to your wallet address',
  's4t':'1,000 AEQ Granted','s4d':'Registration confirmed on Aequitas BlockDAG within 1 second · 1,000 AEQ credited instantly · your identity is permanently recorded as a verified human',
  'priv-bar':'🔒 Device-bound cryptographic key · Groth16 ZKP · Data never leaves device · One registration per device',
  'conn-wallet':'CONNECTED WALLET','proof-recv':'⚡ ZK PROOF RECEIVED','proof-hint':'Connect wallet to register',
  'btn-conn':'🦊 CONNECT METAMASK','btn-reg':'🔐 REGISTER ON-CHAIN',
  'btn-wc':'🔗 CONNECT WALLETCONNECT',
  'reg-log-hint':'// Open Aequitas Android App to generate your proof, then return here...',
  'reg-details':'Registration Details','k-network':'Network','k-chainid':'Chain ID','k-grant':'UBI Grant',
  'k-fee':'Gas Fee','free':'FREE — completely gasless','k-limit':'Registrations','k-limit-v':'Once per device · permanent · immutable',
  'k-bio':'Device Key','never-stored':'Never leaves your device — no biometric data is produced or stored',
  'k-proof':'Proof System','k-conf':'Confirmation','k-conf-v':'Within 1 second (1 block)',
  'k-sybil':'Sybil Protection','k-sybil-v':'One identity per device · permanent lock (device-bound, not yet body-bound)',
  'live-stats':'Live Chain Statistics',
  's-height':'Block Height',
  's-humans':'Verified Humans','s-humans-sub':'Device-bound ZK proof · one registration per device',
  's-supply':'Total Supply','s-supply-sub':'Always = Humans × 1,000 AEQ',
  's-index':'Aequitas Index','s-index-sub':'0 = perfect equality · 100 = max inequality',
  's-uptime':'Uptime','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'Proof of Humanity','ib-poh-t':'Every AEQ holder proves control of a device-bound cryptographic key via a Groth16 zero-knowledge proof — no bots, no corporations, no AI. No biometric data ever leaves your device, only a mathematical proof is transmitted. This currently binds one registration per device, not yet per unique person — see the FAQ.',
  'ib-fair':'Radically Fair Distribution','ib-fair-t':'Every verified human receives exactly 1,000 AEQ upon registration — no more, no less. No pre-mine, no founder allocation, no investor rounds. Total supply always equals verified humans × 1,000.',
  'ib-dag':'BlockDAG Architecture','ib-dag-t':'Multiple blocks can be produced simultaneously and merged into the DAG. Higher throughput, lower latency, better fault tolerance than traditional linear blockchains.',
  'ib-gas':'Truly Gasless','ib-gas-t':'Registration and AEQ transfers cost absolutely nothing. No ETH, BNB, or MATIC required. No credit card, no bank account, no prior cryptocurrency needed.',
  'recent-blocks':'Recent Blocks','blocks-desc':'MERGE = multiple parents merged (BlockDAG). TX = registration transaction. Block time: __BT__.',
  'loading':'Loading blocks...','net-info':'Network Info','k-chain':'Chain Name','k-symbol':'Symbol','k-btime':'Block Time',
  'k-cons':'Consensus','k-nodes':'Active Nodes','k-storage':'Storage','add-mm':'🦊 ADD TO METAMASK','k-dec':'Decimals',
  'btn-add-mm':'+ ADD AEQUITAS NETWORK',
  'phil':'"Money exists because people exist.<br>Nothing more, nothing less."','phil-sub':'— THE AEQUITAS PRINCIPLE —',
  'humans-title':'Verified Humans on Aequitas Chain',
  'h-what':'What is a Verified Human?','h-what-t':'A Verified Human is currently a wallet address cryptographically proven to control one specific device\'s secure hardware key. Verification generates that key inside the phone\'s Secure Enclave/StrongBox, gated behind the device\'s own screen-unlock — no separate sensor kit. Only a Groth16 ZK proof is transmitted; no biometric data ever leaves the device. <strong style="color:var(--gold)">Today this verifies one device, not yet one unique person</strong> — see the FAQ for what that means in practice.',
  'h-zkp':'Zero-Knowledge Proof System','h-zkp-t':'Aequitas uses Groth16 on BN128 — same curve as Ethereum and Zcash. Proof: ~200 bytes. Verification: ~10ms. commitment = keccak256(deviceKey‖wallet). The nullifier is bound to this device: losing your phone does not let you create a second identity on it, but a different device can still register separately. No key material is ever revealed or stored server-side.',
  'h-sybil':'Sybil Resistance — Current State','h-sybil-t':'Today\'s nullifier is derived from a per-device hardware key, so it reliably blocks re-registering the SAME device, and blocks reusing the same identity across wallets. It does not yet detect the same person registering from a second physical device — closing that gap requires a real cross-device biological uniqueness check, which is planned as post-beta work rather than shipped today.',
  'h-global':'Global Financial Inclusion','h-global-t':'No bank account, no credit card, no prior cryptocurrency required. Just an Android smartphone with the screen-unlock (fingerprint/face/PIN) you already use. Aequitas is designed to be accessible to every human on Earth.',
  'h-bio-hw':'Identity Verification Roadmap','h-bio-hw-t':'Today (beta): a per-device hardware-backed cryptographic key, honestly labeled as device-bound rather than body-bound. Planned (post-beta): a real cross-device biological uniqueness check, to be specified, built, and independently audited before any stronger Sybil-resistance claim is made.',
  'reg-humans':'Registered Humans','h-desc':'Every address below has proven control of a device-bound cryptographic key via ZK proof and received exactly 1,000 AEQ. The registry is permanent, immutable, and on-chain. See the FAQ for what "device-bound" means for Sybil resistance today.',
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
  'curr-idx':'Current Index','bar-0':'0 — Perfect Equality','bar-100':'100 — Max Inequality','wcap-lbl':'Current Wealth Cap:','wcap-mult':'Multiplier:','wcap-avg':'Avg balance:',
  'gini':'Gini Coefficient','gini-desc':'0 = equal · 1 = unequal',
  'supply-desc':'Always = Humans × 1,000 AEQ',
  'phase':'Protocol Phase','phase-desc':'Auto-advances by human count',
  'humans-desc':'Device-bound ZK-verified registrations',
  'pools-title':'Redistribution Pools',
  'pools-desc':'Every swap fee, demurrage charge, and wealth cap overflow is automatically split across four pools. No manual intervention — the protocol handles all redistribution through code alone. All pools pay out daily.',
  'vel-pool':'Validators Pool','vel-pool-desc':'40% of all fees → node operators who secure the network',
  'liq-pool':'Liquidity Pool','liq-pool-desc':'30% of all fees → liquidity providers, proportional to LP shares',
  'ubi-pool':'UBI Pool','ubi-pool-desc':'20% of all fees → all verified humans equally, every 24 hours',
  'treasury':'Treasury','treasury-desc':'10% of all fees → protocol development and maintenance',
  'phases-title':'Protocol Phases',
  'phases-desc':'The wealth cap uses a bootstrap multiplier during Phase 0: max(5, min(N, 25))× average balance. With 1–4 humans: 5× average. Each new human adds 1×. At 25+ humans: locks permanently at 25×. Phase 1+ maintains 25× fixed. All transitions trigger automatically by human count — no governance, no admin key.',
  'p0':'Bootstrap · &lt;100 humans · Wealth Cap: max(5,min(N,25))× average · Slides 5×→25× until 25th human · Currently active',
  'p1':'Growth · 100–10,000 humans · Wealth Cap: 25× average balance',
  'p2':'Stability · 10,000–1M humans · Wealth Cap: 25× average balance',
  'p3':'Maturity · 1M+ humans · Wealth Cap: 25× average balance',
  'wealth-cap-explain':'The Wealth Cap in Phase 0 (Bootstrap) uses max(5, min(N, 25))× average AEQ balance, where N = registered humans. 1–4 humans: cap = 5× average. Each new human adds 1×. 25+ humans: locked permanently at 25×. The cap always scales with the live average balance.',
  'demurrage-title':'Demurrage — Incentive to Circulate',
  'demurrage-desc':'Aequitas implements a demurrage mechanism inspired by historical complementary currencies. Idle AEQ balances slowly lose value to discourage hoarding and incentivize economic participation.',
  'dem-rate-k':'Decay Rate','dem-rate-v':'0.5% per month (continuous, not stepped)',
  'dem-grace-k':'Grace Period','dem-grace-v':'3 months of inactivity before decay begins',
  'dem-reset-k':'Clock Reset','dem-reset-v':'Any transfer, swap, or liquidity action resets the timer to zero',
  'dem-dest-k':'Decayed AEQ goes to','dem-dest-v':'Redistribution pools (40/30/20/10 split)',
  'dem-warn-k':'Warning System','dem-warn-v':'14-day notice (shown once) + 7-day repeated reminder at each login',
  'story-title':'The Story of Aequitas — Why This Exists',
  'story-text':'<p>The year is 2009. Satoshi Nakamoto releases Bitcoin. For the first time, value can transfer between any two people without a bank. A genuine revolution. But something goes wrong almost immediately.</p><p>Early miners accumulate millions of coins at almost zero cost. By 2021, the top 1% of Bitcoin addresses control over 90% of all Bitcoin. Bitcoin\'s estimated Gini coefficient exceeds 0.85 — higher than any country on Earth. The cryptocurrency that was supposed to democratize finance created the most extreme wealth concentration in human history.</p><p><span style="color:var(--gold)">Aequitas</span> — Latin for "fairness" and "equality" — was created to answer a single question: <em style="color:var(--gold)">"What would a cryptocurrency look like if designed from first principles to be fair to every human being?"</em></p><p>The answer is simple: <strong style="color:var(--text)">Money exists because people exist. Therefore, every person should have an equal share of money simply by virtue of being human.</strong></p><p>Aequitas implements this mathematically. Every verified human receives 1,000 AEQ. No mining, no staking, no early-adopter advantage. The wealth cap, demurrage, and redistribution pools ensure inequality cannot accumulate indefinitely. The protocol adjusts automatically as the network grows.</p><p>The Aequitas network launched in June 2026. Currently in Phase 0. The goal: demonstrate that money can be distributed fairly, Gini coefficient held below 0.30 (comparable to the most equal developed nations), and financial inclusion achieved at global scale — without any central authority.</p><p><em style="color:var(--gold)">"Money exists because people exist. Nothing more, nothing less."</em></p>',
  'nodes-title':'Active Nodes — Current Network Topology',
  'nodes-desc':'The Aequitas network currently operates on multiple geographically distributed nodes (live count above). All of them participate in block production, state synchronization, and API serving. They communicate peer-to-peer via libp2p and synchronize block state via HTTP. Each node runs its own PostgreSQL database for persistent state. The network is designed to support additional nodes — any operator can join.',
  'run-node-title':'Run Your Own Node — Help Secure the Network',
  'run-node-desc':'Anyone can run an Aequitas node — no permission, no stake, no application required. Nodes participate in block production, validate the human registry, and synchronize the BlockDAG. Node operators earn a share of protocol fees via the Validators Pool (40% of all swap fees, distributed daily).',
  'bootstrap-title':'Connect a New Node','bootstrap-desc':'To run your own Aequitas node, set PRIMARY_NODE_URL=https://aequitas.digital in your environment. Your node registers automatically, syncs the full chain state, and begins participating in block production.',
  'tech-title':'Technical Specifications','mm-config':'MetaMask Configuration',
  'k-lang':'Language','k-src':'Source','evm-yes':'Yes — JSON-RPC /rpc · MetaMask compatible',
  'proto-label':'Aequitas V7 Protocol — Technical Documentation',
  'ca-title':'Contract Addresses',
  'ca-text':'Chain: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Main): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 is the single source of truth for the entire Aequitas economy. Every AEQ balance, every human registration, every UBI payout, and every wealth cap enforcement is governed by this one immutable contract — deployed on Aequitas Chain, a custom EVM-compatible blockchain running a BlockDAG consensus engine. There is no admin key, no upgrade proxy, no governance vote that can change a single line of its logic. The code that runs today is the code that will run in ten years.<br><br>The BioVerifier contract receives Groth16 zero-knowledge proofs generated entirely on the user\'s Android device. It verifies mathematically on-chain in ~10 ms that a new registrant is a unique living human — without ever learning their name, identity, or biometric data. This is what makes gasless, investment-free registration possible: the proof is the only thing that ever leaves the device.<br><br>Together, these two contracts make possible something that has never existed in any currency system in history: a money supply whose rules — who gets it, how much exists, how it redistributes — cannot be altered by any person, company, or government. Ever.',
  'poa-title':'1. PROOF OF ALIVE','poa-text':'<p>What happens to AEQ when people die or disappear? In Bitcoin, millions of BTC are permanently lost. In Aequitas, if someone is inactive for an extended period, their AEQ eventually returns to the community through the UBI pool.</p>',
  'poa-box':'Year 0-2: Normal usage<br>Year 2: Warning 1 — Guardian can respond<br>Year 2 + 60 days: Warning 2<br>Year 2 + 120 days: Warning 3<br>Year 2 + 180 days (2.5 years total, day 910): AEQ goes to PERSONAL ESCROW<br>Year 4 (day ~1460): If still inactive — returns to UBI Pool',
  'guard-title':'2. GUARDIAN SYSTEM','guard-text':'<p>What if someone cannot access their device for months? A trusted Guardian — another verified human — can confirm they are still alive, without any transaction rights.</p>',
  'guard-box':'1 Guardian per human (must be another verified human)<br>Guardian can ONLY call confirmAlive() — zero transaction rights<br>Guardian CANNOT move funds or transfer AEQ<br>Max 3 wards · 7-day timelock · No circular relationships allowed',
  'dem-title':'3. DEMURRAGE — Anti-Hoarding Mechanism',
  'dem-box':'Rate: 0.5%/month after 3 months grace period<br>Clock resets on any transfer, swap, or liquidity action<br>Decayed AEQ redistributed to pools (not burned)',
  'dem-text':'<p>Historical precedent: The Wörgl experiment (Austria, 1932) used a demurrage currency and reduced unemployment by 25% in one year. The Chiemgauer (Germany, 2003) has operated successfully for over 20 years using a similar mechanism.</p>',
  'cap-title':'4. WEALTH CAP — Mathematical Fairness','cap-box':'Bootstrap cap: max(5,min(N,25))× current average AEQ balance<br>1–4 humans: 5× · +1× per human · 25+: 25× permanently<br>Excess AEQ instantly redistributed · No manual intervention',
  'ubi-title':'5. UNIVERSAL BASIC INCOME','ubi-box':'Sources: Swap fees (20%) · Wealth cap overflow · Demurrage · Inactive escrow<br><br>Daily: UBI Pool divided equally among all registered humans. Pool resets to zero after each distribution and refills continuously.',
  'inf-title':'6. NO ALGORITHMIC INFLATION','inf-box':'The ONLY event that creates new AEQ: a new verified human registers<br><br>Total Supply = Verified Humans × 1,000 AEQ — always, exactly.',
  'explore-title':'Explore Aequitas',
  'expl-score':'Equality Score','expl-score-d':'Live Gini coefficient · Aequitas Index · wealth distribution in real time',
  'expl-economy':'UBI &amp; Redistribution Pools','expl-economy-d':'Daily UBI countdown · 4 on-chain pools · demurrage · Protocol Phases',
  'expl-charts':'Charts &amp; History','expl-charts-d':'Gini history · Lorenz curve · Wealth Cap bootstrap slider · The story of Aequitas',
  'expl-v7':'Protocol V7 Docs','expl-v7-d':'AequitasV7 contract · 6 mechanisms · ZK proof · wealth cap · demurrage · immutable code',
  'expl-explorer':'Block Explorer','expl-explorer-d':'Live BlockDAG · click any block to see validator, hash, transactions, parent hashes',
    'btn-download-app':'DOWNLOAD AEQUITAS APP',
  'usp-headline':'For the first time in history — everyone starts equal',
  'usp-sub':'If you own an Android smartphone, you qualify. No bank, no crypto background, no investment needed.',
  'usp-c1-title':'0.00 Start Investment','usp-c1-desc':'Registration is completely gasless. No ETH, no MATIC, no credit card. The protocol pays all fees on your behalf.',
  'usp-c2-title':'1,000 AEQ for every human','usp-c2-desc':'Billionaire or subsistence farmer — everyone gets exactly 1,000 AEQ. Not more, not less. Equal start, guaranteed by math.',
  'usp-c3-title':'Accessible to all','usp-c3-desc':'No bank account, no credit card, no government ID, no extra hardware to buy — just the screen-unlock already built into your Android phone.',
  'usp-c4-title':'Daily UBI forever','usp-c4-desc':'Once registered, you receive a daily share of UBI payouts automatically — every day, no action required.',
  'ubi-hero-title':'UNIVERSAL BASIC INCOME POOL','ubi-hero-sub':'Accumulating — next payout distributed equally to all verified humans in:',
  'ubi-hero-desc':'Split equally among all verified humans · paid every 24h · pool resets to zero after each payout · no minimum balance required',
  'ubi-bal-lbl':'current pool balance','ubi-how-fills':'HOW THE UBI POOL FILLS UP',
  'ubi-see-above':'see countdown above','ubi-timer-above':'⏰ countdown displayed above',
  'ubi-src-swap':'20% Swap Fees','ubi-src-swap-d':'Every AEQ↔tUSD swap contributes 20% of its 0.1% fee here. More trading activity = faster pool fill.',
  'ubi-src-dem':'variable Demurrage','ubi-src-dem-d':'Idle AEQ (3+ months inactive) decays at 0.5%/month. The decayed amount enters the 40/30/20/10 split — 20% goes to UBI.',
  'ubi-src-cap':'variable Wealth Cap Overflow','ubi-src-cap-d':'Wallets exceeding 25+ average balance have the excess confiscated instantly. 20% flows to UBI immediately.',
  'ubi-pool-desc':'20% of swap fees + demurrage + wealth cap overflow → divided equally among all verified humans every 24 hours. Even with zero trading, demurrage and wealth cap ensure the pool always fills.',
  'pool-t-timer':'Accumulates — no timer',
  'pools4-header':'ALL FOUR REDISTRIBUTION POOLS',
  'swap-title':'🔄 Swap AEQ ↔ tUSD',
  'swap-sub':'Exchange AEQ for tUSD (a simulated test-dollar) through the native liquidity pool. A 0.1% fee applies only to swaps — ordinary AEQ transfers between people remain completely free.',
  'swap-faucet-desc':'Claim 1,000 tUSD once to pair with your AEQ — for your first liquidity deposit.',
  'swap-btn-faucet':'CLAIM TEST tUSD (once)','swap-btn-conn':'🦊 CONNECT METAMASK','swap-btn-go':'🔄 SWAP',
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
  'pools-addr-title':'Pool Contract Addresses','swap-pools-addr-title':'Pool Addresses','swap-priv-bar':'🔒 Non-custodial · AMM x·y=k · 0.1% fee · Instant settlement · No slippage protection needed at small sizes',
  'v7-intro-title':'What is AequitasV7?',
  'v7-intro-text':'AequitasV7 is the single source of truth for the entire Aequitas economy. Every AEQ balance, every human registration, every UBI payout, and every wealth cap enforcement is governed by this one immutable contract.',
'expl-network':'Network &amp; Nodes','expl-network-d':'Node topology · run your own node · technical specs · Chain ID 1926'
,'swap-sell-label':'Sell','swap-receive-label':'Receive',
  'guard-title':'🛡 Guardian System','guard-my-lbl':'My Guardian','guard-none':'None',
  'guard-set-lbl':'Set / Change Guardian','guard-set-hint':'Must be a registered Aequitas human · 7-day timelock · Guardian can only confirm your liveness, not access funds · Max 3 wards per guardian',
  'guard-confirm-lbl':'Confirm Alive (As Guardian)','guard-confirm-hint':'If your ward cannot access their wallet, confirm their liveness to prevent their funds moving to escrow after 910 days of inactivity.','guard-recover-btn':'🔓 RECOVER FROM ESCROW',
  'faq-title':'❓ FAQ','faq-q1':'Is my biometric data safe?','faq-a1':'Yes. No fingerprint, face, or other biometric image is ever captured, processed, or transmitted by Aequitas. Your phone\'s own screen-unlock simply gates access to a random key generated and stored in its secure hardware. Only a mathematical proof derived from that key is ever sent to Aequitas — never the key itself, never any biometric data.',
  'faq-q1b':'Does registration prove I am a unique real person?','faq-a1b':'Not yet, fully. Today\'s proof cryptographically proves you control one specific device\'s secure hardware key — it reliably stops that same device (or wallet) from registering twice, but it cannot currently tell two different physical devices apart if the same person owns both. A genuine cross-device biological uniqueness check is planned work, not something already shipped — we\'d rather say that plainly than overstate what today\'s device-bound proof guarantees.',
  'faq-q2':'Can I register with a different wallet later?','faq-a2':'No. Registration is permanently bound to one wallet address per device key. This is by design — it prevents re-registering the same device and enforces one-wallet-per-device-identity.',
  'faq-q3':'What happens if I lose my phone?','faq-a3':'Your AEQ remains in your wallet — it is tied to your private key, not your phone. You can still access your wallet via MetaMask with your seed phrase. Wallet recovery is independent of the device-key registration.',
  'path-title':'Choose Your Path','path-human-title':'I am a Human','path-human-desc':'I want to register, receive 1,000 AEQ, and join the basic income network.','path-human-steps':'1. Download the Aequitas Android App<br>2. Unlock with your device\'s screen-lock (fingerprint/face/PIN)<br>3. Connect MetaMask<br>4. Receive 1,000 AEQ instantly',
  'path-node-title':'I am a Node Operator','path-node-desc':'I want to run a full node, participate in block production, and earn from the 40% validator pool.','path-node-steps':'1. Register as a human (required)<br>2. Set PRIMARY_NODE_URL=https://aequitas.digital<br>3. Deploy on Railway/Contabo/VPS<br>4. Earn daily from validator pool',
  'path-dev-title':'I am a Developer','path-dev-desc':'I want to build on Aequitas, integrate the API, or contribute to the protocol.','path-dev-steps':'1. EVM-compatible JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Metrics: /metrics (Prometheus)',
  'story-flow-title':'AEQ Token Flow Diagram','story-topo-title':'Network Topology — Current State',
  'swap-price-title':'AEQ / tUSD — Live Price','swap-price-desc':'Real-time price derived from pool reserves (x·y=k). Updates every 8 seconds as new pool data arrives.','swap-price-empty':'No pool data yet — add liquidity to see the price chart.',
  'node-guide-lang-note':'This inline guide is in English. A translated PDF is available in your language using the button above.',
  'k-zkp':'ZKP System','k-hash':'Hash System','k-sybil-prot':'Sybil Protection',
},
de:{
  'logo-sub':'MENSCHLICHKEITSNACHWEIS','live':'LIVE',
  'tab-register':'🔐 Registrieren','tab-explorer':'🔍 Explorer','tab-humans':'👥 Menschen','tab-index':'📊 Index','tab-network':'🌐 Netzwerk','tab-protocol':'📜 Protokoll V7','tab-swap':'🔄 Tauschen',
  'reg-title':'🔐 Als verifizierter Mensch registrieren',
  'reg-sub':'Tritt dem Aequitas-Netzwerk bei und erhalte dein Universelles Grundeinkommen von 1.000 AEQ. Einmalig, permanent und vollständig gebührenfrei. Keine persönlichen Daten werden jemals gespeichert.',
  'app-title':'REGISTRIERUNG NUR ÜBER ANDROID-APP',
  'app-text':'Die Registrierung erzeugt einen kryptografischen Schlüssel in der sicheren Hardware deines Telefons (Secure Enclave / StrongBox), abgesichert durch die Bildschirmsperre deines Geräts — kein separater Sensor, keine rohen biometrischen Daten werden je erzeugt, verarbeitet oder übertragen. Ein Groth16-ZK-Beweis beweist, dass du diesen Geräteschlüssel besitzt, ohne ihn preiszugeben. Deine <strong style="color:var(--gold)">1.000 AEQ werden automatisch gutgeschrieben</strong>. <strong style="color:var(--gold)">Hinweis:</strong> Das beweist aktuell den Besitz eines Geräts, nicht biologische Einzigartigkeit über mehrere Geräte hinweg — siehe FAQ unten.',
  's1t':'Geräteschlüssel','s1d':'Die sichere Hardware deines Telefons erzeugt einen privaten Schlüssel, abgesichert durch deine bestehende Bildschirmsperre (Fingerabdruck/Gesicht/PIN — was auch immer du bereits zum Entsperren nutzt). Kein separates Sensor-Kit, keine rohen biometrischen Daten verlassen je das Gerät.',
  's2t':'ZK-Beweis-Erzeugung','s2d':'Ein Groth16-ZK-Beweis bindet deinen Geräteschlüssel in commitment = keccak256(deviceKey‖wallet), ohne den Schlüssel selbst preiszugeben. Der Nullifier ist an dieses Gerät gebunden, nicht an deinen Körper — siehe FAQ unten.',
  's3t':'Wallet verbinden','s3d':'Die App öffnet MetaMask auf dieser Seite · verbinde deine Ethereum-Wallet · der Beweis ist kryptografisch an deine Wallet-Adresse gebunden',
  's4t':'1.000 AEQ gutgeschrieben','s4d':'Registrierung auf Aequitas BlockDAG innerhalb von 1 Sekunde bestätigt · 1.000 AEQ sofort gutgeschrieben · deine Identität ist dauerhaft als verifizierter Mensch gespeichert',
  'priv-bar':'🔒 Gerätegebundener kryptografischer Schlüssel · Groth16 ZKP · Daten verlassen nie das Gerät · Eine Registrierung pro Gerät',
  'conn-wallet':'VERBUNDENE WALLET','proof-recv':'⚡ ZK-BEWEIS EMPFANGEN','proof-hint':'Wallet verbinden um zu registrieren',
  'btn-conn':'🦊 METAMASK VERBINDEN','btn-reg':'🔐 ON-CHAIN REGISTRIEREN',
  'btn-wc':'🔗 WALLETCONNECT VERBINDEN',
  'reg-log-hint':'// Öffne die Aequitas Android App um deinen Beweis zu erstellen, dann kehre hierher zurück...',
  'reg-details':'Registrierungsdetails','k-network':'Netzwerk','k-chainid':'Chain-ID','k-grant':'UBI-Zuteilung',
  'k-fee':'Gasgebühr','free':'KOSTENLOS — vollständig gebührenfrei','k-limit':'Registrierungen','k-limit-v':'Einmal pro Gerät · permanent · unveränderlich',
  'k-bio':'Geräteschlüssel','never-stored':'Verlässt nie dein Gerät — es werden keine biometrischen Daten erzeugt oder gespeichert',
  'k-proof':'Beweissystem','k-conf':'Bestätigung','k-conf-v':'Innerhalb von 1 Sekunde (1 Block)',
  'k-sybil':'Sybil-Schutz','k-sybil-v':'Eine Identität pro Gerät · dauerhaft gesperrt (gerätegebunden, noch nicht körpergebunden)',
  'live-stats':'Live-Chain-Statistiken',
  's-height':'Blockhöhe',
  's-humans':'Verifizierte Menschen','s-humans-sub':'Gerätegebundener ZK-Beweis · eine Registrierung pro Gerät',
  's-supply':'Gesamtmenge','s-supply-sub':'Immer = Menschen × 1.000 AEQ',
  's-index':'Aequitas-Index','s-index-sub':'0 = perfekte Gleichheit · 100 = maximale Ungleichheit',
  's-uptime':'Laufzeit','s-uptime-sub':'Node v0.3.0 · Railway (Primär) + 2x Contabo VPS (Sekundär) · je eigene PostgreSQL',
  'ib-poh':'Menschlichkeitsnachweis','ib-poh-t':'Jeder AEQ-Inhaber beweist den Besitz eines gerätegebundenen kryptografischen Schlüssels per Groth16-ZK-Beweis. Keine Bots, keine Unternehmen, keine KI. Es werden nie biometrische Daten übertragen, nur ein mathematischer Beweis. Das bindet aktuell eine Registrierung pro Gerät, noch nicht pro einzigartigem Menschen — siehe FAQ.',
  'ib-fair':'Radikal gerechte Verteilung','ib-fair-t':'Jeder verifizierte Mensch erhält genau 1.000 AEQ bei der Registrierung. Kein Pre-Mining, keine Gründerzuteilung. Gesamtmenge entspricht immer Verifizierte Menschen × 1.000.',
  'ib-dag':'BlockDAG-Architektur','ib-dag-t':'Mehrere Blöcke können gleichzeitig produziert und zusammengeführt werden. Höherer Durchsatz, geringere Latenz als lineare Blockchains.',
  'ib-gas':'Wirklich gebührenfrei','ib-gas-t':'Registrierung und AEQ-Transfers kosten absolut nichts. Kein ETH, BNB oder MATIC erforderlich. Kein Bankkonto, keine Kreditkarte nötig.',
  'recent-blocks':'Aktuelle Blöcke','blocks-desc':'MERGE = mehrere Eltern zusammengeführt (BlockDAG). TX = Registrierungstransaktion. Blockzeit: __BT__.',
  'loading':'Blöcke werden geladen...','net-info':'Netzwerkinformationen','k-chain':'Chain-Name','k-symbol':'Symbol','k-btime':'Blockzeit',
  'k-cons':'Konsens','k-nodes':'Aktive Nodes','k-storage':'Speicher','add-mm':'🦊 ZU METAMASK HINZUFÜGEN','k-dec':'Dezimalstellen',
  'btn-add-mm':'+ AEQUITAS-NETZWERK HINZUFÜGEN',
  'phil':'"Geld existiert weil Menschen existieren.<br>Nichts mehr, nichts weniger."','phil-sub':'— DAS AEQUITAS-PRINZIP —',
  'humans-title':'Verifizierte Menschen auf der Aequitas Chain',
  'h-what':'Was ist ein verifizierter Mensch?','h-what-t':'Ein verifizierter Mensch ist aktuell eine Wallet-Adresse, die kryptografisch bewiesen einen gerätegebundenen Schlüssel besitzt. Der Schlüssel wird in der sicheren Hardware des Telefons erzeugt, abgesichert durch die Bildschirmsperre — kein separates Sensor-Kit. Nur ein Groth16-ZK-Beweis wird übertragen. <strong style="color:var(--gold)">Das verifiziert heute ein Gerät, noch nicht zwingend einen einzigartigen Menschen</strong> — siehe FAQ.',
  'h-zkp':'Zero-Knowledge-Beweissystem','h-zkp-t':'Aequitas verwendet Groth16 auf BN128 — dieselbe Kurve wie Ethereum und Zcash. ~200 Bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). Der Nullifier ist an dieses Gerät gebunden: Telefonverlust erzeugt auf diesem Gerät keine zweite Identität, ein anderes Gerät kann sich aber weiterhin separat registrieren.',
  'h-sybil':'Sybil-Resistenz — Aktueller Stand','h-sybil-t':'Der heutige Nullifier basiert auf einem gerätegebundenen Hardware-Schlüssel — er verhindert zuverlässig, dass dasselbe Gerät oder dieselbe Wallet zweimal registriert. Er erkennt aber noch nicht, wenn dieselbe Person sich von einem zweiten physischen Gerät aus registriert. Das zu schließen erfordert eine echte geräteübergreifende biologische Einzigartigkeitsprüfung — geplant für nach der Beta, nicht bereits ausgeliefert.',
  'h-global':'Globale finanzielle Inklusion','h-global-t':'Kein Bankkonto, keine Kreditkarte, keine Kryptowährung erforderlich. Nur ein Android-Smartphone mit der Bildschirmsperre (Fingerabdruck/Gesicht/PIN), die du ohnehin schon nutzt.',
  'h-bio-hw':'Identitätsverifikation — Roadmap','h-bio-hw-t':'Heute (Beta): ein gerätegebundener Hardware-Schlüssel pro Gerät, ehrlich als gerätegebunden statt körpergebunden gekennzeichnet. Geplant (nach der Beta): eine echte geräteübergreifende biologische Einzigartigkeitsprüfung — spezifiziert, gebaut und unabhängig geprüft, bevor eine stärkere Sybil-Schutz-Aussage getroffen wird.',
  'reg-humans':'Registrierte Menschen','h-desc':'Jede Adresse hat per ZK-Beweis den Besitz eines gerätegebundenen kryptografischen Schlüssels bewiesen und genau 1.000 AEQ erhalten. Dauerhaft, unveränderlich, on-chain. Siehe FAQ, was "gerätegebunden" für den Sybil-Schutz heute bedeutet.',
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
  'curr-idx':'Aktueller Index','bar-0':'0 — Perfekte Gleichheit','bar-100':'100 — Max. Ungleichheit','wcap-lbl':'Aktuelle Vermögensobergrenze:','wcap-mult':'Multiplikator:','wcap-avg':'Durchschnittsguthaben:',
  'gini':'Gini-Koeffizient','gini-desc':'0 = gleich · 1 = ungleich',
  'supply-desc':'Immer = Menschen × 1.000 AEQ',
  'phase':'Protokollphase','phase-desc':'Automatisch nach Menschenanzahl',
  'humans-desc':'Gerätegebunden ZK-verifizierte Registrierungen',
  'pools-title':'Umverteilungspools',
  'pools-desc':'Jede Swap-Gebühr, Demurrage-Belastung und Vermögensobergrenze-Überschuss wird automatisch auf vier Pools aufgeteilt. Keine manuelle Eingriffe. Alle Pools zahlen täglich aus.',
  'vel-pool':'Validatoren-Pool','vel-pool-desc':'40% aller Gebühren → Node-Betreiber die das Netzwerk sichern',
  'liq-pool':'Liquiditäts-Pool','liq-pool-desc':'30% aller Gebühren → Liquiditätsanbieter, proportional zu LP-Anteilen',
  'ubi-pool':'UBI-Pool','ubi-pool-desc':'20% aller Gebühren → alle verifizierten Menschen gleichmäßig, alle 24 Stunden',
  'treasury':'Schatzkammer','treasury-desc':'10% aller Gebühren → Protokollentwicklung und -wartung',
  'phases-title':'Protokollphasen',
  'phases-desc':'In Phase 0 verwendet die Vermögensobergrenze einen Bootstrap-Multiplikator: max(5, min(N, 25))× Durchschnittsguthaben. Mit 1–4 Menschen: 5× Durchschnitt. Jeder neue Mensch erhöht um 1×. Ab 25+ Menschen: dauerhaft auf 25× fixiert. Phase 1+ behält 25× fest. Alle Übergänge erfolgen automatisch — kein Governance-Vote, kein Admin-Key.',
  'p0':'Bootstrap · &lt;100 Menschen · Vermögensobergrenze: max(5,min(N,25))× Durchschnitt · Gleitet 5×→25× bis zum 25. Menschen · Derzeit aktiv',
  'p1':'Wachstum · 100–10.000 Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben',
  'p2':'Stabilität · 10.000–1M Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben',
  'p3':'Reife · 1M+ Menschen · Vermögensobergrenze: 25× Durchschnittsguthaben',
  'wealth-cap-explain':'Die Vermögensobergrenze in Phase 0 (Bootstrap) verwendet max(5, min(N, 25))× Durchschnittsguthaben, wobei N = registrierte Menschen. 1–4 Menschen: 5× Durchschnitt. Jeder neue Mensch erhöht um 1×. Ab 25+ Menschen: dauerhaft 25×. Die Obergrenze skaliert stets mit dem Live-Durchschnittsguthaben.',
  'demurrage-title':'Demurrage — Anreiz zum Zirkulieren',
  'demurrage-desc':'Aequitas implementiert einen Demurrage-Mechanismus inspiriert von historischen Komplementärwährungen. Inaktive AEQ-Guthaben verlieren langsam an Wert um Hortung zu entmutigen.',
  'dem-rate-k':'Verfallsrate','dem-rate-v':'0,5% pro Monat (kontinuierlich, nicht gestuft)',
  'dem-grace-k':'Schonfrist','dem-grace-v':'3 Monate Inaktivität bevor der Verfall beginnt',
  'dem-reset-k':'Uhr-Reset','dem-reset-v':'Jede Überweisung, Swap oder Liquiditätsaktion setzt den Timer zurück',
  'dem-dest-k':'Verfallenes AEQ geht an','dem-dest-v':'Umverteilungspools (40/30/20/10 Aufteilung)',
  'dem-warn-k':'Warnsystem','dem-warn-v':'14-Tage-Hinweis (einmal) + 7-Tage-Wiederholung bei jedem Login',
  'story-title':'Die Geschichte von Aequitas — Warum es das gibt',
  'story-text':'<p>Das Jahr ist 2009. Satoshi Nakamoto veröffentlicht Bitcoin. Zum ersten Mal kann Wert zwischen zwei Menschen ohne eine Bank übertragen werden. Eine echte Revolution. Aber fast sofort läuft etwas schief.</p><p>Frühe Miner häufen Millionen von Coins zu fast null Kosten an. Bis 2021 kontrollieren die obersten 1% der Bitcoin-Adressen über 90% aller Bitcoin. Bitcoins geschätzter Gini-Koeffizient übersteigt 0,85 — höher als in jedem Land auf der Erde.</p><p><span style="color:var(--gold)">Aequitas</span> — Lateinisch für "Fairness" und "Gleichheit" — wurde geschaffen um eine einzige Frage zu beantworten: <em style="color:var(--gold)">"Wie würde eine Kryptowährung aussehen die von Grund auf fair für jeden Menschen konzipiert wurde?"</em></p><p>Die Antwort ist einfach: <strong style="color:var(--text)">Geld existiert weil Menschen existieren. Daher sollte jeder Mensch einfach durch seine Existenz einen gleichen Anteil am Geld haben.</strong></p><p>Aequitas setzt dies mathematisch um. Jeder verifizierte Mensch erhält 1.000 AEQ. Kein Mining, kein Staking, kein Frühanwender-Vorteil. Die Vermögensobergrenze, Demurrage und Umverteilungspools stellen sicher dass sich Ungleichheit nicht unbegrenzt anhäufen kann.</p><p><em style="color:var(--gold)">"Geld existiert weil Menschen existieren. Nichts mehr, nichts weniger."</em></p>',
  'nodes-title':'Aktive Nodes — Aktuelle Netzwerktopologie',
  'nodes-desc':'Das Aequitas-Netzwerk betreibt derzeit mehrere geografisch verteilte Nodes (aktuelle Anzahl oben). Alle nehmen an Blockproduktion, Statussynchronisation und API-Bereitstellung teil. Sie kommunizieren per libp2p und synchronisieren Blockzustände via HTTP. Das Netzwerk ist für zusätzliche Nodes ausgelegt — jeder Betreiber kann beitreten.',
  'run-node-title':'Eigenen Node betreiben — Das Netzwerk sichern',
  'run-node-desc':'Jeder kann einen Aequitas-Node betreiben — keine Genehmigung, kein Stake, keine Bewerbung erforderlich. Nodes nehmen an der Blockproduktion teil und validieren die Menschenregistrierung. Node-Betreiber erhalten täglich einen Anteil der Protokollgebühren über den Validators-Pool (40% aller Swap-Gebühren).',
  'bootstrap-title':'Neuen Node verbinden','bootstrap-desc':'Um einen eigenen Aequitas-Node zu betreiben, setze die PRIMARY_NODE_URL=https://aequitas.digital in deiner Umgebung. Dein Node synchronisiert automatisch den vollständigen Chain-Zustand und beginnt mit der Blockproduktion.',
  'tech-title':'Technische Spezifikationen','mm-config':'MetaMask-Konfiguration',
  'k-lang':'Sprache','k-src':'Quellcode','evm-yes':'Ja — JSON-RPC /rpc · MetaMask-kompatibel',
  'proto-label':'Aequitas V7 Protokoll — Technische Dokumentation',
  'ca-title':'Contract- & Netzwerk-Adressen','ca-text':'Chain: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier (Groth16 On-Chain-Verifier): 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Haupt-Contract): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 ist die einzige Wahrheitsquelle der gesamten Aequitas-Wirtschaft. Jedes AEQ-Guthaben, jede Menschenregistrierung, jede UBI-Auszahlung und jede Durchsetzung der Vermögensobergrenze wird durch diesen einen unveränderlichen Contract geregelt — deployed auf der Aequitas Chain, einer maßgeschneiderten EVM-kompatiblen Blockchain mit BlockDAG-Konsens. Es gibt keinen Admin-Schlüssel, keinen Upgrade-Proxy, keine Governance-Abstimmung die eine einzige Zeile seiner Logik ändern könnte. Der Code der heute läuft ist der Code der in zehn Jahren läuft.<br><br>Der BioVerifier-Contract empfängt Groth16-Zero-Knowledge-Beweise die vollständig auf dem Android-Gerät des Nutzers erzeugt werden. Er verifiziert mathematisch on-chain in ~10 ms dass ein neuer Registrierungskandidat ein einzigartiger lebender Mensch ist — ohne jemals seinen Namen, seine Identität oder seine biometrischen Daten zu erfahren. Das ist es was die gasfreie, investitionsfreie Registrierung möglich macht: Der Beweis ist das Einzige was das Gerät je verlässt.<br><br>Zusammen machen diese zwei Contracts etwas möglich das in keinem Währungssystem der Geschichte je existiert hat: eine Geldmenge deren Regeln — wie viel existiert, wer es bekommt, wie es umverteilt wird — von keiner Person, keinem Unternehmen und keiner Regierung je geändert werden können. Niemals.',
  'ib-poh':'Menschlichkeitsnachweis','ib-poh-t':'Jeder AEQ-Inhaber beweist kryptografisch den Besitz eines gerätegebundenen Schlüssels. Keine Bots, keine Unternehmen, keine KI. Es werden nie biometrische Daten übertragen, nur ein mathematischer Beweis. Das bindet aktuell eine Registrierung pro Gerät — siehe FAQ für den heutigen Stand des Sybil-Schutzes.',
  'ib-fair':'Radikal faire Verteilung','ib-fair-t':'Jeder verifizierte Mensch erhält bei der Registrierung genau 1.000 AEQ — nicht mehr, nicht weniger. Kein Pre-Mining, keine Gründer-Zuteilung, keine Investorenrunden. Die Gesamtmenge ist immer und exakt gleich der Anzahl verifizierter Menschen multipliziert mit 1.000. Dies wird mathematisch erzwungen, nicht durch Richtlinien.',
  'ib-dag':'BlockDAG-Architektur','ib-dag-t':'Im Gegensatz zu traditionellen Blockchains wo nur ein Block pro Höhe existieren kann, verwendet Aequitas eine DAG-Struktur. Mehrere Blöcke können gleichzeitig von verschiedenen Nodes produziert und später in den DAG zusammengeführt werden. Dies ermöglicht höheren Durchsatz, niedrigere Latenz und eliminiert Einzelknoten-Engpässe. Merge-Ereignisse werden im Explorer mit einem speziellen Badge markiert.',
  'ib-gas':'Wirklich gebührenfrei','ib-gas-t':'Alle Registrierungen und AEQ-Übertragungen kosten absolut nichts. Kein ETH, BNB oder MATIC erforderlich. Keine Kreditkarte, kein Bankkonto, keine vorherige Kryptowährung nötig. Der Relayer übernimmt alle Transaktionskosten. Wenn du ein Mensch mit einem Smartphone bist, kannst du teilnehmen — unabhängig von deiner wirtschaftlichen Situation.',
  'h-what':'Was ist ein verifizierter Mensch?','h-what-t':'Ein verifizierter Mensch ist aktuell eine Wallet-Adresse, die kryptografisch den Besitz eines gerätegebundenen Schlüssels bewiesen hat. Der Schlüssel wird in der sicheren Hardware des Telefons erzeugt, abgesichert durch die Bildschirmsperre — kein separates Sensor-Kit. Nur ein Groth16-ZK-Beweis wird übertragen, keine biometrischen Daten verlassen das Gerät. <strong style="color:var(--gold)">Das verifiziert heute ein Gerät, noch nicht zwingend einen einzigartigen Menschen</strong> — siehe FAQ.',
  'h-zkp':'Zero-Knowledge-Proof-System','h-zkp-t':'Aequitas verwendet Groth16 auf BN128 — dieselbe Kurve wie Ethereum und Zcash. Beweisgröße: ~200 Byte. Verifikationszeit: ~10ms. commitment = keccak256(deviceKey‖wallet). Der Nullifier ist an dieses Gerät gebunden: Telefonverlust erzeugt keine zweite Identität auf diesem Gerät, ein anderes Gerät kann sich aber weiterhin separat registrieren. Kein Schlüsselmaterial wird je serverseitig offengelegt oder gespeichert.',
  'h-sybil':'Sybil-Resistenz — Aktueller Stand','h-sybil-t':'Der heutige Nullifier basiert auf einem gerätegebundenen Hardware-Schlüssel — er verhindert zuverlässig die doppelte Registrierung desselben Geräts oder derselben Wallet. Er erkennt aber noch nicht, wenn dieselbe Person sich von einem zweiten physischen Gerät aus registriert. Das zu schließen erfordert eine echte geräteübergreifende biologische Einzigartigkeitsprüfung — geplant für nach der Beta.',
  'h-global':'Globale finanzielle Inklusion','h-global-t':'1,4 Milliarden Erwachsene weltweit haben kein Bankkonto. Aequitas benötigt nur ein Android-Smartphone mit einem Fingerabdruck- oder Gesichtssensor — ein Gerät das über 3 Milliarden Menschen bereits besitzen. Kein Bankkonto, keine Kreditkarte, keine vorherige Kryptowährung, kein Personalausweis. Einfach Mensch zu sein reicht aus.',
  'h-bio-hw':'Identitätsverifikation — Roadmap','h-bio-hw-t':'Heute (Beta): ein gerätegebundener Hardware-Schlüssel pro Gerät, ehrlich als gerätegebunden statt körpergebunden gekennzeichnet. Geplant (nach der Beta): eine echte geräteübergreifende biologische Einzigartigkeitsprüfung — spezifiziert, gebaut und unabhängig geprüft, bevor eine stärkere Sybil-Schutz-Aussage getroffen wird.',
  'poa-title':'1. LEBENSNACHWEIS — Inaktive Guthaben-Rückgewinnung','poa-text':'<p>Was passiert mit AEQ wenn Menschen sterben oder dauerhaft handlungsunfähig werden? Bei Bitcoin und den meisten Kryptowährungen bedeuten verlorene Wallets dauerhaft verlorene Menge. Aequitas löst dies durch ein mehrstufiges Inaktivitäts-Rückgewinnungssystem: Wenn eine Wallet über einen längeren Zeitraum keine Aktivität zeigt, wird ihr Guthaben schrittweise über den UBI-Pool zur Gemeinschaft zurückgeführt.</p>',
  'poa-box':'Jahr 0–2: Normale Nutzung — keine Einschränkungen<br>Jahr 2: Warnung 1 — Guardian kann im Namen antworten<br>Jahr 2 + 60 Tage: Warnung 2 — steigende Dringlichkeit<br>Jahr 2 + 120 Tage: Warnung 3 — letzte Benachrichtigung<br>Jahr 2 + 180 Tage (insgesamt 2,5 Jahre, Tag 910): AEQ in persönliches TREUHANDKONTO verschoben (noch rückgewinnbar)<br>Jahr 4 (Tag ~1460): Bei weiter Inaktivität — Treuhand an UBI-Pool freigegeben',
  'guard-title':'2. GUARDIAN-SYSTEM — Menschliche Absicherung','guard-text':'<p>Was wenn jemand hospitalisiert, inhaftiert oder anderweitig monatelang nicht in der Lage ist auf sein Gerät zuzugreifen? Das Guardian-System erlaubt einer vertrauenswürdigen Person — einem anderen verifizierten Menschen — zu bestätigen dass der Wallet-Inhaber noch lebt, wodurch verhindert wird dass sein AEQ ins Treuhandkonto verschoben wird. Der Guardian hat strikt null finanziellen Zugang: Er kann nur eine einzige Funktion aufrufen die den Inaktivitätstimer zurücksetzt. Er kann unter keinen Umständen Gelder verschieben, ausgeben oder darauf zugreifen.</p>',
  'guard-box':'1 Guardian pro Mensch · muss ein verifizierter Mensch auf Aequitas sein<br>Guardian kann NUR confirmAlive() aufrufen — null Transaktionsrechte<br>Guardian KANN KEINE Gelder verschieben, AEQ übertragen oder auf die Wallet zugreifen<br>Maximal 3 Schutzbefohlene pro Guardian (verhindert Zentralisierung des Vertrauens)<br>7-Tage-Zeitsperre bei Guardian-Zuweisung (verhindert erzwungene Zuweisung)<br>Keine zirkulären Guardian-Beziehungen erlaubt',
  'dem-title':'3. DEMURRAGE — Anti-Hortungs-Mechanismus',
  'dem-box':'Rate: 0,5% pro Monat nach 3 Monaten Inaktivität (kontinuierlich, nicht gestuft)<br>Uhr setzt sich automatisch zurück bei jeder Überweisung, Swap oder Liquiditätsaktion<br>Verfallenes AEQ wird an die vier Pools umverteilt — niemals vernichtet<br>14-Tage-Warnung einmalig angezeigt · 7-Tage-Warnung bei jeder aktiven Sitzung wiederholt',
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
  'swap-btn-conn':'🦊 METAMASK VERBINDEN','swap-btn-go':'🔄 TAUSCHEN',
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
  'swap-pools-addr-title':'Tokenomics-Pool-Adressen','pools-addr-title':'Pool-Vertragsadressen',
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
  'usp-c3-title':'Für alle zugänglich','usp-c3-desc':'Kein Bankkonto, keine Kreditkarte, kein Personalausweis, keine zusätzliche Hardware zu kaufen — nur die Bildschirmsperre, die dein Android-Telefon bereits eingebaut hat.',
  'usp-c4-title':'Täglich UBI empfangen','usp-c4-desc':'Nach der Registrierung erhältst du automatisch täglich einen Anteil der UBI-Ausschüttung — jeden Tag, ohne Aktion, solange du AEQ hältst.',
  'v7-intro-title':'Was ist AequitasV7?',
  'v7-intro-text':'AequitasV7 ist der zentrale Smart Contract des Aequitas-Protokolls. "V7" steht für die 7. Hauptversion des Fairness-Contracts — das Ergebnis iterativer Designverbesserung. Er ist unveränderlich auf der Aequitas Chain (Chain ID 1926) deployed und regelt jeden Aspekt des Protokolls: Menschenregistrierung, ZK-Beweisverifizierung, Guthabenverwaltung, Vermögensobergrenze, UBI-Ausschüttung, Swap-Gebühren und alle Governance-Parameter. Kein Admin kann den Contract upgraden oder ersetzen — er ist das unveränderliche Gesetz der Aequitas-Wirtschaft.',
  'explore-title':'Aequitas entdecken',
  'expl-score':'Gleichheits-Score','expl-score-d':'Live-Gini-Koeffizient · Aequitas-Index · Vermögensverteilung in Echtzeit',
  'expl-economy':'UBI &amp; Umverteilungspools','expl-economy-d':'Täglicher UBI-Countdown · 4 On-Chain-Pools · Demurrage · Protokollphasen',
  'expl-charts':'Diagramme &amp; Verlauf','expl-charts-d':'Gini-Verlauf · Lorenz-Kurve · Vermögensobergrenze-Bootstrap-Slider · Die Geschichte von Aequitas',
  'expl-v7':'Protokoll V7 Dokumentation','expl-v7-d':'AequitasV7-Contract · 6 Mechanismen · ZK-Beweis · Vermögensobergrenze · Demurrage · unveränderlicher Code',
  'expl-explorer':'Block-Explorer','expl-explorer-d':'Live-BlockDAG · Block anklicken um Validator, Hash, Transaktionen, Eltern-Hashes zu sehen',
  'swap-sell-label':'Verkaufen','swap-receive-label':'Erhalten',
  'expl-network':'Netzwerk &amp; Nodes','expl-network-d':'Node-Topologie · eigenen Node betreiben · technische Spezifikationen · Chain-ID 1926',
  'guard-title':'🛡 Guardian-System','guard-my-lbl':'Mein Guardian','guard-none':'Keiner',
  'guard-set-lbl':'Guardian festlegen / ändern','guard-set-hint':'Muss ein registrierter Aequitas-Mensch sein · 7-Tage-Zeitsperre · Guardian kann nur deine Lebendigkeit bestätigen, nicht auf Guthaben zugreifen · Max. 3 Schützlinge pro Guardian',
  'guard-confirm-lbl':'Lebendig bestätigen (Als Guardian)','guard-confirm-hint':'Falls dein Schützling keinen Zugang zu seiner Wallet hat, bestätige seine Lebendigkeit, um zu verhindern, dass Gelder nach 910 Tagen Inaktivität ins Escrow überführt werden.','guard-recover-btn':'🔓 AUS ESCROW ZURÜCKFORDERN',
  'faq-title':'❓ FAQ','faq-q1':'Sind meine biometrischen Daten sicher?','faq-a1':'Ja. Kein Fingerabdruck, kein Gesichtsbild und keine anderen biometrischen Daten werden von Aequitas je erfasst, verarbeitet oder übertragen. Die Bildschirmsperre deines Telefons gibt lediglich Zugriff auf einen zufälligen Schlüssel frei, der in der sicheren Hardware erzeugt und gespeichert wird. Nur ein mathematischer Beweis, der von diesem Schlüssel abgeleitet wird, wird an Aequitas gesendet — nie der Schlüssel selbst, nie biometrische Daten.',
  'faq-q1b':'Beweist die Registrierung, dass ich ein einzigartiger echter Mensch bin?','faq-a1b':'Noch nicht vollständig. Der heutige Beweis zeigt kryptografisch, dass du einen bestimmten gerätegebundenen Schlüssel besitzt — das verhindert zuverlässig, dass dasselbe Gerät (oder dieselbe Wallet) sich zweimal registriert, kann aber aktuell nicht unterscheiden, ob dieselbe Person zwei verschiedene physische Geräte besitzt. Eine echte geräteübergreifende biologische Einzigartigkeitsprüfung ist geplant, aber noch nicht ausgeliefert — das sagen wir lieber offen, als zu behaupten, der heutige gerätegebundene Beweis würde mehr garantieren.',
  'faq-q2':'Kann ich mich später mit einer anderen Wallet registrieren?','faq-a2':'Nein. Die Registrierung ist dauerhaft an eine Wallet-Adresse pro Geräteschlüssel gebunden. Dies ist beabsichtigt — es verhindert die erneute Registrierung desselben Geräts und erzwingt eine Wallet pro Geräte-Identität.',
  'faq-q3':'Was passiert, wenn ich mein Handy verliere?','faq-a3':'Deine AEQ bleiben in deiner Wallet — sie sind mit deinem privaten Schlüssel verknüpft, nicht mit deinem Handy. Du kannst weiterhin über MetaMask mit deiner Seed-Phrase auf deine Wallet zugreifen. Die Wallet-Wiederherstellung ist unabhängig von der biometrischen Registrierung.',
  'path-title':'Wähle deinen Weg','path-human-title':'Ich bin ein Mensch','path-human-desc':'Ich möchte mich registrieren, 1.000 AEQ erhalten und dem Grundeinkommensnetzwerk beitreten.','path-human-steps':'1. Aequitas Android App herunterladen<br>2. Mit der Bildschirmsperre deines Geräts entsperren (Fingerabdruck/Gesicht/PIN)<br>3. MetaMask verbinden<br>4. Sofort 1.000 AEQ erhalten',
  'path-node-title':'Ich bin ein Node-Betreiber','path-node-desc':'Ich möchte einen vollständigen Node betreiben, an der Blockproduktion teilnehmen und aus dem 40%-Validator-Pool verdienen.','path-node-steps':'1. Als Mensch registrieren (erforderlich)<br>2. PRIMARY_NODE_URL=https://aequitas.digital setzen<br>3. Auf Railway/Contabo/VPS deployen<br>4. Täglich aus dem Validator-Pool verdienen',
  'path-dev-title':'Ich bin ein Entwickler','path-dev-desc':'Ich möchte auf Aequitas aufbauen, die API integrieren oder zum Protokoll beitragen.','path-dev-steps':'1. EVM-kompatibler JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* Endpunkte<br>4. Metriken: /metrics (Prometheus)',
  'story-flow-title':'AEQ Token-Flussdiagramm','story-topo-title':'Netzwerktopologie — Aktueller Zustand',
  'swap-price-title':'AEQ / tUSD — Live-Preis','swap-price-desc':'Echtzeit-Preis aus Pool-Reserven (x·y=k). Aktualisiert alle 8 Sekunden mit neuen Pool-Daten.','swap-price-empty':'Noch keine Pool-Daten — Liquidität hinzufügen, um das Preisdiagramm zu sehen.',
  'node-guide-lang-note':'Diese Anleitung ist auf Englisch. Eine übersetzte PDF-Version ist in deiner Sprache über den Button oben verfügbar.',
  'k-zkp':'ZKP-System','k-hash':'Hash-System','k-sybil-prot':'Sybil-Schutz',
},
es:{
  'logo-sub':'PRUEBA DE HUMANIDAD','live':'EN VIVO',
  'tab-register':'🔐 Registrar','tab-explorer':'🔍 Explorador','tab-humans':'👥 Humanos','tab-index':'📊 Índice','tab-network':'🌐 Red','tab-protocol':'📜 Protocolo V7','tab-swap':'🔄 Intercambiar',
  'reg-title':'🔐 Regístrate como Humano Verificado',
  'reg-sub':'Únete a la red Aequitas y recibe tu subsidio de Renta Básica Universal de 1,000 AEQ. Único, permanente y completamente gratuito. Ningún dato personal es almacenado.',
  'app-title':'REGISTRO SOLO VÍA APP ANDROID',
  'app-text':'El registro genera una clave criptográfica dentro del hardware seguro de tu teléfono (Secure Enclave / StrongBox), protegida por el propio bloqueo de pantalla del dispositivo — sin sensor adicional, sin datos biométricos jamás producidos, procesados o transmitidos. Una prueba ZK Groth16 demuestra que posees esa clave sin revelarla. Tus 1.000 AEQ se acreditan automáticamente al verificar. Nota: esto demuestra actualmente el control de un dispositivo, no la unicidad biológica entre dispositivos — ver las preguntas frecuentes.',
  's1t':'Clave del Dispositivo','s1d':'El hardware seguro de tu teléfono genera una clave privada protegida por tu bloqueo de pantalla habitual (huella/rostro/PIN, el que ya uses). Sin kit de sensores separado, ningún dato biométrico crudo sale jamás del dispositivo.',
  's2t':'Generación de Prueba ZK','s2d':'Una prueba ZK Groth16 compromete tu clave de dispositivo en un compromiso y nullifier únicos sin revelar la clave misma. Esto demuestra criptográficamente que posees la clave de este dispositivo — ver las preguntas frecuentes para lo que esto garantiza y lo que no.',
  's3t':'Conectar Wallet','s3d':'La app abre MetaMask en esta página · conecta tu wallet Ethereum · la prueba está criptográficamente vinculada a tu dirección',
  's4t':'1,000 AEQ Acreditados','s4d':'Registro confirmado en el BlockDAG de Aequitas en 1 segundo · 1,000 AEQ acreditados instantáneamente · tu identidad queda permanentemente registrada',
  'priv-bar':'🔒 Clave criptográfica vinculada al dispositivo · Groth16 ZKP · Los datos nunca salen del dispositivo · Un registro por dispositivo',
  'conn-wallet':'WALLET CONECTADA','proof-recv':'⚡ PRUEBA ZK RECIBIDA','proof-hint':'Conecta wallet para registrar',
  'btn-conn':'🦊 CONECTAR METAMASK','btn-reg':'🔐 REGISTRAR ON-CHAIN',
  'btn-wc':'🔗 CONECTAR WALLETCONNECT',
  'reg-log-hint':'// Abre la App Android Aequitas para generar tu prueba, luego regresa aquí...',
  'reg-details':'Detalles del Registro','k-network':'Red','k-chainid':'ID de Cadena','k-grant':'Subsidio UBI',
  'k-fee':'Tarifa de Gas','free':'GRATIS — completamente sin gas','k-limit':'Registros','k-limit-v':'Una vez por dispositivo · permanente · inmutable',
  'k-bio':'Clave del Dispositivo','never-stored':'Nunca sale de tu dispositivo — no se genera ni almacena ningún dato biométrico',
  'k-proof':'Sistema de Prueba','k-conf':'Confirmación','k-conf-v':'En 1 segundo (1 bloque)',
  'k-sybil':'Protección Sybil','k-sybil-v':'Una identidad por dispositivo · bloqueo permanente (vinculado al dispositivo, aún no al cuerpo)',
  'live-stats':'Estadísticas de Cadena en Vivo',
  's-height':'Altura de Bloque',
  's-humans':'Humanos Verificados','s-humans-sub':'Prueba ZK vinculada al dispositivo · un registro por dispositivo',
  's-supply':'Suministro Total','s-supply-sub':'Siempre = Humanos × 1,000 AEQ',
  's-index':'Índice Aequitas','s-index-sub':'0 = igualdad perfecta · 100 = desigualdad máxima',
  's-uptime':'Tiempo Activo','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'Prueba de Humanidad','ib-poh-t':'Cada titular de AEQ prueba criptográficamente el control de una clave vinculada a su dispositivo mediante una prueba ZK Groth16. Sin bots, sin corporaciones, sin IA. Nunca se transmiten datos biométricos, solo una prueba matemática. Esto vincula hoy un registro por dispositivo, no todavía por persona única — ver las preguntas frecuentes.',
  'ib-fair':'Distribución Radicalmente Justa','ib-fair-t':'Cada humano verificado recibe exactamente 1,000 AEQ al registrarse. Sin pre-minado, sin asignación a fundadores. El suministro total siempre equivale a humanos verificados × 1,000.',
  'ib-dag':'Arquitectura BlockDAG','ib-dag-t':'Múltiples bloques pueden producirse simultáneamente y fusionarse. Mayor rendimiento, menor latencia que las blockchains lineales.',
  'ib-gas':'Verdaderamente Sin Gas','ib-gas-t':'El registro y las transferencias no cuestan nada. No se necesita ETH, BNB ni MATIC. Sin cuenta bancaria ni tarjeta de crédito.',
  'recent-blocks':'Bloques Recientes','blocks-desc':'MERGE = múltiples padres fusionados (BlockDAG). TX = transacción de registro. Tiempo de bloque: __BT__.',
  'loading':'Cargando bloques...','net-info':'Información de Red','k-chain':'Nombre de Cadena','k-symbol':'Símbolo','k-btime':'Tiempo de Bloque',
  'k-cons':'Consenso','k-nodes':'Nodos Activos','k-storage':'Almacenamiento','add-mm':'🦊 AGREGAR A METAMASK','k-dec':'Decimales',
  'btn-add-mm':'+ AGREGAR RED AEQUITAS',
  'phil':'"El dinero existe porque las personas existen.<br>Nada más, nada menos."','phil-sub':'— EL PRINCIPIO AEQUITAS —',
  'humans-title':'Humanos Verificados en Aequitas Chain',
  'h-what':'¿Qué es un Humano Verificado?','h-what-t':'Un Humano Verificado es actualmente una dirección wallet que ha probado criptográficamente el control de una clave vinculada a un dispositivo específico. La clave se genera dentro del hardware seguro del teléfono, protegida por el bloqueo de pantalla del dispositivo — sin kit de sensores separado. Solo se transmite una prueba ZK Groth16, ningún dato biométrico sale del dispositivo. Esto verifica hoy un dispositivo, no necesariamente todavía una persona única — ver las preguntas frecuentes.',
  'h-zkp':'Sistema de Prueba ZK','h-zkp-t':'Aequitas usa Groth16 en BN128 — misma curva que Ethereum y Zcash. ~200 bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). El nullifier está vinculado a este dispositivo: perder tu teléfono no crea una segunda identidad en él, pero otro dispositivo puede seguir registrándose por separado. Ningún material de clave se revela ni almacena en el servidor.',
  'h-sybil':'Resistencia Sybil — Estado Actual','h-sybil-t':'El nullifier actual se deriva de una clave de hardware vinculada al dispositivo — bloquea de forma fiable el doble registro del mismo dispositivo o la misma wallet. Todavía no detecta que la misma persona se registre desde un segundo dispositivo físico. Cerrar esa brecha requiere una verificación real de unicidad biológica entre dispositivos, planificada para después del lanzamiento beta, no implementada hoy.',
  'h-global':'Inclusión Financiera Global','h-global-t':'Sin cuenta bancaria, tarjeta de crédito ni criptomoneda previa. Solo un smartphone Android con el bloqueo de pantalla (huella/rostro/PIN) que ya usas.',
  'h-bio-hw':'Hoja de Ruta de Verificación de Identidad','h-bio-hw-t':'Hoy (beta): una clave criptográfica vinculada al dispositivo, etiquetada honestamente como tal en lugar de como vinculada al cuerpo. Planificado (después de la beta): una verificación real de unicidad biológica entre dispositivos — especificada, construida y auditada de forma independiente antes de hacer una afirmación más fuerte sobre resistencia Sybil.',
  'reg-humans':'Humanos Registrados','h-desc':'Cada dirección ha probado mediante prueba ZK el control de una clave criptográfica vinculada a dispositivo y recibió exactamente 1.000 AEQ. Permanente, inmutable, on-chain. Ver las preguntas frecuentes sobre lo que "vinculado al dispositivo" significa hoy para la resistencia Sybil.',
  'no-humans':'No hay humanos registrados aún.\n\n¡Descarga la App Android Aequitas y sé el primero!',
  'reg-stats':'Estadísticas del Registro','total-humans':'Total de Humanos',
  'idx-title':'Índice Aequitas — Puntuación de Igualdad Económica en Tiempo Real',
  'idx-desc':'El Índice Aequitas mide la desigualdad económica de todos los humanos verificados en tiempo real. Se calcula desde el coeficiente Gini de la distribución de saldos on-chain. 0 = igualdad perfecta. 100 = desigualdad máxima.',
  'gini-what-title':'¿Qué es el Coeficiente de Gini?','gini-what-text':'Desarrollado por el estadístico italiano Corrado Gini (1912). Mide la distribución de la riqueza comparando los saldos reales con una línea base hipotéticamente igualitaria — visualizado como la curva de Lorenz. Escala: 0 (todos tienen lo mismo) a 1 (una persona lo tiene todo). Usado por el Banco Mundial, la OCDE y la ONU para comparar países. Valores de referencia: Bitcoin ≈ 0,85 · Sudáfrica (récord mundial) ≈ 0,63 · EE.UU. ≈ 0,41 · Alemania ≈ 0,31 · Escandinavia ≈ 0,27 · Objetivo a largo plazo de Aequitas: Gini por debajo de 0,30 — comparable a los países escandinavos, impuesto por el límite de riqueza.',
  'gini-calc-title':'¿Cómo se calcula el Índice Aequitas?','gini-calc-text':'Se recopilan todos los saldos de AEQ de humanos verificados. La fórmula calcula la diferencia absoluta media entre cada par posible de saldos, normalizada por la población al cuadrado (n²) y el saldo medio (x̄). El resultado 0–1 multiplicado por 100 = Índice Aequitas. Se actualiza on-chain tras cada registro, demurrage mensual, pago de pool y evento de límite de riqueza.',
  'gini-why-title':'¿Por qué Gini — y no una métrica más simple?','gini-why-text':'Una simple relación rico-pobre es fácil de manipular: 10.000 wallets podrían mostrar una dispersión baja pero con el 90% del AEQ concentrado en 100 manos — Gini detecta esto, una relación simple no. El coeficiente captura la distribución completa entre todos los humanos verificados en un único número auditable. Aequitas publica esto on-chain — transparente, a prueba de manipulaciones, verificable globalmente. Es la señal principal para las transiciones automáticas de fase, la calibración del límite de riqueza y la intensidad de redistribución.',
  'curr-idx':'Índice Actual','bar-0':'0 — Igualdad Perfecta','bar-100':'100 — Máx. Desigualdad','wcap-lbl':'Límite de Riqueza Actual:','wcap-mult':'Multiplicador:','wcap-avg':'Saldo promedio:',
  'gini':'Coeficiente Gini','gini-desc':'0 = igual · 1 = desigual',
  'supply-desc':'Siempre = Humanos × 1,000 AEQ',
  'phase':'Fase del Protocolo','phase-desc':'Avanza automáticamente por recuento humano',
  'humans-desc':'Registros verificados por ZK vinculados a dispositivo',
  'pools-title':'Pools de Redistribución',
  'pools-desc':'Cada tarifa de swap, cargo de demurrage y desbordamiento del límite de riqueza se divide automáticamente entre cuatro pools. Sin intervención manual. Todos los pools pagan diariamente.',
  'vel-pool':'Pool Validadores','vel-pool-desc':'40% de todas las tarifas → operadores de nodos que aseguran la red',
  'liq-pool':'Pool Liquidez','liq-pool-desc':'30% de todas las tarifas → proveedores de liquidez, proporcional a participaciones LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% de todas las tarifas → todos los humanos verificados por igual, cada 24 horas',
  'treasury':'Tesorería','treasury-desc':'10% de todas las tarifas → desarrollo y mantenimiento del protocolo',
  'phases-title':'Fases del Protocolo',
  'phases-desc':'En Fase 0, el límite de riqueza usa un multiplicador de arranque: max(5, min(N, 25))× saldo promedio. Con 1–4 humanos: 5× promedio. Cada nuevo humano añade 1×. A 25+ humanos: fijado permanentemente en 25×. Fase 1+ mantiene 25× fijo. Todas las transiciones son automáticas — sin voto de gobernanza, sin clave de administrador.',
  'p0':'Bootstrap · &lt;100 humanos · Límite de Riqueza: max(5,min(N,25))× promedio · Deslizamiento 5×→25× hasta el 25.º humano · Actualmente activo',
  'p1':'Crecimiento · 100–10,000 humanos · Límite de Riqueza: 25× saldo promedio',
  'p2':'Estabilidad · 10,000–1M humanos · Límite de Riqueza: 25× saldo promedio',
  'p3':'Madurez · 1M+ humanos · Límite de Riqueza: 25× saldo promedio',
  'wealth-cap-explain':'El Límite de Riqueza en Fase 0 (Bootstrap) usa max(5, min(N, 25))× saldo promedio, donde N = humanos registrados. 1–4 humanos: 5× promedio. Cada nuevo humano añade 1×. 25+ humanos: bloqueado en 25× permanentemente. El límite siempre se escala con el saldo promedio actual.',
  'btn-download-app':'DESCARGAR APP AEQUITAS',
  'swap-title':'🔄 Intercambiar AEQ ↔ tUSD','swap-sub':'Intercambia AEQ por tUSD (un dólar de prueba simulado) a través del pool de liquidez nativo. Se aplica una comisión del 0,1% solo a los intercambios — las transferencias ordinarias de AEQ entre personas permanecen completamente gratuitas.',
  'swap-priv-bar':'🔒 Solo 0,1% de comisión de swap · Transferencias AEQ a AEQ gratuitas · tUSD es una moneda de prueba sin valor real',
  'swap-your-aeq':'Tu AEQ','swap-your-tusd':'Tu tUSD',
  'swap-fee-est':'Comisión de protocolo (0,1%)','swap-details-hdr':'Detalles del Swap',
  'swap-out-lbl':'Recibes (est.)','swap-impact-lbl':'Impacto en precio','swap-rate-lbl':'Tipo de cambio',
  'swap-depth-lbl':'Composición del Pool','amm-title':'x × y = k — AMM de Producto Constante',
  'amm-text':'Cuando intercambias AEQ por tUSD, la reserva de AEQ crece y la de tUSD decrece — su producto siempre permanece igual a k. Swaps más grandes causan mayor impacto en precio. La comisión del 0,1% se descuenta antes de aplicar la fórmula.',
  'swap-btn-conn':'🦊 CONECTAR METAMASK','swap-btn-go':'🔄 INTERCAMBIAR',
  'swap-log-hint':'// Conecta tu wallet para intercambiar...',
  'swap-no-liquidity':'¿Sin tUSD todavía?','swap-faucet-desc':'Los humanos registrados pueden reclamar tUSD de prueba una vez','swap-btn-faucet':'💧 RECLAMAR tUSD DE PRUEBA',
  'swap-addliq-title':'Proporcionar Liquidez','swap-addliq-desc':'Sé el primero en depositar — tu ratio establece el precio inicial.','swap-btn-addliq':'💧 AGREGAR LIQUIDEZ',
  'swap-lp-title':'Tu Posición LP','swap-lp-share':'Participación del Pool','swap-lp-withdrawable':'Retirable',
  'swap-lp-pct-label':'% de tu posición','swap-lp-youget':'Recibirás','swap-btn-removeliq':'🔥 RETIRAR LIQUIDEZ',
  'swap-pool-title':'AEQ / tUSD — Estado del Pool',
  'swap-pool-aeq':'Reserva AEQ','swap-pool-tusd':'Reserva tUSD','swap-pool-price':'Precio Spot',
  'swap-fee-bps':'Comisión de Swap',
  'swap-pools-addr-title':'Direcciones de Pools Tokenomics','pools-addr-title':'Direcciones de Contrato de Pools',
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
  'usp-c3-title':'Accesible para todos','usp-c3-desc':'Sin cuenta bancaria, tarjeta de crédito ni documento de identidad, sin hardware adicional que comprar — solo el bloqueo de pantalla que tu teléfono Android ya incluye.',
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
  'story-title':'La Historia de Aequitas','story-text':'<p>El año es 2009. Satoshi Nakamoto lanza Bitcoin. Por primera vez el valor puede transferirse sin bancos. Una revolución genuina. Pero casi de inmediato algo sale mal.</p><p>Los primeros mineros acumulan millones de monedas a costo casi cero. Para 2021, el 1% superior controla más del 90% de todo el Bitcoin. El coeficiente Gini estimado de Bitcoin supera 0.85 — más alto que cualquier país en la Tierra.</p><p><span style="color:var(--gold)">Aequitas</span> fue creado para responder: <em style="color:var(--gold)">"¿Cómo sería una criptomoneda diseñada para ser justa con todo ser humano?"</em></p><p>La respuesta: <strong style="color:var(--text)">El dinero existe porque las personas existen. Por lo tanto, cada persona debería tener una parte igual del dinero por el simple hecho de ser humana.</strong></p><p><em style="color:var(--gold)">"El dinero existe porque las personas existen. Nada más, nada menos."</em></p>',
  'nodes-title':'Nodos Activos — Topología Actual de la Red',
  'nodes-desc':'La red Aequitas opera actualmente en múltiples nodos distribuidos geográficamente (número actual arriba). Todos participan en la producción de bloques, sincronización de estado y servicio de API. Se comunican peer-to-peer via libp2p y sincronizan el estado de bloques via HTTP. La red está diseñada para soportar nodos adicionales.',
  'run-node-title':'Ejecuta Tu Propio Nodo — Ayuda a Asegurar la Red',
  'run-node-desc':'Cualquiera puede ejecutar un nodo de Aequitas — sin permiso, sin stake, sin solicitud requerida. Los nodos participan en la producción de bloques y validan el registro humano. Los operadores de nodos ganan una parte de las comisiones del protocolo via el Pool de Validadores (40% de todas las comisiones de swap, distribuidas diariamente).',
  'bootstrap-title':'Conectar un Nuevo Nodo','bootstrap-desc':'Para ejecutar tu propio nodo, establece PRIMARY_NODE_URL=https://aequitas.digital en tu entorno. Tu nodo sincronizará automáticamente el estado completo de la cadena.',
  'tech-title':'Especificaciones Técnicas','mm-config':'Configuración MetaMask',
  'k-lang':'Idioma','k-src':'Código Fuente','evm-yes':'Sí — JSON-RPC /rpc · Compatible con MetaMask',
  'proto-label':'Protocolo Aequitas V7 — Documentación Técnica',
  'ca-title':'Contratos y Direcciones de Red','ca-text':'Cadena: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier (verificador Groth16 on-chain): 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (contrato principal): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 es la única fuente de verdad para toda la economía Aequitas. Cada saldo AEQ, cada registro humano, cada pago UBI y cada aplicación del límite de riqueza está gobernado por este único contrato inmutable — desplegado en Aequitas Chain, una blockchain personalizada compatible con EVM que ejecuta un motor de consenso BlockDAG. No hay clave de administrador, no hay proxy de actualización, no hay votación de gobernanza que pueda cambiar una sola línea de su lógica. El código que funciona hoy es el código que funcionará en diez años.<br><br>El contrato BioVerifier recibe pruebas de conocimiento cero Groth16 generadas completamente en el dispositivo Android del usuario. Verifica matemáticamente on-chain en ~10 ms que un nuevo registrante es un ser humano único y vivo — sin conocer jamás su nombre, identidad o datos biométricos. Esto es lo que hace posible el registro sin gas y sin inversión: la prueba es lo único que sale del dispositivo.<br><br>Juntos, estos dos contratos hacen posible algo que nunca ha existido en ningún sistema monetario de la historia: una oferta monetaria cuyas reglas — quién la recibe, cuánto existe, cómo se redistribuye — no puede ser alterada por ninguna persona, empresa o gobierno. Jamás.',
  'ib-poh':'Prueba de Humanidad','ib-poh-t':'Cada titular de AEQ prueba criptográficamente el control de una clave vinculada a su dispositivo. Sin bots, sin corporaciones, sin IA. Nunca se transmiten datos biométricos, solo una prueba matemática. Esto vincula hoy un registro por dispositivo — ver las preguntas frecuentes sobre el estado actual de la resistencia Sybil.',
  'ib-fair':'Distribución Radicalmente Justa','ib-fair-t':'Cada humano verificado recibe exactamente 1.000 AEQ al registrarse. Sin pre-minado, sin asignación a fundadores, sin rondas de inversores. El suministro total es siempre y exactamente igual al número de humanos verificados multiplicado por 1.000. Esto se aplica matemáticamente, no por política.',
  'ib-dag':'Arquitectura BlockDAG','ib-dag-t':'A diferencia de las blockchains tradicionales donde solo puede existir un bloque por altura, Aequitas usa una estructura DAG. Múltiples bloques pueden producirse simultáneamente por diferentes nodos y luego fusionarse en el DAG. Esto permite mayor rendimiento, menor latencia y elimina cuellos de botella. Los eventos de fusión se marcan con una insignia especial en el explorador.',
  'ib-gas':'Verdaderamente Sin Gas','ib-gas-t':'Todos los registros y transferencias de AEQ no cuestan absolutamente nada. No se necesita ETH, BNB ni MATIC. Sin tarjeta de crédito, sin cuenta bancaria, sin criptomoneda previa. El relayer cubre todos los costos de transacción. Si eres humano con un smartphone, puedes participar independientemente de tu situación económica.',
  'h-what':'¿Qué es un Humano Verificado?','h-what-t':'Un Humano Verificado es actualmente una dirección wallet que ha probado criptográficamente el control de una clave vinculada a un dispositivo específico. La clave se genera en el hardware seguro del teléfono, protegida por el bloqueo de pantalla del dispositivo — sin kit de sensores separado. Solo se transmite una prueba ZK Groth16, ningún dato biométrico sale del dispositivo. Esto verifica hoy un dispositivo, no necesariamente todavía una persona única — ver las preguntas frecuentes.',
  'h-zkp':'Sistema de Prueba ZK','h-zkp-t':'Aequitas usa Groth16 en BN128 — misma curva que Ethereum y Zcash. ~200 bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). El nullifier está vinculado a este dispositivo: perder tu teléfono no crea una segunda identidad en él, pero otro dispositivo puede seguir registrándose por separado. Ningún material de clave se revela ni almacena en el servidor.',
  'h-sybil':'Resistencia Sybil — Estado Actual','h-sybil-t':'El nullifier actual se deriva de una clave de hardware vinculada al dispositivo — bloquea de forma fiable el doble registro del mismo dispositivo o la misma wallet. Todavía no detecta que la misma persona se registre desde un segundo dispositivo físico. Cerrar esa brecha requiere una verificación real de unicidad biológica entre dispositivos, planificada para después de la beta.',
  'h-global':'Inclusión Financiera Global','h-global-t':'1.400 millones de adultos en todo el mundo no tienen cuenta bancaria. Aequitas solo requiere un smartphone Android con sensor biométrico — un dispositivo que más de 3.000 millones de personas ya poseen. Sin cuenta bancaria, sin tarjeta de crédito, sin criptomoneda previa, sin documento de identidad. Simplemente ser humano es suficiente.',
  'h-bio-hw':'Hoja de Ruta de Verificación de Identidad','h-bio-hw-t':'Hoy (beta): una clave criptográfica vinculada al dispositivo, etiquetada honestamente como tal en lugar de como vinculada al cuerpo. Planificado (después de la beta): una verificación real de unicidad biológica entre dispositivos — especificada, construida y auditada de forma independiente antes de hacer una afirmación más fuerte sobre resistencia Sybil.',
  'poa-title':'1. PRUEBA DE VIDA — Recuperación de Saldos Inactivos','poa-text':'<p>¿Qué pasa con AEQ cuando las personas mueren o quedan permanentemente incapacitadas? En Bitcoin, las wallets perdidas significan suministro perdido permanentemente. Aequitas soluciona esto mediante un sistema de recuperación por inactividad de múltiples etapas: si una wallet no muestra actividad durante un período prolongado, su saldo se devuelve gradualmente a la comunidad a través del pool UBI.</p>',
  'poa-box':'Año 0–2: Uso normal — sin restricciones<br>Año 2: Aviso 1 — el Guardian puede responder en nombre<br>Año 2+60d: Aviso 2 — urgencia creciente<br>Año 2+120d: Aviso 3 — aviso final<br>Año 2+180d: AEQ movido a CUSTODIA personal (aún recuperable)<br>Año 4: Si aún inactivo — CUSTODIA liberada al Pool UBI',
  'guard-title':'2. SISTEMA GUARDIAN — Salvaguarda Humana','guard-text':'<p>¿Y si alguien está hospitalizado, encarcelado o de algún modo incapaz de acceder a su dispositivo por meses? El sistema Guardian permite a una persona de confianza — otro humano verificado — confirmar que el propietario de la wallet sigue vivo. El Guardian tiene estrictamente cero acceso financiero: solo puede llamar una función que reinicia el temporizador de inactividad.</p>',
  'guard-box':'1 Guardian por humano · debe ser un humano verificado en Aequitas<br>Guardian SOLO puede llamar confirmAlive() — cero derechos de transacción<br>Guardian NO PUEDE mover fondos, transferir AEQ ni acceder a la wallet<br>Máximo 3 tutelados por Guardian (evita centralización de confianza)<br>Bloqueo de 7 días en asignación de Guardian (evita asignación forzada)<br>No se permiten relaciones Guardian circulares',
  'dem-title':'3. DEMURRAGE — Mecanismo Anti-Acaparamiento',
  'dem-box':'Tasa: 0,5% por mes después de 3 meses de inactividad (continuo, no escalonado)<br>El reloj se reinicia automáticamente con cualquier transferencia, swap o acción de liquidez<br>AEQ decaído redistribuido a los cuatro pools — nunca destruido<br>Aviso de 14 días mostrado una vez · aviso de 7 días repetido en cada sesión activa',
  'dem-text':'<p>El demurrage es un costo de tenencia sobre el dinero — una tasa de interés negativa que hace costoso acumular y atractivo circular. El experimento de Wörgl (Austria, 1932) usó una moneda con demurrage y redujo el desempleo local un 25% en un año. El Banco Central de Austria lo cerró precisamente porque funcionó demasiado bien. El Chiemgauer (Alemania, 2003) opera según el mismo principio con éxito desde hace más de 20 años.</p>',
  'cap-title':'4. LÍMITE DE RIQUEZA — Aplicación de Justicia Matemática','cap-box':'Límite bootstrap: max(5,min(N,25))× saldo promedio actual<br>1–4 humanos: 5× · +1× por humano · 25+: 25× permanente<br>Se aplica a TODAS las direcciones excepto las 4 pools del protocolo<br>Exceso AEQ redistribuido instantáneamente · Sin intervención manual',
  'ubi-title':'5. RENTA BÁSICA UNIVERSAL — Redistribución Diaria','ubi-box':'Fuentes de ingresos del Pool UBI:<br>· 20% de todas las comisiones de swap del pool AMM AEQ↔tUSD<br>· Desbordamiento de la aplicación del límite de riqueza<br>· Cargos de demurrage de cuentas inactivas<br>· Custodia inactiva liberada después de 4 años<br><br>Distribución: Cada 24 horas, todo el saldo del pool UBI se divide igualmente entre todos los humanos verificados registrados. El pool se reinicia a cero y comienza a llenarse inmediatamente de la actividad continua del protocolo.',
  'inf-title':'6. SIN INFLACIÓN ALGORÍTMICA — Fórmula de Suministro Fijo','inf-box':'El ÚNICO evento que crea nuevo AEQ: un nuevo humano verificado se registra.<br><br>Suministro Total = Humanos Verificados × 1.000 AEQ<br><br>Esto no es una política — es aplicado por el protocolo. Ningún administrador puede acuñar AEQ adicional, ningún voto de gobernanza puede cambiar la emisión. AEQ es la única criptomoneda donde el suministro total está determinado únicamente por el número de humanos vivos verificados.',
  'explore-title':'Explorar Aequitas',
  'expl-score':'Puntuación de Igualdad','expl-score-d':'Coeficiente Gini en vivo · Índice Aequitas · distribución de riqueza en tiempo real',
  'expl-economy':'UBI y Pools de Redistribución','expl-economy-d':'Cuenta regresiva UBI diaria · 4 pools on-chain · demurrage · Fases del Protocolo',
  'expl-charts':'Gráficos e Historial','expl-charts-d':'Historial Gini · curva de Lorenz · slider bootstrap del límite de riqueza · La historia de Aequitas',
  'expl-v7':'Documentación Protocolo V7','expl-v7-d':'Contrato AequitasV7 · 6 mecanismos · prueba ZK · límite de riqueza · demurrage · código inmutable',
  'expl-explorer':'Explorador de Bloques','expl-explorer-d':'BlockDAG en vivo · haz clic en cualquier bloque para ver validador, hash, transacciones, hashes padres',
  'swap-sell-label':'Vender','swap-receive-label':'Recibir',
  'expl-network':'Red y Nodos','expl-network-d':'Topología de nodos · ejecutar tu propio nodo · especificaciones técnicas · Chain ID 1926',
  'guard-title':'🛡 Sistema Guardian','guard-my-lbl':'Mi Guardian','guard-none':'Ninguno',
  'guard-set-lbl':'Establecer / Cambiar Guardian','guard-set-hint':'Debe ser un humano registrado en Aequitas · Bloqueo de 7 días · El Guardian solo puede confirmar tu vitalidad, no acceder a fondos · Máximo 3 protegidos por Guardian',
  'guard-confirm-lbl':'Confirmar Vivo (Como Guardian)','guard-confirm-hint':'Si tu protegido no puede acceder a su wallet, confirma su vitalidad para evitar que sus fondos vayan al escrow después de 910 días de inactividad.','guard-recover-btn':'🔓 RECUPERAR DEL ESCROW',
  'faq-title':'❓ Preguntas Frecuentes','faq-q1':'¿Están seguros mis datos biométricos?','faq-a1':'Sí. Ningún dato biométrico es capturado, procesado ni transmitido por Aequitas. El bloqueo de pantalla de tu teléfono solo da acceso a una clave aleatoria generada y almacenada en su hardware seguro. Únicamente se envía una prueba matemática derivada de esa clave — nunca la clave en sí, nunca datos biométricos.',
  'faq-q1b':'¿El registro demuestra que soy una persona real y única?','faq-a1b':'Todavía no del todo. La prueba actual demuestra criptográficamente que controlas la clave segura de un dispositivo específico — evita de forma fiable que ese mismo dispositivo (o wallet) se registre dos veces, pero hoy no puede distinguir si la misma persona posee dos dispositivos físicos distintos. Una verificación real de unicidad biológica entre dispositivos está planificada, no implementada todavía — preferimos decirlo con claridad antes que exagerar lo que la prueba actual garantiza.',
  'faq-q2':'¿Puedo registrarme con una wallet diferente más adelante?','faq-a2':'No. El registro está vinculado permanentemente a una dirección de wallet por clave de dispositivo. Esto es intencional — evita volver a registrar el mismo dispositivo y garantiza una wallet por identidad de dispositivo.',
  'faq-q3':'¿Qué pasa si pierdo mi teléfono?','faq-a3':'Tu AEQ permanece en tu wallet — está vinculado a tu clave privada, no a tu teléfono. Puedes acceder a tu wallet a través de MetaMask con tu frase semilla. La recuperación de la wallet es independiente del registro biométrico.',
  'path-title':'Elige Tu Camino','path-human-title':'Soy un Humano','path-human-desc':'Quiero registrarme, recibir 1.000 AEQ y unirme a la red de ingreso básico.','path-human-steps':'1. Descargar la App Android de Aequitas<br>2. Desbloquear con el bloqueo de pantalla de tu dispositivo (huella/rostro/PIN)<br>3. Conectar MetaMask<br>4. Recibir 1.000 AEQ al instante',
  'path-node-title':'Soy un Operador de Nodo','path-node-desc':'Quiero ejecutar un nodo completo, participar en la producción de bloques y ganar del pool de validadores del 40%.','path-node-steps':'1. Registrarse como humano (obligatorio)<br>2. Establecer PRIMARY_NODE_URL=https://aequitas.digital<br>3. Desplegar en Railway/Contabo/VPS<br>4. Ganar diariamente del pool de validadores',
  'path-dev-title':'Soy un Desarrollador','path-dev-desc':'Quiero construir sobre Aequitas, integrar la API o contribuir al protocolo.','path-dev-steps':'1. JSON-RPC compatible con EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Métricas: /metrics (Prometheus)',
  'story-flow-title':'Diagrama de Flujo del Token AEQ','story-topo-title':'Topología de Red — Estado Actual',
  'swap-price-title':'AEQ / tUSD — Precio en Vivo','swap-price-desc':'Precio en tiempo real derivado de las reservas del pool (x·y=k). Se actualiza cada 8 segundos con nuevos datos del pool.','swap-price-empty':'Sin datos del pool aún — añade liquidez para ver el gráfico de precios.',
  'node-guide-lang-note':'Esta guía está en inglés. Una traducción en PDF está disponible en tu idioma con el botón de arriba.',
  'k-zkp':'Sistema ZKP','k-hash':'Sistema Hash','k-sybil-prot':'Protección Sybil',
},
ru:{
  'logo-sub':'ДОКАЗАТЕЛЬСТВО ЧЕЛОВЕЧНОСТИ','live':'ОНЛАЙН',
  'tab-register':'🔐 Регистрация','tab-explorer':'🔍 Проводник','tab-humans':'👥 Люди','tab-index':'📊 Индекс','tab-network':'🌐 Сеть','tab-protocol':'📜 Протокол V7','tab-swap':'🔄 Обмен',
  'reg-title':'🔐 Зарегистрируйтесь как Верифицированный Человек',
  'reg-sub':'Присоединитесь к сети Aequitas и получите 1 000 AEQ в качестве Универсального Базового Дохода. Однократно, постоянно и полностью бесплатно. Никакие личные данные никогда не сохраняются.',
  'app-title':'РЕГИСТРАЦИЯ ТОЛЬКО ЧЕРЕЗ ANDROID-ПРИЛОЖЕНИЕ',
  'app-text':'Регистрация создаёт криптографический ключ внутри защищённого оборудования вашего телефона (Secure Enclave / StrongBox), защищённый экраном блокировки устройства — без отдельного датчика, без каких-либо биометрических данных, которые создаются, обрабатываются или передаются. Доказательство Groth16 ZK подтверждает владение этим ключом, не раскрывая его. 1 000 AEQ зачисляются автоматически после верификации. Важно: это подтверждает контроль над одним устройством, а не биологическую уникальность между устройствами — см. вопросы и ответы.',
  's1t':'Ключ Устройства','s1d':'Защищённое оборудование вашего телефона создаёт приватный ключ, защищённый вашей обычной блокировкой экрана (отпечаток/лицо/PIN — что вы уже используете). Без отдельного набора датчиков, никакие необработанные биометрические данные никогда не покидают устройство.',
  's2t':'Создание ZK-Доказательства','s2d':'Доказательство Groth16 ZK фиксирует ваш ключ устройства в единый commitment и nullifier, не раскрывая сам ключ. Это криптографически доказывает владение ключом этого устройства — см. вопросы и ответы о том, что это гарантирует, а что нет.',
  's3t':'Подключение Кошелька','s3d':'Приложение открывает MetaMask на этой странице · подключите кошелёк Ethereum · доказательство криптографически привязано к вашему адресу',
  's4t':'1 000 AEQ Зачислены','s4d':'Регистрация подтверждена на BlockDAG Aequitas за 1 секунду · 1 000 AEQ зачислены мгновенно · личность навсегда записана как верифицированный человек',
  'priv-bar':'🔒 Криптографический ключ, привязанный к устройству · Groth16 ZKP · Данные никогда не покидают устройство · Одна регистрация на устройство',
  'conn-wallet':'ПОДКЛЮЧЁННЫЙ КОШЕЛЁК','proof-recv':'⚡ ZK-ДОКАЗАТЕЛЬСТВО ПОЛУЧЕНО','proof-hint':'Подключите кошелёк для регистрации',
  'btn-conn':'🦊 ПОДКЛЮЧИТЬ METAMASK','btn-reg':'🔐 ЗАРЕГИСТРИРОВАТЬ ОН-ЧЕЙН',
  'btn-wc':'🔗 ПОДКЛЮЧИТЬ WALLETCONNECT',
  'reg-log-hint':'// Откройте Android-приложение Aequitas для создания доказательства, затем вернитесь сюда...',
  'reg-details':'Детали Регистрации','k-network':'Сеть','k-chainid':'ID Цепи','k-grant':'Субсидия UBI',
  'k-fee':'Комиссия Gas','free':'БЕСПЛАТНО — полностью без комиссий','k-limit':'Регистрации','k-limit-v':'Один раз на устройство · постоянно · неизменно',
  'k-bio':'Ключ Устройства','never-stored':'Никогда не покидает устройство — биометрические данные не создаются и не хранятся',
  'k-proof':'Система Доказательств','k-conf':'Подтверждение','k-conf-v':'В течение 1 секунды (1 блок)',
  'k-sybil':'Защита от Сибилл','k-sybil-v':'Одна идентичность на устройство · постоянная блокировка (привязка к устройству, ещё не к телу)',
  'live-stats':'Статистика Цепи в Реальном Времени',
  's-height':'Высота Блока',
  's-humans':'Верифицированные Люди','s-humans-sub':'ZK-доказательство, привязанное к устройству · одна регистрация на устройство',
  's-supply':'Общий Объём','s-supply-sub':'Всегда = Люди × 1 000 AEQ',
  's-index':'Индекс Aequitas','s-index-sub':'0 = идеальное равенство · 100 = максимальное неравенство',
  's-uptime':'Время Работы','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'Доказательство Человечности','ib-poh-t':'Каждый владелец AEQ криптографически доказывает владение ключом, привязанным к устройству. Никаких ботов, корпораций, ИИ. Биометрические данные никогда не передаются — только математическое доказательство. Это привязывает сегодня одну регистрацию к устройству, ещё не к уникальному человеку — см. вопросы и ответы.',
  'ib-fair':'Радикально Справедливое Распределение','ib-fair-t':'Каждый верифицированный человек получает ровно 1 000 AEQ при регистрации. Никакого предварительного майнинга, никаких аллокаций основателям. Общий объём всегда равен верифицированные люди × 1 000.',
  'ib-dag':'Архитектура BlockDAG','ib-dag-t':'Несколько блоков могут производиться одновременно и объединяться. Более высокая пропускная способность, меньшая задержка.',
  'ib-gas':'Действительно Без Комиссий','ib-gas-t':'Регистрация и переводы AEQ не стоят ничего. ETH, BNB или MATIC не требуются. Банковский счёт и кредитная карта не нужны.',
  'recent-blocks':'Последние Блоки','blocks-desc':'MERGE = объединение нескольких родителей (BlockDAG). TX = транзакция регистрации. Время блока: __BT__.',
  'loading':'Загрузка блоков...','net-info':'Информация о Сети','k-chain':'Имя Цепи','k-symbol':'Символ','k-btime':'Время Блока',
  'k-cons':'Консенсус','k-nodes':'Активные Ноды','k-storage':'Хранилище','add-mm':'🦊 ДОБАВИТЬ В METAMASK','k-dec':'Десятичные',
  'btn-add-mm':'+ ДОБАВИТЬ СЕТЬ AEQUITAS',
  'phil':'"Деньги существуют потому что существуют люди.<br>Ничего более, ничего менее."','phil-sub':'— ПРИНЦИП AEQUITAS —',
  'humans-title':'Верифицированные Люди в Aequitas Chain',
  'h-what':'Что такое Верифицированный Человек?','h-what-t':'Верифицированный Человек — это сейчас адрес кошелька, криптографически доказавший владение ключом, привязанным к конкретному устройству. Ключ создаётся в защищённом оборудовании телефона, защищён блокировкой экрана устройства — без отдельного набора датчиков. Передаётся только доказательство Groth16 ZK, биометрические данные никогда не покидают устройство. Сегодня это верифицирует устройство, не обязательно ещё уникального человека — см. вопросы и ответы.',
  'h-zkp':'Система ZK-Доказательств','h-zkp-t':'Aequitas использует Groth16 на BN128 — та же кривая, что Ethereum и Zcash. ~200 байт, ~10мс. commitment = keccak256(deviceKey‖wallet). Nullifier привязан к этому устройству: потеря телефона не создаёт вторую идентичность на нём, но другое устройство может зарегистрироваться отдельно. Материал ключа никогда не раскрывается и не хранится на сервере.',
  'h-sybil':'Устойчивость к Sybil — Текущее Состояние','h-sybil-t':'Сегодняшний nullifier получен из аппаратного ключа, привязанного к устройству — он надёжно блокирует повторную регистрацию того же устройства или кошелька. Пока не обнаруживает регистрацию того же человека со второго физического устройства. Закрытие этого пробела требует настоящей межустройственной проверки биологической уникальности, запланированной на период после бета-версии.',
  'h-global':'Глобальная Финансовая Инклюзия','h-global-t':'Банковский счёт, кредитная карта или криптовалюта не требуются. Только Android-смартфон с блокировкой экрана (отпечаток/лицо/PIN), которую вы уже используете.',
  'h-bio-hw':'Дорожная Карта Верификации Личности','h-bio-hw-t':'Сегодня (бета): аппаратный ключ, привязанный к устройству, честно обозначенный как таковой, а не как привязанный к телу. Запланировано (после бета-версии): настоящая межустройственная проверка биологической уникальности — специфицированная, реализованная и независимо проверенная, прежде чем делать более сильное заявление об устойчивости к Sybil.',
  'reg-humans':'Зарегистрированные Люди','h-desc':'Каждый адрес доказал через ZK-доказательство владение криптографическим ключом, привязанным к устройству, и получил ровно 1 000 AEQ. Постоянно, неизменно, он-чейн. См. вопросы и ответы о том, что означает "привязка к устройству" для защиты от Sybil сегодня.',
  'no-humans':'Люди ещё не зарегистрированы.\n\nСкачайте Android-приложение Aequitas и будьте первым!',
  'reg-stats':'Статистика Реестра','total-humans':'Всего Людей',
  'idx-title':'Индекс Aequitas — Оценка Экономического Равенства в Реальном Времени',
  'idx-desc':'Индекс Aequitas измеряет экономическое неравенство всех верифицированных людей в реальном времени. Рассчитывается из коэффициента Джини распределения балансов он-чейн. 0 = идеальное равенство. 100 = максимальное неравенство.',
  'curr-idx':'Текущий Индекс','bar-0':'0 — Идеальное Равенство','bar-100':'100 — Макс. Неравенство','wcap-lbl':'Текущий Потолок Богатства:','wcap-mult':'Множитель:','wcap-avg':'Средний баланс:',
  'gini':'Коэффициент Джини','gini-desc':'0 = равно · 1 = неравно',
  'supply-desc':'Всегда = Люди × 1 000 AEQ',
  'phase':'Фаза Протокола','phase-desc':'Автоматически по количеству людей',
  'humans-desc':'ZK-верифицированные регистрации, привязанные к устройству',
  'pools-title':'Пулы Перераспределения',
  'pools-desc':'Каждая комиссия свопа, плата за демередж и превышение лимита богатства автоматически делится между четырьмя пулами. Все пулы выплачивают ежедневно.',
  'vel-pool':'Пул Валидаторов','vel-pool-desc':'40% всех комиссий → операторы нод, обеспечивающие сеть',
  'liq-pool':'Пул Ликвидности','liq-pool-desc':'30% всех комиссий → поставщики ликвидности, пропорционально LP-долям',
  'ubi-pool':'Пул UBI','ubi-pool-desc':'20% всех комиссий → все верифицированные люди поровну, каждые 24 часа',
  'treasury':'Казначейство','treasury-desc':'10% всех комиссий → разработка и обслуживание протокола',
  'phases-title':'Фазы Протокола',
  'phases-desc':'В Фазе 0 (Bootstrap) применяется скользящий множитель: max(5, min(N, 25))× средний баланс. При 1–4 людях: 5× средний. Каждый новый человек прибавляет 1×. При 25+ людях: фиксируется навсегда на 25×. Фаза 1+ сохраняет 25× фиксированным. Переходы автоматические — без голосования, без административных ключей.',
  'p0':'Bootstrap · &lt;100 людей · Лимит богатства: max(5,min(N,25))× средний · Скользит 5×→25× до 25-го человека · Сейчас активен',
  'p1':'Рост · 100–10 000 людей · Лимит богатства: 25× средний баланс',
  'p2':'Стабильность · 10 000–1М людей · Лимит богатства: 25× средний баланс',
  'p3':'Зрелость · 1М+ людей · Лимит богатства: 25× средний баланс',
  'wealth-cap-explain':'В Фазе 0 (Bootstrap) Лимит Богатства = max(5, min(N, 25))× средний баланс AEQ, где N = количество зарегистрированных людей. 1–4 человека: 5× средний. Каждый новый человек прибавляет 1×. 25+ людей: фиксируется навсегда на 25×. Лимит всегда привязан к актуальному среднему балансу.',
  'demurrage-title':'Демередж — Стимул к Обращению',
  'demurrage-desc':'Aequitas реализует механизм демереджа, вдохновлённый историческими дополнительными валютами. Бездействующие балансы AEQ постепенно теряют стоимость для предотвращения накопления.',
  'dem-rate-k':'Скорость Распада','dem-rate-v':'0,5% в месяц (непрерывно)',
  'dem-grace-k':'Льготный Период','dem-grace-v':'3 месяца бездействия до начала распада',
  'dem-reset-k':'Сброс Таймера','dem-reset-v':'Любой перевод, своп или операция с ликвидностью сбрасывает таймер',
  'dem-dest-k':'Распавшийся AEQ идёт в','dem-dest-v':'Пулы перераспределения (40/30/20/10)',
  'dem-warn-k':'Система Предупреждений','dem-warn-v':'14-дневное уведомление (один раз) + 7-дневное повторение при каждом входе',
  'story-title':'История Aequitas — Почему это существует',
  'story-text':'<p>Год 2009. Сатоши Накамото выпускает Bitcoin. Впервые ценность может передаваться между людьми без банка. Настоящая революция. Но почти сразу что-то идёт не так.</p><p>Ранние майнеры накапливают миллионы монет почти бесплатно. К 2021 году топ 1% адресов Bitcoin контролирует более 90% всех Bitcoin. Коэффициент Джини Bitcoin превышает 0,85 — выше чем в любой стране мира.</p><p><span style="color:var(--gold)">Aequitas</span> был создан чтобы ответить на один вопрос: <em style="color:var(--gold)">"Как выглядела бы криптовалюта, спроектированная с нуля чтобы быть справедливой для каждого человека?"</em></p><p>Ответ прост: <strong style="color:var(--text)">Деньги существуют потому что существуют люди. Поэтому каждый человек должен иметь равную долю денег просто потому что он человек.</strong></p><p><em style="color:var(--gold)">"Деньги существуют потому что существуют люди. Ничего более, ничего менее."</em></p>',
  'nodes-title':'Активные Ноды — Текущая Топология Сети','nodes-desc':'Сеть Aequitas работает на нескольких географически распределённых нодах (текущее число выше). Все они участвуют в производстве блоков и синхронизации. Сеть рассчитана на дополнительные ноды.',
  'run-node-title':'Запустите Свою Ноду — Помогите Защитить Сеть',
  'run-node-desc':'Любой может запустить ноду без разрешения. Операторы нод получают 40% всех комиссий свопа ежедневно через Пул Валидаторов.',
  'bootstrap-title':'Подключить Новую Ноду','bootstrap-desc':'Установите PRIMARY_NODE_URL=https://aequitas.digital в вашей среде. Нода автоматически синхронизируется и начнёт производство блоков.',
  'tech-title':'Технические Характеристики','mm-config':'Конфигурация MetaMask',
  'k-lang':'Язык','k-src':'Исходный Код','evm-yes':'Да — JSON-RPC /rpc · Совместимо с MetaMask',
  'proto-label':'Протокол Aequitas V7 — Техническая Документация',
  'ca-title':'Адреса Контрактов','ca-text':'Цепь: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 является единственным источником истины для всей экономики Aequitas. Каждый баланс AEQ, каждая регистрация человека, каждая выплата UBI и каждое применение ограничения богатства управляется этим одним неизменяемым контрактом — развёрнутым на Aequitas Chain, специализированном блокчейне совместимом с EVM работающем на механизме консенсуса BlockDAG. Нет ключа администратора, нет прокси обновления, нет голосования по управлению которое могло бы изменить хотя бы одну строку его логики. Код работающий сегодня — это код который будет работать через десять лет.<br><br>Контракт BioVerifier получает доказательства с нулевым разглашением Groth16 сгенерированные полностью на Android-устройстве пользователя. Он математически проверяет on-chain примерно за 10 мс что новый регистрант является уникальным живым человеком — не узнавая никогда его имени, личности или биометрических данных. Именно это делает возможной безгазовую регистрацию без инвестиций: доказательство — единственное что когда-либо покидает устройство.<br><br>Вместе эти два контракта делают возможным то чего никогда не существовало ни в одной денежной системе в истории: денежное предложение правила которого — кто его получает, сколько существует, как оно перераспределяется — не могут быть изменены ни одним человеком, компанией или правительством. Никогда.',
  'poa-title':'1. ДОКАЗАТЕЛЬСТВО ЖИЗНИ — Восстановление Неактивных Балансов','poa-text':'<p>Что происходит с AEQ когда люди умирают или становятся недееспособными? В Bitcoin потерянные кошельки означают навсегда потерянный объём. Aequitas решает это через многоуровневую систему: если кошелёк не проявляет активности в течение длительного периода, его баланс постепенно возвращается сообществу через пул UBI.</p>',
  'poa-box':'Год 0–2: Обычное использование — без ограничений<br>Год 2: Предупреждение 1 — Guardian может ответить от имени<br>Год 2+60д: Предупреждение 2 — нарастающая срочность<br>Год 2+120д: Предупреждение 3 — последнее уведомление<br>Год 2+180д: AEQ перемещён в личный ЭСКРОУ (ещё восстановимо)<br>Год 4: При сохранении бездействия — ЭСКРОУ в Пул UBI',
  'guard-title':'2. СИСТЕМА GUARDIAN — Человеческая Защита','guard-text':'<p>Что если кто-то госпитализирован или иначе не может получить доступ к устройству месяцами? Система Guardian позволяет доверенному лицу — другому верифицированному человеку — подтвердить что владелец кошелька жив. Guardian имеет строго нулевой финансовый доступ: он может только сбросить таймер бездействия.</p>',
  'guard-box':'1 Guardian на человека · должен быть верифицированным человеком в Aequitas<br>Guardian может ТОЛЬКО вызывать confirmAlive() — ноль прав транзакций<br>Guardian НЕ МОЖЕТ перемещать средства, переводить AEQ или получать доступ к кошельку<br>Максимум 3 подопечных · Блокировка 7 дней при назначении · Без круговых отношений',
  'dem-title':'3. ДЕМЕРЕДЖ — Механизм Против Накопления',
  'dem-box':'Ставка: 0,5%/месяц после 3 месяцев бездействия (непрерывно, не ступенчато)<br>Таймер сбрасывается при любом переводе, свопе или операции с ликвидностью<br>Decayed AEQ перераспределяется в пулы — никогда не сжигается',
  'dem-text':'<p>Демередж — стоимость хранения денег. Эксперимент Вёрглена (Австрия, 1932) сократил местную безработицу на 25% за год. Chiemgauer (Германия, 2003) работает по тому же принципу уже более 20 лет.</p>',
  'cap-title':'4. ЛИМИТ БОГАТСТВА — Математическое Обеспечение Справедливости','cap-box':'Bootstrap-лимит: max(5,min(N,25))× текущий средний баланс<br>1–4 людей: 5× · +1× за человека · 25+: 25× навсегда<br>Применяется ко всем адресам кроме 4 протокольных пулов<br>Избыток AEQ мгновенно перераспределяется · Без ручного вмешательства',
  'ubi-title':'5. УНИВЕРСАЛЬНЫЙ БАЗОВЫЙ ДОХОД — Ежедневное Перераспределение','ubi-box':'Источники: Комиссии свопов (20%) · Превышение лимита богатства · Демередж · Эскроу после 4 лет<br><br>Ежедневно: весь пул UBI делится поровну между всеми зарегистрированными людьми. Пул сбрасывается и сразу наполняется снова.',
  'inf-title':'6. НИКАКОЙ АЛГОРИТМИЧЕСКОЙ ИНФЛЯЦИИ — Фиксированная Формула','inf-box':'ЕДИНСТВЕННОЕ событие создающее новый AEQ: регистрируется новый верифицированный человек.<br><br>Общий Объём = Верифицированные Люди × 1 000 AEQ<br><br>Это не политика — обеспечивается протоколом. AEQ — единственная криптовалюта где объём определяется исключительно числом верифицированных живых людей.',
  'phases-desc':'В Фазе 0 лимит богатства использует скользящий Bootstrap-множитель: max(5, min(N, 25))× средний баланс. При 1–4 людях: 5× средний. Каждый новый человек прибавляет 1×. При 25+ людях: фиксируется навсегда на 25×. Фаза 1+ сохраняет 25× фиксированным. Переходы автоматические — без голосования, без административных ключей.',
  'p0':'Bootstrap · &lt;100 людей · Лимит богатства: max(5,min(N,25))× средний · Скользит 5×→25× до 25-го человека · Сейчас активен',
  'p1':'Рост · 100–10 000 людей · Лимит богатства: 25× средний баланс',
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
  'swap-btn-conn':'🦊 ПОДКЛЮЧИТЬ METAMASK','swap-btn-go':'🔄 ОБМЕНЯТЬ',
  'swap-log-hint':'// Подключите кошелёк для обмена...',
  'swap-no-liquidity':'Нет tUSD?','swap-faucet-desc':'Зарегистрированные люди могут получить тестовый tUSD один раз','swap-btn-faucet':'💧 ПОЛУЧИТЬ ТЕСТОВЫЙ tUSD',
  'swap-addliq-title':'Предоставить Ликвидность','swap-addliq-desc':'Будьте первым кто внесёт — ваше соотношение устанавливает начальную цену.','swap-btn-addliq':'💧 ДОБАВИТЬ ЛИКВИДНОСТЬ',
  'swap-lp-title':'Ваша LP-Позиция','swap-lp-share':'Доля в Пуле','swap-lp-withdrawable':'Доступно к выводу',
  'swap-lp-pct-label':'% вашей позиции','swap-lp-youget':'Вы получите','swap-btn-removeliq':'🔥 ВЫВЕСТИ ЛИКВИДНОСТЬ',
  'swap-pool-title':'AEQ / tUSD — Статус Пула',
  'swap-pool-aeq':'Резерв AEQ','swap-pool-tusd':'Резерв tUSD','swap-pool-price':'Спотовая Цена',
  'swap-fee-bps':'Комиссия Свопа',
  'swap-pools-addr-title':'Адреса Пулов Токеномики','pools-addr-title':'Адреса Контрактов Пулов',
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
  'usp-c3-title':'Доступно для всех','usp-c3-desc':'Без банковского счёта, кредитной карты и документов. Регистрация через доступный биометрический комплект (сканер отпечатков + датчик пульса, ~$15) — для глобального доступа.',
  'usp-c4-title':'Ежедневный UBI навсегда','usp-c4-desc':'После регистрации вы автоматически получаете ежедневную долю выплат UBI — каждый день, без каких-либо действий.',
  'v7-intro-title':'Что такое AequitasV7?',
  'v7-intro-text':'AequitasV7 — центральный смарт-контракт протокола Aequitas. "V7" — 7-я основная версия контракта справедливости. Развёрнут неизменяемым образом в Aequitas Chain (ID 1926) и управляет всем: регистрация людей, верификация ZK, управление балансами, лимит богатства, распределение UBI, комиссии свопов. Ни один администратор не может обновить его. Шесть механизмов образуют самоусиливающуюся систему.',
  'explore-title':'Исследовать Aequitas',
  'expl-score':'Индекс равенства','expl-score-d':'Коэффициент Джини · Индекс Aequitas · распределение богатства в реальном времени',
  'expl-economy':'UBI и пулы перераспределения','expl-economy-d':'Ежедневный обратный отсчёт UBI · 4 on-chain пула · демерредж · Фазы протокола',
  'expl-charts':'Графики и история','expl-charts-d':'История Джини · кривая Лоренца · ползунок начального загрузчика богатства · История Aequitas',
  'expl-v7':'Документация Протокола V7','expl-v7-d':'Контракт AequitasV7 · 6 механизмов · ZK-доказательство · лимит богатства · демерредж · неизменяемый код',
  'expl-explorer':'Обозреватель блоков','expl-explorer-d':'Живой BlockDAG · нажмите на блок чтобы увидеть валидатора, хэш, транзакции, родительские хэши',
  'swap-sell-label':'Продать','swap-receive-label':'Получить',
  'gini-what-title':'Что такое коэффициент Джини?','gini-what-text':'Разработан итальянским статистиком Коррадо Джини (1912). Измеряет распределение богатства, сравнивая фактические балансы с гипотетически равным базовым уровнем. Шкала: 0 (у всех одинаково) до 1 (у одного всё). Используется Всемирным банком, ОЭСР, ООН для сравнения стран. Справочные значения: Bitcoin ≈ 0,85 · ЮАР (мировой рекорд) ≈ 0,63 · США ≈ 0,41 · Германия ≈ 0,31 · Скандинавия ≈ 0,27 · Долгосрочная цель Aequitas: Джини ниже 0,30.','gini-calc-title':'Как рассчитывается Индекс Aequitas','gini-calc-text':'Собираются все балансы AEQ. Формула вычисляет среднее абсолютное отклонение нормализованное на n2. Результат 0-1 x 100 = Индекс.','gini-why-title':'Почему Gini','gini-why-text':'Gini учитывает полное распределение среди всех людей в одном числе.','expl-network':'Сеть и узлы','expl-network-d':'Топология узлов · запустить собственный узел · технические характеристики · Chain ID 1926',
  'guard-title':'🛡 Система Хранителя','guard-my-lbl':'Мой Хранитель','guard-none':'Нет',
  'guard-set-lbl':'Установить / Изменить Хранителя','guard-set-hint':'Должен быть зарегистрированным человеком Aequitas · Блокировка на 7 дней · Хранитель может только подтвердить вашу активность, не имея доступа к средствам · Макс. 3 подопечных на хранителя',
  'guard-confirm-lbl':'Подтвердить Активность (Как Хранитель)','guard-confirm-hint':'Если ваш подопечный не может получить доступ к кошельку, подтвердите его активность, чтобы предотвратить перевод средств на эскроу после 910 дней бездействия.','guard-recover-btn':'🔓 ВЕРНУТЬ ИЗ ЭСКРОУ',
  'faq-title':'❓ Вопросы и Ответы','faq-q1':'Мои биометрические данные в безопасности?','faq-a1':'Да. Aequitas никогда не собирает, не обрабатывает и не передаёт биометрические данные. Экран блокировки вашего телефона просто открывает доступ к случайному ключу, созданному и хранящемуся в его защищённом оборудовании. Передаётся только математическое доказательство, выведенное из этого ключа — никогда сам ключ, никогда биометрические данные.',
  'faq-q1b':'Доказывает ли регистрация, что я уникальный реальный человек?','faq-a1b':'Пока не полностью. Сегодняшнее доказательство криптографически подтверждает владение ключом конкретного устройства — это надёжно блокирует повторную регистрацию того же устройства (или кошелька), но пока не может отличить два разных физических устройства одного и того же человека. Настоящая межустройственная проверка биологической уникальности запланирована, но ещё не реализована — лучше сказать это прямо, чем преувеличивать гарантии сегодняшнего доказательства.',
  'faq-q2':'Могу ли я зарегистрироваться с другим кошельком позже?','faq-a2':'Нет. Регистрация постоянно привязана к одному адресу кошелька на ключ устройства. Это сделано намеренно — предотвращает повторную регистрацию того же устройства и обеспечивает один кошелёк на идентичность устройства.',
  'faq-q3':'Что произойдёт, если я потеряю телефон?','faq-a3':'Ваши AEQ остаются в кошельке — они привязаны к вашему приватному ключу, а не к телефону. Доступ к кошельку возможен через MetaMask с помощью сид-фразы. Восстановление кошелька не зависит от биометрической регистрации.',
  'path-title':'Выберите Свой Путь','path-human-title':'Я Человек','path-human-desc':'Хочу зарегистрироваться, получить 1 000 AEQ и присоединиться к сети базового дохода.','path-human-steps':'1. Скачать Android-приложение Aequitas<br>2. Разблокировать экраном блокировки устройства (отпечаток/лицо/PIN)<br>3. Подключить MetaMask<br>4. Получить 1 000 AEQ мгновенно',
  'path-node-title':'Я Оператор Ноды','path-node-desc':'Хочу запустить полную ноду, участвовать в производстве блоков и зарабатывать из 40%-ного пула валидаторов.','path-node-steps':'1. Зарегистрироваться как человек (обязательно)<br>2. Установить PRIMARY_NODE_URL=https://aequitas.digital<br>3. Развернуть на Railway/Contabo/VPS<br>4. Ежедневно зарабатывать из пула валидаторов',
  'path-dev-title':'Я Разработчик','path-dev-desc':'Хочу создавать на базе Aequitas, интегрировать API или вносить вклад в протокол.','path-dev-steps':'1. EVM-совместимый JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* эндпоинты<br>4. Метрики: /metrics (Prometheus)',
  'story-flow-title':'Схема движения токена AEQ','story-topo-title':'Топология Сети — Текущее Состояние',
  'swap-price-title':'AEQ / tUSD — Живая Цена','swap-price-desc':'Цена в реальном времени из резервов пула (x·y=k). Обновляется каждые 8 секунд с новыми данными пула.','swap-price-empty':'Данных пула ещё нет — добавьте ликвидность для просмотра графика цены.',
  'node-guide-lang-note':'Это руководство на английском. Перевод доступен в PDF на вашем языке — используйте кнопку выше.',
  'k-zkp':'ZKP-Система','k-hash':'Хеш-Система','k-sybil-prot':'Защита от Sybil',
},
zh:{
  'logo-sub':'人类证明','live':'实时',
  'tab-register':'🔐 注册','tab-explorer':'🔍 浏览器','tab-humans':'👥 人类','tab-index':'📊 指数','tab-network':'🌐 网络','tab-protocol':'📜 协议 V7','tab-swap':'🔄 兑换',
  'reg-title':'🔐 注册成为经过验证的人类',
  'reg-sub':'加入Aequitas网络并获得1,000 AEQ的普遍基本收入补贴。一次性、永久性且完全免费。永远不会存储任何个人数据。',
  'app-title':'仅通过安卓应用注册',
  'app-text':'注册会在您手机的安全硬件（Secure Enclave / StrongBox）内生成一个密码学密钥，由设备自身的锁屏保护——无需额外传感器，从不产生、处理或传输任何生物特征数据。Groth16零知识证明证明您持有该密钥而不泄露它。验证后自动记入您的1,000 AEQ。注意：这目前证明的是对一台设备的控制，而非跨设备的生物唯一性——详见常见问题。',
  's1t':'设备密钥','s1d':'您手机的安全硬件在您现有的锁屏保护下生成一个私钥（指纹/面部/PIN，无论您已在使用哪种）。无需单独的传感器套件，任何原始生物特征数据都不会离开设备。',
  's2t':'ZK证明生成','s2d':'Groth16 ZK证明将您的设备密钥提交为单一的承诺和nullifier，而不泄露密钥本身。这在密码学上证明您持有此设备的密钥——具体保证了什么、未保证什么请见常见问题。',
  's3t':'连接钱包','s3d':'应用在此页面打开MetaMask · 连接您的以太坊钱包 · 证明与您的地址密码绑定',
  's4t':'获得1,000 AEQ','s4d':'在6秒内在Aequitas BlockDAG上确认注册 · 立即记入1,000 AEQ · 身份永久记录为经过验证的人类',
  'priv-bar':'🔒 设备绑定的加密密钥 · Groth16 ZKP · 数据永不离开设备 · 每台设备一次注册',
  'conn-wallet':'已连接钱包','proof-recv':'⚡ 已收到ZK证明','proof-hint':'连接钱包以注册',
  'btn-conn':'🦊 连接 METAMASK','btn-reg':'🔐 链上注册',
  'btn-wc':'🔗 连接 WALLETCONNECT',
  'reg-log-hint':'// 打开Aequitas安卓应用生成您的证明，然后返回此处...',
  'reg-details':'注册详情','k-network':'网络','k-chainid':'链ID','k-grant':'UBI补贴',
  'k-fee':'Gas费','free':'免费——完全无Gas','k-limit':'注册','k-limit-v':'每台设备一次 · 永久 · 不可更改',
  'k-bio':'设备密钥','never-stored':'从不离开您的设备——不产生也不存储任何生物特征数据',
  'k-proof':'证明系统','k-conf':'确认','k-conf-v':'1秒内（1个区块）',
  'k-sybil':'女巫攻击防护','k-sybil-v':'每台设备一个身份 · 永久锁定（设备绑定，尚非身体绑定）',
  'live-stats':'实时链统计',
  's-height':'区块高度',
  's-humans':'已验证人类','s-humans-sub':'设备绑定的ZK证明 · 每台设备一次注册',
  's-supply':'总供应量','s-supply-sub':'始终 = 人类 × 1,000 AEQ',
  's-index':'Aequitas指数','s-index-sub':'0 = 完全平等 · 100 = 最大不平等',
  's-uptime':'运行时间','s-uptime-sub':'节点 v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · 各自独立PostgreSQL',
  'ib-poh':'人类证明','ib-poh-t':'每个AEQ持有者通过Groth16 ZK证明密码学地证明其控制一个设备绑定的密钥。没有机器人、公司、人工智能。从不传输生物特征数据，仅传输数学证明。这目前将一次注册绑定到一台设备，尚未绑定到唯一的人——详见常见问题。',
  'ib-fair':'彻底公平的分配','ib-fair-t':'每个经过验证的人类注册时恰好获得1,000 AEQ。没有预挖矿，没有创始人分配。总供应量始终等于已验证人类 × 1,000。',
  'ib-dag':'BlockDAG架构','ib-dag-t':'多个区块可以同时生产并合并。比线性区块链更高吞吐量、更低延迟。',
  'ib-gas':'真正无Gas','ib-gas-t':'注册和AEQ转账完全免费。不需要ETH、BNB或MATIC。无需银行账户或信用卡。',
  'recent-blocks':'最近区块','blocks-desc':'MERGE = 多个父区块合并（BlockDAG）。TX = 注册交易。区块时间：__BT__。',
  'loading':'加载区块中...','net-info':'网络信息','k-chain':'链名称','k-symbol':'符号','k-btime':'区块时间',
  'k-cons':'共识','k-nodes':'活跃节点','k-storage':'存储','add-mm':'🦊 添加到METAMASK','k-dec':'小数位',
  'btn-add-mm':'+ 添加AEQUITAS网络',
  'phil':'"货币存在是因为人类存在。<br>仅此而已，别无其他。"','phil-sub':'— AEQUITAS原则 —',
  'humans-title':'Aequitas链上的已验证人类',
  'h-what':'什么是已验证人类？','h-what-t':'已验证人类目前是指密码学地证明控制某一特定设备安全密钥的钱包地址。该密钥在手机的安全硬件内生成，由设备自身锁屏保护——无需单独的传感器套件。仅传输Groth16 ZK证明，任何生物特征数据都不会离开设备。这目前验证的是一台设备，未必已是一个独特的人——详见常见问题。',
  'h-zkp':'零知识证明系统','h-zkp-t':'Aequitas在BN128上使用Groth16——与Ethereum和Zcash相同的曲线。约200字节，约10毫秒。commitment = keccak256(deviceKey‖wallet)。Nullifier绑定到此设备：丢失手机不会在该设备上创建第二身份，但另一台设备仍可单独注册。密钥材料从不在服务器端泄露或存储。',
  'h-sybil':'女巫攻击抵御——当前状态','h-sybil-t':'当前的nullifier源自设备绑定的硬件密钥——它能可靠地阻止同一设备（或钱包）重复注册，但目前无法检测同一个人从第二台物理设备进行注册。填补这一空白需要真正的跨设备生物唯一性验证，计划在测试版之后进行，尚未上线。',
  'h-global':'全球金融包容','h-global-t':'无需银行账户、信用卡或加密货币。只需一台使用您已在用的锁屏方式（指纹/面部/PIN）的安卓手机。',
  'h-bio-hw':'身份验证路线图','h-bio-hw-t':'今天（测试版）：每台设备一个设备绑定的硬件密钥，诚实地标记为设备绑定而非身体绑定。计划（测试版之后）：真正的跨设备生物唯一性验证——先规范、构建，再经过独立审计，然后才做出更强的女巫抵御声明。',
  'reg-humans':'已注册人类','h-desc':'每个地址已通过ZK证明证明其控制一个设备绑定的加密密钥，并恰好获得1,000 AEQ。永久、不可更改、链上。关于"设备绑定"目前对女巫防护意味着什么，详见常见问题。',
  'no-humans':'尚未注册人类。\n\n下载Aequitas安卓应用，成为链上第一个人类！',
  'reg-stats':'注册统计','total-humans':'总人数',
  'idx-title':'Aequitas指数——实时经济平等评分',
  'idx-desc':'Aequitas指数实时衡量所有经过验证的人类的经济不平等。从链上余额分布的基尼系数导出。0 = 完全平等。100 = 最大不平等。',
  'curr-idx':'当前指数','bar-0':'0 — 完全平等','bar-100':'100 — 最大不平等','wcap-lbl':'当前财富上限：','wcap-mult':'倍数：','wcap-avg':'平均余额：',
  'gini':'基尼系数','gini-desc':'0 = 平等 · 1 = 不平等',
  'supply-desc':'始终 = 人类 × 1,000 AEQ',
  'phase':'协议阶段','phase-desc':'按人类数量自动推进',
  'humans-desc':'设备绑定的ZK验证注册',
  'pools-title':'再分配池',
  'pools-desc':'每笔兑换费用、滞期费和财富上限溢出自动在四个池之间分配。无需人工干预。所有池每日分配。',
  'vel-pool':'验证者池','vel-pool-desc':'所有费用的40% → 保障网络安全的节点运营商',
  'liq-pool':'流动性池','liq-pool-desc':'所有费用的30% → 流动性提供者，按LP份额比例',
  'ubi-pool':'UBI池','ubi-pool-desc':'所有费用的20% → 所有经过验证的人类均等，每24小时',
  'treasury':'国库','treasury-desc':'所有费用的10% → 协议开发和维护',
  'phases-title':'协议阶段',
  'phases-desc':'阶段转换由人类数量自动触发——无需投票、治理或管理员密钥。',
  'p0':'启动 · &lt;100人类 · 财富上限：50×平均余额 · 当前活跃',
  'p1':'增长 · 100–10,000人类 · 财富上限：20×平均余额',
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
  'story-text':'<p>2009年。中本聪发布比特币。有史以来第一次，价值可以在不经过银行的情况下在两人之间转移。一场真正的革命。但几乎立刻出现了问题。</p><p>早期矿工以接近零的成本积累了数百万枚代币。到2021年，比特币地址中的前1%控制了90%以上的比特币。比特币的基尼系数超过0.85——高于地球上任何国家。</p><p><span style="color:var(--gold)">Aequitas</span>——拉丁语"公平"和"平等"——的创建是为了回答一个问题：<em style="color:var(--gold)">"如果从第一原则设计一种对每个人都公平的加密货币会是什么样？"</em></p><p>答案很简单：<strong style="color:var(--text)">货币存在是因为人类存在。因此，每个人仅凭成为人类就应该拥有等份的货币。</strong></p><p><em style="color:var(--gold)">"货币存在是因为人类存在。仅此而已，别无其他。"</em></p>',
  'nodes-title':'活跃节点 — 当前网络拓扑','nodes-desc':'Aequitas网络目前在多个地理分布的节点上运行（当前数量见上方）。所有节点均参与区块生产、状态同步和API服务。通过libp2p点对点通信，通过HTTP同步区块状态。网络设计支持更多节点——任何运营商均可加入。',
  'run-node-title':'运行您自己的节点 — 帮助保护网络',
  'run-node-desc':'任何人都可以运行Aequitas节点——无需许可、无需质押、无需申请。节点参与区块生产并验证人类注册表。节点运营商通过验证者池（每日分配的所有互换费用的40%）赚取协议费用份额。',
  'run-node-title':'运行您自己的节点 — 帮助保护网络',
  'run-node-desc':'任何人都可以运行Aequitas节点——无需许可、无需质押、无需申请。节点参与区块生产并验证人类注册表。节点运营商通过验证者池（每日分配的所有互换费用的40%）赚取协议费用份额。',
  'bootstrap-title':'运行自己的节点','bootstrap-desc':'任何人都可以通过运行节点加入Aequitas网络。下载节点指南获取分步说明。',
  'tech-title':'技术规格','mm-config':'MetaMask配置',
  'k-lang':'语言','k-src':'源代码','evm-yes':'是 — JSON-RPC /rpc · MetaMask兼容',
  'proto-label':'Aequitas V7协议——技术文档',
  'ca-title':'合约地址','ca-text':'链：Aequitas Chain（链ID：1926 · 0x786）<br>RPC：https://aequitas.digital/rpc<br><br>BioVerifier：0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7：0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7是整个Aequitas经济体系的唯一真实来源。每一个AEQ余额、每一次人类注册、每一次UBI支付以及每一次财富上限执行，都由这一个不可变合约管理——部署在Aequitas Chain上，这是一个运行BlockDAG共识引擎的定制EVM兼容区块链。没有管理员密钥、没有升级代理、没有任何治理投票能够改变其逻辑中的任何一行代码。今天运行的代码就是十年后运行的代码。<br><br>BioVerifier合约接收完全在用户Android设备上生成的Groth16零知识证明。它在约10毫秒内在链上数学验证新注册者是唯一的活体人类——而不会泄露他们的姓名、身份或生物特征数据。这使得无gas、无需投资的注册成为可能：证明是唯一离开设备的东西。<br><br>这两个合约共同使在历史上任何货币体系中从未存在过的事情成为可能：一种货币供应，其规则——谁获得它、有多少存在、如何重新分配——永远无法被任何人、公司或政府改变。永远。',
  'poa-title':'1. 生存证明 — 非活跃余额恢复','poa-text':'<p>当人们死亡或永久失去行为能力时AEQ会怎样？在比特币中，丢失的钱包意味着永久丢失的供应量。Aequitas通过多阶段非活跃恢复系统解决这个问题：如果一个钱包长时间没有活动，其余额会逐渐通过UBI池返回社区。</p>',
  'poa-box':'第0–2年：正常使用 — 无限制<br>第2年：警告1 — 监护人可以代表回应<br>第2年+60天：警告2 — 紧迫性增加<br>第2年+120天：警告3 — 最终通知<br>第2年+180天：AEQ移至个人托管（仍可恢复）<br>第4年：如果仍不活跃 — 托管释放至UBI池',
  'guard-title':'2. 监护人系统 — 人类安全保障','guard-text':'<p>如果有人住院或因其他原因数月无法访问其设备怎么办？监护人系统允许可信任的人——另一个经过验证的人类——确认钱包所有者仍然活着。监护人拥有严格为零的财务访问权限：只能调用重置非活跃计时器的单一函数。在任何情况下都不能移动、花费或访问资金。</p>',
  'guard-box':'每人1个监护人 · 必须是Aequitas上的经过验证的人类<br>监护人只能调用confirmAlive() — 零交易权限<br>监护人不能移动资金、转移AEQ或访问钱包<br>每个监护人最多3名受监护人 · 分配7天时间锁 · 不允许循环关系',
  'dem-title':'3. 滞期费 — 防囤积机制',
  'dem-box':'费率：3个月非活跃后每月0.5%（连续，非分步）<br>任何转账、互换或流动性操作会自动重置计时器<br>衰减的AEQ重新分配到四个池中 — 从不销毁<br>14天通知显示一次 · 每次活跃会话重复7天提醒',
  'dem-text':'<p>滞期费是货币的持有成本——一种使囤积变得昂贵、流通变得有吸引力的负利率。沃尔格实验（奥地利，1932年）使用滞期费货币在一年内将当地失业率降低了25%。奥地利中央银行正因为它运作得太好而关闭了它。Chiemgauer（德国，2003年）按照相同原则成功运营了20多年。</p>',
  'cap-title':'4. 财富上限 — 数学公平执行','cap-box':'启动上限：max(5,min(N,25))× 平均AEQ余额<br>1–4人：5×（5,000 AEQ）· 每增1人加1× · 25+人：25×（25,000 AEQ）永久<br>适用于除4个协议池外的所有地址<br>超额AEQ立即重新分配 · 无需手动干预',
  'ubi-title':'5. 普遍基本收入 — 每日再分配','ubi-box':'UBI池收入来源：<br>· AEQ↔tUSD AMM池所有互换费用的20%<br>· 财富上限执行的溢出<br>· 非活跃账户的滞期费<br>· 4年后释放的非活跃托管<br><br>分配：每24小时，整个UBI池余额在所有注册的经过验证的人类中平均分配。池重置为零并立即开始从持续的协议活动中重新填充。',
  'inf-title':'6. 无算法通胀 — 固定供应公式','inf-box':'创建新AEQ的唯一事件：新的经过验证的人类注册。<br><br>总供应量 = 经过验证的人类 × 1,000 AEQ<br><br>这不是政策——它由协议执行。没有管理员可以铸造额外的AEQ，没有治理投票可以改变发行，没有预挖矿的创始人分配。AEQ是唯一一种总供应量完全由经过验证的活人数量决定的加密货币。',
  'phases-desc':'阶段边界定义网络增长里程碑。启动阶段（&lt;25名注册人类）财富上限使用滑动乘数：max(5,min(N,25))×平均余额 — 1–4人时为5×，每增加1人加1×，25+人时达到完整25×。防止早期参与者在真正参与形成前集中财富。',
  'p0':'引导期 · 不足100人 · 上限：max(5,min(N,25))×平均 · 滑动5×→25×直至25人 · 当前激活',
  'p1':'增长期 · 100–10,000人 · 财富上限：25×平均余额',
  'p2':'稳定期 · 10,000–1M人 · 财富上限：25×平均余额',
  'p3':'成熟期 · 1M+人 · 财富上限：25×平均余额',
  'wealth-cap-explain':'财富上限在启动阶段动态调整：max(5, min(N, 25)) × 平均余额，N为已注册人类数。1–4人时：5×（5,000 AEQ）。每新增1人多1×。25+人时：永久25×（25,000 AEQ）。防止早期采用者在真实参与形成前过度积累。始终相对于当前平均余额。',
  'btn-download-app':'下载 AEQUITAS 应用',
  'swap-title':'🔄 兑换 AEQ ↔ tUSD','swap-sub':'通过原生流动性池将AEQ兑换为tUSD（模拟测试美元）。0.1%手续费仅适用于兑换 — 人与人之间的普通AEQ转账完全免费。',
  'swap-priv-bar':'🔒 仅0.1%兑换费 · AEQ到AEQ转账免费 · tUSD是无实际价值的测试货币',
  'swap-your-aeq':'你的 AEQ','swap-your-tusd':'你的 tUSD',
  'swap-fee-est':'协议手续费 (0.1%)','swap-details-hdr':'兑换详情',
  'swap-out-lbl':'你获得（估算）','swap-impact-lbl':'价格影响','swap-rate-lbl':'汇率',
  'swap-depth-lbl':'池子构成','amm-title':'x × y = k — 恒定乘积 AMM',
  'amm-text':'当你用AEQ兑换tUSD时，AEQ储备增加，tUSD储备减少——它们的乘积始终等于k。更大的兑换造成更大的价格影响。0.1%手续费在应用公式前从输入中扣除。',
  'swap-btn-conn':'🦊 连接 METAMASK','swap-btn-go':'🔄 兑换',
  'swap-log-hint':'// 连接钱包以兑换...',
  'swap-no-liquidity':'还没有 tUSD？','swap-faucet-desc':'已注册的人类可以申领一次测试 tUSD','swap-btn-faucet':'💧 申领测试 tUSD',
  'swap-addliq-title':'提供流动性','swap-addliq-desc':'成为第一个存款者 — 你的比例设定起始价格。','swap-btn-addliq':'💧 添加流动性',
  'swap-lp-title':'你的 LP 仓位','swap-lp-share':'池子份额','swap-lp-withdrawable':'可提取',
  'swap-lp-pct-label':'% 你的仓位','swap-lp-youget':'你将收到','swap-btn-removeliq':'🔥 移除流动性',
  'swap-pool-title':'AEQ / tUSD — 池子状态',
  'swap-pool-aeq':'AEQ 储备','swap-pool-tusd':'tUSD 储备','swap-pool-price':'现货价格',
  'swap-fee-bps':'兑换手续费',
  'swap-pools-addr-title':'代币经济池地址','pools-addr-title':'池合约地址',
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
  'usp-c3-title':'人人可参与','usp-c3-desc':'无需银行账户、信用卡或身份证件，无需购买额外硬件——只需您的安卓手机已内置的锁屏功能。',
  'usp-c4-title':'永久每日UBI','usp-c4-desc':'注册后，您每天自动获得UBI支付份额——每天，无需任何操作。',
  'v7-intro-title':'什么是 AequitasV7？',
  'v7-intro-text':'AequitasV7是Aequitas协议的核心智能合约。"V7"指公平合约的第7个主要版本。它不可更改地部署在Aequitas Chain（链ID 1926）上，处理所有方面：人类注册、ZK证明验证、余额管理、财富上限、UBI分配、兑换手续费。没有管理员可以升级它。六个机制形成自我强化系统。',
  'explore-title':'探索 Aequitas',
  'expl-score':'平等指数','expl-score-d':'实时基尼系数 · Aequitas指数 · 实时财富分配',
  'expl-economy':'UBI与再分配池','expl-economy-d':'每日UBI倒计时 · 4个链上池 · 货币持有税 · 协议阶段',
  'expl-charts':'图表与历史','expl-charts-d':'基尼历史 · 洛伦兹曲线 · 财富上限启动滑块 · Aequitas的故事',
  'expl-v7':'协议V7文档','expl-v7-d':'AequitasV7合约 · 6个机制 · ZK证明 · 财富上限 · 货币持有税 · 不可更改代码',
  'expl-explorer':'区块浏览器','expl-explorer-d':'实时BlockDAG · 点击任意区块查看验证者、哈希、交易、父哈希',
  'swap-sell-label':'卖出','swap-receive-label':'接收',
  'gini-what-title':'什么是基尼系数？','gini-what-text':'由意大利统计学家科拉多·基尼于1912年提出。通过将实际余额与假设的完全平等基线进行比较来衡量财富分配。范围：0（人人均等）到1（一人独占）。世界银行、经合组织、联合国用于比较各国。参考值：比特币≈0.85 · 南非（世界纪录）≈0.63 · 美国≈0.41 · 德国≈0.31 · 北欧≈0.27 · Aequitas长期目标：基尼系数低于0.30。','gini-calc-title':'如何计算Aequitas指数','gini-calc-text':'收集所有AEQ余额。公式计算每对余额之间的平均绝对差，结果0-1乘以100=Aequitas指数。','gini-why-title':'为什么选择基尼系数','gini-why-text':'基尼系数捕捉所有已验证人类的完整分布。Aequitas将此数据发布在链上。','expl-network':'网络与节点','expl-network-d':'节点拓扑 · 运行自己的节点 · 技术规格 · Chain ID 1926',
  'guard-title':'🛡 守护者系统','guard-my-lbl':'我的守护者','guard-none':'无',
  'guard-set-lbl':'设置 / 更改守护者','guard-set-hint':'必须是已注册的Aequitas人类 · 7天时间锁 · 守护者只能确认您的活跃状态，不能访问资金 · 每位守护者最多3名被保护者',
  'guard-confirm-lbl':'确认存活（作为守护者）','guard-confirm-hint':'如果您的被保护者无法访问其钱包，请确认其活跃状态，以防止其资金在910天不活跃后转入托管。','guard-recover-btn':'🔓 从托管中恢复',
  'faq-title':'❓ 常见问题','faq-q1':'我的生物特征数据安全吗？','faq-a1':'是的。Aequitas 从不采集、处理或传输任何生物特征数据。您手机的锁屏只是授权访问一个在安全硬件中生成并存储的随机密钥。只会传输从该密钥推导出的数学证明——密钥本身和任何生物特征数据都不会被传输。',
  'faq-q1b':'注册能证明我是独特的真实人类吗？','faq-a1b':'目前还不能完全证明。今天的证明只能密码学地证明你控制着某一特定设备的安全密钥——它能可靠地阻止同一设备（或钱包）重复注册，但目前无法区分同一个人拥有的两台不同物理设备。真正的跨设备生物唯一性验证已在规划中，但尚未上线——我们宁愿坦率说明，也不愿夸大现有设备绑定证明的保证范围。',
  'faq-q2':'我以后可以用不同的钱包注册吗？','faq-a2':'不可以。注册永久绑定到每个设备密钥的一个钱包地址。这是有意设计——防止同一设备重复注册，并确保每个设备身份对应一个钱包。',
  'faq-q3':'如果我丢失手机会怎样？','faq-a3':'您的AEQ保留在您的钱包中——它与您的私钥绑定，而非手机。您仍然可以通过MetaMask使用助记词访问钱包。钱包恢复与生物特征注册无关。',
  'path-title':'选择您的路径','path-human-title':'我是人类','path-human-desc':'我想注册、获得1,000 AEQ并加入基本收入网络。','path-human-steps':'1. 下载Aequitas安卓应用<br>2. 用设备锁屏解锁（指纹/面部/PIN）<br>3. 连接MetaMask<br>4. 立即获得1,000 AEQ',
  'path-node-title':'我是节点运营商','path-node-desc':'我想运行完整节点，参与区块生产，并从40%验证者池中获利。','path-node-steps':'1. 注册为人类（必须）<br>2. 设置PRIMARY_NODE_URL=https://aequitas.digital<br>3. 部署在Railway/Contabo/VPS<br>4. 每日从验证者池获利',
  'path-dev-title':'我是开发者','path-dev-desc':'我想在Aequitas上构建，集成API，或为协议做贡献。','path-dev-steps':'1. EVM兼容JSON-RPC<br>2. 链ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* 端点<br>4. 指标: /metrics (Prometheus)',
  'story-flow-title':'AEQ代币流向图','story-topo-title':'网络拓扑——当前状态',
  'swap-price-title':'AEQ / tUSD — 实时价格','swap-price-desc':'从池储备（x·y=k）实时派生的价格。每8秒更新一次。','swap-price-empty':'暂无池数据——添加流动性以查看价格图表。',
  'node-guide-lang-note':'此内联指南为英文。您语言的翻译PDF可通过上方按钮获取。',
  'k-zkp':'ZKP系统','k-hash':'哈希系统','k-sybil-prot':'女巫攻击防护',
},
id:{
  'logo-sub':'BUKTI KEMANUSIAAN','live':'LANGSUNG',
  'tab-register':'🔐 Daftar','tab-explorer':'🔍 Penjelajah','tab-humans':'👥 Manusia','tab-index':'📊 Indeks','tab-network':'🌐 Jaringan','tab-protocol':'📜 Protokol V7','tab-swap':'🔄 Tukar',
  'reg-title':'🔐 Daftar sebagai Manusia Terverifikasi',
  'reg-sub':'Bergabunglah dengan jaringan Aequitas dan terima hibah Pendapatan Dasar Universal sebesar 1.000 AEQ. Satu kali, permanen, dan sepenuhnya gratis. Tidak ada data pribadi yang pernah disimpan.',
  'app-title':'PENDAFTARAN HANYA MELALUI APLIKASI ANDROID',
  'app-text':'Pendaftaran menghasilkan kunci kriptografi di dalam perangkat keras aman ponsel Anda (Secure Enclave / StrongBox), dilindungi oleh kunci layar perangkat itu sendiri — tanpa sensor terpisah, tanpa data biometrik apa pun yang pernah dihasilkan, diproses, atau ditransmisikan. Bukti ZK Groth16 membuktikan Anda memiliki kunci tersebut tanpa mengungkapkannya. 1.000 AEQ Anda dikreditkan otomatis setelah verifikasi. Catatan: ini saat ini membuktikan kontrol atas satu perangkat, bukan keunikan biologis lintas-perangkat — lihat FAQ.',
  's1t':'Kunci Perangkat','s1d':'Perangkat keras aman ponsel Anda menghasilkan kunci privat yang dilindungi oleh kunci layar Anda yang sudah ada (sidik jari/wajah/PIN, yang mana pun sudah Anda gunakan). Tanpa kit sensor terpisah, tidak ada data biometrik mentah yang pernah meninggalkan perangkat.',
  's2t':'Pembuatan Bukti ZK','s2d':'Bukti ZK Groth16 mengkomit kunci perangkat Anda menjadi satu commitment dan nullifier tanpa mengungkapkan kunci itu sendiri. Ini secara kriptografis membuktikan Anda memiliki kunci perangkat ini — lihat FAQ untuk apa yang dijamin dan tidak dijamin oleh ini.',
  's3t':'Hubungkan Dompet','s3d':'Aplikasi membuka MetaMask di halaman ini · hubungkan dompet Ethereum Anda · bukti terikat secara kriptografis ke alamat Anda',
  's4t':'1.000 AEQ Dikreditkan','s4d':'Pendaftaran dikonfirmasi di BlockDAG Aequitas dalam 6 detik · 1.000 AEQ dikreditkan seketika · identitas Anda dicatat permanen sebagai manusia terverifikasi',
  'priv-bar':'🔒 Kunci kriptografi terikat perangkat · Groth16 ZKP · Data tidak pernah meninggalkan perangkat · Satu pendaftaran per perangkat',
  'conn-wallet':'DOMPET TERHUBUNG','proof-recv':'⚡ BUKTI ZK DITERIMA','proof-hint':'Hubungkan dompet untuk mendaftar',
  'btn-conn':'🦊 HUBUNGKAN METAMASK','btn-reg':'🔐 DAFTAR ON-CHAIN',
  'btn-wc':'🔗 HUBUNGKAN WALLETCONNECT',
  'reg-log-hint':'// Buka Aplikasi Android Aequitas untuk membuat bukti Anda, lalu kembali ke sini...',
  'reg-details':'Detail Pendaftaran','k-network':'Jaringan','k-chainid':'ID Rantai','k-grant':'Hibah UBI',
  'k-fee':'Biaya Gas','free':'GRATIS — sepenuhnya tanpa gas','k-limit':'Pendaftaran','k-limit-v':'Satu kali per perangkat · permanen · tidak dapat diubah',
  'k-bio':'Kunci Perangkat','never-stored':'Tidak pernah meninggalkan perangkat Anda — tidak ada data biometrik yang dihasilkan atau disimpan',
  'k-proof':'Sistem Bukti','k-conf':'Konfirmasi','k-conf-v':'Dalam 1 detik (1 blok)',
  'k-sybil':'Perlindungan Sybil','k-sybil-v':'Satu identitas per perangkat · kunci permanen (terikat perangkat, belum terikat tubuh)',
  'live-stats':'Statistik Rantai Langsung',
  's-height':'Tinggi Blok',
  's-humans':'Manusia Terverifikasi','s-humans-sub':'Bukti ZK terikat perangkat · satu pendaftaran per perangkat',
  's-supply':'Total Pasokan','s-supply-sub':'Selalu = Manusia × 1.000 AEQ',
  's-index':'Indeks Aequitas','s-index-sub':'0 = kesetaraan sempurna · 100 = ketidaksetaraan maksimum',
  's-uptime':'Waktu Aktif','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'Bukti Kemanusiaan','ib-poh-t':'Setiap pemegang AEQ membuktikan secara kriptografis kepemilikan kunci yang terikat perangkat melalui bukti ZK Groth16. Tidak ada bot, korporasi, AI. Data biometrik tidak pernah ditransmisikan, hanya bukti matematis. Ini saat ini mengikat satu pendaftaran per perangkat, belum per orang unik — lihat FAQ.',
  'ib-fair':'Distribusi yang Benar-benar Adil','ib-fair-t':'Setiap manusia terverifikasi menerima tepat 1.000 AEQ saat pendaftaran. Tanpa pre-mining, tanpa alokasi pendiri. Total pasokan selalu sama dengan manusia terverifikasi × 1.000.',
  'ib-dag':'Arsitektur BlockDAG','ib-dag-t':'Beberapa blok dapat diproduksi secara bersamaan dan digabungkan. Throughput lebih tinggi, latensi lebih rendah.',
  'ib-gas':'Benar-benar Tanpa Gas','ib-gas-t':'Pendaftaran dan transfer AEQ tidak memerlukan biaya. Tidak perlu ETH, BNB, atau MATIC. Tidak perlu rekening bank atau kartu kredit.',
  'recent-blocks':'Blok Terbaru','blocks-desc':'MERGE = beberapa induk digabung (BlockDAG). TX = transaksi pendaftaran. Waktu blok: __BT__.',
  'loading':'Memuat blok...','net-info':'Informasi Jaringan','k-chain':'Nama Rantai','k-symbol':'Simbol','k-btime':'Waktu Blok',
  'k-cons':'Konsensus','k-nodes':'Node Aktif','k-storage':'Penyimpanan','add-mm':'🦊 TAMBAHKAN KE METAMASK','k-dec':'Desimal',
  'btn-add-mm':'+ TAMBAHKAN JARINGAN AEQUITAS',
  'phil':'"Uang ada karena manusia ada.<br>Tidak lebih, tidak kurang."','phil-sub':'— PRINSIP AEQUITAS —',
  'humans-title':'Manusia Terverifikasi di Aequitas Chain',
  'h-what':'Apa itu Manusia Terverifikasi?','h-what-t':'Manusia Terverifikasi saat ini adalah alamat dompet yang terbukti secara kriptografis menguasai kunci aman satu perangkat tertentu. Kunci dibuat di dalam perangkat keras aman ponsel, dilindungi oleh kunci layar perangkat — tanpa kit sensor terpisah. Hanya bukti ZK Groth16 yang ditransmisikan, tidak ada data biometrik yang meninggalkan perangkat. Ini saat ini memverifikasi satu perangkat, belum tentu satu orang unik — lihat FAQ.',
  'h-zkp':'Sistem Bukti ZK','h-zkp-t':'Aequitas menggunakan Groth16 pada BN128 — kurva yang sama dengan Ethereum dan Zcash. ~200 byte, ~10ms. commitment = keccak256(deviceKey‖wallet). Nullifier terikat ke perangkat ini: kehilangan ponsel tidak membuat identitas kedua di perangkat itu, tetapi perangkat lain masih dapat mendaftar secara terpisah. Materi kunci tidak pernah diungkapkan atau disimpan di server.',
  'h-sybil':'Ketahanan Sybil — Kondisi Saat Ini','h-sybil-t':'Nullifier saat ini berasal dari kunci perangkat keras yang terikat perangkat — ini secara andal mencegah pendaftaran ganda pada perangkat atau dompet yang sama, tetapi belum dapat mendeteksi orang yang sama mendaftar dari perangkat fisik kedua. Menutup celah itu memerlukan pemeriksaan keunikan biologis lintas-perangkat yang sesungguhnya, direncanakan untuk pasca-beta, belum dirilis.',
  'h-global':'Inklusi Keuangan Global','h-global-t':'Tidak perlu rekening bank, kartu kredit, atau cryptocurrency sebelumnya. Hanya smartphone Android dengan kunci layar (sidik jari/wajah/PIN) yang sudah Anda gunakan.',
  'h-bio-hw':'Peta Jalan Verifikasi Identitas','h-bio-hw-t':'Hari ini (beta): kunci perangkat keras terikat perangkat per perangkat, secara jujur diberi label sebagai terikat perangkat, bukan terikat tubuh. Direncanakan (pasca-beta): pemeriksaan keunikan biologis lintas-perangkat yang sesungguhnya — dispesifikasikan, dibangun, dan diaudit secara independen sebelum membuat klaim ketahanan Sybil yang lebih kuat.',
  'reg-humans':'Manusia Terdaftar','h-desc':'Setiap alamat telah membuktikan melalui bukti ZK kepemilikan kunci kriptografi terikat perangkat dan menerima tepat 1.000 AEQ. Permanen, tidak dapat diubah, on-chain. Lihat FAQ untuk apa arti "terikat perangkat" bagi ketahanan Sybil saat ini.',
  'no-humans':'Belum ada manusia terdaftar.\n\nUnduh Aplikasi Android Aequitas dan jadilah yang pertama!',
  'reg-stats':'Statistik Registri','total-humans':'Total Manusia',
  'idx-title':'Indeks Aequitas — Skor Kesetaraan Ekonomi Real-Time',
  'idx-desc':'Indeks Aequitas mengukur ketidaksetaraan ekonomi semua manusia terverifikasi secara real-time. Diturunkan dari koefisien Gini distribusi saldo on-chain. 0 = kesetaraan sempurna. 100 = ketidaksetaraan maksimum.',
  'curr-idx':'Indeks Saat Ini','bar-0':'0 — Kesetaraan Sempurna','bar-100':'100 — Maks. Ketidaksetaraan','wcap-lbl':'Batas Kekayaan Saat Ini:','wcap-mult':'Pengganda:','wcap-avg':'Saldo rata-rata:',
  'gini':'Koefisien Gini','gini-desc':'0 = setara · 1 = tidak setara',
  'supply-desc':'Selalu = Manusia × 1.000 AEQ',
  'phase':'Fase Protokol','phase-desc':'Otomatis berdasarkan jumlah manusia',
  'humans-desc':'Pendaftaran terverifikasi ZK terikat perangkat',
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
  'story-text':'<p>Tahun 2009. Satoshi Nakamoto merilis Bitcoin. Untuk pertama kalinya, nilai dapat ditransfer antara dua orang tanpa bank. Sebuah revolusi sejati. Tetapi hampir segera sesuatu yang salah terjadi.</p><p>Para penambang awal mengumpulkan jutaan koin dengan biaya hampir nol. Pada 2021, 1% teratas alamat Bitcoin menguasai lebih dari 90% semua Bitcoin. Koefisien Gini Bitcoin melebihi 0,85 — lebih tinggi dari negara mana pun di Bumi.</p><p><span style="color:var(--gold)">Aequitas</span> — Latin untuk "keadilan" dan "kesetaraan" — diciptakan untuk menjawab: <em style="color:var(--gold)">"Seperti apa cryptocurrency yang dirancang dari prinsip pertama untuk adil bagi setiap manusia?"</em></p><p>Jawabannya sederhana: <strong style="color:var(--text)">Uang ada karena manusia ada. Oleh karena itu, setiap orang harus memiliki bagian yang sama dari uang hanya karena menjadi manusia.</strong></p><p><em style="color:var(--gold)">"Uang ada karena manusia ada. Tidak lebih, tidak kurang."</em></p>',
  'nodes-title':'Node Aktif — Topologi Jaringan Saat Ini','nodes-desc':'Jaringan Aequitas saat ini beroperasi pada beberapa node yang tersebar secara geografis (jumlah saat ini di atas). Semuanya berpartisipasi dalam produksi blok, sinkronisasi status, dan layanan API. Jaringan dirancang untuk mendukung node tambahan — operator mana pun dapat bergabung.',
  'run-node-title':'Jalankan Node Anda Sendiri — Bantu Amankan Jaringan',
  'run-node-desc':'Siapa pun dapat menjalankan node Aequitas — tanpa izin, tanpa stake, tanpa pendaftaran. Node berpartisipasi dalam produksi blok dan memvalidasi registri manusia. Operator node mendapatkan bagian biaya protokol melalui Pool Validator (40% semua biaya swap, didistribusikan setiap hari).',
  'bootstrap-title':'Jalankan Node Anda Sendiri','bootstrap-desc':'Siapa pun dapat bergabung dengan jaringan Aequitas dengan menjalankan node. Unduh panduan node untuk instruksi langkah demi langkah.',
  'tech-title':'Spesifikasi Teknis','mm-config':'Konfigurasi MetaMask',
  'k-lang':'Bahasa','k-src':'Kode Sumber','evm-yes':'Ya — JSON-RPC /rpc · Kompatibel MetaMask',
  'proto-label':'Protokol Aequitas V7 — Dokumentasi Teknis',
  'ca-title':'Alamat Kontrak','ca-text':'Rantai: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 adalah satu-satunya sumber kebenaran untuk seluruh ekonomi Aequitas. Setiap saldo AEQ, setiap registrasi manusia, setiap pembayaran UBI, dan setiap penegakan batas kekayaan diatur oleh satu kontrak yang tidak dapat diubah ini — dikerahkan di Aequitas Chain, blockchain khusus yang kompatibel dengan EVM yang menjalankan mesin konsensus BlockDAG. Tidak ada kunci admin, tidak ada proxy upgrade, tidak ada pemungutan suara tata kelola yang dapat mengubah satu baris pun logikanya. Kode yang berjalan hari ini adalah kode yang akan berjalan sepuluh tahun lagi.<br><br>Kontrak BioVerifier menerima bukti zero-knowledge Groth16 yang dihasilkan sepenuhnya di perangkat Android pengguna. Ia memverifikasi secara matematis on-chain dalam ~10 ms bahwa pendaftar baru adalah manusia hidup yang unik — tanpa pernah mengetahui nama, identitas, atau data biometrik mereka. Inilah yang membuat registrasi tanpa gas dan tanpa investasi menjadi mungkin: bukti adalah satu-satunya hal yang pernah meninggalkan perangkat.<br><br>Bersama-sama, dua kontrak ini memungkinkan sesuatu yang belum pernah ada dalam sistem mata uang manapun dalam sejarah: pasokan uang yang aturannya — siapa yang mendapatkannya, berapa banyak yang ada, bagaimana redistribusinya — tidak dapat diubah oleh siapapun, perusahaan manapun, atau pemerintah manapun. Selamanya.',
  'poa-title':'1. BUKTI KEHIDUPAN — Pemulihan Saldo Tidak Aktif','poa-text':'<p>Apa yang terjadi dengan AEQ ketika orang meninggal atau menjadi tidak mampu secara permanen? Di Bitcoin, dompet yang hilang berarti pasokan yang hilang selamanya. Aequitas menyelesaikan ini melalui sistem pemulihan ketidakaktifan multi-tahap: jika dompet tidak menunjukkan aktivitas untuk jangka waktu yang lama, saldonya secara bertahap dikembalikan ke komunitas melalui pool UBI.</p>',
  'poa-box':'Tahun 0–2: Penggunaan normal — tanpa batasan<br>Tahun 2: Peringatan 1 — Guardian dapat merespons atas nama<br>Tahun 2+60h: Peringatan 2 — urgensi meningkat<br>Tahun 2+120h: Peringatan 3 — pemberitahuan terakhir<br>Tahun 2+180h: AEQ dipindahkan ke ESCROW pribadi (masih dapat dipulihkan)<br>Tahun 4: Jika masih tidak aktif — ESCROW dirilis ke Pool UBI',
  'guard-title':'2. SISTEM GUARDIAN — Perlindungan Manusia','guard-text':'<p>Bagaimana jika seseorang dirawat di rumah sakit atau tidak dapat mengakses perangkatnya selama berbulan-bulan? Sistem Guardian memungkinkan orang terpercaya — manusia terverifikasi lainnya — mengonfirmasi bahwa pemilik dompet masih hidup. Guardian memiliki nol akses keuangan: hanya dapat memanggil satu fungsi yang mereset timer ketidakaktifan. Tidak dapat memindahkan, membelanjakan, atau mengakses dana dalam keadaan apapun.</p>',
  'guard-box':'1 Guardian per manusia · harus manusia terverifikasi di Aequitas<br>Guardian HANYA dapat memanggil confirmAlive() — nol hak transaksi<br>Guardian TIDAK DAPAT memindahkan dana, mentransfer AEQ, atau mengakses dompet<br>Maksimal 3 wali per Guardian · Kunci waktu 7 hari · Tanpa hubungan melingkar',
  'dem-title':'3. DEMURRAGE — Mekanisme Anti-Penimbunan',
  'dem-box':'Tingkat: 0,5%/bulan setelah 3 bulan ketidakaktifan (berkelanjutan, tidak bertahap)<br>Timer direset secara otomatis dengan transfer, swap, atau tindakan likuiditas apapun<br>AEQ yang meluruh didistribusikan ulang ke empat pool — tidak pernah dibakar<br>Pemberitahuan 14 hari ditampilkan sekali · 7 hari diulang di setiap sesi aktif',
  'dem-text':'<p>Demurrage adalah biaya kepemilikan uang — suku bunga negatif yang membuat penimbunan mahal dan sirkulasi menarik. Eksperimen Wörgl (Austria, 1932) mengurangi pengangguran lokal 25% dalam satu tahun. Bank Sentral Austria menutupnya justru karena bekerja terlalu baik. Chiemgauer (Jerman, 2003) beroperasi dengan prinsip yang sama dengan sukses selama lebih dari 20 tahun.</p>',
  'cap-title':'4. BATAS KEKAYAAN — Penerapan Keadilan Matematis','cap-box':'Batas bootstrap: max(5,min(N,25))× saldo rata-rata saat ini<br>1–4 manusia: 5× · +1× per manusia · 25+: 25× permanen<br>Berlaku untuk SEMUA alamat kecuali 4 pool protokol<br>Kelebihan AEQ langsung didistribusikan ulang · Tanpa intervensi manual',
  'ubi-title':'5. PENDAPATAN DASAR UNIVERSAL — Redistribusi Harian','ubi-box':'Sumber pendapatan Pool UBI:<br>· 20% semua biaya swap dari pool AMM AEQ↔tUSD<br>· Overflow dari penerapan batas kekayaan<br>· Biaya demurrage dari akun tidak aktif<br>· Escrow tidak aktif dirilis setelah 4 tahun<br><br>Distribusi: Setiap 24 jam, seluruh saldo pool UBI dibagi rata di antara semua manusia terverifikasi yang terdaftar. Pool direset ke nol dan segera mulai diisi ulang dari aktivitas protokol yang berkelanjutan.',
  'inf-title':'6. TANPA INFLASI ALGORITMIK — Formula Pasokan Tetap','inf-box':'SATU-SATUNYA peristiwa yang menciptakan AEQ baru: manusia terverifikasi baru mendaftar.<br><br>Total Pasokan = Manusia Terverifikasi × 1.000 AEQ<br><br>Ini bukan kebijakan — ini diterapkan oleh protokol. Tidak ada admin yang dapat mencetak AEQ tambahan, tidak ada suara tata kelola yang dapat mengubah penerbitan. AEQ adalah satu-satunya cryptocurrency di mana total pasokan ditentukan semata-mata oleh jumlah manusia hidup yang terverifikasi.',
  'phases-desc':'Pada Fase 0, batas kekayaan menggunakan pengganda bootstrap: max(5, min(N, 25))× saldo rata-rata. Dengan 1–4 manusia: 5× rata-rata. Setiap manusia baru menambah 1×. Pada 25+ manusia: terkunci permanen di 25×. Fase 1+ mempertahankan 25× tetap. Semua transisi otomatis — tanpa pemungutan suara, tanpa kunci admin.',
  'p0':'Bootstrap · &lt;100 manusia · Batas Kekayaan: max(5,min(N,25))× rata-rata · Meluncur 5×→25× hingga manusia ke-25 · Saat ini aktif',
  'p1':'Pertumbuhan · 100–10.000 manusia · Batas Kekayaan: 25× saldo rata-rata',
  'p2':'Stabilitas · 10.000–1M manusia · Batas Kekayaan: 25× saldo rata-rata',
  'p3':'Kematangan · 1M+ manusia · Batas Kekayaan: 25× saldo rata-rata',
  'wealth-cap-explain':'Batas Kekayaan pada Fase 0 (Bootstrap) menggunakan max(5, min(N, 25))× saldo AEQ rata-rata, di mana N = manusia terdaftar. 1–4 manusia: 5× rata-rata. Setiap manusia baru menambah 1×. 25+ manusia: terkunci permanen di 25×. Batas selalu mengikuti saldo rata-rata saat ini.',
  'btn-download-app':'UNDUH APLIKASI AEQUITAS',
  'swap-title':'🔄 Tukar AEQ ↔ tUSD','swap-sub':'Tukarkan AEQ dengan tUSD (dolar uji simulasi) melalui pool likuiditas asli. Biaya 0,1% hanya berlaku untuk pertukaran — transfer AEQ biasa antar orang tetap sepenuhnya gratis.',
  'swap-priv-bar':'🔒 Hanya 0,1% biaya swap · Transfer AEQ-ke-AEQ gratis · tUSD adalah mata uang uji tanpa nilai nyata',
  'swap-your-aeq':'AEQ Anda','swap-your-tusd':'tUSD Anda',
  'swap-fee-est':'Biaya protokol (0,1%)','swap-details-hdr':'Detail Pertukaran',
  'swap-out-lbl':'Anda terima (est.)','swap-impact-lbl':'Dampak harga','swap-rate-lbl':'Nilai tukar',
  'swap-depth-lbl':'Komposisi Pool','amm-title':'x × y = k — AMM Produk Konstan',
  'amm-text':'Saat Anda menukar AEQ dengan tUSD, cadangan AEQ bertambah dan cadangan tUSD berkurang — produknya selalu sama dengan k. Pertukaran lebih besar menyebabkan dampak harga lebih besar. Biaya 0,1% dipotong sebelum rumus diterapkan.',
  'swap-btn-conn':'🦊 HUBUNGKAN METAMASK','swap-btn-go':'🔄 TUKAR',
  'swap-log-hint':'// Hubungkan dompet untuk menukar...',
  'swap-no-liquidity':'Belum punya tUSD?','swap-faucet-desc':'Manusia terdaftar dapat klaim tUSD uji sekali','swap-btn-faucet':'💧 KLAIM tUSD UJI',
  'swap-addliq-title':'Sediakan Likuiditas','swap-addliq-desc':'Jadilah yang pertama menyetor — rasio Anda menetapkan harga awal.','swap-btn-addliq':'💧 TAMBAH LIKUIDITAS',
  'swap-lp-title':'Posisi LP Anda','swap-lp-share':'Bagian Pool','swap-lp-withdrawable':'Dapat Ditarik',
  'swap-lp-pct-label':'% posisi Anda','swap-lp-youget':'Anda akan terima','swap-btn-removeliq':'🔥 HAPUS LIKUIDITAS',
  'swap-pool-title':'AEQ / tUSD — Status Pool',
  'swap-pool-aeq':'Cadangan AEQ','swap-pool-tusd':'Cadangan tUSD','swap-pool-price':'Harga Spot',
  'swap-fee-bps':'Biaya Swap',
  'swap-pools-addr-title':'Alamat Pool Tokenomik','pools-addr-title':'Alamat Kontrak Pool',
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
  'usp-c3-title':'Dapat diakses semua orang','usp-c3-desc':'Tanpa rekening bank, kartu kredit, atau dokumen ID, tanpa perangkat keras tambahan untuk dibeli — hanya kunci layar yang sudah ada di ponsel Android Anda.',
  'usp-c4-title':'UBI harian selamanya','usp-c4-desc':'Setelah terdaftar, Anda secara otomatis menerima bagian harian dari pembayaran UBI — setiap hari, tanpa tindakan apa pun.',
  'v7-intro-title':'Apa itu AequitasV7?',
  'v7-intro-text':'AequitasV7 adalah kontrak pintar inti dari protokol Aequitas. "V7" mengacu pada versi utama ke-7 dari kontrak keadilan. Dikerahkan secara tidak dapat diubah di Aequitas Chain (ID 1926) dan menangani setiap aspek: pendaftaran manusia, verifikasi ZK, manajemen saldo, batas kekayaan, distribusi UBI, biaya swap. Tidak ada admin yang dapat memperbaruinya. Keenam mekanisme membentuk sistem yang saling memperkuat.',
  'explore-title':'Jelajahi Aequitas',
  'expl-score':'Skor Kesetaraan','expl-score-d':'Koefisien Gini langsung · Indeks Aequitas · distribusi kekayaan secara real time',
  'expl-economy':'UBI &amp; Pool Redistribusi','expl-economy-d':'Hitung mundur UBI harian · 4 pool on-chain · demurrage · Fase Protokol',
  'expl-charts':'Grafik &amp; Riwayat','expl-charts-d':'Riwayat Gini · kurva Lorenz · slider bootstrap batas kekayaan · Kisah Aequitas',
  'expl-v7':'Dokumentasi Protokol V7','expl-v7-d':'Kontrak AequitasV7 · 6 mekanisme · bukti ZK · batas kekayaan · demurrage · kode tak berubah',
  'expl-explorer':'Block Explorer','expl-explorer-d':'BlockDAG langsung · klik blok apapun untuk melihat validator, hash, transaksi, hash induk',
  'swap-sell-label':'Jual','swap-receive-label':'Terima',
  'gini-what-title':'Apa itu Koefisien Gini?','gini-what-text':'Dikembangkan oleh ahli statistik Italia Corrado Gini (1912). Mengukur distribusi kekayaan dengan membandingkan saldo aktual dengan basis yang secara hipotetis sepenuhnya setara. Skala: 0 (semua sama) hingga 1 (satu orang menguasai semua). Digunakan oleh Bank Dunia, OECD, PBB untuk membandingkan negara. Nilai referensi: Bitcoin ≈ 0,85 · Afrika Selatan (rekor dunia) ≈ 0,63 · AS ≈ 0,41 · Jerman ≈ 0,31 · Skandinavia ≈ 0,27 · Target jangka panjang Aequitas: Gini di bawah 0,30.','gini-calc-title':'Bagaimana Indeks Aequitas dihitung','gini-calc-text':'Semua saldo AEQ dikumpulkan. Rumus menghitung perbedaan absolut rata-rata dinormalisasi dengan n2. Hasil 0-1 dikali 100 = Indeks Aequitas.','gini-why-title':'Mengapa Gini','gini-why-text':'Koefisien Gini menangkap distribusi lengkap semua manusia terverifikasi.','expl-network':'Jaringan &amp; Node','expl-network-d':'Topologi node · jalankan node sendiri · spesifikasi teknis · Chain ID 1926',
  'guard-title':'🛡 Sistem Guardian','guard-my-lbl':'Guardian Saya','guard-none':'Tidak Ada',
  'guard-set-lbl':'Tetapkan / Ubah Guardian','guard-set-hint':'Harus manusia Aequitas yang terdaftar · Kunci waktu 7 hari · Guardian hanya bisa mengkonfirmasi kelayakan hidup Anda, tidak mengakses dana · Maks. 3 wali per guardian',
  'guard-confirm-lbl':'Konfirmasi Masih Hidup (Sebagai Guardian)','guard-confirm-hint':'Jika wali Anda tidak dapat mengakses wallet mereka, konfirmasi kelayakan hidup mereka untuk mencegah dana mereka berpindah ke escrow setelah 910 hari tidak aktif.','guard-recover-btn':'🔓 PULIHKAN DARI ESCROW',
  'faq-title':'❓ Pertanyaan Umum','faq-q1':'Apakah data biometrik saya aman?','faq-a1':'Ya. Tidak ada data biometrik yang pernah ditangkap, diproses, atau ditransmisikan oleh Aequitas. Layar kunci ponsel Anda hanya memberi akses ke kunci acak yang dibuat dan disimpan di perangkat keras amannya. Hanya bukti matematis yang diturunkan dari kunci tersebut yang pernah dikirim — bukan kuncinya, bukan data biometrik apa pun.',
  'faq-q1b':'Apakah pendaftaran membuktikan saya orang unik yang nyata?','faq-a1b':'Belum sepenuhnya. Bukti saat ini secara kriptografis membuktikan Anda menguasai kunci aman satu perangkat tertentu — ini secara andal mencegah perangkat (atau wallet) yang sama mendaftar dua kali, tetapi belum bisa membedakan dua perangkat fisik berbeda milik orang yang sama. Pemeriksaan keunikan biologis lintas-perangkat yang sesungguhnya sudah direncanakan, belum tersedia — kami lebih memilih mengatakannya secara terus terang daripada melebih-lebihkan jaminan bukti berbasis perangkat saat ini.',
  'faq-q2':'Bisakah saya mendaftar dengan wallet berbeda nanti?','faq-a2':'Tidak. Pendaftaran terikat permanen ke satu alamat dompet per kunci perangkat. Ini disengaja — mencegah pendaftaran ulang perangkat yang sama dan memastikan satu dompet per identitas perangkat.',
  'faq-q3':'Apa yang terjadi jika saya kehilangan ponsel?','faq-a3':'AEQ Anda tetap di wallet — terikat ke kunci privat Anda, bukan ponsel. Anda masih bisa mengakses wallet melalui MetaMask dengan frasa benih. Pemulihan wallet tidak bergantung pada pendaftaran biometrik.',
  'path-title':'Pilih Jalur Anda','path-human-title':'Saya adalah Manusia','path-human-desc':'Saya ingin mendaftar, menerima 1.000 AEQ, dan bergabung dengan jaringan penghasilan dasar.','path-human-steps':'1. Unduh Aplikasi Android Aequitas<br>2. Buka kunci dengan kunci layar perangkat Anda (sidik jari/wajah/PIN)<br>3. Hubungkan MetaMask<br>4. Terima 1.000 AEQ seketika',
  'path-node-title':'Saya adalah Operator Node','path-node-desc':'Saya ingin menjalankan node penuh, berpartisipasi dalam produksi blok, dan menghasilkan dari pool validator 40%.','path-node-steps':'1. Daftar sebagai manusia (wajib)<br>2. Set PRIMARY_NODE_URL=https://aequitas.digital<br>3. Deploy di Railway/Contabo/VPS<br>4. Hasilkan harian dari pool validator',
  'path-dev-title':'Saya adalah Pengembang','path-dev-desc':'Saya ingin membangun di Aequitas, mengintegrasikan API, atau berkontribusi pada protokol.','path-dev-steps':'1. JSON-RPC kompatibel EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Metrik: /metrics (Prometheus)',
  'story-flow-title':'Diagram Aliran Token AEQ','story-topo-title':'Topologi Jaringan — Status Saat Ini',
  'swap-price-title':'AEQ / tUSD — Harga Live','swap-price-desc':'Harga real-time dari cadangan pool (x·y=k). Diperbarui setiap 8 detik dengan data pool terbaru.','swap-price-empty':'Belum ada data pool — tambahkan likuiditas untuk melihat grafik harga.',
  'node-guide-lang-note':'Panduan inline ini dalam bahasa Inggris. PDF terjemahan tersedia dalam bahasa Anda menggunakan tombol di atas.',
  'k-zkp':'Sistem ZKP','k-hash':'Sistem Hash','k-sybil-prot':'Perlindungan Sybil',
},
it:{
  'logo-sub':'PROVA DI UMANITÀ','live':'LIVE',
  'tab-register':'🔐 Registrati','tab-explorer':'🔍 Explorer','tab-humans':'👥 Umani','tab-index':'📊 Indice','tab-network':'🌐 Rete','tab-protocol':'📜 Protocollo V7','tab-swap':'🔄 Scambia',
  'reg-title':'🔐 Registrati come Umano Verificato',
  'reg-sub':'Unisciti alla rete Aequitas e ricevi il tuo sussidio di Reddito Universale di Base di 1.000 AEQ. Una tantum, permanente e completamente gratuito. Nessun dato personale viene mai memorizzato.',
  'app-title':'REGISTRAZIONE SOLO VIA APP ANDROID',
  'app-text':'La registrazione genera una chiave crittografica all\'interno dell\'hardware sicuro del tuo telefono (Secure Enclave / StrongBox), protetta dal blocco schermo del dispositivo stesso — nessun sensore separato, nessun dato biometrico viene mai prodotto, elaborato o trasmesso. Una prova ZK Groth16 dimostra che possiedi quella chiave senza rivelarla. I tuoi 1.000 AEQ vengono accreditati automaticamente al momento della verifica. Nota: questo dimostra attualmente il controllo di un dispositivo, non l\'unicità biologica tra dispositivi — vedi le FAQ.',
  's1t':'Chiave del Dispositivo','s1d':'L\'hardware sicuro del tuo telefono genera una chiave privata protetta dal tuo blocco schermo esistente (impronta/volto/PIN, qualunque tu già usi). Nessun kit di sensori separato, nessun dato biometrico grezzo lascia mai il dispositivo.',
  's2t':'Generazione Prova ZK','s2d':'Una prova ZK Groth16 impegna la tua chiave del dispositivo in un unico commitment e nullifier senza rivelare la chiave stessa. Questo dimostra crittograficamente che possiedi la chiave di questo dispositivo — vedi le FAQ per cosa garantisce e cosa no.',
  's3t':'Connetti Wallet','s3d':'L\'app apre MetaMask su questa pagina · connetti il tuo wallet Ethereum · la prova è crittograficamente legata al tuo indirizzo',
  's4t':'1.000 AEQ Accreditati','s4d':'Registrazione confermata su Aequitas BlockDAG entro 1 secondo · 1.000 AEQ accreditati istantaneamente · la tua identità è registrata permanentemente come umano verificato',
  'priv-bar':'🔒 Chiave crittografica legata al dispositivo · Groth16 ZKP · I dati non lasciano mai il dispositivo · Una registrazione per dispositivo',
  'conn-wallet':'WALLET CONNESSO','proof-recv':'⚡ PROVA ZK RICEVUTA','proof-hint':'Connetti wallet per registrarti',
  'btn-conn':'🦊 CONNETTI METAMASK','btn-reg':'🔐 REGISTRA ON-CHAIN',
  'btn-wc':'🔗 CONNETTI WALLETCONNECT',
  'reg-log-hint':'// Apri l\'App Android Aequitas per generare la tua prova, poi torna qui...',
  'reg-details':'Dettagli Registrazione','k-network':'Rete','k-chainid':'ID Catena','k-grant':'Sussidio UBI',
  'k-fee':'Commissione Gas','free':'GRATUITO — completamente senza gas','k-limit':'Registrazioni','k-limit-v':'Una volta per dispositivo · permanente · immutabile',
  'k-bio':'Chiave del Dispositivo','never-stored':'Non lascia mai il tuo dispositivo — nessun dato biometrico viene prodotto o memorizzato',
  'k-proof':'Sistema di Prova','k-conf':'Conferma','k-conf-v':'Entro 1 secondo (1 blocco)',
  'k-sybil':'Protezione Sybil','k-sybil-v':'Una identità per dispositivo · blocco permanente (legato al dispositivo, non ancora al corpo)',
  'live-stats':'Statistiche Chain in Tempo Reale',
  's-height':'Altezza Blocco',
  's-humans':'Umani Verificati','s-humans-sub':'Prova ZK legata al dispositivo · una registrazione per dispositivo',
  's-supply':'Offerta Totale','s-supply-sub':'Sempre = Umani × 1.000 AEQ',
  's-index':'Indice Aequitas','s-index-sub':'0 = perfetta uguaglianza · 100 = massima disuguaglianza',
  's-uptime':'Uptime','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'Prova di Umanità','ib-poh-t':'Ogni detentore di AEQ dimostra crittograficamente il controllo di una chiave legata al dispositivo tramite una prova ZK Groth16. Nessun bot, nessuna azienda, nessuna IA. I dati biometrici non vengono mai trasmessi, solo una prova matematica. Questo lega oggi una registrazione per dispositivo, non ancora per persona unica — vedi le FAQ.',
  'ib-fair':'Distribuzione Radicalmente Equa','ib-fair-t':'Ogni umano verificato riceve esattamente 1.000 AEQ alla registrazione. Nessun pre-mining, nessuna allocazione ai fondatori. L\'offerta totale è sempre uguale a umani verificati × 1.000.',
  'ib-dag':'Architettura BlockDAG','ib-dag-t':'Più blocchi possono essere prodotti simultaneamente e uniti. Throughput più alto, latenza più bassa rispetto alle blockchain lineari tradizionali.',
  'ib-gas':'Veramente Senza Gas','ib-gas-t':'La registrazione e i trasferimenti AEQ non costano assolutamente nulla. Non servono ETH, BNB o MATIC. Nessun conto bancario, nessuna carta di credito.',
  'recent-blocks':'Blocchi Recenti','blocks-desc':'MERGE = più genitori uniti (BlockDAG). TX = transazione di registrazione. Tempo blocco: __BT__.',
  'loading':'Caricamento blocchi...','net-info':'Info Rete','k-chain':'Nome Catena','k-symbol':'Simbolo','k-btime':'Tempo Blocco',
  'k-cons':'Consenso','k-nodes':'Node Attivi','k-storage':'Archiviazione','add-mm':'🦊 AGGIUNGI A METAMASK','k-dec':'Decimali',
  'btn-add-mm':'+ AGGIUNGI RETE AEQUITAS',
  'phil':'"Il denaro esiste perché le persone esistono.<br>Niente di più, niente di meno."','phil-sub':'— IL PRINCIPIO AEQUITAS —',
  'humans-title':'Umani Verificati su Aequitas Chain',
  'h-what':'Cos\'è un Umano Verificato?','h-what-t':'Un Umano Verificato è attualmente un indirizzo wallet che ha dimostrato crittograficamente il controllo della chiave sicura di uno specifico dispositivo. La chiave viene generata nell\'hardware sicuro del telefono, protetta dal blocco schermo del dispositivo — nessun kit di sensori separato. Viene trasmessa solo una prova ZK Groth16, nessun dato biometrico lascia il dispositivo. Questo verifica oggi un dispositivo, non necessariamente ancora una persona unica — vedi le FAQ.',
  'h-zkp':'Sistema di Prova a Conoscenza Zero','h-zkp-t':'Aequitas usa Groth16 su BN128 — stessa curva di Ethereum e Zcash. ~200 byte, ~10ms. commitment = keccak256(deviceKey‖wallet). Il nullifier è legato a questo dispositivo: perdere il telefono non crea una seconda identità su di esso, ma un altro dispositivo può comunque registrarsi separatamente. Nessun materiale della chiave viene mai rivelato o memorizzato lato server.',
  'h-sybil':'Resistenza Sybil — Stato Attuale','h-sybil-t':'Il nullifier odierno deriva da una chiave hardware legata al dispositivo — impedisce in modo affidabile la doppia registrazione dello stesso dispositivo o wallet. Non rileva ancora se la stessa persona si registra da un secondo dispositivo fisico. Colmare questa lacuna richiede una vera verifica di unicità biologica tra dispositivi, pianificata per dopo la beta, non ancora rilasciata.',
  'h-global':'Inclusione Finanziaria Globale','h-global-t':'Nessun conto bancario, nessuna carta di credito, nessuna criptovaluta precedente necessaria. Solo uno smartphone Android con il blocco schermo (impronta/volto/PIN) che già usi. Aequitas è progettato per essere accessibile a ogni essere umano sulla Terra.',
  'h-bio-hw':'Roadmap di Verifica dell\'Identità','h-bio-hw-t':'Oggi (beta): una chiave hardware legata al dispositivo per dispositivo, onestamente etichettata come legata al dispositivo anziché al corpo. Pianificato (post-beta): una vera verifica di unicità biologica tra dispositivi — specificata, costruita e verificata in modo indipendente prima di fare un\'affermazione più forte sulla resistenza Sybil.',
  'reg-humans':'Umani Registrati','h-desc':'Ogni indirizzo ha dimostrato tramite prova ZK il controllo di una chiave crittografica legata al dispositivo e ha ricevuto esattamente 1.000 AEQ. Il registro è permanente, immutabile e on-chain. Vedi le FAQ per cosa significa oggi "legato al dispositivo" per la resistenza Sybil.',
  'no-humans':'Nessun umano registrato ancora.\n\nScarica l\'App Android Aequitas e sii il primo umano sulla chain!',
  'reg-stats':'Statistiche Registro','total-humans':'Totale Umani',
  'idx-title':'Indice Aequitas — Punteggio di Uguaglianza Economica in Tempo Reale',
  'idx-desc':'L\'Indice Aequitas misura la disuguaglianza economica tra tutti gli umani verificati in tempo reale. È derivato dal coefficiente Gini della distribuzione dei saldi on-chain. 0 = perfetta uguaglianza. 100 = massima disuguaglianza. Il protocollo attiva automaticamente i meccanismi di redistribuzione quando l\'indice sale.',
  'curr-idx':'Indice Attuale','bar-0':'0 — Perfetta Uguaglianza','bar-100':'100 — Massima Disuguaglianza','wcap-lbl':'Tetto Patrimoniale Attuale:','wcap-mult':'Moltiplicatore:','wcap-avg':'Saldo medio:',
  'gini':'Coefficiente Gini','gini-desc':'0 = uguale · 1 = disuguale',
  'supply-desc':'Sempre = Umani × 1.000 AEQ',
  'phase':'Fase Protocollo','phase-desc':'Avanza automaticamente per numero di umani',
  'humans-desc':'Registrazioni verificate ZK legate al dispositivo',
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
  'story-text':'<p>L\'anno è 2009. Satoshi Nakamoto rilascia Bitcoin. Per la prima volta, il valore può trasferirsi tra due persone senza una banca. Una vera rivoluzione. Ma quasi immediatamente qualcosa va storto.</p><p>I primi miner accumulano milioni di monete a costo quasi zero. Entro il 2021, l\'1% superiore degli indirizzi Bitcoin controlla oltre il 90% di tutti i Bitcoin. Il coefficiente Gini stimato di Bitcoin supera 0,85 — più alto di qualsiasi paese sulla Terra. La criptovaluta che avrebbe dovuto democratizzare la finanza ha creato la più estrema concentrazione di ricchezza nella storia umana.</p><p><span style="color:var(--gold)">Aequitas</span> — Latino per "equità" e "uguaglianza" — è stato creato per rispondere a una singola domanda: <em style="color:var(--gold)">"Come sarebbe una criptovaluta progettata dai principi fondamentali per essere equa per ogni essere umano?"</em></p><p>La risposta è semplice: <strong style="color:var(--text)">Il denaro esiste perché le persone esistono. Quindi ogni persona dovrebbe avere una quota uguale di denaro semplicemente in virtù di essere umana.</strong></p><p>Aequitas implementa questo matematicamente. Ogni umano verificato riceve 1.000 AEQ. Nessun mining, nessuno staking, nessun vantaggio per i primi adottanti. Il protocollo si adatta automaticamente man mano che la rete cresce.</p><p><em style="color:var(--gold)">"Il denaro esiste perché le persone esistono. Niente di più, niente di meno."</em></p>',
  'nodes-title':'Node Attivi — Topologia Attuale della Rete',
  'nodes-desc':'La rete Aequitas opera attualmente su più node distribuiti geograficamente (numero attuale sopra). Tutti partecipano alla produzione di blocchi, sincronizzazione dello stato e servizio API. Comunicano peer-to-peer via libp2p e sincronizzano lo stato dei blocchi via HTTP. La rete è progettata per supportare node aggiuntivi.',
  'run-node-title':'Esegui il Tuo Node — Aiuta a Proteggere la Rete',
  'run-node-desc':'Chiunque può eseguire un node Aequitas — senza permesso, senza stake, senza candidatura richiesta. I node partecipano alla produzione di blocchi e validano il registro umano. Gli operatori di node guadagnano una quota delle commissioni del protocollo tramite il Pool Validatori (40% di tutte le commissioni di swap, distribuite quotidianamente).',
  'bootstrap-title':'Connettere un Nuovo Node','bootstrap-desc':'Per eseguire il tuo node, imposta PRIMARY_NODE_URL=https://aequitas.digital nel tuo ambiente. Il tuo node si sincronizzerà automaticamente con lo stato completo della chain.',
  'tech-title':'Specifiche Tecniche','mm-config':'Configurazione MetaMask',
  'k-lang':'Lingua','k-src':'Codice Sorgente','evm-yes':'Sì — JSON-RPC /rpc · Compatibile MetaMask',
  'proto-label':'Protocollo Aequitas V7 — Documentazione Tecnica',
  'ca-title':'Indirizzi Contratto','ca-text':'Chain: Aequitas Chain (ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Principale): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 è l\'unica fonte di verità per l\'intera economia Aequitas. Ogni saldo AEQ, ogni registrazione umana, ogni pagamento UBI e ogni applicazione del limite di ricchezza è governato da questo unico contratto immutabile — distribuito su Aequitas Chain, una blockchain personalizzata compatibile con EVM che esegue un motore di consenso BlockDAG. Non c\'è chiave amministratore, nessun proxy di aggiornamento, nessun voto di governance che possa cambiare una singola riga della sua logica. Il codice che funziona oggi è il codice che funzionerà tra dieci anni.<br><br>Il contratto BioVerifier riceve prove a conoscenza zero Groth16 generate interamente sul dispositivo Android dell\'utente. Verifica matematicamente on-chain in ~10 ms che un nuovo registrante è un essere umano unico e vivo — senza mai conoscere il suo nome, identità o dati biometrici. Questo è ciò che rende possibile la registrazione senza gas e senza investimenti: la prova è l\'unica cosa che lascia mai il dispositivo.<br><br>Insieme, questi due contratti rendono possibile qualcosa che non è mai esistito in nessun sistema monetario nella storia: un\'offerta monetaria le cui regole — chi la ottiene, quanta ne esiste, come si ridistribuisce — non può essere alterata da nessuna persona, azienda o governo. Mai.',
  'poa-title':'1. PROVA DI VITA — Recupero Saldi Inattivi','poa-text':'<p>Cosa succede all\'AEQ quando le persone muoiono o diventano permanentemente incapaci? In Bitcoin, i portafogli persi significano fornitura persa permanentemente. Aequitas risolve questo con un sistema di recupero dell\'inattività a più fasi: se un portafoglio non mostra attività per un periodo prolungato, il suo saldo viene gradualmente restituito alla comunità attraverso il pool UBI.</p>',
  'poa-box':'Anno 0–2: Uso normale — nessuna restrizione<br>Anno 2: Avviso 1 — il Guardian può rispondere a nome<br>Anno 2+60g: Avviso 2 — urgenza crescente<br>Anno 2+120g: Avviso 3 — avviso finale<br>Anno 2+180g: AEQ spostato in ESCROW personale (ancora recuperabile)<br>Anno 4: Se ancora inattivo — ESCROW rilasciato al Pool UBI',
  'guard-title':'2. SISTEMA GUARDIAN — Protezione Umana','guard-text':'<p>E se qualcuno è ricoverato in ospedale o non riesce ad accedere al proprio dispositivo per mesi? Il sistema Guardian permette a una persona di fiducia — un altro umano verificato — di confermare che il proprietario del portafoglio è ancora vivo. Il Guardian ha accesso finanziario strettamente nullo: può solo chiamare una singola funzione che reimposta il timer di inattività. Non può spostare, spendere o accedere ai fondi in nessuna circostanza.</p>',
  'guard-box':'1 Guardian per umano · deve essere un umano verificato su Aequitas<br>Il Guardian può SOLO chiamare confirmAlive() — zero diritti di transazione<br>Il Guardian NON PUÒ spostare fondi, trasferire AEQ o accedere al portafoglio<br>Massimo 3 tutelati per Guardian · Blocco di 7 giorni all\'assegnazione · Nessuna relazione circolare',
  'dem-title':'3. DEMURRAGE — Meccanismo Anti-Accumulo',
  'dem-box':'Tasso: 0,5%/mese dopo 3 mesi di inattività (continuo, non a gradini)<br>Il timer si azzera automaticamente con qualsiasi trasferimento, swap o azione di liquidità<br>AEQ decaduto ridistribuito ai quattro pool — mai bruciato<br>Avviso di 14 giorni mostrato una volta · 7 giorni ripetuto in ogni sessione attiva',
  'dem-text':'<p>Il demurrage è un costo di detenzione sul denaro — un tasso di interesse negativo che rende costoso accumulare e attraente la circolazione. L\'esperimento di Wörgl (Austria, 1932) usò una valuta con demurrage e ridusse la disoccupazione locale del 25% in un anno. La Banca Centrale austriaca lo chiuse proprio perché funzionava troppo bene. Il Chiemgauer (Germania, 2003) opera con lo stesso principio con successo da oltre 20 anni.</p>',
  'cap-title':'4. LIMITE DI RICCHEZZA — Applicazione dell\'Equità Matematica','cap-box':'Bootstrap: max(5,min(N,25))× saldo AEQ medio<br>1–4 umani: 5× (5.000 AEQ) · Cresce 1× per umano · 25+: 25× (25.000 AEQ) permanente<br>Si applica a TUTTI gli indirizzi tranne i 4 pool del protocollo<br>L\'eccesso di AEQ viene immediatamente ridistribuito · Nessun intervento manuale',
  'ubi-title':'5. REDDITO UNIVERSALE DI BASE — Ridistribuzione Giornaliera','ubi-box':'Fonti di reddito del Pool UBI:<br>· 20% di tutte le commissioni di swap del pool AMM AEQ↔tUSD<br>· Overflow dall\'applicazione del limite di ricchezza<br>· Addebiti di demurrage da account inattivi<br>· Escrow inattivo rilasciato dopo 4 anni<br><br>Distribuzione: Ogni 24 ore, l\'intero saldo del pool UBI viene diviso equamente tra tutti gli umani verificati registrati. Il pool si azzera e inizia immediatamente a riempirsi di nuovo dall\'attività continua del protocollo.',
  'inf-title':'6. NESSUNA INFLAZIONE ALGORITMICA — Formula di Fornitura Fissa','inf-box':'L\'UNICO evento che crea nuovo AEQ: un nuovo umano verificato si registra.<br><br>Offerta Totale = Umani Verificati × 1.000 AEQ<br><br>Questo non è una politica — è applicato dal protocollo. Nessun amministratore può coniare AEQ aggiuntivo, nessun voto di governance può modificare l\'emissione. AEQ è l\'unica criptovaluta in cui l\'offerta totale è determinata esclusivamente dal numero di esseri umani vivi verificati.',
  'phases-desc':'In Fase 0 (Bootstrap) il limite di ricchezza usa un moltiplicatore scorrevole: max(5, min(N, 25))× saldo medio. Con 1–4 umani: 5× media. Ogni nuovo umano aggiunge 1×. A 25+ umani: bloccato permanentemente a 25×. Fase 1+ mantiene 25× fisso. Tutte le transizioni sono automatiche — nessun voto, nessuna chiave admin.',
  'p0':'Bootstrap · &lt;100 umani · Limite di Ricchezza: max(5,min(N,25))× media · Scorre 5×→25× fino al 25° umano · Attualmente attivo',
  'p1':'Crescita · 100–10.000 umani · Limite di Ricchezza: 25× saldo medio',
  'p2':'Stabilità · 10.000–1M umani · Limite di Ricchezza: 25× saldo medio',
  'p3':'Maturità · 1M+ umani · Limite di Ricchezza: 25× saldo medio',
  'wealth-cap-explain':'Il Limite di Ricchezza in Fase 0 (Bootstrap) usa max(5, min(N, 25))× saldo AEQ medio, dove N = umani registrati. 1–4 umani: 5× media. Ogni nuovo umano aggiunge 1×. 25+ umani: bloccato permanentemente a 25×. Il limite si adatta sempre al saldo medio corrente.',
  'btn-download-app':'SCARICA L\'APP AEQUITAS',
  'swap-title':'🔄 Scambia AEQ ↔ tUSD','swap-sub':'Scambia AEQ con tUSD (un dollaro di test simulato) attraverso il pool di liquidità nativo. Una commissione dello 0,1% si applica solo agli scambi — i normali trasferimenti AEQ tra persone rimangono completamente gratuiti.',
  'swap-priv-bar':'🔒 Solo 0,1% commissione swap · Trasferimenti AEQ-AEQ gratuiti · tUSD è una valuta di test senza valore reale',
  'swap-your-aeq':'Il tuo AEQ','swap-your-tusd':'Il tuo tUSD',
  'swap-fee-est':'Commissione protocollo (0,1%)','swap-details-hdr':'Dettagli Scambio',
  'swap-out-lbl':'Ricevi (est.)','swap-impact-lbl':'Impatto sul prezzo','swap-rate-lbl':'Tasso di cambio',
  'swap-depth-lbl':'Composizione del Pool','amm-title':'x × y = k — AMM a Prodotto Costante',
  'amm-text':'Quando scambi AEQ con tUSD, la riserva AEQ cresce e quella tUSD diminuisce — il loro prodotto rimane sempre uguale a k. Scambi più grandi causano un maggiore impatto sul prezzo. La commissione dello 0,1% viene detratta prima di applicare la formula.',
  'swap-btn-conn':'🦊 COLLEGA METAMASK','swap-btn-go':'🔄 SCAMBIA',
  'swap-log-hint':'// Collega il wallet per scambiare...',
  'swap-no-liquidity':'Nessun tUSD ancora?','swap-faucet-desc':'Gli umani registrati possono richiedere tUSD di test una volta','swap-btn-faucet':'💧 RICHIEDI tUSD DI TEST',
  'swap-addliq-title':'Fornire Liquidità','swap-addliq-desc':'Sii il primo a depositare — il tuo rapporto imposta il prezzo iniziale.','swap-btn-addliq':'💧 AGGIUNGI LIQUIDITÀ',
  'swap-lp-title':'La tua Posizione LP','swap-lp-share':'Quota del Pool','swap-lp-withdrawable':'Prelevabile',
  'swap-lp-pct-label':'% della tua posizione','swap-lp-youget':'Riceverai','swap-btn-removeliq':'🔥 RIMUOVI LIQUIDITÀ',
  'swap-pool-title':'AEQ / tUSD — Stato del Pool',
  'swap-pool-aeq':'Riserva AEQ','swap-pool-tusd':'Riserva tUSD','swap-pool-price':'Prezzo Spot',
  'swap-fee-bps':'Commissione Swap',
  'swap-pools-addr-title':'Indirizzi Pool Tokenomics','pools-addr-title':'Indirizzi Contratto Pool',
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
  'usp-c3-title':'Accessibile a tutti','usp-c3-desc':'Nessun conto bancario, carta di credito o documento d\'identità, nessun hardware aggiuntivo da acquistare — solo il blocco schermo già integrato nel tuo telefono Android.',
  'usp-c4-title':'UBI quotidiano per sempre','usp-c4-desc':'Una volta registrato, ricevi automaticamente una quota giornaliera dei pagamenti UBI — ogni giorno, senza alcuna azione richiesta.',
  'v7-intro-title':'Cos\'è AequitasV7?',
  'v7-intro-text':'AequitasV7 è il contratto intelligente centrale del protocollo Aequitas. "V7" si riferisce alla 7ª versione principale del contratto di equità. È distribuito immutabilmente su Aequitas Chain (ID 1926) e gestisce ogni aspetto: registrazione umana, verifica ZK, gestione saldi, limite di ricchezza, distribuzione UBI, commissioni swap. Nessun amministratore può aggiornarlo. I sei meccanismi formano un sistema auto-rinforzante.',
  'explore-title':'Esplora Aequitas',
  'expl-score':'Punteggio Uguaglianza','expl-score-d':'Coefficiente Gini live · Indice Aequitas · distribuzione ricchezza in tempo reale',
  'expl-economy':'UBI e Pool di Redistribuzione','expl-economy-d':'Conto alla rovescia UBI giornaliero · 4 pool on-chain · demurrage · Fasi del Protocollo',
  'expl-charts':'Grafici e Storia','expl-charts-d':'Storia Gini · curva di Lorenz · slider bootstrap limite ricchezza · La storia di Aequitas',
  'expl-v7':'Documentazione Protocollo V7','expl-v7-d':'Contratto AequitasV7 · 6 meccanismi · prova ZK · limite ricchezza · demurrage · codice immutabile',
  'expl-explorer':'Block Explorer','expl-explorer-d':'BlockDAG live · clicca qualsiasi blocco per vedere validatore, hash, transazioni, hash genitori',
  'swap-sell-label':'Vendi','swap-receive-label':'Ricevi',
  'gini-what-title':'Cos e il Coefficiente di Gini?','gini-what-text':'Sviluppato dallo statistico italiano Corrado Gini (1912). Misura la distribuzione della ricchezza confrontando i saldi reali con una linea di base ipoteticamente perfettamente equa. Scala: 0 (tutti hanno lo stesso) a 1 (una persona ha tutto). Utilizzato da Banca Mondiale, OCSE, ONU per confrontare i paesi. Valori di riferimento: Bitcoin ≈ 0,85 · Sudafrica (record mondiale) ≈ 0,63 · USA ≈ 0,41 · Germania ≈ 0,31 · Scandinavia ≈ 0,27 · Obiettivo a lungo termine di Aequitas: Gini sotto 0,30.','gini-calc-title':'Come si calcola l indice','gini-calc-text':'Vengono raccolti tutti i saldi AEQ. La formula calcola la differenza assoluta media normalizzata per n2. Risultato 0-1 x 100 = Indice Aequitas.','gini-why-title':'Perche Gini','gini-why-text':'Il coefficiente Gini cattura la distribuzione completa in un numero verificabile.','expl-network':'Rete e Nodi','expl-network-d':'Topologia nodi · esegui il tuo nodo · specifiche tecniche · Chain ID 1926',
  'guard-title':'🛡 Sistema Guardian','guard-my-lbl':'Il mio Guardian','guard-none':'Nessuno',
  'guard-set-lbl':'Imposta / Cambia Guardian','guard-set-hint':'Deve essere un umano registrato su Aequitas · Blocco temporale di 7 giorni · Il Guardian può solo confermare la tua vitalità, non accedere ai fondi · Max 3 assistiti per Guardian',
  'guard-confirm-lbl':'Conferma in Vita (Come Guardian)','guard-confirm-hint':'Se il tuo assistito non riesce ad accedere al proprio wallet, conferma la sua vitalità per evitare che i fondi vengano trasferiti in escrow dopo 910 giorni di inattività.','guard-recover-btn':'🔓 RECUPERA DALL\'ESCROW',
  'faq-title':'❓ FAQ','faq-q1':'I miei dati biometrici sono al sicuro?','faq-a1':'Sì. Nessun dato biometrico viene mai catturato, elaborato o trasmesso da Aequitas. Il blocco schermo del tuo telefono si limita a sbloccare una chiave casuale generata e conservata nel suo hardware sicuro. Viene trasmessa solo una prova matematica derivata da quella chiave — mai la chiave stessa, mai dati biometrici.',
  'faq-q1b':'La registrazione dimostra che sono una persona reale e unica?','faq-a1b':'Non ancora del tutto. La prova odierna dimostra crittograficamente che controlli la chiave sicura di un dispositivo specifico — impedisce in modo affidabile che lo stesso dispositivo (o wallet) si registri due volte, ma oggi non può distinguere due dispositivi fisici diversi posseduti dalla stessa persona. Una vera verifica di unicità biologica tra dispositivi è pianificata, non ancora realizzata — preferiamo dirlo chiaramente piuttosto che sopravvalutare ciò che la prova odierna garantisce.',
  'faq-q2':'Posso registrarmi con un wallet diverso in seguito?','faq-a2':'No. La registrazione è permanentemente vincolata a un indirizzo wallet per chiave del dispositivo. È una scelta progettuale — impedisce la nuova registrazione dello stesso dispositivo e garantisce un wallet per identità del dispositivo.',
  'faq-q3':'Cosa succede se perdo il telefono?','faq-a3':'I tuoi AEQ rimangono nel wallet — sono collegati alla tua chiave privata, non al telefono. Puoi comunque accedere al wallet tramite MetaMask con la frase seed. Il recupero del wallet è indipendente dalla registrazione biometrica.',
  'path-title':'Scegli il Tuo Percorso','path-human-title':'Sono un Umano','path-human-desc':'Voglio registrarmi, ricevere 1.000 AEQ e unirmi alla rete di reddito di base.','path-human-steps':'1. Scarica l\'App Android Aequitas<br>2. Sblocca con il blocco schermo del tuo dispositivo (impronta/volto/PIN)<br>3. Connetti MetaMask<br>4. Ricevi 1.000 AEQ istantaneamente',
  'path-node-title':'Sono un Operatore di Node','path-node-desc':'Voglio eseguire un node completo, partecipare alla produzione di blocchi e guadagnare dal pool validatori del 40%.','path-node-steps':'1. Registrarsi come umano (obbligatorio)<br>2. Impostare PRIMARY_NODE_URL=https://aequitas.digital<br>3. Distribuire su Railway/Contabo/VPS<br>4. Guadagnare giornalmente dal pool validatori',
  'path-dev-title':'Sono uno Sviluppatore','path-dev-desc':'Voglio costruire su Aequitas, integrare l\'API o contribuire al protocollo.','path-dev-steps':'1. JSON-RPC compatibile EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoint<br>4. Metriche: /metrics (Prometheus)',
  'story-flow-title':'Diagramma di Flusso Token AEQ','story-topo-title':'Topologia di Rete — Stato Attuale',
  'swap-price-title':'AEQ / tUSD — Prezzo Live','swap-price-desc':'Prezzo in tempo reale derivato dalle riserve del pool (x·y=k). Si aggiorna ogni 8 secondi con nuovi dati.','swap-price-empty':'Nessun dato del pool ancora — aggiungi liquidità per vedere il grafico dei prezzi.',
  'node-guide-lang-note':'Questa guida inline è in inglese. Un PDF tradotto nella tua lingua è disponibile tramite il pulsante sopra.',
  'k-zkp':'Sistema ZKP','k-hash':'Sistema Hash','k-sybil-prot':'Protezione Sybil',
},
tr:{
  'logo-sub':'İNSANLIK KANITI','live':'CANLI',
  'tab-register':'🔐 Kayıt','tab-explorer':'🔍 Gezgin','tab-humans':'👥 İnsanlar','tab-index':'📊 Endeks','tab-network':'🌐 Ağ','tab-protocol':'📜 Protokol V7','tab-swap':'🔄 Takas',
  'reg-title':'🔐 Doğrulanmış İnsan Olarak Kayıt Ol',
  'reg-sub':'Aequitas ağına katıl ve 1.000 AEQ Evrensel Temel Gelir hibeni al. Tek seferlik, kalıcı ve tamamen ücretsiz. Hiçbir kişisel veri asla saklanmaz.',
  'app-title':'KAYIT YALNIZCA ANDROİD UYGULAMASI İLE',
  'app-text':'Kayıt, telefonunuzun güvenli donanımı (Secure Enclave / StrongBox) içinde, cihazın kendi ekran kilidiyle korunan bir kriptografik anahtar oluşturur — ayrı bir sensör yok, hiçbir biyometrik veri asla üretilmez, işlenmez veya iletilmez. Bir Groth16 ZK kanıtı, bu anahtarı ifşa etmeden sahip olduğunuzu kanıtlar. 1.000 AEQ\'niz doğrulama sonrası otomatik olarak yatırılır. Not: bu şu anda bir cihazın kontrolünü kanıtlar, cihazlar arası biyolojik benzersizliği değil — SSS\'ye bakın.',
  's1t':'Cihaz Anahtarı','s1d':'Telefonunuzun güvenli donanımı, zaten kullandığınız ekran kilidinizin (parmak izi/yüz/PIN) arkasında bir özel anahtar oluşturur. Ayrı bir sensör kiti yok, hiçbir ham biyometrik veri cihazı asla terk etmez.',
  's2t':'ZK Kanıtı Oluşturma','s2d':'Bir Groth16 ZK kanıtı, cihaz anahtarınızı anahtarın kendisini ifşa etmeden tek bir commitment ve nullifier\'a taahhüt eder. Bu, bu cihazın anahtarına sahip olduğunuzu kriptografik olarak kanıtlar — bunun neyi garanti edip neyi garanti etmediği için SSS\'ye bakın.',
  's3t':'Cüzdan Bağla','s3d':'Uygulama bu sayfada MetaMask\'ı açar · Ethereum cüzdanını bağla · kanıt kriptografik olarak adresine bağlanır',
  's4t':'1.000 AEQ Yatırıldı','s4d':'Kayıt 6 saniye içinde Aequitas BlockDAG\'da onaylandı · 1.000 AEQ anında yatırıldı · kimliğin kalıcı olarak doğrulanmış insan olarak kaydedildi',
  'priv-bar':'🔒 Cihaza bağlı kriptografik anahtar · Groth16 ZKP · Veriler asla cihazı terk etmez · Cihaz başına bir kayıt',
  'conn-wallet':'BAĞLI CÜZDAN','proof-recv':'⚡ ZK KANITI ALINDI','proof-hint':'Kayıt için cüzdan bağla',
  'btn-conn':'🦊 METAMASK BAĞLA','btn-reg':'🔐 ZİNCİRE KAYIT OL',
  'btn-wc':'🔗 WALLETCONNECT BAĞLA',
  'reg-log-hint':'// Kanıtını oluşturmak için Aequitas Android Uygulamasını aç, ardından buraya dön...',
  'reg-details':'Kayıt Detayları','k-network':'Ağ','k-chainid':'Zincir ID','k-grant':'UBI Hibesi',
  'k-fee':'Gas Ücreti','free':'ÜCRETSİZ — tamamen gas\'sız','k-limit':'Kayıtlar','k-limit-v':'Cihaz başına bir kez · kalıcı · değiştirilemez',
  'k-bio':'Cihaz Anahtarı','never-stored':'Asla cihazınızı terk etmez — hiçbir biyometrik veri üretilmez veya saklanmaz',
  'k-proof':'Kanıt Sistemi','k-conf':'Onay','k-conf-v':'1 saniye içinde (1 blok)',
  'k-sybil':'Sybil Koruması','k-sybil-v':'Cihaz başına bir kimlik · kalıcı kilit (cihaza bağlı, henüz bedene bağlı değil)',
  'live-stats':'Canlı Zincir İstatistikleri',
  's-height':'Blok Yüksekliği',
  's-humans':'Doğrulanmış İnsanlar','s-humans-sub':'Cihaza bağlı ZK kanıtı · cihaz başına bir kayıt',
  's-supply':'Toplam Arz','s-supply-sub':'Her zaman = İnsanlar × 1.000 AEQ',
  's-index':'Aequitas Endeksi','s-index-sub':'0 = mükemmel eşitlik · 100 = maksimum eşitsizlik',
  's-uptime':'Çalışma Süresi','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'İnsanlık Kanıtı','ib-poh-t':'Her AEQ sahibi, bir Groth16 ZK kanıtı aracılığıyla cihaza bağlı bir anahtarın kontrolünü kriptografik olarak kanıtlar. Robot yok, şirket yok, yapay zeka yok. Biyometrik veriler asla iletilmez, yalnızca matematiksel bir kanıt. Bu bugün cihaz başına bir kaydı bağlar, henüz benzersiz kişi başına değil — SSS\'ye bakın.',
  'ib-fair':'Radikal Şekilde Adil Dağıtım','ib-fair-t':'Her doğrulanmış insan kayıt sırasında tam olarak 1.000 AEQ alır. Ön madencilik yok, kurucu tahsisi yok. Toplam arz her zaman doğrulanmış insanlar × 1.000 eşittir.',
  'ib-dag':'BlockDAG Mimarisi','ib-dag-t':'Birden fazla blok eş zamanlı olarak üretilebilir ve birleştirilebilir. Doğrusal blok zincirlerine kıyasla daha yüksek verim, daha düşük gecikme.',
  'ib-gas':'Gerçekten Gas\'sız','ib-gas-t':'Kayıt ve AEQ transferleri kesinlikle ücretsizdir. ETH, BNB veya MATIC gerekmez. Banka hesabı veya kredi kartı gerekmez.',
  'recent-blocks':'Son Bloklar','blocks-desc':'MERGE = birden fazla ebeveyn birleştirildi (BlockDAG). TX = kayıt işlemi. Blok süresi: __BT__. Bloka tıklayarak detayları, doğrulayıcıyı ve işlemleri görüntüle.',
  'loading':'Bloklar yükleniyor...','net-info':'Ağ Bilgisi','k-chain':'Zincir Adı','k-symbol':'Sembol','k-btime':'Blok Süresi',
  'k-cons':'Konsensüs','k-nodes':'Aktif Node\'lar','k-storage':'Depolama','add-mm':'🦊 METAMASK\'A EKLE','k-dec':'Ondalık',
  'btn-add-mm':'+ AEQUITAS AĞINI EKLE',
  'phil':'"Para insanlar var olduğu için var.<br>Bundan fazlası değil, bundan azı değil."','phil-sub':'— AEQUİTAS İLKESİ —',
  'humans-title':'Aequitas Zincirindeki Doğrulanmış İnsanlar',
  'h-what':'Doğrulanmış İnsan Nedir?','h-what-t':'Doğrulanmış İnsan, şu anda belirli bir cihazın güvenli anahtarının kontrolünü kriptografik olarak kanıtlamış bir cüzdan adresidir. Anahtar, telefonun güvenli donanımında, cihazın kendi ekran kilidiyle korunarak oluşturulur — ayrı bir sensör kiti yok. Yalnızca Groth16 ZK kanıtı iletilir, hiçbir biyometrik veri cihazı terk etmez. Bu bugün bir cihazı doğrular, henüz zorunlu olarak benzersiz bir kişiyi değil — SSS\'ye bakın.',
  'h-zkp':'Sıfır Bilgi Kanıtı Sistemi','h-zkp-t':'Aequitas, BN128 üzerinde Groth16 kullanır — Ethereum ve Zcash ile aynı eğri. ~200 bayt, ~10ms. commitment = keccak256(deviceKey‖wallet). Nullifier bu cihaza bağlıdır: telefonu kaybetmek bu cihazda ikinci bir kimlik oluşturmaz, ancak başka bir cihaz yine de ayrı olarak kayıt olabilir. Anahtar materyali sunucu tarafında asla ifşa edilmez veya saklanmaz.',
  'h-sybil':'Sybil Direnci — Mevcut Durum','h-sybil-t':'Bugünkü nullifier, cihaza bağlı bir donanım anahtarından türetilir — bu, aynı cihazın (veya cüzdanın) iki kez kayıt olmasını güvenilir şekilde engeller, ancak aynı kişinin ikinci bir fiziksel cihazdan kayıt olup olmadığını henüz tespit edemez. Bu açığı kapatmak, planlanan ancak henüz sunulmayan gerçek bir cihazlar arası biyolojik benzersizlik kontrolü gerektirir.',
  'h-global':'Küresel Finansal Kapsayıcılık','h-global-t':'Banka hesabı, kredi kartı veya önceden kripto para gerekmez. Yalnızca zaten kullandığınız ekran kilidine (parmak izi/yüz/PIN) sahip bir Android akıllı telefon yeterlidir.',
  'h-bio-hw':'Kimlik Doğrulama Yol Haritası','h-bio-hw-t':'Bugün (beta): cihaz başına bir donanım anahtarı, bedene bağlı değil dürüstçe cihaza bağlı olarak etiketlenmiştir. Planlanan (beta sonrası): daha güçlü bir Sybil direnci iddiasında bulunmadan önce belirlenecek, oluşturulacak ve bağımsız olarak denetlenecek gerçek bir cihazlar arası biyolojik benzersizlik kontrolü.',
  'reg-humans':'Kayıtlı İnsanlar','h-desc':'Aşağıdaki her adres, ZK kanıtı aracılığıyla cihaza bağlı bir kriptografik anahtarın kontrolünü kanıtladı ve tam olarak 1.000 AEQ aldı. Kalıcı, değiştirilemez, zincir üzerinde. "Cihaza bağlı"nın bugün Sybil direnci için ne anlama geldiği için SSS\'ye bakın.',
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
  'wcap-lbl':'Mevcut Servet Tavanı:','wcap-mult':'Çarpan:','wcap-avg':'Ort. bakiye:',
  'gini':'Gini Katsayısı','gini-desc':'0 = eşit · 1 = eşitsiz',
  'supply-desc':'Her zaman = İnsanlar × 1.000 AEQ',
  'phase':'Protokol Aşaması','phase-desc':'İnsan sayısına göre otomatik ilerler',
  'humans-desc':'Cihaza bağlı ZK doğrulamalı kayıtlar',
  'pools-title':'Yeniden Dağıtım Havuzları',
  'pools-desc':'Her takas ücreti, gecikme ücreti ve servet tavanı taşması otomatik olarak dört havuza bölünür. Manuel müdahale yok. Tüm havuzlar günlük ödeme yapar.',
  'vel-pool':'Doğrulayıcı Havuzu','vel-pool-desc':'Tüm ücretlerin %40\'ı → ağı güvence altına alan node operatörleri',
  'liq-pool':'Likidite Havuzu','liq-pool-desc':'Tüm ücretlerin %30\'u → LP paylarıyla orantılı likidite sağlayıcıları',
  'ubi-pool':'UBI Havuzu','ubi-pool-desc':'Tüm ücretlerin %20\'si → her 24 saatte tüm doğrulanmış insanlar eşit olarak',
  'treasury':'Hazine','treasury-desc':'Tüm ücretlerin %10\'u → protokol geliştirme ve bakımı',
  'phases-title':'Protokol Aşamaları',
  'phases-desc':'Aşama 0\'da servet tavanı bir bootstrap çarpanı kullanır: max(5, min(N, 25))× ortalama bakiye. 1–4 insanla: 5× ortalama. Her yeni insan 1× ekler. 25+ insanda: kalıcı olarak 25×\'e sabitlenir. Aşama 1+ 25×\'i sabit tutar. Tüm geçişler otomatiktir — yönetişim oyu yok, yönetici anahtarı yok.',
  'p0':'Bootstrap · &lt;100 insan · Servet Tavanı: max(5,min(N,25))× ort. · 5×→25× arası kayar · Şu anda aktif',
  'p1':'Büyüme · 100–10.000 insan · Servet Tavanı: 25× ortalama bakiye',
  'p2':'Kararlılık · 10.000–1M insan · Servet Tavanı: 25× ortalama bakiye',
  'p3':'Olgunluk · 1M+ insan · Servet Tavanı: 25× ortalama bakiye',
  'wealth-cap-explain':'Aşama 0\'daki (Bootstrap) Servet Tavanı max(5, min(N, 25))× ortalama AEQ bakiyesi kullanır; burada N = kayıtlı insan sayısı. 1–4 insan: 5× ortalama. Her yeni insan 1× ekler. 25+ insan: kalıcı olarak 25×. Tavan her zaman mevcut ortalama bakiyeyle ölçeklenir.',
  'demurrage-title':'Gecikme Ücreti — Dolaşım Teşviki',
  'demurrage-desc':'Aequitas, tarihi tamamlayıcı para birimlerinden ilham alan bir gecikme ücreti mekanizması uygular. Atıl AEQ bakiyeleri, biriktirmeyi caydırmak için yavaşça değer kaybeder.',
  'dem-rate-k':'Bozunma Hızı','dem-rate-v':'Ayda %0,5 (sürekli, kademeli değil)',
  'dem-grace-k':'İzin Süresi','dem-grace-v':'Bozunma başlamadan önce 3 aylık hareketsizlik',
  'dem-reset-k':'Saat Sıfırlama','dem-reset-v':'Herhangi bir transfer, takas veya likidite işlemi zamanlayıcıyı sıfırlar',
  'dem-dest-k':'Bozunan AEQ şuraya gider','dem-dest-v':'Yeniden dağıtım havuzları (40/30/20/10 bölünmesi)',
  'dem-warn-k':'Uyarı Sistemi','dem-warn-v':'14 günlük bildirim (bir kez) + her girişte 7 günlük tekrarlayan hatırlatma',
  'story-title':'Aequitas\'ın Hikayesi — Neden Var Olduğu',
  'story-text':'<p>Yıl 2009. Satoshi Nakamoto Bitcoin\'i yayınlıyor. İlk kez, değer bir banka olmadan iki kişi arasında transfer edilebiliyor. Gerçek bir devrim. Ama neredeyse hemen bir şeyler ters gidiyor.</p><p>Erken madenciler neredeyse sıfır maliyetle milyonlarca coin biriktiriyor. 2021\'e kadar Bitcoin adreslerinin en üst %1\'i tüm Bitcoin\'in %90\'ından fazlasını kontrol ediyor. Bitcoin\'in tahmini Gini katsayısı 0,85\'i aşıyor — Dünya\'daki herhangi bir ülkeden daha yüksek.</p><p><span style="color:var(--gold)">Aequitas</span> — Latince "adalet" ve "eşitlik" anlamına gelir — tek bir soruyu yanıtlamak için yaratıldı: <em style="color:var(--gold)">"Her insana adil olacak şekilde ilk ilkelerden tasarlanmış bir kripto para nasıl görünürdü?"</em></p><p>Cevap basit: <strong style="color:var(--text)">Para insanlar var olduğu için var. Bu nedenle her insan, sadece insan olduğu için paradan eşit pay almalıdır.</strong></p><p><em style="color:var(--gold)">"Para insanlar var olduğu için var. Bundan fazlası değil, bundan azı değil."</em></p>',
  'nodes-title':'Aktif Node\'lar — Mevcut Ağ Topolojisi',
  'nodes-desc':'Aequitas ağı şu anda birden fazla coğrafi olarak dağıtılmış node üzerinde çalışıyor (güncel sayı yukarıda). Hepsi blok üretimine, durum senkronizasyonuna ve API hizmetine katılıyor. libp2p aracılığıyla eşler arası iletişim kuruyor ve HTTP aracılığıyla blok durumunu senkronize ediyorlar. Ağ ek node\'ları desteklemek üzere tasarlanmıştır.',
  'run-node-title':'Kendi Node\'unu Çalıştır — Ağı Güvence Altına Almaya Yardım Et',
  'run-node-desc':'Herkes bir Aequitas node\'u çalıştırabilir — izin, stake veya başvuru gerekmez. Node\'lar blok üretimine katılır ve insan kaydını doğrular. Node operatörleri, Doğrulayıcı Havuzu aracılığıyla protokol ücretlerinden pay kazanır (tüm takas ücretlerinin %40\'ı, günlük dağıtılır).',
  'bootstrap-title':'Yeni Node Bağla','bootstrap-desc':'Kendi Aequitas node\'unu çalıştırmak için PRIMARY_NODE_URL=https://aequitas.digital ortam değişkenini ayarla. Node\'un tam zincir durumunu otomatik olarak senkronize edecek ve blok üretimine başlayacak.',
  'tech-title':'Teknik Özellikler','mm-config':'MetaMask Yapılandırması',
  'k-lang':'Dil','k-src':'Kaynak Kodu','evm-yes':'Evet — JSON-RPC /rpc · MetaMask uyumlu',
  'proto-label':'Aequitas V7 Protokolü — Teknik Dokümantasyon',
  'ca-title':'Sözleşme Adresleri','ca-text':'Zincir: Aequitas Chain (Zincir ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 (Ana): 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7, tüm Aequitas ekonomisinin tek gerçek kaynağıdır. Her AEQ bakiyesi, her insan kaydı, her UBI ödemesi ve her servet tavanı uygulaması, bu tek değiştirilemez sözleşme tarafından yönetilir. Yönetici anahtarı yok, yükseltme proxy\'si yok, mantığının tek bir satırını değiştirebilecek yönetişim oyu yok. Bugün çalışan kod on yıl sonra da çalışacak koddur.',
  'poa-title':'1. HAYAT KANITI — Hareketsiz Bakiye Kurtarma','poa-text':'<p>İnsanlar ölünce veya kalıcı olarak yetersiz hale gelince AEQ\'ya ne olur? Bitcoin\'de kaybedilen cüzdanlar, kalıcı olarak kaybedilen arz anlamına gelir. Aequitas bunu çok aşamalı bir hareketsizlik kurtarma sistemiyle çözer.</p>',
  'poa-box':'Yıl 0–2: Normal kullanım — kısıtlama yok<br>Yıl 2: Uyarı 1 — Vasi adına yanıt verebilir<br>Yıl 2+60g: Uyarı 2 — artan aciliyet<br>Yıl 2+120g: Uyarı 3 — son bildirim<br>Yıl 2+180g: AEQ kişisel EMANET\'e taşındı (hâlâ kurtarılabilir)<br>Yıl 4: Hâlâ hareketsizse — EMANET UBI Havuzuna serbest bırakıldı',
  'guard-title':'2. VASİ SİSTEMİ — İnsani Güvence','guard-text':'<p>Ya biri hastanede ya da başka bir nedenle aylarca cihazına erişemiyorsa? Vasi sistemi, güvenilen bir kişinin — başka bir doğrulanmış insanın — cüzdan sahibinin hâlâ hayatta olduğunu onaylamasına izin verir. Vasinin kesinlikle sıfır finansal erişimi vardır: yalnızca hareketsizlik zamanlayıcısını sıfırlayan tek bir işlevi çağırabilir.</p>',
  'guard-box':'İnsan başına 1 Vasi · Aequitas\'ta doğrulanmış insan olmalı<br>Vasi YALNIZCA confirmAlive() çağırabilir — sıfır işlem hakkı<br>Vasi fon taşıyamaz, AEQ transfer edemez veya cüzdana erişemez<br>Vasi başına en fazla 3 korunan · 7 günlük kilit · Döngüsel ilişkiye izin yok',
  'dem-title':'3. GECİKME ÜCRETİ — Biriktirme Karşıtı Mekanizma',
  'dem-box':'Hız: 3 aylık hareketsizlikten sonra ayda %0,5 (sürekli, kademeli değil)<br>Herhangi bir transfer, takas veya likidite işlemi zamanlayıcıyı otomatik olarak sıfırlar<br>Bozunan AEQ dört havuza yeniden dağıtılır — asla yakılmaz<br>14 günlük uyarı bir kez gösterilir · 7 günlük uyarı her aktif oturumda tekrarlanır',
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
  'swap-btn-conn':'🦊 METAMASK BAĞLA','swap-btn-go':'🔄 TAKAS ET',
  'swap-log-hint':'// Takas yapmak için cüzdan bağla...',
  'swap-no-liquidity':'Henüz tUSD yok mu?','swap-faucet-desc':'Kayıtlı insanlar bir kez test tUSD talep edebilir','swap-btn-faucet':'💧 TEST tUSD TALEP ET',
  'swap-addliq-title':'Likidite Sağla','swap-addliq-desc':'İlk yatıran ol — oranın başlangıç fiyatını belirler.','swap-btn-addliq':'💧 LİKİDİTE EKLE',
  'swap-lp-title':'LP Pozisyonun','swap-lp-share':'Havuz Payı','swap-lp-withdrawable':'Çekilebilir',
  'swap-lp-pct-label':'% pozisyonun','swap-lp-youget':'Alacaksın','swap-btn-removeliq':'🔥 LİKİDİTE KALDIR',
  'swap-pool-title':'AEQ / tUSD — Havuz Durumu',
  'swap-pool-aeq':'AEQ Rezervi','swap-pool-tusd':'tUSD Rezervi','swap-pool-price':'Spot Fiyat',
  'swap-fee-bps':'Takas Ücreti',
  'swap-pools-addr-title':'Tokenomik Havuz Adresleri','pools-addr-title':'Havuz Sözleşme Adresleri',
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
  'usp-c3-title':'Herkese erişilebilir','usp-c3-desc':'Banka hesabı, kredi kartı veya kimlik belgesi gerekmez, satın alınacak ek donanım yok — yalnızca Android telefonunuzda zaten bulunan ekran kilidi.',
  'usp-c4-title':'Sonsuza kadar günlük UBI','usp-c4-desc':'Kaydolduktan sonra, her gün otomatik olarak UBI ödemelerinden pay alırsın — her gün, hiçbir işlem gerektirmez.',
  'v7-intro-title':'AequitasV7 Nedir?',
  'v7-intro-text':'AequitasV7, Aequitas protokolünün merkezi akıllı sözleşmesidir. "V7", adalet sözleşmesinin 7. ana sürümüdür. Aequitas Chain\'de (Zincir ID 1926) değiştirilemez şekilde dağıtılmıştır ve her şeyi yönetir: insan kaydı, ZK doğrulaması, bakiye yönetimi, servet tavanı, UBI dağıtımı, takas ücretleri. Hiçbir yönetici onu güncelleyemez. Altı mekanizma kendi kendini güçlendiren bir sistem oluşturur.',
  'explore-title':'Aequitas\'ı Keşfet',
  'expl-score':'Eşitlik Skoru','expl-score-d':'Canlı Gini katsayısı · Aequitas Endeksi · gerçek zamanlı servet dağılımı',
  'expl-economy':'UBI ve Yeniden Dağıtım Havuzları','expl-economy-d':'Günlük UBI geri sayımı · 4 on-chain havuz · demurrage · Protokol Aşamaları',
  'expl-charts':'Grafikler ve Tarih','expl-charts-d':'Gini geçmişi · Lorenz eğrisi · servet tavanı bootstrap kaydırıcısı · Aequitas\'ın hikayesi',
  'expl-v7':'Protokol V7 Dokümantasyonu','expl-v7-d':'AequitasV7 sözleşmesi · 6 mekanizma · ZK kanıtı · servet tavanı · demurrage · değiştirilemez kod',
  'expl-explorer':'Blok Gezgini','expl-explorer-d':'Canlı BlockDAG · doğrulayıcıyı, hash\'i, işlemleri, üst hash\'leri görmek için herhangi bir bloğa tıklayın',
  'swap-sell-label':'Sat','swap-receive-label':'Al',
  'expl-network':'Ağ ve Düğümler','expl-network-d':'Düğüm topolojisi · kendi düğümünü çalıştır · teknik özellikler · Zincir ID 1926',
  'guard-title':'🛡 Koruyucu Sistemi','guard-my-lbl':'Koruyucum','guard-none':'Yok',
  'guard-set-lbl':'Koruyucu Belirle / Değiştir','guard-set-hint':'Kayıtlı bir Aequitas insanı olmalıdır · 7 günlük zaman kilidi · Koruyucu yalnızca canlılığınızı onaylayabilir, fonlara erişemez · Koruyucu başına maks. 3 korunan',
  'guard-confirm-lbl':'Hayatta Olduğunu Onayla (Koruyucu Olarak)','guard-confirm-hint':'Korunanınız cüzdanına erişemiyorsa, 910 günlük hareketsizlik sonrasında fonlarının emanete geçmesini önlemek için canlılığını onaylayın.','guard-recover-btn':'🔓 EMANETTEN GERİ AL',
  'faq-title':'❓ Sık Sorulan Sorular','faq-q1':'Biyometrik verilerim güvende mi?','faq-a1':'Evet. Aequitas hiçbir biyometrik veriyi asla yakalamaz, işlemez veya iletmez. Telefonunuzun ekran kilidi yalnızca güvenli donanımında oluşturulan ve saklanan rastgele bir anahtara erişim sağlar. Yalnızca bu anahtardan türetilen matematiksel bir kanıt gönderilir — anahtarın kendisi asla, biyometrik veri asla.',
  'faq-q1b':'Kayıt, benzersiz gerçek bir insan olduğumu kanıtlıyor mu?','faq-a1b':'Henüz tam olarak değil. Bugünkü kanıt, yalnızca belirli bir cihazın güvenli anahtarına sahip olduğunuzu kriptografik olarak kanıtlar — aynı cihazın (veya cüzdanın) iki kez kayıt olmasını güvenilir şekilde engeller, ancak aynı kişinin sahip olduğu iki farklı fiziksel cihazı bugün ayırt edemez. Cihazlar arası gerçek biyolojik benzersizlik kontrolü planlanıyor, henüz sunulmadı — bunu açıkça söylemeyi, bugünkü cihaza bağlı kanıtın garanti ettiğinden fazlasını iddia etmeye tercih ederiz.',
  'faq-q2':'Daha sonra farklı bir cüzdanla kayıt olabilir miyim?','faq-a2':'Hayır. Kayıt, cihaz anahtarı başına bir cüzdan adresine kalıcı olarak bağlıdır. Bu tasarım gereğidir — aynı cihazın yeniden kayıt olmasını engeller ve cihaz kimliği başına bir cüzdan sağlar.',
  'faq-q3':'Telefonumu kaybedersem ne olur?','faq-a3':'AEQ\'leriniz cüzdanınızda kalır — özel anahtarınıza bağlıdır, telefonunuza değil. MetaMask\'ı tohum ifadenizle kullanarak cüzdanınıza erişmeye devam edebilirsiniz. Cüzdan kurtarma, biyometrik kayıttan bağımsızdır.',
  'path-title':'Yolunuzu Seçin','path-human-title':'Ben bir İnsanım','path-human-desc':'Kayıt olmak, 1.000 AEQ almak ve temel gelir ağına katılmak istiyorum.','path-human-steps':'1. Aequitas Android Uygulamasını indir<br>2. Cihazının ekran kilidiyle kilidini aç (parmak izi/yüz/PIN)<br>3. MetaMask\'ı bağla<br>4. Anında 1.000 AEQ al',
  'path-node-title':'Ben bir Node Operatörüyüm','path-node-desc':'Tam bir node çalıştırmak, blok üretimine katılmak ve %40 doğrulayıcı havuzundan kazanmak istiyorum.','path-node-steps':'1. İnsan olarak kayıt ol (zorunlu)<br>2. PRIMARY_NODE_URL=https://aequitas.digital ayarla<br>3. Railway/Contabo/VPS\'de dağıt<br>4. Doğrulayıcı havuzundan günlük kazan',
  'path-dev-title':'Ben bir Geliştiriciyim','path-dev-desc':'Aequitas üzerinde inşa etmek, API\'yi entegre etmek veya protokole katkıda bulunmak istiyorum.','path-dev-steps':'1. EVM uyumlu JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* uç noktaları<br>4. Metrikler: /metrics (Prometheus)',
  'story-flow-title':'AEQ Token Akış Şeması','story-topo-title':'Ağ Topolojisi — Mevcut Durum',
  'swap-price-title':'AEQ / tUSD — Canlı Fiyat','swap-price-desc':'Havuz rezervlerinden gerçek zamanlı fiyat (x·y=k). Her 8 saniyede yeni havuz verileriyle güncellenir.','swap-price-empty':'Henüz havuz verisi yok — fiyat grafiğini görmek için likidite ekleyin.',
  'node-guide-lang-note':'Bu kılavuz İngilizce\'dir. Dilinizde çevrilmiş PDF yukarıdaki düğmeyle mevcuttur.',
  'k-zkp':'ZKP Sistemi','k-hash':'Hash Sistemi','k-sybil-prot':'Sybil Koruması',
},
fr:{
  'logo-sub':'PREUVE D\'HUMANITÉ','live':'EN DIRECT',
  'tab-register':'🔐 S\'inscrire','tab-explorer':'🔍 Explorateur','tab-humans':'👥 Humains','tab-index':'📊 Index','tab-network':'🌐 Réseau','tab-protocol':'📜 Protocole V7','tab-swap':'🔄 Échanger',
  'reg-title':'🔐 S\'inscrire en tant qu\'humain vérifié',
  'reg-sub':'Rejoignez le réseau Aequitas et recevez 1 000 AEQ de Revenu de Base Universel. L\'inscription est unique, permanente et totalement sans frais. Aucune donnée personnelle n\'est stockée.',
  'app-title':'INSCRIPTION VIA L\'APPLICATION ANDROID',
  'app-text':'L\'inscription crée une clé cryptographique à l\'intérieur du matériel sécurisé de votre téléphone (Secure Enclave / StrongBox), protégée par le verrouillage d\'écran de l\'appareil lui-même — aucun capteur séparé, aucune donnée biométrique n\'est jamais générée, traitée ou transmise. Une preuve ZK Groth16 prouve que vous possédez cette clé sans la révéler. Vos 1 000 AEQ sont crédités automatiquement à la vérification. Remarque : cela prouve actuellement le contrôle d\'un appareil, pas l\'unicité biologique inter-appareils — voir la FAQ.',
  's1t':'Clé de l\'Appareil','s1d':'Le matériel sécurisé de votre téléphone génère une clé privée derrière le verrouillage d\'écran que vous utilisez déjà (empreinte/visage/code). Aucun kit de capteur séparé, aucune donnée biométrique brute ne quitte jamais l\'appareil.',
  's2t':'Génération de Preuve ZK','s2d':'Une preuve ZK Groth16 engage votre clé d\'appareil dans un commitment et un nullifier uniques, sans révéler la clé elle-même. Cela prouve cryptographiquement que vous possédez la clé de cet appareil — voir la FAQ pour ce que cela garantit et ce que cela ne garantit pas.',
  's3t':'Connecter le Portefeuille','s3d':'L\'app ouvre MetaMask · connectez votre portefeuille Ethereum · la preuve est liée cryptographiquement à votre adresse',
  's4t':'1 000 AEQ Accordés','s4d':'Inscription confirmée sur le BlockDAG en 1 seconde · 1 000 AEQ crédités instantanément · identité enregistrée en permanence',
  'priv-bar':'🔒 Clé cryptographique liée à l\'appareil · Groth16 ZKP · Les données ne quittent jamais l\'appareil · Une inscription par appareil',
  'conn-wallet':'PORTEFEUILLE CONNECTÉ','proof-recv':'⚡ PREUVE ZK REÇUE','proof-hint':'Connecter un portefeuille pour s\'inscrire',
  'btn-conn':'🦊 CONNECTER METAMASK','btn-reg':'🔐 INSCRIPTION ON-CHAIN',
  'btn-wc':'🔗 CONNECTER WALLETCONNECT',
  'reg-log-hint':'// Ouvrir l\'app Android Aequitas pour générer votre preuve, puis revenir ici...',
  'reg-details':'Détails d\'inscription','k-network':'Réseau','k-chainid':'ID de chaîne','k-grant':'Allocation UBI',
  'k-fee':'Frais de gaz','free':'GRATUIT — totalement sans frais','k-limit':'Inscriptions','k-limit-v':'Une fois par appareil · permanent · immuable',
  'k-bio':'Clé de l\'Appareil','never-stored':'Ne quitte jamais votre appareil — aucune donnée biométrique n\'est générée ni stockée',
  'k-proof':'Système de preuve','k-conf':'Confirmation','k-conf-v':'En 1 seconde (1 bloc)',
  'k-sybil':'Protection Sybil','k-sybil-v':'Une identité par appareil · verrouillage permanent (lié à l\'appareil, pas encore au corps)',
  'live-stats':'Statistiques de la chaîne en direct',
  's-height':'Hauteur de bloc',
  's-humans':'Humains vérifiés','s-humans-sub':'Preuve ZK liée à l\'appareil · une inscription par appareil',
  's-supply':'Offre totale','s-supply-sub':'Toujours = Humains × 1 000 AEQ',
  's-index':'Index Aequitas','s-index-sub':'0 = égalité parfaite · 100 = inégalité maximale',
  's-uptime':'Disponibilité','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'Preuve d\'Humanité','ib-poh-t':'Chaque détenteur d\'AEQ prouve cryptographiquement le contrôle d\'une clé liée à l\'appareil via une preuve ZK Groth16. Pas de robots, sociétés ni IA. Les données biométriques ne sont jamais transmises, seulement une preuve mathématique. Cela lie aujourd\'hui une inscription par appareil, pas encore par personne unique — voir la FAQ.',
  'ib-fair':'Distribution radicalement équitable','ib-fair-t':'Chaque humain vérifié reçoit exactement 1 000 AEQ. Pas de pré-minage ni d\'allocation fondateurs. Offre = Humains × 1 000.',
  'ib-dag':'Architecture BlockDAG','ib-dag-t':'Plusieurs blocs produits simultanément et fusionnés. Débit plus élevé, latence plus faible.',
  'ib-gas':'Vraiment sans frais','ib-gas-t':'Inscription et transferts AEQ gratuits. Pas d\'ETH, BNB ou MATIC. Pas de carte bancaire nécessaire.',
  'recent-blocks':'Blocs récents','blocks-desc':'MERGE = plusieurs parents fusionnés (BlockDAG). TX = transaction d\'inscription. Temps de bloc : __BT__.',
  'loading':'Chargement des blocs...','net-info':'Informations réseau','k-chain':'Nom de chaîne','k-symbol':'Symbole','k-btime':'Temps de bloc',
  'k-cons':'Consensus','k-nodes':'Nœuds actifs','k-storage':'Stockage','add-mm':'🦊 AJOUTER À METAMASK','k-dec':'Décimales',
  'btn-add-mm':'+ AJOUTER LE RÉSEAU AEQUITAS',
  'phil':'"L\'argent existe parce que les gens existent.<br>Rien de plus, rien de moins."','phil-sub':'— LE PRINCIPE AEQUITAS —',
  'humans-title':'Humains vérifiés sur Aequitas Chain',
  'h-what':'Qu\'est-ce qu\'un humain vérifié ?','h-what-t':'Un Humain vérifié est une adresse de portefeuille qui a cryptographiquement prouvé le contrôle de la clé sécurisée d\'un appareil spécifique à ce jour. La clé est générée dans le matériel sécurisé du téléphone, protégée par le verrouillage d\'écran de l\'appareil lui-même — aucun kit de capteur séparé. Seule une preuve ZK Groth16 est transmise, aucune donnée biométrique ne quitte l\'appareil. Cela vérifie aujourd\'hui un appareil, pas encore nécessairement une personne unique — voir la FAQ.',
  'h-zkp':'Système de preuve ZK','h-zkp-t':'Aequitas utilise Groth16 sur BN128 — même courbe qu\'Ethereum et Zcash. ~200 octets, ~10ms. commitment = keccak256(deviceKey‖wallet). Le nullifier est lié à cet appareil : perdre le téléphone ne crée pas une seconde identité sur cet appareil, mais un autre appareil peut toujours s\'inscrire séparément. Le matériel de la clé n\'est jamais révélé ni stocké côté serveur.',
  'h-sybil':'Résistance Sybil — État Actuel','h-sybil-t':'Le nullifier d\'aujourd\'hui est dérivé d\'une clé matérielle liée à l\'appareil — cela empêche de manière fiable le même appareil (ou portefeuille) de s\'inscrire deux fois, mais ne peut pas encore détecter si la même personne s\'inscrit depuis un second appareil physique. Combler cette lacune nécessite une véritable vérification d\'unicité biologique inter-appareils, prévue mais pas encore livrée.',
  'h-global':'Inclusion financière mondiale','h-global-t':'Pas de compte bancaire, carte de crédit ou crypto préalable requis. Un smartphone Android avec le verrouillage d\'écran que vous utilisez déjà (empreinte/visage/code) suffit.',
  'h-bio-hw':'Feuille de Route de Vérification d\'Identité','h-bio-hw-t':'Aujourd\'hui (bêta) : une clé matérielle par appareil, honnêtement étiquetée comme liée à l\'appareil, pas au corps. Prévu (post-bêta) : une véritable vérification d\'unicité biologique inter-appareils, à définir, construire et auditer indépendamment avant de revendiquer une résistance Sybil plus forte.',
  'reg-humans':'Humains inscrits','h-desc':'Chaque adresse ci-dessous a prouvé le contrôle d\'une clé cryptographique liée à l\'appareil via une preuve ZK et a reçu exactement 1 000 AEQ. Permanent, immuable, on-chain. Voir la FAQ pour ce que « lié à l\'appareil » signifie pour la résistance Sybil aujourd\'hui.',
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
  'curr-idx':'Index actuel','bar-0':'0 — Égalité parfaite','bar-100':'100 — Inégalité max','wcap-lbl':'Plafond de richesse :','wcap-mult':'Multiplicateur :','wcap-avg':'Solde moyen :',
  'gini':'Coefficient de Gini','gini-desc':'0 = égal · 1 = inégal',
  'supply-desc':'Toujours = Humains × 1 000 AEQ',
  'phase':'Phase du protocole','phase-desc':'Avance automatiquement par nombre d\'humains',
  'humans-desc':'Inscriptions vérifiées par ZK liées à l\'appareil',
  'pools-title':'Pools de redistribution',
  'pools-desc':'Chaque frais de swap, demurrage et dépassement du plafond est divisé entre quatre pools. Tous versent quotidiennement.',
  'vel-pool':'Pool des validateurs','vel-pool-desc':'40% de tous les frais → opérateurs de nœuds qui sécurisent le réseau',
  'liq-pool':'Pool de liquidité','liq-pool-desc':'30% de tous les frais → fournisseurs de liquidité, proportionnellement aux parts LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% de tous les frais → tous les humains vérifiés également, toutes les 24 heures',
  'treasury':'Trésorerie','treasury-desc':'10% de tous les frais → développement et maintenance du protocole',
  'phases-title':'Phases du protocole',
  'phases-desc':'Plafond bootstrap Phase 0 : max(5, min(N, 25))× solde moyen. 1–4 humains : 5×. Chaque humain ajoute 1×. 25+ humains : verrouillé à 25×. Transitions automatiques.',
  'p0':'Bootstrap · &lt;100 humains · Plafond : max(5,min(N,25))× moyen · 5×→25× · Actuellement actif',
  'p1':'Croissance · 100–10 000 humains · Plafond : 25× solde moyen',
  'p2':'Stabilité · 10 000–1M humains · Plafond : 25× solde moyen',
  'p3':'Maturité · 1M+ humains · Plafond : 25× solde moyen',
  'wealth-cap-explain':'Plafond Phase 0 : max(5, min(N, 25))× solde moyen. 1–4 humains : 5×. Chaque humain +1×. 25+ : verrouillé à 25×.',
  'demurrage-title':'Demurrage — Incitation à la circulation',
  'demurrage-desc':'Les soldes AEQ inactifs perdent lentement de la valeur pour décourager l\'accumulation.',
  'dem-rate-k':'Taux de décroissance','dem-rate-v':'0,5 % par mois (continu)',
  'dem-grace-k':'Période de grâce','dem-grace-v':'3 mois d\'inactivité avant début de décroissance',
  'dem-reset-k':'Réinitialisation','dem-reset-v':'Tout transfert, swap ou action de liquidité remet le compteur à zéro',
  'dem-dest-k':'L\'AEQ décroissant va vers','dem-dest-v':'Pools de redistribution (40/30/20/10)',
  'dem-warn-k':'Système d\'avertissement','dem-warn-v':'Avis 14 jours (une fois) + rappel 7 jours à chaque connexion',
  'story-title':'L\'histoire d\'Aequitas',
  'story-text':'<p>En 2009, Satoshi Nakamoto publie Bitcoin. Révolution genuïne — mais les premiers mineurs accumulent des millions à coût quasi nul. En 2021, le top 1% contrôle plus de 90% du Bitcoin. Gini Bitcoin &gt; 0,85.</p><p><span style="color:var(--gold)">Aequitas</span> — latin pour « équité » — répond : <em style="color:var(--gold)">« Quelle serait une cryptomonnaie conçue pour être juste envers chaque humain ? »</em></p><p><strong style="color:var(--text)">L\'argent existe parce que les gens existent. Donc chaque personne devrait avoir une part égale.</strong></p><p><em style="color:var(--gold)">« L\'argent existe parce que les gens existent. Rien de plus, rien de moins. »</em></p>',
  'nodes-title':'Nœuds actifs — Topologie réseau actuelle',
  'nodes-desc':'Le réseau Aequitas fonctionne sur plusieurs nœuds géographiquement distribués (nombre actuel ci-dessus) participant à la production de blocs, synchronisation d\'état et service API. Nœuds supplémentaires bienvenus.',
  'run-node-title':'Exécuter votre propre nœud','run-node-desc':'N\'importe qui peut exécuter un nœud Aequitas — sans permission, sans stake. Opérateurs gagnent 40% des frais de swap distribués quotidiennement.',
  'bootstrap-title':'Connecter un nouveau nœud','bootstrap-desc':'Définissez PRIMARY_NODE_URL=https://aequitas.digital dans votre environnement. Votre nœud synchronise automatiquement l\'état complet.',
  'tech-title':'Spécifications techniques','mm-config':'Configuration MetaMask',
  'k-lang':'Langue','k-src':'Source','evm-yes':'Oui — JSON-RPC /rpc · Compatible MetaMask',
  'proto-label':'Protocole Aequitas V7 — Documentation technique',
  'ca-title':'Adresses des contrats',
  'ca-text':'Chaîne : Aequitas Chain (Chain ID : 1926 · 0x786)<br>RPC : https://aequitas.digital/rpc<br><br>BioVerifier : 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7 : 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 est l\'unique source de vérité pour toute l\'économie Aequitas. Aucune clé d\'administration ni vote de gouvernance ne peut modifier sa logique. Le code actuel fonctionnera dans dix ans.',
  'poa-title':'1. PREUVE DE VIE','poa-text':'<p>Quand les gens décèdent, leurs AEQ retournent progressivement à la communauté via le pool UBI plutôt que d\'être perdus comme dans Bitcoin.</p>',
  'poa-box':'Années 0–2 : Utilisation normale<br>Année 2 : Avertissement 1 — Gardien peut répondre<br>Année 2+60j : Avertissement 2<br>Année 2+120j : Avertissement 3<br>Année 2+180j : AEQ en séquestre personnel<br>Année 4 : Si inactif — retourne au Pool UBI',
  'guard-title':'2. SYSTÈME DE GARDIEN','guard-text':'<p>Un Gardien de confiance (autre humain vérifié) peut confirmer qu\'une personne est encore en vie, sans aucun droit de transaction.</p>',
  'guard-box':'1 Gardien par humain · doit être humain vérifié Aequitas<br>Gardien peut UNIQUEMENT appeler confirmAlive() · zéro droit financier<br>Gardien NE PEUT PAS déplacer des fonds · Max 3 protégés · Timelock 7j',
  'dem-title':'3. DEMURRAGE — Anti-accumulation',
  'dem-box':'Taux : 0,5%/mois après 3 mois de grâce<br>Réinitialisation à chaque transfert, swap ou action de liquidité<br>AEQ décroissant redistribué dans les pools (non brûlé)',
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
  'swap-btn-conn':'🦊 CONNECTER METAMASK','swap-btn-go':'🔄 ÉCHANGER',
  'swap-log-hint':'// Connecter un portefeuille pour échanger...',
  'swap-no-liquidity':'Pas encore de tUSD ?','swap-faucet-desc':'Humains inscrits peuvent réclamer du tUSD test une fois','swap-btn-faucet':'💧 RÉCLAMER tUSD TEST',
  'swap-addliq-title':'Fournir de la liquidité','swap-addliq-desc':'Soyez le premier à déposer — votre ratio fixe le prix initial.','swap-btn-addliq':'💧 AJOUTER LIQUIDITÉ',
  'swap-lp-title':'Votre position LP','swap-lp-share':'Part du Pool','swap-lp-withdrawable':'Retirable',
  'swap-lp-pct-label':'% de votre position','swap-lp-youget':'Vous recevrez','swap-btn-removeliq':'🔥 RETIRER LIQUIDITÉ',
  'swap-pool-title':'AEQ / tUSD — Statut du Pool',
  'swap-pool-aeq':'Réserve AEQ','swap-pool-tusd':'Réserve tUSD','swap-pool-price':'Prix Spot',
  'swap-fee-bps':'Frais de Swap',
  'swap-pools-addr-title':'Adresses des Pools Tokenomiques','pools-addr-title':'Adresses des Contrats de Pools',
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
  'usp-c3-title':'Accessible à tous','usp-c3-desc':'Pas besoin de compte bancaire, de carte de crédit ni de pièce d\'identité, aucun matériel supplémentaire à acheter — juste le verrouillage d\'écran déjà présent sur votre téléphone Android.',
  'usp-c4-title':'UBI quotidien pour toujours','usp-c4-desc':'Une fois inscrit, votre part des paiements UBI arrive automatiquement chaque jour — sans aucune action.',
  'v7-intro-title':'Qu\'est-ce qu\'AequitasV7 ?',
  'v7-intro-text':'AequitasV7 est le contrat intelligent central d\'Aequitas. Déployé de manière immuable sur Aequitas Chain (ID 1926). Gère tout : inscription humaine, vérification ZK, soldes, plafond de richesse, UBI, frais de swap. Aucun administrateur ne peut le modifier.',
  'explore-title':'Explorer Aequitas',
  'expl-score':'Score d\'égalité','expl-score-d':'Coefficient de Gini en direct · Index Aequitas · distribution des richesses en temps réel',
  'expl-economy':'UBI &amp; Redistribution','expl-economy-d':'Compte à rebours UBI · 4 pools on-chain · demurrage · Phases du protocole',
  'expl-charts':'Graphiques &amp; Historique','expl-charts-d':'Historique Gini · courbe de Lorenz · curseur du plafond · L\'histoire d\'Aequitas',
  'expl-v7':'Docs Protocole V7','expl-v7-d':'Contrat AequitasV7 · 6 mécanismes · preuve ZK · plafond · demurrage · code immuable',
  'expl-explorer':'Explorateur de blocs','expl-explorer-d':'BlockDAG en direct · cliquez sur un bloc pour voir validateur, hash, transactions',
  'swap-sell-label':'Vendre','swap-receive-label':'Recevoir',
  'expl-network':'Réseau &amp; Nœuds','expl-network-d':'Topologie des nœuds · exécuter votre propre nœud · spécifications · Chain ID 1926',
  'guard-title':'🛡 Système de Gardien','guard-my-lbl':'Mon Gardien','guard-none':'Aucun',
  'guard-set-lbl':'Définir / Changer de Gardien','guard-set-hint':'Doit être un humain enregistré sur Aequitas · Verrou temporel de 7 jours · Le gardien peut uniquement confirmer votre vitalité, pas accéder aux fonds · Max 3 protégés par gardien',
  'guard-confirm-lbl':'Confirmer en Vie (En tant que Gardien)','guard-confirm-hint':'Si votre protégé ne peut pas accéder à son portefeuille, confirmez sa vitalité pour éviter que ses fonds soient transférés en séquestre après 910 jours d\'inactivité.','guard-recover-btn':'🔓 RÉCUPÉRER DU SÉQUESTRE',
  'faq-title':'❓ FAQ','faq-q1':'Mes données biométriques sont-elles sécurisées ?','faq-a1':'Oui. Aucune donnée biométrique n\'est jamais capturée, traitée ou transmise par Aequitas. Le verrouillage d\'écran de votre téléphone donne simplement accès à une clé aléatoire générée et stockée dans son matériel sécurisé. Seule une preuve mathématique dérivée de cette clé est envoyée — jamais la clé elle-même, jamais de données biométriques.',
  'faq-q1b':'L\'inscription prouve-t-elle que je suis une personne réelle et unique ?','faq-a1b':'Pas encore complètement. La preuve actuelle démontre cryptographiquement que vous contrôlez la clé sécurisée d\'un appareil spécifique — elle empêche de manière fiable ce même appareil (ou portefeuille) de s\'inscrire deux fois, mais ne peut pas encore distinguer deux appareils physiques différents appartenant à la même personne. Une véritable vérification d\'unicité biologique inter-appareils est prévue, mais pas encore livrée — nous préférons le dire clairement plutôt que de surestimer ce que la preuve actuelle, liée à l\'appareil, garantit.',
  'faq-q2':'Puis-je m\'inscrire avec un portefeuille différent plus tard ?','faq-a2':'Non. L\'inscription est liée en permanence à une adresse de portefeuille par clé d\'appareil. C\'est un choix de conception — cela empêche le même appareil de se réinscrire et garantit un portefeuille par identité d\'appareil.',
  'faq-q3':'Que se passe-t-il si je perds mon téléphone ?','faq-a3':'Vos AEQ restent dans votre portefeuille — ils sont liés à votre clé privée, pas à votre téléphone. Vous pouvez toujours accéder à votre portefeuille via MetaMask avec votre phrase de récupération. La récupération du portefeuille est indépendante de l\'inscription biométrique.',
  'path-title':'Choisissez Votre Voie','path-human-title':'Je suis un Humain','path-human-desc':'Je veux m\'inscrire, recevoir 1 000 AEQ et rejoindre le réseau de revenu de base.','path-human-steps':'1. Télécharger l\'app Android Aequitas<br>2. Déverrouiller avec le verrouillage d\'écran de votre appareil (empreinte/visage/code)<br>3. Connecter MetaMask<br>4. Recevoir 1 000 AEQ instantanément',
  'path-node-title':'Je suis un Opérateur de Nœud','path-node-desc':'Je veux exécuter un nœud complet, participer à la production de blocs et gagner du pool de validateurs à 40%.','path-node-steps':'1. S\'inscrire en tant qu\'humain (obligatoire)<br>2. Définir PRIMARY_NODE_URL=https://aequitas.digital<br>3. Déployer sur Railway/Contabo/VPS<br>4. Gagner quotidiennement du pool de validateurs',
  'path-dev-title':'Je suis un Développeur','path-dev-desc':'Je veux construire sur Aequitas, intégrer l\'API ou contribuer au protocole.','path-dev-steps':'1. JSON-RPC compatible EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Métriques: /metrics (Prometheus)',
  'story-flow-title':'Diagramme de Flux du Token AEQ','story-topo-title':'Topologie Réseau — État Actuel',
  'swap-price-title':'AEQ / tUSD — Prix en Direct','swap-price-desc':'Prix en temps réel dérivé des réserves du pool (x·y=k). Mis à jour toutes les 8 secondes.','swap-price-empty':'Pas encore de données de pool — ajoutez de la liquidité pour voir le graphique de prix.',
  'node-guide-lang-note':'Ce guide en ligne est en anglais. Un PDF traduit dans votre langue est disponible via le bouton ci-dessus.',
  'k-zkp':'Système ZKP','k-hash':'Système de Hachage','k-sybil-prot':'Protection Sybil',
},
pt:{
  'logo-sub':'PROVA DE HUMANIDADE','live':'AO VIVO',
  'tab-register':'🔐 Registrar','tab-explorer':'🔍 Explorador','tab-humans':'👥 Humanos','tab-index':'📊 Índice','tab-network':'🌐 Rede','tab-protocol':'📜 Protocolo V7','tab-swap':'🔄 Trocar',
  'reg-title':'🔐 Registrar como Humano Verificado',
  'reg-sub':'Junte-se à rede Aequitas e receba 1.000 AEQ de Renda Básica Universal. Registro único, permanente e completamente sem taxas. Nenhum dado pessoal é armazenado.',
  'app-title':'REGISTRO VIA APLICATIVO ANDROID',
  'app-text':'O registo cria uma chave criptográfica dentro do hardware seguro do seu telemóvel (Secure Enclave / StrongBox), protegida pelo bloqueio de ecrã do próprio dispositivo — sem sensor separado, nenhum dado biométrico é alguma vez gerado, processado ou transmitido. Uma prova ZK Groth16 prova que possui essa chave sem a revelar. Os seus 1.000 AEQ são creditados automaticamente após a verificação. Nota: isto prova atualmente o controlo de um dispositivo, não a unicidade biológica entre dispositivos — ver FAQ.',
  's1t':'Chave do Dispositivo','s1d':'O hardware seguro do seu telemóvel gera uma chave privada atrás do bloqueio de ecrã que já utiliza (impressão digital/rosto/PIN). Sem kit de sensores separado, nenhum dado biométrico em bruto sai alguma vez do dispositivo.',
  's2t':'Geração de Prova ZK','s2d':'Uma prova ZK Groth16 vincula a chave do seu dispositivo a um único commitment e nullifier, sem revelar a chave em si. Isto prova criptograficamente que possui a chave deste dispositivo — ver FAQ para o que isto garante e o que não garante.',
  's3t':'Conectar Carteira','s3d':'O app abre MetaMask · conecte sua carteira Ethereum · prova ligada criptograficamente ao seu endereço',
  's4t':'1.000 AEQ Concedidos','s4d':'Registro confirmado no BlockDAG em 1 segundo · 1.000 AEQ creditados instantaneamente · identidade registrada permanentemente',
  'priv-bar':'🔒 Chave criptográfica vinculada ao dispositivo · Groth16 ZKP · Os dados nunca saem do dispositivo · Um registo por dispositivo',
  'conn-wallet':'CARTEIRA CONECTADA','proof-recv':'⚡ PROVA ZK RECEBIDA','proof-hint':'Conectar carteira para registrar',
  'btn-conn':'🦊 CONECTAR METAMASK','btn-reg':'🔐 REGISTRAR ON-CHAIN',
  'btn-wc':'🔗 CONECTAR WALLETCONNECT',
  'reg-log-hint':'// Abra o App Android Aequitas para gerar sua prova, depois retorne aqui...',
  'reg-details':'Detalhes do Registro','k-network':'Rede','k-chainid':'ID da Cadeia','k-grant':'Concessão UBI',
  'k-fee':'Taxa de Gás','free':'GRATUITO — completamente sem taxas','k-limit':'Registros','k-limit-v':'Uma vez por dispositivo · permanente · imutável',
  'k-bio':'Chave do Dispositivo','never-stored':'Nunca sai do seu dispositivo — nenhum dado biométrico é gerado ou armazenado',
  'k-proof':'Sistema de Prova','k-conf':'Confirmação','k-conf-v':'Em 1 segundo (1 bloco)',
  'k-sybil':'Proteção Sybil','k-sybil-v':'Uma identidade por dispositivo · bloqueio permanente (vinculado ao dispositivo, ainda não ao corpo)',
  'live-stats':'Estatísticas ao Vivo da Cadeia',
  's-height':'Altura do Bloco',
  's-humans':'Humanos Verificados','s-humans-sub':'Prova ZK vinculada ao dispositivo · um registo por dispositivo',
  's-supply':'Oferta Total','s-supply-sub':'Sempre = Humanos × 1.000 AEQ',
  's-index':'Índice Aequitas','s-index-sub':'0 = igualdade perfeita · 100 = desigualdade máxima',
  's-uptime':'Disponibilidade','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'Prova de Humanidade','ib-poh-t':'Cada detentor de AEQ prova criptograficamente o controlo de uma chave vinculada ao dispositivo através de uma prova ZK Groth16. Sem bots, corporações ou IA. Os dados biométricos nunca são transmitidos, apenas uma prova matemática. Isto vincula hoje um registo por dispositivo, ainda não por pessoa única — ver FAQ.',
  'ib-fair':'Distribuição Radicalmente Justa','ib-fair-t':'Cada humano verificado recebe exatamente 1.000 AEQ no registro. Sem pré-mineração. Oferta = Humanos × 1.000.',
  'ib-dag':'Arquitetura BlockDAG','ib-dag-t':'Vários blocos produzidos simultaneamente e mesclados. Maior throughput, menor latência.',
  'ib-gas':'Verdadeiramente Sem Taxas','ib-gas-t':'Registro e transferências AEQ custam absolutamente nada. Sem ETH, BNB ou MATIC. Sem conta bancária.',
  'recent-blocks':'Blocos Recentes','blocks-desc':'MERGE = vários pais mesclados (BlockDAG). TX = transação de registro. Tempo de bloco: __BT__.',
  'loading':'Carregando blocos...','net-info':'Informações de Rede','k-chain':'Nome da Cadeia','k-symbol':'Símbolo','k-btime':'Tempo de Bloco',
  'k-cons':'Consenso','k-nodes':'Nodes Ativos','k-storage':'Armazenamento','add-mm':'🦊 ADICIONAR AO METAMASK','k-dec':'Decimais',
  'btn-add-mm':'+ ADICIONAR REDE AEQUITAS',
  'phil':'"O dinheiro existe porque as pessoas existem.<br>Nada mais, nada menos."','phil-sub':'— O PRINCÍPIO AEQUITAS —',
  'humans-title':'Humanos Verificados na Aequitas Chain',
  'h-what':'O que é um Humano Verificado?','h-what-t':'Um Humano Verificado é um endereço de carteira que provou criptograficamente o controlo da chave segura de um dispositivo específico até à data. A chave é gerada no hardware seguro do telemóvel, protegida pelo bloqueio de ecrã do próprio dispositivo — sem kit de sensores separado. Apenas uma prova ZK Groth16 é transmitida, nenhum dado biométrico sai do dispositivo. Isto verifica hoje um dispositivo, ainda não necessariamente uma pessoa única — ver FAQ.',
  'h-zkp':'Sistema de Prova ZK','h-zkp-t':'Aequitas usa Groth16 sobre BN128 — a mesma curva do Ethereum e Zcash. ~200 bytes, ~10ms. commitment = keccak256(deviceKey‖wallet). O nullifier está vinculado a este dispositivo: perder o telemóvel não cria uma segunda identidade neste dispositivo, mas outro dispositivo ainda pode registar-se separadamente. O material da chave nunca é revelado ou armazenado no servidor.',
  'h-sybil':'Resistência Sybil — Estado Atual','h-sybil-t':'O nullifier de hoje deriva de uma chave de hardware vinculada ao dispositivo — isto impede de forma fiável que o mesmo dispositivo (ou carteira) se registe duas vezes, mas ainda não consegue detetar se a mesma pessoa se regista a partir de um segundo dispositivo físico. Fechar esta lacuna requer uma verificação real de unicidade biológica entre dispositivos, planeada mas ainda não implementada.',
  'h-global':'Inclusão Financeira Global','h-global-t':'Sem conta bancária, cartão ou criptomoeda prévia. Apenas um smartphone Android com o bloqueio de ecrã que já utiliza.',
  'h-bio-hw':'Roteiro de Verificação de Identidade','h-bio-hw-t':'Hoje (beta): uma chave de hardware por dispositivo, honestamente rotulada como vinculada ao dispositivo, não ao corpo. Planeado (pós-beta): uma verificação real de unicidade biológica entre dispositivos, a ser definida, construída e auditada de forma independente antes de reivindicar uma resistência Sybil mais forte.',
  'reg-humans':'Humanos Registrados','h-desc':'Cada endereço abaixo provou o controlo de uma chave criptográfica vinculada ao dispositivo através de uma prova ZK e recebeu exatamente 1.000 AEQ. Permanente, imutável, on-chain. Ver FAQ para o que "vinculado ao dispositivo" significa para a resistência Sybil hoje.',
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
  'curr-idx':'Índice Atual','bar-0':'0 — Igualdade Perfeita','bar-100':'100 — Desigualdade Máx.','wcap-lbl':'Teto de Riqueza Atual:','wcap-mult':'Multiplicador:','wcap-avg':'Saldo médio:',
  'gini':'Coeficiente de Gini','gini-desc':'0 = igual · 1 = desigual',
  'supply-desc':'Sempre = Humanos × 1.000 AEQ',
  'phase':'Fase do Protocolo','phase-desc':'Avança automaticamente pelo número de humanos',
  'humans-desc':'Registos verificados por ZK vinculados ao dispositivo',
  'pools-title':'Pools de Redistribuição',
  'pools-desc':'Cada taxa de swap, demurrage e excesso do teto é dividido entre quatro pools. Todos pagam diariamente.',
  'vel-pool':'Pool de Validadores','vel-pool-desc':'40% de todas as taxas → operadores de nodes que protegem a rede',
  'liq-pool':'Pool de Liquidez','liq-pool-desc':'30% de todas as taxas → provedores de liquidez, proporcional às cotas LP',
  'ubi-pool':'Pool UBI','ubi-pool-desc':'20% de todas as taxas → todos os humanos verificados igualmente, a cada 24 horas',
  'treasury':'Tesouro','treasury-desc':'10% de todas as taxas → desenvolvimento e manutenção do protocolo',
  'phases-title':'Fases do Protocolo',
  'phases-desc':'Teto bootstrap Fase 0: max(5, min(N, 25))× saldo médio. 1–4 humanos: 5×. Cada humano +1×. 25+ humanos: travado em 25×. Transições automáticas.',
  'p0':'Bootstrap · &lt;100 humanos · Teto: max(5,min(N,25))× médio · 5×→25× · Ativo agora',
  'p1':'Crescimento · 100–10.000 humanos · Teto: 25× saldo médio',
  'p2':'Estabilidade · 10.000–1M humanos · Teto: 25× saldo médio',
  'p3':'Maturidade · 1M+ humanos · Teto: 25× saldo médio',
  'wealth-cap-explain':'Teto Fase 0: max(5, min(N, 25))× saldo médio. 1–4 humanos: 5×. Cada humano +1×. 25+: travado em 25×.',
  'demurrage-title':'Demurrage — Incentivo para Circular',
  'demurrage-desc':'Saldos AEQ inativos perdem lentamente valor para desencorajar acumulação.',
  'dem-rate-k':'Taxa de Decaimento','dem-rate-v':'0,5% por mês (contínuo)',
  'dem-grace-k':'Período de Graça','dem-grace-v':'3 meses de inatividade antes do decaimento começar',
  'dem-reset-k':'Reinicialização','dem-reset-v':'Qualquer transferência, swap ou liquidez reinicia o contador',
  'dem-dest-k':'AEQ decaído vai para','dem-dest-v':'Pools de redistribuição (40/30/20/10)',
  'dem-warn-k':'Sistema de Aviso','dem-warn-v':'Aviso 14 dias (uma vez) + lembrete 7 dias repetido em cada login',
  'story-title':'A História da Aequitas',
  'story-text':'<p>Em 2009, Satoshi Nakamoto lança o Bitcoin. Revolução genuína — mas os primeiros mineradores acumulam milhões a custo quase zero. Em 2021, top 1% controla mais de 90% do Bitcoin. Gini Bitcoin &gt; 0,85.</p><p><span style="color:var(--gold)">Aequitas</span> — latim para "equidade" — responde: <em style="color:var(--gold)">"Como seria uma criptomoeda projetada para ser justa com cada ser humano?"</em></p><p><strong style="color:var(--text)">O dinheiro existe porque as pessoas existem. Portanto, cada pessoa deveria ter uma parte igual.</strong></p><p><em style="color:var(--gold)">"O dinheiro existe porque as pessoas existem. Nada mais, nada menos."</em></p>',
  'nodes-title':'Nodes Ativos — Topologia de Rede Atual',
  'nodes-desc':'A rede Aequitas opera atualmente em múltiplos nodes distribuídos geograficamente (número atual acima), participando da produção de blocos, sincronização e API. Nodes adicionais são bem-vindos.',
  'run-node-title':'Execute seu Próprio Node','run-node-desc':'Qualquer um pode executar um node Aequitas — sem permissão, sem stake. Operadores ganham 40% das taxas de swap distribuídas diariamente.',
  'bootstrap-title':'Conectar um Novo Node','bootstrap-desc':'Defina PRIMARY_NODE_URL=https://aequitas.digital no seu ambiente. Seu node sincroniza automaticamente o estado completo da cadeia.',
  'tech-title':'Especificações Técnicas','mm-config':'Configuração MetaMask',
  'k-lang':'Idioma','k-src':'Fonte','evm-yes':'Sim — JSON-RPC /rpc · Compatível MetaMask',
  'proto-label':'Protocolo Aequitas V7 — Documentação Técnica',
  'ca-title':'Endereços dos Contratos',
  'ca-text':'Cadeia: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 é a única fonte de verdade para toda a economia Aequitas. Nenhuma chave de administrador nem voto de governança pode alterar sua lógica. O código atual rodará em dez anos.',
  'poa-title':'1. PROVA DE VIDA','poa-text':'<p>AEQ de pessoas falecidas retorna gradualmente à comunidade via pool UBI, em vez de ser perdido para sempre como no Bitcoin.</p>',
  'poa-box':'Anos 0–2: Uso normal<br>Ano 2: Aviso 1 — Guardião pode responder<br>Ano 2+60d: Aviso 2<br>Ano 2+120d: Aviso 3<br>Ano 2+180d: AEQ em custódia pessoal<br>Ano 4: Se inativo — retorna ao Pool UBI',
  'guard-title':'2. SISTEMA DE GUARDIÃO','guard-text':'<p>Um Guardião de confiança (outro humano verificado) pode confirmar que alguém está vivo, sem nenhum direito de transação.</p>',
  'guard-box':'1 Guardião por humano · deve ser humano verificado Aequitas<br>Guardião pode APENAS chamar confirmAlive() · zero direitos financeiros<br>Guardião NÃO PODE mover fundos · Máx. 3 protegidos · Timelock 7d',
  'dem-title':'3. DEMURRAGE — Anti-Acumulação',
  'dem-box':'Taxa: 0,5%/mês após 3 meses de graça<br>Reinicialização a cada transferência, swap ou liquidez<br>AEQ decaído redistribuído nos pools (não queimado)',
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
  'swap-btn-conn':'🦊 CONECTAR METAMASK','swap-btn-go':'🔄 TROCAR',
  'swap-log-hint':'// Conectar carteira para trocar...',
  'swap-no-liquidity':'Ainda sem tUSD?','swap-faucet-desc':'Humanos registrados podem reivindicar tUSD de teste uma vez','swap-btn-faucet':'💧 REIVINDICAR tUSD TESTE',
  'swap-addliq-title':'Fornecer Liquidez','swap-addliq-desc':'Seja o primeiro a depositar — sua proporção define o preço inicial.','swap-btn-addliq':'💧 ADICIONAR LIQUIDEZ',
  'swap-lp-title':'Sua Posição LP','swap-lp-share':'Cota do Pool','swap-lp-withdrawable':'Retirável',
  'swap-lp-pct-label':'% da sua posição','swap-lp-youget':'Você receberá','swap-btn-removeliq':'🔥 REMOVER LIQUIDEZ',
  'swap-pool-title':'AEQ / tUSD — Status do Pool',
  'swap-pool-aeq':'Reserva AEQ','swap-pool-tusd':'Reserva tUSD','swap-pool-price':'Preço Spot',
  'swap-fee-bps':'Taxa de Swap',
  'swap-pools-addr-title':'Endereços dos Pools Tokenômicos','pools-addr-title':'Endereços dos Contratos de Pools',
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
  'usp-c3-title':'Acessível a todos','usp-c3-desc':'Sem conta bancária, cartão de crédito ou documento de identidade, sem hardware adicional para comprar — apenas o bloqueio de ecrã que já tem no seu telemóvel Android.',
  'usp-c4-title':'UBI diário para sempre','usp-c4-desc':'Após registrado, sua parte do UBI chega automaticamente todos os dias — sem nenhuma ação.',
  'v7-intro-title':'O que é AequitasV7?',
  'v7-intro-text':'AequitasV7 é o contrato inteligente central do protocolo Aequitas. Implantado de forma imutável na Aequitas Chain (ID 1926). Gerencia tudo: registro humano, verificação ZK, saldos, teto de riqueza, UBI, taxas de swap. Nenhum administrador pode modificá-lo.',
  'explore-title':'Explorar Aequitas',
  'expl-score':'Pontuação de Igualdade','expl-score-d':'Coeficiente de Gini ao vivo · Índice Aequitas · distribuição de riqueza em tempo real',
  'expl-economy':'UBI &amp; Redistribuição','expl-economy-d':'Contagem regressiva UBI · 4 pools on-chain · demurrage · Fases do Protocolo',
  'expl-charts':'Gráficos &amp; Histórico','expl-charts-d':'Histórico Gini · curva de Lorenz · controle do teto · A história da Aequitas',
  'expl-v7':'Docs Protocolo V7','expl-v7-d':'Contrato AequitasV7 · 6 mecanismos · prova ZK · teto · demurrage · código imutável',
  'expl-explorer':'Explorador de Blocos','expl-explorer-d':'BlockDAG ao vivo · clique em qualquer bloco para ver validador, hash, transações',
  'swap-sell-label':'Vender','swap-receive-label':'Receber',
  'expl-network':'Rede &amp; Nodes','expl-network-d':'Topologia de nodes · executar seu próprio node · especificações · Chain ID 1926',
  'guard-title':'🛡 Sistema Guardian','guard-my-lbl':'Meu Guardian','guard-none':'Nenhum',
  'guard-set-lbl':'Definir / Alterar Guardian','guard-set-hint':'Deve ser um humano registado na Aequitas · Bloqueio temporal de 7 dias · O Guardian só pode confirmar a sua vitalidade, não aceder a fundos · Máx. 3 protegidos por Guardian',
  'guard-confirm-lbl':'Confirmar Vivo (Como Guardian)','guard-confirm-hint':'Se o seu protegido não conseguir aceder à sua carteira, confirme a sua vitalidade para evitar que os fundos sejam transferidos para custódia após 910 dias de inatividade.','guard-recover-btn':'🔓 RECUPERAR DA CUSTÓDIA',
  'faq-title':'❓ Perguntas Frequentes','faq-q1':'Os meus dados biométricos estão seguros?','faq-a1':'Sim. Nenhum dado biométrico é capturado, processado ou transmitido pela Aequitas. O bloqueio de ecrã do seu telemóvel apenas dá acesso a uma chave aleatória gerada e armazenada no seu hardware seguro. Só é enviada uma prova matemática derivada dessa chave — nunca a chave em si, nunca dados biométricos.',
  'faq-q1b':'O registo prova que sou uma pessoa real e única?','faq-a1b':'Ainda não totalmente. A prova de hoje demonstra criptograficamente que você controla a chave segura de um dispositivo específico — isso impede de forma confiável que o mesmo dispositivo (ou carteira) se registe duas vezes, mas ainda não consegue distinguir dois dispositivos físicos diferentes pertencentes à mesma pessoa. Uma verificação real de unicidade biológica entre dispositivos está planeada, mas ainda não foi implementada — preferimos dizer isso claramente a exagerar o que a prova vinculada ao dispositivo garante hoje.',
  'faq-q2':'Posso registar-me com uma carteira diferente mais tarde?','faq-a2':'Não. O registo está permanentemente vinculado a um endereço de carteira por chave de dispositivo. É por design — impede que o mesmo dispositivo se registe novamente e garante uma carteira por identidade de dispositivo.',
  'faq-q3':'O que acontece se perder o telemóvel?','faq-a3':'Os seus AEQ permanecem na carteira — estão vinculados à sua chave privada, não ao telemóvel. Ainda pode aceder à carteira via MetaMask com a frase de recuperação. A recuperação da carteira é independente do registo biométrico.',
  'path-title':'Escolha o Seu Caminho','path-human-title':'Sou um Humano','path-human-desc':'Quero registar-me, receber 1.000 AEQ e juntar-me à rede de rendimento básico.','path-human-steps':'1. Descarregar a app Android Aequitas<br>2. Desbloquear com o bloqueio de ecrã do seu dispositivo (impressão digital/rosto/PIN)<br>3. Conectar MetaMask<br>4. Receber 1.000 AEQ instantaneamente',
  'path-node-title':'Sou um Operador de Node','path-node-desc':'Quero executar um node completo, participar na produção de blocos e ganhar do pool de validadores de 40%.','path-node-steps':'1. Registar como humano (obrigatório)<br>2. Definir PRIMARY_NODE_URL=https://aequitas.digital<br>3. Implementar em Railway/Contabo/VPS<br>4. Ganhar diariamente do pool de validadores',
  'path-dev-title':'Sou um Desenvolvedor','path-dev-desc':'Quero construir no Aequitas, integrar a API ou contribuir para o protocolo.','path-dev-steps':'1. JSON-RPC compatível com EVM<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* endpoints<br>4. Métricas: /metrics (Prometheus)',
  'story-flow-title':'Diagrama de Fluxo do Token AEQ','story-topo-title':'Topologia de Rede — Estado Atual',
  'swap-price-title':'AEQ / tUSD — Preço ao Vivo','swap-price-desc':'Preço em tempo real derivado das reservas do pool (x·y=k). Atualizado a cada 8 segundos.','swap-price-empty':'Sem dados do pool ainda — adicione liquidez para ver o gráfico de preços.',
  'node-guide-lang-note':'Este guia inline está em inglês. Um PDF traduzido na sua língua está disponível através do botão acima.',
  'k-zkp':'Sistema ZKP','k-hash':'Sistema Hash','k-sybil-prot':'Proteção Sybil',
},
ar:{
  'logo-sub':'إثبات الإنسانية','live':'مباشر',
  'tab-register':'🔐 تسجيل','tab-explorer':'🔍 المستكشف','tab-humans':'👥 البشر','tab-index':'📊 المؤشر','tab-network':'🌐 الشبكة','tab-protocol':'📜 البروتوكول V7','tab-swap':'🔄 تبادل',
  'reg-title':'🔐 التسجيل كإنسان موثق',
  'reg-sub':'انضم إلى شبكة Aequitas واحصل على منحة دخل أساسي شامل تبلغ 1,000 AEQ. التسجيل لمرة واحدة، دائم، ومجاني تماماً. لا يتم تخزين أي بيانات شخصية.',
  'app-title':'التسجيل عبر تطبيق أندرويد',
  'app-text':'يُنشئ التسجيل مفتاحاً تشفيرياً داخل العتاد الآمن لهاتفك (Secure Enclave / StrongBox)، محمياً بقفل شاشة الجهاز نفسه — لا يوجد مستشعر منفصل، ولا تُنشأ أو تُعالَج أو تُنقَل أي بيانات بيومترية على الإطلاق. يثبت دليل ZK من نوع Groth16 أنك تملك هذا المفتاح دون الكشف عنه. يُضاف 1,000 AEQ تلقائياً بعد التحقق. ملاحظة: هذا يثبت حالياً التحكم بجهاز واحد، وليس التفرد البيولوجي عبر الأجهزة — انظر الأسئلة الشائعة.',
  's1t':'مفتاح الجهاز','s1d':'يُنشئ العتاد الآمن لهاتفك مفتاحاً خاصاً خلف قفل الشاشة الذي تستخدمه بالفعل (بصمة/وجه/رمز PIN). لا توجد مجموعة مستشعرات منفصلة، ولا تغادر أي بيانات بيومترية خام الجهاز أبداً.',
  's2t':'توليد دليل ZK','s2d':'يربط دليل ZK من نوع Groth16 مفتاح جهازك بالتزام (commitment) و nullifier فريدين، دون الكشف عن المفتاح نفسه. يثبت هذا تشفيرياً أنك تملك مفتاح هذا الجهاز — انظر الأسئلة الشائعة لمعرفة ما يضمنه هذا وما لا يضمنه.',
  's3t':'ربط المحفظة','s3d':'يفتح التطبيق MetaMask · ارتبط بمحفظة Ethereum · الدليل مرتبط تشفيرياً بعنوان محفظتك',
  's4t':'تم منح 1,000 AEQ','s4d':'تم تأكيد التسجيل على BlockDAG خلال 6 ثوانٍ · اعتماد 1,000 AEQ فوراً · هويتك مسجلة بشكل دائم',
  'priv-bar':'🔒 مفتاح تشفيري مرتبط بالجهاز · Groth16 ZKP · البيانات لا تغادر الجهاز أبداً · تسجيل واحد لكل جهاز',
  'conn-wallet':'المحفظة المتصلة','proof-recv':'⚡ تم استلام دليل ZK','proof-hint':'ربط محفظة للتسجيل',
  'btn-conn':'🦊 ربط METAMASK','btn-reg':'🔐 التسجيل ON-CHAIN',
  'btn-wc':'🔗 ربط WALLETCONNECT',
  'reg-log-hint':'// افتح تطبيق Aequitas Android لتوليد دليلك، ثم عد هنا...',
  'reg-details':'تفاصيل التسجيل','k-network':'الشبكة','k-chainid':'معرّف السلسلة','k-grant':'منحة UBI',
  'k-fee':'رسوم الغاز','free':'مجاني — بدون رسوم تماماً','k-limit':'التسجيلات','k-limit-v':'مرة واحدة لكل جهاز · دائم · غير قابل للتغيير',
  'k-bio':'مفتاح الجهاز','never-stored':'لا يغادر جهازك أبداً — لا تُنشأ أو تُخزَّن أي بيانات بيومترية',
  'k-proof':'نظام الأدلة','k-conf':'التأكيد','k-conf-v':'خلال ثانية واحدة (كتلة واحدة)',
  'k-sybil':'حماية Sybil','k-sybil-v':'هوية واحدة لكل جهاز · قفل دائم (مرتبط بالجهاز، وليس بالجسم بعد)',
  'live-stats':'إحصائيات السلسلة المباشرة',
  's-height':'ارتفاع الكتلة',
  's-humans':'البشر الموثقون','s-humans-sub':'دليل ZK مرتبط بالجهاز · تسجيل واحد لكل جهاز',
  's-supply':'إجمالي العرض','s-supply-sub':'دائماً = البشر × 1,000 AEQ',
  's-index':'مؤشر Aequitas','s-index-sub':'0 = مساواة مثالية · 100 = أقصى عدم مساواة',
  's-uptime':'وقت التشغيل','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'إثبات الإنسانية','ib-poh-t':'يثبت كل حامل AEQ تشفيرياً تحكمه بمفتاح مرتبط بالجهاز عبر دليل ZK من نوع Groth16. لا بوتات ولا شركات ولا ذكاء اصطناعي. لا تُنقَل البيانات البيومترية أبداً، بل دليل رياضي فقط. هذا يربط اليوم تسجيلاً واحداً لكل جهاز، وليس بعد لكل شخص فريد — انظر الأسئلة الشائعة.',
  'ib-fair':'توزيع عادل جذرياً','ib-fair-t':'كل إنسان موثق يحصل على 1,000 AEQ بالضبط عند التسجيل. لا تعدين مسبق. الإجمالي = البشر × 1,000.',
  'ib-dag':'بنية BlockDAG','ib-dag-t':'يمكن إنتاج كتل متعددة في وقت واحد ودمجها. إنتاجية أعلى وزمن استجابة أقل.',
  'ib-gas':'مجاني حقاً','ib-gas-t':'التسجيل وتحويلات AEQ لا تكلف شيئاً. لا حاجة لـ ETH أو BNB أو MATIC أو حساب بنكي.',
  'recent-blocks':'الكتل الأخيرة','blocks-desc':'MERGE = دمج عدة والدين (BlockDAG). TX = معاملة تسجيل. وقت الكتلة: __BT__.',
  'loading':'جارٍ تحميل الكتل...','net-info':'معلومات الشبكة','k-chain':'اسم السلسلة','k-symbol':'الرمز','k-btime':'وقت الكتلة',
  'k-cons':'التوافق','k-nodes':'العقد النشطة','k-storage':'التخزين','add-mm':'🦊 إضافة إلى METAMASK','k-dec':'الأرقام العشرية',
  'btn-add-mm':'+ إضافة شبكة AEQUITAS',
  'phil':'"المال موجود لأن البشر موجودون.<br>لا أكثر، ولا أقل."','phil-sub':'— مبدأ AEQUITAS —',
  'humans-title':'البشر الموثقون على Aequitas Chain',
  'h-what':'ما هو الإنسان الموثق؟','h-what-t':'الإنسان الموثق هو عنوان محفظة أثبت تشفيرياً تحكمه بالمفتاح الآمن لجهاز معين حتى الآن. يُنشأ المفتاح في العتاد الآمن للهاتف، محمياً بقفل شاشة الجهاز نفسه — لا توجد مجموعة مستشعرات منفصلة. يُرسَل دليل Groth16 ZK فقط، ولا تغادر أي بيانات بيومترية الجهاز. هذا يوثّق اليوم جهازاً، وليس بالضرورة بعد شخصاً فريداً — انظر الأسئلة الشائعة.',
  'h-zkp':'نظام أدلة ZK','h-zkp-t':'تستخدم Aequitas بروتوكول Groth16 على المنحنى BN128 — نفس المنحنى المستخدم في Ethereum وZcash. ~200 بايت، ~10ms. commitment = keccak256(deviceKey‖wallet). يرتبط الـ nullifier بهذا الجهاز: فقدان الهاتف لا يُنشئ هوية ثانية على هذا الجهاز، لكن جهازاً آخر لا يزال بإمكانه التسجيل بشكل منفصل. لا يُكشَف عن مادة المفتاح أو تُخزَّن على الخادم أبداً.',
  'h-sybil':'مقاومة Sybil — الوضع الحالي','h-sybil-t':'يُشتق الـ nullifier اليوم من مفتاح عتاد مرتبط بالجهاز — وهذا يمنع بشكل موثوق تسجيل نفس الجهاز (أو المحفظة) مرتين، لكنه لا يستطيع بعد اكتشاف ما إذا كان نفس الشخص يسجل من جهاز مادي ثانٍ. يتطلب سد هذه الثغرة تحققاً حقيقياً من التفرد البيولوجي عبر الأجهزة، وهو مخطط له لكن لم يُطرح بعد.',
  'h-global':'الشمول المالي العالمي','h-global-t':'لا حاجة لحساب بنكي أو بطاقة ائتمان أو عملة مشفرة مسبقة. هاتف أندرويد بقفل الشاشة الذي تستخدمه بالفعل يكفي.',
  'h-bio-hw':'خارطة طريق التحقق من الهوية','h-bio-hw-t':'اليوم (النسخة التجريبية): مفتاح عتاد واحد لكل جهاز، موسوم بصدق كمرتبط بالجهاز، وليس بالجسم. المخطط (بعد النسخة التجريبية): تحقق حقيقي من التفرد البيولوجي عبر الأجهزة، يُحدَّد ويُبنى ويُدقَّق بشكل مستقل قبل ادعاء مقاومة أقوى لهجمات Sybil.',
  'reg-humans':'البشر المسجلون','h-desc':'كل عنوان أدناه أثبت تحكمه بمفتاح تشفيري مرتبط بالجهاز عبر دليل ZK وحصل على 1,000 AEQ بالضبط. دائم وغير قابل للتغيير، على السلسلة. انظر الأسئلة الشائعة لمعرفة ما يعنيه "مرتبط بالجهاز" لمقاومة Sybil اليوم.',
  'no-humans':'لا يوجد بشر مسجلون بعد.\n\nحمّل تطبيق Aequitas Android وكن أول إنسان على السلسلة!',
  'reg-stats':'إحصائيات السجل','total-humans':'إجمالي البشر',
  'idx-title':'مؤشر Aequitas — درجة المساواة الاقتصادية في الوقت الفعلي',
  'idx-desc':'مؤشر Aequitas مشتق من <strong style="color:var(--teal)">معامل جيني</strong> — المعيار الدولي لقياس عدم المساواة (البنك الدولي، OECD، الأمم المتحدة). <strong style="color:var(--neon)">0 = مساواة مثالية</strong>. <strong style="color:var(--red)">100 = تركيز كامل</strong>. الهدف: جيني أقل من 0.30.',
  'gini-what-title':'ما هو معامل جيني؟',
  'gini-what-text':'طوّره كورادو جيني (1912). يقيس توزيع الثروة. المقياس: 0 (الجميع متساوون) إلى 1 (شخص واحد يملك كل شيء). يُستخدم من قِبل البنك الدولي وOECD والأمم المتحدة.',
  'gini-calc-title':'كيف يتم حساب مؤشر Aequitas؟','gini-calc-text':'يتم جمع جميع أرصدة AEQ للبشر المعتمدين. تحسب الصيغة الفرق المطلق المتوسط بين كل زوج من الأرصدة، مقسومًا على مربع عدد السكان (n²) والرصيد المتوسط. النتيجة 0-1 مضروبة في 100 = مؤشر Aequitas.',
  'gini-why-title':'لماذا جيني — ولا مقياس أبسط؟','gini-why-text':'نسبة الأغنى-الأفقر بسيطة وسهلة التحايل عليها — معامل جيني يكتشف ذلك. يلتقط المعامل التوزيع الكامل بين جميع البشر المعتمدين في رقم واحد قابل للتدقيق. تنشر Aequitas هذا على السلسلة — شفاف وقابل للتحقق عالميًا.',
  'curr-idx':'المؤشر الحالي','bar-0':'0 — مساواة مثالية','bar-100':'100 — أقصى عدم مساواة','wcap-lbl':'سقف الثروة الحالي:','wcap-mult':'المضاعف:','wcap-avg':'متوسط الرصيد:',
  'phases-desc':'في المرحلة 0 (التأسيس)، يستخدم سقف الثروة مضاعفًا متحركًا: max(5, min(N, 25)) × متوسط الرصيد. مع 1-4 بشر: 5× المتوسط. كل إنسان جديد يضيف 1×. عند 25+ إنسانًا: يُثبَّت بشكل دائم عند 25×. تحدث جميع الانتقالات تلقائيًا بحسب عدد البشر — بدون تصويت إداري، بدون مفتاح إداري.',
  'wealth-cap-explain':'سقف الثروة في المرحلة 0 (التأسيس) يستخدم max(5, min(N, 25)) × متوسط رصيد AEQ، حيث N = عدد البشر المسجلين. 1-4 بشر: السقف = 5× المتوسط. كل إنسان جديد يضيف 1×. عند 25+: يُثبَّت دائمًا عند 25×. يتكيف السقف دائمًا مع متوسط الرصيد الحالي.',
  'p0':'التأسيس · أقل من 100 إنسان · سقف الثروة: max(5,min(N,25))× المتوسط · ينزلق من 5× إلى 25× حتى الإنسان الخامس والعشرين · نشط حاليًا',
  'p1':'النمو · 100–10,000 إنسان · سقف الثروة: 25× متوسط الرصيد',
  'p2':'الاستقرار · 10,000–1M إنسان · سقف الثروة: 25× متوسط الرصيد',
  'p3':'النضج · 1M+ إنسان · سقف الثروة: 25× متوسط الرصيد',
  'gini':'معامل جيني','gini-desc':'0 = متساوٍ · 1 = غير متساوٍ',
  'supply-desc':'دائماً = البشر × 1,000 AEQ',
  'phase':'مرحلة البروتوكول','phase-desc':'يتقدم تلقائياً بعدد البشر',
  'humans-desc':'تسجيلات موثقة بـ ZK مرتبطة بالجهاز',
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
  'story-text':'<p>عام 2009، أصدر ساتوشي ناكاموتو Bitcoin. ثورة حقيقية — لكن المنقبين الأوائل جمعوا الملايين بتكلفة شبه معدومة. في 2021، يتحكم أعلى 1% في أكثر من 90% من Bitcoin. جيني Bitcoin &gt; 0.85.</p><p><span style="color:var(--gold)">Aequitas</span> — لاتينية لـ "العدالة" — أُنشئ للإجابة على: <em style="color:var(--gold)">"كيف ستبدو عملة مشفرة صُمِّمت لتكون عادلة لكل إنسان؟"</em></p><p><strong style="color:var(--text)">المال موجود لأن البشر موجودون. لذا يجب أن يحصل كل شخص على حصة متساوية.</strong></p><p><em style="color:var(--gold)">"المال موجود لأن البشر موجودون. لا أكثر، ولا أقل."</em></p>',
  'nodes-title':'العقد النشطة — طوبولوجيا الشبكة الحالية',
  'nodes-desc':'تعمل شبكة Aequitas على عدة عقد موزعة جغرافياً (العدد الحالي أعلاه)، تشارك في إنتاج الكتل والمزامنة وخدمة API.',
  'run-node-title':'قم بتشغيل عقدتك الخاصة','run-node-desc':'يمكن لأي شخص تشغيل عقدة Aequitas — بدون إذن أو حصة. المشغّلون يكسبون 40% من رسوم المبادلة يومياً.',
  'bootstrap-title':'ربط عقدة جديدة','bootstrap-desc':'اضبط PRIMARY_NODE_URL=https://aequitas.digital في بيئتك. عقدتك ستزامن حالة السلسلة الكاملة تلقائياً.',
  'tech-title':'المواصفات التقنية','mm-config':'إعداد MetaMask',
  'k-lang':'اللغة','k-src':'المصدر','evm-yes':'نعم — JSON-RPC /rpc · متوافق مع MetaMask',
  'proto-label':'بروتوكول Aequitas V7 — وثائق تقنية',
  'ca-title':'عناوين العقود',
  'ca-text':'السلسلة: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 هو المصدر الوحيد للحقيقة لاقتصاد Aequitas بأكمله. لا مفتاح إدارة ولا تصويت حوكمة يمكنه تغيير منطقه.',
  'poa-title':'1. إثبات الحياة','poa-text':'<p>عند وفاة الأشخاص، تعود AEQ الخاصة بهم تدريجياً إلى المجتمع عبر مجمع UBI بدلاً من ضياعها للأبد.</p>',
  'poa-box':'السنوات 0–2: استخدام طبيعي<br>السنة 2: تحذير 1 — الحارس يمكنه الرد<br>السنة 2+60 يوم: تحذير 2<br>السنة 2+120 يوم: تحذير 3<br>السنة 2+180 يوم: AEQ في ضمان شخصي<br>السنة 4: إذا لا يزال خاملاً — يعود لمجمع UBI',
  'guard-title':'2. نظام الحارس','guard-text':'<p>حارس موثوق (إنسان موثق آخر) يمكنه تأكيد أن شخصاً ما لا يزال حياً، دون أي حقوق مالية.</p>',
  'guard-box':'حارس واحد لكل إنسان · يجب أن يكون إنساناً موثقاً<br>الحارس يمكنه فقط استدعاء confirmAlive() · صفر حقوق مالية<br>الحارس لا يمكنه تحريك الأموال · الحد الأقصى 3 · Timelock 7 أيام',
  'dem-title':'3. التلاشي — آلية مكافحة الاكتناز',
  'dem-box':'المعدل: 0.5%/شهر بعد 3 أشهر سماح<br>إعادة تعيين عند أي تحويل أو مبادلة أو سيولة<br>AEQ المتلاشي يُعاد توزيعه في المجمعات (لا يُحرق)',
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
  'swap-btn-conn':'🦊 ربط METAMASK','swap-btn-go':'🔄 تبادل',
  'swap-log-hint':'// ربط محفظة للتبادل...',
  'swap-no-liquidity':'لا يوجد tUSD بعد?','swap-faucet-desc':'البشر المسجلون يمكنهم المطالبة بـ tUSD اختبار مرة واحدة','swap-btn-faucet':'💧 المطالبة بـ tUSD الاختبار',
  'swap-addliq-title':'توفير السيولة','swap-addliq-desc':'كن أول من يودع — نسبتك تحدد السعر الأولي.','swap-btn-addliq':'💧 إضافة سيولة',
  'swap-lp-title':'مركز LP الخاص بك','swap-lp-share':'حصة المجمع','swap-lp-withdrawable':'قابل للسحب',
  'swap-lp-pct-label':'% من مركزك','swap-lp-youget':'ستحصل على','swap-btn-removeliq':'🔥 سحب السيولة',
  'swap-pool-title':'AEQ / tUSD — حالة المجمع',
  'swap-pool-aeq':'احتياطي AEQ','swap-pool-tusd':'احتياطي tUSD','swap-pool-price':'السعر الفوري',
  'swap-fee-bps':'رسوم المبادلة',
  'swap-pools-addr-title':'عناوين مجمعات التوكينوميكس','pools-addr-title':'عناوين عقود المجمعات',
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
  'usp-c3-title':'متاح للجميع','usp-c3-desc':'لا حاجة لحساب بنكي أو بطاقة ائتمان أو وثيقة هوية، ولا عتاد إضافي للشراء — فقط قفل الشاشة الموجود بالفعل على هاتفك الأندرويد.',
  'usp-c4-title':'UBI يومي إلى الأبد','usp-c4-desc':'بعد التسجيل، تصل حصتك من UBI تلقائياً كل يوم — دون أي إجراء.',
  'v7-intro-title':'ما هو AequitasV7؟',
  'v7-intro-text':'AequitasV7 هو العقد الذكي المركزي لبروتوكول Aequitas. مُنشر بشكل غير قابل للتغيير على Aequitas Chain (ID 1926). يدير كل شيء: التسجيل البشري، التحقق ZK، الأرصدة، سقف الثروة، UBI، رسوم المبادلة. لا يمكن لأي مدير تعديله.',
  'explore-title':'استكشف Aequitas',
  'expl-score':'درجة المساواة','expl-score-d':'معامل جيني مباشر · مؤشر Aequitas · توزيع الثروة في الوقت الفعلي',
  'expl-economy':'UBI وإعادة التوزيع','expl-economy-d':'عد UBI التنازلي اليومي · 4 مجمعات on-chain · تلاشي · مراحل البروتوكول',
  'expl-charts':'الرسوم البيانية والتاريخ','expl-charts-d':'تاريخ جيني · منحنى لورينز · شريط سقف الثروة · قصة Aequitas',
  'expl-v7':'وثائق البروتوكول V7','expl-v7-d':'عقد AequitasV7 · 6 آليات · دليل ZK · سقف الثروة · تلاشي · كود غير قابل للتغيير',
  'expl-explorer':'مستكشف الكتل','expl-explorer-d':'BlockDAG مباشر · انقر على أي كتلة لرؤية المدقق والهاش والمعاملات',
  'swap-sell-label':'بيع','swap-receive-label':'استلام',
  'expl-network':'الشبكة والعقد','expl-network-d':'طوبولوجيا العقد · تشغيل عقدتك الخاصة · المواصفات التقنية · Chain ID 1926',
  'guard-title':'🛡 نظام الوصي','guard-my-lbl':'وصيّي','guard-none':'لا يوجد',
  'guard-set-lbl':'تعيين / تغيير الوصي','guard-set-hint':'يجب أن يكون إنساناً مسجلاً في Aequitas · قفل زمني لمدة 7 أيام · الوصي يستطيع فقط تأكيد حياتك، لا الوصول إلى الأموال · الحد الأقصى 3 محميين لكل وصي',
  'guard-confirm-lbl':'تأكيد الحياة (بصفة وصي)','guard-confirm-hint':'إذا لم يستطع محميّك الوصول إلى محفظته، أكّد حياته لمنع نقل أمواله إلى الضمان بعد 910 يوماً من الخمول.','guard-recover-btn':'🔓 استرداد من الضمان',
  'faq-title':'❓ الأسئلة الشائعة','faq-q1':'هل بياناتي البيومترية آمنة؟','faq-a1':'نعم. لا تلتقط Aequitas أو تعالج أو تُرسِل أي بيانات بيومترية على الإطلاق. تمنح شاشة قفل هاتفك فقط إمكانية الوصول إلى مفتاح عشوائي يُنشأ ويُخزَّن في عتاده الآمن. يُرسَل فقط إثبات رياضي مشتق من ذلك المفتاح — أبداً المفتاح نفسه، وأبداً أي بيانات بيومترية.',
  'faq-q1b':'هل يثبت التسجيل أنني شخص حقيقي وفريد؟','faq-a1b':'ليس بشكل كامل بعد. يثبت الدليل الحالي بشكل تشفيري أنك تتحكم في المفتاح الآمن لجهاز معين — وهذا يمنع بشكل موثوق تسجيل نفس الجهاز (أو المحفظة) مرتين، لكنه لا يستطيع حاليًا التمييز بين جهازين ماديين مختلفين يملكهما نفس الشخص. من المخطط إجراء تحقق حقيقي من التفرد البيولوجي عبر الأجهزة، لكنه لم يُطرح بعد — نفضل قول ذلك بصراحة بدلاً من المبالغة فيما يضمنه الدليل المرتبط بالجهاز اليوم.',
  'faq-q2':'هل يمكنني التسجيل بمحفظة مختلفة لاحقاً؟','faq-a2':'لا. يرتبط التسجيل بشكل دائم بعنوان محفظة واحد لكل مفتاح جهاز. هذا قصد تصميمي — يمنع إعادة تسجيل نفس الجهاز ويضمن محفظة واحدة لكل هوية جهاز.',
  'faq-q3':'ماذا يحدث إذا فقدت هاتفي؟','faq-a3':'يبقى AEQ الخاص بك في محفظتك — مرتبط بمفتاحك الخاص، وليس بهاتفك. لا يزال بإمكانك الوصول إلى محفظتك عبر MetaMask باستخدام عبارة الاسترداد. استرداد المحفظة مستقل عن التسجيل البيومتري.',
  'path-title':'اختر مسارك','path-human-title':'أنا إنسان','path-human-desc':'أريد التسجيل وتلقي 1,000 AEQ والانضمام إلى شبكة الدخل الأساسي.','path-human-steps':'1. تحميل تطبيق Aequitas Android<br>2. فتح القفل بقفل شاشة جهازك (بصمة/وجه/رمز PIN)<br>3. ربط MetaMask<br>4. استلام 1,000 AEQ فوراً',
  'path-node-title':'أنا مشغّل عقدة','path-node-desc':'أريد تشغيل عقدة كاملة والمشاركة في إنتاج الكتل والكسب من مجموعة المتحققين 40%.','path-node-steps':'1. التسجيل كإنسان (مطلوب)<br>2. تعيين PRIMARY_NODE_URL=https://aequitas.digital<br>3. النشر على Railway/Contabo/VPS<br>4. الكسب اليومي من مجموعة المتحققين',
  'path-dev-title':'أنا مطوّر','path-dev-desc':'أريد البناء على Aequitas أو دمج API أو المساهمة في البروتوكول.','path-dev-steps':'1. JSON-RPC متوافق مع EVM<br>2. معرّف السلسلة: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* نقاط النهاية<br>4. المقاييس: /metrics (Prometheus)',
  'story-flow-title':'مخطط تدفق رمز AEQ','story-topo-title':'طوبولوجيا الشبكة — الحالة الراهنة',
  'swap-price-title':'AEQ / tUSD — السعر المباشر','swap-price-desc':'سعر فوري مشتق من احتياطيات المجموعة (x·y=k). يتحدث كل 8 ثوانٍ.','swap-price-empty':'لا توجد بيانات مجموعة بعد — أضف سيولة لرؤية مخطط السعر.',
  'node-guide-lang-note':'هذا الدليل المضمّن باللغة الإنجليزية. ملف PDF مترجم بلغتك متاح عبر الزر أعلاه.',
  'k-zkp':'نظام ZKP','k-hash':'نظام التجزئة','k-sybil-prot':'حماية سيبل',
},
hi:{
  'logo-sub':'मानवता का प्रमाण','live':'लाइव',
  'tab-register':'🔐 रजिस्टर','tab-explorer':'🔍 एक्सप्लोरर','tab-humans':'👥 मनुष्य','tab-index':'📊 इंडेक्स','tab-network':'🌐 नेटवर्क','tab-protocol':'📜 प्रोटोकॉल V7','tab-swap':'🔄 स्वैप',
  'reg-title':'🔐 सत्यापित मानव के रूप में रजिस्टर करें',
  'reg-sub':'Aequitas नेटवर्क से जुड़ें और 1,000 AEQ का यूनिवर्सल बेसिक इनकम अनुदान प्राप्त करें। रजिस्ट्रेशन एक बार, स्थायी और पूरी तरह निःशुल्क है। कोई व्यक्तिगत डेटा संग्रहीत नहीं किया जाता।',
  'app-title':'एंड्रॉयड ऐप के माध्यम से रजिस्ट्रेशन',
  'app-text':'पंजीकरण आपके फोन के सुरक्षित हार्डवेयर (Secure Enclave / StrongBox) के भीतर एक क्रिप्टोग्राफिक कुंजी बनाता है, जो डिवाइस के अपने स्क्रीन-लॉक द्वारा सुरक्षित है — कोई अलग सेंसर नहीं, कोई बायोमेट्रिक डेटा कभी उत्पन्न, प्रोसेस या ट्रांसमिट नहीं होता। एक Groth16 ZK प्रमाण बिना इसे प्रकट किए साबित करता है कि आपके पास यह कुंजी है। सत्यापन पर आपके 1,000 AEQ स्वचालित रूप से जमा हो जाते हैं। नोट: यह वर्तमान में एक डिवाइस के नियंत्रण को साबित करता है, क्रॉस-डिवाइस जैविक विशिष्टता को नहीं — FAQ देखें।',
  's1t':'डिवाइस कुंजी','s1d':'आपके फोन का सुरक्षित हार्डवेयर उस स्क्रीन-लॉक के पीछे एक निजी कुंजी बनाता है जिसे आप पहले से उपयोग करते हैं (फिंगरप्रिंट/फेस/PIN)। कोई अलग सेंसर किट नहीं, कोई कच्चा बायोमेट्रिक डेटा कभी डिवाइस नहीं छोड़ता।',
  's2t':'ZK प्रमाण जनरेशन','s2d':'एक Groth16 ZK प्रमाण आपकी डिवाइस कुंजी को बिना कुंजी को ही प्रकट किए एक अद्वितीय commitment और nullifier से बांधता है। यह क्रिप्टोग्राफिक रूप से साबित करता है कि आपके पास इस डिवाइस की कुंजी है — यह क्या गारंटी देता है और क्या नहीं, इसके लिए FAQ देखें।',
  's3t':'वॉलेट कनेक्ट करें','s3d':'ऐप इस पेज पर MetaMask खोलती है · अपना Ethereum वॉलेट कनेक्ट करें · प्रमाण आपके वॉलेट पते से क्रिप्टोग्राफिक रूप से जुड़ा है',
  's4t':'1,000 AEQ प्रदान','s4d':'Aequitas BlockDAG पर 6 सेकंड में रजिस्ट्रेशन की पुष्टि · 1,000 AEQ तुरंत जमा · आपकी पहचान स्थायी रूप से दर्ज',
  'priv-bar':'🔒 डिवाइस से बंधी क्रिप्टोग्राफिक कुंजी · Groth16 ZKP · डेटा कभी डिवाइस नहीं छोड़ता · प्रति डिवाइस एक पंजीकरण',
  'conn-wallet':'कनेक्टेड वॉलेट','proof-recv':'⚡ ZK प्रमाण प्राप्त','proof-hint':'रजिस्टर करने के लिए वॉलेट कनेक्ट करें',
  'btn-conn':'🦊 METAMASK कनेक्ट करें','btn-reg':'🔐 ON-CHAIN रजिस्टर करें',
  'btn-wc':'🔗 WALLETCONNECT कनेक्ट करें',
  'reg-log-hint':'// अपना प्रमाण उत्पन्न करने के लिए Aequitas Android App खोलें, फिर यहाँ वापस आएं...',
  'reg-details':'रजिस्ट्रेशन विवरण','k-network':'नेटवर्क','k-chainid':'चेन ID','k-grant':'UBI अनुदान',
  'k-fee':'गैस शुल्क','free':'निःशुल्क — पूरी तरह गैसलेस','k-limit':'रजिस्ट्रेशन','k-limit-v':'प्रति डिवाइस एक बार · स्थायी · अपरिवर्तनीय',
  'k-bio':'डिवाइस कुंजी','never-stored':'आपके डिवाइस से कभी बाहर नहीं जाता — कोई बायोमेट्रिक डेटा उत्पन्न या संग्रहीत नहीं होता',
  'k-proof':'प्रमाण प्रणाली','k-conf':'पुष्टि','k-conf-v':'1 सेकंड के भीतर (1 ब्लॉक)',
  'k-sybil':'Sybil सुरक्षा','k-sybil-v':'प्रति डिवाइस एक पहचान · स्थायी लॉक (डिवाइस से बंधा, अभी शरीर से नहीं)',
  'live-stats':'लाइव चेन सांख्यिकी',
  's-height':'ब्लॉक हाइट',
  's-humans':'सत्यापित मनुष्य','s-humans-sub':'डिवाइस से बंधा ZK प्रमाण · प्रति डिवाइस एक पंजीकरण',
  's-supply':'कुल आपूर्ति','s-supply-sub':'हमेशा = मनुष्य × 1,000 AEQ',
  's-index':'Aequitas इंडेक्स','s-index-sub':'0 = पूर्ण समानता · 100 = अधिकतम असमानता',
  's-uptime':'अपटाइम','s-uptime-sub':'Node v0.3.0 · Railway (Primary) + 2x Contabo VPS (Secondary) · own PostgreSQL each',
  'ib-poh':'मानवता का प्रमाण','ib-poh-t':'प्रत्येक AEQ धारक Groth16 ZK प्रमाण के माध्यम से डिवाइस से बंधी कुंजी के नियंत्रण को क्रिप्टोग्राफिक रूप से साबित करता है। कोई बॉट, कंपनी या AI नहीं। बायोमेट्रिक डेटा कभी ट्रांसमिट नहीं होता, केवल एक गणितीय प्रमाण। यह आज प्रति डिवाइस एक पंजीकरण को बांधता है, अभी प्रति अद्वितीय व्यक्ति नहीं — FAQ देखें।',
  'ib-fair':'मौलिक रूप से उचित वितरण','ib-fair-t':'प्रत्येक सत्यापित मानव को रजिस्ट्रेशन पर बिल्कुल 1,000 AEQ मिलता है। कोई प्री-माइनिंग नहीं। कुल आपूर्ति = मनुष्य × 1,000।',
  'ib-dag':'BlockDAG आर्किटेक्चर','ib-dag-t':'कई ब्लॉक एक साथ उत्पन्न और मर्ज किए जा सकते हैं। उच्च थ्रूपुट, कम विलंबता।',
  'ib-gas':'सच में निःशुल्क','ib-gas-t':'रजिस्ट्रेशन और AEQ ट्रांसफर में कुछ भी खर्च नहीं होता। ETH, BNB या MATIC की जरूरत नहीं।',
  'recent-blocks':'हालिया ब्लॉक','blocks-desc':'MERGE = कई पेरेंट मर्ज (BlockDAG)। TX = रजिस्ट्रेशन ट्रांजेक्शन। ब्लॉक समय: __BT__।',
  'loading':'ब्लॉक लोड हो रहे हैं...','net-info':'नेटवर्क जानकारी','k-chain':'चेन नाम','k-symbol':'प्रतीक','k-btime':'ब्लॉक समय',
  'k-cons':'सहमति','k-nodes':'सक्रिय नोड्स','k-storage':'स्टोरेज','add-mm':'🦊 METAMASK में जोड़ें','k-dec':'दशमलव',
  'btn-add-mm':'+ AEQUITAS नेटवर्क जोड़ें',
  'phil':'"पैसा इसलिए है क्योंकि लोग हैं।<br>इससे ज़्यादा नहीं, इससे कम नहीं।"','phil-sub':'— AEQUITAS सिद्धांत —',
  'humans-title':'Aequitas Chain पर सत्यापित मनुष्य',
  'h-what':'सत्यापित मानव क्या है?','h-what-t':'एक सत्यापित मानव एक वॉलेट पता है जिसने आज तक क्रिप्टोग्राफिक रूप से एक विशिष्ट डिवाइस की सुरक्षित कुंजी के नियंत्रण को साबित किया है। कुंजी फोन के सुरक्षित हार्डवेयर में बनाई जाती है, जो डिवाइस के अपने स्क्रीन-लॉक द्वारा सुरक्षित है — कोई अलग सेंसर किट नहीं। केवल एक Groth16 ZK प्रमाण प्रेषित होता है, कोई बायोमेट्रिक डेटा डिवाइस नहीं छोड़ता। यह आज एक डिवाइस को सत्यापित करता है, अभी जरूरी नहीं कि एक अद्वितीय व्यक्ति को — FAQ देखें।',
  'h-zkp':'ZK प्रमाण प्रणाली','h-zkp-t':'Aequitas BN128 पर Groth16 उपयोग करता है — Ethereum और Zcash जैसा ही वक्र। ~200 बाइट, ~10ms। commitment = keccak256(deviceKey‖wallet)। Nullifier इस डिवाइस से बंधा है: फोन खोने से इस डिवाइस पर दूसरी पहचान नहीं बनती, लेकिन कोई अन्य डिवाइस अभी भी अलग से पंजीकरण कर सकता है। कुंजी सामग्री कभी सर्वर साइड पर प्रकट या संग्रहीत नहीं होती।',
  'h-sybil':'Sybil प्रतिरोध — वर्तमान स्थिति','h-sybil-t':'आज का nullifier एक डिवाइस से बंधी हार्डवेयर कुंजी से लिया जाता है — यह उसी डिवाइस (या वॉलेट) को दो बार पंजीकृत होने से मज़बूती से रोकता है, लेकिन यह अभी पता नहीं लगा सकता कि क्या वही व्यक्ति दूसरे भौतिक डिवाइस से पंजीकरण कर रहा है। इस अंतर को पाटने के लिए एक वास्तविक क्रॉस-डिवाइस जैविक विशिष्टता जांच की आवश्यकता है, जिसकी योजना है लेकिन अभी उपलब्ध नहीं है।',
  'h-global':'वैश्विक वित्तीय समावेशन','h-global-t':'कोई बैंक खाता, क्रेडिट कार्ड या पूर्व-क्रिप्टोकरेंसी की जरूरत नहीं। बस वह स्क्रीन-लॉक वाला Android स्मार्टफोन जिसे आप पहले से उपयोग करते हैं।',
  'h-bio-hw':'पहचान सत्यापन रोडमैप','h-bio-hw-t':'आज (बीटा): प्रति डिवाइस एक हार्डवेयर कुंजी, ईमानदारी से डिवाइस-बाउंड के रूप में लेबल की गई, शरीर-बाउंड नहीं। योजनाबद्ध (बीटा के बाद): एक मजबूत Sybil प्रतिरोध का दावा करने से पहले परिभाषित, निर्मित और स्वतंत्र रूप से ऑडिट की जाने वाली एक वास्तविक क्रॉस-डिवाइस जैविक विशिष्टता जांच।',
  'reg-humans':'रजिस्टर्ड मनुष्य','h-desc':'नीचे प्रत्येक पते ने ZK प्रमाण के माध्यम से एक डिवाइस से बंधी क्रिप्टोग्राफिक कुंजी के नियंत्रण को साबित किया और बिल्कुल 1,000 AEQ प्राप्त किया। स्थायी, अपरिवर्तनीय, ऑन-चेन। आज Sybil प्रतिरोध के लिए "डिवाइस-बाउंड" का क्या अर्थ है, इसके लिए FAQ देखें।',
  'no-humans':'अभी तक कोई मानव रजिस्टर्ड नहीं।\n\nAequitas Android App डाउनलोड करें और चेन पर पहले मानव बनें!',
  'reg-stats':'रजिस्ट्री आँकड़े','total-humans':'कुल मनुष्य',
  'idx-title':'Aequitas इंडेक्स — रियल-टाइम आर्थिक समानता स्कोर',
  'idx-desc':'Aequitas इंडेक्स <strong style="color:var(--teal)">जिनी गुणांक</strong> से लिया गया है — विश्व बैंक, OECD और UN द्वारा अपनाया गया अंतरराष्ट्रीय मानक। <strong style="color:var(--neon)">0 = पूर्ण समानता</strong>। <strong style="color:var(--red)">100 = अधिकतम एकाग्रता</strong>। लक्ष्य: जिनी 0.30 से कम।',
  'gini-what-title':'जिनी गुणांक क्या है?',
  'gini-what-text':'इतालवी सांख्यिकीविद् कोर्राडो जिनी (1912) द्वारा विकसित। धन वितरण मापता है। पैमाना: 0 (सब समान) से 1 (एक व्यक्ति के पास सब कुछ)। विश्व बैंक, OECD, UN उपयोग करते हैं।',
  'gini-calc-title':'Aequitas इंडेक्स की गणना कैसे होती है?','gini-calc-text':'सभी सत्यापित मानवों के AEQ बैलेंस एकत्र किए जाते हैं। फॉर्मूला हर संभावित जोड़ी के बैलेंस के बीच माध्य निरपेक्ष अंतर की गणना करता है, जनसंख्या वर्ग (n²) और माध्य बैलेंस से सामान्यीकृत। परिणाम 0–1 को 100 से गुणा = Aequitas इंडेक्स।',
  'gini-why-title':'जिनी ही क्यों — कोई सरल मेट्रिक नहीं?','gini-why-text':'सरल अमीर-गरीब अनुपात में हेरफेर आसान है — जिनी इसे पकड़ लेता है। यह गुणांक सभी सत्यापित मानवों के बीच पूर्ण वितरण को एक ऑडिट योग्य संख्या में दर्शाता है। Aequitas इसे ऑन-चेन प्रकाशित करता है — पारदर्शी, विश्व स्तर पर सत्यापन योग्य।',
  'curr-idx':'वर्तमान इंडेक्स','bar-0':'0 — पूर्ण समानता','bar-100':'100 — अधिकतम असमानता','wcap-lbl':'वर्तमान धन सीमा:','wcap-mult':'गुणक:','wcap-avg':'औसत बैलेंस:',
  'phases-desc':'चरण 0 (बूटस्ट्रैप) में धन सीमा एक स्लाइडिंग गुणक का उपयोग करती है: max(5, min(N, 25)) × औसत बैलेंस। 1–4 मनुष्यों के साथ: 5× औसत। हर नया मनुष्य 1× जोड़ता है। 25+ मनुष्यों पर: स्थायी रूप से 25× पर स्थिर। सभी बदलाव मनुष्यों की संख्या से स्वचालित रूप से होते हैं — कोई गवर्नेंस वोट नहीं, कोई एडमिन कुंजी नहीं।',
  'wealth-cap-explain':'चरण 0 (बूटस्ट्रैप) में धन सीमा max(5, min(N, 25)) × औसत AEQ बैलेंस का उपयोग करती है, जहाँ N = पंजीकृत मनुष्य। 1–4 मनुष्य: सीमा = 5× औसत। हर नया मनुष्य 1× जोड़ता है। 25+ पर: स्थायी रूप से 25× पर स्थिर। सीमा हमेशा वर्तमान औसत बैलेंस के साथ बदलती है।',
  'p0':'बूटस्ट्रैप · 100 से कम मनुष्य · धन सीमा: max(5,min(N,25))× औसत · 25वें मनुष्य तक 5× से 25× तक बढ़ता है · वर्तमान में सक्रिय',
  'p1':'विकास · 100–10,000 मनुष्य · धन सीमा: 25× औसत बैलेंस',
  'p2':'स्थिरता · 10,000–1M मनुष्य · धन सीमा: 25× औसत बैलेंस',
  'p3':'परिपक्वता · 1M+ मनुष्य · धन सीमा: 25× औसत बैलेंस',
  'gini':'जिनी गुणांक','gini-desc':'0 = समान · 1 = असमान',
  'supply-desc':'हमेशा = मनुष्य × 1,000 AEQ',
  'phase':'प्रोटोकॉल चरण','phase-desc':'मानवों की संख्या से स्वचालित रूप से आगे बढ़ता है',
  'humans-desc':'डिवाइस से बंधे ZK-सत्यापित पंजीकरण',
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
  'story-text':'<p>2009 में सातोशी नाकामोतो ने Bitcoin जारी किया। पहली बार बैंक के बिना मूल्य हस्तांतरण संभव हुआ। एक सच्ची क्रांति। लेकिन लगभग तुरंत कुछ गलत हो गया।</p><p>शुरुआती माइनर्स ने लाखों सिक्के लगभग शून्य लागत पर जमा किए। 2021 में, शीर्ष 1% Bitcoin पते 90% से अधिक Bitcoin नियंत्रित करते हैं। Bitcoin का जिनी गुणांक 0.85 से अधिक है।</p><p><span style="color:var(--gold)">Aequitas</span> — "न्याय" के लिए लैटिन — एक प्रश्न का उत्तर देने के लिए बनाया गया: <em style="color:var(--gold)">"एक क्रिप्टोकरेंसी कैसी दिखेगी जो हर मानव के लिए न्यायपूर्ण हो?"</em></p><p><strong style="color:var(--text)">पैसा इसलिए है क्योंकि लोग हैं। इसलिए हर व्यक्ति को केवल मानव होने के कारण धन का समान हिस्सा मिलना चाहिए।</strong></p>',
  'nodes-title':'सक्रिय नोड्स — वर्तमान नेटवर्क टोपोलॉजी',
  'nodes-desc':'Aequitas नेटवर्क वर्तमान में कई भौगोलिक रूप से वितरित नोड्स पर चलता है (वर्तमान संख्या ऊपर)। सभी ब्लॉक उत्पादन, स्टेट सिंक्रोनाइज़ेशन और API सेवा में भाग लेते हैं।',
  'run-node-title':'अपना नोड चलाएं','run-node-desc':'कोई भी Aequitas नोड चला सकता है — बिना अनुमति, बिना स्टेक। ऑपरेटर दैनिक वितरित स्वैप शुल्क का 40% कमाते हैं।',
  'bootstrap-title':'नया नोड कनेक्ट करें','bootstrap-desc':'PRIMARY_NODE_URL=https://aequitas.digital अपने environment में सेट करें। आपका नोड स्वचालित रूप से पूर्ण चेन स्टेट सिंक करेगा।',
  'tech-title':'तकनीकी विशिष्टताएं','mm-config':'MetaMask कॉन्फ़िगरेशन',
  'k-lang':'भाषा','k-src':'स्रोत','evm-yes':'हाँ — JSON-RPC /rpc · MetaMask संगत',
  'proto-label':'Aequitas V7 प्रोटोकॉल — तकनीकी दस्तावेज़ीकरण',
  'ca-title':'अनुबंध पते',
  'ca-text':'चेन: Aequitas Chain (Chain ID: 1926 · 0x786)<br>RPC: https://aequitas.digital/rpc<br><br>BioVerifier: 0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2<br>AequitasV7: 0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78',
  'ca-desc':'AequitasV7 पूरी Aequitas अर्थव्यवस्था के लिए एकमात्र सच्चाई का स्रोत है। कोई एडमिन की, अपग्रेड प्रॉक्सी या गवर्नेंस वोट इसका तर्क नहीं बदल सकता।',
  'poa-title':'1. जीवन का प्रमाण','poa-text':'<p>जब लोग मरते हैं, उनका AEQ धीरे-धीरे UBI पूल के माध्यम से समुदाय को वापस जाता है, बजाय Bitcoin की तरह हमेशा के लिए खोने के।</p>',
  'poa-box':'वर्ष 0–2: सामान्य उपयोग<br>वर्ष 2: चेतावनी 1 — Guardian जवाब दे सकता है<br>वर्ष 2+60 दिन: चेतावनी 2<br>वर्ष 2+120 दिन: चेतावनी 3<br>वर्ष 2+180 दिन: AEQ व्यक्तिगत एस्क्रो में<br>वर्ष 4: निष्क्रिय रहने पर — UBI पूल में वापस',
  'guard-title':'2. गार्जियन सिस्टम','guard-text':'<p>एक विश्वसनीय Guardian (दूसरा सत्यापित मानव) पुष्टि कर सकता है कि कोई अभी भी जीवित है, बिना किसी वित्तीय अधिकार के।</p>',
  'guard-box':'प्रति मानव 1 Guardian · दूसरा सत्यापित मानव होना चाहिए<br>Guardian केवल confirmAlive() कॉल कर सकता है · शून्य वित्तीय अधिकार<br>Guardian धन नहीं हिला सकता · अधिकतम 3 · Timelock 7 दिन',
  'dem-title':'3. डेमरेज — संचय-विरोधी तंत्र',
  'dem-box':'दर: 3 महीने की छूट के बाद 0.5%/माह<br>किसी भी ट्रांसफर, स्वैप या लिक्विडिटी पर रीसेट<br>क्षयित AEQ पूलों में पुनर्वितरित (जला नहीं जाता)',
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
  'swap-btn-conn':'🦊 METAMASK कनेक्ट करें','swap-btn-go':'🔄 स्वैप करें',
  'swap-log-hint':'// स्वैप करने के लिए वॉलेट कनेक्ट करें...',
  'swap-no-liquidity':'अभी tUSD नहीं है?','swap-faucet-desc':'पंजीकृत मनुष्य एक बार टेस्ट tUSD का दावा कर सकते हैं','swap-btn-faucet':'💧 टेस्ट tUSD का दावा करें',
  'swap-addliq-title':'लिक्विडिटी प्रदान करें','swap-addliq-desc':'पहले डिपॉजिट करें — आपका अनुपात प्रारंभिक मूल्य तय करता है।','swap-btn-addliq':'💧 लिक्विडिटी जोड़ें',
  'swap-lp-title':'आपकी LP स्थिति','swap-lp-share':'पूल हिस्सा','swap-lp-withdrawable':'निकालने योग्य',
  'swap-lp-pct-label':'आपकी स्थिति का %','swap-lp-youget':'आप प्राप्त करेंगे','swap-btn-removeliq':'🔥 लिक्विडिटी हटाएं',
  'swap-pool-title':'AEQ / tUSD — पूल स्थिति',
  'swap-pool-aeq':'AEQ रिजर्व','swap-pool-tusd':'tUSD रिजर्व','swap-pool-price':'स्पॉट मूल्य',
  'swap-fee-bps':'स्वैप शुल्क',
  'swap-pools-addr-title':'टोकनोमिक्स पूल पते','pools-addr-title':'पूल अनुबंध पते',
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
  'usp-c3-title':'सभी के लिए सुलभ','usp-c3-desc':'कोई बैंक खाता, क्रेडिट कार्ड या सरकारी ID नहीं चाहिए, खरीदने के लिए कोई अतिरिक्त हार्डवेयर नहीं — बस वह स्क्रीन-लॉक जो आपके Android फोन में पहले से मौजूद है।',
  'usp-c4-title':'हमेशा के लिए दैनिक UBI','usp-c4-desc':'पंजीकरण के बाद, आपका UBI हिस्सा हर दिन स्वचालित रूप से आता है — बिना किसी कार्रवाई के।',
  'v7-intro-title':'AequitasV7 क्या है?',
  'v7-intro-text':'AequitasV7, Aequitas प्रोटोकॉल का केंद्रीय स्मार्ट अनुबंध है। Aequitas Chain (ID 1926) पर अपरिवर्तनीय रूप से तैनात। सब कुछ प्रबंधित करता है: मानव पंजीकरण, ZK सत्यापन, बैलेंस प्रबंधन, धन सीमा, UBI वितरण, स्वैप शुल्क। कोई व्यवस्थापक इसे अपडेट नहीं कर सकता।',
  'explore-title':'Aequitas एक्सप्लोर करें',
  'expl-score':'समानता स्कोर','expl-score-d':'लाइव जिनी गुणांक · Aequitas इंडेक्स · रियल-टाइम धन वितरण',
  'expl-economy':'UBI और पुनर्वितरण','expl-economy-d':'दैनिक UBI काउंटडाउन · 4 ऑन-चेन पूल · डेमरेज · प्रोटोकॉल चरण',
  'expl-charts':'चार्ट और इतिहास','expl-charts-d':'जिनी इतिहास · लॉरेंज वक्र · धन सीमा स्लाइडर · Aequitas की कहानी',
  'expl-v7':'प्रोटोकॉल V7 दस्तावेज़','expl-v7-d':'AequitasV7 अनुबंध · 6 तंत्र · ZK प्रमाण · धन सीमा · डेमरेज · अपरिवर्तनीय कोड',
  'expl-explorer':'ब्लॉक एक्सप्लोरर','expl-explorer-d':'लाइव BlockDAG · वैलिडेटर, हैश, ट्रांजेक्शन देखने के लिए किसी भी ब्लॉक पर क्लिक करें',
  'swap-sell-label':'बेचें','swap-receive-label':'प्राप्त करें',
  'expl-network':'नेटवर्क और नोड्स','expl-network-d':'नोड टोपोलॉजी · अपना नोड चलाएं · तकनीकी विशिष्टताएं · Chain ID 1926',
  'guard-title':'🛡 गार्जियन सिस्टम','guard-my-lbl':'मेरा गार्जियन','guard-none':'कोई नहीं',
  'guard-set-lbl':'गार्जियन सेट / बदलें','guard-set-hint':'Aequitas का पंजीकृत मानव होना आवश्यक · 7-दिन का टाइम लॉक · गार्जियन केवल आपकी जीवितता की पुष्टि कर सकता है, फंड तक नहीं पहुंच सकता · प्रति गार्जियन अधिकतम 3 वार्ड',
  'guard-confirm-lbl':'जीवित होने की पुष्टि करें (गार्जियन के रूप में)','guard-confirm-hint':'यदि आपका वार्ड अपने वॉलेट तक नहीं पहुंच सकता, तो 910 दिनों की निष्क्रियता के बाद उनके फंड एस्क्रो में जाने से रोकने के लिए उनकी जीवितता की पुष्टि करें।','guard-recover-btn':'🔓 एस्क्रो से वापस लें',
  'faq-title':'❓ सामान्य प्रश्न','faq-q1':'क्या मेरा बायोमेट्रिक डेटा सुरक्षित है?','faq-a1':'हाँ। Aequitas द्वारा कोई भी बायोमेट्रिक डेटा कभी कैप्चर, प्रोसेस या ट्रांसमिट नहीं किया जाता। आपके फोन का स्क्रीन-लॉक केवल उसके सुरक्षित हार्डवेयर में बनाई और संग्रहीत एक रैंडम कुंजी तक पहुंच देता है। केवल उस कुंजी से प्राप्त एक गणितीय प्रमाण भेजा जाता है — न कि कुंजी स्वयं, न ही कोई बायोमेट्रिक डेटा।',
  'faq-q1b':'क्या पंजीकरण साबित करता है कि मैं एक अद्वितीय वास्तविक व्यक्ति हूं?','faq-a1b':'अभी पूरी तरह से नहीं। आज का प्रमाण क्रिप्टोग्राफिक रूप से यह साबित करता है कि आप एक विशिष्ट डिवाइस की सुरक्षित कुंजी को नियंत्रित करते हैं — यह उसी डिवाइस (या वॉलेट) को दो बार पंजीकृत होने से मज़बूती से रोकता है, लेकिन यह अभी यह पता नहीं लगा सकता कि क्या एक ही व्यक्ति के पास दो अलग-अलग भौतिक डिवाइस हैं। एक वास्तविक क्रॉस-डिवाइस जैविक विशिष्टता जांच की योजना है, अभी उपलब्ध नहीं है — हम आज के डिवाइस-बाउंड प्रमाण की गारंटी को बढ़ा-चढ़ाकर बताने के बजाय इसे स्पष्ट रूप से कहना पसंद करते हैं।',
  'faq-q2':'क्या मैं बाद में अलग वॉलेट से रजिस्टर कर सकता/सकती हूं?','faq-a2':'नहीं। पंजीकरण प्रति डिवाइस कुंजी एक वॉलेट पते से स्थायी रूप से जुड़ा होता है। यह डिज़ाइन के अनुसार है — यह उसी डिवाइस को फिर से पंजीकृत होने से रोकता है और प्रति डिवाइस पहचान एक वॉलेट सुनिश्चित करता है।',
  'faq-q3':'अगर मैं अपना फोन खो दूं तो क्या होगा?','faq-a3':'आपके AEQ आपके वॉलेट में रहते हैं — वे आपकी प्राइवेट कुंजी से जुड़े हैं, फोन से नहीं। आप अभी भी अपने सीड फ्रेज से MetaMask के जरिए वॉलेट एक्सेस कर सकते हैं। वॉलेट रिकवरी बायोमेट्रिक पंजीकरण से स्वतंत्र है।',
  'path-title':'अपना रास्ता चुनें','path-human-title':'मैं एक मानव हूं','path-human-desc':'मैं पंजीकरण करना, 1,000 AEQ प्राप्त करना और बेसिक इनकम नेटवर्क में शामिल होना चाहता/चाहती हूं।','path-human-steps':'1. Aequitas Android ऐप डाउनलोड करें<br>2. अपने डिवाइस के स्क्रीन-लॉक से अनलॉक करें (फिंगरप्रिंट/फेस/PIN)<br>3. MetaMask कनेक्ट करें<br>4. तुरंत 1,000 AEQ प्राप्त करें',
  'path-node-title':'मैं एक नोड ऑपरेटर हूं','path-node-desc':'मैं पूर्ण नोड चलाना, ब्लॉक उत्पादन में भाग लेना और 40% वैलिडेटर पूल से कमाना चाहता/चाहती हूं।','path-node-steps':'1. मानव के रूप में रजिस्टर करें (अनिवार्य)<br>2. PRIMARY_NODE_URL=https://aequitas.digital सेट करें<br>3. Railway/Contabo/VPS पर डिप्लॉय करें<br>4. वैलिडेटर पूल से दैनिक कमाएं',
  'path-dev-title':'मैं एक डेवलपर हूं','path-dev-desc':'मैं Aequitas पर निर्माण करना, API को एकीकृत करना या प्रोटोकॉल में योगदान देना चाहता/चाहती हूं।','path-dev-steps':'1. EVM-संगत JSON-RPC<br>2. Chain ID: 1926 · RPC: /rpc<br>3. OpenAPI: /api/* एंडपॉइंट<br>4. मेट्रिक्स: /metrics (Prometheus)',
  'story-flow-title':'AEQ टोकन प्रवाह आरेख','story-topo-title':'नेटवर्क टोपोलॉजी — वर्तमान स्थिति',
  'swap-price-title':'AEQ / tUSD — लाइव मूल्य','swap-price-desc':'पूल रिज़र्व से रियल-टाइम मूल्य (x·y=k)। हर 8 सेकंड में नए पूल डेटा के साथ अपडेट।','swap-price-empty':'अभी पूल डेटा नहीं — मूल्य चार्ट देखने के लिए लिक्विडिटी जोड़ें।',
  'node-guide-lang-note':'यह इनलाइन गाइड अंग्रेज़ी में है। आपकी भाषा में PDF ऊपर के बटन से उपलब्ध है।',
  'k-zkp':'ZKP सिस्टम','k-hash':'हैश सिस्टम','k-sybil-prot':'Sybil सुरक्षा',
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

function setLang(lang) {
  curLang = lang;
  document.getElementById('lang-sel').value = lang;
  document.documentElement.dir = lang === 'ar' ? 'rtl' : 'ltr';
  document.documentElement.lang = lang;
  const t = T[lang];
  if (!t) return;
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
        rpcUrls: ['https://aequitas.digital/rpc'],
        blockExplorerUrls: ['https://aequitas.digital']
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
    const nodes = [{ url: selfUrl, isPrimary: !!statusRes.is_primary, self: true, gitCommit: statusRes.git_commit }]
      .concat(dedupedPeers.map(function(u) { return { url: u, isPrimary: false, self: false, gitCommit: null }; }));

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
    await Promise.all(nodes.filter(function(n) { return !n.self; }).map(function(n) {
      return fetch(n.url + '/api/status').then(function(r) { return r.json(); }).then(function(s) {
        n.gitCommit = s.git_commit || null;
      }).catch(function() { n.gitCommit = null; });
    }));
    if (mySeq !== loadTopologySeq) return;

    if (grid) {
      const cols = Math.max(1, Math.min(nodes.length, 4));
      grid.style.gridTemplateColumns = 'repeat(' + cols + ', 1fr)';
      const commits = nodes.map(function(n) { return n.gitCommit; }).filter(Boolean);
      const allSame = commits.length > 1 && commits.every(function(c) { return c === commits[0]; });
      grid.innerHTML = nodes.map(function(n) {
        let host;
        try { host = new URL(n.url).host; } catch (e) { host = n.url; }
        const role = n.isPrimary ? 'Primary' : 'Secondary';
        const tag = n.self ? ' (this node)' : '';
        const mismatch = n.gitCommit && commits.length > 1 && !allSame;
        const commitLine = '<div class="ndesc" style="' + (mismatch ? 'color:var(--dag-red);font-weight:700' : 'color:var(--muted)') + '" title="Build commit this node is running — compare across nodes to confirm the fleet is in sync">'
          + (n.gitCommit ? ('commit ' + sanitize(n.gitCommit) + (mismatch ? ' ⚠ differs from other nodes' : '')) : 'commit — (peer unreachable from this page, e.g. plain-HTTP node on an HTTPS page)')
          + '</div>';
        return '<div class="nbox">' +
          '<div class="nstat"><span class="ndot"></span>' + sanitize(role + tag) + '</div>' +
          '<div class="nurl">' + sanitize(host) + '</div>' +
          '<div class="ndesc">API &middot; Block producer &middot; P2P peer &middot; Own PostgreSQL state</div>' +
          commitLine +
          '</div>';
      }).join('');
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
    const roleLine = (node.isPrimary ? 'Primary' : 'Secondary') + (node.self ? ' (this node)' : '');
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
    const history = (d.history || []).slice().reverse();
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
    ctx.fillStyle='rgba(4,120,87,0.85)'; ctx.font='bold 9px JetBrains Mono,monospace'; ctx.textAlign='right';
    ctx.fillText('TARGET 0.30', W-pad.r-2, targetY-5);
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
    var lpt=history[history.length-1], lx=toX(history.length-1), ly=toY(lpt.idx);
    ctx.fillStyle='rgba(200,168,76,0.95)'; ctx.font='bold 11px JetBrains Mono,monospace';
    ctx.textAlign = lx>W*0.7?'right':'left';
    ctx.fillText('Gini: '+lpt.idx.toFixed(3), lx+(lx>W*0.7?-8:8), ly-9);
    // title
    ctx.fillStyle='rgba(107,70,193,0.55)'; ctx.font='10px Inter,sans-serif'; ctx.textAlign='left';
    ctx.fillText('GINI INDEX HISTORY  —  0 = perfect equality  ·  100 = max inequality', pad.l, 20);
  } catch(e) {}
}

async function drawLorenzCurve() {
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
    var d = await (await fetch('/api/humans')).json();
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
    // Apply same small-sample bias correction as Go's calcGiniFromBalances: gini * n/(n-1)
    // Without this the Lorenz Gini differs from the Score Gini by factor n/(n-1).
    // At n=7: 0.0841 * 7/6 = 0.0981 — matching the server value.
    if(n>1) gini=Math.min(1, gini * n/(n-1));

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
  data.forEach(function(pt) {
    var b = Math.floor(pt.t/tfMs)*tfMs;
    if (!buckets[b]) { buckets[b]={time:Math.floor(b/1000),open:pt.p,high:pt.p,low:pt.p,close:pt.p,vol:1}; }
    else { buckets[b].high=Math.max(buckets[b].high,pt.p); buckets[b].low=Math.min(buckets[b].low,pt.p); buckets[b].close=pt.p; buckets[b].vol++; }
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
    (rawBlocks || []).forEach(function(b) { siblingsAt[b.height] = (siblingsAt[b.height] || 0) + 1; });
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
        const txCount = (b.transactions || []).length;
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
    // Collect all transactions across blocks, newest first
    if (txList) {
      const allTxs = [];
      dedupedBlocks.forEach(function(b) {
        (b.transactions || []).forEach(function(tx) {
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
    const d = await (await fetch('/api/humans')).json();
    if (mySeq !== loadHumansSeq) return;
    document.getElementById('h-count').textContent = fmt(d.total);
    const list = document.getElementById('humans-list');
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
        if (!existing.has(pt.t)) priceHistory.push({t: pt.t, p: pt.p});
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
      if (!existing.has(pt.t)) priceHistory.push({t: pt.t, p: pt.p});
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
      priceHistory.push({ t: Date.now(), p: d.price_aeq_in_tusd });
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
  const expSearchInput = document.getElementById('exp-search-input');
  if (expSearchInput) expSearchInput.addEventListener('keydown', function(ev) { if (ev.key === 'Enter') expSearch(); });
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

document.addEventListener('click', function(ev) {
  const el = ev.target.closest('[data-act]');
  if (!el) return;
  const fn = CLICK_ACTIONS[el.getAttribute('data-act')];
  if (typeof fn !== 'function') return;
  let args = [];
  const raw = el.getAttribute('data-args');
  if (raw) {
    try { args = JSON.parse(raw); } catch (e) { args = []; }
  }
  fn.apply(null, args.concat([el, ev]));
});

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
    rpcMap: { 1926: 'https://aequitas.digital/rpc' },
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

