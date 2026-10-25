# Wirtschaftsregeln: Endfassung (Vorschlag „Gleichgewicht“)

Stand 25.10.2026. Dieses Dokument schließt die Prüfungen vom 02.10.2026 ab
(`WIRTSCHAFT_REIFEPRUEFUNG.md`, `WIRTSCHAFT_ZAHLENPRUEFUNG.md`,
`WIRTSCHAFT_LITERATUR.md`, `GRUNDPFEILER.md`) und kommt zu einem Ergebnis:
welche Regeln Aequitas auf Dauer haben sollte, warum, was das kostet und wie
es in den Code kommt.

**Status:** Vorschlag zur Entscheidung. Gebaut sind das Rechenmodell
(`go run ./cmd/wirtschaftsmodell -vorschlag`, Tests in
`x/humanity/keeper/wirtschaftsmodell_gleichgewicht_test.go`) und, seit
25.10.2026, **die Regel in der Kette, schlafend** (`wirtschaft3.go`,
`nachrechnen_umlauf.go`, Aktivierung `gleichgewichtAktivAbUnix` = 2100;
Stand in Abschnitt 10). Bis zur Aktivierung rechnet die Kette nach
`wirtschaft.go`. Scharf wird sie nur mit eurer Entscheidung und einem echten
Datum, das nach dem Ausrollen auf alle Knoten liegt.

**Grenze:** Die Originalstudien ließen sich aus der Arbeitsumgebung wieder
nicht abrufen (Netzwerkrichtlinie). Ausgewertet sind Suchergebnisse,
Zusammenfassungen und Projektdokumentationen; die Quellen stehen am Ende.
Die Mathematik in Abschnitt 4 hängt an keiner Quelle, sie ist nachgerechnet
und getestet.

---

## 1. Ergebnis

1. **Eine Regel für jedes Konto:** Jedes Guthaben gibt **0,5 % im Monat** ab,
   ab dem ersten AEQ, für Menschen, Unternehmen und freie Adressen gleich.
   LP-Anteile zählen mit, Protokoll-Töpfe nicht.
2. **Alles an alle:** Was abgegeben wird, geht täglich zu gleichen Teilen an
   jeden Menschen mit gültigem Lebensnachweis. Das sind rund **5 AEQ im Monat
   je Mensch** (0,5 % des fairen Anteils), egal wer wie viel hält: etwas
   weniger, soweit Geld in Protokoll-Töpfen liegt, etwas mehr, soweit Menschen
   pausiert sind.
3. **Netto zahlt nur, wer mehr als den fairen Anteil hat.** Ein Mensch zahlt
   netto `0,5 % × (Guthaben − 1.000 AEQ)` im Monat. Wer weniger hat, bekommt
   dazu, wer genau 1.000 AEQ hat, zahlt nichts. Jedes Guthaben nähert sich mit
   der Zeit dem fairen Anteil.
4. **Keine Überweisungsgebühr.** Bezahlen kostet nichts. Die Freibeträge, die
   Gebührenstaffel und das Liegegeld nach Umsatz entfallen.
5. **Es bleiben:** 1.000 AEQ je Mensch bei der Registrierung, die
   Vermögensgrenze der Menschen (25 ×, Eingänge darüber werden abgelehnt), die
   Grenze freier Adressen (250 AEQ), Swap-Gebühr und Ausstiegsabgabe bis zum
   echten Ausgang.

In einem Satz für die Website: **„Jedes Guthaben gibt 0,5 % im Monat ab, und
alles geht zu gleichen Teilen an alle Menschen zurück: Wer weniger als den
fairen Anteil hat, bekommt dazu.“**

---

## 2. Befund: was an den heutigen Regeln nicht trägt

Die heutigen Regeln sind sorgfältig gebaut und gegen viele Angriffe geprüft.
Als Endzustand haben sie fünf Schwächen, die sich nicht durch andere Zahlen
beheben lassen, weil sie aus dem Aufbau kommen.

| # | Schwäche | Woran man sie sieht |
|---|---|---|
| 1 | **Freibeträge erzeugen Verstecke, Verstecke erzeugen Abwehrregeln.** Weil Menschen 5.000 AEQ frei halten und Unternehmen 1,5 Monatsumsätze, muss man verhindern, dass Geld dorthin umzieht. Daraus folgen Umsatzzählung je Kunde und Quartal, Rückzahlungsregel, B2B-Deckel, Gründungsphase, Erstes-Unternehmen-Regel. | 16 Zahlen im Modell (`ModellParameter`), dazu Monats- und Quartalszähler |
| 2 | **Nicht jeder Knoten kann jede Zahl nachrechnen.** Das Liegegeld eines Unternehmens hängt an der Umsatz-Buchführung des Knotens; ein Knoten mit kürzerer Geschichte überspringt die Prüfung (`liegegeld_pruefung.go`, „nur wenn seine Buchführung das ganze Fenster abdeckt“). Das widerspricht AGENTS.md, Frage 4. | `liegegeldUebersprungen` |
| 3 | **Das Grundeinkommen ist klein und zufällig.** Es hängt an Gebühren und an wenigen Menschen über 5.000 AEQ. Geschätzt 1 bis 5 AEQ im Monat, im Modell 1,43 AEQ. | `WIRTSCHAFT_REIFEPRUEFUNG.md` 3.3 |
| 4 | **Liegendes Geld bleibt liegen.** Ein Startguthaben, das nie ausgegeben wird, und Geld hinter verlorenen Schlüsseln trifft keine Regel, bis nach 2,5 Jahren die Treuhand greift. Bei Sarafu lagen 58 % des Geldes still. | `WIRTSCHAFT_LITERATUR.md` 1.2 |
| 5 | **Die Gebühr besteuert genau den Umlauf, den die Haltegebühr fördern soll.** Sie schützt auch nicht vor Spam (auf Kleinstbeträge ist sie null); das tun Ratenbegrenzer und Mindestbetrag. | `ueberweisungsgebuehr.go`, `betrag_untergrenze.go` |

