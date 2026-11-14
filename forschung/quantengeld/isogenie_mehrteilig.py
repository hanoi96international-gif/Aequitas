#!/usr/bin/env python3
"""Mehrteilige, rauschfeste Isogenie-Note -- scharf statt 1/h.

Forschung, kein Produkt (README.md, Abschnitt 3d). Keine Abhaengigkeiten.
Baut auf isogenie_geld.py auf (echte Isogenie-Gruppenaktion).

AUFBAU. Eine Banknote besteht aus k unabhaengig gedruckten Teilnoten
|$_{s_1}>, ..., |$_{s_k}>; ihre Seriennummer ist (s_1, ..., s_k). Geprueft
wird jede Teilnote oeffentlich (Phasenschaetzung); die Note gilt, wenn
mindestens tau Teilnoten bestehen. Die Toleranz k - tau schluckt Rauschen
(rauschen_oeffentlich.py).

SICHERHEIT -- zwei Angriffe, beide hier EXAKT gerechnet (obere Schranken):

  A. Faelschen ohne Note. Ein Faelscher, der zu keiner Teilnote den Zustand
     herstellen kann (Annahme: Vektorisierung ist schwer), besteht je Teilnote
     hoechstens mit 1/h -- fuer JEDEN Zustand gemittelt ueber die zufaellige
     Seriennummer, weil {|$_s>} eine Orthonormalbasis ist (am Zustandsvektor
     geprueft). Also P[A] <= P[Bin(k, 1/h) >= tau].

  B. Aufteilen. Wer EINE echte Note hat, verteilt ihre k echten Teilnoten auf
     zwei Noten (a und k - a) und fuellt den Rest mit Faelschungen. Selbst wenn
     jede echte Teilnote sicher besteht, brauchen beide zusammen mindestens
     2*tau - k bestandene Faelschungen. Ist tau nur "mehr als die Haelfte",
     genuegt EINE -- bei h = 1491 eine Chance von 1:250. Deshalb braucht die
     Schwelle einen Abstand d = 2*tau - k ueber der Haelfte:
        P[B] <= max_a P[Bin(k-a, 1/h) >= tau-a] * P[Bin(a, 1/h) >= tau-(k-a)].
     (Kopieren einer Teilnote ist nach Annahme so schwer wie Vektorisieren.)

ERGEBNIS (main): mit der echten Gruppenaktion p = 868019 (h = 1491) eine Note
aus k Teilen, deren Faelschung <= 1e-12 ist und die trotzdem Rauschen
vertraegt -- und dieselbe Rechnung fuer kryptografische Groesse (h ~ 2^256),
wo schon d = 1 genuegt.

Aufruf: python3 -I isogenie_mehrteilig.py [--selbsttest]
"""
from __future__ import annotations

import importlib.util
import math
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("isogenie_geld", os.path.join(_hier, "isogenie_geld.py"))
iso = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(iso)

GROSS_P = 868019
GROSS_ELLS = (3, 5, 17, 23, 37)

ZIEL_FAELSCHUNG = 1e-12
ZIEL_EHRLICH = 0.99


def log_binom_sf(n: int, k: int, q: float) -> float:
    """log P[Bin(n, q) >= k] (stabil, auch fuer winzige q)."""
    if k <= 0:
        return 0.0
    if k > n:
        return -math.inf
    lq, lr = math.log(q), math.log1p(-q)
    log_c = [0.0] * (n + 1)
    for i in range(1, n + 1):
        log_c[i] = log_c[i - 1] + math.log(n - i + 1) - math.log(i)
    t = [log_c[i] + i * lq + (n - i) * lr for i in range(k, n + 1)]
    m = max(t)
    return m + math.log(sum(math.exp(x - m) for x in t))


def faelschen(k: int, tau: int, h: float) -> float:
    """Angriff A: obere Schranke."""
    return math.exp(log_binom_sf(k, tau, 1.0 / h))


def aufteilen(k: int, tau: int, h: float) -> float:
    """Angriff B: obere Schranke (echte Teilnoten bestehen sicher)."""
    beste = 0.0
    for a in range(0, k + 1):
        lp = log_binom_sf(k - a, tau - a, 1.0 / h) + log_binom_sf(a, tau - (k - a), 1.0 / h)
        beste = max(beste, math.exp(lp) if lp > -math.inf else 0.0)
    return beste


