# Startbetrag: 1.000 oder 100 AEQ je Mensch?

Stand 17.11.2026. Frage des Betreibers: „Sollte man anstatt 1.000 lieber mit 100 AEQ
starten?“ Dazu ein Durchgang durch die geltenden Wirtschaftsregeln (Gleichgewicht seit
07.11.2026, `wirtschaft3.go`) mit Fachliteratur. Aufbauend auf `WIRTSCHAFT_ENDFASSUNG.md`
und `WIRTSCHAFT_LITERATUR.md`, nicht als Ersatz.

**Nachprüfbarkeit:** Jede Quelle steht am Ende mit Link oder DOI und mit ihrem Prüfstand.
Am 17.11.2026 per Websuche geprüft wurden Erscheinungsort, Jahr und die zitierte Kernaussage
(Abstract bzw. Zusammenfassung). Die Volltexte hinter Bezahlschranken sind nicht gelesen.
Was sich nicht bestätigen ließ, ist gestrichen oder ausdrücklich als ungeprüft markiert. Die
Rechnungen in Abschnitt 2 hängen an keiner Quelle; jede Zahl darin lässt sich aus dem Code
nachrechnen (Dateien genannt).

---

## 1. Ergebnis

**Bei 1.000 AEQ bleiben.** Der Startbetrag legt nur fest, *in welcher Zahl* der faire
Anteil ausgedrückt wird. Was er *wert* ist, entscheidet er nicht. Ein Wechsel auf 100 AEQ
würde nichts verbessern und kostet eine Konsensänderung samt Umrechnung aller Bestände.

Was wirklich zählt, steht in Abschnitt 4. Allem voran: Wenn hinter der Frage die Sorge
steht, dass 1.000 AEQ eine gekaufte oder gefälschte Identität zu lohnend machen, ist der
gestaffelte Zuschuss das richtige Mittel, nicht ein kleinerer Betrag.

---

## 2. Warum die Zahl den Wert nicht bestimmt

**Geldmenge.** Jeder Mensch bringt bei der Registrierung den Startbetrag `g` in die Welt.
Die Umlaufabgabe nimmt nichts aus dem Umlauf, sie verteilt nur um. Also gilt als Regel
`M = N × g` (Geldmenge = Menschen × Startbetrag). `TotalSupply()` gibt diese Regel aus.
`supply_measured.go` misst den tatsächlichen Bestand (alle Guthaben plus Pool-Reserve) und
schlägt Alarm, wenn er davon abweicht.
Pro Kopf ist die Geldmenge damit nach der Regel genau `g`, der „faire Anteil“.

**Wert.** Nach der Verkehrsgleichung (Fisher 1911) gilt pro Kopf `g × V = P × y`, mit
Umlaufgeschwindigkeit V, Preisniveau P und dem, was ein Mensch im Zeitraum in AEQ
umsetzt, y. Der Wert des fairen Anteils in Gütern ist dann

```
g / P = y / V
```

Der Startbetrag `g` kommt rechts nicht vor. Wie viel ein fairer Anteil kaufen kann, hängt
davon ab, wie viel Menschen in AEQ handeln und wie schnell das Geld umläuft. Ob die Zahl
1.000 oder 100 heißt, spielt dafür keine Rolle. Das ist die Neutralität einer einmaligen
Änderung der Nennwerte (Patinkin 1956; Lucas 1996). Bei Aequitas gilt sie besonders
sauber, weil es keine Zinsen und keine Altschulden in AEQ gibt.

Am Markt (AEQ/tUSD-Pool) hieße das: Bei 100 AEQ stünde ein AEQ ungefähr beim
Zehnfachen in tUSD, und der faire Anteil wäre gleich viel wert.

**Was sich mit `g` ändern würde (alles nominal):**

| Größe | bei 1.000 AEQ | bei 100 AEQ |
|---|---|---|
| Umlaufabgabe des fairen Anteils je Tag (0,5 %/Monat) | 0,167 AEQ | 0,0167 AEQ |
| Grundeinkommen je Mensch und Monat | ≈ 5 AEQ | ≈ 0,5 AEQ |
| unter der Einzugsschwelle (0,001 AEQ je Tag) | Guthaben < 6 AEQ = 0,6 % des fairen Anteils | < 6 AEQ = 6 % des fairen Anteils |
| Vermögensgrenze 25 × | 25.000 AEQ | 2.500 AEQ |

