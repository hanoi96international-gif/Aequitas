#!/usr/bin/env python3
"""Variante W auf echter Hardware: eine Note von Anfang bis Ende (HEUTE.md 3.3).

Forschung, kein Produkt. Keine Abhaengigkeiten (die Schaltkreise sind die von
hardware.py; auf IBM bringt sie hardware_ibm.py --wiesner).

ABLAUF EINES LAUFS
  1. Die Kette legt Basen und Bits der Note fest und verteilt sie als
     Shamir-Anteile an die Validatoren (wiesner_verteilt.py). Hier: Zufall
     des Laeufers, sofort geteilt.
  2. PRAEGEN: n Qubits in BB84-Zustaenden (Basis Z oder X, Bit 0 oder 1).
  3. HALTEN: die Note liegt W Mikrosekunden im Speicher.
  4. BEZAHLEN (wahlweise): Teleportation an ein zweites Register -- der
     Empfaenger hat die Note danach, beim Zahler bleibt nichts.
  5. PRUEFEN, klassisch: gemessen wird in den Basen der Aufforderung; die
     Validatoren zaehlen auf ihren Anteilen Treffer (passende Basis) und
     Fehler (falsches Bit) und urteilen mit Toleranz.

WAS DAS ZEIGT -- UND WAS NICHT
  Es zeigt, dass der ganze Ablauf auf echter Hardware laeuft, und es misst
  die ehrliche Fehlerrate je Qubit nach W Mikrosekunden (mit und ohne
  Teleportation). Daraus folgen Toleranz und Notengroesse der Variante W.
  Es bringt KEINE Sicherheit: die Note ist klein, lebt Mikrosekunden,
  Praegestelle und Halter sind derselbe Schaltkreis, und den Lauf bezeugt nur
  der Betreiber der Maschine. Nie mit echten AEQ.

Aufruf: python3 -I wiesner_hardware.py --selbsttest
"""
from __future__ import annotations

import importlib.util
import math
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))


