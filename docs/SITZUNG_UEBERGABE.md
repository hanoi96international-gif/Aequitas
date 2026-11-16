# Übergabe für die nächste Sitzung

**Zuerst lesen.** Diese Liste wird nach jedem Merge und jedem größeren Schritt aktualisiert,
damit eine Sitzung, die am Nutzungslimit abbricht, ohne Rückfragen weitermachen kann.
Stand: 16.11.2026 abends, Sitzung „Weiter“ (Auftrag des Betreibers: „Alles erledigen
für dezentrales Quantengeld“).

## Arbeitsweise (vom Betreiber festgelegt)

- Antworten auf Deutsch. Bei allem, was eine Freigabe braucht, den Link schicken.
  Mergen ist vorab freigegeben.
- Nie Geheimnisse im Chat erfragen (GitHub-Secrets oder Credential-Formular). Keine
  Impressum-Daten erfinden.
- Push nur auf den Zweig, den die Sitzung vorgibt (alle vier Repos; zuletzt
  `claude/weiter-8vzgu9`, davor `claude/aequitas-fortsetzen-osrea1`). Nach einem
  Squash-Merge, `<zweig>` = der Zweig der Sitzung:
  1. `git checkout -B <zweig> origin/main`
  2. committen
  3. `git merge -s ours origin/<zweig>`
  4. normal pushen

  **Nie Force-Push**, auch nicht in Hintergrundjobs. Die App hat die Basis `aequitas-app`.
- Squash-Merge immer mit der vollen 40-stelligen SHA (`expectedHeadSha`).
- Jeder PR hat einen Abschnitt **Sicherheit** (5 Fragen aus AGENTS.md). Änderungen an
  Konsens, Signaturen, Kontoständen und Netzwerk-Eingängen bekommen vor dem Merge eine
  eigene Sicherheitsprüfung des Diffs als PR-Kommentar. Zu jeder neuen Regel ein
  Missbrauchstest; Gegenprobe durch Mutation (Regel zurückdrehen → Test muss rot werden).
- Lokale Go-Tests: Erst `df -h /` prüfen. Bei vollem Platz `go clean -cache`. Die
  Fehlermeldung „MB of disk left“ kommt vom Container, nicht vom Code.

## Offene PRs

Keine (Stand dieser Zeile; den PR zur Aufgabe #65 unten nachtragen, sobald er offen ist).

Gemergt am 16.11.: #540 (HEUTE.md, IBM-Läufe), #541 (W2b Teil 1: Regeln geschärft),
#542 (W2b Teil 2: Variante W schlafend in der Kette, `quanten_w_kette.go`, bis
`quantenWAbUnix = MaxInt64`; W5: Prüfauftrag um Variante W erweitert). Einzelheiten zu
W2b stehen im PR #542 und im Kopf von `x/humanity/keeper/quanten_w_kette.go`.

## In Arbeit: Unruhe im Vier-Knoten-Test (Aufgabe #65)

`TestVierKnoten_RealDB` kippte am 16.11. zweimal im Push-Lauf (`db-tests`), mit zwei
verschiedenen Bildern:

1. **Lauf von d75ba6ca:** `TrustedSeedsFromSync` → „Schwelle ohne Beleg nicht als abgewehrt
   gezaehlt“. Ursache: Die Replay-Mauer zählt Abweisungen in Folge und löscht die Zählung
   bei jedem erfolgreich nachgespielten Block. Die Ruhe begann erst nach dem Bau des
   Angriffsblocks; ein spät ankommender ehrlicher Block setzte die Folge zurück, die
   Schleife endete nach drei Abweisungen insgesamt. Korrektur im Test
   (`vier_knoten_snapshot_seeds_test.go`): Ruhe vor dem Bau, `warteStill`, gewartet wird
   auf die beiden Ereignisse selbst.
2. **Lauf von 3e0dec65:** `LeiterFaelltAusUndZiehtNach` und danach `TrustedSeedsFromSync`
   laufen in `pruefeGleichstand` 90 s ohne Einigkeit: alle vier auf Höhe 124 mit derselben
   Wurzel, Knoten 0 (alter Leiter) mit einer Spitze, die anderen mit zweien. Ursache noch
   **nicht belegt**. `spitzenBefund` schreibt bei Uneinigkeit jetzt auf, wer welche Spitze
   kennt. Idee der Vorsitzung: in der Ruhe `reannounceTips` (tip_reannounce.go) rufen –
   erst einbauen, wenn der Befund zeigt, dass der Knoten die Spitze über den Push annimmt.

Lokal nachstellen: PostgreSQL 16 liegt im Container (`pg_ctlcluster 16 main start`,
Passwort per `ALTER USER postgres PASSWORD 'postgres'`), dann

```
AEQUITAS_TPS_BENCH=1 DATABASE_URL="postgres://postgres:postgres@localhost:5432/aeq_ci_realdb?sslmode=disable" \
  go test ./x/humanity/keeper/ -run 'TestVierKnoten_RealDB$' -v -timeout 15m
```

Ein Lauf dauert rund 130 s und ist ohne Last grün; rot wird er nur gelegentlich.

## Danach (Reihenfolge)

1. **W5 (Aufgabe #64):** `docs/QUANTENGELD_PRUEFAUFTRAG.md` ist um Variante W erweitert
   (#542). Offen: Gutachter suchen (Anschreiben wie `ANSCHREIBEN.md`).
2. **Flake (#65):** siehe „In Arbeit“ oben. Test robust machen, nicht abschwächen.
3. **Ionenfalle:** Konto des Betreibers nötig (Quantinuum Nexus oder die Alternativen in
   ANALYSE.md), als GitHub-Secret `QUANTINUUM_TOKEN`. Erst fragen, wenn der Betreiber so weit
   ist.
4. **PQ Stufe 1 (#44):** Registrierung des Nutzers (Höhe 4304, tx 0x34233121…) auf den
   PQ-Teil prüfen, dann `pq_ab_unix` setzen.
5. **Ältere offene Aufgaben:**
   - #42 Play Store
   - #45 Stufe 0
   - #54 und #56: wahrscheinlich durch #516–#518 erledigt, prüfen
   - #57 V8-Vertragsprüfung

## Live-Stand (zuletzt geprüft)

- Kette neu ab Genesis 15.11.2026 16:00 UTC (`aequitas-1926-1794758400`), Höhe ~77.600.
- 1 Mensch, C1 gebunden, PQ-Stempel 51 ok.
- Quantengeld auf IBM `ibm_marrakesh`: W4 ohne Wartezeit echt eingelöst (Annahme 0,998).
  Mit 20 µs plus Teleportation abgewiesen. Der 3-Qubit-Schutz brachte keinen messbaren
  Gewinn (`forschung/quantengeld/HEUTE.md`).
