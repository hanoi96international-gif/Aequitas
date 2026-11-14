#!/usr/bin/env python3
"""Brücke AEQ <-> Quanten-Note: Protokollmodell mit idealem Quantum Lightning.

Forschung, kein Produktcode. Grundlage: Coladangelo & Sattath, "A Quantum Money
Solution to the Blockchain Scalability Problem", Quantum 4, 297 (2020). Dort:
Quanten-Banknoten aus *Quantum Lightning* (selbst die ehrliche Herstellung kann
keine zwei gültigen Noten mit derselben Seriennummer erzeugen), eine klassische
Kette führt die gültigen Seriennummern, und "bolt-to-signature" macht aus einer
Note wieder einen klassischen Anspruch.

Dieses Modell prüft NUR die Protokolllogik auf der Kette. Den Quantenteil
ersetzt eine ideale Funktionalität `IdealesLightning`, die genau die Eigenschaften
hat, die ein echtes Verfahren bräuchte (und die heute niemand beweisen kann):
  - erzeugen() liefert eine Note mit zufälliger, nie wiederholter Seriennummer;
  - eine Note lässt sich nicht kopieren (das Objekt ist einmalig, wer sie
    weitergibt, hat sie nicht mehr);
  - einloesen() verbraucht die Note und liefert einen Einlöse-Nachweis, der nur
    für genau eine Zieladresse gilt.

Geprüfte Eigenschaften (Selbsttest), je mit einem Angriff:
  1. Erhaltung: AEQ auf Konten + AEQ in gesperrten Noten = konstant.
  2. Keine doppelte Einlösung derselben Seriennummer.
  3. Keine Einlösung einer Seriennummer, die die Kette nicht kennt.
  4. Ein abgefangener Einlöse-Nachweis lässt sich nicht auf eine andere Adresse
     umlenken (er ist an das Ziel gebunden).
  5. Niemand wählt Seriennummern; Sperren ohne Deckung scheitert.
  6. Verlust: Eine Note, die nach Ablauf der Frist nicht eingelöst wurde, kann
     der Sperrende mit seinem Schlüssel zurückholen, aber nur, wenn er beim
     Sperren diese Rückholung erlaubt hat (Abwägung: Inhaberpapier oder
     Rückholbarkeit, siehe BRUECKE.md).

Aufruf:  python3 -I bruecke.py [--selbsttest]
"""
import hashlib
import secrets
import sys


class IdealesLightning:
    """Ideale Quanten-Note: einmalig, nicht kopierbar, Seriennummer zufällig."""

    def __init__(self):
        self._vergeben = set()

    def erzeugen(self, betrag):
        while True:
            seriennummer = secrets.token_hex(16)
            if seriennummer not in self._vergeben:
                self._vergeben.add(seriennummer)
                return Note(seriennummer, betrag)


class Note:
    """Steht für einen Quantenzustand: wer ihn verbraucht, hat ihn nicht mehr."""

    def __init__(self, seriennummer, betrag):
        self.seriennummer = seriennummer
        self.betrag = betrag
        self._verbraucht = False

    def pruefen(self):
        return not self._verbraucht

    def einloesen(self, ziel):
        """bolt-to-signature: verbraucht die Note, Nachweis gilt nur für `ziel`."""
        if self._verbraucht:
            raise ValueError("Note schon verbraucht")
        self._verbraucht = True
        return Nachweis(self.seriennummer, ziel, _bindung(self.seriennummer, ziel))


class Nachweis:
    def __init__(self, seriennummer, ziel, bindung):
        self.seriennummer = seriennummer
        self.ziel = ziel
        self.bindung = bindung


def _bindung(seriennummer, ziel):
    # Steht für die Unterschrift, die nur der Inhaber des Zustands erzeugen kann.
    return hashlib.sha256(f"aeq-bolt|{seriennummer}|{ziel}".encode()).hexdigest()


class Kette:
    """Der klassische Teil auf Aequitas: Konten, gesperrte Seriennummern."""

    def __init__(self, konten, frist):
        self.konten = dict(konten)
        self.gesperrt = {}   # seriennummer -> (betrag, sperrender, ablauf, rueckholbar)
        self.eingeloest = set()
        self.frist = frist
        self.zeit = 0

    def summe(self):
        return sum(self.konten.values()) + sum(b for b, _, _, _ in self.gesperrt.values())

    def sperren(self, wer, betrag, lightning, rueckholbar):
        if betrag <= 0 or self.konten.get(wer, 0) < betrag:
            raise ValueError("keine Deckung")
        note = lightning.erzeugen(betrag)
        if note.seriennummer in self.gesperrt or note.seriennummer in self.eingeloest:
            raise ValueError("Seriennummer schon bekannt")
        self.konten[wer] -= betrag
        self.gesperrt[note.seriennummer] = (betrag, wer, self.zeit + self.frist, rueckholbar)
        return note

    def einloesen(self, nachweis):
        s = nachweis.seriennummer
        if s in self.eingeloest:
            raise ValueError("schon eingelöst")
        if s not in self.gesperrt:
            raise ValueError("unbekannte Seriennummer")
        if nachweis.bindung != _bindung(s, nachweis.ziel):
            raise ValueError("Nachweis passt nicht zum Ziel")
        betrag, _, _, _ = self.gesperrt.pop(s)
        self.eingeloest.add(s)
        self.konten[nachweis.ziel] = self.konten.get(nachweis.ziel, 0) + betrag

    def zurueckholen(self, wer, seriennummer):
        if seriennummer not in self.gesperrt:
            raise ValueError("nicht gesperrt")
        betrag, sperrender, ablauf, rueckholbar = self.gesperrt[seriennummer]
        if wer != sperrender or not rueckholbar or self.zeit < ablauf:
            raise ValueError("Rückholung nicht erlaubt")
        del self.gesperrt[seriennummer]
        self.eingeloest.add(seriennummer)  # eine spätere Einlösung scheitert
        self.konten[wer] += betrag


