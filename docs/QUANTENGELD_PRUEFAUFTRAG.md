# Quantengeld – Prüfauftrag für eine externe Begutachtung

Stand 15.11.2026 (Variante W: 16.11.2026). Bevor ein Parametersatz freigeschaltet oder eine Einmal-Unterschrift
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

Stand 16.11.2026 (W5). Variante W braucht keine Gruppenaktion und keinen großen
Quantenrechner: Eine Note ist eine Folge von BB84-Zuständen, und das Geheimnis jeder Stelle
liegt als Shamir-Anteile bei den Validatoren der Note. Sie ist **schlafend** eingebaut
(`quantenWAbUnix = MaxInt64`). Freischalten (W6) setzt ein Gutachten zu den folgenden
Punkten voraus, ebenso den Partnerversuch W4 mit einem echten Speicher.

| Teil | Ort | Was es tut |
|---|---|---|
| Regeln, ausführbar | `forschung/quantengeld/wiesner_kette.py` | Spezifikation mit Missbrauchsfällen; Prüfvektor für den Knoten |
| Regeln im Knoten | `quanten_w.go`, `quanten_w_register.go` | Prüfsummen und Merkle, Shamir über 2¹²⁷−1, Hash-Ketten-Zufall, Aufforderung, Urteil, Übergänge |
| Einbindung | `quanten_w_kette.go` | 6 Transaktionsarten, Unterschriften, Buchungen (W-Topf, Pfand-Topf, Grundeinkommen), Register, StateRoot, Snapshot |
| Validator-Dienst | `quanten_w_validator.go` | Tresor für Anteile (verschlüsselt), Bestätigung beim Prägen, Zufall und Offenlegung von selbst, beides erst nach der Finalität |
| Prägen ohne Vertrauen | `wiesner_praegen.py`, `wiesner_hardware.py` (`note_fern`) | Validatoren messen verschränkte Hälften; niemand kennt die ganze Note |
| Messungen | `HEUTE.md`, `hardware_ibm.py --wiesner` | IBM `ibm_marrakesh`: echt eingelöst ohne Wartezeit (0,998); mit 20 µs und Teleportation abgewiesen |

**Fragen an die Begutachtung (Variante W):**

1. **Fälschung mit einer Note.** Die Toleranz liegt bei 15 % Fehlern unter den Treffern,
   bei mindestens 4 Treffern; beim Einlösen werden k = min(64, frei) Stellen geprüft.
   - Wie wahrscheinlich besteht ein Fälscher, der eine echte Note hatte und optimal
     geklont hat (Wiesner-Schranke, Kloner mit Fidelität 3/4 je Stelle)?
   - Mehrere Prüfungen nacheinander: Wächst die Erfolgsaussicht über die
     Fehlversuche hinweg? Drei Fehlversuche sperren Prüfungen, aber nicht das Einlösen.
2. **Absprachen.** Weniger als t Validatoren erfahren nichts über eine Stelle (Shamir).
   - Wie viel hilft einer Absprache das Wissen über einen Teil der Stellen?
     Gerechnet: 1,5·10⁻³ bei k = 64 und einem Fünftel der Stellen.
   - Darf ein Validator zugleich Prüfer oder Präger sein?
3. **Zufall.** Die Aufforderung folgt aus den Hash-Ketten **aller** v Validatoren, deren
   Anker beim Prägen bestätigt wurden.
   - Ist die Saat für einen Erzeuger, Anfragenden oder weniger als v Validatoren
     unvorhersagbar und unsteuerbar?
   - Schadet es, dass ein Validator den Zufall zurückhalten kann? Er verzögert dann nur:
     Es gibt keine Frist, das Pfand bleibt gesperrt.
   - Lebendigkeit: Fällt ein Validator für immer aus, steht die Note. Ist eine
     Schwellen-Signatur (t von v, eindeutig, z. B. BLS) als Ersatz geboten?
4. **Offenlegen statt gemeinsam Rechnen.** Gibt die Offenlegung der geprüften Stellen
   (Pfad und t Anteile je Stelle) etwas über die ungeprüften preis?
   - Die gesalzene Prüfsumme hat ein Salz von 124 Bit, je Stelle ein eigenes Polynom.
   - Kann ein unehrlicher Präger inkonsistente Anteile verteilen, ohne dass es der eigenen
     Note schadet? Bringen verifizierbare Anteile (Feldman/Pedersen) etwas?
5. **Pfand und Störung.** Das Pfand ist max(1 AEQ, Wert · k / n). Ein falsches Einlösen
   tötet die Note nicht; erschöpft (weniger als 8 Stellen frei) geht ihr Wert ans
   Grundeinkommen.
   - Kostet jede Störung (Prüfungen oder Einlöseversuche ohne Note) mindestens so viel,
     wie sie dem Halter schadet?
   - Gibt es einen billigeren Weg, eine fremde Note zu entwerten?
6. **Kettenseite.**
   - Kann ein Erzeuger oder Peer durch gerechnete Felder Geld aus W-Topf oder Pfand-Topf
     ziehen?
   - Kann er eine Antwort oder Anfrage wiederholen, eine Note doppelt oder neu prägen,
     Validatoren desselben Menschen einsetzen oder die Umlaufabgabe umgehen?
   - Tests: `quanten_w_test.go`, `quanten_w_kette_test.go`; Mutationen zu jeder Regel.
7. **Validator-Dienst.**
   - Finalität: Der Dienst gibt Zufall und Anteile heraus, wenn der Stand bei DAG-Höhe h
     gesehen wurde und der finalisierte Checkpoint h erreicht hat. Reicht das, damit eine
     Anfrage oder Antwort nicht mehr aus der Kette fallen kann? Welche Folgen hat ein
     Snapshot-Resync des Validators dazwischen?
   - Hinterlegen: Der Präger schickt jedem Validator dessen Anteile im Klartext über TLS.
     Genügt das, oder braucht es Ende-zu-Ende-Verschlüsselung an einen Schlüssel des
     Validators? Bei der Fern-Präparation (W3) entstehen die Anteile beim Validator selbst.
   - Tresor: AES-256-GCM mit einem Schlüssel aus dem Signierschlüssel, an K gebunden. Ist
     die Ableitung angemessen? Wie soll ein Validator den Tresor sichern und wiederherstellen,
     ohne dass die Sicherung zum Angriffsziel wird?
   - Verfügbarkeit: Hinterlegen kostet nichts (4 ungeprägte je Präger, 128 gesamt, Verfall
     nach dem Prägefenster). Braucht es ein Pfand?
   - Sammeln: Anteile anderer Validatoren werden gegen die eigenen Blätter geprüft, bevor
     etwas eingereicht wird. Kann ein Validator die Offenlegung trotzdem verhindern oder
     verfälschen?
   - Tests: `quanten_w_validator_test.go` (Mutationen zu jeder Prüfung),
     `vier_knoten_quanten_w_test.go` (vier Knoten, echte Datenbanken, echtes HTTP).

## Was erst die Hardware entscheidet (keine Frage an die Kryptografie)

- **Lebensdauer:** eine Note muss länger halten, als eine Zahlung dauert (heute ~20 µs
  auf IBM-Chips; siehe `forschung/quantengeld/HARDWARE_WEG.md`).
- **Größe:** ein Satz mit N ≥ 2²⁵⁶ braucht einen Verifizierer mit sehr vielen logischen
  Qubits.
- **Weitergabe:** eine Note von Hand zu Hand braucht ein Quantennetz.

Die Prüfer bezeugen diese drei Punkte gesondert, bevor sie für einen Satz stimmen.
