# AequitasV8 – Entwurf und Umsetzung

Stand 30.09.2026. Löst V7 beim Neustart der Kette bei null ab (Launch-Checkliste, Neustart-Paket).
Entwurf vom 29.09.2026 (Branch `claude/v8`), umgesetzt auf `claude/v8-contract`:

| Datei | Inhalt |
|---|---|
| `contracts/AequitasV8.sol` | der Contract (Register + ERC-20-Fassade), Speicherbelegung im Kopf dokumentiert |
| `contracts/v8_slots.json` | **vollständige** maschinenlesbare Tabelle: alle Speicherplätze und alle Selektoren |
| `contracts/MockGroth16Verifier.sol` | Test-Verifier, der auch „nein“ sagen kann |
| `test/AequitasV8_register.ts` | Gutfall und Missbrauchstests (29 Tests) |
| `test/AequitasV8_storage_layout.ts` | Compiler-`storageLayout` und ABI gegen `v8_slots.json`, Slot-Rechnung von außen wie Go (5 Tests) |
| `hardhat.config.ts` | `outputSelection` um `storageLayout` ergänzt (ändert keinen Bytecode, V7-Bytecode-Sync-Test bleibt grün) |

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

Weitere Schwächen von V7:

- **Die Speicherbelegung ist eine ungeschriebene Annahme.** Go schreibt an fest verdrahtete
  Plätze in V7 (`evm_storage.go`, `evm_engine.go`, `guardian.go`, `register.go`):

| Slot | V7-Variable | geschrieben/gelesen von Go |
|---|---|---|
| 0 | `totalSupply` | Spiegel |
| 1 | `totalHumans` | Migration, Registrierungs-Fallback |
| 2, 3 | `ubiPool`, `ubiPerHumanAccumulated` | Migration, Redeploy |
| 4 | `balanceOf` | `SyncBalancesToEVM` |
| 5 | `escrowOf` | Guardian/Escrow-Spiegel |
| 6 | `isHuman` | Spiegel, Registrierungs-Diagnose |
| 7 | `usedCommitments` | Registrierung |
| 8 | `usedNullifiers` | Registrierung |
| 9 | `commitmentOf` | Registrierung |
| 10, 11 | `lastActivity`, `lastDemurrage` | Spiegel |
| 12 | `ubiClaimed` | Spiegel |
| 13, 14 | `guardianOf`, `pendingGuardian` | Guardian-Spiegel, Pre-Call-Lesen |
| 17–26 | `CAPS`, `THRESHOLDS` | Persistenz |
| 28 | `nullifierOf` | Registrierung, Sweep |
| 29 | `grantIssuedTo` | Registrierung |

- **Die Nutzersignatur ist `personal_sign` über `encodePacked`**, ohne Frist und ohne Nonce.

## Was V8 ist

**Genau zwei Aufgaben, eine Wahrheit. Kein Owner, keine Admin-Funktion, kein Upgrade.**

1. **Register der Menschen**
   `registerWithSig(pA, pB, pC, pubSignals, human, deadline, signature)`
   - Groth16-Beweis über den Verifier aus der Zeremonie (Interface wie heute,
     `verifyProof(uint[2],uint[2][2],uint[2],uint[2])`, Adresse `immutable` im Konstruktor).
   - EIP-712-Signatur **des Nutzers** (der Wallet, die Mensch wird), siehe unten.
   - Aufrufer muss ein **Registrar** aus der Genesis sein (Relayer der Validatoren).
   - Jeder Nullifier und jedes Commitment genau einmal; `isHuman`, `nullifierOf`,
     `commitmentOf`, `usedCommitments`, `usedNullifiers`, `nonces`, `totalHumans`.
   - Ereignis `Registered(address indexed human, uint256 commitment, bytes32 indexed nullifier)`
     – ohne Betrag.
   - **Kein Startguthaben im Contract.** Das Startguthaben bucht Go, wie heute schon real.
