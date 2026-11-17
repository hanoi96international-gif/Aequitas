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
  - S3, Ausgabe ueber eine echte Leitung (Kent u. a. 2022, arXiv:2104.11717):
    faelscher_schranke(theta, beta) (Faelscher-Fehlerrate bei verkippten Basen,
    14,64 % nur im Idealfall), p_mehrphoton(mu), kent_qt1() (Lemma 2, Lemma 3,
    Satz 1 der Arbeit; der Selbsttest rechnet ihre Abb. 5 nach) und
    s3_stichprobe() (unsere Stichprobe je Annahmestelle: Mehrphotonen-Stellen
    zaehlen als bekannt, der Rest mit lambda(theta)).

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


def breidbart_fehler() -> list[float]:
    """Variante S (S-Token, ANALYSE.md 5b): wer an ZWEI Stellen vorlegen will,
    braucht Ergebnisse in beiden Basen und misst am besten in der Basis
    zwischen X und Z (Breidbart, Winkel pi/8). Fehler je BB84-Zustand --
    dieselben 14,6 % wie der Kloner, also dieselbe Toleranzschwelle."""
    c, s = math.cos(math.pi / 8), math.sin(math.pi / 8)
    b0, b1 = (c, s), (-s, c)                    # Ergebnis 0 bzw. 1 in beiden Basen
    w = 1 / math.sqrt(2)
    zustaende = {((1, 0), 0), ((0, 1), 1), ((w, w), 0), ((w, -w), 1)}
    aus = []
    for psi, bit in zustaende:
        richtig = b0 if bit == 0 else b1
        aus.append(1 - (richtig[0] * psi[0] + richtig[1] * psi[1]) ** 2)
    return aus


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


def faelscher_fehler(f: float, schranke: float = KLON_FEHLER) -> float:
    """Fehlerrate je Treffer: bekannte Stellen (Anteil f) fehlerfrei, die
    anderen geklont. schranke: Fehlerrate des besten Faelschers auf einer
    unbekannten Stelle (ideal der Kloner; ueber eine echte Leitung kleiner,
    faelscher_schranke)."""
    return (1 - f) * schranke


def _ok(k: int, e: float, c: float, tol: int, mt: int) -> tuple[bool, float, float]:
    fa = bedingt(k, c, tol, mt)
    eh, uh = annahme(k, e, tol, mt)
    faellt = 1 - eh - uh            # unentschieden: Pfand zurueck, kein Schaden
    return fa <= ZIEL_FAELSCHER and faellt <= ZIEL_EHRLICH, fa, faellt


def suche(e: float, f: float = 0.0, k_max: int = 8192, min_treffer: int = 1,
          schranke: float = KLON_FEHLER) -> dict | None:
    """Kleinste Pruefung (k, in Achterschritten) ueber alle ganzzahligen
    Toleranzen zwischen e und der Faelscher-Fehlerrate, die beide Ziele
    erreicht. None, wenn es keine solche Toleranz gibt."""
    if not 0 <= f < 1:
        return None
    c = faelscher_fehler(f, schranke)
    best = None
    for tol in range(1, 100):
        if tol / 100 >= c:
            break
        if tol / 100 <= e:
            continue
        if best is not None and not _ok(best["k"], e, c, tol, min_treffer)[0]:
            continue                      # kann die beste Pruefung nicht unterbieten
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


