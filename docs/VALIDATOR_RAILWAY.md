# Dritter Validator auf Railway — Schritt für Schritt

**Stand 26.09.2026.** Für jemanden ohne eigenen Server. Der normale Weg
(eigener VPS mit Docker) steht in `VALIDATOR_EINRICHTEN.md`. Railway geht
auch, hat aber drei Eigenheiten. Sie stehen jeweils an der Stelle, an der sie
wichtig werden.

Preise und Plangrenzen von Railway ändern sich. Was hier dazu steht, ist
eine Einschätzung. Prüfe es vor dem Start auf https://railway.com/pricing.

---

## 0. Vorher klären (5 Minuten)

| | Was | Warum |
|---|---|---|
| 1 | **Du bist ein registrierter Mensch** (Registrierung in der Aequitas-App abgeschlossen, eigene Wallet). | Ein Mensch = ein Validator. Das Netz lehnt jede andere Wallet ab. |
| 2 | **Railway-Konto mit Pro-Plan.** | Die Datenbank ist heute ~20 GB groß und wächst. Das Volumen des kleinen Plans reicht dafür nicht. Rechne grob mit 8 GB RAM für den Knoten und 2–4 GB für Postgres. Das kostet im Monat eher dreistellig als zweistellig. |
| 3 | **Eine eigene Adresse**, am besten `validator3.aequitas.digital` (vom Domain-Inhaber als CNAME auf Railway gesetzt). | Railway hat Adressen in der Vergangenheit zweimal neu vergeben (`MIGRATION_RAILWAY_TO_CONTABO.md`). Unter der eigenen Adresse bleibt der Knoten für die anderen erreichbar. |

---

## 1. Projekt und Datenbank anlegen

1. https://railway.com → **New Project** → **Deploy PostgreSQL**.
   Der Dienst heißt danach `Postgres`.
2. Im Postgres-Dienst → **Settings** → **Volume**: auf **mindestens 60 GB**
   stellen, besser 100 GB.
3. Postgres wie auf den Gründerboxen einstellen: Dienst `Postgres` →
   **Data** → **Query** und nacheinander ausführen:
   ```sql
   ALTER SYSTEM SET max_connections = 250;
   ALTER SYSTEM SET shared_buffers = '1GB';
   ALTER SYSTEM SET synchronous_commit = off;
   ALTER SYSTEM SET max_wal_size = '8GB';
   ALTER SYSTEM SET checkpoint_timeout = '15min';
   ALTER SYSTEM SET wal_compression = on;
   CREATE DATABASE aequitas;
   ```
   Danach den Postgres-Dienst einmal neu starten (**⋮** → **Restart**).
   `synchronous_commit=off` ist Absicht: Die Haltbarkeit liegt im WAL des
   Knotens, nicht in den Postgres-Commits.

## 2. Den Knoten anlegen

1. Im selben Projekt → **New** → **GitHub Repo** →
   `hanoi96international-gif/Aequitas`, Branch `main`.
   Railway findet das `Dockerfile` im Hauptordner und baut daraus. Das
   dauert beim ersten Mal etwa 10 Minuten.
2. Dienst umbenennen in `aequitas-node`.
3. **Settings** → **Volume** → **Add Volume**, Mount-Pfad **`/data/wal`**,
   10 GB. Darin liegt das Transaktions-WAL. Ohne Volumen wäre es nach
   jedem Neustart weg.
4. **Settings** → **Resources**: 8 vCPU / 8 GB RAM (oder was der Plan
   hergibt, mindestens 4 GB).

## 3. Variablen setzen

Dienst `aequitas-node` → **Variables** → **Raw Editor** → einfügen und die
zwei Werte ersetzen (`SELF_URL`, `NODE_OPERATOR_WALLET`):

```
DATABASE_URL=postgres://${{Postgres.PGUSER}}:${{Postgres.PGPASSWORD}}@${{Postgres.RAILWAY_PRIVATE_DOMAIN}}:5432/aequitas?sslmode=disable
ANNAHME_ROLLE=nur_lesend
SELF_URL=https://validator3.aequitas.digital
NODE_OPERATOR_WALLET=0xDEINE_MENSCHEN_WALLET
PRIMARY_NODE_URLS=http://188.172.229.121:8080,http://194.163.188.71:8080
PRIMARY_NODE_URL=http://188.172.229.121:8080
AEQUITAS_WAL_ENABLED=1
AEQUITAS_WAL_PATH=/data/wal/aequitas_transfers.wal
AUTO_HEAL_ON_DIVERGENCE=true
AEQUITAS_DIVERGENZ_AUTORESYNC=1
GOMEMLIMIT=5GiB
API_PORT=8080
GIT_COMMIT=${{RAILWAY_GIT_COMMIT_SHA}}
```

`DATABASE_URL` bleibt genau so. Railway setzt die `${{…}}`-Teile selbst
ein. Der Knoten spricht die Datenbank `aequitas` über das interne Netz an,
deshalb ohne SSL.

**`ANNAHME_ROLLE=nur_lesend` auf keinen Fall weglassen.** Heute nimmt genau
ein Knoten im Netz Überweisungen an, nämlich C1. Ein zweiter Annehmender
kann die Kontostände dauerhaft auseinanderlaufen lassen. Dein Knoten
produziert trotzdem Blöcke, prüft alles nach, bekommt Belohnungen und zählt
als Validator. Er nimmt nur keine neuen Überweisungen an. Das ändert sich
erst mit Stufe 2 (`SKALIERUNG_DEZENTRAL.md`), und dafür braucht es genau
dich als dritten Validator.

`RELAYER_PRIVATE_KEY` und `NODE_KEY` lässt du vorerst leer (Schritt 5).

## 4. Erreichbar machen

