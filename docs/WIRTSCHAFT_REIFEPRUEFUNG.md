# Wirtschaft: Reifeprüfung

> **Umsetzung (02.10.2026, später am Tag):** Entscheidungen 1 bis 5 sind
> gebaut und getestet (`x/humanity/keeper/wirtschaft2.go`, `kundschaft.go`).
> A, B und C gelten ab **15.10.2026, 00:00 UTC**; Kundschaft und
> Grundeinkommen sind sofort als Zahlen in der API. Kredit (6) ist **nicht**
> gebaut: Er hängt an der Rechtsprüfung (`RECHTSFRAGEN_UNTERNEHMEN.md`, F).
> Die Verifizierungsschichten 3 bis 5 (Website, vLEI, Bewertungen) sind
> beschrieben, aber nicht gebaut; sie gehören in App und Verzeichnis.

Stand 02.10.2026. Prüft die geltenden Wirtschaftsregeln (`docs/UNTERNEHMEN_KONZEPT.md`,
Fassung 3) gegen drei Maßstäbe: **fair** (für den Menschen mit wenig),
**Anreiz** (bringt es AEQ in Umlauf?) und **reif** (hält es dem Betrieb stand?).
Dazu: wie sich Unternehmen ausweisen, und wie Kredit ohne Banken gehen kann.
Vorschläge sind **nicht beschlossen und nicht gebaut**.

---

## 1. Kurzfassung

- **Die Grundidee trägt:** gleiches Startguthaben für jeden Menschen, kein
  Geld für Unternehmen, Horten kostet, Abgaben gehen an alle. Missbrauch ist
  begrenzt.
- **Drei Regeln sind nicht fair genug oder setzen falsche Anreize** (Abschnitt
  3.2): Kleinbetriebe stehen schlechter da als Menschen; wer AEQ an Lieferanten
  weitergibt, verliert Freibetrag; Zahlungen über der Vermögensgrenze werden
  dem Empfänger weggenommen statt abgelehnt.
- **Das Grundeinkommen wird anfangs klein sein** (Schätzung 3.3). Das muss
  ehrlich gesagt werden.
- **Unternehmen weisen sich in Schichten aus**, nicht über eine Behörde. Die
  stärkste Schicht kann nur Aequitas: echte Kundschaft von verifizierten
  Menschen (Abschnitt 2).
- **Kredit:** Normale Lending-Protokolle passen nicht und funktionieren nach den
  heutigen Regeln auch nicht. Es gibt einen faireren Weg in drei Stufen
  (Abschnitt 4), nichts davon vor dem Pilot.

---

## 2. Wie weisen sich Unternehmen aus?

### 2.1 Die Trennung, auf die es ankommt

| Frage | Braucht es eine Prüfung? |
|---|---|
| **Welche Geldregeln gelten?** | **Nein.** Liegegeld und Grenzen wirken auf jedes AEQ. Eine Scheinfirma hortet nicht billiger als freie Adressen (Konzept 4.2, 8). |
| **Wem kann ich als Kundin vertrauen?** | **Ja**, aber nicht durch eine Stelle, die zulässt oder ablehnt, sondern durch Nachweise, die jeder selbst prüfen kann. |

Damit ist die Frage „Wie prüfen wir weltweit?“ beantwortbar: **Für das Geld gar
nicht. Für das Vertrauen in Schichten, die jeder nachrechnen kann.**

### 2.2 Die Schichten

| Schicht | Nachweis | Wer prüft | Fälschen kostet |
|---|---|---|---|
| **1 Verantwortung** | mindestens ein verifizierter Mensch (Gesicht, später Iris) ist eingetragen; höchstens 3 Firmen je Mensch | die Kette | eine echte, einmalige Person |
| **2 Echte Kundschaft** | Zahl der **verschiedenen** verifizierten Menschen, die in den letzten 90 Tagen dort bezahlt haben (je Zahlung mindestens 1 AEQ; Verantwortliche und Lohnempfänger zählen nicht) | jeder, aus den Blöcken | je gefälschtem Kunden ein echter Mensch, der mitmacht |
| **3 Website** | Datei `/.well-known/aequitas.txt` auf der eigenen Domain nennt die Unternehmensadresse | jede App | Kontrolle über die Domain |
| **4 Registernachweis** (freiwillig) | anerkannter digitaler Firmennachweis, z. B. **vLEI** (weltweit, GLEIF) oder künftig die **EU-Business-Wallet** | jede App, gegen die Herausgeber | eine echte Eintragung |
| **5 Bewertungen** | nur Menschen, die dort nachweislich bezahlt haben, können bewerten; je Mensch eine Stimme | jeder | echte Kunden |

