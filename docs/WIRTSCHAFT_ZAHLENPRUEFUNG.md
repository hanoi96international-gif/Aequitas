# Wirtschaft: Zahlenprüfung

> **Korrektur nach Literaturauswertung (`WIRTSCHAFT_LITERATUR.md`):**
> Abschnitt 3.2 überschätzt das Problem. Läuft AEQ um wie der Chiemgauer
> (4 bis 6 Mal im Jahr), liegen die Monatsgrenzen im plausiblen Bereich; die
> Rechnung mit 25 Monaten galt für Ersparnisse, nicht für ein Zahlungsmittel.
> Abschnitt 3.3 gilt erst, wenn AEQ die Hauptwährung eines Betriebs ist:
> Chiemgauer-Firmen zahlen 8 % im Jahr auf alles und machen mit, weil sie
> Rücklagen in Euro halten. Der Vorschlag 3 / 6 ist darum nicht dringend.

Stand 02.10.2026. Ehrliche Antwort auf drei Fragen: Wie erkennen wir ein
Unternehmen? Sind die Regeln fair? Sind die Zahlen durchdacht?
**Analyse und Vorschläge, nichts davon gebaut.**

## 1. Urteil

- **Die Grundsätze sind richtig:** gleiche Ausgabe je Mensch, kein Geld für
  Unternehmen, Horten kostet, alles geht an alle.
- **Die Zahlen sind nicht hergeleitet.** Die meisten sind plausible Setzungen,
  verglichen mit Chiemgauer und Wörgl, aber nicht aus einem Modell oder aus
  Daten abgeleitet. Bei der Prüfung zeigen sich zwei Fehler im Aufbau
  (Abschnitt 3.2 und 3.3), nicht nur in einzelnen Werten.
- **Ein Unternehmen lässt sich auf der Kette nicht als Unternehmen erkennen,
  nur an seinem Verhalten.** Das Regelwerk muss deshalb so gebaut sein, dass
  das Etikett „Unternehmen“ nichts bringt, was man nicht durch Verhalten
  verdient (Abschnitt 2).

## 2. Wie erkennen wir ein Unternehmen?

### 2.1 Was die Kette kann und was nicht

Die Kette sieht Adressen, Unterschriften und Zahlungen. Sie sieht keinen
Gewerbeschein, keinen Laden, keine Steuernummer. **„Ist das ein
Unternehmen?“ ist auf der Kette nicht entscheidbar.** Wer etwas anderes
verspricht, braucht eine Stelle, die prüft, und die gibt es in einem
dezentralen Netz nicht.

Entscheidbar ist dagegen: **Wer zahlt an dieses Konto, wie viel, wie oft?**
Und weil jeder Mensch bei Aequitas nur einmal existiert: **Wie viele
verschiedene echte Menschen?**

### 2.2 Drei Arten von Nachweis

| Art | Was es zeigt | Fälschen kostet | Stand |
|---|---|---|---|
| **Verhalten** | verschiedene verifizierte Menschen, die dort bezahlt haben (`kundschaft_90_tage`) | je gefälschtem Kunden ein echter, einmaliger Mensch | gebaut (Anzeige) |
| **Bürgen** | verifizierte Menschen bestätigen: „Diesen Betrieb gibt es, ich war dort.“ Je Mensch wenige Bürgschaften im Jahr, öffentlich, keine Verantwortlichen und Angestellten | je Bürgschaft ein echter Mensch, der seinen Ruf einsetzt | Entwurf (2.3) |
| **Urkunde** | digitaler Firmennachweis aus einem Register (vLEI weltweit, EU-Business-Wallet in Vorbereitung) | eine echte Eintragung | Entwurf, nur dort, wo es Register gibt |

Keine Art allein reicht. Zusammen ergeben sie ein Bild, das jede App selbst
prüfen kann: „bei Eröffnung von einer verifizierten Person eingetragen; 47
verschiedene Kundinnen und Kunden in 90 Tagen; 5 Bürgen; vLEI vorhanden.“

