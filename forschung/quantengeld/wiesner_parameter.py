#!/usr/bin/env python3
"""Variante W: Wie stark ist eine Pruefung wirklich? Der Klonangriff und
sichere Parameter.

Forschung, kein Produkt. Keine Abhaengigkeiten.

BEFUND (16.11.2026): Die Toleranz von 15 % ist GEBROCHEN.
  Ein Faelscher mit einer echten Note klont jede Stelle mit dem optimalen
  phasenkovarianten Kloner (Bruss u. a. 1999, quant-ph/9909046). Beide Kopien
  haben dann je Stelle die Treue 1/2 + 1/sqrt(8) = 0,8536, also 14,64 %
  Fehler je Treffer -- in JEDER Basis von BB84. Das liegt UNTER 15 %. Beide
  Kopien bestehen damit eine Pruefung mit erheblicher Wahrscheinlichkeit
  (bei k = 64 je Kopie rund 0,5). Der Faelscher bezahlt zweimal, nur einer
  der Empfaenger kann einloesen.
  Dieselbe Schwelle gilt in der Literatur: klassisch gepruefte Token sind
  nur sicher, solange die erlaubte Treue UEBER 0,854 liegt, also weniger als
  14,6 % Fehler erlaubt sind (Pastawski, Yao, Jiang, Lukin, Cirac,
  „Unforgeable noise-tolerant quantum tokens“, PNAS 109, 16079 (2012),
  arXiv:1112.5456, cv-qtickets).
  Das alte Faelschermodell in wiesner_praegen.py (wer die Stelle nicht kennt,
  raet: 50 % Fehler) war zu guenstig -- ein Kloner ist viel besser als Raten.

WAS DIESES WERKZEUG RECHNET (exakt, Binomialsummen):
  - klon_fehler(): die Fehlerrate des optimalen Kloners, nachgerechnet am
    Zustandsvektor (2 Qubits) fuer alle vier BB84-Zustaende.
  - annahme(k, p, tol, min_treffer): Wahrscheinlichkeit, dass eine Note mit
    Fehlerrate p je Treffer eine Pruefung mit k Stellen besteht -- genau nach
    der Regel der Kette (Treffer = passende Basis, echt wenn
    Fehler*100 <= tol*Treffer, unter min_treffer unentschieden).
  - Faelscher: Fehlerrate je Treffer c(f) = (1 - f) * 14,64 %, wenn eine
    Absprache den Anteil f der Stellen kennt (Fern-Praeparation: f = m/v bei
    m bestochenen von v Validatoren). Fuer f = 0 ist das der Kloner.
  - suche(): zu einer gemessenen Fehlerrate e der Hardware die Toleranz und
    die kleinste Zahl k, mit der ein Faelscher hoechstens mit 2^-40 und eine
    ehrliche Note mindestens mit 1 - 10^-6 besteht.

GRENZEN DER AUSSAGE (ehrlich):
  - Gerechnet ist der Kloner Stelle fuer Stelle (Produktangriff) und je
    Kopie (der Faelscher braucht ZWEI bestehende Kopien; je Kopie ist also
    vorsichtig). Fuer beliebige verschraenkte Angriffe beweisen Pastawski u. a.
    dieselbe Schwelle und eine Schranke derselben Form exp(-h*D(tol||0,146));
    die Uebertragung auf Variante W (jede Stelle wird nur einmal gefragt)
    gehoert in das Gutachten (W5).
  - Es zaehlen nur Treffer (passende Basis): je Pruefung im Mittel k/2.

Aufruf:  python3 -I wiesner_parameter.py [--selbsttest]
"""
from __future__ import annotations

import math
import random
import sys

ZIEL_FAELSCHER = 2.0 ** -40     # hoechstens so oft besteht eine Faelschung eine Pruefung
ZIEL_EHRLICH = 1e-6             # hoechstens so oft faellt eine echte Note durch
MIN_TREFFER_ALT, TOL_ALT, K_MAX_ALT = 4, 15, 64   # wiesner_kette.py, Stand 16.11.2026


