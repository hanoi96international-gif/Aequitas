#!/usr/bin/env python3
"""Variante W, Praegung ohne Vertrauen (W3) und der ganze Ablauf im Simulator.

Forschung, kein Produkt. Keine Abhaengigkeiten.

DAS PROBLEM (ANALYSE.md, Abschnitt 5): wer die BB84-Zustaende herstellt,
kennt Basen und Bits -- und koennte die Note beliebig oft nachbauen.

DIE LOESUNG HIER: FERN-PRAEPARATION (wiesner_hardware.note_fern)
  Die Praegestelle erzeugt je Stelle ein verschraenktes Paar und misst
  nichts. Die Haelften gehen (ueber eine kurze Quantenleitung, wie bei QKD)
  an die Validatoren; jede Stelle an genau einen. Der Validator misst in
  einer eigenen Zufallsbasis -- erst damit steht die Stelle der Note fest,
  und nur er kennt sie. Danach teilt er (Basis, Bit, Salz) als Shamir-Anteile
  mit den anderen und meldet die Pruefsumme (wiesner_kette.Praegung).

  Folge: KEINE einzelne Stelle kennt die ganze Note -- weder die
  Praegestelle noch ein Validator. Wer faelschen will, braucht eine Absprache,
  die einen grossen Teil der Stellen kennt.

WIE SCHLIMM IST EINE ABSPRACHE? (faelschen_wahrscheinlichkeit, exakt)
  Eine Absprache kennt den Anteil f der Stellen. Fuer jede gepruefte Stelle
  passt die Basis der Aufforderung mit 1/2; ist die Stelle bekannt, ist die
  Antwort richtig, sonst rät der Faelscher (Fehler mit 1/2 je Treffer --
  das Modell des klassischen Faelschers; quantenmechanisch gilt je Qubit
  die Schranke 0,927, wiesner_verteilt.py). Gerechnet wird die Annahme einer
  Pruefung mit k Stellen, Toleranz und Mindesttreffern wie auf der Kette.

DER GANZE ABLAUF IM SIMULATOR (ende_zu_ende)
  Validatoren messen (Fern-Praeparation) -> Kette praegt (Pruefsummen,
  Anteile, Bestaetigungen) -> die Note wird per Teleportation bezahlt -> der
  Empfaenger loest ein: Anfrage, Aufforderung aus dem Block, Messung, Antwort,
  Offenlegen, Urteil, AEQ auf seinem Konto. Die Quantenseite ist exakt
  simuliert (hardware.verteilung), die Kettenseite ist wiesner_kette.KetteW.

Aufruf:  python3 -I wiesner_praegen.py [--selbsttest]
"""
from __future__ import annotations

import hashlib
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
wh = _laden("wiesner_hardware")
wk = _laden("wiesner_kette")


def zuordnung(n: int, validatoren: list[str], rng: random.Random) -> list[str]:
    """Jede Stelle an genau einen Validator, gleichmaessig, zufaellig verteilt."""
    liste = [validatoren[i % len(validatoren)] for i in range(n)]
    rng.shuffle(liste)
    return liste


def faelschen_wahrscheinlichkeit(f: float, k: int, toleranz: float = wk.TOLERANZ,
                                 min_treffer: int = wk.MIN_TREFFER) -> float:
    """Annahme einer Pruefung mit k Stellen fuer eine Absprache, die den
    Anteil f der Stellen kennt (exakt, dynamische Programmierung)."""
    p_un = 0.5                         # Basis passt nicht: zaehlt nicht
    p_ok = 0.5 * (f + (1 - f) * 0.5)   # Treffer, richtig
    p_f = 0.5 * (1 - f) * 0.5          # Treffer, falsch
    # dp[(t, e)] = Wahrscheinlichkeit fuer t Treffer, e Fehler
    dp = {(0, 0): 1.0}
    for _ in range(k):
        neu: dict[tuple[int, int], float] = {}
        for (t, e), w in dp.items():
            for (dt, de), p in (((0, 0), p_un), ((1, 0), p_ok), ((1, 1), p_f)):
                key = (t + dt, e + de)
                neu[key] = neu.get(key, 0.0) + w * p
        dp = neu
    return sum(w for (t, e), w in dp.items() if t >= min_treffer and e <= int(toleranz * t))


