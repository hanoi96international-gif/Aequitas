# Aequitas für Unternehmen – Konzept

Stand: 25.09.2026 · Status: **Entwurf zur Entscheidung** (nichts davon ist gebaut)

## In einem Satz

Unternehmen dürfen AEQ annehmen, halten und ausgeben – aber AEQ ist so
gebaut, dass es **durch Unternehmen hindurchfließt und bei Menschen ankommt**:
Liegenlassen kostet, an Menschen weitergeben ist gratis, Aussteigen in
Euro/Dollar kostet eine Abgabe, die ins Grundeinkommen geht.

---

## 1. Warum nicht „Unternehmen nur in USD“?

Die naheliegende Idee – Menschen halten AEQ, Unternehmen nehmen nur Stablecoins –
schützt das Geld nicht, sondern entwertet es:

1. Jeder Einkauf wird zu einem **Verkauf von AEQ** (AEQ → Stable → Händler).
2. Käufer für AEQ gibt es dann kaum: Wofür sollte jemand AEQ kaufen, wenn man
   damit nirgends bezahlen kann?
3. Dauerhafter Verkaufsdruck, **fallender Kurs**, das Grundeinkommen ist immer
   weniger wert. AEQ würde zur Wertmarke, die man sofort loswerden will.

Dazu kommen Reibung bei jedem Einkauf (Tausch, Kursrisiko, Liquidität) und die
Abhängigkeit von einem Stablecoin-Anbieter. **Zirkulation entsteht nur, wenn
Unternehmen AEQ selbst weitergeben können** – an Beschäftigte, Lieferanten,
andere Unternehmen.

## 2. Leitlinien

1. **Das Geld ist für Menschen.** Grundeinkommen, Stimmrecht und der faire
   Anteil gehören nur verifizierten Menschen.
2. **Unternehmen sind Durchlauf, kein Speicher.** Wer AEQ hortet, zahlt. Wer es
   weitergibt, zahlt nichts.
3. **Kein Vorrecht, kein neues Geld.** Für Unternehmen wird kein AEQ geschöpft.
   Alles, was sie zahlen, fließt ins Grundeinkommen.
4. **Hinter jedem Unternehmen steht ein Mensch.** Verantwortung ist nicht
   anonym, das Unternehmen selbst kann es sein.
5. **Einfach für den Laden.** Bezahlen per QR, Preise auch in Euro sichtbar,
   Buchhaltungs-Export. Ohne das nimmt kein Bäcker AEQ an.

## 3. Die Fairness-Garantie für Menschen

Das fairste Geld der Welt muss sich zuerst für den einzelnen Menschen fair
anfühlen, und zwar für den mit wenig. Deshalb gelten für Menschen sechs Zusagen.
Alle Zahlen im Rest dieses Konzepts ordnen sich ihnen unter.

1. **Dein fairer Anteil ist unantastbar.** Auf die ersten 1.000 AEQ (der faire
   Anteil) gibt es nie Haltegebühr, Demurrage oder Abgabe.
2. **Der Alltag kostet nichts.** Die ersten **1.000 AEQ, die du im Monat
   ausgibst** (an Menschen oder Läden), sind **gebührenfrei**. Die
   Überweisungsgebühr zahlt erst, wer mehr ausgibt.
3. **Lohn ist Lohn.** AEQ, das du als Lohn von einem Unternehmen bekommst,
   kannst du **ohne Abgabe** in Euro/Dollar tauschen. Zusätzlich hat jeder
   Mensch **1.000 AEQ im Monat** Tausch-Freibetrag.
4. **Sparen ist erlaubt.** Bis **5.000 AEQ** (5 × fairer Anteil) verliert
   Erspartes nichts. Erst auf den Teil darüber wirkt die Demurrage.
5. **Wer mehr hat, trägt mehr.** Gebühren und Demurrage steigen erst mit großem
   Guthaben. Die Grenze von 25.000 AEQ bleibt.
6. **Halten und Aussteigen kosten Menschen immer weniger als Unternehmen.**
   Die Demurrage für Menschen liegt unter der Haltegebühr für Unternehmen, und **alles, was irgendwer zahlt, geht zu
   gleichen Teilen an alle Menschen zurück.**

