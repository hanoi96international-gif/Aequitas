# Teil 2: Einzigartigkeitsmerkmale gegen die realen Fähigkeiten des Smartphones

Stand: 2026-07-29 · Fortsetzung von `BIOMETRIE_ANALYSE.md` · Methodische Tiefenprüfung

---

## 0. Methode: vier Tore

Teil 1 hat Merkmale katalogisiert und das Gerät inventarisiert. Das reicht nicht, weil „das Handy hat eine Kamera" und „das Handy kann Iriserkennung" zwei völlig verschiedene Aussagen sind. Jedes Merkmal muss **vier voneinander unabhängige Tore** passieren. Es genügt, an einem zu scheitern.

| Tor | Frage | Scheitert wenn |
|---|---|---|
| **G1 — Physik** | Kann der Sensor das Signal aufnehmen? | Falsche Wellenlänge, zu grobe Ortsauflösung, zu wenig Kontrast |
| **G2 — Zugriff** | Lässt das Betriebssystem die App an die Rohdaten? | Sensor OS-gekapselt (Fingerabdruck, Face ID) |
| **G3 — Entropie** | Bleibt nach Rauschen genug Unterscheidungskraft? | Intra-Klassen-Varianz ≈ Inter-Klassen-Varianz |
| **G4 — Determinismus** | Lässt sich ein reproduzierbarer, globaler Nullifier ableiten? | Kein deterministischer Schlüssel ohne Vergleichsinstanz |

