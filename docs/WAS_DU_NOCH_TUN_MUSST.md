# Was du noch tun musst

**Stand 29.08.2026, 09:00 UTC.** Alles, was ohne dich geht, ist erledigt und
live geprüft. Hier steht nur, was an einem Zugang, einer Unterschrift oder
einer Entscheidung von dir hängt.

Die Reihenfolge ist nach Dringlichkeit für die Beta sortiert.

---

## 1 · Verifier-Image — **erledigt am 29.08.**

Das Paket `aequitas-biometric-beta/matching` ist öffentlich. Nachgeprüft: das
Manifest lässt sich **ohne jede Anmeldung** abrufen (`200`). Damit ist der
Leitfaden „Run a Verifier" zum ersten Mal für Fremde ausführbar — und der Weg
zu einem dritten Betreiber offen.

Geprüft war vorher, dass dabei nichts mitgeht: das Dockerfile kopiert nur
`requirements.txt`, `app/` und `scripts/`; `poh_beta.db`, `sample_test_images`
und `models/` bleiben draußen, und im ausgelieferten Quelltext stecken keine
Zugangsdaten.

---

## 2 · Der Testmodus ist entschieden — und jetzt abgesichert

Du hast entschieden: die Beta startet bewusst mit `SERVICE_MODE=test`.

**Dabei ist ein Geldschöpfungspfad aufgefallen, der jetzt verschlossen ist.**
Die Galerien sind strikt nach Modus getrennt — `sketch_gallery.py` sagt es
selbst, *„eine echte Einschreibung wäre im Testmodus unsichtbar"*. Umgekehrt
gilt dasselbe:

1. Jede Beta-Anmeldung landet nur in der **Test**-Galerie. Die Kette zahlt
   dafür den Registrierungszuschuss.
2. Beim späteren Umstellen auf `real` ist die **echte** Galerie leer.
3. Dieselbe Person meldet sich erneut an → kein Kandidat → `new_enrollment` →
   frisch gewürfelter `bio_hash` (`secrets.randbelow`) → **neuer Nullifier** →
   **zweiter Zuschuss**. Einer je Beta-Teilnehmer.

Der Nullifier-Schutz der Kette greift nicht: er hängt genau an dem Hash, der
neu gewürfelt wurde. Dieselbe Klasse wie die Löschfunktion am 24.08.

**Jetzt live auf beiden Boxen:** ein Tor weist den Realmodus ab, solange die
Realgalerie leer ist *und* Testeinschreibungen bestehen. `/matching/health`
zeigt den Zustand — aktuell `test_galerie: 1`, `real_galerie: 0`,
`wuerde_realmodus_abweisen: true`.

### Wenn du später auf real umstellst

Entscheide **zuerst**, was mit den Testeinschreibungen geschieht — übernehmen
oder verwerfen. Ich kopiere sie bewusst nicht automatisch: sie wurden unter
einer anderen Einwilligungsfassung und bei geschlossener Rechtsschranke
erhoben. Danach auf beiden Boxen:

```
REAL_MODE_START_ACCEPTED="<warum, mit Datum>"
```

Ein leerer Wert genügt nicht. Zusätzlich verlangt der Realmodus weiterhin
`ALLOW_REAL_BIOMETRIC_DATA=true` **und** `LEGAL_SIGNOFF_DATE` — und `gate.py`
sagt selbst, dass dieses Datum erst nach abgeschlossener Phase-2-Rechtsprüfung
(DSGVO Art. 9, Einwilligungsablauf, Löschkonzept) gesetzt werden darf. Das ist
keine Konfiguration, sondern eine Behauptung über eine stattgefundene Prüfung.

---

## 3 · C1 als Validator — **erledigt am 29.08.**

Das Register trägt jetzt **beide** Knoten, jeweils mit Bezeugungsschlüssel und
Vergleichsdienst-Adresse:

