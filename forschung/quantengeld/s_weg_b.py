#!/usr/bin/env python3
"""Weg B: Praeparatoren aus dem Menschenregister -- Spezifikation mit Selbsttest.

Forschung, kein Produkt. Keine Abhaengigkeiten. Baut auf s_kette.py (Weg A,
Ausgabe in Abschnitten) auf. Einordnung: DEZENTRAL.md, Abschnitte 5 und 18.

WARUM
  Weg A wirkt erst, wenn mehrere MENSCHEN je einen Validator mit eigenem Server
  und eigenem Konto bei einem Quantenrechner betreiben. Das ist eine hohe
  Huerde. Weg B senkt sie: Jeder registrierte Mensch (ein Mensch, ein Eintrag;
  Biometrie-Register) mit einem Konto bei einem Quantenrechner kann sich als
  PRAEPARATOR melden -- mit dem Handy, ohne Server. Je Token LOST die Kette
  v Praeparatoren aus; jeder praepariert seinen Abschnitt in seinem eigenen
  Auftrag und haelt Anteile der anderen (genau wie ein Validator in Weg A).

WAS DIE KETTE NEU PRUEFT (alles andere bleibt wie Weg A)
  1. Register: nur aktive Menschen, je Mensch hoechstens ein Praeparator,
     gemeldet mindestens MELDE_VORLAUF Bloecke vor der Bestellung (wer sich
     nach der Bestellung meldet, ist nicht im Los).
  2. Los: Die Bestellung wird auf Hoehe h endgueltig. Gelost wird mit dem
     Zufall der Hoehe h + LOS_ABSTAND, den bei der Bestellung niemand kennt.
     Aus dem Abbild des Registers auf Hoehe h nimmt das Los die v Eintraege
     mit dem kleinsten H(Zufall, K, Adresse). Der Ausgeber waehlt also weder
     die Praeparatoren noch kann er durch Probieren vieler K welche erzwingen.
  3. Ausgabe: genau die gelosten v liefern Blaetter und Teil-Hashes; ein
     Blatt eines nicht gelosten Kontos weist die Kette ab.

SICHERHEIT IN ZAHLEN
  Faelschen (an zwei Stellen durchkommen) braucht t Verschworene unter den v
  Gelosten (Weg A, s_kette 5d). Sind b von N Registrierten boese, ist das die
  hypergeometrische Wahrscheinlichkeit P(X >= t), X ~ Hyp(N, b, v)
  (erfassung). Darunter wissen die Verschworenen hoechstens den Anteil
  (t-1)/v -- genau die Annahme der Pruefgroesse k_note.

VERFUEGBARKEIT (Handys sind nicht immer erreichbar)
  Die Annahmestelle laedt ihre Stichprobe nach der Bindung von jedem
  Praeparator ODER aus t Anteilen der anderen (R9). Sind m der v Praeparatoren
  in der Frist erreichbar, fehlt bei m >= t nichts; bei m < t fehlt jeder
  Abschnitt eines nicht Erreichbaren. Also P(einloesbar in der Frist) =
  P(Bin(v, p) >= t) (verfuegbar). Faellt es aus, wartet die Einloesung --
  der Wert geht nicht verloren (die Anmeldung kommt erst nach dem Laden).

QUANTENRECHNER-KONTINGENT
  Das kostenlose Konto bei IBM (Open Plan) hat 10 Minuten Rechenzeit je 28
  Tage. Ein Abschnitt ist ein Auftrag; kontingent() schaetzt, wie viele
  Abschnitte ein Mensch damit praeparieren kann. Die Sekunden je Auftrag sind
  eine SCHAETZUNG und werden im ersten Lauf gemessen.

GRENZEN (ehrlich)
  - Den Zufall des Loses darf kein Einzelner bestimmen. Heute erzeugt ein
    einziger Knoten (C1) die Bloecke; er koennte den Zufall verzerren. Weg B
    braucht darum Zufall, zu dem mehrere beitragen (wie W3: Zusage, dann
    Offenlegung) -- siehe zufall_aus_beitraegen. Wer als Letzter offenlegt,
    kann noch zurueckhalten; das kostet ihn hier nur eine Wiederholung des
    Loses mit neuer Hoehe, die Haeufigkeit dafuer begrenzt die Kette.
  - Der Anbieter des Quantenrechners sieht jeden Auftrag. Nutzen alle IBM,
    kennt IBM den Token (wie Weg A).
  - Die Inhaberin muss dem Praeparator ihr x glauben (keine Quantenleitung);
    Stoerung ist zuordenbar (fehler_je_abschnitt), Faelschen nicht moeglich.

Aufruf:  python3 -I s_weg_b.py --selbsttest
"""
from __future__ import annotations

