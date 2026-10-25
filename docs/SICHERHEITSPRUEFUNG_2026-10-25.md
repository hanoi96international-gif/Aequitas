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

## 2. Gleichgewicht (schlafend)

Dateien: `wirtschaft3.go` (neu), `nachrechnen_umlauf.go` (neu),
`wirtschaft.go` (`umlaufBetragRoh`, `umlaufLocked`, `gebuehrMitWirtschaft`),
`liegegeld_pruefung.go`, `nachrechnen_ueberweisung.go`
(`gebuehr_nach_gleichgewicht`, `lohnGebuehrPraefix`), `nachrechnen.go`,
`state.go` (Rundenstand, Snapshot, Neustart), `wirtschaft_api.go`. Alles
schlafend bis `gleichgewichtAktivAbUnix` (2100) und nur zusammen mit
`wirtschaftAktiv`.

| # | Frage | Befund | Stand |
|---|---|---|---|
| 1 | Ändert sich vor der Aktivierung etwas? | Nein. Betrag, Kandidaten (nur Datenbank, Menschen über dem Sparfreibetrag), Gebühr und Prüfung wie bisher; die Vollständigkeit merkt sich nur die Phase der Runde, ohne Mengen, ohne Meldung. Einzige Änderung im heutigen Pfad: ein Lesefehler beim Durchlaufen der Kandidaten (`rows.Err`) bricht die Runde jetzt ab, statt mit einer unvollständigen Liste weiterzumachen (fail-closed, die Runde wird wiederholt). | Tests `…_SchlaeftBis2100`, `…_VollstaendigkeitSchlaeft`, ganzes Paket grün |
| 2 | Kann ein Erzeuger Konten auslassen? | Nein: `umlauf_ausgelassen`, auch wenn er die ganze Runde folgerichtig ohne das Konto rechnet (Grundeinkommen und Topf passen dazu). | Test `…_Missbrauch` („verbergen“) |
| 3 | Doppelt belasten, zu hoch, falsche Reihenfolge, andere Rundenzeit | je eine Regel; jeder Betrag wird nachgerechnet, auch bei Unternehmen ohne volles Umsatzfenster auf diesem Knoten | Tests `…_Missbrauch`, `…_UnternehmenOhneBuchfuehrungGeprueft` |
| 4 | Falscher Alarm bei ehrlichen Blöcken (hält die Kette an) | Der Stand nicht belasteter Konten wird am Ende des Umlaufteils festgehalten, wenn er dem des Erzeugers gleicht; gemeldet wird nur klar über der Schwelle (Toleranz 2·10⁻⁶). **Gefunden in diesem Durchgang:** ein Konto, dessen erstes Geld der WAL-Schnellpfad noch nicht in die Datenbank geschrieben hat, sah der Erzeuger nicht, der Nachspielende schon. Behoben: ab dem Gleichgewicht Kandidaten aus Datenbank **und** Speicher, auf beiden Seiten. Neustart: bis zur nächsten Rundenmarke keine Meldung. Runde über der Aktivierungsgrenze: nicht geprüft. | Tests `…_EchteRundenOhneAbweichung` (ein und zwei Blöcke, streng), `…_NeustartMeldetNichts`, `…_KandidatenAusDatenbankUndSpeicher_RealDB` |
| 5 | Gebühr trotz Gleichgewicht | serieller Pfad: `gebuehr_nach_gleichgewicht`; der schnelle Pfad kannte nur die 0,1 %-Obergrenze und hätte gebucht — er gibt jede Gebühr jetzt an den seriellen ab. Entscheidend ist der Buchungsaugenblick (höchstens eine Woche vor dem Block): eine kurz vor der Aktivierung angenommene Überweisung behält ihre Gebühr. | Test `…_GebuehrImBlock` |
| 6 | Grenzen | Belastete nur, wenn das Konto im eigenen Zustand existiert; offene nur, wenn sie in sieben Tagen die Schwelle erreichen könnten — beides durch die Zahl der Konten begrenzt. Aufwand je Runde wie beim Erzeuger, O(Konten) unter `cs.mu`; ab etwa 10⁵ Konten in Abschnitte teilen (Endfassung, Abschnitt 10). | hingenommen, dokumentiert |
| 7 | Rückrollen, Snapshot-Import | Rundenstand im Rückroll-Snapshot, Einträge des abgewiesenen Blocks fallen weg (laufende Nummer). Nach Neustart **und** nach einem Snapshot-Import (`replaceInMemoryFromSnapshotLocked`, Zusammenführen beim Kaltstart) gilt die laufende Runde als unbeobachtet — ein veralteter Stand könnte sonst bei jedem folgenden Block anschlagen und den Knoten festhalten. | Test `…_RundeRollt`; abgewiesene Blöcke ändern weder Topf noch Konten (`…_Missbrauch`, streng) |
| 8 | **Heutige Regeln: Umlauf aus einer alten Runde** | **Gefunden in diesem Durchgang, HOCH, besteht seit der Umlaufsicherung.** `pruefeUmlaufLocked` ließ einen Umlauf, dessen Rundenzeit vor der letzten Runde liegt, ungeprüft durch („älterer Lauf“), und `applyUmlaufDeltaLocked` zog ihn bis zum ganzen Guthaben ab. Ein Erzeuger konnte so jedem Konto sein AEQ ins Grundeinkommen nehmen. Eine Untergrenze der Rundenzeit gab es weder beim Nachspielen noch in `block_tauglich.go`, und ob streng geprüft wird, hätte die gefälschte Rundenzeit entschieden. Behoben: Regel `umlauf_alte_runde`, streng ab `nachrechnenStrengAbUnix` nach der **Blockzeit**. Ehrlich entsteht das nie (Runden laufen vorwärts; eine schon angewandte Runde überspringt das Nachspielen an ihrer Marke). Gleich alt — Neustart mitten in der Runde — bleibt unverändert. **Wirkt auf die laufende Kette:** vor dem 01.11.2026 auf beide Knoten ausrollen, sonst lehnt nur der neue Stand einen solchen Block ab. | Test `TestUmlauf_AlteRundeNimmtKeinGeld` (beobachten zählt, streng lehnt ab, Guthaben bleibt; Neustart lehnt nicht ab) |

