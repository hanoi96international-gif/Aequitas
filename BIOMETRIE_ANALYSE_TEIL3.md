# Teil 3: Die drei Wände — warum das Problem nicht auf der Identitätsebene liegt

Stand: 2026-07-29 · Fortsetzung von `BIOMETRIE_ANALYSE.md` und `BIOMETRIE_ANALYSE_TEIL2.md`

---

## 0. Das Ergebnis vorweg

Teil 1 und 2 haben gezeigt, dass ein Smartphone keinen körpergebundenen Nullifier liefern kann. Dieser Teil geht eine Ebene tiefer und kommt zu einem unbequemeren Schluss:

> **Auch wenn du „1 Registrierung = 1 Mensch" perfekt lösen würdest, hättest du dein Problem nicht gelöst.**

Es gibt drei aufeinander aufbauende Wände. Die erste ist technisch, die zweite strukturell, die dritte ökonomisch. Aequitas arbeitet seit Beginn an der ersten — und die dritte ist die, die tatsächlich entscheidet.

Der Ausweg liegt nicht darin, Identität besser zu prüfen. Er liegt darin, das Geld so zu bauen, dass falsche Identitäten wertlos werden. Genau das hat Circles UBI getan, und es ist die einzige mir bekannte Konstruktion, die alle drei Wände umgeht.

---

## 1. Wand 1 — Tor 4: kein deterministischer Nullifier aus dem Körper

Kurzfassung aus Teil 2, §5. Ein Fuzzy Extractor liefert `Gen(w) → (R, P)` und `Rep(w', P) → R`. Wer sich ein zweites Mal registriert, ruft `Gen` erneut auf und erhält aus demselben Körper einen anderen Schlüssel. **Fuzzy Extractors authentifizieren, sie deduplizieren nicht.**

Gilt unabhängig von der Sensorqualität, also auch mit Iris-Hardware. Einmaligkeit braucht deshalb zwingend eine Vergleichsinstanz oder einen Anker außerhalb des Körpers.

**Das ist die Wand, an der Aequitas bisher gearbeitet hat.** Sie ist real — aber sie ist die harmloseste der drei.

---

## 2. Wand 2 — Die Relay-Wand: Fernverifikation kann Vermietung nicht verhindern

### 2.1 Das formale Argument

In der Sicherheitsforschung ist das Relay-Problem gelöst — durch **Distance Bounding**. Der Prüfer misst die Laufzeit von Challenge und Response. Ein Relay kann Verzögerung nur *hinzufügen*, niemals die Lichtgeschwindigkeit unterbieten. Deshalb ist Distance Bounding relay-resistent **by construction, not by patch**.

Die Voraussetzung dafür ist entscheidend: Der Prüfer muss einen **physischen Endpunkt in der Nähe des Beweisenden kontrollieren** — ein Zahlungsterminal, ein Lesegerät, eine Antenne.

Bei Fernverifikation der Personenhaftigkeit existiert dieser Endpunkt per Definition nicht. Der einzige Endpunkt ist das Telefon des Nutzers, und im Vermietungsszenario gehört es zur Gegenseite. Daraus folgt unmittelbar:

> **Kein Fernverfahren kann verhindern, dass ein echter Mensch die Prüfung für einen Betreiber durchführt.** Nicht mit besserer Biometrie, nicht mit mehr Kanälen, nicht mit Challenge-Response. Der Mensch *ist* echt, die Prüfung *ist* ehrlich — nur der Ertrag fließt woanders hin.

Das ist kein Implementierungsproblem. Es ist dieselbe Struktur, die Distance Bounding erfunden hat, in einem Setting, in dem Distance Bounding nicht anwendbar ist.

### 2.2 Die Empirie: Idena