1. `aequitas-node` → **Settings** → **Networking** → **Custom Domain** →
   `validator3.aequitas.digital`, **Port 8080**. Railway zeigt ein
   CNAME-Ziel an. Das trägt der Domain-Inhaber beim Registrar ein.
   Ohne eigene Domain geht es vorübergehend auch mit
   **Generate Domain** (`…up.railway.app`, Port 8080). Diese Adresse
   gehört dann als `SELF_URL` eingetragen.
2. **P2P (Port 4001) ist nicht nötig.** Dein Knoten verbindet sich selbst
   nach außen zu C1 und C2 und synchronisiert zusätzlich über HTTP. Einen
   TCP-Proxy für 4001 kannst du anlegen, musst du aber nicht.

## 5. Erster Start und Schlüssel sichern

1. **Deploy** klicken (oder er startet nach dem Setzen der Variablen von
   selbst).
2. **Deployments** → **View Logs**. Beim ersten Start steht dort je eine
   Zeile `SET THIS AS RELAYER_PRIVATE_KEY …` und
   `SAVE THIS AS NODE_KEY …`, jeweils mit dem Wert in der Zeile darunter.
3. Beide Werte **sofort** unter **Variables** eintragen. Sie sind geheim
   und gehören nirgends sonst hin. Ohne sie bekommt der Knoten bei jedem
   Neustart eine neue Identität, und das Netz hält ihn für jemand anderen.
   Railway startet den Dienst danach neu.
4. Der Knoten lädt jetzt den Zustand des Netzes (Snapshot, signiert) von
   C1/C2 und holt die Blöcke nach. Im Log erscheinen
   `[BOOTSTRAP] Fresh node — importing state from …`, dann
   `[HTTP-SYNC] Added … new blocks`, und zuletzt `[Block #…]`-Zeilen.
   Je nach Datenmenge dauert das 30–90 Minuten.

## 6. Knoten an deine Wallet binden

Der Signierschlüssel des Knotens ist ein eigener Schlüssel, nicht der deiner
Wallet. Das ist Absicht: Wer Railway knackt, bekommt so nicht dein Geld.

1. Öffne **auf deinem eigenen Knoten**
   `https://validator3.aequitas.digital/node-binding`, im Browser mit deiner
   Menschen-Wallet (MetaMask o. ä.). Nicht auf aequitas.digital: Die Seite
   lässt den Knoten, auf dem sie läuft, seinen eigenen Schlüssel beweisen.
2. Das Feld „matching service“ bleibt leer. Du betreibst keinen.
3. **Connect Wallet & Register** und mit der Wallet signieren.
   Am Ende zeigt die Seite `NODE_OPERATOR_BINDING_SIGNATURE=0x…`. Diesen
   Wert unter **Variables** eintragen.

**Nie** den privaten Schlüssel deiner Wallet als `RELAYER_PRIVATE_KEY`
eintragen.

## 7. Prüfen, ob alles läuft

Im Browser oder mit `curl`:

| Aufruf | Erwartet |
|---|---|
| `https://validator3.aequitas.digital/api/status` | `height` fast gleich wie bei `https://aequitas.digital/api/status` (wenige Blöcke Abstand) |
| `https://validator3.aequitas.digital/api/health/combined` | `annahme_tor.nimmt_an` = **false** |
| `https://validator3.aequitas.digital/api/wache` | HTTP 200 |
| `https://aequitas.digital/api/validators` | deine Wallet taucht auf |

Dann dem Netzbetreiber Bescheid geben. Er prüft von seiner Seite aus
(`neuer-validator-probe.yml`).

## 8. Betrieb

- **Updates:** Railway baut bei jedem Push auf `main` neu. Das ist
  bequem, aber **alle Validatoren dürfen nie gleichzeitig neu starten**.
  Stell unter **Settings** → **Deploy** das automatische Deployment
  besser **aus** und klick **Deploy** erst, wenn der Netzbetreiber sagt,
  dass C1 und C2 durch sind.
- **Selbstheilung:** Fällt der Knoten zurück oder weicht er ab, holt er
  sich den Zustand selbst neu (`[AUTO-HEAL]` im Log). Du musst nichts tun.
- **Kosten im Blick behalten:** Railway rechnet nach Verbrauch ab. Unter
  Last braucht der Knoten 3–4 Kerne und bis 5 GB RAM, in Ruhe unter 1 GB.
  Setz unter **Usage** eine Obergrenze (Usage Limit), damit keine
  Überraschung kommt.
- **Aufhören:** Dienst stoppen genügt. Das Netz läuft ohne dich weiter.
  Gib vorher Bescheid, damit Stufe 2 nicht auf dich zählt.

## Wenn etwas nicht geht

| Log-Zeile / Symptom | Ursache | Lösung |
|---|---|---|
| `connection refused` / `database "aequitas" does not exist` | Datenbank fehlt oder falsche URL | Schritt 1.3 (`CREATE DATABASE aequitas`) und `DATABASE_URL` aus Schritt 3 prüfen |
| Knoten startet immer wieder neu, `out of memory` | zu wenig RAM | Resources erhöhen oder `GOMEMLIMIT` senken (z. B. `3GiB`) |
| `no space left on device` | Postgres-Volumen voll | Volumen vergrößern (Schritt 1.2) |
| Höhe steigt nicht | Snapshot/Sync hängt | Log nach `[BOOTSTRAP]`/`[HTTP-SYNC]` durchsehen; Netzbetreiber fragen |
| `NODE_OPERATOR_WALLET is not a registered human` | Wallet nicht registriert | erst die Registrierung in der App abschließen |
| `nimmt_an: true` | `ANNAHME_ROLLE` fehlt | sofort `ANNAHME_ROLLE=nur_lesend` setzen |
