# AEQUITAS — Proof of Humanity Chain

> *"Geld existiert, weil Menschen existieren. Nicht mehr, nicht weniger."*
> *"Money exists because people exist. Nothing more, nothing less."*

[![Website](https://img.shields.io/badge/Website-aequitas.digital-purple)](https://aequitas.digital)
[![Chain ID](https://img.shields.io/badge/Chain%20ID-1926-blue)](https://aequitas.digital/rpc)
[![EVM](https://img.shields.io/badge/EVM-Compatible-green)](https://aequitas.digital/rpc)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)
[![Beta](https://img.shields.io/badge/Beta-Live-gold)](https://aequitas.digital)

---

## Was ist Aequitas? / What is Aequitas?

Aequitas ist das erste Währungssystem, in dem das Geldangebot direkt und mathematisch an die Existenz verifizierten menschlichen Lebens geknüpft ist.

Aequitas is the first monetary system where the money supply is directly and mathematically tied to verified human existence.

```
Gesamtangebot / Total Supply  =  Verifizierte Menschen × 1.000 AEQ
                                  Verified Humans       × 1,000 AEQ
```

**Kein Pre-Mine. Keine Gründer-Zuteilung. Keine Investorenrunde. Kein Early-Adopter-Vorteil.**
**No pre-mine. No founder allocation. No investor round. No early-adopter advantage.**

Jede Person, die sich registriert — ob als erste oder als millionste — erhält exakt 1.000 AEQ.
Every person who registers — whether first or millionth — receives exactly 1,000 AEQ.

Der Gini-Koeffizient von Aequitas wird live gemessen (Explorer → Gleichheit) — zum Vergleich: Bitcoin liegt bei ~0,85, dem ungleichsten Währungssystem der Geschichte.
Aequitas's Gini coefficient is measured live (explorer → Equality) — for comparison, Bitcoin is at ~0.85, the most unequal monetary system in history.

---

## Live / Website

> Alle Dienste laufen auf den Contabo-Validatoren; Railway ist seit August 2026 abgeschaltet
> ([`docs/MIGRATION_RAILWAY_TO_CONTABO.md`](docs/MIGRATION_RAILWAY_TO_CONTABO.md)).

| | URL |
|---|---|
| 🌐 Website & Explorer | https://aequitas.digital |
| ⛓ RPC Endpoint | https://aequitas.digital/rpc |
| 🔒 Proof Server | Pro Validator eine eigene Instanz, konfiguriert über `PROOF_SERVER_URLS` — es gibt bewusst **keinen** eingebauten Default mehr (ein Node soll bei Fehlkonfiguration laut scheitern, statt still die Infrastruktur eines Dritten zu benutzen) |
| 📡 Bootstrap Node (Contabo1) | `/ip4/173.249.37.118/tcp/4001/p2p/12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm` |
| 📡 Bootstrap Node (Contabo2) | `/ip4/194.163.188.71/tcp/4001/p2p/12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN` |

---

## Smart Contracts (Aequitas Chain — Chain ID 1926)

| Contract | Address |
|----------|---------|
| **AequitasV7** (Main) | `0x20D271028f32577FCd07b4583A8e0E4eBBdB4F78` |
| **BioVerifier** (Groth16 ZKP) | `0xc369D27b49DE017d113Bbcb9A1884a9e745B6BE2` |
| AequitasV5 (Sepolia Legacy) | `0x4f147d5B3388AF07993CC4fC548502A78Af0B8b5` |

---

## MetaMask / Wallet Konfiguration

| Parameter | Wert / Value |
|-----------|-------------|
| Network Name | Aequitas Chain |
| RPC URL | https://aequitas.digital/rpc |
| Chain ID | **1926** |
| Symbol | AEQ |
| Decimals | 18 |
| Block Explorer | https://aequitas.digital |

---

## Kernprinzipien / Core Principles

### 1. Proof of Humanity — Nachweis der Menschlichkeit

Jeder AEQ-Halter muss nachweisen, dass er ein einzigartiger lebender Mensch ist — durch biometrische Verifikation und ein Zero-Knowledge-Proof-System.

Every AEQ holder must prove they are a unique living human through biometric verification and Zero-Knowledge Proofs.

- 📱 **Android App** → kurze Live-Gesichtsprüfung: Blinzeln, Blick zu einer zufälligen Seite, Farbblitze; nur das Gesicht, kein Fingerabdruck, kein Ausweis / short live face check: blink, glance to a random side, colour flashes; face only, no fingerprint, no ID
- 🧑‍⚖️ Zwei unabhängige Vergleichsdienste (verschiedene Eigentümer) prüfen, dass das Gesicht noch nicht registriert ist; beide müssen zustimmen / two independent matching services (different owners) check the face is not yet registered; both must agree
- 🔒 Foto und Template werden nach Sekunden gelöscht; die Vergleichsdienste behalten nur einen 64-Byte-Auszug, aus dem sich das Gesicht nicht rekonstruieren lässt / photo and template are deleted within seconds; the matching services keep only a 64-byte sketch from which the face cannot be reconstructed
- 🔐 Groth16 ZKP auf dem Proof-Server, nur gegen Wallet-Bindung + 2 signierte Bescheinigungen / Groth16 ZKP on the proof server, only against the wallet binding + 2 signed attestations
- ⚖️ Abgewiesen? Kennung `W-…` in der App, Widerspruch binnen 90 Tagen, ein Mensch prüft / Rejected? Identifier `W-…` in the app, objection within 90 days, a human reviews
- ⛓ Commitment-Hash dauerhaft on-chain gespeichert / Commitment stored permanently on-chain
- 👤 **Ein Mensch, eine Wallet, für immer / One human, one wallet, forever**
- 👁 **Langfristig: Iris-Scan.** Um wirklich 1 Mensch = 1 Registrierung zu gewährleisten, setzt Aequitas langfristig auf den Iris-Scan. Wie das umgesetzt werden kann, daran wird derzeit gearbeitet; Hardware und Zeitplan stehen noch nicht fest. Die Gesichtsprüfung ist der Zwischenschritt, mit benannten Grenzen (Schwelle noch nicht an echten Aufnahmen kalibriert). / **Long term: iris scan.** To truly guarantee one person = one registration, Aequitas will rely on the iris scan in the long run. How it can be implemented is being worked on now; hardware and timing are not decided. The face check is the interim step, with named limits (threshold not yet calibrated on real captures).

### 2. Universal Basic Income (UBI) — Universelles Grundeinkommen

UBI aus Protokoll-Ökonomie — ohne Steuern, ohne Regierung, ohne politische Entscheidung.
UBI from protocol economics — no taxation, no government, no political decision required.

**Quellen / Sources:**
- Überweisungsgebühren (0,1 % auf jede Überweisung, obendrauf; bis 30.09.2026 Aufschlag ab dem 5-/10-/20-fachen des fairen Anteils, ab 1.10.2026 ohne Aufschlag und die ersten 1.000 AEQ im Monat für Menschen frei) → 100% an UBI-Pool / Transfer fees (0.1% on every transfer, paid on top; until 30 Sep 2026 a surcharge from 5×/10×/20× the fair share, from 1 Oct 2026 no surcharge and people's first 1,000 AEQ a month free) → 100% to UBI Pool
- Swap-Gebühren → 30% an UBI-Pool / Swap fees → 30% to UBI Pool
- Wealth-Cap-Überschuss → 100% an UBI-Pool / Wealth cap overflow → 100% to UBI Pool
- Liegegeld (Demurrage) → 100% an UBI-Pool / Idle-money levy (demurrage) → 100% to UBI Pool
- Ausstiegsabgabe 2 % beim Umtausch in einen Stablecoin (ab 1.10.2026) → 100% an UBI-Pool / 2% exit levy when exchanging into a stable coin (from 1 Oct 2026) → 100% to UBI Pool
- Inaktive Wallets nach 4 Jahren / Inactive wallet escrow after 4 years

### 3. Wealth Cap — Vermögensobergrenze

Dynamische Obergrenze — kein Admin-Key, kein Governance-Vote, automatisch durch Human-Count ausgelöst:
Dynamic ceiling — no admin key, no governance vote, triggered automatically by human count:

| Phase | Menschen / Humans | Formel / Formula | Cap |
|-------|------------------|-----------------|-----|
| **0** Bootstrap | 1–99 | `max(5, min(N, 25)) × Ø-Balance` | 5×→25× (wächst mit jedem neuen Menschen / grows with each human) |
| **1** Growth | 100–9.999 | `25 × Ø-Balance` | 25× Durchschnittsbalance |
| **2** Stability | 10.000–999.999 | `25 × Ø-Balance` | 25× Durchschnittsbalance |
| **3** Maturity | 1.000.000+ | `25 × Ø-Balance` | 25× Durchschnittsbalance |

**Phase 0 Bootstrap-Mechanismus / Bootstrap mechanism:**
- 1–4 Menschen / humans: **5× Durchschnitt / 5× average**
- Jeder neue Mensch / each new human: **+1×**
- Ab 25. Mensch / from 25th human: dauerhaft **25×** (kein Governance-Vote nötig)

Überschuss fließt sofort in die Tokenomics-Pools — kein AEQ geht verloren.
Excess flows instantly into tokenomics pools — no AEQ is destroyed.

### 4. Demurrage — Haltegebühr

**Ab 1.10.2026:** Menschen zahlen 0,5 %/Monat nur auf den Teil über 5.000 AEQ. Unternehmen: bis 1,5 Monatsumsätze frei (mindestens 2.000 AEQ), darüber 0,5 %/Monat, über 3 Monatsumsätzen 2 %/Monat. Sonstige Adressen: 1 %/Monat, höchstens 1.000 AEQ. Täglich abgerechnet, 100 % ins Grundeinkommen — wird nie vernichtet. **Bis 30.09.2026** gilt die bisherige Regel: 0,5 %/Monat auf den Teil über dem fairen Anteil nach 3 Monaten ohne Aktivität. Details: [`docs/UNTERNEHMEN_KONZEPT.md`](docs/UNTERNEHMEN_KONZEPT.md).
**From 1 Oct 2026:** people pay 0.5%/month only on the part above 5,000 AEQ. Businesses: up to 1.5 months' turnover free (at least 2,000 AEQ), above that 0.5%/month, above 3 months' turnover 2%/month. Other addresses: 1%/month, at most 1,000 AEQ. Settled daily, 100% to the basic income — never destroyed. **Until 30 Sep 2026** the previous rule applies: 0.5%/month on the part above the fair share after 3 months without activity.

Historisches Vorbild: Wörgl, Österreich (1932) — Demurrage-Währung reduzierte die Arbeitslosigkeit um 25% in einem Jahr.
Historical precedent: Wörgl, Austria (1932) — demurrage currency reduced unemployment by 25% in one year.

### 5. Exchange & Liquidity Pool

Integrierter AMM-DEX (AEQ ↔ tUSD) mit automatischer Preisfindung (x·y=k Formel):
Built-in AMM DEX (AEQ ↔ tUSD) with automatic price discovery (x·y=k formula):

- 0,1% Swap-Gebühr → 40% Validatoren, 30% LPs, 30% UBI (Treasury seit 24.09.2026 0%: aus keinem Topf darf jemand auszahlen, auch nicht mit dessen Schlüssel)
- Validator-Anteil: gleicher Anteil für jeden Menschen, der einen Validator betreibt, nur gewichtet nach Minuten online — nicht nach Hardware, nicht nach Dienstalter
- Liquidity Provider Shares proportional zur Einlage
- Preishistorie und Lorenz-Kurve live on-chain

### 6. Keine algorithmische Inflation / No Algorithmic Inflation

Das **einzige** Ereignis das neues AEQ erschafft: ein neuer verifizierter Mensch registriert sich → 1.000 AEQ werden erstellt.
The **only** event that creates new AEQ: a new verified human registers → 1,000 AEQ created.

Kein Mining. Kein Staking. Keine Protokoll-Emissionen.
No mining. No staking. No protocol emissions.

---

## Architektur / Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  Android App                            │
│    Live-Gesichtsprüfung → unabhängige Vergleichsdienste │
│    2 von 2 → bio_hash + signierte Bescheinigungen       │
└──────────────────────┬──────────────────────────────────┘
                       │ biometric hash
┌──────────────────────▼──────────────────────────────────┐
│              Proof Server (Node.js)                     │
│    Groth16 ZKP Generation · Nullifier Binding           │
│    circom circuits · snarkjs · BN128 curve              │
└──────────────────────┬──────────────────────────────────┘
                       │ pA, pB, pC, pubSignals, nullifier
┌──────────────────────▼──────────────────────────────────┐
│           Aequitas Layer 1 (Go 1.24)                   │
│    Mehrere Validator-Nodes ←── libp2p + HTTP ──→         │
│    BlockDAG + GHOSTDAG Konsens · EVM Engine (go-eth)    │
│    JSON-RPC · Dual-Ledger (Go + EVM)                    │
│    PostgreSQL (je Node eigene persistente DB)           │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│           AequitasV7 Smart Contract                     │
│    BioVerifier (Groth16) · Wealth Cap · Guardian        │
│    Demurrage · UBI Pool · AMM Exchange                  │
│    Transfer Fees · Nullifier Binding                    │
└─────────────────────────────────────────────────────────┘
```

**Hinweis für Integratoren / Note for integrators:** Der Go-Ledger ist die
Quelle der Wahrheit für Guthaben, Registrierung, Demurrage und UBI — die
EVM-Schicht spiegelt diese Werte für `balanceOf`/`isHuman` live. Vier
Contract-Felder (`ubiPool`, `ubiClaimed`, `pendingGuardian`, `wardCount`)
werden dagegen bewusst nur beim Deploy/Migration gesetzt, nicht live
synchronisiert — das Go-Modell für UBI/Guardian hat kein 1:1-Äquivalent
für diese Werte (siehe `evm_storage.go`'s `syncGuardianEscrowSlotsLocked`
für die volle Begründung). Für aktuelle Werte `/api/escrow`, `/api/pool`
bzw. die Guardian-Endpunkte verwenden, nicht einen rohen `eth_call` auf
diese vier Felder. / The Go ledger is the source of truth for balances,
registration, demurrage, and UBI — the EVM layer mirrors these live for
`balanceOf`/`isHuman`. Four contract fields (`ubiPool`, `ubiClaimed`,
`pendingGuardian`, `wardCount`) are deliberately only set at deploy/
migration time, not kept live — the Go-side UBI/guardian model has no
1:1 equivalent for these values (see `evm_storage.go`'s
`syncGuardianEscrowSlotsLocked` for the full rationale). Use `/api/escrow`,
`/api/pool`, or the guardian endpoints for current values, not a raw
`eth_call` against these four fields.

---

## Technische Spezifikationen / Technical Specifications

| Parameter | Wert / Value |
|-----------|-------------|
| Sprache / Language | Go 1.24 (Chain) · Node.js (Proof Server) |
| Konsens / Consensus | BlockDAG + GHOSTDAG (Sompolinsky-Zohar) + Proof of Humanity |
| Blockzeit / Block Time | ~1 Sekunde / second |
| Chain ID | 1926 (0x786) |
| EVM | Ja / Yes — go-ethereum Engine |
| ZKP-System / ZKP System | Groth16 / snarkjs / circom |
| Kurve / Curve | BN128 (alt-bn128) |
| Bio-Hash | keccak256 |
| P2P-Protokoll / P2P Protocol | libp2p (Go) |
| State Storage | PostgreSQL |
| Startguthaben / Initial Grant | 1.000 / 1,000 AEQ |
| Transaktionsgebühr / Fee | 0,1% / 0.1% |
| Gini-Ziel / Gini Target | < 0,30 (Skandinavien-Niveau) |

---

## Repository-Struktur / Repository Structure

```
aequitas-chain/
├── cmd/aequitasd/              — Node-Binary Einstiegspunkt / Node binary entry
├── x/humanity/keeper/
│   ├── api.go                  — HTTP API Server
│   ├── api_html.go             — Web-Explorer Embed-Shim (Inhalt in assets/) / embed shim (content in assets/)
│   ├── assets/                 — Web-Explorer UI: HTML/CSS/JS (mehrsprachig / multilingual)
│   ├── block.go                — BlockDAG Konsens / Consensus
│   ├── decimal.go              — Präzisions-Arithmetik / Precision arithmetic
│   ├── evm_engine.go           — EVM-Ausführung (go-ethereum) / EVM execution
│   ├── evm_rpc.go              — JSON-RPC Handler
│   ├── evm_storage.go          — Contract-Storage (PostgreSQL)
│   ├── p2p.go                  — libp2p Networking
│   ├── register.go             — ZKP Registrierungs-Handler / Registration handler
│   ├── state.go                — Chain-State + PostgreSQL
│   └── sync_blocks.go          — Block-Synchronisierung / Block sync
├── AequitasV7.sol              — V7 Haupt-Contract / Main contract
├── BioVerifier.sol             — Groth16 ZKP Verifier
├── WHITEPAPER.md               — Whitepaper (DE + EN)
└── README.md
```

---

## Registrierungsablauf / Registration Flow

```
1. App          → Live-Gesichtsaufnahme mit Lebendigkeitsprüfung (Blinzeln, Blick zur Seite, Farbblitze)
2. Vergleich    → Zwei unabhängige Vergleichsdienste prüfen auf Duplikate (beide müssen zustimmen); Coordinator stellt bio_hash + Wallet-Bindung aus, die Dienste bescheinigen
3. App          → Proof Server erzeugt Groth16 ZKP nur gegen Wallet-Bindung + 2 Bescheinigungen
4. Proof Server → Gibt pubSignals (commitment, nullifier) zurück
5. App          → Signiert mit der Wallet in der App (kein MetaMask nötig)
6. App          → Sendet /api/register mit ZKP-Proof an den Knoten, der den Beweis ausgestellt hat
7. Node         → Verifiziert ZKP → Prüft Nullifier on-chain (Replay-Schutz)
8. Node         → Ruft AequitasV7 auf → Synchronisiert Dual-Ledger
9. Wallet       → Empfängt 1.000 AEQ · App zeigt Bestätigung
```

---

## Warum Aequitas? / Why Aequitas?

Bitcoins Gini-Koeffizient liegt über 0,85 — höher als jedes Land der Erde. Die Top 1% der Bitcoin-Adressen kontrollieren über 90% aller Bitcoin. Die Kryptowährung, die das Finanzwesen demokratisieren sollte, erschuf die extremste Vermögenskonzentration in der Menschheitsgeschichte.

Bitcoin's estimated Gini coefficient exceeds 0.85 — higher than any country on Earth. The top 1% of Bitcoin addresses control over 90% of all Bitcoin. The cryptocurrency meant to democratize finance created the most extreme wealth concentration in human history.

Aequitas gibt eine Antwort auf die Frage:
Aequitas answers the question:

> *Was wäre eine Kryptowährung, wenn sie von Grund auf fair für jeden Menschen konzipiert worden wäre?*
> *What would a cryptocurrency look like if designed from first principles to be fair to every human being?*

Die Antwort ist einfach: **Geld existiert, weil Menschen existieren. Jede Person sollte daher einen gleichen Anteil am Geld haben — allein weil sie ein Mensch ist.**

The answer is simple: **Money exists because people exist. Therefore, every person should have an equal share of money simply by virtue of being human.**

---

## Roadmap

| Phase | Status | Beschreibung / Description |
|-------|--------|---------------------------|
| 0 | ✅ | Smart Contracts · ZKP · Android App · Proof Server |
| 0+ | ✅ | Aequitas Layer 1 (Go) · BlockDAG + GHOSTDAG · P2P · Explorer |
| V7 | ✅ | EVM · Dual-Ledger · Exchange/AMM · Lorenz-Kurve · Gini-Index · UBI · Demurrage |
| V7.x | ✅ | Proof of Alive · Guardian-System (Eskrow + UBI-Freigabe) live |
| 1 | 🔄 | APK-Release · Live-Gesichtsprüfung · Community-Wachstum · Grant-Anträge · Mehr-Knoten-Skalierung |
| 1.10.2026 | ⏳ | Wirtschaftsregeln: drei Kontoarten, Liegegeld, Ausstiegsabgabe, freie Monatsbeträge / economy rules |
| Iris | 🔄 | Iris-Scan für wirklich 1 Mensch = 1 Registrierung — Umsetzung in Arbeit, kein Datum / iris scan — implementation in progress, no date |
| 2 | ⬜ | iOS App |
| 3 | ⬜ | Cross-Chain Bridges · Externe DEX-Integration |
| 4 | ⬜ | Vollständige Dezentralisierung · Community Governance |

---

## Links

- 🌐 [Website & Explorer](https://aequitas.digital)
- 📄 [Whitepaper](WHITEPAPER.md)
- 💻 [GitHub](https://github.com/hanoi96international-gif/Aequitas)
- 𝕏 [X / Twitter — @AequitasMoney](https://x.com/AequitasMoney)
- 💬 [Telegram-Gruppe / Telegram group](https://t.me/aequitasmoney)
- 🔍 [V5 Sepolia (Legacy)](https://sepolia.etherscan.io/address/0x4f147d5B3388AF07993CC4fC548502A78Af0B8b5)

---

*Aequitas — gestartet Juni 2026 · Beta · Chain ID 1926*
*Aequitas — launched June 2026 · Beta · Chain ID 1926*
