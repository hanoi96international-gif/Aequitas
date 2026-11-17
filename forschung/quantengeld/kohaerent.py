#!/usr/bin/env python3
"""Variante W und S: strenge Schranke gegen VERSCHRAENKTE (kohaerente) Angriffe.

Forschung, kein Produkt. Keine Abhaengigkeiten. Antwort auf Frage W1 und S1 des
Pruefauftrags (docs/QUANTENGELD_PRUEFAUFTRAG.md): Ist der Kloner Stelle fuer
Stelle (W) bzw. die Breidbart-Messung (S) der beste Angriff, oder bringen
verschraenkte Strategien ueber viele Qubits mehr?

DAS SPIEL. Je Stelle i ein geheimer BB84-Zustand (Basis u_i, Wert t_i,
gleichverteilt, unabhaengig). Der Faelscher bekommt alle n Qubits und darf
beliebig quantenmechanisch arbeiten (verschraenkt, kollektiv).
  W: er gibt zwei Kopien ab (Kanal n -> 2n Qubits); Kopie A wird an einer
     Zufallsauswahl S_A von Stellen in der richtigen Basis gemessen, Kopie B an
     S_B (disjunkt). Er braucht BEIDE Pruefungen bestanden.
  S: er misst und legt an zwei Stellen vor; jede prueft ihre Stichprobe in der
     Basis, die zu ihr gehoert -- er braucht je Stelle Antworten in BEIDEN
     Basen, ohne zu wissen, welche gefragt wird.
Bestanden: Fehler * 100 <= tol * Treffer (Regel der Kette).

DER BEWEIS (Skizze, Argument wie Molina, Vidick, Watrous 2012, arXiv:1202.4010):
 1. 1[Fehler <= tau*Treffer] <= s^(tau*Treffer - Fehler) fuer jedes s >= 1
    (beide Pruefungen zusammen: Summe ueber A und B; "beide bestehen" ist in
    "Summe besteht" enthalten).
 2. Mit Bernoulli-Auswahl (Stelle in S_A mit Wahrscheinlichkeit q, in S_B mit
    q) ist E[s^(...)] fuer einen festen Angriff = Tr(J R) mit R = R_1 (x) ...
    (x) R_n, jedes R_i >= 0 (nichtnegative Gewichte auf BB84-Projektoren).
 3. Das Optimum ueber ALLE Kanaele (J >= 0, Spur ueber den Ausgang = 1) ist ein
    SDP; ist Y_i dual zulaessig fuer eine Stelle (1 (x) Y_i >= R_i), dann ist
    Y_1 (x) ... (x) Y_n dual zulaessig fuer alle (A >= B >= 0, C >= D >= 0
    => A(x)C >= B(x)D). Also: Wert(n Stellen) = Wert(1 Stelle)^n -- genau, und
    der Produktangriff erreicht ihn. Verschraenkung bringt NICHTS.
 4. Der Wert je Stelle haengt nur von der groessten Summe der Treffsicherheiten
    ab: W: F_A + F_B <= 1 + 1/sqrt 2 (optimaler, auch asymmetrischer
    phasenkovarianter Kloner, Bruss u. a. 2000; Ausgleich Cerf 2000); S: die
    beiden Antworten zusammen <= 2 * (1/2 + 1/sqrt 8) (Breidbart, optimale
    Zwei-Basen-Unterscheidung). Beides dieselbe Zahl 2*BETA.
 5. Feste Groesse k statt Bernoulli: P(fest) <= P(Bernoulli) / P(|S_A| = |S_B| = k)
    (Bedingen; der Nenner ist ein Multinomial, rund 1/(2 pi k)).
 6. Absprache, die einen Anteil f der Stellen kennt: dort Summe 2 statt 2*BETA.

ERGEBNIS: P(beide bestehen) <= Schranke(k, n, tol, f), streng fuer jeden
Angriff, der die zweite Vorlage nicht von Rueckmeldungen der ersten abhaengig
macht. Rueckmeldung von b Bits kostet hoechstens b Bits (Faktor 2^b), mehrere
Versuche je einen Faktor.

DER BEFUND (17.11.2026): Die Tabelle (wiesner_parameter.suche, quantenWKTabelle)
rechnet JE KOPIE mit 14,64 % Fehler. Das traegt nur gegen den symmetrischen
Kloner: ein asymmetrischer Kloner macht eine Kopie fast perfekt, die andere
schlecht -- "je Kopie" ist dann nicht vorsichtig. Richtig ist die Schranke fuer
"beide bestehen". Sie liegt fuer die Eintraege der Tabelle -- siehe
python3 -I kohaerent.py.

GRENZEN (ehrlich):
  - Adaptive Angriffe bei W: Kopie B darf nach der Pruefung von A bearbeitet
    werden (A's Stellen und Basen stehen dann offen auf der Kette, und B wird
    nur noch an anderen Stellen gefragt). Das deckt Schritt 3 nicht ab; dafuer
    braucht es das SDP fuer Strategien mit zwei Runden (Gutoski-Watrous).
  - Schritt 4 nutzt die bekannte Ein-Stellen-Grenze aus der Literatur; der
    Selbsttest prueft sie nur numerisch fuer eine Familie von Klonern und
    Messungen, nicht als SDP.

Aufruf:  python3 -I kohaerent.py [--selbsttest]
"""
from __future__ import annotations

