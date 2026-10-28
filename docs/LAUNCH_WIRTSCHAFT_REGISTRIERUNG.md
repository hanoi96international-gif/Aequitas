# Launch: Wirtschaftsregeln und Registrierung

Stand 25.10.2026. Was für den Launch gebaut ist, in welcher Reihenfolge es
scharf wird, woran man sieht, dass es läuft, und was nur der Betreiber tun
kann. Hintergrund: `WIRTSCHAFT_ENDFASSUNG.md`, `REGISTRIERUNG_ENDFASSUNG.md`,
Prüfung der Diffs: `SICHERHEITSPRUEFUNG_2026-10-25.md`.

**Grundsatz: nichts ist endgültig festgezurrt.** Jede neue Regel hat einen
eigenen Schalter (Aktivierungszeit im Code, die beim Ausrollen gesetzt wird)
oder eine Einstellung im Betrieb; der Satz des Gleichgewichts lässt sich in
festen Grenzen und mit Vorankündigung ändern (`satz_plan.go`). Scharf wird
nichts ohne eure Entscheidung.

## 1. Was gebaut ist

| Baustein | Wo | Stand |
|---|---|---|
| Umlauf aus alter Runde nimmt kein Geld (`umlauf_alte_runde`) | Kette | **wirkt auf die laufende Kette**, streng ab 01.11.2026 |
| Tagesrunde unter Last (gesammelt schreiben, Stücke zu 5.000) | Kette | **wirkt sofort**; vorher fiel die Runde ab etwa 50.000 Menschen täglich aus |
| Staffel: Rest spätestens an Tag 30, Einspruch zweier Betreiber | Kette | schlafend (`stagedGrantActivationUnix`) |
| Vor der Kalibrierung jede Registrierung gestaffelt | Coordinator | wirkt mit `LEBENDIGKEIT_VERBINDLICH=true` |
| Widerspruch: Antwortfrist 14 Tage, Zahlen auf `/health` | Coordinator | **wirkt sofort** nach Ausrollen |
| Gleichgewicht (0,5 %/Monat auf jedes Guthaben, keine Überweisungsgebühr, Vollständigkeit der Runde) | Kette | schlafend (`gleichgewichtAktivAbUnix`) |
| Satz einstellbar (0,25–1 %, ±0,25 je Änderung, 30–90 Tage Vorlauf, einmal je Quartal, zwei Betreiber) | Kette | schlafend wie das Gleichgewicht |
| Umzug (Konto zurückholen) | Entwurf `UMZUG_ENTWURF.md` | wartet auf eure Entscheidung zu `kein_entzug_test.go` |

## 2. Reihenfolge

Jeder Schritt: erst auf **beide** Knoten ausrollen, dann das Datum setzen
(das Datum liegt immer nach dem Ausrollen — eine Regel, die auf einem Knoten
gilt und auf dem anderen nicht, spaltet die Kette).

1. **Vor dem 01.11.2026:** diesen Stand auf C1 und C2 ausrollen
   (`umlauf_alte_runde`, Tagesrunde unter Last). Am 01.11. wird das
   Nachrechnen für alle Knoten streng (`nachrechnenStrengAbUnix`).
2. **Vier Wochen beobachten:** `/api/wirtschaft/regeln` →
   `unternehmen.nachrechnen` und `liegegeld_pruefung` ohne neue
   Abweichungen. Jede Abweichung ist vor dem nächsten Schritt zu klären.
3. **Coordinatoren im Konsens zulassen:** `registerLeserAbUnix` (heute
   `MaxInt64`) auf ein Datum nach dem Ausrollen. Mindestens zwei Betreiber-
   Menschen mit Validator-Schlüssel im Register — ohne sie gibt es weder
   Lebensnachweis-Pausen noch Einsprüche noch Satzänderungen.
   `TestLebensnachweis_AktivierungErstNachBefunden` hält fest, dass
   verteilte Annahme und Lebensnachweis nicht zugleich scharf werden.
4. **App mit Zweitprüfung draußen** (WP 3), dann die **Staffel**:
   `stagedGrantActivationUnix` setzen, am Tag danach
   `LEBENDIGKEIT_VERBINDLICH=true` an beiden Coordinatoren. Bis zur
   Kalibrierung ist jede Registrierung gestaffelt (200 sofort, Rest
   spätestens an Tag 30).
5. **Kalibrieren** (WP 4, mindestens 20 echte Registrierungen, rund 1.000
   Paare für die Verwechslungsrate), dann `LEBENDIGKEIT_KALIBRIERT=true`.
6. **Gleichgewicht:** Entscheidung, Texte (Website, App, 12 Sprachen),
   Ankündigung **30 Tage vorher** mit dem eigenen Netto-Betrag, dann
   `gleichgewichtAktivAbUnix` setzen. Der Satz beginnt bei 0,5 % und ist
   danach nur noch über `POST /api/wirtschaft/satz` änderbar.

## 3. Last

Gemessen mit `BenchmarkTagesrundeDB` (Erzeuger und ein zweiter Knoten, der
nachspielt; beide enden mit denselben Kontoständen):

| Konten | Regeln | Leiter steht | Nachspielen |
|---|---|---|---|
| 20.000 | heute | 1,3 s | 1,7 s |
| 20.000 | Gleichgewicht | 1,9 s | 2,5 s |
| 100.000 | heute | 9,9 s | 11,2 s |
| 100.000 | Gleichgewicht | 12,2 s | 20,1 s (rund 1 s je Block) |

Reicht für den Launch bis 10⁵ Konten. Darüber die Runde in Abschnitte
teilen (`ubi_chunked.go`, gebaut, nicht verdrahtet) — vorher messen.

```
AEQUITAS_TPS_BENCH=1 DATABASE_URL=postgres://…/wegwerf AEQUITAS_RUNDE_KONTEN=100000 \
  go test ./x/humanity/keeper/ -run '^$' -bench BenchmarkTagesrundeDB -benchtime 1x
```

## 4. Woran man sieht, dass es läuft

| Wo | Was |
|---|---|
| `GET /api/wirtschaft/regeln` → `unternehmen.nachrechnen` | Abweichungen je Regel (`liegegeld`, `umlauf_alte_runde`, `freigabe_*`, `einspruch_*`, `umlauf_ausgelassen`, `satz_*`, …) |
| `GET /api/wirtschaft/regeln` → `gleichgewicht` | aktiv, Satz jetzt, angekündigter Satz, Vollständigkeitsprüfung (Runden geprüft/übersprungen) |
| Coordinator `/health` → `widersprueche` | offene, über der Antwortfrist, älteste Wartezeit; Warnungen `widerspruch_ueber_antwortfrist`, `widerspruch_ueber_einen_monat_offen` |
| Coordinator `/health` → `lebendigkeit` | `verbindlich`, `kalibriert`, Klassenzähler |

## 5. Was nur ihr tun könnt

- Ausrollen und die Daten setzen (Abschnitt 2).
- Rechtstexte: Datenschutzerklärung (Antwortfrist im Widerspruchsweg,
  Fristen), AGB, Ankündigung des Gleichgewichts.
- Ein zweiter, unabhängiger Betreiber-Mensch mit Validator-Schlüssel.
- Echte Testpersonen für die Kalibrierung (WP 4).
- Entscheidung zum Umzug (Ausnahme in `kein_entzug_test.go`).
- App-Texte zur Staffel und zum Gleichgewicht: warten auf den Stand 1.13.0
  der anderen Sitzung (nicht gepusht), damit keine Versionen kollidieren.
