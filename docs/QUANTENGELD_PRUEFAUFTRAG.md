# Quantengeld – Prüfauftrag für eine externe Begutachtung

Stand 15.11.2026 (Varianten W und S: 16.11.2026 abends). Bevor ein Parametersatz freigeschaltet oder eine Einmal-Unterschrift
zum Einlösen zugelassen wird, prüft eine **unabhängige** Stelle (Kryptografie und
Quanteninformation, nicht am Projekt beteiligt) die folgenden Punkte. Das Ergebnis wird
veröffentlicht; ohne ein Gutachten ohne offene kritische Punkte stimmen die Prüfer nicht
für eine Freischaltung.

## Gegenstand

| Teil | Ort | Was es tut |
|---|---|---|
| Lightning-Noten (Zhandry 2025, Gruppenaktionen) | `forschung/quantengeld/`, `x/humanity/keeper/quanten_note.go` | Prägen, Seriennummer, Verifikation |
| Anmeldung und Register | `quanten_geld.go` | Note genau einmal, Deckung in AEQ, Anteile am Topf |
| Umlaufabgabe im Topf | `wirtschaft.go` (`umlaufArt`), `quanten_geld.go` | Noten verlieren anteilig wie jedes Guthaben |
| Einlösen | `quanten_einloesen.go` | Einmal-Unterschrift der Note → Auszahlung des aktuellen Werts |
| Freischaltung | `quanten_geld.go`, `pruefer_pq.go` | ⅔ der Prüfer, mindestens 5 verschiedene, hybrid unterschrieben, N ≥ 2²⁵⁶, 30 Tage Vorlauf |

## Fragen an die Begutachtung

1. **Fälschungssicherheit der Noten.** Hält die Konstruktion für den vorgeschlagenen Satz
   (Gruppenordnung N, Gruppenaktion, Parameter) gegen bekannte Angriffe auf
   Gruppenaktionen (Kuperberg-Sieb, Regev, hybride Angriffe) und gegen Angriffe auf das
   Lightning-Argument selbst (zwei gültige Noten mit derselben Seriennummer)?
   Sicherheitsniveau in Bit, klassisch und quantenmechanisch.
2. **Verifikation unter Rauschen.** Mit welcher Wahrscheinlichkeit besteht eine echte Note
   auf realer Hardware, und mit welcher eine Fälschung? Reicht der Abstand (heute auf IBM:
   0,81 gegen 0,22) bei der vorgeschlagenen Schwelle, und wie oft muss geprüft werden?
3. **Einmal-Unterschrift.** Für den Satz, der eingelöst werden soll: Ist das Verfahren
   eine echte Einmal-Unterschrift (nach einer Unterschrift keine zweite, auch nicht durch
   Kopieren vor dem Unterschreiben)? Unter welchen Annahmen (iO, LWE, Gruppenaktionen)?
   Passt der klassische Prüfer (`quantenEinmalPruefer`) genau zur Seriennummer der Note?
4. **Kettenseite.** Kann ein Erzeuger oder Peer durch die Felder einer Transaktion
   (Anteile, Betrag, Seriennummer, K) Geld aus dem Topf ziehen, eine Note doppelt
   anmelden oder einlösen, oder die Abgabe des Topfs umgehen? (Tests:
   `quanten_*_test.go`, `kein_entzug_test.go`.)
5. **Anteile und Abgabe.** Ist die Rechnung (`quantenAnteileFuer`, `quantenWert`)
   rundungsfest, nie zugunsten des Einlösenden, und hält Topf = Summe der Noten?
6. **Freischaltung.** Genügen ⅔ der Prüfer, mindestens 5 verschiedene (hybrid: EIP-191 +
   ML-DSA-65), Vorlauf und N ≥ 2²⁵⁶ als Schutz gegen eine voreilige oder böswillige
   Freischaltung? Ist 5 als Untergrenze angemessen, solange es nur wenige Prüfer gibt?

## Variante W: privat prüfbare Noten (Wiesner) mit verteiltem Geheimnis

