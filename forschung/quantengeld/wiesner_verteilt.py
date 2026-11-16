#!/usr/bin/env python3
"""Variante W: privat pruefbare Noten, klassische Pruefung, Geheimnis verteilt.

Forschung, kein Produkt (ANALYSE.md, Abschnitt 5). Keine Abhaengigkeiten.

WAS SIMULIERT WIRD
  Eine Note sind n Qubits in BB84-Zustaenden (Basis Z oder X, Bit 0 oder 1),
  wie bei Wiesner. Anders als bei Wiesner gibt es keine Bank: Basen und Bits
  jeder Note liegen nur als Shamir-Anteile (t von v) bei den Validatoren und
  werden nie zusammengesetzt. Geprueft wird KLASSISCH: die Kette waehlt nach
  der Anfrage eine Teilmenge unbenutzter Qubits und je eine Basis; wer die
  Note haelt, misst und schickt nur die Messwerte. Die Validatoren rechnen
  gemeinsam (Beaver-Multiplikation auf den Anteilen), wie viele Stellen mit
  passender Basis es gab und wie viele davon falsch waren, und oeffnen nur
  diese zwei Zahlen. Gemessene Qubits sind verbraucht; eine abgelehnte Note
  wird gesperrt.

WAS GEZEIGT WIRD (Selbsttest)
  - Shamir: t Anteile ergeben das Geheimnis; t-1 Anteile verraten nichts
    (exakt nachgezaehlt in einem kleinen Koerper).
  - Die gemeinsame Pruefung rechnet dasselbe wie eine Bank, die alles weiss,
    ohne das Geheimnis zusammenzusetzen.
  - Echte Noten bestehen, auch unter Rauschen innerhalb der Toleranz.
  - Angriffe, alle abgewiesen bzw. mit der erwarteten Rate:
      A1 Faelschen ohne Note (zufaellige Zustaende),
      A2 Kopieren durch Messen und Nachbauen, dann doppelt ausgeben,
      A3 Raten mit Rueckmeldung (ohne Sperre steigt der Erfolg -- mit
         Sperre nicht: Gegenprobe zur Gegenmassnahme gegen adaptive Angriffe),
      A4 Absprache von weniger als t Validatoren (lernen nichts) -- und von t
         (koennen faelschen: das ist die Vertrauensannahme),
      A5 alte Antwort fuer eine neue Pruefung wiederholen,
      A6 mehr Pruefungen als Qubits,
      A7 ein Fremder sperrt eine Note mit Unsinn-Antworten -- der echte
         Halter loest sie trotzdem ein (vollstaendige Pruefung, einmalig).
  - Gegenprobe: ohne Sperre nach Ablehnung, ohne Verbrauch gemessener
    Qubits oder mit vorhersagbarer Aufforderung schlagen Tests an.

GRENZEN, EHRLICH
  - A2 simuliert den einfachen Faelscher (messen, nachbauen). Die beste
    moegliche Strategie ist schwerer zu simulieren; ihre Schranke ist
    bewiesen: zwei klassische Pruefungen aus einer Note bestehen hoechstens
    mit (3/4 + sqrt(2)/8)^n (Molina, Vidick, Watrous 2012). Die Parameter
    unten rechnen mit DIESER Schranke, nicht mit dem simulierten Faelscher.
  - Rauschtoleranz mit Beweis: Kumar 2019 (bis 21,4 %). Hier nur das
    einfache Modell aus rauschen.py (je gemessenem Qubit Kippen mit p).
  - Die Beaver-Tripel kommen hier von einem Haendler im Test; im Knoten
    kaemen sie aus der Vorberechnung (x/humanity/mpc).
  - Ein verteilter Schluessel fuer Quantengeld ist in der Literatur so nicht
    beschrieben (ANALYSE.md) -- dieses Modell ersetzt kein Gutachten.

Aufruf:  python3 -I wiesner_verteilt.py [--selbsttest]
"""
from __future__ import annotations

