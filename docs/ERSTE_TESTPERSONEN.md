# Erste Testpersonen: Einladung und Anleitung

Stand 20.10.2026, App 1.12.1 (Aequitas-App #37; bis zu deren Release gilt 1.12.0). Für die ersten 20–50 Menschen aus deinem Umfeld.
Ziel: echte Registrierungen, echte Überweisungen, echte Rückmeldungen, damit
sich zeigt, ob App, Gesichtsprüfung und Grundeinkommen im Alltag tragen.

**Vorher (Betreiber):** Impressum und Datenschutzerklärung sind noch nicht
veröffentlicht (`BETA_BEREITSCHAFT.md`, Abschnitt 3). Lade deshalb vorerst nur
Menschen ein, die du persönlich kennst, und sag ihnen das offen. Die
Einwilligung in der App erklärt die Gesichtsprüfung vollständig; sie ersetzt
die Datenschutzerklärung nicht.

---

## Text zum Weiterschicken

> Hallo, ich baue mit Aequitas ein Grundeinkommen auf einer eigenen
> Blockchain: Jeder Mensch bekommt einmal 1.000 AEQ und danach jeden Tag
> denselben Anteil aus einem gemeinsamen Topf. Damit niemand doppelt
> kassiert, prüft eine kurze Gesichtsaufnahme, dass du ein echter Mensch bist
> und dich zum ersten Mal anmeldest.
>
> Es ist eine frühe Testphase. Ich würde mich freuen, wenn du mitmachst und
> mir sagst, was hakt. Dauer: etwa 10 Minuten. Du brauchst ein
> Android-Handy mit Kamera.
>
> Anleitung: siehe unten. Fragen jederzeit an mich.

---

## Anleitung für Testpersonen

### Was du wissen solltest

- **Frühe Testphase.** Es läuft derzeit ein Knoten (Server), betrieben von
  mir. Es kann Ausfälle geben.
- **AEQ sind kein Euro.** Aequitas bietet keinen Umtausch in Euro an. Du
  zahlst nichts, und du kannst nichts verlieren außer deiner Zeit.
- **Gesichtsprüfung:** Die App nimmt eine kurze Sequenz mit Blinzeln und
  Kopfbewegung auf. Die Bilder werden nach Sekunden gelöscht; es bleibt ein
  kleines Merkmal, mit dem eine zweite Anmeldung derselben Person erkannt
  wird. In der Testphase prüft **ein** Prüfdienst (später mehrere
  unabhängige). Du kannst dein Profil jederzeit in der App löschen.
- **Höchstens 25 neue Menschen pro Tag.** Meldet die App, dass die Grenze
  für heute erreicht ist, einfach am nächsten Tag noch einmal (die Fassung
  1.12.0 zeigt dafür noch „konnte nicht abgeschlossen werden“).

### 1. App installieren (Android)

1. Auf dem Handy öffnen:
   `https://github.com/hanoi96international-gif/Aequitas-App/releases/latest/download/app-release.apk`
   (oder `https://aequitas.digital/download/app.apk`).
2. Die Datei öffnen. Android fragt, ob der Browser Apps installieren darf:
   für diese eine Installation erlauben.
3. Hattest du eine sehr alte Fassung (1.5 oder älter): erst deinstallieren.

### 2. Wallet anlegen und Wörterliste sichern

1. App öffnen → Tab **Wallet** → neue Wallet anlegen.
2. Die **Wörterliste (Seed-Phrase)** auf Papier schreiben und sicher
   aufbewahren. **Niemandem zeigen, auch mir nicht.** Ohne sie ist die Wallet
   bei Verlust des Handys weg; mit ihr kann jeder über dein Guthaben
   verfügen.

### 3. Als Mensch registrieren

1. Tab **Identity** → **MENSCHLICHKEIT NACHWEISEN**.
2. Einwilligung lesen und zustimmen.
3. Gesichtsaufnahme: gutes Licht, Gesicht ganz im Bild, den Anweisungen
   folgen (blinzeln, Kopf leicht drehen).
4. Warten, bis der Nachweis erstellt ist (bis zu einer Minute).
5. Fertig: **„Registriert! 1.000 AEQ gutgeschrieben“**.

### 4. Ausprobieren

- Einer anderen Testperson ein paar AEQ schicken (Tab **Wallet** →
  Senden, Adresse per QR-Code unter **Empfangen**).
- Nach 20:00 Uhr nachsehen, ob das Grundeinkommen angekommen ist. In den
  ersten Tagen kann es klein oder null sein: Der Topf füllt sich aus den
  Gebühren der Überweisungen.

### 5. Rückmeldung (das Wichtigste)

Bitte schick mir kurz:

- Hat die Registrierung geklappt? Wenn nicht: **Bildschirmfoto der
  Meldung** und ungefähre Uhrzeit.
- Wie lange hat es gedauert, was war unklar?
- Handy-Modell und Android-Version.

**Nie mitschicken:** die Wörterliste oder Bildschirmfotos davon.

---

## Für den Betreiber: was du danach ansiehst

- Wache und `/api/health/combined`: Registrierungen des Tages
  (Coordinator `einzelbetrieb.heute`), Fehler, Divergenz.
- Ab den ersten Überweisungen hat der Grundeinkommens-Topf Gebühren. Dann
  liefert `nachrechnen-beobachter.yml` zum ersten Mal Runden **mit**
  Aussage – Voraussetzung für das strenge Nachrechnen
  (`BETA_BEREITSCHAFT.md`, Abschnitt 3, Punkt 7).
- Rückmeldungen sammeln; was mehrfach kommt, wird zuerst behoben.