2. **ERC-20-Fassade** für Wallets und Explorer: `name`, `symbol`, `decimals` (Konstanten),
   `totalSupply`, `balanceOf` (Views über Speicherplätze, die Go spiegelt),
   `transfer` mit dem ERC-20-Selektor `0xa9059cbb` und das `Transfer`-Ereignis im ABI.
   Die Kette fängt `transfer` wie heute vor der EVM ab und bucht in Go. Erreicht der Aufruf
   doch einmal den Bytecode, **revertiert** er (`"V8: transfer is booked by the chain"`):
   fail-closed statt einer vorgetäuschten Buchung auf einem Ledger, das nicht das echte ist.
   Kein `approve`/`transferFrom`/`allowance`: das wäre eine zweite Wirtschaft neben Go, die
   niemand braucht (Go bucht nur `transfer`); ein Test stellt sicher, dass
   `registerWithSig` die einzige zustandsändernde Funktion ist.

**Nicht mehr im Contract:** Grundeinkommen, Demurrage, Vermögensgrenze, Escrow,
Guardians, Gebühren. Das gibt es nur noch in Go, einmal und getestet.

Revert-Gründe sind bewusst `Error(string)` mit Präfix `V8:` statt Custom Errors, damit
`decodeRevertReason` (`evm_engine.go:1101`) sie unverändert an `/api/register` durchreicht.

## Offene Punkte aus der Prüfung – gelöst

### (i) Wer signiert: der Nutzer, und zusätzlich muss der Registrar einreichen

**Signatur (EIP-712, Nutzer):**

```
Domain   EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)
         name = "Aequitas", version = "8", chainId = block.chainid (Kette: 1926),
         verifyingContract = address(this), salt = NETZ_SALT = keccak256(bytes(netz_kennung))
Struct   Register(address human,uint256 commitment,uint256 nullifier,uint256 nonce,uint256 deadline)
         commitment = pubSignals[0], nullifier = pubSignals[1], nonce = nonces[human]
Digest   keccak256(0x1901 ‖ DOMAIN_SEPARATOR ‖ hashStruct)
```

Das bindet Contract, Chain-ID, **Netz**, Beweis (über seine öffentlichen Signale Commitment und
Nullifier), Empfänger und Frist.

**Warum `salt` (Nachtrag 30.09.):** Beim Neustart bei null bleibt die Chain-ID 1926, und V8 kann
wieder an derselben Genesis-Adresse liegen. Ohne `salt` wäre eine Signatur der alten Kette
(Nonce 0, Frist noch nicht abgelaufen) auf der neuen gültig. `NETZ_SALT` ist
`keccak256(bytes(netz_kennung))` mit `netz_kennung = "aequitas-<chainId>-<genesis-unix>"`
(dieselbe Kennung, die `/api/status` liefert und an der die App den Neustart erkennt). Sie wird
im Konstruktor gesetzt (`immutable`, im Bytecode, überlebt den Code-Umzug an die Genesis-Adresse)
und ist nie null. `eip712Domain()` (EIP-5267) liefert die Domäne, damit App und Wallets genau sie
signieren; die App gleicht `salt` zusätzlich mit `keccak256(netz_kennung)` aus `/api/status` ab.
Test: „restart at zero: a signature from the previous network … is void“. Einen eigenen Hash über `pA/pB/pC` bindet die Signatur
nicht: der Beweis ist durch seine öffentlichen Signale vollständig bestimmt, was er
beweist; ein zweiter gültiger Beweis für dieselben Signale beweist dasselbe.
Signaturprüfung streng: 65 Byte, `v ∈ {27, 28}`, `s ≤ n/2` (EIP-2), Ergebnis ≠ 0.
`DOMAIN_SEPARATOR()` wird bei jedem Aufruf aus `address(this)` und `block.chainid` berechnet,
**nicht** im Konstruktor gecacht: `contract_deploy.go` legt den Runtime-Code nach dem
Deploy unter die Genesis-Adresse (`contract_deploy.go:331–380`) – ein gecachter Separator
würde die falsche Adresse enthalten. `registrationDigest(human, commitment, nullifier, deadline)`
liefert den zu signierenden Digest mit der aktuellen Nonce, zum Gegenprüfen für App und Go
(Test vergleicht mit einer unabhängigen Kodierung).

