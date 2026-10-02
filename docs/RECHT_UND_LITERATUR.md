# Recht und Forschung: was Komplementärgeld, MiCA, BaFin und DSGVO für Aequitas bedeuten

Stand 02.10.2026. **Keine Rechtsauskunft.** Dieses Dokument fasst
Fachveröffentlichungen, Behördenentscheidungen und Kanzleiartikel zusammen und
übersetzt sie in Folgen für Aequitas. Die Primärtexte (Papiere, Verordnungen)
waren aus dieser Umgebung nicht direkt abrufbar; gestützt ist alles auf
Suchergebnisse und Zusammenfassungen. Jede Aussage mit **(prüfen)** braucht
eine Kanzlei oder den Blick in den Originaltext, bevor darauf gebaut wird.

Ergänzt: `GRUNDPFEILER.md` Abschnitt 10, `WIRTSCHAFT_LITERATUR.md` Abschnitt
1.6, `RECHTSFRAGEN_UNTERNEHMEN.md` (neue Fragen I und J) und das
DSGVO-Unterlagenpaket im Repo `aequitas-biometric-beta/docs/dsgvo/`.

---

## Das Wichtigste in zehn Sätzen

1. **Die meisten Komplementärwährungen bleiben klein und sterben jung.** Die
   Forschung nennt als Grund fast immer fehlende kritische Masse und fehlende
   Annahme bei Unternehmen, nicht schlechte Regeln.
2. **Nachgewiesen ist vor allem sozialer Nutzen.** Messbare Einkommenseffekte
   gibt es in einer Minderheit der Fälle.
3. **Ein Startguthaben ohne Gegenleistung in Geld ist vermutlich kein E-Geld**
   nach ZAG, denn E-Geld setzt eine Ausgabe gegen Zahlung voraus. **(prüfen)**
4. **AEQ wäre unter MiCA am ehesten ein „sonstiger Kryptowert“** (Titel II).
5. **Die Ausnahme „vollständig dezentral“ greift sehr wahrscheinlich nicht**,
   solange es einen erkennbaren Betreiber gibt, der App, Registrierung und
   Server führt.
6. **Die Ausnahme für kostenlose Angebote greift sehr wahrscheinlich nicht.**
   MiCA sagt ausdrücklich: Ein Kryptowert ist nicht kostenlos, wenn die
   Empfänger dafür personenbezogene Daten geben müssen. Die Gesichtsprüfung
   ist genau das.
7. **Der eingebaute Tausch AEQ ↔ Stable ist ein Kandidat für eine
   erlaubnispflichtige Krypto-Dienstleistung**, sobald ein echter Stablecoin
   daran hängt. Mit einem E-Geld-Token (EURC o. ä.) kommt seit 02.03.2026
   möglicherweise auch eine Zahlungsdienste-Erlaubnis hinzu.
8. **Die Gesichtsprüfung ist besondere Datenkategorie (Art. 9 DSGVO).** Das
   wichtigste deutsche Vorbild ist der BayLDA-Bescheid gegen Worldcoin
   (Dezember 2024): Löschpflicht, ausdrückliche Einwilligung, sichere
   Speicherung.
9. **Spanien und Portugal haben Worldcoin 2024 vor allem wegen Minderjährigen
   und nicht widerrufbarer Einwilligung gestoppt.** **Aequitas hat heute
   keine Altersabfrage** (in App und Biometrie-Repo nicht gefunden). Das ist
   der konkreteste Handlungspunkt dieses Dokuments.
10. **Der AI Act stuft den Abgleich „bin ich schon registriert?“ vermutlich
    nicht als Hochrisiko-Fernidentifizierung ein**, weil die Person aktiv
    mitwirkt. Das ist eine Auslegung, keine gefestigte Linie. Die
    Hochrisiko-Pflichten nach Anhang III gelten ohnehin erst ab 02.12.2027.

---

## Teil 1: Komplementärwährungen in der Forschung

### 1.1 Wie viele es gibt und warum sie scheitern

- **Seyfang & Longhurst (2013)** erfassten 3.418 Projekte in 23 Ländern in
  vier Typen: Dienstleistungsbörsen/Zeitbanken, gegenseitiger Kredit,
  regionales Geld und Tauschringe (LETS). Ihr Befund: Diese Projekte sind
  „grassroots innovations“ und erreichen selten die Masse, ab der sie sich
  selbst tragen.
