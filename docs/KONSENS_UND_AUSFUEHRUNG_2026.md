# Aequitas 2026+: Konsens und Ausführung neu gedacht

Recherche und Fahrplan, Stand 01.10.2026. Grundlage sind eigene Messungen auf dem C1-Prüfstand (Läufe 4–6) und der veröffentlichte Stand der Forschung und Praxis 2025/2026.

Leitlinie: **Sicherheit vor Tempo.** Jede Stufe unten hat eigene Sicherheitsbeweise, Missbrauchstests und eine getrennte Sicherheitsprüfung (AGENTS.md). Keine Stufe lockert eine Schutzgrenze oder lässt eine Prüfung weg, die heute jeder Knoten selbst macht.

---

## 1. Was heute wirklich bremst (gemessen, nicht geschätzt)

Prüfstand C1 (8 Kerne): Last von 3 Rechnern, 6.000 Konten, 40 s.

| | Lauf 4 | Lauf 5 (JSON einmal) | Lauf 6 |
|---|---|---|---|
| Ketten-TPS | 6.590 | 7.993 | ~7.000 |
| Blöcke/s | 0,96 | 1,00 | ~1 |
| CPU Knoten / Postgres | 3,7 / 0,8 Kerne | 3,6 / 1,0 | 3,6 / 1,0 |

Das Protokoll je Produktionsversuch (Lauf 6, `/api/produktion`) zeigt die Ursache eindeutig:

- **Ohne Last** dauert ein Block 3–12 ms.
- **Unter Last** dauert er 300–2.500 ms. Fast die ganze Zeit liegt in zwei Postgres-Schritten:
  - **Laden** der offenen Überweisungen: 100–1.144 ms
  - **Speichern** von Block und Markierung: 150–1.363 ms
- Deshalb gab es in 45 s nur 7 Takte mit Zusatzblöcken. Bei einem Takt von 1 s und dem Deckel von 10.000 je Block ist ~10.000 TPS die Decke. Erreicht werden ~8.000, weil viele Blöcke länger als den Takt brauchen.

Der Grund liegt in der Architektur: Jede Überweisung macht eine **Runde durch Postgres**.

1. WAL schreiben (fsync).
2. Flush in die Tabelle `pending_txs`.
3. Der Block lädt sie aus derselben Tabelle zurück.
4. Der Block markiert sie (`UPDATE`).
5. Der Block speichert sich.
6. Die Zeilen werden gelöscht.

Jede Überweisung kostet so vier Schreibvorgänge auf derselben Tabelle, während der Flush gleichzeitig die nächsten 10.000 Zeilen hineinschreibt.

**Sicherheitsbefund am Rand (01.10.2026):** Die Zustandswurzel im Block enthält auch die angenommenen, noch nicht verblockten Überweisungen des Produzenten (`divergenz_waechter.go`). Ein anderer Knoten kann sie deshalb **nicht Block für Block** gegenprüfen; Abweichungen fallen erst in einer Ruhephase auf. Die Salden bleiben sicher, weil jeder Knoten die Transaktionen selbst nachspielt. Ziel von Stufe 2/3 ist trotzdem eine Wurzel, die **jeder Knoten je Block prüfen kann**: der ausgeführte Zustand nach Block n−D, nicht der spekulative.

**Ordnungsfehler gefunden und behoben (01.10.2026):** `LoadPendingTxsWithLimit` ordnete nach `wal_seq`, sortierte danach aber wieder nach Zeilen-ID, also nach der Flush-Reihenfolge. Ein Block konnte Überweisungen in anderer Reihenfolge tragen, als sie angewendet wurden; das ist die Fehlerklasse hinter der C1/C2-Abweichung vom 12.09. Behoben, mit einem Test, der das Ergebnis prüft und nicht nur den SQL-Text.

**Die CPU ist nicht die Grenze.** Die Signaturprüfung (ecrecover) ist mit ~43 % der größte CPU-Posten. Der Leistungsnachweis misst auf C1 85.000–110.000 Signaturen/s; das ist die Obergrenze je Knoten auf dieser Hardware, rund das Zehnfache des heutigen Durchsatzes.

