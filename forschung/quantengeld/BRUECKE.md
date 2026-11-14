# Brücke AEQ ↔ Quanten-Note (Entwurf, Forschung)

Stand 14.11.2026. Gedankenmodell, keine Zusage und kein Produkt.

## Grundlage

Die Idee ist nicht neu, und das ist gut so. **Coladangelo & Sattath, „A Quantum
Money Solution to the Blockchain Scalability Problem“, Quantum 4, 297 (2020)**
(arXiv:2002.11998; ersetzt Coladangelo, arXiv:1902.05214) beschreiben genau
dieses Modell:
- Die Banknoten sind Quantenzustände.
- Eine klassische Kette führt die gültigen Seriennummern und schlichtet Streit.
- Eine Note wird über „bolt-to-signature“ wieder zu einem klassischen Anspruch.
- Wer eine Note verliert, kann den Wert unter Bedingungen zurückholen.
- Laut den Autoren löst das die Skalierung nur für Zahlungen, nicht für
  allgemeine Smart Contracts.

**Entscheidend:** Gewöhnliches Quantengeld reicht nicht. Es braucht
**Quantum Lightning**. Bei Quantum Lightning kann *niemand*, auch nicht die
ehrliche Herstellung, zwei gültige Noten mit derselben Seriennummer erzeugen.
Sonst könnte, wer AEQ sperrt, zwei Noten für denselben Betrag in Umlauf
bringen.

## Wie es bei Aequitas aussähe

1. **Sperren:** Ein Mensch sperrt x AEQ auf der Kette. Dabei entsteht eine Note
   mit zufälliger Seriennummer. Die Kette vermerkt: Seriennummer → x AEQ.
2. **Bezahlen:** Die Note geht offline von Gerät zu Gerät, als Quantenzustand.
   Der Empfänger prüft sie selbst, mit dem öffentlichen Verfahren und der
   Seriennummernliste der Kette (die darf er auch vorher geladen haben).
3. **Einlösen:** Wer die Note hat, verbraucht sie per bolt-to-signature und
   erhält einen Nachweis, der nur für **seine** Zieladresse gilt. Die Kette
   bucht x AEQ dorthin und markiert die Seriennummer als eingelöst.
4. **Verlust:** Der Sperrende entscheidet beim Sperren zwischen zwei Arten:
   - **Inhaberpapier:** Ist die Note weg, ist der Wert weg, wie Bargeld.
   - **Rückholbar:** Nach Ablauf einer Frist holt der Sperrende den Wert
     zurück. Taucht die Note danach auf, ist sie wertlos. Der Preis dafür: Ein
     Empfänger muss die Frist kennen und vor ihrem Ende einlösen.

**Was gleich bleibt:** „Ein Mensch, einmal 1.000 AEQ“ entsteht weiter nur auf
der Kette, durch die Registrierung. Die Noten tragen nur vorhandene AEQ weiter
und schaffen nie neue.

## Was das Modell `bruecke.py` zeigt

`bruecke.py` prüft die Logik auf der Kette mit einem **idealen** Quantum
Lightning, einem Platzhalter mit genau den Eigenschaften, die ein echtes
Verfahren bräuchte. Selbsttest mit Angriffen:

| Eigenschaft | Angriff | Ergebnis |
|---|---|---|
| Erhaltung der AEQ | – | Summe bleibt gleich |
| Keine Doppeleinlösung | Note zweimal verbrauchen, Nachweis wiederholen | abgewiesen |
| Nur bekannte Seriennummern | erfundene Seriennummer | abgewiesen |
| Nachweis an Ziel gebunden | abgefangenen Nachweis umlenken | abgewiesen |
| Deckung | Sperren ohne Guthaben, negativer Betrag | abgewiesen |
| Rückholung | vor Fristablauf, durch Fremde, bei Inhaberpapier | abgewiesen |
| Nach Rückholung | verlorene Note doch noch einlösen | abgewiesen |

Gegenprobe mit eingebauten Fehlern: Wird die Zielbindung oder die
Fristprüfung entfernt, schlägt der Test an. Zwei Prüfungen sind doppelt
abgesichert, nämlich die Doppeleinlösung und die Einlösung nach Rückholung:
Fehlt eine davon, fängt die zweite den Fall ab.

## Der Einlöse-Nachweis: Einmal-Signaturen (Stand 14.11.2026)

Das ideale Lightning in `bruecke.py` braucht genau eine Eigenschaft, die
gewöhnliches Quantengeld nicht hat: Wer die Note **verbraucht**, erhält einen
**klassischen** Nachweis, den die Kette prüfen kann – und mehr als einen
solchen Nachweis gibt es nie. Das leistet eine **Einmal-Signatur**
(one-shot signature, OSS):

- Die Note *ist* ein quantenmechanischer Signierzustand; ihre Seriennummer ist
  der öffentliche Schlüssel pk.
- **Einlösen** heißt: mit dem Zustand **eine** Nachricht unterschreiben, etwa
  „einlösen an Wallet W“. Dabei wird der Zustand verbraucht; eine zweite
  Nachricht unter demselben pk lässt sich nie unterschreiben.
