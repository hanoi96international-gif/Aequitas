# Staging-Runbook — Scaling-Änderungen vor Produktions-Rollout validieren

Zweite Hälfte von "Beides" (lokale Mehrprozess-Simulation + Runbook). Die
lokale Simulation (siehe Abschnitt "Was die lokale Simulation bereits
gezeigt hat" unten) hat gezeigt, dass die Cross-Node-Mechanik funktioniert —
sie ersetzt aber NICHT echtes Staging: kein Multi-Tage-Lauf, keine echte
Netzwerklatenz/-jitter, keine echte Diskhardware, nur 2 Knoten statt der
realen Validator-Anzahl, keine adversariellen Bedingungen.

Dieses Dokument ist die konkrete, ausführbare Anleitung für einen echten
Staging-Lauf auf echter Infrastruktur, getrennt von den live laufenden
Contabo-Produktionsknoten.

## Was validiert werden muss, bevor `main` es bekommt

Fünf reale Risikoänderungen aus dieser Session, jede einzeln markiert
`NOT staging-validated` im Code:

| Änderung | Commit | Risikoklasse | Env-Var (opt-in) |
|---|---|---|---|
| `TryLockAddrs`-Shard-Locking (Transfer-Fastpath) | 575ef7b | Nebenläufigkeitskorrektheit | keine — immer aktiv im Fastpath |
| WAL-durable Transfer-Pfad | 9764d6f | Durability-Semantik (Crash-Recovery) | `AEQUITAS_WAL_ENABLED` / `AEQUITAS_WAL_PATH` |
| Shard-gelockte Registrierung | 74a5551 | Nebenläufigkeitskorrektheit | keine — immer aktiv im Fastpath |
| `maxTxsPerBlock` 20k→50k + P2P-Transportcap-Fix | 1d2736f | Blockgröße/-verteilung | keine — immer aktiv |
| Mehrere Blöcke/Tick bei Backlog | c6e5bfb | Konsens-Timing (netzwerkweit sichtbar) | `ENABLE_MULTI_BLOCK_TICK` |

Jede Zeile ist unabhängig abschaltbar (die vier Fastpath-Änderungen fallen
bei Fehlern automatisch auf den bestehenden langsamen Pfad zurück, siehe
deren jeweilige `TransferAtomic`/`RegisterHumanAtomic`-Fallback-Logik) —
Staging kann sie daher **einzeln, nacheinander** aktivieren statt als
Big-Bang.

## Infrastruktur

- **Mindestens 2, idealerweise 3 Knoten**, getrennt von Produktion: eigene
  Contabo-VPS (oder gleichwertig), eigene Postgres-Instanzen, eigene
  Domain/Subdomain (z. B. `staging.aequitas.digital`), eigenes
  `CHAIN_ID`/Genesis, damit ein Staging-Block niemals versehentlich als
  Produktionsblock akzeptiert werden kann.
- Hardware so nah wie praktikabel an den Produktionsknoten (gleiche
  VPS-Klasse, insbesondere gleiche Diskklasse — WAL-Fsync-Durchsatz hängt
  direkt daran).
- Getrennter `PEER_SECRET`/`NODE_KEY`/`RELAYER_PRIVATE_KEY`-Satz pro
  Staging-Knoten, niemals aus Produktion wiederverwendet.

## Validator-Autorisierung in Staging (NICHT der lokale Simulations-Shortcut)

Die lokale Simulation hat Validator-Autorisierung durch einen direkten
`INSERT INTO validator_keys` umgangen (kein Docker/echter Multi-Host-Zugriff
aus der Sandbox heraus, ZK-Proof-Registrierung wäre unverhältnismäßiger aus
für einen reinen Konsens-Mechanik-Test). **In Staging ist das falsch** —
Staging soll den echten Pfad testen, nicht ihn umgehen:

1. Für jeden Staging-Knoten einen echten Menschen (Testperson, echte
   ZK-Proof-Registrierung über die normale App/`register`-Flow) auf der
   Staging-Chain registrieren.
2. Den Knoten-Betreiber-Workflow reell durchlaufen: `NODE_OPERATOR_WALLET`
   setzen, `RegisterNode`, dann echten `personal_sign` über
   `"Aequitas: authorize validator <signing_address>"` mit der
   Test-Wallet, `BindValidatorSlot` über `/api/peers/register` mit
   `operator_binding_signature` — exakt der Pfad, den ein echter Operator
   in Produktion durchläuft (`api.go: handlePeerRegister`).
3. Erst wenn dieser Pfad in Staging sauber durchläuft (Validator wird über
   `syncValidatorsFromAllPeers` netzwerkweit bekannt, Blöcke werden
   akzeptiert), gilt die Autorisierungskette selbst als mitgetestet — das
   war in der lokalen Simulation bewusst ausgeklammert.

## Testkampagne (mehrtägig, gestuft)

**Tag 1-2 — WAL-Durability isoliert:**
- `AEQUITAS_WAL_ENABLED=1` auf einem einzelnen Staging-Knoten, moderate
  synthetische Transfer-Last (nicht die volle 50k-TPS-Zielrate — erst mal
  nur WAL-Korrektheit, nicht Spitzenlast).
- Geplante Prozess-Kills (`kill -9`) an zufälligen Zeitpunkten während
  aktiver Last, jeweils gefolgt von Neustart + Prüfung: `recoverFromWAL`
  reproduziert exakt den Vor-Crash-Zustand, kein Salden-Verlust, keine
  doppelte Anwendung (idempotent über `WALSeq`, bereits unit-getestet —
  Staging bestätigt es unter echtem Fsync-Timing statt In-Memory-Fake).
- Abbruchkriterium: jede Saldo-Abweichung nach Recovery, jeder Crash-Loop.

**Tag 3-4 — Mehrknoten-Konsens unter echter Last:**
- Alle Staging-Knoten mit echter Validator-Autorisierung (siehe oben),
  WAL weiterhin aktiv, synthetische Last aus mehreren gleichzeitigen
  Quellen (mehrere Lastgeneratoren, disjunkte UND überlappende
  Empfänger-Mengen — siehe `tps_bench_test.go`s zwei Szenarien als
  Vorlage für Lastprofile).
- Konvergenzprüfung alle paar Minuten über `/api/status` auf allen
  Knoten: `height` und `latest_hash` müssen nach jeder Beobachtung
  identisch sein (exakt die Prüfung, die die lokale Simulation bereits
  einmal bestanden hat — jetzt über echte Knoten/Netzwerk statt
  localhost).
- Abbruchkriterium: anhaltende Divergenz (>1 Tick ohne Konvergenz),
  Orphan-Rate signifikant über Baseline, jeder Panic/Crash.

**Tag 5 — `ENABLE_MULTI_BLOCK_TICK` (höchstes Risiko, zuletzt):**
- Nur auf EINEM Staging-Knoten aktivieren, andere Knoten unverändert
  lassen (genau das Verhalten, das dieses Feature laut eigenem
  Doc-Kommentar erlaubt: pro Validator opt-in, kein netzwerkweites
  Upgrade nötig).
- Last so hoch fahren, dass Blöcke tatsächlich voll werden (>= 50.000 TX,
  `maxTxsPerBlock`) — nur dann wird die Zusatzproduktions-Schleife
  überhaupt ausgelöst (siehe `produceBlocksForTick`s Backlog-Gate). Ohne
  echten Backlog bleibt dieser Pfad ungetestet, wie schon in der lokalen
  Simulation beobachtet.
- Beobachten: bleibt "jeder Validator produziert binnen ~50 s von jedem
  anderen" für den ERSTEN Block pro Tick erhalten (Kommentar in
  `block_cadence.go`)? Verursachen die Zusatzblöcke ungewöhnliche
  Merge-Muster (viele gleichzeitige Tips, verzögerte Finality)?
