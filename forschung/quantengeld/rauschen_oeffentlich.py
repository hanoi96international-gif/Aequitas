#!/usr/bin/env python3
"""Oeffentlich pruefbares Quantengeld unter Rauschen -- der Preis der Oeffentlichkeit.

Forschung, kein Produkt (README.md, Abschnitt 3b). Keine Abhaengigkeiten.

MODELL. Die Note des Gruppenaktions-Verfahrens (zhandry_spielzeug.py) ist ein
Fourier-Zustand auf einem Register aus m Qubits (N = 2^m):
    |$_y> = 1/sqrt(N) * Summe_x  w^(x*y) |x>,   w = exp(2 pi i / N).
Geprueft wird OEFFENTLICH durch Phasenschaetzung -- eine Projektion auf |$_y>.
Anders als bei Wiesner gibt es keine Mehrheit ueber viele Qubits: die Note
besteht genau mit Wahrscheinlichkeit |<$_y| rho |$_y>|.

RAUSCHEN. Jedes Qubit erleidet waehrend der Lagerzeit unabhaengig einen
Pauli-Fehler (Depolarisation): X, Y, Z je mit q/3. Dieselbe Groesse wie in
rauschen.py: die Kippwahrscheinlichkeit p = 2q/3 = (1 - exp(-t/T)) / 2.

SATZ (exakt, hier hergeleitet und im Selbsttest am Zustandsvektor geprueft).
Fuer jede Seriennummer y gilt
    P[echte Note besteht] = (1 - p)^m.
Beweis-Skizze: <$_y| X^a Z^b |$_y> zerfaellt in ein Produkt ueber die Bits,
weil (x XOR 2^j) - x = 2^j (1 - 2 x_j). Je Bit: I -> 1, X -> cos(t_j),
Z -> 0, Y -> i sin(t_j) mit t_j = 2 pi y 2^j / N. Erwartung je Bit:
(1-q) + (q/3)(cos^2 + 0 + sin^2) = 1 - 2q/3 = 1 - p.

FOLGERUNG. Fuer >= 99 % muss p <= 1 - 0,99^(1/m) ~ 0,01/m sein -- jedes
einzelne Qubit muss also m-mal besser sein als bei Wiesner. Mit Isogenien
(m in der Groessenordnung hunderter bis tausender Qubits) heisst das ohne
Fehlerkorrektur: Kohaerenz zehntausendfach laenger als die Lagerzeit.

AUSWEG IM MODELL: Mehrteilige Note. k unabhaengige Teilnoten, angenommen wird
bei mindestens tau*k bestandenen. Gegen den Aufteilungsangriff (ein Faelscher
verteilt die echten Teilnoten auf zwei Noten) muss tau > 1/2 sein -- dann
brauchte er mehr als k echte Teilnoten. Eine Teilnote muss dann nur noch mit
etwa 60-70 % bestehen statt mit 99 %. Das senkt die Forderung um rund das
50- bis 100-fache, bleibt aber hunderte Male ueber Wiesner.

Aufruf: python3 -I rauschen_oeffentlich.py [--selbsttest]
"""
from __future__ import annotations

import cmath
import math
import random
import sys

EHRLICH_MIN = 0.99
FAELSCHUNG_MAX = 1e-12


def bestehen(m: int, p: float) -> float:
    """Exakt: P[echte Note mit m Qubits besteht] bei Kippwahrscheinlichkeit p je Qubit."""
    return (1.0 - p) ** m


def p_aus_zeit(t: float, kohaerenz: float) -> float:
    return (1.0 - math.exp(-t / kohaerenz)) / 2.0


def kohaerenz_fuer(p: float, t: float = 1.0) -> float:
    """T, bei dem nach Lagerzeit t die Kippwahrscheinlichkeit p ist."""
    return -t / math.log(1.0 - 2.0 * p)


def binom_sf(n: int, k: int, s: float) -> float:
    """P[Bin(n, s) >= k]."""
    if k <= 0:
        return 1.0
    if k > n:
        return 0.0
    if s <= 0.0:
        return 0.0
    if s >= 1.0:
        return 1.0
    ls, lf = math.log(s), math.log1p(-s)
    log_c = [0.0] * (n + 1)
    for i in range(1, n + 1):
        log_c[i] = log_c[i - 1] + math.log(n - i + 1) - math.log(i)
    terme = [log_c[i] + i * ls + (n - i) * lf for i in range(k, n + 1)]
    mx = max(terme)
    return min(1.0, math.exp(mx) * sum(math.exp(x - mx) for x in terme))


def einteilig(m: int) -> float:
    """Noetige Kohaerenz (Vielfaches der Lagerzeit) fuer eine einteilige Note."""
    p = 1.0 - EHRLICH_MIN ** (1.0 / m)
    return kohaerenz_fuer(p)


