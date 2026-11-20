# Übergabe für die nächste Sitzung

**Zuerst lesen.** Diese Liste wird nach jedem Merge und jedem größeren Schritt aktualisiert,
damit eine Sitzung, die am Nutzungslimit abbricht, ohne Rückfragen weitermachen kann.
Stand: 20.11.2026 ~12:00 UTC, Sitzung „Weiter“ (Zweig `claude/weiter-uw2p42`, alle vier
Repos); davor ~19:50 UTC Sitzung „Übergabe“ (`claude/ubergabe-v2cgv1`), ~18:30 UTC Sitzung
„Weitermachen“ (`claude/weitermachen-qbkbfs`). Auftrag des Betreibers: „Alles erledigen für
dezentrales Quantengeld“.

**Zuletzt (Sitzung „Weiter“, 20.11. ~10:00 UTC):**
- **#598 gemergt (90e0e622): Pilot mit Stichprobe v2 grün** (`HEUTE.md` 3.3h, `DEZENTRAL.md` 16
  Schritt 5). Eingabe `stichprobe_v2`, Beobachter im Vier-Knoten-Test liest im Register jedes
  Knotens `probe_art` und die nachgerechneten Salze nach. Läufe
  [44564851989](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44564851989) und
  [44568310804](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44568310804)
  grün (beide Tests, auch mit Ausfall; Lauf 2: „probe_art 2, 4 Salze“ auf der Kette).
  Gegenprobe (Werkzeug im alten Weg bei verlangtem v2) rot. Am Knoten ändert sich nichts
  (nur Test, Workflow, Doku).
