# Aequitas: Die Grundpfeiler

Stand 02.10.2026. Dieses Dokument legt das Fundament, auf dem alle anderen
Regeln stehen. Es fasst Forschung, Praxiserfahrung anderer Systeme und den
Stand des Codes zusammen und benennt für jeden Pfeiler einen **Grundsatz**,
die **offenen Entscheidungen** und woran man im Pilot misst, ob er trägt.

Es ersetzt keine der Detailschriften, sondern ordnet sie:
`UNTERNEHMEN_KONZEPT.md` (Regeln), `WIRTSCHAFT_REIFEPRUEFUNG.md`,
`WIRTSCHAFT_ZAHLENPRUEFUNG.md`, `WIRTSCHAFT_LITERATUR.md`,
`RECHTSFRAGEN_UNTERNEHMEN.md`, `AUDIT_VON_NULL_2026-10-02_BEWERTUNG.md`.

**Grenze:** Studien wurden über Zusammenfassungen und Berichte ausgewertet,
nicht im Original (Netzwerkrichtlinie der Arbeitsumgebung). Rechtliche
Aussagen sind Hinweise aus Fachveröffentlichungen, keine Rechtsauskunft.
Beides muss vor echtem Geld eine Fachperson bestätigen.

---

## Überblick

| # | Pfeiler | Kernfrage | Stand | Trägt heute? |
|---|---|---|---|---|
| 1 | Identität | Ist jeder Mensch genau einmal da, und bleibt er es? | Gesicht + ZK, Iris geplant | **nein, das ist der Engpass** |
| 2 | Geldschöpfung und Verteilung | Wer bekommt wann wie viel? | 1.000 AEQ einmalig, Abgaben an alle | im Grundsatz ja |
| 3 | Wert und Annahme | Warum nimmt jemand AEQ? | ein Mensch auf der Kette, kein Ausgang | **noch nicht** |
| 4 | Umlauf und Haltegebühr | Wie bleibt Geld in Bewegung? | 0,5 % / 1 % im Monat | ja, im erprobten Bereich |
| 5 | Unternehmen | Wie nehmen Firmen teil, ohne Vorrecht? | Regeln gebaut, nichts in der App | Regeln ja, Nutzung nein |
| 6 | Kredit | Wie kommt Liquidität ohne Banken? | nichts | **fehlt** |
| 7 | Grundeinkommen | Was ist es, was kann es sein? | aus Abgaben, klein | ehrlich zu benennen |
| 8 | Mitbestimmung | Wer ändert die Regeln, wie? | nicht gebaut | **fehlt** |
| 9 | Betrieb und Finanzierung | Wer bezahlt das alles? | ungeklärt | **fehlt** |
| 10 | Recht | Darf es so laufen? | Fragen gesammelt | **ungeklärt** |
| 11 | Sicherheit und Konsens | Rechnet jeder Knoten alles selbst nach? | ein Erzeuger, K-2 offen | Übergang |

Die Pfeiler hängen voneinander ab. Die Reihenfolge am Ende (Abschnitt 12)
folgt dieser Abhängigkeit.

---

## 1. Identität

**Warum es der erste Pfeiler ist.** Jede faire Regel bei Aequitas lautet „je
Mensch“: Startguthaben, Grenze, Freibeträge, Stimmrecht, Kundschaft als
Nachweis für Unternehmen. Alle sind nur so stark wie die Gewissheit, dass ein
Mensch genau einmal da ist.

**Forschung und Praxis**
- Es gibt **keine ideale Form** des Personennachweises. Buterin (2023) nennt
  für biometrische Verfahren vier Risiken: Privatsphäre, Zugänglichkeit,
  Zentralisierung, Sicherheit (gehackte Telefone, **Zwang** zum Scan mit
  fremdem Schlüssel, künstliche „Menschen“). Er empfiehlt, Verfahren zu
  kombinieren: soziale Bürgschaft, allgemeine und spezielle Biometrie.