Die Einzugsschwelle ist ein fester Betrag (`umlaufMindestBetrag`). Bei 100 AEQ würde sie
zehnmal mehr vom fairen Anteil freistellen. Das ist der einzige Punkt in der Tabelle, an dem
sich die Wirkung ändert und nicht nur die Zahl.

---

## 3. Was für 1.000 spricht

1. **Gewohnte Preiszahlen.** Bekannte Komplementärwährungen stehen 1 : 1 zur
   Landeswährung: der Chiemgauer zum Euro (fester Kurs), der WIR zum Franken (als
   Verrechnungswert, nicht frei tauschbar). Preise bleiben dadurch lesbar, ohne Umrechnung
   im Kopf. (Für Sardex und Bristol Pound ließ sich ein 1 : 1 nicht bestätigen. Sie sind
   deshalb hier nicht genannt.)

   Aequitas hat keinen festen Kurs. Mit 1.000 AEQ landet ein AEQ aber in einer ähnlichen
   Größenordnung, sobald der faire Anteil so viel wert ist wie das, was Menschen
   üblicherweise als Zahlungsmittel halten: einige hundert bis einige tausend Euro. Ein
   Kaffee kostet dann wenige AEQ. Bei 100 AEQ kostete er Bruchteile eines AEQ, also
   Kommabeträge im Alltag.

2. **Die Grenzen sind Vielfache des fairen Anteils** (`wirtschaft.go`: 1 ×, 3 ×, 5 ×,
   25 ×, 0,25 ×), und die Website erklärt sie so („Every limit is a multiple of the fair
   share“). Die Rechenbeispiele dort (2.000 = 2 ×, 25.000 = 25 ×) sind mit 1.000 im Kopf
   nachvollziehbar.

3. **Eine spätere Umstellung hat Nebenwirkungen.**
   - Nach der Euro-Bargeldumstellung stiegen die Preise billiger Waren stärker, und zwar
     umso mehr, je schwerer den Menschen der Umgang mit der neuen Währung fiel (Dziuda &
     Mastrobuoni 2009).
   - Wie stark die Lebensmittelpreise stiegen, hing davon ab, wie umständlich der
     Umrechnungskurs war. Ehrmann (2011) führt das auf die begrenzte Aufmerksamkeit der
     Verbraucher zurück.
   - Menschen denken in Nennwerten, nicht nur in Kaufkraft (Geldillusion, Shafir,
     Diamond & Tversky 1997).
   - Länder streichen Nullen vor allem nach hoher Inflation (Mosley 2005). Ohne Inflation,
     wie bei Aequitas, fehlt der übliche Anlass.

   Für Aequitas folgt daraus: Die Zahl einmal festlegen und dabei bleiben. Wenn man sie
   ändern wollte, dann jetzt, solange ein einziger Mensch registriert ist. Einen Grund dafür
   gibt es aber nicht.

4. **Die Zahl steckt an vielen Stellen.** Außer `registrationGrant` stehen feste
   AEQ-Beträge im Code: der gestaffelte Zuschuss 200 + 800 (`grant_staffel.go`), das
   Offline-Bargeld 1 bis 50 AEQ Pfand und 20 AEQ je Zahlung (`offline_bargeld.go`), die
   Slashing-Strafe 50 AEQ, die Mindestdeckung beim Quantengeld 1 AEQ, die Einzugsschwelle
   0,001 AEQ, `registrationGrantAEQ` im Sybil-Bericht. Dazu kommen die Texte in 12
   Sprachen auf Website und App.

   Bei 100 AEQ wären 200 AEQ „sofort“ mehr als der ganze Zuschuss, und 50 AEQ Offline-Pfand
   wären der halbe faire Anteil statt 5 %. Alles müsste mitgezogen werden, dazu die
   Bestände in der Kette: Guthaben, Pool-Reserven, LP-Anteile, Pfänder, Deckungen. Das ist
   eine Änderung der StateRoot an einem Stichtag, mit eigener Sicherheitsprüfung.

