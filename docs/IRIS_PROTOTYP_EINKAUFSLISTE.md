# Einkaufsliste: Iris-Scanner-Prototyp

Stand: 01.10.2026. Die Preise sind Richtwerte (Euro, inkl. MwSt.) und **nicht
tagesaktuell geprüft**. Vor der Bestellung bitte bei Händlern wie Berrybase,
Reichelt, Mouser oder Pimoroni gegenprüfen.

Ziel des Prototyps: Wir wollen messen, ob wir mit bezahlbarer Hardware
Iris-Aufnahmen in einer Qualität bekommen, aus der ein stabiles Merkmal entsteht
(siehe [`WHITEPAPER.md` §3.1](../WHITEPAPER.md) und
[`BIOMETRIE_ANALYSE.md` §3.3 und §6](../BIOMETRIE_ANALYSE.md)). Das Gerät ist
noch kein Gerät für Endnutzer. Es dient dazu, Aufnahmequalität, Fehlerraten und
Lebenderkennung zu messen.

---

## Warum diese Bauteile

| Anforderung | Begründung | Folge für den Einkauf |
|---|---|---|
| **Nahes Infrarot (≈ 850 nm)** | Braune Iriden zeigen im sichtbaren Licht kaum Struktur. Die Textur sieht man erst im NIR (`BIOMETRIE_ANALYSE.md` §3.3). | Kamera **ohne** IR-Sperrfilter, 850-nm-LEDs, 850-nm-Bandpassfilter |
| **≥ 200 px Irisdurchmesser** | ISO/IEC 29794-6 nennt etwa 150 px als Untergrenze. Bei 200 px oder mehr sind die Aufnahmen gut. | 12-MP-Sensor + 16-mm-Objektiv (Rechnung unten) |
| **Feste Geometrie** | Bei 25 cm und f/4 beträgt die Schärfentiefe nur ≈ 1 cm. | Kinn- und Stirnstütze sind **Pflicht**, dazu ein Abstandssensor zum Auslösen |
| **Lebenderkennung** | Gedrucktes Foto, Display oder bedruckte Kontaktlinse müssen erkannt werden. | Zweite Wellenlänge (940 nm) und weiße LED für den Pupillenreflex |
| **Kein eingespeister Videostrom** | Bei Injection-Angriffen hilft Liveness nicht (`BIOMETRIE_ANALYSE.md` §7). | Secure Element signiert die Frame-Hashes |
| **Augensicherheit** | NIR ist unsichtbar. Das Auge schließt sich deshalb nicht reflexhaft. | LED-Treiber mit Konstantstrom, gepulster Betrieb, Messung vor dem ersten Einsatz |

**Optik-Rechnung (IMX477, 4056 × 3040 px, 1,55 µm Pixel, 16 mm Objektiv):**

- Sensorbreite 6,29 mm → Bildbreite bei 25 cm ≈ 98 mm → ≈ 41 px/mm
- Iris ≈ 11,7 mm → **≈ 480 px Durchmesser** bei 25 cm, ≈ 350 px bei 35 cm
- Bildhöhe bei 25 cm ≈ 73 mm. Beide Augen (Pupillenabstand ≈ 63 mm) passen gemeinsam ins Bild.

---

## A. Kern-Aufbau (Pflicht)

