# FromSync-Stichtag

Stand: 16.10.2026. Im Code ist der Stichtag **schlafend**
(`fromSyncStrengAbUnix = math.MaxInt64`, Rücknahme
`fromSyncStrengBisUnix = math.MaxInt64`,
`x/humanity/keeper/fromsync_stichtag.go`). Solange er schläft, verhält sich
jeder Knoten byte-gleich wie vorher (`TestFromSyncStichtag_SchlafendByteGleich`).

## Warum

Ein Block, den ein Knoten per HTTP-Sync von einem **eingetragenen Seed**
(`PRIMARY_NODE_URL`, `PRIMARY_NODE_URLS`, `PEER_NODES`) holt, trägt
`FromSync = true`. Für solche Blöcke fielen bisher Prüfungen weg. Ein Seed darf
`http://IP:8080` sein, also ohne TLS. Wer sich in diese Verbindung hängt,
liefert Blöcke, die als „vom Seed“ gelten.

## Was der Stichtag ändert (Blockzeit in `[Ab, Bis)`)

Maßgeblich ist `block.Timestamp`. Die Toleranzen von 120 s wirken zugunsten der
Prüfung: Ein Block mit Zeit ≥ `Ab − 120 s` und < `Bis + 120 s` wird streng
geprüft. Solange `Bis = MaxInt64`, gibt es kein Ende.

| Prüfung | Stelle | vor `Ab` | ab `Ab` |
|---|---|---|---|
| Sperre nach Doppelsignatur | `block.go`, `IsValidatorSuspended` in `AddPeerBlock` (`sperrpruefungEntfaellt`) | FromSync und SelfFetched entbinden | gilt für jeden Block — **außer** eine Waise wartet auf genau diesen Block und er kam per FromSync/SelfFetched |
| Finalität | `finality.go`, `isFinalityViolation` (`fromSyncAusnahmeFinalitaet`) | FromSync entbindet | gilt auch für FromSync; zusätzlich ab **Empfangszeit** `Ab` für jeden FromSync-Block, gleich welche Blockzeit |
| Strafe bei Doppelsignatur | `block.go`, `checkAndIndexEquivocation` | FromSync-Block straft nicht | straft auch beim Sync |
| Spät eingehängte Bindung/Strafe | `spaet_eingehaengt.go` | FromSync entbindet | gilt auch für FromSync |

### Sperre: warum die Waisen-Regel (Befund F1)

`lastOffenseAt` ist die Blockzeit der Doppelsignatur. Die Folgeblöcke von V
zwischen Tat und Erkennung stehen in der kanonischen Geschichte; ehrliche
Nachfahren bauen auf ihnen auf. Würde ein nachholender Knoten sie abweisen,
blieben die Nachfahren Waisen, wegen `suspendedUntil > now` bis zu 14 Tage.

Gewählt ist die Waisen-Regel, nicht die Alternative „Grenze = Zeitpunkt, an dem
die Strafe Konsensstand wurde“:

- Eine Waise entsteht erst hinter Signatur-, Erzeuger-, Finalitäts- und
  Sperrprüfung. Sie belegt also, dass ein **anderer, zugelassener und
  ungesperrter** Erzeuger auf dem Block aufgebaut hat. V selbst kann sich die
  Ausnahme nicht verschaffen: sein eigenes Kind fällt schon an der Sperre und
  wird keine Waise (`TestFromSyncStichtag_NachholenderBleibtNichtStehen_RealDB`).
- Sie braucht keinen neuen Zustand. Die Alternative bräuchte eine neue Spalte
  in `validator_penalties` (Blockzeit des Blocks mit `slash_equivocation`), die
  in die StateRoot und in Snapshot/Resync müsste, und sie hätte eine eigene
  Reihenfolge-Lücke: Der Nachholende erkennt die Doppelsignatur selbst, bevor
  er den Block mit der Strafe nachspielt.
- Ein Block von außen (Push, weder FromSync noch SelfFetched) bleibt immer
  gesperrt, auch mit wartender Waise. Am Live-Pfad ändert sich nichts.
- Die Prüfung ist eine Map-Abfrage unter `orphansMu` (begrenzt wie der
  Waisenpuffer selbst).

### Finalität: warum die Empfangszeit (Befund F2)