Ergebnis: ein Befund HOCH in den heutigen Regeln (8), behoben; sonst keine
offenen Befunde über NIEDRIG. Offen und bewusst nicht
gebaut: Punkt f (Buchführung abschalten), Anzeige in der App, Teilen der
Runde für sehr viele Konten. Vor der Aktivierung nötig: eure Entscheidung,
ein echtes Datum nach dem Ausrollen, vier Wochen strenges Nachrechnen ohne
Abweichung (Voraussetzung 1).

## 3. Tagesrunde unter Last

Dateien: `wirtschaft.go` (`umlaufSammelndLocked`, `applyUmlaufDeltaLocked`
mit Sammler), `state.go` (`distributeUBIPoolSammelndLocked`,
`applyUBIRewardDeltaLocked` mit Sammler, `saveAccountsToDBBatchCtx` in
Stücken, Ausgang der Runde), `evm_storage.go` (`savePendingTxsGeordnetExec`),
`block.go` (Nachspielen mit dem Block-Sammler).

| # | Frage | Befund | Stand |
|---|---|---|---|
| 1 | Ändert das ein Ergebnis? | Nein. Nur wann geschrieben wird: dieselbe Datenbanktransaktion, dieselben Endstände; das Blatt der StateRoot wird je Konto sofort fortgeschrieben (`updateAccountLeafLocked`), wie im parallelen Nachspielen. Zwischen Sammeln und Schreiben liest kein Schritt Kontostände aus der Datenbank (der Durchschnitt der Vermögensgrenze ist die Konstante 1.000; Gini aus dem Speicher); geschrieben wird vor Freigaben, Validatoren und Treuhand. | Ganzes Paket und Datenbankgruppen grün; der Benchmark vergleicht Erzeuger und Nachspieler |
| 2 | Reihenfolge im Block | Der Ausgang bekommt seine IDs in der Reihenfolge der Runde (`WITH ORDINALITY … ORDER BY`), über alle Stücke. | `TestAusgangGeordnet_RealDB` |
| 3 | Sperrreihenfolge | Konten in Stücken, nach Adresse sortiert über alle Stücke — dieselbe Reihenfolge wie ein Statement, keine neue Verklemmung. | geprüft |
| 4 | **Befund: Runde fällt ab etwa 50.000 Menschen aus** | Alle Menschen in einem Statement rissen `statement_timeout=5000` (jede Verbindung), die Runde wurde zurückgerollt — jeden Tag, auch mit den heutigen Regeln. Behoben mit Stücken zu 5.000. | gemessen mit 100.000 Konten |

Ergebnis: ein Befund (4), behoben; keine offenen.

