#!/usr/bin/env python3
"""Umkehrbar rechnen kostet: Zeit gegen Speicher beim Pruefen einer Note.

Forschung, kein Produkt (BAUVORGABE §2e). Keine Abhaengigkeiten.

Die Toffoli-Zahl in isogenie_kosten.py / isogenie_hebel.py tut so, als
verschwaenden Zwischenergebnisse von selbst. Im Quantenschaltkreis muss
jeder Zwischenstand entweder gehalten oder wieder ZURUECKGERECHNET werden
-- sonst bleibt Muell mit der Note verschraenkt und die Pruefung zerstoert
sie. Die Isogeniekette ist eine lange Folge von T Schritten, jeder mit einem
Zustand von S Qubits (Kurve, Punkte, Zwischenwerte). Klassischer Ausweg:
Bennetts Kieselspiel (1989) -- Zwischenstaende als "Kiesel" setzen und
wieder abraeumen.

Gebaut wird Bennetts b-faches Schema: die Strecke in b Teile, jeden Teil
rekursiv vorwaerts, dann die b-1 Zwischenkiesel rueckwaerts abraeumen.
  Kiesel  K = (b - 1) * d + 1,   d = ceil(log_b T)
  Zeit    ~ T * ((2b - 1) / b)^d   (Faktor gegenueber "Muell stehen lassen")
Der Selbsttest SPIELT das Spiel Zug fuer Zug (Regel: ein Kiesel darf nur
gesetzt oder entfernt werden, wenn sein Vorgaenger liegt) und prueft
Zeit und Kieselzahl gegen die Formel.

Die "2 Aktionen" der Pruefung (Hin- und Rueckweg der Gruppenaktion) sind
etwas anderes; das Abraeumen hier kommt obendrauf.

ERGEBNIS (main): bei 8.192 Bit kostet Umkehrbarkeit je nach Arbeitsspeicher
Faktor ~8 (7*10^8 logische Qubits) bis ~100 (7*10^6) -- die Pruefung liegt
damit bei 2^58 bis 2^62 Toffoli statt 2^55, und das PRUEFGERAET braucht
10^7 bis 10^9 logische Qubits Arbeitsspeicher. Bennett ist allgemein und
damit eine OBERE Schranke; gezielte Schaltkreise koennen besser sein.

Aufruf: python3 -I umkehrbar.py [--selbsttest]
"""
from __future__ import annotations

import math
import sys


class Spiel:
    """Kieselspiel auf der Kette 0 -> 1 -> ... -> T (Knoten 0 liegt immer)."""

    def __init__(self):
        self.liegt = {0}
        self.zuege = 0
        self.hoechst = 0

    def setze(self, i: int) -> None:
        assert i - 1 in self.liegt and i not in self.liegt, i
        self.liegt.add(i)
        self.zuege += 1
        self.hoechst = max(self.hoechst, len(self.liegt) - 1)

    def nimm(self, i: int) -> None:
        assert i - 1 in self.liegt and i in self.liegt, i
        self.liegt.remove(i)
        self.zuege += 1


def zuege(a: int, n: int, b: int) -> list[tuple[int, int]]:
    """Zuege (Knoten, +1 setzen / -1 nehmen), die bei liegendem a einen
    Kiesel auf a + n bringen und alle Zwischenkiesel wieder abraeumen --
    Bennett, b-fach rekursiv. Rueckwaerts gespielt raeumen sie a + n ab."""
    if n == 1:
        return [(a + 1, +1)]
    teil = math.ceil(n / b)
    grenzen = list(range(a + teil, a + n, teil)) + [a + n]
    starts = [a] + grenzen[:-1]
    out: list[tuple[int, int]] = []
    teile = [zuege(s_, e - s_, b) for s_, e in zip(starts, grenzen)]
    for t in teile:                       # vorwaerts bis a + n
        out += t
    for t in reversed(teile[:-1]):        # Zwischenkiesel abraeumen
        out += rueckwaerts(t)
    return out


def rueckwaerts(z: list[tuple[int, int]]) -> list[tuple[int, int]]:
    return [(k, -v) for k, v in reversed(z)]


def spiele(sp: Spiel, z: list[tuple[int, int]]) -> None:
    for k, v in z:
        (sp.setze if v > 0 else sp.nimm)(k)


def formel(T: int, b: int) -> tuple[int, float]:
    d = max(1, math.ceil(math.log(T, b) - 1e-9))
    return (b - 1) * d + 1, ((2 * b - 1) / b) ** d


def selbsttest() -> None:
    # 1. Das Spiel geht auf: am Ende liegt nur der Zielkiesel, Regeln
    #    eingehalten (assert in setze/nimm); Hin und Rueck raeumt alles ab.
    for T in (1, 2, 5, 8, 27, 64, 100):
        for b in (2, 3, 4):
            sp = Spiel()
            z = zuege(0, T, b)
            spiele(sp, z)
            assert sp.liegt == {0, T}, (T, b, sp.liegt)
            spiele(sp, rueckwaerts(z))
            assert sp.liegt == {0}
    # 2. Fuer T = b^d trifft das Spiel die Formel: Kiesel genau, Zeit genau.
    for b, d in ((2, 6), (3, 4), (4, 3), (8, 2)):
        T = b ** d
        sp = Spiel()
        spiele(sp, zuege(0, T, b))
        k, f = formel(T, b)
        assert sp.hoechst == k, (b, d, sp.hoechst, k)
        assert abs(sp.zuege - T * f) < 1e-6 * T * f, (b, d, sp.zuege, T * f)
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    # Pruefen einer Note bei 8.192 / 17.000 Bit (isogenie_hebel.py: ~2^55,1 /
    # 2^59,3 Toffoli, je Multiplikation 4 B^2): Schritte ~ Multiplikationen,
    # Ein Schritt = ein Leiterschritt (~12 Multiplikationen; seine eigenen
    # Zwischenwerte raeumt er selbst ab: Faktor 2). Zustand je Kiesel ~ 10 B
    # Qubits (Kurve, zwei Punkte, Zwischenwerte).
    for bits, log_tof in ((8192, 55.1), (17000, 59.3)):
        mult = 2 ** log_tof / (4 * bits * bits)
        T = int(mult / 12)
        S = 10 * bits
        print(f"\n{bits} Bit: ~2^{math.log2(T):.1f} Schritte, Zustand ~{S:,} Qubits je Kiesel".replace(",", "."))
        print("| b | Kiesel | logische Qubits (Arbeitsspeicher) | Zeitfaktor (x2 lokal) | Toffoli gesamt |")
        print("|---|---|---|---|---|")
        for b in (2, 4, 16, 64, 256, 4096):
            k, f = formel(T, b)
            f *= 2
            print(f"| {b} | {k} | {k * S:.1e} | {f:.0f}x (2^{math.log2(f):.1f}) | 2^{log_tof + math.log2(f):.1f} |")


if __name__ == "__main__":
    main()