- Übersichtsarbeiten zu Personennachweisen (Siddarth u. a. 2020; Ford 2020)
  vergleichen Pseudonym-Partys (Anwesenheit zur selben Zeit), Idena (Rätsel
  unter Zeitdruck, probabilistisch), BrightID (sozialer Graph). Jedes hat
  Lücken; keines löst Verkauf.
- **Verkauf ist real:** Verifizierte World-IDs wurden 2023 für rund 30 US-Dollar
  gehandelt. World bietet seit 2024 nur das Löschen mit sechs Monaten
  Sperrfrist, kein Zurückholen einer verkauften Identität.

**Stand bei Aequitas:** Gesichtsprüfung im Testmodus, Vergleichsdienste mit
Quorum, Zero-Knowledge-Beweis, Herkunftsnotiz. Gestaffeltes Startguthaben mit
zweiter Lebendigkeitsprüfung ist gebaut, aber bis 2100 abgeschaltet. Der
Menschenstatus lässt sich nicht entziehen.

**Grundsatz**
> Kein Wert, der an „einen Menschen“ gebunden ist, darf größer sein als das,
> was eine gefälschte oder gekaufte Identität kostet, solange wir das nicht
> verhindern können.

**Folgerungen**
1. Das gestaffelte Startguthaben **aktivieren**, bevor AEQ echten Wert hat:
   Eine gekaufte Identität bringt dann zunächst nur 200 AEQ.
2. **Zurückholen statt nur Löschen** entwerfen (Umzug per Iris): Wer mit seinem
   Merkmal nachweist, dass er es ist, zieht seine Identität auf eine neue
   Wallet. Das macht gekaufte Identitäten für Käufer wertlos. Offene
   Schutzfrage: Zwang zum Scan (Buterin). Vorschlag: Wartefrist, in der die
   alte Wallet widersprechen kann, und eine Bürgschaft durch Menschen, die die
   Person kennen.
3. Eine Regel, wann ein Menschenstatus **entzogen** wird (nachgewiesene
   Doppelregistrierung), mit Widerspruchsweg.
4. Biometrie mit einem **zweiten, unabhängigen Verfahren** kombinieren (z. B.
   Bürgschaft durch bereits verifizierte Menschen), wie Buterin empfiehlt.

**Messen:** Doppelungen, die der Vergleichsdienst findet; Anteil gestaffelter
Registrierungen; Widersprüche.

---

## 2. Geldschöpfung und Verteilung

**Forschung und Praxis**
- **Circles** gibt jedem Menschen laufend neues Geld (eine Einheit je Stunde)
  und belastet alles mit 7 % im Jahr. Ergebnis: spürbares, laufendes
  Grundeinkommen; liegendes Geld schwindet.
- **GoodDollar** finanziert Grundeinkommen aus Zinserträgen gestakten Kapitals.
  Ergebnis: abhängig von Zinsen und Spendern.
- **Bargeldtransfers wirken:** In Kenia erzeugten einmalige Zahlungen von etwa
  1.000 US-Dollar je Haushalt einen lokalen Multiplikator von 2,5 bei kaum
  Preissteigerung (Egger u. a., Econometrica 2022, Frisch-Medaille 2024).
- **Aber verschenktes Komplementärgeld liegt oft:** Bei Sarafu lagen 58 % still
  (Mattsson u. a. 2022/2023).

**Stand bei Aequitas:** Je Mensch einmal 1.000 AEQ; Geldmenge = Menschen ×
1.000; kein anderes Geld entsteht. Grundeinkommen nur aus Abgaben.

**Grundsatz**
> Geld entsteht nur für Menschen, für jeden gleich. Niemand bekommt mehr, weil
> er früher kam, mehr hat oder ein Unternehmen ist.