| # | Teil | Beispiel / Spezifikation | Menge | ca. € |
|---|---|---|---|---|
| 1 | Einplatinenrechner | Raspberry Pi 5, 4 GB | 1 | 65 |
| 2 | Netzteil | Offizielles Raspberry Pi 27 W USB-C | 1 | 13 |
| 3 | Kühlung | Raspberry Pi 5 Active Cooler | 1 | 6 |
| 4 | Speicher | microSD 64 GB, A2 (z. B. SanDisk Extreme) | 1 | 12 |
| 5 | Kamera | **Raspberry Pi HQ Camera M12-Mount** (Sony IMX477, 12 MP). Den IR-Sperrfilter entfernen oder direkt eine NoIR-Variante von Arducam nehmen. | 1 | 55–75 |
| 6 | Kamerakabel | Raspberry Pi 5 Kamerakabel 22→15 Pin, 200–300 mm | 1 | 3 |
| 7 | Objektiv | M12, **16 mm**, f/2.0–2.8, „IR-corrected“ bzw. für NIR optimiert, geringe Verzeichnung | 1 | 20–35 |
| 8 | Ersatzobjektiv | M12, 25 mm (für einen größeren Abstand von 35–45 cm) | 1 | 20–35 |
| 9 | NIR-Bandpassfilter | 850 nm, ±25–30 nm, passend für M12 oder als Ø 12–25 mm Scheibe vor dem Objektiv | 1 | 15–40 |
| 10 | NIR-LEDs 850 nm | z. B. OSRAM SFH 4715AS (High-Power, 850 nm) auf Sternplatine | 4 | 4 × 6 |
| 11 | NIR-LEDs 940 nm | z. B. OSRAM SFH 4725AS (zweite Wellenlänge für die Lebenderkennung) | 2 | 2 × 6 |
| 12 | Weiße LED | 5 mm oder SMD, warmweiß (Reiz für den Pupillenreflex) | 2 | 1 |
| 13 | LED-Treiber | Konstantstrom, dimmbar per PWM, z. B. Mean Well LDD-350H / LDD-700H | 2 | 2 × 8 |
| 14 | Schalt-MOSFETs | Logic-Level N-MOSFET (z. B. AO3400 oder IRLZ44N) für Blitzbetrieb synchron zur Belichtung | 5 | 3 |
| 15 | Diffusor | Opal-Diffusorfolie oder Acrylscheibe für die LEDs (gegen Hotspots und Spitzenbestrahlung) | 1 | 8 |
| 16 | Abstandssensor | VL53L1X ToF-Breakout (I²C) zum Auslösen erst im Schärfebereich | 1 | 15 |
| 17 | Secure Element | Microchip ATECC608B Breakout (Adafruit oder SparkFun, I²C) zum Signieren der Frame-Hashes | 1 | 12 |
| 18 | Kinn- und Stirnstütze | Optiker-Kinnstütze gebraucht oder selbst gedruckt (siehe C) | 1 | 0–60 |
| 19 | Kleinteile | Steckbrett, Jumperkabel, Widerstände, Kühlkörper für LED-Sterne, Wärmeleitkleber, M2.5-Abstandshalter | 1 Set | 25 |
| 20 | Netzteil für LEDs | 12 V / 2 A Steckernetzteil + Hohlbuchse | 1 | 12 |

**Summe A: ≈ 350–450 €**

## B. Referenz und Messung (stark empfohlen)

| # | Teil | Zweck | ca. € |
|---|---|---|---|
| 21 | **Zertifizierter USB-Iris-Scanner**, z. B. IriTech IriShield-USB MK 2120U | Liefert Vergleichsaufnahmen in Normqualität. Daran sehen wir, ob unser Eigenbau mithält, bevor wir an der Software zweifeln. | 300–450 |
| 22 | IR-Sichtkarte (Detektorkarte 800–1100 nm) | Zeigt, ob die NIR-LEDs leuchten und wohin sie strahlen | 10 |
| 23 | Auflösungstestbild USAF 1951 (gedruckt oder Glas) | Prüft Schärfe und px/mm im Arbeitsabstand | 10–40 |
| 24 | **Optisches Leistungsmessgerät** (z. B. Thorlabs PM100D + Fotodiodensensor S120C) | Misst die Bestrahlungsstärke am Auge (siehe Augensicherheit). **Lieber bei einer Hochschule oder einem Prüflabor ausleihen**, neu kostet es über 1.000 €. | leihen |

## C. Gehäuse und Mechanik

| # | Teil | Hinweis | ca. € |
|---|---|---|---|
| 25 | PETG-Filament, schwarz, matt | Gehäuse, Kamerahalter, Lichttubus. Matt und schwarz verringert NIR-Reflexe. | 25 |
| 26 | Lichttubus / Blende | Schirmt Umgebungslicht ab und hält den Abstand fest | gedruckt |
| 27 | Stativ oder Alu-Profil 20 × 20 | Feste Verbindung zwischen Kinnstütze und Kamera | 20 |