---

## 2. Stand der Technik 2025/2026: was es gibt und was davon zu Aequitas passt

### 2.1 Konsens

| Ansatz | Kern | Zahlen | Passt zu Aequitas? |
|---|---|---|---|
| **Mysticeti** (Sui, NDSS 2025) | DAG-BFT ohne Zertifikate; jeder Block kann ohne Wartezeit committet werden; 3 Nachrichtenrunden (theoretisches Minimum) | 0,5 s Commit im WAN bei >200k TPS; Sui-Mainnet −80 % Latenz | **Ja, als Vorbild für Stufe 3.** Aequitas ist schon ein DAG mit parallelen Produzenten, hat aber nur probabilistische Finalität (50 Blöcke) |
| **Shoal++ / Autobahn** (2024/25) | Datenverbreitung (horizontal skalierbar) getrennt vom Konsens über „Schnappschüsse“ der Datenschicht | 4,5 Nachrichtenverzögerungen; Autobahn halbiert die DAG-Latenz und erholt sich nahtlos von Aussetzern | Ja, für die Trennung Daten/Ordnung |
| **Cadence** (Monad/Category Labs, Juli 2026) | Mehrere gleichzeitige Proposer je Slot, „extremes Pipelining“: Blöcke bauen nicht auf dem Vorgänger auf | 100-ms-Blöcke, ~220 ms Finalität bei 200 Validatoren (Simulation) | **Sehr gut.** Mehrere Proposer verhindern, dass ein einzelner Validator Überweisungen zurückhält. Das passt zum Fairness-Auftrag von Aequitas |
| **Alpenglow** (Solana, 2026) | Votor (1–2 Abstimmungsrunden, 80 %/60 %), Rotor (Verbreitung) | 100–150 ms Finalität; Mainnet-Aktivierung Stand 23.09.2026 noch nicht | Ideen ja (Schnellpfad bei 80 %), Implementierung nein (Solana-spezifisch) |
| **Clownfish** (Juni 2026) | DAG-BFT mit dünn besetzten Kanten: quadratische statt kubische Kommunikation, mehrere Leader je Runde | – | Wichtig, sobald es **viele** Validatoren gibt (1 Mensch = 1 Validator) |
| **Vantage** (Aug. 2026) | Signaturfreies BFT nur mit Hashes und authentifizierten Kanälen | 250k TPS, <500 ms bei 100 Parteien, 10 Regionen | Beobachten; die Signaturen sparen Validator-CPU |
| **DAGKnight** (Kaspa, für 2026 geplant) | Parameterloses GHOSTDAG, passt sich der echten Netzlatenz an | Kaspa läuft seit Mai 2025 mit 10 Blöcken/s | Aequitas' KnightDAG ist bereits davon inspiriert. Kaspa bleibt aber Nakamoto-artig (probabilistisch); für einen Validatorsatz aus bekannten Menschen ist BFT-Finalität stärker |

**Folgerung:** Aequitas hat eine Besonderheit, die kein anderes System hat: Der Validatorsatz besteht aus **verifizierten Menschen** (Proof of Humanity, 1 Mensch = 1 Stimme). Genau das ist die Voraussetzung, die BFT-Protokolle brauchen: ein bekannter Satz von Teilnehmern mit gleichem Gewicht und ohne Sybil-Angriffe. Andere Ketten müssen das über Kapital (Stake) erzwingen. Aequitas kann **deterministische Finalität unter einer Sekunde** über einen menschengewichteten DAG-BFT erreichen, statt 50 Blöcke probabilistisch zu warten.

### 2.2 Ausführung

