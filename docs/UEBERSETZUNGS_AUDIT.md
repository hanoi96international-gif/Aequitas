# Übersetzungs-Audit der Website

**Stand 26.08.2026.** Jede Zeile von `assets/explorer.html` maschinell geprüft.

## Ergebnis

| | vorher | nachher |
|---|---|---|
| Sichtbare Textstellen **ohne** `data-i18n` | **392** | **15** |
| Übersetzte Schlüssel je Sprache | 0 | **145** (plus 34 für den Bio-Verifier) |
| Sprachen | — | **alle 12** |

## Was bewusst NICHT übersetzt wird

**Die beiden Leitfäden** (Node und Bio-Verifier). Sie sind per Entwurf englisch, mit Sprachhinweis und übersetzten PDFs — dieselbe Regel, die der Node-Guide seit jeher befolgt.

**Code, Adressen und Platzhalter.** Shell-Befehle, `0xYOUR_PRIVATE_KEY`, libp2p-Adressen, Formeln wie `AEQ_reserve × tUSD_reserve = k`. Ein übersetzter Shell-Befehl ist kein Schönheitsfehler, sondern eine Anleitung, die nicht mehr funktioniert.

**Fachbegriffe und Eigennamen** bleiben in allen Sprachen gleich: `Groth16 / BN128`, `libp2p`, `PostgreSQL`, URLs. Sie zu übersetzen hieße, eine Angabe zu erfinden.

## Warum die vorhandenen Tests das nicht gefunden haben

`TestI18nLocaleKeysMatchEnglish` prüft, ob **jeder vorhandene Schlüssel** in jeder Sprache existiert. `TestI18nMarkupKeysExistInDictionary` prüft, ob jeder benutzte Schlüssel **definiert** ist.

Keiner von beiden prüft, ob sichtbarer Text **überhaupt einen Schlüssel hat**. Genau dort lag die Lücke: 392 Stellen ohne Schlüssel blieben in allen zwölf Sprachen englisch, und nichts meldete es.

## Verbliebene Stellen

- Zeile 1746: Protocol Mechanisms
- Zeile 1754: Contract Code
- Zeile 1766: What happens to AEQ when people die or become permanently incapacitated? In Bitcoin and most cryptoc
- Zeile 1771: What if someone is hospitalized, incarcerated, or otherwise unable to access their device for months
- Zeile 1776: Demurrage is a holding cost on money — a negative interest rate that makes hoarding expensive and ci
- Zeile 1792: Open Source Chain Logic
- Zeile 1802: ◆ Consensus: GHOSTDAG + KNIGHTDAG
- Zeile 1803: → Network / Consensus
- Zeile 1806: Node Decentralization Roadmap
- Zeile 1808: Phase 0 (now):
- Zeile 1809: Phase 1 (100+ humans):
- Zeile 1810: Phase 2 (1,000+ humans):
- Zeile 1811: Phase 3 (10,000+ humans):
- Zeile 1830: @AequitasMoney
- Zeile 1831: t.me/aequitasmoney
