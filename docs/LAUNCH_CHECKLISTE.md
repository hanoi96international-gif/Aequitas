# Launch-Checkliste

**Stand 14.09.2026, 03:45.** Wache: GRÜN — prüft seit heute auch, dass das Tor zu ist (`required`, Quorum ≥ 2) und beide Coordinatoren antworten. App v1.7.0 veröffentlicht und ausgeliefert.

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
| 2 | **Registrierung funktioniert** | App (Gesichtsprüfung) → Coordinator (Quorum 2) → `/api/prove` → Proof-Server `required` → `register_human` in einem Block; Wache grün | ⚠ Kette, Proof-Server (`/api/prove` ohne Bescheinigung → 403, gemessen) und beide Coordinatoren stehen; **die Website liefert seit 14.09. 03:40 app-v1.7.1** (beide Boxen, sha256 `5a5842…`, Release-Schlüssel `473e9d…`; Coordinator proof1 → proof2; 429-Wiederholung). Seit dem 25.08. hat genau **eine** Person den Weg durchlaufen (`gallery_test: 1`) — mit Quorum 2 noch niemand nachweislich. 14.09.: Coordinator → beide Vergleichsdienste → Quorum live durchgespielt (Nicht-Gesicht → `capture_failed` von beiden, keine Bescheinigung); alle 14 App-Endpunkte + RPC antworten ≤ 0,2 s; **Gruppen hinter einer IP sperren sich nicht mehr gegenseitig** (`ip_burst.go`) | **du:** eine Registrierung mit einem frischen Wallet durchlaufen; Beleg: `total_humans` +1, `gallery_test` +1 auf proof1 **und** proof2 |
| 3 | **Impressum & Datenschutzerklärung** | `/impressum` und `/datenschutz` antworten 200 | ❌ beide 404 — alle sieben `LEGAL_*`-Angaben fehlen (`/api/legal-status`). Seit 14.09. ein Klick: `rechtstexte-setzen.yml` (7 Felder, schreibt beide Boxen, startet die Knoten nacheinander neu, prüft 200) | **du:** die sieben Felder — Name, Anschrift, E-Mail, Verantwortliche(r), Aufsichtsbehörde |
| 4 | **Ein Mensch = ein Konto** | Dieselbe Person, zweites Gerät → `duplicate`; andere Person → durch. Schwelle kalibriert. Altkonten mit Gesicht nachgezogen | ⚠ Tor scharf (`required` seit 13.09. 15:13, Quorum 2, Herkunftspflicht an `/api/register`). **Nachzieh-Weg gebaut und live** (`POST /nachziehen` auf proof1 + proof2, Wallet-Signatur + Ketten-Abgleich + Quorum, keine Prägung; App-Seite in v1.7.0 committed, Identity-Tab „Gesicht nachziehen"). Offen: Schwelle nie mit echten Menschen kalibriert; **die 18 bestehenden Menschen haben kein Gesichts-Template**, bis sie nachziehen (auf der Website benannt); `SERVICE_MODE=test` (Wechsel auf `real` erst mit Galerie-Übernahme, `wuerde_realmodus_abweisen`) | **du + eine zweite Person** vor der Kamera nach `docs/DOPPELREGISTRIERUNG_TEST.md` (Schritte 1–7); danach die 18 einmal durch den Nachzieh-Weg (App v1.7.0) |
| 5 | **DSGVO Phase 2** (Phase 1) | `ALLOW_REAL_BIOMETRIC_DATA=true` + `LEGAL_SIGNOFF_DATE` gesetzt, Drittland (Railway in `sfo`) entschieden | ❌ Unterlagen liegen in `aequitas-biometric-beta/docs/dsgvo/`; Entscheidung offen | **du / Jurist** |
| 6 | **Betreiber merkt, wenn es brennt** | Alarm binnen Minuten | ⚠ `wache.yml` läuft bei GitHub nur alle paar Stunden. `/api/wache` (200/503) auf beiden Boxen prüft Produktion, Tor (`required`, Quorum ≥ 2), Coordinator, Divergenz, Platte, Partner. **Neu 14.09.: `wachhund-telegram-installieren.yml`** — Cron auf beiden Boxen alle 5 min, Telegram bei Rot (sofort, dann alle 6 h) und bei Grün; braucht nur Bot-Token + Chat-ID als Secrets | **du (2 min):** @BotFather → Token; Chat-ID; Secrets `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`; Workflow starten. Alternative ohne Bot: UptimeRobot auf die beiden `/api/wache` |

## Wichtig, aber kein Blocker

| # | Punkt | Fertig heißt | Stand | Wer |
|---|---|---|---|---|
| 7 | Neuer Validator nach Anleitung | Fremde Box folgt der Kette und produziert nach Bestätigung; heilt sich selbst | ✅ Weg viermal auf einem Runner geprüft (zuletzt 13.09. 23:15 gegen den Phase-1-Stand: Spitze erreicht, 0 eigene Blöcke). **Selbstheilung:** Rückstand, Stillstand, hängender Block, Totmann **und seit 13.09. belegte Kontostand-Abweichung** lösen beim Laien-Validator den Resync vom signierten Snapshot aus (`AEQUITAS_DIVERGENZ_AUTORESYNC=1` in der Compose-Vorlage; auf C1/C2 aus). `/api/wache` überspringt dort den fehlenden Proof-Server. Guides (12 PDFs) erklären `[AUTO-HEAL]`. Echter Beitritt mit registriertem Wallet fehlt | **du:** ein VPS, `docs/VALIDATOR_EINRICHTEN.md` |
| 8 | Backup | täglich grün, Restore geprüft | ✅ seit 12.09. (Zustand ohne Blöcke, 31–61 MB) | — |
| 9 | Coordinator hängt an Railway (USA) | App zeigt auf einen Contabo-Coordinator; App-Repo auf der Phase-1-Linie; v1.7.0 auf der Website | ✅ 13.09. 22:41 app-v1.7.0, **14.09. 03:30 app-v1.7.1** veröffentlicht (Release, Notiz nennt den echten Signierer) und auf beiden Boxen ausgeliefert (`host-apk-locally.yml`); app-v0.3.1-phase0 als *zurückgezogen* markiert (Pre-Release + Hinweis). **Railway-Coordinator ist seit 14.09. 03:45 weg (404 „Application not found“)** — v1.6.0-Installationen können nicht mehr registrieren, v1.7.1 braucht ihn nicht; Wache meldet ihn nur noch als Hinweis | — |
| 10 | Node-Guide in allen 12 Sprachen | keine Railway-Anleitung mehr erreichbar | ✅ alle zwölf am 12.09. neu erzeugt (Docker Compose), 0× „Railway" | — |
| 11 | Lasttest-Konten | ≥ 300 Paare je Box zahlungsfähig | ❌ 594/661 bzw. 375/662 Konten unter 0,001 AEQ | **du:** `loadtest-widen-senders.yml confirm=true` |
| 12 | App im Play Store | — | ⏸ bewusst offen bis Punkt 5 | du |
| 13 | Dritter Betreiber | Quorum aus drei Haushalten | ⏸ nach Punkt 4 | du |
| 14 | Lebendigkeit gegen Deepfakes | Blitz + Kopfdrehung + Puls als Score, Herkunft als Risiko, gestaffelter Zuschuss (Klassen grün/gelb/rot), Schattenmodus zuerst | ⚠ **WP 1 live im Schatten** (Score, Risiko, Klasse je Registrierung; `coordinator/health → lebendigkeit`). **WP 2 gebaut und ausgerollt, schlafend** (14.09.: Kette `grant_staffel.go` mit Aktivierung 2100, Proof-Server-Durchreichung, Coordinator-Signatur hinter `LEBENDIGKEIT_VERBINDLICH`, App-Weiterreichung; `account_set_xor` beider Boxen vor/nach Deploy byte-gleich). Scharf nur die Kopfdreh-Challenge. Schwellen erst nach ≥ 20 echten Registrierungen (WP 4) | du (Messlauf = Zwei-Personen-Test), ich (WP 3 Coordinator-Teil, WP 4 nach Daten) |
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
