# Übersetzungs-Audit der Website

**Stand 26.08.2026, maschinell erhoben.** Jede Zeile von `assets/explorer.html` geprüft.

## Ergebnis

| | |
|---|---|
| Sichtbare Textstellen **ohne** `data-i18n` (außerhalb der Leitfäden) | **392** |
| Bei 12 Sprachen entspricht das | ~4704 Übersetzungen |

Die beiden Leitfäden (Node und Bio-Verifier) sind hier **ausgenommen**: sie sind per Entwurf englisch, mit Sprachhinweis und übersetzten PDFs — dieselbe Regel, die der Node-Guide seit jeher befolgt.

## Was das bedeutet

Diese Stellen bleiben in **allen zwölf Sprachen englisch**. Die vorhandenen Wächtertests bemerken das nicht: sie prüfen, ob jeder *vorhandene* Schlüssel in jeder Sprache existiert — nicht, ob Text überhaupt einen Schlüssel hat. Genau diese Lücke schließt dieser Bericht.

## Die Stellen

| Zeile | Text |
|---|---|
| 10 | Aequitas — Proof of Humanity Chain |
| 95 | Android APK · direct download |
| 125 | 1,000 AEQ will be credited automatically |
| 134 | 📱 MetaMask Mobile: if AEQ balance shows 0 after registration, go to Settings → Networks → delete Aequitas Chai |
| 141 | ⊘ DISCONNECT WALLET |
| 164 | 🛡 SET GUARDIAN |
| 173 | ✓ CONFIRM WARD IS ALIVE |
| 183 | Aequitas Chain (BlockDAG) |
| 185 | 1,000 AEQ per human |
| 189 | Groth16 ZKP (Zero-Knowledge) |
| 216 | BlockDAG · Parallel production |
| 221 | Device-bound ZK proof · one registration per device |
| 231 | Multi-validator network |
| 236 | Distinct proposers, recent blocks |
| 242 | · KnightDAG-secured |
| 257 | Fill = GHOSTDAG verdict · thin ring = proposer · one column per height. Hover any block for details. |
| 269 | canonical winner |
| 269 | Active Validators |
| 273 | Loading blocks... |
| 285 | Hash / Wallet |
| 286 | Loading transactions... |
| 309 | Today this verifies one device, not yet one unique person |
| 314 | commitment = keccak256(deviceKey ‖ wallet) |
| 330 | Today (beta) |
| 331 | Planned (post-beta) |
| 349 | Groth16 / BN128 |
| 352 | Permanent · On-chain |
| 468 | ⊘ DISCONNECT WALLET |
| 484 | Provide AEQ / tUSD liquidity to earn 30% of all swap fees, distributed daily. |
| 503 | 0.1% · split 40/30/20/10 |
| 506 | AEQ_reserve × tUSD_reserve = k (constant) |
| 564 | 📈 Distribution |
| 572 | Gini coefficient |
| 572 | entire distribution |
| 572 | 0 = perfect equality |
| 572 | 100 = maximum concentration |
| 586 | ⚠ N&lt;10: not yet significant |
| 611 | Aequitas Index = G × 100 |
| 619 | Healthier than most nations on Earth. Comparable to Scandinavia (0.27) and Germany (0.31). Wealth cap and demu |
| 624 | Comparable to the USA (0.41) or France (0.32). Within the range of most developed economies. Redistribution me |
| 629 | Higher than most European nations — comparable to Brazil (0.53) or Russia. Protocol redistribution at elevated |
| 634 | Worse than any country on Earth (South Africa record: 0.63). Approaching Bitcoin (0.85). Protocol at maximum i |
| 653 | Gini Index History |
| 654 | Recorded after each UBI distribution. Shows how equality evolves as the network grows. Lower is better — targe |
| 656 | No snapshots yet — first one saved after the next UBI distribution. |
| 663 | Wealth Distribution Analysis |
| 664 | Lorenz Curve — AEQ Distribution Across Humans |
| 666 | Lorenz Curve |
| 666 | diagonal line = perfect equality |
| 674 | Aequitas Now |
| 676 | Gini coefficient (0–1) |
| 681 | Like Scandinavia (~0.27) |
| 684 | Bitcoin Gini |
| 686 | Most unequal currency ever |
| 690 | How to read this chart: |
| 781 | max(5, min(N, 25))× average AEQ balance |
| 795 | Wealth Cap Multiplier — Bootstrap Slider |
| 809 | The year is 2009. Satoshi Nakamoto releases Bitcoin. For the first time, value can transfer between any two pe |
| 813 | "What would a cryptocurrency look like if designed from first principles to be fair to every human being?" |
| 815 | Money exists because people exist. Therefore every person should have an equal share of money simply by virtue |
| 816 | Aequitas implements this mathematically. Every verified human receives exactly 1,000 AEQ &#8212; billionaire o |
| 817 | "Money exists because people exist. Nothing more, nothing less." |
| 824 | The Core Innovation |
| 826 | ZK Device-Key Proof |
| 827 | No-Stake Blockchain |
| 828 | One Human = One Wallet = 1,000 AEQ |
| 832 | The 4 Redistribution Mechanisms |
| 834 | UBI Pool (20%) |
| 835 | Validators Pool (40%) |
| 836 | Liquidity Pool (30%) |
| 837 | Treasury (10%) |
| 844 | Phase Roadmap &#8212; The Path to Global Scale |
| 850 | 0 &#8211; 100 humans. Sliding wealth cap 5x &#8594; 25x. Foundation building. |
| 855 | 100 &#8211; 10,000 humans. Fixed cap 25x. Open node joining. |
| 860 | 10,000 &#8211; 1M humans. Min 10 nodes. Fully decentralized. |
| 865 | 1M+ humans. Global UBI at scale. Gini target &lt;0.30. |
| 868 | Phase transitions are automatic &#8212; triggered by human count thresholds, enforced by the smart contract. N |
| 873 | Guardian System &#8212; Human Failsafe for Lost Wallets |
| 874 | What happens when someone is hospitalized, incarcerated, or dies? In most crypto systems, lost wallets mean lo |
| 877 | What is a Guardian? |
| 878 | trusted verified human |
| 881 | Inactivity Timeline |
| 883 | 0 &#8211; 2 years |
| 894 | Year 2 +180d |
| 899 | Key protections: |
| 904 | Sybil Resistance &#8212; Current State, Honestly |
| 908 | The matching threshold has not yet been calibrated against real captures |
| 911 | How It Works |
| 915 | What is and is not private: |
| 915 | Honest limitation: |
| 920 | The Vision &#8212; A Global Basic Income Protocol |
| 921 | "Imagine a world where every person on Earth &#8212; regardless of where they were born, what language they sp |
| 925 | humans could register |
| 929 | Gini target (Scandinavian level) |
| 933 | admin keys or governance votes |
| 950 | ⚙️ Run a Node |
| 951 | 📜 Protocol V7 |
| 970 | LIBP2P BOOTSTRAP ADDRESSES |
| 971 | /ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm |
| 972 | /ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN |
| 973 | BOOTSTRAP_P2P_ADDR |
| 979 | Architecture |
| 979 | BlockDAG (Directed Acyclic Graph) |
| 980 | EVM Compatible |
| 982 | BlockDAG + Proof of Humanity |
| 983 | P2P Protocol |
| 983 | libp2p (Go implementation) |
| 984 | Groth16 / snarkjs / circom |
| 985 | Elliptic Curve |
| 986 | keccak256 (post-quantum safe) |
| 987 | PostgreSQL (persistent) |
| 988 | Go 1.24 (chain) · Node.js (proof server) |
| 989 | GitHub — Open Source |
| 995 | Add Aequitas Chain to MetaMask to view your AEQ balance, send transactions, and interact with the V7 contract  |
| 997 | Aequitas Chain |
| 998 | https://aequitas.digital/rpc |
| 1004 | 📱 MetaMask Mobile: if AEQ shows 0 after adding, delete the network and re-add it using the button above. |
| 1014 | Core Technology |
| 1017 | GHOSTDAG + KnightDAG |
| 1018 | This is not a 0815 blockchain with one block at a time. Aequitas runs a real BlockDAG, ordered by GHOSTDAG — a |
| 1022 | Why a normal blockchain isn't enough |
| 1026 | Traditional blockchain — wasted work |
| 1039 | Two validators produce at once → one wins, one is discarded — wasted work, and it caps how fast the network ca |
| 1042 | Aequitas BlockDAG — nothing wasted |
| 1055 | Both blocks are kept — GHOSTDAG merges the concurrent one in and still counts it toward the canonical order. |
| 1060 | GHOSTDAG (2018) — one true order out of a tangled graph |
| 1061 | "blue score" |
| 1065 | ◆ KNIGHTDAG (2026) — Aequitas's own upgrade beyond fixed-K GHOSTDAG |
| 1109 | +1,000 AEQ minted |
| 1112 | AEQ ACTIVITY |
| 1113 | Swap fees (0.1%) |
| 1114 | Demurrage (0.5%/mo) |
| 1115 | Wealth cap overflow |
| 1116 | Inactive escrow |
| 1119 | REDISTRIBUTION |
| 1120 | &#9679; UBI Pool 20% |
| 1121 | &#9679; Validators 40% |
| 1122 | &#9679; Liquidity LP 30% |
| 1123 | &#9679; Treasury 10% |
| 1124 | paid out daily |
| 1125 | automatic on-chain |
| 1127 | daily UBI returns to all verified humans |
| 1142 | Loading topology… |
| 1154 | NODE_OPERATOR_WALLET must be a registered Aequitas human |
| 1156 | 📄 Node Operator Guide (PDF) |
| 1159 | 🐙 View Source on GitHub |
| 1162 | 🔗 Generate Node Binding Signature |
| 1174 | each human registers only once |
| 1180 | not public yet |
| 1187 | additive share |
| 1187 | duplicate: yes or no |
| 1194 | several different verifiers |
| 1194 | one human may hold exactly one validator key |
| 1203 | v1.0 &middot; August 2026 |
| 1210 | A registered Aequitas human account. |
| 1211 | A small Linux server with Docker. |
| 1212 | A domain name with HTTPS. |
| 1213 | To stay online. |
| 1220 | # Encrypts what little is stored on disk |
| 1222 | # Defines your own projection — see the note below |
| 1224 | # Your Ed25519 attestation key pair |
| 1233 | Your own projection seed matters: |
| 1241 | # --- paste, with your own values --- |
| 1251 | Leave ALLOW_REAL_BIOMETRIC_DATA on false |
| 1264 | # Did it come up? |
| 1268 | plaintext_templates: 0 |
| 1268 | sketch_seed_configured: true |
| 1275 | # /root/Caddyfile |
| 1299 | # Reachable from outside, with a valid certificate? |
| 1301 | # Does the network see you and agree with the others? |
| 1329 | Complete step-by-step guide &middot; No prior blockchain experience required &middot; Estimated time: 20&ndash |
| 1335 | What is an Aequitas Node? |
| 1336 | An Aequitas node is a program that runs in the cloud and participates in the Aequitas network. It keeps a copy |
| 1340 | Before You Start &mdash; What You Need |
| 1342 | An Aequitas account: |
| 1343 | A GitHub account (free): |
| 1344 | A Linux server (VPS): |
| 1345 | Node signing key (RELAYER_PRIVATE_KEY): |
| 1345 | registered Aequitas human wallet |
| 1346 | 20&ndash;40 minutes of your time. |
| 1350 | Step 1 &mdash; Fork the Aequitas Repository on GitHub |
| 1351 | What is a fork? |
| 1351 | A fork is your own personal copy of the Aequitas code on GitHub. You only need one if you intend to modify the |
| 1353 | github.com/hanoi96international-gif/Aequitas |
| 1360 | Step 2 &mdash; Create a PostgreSQL Database |
| 1361 | What is a database? |
| 1361 | own dedicated database |
| 1363 | On your VPS (Contabo, Hetzner, DigitalOcean, or any Linux server) |
| 1364 | One database per node. |
| 1366 | # 1. Install PostgreSQL (Ubuntu / Debian) |
| 1369 | # 2. Create database and user (run as root or with sudo) |
| 1370 | "CREATE USER aequitas WITH PASSWORD 'CHOOSE_A_STRONG_PASSWORD';" |
| 1371 | "CREATE DATABASE aequitas OWNER aequitas;" |
| 1372 | # 3. Allow connections from Docker containers (Docker bridge: 172.17.0.0/16) |
| 1373 | "SHOW hba_file;" |
| 1374 | "host aequitas aequitas 172.17.0.0/16 md5" |
| 1375 | "ALTER SYSTEM SET listen_addresses = '*';" |
| 1377 | # 4. Test (should print one row) |
| 1378 | "postgres://aequitas:CHOOSE_A_STRONG_PASSWORD@localhost:5432/aequitas" |
| 1379 | # Your DATABASE_URL for the docker run command below: |
| 1380 | # postgres://aequitas:CHOOSE_A_STRONG_PASSWORD@172.17.0.1:5432/aequitas |
| 1381 | # (172.17.0.1 = Docker bridge gateway, how containers reach the VPS host) |
| 1383 | ufw deny 5432 |
| 1386 | Step 3 &mdash; Understand the Environment Variables |
| 1387 | Environment variables are configuration settings you pass to your node before it starts. Think of them like a  |
| 1388 | Security Warning: |
| 1388 | Your RELAYER_PRIVATE_KEY is like a master password. Anyone who has it controls your node wallet. Never share i |
| 1393 | What to enter and where to find it |
| 1396 | DATABASE_URL |
| 1398 | postgres://user:pass@host:5432/dbname |
| 1401 | RELAYER_PRIVATE_KEY |
| 1403 | The private key (starts with 0x, 66 characters total) of your dedicated node wallet. In MetaMask: click accoun |
| 1406 | RELAYER_ADDRESS |
| 1408 | The wallet address (starts with 0x, 42 characters) matching RELAYER_PRIVATE_KEY. This is the public address &m |
| 1411 | NODE_OPERATOR_WALLET |
| 1413 | Your Aequitas human wallet address &mdash; the one you registered with via the Android app. This wallet receiv |
| 1418 | No longer required. Validator authorization is now identity-based: a verified NODE_OPERATOR_WALLET plus the bi |
| 1421 | NODE_OPERATOR_BINDING_SIGNATURE |
| 1422 | For multi-node |
| 1423 | /node-binding |
| 1428 | http://YOUR-IP:8080 |
| 1431 | PRIMARY_NODE_URL |
| 1432 | For multi-node |
| 1433 | https://aequitas.digital |
| 1438 | Leave unset unless your host requires a specific port. Default is 8080. |
| 1443 | SAVE THIS AS NODE_KEY ENVIRONMENT VAR: &lt;base64&gt; |
| 1446 | IS_PRIMARY_NODE |
| 1448 | Removed — does nothing. Leave unset. |
| 1451 | DISTRIBUTION_ENABLED |
| 1456 | BOOTSTRAP_SNAPSHOT_URL |
| 1458 | https://aequitas.digital/api/snapshot |
| 1461 | BOOTSTRAP_SIGNER |
| 1463 | https://aequitas.digital/api/status |
| 1463 | signing_address |
| 1466 | SNAPSHOT_TOKEN |
| 1468 | Optional &mdash; no longer required to bootstrap a new node. Without a token, BOOTSTRAP_SNAPSHOT_URL still dow |
| 1471 | RESYNC_FROM_SNAPSHOT |
| 1476 | AUTO_HEAL_ON_DIVERGENCE |
| 1477 | Strongly recommended |
| 1478 | Important for network security and speed: |
| 1481 | SNAPSHOT_RESTRICT_TO_PRIVATE_NETWORK |
| 1488 | DANGEROUS: Setting this to true wipes your entire database on every restart. Development use only. Never in pr |
| 1491 | RESET_DB_STATE |
| 1493 | DANGEROUS, one-time use: truncates bootstrap-related tables (including evm_upgrade_relationship_slots) so a no |
| 1496 | CLEAR_REGISTRATIONS |
| 1498 | DANGEROUS, one-time use: wipes all human registration data (chain_accounts' human flags, nullifiers, bio_hashe |
| 1501 | CLEAR_REGISTRATIONS_CONFIRM |
| 1503 | I_UNDERSTAND_THIS_DELETES_ALL_REGISTRATIONS |
| 1506 | ALLOW_DESTRUCTIVE_MAINTENANCE |
| 1511 | BOOTSTRAP_P2P_ADDR |
| 1513 | Overrides the built-in libp2p bootstrap multiaddress (see Step 7 below). Only needed if you want to pin a spec |
| 1528 | MPC_COMMITTEE_SIZE |
| 1529 | MPC_TRIPLE_FILE |
| 1529 | /data/triples-party-N.bin |
| 1538 | Step 4 &mdash; Alternative: Deploy on a VPS with Docker |
| 1539 | Do not point two nodes at one PostgreSQL |
| 1541 | # 1. Install Docker (if not already installed) |
| 1543 | # 2. Clone and build the node (~3 min Go compile) |
| 1546 | # 3. First start (NODE_KEY will be printed in logs — see step 4) |
| 1547 | #    DATABASE_URL uses 172.17.0.1 = Docker bridge gateway → host PostgreSQL |
| 1548 | #    (set up PostgreSQL first via Step 2 Option B above) |
| 1550 | DATABASE_URL |
| 1550 | postgres://aequitas:YOUR_DB_PASSWORD@172.17.0.1:5432/aequitas |
| 1551 | RELAYER_PRIVATE_KEY |
| 1551 | 0xYOUR_PRIVATE_KEY |
| 1552 | RELAYER_ADDRESS |
| 1552 | 0xYOUR_NODE_SIGNING_ADDRESS |
| 1553 | NODE_OPERATOR_WALLET |
| 1553 | 0xYOUR_REGISTERED_HUMAN_WALLET |
| 1554 | NODE_OPERATOR_BINDING_SIGNATURE |
| 1554 | generate-at-/node-binding |
| 1555 | http://YOUR-SERVER-IP:8080 |
| 1556 | PRIMARY_NODE_URL |
| 1557 | BOOTSTRAP_SNAPSHOT_URL |
| 1558 | BOOTSTRAP_SIGNER |
| 1559 | AUTO_HEAL_ON_DIVERGENCE |
| 1559 | # strongly recommended |
| 1561 | # 4. Copy NODE_KEY from logs (do this once — gives your node a stable P2P identity) |
| 1563 | # 5. Stop, add NODE_KEY, restart permanently: |
| 1566 | DATABASE_URL |
| 1566 | postgres://aequitas:YOUR_DB_PASSWORD@172.17.0.1:5432/aequitas |
| 1567 | RELAYER_PRIVATE_KEY |
| 1567 | 0xYOUR_PRIVATE_KEY |
| 1568 | RELAYER_ADDRESS |
| 1568 | 0xYOUR_NODE_SIGNING_ADDRESS |
| 1569 | NODE_OPERATOR_WALLET |
| 1569 | 0xYOUR_REGISTERED_HUMAN_WALLET |
| 1570 | NODE_OPERATOR_BINDING_SIGNATURE |
| 1570 | generate-at-/node-binding |
| 1571 | base64-from-step-4 |
| 1572 | http://YOUR-SERVER-IP:8080 |
| 1573 | PRIMARY_NODE_URL |
| 1574 | BOOTSTRAP_SNAPSHOT_URL |
| 1575 | BOOTSTRAP_SIGNER |
| 1576 | AUTO_HEAL_ON_DIVERGENCE |
| 1576 | # strongly recommended |
| 1580 | /root/.aequitas.env |
| 1580 | --env-file /root/.aequitas.env |
| 1583 | Port requirements: |
| 1583 | ufw allow 8080/tcp |
| 1587 | Step 5 &mdash; Verify Your Node is Running |
| 1588 | YOUR-NODE-URL |
| 1591 | &nbsp;&rarr; Expected: {"height": 1234, "total_humans": N, "aequitas_index": N} |
| 1593 | &nbsp;&rarr; Expected: {"jsonrpc":"2.0","error":"method not specified"} &mdash; this confirms RPC is alive |
| 1595 | The block height should match the primary node within 1&ndash;2 blocks within seconds of startup. If it stays  |
| 1598 | Step 5b &mdash; Link Wallet for Rewards |
| 1602 | ✓ Usually automatic — most users skip this step |
| 1603 | automatically |
| 1603 | NODE_OPERATOR_WALLET |
| 1604 | You only need Step 5b if: |
| 1606 | [NODE] validator key not authorized |
| 1607 | You want to change your reward wallet without restarting the node |
| 1608 | You are running a Docker/VPS node and auto-registration failed |
| 1614 | Check if already registered: |
| 1614 | [PEERS] Registered with primary node |
| 1619 | Manual Registration (if auto-registration failed) |
| 1621 | RELAYER_ADDRESS |
| 1621 | [NODE] Signing address: 0x... |
| 1625 | ℹ The signature is fetched automatically from your node — you only need to provide your RELAYER_ADDRESS above  |
| 1626 | 🔑 Register Validator Key with MetaMask |
| 1631 | Step 6 &mdash; Connect MetaMask to Your Node (Optional) |
| 1632 | Add a network manually |
| 1634 | Network Name |
| 1634 | Aequitas Chain |
| 1635 | https://YOUR-NODE-URL/rpc |
| 1637 | Currency Symbol |
| 1639 | Block Explorer |
| 1639 | https://aequitas.digital |
| 1643 | Step 7 &mdash; Earning Validator Rewards |
| 1644 | 20:00 Berlin time |
| 1646 | Make sure you are registered as a human on Aequitas. If not: install the Android app and complete biometric re |
| 1647 | NODE_OPERATOR_WALLET |
| 1648 | docker restart aequitas-node |
| 1649 | [NODE] Registered node operator wallet: 0x... |
| 1650 | Rewards are distributed automatically every day at 20:00 Berlin time (CEST/CET). Just keep your node running & |
| 1654 | Troubleshooting |
| 1658 | Likely cause |
| 1662 | Block height stays at 0 |
| 1663 | PRIMARY_NODE_URL not set or wrong |
| 1664 | Set PRIMARY_NODE_URL=https://aequitas.digital and redeploy. Also set SELF_URL to your own node's public URL. |
| 1667 | DATABASE_URL error on startup |
| 1668 | Wrong connection string or PostgreSQL unreachable |
| 1669 | Check format: postgres://user:pass@host:5432/dbname &mdash; make sure PostgreSQL is running and accessible |
| 1672 | "no code at address" in logs |
| 1673 | V7 contract not yet deployed in this EVM |
| 1674 | Normal on first start when RELAYER_ADDRESS is set &mdash; node auto-deploys V7. Wait a few seconds and check a |
| 1677 | "NODE_OPERATOR_WALLET not set" in logs |
| 1678 | Missing environment variable |
| 1679 | Add NODE_OPERATOR_WALLET=0xYOUR_HUMAN_WALLET to your variables. Node runs fine without it but you won't receiv |
| 1682 | Node container exits immediately |
| 1683 | Build or startup failure |
| 1684 | Run docker logs aequitas-node for the error message. Most common cause: DATABASE_URL missing or RELAYER_PRIVAT |
| 1687 | Port 8080 not reachable (Docker) |
| 1688 | Firewall or cloud provider config |
| 1689 | Open TCP port 8080 inbound in your firewall or cloud security group settings. |
| 1692 | Docker build fails with module error |
| 1693 | No internet access during build |
| 1694 | The Docker build needs outbound internet to download Go modules. Check the VPS firewall allows outbound HTTPS. |
| 1697 | ⚠ P2P bootstrap unreachable (HTTP sync still works) |
| 1698 | libp2p port 4001 firewalled (very common) |
| 1699 | ufw allow 4001/tcp |
| 1702 | Bootstrap snapshot failed / StateRoot mismatch |
| 1703 | SNAPSHOT_TOKEN not set on primary, or BOOTSTRAP_SIGNER wrong |
| 1704 | Set BOOTSTRAP_SNAPSHOT_URL=https://aequitas.digital/api/snapshot, BOOTSTRAP_SIGNER=0x92cbedec9d348b4762cb9af99 |
| 1707 | Node not in block explorer / no MERGE blocks |
| 1708 | Port 8080 not reachable from outside OR Step 5b not done |
| 1709 | ufw allow 8080/tcp |
| 1712 | MetaMask shows 0 AEQ or wrong balance after registration |
| 1713 | Stale network config in MetaMask (cached old RPC data) |
| 1714 | MetaMask → Settings → Networks → delete all "Aequitas Chain" entries → re-add via the "+ ADD AEQUITAS NETWORK" |
| 1717 | NODE_KEY generating new key on every restart |
| 1718 | NODE_KEY env var not set |
| 1719 | SAVE THIS AS NODE_KEY ENVIRONMENT VAR: &lt;base64&gt; |
| 1724 | Questions / Feedback |
| 1725 | Telegram group |
| 1725 | X (@AequitasMoney) |
| 1741 | Protocol Mechanisms |
| 1749 | Contract Code |
| 1761 | What happens to AEQ when people die or become permanently incapacitated? In Bitcoin and most cryptocurrencies, |
| 1766 | What if someone is hospitalized, incarcerated, or otherwise unable to access their device for months? The Guar |
| 1771 | Demurrage is a holding cost on money — a negative interest rate that makes hoarding expensive and circulation  |
| 1787 | Open Source Chain Logic |
| 1788 | https://aequitas.digital/rpc |
| 1797 | ◆ Consensus: GHOSTDAG + KNIGHTDAG |
| 1798 | → Network / Consensus |
| 1801 | Node Decentralization Roadmap |
| 1803 | Phase 0 (now): |
| 1804 | Phase 1 (100+ humans): |
| 1805 | Phase 2 (1,000+ humans): |
| 1806 | Phase 3 (10,000+ humans): |
| 1825 | @AequitasMoney |
| 1826 | t.me/aequitasmoney |
