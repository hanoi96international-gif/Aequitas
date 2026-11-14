#!/usr/bin/env python3
"""Was kostet EINE Abfrage? Isogenie-Auswertung im Quantenschaltkreis -- Schaetzung.

Forschung, kein Produkt (BAUVORGABE.md §2c, §5.1). Keine Abhaengigkeiten.

Jede Abfrage des Kuperberg-Siebs (kuperberg.py) wertet die Gruppenaktion
einmal in Ueberlagerung aus. Ihre Kosten = (Koerpermultiplikationen je
Gruppenaktion) x (Quantengatter je Multiplikation).

1. MULTIPLIKATIONEN, GEZAEHLT. Die x-Koordinaten-Arithmetik aus
   isogenie_geld.py wird hier mit einem Zaehler nachgebaut und gegen das
   Original geprueft (gleiche Ergebnisse). Gezaehlt fuer eine Aktion mit
   Exponenten in {-1, 0, 1} (die "niedrigen Exponenten" fuer grosse
   Klassengruppen):
     je Primzahl l_i: eine Skalarmultiplikation mit dem Kofaktor (~ log2 p
     Leiterschritte) + Kern und Isogenie (~ l_i).
   Daraus ein Modell M(p) = Summe_i [ a * log2 p + b * l_i ] * Anteil, dessen
   Konstanten a, b am gezaehlten Kleinfall angepasst werden.

2. HOCHRECHNUNG. Fuer p mit B Bit braucht man so viele kleine ungerade
   Primzahlen l_i, dass ihr Produkt ~ p/4 ist.

3. GATTER. Eine modulare Multiplikation zweier B-Bit-Zahlen kostet im
   Quantenschaltkreis grob c * B^2 Toffoli-Gatter (Schulbuch, umkehrbar;
   Literatur: c ~ 4 bis 10). Hier c = 4 (guenstig fuer den Angreifer).

ERGEBNIS: Gatter je Abfrage, mal Abfragen (kuperberg.py) = Quantenanteil des
Angriffs; daneben die klassische Arbeit des Siebs. Der Angriff kostet das
Groessere.

Aufruf: python3 -I isogenie_kosten.py [--selbsttest]
"""
from __future__ import annotations

import importlib.util
import math
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("isogenie_geld", os.path.join(_hier, "isogenie_geld.py"))
iso = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(iso)

TOFFOLI_JE_MUL = 4  # c in c * B^2


class Zaehler:
    def __init__(self, p: int):
        self.p = p
        self.mul = 0

    def m(self, a: int, b: int) -> int:
        self.mul += 1
        return a * b % self.p

    def inv(self, a: int) -> int:
        # Inversion per Fermat: ~ 1,5 * log2 p Multiplikationen (Quadrieren + Mal).
        self.mul += int(1.5 * self.p.bit_length())
        return pow(a % self.p, self.p - 2, self.p)


def xmul_z(z: Zaehler, x: int, n: int, A: int):
    P = z.p
    if n == 0:
        return None
    a24 = z.m(A + 2, z.inv(4))

    def xdbl(X, Z):
        t0 = z.m(X + Z, X + Z)
        t1 = z.m(X - Z, X - Z)
        X2 = z.m(t0, t1)
        t2 = (t0 - t1) % P
        return X2, z.m(t2, (t1 + z.m(a24, t2)) % P)

    def xadd(XP, ZP, XQ, ZQ, Xd, Zd):
        u = z.m(XP - ZP, XQ + ZQ)
        v = z.m(XP + ZP, XQ - ZQ)
        s, d = (u + v) % P, (u - v) % P
        return z.m(Zd, z.m(s, s)), z.m(Xd, z.m(d, d))

    R0, R1 = (1, 0), (x % P, 1)
    for bit in bin(n)[2:]:
        if bit == "0":
            R1 = xadd(*R0, *R1, x % P, 1)
            R0 = xdbl(*R0)
        else:
            R0 = xadd(*R0, *R1, x % P, 1)
            R1 = xdbl(*R1)
    X, Z = R0
    if Z % P == 0:
        return None
    return z.m(X, z.inv(Z))


def isogenie_z(z: Zaehler, A: int, kern_x: int, ell: int, bild_x):
    """Wie iso.isogenie, aber Kern-Vielfache ueber xADD (~ell Mult.) statt
    je ein xmul (so rechnet jede ernsthafte Umsetzung)."""
    P = z.p
    xs = [kern_x]
    if ell > 3:
        xs.append(xmul_z(z, kern_x, 2, A))
    while len(xs) < (ell - 1) // 2:
        # x([j+1]K) aus x([j]K), x(K), x([j-1]K): ein xADD (~6 Mult.)
        j = len(xs)
        nxt = xmul_z(z, kern_x, j + 1, A) if False else None
        # Differenz-Addition in affinen x: x_{j+1} = (x_j x_1 - 1)^2 / (x_{j-1} (x_j - x_1)^2)
        num = z.m(z.m(xs[-1], kern_x) - 1, z.m(xs[-1], kern_x) - 1)
        den = z.m(xs[-2] if j >= 2 else 1, z.m(xs[-1] - kern_x, xs[-1] - kern_x))
        xs.append(z.m(num, z.inv(den)) if j >= 2 else nxt)
    pi, sigma = 1, 0
    for xj in xs:
        pi = z.m(pi, xj)
        sigma = (sigma + xj - z.inv(xj)) % P
    A_neu = z.m(z.m(pi, pi), (A - 6 * sigma) % P)
    bild = None
    if bild_x is not None:
        b = bild_x % P
        for xj in xs:
            f = z.m(z.m(bild_x, xj) - 1, z.inv(bild_x - xj))
            b = z.m(b, z.m(f, f))
        bild = b
    return A_neu, bild