**Die Grundsatzfrage, die offen ist:** Einmal 1.000 (heute) oder laufend wie
Circles? Einmalig ist einfacher und hält die Menge je Mensch fest, liefert aber
ein kleines Grundeinkommen und lässt Startguthaben liegen. Laufend liefert ein
echtes Grundeinkommen, braucht aber eine Haltegebühr auf alles, auch auf den
fairen Anteil. Das ist eine Entscheidung über das Wesen des Geldes und gehört
zur Abstimmung (Pfeiler 8), nicht in eine Feinjustierung.

**Messen:** Anteil der Startguthaben, der in 30 / 90 Tagen ausgegeben wird.

---

## 3. Wert und Annahme

**Forschung und Praxis**
- „Jeder kann Geld schaffen; das Problem ist, es angenommen zu bekommen“
  (Minsky 1986). Staatliches Geld wird angenommen, weil Steuern darin zu
  zahlen sind. **Aequitas hat keine solche Pflicht.** Sein Wert entsteht nur
  aus einem Netz von Menschen und Läden, die es annehmen.
- **Greshamsches Gesetz** (Godschalk 2012): Wer zwei Gelder hat, gibt das
  schwächere zuerst aus. Für AEQ heißt das: Es wird gern ausgegeben, aber wer es
  bekommt, will es auch schnell loswerden. Ohne Weitergabe endet es im Ausstieg.
- **Bristol Pound:** scheiterte an geringer Weitergabe und Kosten.
  **Chiemgauer:** überlebt, weil Läden weitergeben und Kunden einen Grund haben
  (Vereinsförderung).
- **Lokaler Multiplikator:** Barcelonas Komplementärwährung REC, mit der
  Sozialleistungen gezahlt wurden, erreichte nach 13 Monaten einen
  LM3-Multiplikator von 2,09 gegen 1,94 beim Euro. Ein messbarer, aber kleiner
  Unterschied.
- **Dünne Liquidität ist angreifbar:** In kleinen Pools verschiebt schon ein
  kleiner Handel den Kurs; berühmt ist DAI bei 1,30 $ mit 89 Mio. $
  Liquidationen. Der interne Pool hält heute rund 1 AEQ.

**Grundsatz**
> AEQ hat keinen versprochenen Kurs. Sein Wert ist, was Menschen und Läden
> dafür geben. Keine Regel darf an einem Kurs aus dem eigenen Pool hängen.

**Folgerungen**
1. Den **echten Ausgang** (Euro-Stablecoin) erst öffnen, wenn der Pilot
   Weitergabe zeigt (`UNTERNEHMEN_KONZEPT.md` 10.1).
2. Keine Regel, kein Anzeigewert für Pflichten an den Poolkurs koppeln (heute
   eingehalten: Grenzen sind Vielfache des fairen Anteils).
3. Pilot als **dichtes Netz** mit Lieferketten.

**Messen:** Weitergabequote der Läden; LM3 im Pilot; Anteil, der aussteigt.

---

## 4. Umlauf und Haltegebühr

**Forschung und Praxis**
- Erprobter Bereich: Gesell 5,2 %/Jahr, Chiemgauer 8 %, Circles 7 %, Wörgl 12 %.
  Fishers 2 % pro Woche (65 %/Jahr) wurden 1933 nicht angenommen.
- Die **Höhe** treibt den Umlauf kaum (Godschalk); die Gebühr ist eine Bremse
  gegen Horten, kein Motor.
- Kritik (Rösl): Wer Schwundgeld hält, hält weniger davon; schnellerer Umlauf
  ist nicht automatisch mehr Wirtschaft.
- WIR begann mit Haltegebühr und gab sie 1948 auf; heute arbeitet WIR mit
  niedrigen Zinsen.

**Stand:** Menschen 0,5 %/Monat über 5.000 AEQ; Unternehmen 0,5 % / 1 %
über 1,5 / 3 Monatsumsätzen; freie Adressen 1 % ab dem ersten AEQ.

