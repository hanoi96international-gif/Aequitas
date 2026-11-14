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
   **2.048 bis 4.096 Bit** genannt. **Vorgabe nach eigener Messung (2a–2d):
   p ≥ 8.192 Bit für 2¹²⁸, p ≥ 17.000 Bit für das Ziel 2⁻¹⁹².**
3. **Mehrteilige Note** (`isogenie_mehrteilig.py`): k Teilnoten, gültig ab
   τ bestandenen, mit Abstand d = 2τ − k ≥ 1 über der Hälfte (gegen
   Aufteilen). Bei kryptografischer Größe genügt d = 1; die Fälschungs­
   schranke 1/h je Teilnote liegt dann weit unter 2⁻¹²⁸ – die Sicherheit
   bestimmt allein Punkt 2.
4. **Fehlerkorrektur:** Jede Teilnote ist ein Register aus m ≈ log₂ h Qubits;
   bei p ≈ 17.000 Bit ist h ≈ √p ≈ 2⁸⁵⁰⁰, also **m ≈ 8.500 logische Qubits je
   Teilnote** (bei 8.192 Bit: 4.096). Nach `rauschen_oeffentlich.py` muss jedes Qubit dann m-mal
   besser sein als bei Wiesner – das geht nur mit **fehlerkorrigierten
   logischen Qubits**.

### 2a. Selbst gemessen: Kuperbergs Sieb (`kuperberg.py`)

Die Zahl der Quantenabfragen hängt nur an der klassischen Buchführung der
Etiketten – sie lässt sich exakt simulieren. Gemessen für n = 12 … 36
(n = log₂ der Gruppengröße), angepasst an **log₂ Q ≈ 2,16·√n**:

| Primzahl p | n ≈ (log₂ p)/2 | Abfragen (erster Algorithmus) | + je Abfrage eine Isogenie im Quantenschaltkreis |
|---|---|---|---|
| 512 Bit | 256 | ≈ 2³⁴ | weit unter 2¹²⁸ |
| 1.024 Bit | 512 | ≈ 2⁴⁹ | unter 2¹²⁸ |
| 2.048 Bit | 1.024 | ≈ 2⁶⁹ | ≈ 2⁶⁹ × Isogeniekosten – knapp |
| **4.096 Bit** | 2.048 | **≈ 2⁹⁸** | **≈ 2⁹⁸ × Isogeniekosten – Ziel 2¹²⁸ erreichbar** |

Einordnung: das ist der *erste* (einfachste) Kuperberg-Algorithmus, also
eine **obere** Schranke für den Angreifer; bessere Siebe (Kuperberg 2011,
Regev, Peikert) brauchen weniger Abfragen, kosten aber mehr Speicher. Die
Hochrechnung von n ≤ 36 auf n = 2.048 ist weit – sie bestätigt die
Größenordnung, nicht die letzte Stelle.

### 2b. Korrektur durch den stärkeren Angreifer: Kollimationssieb

Der *zweite* Kuperberg-Algorithmus (Kollimation, Kuperberg 2011 / Peikert
2020) tauscht Abfragen gegen Speicher. Ebenfalls exakt über die Etiketten
gemessen (`kuperberg.py`, `kollimation`), n = 16 … 72:
**log₂ Q ≈ 1,06·√n + 0,7**.

| Primzahl p | erster Algorithmus | **Kollimation** |
|---|---|---|
| 512 Bit | 2³⁴ | **2¹⁸** (Literatur, Peikert 2020: 2¹⁶–2¹⁹ – unser Modell trifft das) |
| 2.048 Bit | 2⁶⁹ | 2³⁵ |
| 4.096 Bit | 2⁹⁸ | **2⁴⁹** |
| 8.192 Bit | – | ≈ 2⁶⁹ |

Damit trägt die Zahl der Abfragen allein bei 4.096 Bit **nicht** bis 2¹²⁸;
die Sicherheit liegt an Rechenzeit und Speicher des Siebs.

### 2c. Der volle Preis: klassische Arbeit und Speicher der Kollimation

Mitgezählt: jede Etiketten-Operation (Zeit) und der größte gleichzeitig
gehaltene Vektor (Speicher), jeweils beim günstigsten L, n = 16 … 72:
**log₂ Arbeit ≈ 2,05·√n + 3,0**, **log₂ Speicher ≈ 0,86·√n + 5,9**.

