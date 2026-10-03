# Aequitas für Unternehmen – Konzept

**Fassung 4 · Stand 02.10.2026.** Diese Fassung ersetzt alle früheren. Jede
Zahl in den Abschnitten 2 bis 7 ist gegen `x/humanity/keeper/wirtschaft.go`
geprüft; der Name der Konstante steht dabei. Weicht der Code ab, gilt der Code,
und dieses Dokument ist falsch. Was sich seit Fassung 1 geändert hat und warum,
steht in Abschnitt 14. Die Analyse vom 02.10. (früher
`docs/UNTERNEHMEN_ANREIZE.md`) ist hier eingearbeitet.

**Regeln aktiv seit:** 26.09.2026, 15:00 UTC (`wirtschaftAktivAbUnix`). Die
heutige Kette beginnt am 30.09.2026; die Regeln gelten auf ihr vom ersten
Block an.

**Zweite Stufe ab 03.10.2026, 12:00 UTC (14:00 MESZ)** (`wirtschaft2AktivAbUnix`,
`x/humanity/keeper/wirtschaft2.go`), aus der Reifeprüfung
(`docs/WIRTSCHAFT_REIFEPRUEFUNG.md`):
- **C** Zahlungen über die Vermögensgrenze werden **abgelehnt statt
  weggenommen** (2, 3).
- **A** Das erste Unternehmen eines Menschen ist **dauerhaft nie schlechter
  als ein Mensch** (4.4).
- **B** Eingänge von anderen Unternehmen zählen **je zahlender Firma bis
  9.000 AEQ im Quartal**; Weitergeben an Lieferanten kostet keinen Umsatz mehr
  (4.3).

Der Zeitpunkt muss nach dem Ausrollen auf alle Knoten liegen. Wird bis dahin
nicht ausgerollt, wird er vorher verschoben.

---

## 0. In drei Sätzen

**Für Läden:** Annehmen kostet nichts, Löhne kosten nichts, bis anderthalb
Monatsumsätze (mindestens 2.000 AEQ) liegen frei. Nur Geld, das weit darüber
liegen bleibt, kostet 0,5 bis 1 % im Monat, und das geht ans Grundeinkommen.

**Für Menschen:** 1.000 AEQ im Monat kostenlos ausgeben, 5.000 AEQ kostenlos
sparen, 3.000 AEQ im Monat ohne Abgabe in Stable tauschen.

**Für das Netz:** Unternehmen werden nicht geprüft, Menschen schon. Die Regeln
wirken auf jedes AEQ, egal wer es hält; darum muss niemand wissen, wer hinter
einem Konto steht.

## 0a. Beta: Menschen zuerst, Unternehmen im Pilot

**Entscheidung für die Beta (02.10.2026):** Die Beta ist für Menschen.
Unternehmen sind nicht abgesagt, sie kommen in einer **Pilotstadt** dazu,
sobald dort genug Menschen registriert sind (Richtwert 50 bis 100).

- **Kein neuer Code für Unternehmen vor dem Pilot.** Die Regeln auf der Kette
  bleiben, sie sind getestet und belasten niemanden, der sie nicht nutzt. Neue
  Regeln kommen erst mit Zahlen aus dem Pilot.
- **Der erste Laden braucht kein Unternehmenskonto.** Die Inhaberin nimmt AEQ
  auf ihr Menschenkonto, mit selbst gesetzter Grenze
  (`docs/PILOTLADEN.md`). Achtung: Ein Menschenkonto hat eine
  Vermögensgrenze (Phase 0: 5.000 bis 25.000 AEQ); was darüber eingeht, wird
  verteilt.
- **Anmeldung, Kasse und Verzeichnis in der App** werden gebaut, wenn der erste
  Laden sie braucht (Abschnitt 13).
- **Recht vor Ware:** Bevor ein Laden echte Ware gegen AEQ abgibt, beantwortet
  eine Kanzlei die Fragen in `docs/RECHTSFRAGEN_UNTERNEHMEN.md`.
- **Nach außen ehrlich:** Die Unternehmensseite der Website sagt, dass es in
  der Beta keinen Euro-Wert, keine Kasse und keinen Export gibt
  (geändert am 02.10.2026, 12 Sprachen).

> **Zahlenprüfung (02.10.2026):** `docs/WIRTSCHAFT_ZAHLENPRUEFUNG.md` zeigt,
> dass die Monatsgrenzen eine ungemessene Umlaufgeschwindigkeit voraussetzen
> und das Liegegeld-Fenster von 1,5 Monatsumsätzen Betriebe mit kleiner Marge
> hart trifft (drei Monate Rücklage kosten einen Supermarkt über ein Drittel
> seines Gewinns). Vorschläge dort, noch nicht beschlossen.

> **Reifeprüfung (02.10.2026):** `docs/WIRTSCHAFT_REIFEPRUEFUNG.md` prüft jede
> Regel auf Fairness und Anreiz, findet drei Schwächen (Kleinbetriebe,
> Weitergabe an Lieferanten, Vermögensgrenze bei Eingängen), beschreibt die
> Verifizierung von Unternehmen in Schichten und Kredit ohne Banken. Die
> Vorschläge dort sind noch nicht beschlossen.

## 1. Grundsätze