# ------------------------------------------- S3: Ausgabe ueber eine echte Leitung
#
# Ueber eine echte Quantenleitung (QKD-Technik) kommt nur ein kleiner Teil der
# Pulse an (P_det), ein schwacher Laserpuls enthaelt manchmal zwei Photonen
# (P_noqub), und die Basen sind um einen Winkel theta verkippt. Quelle:
# Kent, Lowndes, Pitalua-Garcia, Rarity, „Practical quantum tokens without
# quantum memories and experimental tests“, npj Quantum Inf. 8, 28 (2022),
# arXiv:2104.11717, Schema QT1: Lemma 2 (Robustheit), Lemma 3 (Korrektheit),
# Satz 1 (Unfaelschbarkeit), Tabellen V und VI. Der Selbsttest rechnet Abb. 5
# der Arbeit nach (beta_max 6e-4 / 2,3e-4 / 6e-6).
#
# Folgerung fuer Aequitas: Die Nutzerin meldet die Menge der angekommenen Pulse
# (Lambda, mindestens gamma_det * N). Mehrphotonen-Pulse kann eine Betruegerin in
# beiden Basen messen; sie meldet sie bevorzugt. Unter den gemeldeten Stellen sind
# also hoechstens nu_unf / gamma_det „frei“ -- genau der Anteil f bekannter Stellen,
# den suche() schon kennt. Auf den uebrigen Stellen hat der Faelscher mindestens die
# Fehlerrate lambda(theta, beta_PB) statt 14,6 %.

def ueberlapp(theta_grad: float) -> float:
    """O(theta) (Kent u. a., Tabelle V Nr. 2): groesste Ueberlappung zweier
    Zustaende verschiedener Basen, wenn die Basen bis auf theta verkippt sind."""
    t = math.radians(theta_grad)
    return (math.cos(t) + math.sin(t)) / math.sqrt(2)


def faelscher_schranke(theta_grad: float = 0.0, beta_pb: float = 0.0) -> float:
    """lambda(theta, beta_PB) (Kent u. a., Tabelle V Nr. 3): Fehlerrate je
    Treffer, die ein Faelscher auf einer unbekannten Stelle mindestens hat.
    theta = beta = 0 ergibt genau den Kloner (14,64 %); Verkippung und schiefe
    Basenwahl machen es dem Faelscher leichter."""
    o = ueberlapp(theta_grad)
    return 0.5 * (1 - math.sqrt(1 - (1 - o * o) * (1 - 4 * beta_pb ** 2)))


def p_mehrphoton(mu: float) -> float:
    """Anteil der Pulse mit zwei oder mehr Photonen bei einer Poisson-Quelle
    (schwacher Laserpuls, mittlere Photonenzahl mu)."""
    return 1 - (1 + mu) * math.exp(-mu)


def _kent_f(g_err: float, b_ps: float, b_pb: float, theta: float, nu: float, g_det: float) -> float:
    """f aus Satz 1, Gl. (8)-(9). Die Klammer (gamma_det - nu) * [...] umfasst
    beide ersten Glieder; so ergibt sich Abb. 5 der Arbeit."""
    lam = faelscher_schranke(theta, b_pb)
    if not 0 < nu < g_det:
        return -1.0
    d = g_det * g_err / (g_det - nu)
    if d >= lam:
        return -1.0
    h = 2 * b_ps * math.sqrt(0.5 + 2 * b_pb ** 2 + (0.5 - 2 * b_pb ** 2) * math.sin(math.radians(2 * theta)))
    return (g_det - nu) * (lam / 2 * (1 - d / lam) ** 2 - math.log(1 + 2 * b_ps)) - (1 - (g_det - nu)) * math.log(1 + h)