**Registrar (Betreiber) zusätzlich: ja.** Begründung:

1. Groth16-Proving-Keys sind öffentlich. Ein gültiger Beweis ist **kein** Nachweis eines
   Menschen: jeder kann einen erfundenen Biometrie-Hash beweisen. Ob der Beweis aus der
   biometrischen Prüfung `/api/prove` stammt, entscheidet nur der Knoten
   (`prove_provenance.go`, `register.go:437`). Ohne Registrar-Pflicht wäre der Contract für
   sich genommen eine offene Münzprägestelle für Sybils – nur die Go-Positivliste davor
   würde das verhindern. So prüft die EVM jedes Knotens es selbst (AGENTS.md Punkt 4).
2. Register und Go-Ledger bleiben auf einem Pfad (dieselbe Regel wie
   `checkPersistedCallAllowed`, jetzt zusätzlich im Contract).

Umsetzung: `mapping(address => bool) isRegistrar`, **einmalig im Konstruktor** aus einer
Genesis-Liste gesetzt (1…16 Adressen, keine Null, keine Duplikate), danach unveränderlich.
Eine Liste statt einer Adresse, weil jeder Validator seinen eigenen Relayer-Schlüssel hat
(`RELAYER_PRIVATE_KEY` je Knoten); geteilte Schlüssel wären schlechter. Ein Registrar kann
sich nicht selbst zum Menschen machen (wie der Guard in `register.go:358`).
Die Betreiber-Signatur wird **nicht** zusätzlich verlangt: `msg.sender` ist bereits durch die
Transaktionssignatur des Relayers belegt; eine zweite Betreiber-Signatur brächte nichts,
was `isRegistrar[msg.sender]` nicht schon prüft.

### (ii) Front-Running und Wiederholung

| Angriff | Abwehr | Test |
|---|---|---|
| Fremder reicht die Anfrage ein | nur Registrar (`V8: caller is not a registrar`) | ✓ |
| Anfrage auf andere Wallet umlenken | Signatur bindet `human` | ✓ |
| Commitment/Nullifier/Frist verändern | alles im Struct | ✓ |
| Wiederholung derselben Anfrage | `isHuman`, Nullifier, Commitment einmalig | ✓ |
| Wiederholung nach Freigabe (Go setzt Register zurück) | `nonces[human]` steigt bei jedem Erfolg | ✓ |
| andere Kette / anderer Contract / V7- oder V9-Domain | Domain mit Chain-ID, Adresse, Name, Version | ✓ |
| Signatur ewig gültig | `deadline ≥ now` **und** `deadline ≤ now + 1 Tag` (`MAX_SIGNATURE_LIFETIME`) | ✓ |
| formbare Signatur (hohes `s`), `v` 0/1, falsche Länge, Müll → `address(0)` | strenge Prüfung | ✓ |
| Feld-Aliasing (`x` und `x + r` sind dasselbe Feldelement) | `pubSignals < r` (BN254) | ✓ |
| Nullwerte | `commitment ≠ 0`, `nullifier ≠ 0`, `human ≠ 0` | ✓ |
| ungültiger Beweis | `verifyProof` (STATICCALL, vor jedem Schreiben) | ✓ |

### (iii) Speicherbelegung festgeschrieben, vollständige Tabelle

`contracts/v8_slots.json` (vollständig, Reihenfolge = Deklaration):