**Schicht 2 ist der Kern.** Kein anderes Geld kann sie bieten, weil nur hier
jeder Mensch genau einmal existiert. Ein Laden, bei dem 40 verschiedene echte
Menschen bezahlt haben, ist ein echter Laden, egal ob in Deutschland, Kenia oder
Vietnam, mit oder ohne Register.

**Schicht 4** gibt es heute schon weltweit: Das vLEI ist ein prüfbarer
digitaler Nachweis zur international genormten Firmenkennung LEI, ausgegeben
über zugelassene Herausgeber. Die EU hat im November 2025 eine Verordnung für
eine Business-Wallet vorgeschlagen; sie ist im Gesetzgebungsverfahren. Beides
wird **angenommen, wenn vorhanden, aber nie verlangt**: Sonst wären Läden in
Ländern ohne verlässliches Register ausgeschlossen.

### 2.3 Drei feste Regeln

1. **Keine Schicht bringt Geld.** Keine niedrigere Gebühr, kein höherer
   Freibetrag. Nur Anzeige. Bringt eine Schicht Geld, wird genau sie gefälscht.
2. **Keine Stelle lässt zu oder sperrt.** Es gibt kein „verifiziert“-Häkchen
   vom Team, nur Zahlen und Nachweise, die jeder nachrechnet.
3. **Angezeigt wird, was belegt ist, wörtlich:** „Name selbst angegeben“,
   „47 verschiedene Kundinnen und Kunden in 90 Tagen“, „Website bestätigt“,
   „vLEI vorhanden“.

### 2.4 Was das für die Wirtschaftsregeln heißt

Der Freibetrag nach Umsatz zählt heute schon Einkäufe **je Mensch gedeckelt**.
Das ist dieselbe Idee wie Schicht 2: Umsatz von echten Menschen ist der
Nachweis. Die Regeln und die Verifizierung passen also zusammen; sie müssen nur
dieselbe Sprache sprechen (Abschnitt 3.2, B).

---

## 3. Die Wirtschaftsregeln, einzeln geprüft

### 3.1 Übersicht

| Regel | Fair? | Anreiz? | Urteil |
|---|---|---|---|
| 1.000 AEQ Startguthaben je Mensch | ja, gleich für alle | **stark**: jeder hat etwas auszugeben | trägt; aber sofort verkäuflich (8.1 Nr. 13) |
| Grundeinkommen aus Abgaben | ja, gleich verteilt | schwach, weil klein | trägt, **Erwartung ehrlich machen** (3.3) |
| 1.000 AEQ/Monat gebührenfrei, dann 0,1 % | ja, schont den Alltag | neutral | trägt |
| 5.000 AEQ frei sparen, darüber 0,5 %/Monat | ja, trifft nur Wohlhabende | fördert Ausgeben bei Großen | trägt |
| Vermögensgrenze 25 × (Phase 0 kleiner) | ja im Ziel | – | **Schwäche C**: Überschuss wird dem Empfänger genommen |
| Freie Adresse 250 AEQ, 1 %/Monat | ja | schützt vor Horten ohne Mensch | trägt |
| Ausstieg: 3.000 AEQ/Monat frei, dann 2 % | ja, Kleine zahlen nichts | bremst Abfluss wenig | trägt |
| Unternehmen: Sockel 2.000, dann nach Umsatz | **nein für Kleine** | **schlecht für Kleine** | **Schwäche A** |
| B2B: nur Überschuss zählt | sicher gegen Kreise | **bestraft Weitergeben** | **Schwäche B** |
| Löhne an Menschen 0 % | ja | **stark** | trägt |
| Gründungsphase | ja | gut | trägt; Gedanke sollte dauerhaft werden (A) |
| Liegegeld 0,5 / 1 % | ja | mild | trägt |

### 3.2 Drei Schwächen und Vorschläge

