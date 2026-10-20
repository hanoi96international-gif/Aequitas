# Beta-Bereitschaft — Prüfung vom 14.10.2026, Stand 17.10.2026

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
| Biometrie- und Proof-Datenbanken: nächtlich auf C1 gesichert, auf der Box mit `age` an einen öffentlichen Schlüssel verschlüsselt, nur die `.age`-Datei verlässt die Box (Rückspielprobe, Host-Schlüssel nur ed25519, Image per Digest). Aktiv, sobald `BIO_BACKUP_AGE_RECIPIENT` gesetzt ist. | MITTEL | Aequitas #382 |
| StateRoot-Abweichungen je Produzent sichtbar (`/api/health/combined` → `stateroot_je_produzent`), alle Zähler auf 64 Produzenten begrenzt; volle Maps verdecken keinen neuen Abweichler. Konsens unverändert (Begründung im PR). | MITTEL | Aequitas #383 |
| `checkpointVorschau` sucht ohne `dag.mu`, mit Nachprüfung danach (fail-closed: kein Block in diesem Takt). Rest: `replayMu` bleibt bis 3 s gehalten. | MITTEL | Aequitas #384 |
| Vier-Knoten-Test um Snapshot-Resync, Snapshot-Fälschungen und Trusted Seeds erweitert. Gefunden und behoben: ein Elternblock wurde Spitze, obwohl schon ein Kind auf ihm stand (abweichende Spitzen, zusätzlicher tiefer Elternteil im nächsten Block). Snapshot-Import verlangt immer einen Unterzeichner. | HOCH | Aequitas #385 |
| Resync holt und prüft den Snapshot vor dem Tor (vorher war der Knoten über den ganzen Abruf, bis zu Minuten bei Drosselung, taub); nur der Austausch ist gesperrt, höchstens einer zugleich, wartet laufende Blockdurchgänge ab. Tote Zweige unter der Finalität werden nicht mehr als Spitzen gemeldet. Ein Block gilt erst als „bekannt“, wenn er nachgespielt ist. | MITTEL | Aequitas #386 |
| Biometrie-Coordinator mit #57/#58 auf C1 ausgerollt (16.10.2026); Sandbox vorher im Container als root geprüft. Schlüssel und Einzelbetrieb unverändert. | – | biometric deploy |
| Alte Actions-Logs geprüft: kein Lauf zeigt einen privaten Schlüssel im Klartext; der Ausgabeweg ist seit #353 geschlossen. Kein Schlüsseltausch nötig. | – | – |

**In Prüfung (15.10.2026):**

| Befund | Schwere | PR |
|---|---|---|
| go-ethereum v1.13 → v1.17.8 mit Konsens-Vergleichstest (Golden-Werte aus v1.13). Die Prüfung fand eine Spaltungslücke: Typ 3 im Sidecar-Format v1 lasen nur neue Knoten. Behoben: Typ 3 und 4 werden nie angenommen. Beleg, dass das geänderte SSTORE-Verhalten keinen Aufruf trifft: jeder zugelassene Selektor (`konsens_evm_knotenaufruf_test.go`, Golden-Werte aus v1.13 byte-gleich) und Aufrufdaten mit 1–3 Byte an V7, V8 und BioVerifier, die ohne Selektorprüfung in die EVM gelangen (`konsens_evm_kurzaufruf_test.go`). **Alle Validatoren müssen zugleich umstellen.** | MITTEL | Aequitas #359 |

## 2. Im Code offen

| Befund | Schwere | Anmerkung |
|---|---|---|
| StateRoot-Abweichung wird nur angezeigt, der Block angenommen. | MITTEL | Abweisen ist auch im scheinbar eindeutigen Fall nicht sicher: der Root enthält die angenommenen, noch nicht verblockten Überweisungen des Produzenten. Lösbar nur per Konsensänderung (Root aus angewandten Blöcken, `KONSENS_UND_AUSFUEHRUNG_2026.md`). Sichtbar je Produzent seit #383. |
| FromSync-Ausnahmen (Sperre, Finalität) über Seed-Verbindungen ohne TLS | MITTEL | Stichtag gesetzt: ab Blockzeit 18.10.2026 00:00 UTC prüft jeder Knoten Seed-Blöcke wie gepushte (`fromsync_stichtag.go`, `docs/FROMSYNC_STICHTAG.md`). |
| Tests mit vier Knoten: libp2p nicht abgedeckt | NIEDRIG | `Start()` wählt sich bei öffentlichen Bootstrap-Knoten ein; ein geschlossenes libp2p-Netz bräuchte Bootstrap-Konfiguration im Produktionscode. Paketweite Zähler teilen sich die Instanzen im Prozess. |
| Snapshot und Autoheal vertrauen C1 | MITTEL | erst mit mehreren Unterzeichnern lösbar (zweiter Validator) |
| Deepfake-Schutz schwach | HOCH | Forschung, kein schneller Fix |

