# Fragen an eine Kanzlei – Unternehmen und AEQ

Stand 02.10.2026. Diese Fragen müssen von einer Kanzlei mit Erfahrung in
Kryptowerten, Zahlungsverkehr und Steuerrecht beantwortet werden, **bevor ein
Laden echte Ware gegen AEQ abgibt** und **bevor ein echter Stablecoin
angebunden wird**. Dieses Dokument ist keine Rechtsauskunft; es sammelt die
Fragen und die Fakten, die die Kanzlei braucht.

## Fakten zum System (für die Kanzlei)

- AEQ entsteht nur bei der Registrierung eines geprüften Menschen
  (1.000 AEQ je Mensch). Niemand kann AEQ kaufen, um neues zu schaffen.
- Ein Grundeinkommen wird täglich aus Gebühren und Abgaben an alle Menschen
  gleich verteilt.
- Die Kette betreibt einen eingebauten Tausch (Liquiditätspool) zwischen AEQ
  und einer Stable-Münze. Heute ist das **tUSD, reines Testgeld** ohne
  Gegenwert.
- Betrieben wird die Kette heute von einem Betreiber auf einem Server; weitere
  unabhängige Validatoren sind vorgesehen.
- Unternehmenskonten werden von verifizierten Menschen eröffnet; das Netz prüft
  keine Firmenidentität (Konzept Abschnitt 8).
- Regeln: `docs/UNTERNEHMEN_KONZEPT.md`; Code öffentlich auf GitHub.

## A. Aufsicht (MiCA, ZAG, KWG)

1. Ist AEQ ein Kryptowert im Sinne der MiCA-Verordnung, und wenn ja, welcher
   Art (E-Geld-Token, vermögenswertreferenziert, sonstiger)?
2. Ist der eingebaute Tausch AEQ ↔ Stable eine Krypto-Dienstleistung (Tausch,
   Handelsplattform)? Wer wäre der Anbieter: der Betreiber, jeder Validator,
   niemand?
3. Ändert sich die Antwort, wenn statt tUSD ein regulierter Euro-Stablecoin
   (z. B. EURC) angebunden wird?
4. Ist das tägliche Grundeinkommen aus Gebühren eine erlaubnispflichtige
   Tätigkeit?
5. Braucht ein Betreiber, der eine App und eine Website für Zahlungen zwischen
   Kundschaft und Läden anbietet, eine Erlaubnis nach ZAG?
6. Was gilt in der Beta, solange es nur Testgeld gibt?

## B. Geldwäsche (GwG, EU-AMLR)

7. Wer ist Verpflichteter, sobald echtes Geld ein- und ausgeht?
8. Reicht die Personenprüfung (einmal je Mensch, ohne Namen) für irgendeine
   Sorgfaltspflicht, oder ist sie rechtlich bedeutungslos?
9. Müssen Unternehmenskonten identifiziert werden (Name, Register), wenn das
   Netz das bewusst nicht tut? Wer müsste es tun?
10. Gilt die Travel Rule für Überweisungen auf der Kette?

## C. Steuern

11. Wie bucht ein Laden AEQ-Einnahmen, solange es keinen Euro-Kurs gibt?
    Tausch gegen Ware? Sachbezug? Werbeaufwand?
12. Wie wird das Grundeinkommen bei Menschen steuerlich behandelt?
13. Wie werden Löhne in AEQ behandelt (Lohnsteuer, Sozialversicherung)?
14. Umsatzsteuer: Ist die Zahlung mit AEQ ein Tausch mit Bemessungsgrundlage,
    und welcher Wert gilt?
15. Sind Liegegeld und Ausstiegsabgabe für Unternehmen abzugsfähige
    Betriebsausgaben?

## D. Verbraucher und Haftung

16. Welche Pflichtangaben braucht die Website (Impressum, Datenschutz,
    Risikohinweise), und welche Formulierungen sind zu vermeiden
    („fairstes Geld“, „Grundeinkommen“)?
17. Wer haftet, wenn ein Laden durch die Vermögensgrenze Geld verliert
    (Überschuss wird verteilt) oder ein Fehler im Code Guthaben verändert?
18. Darf ein Laden AEQ nur teilweise annehmen und den AEQ-Preis selbst
    festlegen?

## E. Biometrie (zur Vollständigkeit, eigenes Thema)

19. DSGVO Art. 9: Grundlage für die Gesichtsprüfung heute und die Iris später;
    Drittlandübermittlung (Server außerhalb der EU).

## F. Kredit (Vorschlag in `docs/WIRTSCHAFT_REIFEPRUEFUNG.md`, Abschnitt 4)