---

## 4. Was für 100 sprechen könnte, und warum es nicht trägt

| Argument | Einordnung |
|---|---|
| Eine Kunstfigur bringt weniger | Nur dem Namen nach. Wert ist `y / V`, gleich bei beiden Zahlen (Abschnitt 2). Was hilft, ist, **wann** und **unter welcher Prüfung** der Anteil frei wird: der gestaffelte Zuschuss, gebaut, schlafend bis 2100 (`grant_staffel.go`). Gekaufte Identitäten sind das größte Risiko für jede Regel „je Mensch“; World-IDs wurden 2023 für rund 30 $ gehandelt (`WIRTSCHAFT_LITERATUR.md` 1.5). |
| „1.000 geschenkt“ klingt nach zu viel, lockt Glücksritter an | Die Werbewirkung einer großen Zahl ist eine Geldillusion. Sie lässt sich ehrlich auffangen: in der App zusätzlich den Anteil am fairen Anteil zeigen. Die Relative Theorie des Geldes (Laborde 2010, Grundlage von Ğ1) schlägt aus demselben Grund vor, Beträge in Anteilen der Dividende zu messen. Wie Ğ1-Programme das heute anzeigen, ist nicht geprüft. |
| Niedrigere Stückzahl, höherer Preis je AEQ, wirkt „wertvoller“ | Das ist die Stückzahl-Heuristik (*unit bias*, Geier, Rozin & Doros 2006), beim Krypto-Handel oft beobachtet. Sie wirkt in beide Richtungen und ist kein Ziel für ein Zahlungsmittel. |

---

## 5. Die Regeln im Ganzen: was bleiben, was kommen sollte

**Bleiben** (in der Literatur gestützt, schon so beschlossen):

- **Umlaufabgabe 0,5 % im Monat auf jedes Guthaben, alles als Grundeinkommen an alle.**
  - Das sind 5,8 % im Jahr, im erprobten Bereich: Gesell 5,2 %, Circles 7 %, Chiemgauer
    8 %, Wörgl 12 %.
  - Die Höhe treibt den Umlauf nicht (Godschalk 2012), zu viel tötet das Geld (Fishers
    Stamp Scrip 1933).
  - Die Abgabe wirkt als Bremse gegen Horten, nicht als Motor.
  - Keynes hielt den Gedanken für richtig, nur die Umsetzung für schwierig (General Theory,
    Kap. 23).
- **Täglich, anteilig.** Das ist gerechter als eine Monatsbuchung, und es gibt keinen
  Stichtag, an dem man das Geld kurz woanders parkt.
- **Angleichung an den fairen Anteil.** Bei 0,5 % im Monat halbiert sich der Abstand eines
  Guthabens zum fairen Anteil in rund 11,5 Jahren (ln 2 / 0,005 ≈ 139 Monate). Das ist
  langsam genug, dass niemand enteignet wird, und stetig genug gegen dauerhafte Ballung.

**Vorschläge** (nach Wirkung geordnet):

1. **Identität vor Wirtschaft.** Den gestaffelten Zuschuss scharf schalten, sobald seine
   Voraussetzungen stehen. Diese stehen in `grant_staffel.go`:
   - Die Zulassung der Coordinatoren muss knapp sein.
   - Der strenge Modus beginnt spätestens mit der Staffel.
   - Die Chain-ID muss in Bindung und Bescheinigung stehen.
   - Kalibriert wird an mindestens 20 echten Registrierungen.

   Das ist der Hebel gegen Kunstfiguren. Ehrliche Menschen verlieren nichts, Tag 30 gibt
   alles frei.
2. **Anzeige in der App.**
   - Grundeinkommen und Abgabe desselben Durchlaufs als eine Zeile, netto.
   - Dazu wahlweise der Stand als Anteil am fairen Anteil („92 % deines fairen Anteils“).
   - Damit wird sichtbar, was die Regel tut: Wer unter dem Anteil liegt, bekommt netto
     dazu.
   - Das ist reine Anzeige, an den Regeln ändert sich nichts.