**A. Kleinbetriebe stehen schlechter da als Menschen.**

Ein Mensch hält 5.000 AEQ frei und zahlt darüber 0,5 %. Ein kleiner Laden mit
1.000 AEQ Monatsumsatz hält nur 2.000 AEQ frei, zahlt bis 3.000 AEQ 0,5 % und
darüber 1 %. Nach der Gründungsphase lohnt sich das Unternehmenskonto für
Kleine also nicht: Es ist teurer, als das Geld auf dem eigenen Konto zu lassen.
Große Läden dagegen bekommen große Freibeträge. Das ist für das fairste Geld
die falsche Richtung.

*Vorschlag „Nie schlechter als ein Mensch“:* Die Rechnung der Gründungsphase
(Gründerin und erstes Unternehmen teilen sich den Freibetrag von 5.000 AEQ, es
gilt der niedrigere Wert) gilt **dauerhaft für das erste offene Unternehmen
jedes Menschen**, nicht nur ein halbes Jahr. Der Code dafür existiert schon
(`liegegeldLocked`); geändert würde nur die Bedingung. Kein Vorteil gegenüber
einem Menschen, weil der Freibetrag geteilt wird; kein Nachteil mehr für Kleine.

**B. Wer AEQ an Lieferanten weitergibt, verliert Freibetrag.**

Zwischen Unternehmen zählt nur der Überschuss: Eingänge minus Zahlungen an
Unternehmen. Eine Mühle, die 10.000 AEQ von Bäckereien bekommt und 8.000 an den
Hof weitergibt, hat 2.000 AEQ „Umsatz“. Hätte sie den Hof in Euro bezahlt,
wären es 10.000. **Die Regel bestraft genau das Weitergeben in AEQ, von dem der
Kreislauf lebt** (Konzept 10). Sie wurde gegen Kreisgeschäfte (A → B → C → A)
eingeführt, und dort wirkt sie; aber der Preis ist zu hoch.

*Vorschlag „Wie bei Menschen“:* Eingänge von anderen Unternehmen zählen **voll**,
aber je zahlendem Unternehmen höchstens 9.000 AEQ im Quartal, genau wie bei
Menschen; eigene Firmen zählen weiter nicht. Zahlungen an Lieferanten senken den
Umsatz nicht mehr. Ein Kreis bringt dann je beteiligter Firma höchstens 9.000
AEQ im Quartal, und jede Firma braucht einen echten Menschen dahinter: dieselbe
Grenze, die bei Kundschaft schon gilt (Konzept 8.1 Nr. 6).

**C. Zahlungen über der Vermögensgrenze werden dem Empfänger genommen.**

Bekommt ein Mensch eine Zahlung, die ihn über die Grenze bringen würde, geht die
Überweisung durch, und der Überschuss wird sofort verteilt. Der Empfänger kann
das nicht verhindern: Ein Lohn, der kurz vor der Grenze ankommt, ist teilweise
weg. Mit wenigen Menschen auf der Kette (Phase 0: 5.000 AEQ) trifft das gerade
die ersten Pilotläden.

*Vorschlag „Ablehnen statt wegnehmen“:* Eine Überweisung, die einen Menschen
über die Grenze bringen würde, wird **abgelehnt**, wie heute schon bei freien
Adressen über 250 AEQ. Das Geld bleibt beim Absender, der Empfänger kann erst
ausgeben und dann annehmen. Das Wegnehmen bleibt nur für Eingänge, die niemand
steuert (Grundeinkommen, Freigaben). Die Grenze bleibt genauso hart, aber
niemand verliert Geld durch eine Zahlung, die er nicht kontrolliert.

Alle drei sind Konsensänderungen: Aktivierungszeit, Missbrauchstests, eigene
Sicherheitsprüfung. **Vorschlag: vor dem ersten Pilotladen entscheiden und
bauen**, weil sie genau die ersten Läden betreffen.

### 3.3 Wie groß wird das Grundeinkommen? (Schätzung)

Das Grundeinkommen besteht nur aus Abgaben; neues Geld entsteht dafür nicht.
Bei normalem Verhalten zahlen die meisten Menschen fast nichts (unter 1.000 AEQ
Ausgaben, unter 5.000 AEQ Guthaben, unter 3.000 AEQ Ausstieg). Eine grobe
Rechnung je Mensch und Monat:

