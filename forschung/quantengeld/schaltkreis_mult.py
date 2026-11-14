#!/usr/bin/env python3
"""Hebel 2: Multiplizieren im Quantenschaltkreis -- Schulbuch gegen Karatsuba.

Forschung, kein Produkt (BAUVORGABE §2e, §5.4). Keine Abhaengigkeiten.

Jede Pruefung einer Note besteht aus ~2^26 Koerpermultiplikationen
(isogenie_hebel.py); bisher angesetzt: 4*B^2 Toffoli je Multiplikation
(Schulbuch). Hier wird der Multiplizierer WIRKLICH gebaut -- als umkehrbarer
Schaltkreis aus X, CNOT und Toffoli -- und Gatter fuer Gatter simuliert:

  - Addierer: Cuccaro u. a. 2004 (2n Toffoli, ein Hilfsbit).
  - Subtraktion: b - a = ~(~b + a).
  - Schulbuch: out += a*b als n gesteuerte Additionen; das UND aus
    Steuerbit und a wird in Hilfsbits berechnet und danach per Messung
    entfernt (Gidney 2018: Abbau eines UND kostet kein Toffoli; im
    Rechenbasis-Test exakt nachgebildet).
  - Karatsuba umkehrbar (Idee nach Gidney 2019, nachzulesen): mit
    a*b = (1 - 2^h)(z0 - 2^h z2) + 2^h z1' reichen DREI rekursive
    Aufrufe, und das Ergebnisregister wird vorher mit (1 - 2^h)^-1 und
    nachher mit (1 - 2^h) multipliziert -- beides umkehrbar, weil
    1 - 2^h ungerade ist. Kein Muell bleibt zurueck.

Der Selbsttest prueft fuer zufaellige Eingaben: out + a*b stimmt, a und b
unveraendert, ALLE Hilfsbits wieder 0. Eine Zaehlfassung (dieselben
Funktionen ohne Gatter) trifft die Simulation exakt und rechnet auf
B = 8.192 hoch.

ERGEBNIS (14.11.2026, gemessen):
  - Schulbuch: genau ~4,0 * n^2 Toffoli -- die bisherige Annahme 4*B^2
    (isogenie_kosten.py) ist bestaetigt.
  - Karatsuba in dieser einfachen, muellfreien Bauart: KEIN Gewinn, schon
    eine Stufe kostet 1,38x mehr (n = 256 ... 8.192). Grund: jede
    Teilrechnung arbeitet auf dem ganzen oberen Rest des Ergebnisregisters,
    damit Uebertraege ankommen; die Additionen werden so lang wie das
    Register, nicht wie die Teilzahlen.
  - Gewinn braucht eine Uebertragsbehandlung mit Fenstern (Literatur:
    Gidney 2019, nachzulesen). Offen -- bis dahin rechnen wir mit 4*B^2.

Aufruf: python3 -I schaltkreis_mult.py [--selbsttest]
"""
from __future__ import annotations

import math
import random
import sys


class Kreis:
    """Umkehrbarer Schaltkreis. simulieren=False: nur zaehlen."""

    def __init__(self, simulieren: bool = True):
        self.sim = simulieren
        self.bits: list[int] = []
        self.frei: list[int] = []
        self.toffoli = 0
        self.und_abbau = 0   # per Messung entfernte UND (kein Toffoli)
        self.hoechst = 0     # groesste Zahl gleichzeitig belegter Bits

    # ---- Register ----
    def belegt(self) -> int:
        return len(self.bits) - len(self.frei)

    def spitze(self, extra: int) -> None:
        """Zaehlfassung: kurzzeitig 'extra' Hilfsbits mehr belegt."""
        self.hoechst = max(self.hoechst, self.belegt() + extra)

    def neu(self, n: int) -> list[int]:
        out = []
        for _ in range(n):
            if self.frei:
                out.append(self.frei.pop())
            else:
                self.bits.append(0)
                out.append(len(self.bits) - 1)
        self.hoechst = max(self.hoechst, len(self.bits) - len(self.frei))
        return out

    def weg(self, reg: list[int]) -> None:
        if self.sim:
            assert all(self.bits[i] == 0 for i in reg), "Hilfsbit nicht sauber"
        self.frei.extend(reg)

    def setze(self, reg: list[int], wert: int) -> None:
        for j, i in enumerate(reg):
            self.bits[i] = (wert >> j) & 1

    def lies(self, reg: list[int]) -> int:
        return sum(self.bits[i] << j for j, i in enumerate(reg))

    # ---- Gatter ----
    def x(self, t):
        if self.sim:
            self.bits[t] ^= 1

    def cx(self, c, t):
        if self.sim:
            self.bits[t] ^= self.bits[c]

    def ccx(self, a, b, t):
        self.toffoli += 1
        if self.sim:
            self.bits[t] ^= self.bits[a] & self.bits[b]

    def und_weg(self, a, b, t):
        """Gidney: t = a AND b messen und entfernen, kein Toffoli."""
        self.und_abbau += 1
        if self.sim:
            assert self.bits[t] == self.bits[a] & self.bits[b]
            self.bits[t] = 0


