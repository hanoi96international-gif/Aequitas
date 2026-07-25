# Aequitas Scaling-Architektur — Design-Dokument für ein eigenes Projekt

**Status (aktualisiert): Phasen 1–7 (für den Transfer-Pfad) und Phase 9 sind inzwischen implementiert und lokal getestet (`-race`, inkl. Crash-Recovery-Simulation) — siehe Task-Historie und STAGING_RUNBOOK.md.** Die ursprüngliche "NICHT implementieren"-Warnung unten bezog sich auf den Startzustand dieses Dokuments und ist für Phasen 1–7/9 überholt; sie gilt aber unverändert fort für **(a) Phase 8** (Swap/Distribution/Guardian/Slashing auf dieselbe Architektur — nicht begonnen, siehe Migrationsplan Punkt 8 unten für eine neue Erkenntnis, warum das für Swap kein einfaches Kopieren ist) und **(b) die tatsächliche Aktivierung von `AEQUITAS_WAL_ENABLED`/`ENABLE_MULTI_BLOCK_TICK` auf den echten Contabo-Produktionsknoten**, die weiterhin die vollständige, noch nicht durchgeführte Staging-Kampagne aus STAGING_RUNBOOK.md voraussetzt (eigene Infrastruktur, mehrtägiger Lauf — existiert aktuell nicht). Ursprüngliche Begründung für die Vorsicht siehe unten, weiterhin gültig für das, was noch nicht validiert ist.

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

**Update — WAL-Primitive gebaut und isoliert getestet (`x/humanity/wal`), NICHT angeschlossen:**

Genau nach dem in diesem Dokument selbst für Phase 7 vorgeschriebenen Muster ("jeweils isoliert gebaut und getestet, bevor es an den restlichen Stack angeschlossen wird", Migrationsplan Punkt 7) existiert jetzt ein eigenständiges, business-logik-unabhängiges Paket `x/humanity/wal`:
- Append-only Datei-Format mit Sequenznummer + CRC32-Prüfsumme pro Eintrag, Gruppen-Commit (identisches Muster zu `transferBatchCh`/`runTransferBatcher`, nur eine Ebene tiefer).
- `Open` scannt beim (Wieder-)Öffnen die Datei, erkennt einen unvollständigen/korrupten Tail-Eintrag (das exakte Bild eines Absturzes mitten im Schreiben) und schneidet ihn sauber ab, bevor neue Appends zugelassen werden.
- `ReplayFile` liest sequentiell, stoppt am ersten beschädigten/unvollständigen Eintrag, meldet aber keinen Fehler dafür — das ist der erwartete, normale Fall eines Absturzes nach dem letzten bestätigten Eintrag, kein Bug.
- `TruncateBefore` (Kompaktierung) per sicherem Rewrite+Rename, niemals ein halb-geschriebener Zustand sichtbar.
- 12 Tests (inkl. simuliertem Absturz durch Byte-Abschneiden, absichtlicher Bit-Korruption mitten im Datensatz, `-race`-geprüfter Nebenläufigkeit über 2000 gleichzeitige Appends) — alle grün.
- **Real gemessen** (`TestWALThroughput`, `AEQUITAS_WAL_BENCH=1`, in dieser Entwicklungsumgebung — NICHT auf echter Contabo-Produktionshardware, siehe Vorbehalt unten): bei 1000 gleichzeitigen Appends **~112.700 appends/sec** durch das Gruppen-Commit-fsync allein — beantwortet das erste der drei in "Realistisches Zielbild" genannten offenen Unbekannten (WAL-Fsync-Durchsatz) für diese Umgebung positiv: kein Hinweis, dass fsync-Durchsatz der limitierende Faktor wäre.

**Ausdrücklich NICHT gemacht — der eigentliche, viel größere restliche Schritt:**
1. **Nicht an `ChainState` angeschlossen.** `cs.accounts`/`cs.pool` bleiben exakt wie heute; nichts liest oder schreibt das WAL im echten Betrieb.
2. **`cs.activeTx`-Kopplung ungelöst.** Bei der Phase-5-Untersuchung (Shard-Locks für den Transfer-Pfad) stellte sich heraus: `cs.activeTx` ist ein EINZIGES, ChainState-weites `*sql.Tx`-Feld, das `dbExec()` und jeder `saveAccountToDB`-Aufrufer implizit liest — echte Nebenläufigkeit mehrerer gleichzeitiger atomarer Operationen (egal ob über Shard-Locks oder über WAL) ist damit strukturell blockiert, bis dieses Feld durch etwas Operation-lokales ersetzt wird (z. B. explizit durchgereicht oder über `context.Context`). Das ist GENAU die Art von Umbau, den die WAL-Integration ohnehin braucht — Phase 5 und Phase 7 sind also enger gekoppelt, als die Phasennummerierung suggeriert; Phase 5 lässt sich nicht als kleinerer, unabhängiger Schritt VOR Phase 7 sauber abschließen.
3. **Contabo-Produktionshardware nicht gemessen.** Die 112.700 appends/sec sind aus der aktuellen Entwicklungsumgebung (Cloud-Container, Overlay-Dateisystem) — ein reales VPS mit anderer Virtualisierungsschicht/Disk könnte spürbar anders abschneiden. Vor jeder Produktionsentscheidung mit echter Zielhardware nachmessen.
4. Go-GC-Pausen bei sehr hoher Allokationsrate und Shard-Zahl-vs-CPU-Kerne (die anderen beiden in "Realistisches Zielbild" genannten Unbekannten) — beide weiterhin ungemessen.

**Update (2026-07-23) — Live-Aktivierungsversuch auf Contabo2: Tooling-Bug gefunden UND gefixt, WAL kurz echt aktiv, dann wegen eines neuen Befunds bewusst wieder zurückgerollt.** Entgegen der Warnung oben wurde `AEQUITAS_WAL_ENABLED`/`ENABLE_MULTI_BLOCK_TICK` bereits einmal live auf Contabo2 gesetzt (`enable-wal-contabo2.yml`), vor der eigentlich vorgeschriebenen Staging-Kampagne. Root Cause bestätigt: `deploy_safe_c2.sh` übernahm Env-Variablen für einen neuen Container ausschließlich aus `docker inspect` des vorherigen Containers, nie aus `/root/.aequitas.env` — die Flags erreichten den laufenden Prozess deshalb nie. Auf explizite Nutzerfreigabe hin wurde der Fix tatsächlich live angewendet (`deploy_safe_c2.sh` gepatcht, per Backup+Syntaxcheck+atomarem Install, siehe STAGING_RUNBOOK.md für den genauen Ablauf) — danach zeigte der Boot-Log zum ersten Mal echte `[WAL] ✓ WAL fast path active`-Zeilen.

Direkt danach zwei neue, ernste Befunde: **(1) Der `docker run` in `deploy_safe_c2.sh` hat keinen Volume-Mount — die WAL-Datei lebt nur im Container-eigenen, schreibbaren Layer.** Da `deploy-contabo2.yml` bei jedem Push auf `main` automatisch redeployt (frischer Container, `docker rm` löscht den alten vollständig), hätte jede WAL-durable, aber noch nicht nach Postgres geflushte Transaktion beim nächsten Redeploy ersatzlos verloren gehen können — ohne Fehler, ohne Replay-Möglichkeit. Ein Risiko, das dieses Dokument vorher nicht kannte (bisherige Sorgen waren Fsync-Durchsatz/GC-Pausen, nicht "die Datei überlebt die eigene Redeploy-Pipeline nicht"). **(2) Der erste Fix-Versuch (v1) hatte selbst einen Bug:** er merged `.aequitas.env` nur ÜBER den geerbten Container-Env, wodurch ein anschließender Rollback-Versuch (Zeilen aus der Datei entfernen) wirkungslos blieb — der alte Container hatte die Flags noch, also wurden sie weitervererbt. Live bestätigt (Rollback gelaufen, WAL laut Boot-Log immer noch aktiv), dann mit v2 behoben (die drei Flags jetzt komplett vom Vererbungs-Pfad ausgeschlossen, kommen nur noch aus der Datei).

Wegen Befund (1) wurde WAL sofort wieder deaktiviert und über einen Verify-Lauf bestätigt: null `[WAL]`-Zeilen, `ENABLE_MULTI_BLOCK_TICK`-Warnung weg, Höhe steigt normal weiter. Contabo2 lief danach wieder auf dem alten, synchronen Pfad.

**Update (2026-07-23, Fortsetzung) — Persistenz-Lücke geschlossen, WAL + Multi-Block-Tick jetzt real und dauerhaft aktiv.** `deploy_safe_c2.sh` v3 bindet ein dediziertes Host-Verzeichnis (`/root/aequitas-wal-data` → `/data/wal` im Container) — überlebt `docker rm` genauso wie das Image selbst, `AEQUITAS_WAL_PATH` zeigt jetzt standardmäßig dorthin. Live verifiziert mit echtem Beweis, nicht nur Konstruktion: die WAL-Datei erscheint tatsächlich auf dem HOST-Dateisystem (vorher strukturell unmöglich), UND ein zweiter, absichtlich ausgelöster Redeploy-Zyklus (identisch zu einem regulären push-getriggerten Deploy) hat gezeigt, dass die Datei sauber wiedereröffnet wird, ohne Fehler, ohne Neuerstellung von Grund auf. WAL und Multi-Block-Tick laufen jetzt echt, dauerhaft und redeploy-sicher auf Contabo2 — Punkt 1 oben ("nichts liest oder schreibt das WAL im echten Betrieb") gilt für Contabo2 nicht mehr; es ist der erste Node, auf dem diese Phase-7-Architektur tatsächlich lebt. Das ändert NICHTS an der "NOT staging-validated"-Einstufung im Code selbst (die vorgeschriebene mehrtägige Staging-Kampagne fand nicht statt, das ist Live-Produktion) — nur das durch den Tooling-Bug zusätzlich eingeführte Datenverlustrisiko ist geschlossen. Details, exakte Patch-Historie (v1/v2/v3) und der Nebenbefund zu Orphan/Reject-Log-Zeilen während `ENABLE_MULTI_BLOCK_TICK=1`: siehe STAGING_RUNBOOK.md.

Kurz: die WAL-Grundlage selbst ist jetzt real gebaut, hart getestet und (in dieser Umgebung) durchsatzseitig vielversprechend — aber die eigentliche Integration in den lebenden, geldbewegenden Kern von `ChainState` ist der Teil, den dieses Dokument von Anfang an als eigenes, mehrwöchiges Teilprojekt mit dedizierter Staging-Validierung einstuft, und das bleibt unverändert richtig.

### 7. Blockproduktion/-relay: bisher unbetrachtete Engpassstelle OBERHALB der Storage-Schicht

Alle bisherigen Abschnitte optimieren, wie schnell `ChainState` einzelne Transaktionen annehmen und persistieren kann. Sie sagen nichts darüber aus, wie schnell die fertigen Blöcke, in denen diese Transaktionen an andere Nodes weitergereicht werden, selbst verarbeitet werden können — und das ist eine eigene, bisher nicht untersuchte Engpassstelle:

`ProduceBlock` hatte bis zu diesem Fix **keine Obergrenze** für Transaktionen pro Block — jeder Tick (`BLOCK_TIME`, ~1–2 s) drainierte den kompletten wartenden Mempool in EINEN Block. Bei 50.000 TPS wären das 50.000–100.000 Transaktionen in einem einzigen Block.

**Update (Phase 9, real gemessen statt nur angenommen):** `TestBlockCostAtScale` (`AEQUITAS_BLOCK_SIZE_BENCH=1`) misst `calculateBlockHash` (JSON-Marshal der TX-Liste + SHA256 — läuft beim Produzenten UND bei jedem verifizierenden Peer) sowie den vollen Block-Payload (`json.Marshal`/`json.Unmarshal`, wie `p2p.go`s `broadcastExcept` ihn tatsächlich verschickt) bei steigender TX-Zahl:

| TXs/Block | calculateBlockHash | json.Marshal(Block) | json.Unmarshal | Payload |
|---|---|---|---|---|
| 100 | 0,7 ms | 0,4 ms | 0,7 ms | 0,02 MB |
| 1.000 | 1,2 ms | 0,8 ms | 3,6 ms | 0,23 MB |
| 10.000 | 37,6 ms | 8,5 ms | 37,9 ms | 2,32 MB |
| 50.000 | 83,1 ms | 41,0 ms | 191,7 ms | 11,59 MB |
| 100.000 | 148,3 ms | 89,7 ms | 379,1 ms | 23,17 MB |

Bei 50.000 TXs/Block kostet allein Hash-Verifikation + Unmarshal auf der Empfängerseite ~275 ms — ein spürbarer Anteil eines 1–2s-`BLOCK_TIME`-Fensters, VOR jeglicher P2P-Übertragungszeit (11,6 MB Payload) oder der eigentlichen TX-Replay-Kosten. GHOSTDAG selbst ist von dieser Zahl nicht betroffen (operiert auf DAG-/Parent-Struktur, nicht auf TX-Zahl pro Block).

