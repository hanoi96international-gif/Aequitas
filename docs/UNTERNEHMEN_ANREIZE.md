# Unternehmen: Warum sollte ein Laden AEQ annehmen?

Stand: 02.10.2026, zweiter Durchgang · Status: **Analyse und Vorschlag, nichts davon ist beschlossen
oder gebaut.** Ergänzt `docs/UNTERNEHMEN_KONZEPT.md`; dort stehen die geltenden
Regeln.

## In drei Sätzen

Die Regeln für Unternehmen sind gründlich durchdacht, **aber nur von der
Kostenseite**: Was kostet Horten, wie wird Missbrauch verhindert, wer zahlt wie
viel. Die Nutzenseite fehlt fast ganz: Heute kann sich ein Unternehmen ohne
Entwickler nicht einmal anmelden, es gibt keine Kasse, kein Verzeichnis und in
der Beta keinen echten Ausstieg. **Keine Gebühr und keine Regel erzeugt
Nachfrage nach AEQ; das tun nur Läden, die es annehmen, und Läden, die es
weitergeben können.**

> **Zweiter Durchgang (02.10.2026, abends).** Drei Vorschläge aus dem ersten
> Durchgang hielten der Gegenprüfung als Angreifer nicht stand und sind
> zurückgenommen oder herabgestuft:
>
> 1. *Ausstiegsabgabe auf das Startguthaben* (alt 4.1): Ein Verkauf von Mensch
>    zu Mensch gegen Euro per Bank läuft gar nicht über den Tausch auf der
>    Kette. 1.000 AEQ im Monat sind zwischen Menschen gebührenfrei, genau das
>    Startguthaben. Die Abgabe verschiebt den Verkauf nur. **Zurückgenommen**,
>    ersetzt durch 4.1 neu.
> 2. *Unternehmen ohne eigenen Schlüssel* (4a, Lücke 1): Ein gekauftes
>    Unternehmenskonto spart gegenüber vielen freien Adressen nur 20 AEQ im
>    Monat. Es ist kein Geldproblem, sondern ein Problem der ehrlichen Anzeige.
>    Und der Umbau verhindert den Verkauf nicht, er bündelt das Unternehmen nur
>    mit der Identität und macht diese wertvoller. **Herabgestuft.**
> 3. *Reparatur der Rückzahlungsregel* (4a, Lücke 2): Mit einem zweiten Freund
>    ist sie wieder offen. **Nicht bauen**, Konzepttext korrigieren.

---

## 1. Was ein Laden heute erlebt (Stand Code 02.10.2026)

| Was der Laden braucht | Stand | Wo |
|---|---|---|
| sich als Unternehmen anmelden | **nur über die API**, zwei Unterschriften (Unternehmensadresse und Mensch) als `personal_sign`, 5-Minuten-Fenster. Weder App noch Website bieten das an | `wirtschaft_api.go`, `POST /api/unternehmen/eroeffnen` |
| Geld annehmen | Kunde kann einen Zahlungslink (EIP-681) **scannen**. Einen Code **erzeugen** kann die App nicht | `Aequitas-App/lib/zahlungslink.ts` |
| sehen, dass bezahlt wurde | nur über den Kontostand | – |
| Buchhaltung | kein Export | – |
| gefunden werden | Register unter `/api/unternehmen`, aber nirgends sichtbar für Kunden | – |
| Euro-Gegenwert | keiner. tUSD ist Testgeld, der Pool winzig | `handleWirtschaftRegeln` |
| Kosten | **im Pilot null**: Eingänge gratis, Löhne gratis, unter 2.000 AEQ Guthaben kein Liegegeld | `wirtschaft.go` |

Die Kosten sind also kein Problem. Das Problem ist, dass nichts **zieht**.

## 2. Wie Komplementärwährungen sterben, und wo wir verwundbar sind

### 2.1 Das bekannte Muster

1. Menschen bekommen Geld, finden aber kaum Orte, es auszugeben.
2. Die wenigen Läden, die es annehmen, können es selbst **nicht weitergeben**
   (Lieferanten, Löhne, Miete). Es stapelt sich, sie tauschen es zurück.