| Slot | Variable | Typ | Schlüssel für Go | geschrieben von |
|---|---|---|---|---|
| 0 | `totalSupply` | `uint256` | – | **nur Go** (Spiegel) |
| 1 | `totalHumans` | `uint256` | – | Contract (+ Go-Migration) |
| 2 | `balanceOf` | `mapping(address => uint256)` | `mappingSlot(addr, 2)` | **nur Go** (Spiegel) |
| 3 | `isHuman` | `mapping(address => bool)` | `mappingSlot(addr, 3)` | Contract (+ Go-Spiegel) |
| 4 | `usedCommitments` | `mapping(uint256 => bool)` | `mappingSlotBytes32(commitment, 4)` | Contract |
| 5 | `usedNullifiers` | `mapping(bytes32 => address)` | `mappingSlotBytes32(nullifier, 5)` | Contract |
| 6 | `commitmentOf` | `mapping(address => uint256)` | `mappingSlot(addr, 6)` | Contract |
| 7 | `nullifierOf` | `mapping(address => bytes32)` | `mappingSlot(addr, 7)` | Contract |
| 8 | `nonces` | `mapping(address => uint256)` | `mappingSlot(addr, 8)` | Contract |
| 9 | `isRegistrar` | `mapping(address => bool)` | `mappingSlot(addr, 9)` | nur Konstruktor – Go schreibt **nie** |

`verifier` ist `immutable` (im Bytecode), alle übrigen Werte sind `constant`; beide belegen
keinen Slot. Keine Packung (alle `offset = 0`).

`test/AequitasV8_storage_layout.ts` liest `storageLayout` aus dem Build-Info des aktuellen
Artefakts und verlangt **Gleichheit in beide Richtungen** (Label, Slot, Offset, Typ, Reihenfolge).
Dieselbe Datei enthält alle 24 Selektoren mit Klasse (`view`/`intercepted`/`registrar`), ebenfalls
gegen das ABI geprüft. Außerdem rechnet der Test die Plätze **von außen wie Go**
(`keccak256(pad32(key) ‖ pad32(slot))`): Go-Spiegelschreibungen auf Slot 0/2 kommen in
`totalSupply()`/`balanceOf()` an, und eine Registrierung landet genau auf 1, 3–8.
Gegenprobe gemacht: verschobenes Label in der Tabelle, fehlendes Nonce-Hochzählen und
fehlende High-s-Prüfung machen je die zuständigen Tests rot.

### (iv) Fassade nur Views, wo möglich

`name/symbol/decimals` sind Konstanten, `totalSupply/balanceOf` Views. Einzige
Nicht-View-Funktion der Fassade ist `transfer` – als `pure` und immer revertierend; sie
existiert nur, damit der Selektor im ABI steht und Wallets/Explorer das Token erkennen.

### (v) Bindet der Beweis die Wallet? Nein – Lücke dokumentiert

Circuit v3 (`aequitas-proof-server/biometric_v3.circom`):
`pubSignals = [commitment = Poseidon(bio, wallet, salt), nullifier = Poseidon(bio)]`.
Die Wallet ist **privater** Eingang; ohne `salt` kann niemand prüfen, für welche Wallet der
Beweis gemacht wurde. V8 schließt die Lücke teilweise:

- Die Nutzersignatur verhindert, dass jemand eine **fremde** Wallet registriert oder eine
  laufende Anfrage auf sich umlenkt.
- Die Registrar-Pflicht verhindert, dass jemand an Go vorbei registriert.

**Was bleibt:** Wer einen noch unbenutzten Beweis in die Hände bekommt (abgefangene
`/prove`-Antwort, bösartige App), kann ihn für **seine eigene** Wallet signieren. Die
Herkunftsprüfung in Go ist heute nur an den Nullifier gebunden
(`hatProveHerkunft(nullifier)`, `prove_provenance.go:131`), nicht an die Wallet. Der Test
`GAP (documented, needs circuit v4)` hält diese Grenze fest, damit v4 sie bewusst umkehrt.

Abhilfe, in dieser Reihenfolge:
1. **Sofort, in Go:** `/prove` kennt die Wallet (sie ist Eingang des Beweises). Die
   Herkunft als (Nullifier → Wallet) speichern und in `registerOnV7` verlangen, dass
   `wallet` gleich der beim `/prove` benutzten ist.