- Die Kette prüft die Unterschrift **klassisch** – gleich wie heute jede
  ML-DSA-Unterschrift (Stufe 3).
- Daraus folgt auch Lightning: zwei gültige Noten mit demselben pk ergäben
  zwei Unterschriften.

**Stand der Forschung** (laut Zusammenfassungen, Volltexte vor Zitierung
lesen): Einmal-Signaturen wurden 2020 eingeführt (Amos, Georgiou, Kiayias,
Zhandry), zunächst nur im Orakelmodell. 2025 wurde eine Konstruktion im
Standardmodell vorgelegt (Shmueli & Zhandry, aus Ununterscheidbarkeits-
Verschleierung und LWE). Damit ist die **Einlöse-Frage theoretisch gelöst,
praktisch nicht**: Ununterscheidbarkeits-Verschleierung ist heute weit von
jeder Anwendung entfernt.

**Folgerung für die Bauvorgabe:** Die Kette braucht für die Brücke nur
klassische Bausteine, die es schon gibt – ein Register der Seriennummern
(öffentliche Schlüssel) und die Prüfung einer Unterschrift beim Einlösen.
Die Quantenseite (Note = OSS-Zustand oder Gruppenaktions-Note mit
Einlöse-Mechanismus) ist austauschbar. Deshalb lässt sich die Kettenseite
**schon jetzt spezifizieren und testen**, ohne sich auf ein Quantenverfahren
festzulegen – sie bleibt schlafend, bis eines trägt (BAUVORGABE.md §4).

## Kettenseite als ausführbare Spezifikation (`bruecke_kette.py`)

Was ein Knoten später Byte für Byte prüft – unabhängig vom Quantenverfahren:

- **Seriennummer** s = SHA-256(öffentlicher Schlüssel der Note).
- **Einlösen** nur mit einer Einmal-Signatur über
  `aequitas-quanten-einloesen-v1|chain:1926|<s>|<ziel>|<wei>` – Ziel und
  Betrag sind gebunden. Für die Spezifikation steht eine Lamport-Signatur
  (SHA-256) an Stelle der Quanten-Einmal-Signatur: dieselbe Prüfung auf der
  Kette, selbst quantensicher.
- **Höchstens eine Einlösung je Seriennummer – erzwingt die Kette selbst**,
  auch wenn jemand zwei gültige Unterschriften vorlegt.
- Sperren nur mit Deckung und neuer Seriennummer; Rückholen nur durch den
  Sperrenden, nur wenn erlaubt, nur nach der Frist; danach ist die Note
  wertlos. Erhaltung: es entsteht nie AEQ.

Selbsttest: **17 Angriffe abgewiesen**, Summe erhalten, und eine
**Gegenprobe** mit absichtlich ausgebauten Prüfungen (ohne Signatur, ohne
Einmal-Regel) zeigt, dass die Tests greifen.

**Im Knoten umgesetzt** (`x/humanity/keeper/quanten_note.go`, schlafend):
dieselben Regeln in Go; eine von `bruecke_kette.py` erzeugte Unterschrift
(`testdata/quanten_note_vektor.json`) besteht im Knoten, für jedes andere
Ziel nicht. Mutationsproben: ohne Signaturprüfung oder ohne Fristprüfung
schlägt der Test an; „höchstens einmal“ ist doppelt abgesichert (Register
und Verbraucht-Liste) – erst ohne beide geht der Angriff durch.

## Offene Fragen (Forschung)

1. **Ein Quantum-Lightning-Verfahren mit belastbarer Sicherheit.** Der
   Isogenie-Ansatz von Montgomery und Sharif wurde 2025/26 angegriffen. Der
   Angriff braucht exponentielle Zeit und ist damit unpraktisch, zeigt aber,
   wie dünn das Eis ist. Zhandrys Verfahren aus Gruppenaktionen ist
   öffentlich prüfbares Geld. Ob und wie es Lightning trägt, ist zu klären.
2. **Rauschen.** Echte Zustände verfallen. Es gibt Arbeiten zu
   rauschtolerantem Quantengeld (arXiv:2407.06463), aber nur im Orakelmodell.
3. **Gerät und Übertragung.** Quantenspeicher und Quanten-Übertragung für
   Endgeräte gibt es nicht.
4. **Klassische Seite quantensicher.** Einlöse-Nachweis, Kontounterschriften
   und Kette müssen selbst quantensicher sein, sonst ist die Brücke der
   schwache Teil. **Erledigt für Registrierung, Zusage und Überweisung**
   (Stufen 1–3, ML-DSA-65, live ab 14.11.2026 22:00 UTC); offen sind
   Validatoren und Netz (Stufe 5).
5. **Einlösen praktisch machen.** Einmal-Signaturen gibt es im
   Standardmodell nur über Ununterscheidbarkeits-Verschleierung. Gesucht: ein
   Einlöse-Mechanismus direkt für die Gruppenaktions-Note (isogenie_geld.py)
   oder eine praktischere OSS-Konstruktion.
