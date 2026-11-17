# Übergabe für die nächste Sitzung

**Zuerst lesen.** Diese Liste wird nach jedem Merge und jedem größeren Schritt aktualisiert,
damit eine Sitzung, die am Nutzungslimit abbricht, ohne Rückfragen weitermachen kann.
Stand: 17.11.2026 ~16 UTC (S-Ausgabe-Werkzeug im PR), Sitzung „Weitermachen“ (Zweig
`claude/weitermachen-qbkbfs`); davor ~10 UTC nach #566 und App #79, Sitzung „Weiter“ (Auftrag des Betreibers: „Alles erledigen
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

## Hauptaugenmerk: dezentrales Quantengeld (Stand 17.11. ~16 UTC)

**Gemergt (#563):** Register von W und S als eigene Tabelle `quanten_w_eintraege`
(`quanten_w_tabelle.go`): eine Zeile je Note, Token und toter Note, nur Geändertes
wird geschrieben, StateRoot und Snapshot-Format unverändert, Umzug aus `chain_config`
beim Laden. Dazu im selben PR die Rechnung zu **B5** (`wiesner_mehrstufig.py`):
zwei Stufen sparen einer echten Note im Mittel 20–52 % Stellen bei unverändert 2⁻⁴⁰
(Vergleich bei gleichem e; mit der Garantie der heutigen Prüfung sind es 16–31 %, siehe
unten).
**W2e entschieden (17.11., `wiesner_mehrstufig.py`):** nicht jetzt bauen.
- **Gerechnet mit der Garantie, die die feste Prüfung heute gibt:** Eine volle Prüfung in
  zwei Stufen braucht im Mittel 16–31 % weniger Stellen. Dafür müssen k_max = 1,5–1,9 ×
  k_note frei sein, und 3–11 % der Prüfungen brauchen eine zweite Runde.
- Beim **Einlösen** hilft es der Lebensdauer nicht, Einlösen ist das Letzte einer Note.
- Es ist eine Optimierung, keine Sicherheitsfrage. Bauen, wenn die Stellen je Note im
  Pilot der Engpass sind.
- **Was bereitliegt:**
  - In `wiesner_mehrstufig.py` (Docstring und `w2e_*`): die Grenzen, ganzzahlig und zur
    sicheren Seite gerundet, und die Tabelle.
  - Ablauf-Entwurf und Missbrauchsfälle: PR #565, Abschnitt „Entwurf W2e“ in dieser Datei.
    Der Entwurf gilt dort fürs Einlösen; sinnvoll ist er aber für die volle Prüfung, mit e
    aus der heutigen Garantie statt einem neuen Feld.

**App-Anzeige für W und S erledigt (App #79, gemergt 17.11.):**
- Auf der Quantengeld-Seite „Note oder Token nachsehen“: Kennung K eingeben, dann den
  Stand von `GET /api/quanten-w/note` anzeigen. Gilt für W-Note, S-Token, tot, unbekannt.
- Nur Anzeige; die Antwort wird streng gelesen (`lib/quantenW.ts`). Texte in 12 Sprachen.
- Noch nicht in einer veröffentlichten Version. Kommt mit dem nächsten App-Release, das
  braucht die Freigabe des Betreibers.
- `docs.expo.dev` ist in der Umgebung erlaubt, war aber in der Sitzung noch nicht
  erreichbar. Geprüft wurde gegen die installierten Pakete (Expo 54.0.35, expo-router
  6.0.24, React Native 0.81.5).

**W/S im Hauptnetz freigeschaltet (Entscheidung des Betreibers, 17.11., #568 gemergt):**
- Stichtag **17.11.2026 17:00 UTC** (`quantenWAbUnix = 1794934800`), auf Wunsch des Betreibers vom 18.11. 18:00 vorgezogen.
- **Deploy auf C1 wartet auf die Freigabe des Betreibers:**
  [44092755346](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44092755346).
  Nach dem Deploy prüfen: `/api/status` zeigt `quanten_w.ab = 1795024800`.
- Ohne Gutachten und ohne Pilot mit Partner; die Risiken sind im Code und im PR genannt.
- Deploy-Wächter `.github/neustart/quanten-w-nicht-verpasst.sh` mit Test; `/api/status`
  nennt `quanten_w.ab`.
- C1 muss vor 17:30 UTC am 18.11. mit diesem Stand laufen, sonst hält der Wächter an.
- Danach: erste S-Ausgabe auf dem Hauptnetz über einen Workflow, mit IBM als Gerät des
  Halters.

**Erste S-Ausgabe: Werkzeug gebaut (PR „Quantengeld S: Werkzeug für die erste Ausgabe“).**
Eine frühere Sitzung hatte das schon lokal gebaut, aber nie gepusht; der Container war weg.
Neu gebaut, darum: **lokale Arbeit sofort committen und pushen.**
- `aequitasd quanten-s-ausgabe` (`x/humanity/keeper/quanten_s_ausgabe.go`): C1 ist mit
  seinem Knotenschlüssel Ausgeber und einziger Validator (t = v = 1, k = 840 je Stelle,
  Toleranz 4 %). Inhaberin und die zweite Stelle sind Wegwerfschlüssel, nur im Speicher.
  Erst die Doppelvorlage an der falschen Stelle (muss abgewiesen werden), dann die
  ehrliche an C1 (der Wert kommt zurück). Das Urteil kommt aus dem Stand, die
  Bestätigung des Dienstes wird nachgeprüft.
- Prüfer-Dienst: Aufträge auch von einer an einen Menschen gebundenen Signieradresse mit
  Deckung (`quantenWPraegerTauglich`), nicht nur vom Menschen selbst.
- `hardware_ibm.py --s-messen DATEI`, `deploy/quanten-s-ausgabe.sh` (läuft als UID 10001
  im Container, Messung nur über stdin, danach gelöscht) und der Workflow
  `quanten-s-ausgabe.yml` (Mess-Job in `quantenhardware`, Ausgabe in `produktion`). Der
  Linter `lint_server_freigabe.py` erlaubt dafür in `quantenhardware` genau
  `secrets.IBM_QUANTUM_TOKEN`.
- Geprüft: Rauschmodell `ibm_marrakesh` (2/847 Fehler) durch die volle Kette in-process,
  Doppelvorlage abgewiesen, ehrliche eingelöst. Gegenprobe: 22 Mutationen, alle rot.
- **Stand 17.11. ~16:05 UTC:** #569 auf C1 ausgerollt (Lauf 44118612415). #570 zieht den
  Stichtag auf **17.11. 17:00 UTC** vor; Deploy [44121195729](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44121195729)
  muss bis 16:30 UTC freigegeben sein, sonst hält der Wächter an (dann neuen Stichtag wählen).
- Signieradresse von C1: `0x0be8b961cbf6564bd1931b0803d35c0659e0d016` (hat ~1000 AEQ aus der
  Registrierung, Deckung reicht).
- **Starten (Betreiber), ab 17:00 UTC:** Workflow `quanten-s-ausgabe.yml` mit `modus = ibm`,
  beide Jobs freigeben. Danach Ergebnis (Doppelvorlage abgewiesen, ehrliche eingelöst) hier
  und in `forschung/quantengeld/HEUTE.md` festhalten.

**Als Nächstes:** Voraussetzungen von W6 (für eine Freischaltung mit Gutachten).
- Gutachten: Vorlagen in `ANSCHREIBEN.md`, verschickt der Betreiber.
- Pilot im Testnetz: mehrere Validatoren mit Dienst, Prägen bis Einlösen auf echter
  Hardware.

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
   Knoten für den Dienst~~ (erledigt, siehe oben); Register als Tabelle (vor W6).
   - **Peers aus dem Knoten** (#557, gemergt):
     - Der Dienst fragt die Sync-Peers und Seeds (`BlockDAG.QuantenWNachbarn`) und dazu
       `QUANTEN_W_PEERS` ab. Alle gleichzeitig, mit 5 s Frist und 1 s Nachlauf, sobald
       t beisammen sind.
     - Nachbarn nur öffentlich, über `pinningDialer`, ohne Weiterleitung. Auch die
       Adressen des Betreibers folgen keiner Weiterleitung mehr.
     - Die Anteile-API unterschreibt (`sig`, `quantenWAnteilNachricht`). Von Nachbarn
       zählt nur, was Validator X der Note unterschrieben hat.
     - Offen: Die Liste ist „alle bekannten Knoten“, nicht „die Validatoren der Note“.
       Eine Zuordnung Validator → URL im Register gibt es noch nicht; mit der
       Unterschrift ist das für die Sicherheit nicht nötig, nur für weniger Abrufe.
5. **App:** W- und S-Anzeige.
6. **Ionenfalle** (Konto des Betreibers), Vorlage 4 abschicken (Betreiber).
7. **Gutachten (W5/S4):** Der Prüfauftrag (`docs/QUANTENGELD_PRUEFAUFTRAG.md`) ist auf
   den Stand von W2c, W2d und Variante S gebracht. Vorher fragte er noch nach 15 % und
   k = min(64, frei). Es fehlt nur noch ein Gutachter (Betreiber).
8. **ePrint-Entwurf** (`forschung/quantengeld/EPRINT_ENTWURF.md`, englisch): Varianten W
   und S, Parameter und Klon-Befund, Umsetzung, Ergebnisse auf IBM, offene Fragen. Die
   Literatur ist an arXiv geprüft. Veröffentlichen (IACR ePrint) nur durch den Betreiber.
9. ~~**W3 auf dem echten Chip**~~ – gelaufen (Lauf 43973771243, `ibm_fez`, HEUTE.md 3.3):
   - Ohne Warten 2,4 % Fehler je Treffer, Annahme 0,90; Note Nr. 1 echt.
   - Sicher prüfbar mit Toleranz 6 % und 1.360 Stellen (`wiesner_parameter.suche`).
   - Mit Teleportation 12,5 %: keine sichere Toleranz unter 14,6 %.
   - Ab 20 µs 29–40 %, die Note ist verloren.
   - `wiesner_hardware.TOLERANZ` steht jetzt auf 4 % statt 15 %.
10. **S3 – Ausgabe über eine echte Leitung gerechnet** (ANALYSE.md 5c,
    `wiesner_parameter.py` Abschnitt S3, Prüfauftrag S Fragen 6 und 7):
    - Kent u. a. 2022, QT1 (Lemma 2/3, Satz 1) ist umgesetzt, ihre Abb. 5 nachgerechnet.
      Mutationen: sechs von sechs erkannt.
    - Befund 1: Kents Handsender ist sicher, sobald Basen und Werte aus dem Zufall der
      Validatoren kommen (β = 0).
    - Befund 2: Die Tabelle für ideale Qubits reicht über eine Leitung nicht. Bei 10° und
      3 % käme ein Fälscher in 1,4 % durch. `s3_stichprobe` rechnet mit λ(θ) und dem
      Mehrphotonen-Anteil.
    - **Lücke geschlossen:** Die Größe der Stichprobe prüfte niemand. Jetzt lädt
      `s_kette.stelle_laden` nur mit mindestens k_note Stellen (fail-closed, `labor_k` nur
      für Tests). Mutationen: drei von drei erkannt, Prüfvektor unverändert. Der Knoten
      prüft die Größe bewusst nicht: Beim Anmelden ist die Annahme schon geschehen. Die
      App der Annahmestelle muss dieselbe Regel bekommen.
    - Meldung Λ und öffentliche Teilmenge sind als Bausteine in `s_kette.py` spezifiziert
      (`s3_meldung`, `s3_teilmenge`; Mutationen vier von vier).
    - Offen vor S3 mit Gerät: die Tabelle im Knoten mit θ und dem Anteil freier Stellen,
      dazu die Einbindung der Bausteine in `KetteS` und den Knoten.
13. **Abschnitt C analysiert** (ANALYSE.md 5d, mit Empfehlung, was zuerst kommt):
    - **C1:** Auf einer Kette fällt der relativistische Vorteil weg. Es bleibt
      Unfälschbarkeit ohne Rechenannahme.
    - **C2:** Kommerzielle QKD-Dienste liefern nur Schlüssel. Es braucht ein Testbett
      oder einen Partner.
    - **C4:** Halb-quantisches Geld braucht beim Halter einen Quantencomputer und einen
      Speicher. Beobachten.
    - **C5:** Quantengeld heißen nur W und S, nie „Quantenwährung“.
    - **Offen:** nur C6 (Ionenfalle, Konto des Betreibers).

11. **B6 beantwortet – Fehlerkorrektur je Stelle** (HEUTE.md 3.3, `wiesner_schutz.py`,
    `wiesner_hardware.note_steane`, Workflow `wiesner = steane`):
    - Im Rauschmodell `ibm_marrakesh` ist der Steane-Code bei jeder Wartezeit schlechter
      als ungeschützt: ohne Warten 0,8 % gegen 0,3 %, nach 20 µs 9,8 % gegen 3,0 %.
    - Exakt gerechnet hilft er mit IBM-Werten nie, mit Werten einer Ionenfalle bei 0,3–6 %
      Rohfehler. Mutationen: sieben von sieben erkannt.
    - Der echte Chip bestätigt es (`ibm_fez`, Lauf 43986013444, 17.11. 06:12 UTC): ohne
      Warten 4,6 % gegen 0,6 %, nach 5 µs 11,4 % gegen 5,3 %. Eingetragen in HEUTE.md
      3.3. B6 ist damit abgeschlossen.
12. **Aufteilung zwischen den Sitzungen** (17.11., Sitzung „friendly-davinci“, Zweig
    `claude/friendly-davinci-i4iww1`; eine Nachricht an die andere Sitzung kam nicht an):
    - **Hier:** S3, B6, Abschnitt C von OFFEN.md als Analyse, S3-Ablauf in `s_kette.py`.
    - **Bei der Sitzung „Weiter“:** Register von W und S als Tabelle, mehrstufige
      Prüfung bei W (B5), App-Anzeige. `quanten_w*.go` und `quanten_s.go` fasst diese
      Sitzung nicht an.

## Offene PRs

| Repo | PR | Inhalt | Nächster Schritt |
|---|---|---|---|
| – | – | keine offenen PRs der Sitzung „Weiter“ | – |

Gemergt am 16./17.11. (diese Sitzung): #561 (B6 Steane-Code je Stelle, Abschnitt C
analysiert, S3-Bausteine), #560 (S3 über eine echte Leitung, Mindestgröße der Stichprobe),
#558.

**Wartet auf Freigabe des Betreibers:**
- Deploy von #557 auf C1 ([43974452277](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43974452277)).

Gemergt am 17.11. (Sitzung „Weiter“):
- #563 (W/S-Register als Tabelle; B5 gerechnet, `wiesner_mehrstufig.py`)
- #559 (Übergabe)

Gemergt am 16.11.:
- #558 (andere Sitzung: Prüfauftrag auf Stand W2c/W2d/S, ePrint-Entwurf, `OFFEN.md`
  aufgeräumt, Prägen ohne Vertrauen auf echtem Chip)
- #557 (Validator-Dienst: Nachbarn des Knotens, gleichzeitig abgefragt, Anteile
  unterschrieben)
- #556 (Übergabe nach #555)
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
Passwort per `ALTER USER postgres PASSWORD 'postgres'`). Liegt schon ein Cluster in
`/home/postgres/pgdata` (Vertrauen ohne Passwort): eine alte `postmaster.pid` löschen, dann
`su postgres -c "/usr/lib/postgresql/16/bin/pg_ctl -D /home/postgres/pgdata -o '-p 5432 -k /tmp' -l /home/postgres/pgdata/pg.log start"`.
Danach

```
AEQUITAS_TPS_BENCH=1 DATABASE_URL="postgres://postgres:postgres@localhost:5432/aeq_ci_realdb?sslmode=disable" \
  go test ./x/humanity/keeper/ -run 'TestVierKnoten_RealDB$' -v -timeout 15m
```

## Danach (Reihenfolge)

0. ~~**App 1.23.2**~~ – vollständig veröffentlicht (APK und IPA, `quelle.json` geprüft).

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