**Grundsatz**
> Die Haltegebühr schützt vor Horten. Sie trifft nie den fairen Anteil eines
> Menschen und nie das, was ein Betrieb für seinen normalen Lauf braucht.

**Bewertung:** im erprobten Bereich, nicht erhöhen. Die Unternehmensgrenze von
1,5 Monatsumsätzen wird durch Sardex gestützt (Guthaben höchstens etwa 10 % des
Jahresumsatzes, rund 1,2 Monate). Sie wird erst knapp, wenn AEQ die
Hauptwährung eines Betriebs ist (`WIRTSCHAFT_ZAHLENPRUEFUNG.md` 3.3).

**Messen:** Umlaufgeschwindigkeit, Haltedauern (Methode wie Mattsson u. a.),
Rücklagen der Pilotbetriebe in AEQ.

---

## 5. Unternehmen

**Forschung und Praxis**
- Firmen bleiben aus vier Gründen: **Kundschaft, Weitergabe, Liquidität,
  Zugehörigkeit**. Hindernisse: das Geld nicht weitergeben können, hohe
  Abwicklungskosten.
- Sardex: Guthaben je Firma gedeckelt (etwa 10 % des Jahresumsatzes),
  Kreditlinie nach eingebrachter Leistung, Betreuung durch „Broker“, Ausfälle
  etwa wie bei italienischen Banken (5 bis 10 %).
- WIR: in Krisen gegenläufig zur Konjunktur, weil es Liquidität für kleine
  Firmen schafft (Stodder 2009; Stodder & Lietaer 2016).

**Stand:** Regeln für Unternehmenskonten gebaut und geprüft; zweite Stufe ab
15.10.2026; echte Kundschaft als Zahl. Anmeldung, Kasse, Verzeichnis fehlen.

**Grundsatz**
> Ein Unternehmen wird nicht geprüft, sondern an seinem Verhalten erkannt. Das
> Etikett bringt nichts, was nicht durch nachgewiesene Kundschaft verdient ist.
> Kein Unternehmen steht schlechter als ein Mensch, keins besser.

**Erkennen in drei Nachweisen:** Kundschaft verschiedener verifizierter Menschen
(gebaut), Bürgen (Entwurf), Urkunde wo vorhanden (vLEI, EU-Business-Wallet).
Keiner bringt Geld.

**Messen:** Unternehmen mit mindestens 10 verschiedenen zahlenden Menschen im
Monat; Weitergabequote; Rücklagen in AEQ.

---

## 6. Kredit

**Forschung und Praxis**
- **WIR:** Kreditlinien gegen Sicherheiten, zu niedrigen Zinsen, weil auf
  WIR-Guthaben keine Zinsen gezahlt werden.
- **Sardex:** zinsfrei, Kreditlinie je Firma nach ihrer eingebrachten Leistung,
  Guthabendeckel, Betreuung; Ausfälle 5 bis 10 %.
- **LETS** ohne Grenzen scheitert an Trittbrettfahrern; Abhilfe sind
  Kreditgrenzen nach der eigenen Handelsgeschichte.
- **JAK:** zinsfrei gegen vorheriges Sparen, Kosten über Gebühren (effektiv
  etwa 2,5 %).
- **Mikrokredit:** sechs randomisierte Studien auf vier Kontinenten zeigen
  „bescheiden positive, nicht transformative“ Wirkungen (Banerjee, Karlan,
  Zinman 2015). Gruppenhaftung hilft nicht überall.
- **Credit Commons** (Slater): Verrechnungsnetze bleiben selbstständig und
  verrechnen untereinander; Trustlines: Kredit zwischen Menschen, die sich
  vertrauen.

**Stand:** kein Kredit; Verträge können nach heutigen Regeln keinen Kreditpool
halten (freie Adresse, 250 AEQ).

**Grundsatz**
> Kredit ist zinsfrei, begrenzt durch nachgewiesene Geschichte, und niemand
> verliert sein Grundeinkommen oder seinen fairen Anteil an eine Schuld.