def erwarte_fehler(fn, name, fehler):
    try:
        fn()
        fehler.append(f"Angriff ging durch: {name}")
    except ValueError:
        pass


def selbsttest():
    fehler = []
    q = IdealesLightning()
    k = Kette({"anna": 1000, "ben": 1000, "mallory": 1000}, frist=10)
    gesamt = k.summe()

    # Gutfall: Anna sperrt 300, gibt die Note offline an Ben, Ben löst ein.
    note = k.sperren("anna", 300, q, rueckholbar=False)
    if not note.pruefen():
        fehler.append("frische Note ungültig")
    weitergegeben = note  # Weitergabe = Übergabe des Zustands
    k.einloesen(weitergegeben.einloesen("ben"))
    if (k.konten["anna"], k.konten["ben"]) != (700, 1300):
        fehler.append(f"Kontostände falsch: {k.konten}")
    if k.summe() != gesamt:
        fehler.append("Erhaltung verletzt (Gutfall)")

    # 2. Doppelt einlösen: der Zustand ist verbraucht ...
    erwarte_fehler(lambda: note.einloesen("mallory"), "Note zweimal verbrauchen", fehler)
    # ... und ein wiederholter Nachweis scheitert auf der Kette.
    n2 = k.sperren("ben", 100, q, rueckholbar=False)
    nachweis = n2.einloesen("ben")
    k.einloesen(nachweis)
    erwarte_fehler(lambda: k.einloesen(nachweis), "Nachweis wiederholen", fehler)

    # 3. Unbekannte Seriennummer.
    falsch = Nachweis("00" * 16, "mallory", _bindung("00" * 16, "mallory"))
    erwarte_fehler(lambda: k.einloesen(falsch), "unbekannte Seriennummer", fehler)

    # 4. Abgefangenen Nachweis auf eigene Adresse umlenken.
    n3 = k.sperren("anna", 50, q, rueckholbar=False)
    echt = n3.einloesen("anna")
    umgelenkt = Nachweis(echt.seriennummer, "mallory", echt.bindung)
    erwarte_fehler(lambda: k.einloesen(umgelenkt), "Nachweis umlenken", fehler)
    k.einloesen(echt)

    # 5. Sperren ohne Deckung, negative Beträge.
    erwarte_fehler(lambda: k.sperren("mallory", 5000, q, rueckholbar=False), "ohne Deckung", fehler)
    erwarte_fehler(lambda: k.sperren("mallory", -1, q, rueckholbar=False), "negativer Betrag", fehler)
    serien = {q.erzeugen(1).seriennummer for _ in range(2000)}
    if len(serien) != 2000:
        fehler.append("Seriennummern wiederholt")

    # 6. Verlust und Rückholung.
    verloren = k.sperren("anna", 100, q, rueckholbar=True)
    erwarte_fehler(lambda: k.zurueckholen("anna", verloren.seriennummer), "Rückholung vor Ablauf", fehler)
    erwarte_fehler(lambda: k.zurueckholen("mallory", verloren.seriennummer), "fremde Rückholung", fehler)
    k.zeit += 11
    k.zurueckholen("anna", verloren.seriennummer)
    # Taucht die verlorene Note doch auf, ist sie wertlos (keine Doppelauszahlung).
    erwarte_fehler(lambda: k.einloesen(verloren.einloesen("mallory")), "nach Rückholung einlösen", fehler)
    inhaber = k.sperren("anna", 10, q, rueckholbar=False)
    k.zeit += 11
    erwarte_fehler(lambda: k.zurueckholen("anna", inhaber.seriennummer), "Inhaberpapier zurückholen", fehler)

    if k.summe() != gesamt:
        fehler.append(f"Erhaltung verletzt: {k.summe()} statt {gesamt}")
    for x in fehler:
        print("FEHLER:", x)
    print("bruecke: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


if __name__ == "__main__":
    sys.exit(0 if selbsttest() else 1)
