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
4. **Fehlerkorrektur:** Jede Teilnote ist ein Register, das eine Kurve
   speichert – den Koeffizienten A ∈ F_p, also **m ≈ log₂ p Qubits**
   (Korrektur 14.11.2026: vorher stand hier log₂ h ≈ (log₂ p)/2 – eine
   kürzere Darstellung der Kurven wäre gerade der diskrete Logarithmus).
   Bei p ≈ 17.000 Bit also **m ≈ 17.000 logische Qubits je Teilnote** (bei
   8.192 Bit: 8.192). Nach `rauschen_oeffentlich.py` muss jedes Qubit dann m-mal
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

**Gegenprobe, stärkerer Angreifer (`sieb_dreifach.py`):** Kollimation mit
drei statt zwei Vektoren (mehr Bits je Stufe, weniger Abfragen) gemessen,
n = 16 … 32: die klassische Arbeit ist **nie kleiner** (1,0× bis 4,0×);
gerechnet wächst sie mit 2^(2,5·√n) statt 2^(2·√n). Die Vorgabe bleibt.

Die Literatur nennt für das unterste Niveau teils 4.096 Bit – unter anderen
Kostenannahmen (z. B. Speicher teurer als Rechenzeit). Unsere Messung ist
strenger; wir bauen nach der strengeren. Die Hochrechnung von n ≤ 72 auf
n ≈ 8.192 bleibt weit und bestätigt die Größenordnung, nicht die letzte
Stelle. 512 Bit sind in jedem Fall gebrochen.

### 2e. Was kostet das PRÜFEN einer Note? (`isogenie_kosten.py`)

Der ehrliche Prüfer projiziert auf den Eigenraum zur Seriennummer: eine
Gruppenaktion in Überlagerung hin, eine zurück, also ≈ 2 Abfragen. Gerechnet
mit 1 µs je logischer Schicht (optimistisch):

| Primzahl | Toffoli | × RSA-2048 brechen | Dauer, wenig Qubits | Dauer, parallel (~B² Qubits) |
|---|---|---|---|---|
| 4.096 Bit | 2⁵¹ | 2¹⁹ | 1,3 Tage | 1,1 h |
| **8.192 Bit** | **2⁵⁴˙⁵** | 2²³ | **9 Tage** | **4,5 h** |
| 17.000 Bit | 2⁵⁹ | 2²⁷ | 74 Tage | 21 h |

(RSA-2048 ≈ 2³¹˙⁵ Toffoli: Literatur, Gidney & Ekerå 2019, nachzulesen.)

**Das ist der eigentliche Engpass, nicht nur der Speicher:** Eine Zahlung
darf nicht Tage dauern. **Billigeres Prüfen schwächt die Note nicht** – ab
8.192 Bit bestimmt die klassische Arbeit des Siebs den Angriffspreis (2c),
nicht die Kosten je Abfrage.

**Umkehrbarkeit, gegen uns (`umkehrbar.py`):** Der Schaltkreis muss jeden
Zwischenstand zurückrechnen (Bennetts Kieselspiel, Zug für Zug gespielt und
gegen die Formel geprüft). Bei 8.192 Bit: Zeitfaktor **≈ 8 bei 7·10⁸** bis
**≈ 100 bei 7·10⁶ logischen Qubits Arbeitsspeicher** – Prüfen also **2⁵⁸–2⁶²
Toffoli**, und ein **Prüfgerät** braucht 10⁷–10⁹ logische Qubits (die Note
selbst 8.192). Prüfen wird damit Sache großer Prüfstellen, nicht des
Telefons. Bennett ist allgemein, also eine obere Schranke.

**Erster Hebel gemessen (`isogenie_hebel.py`, projektiv gezählt, gegen das
Original geprüft – gleiche Kurven):**

| Primzahl | bisher | + Überdeckung (gegen uns) | + Strategie, Schranken | + √Vélu (Literatur) |
|---|---|---|---|---|
| 4.096 Bit | 2⁴⁹ | 2⁵³ | 2⁵² | **2⁵¹** |
| 8.192 Bit | 2⁵³ | 2⁵⁷ | 2⁵⁶ | **2⁵⁵** |
| 17.000 Bit | 2⁵⁷ | 2⁶² | 2⁶¹ | **2⁵⁹** |