### 2.3 Bürgen, genauer

- Bürgen kann nur ein verifizierter Mensch, der dort selbst bezahlt hat und
  weder verantwortlich noch angestellt ist.
- Höchstens 5 Bürgschaften je Mensch und Jahr. Damit ist ein Bürge knapp und
  etwas wert.
- Bürgschaften sind öffentlich und tragen das Datum. Wer für viele Firmen
  bürgt, die nie Kundschaft haben, fällt auf.
- Eine Bürgschaft bringt der Firma kein Geld. Sie ist Anzeige.
- Ehrlich: Freunde können füreinander bürgen. Bürgen ist ein Hinweis, kein
  Beweis. Der Beweis ist Kundschaft über die Zeit.

### 2.4 Die Regel, die das Problem auflöst

> **Das Etikett „Unternehmen“ darf nichts bringen, was man nicht durch
> nachgewiesenes Verhalten verdient.**

Heute bringt es drei Dinge, und alle sind daran gebunden:
- keine feste Grenze, aber Liegegeld, solange kein Umsatz da ist (so teuer wie
  freie Adressen);
- Spielraum nach Umsatz, und Umsatz zählt nur von verschiedenen echten
  Menschen und gedeckelt;
- gebührenfreie Löhne: Die Ersparnis ist klein, denn die Gebühr zahlen Menschen
  beim Einkauf ohnehin.

Darum ist es nicht schlimm, dass wir Unternehmen nicht erkennen. Gefährlich
würde es nur, wenn das Etikett allein einen Vorteil brächte.

### 2.5 Recht

Wo echtes Geld ein- und ausgeht, müssen Unternehmen nach Geldwäscheregeln
identifiziert werden. Das leistet nicht das Netz, sondern die Stelle, an der
Euro hinein- und herausgehen (Stablecoin-Ausgeber, Börse). Ob das reicht,
klärt die Kanzlei (`RECHTSFRAGEN_UNTERNEHMEN.md`, B).

## 3. Sind die Zahlen durchdacht?

### 3.1 Drei Arten von Zahlen

| Art | Beispiele | Hergeleitet? |
|---|---|---|
| **Bestand im Verhältnis zum Durchschnitt** | Grenze 25 ×, Sparfreibetrag 5 ×, freie Adresse 0,25 ×, Sockel 2 × | **ja, im Prinzip:** Der Durchschnitt hält immer 1 ×, die Zahlen sagen „so viel mal der Durchschnitt“ |
| **Fluss je Monat** | 1.000 AEQ gebührenfrei, 3.000 AEQ Tausch frei, 9.000 AEQ je Kunde und Quartal | **nein** (3.2) |
| **Liegegeld für Unternehmen** | frei bis 1,5 Monatsumsätze, 0,5 % bis 3, dann 1 % | **nein**, und ungerecht für kleine Margen (3.3) |

### 3.2 Fehler im Aufbau: Bestand und Fluss werden vermischt

Die Monatsgrenzen sind als Vielfache des fairen Anteils festgelegt, also eines
**Bestands**. Sie begrenzen aber **Flüsse** pro Monat. Das passt nur, wenn ein
fairer Anteil ungefähr einem Monat Ausgaben entspricht. Ob das so ist, weiß
niemand; es hängt davon ab, wie schnell AEQ umläuft.

| Annahme | Monatskonsum je Mensch | 1.000 AEQ gebührenfrei sind | Kundendeckel 3.000/Monat sind |
|---|---|---|---|
| wie beim Euro heute (Bestand ≈ 25 Monate Konsum, Abschnitt 9 des Konzepts) | 40 AEQ | **25 Monate** Konsum | **75 Monate** Konsum |
| 1 × ≈ 3 Monate Konsum | 333 AEQ | 3 Monate | 9 Monate |
| 1 × ≈ 1 Monat Konsum | 1.000 AEQ | 1 Monat | 3 Monate |