Stand 16.11.2026 abends (W5, nach W2c und W2d). Variante W braucht keine Gruppenaktion und
keinen großen Quantenrechner: Eine Note ist eine Folge von BB84-Zuständen, und das
Geheimnis jeder Stelle liegt als Shamir-Anteile bei den Validatoren der Note. Sie ist
**schlafend** eingebaut (`quantenWAbUnix = MaxInt64`). Freischalten (W6) setzt ein
Gutachten zu den folgenden Punkten voraus, ebenso den Partnerversuch W4 mit einem echten
Speicher.

| Teil | Ort | Was es tut |
|---|---|---|
| Regeln, ausführbar | `forschung/quantengeld/wiesner_kette.py` | Spezifikation mit Missbrauchsfällen; Prüfvektor für den Knoten |
| Prüfgrößen | `forschung/quantengeld/wiesner_parameter.py` | Kloner und Breidbart am Zustandsvektor, exakte Annahme nach der Regel der Kette, Tabelle k_note |
| Regeln im Knoten | `quanten_w.go`, `quanten_w_register.go` | Prüfsummen und Merkle, Shamir über 2¹²⁷−1, Hash-Ketten-Zufall, Aufforderung, Urteil, Übergänge, Offenlegen in Teilen |
| Einbindung | `quanten_w_kette.go` | 6 Transaktionsarten, Unterschriften, Buchungen (W-Topf, Pfand-Topf, Grundeinkommen), Register, StateRoot, Snapshot |
| Dienst der Validatoren | `quanten_w_validator.go` | Tresor, Bestätigung, Herausgabe erst nach der Finalität, Hintergrund (Zufall, Offenlegen, Fristen) |
| Prägen ohne Vertrauen | `wiesner_praegen.py`, `wiesner_hardware.py` (`note_fern`) | Validatoren messen verschränkte Hälften; niemand kennt die ganze Note |
| Messungen | `HEUTE.md`, `hardware_ibm.py --wiesner` | IBM `ibm_marrakesh`: 0,7 % Fehler je Treffer ohne Wartezeit; mit 20 µs und Teleportation 39 % |