Geklärt ohne Code: **App bio/salt** gehen nur an die beim Bau festgelegten
https-Adressen des Projekts (`API_BASE`, `API_FALLBACKS`) -- beides eigene
Knoten; „nur https“ verhindert den Abstieg.

## 3. Nur der Betreiber

Diese Punkte kann kein Code-Änderung erledigen. Sie stehen hier in der
Reihenfolge, in der sie gemacht werden sollten.

1. **Umgebung `produktion` scharf machen** (Blocker 17), in allen vier Repos. Der Proof-Server-Deploy vom 14.10. lief ohne Freigabe durch, dort sind also noch keine Prüfer eingetragen.
   - In Settings → Environments → `produktion` Prüfer eintragen.
   - Die SSH-Secrets (`BOX_SSH_KEY`, `CONTABO_SSH_KEY`, `MATCHING_SERVICE_SSH_KEY` …) dort als Environment-Secrets anlegen und die Repo-Secrets löschen.
   - Erst dann wirkt die Sperre aus biometric #46 und proof-server #12.
   - Bei einem privaten Repo im Free-Plan gibt es keine Schutzregeln.
   - Achtung: Der Proof-Server-Deploy nach einem Merge wartet dann auf eine Freigabe.
2. **Alte Logs:** geprüft am 17.10.2026 -- kein Actions-Lauf zeigt einen privaten Schlüssel im Klartext, kein Tausch nötig. Nicht geprüft: Logs vor dem 18.07.2026, Railway-Logs und `docker logs` auf den Boxen.
3. **Vor dem nächsten Vergleichsdienst-Deploy:** Der Workflow prüft jetzt selbst, ob der Coordinator-Schlüssel in `COORDINATOR_PUBLIC_KEYS` steht, und bricht sonst ab. Fehlt er, den Schlüssel in `vergleichsdienst-neuer-c1.yml` eintragen.
4. **Externe Überwachung:** Es gibt sie: `wache.yml` prüft von GitHub aus Höhe, Gleichstand, Proof-Server, Vergleichsdienst, Coordinator-Quorum und Backup-Alter und schickt bei Rot eine E-Mail; der `wachhund` auf der Box meldet alle 5 min über Telegram/ntfy. Lücke: GitHub führt den 10-min-Zeitplan nur etwa alle 4 h aus. Für Minuten-Alarm einen externen Uptime-Dienst auf `https://aequitas.digital/api/wache` setzen (antwortet bei Rot mit 503).
5. **Sicherung außerhalb der Box:** Die Biometrie- und Proof-Datenbanken liegen nur auf C1. Täglich verschlüsselt auf einen zweiten Ort sichern und das Zurückspielen einmal üben (DR-Runbook). Workflow und Skript sind gemergt (#382); offen sind das Secret `BIO_BACKUP_AGE_RECIPIENT` und die Übung (`docs/BIOMETRIE_SICHERUNG.md`).
6. **Mehr Validatoren** (`docs/VALIDATOREN_HINZUFUEGEN.md`):
   - Server und Betreiber kommen von dir.
   - Erst ab drei Validatoren wählt die Leitung bei Ausfall selbst einen neuen Leiter.
   - Erst ab vier bestätigen Checkpoints etwas.
7. **Strenges Nachrechnen** (`nachrechnenStrengAbUnix`): Sobald `/api/wirtschaft/regeln` über längere Zeit null Abweichungen zeigt, den Stichtag per PR setzen, **bevor** ein zweiter unabhängiger Produzent zugelassen wird. Ohne ihn könnte ein fremder Produzent Geld erzeugen.
8. **FromSync-Stichtag** (`fromSyncStrengAbUnix`): gesetzt auf 18.10.2026 00:00 UTC (vorgezogen) (17.10.2026, auf deine Anweisung). Für dich bleibt: in der ersten Stunde nach `Ab` die Logs von C1 auf `[FINALITY] ✗` / `[SLASHING] ✗` ansehen, jeden neuen Validator vor dem Start mit diesem Code betreiben (spätestens 17.10.2026 23:58 UTC), und C1 keinen Seed eintragen, ohne vorher Finalität und StateRoot des Seeds zu vergleichen (sonst kann das Sync-Tor C1 nach einem Neustart anhalten, F4). Rücknahme nur über `fromSyncStrengBisUnix`: `docs/FROMSYNC_STICHTAG.md`.
9. **Checkpoint-Stichtag** (`checkpointTxAbUnix`): erst mit vier oder mehr Validatoren, nach dem Abschnitt „Einschalten“ in `CHECKPOINT_FAHRPLAN.md`.
10. **Chain-ID-Wechsel** (entschieden: neue Chain-ID 19260930):
   - ID in ethereum-lists/chains eintragen: Die Datei liegt fertig unter `deploy/ethereum-lists/eip155-19260930.json` (Name, RPC `https://aequitas.digital/rpc`, AEQ mit 18 Stellen). Als PR an `ethereum-lists/chains` unter `_data/chains/` einreichen, sobald der Stichtag feststeht -- deren Prüfung erwartet, dass der RPC die ID schon meldet. Am 16.10.2026 war 19260930 frei; vorher erneut prüfen.
   - Stichtag gesetzt (17.10.2026): **18.10.2026, 00:00 UTC** (vorgezogen, ohne 7-Tage-Vorlauf), App 1.11.0 dazu veröffentlicht, Hinweis auf der Startseite. Für dich bleibt: den Wechsel zusätzlich in deinen Kanälen ankündigen (MetaMask-Nutzer legen das Netz neu an), nach dem Stichtag den ethereum-lists-Eintrag einreichen.
   - Den Ablauf beschreibt `docs/CHAIN_ID_WECHSEL.md`.
11. **Sybil über das Gerätegeheimnis** (`ALLOW_DEVICE_SECRET_REGISTER`): Solange die Registrierung ohne Gesicht erlaubt ist, kann ein Mensch mit mehreren Geräten mehrfach registrieren. Entscheiden, ob die Beta das zulässt.
12. **Trusted Setup:** Der Beweisschlüssel hat nur einen Beitrag. Eine Zeremonie mit mehreren unabhängigen Beiträgen vor echten Werten.
13. **Server:** Workflow `server-haerten.yml` (Skript `deploy/server-haerten.sh`, Wächter `scripts/lint_server_haerten.py`). Erst mit `modus=pruefen` (Voreinstellung, nur lesen) den Bericht ansehen, dann mit `modus=anwenden` freigeben.
    - Root-Passwort und SSH nur mit Schlüssel: `anwenden` setzt ein sshd-Drop-in (kein Passwort-, kein Tastatur-Login, root nur mit Schlüssel). sshd wird nur neu geladen, und erst wenn `authorized_keys` einen Schlüssel enthält und `sshd -t` besteht. Das root-Passwort wird erst gesperrt, wenn ein neuer Schlüssel-Login danach klappt und sshd nur noch `publickey` anbietet. Danach nimmt auch die Konsole des Anbieters kein root-Passwort mehr; Notzugang über dessen Rettungssystem.
    - **Erledigt am 17.10.2026 auf C1** (`anwenden`, Lauf 39032183542): SSH nur mit Schlüssel, root-Passwort gesperrt.
    - Log-Rotation für alle Container (Docker `max-size`): `anwenden` hat `/etc/docker/daemon.json` um `max-size 50m`, `max-file 5` ergänzt; Docker wurde **nicht** neu gestartet. Die Container rotieren deshalb über ihre eigenen Optionen:
      - `aequitas-node`, `aequitas-postgres`: schon über `deploy/validator/docker-compose.yml`.
      - `proof-server`: über `proof-deploy.sh` ab jedem Deploy (proof-server #20, ausgerollt).
      - `aequitas-caddy`: einmal `neuer-server-caddy.yml` mit `neu_anlegen=true` starten. Legt Caddy mit gleichen Volumes neu an (Zertifikate bleiben), wenige Sekunden ohne HTTPS, prüft danach alle öffentlichen Wege.
      - Danach `server-haerten.yml` (`pruefen`): Der Bericht darf keinen Container ohne `max-size` mehr nennen.
    - Port 8080 nur hinter dem Proxy öffentlich: **bleibt deine Entscheidung.** Der Workflow schließt ihn nicht, er berichtet nur. Peers sprechen den Knoten direkt über `http://IP:8080` an (`SELF_URL`, `PRIMARY_NODE_URLS`; `deploy/neuer-server.sh` öffnet 8080 in ufw). Ihn zu schließen ist ein Netzwechsel: erst alle Peers auf die Proxy-Adresse umstellen. Von Docker veröffentlichte Ports umgehen ufw.

## 4. Schnell

- Die Checkpoint-Prüfung läuft nur in Blöcken, die ein Zertifikat tragen. Ihre Suche ist durch Zählgrenzen und eine Frist von 3 s begrenzt.
- `/prove` und `/challenge` haben feste Grenzen. Die App wartet höchstens 20 s, Aufnahmen 120 s.
- Offen bleibt der Durchsatz unter echter Last mit mehreren Validatoren. Das lässt sich erst mit vier Knoten messen.
