# Einen Aequitas-Verifier betreiben

**Stand 30.09.2026.** Ersetzt den elfstufigen Verifier-Leitfaden vom August
2026 (Explorer, „Run a Verifier“).

## Worum es geht, in drei Sätzen

Ein Verifier (Vergleichsdienst) prüft bei jeder Registrierung, ob dieses
Gesicht schon eingeschrieben ist. Eine Registrierung zählt erst, wenn mehrere
**verschiedene** Verifier zustimmen. Je mehr unabhängige Verifier es gibt,
desto schwerer ist es, sich zweimal zu registrieren.

Ein Verifier ist etwas anderes als ein Validator
(`VALIDATOR_EINRICHTEN.md`): Der Validator führt die Kette und prüft die
Blöcke, der Verifier schützt „ein Mensch, ein Konto“. Beides auf einem Server
ist möglich, braucht dann aber mehr Arbeitsspeicher.

## Was du brauchst

| | Mindestens |
|---|---|
| Server | ein gemieteter Server (VPS) mit Ubuntu 22.04 oder 24.04 und einer öffentlichen IPv4-Adresse |
| Arbeitsspeicher | 4 GB |
| Festplatte | 40 GB SSD (das Programm allein ist etwa 5 GB groß) |
| Offene Ports | 80 und 443 (für HTTPS) |
| Du selbst | in der Aequitas-App **registriert** |

**Eine eigene Domain brauchst du nicht.** Das Skript nutzt die Adresse
`https://<deine-IP-mit-Strichen>.sslip.io`. Sie zeigt ohne Einrichtung auf
deinen Server, und das HTTPS-Zertifikat holt sich der Server selbst.

**Was nicht auf den Server gehört:** deine Wallet, ihre Wörterliste und ihr
privater Schlüssel. Auf dem Server liegt nur die **Adresse** deiner Wallet.

## Einrichten: ein Befehl, eine Frage

Mit dem Server verbinden (Windows: „PowerShell“, Mac: „Terminal“):

```bash
ssh root@DEINE-SERVER-IP
```

Dann diese drei Zeilen einfügen:

```bash
curl -fsSL https://get.docker.com | sh
git clone https://github.com/hanoi96international-gif/Aequitas.git
cd Aequitas/deploy/verifier && bash einrichten.sh
```

Das Skript fragt nur nach **deiner Wallet-Adresse** (in der App unter
„Empfangen“, beginnt mit `0x`) und lässt dich die IP-Adresse bestätigen.
Alles andere erledigt es selbst:

- Es erzeugt **alle Schlüssel auf deinem Server**: den Schlüssel, mit dem der
  Verifier Registrierungen bezeugt, einen eigenen Schlüssel für die
  gespeicherten Daten und eine eigene Projektion für die Gesichtsskizzen.
  Nimm nie Schlüssel von jemand anderem, auch nicht von uns: Zwei Verifier mit
  demselben Geheimnis zählen als einer.
- Es holt die Liste der anerkannten Coordinatoren vom Netz.
- Es lädt das Programm (beim ersten Mal etwa 5 GB), startet es und richtet
  HTTPS ein.

Am Ende zeigt es drei Zeilen:

```
Verifier: https://203-0-113-9.sslip.io
Wallet:   0x…
Schluessel/key: …
```

## Danach: Aufnahme (automatisch, ohne Telegram)

Ab hier brauchst du nur noch die App. Niemand muss etwas von Hand eintragen.

1. **Schlüssel freigeben.** App → Tab **Knoten** → Karte **„Prüfer“** →
   **„Schlüssel freigeben“**. Als Adresse die erste Zeile von oben eintragen
   (`https://…sslip.io`), **„Nachweis holen“** antippen, dann mit deiner Wallet
   freigeben. Dein Verifier stellt den Nachweis nur für deine eigene Wallet aus
   (`VALIDATOR_BETREIBER_WALLET`, setzt das Skript gleich `OPERATOR_WALLET`) --
   sonst könnte jemand anderes deinen Schlüssel unter seinem Namen eintragen.
2. **Als Prüfer bewerben.** App → Karte „Prüfer“ → **„Prüfer-Register öffnen“**
   → **„Bewerben“**. Alle Prüfer sehen deine Bewerbung.
3. **Abstimmung.** Sobald zwei Drittel der Prüfer zugestimmt haben, trägt die
   Kette dich selbst ein. Wirksam wird das nach einer Frist von etwa zwei
   Stunden.
4. **Ab dann läuft alles von selbst.** Dein Verifier meldet sich alle
   5 Minuten beim Netz an. Jeder Coordinator prüft die Anmeldung selbst nach
   und nutzt deinen Dienst, solange er antwortet. Seine Unterschriften zählen
   in jeder Registrierung, die jeder Knoten selbst prüft (ab dem
   08.11.2026 18:00 UTC). Fällt dein Server aus, wird er nach spätestens
   30 Minuten nicht mehr genutzt, ohne dass jemand etwas tun muss.