Ein Ast unterhalb der Finalität hat zwangsläufig alte Eltern und damit eine alte
Blockzeit; nach der Blockzeit allein bliebe er ausgenommen. Die Finalität ist
knotenlokal: eigener Checkpoint, eigene Entscheidung, ob ein Block in den
eigenen DAG kommt. Sie ist keine Regel des Nachspielens, die jeder Knoten gleich
entscheiden müsste (für Nicht-Seed-Blöcke gilt sie schon heute so). Die
Empfangszeit kann dort also keine Divergenz im Konsens erzeugen. Der legitime
Fall „ein Kind wartet auf genau diesen alten Block“ bleibt über die enge
SelfFetched-Regel offen.

Unverändert:

- **Erzeugerprüfung:** hat schon einen eigenen, abgelaufenen Stichtag
  (`syncGeschichte`, 30.09.2026).
- **Proposer-Sperre (Circuit Breaker):** bleibt für FromSync und SelfFetched
  ausgenommen. Sie ist eine Lastbremse vor allen Prüfungen, keine
  Gültigkeitsprüfung. Ein Block, der an ihr vorbeikommt, durchläuft danach
  jede Prüfung. Würde sie auch für Seeds greifen, käme die Verklemmung zwischen
  Seed und nachholendem Knoten zurück (Kommentar in `AddPeerBlock`).
- **Enge SelfFetched-Ausnahme der Finalität:** Sie greift nur, solange eine
  Waise auf genau diesen Block wartet. Sie gilt für jeden Peer, nicht nur für
  Seeds.
- Signaturen von Block und Überweisungen, Noncen, Deckung, Hash und Höhe hat
  FromSync nie entbunden.

Ältere Blöcke spielen unverändert nach; einzige Ausnahme ist die knotenlokale
Finalität, die ab Empfangszeit `Ab` auch alte FromSync-Blöcke unter dem
Checkpoint abweist. Für die Sperre gilt das ohnehin:
`IsValidatorSuspended` lässt Blöcke vor dem Vergehen durch
(`blockTimestamp < lastOffenseAt`).

## Was danach anders ist

- Ein Seed, der nur Blöcke **unter der eigenen Finalität** liefert, gilt nicht
  mehr als „sauberer Sync-Zyklus“ (`sync_blocks.go`, `sawUnmergedBlocks`).
  Das gilt ab Empfangszeit `Ab` für jeden solchen Block, gleich welche
  Blockzeit. Das ist die Signatur einer Gabel. Ein Knoten mit diesem Seed hält
  das Sync-Tor (`hasCaughtUpWithAllPeers`) geschlossen und produziert dann
  nicht (siehe „Vorbedingungen“, F4). Abhilfe ist ein Resync vom Seed.
- Ein Ast mit gefälschtem Erzeuger und Blockzeit vor dem 30.09. kommt unter der
  Finalität nicht mehr über den Sync herein (`syncGeschichte`, F2).
- Ein Validator, der gesperrt ist, kann seine Blöcke ab `Ab` auch nicht mehr
  über den Sync eines Seeds einschleusen.

## Ablauf für den Betreiber

1. **Voraussetzung:** Alle Validatoren laufen mit diesem Code. Prüfen:
   `/api/status` auf jedem Knoten, gleiche Version.
2. **Vorher prüfen:** Auf jedem Knoten in `/api/health/combined` die Werte
   `sync_diagnostics` und `finalized_height` ansehen. Hängt ein Knoten an der
   Finalitätswand eines Seeds fest, zuerst per Resync heilen.
3. **Stichtag setzen:** In einem eigenen PR `fromSyncStrengAbUnix` auf eine
   Unix-Zeit mit **mindestens 48 h Vorlauf** setzen, auf eine volle Stunde UTC.
   Der PR bekommt eine eigene Sicherheitsprüfung (Konsens).
   `TestFromSyncStichtag_SchlafendByteGleich` läuft weiter (er nutzt den
   Override `math.MaxInt64`).
4. **Ausrollen:** Auf allen Validatoren vor `Ab` deployen. Ein Knoten mit dem
   alten Code nimmt nach `Ab` Seed-Blöcke an, die die anderen abweisen. Dann
   laufen die Knoten auseinander.
