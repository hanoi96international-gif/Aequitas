# Rechtsvorlagen – ENTWURF

> **Entwurf, nicht geprüft.** Diese Texte sind keine Rechtsberatung. Sie
> sollen einer Anwältin oder einem Anwalt die Arbeit erleichtern: Sie halten
> fest, was technisch tatsächlich geschieht, damit die Prüfung am echten
> System ansetzt. Vor dem Einsatz müssen sie geprüft und angepasst werden.
> Platzhalter stehen in `[[…]]`. Personen- oder Firmendaten sind hier nie
> eingetragen.

Hintergrund: `RECHT_UND_LITERATUR.md` (Abschnitt Knoten als gemeinsam
Verantwortliche), `RECHTSFRAGEN_UNTERNEHMEN.md` Frage 34, `LAUNCH_CHECKLISTE.md`
Blocker 5, `VERIFIER_PRINZIPIEN.md`.

---

## 1. Vereinbarung über gemeinsame Verantwortung (Art. 26 DSGVO)

Diese Vereinbarung wird zwischen den Betreibern der Prüfdienste geschlossen:
Coordinator, Vergleichsdienst und Proof-Server. Für jeden Betreiber gilt das
Muster in der Tabelle unten.

| Rolle | Betreiber | Kontakt Datenschutz |
|---|---|---|
| Coordinator | [[Name/Firma, Anschrift]] | [[E-Mail]] |
| Vergleichsdienst | [[Name/Firma, Anschrift]] | [[E-Mail]] |
| Proof-Server | [[Name/Firma, Anschrift]] | [[E-Mail]] |
| Validator (Kette) | [[offen: Art. 26 oder keine Verantwortung für Biometrie, siehe §7]] | [[E-Mail]] |

### §1 Gegenstand

Die Parteien prüfen gemeinsam, ob ein Mensch nur einmal registriert ist
(„Proof of Personhood“), und schreiben das Ergebnis auf die Kette. Zweck und
wesentliche Mittel legen sie gemeinsam fest.

### §2 Verarbeitete Daten (technisch belegt)

| Daten | Wo | Wie lange |
|---|---|---|
| Kamerabild und Video von Gesicht und Hand | nur im Arbeitsspeicher des Coordinators | gelöscht nach dem Abgleich (Sekunden) |
| Biometrisches Merkmal (Template, Skizze) | Vergleichsdienst, verschlüsselt (AES-GCM) | bis zur Löschung durch den Menschen (A4) |
| Abgleichsschlüssel `enroll_key` = HMAC(Schlüssel, Merkmal) | Vergleichsdienst | wie oben |
| Nullifier, Wallet-Adresse, „ist Mensch“ | öffentlich auf der Kette | dauerhaft (siehe §6) |
| Altersschätzung, Selbstangabe Alter | Coordinator, Ergebnis ohne Bild | [[Frist festlegen]] |
| Geräteprüfung (Play Integrity / App Attest) | Coordinator, nur Ergebnis | [[Frist festlegen]] |

Auf der Kette liegen **keine biometrischen Daten**.

### §3 Zuständigkeiten

| Pflicht | zuständig |
|---|---|
| Information der Betroffenen (Art. 13), Einwilligung (Art. 9 Abs. 2 lit. a) | Coordinator (App-Ablauf), Text gemeinsam |
| Auskunft, Löschung, Widerruf (Art. 15–17, 7 Abs. 3) | Anlaufstelle: [[eine Partei]]; die anderen helfen innerhalb von [[7]] Tagen |
| Löschung per Wallet-Unterschrift (technisch umgesetzt) | jeder Dienst für seine Daten |
| Meldung von Datenschutzverletzungen (Art. 33/34) | die betroffene Partei; Information der anderen innerhalb von 24 h |
| Datenschutz-Folgenabschätzung (Art. 35) | gemeinsam |
| Technische und organisatorische Maßnahmen (Art. 32) | jeder für seinen Dienst; Mindeststand siehe Anlage |

### §4 Wesentlicher Inhalt für Betroffene (Art. 26 Abs. 2 Satz 2)

Der wesentliche Inhalt dieser Vereinbarung wird in der Datenschutzerklärung
veröffentlicht. Betroffene können ihre Rechte bei jeder Partei geltend
machen (Art. 26 Abs. 3).

