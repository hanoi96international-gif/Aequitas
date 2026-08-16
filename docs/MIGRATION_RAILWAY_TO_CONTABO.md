# Migration: Railway → Contabo

**Stand: 2026-08-14.** Railway ist abgeschaltet und hostet keinen Teil dieses
Netzwerks mehr. Dieses Dokument beschreibt, was im Code bereits umgestellt
wurde, und was am **18.08.2026** (Freigabe der Domain `aequitas.digital`) noch
manuell zu tun ist.

---

## 1. Ausgangslage (live verifiziert am 2026-08-14)

| Prüfung | Ergebnis |
|---|---|
| `https://aequitas.digital/api/status` | **HTTP 404** — DNS zeigt auf `69.46.46.73`, nicht auf Contabo |
| P2P-Bootstrap `reseau.proxy.rlwy.net:41277` | Railway-Proxy, existiert nicht mehr |
| `/api/peers` auf C1 **und** C2 | `{"peers":null}` — **beide Nodes kennen null Peers** |
| TLS auf C1/C2 Port 443 | Handshake schlägt für **jeden** Hostnamen fehl — kein gültiges Zertifikat |
| Port 80 auf C1/C2 | 308 Redirect → HTTPS (Reverse Proxy läuft, aber TLS ist unbrauchbar) |
| Einziger funktionierender Zugang | `http://<ip>:8080` direkt |

**Kernproblem:** Der eingebaute P2P-Bootstrap *und* der eingebaute HTTP-Seed
zeigten beide auf Railway. Beide starben gleichzeitig. Ein Node ohne
explizite Konfiguration konnte das Netz über **keinen** Transportweg
erreichen — deshalb `peers:null` auf beiden Boxen.

### Adressen

| Rolle | IP | P2P-PeerID (aus `/api/status` → `node_id`) |
|---|---|---|
| Contabo1 | `173.249.37.118` | `12D3KooWHfPy6g3jvyC1mvqzCHvy5QBsDmHHsvfwvwXQGrtQ2pVm` |
| Contabo2 | `194.163.188.71` | `12D3KooWBv34kuVcmNDxZT4kCZFvNVGhy4zgkBZDGMtp7YSx2UUN` |

---

## 2. Was im Code bereits umgestellt ist

- **`x/humanity/keeper/p2p.go`** — `defaultBootstrapNodes` zeigt jetzt auf
  beide Contabo-Boxen als IP-Multiaddr (`/ip4/…/tcp/4001/p2p/…`). Bewusst per
  IP statt DNS: dedizierte Server mit statischer Adresse, keine
  Plattformschicht mehr, die Hostnamen neu generiert (was Railway zweimal tat).
- **`x/humanity/keeper/p2p.go`** — der Selbst-Ausschluss beim Bootstrap
  verglich gegen die **hartkodierte PeerID des alten Railway-Primary**. Diese
  konnte nie wieder greifen; C1/C2 hätten sich nach der Umstellung mit sich
  selbst verbunden. Ersetzt durch `bootstrapAddrIsSelf()`, das die PeerID aus
  der Multiaddr gegen die eigene vergleicht.
- **`x/humanity/keeper/sync_blocks.go`** — `defaultPublicSeeds` ist jetzt eine
  Liste: `aequitas.digital` zuerst (Langfrist-Ziel), dahinter beide
  Validator-IPs über `http://…:8080` als Fallback. `isAllowedPeerURL` erlaubt
  `http` für **literale IPs** (die HTTPS-Pflicht existiert gegen DNS-Rebinding,
  wogegen eine IP-Adresse nicht anfällig ist).
- **`deploy/Caddyfile`** — neu, TLS-Terminierung für die Domain.
- **Regressionstests** — `TestDefaultBootstrapNodes_NoDecommissionedHosts` und
  `TestDefaultPublicSeeds_NoDecommissionedHosts` lassen den Build rot werden,
  falls je wieder ein `rlwy.net`/`railway.app`-Host als Default auftaucht.

---

## 3. Checkliste für den 18.08.2026

### 3.1 DNS
```
aequitas.digital.      A    173.249.37.118
www.aequitas.digital.  A    173.249.37.118
```
Vor dem nächsten Schritt abwarten, bis die Auflösung greift:
```bash
dig +short aequitas.digital
```

### 3.2 TLS auf Contabo1
```bash
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```
Caddy holt das Zertifikat automatisch über Let's Encrypt (HTTP-01, Port 80).
Port 80 muss offen bleiben — ist er bereits.

### 3.3 Environment-Variablen

**Contabo1:**
```
SELF_URL=https://aequitas.digital
PRIMARY_NODE_URLS=http://194.163.188.71:8080
```