1. **Das Geld ist für Menschen.** AEQ entsteht nur, wenn sich ein Mensch
   registriert (1.000 AEQ, `registrationGrant`). Grundeinkommen und
   Stimmrecht haben nur Menschen. Für Unternehmen entsteht kein AEQ.
2. **Unternehmen sind Durchlauf, kein Speicher.** Weitergeben ist billig,
   Liegenlassen kostet.
3. **Kein Vorrecht.** Jeder Vorteil eines Unternehmenskontos hat einen Preis.
   Alles, was irgendwer zahlt, geht zu 100 % ins Grundeinkommen.
4. **Wir prüfen Menschen, nicht Unternehmen.** Siehe Abschnitt 8.
5. **Einfach für den Laden.** Regeln, die man in einem Satz sagen kann; kein
   Alter des Geldes, ein AEQ ist wie das andere.

## 2. Drei Kontoarten

Welche Art ein Konto hat, folgt aus dem Zustand (`kontoartVon`): erst
Protokoll-Topf, dann Mensch, dann offenes Unternehmen, sonst freie Adresse.

| | **Mensch** | **Unternehmen** | **Freie Adresse** |
|---|---|---|---|
| Wer | verifizierter Mensch | eröffnet von einem Menschen, bis 10 Verantwortliche | jede andere Adresse, auch Verträge |
| Grundeinkommen, Stimme | ja | nein | nein |
| Höchstbetrag | 25.000 AEQ¹; ab 03.10.2026 werden Eingänge darüber abgelehnt | keiner | **250 AEQ** (`freiGrenze`), Eingänge darüber abgelehnt |
| Halten kostet | 0,5 %/Monat auf den Teil über **5.000 AEQ** | Liegegeld nach Umsatz (4.2) | **1 %/Monat** ab dem ersten AEQ |
| Überweisen | erste **1.000 AEQ/Monat** frei, dann 0,1 % | an Menschen **0 %**, sonst 0,1 % | 0,1 % |
| In Stable tauschen | **3.000 AEQ/Monat** ohne Abgabe, dann 2 % | 2 % | 2 % |
| eigene Einlage zurücktauschen | ohne Abgabe | ohne Abgabe | ohne Abgabe |

¹ Phase 0: `max(5, min(N, 25)) × 1.000 AEQ` bei N Menschen. Ab 25 Menschen
dauerhaft 25.000 AEQ. Mit einem Menschen auf der Kette (Stand 02.10.) sind es
5.000 AEQ.

Protokoll-Töpfe (Grundeinkommen, Liquidität, Validatoren) sind ausgenommen;
sie sind Durchlauf.

Alle Beträge sind Vielfache des fairen Anteils (1.000 AEQ), nicht an Euro oder
Dollar gekoppelt. Warum: Abschnitt 9.

## 3. Regeln für Menschen

| Regel | Wert | Konstante |
|---|---|---|
| gebührenfreie Ausgaben | 1.000 AEQ je Kalendermonat | `menschFreiAusgabenMonat` |
| Gebühr darüber | 0,1 %, vom Absender obendrauf | `ueberweisungsGebuehrBps` |
| Tausch AEQ → Stable ohne Abgabe | 3.000 AEQ je Kalendermonat, egal woher das Geld kam | `menschTauschFreiMonat` |
| Ausstiegsabgabe darüber | 2 % | `ausstiegsAbgabeBps` |
| Sparfreibetrag | 5.000 AEQ; LP-Anteile zählen mit | `menschSparFreibetrag` |
| Umlauf darüber | 0,5 % im Monat, täglich anteilig, ohne Schonfrist | `menschUmlaufMonat` |

- **Kalendermonat** heißt: Die Zähler springen am Monatsersten 00:00 UTC auf
  null (`kontoLocked`), nicht 30 Tage nach der ersten Zahlung.
- **Eigene Einlage:** Was ein Konto selbst von Stable in AEQ getauscht hat,
  geht ohne Abgabe zurück, bevor der Monatsfreibetrag angegriffen wird
  (`nachTausch`). Wer einzahlt und abhebt, gewinnt nichts und nimmt niemandem
  etwas.
- Die Tauschgebühr von 0,1 % (`swapFeeBps`) gilt für jeden Tausch zusätzlich;
  sie geht an Validatoren, Liquiditätsgeber und Grundeinkommen.
- Die frühere Demurrage (über 1.000 AEQ, erst nach 90 Tagen ohne Bewegung) ist
  seit der Aktivierung abgeschaltet (`effectiveBalance`).
- **Vermögensgrenze (ab 03.10.2026):** Eine Überweisung oder ein Tausch, der
  einen Menschen über die Grenze bringen würde, wird abgelehnt
  (`pruefeVermoegensgrenzeAnnahmeLocked`). Das Geld bleibt beim Absender, der
  Empfänger verliert nichts. Gezählt wird wie bei der Kappung: Guthaben plus
  Wert der LP-Anteile. Weggenommen und verteilt wird nur noch, was niemand
  steuert (Grundeinkommen, Freigaben, Registrierung). Vorher ging die Zahlung
  durch, und der Überschuss wurde verteilt.

## 4. Regeln für Unternehmen

### 4.1 Eröffnen, Mitinhaber, Schließen

