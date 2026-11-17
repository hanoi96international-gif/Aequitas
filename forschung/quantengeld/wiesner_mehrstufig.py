#!/usr/bin/env python3
"""Variante W, B5: mehrstufige (sequenzielle) Pruefung -- die Rechnung.

Forschung, kein Produkt. Keine Abhaengigkeiten ausser wiesner_parameter.py
(daneben).

FRAGE (OFFEN.md, B5): Einloesen erst nach zwei unabhaengigen Ziehungen oder
nach einem sequenziellen Test? Spart das Stellen, ohne die Ziele zu lockern
(Faelscher hoechstens 2^-40, echte Note faellt hoechstens 10^-6), auch gegen
eine Absprache, die den Anteil f der Stellen kennt?

MODELL (wie wiesner_parameter.py): Treffer = Stelle mit passender Basis
(Wahrscheinlichkeit 1/2, nur Treffer tragen Information); je Treffer ein
Fehler mit Rate e (echte Note, Hardware) bzw. c(f) = (1 - f) * 14,64 %
(Faelscher: bekannte Stellen fehlerfrei, die anderen geklont).

SEQUENZIELLER TEST (Wald, SPRT), geprueft nach jedem Teil von 64 Stellen
(so legen die Validatoren ohnehin offen):
  L = Summe ueber die Treffer von ln(P_Faelscher / P_echt) --
      Fehler: ln(c/e), kein Fehler: ln((1-c)/(1-e)).
  L <= -A  ->  echt (angenommen),    A = 40 ln 2
  L >=  B  ->  faellt durch,         B = ln(2 * 10^6)
  sonst weiter; nach k_max Stellen ohne Entscheidung: faellt durch.

STRENGE SCHRANKEN (fuer jede Abbruchregel, Ville/Wald: exp(L) ist unter
"echt" ein Martingal mit Erwartung 1, exp(-L) unter "Faelscher"):
  P_Faelscher(angenommen) <= e^-A = 2^-40
  P_echt(faellt)          <= e^-B + P_echt(L nach k_max > -A)   (Chernoff)
  Ein Faelscher mit HOEHERER Fehlerrate als c wird hoechstens so oft
  angenommen (Kopplung: mehr Fehler schieben L nur nach oben).
  Kein "unentschieden": der Test entscheidet immer.

GERECHNET (main): k der festen Pruefung (wiesner_parameter.suche) gegen den
sequenziellen Test -- k_max (so viele Stellen braucht er hoechstens) und die
erwartete Zahl Stellen einer echten Note (Simulation, und Walds Naeherung
2A/D), dazu "zwei unabhaengige Ziehungen" (jede fest, je 2^-20).

BEFUND (17.11.2026, Tabelle von main()):
  - Zwei unabhaengige feste Ziehungen (je 2^-20) sparen nichts: zusammen
    brauchen sie meist MEHR Stellen als eine feste Pruefung zu 2^-40 (der
    Exponent waechst linear in k), nur in einem Fall 8 % weniger. Gegen
    bestochene Validatoren hilft eine zweite Ziehung auch nicht: der Zufall
    kommt aus den Hash-Ketten ALLER Validatoren, ein ehrlicher genuegt.
  - Der sequenzielle Test entscheidet fuer eine echte Note im Mittel nach
    36-64 % weniger Stellen als die feste Pruefung; seine Hoechstzahl k_max
    liegt aber 2-62 % ueber dem festen k. Zieht die Anfrage weiter alles auf
    einmal, misst der Halter k_max Stellen -- das spart nur Offenlegen.
  - ZWEI STUFEN gewinnen: erst s1 Stellen ziehen, beantworten, offenlegen;
    nur wenn der Test danach nicht entschieden hat, eine zweite Ziehung bis
    k_max. Eine echte Note verbraucht dann im Mittel 20-52 % weniger Stellen
    als heute (eine Note haelt entsprechend mehr Pruefungen aus); nur 4-12 %
    der Pruefungen brauchen die zweite Stufe. Der Faelscher bleibt streng
    bei 2^-40 je Pruefung (Wald), auch mit Absprache f, ohne Unentschieden.
  - Vorschlag fuer den Knoten (W2e, eigener Schritt, Konsens): die Grenzen
    als ganzzahlige Tabelle je Treffer-Zahl h (hoechstens a(h) Fehler ->
    echt, mindestens r(h) Fehler -> faellt), s1 und k_max je (Toleranz,
    Absprache) wie quantenWKTabelle; die Anfrage bekommt eine zweite Runde
    (neuer Zufall, neue Antwort), ohne Entscheidung nach k_max: faellt.
    Erst Spezifikation (wiesner_kette.py) und Pruefvektor, dann Knoten.

Aufruf:  python3 -I wiesner_mehrstufig.py [--selbsttest]
"""
from __future__ import annotations