- **Michel & Hudon (2015)**, systematische Übersicht in *Ecological
  Economics*: Der Nutzen ist überwiegend **sozial** (Netzwerke, Vertrauen,
  Einbindung). Wirtschaftliche Effekte sind schwächer und seltener belegt.
  ([ScienceDirect](https://www.sciencedirect.com/science/article/abs/pii/S0921800915002086),
  [RePEc](https://ideas.repec.org/a/eee/ecolec/v116y2015icp160-171.html))
- **Blanc (2011)** ordnet die Systeme in Generationen. Die jüngste Generation
  ist digital und verbindet mehrere Ziele. Aequitas gehört dorthin.
  ([IJCCR](https://www.ijccr.net/article/view/9128))
- **2026: Gestaltungsprinzipien für Gemeinschaftswährungen als Gemeingut**,
  angelehnt an Ostrom.
  ([ScienceDirect](https://www.sciencedirect.com/science/article/pii/S0166497226000453))
  Die Kernaussage: Systeme überleben, wenn die Nutzer die Regeln mitgestalten,
  Verstöße sichtbar werden und es abgestufte Sanktionen gibt.
- **Nesta, „More than Money“**: Literaturüberblick mit demselben Ergebnis.
  ([Nesta](https://www.nesta.org.uk/report/more-than-money-literature-review/))
- **Belfer Center**: Krypto-Stadtmünzen erben die Probleme der alten
  Regionalwährungen (geringe Annahme, Umtausch in die Landeswährung) und
  fügen neue hinzu (Volatilität, Spekulation).
  ([Belfer](https://www.belfercenter.org/publication/community-currency-crypto-city-tokens-potentials-shortfalls-and-future-outlooks-new-old))

### 1.2 Einzelfälle mit Lehren für Aequitas

| Fall | Was geschah | Lehre für Aequitas |
|---|---|---|
| **Wörgl 1932/33** | Haltegebühr, schneller Umlauf, von der Nationalbank verboten ([SAGE 2025](https://journals.sagepub.com/doi/10.1177/02690942251319247)) | Haltegebühr wirkt. Das Risiko ist rechtlich, nicht wirtschaftlich |
| **Chiemgauer** | Euro-gedeckt, 2 %/Quartal Haltegebühr, 5 % Rücktauschgebühr an Vereine ([Wikipedia](https://en.wikipedia.org/wiki/Chiemgauer), [IJCCR](https://www.ijccr.net/article/view/9176)) | Funktioniert regional; Läden machen mit, weil Vereinskundschaft kommt |
| **Bristol Pound** | 2012–2021, eingestellt; Annahme blieb oberflächlich ([IJCCR](https://www.ijccr.net/article/view/8851)) | Annahme ohne echten Weitergabeweg für Läden reicht nicht |
| **Sarafu (Kenia)** | Daten zu 40.000 Nutzern; Umlauf vor allem zwischen Händlern im Nahbereich ([Nature Sci. Data](https://www.nature.com/articles/s41597-022-01539-4)) | Gesundes Geld zirkuliert in kleinen Kreisen; Firmen-Firmen-Kreise sind entscheidend |
| **WIR (Schweiz)** | Seit 1934; gegenseitiger Kredit, antizyklisch ([Stodder & Lietaer](https://bernard-lietaer.org/wp-content/uploads/2022/07/2016-The-Macro-Stability-of-Swiss-WIR-Bank-Credits-Balance-Velocity-and-Leverage-Stodder-Lietaer-annotated.pdf)) | Kredit zwischen Firmen ist das, was Firmen hält. Aber WIR ist eine Bank |
| **Sardex (Sardinien)** | Gegenseitiger Kredit für KMU ([LSE](https://eprints.lse.ac.uk/67135/7/Dini_From%20complimentary%20currency.pdf)) | Wächst mit aktiver Vermittlung (Makler), nicht von selbst |
| **Circles / GoodDollar** | Grundeinkommen in Kryptoform mit Haltegebühr ([Circles](https://docs.aboutcircles.com/user-guides/circles-features/daily-burn-demurrage), [GoodDollar](https://docs.gooddollar.org/protocol-v3/architecture-and-value-flow)) | Ohne Annahme in Läden fließt das Geld sofort in den Umtausch |
| **Barcelona REC** | Grundeinkommen teils in Komplementärgeld; lokaler Multiplikator gemessen ([Urban Studies](https://www.urbanstudiesonline.com/resources/resource/the-local-multiplier-of-income-support-paid-in-a-complementary-currency-comparative-evaluation-in-the-city-of-barcelona/)) | Grundeinkommen plus Laden-Netz ist die Kombination mit der besten Evidenz |

### 1.3 Haltegebühr: was wirklich belegt ist

- Eine Haltegebühr beschleunigt den Umlauf. Ob sie den **wirtschaftlichen**
  Nutzen erhöht, ist umstritten.
  ([IJCCR, „Does Demurrage matter?“](https://www.ijccr.net/article/view/9173))
- Sie wird von Nutzern als Strafe erlebt, wenn sie nicht verstanden wird.
  ([IJCCR, psychologische Faktoren](https://www.ijccr.net/article/view/9207))
- **Folge für Aequitas:** Die heutige Form ist vertretbar: Freibetrag 5.000,
  danach 0,5 %/Monat; in Stufe 2 trifft sie zuerst nur große Bestände. Die
  Erklärung in der App ist mindestens so wichtig wie der Satz.

### 1.4 Die Bundesbank zu Regionalgeld

- **Rösl (2006), Diskussionspapier 43:** Deutsches Regionalgeld war insgesamt
  bei rund 200.000 € im Umlauf. Es wird geduldet, **weil** das Volumen
  unbedeutend ist.
  ([Bundesbank](https://www.bundesbank.de/resource/blob/703356/01dbc05172e7793f725edcbfd5e15a92/mL/2006-12-29-dkp-43-data.pdf))
- Spätere Bundesbank-Veröffentlichung zur Zukunft der Gemeinschaftswährungen:
  ([Bundesbank 2014](https://www.bundesbank.de/resource/blob/635110/82037fb76367f51215e749eec95330d6/mL/2014-09-16-the-future-of-community-currencies-data.pdf))
- **Folge:** Duldung ist keine Rechtsgrundlage. Wächst AEQ über die
  Bedeutungslosigkeit hinaus, ändert sich die Lage.

---

## Teil 2: MiCA (EU-Verordnung 2023/1114)

### 2.1 Welche Art Kryptowert ist AEQ?

MiCA kennt drei Arten: **E-Geld-Token** (bezieht sich auf eine Währung),
**vermögenswertreferenzierter Token** (bezieht sich auf einen Korb) und
**sonstige Kryptowerte**. AEQ hat keinen Bezug auf eine Währung und keine
Deckung, ist also am ehesten ein **sonstiger Kryptowert (Titel II)**.
**(prüfen)**

Titel II verlangt für ein öffentliches Angebot im Kern Folgendes:

- ein **Whitepaper** nach Art. 6 und Anhang I, das der Aufsicht gemeldet wird
  (Art. 8) ([springlex Art. 8](https://www.springlex.eu/de/packages/mica/mica-regulation/article-8/));
- einen **Anbieter, der eine juristische Person ist**;
- eine **Haftung für falsche Angaben** im Whitepaper (Art. 15)
  ([ESMA Rulebook](https://www.esma.europa.eu/publications-and-data/interactive-single-rulebook/mica/article-15-liability-information-given)).

### 2.2 Ausnahmen und warum sie für Aequitas wackeln

| Ausnahme | Was sie sagt | Passt sie? |
|---|---|---|
| **Vollständig dezentral** (Erwägungsgrund 22) | Dienstleistungen ohne Vermittler fallen nicht unter MiCA; ohne erkennbaren Emittenten gelten Titel II–IV nicht | **Eher nein.** Es gibt einen Betreiber, der App, Registrierung, Coordinator und den einzigen Validator betreibt. Auch EBA/ESMA legen „dezentral“ eng aus ([LegalBison-Studie](https://globallawexperts.com/legalbison-study-we-are-defi-so-mica-does-not-apply-to-us-eba-esma-disagree/), [bitsofblocks](https://www.bitsofblocks.io/post/esma-consults-on-proposed-mica-regulations)) |
| **Kostenloses Angebot** (Art. 4 Abs. 3 lit. a) | Kein Whitepaper nötig, wenn kostenlos | **Sehr wahrscheinlich nein.** Art. 4 Abs. 3 Unterabs. 2: Nicht kostenlos ist ein Angebot, wenn die Erwerber **personenbezogene Daten geben müssen** ([springlex Art. 4](https://www.springlex.eu/en/packages/mica/mica-regulation/article-4/), [Axis Advisory](https://www.axisadvisory.xyz/blog-posts/are-airdrops-actually-exempt-under-mica)) |
| **Weniger als 150 Personen je Mitgliedstaat** (Art. 4 Abs. 2) | Kein Whitepaper | **Für die Beta ja**, danach nicht |
| **Unter 1 Mio. € in 12 Monaten** (Art. 4 Abs. 2) | Kein Whitepaper | Bei Testgeld ohne Gegenwert unklar; mit echtem Kurs schnell überschritten |
| **Belohnung für Validierung** (Art. 4 Abs. 3 lit. b) | Automatisch erzeugte Belohnungen | Nein, das Startguthaben ist keine Validierungsbelohnung |

**Folgerung:** Solange es nur Testgeld gibt und weniger als 150 Personen je
Land teilnehmen, ist das Risiko gering. **Vor dem Schritt zu echtem Wert
braucht Aequitas entweder ein Whitepaper mit einer juristischen Person als
Anbieter oder eine belastbare Begründung, warum es keinen Anbieter gibt.**
Die zweite Begründung trägt erst, wenn Betrieb, Registrierung und Validatoren
tatsächlich auf mehrere unabhängige Parteien verteilt sind.

### 2.3 Krypto-Dienstleister (CASP)

- MiCA kennt zehn Krypto-Dienstleistungen. Jede braucht eine eigene Erlaubnis
  ([21analytics](https://www.21analytics.co/glossary/crypto-asset-service-provider-casp/)).
- Für Aequitas kommen in Frage:
  - **Tausch Kryptowert gegen Kryptowert** und **Betrieb einer
    Handelsplattform**: der eingebaute Pool;
  - **Verwahrung**: nur, wenn ein Betreiber Schlüssel für Nutzer hält. Die
    App ist selbstverwahrend; das ist günstig.
- **Übergangsfristen in Deutschland** (KMAG, Bestandsschutz) sind
  ausgelaufen ([ESMA-Liste](https://www.esma.europa.eu/sites/default/files/2024-12/List_of_MiCA_grandfathering_periods_art._143_3.pdf)).
- **Folge:** Ein Pool, den ein einzelner Betreiber auf seinem Server führt,
  sieht für eine Aufsicht wie eine Tauschplattform aus. **Kein echter
  Stablecoin am Pool, bevor das geklärt ist.** Mit tUSD (reines Testgeld) ist
  das heute kein Problem.

### 2.4 E-Geld-Token am Pool: doppelte Erlaubnis

- Die EBA hat im Juni 2025 empfohlen, die **Übertragung und Verwahrung von
  E-Geld-Token** im Auftrag von Kunden **ab 02.03.2026** auch als
  Zahlungsdienst nach PSD2 zu behandeln
  ([EBA](https://www.eba.europa.eu/publications-and-media/press-releases/eba-publishes-no-action-letter-interplay-between-payment-services-directive-psd23-and-markets-crypto),
  [Morgan Lewis](https://www.morganlewis.com/pubs/2025/06/e-money-tokens-european-banking-authority-clarifies-psd2-mica-interplay-implications-for-casps),
  [FMA Österreich](https://www.fma.gv.at/en/fma-on-micar-and-psd2-for-emt-services/)).
- **Folge:** Die Idee „EURC statt tUSD“ (Rechtsfrage A3) ist rechtlich
  schwerer als gedacht. Sie bringt möglicherweise zwei Erlaubnisse.

### 2.5 Was sich gerade bewegt

- Die Kommission hat vom **20.05. bis 31.08.2026** eine Konsultation zur
  Überprüfung von MiCA geführt, mit **DeFi, Lending und Staking** als Themen
  ([Konsultationspapier](https://finance.ec.europa.eu/document/download/62be7015-f066-4fac-b74e-71bacdbcc9f5_en?filename=2026-mica-review-targeted-consultation-document_en.pdf),
  [Taylor Wessing](https://www.taylorwessing.com/en/insights-and-events/insights/2026/05/mica-20),
  [Maples](https://maples.com/knowledge/european-commission-launches-targeted-consultation-on-the-review-of-mica)).
- ESMA-Fragen und -Antworten:
  [Q&A 2552](https://www.esma.europa.eu/publications-data/questions-answers/2552),
  [Q&A 2671](https://www.esma.europa.eu/publications-data/questions-answers/2671).
- **Folge:** Die Regeln für dezentrale Kreditprotokolle (Grundpfeiler 6) sind
  in Arbeit. Ein Kreditprotokoll jetzt zu bauen hieße, auf ein bewegtes Ziel
  zu bauen.

### 2.6 Geldwäsche: Travel Rule und AMLR

- **Travel Rule (Verordnung 2023/1113):** Krypto-Dienstleister müssen bei
  jeder Übertragung Angaben zu Absender und Empfänger mitschicken; bei
  selbstverwahrten Wallets über 1.000 € zusätzlich prüfen
  ([21analytics](https://www.21analytics.co/blog/what-does-the-revised-transfer-of-funds-regulation-entail/)).
  Sie gilt für **Dienstleister**, nicht für Überweisungen zwischen zwei
  selbstverwahrten Wallets auf der Kette.
- **AMLR (ab 2027):** Krypto-Dienstleister sind Verpflichtete, anonyme
  Kryptokonten sind verboten. Selbstverwahrte Wallets sind als solche nicht
  erfasst ([JD Supra](https://www.jdsupra.com/legalnews/proposed-eu-aml-cft-regulation-8342508/)).
- **Folge:** Die Personenprüfung ohne Namen ist **keine
  Geldwäsche-Identifizierung** und ersetzt keine. Wer später Euro hinein- und
  herausführt (eine Rampe), muss selbst identifizieren. Das ist ein Argument,
  **die Rampe nicht selbst zu betreiben**, sondern einem lizenzierten
  Dienstleister zu überlassen.

---

## Teil 3: BaFin und deutsches Recht

### 3.1 E-Geld und Zahlungsdienste (ZAG)

- **E-Geld** ist ein Geldwert, der **gegen Zahlung eines Geldbetrags
  ausgegeben** wird (§ 1 Abs. 2 S. 3 ZAG). AEQ wird nicht gegen Geld
  ausgegeben, sondern nach einer Personenprüfung verschenkt. **Vermutlich
  kein E-Geld.** **(prüfen)**
  ([Bundesbank](https://www.bundesbank.de/de/aufgaben/bankenaufsicht/einzelaspekte/zahlungsinstitute-und-e-geld-institute-598334),
  [BaFin](https://bafin.de/EN/Aufsicht/ZahlungsdienstePSD2/ZulassungspflichtigeZahlungsdienste/ZulassungspflichtigeZahlungsdienste_node_en.html))
- Der **Chiemgauer** ist anders: Er wird gegen Euro gekauft. Für ihn ist die
  Frage nach E-Geld und begrenzten Netzen zentral. Für AEQ ist sie es nur,
  wenn AEQ jemals gegen Euro verkauft wird.
- **Zahlungsdienste** setzen „Geldbeträge“ voraus (Bargeld, Buchgeld,
  E-Geld). Überweisungen von AEQ sind damit vermutlich **kein
  Zahlungsdienst**. Überweisungen von E-Geld-Token sind es möglicherweise
  (siehe 2.4).
- **Korrektur gegenüber `WIRTSCHAFT_LITERATUR.md` 1.6:** Dort stand, dass
  Gutscheine für mehrere Unternehmen in der Regel über ein E-Geld-Institut
  laufen. Das gilt für **gekaufte** Gutscheine. Für ein verschenktes,
  nicht gedecktes Guthaben ist die E-Geld-Frage wahrscheinlich nicht die
  entscheidende; entscheidend ist MiCA.

### 3.2 Kryptowerte im KWG und KMAG

- § 1 Abs. 11 KWG führt **Kryptowerte als Finanzinstrumente**
  (Auffangtatbestand für alles, was nicht unter MiCA fällt). Die Verwahrung
  von MiCA-Kryptowerten fällt jetzt unter das KMAG
  ([nexvyra](https://nexvyra.de/fakten/kwg-1-abs-11-kryptowerte-definition),
  [IHK Bayreuth](https://www.ihk.de/bayreuth/hauptnavigation/service/gewerberechtliche-erlaubnisse/aktuelle-themen/erlaubnispflicht-fuer-kryptoverwahrgeschaefte-4671636)).
- **Folge:** Fällt AEQ unter MiCA, gilt das KMAG. Fällt es aus irgendeinem
  Grund nicht darunter, bleibt das KWG als Auffangnetz. **Eine Lücke, in der
  „gar nichts gilt“, gibt es nicht.**

### 3.3 Kredit

- Das **Kreditgeschäft** im KWG meint Gelddarlehen. Mehrere
  Fachveröffentlichungen folgern, dass das Verleihen von Kryptowerten
  **nicht automatisch** Kreditgeschäft ist; die wirtschaftliche Gestaltung
  zählt aber
  ([BTC-Echo zu Krypto-Lending](https://www.btc-echo.de/news/defi-geschaeftsmodell-krypto-lending-ist-das-reguliert-132601/),
  [BTC-Echo zu Liquiditätsanbietern](https://www.btc-echo.de/news/yield-farming-interessiert-sich-die-bafin-fuer-liquidity-provider-150846/)).
- MiCA regelt Krypto-Lending heute **nicht**; genau das ist Gegenstand der
  Überprüfung 2026 (siehe 2.5).
- **Folge für Grundpfeiler 6:** Ein zinsfreies Leihprotokoll zwischen
  Menschen ist rechtlich offener als ein verzinstes. Ein Protokoll, in dem der
  Betreiber Mittel bündelt und weiterverleiht, ist das riskanteste Modell.
  Gegenseitiger Kredit zwischen Firmen (WIR/Sardex) läuft in der Schweiz über
  eine Bank, in Italien über eine Gesellschaft. **Beides nicht ohne
  Kanzlei.**

---

## Teil 4: DSGVO und AI Act

### 4.1 Der Worldcoin-Bescheid des BayLDA (Dezember 2024)

Der deutsche Präzedenzfall für Biometrie mit Kryptoguthaben
([Bescheid beim EDPB](https://www.edpb.europa.eu/system/files/2025-02/decision1594_0.pdf),
[ID Tech](https://idtechwire.com/german-regulator-orders-worldcoin-to-delete-biometric-data-over-gdpr-violations/),
[BeInCrypto](https://beincrypto.com/baylda-worldcoin-iris-deletion-ruling/)):

- **Löschung** der Iris-Codes aus Juli 2023 bis Dezember 2024, soweit sie
  für den passiven Abgleich weiter genutzt werden;
- **ein DSGVO-konformes Löschverfahren** innerhalb eines Monats;
- **ausdrückliche Einwilligung** für bestimmte Verarbeitungen;
- **Art. 32:** Iris-Codes lagen im Klartext in einer Datenbank.
- Worldcoin hat Rechtsmittel eingelegt.

**Abgleich mit Aequitas** (nach `aequitas-biometric-beta/docs/dsgvo/`):

| Punkt aus dem Bescheid | Aequitas heute |
|---|---|
| Klartext-Vorlagen | Keine Vorlage gespeichert, nur ein 64-Byte-Sketch, verschlüsselt; Seed je Validator; widerrufbar durch Seed-Wechsel ✅ |
| Löschverfahren | Selbstbedienung und Betreiber-Weg vorhanden ✅; **ein Merkmal bleibt nach Löschung**, damit niemand doppelt registriert. Die Datenschutzerklärung sagt das offen (Abschnitt 5). Beim BayLDA ging es um genau diese Frage: Iris-Codes, die nach Löschwunsch für den Abgleich weiterlebten. **Die Abwägung in der DSFA sollte den Bescheid ausdrücklich behandeln** ⚠️ |
| Ausdrückliche Einwilligung | Einwilligungstext vorhanden und korrigiert ✅; Verantwortlicher noch **[OFFEN]** ❌ |
| Festplattenverschlüsselung | **Nicht vorhanden** auf den Contabo-Servern (eigene TOM, 26.08.2026) ❌ |

### 4.2 Spanien und Portugal (März 2024)

- **AEPD (Spanien):** vorläufiges Verbot für bis zu drei Monate. Gründe:
  unzureichende Information, **Daten von Minderjährigen**, **Einwilligung
  nicht widerrufbar**
  ([AEPD](https://www.aepd.es/en/press-and-communication/press-releases/agency-orders-precautionary-measure-which-prevents-Worldcoin-from-continuing-toprocess-personal-data-in-spain)).
  Ein Gericht hat das Verbot bestätigt
  ([Canadian Lawyer](https://www.canadianlawyermag.com/news/international/spanish-court-upholds-temporary-ban-on-worldcoins-data-collection-activities/384772)).
- **CNPD (Portugal):** 90 Tage Aussetzung, vor allem weil **Minderjährige ohne
  Zustimmung der Eltern** erfasst wurden und es **keine Altersprüfung** gab
  ([Mobile ID World](https://mobileidworld.com/portugal-puts-worldcoin-on-pause/),
  [MediaNama](https://www.medianama.com/2024/03/223-worldcoins-data-collection-suspended-in-portugal-for-the-collection-of-minors-data/)).
  Worldcoin hat danach eine Altersprüfung und eine Löschfunktion eingeführt
  ([Cointelegraph](https://cointelegraph.com/news/worldcoin-world-id-unverify-iris-age-check)).

**Befund für Aequitas:** In der App (`Aequitas-App`) und im
Biometrie-Repo findet sich **keine Altersabfrage** und kein Hinweis auf ein
Mindestalter. Ein Grundeinkommen, das auch Kinder erreichen soll, ist eine
Wertentscheidung. Biometrie von Kindern ohne Zustimmung der Eltern ist aber
genau der Grund, aus dem zwei Aufsichtsbehörden Worldcoin gestoppt haben.
**Vorschlag:** Vor dem Öffnen des Biometrie-Riegels mindestens eine
Selbsterklärung „ich bin mindestens 18“ (oder das Alter, das die Kanzlei
nennt) in Einwilligung und App. Eine echte Altersprüfung ohne Ausweis ist
schwer; die Selbsterklärung ist der Mindeststandard, den beide Behörden
vermisst haben. **Nicht umgesetzt; braucht eure Entscheidung.**

### 4.3 EDPB-Leitlinien 02/2025 zu Blockchain (Fassung 2.0 vom 07.07.2026)

([v1.1](https://www.edpb.europa.eu/system/files/2025-04/edpb_guidelines_202502_blockchain_en.pdf),
[v2.0](https://www.edpb.europa.eu/system/files/2026-07/edpb_guidelines_202502_blockchain_v2_en.pdf),
[PPC Land](https://ppc.land/edpb-forces-blockchain-firms-to-avoid-storing-personal-data-on-chain/),
[Bitkom-Stellungnahme](https://www.bitkom.org/sites/main/files/2025-06/bitkom-stellungnahme-edsa-leitlinien-blockchain-datenschutz.pdf))

- Personenbezogene Daten gehören **nicht auf die Kette**, sondern daneben;
  auf der Kette höchstens Hashes oder Verweise.
- „Technisch unmöglich“ ist **keine** Entschuldigung für fehlende Löschung.
- Löschung kann über **Vernichtung des Schlüssels** oder der Daten neben der
  Kette erreicht werden.
- Knoten können **gemeinsam Verantwortliche** sein. Wer schreibt und
  validiert, entscheidet über Mittel der Verarbeitung (siehe auch CNIL und
  *Fashion ID*, EuGH
  ([McCann FitzGerald](https://www.mccannfitzgerald.com/knowledge/gdpr/blockchain-and-gdpr-what-can-we-learn-from-the-european-parliaments-recent-study),
  [Inside Privacy zur CNIL](https://www.insideprivacy.com/financial-institutions/the-cnil-publishes-report-on-blockchain-and-the-gdpr/))).

**Abgleich mit Aequitas:**

- **Gut:** Biometrie liegt nicht auf der Kette. Auf der Kette steht nur ein
  zufälliger Nullifier, der keine Funktion des Gesichts ist.
- **Schon erfasst:** Auf der Kette stehen dauerhaft und öffentlich die
  **Wallet-Adresse, das Merkmal „ist ein Mensch“** (`/api/humans`) und der
  ganze Zahlungsverlauf. DSFA (R4) und Datenschutzerklärung (Abschnitt 4)
  sagen das bereits offen. Neu ist nur der Bezug auf die Leitlinien: „technisch
  unmöglich“ reicht dort nicht als Begründung. Die Abwägung muss zeigen, dass
  **nichts auf der Kette steht, was nicht dort stehen muss**. Der Zahlungsverlauf
  einer Adresse, die einmal mit einem Namen verknüpft wurde (etwa im Laden),
  bleibt für immer nachverfolgbar.
- **Offen:** Wenn weitere Validatoren hinzukommen, werden sie nach diesen
  Leitlinien wahrscheinlich **gemeinsam Verantwortliche** (Art. 26). Eine
  Vereinbarung dafür sollte vor dem zweiten Validator stehen. Das Paket hat
  schon `06_AUFTRAGSVERARBEITUNG.md`; dort fehlt der Validator der Kette.

### 4.4 Pseudonym ist nicht anonym

- Nach den EDPB-Leitlinien zur Pseudonymisierung (Januar 2025) bleiben
  pseudonyme Daten personenbezogen, solange eine Wiederherstellung möglich
  ist ([Stibbe](https://stibbe.com/publications-and-insights/key-takeaways-and-insights-from-the-edpb-pseudonymisation-guidelines)).
- **Folge:** Der Sketch ist und bleibt Art.-9-Datum, so steht es bereits in
  den eigenen Unterlagen. Der Wallet-Marker (HMAC) ist personenbezogen für
  den, der den Schlüssel hat. Die Unterlagen sind hier ehrlich; das sollte
  so bleiben.

### 4.5 AI Act

- **Anhang III Nr. 1** (Hochrisiko): biometrische **Fernidentifizierung**,
  Kategorisierung nach sensiblen Merkmalen, Emotionserkennung.
  **Ausgenommen ist die Verifizierung** (1:1, „bin ich, wer ich behaupte“)
  ([IAPP](https://iapp.org/news/a/biometrics-under-the-eu-ai-act),
  [Bird & Bird](https://www.twobirds.com/en/insights/2023/global/biometrics-under-the-eu-ai-act)).
- **Fernidentifizierung** heißt: Identifizierung **ohne aktive Mitwirkung**,
  typischerweise aus der Ferne (Art. 3 Nr. 41, Erwägungsgrund 17)
  ([AI Act Service Desk](https://ai-act-service-desk.ec.europa.eu/en/ai-act/recital-17)).
- Der Aequitas-Abgleich ist **1:N** (gegen alle Eingeschriebenen), aber die
  Person **wirkt aktiv mit** (Kamera, Blinzeln, Blickrichtung). Er ist weder
  klassische Verifizierung noch Fernidentifizierung. **Vermutlich nicht
  Hochrisiko nach Anhang III Nr. 1 lit. a.** **(prüfen)** Das ist eine
  Grauzone, keine gefestigte Linie.
- **Zeitplan:** Das „Digital Omnibus“ hat die Pflichten für Hochrisiko nach
  Anhang III auf den **02.12.2027** verschoben; in Kraft seit 27.07.2026
  ([Gibson Dunn](https://www.gibsondunn.com/eu-ai-act-omnibus-agreement-postponed-high-risk-deadlines-and-other-key-changes/),
  [Cooley](https://cdp.cooley.com/digital-ai-omnibus-delays-key-deadlines-introduces-new-rules/)).
- **Verbote** (Art. 5, seit Februar 2025) betreffen Aequitas nicht: kein
  ungezieltes Sammeln von Gesichtsbildern, keine Emotionserkennung, keine
  Kategorisierung nach sensiblen Merkmalen.

---

## Teil 5: Was daraus folgt, nach Dringlichkeit

### Vor dem Öffnen des Biometrie-Riegels (`ALLOW_REAL_BIOMETRIC_DATA`)

1. **Verantwortlichen benennen** (Name, Anschrift). Ohne ihn sind
   Einwilligung, Datenschutzerklärung und Impressum unvollständig.
2. **Altersregel entscheiden und in Einwilligung und App aufnehmen** (4.2).
3. **Festplattenverschlüsselung** auf den Servern, die Sketches halten (4.1).
4. **DSFA-Abwägung zum verbleibenden Merkmal** ausdrücklich gegen den
   BayLDA-Bescheid prüfen lassen (4.1). Die Abwägung selbst steht schon da.
5. **DSFA-Abschnitt R4 (Kette)** mit den EDPB-Leitlinien 2.0 abgleichen
   (4.3).

### Vor echtem Wert (Stablecoin, Euro-Rampe, Pilotladen mit echter Ware)

6. **MiCA-Einordnung durch eine Kanzlei**: Whitepaper-Pflicht und Anbieter
   (2.2), Tausch-Pool als CASP-Dienst (2.3), E-Geld-Token am Pool (2.4).
7. **Keine eigene Euro-Rampe**; Rampe über lizenzierte Dienstleister (2.6).
8. **Vereinbarung über gemeinsame Verantwortung** vor dem zweiten Validator
   (4.3).

### Vor einem Kreditprotokoll

9. Die MiCA-Überprüfung 2026 abwarten (2.5) und das Modell mit der Kanzlei
   wählen (3.3). Bis dahin: **kein Kredit auf der Kette.**

### Was die Literatur für die Wirtschaft sagt (keine Rechtsfrage)

10. **Die Gefahr ist nicht die Regel, sondern die fehlende Masse.** Erfolgreiche
    Systeme haben ein dichtes Netz in einem kleinen Raum (Chiemgauer, Sarafu)
    oder einen echten Grund für Firmen (WIR-Kredit, Sardex-Makler). Für
    Aequitas heißt das: **ein Ort, mehrere Läden, die einander beliefern**,
    statt weltweiter Streuung (siehe `PILOTLADEN.md`).
11. **Nutzer gestalten die Regeln mit** (Ostrom, Gemeingut-Prinzipien 2026).
    Grundpfeiler 8 (Mitbestimmung) ist damit keine Kür, sondern eine
    Überlebensbedingung.

---

## Was dieses Dokument nicht leistet

- Es ersetzt **keine** Kanzlei. Die Punkte mit **(prüfen)** sind Folgerungen
  aus Sekundärquellen.
- Die Originaltexte (MiCA, ZAG, KWG, BayLDA-Bescheid, EDPB-Leitlinien) waren
  aus dieser Umgebung nicht abrufbar; zitiert sind Zusammenfassungen und
  Kanzleiartikel. Seyfang & Longhurst (2013) ist ohne Link aufgeführt.
- Es gibt **keine** veröffentlichte BaFin-Stellungnahme speziell zu
  Regionalgeld oder zu verschenkten Kryptoguthaben, die gefunden wurde.
