#!/usr/bin/env python3
"""B6: Lohnt sich Fehlerkorrektur je Stelle einer Wiesner-Note -- und ab wann?

Forschung, kein Produkt. Keine Abhaengigkeiten.

FRAGE (OFFEN.md B6): Eine Note lebt auf IBM-Chips Mikrosekunden. Wird sie
laenger tragfaehig, wenn jede Stelle im Steane-Code [[7,1,3]] liegt
(wiesner_hardware.note_steane: 9 CNOT je Stelle, transversal gemessen,
klassisch dekodiert)?

WAS DIESES WERKZEUG RECHNET (exakt, ohne Zufall):
  Fehlermodell: depolarisierend nach jedem 1-Qubit-Gatter (p1), nach jedem
  CNOT (p2, eines der 15 Pauli-Paare), im Speicher je Qubit (pw), beim Messen
  ein gekipptes Bit (pm). Alle Gatter sind Clifford-Gatter, also reicht der
  Pauli-Rahmen: Fuer eine Messung in der Basis der Praegung zaehlt je Qubit
  nur, ob das Messbit kippt (X-Anteil vor der Messung; bei X-Basis die
  Z-Anteile, die die transversale Hadamard zu X macht). Der Rahmen hat 2^7
  Zustaende; das Werkzeug traegt die Wahrscheinlichkeit jedes Zustands exakt
  durch den Schaltkreis und dekodiert am Ende wie wiesner_hardware.
  - steane_fehler(p1, p2, pw, pm, cnot_faktor): logische Fehlerrate je Treffer,
    gemittelt ueber die vier BB84-Zustaende. cnot_faktor > 1: mehr CNOT durch
    das Gitter des Chips (auf ibm_marrakesh wurden aus 18 CNOT 43, Faktor 2,4).
  - roh_fehler(p1, pw, pm): dieselbe Stelle ungeschuetzt (1 Qubit).
  - fenster(...): fuer welche Speicherfehler pw der Schutz hilft.

ERGEBNIS IN WORTEN (main() druckt die Tabelle):
  - Der Codierer ist nicht fehlertolerant: Ein einzelner CNOT-Fehler kann die
    Stelle kippen. Darum gibt es eine Untergrenze -- unterhalb ist die
    ungeschuetzte Stelle besser.
  - Oberhalb von etwa 2 Fehlern je 7 Qubits versagt der Code (Abstand 3).
  - Dazwischen liegt ein Fenster. Auf heutigen IBM-Chips ist es schmal und
    liegt bei Wartezeiten von wenigen Mikrosekunden -- weit unter der Zeit
    einer Zahlung. Mit Gatterfehlern einer Ionenfalle wird es breit.

GRENZEN (ehrlich):
  - Depolarisierendes Modell, keine Leck- und Uebersprechfehler, keine
    korrelierten Fehler; das Gitter des Chips nur ueber cnot_faktor.
  - Nur ein Abschnitt Speicher, keine Korrektur waehrend des Haltens.
  - Die Sicherheit aendert der Code nicht (oeffentlich, dekodierbar).

Aufruf:  python3 -I wiesner_schutz.py [--selbsttest]
"""
from __future__ import annotations

import itertools
import sys

H = ((0, 0, 0, 1, 1, 1, 1), (0, 1, 1, 0, 0, 1, 1), (1, 0, 1, 0, 1, 0, 1))   # wie wiesner_hardware.STEANE_H
LOGISCH = (0, 1, 2)
CODIERER = ((3, (4, 5, 6)), (1, (2, 5, 6)), (0, (2, 4, 6)))                 # wie wiesner_hardware.steane_null
FLIP = 2.0 / 3.0          # depolarisierend: 2 der 3 Pauli kippen ein Messbit


def _syndrom(e: int) -> int:
    return sum((sum(H[r][j] * ((e >> j) & 1) for j in range(7)) & 1) << (2 - r) for r in range(3))


def _logisch_falsch(e: int) -> int:
    """1, wenn das Fehlermuster e (Bit j = Messbit j gekippt) nach der
    Korrektur den logischen Wert kippt."""
    s = _syndrom(e)
    if s:
        e ^= 1 << (s - 1)
    return bin(e).count("1") & 1


