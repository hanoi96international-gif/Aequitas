# Forschung: Quantengeld für Aequitas

Stand 14.11.2026. **Forschung, kein Produkt.** Nichts hier läuft in der Kette,
in der App oder auf einem Server. Nach außen gilt: Aequitas ist
*quantensicher* (`docs/QUANTENSICHERHEIT.md`), nicht „Quantengeld“.

## 1. Das Problem, das wir lösen wollen

Aequitas gibt jedem Menschen einmal 1.000 AEQ. Heute liegt jede Überweisung
auf der Kette: Sie braucht Netz, ist öffentlich und wartet auf einen Block.

**Quantengeld würde Bargeld ermöglichen, das**
1. sich nicht kopieren lässt, weil die Physik das verbietet (Kopierverbot der
   Quantenmechanik), nicht weil eine Kette Doppelausgaben verhindert;
2. **jeder selbst prüfen kann**, ohne Bank, ohne Kette, ohne Netz;
3. offline und privat weitergegeben wird, so wie ein Geldschein.

Für Aequitas hieße das: Die Kette bleibt die **Ausgabestelle**. Dort entsteht
„ein Mensch, einmal 1.000 AEQ“, dort wird die Einmaligkeit des Menschen
geprüft. Wer will, tauscht AEQ in Quanten-Banknoten und zurück. Die
Einmaligkeit des Menschen leistet die Registrierung, die Unkopierbarkeit
der Noten die Physik. **Eines kann die Quantenphysik nicht:** verhindern,
dass sich eine Person zweimal anmeldet. Das bleibt Aufgabe des
Gesichtsabgleichs.

## 2. Was es schon gibt, und warum es nicht reicht

| Verfahren | Prüfen | Stand | Simulation hier |
|---|---|---|---|
| **Wiesner (1983)** | nur die Bank mit Geheimschlüssel | sicher, aber zentral | `wiesner.py` |
| Aaronson (2009), versteckte Unterräume u. a. | öffentlich | **gebrochen** | – |
| Gitter-Varianten | öffentlich | mehrere **gebrochen** (2022) | – |
| **Zhandry (2024)**, abelsche Gruppenaktionen | öffentlich | im generischen Modell bewiesen (2025), **kein Beweis aus Standardannahmen** | `zhandry_spielzeug.py` (unsicheres Spielzeug) |

**Die drei Lücken:**
1. **Mathematik:** Öffentlich prüfbares Quantengeld braucht ein Verfahren,
   dessen Sicherheit auf einer anerkannten, schweren Annahme beruht. Bisher
   gibt es Kandidaten, aber keinen solchen Beweis.
2. **Speicher:** Die Banknote *ist* ein Quantenzustand. Heutige Qubits halten
   Sekunden, und das nur gekühlt im Labor. Ein Handy, das eine Note tagelang
   hält, gibt es nicht.
3. **Übertragung:** Um eine Note zu überweisen, müsste der Quantenzustand von
   Gerät zu Gerät wandern. Ein solches Quanten-Internet für alle gibt es nicht.

## 3. Was die Simulationen zeigen

Beide rechnen exakt mit Zustandsvektoren. Sie brauchen keine Bibliotheken und
haben einen Selbsttest (`--selbsttest`), der in der CI läuft.

**`wiesner.py`:** Echte Noten bestehen immer. Ein Fälscher ohne Schlüssel
kommt je Qubit nur mit 3/4 durch, insgesamt mit (3/4)^n. Gemessen:

| Qubits | Fälschung besteht | Theorie |
|---|---|---|
| 1 | 0,7504 | 0,75 |
| 8 | 0,0996 | 0,100 |
| 16 | 0,0103 | 0,010 |
| 64 | 0 | 1,0·10⁻⁸ |

Grenze: Prüfen kann nur die Bank. Das ist für Aequitas zu zentral.

**`zhandry_spielzeug.py`:** Drucken und Prüfen funktionieren **ohne Bank**:
- Die Seriennummer entsteht als zufälliges Messergebnis. Niemand kann sie
  sich aussuchen, auch die Ausgabestelle nicht.
