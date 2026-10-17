# Beta-Bereitschaft — Prüfung vom 14.10.2026, Stand 16.10.2026

Geprüft wurden alle vier Repos: Kette, App, Biometrie und Proof-Server. Die
Fragen waren: funktioniert es, ist es sicher, ist es schnell, und ist der Weg
zu mehr Validatoren frei? Dieses Dokument hält fest, was **erledigt** ist,
was **im Code offen** ist und was **nur der Betreiber** tun kann.

Massstab ist AGENTS.md, „Sicherheit — Launch-Massstab“. Jede Änderung unten
hatte eine getrennte Sicherheitsprüfung; das Ergebnis steht als Kommentar im
jeweiligen PR.

## 1. Erledigt (gemergt)

| Befund | Schwere | PR |
|---|---|---|
| Checkpoints: die Sperren vor dem Stichtag gelöst; Köpfe im Snapshot; der Erzeuger schreibt den Wert in derselben DB-Transaktion wie den Block. Alles schlafend. | – | Aequitas #348–#352 |
| Validatorenliste konsensfest (Hash) und Runbook `VALIDATOREN_HINZUFUEGEN.md` | – | Aequitas #349 |
| Private Schlüssel (`RELAYER_PRIVATE_KEY`, `NODE_KEY`) standen beim ersten Start im Log. Jetzt gibt es `aequitasd schluessel`; Log-Ausgaben werden geschwärzt. | HOCH | Aequitas #353 |
| Ketten-Register (Selbstbedienung) bestimmte Vergleichsdienste und Coordinator-Schlüssel. Jetzt gelten nur die Listen des Betreibers. Der Deploy bricht ab, wenn der eigene Coordinator-Schlüssel fehlt. | BLOCKER | biometric #44 |
| Einwilligung wurde nur im Modus „real“ geprüft. Jetzt in jedem Modus. | BLOCKER | biometric #45 |
| `/challenge` füllte den Speicher ohne Grenze. Jetzt Grenze je Adresse (IPv6 je /64), Obergrenze 20 000, darüber 503. | HOCH | biometric #45 |
| Root-SSH-Workflows liefen bei Push auf einen Zweig an, ohne Umgebung und ohne Rechte-Block. Jetzt nur von Hand, in `produktion`, mit `contents: read`. | BLOCKER | biometric #46, proof-server #12 |
| `/prove`: das gemeinsame IP-Budget ließ sich mit unbescheinigten Anfragen leeren. Es zählt jetzt erst nach der Bescheinigung; höchstens 17 Signaturen. | MITTEL | proof-server #13 |
| Proof-Server: Node 20 (ohne Sicherheitsupdates) auf 22; `npm audit` meldet 0 offene Befunde. | MITTEL | proof-server #13 |

| App: Zeitgrenze für jede Anfrage (20/40/240 s), nur https, fatale Fehler werden nicht mehr verschluckt, Keystore-Passwörter nicht in der Prozessliste, keine Injection im Release-Workflow | MITTEL | App #31 |
| Ein nur lesender Knoten kann nie mehr Leiter werden; Systemaufträge (tägliche Verteilung) laufen nur beim Leiter mit Lease; kein Verteilungstag fällt aus | BLOCKER | Aequitas #354 |

