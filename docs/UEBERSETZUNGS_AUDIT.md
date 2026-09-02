# Übersetzungs-Audit der Website

**Stand 26.08.2026.** Jede Zeile von `assets/explorer.html` maschinell geprüft.

## Ergebnis

| | vorher | nachher |
|---|---|---|
| Sichtbare Textstellen **ohne** `data-i18n` | **392** | **0** |
| Übersetzte Schlüssel je Sprache | 0 | **192** |
| Sprachen mit vollständigem Schlüsselsatz | — | **alle 12** |

Aufteilung der 192: 145 aus dem ersten Durchlauf, 34 für den Bio-Verifier-Leitfaden, 13 aus dem Nachlauf.

## Was bewusst NICHT übersetzt wird

**Die beiden Leitfäden** (Node und Bio-Verifier). Sie sind per Entwurf englisch, mit Sprachhinweis und übersetzten PDFs — dieselbe Regel, die der Node-Guide seit jeher befolgt.

**Code, Adressen und Platzhalter.** Shell-Befehle, `0xYOUR_PRIVATE_KEY`, libp2p-Adressen, Formeln wie `AEQ_reserve × tUSD_reserve = k`. Ein übersetzter Shell-Befehl ist kein Schönheitsfehler, sondern eine Anleitung, die nicht mehr funktioniert.

**Fachbegriffe und Eigennamen** bleiben in allen Sprachen gleich: `Groth16 / BN128`, `libp2p`, `PostgreSQL`, URLs, `@AequitasMoney`. Sie zu übersetzen hieße, eine Angabe zu erfinden.

## Warum die vorhandenen Tests das nicht gefunden haben

`TestI18nLocaleKeysMatchEnglish` prüft, ob **jeder vorhandene Schlüssel** in jeder Sprache existiert. `TestI18nMarkupKeysExistInDictionary` prüft, ob jeder benutzte Schlüssel **definiert** ist.

Keiner von beiden prüft, ob sichtbarer Text **überhaupt einen Schlüssel hat**. Genau dort lag die Lücke: 392 Stellen blieben in allen zwölf Sprachen englisch, und nichts meldete es.

**Empfehlung:** diese Prüfung als eigenen Test aufnehmen, sonst wächst die Lücke mit jedem neuen Abschnitt wieder nach.

## Zwei eigene Fehler auf dem Weg, beide vom Wächtertest gefangen

1. **Der erste Durchlauf erfasste die Leitfäden mit** — der Ausschlussbereich war per `min/max` zweier Marken berechnet statt per Klammerzählung, was ein winziges Fenster ergab. Dabei landeten Shell-Befehle in der Übersetzungsliste. Zurückgesetzt und neu gemacht.

2. **Das Einfüge-Werkzeug räumte pauschal alle `bv-*`-Zeilen weg** und schrieb nur die übergebenen zurück; beim Nachtragen eines einzigen Schlüssels verschwanden 34 andere. Zweimal gefangen, dann zielgenau gemacht.
