# Quantengeld heute – was unter den heutigen Umständen funktionieren kann

Stand 16.11.2026, Kurzfassung aktualisiert am 20.11.2026. Ergänzt `ANALYSE.md` um die Frage: **Was davon kann Aequitas heute
schon sein, und was fehlt dafür noch?** Grundlage sind die Messungen in diesem Ordner,
der Code im Knoten und in der App sowie die Quellen in `ANALYSE.md`.

## Die Antwort in Kürze

1. **Echtes Quantengeld** (die Noten selbst sind Quantenzustände, und Menschen tragen
   sie bei sich) **funktioniert heute nirgends auf der Welt.** Das liegt nicht an
   Aequitas. Es gibt weder Quantenspeicher in Geräten noch Quantenleitungen zu
   Endnutzern. Wer es verspricht, verspricht etwas, das niemand liefern kann. Unsere
   eigene Regel lautet deshalb: Wir werben nie mit „Quantenwährung“
   (`docs/QUANTENSICHERHEIT.md`).
2. **Heute funktionieren können vier Dinge, und Aequitas kann alle vier sein:**
   - **(a) quantensicheres Geld für jeden, ab Werk:** Kein Quantenrechner kann AEQ
     stehlen oder Menschen erfinden;
   - **(b) die erste laufende Ausgabestelle für Quantengeld:** Register, Deckung,
     Abgabe, Freischaltung, Einlösen;
   - **(c) echte Quanten-Noten von Anfang bis Ende auf echter Hardware,** als
     Vorführung im Labormaßstab;
   - **(d) Bargeld-Verhalten ohne Quanten:** offline zahlen mit Haftung (Offline-Bargeld).
3. **Davon ist (b) fertig, (a) zu großen Teilen.** Für (a) fehlt vor allem, dass der
   Quantenschutz **automatisch** an ist. Heute muss jeder ihn selbst einschalten. (d) ist
   zur Hälfte gebaut.
4. **(c) läuft seit dem 17.11.2026 im Hauptnetz** (Quanten-Token ohne Speicher,
   Variante S):
   - Ausgegeben und eingelöst auf echten IBM-Chips; die Doppelvorlage wurde jedes Mal
     abgewiesen (3.3c).
   - Seit dem **20.11.2026** auch mit **Stichprobe erst nach der Bindung**: Keine
     Annahmestelle kennt vorher eine Stelle der Stichproben (3.3i, Befund 5i).
   - Im Pilot mit **vier Validatoren**, von denen jeder seinen Abschnitt selbst
     präpariert, auch wenn einer ausfällt (3.3d–3.3h).
   - Seit dem **21.11.2026** mit **öffentlichem Nachweis nach dem Ende**: Nach dem
     Einlösen legt der Validator die Präparation offen. Jeder kann nachrechnen, dass
     sie genau die bei der Ausgabe festgelegte war, und die Messfehler zählen. Im
     Hauptnetz: 0 Fehler bei 540 Treffern (3.3k). Das beweist nicht, dass ein
     Quantenrechner gemessen hat.
   - Seit dem 20.11.2026 mit **getrennten Rollen** gebaut: Inhaberin, Ausgeber und
     Annahmestelle haben je einen eigenen Schlüssel, und der Ausgeber sieht x nie. Auf
     vier echten Knoten grün, im Hauptnetz noch nicht gelaufen (3.3j).
   - **Ehrlich:** Es ist eine Vorführung. Präparation und Messung laufen im selben Auftrag
     beim Betreiber. Im Hauptnetz gibt es nur einen Validator (C1), der damit den ganzen
     Token kennt. Es liegt kein Gutachten vor. Echtes Quantengeld in den Händen der
     Menschen gilt weiter nicht (Punkt 1).

## 1. Drei Bedeutungen von „funktionierendem Quantengeld“

| Bedeutung | Was es heißt | Heute möglich? | Aequitas heute |
|---|---|---|---|
| **Quantensicheres Geld** | Geld, das auch ein Quantenrechner nicht stehlen, fälschen oder erfinden kann. Der Schutz kommt aus Mathematik (Post-Quanten-Kryptografie), nicht aus Physik. | **ja**, auf normalen Handys | Stufen 1–3, 3.6, 5, 5b, 1b live; Lücken in Abschnitt 3.1 |
| **Physikalisches Quantengeld** | Die Note ist ein Quantenzustand und lässt sich physikalisch nicht kopieren. | **nein** für den Alltag; **ja** als Vorführung auf Laborhardware | Ausgabestelle live (schlafend); Noten auf IBM gemessen (Z₈, ~20 µs) |
| **Quanten-Zufall** | Zufall aus einem Quantenrechner, dessen Echtheit klassisch geprüft wird (z. B. Quantinuum/JPMorgan, Nature 2025). | ja, bei wenigen Anbietern | nicht genutzt; wäre ein Baustein für faire Auswahl, kein Geld |

## 2. Die harten Grenzen von heute

| Grenze | Beleg | Folge |
|---|---|---|
| Eine Note lebt auf IBM etwa **20 µs**, auf Ionenfallen **Sekunden** | eigene Messung (`HARDWARE_WEG.md`); Datenblatt Quantinuum | Eine Note überlebt keinen Gang von Hand zu Hand |
| Ein Cloud-Lauf endet mit der Messung, nichts **überdauert einen Lauf** | Arbeitsweise aller Cloud-Quantenrechner | Eine Note existiert nur innerhalb **eines** Laufs |
| Ein Cloud-Lauf ist **gegenüber Dritten nicht als echt beweisbar** | Das Ergebnis meldet der Betreiber | Eine „Quanten-Note in der Cloud“ ist nicht sicherer als eine Buchung. Sie zeigt, dass der **Ablauf** funktioniert, schützt aber nichts. Seit dem 20.11. (R7, `DEZENTRAL.md` 17) ist nach dem Einlösen öffentlich nachrechenbar, dass genau diese Präparation bei der Ausgabe festgelegt war und wie viele Messfehler es gab – nicht, dass ein Quantenrechner gemessen hat. |
| Endnutzer haben **keinen Quantenspeicher und keine Quantenleitung** | Speicher > 10 h nur im Labor (Wang 2025); Quantennetze sind Testbetten | Keine Note in der Hosentasche, auf Jahre |
| Öffentliche Prüfung braucht **fehlerkorrigierte Speicher**; Einlösen braucht **iO** | `rauschen_oeffentlich.py`; Huang & Vaikuntanathan 2026, Satz 1.1 | Der Weg der Bauvorgabe liegt in der fehlertoleranten Ära |

**Folgerung:**
- **Physikalisches Quantengeld** ist heute eine **Vorführung**, kein Schutz.
- **Quantensicheres Geld** ist heute ein **echter Schutz**. Dort liegt der größte Nutzen
  für Menschen mit AEQ.

## 3. Was heute funktioniert – und was dafür noch fehlt

### 3.1 Quantensicheres Geld für jeden (größte Wirkung)

**Live:**
- Registrierung hybrid (Stufe 1, scharf nach der ersten echten Registrierung);
- PQ-Zusage je Wallet (Stufe 2);
- PQ-Adressen und Überweisungen mit ML-DSA-65 (Stufe 3, Tresor);
- Menschsein und Grundeinkommen in den Tresor (3.6);
- PQ-Stempel der Blöcke (5);
- Prüfer-Stimmen hybrid (5b);
- PQ-Schlüssel der Prüfer-Dienste (1b).

**Lücken:**

| Lücke | Warum sie zählt | Was zu tun ist | Wer |
|---|---|---|---|
| **Quantenschutz ist freiwillig.** Die App bietet „Quantenschutz aktivieren“ als Karte an, der Tresor ist ein eigener Schritt. | Eine Wallet ohne Zusage ist nach ihrer ersten Überweisung angreifbar, sobald es einen passenden Quantenrechner gibt. Die meisten Menschen schalten nichts selbst ein. | **Ab Werk:** Die App hinterlegt die Zusage nach der Registrierung selbst. Der Schlüssel kommt aus derselben Seed-Phrase, der Mensch muss nichts tun. Für Wallets ohne Seed-Phrase in der App (WalletConnect) bleibt es ein Hinweis. | ich (App), danach dein Release |
| **Stufe 0:** Biometrie-Sicherung hybrid | Gesichtsdaten im Backup wären sonst später entschlüsselbar („jetzt speichern, später knacken“) | PQ-Schlüssel des Betreibers anlegen | du |
| **Stufe 1 scharf** | Seit dem Neustart bei null fehlt die erste echte Registrierung | dich registrieren (App 1.21+) | du |
| **Vom Tresor aus noch nicht möglich:** Prüfer werden, Wächter setzen, Treuhand zurückholen, Unternehmen, EVM-Verträge | Wer ganz in den Tresor umzieht, verliert diese Wege | je ein eigener Schritt (Knoten, App) | ich, nacheinander |
| **Komitee-Checkpoints hybrid** schlafend; libp2p klassisch | Betrifft heute nur öffentliche Kettendaten | erst, wenn es mehrere Erzeuger gibt bzw. go-libp2p hybride Schlüssel kann | später |
| **Notbremse** (Stufe 4) schlafend | Sie ist richtig so: erst scharf, wenn ein Rechner in Reichweite ist | nichts | – |

### 3.2 Laufende Ausgabestelle für Quantengeld (fertig)

**Fertig gebaut sind:**
- Register und Topf mit Umlaufabgabe;
- Freischaltung durch ⅔ der Prüfer, mindestens 5;
- Einlösen, gesperrt bis zu einem geprüften Verfahren;
- QG3 (Noten je Zeile).

**Das alles läuft auf C1** (Stand 20.11.: jeder Deploy seit dem 16.11. enthält es,
zuletzt [44737199569](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44737199569)).
Freigeschaltet ist noch kein Satz. Das ist die Seite, die eine Kette
leisten muss, und sie ist da. Ein Satz wird erst freigeschaltet, wenn es Noten gibt,
die tragen. Bis dahin ist „schlafend“ der ehrliche Zustand.

### 3.3 Echte Quanten-Noten von Anfang bis Ende (Vorführung, Labormaßstab)

Mit dem vorhandenen IBM-Zugang (Umgebung `quantenhardware`, 14 Läufe bisher) und dem
vorhandenen Läufer (`hardware_ibm.py`: dynamische Schaltkreise, Warten, Teleportation)
geht heute in **einem** Lauf:
1. **Prägen** einer Variante-W-Note (8–16 Qubits, BB84). Basen und Bits legt vorher die
   Kette fest und verteilt sie als Anteile, wie in `wiesner_verteilt.py`.
2. **Halten** für eine Wartezeit (0–1.000 µs).
3. **Bezahlen** per Teleportation an ein zweites Register (den Empfänger).
4. **Prüfen, klassisch:** Die Basen der Aufforderung kommen aus Quanten-Zufall im selben
   Lauf. Messen, dann prüfen die (simulierten) Validatoren auf den Anteilen.
5. **Ergebnis:** Lauf-ID, Messwerte und Urteil, veröffentlicht und nachrechenbar.

**Was das bringt:**
- **Erstens** zeigt es, dass der ganze Ablauf auf echter Hardware läuft.
- **Zweitens** liefert es **echte Fehlerraten** je Qubit und Wartezeit. Genau daraus
  folgen Toleranz und Notengröße der Variante W. Bisher sind sie nur angenommen (2 %).

**Was es nicht bringt:** Sicherheit. Die Note ist zu klein und lebt Mikrosekunden.
Prägestelle und Halter sind derselbe Schaltkreis, und den Lauf bezeugt nur IBM. So muss
es auch dastehen: als **Vorführung**, mit Symbolwert, nie mit echten AEQ.

**Kosten:** wenige Sekunden Rechenzeit aus dem Freikontingent. Ein Lauf braucht deine
Freigabe der Umgebung `quantenhardware`.

**Gebaut (16.11.2026):**
- `wiesner_hardware.py`: Schaltkreise und Prüfung, mit Selbsttest in der CI.
- `hardware_ibm.py --wiesner [--bezahlen]` und im Workflow „Quantengeld auf
  Quanten-Hardware“ der Schalter `wiesner`.
