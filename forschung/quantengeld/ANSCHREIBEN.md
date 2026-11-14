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