3. Jeder Rücktausch ist ein Verkauf. Der Kurs fällt, die nächsten Läden winken ab.

Bristol Pound ist 2021 genau daran gescheitert (Punkt 2). Der Chiemgauer
überlebt seit 2003 trotz 5 % Rücktauschgebühr, weil die Läden ihn in der Region
weitergeben können **und** weil Kunden über die Vereinsförderung einen eigenen
Grund haben, ihn zu benutzen. WIR und Sardex funktionieren, weil sie bei den
Unternehmen und ihren Lieferketten anfangen, nicht bei den Kunden.

### 2.2 Unsere eigene Schwachstelle: Das Startguthaben ist sofort verkäuflich

- Jeder neue Mensch bekommt **1.000 AEQ** (`registrationGrant`).
- Jeder Mensch darf **3.000 AEQ im Monat ohne Abgabe** in Stable tauschen
  (`menschTauschFreiMonat`), und **1.000 AEQ im Monat gebührenfrei an andere
  Menschen** überweisen.
- Folge: **Das gesamte Startguthaben kann am Tag der Registrierung verkauft
  werden**, über den Tausch auf der Kette oder an jemanden, der Euro per Bank
  zahlt.
- Die Staffelung des Startguthabens (`grant_staffel.go`, 200 sofort und 800
  über 30 Tage) gibt es, sie ist aber bis 2100 abgeschaltet und nur für
  auffällige Registrierungen gedacht.

Solange es nur tUSD gibt, ist das egal. Sobald ein echter Euro-Stablecoin
angebunden ist, ist es der kürzeste Weg in die Spirale aus 2.1: Neue Menschen
verkaufen, weil sie (noch) nichts kaufen können, der Kurs fällt, Läden steigen
aus.

**Was dagegen nicht hilft:** eine Abgabe. Wer Geld geschenkt bekommt und nichts
dafür kaufen kann, verkauft es auch mit 2 % oder 5 % Abschlag, notfalls an der
Kette vorbei. Eine Gebühr hoch genug, um das zu bremsen, wäre unfair gegenüber
allen, die ehrlich tauschen müssen. **Was hilft:** Gründe zu bleiben, bevor der
Ausgang aufgeht (4.1).

## 3. Was ein Unternehmen wirklich will

| Wunsch | Was wir heute bieten | Was fehlt |
|---|---|---|
| **mehr Kunden** | Menschen mit Startguthaben und Grundeinkommen | ein Verzeichnis, in dem Kunden den Laden finden |
| **keine Kosten** | ✅ gratis annehmen, gratis Löhne, Sockel 2.000 AEQ | es sagt ihnen niemand so einfach |
| **kein Risiko** | ❌ Kurs schwankt, in der Beta gar kein Ausstieg | eine Art, das Risiko klein und selbstbestimmt zu halten |
| **kein Aufwand** | ❌ Anmeldung nur per API | Anmeldung, Kasse, Export in der App |
| **das Geld wieder loswerden** | 0 % an Menschen, 0,1 % an Unternehmen | Lieferanten und Mitarbeitende, die AEQ nehmen |

Ein Laden nimmt AEQ an, wenn ihm das **Kunden bringt** und ihn **wenig Risiko
und keinen Aufwand** kostet. Geld vom Protokoll braucht er dafür nicht.

## 4. Vorschläge

Jeder Vorschlag ist gegen den Kern geprüft: **kein neues Geld, kein Vorrecht
für Unternehmen, Regeln pro Mensch.** Was dagegen verstößt, steht in
Abschnitt 5.

### 4.1 Den echten Ausgang erst öffnen, wenn es Gründe zum Bleiben gibt

Die Beta hat einen Schutz, den wir nicht verschenken sollten: **Es gibt keinen
Umtausch in echtes Geld.** Ein Startguthaben, das man nicht verkaufen kann, wird
ausgegeben oder liegt. Das ist die Zeit, in der der Kreislauf wachsen muss.

- Die Anbindung eines echten Euro-Stablecoins (ohnehin erst nach der
  rechtlichen Prüfung, Konzept Abschnitt 11) wird **an Messwerte aus dem Pilot
  gebunden**, nicht an ein Datum. Vorschlag: wenn ein nennenswerter Teil der
  Startguthaben innerhalb von 30 Tagen bei Unternehmen ankommt und
  Unternehmen im Mittel den größeren Teil ihrer Einnahmen weitergeben (6.).
  Die Schwellen setzt ihr nach den ersten Pilotwochen.