Wer keinen 3D-Drucker hat, kann die Teile bei einem Druckdienst bestellen (ca. 30–60 €).

## D. Testmaterial für Angriffe (Pflicht laut AGENTS.md, Punkt 5)

Zu jeder Lebenderkennung braucht es einen Test, der den Angriff zeigt:

| # | Teil | Angriff | ca. € |
|---|---|---|---|
| 28 | Laserdruck eines NIR-Irisbilds (eigene Aufnahme), matt und glänzend | Fotoangriff | 2 |
| 29 | Altes Smartphone oder Tablet als Anzeige | Display- und Videoangriff | vorhanden |
| 30 | Kosmetische Farbkontaktlinsen mit aufgedrucktem Muster (Nullstärke) | Linsenangriff | 15–25 |
| 31 | Puppen- oder Kunstauge | Attrappe | 10–20 |

---

## Gesamtkosten

| Paket | ca. € |
|---|---|
| Nur Kern-Aufbau (A + C) | 400–500 |
| **Empfohlen: A + B (ohne Messgerät) + C + D** | **750–1.050** |

---

## Vor dem ersten Einschalten

1. **Augensicherheit.** Die Bestrahlungsstärke am Auge muss weit unter dem
   Grenzwert der freien Gruppe nach IEC 62471 liegen. Für die Hornhaut im
   Infrarot sind das 100 W/m² (10 mW/cm²) bei Dauerbetrachtung. Unser Ziel ist
   höchstens 1–2 mW/cm². Die LEDs leuchten nur während der Belichtung (gepulst),
   der Strom wird hardwareseitig über den Konstantstromtreiber begrenzt. Bevor
   ein Mensch hineinschaut, wird die Bestrahlungsstärke im Arbeitsabstand
   **gemessen**. Eine Berechnung reicht nicht. Die Firmware begrenzt
   Pulsdauer und Tastgrad zusätzlich (fail-closed: Fehler → LEDs aus).
2. **Datenschutz.** Iris-Aufnahmen sind biometrische Daten nach Art. 9 DSGVO.
   Im Prototyp nehmen wir nur Teammitglieder auf, die schriftlich eingewilligt
   haben. Die Rohbilder bleiben lokal und verschlüsselt auf dem Gerät und werden
   nach der Messreihe gelöscht. Es gibt keinen Upload zur Beta-Infrastruktur.
   Die Grundlage ist `aequitas-biometric-beta/docs/dsgvo/02_DSFA.md`. Sie muss
   für die Iris ergänzt werden.
3. **Grenzen des Secure Elements.** Der ATECC608B signiert, was der Pi ihm
   gibt. Gegen einen eingespeisten Videostrom hilft das nur, wenn der Pi selbst
   abgesichert ist (Secure Boot, kein SSH im Messbetrieb, schreibgeschütztes
   System). Im Prototyp zeigen wir nur den Ablauf. Echte Fälschungssicherheit
   braucht eine Signatur nahe am Sensor. Das ist eine Frage für die
   Seriengeräte.

## Software (kostenlos, nicht Teil der Bestellung)

- `libcamera` / `picamera2` für die Aufnahme mit manueller Belichtung und Blitzsynchronisation
- [open-iris](https://github.com/worldcoin/open-iris) (Segmentierung, Normalisierung, Iris-Code) als Ausgangspunkt
- Fuzzy-Extractor-Konstruktion nach „Fuzzy Extractors are Practical“ (ACM CCS 2025), siehe `BIOMETRIE_ANALYSE.md` §6

## Was der Prototyp messen soll

- Irisdurchmesser in px und Schärfe (ISO/IEC 29794-6 Qualitätswerte) gegenüber dem Referenzscanner
- Hamming-Distanz derselben Iris über mehrere Sitzungen (Ziel: Ø unter 0,2) und verschiedener Iriden (≈ 0,5)
- Wie viele Bits ein Fuzzy Extractor bei welcher True-Accept-Rate liefert
- Erkennungsrate der Angriffe aus Abschnitt D
