#!/usr/bin/env python3
"""Der Hebel: weniger Multiplikationen je Gruppenaktion (BAUVORGABE §2e, §5.4).

Forschung, kein Produkt. Keine Abhaengigkeiten.

Das Pruefen einer Note kostet ~2 Gruppenaktionen in Ueberlagerung
(isogenie_kosten.py). Bisher gezaehlt: der Lehrbuch-Algorithmus aus
isogenie_geld.py -- je Primzahl l_i eine eigene Skalarmultiplikation
[k / l_i] Q, also ~ s * B/2 Leiterschritte fuer s Primzahlen und B Bit.

HEBEL 1 (hier gebaut, exakt gezaehlt): Teile und herrsche (Literatur:
"Strategien" fuer SIDH/CSIDH, De Feo-Jao-Plut 2011, Hutchinson u. a. 2020,
nachzulesen). Die Menge S wird halbiert; [prod S2] Q liefert den Kernpunkt
fuer die erste Haelfte, Q selbst wird durch deren Isogenien geschoben und
traegt die zweite Haelfte. Leiterschritte ~ B/2 * log2 s statt s * B/2.
Selbsttest: GLEICHE Kurve wie das Original (isogenie_geld.wirkung), fuer
Primzahlen mit 3, 11, 19 und 29 kleinen Teilern.

KORREKTUR (gegen uns, ehrlich): Der Pruefer projiziert auf den Eigenraum
der ganzen Gruppe -- die Exponenten muessen die Gruppe ueberdecken:
(2m+1)^s >= N ~ sqrt(p). Bei 8.192 Bit und 758 Primzahlen: m = 21, nicht 1.
Das kostet ~m Runden statt einer. (Der Angriff bleibt davon unberuehrt: ab
8.192 Bit bestimmt ihn die klassische Arbeit des Siebs.)

HEBEL 2 (nur Literatur-Modell, NICHT gebaut): "sqrt-Velu" (Bernstein,
De Feo, Leroux, Smith 2020, nachzulesen) senkt die Kosten einer Isogenie vom
Grad l von ~l auf ~sqrt(l)*log2(l); Uebergang laut Literatur bei l ~ 100.
Hier nur als Schaetzung ausgewiesen.

ERGEBNIS (14.11.2026): Gezaehlt wird jetzt PROJEKTIV (ohne Division),
die Kostenwanderung trifft die exakte Zaehlung auf < 20 %. Bei 8.192 Bit:
  - Ueberdeckung kostet 2^4 (gegen uns, vorher uebersehen).
  - Baum / optimale Strategie / Schranken je Primzahl: zusammen nur 2^0,7 --
    bei kryptografischer Groesse kostet nicht die Leiter, sondern das
    Schieben von Punkten und die Isogenien selbst (~ Summe |e_i| * l_i).
  - sqrt-Velu (Literatur-Modell): weitere 2^1,3.
  Pruefen: ~2^55 Toffoli. Die einfachen Hebel sind damit ausgeschoepft;
  der Faktor 2^5 bis 2^10 muss STRUKTURELL kommen (BAUVORGABE §2e, §5.4).

Aufruf: python3 -I isogenie_hebel.py [--selbsttest]
"""
from __future__ import annotations

import importlib.util
import math
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))