Schwäche 1 ist die Wurzel. Solange irgendwer einen Freibetrag hat, muss die
Kette wissen, wer was ist, und das kann sie für Unternehmen nicht
(`WIRTSCHAFT_ZAHLENPRUEFUNG.md` 2.1). Die bisherigen Dokumente lösen das
mit „das Etikett darf nichts bringen“ und vielen Regeln, die das absichern.
Die Endfassung löst es, indem es **nichts mehr gibt, was das Etikett bringen
könnte**.

---

## 3. Was Forschung und Praxis dazu sagen

Ergänzt `WIRTSCHAFT_LITERATUR.md` um das, was für die Endfassung entscheidet.

| Befund | Was daraus folgt | Quelle |
|---|---|---|
| Gesells Haltegebühr auf Geld ist ökonomisch stimmig; Buiter (2003) zeigt sie als Mittel gegen die Nullzinsgrenze und nennt als Preis vor allem Verwaltungskosten. Keynes hielt die Idee für richtig, warnte aber: Andere liquide Werte treten an die Stelle des belasteten Geldes. | Die Gebühr muss auf **alles** liegen, was wie AEQ wirkt (LP-Anteile zählen schon mit). Dass Menschen Rücklagen in Euro halten, ist erwünscht: AEQ ist Zahlungsmittel. | Buiter, Economic Journal 113 (2003); Keynes, General Theory, Kap. 23 |
| **Haltegebühr plus gleiche Verteilung** ist in digitalen Gemeinschaftswährungen erprobt: Sarafu (2 % im Monat in einen Topf für die Nutzer), Circles (7 % im Jahr, laufende Ausgabe je Mensch), Encointer (Haltegebühr, Grundeinkommen je Teilnahme an Treffen). | Der Vorschlag ist keine Erfindung, sondern die übliche Form, nur mit fester Geldmenge. | Grassroots-Economics-Spezifikation; Circles-Dokumentation; Encointer |
| Circles: 1 CRC je Stunde und Mensch, 7 % Haltegebühr im Jahr; jedes Konto nähert sich einem Höchststand von rund 120.000 CRC. Organisationen schöpfen nichts und zahlen die Gebühr voll. | Das ist mathematisch dasselbe wie der Vorschlag (Abschnitt 4), in einer anderen Einheit. | Circles-Dokumentation |
| Ğ1 (Duniter) bemisst die Dividende an der Geldmenge je Mitglied und begründet die Rate damit, dass jedes Mitglied über seine Lebenszeit denselben Anteil der neuen Geldmenge erhält, egal wann es beitritt. Die Mitgliedschaft muss jährlich erneuert werden. | Gleichheit über die Zeit (früh oder spät dabei) ist ein ausdrückliches Ziel. Aequitas hat mit dem Lebensnachweis dieselbe jährliche Erneuerung. | Duniter-Wiki und -Forum |
| Ackerman und Alstott schlagen ein **einmaliges Startkapital** (rund 80.000 $) vor, bezahlt aus einer jährlichen Vermögensabgabe von 2 %; Van Parijs ein **laufendes Grundeinkommen**. Der Kern der Debatte: Einmalbetrag als Startkapital oder dauerhafte Untergrenze. | Aequitas heute ist das Startkapital-Modell (1.000 AEQ einmal, Abgabe auf Vermögen über 5.000). Der Vorschlag verbindet beides: Startkapital **und** laufende Untergrenze. | Ackerman & Alstott 1999; Redesigning Distribution 2006 |
| Chiemgauer: Betriebe zahlen 8 % im Jahr auf alles, was sie halten, ohne Freibetrag, und machen trotzdem mit, weil sie Rücklagen in Euro halten. | Ein Satz für alle trägt auch für Betriebe, solange AEQ Zweitgeld ist. | `WIRTSCHAFT_LITERATUR.md` 1.1, 1.3 |
| Kartenzahlungen kosten den deutschen Handel bei der girocard (kontaktlos) rund 0,2 % vom Umsatz, internationale Debitkarten, Kreditkarten und PayPal das Zwei- bis Dreifache. | Maßstab für „was darf Annehmen kosten“ (Abschnitt 8.1). | EHI Zahlungssysteme 2025; Handelsblatt 21.08.2025 |
| Freicoin (Bitcoin mit Haltegebühr, rund 5 % im Jahr) fand kaum Annahme. Circles' Berliner Pilot zeigt: Grundeinkommen allein trägt keine Währung. | Die Regel ist nötig, aber nicht hinreichend. Annahme entsteht durch Läden und Lieferketten, nicht durch Formeln. | Bitcoin Magazine (Timón); Papadimitropoulos & Perperidis, Frontiers in Blockchain 2024 |
| Steuern auf Umsätze verringern die Zahl der Umsätze; wie stark, ist umstritten. | Eine Gebühr auf Zahlungen arbeitet gegen das Ziel „Geld soll umlaufen“. | Matheson, IMF WP/11/54 |

---

## 4. Die Kernaussage: „einmal oder laufend“ ist keine Grundsatzfrage mehr