| Quelle | Annahme | je Mensch/Monat |
|---|---|---|
| Überweisungsgebühr | Ausgaben im Mittel 1.500 AEQ | 0,5 AEQ |
| Ausstiegsabgabe | jeder Zehnte tauscht 5.000 AEQ | 4 AEQ |
| Liegegeld, Umlauf, Grenze | wenige Große | < 1 AEQ |
| **zusammen** | | **etwa 1 bis 5 AEQ**, also 0,1 bis 0,5 % des fairen Anteils |

Das ist ein Ausgleich, kein Lebensunterhalt. **Fair ist das Geld vor allem durch
die gleiche Ausgabe** (jeder bekommt dasselbe) und die Grenze, nicht durch die
Höhe des Grundeinkommens. Vorschlag: In App und Website das tatsächliche
Grundeinkommen der letzten 30 Tage je Mensch anzeigen, statt das Wort allein
stehen zu lassen.

### 3.4 Was keine Regel lösen kann

Nachfrage. AEQ ist so viel wert, wie man dafür bekommt. Die Regeln verhindern
Horten und Missbrauch; Gründe zum Bleiben schaffen nur Menschen und Läden am
selben Ort (Konzept 0a, `docs/PILOTLADEN.md`).

---

## 4. Kredit und Zinsen ohne Banken

### 4.1 Warum „ganz normale“ Lending-Protokolle nicht passen

Protokolle wie Aave oder Compound verleihen nur **gegen Sicherheit**, die mehr
wert ist als der Kredit. Das hilft, wer schon Vermögen hat (meist zum Hebeln
für Spekulation), und nicht der Bäckerei, die einen Ofen braucht. Für das
fairste Geld ist das der falsche Kredit.

Dazu funktionieren sie nach den heutigen Regeln **gar nicht**:
- Ein Vertrag ist eine freie Adresse und hält höchstens 250 AEQ. Ein
  Kreditpool kann so nicht arbeiten.
- Würde man ihn freischalten, wären die Anteilsscheine des Pools ein Versteck vor
  Umlauf und Vermögensgrenze, dieselbe Lücke, die bei der Liquidität am
  20.08.2026 geschlossen wurde (`WHO_MAY_HOLD_AEQ.md`).
- Es gibt keinen echten Stable-Wert als Sicherheit, nur tUSD.

### 4.2 Was Aequitas anders kann

1. **Jeder Mensch existiert einmal.** Wer einen Kredit nicht zurückzahlt, kann
   nicht mit einem neuen Konto von vorn anfangen. Das macht Kredit **ohne
   Sicherheit, nur auf Ruf**, möglich, was in anderen Netzen an Scheinkonten
   scheitert.
2. **Umlaufsicherung macht zinsfreies Verleihen vernünftig.** Wer mehr hält, als
   frei ist, zahlt dafür. Es verleihen, ohne Zins, kostet ihn nichts, solange er
   es zurückbekommt. Silvio Gesell, auf den die Umlaufsicherung zurückgeht, hat
   genau das vorhergesagt: Zins fällt, wenn Geldhalten kostet.
3. **Vorbilder:** Die schwedische JAK Medlemsbank verleiht seit Jahrzehnten
   ohne Zins: Wer gespart hat, sammelt Sparpunkte und darf entsprechend leihen;
   die Kosten decken Gebühren. WIR und Sardex geben Unternehmen zinsfreie
   Kreditlinien im eigenen Verrechnungsnetz.

### 4.3 Vorschlag in drei Stufen

| Stufe | Was | Für wen | Wann |
|---|---|---|---|
| **1 Zahlungsziel** | Rechnung mit Frist auf der Kette: Lieferung jetzt, Zahlung in 30 Tagen, für beide sichtbar. Kein neues Geld, kein Zins | Unternehmen untereinander | im Pilot, wenn gewünscht |
| **2 Darlehen zwischen Menschen** | zinsfrei oder mit fester, kleiner Gebühr (geht ans Grundeinkommen, nicht an den Verleiher); Rückzahlung automatisch aus künftigen Eingängen | Menschen und Kleinbetriebe | nach dem Pilot, nach Rechtsprüfung |
| **3 Gegenseitiger Kredit** | Unternehmen dürfen bis zu einer Grenze ins Minus, gedeckt durch ihre echte Kundschaft (Schicht 2), zinsfrei, wie WIR und Sardex | Unternehmen mit Geschichte | erst nach Abstimmung der Menschen |