_FALSCH = [_logisch_falsch(e) for e in range(128)]


def _flip(v: list[float], q: int, p: float) -> list[float]:
    """Qubit q kippt mit Wahrscheinlichkeit p."""
    if p <= 0:
        return v
    m = 1 << q
    return [(1 - p) * v[e] + p * v[e ^ m] for e in range(128)]


def _cnot(v: list[float], c: int, t: int, p2: float) -> list[float]:
    """CNOT: der Kipp-Anteil der Kontrolle wandert zum Ziel; danach ein
    depolarisierender 2-Qubit-Fehler: von den 15 Pauli-Paaren kippen je 4
    nur c, nur t oder beide."""
    mc, mt = 1 << c, 1 << t
    v = [v[e ^ mt] if e & mc else v[e] for e in range(128)]        # e_t ^= e_c (Permutation)
    if p2 <= 0:
        return v
    a = p2 * 4 / 15
    return [(1 - 3 * a) * v[e] + a * (v[e ^ mc] + v[e ^ mt] + v[e ^ mc ^ mt]) for e in range(128)]


def _steane_zustand(basis: int, bit: int, p1: float, p2: float, pw: float, pm: float, cnot_faktor: float) -> float:
    v = [0.0] * 128
    v[0] = 1.0
    # Codierer |0_L>: Hadamard auf die Pivots, dann 9 CNOT. Zusaetzliche CNOT
    # durch das Gitter (cnot_faktor) gehen als erhoehter Fehler je CNOT ein.
    for piv, _ in CODIERER:
        v = _flip(v, piv, FLIP * p1)
    for piv, rest in CODIERER:
        for z in rest:
            v = _cnot(v, piv, z, min(1.0, p2 * cnot_faktor))
    if bit:
        for q in LOGISCH:
            v = _flip(v, q, FLIP * p1)
    if basis:
        # Transversale Hadamard vor und nach dem Speicher: H*H = 1 fuer den
        # Rahmen der Codierung; je Gatter ein Fehler mit p1.
        for q in range(7):
            v = _flip(v, q, FLIP * p1)
    for q in range(7):
        v = _flip(v, q, FLIP * pw)
    if basis:
        for q in range(7):
            v = _flip(v, q, FLIP * p1)
    for q in range(7):
        v = _flip(v, q, pm)
    return sum(p for e, p in enumerate(v) if _FALSCH[e])


def steane_fehler(p1: float, p2: float, pw: float, pm: float, cnot_faktor: float = 1.0) -> float:
    """Logische Fehlerrate je Treffer, gemittelt ueber |0>, |1>, |+>, |->."""
    return sum(_steane_zustand(b, x, p1, p2, pw, pm, cnot_faktor) for b in (0, 1) for x in (0, 1)) / 4


def _oder(*ps: float) -> float:
    """Wahrscheinlichkeit, dass ein Bit insgesamt gekippt ist (XOR)."""
    r = 0.0
    for p in ps:
        r = r * (1 - p) + p * (1 - r)
    return r


def roh_fehler(p1: float, pw: float, pm: float) -> float:
    """Ungeschuetzte Stelle (wiesner_hardware.note): X bei Bit 1, H bei Basis X
    vor und nach dem Speicher."""
    z0 = _oder(FLIP * pw, pm)
    z1 = _oder(FLIP * p1, FLIP * pw, pm)
    x0 = _oder(FLIP * p1, FLIP * pw, FLIP * p1, pm)
    x1 = _oder(FLIP * p1, FLIP * p1, FLIP * pw, FLIP * p1, pm)
    return (z0 + z1 + x0 + x1) / 4