### §5 Auftragsverarbeiter und Drittland

[[Hoster je Dienst, Ort der Server; Übermittlung in Drittländer: keine /
Grundlage]].

### §6 Kette und Löschung

Nullifier und Wallet-Adresse sind öffentlich und lassen sich technisch nicht
entfernen. Löscht ein Mensch seine Registrierung, werden das Template und
der Abgleichsschlüssel beim Vergleichsdienst gelöscht. Damit lässt sich der
Nullifier keinem Merkmal mehr zuordnen.
[[Prüfen: Reicht das als Löschung oder Anonymisierung im Sinne von Art. 17?
EDPB-Leitlinien 02/2025 zur Blockchain.]]

### §7 Validatoren

[[Offene Rechtsfrage 34. Validatoren sehen keine Biometrie, nur öffentliche
Transaktionen. Vorschlag: keine gemeinsame Verantwortung für die
biometrische Prüfung, aber Verantwortung für die Daten auf der Kette prüfen
lassen.]]

### §8 Haftung, Laufzeit, Austritt

Im Innenverhältnis haftet jede Partei für ihren Dienst. Austritt ist mit
[[Frist]] möglich. Die austretende Partei löscht ihre Daten oder gibt sie an
einen Nachfolger, der die Vereinbarung übernimmt.

**Anlage (Mindeststand TOM):**
- Templates verschlüsselt gespeichert
- `bio_hash` nur als HMAC
- Zugriff nur per Schlüssel, nur aus der Umgebung `produktion`
- Protokolle ohne Biometrie und ohne Geheimnisse
- tägliche Sicherung, 7 Tage, verschlüsselt [[prüfen]]

---

## 2. Einwilligung bei der Registrierung (Entwurf)

Angezeigt in der App vor der Kamera. Jeder Punkt muss einzeln bestätigt
werden. Ohne Einwilligung ist keine Registrierung möglich, aber die
Wallet-Funktionen bleiben nutzbar.

> **Einmal-Mensch-Prüfung – deine Einwilligung**
>
> Damit jeder Mensch nur einmal Aequitas erhält, wird geprüft, ob du schon
> registriert bist. Dafür nimmt die App kurz dein Gesicht und deine Hand auf.
>
> **Wer verarbeitet deine Daten?** Gemeinsam verantwortlich sind
> [[Coordinator-Betreiber]], [[Vergleichsdienst-Betreiber]] und
> [[Proof-Server-Betreiber]]. Kontakt: [[Anlaufstelle]]. Du kannst dich an
> jeden von ihnen wenden.
>
> **Was wird gespeichert?**
> - Ein verschlüsseltes Merkmal deines Gesichts und deiner Hand. Es dient
>   nur dem Abgleich und ist kein Foto.
> - Auf der öffentlichen Kette steht nur eine Zufallskennung (Nullifier)
>   neben deiner Wallet-Adresse, nie ein Bild oder Merkmal.
>
> **Wie lange?** Bis du die Löschung verlangst.
>
> **Widerruf und Löschung:** In der App unter „Registrierung löschen“,
> unterschrieben mit deiner Wallet. Danach wird dein Merkmal gelöscht. Was
> du bis dahin erhalten hast, behältst du.
>
> ☐ Ich willige ausdrücklich ein, dass meine biometrischen Daten zu diesem
>   Zweck verarbeitet werden (Art. 9 Abs. 2 lit. a DSGVO).
> ☐ Ich bin mindestens 18 Jahre alt und in meinem Land volljährig.
> ☐ Ich habe die Datenschutzerklärung gelesen: [[Link]].

**Hinweise für die Prüfung:**
- Die Einwilligung schwächt keine gesetzliche Pflicht ab. Impressum, Art. 13
  und Art. 26 gelten trotzdem (siehe `RECHT_UND_LITERATUR.md`).
- Freiwilligkeit (Art. 7 Abs. 4): Der Grundbetrag hängt an der Registrierung.
  [[Prüfen: Koppelungsverbot.]]
- Die Versionsnummer der Einwilligung (`CURRENT_CONSENT_VERSION`) muss bei
  jeder Textänderung erhöht werden.
