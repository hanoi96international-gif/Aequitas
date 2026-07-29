# Biometrie-Analyse: Einzigartigkeitsmerkmale des Menschen und was ein Smartphone ohne Zubehör davon zuverlässig leisten kann

Stand: 2026-07-29 · Gegenstand: Aequitas Proof-of-Humanity · Autor: technische Analyse, keine Marketing-Aussage

---

## 0. Kurzfassung

Drei Ergebnisse, die den Rest des Dokuments tragen:

1. **Der aktuell deployte Aequitas-Nullifier enthält kein einziges gemessenes Körpermerkmal.** Er ist an einen gerätegebundenen Schlüssel im Android-Keystore gebunden, der hinter der Bildschirmsperre liegt. Das ist Geräte-Bindung, nicht Körper-Bindung. Eine Person mit *n* Telefonen ist *n* Menschen. Die Einschätzung „Spielerei" ist zutreffend und lässt sich im Code belegen (§1).

2. **Ein Smartphone ohne Zubehör kann Menschen *wiedererkennen*, aber es kann keinen *körpergebundenen kryptographischen Nullifier* erzeugen.** Das sind zwei völlig verschiedene Aufgaben, und die zweite scheitert an einer harten physikalischen und informationstheoretischen Grenze, nicht an fehlendem Engineering (§6). Der beste publizierte Stand der Technik zieht **105 Bit** aus einer Iris — und braucht dafür NIR-Beleuchtung, die kein Serien-Smartphone hat. Aus einem Gesicht sind es **~45 Bit**, was für einen globalen Nullifier unbrauchbar ist.

3. **Auf einem nackten Smartphone gibt es genau einen Weg zu echter Sybil-Resistenz, und er ist nicht biometrisch im engeren Sinne: NFC-Auslesen des ePass-/eID-Chips (ICAO 9303) mit Signaturkette und Chip-Authentication, plus Gesichtsabgleich gegen das im Chip signierte Lichtbild (DG2) und Liveness** (§8). Alles andere — Selfie-Biometrie, Fingerabdruck über die Kamera, Stimme, Verhaltensbiometrie — ist gegen einen motivierten Angreifer im unbeaufsichtigten Fernverfahren nicht haltbar (§7).

Das Whitepaper (§3.1) beschreibt einen Zustand, der weder deployt ist noch mit den angegebenen Zahlen erreichbar wäre. Konkrete Korrekturliste in §9.

---

## 1. Ist-Zustand: was in diesem Repo tatsächlich passiert

### 1.1 Der Registrierungspfad, wie er wirklich läuft

`x/humanity/keeper/register.go:377-394` erzwingt Circuit v3 und nimmt `pubSignals[1]` als Nullifier:

```go
if req.CircuitVersion != 3 || req.ZKNullifier == "" {
    return "", fmt.Errorf("only circuit version 3 is accepted: ... v3 Poseidon nullifier is required")
}
```

Kryptographisch ist das sauber: der Nullifier ist per Groth16-Proof an die Circuit-Eingaben gebunden, `IsNullifierUsed` (`register.go:399`) verhindert Doppelnutzung, die TOCTOU-Lücke ist geschlossen (`register.go:525-539`). **Die Sybil-Mechanik funktioniert einwandfrei — sie schützt nur nichts, was mit einem Körper zu tun hat.**

Denn was geht in den Circuit hinein? Nachgeprüft **im Quelltext der App selbst** (`hanoi96international-gif/aequitas-app`, HEAD `db5e359`), nicht nur im Audit-Kommentar des Chain-Repos — `lib/identity.ts:39-41`:

```ts
const bytes = await Crypto.getRandomBytesAsync(32);
```

**Der „biometrische Hash" ist eine Zufallszahl, erzeugt beim ersten App-Start.**

**Präzisierung zur Kameraerfassung.** Das App-Repo hat zwei divergente Branches. Auf `aequitas-app` (Default, `db5e359`, 13.07.) existiert keinerlei Kamera-Code. Auf `main` (`14e7ccf`, 28.06.) liegt ein natives `android/app/src/main/java/com/aequitasbio/FaceModule.kt` mit einer echten camera2-Erfassung — **es wird aus der JS-Schicht jedoch nirgends aufgerufen** (`captureFace` hat keinen einzigen Aufrufer). Die pauschale Aussage „kein Client hat je ein Gesicht gescannt" aus `aequitas-dapp.html:393-403` ist damit als Aussage über den *Code* ungenau, als Aussage über den *Registrierungspfad* aber zutreffend: In den Nullifier fließt auf beiden Branches ausschließlich das Zufallsgeheimnis.

Bewertung des `FaceModule` selbst, da es den beabsichtigten Entwurf zeigt: Es mittelt die Luminanz über 8×8 Blöcke eines fest zugeschnittenen Bildmittendrittels (keine Gesichtserkennung), quantisiert auf 4 Bit je Block und bildet **SHA-256 über das Ergebnis**. Das ist ein Average-Hash — ein Verfahren zur Duplikatsuche in Bildarchiven, kein Erkennungsmerkmal. Entscheidend ist die Hashbildung: Sie zerstört die Metrik, auf die jede Biometrie angewiesen ist. Ein einziger Block, der um eine Quantisierungsstufe springt (Licht, Abstand, Kopfhaltung), ergibt einen völlig anderen Hash.

Die Folge ist nicht nur eine hohe Falschrückweisungsrate, sondern eine Umkehrung des Schutzziels: **Aus einem Gesicht lassen sich durch minimale Beleuchtungsänderung beliebig viele verschiedene Hashes erzeugen.** Als Nullifier-Eingabe eingesetzt wäre die Konstruktion ein Sybil-Verstärker, kein Sybil-Schutz. Keine Verbesserung der Lebenderkennung — auch kein Blinzeltest — behebt das, weil der Fehler in der Merkmalsableitung liegt, nicht in der Angriffserkennung.

Ansonsten hat das Projekt keine `expo-camera`-Abhängigkeit; nur `expo-local-authentication` (die Ja/Nein-Schranke, §4.2) und `expo-secure-store`.

Drei zusätzliche Befunde aus demselben Quelltext, die über die reine Gerätebindung hinausgehen:

**(a) `deriveBioHash` ist keine Hashfunktion** (`lib/identity.ts:23-29`):
```ts
h = (h * 256n + BigInt(input.charCodeAt(i))) % FIELD_SIZE;
```
Eine Basis-256-Stellenwertinterpretation modulo der BN254-Skalarordnung. Keine Einwegfunktion, keine Preimage-Resistenz, keine Kollisionsresistenz. Der Name behauptet eine Eigenschaft, die der Code nicht besitzt.