- **Eröffnen** (`unternehmen_eroeffnen`): unterschrieben von der
  Unternehmensadresse **und** einem registrierten Menschen, innerhalb von
  5 Minuten eingereicht. Die Adresse darf kein Mensch, kein Protokoll-Topf und
  kein offenes Unternehmen sein. Name freiwillig, höchstens 60 Zeichen
  (`normName`). Kategorie aus einer festen Liste: Lebensmittel, Gastronomie,
  Handel, Handwerk, Dienstleistung, Gesundheit, Bildung, Kultur, Verein,
  Landwirtschaft, Technik, Sonstiges.
- **Grenzen:** Ein Mensch ist für höchstens **3 offene Unternehmen**
  verantwortlich (`maxUnternehmenJeMensch`), ein Unternehmen hat höchstens
  **10 Verantwortliche** (`maxVerantwortlicheJeUnt`).
- **Mitinhaber** (`unternehmen_mitinhaber`): unterschrieben vom neuen Menschen
  und einem bisherigen Verantwortlichen. Die Person, die eröffnet hat, bleibt
  als Gründerin an erster Stelle.²
- **Schließen** (`unternehmen_schliessen`): nur durch einen Verantwortlichen
  und nur mit leerem Konto. Danach ist die Adresse eine freie Adresse.
- **Austreten** gibt es nicht. Ein Verantwortlicher bleibt eingetragen, bis das
  Konto geschlossen wird (offene Entscheidung, Abschnitt 12).
- **Öffentlich** (`/api/unternehmen`): Name, Kategorie, Zahl der
  Verantwortlichen, Eröffnung, Guthaben und je Kalendermonat Einnahmen, gezahlte
  Löhne und Entnahmen. Die Wallets der Verantwortlichen stehen in der
  Eröffnungstransaktion und sind damit auf der Kette sichtbar.

² Bis 02.10.2026 wurde die Liste beim Aufnehmen alphabetisch sortiert. Ein
Mitinhaber mit kleinerer Adresse wurde so zum „Gründer“, und eine zweite Firma
der echten Gründerin bekam im selben Jahr noch einmal die Gründungsphase
(4.4). Korrigiert auf `claude/beta-launch-business-integration-vr7u4c`, Test
`TestGruenderBleibtNachMitinhaber`; vor dem Merge eigene Sicherheitsprüfung.

### 4.2 Liegegeld: Freibetrag nach Umsatz

| Guthaben | Liegegeld pro Monat | Konstante |
|---|---|---|
| bis **1,5 × Monatsumsatz**, mindestens **2.000 AEQ** | 0 | `umsatzFreiFaktor`, `unternehmenSockel` |
| bis **3 × Monatsumsatz** (mindestens 2.000 AEQ) | **0,5 %** auf diesen Teil | `umsatzStufe2Faktor`, `liegeRate1Monat` |
| darüber | **1 %** auf diesen Teil | `liegeRate2Monat` |

Ohne Umsatz liegen also 2.000 AEQ frei, und alles darüber kostet 1 %/Monat,
genau so viel wie auf freien Adressen. Ein Unternehmenskonto ist zum Horten nie
billiger als eine freie Adresse; es spart höchstens die 20 AEQ, die auf den
Sockel entfallen würden.

### 4.3 Was als Monatsumsatz zählt

**Monatsumsatz** = der höhere Wert aus dem Durchschnitt der letzten **90 Tage**
und dem der letzten **365 Tage** (Saisonbetriebe), auf 30 Tage gerechnet
(`umsatzLocked`). Ein neues Unternehmen wird über **mindestens 30 Tage**
gemittelt (`umsatzMindestTage`), damit wenige gute Tage nicht hochgerechnet
werden.

Es zählen nur **anrechenbare Eingänge**:

| Eingang | zählt | Grund |
|---|---|---|
| Einkauf eines Menschen | ja, je Mensch höchstens **9.000 AEQ je Kalenderquartal und Unternehmen** (`menschZaehltJeUntQuartal`) | echter Umsatz, gegen Aufblähen gedeckelt |
| … von einem eigenen Verantwortlichen | nein | sonst kauft man sich den Freibetrag selbst |
| Zahlung des Unternehmens an denselben Menschen | zieht dessen schon gezählte Einkäufe dort ab (laufendes und voriges Quartal, `rueckzahlungLocked`) | einkaufen und das Geld zurückbekommen ist kein Umsatz |
| von anderen Unternehmen | bis 03.10.2026, 12:00 UTC, nur der **Überschuss** (Eingänge minus Zahlungen an Unternehmen im selben Fenster). **Ab 03.10.2026 der höhere Wert** aus Überschuss und der Summe der Eingänge, **je zahlender Firma höchstens 9.000 AEQ im Kalenderquartal**; zahlt die Firma später an dieselbe Firma zurück, hebt das deren Gezähltes auf (`zahlungZwischenFirmenLocked`) | Weitergeben an Lieferanten soll nicht kosten. Gefälschter Umsatz ist auf den Deckel je zahlender Firma begrenzt (8.1, Nr. 3) |
| … von Unternehmen mit gemeinsamen Verantwortlichen | nein | eigene Firmen zählen füreinander nicht |
| von freien Adressen | nein | ohne Menschen dahinter |
| Einstieg aus Stable | nein | Einzahlung ist kein Umsatz |

Löhne und Entnahmen sind Ausgänge und senken den Umsatz nicht. Eine Zahlung an
einen Verantwortlichen heißt Entnahme, an jeden anderen Menschen Lohn; beides
kostet 0 % und wird öffentlich summiert.