Warum die Freibeträge **pro Mensch** gelten und nicht pro Konto: Jeder Mensch
existiert bei Aequitas genau einmal. Ein Freibetrag pro Mensch lässt sich
deshalb nicht vervielfachen. Das kann kein anderes Geld.

## 4. Die Lücke, die dieses Konzept mit schließt

`docs/WHO_MAY_HOLD_AEQ.md` (20.08.2026) regelt: *Jeder darf AEQ halten, jede
Adresse unterliegt Demurrage und Vermögensgrenze.* Zwei Punkte halten im Code
nicht, was die Regel verspricht:

- **Die Vermögensgrenze gilt pro Adresse, nicht pro Mensch.** 25.000 AEQ je
  Adresse (`wealthCapMultiplier` × fairer Anteil). Wer sein Vermögen auf zehn
  unregistrierte Adressen verteilt, hat eine Grenze von 250.000 AEQ. Adressen
  kosten nichts, die Grenze hält also nicht.
- **Demurrage trifft aktive Konten nie.** Sie beginnt erst nach 90 Tagen ohne
  Bewegung (`demurrageGracePeriodSeconds`), jede Bewegung setzt die Uhr zurück.
  Die tägliche UBI-Gutschrift setzt sie bei jedem Menschen zurück, ein aktives
  Unternehmen setzt sie mit jedem Verkauf zurück. Sie ist in der Praxis ein
  Aufräumen verlassener Konten, keine Umlaufsicherung (so auch schon in
  `WHO_MAY_HOLD_AEQ.md` festgestellt).

Ein Unternehmensmodell auf dieser Grundlage hätte weder eine Grenze noch einen
Umlaufanreiz. Deshalb braucht es drei Kontoarten mit klaren Regeln statt einer.

## 5. Drei Kontoarten

| | **Mensch** | **Unternehmen** (neu) | **Freie Adresse** |
|---|---|---|---|
| Wer | verifizierter Mensch | eröffnet von 1–N verifizierten Menschen | jede sonstige Adresse, auch Verträge |
| Grundeinkommen | ✅ | ❌ | ❌ |
| Stimmrecht | ✅ | ❌ | ❌ |
| Obergrenze | 25.000 AEQ (wie heute) | **keine feste** – dafür Haltegebühr (6.2) | **1.000 AEQ** (= fairer Anteil) |
| Umlaufsicherung | **0,5 %/Monat nur auf den Teil über 5.000 AEQ** | **1–3 %/Monat auf das, was über dem Freibetrag liegt** | **1 %/Monat ab dem ersten AEQ** |
| Überweisen | **erste 1.000 AEQ im Monat gratis**, dann 0,1 % (Aufschlag nur bei großem Guthaben) | an Menschen **0 %**, sonst 0,1 % | 0,1 % |
| Umtausch in Euro/Dollar | **Lohn + 1.000 AEQ/Monat ohne Abgabe**, darüber 2 % | 2 % | 2 % |

Umlaufsicherung und Haltegebühr laufen bei allen drei Kontoarten **ohne
Schonfrist, und Aktivität setzt sie nicht zurück.** Nur so wirken sie überhaupt
(siehe Abschnitt 4). Für Menschen ist das keine Verschärfung, sondern das
Gegenteil: Heute gilt die Demurrage (auf dem Papier) ab 1.000 AEQ, künftig erst
ab 5.000. Wer normal lebt und spart, ist nie betroffen.

Die **freie Adresse** bleibt für alles Kleine möglich: Geldbörse eines Besuchers,
Test, Trinkgeldkasse, ein einfacher Vertrag. Als Versteck taugt sie nicht mehr:
höchstens 1.000 AEQ je Adresse, und Halten kostet vom ersten Tag an. Wer mehr
Geld dauerhaft außerhalb des eigenen Kontos halten will, eröffnet ein
Unternehmenskonto und steht mit seinem Namen dafür.

Die **Protokoll-Töpfe** (UBI, LP, Validatoren) bleiben wie heute ausgenommen,
sie sind Durchlauf.

## 6. Regeln für Unternehmenskonten

### 6.1 Eröffnen, Mitinhaber, Schließen

- **Eröffnen:** Transaktion `unternehmen_eroeffnen`, signiert von einem
  verifizierten Menschen. Enthält einen Anzeigenamen (freiwillig) und eine
  Kategorie (z. B. Lebensmittel, Handwerk, Dienstleistung, Verein).
