#!/usr/bin/env python3
"""Fehlerbild einer S-Messung gegen die Kalibrierung des Chips (HEUTE.md 3.3l).

WOZU: Der Nachweis nach dem Ende (DEZENTRAL.md 17, R7) zeigt, dass genau
diese Praeparation bei der Ausgabe festgelegt war -- nicht, dass ein
Quantenrechner gemessen hat. Das Fehlerbild ist ein zweiter, unabhaengiger
Blick: Passen die Fehler der einzelnen Schuesse zu dem, was die
Tageskalibrierung der benutzten Qubits erwarten laesst?

  1. Passende Basis (y = u): Jeder Schuss zeigt t, ausser mit dem
     Auslesefehler p_q des Qubits q. Beobachtet E Fehler, erwartet
     mu = Summe R p_q. Viel weniger als erwartet ("zu glatt") passt zu einer
     Rechnung ohne Rauschen, viel mehr zu Drift, falschen Qubits oder einem
     Eingriff zwischen Praeparation und Messung.
  2. Wo die Fehler liegen: Likelihood-Verhaeltnis der Kalibrierung je Qubit
     gegen eine gleiche Rate fuer alle (positiv = die Fehler sitzen dort, wo
     die Kalibrierung sie erwartet). Bei wenigen Fehlern schwach -- nur
     berichtet, kein Urteil.
  3. Andere Basis (y != u): Jeder Schuss ist eine neue Praeparation und
     Messung, also ein faires Muenzwurf-Ergebnis, unabhaengig von t und von
     den anderen Schuessen. Geprueft: Anteil gleich t (muss 1/2 sein) und wie
     oft alle R Schuesse einer Stelle gleich sind (erwartet 2^(1-R)). Wer
     Schuesse kopiert oder t durchsickern laesst, faellt hier auf.

Veroeffentlicht werden nur diese Summen: Sie verraten keine Stelle, kein t
und kein x (die Zahl der Fehler bei passender Basis steht ohnehin in jeder
Ausgabe). Die rohen Schuesse verlassen den Messlauf nicht.

GRENZEN (ehrlich): "passt" heisst nur "nicht im Widerspruch". Ein Simulator
mit dem Rauschmodell derselben Kalibrierung (qiskit Fake-Backends) besteht
genauso -- das zeigt der Selbsttest an der Probe im Rauschmodell. Was sich
damit ausschliessen laesst, ist die einfache Faelschung (keine Fehler,
kopierte Schuesse, durchgesickertes t) und ein Chip, der schlechter misst
als kalibriert.

Aufruf: fehlerbild.py --selbsttest
"""
from __future__ import annotations

import json
import math
import random
import sys

ALPHA = 1e-3          # unter dieser Wahrscheinlichkeit ist ein Befund ein Befund
P_MIN, P_MAX = 1e-6, 0.5
MAX_STELLEN = 16384   # wie quantenWNMax
MAX_SCHUESSE = 15


class FehlerbildFehler(ValueError):
    pass


def _poisson_unten(k: int, lam: float) -> float:
    """P(X <= k) fuer X ~ Poisson(lam) (fuer kleine p eng an der Summe der Bernoulli)."""
    if lam <= 0:
        return 1.0
    s, term = 0.0, math.exp(-lam)
    for i in range(k + 1):
        if i > 0:
            term *= lam / i
        s += term
    return min(1.0, s)


def _poisson_oben(k: int, lam: float) -> float:
    """P(X >= k)."""
    return 1.0 if k <= 0 else max(0.0, 1.0 - _poisson_unten(k - 1, lam))


def _binom_oben(k: int, n: int, p: float) -> float:
    """P(X >= k) fuer X ~ Bin(n, p), ueber den Logarithmus (gross genug fuer n bis ~10^5)."""
    if k <= 0:
        return 1.0
    if k > n:
        return 0.0
    lp, lq = math.log(p), math.log1p(-p)
    s = 0.0
    for i in range(k, n + 1):
        s += math.exp(math.lgamma(n + 1) - math.lgamma(i + 1) - math.lgamma(n - i + 1) + i * lp + (n - i) * lq)
    return min(1.0, s)


def _binom_zweiseitig_halb(k: int, n: int) -> float:
    """Zweiseitig fuer Bin(n, 1/2) (Normalnaeherung mit Stetigkeitskorrektur; n gross)."""
    if n == 0:
        return 1.0
    z = (abs(k - n / 2) - 0.5) / math.sqrt(n / 4)
    return min(1.0, math.erfc(max(z, 0.0) / math.sqrt(2)))


