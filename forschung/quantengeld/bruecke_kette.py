#!/usr/bin/env python3
"""Kettenseite der Bruecke AEQ <-> Quanten-Note: ausfuehrbare Spezifikation.

Forschung, kein Produktcode (BRUECKE.md, BAUVORGABE.md §4). Keine
Abhaengigkeiten. Legt fest, was ein Knoten spaeter Byte fuer Byte pruefen
muss -- unabhaengig davon, welches Quantenverfahren die Note traegt.

DER EINLOESE-NACHWEIS IST EINE EINMAL-SIGNATUR. Die Quanten-Note ist ein
Signierzustand (one-shot signature, OSS); ihre Seriennummer ist der
oeffentliche Schluessel. Einloesen heisst: GENAU EINE Nachricht unterschreiben
-- dabei wird der Zustand verbraucht. Die Kette prueft die Unterschrift
klassisch. Fuer die Spezifikation steht hier eine Lamport-Einmal-Signatur
(SHA-256) an ihrer Stelle: dieselbe Pruefung auf der Kette, hashbasiert und
damit selbst quantensicher. Der Unterschied liegt nur auf der Quantenseite --
einen Lamport-Schluessel kann man kopieren, einen OSS-Zustand nicht; deshalb
erzwingt die KETTE "hoechstens eine Einloesung je Seriennummer" selbst.

NACHRICHTEN (Byte fuer Byte, wie spaeter im Knoten):
  Seriennummer  s = SHA-256(oeffentlicher Schluessel)          (32 Byte, hex)
  Sperren       "aequitas-quanten-sperren-v1|chain:1926|<s>|<wei>|<frist>|<rueckholbar>"
                unterschrieben vom Sperrenden (hier nur vermerkt; im Knoten ML-DSA, Stufe 3)
  Einloesen     "aequitas-quanten-einloesen-v1|chain:1926|<s>|<ziel>|<wei>"
                unterschrieben mit der Note (Einmal-Signatur)

REGELN, die jeder Knoten selbst prueft (Selbsttest: je ein Angriff):
  1. Erhaltung: Konten + gesperrte Betraege bleiben gleich; es entsteht kein AEQ.
  2. Sperren nur mit Deckung, Betrag > 0, Seriennummer neu (nie zuvor gesperrt
     ODER eingeloest -- eine Seriennummer lebt genau einmal).
  3. Einloesen nur einer gesperrten Seriennummer, nur mit gueltiger
     Einmal-Signatur ueber GENAU diese Nachricht (Ziel und Betrag gebunden).
  4. Hoechstens eine Einloesung je Seriennummer -- auch wenn jemand zwei
     verschiedene gueltige Unterschriften vorlegt (bei Lamport moeglich, bei
     OSS nicht: die Kette verlaesst sich nicht darauf).
  5. Rueckholen nur durch den Sperrenden, nur wenn beim Sperren erlaubt, nur
     nach Ablauf der Frist; danach ist die Note wertlos.
  6. Grenzen: Unterschrift genau 2 * 256 * 32 Byte, Schluessel 2 * 256 * 32
     Byte, Register hoechstens REGISTER_MAX Eintraege.

Aufruf: python3 -I bruecke_kette.py [--selbsttest]
"""
from __future__ import annotations

import hashlib
import secrets
import sys

KETTE = 1926
REGISTER_MAX = 1_000_000
BITS = 256  # Lamport ueber SHA-256 der Nachricht


def _h(b: bytes) -> bytes:
    return hashlib.sha256(b).digest()


# ---- Einmal-Signatur (Lamport, SHA-256) als Platzhalter fuer OSS ----

