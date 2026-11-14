#!/usr/bin/env python3
"""Quantengeld aus einer ECHTEN Isogenie-Gruppenaktion (CSIDH-Art) -- klein.

Forschung, kein Produkt (README.md, Abschnitt 3c). Keine Abhaengigkeiten.

WARUM. zhandry_spielzeug.py benutzt Z_N, das auf sich selbst wirkt -- dort
baut jeder die Note aus der Seriennummer nach. Hier wirkt stattdessen die
Klassengruppe cl(Z[sqrt(-p)]) auf supersingulaere Montgomery-Kurven
    E_A : y^2 = x^3 + A x^2 + x   ueber F_p,  p = 4 * l_1 * ... * l_n - 1,
genau wie in CSIDH (Castryck, Lange, Martindale, Panny, Renes 2018) und wie
Zhandry (2024) es fuer Quantengeld vorschlaegt. Ein Ideal l_i = (l_i, pi - 1)
wirkt durch eine Isogenie vom Grad l_i (Velu-Formeln, x-Koordinaten); das
Inverse ueber Punkte auf dem Twist. Jeder kann die Wirkung berechnen.

WAS DIE SIMULATION ZEIGT.
  1. Die Wirkung ist eine echte, kommutative, freie Gruppenaktion: die Bahn von
     E_0 hat h Elemente (h = Klassenzahl), [l_1][l_2] = [l_2][l_1],
     [l][l]^-1 = 1.
  2. Ist die Klassengruppe zyklisch mit Erzeuger g, ist die Note
        |$_y> = 1/sqrt(h) * Summe_k  w^(y k) |g^k * E_0>
     ein Eigenzustand der oeffentlichen Wirkung von g. Pruefen kann jeder
     (Phasenschaetzung), Drucken auch -- die Seriennummer ist Zufall.
  3. WO DIE SICHERHEIT SITZT: Wer |$_y> zu einer gewuenschten Seriennummer
     direkt bauen will, muss zu jeder Kurve E ihren Index k mit E = g^k * E_0
     kennen -- das Vektorisierungs-Problem (diskreter Logarithmus der
     Gruppenaktion). Hier loest es eine vollstaendige Tabelle in h Schritten,
     weil h winzig ist. Fuer kryptografische p (h ~ 2^256) ist das auch fuer
     Quantenrechner nur subexponentiell (Kuperberg) und praktisch aussichtslos.
     Das Spielzeug Z_N hatte diese Stufe nicht -- hier ist sie sichtbar.

Grenzen: winzige Parameter (unsicher), ideale Phasenschaetzung, kein
Rauschen (dafuer rauschen_oeffentlich.py). Der Beweis der Sicherheit ist
weiterhin nur im generischen Modell (Doliskani 2025).

Aufruf: python3 -I isogenie_geld.py [--selbsttest]
"""
from __future__ import annotations

import cmath
import math
import random
import sys

# p = 4 * 3 * 5 * 7 - 1 = 419, das Lehrbeispiel aus dem CSIDH-Artikel.
ELLS = (3, 5, 7)
P = 4 * 3 * 5 * 7 - 1


def inv(a: int) -> int:
    return pow(a % P, P - 2, P)