20. Ist ein Protokoll, über das Menschen einander zinsfrei Geld leihen (mit
    automatischer Rückzahlung aus künftigen Eingängen), ein erlaubnispflichtiges
    Kreditgeschäft oder eine Kreditvermittlung? Für wen: Betreiber,
    Validatoren, niemand?
21. Ist die automatische Rückzahlung aus Eingängen zulässig, und welche
    Pfändungsgrenzen gelten entsprechend?
22. Ist ein gegenseitiger Kredit zwischen Unternehmen (Minus bis zu einer
    Grenze, wie WIR oder Sardex) ein Einlagen- oder Kreditgeschäft?
23. Darf eine Schuld im Protokoll nach drei Jahren erlöschen, und was
    bedeutet das für die Forderung außerhalb der Kette?

## G. Einordnung als Gutschein- oder Zahlungssystem

24. Die BaFin legt die Ausnahme für „begrenzte Netze“ eng aus; Gutscheine,
    die bei mehreren unabhängigen Unternehmen gelten, laufen nach ihren
    Hinweisen in der Regel über ein E-Geld-Institut. Der Chiemgauer ist
    euro-gedeckt, AEQ nicht. Wie ist AEQ einzuordnen, und was folgt daraus für
    einen Pilot mit mehreren Läden in der Beta (Testgeld) und danach?

## H. MiCA Titel II (Angebot, Whitepaper)

25. Die Whitepaper-Pflicht entfällt für kostenlos angebotene Kryptowerte. Nach
    Darstellung von Kanzleien gilt ein Angebot nicht als kostenlos, wenn der
    Anbieter personenbezogene Daten verlangt. Aequitas gibt 1.000 AEQ nur nach
    einer Gesichtsprüfung. Ist das Startguthaben ein öffentliches Angebot mit
    Whitepaper-Pflicht? Wer wäre Anbieter?
26. Greifen in der Beta andere Ausnahmen (weniger als 150 Personen je
    Mitgliedstaat, unter 1 Mio. € in 12 Monaten), und was passiert beim
    Überschreiten?
27. Entfällt eine Ausnahme, wenn Website oder Kanäle einen späteren Handel
    oder Umtausch ankündigen?

## I. Ergänzungen aus der Recherche vom 02.10.2026 (`RECHT_UND_LITERATUR.md`)

28. Nach Art. 4 Abs. 3 MiCA ist ein Angebot nicht kostenlos, wenn Erwerber
    personenbezogene Daten geben müssen. Wer ist bei Aequitas „Anbieter“, wenn
    die Kette das Startguthaben schreibt, der Betreiber aber App, Registrierung
    und Coordinator führt? Ab welchem Grad der Verteilung (Validatoren,
    Coordinatoren) gäbe es keinen Anbieter mehr?
29. AEQ wird nicht gegen Zahlung ausgegeben. Ist es damit sicher kein E-Geld
    nach § 1 Abs. 2 ZAG, und sind Überweisungen von AEQ damit keine
    Zahlungsdienste?
30. Nach dem No-Action-Letter der EBA (Juni 2025) gelten Übertragung und
    Verwahrung von E-Geld-Token ab 02.03.2026 auch als Zahlungsdienst. Was
    bedeutet das für einen Pool AEQ ↔ EURC auf der eigenen Kette?
31. Fällt ein zinsfreies Leihprotokoll in AEQ unter § 1 KWG, unter MiCA oder
    unter keines von beiden? Was ändert die MiCA-Überprüfung 2026?

## J. Datenschutz und AI Act

32. Welches Mindestalter gilt für die Gesichtsprüfung, und reicht eine
    Selbsterklärung? (Spanien und Portugal haben Worldcoin 2024 wegen
    Minderjährigen gestoppt; Aequitas fragt heute kein Alter ab.)
33. Hält die Abwägung „ein Merkmal bleibt nach Löschung, damit niemand doppelt
    bezieht“ dem BayLDA-Bescheid gegen Worldcoin (Dezember 2024) stand?
34. Sind künftige Validatoren der Kette gemeinsam Verantwortliche nach
    Art. 26 DSGVO (EDPB-Leitlinien 02/2025, Fassung 2.0)?
35. Ist der 1:N-Abgleich bei aktiver Mitwirkung eine „biometrische
    Fernidentifizierung“ nach Anhang III AI Act oder nicht?
36. Ist eine vorherige Konsultation der Aufsicht nach Art. 36 DSGVO nötig?

## Was wir von der Kanzlei brauchen

Für jede Frage: Antwort, Begründung, was **vor der Beta**, was **vor einem
Pilotladen** und was **vor echtem Geld** erledigt sein muss.