- Abbruchkriterium: Finality-Checkpoint-Fortschritt stoppt oder
  verlangsamt sich sichtbar, Tip-Anzahl wächst unbegrenzt statt zu
  konvergieren.

**Danach:** mindestens 48h Dauerlast auf allen aktivierten Features
gleichzeitig, unbeaufsichtigt, mit Alerting auf genau die
Abbruchkriterien oben — bevor irgendeine dieser fünf Änderungen als
"staging-validated" gilt und der `NOT staging-validated`-Hinweis aus dem
jeweiligen Code-Kommentar entfernt wird.

## Rollback

- Die vier env-var-gesteuerten Features (`AEQUITAS_WAL_ENABLED`,
  `ENABLE_MULTI_BLOCK_TICK`) lassen sich durch Entfernen der Env-Var +
  Neustart abschalten — außer bei bereits WAL-durable, aber noch nicht
  nach Postgres geflushten Transfers: vor einem Rollback IMMER erst
  `FlushWALNow()` (oder gleichwertig: Knoten sauber stoppen, nicht
  kill -9, damit der Flush-Worker den Rest noch abarbeitet) abwarten,
  sonst gehen WAL-durable-aber-ungeflushte Transfers beim Zurückschalten
  auf den alten Pfad verloren.
- Die beiden immer-aktiven Fastpath-Änderungen (Shard-Lock-Transfer,
  Shard-Lock-Registrierung) haben keinen Feature-Flag-Rollback — deren
  Fallback ist der bestehende langsame Pfad bei Kontention, nicht
  abschaltbar. Ein Rollback bedeutet hier: den Commit revertieren.

