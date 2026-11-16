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

## Hauptaugenmerk: dezentrales Quantengeld (Stand 16.11. ~19:25 UTC)

Auftrag des Betreibers: dezentrales Quantengeld auf heutigem Stand funktionsfähig, damit
Aequitas ernstzunehmendes Quantengeld ist. „Alles umsetzen.“ Die Liste der offenen Punkte
steht in `forschung/quantengeld/OFFEN.md`.

**Erledigt heute Abend:**
- **#545 Befund Klonangriff:** Die Toleranz von 15 % war gebrochen (Kloner 14,6 %).
  Dazu kamen `wiesner_parameter.py` und Variante S (S-Token ohne Speicher,
  Stichproben-Annahmestellen, `s_kette.py`).
- **#546 W2c sichere Prüfgrößen** (Knoten und Spezifikation):
  - Toleranz je Note, k_note aus einer exakten Tabelle mit Absprache-Grenze
    f = (t−1)/v.
  - Offenlegen in Teilen.
  - Kleinste Prüfung ⌈n/255⌉, damit die Ketten nicht billig leerbar sind.
- **#547 (gemergt):**
  - **W2d Validator-Dienst** (`quanten_w_validator.go`): Tresor, Bestätigung mit
    unterschriebenem Auftrag, Anteile erst nach der Antwort, Hintergrund (Zufall, Teile
    über `QUANTEN_W_PEERS`, Fristen).
  - **S2a:** Regeln für Variante S im Knoten (`quanten_s.go`, Prüfvektor).
  - **S2b Teil 1:** Einlösen in Teilen (anmelden, offenlegen, Frist, Fehlversuche).
- **App 1.23.2** komplett veröffentlicht (APK und IPA, `quelle.json` geprüft).
- **S-Token auf echter IBM-Hardware** (Lauf 43927782462): 1.680 Qubits, 0,46 % Fehler,
  angenommen; Doppelvorlage abgewiesen.