- Prüfen kann jeder, mit der öffentlichen Gruppenwirkung.
- Ein Fälscher mit einem klassisch bekannten Zustand besteht nur zu 1/N.

Das Spielzeug benutzt aber Z_N. Dort lässt sich jede Note aus ihrer
Seriennummer nachbauen, und genau diesen Angriff zeigt die Simulation.
Sicher wäre das Verfahren erst mit einer Gruppenaktion, in der das schwer
ist. Kandidaten sind Isogenien. **Das ist die offene Forschungsfrage.**

## 3a. Was die Hardware können muss (`rauschen.py`)

Eine Note liegt eine Zeit im Speicher, jedes Qubit zerfällt mit der
Kohärenzzeit T. Der Prüfer muss deshalb einige falsche Qubits dulden – und
jede Toleranz hilft auch dem Fälscher. Gerechnet ist exakt (Binomialschwänze),
mit zwei Forderungen: **die echte Note besteht zu ≥ 99 %, eine Fälschung
höchstens zu 10⁻¹²** (Wiesner-Noten, bester Fälscher 1/4 je Qubit; für
öffentlich prüfbare Verfahren ist das eine *untere* Schranke).

| Lagerzeit der Note | Note ≤ 256 Qubits | Note ≤ 2.048 Qubits |
|---|---|---|
| 1 Minute | T ≥ 10,6 Min. | T ≥ 2,5 Min. |
| 1 Stunde | T ≥ 10,6 Std. | T ≥ 2,5 Std. |
| **1 Tag** | **T ≥ 10,6 Tage** | **T ≥ 2,5 Tage** |
| 1 Woche | T ≥ 74 Tage | T ≥ 17 Tage |
| 30 Tage | T ≥ 318 Tage | T ≥ 75 Tage |

**Die Faustregel:** die Kohärenzzeit muss etwa das 2,5- bis 10-fache der
Lagerzeit betragen – je mehr Qubits, desto weniger. Größere Noten helfen nur
bis etwa zum Faktor 2: unterhalb von T ≈ 2 × Lagerzeit fallen echte und
gefälschte Noten statistisch zusammen, und keine Notengröße rettet das.

Zum Vergleich (Laborwerte, Größenordnung, vor Zitierung prüfen):
supraleitende Qubits Millisekunden, Ionenfallen Sekunden bis Minuten,
Kernspins in Festkörpern einige Stunden – jeweils gekühlt und ohne
Transport. **Eine Note, die man einen Tag in der Tasche trägt, braucht
Speicher, die Tage halten: rund 10- bis 100-mal länger als die besten
gekühlten Laborspeicher (Stunden) und milliardenfach länger als heutige
Rechen-Qubits (Millisekunden) – und das im Gerät, nicht im Labor.** Das ist die ehrliche Größe
der Hardware-Lücke – und zugleich der Maßstab, an dem sich jede künftige
Ankündigung messen lässt.

## 3b. Der Preis der Öffentlichkeit (`rauschen_oeffentlich.py`)

Beim öffentlich prüfbaren Verfahren (Gruppenaktion, Zhandry) ist die Note ein
Fourier-Zustand über ein Register aus m Qubits, geprüft wird durch eine
einzige Projektion. Hergeleitet und am Zustandsvektor geprüft (alle 4^m
Pauli-Fehler für kleine m, Monte-Carlo für größere):

> **Eine echte Note besteht nach der Lagerung genau mit (1 − p)^m** – für jede
> Seriennummer, p = Kippwahrscheinlichkeit eines Qubits.

Es gibt keine Mehrheit über viele Qubits wie bei Wiesner: **jedes Qubit muss
m-mal besser sein.** Für eine Note, die einen Tag lagert und zu ≥ 99 % besteht:

| Register m | einteilig: Kohärenz / Lagerzeit | für 1 Tag | mehrteilig (101 Teile, > 50 %) | für 1 Tag |
|---|---|---|---|---|
| 64 | 3.184 | 9 Jahre | 65 | 65 Tage |
| 256 | 12.736 | 35 Jahre | 262 | 262 Tage |
| 512 | 25.472 | 70 Jahre | 524 | 1 Jahr |
| 2.048 | 101.887 | 279 Jahre | 2.099 | 6 Jahre |

**Mehrteilige Note:** k unabhängige Teilnoten, angenommen bei mehr als der
Hälfte. Mehr als die Hälfte ist nötig gegen den Aufteilungsangriff (ein
Fälscher verteilt die echten Teilnoten auf zwei Noten – dann bräuchte er mehr
als k echte). Eine Teilnote muss dann nur noch zu etwa 60 % bestehen statt zu
99 % – das senkt die Forderung um das ~50-Fache.

**Folgerung für Aequitas:** Öffentlich prüfbares Quantengeld ist ohne
Quanten-Fehlerkorrektur nicht lagerfähig. Selbst mehrteilig liegt die
Forderung hunderte Male über Wiesner (2,5–10). Der realistische Weg führt
über **fehlerkorrigierte logische Qubits** als Speicher – genau die Technik,
die Quantenrechner ohnehin brauchen. Das ist der Meilenstein, an dem sich
messen lässt, wann eine Aequitas-Quanten-Note in Reichweite kommt. Und es ist
eine präzise, neue Frage an die Forschung: **rauschtolerante öffentliche
Prüfung** (bisher nur im Orakelmodell, arXiv:2407.06463).

## 4. Weitere Dokumente

- `BRUECKE.md` und `bruecke.py`: der Entwurf der Brücke AEQ ↔ Quanten-Note.
  Grundlage ist Coladangelo & Sattath (2020). Das Protokollmodell hat einen
  Selbsttest mit Angriffen.
- `FORSCHUNGSSTAND.md`: Arbeiten, mögliche Ansprechpartner, Förderung.
- `ANSCHREIBEN.md`: Vorlage für den ersten Kontakt. Verschickt wird sie nur
  vom Betreiber.

## 5. Plan: was wir tun können

1. **Verstehen und zeigen:** die Simulationen ausbauen. Erledigt:
   Hardware-Forderung unter Rauschen (`rauschen.py`, Abschnitt 3a). Als
   Nächstes: eine Gruppenaktion auf einer Menge, in der der Nachbau nicht
   trivial ist, als Lehrbeispiel ohne Sicherheitsanspruch. Erledigt:
   Rauschrechnung für das öffentlich prüfbare Verfahren (Abschnitt 3b).
2. **Brücke entwerfen** (Gedankenmodell): AEQ auf der Kette sperren, dafür
   eine Note mit öffentlicher Seriennummer; eine Note zurückgeben, dafür die
   AEQ zurück. Die Seriennummer steht auf der Kette, sodass sich jede Note nur
   einmal zurücktauschen lässt.
3. **Mit der Forschung sprechen:** Problembeschreibung und Simulationen an
   Gruppen schicken, die zu Quantengeld arbeiten (z. B. QuSoft Amsterdam, die
   Gruppen um Scott Aaronson und Mark Zhandry). Ansprechpartner und
   Förderprogramme prüft der Betreiber selbst. Hier stehen keine Namen oder
   Zusagen, die nicht bestätigt sind.
4. **Bis dahin:** Aequitas quantensicher machen (`docs/QUANTENSICHERHEIT.md`,
   Stufen 0 bis 5). Das schützt heute, und eine spätere Ausgabestelle für
   Quanten-Noten braucht genau diese Grundlage.

## Quellen

- S. Wiesner, „Conjugate coding“, SIGACT News 15(1), 1983
- M. Zhandry, „Quantum Money from Abelian Group Actions“: https://arxiv.org/pdf/2307.12120
- „Public-Key Quantum Money From Standard Assumptions (In The Generic Model)“, 2025: https://eprint.iacr.org/2025/092
- „Another Round of Breaking and Making Quantum Money“: https://arxiv.org/pdf/2211.11994