def aktion_mult(p: int, ells: tuple, rng: random.Random, laeufe: int = 20) -> float:
    """Mittlere Multiplikationen einer Aktion mit Exponenten in {-1,0,1}."""
    iso.P, iso.ELLS = p, ells
    gesamt = 0
    for _ in range(laeufe):
        z = Zaehler(p)
        e = [rng.choice((-1, 0, 1)) for _ in ells]
        A = 0
        while any(e):
            x = rng.randrange(1, p)
            if iso.rhs(A, x) == 0:
                continue
            s = 1 if iso.ist_quadrat(iso.rhs(A, x)) else -1
            S = [i for i, ei in enumerate(e) if ei != 0 and (1 if ei > 0 else -1) == s]
            if not S:
                continue
            k = 1
            for i in S:
                k *= ells[i]
            Q = xmul_z(z, x, (p + 1) // k, A)
            for i in S:
                if Q is None:
                    break
                R = xmul_z(z, Q, k // ells[i], A)
                k //= ells[i]
                if R is None:
                    continue
                A, Q = isogenie_z(z, A, R, ells[i], Q)
                e[i] -= s
        gesamt += z.mul
    return gesamt / laeufe


def kleine_primzahlen(bits: int) -> list[int]:
    """Kleinste ungerade Primzahlen, deren Produkt mal 4 >= 2^bits."""
    out, prod, q = [], 4, 3
    while prod.bit_length() < bits:
        if all(q % d for d in range(3, int(q ** 0.5) + 1, 2)):
            out.append(q)
            prod *= q
        q += 2
    return out


def modell_mult(bits: int, a: float = 9.0, b: float = 8.0) -> float:
    """M = Summe_i (2/3) * [a * bits + b * l_i]: je Primzahl (Anteil 2/3 mit
    Exponent ungleich 0) Kofaktor-Leiter plus Kern/Isogenie."""
    return sum((2 / 3) * (a * bits + b * l) for l in kleine_primzahlen(bits))


def selbsttest() -> None:
    rng = random.Random(5)
    # 1. Die zaehlende Arithmetik rechnet dasselbe wie das Original.
    iso.P, iso.ELLS = 419, (3, 5, 7)
    for _ in range(30):
        x, A = rng.randrange(1, 419), rng.choice((0, 158, 261))
        n = rng.randrange(1, 500)
        assert xmul_z(Zaehler(419), x, n, A) == iso.xmul(x, n, A)
    for A in (0, 158, 261):
        for ell in (3, 5, 7):
            for _ in range(200):
                x = rng.randrange(1, 419)
                if iso.rhs(A, x) == 0:
                    continue
                R = iso.xmul(x, 420 // ell, A)
                if R is None:
                    continue
                z = next(z for z in range(1, 419) if iso.rhs(A, z) and iso.xmul(z, ell, A) is not None)
                assert isogenie_z(Zaehler(419), A, R, ell, z) == iso.isogenie(A, R, ell, z), (A, ell)
                break
    # 2. Die gezaehlte Aktion liegt in der Groessenordnung des Modells
    #    (Faktor < 3 fuer beide kleinen Faelle).
    for p, ells in ((419, (3, 5, 7)), (868019, (3, 5, 17, 23, 37))):
        gez = aktion_mult(p, ells, rng, 10)
        mod = sum((2 / 3) * (9 * p.bit_length() + 8 * l) for l in ells)
        assert 1 / 3 < gez / mod < 3, (p, gez, mod)
    # 3. Primzahlliste: Produkt reicht.
    l = kleine_primzahlen(512)
    prod = 4
    for q in l:
        prod *= q
    assert prod.bit_length() >= 512 and len(l) > 60
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    rng = random.Random(1)
    for p, ells in ((419, (3, 5, 7)), (868019, (3, 5, 17, 23, 37))):
        gez = aktion_mult(p, ells, rng)
        mod = sum((2 / 3) * (9 * p.bit_length() + 8 * l) for l in ells)
        print(f"p = {p}: gezaehlt {gez:.0f} Multiplikationen je Aktion, Modell {mod:.0f}")
    # Abfragen und klassische Arbeit aus kuperberg.py (Kollimation, BAUVORGABE 2b/2c)
    q_fit, w_fit = (1.06, 0.70), (2.05, 2.96)
    print("\n| Primzahl | kleine Primzahlen | Mult. je Aktion | Toffoli je Abfrage | Abfragen | Quantenanteil | klassische Arbeit | Angriff kostet |")
    print("|---|---|---|---|---|---|---|---|")
    for bits in (512, 4096, 8192, 16384):
        n = bits // 2
        k = len(kleine_primzahlen(bits))
        mult = modell_mult(bits)
        tof = mult * TOFFOLI_JE_MUL * bits * bits
        lq = q_fit[0] * math.sqrt(n) + q_fit[1]
        lw = w_fit[0] * math.sqrt(n) + w_fit[1]
        quant = math.log2(tof) + lq
        print(f"| {bits} Bit | {k} | 2^{math.log2(mult):.1f} | 2^{math.log2(tof):.1f} | 2^{lq:.0f} | 2^{quant:.0f} | 2^{lw:.0f} | **2^{max(quant, lw):.0f}** |")


if __name__ == "__main__":
    main()
