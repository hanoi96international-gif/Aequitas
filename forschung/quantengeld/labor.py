#!/usr/bin/env python3
"""Das dezentrale Quantengeld von Aequitas -- einmal ganz durchgespielt.

Forschung, kein Produkt (BAUVORGABE §8). Keine Abhaengigkeiten.

DER ABLAUF (Zhandry 2025 als Quantum Lightning + Einweg-Bruecke):

  1. PRAEGEN -- darf JEDER. Gen: Ueberlagerung ueber die Gruppe, Aktion,
     QFT, messen -> Seriennummer h und Note |G^h * x>. Quantum Lightning:
     auch der Praeger selbst kann keine zweite Note mit DIESEM h machen
     (er muesste Gen wiederholen, bis h zufaellig wieder faellt).
  2. ANMELDEN -- auf der Kette: wer eine Note praegt, sperrt x AEQ und traegt
     SHA-256(h) ein (Einweg, Inhaberpapier). Jede Seriennummer genau einmal.
     Ohne Eintrag ist eine Note nichts wert. Keine Bank, kein Sonderrecht.
  3. BEZAHLEN -- die Note wandert als Quantenzustand, h als Zettel dazu.
  4. PRUEFEN -- jeder Empfaenger selbst: h in der Kette? Dann Variante B
     (u klassisch gleichverteilt, T Hadamard-Tests; variante_b.py).
  5. ZURUECK -- in Version 1 nur durch Tausch (Einweg-Bruecke).

Gespielt an der echten kleinen Isogenie-Gruppe (p = 868019, h = 1491; die
Zyklus-Darstellung ist in schwache_pruefung.py gegen echte Isogenien
geprueft). Die Kettenregeln sind dieselben wie in quanten_note.go
(quantenPraegen); der Pruefvektor testdata/quanten_praegen_vektor.json
haelt Python und Go Byte fuer Byte gleich.

ANGRIFFE, die abprallen muessen: nicht angemeldete Note, Seriennummer
doppelt anmelden, Note ohne Deckung, gefaelschter Zustand fuer eine echte
Seriennummer (fremde Note, Glocke, Messen-und-Nachbauen), zweite Note mit
derselben Seriennummer praegen (Lightning). Und: die Erhaltung
(Konten + angemeldete Noten = konstant).

Aufruf: python3 -I labor.py [--selbsttest] [--vektor]
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))


def _lade(name):
    spec = importlib.util.spec_from_file_location(name, os.path.join(_hier, name + ".py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


sp = _lade("schwache_pruefung")
vb = _lade("variante_b")

DOMAENE = "aequitas-quanten-serie-v1"
SERIE_MAX_BYTES = 1024  # h in Z_N, N ~ 2^4096 bei 8.192 Bit: 512 Byte; Reserve


def serie_bytes(h: int) -> bytes:
    """Kanonisch: big-endian, kuerzeste Form (h = 0 -> b"\\x00"). Sonst haette
    dieselbe Note mehrere Schreibweisen und liesse sich mehrfach anmelden."""
    return h.to_bytes(max(1, (h.bit_length() + 7) // 8), "big")


def kanonisch(serie: bytes) -> bool:
    return 0 < len(serie) <= SERIE_MAX_BYTES and (len(serie) == 1 or serie[0] != 0)


def serie_schluessel(serie: bytes) -> str:
    """Was die Kette eintraegt: SHA-256(Domaene | Seriennummer)."""
    return hashlib.sha256(DOMAENE.encode() + b"|" + serie).hexdigest()


class Kette:
    """Die Regeln der Kettenseite (wie quantenPraegen in quanten_note.go)."""

    def __init__(self, konten: dict):
        self.konten = dict(konten)
        self.register: dict[str, int] = {}  # serie_schluessel -> wei

    def summe(self) -> int:
        return sum(self.konten.values()) + sum(self.register.values())

    def anmelden(self, wer: str, serie: bytes, wei: int) -> str:
        if not kanonisch(serie):
            raise ValueError("Seriennummer in falscher Form")
        if wei <= 0 or self.konten.get(wer, 0) < wei:
            raise ValueError("keine Deckung oder Betrag ungueltig")
        k = serie_schluessel(serie)
        if k in self.register:
            raise ValueError("Seriennummer schon angemeldet")
        self.konten[wer] -= wei
        self.register[k] = wei
        return k

    def wert(self, serie: bytes) -> int:
        if not kanonisch(serie):
            return 0
        return self.register.get(serie_schluessel(serie), 0)


class Labor:
    def __init__(self, rng: random.Random):
        self.rng = rng
        self.g = sp.Gruppe(random.Random(1491))

    def praegen(self):
        """Gen: Seriennummer gleichverteilt (Messung nach der QFT), Note dazu."""
        h = self.rng.randrange(self.g.h)
        return serie_bytes(h), self.g.note(h)

    def pruefen(self, kette: Kette, serie: bytes, psi: list, tests: int) -> tuple[bool, float]:
        """Empfaenger: angemeldet und h < Gruppenordnung? Dann Variante B.
        Rueckgabe (besteht, P)."""
        h = int.from_bytes(serie, "big")
        if kette.wert(serie) == 0 or h >= self.g.h:
            return False, 0.0
        p = vb.mittel(self.g, h, psi, tests, 40, self.rng)
        return self.rng.random() < p, p


def lauf(rng: random.Random, tests: int = 6) -> dict:
    lab = Labor(rng)
    kette = Kette({"anna": 1000, "ben": 0, "dieb": 5})
    start = kette.summe()
    erg = {}

    # 1-4: Anna praegt, meldet an, bezahlt Ben; Ben prueft.
    serie, note = lab.praegen()
    kette.anmelden("anna", serie, 300)
    ok, p = lab.pruefen(kette, serie, note, tests)
    erg["echte Note"] = p

    # Angriffe
    s2, n2 = lab.praegen()
    erg["nicht angemeldete Note"] = lab.pruefen(kette, s2, n2, tests)[1]
    try:
        kette.anmelden("dieb", serie, 1)
        erg["Seriennummer doppelt anmelden"] = "ANGENOMMEN"
    except ValueError:
        erg["Seriennummer doppelt anmelden"] = "abgewiesen"
    try:
        kette.anmelden("dieb", lab.praegen()[0], 100)
        erg["Note ohne Deckung anmelden"] = "ANGENOMMEN"
    except ValueError:
        erg["Note ohne Deckung anmelden"] = "abgewiesen"
    erg["fremde Note unter echter Seriennummer"] = lab.pruefen(kette, serie, n2, tests)[1]
    erg["glatte Faelschung (Glocke)"] = lab.pruefen(kette, serie, lab.g.glocke(int.from_bytes(serie, "big"), 0.5), tests)[1]
    # Messen und nachbauen: Basiszustand |k> mit k nach |<k|note>|^2.
    k = rng.randrange(lab.g.h)
    basis = [0j] * lab.g.h
    basis[k] = 1
    erg["Messen und nachbauen"] = lab.pruefen(kette, serie, basis, tests)[1]
    # Lightning: zweite Note mit derselben Seriennummer praegen.
    versuche = 0
    while lab.praegen()[0] != serie:
        versuche += 1
    erg["Lightning: Versuche bis zur gleichen Seriennummer"] = versuche
    # Dieselbe Seriennummer in zweiter Schreibweise (fuehrende Null).
    try:
        kette.anmelden("anna", b"\x00" + serie, 1)
        erg["dieselbe Seriennummer mit fuehrender Null"] = "ANGENOMMEN"
    except ValueError:
        erg["dieselbe Seriennummer mit fuehrender Null"] = "abgewiesen"
    erg["Erhaltung"] = kette.summe() == start
    erg["h"] = lab.g.h
    return erg


def vektor() -> dict:
    """Pruefvektor fuer quanten_note.go (quantenPraegen)."""
    serie = serie_bytes(1234)
    return {"domaene": DOMAENE, "serie_hex": serie.hex(), "schluessel": serie_schluessel(serie),
            "serie_max_bytes": SERIE_MAX_BYTES,
            "null_hex": serie_bytes(0).hex(), "null_schluessel": serie_schluessel(serie_bytes(0)),
            "nicht_kanonisch_hex": ["", "0000", "0004d2", "00ff"]}


def selbsttest() -> None:
    rng = random.Random(9)
    e = lauf(rng)
    assert e["echte Note"] > 0.999, e
    assert e["nicht angemeldete Note"] == 0.0, e
    assert e["Seriennummer doppelt anmelden"] == "abgewiesen", e
    assert e["Note ohne Deckung anmelden"] == "abgewiesen", e
    assert e["dieselbe Seriennummer mit fuehrender Null"] == "abgewiesen", e
    t = 6
    assert e["fremde Note unter echter Seriennummer"] < 2 ** -t + 0.03, e
    assert e["Messen und nachbauen"] < 2 ** -t + 1 / e["h"] + 0.03, e
    assert e["glatte Faelschung (Glocke)"] < 0.2, e
    assert e["Erhaltung"] is True, e
    assert e["Lightning: Versuche bis zur gleichen Seriennummer"] >= 0
    # Kettenregeln einzeln
    k = Kette({"a": 10})
    zu_lang = b"\x01" * (SERIE_MAX_BYTES + 1)
    for schlecht in [bytes.fromhex(x) for x in vektor()["nicht_kanonisch_hex"]] + [zu_lang]:
        try:
            k.anmelden("a", schlecht, 1)
            raise AssertionError("falsche Form angenommen")
        except ValueError:
            pass
        assert k.wert(schlecht) == 0
    for h in (0, 1, 255, 256, 1490, 2 ** 4095):
        assert kanonisch(serie_bytes(h)) and int.from_bytes(serie_bytes(h), "big") == h
    # Pruefvektor stimmt mit der Datei, die der Go-Test liest
    pfad = os.path.join(_hier, "..", "..", "x", "humanity", "keeper", "testdata", "quanten_praegen_vektor.json")
    if os.path.exists(pfad):
        assert json.load(open(pfad)) == vektor(), "Pruefvektor veraltet: python3 -I labor.py --vektor"
    print("selbsttest ok")


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    if "--vektor" in sys.argv:
        print(json.dumps(vektor(), indent=2))
        return
    e = lauf(random.Random(3))
    print(f"Dezentrales Quantengeld, ganz durchgespielt (echte Isogenie-Gruppe, h = {e['h']}, 6 Tests je Pruefung):\n")
    print("| Schritt / Angriff | Ergebnis |")
    print("|---|---|")
    for k, v in e.items():
        if k == "h":
            continue
        print(f"| {k} | {v if not isinstance(v, float) else f'besteht mit {v:.3f}'} |")
    print("\nBei kryptografischer Groesse: Lightning-Versuche ~ 2^4096, Faelschungen <= 2^-128 (128 Tests).")


if __name__ == "__main__":
    main()