2. **Circuit v4:** Wallet als öffentliches Signal `pubSignals[2]`. Das verlangt einen neuen
   Verifier mit `uint[3]` und damit einen Nachfolger-Contract, der
   `pubSignals[2] == uint160(human)` prüft. Nicht in V8, weil das Verifier-Interface
   ausdrücklich wie heute bleiben soll und die Zeremonie für v4 noch aussteht.

## Go-Anbindung

**Nicht Teil dieses Branches** (keine Go-Datei geändert). Zeilen beziehen sich auf `main`
bei Commit `cc7babbe`. Grundsatz: **Keine V7-Slot-Zahl darf stehen bleiben.** Die
Nummern kollidieren gefährlich: V7-Slot 4 (`balanceOf`) ist in V8 `usedCommitments`,
V7-6 (`isHuman`) ist V8 `commitmentOf`, V7-8 (`usedNullifiers`) ist V8 `nonces`, V7-5
(`escrowOf`) ist V8 `usedNullifiers`. Ein vergessener V7-Schreiber schreibt also still in
fremde V8-Mappings.

### 1. Eine Slot-Datei statt verstreuter Zahlen

- Neu `x/humanity/keeper/v8_slots.go` mit genau den Konstanten aus `contracts/v8_slots.json`:
  `v8SlotTotalSupply=0, v8SlotTotalHumans=1, v8SlotBalanceOf=2, v8SlotIsHuman=3,
  v8SlotUsedCommitments=4, v8SlotUsedNullifiers=5, v8SlotCommitmentOf=6,
  v8SlotNullifierOf=7, v8SlotNonces=8, v8SlotIsRegistrar=9`.
- `evm_v7_slots_source_test.go` (ganze Datei, `TestV7SlotListsMatchContractSource`, liest
  heute `../../../AequitasV7.sol` per Regex) ersetzen durch einen Test, der
  `../../../contracts/v8_slots.json` liest und **jede** Konstante sowie die Persistenzlisten
  (unten) damit vergleicht. Zusätzlich ein Test, der in allen Nicht-Test-`.go`-Dateien
  `mappingSlot(…, <Zahl>)` / `mappingSlotBytes32(…, <Zahl>)` / `big.NewInt(<Zahl>)).Hex()`
  mit literaler Slot-Zahl außerhalb `v8_slots.go` verbietet.
- `evm_engine.go:704–802`: `v7SimpleSlots` → `{0, 1}`; `v7AddressMappingSlots` →
  `{2, 3, 6, 7, 8}`; `v7ArrayBaseSlots` entfällt; `v7SlotsVerifiedForVersion`,
  `checkV7SlotsMatchDeployedVersion`, `v7SlotsVerifiedFor` (und `api.go:661–672`) entfallen
  oder werden auf V8 umgestellt (der JSON-Test ersetzt sie).

### 2. Selektoren, Positivliste, Aufrufdaten-Offsets

- `evm_engine.go:428–436` `knownV7PublicPersistSelectors`: auf V8 zuschneiden.
  Empfehlung fail-closed: **leer** (Views brauchen keinen persistierenden Aufruf;
  `transfer` wird vorher abgefangen und revertiert sonst ohnehin). `dd62ed3e` (`allowance`)
  gab es schon in V7 nicht – streichen.
- `evm_engine.go:467` `checkPersistedCallAllowed`: `"13b81eb0"` → **`"60529762"`**
  = `registerWithSig(uint256[2],uint256[2][2],uint256[2],uint256[2],address,uint256,bytes)`.
  Relayer-Prüfung bleibt (der Contract prüft zusätzlich `isRegistrar`).
- `evm_engine.go:813–828` `extractTouchedEntitiesWithNullifier`: Selektor `60529762`;
  der Nullifier steht **nicht mehr** als eigener Parameter bei Offset 388, sondern ist
  `pubSignals[1]` bei **Offset 292–324**. Neue ABI-Kopf-Offsets (ab Byte 0):
  `pA 4–68, pB 68–196, pC 196–260, pubSignals[0] 260–292, pubSignals[1] 292–324,
  human 324–356, deadline 356–388, Offset von signature 388–420`.