def kent_qt1(n_puls: float, p_det: float, e: float, p_noqub: float, theta_grad: float,
             beta_pb: float, beta_ps: float, gamma_det: float, ziel: float = 1e-9) -> dict | None:
    """Erfuellt eine Leitung mit diesen Messwerten Robustheit, Korrektheit und
    Unfaelschbarkeit je <= ziel (Lemmas 2, 3, Satz 1)? Wahl der freien Groessen wie
    in der Arbeit: jeder Summand ziel/2, gamma_err so klein wie die Korrektheit
    erlaubt (f faellt mit gamma_err). None, wenn nicht."""
    if not (0 < gamma_det < p_det < 1 and 0 < e < 0.5 and p_noqub > 0 and n_puls > 0):
        return None
    l2 = math.log(2.000001 / ziel)    # je Summand knapp unter ziel/2 (Rundung)
    rob = math.exp(-p_det * n_puls / 2 * (1 - gamma_det / p_det) ** 2)
    a = p_det * (1 - 2 * beta_pb)
    nu_cor = a / 2 * (1 - math.sqrt(4 * l2 / (a * n_puls)))
    if nu_cor <= 0:
        return None
    g_err = e * (1 + math.sqrt(3 * l2 / (e * nu_cor * n_puls)))
    nu = p_noqub * (1 + math.sqrt(3 * l2 / (p_noqub * n_puls)))
    lam = faelscher_schranke(theta_grad, beta_pb)
    if not (g_err < 2 * e and g_err < lam and nu < min(2 * p_noqub, gamma_det * (1 - g_err / lam))):
        return None
    f = _kent_f(g_err, beta_ps, beta_pb, theta_grad, nu, gamma_det)
    unf = math.exp(-p_noqub * n_puls / 3 * (nu / p_noqub - 1) ** 2) + (math.exp(-n_puls * f) if f > 0 else 1.0)
    cor = math.exp(-a * n_puls / 4 * (1 - 2 * nu_cor / a) ** 2) + math.exp(-e * nu_cor * n_puls / 3 * (g_err / e - 1) ** 2)
    if max(rob, cor, unf) > ziel:
        return None
    return {"gamma_err": g_err, "nu_unf": nu, "nu_cor": nu_cor, "lambda": lam,
            "rob": rob, "cor": cor, "unf": unf, "frei": nu / gamma_det}