# ---- Addieren ----

def addiere(k: Kreis, a: list[int], b: list[int]) -> None:
    """b += a (mod 2^len(b)), len(a) <= len(b); a bleibt. Cuccaro-Addierer,
    a mit Null-Hilfsbits auf len(b) aufgefuellt: 2*len(b) Toffoli."""
    n = len(b)
    if n == 0:
        return
    if not k.sim:
        k.toffoli += 2 * n
        k.spitze(max(0, n - len(a)) + 1)
        return
    pad = k.neu(n - len(a)) if len(a) < n else []
    a = list(a[:n]) + pad
    c = k.neu(1)[0]
    kette = [c] + a[:-1]
    for i in range(n):                      # MAJ
        k.cx(a[i], b[i])
        k.cx(a[i], kette[i])
        k.ccx(kette[i], b[i], a[i])
    for i in reversed(range(n)):            # UMA
        k.ccx(kette[i], b[i], a[i])
        k.cx(a[i], kette[i])
        k.cx(kette[i], b[i])
    k.weg([c])
    k.weg(pad)


def nicht(k: Kreis, reg: list[int]) -> None:
    if not k.sim:
        return  # X-Gatter werden nicht gezaehlt
    for i in reg:
        k.x(i)


def subtrahiere(k: Kreis, a: list[int], b: list[int]) -> None:
    """b -= a: b = ~(~b + a)."""
    nicht(k, b)
    addiere(k, a, b)
    nicht(k, b)


def gesteuert_addiere(k: Kreis, ctl: int, a: list[int], b: list[int]) -> None:
    """Wenn ctl: b += a. UND in Hilfsbits, Abbau per Messung."""
    a = a[:len(b)]
    if not a:
        return
    if not k.sim:   # gleiche Zaehlung wie unten, ohne Gatter einzeln
        n = len(b)
        k.toffoli += len(a) + 2 * n
        k.und_abbau += len(a)
        k.spitze(len(a) + max(0, n - len(a)) + 1)
        return
    t = k.neu(len(a))
    for ai, ti in zip(a, t):
        k.ccx(ctl, ai, ti)
    addiere(k, t, b)
    for ai, ti in zip(a, t):
        k.und_weg(ctl, ai, ti)
    k.weg(t)


# ---- Multiplizieren ----

def schulbuch(k: Kreis, out: list[int], a: list[int], b: list[int]) -> None:
    """out += a*b (mod 2^len(out))."""
    for i, bi in enumerate(b):
        if i >= len(out):
            break
        gesteuert_addiere(k, bi, a, out[i:])


def mal_inv_1_minus_2h(k: Kreis, out: list[int], h: int) -> None:
    """out <- out * (1 - 2^h)^-1: y = x + (y << h), Block fuer Block von unten."""
    K = len(out)
    j = 1
    while j * h < K:
        addiere(k, out[(j - 1) * h: j * h], out[j * h:])
        j += 1


def mal_1_minus_2h(k: Kreis, out: list[int], h: int) -> None:
    """out <- out * (1 - 2^h): x - (x << h), Block fuer Block von oben."""
    K = len(out)
    j = (K - 1) // h
    while j >= 1:
        subtrahiere(k, out[(j - 1) * h: j * h], out[j * h:])
        j -= 1


def karatsuba(k: Kreis, out: list[int], a: list[int], b: list[int], basis: int, vorzeichen: int = 1) -> None:
    """out += vorzeichen * a*b (mod 2^len(out)), umkehrbar, ohne Muell."""
    if vorzeichen < 0:
        nicht(k, out)
        karatsuba(k, out, a, b, basis, 1)
        nicht(k, out)
        return
    n = max(len(a), len(b))
    if n <= basis or len(out) <= basis:
        schulbuch(k, out, a, b)
        return
    h = n // 2
    a0, a1 = a[:h], a[h:]
    b0, b1 = b[:h], b[h:]
    mal_inv_1_minus_2h(k, out, h)
    karatsuba(k, out, a0, b0, basis, 1)          # + z0
    karatsuba(k, out[h:], a1, b1, basis, -1)     # - 2^h z2
    mal_1_minus_2h(k, out, h)
    # Summen in place: a1 += a0, b1 += b0 (je ein Uebertragsbit)
    ca, cb = k.neu(1), k.neu(1)
    sa, sb = a1 + ca, b1 + cb
    addiere(k, a0, sa)
    addiere(k, b0, sb)
    karatsuba(k, out[h:], sa, sb, basis, 1)      # + 2^h z1'
    subtrahiere(k, a0, sa)
    subtrahiere(k, b0, sb)
    k.weg(ca)
    k.weg(cb)


