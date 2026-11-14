#!/usr/bin/env python3
"""Reicht eine schwache Pruefung? (BAUVORGABE §5.5) -- gemessen: NEIN.

Forschung, kein Produkt. Keine Abhaengigkeiten.

VOLLE PRUEFUNG: Projektion auf |$_c> (Phasenschaetzung ueber die ganze
Gruppe; Exponenten bis m = 21 bei 8.192 Bit, je Aktion m Runden, 2 Aktionen).

SCHWACHE PRUEFUNG: je Erzeuger l_i t Hadamard-Tests mit einer einzigen
gesteuerten l_i-Isogenie: annehmen mit (1 + Re lambda_i^* <psi|U_i|psi>)/2,
lambda_i = Eigenwert von U_i auf |$_c>. Eine Runde ueber alle Erzeuger
kostet etwa eine Runde einer Aktion -- billig, wenn t klein ist.

DER ANGRIFF: ein GLATTER Zustand -- eine Gaussglocke ueber Exponenten-
vektoren e mit Breite sigma, mit den Phasen der Note:
  |phi> = sum_e exp(-|e|^2 / (4 sigma^2)) * omega^(c <e,a>) |l^e E_0>.
Er besteht jeden Einzeltest fast (Ueberlappung exp(-1/(8 sigma^2))), und
erzeugen kann ihn, wer e aus der Kurve l^e E_0 zurueckrechnet -- auf einem
Traeger von ~ (sigma sqrt(2 pi e))^n Elementen statt der ganzen Gruppe.
Die volle Pruefung laesst ihn mit ~ Traeger/N durch.

ERGEBNIS: Damit glatte Faelschungen mit Traeger <= 2^-128 * N (dieselbe
Annahme wie fuer die volle Pruefung) hoechstens mit 2^-128 durchkommen,
braucht die schwache Pruefung bei 8.192 Bit ~150 Runden -- die volle 42.
Die schwache Pruefung ist TEURER. §5.5 ist damit geschlossen.

Kleine Gruppe: eine Glocke mit sigma = 0,4 besteht die schwache Pruefung
(1 Runde) mit 0,16, die volle mit 0,015 -- 11x besser; die Produktformel
trifft die exakte Simulation (sigma >= 0,5 liegt sie darunter: nach jedem
bestandenen Test wird die Glocke glatter, das hilft dem Faelscher).

Teil 1 rechnet das an der echten kleinen Gruppe (p = 868019, h = 1491,
Zustaende exakt simuliert, Tests als Kraus-Operatoren nacheinander);
Teil 2 rechnet hoch.

Aufruf: python3 -I schwache_pruefung.py [--selbsttest]
"""
from __future__ import annotations

import cmath
import importlib.util
import itertools
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
hebel = _lade("isogenie_hebel")

P, ELLS = 868019, (3, 5, 17, 23, 37)


class Gruppe:
    """Die Bahn als Zyklus: Index k <-> Kurve g^k E_0; a_i = Index von l_i."""

    def __init__(self, rng: random.Random):
        iso.P, iso.ELLS = P, ELLS
        self.geld = iso.Geld(rng)
        self.h = self.geld.h
        self.a = []
        for i in range(len(ELLS)):
            e = [0] * len(ELLS)
            e[i] = 1
            self.a.append(self.geld.index[iso.wirkung(0, e, rng)])

    def w(self, x: float) -> complex:
        return cmath.exp(2j * math.pi * x / self.h)

    def note(self, c: int) -> list:
        return [self.w(c * k) / math.sqrt(self.h) for k in range(self.h)]

    def eigenwert(self, c: int, i: int) -> complex:
        # U_i |k> = |k + a_i>  =>  U_i |$_c> = omega^(-c a_i) |$_c>
        return self.w(-c * self.a[i])

    def U(self, psi: list, i: int) -> list:
        h, a = self.h, self.a[i]
        out = [0j] * h
        for k, amp in enumerate(psi):
            out[(k + a) % h] += amp
        return out

    def glocke(self, c: int, sigma: float) -> list:
        """Gaussglocke ueber Exponentenvektoren, mit den Phasen von |$_c>."""
        r = max(1, int(4 * sigma))
        psi = [0j] * self.h
        for e in itertools.product(range(-r, r + 1), repeat=len(ELLS)):
            k = sum(ei * ai for ei, ai in zip(e, self.a)) % self.h
            psi[k] += math.exp(-sum(x * x for x in e) / (4 * sigma * sigma))
        psi = [amp * self.w(c * k) for k, amp in enumerate(psi)]
        n = math.sqrt(sum(abs(x) ** 2 for x in psi))
        return [x / n for x in psi]

    def voll(self, c: int, psi: list) -> float:
        ziel = self.note(c)
        return abs(sum(z.conjugate() * x for z, x in zip(ziel, psi))) ** 2

    def schwach(self, c: int, psi: list, t: int) -> float:
        """t Runden; je Runde ein Hadamard-Test je Erzeuger, nacheinander.
        Annehmen = Kraus K0 = (I + lambda^* U)/2. Rueckgabe: P(alle bestanden)."""
        for _ in range(t):
            for i in range(len(ELLS)):
                lam = self.eigenwert(c, i).conjugate()
                Upsi = self.U(psi, i)
                psi = [(x + lam * y) / 2 for x, y in zip(psi, Upsi)]
        return sum(abs(x) ** 2 for x in psi)


def ueberlappung(sigma: float) -> float:
    """<phi|verschoben um 1|phi> der diskreten Glocke in EINER Koordinate
    (fuer grosse sigma ~ exp(-1/(8 sigma^2)))."""
    r = int(10 * sigma) + 5
    a = [math.exp(-e * e / (4 * sigma * sigma)) for e in range(-r, r + 2)]
    return sum(x * y for x, y in zip(a, a[1:])) / sum(x * x for x in a)