- Ideal (Aer ohne Rauschen) läuft alles fehlerfrei, auch mit Teleportation.

**Rauschmodell `ibm_marrakesh`** (FakeMarrakesh, nicht die Maschine; Note aus 6 Qubits,
4 Noten je Fall, 500 Schüsse, Toleranz 15 %):

| Wartezeit | Fehler je Treffer | mit Bezahlen | Annahme | mit Bezahlen |
|---|---|---|---|---|
| 0 µs | 0,4 % | 2,0 % | 0,98 | 0,92 |
| 20 µs | 6,5 % | 8,9 % | 0,79 | 0,73 |
| 50 µs | 8,1 % | 16,2 % | 0,79 | 0,68 |
| 100 µs | 23 % | 30 % | 0,44 | 0,33 |

**Lesart:**
- Eine Variante-W-Note hält im Modell etwa **50 µs** innerhalb der Toleranz. Die alte
  Gruppenaktions-Note war nach rund 20 µs verloren (`HARDWARE_WEG.md`).
- Die Teleportation kostet etwa 2 Prozentpunkte.
- Der nächste Schritt ist derselbe Lauf auf der echten Maschine. Workflow, Modus `ibm`,
  `wiesner` an, `bezahlen` an, Wartezeiten `0,20,50,100`.

**Prägen ohne Vertrauen (W3, nachgetragen 16.11.2026):** Dieselbe Messung, aber die
Note entsteht durch Fern-Präparation (`hardware_ibm.py --wiesner --fern`): je Stelle ein
verschränktes Paar, die Validatoren messen ihre Hälften in eigenen Zufallsbasen. Das
Geheimnis kennt nur der Validator, nicht die Prägestelle. Rauschmodell `ibm_marrakesh`,
6 Stellen, 4 Noten je Fall, 500 Schüsse:

| Wartezeit | Fehler je Treffer | mit Bezahlen | Annahme | mit Bezahlen |
|---|---|---|---|---|
| 0 µs | 1,6 % | 3,0 % | 0,94 | 0,89 |
| 20 µs | 6,1 % | 5,6 % | 0,81 | 0,83 |
| 50 µs | 8,5 % | 9,6 % | 0,78 | 0,75 |
| 100 µs | 20,5 % | 22,6 % | 0,37 | 0,34 |

Die Fern-Präparation kostet im Modell etwa einen Prozentpunkt (ein CNOT mehr je Stelle).

Bestätigt im Workflow (16.11.2026, [Lauf 43894244447](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43894244447),
Rauschmodell `ibm_marrakesh`, 8 Noten je Fall, 1000 Schüsse): Fehler je Treffer 1,6 % /
5,9 % / 11,8 % / 22,0 %, Annahme 0,95 / 0,83 / 0,70 / 0,44 bei 0 / 20 / 50 / 100 µs.

**Prägen ohne Vertrauen auf der echten Maschine** (16.11.2026, `ibm_fez` – IBM hat den
freien Chip gewählt; [Lauf 43973771243](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43973771243),
IBM-Job `dbtoib1hlc8s738kbgdg`; 6 Stellen, also 12 Qubits, mit Bezahlen 24; 4 Noten je
Fall, 500 Schüsse):

| Wartezeit | Fehler je Treffer | mit Bezahlen | Annahme | mit Bezahlen |
|---|---|---|---|---|
| 0 µs | **2,4 %** | 12,5 % | **0,90** | 0,54 |
| 20 µs | 29,2 % | 32,7 % | 0,26 | 0,22 |
| 50 µs | 35,9 % | 37,6 % | 0,28 | 0,27 |
| 100 µs | 39,9 % | 39,2 % | 0,26 | 0,28 |

Note Nr. 1 (0 µs): 5 Treffer, 0 Fehler, von den Validatoren auf ihren Anteilen gezählt
→ **echt**.

**Lesart:**
- **Ohne Warten trägt die Fern-Präparation auf dem echten Chip:** 2,4 % Fehler je
  Treffer. Die Note aus einer Hand hatte auf `ibm_marrakesh` 0,7 % (anderer Chip). Das Geheimnis
  entsteht dabei erst bei der Messung der Validatoren. Mit der Prüfung der Kette heißt
  das (`wiesner_parameter.suche(0.024)`): **Toleranz 6 %, 1.360 geprüfte Stellen**.
  Eine ehrliche Note fällt dann mit höchstens 1,1·10⁻⁷ durch, ein Fälscher kommt
  höchstens mit 2⁻⁴⁰ durch. Bei der Standard-Toleranz von 4 % fiele jede 44. ehrliche
  Note durch (2,3 %). Die Toleranz je Note (W2c) ist genau dafür da.
- **Teleportation kostet auf diesem Chip zu viel:** 12,5 % Fehler. Dafür gibt es keine
  sichere Toleranz unter der Kloner-Schwelle von 14,6 % (`suche(0.125)` findet bis
  k = 8.192 nichts). Bezahlen per Teleportation ist auf `ibm_fez` heute nicht sicher
  prüfbar. Im Modell von `ibm_marrakesh` waren es 3 %.
- **Mit Warten ist die Note verloren:** Ab 20 µs liegen die Fehler bei 29–40 %, nahe am
  Raten (50 %). Auf `ibm_marrakesh` waren es nach 20 µs noch 23 %, es liegt also auch am
  Chip. Ohne Speicher (Ionenfalle, Fehlerkorrektur) bleibt es dabei: Prägen und Prüfen
  in einem Zug, oder Variante S.
- **Messwerte unter der alten Toleranz:** Die Annahme-Spalten sind noch mit 15 %
  gerechnet. Bei 6 Stellen ist aber so oder so kein Fehler erlaubt, mit 4 % ergeben
  sich dieselben Werte. Das Werkzeug (`wiesner_hardware.TOLERANZ`) steht jetzt auf
  4 %.

**Von Anfang bis Ende mit der Kette (W4 ohne Partner):** `hardware_ibm.py --wiesner
--kette --bezahlen` lässt eine Note aus 16 Qubits durch die Regeln der Kette laufen
(`wiesner_kette.KetteW`, dieselben Regeln wie im Knoten): Prägen mit Prüfsummen und
Bestätigung von 5 Validatoren (Schwelle 3) → Einlösen angefragt, Aufforderung aus dem
Block → Note 20 µs gehalten, per Teleportation an den Empfänger, **ein** Schuss als
Messung → Antwort → drei Validatoren legen offen, jeder Knoten rechnet → Urteil, AEQ
beim Empfänger, Summe erhalten.
- Ideal: echt (6 Treffer, 0 Fehler), 10 AEQ beim Empfänger.
- Rauschmodell `ibm_marrakesh`: echt (5 Treffer, 0 Fehler). Über alle 200 Schüsse
  desselben Schaltkreises: 10,7 % Fehler je Treffer, Annahme 0,57 – bei 16 Stellen
  erlaubt die Toleranz nur einen Fehler, jede zweite Note fiele durch.
  Im Workflow bestätigt (16.11.2026, [Lauf 43894251112](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43894251112),
  Aufforderung erst aus dem Block nach der Anfrage): echt (7 Treffer, 0 Fehler), 10 AEQ
  beim Empfänger, Topf 0, Summe erhalten.