- `evm_engine.go:830–898` `extractTouchedEntities`: Fall `13b81eb0` → `60529762`
  (human weiterhin 324, commitment weiterhin 260). Die Fälle `d3d2770c, e54655d2, 35a1e72b,
  434c099e, e14f2020, c304555f` entfallen (Funktionen gibt es nicht mehr); `a9059cbb` darf
  bleiben (revertiert in V8, schreibt nichts).
- `evm_engine.go:914–958` `extractPreCallEntities`: entfällt komplett (liest V7-Slots 13, 14, 28
  für Funktionen, die es nicht mehr gibt).
- `evm_engine.go:960–1006` `dumpAndPersistStorageWithNullifier`: Slot 8 → **5**
  (`usedNullifiers`); Slot 29 (`grantIssuedTo`) entfällt; `releasedNullifiers` entfällt.
- `evm_engine.go:1041` `dumpAndPersistStorage`: `usedCommitments` Slot 7 → **4**.
- Unverändert kompatibel: `transfer` `a9059cbb` (`evm_rpc.go:1460–1507`,
  `signierte_ueberweisung.go:127`), `balanceOf` `70a08231` (`evm_rpc.go:984–1010`),
  `isHuman` `f72c436f` (`evm_rpc.go:953–982`) – Selektoren bleiben gleich.
- Empfohlen: `eth_call` für `totalSupply()` `18160ddd` in `evm_rpc.go` (neben dem
  `balanceOf`-Abfang, ~Zeile 984) ebenfalls direkt aus Go beantworten; heute schreibt Go
  Slot 0 nur bei Migration und Registrierungs-Fallback, im Betrieb wäre `totalSupply()` sonst
  veraltet. Alternativ Slot 0 im Spiegel (`doSyncBalanceRLockedCtx`) mitschreiben.
- Hinweis: `transfer` per `eth_call` revertiert jetzt in der EVM. `eth_estimateGas` läuft nicht
  durch die EVM (`evm_rpc.go:1773`), MetaMask ist also nicht betroffen; sollte eine Wallet
  `transfer` per `eth_call` simulieren, diesen Selektor in `ethCall` mit `true` beantworten.

### 3. Spiegel (`evm_storage.go`, `guardian.go`, `state.go`)

- `evm_storage.go:822–833` (`doSyncBalanceRLockedCtx`): `balanceOf` Slot 4 → **2**,
  `isHuman` Slot 6 → **3**; die Schreibungen auf 10/11 (`lastActivity/lastDemurrage`)
  **entfallen**.
- `evm_storage.go:909–960` (`syncGuardianEscrowSlotsLocked*`, `SyncGuardianEscrowSlots`,
  schreibt V7-Slots 13 und 5) **entfällt**, ebenso die Aufrufe in `guardian.go:210, 397, 606, 689`.
  Achtung: V7-5 ist V8 `usedNullifiers` – dieser Spiegel darf auf V8 auf keinen Fall laufen.
- `evm_storage.go:1270` (`isHuman`-Diagnose): Slot 6 → **3**.
- `evm_storage.go:320–532` `MigrateEVMFromGoState`: schreibt nur noch Slot 0
  (`totalSupply`), 1 (`totalHumans`), 2 (`balanceOf`), 3 (`isHuman`), 4 (`usedCommitments`),
  5 (`usedNullifiers`, Wert = Wallet), 6 (`commitmentOf`), 7 (`nullifierOf`, neu – heute nicht
  migriert). Slots 2/3 (`ubiPool/ubiPerHumanAccumulated`), 10–12 entfallen. **Nie** Slot 8
  (`nonces`, nur der Contract) und **nie** Slot 9 (`isRegistrar`).
- `evm_storage.go:534–660` `Save/RestorePreUpgradeRelationshipSlots` (Guardian-/Escrow-Slots)
  entfällt.
- `state.go` (`syncBalanceLocked`/`syncHumanRegistrationLocked`-Aufrufe, z. B. 3717, 4548,
  5611): nur die Adresskonstante; die Slot-Zahlen stecken im Spiegel oben.