def schwach_produkt(sigma: float, tests: int) -> float:
    """Produktformel (Einzeltests unabhaengig): untere Schranke fuer den
    Faelscher -- nach jedem bestandenen Test wird die Glocke glatter."""
    return ((1 + ueberlappung(sigma)) / 2) ** tests


# ---- Hochrechnung ----

def traeger_bits(sigma: float, n: int) -> float:
    return n * math.log2(sigma * math.sqrt(2 * math.pi * math.e))


def runden_schwach(bits: int, lam: int = 128) -> tuple[int, float]:
    """Kleinste Rundenzahl t, so dass jede Glocke mit Traeger <= N * 2^-lam
    (log2 N = bits/2) hoechstens mit 2^-lam besteht (Produktformel)."""
    n = len(kosten.kleine_primzahlen(bits))
    ziel = bits / 2 - lam
    sigma = 2 ** (ziel / n) / math.sqrt(2 * math.pi * math.e)
    je_test = -math.log2((1 + ueberlappung(sigma)) / 2)
    return math.ceil(lam / je_test / n), sigma


def runden_voll(bits: int) -> int:
    n = len(kosten.kleine_primzahlen(bits))
    return 2 * hebel.exponentenbereich(bits, n)  # 2 Aktionen zu m Runden


def selbsttest() -> None:
    rng = random.Random(1491)
    g = Gruppe(rng)
    assert g.h == 1491
    # 1. Die Zyklus-Darstellung stimmt mit echten Isogenien: l_i verschiebt
    #    jeden Index um a_i.
    for i in range(len(ELLS)):
        e = [0] * len(ELLS)
        e[i] = 1
        for k in rng.sample(range(g.h), 4):
            assert g.geld.index[iso.wirkung(g.geld.kurven[k], e, rng)] == (k + g.a[i]) % g.h
    c = rng.randrange(g.h)
    # 2. Die echte Note besteht beide Pruefungen sicher.
    note = g.note(c)
    assert abs(g.voll(c, note) - 1) < 1e-9 and abs(g.schwach(c, note, 3) - 1) < 1e-9
    # 3. Eine andere Note (fremde Seriennummer) faellt bei beiden durch
    #    (bei nur 5 Erzeugern ist die schwache Pruefung entsprechend grob).
    d = (c + 700) % g.h
    assert g.voll(c, g.note(d)) < 1e-9 and g.schwach(c, g.note(d), 3) < 0.5
    # 4. Glatte Faelschung: besteht die SCHWACHE Pruefung weit besser als die
    #    volle, und mindestens so gut wie die Produktformel sagt.
    #    (Die kleine Gruppe hat nur h = 1491 Elemente: ab sigma ~ 1 deckt die
    #    Glocke sie ganz ab, dann besteht sie auch voll -- daher kleine sigma.)
    for sigma in (0.4, 0.5, 0.6, 0.7):
        phi = g.glocke(c, sigma)
        for t in (1, 3):
            s = g.schwach(c, phi, t)
            assert s >= 0.95 * schwach_produkt(sigma, t * len(ELLS)), (sigma, t, s)
        assert g.schwach(c, phi, 1) > 1.9 * g.voll(c, phi), sigma
    assert abs(ueberlappung(9.0) - math.exp(-1 / (8 * 81))) < 1e-9
    # 5. Hochrechnung: die schwache Pruefung braucht MEHR Runden als die volle.
    for bits in (4096, 8192, 17000):
        t, _ = runden_schwach(bits)
        assert t > runden_voll(bits), (bits, t, runden_voll(bits))
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    rng = random.Random(1491)
    g = Gruppe(rng)
    c = rng.randrange(g.h)
    print(f"Echte kleine Gruppe p = {P}, h = {g.h}, 5 Erzeuger; Seriennummer c = {c}.")
    print("| Zustand | volle Pruefung | schwach, 1 Runde | 3 Runden | Produktformel (1 Runde) |")
    print("|---|---|---|---|---|")
    print(f"| echte Note | {g.voll(c, g.note(c)):.3f} | {g.schwach(c, g.note(c), 1):.3f} | {g.schwach(c, g.note(c), 3):.3f} | 1 |")
    d = (c + 700) % g.h
    print(f"| fremde Note | {g.voll(c, g.note(d)):.3f} | {g.schwach(c, g.note(d), 1):.3f} | {g.schwach(c, g.note(d), 3):.3f} | - |")
    for sigma in (0.4, 0.5, 0.7, 1.0, 1.5):
        phi = g.glocke(c, sigma)
        print(f"| Glocke sigma = {sigma} | {g.voll(c, phi):.3f} | {g.schwach(c, phi, 1):.3f} | {g.schwach(c, phi, 3):.3f} | {schwach_produkt(sigma, len(ELLS)):.3f} |")
    print("\nHochrechnung (2^-128 gegen Glocken mit Traeger <= 2^-128 * N):")
    print("| Primzahl | Erzeuger | groesste erlaubte Glocke sigma | Runden schwach | Runden voll | schwach / voll |")
    print("|---|---|---|---|---|---|")
    for bits in (4096, 8192, 17000):
        t, sigma = runden_schwach(bits)
        v = runden_voll(bits)
        n = len(kosten.kleine_primzahlen(bits))
        print(f"| {bits} Bit | {n} | {sigma:.1f} | {t} | {v} | {t / v:.1f}x |")


if __name__ == "__main__":
    main()