class EinmalSchluessel:
    """Steht fuer den Quanten-Signierzustand der Note."""

    def __init__(self):
        self._geheim = [(secrets.token_bytes(32), secrets.token_bytes(32)) for _ in range(BITS)]
        self.oeffentlich = b"".join(_h(a) + _h(b) for a, b in self._geheim)

    def unterschreiben(self, nachricht: bytes) -> bytes:
        d = _h(nachricht)
        teile = []
        for i in range(BITS):
            bit = (d[i // 8] >> (7 - i % 8)) & 1
            teile.append(self._geheim[i][bit])
        return b"".join(teile)


def pruefe_einmal(oeffentlich: bytes, nachricht: bytes, sig: bytes) -> bool:
    if len(oeffentlich) != 2 * BITS * 32 or len(sig) != BITS * 32:
        return False
    d = _h(nachricht)
    for i in range(BITS):
        bit = (d[i // 8] >> (7 - i % 8)) & 1
        erwartet = oeffentlich[(2 * i + bit) * 32:(2 * i + bit + 1) * 32]
        if _h(sig[i * 32:(i + 1) * 32]) != erwartet:
            return False
    return True


def seriennummer(oeffentlich: bytes) -> str:
    return _h(oeffentlich).hex()


def einloese_nachricht(s: str, ziel: str, wei: int) -> bytes:
    return f"aequitas-quanten-einloesen-v1|chain:{KETTE}|{s}|{ziel}|{wei}".encode()


# ---- Die Kette ----

class Abgelehnt(Exception):
    pass


class Kette:
    def __init__(self, konten: dict[str, int]):
        self.konten = dict(konten)
        # s -> (wei, sperrender, ablauf, rueckholbar, oeffentlicher Schluessel)
        self.gesperrt: dict[str, tuple[int, str, int, bool, bytes]] = {}
        self.verbraucht: set[str] = set()
        self.zeit = 0

    def summe(self) -> int:
        return sum(self.konten.values()) + sum(e[0] for e in self.gesperrt.values())

    def sperren(self, wer: str, wei: int, oeffentlich: bytes, frist: int, rueckholbar: bool) -> str:
        if not isinstance(wei, int) or wei <= 0 or self.konten.get(wer, 0) < wei:
            raise Abgelehnt("keine Deckung oder Betrag ungueltig")
        if len(oeffentlich) != 2 * BITS * 32:
            raise Abgelehnt("Schluessel in falscher Form")
        if not isinstance(frist, int) or frist <= 0:
            raise Abgelehnt("Frist ungueltig")
        s = seriennummer(oeffentlich)
        if s in self.gesperrt or s in self.verbraucht:
            raise Abgelehnt("Seriennummer nicht neu")
        if len(self.gesperrt) + len(self.verbraucht) >= REGISTER_MAX:
            raise Abgelehnt("Register voll")
        self.konten[wer] -= wei
        self.gesperrt[s] = (wei, wer, self.zeit + frist, rueckholbar, oeffentlich)
        return s

    def einloesen(self, s: str, ziel: str, wei: int, sig: bytes) -> None:
        if s in self.verbraucht:
            raise Abgelehnt("schon eingeloest oder zurueckgeholt")
        if s not in self.gesperrt:
            raise Abgelehnt("unbekannte Seriennummer")
        betrag, _, _, _, oeffentlich = self.gesperrt[s]
        if wei != betrag:
            raise Abgelehnt("Betrag passt nicht")
        if not pruefe_einmal(oeffentlich, einloese_nachricht(s, ziel, wei), sig):
            raise Abgelehnt("Einmal-Signatur ungueltig")
        del self.gesperrt[s]
        self.verbraucht.add(s)
        self.konten[ziel] = self.konten.get(ziel, 0) + betrag

    def zurueckholen(self, wer: str, s: str) -> None:
        if s not in self.gesperrt:
            raise Abgelehnt("nicht gesperrt")
        betrag, sperrender, ablauf, rueckholbar, _ = self.gesperrt[s]
        if wer != sperrender or not rueckholbar or self.zeit < ablauf:
            raise Abgelehnt("Rueckholung nicht erlaubt")
        del self.gesperrt[s]
        self.verbraucht.add(s)
        self.konten[wer] += betrag


def selbsttest() -> None:
    fehler = []

    def angriff(name, fn):
        try:
            fn()
            fehler.append(name)
        except Abgelehnt:
            pass

    k = Kette({"anna": 1000, "ben": 0, "dieb": 0})
    start = k.summe()

    # Gutfall: sperren, offline weitergeben (die Note), einloesen an ben.
    note = EinmalSchluessel()
    s = k.sperren("anna", 300, note.oeffentlich, frist=100, rueckholbar=False)
    sig = note.unterschreiben(einloese_nachricht(s, "ben", 300))
    k.einloesen(s, "ben", 300, sig)
    assert k.konten["ben"] == 300 and k.summe() == start

    # 3./4. Wiederholung, Umlenken, zweite gueltige Unterschrift (Lamport erlaubt sie).
    angriff("Wiederholung derselben Einloesung", lambda: k.einloesen(s, "ben", 300, sig))
    sig2 = note.unterschreiben(einloese_nachricht(s, "dieb", 300))
    angriff("zweite gueltige Unterschrift an anderes Ziel", lambda: k.einloesen(s, "dieb", 300, sig2))

    note2 = EinmalSchluessel()
    s2 = k.sperren("anna", 200, note2.oeffentlich, frist=100, rueckholbar=True)
    ok = note2.unterschreiben(einloese_nachricht(s2, "ben", 200))
    angriff("abgefangene Unterschrift umlenken", lambda: k.einloesen(s2, "dieb", 200, ok))
    angriff("Betrag erhoehen", lambda: k.einloesen(s2, "ben", 201, ok))
    fremd = EinmalSchluessel()
    angriff("Unterschrift mit fremdem Schluessel",
            lambda: k.einloesen(s2, "dieb", 200, fremd.unterschreiben(einloese_nachricht(s2, "dieb", 200))))
    kaputt = bytearray(ok)
    kaputt[5] ^= 1
    angriff("ein Bit der Unterschrift falsch", lambda: k.einloesen(s2, "ben", 200, bytes(kaputt)))
    angriff("Unterschrift abgeschnitten", lambda: k.einloesen(s2, "ben", 200, ok[:-1]))
    angriff("unbekannte Seriennummer", lambda: k.einloesen("00" * 32, "dieb", 200, ok))
    # 2. Sperren.
    angriff("Sperren ohne Deckung", lambda: k.sperren("dieb", 1, EinmalSchluessel().oeffentlich, 10, False))
    angriff("Sperren mit Betrag 0", lambda: k.sperren("anna", 0, EinmalSchluessel().oeffentlich, 10, False))
    angriff("Seriennummer erneut sperren", lambda: k.sperren("anna", 1, note2.oeffentlich, 10, False))
    angriff("verbrauchte Seriennummer erneut sperren", lambda: k.sperren("anna", 1, note.oeffentlich, 10, False))
    angriff("Schluessel in falscher Form", lambda: k.sperren("anna", 1, b"x" * 10, 10, False))
    # 5. Rueckholen.
    angriff("Rueckholen vor Fristablauf", lambda: k.zurueckholen("anna", s2))
    k.zeit = 200
    angriff("Rueckholen durch Fremde", lambda: k.zurueckholen("dieb", s2))
    k.zurueckholen("anna", s2)
    angriff("Note nach Rueckholung einloesen", lambda: k.einloesen(s2, "ben", 200, ok))
    note3 = EinmalSchluessel()
    s3 = k.sperren("anna", 100, note3.oeffentlich, frist=1, rueckholbar=False)
    k.zeit = 1000
    angriff("Inhaberpapier zurueckholen", lambda: k.zurueckholen("anna", s3))

    # 1. Erhaltung nach allem.
    assert k.summe() == start, (k.summe(), start)
    # Lamport-Pruefung selbst: gueltig/ungueltig.
    t = EinmalSchluessel()
    m = b"test"
    assert pruefe_einmal(t.oeffentlich, m, t.unterschreiben(m))
    assert not pruefe_einmal(t.oeffentlich, b"test2", t.unterschreiben(m))

    # Gegenprobe: ohne Zielbindung ginge der Umlenk-Angriff durch.
    k2 = Kette({"anna": 10})
    n4 = EinmalSchluessel()
    s4 = k2.sperren("anna", 10, n4.oeffentlich, 5, False)
    sig4 = n4.unterschreiben(einloese_nachricht(s4, "ben", 10))
    assert not pruefe_einmal(n4.oeffentlich, einloese_nachricht(s4, "dieb", 10), sig4)

    if fehler:
        print("ANGRIFFE DURCHGEKOMMEN:", fehler)
        sys.exit(1)

    # Gegenprobe mit eingebautem Fehler: ohne die Signaturpruefung bzw. ohne
    # "hoechstens einmal" MUSS ein Angriff durchgehen -- sonst waeren die Tests
    # zahnlos.
    class OhneSignatur(Kette):
        def einloesen(self, s, ziel, wei, sig):
            betrag = self.gesperrt[s][0]
            del self.gesperrt[s]
            self.verbraucht.add(s)
            self.konten[ziel] = self.konten.get(ziel, 0) + betrag

    class OhneEinmal(Kette):
        def einloesen(self, s, ziel, wei, sig):
            betrag, _, _, _, oeff = self.gesperrt[s]
            if not pruefe_einmal(oeff, einloese_nachricht(s, ziel, wei), sig):
                raise Abgelehnt("Einmal-Signatur ungueltig")
            self.konten[ziel] = self.konten.get(ziel, 0) + betrag  # bleibt gesperrt!

    # a) Ohne Signaturpruefung loest ein Dieb mit Unsinn ein.
    kk = OhneSignatur({"anna": 100})
    n = EinmalSchluessel()
    ss = kk.sperren("anna", 100, n.oeffentlich, 10, False)
    kk.einloesen(ss, "dieb", 100, b"unsinn")
    assert kk.konten.get("dieb") == 100, "Gegenprobe ohne Signaturpruefung: Test zahnlos"
    # b) Ohne Einmal-Regel entsteht Geld aus dem Nichts (zwei Einloesungen).
    kk = OhneEinmal({"anna": 100})
    n = EinmalSchluessel()
    ss = kk.sperren("anna", 100, n.oeffentlich, 10, False)
    kk.einloesen(ss, "ben", 100, n.unterschreiben(einloese_nachricht(ss, "ben", 100)))
    kk.einloesen(ss, "dieb", 100, n.unterschreiben(einloese_nachricht(ss, "dieb", 100)))
    assert kk.summe() > 100, "Gegenprobe ohne Einmal-Regel: Test zahnlos"
    print("selbsttest ok (Erhaltung, 17 Angriffe abgewiesen, Gegenprobe greift)")


if __name__ == "__main__":
    if "--selbsttest" in sys.argv:
        selbsttest()
    else:
        selbsttest()
