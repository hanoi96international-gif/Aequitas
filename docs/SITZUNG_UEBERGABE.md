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

## Hauptaugenmerk nächste Sitzung: dezentrales Quantengeld

Auftrag des Betreibers (16.11. abends): Dezentrales Quantengeld soll auf heutigem Stand
funktionsfähig werden, damit Aequitas ernstzunehmendes Quantengeld ist. Die vollständige
Liste steht in **`forschung/quantengeld/OFFEN.md`**. Die Kurzfassung:

1. **Die Prüfung deutlich stärker machen.** Heute gelten 15 % Toleranz bei höchstens
   64 Stellen. Damit kommt eine Absprache, die ein Fünftel der Stellen kennt, mit
   1,5·10⁻³ durch. Ziel ist ≤ 2⁻⁴⁰. Die Toleranz soll an die gemessene Fehlerrate
   gekoppelt werden; dazu größere k, mehr Validatoren und eine mehrstufige Prüfung.
2. **Wege ohne Quantenspeicher analysieren.** „Geld zum Mitnehmen“ gibt es heute erst
   mit einem Speicher. Kandidaten: Raumzeit-Token ohne Quantenspeicher (Kent u. a. 2022,
   Quelle prüfen), Zustellen über QKD-Netze, Ionenfalle als Verwahrung, Abgrenzung zum
   Offline-Bargeld. Das Ergebnis kommt als neuer Abschnitt in ANALYSE.md, mit
   Empfehlung.
3. **Danach bauen:** Prüfer-Dienst für Validatoren, App-Anzeige, `fern` auf dem echten
   Chip, Ionenfalle.

## Offene PRs

| Repo | PR | Inhalt | Nächster Schritt |
|---|---|---|---|
| Aequitas | (dieser Zweig) | `forschung/quantengeld/OFFEN.md` und diese Übergabe | CI grün abwarten, squash-mergen |

Gemergt am 16.11.: #539 (Variante W: Regeln im Knoten W2a, Prägen ohne Vertrauen W3,
Ablauf mit der Kette W4 ohne Partner; Sicherheitsprüfung als PR-Kommentar), #543
(Vier-Knoten-Test).

Gemergt am 16.11.: #540 (HEUTE.md, IBM-Läufe), #541 (W2b Teil 1: Regeln geschärft),
#542 (W2b Teil 2: Variante W schlafend in der Kette, `quanten_w_kette.go`, bis
`quantenWAbUnix = MaxInt64`; W5: Prüfauftrag um Variante W erweitert). Einzelheiten zu
W2b stehen im PR #542 und im Kopf von `x/humanity/keeper/quanten_w_kette.go`.

## Unruhe im Vier-Knoten-Test (Aufgabe #65) — Korrektur in #543

`TestVierKnoten_RealDB` kippte am 16.11. zweimal im Push-Lauf (`db-tests`), mit zwei
verschiedenen Ursachen. Beide lagen im Test, nicht im Knoten; keine Prüfung wurde
gelockert.

1. **Lauf von 3e0dec65** (`LeiterFaelltAusUndZiehtNach`, 90 s „keine Einigkeit“): Holt der
   alte Leiter seinen Zustand per Snapshot, während der Spender zwei Geschwister-Spitzen
   auf der Snapshot-Höhe führt, setzt er nur die kanonische als Spitze (block.go, so
   gewollt). Wurzel und Konten sind gleich, die Spitzen erst nach dem nächsten Block – und
   in der Ruhe des Tests erzeugt niemand. Korrektur: Steht alles außer den Spitzen 5 s
   gleich, erzeugt ein Knoten genau einen Block; danach gilt die volle Prüfung
   unverändert. `leiterAusfall` stellt die Lage jetzt bei jedem Lauf her
   (`zweiGeschwisterSpitzen`). Gegenprobe: ohne die Zusammenführung kippt der Lauf lokal
   genau wie in der CI. Erneutes Anbieten der Spitzen (Idee der Vorsitzung) hilft nicht:
   der Knoten kennt den Block bereits.
2. **Lauf von d75ba6ca** (`TrustedSeedsFromSync`, „Schwelle ohne Beleg nicht als abgewehrt
   gezaehlt“): Die Replay-Mauer zählt Abweisungen in Folge und löscht die Zählung bei jedem
   erfolgreich nachgespielten Block. Die Ruhe begann erst nach dem Bau des Angriffsblocks;
   ein spät ankommender ehrlicher Block setzte die Folge zurück. Korrektur: Ruhe vor dem
   Bau, `warteStill`, gewartet wird auf die beiden Ereignisse selbst. Diese Ursache ist aus
   Code und CI-Meldung abgeleitet, lokal nicht nachgestellt.

Dazu: `spitzenBefund` (wer kennt welche Spitze) bei Uneinigkeit, und `db-tests` druckt bei
Rot die letzten Testmeldungen.

Lokal laufen lassen: PostgreSQL 16 liegt im Container (`pg_ctlcluster 16 main start`,
Passwort per `ALTER USER postgres PASSWORD 'postgres'`), dann

```
AEQUITAS_TPS_BENCH=1 DATABASE_URL="postgres://postgres:postgres@localhost:5432/aeq_ci_realdb?sslmode=disable" \
  go test ./x/humanity/keeper/ -run 'TestVierKnoten_RealDB$' -v -timeout 15m
```

## Danach (Reihenfolge)

0. **App 1.23.2:** Die APK ist veröffentlicht, die **iOS-IPA fehlt im Release**
   (release-ios mit Lauf 43884174136 auf der Basis `aequitas-app`). Danach `quelle.json`
   prüfen.

1. **W5 (Aufgabe #64):** `docs/QUANTENGELD_PRUEFAUFTRAG.md` ist um Variante W erweitert
   (#542). Offen: Gutachter suchen (Anschreiben wie `ANSCHREIBEN.md`).
2. **Flake (#65):** Korrektur im PR (Abschnitt oben). Nach dem Merge ein paar Push-Läufe
   beobachten; kippt `db-tests` wieder, stehen die Meldungen jetzt am Ende des Protokolls.
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