import math
import os
import random
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import wiesner_parameter as wp  # noqa: E402  (daneben, eigener Code)

A = 40 * math.log(2)            # Annahme-Schwelle: Faelscher <= 2^-40
B = math.log(2e6)               # Ablehnung: echte Note <= 5e-7 durch Fehlalarm
ZIEL_ECHT = 1e-6                # echte Note faellt hoechstens so oft
TEIL = 64                       # Stellen je Teil (quantenWOffenTeil)


def schritte(e: float, c: float) -> tuple[float, float]:
    """L-Zuwachs je Treffer: (mit Fehler, ohne Fehler)."""
    return math.log(c / e), math.log((1 - c) / (1 - e))


def kl(e: float, c: float) -> float:
    """D(e || c) in nats: mittlerer Abstieg von L je Treffer einer echten Note."""
    return e * math.log(e / c) + (1 - e) * math.log((1 - e) / (1 - c))


def chernoff_unentschieden(k: int, e: float, c: float, a: float | None = None) -> float:
    """Obere Schranke fuer P_echt(L nach k Stellen > -a): Chernoff ueber die
    Stellen (je Stelle: kein Treffer -> 0, Treffer -> Zuwachs)."""
    a = A if a is None else a
    hoch, tief = schritte(e, c)
    best = 1.0
    for i in range(1, 400):
        lam = i / 400
        m = 0.5 + 0.5 * (e * math.exp(lam * hoch) + (1 - e) * math.exp(lam * tief))
        log_s = k * math.log(m) + lam * a
        best = min(best, math.exp(min(log_s, 0.0)))
    return best


def genau_unentschieden(k: int, e: float, c: float, a: float) -> float:
    """P_echt(L nach k Stellen > -a) exakt (Binomialsummen) -- Gegenprobe
    fuer die Chernoff-Schranke."""
    hoch, tief = schritte(e, c)
    summe = 0.0
    for h in range(k + 1):
        ph = math.exp(wp._log_bin(k, h, 0.5))
        if ph < 1e-300:
            continue
        for j in range(h + 1):
            if j * hoch + (h - j) * tief > -a:
                summe += ph * math.exp(wp._log_bin(h, j, e))
    return summe


def k_max(e: float, c: float, grenze: int = 1 << 16, ziel: float = ZIEL_ECHT) -> int | None:
    """Kleinste Zahl Stellen (in Teilen), bei der eine echte Note hoechstens
    mit ziel durchfaellt (strenge Schranke: Fehlalarm plus unentschieden)."""
    if not 0 < e < c < 1:
        return None
    k = TEIL
    while k <= grenze:
        if math.exp(-B) + chernoff_unentschieden(k, e, c) <= ziel:
            return k
        k += TEIL
    return None


def sprt(p: float, e: float, c: float, kmax: int, rng: random.Random, a: float | None = None) -> tuple[bool, int]:
    """Ein Lauf mit Fehlerrate p je Treffer: (angenommen, gepruefte Stellen).
    a: Annahme-Schwelle (Vorgabe A; der Selbsttest misst an einer kleinen)."""
    a = A if a is None else a
    hoch, tief = schritte(e, c)
    L = 0.0
    k = 0
    while k < kmax:
        for _ in range(TEIL):
            if rng.random() < 0.5:
                L += hoch if rng.random() < p else tief
        k += TEIL
        if L <= -a:
            return True, k
        if L >= B:
            return False, k
    return False, k


def entscheidungen(e: float, c: float, kmax: int, laeufe: int, saat: int = 1) -> list[tuple[bool, int]]:
    """Simulierte Laeufe echter Noten: (angenommen, Stellen bis zur Entscheidung)."""
    rng = random.Random(saat)
    return [sprt(e, e, c, kmax, rng) for _ in range(laeufe)]


