#!/usr/bin/env python3
"""Der Weg in Zahlen: Pruefen einer Note mit qt-Pegasis als Rechenwerk.

Forschung, kein Produkt (BAUVORGABE §8). Keine Abhaengigkeiten.

QUELLEN (Volltexte gelesen, 14.11.2026):
  [Z]  Zhandry, Quantum Money from Abelian Group Actions, TheoretiCS 2025.
       EGA-Fassung: Pruefen = 2 Gruppenaktionen beliebiger Elemente.
       REGA-Fassung (CSIDH): lambda Hadamard-Tests, je Test ehrlich >= 0,961,
       fremd <= 0,848 (Satz 6.7).
  [Q]  Dartois, Komada Eriksen, Invernizzi, Vercauteren: qt-Pegasis,
       ePrint 2025/1859. Beliebige Elemente, scheitert nie; 91-95 % der Zeit
       in EINER 4-dimensionalen 2^e-Isogeniekette, e ~ log2 p.
  [DD] Dartois, Duparc: Chasing rabbits through hypercubes. Zeitkonstante
       C-Umsetzung der 4D-Kette, Tabelle 1 (Apple M1, Mcycles):
       505 Bit 146,95 | 1008 Bit 871,34 | 1525 Bit 3163,05 |
       2017 Bit 7041,82 | 4030 Bit 55228,89.

UMRECHNUNG (Annahme, offen ausgewiesen): eine Montgomery-Multiplikation mit
L = ceil(Bit/64) Woertern kostet ~ 2 L^2 Takte. Daraus Multiplikationen je
Kette; die Steigung ueber 505...4030 Bit wird angepasst und auf 8.192 Bit
fortgeschrieben (die Kettenlaenge waechst linear mit log2 p, [Q]).

SCHRITT 1 (Normgleichung) in Ueberlagerung: [Q] sucht per Schleife eine
Zahl, die Summe zweier Quadrate ist (Cornacchia: Primzahl-Kandidaten). Im
Schaltkreis braucht die Schleife eine FESTE Obergrenze: so viele Kandidaten,
dass die ehrliche Note nur mit <= 2^-40 durchfaellt; Siebung durch kleine
Primzahlen (billig) laesst ~1/20 fuer den Miller-Rabin-Test (~ log2 p
Multiplikationen) uebrig. Das ist eine eigene, grobe Schaetzung.

VARIANTE B (eigener Vorschlag): u klassisch ziehen, Schritt 1 klassisch,
nur die gesteuerte Kette quantisch -- 128 Tests, aber KEINE Primzahlsuche
und keine Ueberlagerung ueber Ideale im Schaltkreis (viel einfacher).

Aufruf: python3 -I pegasis_kosten.py [--selbsttest]
"""
from __future__ import annotations

import importlib.util
import math
import os
import sys

_spec = importlib.util.spec_from_file_location(
    "isogenie_hebel", os.path.join(os.path.dirname(os.path.abspath(__file__)), "isogenie_hebel.py"))
hebel = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(hebel)

DD_M1 = ((505, 146.95e6), (1008, 871.34e6), (1525, 3163.05e6), (2017, 7041.82e6), (4030, 55228.89e6))
TOFFOLI_JE_MUL = 4  # * B^2, gemessen in schaltkreis_mult.py (Schulbuch)


def mult_je_kette(bits: int, takte: float) -> float:
    L = math.ceil(bits / 64)
    return takte / (2 * L * L)


def anpassen():
    """log2 M = a * log2 bits + c ueber die gemessenen Punkte."""
    xs = [math.log2(b) for b, _ in DD_M1]
    ys = [math.log2(mult_je_kette(b, t)) for b, t in DD_M1]
    mx, my = sum(xs) / len(xs), sum(ys) / len(ys)
    a = sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / sum((x - mx) ** 2 for x in xs)
    return a, my - a * mx


def kette_mult(bits: int) -> float:
    a, c = anpassen()
    return 2 ** (a * math.log2(bits) + c)


def kette_formel(bits: int) -> float:
    """Gegenrechnung aus den Formeln ([DD] Abschnitt 2, Dimension g = 4,
    2^g = 16 Theta-Koordinaten): Auswertung einer generischen 2-Isogenie
    16 S + 16 M; Verdopplung ~ zweimal das (64); Bildkurve ~ 3(2^g - 2) M
    + 16 S + Kleinkram (~100); Kette mit e ~ log2 p Schritten, ausgewogene
    Strategie: je der g = 4 Kernpunkte ~ (e/2) log2 e Verdopplungen und
    ebenso viele Auswertungen. Eigene Schaetzung der Strategiekosten."""
    e = bits
    je_punkt = (e / 2) * math.log2(e)
    return 4 * je_punkt * (64 + 32) + 100 * e