- **C1 läuft mit #602 (a6f4674, Deploy #108, 20.11. 06:58 UTC), davor #599 (cb3b494): Stichtag der Stichprobe v2 ausgerollt.** Deploy #107
  [44584732345](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44584732345)
  freigegeben 18:33 UTC, grün (Höhe 348716 → 348746). Wächter-Zeile: „Stichtag Stichprobe v2:
  Repo 1795116600, laufend 9223372036854775807, jetzt 1795113226 – neuer Stichtag liegt in
  der Zukunft – ausrollen erlaubt“. **Scharf ab 19.11.2026 19:30 UTC (Blockzeit).**
  Freigabe des Betreibers 19.11. ~17:50 UTC: „Stichtag heute so schnell wie möglich“
  (#599 gemergt, cb3b4948, mit Sicherheit und eigener Prüfung).
  - **Belegt scharf:** Wache-Lauf [44601606565](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44601606565)
    (19:45:58 UTC, grün): „Quantengeld ab 1794934800 (aktiv), probe_v2_ab 1795116600
    (aktiv)“. Seit **#600 (fa17e401)** gibt die Wache beide Stichtage als Hinweis aus.
    Aus der Sandbox ist `/api/status` von C1 nicht erreichbar (Netzsperre).
  - Die Wache läuft trotz `*/10` nur alle paar Stunden (GitHub drosselt). Für einen
    Beleg sofort: `wache.yml` per `workflow_dispatch` starten (nur lesend, keine Freigabe).
  - Doku zur Freischaltung: `HEUTE.md` 3.3h und 4, `DEZENTRAL.md` 16 Schritt 6,
    Prüfauftrag S 2 (**#601 gemergt, c9a964dd**).
  - **Nächste Schritte:**
    - ~~Erster Hauptnetz-Token mit `--stichprobe-v2`~~ – erledigt 20.11. (oben). **Vorbereitet, #602 gemergt
      (a6f46747, `DEZENTRAL.md` 16 Schritt 7):** Messverzeichnis auf C1 beim Deploy,
      `quanten-s-ausgabe.yml` mit `weg = abschnitte-v2`, k = 552 bei 2 %. Reihenfolge:
      PR mergen → C1-Deploy (Freigabe `produktion`, legt Verzeichnis und override an;
      im Protokoll `AEQUITAS_QS_MESSQUELLE=/data/quanten`) → Workflow mit `modus = ibm`,
      `weg = abschnitte-v2` (Freigaben `quantenhardware` und `produktion`).
      **C1-Deploy #108 grün** ([44611475140](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44611475140),
      freigegeben 20.11. 06:56 UTC): „Messverzeichnis /srv/aequitas-quanten (10001, 0700) und
      docker-compose.override.yml aus docker-compose.quanten.yml“, im Knoten
      `AEQUITAS_QS_MESSQUELLE=/data/quanten`, C1 nimmt an, Höhe 393266 → 393296.
      **ERSTER TOKEN MIT STICHPROBE V2 IM HAUPTNETZ: GRÜN** (20.11. 09:00–09:16 UTC,
      [44691759501](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44691759501)):
      `ibm_kingston`, Job `dbvvuols2omc7388hnu0`, 1.104 Qubits, Fehler 0/548 (ein Schuss 1/548);
      K = `1a1e0800…6f23edd5`, Deckung 1 AEQ; Stichproben 5m02s nach der Wahl (Finalität);
      Doppelvorlage an Stelle 0 abgewiesen, ehrliche Vorlage an C1 eingelöst, 15m21s.
      Dokumentiert in `HEUTE.md` 3.3i und `DEZENTRAL.md` 16 Schritt 7 (**#603 gemergt, c8b79f9b**).
      Probelauf im Rauschmodell grün: [44605960500](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44605960500).
    - **#604 gemergt (b0b8cb17, 20.11. 10:38 UTC):** Token-Ansicht (`/api/quanten-w/note`)
      nennt `probe_art: 2` bei v2-Tokens (bisherige ohne Feld); Kurzfassungen `HEUTE.md` und
      `DEZENTRAL.md` 1 auf Stand 20.11. **C1-Deploy wartet auf Freigabe `produktion`:**
      [44720986652](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44720986652).
      App-Anzeige („Stichprobe erst nach der Bindung“) gemergt (App #87, 1b0de09c).
    - **Jetzt (Auftrag 20.11.: „Dezentrales Quantengeld muss komplett implementiert werden“):**
      **PR (dieser Stand): getrennte Rollen R1–R3** (`DEZENTRAL.md` 17, `HEUTE.md` 3.3j).
      - Neu: `quanten_s_rollen.go`; Befehle `quanten-s-inhaberin neu|annehmen|einloesen`,
        `quanten-s-annahme`, `quanten-s-ausgabe --fuer-inhaberin`.
      - Ansicht `/api/quanten-w/note`: zusätzlich `wurzel`, `proben`, `d` (nach der
        Bindung), `c` (nach der Wahl). Kein Konsens geändert.
      - Werkzeug in Schritte zerlegt (`quantenSRunde`, `quantenSAusgabeEinreichen`,
        `quantenSBinden`/`Waehlen`, `quantenSAnmeldenUrteil`,
        `quantenSStichprobeV2Laden` je Stelle); bisherige Tests unverändert grün.
      - **#605 gemergt (a90b5386, 20.11. ~12:00 UTC).** Belege: 15 Tests, 24/24 Mutationen rot, `-race` grün. Vier Knoten lokal grün
        (`TestVierKnotenQuantenSRollen(Ausfall)_RealDB`, 51 s / 62 s); die bisherigen
        Vier-Knoten-Wege ebenfalls.
      - Pilot-Workflow: Eingabe `rollen = ja` (mit `abschnitte`, `stichprobe_v2 = ja`).
      - **Pilot `rollen = ja` auf GitHub grün:** [44728392142](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44728392142)
        (k = 1.760, Toleranz 2 %, beide Tests; `DEZENTRAL.md` 17). **C1-Deploy (Ansicht)
        wartet auf Freigabe `produktion`** (deploy-c1.yml nach dem Merge).
      - **Jetzt R4a** (Aufgabe #29): Endpunkt `GET /api/quanten-w/validator/fuer-inhaberin?k=`
        (öffentlich, nur lesend, erst wenn der Token auf der Kette steht, nur v2),
        `quanten-s-inhaberin abholen --k` (ohne Paket des Ausgebers), Vorlage ohne Blätter
        (Stelle holt und prüft sie selbst). Danach R4b: App als Inhaberin (QR-Vorlage).
      - **Danach:** R4 App als Inhaberin; Hauptnetz-Lauf mit getrennten Rollen
        (Workflow); Öffnung aus Anteilen (Verfügbarkeit); y je Validator verschlüsseln.
      - Lokale Postgres für RealDB-Tests: `pg_ctlcluster 16 main start`, Datenbank
        `aeq_pilot`, `DATABASE_URL=postgres://postgres:postgres@localhost:5432/aeq_pilot?sslmode=disable`,
        `AEQUITAS_TPS_BENCH=1`. Vorher `df -h /`; der Go-Cache wuchs bei Mutationen auf
        26 GB (`go clean -cache`).
    - Der zweite Mensch als Validator (Rollen trennen, A5).
    - Ein Gutachter (Prüfauftrag S 2).
- Davor: #597 gemergt (d079bf34, Werkzeug im v2-Weg, 14 Mutationen rot).

**Davor (19.11. ~09:45 UTC):**
- **C1 läuft mit #592** (Deploy #102 [44476883826](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44476883826), grün).
- **#593 gemergt (702f4490): Befund 5i** (`DEZENTRAL.md` 16, Prüfauftrag S 2 und S 8).
  - Eine Annahmestelle, die mit der Inhaberin zusammenarbeitet, lässt sie an beiden
    Stellen durchkommen. Belegt in `s_kette.py` 5i, mit Gegenprobe.
  - Heute ohne Gefahr: Das Werkzeug war in allen Läufen alle Rollen zugleich.
  - Vor einer Ausgabe an Dritte: Rollen trennen, die beiden Stellen müssen einander
    vertrauen.
  - Ausweg spezifiziert in `s_kette.py` 5j (8 Mutationen rot): Stichprobe erst nach der
    Bindung (gesalzener Teil-Hash und Zählung je Abschnitt beim Validator, Öffnung erst
    nach Bindung und Wahl, Kette rechnet `probe_hash_v2` nach).
  - **#594 gemergt (822dc7bb):** Prüfvektor `--vektor-probe-v2` und Go-Bausteine
    `quanten_s_probe_v2.go` (noch nicht eingehängt). Kein C1-Deploy nötig (nichts
    aufgerufen); er kommt mit dem nächsten Merge, der den Knoten ändert.
  - **#595 gemergt (bb5f5cb0): Stichprobe v2 im Dienst** (`DEZENTRAL.md` 16, Schritt 2): Teil-Hash
    und Zählung in der Antwort, Öffnung erst nach Bindung und Wahl **und deren
    Finalität** (eigener Befund); bisherige Aufträge unverändert. 14 Mutationen rot.
  - **#596 gemergt (1cd3dcc3): Kettenregel `probe_art = 2`, schlafend** (`DEZENTRAL.md` 16,
    Schritt 3), mit Deploy-Wächter und `/api/status`. Ganzer v2-Weg im Test eingelöst.
    Wichtig: vor dem Stichtag **abweisen**, nicht übergehen, denn der Blockhash läuft
    über das JSON der Transaktionen.
  - **Nächster Schritt:** Werkzeug im v2-Weg. Danach die Freigabe des Betreibers mit
    Stichtag; nur die Zahl `quantenSProbeV2AbUnix` ändert sich.
- **#592 gemergt (f0ca4d53): Basen automatisch zum Validator** (`DEZENTRAL.md` 15).
  - Endpunkt `POST /api/quanten-w/validator/vormerken` (unterschrieben, Mensch mit
    Deckung, begrenzt: 8 wartend, 8 je Ausgeber und Tag, 32 am Tag).
  - Server: `quanten-messen.sh --warteschlange`; mit `quanten-einrichten.sh
    --automatisch` jede Minute per systemd-Timer, höchstens `QS_JE_TAG` am Tag; der
    Zähler liegt außerhalb des Knoten-Verzeichnisses.
  - Werkzeug: `aequitasd quanten-s-ausgabe --messen-warten DAUER`; ohne Basen zieht es
    sie selbst.
  - Dieselben Basen höchstens einmal am Tag (gegen wiederholte Vormerkungen).
  - Gegenprobe: 36 Mutationen, 35 rot, eine gleichwertig.
  - C1 braucht den Deploy (Freigabe `produktion`; neuer Endpunkt, ohne Messverzeichnis
    weist C1 jede Vormerkung ab).
  - Sandbox-Rest: Ein Heredoc-Fehler hat `/aequitas-quanten.timer` im Wurzelverzeichnis
    der Sandbox angelegt. Das Entfernen hat die Sicherheitsprüfung abgelehnt; die Datei
    ist harmlos und nur in dieser Sandbox.
- **C1 läuft mit #591** (Deploy #101 [44407183836](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44407183836),
  freigegeben, ausgerollt 22:51–22:53 UTC, lebt, Höhe steigt, keine Rücknahme).
- **#591 gemergt (69ec4382, A5-Werkzeug).** Für C1 ändert sich nichts am Knoten (nur
  Skripte, Doku, Geräteangabe in `hardware_ibm.py`).
- **C1 läuft mit #590** (Deploy #100 [44377653613](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44377653613), 20:12 UTC grün).
- **Chip-Pilot 2 % / k = 1.760 grün** ([44388535029](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44388535029),
  `HEUTE.md` 3.3g): 0/3.497 Fehler mit Mehrheit (ein Schuss: 24), beide Tests grün, auch
  mit Ausfall. Befund: Jeder Auftrag wählt seinen Chip selbst; die Messung nannte nur
  den letzten (`ibm_fez` und `ibm_marrakesh` belegt). Jetzt steht das Gerät in jeder Zeile.
- **#591 (A5-Werkzeug für den Server eines Validators, `DEZENTRAL.md` 14):**
  `deploy/validator/quanten-einrichten.sh`, `quanten-messen.sh`,
  `docker-compose.quanten.yml`, Test `scripts/test_quanten_messen_skript.sh` (in CI),
  Anleitung `docs/VALIDATOR_EINRICHTEN.md` „Quantengeld“. Danach hängt A5 nur noch am
  zweiten Menschen.
- Davor: **C1 lief mit #588** (Deploy #99, 17:54 UTC grün). Davor #587 (Deploy #98, 16:09 UTC; perf-c1 Lauf 4 grün). Damit sind auch
  #580 (S-Ansicht nennt v) und #585 (Abschnitts-Dienst) auf C1. C1 präpariert Abschnitte
  erst mit `AEQUITAS_QS_MESSQUELLE`; ohne weist es jeden Abschnitts-Auftrag ab (Vorgabe).
- **Auftrag 18.11.: „Bring das dezentrale Quantengeld komplett zu laufen, ggf. mit eigener
  Forschung“.** Plan und Stand in `forschung/quantengeld/DEZENTRAL.md`.
  - **#582 gemergt: Befund und A1.** Die Ausgabe präparierte alle Stellen in einem
    Auftrag. Weg A: Präparation je Validator, spezifiziert in `s_kette.py`, Selbsttest 5.
  - **#583 gemergt (A2-Entwurf, Stellen-Bit):**
    - Befund: Die Stichproben müssen bis zur Bindung geheim sein (5g).
    - Lösung: das Stellen-Bit im Salz (5h), Kette unverändert.
    - Entwurf des Dienstes in `DEZENTRAL.md` 8.
  - **#584 gemergt (A2a, Bausteine in Go):** `quanten_s_abschnitt.go` mit Test und
    Prüfvektor aus der Spezifikation.
  - **#585 gemergt (A2b Dienst):** Endpunkte `abschnitt` und `oeffnung`, Bestätigung
    mit Bündeln, Messquelle `AEQUITAS_QS_MESSQUELLE` (`DEZENTRAL.md` 10).
  - **#586 gemergt (A3 Werkzeug im Abschnitts-Modus):** Eingabe `basen` →
    Abschnitts-Modus. Endpunkt `schluessel` (`DEZENTRAL.md` 11).
  - **#587 gemergt (A4 Pilot im Abschnitts-Modus):** `--s-abschnitte`, Vier-Knoten-Tests
    im Abschnitts-Modus, Workflow-Wahl `ausgabe`.
  - **Erste Pilotläufe im Abschnitts-Modus (`HEUTE.md` 3.3e):**
    - Rauschmodell [44340478883](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44340478883)
      und **IBM `ibm_marrakesh`** [44341842380](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44341842380):
      Test ohne Ausfall **grün**. Je Validator ein eigener Auftrag, Doppelvorlage
      abgewiesen, ehrliche eingelöst, 4 Knoten gleich, Chip-Fehler 0,79 %.
    - Ausfall-Test in beiden **rot** nach 15 s: „keine Messung zu diesen Basen“. Ursache
      im Pilot: Beide Tests lasen dieselbe Messung, der Dienst gibt jeden Abschnitt nur
      einmal her (richtig so).
  - **#588 gemergt (799f6783, Pilot mit zwei Ausgaben + A5 vorbereitet):**
    - `--ausgaben 2`, je Test eine Ausgabe. Nachlauf im Rauschmodell
      [44347416357](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44347416357):
      **beide Tests grün**, auch mit Ausfall eines Validators.
    - A5 vorbereitet (`DEZENTRAL.md` 13): Messquelle als Verzeichnis (je Messung eine Datei
      `<sha256(y)>.json`, der Dienst entfernt vor Verwendung) und
      `hardware_ibm.py --s-abschnitt VERZEICHNIS --basen Y` für einen Validator im Betrieb.
  - **IBM-Nachlauf [44356741223](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44356741223)
    auf `ibm_fez`: beide Tests grün**, auch mit Ausfall eines Validators (Fehler
    0,52–0,68 %). **A4 fertig.** Auffällig: im Test „alle vier“ 192 von 392 Einreichungen
    abgewiesen (Doppelarbeit, nichts falsch verbucht), 772–845 Stellen/min.
  - **#589 gemergt** (f8736ffb, nur Doku: IBM-Nachlauf grün, A4 fertig).
  - **#590 gemergt (3076da22).** Nach dem Merge braucht C1 den Deploy (Freigabe
    `produktion`); für C1 ändert sich nur das Werkzeug (Probe, `--toleranz`).
  - **Auftrag 18:40 UTC: „Es muss zu 99,9999999 Prozent safe sein.“** Im selben PR #590:
    exakte Probe der ehrlichen Vorlage vor jeder Ausgabe (`quantenSEhrlichBesteht`, beide
    Stellen, jede Toleranz), Toleranz wählbar (`--toleranz`; 2 % → k = 1.760). Fälschung
    bleibt ≤ 2⁻⁴⁰. Pilot 2 % / k 1.760 im Rauschmodell grün (`HEUTE.md` 3.3g).
  - **Auftrag 18:00 UTC: „Die Fehlerquote muss deutlich gesenkt werden.“** PR (dieser Stand):
    `hardware_ibm.py --wiederholen R` (Standard 5): je Stelle R Schüsse in derselben Basis y,
    die Mehrheit entscheidet. Rauschmodell: ein Schuss 0,30–0,54 %, Mehrheit aus 5 je
    0/≈2.000. Bei falscher Basis bleibt x Zufall (geprüft). Die Qubit-Wahl war schon
    optimal (`HEUTE.md` 3.3f). Chip `ibm_fez`: 4/10.393 statt 82/10.393 (0,04 % statt
    0,79 %); Vier-Knoten-Pilot im Rauschmodell und auf dem Chip grün, auch mit Ausfall.
  - **Nächste Schritte:** A5 (zweiter Mensch als Validator mit eigenem Server und
    Quantenkonto) ist Sache des Betreibers. Zu beobachten: Doppelarbeit beim Offenlegen
    (Zähler „abgewiesen“), falls die Rate im Betrieb knapp wird.