import hashlib
import importlib.util
import math
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("s_kette", os.path.join(_hier, "s_kette.py"))
sk = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(sk)
wk = sk.wk
Abgewiesen = sk.Abgewiesen

MELDE_VORLAUF = 1000      # Bloecke: so lange vor der Bestellung gemeldet
LOS_ABSTAND = 10          # Bloecke nach der Bestellung: dieser Zufall lost
V_MIN, V_MAX = 4, 16      # so viele Praeparatoren je Token
POOL_MIN_FAKTOR = 3       # das Register muss mindestens 3 v Eintraege haben
DOM_LOS = b"aequitas-quanten-s-weg-b-los-v1"
DOM_ZUFALL = b"aequitas-quanten-s-weg-b-zufall-v1"

IBM_OPEN_SEKUNDEN = 600   # 10 Minuten je 28 Tage (IBM Quantum Open Plan)
SEKUNDEN_JE_AUFTRAG = 10  # SCHAETZUNG, im ersten Lauf zu messen


def h(*teile: bytes) -> bytes:
    m = hashlib.sha256()
    for t in teile:
        m.update(len(t).to_bytes(4, "big"))
        m.update(t)
    return m.digest()


# ---------------------------------------------------------------- Register
class Register:
    """Die Praeparatoren. Jeder Eintrag gehoert genau einem aktiven Menschen
    aus dem Menschenregister; die Meldehoehe zaehlt fuer das Los."""

    def __init__(self, menschen: dict[str, str]):
        # Konto -> Mensch (aus dem Biometrie-Register; hier vorgegeben)
        self.menschen = dict(menschen)
        self.eintraege: dict[str, int] = {}      # Konto -> Meldehoehe
        # Abgemeldete bleiben mit Abmeldehoehe stehen: fuer das Los zaehlt der
        # Stand zur Bestellung, nicht zum Losen. Sonst koennte, wer den Zufall
        # schon kennt, sich vor dem Los abmelden und es so verschieben.
        self.abgemeldet: dict[str, int] = {}     # Konto -> Abmeldehoehe
        self._mensch_von: dict[str, str] = {}    # Mensch -> Konto

    def melden(self, konto: str, hoehe: int) -> None:
        m = self.menschen.get(konto)
        if m is None:
            raise Abgewiesen("Register: kein registrierter Mensch")
        if konto in self.eintraege:
            raise Abgewiesen("Register: schon gemeldet")
        if m in self._mensch_von:
            raise Abgewiesen("Register: dieser Mensch hat schon einen Praeparator")
        self.eintraege[konto] = hoehe
        self._mensch_von[m] = konto

    def abmelden(self, konto: str, hoehe: int) -> None:
        if konto not in self.eintraege or konto in self.abgemeldet:
            raise Abgewiesen("Register: nicht gemeldet")
        self.abgemeldet[konto] = hoehe

    def abbild(self, bestell_hoehe: int) -> list[str]:
        """Wer zum Zeitpunkt der Bestellung im Los ist: mindestens
        MELDE_VORLAUF Bloecke vorher gemeldet. Sortiert (deterministisch)."""
        return sorted(k for k, hm in self.eintraege.items()
                      if hm + MELDE_VORLAUF <= bestell_hoehe and self.abgemeldet.get(k, 1 << 62) > bestell_hoehe)