| Betreiber-Wallet | Bezeuger | Vergleichsdienst |
|---|---|---|
| `0x0be8b961…` (C1) | `222ad549…` | `https://proof1.aequitas.digital/matching` |
| `0x1a37dcda…` (C2) | `207b2894…` | `https://proof2.aequitas.digital/matching` |

---

## 4 · Proof-Server-Auslieferung — **erledigt am 29.08.**

Du hast `CONTABO_SSH_KEY` und `CONTABO2_SSH_KEY` gesetzt. Der erste Lauf ist
durchgelaufen: **success**. Damit funktioniert der Auslieferungsweg des
Proof-Servers zum ersten Mal überhaupt — vorher fehlten sowohl die Secrets als
auch das aufgerufene Skript.

Ab jetzt genügt:

```bash
gh workflow run deploy.yml --repo hanoi96international-gif/aequitas-proof-server
```

Das Skript (`deploy/proof-deploy.sh`) sichert vorher die Konfiguration des
laufenden Containers, baut unter einem neuen Tag und nimmt automatisch zurück,
wenn `/health` danach nicht antwortet.

---

## 5 · Impressum und Datenschutz — deine Entscheidung, bereits umgesetzt

Du hast entschieden, dass es kein Impressum gibt. Das ist sauber umgesetzt:
`/impressum` und `/datenschutz` antworten mit 404, und **keine Seite der
Website verlinkt sie** — es gibt also keinen toten Verweis. Die sieben
`LEGAL_*`-Felder bleiben leer.

Falls du es später doch willst, sagt `https://aequitas.digital/api/legal-status`
genau, welche Felder fehlen und was sie bedeuten.

---

## 6 · Heute 12:09 UTC — Übertrag prüfen (2 Minuten)

Das Tor `POOL_REMAINDER_CARRY_FROM_UNIX` öffnet um **12:09 UTC**. Danach:

```bash
curl -s https://aequitas.digital/api/health/combined | python -c "import sys,json;r=json.load(sys.stdin)['chain']['supply_reconciliation'];print(r['difference'],r['beyond_known_gap'],r['supply_alarm'])"
```

**Erwartet:** `-0.000014 -0.000014 False` — unverändert.

Wichtig: die Differenz darf **nicht** auf 0 zurückgehen. Die 0,000014 AEQ sind
vernichtet, nicht verlegt; eine Rückkehr auf 0 hieße, dass irgendwo neu
geschöpft wurde. Erst wenn der Wert nach dem Tor stabil bleibt, darf die
Grundlinie der Überwachung eingefroren werden.

---

## Am 29.08. nachmittags noch geschlossen

**Die letzte handgepflegte Liste ist weg.** Die MPC-Mitgliedschaft lief über ein
statisches `MPC_PEERS` auf beiden Boxen — ein dritter Betreiber hätte dort von
Hand eingetragen werden müssen. Beide laufen jetzt im Entdeckungsmodus und
melden es selbst:

> `[MPC] serving /mpc/exchange as 0x…; committee of 2 drawn from the chain,
> membership resolved per registration`

Vorher geprüft, dass das nicht MPC still abschaltet: der damalige Grund für die
feste Liste war, dass `mpc_ready` nie **gesendet** wurde. Das ist behoben
(`sync_blocks.go` sendet es, die Gegenseite speichert es über
`RegisterWithMPC`). `MPC_PEERS` und `MPC_PARTY_INDEX` sind aus dem Prozess
**und** aus `.aequitas.env` verschwunden.

**SSH: Passwort-Login ist auf beiden Boxen zu.** Vorher nahmen beide
Root-Login per Passwort aus dem offenen Netz an — bei 69.026 bzw. 62.135
Rateversuchen von je rund tausend IPs, laufend, ohne fail2ban. Das war der
einzige reale Weg an den Wallet-Schlüssel; über die Anwendung ist er nicht
erreichbar (kein Endpunkt gibt die Umgebung aus, nie geloggt, nicht im Git,
env-Datei `600 root`, kein Container mit Docker-Socket).

