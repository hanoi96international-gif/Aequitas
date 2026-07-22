# Aequitas Scaling-Architektur — Design-Dokument für ein eigenes Projekt

**Status: NICHT implementieren.** Dies ist ein Planungsdokument für ein separates, dediziertes Vorhaben — nicht für die laufende Session, nicht ohne Staging/Testnet-Validierung, nicht als Nebeneffekt eines anderen Tasks. Grund siehe unten.

## Ziel

Aequitas soll deutlich mehr TPS verarbeiten können, ohne die beiden nicht verhandelbaren Eigenschaften des Projekts zu verletzen:

1. **Dezentralität** — kein Design, das teure/spezialisierte Validator-Hardware voraussetzt.
2. **1 Mensch = 1 Validator-Slot** — die PoH-Gate bleibt die alleinige Zugangskontrolle für Validatoren, unabhängig von Rechenleistung.

Das schließt einen Teil der Techniken aus, mit denen andere Hochdurchsatz-Ketten (Solana, Keeta, etc.) ihre Zahlen erreichen — die setzen teils bewusst auf leistungsstarke, spezialisierte Validator-Nodes. Für Aequitas ist "schnell, aber nur mit High-End-Server betreibbar" kein akzeptabler Trade-off.

## Ausgangslage (Stand dieser Session, real gemessen und live deployed)

Alle Zahlen: `TestSimulateMaxTPS_Ingestion`, Einzel-Node, echtes lokales Postgres, keine Netzwerklatenz zu anderen Nodes.

| Schritt | TPS | Fix |
|---|---|---|
| Baseline (vor dieser Session) | **0** (Deadlock unter Last) | — |
| Connection-Pool-Deadlock behoben | 186 | `runAtomicWithOutbox`/`runAtomicDistributionWithOutbox` öffneten `cs.db.Begin()` **vor** einem Lesezugriff, der selbst noch eine freie Pool-Verbindung brauchte — bei `MaxOpenConns=20` und genug gleichzeitigen Callern ging der Pool aus, bevor irgendjemand `cs.mu` bekam. Reihenfolge getauscht + 5 weitere Stellen (`ensureAccountLoaded`, `RemoveFromEVMMirrorSyncQueue` u.a.), die `cs.db` statt `cs.dbExec()` benutzten und dieselbe Pool-Erschöpfung auslösten. |
| Group-Commit für Transfers | 285 | `TransferAtomic` bündelt jetzt bis zu 200 gleichzeitige Aufrufe in eine gemeinsame DB-Transaktion/Commit statt eine pro Transfer. All-or-nothing-Semantik (siehe unten, "Anti-Pattern: zwei Wahrheitsquellen"). |
| EVM-Mirror-Sync-Batching | 348 | 6 einzelne `DELETE`-Aufrufe pro Transfer (Cleanup der Retry-Queue) zu einem gebündelten `DELETE ... WHERE address = ANY($1)` zusammengefasst. |
| Skip bei leerer Retry-Queue | ~400 | Atomarer Flag (`evmMirrorQueueMaybeNonEmpty`), der den Cleanup-Round-Trip komplett überspringt, wenn nichts zu bereinigen ist (Normalfall). |

Alles davon: vollständig getestet (`go test ./... -race`), inklusive adversarialer Korrektheitstests (ein fehlschlagendes Batch-Mitglied darf keinen Teil-Zustand hinterlassen — bewiesen durch sauberen Retry mit exaktem erwarteten Saldo), live deployed auf beiden Contabo-Nodes.

## Warum bei ~400 TPS Schluss ist ohne größeren Umbau

CPU-Profiling von `TestSimulateMaxTPS_Ingestion` (siehe `AEQUITAS_TPS_CPUPROFILE`-Hook im Benchmark-Test) zeigt eindeutig:

- Nur **33 % CPU-Auslastung** über die gesamte Laufzeit — der Prozess **wartet** die meiste Zeit, er rechnet nicht.
- Von der tatsächlich genutzten CPU-Zeit: **~58 % unter `database/sql.withLock`**, **~45 % in rohen Syscalls** (Netzwerk-I/O zum lokalen Postgres).
- `syncBalanceLocked` (EVM-Mirror-Sync) allein: über 50 % der Batch-Verarbeitungszeit, selbst nach Batching der eigenen Queries.

Kurz: **jeder Transfer braucht mehrere sequentielle Netzwerk-Round-Trips zu Postgres, und die passieren heute komplett seriell hinter einem einzigen globalen Mutex (`cs.mu`)**. Group-Commit hat die Commit/fsync-Kosten amortisiert (daher der erste große Sprung), aber die Round-Trip-Kette selbst blieb seriell — daher die sinkenden Grenzerträge bei jedem weiteren Fix (53 % → 22 % → 13 % → 1,5 %).

Um eine Größenordnung weiterzukommen, muss die **Serialisierung selbst** verschwinden, nicht nur ihre Kosten pro Durchlauf sinken.

## Zielarchitektur

### 1. Sharded Account-Store statt einer globalen Map + einem globalen Mutex

`cs.accounts map[string]*AccountState`, geschützt durch `cs.mu`, wird ersetzt durch N unabhängige Shards (eigene Map + eigener Mutex pro Shard, Routing via `hash(address) % N`). Operationen, die nur Konten in **unterschiedlichen** Shards berühren, laufen dann echt parallel.

**Wichtige Falle, die im Design vermieden werden muss:** Go-Maps sind **nicht** nebenläufigkeitssicher — auch nicht für unterschiedliche Keys von unterschiedlichen Goroutinen (Rehashing kann die gesamte interne Struktur betreffen, nicht nur den einen Bucket). Es müssen echte separate Map-Instanzen pro Shard sein, kein "ein Mutex-Array über eine gemeinsame Map".

### 2. Cross-Shard-Transfers: deterministische Lock-Reihenfolge

Ein Transfer zwischen zwei Konten in unterschiedlichen Shards braucht beide Shard-Locks. Reihenfolge **immer nach Shard-Index aufsteigend**, sonst klassisches Deadlock-Risiko (A sperrt Shard 3 dann 7, B sperrt Shard 7 dann 3, beide warten für immer).

### 3. Tokenomics-Pools von der Fast-Path entkoppeln

Die vier Pool-Adressen (`validatorsPoolAddr`, `lpPoolAddr`, `ubiPoolAddr`, `treasuryPoolAddr`) werden potenziell von **jedem** Transfer berührt (Demurrage-Gebührenverteilung). Ohne Entkopplung wäre das der eine gemeinsame Hot-Spot, der jede Sharding-Parallelität wieder zunichtemacht — jeder Transfer bräuchte am Ende doch wieder denselben Lock.

Lösung: atomare In-Memory-Akkumulatoren (`atomic.Int64` pro Pool, Mikro-Einheiten) statt synchroner Kontomutation im kritischen Pfad. Ein Hintergrund-Worker flusht die akkumulierten Beträge periodisch (z. B. alle 1–5 Sekunden) in einer gebündelten DB-Schreibung. Trade-off: Pool-Salden sind für diesen kurzen Zeitraum "eventually consistent" statt sofort sichtbar — für Nutzer-kritische Pfade (Sender-Guthaben-Prüfung) irrelevant, das bleibt synchron und korrekt.

### 4. State-Root-Akkumulatoren (`accountSetXOR`, `nullifierSetXOR`) lock-frei oder fein-granular

Aktuell werden diese globalen XOR-Akkumulatoren bei **jedem** Kontospeichern unter `cs.mu` aktualisiert (`saveAccountToDB` → `xorInto`). Das ist eine zweite globale Kontention, unabhängig vom Account-Sharding. Zwei Optionen:
   - Ein eigener, sehr schlanker Mutex nur für diese ~32-Byte-XOR-Operation (kurze Critical Section, aber immer noch global seriell) — einfacher, aber begrenzter Gewinn.
   - Per-Shard-Partial-XORs, kombiniert nur beim tatsächlichen Lesen des State-Roots — mehr Parallelität, mehr Komplexität, muss mit `StateRoot()`s bestehendem Vertrags-Design abgeglichen werden.