Wenn AEQ sich verhält wie Geld heute, sind die Monatsgrenzen **25-mal zu
großzügig**: Gebühren und Abgaben greifen fast nie, das Grundeinkommen bleibt
nahe null, und der Kundendeckel gegen aufgeblähten Umsatz deckelt nichts.
Wenn AEQ sehr schnell umläuft, passen sie. **Die Zahlen setzen eine
Umlaufgeschwindigkeit voraus, die niemand gemessen hat.**

*Was richtig wäre:* Flussgrenzen an den **gemessenen** Median der
Monatsausgaben binden (die Idee steht schon im Konzept 6.7, war aber erst nach
dem Pilot vorgesehen). Bis dahin sind die heutigen Werte Platzhalter. In der
Beta mit Testgeld ist das unschädlich; vor echtem Geld nicht.

### 3.3 Fehler im Aufbau: Liegegeld nach Umsatz trifft kleine Margen hart

Liegegeld wächst mit dem Umsatz, der Gewinn aber mit der Marge. Was kostet eine
Rücklage einen Betrieb, gemessen an seinem **Monatsgewinn**?

**Heutige Regel** (frei 1,5 Monatsumsätze, 0,5 % bis 3, 1 % darüber):

| Rücklage | Marge 2 % (Supermarkt) | 5 % (Bäckerei) | 10 % (Café) | 30 % (Beratung) |
|---|---|---|---|---|
| 1 Monat | 0 % | 0 % | 0 % | 0 % |
| 2 Monate | 12 % | 5 % | 2 % | 1 % |
| **3 Monate** | **38 %** | **15 %** | 8 % | 2 % |
| 4 Monate | 88 % | 35 % | 18 % | 6 % |
| 6 Monate | 188 % | 75 % | 37 % | 12 % |

Drei Monate Fixkosten Rücklage gelten als übliche Vorsicht (Konzept 14.8). Nach
der heutigen Regel kostet das einen Supermarkt **mehr als ein Drittel seines
Gewinns**, eine Bäckerei 15 %, eine Beratung 2 %. Getroffen werden genau die
Betriebe, mit denen der Pilot beginnen soll (Bäckerei, Mühle, Hof, Café). Sie
würden ihre Rücklage in Euro halten und AEQ sofort tauschen: der Abfluss, den
die Regeln verhindern sollen.

**Vorschlag** (frei 3 Monatsumsätze, 0,5 % bis 6, 1 % darüber):

| Rücklage | 2 % | 5 % | 10 % | 30 % |
|---|---|---|---|---|
| bis 3 Monate | 0 % | 0 % | 0 % | 0 % |
| 4 Monate | 25 % | 10 % | 5 % | 2 % |
| 6 Monate | 75 % | 30 % | 15 % | 5 % |
| 8 Monate | 175 % | 70 % | 35 % | 12 % |

Normale Vorsicht kostet nichts mehr; wer mehr als ein halbes Jahr Umsatz
liegen lässt, zahlt deutlich. Der Unterschied nach Marge bleibt (das hat jede
Haltegebühr), trifft aber erst Rücklagen, die über das Übliche hinausgehen.

*Preis:* Gefälschter Umsatz bringt doppelt so viel Spielraum (3 statt 1,5
Monate je gefälschtem AEQ). Die Grenzen dagegen bleiben (9.000 AEQ je Mensch
bzw. zahlender Firma und Quartal, ein echter Mensch je Kunde); die mögliche
Ersparnis je eingespanntem Menschen steigt von etwa 45 auf etwa 90 AEQ im
Monat. Eine Hülle ohne Umsatz zahlt unverändert 1 % über 2.000 AEQ.

### 3.4 Ungleichgewicht zwischen Menschen und Unternehmen