Der Mechanismus, den Teil 2 §4 als vielversprechend beschrieben hat — Gleichzeitigkeit als Knappheit — ist nicht neu. Bryan Ford hat ihn 2008 als **Pseudonym Parties** publiziert, mit exakt derselben Begründung: Ein Mensch kann nicht an zwei Orten gleichzeitig sein. In seiner Evaluation von 2020 kommt er zu dem Schluss, föderierte Pseudonym-Parties seien der einzige plausible Weg, **Inklusion, Gleichheit, Sicherheit und Privatsphäre gleichzeitig** zu erfüllen.

**Idena hat es online implementiert.** Synchrone Validierungszeremonien, weltweit gleichzeitig entschlüsselte FLIP-Aufgaben, vier aufeinanderfolgende Zeremonien. Start August 2019.

Ohlhaver dokumentiert in *„Compressed to 0"* (Harvard Ash Center), was daraus wurde: Bis Mai 2022 bildeten sich verdeckte Pools unter **„Puppeteers"** — Betreibern, die echte Menschen dafür bezahlten, regelmäßig ihre Einmaligkeit nachzuweisen, im Tausch gegen deren geheime Schlüssel und die Kontrolle über die Konten. Ohlhaver beschreibt das als fortlaufende ökonomische Beziehung zwischen Prinzipal und Agent, deren Charakter von der Tiefe der Asymmetrie abhängt.

Das System hat funktioniert. Es hat echte, einmalige, gleichzeitig anwesende Menschen verifiziert — und wurde trotzdem gekapert. **Wand 2 ist empirisch belegt, nicht bloß theoretisch.**

### 2.3 Fords Antwort und ihr Preis

Ford besteht aus genau diesem Grund auf **physischer Präsenz**: Erst wenn der Prüfer den Ort kontrolliert, greift das Gleichzeitigkeitsargument wirklich. Der Preis ist Logistik — periodische Präsenzveranstaltungen, föderiert organisiert, mit Tokens begrenzter Gültigkeit bis zum nächsten Termin.

Das ist ein ehrlicher Trade: reale Sicherheit gegen realen organisatorischen Aufwand. Es ist kein Weg, der sich in eine App einbauen lässt.

### 2.4 Die Unterscheidung, die Idena gekostet hat

Hier liegt ein Denkfehler, der sich durch die gesamte Personhood-Literatur zieht und den es zu benennen lohnt:

**Ein gemieteter echter Mensch ist kein Sybil-Angriff.**

Ein Sybil-Angriff ist, wenn eine Entität *mehr Anteile beansprucht, als ihr Menschen zur Verfügung stehen*. Wenn ein Betreiber tausend echte Menschen dafür bezahlt, ihre Anteile abzutreten, dann hat das System korrekt gearbeitet: Tausend Menschen haben je einen Anteil erhalten. Dass sie ihn verkauft haben, ist ein Verteilungs- und Machtproblem, kein Identitätsproblem.

Und daraus folgt etwas Wichtiges: **Es gibt keine technische Abwehr dagegen, dass jemand seinen Anteil freiwillig weggibt — und es sollte auch keine geben.** Ein System, das das verhindern wollte, müsste seine Nutzer entmündigen.

Das realistische Ziel ist deshalb nicht, Vermietung zu unterbinden, sondern:

1. Sie **unprofitabel** zu machen (Wand 3, unten), und
2. **Defektion jederzeit möglich** zu halten — der Mensch muss sein Konto jederzeit zurückholen können.

Punkt 2 ist die Stelle, an der Biometrie auf dem Handy tatsächlich funktioniert: **Rückholbarkeit braucht nur Ähnlichkeit, keinen reproduzierbaren Schlüssel.** Tor 4 gilt hier nicht. Ein Gesichts-Embedding plus die Kohärenzprüfungen aus Teil 2 reichen völlig aus, um zu belegen: *derselbe Körper, der dieses Konto eröffnet hat, ist jetzt hier.*

Wenn der Schlüsselverkauf jederzeit widerrufbar ist, kauft niemand mehr Schlüssel. Der Mietmarkt kollabiert nicht durch Kryptographie, sondern durch fehlende Eigentumssicherheit auf Käuferseite.