| Ansatz | Kern | Passt? |
|---|---|---|
| **Asynchrone Ausführung** (Monad) | Konsens einigt sich nur auf die **Reihenfolge**. Ausgeführt wird danach in eigener Spur; die Zustandswurzel kommt D=3 Blöcke später. Bei der Ordnung wird nur geprüft: Signatur, Nonce und ob der Absender die maximale Abbuchung decken kann („Reserve Balance“) | **Ja.** Aequitas bucht heute schon bei der Annahme ab (WAL), die Deckung ist also vor dem Block geprüft. Der Konsens muss nicht auf die Ausführung warten |
| **Sei Giga** (2025) | Mehrere Proposer (Autobahn), Ausführung abseits des kritischen Pfads, spätere Bestätigung über kompakte Prüfsummen des Schreibprotokolls | Bestätigt die Richtung; 200k TPS/400 ms als Testnetz-Ziel |
| **Block-STM v2** (Aptos) | Optimistisch parallele Ausführung, nur Konflikte werden wiederholt; skaliert bis 256 Kerne | Teilweise vorhanden (disjunkte Überweisungen im Replay). Demurrage, UBI und Pools brauchen eine klare Konfliktregel |
| **Hyperliquid** | Ganzer Zustand im RAM, Ereignisprotokoll auf Platte, Schnappschuss etwa alle 12 min | **Ja, für Stufe 2:** Postgres raus aus dem heißen Pfad |
| **QMDB** (LayerZero, 2025) | Verifizierbare Datenbank: Append-only-Log, Merkle-Baum im RAM mit 2,3 Byte je Eintrag, 2,28 Mio. Zustandsänderungen/s, 6× RocksDB | Vorbild für die Zustandswurzel ohne Postgres; als Rust-Bibliothek nur mit Begründung (neue Abhängigkeit) |

### 2.3 Signaturen

- **Firedancer** prüft Ed25519 mit AVX-512 in eigenen Kernen („Tiles“), etwa 50.000 je Kern. Für secp256k1 (MetaMask) gibt es AVX-512-Batch-ecrecover (asmcrypto, ~1,4× libsecp256k1) und GPU-Bibliotheken (UltrafastSecp256k1). **Diese sind nicht auditiert und kommen für Aequitas nicht in Frage.** Kryptobibliotheken nur, wenn sie geprüft sind; libsecp256k1 (wie heute über go-ethereum) bleibt.
- Echte ECDSA-Batch-Verifikation erfordert ein anderes Signaturformat (IACR 2026/663) und ist nicht MetaMask-kompatibel.
- **Der wirkliche Hebel ist die Parallelisierung über Kerne** (heute schon umgesetzt). Ein 8-Kern-Knoten schafft etwa 85.000 Signaturen/s.

### 2.4 Validieren ohne Neu-Ausführen: ZK-Gültigkeitsbeweise

- **SP1 Hypercube** (Succinct, seit 2025 auf Mainnet) beweist 99,7 % aller Ethereum-Blöcke in Echtzeit mit 16 Consumer-GPUs (Cluster unter 100.000 $). Die Ethereum Foundation plant, dass L1-Validatoren Blöcke **nicht mehr neu ausführen**, sondern einen Beweis prüfen.
- **Für Aequitas ist das der revolutionärste Baustein.** Ein Beweis lässt sich in Millisekunden prüfen, auch auf einem Handy. „Jeder Mensch kann validieren“ wird damit wörtlich möglich: Validator ohne Server, direkt in der App.
- Der Beweiser braucht starke Hardware, ist aber **nicht vertrauenswürdig nötig**: Ein falscher Beweis wird abgelehnt. Jeder Knoten mit GPU kann beweisen.
- Aequitas hat schon Groth16/circom im Haus (`zk-forschung` im CI).

### 2.5 Post-Quanten

- NIST hat 2024 ML-DSA, SLH-DSA, XMSS und LMS standardisiert. Ethereum migriert über Account Abstraction; OP Labs setzt 2036 als Frist für secp256k1-EOAs.
- Für Aequitas: Konten über einen Vertrags-/Kontotyp auf PQ-Signaturen umstellbar machen, bevor Quantenrechner relevant werden. Validator-Stimmen mit hashbasierten Signaturen (XMSS) sind der konservativste Weg.