- **Korrektur gegen uns:** Der Prüfer braucht die *ganze* Gruppe, also
  Exponenten bis m = 21 (8.192 Bit), nicht nur −1…1. Das kostet 2⁴.
- **Die einfachen Hebel sind ausgeschöpft:** Teile-und-herrsche, optimale
  Strategie, Schranken je Primzahl bringen zusammen nur 2⁰˙⁷, √Vélu
  (Literaturmodell) 2¹˙³. Bei dieser Größe kostet nicht die Leiter, sondern
  das Schieben von Punkten und die Isogenien selbst (~ Σ |eᵢ|·lᵢ).
- **Stand: Prüfen ≈ 2⁵⁵ Toffoli bei 8.192 Bit.** Der Faktor 2⁵–2¹⁰ muss
  **strukturell** kommen (§5.4): Gruppenaktion über höherdimensionale
  Isogenien (Aufwand wächst mit log p statt mit Σ lᵢ; offen: die
  Normgleichungen in Überlagerung lösen) und schnellere Multiplizierer im
  Schaltkreis.
- **Multiplizierer gebaut und gemessen (`schaltkreis_mult.py`):** echter
  umkehrbarer Schaltkreis, Gatter für Gatter simuliert, ohne Müll.
  Schulbuch kostet **4,0·B²** Toffoli – die Annahme oben stimmt. Karatsuba
  in einfacher Bauart gewinnt **nicht** (eine Stufe 1,38× teurer): die
  Überträge laufen durch das ganze Ergebnisregister. Gewinn braucht eine
  Übertragsbehandlung mit Fenstern (Literatur: Gidney 2019, nachzulesen).

## 3. Parameter (gerechnet)

Aus `isogenie_mehrteilig.py`, Ziel 2⁻¹²⁸ und 2⁻¹⁹²:

| Teilnote besteht (nach Lagerung) | Teile k | Schwelle τ | logische Qubits je Note (m ≈ 17.000, p ≈ 17.000 Bit) |
|---|---|---|---|
| 100 % | 1 | 1 | ~17.000 |
| 90 % | 5 | 3 | ~85.000 |
| 75 % | 19 | 10 | ~323.000 |

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
4. **Prüfkosten senken (höchste Priorität – 2e):** Die klassischen
   CSIDH-Hebel sind gemessen und ausgeschöpft (zusammen ≈ 2²). Nächstes:
   Gruppenaktion mit höherdimensionalen Isogenien auswerten (Literatur:
   Ansatz „PEGASIS“ 2025, nachzulesen) statt über ~760 kleine Primzahlen,
   und schnellere Multiplizierer (einfaches Karatsuba gemessen: kein
   Gewinn) – Kandidaten für den Faktor 2⁵–2¹⁰. Erst zählen, dann behaupten.
   **Literaturstand (Suche 14.11.2026, Volltexte von hier nicht erreichbar):**
   PEGASIS (CRYPTO 2025) wertet die Aktion *beliebiger* Elemente in
   Polynomzeit aus, ohne Klassengruppenstruktur und ohne Gitterreduktion –
   damit entfiele auch unsere Überdeckungs-Korrektur (2⁴). Gemessen (Sage):
   1,5 s bei 512 Bit, 21 s bei 2.048, ~2 min bei 4.096 Bit. Der Nachfolger
   qt-Pegasis (ePrint 2025/1859) ist 2,6× schneller bei 4.000 Bit und
   **scheitert nie** – für eine Quantenschaltung entscheidend, denn jede
   Wiederholung „bis es klappt“ müsste sonst in Überlagerung mitgerechnet
   werden. Grobe Einordnung aus den Zeiten: bei 4.096 Bit etwa 2²²–2²⁵
   Multiplikationen je Aktion, unsere CSIDH-Auswertung mit Überdeckung
   ~2²⁵ – also **Gewinn zwischen 1× und 2³, nicht sicher 2⁵**. Die echten
   Operationszahlen (Länge der 4-dimensionalen Kette, Kosten je Schritt)
   stehen im Volltext; nächster Schritt: nachlesen und nachzählen.
