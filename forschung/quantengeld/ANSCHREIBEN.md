# Anschreiben an Forschende (Vorlage)

Verschickt **nur der Betreiber**, von der eigenen Adresse. Vor dem Senden:
- `<Name>` und `<Arbeit>` ausfüllen,
- die genannte Arbeit selbst gelesen haben,
- nichts versprechen, was nicht beschlossen ist (Geld, Stellen, Zeitpläne).

Forschende antworten eher auf eine kurze, konkrete Frage als auf eine Vision.

---

**Betreff:** Question on quantum lightning for a proof-of-personhood ledger

Dear Prof. `<Name>`,

I run Aequitas, an open-source blockchain where each verified person receives
the same one-time allocation (1,000 AEQ). Uniqueness of persons comes from
biometric deduplication, not from physics. We are making the classical side
post-quantum (ML-DSA, ML-KEM).

We are studying whether the ledger could later act as the issuer for quantum
banknotes, following `<Arbeit>` (e.g. Coladangelo & Sattath, Quantum 2020).
Small exact simulations, a protocol model of lock → offline transfer →
bolt-to-signature → redeem, and the open questions are public:
https://github.com/hanoi96international-gif/Aequitas/tree/main/forschung/quantengeld

One result from our simulations: for a group-action note verified by a single
projection on an m-qubit Fourier state, an honest note survives storage with
exactly (1 − p)^m under independent depolarizing noise (p = per-qubit flip
probability, any serial number; checked against all 4^m Pauli errors). Without
error correction, a note held for one day would need coherence times of
decades, while Wiesner-style private verification needs only 2.5–10× the
storage time (rauschen.py, rauschen_oeffentlich.py).

Our concrete question: which quantum-lightning candidate would you consider
the most credible basis today, and is anyone working on noise-tolerant
public verification beyond the oracle model (e.g. arXiv:2407.06463)?

We are not asking for a commitment. A short pointer would already help a lot.

Best regards,
`<Name des Betreibers>`

---

**Deutsche Fassung** (für deutschsprachige Gruppen) sinngemäß gleich. Kurz
halten und die eine konkrete Frage an den Schluss stellen.

---

## Vorlage 2: an den BMFTR-Verbund „Quanten-Token“ (deutsch)

Stand 16.11.2026. Hintergrund in `ANALYSE.md`, Abschnitt 6. Adressaten sind
die Autorinnen und Autoren von *Practical quantum tokens: challenges and
perspectives* (arXiv 2602.10621), z. B. die Koordination eines der Projekte
QuaMToMe, HybridQToken, Q.TOK, QPIS, DIQTOK, NEQSIS oder Q-ToRX. Für die
Kryptografie: der Abschnitt „Quantum tokens in the post-quantum world“.

Vor dem Senden: den Artikel selbst gelesen haben, `<Name>` ausfüllen, nichts
zusagen, was nicht beschlossen ist.

---

**Betreff:** Quanten-Token auf einer laufenden Kette – Anwendungsfall für Ihre Speicher?

Sehr geehrte/r `<Name>`,

ich betreibe Aequitas, eine quelloffene Blockchain mit „ein Mensch, ein
Konto“. Die klassische Seite ist post-quantensicher (ML-DSA-65). Ein
Register für Quanten-Noten läuft schon im Hauptnetz: Seriennummern, Deckung
in der Währung, Freischaltung durch eine Mehrheit unabhängiger Prüfer,
Einlösen. Es ist schlafend, bis es ein tragfähiges Verfahren gibt.

In Ihrem Artikel schreiben Sie, dass eine durchgehende Token-Infrastruktur von
der Ausgabe bis zur Prüfung noch nicht gezeigt ist. Genau diesen Teil könnten
wir beitragen. Wir haben dafür ein Modell gebaut, Wiesner-Noten mit klassischer
Prüfung, bei dem das Prüfgeheimnis nicht bei einer Bank liegt, sondern in
Anteilen bei den Validatoren der Kette:
https://github.com/hanoi96international-gif/Aequitas/blob/main/forschung/quantengeld/ANALYSE.md
(Simulation mit Angriffen: `wiesner_verteilt.py`).

Unsere Frage: Wäre ein gemeinsamer Versuch denkbar, bei dem ein Token aus
einem Ihrer Speicher über die Kette ausgegeben, klassisch geprüft und
eingelöst wird? Wir stellen Software und Kette, Sie den Speicher.

Wir bitten um keine Zusage. Schon ein Hinweis, welches Projekt am besten
passt, hilft uns sehr.

Mit freundlichen Grüßen
`<Name des Betreibers>`

---

## Vorlage 3: Kleinförderung der Unitary Foundation (englisch, Entwurf)

Stand 16.11.2026. Laut Suchergebnis 4.000 $, laufend (vor dem Antrag auf
der offiziellen Seite prüfen: Formular, Bedingungen, Lizenz). Nur Arbeiten
nennen, die es gibt; kein Ergebnis versprechen.

> **Project:** Open-source simulation and ledger specification for decentralized,
> classically verified quantum tokens.
>
> **What exists:** an open-source ledger (Go) with a live, dormant register for quantum
> notes (issuance, backing, threshold activation, redemption), and exact simulations of
> Wiesner-type notes, group-action notes and their noise requirements
> (`forschung/quantengeld/`).
>
> **What the grant funds (6 months):**
> 1. Noise-tolerant parameters for classically verified private notes, following
>    Kumar 2019, inside `wiesner_verteilt.py`.
> 2. Blind verification requests, so validators do not learn which note is checked.
> 3. A runnable ledger specification (like `bruecke_kette.py`) and a short write-up for
>    IACR ePrint.
> 4. A small hardware experiment on a trapped-ion device (minting and classical
>    verification of a few-qubit note).
>
> **Open source:** everything stays public in the repository.
