# Übergabe für die nächste Sitzung

**Zuerst lesen.** Diese Liste wird nach jedem Merge und jedem größeren Schritt aktualisiert,
damit eine Sitzung, die am Nutzungslimit abbricht, ohne Rückfragen weitermachen kann.
Stand: 16.11.2026, Sitzung „Aequitas fortsetzen“ (Auftrag des Betreibers: „Alles erledigen
für dezentrales Quantengeld“).

## Arbeitsweise (vom Betreiber festgelegt)

- Antworten auf Deutsch. Bei allem, was eine Freigabe braucht, den Link schicken.
  Mergen ist vorab freigegeben.
- Nie Geheimnisse im Chat erfragen (GitHub-Secrets oder Credential-Formular). Keine
  Impressum-Daten erfinden.
- Push nur auf `claude/aequitas-fortsetzen-osrea1` (alle vier Repos). Nach einem
  Squash-Merge:
  1. `git checkout -B claude/aequitas-fortsetzen-osrea1 origin/main`
  2. committen
  3. `git merge -s ours origin/claude/aequitas-fortsetzen-osrea1`
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

| Repo | PR | Inhalt | Nächster Schritt |
|---|---|---|---|
| Aequitas | #542 | Variante W schlafend in die Kette eingehängt (`quanten_w_kette.go`) und W5 (Prüfauftrag) | Sicherheitsprüfung als Kommentar posten, CI grün abwarten, squash-mergen |

Gemergt in dieser Sitzung: #540 (HEUTE.md, IBM-Läufe), #541 (W2b Teil 1: Regeln geschärft).

## W2b Teil 2 — Stand

Neue Datei `x/humanity/keeper/quanten_w_kette.go`, schlafend bis `quantenWAbUnix = MaxInt64`.

**Transaktionsarten** (6):

| Art | Wer | Wirkung |
|---|---|---|
| `quanten_w_praegen` | Wallet, unterschrieben | Deckung → W-Topf |
| `quanten_w_anfrage` | Wallet, unterschrieben | Pfand → Pfand-Topf; das Pfand rechnet jeder Knoten selbst aus dem Wert der Note |
| `quanten_w_zufall` | ohne Wallet | Werte der Hash-Ketten |
| `quanten_w_antwort` | Wallet, unterschrieben | die Bits |
| `quanten_w_offenlegen` | ohne Wallet | Anteile und Pfade; Urteil und Buchungen rechnet jeder Knoten |
| `quanten_w_frist` | ohne Wallet | beendet eine Anfrage nach ihrer Frist |

**Validatoren:** Alle bestätigen per EIP-191 (Wurzel, Anker, Prägezeit). Jeder ist im
Validator-Register an einen Menschen gebunden, alle an **verschiedene** Menschen.

**Töpfe:**
- `quantenWTopfAddr`: Deckung, Anteilsmodell, Umlaufabgabe über `istNotenTopf`.
- `quantenWPfandAddr`: offene Pfänder, ohne Abgabe.
- Was verfällt, geht an `ubiPoolAddr`.

**Register und Block:**
- Register in chain_config unter `quanten_w` (JSON), höchstens 64 lebende Noten; tote
  bleiben gemerkt, solange ihre Bestätigung gelten könnte.
- StateRoot-Teil `|quantenw:` + Hash, nur wenn das Register nicht leer ist.
- Der Block nennt die gerechneten Felder (Anteile, Pfand, PfandAn, Wert, WertAn, Urteil),
  jeder Knoten rechnet nach.
- **BuchAt** wie bei Überweisungen: Der Erzeuger stempelt die Annahmezeit, jeder Knoten
  rechnet mit ihr, wenn sie zum Block passt (`quantenWBuchZeit`: ≤ 60 s nach,
  ≤ 7 Tage + 60 s vor dem Block). Tote bleiben entsprechend länger gemerkt
  (`quantenWToteBehalten`).

**Eingehängt:**
- block.go: `bekannteTxArt`, Replay-Fall, `quantenWFeldErlaubt`
- block_tauglich.go
- state.go: `blockTouchedAddresses`, `isTokenomicsPoolAddress`, StateRoot,
  Rückroll-Snapshot, Laden beim Start
- `registerSicherung`, `umlaufArt`/`umlaufZahlt`, `pruefeTopfNurMitNote`,
  `replay_parallel`
- snapshot.go, checkpoint_tx.go
- api.go: `POST /api/quanten-w/<art>`, `GET /api/quanten-w/note?k=`
- `kein_entzug_test.go` (Einordnung der Arten)

**Tests:** `quanten_w_kette_test.go` mit 11 Tests. Alle Keeper-Tests grün; 19 Mutationen, alle rot.

## Danach (Reihenfolge)

1. **W5 (Aufgabe #64):** `docs/QUANTENGELD_PRUEFAUFTRAG.md` um Variante W erweitert
   (liegt im W2b-Teil-2-PR). Danach: Gutachter suchen (Anschreiben wie `ANSCHREIBEN.md`).
2. **Flake (#65):** `TestVierKnoten_RealDB` → „Knoten 3: Schwelle ohne Beleg nicht als
   abgewehrt gezaehlt“ (vier_knoten_realdb_test.go:150). Rot im Push-Lauf von d75ba6ca, grün
   im PR-Lauf. Ursache finden, Test robust machen, nicht abschwächen.
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