def ist_quadrat(a: int) -> bool:
    a %= P
    return a == 0 or pow(a, (P - 1) // 2, P) == 1


def rhs(A: int, x: int) -> int:
    return (x * x * x + A * x * x + x) % P


# ---- Montgomery-Leiter auf x (projektiv X:Z) ----

def xdbl(X: int, Z: int, a24: int):
    t0 = (X + Z) % P
    t1 = (X - Z) % P
    t0 = t0 * t0 % P
    t1 = t1 * t1 % P
    X2 = t0 * t1 % P
    t2 = (t0 - t1) % P
    Z2 = t2 * (t1 + a24 * t2) % P
    return X2, Z2


def xadd(XP, ZP, XQ, ZQ, Xd, Zd):
    u = (XP - ZP) * (XQ + ZQ) % P
    v = (XP + ZP) * (XQ - ZQ) % P
    s, d = (u + v) % P, (u - v) % P
    return Zd * s * s % P, Xd * d * d % P


def xmul(x: int, n: int, A: int):
    """[n] x auf E_A; Rueckgabe x oder None (Unendlich)."""
    if n == 0:
        return None
    a24 = (A + 2) * inv(4) % P
    R0, R1 = (1, 0), (x % P, 1)
    for bit in bin(n)[2:]:
        if bit == "0":
            R1 = xadd(*R0, *R1, x % P, 1)
            R0 = xdbl(*R0, a24)
        else:
            R0 = xadd(*R0, *R1, x % P, 1)
            R1 = xdbl(*R1, a24)
    X, Z = R0
    if Z % P == 0:
        return None
    return X * inv(Z) % P


def isogenie(A: int, kern_x: int, ell: int, bild_x):
    """Isogenie vom Grad ell mit Kern <K> (x(K) = kern_x).
    Neues A = pi^2 (A - 6 sigma) mit pi = prod x_j, sigma = sum (x_j - 1/x_j)
    ueber j = 1..(ell-1)/2 (Costello-Hisil, x-Koordinaten); Bild von bild_x
    x * prod ((x x_j - 1)/(x - x_j))^2. Am Zustand geprueft: die Bildkurve ist
    supersingulaer, das Bild liegt auf ihr (Selbsttest)."""
    xs = []
    for j in range(1, (ell - 1) // 2 + 1):
        xs.append(xmul(kern_x, j, A))
    pi, sigma = 1, 0
    for xj in xs:
        pi = pi * xj % P
        sigma = (sigma + xj - inv(xj)) % P
    A_neu = pi * pi * (A - 6 * sigma) % P
    bild = None
    if bild_x is not None:
        b = bild_x % P
        for xj in xs:
            b = b * ((bild_x * xj - 1) * inv(bild_x - xj)) ** 2 % P
        bild = b % P
    return A_neu, bild


def wirkung(A: int, exponenten, rng: random.Random) -> int:
    """[l_1^e1 ... l_n^en] * E_A (CSIDH-Algorithmus, Zufallspunkte)."""
    e = list(exponenten)
    while any(e):
        x = rng.randrange(1, P)
        if rhs(A, x) == 0:
            continue  # Punkt der Ordnung 2
        s = 1 if ist_quadrat(rhs(A, x)) else -1
        S = [i for i, ei in enumerate(e) if ei != 0 and (1 if ei > 0 else -1) == s]
        if not S:
            continue
        k = 1
        for i in S:
            k *= ELLS[i]
        Q = xmul(x, (P + 1) // k, A)
        for i in S:
            if Q is None:
                break
            R = xmul(Q, k // ELLS[i], A)
            k //= ELLS[i]
            if R is None:
                continue
            # x-Koordinaten-Arithmetik gilt fuer E_A UND seinen Twist: ein
            # Kern auf dem Twist (s = -1) ergibt mit derselben Formel die
            # Isogenie zu l_i^-1.
            A, Q = isogenie(A, R, ELLS[i], Q)
            e[i] -= s
    return A % P


# ---- Bahn, Klassenzahl, Erzeuger ----

def bahn(rng: random.Random):
    """Alle Kurven der Bahn von E_0 (A = 0) unter <l_1, ..., l_n>."""
    gesehen = {0}
    rand = [0]
    while rand:
        A = rand.pop()
        for i in range(len(ELLS)):
            for s in (1, -1):
                e = [0] * len(ELLS)
                e[i] = s
                B = wirkung(A, e, rng)
                if B not in gesehen:
                    gesehen.add(B)
                    rand.append(B)
    return sorted(gesehen)


def zyklus(A0: int, i: int, rng: random.Random):
    """Die Kurven A0, l_i*A0, l_i^2*A0, ... bis zur Rueckkehr."""
    e = [0] * len(ELLS)
    e[i] = 1
    folge = [A0]
    A = wirkung(A0, e, rng)
    while A != A0:
        folge.append(A)
        A = wirkung(A, e, rng)
    return folge


# ---- Die Note (Zustandsvektor ueber den h Kurven) ----

class Geld:
    def __init__(self, rng: random.Random):
        self.rng = rng
        alle = bahn(rng)
        self.h = len(alle)
        # Erzeuger: ein l_i, dessen Zyklus die ganze Bahn ist.
        for i in range(len(ELLS)):
            z = zyklus(0, i, rng)
            if len(z) == self.h:
                self.erzeuger = i
                self.kurven = z  # kurven[k] = g^k * E_0 -- die "DL-Tabelle"
                break
        else:
            raise ValueError("Klassengruppe nicht von einem l_i erzeugt")
        self.index = {A: k for k, A in enumerate(self.kurven)}

    def w(self, a: int) -> complex:
        return cmath.exp(2j * math.pi * a / self.h)

    def g_wirkung(self, zustand):
        """Oeffentliche Wirkung von g: |E> -> |g*E> -- berechnet mit der
        Isogenie, NICHT mit der Tabelle (jeder kann das)."""
        e = [0] * len(ELLS)
        e[self.erzeuger] = 1
        neu = {}
        for A, amp in zustand.items():
            B = wirkung(A, e, self.rng)
            neu[B] = neu.get(B, 0) + amp
        return neu

    def note_aus_tabelle(self, y: int):
        """|$_y> -- braucht die DL-Tabelle (Index jeder Kurve)."""
        return {A: self.w(y * k) / math.sqrt(self.h) for k, A in enumerate(self.kurven)}

    def drucken(self):
        """Drucken ohne Wunsch-Seriennummer: Summe_k |k>|g^k E_0>, QFT auf das
        erste Register, messen. Ergebnis y ist gleichverteilt; uebrig bleibt
        |$_{-y}>. (Die Auswertung von g^k * E_0 fuer jedes k braucht die
        Gruppenstruktur, nicht den DL einer GEGEBENEN Kurve.)"""
        y = self.rng.randrange(self.h)
        return (-y) % self.h, self.note_aus_tabelle((-y) % self.h)

    def pruefen(self, seriennummer: int, zustand) -> float:
        """Ideale Phasenschaetzung: Wahrscheinlichkeit, dass sie die
        Seriennummer ergibt = |<$_s|zustand>|^2."""
        ziel = self.note_aus_tabelle(seriennummer)
        u = sum(ziel.get(A, 0).conjugate() * a for A, a in zustand.items())
        return abs(u) ** 2


def selbsttest():
    rng = random.Random(419)
    # 1. Die Wirkung ist kommutativ und invertierbar.
    for _ in range(5):
        A = 0
        a = wirkung(wirkung(A, (1, 0, 0), rng), (0, 1, 0), rng)
        b = wirkung(wirkung(A, (0, 1, 0), rng), (1, 0, 0), rng)
        assert a == b, "nicht kommutativ"
        assert wirkung(wirkung(A, (0, 0, 1), rng), (0, 0, -1), rng) == A, "nicht invertierbar"
    # 2. Jede Kurve der Bahn ist supersingulaer: #E(F_p) = p + 1, also
    #    [p+1] x = Unendlich fuer jedes x auf der Kurve.
    geld = Geld(rng)
    for A in geld.kurven:
        for _ in range(3):
            x = rng.randrange(1, P)
            if ist_quadrat(rhs(A, x)):
                assert xmul(x, P + 1, A) is None, ("nicht supersingulaer", A)
    # 2b. Isogenien: Bild eines Punktes liegt auf der Bildkurve (auch fuer
    #     Kerne auf dem Twist), die Bildkurve ist in der Bahn.
    for A in geld.kurven[:9]:
        for ell in ELLS:
            for _ in range(60):
                x = rng.randrange(1, P)
                if rhs(A, x) == 0:
                    continue
                R = xmul(x, (P + 1) // ell, A)
                if R is None:
                    continue
                z = next(z for z in range(1, P) if rhs(A, z) and xmul(z, ell, A) is not None)
                A2, bild = isogenie(A, R, ell, z)
                assert A2 in geld.index, ("Bildkurve nicht in der Bahn", A, ell)
                assert bild is None or xmul(bild, P + 1, A2) is None, ("Bild nicht auf der Kurve", A, ell)
                break
    # 3. Freie Wirkung: g^h = 1 und die Bahn hat genau h Kurven.
    assert len(set(geld.kurven)) == geld.h
    # 4. Die Note ist Eigenzustand der oeffentlichen Wirkung von g.
    for y in range(geld.h):
        note = geld.note_aus_tabelle(y)
        bild = geld.g_wirkung(note)
        lam = geld.w(-y)
        for A in note:
            assert abs(bild[A] - lam * note[A]) < 1e-9, ("kein Eigenzustand", y)
    # 5. Gedruckt und oeffentlich geprueft: besteht sicher.
    for _ in range(5):
        s, note = geld.drucken()
        assert abs(geld.pruefen(s, note) - 1) < 1e-9
        # Unter einer anderen Seriennummer nie.
        assert geld.pruefen((s + 1) % geld.h, note) < 1e-9
    # 6. Ein Faelscher mit einer klassisch bekannten Kurve besteht nur zu 1/h.
    for A in geld.kurven[:5]:
        assert abs(geld.pruefen(3, {A: 1.0}) - 1 / geld.h) < 1e-9
    print(f"selbsttest ok (p = {P}, Klassenzahl h = {geld.h}, Erzeuger l = {ELLS[geld.erzeuger]})")


def main():
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    rng = random.Random(1)
    geld = Geld(rng)
    print(f"p = {P}, Bahn von E_0: h = {geld.h} Kurven, Erzeuger [l_{ELLS[geld.erzeuger]}]")
    print("A-Koeffizienten der Bahn (g^k * E_0):", geld.kurven)
    s, note = geld.drucken()
    print(f"gedruckt: Seriennummer {s}, oeffentlich geprueft: {geld.pruefen(s, note):.3f}")
    print(f"Faelschung mit klassischer Kurve: {geld.pruefen(s, {geld.kurven[0]: 1.0}):.3f} (= 1/h)")
    print("Nachbau zu Wunsch-Seriennummer braucht die Index-Tabelle: hier h =", geld.h,
          "Schritte; fuer kryptografische p ~ 2^256 Kurven -- ausser Reichweite.")


if __name__ == "__main__":
    main()