| Gemerkte Peers über Neustarts, Leitungsmitglieder als Sync-Peers (höchstens 4 Plätze), ein Abgleich gibt seinen Platz nach Fehlschlägen frei | BLOCKER | Aequitas #355 |
| `/api/blocks/push`: Obergrenze 8 bzw. 2 je IP, jede Ablehnung zählt, Kopfprüfung vor dem Abruf | HOCH | Aequitas #356 |
| SSRF über URLs aus der Leitung (Weiterleitung gab Loopback-Antworten zurück) | MITTEL | Aequitas #356 |
| Verklemmung zwischen Datenbank-Pool und `cs.mu`: der Knoten stand bei 20 gleichzeitigen Aufträgen | HOCH | Aequitas #356 |
| Lesefrist auf dem libp2p-Stream; Validator-Abgleich gebündelt; Kopfgrenze im Snapshot 20 000 | MITTEL | Aequitas #356 |
| 183 Workflows nur noch mit `contents: read` | NIEDRIG | Aequitas #357 |
| Chain-ID-Wechsel 1926 → 19260930 gegen die Wiederholung alter Überweisungen, **schlafend** (Stichtag `chainIDWechselAbUnix`). Die Prüfung fand zwei Lücken, beide behoben: eine als RLP-String verpackte Transaktion bekam kein Zeitfenster, und im Bündel wurden Nonces vor der Chain-ID-Prüfung reserviert. | HOCH | Aequitas #360 |
| Replay-Mauer: drei abgewiesene Blöcke derselben Höhe lösten Selbstheilung samt `os.Exit` aus, auch Blöcke, die nur ein böswilliger Erzeuger führt. Jetzt zählt sie je Block und je Knoten, mit höchstens 64 Einträgen. Geheilt wird nur noch mit Beleg, dass das Netz den Block führt: Er kam vom Seed, oder ein anderer Erzeuger hat darauf gebaut. Missbrauch steht im Vier-Knoten-Test. | MITTEL | Aequitas #375 |
| Der Betriebsmodus („test“/„real“) steht in der Bescheinigung (Form v2); der Proof-Server zählt nur den erwarteten Modus, eine einzige Test-Aussage weist die Anfrage ab. | HOCH | proof-server #14, #18; biometric #55 |
| Chain-ID-Wechsel: App, Web (`explorer.js`, `dapp.js`) und Workflows lesen die ID vom Knoten und prüfen sie gegen die Uhr. Offen ist nur noch der Stichtag (Betreiber, Abschnitt 3 Punkt 9). | HOCH | Aequitas #370, App #32 |
| Deploy: volle CI als Pforte, Rücknahme auf das vorige Image | MITTEL | Aequitas #362 |
| GitHub Actions auf Commit-SHAs gepinnt, Dependabot hält sie aktuell | NIEDRIG | Aequitas #364, biometric #49, proof-server #15 |
| `pinningDialer`/`isAllowedPeerURL`: CGNAT (außer vom Betreiber genannt), NAT64 und Sonderbereiche werden nicht mehr gewählt | NIEDRIG | Aequitas #376 |
| Biometrie-Workflows: `inputs.*` über `env` statt direkt in `run:` | NIEDRIG | biometric #56 |
| Coordinator: alle Grenzen je IPv6-/64 (vorher je Einzeladresse: ein /64 umging `/register` ganz); offene Challenges je Netz (10) und je /48 (100), Obergrenze 100 000; X-Forwarded-For auch hinter Provider-NAT-Proxy. **Wirkt erst nach dem Coordinator-Deploy** (`deploy-coordinator-to-box.yml`, von Hand). | MITTEL | biometric #57 |
| Seed liefert nur nachgespielte Blöcke aus: Ein gepushter Block stand vor dem Nachspielen schon in `chain_blocks` und ging über `/api/blocks`, `/api/block`, `/api/blocks/by-hash` und RPC an Nachholende, die ihn dann als vom Seed geführt (`FromSync`) behandelten. | NIEDRIG | Aequitas #380 |
| Ahnen-Leerlauf: Netzfehler zählten als leere Runde, der Zähler galt prozessweit für alle Peers, und ein selbst eingetragener Peer konnte mit leeren Antworten Resync samt Neustart auslösen. Jetzt nur vollständige Antworten, je Peer und je Knoten; heilen nur, wenn ein eingetragener Seed die Historie nicht liefert. | MITTEL | Aequitas #380 |
| Push-Plätze: Wer einen der 8 Plätze belegt, muss mit mindestens 32 KiB/s senden (nach 5 s Anlauf), und kein Push hält einen Platz länger als 10 s; nach dem Rumpf liegt keine Frist mehr auf der Verbindung (sonst brach net/http den Kontext folgender Keep-Alive-Anfragen ab). Vorher hielt nur die ReadTimeout von 30 s einen tropfenden Push, und vier Adressen belegten alle Plätze. Der Abbruch zählt im Flutschutz je /64. | MITTEL | Aequitas #381 |
| App: Abfragen ruhen im Hintergrund und bei unsichtbarem Tab, laufen nie parallel, verlängern die Pause bei Fehlschlägen bis 2 min und zeigen einen Offline-Hinweis (vorher `setInterval` ohne Rücksicht auf Knoten und Akku). | NIEDRIG | Aequitas-App #33 |