3. **Feste Beträge als Vielfache schreiben**, wo sie den fairen Anteil meinen. Der
   gestaffelte Zuschuss ist 0,2 × + 0,8 × statt 200 + 800 AEQ. Ändern würde sich dadurch
   kein Wert, nur die Aussage „jede Grenze ist ein Vielfaches“ würde auch im Code stimmen.
   Sicherheitsgrenzen (Offline-Pfand, Slashing) bleiben feste Beträge. Sie begrenzen ein
   Risiko, keinen Anteil.
4. **Was keine Regel lösen kann** (`WIRTSCHAFT_LITERATUR.md`, weiter offen):
   - Betriebskosten: Bristol scheiterte an den Kosten, nicht an den Regeln.
   - Ein Anreiz für Betriebe durch zinsfreien gegenseitigen Kredit, wie bei WIR und Sardex.
   - Zugehörigkeit sichtbar machen, wie beim Chiemgauer: „dein Einkauf hat X AEQ zum
     Grundeinkommen beigetragen“.

**Zur Zeit mit einem Menschen:** Abgabe und Grundeinkommen heben sich auf (0,166 AEQ hin und
zurück), das Netz verteilt an sich selbst. Das ist richtig so und kein Fehler. Die Wirkung
beginnt mit dem zweiten Menschen.

---

## Quellen

Prüfstand: **geprüft** = Erscheinungsort und zitierte Kernaussage am 17.11.2026 per
Websuche bestätigt (Abstract bzw. Zusammenfassung, nicht der Volltext). **Standardwerk** =
Buch, dessen Inhalt in der Fachliteratur feststeht; nicht eigens nachgeschlagen.