5. **Nach `Ab` beobachten (erste Stunde):**
   - `[FINALITY] ✗ Rejected block` und `[SLASHING] ✗ Rejected block` im Log
     der nachholenden Knoten. Einzelne Meldungen sind erwartet. Eine Flut von
     einem Seed heißt: Er liefert eine Gabel.
   - Höhe und StateRoot aller Knoten auf `/api/status` vergleichen.
6. **Zurücknehmen:** Nie `fromSyncStrengAbUnix` zurück auf `math.MaxInt64`
   setzen. Das änderte die Regel für schon geschriebene Blöcke rückwirkend:
   Ein frisch synchronisierender Knoten nähme dann an, was die laufenden Knoten
   abgewiesen haben, und die Knoten liefen auseinander. Stattdessen setzt ein
   PR `fromSyncStrengBisUnix` auf eine Zeit nach `Ab`.
   - Die Rücknahme ist **selbst ein Konsenswechsel**: wie Schritt 3 bis 5, mit
     mindestens 48 h Vorlauf, eigener Sicherheitsprüfung und Ausrollen auf
     allen Validatoren vor `Bis`.
   - Blöcke mit Blockzeit in `[Ab, Bis)` bleiben für immer streng geprüft,
     auch für einen Knoten, der sie erst nach `Bis` nachholt. Erst ab `Bis`
     gilt wieder die alte Ausnahme.
   - **Strafen aus der Zwischenzeit bleiben bestehen.** `slash_equivocation`
     aus `[Ab, Bis)` steht in der Kette; Sperre, Zähler und Geldstrafe werden
     nicht zurückgedreht.
   - Ein `Bis ≤ Ab` wird nicht beachtet (fail-closed), der Stichtag bliebe
     scharf. `TestFromSyncStichtag_KonstantenGeordnet` fängt das ab.
   - Ein Knoten, der in der Zwischenzeit an einem Seed-Block hängen blieb,
     heilt per Resync vom Seed.

## Vorbedingungen für das Einschalten

- **F4: Minderheitsgabel und Sync-Tor.** Ein Knoten, der auf einer
  Minderheitsgabel finalisiert hat, heilt ab `Ab` nicht mehr über den Sync. Der
  Seed liefert ihm nur Blöcke unter seiner Finalität, und die zählen nicht mehr
  als sauberer Zyklus (`sync_blocks.go`, `sawUnmergedBlocks`). Geklärt im Code
  (`ProduceBlock`, `block.go`): Ein geschlossenes Sync-Tor
  (`hasCaughtUpWithAllPeers`) **hält die Produktion an**:
  - Ein Prozess mit `bootHeight == 0`, der seit seinem Start noch nie mit allen
    Seeds gleichauf war (`jemalsAufgeholt` liegt nur im Speicher, also nach
    **jedem Neustart**): ohne Notausstieg, die Produktion steht, bis das Tor
    aufgeht.
  - Ein Knoten aus dem Snapshot (`bootHeight > 0`): Notausstieg erst nach
    `syncStallTimeout` (90 s) ohne jeden Peer-Kontakt. Den gibt es nicht,
    solange der Seed Blöcke liefert, auch abgewiesene, weil
    `lastPeerContactAt` vor jeder Prüfung gesetzt wird. Die Produktion steht
    also ebenfalls.
  - Ein Knoten, der in diesem Prozess schon einmal aufgeholt hat
    (`bootHeight == 0`, `jemalsAufgeholt`): Das Tor prüft ihn nicht mehr, er
    produziert weiter.

  Vor dem Einschalten muss deshalb auf jedem Validator geprüft sein, dass er
  nicht auf einer Minderheitsgabel finalisiert hat (Schritt 2: gleicher
  `finalized_height` und gleiche StateRoot wie der Seed). Ein solcher Knoten
  wird vorher per Resync geheilt; sonst steht er nach dem nächsten Neustart.
- Die Befunde F1, F2, F3 und F5 der Sicherheitsprüfung von #373 sind behoben
  (siehe oben und „Restrisiken“).

## Restrisiken

