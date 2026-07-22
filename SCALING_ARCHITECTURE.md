# Aequitas Scaling-Architektur — Design-Dokument für ein eigenes Projekt

**Status: NICHT implementieren.** Dies ist ein Planungsdokument für ein separates, dediziertes Vorhaben — nicht für die laufende Session, nicht ohne Staging/Testnet-Validierung, nicht als Nebeneffekt eines anderen Tasks. Grund siehe unten.

## Ziel

**Explizites Zahlenziel: mindestens 50.000 TPS sustained, pro Node.** Nicht "so schnell wie machbar" — 50.000 ist der Maßstab, an dem sich dieses Design messen lassen muss.

Dabei gelten zwei nicht verhandelbare Nebenbedingungen:

1. **Dezentralität** — kein Design, das teure/spezialisierte Validator-Hardware voraussetzt.
2. **1 Mensch = 1 Validator-Slot** — die PoH-Gate bleibt die alleinige Zugangskontrolle für Validatoren, unabhängig von Rechenleistung.

Das schließt einen Teil der Techniken aus, mit denen andere Hochdurchsatz-Ketten (Solana, Keeta, etc.) ihre Zahlen erreichen — die setzen teils bewusst auf leistungsstarke, spezialisierte Validator-Nodes. Für Aequitas ist "schnell, aber nur mit High-End-Server betreibbar" kein akzeptabler Trade-off. Die Konsequenz daraus ist weiter unten (Abschnitt "Realistisches Zielbild") technisch eingeordnet: 50.000 TPS auf bescheidener, für Einzelpersonen erschwinglicher Hardware ist ambitioniert, aber mit dem richtigen Architekturwechsel (Zielarchitektur Punkt 6, "State primär im RAM") kein Fantasiewert — vergleichbare Größenordnungen erreichen In-Memory-Systeme wie Redis auf einem einzelnen Kern schon heute für einfache Operationen. Der Unterschied zu "einfach mehr Server" ist unten konkret ausgearbeitet, nicht nur behauptet.

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

### 6. State primär im RAM — Postgres wird vom synchronen Schreibpfad zum asynchronen Durability-Log

Das ist der Baustein, der tatsächlich nötig ist, um von "niedriger vierstelliger Bereich" (Sharding allein, Abschnitte 1–5) auf **50.000 TPS** zu kommen — und der größte Architektur-Einschnitt in diesem Dokument. Ohne ihn bleibt Postgres-Netzwerklatenz pro Kontomutation der Deckel, unabhängig davon, wie fein geshardet wird (siehe Profiling-Befund oben: 58 % der CPU-Zeit unter `database/sql.withLock`, nicht unter eigener Rechenarbeit).

**Grundprinzip** (Standardmuster für Hochdurchsatz-Systeme, z. B. wie Redis/Kafka/klassische RDBMS-Commit-Logs intern arbeiten): die In-Memory-Kopie eines Kontos (bereits heute vorhanden: `cs.accounts`/geshardeter Store) wird zur **primären** Wahrheitsquelle für Lese-/Schreiblogik. Postgres wird vom "muss vor jeder Bestätigung synchron beschrieben werden" zu "wird asynchron, gebündelt nachgeführt, für Durability und für alles, was SQL-Abfragen braucht (Explorer, Reporting)".

**Konkreter Mechanismus:**
1. **Lokales, sequentielles Write-Ahead-Log (WAL)**: jede eingehende, business-logisch bereits validierte Transaktion (Guthaben geprüft etc.) wird zuerst als kompakter Eintrag an eine lokale, ausschließlich anhängende Log-Datei angehängt (append-only, sequentielles Schreiben ist um Größenordnungen billiger als zufällige Schreibzugriffe — das ist exakt, warum Datenbank-Commit-Logs so funktionieren). Erst NACH einem erfolgreichen WAL-append gilt die Transaktion als "angenommen".
2. **Gruppen-Commit auf dem WAL selbst**: mehrere gleichzeitig eintreffende Transaktionen werden zu einem einzigen `fsync` auf das WAL gebündelt (dasselbe Prinzip wie das bereits gebaute Postgres-Group-Commit, nur eine Ebene tiefer und auf einem viel billigeren Medium — ein lokales sequentielles Log statt einer vollen relationalen Transaktion).
3. **Sofortige In-Memory-Anwendung**: nach dem WAL-append wird die Mutation sofort auf den geshardeten In-Memory-Store angewendet (Abschnitte 1–2) — keine Netzwerklatenz, reine Speicheroperation, im Mikrosekundenbereich.
4. **Asynchrone Nachführung nach Postgres**: ein Hintergrundprozess liest das WAL (oder einen In-Memory-Puffer bereits angewendeter Änderungen) und schreibt in gebündelten Batches nach `chain_accounts`/`pending_txs` — für Explorer-Abfragen, Cross-Node-Sync (Block-Relay/Replay bleibt wie heute über `pending_txs` laufen) und als zweite, langsamere Durability-Schicht.
5. **Crash-Recovery**: beim Neustart wird der letzte durch Postgres bestätigte Stand geladen, dann das WAL ab diesem Punkt erneut angewendet (klassisches WAL-Replay, exakt das Muster, das `chain_blocks.replayed`/`LoadUnreplayedBlocksFromDB` in diesem Repo für Blöcke schon heute kennt — hier auf der Konto-Ebene, nicht der Block-Ebene).

