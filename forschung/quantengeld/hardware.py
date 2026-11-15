#!/usr/bin/env python3
"""Quantengeld auf echter Quanten-Hardware -- die Schaltkreise.

Forschung, kein Produkt (BAUVORGABE §8). Keine Abhaengigkeiten.

Ein Prüfgerät mit 10^8..10^10 logischen Qubits gibt es heute nirgends. Was
es gibt: Quantenrechner mit ~100 verrauschten Qubits (IBM, IonQ,
Quantinuum, ...). Darauf laeuft das Protokoll in SPIELZEUGGROESSE -- echte
Note, echte Pruefung, echte Faelschungen, echtes Rauschen. Das ist der
erste Hardware-Schritt: die Kette Praegen -> Pruefen laeuft auf einem
physischen Quantenrechner, und wir messen, wie gut.

SPIELZEUG: die Gruppe ist der Zyklus Z_N mit N = 2^n (wie die Zyklus-
Darstellung in schwache_pruefung.py, nur mit N = 2^n, damit die QFT genau
passt). Die Note zur Seriennummer k ist der Fourier-Zustand
    |$_k> = N^-1/2 * sum_a w^(a k) |a>,   w = exp(2 pi i / N),
und die Gruppenaktion von u ist die Verschiebung S_u|a> = |a + u>, also
S_u|$_k> = w^(-u k) |$_k> (wie in variante_b.py).

  PRAEGEN (Gen, darf jeder):  A <- H^n (Ueberlagerung ueber die Gruppe),
      B <- A (Aktion auf den Startpunkt 0), QFT auf A, A messen -> k.
      Dann ist B = |$_k>, k gleichverteilt (Quantum Lightning).
  PRUEFEN (Variante B):  je Test ein Hilfsqubit in |+>, gesteuertes S_u
      (u klassisch gleichverteilt), Phase w^(u k) auf |1>, H, messen.
      0 = bestanden. Echte Note: immer 0. Fremde Zustaende: je Test 1/2.

Das gesteuerte S_u wird als Draper-Addierer gebaut: S_u = QFT * diag(w^(-u j))
* QFT^-1, also auf B einmal QFT^-1, dann je Test n gesteuerte Phasen, am
Ende QFT. EHRLICH: im Zyklus Z_{2^n} ist das billig, weil die Gruppe
bekannt ist; in der echten Isogenie-Gruppe ist genau dieses gesteuerte S_u
die 4D-Isogenie-Kette mit ~2^63 Toffoli (pegasis_kosten.py). Der Hardware-
Lauf misst also Zustandsherstellung, Kohaerenz und Hadamard-Test --
nicht die Sicherheit (die hat ein Zyklus der Ordnung 8 nicht).

Mit der Seriennummer k: im Praeg-Schaltkreis steht k noch kohaerent im
Register A (aufgeschobene Messung); die Phase w^(u k) ist dann je Bit j
eine gesteuerte Phase zwischen Hilfsqubit und A_j. Fuer die Faelschungen
ist k klassisch (eine Phase auf dem Hilfsqubit).

Bausteine: h, x, cx, p(t), cp(t), measure -- auf jeder Hardware vorhanden.
Ausgabe: Gatterlisten (fuer hardware_ibm.py) und OpenQASM 3.

Aufruf: python3 -I hardware.py [--selbsttest] [--qasm FALL]
"""
from __future__ import annotations

import cmath
import math
import random
import sys

TAU = 2 * math.pi


