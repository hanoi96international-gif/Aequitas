# Chain-ID-Wechsel 1926 → 19260930 (0x125E602)

Stand: 17.10.2026. Der Stichtag ist **gesetzt**:
`chainIDWechselAbUnix = 1792281600` = **18.10.2026, 00:00:00 UTC**
(auf Anweisung des Betreibers vom 27.10. vorgezogen, ohne 7-Tage-Ankündigung)
(`x/humanity/keeper/chain_id_wechsel.go`). Annahmepause ab 17.10.2026,
23:50 UTC. App 1.11.0 (liest die ID live) ist das Release dazu; die
Startseite kündigt den Wechsel an (`chain_id_hinweis.go`, bis zum Stichtag).
Rückweg nur **vor** `Ab`: wieder `math.MaxInt64`, ausrollen.

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
Das passiert nur bei einem Ausfall oder Neustart rund um den Stichtag oder
wenn eine Uhr mehr als etwa 600 s abweicht.

**Restfenster nach dem Stichtag:** Der Konsens lässt Blockzeiten bis 120 s
vor dem neuesten Elternblock zu (`zeitstempelRueckToleranz`). Ein böswilliger
Validator kann deshalb noch etwa 2 Minuten nach dem Stichtag alte
1926-Überweisungen in Blöcke mit einer Zeit vor `Ab` legen. Neues Geld
entsteht dabei nicht: Signatur und Nonce prüft jeder Knoten gegen den
gemeinsamen Zustand, und jede dieser Überweisungen kann nur einmal laufen.
Wer eine alte, noch nicht verbrauchte Überweisung signiert hat, sollte sie
vor dem Stichtag mit einer neuen Überweisung derselben Nonce ersetzen.

Die Form der Einreichung ändert daran nichts. Auch eine typisierte
Transaktion, die als RLP-String verpackt eingereicht wird, bekommt das
richtige Zeitfenster (`rohChainID` dekodiert sie voll). Im Bündel reserviert
der Knoten keine Nonce, bevor die Chain-ID geprüft ist.

`/api/status` hat das neue Feld `chain_id_wechsel_ab`. Es nennt die Unix-Zeit
des Stichtags oder 0, solange der Wechsel schläft.

Alte Blöcke spielen unverändert nach: Ihre Blockzeit liegt vor `Ab`.

## Am Stichtag (18.10.2026) -- Betrieb mit einem Validator

Solange C1 der einzige Validator ist, gibt es keinen Seed, von dem er sich
heilen könnte. Der Schutz ist deshalb Vorbeugung:

1. **Deploy-Stopp 17.10.2026 23:00 bis 18.10.2026 01:00 UTC.** Kein Merge
   auf `main` (jeder Merge startet C1 über `deploy-c1-dann-c2.yml` neu), kein
   Neustart, keine Workflows, die Container anfassen. Grund: Ein Neustart
   über den Stichtag hinweg mit noch offenen 1926-Überweisungen im Ausgang ist
   der einzige Weg in den Stillstand unten.
2. **Vorher prüfen (17.10., bis 23:00 UTC):** `/api/status` meldet
   `chain_id_wechsel_ab = 1792281600` auf jedem Knoten (auch Leseknoten);
   die App-Version 1.11.0 ist das neueste Release.
3. **Ab 23:50 UTC** nimmt C1 keine Überweisungen an (`-32005`, wiederholbar).
   Der Ausgang leert sich in dieser Zeit (Blöcke alle paar Sekunden); was
   beim Stichtag noch darin liegt, bekommt die Blockzeit `Ab − 1`.
4. **Nach 00:00 UTC beobachten:** Höhe steigt (`/api/status`), `eth_chainId`
   meldet `0x125e602`, im Log kein `[BLOCK] ✗ Kein Block`.

