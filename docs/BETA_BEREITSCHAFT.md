# Beta-Bereitschaft — Prüfung vom 14.10.2026, Stand 28.10.2026

Geprüft wurden alle vier Repos: Kette, App, Biometrie und Proof-Server. Die
Fragen waren: funktioniert es, ist es sicher, ist es schnell, und ist der Weg
zu mehr Validatoren frei? Dieses Dokument hält fest, was **erledigt** ist,
was **im Code offen** ist und was **nur der Betreiber** tun kann.

Massstab ist AGENTS.md, „Sicherheit — Launch-Massstab“. Jede Änderung unten
hatte eine getrennte Sicherheitsprüfung; das Ergebnis steht als Kommentar im
jeweiligen PR.

## 0. Live-Stand 28.10.2026

Gelesen mit `pruefstand-live.yml`, `wache.yml` und `betrieb-pruefen-c1.yml`
(nur lesend), alle drei grün. Es läuft **ein** Server: C1 (netcup,
188.172.229.121, `proof1.aequitas.digital`). Der alte Contabo1 (seit 24.09.)
und C2 (seit 03.10.) sind abgeschaltet und aus Code, Seeds, Workflows und
Tests entfernt (Aequitas #419–#422, biometric #87, proof-server #24);
`lint_alte_hosts.py` hält ihre IPs draußen.

| Bereich | Stand |
|---|---|
| Kette | Höhe ~571.900 (28.10.), Chain-ID 19260930, keine Kontostand-Abweichung, nur C1 nimmt an, keine Fehler im Knoten-Log |
| Registrierung | Coordinator proof1 ok, **Quorum 1 im Einzelbetrieb** (höchstens 25 Registrierungen am Tag), Vergleichsdienst ok, Proof-Server `required`, Herkunftspflicht an `/api/register` |
| App | Neuestes Release **v1.13.0** (25.10., App #46): „Knoten“ → „Bestätigen“ trägt einen Server auch ins Validator-Register auf der Kette ein. Seit v1.12.9 keine proof2-Adresse mehr in der APK, die Bauvariablen `EXPO_PUBLIC_*_FALLBACKS` sind geleert. Die App weist abgeschaltete Server in jeder Schreibweise selbst ab (`abgeschaltet()` in `lib/config.ts`). |
| Sicherung | Umgebung `sicherung` eingerichtet (25.10.): Die Ledger-Sicherung läuft nachts ohne Freigabe mit dem beschränkten Schlüssel (`deploy/sicherung/`), zuletzt erfolgreich am 27.10. **Die Biometrie-Sicherung scheitert jede Nacht**: auf C1 ist kein age-Empfänger hinterlegt, also wird nichts gesichert (Punkt 4). |
| RPC-Ausnahme | `AEQUITAS_RPC_RATE_LIMIT_FREI` auf C1 geleert (galt der neu vergebenen C2-IP) |
| Produzenten | `AUTHORIZED_VALIDATORS` auf C1 = nur C1; `produzenten_geschlossen=true` (live; `pruefstand-live.yml` und `wache.yml` werden rot, wenn der Wert nicht genau `true` ist) |
| Register-Bindung | C1 im Validator-Register auf der Kette gebunden (25.10., App 1.13.0, `knoten-binden.yml`); `wache.yml` am 28.10.: „jeder Erzeuger im Validator-Register gebunden“. **Seit 28.10.2026 10:00 UTC erzeugen nur gebundene Server** (`registerLeserAb`, `erzeugerSchnittAb`, #442, auf C1 seit 06:46 UTC). Fällt die Bindung von C1 weg, steht die Kette -- die Wache prüft es bei jedem Lauf. |
| Railway | Abgeschaltet (antwortet „Application not found“). Reste in Workflows, Wache und Biometrie-Skripten entfernt (Aequitas #437, biometric #91); `lint_alte_hosts.py` lässt `*.up.railway.app` in ausführbaren Zeilen nicht mehr zu. |
| Root-Zugang C1 | Neuer Root-Schlüssel nur als Umgebungs-Secret in `produktion`; der alte (lag als Repository-Secret im öffentlichen Repo) ist am 25.10. auf C1 entfernt, ebenso der unbekannte Schlüssel `github-actions-deploy-contabo1` (`c1-root-schluessel-tauschen.yml`, #433/#436). |
| Coordinator-Schlüssel | Für den Notfall „Signierschlüssel entwendet“ gibt es wieder einen Weg auf C1: `coordinator-schluessel-wechseln.yml` (biometric #92), mit Rücknahme, wenn ein Dienst den neuen Schlüssel nicht annimmt. |
| Caddy | Erledigt 25.10. (`neuer-server-caddy.yml`, Lauf 40199773044): Log-Rotation war schon aktiv, die Konfiguration kommt aus dem Repo, alle sechs öffentlichen Wege antworten 200. |
| Deploy | Seit #441 startet `deploy-c1.yml` nur bei Änderungen am Knoten-Image (Positivliste, geprüft von `lint_deploy_pfade.py`); Änderungen an Tests, Skripten und Doku lösen keinen Deploy mehr aus. |
| Freigaben | `produktion` (alle vier Repos) und `app-signatur` haben seit dem 24.10. Pflicht-Prüfer; belegt an je einem wartenden Lauf in Aequitas und Biometrie. App und Proof-Server zeigen es beim nächsten Bau bzw. Deploy. |

**Was die Beta noch braucht** (alles Betreiber, Reihenfolge = Vorschlag):

1. **Impressum und Datenschutz** (Blocker 3): `/impressum` und
   `/datenschutz` antworten 404, alle sieben `LEGAL_*` fehlen.
   `rechtstexte-setzen.yml` mit den sieben Angaben starten (erlaubte
   Zeichen: Buchstaben, Ziffern, Leerzeichen, `. , - / @ + & ( ) : _`).
2. ~~C1 im Validator-Register auf der Kette eintragen~~ **erledigt**
   (25.10.2026); „nur gebundene Server erzeugen“ gilt seit 28.10.2026
   10:00 UTC (#442).
3. **Umgebung `produktion` scharf machen** (3.1).
4. **Biometrie-Sicherung freischalten** (3.5): Die Umgebung `sicherung` und
   `SICHERUNG_SSH_KEY` sind eingerichtet, es fehlt nur der age-Empfänger.
   Auf deinem Rechner (nicht am Server, nicht im Repo)
   `age-keygen -o bio-backup.key` ausführen und die Datei offline verwahren;
   dann `sicherung-einrichten.yml` mit `bio_empfaenger=age1…` (die
   öffentliche Zeile, die `age-keygen` ausgibt) starten und freigeben
   (`docs/BIOMETRIE_SICHERUNG.md`). Bis dahin scheitert
   `backup-biometrie-verschluesselt.yml` jede Nacht und sichert nichts.
5. **Registrierung mit frischem Wallet** (Blocker 2): Beleg `total_humans` +1
   und `gallery_test` +1.
6. **Zwei-Personen-Test** nach `docs/DOPPELREGISTRIERUNG_TEST.md` (Blocker 4).
7. **DSGVO-Entscheidung** (Blocker 5); bis dahin bleibt der Vergleichsdienst
   im Testmodus (`real_data_unlocked=false`), die Altersschätzung beobachtet
   nur (`modell_bereit=false`).
8. ~~App-Bauvariablen leeren~~ **erledigt** (24.10.2026): Die Variablen
   sind leer, v1.12.9 ist ohne proof2 gebaut und ausgeliefert.
9. ~~Railway-Coordinator abschalten~~ **erledigt**: abgeschaltet, Reste
   entfernt (Aequitas #437, biometric #91).
10. **Secrets aufräumen**: `Railway` und `CONTABO2_SSH_KEY` löschen (beide
    werden von keinem Workflow mehr gelesen).
11. ~~Caddy-Log-Rotation~~ **erledigt** (25.10.2026).
12. **Überwachung von außen** (Minuten statt Stunden): GitHub führt
    `wache.yml` statt alle 10 Minuten nur alle 2–5 Stunden aus (25.10.: zuletzt
    09:54 UTC). Bei UptimeRobot (kostenlos) zwei HTTP(S)-Monitore mit 5 min
    Abstand und E-Mail-Alarm anlegen:
    `https://aequitas.digital/api/wache` (200 = alles da, 503 = der Rumpf
    nennt, was fehlt) und `https://proof1.aequitas.digital/health`. Kein
    Geheimnis nötig, beide Adressen sind öffentlich.

Ein zweiter Validator ist für die Beta nicht nötig, für den Launch schon
(Punkt 3.6). Bis dahin trägt C1 allein: Snapshot und Autoheal vertrauen C1,
der Einzelbetrieb begrenzt Registrierungen auf 25 am Tag.

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
| Biometrie- und Proof-Datenbanken: nächtlich auf C1 gesichert, auf der Box mit `age` an einen öffentlichen Schlüssel verschlüsselt, nur die `.age`-Datei verlässt die Box (Rückspielprobe, Host-Schlüssel nur ed25519, Image per Digest). Aktiv, sobald der age-Empfänger auf C1 hinterlegt ist (`sicherung-einrichten.yml`). | MITTEL | Aequitas #382 |
| StateRoot-Abweichungen je Produzent sichtbar (`/api/health/combined` → `stateroot_je_produzent`), alle Zähler auf 64 Produzenten begrenzt; volle Maps verdecken keinen neuen Abweichler. Konsens unverändert (Begründung im PR). | MITTEL | Aequitas #383 |
| `checkpointVorschau` sucht ohne `dag.mu`, mit Nachprüfung danach (fail-closed: kein Block in diesem Takt). Rest: `replayMu` bleibt bis 3 s gehalten. | MITTEL | Aequitas #384 |
| Vier-Knoten-Test um Snapshot-Resync, Snapshot-Fälschungen und Trusted Seeds erweitert. Gefunden und behoben: ein Elternblock wurde Spitze, obwohl schon ein Kind auf ihm stand (abweichende Spitzen, zusätzlicher tiefer Elternteil im nächsten Block). Snapshot-Import verlangt immer einen Unterzeichner. | HOCH | Aequitas #385 |
| Resync holt und prüft den Snapshot vor dem Tor (vorher war der Knoten über den ganzen Abruf, bis zu Minuten bei Drosselung, taub); nur der Austausch ist gesperrt, höchstens einer zugleich, wartet laufende Blockdurchgänge ab. Tote Zweige unter der Finalität werden nicht mehr als Spitzen gemeldet. Ein Block gilt erst als „bekannt“, wenn er nachgespielt ist. | MITTEL | Aequitas #386 |
| Biometrie-Coordinator mit #57/#58 auf C1 ausgerollt (16.10.2026); Sandbox vorher im Container als root geprüft. Schlüssel und Einzelbetrieb unverändert. | – | biometric deploy |
| Alte Actions-Logs geprüft: kein Lauf zeigt einen privaten Schlüssel im Klartext; der Ausgabeweg ist seit #353 geschlossen. Kein Schlüsseltausch nötig. | – | – |
| `zumLeiter`: POST-Rumpf höchstens 64 KiB mit eigener Lese-Frist (10 s), angekündigt zu Großes ohne Lesen abgewiesen, eine an der Server-Frist gescheiterte Verbindung wird geschlossen statt weiterverwendet. Dazu ist der WAL-Gruppen-Commit-Test unabhängig vom fsync-Tempo. | MITTEL | Aequitas #427 (löst #414 ab) |
| App: abgeschaltete Server (proof2, alte IPs) zählen in keiner Schreibweise, auch nicht aus Bauvariablen | HOCH | Aequitas-App #45 |
| go-ethereum v1.13 → v1.17.8 mit Konsens-Vergleichstest (Golden-Werte aus v1.13). Die Prüfung fand eine Spaltungslücke: Typ 3 im Sidecar-Format v1 lasen nur neue Knoten. Behoben: Typ 3 und 4 werden nie angenommen. Beleg, dass das geänderte SSTORE-Verhalten keinen Aufruf trifft: jeder zugelassene Selektor (`konsens_evm_knotenaufruf_test.go`, Golden-Werte aus v1.13 byte-gleich) und Aufrufdaten mit 1–3 Byte an V7, V8 und BioVerifier, die ohne Selektorprüfung in die EVM gelangen (`konsens_evm_kurzaufruf_test.go`). Ausgerollt mit den Deploys seit dem 15.10.2026; jeder neue Validator startet mit diesem Stand. | MITTEL | Aequitas #359 |

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
5. **Sicherung außerhalb der Box:** Die Biometrie- und Proof-Datenbanken liegen nur auf C1. Täglich verschlüsselt auf einen zweiten Ort sichern und das Zurückspielen einmal üben (DR-Runbook). Workflow und Skript sind gemergt (#382); offen sind der öffentliche age-Schlüssel (seit 24.10.2026 über `sicherung-einrichten.yml` fest auf C1, nicht mehr als Secret) und die Übung (`docs/BIOMETRIE_SICHERUNG.md`). Bis er hinterlegt ist, gibt es **keine** Sicherung der Biometrie außer Haus.
6a. **C1 im Validator-Register binden** (seit dem Stichtag 18.10.2026 12:00 UTC möglich, einmal): Die Bindung **beim Knoten** ist seit dem 21.10.2026 erledigt (App, `knoten-binden.yml`, Lauf 39659933383). Der Eintrag **auf der Kette** fehlt noch: Die App konnte ihn bis 1.12.9 nicht machen, nur die Browserseite `/node-binding`. Ab App 1.13.0 genügt `knoten-binden.yml` starten und in der App unter „Knoten“ einmal bestätigen; die Wallet fragt dann zweimal. Ohne Bindung schlösse der spätere Erzeugerschnitt C1 aus, und ein zweiter Validator kann sich nur neben einem gebundenen C1 sauber einreihen. Die Wache zeigt seit 20.10.2026, ob noch ein Erzeuger ohne Bindung ist (Hinweis „Erzeuger OHNE Bindung“, `erzeuger_ohne_bindung` in `/api/status`).
6. **Mehr Validatoren** (`docs/VALIDATOREN_HINZUFUEGEN.md`):
   - Server und Betreiber kommen von dir.
   - Erst ab drei Validatoren wählt die Leitung bei Ausfall selbst einen neuen Leiter.
   - Erst ab vier bestätigen Checkpoints etwas.
7. **Strenges Nachrechnen** (`nachrechnenStrengAbUnix`): Sobald `/api/wirtschaft/regeln` über längere Zeit null Abweichungen zeigt, den Stichtag per PR setzen, **bevor** ein zweiter unabhängiger Produzent zugelassen wird. Ohne ihn könnte ein fremder Produzent Geld erzeugen.
8. **FromSync-Stichtag** (`fromSyncStrengAbUnix`): gesetzt auf 18.10.2026 00:00 UTC (vorgezogen) (17.10.2026, auf deine Anweisung). Für dich bleibt: in der ersten Stunde nach `Ab` die Logs von C1 auf `[FINALITY] ✗` / `[SLASHING] ✗` ansehen, jeden neuen Validator vor dem Start mit diesem Code betreiben (spätestens 17.10.2026 23:58 UTC), und C1 keinen Seed eintragen, ohne vorher Finalität und StateRoot des Seeds zu vergleichen (sonst kann das Sync-Tor C1 nach einem Neustart anhalten, F4). Rücknahme nur über `fromSyncStrengBisUnix`: `docs/FROMSYNC_STICHTAG.md`.
9. **Checkpoint-Stichtag** (`checkpointTxAbUnix`): erst mit vier oder mehr Validatoren, nach dem Abschnitt „Einschalten“ in `CHECKPOINT_FAHRPLAN.md`.
10. **Chain-ID-Wechsel** (entschieden: neue Chain-ID 19260930):
   - ID in ethereum-lists/chains eintragen: Die Datei liegt fertig unter `deploy/ethereum-lists/eip155-19260930.json` (Name, RPC `https://aequitas.digital/rpc`, AEQ mit 18 Stellen). Als PR an `ethereum-lists/chains` unter `_data/chains/` einreichen; Schritte, Titel und Text: `docs/ETHEREUM_LISTS_PR.md`. Am 16.10.2026 war 19260930 frei; vorher erneut prüfen.
   - Stichtag gesetzt (17.10.2026): **18.10.2026, 00:00 UTC** (vorgezogen, ohne 7-Tage-Vorlauf), App 1.11.0 dazu veröffentlicht, Hinweis auf der Startseite. Für dich bleibt: den Wechsel zusätzlich in deinen Kanälen ankündigen (MetaMask-Nutzer legen das Netz neu an), nach dem Stichtag den ethereum-lists-Eintrag einreichen.
   - Den Ablauf beschreibt `docs/CHAIN_ID_WECHSEL.md`.
11. **Sybil über das Gerätegeheimnis** (`ALLOW_DEVICE_SECRET_REGISTER`): **im Code geschlossen, keine Entscheidung mehr nötig** (geprüft 20.10.2026). Eine Registrierung ohne Gesicht kommt nicht mehr auf die Kette, auch wenn eine App mit dem Schalter gebaut ist:
    - Der Knoten nimmt nur Beweise an, die aus einem `/api/prove` dieses Knotens für dieselbe Wallet stammen (`prove_provenance.go`, `REQUIRE_PROVE_PROVENANCE` ist ohne Angabe an).
    - Der Proof-Server verlangt im Betrieb eine Bescheinigung des Coordinators (`BIO_ATTESTATION_MODE=required`; ohne sie startet er nicht, `produktionsPruefung`). Der Gerätegeheimnis-Weg liefert keine und bekommt 403.
    - `wache.yml` (`.github/wache_pruefung.py`) wird rot, sobald der Proof-Server einen anderen Modus meldet.
    - Seit 20.10.2026 meldet der Knoten das Tor in `/api/health/combined` (`prove_herkunft.verlangt`), und die Wache wird rot, wenn es nicht `true` ist (vorher stand es nur in der Umgebung des Containers). Missbrauchsfälle in `scripts/lint_wache_pruefung.py`, im CI.
    - Für dich bleibt nur: `REQUIRE_PROVE_PROVENANCE=false` und einen anderen Modus als `required` nie setzen.
12. **Trusted Setup:** erledigt am 21.10.2026 (Zeremonie v4, `aequitas-proof-server/zeremonie/`). Phase 1 ist die öffentliche Perpetual Powers of Tau (80 Beiträge, kryptographisch geprüft), Phase 2 hat zwei Beiträge (flüchtiger Claude-Container, flüchtiger GitHub-Runner) und einen Beacon aus der vorher festgelegten drand-Runde 33260812. Wirksam mit dem Neustart bei null. Weitere Beiträge unabhängiger Menschen (`zeremonie/beitragen.sh`) vor einem späteren Schlüsselwechsel sind willkommen.
13. **Server:** Workflow `server-haerten.yml` (Skript `deploy/server-haerten.sh`, Wächter `scripts/lint_server_haerten.py`). Erst mit `modus=pruefen` (Voreinstellung, nur lesen) den Bericht ansehen, dann mit `modus=anwenden` freigeben.
    - Root-Passwort und SSH nur mit Schlüssel: `anwenden` setzt ein sshd-Drop-in (kein Passwort-, kein Tastatur-Login, root nur mit Schlüssel). sshd wird nur neu geladen, und erst wenn `authorized_keys` einen Schlüssel enthält und `sshd -t` besteht. Das root-Passwort wird erst gesperrt, wenn ein neuer Schlüssel-Login danach klappt und sshd nur noch `publickey` anbietet. Danach nimmt auch die Konsole des Anbieters kein root-Passwort mehr; Notzugang über dessen Rettungssystem.
    - **Erledigt am 17.10.2026 auf C1** (`anwenden`, Lauf 39032183542): SSH nur mit Schlüssel, root-Passwort gesperrt.
    - Log-Rotation für alle Container (Docker `max-size`): `anwenden` hat `/etc/docker/daemon.json` um `max-size 50m`, `max-file 5` ergänzt; Docker wurde **nicht** neu gestartet. Die Container rotieren deshalb über ihre eigenen Optionen:
      - `aequitas-node`, `aequitas-postgres`: schon über `deploy/validator/docker-compose.yml`.
      - `proof-server`: über `proof-deploy.sh` ab jedem Deploy (proof-server #20, ausgerollt).
      - `aequitas-caddy`: erledigt (25.10.2026, `neuer-server-caddy.yml`): rotiert seine Logs.
      - Danach `server-haerten.yml` (`pruefen`): Der Bericht darf keinen Container ohne `max-size` mehr nennen.
    - Port 8080 nur hinter dem Proxy öffentlich: **bleibt deine Entscheidung.** Der Workflow schließt ihn nicht, er berichtet nur. Peers sprechen den Knoten direkt über `http://IP:8080` an (`SELF_URL`, `PRIMARY_NODE_URLS`; `deploy/neuer-server.sh` öffnet 8080 in ufw). Ihn zu schließen ist ein Netzwechsel: erst alle Peers auf die Proxy-Adresse umstellen. Von Docker veröffentlichte Ports umgehen ufw.

## 4. Schnell

- Die Checkpoint-Prüfung läuft nur in Blöcken, die ein Zertifikat tragen. Ihre Suche ist durch Zählgrenzen und eine Frist von 3 s begrenzt.
- `/prove` und `/challenge` haben feste Grenzen. Die App wartet höchstens 20 s, Aufnahmen 120 s.
- Offen bleibt der Durchsatz unter echter Last mit mehreren Validatoren. Das lässt sich erst mit vier Knoten messen.