**Entwurf (zur Abstimmung und Rechtsprüfung)**

| Stufe | Für wen | Grenze | Kosten | Bei Ausfall |
|---|---|---|---|---|
| Zahlungsziel | Firma an Firma | Rechnungsbetrag | keine | öffentlich sichtbar |
| Gegenseitiger Kredit | Unternehmen | **1 % bis 10 % des Jahresumsatzes aus verifizierter Kundschaft** (Sardex-Spanne), Start niedrig | kleine feste Gebühr ans Grundeinkommen | Rückzahlung aus künftigen Eingängen; Ausfall trägt ein Ausfalltopf aus den Gebühren |
| Darlehen zwischen Menschen | Menschen | höchstens 1 × fairer Anteil offen | zinsfrei | höchstens 20 % je Eingang; nie Grundeinkommen; Verjährung nach 3 Jahren |

Der gegenseitige Kredit lässt vorübergehend Guthaben ohne Startguthaben
dahinter entstehen (Summe aller Konten bleibt gleich). Das berührt Pfeiler 2
und braucht deshalb die Abstimmung der Menschen.

**Messen (wenn eingeführt):** Ausfallquote, Auslastung der Linien, Umsatz in
Krisenzeiten.

---

## 7. Grundeinkommen

**Forschung und Praxis**
- Bargeld wirkt lokal (Kenia: Multiplikator 2,5). In den USA (OpenResearch,
  1.000 $ im Monat über drei Jahre) arbeiteten Empfänger etwa 1,4 Stunden pro
  Woche weniger; Stress sank kurzfristig, die körperliche Gesundheit änderte
  sich nicht.
- Komplementärwährungen als Auszahlungsweg (REC Barcelona) halten etwas mehr
  Geld vor Ort, der Unterschied ist klein.

**Stand:** aus Abgaben, geschätzt 1 bis 5 AEQ im Monat je Mensch; die echte
Zahl steht jetzt in `/api/wirtschaft/regeln`.

**Grundsatz**
> Das Grundeinkommen wird so groß genannt, wie es ist. Fair ist Aequitas durch
> die gleiche Ausgabe und die Grenze, nicht durch die Höhe der Ausschüttung.

**Folgerung:** Auf Website und in der App die gemessene Zahl zeigen, nicht das
Wort allein. Eine spürbare Höhe ist nur mit Pfeiler 2 (laufende Ausgabe) zu
haben.

---

## 8. Mitbestimmung

**Forschung und Praxis**
- **Ostroms acht Prinzipien** für Gemeingüter: klare Grenzen, Regeln passend
  zur Lage, Mitbestimmung der Betroffenen, Überwachung durch sie selbst,
  abgestufte Sanktionen, günstige Konfliktlösung, Anerkennung durch höhere
  Ebenen, verschachtelte Zuständigkeiten.
- **Token-Abstimmungen** werden zur Plutokratie; in den meisten DAOs halten die
  zehn größten Halter 60 bis 80 % der Stimmen. Quadratisches Abstimmen hilft
  nur mit verlässlichem Personennachweis, sonst teilt ein Reicher sein Geld auf
  viele Adressen.

**Was Aequitas besonders kann:** Mit einem Personennachweis ist **eine Person,
eine Stimme** möglich, und quadratisches Abstimmen wird erst dadurch
sinnvoll.

**Stand:** keine Abstimmung gebaut; Regeln ändert heute das Team per Code.

**Grundsatz**
> Regeln ändern die Menschen, die das Geld nutzen, mit einer Stimme je Mensch,
> innerhalb fester Grenzen, die nur eine große Mehrheit verschieben kann.

**Entwurf**
1. **Was abgestimmt wird:** Zahlen in festen Spannen (z. B. Haltegebühr 0,25
   bis 1 % im Monat; Tausch-Freibetrag 1 bis 5 × im Monat), keine Regeln
   außerhalb.