- **`syncGeschichte` an der Sync-Front (F2, nicht im Code lösbar).** Ein
  FromSync-Block mit Blockzeit vor dem 30.09.2026 überspringt weiter die
  Erzeugerprüfung (`erzeugerErlaubt`), damit alte Blöcke früherer Validatoren
  nachspielen. Ab `Ab` (Empfangszeit) fällt er an der Finalität, sobald er
  unter ihr liegt. Ein Knoten, der die Geschichte vor dem 30.09. **gerade
  nachspielt**, hat seine Finalität aber noch nicht über diese Höhen gehoben.
  Dort kann ein Block mit gefälschtem Erzeuger, eingeschleust in eine
  Seed-Verbindung ohne TLS, weiter durchkommen. Eine Bindung an einen
  signierten Checkpoint geht nicht: Das Zertifikat in der Kette schläft
  (`checkpointTxAb`), und ein vorwärts nachspielender Knoten bekommt den
  Checkpoint-Block erst **nach** den alten Blöcken. Ob ein alter Block dessen
  Vorfahr ist, lässt sich beim Empfang also nicht prüfen. Was bleibt:
  Signaturen der Überweisungen, Noncen, Deckung und die Regeln zur Blockzeit
  gelten weiter, die Gefahr ist eine abweichende StateRoot nur auf diesem
  Knoten (sichtbar in `/api/status`, Heilung per Resync). Abhilfe im Betrieb:
  Seeds nur über `https://` eintragen, oder einen neuen Knoten aus einem
  signierten Snapshot starten (`bootHeight` über dem 30.09.).
- **Doppelte Beweis-Transaktion (F5).** Erkennt ein nachholender Knoten eine
  Doppelsignatur, deren Strafe schon in der Kette steht (innerhalb der
  Beweisfrist `strafBeweisFrisch`), legt er eine zweite `slash_equivocation`
  für dasselbe Paar in den Ausgang. Sie ist harmlos:
  `UNIQUE (block_a_hash, block_b_hash)` mit `ON CONFLICT DO NOTHING` zählt
  nichts doppelt, bucht nichts doppelt und verlängert keine Sperre, weder beim
  Erkennen noch beim Nachspielen
  (`TestFromSyncStichtag_StrafeSchonInDerKette_RealDB`). Ältere Beweise
  vermerkt `DoppelsignaturErkannt` gar nicht.

## Tests

- `TestFromSyncStichtag_SchlafendByteGleich`: schlafend gleiche Entscheidung
  wie vorher, für jede Blockzeit; Finalität und Spät-Prüfung mit FromSync wie
  bisher.
- `TestFromSyncStichtag_Grenze`: Grenze an der Blockzeit, mit Rück-Toleranz,
  ohne Überlauf.
- `TestFromSyncStichtag_FinalitaetGiltAuchBeimSync`: Ein FromSync-Block unter
  dem Checkpoint wird ab `Ab` abgewiesen, auch mit alter Blockzeit (F2).
  Gegenprobe: vor `Ab` (Empfangszeit) wie bisher angenommen.
- `TestFromSyncStichtag_SyncGeschichteGefaelschterErzeuger` (F2): gefälschter
  Erzeuger, Blockzeit vor dem 30.09., unter der Finalität: ab `Ab` abgewiesen,
  schlafend angenommen.
- `TestFromSyncStichtag_SperreNurOhneWartendeWaise`,
  `TestFromSyncStichtag_NachholenderBleibtNichtStehen_RealDB` (F1): Der
  Nachholende hängt V's Folgeblock und den ehrlichen Nachfahren ein. V's eigenes
  Kind verschafft keine Ausnahme, ein Push bleibt gesperrt.
- `TestFromSyncStichtag_Ruecknahme`, `TestFromSyncStichtag_KonstantenGeordnet`
  (F3): Fenster `[Ab, Bis)` mit Toleranzen, Blöcke aus dem Fenster bleiben nach
  `Bis` streng, `Bis ≤ Ab` wird nicht beachtet.
- `TestFromSyncStichtag_StrafeSchonInDerKette_RealDB` (F5): kein zweites
  Vergehen, kein zweiter Abzug, keine längere Sperre; Gegenprobe: ein neues
  Paar zählt.
- `TestFromSyncStichtag_SperreGiltAuchBeimSync_RealDB`: Ein gesperrter
  Validator kommt ab `Ab` weder über FromSync noch über SelfFetched durch. Die
  Gegenproben: ungesperrt angenommen, schlafend angenommen.
- `TestFromSyncStichtag_SpaetEingehaengtGiltAuchBeimSync`,
  `TestFromSyncStichtag_DoppelsignaturStraftAuchBeimSync`.
