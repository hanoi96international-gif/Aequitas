# Chain-ID-Wechsel 1926 → 19260930 (0x125E602)

Stand: 15.10.2026. Der Betreiber hat den Wechsel entschieden. Im Code ist er
**schlafend** (`chainIDWechselAbUnix = math.MaxInt64`,
`x/humanity/keeper/chain_id_wechsel.go`).

## Warum

Die Kette wurde am 30.09.2026 bei null neu gestartet (V8-Genesis). Die
EVM-Chain-ID 1926 der alten Kette blieb aber gleich. Eine auf der alten Kette
signierte Überweisung (Nonce n, Chain-ID 1926) ist deshalb auch hier gültig
signiert. `NaechsteNonce` begann hier wieder bei 0. Wer alte Rohformen
gesammelt hat, kann sie erneut einreichen.

## Was der Wechsel tut (ab dem Stichtag `Ab`)

| Stelle | vor `Ab` | ab `Ab` |
|---|---|---|
| Blockprüfung jeder Überweisung (`pruefeUeberweisungsRoh`, an `block.Timestamp`) | nur 1926 | nur 19260930 |
| V8-Registrierung beim Nachspielen (`pruefeRegistrierungV8`, an `RegAt`) | Domäne mit 1926 | Domäne mit 19260930 |
| EVM `block.chainid` (`chainConfigFuer(ts)`) | 1926 | 19260930 |
| `eth_chainId` / `net_version` / `/api/status` `chain_evm_id` | 0x786 / 1926 | 0x125e602 / 19260930 |
| Annahme von Überweisungen (`chainIDAnnahmePruefen`) | 1926 bis `Ab − 600 s` | 19260930 ab `Ab` |

Zwischen `Ab − 600 s` und `Ab` nimmt der Knoten **keine Überweisungen** an
(wiederholbar, `-32005`). In dieser Zeit kommt der Ausgang in Blöcke. Eine
1926-Überweisung, die beim Stichtag noch im Ausgang liegt, bekommt einen
Block mit der Zeit `Ab − 1` (`block_tauglich.go`, wie nach einem Absturz).
Nie bekommt sie einen Block ab `Ab`. Gibt es schon einen Block ab `Ab`, baut
der Knoten keinen Block mehr (fail-closed). Dann hilft: Resync vom Seed.

`/api/status` hat das neue Feld `chain_id_wechsel_ab`. Es nennt die Unix-Zeit
des Stichtags oder 0, solange der Wechsel schläft.

Alte Blöcke spielen unverändert nach: Ihre Blockzeit liegt vor `Ab`.

## Was eingefroren bleibt (immer 1926)

- `netzKennung` = `aequitas-1926-<genesis>` und damit `v8NetzSalt`.
  `NETZ_SALT` ist im V8-Vertrag immutable.
- Die Sätze der Coordinator-Bindung und der Bescheinigung (`chain:1926`),
  dazu die Validator-Bindung. Gespeicherte Freigaben werden erneut geprüft.
- `checkpointChainID` (Checkpoint-Zertifikate).
- Die PoH-Bescheinigung `/api/credential` (`chain=1926`).

Test: `TestChainIDWechsel_EingefrorenesBleibt`.

## Ablauf für den Betreiber

1. **Dieser PR, schlafend.** Er ändert nichts, alles bleibt byte-gleich
   (`TestChainIDWechsel_SchlafendByteGleich`). Er wird auf allen Knoten
   ausgerollt.
2. **App-Release mit beiden IDs.** Die App muss vor und nach `Ab` richtig
   signieren. Sie liest `chain_evm_id` und `chain_id_wechsel_ab` aus
   `/api/status` und nimmt dieselbe Regel wie der Knoten. Für Überweisungen
   zeigt sie in der Pause einen Hinweis.
3. **Ankündigung mindestens 7 Tage vorher.** MetaMask und andere Wallets:
   das Netz mit Chain-ID 19260930 neu anlegen. Das alte Netz bleibt bis `Ab`
   gültig und wird danach entfernt.
4. **PR mit dem Stichtag**, mindestens 48 h Vorlauf vor `Ab`. Er setzt
   `chainIDWechselAbUnix` auf einen Wert, der auf allen Knoten gleich ist.
   Alle Knoten müssen ihn **vor** `Ab − 600 s` laufen. Ein Knoten ohne ihn
   verwirft ab `Ab` jeden Block mit neuen Signaturen.
   Rückweg vor `Ab`: wieder `math.MaxInt64`, ausrollen.
5. **Workflows und Web nachziehen** (siehe unten).

**Vor dem Festlegen:** In
[ethereum-lists/chains](https://github.com/ethereum-lists/chains) prüfen,
dass 19260930 frei ist. Am besten die ID dort eintragen.

## Offen außerhalb dieses PRs

- **App (Aequitas-App):** `lib/config.ts` und `lib/signer.ts` sowie die
  Prüfung des Netzes vor dem Signieren (`chain_evm_id`). EIP-712 für V8 liest
  `eip712Domain()` am Vertrag; damit folgt `chainId` von selbst. Dazu
  Zahlungslinks (`zahlungslink`) und das WalletConnect-Netz. Die
  Tests auf beide IDs umstellen.
- **Web/Explorer** (`x/humanity/keeper/assets/dapp.js`, `explorer.js`,
  `explorer.html`, `landing.js`, `aequitas-dapp.html`): `wallet_addEthereumChain`
  und Chain-Prüfung auf die ID aus `/api/status`. Landing-Texte
  („Chain ID 1926“ in `landing.go`) anpassen.
- **Workflows:** `pruefstand-live.yml:130`, `contabo1-serve-domain.yml:170-182`,
  `contabo2-api-route.yml:86` prüfen fest auf 0x786/1926. Sie müssen ab `Ab`
  die ID aus `chain_evm_id` bzw. beide IDs zulassen.
- `tools/coordinator-signieren.html` bleibt bei `chain:1926` (eingefroren).