2. **Wer:** jeder verifizierte Mensch, eine Stimme; Mindestbeteiligung.
3. **Grundsätze** (Pfeiler 1 bis 3) nur mit großer Mehrheit und Wartezeit.
4. **Bis es gebaut ist:** Das Team ändert Zahlen nur innerhalb der Spannen und
   veröffentlicht jede Änderung mit Begründung vorher.

---

## 9. Betrieb und Finanzierung

**Forschung und Praxis**
- **Bristol Pound:** rund 13.000 £ Kosten im Monat, abhängig von Fördergeld;
  geschlossen 2021.
- **Chiemgauer:** Rücktausch 5 %, davon ein Teil an Vereine, ein Teil an den
  Betrieb.
- **Öffentliche Güter in Blockchain-Netzen:** Gitcoin (quadratische
  Förderung, über 72 Mio. $), Optimism (rückwirkende Förderung, 20 % des
  Angebots reserviert). Fazit der Forschung: Förderrunden sind eine Brücke,
  kein dauerhaftes Modell.

**Stand bei Aequitas:** Seit dem 24.09.2026 geht nichts an eine Treasury; die
Swap-Gebühr geht zu 40 % an Validatoren, 30 % an Liquidität, 30 % ans
Grundeinkommen. Betreuung von Läden, Support, Server, Recht: ungeklärt.

**Grundsatz**
> Wer für das Netz arbeitet, wird offen bezahlt, aus einer festen, sichtbaren
> Quelle, nie aus neu geschaffenem Geld und nie verdeckt.

**Optionen (zu entscheiden)**
1. **Ein fester, kleiner Anteil** an Swap-Gebühr oder Ausstiegsabgabe für den
   Betrieb, öffentlich verbucht, mit Obergrenze und jährlicher Abstimmung.
2. **Förderung von außen** (Stiftungen, öffentliche Mittel) mit Plan, wann sie
   endet.
3. **Validatoren tragen die Infrastruktur** (heute schon über 40 % der
   Swap-Gebühr), das Team nur Entwicklung und Pilot.
4. **Rückwirkende Förderung:** Was nachweislich genutzt wird, wird nachträglich
   bezahlt (Optimism-Modell), durch Abstimmung der Menschen.

Ohne eine dieser Quellen wiederholt sich Bristol.

---

## 10. Recht

**Hinweise aus Fachveröffentlichungen (keine Rechtsauskunft)**
- **MiCA, Whitepaper-Pflicht:** Für Kryptowerte, die **kostenlos** angeboten
  werden, entfällt sie. Nach Darstellung mehrerer Kanzleien gilt ein Angebot
  aber **nicht als kostenlos, wenn der Anbieter personenbezogene Daten
  verlangt**, und die Ausnahme entfällt, wenn eine Zulassung zum Handel
  angekündigt wird. Aequitas verlangt eine Gesichtsprüfung. **Das
  Startguthaben ist damit möglicherweise kein kostenloses Angebot im Sinne von
  MiCA.** Weitere Ausnahmen: weniger als 150 Personen je Mitgliedstaat, unter
  1 Mio. € in 12 Monaten.
- **ZAG / E-Geld:** Die BaFin legt „begrenzte Netze“ eng aus; Gutscheine für
  mehrere unabhängige Unternehmen laufen in der Regel über ein
  E-Geld-Institut.
- **Kredit** kann ein erlaubnispflichtiges Bankgeschäft sein.
- **DSGVO Art. 9** für biometrische Daten; Drittlandübermittlung.

**Grundsatz**
> Kein echter Wert, bevor die Rechtslage geklärt ist. Die Beta bleibt Testgeld.

**Folgerung:** Die Kanzlei-Fragen (`RECHTSFRAGEN_UNTERNEHMEN.md`) um MiCA
Titel II ergänzen (Whitepaper, „kostenlos“ bei Biometrie,
Schwellenwerte für die Beta).