| Primzahl p | Quantenabfragen | **klassische Arbeit** | Speicher |
|---|---|---|---|
| 512 Bit | 2¹⁸ | 2³⁶ | 2²⁰ |
| 4.096 Bit | 2⁴⁹ | **2⁹⁶ – unter 2¹²⁸** | 2⁴⁵ |
| 8.192 Bit | 2⁶⁹ | **2¹³⁴ – über 2¹²⁸** | 2⁶¹ |
| 16.384 Bit | ≈ 2⁹⁸ | **2¹⁸⁹ – etwa 2¹⁹²** | 2⁸⁴ |

### 2d. Was kostet eine Abfrage? (`isogenie_kosten.py`)

Gezählt: Körpermultiplikationen einer Gruppenaktion mit Exponenten in
{−1, 0, 1} (zählende Arithmetik, gegen `isogenie_geld.py` geprüft), Modell
je kleiner Primzahl l: Kofaktor-Leiter (~ log₂ p) plus Kern/Isogenie (~ l);
das Modell liegt unter der Zählung (günstig für den Angreifer). Je
Multiplikation 4·B² Toffoli-Gatter.

| Primzahl | Toffoli je Abfrage | Quantenanteil (× Abfragen) | klassische Arbeit | Angriff kostet |
|---|---|---|---|---|
| 512 Bit | 2³⁸ | 2⁵⁶ | 2³⁶ | 2⁵⁶ |
| 4.096 Bit | 2⁵⁰ | 2⁹⁸ | 2⁹⁶ | 2⁹⁸ |
| 8.192 Bit | 2⁵⁴ | 2¹²² | **2¹³⁴** | **2¹³⁴** |
| 16.384 Bit | 2⁵⁷ | 2¹⁵⁴ | **2¹⁸⁹** | **2¹⁸⁹** |

Ab 8.192 Bit bestimmt die **klassische Arbeit** des Siebs den Preis.
Aufgelöst nach dem Ziel (log₂ Arbeit = 2,05·√n + 3,0):
**2¹²⁸ ab ≈ 7.450 Bit, 2¹⁹² ab ≈ 17.000 Bit.**

**Vorgabe (konservativ, „so scharf wie es geht“):**
- **2¹²⁸: p ≥ 8.192 Bit.**
- **2¹⁹² (unser Ziel, ML-DSA-65-Niveau): p ≥ 17.000 Bit** (16.384 Bit
  reichen mit 2¹⁸⁹ knapp nicht).

Die Literatur nennt für das unterste Niveau teils 4.096 Bit – unter anderen
Kostenannahmen (z. B. Speicher teurer als Rechenzeit). Unsere Messung ist
strenger; wir bauen nach der strengeren. Die Hochrechnung von n ≤ 72 auf
n ≈ 8.192 bleibt weit und bestätigt die Größenordnung, nicht die letzte
Stelle. 512 Bit sind in jedem Fall gebrochen.

## 3. Parameter (gerechnet)

Aus `isogenie_mehrteilig.py`, Ziel 2⁻¹²⁸ und 2⁻¹⁹²:

| Teilnote besteht (nach Lagerung) | Teile k | Schwelle τ | logische Qubits je Note (m ≈ 8.500, p ≈ 17.000 Bit) |
|---|---|---|---|
| 100 % | 1 | 1 | ~8.500 |
| 90 % | 5 | 3 | ~42.500 |
| 75 % | 19 | 10 | ~161.500 |

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
  bei p ≥ 17.000 Bit für tragfähig hält, und **(b)** fehlerkorrigierte
  Speicher die Lagerzeit aus Punkt 1 schaffen. Bis dahin bleibt der Code –
  wie alle Stichtage – schlafend.

## 5. Was wir als Forschungsgruppe als Nächstes prüfen

1. Die Literaturschätzungen aus Punkt 2 selbst nachrechnen – **weit
   gediehen** (2a erster Algorithmus, 2b Kollimation – trifft Peikerts Werte
   für 512 Bit, 2c klassische Arbeit und Speicher, 2d Kosten je Abfrage).
   Offen: bessere Zusammenführungen im Sieb, die die klassische Arbeit senken
   könnten – sie würden die Vorgabe weiter nach oben schieben.
2. Kopieren einer Teilnote formal auf Vektorisierung zurückführen (bisher
   als Annahme benutzt – genau das zeigt Doliskani im generischen Modell).
3. Rauschtolerante Prüfung *innerhalb* einer Teilnote statt nur über
   Teilnoten hinweg.
