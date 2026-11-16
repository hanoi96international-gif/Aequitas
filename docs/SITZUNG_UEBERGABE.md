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
| Aequitas | #541 | Variante W: Regeln geschärft (W2b Teil 1) — Zufall aus Hash-Ketten, feste Prüfgröße beim Einlösen, falsches Einlösen tötet die Note nicht, keine Zufallsfrist, Prägezeit in der Bestätigung | CI abwarten, Sicherheitsprüfung steht als Kommentar → squash-mergen |

## In Arbeit: W2b Teil 2 — Variante W in die Kette einhängen (schlafend)

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
- `quantenWTopfAddr`: Deckung, Anteilsmodell wie der Quantengeld-Topf, Umlaufabgabe über
  `istNotenTopf`.
- `quantenWPfandAddr`: offene Pfänder, ohne Abgabe.
- Was verfällt, geht an `ubiPoolAddr`.

**Register:**
- Liegt in chain_config unter `quanten_w` als JSON. Höchstens 64 lebende Noten und 1024 tote.
  Tote Noten bleiben gemerkt, solange ihre Bestätigung gelten könnte.
- StateRoot-Teil `|quantenw:` + Hash, nur wenn das Register nicht leer ist.

**Block:** nennt die gerechneten Felder (Anteile, Pfand, PfandAn, Wert, WertAn, Urteil).
Jeder Knoten rechnet nach.

**Eingehängt (lokal, baut, noch ohne Tests):**
- `bekannteTxArt`, Replay-Fall und `quantenWFeldErlaubt` in block.go
- block_tauglich.go (Zeitfenster)
- `blockTouchedAddresses`, `isTokenomicsPoolAddress`, StateRoot, Rückroll-Snapshot und
  Laden beim Start in state.go
- `registerSicherung`
- `umlaufArt`/`umlaufZahlt` (`istNotenTopf`)
- `pruefeTopfNurMitNote`
- `replay_parallel` (seriell)
- snapshot.go (Export, Import, Resync), checkpoint_tx.go (`pruefeSnapshotKonfig`)
- api.go: `POST /api/quanten-w/<art>`, `GET /api/quanten-w/note?k=`
- Transaction-Feld `QuantenW` (`omitempty`)

**Fehlt noch:**
1. Tests in `quanten_w_kette_test.go`:
   - Ablauf: schlafend (409, unbekannte Art, StateRoot byte-gleich) → Gutfall von Annahme
     bis Nachspielen auf zweitem Knoten, mit gleicher StateRoot und Erhaltung.
   - Prägen: fremder Validator, gleicher Mensch zweimal, Bestätigung fehlt, alte Prägezeit,
     tote Note neu prägen, Register voll.
   - Block mit falschen gerechneten Feldern; W-Angaben an einer Überweisung; Überweisung an
     die W-Töpfe; Registerlesen mit kaputtem JSON.
   - Test-Helfer: `quantenWAbOverride`, `quantenWBetreiberVon` überschreiben,
     `qwPraegen` aus `quanten_w_test.go`.
2. Volle Keeper-Tests (`go test ./x/humanity/keeper/`, `-race`), gofmt, vet.
3. PR mit Sicherheitsabschnitt, eigene Sicherheitsprüfung als Kommentar, Mutationen.
4. Danach ANALYSE.md: in der Stufentabelle W2b auf „fertig“ setzen.

## Danach (Reihenfolge)

1. **W5 (Aufgabe #64):** `docs/QUANTENGELD_PRUEFAUFTRAG.md` um Variante W erweitern
   (Hash-Ketten-Zufall, Pfandmodell, Grenzen: alle v Validatoren nötig, Schwellen-Signatur
   später).
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