Die Falle dabei: `/etc/ssh/sshd_config.d/50-cloud-init.conf` machte das
Passwort wieder auf. Nur die Hauptdatei zu ändern hätte **nichts** bewirkt.

---

## Was bewusst offen bleibt

- **MPC-Schwelle nicht kalibriert.** Braucht eine zweite Person vor der Kamera;
  ohne echten Doppelversuch lässt sich keine Schwelle belegen.
- **Quorum 2 bei zwei laufenden Vergleichsdiensten.** `/health` sagt das jetzt
  selbst (`bezeuger_bedeutung`) und warnt beim Gleichstand. Echter Ausfallschutz
  braucht einen dritten Betreiber — und der braucht Punkt 1.

---

## Aufgefallen: beide Boxen tragen den Wallet-Schlüssel ihres Betreibers

Auf C1 **und** C2 ist die Signieradresse des Knotens identisch mit
`NODE_OPERATOR_WALLET`:

| Box | Signieradresse = Betreiber-Wallet |
|-----|-----------------------------------|
| C1  | `0x0be8b961…` |
| C2  | `0x1a37DcDa…` |

Der private Schlüssel deiner personengebundenen Wallet liegt damit auf einem
Server am offenen Netz. Wer eine Box übernimmt, übernimmt die Identität
mitsamt Guthaben — und die dreifache Validator-Prüfung schrumpft dort auf
eine, weil „Mensch autorisiert" und „Schlüsselbesitz" derselbe Schlüssel sind.

**Kein Beta-Blocker, aber vor echtem Betrieb zu trennen:**
`RELAYER_PRIVATE_KEY` sollte ein eigener, nur für den Knoten erzeugter
Schlüssel sein; `NODE_OPERATOR_WALLET` bleibt einfach die Adresse, an die
Belohnungen gehen — dafür braucht der Server keinen Schlüssel.

Das ist bewusst nicht heute geändert: ein neuer Signierschlüssel ändert die
Blockproduktions-Identität und macht C2s Registereintrag ungültig. Das gehört
in ein ruhiges Fenster, nicht in die Nacht vor dem Start.

---

## Erste echte Sperren-Messung, 29.08.2026

Bis heute war der Engpass geraten — dreimal, dreimal falsch. Jetzt gemessen
(Go-Mutex-Profil auf C1, unter Last von C2):

| Anteil an der Wartezeit | Ort |
|---|---|
| **74,7 %** | `processTransferBatch` → `runAtomicWithOutbox` |
| 19,1 % | `flushWALBatch` → `shardedAccounts.LockAddrs` |

`runAtomicWithOutbox` hält die **globale** Zustandssperre `cs.mu` über eine
komplette Datenbank-Transaktion, `tx.Commit()` eingeschlossen. Das ist
Absicht — der Kommentar daneben erklärt, dass Speicherzustand und DB-Transaktion
sonst auseinanderlaufen. Der Preis ist die Serialisierung: jede Überweisung
wartet auf jede andere, chainweit.

Das erklärt alles, was vorher widersprüchlich aussah: nur ein Kern von sechs
ausgelastet, mehr Sender ohne Wirkung, einbrechende Blockproduktion.

**Der Weg zu 10k steht im Code selbst** (`dbExecCtx`, state.go): `cs.activeTx`
ist ein einziges ChainState-weites Feld, das echte Nebenläufigkeit unmöglich
macht. Die Migration auf per-Operation-`ctx` läuft seit Juli, **48 Aufrufstellen
sind noch offen** (state.go 22, evm_storage.go 12, block.go 5, guardian.go 4,
snapshot.go 2, slashing.go 2, register.go 1). Der Transferpfad selbst ist
bereits migriert.

Danach — und erst danach — lässt sich die Sperre auf die bereits vorhandenen
Konten-Shards verengen (`shardedAccounts.LockAddrs`, die der WAL-Schreiber
schon benutzt).

