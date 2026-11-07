# Deepfake-Test — Anleitung für das Handy (WP 4)

Zweck: messen, ob Angriffe mit Bildschirm, Video oder Face-Swap **gelb oder
rot** landen und echte Aufnahmen **grün**. Erst dann dürfen die Schwellen
scharf werden (`LEBENDIGKEIT_GEGEN_DEEPFAKES.md`, WP 4). Ausgewertet wird mit
`bio-lebendigkeit-auswerten.yml`. Der Workflow liest nur die öffentliche
Antwort `https://proof1.aequitas.digital/coordinator/health` und braucht kein
Geheimnis und keine Freigabe.

## Warum das gefahrlos ist

Teste **nur mit deinem eigenen, schon registrierten Gesicht**. Nimm dafür in
der App eine **neue Wallet**: Nur so startet die App die Registrierung
überhaupt.

- **Was passiert:** Der Vergleichsdienst erkennt dein Gesicht. Der Versuch
  endet als `duplicate_detected`, es wird kein AEQ geprägt.
- **Was trotzdem erfasst wird:** Der Coordinator bewertet die Lebendigkeit
  vorher und legt sie ab, auch beim Duplikat. Gespeichert werden nur Klasse,
  L, R und die Signale, ohne Kennung (`coordinator/app/main.py`, `merke`).
- **Wenn ein Versuch doch registriert wird:** Das ist selbst ein Befund. Die
  Gesichtserkennung hat dann dich nicht erkannt. Den Vorgang notieren und mir
  schicken.

Face-Swap also nur so, dass am Ende **dein** Gesicht zu sehen ist. Das
Gesicht eines anderen Menschen wäre eine neue Identität und eine echte
Registrierung.

## Vorher

1. Der Coordinator muss aktuell sein. Der Messstand `letzte` lebt im Speicher,
   ein Neustart oder Deploy leert ihn. Also nicht während eines Deploys
   testen.
2. Ein zweites Gerät (Laptop, Tablet, zweites Handy) als Angriffsbildschirm.
3. Grenzen: höchstens 30 Versuche je 10 Minuten von einer Adresse.
   Wiederholte Duplikate werden absichtlich langsamer beantwortet.

## Ablauf (eine Sitzung, ohne Pause dazwischen)

**A. Echte Aufnahmen: 5 Stück**, live vor dem Handy, Anweisungen der App
befolgen. Davon 3 bei gutem Licht, 1 bei schwachem Licht und 1 mit Brille,
falls du eine trägst.

**B. Angriffe**, jeweils mit einer neuen Wallet oder derselben Test-Wallet:

| Nr | Angriff | So geht's |
|---|---|---|
| 1 | Foto auf Bildschirm | Ein Foto deines Gesichts auf dem zweiten Gerät vor die Handykamera halten |
| 2 | Video auf Bildschirm | Ein Video, in dem du blinzelst und den Kopf links/rechts drehst, vor die Kamera halten |
| 3 | Video mit passender Drehung | Wie 2, aber die Drehung ansehen und im Video zur passenden Stelle springen |
| 4 | Face-Swap (falls verfügbar) | Eine Face-Swap-App, die **dein** Gesicht auf ein Video legt, vor die Kamera halten |

Zähle die Angriffe, die tatsächlich gesendet wurden (die App zeigt ein
Ergebnis). Brich ein Versuch vorher ab, zählt er nicht.

## Auswerten

Direkt danach starten:
<https://github.com/hanoi96international-gif/Aequitas/actions/workflows/bio-lebendigkeit-auswerten.yml>
→ „Run workflow“. Unter `angriffe` die Zahl der gesendeten Angriffe
eintragen, `stunden` bleibt 24.

Die Zusammenfassung des Laufs zeigt jede Aufnahme mit Klasse, L, R und den
Signalen.

- **Lauf grün:** Kein Angriff ist grün gelandet.
- **Lauf rot:** Mindestens ein Angriff ist grün gelandet. Dieser Angriff käme
  nach dem Scharfschalten **ohne Staffel** durch. Nicht kalibrieren.
- **„trennt ja“:** Bei diesem Signal liegt jede echte Aufnahme über jedem
  Angriff. Daran sieht man, welches Signal die Arbeit macht.
- **Schwellen:** Ein Vorschlag kommt erst ab 20 echten Aufnahmen und nur ohne
  Überlappung. Darunter steht ausdrücklich „ZU WENIG“.

## Was der Test nicht zeigt

Eine **virtuelle Kamera** (Injektion auf einem gerooteten Gerät) umgeht die
Kamera ganz. Die Blitzprüfung hilft dagegen nur begrenzt. Die App kennt die
Farbfolge, und `flash_liveness.py` misst die Farbbalance des ganzen Bildes,
nicht die 3D-Spiegelung im Gesicht. Eine Injektion kann die Bilder also
passend einfärben. Dagegen hilft die Geräteprüfung. Seit 07.11.2026
ist sie an genau die hochgeladenen Bilder gebunden, und ohne sie gibt es kein
„grün“. Ein gerootetes Telefon oder ein Emulator besteht die Android Key
Attestation nicht. Dazu kommen die Staffel und die Tagesgrenze im
Einzelbetrieb (25 Registrierungen am Tag). Siehe `docs/BETA_BEREITSCHAFT.md`,
Abschnitt 0a „Deepfake“.
