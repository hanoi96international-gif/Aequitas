# Einen Aequitas-Validator betreiben

**Stand 30.09.2026, nach dem Neustart des Netzes bei null.** Ersetzt die
Fassung vom 12.09.2026.

## Worum es geht, in drei Sätzen

Ein Validator ist ein Rechner, der die Kette mitführt, jeden Block selbst
nachprüft und selbst Blöcke erzeugt. Für diese Arbeit bekommt der Mensch, dem
er gehört, jeden Tag einen gleichen Anteil am Validator-Topf. Ein Mensch kann
genau einen Validator betreiben. Rechenleistung kaufen bringt keine
zusätzliche Stimme.

## Was du brauchst

| | Mindestens | Wie die Gründerboxen |
|---|---|---|
| Server | ein gemieteter Server (VPS) mit Ubuntu 22.04 oder 24.04 und einer öffentlichen IPv4-Adresse | netcup, 8 Kerne |
| Arbeitsspeicher | 8 GB | 15 GB |
| Festplatte | 60 GB SSD | 250 GB |
| Offene Ports | 8080 (Schnittstelle) und 4001 (Verbindung zu anderen Knoten) | dasselbe |
| Du selbst | in der Aequitas-App **registriert** (Gesichtsprüfung abgeschlossen) | – |

Einen passenden Server gibt es bei den meisten Anbietern für 10 bis 20 Euro
im Monat. Beim Bestellen „Ubuntu 24.04“ wählen. Die Zugangsdaten (IP-Adresse
und Passwort) kommen per E-Mail.

**Was nicht auf den Server gehört:** deine Wallet, ihre Wörterliste
(Seed-Phrase) und ihr privater Schlüssel. Auf dem Server liegt nur die
**Adresse** deiner Wallet. Wer den Server knackt, kommt so nicht an dein Geld.

## Einrichten: ein Befehl, zwei Fragen

Mit dem Server verbinden (auf Windows mit „PowerShell“, auf dem Mac mit
„Terminal“):

```bash
ssh root@DEINE-SERVER-IP
```

Dann diese drei Zeilen einfügen:

```bash
curl -fsSL https://get.docker.com | sh
git clone https://github.com/hanoi96international-gif/Aequitas.git
cd Aequitas/deploy/validator && bash einrichten.sh
```

Die erste Zeile installiert Docker (falls noch nicht da), die zweite holt
Aequitas, die dritte richtet alles ein. Das Skript fragt nur nach:

1. **deiner Wallet-Adresse**, der Adresse, mit der du dich in der App
   registriert hast. Sie steht in der App unter „Empfangen“ und beginnt mit `0x`.
2. **Bestätigung der IP-Adresse**, die es selbst ermittelt.

Alles andere erledigt es selbst: ein zufälliges Datenbank-Passwort, die
Konfiguration, Bauen und Starten (beim ersten Mal etwa 10 Minuten) und das
dauerhafte Speichern der beiden Schlüssel, die dein Knoten beim ersten Start
für sich erzeugt.

## Am Ende: QR-Code scannen

Die Belohnungen gehen an deine Wallet. Dafür muss das Netz wissen, dass
dieser Knoten dir gehört. Das erledigst du mit dem Handy:

1. Das Skript zeigt am Ende einen **QR-Code** im Terminal.
2. Öffne die Aequitas-App → Tab **„Knoten“** → **„Knoten binden (QR scannen)“**
   und scanne den Code.
3. Die App zeigt die Signieradresse deines Knotens. Tippe auf **„Bestätigen“**.

Die Unterschrift kostet nichts und bewegt kein Geld. Das Skript merkt die
Bestätigung nach wenigen Sekunden, trägt die Bindung selbst ein und startet den
Knoten neu. Du musst nichts kopieren und nichts eintippen. Kommt innerhalb von
15 Minuten keine Bestätigung, einfach `bash einrichten.sh` noch einmal starten.

Die App prüft dabei selbst, dass der Code für **deine** Wallet erzeugt wurde
und dass der Knoten den Schlüssel zur gezeigten Signieradresse wirklich hat.
Ein fremder oder verfälschter Code wird abgelehnt, bevor unterschrieben wird.

**Ohne die App** (zum Beispiel mit MetaMask im Browser): die Bindungsseite des
eigenen Knotens `http://DEINE-SERVER-IP:8080/node-binding` öffnen, mit derselben
Wallet unterschreiben, die angezeigte Zeile `NODE_OPERATOR_BINDING_SIGNATURE=…`
in `.env` eintragen und `docker compose up -d node` ausführen.

## Danach: Aufnahme als Blockproduzent

Heute nimmt der Betreiber neue Blockproduzenten noch von Hand auf. Der Grund:
Solange nicht jeder Knoten jeden Wert eines Blocks selbst nachrechnet, könnte
ein böswilliger Produzent sonst Geld erzeugen. Sobald diese Prüfung vollständig
ist, entfällt die Aufnahme, und jeder registrierte Mensch wird mit seinem Knoten
automatisch Produzent.

