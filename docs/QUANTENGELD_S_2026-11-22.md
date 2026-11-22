# Nachweis: erstes Quantengeld S komplett über die App (22.11.2026)

Erster Durchlauf von Quantengeld Variante S im Hauptnetz, bei dem Inhaberin
und Annahmestelle die Aequitas-App waren (App 1.30.1). Diese Datei hält die
Angaben dauerhaft fest; die Lauf-Logs bei GitHub verfallen nach 90 Tagen, und
die Kette räumt beendete Token nach gut einer Woche aus dem Register
(`quantenWToteBehalten`).

## Angaben

| | |
|---|---|
| Token K | `ffb9ff3538bd51abaad2c303adc8f55a07aff983c2c366d456d6b93d114aad84` |
| Stellen n | 1104 |
| Validatoren | 1 (C1, `0x3066639af1653325100c1074a9696612dbdc42dc`) |
| Inhaberin | `0x3797955ab19367dd27215b99263fc8a98e2047d9` (App) |
| Annahmestellen | 0: `0x3066…42dc` (C1), 1: `0x5abcd4524f06df8895f56d8afefc8559622560b4` (App) |
| Deckung | 1 AEQ (W-Topf) |
| Ausgabe | Workflow `quanten-s-ausgabe.yml`, Lauf [45050231784](https://github.com/hanoi96international-gif/Aequitas/actions/runs/45050231784), Weg `rollen`, Modus `ibm` |
| Quantenrechner | IBM `ibm_marrakesh`, Job `dc1ifvlhdvas73bksj8g`, Mehrheit aus 5 Schüssen |
| Fehler bei passender Basis (Ausgabe) | 0 von 585 |

## Ablauf

1. **Bestellung** in der App: Schlüssel und Basen auf dem Gerät erzeugt, zwei
   Annahmestellen genannt.
2. **Ausgabe** über den Workflow (Freigabe `produktion`): Präparation und
   Messung auf `ibm_marrakesh`, Token auf der Kette ausgegeben.
3. **Abholen** in der App: Abschnitt beim Validator geholt, selbst gegen Kette
   und Validator geprüft („Token … angenommen").
4. **Bindung und Wahl** von Annahmestelle 1 durch die Inhaberin; Vorlage als
   QR-Code.
5. **Annahme** in der App als Annahmestelle 1: Vorlage gescannt, Stichprobe
   erst nach der Finalität von Bindung und Wahl erhalten, **301 Stellen
   nachgerechnet, 0 Fehler** („vorab: besteht"), Anmeldung eingereicht.
6. **Urteil der Kette: `eingeloest`** (gegen 17:55 UTC). Der Wert ging aus dem
   W-Topf an die Annahmestelle.
7. **Zweite Vorlage** derselben Vorlage und erneute Wahl als Inhaberin: von der
   App abgelehnt, bevor etwas eingereicht wurde (die Kette hätte sie ebenfalls
   abgewiesen; der Token stand nicht mehr im Register). Die zunächst
   irreführende Meldung ist in App-PR #105 korrigiert.

## Selbst prüfen

- Bis etwa 30.11.2026: `GET https://aequitas.digital/api/quanten-w/note?k=ffb9ff35…aad84`
  liefert `"tot": true, "urteil": "eingeloest"`.
- Danach: Blöcke des Hauptnetzes (Transaktionen `quanten_w_s_ausgeben`,
  `…_binden`, `…_waehlen`, `…_anmelden`, `…_offenlegen` zu diesem K).

## Ehrlich

- Der Token besteht aus gespeicherten Messdaten, nicht aus Quantenzuständen in
  der Hand. Den Schutz geben geheime Basen, Stichprobe und Kette.
- Es arbeitete ein einziger Validator (C1, Betreiber). Inhaberin und
  Annahmestelle 1 gehörten derselben Person (Test des Betreibers).
- Erst mehrere unabhängige Validatoren, die selbst messen, machen daraus ein
  dezentrales Verfahren.