**Wenn C1 trotzdem steht** (`[BLOCK] ✗ Kein Block ... Resync vom Seed,
Ausgang verwerfen`): Eine 1926-Überweisung im Ausgang passt in keinen Block
mehr. Den Ausgang einfach zu leeren reicht **nicht** -- C1 hat diese
Überweisungen beim Annehmen schon in seinen Zustand übernommen und würde
danach eine StateRoot erzeugen, die kein anderer Knoten nachrechnet. Weg:
`chainIDWechselAbUnix` lässt sich nach `Ab` nicht zurücknehmen (es gäbe
schon Blöcke ab `Ab`). Stattdessen C1 auf den letzten Stand vor dem
Stillstand zurücksetzen: Läuft bis dahin ein zweiter Validator, per Resync
von ihm (`recover-contabo1-resync.yml`); sonst aus der Ledger-Sicherung
(`backup-ledger.yml`) mit leerem Ausgang. Danach die betroffenen Nutzer
bitten, die Überweisung mit der neuen ID erneut zu senden. Das ist ein Eingriff des Betreibers mit
Datenverlust der nicht eingeschlossenen Überweisungen -- deshalb Punkt 1.

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
Geprüft am 16.10.2026 (Stand `9073ce8`, 15.10.2026): keine
`eip155-19260930.json`, kein Eintrag mit `chainId` 19260930, kein Name
„Aequitas“ — frei. (1926 steht dort übrigens auch nicht.) Vor dem
Stichtags-PR erneut prüfen.

## Clients (Zweig `claude/weiter-gehts-vm8979-chainid-clients`)

Grundsatz überall: Die Chain-ID zum Signieren kommt live vom Knoten
(`/api/status` `chain_evm_id` **und** `/rpc` `eth_chainId`, beide gleich) und
gilt nur, wenn sie genau 1926 oder 19260930 ist (19260930 nur mit
`chain_id_wechsel_ab` > 0). Unerreichbar, ungültig oder uneinig → es wird
nicht signiert. Kein Rückfall auf eine eingebaute Zahl.
Zusätzlich gegen die Geräte- bzw. Serveruhr (Toleranz 300 s): 1926 gilt
nicht mehr ab `Ab + 300 s`, 19260930 nicht vor `Ab − 300 s` — für jede
Unterschrift und in den Workflows. Abfragen haben eine Zeitgrenze, die auch
das Lesen der Antwort umfasst.

- **App (Aequitas-App):** `lib/kette.ts`, `lib/ketteLive.ts`; Überweisung
  (eigener Schlüssel und WalletConnect), EIP-712 V8, V7-Nachricht,
  Netzeinrichtung in der Wallet, Zahlungslinks (lesen: beide IDs; Kasse:
  live). In der Pause vor dem Stichtag keine Überweisung (Hinweis).
- **Web:** `assets/dapp.js` (Überweisung mit `chainId` im Auftrag),
  `assets/explorer.js` (MetaMask/WalletConnect beide IDs, V7/V8 mit der live
  gemeldeten ID, Texte über `.aeq-cid`), `explorer.html`,
  `aequitas-dapp.html`; `landing.js`/`landing.go` neutral ohne ID.
  Prüfvektor für 19260930: `TestV8_GoldenVektorNeueChainID` (Go),
  `test/Explorer_registerV8.ts`, App `registerV8.test.ts`.
- **Workflows:** `pruefstand-live.yml`, `contabo1-serve-domain.yml`,
  `contabo2-api-route.yml`: `eth_chainId` muss zu `chain_evm_id` desselben
  Knotens passen (1926 oder 19260930 mit Stichtag), sonst rot/Abbruch.

## Offen

- `tools/coordinator-signieren.html` bleibt bei `chain:1926` (eingefroren).
- `tools/contabo-loadtest/main.go` liest die ID jetzt live (`eth_chainId`,
  nur 1926 oder 19260930). Um den Stichtag keine Lasttests fahren.
- Betriebshandbücher aus `build/generate_*_guides.py` /
  `build/patch_pdf_langs.py` nennen „Chain ID 1926“: vor der Ankündigung
  neu erzeugen.
- Nach dem Stichtag (alle Wallets umgestellt): in einem eigenen Release
  1926 aus der App-Liste, AppKit-`defaultNetwork` und den Web-Listen
  streichen.
