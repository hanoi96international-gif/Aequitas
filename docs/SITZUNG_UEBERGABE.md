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

## Hauptaugenmerk: dezentrales Quantengeld (Stand 16.11. ~18:30 UTC)

Auftrag des Betreibers: Dezentrales Quantengeld soll auf heutigem Stand funktionieren,
damit Aequitas ernstzunehmendes Quantengeld ist. Die Liste steht in
`forschung/quantengeld/OFFEN.md`.

**Neu seit #544 (PR #545, Zweig `claude/friendly-davinci-i4iww1`):**
1. **Befund Klonangriff:**
   - Der optimale Kloner macht zwei Kopien mit je 14,6 % Fehler. Damit ist die
     Toleranz von 15 % gebrochen: Eine geklonte Kopie besteht bei k = 64 mit 0,55.
   - Rechner: `wiesner_parameter.py`.
   - Sicher bei 0,7 % Hardware-Fehler: Toleranz 4 %, k = 840.
   - Die Kettenseite schläft, es gibt keinen Schaden. **Vor jeder Freischaltung müssen
     die Grenzen neu gesetzt werden.**
2. **Variante S (ANALYSE.md 5b):**
   - S-Token nach Kent u. a. 2022: ohne Quantenspeicher, unfälschbar, privat, dezentral
     prüfbar. Das ist das Quantengeld, das heute geht – aber kein Hand-zu-Hand-Bargeld.
   - Doppelvorlage per Breidbart-Messung: ebenfalls 14,6 % Fehler.

**Nächste Schritte, in dieser Reihenfolge:**
1. #545 nach grüner CI mergen.
2. **Neue Grenzen in Spezifikation und Knoten:**
   - Toleranz an e gekoppelt (z. B. 4 %), k bis ~1000+, `N_MAX` größer.
   - Offenlegen in Teilen (Pfade und Anteile für ~1000 Stellen passen nicht in eine
     Transaktion).
   - Prüfvektor neu erzeugen, Missbrauchstests.
   - Eigener PR mit Sicherheitsprüfung.
3. **Variante S als Spezifikation (`s_kette.py`):**
   - Ausgabe, maskierte Basen, Vorlage, Offenlegen, Urteil.
   - Missbrauch: Doppelvorlage, fremde Vorlage, Wiederholung.
   - Danach die S-Vorführung auf IBM (Teil 1 = vorhandener Schaltkreis ohne Warten).
4. **Anschreiben Vorlage 4** an Kent und Pitalúa-García (Cambridge) und Rarity (Bristol).

**Achtung, Arbeit einer anderen Sitzung:**
- Die Sitzung „Weiter“ hat einen Validator-Dienst begonnen: `quanten_w_validator.go`
  (Tresor, Prägebestätigung, Anteilsherausgabe, Ende-zu-Ende-Test mit vier Knoten,
  28 Mutationen).
- Bis 17:41 UTC war er **nicht gepusht**; `claude/weiter-8vzgu9` endet bei der
  Übergabe zu #543.
- Wenn jene Sitzung weiterläuft: zuerst committen und pushen. Nicht doppelt bauen.

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
