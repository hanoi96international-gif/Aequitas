# AequitasV8 – Entwurf

Stand 29.09.2026. Löst V7 beim Neustart der Kette bei null ab (Launch-Checkliste, Neustart-Paket).

## Warum V7 abgelöst wird

Die Kette führt die Buchhaltung **nativ in Go**: Kontostände, Grundeinkommen, Demurrage,
Vermögensgrenze, Guardians und Escrow. V7 ist daneben eine **zweite Buchhaltung**, die die
Kette nur nachschreibt. Erreichbar ist von V7 praktisch nur dreierlei:

| V7-Funktion | Was tatsächlich passiert |
|---|---|
| `balanceOf`, `totalSupply`, `name`, `symbol`, `decimals` | Werte aus dem Spiegel; `balanceOf` per RPC direkt aus Go (`evm_rpc.go`) |
| `transfer` | Die Kette fängt den Aufruf ab und bucht ihn in Go (`TransferWithV7FeeAtomic`) |
| `registerWithSig` | Register der Menschen; nur mit dem Betreiberschlüssel (`checkPersistedCallAllowed`) |

Alles andere, also `claimUBI`, `accumulateUBI`, `confirmAlive`, Guardian, Escrow, Demurrage
und Vermögensgrenze, blockiert eine Positivliste (`knownV7PublicPersistSelectors`). Damit
sind **rund 700 der 807 Zeilen toter Code**, der von der Kette abweicht. Das Audit vom
16.08. fand dort eigene Fehler und eine andere Gebührentabelle als in Go (Befund F2).
Toter, abweichender Code ist Angriffsfläche und führt jeden Prüfer in die Irre.

Weitere Schwächen:

- **Der Verifier ist `immutable`.** Ein neuer Schlüssel aus der Trusted-Setup-Zeremonie
  verlangt ohnehin einen neuen Contract.
- **Die Speicherbelegung ist eine ungeschriebene Annahme.** Go schreibt an 14 fest
  verdrahtete Plätze in V7 (`evm_storage.go`, `evm_engine.go`, `guardian.go`):

| Slot | V7-Variable | geschrieben/gelesen von Go |
|---|---|---|
| 0 | `totalSupply` | Spiegel |
| 4 | `balanceOf` | `SyncBalancesToEVM` |
| 5 | `escrowOf` | Guardian/Escrow-Spiegel |
| 6 | `isHuman` | Spiegel, Registrierungs-Diagnose |
| 7 | `usedCommitments` | Registrierung |
| 8 | `usedNullifiers` | Registrierung |
| 9 | `commitmentOf` | Registrierung |
| 10, 11 | `lastActivity`, `lastDemurrage` | Spiegel |
| 12 | `ubiClaimed` | Spiegel |
| 13 | `guardianOf` | Guardian-Spiegel |
| 28 | `nullifierOf` | Registrierung |
| 29 | `grantIssuedTo` | Registrierung |

  Eine neue Variable an der falschen Stelle in V7 hätte die Kette still gegen falsche
  Plätze schreiben lassen. Kein Test prüft das heute.

## Was V8 ist

**Genau zwei Aufgaben, eine Wahrheit.**

1. **Register der Menschen**
   - `registerWithSig(pA, pB, pC, pubSignals, claimedHuman, signature)`: nur gültig mit
     Signatur des Betreibers über (Contract, Chain-ID, Beweis, Empfänger); schützt gegen
     Front-Running und Wiederholung.
   - ZK-Beweis über den Verifier aus der Zeremonie.
   - Jeder Nullifier und jedes Commitment genau einmal; `isHuman`, `nullifierOf`,
     `commitmentOf`; Ereignis `Registered`.
   - **Kein Startguthaben im Contract.** Das Startguthaben bucht Go, wie heute schon real.
2. **ERC-20-Fassade** für Wallets und Explorer: `name`, `symbol`, `decimals`, `totalSupply`,
   `balanceOf`, `transfer`, `Transfer`-Ereignis. Die Werte kommen aus der Kette; `transfer`
   bucht die Kette in Go wie heute.

**Nicht mehr im Contract:** Grundeinkommen, Demurrage, Vermögensgrenze, Escrow,
Guardians, Gebühren. Das gibt es nur noch in Go, einmal und getestet.

## Speicherbelegung – festgeschrieben

V8 legt die Plätze ausdrücklich fest und dokumentiert sie im Contract selbst. Ein Test
liest die Belegung aus dem Compiler (`storageLayout`) und vergleicht sie mit den
Konstanten, die Go benutzt. Eine Verschiebung macht den Build rot, statt die Kette still
falsch schreiben zu lassen. Die Go-Seite bekommt **eine** Datei mit allen V8-Plätzen, statt
über drei Dateien verteilte Zahlen.

## Was sich in der Kette ändert

- `V7_CONTRACT_ADDR` → Adresse von V8 (Genesis), alle 14 Slot-Zahlen aus einer Tabelle.
- Der Spiegel schreibt nur noch, was V8 hat: `balanceOf`, `totalSupply`, `isHuman`,
  Register. Die Spiegel von Grundeinkommen, Demurrage, Guardian und Escrow fallen weg.
- Die Positivliste erreichbarer Funktionen wird auf V8 zugeschnitten.
- Der Gegentest in Go: registrieren, überweisen, Grundeinkommen auszahlen, danach
  `balanceOf`/`isHuman`/`totalSupply` aus dem EVM gleich Go.

## Tests

- **Solidity:** Registrierung Gutfall; doppelter Nullifier; doppeltes Commitment; fremde
  oder fehlende Signatur; Signatur für anderen Empfänger (Front-Running); Wiederholung
  derselben Daten; ungültiger Beweis; Signatur mit hohem `s` (Formbarkeit); Nullwerte.
- **Speicherbelegung:** Compiler-Layout gleich Go-Tabelle.
- **Go:** Gegentest EVM ↔ Go, Positivliste.
- **Danach** eine getrennte Sicherheitsprüfung des gesamten Diffs (AGENTS.md).

## Ausrollen

Nur mit dem Neustart der Kette bei null. Die V7-Adresse ist im Knoten fest verdrahtet, und
ein Wechsel ist ein Konsens-Update beider Knoten. Auf der laufenden Kette müssten die
bestehenden Testmenschen aus V7 migriert werden, und der Neustart löscht sie ohnehin.