`GRUNDPFEILER.md` (Pfeiler 2) und `WIRTSCHAFT_LITERATUR.md` (5.) nennen als
offene Grundsatzfrage: einmal 1.000 AEQ wie heute oder laufend neues Geld wie
Circles. Die Rechnung zeigt, dass beides **dasselbe** ist, sobald die
Haltegebühr auf jedes Guthaben liegt und alles gleich verteilt wird.

Sei `F` der faire Anteil (1.000 AEQ), `d` der Satz je Monat (0,005) und `N`
die Zahl der Menschen. Die Geldmenge ist `N × F` (`TotalSupply`). Gibt jedes
Konto `d × Guthaben` ab und bekommt jeder Mensch ein gleiches Stück, dann ist
der Topf `d × N × F` und das Stück `d × F`. Für einen Menschen mit Guthaben
`b` gilt je Monat:

```
b_neu = b − d·b + d·F = F + (b − F)·(1 − d)
```

Daraus folgt, ohne jede Annahme über Verhalten:

| Eigenschaft | Formel | bei d = 0,5 % |
|---|---|---|
| Dividende je Mensch | `d × F` | 5 AEQ im Monat, 60 AEQ im Jahr |
| Netto je Mensch | `d × (F − b)` | +3,50 AEQ bei 300, 0 bei 1.000, −20 bei 5.000 |
| Annäherung an den fairen Anteil | `b(n) = F + (b₀ − F)(1 − d)ⁿ` | Abstand halbiert in 138 Monaten (11,5 Jahren) |
| Unternehmen und freie Adressen | `−d × b` | gehen netto ganz an die Menschen |
| Geldmenge | bleibt `N × F` | nichts wird geschöpft oder vernichtet |

Circles schöpft je Mensch 8.760 CRC im Jahr und lässt 7 % im Jahr
schwinden; das Gleichgewicht liegt bei `8.760 / 0,0726 ≈ 120.700 CRC`, die
Dividende ist 7,3 % davon im Jahr. Aequitas mit dem Vorschlag: Dividende 6 %
des Gleichgewichts (1.000 AEQ) im Jahr. **Gleiche Wirkung, nur in einer
Einheit, die nicht wächst.** Darum bleibt alles, was in Aequitas als Vielfaches
des fairen Anteils gerechnet ist (Grenzen, Staffel, Anzeigen), richtig.

Der Unterschied zu heute liegt nur darin, ob die Gebühr auch auf den fairen
Anteil liegt. Heute liegt sie erst über 5.000 AEQ. Dann bleibt das
Grundeinkommen klein, liegendes Geld liegt, und es braucht Freibeträge mit
Abwehrregeln. Liegt sie auf allem, ist der faire Anteil trotzdem geschützt,
nur **netto statt brutto**: Wer 1.000 AEQ hält, gibt 5 ab und bekommt 5.

---

## 5. Die Regeln der Endfassung

| Regel | Wert | heute |
|---|---|---|
| Registrierung | 1.000 AEQ je Mensch (gestaffelt bei „gelb“, siehe `REGISTRIERUNG_ENDFASSUNG.md`) | gleich |
| Umlauf | **0,5 % im Monat auf jedes Guthaben**, sekundengenau anteilig im Tageslauf | Menschen über 5.000 AEQ 0,5 %; Unternehmen nach Umsatz; freie Adressen 1 % |
| Was zählt | Guthaben + Wert der LP-Anteile; Staffelrest nicht (ein Anspruch, kein Guthaben) | gleich |
| Ausgenommen | Protokoll-Töpfe | gleich |
| Grundeinkommen | der ganze Topf täglich zu gleichen Teilen an jeden Menschen ohne überfälligen Lebensnachweis | gleich (nur der Topf ist größer) |
| Überweisung | **keine Gebühr** | 0,1 % über 1.000 AEQ/Monat, Aufschläge ab 5 ×, 10 ×, 20 × |
| Vermögensgrenze Menschen | 25 × fairer Anteil, Phase 0 kleiner; Eingänge darüber abgelehnt, Grundeinkommen gekappt | gleich |
| freie Adressen | höchstens 250 AEQ | gleich |
| Unternehmen | Register, Mitinhaber, Verzeichnis, Kundschaft, Bürgen als **Anzeige**; keine Geldregel eigener Art | dazu Liegegeld nach Umsatz |
| Swap-Gebühr | 0,1 %, 40 / 30 / 30 an Validatoren, Liquidität, Grundeinkommen | gleich |
| Ausstieg in Stable | wie heute, bis der echte Ausgang öffnet (Abschnitt 12) | 2 % über 3.000 AEQ/Monat |
| Treuhand inaktiver Konten | wie heute | gleich |
| Spanne für Mitbestimmung | Satz zwischen 0,25 % und 1 % im Monat, Schritte von 0,05 %, höchstens eine Änderung je Quartal, 30 Tage vorher angekündigt | – |

**Warum 0,5 %:** Es ist der heutige Satz der Menschen, also keine neue Zahl.
Er liegt im erprobten Bereich (Gesell 5,2 %, Circles 7 %, Chiemgauer 8 %,
Wörgl 12 % im Jahr; 0,5 % im Monat sind 5,8 % im Jahr). Für Betriebe kostet er
etwa so viel wie die girocard, wenn das Geld in zwei Wochen weitergeht
(Abschnitt 8.1). Ğ1 würde mit seiner Begründung (gleicher Anteil über ein
Leben) eher einen höheren Satz wählen, Betriebe eher einen niedrigeren. Die
Spanne lässt beides zu, aber nur durch die Menschen.

---

## 6. Was entfällt