Die meisten Diskussionen bleiben bei G1 stehen („die Kamera hat 50 Megapixel, das muss doch reichen"). Die Entscheidung fällt bei G4 — und dort scheitert **jedes** Merkmal, auch die, die G1 bis G3 glänzend bestehen. Das ist der wichtigste Befund dieses Dokuments und steht in §5.

---

## 1. Das Handy physikalisch vermessen

### 1.1 Spektral — was das Gerät sieht und was es aussendet

**Empfindlichkeit.** Der Silizium-Bildsensor ist bis etwa 1100 nm empfindlich, deckt also das nahe Infrarot vollständig ab. Genau davor sitzt jedoch ein fest verbauter IR-Sperrfilter, weil ungefilterte NIR-Anteile die Farbwiedergabe zerstören. Dieser Filter *eliminiert* NIR nicht, er dämpft es typisch um den Faktor 10³–10⁴.

Das ist der entscheidende Punkt für alle NIR-Merkmale: Der Kanal ist **undicht, aber unbrauchbar**. Mit einer starken externen 850-nm-Quelle und langer Belichtung bekommt man ein Signal — aber das Gerät hat keine solche Quelle, die auf das Motiv gerichtet wäre. Die IR-LED des Näherungssensors ist schwach, ungerichtet und für Zentimeterabstand ausgelegt.

**Emitter — was das Gerät aktiv aussenden kann:**

| Emitter | Physik | Für Biometrie nutzbar |
|---|---|---|
| OLED-Display | R ≈ 620–630 nm, G ≈ 530 nm, B ≈ 460 nm, je ~20–40 nm Halbwertsbreite, pixelweise und zeitlich frei programmierbar | **Ja — schmalbandige, steuerbare Lichtquelle** |
| Blitz-LED | Weiß (blaue LED + Phosphor), hohe Leistung | Ja — Transillumination, PPG |
| Lautsprecher (2×) | bis ~22 kHz nutzbar | Ja — aktive Akustik |
| Vibrationsmotor | ~50–250 Hz | Ja — Körperschall-Anregung |
| IR-Näherungs-LED | 850–940 nm, schwach, ungerichtet | Praktisch nein |
| NIR-Flood/Dot-Projector (Face ID) | 940 nm, stark, gerichtet | **Vorhanden, aber G2-gesperrt** |
| NFC | 13,56 MHz | Ja — Chipauslesen |

Die dritte Zeile von unten ist die bitterste des ganzen Projekts: Auf jedem iPhone seit dem X sitzt exakt die NIR-Beleuchtung, die man für Iriserkennung bräuchte — und sie ist für Apps unerreichbar (in §2.3 verifiziert).

**Praktische Konsequenz:** Das Display ist der wertvollste, am meisten unterschätzte Sensor-Partner im Gerät. Drei unabhängig steuerbare Schmalbandquellen, kombiniert mit dem RGB-Bayer-Filter der Kamera, ergeben eine 3×3-Spektralmatrix. Menschliche Haut hat eine charakteristische Reflexionssignatur (Hämoglobin absorbiert stark bei 540/576 nm, Melanin fällt monoton zum Roten hin) — Papier, Displays und Silikonmasken haben sie nicht. Das ist eine echte, wenn auch grobe Multispektralanalyse ohne jedes Zubehör.

### 1.2 Räumlich — die Auflösungsrechnung

Hier wird meistens geraten. Rechnen wir.

Für eine Kamera mit horizontalem Bildwinkel 2α und *N* Pixeln über die Bildbreite gilt im Abstand *d*:

```
Bildbreite W = 2 · d · tan α          Ortsauflösung = W / N
```

**Hauptkamera** (≈ 26 mm KB-äquivalent → α ≈ 34,7°; 12 MP → 4000 px Breite):

| Abstand | Bildbreite | mm/Pixel | ppi |
|---|---|---|---|
| 10 cm | 139 mm | 0,035 | **732** |
| 20 cm | 277 mm | 0,069 | 366 |
| 30 cm | 416 mm | 0,104 | 244 |

**Frontkamera** (≈ 24 mm → α ≈ 37,5°; 12 MP → 4000 px):

| Abstand | Bildbreite | mm/Pixel | Iris (11,7 mm) |
|---|---|---|---|
| 10 cm | 153 mm | 0,038 | **305 px** |
| 30 cm (Selfie) | 460 mm | 0,115 | **102 px** |
| 50 cm | 767 mm | 0,192 | 61 px |

Diese Zahlen kippen zwei verbreitete Annahmen:

**Fingerabdruck: die Auflösung ist nicht das Problem.** Bei 10 cm liefert die Hauptkamera 732 ppi und übertrifft damit den forensischen 500-ppi-Standard. Papillarleisten mit ~0,45 mm Abstand werden mit ~13 Pixeln pro Periode abgetastet — mehr als genug für Minutien. Der Fingerabdruck scheitert also **nicht an G1-Auflösung**, sondern an vier anderen Dingen: der minimalen Fokusdistanz vieler Geräte, einer Schärfentiefe von wenigen Millimetern bei gewölbtem Finger, dem fehlenden Kontrast ohne Auflageglas (trockene Haut streut diffus) und vor allem der **fehlenden festen Geometrie** — ohne Platte variieren Maßstab, Neigung und Hautdehnung von Aufnahme zu Aufnahme, und genau das zerstört die Reproduzierbarkeit.

**Iris: bei Selfie-Abstand fehlt die Auflösung, bei Nahabstand der Fokus.** ISO/IEC 19794-6 verlangt für hohe Qualität ≥ 200 Pixel über den Irisdurchmesser. Bei 30 cm liefert die Frontkamera 102 px — Grenzbereich nach unten. Bei 10 cm wären es 305 px, also reichlich; nur sind Frontkameras meist auf 30–50 cm fixfokussiert und bei 10 cm unscharf. Und selbst wenn beides passte, bliebe die Wellenlängensperre aus §1.1 bestehen, die bei dunklen Iriden entscheidet.

**Touchscreen: eindeutig zu grob.** Die Sensorgitter der gegenseitigen Kapazität haben typisch 4–5 mm Rasterabstand. Die gemeldete Koordinatenpräzision von ~0,1 mm ist Interpolation eines Schwerpunkts, keine Ortsauflösung. Gegen 0,45 mm Leistenabstand fehlt **eine Größenordnung**. Ein kapazitiver Touchscreen kann Papillarleisten prinzipiell nicht abbilden — das ist keine Frage besserer Software.

### 1.3 Zeitlich — wo das Gerät stark ist

| Kanal | Abtastrate | Erfasst damit |
|---|---|---|
| Audio | 48 kHz (teils 96) | Stimme, Ultraschall-Echo bis 22 kHz |
| Kamera Zeitlupe | 240 fps, teils 960 | Pupillendynamik, Mikrobewegung |
| Rolling Shutter | zeilenweise, ~10–30 µs/Zeile | **effektiv kHz-Abtastung innerhalb eines Frames** |
| IMU | 200–800 Hz, teils 4 kHz | Physiologischer Tremor (8–12 Hz), Körperschall |
| Kamera regulär | 30–60 fps | rPPG, Blinzeln, Kopfbewegung |

Der Rolling Shutter verdient Beachtung: Der Sensor liest zeilenweise aus, ein einzelnes Bild enthält also zeitlich gestaffelte Proben. Moduliert man das Display im kHz-Bereich, tastet jede Bildzeile eine andere Phase ab — aus einer 30-fps-Kamera wird ein kHz-Instrument. Das ist der Mechanismus hinter der Erkennung von Display-Wiedergaben: Ein abgefilmter Bildschirm erzeugt charakteristische Schwebungsmuster gegen die eigene Bildwiederholrate, die ein echtes Gesicht nicht erzeugt.

### 1.4 Der Befund, der alles ordnet

> **Das Smartphone ist ein hervorragendes Zeitinstrument und ein mittelmäßiges Rauminstrument.**

Es misst *Prozesse* — Puls, Tremor, Pupillenreaktion, Stimme, Echo — mit beeindruckender Auflösung. Es misst *Strukturen* — Leisten, Iristextur, Venen — schlecht oder gar nicht.

Und die Einzigartigkeit des Menschen sitzt fast vollständig in den Strukturen. Prozesse sind zustandsabhängig: Puls, Stimme und Tremor ändern sich mit Aufregung, Koffein, Krankheit, Tageszeit. Das ist der physikalische Kern des ganzen Problems, in einem Satz:

> **Das Handy misst gut, was sich ändert, und schlecht, was einen Menschen unterscheidet.**

---

## 2. Merkmal für Merkmal durch die vier Tore

Legende: ✅ passiert · ⚠️ grenzwertig · ❌ scheitert · 🔒 OS-gesperrt

### 2.1 Papillarleisten

| Merkmal | G1 Physik | G2 Zugriff | G3 Entropie | G4 Determinismus |
|---|---|---|---|---|
| Fingerabdruck (Gerätesensor) | ✅ | 🔒 **nur Ja/Nein** | — | — |
| Fingerabdruck (Foto, 10 cm) | ⚠️ Auflösung ok, Fokus/Kontrast/Geometrie kritisch | ✅ | ⚠️ | ❌ |
| Handfläche (Foto) | ⚠️ | ✅ | ⚠️ | ❌ |

Der Gerätesensor scheitert an G2 und ist damit erledigt, unabhängig von seiner Qualität. Die Kamera-Variante überrascht bei G1 positiv (§1.2), bricht aber an der fehlenden Auflagegeometrie: Ohne feste Bezugsebene ist jede Aufnahme unterschiedlich skaliert und verzerrt, und die Hautdehnung beim Andrücken variiert. Erschwerend: Fingerabdrücke sind aus hochauflösenden Alltagsfotos in sozialen Netzwerken rekonstruierbar — ein aus Fotos ableitbares Merkmal ist als Geheimnis untauglich.

### 2.2 Okular

| Merkmal | G1 | G2 | G3 | G4 |
|---|---|---|---|---|
| Iris NIR | ❌ **Sperrfilter + keine Quelle** | ✅ | ✅ (bestes Merkmal) | ❌ |
| Iris sichtbares Licht | ❌ dunkle Iriden texturlos; 102 px bei Selfie-Abstand | ✅ | ⚠️ nur helle Iriden | ❌ |
| Retina | ❌ Optik unmöglich | ✅ | ✅ | ❌ |
| Sklera-Gefäße | ⚠️ ab ~15 cm auflösbar | ✅ | ⚠️ ~10⁻⁴ | ❌ |
| Pupillenlichtreflex | ✅ zeitlich gut messbar | ✅ | ❌ zustandsabhängig | ❌ |

Die Iris ist das informationsreichste Merkmal des Menschen und fällt an G1 — dem Tor, an dem am wenigsten zu machen ist. Die Physik ist eindeutig: Melanin absorbiert im Sichtbaren, die Textur tritt erst ab ~700 nm hervor, und dort steht der Sperrfilter. Bei einer Weltbevölkerung mit rund 79 % braunen Augen ist das kein Randfall.

Der Pupillenlichtreflex ist umgekehrt gelagert und interessant: als Identitätsmerkmal wertlos, als **Lebendnachweis** hervorragend, weil er ein physiologischer Regelkreis mit fester Latenz (~200–250 ms) ist, den ein Angreifer in Echtzeit korrekt simulieren müsste.

### 2.3 Vaskulär

| Merkmal | G1 | G2 | G3 | G4 |
|---|---|---|---|---|
| Handvenen | ❌ keine NIR-Quelle | ✅ | ✅ | ❌ |
| Fingervenen (Blitz-Transillumination) | ⚠️ Licht durchdringt, Streuung zerstört Struktur | ✅ | ❌ | ❌ |

Zur Transillumination, weil sie verlockend klingt: Weißes LED-Licht durchdringt eine Fingerkuppe tatsächlich — das ist das Prinzip der Kontakt-PPG. Was ankommt, ist aber durch vielfache Streuung im Gewebe räumlich vollständig verwischt. Man erhält ein **zeitlich** hochwertiges Signal (den Puls) und **räumlich** gar nichts. Genau die Zeit/Raum-Asymmetrie aus §1.4.

Hitachi hat 2016 Fingervenenerkennung mit Sichtlicht-Kameras angekündigt, LG hat 2019 im G8 ThinQ eine Handvenen-Erkennung ausgeliefert — mit dedizierter ToF/IR-Sensorik, nicht mit der normalen Kamera. Die Funktion wurde nicht fortgeführt. Das ist der übliche Verlauf: demonstrierbar unter Laborbedingungen, nicht feldtauglich.

### 2.4 Kraniofazial

| Merkmal | G1 | G2 | G3 | G4 |
|---|---|---|---|---|
| Gesicht 2D | ✅ mühelos | ✅ | ⚠️ **versagt bei eineiigen Zwillingen** | ❌ ~45 Bit |
| Gesicht 3D (ToF/LiDAR) | ⚠️ grobe Tiefe | ⚠️ teils | ⚠️ | ❌ |
| Gesicht 3D (Face ID) | ✅ exzellent | 🔒 **nur Ja/Nein** | — | — |
| Ohrmuschel | ✅ | ✅ | ❌ ~10⁻³ | ❌ |
| Hautporen (Level 3) | ⚠️ nur Makro | ✅ | ⚠️ | ❌ |

Zu G2 bei Face ID, weil das der häufigste Irrtum ist: **Rohe Infrarotbilder der TrueDepth-Kamera sind über AVFoundation nicht zugänglich.** Apps erhalten aufbereitete Tiefendaten und ARKit-Gesichtsgeometrie, nie den IR-Strom. Die Einschränkung besteht seit dem iPhone X unverändert und ist bewusst gesetzt. Dieselbe Kapselung gilt für Androids `BiometricPrompt`: Der Abgleich läuft in der TEE, die App bekommt ein Boolean.

Bei Gesicht 2D liegt die Hürde nicht bei G1 — die Kamera kann das mühelos —, sondern bei G3: Für eineiige Zwillinge wird eine True-Accept-Rate über null erst bei einer False-Accept-Rate über 10 % erreicht. Das Merkmal ist weitgehend genetisch determiniert; kein Modell kann trennen, was physisch gleich ist. Bei ~0,4 % eineiigen Zwillingen sind das weltweit etwa 30 Millionen Menschen.

### 2.5 Akustisch, dynamisch, verhaltensbasiert

| Merkmal | G1 | G2 | G3 | G4 |
|---|---|---|---|---|
| Stimme | ✅ | ✅ | ❌ generativ klonbar | ❌ |
| Ohrkanal-Echo | ✅ | ✅ | ⚠️ EER 7,6 % | ❌ |
| Gesichts-Echo (Ultraschall) | ✅ | ✅ | ⚠️ nur in Fusion | ❌ |
| Knochenleitung (Vibration + IMU) | ✅ | ✅ | ⚠️ | ❌ |
| PPG-Morphologie | ✅ | ✅ | ❌ zustandsabhängig | ❌ |
| Gang, Touch-Dynamik | ✅ | ✅ | ❌ EER 5–15 % | ❌ |

Diese ganze Klasse besteht G1 und G2 mühelos und scheitert geschlossen an G3. Eine EER von 7,6 % bedeutet bei einer einmaligen Registrierungsentscheidung: Von hundert Menschen werden knapp acht falsch behandelt. Für eine begleitende Risikobewertung brauchbar, für eine Identitätsfeststellung nicht.

### 2.6 Molekular

DNA, HLA, Mikrobiom, Immunrepertoire scheitern sämtlich an G1 — es gibt keinen Sensor und keinen plausiblen Pfad zu einem. Vollständigkeitshalber genannt, weil DNA in Diskussionen oft als Fluchtpunkt auftaucht; sie versagt zudem ausgerechnet bei eineiigen Zwillingen.

---

## 3. Zwischenstand nach drei Toren

Nach G1–G3 bleibt genau **ein** Merkmal übrig, das ein Smartphone ohne Zubehör mit brauchbarer Trennschärfe erfasst: **das Gesicht in 2D**, mit der bekannten Zwillingslücke.

Alles andere ist ausgeschieden:
- an der **Wellenlänge** (Iris, Venen),
- an der **OS-Kapselung** (Fingerabdrucksensor, Face-ID-Hardware),
- an der **Ortsauflösung** (Touchscreen-Leisten, Retina),
- an der **Geometrie** (Fingerfoto),
- an der **Zustandsabhängigkeit** (Puls, Stimme, Gang, Tremor).

Und dieses eine übrig gebliebene Merkmal scheitert an G4.

---

## 4. Wenn man das Gerät nicht als Kamera, sondern als Messplatz begreift

Bevor §5 die Bilanz zieht: Es gibt einen Perspektivwechsel, der real etwas bringt — nur nicht für die Einmaligkeit.

Passive Aufnahme ist gegen Injection-Angriffe strukturell verloren (Teil 1, §7): Wer den Videostrom unterhalb der Aufnahmeschicht einspeist, dem präsentiert man nichts, dem *ersetzt* man den Sensor. Keine Lebenderkennung, die nur das Bild betrachtet, kann das erkennen.

**Aktive Anregung ändert die Lage.** Das Gerät sendet ein unvorhersagbares Signal aus und misst die Antwort:

1. **Display als programmierbarer Illuminant.** Zufällige Farb- und Helligkeitssequenz, gemessen wird die Reflexion. Liefert photometrisches Stereo (grobe 3D-Rekonstruktion), die spektrale Hautsignatur aus §1.1 und — entscheidend — eine **Challenge-Response-Bindung**: Der Angreifer muss seinen synthetischen Strom in Echtzeit auf ein serverseitig gewähltes Lichtmuster antworten lassen. Bei Musterwechseln alle 30–50 ms und enger Phasenprüfung ist das die stärkste Anti-Injection-Primitive auf Standardhardware.

2. **Korneale Reflexion.** Das Displaymuster spiegelt sich in der Hornhaut, geometrisch verzerrt durch deren Krümmung (~7,8 mm Radius) und mit korrekter Disparität zwischen beiden Augen. Synthetische Bilder bekommen das notorisch falsch oder inkonsistent hin.

3. **Kanalübergreifende Kohärenz.** Der unterschätzteste Ansatz, weil er nichts kostet: Während einer Aufnahme hält eine Hand das Gerät. Der physiologische Tremor (8–12 Hz) steht im IMU **und** muss mit der Bildbewegung korrelieren. Ein eingespeistes Video korreliert mit nichts. Ebenso: Audio ↔ Lippenbewegung ↔ knochengeleitetes IMU-Signal ↔ Raumakustik ↔ Bildhintergrund. Der Angreifer muss *n* physikalisch konsistente Kanäle gleichzeitig synthetisieren.

4. **Umgebungszeugen gegen Gerätefarmen.** Magnetometer, Barometer, Umgebungslicht und Gerätetemperatur eines Telefons in einem Rack sehen völlig anders aus als bei einem Telefon in einer Hand. Billig und wirksam gegen Massenregistrierung.

**Der Wert dieser Klasse ist real — aber er liegt vollständig bei der Angriffserkennung, nicht bei der Einmaligkeit.** Sie beantworten „ist da gerade ein echter Mensch vor einem echten Sensor?", nicht „ist es *dieser* Mensch und war er schon einmal hier?".

---

## 5. Tor 4: warum auch perfekte Sensorik keinen Nullifier ergibt

Das ist der Befund, der die Architekturfrage entscheidet — und er gilt unabhängig von jeder Sensorqualität.

Ein Fuzzy Extractor besteht aus zwei Algorithmen:

```
Gen(w)      → (R, P)      bei der Registrierung
Rep(w', P)  → R           bei der Wiederholung, falls w' nahe an w
```

`R` ist der Schlüssel, `P` sind öffentliche Hilfsdaten. Reproduzierbar ist `R` **nur zusammen mit `P`**.

Daraus folgt unmittelbar: Ein Angreifer, der sich ein zweites Mal registriert, ruft schlicht erneut `Gen` auf, erhält frische Hilfsdaten `P'` und damit einen **anderen** Schlüssel `R'` — aus demselben Körper. Der Nullifier ist verschieden. Die Doppelregistrierung fällt nicht auf.

> **Ein Fuzzy Extractor leistet 1:1-Authentifizierung („ist das derselbe Körper wie der, der diese Hilfsdaten erzeugt hat?"), nicht globale Deduplikation („war dieser Körper schon irgendwo registriert?").**

Das entwertet die naheliegende Rettung. Auch mit einem Iris-Scanner und den 105 Bit aus dem besten publizierten Verfahren bekommt Aequitas keinen globalen Nullifier — nur einen sehr guten Wiedererkennungsmechanismus für ein bereits bekanntes Konto.

**Und die helferdatenfreie Variante?** Man könnte versuchen, den Schlüssel deterministisch allein aus dem Körper abzuleiten, ohne `P`. Dann muss der Merkmalsvektor global auf dasselbe Codewort quantisieren. Bei 10–20 % Bitfehlern zwischen zwei Aufnahmen derselben Iris braucht der Code einen entsprechend großen Korrekturradius — und je größer der Radius, desto weniger unterscheidbare Codewörter passen in den Raum. Der Kompromiss ist unausweichlich: **Determinismus wird direkt in Entropie bezahlt.** Bei praktikablen Fehlerraten bleiben Dutzende Bit statt Hunderte, und das ist für einen globalen Nullifier zu wenig.

**Die Falle, in die jeder Reparaturversuch läuft.** Die intuitive Rettung lautet: Entropie durch eine Nutzereingabe aufstocken — Passphrase, Geräteschlüssel, Salt. Das macht die Sache schlimmer. Jede Größe, die der Nutzer wählen kann, lässt sich mehrfach wählen:

> **Ein Nullifier, in den irgendeine nutzerkontrollierte Eingabe einfließt, ist kein Sybil-Schutz — er ist ein Identitätsautomat.**

Genau das ist der heutige Zustand von Aequitas in Reinform: Der „biometrische Hash" ist ein Zufallswert, den der Nutzer beliebig oft neu erzeugen kann. Es ist kein Implementierungsfehler, sondern derselbe Denkfehler, den auch „Biometrie plus Passphrase" begehen würde.

---

## 6. Bilanz

**Alle vier Tore, zusammengefasst:**

| Tor | Was hier ausscheidet |
|---|---|
| G1 Physik | Iris, Venen, Retina, Papillarleisten via Touchscreen, alles Molekulare |
| G2 Zugriff | Fingerabdrucksensor, Face-ID-Hardware — die zwei besten Sensoren im Gerät |
| G3 Entropie | Stimme, Gang, Puls, Ohr, Touch-Dynamik; Gesicht bei eineiigen Zwillingen |
| G4 Determinismus | **alles Übrige, einschließlich perfekter Iris-Hardware** |

Die Schlussfolgerung ist nicht, dass das Handy nichts kann. Sie ist präziser:

> **Ein Smartphone ohne Zubehör kann sehr gut feststellen, dass gerade ein echter, lebendiger Mensch vor einem echten Sensor sitzt. Es kann nicht feststellen, welcher — und schon gar nicht, ob dieser Mensch schon einmal da war.**

Die erste Fähigkeit ist wertvoll und wird von Aequitas derzeit überhaupt nicht genutzt. Die zweite ist die, um die es geht, und sie ist auf dieser Hardware nicht zu haben — nicht durch bessere Modelle, nicht durch mehr Sensoren, nicht durch bessere Lebenderkennung.

Einmaligkeit muss deshalb aus einer der beiden Quellen kommen, die Teil 1 §6 benennt: einer **Vergleichsinstanz**, die Merkmale aller Registrierten hält, oder einem **Anker außerhalb des Körpers**. Es gibt keine dritte.

---

## Quellen

- [Daugman, „How Iris Recognition Works" — 249 Freiheitsgrade](https://www.cl.cam.ac.uk/~jgd1000/irisrecog.pdf)
- [„Fuzzy Extractors are Practical: Cryptographic Strength Key Derivation from the Iris", ACM CCS 2025](https://dl.acm.org/doi/10.1145/3719027.3765098) · [ePrint](https://eprint.iacr.org/2024/100)
- [Dodis et al., „Fuzzy Extractors: How to Generate Strong Keys from Biometrics and Other Noisy Data" — Gen/Rep-Konstruktion](http://web.cs.ucla.edu/~rafail/PUBLIC/89.pdf)
- [Untargeted Near-collision Attacks on Biometrics: Real-world Bounds and Theoretical Limits](https://arxiv.org/pdf/2304.01580)
- [Apple Developer: Streaming depth data from the TrueDepth camera](https://developer.apple.com/documentation/avfoundation/streaming-depth-data-from-the-truedepth-camera) · [Kein Zugriff auf rohe IR-Daten](https://developer.apple.com/forums/thread/120712)
- [Android: TEE-gestützte Biometrie, Apps erhalten nur das Auth-Ergebnis](https://medium.com/softaai-blogs/how-tee-backed-fingerprint-authentication-works-in-android-for-enhanced-security-3e2ee13ba6e8)
- [Benchmarking Human Face Similarity Using Identical Twins](https://arxiv.org/pdf/2208.11822)
- [A Study of Multibiometric Traits of Identical Twins (MSU)](http://biometrics.cse.msu.edu/Publications/Multibiometrics/Sunetal_IdentTwins_SPIE10.pdf)
- [Face Flashing: a Secure Liveness Detection Protocol based on Light Reflections (NDSS 2018)](https://www.semanticscholar.org/paper/6c0fa024879d526a67bb9bfd8017a4ca288717da) · [Aurora Guard: Face Anti-Spoofing via Mobile Lighting System](https://arxiv.org/pdf/2102.00713) · [Anti-Spoofing unter Screen Flash](https://arxiv.org/pdf/2308.15346)
- [EarEcho: Using Ear Canal Echo for Wearable Authentication — EER 7,62 %](https://dl.acm.org/doi/10.1145/3351239)
- [HCR-Auth: Bone Conduction Head Contact Response](https://dl.acm.org/doi/10.1145/3699780) · [WristConduct](https://vali.de/wp-content/uploads/2022/07/Muc2022-WristConduct.pdf)
- [Hitachi: Finger Vein Authentication using Smartphone Camera (2016)](https://www.hitachi.com/en/press/articles/2016/10/1024/) · [Vein Biometric Recognition on an Ordinary Smartphone (MDPI)](https://www.mdpi.com/2076-3417/12/7/3522)
- [rPPG-Zuverlässigkeit bei schwachem Licht und erhöhtem Puls (npj Digital Medicine, 2025)](https://www.nature.com/articles/s41746-025-02192-y)
- [World: „Opening the Orb" — warum Iris dedizierte Optik braucht](https://world.org/blog/engineering/opening-orb-look-inside-worldcoin-biometric-imaging-device)