5. **Reicht eine schwache Prüfung? – Nein, gemessen (`schwache_pruefung.py`).**
   Idee: statt der vollen Projektion (Exponenten bis m = 21) je Erzeuger
   ein paar Hadamard-Tests mit einer einzigen Isogenie. Der beste Angriff
   darauf ist ein **glatter Zustand**: eine Gaußglocke über
   Exponentenvektoren mit den Phasen der Note. Er besteht jeden Einzeltest
   fast, braucht aber nur einen Träger kleiner als die Gruppe. An der
   echten kleinen Gruppe (h = 1.491) exakt simuliert: Glocke σ = 0,4
   besteht schwach mit 0,16, voll nur mit 0,015 – und die Formel trifft die
   Simulation. Hochgerechnet, mit derselben Annahme für beide Prüfungen
   (Träger ≤ 2⁻¹²⁸·N): die schwache Prüfung braucht bei 8.192 Bit **156
   Runden, die volle 42** – sie ist 3,7× **teurer**. Die volle Prüfung
   bleibt. (Nebenbefund: auch die volle Prüfung ruht darauf, dass
   Vektorisierung *in einem Ausschnitt* der Größe 2⁻¹²⁸·N schwer bleibt –
   als Annahme festgehalten.)

## 6. Das Gesamtpaket – was wir bauen, was andere bauen

Der Betreiber: *„Wir können das bauen – aus vielen einzelnen Forschungen ein
Gesamtpaket.“* Das ist der Weg. Die drei Sperren, ehrlich nach Zuständigkeit:

| Sperre | Wer löst sie | Unser Beitrag |
|---|---|---|
| **Prüfkosten** (2e: Tage je Prüfung) | **wir** – Algorithmen | gemessen: Strategien 2², Karatsuba einfach 0, schwache Prüfung 0 – Faktor 2⁵–2¹⁰ nur strukturell (höherdimensionale Isogenien) |
| **Einlösen** (Einmal-Signatur, heute nur aus iO) | **wir** – Protokoll | Einweg-Brücke (unten) braucht sie gar nicht |
| **Speicher** (≈ 8.200–17.000 fehlerkorrigierte logische Qubits je Teilnote, über Tage; dazu der Arbeitsspeicher des Prüfers, noch nicht gezählt) | Hardware-Hersteller | Vorgabe festhalten; Stand der Fahrpläne verfolgen |

**Einweg-Brücke (Version 1 ohne Einmal-Signatur):** AEQ → Note geht (Sperren
als Inhaberpapier, die Kette vermerkt nur die Seriennummer). Zurück zur Kette
geht es nicht; wer AEQ will, tauscht die Note gegen AEQ bei jemandem, der
Noten nimmt – wie Bargeld gegen Kontoguthaben. Die Erhaltung bleibt
(Konten + ausgegebene Noten = konstant), eine verlorene Note ist verloren wie
ein Geldschein. Der Rückweg (Einlösen, schon im Knoten, schlafend) kommt dazu,
sobald es eine Einmal-Signatur ohne iO gibt.

Damit hängt Version 1 nur noch an **zwei** Dingen: an unserem Prüfkosten-
Hebel und am Speicher. Die Kettenseite und die Spezifikation stehen.

## 7. Andere Wege – Literaturstand (Suche 14.11.2026, Volltexte von hier nicht erreichbar)

| Weg | Was er verspricht | Haken | Für uns |
|---|---|---|---|
| **Gruppenaktion (Zhandry, veröffentlicht 2025)** – unser Weg | öffentlich prüfbar, offline | braucht laut Autor **eine zusätzliche Annahme, deren Tragfähigkeit offen ist**; Kosten siehe §2e | bleibt Hauptweg, Annahme in §1 als offen führen |
| **Einmal-Speicher (Genovese & Stambler, arXiv 2512.21304, 2025/26)** | öffentlich prüfbar mit **QKD-artiger Technik** statt Universal-Quantenrechner | Hardware-Annahme (Einmal-Speicher sind ohne sie beweisbar unmöglich), nur **N Prüfungen je Note**, Quantenspeicher mit langer Kohärenz nötig | **ernsthafter Kandidat für einen früheren Versuch** – Vertrauen in Hardware statt in eine Stelle; Volltext lesen |
| **Quanten-Schecks / Löschnachweis aus LWE (Barhoush & Salvail, arXiv 2401.16116)** | klassisch einlösbar, Löschnachweis öffentlich prüfbar | **vom Autor zurückgezogen** (v4) | nicht darauf bauen |
| Gitter (Khesin–Lu–Shor u. a.) | – | **gebrochen** (Liu–Montgomery–Zhandry 2023) | ausgeschlossen |

