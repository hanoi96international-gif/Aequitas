# Forschungsstand und mögliche Ansprechpartner

Stand 14.11.2026. Grundlage sind Suchergebnisse zu veröffentlichten Arbeiten.
Die Volltexte konnte ich nicht lesen, weil arXiv von hier aus gesperrt war.
**Vor jeder Zitierung oder Kontaktaufnahme die Arbeit selbst lesen.** Genannt
sind nur Autorinnen und Autoren veröffentlichter Arbeiten, keine Zusagen und
keine Kontaktdaten.

## Arbeiten

| Arbeit | Inhalt (laut Zusammenfassung) | Bezug zu Aequitas |
|---|---|---|
| **Coladangelo & Sattath**, *A Quantum Money Solution to the Blockchain Scalability Problem*, Quantum 4, 297 (2020), arXiv:2002.11998 | Quanten-Banknoten aus Quantum Lightning, Kette führt Seriennummern, „bolt-to-signature“ zurück in digitales Geld, Rückholung bei Verlust | **die Brücke selbst** (BRUECKE.md) |
| **M. Zhandry**, *Quantum Money from Abelian Group Actions*, TheoretiCS (Aug. 2025), arXiv:2307.12120 | öffentlich prüfbares Quantengeld aus abelschen Gruppenaktionen, z. B. Isogenien gewöhnlicher elliptischer Kurven | `zhandry_spielzeug.py` |
| **J. Doliskani**, IACR ePrint 2025/092 | Kopieren der Noten → diskreter Logarithmus der Gruppenaktion; damit Zhandrys Verfahren im generischen Modell sicher | Sicherheitsgrundlage |
| **Doliskani, Mirzaei, Mousavi**, *Public-Key Quantum Money and Fast Real Transforms*, arXiv:2503.18890 (überarbeitet März 2026) | Variante mit Hartley- statt Fouriertransformation, reelle Amplituden | einfachere Umsetzung? |
| **Kim, Heo, Hong** (Korea University), *Cryptanalysis of Isogeny-Based Quantum Money with Rational Points*, IACR ePrint 2025/201, arXiv:2508.00351, Quantum (10.11.2026) | Angriff auf das Isogenie-Lightning von Montgomery & Sharif; exponentielle Zeit, also unpraktisch | Risiko für Lightning |
| **Liu, Montgomery, Zhandry**, *Another Round of Breaking and Making Quantum Money*, arXiv:2211.11994 | Gitter-Ansätze gebrochen | Warnung |
| *Quantum Key Exchange from Public Quantum Money*, arXiv:2611.05985 (4.11.2026) | untere Schranke: aus Quantengeld und Lightning folgen bestimmte Rätsel-Primitive | Theorie |
| *Noise-tolerant public-key quantum money from a classical oracle*, arXiv:2407.06463 | rauschtolerant, aber nur im Orakelmodell | Rauschen |

## Mögliche Ansprechpartner (Autoren der Arbeiten oben)

- **Or Sattath** (Ben-Gurion-Universität) und **Andrea Coladangelo**: Autoren
  der Brücke. Sie sind der naheliegendste erste Kontakt.
- **Mark Zhandry**: öffentlich prüfbares Quantengeld aus Gruppenaktionen,
  Quantum Lightning.
- **Jake Doliskani** und Koautoren: Sicherheitsbeweis und Hartley-Variante.

Kontaktdaten stehen auf den Arbeiten und den Seiten der Institute.
**Anschreiben verschickt nur der Betreiber**, Vorlage in ANSCHREIBEN.md.

## Förderung (vor einer Bewerbung auf den offiziellen Seiten prüfen)

- **NLnet / NGI Zero** (nlnet.nl): fördert offene Software. Ausschreibungen
  und Fristen wechseln; eine Frist vom 01.08.2026 ist vorbei.
- **EU Horizont Europa / Digitales Europa**: Themen zu Post-Quanten-Kryptografie.
  Sie sind eher für die Stufen 0–5 (Quantensicherheit) passend als für
  Quantengeld.
- **Deutschland (BMBF), Österreich (Quantum Austria)**: Quantentechnologie-
  Programme, meist für Hochschulen und Forschungseinrichtungen. Ein Weg führt
  über eine Kooperation mit einem Lehrstuhl.