- **Mitinhaber:** weitere verifizierte Menschen können beitreten
  (`unternehmen_mitinhaber`, signiert vom Beitretenden und einem bisherigen
  Verantwortlichen). So bildet ein Konto eine GbR, GmbH oder Genossenschaft ab.
- **Grenze:** Jeder Mensch ist für **höchstens 3 Unternehmenskonten**
  verantwortlich. Das verhindert, dass jemand den Freibetrag über viele Konten
  vervielfacht.
- **Schließen:** Das Restguthaben geht zu gleichen Teilen an die
  Verantwortlichen (gebührenfrei, aber unter ihrer 25.000-Grenze). Was darüber
  liegt, geht ins Grundeinkommen.
- **Öffentlich im Explorer:** Anzeigename, Kategorie, Zahl der Verantwortlichen,
  Umsatz- und Lohnsummen je Monat. **Nicht öffentlich:** welche Menschen dahinter
  stehen (nur „verifiziert: ja, Anzahl: 2“).

### 6.2 Haltegebühr statt Obergrenze

Ein Unternehmen hat Umsatz, und Umsatz ist kein Vermögen. Eine Bäckerei mit
40.000 AEQ Monatsumsatz würde an einer 25.000-Grenze scheitern, ohne reich zu
sein. Deshalb gilt für Unternehmen **keine feste Obergrenze**, sondern eine
Gebühr auf das, was *liegen bleibt*:

- **Freibetrag** = **2.000 AEQ Sockel + die Netto-Ausgaben des Vormonats.**
  Wer viel weitergibt, darf auch viel Betriebsmittel halten.
- **Netto-Ausgaben** zählen **je Gegenüber**: was an ein Konto gezahlt wurde,
  minus was von demselben Konto zurückkam. Geld im Kreis zu schicken
  (A → B → A) ergibt null und erhöht den Freibetrag nicht.
- **Nicht mit gezählt** werden Zahlungen zwischen Konten mit gemeinsamen
  Verantwortlichen und Entnahmen an die eigenen Verantwortlichen. Man kann den
  Freibetrag also nicht mit dem eigenen Geld aufblasen.
- Auf den Teil **zwischen Freibetrag und 3 × Freibetrag: 1 % pro Monat.**
- Auf den Teil **über 3 × Freibetrag: 3 % pro Monat.**
- Läuft **sekundengenau** (wie die heutige Demurrage) und wird bei jeder Bewegung
  verrechnet. Es gibt **keine Schonfrist, und Aktivität setzt nichts zurück.**
  Genau das unterscheidet sie von der heutigen Demurrage.
- Die Einnahmen gehen **zu 100 % ins Grundeinkommen.**

Zum Vergleich: Der Chiemgauer verliert rund 2 % je Quartal (≈ 0,66 %/Monat),
Wörgl 1932 hatte 1 %/Monat. 1 % liegt also in der erprobten Spanne. 3 % gelten
nur für echtes Horten.

### 6.3 Gebühren nach Richtung

| Richtung | Gebühr | Begründung |
|---|---|---|
| Mensch → Unternehmen (Einkauf) und Mensch → Mensch | **0 % für die ersten 1.000 AEQ im Monat**, danach 0,1 % (+ Aufschlag ab 5/10/20 × fairer Anteil). Zahlt der Absender obendrauf | der Alltag kostet nichts. Preis 10 AEQ bringt dem Laden genau 10 AEQ |
| **Unternehmen → Mensch** (Lohn, Entnahme, Erstattung) | **0 %** | der Weg, den das Geld nehmen soll, ist der günstigste |
| Unternehmen → Unternehmen (Lieferant) | 0,1 %, **ohne** den Aufschlag für große Guthaben | Lieferketten sollen nicht bestraft werden, Horten regelt 6.2 |
| Unternehmen → freie Adresse | 0,1 % | |

Der heutige Aufschlag für große Guthaben (+0,1 / +0,5 / +1 % ab 5/10/20 × fairer
Anteil) gilt **nur für Menschen und freie Adressen**. Bei Unternehmen übernimmt
die Haltegebühr diese Rolle, sonst würden gerade Unternehmen mit vielen Löhnen
doppelt zahlen.

