#!/usr/bin/env python3
"""Wie schwer ist die Vektorisierung fuer einen Quantenrechner? -- selbst gemessen.

Forschung, kein Produkt (BAUVORGABE.md, Punkt 2 und 5.1). Keine Abhaengigkeiten.

HINTERGRUND. Die Sicherheit der Isogenie-Note (isogenie_geld.py) haengt daran,
dass niemand zu einer Kurve E ihren Index k mit E = g^k * E_0 findet. Ein
Quantenrechner fuehrt das auf ein "verborgenes Verschiebungsproblem" in einer
Gruppe der Groesse N = h zurueck und loest es mit Kuperbergs Sieb in
subexponentieller Zeit 2^O(sqrt(log N)). Jede ABFRAGE kostet eine
Isogenie-Auswertung im Quantenschaltkreis.

WAS HIER GEMESSEN WIRD. Jede Abfrage liefert ein Qubit
    (|0> + exp(2 pi i s l / N) |1>) / sqrt(2)
mit einem zufaelligen, BEKANNTEN Etikett l in Z_N (N = 2^n). Das Sieb
verknuepft zwei Qubits mit Etiketten l1, l2; die Messung ergibt mit
Wahrscheinlichkeit 1/2 ein Qubit mit Etikett l1 - l2 (sonst l1 + l2, dann
verworfen -- Kuperbergs erster Algorithmus). Ziel: ein Qubit mit Etikett
genau N/2; seine Messung verraet das unterste Bit von s, und mit derselben
Arbeit je Bit folgt der ganze Index. Ob das gelingt und wie viele Abfragen es
braucht, haengt NUR an den Etiketten -- die Phasen muessen dafuer nicht
simuliert werden. Darum ist die Abfragezahl hier exakt messbar.

SIEB (erster Algorithmus, Stufen zu je b Bit): Etiketten nach den untersten
b noch nicht genullten Bits gruppieren, je zwei mit gleichen Bits verknuepfen
(Differenz nullt diese Bits), die Haelfte ueberlebt. Nach etwa n/b Stufen
bleibt das oberste Bit.

ERGEBNIS (main): gemessene Abfragen fuer n = 12 ... 40, Anpassung an
log2 Q = a * sqrt(n) + c, Hochrechnung auf kryptografische n -- als OBERE
Schranke fuer diesen Algorithmus; bessere Siebe (Kuperberg 2011, Regev,
Peikert) brauchen weniger, siehe BAUVORGABE.md.

Aufruf: python3 -I kuperberg.py [--selbsttest]
"""
from __future__ import annotations

import math
import random
import sys


def sieb(n: int, abfragen: int, b: int, rng: random.Random) -> bool:
    """Ein Lauf des Siebs mit `abfragen` Start-Qubits; True, wenn ein Etikett
    genau 2^(n-1) entsteht."""
    N = 1 << n
    ziel = N >> 1
    pool = [rng.randrange(1, N) for _ in range(abfragen)]
    genullt = 0  # so viele unterste Bits sind bei allen Etiketten 0
    while genullt < n - 1:
        schritt = min(b, n - 1 - genullt)
        maske = ((1 << (genullt + schritt)) - 1)
        eimer: dict[int, list[int]] = {}
        for l in pool:
            eimer.setdefault(l & maske, []).append(l)
        neu = []
        for gruppe in eimer.values():
            for i in range(0, len(gruppe) - 1, 2):
                if rng.random() < 0.5:  # Messung ergibt die Differenz
                    d = (gruppe[i] - gruppe[i + 1]) % N
                    if d:
                        neu.append(d)
        pool = neu
        genullt += schritt
        if not pool:
            return False
    return ziel in pool


# Obergrenze der Startzahl je Versuch: begrenzt Speicher und Zeit (eine
# ungeeignete Stufenbreite braeuchte sonst beliebig viele Abfragen).
Q_MAX = 1 << 21