def pruefen_eingabe(u, t, y, qubits, schuesse, auslesefehler, gatterfehler=None) -> int:
    """Form fail-closed: gleiche Laengen, Bits, je Stelle R Schuesse, je Qubit ein Fehler in (0, 1)."""
    n = len(u)
    if not (1 <= n <= MAX_STELLEN) or len(t) != n or len(y) != n or len(qubits) != n or len(schuesse) != n:
        raise FehlerbildFehler("Fehlerbild: Laengen passen nicht")
    r = len(schuesse[0]) if schuesse else 0
    if not (1 <= r <= MAX_SCHUESSE):
        raise FehlerbildFehler("Fehlerbild: Zahl der Schuesse")
    for v in (u, t, y):
        if any(b not in (0, 1) for b in v):
            raise FehlerbildFehler("Fehlerbild: keine Bits")
    for s in schuesse:
        if len(s) != r or any(b not in (0, 1) for b in s):
            raise FehlerbildFehler("Fehlerbild: Schuesse")
    for q in qubits:
        p = auslesefehler.get(q) if isinstance(auslesefehler, dict) else None
        if not isinstance(q, int) or q < 0 or p is None or not (0 < float(p) < 1):
            raise FehlerbildFehler(f"Fehlerbild: kein Auslesefehler fuer Qubit {q}")
    if gatterfehler is not None and (len(gatterfehler) != n or any(not (0 <= float(g) < 1) for g in gatterfehler)):
        raise FehlerbildFehler("Fehlerbild: Gatterfehler je Stelle")
    return r


def auswerten(u, t, y, qubits, schuesse, auslesefehler, gatterfehler=None, alpha: float = ALPHA) -> dict:
    """Die Summen des Fehlerbilds und das Urteil ("passt" oder Befunde).
    auslesefehler: je physischem Qubit; gatterfehler (optional): je Stelle die
    Summe der Fehler ihrer 1-Qubit-Gatter (Praeparation, Basiswechsel) aus
    derselben Kalibrierung. Erwartet je Schuss: Auslese plus Gatter."""
    r = pruefen_eingabe(u, t, y, qubits, schuesse, auslesefehler, gatterfehler)
    ro = {q: float(v) for q, v in auslesefehler.items()}
    g = [float(x) for x in gatterfehler] if gatterfehler is not None else [0.0] * len(u)
    pi_ = [min(P_MAX, max(P_MIN, ro[qubits[i]] + g[i])) for i in range(len(u))]
    passend = [i for i in range(len(u)) if y[i] == u[i]]
    andere = [i for i in range(len(u)) if y[i] != u[i]]
    # 1. Passende Basis: Fehler je Schuss gegen die Kalibrierung.
    e = {i: sum(1 for s in schuesse[i] if s != t[i]) for i in passend}
    beobachtet = sum(e.values())
    erwartet = sum(r * pi_[i] for i in passend)
    unten, oben = _poisson_unten(beobachtet, erwartet), _poisson_oben(beobachtet, erwartet)
    # 2. Wo die Fehler liegen (nur berichtet).
    lr = 0.0
    if passend and erwartet > 0:
        pq = erwartet / (r * len(passend))
        for i in passend:
            pi = pi_[i]
            lr += e[i] * math.log(pi / pq) + (r - e[i]) * (math.log1p(-pi) - math.log1p(-pq))
    # 3. Andere Basis: faire, unabhaengige Schuesse.
    gleich_t = sum(1 for i in andere for s in schuesse[i] if s == t[i])
    schuesse_andere = r * len(andere)
    fair = _binom_zweiseitig_halb(gleich_t, schuesse_andere)
    alle_gleich = sum(1 for i in andere if len(set(schuesse[i])) == 1)
    p_gleich = 2.0 ** (1 - r)
    kopiert = _binom_oben(alle_gleich, len(andere), p_gleich) if r > 1 else 1.0
    befunde = []
    if unten < alpha:
        befunde.append(f"weniger Fehler als die Kalibrierung erwarten laesst ({beobachtet} statt ~{erwartet:.1f}; P = {unten:.1e}) -- zu glatt")
    if oben < alpha:
        befunde.append(f"mehr Fehler als die Kalibrierung erwarten laesst ({beobachtet} statt ~{erwartet:.1f}; P = {oben:.1e})")
    if fair < alpha:
        befunde.append(f"andere Basis nicht fair: {gleich_t} von {schuesse_andere} Schuessen gleich t (P = {fair:.1e})")
    if kopiert < alpha:
        befunde.append(f"Schuesse nicht unabhaengig: an {alle_gleich} von {len(andere)} Stellen alle {r} gleich, erwartet ~{p_gleich * len(andere):.1f} (P = {kopiert:.1e})")
    return {
        "schuesse_je_stelle": r, "stellen": len(u), "passend": len(passend), "andere": len(andere),
        "fehler_beobachtet": beobachtet, "fehler_erwartet": round(erwartet, 3),
        "p_zu_wenige": unten, "p_zu_viele": oben, "lr_je_qubit": round(lr, 3),
        "andere_gleich_t": gleich_t, "andere_schuesse": schuesse_andere, "p_fair": fair,
        "andere_alle_gleich": alle_gleich, "andere_alle_gleich_erwartet": round(p_gleich * len(andere), 3), "p_kopiert": kopiert,
        "auslesefehler_mittel": round(sum(min(P_MAX, max(P_MIN, ro[q])) for q in qubits) / len(u), 6),
        "gatterfehler_mittel": round(sum(g) / len(u), 6),
        "qubits": len(set(qubits)),
        "urteil": "passt" if not befunde else "befund", "befunde": befunde,
    }