| Regel | Konstante | Warum sie nicht mehr gebraucht wird |
|---|---|---|
| Sparfreibetrag 5.000 AEQ | `menschSparFreibetrag` | Netto-Schutz des fairen Anteils ersetzt ihn |
| 1.000 AEQ/Monat gebührenfrei, Gebühr 0,1 %, Aufschläge | `menschFreiAusgabenMonat`, `ueberweisungsGebuehrBps` | keine Gebühr |
| Liegegeld nach Umsatz (Sockel, 1,5 / 3 Monatsumsätze, 0,5 / 1 %) | `unternehmenSockel`, `umsatzFreiFaktor`, `umsatzStufe2Faktor`, `liegeRate1Monat`, `liegeRate2Monat` | ein Satz für alle |
| Umsatzzählung (9.000 AEQ je Kunde und Quartal, Rückzahlung, B2B, 90 / 365 Tage, 30 Tage Mindestdaten) | `menschZaehltJeUntQuartal`, `umsatzFensterTage`, `umsatzJahrTage`, `umsatzMindestTage` | nichts hängt mehr am Umsatz |
| Gründungsphase, Erstes-Unternehmen-Regel | `gruendungTage`, `gruendungAbstandTage`, `liegegeldLocked` | „nie schlechter als ein Mensch“ gilt von selbst: gleicher Satz |
| 1 % für freie Adressen | `freiUmlaufMonat` | kein Versteck mehr, gegen das es schützen müsste |

Aus 16 Zahlen im Modell werden 6 (fairer Anteil, Satz, Grenze Menschen,
Grenze freie Adressen, Ausstiegsabgabe, Ausstiegs-Freibetrag). Die
Buchführung des Erzeugers für Umsätze und Monatszähler wird für neue Blöcke
nicht mehr gebraucht. Alte Blöcke spielen weiter nach den alten Regeln nach.

**Was als Anzeige bleibt:** Kundschaft in 90 Tagen, Bürgen, Verzeichnis,
Website-Nachweis (`WIRTSCHAFT_REIFEPRUEFUNG.md` 2.2). Sie zeigen Kundinnen,
wem sie trauen können. Mit Geld haben sie nichts mehr zu tun, und genau das
war die Regel „keine Schicht bringt Geld“.

---

## 7. Zahlen aus dem Modell

`go run ./cmd/wirtschaftsmodell -vorschlag`: dieselben Akteure und Zahlungen
wie in `WIRTSCHAFTSMODELL.md`, 12 Monate, dazu zwei Menschen, an denen sich
die Regeln am deutlichsten unterscheiden. Der Ausstieg ist in beiden Spalten
nach der heutigen Regel gerechnet, damit nur die Umlaufregel sich
unterscheidet.

| Akteur | Art | Start | Ende heute | Ende Vorschlag | Abgaben/Monat heute | Abgaben/Monat Vorschlag | Dividende/Monat | Netto/Monat Vorschlag |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| Mensch mit wenig | mensch | 300 | 540 | 573 | 0.00 | 2.23 | 5.00 | +2.77 |
| Mensch mit viel | mensch | 30000 | 20711 | 20510 | 474.12 | 495.84 | 5.00 | -490.84 |
| Baeckerei | unternehmen | 4000 | 6328 | 6090 | 6.00 | 25.84 | – | -25.84 |
| Supermarkt | unternehmen | 40000 | 51460 | 49282 | 45.00 | 226.50 | – | -226.50 |
| Grosshaendler | unternehmen | 120000 | 141840 | 136229 | 180.00 | 647.60 | – | -647.60 |
| Huelle | unternehmen | 50000 | 44546 | 47081 | 454.46 | 243.24 | – | -243.24 |
| Absprache unter Freunden | unternehmen | 30000 | 26819 | 28249 | 265.10 | 145.94 | – | -145.94 |
| Freie Adresse (Kasse) | frei | 200 | 176 | 188 | 2.03 | 0.97 | – | -0.97 |
| Mensch, Startguthaben liegt | mensch | 1000 | 1000 | 1000 | 0.00 | 5.00 | 5.00 | +0.00 |
| Verlorener Schluessel | mensch | 1000 | 1000 | 942 | 0.00 | 4.86 | 0.00 | -4.86 |

Heute bringen diese Akteure 1,43 AEQ je Mensch und Monat (bei 1.000 Menschen).
Im Vorschlag sind es rund 5,00 AEQ, aus der ganzen Geldmenge. Mit 90 %
aktiven Menschen (`-aktiv 0.9`) sind es 5,56 AEQ.

**Was Annehmen einen Betrieb kostet**, gemessen am Monatsumsatz (9.000 AEQ,
zweites Unternehmen ohne die Regel für das erste):

| Geld liegt im Mittel | heute | Vorschlag |
|---|---:|---:|
| ¼ Monatsumsatz (1 Woche) | 0 % | 0,125 % |
| ½ Monatsumsatz (2 Wochen) | 0 % | 0,25 % |
| 1 Monatsumsatz | 0 % | 0,50 % |
| 1,5 Monatsumsätze | 0 % | 0,75 % |
| 3 Monatsumsätze | 0,75 % | 1,50 % |
| 6 Monatsumsätze | 3,75 % | 3,00 % |

**Ein Mensch, der nichts tut** (Lebensnachweis gültig):

| Start | nach 1 J. | nach 5 J. | nach 10 J. | nach 20 J. |
|---:|---:|---:|---:|---:|
| 0 | 58 | 260 | 452 | 700 |
| 300 | 341 | 482 | 616 | 790 |
| 1000 | 1000 | 1000 | 1000 | 1000 |
| 5000 | 4766 | 3961 | 3192 | 2201 |
| 25000 | 23599 | 18766 | 14152 | 8207 |

