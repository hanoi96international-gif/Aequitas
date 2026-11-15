#!/usr/bin/env python3
"""Das dezentrale Quantengeld von Aequitas -- einmal ganz durchgespielt.

Forschung, kein Produkt (BAUVORGABE §8). Keine Abhaengigkeiten.

DER ABLAUF (Zhandry 2025 als Quantum Lightning + Einweg-Bruecke):

  1. PRAEGEN -- darf JEDER. Gen: Ueberlagerung ueber die Gruppe, Aktion,
     QFT, messen -> Seriennummer h und Note |G^h * x>. Quantum Lightning:
     auch der Praeger selbst kann keine zweite Note mit DIESEM h machen
     (er muesste Gen wiederholen, bis h zufaellig wieder faellt).
  2. ANMELDEN -- auf der Kette: wer eine Note praegt, sperrt x AEQ und traegt
     K = SHA-256(Domaene | Anmelder | h) ein (Einweg, Inhaberpapier); je
     Anmelder jede Seriennummer genau einmal. Der Anmelder gehoert in K, damit
     niemand die Anmeldung vorlaufend blockieren kann. Ohne Eintrag ist eine
     Note nichts wert. Keine Bank, kein Sonderrecht.
  3. BEZAHLEN -- die Note wandert als Quantenzustand, (Anmelder, h) als
     Zettel dazu.
  4. PRUEFEN -- jeder Empfaenger selbst: K in der Kette, h < Gruppenordnung?
     Dann Variante B (u klassisch gleichverteilt, T Hadamard-Tests;
     variante_b.py).
  5. ZURUECK -- in Version 1 nur durch Tausch (Einweg-Bruecke).

Gespielt an der echten kleinen Isogenie-Gruppe (p = 868019, h = 1491; die
Zyklus-Darstellung ist in schwache_pruefung.py gegen echte Isogenien
geprueft). Die Kettenregeln sind dieselben wie in quanten_note.go
(quantenPraegen); der Pruefvektor testdata/quanten_praegen_vektor.json
haelt Python und Go Byte fuer Byte gleich.

ANGRIFFE, die abprallen muessen: nicht angemeldete Note, Seriennummer
doppelt oder in zweiter Schreibweise anmelden, Vorlaufen, Note ohne Deckung, gefaelschter Zustand fuer eine echte
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
ANNA = "0x00000000000000000000000000000000000000a1"
BEN = "0x00000000000000000000000000000000000000b2"
DIEB = "0x00000000000000000000000000000000000000d3"
# Der Spielzeug-Parametersatz dieses Labors (h = 1491). Gehoert NIE in den
# Knoten: dort verlangt ein Waechtertest N >= 2^256.
SATZ = "labor-h1491"
SERIE_MAX_BYTES = 1024  # h in Z_N, N ~ 2^4096 bei 8.192 Bit: 512 Byte; Reserve


def serie_bytes(h: int) -> bytes:
    """Kanonisch: big-endian, kuerzeste Form (h = 0 -> b"\\x00"). Sonst haette
    dieselbe Note mehrere Schreibweisen und liesse sich mehrfach anmelden."""
    return h.to_bytes(max(1, (h.bit_length() + 7) // 8), "big")


def kanonisch(serie: bytes) -> bool:
    return 0 < len(serie) <= SERIE_MAX_BYTES and (len(serie) == 1 or serie[0] != 0)


def serie_schluessel(satz: str, von: str, serie: bytes) -> str:
    """Was die Kette eintraegt: SHA-256(Domaene | Satz | Anmelder | Seriennummer).
    Der Anmelder (kanonische 0x-Adresse, feste Laenge) gehoert dazu: sonst
    koennte jeder, der die Anmeldung im Mempool sieht, dieselbe Seriennummer
    mit 1 Wei zuerst anmelden und die echte Anmeldung blockieren. Der
    Parametersatz gehoert dazu, damit gleiche Zahlen aus verschiedenen
    Gruppen nie dieselbe Anmeldung sind."""
    return hashlib.sha256(
        DOMAENE.encode() + b"|" + satz.encode() + b"|" + von.encode() + b"|" + serie
    ).hexdigest()


def satz_gueltig(satz: str) -> bool:
    """Name eines Parametersatzes: 1..32 Zeichen a-z 0-9 '-' (kein '|')."""
    return 0 < len(satz) <= 32 and all(c in "abcdefghijklmnopqrstuvwxyz0123456789-" for c in satz)


class Kette:
    """Die Regeln der Kettenseite (wie quantenPraegen in quanten_note.go).

    saetze: Parametersatz -> Gruppenordnung N. Die Kette weist jede
    Seriennummer h >= N ab (dieselbe Note haette sonst die Seriennummern h
    und h + N). Unbekannter Satz -> abgewiesen."""

    def __init__(self, konten: dict, saetze: dict):
        self.konten = dict(konten)
        self.saetze = dict(saetze)
        self.register: dict[str, int] = {}  # serie_schluessel -> wei

    def _form(self, satz: str, serie: bytes) -> bool:
        n = self.saetze.get(satz) if satz_gueltig(satz) else None
        return n is not None and kanonisch(serie) and int.from_bytes(serie, "big") < n

    def summe(self) -> int:
        return sum(self.konten.values()) + sum(self.register.values())

    def anmelden(self, wer: str, satz: str, serie: bytes, wei: int) -> str:
        if not self._form(satz, serie):
            raise ValueError("Seriennummer in falscher Form, zu gross oder Satz unbekannt")
        if wei <= 0 or self.konten.get(wer, 0) < wei:
            raise ValueError("keine Deckung oder Betrag ungueltig")
        k = serie_schluessel(satz, wer, serie)
        if k in self.register:
            raise ValueError("Seriennummer schon angemeldet")
        self.konten[wer] -= wei
        self.register[k] = wei
        return k

    def wert(self, satz: str, von: str, serie: bytes) -> int:
        if not self._form(satz, serie):
            return 0
        return self.register.get(serie_schluessel(satz, von, serie), 0)


class Labor:
    def __init__(self, rng: random.Random):
        self.rng = rng
        self.g = sp.Gruppe(random.Random(1491))

    def praegen(self):
        """Gen: Seriennummer gleichverteilt (Messung nach der QFT), Note dazu."""
        h = self.rng.randrange(self.g.h)
        return serie_bytes(h), self.g.note(h)

    def pruefen(self, kette: Kette, von: str, serie: bytes, psi: list, tests: int) -> tuple[bool, float]:
        """Empfaenger: angemeldet (die Kette prueft auch h < N)? Dann
        Variante B. Rueckgabe (besteht, P)."""
        h = int.from_bytes(serie, "big")
        if kette.wert(SATZ, von, serie) == 0:
            return False, 0.0
        p = vb.mittel(self.g, h, psi, tests, 40, self.rng)
        return self.rng.random() < p, p


def lauf(rng: random.Random, tests: int = 6) -> dict:
    lab = Labor(rng)
    kette = Kette({ANNA: 1000, BEN: 0, DIEB: 5}, {SATZ: lab.g.h})
    start = kette.summe()
    erg = {}

    # 1-4: Anna praegt, meldet an, bezahlt Ben; Ben prueft.
    serie, note = lab.praegen()
    kette.anmelden(ANNA, SATZ, serie, 300)
    ok, p = lab.pruefen(kette, ANNA, serie, note, tests)
    erg["echte Note"] = p

    # Angriffe
    s2, n2 = lab.praegen()
    erg["nicht angemeldete Note"] = lab.pruefen(kette, ANNA, s2, n2, tests)[1]
    try:
        kette.anmelden(ANNA, SATZ, serie, 1)
        erg["Seriennummer doppelt anmelden"] = "ANGENOMMEN"
    except ValueError:
        erg["Seriennummer doppelt anmelden"] = "abgewiesen"
    # Vorlaufen: der Dieb sieht Bens Anmeldung im Mempool und meldet die
    # Seriennummer zuerst unter sich an. Bens Anmeldung muss trotzdem gehen,
    # und unter Dieb hat die Note fuer Ben keinen Wert (er prueft ben|serie).
    kette.konten[BEN] = 50
    s3, n3 = lab.praegen()
    kette.anmelden(DIEB, SATZ, s3, 1)
    try:
        kette.anmelden(BEN, SATZ, s3, 50)
        erg["Vorlaufen blockiert die Anmeldung nicht"] = kette.wert(SATZ, BEN, s3) == 50
    except ValueError:
        erg["Vorlaufen blockiert die Anmeldung nicht"] = False
    start += 50
    try:
        kette.anmelden(DIEB, SATZ, lab.praegen()[0], 100)
        erg["Note ohne Deckung anmelden"] = "ANGENOMMEN"
    except ValueError:
        erg["Note ohne Deckung anmelden"] = "abgewiesen"
    erg["fremde Note unter echter Seriennummer"] = lab.pruefen(kette, ANNA, serie, n2, tests)[1]
    erg["glatte Faelschung (Glocke)"] = lab.pruefen(kette, ANNA, serie, lab.g.glocke(int.from_bytes(serie, "big"), 0.5), tests)[1]
    # Messen und nachbauen: Basiszustand |k> mit k nach |<k|note>|^2.
    k = rng.randrange(lab.g.h)
    basis = [0j] * lab.g.h
    basis[k] = 1
    erg["Messen und nachbauen"] = lab.pruefen(kette, ANNA, serie, basis, tests)[1]
    # Lightning: zweite Note mit derselben Seriennummer praegen.
    versuche = 0
    while lab.praegen()[0] != serie:
        versuche += 1
    erg["Lightning: Versuche bis zur gleichen Seriennummer"] = versuche
    # Dieselbe Seriennummer in zweiter Schreibweise (fuehrende Null).
    try:
        kette.anmelden(ANNA, SATZ, b"\x00" + serie, 1)
        erg["dieselbe Seriennummer mit fuehrender Null"] = "ANGENOMMEN"
    except ValueError:
        erg["dieselbe Seriennummer mit fuehrender Null"] = "abgewiesen"
    # Dieselbe Note unter h + N (F4): die Kette kennt N und weist ab.
    try:
        kette.anmelden(ANNA, SATZ, serie_bytes(int.from_bytes(serie, "big") + lab.g.h), 1)
        erg["dieselbe Note unter h + N"] = "ANGENOMMEN"
    except ValueError:
        erg["dieselbe Note unter h + N"] = "abgewiesen"
    try:
        kette.anmelden(ANNA, "unbekannt", serie, 1)
        erg["unbekannter Parametersatz"] = "ANGENOMMEN"
    except ValueError:
        erg["unbekannter Parametersatz"] = "abgewiesen"
    erg["Erhaltung"] = kette.summe() == start
    erg["h"] = lab.g.h
    return erg


def vektor() -> dict:
    """Pruefvektor fuer quanten_note.go (quantenPraegen)."""
    serie = serie_bytes(1234)
    n = 1491
    return {"domaene": DOMAENE, "satz": SATZ, "ordnung": n, "von": ANNA, "serie_hex": serie.hex(),
            "schluessel": serie_schluessel(SATZ, ANNA, serie),
            "serie_max_bytes": SERIE_MAX_BYTES,
            "null_hex": serie_bytes(0).hex(), "null_schluessel": serie_schluessel(SATZ, ANNA, serie_bytes(0)),
            "fremd_von": DIEB, "fremd_schluessel": serie_schluessel(SATZ, DIEB, serie),
            "anderer_satz": "labor-b", "anderer_satz_schluessel": serie_schluessel("labor-b", ANNA, serie),
            "groesste_hex": serie_bytes(n - 1).hex(), "zu_gross_hex": [serie_bytes(n).hex(), serie_bytes(1234 + n).hex()],
            "nicht_kanonisch_hex": ["", "0000", "0004d2", "00ff"]}


def selbsttest() -> None:
    rng = random.Random(9)
    e = lauf(rng)
    assert e["echte Note"] > 0.999, e
    assert e["nicht angemeldete Note"] == 0.0, e
    assert e["Seriennummer doppelt anmelden"] == "abgewiesen", e
    assert e["Note ohne Deckung anmelden"] == "abgewiesen", e
    assert e["dieselbe Seriennummer mit fuehrender Null"] == "abgewiesen", e
    assert e["Vorlaufen blockiert die Anmeldung nicht"] is True, e
    assert e["dieselbe Note unter h + N"] == "abgewiesen", e
    assert e["unbekannter Parametersatz"] == "abgewiesen", e
    t = 6
    assert e["fremde Note unter echter Seriennummer"] < 2 ** -t + 0.03, e
    assert e["Messen und nachbauen"] < 2 ** -t + 1 / e["h"] + 0.03, e
    assert e["glatte Faelschung (Glocke)"] < 0.2, e
    assert e["Erhaltung"] is True, e
    assert e["Lightning: Versuche bis zur gleichen Seriennummer"] >= 0
    # Kettenregeln einzeln
    k = Kette({ANNA: 10}, {SATZ: 1491})
    v = vektor()
    assert v["ordnung"] == Labor(random.Random(0)).g.h, "Vektor-Ordnung passt nicht zur Labor-Gruppe"
    for x in v["zu_gross_hex"]:
        try:
            k.anmelden(ANNA, SATZ, bytes.fromhex(x), 1)
            raise AssertionError("h >= N angenommen")
        except ValueError:
            pass
    k.anmelden(ANNA, SATZ, bytes.fromhex(v["groesste_hex"]), 1)
    for schlecht_satz in ("", "A", "a|b", "x" * 33):
        assert not satz_gueltig(schlecht_satz)
    zu_lang = b"\x01" * (SERIE_MAX_BYTES + 1)
    for schlecht in [bytes.fromhex(x) for x in vektor()["nicht_kanonisch_hex"]] + [zu_lang]:
        try:
            k.anmelden(ANNA, SATZ, schlecht, 1)
            raise AssertionError("falsche Form angenommen")
        except ValueError:
            pass
        assert k.wert(SATZ, ANNA, schlecht) == 0
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