def s3_stichprobe(e: float, theta_grad: float, p_noqub: float, gamma_det: float,
                  n_puls: float, f_absprache: float = 0.0, beta_pb: float = 0.0,
                  ziel: float = 1e-9) -> dict | None:
    """Pruefgroesse der Variante S (geheime Stichprobe je Annahmestelle) fuer
    eine Ausgabe ueber eine echte Leitung: frei sind die Mehrphotonen-Stellen
    (hoechstens nu_unf / gamma_det der gemeldeten, mit derselben Sicherheitsmarge
    wie Satz 1) plus der Anteil einer Absprache; auf den anderen Stellen hat der
    Faelscher mindestens lambda(theta, beta_PB). Rechnet dann suche()."""
    if not (0 < gamma_det < 1 and p_noqub >= 0 and n_puls > 0):
        return None
    nu = p_noqub * (1 + math.sqrt(3 * math.log(2.000001 / ziel) / (p_noqub * n_puls))) if p_noqub > 0 else 0.0
    frei = f_absprache + nu / gamma_det
    r = suche(e, frei, schranke=faelscher_schranke(theta_grad, beta_pb))
    if r is not None:
        r["frei"], r["lambda"] = frei, faelscher_schranke(theta_grad, beta_pb)
    return r


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
        # Variante S: Doppelvorlage per Breidbart-Messung hat denselben Fehler.
        bf = breidbart_fehler()
        soll(len(bf) == 4 and all(abs(x - KLON_FEHLER) < 1e-12 for x in bf), f"Breidbart: {bf}")

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

        # 5. S3, Ausgabe ueber eine echte Leitung (Kent u. a. 2022).
        soll(abs(faelscher_schranke(0, 0) - KLON_FEHLER) < 1e-15, "lambda(0, 0) muss der Kloner sein")
        lams = [faelscher_schranke(t, 0) for t in (0, 2, 5, 10)]
        soll(all(x > y for x, y in zip(lams, lams[1:])) and faelscher_schranke(0, 0.01) < KLON_FEHLER,
             f"lambda muss mit Verkippung und Schieflage fallen: {lams}")
        soll(abs(p_mehrphoton(0.09) - 3.8e-3) < 5e-5, f"P_noqub(0,09) = {p_mehrphoton(0.09)} (Arbeit: 3,8e-3)")
        # Abb. 5 der Arbeit nachgerechnet: beta_max bei (theta, E) = (0, 1 %), (5, 3 %),
        # (10, 5,8 %) etwa 6e-4, 2,3e-4, 6e-6 (N = 4e7, P_det = 0,019, gamma_det = 0,018).
        def kent(theta, e, b):
            return kent_qt1(4e7, 0.019, e, p_mehrphoton(0.09), theta, b, b, 0.018)
        for theta, e, b in ((0, 0.01, 6e-4), (5, 0.03, 2.3e-4), (10, 0.058, 6e-6)):
            soll(kent(theta, e, 0.8 * b) is not None and kent(theta, e, 1.3 * b) is None,
                 f"Abb. 5 nicht nachgerechnet bei theta = {theta}, E = {e}")
        # Die gemessenen Werte der Arbeit (beta_PB = 2,4e-3, beta_PS = 3,6e-3) reichen
        # nicht -- so schreiben es auch die Autoren. Mit Basen und Werten aus dem
        # Zufall der Validatoren (beta = 0) reicht derselbe Handsender.
        soll(kent_qt1(4e7, 0.019, 0.058, p_mehrphoton(0.09), 10, 2.4e-3, 3.6e-3, 0.018) is None,
             "gemessene Schieflage darf nicht als sicher gelten")
        soll(kent(10, 0.058, 0.0) is not None, "Handsender mit beta = 0 sollte nach Satz 1 sicher sein")
        # Missbrauchstest: die Tabelle fuer ideale Qubits (14,6 %) ueber eine Leitung
        # mit 10 Grad Verkippung -- ein Faelscher kaeme bei e = 3 % in rund 1 % durch.
        r = suche(0.03)
        soll(bedingt(r["k"], faelscher_schranke(10), r["tol"], 1) > 1e-3,
             "Ideal-Tabelle bei 10 Grad muesste brechen")
        # Gegenprobe: s3_stichprobe rechnet mit lambda(theta) und den Mehrphotonen-Stellen.
        s5 = s3_stichprobe(0.03, 5, p_mehrphoton(0.09), 0.018, 4e7)
        s0 = s3_stichprobe(0.03, 5, 1e-10, 0.018, 4e7)
        soll(s5 is not None and s0 is not None and s5["k"] > s0["k"] > r["k"]
             and bedingt(s5["k"], (1 - s5["frei"]) * faelscher_schranke(5), s5["tol"], 1) <= ZIEL_FAELSCHER,
             f"S3: {s5} {s0}")
        soll(s3_stichprobe(0.058, 10, p_mehrphoton(0.09), 0.018, 4e7) is None,
             "gemessener Handsender (5,8 %, 10 Grad) passt nicht in k <= 8192")
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
    print()
    print("S3, AUSGABE UEBER EINE ECHTE LEITUNG (Kent u. a. 2022): Stichprobe je Annahmestelle")
    print("  frei = Mehrphotonen-Stellen unter den gemeldeten; lambda = Faelscher-Schranke bei Verkippung")
    print(f"  {'Leitung':<44} | {'E':>5} | {'theta':>5} | {'frei':>6} | {'lambda':>6} | Toleranz | {'k':>5}")
    for name, e, th, mu, pd, n_puls in (
            ("Handsender der Arbeit (mu 0,09, P_det 0,019)", 0.058, 10, 0.09, 0.019, 4e7),
            ("derselbe, verbessert", 0.03, 5, 0.09, 0.019, 4e7),
            ("schwacher Puls mu 0,02", 0.03, 5, 0.02, 0.0042, 2e8),
            ("schwacher Puls mu 0,02, besser justiert", 0.01, 2, 0.02, 0.0042, 2e8),
            ("Einzelphotonen (angekuendigt)", 0.01, 2, None, 0.05, 1e7)):
        pnq = p_mehrphoton(mu) if mu else 1e-10
        r = s3_stichprobe(e, th, pnq, 0.95 * pd, n_puls)
        nu = pnq * (1 + math.sqrt(3 * math.log(2.000001 / 1e-9) / (pnq * n_puls)))
        frei, lam = nu / (0.95 * pd), faelscher_schranke(th)
        print(f"  {name:<44} | {e:>5.1%} | {th:>4}° | {frei:>6.3f} | {lam:>6.4f} | "
              + (f"{r['tol']:>7}% | {r['k']:>5}" if r else "  nicht sicher bis k = 8192"))


if __name__ == "__main__":
    sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else main()