- Bis dahin hat AEQ für einen Laden **keinen Euro-Wert, nur einen
  Kundenwert**. Darum ist die Teilannahme (4.6) am Anfang der wichtigste Hebel,
  nicht eine Randnotiz: Der Laden behandelt AEQ wie einen Gutschein der
  Gemeinschaft, mit einer Grenze, die er selbst setzt.
- Ehrlich: Das löst die Spirale nicht für immer. Sobald es einen Ausgang gibt,
  zählt nur noch, ob es mehr Gründe zum Bleiben als zum Gehen gibt. Darum
  kommen Kasse, Verzeichnis und Pilotkette (4.2 bis 4.7) zuerst.

Eine Abgabe auf frühen Umtausch (erster Durchgang) ist zurückgenommen, siehe
2.2.

### 4.2 Unternehmen in der App anmelden (App, Beta-Pilot)

Ohne das gibt es keinen Pilot, nur Einzelfälle mit Entwicklerhilfe.

- Neuer Bereich „Mein Unternehmen“ in der App: Name, Kategorie, fertig.
- Die Eröffnung braucht zwei Unterschriften: eine vom Unternehmensschlüssel
  und eine vom Menschen. Die App erzeugt den Unternehmensschlüssel auf dem
  Gerät (SecureStore) und unterschreibt damit; der Mensch unterschreibt mit
  seiner Wallet. Kein Schlüssel verlässt das Gerät.
- Danach dasselbe für Mitinhaber und Schließen.
- Zu klären vor dem Bau: Verlust des Geräts (verschlüsselter Export des
  Unternehmensschlüssels), keine Kopie auf einem Server. Der Umbau auf
  Unternehmen ohne eigenen Schlüssel ist nach dem zweiten Durchgang keine
  Voraussetzung mehr (4a, Lücke 1).

### 4.3 Kassenmodus (App, Beta-Pilot)

- Betrag eingeben → die App zeigt einen EIP-681-Code (`ethereum:<adresse>@1926?value=…`).
  Das Lesen dieser Codes kann die Kundenseite schon (`zahlungslink.ts`).
- Die App wartet auf den Eingang und zeigt „bezahlt“ mit Betrag und Absenderart
  (Mensch / Unternehmen). Wartezeit fest begrenzt.
- Tagesliste und **CSV-Export** (Datum, Betrag, Gegenkonto, Transaktion).
  Euro-Wert erst, wenn es einen echten Kurs gibt; bis dahin ehrlich leer.

### 4.4 „Wo kann ich AEQ ausgeben?“ (App und Website)

Das ist der eigentliche Kundenbringer und kostet uns fast nichts.

- Liste und Karte aller Unternehmen aus `/api/unternehmen`, filterbar nach
  Kategorie und Ort.
- Ort ist **freiwillig** und wird vom Unternehmen selbst angegeben
  (Postleitzahl oder Stadt reicht). Dafür braucht das Register ein optionales
  Feld oder eine eigene Transaktion; das ist keine Wirtschaftsregel, aber
  Konsenszustand und braucht dieselbe Sorgfalt (Längenlimit, Zeichensatz).
- Auf der Startseite der App für jeden neuen Menschen: „Hier kannst du dein
  Startguthaben ausgeben.“

### 4.5 Weitergabequote: Ruf statt Rabatt

Aus den öffentlichen Monatssummen lässt sich je Unternehmen ablesen, wie viel
vom eingenommenen AEQ es **im Netz weitergibt** (Löhne, Lieferanten, Menschen)
und wie viel es **aussteigt**.

- Im Verzeichnis sichtbar, z. B. „gibt 85 % weiter“.
- Sortierung: Wer weitergibt, steht weiter oben.
- Kein Geld, kein Vorrecht, nur Sichtbarkeit für das Verhalten, das wir wollen.
  Kunden, denen das wichtig ist, gehen genau dorthin.