### 6.4 Umtausch in Euro/Dollar: Ausstiegsabgabe

| Wer tauscht AEQ → Stable | Abgabe |
|---|---|
| Mensch: **erhaltener Lohn** (vom Unternehmen, bei dem man *nicht* Verantwortlicher ist) | keine Abgabe |
| Mensch: dazu **1.000 AEQ je Kalendermonat** | keine Abgabe |
| Mensch: darüber | 2 % |
| Unternehmen | 2 % |
| Freie Adresse | 2 % |

Die Abgabe geht **zu 100 % ins Grundeinkommen.** Die normale Tauschgebühr von
0,1 % (an Liquiditätsgeber, Validatoren und Grundeinkommen) bleibt für alle
bestehen. Stable → AEQ (Einsteigen) kostet nur diese 0,1 %.

**Lohn oder Entnahme?** Die Kette weiß, wer für ein Unternehmenskonto
verantwortlich ist. Zahlungen an Verantwortliche sind **Entnahmen** und erhöhen
den Tausch-Freibetrag nicht. Zahlungen an alle anderen Menschen sind **Lohn**,
und wer für seine Arbeit in AEQ bezahlt wird, kann diesen Lohn ohne Abgabe
tauschen. Eine Restlücke bleibt: Zwei Inhaber könnten sich gegenseitig als
„Angestellte“ bezahlen. Das ist auffällig (öffentliche Lohnsummen im Explorer)
und durch die Vermögensgrenze der Menschen begrenzt.

**Warum ein Freibetrag pro Mensch und nicht pro Adresse:** Ohne ihn könnte ein
Unternehmen seine Einnahmen gebührenfrei an den Inhaber zahlen, und der
tauscht für 0,1 % um. Mit dem Freibetrag lassen sich über diesen Umweg höchstens
1.000 AEQ im Monat pro Mensch herausnehmen. Da jeder Mensch nur einmal existiert,
lässt sich dieser Freibetrag nicht vervielfachen. Das ist der eigentliche Vorteil
von Aequitas: **Regeln pro Mensch sind hier wirklich durchsetzbar.**

Richtwert: Der Chiemgauer verlangt beim Rücktausch 5 %. 2 % liegen darunter und
im Bereich üblicher Kartengebühren für kleine Händler.

### 6.5 Wer ist ein Unternehmen? Wir müssen es nicht wissen.

Ein dezentrales Netz kann nicht prüfen, ob hinter einem Konto eine echte Firma
steht, und soll es auch nicht: Kein Handelsregister, kein Amt, keine zentrale
Stelle entscheidet. Stattdessen gilt:

> **Wir prüfen nicht, was jemand ist. Wir machen Horten in jeder Form teuer
> und Weitergeben in jeder Form billig.**

Dann ist die Anmeldung als Unternehmen **selbst-selektierend**: Für einen echten
Laden, der Geld weitergibt, ist das Unternehmenskonto günstig. Für jemanden, der
nur horten will, ist es die teuerste aller Möglichkeiten.

**Beispiel: 100.000 AEQ horten, ohne echte Ausgaben**

| Weg | Kosten |
|---|---|
| als Mensch | **unmöglich**, Grenze 25.000 AEQ |
| auf 100 freien Adressen à 1.000 AEQ | 1 %/Monat = **1.000 AEQ/Monat** |
| als „Unternehmen“ (Freibetrag 2.000) | 4.000 × 1 % + 94.000 × 3 % = **2.860 AEQ/Monat ≈ 34 % im Jahr** |

**Die Umgehungswege, geprüft:**

