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

TOLERANZ = 0.15        # wie im Selbsttest von wiesner_verteilt.py
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


def geheimnis_aus_lauf(s, schluessel: str) -> list[int]:
    """Bei Fern-Praeparation: die Bits, die die Validatoren gemessen haben."""
    n = s.bedeutung["n"]
    return [int(schluessel[-1 - (n + i)]) for i in range(n)]


def antworten(s, schluessel: str) -> list[int]:
    """Messwerte der Note aus einem Ergebnis-Schluessel (Bit 0 ganz rechts)."""
    n = s.bedeutung["n"]
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
         fern: bool = False) -> list:
    """Je Wartezeit `laeufe` Noten mit frischen Basen, Bits, Aufforderungen;
    mit bezahlen dieselbe Note zusaetzlich teleportiert. fern: Praegen ohne
    Vertrauen (note_fern) -- die Basen sind die der Validatoren, die Bits
    entstehen erst bei deren Messung."""
    out = []
    for w in reihe:
        for _ in range(laeufe):
            b = [rng.randrange(2) for _ in range(n)]
            x = [rng.randrange(2) for _ in range(n)]
            a = [rng.randrange(2) for _ in range(n)]
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