Nächster Schritt: die Volltexte von Zhandry 2025 (welche Zusatzannahme?) und
Genovese & Stambler (welche Hardware, wie viele Qubits, wie lange Speicher?)
lesen – beide sind über arxiv.org zugänglich, von dieser Sitzung aus derzeit
nicht.

## 8. Der Weg (14.11.2026, nach Lektüre von Zhandry 2025 im Volltext)

**Neuer Befund aus dem Volltext (TheoretiCS 2025, Abschnitt 6):** Für
CSIDH-artige Gruppen, in denen nur *kurze* Elemente berechenbar sind
(„REGA“), prüft Zhandrys Verfahren nicht mit 2 Gruppenaktionen, sondern mit
λ Hadamard-Tests – jeder eine volle Gruppenaktion mit Exponenten aus einer
gruppenüberdeckenden Verteilung. Je Test: echte Note besteht mit ≥ 0,961,
jede andere mit ≤ 0,848 (Beweisschranken des Papiers). Für 2⁻¹²⁸ (ehrliche
Note fällt mit ≤ 2⁻⁴⁰ durch) gerechnet: **29.250 Tests** mit der Schwelle
7/8 des Papiers, **2.750 Tests** mit optimierter Schwelle 0,93. Das ist
**2¹¹ mehr**, als §2e angenommen hat. Die saubere Fassung mit 2 Aktionen
(Abschnitt 1.1/3) braucht eine Gruppenaktion für **beliebige** Elemente
(„EGA“) – für CSIDH heute nur über Gitterreduktion in Dimension ~760
(Kosten laut Zhandry ≈ 2^(n^(1/3))).

**Daraus der Weg – vier Bausteine, jeder aus vorhandener Forschung:**

1. **Rechenwerk: qt-Pegasis** (Dartois, Komada Eriksen, Invernizzi,
   Vercauteren; ePrint 2025/1859). Wertet die Klassengruppenaktion für
   *beliebige* Elemente in Polynomzeit aus, **scheitert nie** (keine
   Wiederholungen – Pflicht für eine Quantenschaltung), fast nur
   4-dimensionale 2-Isogenien. Es gibt eine **zeitkonstante C-Umsetzung**
   der 4D-Kette (ePrint 2026/114, < 25 ms bei 500 Bit) – zeitkonstant heißt
   datenunabhängiger Ablauf, genau was ein Schaltkreis braucht. Damit wird
   aus der REGA- die EGA-Fassung: **Prüfen = 2 Aktionen statt 2.750 Tests.**
   Grobe Hochrechnung (aus 25 ms ≈ 2²⁰ Multiplikationen bei 500 Bit,
   linear in der Bitlänge): ~2²⁴ Multiplikationen je Aktion bei 8.192 Bit –
   **nachzuzählen**, sobald der Volltext vorliegt.
2. **Verfahren: Zhandry 2025, EGA-Fassung**, Sicherheit im generischen
   Modell plus eine Verstärkung der DLog-Annahme (§7).
3. **Brücke: Einweg** (§6) – braucht keine Einmal-Signatur; die
   Kettenseite liegt schlafend im Knoten.
4. **Hardware:** Note 8.192 logische Qubits (2¹²⁸), Prüfgerät 10⁷–10⁹
   (§2e, `umkehrbar.py`). Prüfen findet an **Prüfstellen** statt – jeder,
   der ein Prüfgerät betreibt, kann prüfen (öffentlich, ohne Geheimnis).

**Zwischenschritt mit heutiger Technik (nur Forschung, kein Echtgeld):**
ein Spielgeld-Demonstrator nach Genovese & Stambler (arXiv 2512.21304,
Volltext gelesen): QKD-artige Einzelqubits, Prüfung per Stichprobe
(„cut-and-choose“), Fälschungsschranke 2^(−2ξ). **Für Echtgeld
ungeeignet:** die Einmal-Speicher hängen an vertrauenswürdiger Hardware
(TEE wie Intel SGX); wer eine TEE knackt, liest alle Urbilder der Note aus
und kann sie **beliebig oft** nachmachen. Das verletzt den Launch-Maßstab.