def schritt1_mult(bits: int, ausfall_log2: int = 40) -> float:
    """Feste Kandidatenzahl K: P[keine Primzahl = 1 mod 4 unter K] <= 2^-40.
    Dichte ~ 1 / (ln n) * 2 / 2 (ungerade, = 1 mod 4) -> q = 1 / (ln n)."""
    q = 1 / (bits * math.log(2))
    K = math.ceil(ausfall_log2 * math.log(2) / q)
    mr = math.ceil(K / 20 * 1.2)           # nach Siebung, mit Reserve
    return mr * 1.5 * bits                 # je Test eine Potenz ~ 1,5 log2 p Mult.


def pruefen_toffoli(bits: int) -> dict:
    """EGA-Pruefung: 2 Aktionen, je Aktion Schritt 1 + Kette."""
    tof = TOFFOLI_JE_MUL * bits * bits
    k, s1 = kette_mult(bits), schritt1_mult(bits)
    return {
        "kette": math.log2(k),
        "schritt1": math.log2(s1),
        "pruefen_mult": math.log2(2 * (k + s1)),
        "pruefen_toffoli": math.log2(2 * (k + s1) * tof),
    }


def klassisch_u_toffoli(bits: int, tests: int = 128) -> float:
    """Variante B: Pruefer zieht u KLASSISCH gleichverteilt, loest Schritt 1
    auf einem gewoehnlichen Rechner; der Quantenrechner fuehrt je Test nur die
    von EINEM Qubit gesteuerte 4D-Kette aus (hin und zurueck: 2 Ketten).
    Weil die Note ein exakter Eigenzustand ist und u gleichverteilt, gilt
    Mittel_u chi(u,h)^* U_u = P_h: jeder Test wirkt wie (I + P_h)/2 -- eine
    fremde Note besteht je Test mit genau 1/2, die echte mit 1. Fuer 2^-128
    also 128 Tests."""
    return math.log2(tests * 2 * kette_mult(bits) * TOFFOLI_JE_MUL * bits * bits)


def nebenkosten_mult(bits: int) -> dict:
    """Je Test in Ueberlagerung auf der Kurve der Note (eigene Schaetzung):
    Traegerpruefung: ist es eine supersingulaere Kurve der Bahn? Ein
      deterministischer Punkt, [p+1]P = 0 und volle 2-Potenz-Ordnung:
      ~ 1 Leiter ueber log2 p Bit (12 Mult. je Schritt).
    Schritt 2.1 [Q]: Basis der 2^(e+2)-Torsion -- feste Folge von
      Kandidatenpunkten, je ~1/2 Erfolg; fuer Ausfall <= 2^-40 bis zu 40
      Versuche, je Kofaktor-Leiter + e Verdopplungen (~6 Mult.).
    Schritt 2.2 [Q]: 2 Punkte mal 5 Skalare (~e Bit): 10 Leitern."""
    leiter = 12 * bits
    return {
        "traeger": leiter,
        "schritt2": 40 * (6 * bits) + 10 * leiter,
    }


def gesamt_b(bits: int, tests: int = 128) -> dict:
    """Variante B komplett: je Test Traeger + Schritt 2 + 2 Ketten; dazu
    Umkehrbarkeit (Bennett, umkehrbar.py) mit dem Zustand der 4D-Kette:
    je Kiesel ~ 16 Theta-Koordinaten x B Bit x log2(e) gehaltene Punkte."""
    ub = _lade_umkehrbar()
    n = nebenkosten_mult(bits)
    je_test = n["traeger"] + n["schritt2"] + 2 * kette_mult(bits)
    mult = tests * je_test
    T = max(2, int(je_test / 50))                       # Schritte je Test (Kette ~50 Mult./Schritt)
    S = 16 * bits * max(1, int(math.log2(bits)))        # Qubits je Kiesel
    zeilen = []
    for b in (16, 256, 4096):
        k, f = ub.formel(T, b)
        zeilen.append((b, k * S, 2 * f, math.log2(mult * 2 * f * TOFFOLI_JE_MUL * bits * bits)))
    return {"mult": math.log2(mult), "neben_anteil": (n["traeger"] + n["schritt2"]) / je_test, "umkehrbar": zeilen}