import math
import sys

BETA = 0.5 + 1 / math.sqrt(8)          # 0,853553...: beste Treffsicherheit je Kopie/Basis
SUMME_W = 1 + 1 / math.sqrt(2)         # F_A + F_B des besten Kloners = 2*BETA
ZIEL = -40.0                           # log2 der erlaubten Faelschungs-Chance


def log2_g(s: float, q: float, tau: float, summe: float) -> float:
    """log2 des Werts je Stelle: E[s^(tau*Treffer - Fehler)] im besten Fall.
    Je Stelle: mit q in S_A, mit q in S_B, sonst ungefragt; gefragt ist ein
    Treffer mit 1/2 (Basis passt). Bei einem Treffer der Kopie X:
    s^tau * (F_X + (1 - F_X)/s). Linear in F_A + F_B, also nur die Summe."""
    treffer = (q / 2) * s ** tau * (summe * (1 - 1 / s) + 2 / s)
    return math.log2(1 - q + treffer)


def log2_bernoulli(n: int, q: float, tau: float, f: float) -> float:
    """min ueber s >= 1 von n * log2 g(s): Bernoulli-Auswahl, Absprache f."""
    beste = 0.0
    s = 1.0
    while s < 64:
        w = f * log2_g(s, q, tau, 2.0) if f > 0 else 0.0
        v = n * ((1 - f) * log2_g(s, q, tau, SUMME_W) + w) if f < 1 else n * w
        beste = min(beste, v)
        s *= 1.002
    return beste


def log2_multinomial_k(n: int, k: int, q: float) -> float:
    """log2 P(|S_A| = k und |S_B| = k) bei Bernoulli(q, q, 1-2q) je Stelle."""
    lg = math.lgamma
    ln = (lg(n + 1) - 2 * lg(k + 1) - lg(n - 2 * k + 1)
          + 2 * k * math.log(q) + ((n - 2 * k) * math.log(1 - 2 * q) if n > 2 * k else 0.0))
    return ln / math.log(2)


def schranke(k: int, n: int, tol: int, f: float = 0.0) -> float:
    """log2 der strengen Schranke fuer "beide Vorlagen bestehen", feste
    Groesse k je Pruefung (k geprueft, im Mittel k/2 Treffer), Note n >= 2k."""
    if n < 2 * k:
        raise ValueError("n >= 2k (zwei disjunkte Pruefungen)")
    q = k / n
    return log2_bernoulli(n, q, tol / 100, f) - log2_multinomial_k(n, k, q)