def ehrlich(k: int, tau: int, s: float) -> float:
    """Echte Note besteht, wenn jede Teilnote mit s besteht."""
    return math.exp(log_binom_sf(k, tau, s)) if s < 1 else 1.0


def beste_parameter(h: float, s: float, ziel: float = ZIEL_FAELSCHUNG, k_max: int = 600):
    """Kleinstes k (mit tau), das beide Ziele erreicht -- oder None."""
    for k in range(1, k_max + 1):
        for tau in range(k // 2 + 1, k + 1):
            if ehrlich(k, tau, s) < ZIEL_EHRLICH:
                break  # hoeheres tau macht es nur schlechter
            if faelschen(k, tau, h) <= ziel and aufteilen(k, tau, h) <= ziel:
                return k, tau
    return None


def selbsttest() -> None:
    rng = random.Random(868)
    # 1. Die echte Gruppenaktion in gross: Bahn, Erzeuger, Note, 1/h.
    iso.P, iso.ELLS = GROSS_P, GROSS_ELLS
    geld = iso.Geld(rng)
    assert geld.h == 1491 and iso.ELLS[geld.erzeuger] == 3, (geld.h, geld.erzeuger)
    s, note = geld.drucken()
    assert abs(geld.pruefen(s, note) - 1) < 1e-9
    # Jeder klassische Zustand besteht im Mittel ueber die Seriennummer mit
    # genau 1/h (Orthonormalbasis) -- hier fuer drei Kurven, alle Seriennummern.
    for A in (geld.kurven[0], geld.kurven[700], geld.kurven[-1]):
        mittel = sum(geld.pruefen(y, {A: 1.0}) for y in range(geld.h)) / geld.h
        assert abs(mittel - 1 / geld.h) < 1e-12
    # 2. Mehrteilig: k echte Teilnoten bestehen alle.
    teile = [geld.drucken() for _ in range(5)]
    assert all(abs(geld.pruefen(si, ni) - 1) < 1e-9 for si, ni in teile)
    # 3. Bloss "mehr als die Haelfte" faellt dem Aufteilen zum Opfer.
    assert aufteilen(11, 6, 1491) > 1e-3
    # 4. Mit Abstand haelt die Note: Schranken fallen geometrisch mit d.
    assert aufteilen(15, 10, 1491) < aufteilen(15, 9, 1491) < aufteilen(15, 8, 1491)
    # 5. Exakte Schranke A gegen Formel fuer tau = k (alle muessen bestehen).
    assert abs(faelschen(4, 4, 10) - 1e-4) < 1e-15
    # 6. Ziele erreichbar, mit und ohne Rauschen.
    for s_teil in (1.0, 0.9):
        kt = beste_parameter(1491, s_teil)
        assert kt is not None
        k, tau = kt
        assert faelschen(k, tau, 1491) <= ZIEL_FAELSCHUNG and aufteilen(k, tau, 1491) <= ZIEL_FAELSCHUNG
        assert ehrlich(k, tau, s_teil) >= ZIEL_EHRLICH
    k, tau = beste_parameter(1491, 0.9, 2.0 ** -128)
    assert aufteilen(k, tau, 1491) <= 2.0 ** -128 and ehrlich(k, tau, 0.9) >= ZIEL_EHRLICH
    print(f"selbsttest ok (p = 868019, h = 1491; 2^-128 bei 90 % je Teilnote: k = {k}, tau = {tau})")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    print("Echte Note besteht >= 99 %; Faelschen und Aufteilen je <= Ziel\n")
    print("| Gruppe | Ziel | Teilnote besteht | Teile k | Schwelle tau | Abstand d | Faelschen | Aufteilen |")
    print("|---|---|---|---|---|---|---|---|")
    for name, h in (("p = 868019 (h = 1.491)", 1491.0), ("kryptografisch (h ~ 2^256)", 2.0 ** 256)):
        for zname, ziel in (("2^-128", 2.0 ** -128), ("2^-192", 2.0 ** -192)):
            for s_teil in (1.0, 0.9, 0.75):
                kt = beste_parameter(h, s_teil, ziel)
                if kt is None:
                    print(f"| {name} | {zname} | {s_teil:.2f} | – | – | – | – | – |")
                    continue
                k, tau = kt
                print(f"| {name} | {zname} | {s_teil:.2f} | {k} | {tau} | {2 * tau - k} | {faelschen(k, tau, h):.1e} | {aufteilen(k, tau, h):.1e} |")


if __name__ == "__main__":
    main()