**Die Prüfgrößen (W2c, #546)** – nach dem Befund, dass die frühere Toleranz von 15 %
gegen einen optimalen Kloner gebrochen war:
- Jede Note hat ihre Toleranz tol ∈ {2, 3, 4, 5, 6, 8, 10} %; die Validatoren bestätigen sie
  mit.
- Geprüft wird beim Einlösen genau k_note = Tabelle[tol][f16] mit f16 = ⌈16(t−1)/v⌉. Die
  Tabelle ist ganzzahlig und exakt gerechnet: Ein Fälscher, dessen Fehlerrate je Treffer
  (1 − f)·14,64 % ist, besteht höchstens mit 2⁻⁴⁰. Felder, für die das nicht geht, sind 0;
  dann wird das Prägen abgewiesen.
- Prüfungen dürfen kleiner sein, mindestens max(8, ⌈n/255⌉), und lassen k_note Stellen
  und den letzten Wert der Hash-Ketten für das Einlösen frei.
- Offengelegt wird in Teilen zu höchstens 64 Stellen; geurteilt wird über alle k.
- Erschöpft (weniger als k_note frei oder die Ketten verbraucht): tot, der Wert geht ans
  Grundeinkommen.

**Fragen an die Begutachtung (Variante W):**

1. **Fälschung mit einer Note.**
   - Ist der phasenkovariante Kloner (14,64 % Fehler je Kopie, in beiden BB84-Basen) der
     beste Angriff auf das Schema, in dem jede Stelle **nur einmal** gefragt wird und die
     Kopien an verschiedenen Stellen geprüft werden? Oder gibt es verschränkte Angriffe
     über mehrere Stellen, die besser sind? Pastawski u. a. (PNAS 2012) beweisen die
     Schwelle für ihre cv-qtickets; gilt die Schranke exp(−h·D(tol‖0,146)) auch hier?
   - Trägt die Tabelle (`wiesner_parameter.py`, `quantenWKTabelle`) die behauptete
     Schranke 2⁻⁴⁰? Sie rechnet je Kopie und lässt unentschiedene Prüfungen wiederholen.
   - **Eigene Antwort zur Prüfung (17.11.2026, `forschung/quantengeld/kohaerent.py`):**
     Gegen jeden **nicht-adaptiven** Angriff, auch verschränkt über alle Stellen, gilt
     eine strenge Schranke für „beide Kopien bestehen“. Argument: Das SDP über alle
     Kanäle zerfällt Stelle für Stelle, weil Produkte dual zulässiger Lösungen dual
     zulässig bleiben (wie Molina, Vidick, Watrous 2012). Je Stelle zählt nur F_A + F_B
     ≤ 1 + 1/√2. Dazu kommen eine Chernoff-Schranke und das Bedingen auf die feste
     Prüfgröße. Alle Einträge der Tabelle halten 2⁻⁴⁰ mit 27–43 Bit Reserve, zum
     Beispiel k = 840 (f = 0): 2⁻⁶⁷; k = 5.200 (t = 3, v = 4): 2⁻⁶⁷·⁵. Gegenprobe: Der beste
     Produktangriff, auch asymmetrisch, liegt mit 2⁻⁸⁰ darunter.
     Korrektur dabei: „je Kopie“ ist nicht vorsichtig, weil ein asymmetrischer Kloner eine
     Kopie fast perfekt macht. Maßgeblich ist „beide bestehen“.
     **Bitte prüfen:** den Beweis, die Ein-Stellen-Grenze F_A + F_B ≤ 1 + 1/√2 (auch
     asymmetrisch) und den **adaptiven** Angriff. Dabei wird Kopie B erst nach der Prüfung
     von A bearbeitet; deren Stellen und Basen stehen dann offen, und B wird nur noch an
     anderen Stellen gefragt. Unser Argument dazu (`kohaerent.py`, ADAPTIV): Jede
     Rückmeldung betrifft nur ihre Stelle, der Prüfer ist also ein Produkt von Prüfern mit
     zwei Runden. Mit der Dualität für Strategien (Gutoski und Watrous 2007) bleibt das
     Optimum ein Produkt. Je Stelle wird A oder B geprüft, nie beide, und beide Runden
     zusammen sind ein Kloner. Also gilt dieselbe Schranke. Trägt das?
   - Mehrere Prüfungen nacheinander: Wächst die Erfolgsaussicht über die Fehlversuche
     hinweg? Drei Fehlversuche sperren Prüfungen, aber nicht das Einlösen.
2. **Absprachen.**
   - Weniger als t Validatoren erfahren nichts über eine Stelle (Shamir). Bei
     Fern-Präparation kennt aber jeder Validator die Stellen, die er selbst gemessen hat.
     Ist die vorsichtige Annahme f = (t−1)/v richtig, und reicht sie?
   - Darf ein Validator zugleich Prüfer oder Präger sein?
3. **Zufall.** Die Aufforderung folgt aus den Hash-Ketten **aller** v Validatoren, deren
   Anker beim Prägen bestätigt wurden.
   - Ist die Saat für einen Erzeuger, Anfragenden oder weniger als v Validatoren
     unvorhersagbar und unsteuerbar?
   - Schadet es, dass ein Validator den Zufall zurückhalten kann? Er verzögert dann nur:
     Es gibt keine Frist, das Pfand bleibt gesperrt.
   - Lebendigkeit: Fällt ein Validator für immer aus, steht die Note. Ist eine
     Schwellen-Signatur (t von v, eindeutig, z. B. BLS) als Ersatz geboten?
4. **Offenlegen statt gemeinsam Rechnen.**
   - Gibt die Offenlegung der geprüften Stellen (Pfad und t Anteile je Stelle) etwas über
     die ungeprüften preis? Die gesalzene Prüfsumme hat ein Salz von 124 Bit, je Stelle ein
     eigenes Polynom.
   - Kann ein unehrlicher Präger inkonsistente Anteile verteilen, ohne dass es der eigenen
     Note schadet? Bringen verifizierbare Anteile (Feldman/Pedersen) etwas?
5. **Der Dienst der Validatoren** (`quanten_w_validator.go`).
   - Er gibt den Wert der Hash-Kette erst heraus, wenn die Anfrage **final** ist, und die
     Anteile erst, wenn die Antwort final ist (finalisierter Checkpoint ≥ Höhe, bei der
     der Stand gesehen wurde). Reicht das, damit der Anfragende die Geheimnisse nie vor
     seiner endgültigen Antwort sieht, auch bei Umordnungen im DAG?
   - Der Auftrag des Prägers bindet den Hash der Anteile; der Tresor ist mit einem Schlüssel
     aus dem Signierschlüssel versiegelt. Ist die Ableitung angemessen?
6. **Pfand und Störung.** Das Pfand ist max(1 AEQ, Wert · k / n). Ein falsches Einlösen
   tötet die Note nicht.
   - Kostet jede Störung (Prüfungen oder Einlöseversuche ohne Note) mindestens so viel,
     wie sie dem Halter schadet? Insbesondere: Lassen sich die Hash-Ketten billiger leeren,
     als die kleinste Prüfung ⌈n/255⌉ verhindern soll?
   - Gibt es einen billigeren Weg, eine fremde Note zu entwerten?
7. **Kettenseite.**
   - Kann ein Erzeuger oder Peer durch gerechnete Felder Geld aus W-Topf oder Pfand-Topf
     ziehen?
   - Kann er eine Antwort oder Anfrage wiederholen, eine Note doppelt oder neu prägen,
     Validatoren desselben Menschen einsetzen oder die Umlaufabgabe umgehen?
   - Tests: `quanten_w_test.go`, `quanten_w_kette_test.go`, `quanten_w_validator_test.go`,
     `TestVierKnotenQuantenW_RealDB`; Mutationen zu jeder Regel.

## Variante S: Quanten-Token ohne Quantenspeicher

Stand 16.11.2026 abends (S0–S2b). Grundlage: S-money von Kent, Lowndes, Pitalúa-García und
Rarity, „Practical quantum tokens without quantum memories and experimental tests“, npj
Quantum Information 8, 28 (2022). Die Nutzerin misst die Zustände **sofort** und hält
danach nur Klartext; gültig ist der Token an genau einer von zwei vorher festgelegten
Annahmestellen. Ebenfalls **schlafend** (dasselbe `quantenWAbUnix`).

| Teil | Ort | Was es tut |
|---|---|---|
| Spezifikation | `forschung/quantengeld/s_kette.py` | Ausgabe, Bindung d, Wahl c, Stichproben-Annahmestellen ohne Netz, Einlösen auf der Kette in Teilen, Prüfvektor |
| Regeln im Knoten | `quanten_s.go` | Stichproben-Bindung, Vorlage-Text, Bindung, Wahl, Anmelden, Offenlegen in Teilen, Frist, Fehlversuche |
| Einbindung | `quanten_w_s_kette.go` | 6 Transaktionsarten im W-Modul (gleicher Stichtag, StateRoot, Snapshot) |
| Dienst der Validatoren | `quanten_w_validator.go` (Art „s“) | Bestätigung der Ausgabe, Stichprobe erst nach einer finalen Anmeldung |
| Messung | `HEUTE.md` 3.3b, `hardware_ibm.py --s-token` | IBM `ibm_marrakesh`: 1.680 Qubits, 0,46 % Fehler; angenommen, Doppelvorlage abgewiesen |

**Fragen an die Begutachtung (Variante S):**

1. **Doppelvorlage.** Wer an beiden Stellen vorlegen will, braucht Ergebnisse in beiden
   Basen. Ist die Breidbart-Messung (14,64 % Fehler in beiden Basen) der beste Angriff, auch
   mit verschränkten Strategien über viele Qubits? Gilt damit dieselbe Tabelle wie bei W?
   **Eigene Antwort zur Prüfung (17.11.2026, `kohaerent.py`):** Ja, für jede Messung, auch
   kollektiv über alle Qubits. Je Stelle ist die Summe der Treffsicherheiten in beiden
   Basen höchstens 2 · (1/2 + 1/√8), und das Optimum zerfällt in ein Produkt. Es gilt
   dieselbe strenge Schranke wie bei W. Die Stellen prüfen ohne Netz, Rückmeldungen
   zwischen den Vorlagen sind also höchstens das Bit „angenommen“, und das kostet
   höchstens 1 Bit.
2. **Stichproben-Annahmestellen** (Ergänzung zu Kent u. a.). Jede Stelle kennt (u, t) nur auf
   ihrer geheimen, disjunkten Stichprobe und prüft ohne Netz.
   - Kann eine bestochene Stelle an der anderen Stelle durchkommen?
   - Was verrät die Wahl c, die vorab auf der Kette steht, über die Stelle, und was die
     Bindung d über die Basen der Nutzerin?
3. **Einlösen auf der Kette.**
   - Nur die bei der Ausgabe festgelegte Stelle b darf anmelden, mit der Vorlage, die die
     Inhaberin unterschrieben hat. Reicht das gegen Vorauslaufen (Stichprobe aus der
     Warteschlange kopieren) und gegen eine Stelle, die ohne Vorlage einlöst?
   - Die Unterschrift der Stelle bindet die Zahl der Fehlversuche. Verhindert das, eine
     fehlgeschlagene Anmeldung zu wiederholen, bis der Token tot ist?
4. **Grenzen des Modells.** Kein Bargeld von Hand zu Hand. Die Ausgabe braucht einen
   QKD-Empfänger bei der Nutzerin. In der Vorführung laufen Ausgabe und Messung im selben
   Schaltkreis. Was davon muss vor einer Freischaltung anders werden?
5. **Kettenseite.** Wie bei W, Punkt 7; Tests: `quanten_s_test.go`, `quanten_w_s_kette_test.go`,
   Mutationen zu jeder Regel.
6. **Ausgabe über eine echte Leitung (S3, `forschung/quantengeld/ANALYSE.md` 5c,
   `wiesner_parameter.py` Abschnitt S3).**
   - Grundlage sind Kent u. a. 2022, Schema QT1, Lemma 2, Lemma 3 und Satz 1. Das Werkzeug
     rechnet ihre Abb. 5 nach.
   - Für unsere Stichprobe je Annahmestelle setzen wir die Mehrphotonen-Stellen als
     bekannt an (Anteil ν/γ_det der gemeldeten). Auf den übrigen Stellen hat der Fälscher
     mindestens λ(θ, β) Fehler. Daraus folgt eine exakte Binomialrechnung wie bei W. Ist
     diese Übertragung von Satz 1 auf eine Stichprobe zulässig?
   - Kents Satz 1 wird mit β = 0 angesetzt, weil Basen und Werte aus dem Zufall der
     Validatoren kommen und das Gerät sie nur umsetzt. Welche Seitenkanäle des Senders
     (Wellenlänge, Zeitpunkt, Phase; Tabelle VI, Annahmen A und G) müssen gemessen werden,
     damit das trägt?
   - Die Nutzerin meldet Λ. Danach zieht der Zufall der Kette eine öffentliche Teilmenge
     von höchstens 16.384 Stellen, und darin liegen die geheimen Stichproben. Bleibt der
     Anteil freier Stellen dabei erhalten, auch wenn die Nutzerin Λ geschickt wählt?
7. **Größe der Stichprobe.**
   - Die Annahmestelle lädt nur eine Stichprobe mit mindestens k_note(tol, t, v)
     verschiedenen Stellen (`s_kette.stelle_laden`, fail-closed).
   - Der Knoten prüft die Größe bewusst nicht, denn beim Anmelden ist die Annahme ohne
     Netz schon geschehen (ANALYSE.md 5c).
   - Ist das die richtige Stelle für die Prüfung, oder braucht es eine Regel schon bei der
     Ausgabe? Dort ist die Stichprobe nur als Hash bekannt.

## Was erst die Hardware entscheidet (keine Frage an die Kryptografie)

- **Lebensdauer:** eine Note muss länger halten, als eine Zahlung dauert (heute ~20 µs
  auf IBM-Chips; siehe `forschung/quantengeld/HARDWARE_WEG.md`).
- **Größe:** ein Satz mit N ≥ 2²⁵⁶ braucht einen Verifizierer mit sehr vielen logischen
  Qubits.
- **Weitergabe:** eine Note von Hand zu Hand braucht ein Quantennetz.

Die Prüfer bezeugen diese drei Punkte gesondert, bevor sie für einen Satz stimmen.