## Was die lokale Simulation bereits gezeigt hat (diese Session)

Zwei echte `aequitasd`-Prozesse, getrennte Postgres-DBs, echte
Peer-zu-Peer-Verbindung über libp2p auf localhost, je ein fest
autorisierter Validator-Signing-Key pro Knoten (via direktem
`validator_keys`-Insert, s.o. — kein Ersatz für den echten
Autorisierungspfad, nur für den Konsens-Mechanik-Test selbst):

- Beide Knoten produzierten unabhängig eigene Blöcke, akzeptierten
  gegenseitig die Blöcke des anderen (`[DAG] ✓ Added peer block`,
  `[BLOCK-SYNC] ✓ Accepted block`), mergten Tips über GHOSTDAG
  (`[DAG] 🔀 Merged 2 tips`).
- Nach ~3 Minuten gleichzeitiger Produktion: `/api/status` auf beiden
  Knoten identisch bei `height=201`, identischer `latest_hash` — echte
  Konvergenz, kein Fork, keine Orphans, keine echten Block-Rejections
  (die einzigen "rejected"-Log-Zeilen waren die erwartete
  Sandbox-Netzwerk-Sperre gegen `aequitas.digital`, nicht Chain-Logik).
- `ENABLE_MULTI_BLOCK_TICK=1` auf einem Knoten aktiviert, Neustart,
  weiterlaufen lassen: Flag greift (Startup-Log bestätigt es), Knoten
  produziert unter Nicht-Backlog-Bedingungen weiterhin exakt 1 Block/Tick
  (korrektes Verhalten laut `produceBlocksForTick`), keine Regression.
  Der Backlog-Pfad selbst (>4 Zusatzblöcke/Tick) wurde NICHT unter echter
  Last ausgelöst — dafür bräuchte es einen echten 50k-TX-Backlog, den
  diese Sandbox nicht sinnvoll erzeugen kann. Bleibt offen für Staging
  Tag 5 oben.
- Nicht getestet (bewusst außerhalb des Sandbox-Rahmens): mehr als 2
  Knoten, echte Netzwerklatenz/-partition, adversariale/byzantinische
  Knoten, Mehrtage-Laufzeit, echte Diskhardware für WAL-Fsync, der echte
  ZK-Proof-Validator-Autorisierungspfad.

Kein Produktionscode wurde für diesen Test verändert — die bestehende
Validator-Sync-Mechanik (`LoadValidatorKeysIntoDAG`,
`syncValidatorsFromAllPeers`, `/api/validators`) funktionierte wie
entworfen, sobald ein Validator überhaupt autorisiert war. Die zuvor
beobachtete stille Ablehnung unbekannter Proposer war kein Bug, sondern
die korrekt funktionierende Sicherheitsgrenze, die genau das verhindert,
was das Projektziel "nur echte Menschen 1x als Validatoren" verlangt.

## Update — lokaler Solo-Node-Crashtest (2026-07-23, Ersatz für den geplanten Contabo-Staging-Lauf)

Die Sandbox konnte die echten Contabo-Boxen nicht erreichen (kein `ssh`
ausgehend erlaubt — Netzwerk-Policy der Umgebung, bestätigt über
`__agentproxy/status`, nicht durch fehlendes Tooling). Ersatzweise ein
echter, isolierter `aequitasd`-Prozess lokal: eigene Postgres-DB
(`aequitas_staging_local`), eigene `chain_id` in der Genesis, eigene Ports,
`AEQUITAS_WAL_ENABLED=1` + `ENABLE_MULTI_BLOCK_TICK=1`, echte signierte
`eth_sendRawTransaction`-Transfers über die eigene JSON-RPC-Schnittstelle
(nicht nur interne Go-Funktionsaufrufe wie in den Unit-Tests). Deckt nur
Tag 1-2 (Solo-WAL-Durability) ab, nicht Tag 3-4 (Mehrknoten-Konsens, bräuchte
einen zweiten Staging-Node).