Schick dem Betreiber deine Signieradresse. Bis zur Aufnahme läuft dein Knoten
als vollwertiger **Beobachter** mit: Er führt die ganze Kette und prüft jeden
Block selbst nach.

## Läuft er?

```bash
curl -s http://localhost:8080/api/status | grep -oE '"height":[0-9]+'
curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+'
```

Beide Zahlen sollten bis auf ein paar Blöcke gleich sein. Die Selbstprüfung
`curl -s http://localhost:8080/api/wache` antwortet mit 200, wenn alles da ist.
Sonst nennt sie den Befund.

Das Log ansehen: `docker compose logs -f node`. Mit Strg+C beendest du nur die
Anzeige, der Knoten läuft weiter.

## Aktualisieren

```bash
cd ~/Aequitas && git pull && cd deploy/validator && docker compose up -d --build
```

Der Knoten kommt nach einem Neustart von allein zurück und holt auf, was er
verpasst hat.

## Was wo liegt

| | Wo | Wofür |
|---|---|---|
| Signierschlüssel (`RELAYER_PRIVATE_KEY`) | nur auf dem Server, in `.env` | der Knoten unterschreibt damit seine Blöcke |
| P2P-Schlüssel (`NODE_KEY`) | nur auf dem Server, in `.env` | seine Kennung gegenüber anderen Knoten |
| Adresse deiner Wallet (`NODE_OPERATOR_WALLET`) | auf dem Server | wohin die Belohnungen gehen |
| Deine Wallet, Wörterliste, privater Schlüssel | **nur bei dir**, nie auf dem Server | – |

**Sichere die Datei `.env`** (zum Beispiel mit `scp root@DEINE-SERVER-IP:Aequitas/deploy/validator/.env .`
auf deinen Rechner). Geht sie verloren, bekommt der Knoten eine neue Identität,
und du musst neu binden und neu aufgenommen werden.

**Umzug auf einen neuen Server:** Die alte `.env` mitnehmen, dann bleibt alles
wie es war. Ohne sie ist es ein neuer Knoten: neu binden, neu aufnehmen lassen.

## Wenn etwas nicht geht

| Meldung | Bedeutung | Was tun |
|---|---|---|
| `NODE_OPERATOR_WALLET is not a registered human` | die Wallet ist nicht registriert | erst in der App registrieren |
| `operator_binding_signature missing or invalid` | die Bindung fehlt oder passt nicht | `bash einrichten.sh` erneut starten und den QR-Code scannen |
| `einrichten.sh`: „Keine Bestätigung aus der App angekommen“ | QR-Code nicht gescannt oder nicht bestätigt | `bash einrichten.sh` noch einmal starten und den neuen Code scannen |
| App: „Dieser Code wurde für eine andere Wallet erzeugt“ | beim Einrichten eine andere Adresse eingegeben | in `.env` `NODE_OPERATOR_WALLET` korrigieren oder neu einrichten |
| `ist registriert, aber nicht in AUTHORIZED_VALIDATORS` | noch nicht als Produzent aufgenommen | Signieradresse an den Betreiber schicken; bis dahin Beobachter |
| Höhe steht, `Not yet 3 consecutive clean sync cycles` | der Knoten holt noch auf | warten |
| Höhe bleibt hinter dem Netz zurück | Port 4001/8080 zu, oder zu wenig Arbeitsspeicher | Firewall des Anbieters prüfen, `docker stats` |
| `einrichten.sh`: „Der Knoten hat keinen Signierschlüssel gemeldet“ | der erste Start ist gescheitert | `docker compose logs node` ansehen und die letzten Zeilen dem Betreiber schicken |

## Für Fortgeschrittene

- Alle Einstellungen stehen kommentiert in `.env.example`. `ANNAHME_ROLLE=nur_lesend`
  bleibt stehen: Nur ein Knoten des Netzes nimmt Überweisungen an, zwei
  Annehmende brächten die Kontostände auseinander.
- Der Knoten heilt sich selbst: Fällt er zurück oder weicht ab, holt er den
  Zustand neu vom signierten Snapshot der Seeds (`AUTO_HEAL_ON_DIVERGENCE`,
  `AEQUITAS_DIVERGENZ_AUTORESYNC` in der Compose-Datei).
- Registrierungen neuer Menschen nimmt nur ein Knoten mit eigenem Proof-Server
  an. Für einen Validator ist das nicht nötig.
- Ohne eigenen Server: `VALIDATOR_RAILWAY.md` (Railway-Pro-Plan und eigene
  Adresse nötig, nichts für den Einstieg).
- Die zweite Rolle, der Verifier (Schutz gegen Doppelregistrierung), hat eine
  eigene Anleitung mit ebenfalls einem Befehl: `VERIFIER_EINRICHTEN.md`.
