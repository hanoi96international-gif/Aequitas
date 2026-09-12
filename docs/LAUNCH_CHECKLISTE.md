# Launch-Checkliste

**Stand 12.09.2026, abends.** Jeder Punkt hat ein „fertig heißt" — etwas, das man
nachmessen kann. Was nicht messbar ist, steht nicht drin. Wer hier „✅" setzt,
hat gemessen, nicht geglaubt.

**Launch heißt:** Fremde registrieren sich in der App, bekommen AEQ, nutzen es;
„ein Mensch = ein Konto" hält; wer will, betreibt einen Validator nach
Anleitung; die Betreiber erfahren, wenn etwas kaputtgeht.

## Blocker (ohne die kein Launch)

| # | Punkt | Fertig heißt | Stand | Wer |
|---|---|---|---|---|
| 1 | **Beide Validatoren einig über jeden Kontostand** | `divergenz.abweichend=false` auf beiden Boxen nach 6 min Volllast, `uebersprungene_ueberweisungen=0` | ❌ abweichend (Lasttest-Staub; Menschen gleich). Ursache behoben (`wal_seq`), Heilung braucht den Resync | **du:** `gh workflow run resync-contabo1-only.yml --ref main -f confirm=true` — dann ich: Lauf + Messung |
| 2 | **Registrierung funktioniert** | App → Coordinator → Quorum → Proof-Server → `register_human` in einem Block; Wache grün | ✅ Proof-Server auf beiden Boxen waren 5 Tage bzw. 34 h tot — am 12.09. repariert; Wache prüft jetzt alle 10 min | — |
| 3 | **Impressum & Datenschutzerklärung** | `/impressum` und `/datenschutz` antworten 200 | ❌ beide 404 — alle sieben `LEGAL_*`-Angaben fehlen (`/api/legal-status`) | **du:** Werte nach `docs/RECHTSTEXTE_FREISCHALTEN.md` setzen, ich starte die Knoten nacheinander neu |
| 4 | **Ein Mensch = ein Konto** | Dieselbe Person, zweites Gerät → `duplicate`; andere Person → durch. Schwelle kalibriert | ⚠ Technisch scharf (`required`, Quorum 2, Sketch-Vergleich) seit 25.08. — aber `SERVICE_MODE=test`, Schwelle nie mit echten Menschen kalibriert, und die 18 bestehenden Menschen haben keinen Sketch (könnten sich erneut anmelden) | **du + eine zweite Person** vor der Kamera nach `docs/DOPPELREGISTRIERUNG_TEST.md` |
| 5 | **DSGVO Phase 2** | `ALLOW_REAL_BIOMETRIC_DATA=true` + `LEGAL_SIGNOFF_DATE` gesetzt, Drittland (Railway in `sfo`) entschieden | ❌ Unterlagen liegen in `aequitas-biometric-beta/docs/dsgvo/`; Entscheidung offen | **du / Jurist** |
| 6 | **Betreiber merkt, wenn es brennt** | `wache.yml` rot → E-Mail | ✅ seit 12.09., alle 10 min: Höhe, Gleichauf, Proof-Server, Divergenz, Platte, Vergleichsdienst, Coordinator, Backup-Alter | — |

## Wichtig, aber kein Blocker

| # | Punkt | Fertig heißt | Stand | Wer |
|---|---|---|---|---|
| 7 | Neuer Validator nach Anleitung | Fremde Box folgt der Kette und produziert nach Bestätigung | ✅ Weg dreimal auf einem Runner geprüft (Snapshot 21 s, an der Spitze, 0 Fremdblöcke, 145 MB). Echter Beitritt mit registriertem Wallet fehlt | **du:** ein VPS, `docs/VALIDATOR_EINRICHTEN.md` |
| 8 | Backup | täglich grün, Restore geprüft | ✅ seit 12.09. (Zustand ohne Blöcke, 31–61 MB) | — |
| 9 | Coordinator hängt an Railway (USA) | Coordinator auf einer Contabo-Box, App zeigt dorthin | ❌ App v1.6.0 nutzt `coordinator-production-e067.up.railway.app`; Contabo-Coordinatoren laufen, aber ohne öffentliche URL und ohne App-Release | ich (Caddy-Route) + **du** (App-Release, Widerspruchsspeicher umziehen) |
| 10 | Node-Guide in allen 12 Sprachen | keine Railway-Anleitung mehr erreichbar | ✅ alle zwölf am 12.09. neu erzeugt (Docker Compose), 0× „Railway" | — |
| 11 | Lasttest-Konten | ≥ 300 Paare je Box zahlungsfähig | ❌ 594/661 bzw. 375/662 Konten unter 0,001 AEQ | **du:** `loadtest-widen-senders.yml confirm=true` |
| 12 | App im Play Store | — | ⏸ bewusst offen bis Punkt 5 | du |
| 13 | Dritter Betreiber | Quorum aus drei Haushalten | ⏸ nach Punkt 4 | du |

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