def text(fb: dict, wo: str = "") -> str:
    """Zusammenfassung fuer GITHUB_STEP_SUMMARY (nur Summen)."""
    z = [f"**Fehlerbild gegen die Kalibrierung{(' (' + wo + ')') if wo else ''}:** "
         + ("passt (kein Widerspruch zur Tageskalibrierung)" if fb["urteil"] == "passt" else "BEFUND")]
    z.append(f"- Passende Basis: {fb['fehler_beobachtet']} Fehler in {fb['passend'] * fb['schuesse_je_stelle']} Schuessen, "
             f"die Kalibrierung erwartet ~{fb['fehler_erwartet']:.1f} (mittlerer Auslesefehler {100 * fb['auslesefehler_mittel']:.2f} %, "
             f"Gatter {100 * fb['gatterfehler_mittel']:.3f} %, "
             f"{fb['qubits']} Qubits); P(so wenige) = {fb['p_zu_wenige']:.2g}, P(so viele) = {fb['p_zu_viele']:.2g}.")
    z.append(f"- Lage der Fehler: Likelihood-Verhaeltnis Kalibrierung je Qubit gegen gleiche Rate {fb['lr_je_qubit']:+.2f} (nur berichtet).")
    z.append(f"- Andere Basis: {fb['andere_gleich_t']} von {fb['andere_schuesse']} Schuessen gleich t (fair: 1/2, P = {fb['p_fair']:.2g}); "
             f"alle {fb['schuesse_je_stelle']} gleich an {fb['andere_alle_gleich']} Stellen, erwartet ~{fb['andere_alle_gleich_erwartet']:.1f} (P = {fb['p_kopiert']:.2g}).")
    for b in fb["befunde"]:
        z.append(f"- **Befund:** {b}")
    z.append("- Grenze: \"passt\" heisst nur \"kein Widerspruch\"; ein Simulator mit demselben Rauschmodell bestuende ebenso.")
    return "\n".join(z)


# ------------------------------------------------------------ Selbsttest

def _messung(rng: random.Random, n: int, r: int, p: dict, art: str = "ehrlich", faktor: float = 1.0):
    """Synthetische Messung: Qubits zyklisch ueber p, Basen und Werte zufaellig."""
    qs = sorted(p)
    u = [rng.randrange(2) for _ in range(n)]
    t = [rng.randrange(2) for _ in range(n)]
    y = [rng.randrange(2) for _ in range(n)]
    qubits = [qs[i % len(qs)] for i in range(n)]
    schuesse = []
    for i in range(n):
        if y[i] == u[i]:
            if art == "glatt":
                s = [t[i]] * r
            else:
                s = [t[i] ^ (1 if rng.random() < min(0.5, faktor * p[qubits[i]]) else 0) for _ in range(r)]
        elif art == "kopiert":
            s = [rng.randrange(2)] * r
        elif art == "durchgesickert":
            s = [t[i] if rng.random() < 0.6 else 1 - t[i] for _ in range(r)]
        else:
            s = [rng.randrange(2) for _ in range(r)]
        schuesse.append(s)
    return u, t, y, qubits, schuesse