Der Code warnt ausdrücklich davor, das in einem Zug zu tun: es quert Transfer,
Swap, Liquidität, Registrierung, Verteilung, Guardian, Slashing, Snapshot-Import
**und Block-Replay**. „one call chain at a time, EACH one verified by the full
test suite."

---

## activeTx-Migration abgeschlossen und gemessen, 29.08.2026

Der Weg zu 10k führt über `cs.activeTx` — ein einziges ChainState-weites Feld,
das echte Nebenläufigkeit unmöglich macht und deshalb `runAtomicWithOutbox`
zwingt, die globale Sperre über die ganze DB-Transaktion zu halten (74,7 % der
gemessenen Wartezeit).

**Fünf Ketten, jede einzeln mit voller Testreihe**, wie der Code es vorschreibt:

| Kette | Was |
|---|---|
| 1 | Konfigurations-Blätter (`getConfigValue`, `…Exists`, `deleteConfigValue`) |
| 2 | Rücknahmepfad (`restoreFromRollbackLocked`) |
| 3 | EVM-Spiegelwarteschlange (4 Stellen) |
| 4 | Blockkopf löschen, GHOSTDAG-Zustand |
| 5 | Straf-Zwischenspeicher (`loadPenaltyCacheLocked`) |

Danach: **null `cs.dbExec()`-Aufrufstellen** im ganzen Repo, ein Wächtertest
verhindert die Rückkehr.

### Das reichte nicht — und das war messbar

`dbExecCtx` fiel weiter auf `cs.activeTx` zurück, wenn der ctx keine
Transaktion trug. Ein Zähler zeigte live **466 Rückfälle im Leerlauf**: es gab
Pfade, die den ctx nicht durchreichten. Hätte man das Feld an dieser Stelle
entfernt, wären diese Schreibvorgänge still außerhalb ihrer Transaktion
gelandet.

Die Herkunftsliste nannte **genau zwei Zeilen**, beide im Block-Replay, beide
gleich oft — einmal je Block. Beide *wollten* in der Transaktion sein; ihre
eigenen Kommentare sagten es. Sie kamen nur über den stillen Rückfall dorthin.

**Jetzt auf beiden Boxen: `rueckfaelle: 0`, `activetx_entfernbar: true`.**

### Unter Last gemessen — und die Pfade nachgezogen

Drei Lastläufe gegen C1 (Generator auf C2), der Zähler als Messgröße:

| Lauf | C1 (nimmt an) | C2 (spielt nach) |
|---|---|---|
| 1 | 0 | 3.295 aus 2 Stellen |
| 2 | 0 | 2.745 aus 4 Pfaden |
| 3 · nach dem Fix | **0** | **0** |

**Der Transferpfad war von Anfang an vollständig** — C1 hatte unter 64.100,
69.100 und 91.600 Überweisungen keinen einzigen Rückfall. Alles Fehlende lag im
**Nachspielen**, und die dreistufige Herkunftsliste hat es beim Namen genannt:

- `applyTransferDeltaLocked` ← `block.go`, zweimal (1.980)
- `applyTransferBatchParallel` ← `block.go` (762)
- `ResetFinalizedCheckpoint` ← `snapshot.go`, Resync (3)

Alle vier gaben ausdrücklich `context.Background()`, während im selben
Sichtbereich eine offene Transaktion lag — und landeten trotzdem darin, über
den stillen Rückfall. Beim Resync stand die Anforderung sogar im Kommentar
daneben: *„resets both atomically"*. Atomar war es nur durch das gemeinsame
Feld.

**Ergebnis: beide Zähler bei 0, `activetx_entfernbar: true` auf beiden Boxen.**

### Was noch fehlt, bevor `activeTx` wirklich weg kann

Die Null gilt für **Überweisungen**. Nicht durchlaufen sind: die tägliche
Verteilung (läuft um 20:00 Berlin), eine Registrierung (braucht ein Gesicht),
ein Swap, der Guardian-Pfad.