**(b) Der „Blinding Factor" blendet nichts** (`lib/identity.ts:61`):
```ts
const salt = (bio * 7n + 12345n) % FIELD_SIZE;
```
`salt` ist eine affine Funktion von `bio` — null zusätzliche Entropie, keine Verschleierung. Der Circuit erhält zwei Eingaben, die dieselbe eine Geheimzahl sind. Jede Sicherheitsaussage, die auf zwei unabhängigen Faktoren beruht (die Kommentare sprechen von einem „2-factor prototype"), ist damit gegenstandslos.

**(c) Der Zeuge wird im Klartext an die Verifizierer übertragen.** `lib/api.ts:176` sendet `{bio, salt, wallet}` an den Chain-Node, der an den Proof-Server weiterreicht; `postRegister` überträgt zusätzlich `bioHash: identity.bio`. Beide Instanzen kennen das vollständige Geheimnis. Der Groth16-Beweis beweist Wissen gegenüber Parteien, die dieses Wissen bereits haben — operativ ist die Zero-Knowledge-Eigenschaft gegenüber Betreiber und Proof-Server nicht vorhanden.

Ergänzend benennt `x/humanity/keeper/api_html.go:77-79` die Gerätebindung für den parallelen WebAuthn-Pfad bereits selbst:

> „…the WebAuthn ‚register via browser' flow (**device-bound, so a person with two devices could register twice**…)"

### 1.2 Was das bedeutet

| Behauptung | Realität |
|---|---|
| „Zero biometric data on-chain" (dapp.html:406) | Korrekt — weil nie welche erhoben wurde |
| „Hardware Secure Element" | Korrekt, aber es schützt einen *Geräteschlüssel*, kein Körpermerkmal |
| „Real Groth16 ZKP" | Korrekt — der Proof ist echt, die Aussage die er beweist ist nur inhaltsleer |
| „biometrischer Hash" (`register.go:186`) | Ein Schlüssel aus dem Keystore, freigeschaltet durch *irgendeine* Bildschirmsperre |
| Nullifier = 1 Mensch | Nullifier = **1 Geräte-Keystore-Eintrag** |

Die Bildschirmsperre ist dabei nicht einmal notwendig biometrisch: `requireAuthentication` wird auf `SecureStore.canUseBiometricAuthentication()` gesetzt — auf einem Gerät ohne eingelernte Biometrie ist der Wert `false` und das Geheimnis wird **ganz ohne Authentifizierungsschranke** abgelegt.

**Kostenkorrektur (wichtig).** Eine frühere Fassung dieses Abschnitts bezifferte die Kosten einer Zusatzidentität mit dem Preis eines Gebrauchtgeräts. Das ist zu hoch gegriffen. Das Geheimnis liegt mit `WHEN_UNLOCKED_THIS_DEVICE_ONLY` in App-Daten, die Android bei der Deinstallation löscht; beim nächsten Start erzeugt `ensureDeviceSecret()` ein neues Zufallsgeheimnis. Der serverseitige `biometric_in_use`-Check kann nicht greifen, weil der neue Wert nie zuvor gesehen wurde.

> **Kosten pro zusätzlicher Identität: eine Neuinstallation der App plus eine neue Wallet. Beides gratis und automatisierbar.**

Empirisch in wenigen Minuten prüfbar (deinstallieren → neu installieren → neue Wallet → registrieren) und vor allen weiteren Maßnahmen zu verifizieren, weil davon die Dringlichkeit der Prämien-Streckung abhängt.

Der zweite Verteidigungsring (`bio_hashes`-Tabelle, `register.go:434-446`) würde ohnehin nur denselben geräteabgeleiteten Hash abgleichen — und ist laut `AUDIT_2026-07-12.md` in Produktion mit `chain_bio_hashes: 0` bei 6 registrierten Humans **für niemanden aktiv**.

### 1.3 Whitepaper vs. Deployment

`WHITEPAPER.md:129-138` deklariert „Phase 1 — Alle 10 Fingerabdrücke + Lebenderkennung *(aktiv)*" mit R503-Scanner und MAX30102-PPG-Sensor. In diesem Repo existiert kein Code, der ein UART-Gerät anspricht, keine PPG-Auswertung, kein Zehn-Finger-Enrollment. Das Hardware-Kit ist als *aktiv* beschrieben und ist es nicht. Das ist die gravierendste Einzelaussage im Dokument, weil ein Leser daraus schließt, dass die Sybil-Resistenz körpergebunden sei.

Die Zahlen sind zusätzlich nicht haltbar — Details in §2 und §9.

---

## 2. Die drei Größen, die systematisch verwechselt werden

Fast jede falsche Zahl in Biometrie-Whitepapers entsteht daraus, dass drei verschiedene Dinge in dieselbe Tabellenspalte geschrieben werden.

**(A) Merkmalsentropie** — wie viel Information steckt theoretisch im Merkmal.
Daugmans Iris-Analyse ergibt ~249 statistische Freiheitsgrade, ~3.2 Bit/mm² Diskriminationsentropie. `2^249 ≈ 10^75` — daher stammt vermutlich die „1 von 10⁷⁸" im Whitepaper. **Diese Zahl ist keine Fehlerrate.** Sie beschreibt die Vielfalt der Muster, nicht die Fähigkeit eines Systems, sie auseinanderzuhalten.

**(B) Matching-Genauigkeit** — wie gut trennt ein reales System unter realen Aufnahmebedingungen.
Hier liegen die Zahlen um viele Größenordnungen niedriger, weil jede Aufnahme verrauscht ist. Iris-Codes weisen zwischen zwei Aufnahmen derselben Iris typisch **10–20 % abweichende Bits** auf. Praktisch erreichbare Falschakzeptanzraten liegen bei 10⁻⁶ bis 10⁻¹¹ pro Vergleich — nicht bei 10⁻⁷⁸.

**(C) Kryptographisch extrahierbare Schlüsselentropie** — wie viele Bits eines *reproduzierbaren, geheimen Schlüssels* lassen sich aus dem Merkmal gewinnen, ohne eine Datenbank.
Das ist die Größe, die Aequitas' Architektur tatsächlich braucht, und sie ist die kleinste von allen. Stand der Technik (ACM CCS 2025, „Fuzzy Extractors are Practical"): **105 Bit aus der Iris bei 92 % True-Accept-Rate**; vorherige Arbeiten kamen auf 32 Bit (Iris) bzw. **45 Bit (Gesicht)**.

Von 249 theoretischen Bits über eine Fehlerrate von 10⁻⁹ bis zu 105 tatsächlich extrahierbaren Schlüsselbits — das ist der Abstand zwischen Whitepaper und Physik. Jede Tabelle unten trennt diese drei Spalten deshalb sauber.

---

## 3. Vollständiger Katalog der Einzigartigkeitsmerkmale des Menschen

Bewertungskriterien:
- **Entropie/Trennschärfe** — realistisch erreichbar (Größenordnung B, nicht A)
- **Stabil** — über Jahrzehnte unverändert?
- **MZ-Zwillinge** — trennt es eineiige Zwillinge? (~0,4 % der Bevölkerung, weltweit ~30 Mio. Menschen — für ein globales UBI-System keine Randnotiz)
- **Phone** — mit einem Serien-Smartphone **ohne Zubehör** erfassbar?

### 3.1 Genetisch / molekular

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| DNA (STR-Profil, 20+ Loci) | ~10⁻¹⁸ Zufallstreffer | lebenslang | ❌ identisch | ❌ |
| DNA (SNP-Array / WGS) | praktisch eindeutig | lebenslang | ❌ (nur somat. Mosaik) | ❌ |
| HLA-Typisierung | ~10⁻⁵ | lebenslang | ❌ identisch | ❌ |
| Immunrepertoire (TCR/BCR) | extrem hoch | ⚠️ driftet | ✅ verschieden | ❌ |
| Darm-/Hautmikrobiom | hoch | ❌ Monate | ✅ verschieden | ❌ |
| Körpergeruch (VOC-Profil) | mittel | ⚠️ ernährungsabhängig | teilweise | ❌ |

Der Goldstandard der Forensik ist für Fernverifikation unbrauchbar: kein Sensor, invasiv, und DNA versagt ausgerechnet bei eineiigen Zwillingen.

### 3.2 Papillarleisten (friction ridge)

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| Fingerabdruck (1 Finger) | FMR 10⁻⁴…10⁻⁶ real | lebenslang* | ✅ (EER +1–2 %) | ⚠️ nur Foto |
| Fingerabdrücke (10 Finger) | FMR 10⁻⁸…10⁻¹² real | lebenslang* | ✅ | ❌ praktisch |
| Handflächenabdruck | vergleichbar 2–3 Fingern | lebenslang* | ✅ | ⚠️ nur Foto |
| Fußsohlenabdruck | hoch | lebenslang | ✅ | ❌ |

\* bei Handarbeit, Alter, Hauterkrankungen degradiert; 2–5 % der Bevölkerung liefern dauerhaft keine verwertbaren Abdrücke.

**Zur Whitepaper-Zahl „10 Finger = 1 von 10⁹⁰":** Diese Zahl entsteht durch Potenzieren einer Einzelfinger-Rate (10⁻⁹)¹⁰. Das setzt voraus, dass die zehn Finger statistisch unabhängig *und* jeder einzeln mit 10⁻⁹ trennbar ist. Beides trifft nicht zu — Fingerabdrücke einer Hand korrelieren im Mustertyp (Bogen/Schleife/Wirbel ist teilweise erblich und handseitig korreliert), und 10⁻⁹ ist bereits für einen Einzelfinger im Feldbetrieb optimistisch. Realistisch für Zehn-Finger-Systeme: 10⁻⁸ bis 10⁻¹². Aadhaar, das größte je gebaute System (1,3 Mrd. Menschen, 10 Finger + 2 Iriden + Gesicht, dedizierte zertifizierte Hardware), braucht trotzdem manuelle Nachprüfung für Grenzfälle. 10⁻⁹⁰ ist um 78 Größenordnungen zu optimistisch.

### 3.3 Okular

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| **Iris (NIR)** | FMR 10⁻⁶…10⁻¹¹ | ab ~1 J. lebenslang | ✅ **vollständig** | ❌ (IR-Sperrfilter) |
| Iris (sichtbares Licht) | FMR ~10⁻³…10⁻⁴, nur helle Iriden | s.o. | ✅ | ⚠️ eingeschränkt |
| Retina-Gefäßmuster | ~10⁻⁷ | ⚠️ diabetes-/gefäßabhängig | ✅ | ❌ |
| Sklera-Gefäßmuster | ~10⁻⁴ | mittel | ✅ | ⚠️ |
| Augenbewegungsdynamik | EER 5–15 % | ❌ zustandsabhängig | ✅ | ⚠️ |

Die Iris ist der informationsreichste nicht-invasive Marker des Menschen — und der Grund, warum sie auf einem Smartphone ohne Zubehör nicht funktioniert, ist rein physikalisch: **Iris-Textur ist bei braunen und dunklen Augen im sichtbaren Licht nahezu unsichtbar.** Das Melanin absorbiert; erst im nahen Infrarot (700–900 nm) tritt die Struktur hervor. Jede Smartphone-Frontkamera hat einen IR-Sperrfilter fest verbaut, um die Farbwiedergabe zu sichern. Samsung hatte in Galaxy S8/Note-Generationen einen *dedizierten* IR-Emitter plus separate IR-Kamera — genau dieses Zusatzsilizium wurde ab der S10-Generation (2019) wieder gestrichen. Das World/Worldcoin-Orb existiert aus demselben Grund: multispektrale Sensorik und eine Gimbal-geführte Schmalfeld-Kamera für hochaufgelöste Iris-Aufnahmen — Dinge, die man nicht in ein Telefon bekommt.

Für ein globales System ist der Effekt zudem systematisch diskriminierend: sichtbares-Licht-Iriserkennung funktioniert bei blauen/grünen Augen deutlich besser als bei braunen. Bei einer Weltbevölkerung, die zu ~79 % braune Augen hat, ist das kein Randfall, sondern der Normalfall.

### 3.4 Vaskulär

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| Fingervenen | FMR ~10⁻⁵…10⁻⁶ | hoch | ✅ | ❌ |
| Handflächenvenen | FMR ~10⁻⁵…10⁻⁷ | hoch | ✅ | ❌ |
| Handrückenvenen | ~10⁻⁴ | hoch | ✅ | ❌ |

Sehr attraktiv (liegt *innerhalb* des Körpers, dadurch kaum nachbildbar, kein latenter Abdruck bleibt zurück) — und ebenfalls strikt NIR-gebunden. Hämoglobin absorbiert bei ~850 nm; im sichtbaren Licht sieht die Kamera Haut, keine Venen. **Ohne IR-Emitter und IR-durchlässige Kamera prinzipiell nicht erfassbar.** Die Whitepaper-Angabe „1 von 10⁷" für Handvenen ist übrigens die einzige Zahl in §3.1, die halbwegs im richtigen Bereich liegt.

### 3.5 Kraniofazial / morphologisch

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| **Gesicht 2D (DNN-Embedding)** | FNIR ~0,07 % @ 12 Mio. Galerie (Top-Algorithmus, gute Aufnahmen) | ⚠️ Alterung, 5–10 J. | ❌ **versagt** | ✅ |
| Gesicht 3D (strukturiertes Licht/ToF) | besser als 2D bei PAD | ⚠️ | ❌ versagt | ⚠️ nur High-End, OS-gesperrt |
| Ohrmuschel-Geometrie | ~10⁻³…10⁻⁴ | hoch | ⚠️ ähnlich | ✅ |
| Zahnstatus / Zahnbogen | hoch (forensisch) | ❌ ändert sich | ✅ | ⚠️ |
| Naevi-/Muttermal-Karte | mittel-hoch | ⚠️ ändert sich | ✅ | ⚠️ |
| Hautporen-/Textur (Level-3) | hoch bei Makro | mittel | ✅ | ⚠️ Makro nötig |
| Nagelbettmuster | mittel | mittel | ✅ | ⚠️ |
| Lippenabdruck (Cheiloskopie) | mittel | hoch | ✅ | ⚠️ |

**Das Zwillingsproblem ist für Gesicht disqualifizierend.** Studien zu eineiigen Zwillingen zeigen: eine True-Accept-Rate über Null wird erst bei einer False-Accept-Rate von **über 10 %** erreicht. Anders gesagt: Gesichtserkennung kann eineiige Zwillinge nicht zuverlässig trennen, egal wie gut das Modell ist — sie misst ein weitgehend genetisch determiniertes Merkmal. Für ein UBI-System bedeutet das: ~30 Millionen Menschen weltweit können systematisch die Identität ihres Zwillings beanspruchen oder werden fälschlich als Duplikat abgelehnt. Iris und Fingerabdruck haben dieses Problem nicht (sie entstehen aus stochastischer Morphogenese, nicht aus dem Genom).

### 3.6 Physiologisch-dynamisch

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| EKG-Morphologie | EER 1–10 % | ⚠️ zustandsabhängig | ✅ | ❌ (Elektroden) |
| EEG („Brainprint") | EER 2–10 % | ❌ | ✅ | ❌ |
| PPG-Wellenform | EER 5–15 % | ❌ stark zustandsabhängig | ✅ | ⚠️ (Kamera+Blitz) |
| Seismokardiographie (IMU) | EER 5–20 % | ❌ | ✅ | ⚠️ |
| Atemmuster / Lungenvolumen | niedrig | ❌ | ✅ | ⚠️ Mikrofon |
| Bioimpedanz | mittel | ⚠️ hydratationsabhängig | ✅ | ❌ |
| Thermogramm (Gesichtsvenen-Wärmebild) | ~10⁻⁴ | mittel | ✅ | ❌ |

Diese Klasse ist als **Liveness-Nachweis** wertvoll und als **Identitätsmerkmal** schwach. Genau so ist der MAX30102 im Whitepaper auch eingesetzt (PPG als Lebendnachweis) — das ist konzeptionell richtig. Ein Puls beweist, dass da ein Mensch ist; er beweist nicht, *welcher*.

### 3.7 Akustisch

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| Stimme / Vokaltrakt | EER 1–5 % (in the wild) | ❌ Alter, Krankheit | ⚠️ sehr ähnlich | ✅ |
| Otoakustische Emissionen (OAE) | EER ~1–5 % | hoch | ✅ | ❌ (Sondenmikrofon) |
| Ohrkanal-Akustik (Resonanz) | EER 1–10 % | hoch | ✅ | ⚠️ nur mit In-Ear |

Stimme ist die am leichtesten zu erfassende Modalität und gleichzeitig die durch generative Modelle am gründlichsten zerstörte: mit wenigen Sekunden Referenzmaterial ist heute ein Klon möglich, der Standard-Sprechererkennung besteht. Als Identitätsanker für Geld ungeeignet.

### 3.8 Verhaltensbiometrie

| Merkmal | Trennschärfe | Stabil | MZ-Zwillinge | Phone |
|---|---|---|---|---|
| Gang (IMU/Video) | EER 5–15 % | ⚠️ Schuhe, Untergrund, Verletzung | ✅ | ✅ |
| Tastatur-/Touch-Dynamik | EER 5–15 % | ⚠️ Kontext | ✅ | ✅ |
| Unterschriftsdynamik | EER 2–8 % | mittel | ✅ | ✅ |
| Blickbewegungsmuster | EER 5–15 % | ❌ | ✅ | ⚠️ |

Verhaltensbiometrie ist **kontinuierliche Risikobewertung**, keine Identitätsfeststellung. Sie taugt, um zu erkennen, dass ein bereits authentifiziertes Gerät den Besitzer gewechselt hat. Für eine einmalige Enrollment-Entscheidung ist eine EER von 10 % gleichbedeutend mit „nicht vorhanden".

---

## 4. Was ein Smartphone physisch hat — und was eine App davon anfassen darf

Das ist die Unterscheidung, an der die meisten Konzepte scheitern.

### 4.1 Sensorinventar (Serien-Smartphone, ohne Zubehör)

| Sensor | App-Zugriff | Biometrisch nutzbar |
|---|---|---|
| RGB-Kamera vorn/hinten | ✅ voller Rohzugriff | ✅ Gesicht, Haut, Ohr, evtl. Fingerfoto |
| Blitz-LED | ✅ | ✅ PPG (Kontakt-Photoplethysmographie) |
| Mikrofon | ✅ | ✅ Stimme, Atem, akustische Sondierung |
| IMU (Accel/Gyro/Magnet) | ✅ | ⚠️ Gang, Zittern, SCG |
| Touchscreen (kapazitiv) | ✅ Druck/Fläche/Timing | ⚠️ Touch-Dynamik — **Auflösung ~1 mm, Papillarleisten ~0,4 mm: reicht prinzipiell nicht** |
| Umgebungslicht / Näherung | ✅ | ❌ |
| Barometer, GPS, WLAN/BLE/UWB | ✅ | ❌ (nur Kontext/Anti-Fraud) |
| **NFC** | ✅ | ✅ **eID/ePass-Chip — siehe §8** |
| ToF / LiDAR (nur manche Modelle) | ⚠️ teils | ⚠️ grobe 3D-Tiefe, Anti-Spoof |
| **Fingerabdrucksensor** | ❌ **nur Ja/Nein** | ❌ |
| **Face ID / Gesichtsentsperrung** | ❌ **nur Ja/Nein** | ❌ |

### 4.2 Die entscheidende Sperre

Die beiden präzisesten Biometriesensoren im Gerät — Fingerabdruckleser und Face-ID-Punktprojektor — sind für Apps **vollständig unzugänglich**. Sowohl Androids `BiometricPrompt` als auch Apples Face ID/Touch ID führen den Abgleich in der TEE bzw. Secure Enclave durch und geben der App ausschließlich „authentifiziert / nicht authentifiziert" zurück. Kein Template, kein Embedding, kein Rohbild, kein stabiler Identifier.

Das ist eine bewusste, richtige Datenschutzentscheidung der Plattformen — und für Aequitas eine harte Wand:

> **Die Biometrie-Hardware des Telefons kann prinzipiell keinen körpergebundenen Identifier liefern. Sie kann nur bestätigen, dass der eingelernte Nutzer *dieses Geräts* anwesend ist.**

Genau in diese Wand ist die aktuelle Implementierung gelaufen (§1). Der Weg vom „Ja" der TEE zum Nullifier führt zwangsläufig über einen Geräteschlüssel — und damit ist der Nullifier gerätegebunden. Das ist kein Implementierungsfehler, den man wegpatchen kann; es ist die einzig mögliche Konsequenz aus dieser API.

**Wer körpergebundene Biometrie auf dem Smartphone will, muss sie zwangsläufig mit der normalen Kamera/dem Mikrofon selbst erheben und selbst auswerten** — und verliert damit sämtlichen Hardware-Schutz und landet direkt im Angriffsszenario aus §7.

---

## 5. Bewertung: was das Smartphone ohne Zubehör pro Modalität wirklich leistet

| Modalität | Erfassbar? | Als Identitätsanker für globalen Nullifier? | Begründung |
|---|---|---|---|
| Gesicht 2D | ✅ gut | ⚠️ **bedingt, nur mit zentraler 1:N-Datenbank** | Eineiige Zwillinge nicht trennbar; Alterung; Injection-Attacken |
| Gesicht 3D | ⚠️ wenige Geräte | ❌ | Rohdaten OS-gesperrt; ToF-Auflösung zu grob |
| Iris | ❌ | ❌ | IR-Sperrfilter; dunkle Iriden im sichtbaren Licht texturlos |
| Venen | ❌ | ❌ | Erfordert 850-nm-Durchleuchtung |
| Fingerabdruck (Foto der Fingerkuppe) | ⚠️ labortauglich | ❌ | Beleuchtung/Fokus/Feuchte; ohne Kontaktfläche keine reproduzierbare Geometrie; Latenzabdrücke aus Fotos in sozialen Medien rekonstruierbar |
| Fingerabdruck (Gerätesensor) | ❌ | ❌ | Nur Ja/Nein (§4.2) |
| Handfläche | ⚠️ | ❌ | Wie Fingerfoto, geringere Trennschärfe |
| Ohrmuschel | ✅ | ❌ | Trennschärfe ~10⁻³, zu niedrig |
| Stimme | ✅ | ❌ | Generativ vollständig klonbar |
| PPG / Puls | ✅ (Kamera+Blitz) | ❌ als Identität, ✅ **als Liveness** | Zustandsabhängig |
| Gang / Touch-Dynamik | ✅ | ❌ | EER 5–15 % |
| **NFC eID/ePass** | ✅ | ✅ **ja — der einzige belastbare Weg** | Staatlich signiert, klonresistent (§8) |

**Zusammenfassung der Spalte 3: Genau eine Zeile trägt ein grünes Häkchen, und es ist nicht Biometrie im engeren Sinne.**

---

## 6. Der eigentliche Bruch: Wiedererkennung ≠ deterministischer Nullifier

Aequitas braucht nicht „erkenne diesen Menschen wieder". Aequitas braucht:

> Aus einem Körper einen **deterministischen, reproduzierbaren, geheimen Wert** ableiten, der bei derselben Person immer identisch und bei jeder anderen verschieden ist — **ohne dass irgendwo eine Datenbank mit Körpermerkmalen liegt.**

Es gibt dafür genau zwei Architekturen.

### Architektur A — Fuzzy Extractor (was das Whitepaper implizit annimmt)

Biometrie ist verrauscht: zwei Aufnahmen derselben Iris unterscheiden sich in 10–20 % der Bits. Ein Fuzzy Extractor korrigiert dieses Rauschen mit öffentlichen Hilfsdaten und liefert einen stabilen Schlüssel. Der Preis ist Entropieverlust — die Fehlerkorrektur, die Rauschen entfernt, entfernt auch Geheimnis.

Stand der Forschung:

| Modalität | Extrahierbare Schlüsselentropie | Quelle |
|---|---|---|
| Iris (NIR, gute Aufnahmen) | **105 Bit @ 92 % TAR** | ACM CCS 2025 — bester publizierter Wert |
| Iris (frühere Arbeiten) | 32 Bit | Vorstand der Technik bis 2024 |
| **Gesicht** | **~45 Bit** | ebd. |

**Was das für Aequitas heißt:**

- Der Bestwert (105 Bit) kommt aus einer Modalität, die ein Smartphone ohne Zubehör nicht erfassen kann (§3.3). Er stammt aus qualitativ hochwertigen NIR-Aufnahmen, und 92 % TAR bedeutet: **8 von 100 echten Menschen werden bei der Wiederholung nicht wiedererkannt** — bei einem UBI-System jeder zwölfte Nutzer, dauerhaft ausgesperrt.
- Der auf dem Smartphone erreichbare Wert (Gesicht, ~45 Bit) ist **kryptographisch wertlos**: 2⁴⁵ ≈ 3,5·10¹³ ist mit heutiger Hardware in Stunden erschöpfend durchsuchbar. Ein Angreifer kann den Schlüsselraum offline durchsuchen und aus den öffentlichen Hilfsdaten sowohl den Nullifier fälschen als auch die Biometrie rekonstruieren.
- Biometrie ist unwiderruflich. Ein kompromittierter 45-Bit-Gesichtsschlüssel lässt sich nicht rotieren — das Gesicht bleibt.

**Fazit A: Auf einem Smartphone ohne Zubehör ist Architektur A nicht realisierbar. Nicht „noch nicht" — mit der vorhandenen Sensorik nicht.**

### Architektur B — Zentrale 1:N-Deduplikation (was Aadhaar und World tun)

Statt eines Schlüssels aus dem Körper: eine Instanz hält Templates aller Registrierten und prüft bei jedem neuen Enrollment gegen alle bisherigen. Passt keiner: neue Identität, neuer zufälliger Nullifier, dauerhaft mit dem Template verknüpft.

Das funktioniert — und hat einen mathematischen Preis, den man nicht wegverhandeln kann:

Bei *N* Registrierten sind pro Enrollment *N* Vergleiche nötig. Damit ein legitimer Neuer nicht fälschlich als Duplikat abgewiesen wird, muss die Falschakzeptanzrate pro Vergleich deutlich unter 1/*N* liegen. Bei *N* = 10⁹ heißt das **FMR < 10⁻¹⁰ bei gleichzeitig niedriger Falschrückweisung** — jenseits jeder einzelnen Modalität. Deshalb fährt Aadhaar zehn Finger *plus* zwei Iriden *plus* Gesicht mit zertifizierter Erfassungshardware und braucht bei 1,3 Mrd. Menschen trotzdem manuelle Adjudikation für Grenzfälle. Die Top-FRVT-Zahl von 0,07 % Fehlerrate bei 12 Mio. Galerie klingt gut, bedeutet aber bei 1 Mrd. Nutzern immer noch Hunderttausende Fehlentscheidungen — und sie stammt aus Visa-/Grenzkontrollaufnahmen unter Aufsicht, nicht aus Selfies.

Zusätzlich: Architektur B **widerspricht dem zentralen Werbeversprechen dieses Projekts.** `WHITEPAPER.md:412-416` sagt zu, dass Fingerabdruck-, Venen- und Irisdaten „niemals" gespeichert werden. 1:N-Deduplikation setzt genau diese Speicherung voraus. Man kann das Vertrauensproblem mildern (verteilte Betreiber, sichere Enklaven, Multi-Party-Computation, verschlüsselte Templates), aber nicht auflösen: irgendwo muss jemand vergleichen können.

**Fazit B: technisch machbar, aber teuer, zentralisierend, und unvereinbar mit dem aktuellen Whitepaper-Versprechen.**

### Fazit §6

> Es gibt keine dritte Architektur. Wer einen körpergebundenen Nullifier ohne Biometrie-Datenbank will, braucht einen Fuzzy Extractor mit ≥ 100 Bit — und damit Iris-Hardware. Wer bei Smartphone-Sensorik bleibt, braucht eine zentrale Datenbank. Aequitas hat bisher weder das eine noch das andere und beweist stattdessen den Besitz eines Telefons.

---

## 7. Angriffsflächen: warum unbeaufsichtigte Fern-Biometrie der schwerste Fall ist

Selbst wenn §6 gelöst wäre, bleibt das Umfeld. Unbeaufsichtigtes Remote-Enrollment ist der schwierigste Betriebsmodus der gesamten Biometrie — der Angreifer besitzt Sensor, Betriebssystem und Übertragungsweg.

**Presentation Attacks (klassisch)** — Foto, Video, Maske, Silikonfinger, Gipsabguss vor die echte Kamera. Dagegen hilft PAD (ISO/IEC 30107-3), und es funktioniert einigermaßen.

**Injection Attacks (das eigentliche Problem)** — der Angreifer speist einen synthetischen Videostrom direkt in die Aufnahmeschicht ein, unterhalb jeder Liveness-Prüfung. Ein virtueller Kameratreiber gibt sich der App gegenüber als normale Kamera aus; fortgeschrittene Varianten kapern den Videopfad auf OS-Ebene. **Liveness-Erkennung greift hier prinzipiell nicht** — es wird dem Sensor nie etwas präsentiert, der Sensor wird ersetzt.

Die Lage 2025/26:
- Native Virtual-Camera-Angriffe: **+2.665 % Jahr über Jahr** (iProov Threat Intelligence)
- 2024 war das Jahr, in dem Injection- die Presentation-Angriffe als primären Vektor überholten
- Deepfake-as-a-Service ab **~15 USD pro synthetischer Identität** (Group-IB, 2026)
- Das WEF hat Virtual-Camera-Injection gegen Live-Selfie-KYC getestet und eine breite Palette aktiver Liveness-Implementierungen umgangen

Für Aequitas heißt das konkret: **ein Angriff, der pro gefälschter Identität 15 USD kostet, gegen ein System, das pro Registrierung 1.000 AEQ ausschüttet, ist ein reines Rechenexempel.** Sobald AEQ einen Marktwert über wenigen Cent hat, ist Gesichts-Selfie-Verifikation ökonomisch gebrochen.

**Was dagegen hilft** (und in jedes ernsthafte Design gehört):
- **Play Integrity / DeviceCheck / App Attest** — beweist, dass ein echtes, ungerootetes Gerät mit unmanipulierter App-Binärdatei sendet
- **Kamera-Attestierung** (Android `KeyStore`-Attestation über Frame-Hashes) — bindet Bilddaten an Gerätehardware
- **Signal-Level-Deepfake-Erkennung** — Kompressionsartefakte, PPG-Konsistenz im Video, Sensor-Rauschmuster
- **Rate-Limits und Kohortenanalyse** — Registrierungsraten pro IP/Gerätemodell/Zeitfenster
- **Ökonomische Dämpfung** — Registrierungsprämie erst nach Wartezeit/Bewährung, nicht sofort

Die aktuelle Codebasis hat von dieser Liste **nichts** (kein Play-Integrity-Check, keine Attestierung, keine Kohortenanalyse in `register.go`). Der einzige vorhandene Schutz — die Nullifier-Eindeutigkeit — greift gegen Injection gar nicht, weil jeder Angreifer einen frisch gültigen Nullifier erzeugt.

---

## 8. Was tatsächlich funktioniert: NFC-Auslesen des eID-/ePass-Chips

Das ist der einzige Weg, auf dem ein Serien-Smartphone ohne Zubehör eine belastbare Einmaligkeitsaussage produziert.

### 8.1 Mechanik

Jeder ICAO-9303-konforme elektronische Reisepass (und die meisten modernen nationalen eIDs) enthält einen kontaktlosen Chip mit:
- **DG1** — MRZ-Daten (Name, Geburtsdatum, Dokumentnummer, Staat)
- **DG2** — das biometrische Lichtbild, hochwertig, staatlich erfasst
- **SOD** — Document Security Object: Hashes aller Datengruppen, signiert vom Document Signer, dessen Zertifikat wiederum von der Country Signing CA (CSCA) des Staates signiert ist
- **Active Authentication / Chip Authentication** — Challenge-Response mit einem privaten Schlüssel, der den Chip nie verlässt

Ein NFC-fähiges Telefon liest den Chip (Zugriffsschlüssel aus der optisch gescannten MRZ per BAC/PACE), verifiziert die Signaturkette gegen die CSCA-Zertifikate und führt Active Authentication durch. Ergebnis: **kryptographischer Beweis, dass ein physisch anwesendes, nicht geklontes, staatlich ausgestelltes Dokument vorliegt.**

Dieser Ansatz ist produktiv im Einsatz — ZKPassport unterstützt Chips aus 130+ Ländern, Rarimo betreibt eine On-Chain-Registry mit Merkle-Baum-Inklusionsbeweisen, damit nicht bei jeder Transaktion die volle Signaturkette bewiesen werden muss.

### 8.2 Warum das zu Aequitas' Architektur passt

Es passt bemerkenswert gut, weil die bestehende ZK-Infrastruktur genau die richtige Form hat:

- Nullifier = `Poseidon(dokumentgebundenes Geheimnis ‖ domain_separator)` — dieselbe Konstruktion wie in v3, nur mit einem sinnvollen Input
- Der Groth16-Verifier-Deploymentpfad, `IsNullifierUsed`, die atomare Nullifier-Beanspruchung, das Outbox-TX-Modell — **alles bleibt unverändert**. Es ändert sich, was in den Circuit geht, nicht der Rest der Chain.
- Es werden weiterhin keine biometrischen Rohdaten gespeichert — das Whitepaper-Versprechen aus Zeile 412–416 bleibt einlösbar
- Kein zentraler Betreiber, keine Template-Datenbank

**Körperbindung entsteht über einen zweiten Schritt:** Selfie gegen das signierte DG2-Lichtbild abgleichen, plus Liveness. Damit hängt der Nullifier nicht nur am Dokument, sondern an der Person, die es vorzeigt — und der Gesichtsabgleich ist hier zulässig, weil er nur 1:1 gegen ein staatlich signiertes Referenzbild läuft (§3.5' Zwillingsproblem entschärft sich: Zwillinge haben verschiedene Pässe).

### 8.3 Die ehrlichen Grenzen

- **Abdeckung.** Weltweit besitzen grob 15–20 % der Menschen einen Reisepass, mit extremer regionaler Spreizung (Skandinavien >80 %, viele Länder Subsahara-Afrikas <5 %). Nationale eIDs verbessern das, schließen die Lücke aber nicht. **Für ein UBI-Projekt, dessen erklärter Zweck globale Teilhabe ist, ist das die schmerzhafteste Einschränkung des gesamten Ansatzes** — und sie muss offen benannt statt weggerechnet werden.
- **Staatliches Vertrauen.** Ein Staat kann beliebig viele gültige Pässe ausstellen. Die Sybil-Resistenz ist genau so gut wie die Passausgabe des schwächsten akzeptierten Landes.
- **Mehrfachstaatsbürgerschaft.** Zwei Pässe = zwei Nullifier, wenn der Nullifier am Dokument hängt. Mildernd: Nullifier aus stabilen Personendaten (Geburtsdatum + Name + Geburtsort) statt aus der Dokumentnummer bilden — kollidiert aber mit Namensänderungen und schafft Namensvetter-Kollisionen.
- **Neuausstellung.** Ein neuer Pass hat eine neue Dokumentnummer. Wer den Nullifier an die Dokumentnummer bindet, sperrt Nutzer bei Passerneuerung aus (oder erlaubt ihnen eine Zweitidentität).
- **Ältere Chips.** Nicht alle Dokumente unterstützen Active/Chip Authentication; bei reiner Passive Authentication ist ein Chip-Klon möglich.
- **Datensparsamkeit.** Die App liest zwangsläufig vollständige Ausweisdaten. Sie dürfen das Gerät nicht verlassen — nur der ZK-Beweis geht raus. Das ist implementierbar, aber es ist eine harte Anforderung, keine Option.

---

## 9. Empfehlungen

### 9.1 Sofort — Ehrlichkeit herstellen (kein Engineering nötig)

1. **`WHITEPAPER.md` §3.1 korrigieren.** „Phase 1 *(aktiv)*" ist unzutreffend: das R503/MAX30102-Kit ist nicht deployt, und der Produktionspfad misst keinen Körper. Entweder als „geplant" kennzeichnen oder den Abschnitt durch eine Beschreibung des tatsächlichen Verfahrens ersetzen.
2. **Die Zahlentabellen streichen oder korrigieren.** „1 von 10⁹⁰" (10 Finger) und „1 von 10⁷⁸" (Iris) sowie „Falsch-Positiv-Rate < 10⁻⁷⁸" sind nicht belegbar (§2, §3.2, §3.3). Belastbare Bereiche: 10 Finger 10⁻⁸…10⁻¹², Iris 10⁻⁶…10⁻¹¹.
3. **Die Gerätebindung explizit dokumentieren.** Solange der Nullifier gerätegebunden ist, muss an jeder Stelle, an der „Proof of Humanity" steht, „Proof of Device" stehen. Der Kommentar in `api_html.go:77-79` sagt es bereits richtig — die nutzersichtbaren Texte tun es nicht.

### 9.2 Kurzfristig — die billigsten wirksamen Härtungen

4. **Play Integrity / App Attest** vor jeder Registrierung erzwingen (§7). Einzelmaßnahme mit dem besten Verhältnis von Aufwand zu Wirkung: sie hebt die Kosten pro Sybil-Identität von „Emulator" auf „physisches Gerät".
5. **Registrierungsprämie zeitlich strecken.** 1.000 AEQ sofort bei Registrierung ist maximaler Anreiz bei minimalen Angriffskosten. Vesting über Wochen mit Widerrufsmöglichkeit reduziert den Erwartungswert eines Massenangriffs drastisch — ohne jede Krypto-Änderung.
6. **`chain_bio_hashes: 0` root-causen** (offener Punkt aus `AUDIT_2026-07-12.md`). Auch wenn die Tabelle im aktuellen Design wenig schützt: eine zweite Verteidigungslinie, die für niemanden aktiv ist, ist schlechter als keine, weil sie in der Risikobewertung mitzählt.

### 9.3 Mittelfristig — die strategische Entscheidung

7. **NFC-eID/ePass als primären Anker implementieren** (§8). Der ZK-Stack (Circuit v3, Poseidon-Nullifier, `IsNullifierUsed`, Outbox) bleibt strukturell erhalten; getauscht wird der Circuit-Input. Konkrete Schritte: MRZ-Scan → PACE/BAC → DG1/DG2/SOD lesen → CSCA-Kette verifizieren → Active Authentication → Nullifier aus stabilen Personendaten ableiten → ZK-Beweis on-device. Für die Nullifier-Ableitung ist die Wahl zwischen Dokumentnummer und Personendaten (§8.3) die zentrale Designentscheidung und sollte bewusst getroffen werden, nicht implizit.
8. **Selfie-Match gegen DG2 + PAD** als zweite Stufe, um Dokument an Person zu binden. Hier ist Gesichtserkennung angemessen eingesetzt: 1:1 gegen ein signiertes Referenzbild, nicht 1:N gegen eine Datenbank.
9. **Fallback-Pfad für Menschen ohne Dokument** von Anfang an mitdenken (§8.3). Ohne diesen Pfad ist die Abdeckung strukturell auf eine Minderheit der Weltbevölkerung begrenzt, und das Projekt verfehlt seinen erklärten Zweck. Realistische Optionen: Web-of-Trust-Bürgschaften, an Ort und Zeit gebundene persönliche Verifikationsevents, Kooperation mit bestehenden Personhood-Netzwerken.

### 9.4 Was man nicht tun sollte

10. **Kein Selfie-Only-Enrollment ohne Dokumentenanker.** Es addressiert das Zwillingsproblem nicht (§3.5), erfordert eine zentrale Template-Datenbank (§6-B) und ist gegen Injection-Angriffe für ~15 USD pro Identität offen (§7).
11. **Keine Iris- oder Venenerkennung über die Standardkamera versprechen.** Physikalisch nicht möglich (§3.3, §3.4). Diese Zusage einzugehen und später zurückzuziehen kostet mehr Glaubwürdigkeit, als sie kurzfristig einbringt.
12. **Den vorhandenen ZK-Stack nicht ersetzen.** Er ist der solideste Teil des Systems. Das Problem war nie die Kryptographie, sondern was in sie hineingereicht wird.

---

## Quellen

- [Daugman, „How Iris Recognition Works" (249 Freiheitsgrade, 3,2 Bit/mm²)](https://www.cl.cam.ac.uk/~jgd1000/irisrecog.pdf)
- [Daugman, „The importance of being random: statistical principles of iris recognition"](https://www.cl.cam.ac.uk/~jgd1000/patrec.pdf)
- [Iris Recognition: Inherent Binomial Degrees of Freedom (arXiv)](https://arxiv.org/pdf/2006.16107)
- [„Fuzzy Extractors are Practical: Cryptographic Strength Key Derivation from the Iris", ACM CCS 2025 — 105 Bit @ 92 % TAR, Vergleichswerte 32 Bit Iris / 45 Bit Gesicht](https://dl.acm.org/doi/10.1145/3719027.3765098) ([ePrint](https://eprint.iacr.org/2024/100))
- [Untargeted Near-collision Attacks on Biometrics: Real-world Bounds and Theoretical Limits](https://arxiv.org/pdf/2304.01580)
- [NIST Face Recognition Vendor Test (FRVT)](https://www.nist.gov/programs-projects/face-recognition-vendor-test-frvt) · [FRVT Part 8, Demographic Effects (NISTIR 8429)](https://pages.nist.gov/frvt/reports/demographics/nistir_8429.pdf)
- [NEC, NIST FRVT 1:N mit 12-Mio.-Galerie, 0,07 % Fehlerrate (2025)](https://www.nec.com/en/press/202504/global_20250409_01.html)
- [Benchmarking Human Face Similarity Using Identical Twins (arXiv)](https://arxiv.org/pdf/2208.11822) · [IET Biometrics](https://ietresearch.onlinelibrary.wiley.com/doi/full/10.1049/bme2.12090)
- [Fingerprint Recognition with Identical Twin Fingerprints (PLOS ONE)](https://journals.plos.org/plosone/article?id=10.1371%2Fjournal.pone.0035704)
- [A Study of Multibiometric Traits of Identical Twins (MSU) — Iris > Fingerabdruck > Gesicht bei Zwillingen](http://biometrics.cse.msu.edu/Publications/Multibiometrics/Sunetal_IdentTwins_SPIE10.pdf)
- [Android: Apps erhalten nie biometrische Rohdaten, nur Auth-Ergebnis (TEE-gestützt)](https://medium.com/softaai-blogs/how-tee-backed-fingerprint-authentication-works-in-android-for-enhanced-security-3e2ee13ba6e8)
- [Security pitfalls in authenticating users and protecting secrets with biometry on mobile devices](https://clement.notin.org/blog/2019/12/17/security-pitfalls-in-authenticating-users-and-protecting-secrets-with-biometry-on-mobile-devices-apple-android/)
- [World: „Opening the Orb" — multispektrale Sensorik und Gimbal-Schmalfeldkamera für Iris](https://world.org/blog/engineering/opening-orb-look-inside-worldcoin-biometric-imaging-device)
- [Vitalik Buterin, „What do I think about biometric proof of personhood?"](https://vitalik.eth.limo/general/2023/07/24/biometric.html)
- [Injection-Angriffe: virtuelle Kameratreiber unterhalb der Liveness-Prüfung](https://www.duckduckgoose.ai/blog/how-injection-attacks-feed-deepfakes-into-verification) · [Bypass von Liveness-Checks](https://www.duckduckgoose.ai/blog/how-deepfakes-bypass-liveness-checks-in-real-identity-verification-systems)
- [Deepfake Injection Attacks Bypass Identity Verification in 2026 (+2.665 % Virtual-Camera-Angriffe, WEF-Test)](https://www.deepidv.com/media/articles/deepfake-injection-attacks-bypass-identity-verification-2026) · [Warum die meisten Systeme scheitern](https://www.deepidv.com/media/articles/deepfake-detection-2026-injection-attacks)
- [Shufti Identity Fraud Index 2026](https://shuftipro.com/resources/whitepapers-reports/deepfake-identity-fraud-index-report-2026/)
- [ZKPassport — NFC-Chip-Auslesen, 130+ Länder](https://zkpassport.id/) · [Docs/FAQ](https://docs.zkpassport.id/faq) · [Circuits](https://github.com/zkpassport/circuits)
- [Rarimo ZK Passport — Nullifier, Active Authentication, On-Chain-Registry](https://docs.rarimo.com/zk-passport/) · [passport-zk-circuits](https://github.com/rarimo/passport-zk-circuits)

## Repo-Belege

- `x/humanity/keeper/register.go:186-200, 377-394, 399-446, 525-539` — Nullifier-Pfad, Circuit-v3-Zwang, Doppelnutzungsschutz
- `x/humanity/keeper/api_html.go:77-79` — Gerätebindung als bekanntes Sybil-Problem benannt
- `aequitas-dapp.html:393-406` — Audit-Korrektur: „no client has ever scanned a face"
- `WHITEPAPER.md:124-192, 394-445` — Hardware-Kit als „aktiv", Entropie-Tabellen, Speicherversprechen
- `AUDIT_2026-07-12.md` — `chain_bio_hashes: 0` bei 6 Humans; WebAuthn-Gerätebindung; toter `?proof=`-Pfad
