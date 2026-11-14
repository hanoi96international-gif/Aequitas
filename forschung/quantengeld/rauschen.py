#!/usr/bin/env python3
"""Quantengeld unter Rauschen: was muss die Hardware koennen?

Forschung, kein Produkt (README.md). Keine Abhaengigkeiten.

FRAGE. Eine Quanten-Banknote aus n Qubits liegt eine Zeit t im Speicher.
Jedes Qubit zerfaellt mit der Kohaerenzzeit T: misst der Pruefer es in der
richtigen Basis, ist es mit p(t) = (1 - exp(-t/T)) / 2 gekippt
(Depolarisation/Dephasierung, das Standardmodell). Der Pruefer muss deshalb
einen Anteil Fehler dulden: er nimmt an, wenn hoechstens k = floor(d*n) Qubits
falsch sind.

ZWEI FORDERUNGEN.
  1. Die echte Note besteht nach der Lagerzeit mit >= 99 %:
     P[Bin(n, p) <= k] >= 0,99.
  2. Eine Faelschung besteht hoechstens mit 10^-12:
     P[Bin(n, 1/4) <= k] <= 10^-12.
     1/4 ist der beste Faelscher je Qubit bei Wiesner-Noten (Molina, Vidick,
     Watrous 2012: optimale Faelschung (3/4)^n ohne Toleranz). Mit Toleranz
     zaehlt der Schwanz der Binomialverteilung. Fuer oeffentlich pruefbare
     Verfahren (Zhandry) ist die Rechnung anders, die Groessenordnung der
     Kohaerenzforderung aehnlich: auch dort muss der Zustand bis zur Pruefung
     nahezu ungestoert bleiben.

ERGEBNIS (main): fuer jede Lagerzeit die noetige Kohaerenzzeit T und die
Notengroesse n. Damit laesst sich jede Hardware-Ankuendigung pruefen: reicht
ihre Kohaerenzzeit fuer eine Note, die man einen Tag in der Tasche traegt?

Exakt gerechnet (Brueche/Logarithmen), Monte-Carlo nur im Selbsttest.
"""
from __future__ import annotations

import math
import random
import sys

EHRLICH_MIN = 0.99
FAELSCHUNG_MAX = 1e-12
FAELSCHER_FEHLER = 0.25  # bester Faelscher je Qubit (Wiesner)


def binom_cdf(n: int, k: int, p: float) -> float:
    """P[Bin(n,p) <= k], stabil ueber Logarithmen."""
    if k < 0:
        return 0.0
    if k >= n:
        return 1.0
    if p <= 0.0:
        return 1.0
    if p >= 1.0:
        return 0.0
    lp, lq = math.log(p), math.log1p(-p)
    # log C(n,i) iterativ
    log_c = 0.0
    terme = []
    for i in range(0, k + 1):
        if i > 0:
            log_c += math.log(n - i + 1) - math.log(i)
        terme.append(log_c + i * lp + (n - i) * lq)
    m = max(terme)
    return min(1.0, math.exp(m) * sum(math.exp(x - m) for x in terme))


def fehlerrate(t: float, kohaerenz: float) -> float:
    """Kippwahrscheinlichkeit eines Qubits nach Lagerzeit t (gleiche Einheit wie kohaerenz)."""
    return (1.0 - math.exp(-t / kohaerenz)) / 2.0


def kleinste_note(p: float, n_max: int = 4096):
    """Kleinstes n (mit passendem k), das beide Forderungen erfuellt -- oder None."""
    if p >= FAELSCHER_FEHLER:
        return None
    for n in range(1, n_max + 1):
        # groesstes k mit Faelschung <= Grenze (monoton in k)
        k_hi = -1
        lo, hi = 0, n
        while lo <= hi:
            mid = (lo + hi) // 2
            if binom_cdf(n, mid, FAELSCHER_FEHLER) <= FAELSCHUNG_MAX:
                k_hi = mid
                lo = mid + 1
            else:
                hi = mid - 1
        if k_hi < 0:
            continue
        if binom_cdf(n, k_hi, p) >= EHRLICH_MIN:
            return n, k_hi
    return None


