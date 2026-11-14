#!/usr/bin/env python3
"""Oeffentlich pruefbares Quantengeld aus einer Gruppenaktion -- SPIELZEUG.

Idee (Zhandry 2024, "Quantum Money from Abelian Group Actions"): Eine abelsche
Gruppe G wirkt auf eine Menge X (g * x). Eine Banknote ist der verdrehte
Bahnzustand

    |$_y>  =  1/sqrt(|G|) * Summe_g  chi_y(g) |g * x0>,

mit dem Charakter chi_y(g) = exp(2 pi i y g / N). Die Seriennummer ist y.
|$_y> ist Eigenzustand der Wirkung jedes Gruppenelements h:
    U_h |$_y> = conj(chi_y(h)) |$_y>.
Deshalb kann JEDER pruefen, ohne Geheimnis: die Wirkung von G ist oeffentlich,
Phasenschaetzung liefert y, und der Vergleich mit der Seriennummer entscheidet.
Gedruckt wird so: zwei Register in den verschraenkten Zustand
Summe_g |g>|g * x0> bringen, auf das erste die Quanten-Fouriertransformation
anwenden und es messen. Das Ergebnis y ist zufaellig gleichverteilt, und das
zweite Register ist danach die Note zur Seriennummer -y. Niemand, auch die
Ausgabestelle nicht, kann sich y aussuchen. (Die gleichmaessige Ueberlagerung
der Bahn allein reicht NICHT: sie ist selbst die Note mit y = 0 -- der
Selbsttest hat genau diesen Fehler im ersten Entwurf gefunden.) Sicherheit beruht darauf, dass man aus y den Zustand |$_y> NICHT
direkt herstellen kann: das hiesse, in der Gruppenaktion den diskreten
Logarithmus zu loesen (fuer Isogenien-Gruppenaktionen vermutlich schwer, auch
fuer Quantenrechner; 2025 im generischen Modell bewiesen, nicht aus
Standardannahmen).

DIESES SPIELZEUG nimmt G = Z_N, das auf X = Z_N durch Addition wirkt. Dort ist
der diskrete Logarithmus trivial (g * x0 = x0 + g). Die Simulation zeigt darum
beides:
  1. Drucken und oeffentliches Pruefen funktionieren -- ohne Bank.
  2. Eine kopierte Note gibt es nicht (Kopierverbot), aber:
  3. ANGRIFF: Wer y kennt, baut |$_y> direkt nach (Fourier-Zustand von |y>).
     In Z_N ist das Verfahren deshalb voellig unsicher. Genau hier sitzt die
     offene Mathematik: eine Gruppenaktion, in der das schwer ist.

Exakt gerechnet als Zustandsvektor der Laenge N (komplexe Zahlen), Messungen
als Projektionen. Keine Abhaengigkeiten.

Aufruf:  python3 -I zhandry_spielzeug.py [--selbsttest]
"""
import cmath
import math
import random
import sys

N = 32  # Groesse der Spielzeuggruppe Z_N


def chi(y, g):
    return cmath.exp(2j * math.pi * y * g / N)


def banknote(y):
    """|$_y> in der Basis |x>, x in Z_N (x0 = 0, also g * x0 = g)."""
    return [chi(y, g) / math.sqrt(N) for g in range(N)]


NOTEN = [banknote(y) for y in range(N)]


def basiszustand(x):
    """Klassisch bekanntes |x> -- was ein Faelscher ohne Quantenzustand hat."""
    z = [0j] * N
    z[x] = 1 + 0j
    return z


def skalarprodukt(a, b):
    return sum(x.conjugate() * z for x, z in zip(a, b))


def wirkung(h, zustand):
    """Oeffentliche Wirkung U_h: |x> -> |h + x>."""
    neu = [0j] * N
    for x, amp in enumerate(zustand):
        neu[(x + h) % N] += amp
    return neu


def phasenschaetzung(zustand, rng):
    """Ideale Phasenschaetzung der Wirkung = Messung in der Basis {|$_y>}.
    Gibt (y, Zustand danach)."""
    ws = [abs(skalarprodukt(NOTEN[y], zustand)) ** 2 for y in range(N)]
    r, summe = rng.random() * sum(ws), 0.0
    for y, w in enumerate(ws):
        summe += w
        if r <= summe:
            return y, NOTEN[y]
    return N - 1, NOTEN[N - 1]