| Trick | Warum er nicht funktioniert |
|---|---|
| Als Unternehmen anmelden, um die 25.000-Grenze zu umgehen | Über dem Freibetrag 1–3 % pro Monat, das ist teurer als jede andere Form |
| Geld zwischen zwei eigenen Firmen hin und her schicken, um den Freibetrag aufzublasen | Netto je Gegenüber = 0, Konten mit gemeinsamen Verantwortlichen zählen gar nicht |
| Mit einem Freund im Kreis zahlen (A → B → A) | Netto je Gegenüber = 0 |
| Viele Firmen gründen, um viele Sockel-Freibeträge zu bekommen | höchstens 3 Konten je Mensch, also höchstens 6.000 AEQ Sockel |
| Sich selbst als „Angestellten“ bezahlen, um die Ausstiegsabgabe zu sparen | Zahlungen an Verantwortliche sind Entnahmen, kein Lohn (6.4) |
| Private Einkäufe über die Firma machen, um den Aufschlag für große Guthaben zu sparen | Das Geld muss erst einmal in die Firma. Wer es aus seinem Menschenkonto dorthin schickt, zahlt den Aufschlag schon dabei |
| Riesige Ausgaben an einen Komplizen, der das Geld behält | Das Geld liegt dann beim Komplizen und unterliegt dort seiner Grenze bzw. Haltegebühr. Horten wird nur verschoben, nicht billiger |

Was bleibt: Zwei Komplizen mit echtem Wechselgeschäft können sich Freibetrag
verschaffen, aber nur so viel, wie tatsächlich dauerhaft zwischen ihnen fließt,
und jede Zahlung kostet 0,1 %. Das ist klein, sichtbar (öffentliche
Umsatzsummen im Explorer) und wird in der Pilotphase gemessen.

### 6.6 Warum es sich für echte Unternehmen lohnt

| Vorteil | |
|---|---|
| **Keine Kartengebühren** | Zahlungen an den Laden sind für ihn gratis. Für Kunden sind die ersten 1.000 AEQ im Monat auch gratis |
| **Kein Wachstumsdeckel** | Anders als Menschen haben Unternehmen keine 25.000-Grenze. Wer Geld weitergibt, zahlt praktisch keine Haltegebühr |
| **Gebührenfreie Löhne** | Löhne in AEQ kosten nichts, Angestellte können sie ohne Abgabe tauschen |
| **Günstige Lieferketten** | 0,1 % zwischen Unternehmen, ohne Aufschlag |
| **Sofortige Zahlung** | Geld ist in Sekunden da, keine Rückbuchungen wie bei Karten oder Lastschrift |
| **Neue Kundschaft** | Menschen mit Grundeinkommen suchen Orte, an denen sie es ausgeben können |
| **Sichtbarkeit** | Eintrag im öffentlichen Unternehmensregister als „Aequitas-Partner“ |

Ein Café wie im Beispiel in Abschnitt 8 zahlt im Normalbetrieb **null**.

## 7. Der Kreislauf

```mermaid
flowchart LR
    M[Menschen] -- Einkauf, erste 1.000/Monat gratis --> U[Unternehmen]
    U -- Löhne & Entnahmen, 0 % --> M
    U -- Lieferanten, 0,1 % --> U2[andere Unternehmen]
    U2 -- Löhne, 0 % --> M
    U -. Haltegebühr 1–3 %/Monat .-> G[Grundeinkommen]
    U -. Ausstieg 2 % .-> G
    M -. Überweisungsgebühr .-> G
    G -- täglich, gleich für alle --> M
```

Jeder Weg, auf dem AEQ **bei Unternehmen liegen bleibt oder das Netz verlässt**,
speist das Grundeinkommen. Jeder Weg **zurück zu Menschen** ist gratis.

## 8. Rechenbeispiele

### Für Menschen

**Anna** lebt vom Grundeinkommen, hat 1.200 AEQ und gibt 800 AEQ im Monat aus.
Sie zahlt **nichts**: keine Überweisungsgebühr (unter 1.000 im Monat), keine
Demurrage (unter 5.000), keine Abgabe.

**Ben** arbeitet im Café und bekommt 2.000 AEQ Lohn. Er gibt 1.500 AEQ aus und
tauscht 1.000 AEQ für die Miete in Euro. Er zahlt **0,5 AEQ** Überweisungsgebühr
(0,1 % auf 500 über dem Freibetrag) und die normale Tauschgebühr von 1 AEQ.
**Keine Abgabe**, weil es sein Lohn ist.

**Clara** hat 20.000 AEQ, gibt 3.000 AEQ im Monat aus und tauscht 5.000 AEQ in
Euro. Sie zahlt:
- Überweisungen: 2.000 × 1,1 % (Aufschlag ab 20 × fairer Anteil) = 22 AEQ
- Demurrage: 15.000 × 0,5 % = 75 AEQ
- Umtausch: 4.000 × 2 % = 80 AEQ (die ersten 1.000 sind frei)
- zusammen **177 AEQ im Monat**. Alles geht ans Grundeinkommen, also auch an
  Anna und Ben.

