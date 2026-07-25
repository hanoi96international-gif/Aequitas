# Vollständige Analyse: warum nichts merged (2026-07-25)

Status: **Kernursache eingegrenzt, eine entscheidende Messung fehlt noch.**
Dieses Dokument trennt strikt zwischen *bewiesen*, *ausgeschlossen* und
*Verdacht*. Nichts hier ist geraten; wo etwas offen ist, steht das explizit.

---

## A. Was heute tatsächlich kaputt war — zwei Fehler, beide selbst verursacht

### A1. 10-MB-Cap in `fetchBlocksSince` (behoben, `d590142`)

`sync_blocks.go` las Sync-Antworten mit `io.LimitReader(resp.Body, 10<<20)`.
`io.LimitReader` liefert bei Erreichen des Limits ein **fehlerfreies** `io.EOF`,
also sah abgeschnittenes JSON exakt aus wie eine vollständige, kaputte Antwort.

Live-Beleg (Contabo1, 15:59):

```
[HTTP-SYNC] ✗ Could not fetch page (min_height=1846753) from https://aequitas.digital:
full page failed (decoding response body (10485760 bytes): unexpected end of JSON input),
smaller fallback page also failed: decoding response body (10485760 bytes): ...
```

`10485760` = exakt `10<<20` — bei **beiden** Versuchen, auch beim Fallback auf
25 Blöcke. Verschärfend: `doSyncOnce` kehrt bei einem Fehler auf Seite 0 sofort
zurück (`return false`), **bevor** `advancePeerSyncHeight` und
`recordCleanSyncCycle` erreicht werden. Folge: `minHeight` bewegt sich nie,
dieselbe Seite scheitert ewig, `cleanSyncStreak` bleibt auf 0 und die
Produktionssperre („Not yet 3 consecutive clean sync cycles") öffnet nie.

Behoben durch Anheben auf `64<<20`. **Verifiziert:** danach loggt Contabo1
`[HTTP-SYNC] ✓ Added 314 new blocks from https://aequitas.digital`, keine
Truncation-Fehler mehr.

### A2. Parallelisierter Replay (zurückgenommen, `41b1eee`)

Die Parallel-Vorstufe in `replayTransactions` erzeugte live auf Contabo2 einen
Postgres-Protokoll-Desync:

```
[BLOCK] ✗ Could not save peer block #1849827 header before replay:
        pq: unexpected Parse response "(D) DataRow" — skipping
[REPLAY] ✗ Block #1849827: replay transaction commit failed
        (rolled back, block rejected): driver: bad connection
```

Exakt die Signatur, die `TestReplayTransactions_ParallelTransfers_RealDB` schon
vor dem Deploy gefunden hatte. Der Schutz war eine **lokale** `sync.Mutex` — die
serialisiert die Worker nur untereinander, aber nicht gegen andere Goroutinen
auf derselben `dag.state.activeTx`. Die behauptete Invariante bestand also nie.
Zurückgenommen; nach dem Revert sind diese Fehler weg.

---

## B. Aktueller Live-Zustand (16:51 UTC, beide auf `41b1eee`)

| | Contabo1 | Contabo2 |
|---|---|---|
| Höhe | 1850329 | 1850336 |
| Eigene Blockproduktion | **ja** (#1850326-1850329) | **ja** (#1850335/1850336) |
| „Merged N tips" | ja | ja |
| DB-Treiberfehler | keine | keine |
| StateRoot-Mismatches | **150 in 10 Min** | **5+ Alert aktiv** |
| Auto-Heal | **Resync gestartet** | — |

Beide Nodes **produzieren und mergen wieder**. Das eigentliche, verbleibende
Problem liegt eine Ebene tiefer.

---

## C. Das Kernproblem: dauerhafte StateRoot-Divergenz → Resync-Schleife

```
[REPLAY] ⚠ StateRoot mismatch on block #1850332 (claimed=f0b2e045a18590ac..., local=17d1751128376a40...)
[REPLAY] ⚠ StateRoot mismatch on block #1850333 (claimed=f0b2e045a18590ac..., local=17d1751128376a40...)
```

Zwei aufeinanderfolgende Blöcke, **identischer claimed- und identischer
local-Wert**. Das ist keine Race und kein Timing-Effekt: beide Seiten haben
einen *stabilen* Zustand, der sich dauerhaft unterscheidet.

Die Kaskade, die daraus folgt, ist die Instabilität die den ganzen Tag geprägt hat:

```
StateRoot-Mismatch  →  Auto-Heal erkennt "diverged"  →  Resync (Snapshot)
        ↑                                                      │
        └──────── Divergenz kehrt zurück ←─────────────────────┘
```

Contabo1 hat um 16:51 exakt diesen Resync erneut gestartet. Ein Resync um 16:39
hatte die Divergenz bereits nicht dauerhaft beseitigt.

---

## D. Woraus der StateRoot besteht — und was ausgeschlossen ist

`stateRootLocked` kombiniert:

1. `accountSetXOR` — XOR aller `accountLeaf`
2. `nullifierSetXOR`
3. Pool-Reserven
4. `last_ubi_at` aus `chain_config`

`accountLeaf` (state.go) verwendet: Adresse, `IsHuman`, `Balance`,
`TUsdBalance`, `LPShares` sowie zwei Flags.

**Ausgeschlossen (jeweils am Code verifiziert, nicht vermutet):**

- **WAL verändert den Leaf-Hash nicht.** `WALSeq` ist *nicht* Teil von
  `accountLeaf`. Die Spalte `wal_seq` ist rein additiv.
- **`TryLockDistribution` ist nicht die Ursache.** Der frühere Bug (Lock schrieb
  `last_ubi_at`) ist bereits behoben — es schreibt heute `distribution_lock_at`.
- **Die abweichenden `ubi_next_payout_secs` sind KEIN Beleg.** Dieser Wert kommt
  aus `SecondsUntilNextUBI()` → Schlüssel `next_ubi_at`. Das ist ein *lokaler
  Scheduler-Anzeigewert* und fließt **nicht** in den StateRoot ein. Nur
  `last_ubi_at` tut das. (Diese Spur sah überzeugend aus und ist falsch.)

---

## E. Hauptverdacht: `last_ubi_at` — begründet, aber noch nicht bewiesen

Entscheidendes Indiz: **alle wirtschaftlichen Aggregate stimmen exakt überein**,
auf allen drei Nodes:

| Feld | Primary | Contabo1 | Contabo2 |
|---|---|---|---|
| `total_supply` | 14000.00 AEQ | 14000.00 AEQ | 14000.00 AEQ |
| `total_humans` | 14 | 14 | 14 |
| `gini` | 0.19207266236750017 | 0.19207266236750017 | 0.19207266236750017 |
| `pool_treasury` | 0.2227 | 0.2227 | 0.2227 |
| `pool_ubi` / `pool_lp` / `pool_validators` | 0.0000 | 0.0000 | 0.0000 |

Wenn Kontostände, Pools und Supply identisch sind, der Root aber differiert,
dann liegt der Unterschied mit hoher Wahrscheinlichkeit in einer der Komponenten
**außerhalb** der Aggregate — und `last_ubi_at` ist die einzige davon, die ein
reiner Zeitstempel ist und historisch bereits genau diese Fehlerklasse erzeugt
hat. Der Code sagt das selbst (state.go, `ApplyUBIDelta`):

> „`last_ubi_at` feeds directly into StateRoot(), so every secondary
> independently calling `time.Now()` here wrote a different value than the
> primary and than every OTHER secondary — guaranteeing a StateRoot mismatch on
> every single UBI distribution."

Dieser konkrete Pfad wurde gefixt. Offen ist, ob ein **anderer** Pfad
`last_ubi_at` noch mit lokaler Zeit schreibt, oder ob der Wert bei Snapshot-
Import/Replay nicht korrekt übernommen wird.

**Restverdacht Nummer 2:** `nullifierSetXOR`. Nullifier sind Teil des Snapshots,
tauchen aber in keinem Aggregat oben auf — eine Abweichung dort wäre ebenfalls
unsichtbar und würde exakt dieses Bild erzeugen.

---

## F. Zusätzlicher, unabhängiger Befund: Konfiguration ist NICHT einheitlich

`enable-wal-contabo2.yml` setzt **ausschließlich auf Contabo2**:

```
AEQUITAS_WAL_ENABLED=1
AEQUITAS_WAL_PATH=/data/wal/aequitas_transfers.wal
ENABLE_MULTI_BLOCK_TICK=1
```

Zwei davon sind verhaltensrelevant:

- **`ENABLE_MULTI_BLOCK_TICK`** — im Code ausdrücklich markiert als
  *„NOT STAGING-VALIDATED: multi-node consensus timing can't be fully validated
  by single-process testing"*. Contabo2 produziert bei Backlog mehrere Blöcke
  pro Tick, Primary und Contabo1 nur einen.
- **WAL-Fastpath** — überspringt per Design Demurrage-Settlement und
  Wealth-Cap-Crediting (`transferConcurrentWAL`). Das ist genau die
  „WAL-Fastpath-Demurrage-Lücke", die bereits dokumentiert und **bewusst noch
  nicht gefixt** wurde.

Solange ein Node andere Konsensregeln fährt als die anderen beiden, ist jede
StateRoot-Analyse mit einem Störfaktor belastet. **Diese Ungleichheit muss weg,
bevor irgendetwas anderes sinnvoll bewertet werden kann.**

Wichtig zur Richtung: WAL **überall einzuschalten** würde die bekannte
Demurrage-Lücke auf alle Nodes ausrolle­n. Der saubere Weg ist daher, die drei
Flags zunächst **überall abzuschalten**, Konsens-Stabilität herzustellen, und
WAL erst danach — mit geschlossener Demurrage-Lücke — flächendeckend zu
aktivieren.

---

## G. Die eine Messung, die den Fall entscheidet

Read-only, keine Zustandsänderung. Auf allen drei Nodes vergleichen:

```sql
SELECT key, value FROM chain_config
 WHERE key IN ('last_ubi_at','next_ubi_at','distribution_lock_at');
SELECT count(*) FROM chain_nullifiers;
SELECT address, balance, tusd_balance, lp_shares, is_human
  FROM chain_accounts ORDER BY address;
```

Damit ist eindeutig bestimmbar, **welche** der vier StateRoot-Komponenten
differiert. Danach ist der Fix gezielt statt geraten.

---

## H. Vorgeschlagene Reihenfolge

1. **Konfiguration vereinheitlichen** — `AEQUITAS_WAL_ENABLED`,
   `AEQUITAS_WAL_PATH`, `ENABLE_MULTI_BLOCK_TICK` auf Contabo2 abschalten, damit
   alle drei Nodes identische Regeln fahren. (Ohne Code-Deploy möglich.)
2. **Messung aus G** ausführen und die divergierende Komponente benennen.
3. **Gezielter Fix** genau dieser Komponente, mit Regressionstest.
4. Erst danach: Auto-Heal-Klassifizierung nachschärfen — der Zustand
   „eingefroren und hinterher" wird aktuell als „holt noch auf" gewertet
   (`Chain-divergence self-check skipped ... still catching up`), weshalb
   Auto-Heal in echten Stillständen nicht eingreift.
5. Erst danach: WAL sauber auf alle Nodes, mit geschlossener Demurrage-Lücke.
6. Erst danach: 50k-TPS-Arbeit fortsetzen.

---

## I. Was heute schiefgelaufen ist (Prozess)

- Jeder Push auf `main` startet **alle drei Nodes** neu (Railway-Auto-Deploy +
  beide Contabo-Workflows, ohne `paths`-Filter). Jeder Neustart reißt eine
  Lücke von 200-350 Blöcken auf. Bei vielen Pushes pro Stunde entsteht daraus
  Dauer-Instabilität, die echte Fehler überdeckt.
  **Empfehlung:** `paths-ignore` für Doku-/Workflow-only-Änderungen ergänzen.
- Zwei meiner eigenen Fixes haben je einen neuen Fehler eingeführt (A1, A2).
  Beide wären durch einen Test gegen eine echte, große Live-Antwort bzw. eine
  echte DB vor dem Deploy auffindbar gewesen — im Fall A2 *war* der Test sogar
  vorhanden und sein Ergebnis wurde falsch interpretiert.