def noetige_kohaerenz(lagerzeit: float, n_max: int = 4096) -> tuple[float, int, int]:
    """Kleinste Kohaerenzzeit T (Vielfaches der Lagerzeit), bei der eine Note
    mit hoechstens n_max Qubits beide Forderungen erfuellt. Rueckgabe (T, n, k)."""
    # p ist monoton fallend in T; suche das groesste p, das noch geht.
    lo, hi = 0.0, FAELSCHER_FEHLER
    best = None
    for _ in range(40):
        mid = (lo + hi) / 2
        r = kleinste_note(mid, n_max)
        if r is not None:
            best = (mid, r)
            lo = mid
        else:
            hi = mid
    if best is None:
        raise ValueError("keine Note moeglich")
    p, (n, k) = best
    # p = (1 - e^{-t/T})/2  ->  T = -t / ln(1 - 2p)
    t_koh = -lagerzeit / math.log(1.0 - 2.0 * p)
    return t_koh, n, k


def _mc(n: int, k: int, p: float, laeufe: int, rng: random.Random) -> float:
    gut = 0
    for _ in range(laeufe):
        fehler = sum(1 for _ in range(n) if rng.random() < p)
        gut += fehler <= k
    return gut / laeufe


def selbsttest() -> None:
    rng = random.Random(7)
    # 1. Exakte Verteilung gegen Monte-Carlo.
    for n, k, p in ((40, 4, 0.05), (64, 10, 0.25), (16, 0, 0.25)):
        exakt = binom_cdf(n, k, p)
        mc = _mc(n, k, p, 20000, rng)
        assert abs(exakt - mc) < 0.02, (n, k, p, exakt, mc)
    # 2. Ohne Toleranz ist die Faelschung (3/4)^n -- das bekannte Ergebnis.
    assert abs(binom_cdf(16, 0, 0.25) - 0.75 ** 16) < 1e-12
    # 3. Ohne Rauschen genuegen 97 Qubits ohne Toleranz ((3/4)^97 < 1e-12).
    n, k = kleinste_note(0.0)
    assert (n, k) == (97, 0), (n, k)
    # 4. Bei p >= 1/4 gibt es keine sichere Note (Pruefer kann echt und falsch
    #    nicht mehr trennen).
    assert kleinste_note(0.25) is None
    # 5. Mehr Rauschen -> groessere Note, und die Forderungen halten.
    n1, k1 = kleinste_note(0.01)
    n2, k2 = kleinste_note(0.05)
    assert n2 > n1
    for n, k, p in ((n1, k1, 0.01), (n2, k2, 0.05)):
        assert binom_cdf(n, k, p) >= EHRLICH_MIN
        assert binom_cdf(n, k, FAELSCHER_FEHLER) <= FAELSCHUNG_MAX
    # 6. Kohaerenz skaliert linear mit der Lagerzeit.
    t1, _, _ = noetige_kohaerenz(1.0, 512)
    t2, _, _ = noetige_kohaerenz(10.0, 512)
    assert abs(t2 / t1 - 10.0) < 1e-6
    print("selbsttest ok")


def _dauer(s: float) -> str:
    for einheit, sek in (("Jahre", 365 * 86400), ("Tage", 86400), ("Std.", 3600), ("Min.", 60), ("s", 1)):
        if s >= sek:
            return f"{s / sek:,.1f} {einheit}".replace(",", ".")
    return f"{s * 1000:.1f} ms"


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    print("Forderung: echte Note besteht >= 99 %, Faelschung <= 1e-12 (Wiesner, bester Faelscher 1/4 je Qubit)\n")
    print("| Lagerzeit | max. Qubits | noetige Kohaerenzzeit T | Note n | Toleranz k |")
    print("|---|---|---|---|---|")
    for lager, name in ((60, "1 Minute"), (3600, "1 Stunde"), (86400, "1 Tag"), (7 * 86400, "1 Woche"), (30 * 86400, "30 Tage")):
        for n_max in (256, 2048):
            t, n, k = noetige_kohaerenz(lager, n_max)
            print(f"| {name} | {n_max} | {_dauer(t)} | {n} | {k} |")
    print("\nZum Vergleich (Stand der Technik, Laborwerte): supraleitende Qubits ~0,1-1 ms,")
    print("Ionenfallen Sekunden bis Minuten, Kernspins in Festkoerpern bis ~6 Stunden (gekuehlt).")


if __name__ == "__main__":
    main()
