"""Weg C, C2b-2b-3a: Hilft es einem Faelscher, nach der Meldung abzubrechen
und neu vorzumerken, bis eine Teilmenge viele freie Pulse traegt?

Modell (vorsichtig fuer den Faelscher): Die Pulse eines Tokens sind je mit
Wahrscheinlichkeit frei (die gemessene, aufgerundete Mehrphotonen-Rate der
Leitung) dem Faelscher bekannt; die Zahl X der freien Stellen unter den n
Stellen schwankt binomial. K_LEITUNG rechnet mit genau frei * n. Ein Versuch
hilft, wenn X darueber liegt: der Faelscher irrt dann nur mit
(1 - X/n - (t-1)/v) * lambda(theta) je Treffer. s3_versuch rechnet die
Wahrscheinlichkeit, dass EIN Versuch die Pruefung mit k = k_leitung besteht,
ueber alle X (in Bloecken, jeweils mit dem fuer den Faelscher besten X des
Blocks -- also eine obere Schranke).

Ergebnis (27.11.2026, theta 0/2/5/10 Grad, tol 2/4/6, frei auf jeder
Sechzehntel-Grenze bis 25 %, n = 2k und n = N_MAX): hoechstens 2^-39,6 je
Versuch. Die Schwankung der Teilmenge verschiebt die Schranke 2^-40 also kaum.
Mit dem Pfand (s_kette.s3_pfand, mindestens 1 AEQ, verfaellt nach der
Meldung) kostet eine Faelschung durch Wiederholen im Mittel mehr als
2^39 AEQ; dazu kommt die Frist (eine offene Vormerkung je Mensch, 1 h).

Aufruf:  python3 -I s3_versuch.py            (ganze Tabelle, einige Minuten)
         python3 -I s3_versuch.py --schnell  (eine Zeile, Sekunden)
"""
import functools
import importlib.util
import math
import os
import sys


def _laden(name: str):
    # wie s_kette.py: aus dem eigenen Ordner, auch mit python3 -I
    spec = importlib.util.spec_from_file_location(name, os.path.join(os.path.dirname(os.path.abspath(__file__)), name + ".py"))
    m = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(m)
    return m


wk = _laden("wiesner_kette")
wp = _laden("wiesner_parameter")


def _lpmf(n: int, p: float, x: int) -> float:
    return math.lgamma(n + 1) - math.lgamma(x + 1) - math.lgamma(n - x + 1) + x * math.log(p) + (n - x) * math.log1p(-p)


@functools.lru_cache(None)
def _bestehen(k: int, c_ppm: int, tol: int) -> float:
    return 1.0 if c_ppm <= 0 else wp.bedingt(k, c_ppm / 1e6, tol, wk.MIN_TREFFER)


def s3_versuch(theta: int, tol: int, t: int, v: int, frei: float, k: int, n: int) -> float:
    """Obere Schranke: Wahrscheinlichkeit, dass ein Versuch besteht."""
    lam = wp.faelscher_schranke(theta)
    mu, sd = n * frei, math.sqrt(n * frei * (1 - frei))
    schritt = max(1, int(sd / 3))

    def c_ppm(x: int) -> int:
        return math.floor((1 - x / n - (t - 1) / v) * lam * 1e6)

    lo = max(0, int(mu - 8 * sd))
    # Alles unter lo: hoechstens Masse 1 mal Erfolg bei lo (vorsichtig).
    unter = math.fsum(math.exp(_lpmf(n, frei, y)) for y in range(lo))
    summe = min(1.0, unter) * _bestehen(k, c_ppm(lo), tol)
    x = lo
    while x <= n and x <= mu + 40 * sd:
        hi = min(n, x + schritt - 1)
        masse = math.fsum(math.exp(_lpmf(n, frei, y)) for y in range(x, hi + 1))
        summe += masse * _bestehen(k, c_ppm(hi), tol)
        x = hi + 1
    return summe


def tabelle(schnell: bool = False):
    zeilen = []
    for theta in ((2,) if schnell else wk.THETA_STUFEN):
        for tol in ((4,) if schnell else (2, 4, 6)):
            for f16 in ((1,) if schnell else range(1, 5)):
                frei_ppm = f16 * 1_000_000 // 16      # genau auf der Grenze: f16 bleibt
                k = wk.k_leitung(tol, 1, 1, theta * 10, frei_ppm)
                if k == 0:
                    continue
                for n in sorted({2 * k, wk.N_MAX}):
                    if n > wk.N_MAX:
                        continue
                    s = s3_versuch(theta, tol, 1, 1, frei_ppm / 1e6, k, n)
                    zeilen.append((s, theta, tol, frei_ppm, k, n))
                    print(f"theta {theta:2d}  tol {tol}  frei {frei_ppm:6d}  k {k:5d}  n {n:5d}  log2 {math.log2(s):6.1f}", flush=True)
    s = max(zeilen)[0]
    print(f"hoechstens 2^{math.log2(s):.1f} je Versuch")
    return s


if __name__ == "__main__":
    schlimmst = tabelle("--schnell" in sys.argv)
    sys.exit(0 if schlimmst <= 2.0 ** -39 else 1)
