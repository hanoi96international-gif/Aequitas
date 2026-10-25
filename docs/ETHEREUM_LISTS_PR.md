# Chain-ID 19260930 bei ethereum-lists eintragen

Die Datei liegt fertig unter `deploy/ethereum-lists/eip155-19260930.json`.
ethereum-lists/chains ist ein fremdes Repo; den PR reichst du mit deinem
GitHub-Konto ein. Am Handy geht es so:

1. https://github.com/ethereum-lists/chains öffnen, **Fork** tippen.
2. Im Fork in den Ordner `_data/chains/` wechseln, **Add file → Create new
   file**.
3. Dateiname: `eip155-19260930.json`. Inhalt: genau die Datei aus
   `deploy/ethereum-lists/` (unten noch einmal).
4. **Commit changes**, dann **Contribute → Open pull request** mit Titel und
   Text von unten.

Vorher prüfen (die Prüfung dort macht dasselbe):
- In `_data/chains/` gibt es noch keine Datei mit `19260930` und keinen
  Eintrag mit `"shortName": "aeq"` (Suche im Repo nach `"aeq"`).
- `https://aequitas.digital/rpc` meldet auf `eth_chainId` die Antwort
  `0x125e602` (= 19260930).

## Datei

```json
{
  "name": "Aequitas Chain",
  "chain": "AEQ",
  "rpc": ["https://aequitas.digital/rpc"],
  "faucets": [],
  "nativeCurrency": {
    "name": "Aequitas",
    "symbol": "AEQ",
    "decimals": 18
  },
  "infoURL": "https://aequitas.digital",
  "shortName": "aeq",
  "chainId": 19260930,
  "networkId": 19260930
}
```

## Titel

```
Add Aequitas Chain (19260930)
```

## Text

```
Adds Aequitas Chain, chainId/networkId 19260930.

- RPC: https://aequitas.digital/rpc (public, no API key; eth_chainId returns 0x125e602)
- Native currency: AEQ, 18 decimals
- Website: https://aequitas.digital
- No faucet

The chain runs since the restart from genesis on 21.10.2026 with this ID.
```