### 4.4 Gründungsphase und erstes Unternehmen

Im **ersten halben Jahr** (`gruendungTage` = 182) werden Gründerin und Firma
zusammen nie schlechter und nie besser gestellt als ein Mensch, der das Geld
selbst hält (`liegegeldLocked`):

- Das Guthaben der Gründerin und der Firma zählen zusammen.
- Bis zur Grenze für Menschen (25.000 AEQ) kostet das Geld in der Firma, was es
  zusätzlich bei der Gründerin kosten würde: 0,5 %/Monat über 5.000 AEQ.
- Was darüber liegt, zahlt nach 4.2. Es gilt der niedrigere der beiden Werte.
- **Einmal je Mensch in 365 Tagen** (`gruendungAbstandTage`): Hat dieselbe
  Person in den 365 Tagen davor schon ein Unternehmen eröffnet (auch ein
  inzwischen geschlossenes), gibt es keine Gründungsphase.

Beispiel: Gründerin hält 1.000 AEQ, Firma 20.000 AEQ. Normal:
18.000 × 1 % = 180 AEQ/Monat. In der Gründungsphase: (21.000 − 5.000) × 0,5 %
= 80 AEQ/Monat.

**Ab 03.10.2026 gilt dieselbe Rechnung dauerhaft für das erste Unternehmen**
(`erstesOffenesLocked`): das älteste noch offene Unternehmen jeder Gründerin
wird nie schlechter gestellt als ein Mensch, auch nach dem ersten halben Jahr.
Weitere Unternehmen derselben Person zahlen nach 4.2. Wird das erste
geschlossen, rückt das nächste nach; es ist immer höchstens eins. Beispiel:
Laden mit 1.000 AEQ Monatsumsatz hält 4.000 AEQ, Gründerin 1.000 AEQ:
bisher 15 AEQ/Monat, ab 03.10. 0, wie bei einem Menschen mit 5.000 AEQ.

Hinweis: Die Gründungsphase rechnet immer mit 25.000 AEQ, auch in Phase 0, in
der Menschen weniger halten dürfen. Das begünstigt Gründer, solange es weniger
als 25 Menschen gibt; bei 25 und mehr fällt es weg.

### 4.5 Überweisungen und Ausstieg

| Richtung | Gebühr |
|---|---|
| Mensch → Unternehmen (Einkauf) | wie jede Ausgabe eines Menschen: erste 1.000 AEQ/Monat frei, dann 0,1 %, vom Kunden. Der Laden bekommt den vollen Preis |
| Unternehmen → Mensch (Lohn, Entnahme, Erstattung) | **0 %** |
| Unternehmen → Unternehmen | 0,1 % |
| Unternehmen → freie Adresse | 0,1 % |
| Unternehmen: AEQ → Stable | 2 % Abgabe, außer auf die eigene Einlage; kein Monatsfreibetrag |

## 5. Freie Adressen

- Höchstens **250 AEQ** (0,25 × fairer Anteil). Geprüft bei jeder Annahme:
  Überweisung, Schnellpfad und Tausch Stable → AEQ. Ein Eingang, der die
  Grenze überschreiten würde, wird abgelehnt.
- **1 %/Monat** ab dem ersten AEQ.
- Gedacht für Besucher, Trinkgeldkassen, Verträge und die Zeit vor der
  Registrierung. Wer mehr halten will, braucht ein Unternehmenskonto.

## 6. Tageslauf und wer rechnet

- Liegegeld und Umlauf werden **täglich** anteilig verrechnet
  (`umlaufLocked`), je Lauf höchstens für 7 Tage. Beträge unter **0,001 AEQ** je
  Konto und Lauf werden nicht eingezogen (`umlaufMindestBetrag`).
- **Der erzeugende Knoten rechnet**, die Beträge stehen im Block.
  Nachspielende Knoten rechnen nach (`liegegeld_pruefung.go`), **zählen
  Abweichungen aber nur** (Beobachtungsmodus). Abgelehnt wird auf allen
  Knoten zugleich ab dem gemeinsamen Stichtag `nachrechnenStrengAbUnix`
  (nachrechnen.go); `AEQUITAS_LIEGEGELD_PRUEFUNG=streng` stellt einen
  einzelnen Knoten nur früher streng. Solange der Stichtag nicht gesetzt
  ist, vertraut das Netz beim Liegegeld dem Erzeuger. Siehe Abschnitt 11.
- **Fehlende Buchführung geht zugunsten der Kontoinhaber:** Hat ein Knoten
  weniger als 30 Tage Daten (nach dem Start oder nach einem Snapshot),
  berechnet er kein Liegegeld.
- Die Buchführung liegt in derselben Datenbank-Transaktion wie die
  Kontostände; ein abgebrochener Block nimmt sie mit zurück.

## 7. Rechenbeispiele (geltende Sätze)

### Unternehmen

| Betrieb | Lage | Liegegeld/Monat |
|---|---|---|
| Café | 3.000 AEQ Umsatz, hält 1.500 | **0** (frei bis 4.500) |
| Supermarkt, normale Reserve | 40.000 Umsatz, hält 80.000 | 20.000 × 0,5 % = **100** |
| Supermarkt, hortet | 40.000 Umsatz, hält 200.000 | 60.000 × 0,5 % + 80.000 × 1 % = **1.100** |
| Konzern | 10 Mio. Umsatz von Menschen, hält 15 Mio. | **0** |
| Großhändler | 500.000 von Läden, 400.000 an Hersteller, hält 250.000 | Überschuss 100.000, frei bis 150.000: **500** |
| Hülle ohne Umsatz | hält 100.000 | 98.000 × 1 % = **980** |
| zum Vergleich: 400 freie Adressen | halten 100.000 | **1.000** |