---

## 11. Sicherheit und Konsens

**Stand (Audit 02.10.2026):** ein Blockerzeuger; Nachspielen rechnet nicht
jeden Wert selbst nach (K-2); StateRoot ist Warnung, keine Regel;
Liegegeld-Prüfung nur beobachtend; kein zweiter unabhängiger Betreiber.

**Grundsatz**
> Jeder Knoten prüft alles selbst. Ein Wert, den ein Knoten einem anderen
> glauben muss, ist ein Vorrecht.

**Folgerung:** Zweiter unabhängiger Betreiber lesend, Liegegeld-Prüfung auf
`streng`, K-2 schließen, bevor ein zweiter Erzeuger zugelassen wird.

---

## 12. Reihenfolge

Die Pfeiler tragen einander. Diese Reihenfolge folgt der Abhängigkeit:

| Schritt | Pfeiler | Was | Wann |
|---|---|---|---|
| 1 | 10 Recht | Kanzlei beantwortet MiCA, ZAG, DSGVO | vor jedem echten Wert |
| 2 | 11 Konsens | zweiter Betreiber, Prüfung streng | vor zweitem Erzeuger |
| 3 | 1 Identität | gestaffeltes Startguthaben an, Entzug regeln, Zurückholen entwerfen | vor dem Pilot |
| 4 | 9 Betrieb | Finanzierungsquelle festlegen | vor dem Pilot |
| 5 | 8 Mitbestimmung | Spannen festlegen, Abstimmung entwerfen | vor dem Pilot |
| 6 | 3 Annahme, 5 Unternehmen | Pilotstadt, Läden mit Lieferanten, Anmeldung und Kasse | Pilot |
| 7 | 2, 4, 7 Messen | Umlauf, Haltedauern, Grundeinkommen, Weitergabe | im Pilot |
| 8 | 6 Kredit | Zahlungsziel; gegenseitiger Kredit zur Abstimmung | nach Pilot und Recht |
| 9 | 3 Ausgang | echter Stablecoin, an Messwerte gebunden | zuletzt |

## 13. Entscheidungen, die nur ihr treffen könnt

1. **Pfeiler 2:** Einmal 1.000 AEQ bleiben, oder laufende Ausgabe wie Circles?
2. **Pfeiler 1:** Gestaffeltes Startguthaben jetzt aktivieren?
3. **Pfeiler 9:** Woher kommt das Geld für den Betrieb?
4. **Pfeiler 8:** Welche Zahlen in welchen Spannen dürfen die Menschen ändern?
5. **Pfeiler 6:** Gegenseitiger Kredit ja oder nein, als Grundsatzfrage?
6. **Pfeiler 10:** Welche Kanzlei, und mit welchem Budget?

## 14. Fachleute, die gegenlesen sollten

Nach Themen, als Richtung, nicht als Empfehlung bestimmter Personen:
- **Komplementärwährungen und Haltegebühr:** Forschende aus dem Umfeld des
  International Journal of Community Currency Research; Praxis Chiemgauer.
- **Verrechnung und Kredit unter Firmen:** Praxis Sardex, WIR; Forschung zu
  Sardex (Dini u. a.) und WIR (Stodder, Lietaer).
- **Digitale Gemeinschaftswährungen und Grundeinkommen:** Grassroots Economics
  (Sarafu), Forschung zu Haltedauern (Mattsson u. a.), Circles.
- **Personennachweis:** Forschung zu Proof of Personhood (Ford u. a.).
- **Recht:** Kanzlei mit MiCA-, ZAG- und DSGVO-Erfahrung.

## Quellen