def drucken(rng):
    """Jeder kann drucken -- aber y ist Zufall (Messergebnis).

    Gemeinsamer Zustand nach der Fouriertransformation des Gruppenregisters:
      1/N * Summe_{y,g} exp(-2 pi i y g / N) |y>|g * x0>.
    Exakt als N x N-Amplituden gerechnet, dann das erste Register gemessen."""
    gemeinsam = [[cmath.exp(-2j * math.pi * y * g / N) / N for g in range(N)] for y in range(N)]
    ws = [sum(abs(a) ** 2 for a in zeile) for zeile in gemeinsam]
    r, summe = rng.random() * sum(ws), 0.0
    for y, w in enumerate(ws):
        summe += w
        if r <= summe:
            break
    rest = gemeinsam[y]
    norm = math.sqrt(sum(abs(a) ** 2 for a in rest))
    note = [a / norm for a in rest]
    seriennummer = (-y) % N
    return seriennummer, note


def pruefen(seriennummer, zustand, rng):
    """Oeffentliche Pruefung: Phasenschaetzung muss die Seriennummer ergeben."""
    y, _ = phasenschaetzung(zustand, rng)
    return y == seriennummer


def angriff_nachbau(seriennummer):
    """Nur im Spielzeug moeglich: aus y direkt |$_y> bauen (DL in Z_N trivial)."""
    return banknote(seriennummer)


def angriff_falscher_zustand(rng):
    """Faelscher ohne Quantenzustand: ein klassisch bekanntes |x>."""
    return basiszustand(rng.randrange(N))


def eigenzustand_ok(y):
    """U_h |$_y> = conj(chi_y(h)) |$_y> fuer alle h."""
    note = banknote(y)
    for h in range(N):
        erwartet = [chi(y, h).conjugate() * a for a in note]
        ist = wirkung(h, note)
        if any(abs(a - b) > 1e-9 for a, b in zip(ist, erwartet)):
            return False
    return True


def selbsttest():
    rng = random.Random(2024)
    fehler = []
    if not all(eigenzustand_ok(y) for y in range(N)):
        fehler.append("Banknote ist kein Eigenzustand der Wirkung")
    # Echte Noten bestehen die oeffentliche Pruefung immer, auch mehrfach.
    for _ in range(200):
        y, note = drucken(rng)
        if not all(pruefen(y, note, rng) for _ in range(3)):
            fehler.append("echte Note bestand die Pruefung nicht")
            break
    # Seriennummern beim Drucken sind gleichverteilt (niemand waehlt y).
    zaehl = [0] * N
    for _ in range(N * 100):
        y, _ = drucken(rng)
        zaehl[y] += 1
    erwart = 100
    if max(abs(z - erwart) for z in zaehl) > 50:
        fehler.append(f"Seriennummern nicht gleichverteilt: {min(zaehl)}..{max(zaehl)}")
    # Falscher Zustand mit fremder Seriennummer besteht nur zu ~1/N.
    treffer = sum(pruefen(5, angriff_falscher_zustand(rng), rng) for _ in range(20000))
    if abs(treffer / 20000 - 1 / N) > 0.01:
        fehler.append(f"falscher Zustand bestand zu {treffer / 20000:.4f}, erwartet {1 / N:.4f}")
    # Spielzeug-Angriff: Nachbau aus y besteht IMMER -> im Spielzeug unsicher.
    if not all(pruefen(y, angriff_nachbau(y), rng) for y in range(N)):
        fehler.append("Nachbau-Angriff sollte im Spielzeug immer bestehen")
    for x in fehler:
        print("FEHLER:", x)
    print("zhandry_spielzeug: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def main():
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    rng = random.Random()
    y, note = drucken(rng)
    print(f"Spielzeug-Gruppe Z_{N}. Gedruckt: Seriennummer y = {y} (Zufall)")
    print("Oeffentliche Pruefung der echten Note (5x):",
          [pruefen(y, note, rng) for _ in range(5)])
    print("Falscher Zustand mit dieser Seriennummer besteht zu ~1/N:",
          sum(pruefen(y, angriff_falscher_zustand(rng), rng) for _ in range(3200)) / 3200)
    print("ANGRIFF Nachbau aus y (nur moeglich, weil der DL in Z_N trivial ist):",
          pruefen(y, angriff_nachbau(y), rng))
    print("=> Funktioniert ohne Bank, ist aber nur mit einer Gruppenaktion sicher,")
    print("   in der dieser Nachbau schwer ist (offene Forschung, siehe README.md).")


if __name__ == "__main__":
    main()