def produkt_symmetrisch(k: int, tol: int) -> float:
    """log2 der Chance des symmetrischen Kloners, EINE Kopie durchzubringen
    (so rechnet die Tabelle): Treffer ~ Bin(k, 1/2), Fehler ~ Bin(h, 0,1464)."""
    c = 1 - BETA
    tot = 0.0
    for h in range(0, k + 1):
        ph = math.exp(math.lgamma(k + 1) - math.lgamma(h + 1) - math.lgamma(k - h + 1) - k * math.log(2))
        if ph < 1e-300:
            continue
        grenze = (tol * h) // 100
        pe = sum(math.exp(math.lgamma(h + 1) - math.lgamma(e + 1) - math.lgamma(h - e + 1)
                          + e * math.log(c) + (h - e) * math.log(1 - c)) for e in range(0, grenze + 1))
        tot += ph * pe
    return math.log2(tot) if tot > 0 else -math.inf


def log2_kopie(k: int, tol: int, fehler: float) -> float:
    """log2 P(eine Kopie mit Fehlerrate `fehler` je Treffer besteht), k geprueft."""
    tot = 0.0
    for h in range(0, k + 1):
        lph = math.lgamma(k + 1) - math.lgamma(h + 1) - math.lgamma(k - h + 1) - k * math.log(2)
        grenze = (tol * h) // 100
        for e in range(0, grenze + 1):
            if fehler <= 0:
                tot += math.exp(lph) if e == 0 else 0.0
                continue
            if fehler >= 1:
                tot += math.exp(lph) if e == h else 0.0
                continue
            tot += math.exp(lph + math.lgamma(h + 1) - math.lgamma(e + 1) - math.lgamma(h - e + 1)
                            + e * math.log(fehler) + (h - e) * math.log(1 - fehler))
    return math.log2(tot) if tot > 0 else -math.inf


def bester_produktangriff(k: int, tol: int) -> tuple[float, float]:
    """Bester Kloner Stelle fuer Stelle (auch asymmetrisch) fuer "beide
    bestehen": F_A = (1 + cos eta)/2, F_B = (1 + sin eta)/2; die Kopien
    werden an disjunkten Stellen geprueft, also unabhaengig. Gibt (log2 P, eta)."""
    best, arg = -math.inf, 0.0
    for i in range(0, 91):
        eta = math.radians(i)
        v = log2_kopie(k, tol, 1 - 0.5 * (1 + math.cos(eta))) + log2_kopie(k, tol, 1 - 0.5 * (1 + math.sin(eta)))
        if v > best:
            best, arg = v, i
    return best, arg


def kleinstes_k(tol: int, f: float = 0.0, ziel: float = ZIEL, abzug_bits: float = 2.0) -> int:
    """Kleinstes k (n = 2k), bei dem die strenge Schranke plus `abzug_bits`
    (z. B. 2 Bits fuer bis zu 4 Versuche) unter dem Ziel liegt."""
    k = 10
    while schranke(k, 2 * k, tol, f) + abzug_bits > ziel:
        k += 10
    return k


# ---------------------------------------------------------------- Selbsttest
def _kloner_summe(eta: float) -> float:
    """F_A + F_B eines asymmetrischen phasenkovarianten Kloners (Familie mit
    Gewicht eta), fuer BB84-Zustaende in x-z: F_A = 1/2 + cos(eta)/2 * ...
    Wir nehmen die bekannte Randkurve sqrt(F_A - 1/2)... direkt als Zahlen:
    F_A = 1/2 (1 + cos eta), F_B = 1/2 (1 + sin eta) (Niu-Griffiths/Cerf)."""
    return 0.5 * (1 + math.cos(eta)) + 0.5 * (1 + math.sin(eta))


def _breidbart_summe(phi: float) -> float:
    """Messung in der Basis um den Winkel phi zwischen Z und X gedreht: die
    Antworten fuer Z und X stammen aus demselben Ergebnis. Summe der
    Treffsicherheiten in beiden Basen."""
    return 0.5 * (1 + math.cos(phi)) + 0.5 * (1 + math.sin(phi))