**Umgesetzt:** `maxTxsPerBlock = 20.000` (`evm_storage.go`, `LoadPendingTxs`' SQL bekam ein `LIMIT`) — deckt das konkret gemessene Worst-Case-Risiko (ein pathologisch großer Block) ab, ohne die größere Kadenz-Frage anzufassen. Jede TX über der Grenze bleibt einfach `included_at = 0` und wird beim nächsten Tick abgeholt — nichts geht verloren, nur verzögert (bewiesen durch `TestLoadPendingTxs_CapsAtMaxTxsPerBlock`).

**Ausdrücklich NICHT gelöst:** 20.000 TXs/Block bei ~1–2s `BLOCK_TIME` deckelt den Durchsatz durch Block-Relay selbst auf realistisch 10.000–20.000 TPS — unabhängig davon, wie schnell die Storage-Schicht (Phasen 1–6) TXs aufnehmen kann. Um 50.000 TPS auch durch Block-Relay tatsächlich durchzureichen, braucht es zusätzlich entweder mehrere Blöcke pro Tick oder ein kürzeres `BLOCK_TIME` — eine eigene, größere Kadenz-Entscheidung, bewusst nicht Teil dieses Fixes (siehe Anti-Pattern-Prinzip oben: den konkret gemessenen Risikopunkt zuerst schließen, nicht mit einer größeren Umstellung vermischen).

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
5. **Transfer-Pfad auf Shard-Locks statt `cs.mu` umstellen** — der erste echte Parallelitätsgewinn (erwartet: niedriger bis mittlerer vierstelliger TPS-Bereich, siehe Zielbild unten). Andere Operationen (Swap, Distribution, etc.) bleiben vorerst auf grobem Locking, sind aber durch Schritt 2 bereits kompatibel mit dem neuen Store. **Update:** Detailuntersuchung ergab, dass echte Nebenläufigkeit hier durch `cs.activeTx` (ein einziges, ChainState-weites `*sql.Tx`-Feld) strukturell blockiert ist, bis dieses Feld operation-lokal wird — siehe Punkt 6/Phase 7's "Update"-Absatz. Ein sauberes Shard-Lock-Design (`cs.mu` als `RWMutex`: bestehende `cs.mu.Lock()`-Konsumenten wie `replayTransactions`/Swap/Distribution unverändert lassen, nur der Transfer-Pfad auf `RLock()` + deterministisch geordnete Shard-Locks umstellen, dediziertes Mutex für die `accountSetXOR`-Mutation) ist entworfen und würde die Rollback-/Replay-Garantien unangetastet lassen — aber ohne die `activeTx`-Auflösung bliebe der eigentliche DB-Commit weiterhin serialisiert, sodass der reale Gewinn begrenzt wäre. Nicht implementiert, bis diese Kopplung mit Phase 7 zusammen angegangen wird.
6. **EVM-Mirror-Sync asynchron** (Abschnitt 5 der Zielarchitektur).
7. **WAL + In-Memory-primär** (Abschnitt 6 der Zielarchitektur) — DER Schritt, der auf dem Weg zu 50.000 TPS liegt. Eigenständiges Teilprojekt: WAL-Format, Gruppen-Commit, Crash-Recovery, asynchrone Postgres-Nachführung, jeweils isoliert gebaut und getestet, bevor es an den restlichen Stack angeschlossen wird.
8. Erst danach, operation-by-operation, weitere Subsysteme (Swap, Distribution, Guardian, Slashing) auf dieselbe WAL+Shard-Architektur umstellen — jedes einzeln, mit eigener Test-Kampagne. Bis dahin profitieren nur Transfers vom vollen Durchsatzgewinn; das ist ein bewusster Zwischenzustand, kein Fehler im Plan.

   **Update (Swap-Voruntersuchung, nach Phase-7-Fertigstellung):** Swap (`swapLocked`) unterscheidet sich strukturell von Transfer auf eine Art, die diesen Schritt für Swap NICHT zu einer mechanischen Kopie von `transferConcurrentWAL` macht:
   - Jeder Swap mutiert `cs.pool` — eine EINZIGE globale AMM-Reserve, nicht zwei beliebige Adressen wie bei Transfer. Es gibt keine "disjunkten Swaps", die parallel laufen könnten — jeder Swap braucht dieselbe Ressource. Der Shard-Lock-Parallelitätsgewinn, der bei Transfer den großen Sprung brachte, ist für Swap strukturell nicht verfügbar, unabhängig von WAL.
   - `reloadPoolFromDB()` liest den Pool-Stand synchron aus Postgres vor jeder Swap-Operation (P2-7-Audit-Fix gegen Stale-Memory-AMM-Invariant-Verletzungen). Ein WAL-Fastpath bräuchte eine eigene Recovery-Invariante für den Pool-Zustand (analog `WALSeq` bei Accounts), damit `cs.pool` als primäre In-Memory-Quelle vertrauenswürdig würde — das ist neue Designarbeit, kein Kopieren.
   - Die Fee-Verteilung (`distributeSwapFeeCtx`) ist bereits seit Phase 3 asynchron entkoppelt (`markPoolAccountsDirtyLocked`) — dieser Teil der Round-Trip-Kette ist also schon amortisiert, unabhängig von Phase 8.
   - Fazit: der realistisch erreichbare Gewinn für Swap ist auf "ein bis zwei synchrone Postgres-Round-Trips pro Swap sparen" begrenzt, nicht "neue Parallelität freischalten" — bei gleichzeitig vollem Risiko am geldbewegenden Kern (AMM-Invariante, Wealth-Cap, Demurrage). Da Swap-Volumen in einem UBI-/Demurrage-System voraussichtlich ein kleiner Bruchteil des Gesamtverkehrs ist (dominiert von Mensch-zu-Mensch-Transfers), ist die Priorität dieses Schritts für das 50.000-TPS-Ziel selbst niedriger als die Aufwands-/Risikoeinstufung "eigene Test-Kampagne" bereits andeutet. Nicht begonnen; siehe STAGING_RUNBOOK.md für den ohnehin vorgeschriebenen Weg, bevor irgendein WAL-Fastpath (Transfer eingeschlossen) auf den echten Contabo-Produktionsknoten aktiviert wird.
9. **Blockproduktion/-relay bei großen Transaktionsmengen pro Block untersuchen und ggf. redesignen** (Abschnitt 7 der Zielarchitektur) — unabhängig von 1–8 messbar (Blockgröße/-serialisierung/-verteilung lässt sich isoliert benchmarken, ohne dass die Storage-Schicht bereits umgebaut sein muss), aber ohne diesen Schritt bleibt unklar, ob die Storage-Schicht ihren Durchsatz überhaupt bis zum Konsens durchreichen kann. **Teilweise erledigt**: Kosten real gemessen (`TestBlockCostAtScale`), Worst-Case-Risiko (unbegrenzte Blockgröße) durch `maxTxsPerBlock` geschlossen — die größere Kadenz-Frage (mehrere Blöcke/Tick oder kürzeres `BLOCK_TIME`, nötig um 50k TPS auch durch Block-Relay durchzureichen) bleibt offen.

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

**Update (2026-07-23) — erstmals real gemessen, nicht mehr nur Zielbild.** `TestSimulateMaxTPS_WarmSteadyState` (neu, `tps_bench_test.go`) misst den WAL-Fastpath isoliert von zwei Kosten, die die älteren Cold-Start-Benchmarks (Tabelle oben, 400 TPS) mit hineinmischen: kaltes Konto-Warmup (Fastpath braucht beide Konten bereits warm in `cs.accounts`) und selbstgemachte Shard-Lock-Kontention durch die Ring-Topologie der älteren Benchmarks (Nachbar-Konten teilten sich einen Schreiber, `TryLockAddrs`s Non-Blocking-Charakter wich dann sofort auf den Batcher aus). Mit beidem behoben (Warmup-Pass vor dem Timer, echte disjunkte Sender/Empfänger-Paare):

| Lauf | TXs | TPS |
|---|---|---|
| Cold-Start (Ring, `TestSimulateMaxTPS_IngestionDisjointRecipients`) | 10.000 | 8.596 |
| Warm, Ring-Kontention (1. Fassung von `WarmSteadyState`) | 20.000 | 7.524 (niedriger als Cold-Start — die Ring-Kontention war der bindende Engpass, nicht die Wärme) |
| **Warm, echte disjunkte Paare** | 20.000 | **44.925 / 47.056** (zwei Läufe) |
| **Warm, echte disjunkte Paare, größeres Volumen** | 100.000 | **40.682**, über 2,46s sustained — kein kurzer Burst |

Alle Zahlen: diese Sandbox (Cloud-Container, lokales Postgres, `GOGC=200`), Einzel-Node, kein Netzwerk zu anderen Nodes — dieselben Vorbehalte wie bei jeder Zahl in diesem Dokument (echte Contabo-Hardware nachmessen, siehe unten). Aber zum ersten Mal in diesem Projekt: **eine einzelne, gemessene Zahl in derselben Größenordnung wie das 50.000er-Ziel selbst**, nicht nur eine Projektion aus Fsync-Benchmarks (`TestWALThroughput`, 112.700 appends/sec) und Architektur-Überlegungen. 40.000–47.000 TPS bei disjunkten, warmen Adressen ist nicht 50.000, aber nah genug, dass die verbleibende Lücke eher Feinabstimmung als ein weiterer Architektur-Umbau sein könnte — bei genug unabhängigen Adresspaaren (real: Millionen Nutzer, nicht 100 Test-Paare) sollte die Kollisionsrate über 16.384 Shards ohnehin verschwindend gering bleiben.

**Wichtig, nicht überinterpretieren:**
- **Warm-Voraussetzung ist real, nicht kostenlos.** Ein Konto ist erst nach seinem ersten Touch warm; in Produktion bedeutet das, dass gerade neu aktive/lang inaktive Wallets weiterhin den langsameren Batcher-Pfad durchlaufen. Bei stetigem Verkehr (die meisten aktiven Wallets sind kürzlich warm geworden) ist das die Regel, nicht die Ausnahme — aber ein initialer Traffic-Burst nach einem Neustart (kompletter Kaltstart, `cs.accounts` leer) sähe die niedrigeren Zahlen der älteren Benchmarks, nicht diese.
- **Korrektur einer ersten Fehleinschätzung:** die `[STATE] Batch committed: 1 transfer(s)`-Zeilen im rohen Test-Output stammten, wie sich bei genauerem Hinsehen zeigte, ausschließlich aus dem UNGETIMTEN Warmup-Pass (jedes Konto ist bei seinem allerersten Touch zwangsläufig kalt) — nicht aus dem getimten Lauf selbst. `TransferFastPathStats()` (neu, `state.go`, ein einfacher atomarer Zähler in `TransferAtomic`, vor dem getimten Lauf zurückgesetzt) bestätigt das direkt: **100,0 % der getimten Transfers liefen über den echten Fastpath**, auch bei 300.000 Transfers am Stück — kein Batcher-Fallback im Steady State bei echten disjunkten Adressen.
- **Nach wie vor nicht auf echter Contabo-Hardware gemessen** (siehe Vorbehalt bei den 112.700 appends/sec weiter oben — gilt hier identisch).
- **Blockproduktion/-relay (Abschnitt 7) ist in dieser Zahl nicht enthalten** — das misst nur, wie schnell `ChainState` einzelne eingehende Transfers annehmen kann, nicht wie schnell fertige Blöcke an andere Validatoren durchgereicht werden (dafür siehe `TestBlockCostAtScale`, `maxTxsPerBlock`, `ENABLE_MULTI_BLOCK_TICK` — inzwischen selbst live auf Contabo2, siehe STAGING_RUNBOOK.md, aber bei aktuell winzigem realem Verkehr dort noch nicht unter echter Last erprobt).
- **Go-GC unter sehr hoher Allokationsrate über viele Minuten/Stunden** bleibt ungemessen — dieser Lauf dauert Sekunden, nicht die Dauer, über die GC-Pausen sich typischerweise bemerkbar machen.

**Update — CPU-Profiling bei ~40.000 TPS, ein echter Befund gefunden UND ausdrücklich NICHT behoben.** `AEQUITAS_TPS_CPUPROFILE` gegen `TestSimulateMaxTPS_WarmSteadyState` (300.000 Transfers) zeigt `enqueueWALFlushLocked` (transfer_wal.go) bei 27,8 % der gesamten CPU-Zeit, dominiert von `runtime.growslice`/`memmove` (19,5 % allein) — `cs.walFlushQueue` wächst per `append` ohne Vorab-Kapazität. Ursache: `walFlushMaxBatch=500` alle `walFlushInterval=500ms` drainiert höchstens 1.000 Einträge/Sekunde — weit unter dem jetzt gemessenen Fastpath-Durchsatz. Bei echtem Dauerbetrieb nahe 40.000+ TPS wächst diese Queue unbegrenzt weiter, mit echten Konsequenzen (unbegrenztes Speicherwachstum, immer größer werdende Verzögerung, bis Explorer/andere Validatoren den aktuellen Zustand sehen).

Drei Fixes ausprobiert, alle am selben 300k-Transfer-Benchmark gemessen — **alle drei schnitten schlechter ab** als das unveränderte Original (37.550 TPS Baseline):
- `walFlushMaxBatch` 500→25.000 (Intervall unverändert): **32.483 TPS.** `flushWALBatch` hält `cs.mu.Lock()` (volle Exklusivität, nicht RLock — siehe dessen eigene Begründung, das ist kein Versehen) für die GESAMTE Snapshot-plus-DB-Schreibung — ein größerer Batch bedeutet proportional längere Blockade jedes Fastpath-Senders.
- `walFlushInterval` 500ms→5ms (Batch-Cap moderat): **14.468 TPS.** Jetzt dominiert der Overhead pro Flush-Roundtrip (viele kleine Flushes statt weniger großer).
- Mittelweg 50ms/5.000: **30.004 TPS.** Ebenfalls schlechter.

**Warum das kein Widerspruch ist:** alle drei Fixes lassen den Flush-Worker tatsächlich mit dem echten Durchsatz mitkommen (korrekt, Queue bleibt beschränkt) — auf Kosten von insgesamt mehr Zeit unter `cs.mu.Lock()` während des kurzen (8–20s) Benchmark-Laufs, verglichen mit dem Original, das in dieser kurzen Zeitspanne kaum je zum Flushen kommt (bei 16 Ticks × 500 Items werden von 300.000 Transfers nur ~8.000 je an Postgres reconciled). Der kurze Benchmark belohnt also "nicht rechtzeitig nach Postgres schreiben" (kaum Lock-Kontention) gegenüber dem eigentlichen Sinn des WAL-Designs (rechtzeitige, beschränkte Nachführung) — "höhere Benchmark-TPS" und "der richtige Produktionswert" sind hier NICHT dasselbe, und dieser kurze Benchmark allein kann beides nicht auseinanderhalten.

**Entscheidung (zum damaligen Zeitpunkt):** zurückgesetzt auf die ursprünglichen, bereits getesteten Werte (500/500ms), NICHT auf einen der drei Versuche. Richtiges Tuning braucht eine deutlich längere (Minuten, nicht Sekunden) Sustained-Load-Messung, die die Flush-Queue ihren eigenen echten Steady-State erreichen lässt.

**Update — Runde 2: `TestSustainedWAL_QueueConvergence`, jetzt tatsächlich entschieden UND behoben.** Statt auf die geplante mehrtägige Staging-Kampagne zu warten, wurde ein neuer, permanenter Test (`x/humanity/keeper/wal_sustained_test.go`, opt-in via `AEQUITAS_WAL_SUSTAINED_BENCH=1`) gebaut, der genau das liefert, was Runde 1 fehlte: eine echte 20-Sekunden-Dauerlast (100 disjunkte warme Adresspaare, kontinuierlich, kein Burst) mit `cs.WALFlushQueueDepth()` (neuer Accessor, `transfer_wal.go`) alle 500ms abgetastet, statt nur eine TPS-Endzahl. Drei Konfigurationen, direkt verglichen (erstes Drittel vs. letztes Drittel der Queue-Tiefen-Samples als Konvergenz-Signal):

| Konfiguration | TPS (20s sustained) | Queue-Tiefe: 1. Drittel → letztes Drittel | Signal |
|---|---|---|---|
| Original 500ms/500 | 16.780 | 66.433 → 273.751 | **klettert unbegrenzt weiter — bestätigt, kein Benchmark-Artefakt** |
| **100ms/2.000** | **16.633** (statistisches Rauschen ggü. Original) | **1.329 → 1.740** | **stabil — hält mit echtem Durchsatz mit** |
| 20ms/1.000 | 10.583 (echter Durchsatzverlust) | 109 → 116 | stabil, aber unnötig teuer |

Der entscheidende Unterschied zu Runde 1: bei 20 echten Sekunden Dauerlast zeigt sich, dass das Original-Setting nicht einfach "wenig flusht", sondern bei echtem Sustained-Verkehr tatsächlich nie aufholt — die Queue-Tiefe steigt über den gesamten Lauf nahezu linear weiter (66k→274k), was bei Produktions-Laufzeiten von Stunden/Tagen unweigerlich zu unbegrenztem Speicherwachstum und immer größerer Explorer/Validator-Staleness führt. `100ms/2.000` dagegen erreicht denselben Durchsatz (16.633 vs. 16.780 TPS — kein messbarer Unterschied) bei einer Queue, die sich nach der anfänglichen Auffüllung stabilisiert, statt weiterzuwachsen — zwei Größenordnungen kleiner als das Original am Ende des Laufs. Anders als alle drei Runde-1-Versuche kostet dieser Fix also **keinen messbaren Durchsatz**.

**Entscheidung:** `walFlushInterval`/`walFlushMaxBatch` in `transfer_wal.go` von 500ms/500 auf **100ms/2.000** geändert — dies ist jetzt der produktive Default, kein reiner Untersuchungsbefund mehr. Verifiziert: vollständige `-race`-Testsuite (`keeper` + `wal` Pakete) läuft sauber gegen den neuen Default; `TestSimulateMaxTPS_WarmSteadyState` bei 300.000 Transfers liefert mit dem neuen Default 300.000/300.000 erfolgreich, 0 Fehler, 100 % Fastpath, 17.724,3 TPS sustained. `TestSustainedWAL_QueueConvergence` bleibt dauerhaft im Repo als wiederholbarer Check — der Kommentar in `transfer_wal.go` weist ausdrücklich darauf hin, ihn nach jeder künftigen Änderung in der Nähe dieses Codes erneut laufen zu lassen, da Runde 1 gezeigt hat, dass der kurze Benchmark allein für diesen Trade-off aktiv in die Irre führt.

Noch offen: Messung auf echter Contabo-Hardware unter echtem Sustained-Verkehr (aktuell nur 14 Menschen live, also weit unter der Last, die dieses Problem überhaupt sichtbar macht) — siehe Vorbehalte oben, gelten hier identisch.

**Update — Runde 3: `flushWALBatch`'s `cs.mu.Lock()` verfeinert, zwei neue, tiefere Befunde, NICHTS davon ausgeliefert.** Mit 100ms/2.000 als validiertem Default blieb eine offene Frage: `flushWALBatch` hält `cs.mu.Lock()` (volle Exklusivität) für den GESAMTEN Postgres-Roundtrip, nicht nur den Snapshot — das blockiert bei jedem Flush-Tick den kompletten Fastpath, nicht nur die geflushten Adressen. Analyse ergab: die einzige tatsächlich nötige Garantie ist, den `cs.mu.Lock()`-basierten SLOW Path (Batcher/`transferLocked`) für die Dauer des Flushs auszuschließen — und `cs.mu.RLock()` (statt vollem `Lock()`) reicht dafür bereits aus (Go's `sync.RWMutex` lässt `Lock()` erst zu, wenn kein `RLock()` mehr offen ist, unabhängig davon wie viele/welche Goroutine ihn hält). Der einzige ZUSÄTZLICHE Schutzbedarf: gegen den ANDEREN `cs.mu.RLock()`-Halter (den Fastpath selbst), der beim Flush-Snapshot dieselbe Adresse gleichzeitig mutieren könnte — dafür genügt `cs.accounts.LockAddrs(...)` (dieselbe Shard-Lock-Primitive wie `TryLockAddrs`) nur für die kurze Snapshot-Lesephase, nicht den DB-Write.