### 4. Registrierungsaufruf (`register.go`)

- `register.go:153–165` `registerWithSigABI`: neue Parameter
  `(uint256[2] pA, uint256[2][2] pB, uint256[2] pC, uint256[2] pubSignals, address human,
  uint256 deadline, bytes signature)` – der `bytes32 nullifier`-Parameter **entfällt**.
- `register.go:180–206` `RegisterRequest`: neues Feld `Deadline uint64`; `Signature` ist jetzt
  EIP-712 (`eth_signTypedData_v4`) statt `personal_sign`. Die **App** (Aequitas-App) muss
  die Typed Data aus Abschnitt (i) mit `chainId = 1926`, `verifyingContract = V8-Adresse`,
  `nonce = nonces(wallet)` (per `eth_call`, `0x7ecebe00`) signieren. Go prüft vor dem
  Dry-Run: `now ≤ deadline ≤ now + 86400` (gleiche Grenze wie im Contract).
- `register.go:522` `parsedABI.Pack("registerWithSig", pA, pB, pC, pubSignals, claimedHuman,
  deadline, sigBytes)`.
- `register.go:1164–1191` `validRegisterSignature` (Mirror-Fallback): auf den EIP-712-Digest
  umstellen (Domain + `Register`-Struct wie oben, Nonce aus Slot 8). Besser: den Fallback
  `ALLOW_V7_MIRROR_FALLBACK` (`register.go:563–630`, `persistRegisterWithSigMirror`
  `register.go:998–1162`, Slots 0, 1, 4, 6, 7, 8, 9–12) beim Neustart **streichen** – V8
  steht ab Genesis, der Startup-Race-Grund entfällt.
- `register.go:712–724` `pendingRegTx` / `block.go:28` `Transaction`: Signatur, Frist und
  Nonce mitgeben. **Jeder Knoten** prüft beim Nachspielen (`block.go:7180–7290`,
  `case "register_human"`) bisher nur den ZK-Beweis und die Nullifier-Bindung, **nicht** die
  Nutzersignatur. Mit V8 muss er zusätzlich die EIP-712-Signatur gegen `tx.Wallet`, die Frist
  gegen `block.Timestamp` (nicht die eigene Uhr) und die Nonce gegen den eigenen Go-Stand
  prüfen (AGENTS.md Punkt 4: nie einem Peer glauben).
- Herkunft an Wallet binden (siehe (v)): `prove_provenance.go` speichert (Nullifier → Wallet),
  `register.go:437` verlangt Gleichheit.
- Antworttext `register.go:333` („Registered … on Aequitas V7! 1,000 AEQ granted.“) anpassen.

### 5. Deploy und Genesis (`contract_deploy.go`, `evm_v6mirror.go`)

- `evm_v6mirror.go:20` `V7_CONTRACT_ADDR` → V8-Adresse (neu, Genesis). Umbenennen in
  `V8_CONTRACT_ADDR` erfasst alle Verwender: `api.go (2), contract_deploy.go (4),
  evm_engine.go (2), evm_rpc.go (5), evm_storage.go (2), guardian.go (4, entfallen),
  register.go (2), signierte_ueberweisung.go (1), snapshot.go (2), state.go (14),
  transfer_*_concurrent.go (2), wirtschaft.go (1)`.
- `evm_v6mirror.go:21` `BIO_VERIFIER_ADDR`: Verifier aus der Zeremonie.
- `contract_deploy.go:108` `V7ContractVersion` → z. B. `"v8.0"`; `:150` `V7ContractBytecode`
  → `V8ContractBytecode` aus `artifacts/contracts/AequitasV8.sol/AequitasV8.json`
  (`bytecode`). Dazu ein Sync-Test wie `test/AequitasV7_bytecode_sync.ts` für V8
  (Vorlage kopieren, Pfade/Konstante anpassen).