Ein Mensch hält **5 × den Durchschnitt** frei. Ein Unternehmen hält
**1,5 Monatsumsätze** frei. Beide Zahlen sind nicht aufeinander bezogen. Wenn AEQ
langsam umläuft (3.2), hält ein Mensch Jahre seines Konsums kostenlos, ein
Laden nur sechs Wochen Umsatz. Das ist das Gegenteil von „Unternehmen sind
Durchlauf“: Es trifft den Durchlauf härter als das Liegen beim Menschen.

Die Regel „erstes Unternehmen nie schlechter als ein Mensch“ (ab 15.10.)
mildert das für Kleine. Für alle anderen hilft der Vorschlag aus 3.3.

### 3.5 Welche Zahl ist was

| Zahl | Herkunft | Urteil |
|---|---|---|
| 1.000 AEQ je Mensch | Grundentscheidung | Setzung, unproblematisch (alles andere ist relativ dazu) |
| Grenze 25 × | Grundentscheidung | relativ zum Durchschnitt, trägt |
| Sparfreibetrag 5 × | Euro-Gegenrechnung (Konzept 9) | relativ, trägt |
| 0,5 %/Monat (Menschen), 0,5/1 % (Unternehmen) | Chiemgauer, Wörgl | im erprobten Bereich |
| freie Adresse 0,25 ×, 1 % | Euro-Gegenrechnung | trägt |
| Sockel 2.000 | Setzung | durch die Erstes-Unternehmen-Regel entschärft |
| frei 1,5 / Stufe 3 Monatsumsätze | Setzung | **zu knapp**, Vorschlag 3 / 6 (3.3) |
| 1.000 AEQ/Monat gebührenfrei | Setzung, als „Alltag“ gedacht | **Platzhalter**, an gemessene Ausgaben binden (3.2) |
| 3.000 AEQ/Monat Tausch frei | Setzung | **Platzhalter** (3.2) |
| 9.000 AEQ je Kunde/Zahler und Quartal | Setzung | **Platzhalter**, an gemessene Ausgaben binden |
| 3 Unternehmen je Mensch, 10 Verantwortliche | Setzung | unkritisch |
| Gründungsphase 182 Tage, einmal je 365 | Setzung | unkritisch, ab 15.10. durch Erstes-Unternehmen-Regel überholt |
| Ausstieg 2 % | Chiemgauer (5 %) halbiert | Setzung; wirkt wenig, weil Verkauf an der Kette vorbei geht |

## 4. Wie die Zahlen richtig werden

1. **Messen, nicht schätzen:** Im Pilot Monatsausgaben je Mensch,
   Umlaufgeschwindigkeit, Rücklagen und Margen der Betriebe erheben (aus den
   Blöcken und im Gespräch mit den Läden).
2. **Durchrechnen:** Ein kleines Modell mit festen Akteuren (Mensch mit wenig,
   Mensch mit viel, Bäckerei, Supermarkt, Großhändler, Hülle, Absprache unter
   Freunden), das die echten Formeln aus `wirtschaft.go` nutzt und jede
   Zahländerung gegen alle Akteure zeigt.
3. **Fachlich prüfen lassen:** eine Ökonomin oder ein Ökonom mit Erfahrung in
   Komplementärwährungen liest Modell und Zahlen gegen.
4. **Flussgrenzen an Messwerte binden**, Bestandsgrenzen relativ lassen.
5. **Änderungen per Abstimmung der Menschen**, mit festen Unter- und
   Obergrenzen, damit niemand die Regeln auf einmal kippen kann.

## 5. Was jetzt zu entscheiden ist

1. **Liegegeld-Fenster 3 / 6 Monatsumsätze statt 1,5 / 3** (3.3). Wenn ja: in
   die zweite Stufe ab 15.10. aufnehmen, eine Konstante je Wert, Tests
   anpassen.
2. **Flussgrenzen als Platzhalter kennzeichnen** und vor echtem Geld an
   gemessene Ausgaben binden (3.2).
3. **Bürgen** (2.3) als zweiten Nachweis neben der Kundschaft für das
   Verzeichnis entwerfen.
4. **Modell und fachliche Prüfung** (4.2, 4.3) vor echtem Geld.