def noetige_abfragen(n: int, b: int, rng: random.Random, laeufe: int = 6) -> int | None:
    """Kleinste Startzahl (Zweierpotenz-Raster, dann fein), mit der das Sieb in
    mindestens der Haelfte der Laeufe gelingt -- None ueber Q_MAX."""
    q = 16
    while True:
        erfolge = sum(sieb(n, q, b, rng) for _ in range(laeufe))
        if erfolge * 2 >= laeufe:
            break
        q *= 2
        if q > Q_MAX:
            return None
    lo, hi = q // 2, q
    for _ in range(5):
        mid = (lo + hi) // 2
        erfolge = sum(sieb(n, mid, b, rng) for _ in range(laeufe))
        if erfolge * 2 >= laeufe:
            hi = mid
        else:
            lo = mid
    return hi


def beste_stufe(n: int, rng: random.Random) -> tuple[int, int]:
    """Stufenbreite b mit den wenigsten Abfragen (Kuperberg: b ~ sqrt(n))."""
    best = None
    w = int(round(math.sqrt(n)))
    for b in range(max(1, w - 1), w + 3):
        q = noetige_abfragen(n, b, rng)
        if q is not None and (best is None or q < best[1]):
            best = (b, q)
    if best is None:
        raise ValueError(f"n = {n}: ueber Q_MAX")
    return best


def anpassen(punkte):
    """Kleinste Quadrate fuer log2 Q = a * sqrt(n) + c."""
    xs = [math.sqrt(n) for n, _ in punkte]
    ys = [math.log2(q) for _, q in punkte]
    mx, my = sum(xs) / len(xs), sum(ys) / len(ys)
    a = sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / sum((x - mx) ** 2 for x in xs)
    return a, my - a * mx


def selbsttest() -> None:
    rng = random.Random(2003)
    # 1. Kleines n: das Sieb findet das Ziel-Etikett, und mit zu wenigen
    #    Abfragen nicht.
    assert sum(sieb(8, 4096, 3, rng) for _ in range(10)) >= 8
    assert not any(sieb(8, 8, 3, rng) for _ in range(10))
    # 2. Jede Verknuepfung nullt wirklich die untersten Bits (Etikettenalgebra).
    N = 1 << 10
    a, b = 0b1011010011, 0b0110110011
    assert ((a - b) % N) & 0b11111 == 0
    # 3. Die Abfragen wachsen subexponentiell: von n = 12 auf n = 24 (Faktor
    #    2 in n) waechst log2 Q deutlich langsamer als linear in n.
    q12 = beste_stufe(12, rng)[1]
    q24 = beste_stufe(24, rng)[1]
    assert q24 > q12
    assert math.log2(q24) / math.log2(q12) < 24 / 12
    print(f"selbsttest ok (n=12: {q12} Abfragen, n=24: {q24})")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    rng = random.Random(1)
    punkte = []
    print("| n = log2 N | beste Stufe b | gemessene Abfragen Q | log2 Q |")
    print("|---|---|---|---|")
    for n in (12, 16, 20, 24, 28, 32, 36):
        b, q = beste_stufe(n, rng)
        punkte.append((n, q))
        print(f"| {n} | {b} | {q:,} | {math.log2(q):.1f} |".replace(",", "."), flush=True)
    a, c = anpassen(punkte)
    print(f"\nAnpassung: log2 Q = {a:.2f} * sqrt(n) {'+' if c >= 0 else '-'} {abs(c):.2f}")
    print("\nHochrechnung (erster Algorithmus, OBERE Schranke; Abfragen allein, ohne Kosten je Isogenie):")
    print("| Primzahl p | n ~ log2 h ~ (log2 p)/2 | log2 Abfragen |")
    print("|---|---|---|")
    for bits in (512, 1024, 2048, 4096):
        n = bits // 2
        print(f"| {bits} Bit | {n} | {a * math.sqrt(n) + c:.0f} |")


if __name__ == "__main__":
    main()