**1) `ENABLE_MULTI_BLOCK_TICK`-Backlog-Pfad zum ersten Mal unter echtem
Rückstau ausgelöst** (vorher nur die reine Schleifenlogik mit einem
Fake-Producer unit-getestet, siehe `block_cadence_test.go`, und in der
2-Knoten-Simulation oben nie wirklich getriggert): 60.000 synthetische
`pending_txs` direkt eingefügt (deutlich über `maxTxsPerBlock`=50.000).
Ergebnis: exakt 2 Blöcke in einem Tick (50.000 + 10.000 TXs), danach
`still_pending=0` — kein TX verloren, keins doppelt eingeschlossen, die
Schleife stoppte korrekt nach dem ersten nicht-vollen Block statt bis
`maxExtraBlocksPerTick`=4 weiterzumachen.

**Neuer, ungünstiger Befund dabei:** der volle 50.000-TX-Block brauchte
in dieser (Cores mit Postgres geteilten) Sandbox **~1,8s** allein für
`ProduceBlock` (`SaveBlockWithPendingTxsAtomic` davon 822ms), der ganze
Tick (2 Blöcke) **~2,17s** — länger als `BLOCK_TIME`=1s selbst. Go's
`time.Ticker` verwirft verpasste Ticks statt sie aufzustauen, es gibt also
kein unbegrenztes Aufstauen — aber unter SUSTAINED (nicht nur kurzzeitigem)
schwerem Rückstau ist der real erreichbare Durchsatz eher "ein voller Block
alle ~2s" als die naive Grenze "bis zu 5 volle Blöcke pro `BLOCK_TIME`".
Auf dedizierter Produktions-Hardware (eigener DB-Host, kein Core-Sharing
mit Postgres) vermutlich besser, aber die grundsätzliche Erkenntnis — ein
maximal voller Block kann `BLOCK_TIME` selbst überschreiten — ist
architektonisch real und gehört in die Tag-5-Bewertung der echten
Staging-Kampagne.

**2) Echter Bug gefunden und gefixt beim WAL-Crash-Test:** zwei echte,
signierte Transfers über die WAL-Fastpath (`transferConcurrentWAL`), zweiter
davon per `kill -9` unmittelbar nach dem WAL-Append (vor dem nächsten
500ms-Flush-Tick) abgebrochen. Neustart: In-Memory-Saldo korrekt
wiederhergestellt (`recoverFromWAL` funktionierte wie erwartet) — aber
Postgres blieb dauerhaft veraltet (>15s beobachtet, kein Auto-Flush), weil
`recoverFromWAL` das wiederhergestellte Item zwar in die Flush-Queue legte,
aber nie `ensureWALFlushWorkerStarted()` aufrief. Ein frisch neugestarteter,
ansonsten idler Node hätte den Hintergrund-Flush-Worker also NIE gestartet
— das wiederhergestellte Transfer wäre auf Dauer im eigenen In-Memory-State
korrekt, aber niemals über `pending_txs` an andere Validatoren weitergereicht
worden: real ein stiller Konsens-Fork-Risiko-Fall (dieser Node's StateRoot
hätte die Mutation, jeder andere Node, der nur die tatsächlich empfangenen
TXs replayed, käme nie auf denselben Wert), nicht nur die im Code bereits
dokumentierte, harmlosere "Explorer-Sicht ist ein paar Sekunden alt"-Lag.

**Fix** (`transfer_wal.go`, `recoverFromWAL`): ruft jetzt `enqueueWALFlushLocked`
statt eines direkten `cs.walFlushQueue`-Appends auf — dieselbe Funktion,
die der normale WAL-Transferpfad benutzt, startet als Seiteneffekt auch den
Worker. Verifiziert: derselbe Crashtest lief danach sauber durch, Postgres
holte den korrekten Saldo innerhalb weniger Sekunden nach — ganz ohne eine
weitere, zufällig auslösende Transaktion. Neuer Regressionstest
`TestTransferConcurrentWAL_CrashRecovery_AutoFlushesWithoutManualTrigger`
(`transfer_wal_test.go`) — bewusst OHNE manuellen `FlushWALNow()`-Aufruf,
anders als der bestehende `..._UnflushedTransfersReconstructed`-Test, dessen
manueller Flush genau diese Lücke seit Einführung des WAL-Pfads verdeckt
hatte. Volle Testsuite (`go test ./... -race`) danach grün.

Zeigt genau, warum `AEQUITAS_WAL_ENABLED` weiterhin `NOT staging-validated`
bleibt, bis die echte mehrtägige Kampagne oben gelaufen ist — dieser eine
Bug wäre in keinem der bisherigen automatisierten Tests aufgefallen, nur
weil ein echter Prozess unter einer echten `kill -9` beobachtet wurde.