def mittel_echt(e: float, c: float, kmax: int, laeufe: int, saat: int = 1) -> tuple[float, float]:
    """(mittlere Stellen einer echten Note, Anteil durchgefallen) per Simulation."""
    r = entscheidungen(e, c, kmax, laeufe, saat)
    return sum(k for _, k in r) / len(r), sum(not ok for ok, _ in r) / len(r)


def zwei_stufen(laeufe: list[tuple[bool, int]], kmax: int) -> tuple[int, float, float]:
    """Ziehung in zwei Stufen: erst s1 Stellen; ist der Test danach nicht
    entschieden, eine zweite Ziehung bis k_max (der Halter hat dann alle
    k_max gemessen). Bestes s1 und die erwarteten Stellen einer echten Note,
    dazu der Anteil, der eine zweite Stufe braucht."""
    best = (kmax, float(kmax), 0.0)
    for s1 in range(TEIL, kmax + 1, TEIL):
        zweite = sum(k > s1 for _, k in laeufe) / len(laeufe)
        erwartet = s1 + zweite * (kmax - s1)
        if erwartet < best[1]:
            best = (s1, erwartet, zweite)
    return best


def zwei_ziehungen(e: float, f: float) -> int | None:
    """Stellen fuer zwei unabhaengige feste Pruefungen zu je 2^-20 (zusammen
    2^-40), echte Note je hoechstens ZIEL_ECHT/2."""
    alt = wp.ZIEL_FAELSCHER, wp.ZIEL_EHRLICH
    try:
        wp.ZIEL_FAELSCHER, wp.ZIEL_EHRLICH = 2.0 ** -20, ZIEL_ECHT / 2
        r = wp.suche(e, f)
    finally:
        wp.ZIEL_FAELSCHER, wp.ZIEL_EHRLICH = alt
    return None if r is None else 2 * r["k"]


