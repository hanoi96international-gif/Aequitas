# Einen Aequitas-Validator betreiben

**Stand 12.09.2026.** Ersetzt den Node-Guide vom Juni 2026, der auf Railway
verwies — Railway gibt es für dieses Projekt nicht mehr. Was hier steht, ist
der Weg, den die beiden Gründerboxen gehen, als Datei statt als Skript, und
er wurde am 12.09.2026 mit einem leeren Rechner gegen das Live-Netz
durchgespielt (siehe unten, „Was zu erwarten ist").

## Was du brauchst

| | Minimum | Wie die Gründerboxen |
|---|---|---|
| Rechner | VPS, Ubuntu 22.04/24.04, öffentliche IPv4 | Contabo, 6 vCPU |
| RAM | 8 GB | 12 GB |
| Platte | 60 GB SSD (die Datenbank ist 18 GB und wächst) | 100+ GB |
| Software | Docker mit Compose-Plugin, git | dasselbe |
| Ports | 8080 (API) und 4001 (P2P) von außen erreichbar | dasselbe |
| Identität | ein **registrierter Mensch** (App-Registrierung abgeschlossen) | — |

Ein Mensch = ein Validator. Das Netz lehnt die Anmeldung eines Wallets ab,
das kein registrierter Mensch ist. Das ist Absicht — Rechenleistung kaufen
soll keine Stimme kaufen.

## Einrichten (etwa 15 Minuten, davon 10 Bauen)

```bash
git clone https://github.com/hanoi96international-gif/Aequitas.git
cd Aequitas/deploy/validator
cp .env.example .env
nano .env        # POSTGRES_PASSWORD, SELF_URL, NODE_OPERATOR_WALLET ausfüllen
docker compose up -d --build
```

Dann:

```bash
docker compose logs -f node
```

Beim ersten Start holt sich der Knoten den Zustand des Netzes (Snapshot) von
den Gründerboxen und prüft dessen Signatur; danach zieht er die Blöcke seit
dem Snapshot nach. Im Log erscheinen `[BOOTSTRAP] Fresh node — importing
state from …`, dann `[HTTP-SYNC] Added … new blocks`, und sobald er an der
Spitze ist, `[Block #…]`-Zeilen — das sind seine eigenen Blöcke.

Prüfen:

```bash
curl -s http://localhost:8080/api/status | grep -oE '"height":[0-9]+'
curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+'
```

Beide Zahlen müssen (bis auf ein paar Blöcke) gleich sein.

## Schlüssel sichern

Lässt du `RELAYER_PRIVATE_KEY` und `NODE_KEY` in `.env` leer, erzeugt der
Knoten beide beim ersten Start und druckt sie **einmal** ins Log (`SAVE THIS
AS …`). Trag sie danach in `.env` ein und starte neu (`docker compose up -d`),
sonst hat dein Knoten nach jedem Neustart eine neue Identität — und das Netz
sieht ihn als jemand anderen.

Ist `RELAYER_PRIVATE_KEY` nicht der Schlüssel deines `NODE_OPERATOR_WALLET`,
musst du die Bindung einmal beweisen: auf https://aequitas.digital/node-binding
die angezeigte Nachricht mit deinem Wallet signieren und die Signatur als
`NODE_OPERATOR_BINDING_SIGNATURE` eintragen.

## Aktualisieren

```bash
cd Aequitas && git pull && cd deploy/validator && docker compose up -d --build
```

Der Knoten kommt nach einem Neustart von allein zurück und holt auf, was er
verpasst hat. **Starte nie alle Validatoren des Netzes gleichzeitig neu** —
mit zwei Validatoren stand das Netz am 12.09.2026 genau deshalb still, bis
der Fehler behoben war; heute fängt der Knoten das ab, aber Reihe nach ist
trotzdem richtig.

## Was zu erwarten ist

- Ruhe: der Knoten braucht unter 1 GB, Postgres unter 2 GB.
- Unter Volllast (gemessen 12.09.2026, ~13.000 Überweisungen/s auf zwei
  Validatoren): Knoten bis 5 GB (`GOMEMLIMIT`), eine CPU-Kern-Auslastung
  von 3–4 Kernen, Platte wächst um mehrere GB je Stunde Volllast.
- Ein Validator ohne eigenen Proof-Server nimmt keine **Registrierungen** an
  (die Endpunkte antworten 503) — Überweisungen, Blöcke und Belohnungen
  funktionieren ohne ihn. Wer Registrierungen bedienen will, braucht die
  Coordinator/Proof-Server-Instanz aus dem App-Repo; das ist ein eigener
  Schritt und für einen Validator nicht nötig.

## Wenn etwas nicht geht

| Symptom | Ursache | Abhilfe |
|---|---|---|
| `NODE_OPERATOR_WALLET is not a registered human` | Wallet ist nicht registriert | erst in der App registrieren |
| `operator_binding_signature missing or invalid` | Signierschlüssel ≠ Wallet | Bindung auf /node-binding erzeugen |
| Höhe steht, `Not yet 3 consecutive clean sync cycles` | Knoten holt noch auf | warten; Produktion beginnt nach dem Aufholen |
| `SELF_URL not set — running in isolated mode` | SELF_URL fehlt | in `.env` setzen, neu starten |
| Höhe bleibt hinter dem Netz zurück | Port 4001/8080 nicht erreichbar, oder zu wenig RAM | Firewall prüfen, `docker stats` |
