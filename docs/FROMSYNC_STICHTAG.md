# FromSync-Stichtag

Stand: 16.10.2026. Im Code ist der Stichtag **schlafend**
(`fromSyncStrengAbUnix = math.MaxInt64`,
`x/humanity/keeper/fromsync_stichtag.go`). Solange er schläft, verhält sich
jeder Knoten byte-gleich wie vorher (`TestFromSyncStichtag_SchlafendByteGleich`).

## Warum

Ein Block, den ein Knoten per HTTP-Sync von einem **eingetragenen Seed**
(`PRIMARY_NODE_URL`, `PRIMARY_NODE_URLS`, `PEER_NODES`) holt, trägt
`FromSync = true`. Für solche Blöcke fielen bisher Prüfungen weg. Ein Seed darf
`http://IP:8080` sein, also ohne TLS. Wer sich in diese Verbindung hängt,
liefert Blöcke, die als „vom Seed“ gelten.

## Was der Stichtag ändert (ab Blockzeit `Ab`)

Maßgeblich ist `block.Timestamp`. Die Rück-Toleranz von 120 s ist
eingerechnet: Ein Block mit Zeit ≥ `Ab − 120 s` wird streng geprüft.

| Prüfung | Stelle | vor `Ab` | ab `Ab` |
|---|---|---|---|
| Sperre nach Doppelsignatur | `block.go`, `IsValidatorSuspended` in `AddPeerBlock` | FromSync und SelfFetched entbinden | gilt für jeden Block |
| Finalität | `finality.go`, `isFinalityViolation` | FromSync entbindet | gilt auch für FromSync |
| Strafe bei Doppelsignatur | `block.go`, `checkAndIndexEquivocation` | FromSync-Block straft nicht | straft auch beim Sync |
| Spät eingehängte Bindung/Strafe | `spaet_eingehaengt.go` | FromSync entbindet | gilt auch für FromSync |

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

Ältere Blöcke spielen unverändert nach. Für die Sperre gilt das ohnehin:
`IsValidatorSuspended` lässt Blöcke vor dem Vergehen durch
(`blockTimestamp < lastOffenseAt`).

## Was danach anders ist

- Ein Seed, der nur Blöcke **unter der eigenen Finalität** liefert, gilt nicht
  mehr als „sauberer Sync-Zyklus“ (`sync_blocks.go`, `sawUnmergedBlocks`).
  Das ist die Signatur einer Gabel. Ein Knoten mit diesem Seed kann am Sync-Tor
  (`hasCaughtUpWithAllPeers`) länger warten. Abhilfe ist ein Resync vom Seed.
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
6. **Zurücknehmen:** Ein PR setzt `fromSyncStrengAbUnix` wieder auf
   `math.MaxInt64` und wird auf allen Validatoren ausgerollt. Ein Knoten, der
   in der Zwischenzeit an einem Seed-Block hängen blieb, heilt per Resync vom
   Seed.

## Tests

- `TestFromSyncStichtag_SchlafendByteGleich`: schlafend gleiche Entscheidung
  wie vorher, für jede Blockzeit; Finalität und Spät-Prüfung mit FromSync wie
  bisher.
- `TestFromSyncStichtag_Grenze`: Grenze an der Blockzeit, mit Rück-Toleranz,
  ohne Überlauf.
- `TestFromSyncStichtag_FinalitaetGiltAuchBeimSync`: Ein FromSync-Block unter
  dem Checkpoint wird ab `Ab` abgewiesen, ein alter weiter angenommen.
- `TestFromSyncStichtag_SperreGiltAuchBeimSync_RealDB`: Ein gesperrter
  Validator kommt ab `Ab` weder über FromSync noch über SelfFetched durch. Die
  Gegenproben: ungesperrt angenommen, schlafend angenommen.
- `TestFromSyncStichtag_SpaetEingehaengtGiltAuchBeimSync`,
  `TestFromSyncStichtag_DoppelsignaturStraftAuchBeimSync`.
