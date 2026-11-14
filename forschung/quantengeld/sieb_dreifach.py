#!/usr/bin/env python3
"""Ist unser Angreifer stark genug? Kollimation mit DREI statt zwei Vektoren.

Forschung, kein Produkt (BAUVORGABE §2c, §5.1). Keine Abhaengigkeiten.

Die Groessenvorgabe (p >= 8.192 / 17.000 Bit) haengt an der klassischen
Arbeit des Kollimationssiebs (kuperberg.py: 2er-Zusammenfuehrung, Hash-
Abgleich, Arbeit ~ 2^(2,05 sqrt n)). Waere ein anderes Sieb billiger, waere
die Vorgabe zu klein -- das muss vor jeder Festlegung ausgeschlossen sein.

Naheliegende Verbesserung: drei Vektoren der Laenge L zusammenfuehren und
2 log2 L Bits auf einmal nullen. Weniger Stufen, weniger Blaetter, weniger
Abfragen -- aber jede Zusammenfuehrung muss alle L^2 Paare durchgehen (ein
3SUM-artiges Problem; schneller geht es allgemein nicht bekannt).

Gemessen, exakt ueber die Etiketten wie in kuperberg.py, jeweils beim
guenstigsten L: Arbeit und Abfragen des 2er- und des 3er-Siebs.

ERGEBNIS (gemessen, n = 16 ... 32, je guenstigstes L): das 3er-Sieb ist
NIE billiger -- Arbeit 1,0x bis 4,0x der des 2er-Siebs. Rechnung dazu:
2er-Sieb Arbeit ~ 2^(n/l + l) -> 2^(2 sqrt n); 3er-Sieb Blaetter
3^(n/2l), je Zusammenfuehrung L^2 = 2^(2l) -> 2^(2 sqrt(1,58 n)) = 2^(2,5 sqrt n).
Der Angreifer nimmt das 2er-Sieb; die Vorgabe, die an der Arbeit haengt,
bleibt. (Messung bei kleinem n verrauscht; die Rechnung traegt die Aussage
fuer grosse n.)

Aufruf: python3 -I sieb_dreifach.py [--selbsttest]
"""
from __future__ import annotations

import importlib.util
import math
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("kuperberg", os.path.join(_hier, "kuperberg.py"))
ku = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(ku)


def _kollimieren3(v1, v2, v3, genullt, r, N, rng, z):
    """Fuegt drei Vektoren zusammen, misst r Bits der Summe ab Bit genullt."""
    m = (1 << r) - 1
    c = ((v1[rng.randrange(len(v1))] + v2[rng.randrange(len(v2))] + v3[rng.randrange(len(v3))]) >> genullt) & m
    nach_rest: dict[int, list[int]] = {}
    for y in v3:
        nach_rest.setdefault((y >> genullt) & m, []).append(y)
    neu = []
    for x in v1:
        for w in v2:
            s = x + w
            for y in nach_rest.get((c - ((s >> genullt) & m)) & m, ()):
                neu.append((s + y - (c << genullt)) % N)
    z.arbeit += len(v1) * len(v2) + len(v3) + len(neu)
    z.speicher = max(z.speicher, len(v1) + len(v2) + len(v3) + len(neu))
    return neu


def kollimation3(n: int, L: int, rng: random.Random, kosten: dict) -> tuple[bool, int]:
    N = 1 << n
    z = ku._Zaehler()
    r = max(1, 2 * int(math.log2(L)))

    def baue(genullt: int) -> list[int]:
        if genullt == 0:
            return ku._blatt(N, L, rng, z)
        vor = max(0, genullt - r)
        while True:
            v = _kollimieren3(baue(vor), baue(vor), baue(vor), vor, genullt - vor, N, rng, z)
            if len(v) >= 2:
                return v[: 4 * L]
            if z.abfragen > ku.Q_MAX:
                return v

    v = baue(n - 1)
    ok = len(v) >= 2 and 0 in v and (N >> 1) in v and all(x in (0, N >> 1) for x in v)
    kosten["arbeit"] = kosten.get("arbeit", 0) + z.arbeit
    return ok, z.abfragen


def messen(n: int, L: int, arity: int, rng: random.Random, laeufe: int = 4):
    """Mittlere (Abfragen, Arbeit) je erfolgreichem Lauf."""
    q = w = 0
    erfolge = 0
    for _ in range(laeufe * 4):
        k: dict = {}
        if arity == 2:
            ok, qq = ku.kollimation(n, L, rng, k)
        else:
            ok, qq = kollimation3(n, L, rng, k)
        q += qq
        w += k.get("arbeit", 0)
        erfolge += ok
        if erfolge >= laeufe:
            break
    if erfolge == 0:
        return None
    return q / erfolge, w / erfolge


def guenstigste_arbeit(n: int, arity: int, rng: random.Random, max_lg: int = 11):
    best = None
    for lg in range(2, max_lg + 1):
        L = 1 << lg
        if arity == 3 and L * L > 1 << 18:
            break  # Paare je Zusammenfuehrung begrenzen (Laufzeit)
        r = messen(n, L, arity, rng)
        if r is None:
            continue
        if best is None or r[1] < best[1]:
            best = (r[0], r[1], L)
    return best


def selbsttest() -> None:
    rng = random.Random(33)
    # 1. Das 3er-Sieb ist korrekt (Ziel: nur Etiketten 0 und N/2).
    for _ in range(3):
        ok, _ = kollimation3(16, 16, rng, {})
    assert any(kollimation3(16, 16, rng, {})[0] for _ in range(6))
    # 2. Beim guenstigsten L kostet das 3er-Sieb mehr Arbeit als das 2er-Sieb.
    for n in (16, 20):
        a2 = guenstigste_arbeit(n, 2, rng, 7)
        a3 = guenstigste_arbeit(n, 3, rng, 7)
        assert a3[1] > a2[1], (n, a2, a3)
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    rng = random.Random(7)
    print("| n | 2er: Abfragen | 2er: Arbeit | L | 3er: Abfragen | 3er: Arbeit | L | Arbeit 3er / 2er |")
    print("|---|---|---|---|---|---|---|---|")
    for n in (16, 20, 24, 28, 32):
        q2, w2, l2 = guenstigste_arbeit(n, 2, rng)
        q3, w3, l3 = guenstigste_arbeit(n, 3, rng)
        print(f"| {n} | 2^{math.log2(q2):.1f} | 2^{math.log2(w2):.1f} | {l2} | 2^{math.log2(q3):.1f} | 2^{math.log2(w3):.1f} | {l3} | {w3 / w2:.1f}x |", flush=True)


if __name__ == "__main__":
    main()
