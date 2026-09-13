# Launch-Checkliste

**Stand 13.09.2026, 23:00.** Wache: GRÜN — prüft seit heute auch, dass das Tor zu ist (`required`, Quorum ≥ 2) und beide Coordinatoren antworten. App v1.7.0 veröffentlicht und ausgeliefert.

> **Launch-Linie seit dem 13.09. nachmittags (deine Entscheidung: „Phase 1 muss umgesetzt werden — 1 Mensch,
> 1 Registrierung"): Phase 1 = die Gesichtsprüfung ist das Tor.** `BIO_ATTESTATION_MODE=required` wieder auf beiden
> Proof-Servern (15:13, Quorum 2), die Kette nimmt an `/api/register` nur Nullifier aus einem `/api/prove` desselben
> Knotens (`prove_provenance.go`, Voreinstellung an). Die Phase-0-Linie vom Morgen (Gerätegeheimnis, App
> app-v0.3.1-phase0, `optional`) ist damit zurückgenommen; **die Phase-0-APK darf nicht ausgeliefert werden** — sie
> würde mit `required` jede Registrierung abgewiesen bekommen. Die Website (12 Sprachen, Landing + Explorer +
> Registrierungsseite) nennt jetzt Tor **und** Grenzen: Altkonten ohne Gesichts-Template, unkalibrierte Schwelle,
> Kopfdreh-Lebendigkeit, MPC im Schatten. Jeder Punkt hat ein „fertig heißt" — etwas, das man
nachmessen kann. Was nicht messbar ist, steht nicht drin. Wer hier „✅" setzt,
hat gemessen, nicht geglaubt.

**Launch heißt:** Fremde registrieren sich in der App, bekommen AEQ, nutzen es;
„ein Mensch = ein Konto" hält; wer will, betreibt einen Validator nach
Anleitung; die Betreiber erfahren, wenn etwas kaputtgeht.

## Blocker (ohne die kein Launch)

| # | Punkt | Fertig heißt | Stand | Wer |
|---|---|---|---|---|
| 1 | **Beide Validatoren einig über jeden Kontostand** | `divergenz.abweichend=false` auf beiden Boxen nach Volllast, `uebersprungene_ueberweisungen=0` | ✅ 13.09. 01:40: Resync C1←C2, danach 4 min Volllast (14.000 Annahmen/s, Kette 6.900/s): **0 übersprungen, 0 Divergenz**, Wache grün | — |
| 2 | **Registrierung funktioniert** | App (Gesichtsprüfung) → Coordinator (Quorum 2) → `/api/prove` → Proof-Server `required` → `register_human` in einem Block; Wache grün | ⚠ Kette, Proof-Server (`/api/prove` ohne Bescheinigung → 403, gemessen) und beide Coordinatoren stehen; **die Website liefert seit 13.09. 22:50 app-v1.7.0** (beide Boxen, 215.027.338 Bytes, sha256 `296ceb7…`, Release-Schlüssel `473e9d…`; Coordinator proof1 → proof2). Seit dem 25.08. hat genau **eine** Person den Weg durchlaufen (`gallery_test: 1`) — mit Quorum 2 noch niemand nachweislich | **du:** eine Registrierung mit einem frischen Wallet durchlaufen; Beleg: `total_humans` +1, `gallery_test` +1 auf proof1 **und** proof2 |
| 3 | **Impressum & Datenschutzerklärung** | `/impressum` und `/datenschutz` antworten 200 | ❌ beide 404 — alle sieben `LEGAL_*`-Angaben fehlen (`/api/legal-status`) | **du:** Werte nach `docs/RECHTSTEXTE_FREISCHALTEN.md` setzen, ich starte die Knoten nacheinander neu |
| 4 | **Ein Mensch = ein Konto** | Dieselbe Person, zweites Gerät → `duplicate`; andere Person → durch. Schwelle kalibriert. Altkonten mit Gesicht nachgezogen | ⚠ Tor scharf (`required` seit 13.09. 15:13, Quorum 2, Herkunftspflicht an `/api/register`). **Nachzieh-Weg gebaut und live** (`POST /nachziehen` auf proof1 + proof2, Wallet-Signatur + Ketten-Abgleich + Quorum, keine Prägung; App-Seite in v1.7.0 committed, Identity-Tab „Gesicht nachziehen"). Offen: Schwelle nie mit echten Menschen kalibriert; **die 18 bestehenden Menschen haben kein Gesichts-Template**, bis sie nachziehen (auf der Website benannt); `SERVICE_MODE=test` (Wechsel auf `real` erst mit Galerie-Übernahme, `wuerde_realmodus_abweisen`) | **du + eine zweite Person** vor der Kamera nach `docs/DOPPELREGISTRIERUNG_TEST.md` (Schritte 1–7); danach die 18 einmal durch den Nachzieh-Weg (App v1.7.0) |
| 5 | **DSGVO Phase 2** (Phase 1) | `ALLOW_REAL_BIOMETRIC_DATA=true` + `LEGAL_SIGNOFF_DATE` gesetzt, Drittland (Railway in `sfo`) entschieden | ❌ Unterlagen liegen in `aequitas-biometric-beta/docs/dsgvo/`; Entscheidung offen | **du / Jurist** |
| 6 | **Betreiber merkt, wenn es brennt** | Alarm binnen Minuten | ⚠ `wache.yml` läuft bei GitHub nur alle paar Stunden (13.09.: 01:11, 06:17, 12:05). Deshalb neu: `https://aequitas.digital/api/wache` — 200/503 mit Befunden | **du:** einen freien Uptime-Monitor (z. B. UptimeRobot) auf `https://aequitas.digital/api/wache` und `http://194.163.188.71:8080/api/wache` alle 5 min, E-Mail bei 503 |

## Wichtig, aber kein Blocker

| # | Punkt | Fertig heißt | Stand | Wer |
|---|---|---|---|---|
| 7 | Neuer Validator nach Anleitung | Fremde Box folgt der Kette und produziert nach Bestätigung | ✅ Weg dreimal auf einem Runner geprüft (Snapshot 21 s, an der Spitze, 0 Fremdblöcke, 145 MB). Echter Beitritt mit registriertem Wallet fehlt | **du:** ein VPS, `docs/VALIDATOR_EINRICHTEN.md` |
| 8 | Backup | täglich grün, Restore geprüft | ✅ seit 12.09. (Zustand ohne Blöcke, 31–61 MB) | — |
| 9 | Coordinator hängt an Railway (USA) | App zeigt auf einen Contabo-Coordinator; App-Repo auf der Phase-1-Linie; v1.7.0 auf der Website | ✅ 13.09. 22:41: **app-v1.7.0 veröffentlicht** (Release, Notiz nennt den echten Signierer) und auf beiden Boxen ausgeliefert (`host-apk-locally.yml`); app-v0.3.1-phase0 als *zurückgezogen* markiert (Pre-Release + Hinweis). Railway wird nur noch von v1.6.0-Installationen genutzt — Wache meldet ihn weiter, bis die letzte davon aktualisiert hat | — |
| 10 | Node-Guide in allen 12 Sprachen | keine Railway-Anleitung mehr erreichbar | ✅ alle zwölf am 12.09. neu erzeugt (Docker Compose), 0× „Railway" | — |
| 11 | Lasttest-Konten | ≥ 300 Paare je Box zahlungsfähig | ❌ 594/661 bzw. 375/662 Konten unter 0,001 AEQ | **du:** `loadtest-widen-senders.yml confirm=true` |
| 12 | App im Play Store | — | ⏸ bewusst offen bis Punkt 5 | du |
| 13 | Dritter Betreiber | Quorum aus drei Haushalten | ⏸ nach Punkt 4 | du |
| 14 | Lebendigkeit gegen Deepfakes | Blitz + Kopfdrehung + Puls als Score, Herkunft als Risiko, gestaffelter Zuschuss (Klassen grün/gelb/rot), Schattenmodus zuerst | ⚠ **WP 1 live im Schatten** (13.09.: Score L, Risiko R, Klasse je Registrierung; `coordinator/health → lebendigkeit`, nichts entscheidet). Scharf nur die Kopfdreh-Challenge. Plan `docs/LEBENDIGKEIT_GEGEN_DEEPFAKES.md`; Schwellen erst nach ≥ 20 echten Registrierungen (WP 4). Entscheidung: **keine Stimme** | ich (WP 2 Kette), du (Messlauf = Zwei-Personen-Test, App-Release WP 3) |
| 15 | Signierschlüssel der Validatoren ≠ persönliche Wallet | `RELAYER_PRIVATE_KEY` auf beiden Boxen ein eigener Schlüssel | ⏸ seit 01.09. bekannt (`docs/WAS_DU_NOCH_TUN_MUSST.md` Punkt 3): auf beiden Boxen ist es derselbe Schlüssel wie deine Wallet. Kein Beta-Blocker; ändert die Blockproduktions-Identität — nicht in der Nacht vor einem Start | **du** (Entscheidung + Schlüssel), ich (Umzug als Workflow, wenn du willst) |

## Bewusst nicht auf der Liste

10.000 TPS. Die Kette packt ~7.000/s auf zwei Boxen; mehr braucht einen dritten
Validator oder einen Umbau der Sperren. Für den Launch reichen 7.000/s um
Größenordnungen.

## Wie gemessen wird

- Kette: `bash /tmp/zaehler.sh`, `/api/health/combined` (`divergenz`, `zustands_ablehnung`, `proof_server`, `produktions_ausfaelle`).
- Registrierung: `https://proof1.aequitas.digital/health`, `…/matching/health`, Coordinator `/health` und `/inventory`.
- Rechtstexte: `https://aequitas.digital/api/legal-status`.
- Neuer Validator: `neuer-validator-probe.yml`.
- Alles zusammen: `wache.yml` (Actions → Wache).