# ---------------------------------------------------------------- Zufall und Los
def zufall_aus_beitraegen(zusagen: dict[str, bytes], offen: dict[str, bytes]) -> bytes:
    """Zufall der Hoehe h + LOS_ABSTAND aus Beitraegen mehrerer (Zusage, dann
    Offenlegung). Jede Offenlegung muss zu ihrer Zusage passen; mindestens zwei
    Beitraege. Kennt ein einziger Beitragender seinen Beitrag nicht vorher
    (ehrlich), kennt niemand das Ergebnis vorher."""
    if len(offen) < 2:
        raise Abgewiesen("Zufall: weniger als zwei Beitraege")
    acc = b""
    for wer in sorted(offen):
        if wer not in zusagen or h(DOM_ZUFALL, offen[wer]) != zusagen[wer]:
            raise Abgewiesen(f"Zufall: Offenlegung von {wer} passt nicht zur Zusage")
        acc = h(DOM_ZUFALL, acc, wer.encode(), offen[wer])
    return acc


def auslosen(abbild: list[str], zufall: bytes, K: str, v: int) -> list[str]:
    """Die v Konten mit dem kleinsten H(Zufall, K, Konto), in Losreihenfolge
    (Praeparator 1 = erster). Fail-closed: v ausserhalb der Grenzen oder zu
    kleines Register."""
    if not V_MIN <= v <= V_MAX:
        raise Abgewiesen(f"Los: v = {v} ausserhalb {V_MIN}..{V_MAX}")
    if len(abbild) < POOL_MIN_FAKTOR * v or len(set(abbild)) != len(abbild):
        raise Abgewiesen(f"Los: Register zu klein ({len(abbild)} fuer v = {v})")
    rang = sorted(abbild, key=lambda k: (h(DOM_LOS, zufall, K.encode(), k.encode()), k))
    return rang[:v]


class KetteB:
    """Was die Kette fuer Weg B zusaetzlich fuehrt: Bestellungen mit Hoehe und
    Abbild, das Los und die Pruefung der Ausgabe gegen das Los."""

    def __init__(self, register: Register):
        self.register = register
        self.bestellungen: dict[str, dict] = {}

    def bestellen(self, K: str, hoehe: int, v: int, t: int) -> None:
        if K in self.bestellungen:
            raise Abgewiesen("Bestellung: K schon bestellt")
        if not 1 <= t <= v:
            raise Abgewiesen("Bestellung: Schwelle")
        if wk.k_note(sk.TOL_PROZENT, t, v) <= 0:
            raise Abgewiesen("Bestellung: t von v ist nicht sicher moeglich")
        # Das Abbild wird erst beim Losen gerechnet -- aus dem Stand ZUR
        # Bestellung (Meldung und Abmeldung mit Hoehe), nicht zum Losen.
        self.bestellungen[K] = {"hoehe": hoehe, "v": v, "t": t, "los": None}

    def losen(self, K: str, hoehe_jetzt: int, zufall: bytes) -> list[str]:
        b = self.bestellungen.get(K)
        if b is None:
            raise Abgewiesen("Los: keine Bestellung")
        if b["los"] is not None:
            raise Abgewiesen("Los: schon gelost")
        if hoehe_jetzt < b["hoehe"] + LOS_ABSTAND:
            raise Abgewiesen("Los: Zufall der Hoehe h + LOS_ABSTAND noch nicht da")
        b["los"] = auslosen(self.register.abbild(b["hoehe"]), zufall, K, b["v"])
        return list(b["los"])

    def ausgabe_pruefen(self, K: str, praeparatoren: list[str]) -> None:
        """Die Ausgabe nennt ihre Praeparatoren in Abschnittsreihenfolge; es
        muessen genau die Gelosten sein, in Losreihenfolge."""
        b = self.bestellungen.get(K)
        if b is None or b["los"] is None:
            raise Abgewiesen("Ausgabe: nicht gelost")
        if praeparatoren != b["los"]:
            raise Abgewiesen("Ausgabe: Praeparatoren sind nicht die Gelosten")


