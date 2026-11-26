#!/usr/bin/env python3
"""Weg B, Pfaender (P1): Spezifikation mit Selbsttest.

Forschung, kein Produkt. Keine Abhaengigkeiten. Einordnung: DEZENTRAL.md,
"Entscheidung Beweisweg und Plan Pfaender" (Betreiber, 25.11.2026).

ENTSCHEIDUNG DES BETREIBERS
  Die Inhaberin haftet mit einem Pfand bei der Bindung, und jede Geloste
  verliert VERFALL_PROZENT ihres Pfands, wenn eine Faelschung nachgewiesen ist.
  Ehrliche verlieren wenig, eine Absprache lohnt sich trotzdem nicht.

WAS BEWEISBAR IST
  Token K endet mit dem Urteil "eingeloest" an Stelle A. Danach legt die
  ANDERE festgelegte Stelle B eine Vorlage zu K vor, die die Inhaberin
  unterschrieben hat und die B's geheime Stichprobe besteht (Pruefung wie
  Anmelden/Offenlegen, gegen die Wurzel auf der Kette). Beides zugleich geht
  ohne Kopie nicht: mindestens t Geloste haben ihr Wissen geteilt. WELCHE, ist
  nicht beweisbar -- darum verlieren alle v Gelosten einen kleinen Anteil und
  die Inhaberin (die beide Vorlagen unterschrieben hat) ihr ganzes Pfand.

REGELN (jeder Knoten prueft sie selbst)
  R-P1  Melden kostet PRAEP_PFAND (10 AEQ) von der Wallet in den Pfandtopf.
        Ohne Deckung: abgewiesen. Eintraege vor dem Stichtag tragen kein
        Pfand (nicht rueckwirkend).
  R-P2  Das Pfand gehoert dem MENSCHEN, nicht der Wallet: nach einem Umzug
        geht jede Rueckgabe an die Wallet, die der Mensch DANN hat, nie an die
        alte.
  R-P3  Zurueck frueestens PRAEP_HALTEZEIT nach der Abmeldung UND erst, wenn
        jeder Token, in dessen Los die Praeparatorin stand, geendet hat und
        seine NACHWEIS_FRIST verstrichen ist (sonst meldete sich ab, wer gerade
        mitgefaelscht hat, und holte das Pfand vor dem Nachweis). Ausloesen
        darf jeder; das Geld geht immer an die Wallet des Menschen.
  R-P4  Die Inhaberin hinterlegt bei der Bindung INHABERIN_PFAND(D). Zurueck
        an die Wallet der Inhaberin, wenn der Token geendet hat, seine Frist
        verstrichen ist und kein Nachweis kam.
  R-P5  Nachweis (neue Art quanten_w_s_faelschung): nur Stelle B (die andere
        als die, an der eingeloest wurde), nur innerhalb der Frist, nur einmal
        je K, nur mit gueltigem Beweis. Folge: das Pfand der Inhaberin an B;
        je Geloster VERFALL_PROZENT ihres Pfands an B.
  R-P6  Geld bleibt erhalten: Summe aller Konten plus Pfandtopf ist vor und
        nach jedem Schritt gleich.

OFFEN (Betreiber): INHABERIN_PFAND (Vorschlag: die Deckung D -- genau der
Schaden der Stelle B) und NACHWEIS_FRIST (Vorschlag: 7 Tage). Beide hier als
Parameter; der Selbsttest prueft die Regeln fuer jeden Wert.

Aufruf:  python3 -I s_pfaender.py --selbsttest
"""
from __future__ import annotations

import sys

MIKRO = 1_000_000
PRAEP_PFAND = 10 * MIKRO                 # 10 AEQ je Praeparatorin (Betreiber, 22.11.)
PRAEP_HALTEZEIT = 30 * 86400             # zurueck frueestens 30 Tage nach der Abmeldung
VERFALL_PROZENT = 10                     # je Geloster bei nachgewiesener Faelschung (Betreiber, 25.11.)
NACHWEIS_FRIST = 7 * 86400               # VORSCHLAG, Betreiber offen
TOPF = "pfandtopf"
OFFEN = 1 << 62                          # "noch nicht geendet"