Teilweise sind die Daten schon da: `/api/unternehmen` zeigt je Monat
`einnahmen`, `lohn_gezahlt` und `entnahmen`. Es fehlen die Zahlungen an andere
Unternehmen und die Summe der Ausstiege in Stable; beide liegen in der
Buchführung und müssten in die Monatssummen aufgenommen werden. Zu klären: ab
welcher Mindestmenge die Quote gezeigt wird, damit Einzelzahlungen nicht
täuschen.

### 4.6 Teilannahme: am Anfang der wichtigste Hebel (kein Code)

Das Risiko eines Ladens ist sein AEQ-Bestand, nicht die einzelne Zahlung. Die
einfachste Bremse ist, dass der Laden **selbst** festlegt, wie viel er annimmt:

- „Bei uns bis 20 % des Einkaufs in AEQ“ oder „bis 500 AEQ im Monat“.
- WIR arbeitet seit 1934 so: Teilzahlungen in WIR, Rest in Franken.
- Den Preis in AEQ legt der Laden fest, nicht das Protokoll. Ein offizieller
  Richtkurs wäre ein Kopplungsversprechen, das wir nicht halten können
  (Konzept 6.7).

Gehört in die Pilot-Unterlagen und auf die Unternehmensseite der Website.

### 4.7 Pilot als Kette, nicht als Sammlung von Läden

Steht schon im Konzept (14.8) und ist der wichtigste Punkt: Bäckerei, Mühle, Hof,
Café **gleichzeitig**, dazu mindestens ein Betrieb, der einen Teil der Löhne in
AEQ zahlt (freiwillig, für Mitarbeitende, die das wollen). Erst wenn ein Laden
AEQ **weitergeben** kann, ist es für ihn mehr als ein Werbegag.

Konkret für die Gewinnung: Den ersten Betrieb der Kette nicht fragen „Nehmt ihr
AEQ an?“, sondern „Würde euer Lieferant AEQ annehmen, wenn ihr damit bezahlt?“
und beide zusammen anwerben.

### 4.8 Klartext für Läden (Website, kein Code)

Die Unternehmensseite erklärt heute das Regelwerk. Ein Laden braucht vier Sätze:

1. **Annehmen kostet euch nichts.** Keine Gebühr auf Zahlungen von Kunden.
2. **Löhne in AEQ kosten nichts.**
3. **Bis 2.000 AEQ Guthaben zahlt ihr nie etwas**, darüber erst ab
   anderthalb Monatsumsätzen.
4. **Ihr bestimmt, wie viel ihr annehmt.** (4.6)

Und ehrlich für die Beta: **Es gibt noch keinen Umtausch in Euro.** Wer
mitmacht, macht es für die Kundschaft und für die Idee, mit einer Grenze, die
er selbst setzt.

## 4a. Wie prüft man Unternehmen weltweit? Gar nicht, man prüft Menschen

### Der Grundsatz

Menschen werden geprüft: heute mit der Gesichtsprüfung, zum richtigen Launch
mit der Iris. **Unternehmen werden nicht geprüft und sollen es nicht werden.**
Es gibt rund 200 Länder mit eigenen Handelsregistern, viele davon nicht
öffentlich oder nicht verlässlich. Wer sie prüft, ist eine zentrale Stelle, die
entscheidet, wer mitmachen darf. Das passt nicht zu Aequitas.

Stattdessen gilt:

1. **Ein Unternehmen ist ein Hut, den ein geprüfter Mensch aufsetzt.** Jedes
   Unternehmenskonto hängt an mindestens einem verifizierten Menschen. Die Iris
   des Menschen ist die einzige Prüfung, die es braucht. Weil jeder Mensch nur
   einmal existiert, sind die Grenzen pro Mensch echt: höchstens 3 Unternehmen,
   die Gründungsphase einmal im Jahr, je 9.000 AEQ Umsatz pro Quartal und Firma.
2. **Ein Unternehmenskonto darf nichts hergeben, das sich auszunutzen lohnt.**
   Jeder Vorteil gegenüber einem Menschen hat einen Preis, der höher ist als
   der Vorteil für jemanden, der kein echtes Geschäft hat.