Ehrlich: Großhändler und Hersteller, die vor allem an Unternehmen verkaufen,
haben als Umsatz nur ihre Marge und zahlen darum mehr als ein Laden gleicher
Größe (hier 0,1 % vom Umsatz, etwa eine niedrige Kartengebühr).

### Menschen

- **Anna**, 1.200 AEQ, gibt 800 im Monat aus: zahlt **0**.
- **Ben** bekommt 2.000 Lohn, gibt 1.500 aus, tauscht 1.000: 0,5 AEQ
  Überweisungsgebühr + 1 AEQ Tauschgebühr = **1,5 AEQ**.
- **Clara**, 20.000 AEQ, gibt 3.000 aus, tauscht 5.000: 2 (Überweisung) + 75
  (Umlauf auf 15.000) + 40 (Abgabe auf 2.000) + 5 (Tauschgebühr) =
  **122 AEQ**, alles ans Grundeinkommen bis auf die Tauschgebühr.

## 8. Wie prüft man Unternehmen weltweit? Gar nicht.

Menschen werden geprüft: heute mit der Gesichtsprüfung, zum richtigen Launch
mit der Iris. Unternehmen werden nicht geprüft. Es gibt rund 200 Länder mit
eigenen Registern, viele nicht öffentlich oder nicht verlässlich; wer sie
prüft, ist eine zentrale Stelle, die entscheidet, wer mitmachen darf.

Das trägt aus zwei Gründen:

1. **Jedes Unternehmen hängt an einem verifizierten Menschen.** Weil jeder
   Mensch nur einmal existiert, sind die Grenzen pro Mensch echt: 3 Firmen,
   eine Gründungsphase im Jahr, 9.000 AEQ gezählter Umsatz je Quartal und
   Firma.
2. **Die Regeln wirken auf jedes AEQ.** Der richtige Vergleich für ein
   Unternehmenskonto ist nicht der Mensch, sondern die freie Adresse, die jeder
   ohne Prüfung anlegen kann. Ein Unternehmenskonto ohne echten Umsatz ist zum
   Horten nicht billiger als freie Adressen (4.2, Beispiel 7).

**Was das Netz nicht ersetzt:** Wo echtes Geld ein- und ausgeht
(Euro-Stablecoin, Börsen), gelten Regeln gegen Geldwäsche, und dort wird
geprüft. Ob der eingebaute Tausch selbst Pflichten auslöst, ist Teil der
MiCA-Prüfung (Abschnitt 11). „Das Netz prüft Menschen, nicht Unternehmen“ ist
eine Entscheidung über das Protokoll, keine Rechtsauskunft.

### 8.1 Bekannte Angriffe und ihr Stand

| # | Angriff | Stand | Warum |
|---|---|---|---|
| 1 | Als Unternehmen die 25.000-Grenze umgehen | **begrenzt** | ohne Umsatz 1 %/Monat über 2.000, so teuer wie freie Adressen |
| 2 | Viele Firmen für viele Sockel | **begrenzt** | 3 offene je Mensch: höchstens 6.000 AEQ, spart rund 30 AEQ/Monat |
| 3 | Kreis zwischen Firmen (A → B → C → A) | bis 03.10., 12:00 UTC, **geschlossen**; danach **begrenzt** | Damit Weitergeben an Lieferanten nicht mehr kostet, zählen Eingänge von Firmen je Zahler bis 9.000 AEQ im Quartal. Gezählt wird je zahlender Firma höchstens dieser Deckel (rund 3.000 AEQ Monatsumsatz, Ersparnis bis etwa 70 AEQ/Monat je zahlendem Partner); im Kreis hat jede Firma einen Zahler, bei einem Stern zählt jeder Zahler einzeln, und jede zahlende Firma braucht einen eigenen verifizierten Menschen (gemeinsame Verantwortliche zählen nicht); hin und zurück zwischen zwei Firmen bringt nur einer Seite etwas. Bewusster Tausch, siehe `WIRTSCHAFT_REIFEPRUEFUNG.md` 3.2 B |
| 4 | Eigene Zahlungen als Umsatz | **geschlossen** | Verantwortliche und Firmen mit gemeinsamen Verantwortlichen zählen nicht |
| 5 | Freund kauft ein, bekommt das Geld als Lohn zurück | **geschlossen** für diese Reihenfolge | `rueckzahlungLocked` |
| 6 | Firma zahlt zuerst, Freund kauft danach mit demselben Geld | **begrenzt, nicht geschlossen** | Die Rückzahlung zieht nur schon Gezähltes ab. Nachgestellt: 3.740 AEQ Monatsumsatz statt 0. Eine Reparatur hilft nicht, weil das Geld sonst über einen zweiten Freund läuft. Grenze: 9.000 AEQ je Mensch, Quartal und Firma; Ersparnis höchstens rund 45 AEQ/Monat je eingespanntem Menschen, öffentlich in den Lohnsummen |
| 7 | Quartalswechsel: 9.000 kurz vor und 9.000 kurz nach dem Wechsel | **begrenzt** | Kalenderquartal, also bis 18.000 AEQ in wenigen Tagen je Mensch; danach wieder ein Quartal Pause |
| 8 | Gründungsphase mehrfach über Mitinhaber-Reihenfolge | **behoben auf dem Branch** | siehe ² in 4.1 |
| 8a | Erstes-Unternehmen-Regel mehrfach nutzen (drei Firmen, alle „wie ein Mensch“) | **geschlossen** | nur das älteste offene Unternehmen je Gründerin; Platz mit ihrem eigenen Guthaben geteilt (`TestNurDasErsteUnternehmenWieEinMensch`) |
| 8b | Jemanden durch eine Zahlung über die Grenze schädigen | ab 03.10. **geschlossen** | Zahlung wird abgelehnt statt weggenommen |
| 9 | Jeden Monat eine neue Firma ohne Liegegeld | **geschlossen** | keine Schonfrist; Gründungsphase einmal je 365 Tage und nie besser als ein Mensch |
| 10 | Unternehmenskonto verkaufen (eigener Schlüssel) | **wirtschaftlich wertlos**, Anzeige offen | Der Käufer spart gegenüber freien Adressen 20 AEQ/Monat. Aber das Register nennt dann einen Menschen, der nichts mehr zu sagen hat |
| 11 | Vertrag als Versteck | **begrenzt** | Verträge sind freie Adressen: 250 AEQ, 1 %/Monat |
| 12 | Unternehmensadresse registriert sich später als Mensch | **geschlossen** | dann gilt die Grenze für Menschen (`TestUnternehmenDasMenschWirdBehaeltGrenze`) |
| 13 | Startguthaben sofort verkaufen | **offen, nicht per Regel lösbar** | 3.000 AEQ/Monat tauschen frei, 1.000 AEQ/Monat an Menschen gebührenfrei; ein Verkauf gegen Euro per Bank läuft ganz an der Kette vorbei. Eine Abgabe verschiebt das nur. Siehe 10.1 |
| 14 | Gefälschte Identität eröffnet Firmen | **hängt an der Personenprüfung** | Der Menschenstatus lässt sich nicht entziehen; fliegt eine Fälschung auf, bleiben ihre Firmen |