### 5. EVM-Mirror-Sync komplett asynchron (post-commit)

`syncBalanceLocked` ist bereits als "Anzeige-Cache für `eth_call`/MetaMask" dokumentiert, nicht als Teil des autoritativen Ledgers (`chain_accounts`/`cs.accounts` bleibt Wahrheitsquelle). Die vorhandene `QueueEVMMirrorSync`/`RetryEVMMirrorSyncQueue`-Infrastruktur (heute nur ein Fallback bei Fehlern) sollte der **primäre** Pfad werden: Transfer committet, EVM-Mirror-Update wird danach asynchron nachgezogen. Halbiert den Round-Trip-Bedarf im kritischen Pfad nochmal.

## Konkret ermittelter Umfang (Stand dieser Session)

- **190 Stellen** in 6 Dateien greifen direkt auf `cs.accounts` zu (state.go: 142, snapshot.go: 15, guardian.go: 14, block.go: 10, evm_storage.go: 8, api.go: 1) — inklusive Mustern wie `for addr, acc := range cs.accounts` (volle Iteration), `json.Marshal(cs.accounts)` (Serialisierung der ganzen Map im No-DB-Fallback-Modus), verketteten Zugriffen wie `cs.accounts[to].Balance = cs.accounts[to].Balance.Add(...)`.
- **~90 Stellen** rufen `cs.mu.Lock()`/`cs.mu.RLock()` direkt auf (state.go allein: ~59 Lock-Acquisitions, 111 Lock-bezogene Aufrufe insgesamt).
- Betroffene Subsysteme, die alle auf dasselbe `cs.accounts`/`cs.mu` angewiesen sind und beim Sharding **gemeinsam** migriert werden müssen (nicht nur Transfers — sonst entstehen zwei nicht synchronisierte Wahrheitsquellen für denselben Kontostand, siehe unten): Transfer, Swap, Liquidity, Registrierung, tägliche Distribution (UBI/Validator/LP/Escrow), Guardian/Escrow-Recovery, Slashing, Snapshot-Export/-Import.

## Anti-Pattern, das im Migrationsdesign explizit vermieden werden muss

**"Nur Transfers shardieren, Rest unverändert lassen"** — naheliegend, aber falsch: `cs.accounts` ist die EINE Wahrheitsquelle, die Swap/Liquidity/Registrierung/Distribution/Guardian/Slashing alle direkt lesen und schreiben. Ein separater, geshardeter Store nur für Transfers würde sofort divergieren — ein Swap, der kurz nach einem Transfer denselben Account liest, sähe den alten Stand nicht. Konsequenz: **der Storage-Layer muss als Ganzes migriert werden**, auch wenn nur der Transfer-Pfad zunächst die neue Locking-Granularität nutzt (andere Operationen können den geshardeten Store zunächst weiterhin komplett unter einem koordinierenden Lock ansprechen — das ist sicher, liefert aber für DIESE Operationen noch keinen Parallelitätsgewinn; das ist der Punkt für eine spätere Phase).

## Historischer Kontext — warum das hier besonders vorsichtig gemacht werden muss

Dieses Repo hat bereits mehrfach reale Production-Incidents genau in diesem Bereich gehabt (Concurrency rund um Kontobewegungen), u. a.:
- Ein Doppel-Credit-Bug durch unsynchronisierten State beim Block-Replay.
- Mindestens zwei Konsens-Forks durch Locking-/Determinismus-Lücken in der GHOSTDAG-Berechnung.
- Zwei Connection-Pool-Deadlocks, in dieser Session selbst gefunden und gefixt (siehe Tabelle oben).