def inhaberin_pfand(deckung: int) -> int:
    """VORSCHLAG, Betreiber offen: die Deckung D (Schaden der Stelle B)."""
    return deckung


class Abgewiesen(Exception):
    pass


class Kette:
    """Konten, Menschen (mit aktueller Wallet), Praeparatorinnen mit Pfand,
    Token mit Los, Stellen, Pfand der Inhaberin und Ende."""

    def __init__(self, guthaben: dict[str, int], menschen: dict[str, str]):
        self.konto = dict(guthaben)
        self.konto.setdefault(TOPF, 0)
        self.wallet_von = dict(menschen)            # Mensch -> aktuelle Wallet
        self.praep: dict[str, dict] = {}            # Mensch -> Eintrag
        self.token: dict[str, dict] = {}

    # -- Hilfen
    def summe(self) -> int:
        return sum(self.konto.values())

    def _ueberweisen(self, von: str, an: str, betrag: int) -> None:
        if betrag < 0 or self.konto.get(von, 0) < betrag:
            raise Abgewiesen(f"Deckung: {von} hat weniger als {betrag}")
        self.konto[von] -= betrag
        self.konto[an] = self.konto.get(an, 0) + betrag

    def _mensch(self, wallet: str) -> str:
        for m, w in self.wallet_von.items():
            if w == wallet:
                return m
        raise Abgewiesen("kein registrierter Mensch")

    # -- R-P1, R-P2
    def melden(self, wallet: str, zeit: int) -> None:
        m = self._mensch(wallet)
        e = self.praep.get(m)
        if e and e["pfand"] > 0:
            raise Abgewiesen("schon gemeldet (Pfand liegt noch)")
        self._ueberweisen(wallet, TOPF, PRAEP_PFAND)
        self.praep[m] = {"pfand": PRAEP_PFAND, "gemeldet": zeit, "abgemeldet": None}

    def abmelden(self, wallet: str, zeit: int) -> None:
        e = self.praep.get(self._mensch(wallet))
        if not e or e["abgemeldet"] is not None:
            raise Abgewiesen("nicht gemeldet")
        e["abgemeldet"] = zeit

    def umzug(self, mensch: str, neu: str) -> None:
        self.wallet_von[mensch] = neu
        self.konto.setdefault(neu, 0)

    def _gebunden_bis(self, mensch: str) -> int:
        """Bis wann das Pfand an Token gebunden ist (R-P3)."""
        bis = 0
        for t in self.token.values():
            if mensch in t["los"]:
                bis = max(bis, OFFEN if t["ende"] is None else t["ende"] + NACHWEIS_FRIST)
        return bis

    def praep_zurueck(self, mensch: str, zeit: int) -> int:
        e = self.praep.get(mensch)
        if not e or e["pfand"] <= 0:
            raise Abgewiesen("kein Pfand")
        if e["abgemeldet"] is None:
            raise Abgewiesen("noch gemeldet")
        if zeit < e["abgemeldet"] + PRAEP_HALTEZEIT:
            raise Abgewiesen("Haltezeit nicht vorbei")
        if zeit <= self._gebunden_bis(mensch):
            raise Abgewiesen("an einen Token gebunden (laeuft oder Frist offen)")
        betrag = e["pfand"]
        e["pfand"] = 0
        self._ueberweisen(TOPF, self.wallet_von[mensch], betrag)
        return betrag

    # -- Token: Ausgabe, Bindung (R-P4), Ende
    def ausgeben(self, K: str, los: list[str], deckung: int) -> None:
        if K in self.token:
            raise Abgewiesen("K schon ausgegeben")
        for m in los:
            e = self.praep.get(m)
            if not e or e["pfand"] <= 0 or e["abgemeldet"] is not None:
                raise Abgewiesen(f"{m} ist keine gemeldete Praeparatorin mit Pfand")
        self.token[K] = {"los": list(los), "deckung": deckung, "inhaberin": None, "pfand": 0,
                         "stellen": None, "ende": None, "eingeloest_bei": None, "nachweis": False}

    def binden(self, K: str, inhaberin_mensch: str, stellen: tuple[str, str]) -> None:
        t = self.token[K]
        if t["inhaberin"] is not None:
            raise Abgewiesen("schon gebunden")
        if stellen[0] == stellen[1]:
            raise Abgewiesen("zwei verschiedene Stellen")
        p = inhaberin_pfand(t["deckung"])
        self._ueberweisen(self.wallet_von[inhaberin_mensch], TOPF, p)
        t.update(inhaberin=inhaberin_mensch, pfand=p, stellen=stellen)

    def einloesen(self, K: str, stelle: str, zeit: int) -> None:
        t = self.token[K]
        if t["ende"] is not None:
            raise Abgewiesen("Token beendet")
        if t["stellen"] is None or stelle not in t["stellen"]:
            raise Abgewiesen("keine festgelegte Stelle")
        t.update(ende=zeit, eingeloest_bei=stelle)

    def inhaberin_zurueck(self, K: str, zeit: int) -> int:
        t = self.token[K]
        if t["pfand"] <= 0:
            raise Abgewiesen("kein Pfand der Inhaberin")
        if t["ende"] is None or zeit <= t["ende"] + NACHWEIS_FRIST:
            raise Abgewiesen("Token laeuft oder Frist fuer einen Nachweis offen")
        betrag = t["pfand"]
        t["pfand"] = 0
        self._ueberweisen(TOPF, self.wallet_von[t["inhaberin"]], betrag)
        return betrag

    # -- R-P5 Nachweis
    def faelschung(self, K: str, stelle: str, beweis_gueltig: bool, zeit: int) -> int:
        """beweis_gueltig steht fuer die Pruefung im Knoten: Vorlage zu K von
        der Inhaberin unterschrieben, besteht die geheime Stichprobe der Stelle
        (Offenlegung in Teilen, Pruefsummen gegen die Wurzel). Gibt, was B bekommt."""
        t = self.token.get(K)
        if t is None or t["ende"] is None or t["eingeloest_bei"] is None:
            raise Abgewiesen("kein eingeloester Token")
        if t["nachweis"]:
            raise Abgewiesen("schon nachgewiesen")
        if stelle not in t["stellen"] or stelle == t["eingeloest_bei"]:
            raise Abgewiesen("nur die andere festgelegte Stelle")
        if zeit > t["ende"] + NACHWEIS_FRIST:
            raise Abgewiesen("Frist fuer den Nachweis vorbei")
        if not beweis_gueltig:
            raise Abgewiesen("Beweis ungueltig")
        t["nachweis"] = True
        an_b = t["pfand"]
        t["pfand"] = 0
        for m in t["los"]:
            e = self.praep[m]
            anteil = e["pfand"] * VERFALL_PROZENT // 100
            e["pfand"] -= anteil
            an_b += anteil
        self._ueberweisen(TOPF, stelle, an_b)
        return an_b