def fenster(p1: float, p2: float, pm: float, cnot_faktor: float = 1.0) -> tuple[float, float] | None:
    """Speicherfehler pw (je Qubit, depolarisierend), fuer die der Code besser
    ist als die ungeschuetzte Stelle. None, wenn nie."""
    def vorteil(pw):
        # Am Rand (pw = 0, pw = 0,75) sind beide gleich; Rundung zaehlt nicht.
        d = roh_fehler(p1, pw, pm) - steane_fehler(p1, p2, pw, pm, cnot_faktor)
        return d if abs(d) > 1e-12 else 0.0
    gitter = [i / 2000 for i in range(0, 1501)]          # pw bis 0,75
    gut = [pw for pw in gitter if vorteil(pw) > 0]
    if not gut:
        return None
    def rand(a, b):                                     # vorteil wechselt zwischen a und b
        for _ in range(40):
            m = (a + b) / 2
            a, b = (m, b) if (vorteil(m) > 0) == (vorteil(a) > 0) else (a, m)
        return (a + b) / 2
    lo = 0.0 if vorteil(0.0) > 0 else rand(max(0.0, gut[0] - 1 / 2000), gut[0])
    hi = rand(gut[-1], min(0.75, gut[-1] + 1 / 2000)) if gut[-1] < 0.75 else 0.75
    return lo, hi


# Vorgaben (Groessenordnung, keine Messung einer bestimmten Maschine):
IBM_HERON = {"p1": 2.5e-4, "p2": 3e-3, "pm": 5e-3, "cnot_faktor": 2.4}
IONENFALLE = {"p1": 3e-5, "p2": 1.5e-3, "pm": 2e-3, "cnot_faktor": 1.0}