Kein Geldsystem verhindert, dass sich viele echte Menschen absprechen. Hier
kostet jede bekannte Absprache mehr, als sie spart, oder sie bleibt auf kleine
Beträge je Mensch begrenzt und ist öffentlich sichtbar.

## 9. Warum diese Zahlen

- **Vielfache des fairen Anteils statt Euro.** Die Geldmenge ist immer
  Menschen × 1.000 AEQ; der Durchschnittsmensch hält immer genau einen fairen
  Anteil. „25 ×“ bleibt „25-mal so viel wie der Durchschnitt“, egal wie der
  Kurs steht. Eine Euro-Kopplung bräuchte eine Kursquelle, die jemand
  verschieben könnte, und würde die Grenzen bei steigendem Kurs still
  verschärfen.
- **Gegenrechnung in Euro (26.09.2026).** Private Haushalte in Deutschland
  halten rund 40.000 € Bargeld und Sichteinlagen je Kopf (Bundesbank 2025),
  etwa 25 Monate Konsum. Trägt AEQ einmal den Alltag, liegt 1 × in dieser
  Größenordnung. Normale Sparziele (Notgroschen, Gebrauchtwagen) bleiben unter
  dem Sparfreibetrag von 5 ×.
- **Vergleiche.** Chiemgauer: 5 % Rücktausch, rund 6 %/Jahr Umlauf. Wörgl
  1932: 1 %/Monat. Kartenzahlung in der EU: 0,2–1,5 % für Händler. Sardex
  begrenzt Guthaben auf rund 10 % des Jahresumsatzes. Unsere 0,5 %/Monat
  liegen im erprobten Bereich, 1 %/Monat gilt nur weit über jedem
  Geschäftsbedarf.
- **1 % statt 2 % für die obere Stufe (26.09.).** 2 %/Monat waren teurer als
  der einmalige Ausstieg von 2 %: Rücklagen wären sofort in Euro gewandert.

## 10. Warum ein Laden mitmachen soll

Die Regeln oben regeln die Kosten. Sie erzeugen keine Nachfrage. AEQ
läuft nur um, wenn Läden es annehmen **und weitergeben** können. Bristol
Pound ist 2021 daran gescheitert, dass Läden es nicht weitergeben konnten; der
Chiemgauer überlebt seit 2003, weil sie es können.

### 10.1 Die Lage in der Beta

| Was der Laden braucht | Stand 02.10.2026 |
|---|---|
| sich anmelden | **nur über die API** (zwei Unterschriften, 5-Minuten-Fenster); weder App noch Website bieten es an |
| kassieren | die App kann Zahlungscodes (EIP-681) **lesen**, aber keine erzeugen |
| sehen, dass bezahlt ist | nur über den Kontostand |
| Buchhaltung | kein Export |
| gefunden werden | Register unter `/api/unternehmen`, für Kunden nirgends sichtbar |
| Euro-Wert | keiner; tUSD ist Testgeld |
| Kosten | im Pilot **null** |