Die Tests halten fest, was unabhängig von den Annahmen gilt: Wer genau den
fairen Anteil hält, zahlt netto null; wer weniger hält, bekommt dazu; die
Dividende hängt bei jeder Verteilung des Geldes nicht davon ab, wer wie viel
hält; **Geld zwischen Mensch, eigenen Unternehmen und freier Adresse zu
verschieben, senkt die Abgabe nie** (2.000 zufällige Aufteilungen); Geld
hinter einem verlorenen Schlüssel fließt zurück.

---

## 8. Was der Vorschlag kostet

### 8.1 Betriebe zahlen mehr als heute

Das ist der Preis, und er ist nicht klein. Heute hält ein Betrieb bis zu 1,5
Monatsumsätze frei. Im Vorschlag zahlt er auf alles: Bäckerei 26 statt 6 AEQ im
Monat, Supermarkt 227 statt 45, Großhändler 648 statt 180. Gemessen am Umsatz
sind das 0,29 %, 0,38 % und 0,32 %, also etwa so viel wie eine Kartenzahlung
(girocard 0,2 %, andere Karten das Zwei- bis Dreifache).

Warum ich es trotzdem empfehle:
- **Jeder Freibetrag wird zum Versteck**, sobald Menschen ab dem ersten AEQ
  zahlen. Hätte jedes Unternehmen nur den Sockel von 2.000 AEQ frei, könnte ein
  Mensch mit drei Unternehmen 6.000 AEQ unbelastet halten und bekäme die
  Dividende obendrauf. Das Modell bricht. Ein Freibetrag nur für Betriebe geht
  also nicht ohne die ganze Abwehr-Buchführung, die wegfallen soll.
- **Weitergeben senkt die Kosten.** Ein Betrieb, der AEQ in zwei Wochen an
  Lieferanten und Löhne weitergibt, zahlt 0,25 % vom Umsatz. Das ist das
  Verhalten, von dem der Kreislauf lebt (Bristol scheiterte an fehlender
  Weitergabe).
- **Der Chiemgauer** zeigt, dass Betriebe einen Satz auf alles tragen, wenn
  AEQ Zweitgeld ist und Rücklagen in Euro liegen. Unser Satz ist niedriger.
- **In der Beta gibt es keine Unternehmen** (Konzept 0a). Die Regel trifft
  heute niemanden; sie muss sich im Pilot bewähren.

**Wenn der Pilot zeigt, dass Betriebe deshalb nicht mitmachen,** ist die
Antwort ein niedrigerer Satz für alle (0,25 %: Dividende 2,50 AEQ, Betrieb mit
zwei Wochen 0,125 %), nie ein Freibetrag für eine Kontoart.

### 8.2 Die Hülle zahlt weniger als heute

Eine Firma ohne Umsatz mit 50.000 AEQ zahlt 243 statt 454 AEQ im Monat. Das ist
kein Versteck: Ein Mensch zahlt auf dasselbe Geld denselben Satz. Sie umgeht
aber die **Vermögensgrenze** (Audit 21.10.2026, M3: „eine Grenze für
Unternehmen wäre eine Wirtschaftsentscheidung“): Ein Mensch kann über eigene Unternehmen mehr als
25 × halten, und das kostet ihn nicht mehr als 5,8 % im Jahr (heute rund 11 %).
Ich halte das für hinnehmbar: Stimmen hängen an Menschen, nicht an AEQ;
Unternehmen sind öffentlich mit ihren Verantwortlichen eingetragen; und eine
Hülle zeigt im Verzeichnis „0 Kundinnen und Kunden in 90 Tagen“. Wer die Grenze
auch über Unternehmen durchsetzen will, braucht eine Zurechnung der Firmen zu
Menschen. Das ist eine eigene Entscheidung (Abschnitt 13, Nr. 4).

### 8.3 Eine Identität wird mehr wert

Heute bringt eine gekaufte oder gefälschte Identität 1.000 AEQ. Im Vorschlag
kommen 60 AEQ im Jahr dazu, solange der Lebensnachweis gelingt. Der verlangt
**jedes Jahr dasselbe Gesicht** (`lebensnachweis.go`). Ein Käufer braucht dafür
jedes Jahr den Verkäufer, eine Fälscherfarm jedes Jahr eine neue gelungene
Täuschung. Das ist genau die Gegenmaßnahme, die World nach dem Handel mit
World-IDs eingeführt hat (Gesichtsabgleich, Zurückholen durch erneuten
Scan). **Darum gehört der Lebensnachweis vor echtem Geld scharf geschaltet**
(Abschnitt 10, Voraussetzung 2).

### 8.4 Wer pausiert ist, zahlt und bekommt nichts

Ein Mensch mit überfälligem Lebensnachweis (Frist plus 30 Tage) zahlt wie
jedes Konto, bekommt aber keine Dividende: −5 AEQ im Monat bei 1.000 AEQ. Das
ist Absicht. So kommt Geld hinter verlorenen Schlüsseln und von Verstorbenen
zu den Lebenden zurück, langsam (5,8 % im Jahr) und ohne dass jemand etwas
entscheiden muss. Nach einem gültigen Nachweis ist er ab der nächsten Runde
wieder dabei. Die App muss das vorher deutlich sagen.

### 8.5 Sichtbar schwindendes Guthaben

Viele Menschen sehen ungern, dass ihr Guthaben schwindet, auch wenn es netto
nicht sinkt. Circles hat damit gekämpft. Gegenmittel: In der App steht nicht
„Haltegebühr −5“, sondern **„Diesen Monat: +3,50 AEQ“** (Dividende minus
Umlauf), mit einer Zeile Erklärung.

