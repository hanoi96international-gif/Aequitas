# Bauvorgabe: die Aequitas-Quanten-Note, „gleich richtig“

Stand 14.11.2026. **Forschungsentwurf, kein Produkt.** Festgelegt vom Betreiber:
*so scharf wie es geht – wenn wir es bauen, dann gleich richtig.* Diese Seite
hält fest, was das in Zahlen heißt, und woher jede Zahl kommt. Werte mit
„Literatur“ sind veröffentlichte Schätzungen und **vor einer Festlegung selbst
nachzulesen** (die Volltexte waren von hier aus nicht erreichbar).

## 1. Sicherheitsziel

| Größe | Vorgabe | Begründung |
|---|---|---|
| Fälschen einer Note | ≤ 2⁻¹²⁸, Ziel 2⁻¹⁹² | gleiches Niveau wie ML-DSA-65, mit dem die Kette (Stufe 1–3) schon unterschreibt – die Note darf nicht das schwächste Glied sein |
| Aufteilen einer echten Note in zwei | ≤ 2⁻¹²⁸ | eigener Angriff, gefunden in `isogenie_mehrteilig.py` |
| Echte Note besteht | ≥ 99 % nach der vorgesehenen Lagerzeit | Nutzbarkeit |

## 2. Bausteine und warum

1. **Gruppenaktion:** Klassengruppe auf supersingulären Kurven (CSIDH-Art),
   wie in `isogenie_geld.py` – heute der ernsthafteste Kandidat für öffentlich
   prüfbares Quantengeld (Zhandry 2024; Sicherheitsbeweis im generischen
   Modell: Doliskani 2025).
2. **Größe der Primzahl:** Die Note ist nur so sicher wie die
   *Vektorisierung* (der „diskrete Logarithmus“ der Gruppenaktion) schwer ist.
   Ein Quantenrechner löst sie in subexponentieller Zeit (Kuperberg). Die
   veröffentlichten Schätzungen (Literatur: Peikert 2020, Bonnetain &
   Schrottenloher 2020) sehen **CSIDH-512 deutlich unter 128 Bit
   Quantensicherheit**; für ~128 Bit werden Primzahlen in der Größenordnung
   **2.048 bis 4.096 Bit** genannt. **Vorgabe: p ≥ 4.096 Bit**, bis eigene
   Nachrechnung eine kleinere Zahl rechtfertigt.
3. **Mehrteilige Note** (`isogenie_mehrteilig.py`): k Teilnoten, gültig ab
   τ bestandenen, mit Abstand d = 2τ − k ≥ 1 über der Hälfte (gegen
   Aufteilen). Bei kryptografischer Größe genügt d = 1; die Fälschungs­
   schranke 1/h je Teilnote liegt dann weit unter 2⁻¹²⁸ – die Sicherheit
   bestimmt allein Punkt 2.
4. **Fehlerkorrektur:** Jede Teilnote ist ein Register aus m ≈ log₂ h Qubits;
   bei p ≈ 4.096 Bit ist h ≈ √p ≈ 2²⁰⁴⁸, also **m ≈ 2.048 logische Qubits je
   Teilnote**. Nach `rauschen_oeffentlich.py` muss jedes Qubit dann m-mal
   besser sein als bei Wiesner – das geht nur mit **fehlerkorrigierten
   logischen Qubits**.

## 3. Parameter (gerechnet)

Aus `isogenie_mehrteilig.py`, Ziel 2⁻¹²⁸ und 2⁻¹⁹²:

| Teilnote besteht (nach Lagerung) | Teile k | Schwelle τ | logische Qubits je Note (m ≈ 2.048) |
|---|---|---|---|
| 100 % | 1 | 1 | ~2.048 |
| 90 % | 5 | 3 | ~10.240 |
| 75 % | 19 | 10 | ~38.912 |

Je schlechter der Speicher, desto mehr Teile – aber die Sicherheit bleibt bei
2⁻¹²⁸. Der Speicher entscheidet, ob eine Note *handlich* ist, nicht ob sie
*sicher* ist.

## 4. Kette (Brücke, Gedankenmodell – `BRUECKE.md`)

- AEQ für eine Note werden auf der Kette **gesperrt**; die Seriennummer
  (s₁ … s_k) steht dort, unterschrieben mit ML-DSA-65 (Stufe 3).
- Zurücktauschen nur gegen einen klassischen Nachweis, der die Note beim
  Messen zerstört (Coladangelo & Sattath); die Seriennummer wird dabei
  verbraucht – keine Note zweimal.
- Einschalten erst, wenn **(a)** ein Verfahren mit Sicherheitsbeweis aus
  anerkannten Annahmen vorliegt oder die Fachwelt das Gruppenaktions-Verfahren
  bei p ≥ 4.096 Bit für tragfähig hält, und **(b)** fehlerkorrigierte
  Speicher die Lagerzeit aus Punkt 1 schaffen. Bis dahin bleibt der Code –
  wie alle Stichtage – schlafend.

## 5. Was wir als Forschungsgruppe als Nächstes prüfen

1. Die Literaturschätzungen aus Punkt 2 selbst nachrechnen (Kuperberg-
   Varianten, Kosten der Isogenie-Auswertung im Quantenschaltkreis).
2. Kopieren einer Teilnote formal auf Vektorisierung zurückführen (bisher
   als Annahme benutzt – genau das zeigt Doliskani im generischen Modell).
3. Rauschtolerante Prüfung *innerhalb* einer Teilnote statt nur über
   Teilnoten hinweg.