### Für Unternehmen

**Café** – Monatsumsatz 3.000 AEQ, Löhne 1.500, Lieferant 800, Entnahme 600.
Freibetrag = 2.000 + 1.500 + 800 = 4.300 AEQ (die Entnahme zählt nicht mit).
Der Kontostand bleibt um 2.000 AEQ, also **keine Haltegebühr.** Abgaben nur, wenn es in Euro tauscht.

**Supermarkt, der hortet** – Netto-Ausgaben im Vormonat 40.000 AEQ, Freibetrag
2.000 + 40.000 = 42.000. Kontostand 200.000 AEQ:
- 42.000 bis 126.000 → 84.000 × 1 % = 840 AEQ/Monat
- über 126.000 → 74.000 × 3 % = 2.220 AEQ/Monat
- zusammen **3.060 AEQ/Monat ins Grundeinkommen**, bis das Geld wieder
  ausgegeben ist. Zahlt er stattdessen Löhne, sinkt die Gebühr, und das Geld
  landet direkt bei Menschen.

**Jemand, der 100.000 AEQ horten will** – siehe Abschnitt 6.5: als Mensch
unmöglich (Grenze 25.000), auf 100 freien Adressen 1.000 AEQ im Monat, als
„Unternehmen“ ohne echte Ausgaben rund 2.860 AEQ im Monat. **Horten lohnt sich
in keiner Form.**

## 9. Was Läden brauchen, damit sie mitmachen

- **Kassenmodus in der App:** Betrag eingeben → QR-Code → Kunde scannt und
  zahlt → Beleg. Preise wahlweise in AEQ oder als Euro-Gegenwert zum aktuellen
  Kurs.
- **Buchhaltungs-Export (CSV):** jede Zahlung mit Datum, Betrag in AEQ und
  **Euro-Wert zum Zahlungszeitpunkt**. Ohne den nimmt kein Steuerberater es an.
- **Sofort-Ausstieg auf Wunsch:** Wer kein Kursrisiko tragen will, tauscht
  Einnahmen automatisch um (2 %). Die Wahl bleibt beim Laden.
- **Argumente für den Laden:** Die Zahlung ist für den Laden gebührenfrei, der
  Kunde zahlt 0,1 %. Kunden mit Grundeinkommen wollen es ausgeben, und Löhne
  lassen sich gebührenfrei in AEQ zahlen.

## 10. Was an der Kette gebaut werden muss

Alles hinter einer **Aktivierungshöhe** (wie `grant_staffel.go`), damit alte
Blöcke gleich nachgespielt werden. Da die Kette vor dem Launch bei null startet,
gibt es keine Altbestände umzustellen.

1. `AccountState`: Feld `Kontoart` (mensch / unternehmen / frei),
   `Verantwortliche []Adresse`, `AusgabenVormonat`, `AusgabenLaufenderMonat`,
   dazu bei Menschen `MonatsAusgaben` (Gebührenfreibetrag), `MonatsTausch` und
   `MonatsLohn` (Tausch-Freibetrag). Alle Zähler springen am Monatsanfang nach
   Blockzeit zurück, damit jeder Knoten gleich rechnet.
2. Neue Transaktionen: `unternehmen_eroeffnen`, `unternehmen_mitinhaber`,
   `unternehmen_schliessen`.
3. `enforceWealthCapLocked`: Unternehmen ausgenommen, freie Adressen auf
   1.000 AEQ.
4. `haltegebuehr.go`: Freibetrag, Stufen, sekundengenaue Verrechnung, Gutschrift
   ans Grundeinkommen. Nach dem Muster von `effectiveBalance`, aber ohne
   Schonfrist und ohne Zurücksetzen durch Aktivität.
5. `ueberweisungsgebuehr.go`: Gebühr nach Richtung (6.3) und der
   Monatsfreibetrag von 1.000 AEQ für Menschen.