class Schaltkreis:
    def __init__(self, qubits: int, name: str):
        self.q = qubits
        self.name = name
        self.gatter: list[tuple] = []  # (name, param|None, qubits...)
        self.mess: list[int] = []      # klassisches Bit i <- Qubit mess[i]
        self.bedeutung: dict = {}

    def h(self, a): self.gatter.append(("h", None, a))
    def x(self, a): self.gatter.append(("x", None, a))
    def cx(self, a, b): self.gatter.append(("cx", None, a, b))
    def p(self, t, a): self.gatter.append(("p", t % TAU, a))
    def cp(self, t, a, b): self.gatter.append(("cp", t % TAU, a, b))

    def barriere(self, qs):
        """Nichts darf ueber diese Linie hinweg zusammengefasst werden: der
        Pruefer kennt die Herstellung der Note nicht."""
        self.gatter.append(("barrier", None, *qs))

    def messen(self, qs):
        """Messen an DIESER Stelle (danach darf nur noch auf anderen Qubits
        gerechnet werden; der Simulator nutzt die aufgeschobene Messung)."""
        for a in qs:
            self.gatter.append(("measure", len(self.mess), a))
            self.mess.append(a)

    def qasm(self) -> str:
        z = ["OPENQASM 3.0;", 'include "stdgates.inc";', f"// {self.name}",
             f"qubit[{self.q}] q;", f"bit[{len(self.mess)}] c;"]
        for g in self.gatter:
            n, t, *qs = g
            if n == "measure":
                z.append(f"c[{t}] = measure q[{qs[0]}];")
                continue
            par = f"({t!r})" if t is not None else ""
            z.append(f"{n}{par} " + ", ".join(f"q[{a}]" for a in qs) + ";")
        return "\n".join(z) + "\n"


# ---- Bausteine (Qubit j eines Registers hat Gewicht 2^j) ----