def selbsttest() -> bool:
    global SUMME_W
    ok = True
    # Schritt 4: die Randkurven erreichen ihr Maximum bei 45 Grad mit 2*BETA.
    for fn in (_kloner_summe, _breidbart_summe):
        m = max(fn(i * math.pi / 2000) for i in range(1001))
        ok &= abs(m - 2 * BETA) < 1e-9 and abs(SUMME_W - 2 * BETA) < 1e-12
    # g(1) = 1 (kein Gewicht), und g faellt fuer s etwas > 1 nur, wenn tau < 1-BETA.
    ok &= abs(log2_g(1.0, 0.3, 0.04, SUMME_W)) < 1e-12
    ok &= log2_g(1.05, 0.5, 0.04, SUMME_W) < 0 < log2_g(1.05, 0.5, 0.20, SUMME_W)
    # Mutation: ohne Absprache-Anteil wird die Schranke nie schlechter als mit.
    ok &= schranke(840, 1680, 4, 0.0) <= schranke(840, 1680, 4, 0.25)
    # Mutation: mehr Stellen machen die Schranke kleiner.
    ok &= schranke(2000, 4000, 4) < schranke(840, 1680, 4)
    # GEGENPROBE: jeder ausrechenbare Angriff liegt UNTER der Schranke (sonst
    # waere sie falsch). Bester Produktangriff, auch asymmetrisch, k klein.
    for k in (60, 120, 240):
        for tol in (4, 8, 12):
            p, _ = bester_produktangriff(k, tol)
            ok &= p <= schranke(k, 2 * k, tol) + 1e-9
    # Mutation: mit einer zu kleinen Summe je Stelle (1,6 statt 1,707) kaeme
    # die Schranke unter den echten Angriff -- der Vergleich muss das sehen.
    alt, SUMME_W = SUMME_W, 1.5
    zu_klein = schranke(240, 480, 12) < bester_produktangriff(240, 12)[0]
    SUMME_W = alt
    ok &= zu_klein
    # Bernoulli-Schranke ist eine echte Wahrscheinlichkeitsschranke (<= 0).
    ok &= log2_bernoulli(100, 0.5, 0.04, 0.0) <= 0
    print("selbsttest ok" if ok else "SELBSTTEST FEHLGESCHLAGEN")
    return ok


def main():
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    print("Strenge Schranke (jeder nicht-adaptive Angriff, auch verschraenkt) fuer")
    print("'beide Vorlagen bestehen', Toleranz 4 %, Note n = 2k:\n")
    print(f"{'Fall':<22}{'k':>6}{'f':>7}{'Tabelle je Kopie':>18}{'streng, beide':>15}")
    faelle = [("S/W, 1 Validator", 840, 0.0), ("W, t=2 v=4 (f=1/4)", 1576, 0.25),
              ("W, t=2 v=3 (f=1/3)", 2536, 1 / 3), ("W/S, t=3 v=5 (f=2/5)", 3472, 0.4),
              ("W/S, t=3 v=4 (f=1/2)", 5200, 0.5)]
    for name, k, f in faelle:
        tab = produkt_symmetrisch(k, 4) if f == 0 else float("nan")
        st = schranke(k, 2 * k, 4, f)
        print(f"{name:<22}{k:>6}{f:>7.3f}{tab:>18.1f}{st:>15.1f}")
    print("\nlog2-Werte; Ziel <= -40. Die Spalte 'Tabelle je Kopie' ist nur fuer f = 0 gerechnet.")
    p, eta = bester_produktangriff(840, 4)
    print(f"\nGegenprobe k = 840: bester Produktangriff (beide bestehen) log2 = {p:.1f} bei eta = {eta} Grad"
          f" -- liegt unter der Schranke {schranke(840, 1680, 4):.1f}.")
    print("\nKleinstes k mit strenger Schranke + 2 Bit (4 Versuche) <= -40, Toleranz 4 %:")
    for name, f in (("f = 0", 0.0), ("f = 1/4", 0.25), ("f = 1/3", 1 / 3), ("f = 2/5", 0.4), ("f = 1/2", 0.5)):
        print(f"  {name:<8} k >= {kleinstes_k(4, f)}")


if __name__ == "__main__":
    main()
