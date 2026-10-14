# Beta-Bereitschaft — Prüfung vom 14.10.2026

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

**In Prüfung:** App #31. Er bringt eine Zeitgrenze für jede Anfrage, nur
noch https, keine verschluckten fatalen Fehler, Keystore-Passwörter nicht
mehr in der Prozessliste und keine Injection im Release-Workflow.

## 2. Im Code offen

| Befund | Schwere | Anmerkung |
|---|---|---|
| Der Modus („test“/„real“) steht nicht in der Bescheinigung; der Proof-Server kann eine Test-Bescheinigung nicht ablehnen. | HOCH | betrifft drei Repos (Coordinator, Proof-Server, Kette), eigener PR |
| `ANNAHME_ROLLE=nur_lesend` hält einen neuen Leiter vom Annehmen ab. | BLOCKER für mehr Validatoren | Plan in Arbeit (Dezentral) |
| Seeds nur C1; Peers werden nicht gespeichert; Leitung-URLs zählen nicht als Sync-Peers. | BLOCKER für mehr Validatoren | Plan in Arbeit |
| Tägliche Verteilung und Systemaufträge laufen auf jedem Knoten. | HOCH | Plan in Arbeit |
| FromSync-Ausnahmen in Prüfungen | MITTEL | Plan in Arbeit |
| Keine Tests mit 4 oder mehr Knoten | HOCH | Plan in Arbeit |
| Snapshot und Autoheal vertrauen C1 | MITTEL | erst mit mehreren Unterzeichnern lösbar |
| Deploy ohne Rücknahme auf das vorige Image (Kette); der Deploy prüft weniger als die CI | MITTEL | |
| GitHub Actions nicht auf SHA gepinnt | NIEDRIG | |
| `${{ inputs.* }}` direkt in `run:` (begrenzt durch `choice`) | NIEDRIG | Biometrie-Workflows |
| App: Polling statt Ereignisse, kein Offline-Zustand | NIEDRIG | |
| App: bio/salt gehen an den Knoten, den die App gerade nutzt | MITTEL | durch „nur https“ entschärft |
| `/challenge`: etwa 340 IPv4-Adressen füllen die Obergrenze (Verfügbarkeit) | MITTEL | |
| Proof-Server: ein `console.warn` je abgewiesener Anfrage | NIEDRIG | |
| Videozerlegung ohne Sandbox; 160 MiB Upload | MITTEL | |
| Deepfake-Schutz schwach | HOCH | Forschung, kein schneller Fix |

## 3. Nur der Betreiber

Diese Punkte kann kein Code-Änderung erledigen. Sie stehen hier in der
Reihenfolge, in der sie gemacht werden sollten.

1. **Umgebung `produktion` scharf machen** (Blocker 17), in allen vier Repos:
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
5. **Sicherung außerhalb der Box:** Die Biometrie- und Proof-Datenbanken liegen nur auf C1. Täglich verschlüsselt auf einen zweiten Ort sichern und das Zurückspielen einmal üben (DR-Runbook).
6. **Mehr Validatoren** (`docs/VALIDATOREN_HINZUFUEGEN.md`):
   - Server und Betreiber kommen von dir.
   - Erst ab drei Validatoren wählt die Leitung bei Ausfall selbst einen neuen Leiter.
   - Erst ab vier bestätigen Checkpoints etwas.
7. **Strenges Nachrechnen** (`nachrechnenStrengAbUnix`): Sobald `/api/wirtschaft/regeln` über längere Zeit null Abweichungen zeigt, den Stichtag per PR setzen, **bevor** ein zweiter unabhängiger Produzent zugelassen wird. Ohne ihn könnte ein fremder Produzent Geld erzeugen.
8. **Checkpoint-Stichtag** (`checkpointTxAbUnix`): erst mit vier oder mehr Validatoren, nach dem Abschnitt „Einschalten“ in `CHECKPOINT_FAHRPLAN.md`.
9. **Sybil über das Gerätegeheimnis** (`ALLOW_DEVICE_SECRET_REGISTER`): Solange die Registrierung ohne Gesicht erlaubt ist, kann ein Mensch mit mehreren Geräten mehrfach registrieren. Entscheiden, ob die Beta das zulässt.
10. **Trusted Setup:** Der Beweisschlüssel hat nur einen Beitrag. Eine Zeremonie mit mehreren unabhängigen Beiträgen vor echten Werten.
11. **Server:**
    - Port 8080 nur hinter dem Proxy öffentlich.
    - Log-Rotation für alle Container (Docker `max-size`).
    - Root-Passwort und SSH nur mit Schlüssel.

## 4. Schnell

- Die Checkpoint-Prüfung läuft nur in Blöcken, die ein Zertifikat tragen. Ihre Suche ist durch Zählgrenzen und eine Frist von 3 s begrenzt.
- `/prove` und `/challenge` haben feste Grenzen. Die App wartet höchstens 20 s, Aufnahmen 120 s.
- Offen bleibt der Durchsatz unter echter Last mit mehreren Validatoren. Das lässt sich erst mit vier Knoten messen.