Jede dieser Klassen von Bug wäre in einem 280-Stellen-Umbau des Kern-Storage-Layers leichter zu übersehen, nicht schwerer.

## Migrationsplan (Phasen, wenn das Projekt gestartet wird)

1. **Sharded-Store-Primitive isoliert bauen und testen** — eigener Typ mit map-ähnlichem Interface (`Get`/`Set`/`Delete`/`Range`/`Len`), inklusive `MarshalJSON` für den No-DB-Fallback-Pfad. Reine Unit-Tests, kein Produktionscode berührt.
2. **Mechanische Migration, verhaltensidentisch**: `cs.accounts` auf den neuen Typ umstellen, aber weiterhin komplett unter `cs.mu` verwenden (keine neue Nebenläufigkeit) — Datei für Datei, nach jeder Datei volle Testsuite + `-race`. Ziel dieser Phase: beweisen, dass der Storage-Layer korrekt ist, ohne gleichzeitig neues Nebenläufigkeitsverhalten einzuführen.
3. **Pool-Fee-Entkopplung** (Punkt 3 oben) — eigenständig testbar, unabhängig von Sharding.
4. **State-Root-Akkumulatoren entkoppeln** (Punkt 4 oben).
5. **Transfer-Pfad auf Shard-Locks statt `cs.mu` umstellen** — der eigentliche Parallelitätsgewinn. Andere Operationen (Swap, Distribution, etc.) bleiben vorerst auf grobem Locking, sind aber durch Schritt 2 bereits kompatibel mit dem neuen Store.
6. **EVM-Mirror-Sync asynchron** (Punkt 5 oben).
7. Erst danach, operation-by-operation, weitere Subsysteme auf feingranulares Locking umstellen — jedes einzeln, mit eigener Test-Kampagne.

## Teststrategie (nicht verhandelbar für Phase 5+)

- Property-based / randomisierte Concurrency-Tests: viele parallele Goroutinen, zufällige Sender/Empfänger-Paare über mehrere Shards, Prüfung auf **Gesamtsaldo-Erhaltung** (Summe aller Konten vor/nach identisch) und **keine verlorenen Updates** unter harter Last.
- `-race` bei jedem Lauf, ohne Ausnahme.
- Chaos-artige Tests: erzwungene Commit-Fehler, erzwungene Teil-Batch-Fehler, Prozess-Kill-Simulation während eines Flushes — jeweils mit Beweis, dass In-Memory- und DB-Zustand danach wieder konsistent sind.
- **Vor Produktions-Deploy: Staging/Testnet-Lauf über mehrere Tage unter synthetischer Last**, nicht direkter Sprung auf die live laufenden Contabo-Nodes. Diese Umgebung existiert aktuell nicht und müsste als Teil dieses Projekts aufgebaut werden.

## Realistisches Zielbild

Ehrlich, nicht optimistisch gerundet: Mit vollständiger Umsetzung der obigen Architektur (Sharding + Pool-Entkopplung + async EVM-Mirror) ist ein Sprung in den **niedrigen bis mittleren Tausender-Bereich pro Node** plausibel — begrenzt durch echte Postgres-Round-Trip-Latenz pro Konto-Mutation und die Anzahl gleichzeitig nutzbarer DB-Connections (`max_connections`). Für 50.000+ TPS **sustained** wäre zusätzlich ein fundamentalerer Wechsel nötig (State primär im RAM, Postgres nur noch als asynchrones Durability-Log statt synchroner Wahrheitsquelle) — das ist ein eigener, noch größerer Entwurfsschritt, hier bewusst nur benannt, nicht weiter ausgeplant.

## Aufwandsschätzung

Kein Wochenend-Projekt. Realistisch: mehrere Wochen fokussierter Arbeit für Phasen 1–6, plus die Zeit für den Aufbau einer Staging-Umgebung, plus Zeit für die Test-Kampagne selbst — nicht etwas, das in einer fortlaufenden Chat-Session sicher zu Ende gebracht werden sollte.