Der Zähler läuft dauerhaft mit und kostet nichts. **Bleibt er über eine
Verteilungsrunde und eine Registrierung hinweg bei 0, kann `cs.activeTx`
entfernt werden** — und erst dann lässt sich die globale Sperre auf die
vorhandenen Konten-Shards verengen (`shardedAccounts.LockAddrs`). Das ist der
Schritt, der die gemessenen 74,7 % Wartezeit auflöst.

Springt er an, nennt die Herkunftsliste den Pfad, wie sie es dreimal getan hat.

---

## Wofür die globale Sperre gehalten wird — gemessen 29.08.2026

Das Mutex-Profil sagte „74,7 % der Wartezeit in `runAtomicWithOutbox`". Es
sagte nicht, wie sich das aufteilt. Jetzt schon, unter Last (50.000
Überweisungen, 83 s):

| | |
|---|---|
| Läufe von `runAtomicWithOutbox` | **44** |
| **Warten auf die Sperre** | **468,8 ms** |
| Halten der Sperre | 50,3 ms |
| — davon Arbeit (`fn`) | 28,3 ms (56 %) |
| — davon Commit | 15,6 ms (31 %) |
| — davon Outbox | 6,2 ms (12 %) |
| — davon Snapshot | 0,15 ms |

### Was das heißt

**Nur 44 Läufe für 50.000 Überweisungen** — der Transferpfad bündelt, rund
1.100 Stück je Vorgang. Pro Überweisung sind das ~45 µs Arbeit unter der
Sperre. Das ist nicht das Problem.

**Das Problem ist das Warten: 469 ms gegen 50 ms Halten — Faktor 9.** Wenn ein
Transferbündel die Sperre will, wartet es eine halbe Sekunde. Nicht auf ein
anderes Bündel (davon gibt es nur 44), sondern auf **alles andere, was `cs.mu`
nimmt**: Blockproduktion und Nachspielen.

Das erklärt die alte Widersprüchlichkeit — Durchsatz gedeckelt, CPU frei,
Replay parallel. Die Überweisungen warten nicht aufeinander. Sie warten auf den
Konsens.

### Was daraus für 10k folgt

Nicht „schneller machen", sondern **entkoppeln**. Ein Transferbündel braucht
die Konten, die es anfasst — nicht die globale Sperre. `runAtomicWithOutbox`
bekommt `touchedAddrs` bereits als Parameter, und `shardedAccounts.LockAddrs`
existiert und wird vom WAL-Schreiber schon benutzt.

Voraussetzung bleibt `cs.activeTx`: zwei gleichzeitige Vorgänge würden sich das
Feld überschreiben. Der strikte Modus läuft dafür seit heute auf beiden Boxen
und hat drei Lastläufe mit **0 Rückfällen** überstanden.

---

## Der Engpass für 10k ist gefunden — 29.08.2026 abends

Vier Messungen, zwei davon negativ, und am Ende eine präzise Stelle.

### Was **nicht** hilft (gemessen, nicht vermutet)

| Hebel | Ergebnis |
|---|---|
| WAL-Adressdeckel (`MAX_ADDRS` 64/128) | **868 → 299/320 TPS** — deutlich schlechter |
| Größere Bündel (4000/8000 statt 1000) | 860 → 703/807 — kein Gewinn, in der Streuung |

Beide Richtungen — kleiner *und* größer — bringen nichts. Die Bündelgröße ist
nicht der Hebel.

### Wo die Zeit wirklich hingeht

Der shard-gesperrte **Schnellpfad macht 92,6 %** aller Überweisungen. Seine
Phasen, je Überweisung:

```
pre_rlock   156,3 ms   ← 68 %: Warten auf cs.mu.RLock()
wal_append   68,7 ms   ← 30 %
cap           0,01 ms  ← wofür die Sperre gehalten wird
lock          0,02 ms  ← TryLockAddrs, unbestritten
apply         0,06 ms  ← die eigentliche Änderung
```