---

## 9. Meine Abwägung

Ich empfehle die Endfassung, aus fünf Gründen, in dieser Reihenfolge:

1. **Sie macht das Versprechen wahr statt ungefähr wahr.** „Jeder Mensch hat
   denselben Anteil“ gilt heute nur am Tag der Registrierung. Mit der
   Endfassung strebt jedes Guthaben dorthin zurück, egal wann jemand
   dazukam (die Gleichheit über die Zeit, die Ğ1 ausdrücklich anstrebt).
2. **Sie löst das schwerste offene Problem, statt es zu verwalten.**
   Unternehmen lassen sich auf der Kette nicht erkennen. Die heutigen Regeln
   sichern mit viel Aufwand ab, dass das Etikett nichts bringt. Die
   Endfassung macht das Etikett für Geld bedeutungslos. Was nicht geprüft
   werden muss, kann nicht falsch geprüft werden.
3. **Jeder Knoten kann jede Zahl nachrechnen.** Der Umlauf hängt nur noch am
   Guthaben zur Rundenzeit und an der Zeit seit der letzten Runde. Das ist der
   Launch-Maßstab (AGENTS.md, Frage 4) und ab dem strengen Nachrechnen
   (01.11.2026) eine Sicherheitsfrage.
4. **Sie ist in einem Satz erklärbar.** Die heutigen Regeln brauchen eine
   Tabelle mit Fußnoten.
5. **Das Grundeinkommen wird verlässlich.** 5 AEQ im Monat sind wenig, aber
   sie sind eine feste Zahl und kein Zufall aus Gebühren.

Was ich **nicht** weiß und im Pilot gemessen werden muss: ob Betriebe 0,25 bis
0,4 % ihres AEQ-Umsatzes tragen (8.1); ob Menschen „netto“ verstehen (8.5);
ob 0,5 % der richtige Satz ist. Was ich **nicht** tun würde: einen Freibetrag
für irgendwen wieder einführen, den Satz nach Kontoart unterscheiden, eine
Zahl an einen Kurs koppeln.

Und ehrlich zur Einordnung: **Keine Regel schafft Nachfrage.** Freicoin hatte
eine Haltegebühr und keine Läden; Circles hatte ein Grundeinkommen und
scheiterte in Berlin an der Annahme. Die Endfassung macht AEQ fairer und
einfacher. Ob es angenommen wird, entscheiden die Pilotstadt, die Läden mit
ihren Lieferanten und die Gewissheit, dass jeder Mensch genau einmal da ist.
Die Identität bleibt der Engpass (`REGISTRIERUNG_ENDFASSUNG.md`).

---

## 10. Umsetzung in Aequitas

### Voraussetzungen

1. **Strenges Nachrechnen läuft sauber.** `nachrechnenStrengAbUnix` ist der
   01.11.2026. Die Umstellung frühestens vier Wochen danach, wenn
   `/api/wirtschaft/regeln` keine Abweichung zeigt. Zwei Konsensänderungen
   gleichzeitig sind schwer auseinanderzuhalten.
2. **Vor echtem Wert: Lebensnachweis aktiv.** Er wirkt erst, wenn
   Coordinatoren im Konsens zugelassen sind (`registerLeserAktiv`,
   `lebensnachweisAktiv`). Für Testgeld in der Beta nicht nötig.

### Verträglichkeit mit den Verifier-Prinzipien

Der Umlauf nimmt Geld ohne Unterschrift. Grundsatz 1 in
`VERIFIER_PRINZIPIEN.md` erlaubt das nur als Regel, „die jeder Knoten selbst
nachrechnet und streng durchsetzt“. Heute gilt das für Menschen und freie
Adressen; Unternehmen überspringt die Prüfung, wenn die Buchführung des
Knotens nicht das ganze Fenster abdeckt. Mit der Endfassung gilt es für jedes
Konto. `umlauf` bleibt in `bekannteGeldAusnahmen` (`kein_entzug_test.go`),
mit dieser Begründung.

### Schritt 1: Konsens (eine Aktivierungszeit)

**Stand 25.10.2026: gebaut, schlafend.** a–e und h sind in der Kette,
g ist unverändert, i liefert `/api/wirtschaft/regeln` → `gleichgewicht`
(Satz, Dividende je Mensch, Zähler der Vollständigkeitsprüfung); die App
zeigt es noch nicht. f ist bewusst offen: die Buchführung läuft weiter,
keine Regel des Gleichgewichts liest sie. Tests:
`x/humanity/keeper/wirtschaft3_test.go` (echte Runden über zwei Knoten, ein
und zwei Blöcke, streng; je Angriff ein Missbrauchstest; Datenbank und
Speicher als Kandidatenquelle gegen Postgres). Prüfung des Diffs:
`docs/SICHERHEITSPRUEFUNG_2026-10-25.md`, Abschnitt 2.

Neue Konstante `gleichgewichtAktivAbUnix` in einer neuen Datei
`x/humanity/keeper/wirtschaft3.go`, wie `wirtschaft2.go`: zuerst auf
`4102444800` (2100, schlafend), das echte Datum nach eurer Entscheidung und
nach dem Ausrollen auf alle Knoten. Alles hängt an der **Blockzeit** der
Runde, nie an der Uhr des Knotens.