# ---------------------------------------------------------------- Zahlen
def erfassung(n_pool: int, boese: int, v: int, t: int) -> float:
    """P(mindestens t Boese unter v Gelosten) bei b Boesen unter N."""
    if not 0 <= boese <= n_pool or v > n_pool:
        raise ValueError("erfassung: Groessen")
    gesamt = math.comb(n_pool, v)
    return sum(math.comb(boese, j) * math.comb(n_pool - boese, v - j)
               for j in range(t, min(v, boese) + 1)) / gesamt


def verfuegbar(v: int, t: int, p: float) -> float:
    """P(die Stichprobe laesst sich in der Frist laden) = P(Bin(v, p) >= t)."""
    return sum(math.comb(v, m) * p ** m * (1 - p) ** (v - m) for m in range(t, v + 1))


def kontingent(sekunden: int = IBM_OPEN_SEKUNDEN, je_auftrag: int = SEKUNDEN_JE_AUFTRAG) -> int:
    """So viele Abschnitte (Auftraege) praepariert ein Mensch je 28 Tage im
    kostenlosen Konto -- auf Grundlage der Schaetzung je Auftrag."""
    return sekunden // je_auftrag


def tabelle(n_pool: int = 200, boese_anteil: float = 0.2) -> list[tuple]:
    """Fuer die Doku: je t von v die Stichprobe k, die Erfassung bei 20 %
    Boesen und die Verfuegbarkeit bei p = 0,8 und 0,6."""
    b = int(n_pool * boese_anteil)
    out = []
    for t, v in [(3, 4), (4, 7), (5, 9), (6, 11), (7, 13)]:
        out.append((t, v, wk.k_note(sk.TOL_PROZENT, t, v), erfassung(n_pool, b, v, t),
                    verfuegbar(v, t, 0.8), verfuegbar(v, t, 0.6)))
    return out


# ---------------------------------------------------------------- Ablauf
def ausgabe_weg_b(kette: KetteB, K: str, los: list[str], n: int, t: int, rng: random.Random):
    """Ausgabe mit den Gelosten als Praeparatoren (Weg A mit gelosten
    Menschen). Rueckgabe: AusgabeAbschnitte; Praeparator j = los[j-1]."""
    kette.ausgabe_pruefen(K, los)
    return sk.AusgabeAbschnitte(K, n, t, len(los), [random.Random(rng.randrange(1 << 62)) for _ in los])


