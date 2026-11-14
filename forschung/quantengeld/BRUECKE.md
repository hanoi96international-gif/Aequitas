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
   schwache Teil. Genau dafür sind die Stufen 1 bis 5 in
   `docs/QUANTENSICHERHEIT.md`.