def _laden(name: str):
    spec = importlib.util.spec_from_file_location(name, os.path.join(_hier, name + ".py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


hw = _laden("hardware")
wv = _laden("wiesner_verteilt")

# Standard-Toleranz der Kette (wiesner_kette.TOL_PROZENT, W2c). Die fruehere
# 15 % war gegen einen optimalen Kloner gebrochen (14,6 % je Kopie, ANALYSE.md
# "Befund"). Bei Noten bis 6 Stellen aendert das die Annahme nicht (dort ist
# so oder so kein Fehler erlaubt), bei der 16er-Note der Kette schon.
TOLERANZ = 0.04
MAX_QUBITS_NOTE = 16


def note(basen: list[int], bits: list[int], aufforderung: list[int],
         warten_us: float = 0.0, bezahlen: bool = False):
    """Schaltkreis: Praegen -> Halten -> (Bezahlen) -> Messen in den Basen
    der Aufforderung. Basis 0 = Z, 1 = X. Messbit i = Qubit i der Note."""
    n = len(basen)
    if not (1 <= n <= MAX_QUBITS_NOTE and len(bits) == n and len(aufforderung) == n):
        raise ValueError("Note: Laengen")
    if any(v not in (0, 1) for v in basen + bits + aufforderung):
        raise ValueError("Note: nur 0/1")
    # Qubits je Stelle nebeneinander (B_j, E_j, C_j): dieselbe Physik, aber
    # der Matrix-Produkt-Simulator bleibt klein (die Paare kreuzen sich nicht).
    w = 3 if bezahlen else 1
    B = [w * j for j in range(n)]
    q = w * n
    s = hw.Schaltkreis(q, f"wiesner n={n} warten={warten_us:g}us" + (" bezahlt" if bezahlen else ""))
    for a, b, x in zip(B, basen, bits):
        if x:
            s.x(a)
        if b:
            s.h(a)
    s.warten(warten_us, B)
    ziel = B
    if bezahlen:
        E = [3 * j + 1 for j in range(n)]
        C = [3 * j + 2 for j in range(n)]
        for e, c in zip(E, C):
            s.h(e)
            s.cx(e, c)
        s.barriere(list(range(q)))
        # Zahler: Bell-Messung (B_j, E_j); Bits n+2j, n+2j+1
        for j, (b, e) in enumerate(zip(B, E)):
            s.cx(b, e)
            s.h(b)
            s.messen([b, e], [n + 2 * j, n + 2 * j + 1])
        # Empfaenger: X je nach E-Bit, Z je nach B-Bit
        for j, c in enumerate(C):
            s.wenn_x(n + 2 * j + 1, c)
            s.wenn(n + 2 * j, math.pi, c)
        ziel = C
    s.barriere(ziel)
    for a, beta in zip(ziel, aufforderung):
        if beta:
            s.h(a)
    s.messen(ziel, list(range(n)))
    s.bedeutung = {"fall": "wiesner", "n": n, "basen": list(basen), "bits": list(bits),
                   "aufforderung": list(aufforderung), "warten_us": float(warten_us), "bezahlt": bezahlen}
    return s


def note_fern(val_basen: list[int], aufforderung: list[int], warten_us: float = 0.0, bezahlen: bool = False):
    """W3, Praegen ohne Vertrauen (Fern-Praeparation): die Praegestelle
    erzeugt je Stelle ein verschraenktes Paar (Note N_i, Haelfte V_i) und
    misst NICHTS. Der Validator misst V_i in seiner Basis und bekommt r_i --
    damit steht N_i fest: Basis Z -> |r_i>, Basis X -> |+> (r=0) / |-> (r=1).
    Das Geheimnis (Basis, r_i) kennt nur der Validator; die Praegestelle
    nicht. Messbits: 0..n-1 Note (Aufforderung), n..2n-1 Validatoren,
    danach die Bits der Teleportation."""
    n = len(val_basen)
    if not (1 <= n <= MAX_QUBITS_NOTE and len(aufforderung) == n):
        raise ValueError("Note: Laengen")
    if any(v not in (0, 1) for v in val_basen + aufforderung):
        raise ValueError("Note: nur 0/1")
    # Qubits je Stelle nebeneinander (N_j, V_j[, E_j, C_j]), siehe note().
    w = 4 if bezahlen else 2
    N = [w * j for j in range(n)]
    V = [w * j + 1 for j in range(n)]
    q = w * n
    s = hw.Schaltkreis(q, f"wiesner-fern n={n} warten={warten_us:g}us" + (" bezahlt" if bezahlen else ""))
    for a, v in zip(N, V):
        s.h(v)
        s.cx(v, a)
    s.barriere(N + V)
    for j, (v, b) in enumerate(zip(V, val_basen)):
        if b:
            s.h(v)
        s.messen([v], [n + j])
    s.warten(warten_us, N)
    ziel = N
    if bezahlen:
        E = [4 * j + 2 for j in range(n)]
        C = [4 * j + 3 for j in range(n)]
        for e, c in zip(E, C):
            s.h(e)
            s.cx(e, c)
        s.barriere(N + E + C)
        for j, (b, e) in enumerate(zip(N, E)):
            s.cx(b, e)
            s.h(b)
            s.messen([b, e], [2 * n + 2 * j, 2 * n + 2 * j + 1])
        for j, c in enumerate(C):
            s.wenn_x(2 * n + 2 * j + 1, c)
            s.wenn(2 * n + 2 * j, math.pi, c)
        ziel = C
    s.barriere(ziel)
    for a, beta in zip(ziel, aufforderung):
        if beta:
            s.h(a)
    s.messen(ziel, list(range(n)))
    s.bedeutung = {"fall": "wiesner-fern", "n": n, "basen": list(val_basen), "bits": None,
                   "aufforderung": list(aufforderung), "warten_us": float(warten_us), "bezahlt": bezahlen}
    return s


# ------------------------------------------------ B6: Schutz je Stelle (Steane)
#
# Jede Stelle der Note liegt im Steane-Code [[7,1,3]]. Der Code ist CSS und
# selbstdual: |0_L> ist die gleichmaessige Ueberlagerung der 8 Codewoerter des
# Zeilenraums von H (gerades Gewicht), X_L = X auf {0,1,2}, und die
# transversale Hadamard macht aus |0_L>/|1_L> genau |+_L>/|-_L>. Gemessen
# wird transversal (alle 7 Qubits in der Basis der Aufforderung) und klassisch
# dekodiert: Syndrom H*b zeigt auf das falsche Bit, der Wert ist die Paritaet.
# Ein Fehler je Stelle und Basis wird so korrigiert -- ohne Syndrom-Runden auf
# dem Chip. Der Schutz aendert die Sicherheit nicht: Der Code ist oeffentlich,
# ein Faelscher kann jede Stelle dekodieren und hat dann ein BB84-Qubit.

STEANE_H = ((0, 0, 0, 1, 1, 1, 1), (0, 1, 1, 0, 0, 1, 1), (1, 0, 1, 0, 1, 0, 1))
STEANE_LOGISCH = (0, 1, 2)        # Traeger von X_L (und, nach Hadamard, Z_L)
MAX_STELLEN_STEANE = 2            # 14 Qubits


def steane_null(s, q: list[int]) -> None:
    """|0_L> auf den 7 Qubits q: Hadamard auf die Pivot-Qubits 3, 1, 0 der drei
    Zeilen von H, dann je 3 CNOT auf den Rest der Zeile (9 CNOT)."""
    for piv in (3, 1, 0):
        s.h(q[piv])
    for piv, rest in ((3, (4, 5, 6)), (1, (2, 5, 6)), (0, (2, 4, 6))):
        for z in rest:
            s.cx(q[piv], q[z])


def steane_syndrom(b7: list[int]) -> int:
    """1..7 = Position+1 des einen falschen Bits, 0 = kein Fehler erkannt."""
    return sum((sum(h * x for h, x in zip(zeile, b7)) & 1) << (2 - r) for r, zeile in enumerate(STEANE_H))


def steane_dekodieren(b7: list[int]) -> int:
    """Logischer Wert aus 7 transversal gemessenen Bits (ein Fehler korrigiert)."""
    b = list(b7)
    syn = steane_syndrom(b)
    if syn:
        b[syn - 1] ^= 1
    return sum(b) & 1


def note_steane(basen: list[int], bits: list[int], aufforderung: list[int], warten_us: float = 0.0,
                fehler: list[tuple[str, int]] | None = None):
    """Wie note(), aber jede Stelle j auf den Qubits 7j..7j+6 im Steane-Code.
    Messbits 7j+k = Qubit k der Stelle j. fehler: nur fuer den Selbsttest
    (("x"|"z", qubit) mitten im Speicher)."""
    n = len(basen)
    if not (1 <= n <= MAX_STELLEN_STEANE and len(bits) == n and len(aufforderung) == n):
        raise ValueError("Steane-Note: Laengen")
    if any(v not in (0, 1) for v in basen + bits + aufforderung):
        raise ValueError("Steane-Note: nur 0/1")
    Q = [list(range(7 * j, 7 * j + 7)) for j in range(n)]
    alle = list(range(7 * n))
    s = hw.Schaltkreis(7 * n, f"wiesner-steane n={n} warten={warten_us:g}us")
    for q, b, x in zip(Q, basen, bits):
        steane_null(s, q)
        if x:
            for k in STEANE_LOGISCH:
                s.x(q[k])
        if b:
            for a in q:
                s.h(a)
    s.barriere(alle)
    s.warten(warten_us, alle)
    for art, a in (fehler or []):
        if art == "z":
            s.p(math.pi, a)
        else:
            s.x(a)
    s.barriere(alle)
    for q, beta in zip(Q, aufforderung):
        if beta:
            for a in q:
                s.h(a)
    s.messen(alle, alle)
    s.bedeutung = {"fall": "wiesner-steane", "n": n, "basen": list(basen), "bits": list(bits),
                   "aufforderung": list(aufforderung), "warten_us": float(warten_us), "bezahlt": False}
    return s


def geheimnis_aus_lauf(s, schluessel: str) -> list[int]:
    """Bei Fern-Praeparation: die Bits, die die Validatoren gemessen haben."""
    n = s.bedeutung["n"]
    return [int(schluessel[-1 - (n + i)]) for i in range(n)]


def antworten(s, schluessel: str) -> list[int]:
    """Messwerte der Note aus einem Ergebnis-Schluessel (Bit 0 ganz rechts).
    Steane-Note: je Stelle die 7 Bits dekodiert (steane_dekodieren)."""
    n = s.bedeutung["n"]
    if s.bedeutung.get("fall") == "wiesner-steane":
        return [steane_dekodieren([int(schluessel[-1 - (7 * j + k)]) for k in range(7)]) for j in range(n)]
    return [int(schluessel[-1 - i]) for i in range(n)]


def auswerten(s, zaehl: dict[str, float], toleranz: float = TOLERANZ) -> dict:
    """Fehlerrate je Treffer (passende Basis) und Annahme mit Toleranz."""
    b, a = s.bedeutung["basen"], s.bedeutung["aufforderung"]
    treffer = [i for i in range(len(b)) if a[i] == b[i]]
    gesamt = sum(zaehl.values())
    fehler_summe = an = 0.0
    for k, v in zaehl.items():
        o = antworten(s, k)
        # Fern-Praeparation: das Geheimnis ist je Schuss, was die Validatoren gemessen haben.
        x = s.bedeutung["bits"] if s.bedeutung["bits"] is not None else geheimnis_aus_lauf(s, k)
        f = sum(o[i] != x[i] for i in treffer)
        fehler_summe += f * v
        if f <= math.floor(toleranz * len(treffer)):
            an += v
    return {
        "treffer": len(treffer),
        "fehlerrate": (fehler_summe / (gesamt * len(treffer))) if treffer else 0.0,
        "annahme": an / gesamt,
    }


def kette_pruefen(s, ergebnis: list[int], rng: random.Random, t: int = 3, v: int = 5,
                  toleranz: float = TOLERANZ, bits: list[int] | None = None) -> tuple[int, int, bool]:
    """Pruefung wie auf der Kette: Geheimnis als Anteile (t von v), gezaehlt
    auf den Anteilen (wiesner_verteilt.Kette), nur Treffer/Fehler geoeffnet.
    bits: bei Fern-Praeparation die Messwerte der Validatoren (geheimnis_aus_lauf)."""
    if bits is None:
        bits = s.bedeutung["bits"]
    if bits is None:
        raise ValueError("Fern-Praeparation: Bits der Validatoren fehlen")
    V = wv.Validatoren(t, v, rng)
    K = wv.Kette(V, toleranz=toleranz, mindest_treffer=1)
    n = s.bedeutung["n"]
    K.noten[0] = {"n": n, "basis": [V.teile(b) for b in s.bedeutung["basen"]],
                  "bit": [V.teile(x) for x in bits],
                  "benutzt": set(), "gesperrt": False, "eingeloest": False}
    stellen = list(range(n))
    tr, fe = K.zaehlen(0, stellen, s.bedeutung["aufforderung"], ergebnis)
    return tr, fe, K.pruefen(0, stellen, s.bedeutung["aufforderung"], ergebnis)


def satz(n: int, reihe: list[float], laeufe: int, bezahlen: bool, rng: random.Random,
         fern: bool = False, steane: bool = False) -> list:
    """Je Wartezeit `laeufe` Noten mit frischen Basen, Bits, Aufforderungen;
    mit bezahlen dieselbe Note zusaetzlich teleportiert. fern: Praegen ohne
    Vertrauen (note_fern) -- die Basen sind die der Validatoren, die Bits
    entstehen erst bei deren Messung. steane: dieselbe Note ungeschuetzt und
    im Steane-Code (B6), Seite an Seite."""
    if steane and (fern or bezahlen or n > MAX_STELLEN_STEANE):
        raise ValueError("Steane: ohne fern/bezahlen, hoechstens 2 Stellen")
    out = []
    for w in reihe:
        for _ in range(laeufe):
            b = [rng.randrange(2) for _ in range(n)]
            x = [rng.randrange(2) for _ in range(n)]
            a = [rng.randrange(2) for _ in range(n)]
            if steane:
                out.append(note(b, x, a, warten_us=w))
                out.append(note_steane(b, x, a, warten_us=w))
                continue
            if fern:
                out.append(note_fern(b, a, warten_us=w))
                if bezahlen:
                    out.append(note_fern(b, a, warten_us=w, bezahlen=True))
                continue
            out.append(note(b, x, a, warten_us=w))
            if bezahlen:
                out.append(note(b, x, a, warten_us=w, bezahlen=True))
    return out


def selbsttest() -> bool:
    rng = random.Random(1983)
    fehler: list[str] = []

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    # Bitreihenfolge: alles Z, Bits 1,0,1,1 -> genau "1101" (Bit 0 rechts).
    s = note([0, 0, 0, 0], [1, 0, 1, 1], [0, 0, 0, 0])
    soll(hw.verteilung(s) == {"1101": 1.0} or set(hw.verteilung(s)) == {"1101"}, f"Bitreihenfolge: {hw.verteilung(s)}")
    soll(antworten(s, "1101") == [1, 0, 1, 1], "antworten()")

    # Ideal: echte Note, mit und ohne Bezahlen -- keine Fehler, immer angenommen.
    for bez in (False, True):
        for _ in range(6):
            n = 4
            b, x, a = ([rng.randrange(2) for _ in range(n)] for _ in range(3))
            s = note(b, x, a, warten_us=20, bezahlen=bez)
            e = auswerten(s, hw.verteilung(s))
            soll(abs(e["fehlerrate"]) < 1e-9 and abs(e["annahme"] - 1) < 1e-9, f"ideal bezahlt={bez}: {e}")
            # Ergebnis eines Schusses durch die Kette: angenommen.
            k = max(hw.verteilung(s).items(), key=lambda kv: kv[1])[0]
            tr, fe, ok = kette_pruefen(s, antworten(s, k), rng)
            soll(ok and fe == 0 and tr == sum(a[i] == b[i] for i in range(n)), f"Kette ideal: {tr} {fe} {ok}")

    # Faelschung: richtige Bits, aber alle Basen vertauscht -> jede passende
    # Stelle ist mit 1/2 falsch (exakt aus der Verteilung).
    n = 4
    b = [0, 1, 0, 1]
    x = [1, 1, 0, 0]
    a = list(b)  # Aufforderung trifft jede Stelle
    falsch = note([1 - y for y in b], x, a)
    falsch.bedeutung["basen"] = b  # die Kette kennt die echten Basen
    e = auswerten(falsch, hw.verteilung(falsch))
    soll(abs(e["fehlerrate"] - 0.5) < 1e-9, f"Faelschung Fehlerrate {e['fehlerrate']}")
    soll(e["annahme"] < 0.35, f"Faelschung Annahme {e['annahme']}")
    # Die Kette weist eine Antwort mit vielen Fehlern ab.
    tr, fe, ok = kette_pruefen(falsch, [1 - y for y in x], rng)
    soll(not ok and fe == n, f"Kette weist Faelschung nicht ab: {tr} {fe} {ok}")

    # W3 Fern-Praeparation, ideal: die Note folgt genau dem, was die
    # Validatoren messen -- mit und ohne Bezahlen keine Fehler; ihre Bits sind
    # gleichverteilt (die Praegestelle kann sie nicht festlegen).
    for bez in (False, True):
        n = 3
        vb, a = [rng.randrange(2) for _ in range(n)], [rng.randrange(2) for _ in range(n)]
        s = note_fern(vb, a, warten_us=10, bezahlen=bez)
        vert = hw.verteilung(s)
        e = auswerten(s, vert)
        soll(abs(e["fehlerrate"]) < 1e-9 and abs(e["annahme"] - 1) < 1e-9, f"fern ideal bezahlt={bez}: {e}")
        p_r = {}
        for k, w in vert.items():
            r = tuple(geheimnis_aus_lauf(s, k))
            p_r[r] = p_r.get(r, 0) + w
        soll(len(p_r) == 2 ** n and all(abs(w - 2 ** -n) < 1e-9 for w in p_r.values()), f"Validator-Bits nicht gleichverteilt: {p_r}")
        # Die Kette prueft gegen die Bits der Validatoren: jedes Ergebnis echt,
        # mit fremden (geratenen) Bits nicht jedes.
        for k in vert:
            tr, fe, ok = kette_pruefen(s, antworten(s, k), rng, bits=geheimnis_aus_lauf(s, k))
            soll(ok and fe == 0, f"fern Kette: {k} {tr} {fe}")
    try:
        kette_pruefen(note_fern([0], [0]), [0], rng)
        soll(False, "fern ohne Bits geprueft")
    except ValueError:
        pass
    k = satz(3, [0], 2, True, rng, fern=True)
    soll(len(k) == 4 and all(x.bedeutung["fall"] == "wiesner-fern" for x in k) and k[1].bedeutung["bezahlt"], "satz(fern)")

    # B6 Steane: ideal immer richtig; JEDER einzelne X- oder Z-Fehler auf
    # jedem der 7 Qubits wird korrigiert; zwei X-Fehler kippen den Wert
    # (Abstand 3) -- dann muss die Pruefung den Fehler sehen.
    soll(all(steane_syndrom([0] * 7) == 0 for _ in [0]) and
         all(steane_syndrom([int(j == k) for j in range(7)]) == k + 1 for k in range(7)), "Steane-Syndrom")
    for b0 in (0, 1):
        for x0 in (0, 1):
            for a0 in (0, 1):
                s = note_steane([b0], [x0], [a0])
                vert = hw.verteilung(s)
                ok = all(antworten(s, k)[0] == x0 for k in vert) if a0 == b0 else \
                    abs(sum(w for k, w in vert.items() if antworten(s, k)[0] == 1) - 0.5) < 1e-9
                soll(ok, f"Steane ideal b={b0} x={x0} a={a0}")
                if a0 != b0:
                    continue
                # Ein Fehler, der in der Messbasis zaehlt: X bei Z-Messung, Z bei X-Messung.
                art = "z" if b0 else "x"
                for q in range(7):
                    s = note_steane([b0], [x0], [a0], fehler=[(art, q)])
                    soll(all(antworten(s, k)[0] == x0 for k in hw.verteilung(s)), f"Steane korrigiert {art}{q} nicht")
                s = note_steane([b0], [x0], [a0], fehler=[(art, 0), (art, 1)])
                soll(all(antworten(s, k)[0] != x0 for k in hw.verteilung(s)), "zwei Fehler muessen den Wert kippen")
    k = satz(2, [0], 1, False, rng, steane=True)
    soll(len(k) == 2 and k[0].bedeutung["fall"] == "wiesner" and k[1].bedeutung["fall"] == "wiesner-steane"
         and k[0].bedeutung["basen"] == k[1].bedeutung["basen"] and k[1].q == 14, "satz(steane)")
    for kaputt in (dict(fern=True), dict(n=3)):
        try:
            satz(kaputt.get("n", 2), [0], 1, False, rng, steane=True, fern=kaputt.get("fern", False))
            soll(False, f"satz(steane) angenommen: {kaputt}")
        except ValueError:
            pass

    # Grenzen.
    for kaputt in (([], [], []), ([2], [0], [0]), ([0] * 17, [0] * 17, [0] * 17)):
        try:
            note(*kaputt)
            soll(False, f"Note angenommen: {kaputt}")
        except ValueError:
            pass

    # satz(): Zahl und Paare.
    k = satz(3, [0, 50], 2, True, rng)
    soll(len(k) == 8 and k[0].bedeutung["basen"] == k[1].bedeutung["basen"] and k[1].bedeutung["bezahlt"], "satz()")

    for f in fehler:
        print("FEHLER:", f)
    print("wiesner_hardware: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


if __name__ == "__main__":
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    print(hw.Schaltkreis.qasm(note([0, 1, 0], [1, 0, 1], [0, 1, 1], warten_us=20, bezahlen=True)))
