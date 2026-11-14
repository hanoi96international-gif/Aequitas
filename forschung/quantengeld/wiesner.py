#!/usr/bin/env python3
"""Wiesner-Quantengeld (1983), simuliert -- Forschung, kein Produktionscode.

Eine Banknote besteht aus n Qubits. Jedes Qubit ist zufaellig einer von vier
Zustaenden: |0>, |1> (Basis Z) oder |+>, |-> (Basis X). Nur die Bank kennt
Basis und Bit jedes Qubits (geheimer Schluessel) und kann deshalb pruefen.

Was diese Simulation zeigt:
  1. Echte Noten bestehen die Pruefung der Bank immer.
  2. Ein Faelscher, der die Note nicht kennt, kann sie nicht kopieren
     (Quanten-Kopierverbot). Die beste einfache Strategie -- jedes Qubit in
     einer geratenen Basis messen und zwei Kopien herstellen -- besteht die
     Pruefung je Qubit nur mit Wahrscheinlichkeit 3/4, also insgesamt mit
     (3/4)^n. Bei n = 64 ist das kleiner als 1e-8.
  3. GRENZE: Pruefen kann NUR die Bank. Das ist genau das, was Aequitas nicht
     will (keine zentrale Stelle). Oeffentlich pruefbares Quantengeld ist das
     offene Problem -- siehe zhandry_spielzeug.py und README.md.

Jedes Qubit wird exakt als 2er-Vektor reeller Amplituden gerechnet (die vier
Zustaende brauchen keine komplexen Zahlen). Keine Abhaengigkeiten.

Aufruf:  python3 -I wiesner.py [--selbsttest]
"""
import math
import random
import sys

S = 1 / math.sqrt(2)
# Zustaende als (Amplitude |0>, Amplitude |1>)
ZUSTAND = {
    ("Z", 0): (1.0, 0.0),
    ("Z", 1): (0.0, 1.0),
    ("X", 0): (S, S),     # |+>
    ("X", 1): (S, -S),    # |->
}


def messen(qubit, basis, rng):
    """Projektive Messung in Basis Z oder X; gibt (Ergebnis, neuer Zustand)."""
    a, b = qubit
    p0 = (a * ZUSTAND[(basis, 0)][0] + b * ZUSTAND[(basis, 0)][1]) ** 2
    ergebnis = 0 if rng.random() < p0 else 1
    return ergebnis, ZUSTAND[(basis, ergebnis)]


def note_drucken(n, rng):
    """Bank: zufaellige Basen und Bits; die Note sind die Qubits."""
    geheim = [(rng.choice("ZX"), rng.randint(0, 1)) for _ in range(n)]
    note = [ZUSTAND[g] for g in geheim]
    return geheim, note


def bank_prueft(geheim, note, rng):
    """Die Bank misst jedes Qubit in seiner richtigen Basis."""
    for (basis, bit), qubit in zip(geheim, note):
        ergebnis, _ = messen(qubit, basis, rng)
        if ergebnis != bit:
            return False
    return True


def faelschen_messen_und_nachbauen(note, rng):
    """Faelscher ohne Schluessel: Basis raten, messen, zwei Kopien bauen."""
    kopie1, kopie2 = [], []
    for qubit in note:
        basis = rng.choice("ZX")
        ergebnis, zustand = messen(qubit, basis, rng)
        kopie1.append(zustand)
        kopie2.append(ZUSTAND[(basis, ergebnis)])
    return kopie1, kopie2


def versuch(n, laeufe, rng):
    echt = gefaelscht = 0
    for _ in range(laeufe):
        geheim, note = note_drucken(n, rng)
        if bank_prueft(geheim, note, rng):
            echt += 1
        k1, _ = faelschen_messen_und_nachbauen(note, rng)
        if bank_prueft(geheim, k1, rng):
            gefaelscht += 1
    return echt / laeufe, gefaelscht / laeufe


def selbsttest():
    rng = random.Random(1983)
    fehler = []
    # 1. Echte Noten bestehen immer.
    e, _ = versuch(16, 2000, rng)
    if e != 1.0:
        fehler.append(f"echte Noten bestehen nur zu {e}")
    # 2. Einzelnes Qubit: Faelschung besteht zu ~3/4 (Toleranz fuer Zufall).
    _, f1 = versuch(1, 40000, rng)
    if abs(f1 - 0.75) > 0.01:
        fehler.append(f"1 Qubit: Faelschung {f1:.4f}, erwartet 0.75")
    # 3. Acht Qubits: ~ (3/4)^8 = 0.1001
    _, f8 = versuch(8, 40000, rng)
    if abs(f8 - 0.75 ** 8) > 0.01:
        fehler.append(f"8 Qubits: Faelschung {f8:.4f}, erwartet {0.75 ** 8:.4f}")
    # 4. 64 Qubits: praktisch nie.
    _, f64 = versuch(64, 5000, rng)
    if f64 != 0.0:
        fehler.append(f"64 Qubits: Faelschung bestand {f64}")
    for x in fehler:
        print("FEHLER:", x)
    print("wiesner: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def main():
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    rng = random.Random()
    print("Wiesner-Quantengeld, simuliert (Pruefen kann nur die Bank)")
    print(f"{'Qubits':>6} | {'echt besteht':>12} | {'Faelschung besteht':>18} | {'Theorie (3/4)^n':>15}")
    for n in (1, 2, 4, 8, 16, 32, 64):
        e, f = versuch(n, 20000 if n <= 16 else 5000, rng)
        print(f"{n:>6} | {e:>12.4f} | {f:>18.4f} | {0.75 ** n:>15.2e}")


if __name__ == "__main__":
    main()