**Contabo2:**
```
SELF_URL=http://194.163.188.71:8080
PRIMARY_NODE_URLS=https://aequitas.digital
```

> **Wichtig — `SELF_URL` auf C2 braucht das explizite `http://`.**
> `NormalizeNodeURL` (sync_blocks.go) setzt bei fehlendem Schema automatisch
> `https://` davor. Ohne Präfix würde C2 sich als `https://194.163.188.71:8080`
> registrieren, was mangels Zertifikat nicht erreichbar ist.

> **Warum `PRIMARY_NODE_URLS` auf beiden Boxen gesetzt sein muss:**
> Sobald das gesetzt ist, greift die eingebaute Default-Seed-Liste nicht mehr.
> Das ist hier notwendig, weil diese Liste die beiden Boxen selbst enthält —
> und `seedURLs` kann nur eine **exakte** `SELF_URL`-Übereinstimmung
> herausfiltern. Es kann nicht wissen, dass `https://aequitas.digital` und
> `http://173.249.37.118:8080` dieselbe Maschine sind.

### 3.4 Verifikation

```bash
curl -s https://aequitas.digital/api/status | head -c 300
```
Danach — **das ist der eigentliche Test**, nicht die Höhe:
```bash
curl -s https://aequitas.digital/api/peers
curl -s http://194.163.188.71:8080/api/peers
```
Beide müssen sich gegenseitig auflisten. `{"peers":null}` bedeutet, dass die
Migration nicht funktioniert hat.

P2P separat prüfen (Port 4001 wird **nicht** über Caddy geleitet):
```bash
nc -vz 173.249.37.118 4001
nc -vz 194.163.188.71 4001
```

Konvergenz **immer am Hash** prüfen, nie an der Höhe:
```bash
curl -s "https://aequitas.digital/api/block?height=<N>" | jq -r .hash
curl -s "http://194.163.188.71:8080/api/block?height=<N>" | jq -r .hash
```
`<N>` nah am aktuellen Tip wählen — siehe Warnung unten.

> ⚠️ **Keine tiefen Höhen abfragen.** Eine Anfrage weit unterhalb des Tips
> hat am 2026-08-14 Contabo1 für über 32 Minuten komplett blockiert. Ursache
> und Fix: siehe `maxCanonicalWalkHops` in `x/humanity/keeper/block.go`. Nach
> Ausrollen dieses Fixes ist das entschärft; davor gilt die Warnung absolut.

---

## 4. Proof-Server (separates Repo: `aequitas-proof-server`)

**Live-Befund 2026-08-14:** `GET http://194.163.188.71:8080/api/prove/get/<id>`
antwortet **HTTP 503 — „no PROOF_SERVER_URL/PROOF_SERVER_URLS configured on
this node"**. Auf Contabo2 ist also **gar kein Proof-Server konfiguriert**;
die Registrierung neuer Menschen ist dort komplett funktionsunfähig. Das deckt
sich mit `total_humans: 15` seit Wochen.

Auf keiner der beiden Boxen ist von außen ein Proof-Server-Port offen
(3000/5000/8000/8001/9000 alle zu) — nur 8080. Das ist als Endzustand richtig
(der Dienst soll nur lokal lauschen und über den Node-Proxy erreichbar sein),
belegt aber nicht, dass er läuft.

### 4.1 Betrieb auf Contabo

Das Repo hat bereits ein (noch **uncommittetes**) `Dockerfile`
(`node:20-alpine`, `EXPOSE 3000`). Pro Validator-Box eine eigene Instanz —
das ist die vorgesehene Architektur (`PROOF_SERVER_URLS` erlaubt genau
deshalb mehrere Einträge, siehe `proofServerURLs()` in `api.go`).

Auf dem Chain-Node dann setzen:
```
PROOF_SERVER_URLS=http://127.0.0.1:3000
```

### 4.2 Drei Railway-Kopplungen, die beim Umzug greifen müssen

Alle drei sind stillschweigend — nichts davon schlägt beim Start fehl.

**(a) Produktionserkennung — sicherheitsrelevant.**
`server.js:23` verweigert den Start, wenn `ALLOW_UNAUTHENTICATED_DEV=true`
*zusammen mit* einer erkannten Produktionsumgebung gesetzt ist. Erkannt wird
sie über `RAILWAY_ENVIRONMENT === 'production'` **oder**
`NODE_ENV === 'production'`. Auf Contabo ist `RAILWAY_ENVIRONMENT` nie
gesetzt — **`NODE_ENV=production` muss also explizit gesetzt werden**, sonst
ist dieser Schutz wirkungslos und ein versehentlich gesetztes
`ALLOW_UNAUTHENTICATED_DEV=true` würde die Chain-Authentifizierung
stillschweigend abschalten.