import math
import random
import sys

# ------------------------------------------------------------ Shamir

P = (1 << 61) - 1  # Primzahl (Mersenne), Koerper der Anteile


def teilen(wert: int, t: int, v: int, rng: random.Random, p: int = P) -> list[int]:
    """Shamir: Anteile fuer die Validatoren x = 1..v; Polynom vom Grad t-1."""
    koeff = [wert % p] + [rng.randrange(p) for _ in range(t - 1)]
    out = []
    for x in range(1, v + 1):
        y = 0
        for c in reversed(koeff):
            y = (y * x + c) % p
        out.append(y)
    return out


def zusammensetzen(anteile: dict[int, int], p: int = P) -> int:
    """Lagrange an der Stelle 0 aus {x: Anteil}."""
    s = 0
    for xi, yi in anteile.items():
        z, n = 1, 1
        for xj in anteile:
            if xj != xi:
                z = z * (-xj) % p
                n = n * (xi - xj) % p
        s = (s + yi * z * pow(n, -1, p)) % p
    return s


class Validatoren:
    """v Validatoren, Schwelle t. Halten nur Anteile; oeffnen nur Zaehler."""

    def __init__(self, t: int, v: int, rng: random.Random):
        if not 1 <= t <= v:
            raise ValueError("Schwelle")
        self.t, self.v, self.rng = t, v, rng
        self.geoeffnet = 0  # wie viele Werte je geoeffnet wurden (Zaehlung)

    def teile(self, wert: int) -> list[int]:
        return teilen(wert, self.t, self.v, self.rng)

    def oeffnen(self, anteile: list[int]) -> int:
        self.geoeffnet += 1
        return zusammensetzen({x + 1: anteile[x] for x in range(self.t)})

    # Lineare Operationen auf Anteilen: jeder Validator rechnet lokal.
    @staticmethod
    def plus(a: list[int], b: list[int]) -> list[int]:
        return [(x + y) % P for x, y in zip(a, b)]

    @staticmethod
    def mal_konst(a: list[int], c: int) -> list[int]:
        return [(x * c) % P for x in a]

    @staticmethod
    def plus_konst(a: list[int], c: int) -> list[int]:
        return [(x + c) % P for x in a]

    def mal(self, a: list[int], b: list[int]) -> list[int]:
        """Beaver-Multiplikation: ein Tripel (r, s, rs), zwei Oeffnungen
        maskierter Werte (verraten nichts, weil r, s gleichverteilt)."""
        r, s = self.rng.randrange(P), self.rng.randrange(P)
        R, S, RS = self.teile(r), self.teile(s), self.teile(r * s % P)
        d = self.oeffnen(self.plus(a, self.mal_konst(R, P - 1)))  # a - r
        e = self.oeffnen(self.plus(b, self.mal_konst(S, P - 1)))  # b - s
        z = self.plus(RS, self.plus(self.mal_konst(S, d), self.mal_konst(R, e)))
        return self.plus_konst(z, d * e % P)


# ------------------------------------------------------------ Physik

ZUSTAND = {}  # (basis, bit) -> Amplituden; Basis 0 = Z, 1 = X
_S = 1 / math.sqrt(2)
ZUSTAND[(0, 0)], ZUSTAND[(0, 1)] = (1.0, 0.0), (0.0, 1.0)
ZUSTAND[(1, 0)], ZUSTAND[(1, 1)] = (_S, _S), (_S, -_S)


def messen(qubit, basis: int, rng: random.Random, p_kipp: float = 0.0):
    """Projektive Messung; mit p_kipp kippt das Ergebnis (Rauschen nach Lagerung)."""
    a, b = qubit
    z = ZUSTAND[(basis, 0)]
    p0 = (a * z[0] + b * z[1]) ** 2
    erg = 0 if rng.random() < p0 else 1
    if p_kipp and rng.random() < p_kipp:
        erg ^= 1
    return erg, ZUSTAND[(basis, erg)]