# ---------------------------------------------------------------- Selbsttest
def selbsttest() -> bool:
    fehler: list[str] = []

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    try:
        e, f = 0.007, 0.0
        c = wp.faelscher_fehler(f)
        km = k_max(e, c)
        soll(km is not None and km % TEIL == 0, f"k_max: {km}")
        # Die strenge Schranke haelt das Ziel, eine Stufe weniger nicht.
        soll(math.exp(-B) + chernoff_unentschieden(km, e, c) <= ZIEL_ECHT, "Schranke bei k_max")
        soll(math.exp(-B) + chernoff_unentschieden(km - TEIL, e, c) > ZIEL_ECHT, "k_max nicht minimal")
        # Chernoff ist eine echte obere Schranke und nicht beliebig lose
        # (exakt nachgerechnet bei kleinem k und kleiner Schwelle).
        for kk, aa in ((256, math.log(20)), (512, math.log(1000))):
            g, ch = genau_unentschieden(kk, e, c, aa), chernoff_unentschieden(kk, e, c, aa)
            soll(g <= ch <= max(50 * g, 1e-12), f"Chernoff bei k={kk}: genau {g:.3e}, Schranke {ch:.3e}")
        # Faelscher-Schwelle genau 2^-40.
        soll(abs(math.exp(-A) - 2.0 ** -40) < 1e-24, "A")
        # Simulation: echte Noten fallen (fast) nie durch; ein Faelscher mit
        # deutlich schlechterer Annahme-Schwelle (A klein) wird je Lauf mit
        # hoechstens e^-A angenommen -- an einem kleinen A gemessen.
        m, durch = mittel_echt(e, c, km, 2000)
        soll(durch == 0.0, f"echte Noten durchgefallen: {durch}")
        soll(m < km, f"im Mittel {m} Stellen, k_max {km}")
        klein = math.log(20)                    # Schranke 5 %
        rng = random.Random(7)
        angen = sum(sprt(c, e, c, 4096, rng, klein)[0] for _ in range(4000)) / 4000
        soll(angen <= 0.05 + 3 * math.sqrt(0.05 * 0.95 / 4000), f"Faelscher bei A=ln20 zu oft angenommen: {angen}")
        soll(angen > 0.005, f"Gegenprobe: Faelscher nie angenommen ({angen}) -- die Messung prueft nichts")
        # Monoton: ein schlechterer Faelscher (Rate 2c) seltener.
        angen2 = sum(sprt(min(2 * c, 0.5), e, c, 4096, rng, klein)[0] for _ in range(4000)) / 4000
        soll(angen2 <= angen, f"schlechterer Faelscher oefter angenommen: {angen2} > {angen}")
        # Walds Naeherung liegt nahe an der Simulation (Ueberschuss je Teil).
        wald = 2 * A / kl(e, c)
        soll(0.8 * wald <= m <= wald + 2 * TEIL, f"Wald {wald:.0f} gegen Simulation {m:.0f}")
        # Zwei Ziehungen sparen nichts gegenueber einer festen Pruefung.
        r = wp.suche(e, f)
        z = zwei_ziehungen(e, f)
        soll(r is not None and z is not None and z >= 0.9 * r["k"], f"zwei Ziehungen {z} gegen fest {r and r['k']}")
        # Knappes Budget: der Fehlalarm (e^-B = 5e-7) zaehlt mit.
        eng = k_max(e, c, ziel=6e-7)
        soll(eng is not None and math.exp(-B) + chernoff_unentschieden(eng, e, c) <= 6e-7, f"k_max bei knappem Budget: {eng}")
        # Ein Faelscher wird frueh abgelehnt, nicht erst bei k_max.
        rng2 = random.Random(3)
        fl = [sprt(c, e, c, km, rng2) for _ in range(500)]
        soll(sum(k for _, k in fl) / len(fl) < km / 2 and not any(ok for ok, _ in fl), "Faelscher nicht frueh abgelehnt")
        # Zwei Stufen: nie mehr als k_max, im Standardfall weniger als die
        # feste Pruefung; wer eine zweite Stufe braucht, verbraucht k_max.
        laeufe = entscheidungen(e, c, km, 2000)
        s1, zm, zw = zwei_stufen(laeufe, km)
        soll(TEIL <= s1 <= km and zm <= km and 0 <= zw <= 1, f"zwei Stufen: {s1} {zm} {zw}")
        soll(abs(zm - (s1 + sum(k > s1 for _, k in laeufe) / len(laeufe) * (km - s1))) < 1e-9 and (zw == 0 or zm > s1),
             f"zwei Stufen: Erwartung {zm} bei s1 {s1}, zweite {zw}")
        soll(zm < r["k"], f"zwei Stufen ({zm:.0f}) nicht besser als fest ({r['k']})")
        # Keine Toleranz ohne Abstand: e >= c -> kein Test.
        soll(k_max(0.2, 0.146) is None, "e >= c muss scheitern")
    except Exception as ex:  # noqa: BLE001
        fehler.append(f"Ausnahme: {ex!r}")
    for f_ in fehler:
        print("FEHLER:", f_)
    print("wiesner_mehrstufig: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def main():
    print("B5: feste Pruefung gegen sequenziellen Test (Faelscher <= 2^-40, echt faellt <= 1e-6)")
    print(f"{'e':>6} | {'f':>5} | {'c':>6} | {'fest k':>6} | {'2 Ziehungen':>11} | {'SPRT k_max':>10} | "
          f"{'SPRT Mittel echt':>16} | {'Wald 2A/D':>9} | {'2 Stufen: s1, Mittel, zweite':>28} | {'Ersparnis':>9}")
    for e in (0.004, 0.007, 0.02):
        for f, name in ((0.0, "0"), (0.25, "1/4"), (0.5, "1/2"), (2 / 3, "2/3")):
            c = wp.faelscher_fehler(f)
            r = wp.suche(e, f)
            km = k_max(e, c)
            if r is None or km is None:
                print(f"{e:>6.1%} | {name:>5} | {c:>6.2%} | {'--':>6} | -- (kein Abstand zwischen e und c)")
                continue
            z = zwei_ziehungen(e, f)
            laeufe = entscheidungen(e, c, km, 3000)
            m = sum(k for _, k in laeufe) / len(laeufe)
            wald = 2 * A / kl(e, c)
            s1, zm, zw = zwei_stufen(laeufe, km)
            stufen = f"{s1}, {zm:.0f}, {zw:.1%}"
            print(f"{e:>6.1%} | {name:>5} | {c:>6.2%} | {r['k']:>6} | {z if z else '--':>11} | {km:>10} | "
                  f"{m:>16.0f} | {wald:>9.0f} | {stufen:>28} | {1 - zm / r['k']:>8.0%}")


if __name__ == "__main__":
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    main()