- **App 1.25.0 (App #86 gemergt, a5f5cf44):** Netto-Zeile und Zahlen der Tagesrunde (#84),
  t von v Validatoren (#85).
  - iOS-Build [44342709659](https://github.com/hanoi96international-gif/Aequitas-App/actions/runs/44342709659) grün.
  - **Veröffentlicht:** APK als Release `v1.25.0`
    ([44360909681](https://github.com/hanoi96international-gif/Aequitas-App/actions/runs/44360909681), 17:52 UTC),
    iOS an dasselbe Release und die SideStore-Quelle
    ([44364337989](https://github.com/hanoi96international-gif/Aequitas-App/actions/runs/44364337989), 18:01 UTC).
- **#581 gemergt (eigene Antworten W7/W8, nur Doku):**
  - Falsche Anteile weist die Kette als Ganzes ab.
  - v − t + 1 Validatoren können blockieren. Bei W kann das die letzte Einlösung
    zunichtemachen; der Wert geht dann ans Grundeinkommen.
  - Restlücke beim Register je Zeile.
  - Im Prüfauftrag und in `OFFEN.md` D.4.
- **#580 und App #85 gemergt.** Die App zeigt bei Note und Token „t von v Validatoren“, bei
  v = 1 einen Hinweis: ein Validator kennt das ganze Geheimnis.
  - Wirksam erst mit dem C1-Deploy von #580 und einem App-Release; beides gibt der
    Betreiber frei.
  - Die App kommt ohne `v` vom Knoten aus: dann zeigt sie die Zeile nicht.
- **#579 gemergt (Prüfauftrag auf Stand 18.11.):**
  - `docs/QUANTENGELD_PRUEFAUFTRAG.md` hat einen Abschnitt „Vorab“: W und S laufen seit
    17.11. 17:00 UTC im Hauptnetz, vor dem Gutachten.
  - Neue Fragen: Betrieb mit einem Validator (W2, S8), Offenlegung durch Ersatz und
    Zuschneiden (W8), Register je Zeile (W7).
  - `OFFEN.md` und `ANALYSE.md`: W6/S5 „anders gekommen“, die Burst-Aussage ist korrigiert.
  - Kopfkommentare der Go-Dateien: „freigeschaltet“ statt „schlafend“, nur Kommentare.
- **Gemergt bis #578.**
  - Pilot S mit Netzlaufzeit `latenz_ms`: 40 und 100 ms je Richtung grün.
  - IBM-Chip `ibm_kingston` grün: 0,59 % Fehler, Doppelvorlage abgewiesen, ehrliche eingelöst.
  - Zähler im Validator-Dienst.
  - Genaues Zuschneiden der Teile: gleich schnell, 38 % weniger Einreichungen.
  - Schlange je Modus.
  - Alle Läufe, auch fehlgeschlagene und zurückgenommene, stehen in `HEUTE.md` 3.3d.
- **Offenlegung:** rund 1.000 Stellen/min. 5.200 Stellen brauchen 5–8 min, die Kettenfrist
  ist 30 min.
  - Engpass ist die Abfolge je Teil (Teil im Stand, dann der nächste; 1 s Nachlauf je
    Nachbarrunde). Laufzeit und Burst-Grenze sind es nicht, beides gemessen.
  - Bewusst nicht weiter beschleunigt. Wird die Frist im Betrieb knapp, ist der nächste
    Hebel das Vorausholen der nächsten Anteile.
  - Der Versuch „Ersatz tritt zurück“ war schlechter und wurde zurückgenommen.
- **Wartet auf den Betreiber:**
  - Gutachter suchen. Der Prüfauftrag ist auf dem Stand vom 18.11.
  - Freigaben: C1-Deploy von #590 (`produktion`). Auf Wunsch: Chip-Pilot mit 2 % und
    k = 1.760 (`quantenhardware`), das ist noch nicht gelaufen.
  - A5: ein zweiter Mensch als Validator mit eigenem Server und eigenem Quantenkonto;
    Hardware-Partner (QKD) für die Messung der Inhaberin.
- **Offen bei Quantengeld (`OFFEN.md` D.4):** Server im echten Netz statt Knoten in einem
  Job, und das Gutachten (Betreiber).
- **Messen von hier:** Zusammenfassung und Artefakte sind gesperrt. Das Ergebnis steht am
  Protokollende (`get_job_logs`, höchstens 5.000 Zeilen). Die Zähler je Dienst stehen im
  Ergebnisblock.
- Wirtschaft: Analyse in `docs/WIRTSCHAFT_STARTBETRAG.md`. Offen sind die Vorschläge in
  Abschnitt 5: Staffel scharf schalten, feste Beträge als Vielfache.

## Arbeitsweise (vom Betreiber festgelegt)

- Antworten auf Deutsch. Bei allem, was eine Freigabe braucht, den Link schicken.
  Mergen ist vorab freigegeben.
- Nie Geheimnisse im Chat erfragen (GitHub-Secrets oder Credential-Formular). Keine
  Impressum-Daten erfinden.
- Push nur auf den Zweig, den die Sitzung vorgibt (alle vier Repos; zuletzt
  `claude/weiter-uw2p42`, davor `claude/weiter-8vzgu9`). Nach einem
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

## Hauptaugenmerk: dezentrales Quantengeld (Stand 17.11. ~17:35 UTC)

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
- Stichtag mit #570 auf **17.11. 17:00 UTC** vorgezogen, auf C1 ausgerollt.
- **ERLEDIGT 17.11. 17:32 UTC: erster S-Token im Hauptnetz** (Lauf 44132434033, `ibm_fez`,
  1/794 Fehler): Doppelvorlage abgewiesen, ehrliche Vorlage eingelöst. Details in
  `forschung/quantengeld/HEUTE.md` 3.3c.
- Der erste Versuch scheiterte, weil die **Signieradresse von C1**
  (`0x3066639af1653325100c1074a9696612dbdc42dc`) keine Deckung hatte. `0x0be8…d016` ist die
  Betreiber-Wallet, nicht C1. Der Betreiber hat 2 AEQ an C1 überwiesen (1 AEQ bleibt dort).
- **Stand 17.11. ~19:50 UTC (Sitzung „Übergabe“, Nutzungslimit fast erreicht):**
  - **PR #572 offen** (Zweig `claude/ubergabe-v2cgv1`): Sicherheitsprüfung als Kommentar
    (keine Einwände). CI abwarten, dann Squash-Merge mit voller SHA, danach C1-Deploy
    freigeben lassen. Inhalt: Pilot-Workflow, Ratengrenze C1, Fuß 15.11.2026,
    `kohaerent.py`, `sdp_nachpruefung.py`, Teil-Kürzung, Rest-Wiederverwendung.
  - **Befund und Korrektur (in #572):** Bei voller Tabellengröße (k = 5.200, 3 von 4) war ein
    Teil der Offenlegung (64 Stellen, 14 Pfadknoten) > 64 KiB (`zumLeiterRumpfHoechstens`)
    → 413, Offenlegung blieb bei 0. Jetzt kürzt der Dienst Teile (`quantenWTeilKuerzen`) und
    verwendet den Rest wieder (`restMerken/restNehmen`). Keine Grenze gelockert, keine
    Kettenregel geändert.
  - **Strenge Schranke (`kohaerent.py`):** Gegen jeden verschränkten und adaptiven Angriff
    hält die Tabelle 2⁻⁴⁰ mit 27–43 Bit Reserve; exakte SDPs bestätigen 1 und 2 Stellen
    (`sdp_nachpruefung.py`, braucht cvxpy). Möglich nach Gutachten: k 550 statt 840.
  - **Pilot lokal in voller Größe** lief zuletzt (Präfix `aeq_pw_`): Offenlegung kommt voran.
    Gemessen mit Kürzung, Rest und eigener Adresse je Knoten: ~224 Stellen/min, also
    ~23 min für 5.200 Stellen > 15 min Frist des Werkzeugs (Kette: 30 min). Vermuteter
    Engpass: Burst-Grenze beim Einreichen (`burstQuantenWJeIP` = 20/min je IP) bei
    ~32 Stellen je Teil -- NICHT lockern. Nächste Schritte: (1) mit Diagnose messen,
    welche Grenze bindet; (2) mehr Stellen je Einreichung ohne Grenzänderung (z. B. Teile
    nicht halbieren, sondern auf das Budget genau zuschneiden); (3) Frist des Werkzeugs
    für volle Größe an die Kettenfrist anlehnen (25 min), im Test ebenso.
    **Nie zwei Vier-Knoten-Tests gleichzeitig mit demselben Präfix** (`DROP ... FORCE`).
  - Nach dem Merge: `quanten-s-pilot.yml` mit `modus=rauschen` selbst starten; `modus=ibm`
    braucht die Freigabe in `quantenhardware` (k = 5.200 → 10.400 Qubits).
  - App 1.24.1 komplett veröffentlicht (APK und IPA).
- **Stand 17.11. ~18:45 UTC (Sitzung „Übergabe“):**
  - **#571 gemergt** (Website `/quanten`, S mit mehreren Validatoren); Sicherheitsprüfung als
    PR-Kommentar. **C1-Deploy wartet auf Freigabe:**
    [44147906409](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44147906409).
  - **Gegenprobe t = 4 bei Ausfall:** erster Lauf ungültig (zwei Vier-Knoten-Tests liefen
    gleichzeitig auf denselben Datenbanken `aeq_vier_0..3`). **Zwei Vier-Knoten-Tests nie
    gleichzeitig ohne eigenes `AEQUITAS_VIER_DB_PRAEFIX` laufen lassen** -- der zweite
    löscht dem ersten die Datenbanken (`DROP ... WITH (FORCE)`), die Knoten laufen
    auseinander (tausende StateRoot-Abweichungen). Wiederholung allein: Ergebnis als
    Kommentar an #571.
  - **Pilot mit vier Validatoren auf echter Hardware** (PR dieser Sitzung):
    `quanten-s-pilot.yml` misst auf IBM (`quantenhardware`) oder im Rauschmodell und lässt
    vier echte Knoten im Job prägen, Doppelvorlage abweisen, einlösen (3 von 4, mit und ohne
    Ausfall). Kein Server, keine Server-Schlüssel. Der Vier-Knoten-Test liest die Messung
    aus `AEQUITAS_QS_MESSUNG`. Lokal mit 1.680 Stellen grün (420 s, 3/831 Fehler, vier
    Knoten gleich). Ehrliche Grenze: 840 Stellen je Stelle, die Tabelle verlangt für 3 von 4
    5.200 -- Ablauf, nicht volle Sicherheitsstufe. Nach dem Merge: Modus `rauschen` selbst
    starten, Modus `ibm` braucht die Freigabe in `quantenhardware`.
  - **Ratengrenze C1:** `perf-c1-setzen.sh` setzt `AEQUITAS_RPC_RATE_LIMIT_MAX` jetzt leer
    (Vorgabe 200 je 10 s) und bricht ab, wenn der Knoten noch eine Anhebung meldet (im
    selben PR, Test `TestRPCRatengrenzeAusUmgebung`). Wirksam erst, wenn der Betreiber
    `perf-c1.yml` startet und freigibt. Vor C1 liegt nur Caddy (kein CDN), die Vorgabe
    trifft also je Nutzer. `AEQUITAS_RPC_QUIET_TX=1` bleibt (nur Protokollzeilen, keine
    Schutzgrenze; steht auch in `docker-compose.yml`).
  - **App 1.24.1:** #83 gemergt. APK-Bau
    [44146926619](https://github.com/hanoi96international-gif/Aequitas-App/actions/runs/44146926619),
    iOS-Bau [44146933331](https://github.com/hanoi96international-gif/Aequitas-App/actions/runs/44146933331)
    (erster Versuch rot: Netzabbruch beim Laden von hermes-engine von Maven, neu gestartet).
    Danach `release-apk.yml` (run_id, Tag `v1.24.1`) und `release-ios.yml` -- Freigaben
    Betreiber. iOS-Release 1.24.0 wartet ebenfalls auf Freigabe.

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
5. **Analyse der Wirtschaftsregeln (Wunsch des Betreibers, 17.11.):** Erster Teil erledigt:
   `docs/WIRTSCHAFT_STARTBETRAG.md`. Frage 100 statt 1.000 AEQ: bei 1.000 bleiben (nominal
   neutral, g/P = y/V; Umstellung kostet Konsensänderung und Umrechnung aller Bestände).
   Vorschläge dort: Staffel scharf schalten, sobald ihre Voraussetzungen stehen, App-Anzeige
   netto und als Anteil am fairen Anteil, feste Beträge als Vielfache. Bisheriger Text: Umlaufabgabe seit dem
   Gleichgewicht (07.11.2026): 0,5 %/Monat auf das ganze Guthaben, ohne Freibetrag, täglich
   anteilig, unter 0,001 AEQ je Tag nicht eingezogen, alles ins Grundeinkommen. Der alte
   Freibetrag (5 × 1.000 AEQ) steht noch im Code, gilt aber nur vor dem Stichtag. Prüfen:
   Wirkung, Fairness, Umgehung, Anzeige. Idee: In der App Grundeinkommen und Abgabe desselben
   Durchlaufs in einer Zeile zeigen (bei einem einzigen Menschen heben sie sich auf). Erst
   nach dem Quantengeld.
6. **Ältere offene Aufgaben:**
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
