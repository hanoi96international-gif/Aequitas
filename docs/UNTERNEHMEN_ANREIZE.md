# Unternehmen: Warum sollte ein Laden AEQ annehmen?

Stand: 02.10.2026 · Status: **Analyse und Vorschlag, nichts davon ist beschlossen
oder gebaut.** Ergänzt `docs/UNTERNEHMEN_KONZEPT.md`; dort stehen die geltenden
Regeln.

## In drei Sätzen

Die Regeln für Unternehmen sind gründlich durchdacht, **aber nur von der
Kostenseite**: Was kostet Horten, wie wird Missbrauch verhindert, wer zahlt wie
viel. Die Nutzenseite fehlt fast ganz. Heute kann sich ein Unternehmen ohne
Entwickler nicht einmal anmelden, es gibt keine Kasse, kein Verzeichnis und in
der Beta keinen echten Ausstieg. Dazu kommt eine Regel, die nach dem Start mit
echtem Geld genau die Todesspirale auslösen kann, vor der wir Angst haben
(Abschnitt 2.2).

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
  (`menschTauschFreiMonat`).
- Folge: **Das gesamte Startguthaben kann am Tag der Registrierung abgabefrei
  verkauft werden.**

Solange es nur tUSD gibt, ist das egal. Sobald ein echter Euro-Stablecoin
angebunden ist, ist es der kürzeste Weg in die Spirale aus 2.1: Neue Menschen
verkaufen, weil sie (noch) nichts kaufen können, der Kurs fällt, Läden steigen
aus. Dazu ist es ein **Sicherheitsthema**: Wer die Personenprüfung einmal
überlistet, kann jede gefälschte Identität sofort in Euro verwandeln. Je
leichter der Ausstieg, desto lohnender der Angriff auf die Biometrie.

Das ist der eine Punkt in diesem Dokument, der **vor echtem Geld entschieden
sein muss**. Vorschlag in 4.1.

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

### 4.1 Startguthaben zum Ausgeben, nicht zum Verkaufen (Konsens, vor echtem Geld)

**Regel:** Wer in den **ersten 90 Tagen nach der Registrierung** AEQ in Stable
tauscht, hat keinen Tausch-Freibetrag. Es gilt die normale Ausstiegsabgabe von
2 %. Danach gelten wie heute 3.000 AEQ im Monat frei. Die eigene Einlage
(selbst eingezahltes Stable) bleibt immer frei.

- **Fair:** gilt für jeden Menschen gleich, nimmt niemandem etwas, das
  Startguthaben bleibt voll nutzbar zum Ausgeben, Überweisen, Sparen.
- **Wirkt gegen die Spirale:** Verkaufen bleibt möglich, aber Ausgeben im
  Netz ist die bessere Wahl. Die 20 AEQ Abgabe auf einen vollen Verkauf gehen
  ans Grundeinkommen.
- **Wirkt gegen Fälschungen:** Eine gefälschte Identität ist nicht mehr
  sofort und kostenlos Bargeld.
- **Einfach:** ein Datum je Mensch, das die Kette schon kennt (Registrierung).

Alternativen, falls 2 % zu mild sind: 5 % in den ersten 90 Tagen (Chiemgauer-Höhe),
oder Freibetrag erst ab 180 Tagen. Empfehlung: mit 2 % / 90 Tage starten und
im Pilot messen, welcher Anteil der Startguthaben in den ersten 30 Tagen bei
Unternehmen landet und welcher im Ausstieg.

> Ändert eine Konsensregel (`ausstiegsAbgabe` in `wirtschaft.go`), braucht eine
> Aktivierungshöhe, Tests für Nachspielen und Missbrauch (Registrierung,
> Weiterleiten an zweites Konto, Tausch) und eine eigene Sicherheitsprüfung des
> Diffs. Nicht in der Woche vor dem Beta-Start bauen. Die Beta hat ohnehin nur
> tUSD; die Regel muss stehen, **bevor** ein echter Stablecoin angebunden wird.

Offen bei der Entscheidung: Ein Mensch kann das Startguthaben an einen Freund
überweisen, der älter als 90 Tage ist und abgabefrei tauscht. Das spart 2 % und
verbraucht dessen Monatsfreibetrag, und der Freund muss mitmachen. Klein und
gedeckelt (3.000 AEQ je Mensch und Monat), aber nicht null.

### 4.2 Unternehmen in der App anmelden (App, Beta-Pilot)

Ohne das gibt es keinen Pilot, nur Einzelfälle mit Entwicklerhilfe.

- Neuer Bereich „Mein Unternehmen“ in der App: Name, Kategorie, fertig.
- Die App erzeugt das Schlüsselpaar der Unternehmensadresse auf dem Gerät
  (SecureStore), unterschreibt damit die Eröffnungsnachricht, der Mensch
  unterschreibt dieselbe Nachricht mit seiner Wallet. Kein Schlüssel verlässt
  das Gerät.
- Danach dasselbe für Mitinhaber und Schließen.

Sicherheitsfragen vor dem Bau: Verlust des Geräts (Wiederherstellung des
Unternehmensschlüssels; Vorschlag: Export als verschlüsselte Datei, und
Mitinhaber als zweite Instanz), keine Server-Kopie des Schlüssels.

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

### 4.6 Teilannahme als Empfehlung für Läden (kein Code)

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

## 5. Was wir bewusst nicht machen

| Idee | Warum nicht |
|---|---|
| Startguthaben oder Grundeinkommen für Unternehmen | neues Geld ohne Menschen dahinter; jeder würde Scheinfirmen gründen |
| Cashback für Kunden aus dem Grundeinkommens-Topf | nimmt allen Menschen, um einigen Läden Kunden zu bringen |
| Gebührenvorteil für „Partner-Läden“ | Vorrecht, das jemand vergeben müsste; zentrale Stelle |
| Fester Euro-Kurs für AEQ | braucht Reserven, die es nicht gibt; Versprechen, das bricht |
| Liegegeld für Unternehmen senken | ist im Pilot ohnehin null (Sockel). Die Grenze ist nicht das Problem |

## 6. Was der Pilot messen muss

Dieselben Zahlen, die über 4.1 entscheiden:

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
| 2 | Anmeldung in der App (4.2) | App | **zur Beta**, sonst kein Pilot |
| 3 | Kassenmodus mit CSV (4.3) | App | Pilotstart |
| 4 | Verzeichnis „Wo kann ich AEQ ausgeben?“ (4.4) | App, Website, Register-Feld | Pilotstart |
| 5 | Weitergabequote (4.5) | API, App | im Pilot |
| 6 | Startguthaben 90 Tage ohne Tausch-Freibetrag (4.1) | Konsens | **entschieden und gebaut, bevor echtes Stable angebunden wird** |

Die Beta selbst muss nicht warten. Menschen können ab Tag 1 mitmachen;
Unternehmen kommen im begleiteten Pilot dazu, sobald 2 und 3 stehen.