| Quelle | Wofür zitiert | Prüfstand | Fundstelle |
|---|---|---|---|
| Fisher, I. (1911): *The Purchasing Power of Money* | Verkehrsgleichung MV = PT | Standardwerk | – |
| Gesell, S. (1916): *Die natürliche Wirtschaftsordnung* | Ursprung der Umlaufabgabe | Standardwerk | – |
| Fisher, I. (1933): *Stamp Scrip* | zu hohe Abgabe (2 %/Woche) setzte sich nicht durch | Standardwerk; Aussage aus `WIRTSCHAFT_LITERATUR.md` | – |
| Keynes, J. M. (1936): *General Theory*, Kap. 23 „Notes on Mercantilism, the Usury Laws, Stamped Money …“ | Idee richtig, Umsetzung schwierig | geprüft | [Kap. 23 (Volltext)](https://www.hetwebsite.net/het/texts/keynes/gt/chap23.htm) |
| Patinkin, D. (1956): *Money, Interest, and Prices* | Neutralität: die Geldmenge bestimmt das Preisniveau, nicht die realen Größen | Erscheinen geprüft; die Neutralitätsaussage steht so in Patinkins Lexikoneintrag „Neutrality of money“ (New Palgrave) | [EconBiz](https://www.econbiz.de/Record/money-interest-and-prices-an-integration-of-monetary-and-value-theory/10004643673), [New Palgrave](https://link.springer.com/referenceworkentry/10.1057/978-1-349-95189-5_957) |
| Lucas, R. E. (1996): „Nobel Lecture: Monetary Neutrality“, *JPE* 104(4), 661–682 | Neutralität | geprüft | [doi:10.1086/262037](https://doi.org/10.1086/262037), [PDF](https://knowledge.uchicago.edu/record/6013/files/Nobel-Lecture-Monetary-Neutrality.pdf) |
| Shafir, E., Diamond, P., Tversky, A. (1997): „Money Illusion“, *QJE* 112(2), 341–374 | Menschen denken in Nennwerten | geprüft (2019 repliziert) | [doi:10.1162/003355397555208](https://doi.org/10.1162/003355397555208) |
| Mosley, L. (2005): „Dropping Zeros, Gaining Credibility? Currency Redenomination in Developing Nations“, APSA-Tagung | Streichungen von Nullen folgen meist hoher Inflation | geprüft (Abstract; Hauptbefund über zitierende Arbeiten) | [PDF](https://laynamosley.scholar.princeton.edu/sites/g/files/toruqf4271/files/laynamosley/files/dropping_zeros_gaining_credibility_currency_redeno.pdf) |
| Geier, A. B., Rozin, P., Doros, G. (2006): „Unit Bias“, *Psychological Science* 17(6), 521–525 | Stückzahl-Heuristik | geprüft; die Übertragung auf Krypto-Handel ist **nicht** aus dieser Quelle | [PubMed 16771803](https://pubmed.ncbi.nlm.nih.gov/16771803/) |
| Dziuda, W., Mastrobuoni, G. (2009): „The Euro Changeover and Its Effects on Price Transparency and Inflation“, *JMCB* 41(1), 101–129 | billige Waren und schwierige Umrechnung: mehr Teuerung | geprüft | [doi:10.1111/j.1538-4616.2008.00189.x](https://doi.org/10.1111/j.1538-4616.2008.00189.x) |
| Laborde, S. (2010): *Théorie Relative de la Monnaie* | Messen in Anteilen der Dividende (Ğ1) | Erscheinen geprüft | [P2P Foundation](https://wiki.p2pfoundation.net/Libre_Currency), [Duniter](https://duniter.org/blog/duniter-why-and-how/) |
| Ehrmann, M. (2011): „Inflation Developments and Perceptions after the Euro Cash Changeover“, *German Economic Review* 12(1), 33–58 | Teuerung hing an der Schwierigkeit des Umrechnungskurses | geprüft | [doi:10.1111/j.1468-0475.2010.00503.x](https://doi.org/10.1111/j.1468-0475.2010.00503.x), [EZB-Fassung](https://www.ecb.europa.eu/pub/pdf/scpwps/ecbwp588.pdf) |
| Godschalk, H. (2012): „Does Demurrage Matter for Complementary Currencies?“, *IJCCR* 16, 58–69 | Die Höhe der Abgabe ist für Umlauf und Nutzung nicht entscheidend | geprüft | [doi:10.15133/j.ijccr.2012.012](https://doi.org/10.15133/j.ijccr.2012.012), [Volltext](https://www.ijccr.net/article/view/9173) |
| Chiemgauer: 1 Chiemgauer = 1 € | 1 : 1-Kopplung | geprüft (Wikipedia) | [Chiemgauer](https://de.wikipedia.org/wiki/Chiemgauer) |
| WIR: Verrechnungswert 1 : 1 zum Franken | 1 : 1-Kopplung | geprüft (Kennedy) | [Kennedy, Complementary Currencies](https://margritkennedy.de/wp-content/uploads/2020/02/art_en_complementary_currencies_54.pdf) |
| Sätze Gesell, Circles, Chiemgauer, Wörgl; World-ID-Kauf 2023 | Vergleich der Sätze; Identitätsrisiko | übernommen aus `WIRTSCHAFT_LITERATUR.md`, hier nicht neu geprüft | dort |

**Nachrechnen im Code:**
- `M = N × g`: Regel in `TotalSupply()`, gemessen und abgeglichen in `supply_measured.go`
  (`MeasuredTotalAEQ`, `supplyAlarm`); die Staffel zählt mit (`grant_staffel.go`, Kopf).
- Monat = 30 Tage: `sekundenJeMonat` (`wirtschaft.go`).
- 0,5 % im Monat: `gleichgewichtSatzMonat` (`wirtschaft3.go`), anteilig je Sekunde:
  `umlaufBetragRoh` (`wirtschaft.go`).
- Einzugsschwelle: `umlaufMindestBetrag` = 0,001 AEQ (`wirtschaft.go`).
- Grenzen als Vielfache: `wirtschaft.go`, Zeilen mit `registrationGrant`.
- Feste Beträge: `grant_staffel.go` (200/800), `offline_bargeld.go` (1/50/20 AEQ),
  `slashing.go` (50 AEQ), `quanten_geld.go` (1 AEQ).