**Zusammengeführt mit der Sitzung „Weiter“ (#549, gemergt 16.11. ~20:15 UTC):**
Deren eigener Validator-Dienst liegt nur zum Nachlesen im Zweig `claude/weiter-8vzgu9`
(Commit 64861990, nicht gemergt). Der Dienst aus #547 deckt ihn ab. Übernommen wurde nur,
was zusätzlich war:
- **Nichts vor der Finalität** (`reif` in `quanten_w_validator.go`): Zufall erst, wenn die
  Anfrage final ist, Anteile erst, wenn die Antwort final ist (gesehen bei DAG-Höhe h,
  finalisierter Checkpoint ≥ h). Vorher reichte „die Antwort steht im eigenen Zustand“.
  `StarteQuantenWDienst` bekommt dafür die DAG-Höhe.
- **Ende-zu-Ende-Test mit vier Knoten** (`TestVierKnotenQuantenW_RealDB`, läuft in
  `db-tests`): Präger und Halter über echtes HTTP, alles Übrige tun die Dienste.
- **Gegenprobe:** 17 Mutationen am Dienst, alle rot. Drei blieben zuerst grün (fremde
  Nummer, voller Tresor, Takt vor dem Stichtag) und eine baute nicht (Zeitfenster); dafür
  gibt es jetzt `TestQuantenWDienst_NummerTresorStichtag`.

Nicht übernommen, als Beobachtung für später:
- Der Auftrag des Prägers (`quantenWAuftragNachricht`) bindet die Wurzel, aber nicht die
  Anteile. Wer einen Auftrag auf dem Weg abfängt, könnte ihn mit anderen Anteilen zuerst
  einreichen; die Note wäre dann unprüfbar und K belegt. Abhilfe: Hash der Anteile in die
  Nachricht.
- `bestaetigen` ist nicht wiederholbar („schon bestaetigt“). Geht die Antwort verloren,
  braucht der Präger ein neues K.
- Alle Validatoren reichen jeden Teil der Offenlegung gleichzeitig ein (keine Staffel);
  drei von vier Einreichungen werden abgewiesen und zählen gegen die Burst-Grenze.
- Bei 3 von 4 Validatoren verlangt die Tabelle k_note = 5200, das sind 82 Teile. Mit
  einem Teil je Takt (5 s) dauert die Offenlegung rund 7 Minuten; die Frist ist 30.

**Nächste Schritte:**
1. ~~S2a und S2b Teil 1~~ – gemergt (#547).
2. **S2b Teil 2 – Variante S in die Kette eingehängt, schlafend** (#551,
   `quanten_w_s_kette.go`): sechs Arten im W-Modul
   (`quanten_w_s_ausgeben`, `_binden`, `_waehlen`, `_anmelden`, `_offenlegen`, `_frist`).
   - Register `token` (höchstens 16), d und x als Bitfeld.
   - Wert über Anteile im W-Topf, mit Umlaufabgabe.
   - Validatorprüfung gemeinsam mit W (`quantenWValidatorenLocked`).
   - **Abweichung von `s_kette.py`:** Die zwei Annahmestellen stehen bei der Ausgabe fest,
     und nur Stelle b darf anmelden. Die Vorlage bindet die Adresse der Stelle nicht, und
     die Stichprobe wäre sonst aus der Warteschlange kopierbar (Vorauslaufen).
   - Die Unterschrift der Stelle bindet die Zahl der Fehlversuche. Sonst ließe sich eine
     fehlgeschlagene Anmeldung wiederholen, bis der Token tot ist.
   - Gemergt mit #551. `s_kette.py` ist um die festen Stellen nachgezogen (#552). Die
     Fehlversuche in der Unterschrift der Stelle sind nur im Knoten modelliert, die
     Spezifikation kennt keine Stellen-Unterschrift.
   - Validator-Dienst für S mit #554 (`quanten_w_validator.go`):
     - Auftrag mit `art: "s"` und Stichproben-Bindungen, Bestätigung ohne Anker.
     - Anteile nur für die Stichprobe einer **finalen** Anmeldung und den laufenden Versuch
       (Nummer = Fehlversuche).
     - Takt: in Teilen offenlegen, nach der Frist die Frist einreichen. Lebende Token
       bleiben im Tresor.
3. **Validator-Dienst W, Beobachtungen aus #549:**
   - Erledigt mit #552: (1) Der Auftrag bindet den Hash der Anteile
     (`quantenWAuftragNachricht` v2). (2) Ein identischer Auftrag bekommt dieselbe
     Bestätigung, ein anderer für dasselbe K bleibt abgewiesen.
   - (3) und (4) mit #555:
     - Je Versuch ein zuständiger Validator (`quantenWZustaendig`, verteilt über K). Er
       reicht bis zu 8 Teile je Takt ein; die anderen springen erst ein, wenn ein Teil
       10 s offen ist.
     - Damit dauern 82 Teile rund 1 Minute statt 7, sofern die Teile sofort im Zustand
       stehen (der Knoten ist Leiter). Sonst begrenzt die Blockzeit.
4. **W:** mehrstufige Prüfung; Fehlerkorrektur messen; ~~Ende-zu-Ende-Test mit vier
   Knoten für den Dienst~~ (erledigt, siehe oben); Peer-Liste aus dem Validator-Register;
   Register als Tabelle (vor W6); die vier Beobachtungen oben.
5. **App:** W- und S-Anzeige.
6. **Ionenfalle** (Konto des Betreibers), Vorlage 4 abschicken (Betreiber).
7. **Gutachten (W5/S4):** Der Prüfauftrag (`docs/QUANTENGELD_PRUEFAUFTRAG.md`) ist auf
   den Stand von W2c, W2d und Variante S gebracht. Vorher fragte er noch nach 15 % und
   k = min(64, frei). Es fehlt nur noch ein Gutachter (Betreiber).
8. **W3 auf dem echten Chip:** IBM-Lauf `wiesner = fern` mit Bezahlen, 4 Noten je Fall,
   500 Schüsse (Lauf 43973771243). Er wartet auf die Freigabe von `quantenhardware`;
   danach die Ergebnisse in HEUTE.md 3.3 eintragen.

## Offene PRs

| Repo | PR | Inhalt | Nächster Schritt |
|---|---|---|---|
| Aequitas | #557 | Validator-Dienst: Nachbarn des Knotens (andere Sitzung, `claude/aequitas-fortsetzen-osrea1`) | deren Sache |
| Aequitas | (dieser Zweig `claude/friendly-davinci-i4iww1`) | Prüfauftrag auf heutigen Stand, OFFEN.md aufgeräumt | CI grün abwarten, squash-mergen |

Gemergt am 16.11.:
- #555 (Validator-Dienst: Staffel, bis 8 Teile je Takt)
- #554 (Validator-Dienst für Variante S)
- #552 (Validator-Dienst W: Auftrag bindet Anteile, Bestätigen wiederholbar; `s_kette.py`
  feste Annahmestellen; Stichtag-Wächter-Tests warten auf den Testserver statt `sleep 1`)
- #551 (S2b Teil 2: Variante S schlafend in der Kette)
- #550 (Übergabe nach #549)
- #549 (Validator-Dienst W: nichts vor der Finalität, Ende-zu-Ende-Test auf vier Knoten)
- #544 (OFFEN.md), #545 (Klonangriff, Variante S), #546 (W2c), #547 (W2d, S2a, S2b Teil 1)
- #539 (Variante W: Regeln im Knoten W2a, Prägen ohne Vertrauen W3, Ablauf mit der Kette
  W4 ohne Partner)
- #540, #541, #542 (W2b schlafend eingehängt, W5 Prüfauftrag)
- #543 (Vier-Knoten-Test)

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