# ---------------------------------------------------------------- Selbsttest
def selbsttest() -> bool:
    fehler: list[str] = []

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    def abgewiesen(f, text):
        try:
            f()
        except Abgewiesen:
            return
        fehler.append("nicht abgewiesen: " + text)

    T = 1_800_000_000
    TAG = 86400
    D = 50 * MIKRO
    v, t = 7, 4

    def welt() -> Kette:
        menschen = {f"m{i}": f"0x{i:040x}" for i in range(1, 12)}
        guthaben = {w: 1000 * MIKRO for w in menschen.values()}
        guthaben["arm"] = 5 * MIKRO
        menschen["arm-mensch"] = "arm"
        guthaben["stelleA"] = 0
        guthaben["stelleB"] = 0
        k = Kette(guthaben, menschen)
        for i in range(1, v + 1):
            k.melden(f"0x{i:040x}", T)
        return k

    los = [f"m{i}" for i in range(1, v + 1)]

    # R-P1: Melden kostet 10 AEQ; ohne Deckung abgewiesen; nicht doppelt.
    k = welt()
    s0 = k.summe()
    soll(k.konto[TOPF] == v * PRAEP_PFAND, "Pfand beim Melden nicht im Topf")
    abgewiesen(lambda: k.melden("arm", T), "Melden ohne Deckung")
    abgewiesen(lambda: k.melden(f"0x{1:040x}", T), "doppelt gemeldet")
    abgewiesen(lambda: k.melden("0x" + "e" * 40, T), "Melden ohne Menschen")

    # R-P3: zu frueh zurueck; nach der Haltezeit an die Wallet; nicht zweimal.
    k.melden(f"0x{8:040x}", T)
    k.abmelden(f"0x{8:040x}", T + 10)
    abgewiesen(lambda: k.praep_zurueck("m8", T + 10 + PRAEP_HALTEZEIT - 1), "Rueckgabe vor der Haltezeit")
    vorher = k.konto[f"0x{8:040x}"]
    soll(k.praep_zurueck("m8", T + 10 + PRAEP_HALTEZEIT) == PRAEP_PFAND, "Rueckgabe nach der Haltezeit")
    soll(k.konto[f"0x{8:040x}"] == vorher + PRAEP_PFAND, "Rueckgabe nicht an die Wallet")
    abgewiesen(lambda: k.praep_zurueck("m8", T + 10 + PRAEP_HALTEZEIT + 1), "Rueckgabe zweimal")
    abgewiesen(lambda: k.praep_zurueck("m1", T + 400 * TAG), "Rueckgabe ohne Abmeldung")
    soll(k.summe() == s0, "Geld nicht erhalten (Rueckgabe)")

    # R-P2: nach einem Umzug geht die Rueckgabe an die NEUE Wallet, nie an die alte.
    k = welt()
    k.abmelden(f"0x{2:040x}", T)
    alt = f"0x{2:040x}"
    k.umzug("m2", "0xneu2")
    vorher_alt = k.konto[alt]
    k.praep_zurueck("m2", T + PRAEP_HALTEZEIT)
    soll(k.konto["0xneu2"] == PRAEP_PFAND and k.konto[alt] == vorher_alt, "Umzug: Rueckgabe nicht an die neue Wallet")
    abgewiesen(lambda: k.abmelden(alt, T), "die alte Wallet handelt nach dem Umzug noch fuer den Menschen")

    # R-P3: Abmelden direkt nach einer Faelschung, Pfand holen, bevor nachgewiesen wird -- gesperrt.
    k = welt()
    s0 = k.summe()
    k.ausgeben("K1", los, D)
    abgewiesen(lambda: k.ausgeben("K1", los, D), "K doppelt ausgegeben")
    abgewiesen(lambda: k.ausgeben("K2", los[:-1] + ["m11"], D), "Ausgabe mit Ungemeldeter im Los")
    abgewiesen(lambda: k.binden("K1", "m9", ("stelleA", "stelleA")), "zweimal dieselbe Stelle (dann gaebe es kein B fuer den Nachweis)")
    k.binden("K1", "m9", ("stelleA", "stelleB"))
    soll(k.konto[TOPF] == v * PRAEP_PFAND + inhaberin_pfand(D), "Pfand der Inhaberin nicht im Topf")
    abgewiesen(lambda: k.binden("K1", "m9", ("stelleA", "stelleB")), "zweimal gebunden")
    for m in los:
        k.abmelden(k.wallet_von[m], T + 1)
    abgewiesen(lambda: k.praep_zurueck("m1", T + 1 + PRAEP_HALTEZEIT), "Rueckgabe, solange der Token laeuft")
    k.einloesen("K1", "stelleA", T + 2 * TAG)
    abgewiesen(lambda: k.einloesen("K1", "stelleB", T + 2 * TAG + 5), "zweimal eingeloest")
    abgewiesen(lambda: k.inhaberin_zurueck("K1", T + 2 * TAG + NACHWEIS_FRIST), "Inhaberin zurueck in der Frist")

    # R-P5: Nachweis -- nur B, nur gueltig, nur in der Frist, nur einmal.
    abgewiesen(lambda: k.faelschung("K1", "stelleA", True, T + 3 * TAG), "Nachweis von der Stelle, an der eingeloest wurde")
    abgewiesen(lambda: k.faelschung("K1", "fremd", True, T + 3 * TAG), "Nachweis von einer fremden Stelle")
    abgewiesen(lambda: k.faelschung("K1", "stelleB", False, T + 3 * TAG), "Nachweis mit ungueltigem Beweis")
    abgewiesen(lambda: k.faelschung("K1", "stelleB", True, T + 2 * TAG + NACHWEIS_FRIST + 1), "Nachweis nach der Frist")
    an_b = k.faelschung("K1", "stelleB", True, T + 3 * TAG)
    erwartet = inhaberin_pfand(D) + v * (PRAEP_PFAND * VERFALL_PROZENT // 100)
    soll(an_b == erwartet and k.konto["stelleB"] == erwartet, f"Nachweis: B bekam {an_b}, erwartet {erwartet}")
    soll(all(k.praep[m]["pfand"] == PRAEP_PFAND * (100 - VERFALL_PROZENT) // 100 for m in los), "Gelosten nicht je 10 % verloren")
    abgewiesen(lambda: k.faelschung("K1", "stelleB", True, T + 3 * TAG + 1), "Nachweis zweimal")
    abgewiesen(lambda: k.inhaberin_zurueck("K1", T + 400 * TAG), "Inhaberin zurueck nach dem Nachweis")
    # Nach der Frist bekommen die Gelosten den Rest zurueck.
    soll(k.praep_zurueck("m1", T + 2 * TAG + NACHWEIS_FRIST + PRAEP_HALTEZEIT) == PRAEP_PFAND * 9 // 10, "Rest nach dem Verfall")
    soll(k.summe() == s0, "Geld nicht erhalten (Nachweis)")
    soll(k.konto["stelleB"] >= D, "Stelle B ist nicht schadlos (Pfand der Inhaberin < D)")

    # Ohne Nachweis: Inhaberin nach der Frist zurueck, Gelosten nach Haltezeit und Frist.
    k = welt()
    s0 = k.summe()
    k.ausgeben("K3", los, D)
    k.binden("K3", "m10", ("stelleA", "stelleB"))
    k.einloesen("K3", "stelleB", T + TAG)
    abgewiesen(lambda: k.inhaberin_zurueck("K3", T + TAG + NACHWEIS_FRIST), "Inhaberin zurueck genau am Fristende")
    soll(k.inhaberin_zurueck("K3", T + TAG + NACHWEIS_FRIST + 1) == inhaberin_pfand(D), "Inhaberin zurueck nach der Frist")
    abgewiesen(lambda: k.faelschung("K3", "stelleA", True, T + TAG + NACHWEIS_FRIST + 2), "Nachweis nach der Frist (andere Stelle)")
    k.abmelden(k.wallet_von["m3"], T + 2)
    soll(k.praep_zurueck("m3", T + 2 + PRAEP_HALTEZEIT) == PRAEP_PFAND, "Gelost, ohne Nachweis: volles Pfand zurueck")
    soll(k.summe() == s0, "Geld nicht erhalten (ohne Nachweis)")

    # Wirtschaft: Inhaberin und t Verschworene gewinnen mit einer Faelschung nichts.
    # Gewinn der Faelschung: die zweite Einloesung (D); Verlust: Pfand der Inhaberin + t * 10 %.
    gewinn = D - inhaberin_pfand(D) - t * (PRAEP_PFAND * VERFALL_PROZENT // 100)
    soll(gewinn < 0, f"Faelschung lohnt sich ({gewinn})")
    # Ehrliche verlieren hoechstens 10 % ihres Pfands je nachgewiesener Faelschung.
    soll(PRAEP_PFAND * VERFALL_PROZENT // 100 == MIKRO, "Verlust einer Ehrlichen nicht 1 AEQ")

    for f in fehler:
        print("FEHLER:", f)
    print("Selbsttest Pfaender:", "gut" if not fehler else f"{len(fehler)} Fehler")
    return not fehler


if __name__ == "__main__":
    if "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    print(__doc__)