Implementiert und mit `-race` verifiziert (u. a. 8× Wiederholung des dedizierten Regressionstests `TestTransferConcurrentWAL_ConcurrentMixedWithSlowPath_PostgresStaysConsistent`, der genau die Race-Klasse prüft, die ursprünglich zum vollen `Lock()` führte — sauber in allen Läufen). Der Durchsatz stieg messbar (`TestSustainedWAL_QueueConvergence`, 100ms/2.000: 16.633 → **19.300 TPS**), aber:

- **Befund A — die Queue wuchs bei ALLEN drei Konfigurationen wieder unbegrenzt**, einschließlich des vorher stabilen 100ms/2.000 (Queue-Tiefe bis 281.973 am Laufende). Ursache: das volle `Lock()` hatte, ohne dass es so geplant war, den Fastpath während jedes Flushs komplett angehalten — das erzeugte eine ZUFÄLLIGE, aber wirksame Backpressure (Input konnte nie schneller laufen als der Flush-Worker ihn periodisch ausbremste). Mit `RLock()` läuft der Fastpath jetzt ungebremst weiter, während der Flush-Worker weiterhin NUR sequentiell, ein Batch nach dem anderen, tatsächlich nach Postgres schreibt — seine eigene Drain-Kapazität hat sich nicht erhöht, nur die Eingangsrate. Die Lock-Verfeinerung allein hätte also genau das Problem wieder geöffnet, das die 100ms/2.000-Abstimmung eigentlich geschlossen hatte.
- **Befund B — Fix-Versuch für Befund A (expliziter Queue-Tiefen-Deckel `walFlushMaxQueueDepth=20.000`, oberhalb dessen neue Transfers statt in den Fastpath in den bestehenden, synchronen Batcher fallen) deckte einen dritten, unabhängigen, ernsteren Befund auf:** unter der dadurch erzeugten Last (60-90 % des gesamten Verkehrs plötzlich gleichzeitig über den Batcher, während der WAL-Flush-Worker weiterhin parallel eigene Postgres-Schreibungen macht) trat wiederholt ein ECHTES Postgres-Deadlock auf (`pq: deadlock detected (40P01)`) im Batcher-eigenen `saveAccountsToDBBatchCtx`-Pfad — kein Go-Level-Race, sondern ein waffenechter SQL-Lock-Ordering-Konflikt zwischen dem Flush-Workers eigenem Multi-Row-UPSERT und dem Batchers eigenem Batch-Save-UPSERT, wenn beide unter genug gleichzeitiger Last überlappende Zeilen in unterschiedlicher Reihenfolge sperren. Ergebnis: Durchsatz brach auf 2.500–3.800 TPS ein (schlechter als vorher UND schlechter als der ursprüngliche Batcher-Pfad allein), UND es gab echte, dem Aufrufer sichtbare Fehler (777 von ~60.000 Versuchen) — nicht nur Verzögerung.

**Entscheidung (zum damaligen Zeitpunkt): beide Änderungen (Lock-Verfeinerung UND Queue-Deckel) vollständig zurückgesetzt.** Zwei neue, echte Probleme, die selbst weitere eigene Untersuchung/Härtung brauchten, bevor irgendetwas davon auch nur in Erwägung gezogen werden sollte, geschweige denn an ein laufendes, geldbewegendes Live-Ledger.

**Update — Runde 4: die eigentliche Ursache gefunden, korrekt gefixt, diesmal erfolgreich verifiziert.** Der entscheidende Hinweis stand bereits im Code: `transfer_batch_concurrent.go`s eigener Kommentar zu `processTransferBatchConcurrent` ("SAFETY ARGUMENT") dokumentiert exakt dasselbe Muster aus einem FRÜHEREN, separaten Versuch dieser Session, den WAL-Flush vom globalen Lock zu entkoppeln (damals bei 500 gleichzeitigen Sendern mit bis zu 23,6 % Fehlerrate gemessen, revertiert) — mit der bereits damals dokumentierten Lösung: `cs.accounts.LockAddrs(...)` muss für die GESAMTE Postgres-Transaktion gehalten werden, nicht nur für die Snapshot-Lesephase. Runde 3s Fehler war genau das: `unlockAddrs()` wurde direkt nach dem Snapshot aufgerufen, VOR `tx.Begin()` — dieselbe Falle wie beim damaligen Versuch, nur neu begangen.

Der Grund, warum das der eigentliche Fix ist (nicht nur eine Variante der Zeilen-Sortierung aus Runde 3): sobald `flushWALBatch` seine Shard-Locks für ALLE berührten Adressen über die komplette Transaktion hält — exakt wie `processTransferBatchConcurrent`'s eigener, bereits bewiesener Sicherheitsbeweis es für jeden `cs.mu.RLock()`-basierten Postgres-Schreiber in diesem Code vorschreibt —, sind die von zwei gleichzeitigen Schreibern berührten Zeilenmengen PER KONSTRUKTION disjunkt, bevor überhaupt eine SQL-Query an Postgres geht. Ein Postgres-Deadlock zwischen ihnen wird dadurch strukturell unmöglich, unabhängig davon, welchen Ausführungsplan Postgres intern wählt — anders als die Zeilen-Sortierung (weiterhin im Code, als zusätzliche Verteidigungsebene für den davon unabhängigen, weiterhin bestehenden Batcher-vs-Batcher-Fall über `parallelBatchPoolSize=4`), die sich als messbar hilfreich, aber NICHT ausreichend erwiesen hatte (Deadlock-Rate sank, verschwand aber nicht: 371 von ~162.000 Versuchen bei einem Testlauf).

**Ergebnis, zweifach unabhängig reproduziert (frische Datenbank, jeweils vollständiger `-race`-Lauf davor):**

| Konfiguration | TPS (20s sustained, 100 hämmernde Paare) | Fehlgeschlagene Transfers | Queue-Tiefe | Fastpath-Anteil |
|---|---|---|---|---|
| original-500ms-500 | 10.584–11.871 | **0** | stabil bei Deckel (20.000) | 16,7–18,7 % (Rest: Batcher) |
| **100ms-2.000** | **14.214–17.360** | **0** | stabil bei/unter Deckel | 95,8–96,1 % |
| 20ms-1.000 | 13.960–14.217 | **0** | stabil, niedriger | 93,0–93,3 % |

Alle drei Konfigurationen: **null fehlgeschlagene Transfers** über beide Läufe (vorher: 215–985 je nach Versuch), Queue-Tiefe durchgehend beschränkt (durch `walFlushMaxQueueDepth=20.000`), kein einziges `pq: deadlock detected` mehr in irgendeinem Lauf. Der Durchsatz bei 100ms/2.000 liegt in derselben Größenordnung wie der bisherige Bestwert (16.633–19.300 TPS), jetzt aber mit einer Queue, die auch unter dieser künstlich konzentrierten 100-Paar-Dauerlast (deutlich härter als reale, breit gestreute Adressverteilung) nie unbegrenzt wächst, UND ohne die neu eingeführten Ausfälle aus Runde 3.

Zusätzlich verifiziert: dedizierter Regressionstest `TestTransferConcurrentWAL_ConcurrentMixedWithSlowPath_PostgresStaysConsistent` 8× unter `-race` sauber; `TestTransferConcurrent_ConcurrentRingTransfersConserveBalance`, `TestTransferConcurrent_ViaTransferAtomic_RingConservesBalance`, `TestTransferConcurrent_ConcurrentOverlappingTransfersNoDeadlockNoCorruption` (alle drei nutzen `processTransferBatchConcurrent`/`saveAccountsToDBBatchCtx`) je 3× unter `-race` sauber; vollständige Standard-`-race`-Suite (`keeper`+`wal`, ohne `DATABASE_URL`, wie durchgehend in dieser Session als Gate verwendet) mehrfach sauber.

**Entscheidung: alle drei Teile (Lock-Verfeinerung mit korrekter Shard-Lock-Dauer, Queue-Deckel, Zeilen-Sortierung als zusätzliche Verteidigungsebene) ausgeliefert.** `flushWALBatch` hält jetzt `cs.mu.RLock()` + `cs.accounts.LockAddrs(...)` für die komplette Funktion (nicht mehr `cs.mu.Lock()`); `walFlushMaxQueueDepth=20.000` begrenzt die Queue explizit; `saveAccountsToDBBatchCtx` sortiert seine Zeilen deterministisch. Zusammengenommen: der Fastpath wird während eines Flushs nicht mehr komplett angehalten (nur noch für die spezifischen, gerade geflushten Adressen — bei Millionen echten Nutzern ein verschwindend kleiner Anteil, siehe `numAccountShards`-Kommentar), die Queue kann nicht mehr unbegrenzt wachsen, und die Interaktion mit dem Batcher-Pfad ist beweisbar deadlock-frei statt nur "meistens".

**Weiterhin offen:** Messung auf echter Contabo-Hardware; die künstliche 100-Paar-Testtopologie erzwingt mehr Adress-Überlappung als eine echte, breit gestreute Nutzerbasis hätte (gilt wie immer in diesem Dokument) — der reale Effekt der jetzt viel kürzeren Fastpath-Blockade dürfte in Produktion daher eher GRÖSSER ausfallen als in diesem Test, nicht kleiner.