Dass es in der Beta keinen Umtausch in echtes Geld gibt, ist auch ein Schutz:
Ein Startguthaben, das man nicht verkaufen kann, wird ausgegeben oder liegt.
Gegen den sofortigen Verkauf (8.1, Nr. 13) hilft keine Abgabe, nur Gründe zu
bleiben. **Darum wird der echte Ausgang (Euro-Stablecoin) erst geöffnet, wenn
der Pilot zeigt, dass der Kreislauf trägt** (Messgrößen in 10.3), und erst nach
der rechtlichen Prüfung.

### 10.2 Was wir bauen und tun

1. **Klartext für Läden** (Website): Annehmen kostet nichts. Löhne kosten
   nichts. Bis 2.000 AEQ zahlt ihr nie etwas. Ihr bestimmt, wie viel ihr
   annehmt. Und ehrlich: In der Beta gibt es keinen Umtausch in Euro.
2. **Teilannahme**, am Anfang der wichtigste Hebel: Der Laden legt selbst fest,
   wie viel er annimmt („bis 20 % des Einkaufs“, „bis 500 AEQ im Monat“), und
   setzt seinen AEQ-Preis selbst. WIR arbeitet seit 1934 so. Ein offizieller
   Richtkurs wäre ein Versprechen, das wir nicht halten können.
3. **Anmeldung in der App** mit dem heutigen Verfahren: Die App erzeugt den
   Unternehmensschlüssel auf dem Gerät (nie auf einem Server), der Mensch
   unterschreibt mit seiner Wallet. Vorher klären: Wiederherstellung bei
   verlorenem Gerät.
4. **Kassenmodus:** Betrag → Zahlungscode → „bezahlt“ mit Betrag und
   Absenderart; Tagesliste und CSV-Export (Datum, Betrag, Gegenkonto,
   Transaktion). Euro-Wert erst, wenn es einen echten Kurs gibt.
5. **Verzeichnis „Wo kann ich AEQ ausgeben?“** in App und Website, Ort
   freiwillig. Namen als „selbst angegeben“ zeigen; ein freiwilliger Nachweis
   über die eigene Website (`/.well-known/aequitas.txt` mit der Adresse), den
   jede App selbst prüfen kann.
5a. **Echte Kundschaft** (gebaut 02.10.2026): `/api/unternehmen` zeigt je
   Unternehmen `kundschaft_90_tage`, die Zahl verschiedener verifizierter
   Menschen, die dort in 90 Tagen bezahlt haben (ohne Verantwortliche, je
   Zahlung mindestens 1 AEQ). Grundlage für das Verzeichnis; bringt kein Geld.
   `/api/wirtschaft/regeln` zeigt `grundeinkommen_30_tage`.
6. **Weitergabequote** im Verzeichnis: wie viel vom eingenommenen AEQ ein
   Laden im Netz weitergibt. Ruf statt Rabatt. Dafür müssen Zahlungen an
   Unternehmen und Ausstiege in die öffentlichen Monatssummen.
7. **Pilot als Kette**: Bäckerei, Mühle, Hof, Café gleichzeitig, mindestens ein
   Betrieb zahlt einen Teil der Löhne in AEQ (freiwillig). Den ersten Betrieb
   nicht fragen „Nehmt ihr AEQ?“, sondern „Würde euer Lieferant AEQ nehmen?“.

### 10.3 Was der Pilot misst

1. Anteil der Startguthaben, der in 30 Tagen bei Unternehmen ankommt, und
   Anteil, der aussteigt.
2. Weitergabequote je Unternehmen und im Mittel.
3. Wie oft ein AEQ im Monat den Besitzer wechselt.
4. Unternehmen mit Eingängen von mindestens 10 verschiedenen Menschen im Monat.
5. Ob 3.000 AEQ Tausch-Freibetrag richtig sind oder 2.000.

Alles ist aus den Blöcken ablesbar.

### 10.4 Was wir bewusst nicht machen

| Idee | Warum nicht |
|---|---|
| Startguthaben oder Grundeinkommen für Unternehmen | neues Geld ohne Menschen; jeder gründete Scheinfirmen |
| Cashback aus dem Grundeinkommens-Topf | nimmt allen, um einigen Läden Kunden zu bringen |
| Gebührenvorteil für „Partner-Läden“ | Vorrecht, das jemand vergeben müsste |
| fester Euro-Kurs | braucht Reserven, die es nicht gibt |
| Abgabe auf frühen Umtausch des Startguthabens | wird über Verkauf von Mensch zu Mensch umgangen (8.1, Nr. 13) |
| Unternehmen ohne eigenen Schlüssel | verhindert den Verkauf nicht, bündelt die Firmen mit der Identität und macht diese wertvoller zum Kaufen; großer Umbau für wenig Schutz |

## 11. Was die Regeln heute noch nicht leisten

Ehrlich, gemessen am externen Audit vom 02.10.2026
(`docs/AUDIT_VON_NULL_2026-10-02_BEWERTUNG.md`):

- **Nicht beobachtet.** Auf der Kette gibt es einen Menschen und kein
  Unternehmen. Die Regeln sind gebaut und getestet, aber nicht im Betrieb mit
  echten Teilnehmern gesehen.
- **Ein Erzeuger, Prüfung nur beobachtend.** Liegegeld steht im Block, und
  nachspielende Knoten lehnen Abweichungen noch nicht ab (Abschnitt 6). Mit
  einem Erzeuger fällt das nicht auf; vor einem zweiten unabhängigen
  Validator muss `streng` gelten.
