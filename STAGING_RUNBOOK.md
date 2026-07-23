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

## Update — lokaler 2-Knoten-Konsenstest mit WAL + Multi-Block-Tick gleichzeitig (2026-07-23)

Direkter Folgeversuch zum Solo-Node-Test oben: zwei echte, per libp2p
direkt verbundene `aequitasd`-Prozesse (`BOOTSTRAP_P2P_ADDR`, feste
`NODE_KEY` für Knoten A, damit seine Adresse vorab bekannt ist; beide
lokal gestartet, um die Lücke zwischen Boot A und Boot B auf
Millisekunden zu drücken), eigene Postgres-DB pro Knoten, beide
Signing-Adressen vorab direkt in `validator_keys` beider DBs eingetragen
(derselbe, in diesem Dokument bereits als "kein Ersatz für den echten
Autorisierungspfad" markierte Shortcut). Beide mit
`AEQUITAS_WAL_ENABLED=1` + `ENABLE_MULTI_BLOCK_TICK=1` — diese Kombination
wurde vorher nie zusammen über zwei Knoten getestet.

**Wichtige Grenze, hart bestätigt statt nur vermutet:** `isAllowedPeerURL`
(`sync_blocks.go`) verweigert jede Loopback-/private Peer-URL für den
HTTP-Sync-/Catch-up-Pfad — zu Recht, das ist die SSRF-/DNS-Rebinding-Härtung,
die Produktion erst sicher macht. Konsequenz für diesen Sandbox-Test: die
libp2p-Gossip-Verbindung selbst funktioniert über `BOOTSTRAP_P2P_ADDR`
einwandfrei (neue, fortlaufend produzierte Blöcke kommen an), aber
JEDE Catch-up-Notwendigkeit — ein fehlender Ancestor-Block, egal aus
welchem Grund — kann von diesem lokalen Setup NIE aufgeholt werden, weil
der einzige konfigurierbare HTTP-Sync-Peer für einen Knoten mit
`localhost`-Adresse kategorisch abgelehnt wird. Ein einziger abgelehnter
Block reicht: jeder DARAUF aufbauende Folgeblock des Verursachers wird als
Orphan geparkt ("missing parent"), und ohne erreichbaren HTTP-Sync-Peer
bleibt er das für immer — ein permanenter Fork, den echte Produktions-Knoten
(mit echten öffentlichen HTTPS-Peers) so nicht hätten, weil deren
Catch-up-Pfad tatsächlich etwas zum Fragen hat. Bestätigt damit konkret,
warum STAGING_RUNBOOK.md von Anfang an eigene, öffentlich erreichbare
Infrastruktur verlangt statt Loopback-Tests — das ist kein Vorsichtsprinzip
mehr, sondern hier direkt reproduziert.

**Zwei Fehlversuche, beide auf eigene Testmethodik zurückgeführt, nicht auf
Produktionscode** (der Reihe nach durchleuchtet, damit klar ist, was
NICHT der Bug war):
1. Synthetischer 60k-Zeilen-Backlog direkt in `pending_txs` eingefügt
   (wie im Solo-Test) — auf EINEM Knoten produzierte das einen Block mit
   50.000 "Transfer"-Einträgen, die nie über echte Transferlogik gelaufen
   waren (nur die Tabelle direkt beschrieben, kein `cs.accounts` je
   berührt). Der Peer replayte den Block ehrlich, fand die Absenderadresse
   nirgends und lehnte korrekt mit "genuine state-inconsistency failure"
   ab — durch die oben beschriebene Catch-up-Grenze dann permanent
   verzweigt. Richtiges, sicheres Verhalten des Codes; falsche Testdaten
   meinerseits (echte Transfers erzeugen ihren `pending_txs`-Eintrag immer
   NACH einer bereits angewendeten Mutation, nie davor/unabhängig davon).
2. Echte, signierte Transfers, aber Startguthaben nur in Knoten A's DB
   direkt eingefügt, nicht in Knoten B's. Gleicher Effekt: Knoten B replayte
   ehrlich, kannte die Absenderadresse nicht (dort nie gesehenes Konto) und
   lehnte korrekt ab. Wieder kein Bug — ein Konto, dessen Guthaben nur auf
   EINEM Knoten per Rohzugriff "aus dem Nichts" entsteht, ist für keinen
   ehrlich replayenden Peer verifizierbar, genau wie es sein soll.

**Sauberer, korrekt aufgesetzter Durchlauf (identisches Startguthaben auf
BEIDEN DBs eingefügt, damit beide Knoten dieselbe Ausgangslage unabhängig
verifizieren können):** zwei echte `eth_sendRawTransaction`-Transfers
(zweiter davon über den WAL-Fastpath), Knoten B replizierte Saldo UND
StateRoot exakt identisch zu Knoten A (`balance:15` auf beiden, gleicher
`latest_hash` bei gleicher Höhe), null Reject-/Orphan-Zeilen im Log. Das
ist die eigentlich gesuchte Bestätigung: ein echter, WAL-fastpath-basierter
Transfer repliziert korrekt über zwei per libp2p verbundene, unabhängige
Knoten — die im Solo-Test gefixte Recovery-Lücke hat hier keine neue
Zwillingslücke im Cross-Node-Pfad aufgezeigt.

**Nicht mehr versucht:** ein echter Multi-Block-Tick-Backlog-Burst
(>50.000 TXs) mit AUSSCHLIESSLICH validen, echt signierten Transaktionen
über zwei Knoten — dafür bräuchte es tausende einzelne RPC-Calls (zu
langsam für diese Sandbox) oder eine zweite, gegen dieselbe DB schreibende
Go-Instanz (verboten, siehe "Test-Setup-Grenzen" — echte Produktion hat
immer nur einen Schreiber pro DB). Die Backlog-Drain-MECHANIK selbst ist
bereits im Solo-Test oben unter echter Last bewiesen; was hier fehlt, ist
nur die zusätzliche Bestätigung, dass ein *Burst* mehrerer echter Blöcke
sich nicht anders verhält als einzelne — strukturell kein Grund, das zu
erwarten (jeder Block wird beim Gossip/Replay einzeln behandelt), aber
ungemessen.

## Update — Contabo2-Live-Versuch: WAL/Multi-Block-Tick kam NIE tatsächlich an (2026-07-23)

Eine vorherige Session dieser Session-Kette hat `enable-wal-contabo2.yml`
tatsächlich gegen den echten Contabo2-Produktionsknoten ausgeführt — vor
der eigentlich vorgeschriebenen, mehrtägigen Staging-Kampagne oben (das
war so nicht geplant und wird hier explizit als Abweichung festgehalten,
nicht als akzeptierter Pfad). Zwei Verify-Läufe direkt danach fanden
durchgehend **keine einzige** `[WAL]`/`MULTI_BLOCK_TICK`-Log-Zeile und
keine WAL-Datei auf der Platte — trotz korrekt gesetzter Flags in
`/root/.aequitas.env`. Root Cause jetzt zweifelsfrei bestätigt (nicht nur
vermutet), durch direktes, read-only `cat` von `/root/deploy_safe_c2.sh`
über einen dritten, gezielt dafür erweiterten Verify-Lauf:

```bash
docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' > /root/.aequitas_env_backup
# ... docker stop/rm ...
while IFS= read -r line; do
  [ -z "$line" ] && continue
  case "$line" in
    PATH=*|HOSTNAME=*|HOME=*|RESYNC_FROM_SNAPSHOT=*) continue ;;
  esac
  ENV_ARGS+=(-e "$line")
done < /root/.aequitas_env_backup
docker run -d --name aequitas-node ... "${ENV_ARGS[@]}" ... aequitas-node:new
```

`deploy_safe_c2.sh` übernimmt die Env-Variablen für den NEUEN Container
ausschließlich aus `docker inspect` des ALTEN, gerade laufenden
Containers — **niemals** aus `/root/.aequitas.env`. Das ist bewusst so
gebaut (der Datei-Pfad würde das dauerhaft gesetzte
`RESYNC_FROM_SNAPSHOT=true` mitziehen und bei jedem Deploy die DB
resetten), hat aber als Nebenwirkung: **jede Änderung an `.aequitas.env`
ist für Contabo2 wirkungslos, unabhängig vom Timing.** `enable-wal-` und
`rollback-wal-contabo2.yml` editieren exakt diese Datei und erwarten,
dass `deploy_safe_c2.sh` sie einliest — das war nie der Fall. Verifiziert
direkt: `docker inspect aequitas-node` zeigt, dass keine der drei Flags im
tatsächlich laufenden Container-Prozess-Env ankommt, obwohl die Datei sie
enthält.

Zusätzlicher Beitrag zum ursprünglichen Rätsel "Container-Neustart
zwischen den Verify-Läufen, obwohl niemand `enable`/`rollback` erneut
ausgelöst hat": `deploy-contabo2.yml` deployt automatisch bei JEDEM Push
auf `main` (Test-Gate + `deploy_safe_c2.sh`), und diese Session hat
während der laufenden Diagnose mehrfach nach `main` gepusht — jeder dieser
Auto-Deploys erzeugt über `docker rm`+`docker run` einen fabrikneuen
Container (`RestartCount` bleibt deshalb immer 0, auch bei tatsächlichem
Neustart) und rennt dabei mit dem manuellen Enable-Versuch um die Wette.
Kein Crash-Loop, kein OOM, keine fremde Cron/Systemd-Quelle — direkt
ausgeschlossen (leere `crontab -l`, keine passenden `systemctl
list-timers`, `RestartPolicy=unless-stopped MaxRetries=0`). Die
Deploy-Run-Historie (`deploy-contabo2.yml`) bestätigt die beobachteten
`StartedAt`-Zeitstempel auf die Sekunde genau als reguläre, push-getriggerte
Deploys, nicht als unerklärten Absturz.

**Fazit:** WAL/Multi-Block-Tick war auf Contabo2 zu keinem Zeitpunkt
tatsächlich aktiv, trotz des Live-Versuchs — die Ledger-Durability-Semantik
wurde nicht verändert, kein zusätzliches Produktionsrisiko eingegangen.
Das eigentliche Problem ist ein reiner Deploy-Tooling-Bug (Env-Herkunft),
keine Aussage über die WAL-Implementierung selbst.

**Separater, neuer Befund aus demselben Log (nicht WAL-bezogen, aber
Postgres-Verbindungsstabilität betreffend, sollte vor jedem echten
Staging-Versuch mit-untersucht werden):** drei `[REPLAY] ✗ Block #...:
replay transaction commit failed (rolled back, block rejected): driver:
bad connection`-Zeilen im selben Log-Fenster. Das ist der bestehende,
synchrone Postgres-Pfad (nicht WAL), der Blöcke bei einem verlorenen
DB-Connection sauber zurückweist statt inkonsistenten Zustand zu
committen (korrektes Verhalten) — aber die Häufigkeit auf einem
Produktionsknoten ist ein eigenes offenes Follow-up, nicht Teil dieser
Diagnose.

## Update — Fix angewendet, live WAL-Aktivierung versucht, dann bewusst zurückgerollt (2026-07-23, Fortsetzung)

Auf explizite Nutzerfreigabe hin wurde `deploy_safe_c2.sh` tatsächlich per
SSH gepatcht (`patch-deploy-safe-c2.yml`, neu: Backup mit Zeitstempel,
Schreiben in eine `.new`-Staging-Datei, `bash -n`-Syntaxcheck VOR jeder
Installation, erst danach atomarer `mv`). Patch v1: `.aequitas.env` wird
jetzt zusätzlich zum geerbten Container-Env eingelesen, Datei-Werte
gewinnen bei Kollision. `enable-wal-contabo2.yml` danach ausgeführt —
**zum ersten Mal überhaupt** zeigte der Boot-Log echte `[WAL]`-Zeilen
(`[WAL] ✓ WAL fast path active for eligible transfers`) und die
`ENABLE_MULTI_BLOCK_TICK=1`-Warnung.

**Zwei neue Befunde direkt danach, beide ernst:**

1. **WAL-Datei ist nicht persistent — `docker run` in `deploy_safe_c2.sh`
   hat keinerlei `-v`-Volume-Mount.** Der WAL-Prozess läuft vollständig im
   Container (`os.OpenFile` mit `O_CREATE` legt die Datei im
   Container-eigenen, schreibbaren Layer an, nicht auf dem Host). Da
   `deploy-contabo2.yml` bei JEDEM Push auf `main` automatisch
   `docker stop && docker rm && docker run` ausführt (frischer Container,
   nichts vom alten überlebt), würde jede WAL-durable, aber noch nicht
   nach Postgres geflushte Transaktion beim nächsten Redeploy
   ersatzlos verloren gehen — ohne Fehler, ohne Möglichkeit für
   `recoverFromWAL`, das Replay durchzuführen (die Datei existiert nach
   `docker rm` schlicht nicht mehr). Das ist ein Risiko, das dieses
   Dokument und SCALING_ARCHITECTURE.md vorher nicht kannten (bisherige
   Sorgen: Fsync-Durchsatz, GC-Pausen — nicht "die WAL-Datei überlebt die
   eigene routinemäßige Redeploy-Pipeline nicht").
2. **Rollback v1 hat nicht wirklich abgeschaltet.** Patch v1 hat
   `.aequitas.env` nur ÜBER den geerbten Container-Env gelegt, nie das
   Entfernen einer Zeile aus der Datei als "diesen Key jetzt entfernen"
   interpretiert. `rollback-wal-contabo2.yml` entfernt die drei Zeilen aus
   der Datei — aber der ALTE Container (der gerade ersetzt wird) hatte sie
   noch gesetzt, also wurden sie unverändert weitervererbt. Live bestätigt:
   nach einem vollständigen Rollback-Lauf zeigte der nächste Boot-Log immer
   noch `[WAL] ✓ WAL fast path active`. **Fix (v2):** die drei Flags
   (`AEQUITAS_WAL_ENABLED`, `AEQUITAS_WAL_PATH`, `ENABLE_MULTI_BLOCK_TICK`)
   sind jetzt zusätzlich vom geerbten-Env-Durchlauf ausgeschlossen (gleiche
   Behandlung wie `PATH`/`HOSTNAME`/`HOME`/`RESYNC_FROM_SNAPSHOT`) — sie
   kommen ab sofort ausschließlich aus `.aequitas.env`, jeder Deploy neu,
   kein Vererben vom Vorgänger-Container. Vor dem Ausrollen lokal mit zwei
   Szenarien (Datei enthält die Flags / Datei enthält sie nicht) verifiziert.

**Reaktion:** Wegen Punkt 1 (echtes Datenverlustrisiko auf einem
produktiven, geldbewegenden Ledger) wurde WAL sofort wieder deaktiviert —
`rollback-wal-contabo2.yml` erneut ausgeführt, diesmal mit dem v2-Patch,
und über einen weiteren Verify-Lauf bestätigt: **null** `[WAL]`-Zeilen im
gesamten Boot-Log, keine `ENABLE_MULTI_BLOCK_TICK`-Warnung, Höhe steigt
normal (+10 in 5s), `[DAG] 🔀 Merged N tips`-Zeilen laufen wieder
regelmäßig. Contabo2 läuft wieder komplett auf dem alten, monatelang
stabilen synchronen Pfad — WAL/Multi-Block-Tick sind aus, bestätigt, nicht
nur angenommen.

**Nebenbefund, der zwischenzeitlich als "Nodes mergen nicht mehr richtig"
gemeldet wurde:** deckt sich mit `ENABLE_MULTI_BLOCK_TICK=1` (Code-Kommentar
des Feature-Flags selbst: "NOT staging-validated, multi-node
consensus-timing change") — vermehrte `queued as orphan`/Reject-Zeilen im
Log-Fenster, während das Flag aktiv war. Mit dem bestätigten Rollback ist
das mit-behoben; eine tiefere Ursachenanalyse (ob multi-block-tick allein
oder das Zusammenspiel mit dem WAL-Fastpath der Auslöser war) steht noch
aus, ist aber jetzt kein akutes Problem mehr, da beide Flags aus sind.

**Aktueller Stand (dieser Absatz war vorher der Schlusspunkt):** `deploy_safe_c2.sh` liest `.aequitas.env` jetzt
korrekt (v2, verifiziert). Der Datei-Mechanismus für enable/rollback
funktioniert damit endlich wie ursprünglich gedacht.

## Update — Persistenz-Lücke geschlossen, WAL + Multi-Block-Tick jetzt real und dauerhaft aktiv auf Contabo2 (2026-07-23, Fortsetzung 2)

Punkt 1 von oben (WAL-Datei überlebt keinen `docker rm`) behoben:
`deploy_safe_c2.sh` v3 bindet jetzt ein dediziertes Host-Verzeichnis
(`/root/aequitas-wal-data`) nach `/data/wal` im Container — dieses
Verzeichnis liegt außerhalb des Container-eigenen, bei jedem Deploy
gelöschten Schreib-Layers und übersteht `docker rm` genauso wie das
Image selbst. `AEQUITAS_WAL_PATH` zeigt jetzt standardmäßig auf
`/data/wal/aequitas_transfers.wal`, innerhalb dieses Mounts.

**Live verifiziert, mit echtem Beweis statt nur Konstruktion:**
- Nach `enable-wal-contabo2.yml`: `ls -la` auf dem HOST (nicht im
  Container) zeigt die WAL-Datei tatsächlich unter
  `/root/aequitas-wal-data/aequitas_transfers.wal` — vorher, ohne Mount,
  war das strukturell unmöglich (Datei lag nur im Container-Layer).
- Boot-Log zeigt `[WAL] AEQUITAS_WAL_ENABLED=1 — opening
  /data/wal/aequitas_transfers.wal` und `[WAL] ✓ WAL fast path active für
  eligible transfers` — WAL nutzt jetzt nachweislich den persistenten Pfad.
- **Zweiter Redeploy-Zyklus zur echten Persistenz-Probe:**
  `enable-wal-contabo2.yml` ein zweites Mal ausgeführt (idempotent, aber
  löst denselben `docker stop && docker rm && docker run` aus wie jeder
  reguläre Redeploy). Danach erneut verifiziert: WAL öffnet denselben
  Pfad sauber neu, keine Fehler, keine Neuerstellung von Grund auf,
  Höhe steigt normal weiter (+5 in 5s), `[DAG] 🔀 Merged N tips`-Zeilen
  laufen weiter regelmäßig. Das ist der konkrete Beweis, dass die Datei
  einen echten Container-Neustart überlebt — nicht nur die theoretische
  Docker-Bind-Mount-Garantie.

**Ergebnis:** WAL (`AEQUITAS_WAL_ENABLED=1`) und Multi-Block-Tick
(`ENABLE_MULTI_BLOCK_TICK=1`) laufen jetzt echt, dauerhaft und
redeploy-sicher auf Contabo2. Das Deploy-Tooling ist vollständig
funktionsfähig (Flags werden zuverlässig gesetzt/entfernt, WAL-Datei
übersteht jeden künftigen Redeploy inklusive der automatischen bei jedem
Push auf `main`). Contabo1 bleibt bewusst unverändert (Fallback,
STAGING_RUNBOOK.md-Prinzip "ein Knoten nach dem anderen").

**Weiterhin unverändert wahr, nicht durch diese Fixes ersetzt:** die
eigentlich vorgeschriebene mehrtägige Staging-Kampagne dieses Dokuments
hat nicht stattgefunden — diese Aktivierung ist live auf Produktion,
nicht in einer separaten Staging-Umgebung. Beide Flags bleiben in ihrem
eigenen Code als "NOT staging-validated" markiert; dieser Status ändert
sich durch das jetzt korrekte Tooling nicht, nur das *Risiko, das der
Tooling-Bug selbst hinzugefügt hätte* (Datenverlust bei Redeploy) ist
geschlossen. Laufende Beobachtung (insbesondere State-Root-Konsistenz
zwischen Contabo1/Contabo2 und die `[REPLAY] driver: bad connection`-Rate)
bleibt sinnvoll.
