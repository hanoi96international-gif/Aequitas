#!/usr/bin/env python3
"""Variante B gegen glatte Faelschungen -- an der echten kleinen Gruppe.

Forschung, kein Produkt (BAUVORGABE §8). Keine Abhaengigkeiten.

Variante B: der Pruefer zieht u KLASSISCH gleichverteilt aus der ganzen
Gruppe und macht einen Hadamard-Test mit der gesteuerten Aktion von u.
Annahme-Operator je Test: K_u = (I + chi(u,c)^* U_u) / 2.

Behauptung (pegasis_kosten.py): weil Mittel_u chi(u,c)^* U_u = P_c, besteht
eine fremde Note je Test mit genau 1/2, und JEDER Zustand psi nach T Tests mit
  P[besteht] = |<$_c|psi>|^2 + (1 - |<$_c|psi>|^2) * 2^-T.
Also: nach 128 Tests genau so scharf wie die volle Projektion -- auch gegen
die glatten Faelschungen (Gaussglocken), an denen die "schwache" Pruefung
scheiterte (schwache_pruefung.py).

Hier wird das an der echten Gruppe p = 868019 (h = 1491) nachgemessen:
Zustaende exakt, Tests als Kraus-Operatoren nacheinander, u zufaellig;
Mittel ueber viele Laeufe gegen die Formel.

Aufruf: python3 -I variante_b.py [--selbsttest]
"""
from __future__ import annotations

import importlib.util
import math
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("schwache_pruefung", os.path.join(_hier, "schwache_pruefung.py"))
sp = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(sp)


def test_b(g, c: int, psi: list, tests: int, rng: random.Random) -> float:
    """T Tests mit je frischem, gleichverteiltem u. Rueckgabe: P[alle bestanden]."""
    h = g.h
    for _ in range(tests):
        u = rng.randrange(h)
        lam = g.w(c * u)  # = conj(chi(u, c)) mit U_u|$_c> = w(-c u)|$_c>
        neu = [0j] * h
        for k, a in enumerate(psi):
            neu[k] += a / 2
            neu[(k + u) % h] += lam * a / 2
        psi = neu
    return sum(abs(x) ** 2 for x in psi)


def formel(voll: float, tests: int) -> float:
    return voll + (1 - voll) * 2.0 ** -tests


def mittel(g, c, psi, tests, laeufe, rng) -> float:
    return sum(test_b(g, c, psi, tests, rng) for _ in range(laeufe)) / laeufe


def selbsttest() -> None:
    rng = random.Random(4)
    g = sp.Gruppe(random.Random(1491))
    c = rng.randrange(g.h)
    # 1. Die echte Note besteht immer.
    assert abs(test_b(g, c, g.note(c), 8, rng) - 1) < 1e-9
    # 2. Eine fremde Note: im Mittel 2^-T.
    d = (c + 500) % g.h
    m = mittel(g, c, g.note(d), 3, 300, rng)
    assert abs(m - 1 / 8) < 0.04, m
    # 3. Glatte Faelschung (Glocke, an der die schwache Pruefung scheiterte):
    #    Mittel trifft |<$_c|psi>|^2 + (1 - ...) 2^-T.
    for sigma in (0.4, 0.7):
        phi = g.glocke(c, sigma)
        v = g.voll(c, phi)
        for T in (2, 5):
            m = mittel(g, c, phi, T, 250, rng)
            f = formel(v, T)
            assert abs(m - f) < 0.03 + 0.1 * f, (sigma, T, m, f)
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    rng = random.Random(7)
    g = sp.Gruppe(random.Random(1491))
    c = rng.randrange(g.h)
    print(f"Echte kleine Gruppe p = {sp.P}, h = {g.h}. Variante B: u gleichverteilt, je Test ein Hadamard-Test.")
    print("| Zustand | volle Projektion | B, T=1 | B, T=5 | Formel T=5 | schwach, 1 Runde (zum Vergleich) |")
    print("|---|---|---|---|---|---|")
    faelle = [("echte Note", g.note(c)), ("fremde Note", g.note((c + 500) % g.h))]
    faelle += [(f"Glocke sigma = {s}", g.glocke(c, s)) for s in (0.4, 0.5, 0.7)]
    for name, psi in faelle:
        v = g.voll(c, psi)
        b1 = mittel(g, c, psi, 1, 300, rng)
        b5 = mittel(g, c, psi, 5, 300, rng)
        print(f"| {name} | {v:.3f} | {b1:.3f} | {b5:.3f} | {formel(v, 5):.3f} | {g.schwach(c, psi, 1):.3f} |")
    print("\nNach 128 Tests: P[besteht] = |<$_c|psi>|^2 + 2^-128 -- so scharf wie die volle Projektion.")


if __name__ == "__main__":
    main()