def qft(s: Schaltkreis, reg: list[int], invers: bool = False) -> None:
    """|x> -> N^-1/2 sum_y w^(x y) |y> (ohne Vertauschungs-Trick: mit SWAPs
    aus cx, damit die Bitordnung stimmt)."""
    n = len(reg)
    ops = []
    for i in reversed(range(n)):
        ops.append(("h", reg[i]))
        for j in reversed(range(i)):
            ops.append(("cp", TAU / 2 ** (i - j + 1), reg[j], reg[i]))
    for i in range(n // 2):
        a, b = reg[i], reg[n - 1 - i]
        ops += [("cx", a, b), ("cx", b, a), ("cx", a, b)]
    if invers:
        ops = ops[::-1]
    for o in ops:
        if o[0] == "h":
            s.h(o[1])
        elif o[0] == "cx":
            s.cx(o[1], o[2])
        else:
            s.cp(-o[1] if invers else o[1], o[2], o[3])


def pruef_tests(s: Schaltkreis, b: list[int], anc: list[int], us: list[int],
                serie_reg: list[int] | None, serie: int | None) -> None:
    """T Tests der Variante B. Vorher B durch QFT^-1 (Draper); das QFT
    zurueck macht der Aufrufer NACH dem Messen der Hilfsqubits (die Note
    ist danach wieder heil und weitergebbar)."""
    n = len(b)
    N = 2 ** n
    qft(s, b, invers=True)
    for a, u in zip(anc, us):
        s.h(a)
        for j in range(n):  # gesteuertes S_u: Phase w^(-u 2^j) auf B_j
            s.cp(-TAU * u * 2 ** j / N, a, b[j])
        if serie_reg is not None:  # Phase w^(u k), k kohaerent in A
            for j in range(n):
                s.cp(TAU * u * 2 ** j / N, a, serie_reg[j])
        else:
            s.p(TAU * u * serie / N, a)
        s.h(a)


def praegen_und_pruefen(n: int, us: list[int]) -> Schaltkreis:
    """Gen (jeder praegt) + T Pruefungen. Qubits: A n, B n, Hilfsqubits T.
    Klassische Bits: zuerst die T Pruefbits, dann die n Bits der Seriennummer."""
    T = len(us)
    A = list(range(n))
    B = list(range(n, 2 * n))
    H = list(range(2 * n, 2 * n + T))
    s = Schaltkreis(2 * n + T, f"praegen+pruefen n={n} u={us}")
    for a in A:
        s.h(a)
    for a, b in zip(A, B):
        s.cx(a, b)
    qft(s, A)
    s.barriere(A + B + H)
    pruef_tests(s, B, H, us, A, None)
    s.messen(H + A)
    qft(s, B)  # Note zurueck in die Fourier-Basis: wieder weitergebbar
    s.bedeutung = {"fall": "echt", "pruefbits": T, "seriebits": n}
    return s


def faelschung(n: int, us: list[int], serie: int, art: str, d: int) -> Schaltkreis:
    """Pruefung eines vorgelegten Zustands gegen die Seriennummer `serie`.
    art: "echt" (|$_serie>, Referenz), "fremd" (|$_d>), "basis" (|d>,
    gemessen-und-nachgebaut). Qubits: B n, Hilfsqubits T."""
    T = len(us)
    B = list(range(n))
    H = list(range(n, n + T))
    s = Schaltkreis(n + T, f"{art} serie={serie} d={d} n={n} u={us}")
    wert = {"echt": serie, "fremd": d, "basis": d}[art]
    for j in range(n):
        if (wert >> j) & 1:
            s.x(B[j])
    if art != "basis":
        qft(s, B)
    s.barriere(B + H)
    pruef_tests(s, B, H, us, None, serie)
    s.messen(H)
    qft(s, B)
    s.bedeutung = {"fall": art, "pruefbits": T, "seriebits": 0}
    return s


# ---- Zustandsvektor-Simulator (zur Pruefung der Schaltkreise) ----

def simuliere(s: Schaltkreis) -> list[complex]:
    psi = [0j] * (1 << s.q)
    psi[0] = 1
    gemessen: set[int] = set()
    for n, t, *qs in s.gatter:
        if n == "barrier":
            continue
        if n == "measure":
            gemessen.add(qs[0])
            continue
        # aufgeschobene Messung gilt nur, wenn danach nichts mehr auf einem
        # gemessenen Qubit passiert
        assert not gemessen.intersection(qs), f"{n} nach Messung auf {qs}"
        if n == "h":
            m = 1 << qs[0]
            r = 1 / math.sqrt(2)
            for i in range(len(psi)):
                if not i & m:
                    a, b = psi[i], psi[i | m]
                    psi[i], psi[i | m] = r * (a + b), r * (a - b)
        elif n == "x":
            m = 1 << qs[0]
            for i in range(len(psi)):
                if not i & m:
                    psi[i], psi[i | m] = psi[i | m], psi[i]
        elif n == "cx":
            c, m = 1 << qs[0], 1 << qs[1]
            for i in range(len(psi)):
                if i & c and not i & m:
                    psi[i], psi[i | m] = psi[i | m], psi[i]
        elif n in ("p", "cp"):
            maske = 0
            for a in qs:
                maske |= 1 << a
            f = cmath.exp(1j * t)
            for i in range(len(psi)):
                if i & maske == maske:
                    psi[i] *= f
        else:
            raise ValueError(f"unbekanntes Gatter {n}")
    return psi


def verteilung(s: Schaltkreis) -> dict[str, float]:
    """Wahrscheinlichkeit je Messergebnis; Schluessel wie bei Qiskit:
    klassisches Bit 0 ganz rechts."""
    out: dict[str, float] = {}
    for i, a in enumerate(simuliere(s)):
        w = abs(a) ** 2
        if w < 1e-15:
            continue
        bits = "".join(str((i >> s.mess[k]) & 1) for k in reversed(range(len(s.mess))))
        out[bits] = out.get(bits, 0) + w
    return out


def auswerten(s: Schaltkreis, zaehl: dict[str, float]) -> dict:
    """Annahmequote (alle Pruefbits 0) und -- beim Praegen -- Seriennummern."""
    T = s.bedeutung["pruefbits"]
    gesamt = sum(zaehl.values())
    an = sum(v for k, v in zaehl.items() if k[-T:] == "0" * T)
    serien: dict[int, float] = {}
    if s.bedeutung["seriebits"]:
        for k, v in zaehl.items():
            serien[int(k[:-T], 2)] = serien.get(int(k[:-T], 2), 0) + v / gesamt
    return {"annahme": an / gesamt, "serien": serien}


def lesekorrektur(vert: dict[str, float], p0: list[float], p1: list[float]) -> dict[str, float]:
    """Auslesefehler herausrechnen (je Qubit unabhaengig, "tensoriert").
    vert: Verteilung ueber Bitketten der Laenge T (Bit 0 ganz rechts);
    p0[i] = P(lese 0 | ist 0), p1[i] = P(lese 1 | ist 1) fuer Bit i.
    Kehrt je Qubit die 2x2-Matrix um; negative Reste werden auf 0 gesetzt
    und neu normiert (ueblich, konservativ)."""
    T = len(p0)
    v = [0.0] * (1 << T)
    for k, w in vert.items():
        v[int(k, 2)] += w
    for i in range(T):
        a, b = p0[i], p1[i]
        det = a + b - 1
        if det <= 0.05:
            raise ValueError(f"Qubit {i}: Auslese zu schlecht fuer eine Korrektur")
        m = 1 << i
        for x in range(1 << T):
            if x & m:
                continue
            r0, r1 = v[x], v[x | m]
            # [[a, 1-b], [1-a, b]]^-1 = [[b, b-1], [a-1, a]] / det
            v[x], v[x | m] = (b * r0 + (b - 1) * r1) / det, ((a - 1) * r0 + a * r1) / det
    v = [max(0.0, w) for w in v]
    z = sum(v) or 1.0
    return {format(x, f"0{T}b"): w / z for x, w in enumerate(v) if w > 0}


def erwartet(fall: str, n: int, T: int) -> float:
    """Erwartete Annahme, gemittelt ueber u (variante_b.formel)."""
    N = 2 ** n
    voll = {"echt": 1.0, "fremd": 0.0, "basis": 1 / N}[fall]
    return voll + (1 - voll) * 2.0 ** -T


def satz(n: int, T: int, laeufe: int, rng: random.Random) -> list[Schaltkreis]:
    """Die Schaltkreise fuer einen Hardware-Lauf: je Fall `laeufe` Stueck mit
    frischen, gleichverteilten u (und Seriennummern)."""
    N = 2 ** n
    out = []
    for _ in range(laeufe):
        us = [rng.randrange(N) for _ in range(T)]
        out.append(praegen_und_pruefen(n, us))
        c = rng.randrange(N)
        d = (c + rng.randrange(1, N)) % N
        out.append(faelschung(n, us, c, "echt", c))
        # "fremd": eine echte Note, aber zu einer ANDEREN Seriennummer d != c.
        # Je Test besteht sie im Mittel ueber u mit genau 1/2.
        out.append(faelschung(n, us, c, "fremd", d))
        out.append(faelschung(n, us, c, "basis", rng.randrange(N)))
    return out


def selbsttest() -> None:
    rng = random.Random(5)
    n = 3
    N = 2 ** n
    # 1. QFT stimmt mit der Definition.
    for x in range(N):
        s = Schaltkreis(n, "qft")
        for j in range(n):
            if (x >> j) & 1:
                s.x(j)
        qft(s, list(range(n)))
        psi = simuliere(s)
        for y in range(N):
            soll = cmath.exp(1j * TAU * x * y / N) / math.sqrt(N)
            assert abs(psi[y] - soll) < 1e-9, (x, y)
        qft(s, list(range(n)), invers=True)
        psi = simuliere(s)
        assert abs(psi[x] - 1) < 1e-9
    # 2. Praegen: Seriennummer gleichverteilt, echte Note besteht IMMER.
    for _ in range(4):
        us = [rng.randrange(N) for _ in range(3)]
        s = praegen_und_pruefen(n, us)
        e = auswerten(s, verteilung(s))
        assert abs(e["annahme"] - 1) < 1e-9, e
        assert len(e["serien"]) == N and all(abs(v - 1 / N) < 1e-9 for v in e["serien"].values()), e
    # 3. Echte Note gegen klassische Seriennummer: besteht immer.
    for c in range(N):
        s = faelschung(n, [rng.randrange(N) for _ in range(3)], c, "echt", c)
        assert abs(auswerten(s, verteilung(s))["annahme"] - 1) < 1e-9
    # 4. Faelschungen: Mittel ueber u trifft die Formel.
    T = 3
    for fall in ("fremd", "basis"):
        werte = []
        for _ in range(300):
            us = [rng.randrange(N) for _ in range(T)]
            c = rng.randrange(N)
            d = (c + rng.randrange(1, N)) % N if fall == "fremd" else rng.randrange(N)
            s = faelschung(n, us, c, fall, d)
            werte.append(auswerten(s, verteilung(s))["annahme"])
        m = sum(werte) / len(werte)
        assert abs(m - erwartet(fall, n, T)) < 0.05, (fall, m, erwartet(fall, n, T))
    # 5. Nur Hardware-Bausteine, QASM ist lesbar.
    s = satz(n, T, 1, rng)[0]
    assert {g[0] for g in s.gatter} <= {"h", "x", "cx", "p", "cp", "barrier", "measure"}
    # 6. Zwischen Herstellen und Pruefen steht eine Barriere (kein
    #    Verrechnen ueber die Grenze), und gemessen wird vor dem QFT zurueck.
    for k in satz(n, T, 1, rng):
        namen = [g[0] for g in k.gatter]
        assert namen.count("barrier") == 1 and "measure" in namen
        assert namen.index("barrier") < namen.index("measure")
    # 7. Lesekorrektur kehrt bekannte Auslesefehler um.
    wahr = {"000": 0.6, "010": 0.25, "111": 0.15}
    p0, p1 = [0.97, 0.95, 0.99], [0.93, 0.9, 0.96]
    gelesen: dict[str, float] = {}
    for k, w in wahr.items():
        for x in range(8):
            pr = w
            for i in range(3):
                b, l = int(k[2 - i]), (x >> i) & 1
                pr *= (p0[i] if l == 0 else 1 - p0[i]) if b == 0 else (p1[i] if l == 1 else 1 - p1[i])
            key = format(x, "03b")
            gelesen[key] = gelesen.get(key, 0) + pr
    korr = lesekorrektur(gelesen, p0, p1)
    for k in set(wahr) | set(korr):
        assert abs(korr.get(k, 0) - wahr.get(k, 0)) < 1e-9, (k, korr, wahr)
    try:
        lesekorrektur(gelesen, [0.5, 0.95, 0.99], [0.5, 0.9, 0.96])
        raise AssertionError("unbrauchbare Auslese angenommen")
    except ValueError:
        pass
    assert s.qasm().startswith("OPENQASM 3.0;")
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    if "--qasm" in sys.argv:
        s = praegen_und_pruefen(3, [1, 3, 5])
        print(s.qasm())
        return
    rng = random.Random(1)
    n, T = 3, 3
    print(f"Spielzeug-Quantengeld fuer echte Hardware: Zyklus N = {2**n}, {T} Tests je Pruefung.")
    print("| Fall | Qubits | Gatter (davon 2-Qubit) | Annahme ideal (Simulator) | erwartet |")
    print("|---|---|---|---|---|")
    faelle: dict[str, list] = {}
    for s in satz(n, T, 40, rng):
        faelle.setdefault(s.bedeutung["fall"] if s.bedeutung["seriebits"] == 0 else "praegen+pruefen", []).append(s)
    for name, ss in faelle.items():
        a = sum(auswerten(s, verteilung(s))["annahme"] for s in ss) / len(ss)
        s0 = ss[0]
        zwei = sum(1 for g in s0.gatter if g[0] in ("cx", "cp"))
        soll = erwartet("echt" if name == "praegen+pruefen" else name, n, T)
        print(f"| {name} | {s0.q} | {len(s0.gatter)} ({zwei}) | {a:.3f} | {soll:.3f} |")


if __name__ == "__main__":
    main()
