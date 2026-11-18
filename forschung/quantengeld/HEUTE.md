# Quantengeld heute – was unter den heutigen Umständen funktionieren kann

Stand 16.11.2026. Ergänzt `ANALYSE.md` um die Frage: **Was davon kann Aequitas heute
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
   Quantenschutz **automatisch** an ist. Heute muss jeder ihn selbst einschalten. (c)
   lässt sich mit dem vorhandenen IBM-Zugang in Tagen bauen. (d) ist zur Hälfte gebaut.

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
| Ein Cloud-Lauf ist **gegenüber Dritten nicht als echt beweisbar** | Das Ergebnis meldet der Betreiber | Eine „Quanten-Note in der Cloud“ ist nicht sicherer als eine Buchung. Sie zeigt, dass der **Ablauf** funktioniert, schützt aber nichts. |
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

**Live ist nur ein Teil davon.** Mindestzahl und QG3 werden erst mit den beiden
Deploys aktiv, die auf deine Freigabe warten. Das ist die Seite, die eine Kette
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
| [44186826204](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44186826204) | `main` nach #575, **IBM-Chip** (10.400 Qubits) | wartet auf Freigabe in `quantenhardware` | – |

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
- Abhilfe auf dem Zweig (43b9c78): Der Ersatz tritt zurück, sobald ein anderer vorankommt.
- Prüflauf: [44264862815](https://github.com/hanoi96international-gif/Aequitas/actions/runs/44264862815).

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

**Ergebnis:** Variante S läuft mit vier Validatoren (3 von 4) in voller Größe der Tabelle
durch, auch wenn ein Validator ausfällt. Dabei gilt: Messung im Rauschmodell, Knoten in
einem Job, kein Gutachten.

**Grenzen der Nachvollziehbarkeit bis hier:** Bei den ersten Läufen ist nur das Ende des
Protokolls abrufbar (GitHub liefert über die Schnittstelle höchstens 5.000 Zeilen). Ob der
Test ohne Ausfall in den Läufen 44164912648 und 44166377116 grün war, ist darum nicht
belegt. Seit #575 legt jeder Lauf die Meldungen der Tests vollständig in die
Zusammenfassung und hebt `ergebnis.txt` und die ganze Ausgabe `ausgabe.txt.gz` 90 Tage als
Artefakt auf, mit SHA-256 in der Zusammenfassung.

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