def stichprobe_laden(aa, b: int, erreichbar: set[int]) -> dict[int, int]:
    """Die Oeffnung fuer Stelle b: von jedem erreichbaren Praeparator direkt,
    fuer die anderen aus t Anteilen der Erreichbaren (R9). Fehlt ein Abschnitt,
    gibt es keine Stichprobe (fail-closed, die Einloesung wartet)."""
    out: dict[int, int] = {}
    for j, p in enumerate(aa.praep, start=1):
        if j in erreichbar:
            out.update(p.oeffnung(b))
            continue
        helfer = sorted(erreichbar - {j})[: aa.t]
        if len(helfer) < aa.t:
            raise Abgewiesen(f"Stichprobe: Abschnitt {j} nicht erreichbar und weniger als t Anteile")
        for i in p.abschnitt:
            wert = wk.zusammensetzen([(xv, aa.tresor[xv][i]) for xv in helfer])
            if sk.stellen_bit(wert) == b:
                out[i] = wert
    return out


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

    rng = random.Random(20261122)

    # 1. Register: ein Mensch, ein Praeparator; nur Registrierte; Vorlauf.
    menschen = {f"0x{i:040x}": f"mensch-{i}" for i in range(1, 61)}
    menschen["0x" + "f" * 40] = "mensch-1"          # zweites Konto desselben Menschen
    reg = Register(menschen)
    for i in range(1, 61):
        reg.melden(f"0x{i:040x}", 100)
    abgewiesen(lambda: reg.melden("0x" + "f" * 40, 100), "zweiter Praeparator desselben Menschen")
    abgewiesen(lambda: reg.melden("0x" + "e" * 40, 100), "Konto ohne Menschen")
    abgewiesen(lambda: reg.melden("0x" + f"{1:040x}", 100), "doppelt gemeldet")
    spaet = Register({"0xa": "a", "0xb": "b"})
    spaet.melden("0xa", 0)
    spaet.melden("0xb", 600)
    soll(spaet.abbild(1000) == ["0xa"], "Vorlauf: zu spaet Gemeldeter im Los")
    ab_reg = Register({"0xa": "a"})
    ab_reg.melden("0xa", 0)
    ab_reg.abmelden("0xa", 4000)
    soll("0xa" not in ab_reg.abbild(5000), "abgemeldet, aber im Los")
    soll("0xa" in ab_reg.abbild(3000), "Abmeldung wirkt rueckwirkend auf fruehere Bestellungen")
    abgewiesen(lambda: ab_reg.abmelden("0xa", 4100), "zweimal abgemeldet")

    # 2. Zufall: Zusage und Offenlegung; mindestens zwei Beitraege.
    beitr = {"c1": b"a" * 32, "c2": b"b" * 32}
    zus = {w: h(DOM_ZUFALL, x) for w, x in beitr.items()}
    z = zufall_aus_beitraegen(zus, beitr)
    abgewiesen(lambda: zufall_aus_beitraegen(zus, {"c1": beitr["c1"]}), "ein einziger Beitrag")
    abgewiesen(lambda: zufall_aus_beitraegen(zus, {"c1": beitr["c1"], "c2": b"x" * 32}),
               "Offenlegung passt nicht zur Zusage")
    soll(zufall_aus_beitraegen(zus, beitr) == z, "Zufall nicht deterministisch")

    # 3. Los: deterministisch, ohne Doppelte, an die Bestellung gebunden.
    kette = KetteB(reg)
    K = hashlib.sha256(b"weg-b-1").hexdigest()
    v, t = 7, 4
    kette.bestellen(K, 5000, v, t)
    abgewiesen(lambda: kette.bestellen(K, 5000, v, t), "K doppelt bestellt")
    abgewiesen(lambda: kette.losen(K, 5000 + LOS_ABSTAND - 1, z), "Los vor dem Zufall")
    los = kette.losen(K, 5000 + LOS_ABSTAND, z)
    soll(len(los) == v == len(set(los)), "Los: Zahl oder Doppelte")
    soll(los == auslosen(reg.abbild(5000), z, K, v), "Los nicht nachrechenbar")
    soll(auslosen(reg.abbild(5000), h(b"anderer Zufall"), K, v) != los, "Los haengt nicht vom Zufall ab")
    abgewiesen(lambda: kette.losen(K, 6000, z), "zweimal gelost")
    # Angriff: Wer den Zufall schon kennt, meldet sich zwischen Bestellung und
    # Los ab, um das Los zu verschieben. Fuer das Los zaehlt der Stand zur
    # Bestellung -- das Ergebnis bleibt gleich.
    K_ab = hashlib.sha256(b"weg-b-abmelden").hexdigest()
    kette.bestellen(K_ab, 5000, v, t)
    vorher = auslosen(reg.abbild(5000), z, K_ab, v)
    reg.abmelden(vorher[0], 5005)
    soll(kette.losen(K_ab, 5000 + LOS_ABSTAND, z) == vorher, "Abmeldung nach der Bestellung verschiebt das Los")
    abgewiesen(lambda: auslosen(reg.abbild(5000)[:3 * v - 1], z, K, v), "Register zu klein")
    abgewiesen(lambda: auslosen(reg.abbild(5000), z, K, V_MAX + 1), "v zu gross")
    # v = 1 ist bestellbar (k_note > 0), scheitert aber am Los: kein Weg B.
    k1 = KetteB(reg)
    try:
        k1.bestellen(K, 5000, 1, 1)
        abgewiesen(lambda: k1.losen(K, 6000, z), "Los mit v = 1")
    except Abgewiesen:
        pass

    # 4. Die Ausgabe muss die Gelosten nennen -- der Ausgeber waehlt nicht.
    n = 2 * wk.k_note(sk.TOL_PROZENT, t, v)
    soll(n == 2 * 3472, f"Stellen bei 4 von 7: {n}")
    falsch = list(los)
    falsch[0] = next(k for k in reg.abbild(5000) if k not in los)
    abgewiesen(lambda: kette.ausgabe_pruefen(K, falsch), "nicht geloster Praeparator")
    abgewiesen(lambda: kette.ausgabe_pruefen(K, list(reversed(los))), "Reihenfolge vertauscht")
    abgewiesen(lambda: kette.ausgabe_pruefen(K, los[:-1]), "ein Praeparator weniger")
    aa = ausgabe_weg_b(kette, K, los, n, t, rng)

    # 5. Ehrlich einloesen, drei der sieben Handys nicht erreichbar (R9).
    y = [rng.randrange(2) for _ in range(n)]
    x = aa.messen(y, 0.007, rng)
    g = {}
    for p in aa.praep:
        g.update(p.geheim())
    u = [g[i][0] for i in range(n)]
    tt = [g[i][1] for i in range(n)]
    ks = sk.KetteS()
    ks.ausgeben(K, n, "anna")
    sk.festlegen(ks, K, [], list(y), 1)
    erreichbar = {1, 2, 4, 6}
    for b in (0, 1):
        o = stichprobe_laden(aa, b, erreichbar)
        soll(o == {i: w for p in aa.praep for i, w in p.oeffnung(b).items()},
             f"R9: aus Anteilen geladene Stichprobe weicht ab (Stelle {b})")
    o1 = stichprobe_laden(aa, 1, erreichbar)
    st1 = sk.stelle_laden_bit(1, K, n, aa.wurzel, aa.blaetter, t, v, o1)
    st1.abgleichen(ks)
    soll(st1.pruefen(K, x) is True, "ehrliche Vorlage mit 3 fehlenden Handys abgewiesen")
    abgewiesen(lambda: stichprobe_laden(aa, 1, {1, 2, 3}), "Stichprobe mit weniger als t Erreichbaren")

    # 6. Verschworene unter den Gelosten (weniger als t) + Inhaberin: an keiner
    #    zweiten Stelle durch. Gegenprobe: duerfte der Ausgeber waehlen, nimmt
    #    er t Verschworene -- dann kommt die Inhaberin an beiden Stellen durch.
    boese = set(reg.abbild(5000)[:12])                 # 12 von 60 = 20 %
    unter = {j for j, k in enumerate(los, start=1) if k in boese}
    koal = set(sorted(unter)[: t - 1]) if len(unter) >= t else unter
    st = [sk.stelle_laden_bit(b, K, n, aa.wurzel, aa.blaetter, t, v, stichprobe_laden(aa, b, set(range(1, v + 1))))
          for b in (0, 1)]
    k2 = sk.KetteS()
    k2.ausgeben(K, n, "anna")
    sk.festlegen(k2, K, st, [rng.randrange(2) for _ in range(n)], 1)
    xb = sk.messen_breidbart(u, tt, rng)
    for i, (_, ti) in aa.wissen(koal).items():
        xb[i] = ti
    soll(not (st[0].pruefen(K, xb) and st[1].pruefen(K, xb)),
         f"Verschworene {sorted(koal)} (< t) + Inhaberin an beiden Stellen durch")
    gewaehlt = set(range(1, t + 1))                    # ohne Los: der Ausgeber waehlt t Komplizen
    xg = sk.messen_breidbart(u, tt, rng)
    for i, (_, ti) in aa.wissen(gewaehlt).items():
        xg[i] = ti
    st_g = [sk.stelle_laden_bit(b, K, n, aa.wurzel, aa.blaetter, t, v, stichprobe_laden(aa, b, set(range(1, v + 1))))
            for b in (0, 1)]
    k3 = sk.KetteS()
    k3.ausgeben(K, n, "anna")
    sk.festlegen(k3, K, st_g, [rng.randrange(2) for _ in range(n)], 1)
    soll(st_g[0].pruefen(K, xg) and st_g[1].pruefen(K, xg),
         "Gegenprobe: t gewaehlte Komplizen kommen nicht an beiden Stellen durch")

    # 7. Probieren vieler K hilft dem Ausgeber nicht, wenn er den Zufall nicht
    #    kennt: der Anteil der Lose mit >= t Boesen bleibt bei der
    #    hypergeometrischen Erwartung. GEGENPROBE: kennt er den Zufall vorher
    #    (ein einziger Blockerzeuger), findet er schnell ein K mit t Boesen.
    ab = reg.abbild(5000)
    erwartet = erfassung(len(ab), len(boese), v, t)
    versuche, treffer = 2000, 0
    for i in range(versuche):
        zi = h(b"unbekannt", i.to_bytes(4, "big"))     # je Bestellung ein neuer, unbekannter Zufall
        Ki = hashlib.sha256(i.to_bytes(4, "big")).hexdigest()
        treffer += sum(k in boese for k in auslosen(ab, zi, Ki, v)) >= t
    soll(abs(treffer / versuche - erwartet) < 0.03, f"Los verzerrt: {treffer / versuche:.3f} statt {erwartet:.3f}")
    bekannt = h(b"vom Blockerzeuger vorher gewusst")
    gefunden = next((i for i in range(5000)
                     if sum(k in boese for k in auslosen(ab, bekannt, hashlib.sha256(i.to_bytes(4, "big")).hexdigest(), v)) >= t),
                    None)
    soll(gefunden is not None, "Gegenprobe: mit bekanntem Zufall findet Probieren kein K mit t Boesen")

    # 8. Zahlen: Erfassung, Verfuegbarkeit, Kontingent.
    soll(erfassung(60, 0, v, t) == 0.0 and erfassung(60, 60, v, t) == 1.0, "erfassung: Raender")
    soll(abs(sum(math.comb(12, j) * math.comb(48, v - j) for j in range(v + 1)) / math.comb(60, v) - 1) < 1e-12,
         "erfassung: Verteilung summiert nicht zu 1")
    soll(erfassung(200, 40, 7, 4) < 0.04, f"Erfassung 4 von 7 bei 20 %: {erfassung(200, 40, 7, 4):.4f}")
    soll(erfassung(200, 40, 13, 7) < 0.01, f"Erfassung 7 von 13 bei 20 %: {erfassung(200, 40, 13, 7):.5f}")
    soll(verfuegbar(7, 4, 1.0) == 1.0 and verfuegbar(7, 4, 0.0) == 0.0, "verfuegbar: Raender")
    soll(verfuegbar(7, 4, 0.8) > 0.95, f"Verfuegbarkeit 4 von 7 bei p = 0,8: {verfuegbar(7, 4, 0.8):.3f}")
    soll(kontingent() == 60, f"Kontingent: {kontingent()}")

    for f in fehler:
        print("FEHLER:", f)
    print("Selbsttest Weg B:", "gut" if not fehler else f"{len(fehler)} Fehler")
    return not fehler


if __name__ == "__main__":
    if "--tabelle" in sys.argv:
        print(" t   v      k   Erfassung(20 %)   verfuegbar(p=0,8)   verfuegbar(p=0,6)")
        for t, v, k, e, a8, a6 in tabelle():
            print(f"{t:2d}  {v:2d}  {k:5d}   {e:15.5f}   {a8:17.4f}   {a6:17.4f}")
    elif "--selbsttest" in sys.argv:
        sys.exit(0 if selbsttest() else 1)
    else:
        print(__doc__)