5a. Demurrage für Menschen: Sparfreibetrag 5.000 AEQ, ohne Schonfrist, ohne
   Zurücksetzen durch Aktivität. Website, Whitepaper und Explorer beschreiben
   heute „0,5 % auf den Teil über 1.000 AEQ“ und müssen mit angepasst werden.
6. Swap AEQ → Stable: Ausstiegsabgabe mit Monatsfreibetrag pro Mensch (6.4).
7. Tests: Verteilen auf Adressen lohnt nicht, Gebühren je Richtung, Umweg über
   den Inhaber ist gedeckelt, Nachspielen ist deterministisch.
8. Explorer: Unternehmensregister. App: Kassenmodus und CSV-Export.

Grober Aufwand: Kette 1–2 Wochen, App-Kassenmodus 1 Woche.

## 11. Recht und offene Punkte (ehrlich)

- **Steuern:** Für Unternehmen sind AEQ-Einnahmen Betriebseinnahmen zum
  Euro-Wert am Zahlungstag. Deshalb ist der CSV-Export Pflicht.
- **MiCA / Finanzaufsicht:** Ob AEQ und der eingebaute Tausch unter die
  EU-Kryptoverordnung fallen, muss **vor echtem Geld** rechtlich geprüft werden.
  Das betrifft das ganze Projekt, nicht nur Unternehmen.
- **Echter Stablecoin:** Heute gibt es nur tUSD (Testwährung). Für echten
  Ausstieg braucht es einen regulierten Euro-Stablecoin (z. B. EURC) und eine
  Brücke. Das kommt erst nach der rechtlichen Prüfung.
- **Kursrisiko für Läden:** Solange AEQ klein ist, schwankt der Kurs. Der
  Sofort-Ausstieg (9.) ist die Antwort für vorsichtige Läden.
- **Die Zahlen sind Startwerte** (Freibetrag 5.000, 1 %/3 %, 2 %, 1.000/Monat).
  In der Pilotstadt messen, dann per Abstimmung der Menschen anpassen.

## 12. Einführung in Schritten

1. **Entscheidung** über die Zahlen in Abschnitt 13.
2. **Kette** bauen und testen (Abschnitt 10), vor dem Neustart bei null.
3. **App**: Kassenmodus und Export.
4. **Pilotstadt**: 5–10 Läden (Café, Bäcker, Hofladen, Friseur, Werkstatt), drei
   Monate, messen: Wie viel bleibt im Kreislauf, wie viel geht raus, wie viel
   landet im Grundeinkommen?
5. **Rechtliche Prüfung** und echter Stablecoin, dann breiter öffnen.

## 13. Zu entscheiden

| Frage | Vorschlag |
|---|---|
| Freibetrag Unternehmen | 2.000 AEQ Sockel + Netto-Ausgaben des Vormonats (je Gegenüber, ohne eigene Konten) |
| Haltegebühr | 1 %/Monat bis 3 × Freibetrag, 3 %/Monat darüber |
| Ausstiegsabgabe | 2 % (Menschen: erste 1.000 AEQ/Monat zu 0,1 %) |
| Obergrenze freie Adresse | 1.000 AEQ, 1 %/Monat ab dem ersten AEQ |
| Unternehmenskonten pro Mensch | höchstens 3 |
| Löhne/Entnahmen an Menschen | gebührenfrei |
| Wohin gehen alle Abgaben | 100 % Grundeinkommen |
| **Menschen:** gebührenfreie Ausgaben | 1.000 AEQ im Monat |
| **Menschen:** Sparfreibetrag (keine Demurrage) | 5.000 AEQ |
| **Menschen:** Demurrage darüber | 0,5 %/Monat |
| **Menschen:** Umtausch ohne Abgabe | erhaltener Lohn + 1.000 AEQ im Monat |

## Vorbilder

- **Chiemgauer** (Bayern, seit 2003): Regionalgeld mit Umlaufsicherung, 5 %
  Rücktauschgebühr zugunsten von Vereinen, hunderte teilnehmende Geschäfte.
- **WIR-Bank** (Schweiz, seit 1934): Verrechnungsgeld zwischen Unternehmen,
  stabilisierend in Krisen.
- **Wörgl** (Österreich, 1932): Schwundgeld mit 1 %/Monat. Das Geld lief so
  schnell um, dass die Gemeinde damit Straßen und Brücken baute, bis die
  Nationalbank es verbot.