| Was ein Unternehmenskonto mehr hat als ein Mensch | Preis dafür |
|---|---|
| keine Grenze von 25.000 AEQ | Liegegeld 0,5–1 %/Monat über 1,5 Monatsumsätzen |
| Löhne an Menschen gebührenfrei | Menschen zahlen beim Einkauf (über 1.000 AEQ/Monat) die 0,1 % schon |
| Sockel 2.000 AEQ frei | höchstens 3 je Mensch, also 6.000 AEQ |
| – (kein Grundeinkommen, keine Stimme, kein Tausch-Freibetrag) | – |

Die Iris macht Unternehmen also nicht prüfbar. Sie macht die **Menschen hinter
ihnen** zählbar. Der zweite Anker ist wichtiger, als er aussieht: **Liegegeld
und Grenzen wirken auf jedes AEQ, egal wer es hält.** Darum muss niemand wissen,
wer hinter einem Konto steht. Der richtige Vergleich für ein
Unternehmenskonto ist deshalb nicht der Mensch, sondern die freie Adresse, die
jeder ohne Prüfung anlegen kann (Lücke 1 rechnet das nach).

**Was das Netz nicht ersetzt:** Wo echtes Geld ein- und ausgeht
(Euro-Stablecoin, Börsen), gelten Regeln gegen Geldwäsche, und dort wird
geprüft. Ob das für Aequitas reicht oder ob der eingebaute Tausch selbst
Pflichten auslöst, ist Teil der MiCA-Prüfung (Konzept Abschnitt 11) und muss
vor echtem Geld rechtlich geklärt sein. Der Grundsatz „das Netz prüft
Menschen, nicht Unternehmen“ ist eine Entscheidung über das Protokoll, keine
Rechtsauskunft.

### Lücke 1: Verantwortung ist heute nur ein Eintrag

Ein Unternehmenskonto ist eine gewöhnliche Adresse mit **eigenem privaten
Schlüssel**. Der Mensch unterschreibt nur einmal bei der Eröffnung. Danach
bewegt, wer den Unternehmensschlüssel hat, das Geld. Der eingetragene Mensch
kann das Konto weder sperren noch verlassen.

**Was ein Käufer davon hat, nachgerechnet** (Liegegeld ohne Umsatz gegen 1 %
je freie Adresse, `liegegeldFuerStand`):

| anonym halten | als gekauftes Unternehmen | auf freien Adressen à 250 AEQ |
|---|---|---|
| 10.000 AEQ | 80 AEQ/Monat | 100 AEQ/Monat (40 Adressen) |
| 100.000 AEQ | 980 AEQ/Monat | 1.000 AEQ/Monat (400 Adressen) |
| 1.000.000 AEQ | 9.980 AEQ/Monat | 10.000 AEQ/Monat (4.000 Adressen) |

Anonym große Beträge halten geht **schon heute ohne jeden Menschen**, zum
gleichen Preis. Das gekaufte Unternehmen spart 20 AEQ im Monat und etwas
Aufwand. **Wirtschaftlich ist das keine Lücke.** Die Regeln greifen, weil sie
auf jedes AEQ wirken, nicht weil wir wissen, wer es hält. Das ist der
Grundsatz aus `WHO_MAY_HOLD_AEQ.md`, und er trägt.

**Was bleibt:** Das Register zeigt einen verantwortlichen Menschen, der vielleicht
nichts mehr zu sagen hat. Wo Kunden dem vertrauen (Verzeichnis, 4.4), ist das
eine falsche Anzeige.

**Warum der Umbau auf „Unternehmen ohne eigenen Schlüssel“ (erster Durchgang)
das nicht löst:**
- Ein Mensch kann seine **eigene Wallet** verkaufen. Es gibt heute keinen Weg,
  eine Identität per Biometrie auf eine neue Wallet umzuziehen, also kann der
  Verkäufer sie auch nicht zurückholen. Mit dem Umbau wären die drei
  Unternehmen im Paket dabei. Die Identität würde **wertvoller zum Kaufen**,
  samt Grundeinkommen und Stimme. Das ist schlechter als heute.
- Ohne Verkauf geht es mit einem Strohmann, der jeden Auftrag unterschreibt.
- Der Umbau betrifft Kette (neue Auftragsart, Nonces, Schnellpfad) und App
  (Löhne und Lieferanten nur über unterschriebene Aufträge, kein MetaMask).
  Großer Aufwand für wenig Schutz.