def multipliziere(n: int, verfahren: str, basis: int = 0, simulieren: bool = False,
                  rng: random.Random | None = None):
    """Ein n x n -> 2n Produkt. Rueckgabe: Kreis (mit Zaehlern)."""
    k = Kreis(simulieren)
    a, b, out = k.neu(n), k.neu(n), k.neu(2 * n)
    if simulieren:
        va, vb, vo = rng.getrandbits(n), rng.getrandbits(n), rng.getrandbits(2 * n)
        k.setze(a, va)
        k.setze(b, vb)
        k.setze(out, vo)
    if verfahren == "schulbuch":
        schulbuch(k, out, a, b)
    else:
        karatsuba(k, out, a, b, basis)
    if simulieren:
        assert k.lies(out) == (vo + va * vb) % (1 << (2 * n)), "falsches Produkt"
        assert k.lies(a) == va and k.lies(b) == vb, "Eingabe veraendert"
        belegt = set(a) | set(b) | set(out)
        assert all(k.bits[i] == 0 for i in range(len(k.bits)) if i not in belegt), "Muell"
    return k


def selbsttest() -> None:
    rng = random.Random(11)
    # 1. Addierer und Subtrahierer, auch mit kuerzerem a.
    for n in (1, 2, 5, 9):
        for m in range(1, n + 1):
            k = Kreis()
            a, b = k.neu(m), k.neu(n)
            for _ in range(20):
                va, vb = rng.getrandbits(m), rng.getrandbits(n)
                k.setze(a, va)
                k.setze(b, vb)
                addiere(k, a, b)
                assert k.lies(b) == (va + vb) % (1 << n) and k.lies(a) == va
                subtrahiere(k, a, b)
                assert k.lies(b) == vb
    # 2. Beide Multiplizierer rechnen richtig, ohne Muell -- fuer viele Groessen
    #    und Basisgroessen.
    for n in (1, 2, 3, 7, 8, 13, 16, 33, 64):
        for _ in range(3):
            multipliziere(n, "schulbuch", simulieren=True, rng=rng)
            for basis in {2, 4, max(2, n // 4)}:
                if n <= 16 or basis >= n // 8:
                    multipliziere(n, "karatsuba", basis, simulieren=True, rng=rng)
    # 3. Die Zaehlfassung trifft die Simulation exakt.
    for n in (8, 31, 64, 129):
        for verf, basis in (("schulbuch", 0), ("karatsuba", max(4, n // 8)), ("karatsuba", max(4, n // 2))):
            s = multipliziere(n, verf, basis, simulieren=True, rng=rng)
            z = multipliziere(n, verf, basis)
            assert (s.toffoli, s.und_abbau) == (z.toffoli, z.und_abbau), (n, verf, basis)
    # 4. Schulbuch kostet ~4 n^2 (bestaetigt die Annahme in isogenie_kosten.py);
    #    die einfache Karatsuba-Bauart gewinnt NICHT (Ergebnis, s. Kopf).
    for n in (256, 1024, 4096):
        s = multipliziere(n, "schulbuch").toffoli
        assert 3.9 < s / n / n < 4.1, (n, s)
        assert multipliziere(n, "karatsuba", n // 2).toffoli > s, n
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    print("Toffoli je Produkt n x n -> 2n (gezaehlt; bis n = 129 Gatter fuer Gatter simuliert):")
    print("| n (Bit) | Schulbuch | / n^2 | Karatsuba 1 Stufe | 2 Stufen | 3 Stufen |")
    print("|---|---|---|---|---|---|")
    for n in (256, 1024, 4096, 8192):
        s = multipliziere(n, "schulbuch").toffoli
        r = [multipliziere(n, "karatsuba", n >> st).toffoli / s for st in (1, 2, 3)]
        print(f"| {n} | 2^{math.log2(s):.1f} | {s / n / n:.2f} | {r[0]:.2f}x | {r[1]:.2f}x | {r[2]:.2f}x |")


if __name__ == "__main__":
    main()