**Update — Runde 5: Flush-Worker selbst parallelisiert (`walFlushConcurrency=4`), A/B-gemessen statt nur theoretisch begründet.** Nach Runde 4 blieb der Flush-Worker (`runWALFlushWorker`) strikt sequentiell: EIN Tick, EIN Flush, warten auf den vollständigen Postgres-Roundtrip, erst dann der nächste. Da `flushWALBatch` seit Runde 4 nicht mehr `cs.mu`'s volle Exklusivität braucht, sondern nur noch die eigenen Shard-Locks über die Dauer der Transaktion hält, steht nichts mehr im Weg, MEHRERE Flushes gleichzeitig laufen zu lassen — zwei gleichzeitige Flushes berühren entweder disjunkte Adressen (kein Konflikt, beide laufen echt parallel) oder überlappen auf einer Adresse (der zweite wartet einfach am Shard-Lock des ersten, weiterhin korrekt dank des `wal_seq`-Monotonie-Guards). Implementiert über einen begrenzten Semaphor (`walFlushSem`, Kapazität 4 — derselbe bereits bewährte Wert wie `parallelBatchPoolSize`), der Ticker selbst blockiert dabei nicht mehr auf einen laufenden Flush.

**A/B-Messung, nicht nur Architektur-Argumentation:** bei der Standard-Testtopologie (100 Paare, 200 Adressen) zeigte sich KEIN klarer Gewinn (Concurrency=1 vs. 4: beide im selben Rausch-Band, ~14.000–17.000 TPS) — plausibel, da bei nur 200 Adressen gleichzeitige Flushes häufig auf denselben Shard-Lock treffen und sich gegenseitig blockieren, was die Parallelität faktisch aufhebt. Eine fairere Messung mit testweise 1.000 Paaren (2.000 Adressen — immer noch winzig gegenüber einer echten Nutzerbasis, aber genug, um den Effekt sichtbar zu machen) bei 100ms/2.000 zeigte den erwarteten Effekt deutlich: Concurrency=1 → **20.989 TPS bei 69,1 % Fastpath-Anteil**; Concurrency=4 → **22.800 TPS bei 89,2 % Fastpath-Anteil** — nicht nur mehr Durchsatz, sondern auch spürbar weniger Ausweichen auf den langsameren Batcher-Pfad, weil die Queue schneller abgebaut wird. Null fehlgeschlagene Transfers in JEDER getesteten Konfiguration, in beiden Topologien.

**Verifiziert:** volle `-race`-Suite (`keeper`+`wal`) mehrfach sauber; alle WAL-/Batch-Concurrent-Regressionstests (inkl. Crash-Recovery, die den Flush-Worker-Lifecycle direkt nutzt) je 3× unter `-race` sauber; `stopWALFlushWorkerForTest` wartet jetzt zusätzlich auf eine `sync.WaitGroup`, damit bei parallel laufenden Flushes kein Goroutine mehr über das Ende eines Tests hinaus in der geteilten Test-Datenbank schreibt (dieselbe Klasse von Cross-Test-Korruption, die diese Funktion schon vorher verhindern sollte — jetzt für den Mehrfach-Flush-Fall erweitert).

**Entscheidung:** ausgeliefert. Anders als eine rein theoretisch bessere Architektur ohne Messung (die dieses Dokument an mehreren Stellen bewusst NICHT ausliefert) steht hier eine echte, reproduzierte A/B-Messung dahinter, die zeigt: kein Nachteil im pessimistischen Fall, echter Gewinn sobald die Adressmenge über die künstliche Testgröße hinausgeht — und eine reale Nutzerbasis liegt bei Weitem darüber.

**Update — Runde 6: Sandbox-Umgebung wechselte mitten in der Session (Container-Neustart), zweite der drei in "Realistisches Zielbild" genannten offenen Unbekannten (Go-GC-Pausen) jetzt gemessen.**

**Wichtig für die Einordnung aller Zahlen ab hier:** die Entwicklungsumgebung dieser Session wurde zwischenzeitlich neu gestartet (Postgres war down, `pg_ctlcluster` mit "Removed stale pid file" beim Neustart, `uptime` zeigte danach nur wenige Minuten). Direkt danach maß derselbe, unveränderte Nicht-WAL-Fastpath-Benchmark (`TestSimulateMaxTPS_WarmSteadyState`, kein `AEQUITAS_WAL_ENABLED`) nur noch ~3.700–3.850 TPS statt der früher in dieser Session dokumentierten 44.925–47.056 TPS — reproduzierbar über 4 Wiederholungen, kein Aufwärm-Effekt. Ein CPU-Profil dieses Laufs zeigt 51 % der Zeit in rohen Syscalls und nur 61,75 % CPU-Auslastung insgesamt (der Prozess wartet die meiste Zeit) — ein Hinweis auf einen langsameren Postgres-Netzwerk-/Syscall-Pfad in diesem konkreten, frisch gestarteten Container, nicht auf eine Code-Regression (an `transfer_concurrent.go`, das dieser Benchmark ausschließlich durchläuft, wurde in dieser Session nichts geändert). Der WAL-Pfad (`AEQUITAS_WAL_ENABLED=1`) blieb im selben Container bei ~7.300–7.556 TPS für denselben Kurzlast-Test — etwa doppelt so schnell wie der Nicht-WAL-Pfad, was die Kernaussage von Phase 7 (Postgres aus dem synchronen Pfad nehmen hilft besonders dann, wenn Postgres selbst langsam ist) direkt bestätigt, aber ebenfalls weit unter dem früheren ~44k-Wert.

**Schlussfolgerung:** absolute TPS-Zahlen aus dieser Sandbox sind NICHT über einen Container-Neustart hinweg vergleichbar — die zugrundeliegende Hardware/Virtualisierung einer ephemeren Session kann sich unterscheiden. Alle A/B-Vergleiche in dieser Session (Runde 3–5) bleiben gültig, weil sie jeweils INNERHALB desselben Laufs/Containers gemessen wurden (Vorher/Nachher unter identischen Bedingungen), nicht über einen Neustart hinweg. Für absolute Werte gilt weiterhin unverändert: nur eine Messung auf echter Contabo-Zielhardware ist aussagekräftig, siehe alle Vorbehalte oben.

**Go-GC-Pausen, jetzt tatsächlich gemessen (`GODEBUG=gctrace=1`) statt nur als offene Frage markiert:** während `TestSustainedWAL_QueueConvergence` (100ms/2.000-Konfiguration, wachsender Heap bis in den zweistelligen MB-Bereich) liegen die Stop-the-World-Pausen (Mark-Start + Mark-Termination, die einzigen Phasen, die die Anwendung wirklich anhalten) durchgehend im Bereich 0,02–0,24 ms, ein einzelner Ausreißer bei 3,2 ms. Die "concurrente Mark"-Phase (5–25 ms) läuft parallel zur Anwendung und blockiert keine Goroutine direkt. Bei einem `walFlushInterval` von 100 ms und einem `BLOCK_TIME` im Sekundenbereich sind das keine spürbaren Pausen — **GC ist bei diesem Durchsatz und dieser Heap-Größe kein Engpass**, Objekt-Pooling wäre für die aktuell gemessene Größenordnung keine sinnvolle Investition. Damit sind zwei der drei in "Realistisches Zielbild" genannten offenen Unbekannten jetzt real gemessen (WAL-Fsync-Durchsatz: siehe oben, 112.700–181.425 appends/sec je nach Nebenläufigkeit; GC-Pausen: siehe hier) — nur "wie viele Shards die konkrete Zielhardware sinnvoll parallel bedienen kann" bleibt ungemessen, weil das echte Contabo-Hardware statt dieser Sandbox braucht.

**Update — Runde 7: `walFlushMaxBatch` weiter erhöht (2.000 → 4.000), `walFlushConcurrency` bewusst NICHT weiter erhöht — beide Entscheidungen mit klarem Zahlenbeweis für unterschiedliche Adressraum-Größen.** Da der Postgres-Connection-Pool bei Runde 5/6 noch nicht ausgereizt war (WAL-Flush nutzt bis zu 4, Batcher bis zu 4, `MaxOpenConns=20`), lag nahe, sowohl Batch-Größe als auch Nebenläufigkeit weiter hochzudrehen. Ergebnis, A/B-gemessen an ZWEI Adressraum-Größen (100 Paare/200 Adressen — Pakets eigene Standard-Testtopologie, absichtlich adversarial konzentriert; 1.000 Paare/2.000 Adressen — realistischer, aber immer noch winzig gegenüber echten Nutzerzahlen):

| Änderung | Große Topologie (1.000 Paare) | Kleine Topologie (100 Paare) |
|---|---|---|
| `walFlushMaxBatch` 2.000→4.000 (Concurrency bleibt 4) | 23.340 → **30.869 TPS** (+32 %) | 11.999 → 10.321 TPS (**-14 %**) |
| `walFlushConcurrency` 4→16 (Batch bleibt 4.000) | 30.869 → **34.560 TPS** (weiterer Gewinn) | 11.999 → **8.558 TPS** (**-29 %**) |

Beide Parameter helfen bei einem großen, realistischen Adressraum spürbar — aber nur die Batch-Größe hat bei der kleinen, konzentrierten Topologie einen MODERATEN Preis (-14 %); höhere Nebenläufigkeit hat dort einen deutlich GRÖSSEREN Preis (-25 bis -29 %), weil bei nur 200 Adressen mehrere gleichzeitige Flushes fast immer um dieselben Shard-Locks konkurrieren (ein Batch von 4.000 Einträgen deckt bei nur 200 Adressen praktisch den gesamten Adressraum ab) und sich dadurch gegenseitig ausbremsen, statt echt parallel zu laufen.

**Entscheidung:** `walFlushMaxBatch=4.000` ausgeliefert, `walFlushConcurrency` bewusst bei **4** belassen (nicht erhöht). Begründung, konsistent mit der bereits mehrfach in diesem Dokument angewendeten Vorsicht: das reale Contabo2 hat aktuell nur 14 Menschen — sporadischer, nicht konzentriert-hämmernder Verkehr, also näher an der kleinen als an der großen Testtopologie. Der moderate Kostenpunkt der Batch-Erhöhung (-14 %) ist bei diesem winzigen realen Verkehr komplett folgenlos, der Gewinn (+32 %) zahlt direkt auf das 50k-Ziel ein, sobald der Verkehr wächst. Der deutlich größere Kostenpunkt einer Concurrency-Erhöhung (-25 bis -29 %) wäre dagegen ein echter, spürbarer Nachteil für den JETZIGEN, realen Zustand — für einen Gewinn, der erst relevant wird, wenn die Nutzerzahl deutlich wächst. Beide Parameter sollten gemeinsam neu bewertet werden, sobald Contabo2s tatsächliche aktive Adressanzahl die kleine Testtopologie überholt hat.

Verifiziert: volle `-race`-Suite (`keeper`+`wal`) sauber; WAL-/Batch-Concurrent-Regressionstests 3× unter `-race` sauber; finaler `TestSustainedWAL_QueueConvergence`-Lauf mit allen vier Konfigurationen (inkl. der neuen `100ms-4000`) — alle vier PASS, null fehlgeschlagene Transfers, Queue-Tiefe in jedem Fall durch `walFlushMaxQueueDepth` beschränkt.

**Update — Runde 8: der ursprüngliche Round-1-CPU-Befund (`enqueueWALFlushLocked`/`growslice`) war nach den vorherigen Runden noch nicht vollständig behoben — jetzt geschlossen.** `flushWALQueue` gab beim Draining bislang immer `cs.walFlushQueue[n:]` zurück — Go-Slices verlieren dabei Kapazität am Anfang (`cap(s[n:]) = cap(s) - n`), sodass der freie Spielraum der Queue mit JEDEM Drain-Zyklus etwas kleiner wird, selbst wenn die Queue selbst dank `walFlushMaxQueueDepth` längst beschränkt ist. Ein frisches CPU-Profil (`TestSustainedWAL_QueueConvergence`, 100ms/4.000-Konfiguration) bestätigte: `enqueueWALFlushLocked` verursacht weiterhin über die Hälfte aller `runtime.growslice`-Zeit — derselbe Befund wie in Runde 1, nur diesmal an einer strukturell anderen Ursache (Kapazitätserosion durch wiederholtes Slicing, nicht unbegrenztes Wachstum, das ja bereits gelöst ist).