| # | Datei / Funktion | Änderung ab Aktivierung |
|---|---|---|
| a | `wirtschaft.go` `umlaufBetragRoh` | `round6(stand × gleichgewichtSatzMonat × anteil)` für Mensch, Unternehmen und freie Adresse; Systemadresse 0. `gleichgewichtSatzMonat = 0.005` |
| b | `wirtschaft.go` `umlaufLocked` | Kandidaten: jedes Konto mit `balance > 0` oder `lp_shares > 0`, Menschen nicht erst ab 5.000 AEQ |
| c | `liegegeld_pruefung.go` `pruefeUmlaufLocked` | kein Überspringen mehr: jeder Knoten rechnet jeden Betrag nach, auch für Unternehmen (keine Buchführung nötig) |
| d | neu: Vollständigkeit der Runde | beim Abschluss einer Umlaufrunde prüft jeder Knoten, dass **jedes** Konto mit einer Schuld ≥ `umlaufMindestBetrag` genau einmal belastet wurde, aus dem eigenen Zustand zur Rundenzeit (wie `ubi_empfaenger` in `nachrechnen_ubi.go`). Sonst könnte ein Erzeuger Freunde auslassen |
| e | `ueberweisungsgebuehr.go`, `wirtschaft.go` `gebuehrMitWirtschaft` | Gebühr 0. Nachrechnen: `Transaction.Gebuehr > 0` in einem Block nach der Aktivierung ist ein Regelbruch (`gebuehr_nach_gleichgewicht`, wie `demurrage_nach_umstellung`) |
| f | `wirtschaft.go` Buchführung (`buchUeberweisungLocked`, Quartalszähler) | nur noch für Blöcke vor der Aktivierung; `kundschaft.go` bleibt (Anzeige) |
| g | `distributeUBIPoolLocked`, `nachrechnen_ubi.go` | unverändert: der ganze Topf an die nicht pausierten Menschen |
| h | `wirtschaftsmodell.go` | `TestWirtschaftsmodell_FormelnGleichDerKette` um den neuen Zweig erweitern: Modell und Kette gleich |
| i | `/api/wirtschaft/regeln`, App | Satz, Dividende der letzten 30 Tage, **Netto je Konto** im Monat |

**Grenzen (AGENTS.md, Frage 2).** Heute läuft der Umlauf nur über Konten über
5.000 AEQ und Nicht-Menschen; künftig über jedes Konto mit Guthaben, also
O(Konten) je Tag unter `cs.mu`. Das ist bei der heutigen Größe unkritisch, ab
etwa 10⁵ Konten nicht. Vorher den Umlauf wie die Verteilung in Abschnitte
teilen (`ubi_chunked.go`) oder Schritt 2 bauen. `umlaufMindestBetrag`
(0,001 AEQ) bleibt: Konten unter rund 6 AEQ werden nicht belastet, zugunsten
der Inhaber.

**Missbrauchstests (AGENTS.md, Frage 5), je einer:**
- Erzeuger setzt einen höheren Umlauf an → Block abgelehnt.
- Erzeuger lässt ein Konto aus → Block abgelehnt (d).
- Block trägt eine Überweisungsgebühr → abgelehnt (e).
- Geld in Unternehmen oder freie Adressen verschoben → Summe der Abgaben
  unverändert (wie `TestGleichgewicht_VerschiebenBringtNichts`, gegen die
  Kette).
- Rundenzeit vordatiert oder sieben Tage zurück → wie heute begrenzt
  (`rundenZeitNachBlock`, höchstens 7 Tage je Runde).
- Einzug, der LP-Anteile auflösen müsste, mit überhöhtem Betrag → immer
  streng abgelehnt (besteht, `umlauf_lp.go`).
- Blöcke vor der Aktivierung spielen unverändert nach (StateRoot gleich).

**Ankündigung:** 30 Tage vorher auf Website und in der App, mit dem Satz aus
Abschnitt 1 und dem eigenen Netto-Betrag.

**Texte:** WHITEPAPER 4.2, 4.3, 4.5; `UNTERNEHMEN_KONZEPT.md` als Fassung 5;
Website (12 Sprachen); App. Weicht der Code ab, gilt der Code.

**Sicherheitsprüfung:** getrennter Durchgang über den Diff, Ergebnis als
PR-Kommentar.

### Schritt 2: ohne tägliche Buchungen (für viele Menschen)

Weil die Regel eine geschlossene Form hat, braucht sie keine tägliche Buchung
je Konto. Ein aktiver Mensch mit Stand `b₀` zur Zeit `t₀` hat zur Zeit `t`:

```
b(t) = D + (b₀ − D)·(1 − d)^((t − t₀)/Monat),   D = Dividende/d
```

Jedes andere Konto: `b(t) = b₀·(1 − d)^((t − t₀)/Monat)`. Abgerechnet wird
beim nächsten Zugriff, mit der Blockzeit, auf jedem Knoten gleich (wie Circles
v2 die Haltegebühr erst beim Lesen anwendet). Der Rest des Topfs, der durch
pausierte Menschen und Protokoll-Töpfe entsteht, geht in einer kleinen
Restrunde an alle. Pausen sind aus `LebensnachweisAm` bekannt, also stückweise
in derselben Formel.

Das **löst die offene Frage aus `UBI_DISTRIBUTION_DESIGN.md`**: Dort scheiterte
das spätere Abholen daran, dass nicht abgeholtes Grundeinkommen keiner
Haltegebühr unterlag. In der geschlossenen Form ist beides in einer Rechnung;
nicht abholen bringt nichts. Schritt 2 ist eine eigene Konsensänderung und
kommt erst, wenn Schritt 1 gemessen ist.

---

## 11. Was die Endfassung für Kredit heißt