Identität
- Buterin über Risiken biometrischer Personennachweise: https://www.theblock.co/post/241088/vitalik-buterin-four-major-risks-worldcoin
- Siddarth u. a., Who Watches the Watchmen? (Übersicht Personennachweise): https://arxiv.org/pdf/2008.05300
- Ford, Identity and Personhood in Digital Democracy: https://arxiv.org/pdf/2011.02412
- World-ID-Schwarzmarkt: https://www.theblock.co/post/231433/worldcoin-iris-black-market
- World-ID löschen mit Sperrfrist: https://www.theblock.co/post/287115/worldcoin-eyeball-scans-to-be-deleted-upon-request

Geld, Wert, Umlauf
- Minsky, Hierarchie des Geldes: https://www.levyinstitute.org/publications/the-hierarchy-of-money/
- Godschalk, Does Demurrage matter?: https://www.ijccr.net/article/view/9173
- Demurrage (Übersicht, WIR 1948): https://en.wikipedia.org/wiki/Demurrage_currency
- Circles, Demurrage: https://docs.aboutcircles.com/user-guides/circles-features/daily-burn-demurrage
- GoodDollar: https://docs.gooddollar.org/protocol-v3/architecture-and-value-flow
- Sarafu: https://pmc.ncbi.nlm.nih.gov/articles/PMC10088680/ ; https://arxiv.org/abs/2209.01512
- REC Barcelona, lokaler Multiplikator: https://news.esci.upf.edu/2023/06/19/complementary-currencies-rec
- AMM und dünne Liquidität: https://arxiv.org/pdf/2101.08778

Unternehmen und Kredit
- Sardex, Kreditgrenzen und Ausfälle (Gespräch mit Littera): https://opencredit.network/2019/10/01/lessons-from-sardinia-a-conversation-with-giuseppe-littera-of-sardex/
- Sardex, Studie: https://eprints.lse.ac.uk/67135/7/Dini_From%20complimentary%20currency.pdf
- WIR Bank: https://en.wikipedia.org/wiki/WIR_Bank ; Stodder & Lietaer: https://bernard-lietaer.org/wp-content/uploads/2022/07/2016-The-Macro-Stability-of-Swiss-WIR-Bank-Credits-Balance-Velocity-and-Leverage-Stodder-Lietaer-annotated.pdf
- Banerjee, Karlan, Zinman, Six Randomized Evaluations of Microcredit: https://pubs.aeaweb.org/doi/pdfplus/10.1257/app.20140287
- Credit Commons: https://wiki.p2pfoundation.net/Credit_Commons_Protocol
- JAK: https://en.wikipedia.org/wiki/JAK_Members_Bank

Grundeinkommen
- Egger u. a., Econometrica 2022: https://www.econometricsociety.org/publications/econometrica/2022/11/01/General-Equilibrium-Effects-of-Cash-Transfers-Experimental-Evidence-from-Kenya
- OpenResearch (NBER w32719): https://www.nber.org/system/files/working_papers/w32719/w32719.pdf

Mitbestimmung und Betrieb
- Ostroms Prinzipien und Blockchain: https://www.frontiersin.org/articles/10.3389/fbloc.2021.577680/full
- Quadratisches Abstimmen, Plutokratie: https://gitcoin.co/mechanisms/quadratic-voting
- Finanzierung öffentlicher Güter: https://ethresear.ch/t/three-fundamental-problems-in-ethereum-public-goods-funding-a-research-agenda/23474
- Bristol Pound, Kosten: https://www.coinbooks.org/v24/esylum_v24n28a30.html ; Petz & Finch: https://www.ijccr.net/article/view/8851

Recht
- MiCA, kostenlose Angebote und personenbezogene Daten: https://www.axisadvisory.xyz/blog-posts/are-airdrops-actually-exempt-under-mica ; https://www.osborneclarke.com/insights/what-are-eus-white-paper-requirements-micar-and-do-they-apply-bitcoin
- BaFin, Zahlungsdienste: https://bafin.de/EN/Aufsicht/ZahlungsdienstePSD2/ZulassungspflichtigeZahlungsdienste/ZulassungspflichtigeZahlungsdienste_node_en.html