**In Prüfung (15.10.2026):**

| Befund | Schwere | PR |
|---|---|---|
| go-ethereum v1.13 → v1.17.8 mit Konsens-Vergleichstest (Golden-Werte aus v1.13). Die Prüfung fand eine Spaltungslücke: Typ 3 im Sidecar-Format v1 lasen nur neue Knoten. Behoben: Typ 3 und 4 werden nie angenommen. Offen ist noch der Beleg, dass das geänderte SSTORE-Verhalten keinen V7-Aufruf trifft. **Alle Validatoren müssen zugleich umstellen.** | MITTEL | Aequitas #359 |

## 2. Im Code offen

| Befund | Schwere | Anmerkung |
|---|---|---|
| StateRoot-Abweichung wird nur geloggt, der Block angenommen. Im DAG mit mehreren Produzenten ergeben Geschwister immer verschiedene Roots, ein hartes Abweisen hielte den Sync an. | MITTEL | tragbar nur bei geschlossener Produzentenliste. Abweisen ist auch im scheinbar eindeutigen Fall (ein Elternblock, Knoten ruht) nicht sicher: der Root enthaelt die angenommenen, noch nicht verblockten Ueberweisungen des Produzenten. Sichtbar je Produzent in `/api/health/combined` → `stateroot_je_produzent` (begrenzt auf 64), siehe `stateroot_beobachtung.go` |
| FromSync-Ausnahmen (Sperre, Finalität) über Seed-Verbindungen ohne TLS | MITTEL | schlafender Stichtag im Code (`fromsync_stichtag.go`); einschalten per PR nach `docs/FROMSYNC_STICHTAG.md` |
| Keine echten Tests mit 4 oder mehr Knoten (Simulationen gibt es) | HOCH | teilweise: `TestVierKnoten_RealDB` (4 Instanzen, eigene Datenbanken, HTTP-Push/-Sync, Leitung, Ausfall und Rueckkehr, Faelschungen). Nicht abgedeckt: Snapshot-Resync ueber Loopback, libp2p, Trusted Seeds; paketweite Zaehler (Push-Plaetze) teilen sich die Instanzen; die Replay-Mauer zaehlt seit #375 je Knoten -- siehe Dateikopf |
| `checkpointVorschau` hielt `dag.mu` (bis über 5 s je Checkpoint-Block). Behoben: die Suche läuft ohne `dag.mu` (`checkpointVorschauOhneDAGSperre`), danach Nachprüfung unter `dag.mu` (Resync, degradiert, Verteilung, Eltern, letzter Checkpoint) – bei jeder Änderung kein Block in diesem Takt. Rest: `replayMu` bleibt bis zur Frist (3 s) gehalten, das Nachspielen fremder Blöcke wartet so lange | NIEDRIG | erst ab dem Checkpoint-Stichtag; Branch `wt-checkpoint` |
| Snapshot und Autoheal vertrauen C1 | MITTEL | erst mit mehreren Unterzeichnern lösbar |
| App: bio/salt gehen an den Knoten, den die App gerade nutzt | MITTEL | durch „nur https“ entschärft |
| Proof-Server: ein `console.warn` je abgewiesener Anfrage | NIEDRIG | |
| Videozerlegung ohne Sandbox; 160 MiB Upload | MITTEL | |
| Deepfake-Schutz schwach | HOCH | Forschung, kein schneller Fix |

## 3. Nur der Betreiber

Diese Punkte kann kein Code-Änderung erledigen. Sie stehen hier in der
Reihenfolge, in der sie gemacht werden sollten.

1. **Umgebung `produktion` scharf machen** (Blocker 17), in allen vier Repos. Der Proof-Server-Deploy vom 14.10. lief ohne Freigabe durch, dort sind also noch keine Prüfer eingetragen.
   - In Settings → Environments → `produktion` Prüfer eintragen.
   - Die SSH-Secrets (`BOX_SSH_KEY`, `CONTABO_SSH_KEY`, `MATCHING_SERVICE_SSH_KEY` …) dort als Environment-Secrets anlegen und die Repo-Secrets löschen.
   - Erst dann wirkt die Sperre aus biometric #46 und proof-server #12.
   - Bei einem privaten Repo im Free-Plan gibt es keine Schutzregeln.
   - Achtung: Der Proof-Server-Deploy nach einem Merge wartet dann auf eine Freigabe.
