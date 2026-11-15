# Quantengeld – Prüfauftrag für eine externe Begutachtung

Stand 15.11.2026. Bevor ein Parametersatz freigeschaltet oder eine Einmal-Unterschrift
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
| Freischaltung | `quanten_geld.go`, `pruefer_pq.go` | ⅔ der Prüfer, hybrid unterschrieben, N ≥ 2²⁵⁶, 30 Tage Vorlauf |

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
6. **Freischaltung.** Genügen ⅔ der Prüfer (hybrid: EIP-191 + ML-DSA-65), Vorlauf und
   N ≥ 2²⁵⁶ als Schutz gegen eine voreilige oder böswillige Freischaltung?

## Was erst die Hardware entscheidet (keine Frage an die Kryptografie)

- **Lebensdauer:** eine Note muss länger halten, als eine Zahlung dauert (heute ~20 µs
  auf IBM-Chips; siehe `forschung/quantengeld/HARDWARE_WEG.md`).
- **Größe:** ein Satz mit N ≥ 2²⁵⁶ braucht einen Verifizierer mit sehr vielen logischen
  Qubits.
- **Weitergabe:** eine Note von Hand zu Hand braucht ein Quantennetz.

Die Prüfer bezeugen diese drei Punkte gesondert, bevor sie für einen Satz stimmen.