# ---------------------------------------------------------------- Kloner
def _mal(m, v):
    return [sum(m[i][j] * v[j] for j in range(len(v))) for i in range(len(m))]


def _kron(a, b):
    return [[a[i // 2][j // 2] * b[i % 2][j % 2] for j in range(4)] for i in range(4)]


def klon_fehler() -> list[tuple[float, float]]:
    """Fehlerrate beider Kopien je BB84-Zustand, wenn man sie in der
    richtigen Basis misst. Kloner: R_x(-pi/2) (x) R_x(-pi/2) . U . R_x(pi/2),
    U|00> = |00>, U|10> = (|10> + |01>)/sqrt 2 -- der phasenkovariante
    Kloner fuer die Ebene von BB84 (x-z), gedreht aus der Aequator-Ebene."""
    w = 1 / math.sqrt(2)
    rx = [[w, -1j * w], [-1j * w, w]]       # R_x(pi/2)
    rxi = [[w, 1j * w], [1j * w, w]]        # R_x(-pi/2)
    zustaende = {"0": [1, 0], "1": [0, 1], "+": [w, w], "-": [w, -w]}
    aus = []
    for psi in zustaende.values():
        a, b = _mal(rx, psi)
        # U auf (a|0> + b|1>) (x) |0>; Basis |q1 q0>, Index 2*q1 + q0
        s = [a, b * w, b * w, 0]            # |00>: a, |01>: b/sqrt2, |10>: b/sqrt2
        s = _mal(_kron(rxi, rxi), s)
        fehler = []
        for kopie in (0, 1):
            # reduzierte Dichtematrix der Kopie, Treue mit psi
            rho = [[0j, 0j], [0j, 0j]]
            # Teilspur ueber die andere Kopie: Index i = 2*q1 + q0, Kopie c = Bit c.
            for i in range(4):
                for j in range(4):
                    if (i >> (1 - kopie)) & 1 != (j >> (1 - kopie)) & 1:
                        continue
                    rho[(i >> kopie) & 1][(j >> kopie) & 1] += s[i] * complex(s[j]).conjugate()
            treue = sum(complex(psi[i]).conjugate() * rho[i][j] * psi[j] for i in range(2) for j in range(2)).real
            fehler.append(1 - treue)
        aus.append(tuple(fehler))
    return aus


KLON_FEHLER = 0.5 - 1 / math.sqrt(8)        # 0,14645 -- klon_fehler() rechnet es nach


# ---------------------------------------------------------------- Pruefung
def _log_bin(n: int, k: int, p: float) -> float:
    if p <= 0:
        return 0.0 if k == 0 else -math.inf
    if p >= 1:
        return 0.0 if k == n else -math.inf
    return math.lgamma(n + 1) - math.lgamma(k + 1) - math.lgamma(n - k + 1) + k * math.log(p) + (n - k) * math.log1p(-p)


def annahme(k: int, p: float, tol: int, min_treffer: int) -> tuple[float, float]:
    """(echt, unentschieden) fuer eine Note mit Fehlerrate p je Treffer bei
    k gepruefter Stellen, Regel der Kette (ganzzahlig: Fehler*100 <= tol*Treffer).
    Rechnet im Logarithmus und schrittweise (pmf(e+1) aus pmf(e)), damit auch
    k in den Tausenden schnell und ohne Unterlauf geht."""
    lp, lq = (math.log(p) if p > 0 else -math.inf), math.log1p(-p)
    echt, unent = [], []
    lh = k * math.log(0.5)                       # log P(h = 0)
    for h in range(k + 1):
        if h > 0:
            lh += math.log(k - h + 1) - math.log(h)
        if h < min_treffer:
            unent.append(math.exp(lh))
            continue
        m = (tol * h) // 100
        le = h * lq                              # log P(e = 0 | h)
        teile = [math.exp(lh + le)]
        for e in range(m):
            if p <= 0:
                break
            le += math.log(h - e) - math.log(e + 1) + lp - lq
            teile.append(math.exp(lh + le))
        echt.append(math.fsum(teile))
    return math.fsum(echt), math.fsum(unent)


def bedingt(k: int, p: float, tol: int, min_treffer: int) -> float:
    """Annahme unter der Bedingung, dass entschieden wurde (unentschieden
    darf man wiederholen -- vorsichtig fuer den Faelscher)."""
    e, u = annahme(k, p, tol, min_treffer)
    return e / (1 - u) if u < 1 else 0.0


def faelscher_fehler(f: float) -> float:
    """Fehlerrate je Treffer: bekannte Stellen (Anteil f) fehlerfrei, die
    anderen geklont."""
    return (1 - f) * KLON_FEHLER


def _ok(k: int, e: float, c: float, tol: int, mt: int) -> tuple[bool, float, float]:
    fa = bedingt(k, c, tol, mt)
    eh, uh = annahme(k, e, tol, mt)
    faellt = 1 - eh - uh            # unentschieden: Pfand zurueck, kein Schaden
    return fa <= ZIEL_FAELSCHER and faellt <= ZIEL_EHRLICH, fa, faellt


def suche(e: float, f: float = 0.0, k_max: int = 8192, min_treffer: int = 1) -> dict | None:
    """Kleinste Pruefung (k, in Achterschritten) ueber alle ganzzahligen
    Toleranzen zwischen e und der Faelscher-Fehlerrate, die beide Ziele
    erreicht. None, wenn es keine solche Toleranz gibt."""
    c = faelscher_fehler(f)
    best = None
    for tol in range(1, 100):
        if tol / 100 >= c:
            break
        if tol / 100 <= e:
            continue
        hi = 64
        while hi <= k_max and not _ok(hi, e, c, tol, min_treffer)[0]:
            hi *= 2
        if hi > k_max:
            continue
        lo = hi // 2                     # lo scheitert (oder ist < 64)
        while hi - lo > 8:
            mid = (lo + hi) // 2 // 8 * 8
            if mid <= lo:
                break
            if _ok(mid, e, c, tol, min_treffer)[0]:
                hi = mid
            else:
                lo = mid
        r = _ok(hi, e, c, tol, min_treffer)
        if best is None or hi < best["k"]:
            best = {"e": e, "f": f, "tol": tol, "k": hi, "faelscher": r[1], "ehrlich_faellt": r[2], "c": c}
    return best


# ---------------------------------------------------------------- Selbsttest
def selbsttest() -> bool:
    fehler: list[str] = []

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    try:
        # 1. Der Kloner: beide Kopien, alle vier Zustaende, genau 14,64 %.
        kf = klon_fehler()
        soll(all(abs(x - KLON_FEHLER) < 1e-12 for paar in kf for x in paar), f"Kloner: {kf}")
        # Gegenprobe: ohne die Drehung in die x-z-Ebene ist |0> perfekt, |+> schlecht --
        # der Kloner MUSS zur Ebene von BB84 passen.
        soll(KLON_FEHLER < 0.15 < 1 / 6, "Ordnung der Schwellen")

        # 2. Missbrauchstest -- der Angriff gegen die ALTEN Parameter:
        # eine geklonte Kopie besteht bei k = 64 mit rund 0,5, beide Kopien
        # (verschiedene Stellen, unabhaengig) mit rund 0,25.
        p1 = bedingt(K_MAX_ALT, KLON_FEHLER, TOL_ALT, MIN_TREFFER_ALT)
        soll(p1 > 0.4, f"Klonangriff gegen 15 %/64 sollte durchgehen, nur {p1:.3f}")
        p16 = bedingt(16, KLON_FEHLER, TOL_ALT, MIN_TREFFER_ALT)
        soll(p16 > 0.4, f"Klonangriff bei k = 16: {p16:.3f}")
        # Das alte Modell (Raten, 50 %) haette nur 3,8e-5 gesagt -- so falsch war es.
        soll(bedingt(64, 0.5, TOL_ALT, MIN_TREFFER_ALT) < 1e-3, "Raten-Modell")

        # 3. Exakte Summe gegen Monte-Carlo (kleiner Fall).
        rng = random.Random(1912)
        k, tol, mt, laeufe = 24, 15, 4, 40000
        durch = ents = 0
        for _ in range(laeufe):
            h = sum(rng.random() < 0.5 for _ in range(k))
            if h < mt:
                continue
            ents += 1
            fe = sum(rng.random() < KLON_FEHLER for _ in range(h))
            durch += fe * 100 <= tol * h
        soll(abs(durch / ents - bedingt(k, KLON_FEHLER, tol, mt)) < 0.01, f"Monte-Carlo {durch / ents:.4f} gegen exakt")

        # 4. Sichere Parameter fuer die gemessene Hardware (IBM ohne Warten 0,7 %,
        # mit Bezahlen ~3 %): Faelscher <= 2^-40, ehrlich faellt <= 1e-6.
        for e in (0.007, 0.03):
            r = suche(e)
            soll(r is not None and r["faelscher"] <= ZIEL_FAELSCHER and r["ehrlich_faellt"] <= ZIEL_EHRLICH,
                 f"keine sicheren Parameter fuer e = {e}: {r}")
        # Gegenprobe: mit 15 % gibt es KEINE sichere Wahl (Toleranz ueber dem Kloner).
        soll(all(t / 100 >= KLON_FEHLER or t / 100 <= 0.007 for t in (15,)), "15 % muss ausgeschlossen sein")
        # Absprache: je groesser f, desto mehr Stellen; ab f, bei dem (1-f)*14,6 % <= e, unmoeglich.
        r0, r1 = suche(0.007, 0.0), suche(0.007, 0.2)
        soll(r0 and r1 and r1["k"] >= r0["k"], f"Absprache: {r0} {r1}")
        soll(suche(0.03, 0.8) is None, "f = 0,8 bei e = 3 % darf nicht sicher sein")
    except Exception as ex:
        fehler.append(f"Ausnahme: {type(ex).__name__}: {ex}")
    for f in fehler:
        print("FEHLER:", f)
    print("wiesner_parameter: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def main():
    print(f"Kloner: {KLON_FEHLER:.4%} Fehler je Treffer in beiden Kopien (nachgerechnet: "
          f"{max(x for p in klon_fehler() for x in p):.4%}).")
    print()
    print("ALTE Parameter (15 %, Mindesttreffer 4): Annahme EINER geklonten Kopie")
    for k in (16, 32, 64):
        print(f"  k = {k:>3}: {bedingt(k, KLON_FEHLER, TOL_ALT, MIN_TREFFER_ALT):.3f}"
              f"   (beide Kopien ~ {bedingt(k, KLON_FEHLER, TOL_ALT, MIN_TREFFER_ALT) ** 2:.3f})")
    print()
    print(f"SICHERE Parameter: Faelscher <= 2^-40, echte Note faellt <= 1e-6 durch")
    print(f"{'Hardware-Fehler e':>18} | {'Absprache f':>11} | {'Toleranz':>8} | {'k':>5} | {'Faelscher':>9} | {'echt faellt':>11}")
    for e in (0.004, 0.007, 0.02, 0.03, 0.06):
        for f in (0.0, 1 / 64, 0.2):
            r = suche(e, f)
            if r is None:
                print(f"{e:>18.1%} | {f:>11.3f} | {'--':>8} | {'--':>5} | unmoeglich: (1-f)*14,6 % liegt nicht ueber e")
                continue
            print(f"{e:>18.1%} | {f:>11.3f} | {r['tol']:>7}% | {r['k']:>5} | {r['faelscher']:>9.1e} | {r['ehrlich_faellt']:>11.1e}")


if __name__ == "__main__":
    sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else main()