**Angriff auf die Rückholbarkeit selbst:** Zwang („halt still, während ich dein Gesicht filme"). Gegenmittel ist keine bessere Biometrie, sondern **Zeit**: Rückholung stellt die Kontrolle erst nach einer Frist von z. B. sieben Tagen her, und die Frist ist nicht abkürzbar. Ein Zwingender müsste sein Opfer eine Woche lang festhalten. Für legitime Nutzer ist die Frist folgenlos.

---

## 3. Wand 3 — Die Fungibilitätswand

Die tiefste der drei, und die einzige, die Aequitas selbst gebaut hat.

### 3.1 Warum die Arbitrage funktioniert

Warum verkauft ein Mensch seinen Anteil für *p*, wenn er ihn selbst für *V > p* behalten könnte? Ohlhavers Befund ist präziser als „Leute verkaufen ihre Schlüssel": Die Beziehung entsteht aus **Asymmetrie** — Wissen, Liquidität, Risikotoleranz.

- Der Betreiber kann tausend Konten bedienen, verwalten, liquidieren; der einzelne Nutzer oft nicht.
- Der Nutzer bevorzugt sichere sofortige Zahlung gegenüber unsicherem, illiquidem Token-Wert.
- In einkommensschwachen Kontexten ist *p* real und *V* spekulativ.

**Die Arbitrage ist also nicht auf Identität, sondern auf Liquidität und Wissen.** Das ist eine bessere Diagnose, weil sie Hebel benennt, die es tatsächlich gibt.

### 3.2 Und warum Fungibilität sie maximiert

Ein global fungibler Token macht beide Angriffe maximal lohnend:

- **Sybil**: Jede zusätzliche Identität liefert dieselbe frei handelbare Einheit. Der Ertrag ist linear in der Kontenzahl.
- **Vermietung**: Der Betreiber kann tausend Ströme in einem Topf bündeln und liquidieren. Aggregation ist verlustfrei.

Genau das ist Aequitas' Tokenomics: eine Einheit, gleich geprägt, frei transferierbar. **Diese Eigenschaft erzeugt das Identitätsproblem, das der Rest des Systems zu lösen versucht.**

> **Globale Fungibilität und identitätsfreie Sybil-Resistenz stehen in direktem Widerspruch.** Man kann nicht beides haben.

---

## 4. Der Ausweg: das Problem auf der Geldebene auflösen

Circles UBI hat genau hier angesetzt, und die Konstruktion verdient genaue Betrachtung, weil sie alle drei Wände umgeht.

### 4.1 Die Mechanik

**Jeder Mensch prägt seine eigene Währung.** Alice prägt AliceCoins, Bob prägt BobCoins. Zahlungen laufen über **transitive Vertrauenspfade**: Wenn Alice Bob vertraut und Bob Carol, findet ein Pathfinder-Algorithmus einen Weg von Carol zu Alice.

Der entscheidende Satz aus dem Whitepaper:

> Selbst wenn Alice hundert Fake-Konten anlegt und sie einander vertrauen lässt, kann sie **nie mehr ausgeben als ihre AliceCoins** — denn das ist das Einzige, dem andere Nutzer vertrauen.

**Sybil wird nicht verhindert. Sybil wird wertlos.** Die Fake-Konten prägen fleißig Geld, dem niemand vertraut, also nicht ausgebbar ist. Das Identitätsproblem löst sich nicht — es verschwindet.

Ergänzt durch **Demurrage**: eine jährliche Inflation von 7 %, die Horten bestraft, die Umlaufgeschwindigkeit erhöht und über die Zeit zwischen Früh- und Spätteilnehmern ausgleicht. Ein Betreiber, der tausend Ströme sammelt, verliert kontinuierlich.

Die Sybil-Abwehr ist damit an die Nutzer ausgelagert, die ein natürliches Eigeninteresse haben, Fake-Konten nicht zu vertrauen — es würde ihr eigenes Netz entwerten. Kein Gatekeeper, keine Behörde, keine Biometrie.

### 4.2 Der Preis, ungeschönt

- **Keine globale Fungibilität.** Ein Circles-Guthaben ist nur so weit ausgebbar, wie Vertrauenspfade reichen. Das ist die Eigenschaft, die die Sicherheit trägt — und es ist genau die Eigenschaft, die ein „normaler" Token verspricht.
- **Es funktioniert dort, wo reale Beziehungen dicht sind.** Berichte über die Praxis kontrastieren einen gescheiterten Berliner Anlauf mit einem funktionierenden in Bali — dichte lokale Ökonomie gegen abstraktes Online-Publikum. Das ist eine ernstzunehmende Warnung für ein Projekt, das global-digital gedacht ist.
- **Bootstrapping ist langsam** und braucht echte soziale Dichte.

---

## 5. Was das für Aequitas bedeutet

Die unbequeme Fassung: **Das Identitätsproblem ist ein Symptom der Tokenomics.** Solange AEQ eine gleich geprägte, global fungible Einheit ist, ist die Einmaligkeitsprüfung der einzige Schutzwall — und dieser Wall ist mit Standardhardware nicht zu bauen.

Drei Wege, jeder mit ehrlichem Preis:

| Weg | Mechanik | Preis |
|---|---|---|
| **A — Fungibilität aufgeben** | Circles-Muster: persönliche Prägung, Vertrauenspfade, Demurrage | AEQ wäre kein handelbarer Token mehr. Tiefer Eingriff, aber die einzige Konstruktion, die ohne Identitätsprüfung auskommt |
| **B — Anker importieren** | World ID (≈ 18 Mio. verifizierte Menschen in 160 Ländern, on-chain verifizierbar), später eID/Pass, mehrere unabhängige Quellen | Fungibilität bleibt. Abdeckung begrenzt, fremde Fehler geerbt. Kein Eigenbau nötig |
| **C — Ökonomische Dämpfung** | Vesting, Demurrage, konvexe Auszahlung, Rückholbarkeit, retrospektive Cluster-Erkennung mit Rückabwicklung | Verhindert nichts, macht aber alles unprofitabel. Sofort umsetzbar, kein Anker nötig |

**A und B schließen einander nicht aus, und C gehört in jeden Fall dazu.**

### 5.1 Der Perspektivwechsel, der am meisten bringt

Registrierung ist ein Einmalereignis und deshalb billig zu farmen. Ausgeben ist fortlaufend.

> **Sichere nicht die Registrierung, sichere die Verwendung.**

Wenn eine Auszahlung einen lebenden Körper erfordert — nicht nur einen Schlüssel —, dann skalieren die Angriffskosten mit der Nutzung statt mit der Kontenzahl. Ein Betreiber müsste seinen Menschen bei *jeder* Auszahlung präsent haben. Und wenn der Mensch ohnehin präsent ist, verschiebt sich die Beziehung von einem einmaligen Schlüsselverkauf zu einer fortlaufenden Aushandlung, aus der er jederzeit aussteigen kann.

Genau dafür sind die Multikanal-Prüfungen aus Teil 2 §4 geeignet — und nur dafür. Sie beweisen Anwesenheit, nicht Identität. Auf der Verwendungsseite ist Anwesenheit genau das Richtige.

---

## 6. Bilanz

| Wand | Natur | Aequitas' bisherige Arbeit |
|---|---|---|
| 1 — Tor 4 | technisch | ✅ hier wurde gearbeitet |
| 2 — Relay/Vermietung | strukturell | ❌ nicht adressiert |
| 3 — Fungibilität | ökonomisch | ❌ selbst erzeugt |

Alle bisherige Anstrengung — Fingerabdruck-Kit, Gesichtsmodul, Blinzeltest, ZK-Nullifier — richtet sich gegen Wand 1. Selbst ein vollständiger Erfolg dort ließe Wand 2 und 3 unberührt, und Idena beweist, dass Wand 2 allein genügt, um ein technisch funktionierendes System zu kapern.

**Die produktivsten offenen Aufgaben sind daher nicht biometrisch:**

1. **Rückholbarkeit mit Zeitschloss** — der Körper holt das Konto jederzeit zurück, wirksam nach Frist. Zerstört den Mietmarkt. Nach dieser Recherche hat das noch niemand gebaut; hier läge ein tatsächlicher Beitrag.
2. **Verwendungsseitige statt registrierungsseitige Prüfung** — Angriffskosten skalieren mit Nutzung.
3. **Ökonomische Dämpfung** — Vesting, Demurrage, konvexe Auszahlung, retrospektive Rückabwicklung. Sofort umsetzbar.
4. **Anker importieren statt bauen** — 18 Millionen verifizierte Menschen sind bereits vorhanden.

Und die Frage, die vor allen anderen beantwortet werden muss, ist keine technische:

> **Soll AEQ ein global fungibler Token sein?**

Ein Ja bedeutet: Einmaligkeit muss von außen kommen (Weg B), und ökonomische Dämpfung ist Pflicht, nicht Kür.
Ein Nein öffnet Weg A — die einzige bekannte Konstruktion, die ohne jede Identitätsprüfung auskommt.

---

## Quellen

- [Bryan Ford, „Identity and Personhood in Digital Democracy: Evaluating Inclusion, Equality, Security, and Privacy in Pseudonym Parties and Other Proofs of Personhood" (arXiv 2011.02412)](https://arxiv.org/pdf/2011.02412) · [Projektseite](https://bford.info/pub/soc/personhood/)
- [Ohlhaver, „Compressed to 0: The Silent Strings of Proof of Personhood" (Harvard Ash Center)](https://ash.harvard.edu/wp-content/uploads/2024/06/proof-of-personhood_ohlhaver.pdf)
- [Idena — Validierungszeremonie-Protokoll](https://docs.idena.io/docs/developer/validation) · [Technologie-Whitepaper](https://docs.idena.io/docs/wp/technology)
- [„Who Watches the Watchmen? A Review of Subjective Approaches for Sybil-Resistance in Proof of Personhood Protocols" (Frontiers in Blockchain)](https://www.frontiersin.org/journals/blockchain/articles/10.3389/fbloc.2020.590171/full)
- [Circles UBI Whitepaper](https://github.com/CirclesUBI/whitepaper) · [Handbook-Fassung](https://handbook.joincircles.net/docs/developers/whitepaper/) · [Circles v2](https://whitepaper.aboutcircles.com/)
- [„Universal basic income on blockchain: the case of Circles UBI" (Frontiers in Blockchain, 2024)](https://www.frontiersin.org/journals/blockchain/articles/10.3389/fbloc.2024.1362939/full)
- [Distance Bounding gegen Relay-Angriffe — Laufzeitmessung als bauartbedingter Schutz](https://needcode.io/solutions/relay-attack-prevention/) · [Shedding Light on RFID Distance Bounding Protocols and Terrorist Fraud Attacks](https://arxiv.org/pdf/0906.4618) · [Analog Physical-Layer Relay Attacks](https://arxiv.org/pdf/2202.06554)
- [World ID — ca. 18 Mio. verifizierte Menschen in 160 Ländern (2026)](https://world.org/blog/announcements/the-new-world-id-and-the-partners-bringing-proof-of-human-to-the-internet) · [Biometric Update zur Neuausrichtung](https://www.biometricupdate.com/202606/world-shifts-from-crypto-identity-experiment-to-enterprise-proof-of-humanity)
- [Patent US9665784B2 — Spoof-Erkennung über kreuzvalidierte kardiale Signale (rPPG gegen Ballistokardiogramm)](https://patents.google.com/patent/US9665784B2/en)
- [Android: Concurrent Camera Streaming](https://source.android.com/docs/core/camera/concurrent-streaming) · [Apple: AVCaptureMultiCamSession](https://developer.apple.com/documentation/avfoundation/avcapturemulticamsession)