**Was das NICHT ändert:** Die Konsens-Semantik bleibt identisch — andere Nodes erfahren von einer Transaktion weiterhin über `pending_txs`/Blockrelay, nicht über das lokale WAL (das ist rein prozessintern für Durability, kein Netzwerkprotokoll). Der StateRoot-Vertrag (`accountSetXOR` etc.) bleibt unverändert, nur die Reihenfolge, WANN etwas in Postgres landet, ändert sich.

**Was das NEU einführt, sorgfältig bedacht werden muss:**
- Ein WAL-Format, Rotation/Kompaktierung (das Log darf nicht unbegrenzt wachsen), und ein sauberer Kontrakt für "ab welchem Punkt gilt eine Transaktion als durable" (heute: Postgres-Commit; neu: WAL-append — API-Antwortverhalten an Aufrufer wie `evm_rpc.go` muss entsprechend angepasst werden).
- Der Explorer/`/api/status` und alle SQL-basierten Reports lesen dann einen **leicht verzögerten** Stand (Sekundenbruchteile bis wenige Sekunden Lag zu Postgres) — muss dokumentiert und für Endnutzer sichtbar sein, wo es relevant ist (z. B. Balance-Anzeige direkt nach einer Transaktion).
- Dieser Baustein ist der mit Abstand größte Vertrauensvorschuss in diesem ganzen Dokument — er ändert die Durability-Garantie des gesamten Systems, nicht nur seine Geschwindigkeit. Braucht die längste, härteste Test-Kampagne von allen hier beschriebenen Schritten (siehe Teststrategie unten, explizit inklusive Crash-Simulation).

### 7. Blockproduktion/-relay: bisher unbetrachtete Engpassstelle OBERHALB der Storage-Schicht

Alle bisherigen Abschnitte optimieren, wie schnell `ChainState` einzelne Transaktionen annehmen und persistieren kann. Sie sagen nichts darüber aus, wie schnell die fertigen Blöcke, in denen diese Transaktionen an andere Nodes weitergereicht werden, selbst verarbeitet werden können — und das ist eine eigene, bisher nicht untersuchte Engpassstelle:

`ProduceBlock` hat aktuell **keine Obergrenze** für Transaktionen pro Block — jeder Tick (`BLOCK_TIME`, ~1–2 s) drainiert den kompletten wartenden Mempool in EINEN Block. Bei 50.000 TPS wären das 50.000–100.000 Transaktionen in einem einzigen Block: JSON-Serialisierung dieser Blockgröße, Hash-Berechnung, P2P-/HTTP-Verteilung an andere Nodes, GHOSTDAG-Verarbeitung der Merge-Sets — alles Kostenfaktoren, die heute nur für kleine, realistische Blockgrößen (wenige bis niedrige Zehntausend Blocks insgesamt, nicht Zehntausende Transaktionen PRO Block) geprüft sind. Muss als eigener Punkt im Projekt untersucht werden — vermutlich mit einer Obergrenze pro Block plus mehreren Blöcken/Tick oder kürzerer `BLOCK_TIME`, statt eines einzigen unbegrenzt wachsenden Blocks. Nicht im Umfang von Abschnitten 1–6 enthalten.

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
5. **Transfer-Pfad auf Shard-Locks statt `cs.mu` umstellen** — der erste echte Parallelitätsgewinn (erwartet: niedriger bis mittlerer vierstelliger TPS-Bereich, siehe Zielbild unten). Andere Operationen (Swap, Distribution, etc.) bleiben vorerst auf grobem Locking, sind aber durch Schritt 2 bereits kompatibel mit dem neuen Store.
6. **EVM-Mirror-Sync asynchron** (Abschnitt 5 der Zielarchitektur).
7. **WAL + In-Memory-primär** (Abschnitt 6 der Zielarchitektur) — DER Schritt, der auf dem Weg zu 50.000 TPS liegt. Eigenständiges Teilprojekt: WAL-Format, Gruppen-Commit, Crash-Recovery, asynchrone Postgres-Nachführung, jeweils isoliert gebaut und getestet, bevor es an den restlichen Stack angeschlossen wird.
8. Erst danach, operation-by-operation, weitere Subsysteme (Swap, Distribution, Guardian, Slashing) auf dieselbe WAL+Shard-Architektur umstellen — jedes einzeln, mit eigener Test-Kampagne. Bis dahin profitieren nur Transfers vom vollen Durchsatzgewinn; das ist ein bewusster Zwischenzustand, kein Fehler im Plan.
9. **Blockproduktion/-relay bei großen Transaktionsmengen pro Block untersuchen und ggf. redesignen** (Abschnitt 7 der Zielarchitektur) — unabhängig von 1–8 messbar (Blockgröße/-serialisierung/-verteilung lässt sich isoliert benchmarken, ohne dass die Storage-Schicht bereits umgebaut sein muss), aber ohne diesen Schritt bleibt unklar, ob die Storage-Schicht ihren Durchsatz überhaupt bis zum Konsens durchreichen kann.