**156 ms Warten für eine Lesung von 0,01 ms.** Go's `RWMutex` sperrt ankommende
Leser aus, sobald ein Schreiber *ansteht*. Die Schreiber sind die **7,4 %**, die
über `runAtomicWithOutbox` laufen — 52 Vorgänge, je 64 ms Halt.

**Sieben Prozent des Verkehrs halten dreiundneunzig auf.**

### Warum die Sperre trotzdem nötig ist

Der langsame Pfad ändert Konten unter `cs.mu` **ohne** Shard-Sperren:
`applyTransferDeltaLocked` ruft `cs.accounts.Get()`, nicht `GetLocked()`. Die
Lesesperre des Schnellpfads ist das Einzige, was ihn davor schützt. Sie zu
entfernen, ohne den langsamen Pfad vorher umzustellen, wäre ein Datenrennen auf
dem Geldpfad.

### Der Schritt, der 10k freimacht

Den langsamen Pfad auf Shard-Sperren umstellen (`GetLocked` statt `Get`, plus
`LockAddrs` um die Änderung) — dann kann der Schnellpfad seine Lesesperre
verlieren und 93 % des Verkehrs warten gar nicht mehr.

**Mit einer Warnung:** derselbe Abend hat gezeigt, dass mehr und kleinere
Sperren schaden können. Hier ist die Lage anders — es geht nicht darum, einen
Halt zu zerlegen, sondern darum, den Großteil des Verkehrs gar nicht warten zu
lassen. Aber das ist eine Vermutung, und Vermutungen über diesen Engpass waren
schon dreimal falsch. Wer es angeht, misst es an einem Zweig.

Das Werkzeug dafür steht: Phasenmessung, Herkunftszähler, reparierter
Lastgenerator, und zwei Stellschrauben, die sich ohne Deploy verändern lassen.

---

## Warum 10k heute nicht erreichbar ist — die geschlossene Rechnung

Nach acht Messungen steht die Arithmetik. Sie ist keine Einschätzung.

### Die Grundgleichung

**Durchsatz = gleichzeitige Sender ÷ Latenz je Überweisung.**

Der Generator bündelt 100 Überweisungen **desselben Absenders** pro Anfrage,
und der Knoten arbeitet ein Bündel seriell ab (`for i, raw := range batch`).
Beides ist richtig so: gleicher Absender heißt fortlaufende Nonces, die
*müssen* in Reihenfolge laufen. Also zählt nur, wie viele **verschiedene**
Absender gleichzeitig senden.

Nachgerechnet: 150 Sender ÷ 0,23 s = 652. Gemessen: 650.

### Was daraus folgt

Für 10.000 TPS bei 230 ms Latenz bräuchte es **2.300 gleichzeitige Sender**.

| Sender | Ergebnis |
|---|---|
| 150 | 650 TPS |
| 288 (alle Paare) | ~1.250 TPS |
| **576 (ring)** | **0 TPS** — jede Anfrage läuft in die 30-s-Grenze |

Der Knoten bricht weit vor 2.300 zusammen. **Mehr Last hilft nicht, sie kippt.**

### Also muss die Latenz fallen

Von 230 ms auf ~29 ms — Faktor 8. Die Aufteilung sagt, wo sie liegt:

```
pre_rlock   156,3 ms   ← Warten auf cs.mu.RLock()
wal_append   68,7 ms   ← langsam, weil zu wenige gleichzeitig anhängen
apply         0,06 ms  ← die Arbeit
```

Der langsame Anhang ist die **Folge** der Serialisierung: die Überweisungen
erreichen den Gruppen-Commit einzeln und finden niemanden zum Bündeln. Die
Platte selbst schafft 46.342 Anhänge/s.

**Beide Posten verschwinden, wenn die Lesesperre fällt. Sonst keiner.**

### Was ausgeschlossen ist — gemessen, nicht vermutet