def mehrteilig(m: int, k: int) -> tuple[float, int, float]:
    """Mehrteilige Note: k Teilnoten, Schwelle tau*k mit tau > 1/2 (gegen den
    Aufteilungsangriff). Gesucht: kleinste Teil-Erfolgswahrscheinlichkeit s,
    mit der die echte Note >= 99 % besteht. Rueckgabe (T/t, Schwelle, s)."""
    schwelle = k // 2 + 1  # mehr als die Haelfte
    lo, hi = 0.5, 1.0
    for _ in range(60):
        mid = (lo + hi) / 2
        if binom_sf(k, schwelle, mid) >= EHRLICH_MIN:
            hi = mid
        else:
            lo = mid
    s = hi
    p = 1.0 - s ** (1.0 / m)
    return kohaerenz_fuer(p), schwelle, s


# ---- Zustandsvektor-Pruefung (kleine m) ----

def note(m: int, y: int) -> list[complex]:
    n = 1 << m
    return [cmath.exp(2j * math.pi * x * y / n) / math.sqrt(n) for x in range(n)]


def pauli(z: list[complex], j: int, art: str) -> list[complex]:
    """Pauli X, Y oder Z auf Bit j."""
    out = [0j] * len(z)
    b = 1 << j
    for x, a in enumerate(z):
        bit = (x >> j) & 1
        if art == "X":
            out[x ^ b] += a
        elif art == "Z":
            out[x] += -a if bit else a
        else:  # Y = i X Z
            out[x ^ b] += 1j * (-a if bit else a)
    return out


def bestehen_simuliert(m: int, y: int, p: float, laeufe: int, rng: random.Random) -> float:
    q = 1.5 * p
    psi = note(m, y)
    summe = 0.0
    for _ in range(laeufe):
        z = psi
        for j in range(m):
            r = rng.random()
            if r < q:
                z = pauli(z, j, "XYZ"[int(3 * r / q) if r < q else 0])
        ueberlapp = sum(a.conjugate() * b for a, b in zip(psi, z))
        summe += abs(ueberlapp) ** 2
    return summe / laeufe


def bestehen_exakt_alle_fehler(m: int, y: int, p: float) -> float:
    """Exakt durch Aufzaehlen aller 4^m Pauli-Fehler (nur sehr kleine m)."""
    q = 1.5 * p
    psi = note(m, y)
    gesamt = 0.0

    def rek(j: int, z: list[complex], w: float) -> None:
        nonlocal gesamt
        if j == m:
            u = sum(a.conjugate() * b for a, b in zip(psi, z))
            gesamt += w * abs(u) ** 2
            return
        rek(j + 1, z, w * (1 - q))
        for art in "XYZ":
            rek(j + 1, pauli(z, j, art), w * q / 3)

    rek(0, psi, 1.0)
    return gesamt


def selbsttest() -> None:
    rng = random.Random(11)
    # 1. Ohne Rauschen besteht jede Note sicher.
    for m, y in ((3, 0), (4, 5), (5, 17)):
        assert abs(bestehen_exakt_alle_fehler(m, y, 0.0) - 1.0) < 1e-12
    # 2. Der Satz: exakt (alle 4^m Fehler) = (1-p)^m, fuer JEDE Seriennummer.
    for m in (2, 3, 4):
        for y in range(1 << m):
            for p in (0.01, 0.1, 0.3):
                ist = bestehen_exakt_alle_fehler(m, y, p)
                assert abs(ist - bestehen(m, p)) < 1e-9, (m, y, p, ist, bestehen(m, p))
    # 3. Monte-Carlo am groesseren Register bestaetigt die Formel.
    for m, y, p in ((7, 37, 0.02), (8, 201, 0.05)):
        mc = bestehen_simuliert(m, y, p, 1500, rng)
        assert abs(mc - bestehen(m, p)) < 0.03, (m, y, p, mc, bestehen(m, p))
    # 4. Einteilig skaliert linear mit m (je Qubit m-mal strenger).
    assert 1.9 < einteilig(512) / einteilig(256) < 2.1
    # 5. Mehrteilig mit tau > 1/2 ist strenger als Wiesner, aber milder als einteilig.
    t_mehr, schwelle, s = mehrteilig(512, 101)
    assert schwelle == 51 and 0.5 < s < 0.7
    assert t_mehr < einteilig(512) / 20
    print("selbsttest ok")


def _dauer(tage: float) -> str:
    if tage >= 365:
        j = round(tage / 365)
        return "1 Jahr" if j == 1 else f"{j:,} Jahre".replace(",", ".")
    if tage >= 1:
        return f"{tage:,.0f} Tage".replace(",", ".")
    return f"{tage * 24:,.1f} Std.".replace(",", ".")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    print("Noetige Kohaerenzzeit, damit eine echte Note nach 1 Tag Lagerung zu >= 99 % besteht\n")
    print("| Register m (Qubits) | einteilig: T/Lagerzeit | einteilig, 1 Tag | mehrteilig (101 Teile, >50 %): T/Lagerzeit | mehrteilig, 1 Tag |")
    print("|---|---|---|---|---|")
    for m in (64, 256, 512, 2048):
        e = einteilig(m)
        mt, _, _ = mehrteilig(m, 101)
        print(f"| {m} | {e:,.0f} | {_dauer(e)} | {mt:,.0f} | {_dauer(mt)} |".replace(",", "."))
    print("\nZum Vergleich Wiesner (nur die Bank prueft, rauschen.py): T/Lagerzeit 2,5 bis 10.")


if __name__ == "__main__":
    main()