# ---------------------------------------------------------------- Selbsttest
def selbsttest() -> bool:
    fehler: list[str] = []

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    try:
        # 1. Ohne Fehler kein Fehler; jedes einzelne gekippte Bit wird korrigiert.
        soll(steane_fehler(0, 0, 0, 0) == 0 and roh_fehler(0, 0, 0) == 0, "ohne Rauschen")
        soll(all(_FALSCH[1 << j] == 0 for j in range(7)), "ein Fehler muss korrigiert werden")
        soll(_FALSCH[0b0000111] == 1 and _FALSCH[0b0000011] == 1, "logisches X / zwei Fehler")
        # Die 8 Codewoerter des Zeilenraums sind Stabilisatoren (kein Fehler).
        zeilen = [sum(H[r][j] << j for j in range(7)) for r in range(3)]
        for a, b, c in itertools.product((0, 1), repeat=3):
            e = (zeilen[0] * a) ^ (zeilen[1] * b) ^ (zeilen[2] * c)
            soll(_FALSCH[e] == 0, f"Stabilisator {e:07b} als Fehler gezaehlt")

        # 2. Nur Speicher- und Messfehler, unabhaengig je Qubit: Abzaehlen aller
        # 128 Muster muss dasselbe geben wie der Rahmen.
        for q in (0.01, 0.1, 0.3):
            pw = q / FLIP
            direkt = sum(q ** bin(e).count("1") * (1 - q) ** (7 - bin(e).count("1")) for e in range(128) if _FALSCH[e])
            soll(abs(steane_fehler(0, 0, pw, 0) - direkt) < 1e-12, f"Rahmen gegen Abzaehlen bei q = {q}")

        # 3. Ordnung: Speicher und Messen gehen quadratisch ein (Abstand 3), der
        # Codierer linear (nicht fehlertolerant) -- genau darum die Untergrenze.
        a, b = steane_fehler(0, 0, 1e-4, 0), steane_fehler(0, 0, 2e-4, 0)
        soll(3.5 < b / a < 4.5, f"Speicher nicht quadratisch: {b / a}")
        a, b = steane_fehler(0, 1e-5, 0, 0), steane_fehler(0, 2e-5, 0, 0)
        soll(1.8 < b / a < 2.2, f"Codierer nicht linear: {b / a}")

        # 3b. Unabhaengige Abzaehlung: jeder einzelne CNOT-Fehler (c, t oder beide
        # gekippt) wird von Hand durch die restlichen CNOT getragen. Der lineare
        # Anteil des Rahmens muss genau (4/15) * Zahl der schaedlichen sein.
        folge = [(piv, z) for piv, rest in CODIERER for z in rest]
        schaedlich = 0
        for k, (c, t) in enumerate(folge):
            for muster in ((1, 0), (0, 1), (1, 1)):
                e = [0] * 7
                e[c] ^= muster[0]
                e[t] ^= muster[1]
                for c2, t2 in folge[k + 1:]:
                    e[t2] ^= e[c2]
                schaedlich += _FALSCH[sum(b << j for j, b in enumerate(e))]
        lin = steane_fehler(0, 1e-7, 0, 0) / 1e-7
        soll(schaedlich > 0 and abs(lin - 4 / 15 * schaedlich) < 1e-3 * max(1.0, lin),
             f"linearer Anteil {lin:.4f} gegen Abzaehlung {4 / 15 * schaedlich:.4f} ({schaedlich} schaedliche Einzelfehler)")

        # 4. Roh: ein Qubit, ein Kippen.
        soll(abs(roh_fehler(0, 0.3, 0) - 0.2) < 1e-12, "roh: 2/3 von 0,3")

        # 5. Fenster: ohne Gatterfehler hilft der Code von pw = 0 an bis knapp
        # unter die Schwelle. Mit IBM-Werten und dem Gitter des Chips (Faktor
        # 2,4) hilft er nie -- wie im Rauschmodell; mit voller Verbindung und
        # Werten einer Ionenfalle in einem breiten Fenster; bei p2 = 5 % nie.
        f0 = fenster(0, 0, 0)
        soll(f0 is not None and f0[0] < 1e-9 and 0.05 < f0[1] < 0.6, f"Fenster ohne Gatterfehler: {f0}")
        fi, fe = fenster(**IBM_HERON), fenster(**IONENFALLE)
        soll(fi is None, f"IBM mit Gitter: kein Fenster erwartet, {fi}")
        soll(fe is not None and fe[0] < 0.005 and fe[1] > 0.05, f"Ionenfalle: Fenster erwartet, {fe}")
        fv = fenster(IBM_HERON["p1"], IBM_HERON["p2"], IBM_HERON["pm"], 1.0)
        soll(fv is not None and fv[1] - fv[0] < fe[1] - fe[0] + 0.02, f"IBM ohne Gitter: {fv}")
        soll(fenster(2.5e-4, 0.05, 5e-3) is None, "bei 5 % CNOT-Fehler darf der Code nie helfen")
        # Gegenprobe zum Rauschmodell (FakeMarrakesh, HEUTE.md 3.3): ohne Warten
        # ist die ungeschuetzte Stelle besser.
        soll(steane_fehler(**IBM_HERON, pw=0) > roh_fehler(IBM_HERON["p1"], 0, IBM_HERON["pm"]),
             "ohne Warten muesste ungeschuetzt besser sein")
    except Exception as ex:
        fehler.append(f"Ausnahme: {type(ex).__name__}: {ex}")
    for f in fehler:
        print("FEHLER:", f)
    print("wiesner_schutz: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def main():
    for name, par in (("IBM Heron (Groessenordnung, Gitter 2,4)", IBM_HERON), ("Ionenfalle (Groessenordnung)", IONENFALLE)):
        p1, pm = par["p1"], par["pm"]
        print(f"{name}: p1 = {p1:g}, p2 = {par['p2']:g}, pm = {pm:g}, CNOT-Faktor {par['cnot_faktor']:g}")
        print(f"  {'Speicherfehler pw':>18} | {'ungeschuetzt':>12} | {'Steane':>8}")
        for pw in (0, 0.003, 0.01, 0.03, 0.06, 0.1, 0.2, 0.33):
            r, s = roh_fehler(p1, pw, pm), steane_fehler(p1, par["p2"], pw, pm, par["cnot_faktor"])
            print(f"  {pw:>18.3f} | {r:>11.2%} | {s:>7.2%}" + ("  <- Code hilft" if s < r else ""))
        f = fenster(**par)
        print("  Fenster:", "keins" if f is None else f"pw von {f[0]:.4f} bis {f[1]:.3f} "
              f"(ungeschuetzt {roh_fehler(p1, f[0], pm):.2%} bis {roh_fehler(p1, f[1], pm):.2%} Fehler)")
        print()


if __name__ == "__main__":
    sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else main()
