# Play Store: was vorbereitet ist und was fehlt

Stand: 03.11.2026. Die Google-Regeln ändern sich oft; vor dem Einreichen die
verlinkten Seiten noch einmal lesen.

## Kurz

Technisch ist die App fertig für den Play Store. Es fehlen das
Entwicklerkonto und drei Angaben, die nur der Betreiber machen kann.

| Punkt | Stand |
|---|---|
| Zielversion Android 16 (API 36), Pflicht seit 31.08.2026 | erfüllt: React Native 0.81 setzt `targetSdk = 36` |
| `.aab`-Build | Profil `production` in `eas.json` |
| Signatur | **Achtung**, siehe unten: Play App Signing mit demselben Schlüssel |
| Konto löschen in der App | vorhanden (Identität → Daten löschen, `DELETE /enrollment`) |
| Konto löschen über das Web (Pflicht) | **fehlt**: eine Seite mit Anleitung; siehe unten |
| Datenschutzerklärung als URL | **fehlt**: hängt am Impressum (`rechtstexte-setzen.yml`) |
| Entwicklerkonto | **fehlt**: als Organisation, siehe unten |

## 1. Entwicklerkonto: als Organisation, nicht privat

- Ein **privates** Konto (nach dem 13.11.2023 angelegt) muss jede neue App
  erst 14 Tage lang mit mindestens 12 Testern im geschlossenen Test laufen
  lassen, bevor sie öffentlich werden darf.
- Ein **Organisationskonto** ist davon ausgenommen. Es braucht eine
  D-U-N-S-Nummer, die kostenlos bei Dun & Bradstreet zu beantragen ist und
  einige Tage bis Wochen dauert. Die Nummer setzt die gegründete
  Organisation voraus (UG, e. V. oder Stiftung).
- Damit hängt der Play Store an derselben Entscheidung wie das Impressum.
- Gebühr: einmalig 25 US-Dollar.

## 2. Krypto-Regeln von Google

Die Regel „Cryptocurrency Exchanges and Software Wallets“ verlangt in der EU
seit dem 30.06.2026 eine MiCA-Zulassung als Krypto-Dienstleister. Sie gilt
nur für **verwahrende** Wallets und Börsen. **Nicht verwahrende Wallets
sind ausdrücklich ausgenommen.**

Aequitas ist nicht verwahrend: Der private Schlüssel entsteht auf dem Gerät
und liegt nur dort in SecureStore (`lib/wallet.ts`, nur auf diesem Gerät, nur
entsperrt). Kein Server sieht ihn.

In der Play Console ist das bei der Erklärung zu Finanzfunktionen
anzugeben: „Software-Wallet, nicht verwahrend“. Ob die Prägung von AEQ
selbst unter MiCA fällt, ist davon unabhängig und eine Frage für den Anwalt
(Launch-Blocker „Krypto-Recht“).

## 3. Datensicherheit (Formular „Data safety“)

Vorschlag nach dem Code und dem Verarbeitungsverzeichnis
(`aequitas-biometric-beta/docs/dsgvo/01_VERZEICHNIS.md`). Der Anwalt sollte
ihn vor dem Absenden gegenlesen.

| Datentyp bei Google | Erhoben? | Zweck | Anmerkung |
|---|---|---|---|
| Fotos und Videos | ja, nicht geteilt | Betrugsschutz und Sicherheit (Einmaligkeit) | Bildserie und Vorlage nur im Arbeitsspeicher; gespeichert wird ein Sketch, der kein Bild ist |
| Audio (sonstige Audiodateien) | ja, nicht geteilt | Betrugsschutz (akustische Lebendigkeit) | Echo eines Ultraschall-Tons, keine Sprache |
| Geräte- oder andere Kennungen | ja, nicht geteilt | Betrugsschutz | Geräte-Geheimnis; Play-Integrity-Nachweis |
| Finanzinformationen | ja | App-Funktion | Wallet-Adresse und Kontostand liegen öffentlich auf der Kette |
| Standort, Kontakte, Nachrichten, Werbe-ID | nein | — | — |
| Absturz- und Nutzungsanalyse | nein | — | Kein Analyse-SDK in der App |

Weitere Angaben im Formular:
- Übertragung verschlüsselt: ja (nur HTTPS, `sichereAdresse`).
- Löschen auf Anfrage: ja.
- Erhebung ist Pflicht, nicht freiwillig: Ohne Aufnahme gibt es keine
  Registrierung.

Biometrische Daten fallen bei Google unter „personenbezogene und sensible
Nutzerdaten“. Verlangt sind eine deutliche Erklärung **vor** der Aufnahme
und eine ausdrückliche Zustimmung. Beides gibt es schon:
Einwilligungsschritt v4, `docs/dsgvo/09_EINWILLIGUNG.md`.

## 4. Konto löschen über das Web (Pflicht)

Google verlangt neben dem Weg in der App einen Link, über den man das
Löschen auch **ohne** die App verlangen kann. Am einfachsten ist eine Seite
auf `aequitas.digital/loeschen` mit:
- dem Weg in der App;
- dem Weg ohne App: die `bio_hash` an `DELETE /enrollment` schicken, oder
  per E-Mail an die Adresse aus dem Impressum;
- dem Hinweis, dass Kettendaten öffentlich und unveränderlich sind (V6) und
  welche Marke den Widerruf überlebt (V4).

Bauen kann ich die Seite sofort. Die E-Mail-Adresse kommt aus dem Impressum,
also erst nach der Gründung.

## 5. Signatur: der wichtigste Stolperstein

Die APK wird heute mit dem eigenen Keystore signiert (`docs/RELEASE_SIGNING.md`
im App-Repo). Bei Play App Signing **denselben** Schlüssel hochladen
(Variante „bestehenden Schlüssel exportieren und hochladen“), nicht Google
einen neuen erzeugen lassen. Sonst sind die APK von der Website und die App
aus dem Store zwei verschiedene Apps: Ein Wechsel ginge nur über
Deinstallieren, und dabei geht die Wallet verloren, wenn sie nicht gesichert
ist.

## 6. Einträge für den Store

- Name: Aequitas.
- Kurzbeschreibung (80 Zeichen): „Ein Mensch, ein Konto: Grundeinkommen auf
  einer eigenen Blockchain.“
- Kategorie: Finanzen.
- Altersfreigabe: Fragebogen ehrlich ausfüllen. Die Registrierung gilt erst ab dem
  Mindestalter des Landes, meist 18 (`coordinator/app/altersregel.py`). Die App enthält kein Glücksspiel, aber eine
  Krypto-Wallet.
- Screenshots: von einem echten Gerät, ohne Testwallet mit echten Beträgen.

Nicht versprechen, was nicht gedeckt ist: kein Betrag und kein Datum für das
Grundeinkommen, solange die Finanzierung offen ist (Launch-Blocker
„Finanzierung“).

## Quellen

- Ausnahme für nicht verwahrende Wallets:
  https://support.google.com/googleplay/android-developer/answer/16329703
- Geschlossener Test für private Konten:
  https://support.google.com/googleplay/android-developer/answer/14151465
- Zielversion: https://developer.android.com/google/play/requirements/target-sdk