### 2.6 Lehren aus Ausfällen 2026

- **Sui stand am 14.01.2026 und am 28.05.2026 jeweils ~6 Stunden still.** Ursachen waren voneinander abweichende Checkpoint-Kandidaten (Garbage Collection plus Optimierungspfad) und ein Fehler in Release 1.72.
- Daraus folgt:
  1. Jede Optimierung braucht einen **Determinismus-Test**: schneller Pfad gleich Replay gleich StateRoot.
  2. Abweichungen müssen **früh erkannt** werden: Die Zustandswurzel von vor D Blöcken steht im Block.
  3. Neue Konsenspfade laufen zuerst **im Schatten** mit (sie berechnen, entscheiden aber nicht), bevor sie umschalten.

---

## 3. Zielarchitektur „Aequitas Strom“

Fünf Schichten. Jede ist für sich nützlich und für sich prüfbar.

```
 Annahme ──► Speicher-Mempool ──► Ordnung (Konsens) ──► Ausführung ──► Beweis/Bestätigung
 (Signatur,   (WAL ist die        (menschengewichteter   (parallel,     (StateRoot D Blöcke
  Nonce,       Quelle, Blöcke      DAG-BFT, mehrere       Zustand        später; später
  Deckung)     aus dem RAM)        Proposer, <1 s final)  im RAM)        ZK-Beweis)
```

**Unverhandelbar in jeder Stufe:**
1. Jeder Knoten prüft Signatur, Nonce, Deckung und StateRoot **selbst**. Optimierungen verschieben oder parallelisieren, lassen aber nichts weg.
2. Dauerhaftigkeit: Was angenommen und quittiert ist, überlebt jeden Absturz (fsync im WAL vor der Quittung, wie heute).
3. Keine doppelte Aufnahme in Blöcke und kein Verlust, auch nicht bei Absturz zwischen zwei Schritten.
4. Alle Schutzgrenzen bleiben: 10.000 je Block, Inflight, Rückstau, WAL-Druck, Rate-Limit.

---

## 4. Fahrplan

### Stufe 1: Blöcke aus dem Speicher statt über Postgres (größter Hebel, jetzt)

**Problem:** Die Postgres-Runde in Laden und Speichern (Abschnitt 1).

**Lösung:**
- Angenommene Überweisungen stehen nach dem WAL-fsync in einer **Speicher-Warteschlange in WAL-Reihenfolge**.
- Der Blockbau nimmt daraus ein Präfix (höchstens Deckel).
- Mit dem Block wird atomar **„aufgenommen bis WAL-Seq S“** gespeichert.
- `pending_txs` fällt aus dem heißen Pfad.

**Sicherheit und Absturz:**
- **Absturz vor dem Speichern des Blocks:** Alles über S ist noch im WAL. Beim Start wird jeder WAL-Eintrag mit Seq > S wieder eingereiht. Es geht nichts verloren und nichts wird doppelt aufgenommen.
- **Absturz nach dem Speichern:** S ist mit dem Block gespeichert, die Einträge ≤ S werden nicht wieder eingereiht.
- **Reihenfolge:** Eine Lücke in der Seq (ein Append noch im fsync) hält die Freigabe an, bis sie geschlossen ist. Ein Block nimmt nie Seq n+1 ohne n.
- **WAL-Kompaktierung:** Sie darf nie über S kürzen (zusätzliche Bedingung zur heutigen).
- **Fremde Blöcke** (Replay) sind unverändert: Jeder Knoten prüft sie selbst.

**Tests:**
- Absturz an jeder Stelle (Fehlereinspeisung): kein Verlust, keine Doppelaufnahme.
- Determinismus: alter und neuer Pfad liefern dieselbe Blockfolge.
- Lücken, Neustart mit vollem WAL, Kompaktierung.
- Missbrauch: Wiederholung derselben Überweisung und fremder Absender werden abgewiesen wie heute.