# ------------------------------------------------------------ Kette

class Note:
    """Was der Halter hat: die Qubits (gemessene sind weg)."""

    def __init__(self, qubits):
        self.qubits = list(qubits)


class Kette:
    """Register der Noten: Anteile bei den Validatoren, benutzte Stellen,
    Sperre. Prueft klassisch und gemeinsam."""

    def __init__(self, val: Validatoren, toleranz: float, mindest_treffer: int,
                 sperre_nach_ablehnung: bool = True, verbrauch: bool = True,
                 vorhersagbar: bool = False):
        self.val = val
        self.toleranz = toleranz            # erlaubter Anteil falscher Treffer
        self.mindest_treffer = mindest_treffer
        self.noten = {}                     # nr -> dict
        self.sperre_nach_ablehnung = sperre_nach_ablehnung  # nur fuer Gegenproben abschaltbar
        self.verbrauch = verbrauch                          # dto.
        self.vorhersagbar = vorhersagbar                    # dto.
        self._naechste = 0

    def praegen(self, n: int, rng: random.Random) -> tuple[int, Note]:
        """Basen/Bits aus gemeinsamer Zufaelligkeit (hier: rng); sofort geteilt."""
        basen = [rng.randrange(2) for _ in range(n)]
        bits = [rng.randrange(2) for _ in range(n)]
        nr = self._naechste
        self._naechste += 1
        self.noten[nr] = {
            "n": n,
            "basis": [self.val.teile(b) for b in basen],
            "bit": [self.val.teile(b) for b in bits],
            "benutzt": set(),
            "gesperrt": False,
            "eingeloest": False,
        }
        return nr, Note(ZUSTAND[(b, x)] for b, x in zip(basen, bits))

    def aufforderung(self, nr: int, k: int, rng: random.Random):
        """Nach der Anfrage: k unbenutzte Stellen und Basen, unvorhersagbar."""
        e = self.noten[nr]
        if e["gesperrt"] or e["eingeloest"]:
            return None
        frei = [i for i in range(e["n"]) if i not in e["benutzt"]]
        if len(frei) < k:
            return None  # A6: keine Pruefung mehr -- fail-closed
        r = random.Random(nr) if self.vorhersagbar else rng
        stellen = sorted(r.sample(frei, k))
        basen = [r.randrange(2) for _ in stellen]
        if self.verbrauch:
            e["benutzt"].update(stellen)
        return stellen, basen

    def zaehlen(self, nr: int, stellen, basen, antworten) -> tuple[int, int]:
        """Gemeinsam: Treffer = Stellen mit passender Basis, Fehler = davon
        falsche Bits. Nur diese zwei Zahlen werden geoeffnet."""
        e, V = self.noten[nr], self.val
        treffer = fehler = [0] * V.v
        for i, beta, o in zip(stellen, basen, antworten):
            b, x = e["basis"][i], e["bit"][i]
            # gleich = [b == beta]: linear, weil beta oeffentlich
            gleich = b if beta == 1 else V.plus_konst(V.mal_konst(b, P - 1), 1)
            # falsch = o XOR x: linear, weil o oeffentlich
            falsch = x if o == 0 else V.plus_konst(V.mal_konst(x, P - 1), 1)
            treffer = V.plus(treffer, gleich)
            fehler = V.plus(fehler, V.mal(gleich, falsch))
        return V.oeffnen(treffer), V.oeffnen(fehler)

    def einloesen(self, nr: int, antwort, rng: random.Random, mindest_frei: int) -> bool:
        """Vollstaendige Pruefung aller unbenutzten Qubits -- auch fuer eine
        gesperrte Note (sonst koennte jeder, der die Seriennummer kennt, eine
        fremde Note mit Unsinn-Antworten wertlos machen). Genau EIN Versuch:
        danach ist die Note eingeloest oder tot; Raten ueber Einloesen geht
        nicht."""
        e = self.noten[nr]
        if e["eingeloest"]:
            return False
        frei = [i for i in range(e["n"]) if i not in e["benutzt"]]
        e["eingeloest"] = True
        if len(frei) < mindest_frei:
            return False
        basen = [rng.randrange(2) for _ in frei]
        antworten = antwort(frei, basen)
        if len(antworten) != len(frei) or any(o not in (0, 1) for o in antworten):
            return False
        t, f = self.zaehlen(nr, frei, basen, antworten)
        return t >= self.mindest_treffer and f <= math.floor(self.toleranz * t)

    def pruefen(self, nr: int, stellen, basen, antworten) -> bool:
        e = self.noten[nr]
        if e["gesperrt"] or e["eingeloest"] or len(antworten) != len(stellen) or any(o not in (0, 1) for o in antworten):
            return False
        t, f = self.zaehlen(nr, stellen, basen, antworten)
        ok = t >= self.mindest_treffer and f <= math.floor(self.toleranz * t)
        if not ok and self.sperre_nach_ablehnung:
            e["gesperrt"] = True
        return ok


