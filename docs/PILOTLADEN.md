# Der erste Pilotladen – Ablauf ohne Unternehmenskonto

Stand 02.10.2026. Für die Beta: Ein Laden probiert AEQ aus, ohne neuen Code,
ohne Unternehmenskonto und mit einem Risiko, das er selbst begrenzt. Die
Regeln dahinter stehen in `docs/UNTERNEHMEN_KONZEPT.md`.

## Wann es losgeht

Erst wenn in der Pilotstadt genug Menschen registriert sind, dass ein Laden
überhaupt Kundschaft mit AEQ hat. Richtwert: 50 bis 100 Menschen im Umkreis.
Vorher bringt ein Laden nichts, und ein enttäuschter erster Laden schadet mehr,
als er nützt.

## Ablauf in fünf Schritten

1. **Inhaberin registriert sich als Mensch** wie alle anderen (App,
   Gesichtsprüfung). Zahlungen gehen zunächst auf ihr eigenes Konto.
2. **Grenze festlegen und aushängen.** Zum Beispiel: „Wir nehmen bis 20 % des
   Einkaufs in AEQ“ oder „bis 500 AEQ im Monat“. Den AEQ-Preis legt der Laden
   selbst fest. Es gibt keinen offiziellen Kurs, und wir nennen keinen.
3. **Kassieren:** In der App auf „Empfangen“ tippen, die Kundin scannt den
   QR-Code und gibt den Betrag ein. Der Laden prüft den Eingang in der App,
   bevor die Ware über den Tresen geht. Annehmen kostet den Laden nichts; die
   Kundin zahlt bis 1.000 AEQ im Monat ebenfalls nichts.
4. **Weitergeben:** Was der Laden einnimmt, kann die Inhaberin selbst ausgeben,
   am besten bei einem anderen Pilotbetrieb (Lieferant, Nachbarladen). Darum
   werden die Läden als Kette geworben, nicht einzeln.
5. **Aufschreiben:** Bis es einen Export gibt, führt der Laden eine einfache
   Liste (Datum, Betrag, Zweck). Der Explorer zeigt jede Zahlung zum Abgleich.

## Die eine Grenze, die man kennen muss

Ein Menschenkonto hat eine **Vermögensgrenze**. Bis zum 03.10.2026, 12:00 UTC, wird, was
darüber eingeht, sofort an alle Menschen verteilt; es ist für die Inhaberin
verloren. **Ab dem 03.10.2026 (12:00 UTC)** wird eine solche Zahlung abgelehnt: Die
Kundin behält ihr Geld, der Laden muss erst ausgeben, bevor er wieder annehmen
kann.

| Menschen auf der Kette | Grenze für ein Menschenkonto |
|---|---|
| bis 5 | 5.000 AEQ |
| 10 | 10.000 AEQ |
| 25 und mehr | 25.000 AEQ |

Dazu: Über 5.000 AEQ kostet das Halten 0,5 % im Monat.

**Faustregel für den Pilot:** Liegen mehr als zwei Drittel der aktuellen
Grenze auf dem Konto, wird ausgegeben oder es wird ein Unternehmenskonto
eröffnet. Wer die Grenze per Teilannahme begrenzt (Schritt 2), kommt in der
Regel gar nicht in die Nähe.

## Wann ein Unternehmenskonto kommt

Wenn ein Laden regelmäßig an die Grenze kommt, Löhne in AEQ zahlen will oder
mehrere Menschen gemeinsam verantwortlich sind. Das Unternehmenskonto hat
keine feste Grenze und hält bis anderthalb Monatsumsätze kostenlos. Ab dem
03.10.2026 ist das **erste** Unternehmen eines Menschen nie teurer als das
eigene Konto (Konzept 4.4); der Wechsel lohnt sich also auch für Kleine. Heute geht
die Eröffnung nur über die Schnittstelle mit Hilfe des Teams; die Anmeldung in
der App wird gebaut, sobald der erste Laden sie braucht.

## Was wir dem Laden ehrlich sagen

- In der Beta gibt es **keinen Umtausch in Euro**. AEQ hat noch keinen
  Euro-Wert. Wer mitmacht, tut es für die Kundschaft und für die Idee.
- Ob und wie AEQ-Einnahmen steuerlich zu buchen sind, ist **noch nicht
  geklärt** (`docs/RECHTSFRAGEN_UNTERNEHMEN.md`). Bis dahin: nur in einem
  Umfang mitmachen, den man auch als Werbung verbuchen würde, und die eigene
  Steuerberatung fragen.
- Es gibt **keine Kasse und keinen Export** in der App, nur Empfangen per QR.

## Was der Pilot misst

Siehe Konzept 10.3. Für den ersten Laden genügt: wie viele verschiedene
Menschen bei ihm zahlen, wie viel er weitergibt und wo er an Grenzen stößt.