Warum es die Abstimmung braucht: Würde jeder Schlüssel, der sich selbst
einträgt, sofort zählen, könnte ein Mensch mit genug Schlüsseln allein ein
ganzes Quorum stellen (Prüfbefund K2, 30.09.2026). Deshalb zählen nur
Prüfer, und Prüfer wird man nur mit den Stimmen von zwei Dritteln der Prüfer.
Und eine Registrierung, an der ein Prüfer-Schlüssel beteiligt ist, braucht
immer mindestens zwei verschiedene Menschen: niemand kann allein Menschen
erfinden.

Bis zur Aufnahme läuft dein Verifier, bekommt aber noch keine Anfragen.

## Was du speicherst, und was nie

- Ein Foto wird nie gespeichert. Nach der Prüfung bleibt nur eine 64-Byte-
  Skizze (512 Ja/Nein-Werte). Aus ihr lässt sich kein Gesicht zurückrechnen,
  eine zweite Registrierung desselben Menschen erkennt sie aber wieder.
- Das Skript startet im **Testbetrieb** (`ALLOW_REAL_BIOMETRIC_DATA=false`),
  wie alle Verifier heute. Echtbetrieb braucht eine eigene Entscheidung und die
  rechtliche Freigabe (`RECHTSTEXTE_FREISCHALTEN.md`).

## Läuft er?

```bash
curl -s https://DEINE-ADRESSE.sslip.io/health
```

`"status":"ok"` und `"sketch_seed_configured":true` heißt: läuft, mit deiner
eigenen Projektion. Das Log: `docker compose logs -f verifier` (Strg+C beendet
nur die Anzeige).

## Aktualisieren

```bash
cd ~/Aequitas && git pull && cd deploy/verifier && bash einrichten.sh
```

Die Programmversion steht fest im Skript, damit alle Verifier dasselbe prüfen.
Eine neue Version kommt mit dem Repo; das erneute Einrichten übernimmt sie und
lässt deine Schlüssel in `.env`, wie sie sind.

Fehlt in einer älteren `.env` die Zeile `VALIDATOR_BETREIBER_WALLET`, trägt das
erneute Einrichten sie einmalig aus `OPERATOR_WALLET` nach (ein zweiter Lauf
ändert nichts mehr). Steht dort schon eine andere Wallet als in
`OPERATOR_WALLET`, überschreibt das Skript nichts und bricht ab.

## Was wo liegt

| | Wo | Wofür |
|---|---|---|
| Bezeugungsschlüssel (`VALIDATOR_SIGNING_KEY`) | nur auf dem Server, in `.env` | der Verifier unterschreibt damit seine Aussagen |
| Datenschlüssel und Projektion | nur auf dem Server, in `.env` | verschlüsseln die gespeicherten Skizzen |
| Adresse deiner Wallet (`OPERATOR_WALLET`, `VALIDATOR_BETREIBER_WALLET`) | auf dem Server, in `.env` | wem der Verifier gehört; nur für sie gibt er den Bezeugungsnachweis aus |
| Deine Wallet, Wörterliste, privater Schlüssel | **nur bei dir** | – |

**Sichere die Datei `.env`** (zum Beispiel mit
`scp root@DEINE-SERVER-IP:Aequitas/deploy/verifier/.env .`). Geht sie verloren,
ist dein Verifier ein neuer: neue Schlüssel, neue Aufnahme.

## Wenn etwas nicht geht

| Meldung | Bedeutung | Was tun |
|---|---|---|
| „Das Verifier-Image ist nicht abrufbar“ | Download unterbrochen oder GitHub kurz nicht erreichbar | `bash einrichten.sh` erneut starten |
| „Port 80 ist belegt“ | auf dem Server läuft schon ein Webserver | einen eigenen Server für den Verifier nehmen |
| „HTTPS noch nicht erreichbar“ | Port 80 oder 443 beim Anbieter gesperrt | in der Firewall des Anbieters freigeben, dann `bash einrichten.sh` erneut |
| „Liste der Coordinatoren nicht lesbar“ | das Netz war kurz nicht erreichbar | später erneut starten |
| `sketch_seed_configured: false` | die `.env` ist beschädigt | `.env` aus der Sicherung zurückspielen |
| „VALIDATOR_BETREIBER_WALLET … ist nicht OPERATOR_WALLET“ | in `.env` stehen zwei verschiedene Wallets | beide auf deine Wallet setzen, dann `bash einrichten.sh` erneut |
| „Kein Bezeugungsnachweis“ | der Verifier stellt den Nachweis nicht aus — meist fehlt `VALIDATOR_BETREIBER_WALLET` oder sie ist nicht deine Wallet | `.env` prüfen, `bash einrichten.sh` erneut; sonst `docker compose logs --tail 50 verifier` |