def _lade(name):
    spec = importlib.util.spec_from_file_location(name, os.path.join(_hier, name + ".py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


iso = _lade("isogenie_geld")
kosten = _lade("isogenie_kosten")

# Kleine CSIDH-Primzahlen p = 4 * (Produkt der ersten k ungeraden Primzahlen) - 1,
# p = 3 mod 8, gefunden durch Suche (Selbsttest prueft die Primalitaet).
TESTPRIMZAHLEN = (11, 19, 29)


def ungerade_primzahlen(k: int) -> tuple:
    out, q = [], 3
    while len(out) < k:
        if all(q % d for d in range(3, int(q ** 0.5) + 1, 2)):
            out.append(q)
        q += 2
    return tuple(out)


def ist_primzahl(n: int) -> bool:
    if n < 2:
        return False
    for d in (2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37):
        if n % d == 0:
            return n == d
    d, s = n - 1, 0
    while d % 2 == 0:
        d //= 2
        s += 1
    for a in (2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37):
        x = pow(a, d, n)
        if x in (1, n - 1):
            continue
        for _ in range(s - 1):
            x = x * x % n
            if x == n - 1:
                break
        else:
            return False
    return True


def csidh_primzahl(k: int):
    ells = ungerade_primzahlen(k)
    p = 4
    for l in ells:
        p *= l
    return p - 1, ells


class Zaehler:
    """Zaehlt Koerpermultiplikationen (Quadrate eingeschlossen) und
    Leiterschritte. Alles PROJEKTIV, ohne Division -- so rechnen ernsthafte
    Umsetzungen; nur je Runde eine Inversion fuer die Punktwahl."""

    def __init__(self, p: int):
        self.p, self.mul, self.leiter = p, 0, 0

    def m(self, a: int, b: int) -> int:
        self.mul += 1
        return a * b % self.p

    def hoch(self, a: int, n: int) -> int:
        r = 1
        for bit in bin(n)[2:]:
            r = self.m(r, r)
            if bit == "1":
                r = self.m(r, a)
        return r

    def inv(self, a: int) -> int:
        return self.hoch(a % self.p, self.p - 2)


# Kurve als (A24 : C24) = (A + 2C : 4C), Punkte als (X : Z).

def xdbl(z, X, Z, A24, C24):
    t0 = z.m(X - Z, X - Z)
    t1 = z.m(X + Z, X + Z)
    c = z.m(C24, t0)
    t2 = (t1 - t0) % z.p
    return z.m(c, t1), z.m(t2, (c + z.m(A24, t2)) % z.p)


def xadd(z, XP, ZP, XQ, ZQ, Xd, Zd):
    u = z.m(XP - ZP, XQ + ZQ)
    v = z.m(XP + ZP, XQ - ZQ)
    s, d = (u + v) % z.p, (u - v) % z.p
    return z.m(Zd, z.m(s, s)), z.m(Xd, z.m(d, d))


def xmul(z, P, n: int, kurve):
    """[n] P per Montgomery-Leiter; None = Unendlich."""
    if P is None or n == 0:
        return None
    z.leiter += n.bit_length()
    A24, C24 = kurve
    X, Z = P
    R0, R1 = (1, 0), (X, Z)
    for bit in bin(n)[2:]:
        if bit == "0":
            R1 = xadd(z, *R0, *R1, X, Z)
            R0 = xdbl(z, *R0, A24, C24)
        else:
            R0 = xadd(z, *R0, *R1, X, Z)
            R1 = xdbl(z, *R1, A24, C24)
    return None if R0[1] % z.p == 0 else R0


def isogenie_mehr(z, kurve, K, ell: int, punkte: list):
    """Isogenie vom Grad ell mit Kern <K>; schiebt alle Punkte mit.
    Bildpunkte: Costello-Hisil 2017. Bildkurve: Edwards-Form (Moody-Shumow,
    Meyer-Reith 2018): a' = a^l pi_Z^8, d' = d^l pi_Y^8 -- gegen die affine
    Formel aus isogenie_geld.py geprueft (Selbsttest: gleiche Kurven)."""
    A24, C24 = kurve
    P = z.p
    ks = [K]
    if ell > 3:
        ks.append(xdbl(z, *K, A24, C24))
    while len(ks) < (ell - 1) // 2:
        ks.append(xadd(z, *ks[-1], *K, *ks[-2]))
    summe = [(X + Z) % P for X, Z in ks]
    diff = [(X - Z) % P for X, Z in ks]
    piZ, piY = 1, 1
    for s_, d_ in zip(summe, diff):
        piZ, piY = z.m(piZ, s_), z.m(piY, d_)
    a, d = A24, (A24 - C24) % P
    p8 = lambda x: z.hoch(x, 8)
    a2 = z.m(z.hoch(a, ell), p8(piZ))
    d2 = z.m(z.hoch(d, ell), p8(piY))
    bilder = []
    for Pt in punkte:
        if Pt is None:
            bilder.append(None)
            continue
        X, Z = Pt
        pp, pm = (X + Z) % P, (X - Z) % P
        fx, fz = 1, 1
        for s_, d_ in zip(summe, diff):
            t0, t1 = z.m(pm, s_), z.m(pp, d_)
            fx, fz = z.m(fx, (t0 + t1) % P), z.m(fz, (t0 - t1) % P)
        Xn, Zn = z.m(X, z.m(fx, fx)), z.m(Z, z.m(fz, fz))
        bilder.append(None if Zn == 0 else (Xn, Zn))
    return (a2, (a2 - d2) % P), bilder


def runde_lehrbuch(z: Zaehler, A, Q, S: list, ells: tuple):
    """Wie isogenie_geld.wirkung: je Primzahl eine eigene Skalarmultiplikation."""
    k = 1
    for i in S:
        k *= ells[i]
    erledigt = []
    for i in S:
        if Q is None:
            break
        R = xmul(z, Q, k // ells[i], A)
        k //= ells[i]
        if R is None:
            continue
        A, (Q,) = isogenie_mehr(z, A, R, ells[i], [Q])
        erledigt.append(i)
    return A, erledigt


def strategie_optimal(n_max: int, leiter: float, schieb: float) -> list:
    """Optimale Teilung (dynamische Programmierung, wie die "optimalen
    Strategien" von De Feo-Jao-Plut, hier mit Durchschnittskosten):
    C(n) = min_k C(k) + C(n-k) + (n-k)*leiter + k*schieb.
    leiter: Mult. der Leiter je Primzahl in S2; schieb: Mult., um Q durch
    eine Isogenie von S1 zu schieben. Rueckgabe: teil[n] = bestes k."""
    C = [0.0] * (n_max + 1)
    teil = [0] * (n_max + 1)
    for n in range(2, n_max + 1):
        best, bk = None, 1
        for k in range(1, n):
            c = C[k] + C[n - k] + (n - k) * leiter + k * schieb
            if best is None or c < best:
                best, bk = c, k
        C[n], teil[n] = best, bk
    return teil, C


def runde_baum(z: Zaehler, A, Q, S: list, ells: tuple, schieben=None, teil=None):
    """Teile und herrsche. Q hat eine Ordnung, die prod(ells[S]) teilt.
    schieben: weitere Punkte, die durch alle Isogenien mitgehen."""
    schieben = list(schieben or [])
    if Q is None or not S:
        return A, [], schieben
    if len(S) == 1:
        i = S[0]
        A_neu, bilder = isogenie_mehr(z, A, Q, ells[i], schieben)
        return A_neu, [i], bilder
    mitte = teil[len(S)] if teil else len(S) // 2
    S1, S2 = S[:mitte], S[mitte:]
    k2 = 1
    for i in S2:
        k2 *= ells[i]
    T = xmul(z, Q, k2, A)          # Ordnung teilt prod(S1)
    A, e1, rest = runde_baum(z, A, T, S1, ells, [Q] + schieben, teil)
    Q2, schieben = rest[0], rest[1:]
    # Q2 = Bild von Q; die S1-Anteile sind weg, nur Teiler von prod(S2) bleiben
    # -- ausser, wo in S1 kein Kern gefunden wurde: dann muss Q2 diese
    # Faktoren noch abwerfen.
    fehlt = 1
    for i in S1:
        if i not in e1:
            fehlt *= ells[i]
    if fehlt > 1:
        Q2 = xmul(z, Q2, fehlt, A)
    A, e2, schieben = runde_baum(z, A, Q2, S2, ells, schieben, teil)
    return A, e1 + e2, schieben


def wirkung(p: int, ells: tuple, exponenten, rng: random.Random, strategie: str):
    """[l_1^e1 ... l_n^en] * E_0 mit gezaehlter, projektiver Arithmetik.
    Rueckgabe: affines A (zum Vergleich mit dem Original) und Zaehler."""
    iso.P, iso.ELLS = p, ells
    z = Zaehler(p)
    e = list(exponenten)
    kurve = (2, 4)  # A = 0, C = 1
    teil = None
    if strategie == "optimal":
        teil, _ = strategie_teilung(p.bit_length(), ells)
    while any(e):
        A24, C24 = kurve
        A = z.m((4 * A24 - 2 * C24) % p, z.inv(C24))  # eine Inversion je Runde
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
        Q = xmul(z, (x, 1), (p + 1) // k, kurve)
        if strategie in ("baum", "optimal"):
            kurve, erledigt, _ = runde_baum(z, kurve, Q, S, ells, None, teil)
        else:
            kurve, erledigt = runde_lehrbuch(z, kurve, Q, S, ells)
        for i in erledigt:
            e[i] -= s
    A24, C24 = kurve
    return (4 * A24 - 2 * C24) * pow(C24, p - 2, p) % p, z


# ---- Hochrechnung fuer kryptografische Groessen ----

def exponentenbereich(bits: int, s: int) -> int:
    """Kleinstes m mit (2m+1)^s >= 2^(bits/2) (Gruppe ueberdecken)."""
    m = 1
    while s * math.log2(2 * m + 1) < bits / 2:
        m += 1
    return m


LEITER_MUL = 12  # xDBL 6 + xADD 6 (oben gezaehlt)


def velu(l: int) -> float:
    """Mult. einer Isogenie vom Grad l, wie isogenie_mehr: Kernpunkte ~6d,
    Produkte 2d, Potenzen ~4 log2 l + 6 (d = (l-1)/2)."""
    d = (l - 1) // 2
    return 8 * d + 4 * math.log2(l) + 10


def velu_schieben(l: int) -> float:
    return 4 * ((l - 1) // 2) + 4  # je Kernpunkt 4 Mult., dann 2 Quadrate + 2


_C_SQRT = velu(101) / (math.sqrt(101) * math.log2(101))


def sqrt_velu(l: int) -> float:
    """Literatur-Modell (nachzulesen): ~ c sqrt(l) log2 l, Uebergang l ~ 100."""
    return min(velu(l), _C_SQRT * math.sqrt(l) * math.log2(l))


def sqrt_velu_schieben(l: int) -> float:
    return min(velu_schieben(l), velu_schieben(101) / (math.sqrt(101) * math.log2(101)) * math.sqrt(l) * math.log2(l))


def wanderung(bits: int, ells: list, strategie: str, iso=velu, schieb=velu_schieben) -> float:
    """Kosten einer Runde ueber ells (alle Kerne gefunden), Schritt fuer
    Schritt wie runde_lehrbuch / runde_baum -- nur gezaehlt, nicht gerechnet."""
    if not ells:
        return 0.0
    k_bits = sum(math.log2(l) for l in ells)
    kosten = LEITER_MUL * max(0.0, bits - k_bits)       # Kofaktor
    if strategie == "lehrbuch":
        rest = k_bits
        for l in ells:
            rest -= math.log2(l)
            kosten += LEITER_MUL * rest + iso(l) + schieb(l)
        return kosten

    def baum(S, stapel):
        if len(S) == 1:
            return iso(S[0]) + stapel * schieb(S[0])
        mitte = len(S) // 2
        S1, S2 = S[:mitte], S[mitte:]
        return (LEITER_MUL * sum(math.log2(l) for l in S2)
                + baum(S1, stapel + 1) + baum(S2, stapel))
    return kosten + baum(list(ells), 0)


def strategie_teilung(bits: int, ells, schieb=None):
    schieb = schieb or velu_schieben
    leiter = LEITER_MUL * sum(math.log2(l) for l in ells) / len(ells)
    q = sum(schieb(l) for l in ells) / len(ells)
    return strategie_optimal(len(ells), leiter, q)


def wanderung_optimal(bits: int, ells: list, iso=velu, schieb=velu_schieben) -> float:
    """Wie wanderung, mit optimaler Teilung (Durchschnittskosten)."""
    if not ells:
        return 0.0
    k_bits = sum(math.log2(l) for l in ells)
    _, C = strategie_teilung(bits, ells, schieb)
    return LEITER_MUL * max(0.0, bits - k_bits) + C[len(ells)] + sum(iso(l) for l in ells)


def exponenten_runden(ells: list, m: int):
    """Runde r = 1..m: je Vorzeichen die Primzahlen mit |e| >= r, bei
    Exponenten gleichverteilt in [-m, m] (gleichmaessig ausgeduennt)."""
    for r in range(1, m + 1):
        anteil = (m - r + 1) / (2 * m + 1)
        n = max(1, round(len(ells) * anteil))
        idx = [round(j * (len(ells) - 1) / max(1, n - 1)) for j in range(n)]
        yield [ells[i] for i in sorted(set(idx))]


def aktion_modell(bits: int, strategie: str, iso=velu, schieb=velu_schieben, m: int | None = None) -> float:
    ells = kosten.kleine_primzahlen(bits)
    if m is None:
        m = exponentenbereich(bits, len(ells))
    if strategie == "optimal":
        return sum(2 * wanderung_optimal(bits, aktiv, iso, schieb) for aktiv in exponenten_runden(ells, m))
    return sum(2 * wanderung(bits, aktiv, strategie, iso, schieb) for aktiv in exponenten_runden(ells, m))


def aktion_erwartet(bits: int, ells: list, ms: list, iso=velu, schieb=velu_schieben) -> float:
    """Erwartete Mult. einer Aktion mit Exponenten e_i gleichverteilt in
    [-m_i, m_i] (Baum-Strategie). Runde r, je Vorzeichen: Primzahl i ist mit
    Gewicht w = (m_i - r + 1)/(2 m_i + 1) dabei; Kosten linear in w."""
    gesamt = 0.0
    for r in range(1, max(ms) + 1):
        w = [(m - r + 1) / (2 * m + 1) if m >= r else 0.0 for m in ms]
        s_r = sum(w)
        if s_r == 0:
            continue
        k_bits = sum(wi * math.log2(l) for wi, l in zip(w, ells))
        tiefe = max(1.0, math.log2(max(s_r, 1.0)))
        runde = (LEITER_MUL * (bits - k_bits) + LEITER_MUL * k_bits / 2 * tiefe
                 + sum(wi * (iso(l) + tiefe / 2 * schieb(l)) for wi, l in zip(w, ells)))
        gesamt += 2 * runde
    return gesamt


def schranken_optimieren(bits: int, iso=velu, schieb=velu_schieben):
    """Exponentenschranken m_i je Primzahl: gierig die guenstigste
    Ueberdeckung je Kosten, bis prod(2 m_i + 1) >= 2^(bits/2)."""
    import heapq
    ells = kosten.kleine_primzahlen(bits)
    ms = [0] * len(ells)
    ziel = bits / 2
    abdeckung = 0.0
    runden = 0
    tiefe = math.log2(len(ells))
    runde_fix = 2 * LEITER_MUL * bits  # je neue Runde: Kofaktor-Leiter, je Vorzeichen

    def schritt(i):
        m = ms[i]
        d_cov = math.log2(2 * m + 3) - math.log2(2 * m + 1)
        e_alt = m * (m + 1) / (2 * m + 1)
        e_neu = (m + 1) * (m + 2) / (2 * m + 3)
        d_cost = (e_neu - e_alt) * (iso(ells[i]) + tiefe / 2 * schieb(ells[i]))
        if m + 1 > runden:
            d_cost += runde_fix
        return -d_cov / d_cost

    heap = [(schritt(i), i) for i in range(len(ells))]
    heapq.heapify(heap)
    while abdeckung < ziel:
        _, i = heapq.heappop(heap)
        abdeckung += math.log2(2 * ms[i] + 3) - math.log2(2 * ms[i] + 1)
        ms[i] += 1
        if ms[i] > runden:
            runden = ms[i]
            heap = [(schritt(j), j) for j in range(len(ells))]
            heapq.heapify(heap)
        else:
            heapq.heappush(heap, (schritt(i), i))
    return ells, ms


def pruef_toffoli(bits: int, mult_je_aktion: float) -> float:
    return math.log2(2 * mult_je_aktion * kosten.TOFFOLI_JE_MUL * bits * bits)


def selbsttest() -> None:
    rng = random.Random(7)
    # 1. Testprimzahlen sind echt.
    for k in (3,) + TESTPRIMZAHLEN:
        p, ells = csidh_primzahl(k)
        assert ist_primzahl(p) and p % 8 == 3, k
    # 2. Baum und Lehrbuch landen auf DERSELBEN Kurve wie das Original.
    for k in (3, 11, 19):
        p, ells = csidh_primzahl(k)
        for _ in range(4 if k < 19 else 2):
            e = [rng.choice((-2, -1, 0, 1, 2)) for _ in ells]
            iso.P, iso.ELLS = p, ells
            soll = iso.wirkung(0, e, random.Random(rng.random()))
            a1, z1 = wirkung(p, ells, e, random.Random(rng.random()), "lehrbuch")
            a2, z2 = wirkung(p, ells, e, random.Random(rng.random()), "baum")
            assert a1 == soll and a2 == soll, (k, e)
    # 2b. Optimale Strategie: ebenfalls dieselbe Kurve.
    p, ells = csidh_primzahl(19)
    e = [rng.choice((-1, 0, 1)) for _ in ells]
    iso.P, iso.ELLS = p, ells
    soll = iso.wirkung(0, e, random.Random(rng.random()))
    assert wirkung(p, ells, e, random.Random(rng.random()), "optimal")[0] == soll
    # 3. Der Baum spart Leiterschritte, und zwar mehr, je mehr Primzahlen.
    vorteil = []
    for k in TESTPRIMZAHLEN:
        p, ells = csidh_primzahl(k)
        e = [1] * len(ells)
        l1 = l2 = 0
        for _ in range(3):
            l1 += wirkung(p, ells, e, random.Random(rng.random()), "lehrbuch")[1].leiter
            l2 += wirkung(p, ells, e, random.Random(rng.random()), "baum")[1].leiter
        assert l2 < l1, (k, l1, l2)
        vorteil.append(l1 / l2)
    assert vorteil[0] < vorteil[-1], vorteil
    # 4. Die Kostenwanderung trifft die exakte Zaehlung (alle Exponenten +1,
    #    Kofaktor-Runde eingeschlossen; Abweichung < 20 %, das Modell liegt
    #    UNTER der Zaehlung -- guenstig fuer den Pruefer, also vorsichtig lesen).
    for k in TESTPRIMZAHLEN:
        p, ells = csidh_primzahl(k)
        e = [1] * len(ells)
        for strat in ("lehrbuch", "baum"):
            gez = []
            for _ in range(6):
                a_, z = wirkung(p, ells, e, random.Random(rng.random()), strat)
                gez.append(z.mul)
            gez = min(gez)  # Laeufe mit genau einer Runde (alle Kerne gefunden)
            mod = wanderung(p.bit_length(), list(ells), strat) + 1.5 * LEITER_MUL / 12 * 2 * p.bit_length()
            assert 0.85 < gez / mod < 1.2, (k, strat, gez, mod)
    # 5. Hochrechnung: Baum billiger, sqrt-Velu billiger, Ueberdeckung teurer.
    for bits in (4096, 8192):
        lb, bb = aktion_modell(bits, "lehrbuch", m=1), aktion_modell(bits, "baum", m=1)
        assert bb < lb, (bits, lb, bb)
        assert aktion_modell(bits, "baum", sqrt_velu, sqrt_velu_schieben, m=1) < bb
        assert aktion_modell(bits, "baum") > bb
        assert aktion_modell(bits, "optimal") <= aktion_modell(bits, "baum") * 1.001
    assert exponentenbereich(8192, 758) == 21
    # 6. Schranken je Primzahl: ueberdecken die Gruppe und sind billiger als
    #    gleiche Schranken; die erwartete Kostenformel trifft die Wanderung.
    for bits in (4096, 8192):
        ells, ms = schranken_optimieren(bits)
        assert sum(math.log2(2 * m + 1) for m in ms) >= bits / 2
        m = exponentenbereich(bits, len(ells))
        gleich = aktion_erwartet(bits, ells, [m] * len(ells))
        assert aktion_erwartet(bits, ells, ms) < gleich
        assert 0.7 < gleich / aktion_modell(bits, "baum") < 1.3
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    rng = random.Random(3)
    print("Exakt gezaehlt (gleiche Kurve wie das Original), Exponenten alle +1:")
    print("| kleine Primzahlen | p | Leiterschritte Lehrbuch | Baum | Mult. Lehrbuch | Baum |")
    print("|---|---|---|---|---|---|")
    for k in TESTPRIMZAHLEN:
        p, ells = csidh_primzahl(k)
        e = [1] * len(ells)
        r1 = [wirkung(p, ells, e, random.Random(rng.random()), "lehrbuch")[1] for _ in range(5)]
        r2 = [wirkung(p, ells, e, random.Random(rng.random()), "baum")[1] for _ in range(5)]
        f = lambda r, a: sum(getattr(z, a) for z in r) / len(r)
        print(f"| {k} | {p.bit_length()} Bit | {f(r1, 'leiter'):.0f} | {f(r2, 'leiter'):.0f} | {f(r1, 'mul'):.0f} | {f(r2, 'mul'):.0f} |")
    print("\nHochrechnung: Pruefen einer Note (2 Aktionen), Toffoli log2:")
    print("| Primzahl | m | bisher (m=1, Lehrbuch) | + Ueberdeckung (gegen uns) | + optimale Strategie | + Schranken je Primzahl | + sqrt-Velu (Literatur) | Hebel gesamt |")
    print("|---|---|---|---|---|---|---|---|")
    for bits in (4096, 8192, 17000):
        ells = kosten.kleine_primzahlen(bits)
        m = exponentenbereich(bits, len(ells))
        a = pruef_toffoli(bits, aktion_modell(bits, "lehrbuch", m=1))
        b = pruef_toffoli(bits, aktion_modell(bits, "lehrbuch"))
        c = pruef_toffoli(bits, aktion_modell(bits, "optimal"))
        e1, ms = schranken_optimieren(bits)
        gleich = aktion_erwartet(bits, e1, [m] * len(e1))
        d = c + math.log2(aktion_erwartet(bits, e1, ms) / gleich)
        e = pruef_toffoli(bits, aktion_modell(bits, "optimal", sqrt_velu, sqrt_velu_schieben)) + (d - c)
        print(f"| {bits} Bit | {m} | 2^{a:.1f} | 2^{b:.1f} | 2^{c:.1f} | 2^{d:.1f} | **2^{e:.1f}** | 2^{b - e:.1f} |")


if __name__ == "__main__":
    main()