def halter_antwortet(note: Note, stellen, basen, rng, p_kipp=0.0):
    out = []
    for i, beta in zip(stellen, basen):
        erg, _ = messen(note.qubits[i], beta, rng, p_kipp)
        out.append(erg)
    return out


# ------------------------------------------------------------ Bank zum Vergleich

def bank_zaehlt(basen, bits, stellen, ch_basen, antworten):
    t = f = 0
    for i, beta, o in zip(stellen, ch_basen, antworten):
        if basen[i] == beta:
            t += 1
            f += o != bits[i]
    return t, f


# ------------------------------------------------------------ Parameter

def mvw_qubits(ziel_bits: float) -> int:
    """n fuer (3/4 + sqrt(2)/8)^n <= 2^-ziel (bewiesene Schranke, ohne Rauschen)."""
    return math.ceil(ziel_bits / -math.log2(0.75 + math.sqrt(2) / 8))


def wiesner_qubits(ziel_bits: float) -> int:
    """n fuer (3/4)^n <= 2^-ziel (Pruefung mit Rueckgabe der Note)."""
    return math.ceil(ziel_bits / -math.log2(0.75))


# ------------------------------------------------------------ Selbsttest

def selbsttest() -> bool:
    fehler: list[str] = []
    try:
        _selbsttest(fehler)
    except Exception as e:  # ein Absturz ist ein Fehler, die Befunde davor bleiben
        fehler.append(f"Ausnahme: {type(e).__name__}: {e}")
    for x in fehler:
        print("FEHLER:", x)
    print("wiesner_verteilt: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def _selbsttest(fehler: list[str]) -> None:
    rng = random.Random(2026)

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    # Shamir: t von v setzt zusammen, egal welche t.
    for t, v in ((1, 1), (2, 3), (3, 5), (5, 7)):
        w = rng.randrange(P)
        a = teilen(w, t, v, rng)
        for _ in range(5):
            xs = rng.sample(range(1, v + 1), t)
            soll(zusammensetzen({x: a[x - 1] for x in xs}) == w, f"Shamir {t} von {v}")
    # t-1 Anteile verraten nichts: im Koerper F_11, Schwelle 3, zwei Anteile
    # an x=1,2 -- fuer JEDES Geheimnis gibt es gleich viele Polynome, die
    # dieselben zwei Anteile liefern (exakt gezaehlt).
    p = 11
    for a1 in range(p):
        for a2 in range(p):
            zahl = []
            for geheim in range(p):
                n = 0
                for c1 in range(p):
                    for c2 in range(p):
                        if (geheim + c1 + c2) % p == a1 and (geheim + 2 * c1 + 4 * c2) % p == a2:
                            n += 1
                zahl.append(n)
            if len(set(zahl)) != 1:
                soll(False, "t-1 Anteile verraten etwas")
                break

    # Beaver: gemeinsames Produkt stimmt.
    V = Validatoren(3, 5, rng)
    for _ in range(50):
        x, y = rng.randrange(2), rng.randrange(2)
        soll(V.oeffnen(V.mal(V.teile(x), V.teile(y))) == x * y, "Beaver-Produkt")

    # Gemeinsame Pruefung = Bank, die alles weiss.
    V = Validatoren(4, 7, rng)
    K = Kette(V, toleranz=0.1, mindest_treffer=1)
    for _ in range(20):
        basen = [rng.randrange(2) for _ in range(16)]
        bits = [rng.randrange(2) for _ in range(16)]
        nr = K._naechste
        K._naechste += 1
        K.noten[nr] = {"n": 16, "basis": [V.teile(b) for b in basen], "bit": [V.teile(b) for b in bits],
                       "benutzt": set(), "gesperrt": False, "eingeloest": False}
        st = sorted(rng.sample(range(16), 8))
        cb = [rng.randrange(2) for _ in st]
        an = [rng.randrange(2) for _ in st]
        soll(K.zaehlen(nr, st, cb, an) == bank_zaehlt(basen, bits, st, cb, an), "MPC zaehlt anders als die Bank")

    def kette(**kw):
        return Kette(Validatoren(3, 5, rng), toleranz=kw.pop("toleranz", 0.15), mindest_treffer=kw.pop("mindest", 8), **kw)

    N, K_PRUEF = 192, 48

    # Echte Noten bestehen, ohne und mit Rauschen (p = 2 %, Toleranz 15 %:
    # ~24 Treffer, bis 3 falsch erlaubt; P[>= 4 Fehler] ~ 0,2 %).
    for p_kipp, soll_rate in ((0.0, 1.0), (0.02, 0.98)):
        K = kette()
        ok = 0
        for _ in range(150):
            nr, note = K.praegen(N, rng)
            st, cb = K.aufforderung(nr, K_PRUEF, rng)
            ok += K.pruefen(nr, st, cb, halter_antwortet(note, st, cb, rng, p_kipp))
        soll(ok / 150 >= soll_rate, f"echte Noten (Rauschen {p_kipp}): {ok}/150")

    # Eine Note traegt N // K_PRUEF Pruefungen, dann keine mehr (A6).
    K = kette()
    nr, note = K.praegen(N, rng)
    for i in range(N // K_PRUEF):
        st, cb = K.aufforderung(nr, K_PRUEF, rng)
        soll(K.pruefen(nr, st, cb, halter_antwortet(note, st, cb, rng)), f"Pruefung {i + 1} der echten Note")
    soll(K.aufforderung(nr, K_PRUEF, rng) is None, "A6: mehr Pruefungen als Qubits angenommen")

    # A1: Faelschen ohne Note -- zufaellige Antworten. Je Treffer 1/2 richtig;
    # bei ~24 Treffern und hoechstens 3 Fehlern ~ 1,4e-4 je Versuch.
    K = kette()
    durch = 0
    for _ in range(300):
        nr, _ = K.praegen(N, rng)
        st, cb = K.aufforderung(nr, K_PRUEF, rng)
        durch += K.pruefen(nr, st, cb, [rng.randrange(2) for _ in st])
    soll(durch <= 2, f"A1: {durch}/300 Faelschungen ohne Note bestanden")

    # A2: Kopieren (Basis raten, messen, zwei Kopien) und beide ausgeben.
    # Einzelner Treffer: beide Kopien bestehen mit 25/32 je Qubit (siehe
    # Text); mit Toleranz 0 und kleinem n gegen die Rechnung.
    K = kette(toleranz=0.0, mindest=0)
    beide, laeufe, n_klein = 0, 4000, 4
    for _ in range(laeufe):
        nr, note = K.praegen(n_klein, rng)
        k1, k2 = [], []
        for q in note.qubits:
            g = rng.randrange(2)
            erg, z = messen(q, g, rng)
            k1.append(z)
            k2.append(ZUSTAND[(g, erg)])
        # zwei Pruefungen derselben Seriennummer auf denselben Stellen sind
        # nicht moeglich (verbraucht) -- der Faelscher braucht zwei Noten-
        # nummern: hier das schwaechste Modell (gleiche Nummer, ohne Verbrauch).
        # Ohne Sperre: gemessen wird, wie oft BEIDE Kopien bestehen; die
        # Sperre ist eine eigene Gegenmassnahme (A3).
        K2 = Kette(K.val, toleranz=0.0, mindest_treffer=0, verbrauch=False, sperre_nach_ablehnung=False)
        K2.noten[0] = K.noten[nr]
        ok = True
        for kopie in (k1, k2):
            st, cb = K2.aufforderung(0, n_klein, rng)
            ok = ok and K2.pruefen(0, st, cb, halter_antwortet(Note(kopie), st, cb, rng))
        beide += ok
    erwartet = (25 / 32) ** n_klein
    soll(abs(beide / laeufe - erwartet) < 0.03, f"A2: beide Kopien {beide / laeufe:.3f}, erwartet {erwartet:.3f}")
    soll(erwartet <= (0.75 + math.sqrt(2) / 8) ** n_klein, "A2 ueber der bewiesenen Schranke")

    # A3: Raten mit Rueckmeldung. Ohne Note, viele Versuche auf DIESELBE Note.
    # Mit Sperre: nach der ersten Ablehnung ist Schluss. Gegenprobe ohne Sperre:
    # mit vielen Versuchen kommt der Faelscher irgendwann durch.
    def raten(sperre: bool) -> int:
        K = kette(sperre_nach_ablehnung=sperre, mindest=2, toleranz=0.0)
        durch = 0
        for _ in range(60):
            nr, _ = K.praegen(64, rng)
            for _ in range(16):
                a = K.aufforderung(nr, 4, rng)
                if a is None:
                    break
                st, cb = a
                if K.pruefen(nr, st, cb, [rng.randrange(2) for _ in st]):
                    durch += 1
                    break
        return durch
    mit, ohne = raten(True), raten(False)
    # Je Versuch ~0,13; mit Sperre ein Versuch je Note (~8 von 60), ohne
    # Sperre bis 16 Versuche (~53 von 60).
    soll(ohne > 3 * max(mit, 1), f"A3: Gegenprobe ohne Sperre greift nicht ({ohne} vs {mit})")
    soll(mit <= 18, f"A3: mit Sperre {mit}/60 durch")

    # A7: Ein Fremder kennt die Seriennummer, fordert eine Pruefung an und
    # schickt Unsinn -- die Note ist gesperrt. Der echte Halter loest sie
    # trotzdem ein (vollstaendig, einmal). Ein Faelscher bekommt beim
    # Einloesen genau einen Versuch.
    K = kette()
    nr, note = K.praegen(N, rng)
    st, cb = K.aufforderung(nr, K_PRUEF, rng)
    K.pruefen(nr, st, cb, [1 - o for o in halter_antwortet(note, st, cb, rng)])
    soll(K.noten[nr]["gesperrt"] and K.aufforderung(nr, K_PRUEF, rng) is None, "A7: Unsinn sperrt nicht")
    soll(K.einloesen(nr, lambda s, b: halter_antwortet(note, s, b, rng), rng, mindest_frei=64), "A7: echter Halter loest gesperrte Note nicht ein")
    soll(not K.einloesen(nr, lambda s, b: halter_antwortet(note, s, b, rng), rng, mindest_frei=64), "A7: zweimal eingeloest")
    nr, _ = K.praegen(N, rng)
    soll(not K.einloesen(nr, lambda s, b: [rng.randrange(2) for _ in s], rng, mindest_frei=64), "A7: Faelscher loest ein")
    soll(not K.einloesen(nr, lambda s, b: [rng.randrange(2) for _ in s], rng, mindest_frei=64), "A7: zweiter Versuch")

    # A4: t-1 Validatoren setzen nichts zusammen (Shamir oben); t koennen es.
    V = Validatoren(3, 5, rng)
    K = Kette(V, toleranz=0.0, mindest_treffer=1)
    nr, _ = K.praegen(32, rng)
    e = K.noten[nr]
    basen = [zusammensetzen({x + 1: e["basis"][i][x] for x in range(3)}) for i in range(32)]
    bits = [zusammensetzen({x + 1: e["bit"][i][x] for x in range(3)}) for i in range(32)]
    falsch = Note(ZUSTAND[(b, x)] for b, x in zip(basen, bits))
    st, cb = K.aufforderung(nr, 16, rng)
    soll(K.pruefen(nr, st, cb, halter_antwortet(falsch, st, cb, rng)), "A4: t Validatoren koennen nachbauen (Vertrauensannahme)")
    zwei = [zusammensetzen({x + 1: e["basis"][i][x] for x in range(2)}) for i in range(32)]
    soll(zwei != basen, "A4: zwei von drei Anteilen ergeben die Basen")

    # A5: alte Antwort fuer eine neue Aufforderung.
    K = kette()
    nr, note = K.praegen(N, rng)
    st, cb = K.aufforderung(nr, K_PRUEF, rng)
    alt = halter_antwortet(note, st, cb, rng)
    soll(K.pruefen(nr, st, cb, alt), "A5: erste Antwort")
    st2, cb2 = K.aufforderung(nr, K_PRUEF, rng)
    soll(set(st2).isdisjoint(st), "A5: gemessene Stellen erneut gefordert")
    soll(not K.pruefen(nr, st2, cb2, alt), "A5: alte Antwort fuer neue Aufforderung angenommen")

    # Gegenprobe: vorhersagbare Aufforderung -- wer die Note einmal hatte,
    # rechnet die Antworten auf kuenftige Pruefungen schon vorher aus und
    # gibt die Note weiter: dann kann er dieselbe Note ein zweites Mal
    # "vorzeigen" (Doppel-Ausgabe). Mit Zufall nach der Anfrage nicht.
    for vorhersagbar, erwart in ((True, True), (False, False)):
        K = kette(vorhersagbar=vorhersagbar)
        nr, note = K.praegen(N, rng)
        r = random.Random(nr)
        frei = list(range(N))
        st_v = sorted(r.sample(frei, K_PRUEF))
        cb_v = [r.randrange(2) for _ in st_v]
        vorab = halter_antwortet(note, st_v, cb_v, rng)  # vor der Aufforderung gemessen
        st, cb = K.aufforderung(nr, K_PRUEF, rng)
        soll(K.pruefen(nr, st, cb, vorab if (st, cb) == (st_v, cb_v) else [rng.randrange(2) for _ in st]) == erwart,
             f"Gegenprobe vorhersagbar={vorhersagbar}")

    # Parameter: bewiesene Schranke.
    soll(wiesner_qubits(128) == 309, "309 Qubits fuer 2^-128 (Rueckgabe)")
    soll(1160 <= mvw_qubits(128) <= 1175, f"MVW 2^-128: {mvw_qubits(128)}")


def main():
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    print("Variante W -- Qubits je Note fuer ein Sicherheitsziel (ohne Rauschen, bewiesene Schranken)")
    print(f"{'Ziel':>8} | {'Pruefung mit Rueckgabe (3/4)^n':>31} | {'klassische Pruefung (3/4+sqrt2/8)^n':>36}")
    for z in (64, 80, 128, 192):
        print(f"{'2^-' + str(z):>8} | {wiesner_qubits(z):>31} | {mvw_qubits(z):>36}")
    print()
    print("Einfacher Kopierer (messen, nachbauen, doppelt ausgeben): 25/32 je Qubit,")
    print(f"bewiesene beste Strategie: {0.75 + math.sqrt(2) / 8:.4f} je Qubit (Molina, Vidick, Watrous 2012).")


if __name__ == "__main__":
    main()