def _lade_umkehrbar():
    spec = importlib.util.spec_from_file_location(
        "umkehrbar", os.path.join(os.path.dirname(os.path.abspath(__file__)), "umkehrbar.py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def rega_toffoli(bits: int, tests: int = 2750, mult_je_aktion_log2: float | None = None) -> float:
    """REGA-Fassung (CSIDH, [Z] Abschnitt 6): Tests x 2 Aktionen.
    Mult. je Aktion aus isogenie_hebel.py (optimale Strategie, Ueberdeckung)."""
    if mult_je_aktion_log2 is None:
        mult_je_aktion_log2 = math.log2(hebel.aktion_modell(bits, "optimal"))
    return math.log2(tests * 2) + mult_je_aktion_log2 + math.log2(TOFFOLI_JE_MUL * bits * bits)


def selbsttest() -> None:
    # 1. Die Kette waechst ~ linear in der Bitlaenge (Steigung 0,7 ... 1,2):
    #    so beschreibt [Q] die Kettenlaenge e ~ log2 p.
    a, _ = anpassen()
    assert 0.7 < a < 1.2, a
    # 2. Die Anpassung trifft jeden Messpunkt auf Faktor 1,5.
    for b, t in DD_M1:
        assert 1 / 1.5 < kette_mult(b) / mult_je_kette(b, t) < 1.5, b
    # 3. Der Weg ist billiger als die REGA-Fassung: Variante A um mindestens
    #    2^8, Variante B um mindestens 2^5.
    for bits in (4096, 8192):
        assert rega_toffoli(bits) - pruefen_toffoli(bits)["pruefen_toffoli"] > 8, bits
        assert rega_toffoli(bits) - klassisch_u_toffoli(bits) > 5, bits
    # 4. Variante B: Mittel ueber u von chi(u, d) ist 0 fuer d != 0 (darauf
    #    ruht "fremde Note besteht je Test mit 1/2") -- an Z_N geprueft.
    import cmath
    for N in (7, 12, 1491):
        for d in (1, 2, N - 1):
            m = sum(cmath.exp(2j * math.pi * u * d / N) for u in range(N)) / N
            assert abs(m) < 1e-9, (N, d)
    # 4b. Zwei unabhaengige Wege zur Kettenlaenge stimmen ueberein: aus den
    #     gemessenen Takten und aus den Formeln -- auf Faktor 2.
    for b, t in DD_M1:
        r = kette_formel(b) / mult_je_kette(b, t)
        assert 1 / 2 < r < 2, (b, r)
    # 5. Traeger und Schritt 2 sind klein gegen die Kette (< 1/3 je Test).
    for bits in (4096, 8192, 17000):
        assert gesamt_b(bits)["neben_anteil"] < 1 / 3, bits
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    a, c = anpassen()
    print("4D-Kette aus [DD] Tabelle 1 (M1), umgerechnet mit ~2 L^2 Takten je Multiplikation:")
    print("| Bit | Mcycles | Multiplikationen |")
    print("|---|---|---|")
    for b, t in DD_M1:
        print(f"| {b} | {t / 1e6:,.0f} | 2^{math.log2(mult_je_kette(b, t)):.1f} |".replace(",", "."))
    print(f"Anpassung: log2 M = {a:.2f} * log2(Bit) + {c:.1f}")
    print("Gegenrechnung aus den Formeln ([DD] Abschn. 2, ausgewogene Strategie):")
    for b, t in DD_M1:
        print(f"  {b} Bit: Formel 2^{math.log2(kette_formel(b)):.1f}, aus Takten 2^{math.log2(mult_je_kette(b, t)):.1f}")
    print(f"  8192 Bit: Formel 2^{math.log2(kette_formel(8192)):.1f}, Anpassung 2^{math.log2(kette_mult(8192)):.1f}")
    print("\nPruefen einer Note (Toffoli, vor Umkehrbarkeit -- dafuer umkehrbar.py: x8 bis x100):")
    print("| Primzahl | Kette je Aktion | Schritt 1 je Aktion (in Ueberlagerung) | **A: qt-Pegasis kohaerent (2 Aktionen)** | **B: u klassisch (128 Tests)** | CSIDH-REGA (2.750 Tests) |")
    print("|---|---|---|---|---|---|")
    for bits in (4096, 8192, 17000):
        w = pruefen_toffoli(bits)
        r = rega_toffoli(bits)
        b = klassisch_u_toffoli(bits)
        print(f"| {bits} Bit | 2^{w['kette']:.1f} Mult. | 2^{w['schritt1']:.1f} Mult. | **2^{w['pruefen_toffoli']:.1f}** | **2^{b:.1f}** | 2^{r:.1f} |")
    drucke_gesamt()


def drucke_gesamt() -> None:
    print("\nVariante B komplett (je Test Traeger + Schritt 2 + 2 Ketten, 128 Tests), mit Umkehrbarkeit:")
    print("| Primzahl | Mult. gesamt | Anteil Traeger+Schritt 2 | Arbeitsspeicher (logische Qubits) | Faktor | Toffoli gesamt |")
    print("|---|---|---|---|---|---|")
    for bits in (4096, 8192, 17000):
        g = gesamt_b(bits)
        for b, q, f, t in g["umkehrbar"]:
            print(f"| {bits} Bit | 2^{g['mult']:.1f} | {100 * g['neben_anteil']:.0f} % | {q:.1e} | {f:.0f}x | 2^{t:.1f} |")


if __name__ == "__main__":
    main()