def ende_zu_ende(rng: random.Random, n: int = 32, k: int = 24, bezahlen: bool = True,
                 faelscher: bool = False) -> dict:
    """Der ganze Ablauf, Quantenseite exakt simuliert. faelscher: der
    Einloesende hat keine Note, er raet."""
    VAL = {f"v{j}": bytes([j]) * 16 for j in range(1, 6)}
    vliste, t = list(VAL), 3
    kette = wk.KetteW(VAL)
    kette.konto("praeger", 50_000_000, b"p" * 16)
    kette.konto("empfaenger", wk.PFAND, b"e" * 16)  # genau das Pfand der Anfrage
    summe0 = kette.summe()

    # 1. Fern-Praeparation: jeder Validator misst seine Stellen in eigenen
    # Zufallsbasen. Die Messwerte sind gleichverteilt (note_fern-Selbsttest);
    # danach ist jede Stelle der Note genau der BB84-Zustand (Basis, Bit).
    wem = zuordnung(n, vliste, rng)
    basen = [rng.randrange(2) for _ in range(n)]
    bits = [rng.randrange(2) for _ in range(n)]
    K = hashlib.sha256(rng.randbytes(16)).hexdigest()
    pr = wk.Praegung(K, basen, bits, t, vliste, rng)
    text = f"{wk.DOM_BEST}{K}|{pr.wurzel.hex()}|{n}|{t}|{','.join(vliste)}"
    best = {v: wk.KetteW.sig(VAL[v], text) for v in vliste}
    kette.praegen("praeger", K, n, pr.wurzel, 10_000_000, vliste, t, best)
    kette.block()

    # 2. Bezahlen: die Note geht (als Quantenzustand) an den Empfaenger --
    # hier Teil des Schaltkreises unten. 3. Einloesen beim Empfaenger.
    zaehl = kette.noten[K]["zaehl"]
    kette.anfrage("empfaenger", K, k, wk.KetteW.sig(b"e" * 16, f"anfrage|{K}|{k}|empfaenger|{zaehl}"), "empfaenger")
    kette.block()  # die Aufforderung zieht der naechste Block
    st, ch = kette.aufforderung_von(K)
    if faelscher:
        antwort = [rng.randrange(2) for _ in st]
    else:
        # Die Stellen einer Note sind unverschraenkt (Produktzustand): jede
        # Stelle exakt fuer sich simulieren -- dieselbe Physik wie die ganze
        # Note, aber 3 statt 3n Qubits. EIN Schuss je Stelle: eine Note wird
        # einmal gemessen.
        antwort = []
        for i, beta in zip(st, ch):
            s = wh.note([basen[i]], [bits[i]], [beta], warten_us=20, bezahlen=bezahlen)
            r, acc = rng.random(), 0.0
            for schluessel, w in sorted(hw.verteilung(s).items()):
                acc += w
                if r <= acc:
                    break
            antwort.append(wh.antworten(s, schluessel)[0])
    kette.antwort("empfaenger", K, antwort, wk.KetteW.sig(b"e" * 16, f"antwort|{K}|{zaehl}|{antwort}"))
    kette.block()
    ok = kette.offenlegen(K, pr.offenlegung(st, [1, 3, 5]))  # None = unentschieden
    return {"urteil": ok, "empfaenger": kette.konten["empfaenger"], "topf": kette.topf,
            "erhaltung": kette.summe() == summe0, "stellen_je_validator": {v: wem.count(v) for v in vliste}}


def selbsttest() -> bool:
    fehler: list[str] = []

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    try:
        rng = random.Random(2027)
        # Zuordnung: jede Stelle genau einmal, gleichmaessig.
        z = zuordnung(50, ["a", "b", "c", "d", "e"], rng)
        soll(len(z) == 50 and all(z.count(v) == 10 for v in "abcde"), "Zuordnung")
        # Absprache: f = 1 besteht sicher (wenn genug Treffer), f = 0 kaum,
        # monoton in f; ein Validator von fuenf (f = 0,2) bei k = 64: winzig.
        werte = [faelschen_wahrscheinlichkeit(f, 64) for f in (0.0, 0.2, 0.5, 0.8, 1.0)]
        soll(all(a <= b + 1e-15 for a, b in zip(werte, werte[1:])), f"nicht monoton: {werte}")
        # Ehrlich: mit 15 % Toleranz ist EINE Pruefung schwach (k = 64:
        # ohne Wissen 3,8e-5, mit einem Fuenftel der Stellen 1,5e-3). Fuer
        # echtes Geld: viel groessere k oder weniger Toleranz (ANALYSE.md).
        soll(werte[-1] > 0.99 and werte[0] < 1e-4 and werte[1] < 2e-3, f"Absprache: {werte}")
        # Gegenprobe gegen die Monte-Carlo-Simulation desselben Modells.
        f, k, laeufe, durch = 0.5, 16, 20000, 0
        for _ in range(laeufe):
            t = e = 0
            for _ in range(k):
                if rng.random() < 0.5:
                    t += 1
                    if rng.random() >= f and rng.random() < 0.5:
                        e += 1
            durch += t >= wk.MIN_TREFFER and e <= int(wk.TOLERANZ * t)
        p = faelschen_wahrscheinlichkeit(f, k)
        soll(abs(durch / laeufe - p) < 0.01, f"DP {p:.4f} gegen Monte-Carlo {durch / laeufe:.4f}")
        # Der ganze Ablauf: mit Note eingeloest, AEQ beim Empfaenger, Erhaltung.
        for bez in (False, True):
            r = ende_zu_ende(rng, bezahlen=bez)
            soll(r["urteil"] is True and r["empfaenger"] == 10_000_000 + wk.PFAND and r["topf"] == 0 and r["erhaltung"], f"Ende zu Ende bezahlt={bez}: {r}")
        # Ohne Note: abgewiesen, nichts beim Empfaenger, Wert ans Grundeinkommen.
        durch = sum(ende_zu_ende(rng, faelscher=True)["urteil"] is True for _ in range(40))
        soll(durch <= 2, f"Faelscher {durch}/40 eingeloest")
    except Exception as e:
        fehler.append(f"Ausnahme: {type(e).__name__}: {e}")
    for f in fehler:
        print("FEHLER:", f)
    print("wiesner_praegen: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def main():
    print("Absprache kennt den Anteil f der Stellen: Annahme einer Pruefung (k Stellen, Toleranz 15 %)")
    print(f"{'f':>6} | " + " | ".join(f"k={k:>3}" for k in (16, 32, 64)))
    for f in (0.0, 0.1, 0.2, 0.34, 0.5, 0.67, 0.9):
        print(f"{f:>6.2f} | " + " | ".join(f"{faelschen_wahrscheinlichkeit(f, k):>5.1e}" for k in (16, 32, 64)))
    print()
    print("Ein Validator von v kennt f = 1/v. EHRLICH: mit 15 % Toleranz ist eine einzelne Pruefung schwach;")
    print("fuer echtes Geld braucht es grosse k (Hunderte Stellen) oder weniger Toleranz -- also gute Speicher.")


if __name__ == "__main__":
    sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else main()