## Teststrategie (nicht verhandelbar für Phase 5+)

- Property-based / randomisierte Concurrency-Tests: viele parallele Goroutinen, zufällige Sender/Empfänger-Paare über mehrere Shards, Prüfung auf **Gesamtsaldo-Erhaltung** (Summe aller Konten vor/nach identisch) und **keine verlorenen Updates** unter harter Last.
- `-race` bei jedem Lauf, ohne Ausnahme.
- Chaos-artige Tests: erzwungene Commit-Fehler, erzwungene Teil-Batch-Fehler, Prozess-Kill-Simulation während eines Flushes — jeweils mit Beweis, dass In-Memory- und DB-Zustand danach wieder konsistent sind.
- **Vor Produktions-Deploy: Staging/Testnet-Lauf über mehrere Tage unter synthetischer Last**, nicht direkter Sprung auf die live laufenden Contabo-Nodes. Diese Umgebung existiert aktuell nicht und müsste als Teil dieses Projekts aufgebaut werden.

## Realistisches Zielbild

Zwei Zwischenstände, ehrlich eingeordnet, keiner davon optimistisch gerundet:

- **Nach Phasen 1–6 (Sharding + Pool-Entkopplung + State-Root-Entkopplung + async EVM-Mirror), Postgres bleibt synchrone Wahrheitsquelle**: niedriger bis mittlerer vierstelliger TPS-Bereich pro Node plausibel — weiterhin begrenzt durch echte Postgres-Round-Trip-Latenz pro Konto-Mutation und die Anzahl gleichzeitig nutzbarer DB-Connections (`max_connections`). **Reicht nicht für das 50.000er-Ziel.**
- **Nach zusätzlich Phase 7 (WAL + In-Memory-primär, Abschnitt 6)**: 50.000 TPS wird ein reales, erreichbares Ziel — die dominante Kostenquelle (Postgres-Netzwerklatenz im synchronen Pfad) ist dann komplett aus dem kritischen Pfad heraus, ersetzt durch sequentielles, lokales WAL-Schreiben (Größenordnung günstiger) plus reine In-Memory-Mutation. Die verbleibenden Begrenzer sind dann: WAL-Fsync-Durchsatz der tatsächlich genutzten Platte, Go-GC-Pausen unter sehr hoher Allokationsrate (muss bei diesem Durchsatz explizit gemessen und ggf. durch Objekt-Pooling entschärft werden), und wie viele Shards die konkrete Hardware (CPU-Kerne) sinnvoll parallel bedienen kann. Keine dieser drei Grenzen ist prinzipiell — alle sind mit sorgfältigem Engineering und Messung adressierbar, aber alle drei müssen im Projekt tatsächlich gemessen werden, nicht nur angenommen.

Kurz: **50.000 TPS ist mit diesem vollständigen Plan (inkl. Phase 7) ein begründetes, kein beliebiges Ziel** — aber nur mit Phase 7, nicht mit Sharding allein.

## Aufwandsschätzung

Kein Wochenend-Projekt. Realistisch: mehrere Wochen fokussierter Arbeit für Phasen 1–6, danach ein eigenes, mindestens ebenso großes Teilprojekt für Phase 7 (WAL/In-Memory-primär) — inklusive der Zeit für den Aufbau einer Staging-Umgebung und die Test-Kampagne (insbesondere Crash-Recovery-Tests für Phase 7, die härtesten in diesem gesamten Plan). Nicht etwas, das in einer fortlaufenden Chat-Session sicher zu Ende gebracht werden sollte — das gilt für Phase 7 noch einmal deutlich stärker als für Phasen 1–6.
