# Sicherheitsprüfung des Diffs, 25.10.2026

Getrennter Durchgang über die Konsensänderungen auf
`claude/aequitas-registrierung-wirtschaftsregeln-arrynf` (AGENTS.md,
„eigene Sicherheitsprüfung des Diffs“). Gehört als Kommentar in den PR,
sobald es einen gibt.

## 1. Staffel: Freigabe spätestens an Tag 30, Einspruch

Dateien: `grant_staffel.go`, `staffel_einspruch.go`, `nachrechnen_freigabe.go`,
`block.go`, `block_tauglich.go`, `state.go`, `api.go`, `leitung_netz.go`,
`weiterleitung_nachweis.go`, `ip_burst.go`. Alles schlafend bis
`stagedGrantActivationUnix` (2100).

| # | Frage | Befund | Stand |
|---|---|---|---|
| 1 | Kann ein Erzeuger mehr oder früher freigeben? | Jeder Knoten rechnet `staffelFreigabeHoechstens` mit der Rundenzeit der Freigabe nach; die Rundenzeit darf höchstens 60 s nach dem Block liegen (`freigabe_rundenzeit`, auch in `block_tauglich.go`). Vordatieren bringt nichts, rückdatieren verkleinert nur. | geprüft, Test `TestStaffel_FreigabeNachrechnen` |
| 2 | Kann ein Erzeuger trotz Einspruch freigeben? | `freigabe_angehalten`. | Test |
| 3 | Reihenfolge: Erneuerung oder Einspruch angenommen, aber noch in keinem Block, während die Runde freigibt | Andere Knoten sähen eine Freigabe ohne gültige Prüfung (im strengen Modus ungültiger Block). **Gefunden in diesem Durchgang** (bestand für Erneuerungen schon vorher, schlafend). Behoben: die Runde lässt genau diese Wallets aus (`offeneStaffelAuftraege`); Lesefehler → keine Runde. | behoben, Test `TestStaffelEinspruch_SpalteUndAusgang_RealDB` |
| 4 | Einspruch mit einem Betreiber, zweimal derselbe Mensch, fremde Bindung, gefälschte Unterschrift, getauschter Grund, fremde Wallet, falsche Domäne, nicht zugelassen, Betreiber mit offener Staffel, zu spät, aus der Zukunft, zu alt, ohne Staffel, Form | jeder Fall nicht angewandt (auch im Beobachtungsmodus) und gemeldet; im strengen Modus Fehler | Test `TestStaffelEinspruch_Missbrauch`, `…_StrengLehntAb` |
| 5 | Hält ein Einspruch Geld ohne Zeitgrenze zurück? | Nein: höchstens bis Tag 180; eine neuere Zweitprüfung beendet die Pause sofort; nur der erste Einspruch zählt. | Test `TestStaffel_KeinZurueckhaltenOhneEnde`, `…_NurDerErste` |
| 6 | Können zwei Betreiber jemandem schaden? | Sie können die Freigabe bis Tag 180 verzögern, nichts nehmen. Gegen eine Absprache aller Coordinatoren hilft die Frist (Tag 180). | hingenommen, dokumentiert |
| 7 | Grenzen | Körper 16 KiB, 2–8 Bescheinigungen, Grund 10–280 Byte, eigene Zähler je Absender (`nachweisGrenze`), weitergeleitete in der begrenzten Karte; alle Unterschriften vor jeder Datenbankabfrage. | geprüft |
| 8 | StateRoot | `StaffelEinspruchAm` im Blatt nur, wenn gesetzt; Spalte in allen Lade-, Speicher- und Stapelpfaden, Neuaufbau gleich dem Speicher. | Tests `…_BlattNurWennGesetzt`, `…_SpalteUndAusgang_RealDB` |
| 9 | Datenschutz | Der Grund steht für immer in der Kette. **Gefunden**: keine Regel dagegen. Behoben als Vorgabe im Code und in der Doku: keine personenbezogenen Daten, nur Befund und Vorgangskennung. Technisch nicht erzwingbar. | Vorgabe |
| 10 | Vor der Aktivierung | Art unbekannt (`bekannteTxArt`), Endpunkt 409, `block_tauglich` lässt sie in keinen Block. | Test `…_LeerlaufVorStaffel` |

Ergebnis: keine offenen Befunde über NIEDRIG. Tests: Paket `keeper` grün
(ohne Datenbank), die fünf Datenbankgruppen der CI lokal grün gegen
Postgres 16 (realdb 257, transfer 28, wal 58, plumbing 7, mpcstore 4),
`-race` für die betroffenen Tests grün.