Mit einer Haltegebühr auf jedem Guthaben wird **zinsfreies Verleihen für
jeden vernünftig**, nicht nur für Wohlhabende: Wer 500 AEQ verleiht und
zurückbekommt, hat 2,50 AEQ im Monat gespart. Das ist Gesells Vorhersage und
das Prinzip der JAK-Bank. Die Stufen aus `WIRTSCHAFT_REIFEPRUEFUNG.md` 4.3
bleiben: Zahlungsziel im Pilot; Darlehen zwischen Menschen nach
Rechtsprüfung; gegenseitiger Kredit zur Abstimmung. Neu ist nur, wer den
Umlauf auf verliehenes Geld zahlt: die Schuldnerin, solange sie es hält; ein
Anspruch ist kein Guthaben. Für die Vermögensgrenze zählt Verliehenes beim
Verleiher weiter mit (Reifeprüfung 4.3). Ein Kreditvertrag darf kein Konto
außerhalb der Regel werden (dieselbe Lehre wie bei der Liquidität,
`WHO_MAY_HOLD_AEQ.md`).

---

## 12. Was bleibt wie beschlossen

- **Ausstieg:** bis der echte Euro-Ausgang öffnet, wie heute. Dann mein
  Vorschlag: 2 % für alle ohne Monatsfreibetrag, eigene Einlage frei.
  Begründung: Der Freibetrag braucht Monatszähler und lädt dazu ein, über
  Monate zu verteilen; Kleine sind durch die Dividende schon geschützt.
  Zu entscheiden, wenn der Ausgang kommt.
- **Betrieb und Finanzierung (Pfeiler 9):** Solange AEQ Testgeld ist, kein
  Anteil aus dem Topf. Den Pilot über Förderung von außen tragen
  (Förderprogramme für offene Software, Stiftungen) mit festem Ende. Vor
  echtem Wert über einen offen verbuchten, gedeckelten Anteil für
  Infrastruktur abstimmen lassen. Ohne Quelle wiederholt sich Bristol.
- **Vermögensgrenze:** wie heute, auch Phase 0.
- **Unternehmensregister, Verzeichnis, Bürgen:** wie heute, als Anzeige.

---

## 13. Zu entscheiden

1. **Endfassung annehmen:** ein Satz von 0,5 % auf jedes Guthaben, alles an
   alle Menschen, keine Überweisungsgebühr. Empfehlung: **ja**.
2. **Zeitpunkt:** Schritt 1 schlafend bauen, scharf frühestens vier Wochen
   nach dem 01.11.2026. Empfehlung: **ja**.
3. **Spanne für die Mitbestimmung:** 0,25 bis 1 % im Monat. Empfehlung: **ja**.
4. **Vermögensgrenze auch über eigene Unternehmen durchsetzen** (8.2).
   Empfehlung: **nein**, vorerst Anzeige im Verzeichnis.
5. **Lebensnachweis vor echtem Wert aktiv** (8.3). Empfehlung: **ja**.

## Quellen

Neu in diesem Dokument (über Suchergebnisse und Zusammenfassungen ausgewertet):
- Circles, Personal Currencies: https://docs.aboutcircles.com/overview/how-it-works/personal-currency ; Daily Burn: https://docs.aboutcircles.com/user-guides/circles-features/daily-burn-demurrage
- Grassroots Economics, Sarafu Holding Fee (Spezifikation 001): https://git.grassecon.net/grassrootseconomics/cic-docs/src/branch/master/spec/001_platform_incentives.md
- Encointer: https://hackernoon.com/encointer-a-blockchain-experiment-in-local-currencies-and-universal-basic-income
- Duniter, Web of Trust: https://duniter.org/wiki/web-of-trust/ ; Dividende: https://forum.duniter.org/t/researching-cryptoubi-variants-making-sure-i-understand-duniter/2655
- Buiter (2003), Overcoming the zero bound … Gesell's solution, Economic Journal 113(490): https://eprints.lse.ac.uk/848
- Keynes, General Theory, Kap. 23: https://hetwebsite.net/het/texts/keynes/gt/gtindexlz.htm
- Ackerman & Alstott, Stakeholder Society: https://www.irp.wisc.edu/publications/focus/pdfs/foc242b.pdf ; Redesigning Distribution: https://havenswrightcenter.wisc.edu/other-programs-activities/the-real-utopias-project/2002-redesigning-redistribution/
- Papadimitropoulos & Perperidis, Circles UBI, Frontiers in Blockchain 2024: https://doi.org/10.3389/fbloc.2024.1362939
- EHI, Zahlungssysteme 2025: https://www.ehi.org/wp-content/uploads/Downloads/Leseproben/2025_EHI-Studie_Zahlungssysteme_Leseprobe.pdf ; Handelsblatt 21.08.2025: https://secupay.com/wp-content/uploads/sites/4/2025/09/Handelsblatt-21.08.2025-Seite-27.pdf
- Freicoin, Gespräch mit Jorge Timón: https://bitcoinmagazine.com/articles/interview-with-jorge-timon-leader-of-freicoin-project-1385157216
- Matheson, Taxing Financial Transactions, IMF WP/11/54: https://www.imf.org/en/publications/wp/issues/2016/12/31/taxing-financial-transactions-issues-and-evidence-24702
- World, Maßnahmen gegen den ID-Handel: https://www.theblock.co/post/231433/worldcoin-iris-black-market ; Face Auth: https://thedefiant.io/worldcoin-pilots-face-auth-account-verification-to-prevent-fraud

Alle weiteren (Godschalk, Chiemgauer, Wörgl, Fisher, Sarafu-Daten, Bristol,
WIR, Sardex, JAK): `WIRTSCHAFT_LITERATUR.md` und `GRUNDPFEILER.md`.