def selbsttest() -> None:
    rng = random.Random(20261121)
    # Auslesefehler wie auf ibm_kingston (0,2 bis 1,5 %, Mittel ~0,5 %), 16 Qubits.
    p = {q: 0.002 + 0.013 * (q % 16) / 15 for q in range(100, 116)}
    n, r = 1104, 5
    # Ehrlich: in 40 Laeufen hoechstens einer mit Befund (alpha = 1e-3, vier Pruefungen).
    befunde = sum(auswerten(*_messung(rng, n, r, p), p)["urteil"] != "passt" for _ in range(40))
    assert befunde <= 1, f"ehrliche Messung {befunde} mal als Befund"
    fb = auswerten(*_messung(rng, n, r, p), p)
    assert 5 < fb["fehler_erwartet"] < 40, fb
    # Lage der Fehler: mit viel Statistik (n = 4000, R = 15) liegt die Kalibrierung vorn.
    viel = auswerten(*_messung(rng, 4000, 15, p), p)
    assert viel["lr_je_qubit"] > 0, viel
    # Zu glatt: keine Fehler, wo ~14 erwartet sind.
    fb = auswerten(*_messung(rng, n, r, p, "glatt"), p)
    assert fb["urteil"] == "befund" and "zu glatt" in fb["befunde"][0], fb
    # Zu viel Rauschen: viermal so viele Fehler.
    fb = auswerten(*_messung(rng, n, r, p, faktor=4.0), p)
    assert any("mehr Fehler" in b for b in fb["befunde"]), fb
    # Kopierte Schuesse bei der anderen Basis.
    fb = auswerten(*_messung(rng, n, r, p, "kopiert"), p)
    assert any("nicht unabhaengig" in b for b in fb["befunde"]), fb
    # t sickert in die andere Basis durch.
    fb = auswerten(*_messung(rng, n, r, p, "durchgesickert"), p)
    assert any("nicht fair" in b for b in fb["befunde"]), fb
    # Ein Schuss je Stelle: die Kopier-Pruefung entfaellt, der Rest geht.
    fb = auswerten(*_messung(rng, n, 1, p), p)
    assert fb["p_kopiert"] == 1.0 and fb["schuesse_je_stelle"] == 1, fb
    # Gatterfehler: eine Messung mit Auslese PLUS Gatter passt nur, wenn die
    # Gatter mitgerechnet werden (sonst "mehr Fehler").
    gf = 0.01
    def mit_gatter():
        u, t, y, q, s = _messung(rng, n, r, {k: v + gf for k, v in p.items()})
        return u, t, y, q, s
    m = mit_gatter()
    assert auswerten(*m, p, [gf] * n)["urteil"] == "passt"
    assert any("mehr Fehler" in b for b in auswerten(*m, p)["befunde"])
    # Fail-closed: Form.
    u, t, y, q, s = _messung(rng, 20, r, p)
    for kaputt in (
        lambda: auswerten(u[:-1], t, y, q, s, p),
        lambda: auswerten(u, t, y, q, s[:-1], p),
        lambda: auswerten(u, t, y, q, [x[:-1] for x in s[:-1]] + [s[-1]], p),
        lambda: auswerten(u, t, y, [999] + q[1:], s, p),
        lambda: auswerten(u, t, y, q, s, {k: 0.0 for k in p}),
        lambda: auswerten(u, t, y, q, s, {k: 1.0 for k in p}),
        lambda: auswerten([2] + u[1:], t, y, q, s, p),
        lambda: auswerten(u, t, y, q, [[2] * r] + s[1:], p),
        lambda: auswerten(u, t, y, q, [[0] * (MAX_SCHUESSE + 1)] * 20, p),
        lambda: auswerten([], [], [], [], [], p),
        lambda: auswerten(u, t, y, q, s, p, [0.0] * 19),
        lambda: auswerten(u, t, y, q, s, p, [-0.1] + [0.0] * 19),
    ):
        try:
            kaputt()
        except FehlerbildFehler:
            continue
        raise AssertionError("kaputte Eingabe angenommen")
    # Nur Summen im Ergebnis: keine Liste mit Stellen, kein t, kein x.
    fb = auswerten(*_messung(rng, n, r, p), p)
    assert not any(isinstance(v, (list, dict)) for k, v in fb.items() if k != "befunde"), fb
    json.dumps(fb)
    assert "passt" in text(fb, "Probe")
    print("fehlerbild: Selbsttest bestanden (ehrlich passt; zu glatt, zu viel Rauschen, kopiert, durchgesickert, Form abgewiesen)")


if __name__ == "__main__":
    if "--selbsttest" in sys.argv:
        selbsttest()
    else:
        print(__doc__)