**Ziel:** Blöcke unter Last im Bereich 50–200 ms statt 300–2.500 ms; Zusatzblöcke greifen; **15.000–25.000 TPS auf C1.**

**Ausrollen:** Schalter `AEQUITAS_BLOCK_AUS_SPEICHER=1` (Standard aus) → Prüfstand → C1 aktiv. Umgesetzt in `speicherkorb.go` mit diesen Tests:
- Lückenloses Präfix, nur Haltbares, Zurücklegen, Wiederholung nicht doppelt.
- Dauerhafte Lücke sperrt (fail-closed); nur ein Blockbauer gleichzeitig.
- Mischen mit langsamen Zeilen; die Kompaktierung kürzt nie über die Marke.
- Gegen echtes Postgres und WAL: Absturz vor dem Block (nichts verloren), Absturz nach dem Block (nichts doppelt), Ausschalten (Zeilen genau einmal nachgeholt), Ausschalten nach Block ohne Flush (nicht doppelt).

### Stufe 2: Zustand im RAM, Postgres als nachlaufender Index

**Lösung:**
- Kontostände und Nonces leben im RAM; das ist teilweise schon so (`shardedAccounts`).
- Dauerhaft ist das WAL plus Schnappschuss (Hyperliquid-Muster).
- Postgres bekommt Blöcke und Kontostände asynchron, nur für API und Explorer.
- Die Zustandswurzel wird inkrementell im RAM fortgeschrieben (QMDB-Muster, in Go, keine neue Abhängigkeit) statt über SQL.

**Sicherheit:**
- Wiederanlauf aus Schnappschuss plus WAL. Der Schnappschuss trägt seine eigene Zustandswurzel und wird beim Laden gegengeprüft.
- Weicht die Zustandswurzel nach dem Wiederanlauf ab, gilt: Halt (fail-closed) statt Weiterlaufen.

**Ziel:** Die Signaturprüfung wird zur Grenze, also **50.000–80.000 TPS auf 8 Kernen.**

### Stufe 3: Menschengewichteter DAG-BFT mit asynchroner Ausführung (Konsensänderung)

**Lösung:**
- Validatorsatz = registrierte Menschen mit Knoten, je eine Stimme.
- Mysticeti/Cadence-artiger DAG ohne Zertifikate, mehrere Proposer je Runde (Zensurschutz).
- Deterministische Finalität in ~3 Nachrichtenrunden (<1 s statt 50 Blöcke).
- Konsens ordnet nur; geprüft wird bei der Ordnung Signatur, Nonce und Deckung (Reserve, Monad-Muster).
- Ausführung D=3 Blöcke versetzt; die Zustandswurzel von Block n−3 steht in Block n.
- Bei Abweichung: Alarm und Halt statt stiller Spaltung.

**Sicherheit:**
- Formale Spezifikation (TLA+) und Modellprüfung **vor** dem Code.
- Schattenbetrieb neben GHOSTDAG, dann Umschalten über eine festgelegte Höhe.
- f < n/3 byzantinische Validatoren. Bei kleinen Netzen (heute 1–2 Knoten) ist BFT **keine** Verbesserung. Diese Stufe lohnt erst ab etwa 4 unabhängigen Menschen-Validatoren.

**Ziel:**
- Finalität < 1 s.
- Durchsatz wächst mit der Datenverbreitung (Autobahn/Narwhal-Prinzip) statt mit einem einzigen Produzenten.
- Zensurschutz durch mehrere Proposer.

### Stufe 4: ZK-Gültigkeitsbeweise: Validieren auf dem Handy

**Lösung:**
- Ein Beweiser (beliebiger Knoten mit GPU) erzeugt je Block bzw. je Epoche einen Beweis über Ausführung und Zustandswurzel.
- Validatoren prüfen den Beweis statt neu auszuführen.

**Sicherheit:**
- Das Beweissystem ist neue Angriffsfläche. Nur auditierte zkVMs/Schaltungen.
- Übergangsphase: Beweis **und** Neu-Ausführung, bis die Beweise über Monate fehlerfrei sind.