**Was stattdessen:**
- **Ehrliche Anzeige:** Im Verzeichnis „bei Eröffnung eingetragen von einem
  verifizierten Menschen“, nicht „ein Mensch steht dafür ein“.
- **Austreten:** Ein eingetragener Mensch kann sich jederzeit austragen. Ist
  danach niemand mehr eingetragen, fällt das Konto auf die Regeln einer freien
  Adresse zurück (Übergangsfrist zum Auszahlen, Regel zu entscheiden). So kann
  sich niemand mit seinem Namen an einem Konto festhalten lassen, das er nicht
  mehr kontrolliert. Kleine Konsensänderung.
- **Das eigentliche Mittel gegen Identitätsverkauf** ist allgemein, nicht
  unternehmensbezogen: ein **Umzug per Iris**. Wer mit seiner Iris
  nachweist, dass er es ist, zieht seine Identität (Grundeinkommen, Stimme,
  Unternehmen) auf eine neue Wallet um, mit Wartefrist. Dann ist jede
  verkaufte Identität für den Käufer wertlos, weil der Verkäufer sie jederzeit
  zurückholen kann. Der Vergleichsdienst erkennt heute schon, dass eine Person
  bereits registriert ist; statt abzulehnen, könnte er den Umzug anbieten.
  Offene Fragen: Was passiert mit dem Guthaben auf der alten Wallet, und wie
  schützt man jemanden, der zum Scan gezwungen wird? Für den Iris-Launch
  durchdenken, nicht für die Beta.

### Lücke 2: Rückzahlung vor dem Einkauf (im Code nachgewiesen)

Die Schutzregel „Was das Unternehmen demselben Menschen zurückzahlt, hebt dessen
gezählte Einkäufe auf“ wirkt nur **in einer Richtung**: `rueckzahlungLocked`
zieht nur ab, was schon gezählt ist. Zahlt die Firma **zuerst** und kauft der
Freund **danach** mit demselben Geld ein, zählt der Einkauf voll.

Nachgestellt am 02.10.2026 (Testkonten, Phase-0-Grenze 5.000):

| Reihenfolge | gezählter Monatsumsatz |
|---|---|
| Freund kauft für 3.990 ein, Firma zahlt 3.990 zurück | **0 AEQ** (Regel wirkt) |
| Firma zahlt 4.000 „Lohn“, Freund kauft für 3.990 ein | **3.740 AEQ** (Regel wirkt nicht) |

Mit echten Zahlen: je Freund 9.000 AEQ pro Quartal, also 3.000 AEQ Monatsumsatz
und 4.500 AEQ mehr Freibetrag. Ersparnis bis 45 AEQ im Monat, Kosten rund
8 AEQ im Quartal (seine Überweisungsgebühr). Der Freund braucht dafür **kein
eigenes Geld**, die Firma stellt es. Das Konzept (14.3) setzte voraus, dass
Freunde jedes Quartal eigenes Geld einzahlen.

**Warum die naheliegende Reparatur nicht hilft:** Rechnet man frühere
Zahlungen der Firma gegen spätere Einkäufe desselben Menschen, nimmt das Geld
einfach einen Umweg: Firma → Freund 1 → Freund 2 → Firma. Freund 2 hat von
der Firma nichts bekommen, sein Einkauf zählt voll. Zwei Freunde tauschen die
Rollen, und es ist wie vorher. Kosten des Umwegs: 0,1 % über 1.000 AEQ im
Monat.

**Was wirklich begrenzt**, und das reicht: je Mensch höchstens 9.000 AEQ pro
Quartal und Firma, und jeder Mensch existiert nur einmal. Jeder eingespannte
Freund bringt der Firma höchstens rund 45 AEQ Ersparnis im Monat. Wer so 1.000
AEQ im Monat sparen will, braucht über 20 Menschen, die jedes Quartal
mitmachen, öffentlich sichtbar in den Lohnsummen.