| Hebel | Ergebnis |
|---|---|
| WAL-Adressdeckel | 868 → 299 TPS, deutlich schlechter |
| Bündelgröße 4000/8000 | kein Effekt |
| WAL-Gruppencommit-Fenster | Streuung Faktor 3,3 — nicht entscheidbar |
| Mehr Sender | Zusammenbruch bei 576 |
| Ring-Topologie | 0 Erfolge |

### Was bleibt

Die Lesesperre zu entfernen verlangt, dass **jeder** Änderer Shard-Sperren
nimmt. Der Versuch scheiterte an einer Zahl: **247 Funktionen** lösen transitiv
Shard-Zugriffe aus, und die erste umgestellte verklemmte sich zwei Ebenen tief.

Der richtige Weg ist kein `LockAddrs` an 22 Stellen, sondern
`accounts.Update(addr, func(*AccountState))` — eine API, die keinen Zeiger
herausgibt, sodass der **Compiler** jede Stelle findet. Das ist ein Umbau der
Datenstruktur.

**Aufwand ehrlich geschätzt:** mehrere Tage, mit einem Lastgenerator, der erst
stabil sättigen muss. Nicht vor dem Beta-Start.

---

## Nachtrag 29.08.: zwei Befunde, die das Bild korrigieren

### 1. Der Zusammenbruch bei 576 Sendern war kein Kapazitätsende

Er ist Warteschlangentheorie. Der Knoten arbeitete durchgehend korrekt:

```
150 Sender x 100 je Bündel = 15.000 gleichzeitig -> 23 s je Bündel  (Client wartet 30 s: knapp)
576 Sender x 100 je Bündel = 57.600 gleichzeitig -> 88 s je Bündel  (Client wartet 30 s: nie)
```

Er nahm unbegrenzt an und lieferte nach Ablauf der Client-Frist aus. **Behoben**
(`inflight_grenze.go`): eine Obergrenze auf gleichzeitig angenommene Arbeit,
Vorgabe 8.000, Ablehnung mit dem wiederholbaren `-32005`. Nach Little's Gesetz
kostet das keinen Durchsatz — es beschränkt die Latenz statt sie explodieren zu
lassen.

### 2. Alle Rückfälle sind Shard-Kollisionen — zu einem Teil ein Prüfstands-Artefakt

Gemessen, nicht vermutet: `shard_belegt` = **134 von 134 = 100 %**. Nicht
Demurrage, nicht Rückstau, nicht fehlende Konten.

Nachgerechnet für die 300 heißesten Testkonten bei 16.384 Shards:

| Anteil | Ursache | im echten Betrieb? |
|---|---|---|
| ~4 % | 12 Konten teilen sich 6 Shards — **dauerhaft**, weil immer dieselben Konten | verschwindet |
| ~3,6 % | flüchtige Kollision bei 300 gesperrten Shards | **bleibt**, hängt an der Gleichzeitigkeit |

Zusammen ~7,6 % gegen 10,4 % gemessen — dieselbe Größenordnung.

**Folge für den geplanten Umbau:** die gemessenen 10,4 % Rückfall überzeichnen
den echten Betrieb um rund das Dreifache. Der Umbau der 247 Funktionen wird
dadurch nicht unnötig, aber sein erwarteter Gewinn ist kleiner als die
Rohmessung nahelegt. Wer ihn angeht, sollte **zuerst** mit einem großen,
wechselnden Kontenvorrat neu messen — sonst optimiert er ein Artefakt.

### Was unverändert bleibt

Die 10k sind mit 623 Konten nicht messbar. Bei niedriger Last braucht eine
Überweisung 11,3 ms (`pre_rlock` 1 ms, WAL 10,1 ms) — der Knoten ist also
schnell; die 230 ms entstehen erst unter Sättigung. Für eine ehrliche
10k-Messung braucht es einen deutlich größeren Kontenvorrat, und **den zu
befüllen ist eine Überweisung, die der Betreiber selbst auslöst.**