- **Kein echter Ausgang.** Die Ausstiegsregeln wirken nur auf tUSD.
- **Recht.** Ob AEQ und der eingebaute Tausch unter MiCA fallen, muss vor
  echtem Geld geprüft sein. Für Unternehmen sind AEQ-Einnahmen
  Betriebseinnahmen zum Euro-Wert am Zahlungstag; ohne Export und Kurs ist das
  für einen Laden heute nicht sauber buchbar.

## 12. Offene Entscheidungen

| Frage | Vorschlag |
|---|---|
| Austreten aus einem Unternehmen | Ein Verantwortlicher kann sich austragen, solange einer bleibt. Der letzte nicht; dann nur Schließen |
| Anzeige der Verantwortung | „bei Eröffnung eingetragen von einem verifizierten Menschen“, nicht „ein Mensch steht dafür ein“ |
| Wann echter Ausgang | an Messwerte aus 10.3 gebunden, Schwellen nach den ersten Pilotwochen |
| Tausch-Freibetrag 3.000 oder 2.000 | nach dem Pilot |
| Monatsfreibetrag mitwachsend | nach dem Pilot: Median der echten Monatsausgaben der Menschen, nie unter 1 ×, höchstens 5 ×, je Monat höchstens ±25 % |
| Menschenstatus entziehen, wenn eine Fälschung auffliegt | Regel nötig; gehört zur Personenprüfung, nicht hierher |
| Umzug einer Identität per Iris auf eine neue Wallet | für den Iris-Launch durchdenken: macht verkaufte Identitäten wertlos; offen ist, wie man jemanden schützt, der zum Scan gezwungen wird |

## 13. Reihenfolge

| # | Was | Wann |
|---|---|---|
| 1 | Klartext und Teilannahme auf der Website (10.2, 1–2) | **erledigt 02.10.2026** |
| 1a | Kanzlei beantwortet `RECHTSFRAGEN_UNTERNEHMEN.md` | vor dem ersten Pilotladen |
| 2 | Gründer-Reihenfolge (4.1 ²) nach eigener Sicherheitsprüfung mergen | zur Beta |
| 3 | Anmeldung in der App (10.2, 3) | wenn der erste Pilotladen sie braucht (0a) |
| 4 | Kassenmodus und Verzeichnis (10.2, 4–5) | Pilotstart |
| 5 | Liegegeld-Prüfung auf `streng` | vor dem zweiten unabhängigen Validator |
| 6 | Weitergabequote, Austreten (10.2, 6; 12) | im Pilot |
| 7 | echter Ausgang an Messwerte gebunden (10.1) | vor jeder Anbindung von echtem Stable |

## 14. Verlauf

| Datum | Änderung | Warum |
|---|---|---|
| 20.08.2026 | Jede Adresse unterliegt Umlauf und Grenze (`WHO_MAY_HOLD_AEQ.md`) | Nicht-Menschen dürfen kein Schlupfloch sein |
| bis 25.09. | Entwurf: Alter des Geldes, 1 % nach 30, 3 % nach 90 Tagen; Gebührenaufschlag ab 5/10/20 ×; Lohn gesondert tauschbar | gründlich gegen Umgehung |
| 25.09. | **Freibetrag nach Umsatz statt Alter des Geldes**; Aufschlagstufen weg; 3.000 AEQ Tausch frei, egal woher | Alter war für echte Firmen zu teuer (12–36 %/Jahr auf Reserven) und für Buchhaltung nicht abbildbar |
| 26.09. | Gründungsphase, Jahresdurchschnitt für Saisonbetriebe, eigene Einlage frei | ehrliches Wirtschaften nicht bestrafen |
| 26.09. | freie Adresse 1.000 → **250 AEQ**; obere Stufe 2 % → **1 %**; Kleinstbeträge unter 0,001 AEQ nicht einziehen | Euro-Gegenrechnung (Abschnitt 9) |
| 26.09., 15:00 UTC | Regeln aktiv (vorgezogen vom 01.10.) | – |
| 02.10. | Fassung 3: ein gültiger Text statt Schichten; Gründer bleibt an erster Stelle; Angriffe 6, 7, 10, 13 offen benannt; Anreize (Abschnitt 10) | Prüfung gegen den Code und externes Audit |
| 02.10. | Fassung 4: zweite Stufe ab 15.10.2026 (am 03.10. vorgezogen auf 03.10.2026, 12:00 UTC) (C ablehnen statt wegnehmen, A erstes Unternehmen wie ein Mensch, B Firmen-Eingänge gedeckelt brutto); echte Kundschaft und Grundeinkommen als Zahl | Reifeprüfung: Kleine und Lieferketten wurden bestraft, Empfänger verloren Geld ohne eigenes Zutun |

## Vorbilder

- **Chiemgauer** (Bayern, seit 2003): Regionalgeld mit Umlaufsicherung, 5 %
  Rücktausch zugunsten von Vereinen, einige hundert Geschäfte.
- **WIR** (Schweiz, seit 1934): Verrechnungsgeld zwischen Unternehmen,
  Teilzahlung in WIR, stabilisierend in Krisen.
- **Wörgl** (1932): 1 %/Monat Schwundgeld, bis die Nationalbank es verbot.
- **Bristol Pound** (2012–2021): gescheitert, weil Läden es kaum weitergeben
  konnten.