2. **Alte Logs:**
   - Actions-Läufe löschen, die den ersten Start eines Knotens geloggt haben, denn sie können Schlüssel enthalten.
   - Ist ein noch genutzter Schlüssel betroffen, ihn tauschen.
3. **Vor dem nächsten Vergleichsdienst-Deploy:** Der Workflow prüft jetzt selbst, ob der Coordinator-Schlüssel in `COORDINATOR_PUBLIC_KEYS` steht, und bricht sonst ab. Fehlt er, den Schlüssel in `vergleichsdienst-neuer-c1.yml` eintragen.
4. **Externe Überwachung:**
   - Ein Dienst außerhalb von C1 fragt `/api/status` (Höhe steigt), `/matching/health` und `/health` des Proof-Servers ab und alarmiert.
   - Heute merkt niemand einen Ausfall von außen.
5. **Sicherung außerhalb der Box:** Die Biometrie- und Proof-Datenbanken liegen nur auf C1. Täglich verschlüsselt auf einen zweiten Ort sichern und das Zurückspielen einmal üben (DR-Runbook). Workflow und Skript stehen bereit; offen sind das Secret `BIO_BACKUP_AGE_RECIPIENT` und die Übung (`docs/BIOMETRIE_SICHERUNG.md`).
6. **Mehr Validatoren** (`docs/VALIDATOREN_HINZUFUEGEN.md`):
   - Server und Betreiber kommen von dir.
   - Erst ab drei Validatoren wählt die Leitung bei Ausfall selbst einen neuen Leiter.
   - Erst ab vier bestätigen Checkpoints etwas.
7. **Strenges Nachrechnen** (`nachrechnenStrengAbUnix`): Sobald `/api/wirtschaft/regeln` über längere Zeit null Abweichungen zeigt, den Stichtag per PR setzen, **bevor** ein zweiter unabhängiger Produzent zugelassen wird. Ohne ihn könnte ein fremder Produzent Geld erzeugen.
8. **Checkpoint-Stichtag** (`checkpointTxAbUnix`): erst mit vier oder mehr Validatoren, nach dem Abschnitt „Einschalten“ in `CHECKPOINT_FAHRPLAN.md`.
9. **Chain-ID-Wechsel** (entschieden: neue Chain-ID 19260930):
   - In ethereum-lists/chains prüfen, ob die ID frei ist, und sie dort eintragen.
   - Nach den App- und Web-PRs einen Stichtag wählen und ankündigen: MetaMask-Nutzer müssen das Netz neu einrichten.
   - Den Ablauf beschreibt `docs/CHAIN_ID_WECHSEL.md`.
10. **Sybil über das Gerätegeheimnis** (`ALLOW_DEVICE_SECRET_REGISTER`): Solange die Registrierung ohne Gesicht erlaubt ist, kann ein Mensch mit mehreren Geräten mehrfach registrieren. Entscheiden, ob die Beta das zulässt.
11. **Trusted Setup:** Der Beweisschlüssel hat nur einen Beitrag. Eine Zeremonie mit mehreren unabhängigen Beiträgen vor echten Werten.
12. **Server:**
    - Port 8080 nur hinter dem Proxy öffentlich.
    - Log-Rotation für alle Container (Docker `max-size`).
    - Root-Passwort und SSH nur mit Schlüssel.

## 4. Schnell

- Die Checkpoint-Prüfung läuft nur in Blöcken, die ein Zertifikat tragen. Ihre Suche ist durch Zählgrenzen und eine Frist von 3 s begrenzt.
- `/prove` und `/challenge` haben feste Grenzen. Die App wartet höchstens 20 s, Aufnahmen 120 s.
- Offen bleibt der Durchsatz unter echter Last mit mehreren Validatoren. Das lässt sich erst mit vier Knoten messen.