- **Auf der echten Maschine** (16.11.2026, `ibm_marrakesh`, [Lauf 43894950189](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43894950189),
  IBM-Job `dbthuqr69a3s73fp0qq0`): der Ablauf läuft vollständig – Prägen, Anfrage,
  Aufforderung aus dem Block, Antwort mit Messwerten vom Chip, Offenlegen, Urteil. Die
  Note hat die 20 µs Halten plus Teleportation aber **nicht überstanden**: 9 Treffer,
  3 Fehler (33 %, Toleranz 15 %) → **abgewiesen**, kein Wert an den Empfänger, Summe
  erhalten. Über alle 1000 Schüsse: 39,1 % Fehler je Treffer, Annahme 0,056.
  Zum Vergleich (Lauf von #538): **sofort** geprüft 0,8 % Fehler, Annahme 0,98; nach
  20 µs 23 %, Annahme 0,53. Heißt: auf dem echten Chip trägt die Note heute nur, wenn
  sie nicht warten muss. Für alles mit Wartezeit braucht es den geschützten Speicher
  (Fehlerkorrektur, #528) oder die Ionenfalle (Sekunden statt Mikrosekunden).
- **Auf der echten Maschine, ohne Warten** (16.11.2026, `ibm_marrakesh`, [Lauf 43895622629](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43895622629),
  IBM-Job `dbti2t369a3s73fp0uq0`): Prägen → Anfrage → Aufforderung aus dem nächsten Block
  → Antwort mit Messwerten vom Chip → drei Validatoren legen offen → jeder Knoten rechnet:
  **10 Treffer, 0 Fehler → ECHT, 10 AEQ beim Empfänger**, Topf 0, Summe erhalten. Über
  alle 1000 Schüsse: 0,7 % Fehler je Treffer, **Annahme 0,998**. Damit läuft der ganze
  Ablauf einer Quanten-Note mit dezentraler Prüfung durch die Validatoren heute auf
  echter Quanten-Hardware – solange die Note nicht gelagert werden muss.
- **Geschützte Note auf der echten Maschine** (Fehlerkorrektur im Speicher, #528;
  16.11.2026, `ibm_marrakesh`, [Lauf 43895628489](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43895628489),
  IBM-Job `dbti3djsbsbs73cb321g`; Gruppenaktions-Note Z₈, Annahme lesekorrigiert):

  | Wartezeit | ungeschützt | 3-Qubit-Code (Phase) | 3-Qubit-Code (Bit) |
  |---|---|---|---|
  | 0 µs | 0,71 | 0,63 | 0,61 |
  | 20 µs | 0,32 | 0,32 | 0,22 |
  | 50 µs | 0,37 | 0,36 | 0,34 |
  | 100 µs | 0,30 | 0,32 | 0,30 |

  Kein messbarer Gewinn: die rund 40 zusätzlichen 2-Qubit-Gatter kosten so viel, wie der
  Schutz zurückgibt; nur bei 100 µs liegt der Phasen-Code knapp vorn (im Rauschen).
  Fehlerkorrektur verlängert die Lebensdauer erst mit kleineren Gatterfehlern
  (Ionenfalle) oder größeren Codes mit Korrektur in Echtzeit.
- **Wiesner-Stellen im Steane-Code (B6, nachgetragen 17.11.2026):**
  - **Aufbau:** Jede Stelle der Note liegt in 7 Qubits (Steane-Code [[7,1,3]],
    `wiesner_hardware.note_steane`). Gemessen wird transversal in der Basis der
    Aufforderung, dekodiert wird klassisch. Ein Fehler je Stelle wird korrigiert, auch
    ein Lesefehler, und es braucht keine Syndrom-Runden auf dem Chip. Die Sicherheit
    ändert sich nicht: Der Code ist öffentlich.
  - **Rauschmodell `ibm_marrakesh`** (2 Stellen, 8 Noten je Fall, 1000 Schüsse; aus 18
    CNOT werden auf dem Gitter des Chips 43):

    | Wartezeit | ungeschützt | Steane |
    |---|---|---|
    | 0 µs | 0,3 % | 0,8 % |
    | 5 µs | 1,0 % | 1,5 % |
    | 10 µs | 1,3 % | 2,1 % |
    | 20 µs | 3,0 % | 9,8 % |
    | 50 µs | 6,1 % | 16,8 % |

  - **Exakt gerechnet (`wiesner_schutz.py`):** Pauli-Rahmen über alle 2⁷ Fehlermuster,
    Codierer, Speicher und Messung. Mit Werten in der Größenordnung von IBM Heron (CNOT
    0,3 %, Lesen 0,5 %, Gitterfaktor 2,4) hilft der Code **bei keinem Speicherfehler**.
    Der Codierer ist nicht fehlertolerant: 6 von 27 Einzelfehlern kippen die Stelle.
  - **Mit voller Verbindung und Werten einer Ionenfalle** (CNOT 0,15 %, Lesen 0,2 %)
    hilft er in einem Fenster: ungeschützt 0,3 % bis 6 % Fehler. Bei 1 % Speicherfehler
    sind es 0,4 % statt 0,9 %.
  - **Lesart:** Auf heutigen IBM-Chips macht Fehlerkorrektur je Stelle die Note
    schlechter, nicht besser. Damit ist B6 beantwortet. Längeres Halten braucht kleinere
    Gatterfehler und volle Verbindung (Ionenfalle) oder einen fehlertoleranten Codierer
    mit Korrektur in Echtzeit.
  - **Auf der echten Maschine** (17.11.2026, `ibm_fez`, [Lauf 43986013444](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43986013444),
    IBM-Job `dbtv0apjbrss73a721l0`; 2 Stellen, 4 Noten je Fall, 1000 Schüsse):

    | Wartezeit | ungeschützt | Steane | Annahme ungeschützt | Annahme Steane |
    |---|---|---|---|---|
    | 0 µs | 0,6 % | 4,6 % | 0,99 | 0,91 |
    | 5 µs | 5,3 % | 11,4 % | 0,94 | 0,85 |
    | 10 µs | 15,4 % | 27,3 % | 0,85 | 0,73 |
    | 20 µs | 17,1 % | 24,2 % | 0,75 | 0,65 |
    | 50 µs | 26,8 % | 40,4 % | 0,63 | 0,41 |

    Der echte Chip bestätigt das Modell: Der Steane-Code ist bei jeder Wartezeit
    schlechter. Der Chip altert die Note dabei viel schneller als sein Rauschmodell:
    nach 5 µs 5,3 % statt 1,0 %. Note Nr. 1 (0 µs, ungeschützt): 2 Treffer, 0 Fehler,
    echt.

**Zählt das als Quantengeld?** Als Vorführung: ja, das ist der ganze Ablauf einer
privat prüfbaren Quanten-Note mit dezentraler Prüfung – auf echter Hardware, sobald der
IBM-Lauf freigegeben ist. Als Zahlungsmittel: nein. Die Note lebt Mikrosekunden, eine
Prüfung mit 16 Stellen ist schwach, und Prägen ohne Vertrauen läuft im selben
Schaltkreis statt über eine Quantenleitung. Dafür bleibt der Partnerversuch (W4 mit
Speicher, ANALYSE.md).

**Nachtrag 16.11.2026 abends:** Die Toleranz von 15 % ist gegen einen optimalen Kloner
gebrochen. Jede Kopie hat 14,6 % Fehler; eine geklonte Kopie besteht bei 64 Stellen mit
0,55. Sicher wären bei 0,7 % Hardware-Fehler eine Toleranz von 4 % und 840 geprüfte
Stellen (`wiesner_parameter.py`, ANALYSE.md „Befund“). Der Ablauf der Vorführung bleibt
richtig; die Prüfgrößen der Kettenseite müssen vor jeder Freischaltung neu gesetzt
werden.

### 3.3b Quanten-Token ohne Speicher (Variante S, nachgetragen 16.11.2026 abends)

Variante S ist der Teil, der **heute ohne Quantenspeicher** funktioniert (ANALYSE.md 5b,
`s_kette.py`).

**Ablauf** (`hardware_ibm.py --s-token`, im Workflow `wiesner` = `s-token`):
1. **Ausgabe:** 1.680 Qubits in zufälligen BB84-Zuständen, als 105 Schaltkreise zu 16
   Qubits mit je einem Schuss.
2. **Sofort messen:** Die Nutzerin misst in zufälligen Basen. Sie hält danach nur
   Klartext; nichts wird gespeichert.
3. **Bindung und Wahl auf der Kette:** die maskierten Basen d und die Wahl c.
4. **Vorlage ohne Netz:** an der gewählten Annahmestelle, die nur ihre geheime
   Stichprobe von 840 Stellen kennt.
5. **Angriff:** Derselbe Token wird an der anderen Stelle vorgelegt.

**Ergebnisse:**
- **Ideal:** Fehler 0 %. Die gewählte Stelle nimmt an (416 Treffer, 0 Fehler), die
  Doppelvorlage wird abgewiesen (409 Treffer, 205 Fehler).
- **Rauschmodell `ibm_marrakesh`:** Fehler 0,50 %. Die gewählte Stelle nimmt an
  (395 Treffer, 2 Fehler, Toleranz 4 %), die Doppelvorlage wird abgewiesen (437 Treffer,
  218 Fehler).
- **Auf der echten Maschine** (16.11.2026, `ibm_marrakesh`,
  [Lauf 43927782462](https://github.com/hanoi96international-gif/Aequitas/actions/runs/43927782462),
  IBM-Job `dbtkmcbrto6s73fsvl20`): Fehler 0,46 %. Die gewählte Stelle nimmt an
  (**438 Treffer, 4 Fehler**, Toleranz 4 %), die Doppelvorlage wird abgewiesen (401 Treffer,
  196 Fehler). **Damit läuft der erste Quanten-Token mit sicheren Prüfgrößen
  (Fälscher ≤ 2⁻⁴⁰) auf echter Quanten-Hardware, ohne Quantenspeicher.**

**Was das ist:** ein Quanten-Token mit sicheren Prüfgrößen. Ein Fälscher kommt mit
höchstens 2⁻⁴⁰ an zwei Stellen durch (`wiesner_parameter.py`).

**Was es nicht ist:**
- **Kein Bargeld von Hand zu Hand.**
- **Noch nicht unfälschbar:** In der Vorführung laufen Ausgabe und Messen im selben
  Schaltkreis beim Betreiber der Maschine. Unfälschbar wird es erst, wenn die Nutzerin
  selbst misst (QKD-Empfänger) und die Ausgabe über die Validatoren läuft.

### 3.3c Erster S-Token im Hauptnetz (17.11.2026, 17:22–17:32 UTC)

Lauf [44132434033](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44132434033),
Werkzeug `aequitasd quanten-s-ausgabe` (#569), Stichtag 17.11. 17:00 UTC (#570).

- **Messung:** IBM `ibm_fez`, Job `dbu8i549po4c73cen9p0`. 1680 Qubits in 105 Schaltkreisen,
  je ein Schuss. Fehler bei passender Basis: 1/794 (0,13 %).
- **Ausgabe:** K = `13664054520c66b595d1ad2e365b2ecd118515b7ba33ea67ba1bd6807666d086`,
  Deckung 1 AEQ. Ausgeber und einziger Validator: C1 (`0x3066…42dc`, gebunden an den
  Betreiber). 840 Stellen je Annahmestelle, Toleranz 4 %.
- **Doppelvorlage** an Stelle 0 (Wegwerfschlüssel): **abgewiesen**, ein Fehlversuch.
- **Ehrliche Vorlage** an Stelle 1 (C1): **eingelöst**, der Wert ging an C1 zurück.
- Ehrlich: Präparation und Messung liefen im selben IBM-Schaltkreis (Vorführung), es gab
  nur einen Validator, und es liegt kein Gutachten vor. Neu ist der ganze Ablauf auf der
  laufenden Kette mit Anteilen beim Validator-Dienst.

### 3.3d Pilot mit vier Validatoren in voller Größe (17.11.2026, Rauschmodell)

Workflow `quanten-s-pilot.yml`: vier echte Knoten im Job, Geheimnis 3-von-4 geteilt,
k = 5.200 Stellen je Annahmestelle (die Tabelle für 3 von 4 bei Toleranz 4 %), also 10.400
Qubits. Gemessen im Rauschmodell `ibm_marrakesh`, nicht auf dem Chip. Je Lauf zwei Tests:
alle vier Knoten und ein Knoten fällt vor der ersten Vorlage aus. Hier stehen alle Läufe,
auch die gescheiterten.

| Lauf | Stand | Ergebnis | Ursache, Folge |
|---|---|---|---|
| [44164912648](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44164912648) | `main` nach #572 | rot: `Urteil: nicht in 15m0s (fehl 0, … offen 3456 von 5200)` (mit Ausfall) | Keine falsche Prüfung, nur zu langsam (rund 230 Stellen/min). Der Dienst reichte denselben Teil mehrfach ein, gegen die Burst-Grenzen → #573. Das Werkzeug gab vor der Kettenfrist auf → #574. |
| [44166377116](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44166377116) | Zweig mit #573 | rot nach Erfolg (mit Ausfall): Werkzeug 16m20s, **Doppelvorlage abgewiesen, ehrliche Vorlage eingelöst**; danach `Knoten 0: Token lebt=true tot=false` | Der Test las den Stand der anderen Knoten nur einmal, Knoten 0 war einen Block hinterher. Der Test wartet jetzt bis zu 2 min (#575); der Gleichstand wird weiter streng geprüft. |
| [44178246231](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44178246231) | `main` nach #574 | abgebrochen | Neu gestartet mit der Test-Korrektur (nächste Zeile). |
| [44178445416](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44178445416) | Zweig mit #574 und Test-Korrektur (f90b7cd) | **grün**, beide Tests. Mit Ausfall: Werkzeug **12m50s**, Fehlerrate bei passender Basis 24/5208, Doppelvorlage **abgewiesen**, ehrliche Vorlage **eingelöst**, die drei lebenden Knoten gleich (Höhe 5821, StateRoot `cebc24bb…51bd07`). Paket `ok` nach 1917,7 s | Ohne Ausfall: bestanden (Paket `ok`); Dauer und Einzelzeilen liegen vor dem abrufbaren Protokollende und sind nicht belegt. |

| [44186803503](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44186803503) | `main` nach #575 (7fb0a38), Rauschmodell | **grün**, beide Tests, Paket `ok` nach 1366 s. Mit Ausfall (666,5 s): Fehlerrate 19/5189; ausgegeben nach 16 s, gebunden/gewählt nach 17 s; **Doppelvorlage**: 5.200 Stellen in 4m30s offengelegt (**1.156 Stellen/min**), **abgewiesen** (fehl 0 → 1); **ehrliche Vorlage**: 5.200 Stellen in 5m28s (**952 Stellen/min**), **eingelöst**; Werkzeug gesamt 10m50s; drei Knoten gleich (Höhe 4962, StateRoot `7803c08d…0781a3`) | Artefakt `pilot-44186803503` (90 Tage) mit ganzer Ausgabe. Ohne Ausfall: bestanden; Einzelzeilen nur im Artefakt bzw. in der Zusammenfassung. |
| [44186826204](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44186826204) | `main` nach #575, **echter IBM-Chip `ibm_kingston`** (freigegeben 18.11. 06:46 UTC) | **grün**, beide Tests, Paket `ok` nach 1698,4 s. **Messung:** 10.400 Qubits in 650 Schaltkreisen, je ein Schuss, Job `dbukk3darooc73fk52hg`, Fehler bei passender Basis **31/5.241 (0,59 %)**. Mit Ausfall (653,0 s): Doppelvorlage 5m00s, 1.040 Stellen/min, **abgewiesen**; ehrliche Vorlage 4m59s, 1.042 Stellen/min, **eingelöst**; Werkzeug 10m37s; drei Knoten gleich (Höhe 4810, StateRoot `927387c0…2aff81`) | Ohne Ausfall: bestanden (Paket `ok`); die Einzelzeilen liegen vor dem abrufbaren Protokollende, im Artefakt `pilot-44186826204` vollständig. |

**Mit Netzlaufzeit zwischen den Knoten** (PR #578, `latenz_ms`): `tc netem` nur auf den
Knotenadressen, der Rundlauf ist etwa 2 × die Einstellung. Der Lauf bricht ab, wenn die
Laufzeit nicht wirkt. Seit #576 steht das ganze Ergebnis am Protokollende und ist damit
für beide Tests belegt.

| Lauf | Laufzeit je Richtung | Test | Doppelvorlage | ehrliche Vorlage | Werkzeug | danach |
|---|---|---|---|---|---|---|
| [44251220417](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44251220417) | 40 ms | alle vier (803,6 s) | 4m56s, 1.053/min, **abgewiesen** | 8m03s, **646/min**, **eingelöst** | 13m06s | 4 Knoten gleich, Höhe 7895 |
| ebenda | 40 ms | ein Knoten fällt aus (660,5 s) | 4m47s, 1.087/min, **abgewiesen** | 4m46s, 1.089/min, **eingelöst** | 10m46s | 3 Knoten gleich, Höhe 4883 |

| [44251224574](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44251224574) | 100 ms | alle vier (704,9 s) | 5m15s, 992/min, **abgewiesen** | 5m59s, 869/min, **eingelöst** | 11m27s | 4 Knoten gleich, Höhe 3447 |
| ebenda | 100 ms | ein Knoten fällt aus (711,3 s) | 4m53s, 1.065/min, **abgewiesen** | 6m28s, 805/min, **eingelöst** | 11m37s | 3 Knoten gleich, Höhe 3498 |

SHA-256:
- Lauf 44251220417: `ergebnis.txt` `090b7f52…450ea4`, `ausgabe.txt.gz` `14945d4f…26e1a4`.
- Lauf 44251224574: `ergebnis.txt` `da95ede8…c2782a`, `ausgabe.txt.gz` `d70490ff…19d07c5`.
- Jeweils im Artefakt `pilot-<Lauf>`.

**Ergebnis Netzlaufzeit:**
- Auch bei 200 ms Rundlauf zwischen den Knoten (interkontinentale Server) läuft die
  Offenlegung mit 800–1.090 Stellen/min.
- Die langsamste Offenlegung aller Läufe dauerte 8 Minuten (646/min, 40 ms, ohne Ausfall).
  Die Kettenfrist ist 30 Minuten, der Abstand also mindestens 3,7-fach.
- Die Laufzeit ist nicht der Engpass.

**Befund mit Zählern belegt** (Lauf [44257006958](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44257006958), 40 ms, a69b1d9; beide Tests grün):

| Test | Dienst | eingereicht | abgewiesen | eingesprungen | Nachbarabfragen |
|---|---|---|---|---|---|
| alle vier (ehrliche Vorlage 934/min) | 0 / 1 / 2 / 3 | 128 / 97 / 53 / 47 | **22 / 72 / 34 / 43** | 1 / 2 / 1 / 0 | 180 / 162 / 80 / 84 |
| ein Knoten fällt aus (1.142/min) | 0 / 1 / 2† / 3 | 160 / 0 / 0 / 166 | **0 / 0 / 0 / 5** | 2 / 0 / 0 / 1 | 161 / 0 / 0 / 180 |

- Ohne Ausfall reichten **alle vier** Dienste ein, und **171 von 496** Einreichungen wurden
  abgewiesen. Es gab 506 statt 341 Nachbarabfragen.
- Ursache:
  - Ein Ersatz springt ein, sobald ein Teil einmal 10 s offen ist.
  - Er blieb danach bis zum Ende des Versuchs dran, auch wenn der Zuständige wieder
    arbeitete. Dann konkurrierten beide um jeden Teil.
- **Versuch 1 (43b9c78): Der Ersatz tritt zurück, sobald ein anderer vorankommt. Gemessen
  schlechter, zurückgenommen (0aafbfe).** Prüflauf
  [44264862815](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44264862815):

  | Test | Rate | abgewiesen | eingesprungen / zurückgetreten |
  |---|---|---|---|
  | alle vier | 687–793/min (vorher 934–990) | 218 | 20 / 18 |
  | ein Knoten fällt aus | 575–613/min (vorher 1.088–1.142) | 217 (vorher 5) | 22 / 20 |

  Die Ersatz-Dienste pendelten bis zu 14-mal zwischen Einspringen und Zurücktreten.
- **Hypothese danach (mit Versuch 2 widerlegt, siehe unten): Engpass sei die Burst-Grenze
  je Absender**, 20 Einreichungen je Minute und Validator. Die Grenze bleibt so.
  - Mit Ausfall reichten zwei Dienste je etwa 17–18 Teile/min ein. Bei 32 Stellen je Teil
    sind das die gemessenen rund 1.100 Stellen/min.
  - Paralleles Einreichen verteilte die Arbeit also auf mehrere Budgets. Es war nicht nur
    Verschwendung.
- **Versuch 2 (6f72941): mehr Stellen je Einreichung.** Der Teil wird genau auf das
  Größenbudget zugeschnitten statt halbiert. Bei 3 von 4 Validatoren und Tiefe 14 sind das
  50 statt 32 Stellen. Prüflauf
  [44271941121](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44271941121)
  (40 ms, beide Tests grün, Vergleich mit Lauf 44257006958):

  | Test | Rate | Einreichungen | abgewiesen | Nachbarabfragen |
  |---|---|---|---|---|
  | alle vier, vorher / **nachher** | 934–990 / **929–993** | 325 / **200** | 171 / **95** | 506 / 582 |
  | ein Ausfall, vorher / **nachher** | 1.088–1.142 / **947–1.360** | 326 / **200** | 5 / **0** | 341 / 426 |

  - **Gleich schnell, aber 38 % weniger Einreichungen**, also weniger Transaktionen in den
    Blöcken für dieselbe Offenlegung, und rund halb so viele Abweisungen.
  - Dafür 15–25 % mehr Nachbarabfragen: Ein kleiner Rest wird nicht mehr eigens eingereicht.
  - Behalten.
- **Damit ist auch die Burst-Grenze als Engpass widerlegt.**
  - Mit größeren Teilen reicht jeder Dienst nur noch etwa 7 Teile/min ein, weit unter 20,
    und die Rate bleibt bei etwa 1.000 Stellen/min.
  - Begrenzend ist die **Abfolge je Teil**: Ein Teil muss im Stand stehen, bevor der nächste
    eingereicht werden kann, und jede Nachbarrunde wartet nach t Antworten noch
    `quantenWPeerNachlauf` (1 s) auf weitere Anteile. Das gibt rund 20 Teile/min, gleich wie
    viele Dienste helfen.
- **Schluss:**
  - Bei rund 1.000 Stellen/min dauert eine Offenlegung von 5.200 Stellen etwa 5–8 Minuten,
    die Kettenfrist ist 30 Minuten.
  - Weitere Beschleunigung (Vorausholen der nächsten Anteile, kürzerer Nachlauf) würde
    Robustheit gegen nicht benötigtes Tempo tauschen und unterbleibt.
  - Wird die Frist im Betrieb knapp, ist das Vorausholen der nächste Schritt.

**Frühere Vermutung (vor den Zählern):**
- Bei 40 ms war die ehrliche Vorlage ohne Ausfall langsamer (646/min) als mit Ausfall
  (1.089/min).
- Bei 100 ms war sie ohne Ausfall *schneller* (869/min). Die 646/min sind also eher Streuung
  als ein festes Muster.
- Belegen sollen es Zähler je Validator-Dienst (eingereicht, abgewiesen, eingesprungen,
  Nachbarabfragen), die der Pilot seit diesem Stand ausgibt.
- Erklärung aus dem Code: Mit Laufzeit erreicht ein Teil die anderen Knoten später. Ist er
  10 s offen, springt Rang 1 ein und bleibt dran (#574). Dann reichen zwei Validatoren
  parallel ein und behindern sich; mit Ausfall fehlt dieser Konkurrent.
- Selbst dann sind es 8 Minuten statt 30 Minuten Kettenfrist, also 3,7-facher Abstand.

**Rate der Offenlegung (Auftrag 3 des Betreibers):**
- Vorher (#572): rund 230 Stellen/min. Das reichte nicht für 5.200 Stellen in 15 Minuten.
- Jetzt mit einem ausgefallenen Validator: 950–1.160 Stellen/min. Das ist das 4- bis 5-fache. Ursachen:
  - #573: kein doppeltes Einreichen.
  - #574: Ersatz in fester Reihenfolge.
- Eine Offenlegung in voller Größe dauert 4½–5½ Minuten. Die Kettenfrist von 30 Minuten
  liegt also gut 5-fach darüber.
- Keine Grenze wurde gelockert, keine Kettenregel geändert.
- Weiteres Beschleunigen ist nicht nötig.
- Zu beobachten bleibt, ob die Rate auf echten Servern im Netz (Latenz statt `127.0.0.x`)
  ähnlich ist. Das zeigt erst ein Testnetz mit mehreren Servern.

**Erster S-Token in voller Prüfgröße auf echter Hardware mit vier Validatoren (18.11.2026):**
- Gemessen auf IBM `ibm_kingston` mit 0,59 % Fehlern, ausgegeben mit den Bestätigungen von
  vier Validatoren (3 von 4).
- Die Doppelvorlage an der falschen Annahmestelle wurde abgewiesen, die ehrliche Vorlage
  eingelöst, auch als ein Validator vor der ersten Vorlage ausfiel.
- Ehrlich dazu:
  - Präparation und Messung liefen im selben IBM-Schaltkreis. Es gab keine Übertragung über
    eine Leitung und keinen Speicher; das ist bei Variante S so vorgesehen, die Leitung ist
    S3.
  - Die vier Knoten liefen in einem Job.
  - Es liegt kein Gutachten vor.

**Ergebnis:** Variante S läuft mit vier Validatoren (3 von 4) in voller Größe der Tabelle
durch, auch wenn ein Validator ausfällt. Dabei gilt: Messung im Rauschmodell, Knoten in
einem Job, kein Gutachten.

**Grenzen der Nachvollziehbarkeit bis hier:** Bei den ersten Läufen ist nur das Ende des
Protokolls abrufbar (GitHub liefert über die Schnittstelle höchstens 5.000 Zeilen). Ob der
Test ohne Ausfall in den Läufen 44164912648 und 44166377116 grün war, ist darum nicht
belegt. Seit #575 legt jeder Lauf die Meldungen der Tests vollständig in die
Zusammenfassung und hebt `ergebnis.txt` und die ganze Ausgabe `ausgabe.txt.gz` 90 Tage als
Artefakt auf, mit SHA-256 in der Zusammenfassung.

### 3.3e Ausgabe in Abschnitten: jeder Validator präpariert selbst (18.11.2026)

Weg A aus `DEZENTRAL.md` (Abschnitte 3 und 7–12): Jeder der vier Validatoren präpariert
**seinen** Abschnitt des Tokens in einem **eigenen Auftrag** am Quantenrechner, mit
**eigener Messquelle**. Das Werkzeug sieht nur die Basen der Inhaberin, Blätter und
Chiffrate, nie u und t. Workflow `quanten-s-pilot.yml` mit `ausgabe = abschnitte`,
k = 5.200 (10.400 Qubits, je Abschnitt 2.600), keine künstliche Netzlaufzeit. Hier stehen
alle Läufe, auch die gescheiterten.

| Lauf | Messung | Test | Ergebnis |
|---|---|---|---|
| [44340478883](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44340478883) | `main` nach #587, Rauschmodell `fake_marrakesh`; 4 Aufträge (`982ef3d1…`, `f16838f2…`, `d4412178…`, `787b5904…`), Fehler bei passender Basis 13/5.133 (aus den Stichproben) | alle vier (695,4 s) | **grün:** Doppelvorlage 4m59s, 1.045 Stellen/min, **abgewiesen**; ehrliche Vorlage 6m14s, 835/min, **eingelöst**; Werkzeug 11m15s; 4 Knoten gleich, Höhe 6806, StateRoot `02d6503f…ee0b6` |
| ebenda | ebenda | ein Knoten fällt aus (15,5 s) | **rot:** `Validator 1 praepariert nicht: HTTP 409: … keine Messung zu diesen Basen` (Ursache unten) |
| [44341842380](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44341842380) | `main` nach #587, **echter IBM-Chip `ibm_marrakesh`** (freigegeben); 4 Aufträge `dbusn45arooc73fkip80`, `dbusnbmps3fc73bjg4m0`, `dbusnj5arooc73fkipp0`, `dbusnpv82tmc73du1mbg`; Fehler bei passender Basis **41/5.214 (0,79 %)** | alle vier (660,4 s) | **grün:** Doppelvorlage 4m58s, 1.048/min, **abgewiesen**; ehrliche Vorlage 5m39s, 920/min, **eingelöst**; Werkzeug 10m40s; 4 Knoten gleich, Höhe 6443, StateRoot `007fb794…45b1ab` |
| ebenda | ebenda | ein Knoten fällt aus (15,7 s) | **rot**, dieselbe Meldung |
| [44347416357](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44347416357) | Zweig mit der Korrektur (cce644a9), Rauschmodell `fake_marrakesh`, **zwei Ausgaben, 8 Aufträge**; Ausgabe 1: Fehler 26/5.069 | alle vier (670,1 s) | **grün:** Doppelvorlage 5m03s, 1.031/min, **abgewiesen**; ehrliche Vorlage 5m45s, 905/min, **eingelöst**; Werkzeug 10m50s; 4 Knoten gleich, Höhe 6542, StateRoot `359c3184…1af3e5` |
| ebenda | Ausgabe 2: Fehler 21/5.277 | **ein Knoten fällt aus** (722,4 s; Knoten 2 vor der ersten Vorlage) | **grün:** Doppelvorlage 5m54s, 882/min, **abgewiesen**; ehrliche Vorlage 5m14s, 994/min, **eingelöst**; Werkzeug 11m45s; 3 Knoten gleich, Höhe 5415, StateRoot `8d91e045…a965e` |
| [44356741223](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44356741223) | `main` nach #588 (799f6783), **echter IBM-Chip `ibm_fez`** (freigegeben), **8 Aufträge**: Ausgabe 1 `dbutuhv82tmc73du3m1g`, `dbutusv82tmc73du3me0`, `dbutv3larooc73fkkq5g`, `dbutvalarooc73fkkql0`, Fehler **36/5.287 (0,68 %)** | alle vier (795,7 s) | **grün:** Doppelvorlage 6m09s, 845/min, **abgewiesen**; ehrliche Vorlage 6m44s, 772/min, **eingelöst**; Werkzeug 12m56s; 4 Knoten gleich, Höhe 7801, StateRoot `4fc4053e…fd4a38f` |
| ebenda | Ausgabe 2: `dbutvh5arooc73fkkqug`, `dbutvnups3fc73bji6cg`, `dbutvu7f8rhs73bmvq20`, `dbuu04eps3fc73bji6p0`, Fehler **27/5.161 (0,52 %)** | **ein Knoten fällt aus** (662,4 s; Knoten 2 vor der ersten Vorlage) | **grün:** Doppelvorlage 4m41s, 1.110/min, **abgewiesen**; ehrliche Vorlage 5m59s, 870/min, **eingelöst**; Werkzeug 10m46s; 3 Knoten gleich, Höhe 4886, StateRoot `99c04a73…7a241f` |

Gerät: Genannt ist das Gerät des letzten Auftrags eines Laufs. Jeder Auftrag wählte sein
Gerät selbst; die übrigen können auf anderen Chips gelaufen sein (3.3g, „Welcher Chip?“).

Zähler je Validator-Dienst (Test „alle vier“):

| Lauf | eingereicht | abgewiesen | eingesprungen | Nachbarabfragen |
|---|---|---|---|---|
| Rauschmodell, Dienst 0 / 1 / 2 / 3 | 72 / 66 / 30 / 32 | 16 / 27 / 25 / 12 | 1 / 2 / 1 / 0 | 202 / 187 / 89 / 91 |
| IBM, Dienst 0 / 1 / 2 / 3 | 69 / 32 / 35 / 64 | 9 / 12 / 0 / 24 | 2 / 1 / 0 / 1 | 183 / 88 / 94 / 187 |
| Nachlauf alle vier, Dienst 0 / 1 / 2 / 3 | 72 / 27 / 33 / 68 | 3 / 3 / 12 / 10 | 2 / 1 / 0 / 1 | 186 / 68 / 93 / 193 |
| Nachlauf mit Ausfall, Dienst 0 / 1 / 2† / 3 | 100 / 50 / 0 / 50 | 0 / 0 / 0 / 4 | 1 / 1 / 0 / 0 | 216 / 104 / 0 / 114 |
| IBM `ibm_fez` alle vier, Dienst 0 / 1 / 2 / 3 | 31 / 46 / 66 / 57 | 7 / 30 / 59 / 96 | 1 / 1 / 1 / 2 | 82 / 145 / 216 / 199 |
| IBM `ibm_fez` mit Ausfall, Dienst 0 / 1 / 2† / 3 | 89 / 25 / 0 / 86 | 0 / 5 / 0 / 7 | 2 / 1 / 0 / 1 | 191 / 56 / 0 / 204 |

SHA-256 (je im Artefakt `pilot-<Lauf>`, 90 Tage):
- Lauf 44340478883: `ergebnis.txt` `9646ccc5…30dad1`, `ausgabe.txt.gz` `7aeb9f3e…e91fb`.
- Lauf 44341842380: `ergebnis.txt` `55ee4c57…168be2`, `ausgabe.txt.gz` `5bf69870…a7c603`.
- Lauf 44347416357: `ergebnis.txt` `3ec90cda…d9b835c`, `ausgabe.txt.gz` `8dc0bd90…d0e7d4`.
- Lauf 44356741223: `ergebnis.txt` `3059d721…7fc1598`, `ausgabe.txt.gz` `c16bb837…9b61598`.

**Ursache der roten Ausfall-Tests: ein Fehler im Pilot, nicht im Dienst.**
- Beide Tests lasen dieselbe Messung: je Validator **ein** Abschnitt in `mess_<x>.json`,
  dazu eine `basen.json`.
- Der Dienst verbraucht jeden gemessenen Abschnitt genau einmal. Er löscht ihn aus der
  Datei, bevor er ihn verwendet (`quantenSMessDatei`). So kann aus einer Messung nie ein
  zweiter Token werden.
- Der erste Test hatte alle vier Abschnitte verbraucht. Die zweite Ausgabe mit denselben
  Basen wies der Dienst darum ab. Das ist das richtige Verhalten (fail-closed): Auch ein
  echter Quantenzustand lässt sich nicht zweimal ausgeben.
- **Korrektur:** `hardware_ibm.py --s-abschnitte … --ausgaben 2` misst zwei unabhängige
  Ausgaben (8 Aufträge). Jeder Test nimmt seine eigene (`basen_1.json` ohne Ausfall,
  `basen_2.json` mit Ausfall). Der Workflow prüft genau diese sechs Dateien und je
  Messquelle zwei Abschnitte.
- **Nachlauf im Rauschmodell (44347416357) und auf dem Chip `ibm_fez` (44356741223):
  beide Tests grün**, auch mit Ausfall eines Validators.
- Auffällig auf dem Chip (Test „alle vier“): 192 von 392 Einreichungen wurden abgewiesen
  (Dienst 3 allein 96), die Offenlegung lief mit 772–845 Stellen/min. Das ist langsamer
  als sonst (800–1.110), aber 4,5-fach unter der Kettenfrist von 30 min. Abgewiesen heißt
  hier: ein anderer Dienst war mit demselben Teil schneller, die Kette nahm ihn einmal.
  Nichts wurde falsch verbucht.

**Ergebnis bis hier:**
- Zum ersten Mal hat **jeder Validator seinen Abschnitt selbst präpariert** und auf einem
  echten Chip messen lassen (`ibm_marrakesh`, dann `ibm_fez`).
- Der Token in voller Prüfgröße ging mit vier Validatoren (3 von 4) durch:
  Doppelvorlage abgewiesen, ehrliche Vorlage eingelöst, alle vier Knoten gleich.
- Die Offenlegung ist so schnell wie bei der Ausgabe in einem Auftrag (835–1.048 statt
  800–1.160 Stellen/min).
- Ehrlich dazu:
  - Im Pilot kommen alle Aufträge vom selben IBM-Konto und laufen im selben Job. Dass
    niemand den ganzen Token kennt, gilt erst, wenn jeder Validator mit eigenem Konto
    selbst misst (A5, Betreiber).
  - Der Ausfall eines Validators bei der Ausgabe in Abschnitten ist im Rauschmodell
    (44347416357) und auf dem Chip `ibm_fez` (44356741223) belegt.
  - Es liegt kein Gutachten vor.

### 3.3f Messfehler senken: Mehrheit aus mehreren Schüssen (18.11.2026)

Auftrag des Betreibers: „Die Fehlerquote muss deutlich gesenkt werden.“ Gemeint ist der
Fehler bei passender Basis: Die Inhaberin misst in der Basis, in der die Stelle präpariert
wurde, und bekommt trotzdem den falschen Wert. Auf den Chips lag er bei 0,52–0,79 %
(3.3d, 3.3e).

**Woher er kommt:**
- Fast ganz aus der **Auslese**. Die Schaltkreise haben nur 1-Qubit-Gatter (Präparation,
  Basiswechsel) und werden sofort gemessen.
- **Die Qubit-Wahl ist schon gut.** Die Übersetzung (Stufe 3) wählt je Schaltkreis die 16
  Qubits mit dem kleinsten Auslesefehler. Geprüft mit den Kalibrierdaten der Rauschmodelle:
  Der mittlere Auslesefehler der gewählten Qubits entspricht dem der besten 16 des Chips
  (`ibm_fez` 0,28 %, `ibm_marrakesh` 0,36 %, `ibm_torino` 0,67 %; `ibm_kingston`
  0,46 % gegen 0,40 %). Hier ist kaum etwas zu holen.

**Hebel: Wiederholen und Mehrheit** (`hardware_ibm.py`, `s_messen_basen`, `--wiederholen R`,
Standard 5):
- Jeder Schaltkreis läuft R-mal. Das sind R Präparationen desselben Zustands, und jede wird
  in derselben Basis y gemessen. Je Stelle entscheidet die Mehrheit.
- **Passt die Basis** (y = u), zeigt jeder Schuss t mit Auslesefehler p. Die Mehrheit irrt
  nur noch mit etwa 10 p³ (bei R = 5).
- **Passt die Basis nicht** (y ≠ u), ist jeder Schuss unabhängig von t gleichverteilt, die
  Mehrheit ebenso. Die Inhaberin erfährt über t nicht mehr als mit einem Schuss. In der
  anderen Basis wird nie gemessen, denn der Auftrag misst jeden Schuss in y.
- Das geht nur, weil Präparation und Messung **im selben Auftrag** laufen (`DEZENTRAL.md`
  4). Mit eigenem Empfänger (Weg C) hat die Inhaberin ein Qubit und eine Messung.

**Gemessen im Rauschmodell** (je Lauf 4.000 Stellen, derselbe Abschnitt, `--s-abschnitt`):

| Rauschmodell | ein Schuss (R = 1) | Mehrheit aus 3 | Mehrheit aus 5 |
|---|---|---|---|
| `ibm_fez` | 10/1.995 (0,50 %) | 1/2.009 (0,05 %) | **0/1.974** |
| `ibm_marrakesh` | 6/2.002 (0,30 %) | 0/2.031 | **0/1.967** |
| `ibm_torino` | 11/2.021 (0,54 %) | 0/1.997 | **0/2.022** |

**Gegenprobe zur Sicherheit:** Bei falscher Basis stimmt x in jedem Lauf an 48,5–51,2 % der
Stellen mit t überein (je rund 2.000 Stellen, |z| ≤ 1,33), mit Mehrheit wie ohne. Das ist
Zufall, wie verlangt. Der Selbsttest zählt es zusätzlich exakt: Gleichverteilte Schüsse
ergeben eine gleichverteilte Mehrheit.

**Gemessen im Pilot** (k = 5.200, zwei Ausgaben, je Validator ein eigener Auftrag, 8
Aufträge, Mehrheit aus 5):

| Lauf | Gerät | Fehler bei passender Basis mit Mehrheit | nur erster Schuss |
|---|---|---|---|
| [44367279629](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44367279629) | Rauschmodell `fake_marrakesh` | **0/10.260** | 42/10.260 (0,41 %) |
| [44367782047](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44367782047) | **echter Chip `ibm_fez`** | **4/10.393 (0,04 %)** | 82/10.393 (0,79 %) |

- Auf dem Chip ist das rund **zwanzigmal weniger**.
- „Chip `ibm_fez`“ ist das Gerät des letzten Auftrags; die übrigen sieben können auf
  anderen Chips gelaufen sein (3.3g, „Welcher Chip?“).
- Die vier Restfehler liegen alle in einem Auftrag (Ausgabe 1, Validator 2). Das ist der
  zusammenhängende Rest, den das Rauschmodell nicht zeigt, etwa ein Qubit, das in jedem
  Schuss gleich falsch ausliest.
- Je Auftrag mit Mehrheit (Ausgabe 1, dann 2; je Validator 1–4): 0, 4, 0, 0 und 0, 0, 0, 0.
  Mit einem Schuss: 11, 11, 11, 8 und 6, 14, 12, 9.

**Vier Knoten mit diesen Messungen, beide Tests grün:**

| Lauf | Test | Fehler in den Stichproben | Doppelvorlage | ehrliche Vorlage | danach |
|---|---|---|---|---|---|
| Rauschmodell 44367279629 | alle vier (692,8 s) | 0/5.125 | 5m09s, 1.008/min, **abgewiesen** | 5m58s, 872/min, **eingelöst** | 4 Knoten gleich, Höhe 6494 |
| ebenda | ein Knoten fällt aus (744,0 s) | 0/5.135 | 5m53s, 884/min, **abgewiesen** | 5m34s, 933/min, **eingelöst** | 3 Knoten gleich, Höhe 5368 |
| **Chip `ibm_fez`** 44367782047 | alle vier (815,4 s) | 4/5.221 | 5m05s, 1.024/min, **abgewiesen** | 8m08s, 640/min, **eingelöst** | 4 Knoten gleich, Höhe 8004 |
| ebenda | ein Knoten fällt aus (592,5 s) | 0/5.172 | 5m15s, 992/min, **abgewiesen** | 4m17s, 1.214/min, **eingelöst** | 3 Knoten gleich, Höhe 4360 |

SHA-256: Lauf 44367279629 `ergebnis.txt` `a83e2c24…cf9ef0`, `ausgabe.txt.gz`
`57b6c234…c273d0`; Lauf 44367782047 `ergebnis.txt` `a55740d0…902281`, `ausgabe.txt.gz`
`ea5a0fa6…777469` (je im Artefakt `pilot-<Lauf>`).

**Ehrlich dazu:**
- Im Rauschmodell sind die Auslesefehler der Schüsse unabhängig voneinander. Auf echter
  Hardware hängen sie teils zusammen (siehe oben), darum bleibt ein kleiner Rest.
- Jede Messung gibt beide Zahlen aus: Fehler mit Mehrheit und nur mit dem ersten Schuss.
- Kosten: R-mal so viele Schüsse; die Zahl der Schaltkreise und Aufträge bleibt gleich.
- Die Toleranz der Kette (4 %) bleibt, wie sie ist. Liegt der Fehler auf dem Chip dauerhaft
  weit darunter, könnte eine kleinere Toleranz die Prüfgröße k senken (Tabelle
  `quantenWKTabelle`). Das wäre eine Änderung der Kettenregeln und ein eigener Schritt.

### 3.3g Sicherheit „99,9999999 %“: die ehrliche Vorlage exakt geprüft, Toleranz wählbar (18.11.2026)

Auftrag des Betreibers: „Es muss zu 99,9999999 Prozent safe sein“, also höchstens einer
von 10⁹ Tokens darf scheitern. Ein Token kann auf zwei Arten scheitern:

1. **Eine Fälschung besteht.** Die Prüfgröße k aus der Tabelle der Kette
   (`quantenWKTabelle`, `wiesner_parameter.py`) hält das für jede Toleranz unter
   **2⁻⁴⁰ ≈ 9 · 10⁻¹³** je Prüfung, solange weniger als t Validatoren zusammenarbeiten. Das
   sind 99,99999999991 %, also schon über dem Auftrag.
2. **Eine ehrliche Vorlage fällt durch, und der Wert ist verloren.**
   - Die Tabelle rechnete hier bisher mit 10⁻⁶ und einer angenommenen Fehlerrate.
   - Nachgerechnet: Für den ersten Token im Hauptnetz (ein Validator, k = 840, ein Schuss
     je Stelle) läge das Durchfallen bei einer Fehlerrate am oberen Rand der Messung bei
     rund 10⁻³. **Das reicht nicht.**

**Lösung: keine Schätzung, sondern die Zählung der Kette, vor der Ausgabe.**
- Bei S stehen beide Stichproben bei der Ausgabe fest; ihr Hash steht auf der Kette. An der
  gewählten Stelle b prüft die Kette genau in den Messbasen y der Inhaberin
  (`quantenSToken.einloesen`: d ⊕ b ⊕ c = y).
- Treffer (y = u) und Fehler (x ≠ t) je Stelle kennt das Werkzeug also schon vor der
  Ausgabe.
- `quantenSEhrlichBesteht` rechnet das Urteil der Kette (`quantenWUrteil`) für **beide**
  Annahmestellen nach. Das Werkzeug gibt nur aus, wenn die ehrliche Vorlage an beiden
  besteht. Sonst bricht es ab, bevor ein Validator bestätigt oder etwas auf die Kette geht.
- Das gilt in beiden Wegen (eine Messung, Abschnitte) und **bei jeder Toleranz**, auch beim
  Standard 4 %.
- **Am Messfehler fällt eine ehrliche Vorlage damit nie mehr durch.** Was bleibt, ist kein
  Messrisiko: Validatoren, die zurückhalten (v − t + 1, Prüfauftrag W8), oder Fehler in
  der Software.

**Toleranz wählbar** (`aequitasd quanten-s-ausgabe --toleranz`, Pilot-Eingabe `toleranz`):
- Die Kette kennt 2, 3, 4, … %; eine andere Zahl weist das Werkzeug sofort ab.
- Kleinere Toleranz heißt kleinere Prüfgröße, bei gleicher Fälschungsschranke 2⁻⁴⁰:

| 3 von 4 Validatoren | Prüfgröße k | Qubits je Token |
|---|---|---|
| 4 % (Standard) | 5.200 | 10.400 |
| 3 % | 2.880 | 5.760 |
| 2 % | 1.760 | 3.520 |

- Bei kleiner Toleranz verlangt die exakte Probe kleine Fehler. Mit der Mehrheit aus
  5 Schüssen (3.3f, 0,04 % auf dem Chip) besteht sie bei 2 % meist; sonst gibt das
  Werkzeug nicht aus, und es wird neu gemessen.

**Tests** (`quanten_s_toleranz_test.go`):
- Die Probe zählt wie die Kette: Grenze genau auf der Toleranz, je Stelle einzeln, unter 4
  Treffern nicht bestanden.
- Missbrauch: Eine Messung, deren ehrliche Vorlage nicht bestünde, bricht ab, bei 2 % und
  bei 4 %, in beiden Wegen. Danach liegt nichts auf der Kette und nichts bei den
  Validatoren.
- Gutfall mit der **echten** Tabelle: Mit 1.760 Stellen gibt das Werkzeug bei 2 % aus
  (Doppelvorlage abgewiesen, ehrliche eingelöst), bei 4 % lehnt es dieselbe Größe ab.
- Eine Toleranz, die die Kette nicht kennt, wird abgewiesen.
- Gegenprobe: 7 Mutationen, alle rot. `go test -race` grün.

**Pilot mit 2 % und k = 1.760** (Lauf
[44372154798](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44372154798),
Rauschmodell, Mehrheit aus 5, je Validator ein eigener Auftrag, 3.520 Qubits je Token statt
10.400), beide Tests grün:

| Test | Fehler in den Stichproben | Doppelvorlage | ehrliche Vorlage | Werkzeug | danach |
|---|---|---|---|---|---|
| alle vier (246,1 s) | 0/1.739 | 1m23s, 1.272/min, **abgewiesen** | 2m20s, 756/min, **eingelöst** | 3m46s | 4 Knoten gleich, Höhe 2120 |
| ein Knoten fällt aus (238,0 s) | 0/1.802 | 1m36s, 1.101/min, **abgewiesen** | 2m01s, 872/min, **eingelöst** | 3m43s | 3 Knoten gleich, Höhe 1577 |

- Ein Token braucht **ein Drittel der Qubits**, und das Werkzeug ist in knapp 4 statt 11–13
  Minuten durch. Der ganze Pilot lief 8 statt rund 24 Minuten.
- Die exakte Probe lief in beiden Tests mit und ließ die Ausgabe zu.
- SHA-256: `ergebnis.txt` `ca3c511b…e88a1b9`, `ausgabe.txt.gz` `3b79038c…8fb59929`.

**Auf echten IBM-Chips mit 2 % und k = 1.760** (Lauf
[44388535029](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44388535029),
`main` nach #590 (3076da22), freigegeben, Mehrheit aus 5, 8 Aufträge), beide Tests grün:

| Ausgabe, Validator 1 / 2 / 3 / 4 | Aufträge | Fehler mit Mehrheit | nur erster Schuss |
|---|---|---|---|
| 1 | `dbv0dn7f8rhs73bn383g`, `dbv0drn82tmc73du74b0`, `dbv0dveps3fc73bjlh4g`, `dbv0e35arooc73fko9k0` | 0/458, 0/426, 0/407, 0/466 | 2, 4, 2, 1 |
| 2 | `dbv0e7darooc73fko9rg`, `dbv0ednf8rhs73bn399g`, `dbv0ehv82tmc73du75d0`, `dbv0em5arooc73fkoae0` | 0/419, 0/431, 0/440, 0/450 | 3, 4, 2, 6 |
| zusammen | | **0/3.497** | 24/3.497 (0,69 %) |

| Test | Fehler in den Stichproben | Doppelvorlage | ehrliche Vorlage | Werkzeug | danach |
|---|---|---|---|---|---|
| alle vier (230,1 s) | 0/1.757 | 1m28s, 1.196/min, **abgewiesen** | 1m59s, 885/min, **eingelöst** | 3m30s | 4 Knoten gleich, Höhe 2153, StateRoot `1453e0db…dea045b` |
| ein Knoten fällt aus (252,4 s; Knoten 2 vor der ersten Vorlage) | 0/1.740 | 1m55s, 920/min, **abgewiesen** | 1m40s, 1.058/min, **eingelöst** | 3m56s | 3 Knoten gleich, Höhe 1848, StateRoot `1dc49c00…8b97de4` |

- Kein einziger Fehler mit Mehrheit auf dem Chip; mit einem Schuss wären es 24 gewesen.
- SHA-256: `ergebnis.txt` `92379fce…d8b27f0`, `ausgabe.txt.gz` `71535e4a…d918fe7c`
  (Artefakt `pilot-44388535029`).
- **Welcher Chip?** Die Kopfzeile der Messung nennt `ibm_marrakesh`, der erste Test
  `ibm_fez`. Beides stimmt, aber nur für je einen Auftrag: Jeder Auftrag wählt sein Gerät
  selbst (`least_busy`), und die Messung nannte bisher nur das Gerät des **letzten**
  Auftrags (Kopfzeile: Ausgabe 2, Validator 4; `basen_1.json`: Ausgabe 1, Validator 4).
  Belegt ist also: Die Aufträge liefen auf mindestens zwei Chips, `ibm_fez` und
  `ibm_marrakesh`. Für die anderen sechs Aufträge steht das Gerät nicht im Protokoll; es
  lässt sich mit den Job-Nummern im IBM-Konto nachsehen. Seit #591 steht das Gerät in jeder
  Zeile, und Kopfzeile und `basen_<a>.json` nennen alle Geräte.
- Dasselbe gilt für die Chip-Angaben der früheren Läufe in 3.3e und 3.3f (44341842380,
  44356741223, 44367782047): belegt für den letzten Auftrag, nicht für jeden.

### 3.3h Stichprobe erst nach der Bindung (v2): Pilot mit vier Validatoren (19.11.2026)

Der Ausweg aus Befund 5i (`DEZENTRAL.md` 16): Bei der Ausgabe kennt niemand eine Stelle
der Stichproben, auch das Werkzeug nicht. Jeder Validator meldet je Annahmestelle nur
einen gesalzenen Teil-Hash und eine Zählung. Stichproben und Salze gibt er erst heraus,
wenn Bindung und Wahl auf der Kette stehen und final sind. Die Kette rechnet die Bindung
beim Anmelden aus den Salzen nach.

Im Hauptnetz gilt diese Regel seit **19.11.2026 19:30 UTC** (Beleg unten). Für den Pilot
wurde sie vorher nur im Testprozess der vier Knoten scharf geschaltet (`AEQUITAS_QS_V2=1`,
Eingabe `stichprobe_v2 = ja`). Sonst ist alles wie im Betrieb: eigene Datenbank je Knoten, echtes
HTTP, je ein Validator-Dienst, 3 von 4.

**Lauf [44564851989](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44564851989)**
(Zweig `claude/weiter-uw2p42`, 9b6a4a95; Rauschmodell `fake_marrakesh`, Mehrheit aus 5,
k = 1.760, Toleranz 2 %, je Validator ein eigener Auftrag, 8 Aufträge), beide Tests grün:

| Test | Fehler (Zählungen der Validatoren) | Stichproben nach der Bindung | Doppelvorlage | ehrliche Vorlage | Werkzeug | danach |
|---|---|---|---|---|---|---|
| alle vier (250,3 s) | 0/1.771 | 1.760/1.760 Stellen, nach 12 s | 1m29s, 1.183/min, **abgewiesen** | 2m08s, 822/min, **eingelöst** | 3m50s | 4 Knoten gleich, Höhe 2355, StateRoot `7952e246…dafa058` |
| ein Knoten fällt aus (232,3 s; Knoten 2 vor der ersten Vorlage) | 0/1.753 | 1.760/1.760 Stellen, nach 12 s | 1m45s, 1.003/min, **abgewiesen** | 1m36s, 1.103/min, **eingelöst** | 3m37s | 3 Knoten gleich, Höhe 1683, StateRoot `1b1596bf…3fc05de` |

- Die Fehlerrate kommt jetzt aus den **Zählungen der Validatoren**. Das Werkzeug sieht
  keine Stelle der Stichproben, bevor Bindung und Wahl final sind.
- Die Stichproben kamen 10 s nach der Wahl, also nach der Finalität der echten Knoten.
  Danach ging es so schnell wie bisher (3.3g: 3m30s–3m56s für das Werkzeug).
- Die Messungen der Aufträge: Ausgabe 1 `8cbda979…`, `ee750313…`, `aa32e459…`,
  `136e54df…`; Ausgabe 2 `70ff2a29…`, `cf16921a…`, `c0c441e4…`, `bb7c45ad…`
  (Rauschmodell, keine IBM-Aufträge).
- SHA-256: `ergebnis.txt` `0d206a82…629b036a`, `ausgabe.txt.gz` `7fc5cd3e…21c7d010`
  (Artefakt `pilot-44564851989`, 90 Tage).
- Zähler je Dienst (eingereicht / abgewiesen / eingesprungen / Nachbarabfragen):
  - alle vier: 13/0/0/30, 21/8/1/59, 20/10/2/51, 6/10/1/20;
  - mit Ausfall: 30/0/1/59, 14/0/1/26, ausgefallen, 16/0/0/36.
- **Lücke in diesem Lauf, danach geschlossen:** Der Test prüfte nur, was das Werkzeug
  meldet. Er wäre auch grün geworden, wenn der v2-Weg still nicht gegriffen hätte. Seit
  #598 liest ein Beobachter im Register jedes Knotens nach: `probe_art = 2` auf
  jedem Stand und beim Anmelden ein nachgerechnetes Salz je Validator. Sonst ist der
  Test rot (`qsKettenSichtPruefen`, mit eigenem Test für 9 Fehlfälle).

**Lauf 2 mit dem Beobachter
[44568310804](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44568310804)**
(40baca7b, sonst gleich), beide Tests grün. In beiden steht **„Auf der Kette: probe_art 2,
4 Validatoren, Anmeldung mit 4 Salzen nachgerechnet“**.

| Test | Fehler (Zählungen der Validatoren) | Doppelvorlage | ehrliche Vorlage | Werkzeug | danach |
|---|---|---|---|---|---|
| alle vier (265,1 s) | 0/1.787 | Stelle 1, 1m29s, 1.184/min, **abgewiesen** | Stelle 0, 2m23s, 737/min, **eingelöst** | 4m05s | 4 Knoten gleich, Höhe 2501, StateRoot `08b61cc9…949c0dc` |
| ein Knoten fällt aus (227,4 s; Knoten 2 vor der ersten Vorlage) | 0/1.772 | Stelle 1, 1m40s, 1.054/min, **abgewiesen** | Stelle 0, 1m36s, 1.098/min, **eingelöst** | 3m32s | 3 Knoten gleich, Höhe 1647, StateRoot `26818b81…02b675c` |

- Hier wählte die Kette Stelle 0 als Stelle der Inhaberin (Lauf 1: Stelle 1). Beide
  Richtungen sind damit belegt.
- Zähler je Dienst (eingereicht / abgewiesen / eingesprungen / Nachbarabfragen):
  - alle vier: 14/0/0/31, 26/6/1/67, 20/10/2/53, **0/34/1/17**;
  - mit Ausfall: 29/0/1/57, 15/0/1/27, ausgefallen, 16/0/0/35.
  - Dienst 3 kam im ersten Test mit keiner Einreichung durch; ein anderer war jedes Mal
    schneller. Die Kette nahm jeden Teil einmal. Das kostet nur Arbeit, nichts wird
    falsch verbucht (wie in 3.3e).
- SHA-256: `ergebnis.txt` `9c6196f1…a03558`, `ausgabe.txt.gz` `ab115112…87d2e9`
  (Artefakt `pilot-44568310804`, 90 Tage).
- Ehrlich dazu: Wie in 3.3e laufen alle Aufträge vom selben Konto im selben Job. Der
  Lauf belegt den Ablauf über echte Knoten, nicht die Trennung der Rollen. Es liegt kein
  Gutachten vor.
- **Stichtag im Hauptnetz:** freigegeben am 19.11.2026 für **19:30 UTC**
  (`DEZENTRAL.md` 16 Schritt 6). C1-Deploy #107
  ([44584732345](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44584732345))
  um 18:33 UTC, grün.
- **Belegt scharf:** Die Wache
  ([44601606565](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44601606565),
  19:45:58 UTC, grün) las aus `/api/status` von C1: „Quantengeld ab 1794934800 (aktiv),
  probe_v2_ab 1795116600 (aktiv)“.

### 3.3i Erster Token mit Stichprobe v2 im Hauptnetz (20.11.2026, 09:00–09:16 UTC)

Lauf [44691759501](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44691759501)
(`main` a6f46747, `quanten-s-ausgabe.yml`, Weg `abschnitte-v2`, Toleranz 2 %). Freigaben:
`quantenhardware` 08:05 UTC, `produktion` 09:00 UTC. C1 lief mit #602 (Deploy #108, Messverzeichnis).

- **Messung:** IBM `ibm_kingston`, Job `dbvvuols2omc7388hnu0`, 1.104 Qubits in
  69 Schaltkreisen, je Stelle 5 Schüsse mit Mehrheit. Fehler bei passender Basis
  **0/548** (mit einem Schuss wären es 1/548 gewesen). Die Basen der Inhaberin kamen
  aus dem Zufall des Betriebssystems.
- **Ausgabe:** K = `1a1e0800770f600a75ac4c1ab7569bbce493e8514ebdd179a2f300de6f23edd5`,
  n = 1.104, k = 552 je Stelle, Deckung 1 AEQ.
  - Ausgeber und einziger Validator: C1 (`0x3066…42dc`).
  - Inhaberin: `0x314b…9919`.
  - Stellen: 0 = Wegwerfschlüssel (`0x4e6b…afeaf`), 1 = C1. Gewählt wurde Stelle 1.
  - **Stichprobe v2:** Die Fehlerrate kam aus der Zählung des Validators. Die Stichproben
    gab C1 erst nach Bindung und Wahl heraus, als diese final waren (5m02s nach der
    Wahl). Angemeldet wurde mit den Salzen.
- **Doppelvorlage** an Stelle 0: **abgewiesen** (Urteil nach 5m08s, ein Fehlversuch).
- **Ehrliche Vorlage** an Stelle 1 (C1): **eingelöst** (Urteil nach 5m10s), der Wert ging
  an C1 zurück. Gesamtdauer 15m21s.
- **Warum je rund 5 Minuten:** Der Dienst gibt Stichproben und Anteile erst heraus, wenn
  Bindung bzw. Anmeldung final sind (`quanten_w_validator.go`, „reif“). Im Hauptnetz
  dauert das rund 5 Minuten; im Pilot sind es Sekunden. Die Anzeige „offen 0 von 552“
  stand so lange still, dann kam das Urteil. Das ist gewollt: kein Anteil vor der
  Finalität.
- **Neu gegenüber dem 17.11. (3.3c):**
  - Keine Annahmestelle und nicht das Werkzeug kennen vor Bindung und Wahl eine Stelle
    der Stichproben; die Kette rechnet die Bindung beim Anmelden aus den Salzen nach.
    Damit greift Befund 5i bei diesem Token nicht.
  - Halb so viele Qubits wie damals (1.104 statt 1.680), bei 2 % statt 4 % Toleranz.
  - Kein Messfehler bei passender Basis.
- **Ehrlich dazu:**
  - Präparation und Messung liefen im selben IBM-Auftrag beim Betreiber (Vorführung).
  - C1 ist der einzige Validator und kannte damit den ganzen Token.
  - Die Inhaberin und die Wegwerf-Stelle lebten nur im Werkzeug.
  - Es liegt kein Gutachten vor.

### 3.3j Getrennte Rollen: Inhaberin, Ausgeber, Annahmestelle (20.11.2026)

Bis hier spielte das Ausgabe-Werkzeug alle Rollen außer den Validatoren: Es erzeugte den
Schlüssel der Inhaberin, las ihr x, band, wählte und legte vor. Jetzt hat jede Rolle ihren
eigenen Befehl und Schlüssel (`DEZENTRAL.md` 17):

- **Die Inhaberin** zieht ihre Basen selbst und gibt dem Ausgeber nur eine öffentliche
  Bestellung. Das Paket des Ausgebers prüft sie gegen die Unterschriften der Validatoren
  und gegen die Kette, bevor sie x liest. Sie unterschreibt die Vorlage nur für die
  gewählte Stelle.
- **Der Ausgeber** hat keinen Schlüssel der Inhaberin und sieht x nie. Einlösen kann er
  ihren Token nicht.
- **Die Annahmestelle** prüft die Vorlage gegen die Kette, bevor sie einen Validator
  fragt. Sie lädt ihre Stichprobe mit dem eigenen Schlüssel, rechnet das Urteil vorab
  nach und wartet auf das Urteil der Kette.
- **Belegt:** Auf vier echten Knoten (3 von 4) grün, auch mit Ausfall eines Validators.
  Jede Rolle sprach mit einem anderen Knoten. Eine böse Inhaberin versuchte zuerst die
  Doppelvorlage; sie wurde vorab und von der Kette abgewiesen. Danach wurde ehrlich
  eingelöst. Dazu kommen 15 Tests mit Fälschungen und 24 Mutationen, alle rot.
  - Auf GitHub in voller Größe (k = 1.760, n = 3.520, Toleranz 2 %, Rauschmodell):
    Lauf [44728392142](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44728392142),
    beide Tests grün. Fehler bei passender Basis 0/1.763 und 0/1.736; Doppelvorlage vorab
    und von der Kette abgewiesen, ehrlich eingelöst (3m40s und 4m03s); Gleichstand aller
    lebenden Knoten (`DEZENTRAL.md` 17).
- **Kein neuer Konsens.** Die Kette prüfte das schon. Neu sind nur öffentliche Felder in
  der Ansicht des Tokens (Wurzel, Proben, d, c).
- **Ohne großes Paket (R4a):** Die Inhaberin holt ihren Token nur mit seiner Kennung K
  direkt bei den Validatoren ab. Die Vorlage für die Annahmestelle ist kompakt (486 Byte
  bei n = 256, rund 1,3 KB bei n = 1.104) und passt in einen QR-Code. Die Blätter holt die
  Stelle selbst. Das ist die Grundlage für die App als Inhaberin.
- **Ehrlich dazu:**
  - Im Hauptnetz ist es noch nicht gelaufen.
  - Getrennte Schlüssel sind noch keine getrennten Menschen. Die App kann jetzt
    Inhaberin sein (R4b: bestellen, abholen mit K, Vorlage als QR-Code; App #88, #89),
    ist aber in keinem ausgelieferten Build und noch keinen Weg im Hauptnetz gegangen.
    Ein zweiter Mensch als Validator (A5) fehlt.
  - Der Ausgeber sieht die Basen y und damit vorab, an welcher Stelle die Inhaberin
    einlösen will (nicht x).
  - Fällt ein Validator nach der Bindung aus, bevor die Stelle ihre Stichprobe geladen
    hat, muss sie warten, bis er zurück ist.

### 3.3k Erster öffentlicher Nachweis nach dem Ende im Hauptnetz (21.11.2026, 12:29–12:51 UTC)

Lauf [44904131085](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44904131085)
(`main` 4b252ab5, `quanten-s-ausgabe.yml`, Weg `abschnitte-v2`, Toleranz 2 %, Deckung
1 AEQ). C1 lief mit #612 (Deploy 44882966225, 11:04 UTC). Freigaben `quantenhardware`
und `produktion` durch den Betreiber.

- **Messung:** IBM `ibm_kingston`, Job `dc0otnthdvas73bjqts0`, 1.104 Qubits in
  69 Schaltkreisen, je Stelle 5 Schüsse mit Mehrheit. Fehler bei passender Basis
  **0/540** (mit einem Schuss wären es 2/540 gewesen). Die Messung ging nur
  versiegelt auf den Schlüssel von C1 durch GitHub (R5).
- **Ausgabe:** K = `a5acd86829f9b42e88e1f53c537e20f364bdc498f9e62919fc7fdfd5b161ab0f`,
  n = 1.104, k = 552 je Stelle, Stichprobe v2.
  - Ausgeber und einziger Validator: C1 (`0x3066…42dc`); Inhaberin `0x68c6…0217`.
  - Stellen: 0 = C1, 1 = Wegwerfschlüssel (`0x1f2d…3d07`). Gewählt wurde Stelle 0.
  - Wurzel `20aebc24…f77c`, Proben `167c8af9…fe2c` und `132c68ba…b254`.
- **Doppelvorlage** an Stelle 1: **abgewiesen** (Urteil nach 10m13s, danach ein
  Fehlversuch auf dem Token).
- **Ehrliche Vorlage** an Stelle 0 (C1): **eingelöst** (nach 15m23s), der Wert ging an
  C1 zurück. Die Stichproben gab C1 erst nach der Finalität von Bindung und Wahl
  heraus (5m02s), wie in 3.3i.
- **Nachweis nach dem Ende (neu):** Rund 5 Minuten nach dem Einlösen war das Ende
  final. C1 legte seinen Abschnitt offen; das Werkzeug holte ihn und rechnete nach:
  - Die offengelegten Werte ergeben die **Wurzel der Ausgabe**, die Salze die
    **Proben** – geprüft gegen das, was vor dem Anmelden auf der Kette stand
    (`wurzel_gegen: kette`).
  - Über alle 1.104 Stellen: **0 Fehler bei 540 Treffern** (passende Basis). An der
    gewählten Stelle: **0 Fehler bei 283 Treffern**.
  - Das passt zur Zählung bei der Ausgabe (0/540) – jetzt aber für jeden aus der
    offengelegten Präparation nachrechenbar, nicht nur aus der Zählung des Validators.
- **Panne und Behebung:** Der letzte Schritt des Laufs, der den Nachweis über die
  öffentliche Adresse holen und als Artefakt ablegen sollte, brach ab. Das Ergebnis des
  Werkzeugs begann mit einer Warnzeile des Knotens (`[RPC] ⚠ AEQUITAS_RPC_QUIET_TX=1 …`):
  Auf C1 ist dieser Leistungsschalter gesetzt, und jeder `aequitasd`-Befehl im Container
  schrieb die Warnung beim Start auf stdout statt stderr. Am Ergebnis selbst ändert das
  nichts (es steht vollständig im Protokoll des Laufs). Behoben in PR „stdout rein“:
  Warnung nach stderr, ein Test startet das Werkzeug mit genau der Umgebung von C1, die
  Workflows lesen das JSON ab der Zeile `{`. Neuer Workflow `quanten-s-nachweis.yml`
  holt jeden Nachweis ohne Schlüssel und Freigabe, rechnet ihn nach und legt ihn als
  Artefakt ab. Der Validator hält den Nachweis nur im Speicher; ein Neustart von C1
  löscht ihn.
- **Ehrlich dazu:**
  - Der Nachweis zeigt, dass genau diese Präparation bei der Ausgabe festgelegt war, in
    welchen Basen geprüft wurde und wie viele Fehler es gab. Er zeigt **nicht**, dass
    ein Quantenrechner gemessen hat (R7).
  - Präparation und Messung liefen im selben IBM-Auftrag beim Betreiber (Vorführung).
  - C1 ist der einzige Validator und kannte damit den ganzen Token.
  - Die Inhaberin und die Wegwerf-Stelle lebten nur im Werkzeug.
  - Es liegt kein Gutachten vor.

### 3.3l Fehlerbild gegen die Kalibrierung (22.11.2026)

Der Nachweis nach dem Ende (3.3k, R7) zeigt, dass genau diese Präparation bei der Ausgabe
festgelegt war. Er zeigt nicht, dass ein Quantenrechner gemessen hat. Das Fehlerbild
(`fehlerbild.py`) ist ein zweiter, unabhängiger Blick: Passen die Fehler der einzelnen Schüsse
zur Tageskalibrierung der benutzten Qubits?

- **Was gemessen wird** (`hardware_ibm.py`, je Stelle): das physische Qubit, alle R Schüsse
  (Standard 5), der Auslesefehler des Qubits und die Fehler der 1-Qubit-Gatter davor, beides
  aus dem Target des Chips.
- **Drei Prüfungen:**
  1. **Passende Basis:** beobachtete Fehler je Schuss gegen die erwarteten (Auslese plus
     Gatter). Viel weniger heißt „zu glatt“ und passt zu einer Rechnung ohne Rauschen. Viel
     mehr passt zu Drift, falschen Qubits oder einem Eingriff.
  2. **Lage der Fehler:** Sitzen sie dort, wo die Kalibrierung sie erwartet
     (Likelihood-Verhältnis)? Bei wenigen Fehlern schwach, deshalb nur berichtet.
  3. **Andere Basis:** Jeder Schuss ist eine neue Präparation, also ein fairer Münzwurf,
     unabhängig von t und von den anderen Schüssen. Kopierte Schüsse oder ein
     durchgesickertes t fallen auf.
- **Was veröffentlicht wird:** nur die Summen. Sie verraten keine Stelle, kein t und kein x
  und dürfen deshalb sofort raus. Der Workflow `quanten-s-ausgabe.yml` legt sie als Artefakt
  `fehlerbild-<Lauf>` ab und schreibt sie in die Zusammenfassung. Die rohen Schüsse verlassen
  den Messlauf nicht.
- **Belegt:**
  - Selbsttest (in der CI): Eine ehrliche Messung passt; zu glatt, zu viel Rauschen,
    kopierte Schüsse, durchgesickertes t, fehlende Gatterfehler und kaputte Eingaben werden
    erkannt.
  - Rauschmodell `FakeMarrakesh`, drei Läufe zu 1.104 Stellen: jeweils „passt“ (8, 10 und
    10 Fehler je Schuss, erwartet ~10,6–11,2).
- **Ehrlich:** „Passt“ heißt nur „kein Widerspruch“. Ein Simulator mit demselben Rauschmodell
  besteht genauso; die Probe im Rauschmodell zeigt genau das. Ausschließen lässt sich damit
  die einfache Fälschung und ein Chip, der schlechter misst als kalibriert. Im Hauptnetz ist
  es noch nicht gelaufen (der nächste `quanten-s-ausgabe.yml`-Lauf liefert das erste).

### 3.4 Bargeld-Verhalten ohne Quanten (Offline-Bargeld)

Was Menschen sich von Quantengeld wünschen, ist meist offline zu zahlen, wie mit
Bargeld. Das leistet das Offline-Bargeld (`docs/OFFLINE_BARGELD.md`):
- **Mechanismus:** Ein Geräteschlüssel im Android-Keystore, ein Pfand und die Haftung
  über „ein Mensch, ein Konto“.
- **Grenze:** Betrug ist nicht unmöglich, aber beweisbar, und der Betrüger haftet.
- **Stand:** Stufe 1 ist gebaut und schläft. Offen sind Stufe 2 (Knoten-API), 3
  (App-Modul, QR), 4 (Schuld aus künftigen Eingängen) und 5 (Stichtag). Das ist nur
  Software.

## 4. Was heute nicht behauptet werden darf

- „Aequitas ist Quantengeld“ oder „Quantenwährung“. Richtig ist:
  **quantensicher** (Kryptografie) plus **Ausgabestelle für Quantengeld** (schlafend)
  plus **Vorführung auf echter Hardware**.
- „Physikalisch unfälschbar“ für AEQ. AEQ ist durch Mathematik geschützt (ML-DSA),
  nicht durch Physik.
- Eine Note aus einem Cloud-Lauf als Wertträger. Sie beweist den Ablauf, nicht die
  Echtheit.
- „Ein S-Token kann an zwei Stellen nicht doppelt ausgegeben werden“, ohne Bedingung.
  - **Bisheriger Weg** (`probe_art` 0): Das gilt nur, solange keine Annahmestelle (und
    nicht das Ausgabe-Werkzeug, das beide Stichproben lädt) vor der Bindung mit der
    Inhaberin zusammenarbeitet (`DEZENTRAL.md` 16, `s_kette.py` Selbsttest 5i).
  - **Stichprobe v2** (`probe_art` 2, Werkzeug `--stichprobe-v2`, im Hauptnetz seit
    19.11.2026 19:30 UTC): Vor Bindung und Wahl kennt keine Annahmestelle und nicht das
    Werkzeug eine Stelle der Stichproben. Jeder Validator kennt nur die in seinem
    Abschnitt. Befund 5i greift dann nicht.
  - In beiden Wegen bleibt: Wer einen Abschnitt präpariert, kennt ihn. Heute ist C1 der
    einzige Validator (t = v = 1) und kennt damit den ganzen Token. Dass ihn niemand
    allein kennt, gilt erst mit mehreren Validatoren, die jeder selbst messen (A5).

Diese Unterscheidung ist auch rechtlich klug, denn irreführende Werbung ist angreifbar.
Für die Menschen, die AEQ halten, ist sie ehrlich.

## 5. Das Heute-Paket

| # | Schritt | Wirkung | Aufwand | Wer |
|---|---|---|---|---|
| 1 | Deploys #534 (Mindestzahl 5 Prüfer) und #535 (Noten je Zeile) freigeben | Ausgabestelle auf letztem Stand | Minuten | du |
| 2 | **Quantenschutz ab Werk** in der App (Zusage automatisch nach der Registrierung), Release | jede neue Wallet quantensicher, ohne Zutun | Tage | ich, dann dein Release |
| 3 | Dich registrieren → Stufe 1 wird scharf | Registrierung vollständig hybrid geprüft | Minuten | du |
| 4 | PQ-Schlüssel für die Biometrie-Sicherung (Stufe 0) | Gesichtsdaten im Backup quantensicher | Minuten | du |
| 5 | **Vorführung „Quanten-Note Nr. 1“** auf IBM (Abschnitt 3.3) | echter Ablauf auf echter Hardware, echte Fehlerraten für Variante W | Tage | ich; Lauf mit deiner Freigabe |
| 6 | Offline-Bargeld Stufe 2 | Weg zu „offline zahlen“ für alle | Wochen | ich |
| 7 | Tresor-Lücken schließen (Prüfer, Wächter, Treuhand vom Tresor aus) | vollständiger Umzug in den Tresor möglich | Wochen | ich |
| 8 | Partner und Förderung (Vorlagen in `ANSCHREIBEN.md`) | Weg zu Noten mit Speichern von Stunden | Monate | du schickst ab |

**Am Ende von 1–5** kann Aequitas ehrlich sagen: *Jedes neue Konto ist ab Werk
quantensicher. Die Kette hat eine laufende Ausgabestelle für Quantengeld. Der ganze
Ablauf einer Quanten-Note ist auf echter Quantenhardware vorgeführt.* Jeder dieser drei
Sätze ist nachprüfbar: im Code, auf der Kette und im veröffentlichten Lauf.