**Ziel:** Validieren ohne Server. Das ist die konsequenteste Form von „1 Mensch = 1 Validator“.

### Stufe 5: Post-Quanten-Pfad

- Kontotyp mit PQ-Signatur (ML-DSA oder hashbasiert) über Account Abstraction.
- Validator-Stimmen mit XMSS.
- Zeitplan vor jeder realistischen Quantenbedrohung, nicht danach.

---

## 5. Was ausdrücklich nicht gemacht wird

- **Prüfungen auf andere Knoten verlagern** („der Peer hat schon geprüft“): Jeder Knoten prüft selbst.
- **Nicht auditierte Krypto** (AVX-512/GPU-ecrecover-Bibliotheken, eigene Signaturbatches).
- **Grenzen anheben, um Messwerte zu verbessern** (10.000 je Block, Inflight, Rückstau, Rate-Limit).
- **Konsensänderung ohne Spezifikation, Modellprüfung und Schattenbetrieb.**
- **Sharding über Validatorgruppen:** Es kollidiert mit Demurrage/UBI über alle Konten und ist für die heutige Größe nicht nötig.

---

## 6. Quellen

- Mysticeti: [arXiv 2310.14821](https://arxiv.org/abs/2310.14821), [Decentralized Thoughts 2026](https://decentralizedthoughts.github.io/2026-03-06-mysticeti-revolutionizing-consensus-on-sui/)
- Shoal++: [arXiv 2405.20488](https://arxiv.org/pdf/2405.20488); Autobahn: [SOSP 2024](https://dl.acm.org/doi/10.1145/3694715.3695942)
- Cadence: [arXiv 2607.02275](https://arxiv.org/abs/2607.02275), [Monad-Blog](https://www.monad.xyz/blog/cadence-multiple-concurrent-proposers)
- Multiple Concurrent Proposers: [IACR 2025/1772](https://eprint.iacr.org/2025/1772); Zensur vs. Durchsatz: [IACR 2026/126](https://eprint.iacr.org/2026/126.pdf)
- Clownfish: [arXiv 2606.04687](https://arxiv.org/abs/2606.04687); Vantage: [arXiv 2608.16504](https://arxiv.org/abs/2608.16504)
- Alpenglow: [solana.com](https://solana.com/upgrades/alpenglow), [crypto.news, Stand Sept. 2026](https://crypto.news/solana-promised-faster-finality-validators-now-have-to-prove-it-works/)
- Monad asynchrone Ausführung: [docs.monad.xyz](https://docs.monad.xyz/monad-arch/consensus/asynchronous-execution), [Spezifikation](https://category-labs.github.io/category-research/monad-initial-spec-proposal.pdf)
- Sei Giga: [arXiv 2505.14914](https://arxiv.org/pdf/2505.14914)
- Block-STM: [arXiv 2203.06871](https://arxiv.org/abs/2203.06871)
- QMDB: [arXiv 2501.05262](https://arxiv.org/abs/2501.05262)
- Hyperliquid-Knoten: [Hyperliquid Docs](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/nodes/l1-data-schemas)
- Kaspa DAGKnight/Crescendo: [Cointribune](https://www.cointribune.com/en/kaspa-targets-100-blocks-per-second-dagknight-zk-and-vprogs)
- Firedancer: [Chainstack](https://chainstack.com/firedancer-explained/); secp256k1 AVX-512: [asmcrypto](https://github.com/atomicincrement/asmcrypto)
- SP1 Hypercube: [Succinct](https://blog.succinct.xyz/real-time-proving-16-gpus/)
- Post-Quanten: [SoK arXiv 2512.13333](https://arxiv.org/pdf/2512.13333)
- Sui-Ausfälle 2026: [KuCoin](https://www.kucoin.com/blog/sui-mainnet-outage-gas-logic-bug-1b-assets-2026), [Bex](https://bex.co/blog/2026/01/20/sui-network-outage-blockchain-reliability-institutional-adoption)
