# AEQUITAS WHITEPAPER v2.0

**Proof of Humanity Chain — Eine faire Währung für alle Menschen**
**Proof of Humanity Chain — A Fair Currency for All of Humanity**

*Version 2.0 · Stand / as of 25.09.2026*
*Chain ID 1926 · aequitas.digital*

---

## Inhalt / Table of Contents

1. [Das Problem / The Problem](#1-das-problem--the-problem)
2. [Die Vision / The Vision](#2-die-vision--the-vision)
3. [Proof of Humanity](#3-proof-of-humanity)
4. [Tokenomics & Wirtschaftsmodell / Economic Model](#4-tokenomics--wirtschaftsmodell--economic-model)
5. [Technische Architektur / Technical Architecture](#5-technische-architektur--technical-architecture)
6. [Smart Contract V7](#6-smart-contract-v7)
7. [Zero-Knowledge-Proofs & Privatsphäre / Privacy](#7-zero-knowledge-proofs--privatsphäre--privacy)
8. [Gleichheitsindex / Equality Index](#8-gleichheitsindex--equality-index)
9. [Exchange & Liquiditätspool / Liquidity Pool](#9-exchange--liquiditätspool--liquidity-pool)
10. [Sicherheit / Security](#10-sicherheit--security)
11. [Roadmap](#11-roadmap)
12. [Fazit / Conclusion](#12-fazit--conclusion)

---

## 1. Das Problem / The Problem

### DE
Bitcoin hat einen Gini-Koeffizienten von über 0,85 — höher als jedes Land der Erde. Die Top 1% der Adressen kontrollieren mehr als 90% aller Bitcoin. Was als dezentrales, demokratisches Geld begann, hat die extremste Vermögenskonzentration der Finanzgeschichte erschaffen.

Das ist kein Versagen der Blockchain-Technologie. Es ist ein Versagen des Designs: Bitcoin wurde ohne Rücksicht auf die Frage entworfen, wer Zugang zu initialem Kapital hat. Wer früh dabei war oder über Rechenleistung verfügte, gewann. Wer später kam oder arm war, verlor.

Das gleiche Muster wiederholt sich bei allen PoW- und PoS-Kryptowährungen: Die Reichen werden reicher, weil sie mehr Kapital einsetzen können. Das ist kein Bug — es ist die Systemarchitektur.

### EN
Bitcoin has a Gini coefficient exceeding 0.85 — higher than any country on Earth. The top 1% of addresses control more than 90% of all Bitcoin. What started as decentralized, democratic money created the most extreme wealth concentration in financial history.

This is not a failure of blockchain technology. It is a failure of design: Bitcoin was built without considering who has access to initial capital. Those who were early or had computing power won. Those who came later or were poor lost.

The same pattern repeats across all PoW and PoS cryptocurrencies: the rich get richer because they can deploy more capital. This is not a bug — it is the system architecture.

---

## 2. Die Vision / The Vision

### DE
Aequitas stellt eine radikale Frage: **Was wäre eine Kryptowährung, wenn sie von Grund auf fair für jeden Menschen auf der Erde konzipiert worden wäre?**

Die Antwort ist überraschend einfach:

> *Geld existiert, weil Menschen existieren. Daher sollte jede Person einen gleichen Anteil am Geld haben — allein weil sie ein Mensch ist.*

Aequitas setzt dieses Prinzip mathematisch um:

```
Gesamtangebot = Verifizierte Menschen × 1.000 AEQ
```

Kein Pre-Mine. Keine Gründer-Zuteilung. Kein früher Vorteil. Wer sich heute registriert und wer sich in zehn Jahren registriert, erhält exakt dasselbe. Es ist kein politisches Versprechen — es ist Code.

### EN
Aequitas asks a radical question: **What would a cryptocurrency look like if designed from first principles to be fair to every human being on Earth?**

The answer is surprisingly simple:

> *Money exists because people exist. Therefore, every person should have an equal share of money simply by virtue of being human.*

Aequitas implements this principle mathematically:

```
Total Supply = Verified Humans × 1,000 AEQ
```

No pre-mine. No founder allocation. No early-adopter advantage. Someone registering today and someone registering in ten years receive exactly the same. This is not a political promise — it is code.

---

## 3. Proof of Humanity

### DE
Das zentrale Problem eines auf menschlicher Existenz basierenden Währungssystems ist die Verifikation: Wie beweist man, dass eine Adresse einem echten, einzigartigen Menschen gehört — ohne Namen, ohne Ausweis und ohne ein Bild des Menschen aufzubewahren?

Aequitas setzt dafür heute auf eine **Live-Gesichtsprüfung** und **Zero-Knowledge-Proofs**, langfristig auf den **Iris-Scan** (§3.1).

**Registrierungsablauf (Stand 25.09.2026):**
1. Die Android-App nimmt ein Foto des Gesichts und eine kurze Bildfolge auf: einmal blinzeln, auf zufällige Aufforderung kurz nach links oder rechts schauen, dazu leuchtet der Bildschirm in einer zufälligen Farbfolge. Nur das Gesicht wird erfasst — kein Fingerabdruck, keine Hand, kein Ausweis.
2. Die Aufnahme geht an **zwei unabhängige Vergleichsdienste** auf verschiedenen Maschinen, die verschiedenen Personen gehören. Sie berechnen kurzzeitig eine Gesichtsbeschreibung und vergleichen sie mit allen bisherigen Anmeldungen. **Beide** müssen übereinstimmen (Quorum 2 von 2); ist einer nicht erreichbar, wird nicht entschieden.
3. Gespeichert wird nur ein **64-Byte-Auszug** (512 Ja/Nein-Werte). Foto und vollständige Gesichtsbeschreibung werden nach wenigen Sekunden verworfen.
4. Ist der Mensch neu, stellt der Coordinator eine zufällige Kennung (`bio_hash`) aus — keine Funktion des Gesichts — und unterschreibt die Bindung an die Wallet; die Vergleichsdienste unterschreiben, dass hinter dieser Kennung ein neuer Mensch steht.
5. Der Proof-Server erzeugt daraus einen **Groth16-Zero-Knowledge-Proof** (BN128) mit `commitment` und `nullifier` — aber nur, wenn die Wallet-Bindung **und** zwei verschiedene Bescheinigungen vorliegen (seit 25./26.08.2026).
6. Die Kette prüft den Beweis und lehnt jeden bereits benutzten Nullifier ab.
7. Bei Erfolg: 1.000 AEQ werden der Wallet gutgeschrieben.

**Garantien — und ihre Grenzen:**
- Derselbe Nullifier zählt nie zweimal. Das ist Kryptografie und lückenlos.
- Dass zwei Aufnahmen desselben Menschen als derselbe erkannt werden, entscheidet der Gesichtsvergleich. Seine Schwelle ist noch nicht an eigenen Aufnahmen kalibriert (dafür braucht es rund 1.000 Impostor-Paare).
- Namen, Ausweisdaten, Anschrift, E-Mail oder Telefonnummer werden nie erhoben.
- Der Eintrag auf der Kette ist öffentlich und dauerhaft.

### EN
The central problem of a monetary system based on human existence is verification: how do you prove that an address belongs to a real, unique human — without names, without ID documents and without keeping a picture of the person?

Today Aequitas relies on a **live face check** and **zero-knowledge proofs**; in the long run on the **iris scan** (§3.1).

**Registration flow (as of 2026-09-25):**
1. The Android app captures a photo of the face and a short sequence: blink once, glance left or right on a random prompt, while the screen flashes a random colour sequence. Only the face is captured — no fingerprint, no hand, no ID document.
2. The capture goes to **two independent matching services** on different machines owned by different people. They briefly compute a face description and compare it with every earlier enrolment. **Both** must agree (quorum 2 of 2); if one is unreachable, nothing is decided.
3. Only a **64-byte sketch** (512 yes/no values) is stored. The photo and the full face description are discarded within seconds.
4. If the person is new, the coordinator issues a random identifier (`bio_hash`) — not a function of the face — and signs its binding to the wallet; the matching services sign that a new human stands behind that identifier.
5. The proof server turns this into a **Groth16 zero-knowledge proof** (BN128) with `commitment` and `nullifier` — but only if the wallet binding **and** two distinct attestations are present (since 2026-08-25/26).
6. The chain verifies the proof and rejects any nullifier it has already seen.
7. On success: 1,000 AEQ are credited to the wallet.

**Guarantees — and their limits:**
- The same nullifier never counts twice. That is cryptography and airtight.
- Whether two captures of the same person are recognised as the same person is decided by the face comparison. Its threshold is not yet calibrated against our own captures (that needs about 1,000 impostor pairs).
- Names, ID data, postal address, e-mail or phone number are never collected.
- The on-chain record is public and permanent.

---

### 3.1 Langfristig: Iris / Long term: iris

#### DE
**Langfristig setzt Aequitas auf den Iris-Scan.** Nur ein Merkmal, das auch unter Milliarden Menschen unverwechselbar bleibt — auch bei eineiigen Zwillingen —, kann wirklich 1 Mensch = 1 Registrierung gewährleisten. Das Gesicht kann das in diesem Maßstab nicht.

Wie das umgesetzt werden kann — zuverlässig, datenschutzfreundlich, mit gemessenen Fehlerraten und bezahlbarer Hardware —, daran wird derzeit gearbeitet. **Hardware und Zeitplan stehen noch nicht fest.** Die Grundsätze stehen: Auch bei der Iris soll kein Bild aufbewahrt werden und kein einzelner Dienst das vollständige Merkmal halten. Genau das erprobt die Beta heute mit dem Gesicht (§3.2).

Die theoretisch sehr kleine Verwechslungsrate der Iris ist eine Eigenschaft des Merkmals, keine gemessene Eigenschaft dieses Systems. Die reale Rate im Maßstab von Milliarden hängt an Aufnahmequalität und Schwelle und muss erst gemessen werden.

*Frühere Entwürfe mit Fingerabdruck-Scanner und Handvenen-Kamera sind verworfen und werden nicht weiterverfolgt. Handfläche, Fingerkuppe, Ohr und ein akustischer Test waren bis zum 23.08.2026 Teil der App und sind entfernt.*

#### EN
**In the long run Aequitas will rely on the iris scan.** Only a feature that stays distinctive among billions of people — identical twins included — can truly guarantee one person = one registration. The face cannot do that at this scale.

How it can be implemented — reliably, privacy-preserving, with measured error rates and affordable hardware — is being worked on now. **Hardware and timing are not decided yet.** The principles are: with the iris too, no image is kept and no single service holds the whole feature. That is exactly what the beta tests today with the face (§3.2).

The iris's theoretically tiny false-match rate is a property of the feature, not a measured property of this system. The real rate at the scale of billions depends on capture quality and threshold and still has to be measured.

*Earlier designs with a fingerprint scanner and a hand-vein camera have been dropped and are not pursued. Palm, fingertip, ear and an acoustic test were part of the app until 2026-08-23 and have been removed.*

---

### 3.2 Die Beta heute / The beta today

#### DE
Stand **25.09.2026**, abgelesen aus dem laufenden Betrieb. Dieser Abschnitt hat Vorrang vor allem anderen, wenn sich etwas widerspricht.

**Wozu die Beta dient.** Sie erprobt, ob ein Mensch wiedererkannt werden kann, ohne dass jemand sein Bild behält und ohne dass irgendwo das vollständige Gesichts-Template liegt. Das ist die Grundlage für den Iris-Scan.

| | Stand |
|---|---|
| Erfasst | nur das Gesicht: Foto + kurze Bildfolge (Blinzeln, Blick zur Seite, Farbblitze) |
| Vergleichsdienste | zwei, auf zwei Contabo-Servern verschiedener Eigentümer; beide müssen zustimmen. Ein dritter Dienst bei Railway ist seit 14.09.2026 abgeschaltet. |
| Bescheinigung | Wallet-Bindung des Coordinators **und** 2 verschiedene Personen-Bescheinigungen, auf beiden Proof-Servern verlangt (seit 26.08.2026) |
| Gespeichert | nur ein 64-Byte-Auszug je Mensch; Foto und Template werden nach Sekunden verworfen (seit 24.08.2026). Aus dem Auszug lässt sich das Gesicht nicht rekonstruieren, er erkennt aber wieder — er gilt deshalb als biometrisches Datum nach Art. 9 DSGVO. |
| Lebendigkeit | verbindlich ist die Blick-Aufgabe. Blitzfarben, Parallaxe, Puls und Antispoof werden gemessen und zu einer Klasse zusammengefasst, entscheiden aber noch nichts (Schattenmodus, seit 13.09.2026) |
| Gestaffelter Zuschuss | gebaut (Klasse signiert, Proof-Server prüft sie), aber abgeschaltet, bis die Schwellen gemessen sind |
| Wiederholte Versuche | verzögert, nicht gesperrt — höchstens zwei Minuten |
| Abgewiesen? | die App zeigt eine Kennung `W-…`; damit kann man innerhalb von 90 Tagen widersprechen, und ein Mensch prüft den Fall (seit App 1.7.4) |
| Konten von vor dem 25.08.2026 | haben kein Gesicht in der Duplikatprüfung. In der App kann jedes solche Konto sein Gesicht nachziehen — ohne neuen Zuschuss. Bis es das tut, könnte sich derselbe Mensch mit einer neuen Wallet noch einmal registrieren. |
| Widerruf | löscht Wallet-Verweis, Einwilligung und alle Merkmale bis auf den 64-Byte-Auszug. Der bleibt ohne Verbindung zu Wallet oder Kennung, sonst ließe sich der Zuschuss durch Löschen und Neuanmelden beliebig oft beziehen. |

**Was noch offen ist:**
- Jeder der beiden Dienste hält heute den vollständigen 64-Byte-Auszug. Der Modus, in dem auch der Auszug geteilt wird und kein Dienst ihn ganz hat (MPC), ist gebaut und getestet, aber nicht maßgeblich: Seine Kandidatensuche findet genau die Wiederkehrer nicht zuverlässig, die sie finden soll, und seine Schwelle ist nicht kalibriert.
- Die Fehlerraten des Gesichtsvergleichs sind nicht gemessen. Erste Messung am echten Gerät (23.08.2026): dieselbe Person wurde mit und ohne Brille als Duplikat erkannt (Ähnlichkeit 0,846 bzw. 0,677 bei Schwelle 0,40) — ein Datenpunkt, keine Rate.
- Ein Live-Deepfake kann die Blick-Aufgabe mitmachen; die stärkeren Signale entscheiden erst nach der Kalibrierung.

#### EN
State as of **2026-09-25**, read from the running system. Where anything else contradicts this section, this section is correct.

**What the beta is for.** It tests whether a person can be recognised again without anyone keeping their picture and without a whole face template being stored anywhere. That is the groundwork for the iris scan.

| | State |
|---|---|
| Captured | the face only: photo + short sequence (blink, glance aside, colour flashes) |
| Matching services | two, on two Contabo servers with different owners; both must agree. A third service on Railway has been off since 2026-09-14. |
| Attestation | coordinator's wallet binding **and** 2 distinct personhood attestations, required on both proof servers (since 2026-08-26) |
| Stored | only a 64-byte sketch per person; photo and template are discarded within seconds (since 2026-08-24). The face cannot be reconstructed from the sketch, but it does recognise — so it counts as biometric data under GDPR Art. 9. |
| Liveness | the glance task is binding. Flash colours, parallax, pulse and anti-spoof are measured and combined into a class, but decide nothing yet (shadow mode, since 2026-09-13) |
| Staged grant | built (class is signed, the proof server checks it) but switched off until the thresholds are measured |
| Repeated attempts | delayed, not banned — two minutes at most |
| Rejected? | the app shows an identifier `W-…`; with it you can object within 90 days and a human reviews the case (since app 1.7.4) |
| Accounts from before 2026-08-25 | have no face in the duplicate check. In the app, every such account can add its face afterwards — no new grant. Until it does, the same person could register again with a new wallet. |
| Withdrawal | deletes the wallet link, the consent record and every feature except the 64-byte sketch. That stays, linked to no wallet and no identifier; otherwise delete-and-re-register would pay the grant again and again. |

**What is still open:**
- Each of the two services holds the whole 64-byte sketch today. The mode in which the sketch itself is split so that no service holds it whole (MPC) is built and tested but not authoritative: its candidate search does not reliably surface the very returning people it must find, and its threshold is not calibrated.
- The face comparison's error rates are not measured. First measurement on a real device (2026-08-23): the same person was detected as a duplicate with and without glasses (similarity 0.846 and 0.677 against a threshold of 0.40) — one data point, not a rate.
- A live deepfake can follow the glance task; the stronger signals only decide after calibration.

#### Background: how the sketch replaced the template (EN)

**The MPC path did not work at first, and finding out why took three separate bugs (2026-08-24).** It is recorded here rather than quietly fixed, because for months it reported "not a duplicate" and looked healthy doing it.

1. *The client asked the parties one after another.* `/mpc/check` runs an interactive two-party protocol, so party 0 blocked waiting for a peer the client had not asked yet — a guaranteed deadlock, ending in a silent three-minute timeout. It looked fine only because the handler answers `duplicate: false` immediately when the bucket lookup finds no candidates, skipping the protocol entirely. Every shadow check that ever completed took that path: 24 of 24 answered "not a duplicate" in about 600 ms, including for a capture the plaintext comparison scored at 1.0 — an identical face.
2. *The Beaver-triple counters drifted apart.* Each party advances its own counter before handing triples out, and every deadlocked attempt burned party 0's supply and none of party 1's. Measured: 10240 against 4096. From there every comparison used non-corresponding triples and produced a value that was neither 0 nor 1. A first repair — ask the peers, take the maximum — was a race rather than a fix, and a 2048 gap survived it intact. One allocator now hands out the range and both parties use it.
3. *The candidate pre-filter cannot see the matches the threshold is meant to accept.* This is the one that still stands. Bucket lookup uses 20 tables of 27 bits. The probability it surfaces a genuine returning person is `1 - (1 - (1 - d/512)^27)^20` for a sketch distance `d` — which is 100% for an identical capture, 62% at `d = 55`, **0.5% at `d = 135`** (the measured same-person-with-glasses pair) and **0.05% at the match threshold of 165**. The secure comparison is correct and now demonstrably runs; it is simply never handed the person it should reject.

Raising that recall is not a matter of turning a dial. Loosening the filter multiplies the candidates, each candidate costs `2 × 2 × 512 = 2048` triples, and the dealer delivered 2,000,000 per party — about 976 comparisons at one candidate each. Recall and capacity pull against each other, and the triple budget is the binding constraint.

So MPC is **not** authoritative, and switching it on today would be worse than leaving it off: duplicate detection would fall back to catching only near-identical re-captures, while the plaintext path compares against every enrolment and catches the rest.

**The template is nevertheless gone (2026-08-24).** MPC was one route to "no party holds the whole template"; it was not the only one, and it turned out not to be the shortest. Since this date the matching service stores no ArcFace template at all — encrypted or otherwise. What it stores is a 512-bit sign-LSH sketch, 64 bytes, one bit per random hyperplane. The template exists only for the duration of the request that computed it.

Comparison is unchanged where it matters. Sign-LSH maps a cosine `s` to an expected bit distance of `512·acos(s)/π`, and that inverts: the measured bit distance is converted back into an estimated cosine, so `ModalityScore`, `decide_duplicate` and `FACE_MATCH_THRESHOLD = 0.40` all keep working on exactly the quantity they always did. What changed is *what is stored*, not *how the decision is made*.

The mapping is measured, not assumed — against this implementation's own projections, which are uniform rather than Gaussian, over 1,500 constructed pairs per point: agreement within 0.5 bits from cosine 0.2 to 0.846. The binarisation costs about ±11 bits of spread, roughly ±0.06 in cosine, and against the distances that matter that is ample: at cosine 0.846 and 0.677 — the same person, measured live, without and with glasses — all 400 test pairs still decide "duplicate", and at cosine 0.10 all 400 decide "not a duplicate". Not one pair flips.

512 bits rather than more is the deliberate point of balance: more bits sharpen the comparison and simultaneously make the direction in embedding space easier to pin down. At one bit per dimension the reconstruction stays coarse while the decision stays right.

Verified on the running service after migrating the existing rows: 2 enrolments, **0 holding a template**, 2 holding a sketch of 64 bytes each, and **zero `AEQT1` sealed-template markers** left anywhere in the database file.

This is not anonymisation, and the app does not claim it is. A sketch remains biometric data under GDPR Art. 9 and still recognises people — that is its purpose. It is simply no longer the template.

#### Uniqueness at the scale this project is for (EN)

A universal basic income is not a system for five hundred people, and the design has to be judged at 10⁸–10⁹. At that size the arithmetic decides more than any implementation choice, so it is written out here rather than discovered later.

**Exhaustive comparison is affordable, and that decides everything below it.** The total work of checking every enrolment against every earlier one is n(n−1)/2 comparisons, which at a billion people is 5·10¹⁷ — about 15.9 CPU-years. That number invites the wrong conclusion, and this document drew it once: it is *total* work spread over however long onboarding takes, not the latency of a check.

Measured in Go with `POPCNT`, one core, 512-bit sketches: **12.16 ns per comparison, 82 million comparisons per second.**

| enrolled | one registration, one core |
|---|---|
| 1 million | 0.01 s |
| 100 million | 1.22 s |
| 1 billion | 12.16 s |

And in continuous operation, which is what actually matters:

| target | registrations/s | cores, on average |
|---|---|---|
| 1 billion in 3 years | 10.6 | **64** |
| 1 billion in 5 years | 6.3 | **39** |
| 1 billion in 10 years | 3.2 | **19** |

Nineteen cores to enrol the world over a decade. The comparison is memory-bandwidth bound — a full pass reads 64 GB — and it shards perfectly across machines.

**So there is no index, and therefore no approximation.** Recall stays at 1, and with it the property that matters: a duplicate is always found. This is worth stating plainly because the alternative was seriously considered here. Every approximate structure — LSH, and graph indexes like HNSW — has recall below 1, and any recall below 1 is grindable, since the attacker chooses how many times to try. At 99.9% recall, roughly a thousand attempts buy a second identity, and in a basic-income system a second identity is income forever. Exhaustive comparison removes that trade entirely, and it costs less than the index would have.

For the record, the numbers that would have applied had an index been necessary: sign-LSH separates this system's own measured distances (genuine d ≈ 135, stranger d ≈ 240 of 512 bits) at ρ = 0.484, predicting 22,653 candidates per query at a billion — but reaching 99.9% recall costs hundreds to thousands of hash tables and still leaves 12–61% of strangers as candidates. The binding constraint there is the *biometric* separation, not the search structure. That remains the lever worth pulling for other reasons: better embeddings would tighten the threshold, the margins, and the cost of every secure comparison at once.

**Repeated duplicate attempts are still counted and delayed**, not because uniqueness depends on it — it does not — but because a stream of rejected attempts against one enrolment is worth seeing, and because rate-limiting abuse is cheap. The counter is keyed to the matched enrolment, since wallets, addresses and devices are replaceable and a face is not. The delay is capped at two minutes: an uncapped delay is a ban wearing a different name, and a ban is the irreversible error this project's first principle forbids.

**Storage is not the constraint.** A sketch is 64 bytes, so the whole world fits in 64 GB. What has to be engineered is retrieval, not capacity.

**And this is why MPC cannot be the mandatory path.** The plaintext comparison costs 12 ns; the secure one costs 1024 Beaver triples, which is roughly eight orders of magnitude more and consumes a finite pre-shared supply. A billion registrations against a growing population would need correlated randomness measured in exabytes. That is not a failure of the MPC work — the property it was built for, that no party holds a whole template, is delivered by the sketch instead, and delivered today. MPC keeps its value as a cross-check on a sample of registrations, where its cost is a matter of choice rather than of population.



Since 2026-08-24 the plaintext comparison works on sketches, not templates: the matching service compares against every enrolled 64-byte sketch and holds no template at all. The mode in which the committee decides on split shares, so that no service holds even the whole sketch, is built and tested but **not authoritative** — for the recall reason above, and because its threshold has never been calibrated against real captures. The geometry behind that threshold was checked on 2026-08-24 and holds: sign-LSH turns a cosine `s` into `512·acos(s)/π` bits, and measured against this implementation's own projections over 1,500 constructed pairs per point, the agreement is within 0.5 bits across the whole range. What remains unmeasured is the other half — what cosine two *different* people actually produce. That still needs roughly 1,000 impostor pairs, and no synthetic test substitutes for it.

**Withdrawal of consent, and why it cannot mean total erasure.** Since 2026-08-24 the app carries the erasure path itself (GDPR Art. 17): it keeps the registrant's `bio_hash` in hardware-backed storage and hands it to `DELETE /enrollment`, which fans out to every matching service. The wallet link, the consent record and every descriptor except one are cleared, and the row is stamped `withdrawn_at`.

One field survives on purpose: the 64-byte face sketch. **Erasing a biometric and still recognising its owner are mutually exclusive** — a token that can answer "is this the same person" *is* the template. Removing it outright, which is what the code did until that date, would have opened an unbounded money printer: the chain pays a 1,000 AEQ registration grant per unseen nullifier, the nullifier derives from the `bio_hash`, and the `bio_hash` is `secrets.randbelow` — a fresh random value, not a function of the face. So *register → spend → delete → register again* would have minted a new identity and paid the grant a second time, repeatably, at no cost. Neither the chain nor any wallet-side rule can catch that: the chain only ever sees an unused nullifier and does exactly what it should, and a fresh wallet defeats wallet-side checks.

What is retained is therefore the minimum that closes the loop, and nothing more: a 64-byte sketch bound to no wallet and no identity, whose only answerable question is "has this person enrolled before". The app says this in plain words before asking for confirmation, in all twelve languages, rather than promising an erasure it cannot perform. Withdrawing returns a person's data; it does not return their eligibility for a second grant, and their on-chain registration stands either way — the ledger is immutable.

If the MPC path ever becomes authoritative this gets strictly better: in that mode not even a whole sketch is stored anywhere, uniqueness rides on shares no single party can reconstruct, and the local row could be dropped outright. That is the reason the MPC path exists. It is not authoritative today because of the recall problem above and because its threshold has never been calibrated.

**What actually carries uniqueness today:** the on-chain nullifier. It is cryptographic and airtight — the same nullifier is refused the second time, whatever path submits it. But it only proves that *the same identity source* cannot count twice. That the source is a human is carried by the face match — with a threshold that is not yet calibrated.

---

## 4. Tokenomics & Wirtschaftsmodell / Economic Model

### 4.1 Geldangebot / Money Supply

Das Angebot ist eine mathematische Funktion menschlicher Existenz. Es gibt genau **eine** Möglichkeit neues AEQ zu erschaffen: ein neuer verifizierter Mensch registriert sich.

The supply is a mathematical function of human existence. There is exactly **one** way to create new AEQ: a new verified human registers.

| Ereignis / Event | AEQ-Änderung / AEQ Change |
|-----------------|--------------------------|
| Neue Registrierung / New Registration | +1.000 AEQ |
| Transfer | ±0 (nur Umverteilung / redistribution only) |
| Swap | ±0 |
| Demurrage | −x von Überschuss / from excess → UBI Pool |
| Wealth Cap Overflow | −x → sofortige Gleichverteilung / instant redistribution |

### 4.2 Universal Basic Income (UBI)

UBI wird täglich aus den Protokoll-Einnahmen verteilt — kein Staat, keine Steuer, kein Beschluss erforderlich.

UBI is distributed daily from protocol revenue — no state, no tax, no vote required.

**UBI-Pool-Quellen / UBI Pool Sources:**
- 100% der Überweisungsgebühren (0,1 % auf jede Überweisung, vom Absender obendrauf gezahlt; Aufschlag ab dem 5-, 10- und 20-fachen des fairen Anteils von 1.000 AEQ: +0,1 %, +0,5 %, +1 %)
- 30% der Swap-Gebühren
- 100% der Wealth-Cap-Überläufe
- 100% der Demurrage auf Überschussguthaben (0,5%/Monat nach 3 Monaten Inaktivität über fairShare). Bis zum 24.09.2026 gingen davon nur 20% ans Grundeinkommen und 80% an Validatoren, Liquiditätsgeber und Treasury — Geld, das Hortenden genommen wird, gehört aber allen Menschen zu gleichen Teilen.
- Inaktive Wallets: nach 2,5 Jahren Inaktivität → Escrow, nach weiteren 1,5 Jahren → UBI Pool

### 4.3 Demurrage — Haltegebühr

**Philosophie:** Geld ist ein Werkzeug, kein Selbstzweck. Horten von Geld über dem fairen Anteil kostet etwas — genau wie das Mieten eines Parkplatzes.

**Philosophy:** Money is a tool, not an end in itself. Hoarding money above the fair share costs something — just like renting a parking space.

```
Haltegebühr = (Guthaben − fairShare) × 0,5%/Monat × Halte-Monate (erst nach 3 Monaten Inaktivität)
Demurrage   = (Balance − fairShare) × 0.5%/month × HoldingMonths (only after a 3-month inactivity grace period)
```

Die Gebühr geht zu 100% in den UBI-Pool. Kein AEQ wird vernichtet.
The fee goes 100% to the UBI pool. No AEQ is destroyed.

### 4.4 Wealth Cap — Vermögensobergrenze

### DE
Die Wealth Cap verwendet einen Bootstrap-Multiplikator in Phase 0: `max(5, min(N, 25)) × Fair Share (1.000 AEQ)`. Kein Admin-Key, kein Governance-Vote — alle Übergänge erfolgen automatisch durch die Anzahl registrierter Menschen.

**Formel Phase 0:** `cap = max(5, min(N, 25)) × 1.000 AEQ`

> **Klarstellung (Audit 2026-08-18).** Frühere Fassungen schrieben hier
> „× Ø-Balance" bzw. „× average balance" und legten damit nahe, die Obergrenze
> wachse mit dem tatsächlich gemessenen Durchschnittsvermögen mit. Sie tut es
> nicht. Der Bezugswert ist der *Fair Share*, und der ist auf dieser Chain per
> Definition konstant: `TotalSupply / Menschen = (Menschen × 1.000) / Menschen
> = 1.000 AEQ`. Die Obergrenze liegt damit dauerhaft bei 25.000 AEQ. Die Regel
> selbst ist unverändert — nur ihre Beschreibung war irreführend.
> / *Clarification: earlier versions wrote "× average balance", implying the cap
> tracks measured average wealth. It does not — the reference is the fair share,
> which is constant by definition (TotalSupply / humans = 1,000 AEQ), so the cap
> is a permanent 25,000 AEQ. The rule is unchanged; only its description was
> misleading.*
- Menschen 1–4: Multiplikator = **5×**
- Jeder neue Mensch: Multiplikator +1×
- Ab dem 25. Menschen: dauerhaft **25×** (unveränderbar, kein Vote erforderlich)

### EN
The wealth cap uses a bootstrap multiplier during Phase 0: `max(5, min(N, 25)) × fair share (1,000 AEQ)`. No admin key, no governance vote — all transitions trigger automatically by human count.

**Phase 0 formula:** `cap = max(5, min(N, 25)) × avg_balance`
- Humans 1–4: multiplier = **5×**
- Each new human: multiplier +1×
- At 25+ humans: permanently **25×** (immutable, no vote required)

---

| Phase | Menschen / Humans | Formel / Formula | Status |
|-------|------------------|-----------------|--------|
| **0** Bootstrap | 1–99 | `max(5, min(N,25)) × Ø` | ● Aktiv / Active |
| **1** Growth | 100–9.999 | `25 × 1.000 AEQ = 25.000 AEQ` | ○ Geplant / Planned |
| **2** Stability | 10.000–999.999 | `25 × 1.000 AEQ = 25.000 AEQ` | ○ Geplant / Planned |
| **3** Maturity | 1.000.000+ | `25 × 1.000 AEQ = 25.000 AEQ` | ○ Geplant / Planned |

**Beispiel / Example (Phase 0, N=10 Menschen, Ø=1.000 AEQ):**
```
cap = max(5, min(10, 25)) × 1.000 = 10 × 1.000 = 10.000 AEQ
```

Überschuss fließt sofort in die Tokenomics-Pools — kein AEQ wird vernichtet.
Excess flows instantly into tokenomics pools — no AEQ is destroyed.

### 4.5 Transaktionsgebühren / Transaction Fees

| Empfänger / Recipient | Anteil / Share |
|----------------------|---------------|
| Validators (Swap) | 40% — gleich je Menschen-Validator, gewichtet nach Minuten online / equal per human validator, weighted by minutes online |
| Liquidity Providers (Swap) | 30% |
| UBI Pool (Swap) | 30% |
| UBI Pool (Überweisungen / transfers) | 100% |
| Treasury | 0% — seit 24.09.2026; aus keinem Topf darf jemand auszahlen / since 24 Sep 2026; nobody may pay out of any pool |

---

## 5. Technische Architektur / Technical Architecture

### 5.1 Layer 1 — Aequitas Chain

Aequitas läuft auf einer eigens entwickelten Layer-1-Blockchain, geschrieben in **Go 1.24**, mit einem hybriden BlockDAG-Konsensus.

Aequitas runs on a custom-built Layer 1 blockchain written in **Go 1.24**, with a hybrid BlockDAG consensus.

**BlockDAG:**
- Mehrere Blöcke können gleichzeitig von verschiedenen Nodes produziert werden
- Blöcke werden später in Merge-Blöcke zusammengeführt (mehrere Eltern)
- Höherer Durchsatz, niedrigere Latenz, bessere Fehlertoleranz
- Multiple blocks can be produced simultaneously by different nodes
- Blocks are merged into merge blocks with multiple parents
- Higher throughput, lower latency, better fault tolerance

**GHOSTDAG (Sompolinsky-Zohar, 2018):**
Damit alle Nodes trotz gleichzeitiger Blockproduktion zur selben Reihenfolge und zum selben Zustand konvergieren, berechnet jeder Node einen deterministischen "Blue Score" für jeden Block (über Selected-Parent-Auswahl und Blue/Red-Klassifizierung im Merge-Set). Daraus ergibt sich eine kanonische Gesamtordnung (Höhe, dann Blue Score, dann Hash), die auf jedem Node identisch ist — unabhängig davon, in welcher Reihenfolge Blöcke per P2P/HTTP eintreffen.

To ensure every node converges on the same order and state despite concurrent block production, each node computes a deterministic "blue score" for every block (via selected-parent selection and blue/red classification within the merge set). This yields a canonical total order (height, then blue score, then hash) that is identical on every node, regardless of the order blocks arrive via P2P/HTTP.

**KNIGHTDAG (adaptives K, inspiriert von DAGKNIGHT, Sompolinsky-Sutton 2022):**
Die Blue/Red-Klassifizierung verwendet kein starres K mehr: Für jeden Block wird deterministisch das kleinste K_eff ≤ K ermittelt, dessen Blue-Menge eine strikte Mehrheit des Merge-Sets abdeckt. Bei guter Netzwerkkonvergenz sinkt K_eff automatisch (engere, schnellere Bestätigung); bei Bursts fällt die Klassifizierung exakt auf das bisherige GHOSTDAG-Verhalten mit dem Epochen-K zurück. Da jeder Node dieselbe Inferenz über denselben Blockgraphen ausführt, bleibt die kanonische Ordnung netzweit identisch. KNIGHTDAG ist ab Blockhöhe **1.520.000** aktiv (`KNIGHTDAG_ACTIVATION_HEIGHT`, auf allen Nodes identisch konfiguriert) — darunter klassifiziert jeder Node bit-genau nach der klassischen GHOSTDAG-Regel, damit ein Node, der historische Blöcke neu ableitet (Resync, Deepscan), exakt denselben BlueScore reproduziert, den das Netzwerk für diese Blöcke bereits vereinbart hat.

**KNIGHTDAG (adaptive K, inspired by DAGKNIGHT, Sompolinsky-Sutton 2022):**
Blue/red classification no longer uses a rigid K: for every block, each node deterministically infers the smallest K_eff ≤ K whose blue set covers a strict majority of the merge set. Under good network convergence K_eff drops automatically (tighter, faster confirmation); under bursts, classification falls back to exactly the previous GHOSTDAG behavior with the epoch K. Since every node runs the same inference over the same block graph, the canonical order remains identical network-wide. KNIGHTDAG is active from block height **1,520,000** onward (`KNIGHTDAG_ACTIVATION_HEIGHT`, configured identically on every node) — below it, every node classifies bit-for-bit with the classic GHOSTDAG rule, so a node re-deriving historical state (resync, deepscan) reproduces exactly the blue_score the network already agreed on for those blocks.

**Dual-Ledger:**
Aequitas führt zwei synchronisierte Ledger parallel:
- **Go-Ledger**: PostgreSQL-gesichert, primäre Wahrheit für Salden und Menschen
- **EVM-Ledger**: go-ethereum Engine, kompatibel mit MetaMask und Web3

Aequitas maintains two synchronized ledgers in parallel:
- **Go-Ledger**: PostgreSQL-backed, primary truth for balances and humans
- **EVM-Ledger**: go-ethereum engine, compatible with MetaMask and Web3

### 5.2 Netzwerk-Topologie / Network Topology

```
Contabo 1 (Lauterbourg, FR)           Contabo 2 (Lauterbourg, FR)
├── Validator-Node + eigene DB        ├── Validator-Node + eigene DB
├── Proof-Server                      ├── Proof-Server
├── Vergleichsdienst (proof1)         ├── Vergleichsdienst (proof2)
└── Coordinator                       └── Coordinator
          └──────── P2P (libp2p) + HTTP-Blocksync ────────┘
```

Zwei Server mit je eigener PostgreSQL-Datenbank, zwei Eigentümer der Vergleichsdienste. Railway ist seit August 2026 für die Kette und seit 14.09.2026 auch für den Vergleichsdienst abgeschaltet. Jeder registrierte Mensch kann einen weiteren Knoten betreiben.

Two servers, each with its own PostgreSQL database; the two matching services have different owners. Railway has been shut down for the chain since August 2026 and for the matching service since 2026-09-14. Any registered human can run a further node.

### 5.3 Technische Kenndaten / Technical Specifications

| Parameter | Wert / Value |
|-----------|-------------|
| Programmiersprache / Language | Go 1.24 |
| Konsens / Consensus | BlockDAG + Proof of Humanity |
| Blockzeit / Block Time | ~1 Sekunde / second |
| Chain ID | 1926 |
| EVM-Kompatibilität / EVM Compat. | Vollständig / Full (go-ethereum) |
| P2P-Protokoll / P2P Protocol | libp2p |
| State-Storage / State Storage | PostgreSQL (persistent) |
| ZKP-System / ZKP System | Groth16 / snarkjs / circom |
| Elliptische Kurve / Elliptic Curve | BN128 (alt-bn128) |
| Bio-Hash | Zufallswert des Coordinators, keine Funktion des Gesichts / random value issued by the coordinator, not a function of the face |
| Dezimalgenauigkeit / Precision | 6 Stellen / decimal places (1 AEQ = 1.000.000 Micro-AEQ) |

---

## 6. Smart Contract V7

### DE
Der AequitasV7-Contract ist das Herzstück des Protokolls. Er ist in Solidity geschrieben, auf der Aequitas Chain deployed und enthält die gesamte Wirtschaftslogik.

**Kernfunktionen:**
- `register()` — Registrierung mit ZKP-Beweis (direkt)
- `registerWithSig()` — Registrierung via Relayer (gaslos für den Nutzer)
- `transfer()` — Token-Transfer mit automatischer Demurrage und Gebühren
- `claimUBI()` — Tägliches UBI einfordern
- `addGuardian()` — Guardian-System für Proof of Alive
- `applyWealthCap()` — Vermögensobergrenze durchsetzen

**Sicherheitsfeatures:**
- Nullifier-Bindung verhindert Doppel-Registrierung
- `registerWithSig` nur von authorisierter Relayer-Adresse aufrufbar
- Optimistic Locking für Multi-Node-Schreibvorgänge
- Vollständiger Storage-Backup vor Contract-Upgrades

### EN
The AequitasV7 contract is the core of the protocol. Written in Solidity, deployed on Aequitas Chain, it contains all economic logic.

**Core Functions:**
- `register()` — Registration with ZKP proof (direct)
- `registerWithSig()` — Registration via relayer (gasless for user)
- `transfer()` — Token transfer with automatic demurrage and fees
- `claimUBI()` — Claim daily UBI
- `addGuardian()` — Guardian system for Proof of Alive
- `applyWealthCap()` — Enforce wealth ceiling

**Security Features:**
- Nullifier binding prevents double registration
- `registerWithSig` callable only from authorized relayer address
- Optimistic locking for multi-node writes
- Full storage backup before contract upgrades

---

## 7. Zero-Knowledge-Proofs & Privatsphäre / Privacy

### DE
Aequitas nutzt Groth16-Proofs auf der BN128-Kurve — eines der effizientesten ZKP-Systeme mit kleinen Proofs (~200 Bytes) und schneller On-Chain-Verifikation (~10ms).

**Nullifier-Bindung:** Der ZKP enthält einen eindeutigen Nullifier (`pubSignals[1]`), der kryptographisch an den biometrischen Hash gebunden ist. **Derselbe Nullifier** kann nie zweimal verwendet werden.

> **Was das nicht heißt.** Eine frühere Fassung dieses Absatzes schrieb, Sybil-Angriffe seien „mathematisch ausgeschlossen". Das ist falsch und wird hier korrigiert. Der Nullifier schließt lückenlos aus, dass *dieselbe Identitätsquelle* zweimal zählt — er sagt nichts darüber, ob zwei Aufnahmen desselben Menschen zum selben `bio_hash` führen. Das entscheidet der Gesichtsabgleich, mit einer Schwelle, die noch nicht gegen eigene Aufnahmen kalibriert ist (§3.2). Die Kryptografie ist hier scharf; die Biometrie darunter ist eine Messung mit einer Fehlerrate, die noch nicht beziffert ist.

**ZK-Commitment der Zielarchitektur (Iris, langfristig):**
```
commitment   = Poseidon(iris_merkmal, wallet_address, salt)   -- Skizze, nicht festgelegt
nullifier    = Poseidon(iris_merkmal)
```

**In dieser Zielarchitektur** wäre der Nullifier ausschließlich an physische Körpermerkmale gebunden — kein Gerät, keine SIM-Karte, kein Betriebssystem; wer sein Telefon verliert, verifiziert sich mit denselben Merkmalen neu, ohne eine zweite Identität zu erzeugen.

**Heute (25.09.2026)** ist das Gesicht die Grundlage: Der Nullifier leitet sich aus der zufälligen Kennung `bio_hash` ab, die erst nach dem Gesichtsvergleich ausgestellt wird. Ein Iris-Merkmal gibt es noch nicht — wie der Iris-Scan umgesetzt wird, daran wird gearbeitet (§3.1). Wer sein Telefon verliert, kommt über eine erneute Gesichtsaufnahme zurück; ob das gelingt, hängt an derselben unkalibrierten Schwelle wie alles andere.

| Phase | Commitment-Faktoren | Nullifier-Faktoren |
|-------|--------------------|--------------------|
| Beta (heute) | bio_hash (nach Gesichtsvergleich) + wallet + deviceSalt | bio_hash |
| Ziel (langfristig) | Iris-Merkmal + wallet | Iris-Merkmal — genaue Konstruktion Teil der laufenden Arbeit |

**Was gespeichert wird:**
- ✅ Auf der Kette: `commitment` (nicht rückführbar auf das Gesicht), `nullifier`, Wallet-Adresse — öffentlich und dauerhaft
- ✅ Bei den zwei Vergleichsdiensten: ein 64-Byte-Auszug des Gesichts, die Kennung `bio_hash`, ein verschlüsselter Verweis auf die Wallet und die Einwilligung (§3.2)
- ❌ Foto oder vollständiges Gesichts-Template — nach Sekunden verworfen
- ❌ Name, Anschrift, Ausweis, E-Mail, Telefonnummer — niemals erhoben
- ❌ IP-Adresse — nur eine Stunde im Arbeitsspeicher gegen Massenanmeldungen, nie in einer Datenbank

### EN
Aequitas uses Groth16 proofs on the BN128 curve — one of the most efficient ZKP systems with small proofs (~200 bytes) and fast on-chain verification (~10ms).

**Nullifier Binding:** The ZKP contains a unique nullifier (`pubSignals[1]`), cryptographically bound to the biometric hash. **The same nullifier** can never be used twice.

> **What that does not mean.** An earlier version of this paragraph said Sybil attacks were "mathematically impossible". That is wrong and is corrected here. The nullifier airtightly prevents *the same identity source* from counting twice — it says nothing about whether two captures of the same human produce the same `bio_hash`. That is decided by the face match, with a threshold not yet calibrated against our own captures (§3.2). The cryptography here is exact; the biometrics underneath it is a measurement with an error rate that has not yet been quantified.

**ZK commitment of the target architecture (iris, long term):**
```
commitment   = Poseidon(iris_feature, wallet_address, salt)   -- sketch, not fixed
nullifier    = Poseidon(iris_feature)
```

**In that target architecture** the nullifier would be bound exclusively to physical body features — no device, no SIM card, no OS; someone who loses their phone re-verifies with the same traits without creating a second identity.

**Today (2026-09-25)** the face is the basis: the nullifier derives from the random identifier `bio_hash`, which is only issued after the face comparison. There is no iris feature yet — how the iris scan will be implemented is being worked on (§3.1). Someone who loses their phone returns through another face capture; whether that succeeds rests on the same uncalibrated threshold as everything else.

| Phase | Commitment factors | Nullifier factors |
|-------|--------------------|-------------------|
| Beta (today) | bio_hash (after face comparison) + wallet + deviceSalt | bio_hash |
| Goal (long term) | iris feature + wallet | iris feature — exact construction part of the ongoing work |

**What is stored:**
- ✅ On chain: `commitment` (not traceable to the face), `nullifier`, wallet address — public and permanent
- ✅ At the two matching services: a 64-byte face sketch, the identifier `bio_hash`, an encrypted reference to the wallet and the consent record (§3.2)
- ❌ Photo or full face template — discarded within seconds
- ❌ Name, postal address, ID document, e-mail, phone number — never collected
- ❌ IP address — held for one hour in memory against mass sign-ups, never in a database

---

## 8. Gleichheitsindex / Equality Index

### DE
Aequitas ist das erste Währungssystem, das seinen eigenen Gleichheitsgrad live und transparent misst und veröffentlicht.

**Gini-Koeffizient:** Misst die Ungleichverteilung des AEQ-Vermögens. 0 = perfekte Gleichheit, 1 = totale Konzentration.

**Lorenz-Kurve:** Zeigt grafisch, wie viel Prozent des Reichtums die ärmsten X% der Menschen besitzen.

**Aequitas-Index:** Kombinierter Score aus Gini, Verteilung, Aktivität und Wachstum.

**Ziel / Target:** Gini < 0,30 (Skandinavien-Niveau)

| Währung / Currency | Gini |
|-------------------|------|
| **Aequitas AEQ** | **~0,08** |
| Skandinavien / Scandinavia | ~0,27 |
| Deutschland / Germany | ~0,31 |
| USA | ~0,41 |
| Brasilien / Brazil | ~0,53 |
| Bitcoin | ~0,85 |

### EN
Aequitas is the first monetary system that measures and publishes its own equality level live and transparently.

**Gini Coefficient:** Measures inequality of AEQ wealth distribution. 0 = perfect equality, 1 = total concentration.

**Lorenz Curve:** Graphically shows what percentage of wealth the poorest X% of humans own.

**Aequitas Index:** Combined score from Gini, distribution, activity, and growth.

**Target:** Gini < 0.30 (Scandinavia level)

---

## 9. Exchange & Liquiditätspool / Liquidity Pool

### DE
Aequitas enthält einen integrierten automatischen Market Maker (AMM) für den Handel zwischen AEQ und tUSD (einem simulierten Test-Dollar auf der Aequitas Chain).

**Mechanismus:** Das klassische `x·y=k`-Modell — der Pool hält automatisch einen Gleichgewichtspreis aufrecht.

**Gebührenverteilung / Fee Distribution:**
- 0,1% Swap-Gebühr wird automatisch aufgeteilt:
  - 40% → Validator-Pool (Netzwerkanreiz)
  - 30% → Liquidity Provider (LP-Rendite)
  - 30% → UBI-Pool (Grundeinkommen)
  - 0% → Treasury (seit 24.09.2026 — ihr Anteil geht ans Grundeinkommen)

**Liquiditäts-Shares:** LPs erhalten proportionale Shares und können jederzeit ihre Anteile plus akkumulierte Gebühren abheben.

### EN
Aequitas contains a built-in Automated Market Maker (AMM) for trading between AEQ and tUSD (a simulated test-dollar on Aequitas Chain).

**Mechanism:** The classic `x·y=k` model — the pool automatically maintains an equilibrium price.

**Fee Distribution:**
- 0.1% swap fee automatically split:
  - 40% → Validator Pool (network incentive)
  - 30% → Liquidity Providers (LP yield)
  - 30% → UBI Pool (basic income)
  - 0% → Treasury (since 24 Sep 2026 — its share goes to basic income)

**Liquidity Shares:** LPs receive proportional shares and can withdraw their stakes plus accumulated fees at any time.

---

## 10. Sicherheit / Security

### Auditierte Sicherheitsmaßnahmen / Audited Security Measures

| Bedrohung / Threat | Schutz / Protection |
|-------------------|---------------------|
| Doppel-Registrierung / Double registration | Nullifier-Bindung on-chain / Nullifier binding on-chain |
| Replay-Attacke / Replay attack | Nonce-System mit CAS / Nonce system with compare-and-swap |
| Sybil-Attacke / Sybil attack | Live-Gesichtsprüfung durch 2 unabhängige Dienste + 2 Bescheinigungen + ZKP-Nullifier (Grenzen: §3.2) / live face check by 2 independent services + 2 attestations + ZKP nullifier (limits: §3.2) |
| Pool-Drain | Wealth Cap + Demurrage + optimistic locking |
| Contract-Upgrade-Risiko | Vollständiger Storage-Backup vor Wipe / Full storage backup before wipe |
| Multi-Node-Konflikte / Multi-node conflicts | PostgreSQL optimistic locking + SELECT FOR UPDATE |
| Öffentliche Contract-Deployments / Public deployments | Deployment auf Relayer-Adresse beschränkt / Restricted to relayer |
| Private Keys in Logs | Ausgabe nur auf stderr (nicht in Log-Aggregatoren) / stderr only |
| XSS | HTML-Escaping aller User-Eingaben / HTML escaping of all user inputs |
| DNS-Rebinding | Peer-URL-Validierung + Öffentliche-IP-Prüfung / Public IP check |

### Dezentralisierung / Decentralization

Aequitas ist in der Beta und in Protokollphase 0 (unter 100 Menschen) mit zwei Validatoren. Das Protokoll ist für beliebig viele Knoten ausgelegt; jeder registrierte Mensch kann einen Knoten betreiben, ohne Antrag und ohne Einsatz. Die Protokollphase ergibt sich automatisch aus der Zahl der Menschen (100 / 10.000 / 1 Mio.); Phase 3 verlangt zusätzlich einen Gini unter 0,30. Mindestzahlen an Knoten sind Ziele, der Code setzt sie noch nicht durch.

Aequitas is in beta and in protocol phase 0 (fewer than 100 humans) with two validators. The protocol is designed for any number of nodes; any registered human can run one, with no application and no stake. The protocol phase follows automatically from the number of humans (100 / 10,000 / 1M); phase 3 additionally requires a Gini below 0.30. Minimum node counts are goals; the code does not enforce them yet.

---

## 11. Roadmap

*Die Zeilen sind Arbeitsstände, keine Protokollphasen. Die Protokollphasen 0–3 stehen in §10. / The rows are milestones, not protocol phases; protocol phases 0–3 are in §10.*

| Stand / Status | DE | EN |
|-------|----|----|
| ✅ | Smart Contracts · ZKP · Android-App · Proof-Server | Smart Contracts · ZKP · Android app · Proof server |
| ✅ | Aequitas Layer 1 · BlockDAG + GHOSTDAG/KNIGHTDAG · P2P · Explorer | Aequitas Layer 1 · BlockDAG + GHOSTDAG/KNIGHTDAG · P2P · Explorer |
| ✅ | EVM · Dual-Ledger · Umtausch/AMM · Grundeinkommen · Vermögensobergrenze · Lorenz/Gini | EVM · dual ledger · exchange/AMM · basic income · wealth cap · Lorenz/Gini |
| ✅ | Proof of Alive · Guardian-System (Treuhand + UBI-Freigabe) | Proof of Alive · guardian system (escrow + UBI release) |
| ✅ | Live-Gesichtsprüfung mit 2 unabhängigen Diensten und 2 Bescheinigungen · nur 64-Byte-Auszug gespeichert · Widerspruch mit menschlicher Prüfung | Live face check with 2 independent services and 2 attestations · only a 64-byte sketch stored · objection with human review |
| 1.10.2026 | Wirtschaftsregeln aktiv: Liegegeld, Freibetrag nach Umsatz, Ausstiegsabgabe | Economy rules active: idle-money levy, turnover-based allowance, exit levy |
| 🔄 Beta | Fehlerraten messen (~1.000 Impostor-Paare) · Lebendigkeit verbindlich machen · gestaffelter Zuschuss · Auszug aufteilen (kein Dienst hält ihn ganz) · mehr Knotenbetreiber | Measure error rates (~1,000 impostor pairs) · make liveness binding · staged grant · split the sketch (no service holds it whole) · more node operators |
| 🔄 | Rechtliche Prüfung (MiCA, DSGVO) · echte Stablecoin statt Test-Währung tUSD | Legal review (MiCA, GDPR) · a real stablecoin instead of the test currency tUSD |
| 🔄 Iris | Iris-Scan, damit wirklich 1 Mensch = 1 Registrierung gilt — Umsetzung in Arbeit, Hardware und Datum offen | Iris scan so that one person = one registration truly holds — implementation in progress, hardware and date open |
| ⬜ | iOS-App | iOS app |
| ⬜ | Cross-Chain-Brücken · externe DEX-Anbindung | Cross-chain bridges · external DEX integration |
| ⬜ | Vollständige Dezentralisierung · Community-Governance | Full decentralisation · community governance |

---

## 12. Fazit / Conclusion

### DE
Aequitas ist kein weiteres Experiment in Kryptospekulation. Es ist ein ernsthafter Versuch, Geld neu zu denken — von Grund auf, für alle Menschen, fair.

Die mathematische Garantie ist simpel und radikal zugleich: Solange Menschen existieren, existiert AEQ. Kein Zentralstaat, keine Bank, kein Algorithmus kann das Grundeinkommen entziehen oder die Gleichheit untergraben — es ist Code.

Der Gini-Koeffizient von Aequitas wird live gemessen und im Explorer angezeigt (Gleichheit). Bitcoin liegt bei ~0,85. Jeder Mensch startet mit genau demselben Anteil — das ist kein Zufall, sondern das Design.

### EN
Aequitas is not another experiment in crypto speculation. It is a serious attempt to rethink money — from first principles, for all people, fairly.

The mathematical guarantee is simple and radical at once: as long as humans exist, AEQ exists. No central state, no bank, no algorithm can remove the basic income or undermine the equality — it is code.

Aequitas's Gini coefficient is measured live and shown in the explorer (Equality). Bitcoin's is ~0.85. Every person starts with exactly the same share — not by coincidence, but by design.

---

*Aequitas · Chain ID 1926 · aequitas.digital*
*Version 2.0 · Stand / as of 25.09.2026*
*Lizenz / License: MIT · Open Source: github.com/hanoi96international-gif/Aequitas*