**(b) `trust proxy` — sicherheitsrelevant, in beide Richtungen.**
`server.js:39-49`: ohne gesetztes `TRUST_PROXY` und ohne
`RAILWAY_ENVIRONMENT` ist der Wert `false`. Das ist der *sichere* Default für
einen VPS ohne Proxy. Sobald der Proof-Server aber **hinter Caddy** liegt,
muss `TRUST_PROXY=1` gesetzt werden — sonst sieht `express-rate-limit` nur
die Proxy-IP, und alle Clients teilen sich ein einziges Rate-Limit-Budget.
Umgekehrt darf `TRUST_PROXY` **nicht** gesetzt werden, wenn der Dienst direkt
erreichbar ist: dann könnte jeder Client sein Limit per gefälschtem
`X-Forwarded-For` umgehen.
*Bei der empfohlenen Variante (nur `127.0.0.1:3000`, Zugriff ausschließlich
über den Node-Proxy) bleibt `TRUST_PROXY` ungesetzt.*

**(c) `CHAIN_BASE_URL`.**
`.env.example` hat `CHAIN_BASE_URL=https://aequitas.digital` — aktuell tot.
Einen Default gibt es im Code bewusst nicht mehr (der Dienst schlägt laut
fehl, statt still gegen eine fremde Chain zu prüfen). Auf
`http://127.0.0.1:8080` setzen — dann prüft jede Instanz gegen ihren eigenen
Node, ohne Netzumweg und ohne Abhängigkeit von der Domain.

**(d) Postgres-TLS.**
`bio_store.js:15`: `dbSslRejectUnauthorized = !process.env.RAILWAY_ENVIRONMENT`.
Ohne `RAILWAY_ENVIRONMENT` ist die Zertifikatsprüfung also **aktiv** — richtig
so, aber gegen ein lokales Postgres mit selbstsigniertem Zertifikat schlägt
die Verbindung dann fehl. Für eine reine Loopback-Verbindung `sslmode=disable`
in der `DATABASE_URL` verwenden.

### 4.3 Verifikation

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://194.163.188.71:8080/api/prove/get/probe
```
`503` = kein Proof-Server konfiguriert (aktueller Zustand).
`502` = konfiguriert, aber nicht erreichbar.
`404`/`400` = Proof-Server antwortet — **das ist das Ziel**.

---

## 5. Noch offen / manuell zu erledigen

- **Restliche Railway-Artefakte löschen** (Löschung war in der Audit-Sitzung
  durch die Berechtigungsprüfung blockiert):
  ```bash
  git rm railway.toml Procfile && git rm -r cmd/dbmigrate cmd/dbbackfill
  ```
  `railway.toml`/`Procfile` sind reine Railway-Deploy-Konfiguration.
  `cmd/dbmigrate` und `cmd/dbbackfill` rufen die `railway`-CLI auf und sind
  laut ihrem eigenen Kopfkommentar Einmal-Werkzeuge („run once …, then
  delete"). Keines wird von Code referenziert.
- **`deploy_v7.cjs`** — `RPC_URL`-Default zeigt auf
  `aequitas-production-9fba.up.railway.app/rpc`.
- **`build/generate_all_guides.py`** — erzeugt die Node-Betreiber-PDFs in 12
  Sprachen als reine Railway-Anleitung („Ein Railway-Konto (kostenlos)…").
  Die PDFs werden über `/download/node-guide-<lang>.pdf` ausgeliefert und
  beschreiben damit einen nicht mehr existierenden Weg.
- **`README.md`** — Bootstrap-Multiaddr zeigt auf `thomas.proxy.rlwy.net`.
- **`~/.ssh/config`** (lokal) — enthält einen toten `railway-aequitas`-Host.
- **`IS_PRIMARY_NODE`** ist auf keiner Box gesetzt. Betrieblich unkritisch
  (die UBI-Verteilung nutzt `TryLockDistribution`, einen Postgres-CAS-Lock,
  nicht diese Variable), aber sie deaktiviert den `RESET_DB_STATE`-Schutz in
  `resetDBStateForBootstrap`. Auf C1 auf `true` setzen ist die sicherere
  Voreinstellung.
- **`git_commit: unknown`** in `/api/status` auf beiden Boxen — der Build
  bekommt keine Commit-Information eingebettet. Damit ist im Betrieb nicht
  feststellbar, welcher Code tatsächlich läuft. Bei beiden Boxen mit
  Uptime > 10 Tagen heißt das: der Stand ist unbekannt.
