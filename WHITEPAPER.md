# AEQUITAS WHITEPAPER v2.0

**Proof of Humanity Chain — Eine faire Währung für alle Menschen**
**Proof of Humanity Chain — A Fair Currency for All of Humanity**

*Version 2.0 · Juni / June 2026*
*Chain ID 1926 · aequitas.digital*

---

## Inhalt / Table of Contents

1. [Das Problem / The Problem](#1-das-problem--the-problem)
2. [Die Vision / The Vision](#2-die-vision--the-vision)
3. [Proof of Humanity](#3-proof-of-humanity)
4. [Tokenomics & Wirtschaftsmodell / Economic Model](#4-tokenomics--wirtschaftsmodell--economic-model)
5. [Technische Architektur / Technical Architecture](#5-technische-architektur--technical-architecture)
6. [Smart Contract V7](#6-smart-contract-v7)
7. [Zero-Knowledge-Proofs & Privatsphäre / Privacy](#7-zero-knowledge-proofs--privatsphäre--privacy)
8. [Gleichheitsindex / Equality Index](#8-gleichheitsindex--equality-index)
9. [Exchange & Liquiditätspool / Liquidity Pool](#9-exchange--liquiditätspool--liquidity-pool)
10. [Sicherheit / Security](#10-sicherheit--security)
11. [Roadmap](#11-roadmap)
12. [Fazit / Conclusion](#12-fazit--conclusion)

---

## 1. Das Problem / The Problem

### DE
Bitcoin hat einen Gini-Koeffizienten von über 0,85 — höher als jedes Land der Erde. Die Top 1% der Adressen kontrollieren mehr als 90% aller Bitcoin. Was als dezentrales, demokratisches Geld begann, hat die extremste Vermögenskonzentration der Finanzgeschichte erschaffen.

Das ist kein Versagen der Blockchain-Technologie. Es ist ein Versagen des Designs: Bitcoin wurde ohne Rücksicht auf die Frage entworfen, wer Zugang zu initialem Kapital hat. Wer früh dabei war oder über Rechenleistung verfügte, gewann. Wer später kam oder arm war, verlor.

Das gleiche Muster wiederholt sich bei allen PoW- und PoS-Kryptowährungen: Die Reichen werden reicher, weil sie mehr Kapital einsetzen können. Das ist kein Bug — es ist die Systemarchitektur.

### EN
Bitcoin has a Gini coefficient exceeding 0.85 — higher than any country on Earth. The top 1% of addresses control more than 90% of all Bitcoin. What started as decentralized, democratic money created the most extreme wealth concentration in financial history.

This is not a failure of blockchain technology. It is a failure of design: Bitcoin was built without considering who has access to initial capital. Those who were early or had computing power won. Those who came later or were poor lost.

The same pattern repeats across all PoW and PoS cryptocurrencies: the rich get richer because they can deploy more capital. This is not a bug — it is the system architecture.

---

## 2. Die Vision / The Vision

### DE
Aequitas stellt eine radikale Frage: **Was wäre eine Kryptowährung, wenn sie von Grund auf fair für jeden Menschen auf der Erde konzipiert worden wäre?**

Die Antwort ist überraschend einfach:

> *Geld existiert, weil Menschen existieren. Daher sollte jede Person einen gleichen Anteil am Geld haben — allein weil sie ein Mensch ist.*

Aequitas setzt dieses Prinzip mathematisch um:

```
Gesamtangebot = Verifizierte Menschen × 1.000 AEQ
```

Kein Pre-Mine. Keine Gründer-Zuteilung. Kein früher Vorteil. Wer sich heute registriert und wer sich in zehn Jahren registriert, erhält exakt dasselbe. Es ist kein politisches Versprechen — es ist Code.

### EN
Aequitas asks a radical question: **What would a cryptocurrency look like if designed from first principles to be fair to every human being on Earth?**

The answer is surprisingly simple:

> *Money exists because people exist. Therefore, every person should have an equal share of money simply by virtue of being human.*

Aequitas implements this principle mathematically:

```
Total Supply = Verified Humans × 1,000 AEQ
```

No pre-mine. No founder allocation. No early-adopter advantage. Someone registering today and someone registering in ten years receive exactly the same. This is not a political promise — it is code.

---

## 3. Proof of Humanity

### DE
Das zentrale Problem eines auf menschlicher Existenz basierenden Währungssystems ist die Verifikation: Wie beweist man, dass eine Adresse einem echten, einzigartigen Menschen gehört — ohne persönliche Daten zu speichern?

Aequitas löst dies mit biometrischer Verifikation und Zero-Knowledge-Proofs:

**Registrierungsablauf:**
1. Die Android-App ermittelt eine Identitätsquelle. Ist der Biometrie-Modus aktiv, ist das eine Kamera-Aufnahme von Gesicht und Handfläche, die von mehreren unabhängigen Vergleichsdiensten per Quorum gegen alle bestehenden Anmeldungen geprüft wird; ist er inaktiv (Standard zum Start), ist es ein zufälliges, gerätegebundenes Geheimnis. Siehe §3.2.
2. Daraus wird ein deterministischer Hash abgeleitet — die Rohdaten verlassen das Gerät **niemals**
3. Der Hash wird an den Proof-Server gesendet
4. Der Proof-Server generiert einen **Groth16 Zero-Knowledge-Proof** (Groth16/BN128-Kurve)
5. Der Proof enthält `commitment` (Bindung an die Identitätsquelle) und `nullifier` (Replay-Schutz)
6. Die Blockchain verifiziert den Proof on-chain via BioVerifier-Contract
7. Bei Erfolg: 1.000 AEQ werden der Wallet gutgeschrieben

**Garantien:**
- Dieselbe Identitätsquelle kann sich nur einmal registrieren (Nullifier-Bindung). Ob diese Quelle ein Mensch oder ein Gerät ist, entscheidet der Modus aus Schritt 1 — siehe §3.2
- Kein persönliches Datum wird gespeichert
- Verifikation ist dauerhaft und unveränderbar
- Kein Dritter (auch nicht Aequitas) kann eine Registrierung rückgängig machen

### EN
The central problem of a monetary system based on human existence is verification: how do you prove that an address belongs to a real, unique human — without storing personal data?

Aequitas solves this with biometric verification and Zero-Knowledge Proofs:

**Registration Flow:**
1. The Android app establishes an identity source. With biometric mode active this is a camera capture of face and palm, checked by quorum across several independent matching services against every existing enrolment; with it inactive (the default at launch) it is a random, device-bound secret. See §3.2.
2. A deterministic hash is derived from it — raw data **never** leaves the device
3. The hash is sent to the Proof Server
4. The Proof Server generates a **Groth16 Zero-Knowledge Proof** (Groth16/BN128 curve)
5. The proof contains `commitment` (binding to the identity source) and `nullifier` (replay protection)
6. The blockchain verifies the proof on-chain via BioVerifier contract
7. On success: 1,000 AEQ credited to the wallet

**Guarantees:**
- The same identity source can only register once (nullifier binding). Whether that source is a human or a device is decided by the mode in step 1 — see §3.2
- No personal data is stored
- Verification is permanent and immutable
- No third party (not even Aequitas) can reverse a registration

---

### 3.1 Biometrisches 3-Faktor-System / 3-Factor Biometric System

#### DE
Langfristig soll biologische Einzigartigkeit vollständig geräteunabhängig nachgewiesen werden. Die folgenden Phasen beschreiben diesen Weg. **Keine davon ist zum Start am 18.08.2026 aktiv** — was tatsächlich ausgeliefert wird, steht direkt darunter unter „Was zum Start läuft".

**Phase 1 — Alle 10 Fingerabdrücke + Lebenderkennung** *(Referenzdesign, nicht ausgeliefert)*
- **R503 optischer Fingerabdruckscanner** (GROW, UART-Interface): Alle 10 Finger würden gescannt und zu einem einzigen biometrischen Hash kombiniert.
- **MAX30102 PPG-Sensor**: Photoplethysmographie-Signal (Herzfrequenz via IR/Rot-LED) als Lebendnachweis gegen Replay-Attacken mit gespeicherten Abdrücken oder Gipsabgüssen.

Dieses Hardware-Kit existiert als Entwurf. Es gibt kein Gerät zu kaufen, und die ausgelieferte App spricht mit keiner solchen Hardware. Die Zahlen unten sind theoretische Eigenschaften der Sensorik, keine gemessenen Eigenschaften des laufenden Systems.

| Eigenschaft (theoretisch) | Wert |
|------------|------|
| Einzigartigkeit Fingerabdruck (einzeln) | 1 von 10⁹ |
| Alle 10 Finger kombiniert | 1 von 10⁹⁰ (theoretisch) |
| Liveness-Nachweis | PPG-Pulssignal (MAX30102) |

**Phase 2 — Handvenen-Muster** *(geplant)*
- **ESP32-CAM + IR-LED (850 nm)**: Infrarot-Durchleuchtung der Hand erzeugt ein eindeutiges Venenmuster aus dem Inneren des Körpers — nicht kopierbar, nicht hinterlegbar, unveränderlich über das gesamte Leben.
- Das Venenmuster wird als zweiter biometrischer Hash `vein_hash` in das ZK-Commitment einbezogen.

| Eigenschaft | Wert |
|------------|------|
| Einzigartigkeit Handvenen | 1 von 10⁷ |
| Auch für eineiige Zwillinge unterschiedlich | ✅ |
| Unveränderlich über das Leben | ✅ |
| Unkopierbares Merkmal (innen) | ✅ |

**Phase 3 — Iris** *(geplant)*
- **IR-Iris-Modul**: Die menschliche Iris ist der Goldstandard biometrischer Einzigartigkeit — 240+ unabhängige Freiheitsgrade, Kollisionswahrscheinlichkeit 1 von 10⁷⁸. Absolut verschieden bei eineiigen Zwillingen, unveränderlich von Geburt an.
- Der Iris-Hash wird in den Nullifier einbezogen — damit ist die Identität rein körpergebunden, nicht gerätegebunden.

| Eigenschaft | Wert |
|------------|------|
| Einzigartigkeit Iris | 1 von 10⁷⁸ |
| Eineiige Zwillinge: identisch? | ❌ (absolut verschieden) |
| Gerätunabhängig | ✅ |
| Falsch-Positiv-Rate (globaler Vergleich) | < 10⁻⁷⁸ |

#### EN
The long-term goal is to prove biological uniqueness fully independently of the device. The phases below describe that path. **None of them is active at the 2026-08-18 launch** — what actually ships is stated directly below, under "What runs at launch".

**Phase 1 — All 10 Fingerprints + Liveness** *(reference design, not shipped)*
- **R503 optical fingerprint scanner** (GROW, UART interface): all 10 fingers would be scanned and combined into a single biometric hash.
- **MAX30102 PPG sensor**: photoplethysmography signal (heart rate via IR/red LED) as a liveness proof against replay attacks using stored fingerprint images or plaster casts.

This hardware kit exists as a design. There is no device to buy, and the shipped app talks to no such hardware. The figures below are theoretical properties of the sensors, not measured properties of the running system.

| Property (theoretical) | Value |
|----------|-------|
| Single fingerprint uniqueness | 1 in 10⁹ |
| All 10 fingers combined | 1 in 10⁹⁰ (theoretical) |
| Liveness proof | PPG pulse signal (MAX30102) |

**Phase 2 — Hand Vein Pattern** *(planned)*
- **ESP32-CAM + 850 nm IR LED**: Infrared illumination of the hand produces a unique vein pattern from inside the body — uncopyable, unstorable, immutable over a lifetime.
- The vein pattern is added as a second biometric hash `vein_hash` to the ZK commitment.

| Property | Value |
|----------|-------|
| Hand vein uniqueness | 1 in 10⁷ |
| Different for identical twins | ✅ |
| Immutable over lifetime | ✅ |
| Uncopyable (internal feature) | ✅ |

**Phase 3 — Iris** *(planned)*
- **IR iris module**: The human iris is the gold standard of biometric uniqueness — 240+ independent degrees of freedom, collision probability 1 in 10⁷⁸. Completely different even in identical twins, immutable from birth.
- The iris hash is incorporated into the nullifier — making identity purely body-bound, not device-bound.

| Property | Value |
|----------|-------|
| Iris uniqueness | 1 in 10⁷⁸ |
| Identical twins: same? | ❌ (absolutely different) |
| Device-independent | ✅ |
| False-match rate (global comparison) | < 10⁻⁷⁸ |

---

### 3.2 Was zum Start läuft / What runs at launch

#### DE
Dieser Abschnitt beschreibt den Stand am 18.08.2026. Er hat Vorrang vor jeder Beschreibung oben, wenn beide sich widersprechen.

Es gibt **keine Spezial-Hardware**. Die Registrierung läuft über die Android-App und die Kamera des Telefons. Zwei Betriebsarten:

**a) Biometrie aktiviert** (Koordinator erreichbar): Die App nimmt Gesicht und Handfläche mit der Telefonkamera auf. Mehrere unabhängige Vergleichsdienste prüfen die Aufnahme gegen die bestehenden Anmeldungen und müssen mehrheitlich (M von N) zustimmen, bevor ein `bio_hash` ausgestellt wird. Dieser Hash geht in den Nullifier des ZK-Beweises ein, und die Kette lehnt jeden bereits benutzten Nullifier ab.

**b) Biometrie deaktiviert** (Standard, wenn kein Koordinator konfiguriert ist): Die App leitet die Identität aus einem **zufälligen, gerätegebundenen Geheimnis** ab. Das ist ausdrücklich *keine* Biometrie. In dieser Betriebsart bindet die Anmeldung an ein **Gerät**, nicht an einen Menschen: dieselbe Person kann sich auf einem zweiten Telefon erneut anmelden und ein zweites Mal 1.000 AEQ erhalten.

**Was die Einmaligkeit zum Start wirklich trägt:** der Nullifier auf der Kette. Er ist kryptografisch und lückenlos — ein zweites Mal derselbe Nullifier wird abgelehnt, egal über welchen Weg er eingereicht wird. Er beweist aber nur, dass *dieselbe Identitätsquelle* nicht zweimal zählt. Ob diese Quelle ein Mensch oder ein Gerät ist, entscheidet die Betriebsart oben.

**Und wie viele Quellen jemand haben kann, entscheidet zurzeit nichts.** Der Nullifier ist `Poseidon(bioHash)`, und `bioHash` ist ein privater Eingabewert, den der Beweis-Circuit nicht prüft. Der Proof-Server hat dafür ein Tor — `BIO_ATTESTATION_MODE` —, das nur eine vom Koordinator signierte Bestätigung durchlässt. Dieses Tor steht auf `off`, dem Auslieferungszustand, auf beiden Validatoren (gemessen 19.08.2026), und es kann derzeit gar nicht geschlossen werden: einen Koordinator gibt es noch nicht. Wer `/prove` erreicht, darf also eine beliebige Zahl als `bioHash` einreichen und erhält dafür einen gültigen, neuen Nullifier.

Praktisch heißt das: die Schranke zum Start ist **nicht** „ein Gerät, eine Anmeldung", sondern „eine frei gewählte Zahl, eine Anmeldung" — mit Ratenbegrenzung pro IP und pro Wallet als einziger Bremse. Solange das so ist, ist „verifizierter Mensch" in §4.1 eine Bezeichnung für einen Registrierungsvorgang, keine Aussage über einen Menschen, und die Geldmenge ist keine Funktion menschlicher Existenz, sondern eine Funktion der Anzahl abgegebener Registrierungen.

Zu schließen ist das in dieser Reihenfolge: Koordinator bauen → `BIO_ATTESTATION_MODE=optional` mit ausgelieferten Schlüsseln → App-Build, der Bestätigungen holt → `required`. Der Zwischenschritt ist nicht optional: direkt auf `required` sperrt jede bereits installierte App aus.

#### EN
This section describes the state on 2026-08-18. Where it contradicts anything above, this section is correct.

There is **no special hardware**. Registration runs through the Android app and the phone's own camera. Two modes:

**a) Biometrics enabled** (coordinator reachable): the app captures face and palm with the phone camera. Several independent matching services compare the capture against existing enrolments and must agree by quorum (M of N) before a `bio_hash` is issued. That hash goes into the ZK proof's nullifier, and the chain rejects any nullifier it has already seen.

**b) Biometrics disabled** (the default when no coordinator is configured): the app derives identity from a **random, device-bound secret**. This is explicitly *not* biometrics. In this mode registration binds to a **device**, not to a person: the same human can register again on a second phone and receive a second 1,000 AEQ grant.

**What actually carries uniqueness at launch:** the on-chain nullifier. It is cryptographic and airtight — the same nullifier is refused the second time, whatever path submits it. But it only proves that *the same identity source* cannot count twice. Whether that source is a human or a device is decided by the mode above.

**And how many sources one person may hold is, at present, decided by nothing.** The nullifier is `Poseidon(bioHash)`, and `bioHash` is a private input the proving circuit does not constrain. The proof server has a gate for exactly this — `BIO_ATTESTATION_MODE` — which admits only a coordinator-signed attestation. That gate is `off`, its shipped default, on both validators (measured 2026-08-19), and it cannot currently be closed: no coordinator exists yet. Anyone who can reach `/prove` may therefore submit any number as `bioHash` and receive a valid, fresh nullifier for it.

In practice the bound at launch is **not** "one device, one registration" but "one freely chosen number, one registration" — with per-IP and per-wallet rate limits as the only brake. While that holds, "verified human" in §4.1 names a registration event rather than stating anything about a person, and the supply is not a function of human existence but a function of how many registrations were submitted.

Closing it has a required order: build the coordinator → `BIO_ATTESTATION_MODE=optional` with keys deployed → ship an app build that obtains attestations → `required`. The intermediate step is not a nicety: going straight to `required` locks out every already-installed app.

---

## 4. Tokenomics & Wirtschaftsmodell / Economic Model

### 4.1 Geldangebot / Money Supply

Das Angebot ist eine mathematische Funktion menschlicher Existenz. Es gibt genau **eine** Möglichkeit neues AEQ zu erschaffen: ein neuer verifizierter Mensch registriert sich.

The supply is a mathematical function of human existence. There is exactly **one** way to create new AEQ: a new verified human registers.

| Ereignis / Event | AEQ-Änderung / AEQ Change |
|-----------------|--------------------------|
| Neue Registrierung / New Registration | +1.000 AEQ |
| Transfer | ±0 (nur Umverteilung / redistribution only) |
| Swap | ±0 |
| Demurrage | −x von Überschuss / from excess → UBI Pool |
| Wealth Cap Overflow | −x → sofortige Gleichverteilung / instant redistribution |

### 4.2 Universal Basic Income (UBI)

UBI wird täglich aus den Protokoll-Einnahmen verteilt — kein Staat, keine Steuer, kein Beschluss erforderlich.

UBI is distributed daily from protocol revenue — no state, no tax, no vote required.

**UBI-Pool-Quellen / UBI Pool Sources:**
- 20% aller Transaktionsgebühren (0,1% × 20% = 0,02% pro Transfer)
- Wealth-Cap-Überläufe (sofortige Gleichverteilung)
- 20% der Demurrage auf Überschussguthaben (0,5%/Monat nach 3 Monaten Inaktivität über fairShare — die übrigen 80% fließen an Validatoren/LP/Treasury, siehe unten)
- Inaktive Wallets: nach 2,5 Jahren Inaktivität → Escrow, nach weiteren 1,5 Jahren → UBI Pool

### 4.3 Demurrage — Haltegebühr

**Philosophie:** Geld ist ein Werkzeug, kein Selbstzweck. Horten von Geld über dem fairen Anteil kostet etwas — genau wie das Mieten eines Parkplatzes.

**Philosophy:** Money is a tool, not an end in itself. Hoarding money above the fair share costs something — just like renting a parking space.

```
Haltegebühr = (Guthaben − fairShare) × 0,5%/Monat × Halte-Monate (erst nach 3 Monaten Inaktivität)
Demurrage   = (Balance − fairShare) × 0.5%/month × HoldingMonths (only after a 3-month inactivity grace period)
```

Die Gebühr verteilt sich auf die vier Tokenomics-Pools (40% Validatoren / 30% LP / 20% UBI / 10% Treasury). Kein AEQ wird vernichtet.
The fee is split across the four tokenomics pools (40% validators / 30% LPs / 20% UBI / 10% treasury). No AEQ is destroyed.

### 4.4 Wealth Cap — Vermögensobergrenze

### DE
Die Wealth Cap verwendet einen Bootstrap-Multiplikator in Phase 0: `max(5, min(N, 25)) × Fair Share (1.000 AEQ)`. Kein Admin-Key, kein Governance-Vote — alle Übergänge erfolgen automatisch durch die Anzahl registrierter Menschen.

**Formel Phase 0:** `cap = max(5, min(N, 25)) × 1.000 AEQ`

> **Klarstellung (Audit 2026-08-18).** Frühere Fassungen schrieben hier
> „× Ø-Balance" bzw. „× average balance" und legten damit nahe, die Obergrenze
> wachse mit dem tatsächlich gemessenen Durchschnittsvermögen mit. Sie tut es
> nicht. Der Bezugswert ist der *Fair Share*, und der ist auf dieser Chain per
> Definition konstant: `TotalSupply / Menschen = (Menschen × 1.000) / Menschen
> = 1.000 AEQ`. Die Obergrenze liegt damit dauerhaft bei 25.000 AEQ. Die Regel
> selbst ist unverändert — nur ihre Beschreibung war irreführend.
> / *Clarification: earlier versions wrote "× average balance", implying the cap
> tracks measured average wealth. It does not — the reference is the fair share,
> which is constant by definition (TotalSupply / humans = 1,000 AEQ), so the cap
> is a permanent 25,000 AEQ. The rule is unchanged; only its description was
> misleading.*
- Menschen 1–4: Multiplikator = **5×**
- Jeder neue Mensch: Multiplikator +1×
- Ab dem 25. Menschen: dauerhaft **25×** (unveränderbar, kein Vote erforderlich)

### EN
The wealth cap uses a bootstrap multiplier during Phase 0: `max(5, min(N, 25)) × fair share (1,000 AEQ)`. No admin key, no governance vote — all transitions trigger automatically by human count.

**Phase 0 formula:** `cap = max(5, min(N, 25)) × avg_balance`
- Humans 1–4: multiplier = **5×**
- Each new human: multiplier +1×
- At 25+ humans: permanently **25×** (immutable, no vote required)

---

| Phase | Menschen / Humans | Formel / Formula | Status |
|-------|------------------|-----------------|--------|
| **0** Bootstrap | 1–99 | `max(5, min(N,25)) × Ø` | ● Aktiv / Active |
| **1** Growth | 100–9.999 | `25 × 1.000 AEQ = 25.000 AEQ` | ○ Geplant / Planned |
| **2** Stability | 10.000–999.999 | `25 × 1.000 AEQ = 25.000 AEQ` | ○ Geplant / Planned |
| **3** Maturity | 1.000.000+ | `25 × 1.000 AEQ = 25.000 AEQ` | ○ Geplant / Planned |

**Beispiel / Example (Phase 0, N=10 Menschen, Ø=1.000 AEQ):**
```
cap = max(5, min(10, 25)) × 1.000 = 10 × 1.000 = 10.000 AEQ
```

Überschuss fließt sofort in die Tokenomics-Pools — kein AEQ wird vernichtet.
Excess flows instantly into tokenomics pools — no AEQ is destroyed.

### 4.5 Transaktionsgebühren / Transaction Fees

| Empfänger / Recipient | Anteil / Share |
|----------------------|---------------|
| Validators | 40% |
| Liquidity Providers | 30% |
| UBI Pool | 20% |
| Treasury | 10% |

---

## 5. Technische Architektur / Technical Architecture

### 5.1 Layer 1 — Aequitas Chain

Aequitas läuft auf einer eigens entwickelten Layer-1-Blockchain, geschrieben in **Go 1.24**, mit einem hybriden BlockDAG-Konsensus.

Aequitas runs on a custom-built Layer 1 blockchain written in **Go 1.24**, with a hybrid BlockDAG consensus.

**BlockDAG:**
- Mehrere Blöcke können gleichzeitig von verschiedenen Nodes produziert werden
- Blöcke werden später in Merge-Blöcke zusammengeführt (mehrere Eltern)
- Höherer Durchsatz, niedrigere Latenz, bessere Fehlertoleranz
- Multiple blocks can be produced simultaneously by different nodes
- Blocks are merged into merge blocks with multiple parents
- Higher throughput, lower latency, better fault tolerance

**GHOSTDAG (Sompolinsky-Zohar, 2018):**
Damit alle Nodes trotz gleichzeitiger Blockproduktion zur selben Reihenfolge und zum selben Zustand konvergieren, berechnet jeder Node einen deterministischen "Blue Score" für jeden Block (über Selected-Parent-Auswahl und Blue/Red-Klassifizierung im Merge-Set). Daraus ergibt sich eine kanonische Gesamtordnung (Höhe, dann Blue Score, dann Hash), die auf jedem Node identisch ist — unabhängig davon, in welcher Reihenfolge Blöcke per P2P/HTTP eintreffen.

To ensure every node converges on the same order and state despite concurrent block production, each node computes a deterministic "blue score" for every block (via selected-parent selection and blue/red classification within the merge set). This yields a canonical total order (height, then blue score, then hash) that is identical on every node, regardless of the order blocks arrive via P2P/HTTP.

**KNIGHTDAG (adaptives K, inspiriert von DAGKNIGHT, Sompolinsky-Sutton 2022):**
Die Blue/Red-Klassifizierung verwendet kein starres K mehr: Für jeden Block wird deterministisch das kleinste K_eff ≤ K ermittelt, dessen Blue-Menge eine strikte Mehrheit des Merge-Sets abdeckt. Bei guter Netzwerkkonvergenz sinkt K_eff automatisch (engere, schnellere Bestätigung); bei Bursts fällt die Klassifizierung exakt auf das bisherige GHOSTDAG-Verhalten mit dem Epochen-K zurück. Da jeder Node dieselbe Inferenz über denselben Blockgraphen ausführt, bleibt die kanonische Ordnung netzweit identisch. KNIGHTDAG ist ab Blockhöhe **1.520.000** aktiv (`KNIGHTDAG_ACTIVATION_HEIGHT`, auf allen Nodes identisch konfiguriert) — darunter klassifiziert jeder Node bit-genau nach der klassischen GHOSTDAG-Regel, damit ein Node, der historische Blöcke neu ableitet (Resync, Deepscan), exakt denselben BlueScore reproduziert, den das Netzwerk für diese Blöcke bereits vereinbart hat.

**KNIGHTDAG (adaptive K, inspired by DAGKNIGHT, Sompolinsky-Sutton 2022):**
Blue/red classification no longer uses a rigid K: for every block, each node deterministically infers the smallest K_eff ≤ K whose blue set covers a strict majority of the merge set. Under good network convergence K_eff drops automatically (tighter, faster confirmation); under bursts, classification falls back to exactly the previous GHOSTDAG behavior with the epoch K. Since every node runs the same inference over the same block graph, the canonical order remains identical network-wide. KNIGHTDAG is active from block height **1,520,000** onward (`KNIGHTDAG_ACTIVATION_HEIGHT`, configured identically on every node) — below it, every node classifies bit-for-bit with the classic GHOSTDAG rule, so a node re-deriving historical state (resync, deepscan) reproduces exactly the blue_score the network already agreed on for those blocks.

**Dual-Ledger:**
Aequitas führt zwei synchronisierte Ledger parallel:
- **Go-Ledger**: PostgreSQL-gesichert, primäre Wahrheit für Salden und Menschen
- **EVM-Ledger**: go-ethereum Engine, kompatibel mit MetaMask und Web3

Aequitas maintains two synchronized ledgers in parallel:
- **Go-Ledger**: PostgreSQL-backed, primary truth for balances and humans
- **EVM-Ledger**: go-ethereum engine, compatible with MetaMask and Web3

### 5.2 Netzwerk-Topologie / Network Topology

```
Node 1 (Railway, Berlin)          Node 2 (Railway/VPS)
├── Primärer API-Server           ├── Sekundärer API-Server
├── Block-Produzent               ├── Block-Produzent
├── UBI-Verteilung (täglich)      ├── P2P-Peer
├── P2P Bootstrap-Node            └── HTTP Block-Sync
└── Geteilter PostgreSQL State ───────────────────────────┘
```

### 5.3 Technische Kenndaten / Technical Specifications

| Parameter | Wert / Value |
|-----------|-------------|
| Programmiersprache / Language | Go 1.24 |
| Konsens / Consensus | BlockDAG + Proof of Humanity |
| Blockzeit / Block Time | ~1 Sekunde / second |
| Chain ID | 1926 |
| EVM-Kompatibilität / EVM Compat. | Vollständig / Full (go-ethereum) |
| P2P-Protokoll / P2P Protocol | libp2p |
| State-Storage / State Storage | PostgreSQL (persistent) |
| ZKP-System / ZKP System | Groth16 / snarkjs / circom |
| Elliptische Kurve / Elliptic Curve | BN128 (alt-bn128) |
| Bio-Hash | keccak256 |
| Dezimalgenauigkeit / Precision | 6 Stellen / decimal places (1 AEQ = 1.000.000 Micro-AEQ) |

---

## 6. Smart Contract V7

### DE
Der AequitasV7-Contract ist das Herzstück des Protokolls. Er ist in Solidity geschrieben, auf der Aequitas Chain deployed und enthält die gesamte Wirtschaftslogik.

**Kernfunktionen:**
- `register()` — Registrierung mit ZKP-Beweis (direkt)
- `registerWithSig()` — Registrierung via Relayer (gaslos für den Nutzer)
- `transfer()` — Token-Transfer mit automatischer Demurrage und Gebühren
- `claimUBI()` — Tägliches UBI einfordern
- `addGuardian()` — Guardian-System für Proof of Alive
- `applyWealthCap()` — Vermögensobergrenze durchsetzen

**Sicherheitsfeatures:**
- Nullifier-Bindung verhindert Doppel-Registrierung
- `registerWithSig` nur von authorisierter Relayer-Adresse aufrufbar
- Optimistic Locking für Multi-Node-Schreibvorgänge
- Vollständiger Storage-Backup vor Contract-Upgrades

### EN
The AequitasV7 contract is the core of the protocol. Written in Solidity, deployed on Aequitas Chain, it contains all economic logic.

**Core Functions:**
- `register()` — Registration with ZKP proof (direct)
- `registerWithSig()` — Registration via relayer (gasless for user)
- `transfer()` — Token transfer with automatic demurrage and fees
- `claimUBI()` — Claim daily UBI
- `addGuardian()` — Guardian system for Proof of Alive
- `applyWealthCap()` — Enforce wealth ceiling

**Security Features:**
- Nullifier binding prevents double registration
- `registerWithSig` callable only from authorized relayer address
- Optimistic locking for multi-node writes
- Full storage backup before contract upgrades

---

## 7. Zero-Knowledge-Proofs & Privatsphäre / Privacy

### DE
Aequitas nutzt Groth16-Proofs auf der BN128-Kurve — eines der effizientesten ZKP-Systeme mit kleinen Proofs (~200 Bytes) und schneller On-Chain-Verifikation (~10ms).

**Nullifier-Bindung:** Der ZKP enthält einen eindeutigen Nullifier (`pubSignals[1]`), der kryptographisch an den biometrischen Hash gebunden ist. Derselbe Mensch kann denselben Nullifier nie zweimal verwenden — Sybil-Attacken sind mathematisch ausgeschlossen.

**Multi-Faktor ZK-Commitment (Phase 3 Zielarchitektur):**
```
fingers_hash = keccak256(f₁ ‖ f₂ ‖ … ‖ f₁₀)   -- alle 10 Fingerabdrücke
commitment   = keccak256(iris_hash ‖ vein_hash ‖ fingers_hash ‖ wallet_address)
nullifier    = keccak256(iris_hash ‖ vein_hash ‖ domain_separator)
```

Der Nullifier ist ausschließlich an physische Körpermerkmale gebunden — kein Gerät, keine SIM-Karte, kein Betriebssystem. Eine Person, die ihr Telefon verliert, kann sich mit denselben biometrischen Merkmalen (Iris + Handvenen) neu verifizieren, ohne eine zweite Identität zu erzeugen.

| Phase | Commitment-Faktoren | Nullifier-Faktoren |
|-------|--------------------|--------------------|
| 1 (aktiv) | fingers_hash + wallet | fingers_hash + domain |
| 2 (geplant) | vein_hash + fingers_hash + wallet | vein_hash + fingers_hash + domain |
| 3 (geplant) | iris_hash + vein_hash + fingers_hash + wallet | iris_hash + vein_hash + domain |

**Was gespeichert wird:**
- ✅ `commitment` — kryptographischer Hash (nicht rückführbar auf Biometrie)
- ✅ `nullifier` — eindeutiger Einmal-Nachweis
- ✅ Wallet-Adresse
- ❌ Fingerabdruck-Daten — niemals
- ❌ Venen- oder Iris-Muster — niemals
- ❌ Name, Adresse, ID — niemals
- ❌ IP-Adresse — nicht gespeichert

### EN
Aequitas uses Groth16 proofs on the BN128 curve — one of the most efficient ZKP systems with small proofs (~200 bytes) and fast on-chain verification (~10ms).

**Nullifier Binding:** The ZKP contains a unique nullifier (`pubSignals[1]`), cryptographically bound to the biometric hash. The same human can never use the same nullifier twice — Sybil attacks are mathematically impossible.

**Multi-Factor ZK Commitment (Phase 3 target architecture):**
```
fingers_hash = keccak256(f₁ ‖ f₂ ‖ … ‖ f₁₀)   -- all 10 fingerprints
commitment   = keccak256(iris_hash ‖ vein_hash ‖ fingers_hash ‖ wallet_address)
nullifier    = keccak256(iris_hash ‖ vein_hash ‖ domain_separator)
```

The nullifier is bound exclusively to physical body features — no device, no SIM card, no OS. A person who loses their phone can re-verify with the same biometric traits (iris + hand veins) without creating a second identity.

| Phase | Commitment factors | Nullifier factors |
|-------|--------------------|-------------------|
| 1 (active) | fingers_hash + wallet | fingers_hash + domain |
| 2 (planned) | vein_hash + fingers_hash + wallet | vein_hash + fingers_hash + domain |
| 3 (planned) | iris_hash + vein_hash + fingers_hash + wallet | iris_hash + vein_hash + domain |

**What is stored:**
- ✅ `commitment` — cryptographic hash (not traceable to biometrics)
- ✅ `nullifier` — unique one-time proof
- ✅ Wallet address
- ❌ Fingerprint data — never
- ❌ Vein or iris patterns — never
- ❌ Name, address, ID — never
- ❌ IP address — not stored

---

## 8. Gleichheitsindex / Equality Index

### DE
Aequitas ist das erste Währungssystem, das seinen eigenen Gleichheitsgrad live und transparent misst und veröffentlicht.

**Gini-Koeffizient:** Misst die Ungleichverteilung des AEQ-Vermögens. 0 = perfekte Gleichheit, 1 = totale Konzentration.

**Lorenz-Kurve:** Zeigt grafisch, wie viel Prozent des Reichtums die ärmsten X% der Menschen besitzen.

**Aequitas-Index:** Kombinierter Score aus Gini, Verteilung, Aktivität und Wachstum.

**Ziel / Target:** Gini < 0,30 (Skandinavien-Niveau)

| Währung / Currency | Gini |
|-------------------|------|
| **Aequitas AEQ** | **~0,08** |
| Skandinavien / Scandinavia | ~0,27 |
| Deutschland / Germany | ~0,31 |
| USA | ~0,41 |
| Brasilien / Brazil | ~0,53 |
| Bitcoin | ~0,85 |

### EN
Aequitas is the first monetary system that measures and publishes its own equality level live and transparently.

**Gini Coefficient:** Measures inequality of AEQ wealth distribution. 0 = perfect equality, 1 = total concentration.

**Lorenz Curve:** Graphically shows what percentage of wealth the poorest X% of humans own.

**Aequitas Index:** Combined score from Gini, distribution, activity, and growth.

**Target:** Gini < 0.30 (Scandinavia level)

---

## 9. Exchange & Liquiditätspool / Liquidity Pool

### DE
Aequitas enthält einen integrierten automatischen Market Maker (AMM) für den Handel zwischen AEQ und tUSD (einem simulierten Test-Dollar auf der Aequitas Chain).

**Mechanismus:** Das klassische `x·y=k`-Modell — der Pool hält automatisch einen Gleichgewichtspreis aufrecht.

**Gebührenverteilung / Fee Distribution:**
- 0,1% Swap-Gebühr wird automatisch aufgeteilt:
  - 40% → Validator-Pool (Netzwerkanreiz)
  - 30% → Liquidity Provider (LP-Rendite)
  - 20% → UBI-Pool (Grundeinkommen)
  - 10% → Treasury (Protokoll-Entwicklung)

**Liquiditäts-Shares:** LPs erhalten proportionale Shares und können jederzeit ihre Anteile plus akkumulierte Gebühren abheben.

### EN
Aequitas contains a built-in Automated Market Maker (AMM) for trading between AEQ and tUSD (a simulated test-dollar on Aequitas Chain).

**Mechanism:** The classic `x·y=k` model — the pool automatically maintains an equilibrium price.

**Fee Distribution:**
- 0.1% swap fee automatically split:
  - 40% → Validator Pool (network incentive)
  - 30% → Liquidity Providers (LP yield)
  - 20% → UBI Pool (basic income)
  - 10% → Treasury (protocol development)

**Liquidity Shares:** LPs receive proportional shares and can withdraw their stakes plus accumulated fees at any time.

---

## 10. Sicherheit / Security

### Auditierte Sicherheitsmaßnahmen / Audited Security Measures

| Bedrohung / Threat | Schutz / Protection |
|-------------------|---------------------|
| Doppel-Registrierung / Double registration | Nullifier-Bindung on-chain / Nullifier binding on-chain |
| Replay-Attacke / Replay attack | Nonce-System mit CAS / Nonce system with compare-and-swap |
| Sybil-Attacke / Sybil attack | Biometrie + ZKP + Hardware Secure Element |
| Pool-Drain | Wealth Cap + Demurrage + optimistic locking |
| Contract-Upgrade-Risiko | Vollständiger Storage-Backup vor Wipe / Full storage backup before wipe |
| Multi-Node-Konflikte / Multi-node conflicts | PostgreSQL optimistic locking + SELECT FOR UPDATE |
| Öffentliche Contract-Deployments / Public deployments | Deployment auf Relayer-Adresse beschränkt / Restricted to relayer |
| Private Keys in Logs | Ausgabe nur auf stderr (nicht in Log-Aggregatoren) / stderr only |
| XSS | HTML-Escaping aller User-Eingaben / HTML escaping of all user inputs |
| DNS-Rebinding | Peer-URL-Validierung + Öffentliche-IP-Prüfung / Public IP check |

### Dezentralisierung / Decentralization

Aequitas befindet sich in Phase 0 mit zwei betriebenen Nodes. Das Protokoll ist für beliebig viele Nodes ausgelegt — jeder Node-Betreiber kann mit `PEER_NODES` beitreten.

Aequitas is in Phase 0 with two operated nodes. The protocol is designed for any number of nodes — any operator can join with `PEER_NODES`.

---

## 11. Roadmap

| Phase | Status | DE | EN |
|-------|--------|----|----|
| 0 | ✅ | Smart Contracts · ZKP · Android App · Proof Server | Smart Contracts · ZKP · Android App · Proof Server |
| 0+ | ✅ | Aequitas Layer 1 · BlockDAG + GHOSTDAG · P2P · Explorer | Aequitas Layer 1 · BlockDAG + GHOSTDAG · P2P · Explorer |
| V7 | ✅ | EVM · Dual-Ledger · Exchange/AMM · UBI · Demurrage · Wealth Cap · Lorenz/Gini | EVM · Dual-Ledger · Exchange/AMM · UBI · Demurrage · Wealth Cap · Lorenz/Gini |
| V7.x | ✅ | Proof of Alive · Guardian-System (Eskrow + UBI-Freigabe) live | Proof of Alive · Guardian System (escrow + UBI release) live |
| 1 | 🔄 | APK-Veröffentlichung · Community-Aufbau · Grant-Anträge · Mehr-Knoten-Skalierung | APK Release · Community Growth · Grant Applications · Multi-Node Scaling |
| 2 | ⬜ | iOS App | iOS App |
| 3 | ⬜ | Cross-Chain Bridges · Externe DEX-Integration | Cross-Chain Bridges · External DEX Integration |
| 4 | ⬜ | Vollständige Dezentralisierung · Community Governance | Full Decentralization · Community Governance |

---

## 12. Fazit / Conclusion

### DE
Aequitas ist kein weiteres Experiment in Kryptospekulation. Es ist ein ernsthafter Versuch, Geld neu zu denken — von Grund auf, für alle Menschen, fair.

Die mathematische Garantie ist simpel und radikal zugleich: Solange Menschen existieren, existiert AEQ. Kein Zentralstaat, keine Bank, kein Algorithmus kann das Grundeinkommen entziehen oder die Gleichheit untergraben — es ist Code.

Der Gini-Koeffizient von Aequitas liegt heute bei ~0,08. Bitcoin liegt bei ~0,85. Der Unterschied ist nicht zufällig — er ist das Ergebnis des Designs.

### EN
Aequitas is not another experiment in crypto speculation. It is a serious attempt to rethink money — from first principles, for all people, fairly.

The mathematical guarantee is simple and radical at once: as long as humans exist, AEQ exists. No central state, no bank, no algorithm can remove the basic income or undermine the equality — it is code.

Aequitas's Gini coefficient today is ~0.08. Bitcoin's is ~0.85. The difference is not coincidence — it is the result of design.

---

*Aequitas · Chain ID 1926 · aequitas.digital*
*Version 2.0 · Juni / June 2026*
*Lizenz / License: MIT · Open Source: github.com/hanoi96international-gif/Aequitas*