**Vorschlag:** Nicht bauen. Im Konzept (14.3) den Satz korrigieren, der eine
Wirkung verspricht, die die Regel nicht hat. Die Regel selbst bleibt: Sie
verhindert den plumpsten Fall (einkaufen und dasselbe Geld zurückbekommen).

### Kleiner, aber zu wissen

- **Name und Kategorie sind selbst angegeben.** Im Verzeichnis (4.4) könnte
  sich jeder „Aldi“ nennen. Vorschlag: Namen als „selbst angegeben“ zeigen,
  dazu ein freiwilliger, **von jedem nachprüfbarer** Nachweis über die eigene
  Website (Datei unter `/.well-known/aequitas.txt` mit der
  Unternehmensadresse). Keine Stelle entscheidet, jeder Knoten und jede App kann
  es selbst prüfen. Er bringt nur Vertrauen, keinen wirtschaftlichen Vorteil.
- **Ein Menschenstatus lässt sich nie entziehen** (kein `IsHuman = false` im
  Code). Fliegt eine gefälschte Identität später auf, bleiben auch ihre
  Unternehmen. Das ist keine Lücke der Unternehmensregeln, sondern der
  Personenprüfung. Mit der Iris wird sie kleiner, braucht aber trotzdem eine
  Regel.
- **Schon bedacht:** Eine Unternehmensadresse, die sich später als Mensch
  registriert, fällt unter die Grenze für Menschen
  (`TestUnternehmenDasMenschWirdBehaeltGrenze`).

## 5. Was wir bewusst nicht machen

| Idee | Warum nicht |
|---|---|
| Startguthaben oder Grundeinkommen für Unternehmen | neues Geld ohne Menschen dahinter; jeder würde Scheinfirmen gründen |
| Cashback für Kunden aus dem Grundeinkommens-Topf | nimmt allen Menschen, um einigen Läden Kunden zu bringen |
| Gebührenvorteil für „Partner-Läden“ | Vorrecht, das jemand vergeben müsste; zentrale Stelle |
| Fester Euro-Kurs für AEQ | braucht Reserven, die es nicht gibt; Versprechen, das bricht |
| Liegegeld für Unternehmen senken | ist im Pilot ohnehin null (Sockel). Die Grenze ist nicht das Problem |

## 6. Was der Pilot messen muss

Dieselben Zahlen, an die 4.1 den echten Ausgang bindet:

1. Anteil der Startguthaben, der in den ersten 30 Tagen **bei Unternehmen**
   ankommt, und Anteil, der **aussteigt**.
2. Weitergabequote je Unternehmen (4.5) und im Mittel.
3. Wie oft ein AEQ im Monat den Besitzer wechselt (Umlaufgeschwindigkeit).
4. Zahl der Unternehmen mit Eingängen von mindestens 10 verschiedenen Menschen
   im Monat.

Alles ist aus den Blöcken ablesbar, ohne neue Datenerhebung.

## 7. Empfohlene Reihenfolge

| # | Was | Art | Wann |
|---|---|---|---|
| 1 | Klartext und Teilannahme auf der Website (4.6, 4.8) | Text | **zur Beta** |
| 2 | Anmeldung in der App (4.2), mit heutigem Zwei-Schlüssel-Verfahren | App | **zur Beta**, sonst kein Pilot |
| 3 | Kassenmodus mit CSV (4.3) | App | Pilotstart |
| 4 | Verzeichnis „Wo kann ich AEQ ausgeben?“ (4.4) mit ehrlicher Anzeige (4a) | App, Website, Register-Feld | Pilotstart |
| 5 | Weitergabequote (4.5) | API, App | im Pilot |
| 6 | Austreten aus einem Unternehmen (4a, Lücke 1) | Konsens, klein | im Pilot |
| 7 | Echten Ausgang an Messwerte binden (4.1) | Entscheidung | **vor jeder Anbindung von echtem Stable** |
| 8 | Umzug per Iris (4a) | Konzept | für den Iris-Launch |
| – | Konzepttext 14.3 zur Rückzahlungsregel korrigieren (4a, Lücke 2) | Text | bald |

Die Beta selbst muss nicht warten. Menschen können ab Tag 1 mitmachen;
Unternehmen kommen im begleiteten Pilot dazu, sobald 2 und 3 stehen.