**Fix:** `flushWALQueue` kompaktiert die Queue jetzt NUR dann (kopiert den Rest in ein frisches Array mit echtem Spielraum), wenn die verbleibende Kapazität unter eine Batch-Größe fällt — im Normalfall (genug Spielraum vorhanden) bleibt das Verhalten exakt wie zuvor (reines Slicing, keine Zusatzkosten). Ein Korrektheits-Hinweis war nötig: der bestehende Fehlerfall-Pfad (`append(batch, cs.walFlushQueue...)` bei fehlgeschlagenem Flush) verlässt sich auf das genaue Aliasing-Verhalten von `batch`/`cs.walFlushQueue` — Analyse ergab, dies bleibt korrekt (Go's `append`-Semantik rekonstruiert das richtige Ergebnis sowohl bei einem harmlosen In-Place-Self-Copy als auch bei einem echten Cross-Array-Copy), unabhängig davon, ob zwischenzeitlich kompaktiert wurde.

**Gemessen:** dasselbe CPU-Profil nach dem Fix zeigt `runtime.growslice` von 7,93 % auf 2,14 % kumulativer Zeit gesunken, `enqueueWALFlushLocked` von 5,80 % auf 3,75 %. Verifiziert: volle `-race`-Suite sauber, `TestFlushWALBatch_FailureRequeuesForRetry` (prüft genau den beschriebenen Fehlerfall-Pfad) 8× unter `-race` sauber, alle WAL-/Batch-Concurrent-Regressionstests 3× unter `-race` sauber.

**Update — Runde 9: zweiter, unabhängiger CPU-Profil-Befund gefunden und behoben — Contabo2s Instanz begann in dieser Session-Runde, bei praktisch jedem Werkzeug-Aufruf neu zu starten, sodass weitere Sustained-Load-A/B-Messungen nicht mehr sinnvoll möglich waren. Stattdessen: gezielte Weiteranalyse des bereits vorhandenen CPU-Profils (Runde 8) auf weitere, umgebungsunabhängige Befunde.** `transferConcurrentWAL`s eigene Line-Level-Aufschlüsselung (`go tool pprof -list`) zeigte `markEVMMirrorDirtyForAddrsLocked`/`markEVMMirrorDirtyLocked` bei 280ms kumulativ (5,00 % der Gesamtzeit) — mehr als `enqueueWALFlushLocked`. Ursache: `markEVMMirrorDirtyLocked(contractAddr string, ...)` ruft `strings.ToLower(contractAddr)` bei JEDEM Aufruf auf — und jeder Fastpath-Transfer (WAL wie Nicht-WAL) ruft das mit derselben, unveränderlichen Konstante `V7_CONTRACT_ADDR` auf, die dabei jedes Mal neu klein geschrieben wird, obwohl das Ergebnis immer identisch ist.

**Fix:** `v7ContractAddrLower` als paketweite, einmalig bei Programmstart berechnete Variable (`evm_v6mirror.go`), an den drei betroffenen Aufrufstellen (`register_concurrent.go`, `transfer_concurrent.go`s `markEVMMirrorDirtyForAddrsLocked`) anstelle der rohen Konstante übergeben. `markEVMMirrorDirtyLocked` selbst bleibt unverändert (der einzige andere Aufrufer, `evm_storage.go`, übergibt eine echte, dynamische Contract-Adresse, die weiterhin real klein geschrieben werden muss) — `strings.ToLower`s eigener Fastpath (kein Allokieren, nur ein kurzer Scan, wenn nichts umgeschrieben werden muss) übernimmt den Rest praktisch kostenlos.

**Gemessen:** frisches CPU-Profil nach dem Fix zeigt `markEVMMirrorDirtyLocked` von 280ms/5,00 % auf 140ms/2,31 % gesunken — die `strings.ToLower(contractAddr)`-Zeile allein von 60ms auf 10ms. Verifiziert: volle `-race`-Suite sauber; EVM-Mirror-Flush-Tests (`TestSyncBalanceLocked_DefersEVMMirrorWrite`, `TestEVMMirrorFlush_*`) 3× unter `-race` sauber; Registrierungs-Tests (`TestRegisterHumanConcurrent_*`) 2× unter `-race` sauber.

**Update — Runde 10: `MaxOpenConns` probeweise erhöht (20→40) — klarer negativer Befund, nicht ausgeliefert.** Runde 7 hatte den Postgres-Connection-Pool (`MaxOpenConns=20`, geteilt zwischen WAL-Flush-Worker und Batcher) als plausible Erklärung dafür identifiziert, warum `walFlushConcurrency` über 16 hinaus keinen weiteren Gewinn mehr brachte (34.560 TPS bei 16, 33.983 bei 20 — flach). Naheliegender nächster Test: den Pool selbst vergrößern.

Direkter A/B-Vergleich in DERSELBEN Sandbox-Session, gleiche Konfiguration (`walFlushConcurrency=16`, `walFlushMaxBatch=4.000`, 1.000-Paar-Topologie), nur `MaxOpenConns`/`MaxIdleConns` verändert:

| `MaxOpenConns` | TPS |
|---|---|
| 20 (aktueller Wert) | **35.340** |
| 40 | 29.926 |

Die Erhöhung brachte **keinen** Gewinn — im direkten Vergleich sogar einen scheinbaren Rückgang (im Rahmen der für diese Sandbox bereits mehrfach dokumentierten Lauf-zu-Lauf-Varianz, aber jedenfalls klar KEIN Beweis für einen Nutzen). Das bestätigt die ursprüngliche, bereits in `state.go`s eigenem Kommentar festgehaltene Vermutung: die Grenze bei ~16-20 ist in dieser Sandbox (4 CPU-Kerne) eher CPU-/Scheduling-gebunden als Connection-Pool-gebunden — mehr Verbindungen helfen nicht, wenn schlicht nicht genug Kerne da sind, um die zusätzliche Nebenläufigkeit tatsächlich zu nutzen.

**Entscheidung: nicht ausgeliefert, vollständig zurückgesetzt.** Zusätzlich unabhängig vom Messergebnis gültig: der bereits ausgelieferte `walFlushConcurrency=4` (Runde 7, wegen der Kleinraum-Regression bewusst nicht erhöht) hätte von einem größeren Pool ohnehin nicht profitiert — die Änderung hätte also so oder so keinen Effekt auf den tatsächlich produktiven Code gehabt. `MaxOpenConns=20` bleibt zusätzlich aus einem von der reinen Performance unabhängigen Grund richtig: mehr Verbindungen für einen einzelnen Node bedeuten weniger Spielraum für andere legitime Postgres-Nutzer (Migrations-Tools, Monitoring, ein zweiter Node während eines Rolling-Restarts) — ein Punkt, den `state.go`s eigener historischer Kommentar bereits vor dieser Session festhielt.

## Aufwandsschätzung

Kein Wochenend-Projekt. Realistisch: mehrere Wochen fokussierter Arbeit für Phasen 1–6, danach ein eigenes, mindestens ebenso großes Teilprojekt für Phase 7 (WAL/In-Memory-primär) — inklusive der Zeit für den Aufbau einer Staging-Umgebung und die Test-Kampagne (insbesondere Crash-Recovery-Tests für Phase 7, die härtesten in diesem gesamten Plan). Nicht etwas, das in einer fortlaufenden Chat-Session sicher zu Ende gebracht werden sollte — das gilt für Phase 7 noch einmal deutlich stärker als für Phasen 1–6.

## Update — Contabo-Produktionshardware erstmals real gemessen (2026-07-25): 6 CPU-Kerne, nicht 1, nicht 8

Die in "Realistisches Zielbild" (Runde 10) als letzte offene Unbekannte benannte Frage — "wie viele Shards die konkrete Zielhardware sinnvoll parallel bedienen kann", bislang nie auf echter Contabo-Hardware gemessen, weil die Sandbox kein ausgehendes SSH erlaubt (siehe STAGING_RUNBOOK.md) — ist jetzt teilweise beantwortet: über einen `workflow_dispatch`-Lauf von `diagnose-resources-contabos.yml` (GitHub-Actions-Runner, nicht die Sandbox selbst, macht den SSH-Zugriff möglich) liegt zum ersten Mal ein echter Live-Schnappschuss beider Produktionsknoten vor.

**Contabo1 (173.249.37.118) und Contabo2 (194.163.188.71), beide identisch:**

| Metrik | Contabo1 | Contabo2 |
|---|---|---|
| CPU-Kerne (`iostat`: `(6 CPU)`) | **6** | **6** |
| RAM gesamt | 11.960 MiB (~12 GB) | 11.960 MiB (~12 GB) |
| RAM frei | 10.170 MiB | 10.293 MiB |
| Swap | 0 (kein Swap konfiguriert) | 0 (kein Swap konfiguriert) |
| Load average (1/5/15 min) | 0.00 / 0.04 / 0.12 | 0.16 / 0.08 / 0.08 |
| `aequitas-node` CPU-Last | 9,1–16,6 % | 14,8–16,7 % |
| Uptime | 24 Tage | 21 Tage |
| Laufende Container | `aequitas-node`, `aequitas-postgres`, `proof-server`, `proof-caddy`, `proof-postgres` | dieselben fünf |

**Damit ist die im Chat aufgeworfene Frage ("wieso 1 Kern, wenn der VPS 8 hat") für DIESES Projekt geklärt, auch wenn ihr ursprünglicher Anlass ein anderer, nicht mit diesem Repo verwandter Kontext war:** weder 1 noch 8 — Contabo1 und Contabo2 haben **6 physische/virtuelle Kerne**, beide aktuell zu über 85 % idle (die realen 14 Nutzer erzeugen praktisch keine Last). Zusätzlich bestätigt, direkt aus `deploy_safe_c2.sh` (Volltext über `patch-deploy-safe-c2.yml` bereits einmal offengelegt): der `docker run`-Aufruf, der `aequitas-node` startet, setzt **weder `--cpus` noch `--cpuset-cpus` noch `-m`/`--memory`** — der Container ist von Docker aus nicht auf einen Bruchteil der Maschine eingeschränkt, Go's Laufzeitumgebung sieht (mangels explizitem `GOMAXPROCS`, das an keiner Stelle in diesem Repo gesetzt wird) via `runtime.NumCPU()` alle 6 Kerne und schedult Goroutinen entsprechend automatisch über alle sechs.

**Einordnung gegenüber den bisherigen Sandbox-Zahlen:** die A/B-Messungen in diesem Dokument (Runden 1–10) liefen alle in einer Cloud-Sandbox mit **4 CPU-Kernen** (explizit in Runde 10 benannt) — Contabo hat also 50 % mehr Kerne als die Umgebung, in der `walFlushConcurrency=4`, `parallelBatchPoolSize=4` und `MaxOpenConns=20` empirisch gefunden wurden. Das sind plausible, aber nicht bewiesene Defaults für 6 Kerne — Runde 10s eigener Befund ("mehr Verbindungen halfen nicht, weil schlicht nicht genug Kerne da waren, um sie zu nutzen") legt nahe, dass auf 6 statt 4 Kernen etwas mehr Nebenläufigkeit (z. B. `walFlushConcurrency=6`) einen weiteren, bisher ungemessenen Gewinn bringen könnte. Das ist jetzt **tatsächlich direkt auf Contabo messbar** (SSH funktioniert über GitHub Actions, s.o.) statt nur in der Sandbox extrapolierbar — bisher aber noch nicht getan; siehe Abschnitt "Nächste Schritte" unten.

**Nicht in diesem Lauf beantwortet:** die Postgres-Verbindungsdiagnose (`SELECT count(*) FROM pg_stat_activity`, `SHOW max_connections`) schlug in demselben Workflow-Lauf mit `Process exited with status 1` fehl, auf beiden Boxen, direkt nach der Kopfzeile — die Ursache ist vermutlich dieselbe Klasse von drone-ssh-Eigenheit, die `patch-deploy-safe-c2.yml`s eigener Kommentar bereits dokumentiert (das SSH-Backend hängt nach JEDER Zeile eine eigene Exit-Code-Prüfung an). Reale Postgres-`max_connections`/aktive Verbindungszahl auf Contabo bleibt damit offen — der Workflow-Schritt braucht einen eigenen Fix (z. B. die drei `docker run postgres:16-alpine psql ...`-Aufrufe in ein einzelnes, in sich geschlossenes `bash -c '...'` verpacken), bevor er zuverlässig läuft.

## Update — konkret gefundenes, noch nicht gehobenes Verbesserungspotenzial (2026-07-25, Code-Review ohne Codeänderung)

Vier neue, konkrete Befunde aus einer gezielten Durchsicht des RPC-Hot-Path und der Nebenläufigkeitsgrenzen, keiner davon bisher umgesetzt oder gemessen — aufgeführt, damit sie nicht erneut mühsam wiederentdeckt werden müssen:

1. **gzip komprimiert auch `/rpc`-Antworten.** `gzipMiddleware` (`api.go:571`) schließt `/api/events` (SSE) und `/download/` explizit aus, aber NICHT `/rpc` — jede `eth_sendRawTransaction`-Antwort ist ein winziges JSON-Objekt (~80–150 Bytes: Hash + ID), für das eine neue `gzip.Writer`-Instanz pro Request eine reine CPU-Kosten ohne Nutzen ist (der Overhead von gzip's eigenem Header/Trailer kann bei so kleinen Payloads die unkomprimierte Größe sogar übersteigen). Bei 50.000 Requests/s ist das 50.000 unnötige Kompressionsläufe/s auf genau der Maschine, deren CPU-Budget für ECDSA-Recovery und Shard-Locking gebraucht wird. Fast risikoloser, isoliert testbarer Fix: `/rpc` zur bestehenden Ausschlussliste in `gzipMiddleware` hinzufügen (gleiches Muster wie `/api/events`).
2. **JSON-RPC-Batches werden innerhalb EINES HTTP-Requests seriell abgearbeitet.** `handleRPC` (`evm_rpc.go:229-233`) iteriert `for _, raw := range batch { result := s.handleSingle(raw) ... }` — bei `maxBatchSize=100` (dem exakten Wert, den `contabo-loadtest/main.go` bewusst wählt, um Round-Trips zu sparen) heißt das: bis zu 100 `types.Sender(signer, tx)`-Aufrufe (ECDSA-Public-Key-Recovery, eine echte, nicht-triviale CPU-Operation) laufen nacheinander in EINER Goroutine, nicht parallel über die verfügbaren Kerne verteilt. Über viele GLEICHZEITIGE HTTP-Requests hinweg nutzt Go's `net/http` ohnehin alle Kerne (jede Verbindung eine eigene Goroutine) — das hier ist speziell der Preis, den EIN einzelner 100er-Batch zahlt, bevor er fertig ist. Ob sich das lohnt, hängt vom Lastprofil ab (viele kleine gleichzeitige Requests vs. wenige große Batches) — nicht blind parallelisieren, sondern erst mit `AEQUITAS_TPS_CPUPROFILE`/einem eigenen Batch-Benchmark messen, ob `handleSingle` innerhalb eines Batches tatsächlich einen spürbaren Anteil der Batch-Laufzeit ausmacht, bevor ein Worker-Pool (mit den nötigen Sperren um `s.mu`-geschützte Maps) gebaut wird.
3. **`shardedAccounts` kennt keine Eviction.** `Get`/`Set` (`sharded_accounts.go`) legen Konten dauerhaft im In-Memory-Store ab, sobald sie einmal "warm" sind (Voraussetzung für den WAL-/Shard-Lock-Fastpath, siehe Runde-2026-07-23-Messung oben) — es gibt keine LRU/TTL/Kaltzustands-Freigabe. Für die aktuellen 14 Nutzer irrelevant; bei einer auf Millionen Menschen wachsenden Nutzerbasis (das erklärte Projektziel) wächst der RAM-Bedarf des Node-Prozesses mit der Zahl JE EINMAL aktiver Konten, nicht mit der Zahl gleichzeitig aktiver — eine bislang nirgends in diesem Dokument diskutierte, aber für die RAM-Dimensionierung eines Langzeit-Knotens relevante Unbekannte.
4. **Postgres-Verbindungspool-Tuning (Runde 10) wurde nur gegen 4 Kerne validiert, nie gegen 6** — siehe Abschnitt oben.

Keiner dieser vier Punkte ist umgesetzt. (1) ist der mit Abstand risikoärmste und am klarsten begründete — reine Bandbreiten-/CPU-Ersparnis ohne Konsens-/Ledger-Berührung, dem gleichen Muster wie die bereits existierende `/api/events`-Ausnahme. (2)–(4) brauchen erst eine Messung, bevor irgendetwas an ihnen geändert wird — exakt die in diesem Dokument wiederholt demonstrierte Disziplin (Runden 1–10 oben), nicht aus Vorsicht um der Vorsicht willen, sondern weil mehrere frühere "offensichtliche" Optimierungen in genau dieser Session gemessen SCHLECHTER abschnitten als der Status quo.

## Update — Skalierung über mehrere Validatoren: Replikation, kein Sharding

Wichtige Klarstellung, die an keiner Stelle oben explizit gemacht wird, aber jede Aussage über "mehr Validatoren = mehr Durchsatz" falsch werden lässt: Aequitas ist (wie jede GHOSTDAG-/klassische Blockchain-Architektur) ein **replizierter** State-Machine-Ansatz, kein **geshardetes** Netzwerk. Jeder Validator führt JEDE Transaktion aus (`replayTransactions`, deterministisch, zwingend sequentiell pro Node — das ist Konsens-Korrektheit, keine zufällige Serialisierung, die "gefixt" werden könnte). Das bedeutet konkret:

- **Aggregierter Netzwerk-Durchsatz wächst NICHT mit der Validator-Anzahl.** Zehn Validatoren, die alle unabhängig dieselben Transaktionen replayen, liefern zusammen keine 500.000 TPS — sie liefern weiterhin die 50.000 TPS des LANGSAMSTEN Validators, der noch mit dem Netzwerk mithält (langsamere Validatoren fallen über die bereits vorhandenen Mechanismen zurück — `proposerBreaker`, `cleanSyncStreakThreshold` — und riskieren Slashing/Ausschluss, siehe block.go).
- **Mehr Validatoren bringen Dezentralität und Fehlertoleranz, nicht Kapazität** — genau im Sinne der beiden nicht-verhandelbaren Nebenbedingungen ganz oben in diesem Dokument ("kein Design, das teure/spezialisierte Validator-Hardware voraussetzt", "1 Mensch = 1 Validator-Slot"). Das ist eine bewusste Architekturentscheidung, kein Mangel — sie ist der Preis für "jeder Mensch kann validieren", nicht nur wer sich Server-Farmen leisten kann.
- **Konsequenz für die Zielsetzung:** "die Ziele noch höher schrauben, wenn weitere Validatoren dazukommen" funktioniert in DIESER Architektur nur, wenn JEDER einzelne neue Validator für sich selbst mindestens die volle Zielrate schafft — ein einziger unterdimensionierter Validator drückt nicht die eigene, sondern potenziell die NETZWEITE beobachtbare Rate (jeder andere Knoten wartet auf dessen Blöcke/Confirmations im GHOSTDAG-Merge). Ein echter Kapazitätsgewinn über mehr Knoten hinaus bräuchte eine grundsätzlich andere Architektur (Sharding über Validator-Gruppen, jede Gruppe verantwortlich für eine Teilmenge der Konten) — das ist ein eigenes, mehrmonatiges Forschungsprojekt, nicht Teil des aktuellen 50k-Plans, und stünde in Spannung zur "1 Mensch = 1 Validator"-Nebenbedingung (Sharding-Gruppen brauchen typischerweise eine Form von Validator-Zuteilung/Committee-Bildung).
- **Für Mindestanforderungen an Node-Hardware heißt das:** die Spezifikation unten gilt für JEDEN Validator, der am 50k-Ziel teilnehmen will — nicht nur für die zwei aktuellen Contabo-Boxen. Ein Netzwerk aus vielen unterdimensionierten Validatoren ist NICHT gleichwertig zu wenigen gut dimensionierten.

## Update — Empfohlene Mindestanforderungen für Validator-Node-Hardware (2026-07-25, erste Fassung)

Abgeleitet aus (a) den jetzt realen Contabo-Zahlen oben, (b) den A/B-Messungen dieses Dokuments (Runden 1–10, 4-Kern-Sandbox) und (c) den weiterhin offenen Risiken (Fsync-Durchsatz auf ECHTER Zielhardware, RAM-Wachstum ohne Eviction) — als ERSTE, konservative Fassung markiert, nicht als vermessene Untergrenze:

| Ressource | Minimum (heutiger Verkehr, weit unter 50k) | Empfohlen fürs 50k-TPS-Ziel |
|---|---|---|
| CPU | 4 dedizierte Kerne | **6–8 dedizierte Kerne**, NICHT mit der Postgres-Instanz geteilt (der Solo-Node-Crashtest oben maß 1,8s statt <1s für einen vollen Block, explizit zurückgeführt auf "Cores mit Postgres geteilt" — dedizierte Kerne pro Dienst sind kein Nice-to-have) |
| RAM | 4 GB | **16 GB+** — 12 GB (aktuelle Contabo-Größe) ist die UNTERGRENZE für den heutigen, winzigen Nutzerkreis; ohne Account-Eviction (siehe Befund 3 oben) muss dieser Wert mit der Nutzerbasis mitwachsen, nicht mit dem Durchsatz |
| Disk | beliebige SSD | **NVMe SSD, nicht Netzwerk-/Cloud-Blockstorage mit unbekannter Fsync-Latenz** — WAL-Durchsatz (Phase 7) hängt direkt und ungepuffert an echter Fsync-Geschwindigkeit; bisher nur in der Sandbox gemessen (112.700–181.425 appends/s), NIE auf der tatsächlichen Zieldisk |
| Netzwerk | — | Öffentlich erreichbare HTTPS-URL zwingend (siehe STAGING_RUNBOOK.md's Catch-up-Grenze: `isAllowedPeerURL` verweigert jede private/Loopback-Peer-Adresse) — kein Validator hinter reinem NAT ohne Port-Forwarding/Reverse-Proxy |
| Postgres | gleicher Host, gemeinsame Kerne akzeptabel | **eigener Host oder mindestens eigene, dedizierte Kerne** — siehe CPU-Zeile |

**Ausdrücklich nicht belastbar, weil ungemessen:** ob 6–8 Kerne tatsächlich für sustained 50k TPS reichen, ist eine Hochrechnung aus 4-Kern-Sandbox-Zahlen (30.000–47.000 TPS je nach Lastprofil), nicht direkt gemessen. Jetzt, wo SSH-Zugriff auf Contabo über GitHub Actions technisch funktioniert (dieser Workflow-Lauf ist der Beweis), ist ein echter Sustained-Load-Test DIREKT auf Contabo-Hardware zum ersten Mal praktisch möglich, ohne auf die separate, nie aufgesetzte Staging-Umgebung zu warten — das ist die mit Abstand wertvollste nächste Messung für alles in diesem Dokument, nicht nur für diesen Abschnitt.

## Update (2026-07-25, tiefgehende Architektur-Analyse) — der eigentliche Flaschenhals: Block-Replay ist beweisbar einkernig, trotz bereits gebauter Parallelisierungs-Infrastruktur

Die bisherigen Runden in diesem Dokument haben Constant-Factor-Optimierungen gefunden (Shard-Anzahl, Connection-Pool-Größe, gzip, gRPC-Batching). Diese Runde stellt eine andere Frage: **welche Funktion läuft für JEDEN Block, auf JEDEM Knoten, egal ob der Block selbst produziert oder von einem Peer übernommen wurde — und ist SIE parallelisiert?** Antwort, direkt aus dem Code: nein, und das ist der eigentliche Deckel über 50k TPS, nicht CPU-Kerne oder Netzwerklatenz.

### Befund A: `replayTransactions` verarbeitet jeden Block strikt sequentiell, unter EINER globalen Sperre, in EINER DB-Transaktion

`block.go:5634-5635`:
```go
dag.state.mu.Lock()
defer dag.state.mu.Unlock()
```
Diese Sperre wird für die GESAMTE Dauer des Replays eines Blocks gehalten — nicht pro Transaktion, sondern einmal für den ganzen Block. `block.go:5664-5673` öffnet dazu genau EINE Postgres-Transaktion (`dbTx`) für den ganzen Block, `block.go:5696` iteriert dann:
```go
for _, tx := range block.Transactions {
    if hardFailure { break }
    ...
    switch tx.Type { ... }
}
```
Eine strikt sequentielle Schleife, eine Transaktion nach der anderen, in EINER Goroutine. Das gilt für jeden Block, den `AddPeerBlock` von einem anderen Validator übernimmt — also für praktisch den gesamten Datenverkehr, den ein Knoten unter Last verarbeitet, sobald mehr als ein Proposer gleichzeitig Blöcke produziert (KnightDAG/Multi-Tip, s.u.).

Das Bemerkenswerte: **die Infrastruktur für parallele Ausführung existiert bereits im selben Repo** — `sharded_accounts.go`s `LockAddrs`/`TryLockAddrs` (deterministische, aufsteigend sortierte Sperr-Reihenfolge über betroffene Adress-Shards, exakt das Muster, das Deadlock-frei parallele Kontenzugriffe ermöglicht) wird von `transfer_concurrent.go` und `transfer_wal.go` für frisch eingereichte, EIGENE Transaktionen bereits genutzt. `replayTransactions` — der Pfad, der PEER-Blöcke validiert — nutzt diese Infrastruktur nicht und fällt stattdessen auf die grobe, block-weite `dag.state.mu` zurück. Der teuerste, am häufigsten durchlaufene Pfad im gesamten System ist der einzige, der die eigene bereits gebaute Parallelisierungs-Grundlage nicht verwendet.

Zusätzlich: `produceBlockPool` (`workerpool.go:42`) — der einzige Worker-Pool im Blockproduktions-Pfad — ist fest auf Größe 2 codiert ("ProduceBlock always submits exactly 2 jobs per tick"), für den `LoadPendingTxs`/`StateRoot`-Zweiklang. Auf 6-Kern-Contabo-Hardware bleiben damit strukturell mindestens 4 Kerne während der Blockproduktion ungenutzt, unabhängig von jedem Pool-Tuning.

### Befund B: das erklärt den heutigen Vorfall — mehr gleichzeitige Proposer erhöhen den seriellen Replay-Load pro Knoten, nicht die Kapazität

KnightDAG erlaubt mehrere Blöcke pro Tick von verschiedenen Proposern gleichzeitig (`ENABLE_MULTI_BLOCK_TICK`, "not staging-validated" laut Dokument oben) — im Contabo1-Statuscheck von eben liefen live z.B. `[DAG] 🔀 Merged 3 tips into block #1837594` auf Contabo2. Jeder gemergte fremde Tip bedeutet für JEDEN ANDEREN Knoten einen vollständigen `replayTransactions`-Durchlauf durch die o.g. einkernige, block-weit gesperrte Schleife. Mehr gleichzeitige Proposer heißt: mehr Blöcke pro Tick, jeder davon muss von jedem anderen Knoten sequentiell nachvollzogen werden, bevor der Knoten wieder frei ist für den nächsten. Das ist exakt die Form von Rückstau, die zum bereits gefundenen und behobenen Replay-Retry-Sturm (Block #1834993, 9+ Minuten Hänger auf Contabo1, siehe Commit `5c3404a`) und zur beobachteten Tip-Fragmentierung (275→292+ auf Contabo1) geführt hat: der Lasttest hat mehr gleichzeitige Blockproduktion erzeugt, als der serielle Replay-Pfad — unabhängig von verfügbaren CPU-Kernen — absorbieren konnte. Das ist kein Zufallsbug, sondern die vorhersagbare Konsequenz dieser Architektur unter Last. **Konsequenz: `ENABLE_MULTI_BLOCK_TICK` sollte bis zur Behebung von Befund A ausgeschaltet bleiben oder sehr eng gedeckelt werden** — es erhöht genau den Parameter (gleichzeitige Proposer/Tick), der den unparallelisierten Replay-Pfad am stärksten belastet.

### Befund C: `LoadPendingTxs` ist ein globaler Synchronisationspunkt unabhängig von Kernanzahl

`evm_storage.go:2483-2488`: eine einzelne SQL-Anweisung pro Block-Tick —
```sql
UPDATE pending_txs SET included_at = $1
 WHERE id IN (SELECT id FROM pending_txs WHERE included_at = 0 ORDER BY id LIMIT $2)
 RETURNING id, tx_json
```
Das ist der GESAMTE Mempool-Zugriffsmechanismus: eine Postgres-Tabelle mit Row-Locking statt eines In-Memory-Mempools mit eigener Gossip-Verbreitung. Das ist an sich nicht falsch (verhindert Doppel-Einschluss robust, auch über Crashes hinweg), aber es bedeutet: die "Mempool-Sichtbarkeit" zwischen Validatoren läuft NICHT über Netzwerk-Gossip fertig ausgehandelter TX-Batches (wie Narwhal/Bullshark, s.u.), sondern jeder Knoten sammelt nur, was direkt bei IHM per RPC eingereicht wurde, und propagiert erst über fertig gebaute Blöcke weiter. Bei künftig vielen Validatoren ist das ein Fairness-/Latenz-Thema (ein TX, der nur bei Validator X eingereicht wird, wartet auf X's eigenen nächsten Block, statt von jedem beliebigen Proposer sofort aufgenommen werden zu können) — aktuell nicht der TPS-Flaschenhals, aber relevant, sobald "mehr Validatoren" wirklich verfolgt wird.

### Befund D: ECDSA-Batch-Verifikation — die naheliegende Idee ist eine Sackgasse, die eigentliche Lösung ist Parallelisierung, nicht Batching

Aequitas verifiziert Transaktionssignaturen per `ecrecover` (secp256k1/ECDSA, EVM-kompatibel — sichtbar an `chain_evm_id` im `/api/status` und den `ecrecover`-Kommentaren in `block.go`/`api.go`). Aktuelle Forschung (2026, IACR ePrint 2026/663) bestätigt: **Standard-ECDSA in der Form, wie sie üblich (und EVM-kompatibel) signiert wird, lässt sich NICHT effizient batch-verifizieren, ohne das Signaturformat selbst zu ändern** (ECDSA*-Varianten mit dem vollen Punkt R statt nur `r` ermöglichen Batch-Verifikation, sind aber nicht wire-kompatibel zu Standard-`ecrecover`). Selbst mit modifiziertem Format sind die gemessenen Gewinne mit ~10–30 % pro Batch moderat. Eine Umstellung des Signaturformats wäre ein hartes, sicherheitskritisches, Kompatibilität-brechendes Unterfangen für einen bescheidenen Gewinn — **nicht empfohlen.**

Der eigentlich lohnende Hebel ist ein anderer: `handleRPC` (`evm_rpc.go:229-233`, bereits in der vorigen Runde als Befund 2 notiert) verifiziert bis zu 100 Signaturen pro Batch-Request SERIELL in einer Goroutine — nicht weil ECDSA das erfordert, sondern weil der Code es so schreibt. `ecrecover` ist eine zustandslose, reine Funktion ohne geteilten Zustand zwischen Aufrufen — trivial über einen Worker-Pool auf alle verfügbaren Kerne zu verteilen, ganz ohne die algebraischen Risiken einer echten Signatur-Batch-Verifikation. Bei 6 Kernen ist das ein strukturell bis zu 6-facher Gewinn auf genau diesem CPU-lastigen Schritt, ohne Sicherheits-Kompromiss. Gleiches Prinzip gilt für die Signaturprüfung innerhalb von `replayTransactions` selbst (Befund A) — sobald dessen Sperre pro Adress-Shard statt block-weit wird, kann auch die Signaturprüfung der einzelnen TXs vorab parallel über einen Worker-Pool laufen, bevor die (dann parallelisierte) Zustandsänderung beginnt.

### Befund E: horizontale Skalierung über mehr Validatoren bräuchte echtes Sharding — 2026-Forschung bestätigt den Ansatz existiert, aber er passt aktuell nicht

Externe Recherche (Juli 2026) bestätigt zwei Architekturfamilien, die genau das o.g. Problem (serielle Block-Verarbeitung) in Produktion gelöst haben:

- **Block-STM (Aptos)** — optimistische parallele Ausführung: alle TXs eines Blocks werden parallel ausgeführt, unter der Annahme, dass sie sich nicht überschneiden; bei erkanntem Konflikt (gelesene Daten wurden von einer früheren TX im selben Block verändert) wird NUR die betroffene TX erneut ausgeführt. Bis zu 160.000 TPS theoretisch, in der Praxis mehrfach in Produktion bestätigt. Das ist konzeptionell fast identisch mit dem, was Befund A oben als Fix vorschlägt — nur mit einem echten Validierungsschritt statt nur "läuft parallel, weil Adressen ohnehin meist disjunkt sind".
- **Narwhal/Bullshark (Sui und andere)** — trennt Datenverteilung (Mempool, hoher Durchsatz, DAG-strukturiert) von Konsens-Ordering (nur Hashes, niedriger Durchsatz). Bis zu 297.000 TPS bei 2s Latenz in Forschungsergebnissen. Das adressiert Befund C (Postgres-Tabelle statt Gossip-Mempool) — aber ist für 2–3 Validatoren aktuell überdimensioniert; relevant erst bei echtem Multi-Validator-Wachstum.

**Beide Ansätze sind Skalierung PRO KNOTEN (mehr Durchsatz aus denselben Kernen holen), nicht Skalierung ÜBER Knoten (echtes Sharding/Partitionierung der Kontenmenge auf Validator-Gruppen).** Letzteres — Accounts nach Adress-Präfix auf Validator-Untergruppen aufteilen, jede Gruppe verantwortlich für ihre eigene Teilmenge, Cross-Shard-Transfers über asynchrone Receipts — würde tatsächlich echte Kapazität ÜBER die Validator-Anzahl skalieren (Befund oben zu "Replikation statt Sharding"), ist aber ein eigenständiges, monatelanges Forschungsprojekt, kollidiert mit der demurrage-/UBI-/Pool-Mechanik (globale, nicht schardierbare gemeinsame Zustände) und mit der "1 Mensch = 1 Validator"-Nebenbedingung (Sharding-Gruppen brauchen typischerweise Committee-Zuteilung). **Nicht empfohlen, solange Befund A–D nicht gehoben sind** — die aktuelle Zwei-Validator-Instabilität zeigt, dass selbst die einfachere Vollreplikation noch nicht robust genug ist, um eine zusätzliche Sharding-Komplexitätsebene zu rechtfertigen.

Explizit weiterhin NICHT empfohlen (Konsistent mit der vorherigen Hintergrund-Recherche dieser Session): Solana Sealevel/Monad 1:1 übernehmen (Hardware-Voraussetzungen kollidieren mit der "1 Mensch = 1 Validator auf Consumer-Hardware"-Nebenbedingung), Ethereum Verkle Trees/Celestia-Style Data-Availability-Trennung (falsche Ebene — Aequitas ist bei 2–3 Knoten nicht DA-limitiert, sondern CPU/Lock-limitiert), volle GHOSTDAG-Parameterlosigkeit statt der bestehenden gedeckelten KnightDAG-Variante (mehr Forschungsrisiko für ein Problem, das nicht das aktuelle ist).

### Priorisierter Fahrplan (aus dieser Analyse)

1. **Höchster Hebel: `replayTransactions` parallelisieren.** Die vorhandene `LockAddrs`/`TryLockAddrs`-Infrastruktur (bereits getestet, bereits in Produktion für `transfer_concurrent.go`) auf den Replay-Pfad anwenden: TXs eines Blocks nach betroffenen Adressen gruppieren, disjunkte Gruppen parallel ausführen (Block-STM-artig: optimistisch parallel, bei Adress-Überlappung seriell nachziehen), EIN gesammeltes Write-Set am Ende in einer einzigen DB-Transaktion committen (bewahrt die bestehende Alles-oder-Nichts-Blockatomarität, ändert nur WIE das Write-Set entsteht). Das ist die einzige Änderung in dieser Liste, die die tatsächlich am häufigsten durchlaufene Funktion im System betrifft.
2. **Signaturverifikation parallelisieren (nicht batchen).** `ecrecover`-Aufrufe im RPC-Batch-Pfad (`evm_rpc.go`) und im Replay-Pfad über einen Worker-Pool verteilen, VOR der (dann ebenfalls parallelen) Zustandsänderung. Kein algebraisches Batching (Befund D) — reine Parallelisierung einer zustandslosen Funktion, geringes Risiko.
3. **`ENABLE_MULTI_BLOCK_TICK` bis Punkt 1 eng gedeckelt lassen.** Es verschärft aktuell genau den Engpass, den es umgehen soll (Befund B).
4. **`/rpc` zur gzip-Ausnahmeliste hinzufügen** (bereits in der vorigen Runde als risikoärmster Punkt 1 notiert) — unabhängig von 1–3, sofort machbar.
5. **Erst NACH 1–3, und nur bei echtem Bedarf (zweistellige Validator-Anzahl): Narwhal/Bullshark-Style Mempool-Trennung** für Befund C — löst ein Fairness-/Latenzproblem, nicht den aktuellen TPS-Deckel.
6. **Echtes Sharding: bewusst zurückgestellt**, siehe Befund E.
7. **Sicherheitsnetz, WICHTIGER als je zuvor sobald Punkt 1 umgesetzt wird:** die bereits von der Hintergrund-Recherche dieser Session empfohlene Determinismus-Fuzzing-Testsuite (Fastpath-Ausführung == Replay-Ausführung == identischer StateRoot) ist die Voraussetzung dafür, dass Punkt 1 nicht dieselbe Klasse von Bug einführt, die Solanas 4,5h-Ausfall im Februar 2026 verursacht hat (Nichtdeterminismus durch fehlerhafte parallele Ausführung). Parallelisierung des Replay-Pfads ohne diese Absicherung ist der riskanteste Einzelschritt in diesem gesamten Fahrplan — nicht weil die Idee falsch ist, sondern weil genau diese Klasse von Optimierung in der Praxis am häufigsten zu konsensrelevanten, schwer reproduzierbaren Bugs führt.

**Quellen (externe Recherche, 2026):** Block-STM / Aptos (aptoslabs.com/pdf/2203.06871.pdf, Everstake-Übersicht 2026); ECDSA-Batch-Verifikation (IACR ePrint 2026/663, "Batch Verification of Modified ECDSA Signatures"); Narwhal/Bullshark (arXiv:2105.11827, arXiv:2507.04956).

## Update (2026-07-25) — Umsetzungsstatus des Fahrplans

Auf Anweisung umgesetzt, auf diesem Branch, NICHT auf Contabo1/2 deployed:

- **Punkt 4 (gzip-Ausnahme für `/rpc`)** — erledigt, `api.go`.
- **Punkt 2 (Signaturprüfung im RPC-Batch-Pfad parallelisieren)** — erledigt, `evm_rpc.go`: `decodeAndRecoverSender` als reine Funktion extrahiert, läuft jetzt über einen Worker-Pool (`runtime.NumCPU()`) für jedes `eth_sendRawTransaction`-Element eines Batches, BEVOR die weiterhin strikt sequentielle Dispatch-Schleife läuft. Zustandsmutation, Nonce-Reservierung und Ausführungsreihenfolge bleiben unverändert. Test unter `-race` grün (`evm_rpc_batch_parallel_test.go`).
- **Punkt 7 (Determinismus-Sicherheitsnetz)** — erledigt, `replay_determinism_fuzz_test.go`: bestätigt an der BESTEHENDEN sequentiellen `replayTransactions`, dass ein Batch paarweise disjunkter Transfers unabhängig von der Ausführungsreihenfolge denselben StateRoot/dieselben Kontostände liefert (1 fixer Reversal-Test + 25 randomisierte Shuffle-Iterationen, alle grün).
- **Punkt 3 (Multi-Block-Tick eng gedeckelt lassen)** — keine Code-Änderung nötig, bereits Opt-in/env-gated; als operative Haltung bestätigt, nicht weiter aufgedreht.

## Update (2026-07-25, Fortsetzung) — Punkt 1 jetzt umgesetzt, mit einem zweiten, ECHTEN Bug unterwegs gefunden

Auf explizite Anweisung ("Punkt 1 umsetzen") doch angegangen — der ursprünglich gefundene RWMutex-Reentranz-Deadlock (siehe oben) wurde NICHT durch ein Lock-Downgrade umgangen (das hätte die neue, ungetestete Kontention mit `transferConcurrent` geöffnet, wie oben beschrieben), sondern durch einen strukturell einfacheren, beweisbar sichereren Ansatz vermieden:

**Das tatsächliche Design:** `replayTransactions` läuft weiterhin komplett unter dem einen, bereits gehaltenen `dag.state.mu.Lock()` — unverändert exklusiv für die GESAMTE Blockdauer, wie bisher. NEU ist eine Vorab-Phase, die NUR beweisbar sichere "transfer"-Transaktionen identifiziert (Typ `transfer`, `FromDemurrageLost == 0 && ToDemurrageLost == 0` — garantiert per `applyDemurrageLossLockedCtx`s eigenem `lost <= 0`-Frühausstieg, dass NIE eine der vier Pool-Adressen berührt wird —, weder Wallet noch To eine Pool-Adresse, und beide Adressen im gesamten Kandidaten-Set nur je EINMAL vorkommen) und diese Teilmenge über einen Worker-Pool (`runtime.NumCPU()`) parallel abarbeitet, BEVOR die bestehende sequentielle Schleife (unverändert) den Rest übernimmt — die bereits per Index übersprungenen Transfers eingeschlossen. Die Worker rufen dieselbe, unveränderte `applyTransferDeltaLocked` auf; da sie NIE selbst `dag.state.mu`/`cs.mu` anfassen (die Exklusivität kommt bereits vom aufrufenden Goroutine), entfällt die Reentranz-Falle vollständig — und da beide Phasen strikt nacheinander laufen (nie gleichzeitig), gibt es auch keine neue Kontention mit dem sequentiellen Rest oder mit `transferConcurrent`, dessen `cs.mu.RLock()` für die gesamte Replay-Dauer weiterhin blockiert, exakt wie vorher. `hardFailure` aus der Parallel-Phase wird in dieselbe Variable gespiegelt, die die sequentielle Schleife bereits als ersten Check hat — ein Fehler irgendwo löst weiterhin den kompletten, bereits bestehenden Block-Rollback aus (`blockTouchedAddresses` deckt alle Adressen inkl. der Pool-Adressen bereits vorher ab, unverändert).

**Der Determinismus-Test (`replay_determinism_fuzz_test.go`, VOR dieser Implementierung geschrieben) validiert jetzt automatisch die ECHTE parallele Ausführung** (nicht mehr nur die sequentielle, gegen die er ursprünglich geschrieben wurde) und bleibt grün — 26/26 Fälle.

**Ein zweiter, echter Bug — nur durch einen dedizierten Real-DB-Test gefunden, nicht durch `-race` oder die In-Memory-Tests:** `TestReplayTransactions_ParallelTransfers_RealDB` (neu, 24 disjunkte Transfers in einem Block gegen eine echte lokale Postgres-Instanz) schlug beim ersten Lauf mit `driver: bad connection` und `pq: unexpected Parse response "(C) CommandComplete"` fehl — mehrere Worker, die gleichzeitig über dieselbe `dag.state.activeTx` (`*sql.Tx`) schreiben/lesen, brachten die Verbindung durcheinander. Das widerspricht der eigenen Stdlib-Analyse von vorhin (`execDC`s `withLock(dc, ...)` sollte das eigentlich serialisieren) — bestätigt aber empirisch: der `lib/pq`-Treiber, den dieses Projekt nutzt, ist in der Praxis NICHT sicher gegen echte Nebenläufigkeit auf derselben `*sql.Tx`, unabhängig davon, was die generische `database/sql`-Doku suggeriert. **Fix:** eine dedizierte `dbMu sync.Mutex` serialisiert jetzt genau den DB-berührenden Teil jedes Worker-Aufrufs (`applyTransferDeltaLocked` komplett, da `ensureAccountLoadedCtx` bei kalten Konten ebenfalls einen DB-Lesezugriff auslöst). 5/5 Wiederholungen unter `-race` grün nach dem Fix.

**Ehrliche Einordnung des tatsächlichen Gewinns:** durch `dbMu` ist der reale Parallelisierungsgewinn für den Postgres-gestützten Fall (= die echte Produktion) kleiner als ein erster Blick auf "N Worker" vermuten lässt — nur die CPU-gebundene Arbeit VOR dem Lock (Shard-Zugriff, Bilanz-/Demurrage-Prüfung) läuft pro Worker wirklich parallel; der eigentliche DB-Schreibzugriff bleibt faktisch seriell. Das ist dennoch strikt besser als der bisherige Zustand (vorher war ALLES seriell, inklusive der CPU-Arbeit) und beweisbar korrekt statt beweisbar kaputt — aber es ist NICHT der volle "6x auf 6 Kernen"-Gewinn, den die reine In-Memory-Messung suggerieren würde. Ein echter DB-seitiger Gewinn bräuchte eine größere Restrukturierung (Schreibmengen im Speicher sammeln, EINMAL am Ende der Parallel-Phase als einzelnes Batch-Statement committen, statt N einzelne Saves über dieselbe Tx zu serialisieren) — nicht in dieser Sitzung umgesetzt, siehe "Nächste Schritte".

**Getestet:** volle Suite (`go test ./x/humanity/keeper/... -race`), `go vet ./...`, Determinismus-Tests (26 Fälle), bestehender Real-DB-Replay-Test, neuer Real-DB-Parallel-Test (5× unter `-race`) — alle grün. **NICHT deployed** auf Contabo1/2 — bleibt auf diesem Branch, bis explizit anders entschieden.

---

# Update (2026-07-25/26) — Roadmap-Schritte 5 und 6 abgeschlossen, 4 teilweise, mit Messwerten

Diese Session hat die Roadmap-Punkte aus der Statustabelle abgearbeitet, so
weit sie ohne Zugriff auf die Produktionsknoten abschließbar sind. Alles
unten ist **gemessen**, nicht geschätzt; jede Zahl ist über einen Test im
Repo reproduzierbar. Zwei Optimierungsversuche sind an echten Messungen
gescheitert und wurden verworfen statt beschönigt — sie stehen hier mit
ihren Zahlen, damit sie niemand erneut versucht.

## Ausgangsmessung: der Replay-Pfad war der eigentliche Deckel

Bisher hat keine Messung in diesem Projekt direkt beantwortet, wie schnell
**ein Knoten einen Block voller Transfers wieder einspielt**. Genau das ist
aber die Zahl, die über 50.000 TPS entscheidet: jeder Sekundärknoten macht
das für jeden Block. `TestReplayThroughput_DisjointTransfers` (neu) misst es:

| Stand | Replay-Durchsatz | SQL-Statements pro Transfer |
|---|---|---|
| vorher | **382 tx/s** (2,6 ms/tx) | 3,0 |
| + Ausdrucks-Index auf `lower(address)` | 1.250 tx/s | 3,0 |
| + ctx-skopierte "aufgelöste Adressen" | 2.000 tx/s | 2,0 |
| + gepufferte Kontoschreibvorgänge | **24.918 tx/s** (40 µs/tx) | **4 Statements für den GANZEN Block** |

Ein CPU-Profil des Ausgangszustands zeigte **13 % CPU, 87 % Warten**. Der
Replay-Pfad war nie rechen-, sondern immer round-trip-gebunden.

**Damit ist auch die Frage zu Roadmap 6 beantwortet, und zwar anders als
erwartet.** Der am 2026-07-25 zurückgerollte Versuch, disjunkte Transfers
auf mehreren Goroutinen auszuführen, hätte sich selbst dann nicht gelohnt,
wenn er sicher gewesen wäre: parallelisiert worden wäre die Arithmetik, und
die war nie der Kostenfaktor. Die zweite Option aus der damaligen
Revert-Notiz ("die DB-Schreibvorgänge ganz aus der parallelen Phase
herausziehen") war von Anfang an die richtige — genau das ist jetzt
umgesetzt, mit demselben einen Goroutine und demselben einen `*sql.Tx`, in
dem der Wire-Protocol-Desync strukturell unmöglich ist.

### Der größte Einzelfund: jeder Kontozugriff war ein Sequential Scan

Jede Kontoabfrage in diesem Code lautet `WHERE lower(address) = $1`. Ein
B-Tree auf der blanken Spalte `address` kann dieses Prädikat **nicht**
bedienen — Postgres weiß nicht, dass `lower()` ordnungserhaltend ist. Also:
voller Tabellenscan bei **jedem einzelnen Kontoladen**, mit linear
wachsenden Kosten.

```
EXPLAIN vorher:  Seq Scan on chain_accounts ... Rows Removed by Filter: 1999
EXPLAIN nachher: Index Scan using idx_chain_accounts_lower_address
```

Bei nur 2.000 Konten kostete das `SELECT` in `ensureAccountLoaded` 750 µs,
gegenüber 32 µs für das `UPDATE` daneben. Zwei Indizes in `initDB` nutzten
die Ausdrucksform bereits (`idx_nullifiers_wallet`,
`idx_bio_registrations_wallet`) — die heißen Tabellen hatten sie nur nie
bekommen. Nachgezogen für `chain_accounts`, `evm_storage`, `evm_contracts`,
`evm_nonces`, `registered_nodes`, `guardians`, `bio_hashes`, `chain_blocks`.

`schema_index_coverage_test.go` leitet die nötige Menge jetzt aus dem SQL
des Pakets selbst ab und schlägt fehl, sobald eine `lower()`-Abfrage ohne
passenden Index existiert — plus ein `EXPLAIN`-Test, dass der Planer ihn
wirklich benutzt.

Die Kosten sind ebenfalls gemessen, nicht angenommen: der Index macht den
100k-Zeilen-Bulk-Upsert **19 % langsamer** (1063 ms → 1306 ms) und einen
Einzel-Lookup **~500× schneller** (56 ms Seq Scan bei 100k Zeilen, linear
wachsend, gegen einen Index Scan). Beim Ingestion-Pfad bringt er dagegen
ehrlicherweise fast nichts (4184 → 4358 TPS, ~4 %), weil dort die Konten
bereits warm in `cs.accounts` liegen.

### Zwei verworfene Optimierungen (mit Zahlen)

1. **Chunking des Multi-Row-Upserts.** `EXPLAIN (ANALYZE, BUFFERS)` zeigt
   bei 100.000 Zeilen einen auf Platte auslagernden Hash (~32 MB Temp-I/O)
   — sieht nach einem Fall für work_mem-große Chunks aus. Ist es nicht:

   | Chunk | 500 | 1000 | 2000 | 5000 | 10000 | ohne |
   |---|---|---|---|---|---|---|
   | tx/s | 7.203 | 7.807 | 10.006 | 15.920 | 19.213 | **22.547** |

   Monoton schlechter. Parse-Kosten pro Statement und der erneut
   ausgewertete `NOT IN`-Subplan kosten mehr als das Auslagern.

2. **`work_mem` auf 256 MB.** Beseitigt das Auslagern vollständig, bewegt
   dasselbe Statement aber nur von 1373 ms auf 1325 ms (3,5 %).

Was bleibt, ist Postgres' echter Boden für diese Tabellenform: **~13 µs pro
upserted Zeile**, dominiert von Heap- und Index-Pflege. Darunter kommt man
nur, indem man nicht mehr jeden Kontostand pro Block synchron schreibt —
also genau über Phase 7 (WAL als Durability-Grenze, Postgres asynchron
dahinter). Der Replay-Pfad ist damit **nicht mehr der Engpass**; die
nächste Grenze ist wieder die architektonische, die dieses Dokument von
Anfang an beschreibt.

## Schritt 5 — `cs.activeTx`-Migration: abgeschlossen

Der Blocker war "[DB-GUARD]-Logs auswerten". Das hätte die falsche Frage
beantwortet: `[DB-GUARD]` feuert nur für die Teilmenge der Pfade, die
zusätzlich auf einer FREMDEN Goroutine laufen. Die Menge, die Nebenläufigkeit
blockiert, ist die viel größere, die auf der EIGENEN Goroutine zurückfällt —
und darüber schweigt die Produktion.

Die Frage ist jetzt lokal beantwortet, doppelt und unabhängig:

- `activetx_static_test.go` läuft den Aufrufgraphen des Pakets von den drei
  Stellen ab, an denen eine Transaktion geöffnet wird, und schlägt bei jedem
  erreichbaren DB-Schreibvorgang fehl, der `ctx` fallen lässt. Die sechs
  Aufrufstellen, die tatsächlich außerhalb der Transaktion laufen, tragen
  eine explizite `activetx:outside-tx`-Markierung und werden bei jedem Lauf
  ausgegeben — keine undurchsichtige Allowlist.
- `activetx_trace.go` protokolliert dieselben Rückfälle zur Laufzeit. Die
  volle Suite gegen ein echtes Postgres erzeugt jetzt **null**.

Von 38 Fundstellen auf 0. `replayTransactions` baut ein `replayCtx` aus dem
eigenen `dbTx` und reicht es durch alle 41 Aufrufe innerhalb der Transaktion.
`TestActiveTx_ConcurrentAtomicOperationsUseSeparateTransactions` hält fest,
wofür das war: zwei gleichzeitige atomare Operationen lösen jetzt auf zwei
getrennte, isolierte `*sql.Tx` auf — vorher strukturell nicht ausdrückbar.

## Schritt 4 — Blockkörper als Referenzen: Voraussetzung fehlt, Teilfix geliefert

Beim Lesen des Relay-Pfads kam eine Voraussetzung zutage, die in der
Roadmap-Notiz fehlt und die den Schritt deutlich größer macht als eine
Formatmigration: **Referenzen helfen nur, wenn die Peers die Transaktions-
körper bereits haben, wenn der Block ankommt — und das haben sie hier
nicht.** `pending_txs` ist ein rein lokaler Outbox-Tisch (`SavePendingTx`
schreibt in die eigene DB des Knotens), und die P2P-Schicht broadcastet
ausschließlich Blöcke. Ein Peer erfährt den Inhalt einer Transaktion also
ausschließlich aus dem Block, der sie trägt. Hash-Referenzen ohne eine
vorgelagerte **Transaktions-Gossip-Schicht** würden jeden Peer zwingen,
jeden Körper beim Empfang nachzuladen: dieselben Bytes plus ein Round-Trip,
also strikt schlechter. Diese Gossip-Schicht ist ein eigenes Projekt und
wurde hier **nicht** angefangen.

Dieselben Bytes lassen sich aber ohne Konsensänderung angreifen — der
Relay-Payload war unkomprimiertes JSON:

| TXs/Block | roh | gzip | Faktor |
|---|---|---|---|
| 1.000 | 0,24 MB | 0,01 MB | 17,6× |
| 20.000 | 4,80 MB | 0,27 MB | 17,7× |
| 50.000 | 12,00 MB | 0,68 MB | 17,7× |

Konsens bleibt unberührt (der Blockhash wird über den dekodierten Block
gebildet — komprimierter und unkomprimierter Relay erzeugen bitgleiche
Hashes, festgehalten in einem Test). **Ausrollung bewusst zweistufig:** Phase
1 (dieser Stand) akzeptiert beide Kodierungen und sendet keine — in
beliebiger Reihenfolge auf beliebige Teilmengen deploybar. Phase 2 setzt
`AEQUITAS_P2P_COMPRESS_BLOCKS=1`, **erst wenn jeder Knoten ein Phase-1-Binary
fährt**, sonst partitioniert man das Netz still.

## `cs.mu`-Kontention: Werkzeug geliefert, Diagnose braucht den Live-Knoten

Diese Untersuchung hing nicht an der Analyse, sondern an einem fehlenden
Werkzeug: es gab **keinen Weg, einen Goroutine-Dump vom Primary zu bekommen**
(pprof ist bewusst localhost-only, die Plattform bietet kein `docker exec`).

`GET /api/debug/goroutines` schließt das, hinter derselben
`SNAPSHOT_TOKEN`-Prüfung wie die übrigen Betreiber-Endpunkte. Entscheidend:
der Endpunkt nimmt **keine Applikationssperre** — weder `cs.mu` noch
`dag.mu` — denn gebraucht wird er genau dann, wenn eine davon hängt. Ein
Test hält `cs.mu` schreibend über eine laufende Anfrage und prüft, dass sie
trotzdem antwortet.

`?summary` gruppiert nach (Scheduler-Zustand, blockierendem Frame,
äußerstem Projekt-Frame), zählt und sortiert nach längster Wartezeit — also
"142 blockiert in `sync.(*RWMutex).Lock` unter `TransferAtomic`, wartet seit
≥13 min" statt acht Megabyte Stacks.

**Nächster Schritt (braucht Produktionszugriff, nicht mehr Code):** auf dem
Primary unter Last `curl -H "Authorization: Bearer $SNAPSHOT_TOKEN"
https://<primary>/api/debug/goroutines?summary` aufrufen. Die oberste
Gruppe benennt den Halter. Die Peer-Registrierung hängt vermutlich an
derselben Ursache und sollte mit demselben Dump beantwortbar sein.

## Weiterhin offen

| Punkt | Status |
|---|---|
| Schritt 1 — WAL auf C2 reaktivieren | Betriebsaufgabe, kein Code-Gap |
| Schritt 3 — UBI auf Lazy-Claim | **nicht angefangen**, siehe unten |
| Schritt 4 — Referenzen | braucht zuerst Transaktions-Gossip |
| Schritt 7 — Finality-Gadget | nicht angefangen |
| `cs.mu`-Kontention | Werkzeug da, Diagnose braucht Live-Knoten |
| Peer-Registrierung | vermutlich Folge der Kontention |

**Zu Schritt 3, damit die Größe nicht unterschätzt wird.** Lazy-Claim ist
konsensverändernd auf der Kontoebene: es braucht einen Akkumulator im
StateRoot, ein `ubi_claimed`-Feld pro Konto (inklusive Spalte, Aufnahme in
`accountLeaf`, Snapshot-Import/-Export) und eine Aktivierungshöhe. Der
heikle Teil ist die **Reihenfolgeabhängigkeit**: der Betrag, den ein Konto
beim Abrechnen gutgeschrieben bekommt, hängt vom Akkumulatorstand in genau
diesem Moment ab. Zwischen der Zustandsänderung auf dem Primary und dem
Block, der die Verteilungs-TX trägt, existiert ein Fenster, in dem ein
Transfer auf dem Primary gegen den NEUEN Akkumulator abrechnet, während der
Sekundärknoten dieselbe Transaktion beim Replay gegen den ALTEN abrechnet —
gleiche Endsalden, aber unterschiedliche gespeicherte Salden, also
divergierender StateRoot. Der gangbare Weg ist der, den dieses Repo für
Demurrage bereits nutzt: der Primary entscheidet den exakten Betrag und
trägt ihn in der TX mit (`FromDemurrageLost` als Vorbild), der Sekundärknoten
rechnet ihn nicht nach. Angefangen wurde davon bewusst nichts — eine halb
verdrahtete Konsensänderung im geldbewegenden Kern wäre schlechter als
keine.