- `contract_deploy.go:313–318` Konstruktor-Argumente: jetzt
  `abi.encode(address verifier, address[] registrars, bytes32 netzSalt)` – mit go-ethereum
  `abi.Arguments.Pack` statt Handkodierung; `netzSalt = keccak256(netzKennung())`.
  `registrars` = **feste Genesis-Liste** der Relayer-Adressen aller Validatoren, in allen
  Knoten identisch (sonst unterscheidet sich der Contract-Zustand je Knoten).
- Der Konstruktor verlangt, dass am Verifier Code liegt: BioVerifier muss vorher deployt und
  in der StateDB sichtbar sein (`contract_deploy.go:165–180` läuft bereits vorher).
- `contract_deploy.go:331–380` (Code/Storage von der CREATE-Adresse zur Genesis-Adresse
  umziehen) **muss bleiben**: der Konstruktor schreibt `isRegistrar` (Slot 9) an der
  CREATE-Adresse; ohne Umzug hätte die Genesis-Adresse keinen Registrar und jede
  Registrierung schlüge fehl (fail-closed, aber Ausfall).
- `contract_deploy.go:187–300` Upgrade-Pfad von V7 (Slot 2/3 sichern, Beziehungs-Slots
  sichern) entfällt beim Neustart bei null.
- `genesis.json`: V8-Adresse, Registrar-Liste, Verifier-Adresse.
- Root-Kopie: V7 hat eine kanonische Kopie `/AequitasV7.sol` plus Sync-Test. V8 lebt
  kanonisch in `contracts/AequitasV8.sol`; keine zweite Kopie anlegen.

### 6. Gegentest in Go

Registrieren (über `/api/register` mit EIP-712), überweisen, Grundeinkommen auszahlen, danach
`balanceOf`/`isHuman`/`totalSupply`/`nonces` aus der EVM gleich Go; Positivliste lehnt alles
außer `60529762` vom Relayer ab; Peer-Nachspielen lehnt fremde Signatur, abgelaufene Frist und
alte Nonce ab.

## Tests

`npx hardhat test` (Hardhat 3, `node:test` + viem): 65 grün, davon 34 für V8.

- **Register:** Gutfall (Ereignis, alle Einträge, Nonce, kein Guthaben); zweiter Registrar;
  Digest gegen unabhängige EIP-712-Kodierung; Nicht-Registrar (auch der Mensch selbst);
  fremde Signatur (Betreiber, Fremder); Front-Running (anderer Empfänger); Manipulation von
  Commitment/Nullifier/Frist; Registrar als Mensch; Null-Adresse; andere Chain-ID; anderer
  Contract; anderer Name/Version; abgelaufene Frist; zu ferne Frist; Wiederholung;
  Wiederholung nach Freigabe (Nonce); doppelter Nullifier; doppeltes Commitment;
  Feld-Aliasing; Nullwerte; ungültiger Beweis (manipuliert und Verifier „nein“);
  hohes `s`; falsche Länge, `v` 0/1, Müll; dokumentierte Lücke (v).
- **Konstruktor:** Verifier null/ohne Code; Registrar-Liste leer, 17, doppelt, null; 16 ok.
- **Fassade:** Konstanten; `transfer` revertiert als `eth_call` und als Transaktion, Selektor
  `0xa9059cbb`; kein `approve/transferFrom/allowance/mint/burn`, einzige Nicht-View ist
  `registerWithSig`.
- **Speicherbelegung:** siehe (iii).
- **Danach** eine getrennte Sicherheitsprüfung des gesamten Diffs (AGENTS.md), auch der
  Go-Anbindung.

## Ausrollen

Nur mit dem Neustart der Kette bei null. Die Contract-Adresse ist im Knoten fest verdrahtet,
und ein Wechsel ist ein Konsens-Update aller Knoten. Registrar- und Verifier-Wechsel
bedeuten einen neuen Contract (kein Admin) – bewusst, beides ist ohnehin ein Konsens-Update.
Auf der laufenden Kette müssten die bestehenden Testmenschen aus V7 migriert werden, und der
Neustart löscht sie ohnehin.