**Schutzregeln für Stufe 2**, damit aus Kredit keine Schuldfalle wird:
- Höchstbetrag je Mensch: ein fairer Anteil (1.000 AEQ) an offenen Schulden.
- Automatische Rückzahlung höchstens 20 % jedes Eingangs; Grundeinkommen und
  Startguthaben sind nie gepfändet.
- Kein Zinseszins, keine Vertragsstrafe.
- **Verjährung:** Nach 3 Jahren erlischt eine Schuld. Das Risiko trägt der
  Verleiher, nicht ein Leben lang die Schuldnerin.
- Öffentlich ist nur, ob jemand eine offene Schuld hat und ob er zurückgezahlt
  hat, nicht bei wem.
- Verliehenes zählt für die Vermögensgrenze des Verleihers mit, sonst wäre
  Verleihen ein Versteck (dieselbe Lehre wie bei der Liquidität).

**Stufe 3 ist eine Grundsatzfrage**, weil sie vorübergehend Guthaben ohne
Startguthaben dahinter erlaubt (das Minus des einen ist das Plus des anderen;
die Summe bleibt gleich). Sie berührt das Versprechen „Geld entsteht nur durch
Menschen“ und gehört deshalb zur Abstimmung, nicht in einen Code-Beschluss.

**Recht:** Gewerbsmäßig Kredite zu geben oder zu vermitteln ist in Deutschland
ein Bankgeschäft beziehungsweise erlaubnispflichtig. Ob das für ein Protokoll
gilt, das Menschen untereinander leihen lässt, muss die Kanzlei klären
(`docs/RECHTSFRAGEN_UNTERNEHMEN.md`, Teil F).

---

## 5. Die fünf Sorgen, mit Lösung

| Sorge | Lösung | Stand |
|---|---|---|
| **Missbrauch** | Regeln wirken auf jedes AEQ; bekannte Angriffe begrenzt; Verifizierung in Schichten ohne Geldvorteil (2) | geprüft, eine Lücke behoben |
| **Niemand macht mit** | Pilotstadt, erst Menschen; Pilotladen ohne Unternehmenskonto; Schwächen A und B beheben, damit Kleine und Lieferketten nicht bestraft werden | Ablauf steht, A/B zu entscheiden |
| **Komplexität** | Vorschläge A und B **vereinfachen** die Regeln: eine Rechnung für Kleine, eine Zählweise für Menschen und Unternehmen | zu entscheiden |
| **Recht** | Kanzlei mit dem Fragenkatalog, ergänzt um Kredit | Fragen liegen bereit |
| **Wirkung nach außen** | Website ehrlich; Grundeinkommen mit echter Zahl (3.3); Verifizierung als Zahlen statt Häkchen | Website geändert, Rest zu bauen |

## 6. Zu entscheiden

1. **A** „Nie schlechter als ein Mensch“ für das erste Unternehmen: ja/nein.
2. **B** Eingänge von Unternehmen voll zählen, je Zahler gedeckelt: ja/nein.
3. **C** Überweisungen über der Vermögensgrenze ablehnen statt wegnehmen: ja/nein.
4. Grundeinkommen der letzten 30 Tage in App und Website anzeigen: ja/nein.
5. Verifizierung in Schichten (2.2) als Grundlage für das Verzeichnis: ja/nein.
6. Kredit: Stufe 1 im Pilot erlauben; Stufe 2 nach Rechtsprüfung; Stufe 3 zur
   Abstimmung: ja/nein.

Empfehlung: 1 bis 5 ja, vor dem ersten Pilotladen bauen; 6 wie beschrieben.

## Quellen

- GLEIF, vLEI: https://www.gleif.org/en/organizational-identity/get-an-lei-vlei/get-a-vlei
- EU-Business-Wallet, Vorschlag COM(2025) 838: https://www.eesc.europa.eu/de/our-work/opinions-information-reports/opinions/european-business-wallet
- JAK Medlemsbank: https://en.wikipedia.org/wiki/JAK_Members_Bank
