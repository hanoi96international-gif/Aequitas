# Hardhat + viem project

## Project layout

```
contracts/        Solidity source files (*.sol) and unit tests (*.t.sol)
test/             TypeScript integration tests and Solidity unit tests (*.sol)
ignition/         Hardhat Ignition deployment modules
scripts/          Standalone scripts run with `hardhat run`
hardhat.config.ts
```

## Working in this project

When writing or modifying tests, configuring `hardhat.config.ts`, or interacting with the network from TypeScript, invoke the **`hardhat`** skill. It covers Solidity and TypeScript testing, how to choose between them, `forge-std` cheatcodes, the `network.create()` API, `networkHelpers`, and the compile-then-typecheck workflow. The skill itself points to the matching `hardhat-toolbox-*` skill for toolbox-specific guidance (clients, contract interaction, assertions).

## Docs

- Hardhat 3 — https://hardhat.org/llms.txt
- viem — https://viem.sh/llms.txt

## Sicherheit — Launch-Massstab (gilt fuer jede Aenderung)

Die Beta laeuft unter Launch-Bedingungen: echte Nutzer, echte Werte. Sicherheit
wird beim Bauen beruecksichtigt, nicht hinterher geprueft. Vor dem ersten
Code-Zeichen einer Aenderung werden diese Fragen beantwortet, und die Antworten
stehen im PR unter **Sicherheit**:

1. **Wer erreicht diesen Code?** Oeffentliches RPC/HTTP, Peer (P2P, Block-Push),
   nur ein berechtigter Validator, nur der Betreiber? Alles, was von aussen
   kommt, ist feindlich, bis es geprueft ist — auch Bloecke von Peers.
2. **Was ist begrenzt?** Jede Schlange, Map, Goroutine-Zahl, Nutzlast und jede
   Wartezeit hat eine feste Obergrenze. Kein Eingang darf Speicher, CPU,
   Verbindungen oder Sperren ohne Grenze binden.
3. **Was passiert im Fehlerfall?** Sicherheitspruefungen schliessen ab
   (fail-closed). Ein Fehler darf nie dazu fuehren, dass eine Signatur-,
   Nonce-, Deckungs- oder Berechtigungspruefung uebersprungen wird.
4. **Bleibt jede Pruefung bei jedem Knoten?** Signaturen, Nonces, Deckung,
   StateRoot prueft jeder Knoten selbst. Optimierungen duerfen Pruefungen
   verschieben oder parallelisieren, nie weglassen oder einem Peer glauben.
5. **Missbrauchstest:** zu jeder neuen Pruefung oder Grenze ein Test, der den
   Angriff zeigt (Faelschung, Wiederholung, Ueberlauf, fremder Absender) —
   nicht nur der Gutfall.

Feste Regeln:

- Schutzgrenzen (Rate-Limits, Inflight-/Rueckstau-Grenzen, Groessenlimits,
  SSRF-Schutz) werden nie fuer Messungen oder Bequemlichkeit gelockert.
- Kein Test wird uebersprungen, abgeschaltet oder abgeschwaecht, um gruen zu
  werden. Nebenlaeufiger Code muss unter `go test -race` gruen sein.
- Keine Geheimnisse (Schluessel, Tokens, Passwoerter) in Code, Logs,
  Workflows oder Fehlermeldungen. Workflows bekommen nur die Rechte, die sie
  brauchen.
- Neue Abhaengigkeiten nur mit Begruendung; `govulncheck` im CI muss gruen sein.
- Aenderungen an Konsens, Signaturen, Kontostaenden, Contracts oder
  Netzwerk-Eingaengen bekommen vor dem Merge eine eigene Sicherheitspruefung
  des Diffs (getrennter Durchgang, Ergebnis als PR-Kommentar).