**Gerechnet nach den Volltexten (`pegasis_kosten.py`):** Die zeitkonstante
C-Umsetzung der 4D-Kette ([DD] Tabelle 1, Apple M1) umgerechnet in
Multiplikationen: **~2²⁰ bei 505 Bit bis ~2²³ bei 4.030 Bit, fast linear
(Steigung 0,88)** – bei 8.192 Bit **2²³˙⁵ je Aktion**. Neuer Engpass ist
**Schritt 1** (Normgleichung): seine Suche nach einer Primzahl muss im
Schaltkreis mit fester Obergrenze laufen – 2²⁶˙⁸ Multiplikationen, 10× die
Kette. Daher zwei Varianten:

| Primzahl | **A: kohärent, 2 Aktionen** | **B: u klassisch, 128 Tests** | CSIDH-REGA, 2.750 Tests |
|---|---|---|---|
| 4.096 Bit | 2⁵² | 2⁵⁷ | 2⁶⁴ |
| **8.192 Bit** | **2⁵⁶** | **2⁵⁹˙⁵** | 2⁶⁸ |
| 17.000 Bit | 2⁶⁰ | 2⁶³ | 2⁷²˙⁵ |

(Toffoli je Prüfung, vor Umkehrbarkeit: ×8 bis ×100, `umkehrbar.py`.)

**Variante B ist eigener Vorschlag:** Der Prüfer zieht u *klassisch*
gleichverteilt und löst Schritt 1 auf einem gewöhnlichen Rechner; der
Quantenrechner führt je Test nur die von *einem* Qubit gesteuerte Kette aus.
Weil die Note ein exakter Eigenzustand ist, gilt Mittel_u χ(u,h)*·U_u = P_h:
jeder Test wirkt wie (I + P_h)/2 – eine fremde Note besteht je Test mit
**genau ½**, die echte mit 1; 128 Tests für 2⁻¹²⁸. Etwas teurer als A, aber
**keine Primzahlsuche und keine Überlagerung über Ideale im Schaltkreis** –
der Schaltkreis ist eine feste, klassisch parametrierte 4D-Kette. Das ist
der Kandidat, mit dem wir weiterbauen. Offen bleibt in beiden Varianten die
Trägerprüfung (liegt der Zustand auf Kurven der Bahn?) und Schritt 2
(Torsionsbasis der Kurve) in Überlagerung – beide laut [Q] billig gegen die
Kette, zu zählen.

**Variante B komplett** (`pegasis_kosten.py`, je Test Trägerprüfung +
Schritt 2 + 2 Ketten, 128 Tests; Träger und Schritt 2 sind ~11 % – klein
gegen die Kette), mit Umkehrbarkeit (Bennett, Zustand der 4D-Kette ~16
Theta-Koordinaten × B Bit × log₂ e gehaltene Punkte je Kiesel):

| Primzahl | Arbeitsspeicher Prüfgerät | Toffoli je Prüfung |
|---|---|---|
| 4.096 Bit | 6·10⁷ … 6·10⁹ logische Qubits | 2⁶³ … 2⁶⁰ |
| **8.192 Bit** | **1·10⁸ … 1·10¹⁰** | **2⁶⁵˙⁵ … 2⁶³** |
| 17.000 Bit | 3·10⁸ … 3·10¹⁰ | 2⁶⁹˙⁵ … 2⁶⁶ |

Damit ist der Weg **vollständig beziffert**: jedes Teil stammt aus
veröffentlichter Forschung, jede Zahl aus einem Skript mit Selbsttest. Die
Lücke ist die Hardware – ein Prüfgerät mit ~10⁸–10¹⁰ logischen Qubits und
~2⁶³ Toffoli je Prüfung (zum Vergleich: RSA-2048 brechen ≈ 2³¹˙⁵).

**Nächste Schritte:** (a) ~~Volltexte~~ gelesen (qt-Pegasis, PEGASIS,
Dartois–Duparc; vom Betreiber hochgeladen). (b) Die 4D-Kette als
umkehrbaren Schaltkreis zählen (Theta-Koordinaten, Kosten je Schritt aus
[DD]: Bildkurve 3·(2⁴−2) M, Auswertung 171 M + 52 S je Klebe-Schritt),
statt aus Takten umzurechnen. (c) Trägerprüfung und Schritt 2 in
Überlagerung zählen.
