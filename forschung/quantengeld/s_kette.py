#!/usr/bin/env python3
"""Variante S: Quanten-Token OHNE Quantenspeicher, angenommen ohne Netz.

Forschung, kein Produkt. Keine Abhaengigkeiten. Grundlage: S-money von Kent
u. a., „Practical quantum tokens without quantum memories and experimental
tests“, npj Quantum Information 8, 28 (2022), arXiv:2104.11717, Schema IQT1.
Einordnung: ANALYSE.md, Abschnitt 5b.

ABLAUF
  1. Ausgabe: N Qubits in zufaelligen BB84-Zustaenden (Basen u, Werte t). Die
     Nutzerin misst SOFORT, jede Stelle in einer zufaelligen Basis y, und hat
     danach nur Klartext x. Nichts wird gespeichert. (u, t) liegen als Anteile
     bei den Validatoren (Fern-Praeparation wie W3), auf der Kette nur die
     Merkle-Wurzel der Pruefsummen (wiesner_kette.Praegung). Jede
     Annahmestelle laedt ihre Stichprobe als Anteile und prueft sie gegen die
     Wurzel (stelle_laden) -- sie glaubt keinem Validator.
  2. Bindung auf der Kette: die Nutzerin veroeffentlicht d = y XOR z (z ein
     geheimes Bit, maskiert ihre Basen).
  3. Wahl auf der Kette, EINMAL je Token: c = b XOR z fuer die Stelle b, an der
     sie vorlegen wird. Wer nur die Kette sieht, erfaehrt b nicht (z ist
     geheim). Die Annahmestellen holen sich (d, c) VOR der Vorlage
     (abgleichen, solange Netz da ist).
  4. Vorlage an der Stelle b, OHNE Netz: sie zeigt x. Die Stelle b prueft die
     Stellen, deren Basis u_k gleich d_k XOR b XOR c ist (fuer die ehrliche
     Nutzerin genau ihre Basen y), nur auf IHRER Stichprobe. An jeder anderen
     Stelle ist das die GEGENbasis -- dort hat sie nur Zufall.
  WARUM c AUF DIE KETTE MUSS: duerfte die Nutzerin c an jeder Stelle selbst
  nennen, waehlte sie es so, dass ueberall ihre Basen geprueft werden -- und
  kaeme ueberall durch (der Selbsttest zeigt genau das). Bei Kent u. a. schickt
  deshalb die Ausgabestelle dasselbe c an alle ihre Pruefstellen.

WAS NEU IST GEGENUEBER KENT U. A.: STICHPROBEN-ANNAHMESTELLEN
  Jede Annahmestelle bekommt vorab nur (u, t) auf einer eigenen, geheimen,
  zu den anderen disjunkten Stichprobe von k Stellen. Folgen:
  - Sie prueft sofort, OHNE Netz und ohne Rueckfrage (der Vorteil der S-Token).
  - Eine bestochene Stelle kennt nur ihre Stichprobe; an einer anderen Stelle
    durchzukommen braucht die Werte auf deren Stichprobe -- sie faelscht nicht.
  - Die Nutzerin braucht KEIN vertrauenswuerdiges Geraet (anders als
    Offline-Bargeld mit Secure Element): ihr Token ist Klartext, aber an zwei
    Stellen durchzukommen verbietet die Physik.
  Abgleich auf der Kette (spaeter, wenn Netz da ist): jede Stelle meldet ihre
  Vorlagen; doppelte Vorlagen eines Tokens fallen auf. Die Physik sorgt dafuer,
  dass hoechstens eine davon die Pruefung bestanden haben kann.

SICHERHEIT IN ZAHLEN (wiesner_parameter.py)
  An zwei Stellen vorlegen heisst: richtige Ergebnisse in beiden Basen. Bestes
  Messen dafuer ist die Breidbart-Basis: 14,64 % Fehler in beiden -- dieselbe
  Schwelle wie der Kloner der Variante W. Darum: Toleranz deutlich darunter,
  Pruefgroesse aus wiesner_parameter.suche(). Bei 0,7 % Hardware-Fehler:
  Toleranz 4 %, k = 840 Stellen je Annahmestelle, N = 2 * 840 Qubits.

GRENZEN (ehrlich)
  - Kein Bargeld von Hand zu Hand: wer x weitergibt, kennt es weiter.
  - Gueltig an genau einer von 2^M vorher festgelegten Stellen (oder
    Zeitfenstern). Die Stellen muessen ihre Stichproben geheim halten.
  - Die Ausgabe braucht QKD-Empfaengertechnik bei der Nutzerin.

Aufruf:  python3 -I s_kette.py [--selbsttest]
"""
from __future__ import annotations

import hashlib
import hmac
import importlib.util
import math
import os
import random
import sys

_spec = importlib.util.spec_from_file_location("wiesner_kette", os.path.join(os.path.dirname(os.path.abspath(__file__)), "wiesner_kette.py"))
wk = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(wk)

TOL_PROZENT = 4          # wiesner_parameter.suche(0.007): Toleranz 4 %
K_STELLE = 840           # Stellen je Annahmestelle (je Stichprobe)
MIN_TREFFER = 1
BREIDBART_RICHTIG = math.cos(math.pi / 8) ** 2   # 0,8536 in beiden Basen


class Abgewiesen(Exception):
    pass


# ---------------------------------------------------------------- Ausgabe
def ausgabe(n: int, rng: random.Random) -> tuple[list[int], list[int]]:
    """Basen u und Werte t der Ausgabestelle (bei Aequitas: Fern-Praeparation,
    als Anteile bei den Validatoren)."""
    return [rng.randrange(2) for _ in range(n)], [rng.randrange(2) for _ in range(n)]


def messen_ehrlich(u, t, rng: random.Random, e: float) -> tuple[list[int], list[int]]:
    """Die Nutzerin misst sofort, jede Stelle in zufaelliger Basis y. Passt die
    Basis, ist das Ergebnis t (mit Hardware-Fehler e), sonst Zufall."""
    y, x = [], []
    for uk, tk in zip(u, t):
        yk = rng.randrange(2)
        xk = (tk ^ (rng.random() < e)) if yk == uk else rng.randrange(2)
        y.append(yk)
        x.append(int(xk))
    return y, x


def messen_breidbart(u, t, rng: random.Random) -> list[int]:
    """Der Faelscher misst jede Stelle in der Basis zwischen X und Z: in BEIDEN
    Basen mit 0,854 richtig -- das Beste, um an zwei Stellen vorzulegen."""
    return [tk if rng.random() < BREIDBART_RICHTIG else 1 - tk for tk in t]


# ---------------------------------------------------------------- Kette
DOM_PROBE = b"aequitas-quanten-s-probe-v1|"
DOM_VORLAGE = "aequitas-quanten-s-vorlage-v1|"


def probe_hash(K: str, b: int, probe: list[int]) -> bytes:
    """Bindet die Stichprobe der Stelle b schon bei der Ausgabe -- eine Stelle
    kann spaeter keine guenstigere behaupten."""
    return wk.h(DOM_PROBE, K.encode(), b"|", str(b).encode(), b"|", ",".join(map(str, sorted(probe))).encode())


def vorlage_text(K: str, b: int, x: list[int]) -> str:
    """Was die Nutzerin bei der Vorlage unterschreibt: dieses Token, diese
    Stelle, dieses x. Ohne ihre Unterschrift kann keine Stelle einloesen --
    sonst loeste eine Stelle, die ihre Stichprobe kennt, jeden Token selbst ein."""
    return f"{DOM_VORLAGE}{K}|{b}|{wk.h(bytes(x)).hex()}"


def sig(schluessel: bytes, text: str) -> str:
    return wk.KetteW.sig(schluessel, text)


class KetteS:
    """Was die Kette je Token weiss: Inhaber, d (Bindung) und c (Wahl), je
    einmal; mit Deckung auch Wurzel, Stichproben-Bindungen und den Stand
    des Einloesens. Ausgabe und Offenlegen wie Variante W (Anteile bei den
    Validatoren, Pruefsummen und Merkle-Wurzel auf der Kette)."""

    def __init__(self):
        self.token: dict[str, dict] = {}
        self.konto_schluessel: dict[str, bytes] = {}
        self.konten: dict[str, int] = {}
        self.topf = 0

    def konto(self, name: str, guthaben: int, schluessel: bytes):
        self.konten[name] = guthaben
        self.konto_schluessel[name] = schluessel

    def ausgeben(self, K: str, n: int, inhaber: str, wurzel: bytes | None = None, deckung: int = 0,
                 proben: list[bytes] | None = None, t: int = 1, n_val: int = 1, tol: int = TOL_PROZENT,
                 von: str | None = None):
        """Ausgabe. Mit Deckung (von zahlt sie in den Topf): Wurzel der
        Pruefsummen, je Stelle die Bindung ihrer Stichprobe, Schwelle t
        von n_val Validatoren, Toleranz."""
        if K in self.token:
            raise Abgewiesen("K schon da")
        if deckung:
            if von is None or self.konten.get(von, 0) < deckung or wurzel is None or not proben or len(proben) != 2:
                raise Abgewiesen("Ausgabe: Deckung, Wurzel oder Stichproben")
            self.konten[von] -= deckung
            self.topf += deckung
        self.token[K] = {"n": n, "inhaber": inhaber, "d": None, "c": None, "wurzel": wurzel, "deckung": deckung,
                         "proben": list(proben or []), "t": t, "n_val": n_val, "tol": tol, "eingeloest": False}

    def einloesen(self, K: str, b: int, stelle_konto: str, x: list[int], vorlage_sig: str,
                  probe: list[int], offen) -> bool:
        """Die Annahmestelle b loest einen angenommenen Token ein: mit der
        Unterschrift der Nutzerin unter ihre Vorlage, ihrer Stichprobe (passt
        zur Bindung bei der Ausgabe) und den offengelegten Anteilen dieser
        Stellen (wie W: jeder Knoten setzt zusammen, prueft Pruefsumme und
        Pfad). Bestanden: die Deckung an die Stelle, der Token ist verbraucht.
        Ein zweites Einloesen gibt es nicht."""
        e = self.token.get(K)
        if e is None or not e["deckung"] or e["eingeloest"]:
            raise Abgewiesen("Einloesen: unbekannt, ohne Deckung oder schon eingeloest")
        if e["d"] is None or e["c"] is None or b not in (0, 1) or stelle_konto not in self.konten:
            raise Abgewiesen("Einloesen: Bindung, Wahl, Stelle")
        if len(x) != e["n"] or any(v not in (0, 1) for v in x):
            raise Abgewiesen("Einloesen: Form von x")
        if not hmac.compare_digest(sig(self.konto_schluessel[e["inhaber"]], vorlage_text(K, b, x)), vorlage_sig):
            raise Abgewiesen("Einloesen: Vorlage nicht von der Inhaberin unterschrieben")
        if probe_hash(K, b, probe) != e["proben"][b] or len(offen) != len(probe):
            raise Abgewiesen("Einloesen: Stichprobe passt nicht zur Ausgabe")
        treffer = fehler = 0
        for i, (anteile, pfad) in zip(sorted(probe), offen):
            xs = [xx for xx, _ in anteile]
            if len(anteile) != e["t"] or len(set(xs)) != len(xs) or any(not 1 <= xx <= e["n_val"] for xx in xs) \
                    or any(not 0 <= y < wk.P for _, y in anteile):
                raise Abgewiesen("Einloesen: Anteile")
            v = wk.zusammensetzen(anteile)
            if v >> (wk.SALZ_BITS + 2) or not wk.merkle_pruefen(e["wurzel"], wk.pruefsumme(K, i, v), i, pfad, e["n"]):
                raise Abgewiesen("Einloesen: Pruefsumme oder Pfad")
            uk, tk = wk.zerlegen(v)
            if (e["d"][i] ^ b ^ e["c"]) == uk:
                treffer += 1
                fehler += x[i] != tk
        if treffer < wk.MIN_TREFFER or fehler * 100 > e["tol"] * treffer:
            raise Abgewiesen(f"Einloesen: Pruefung nicht bestanden ({fehler} Fehler bei {treffer} Treffern)")
        e["eingeloest"] = True
        self.topf -= e["deckung"]
        self.konten[stelle_konto] += e["deckung"]
        return True

    def binden(self, K: str, von: str, d: list[int]):
        e = self.token.get(K)
        if e is None or von != e["inhaber"] or e["d"] is not None:
            raise Abgewiesen("Bindung: fremd oder schon gebunden")
        if len(d) != e["n"] or any(v not in (0, 1) for v in d):
            raise Abgewiesen("Bindung: Form")
        e["d"] = list(d)

    def waehlen(self, K: str, von: str, c: int):
        e = self.token.get(K)
        if e is None or von != e["inhaber"] or e["d"] is None or e["c"] is not None:
            raise Abgewiesen("Wahl: fremd, ungebunden oder schon gewaehlt")
        if c not in (0, 1):
            raise Abgewiesen("Wahl: Form")
        e["c"] = c


# ---------------------------------------------------------------- Annahmestellen
def stichproben(n: int, stellen: int, k: int, rng: random.Random) -> list[list[int]]:
    """Disjunkte, geheime Stichproben je Annahmestelle."""
    if stellen * k > n:
        raise ValueError("zu wenige Qubits fuer die Stichproben")
    perm = list(range(n))
    rng.shuffle(perm)
    return [sorted(perm[i * k:(i + 1) * k]) for i in range(stellen)]


class Annahmestelle:
    """Prueft sofort und ohne Netz. Kennt (u, t) NUR auf ihrer Stichprobe."""

    def __init__(self, nummer: int, K: str, stichprobe: list[int], u, t):
        self.nummer = nummer
        self.K = K
        self.daten = {i: (u[i], t[i]) for i in stichprobe}
        self.gesehen: set[str] = set()
        self.meldungen: list[dict] = []
        self.stand: dict[str, tuple[list[int], int]] = {}

    def abgleichen(self, kette: KetteS):
        """Solange Netz da ist: (d, c) der Token holen, die schon gewaehlt sind."""
        e = kette.token.get(self.K)
        if e and e["d"] is not None and e["c"] is not None:
            self.stand[self.K] = (list(e["d"]), e["c"])

    def pruefen(self, K: str, x: list[int], tol: int = TOL_PROZENT,
                min_treffer: int = MIN_TREFFER) -> bool | None:
        """Ohne Netz: nur mit dem abgeglichenen (d, c); ohne Abgleich keine
        Annahme (fail-closed)."""
        if K != self.K or K in self.gesehen:
            raise Abgewiesen("fremder Token oder schon vorgelegt")
        if K not in self.stand:
            raise Abgewiesen("Wahl nicht abgeglichen")
        d, c = self.stand[K]
        if len(x) != len(d) or any(v not in (0, 1) for v in x):
            raise Abgewiesen("Form")
        self.gesehen.add(K)
        b = self.nummer
        treffer = fehler = 0
        for i, (uk, tk) in self.daten.items():
            if (d[i] ^ b ^ c) == uk:      # Basis der Nutzerin passt zur Ausgabe
                treffer += 1
                fehler += x[i] != tk
        urteil = None if treffer < min_treffer else fehler * 100 <= tol * treffer
        self.meldungen.append({"K": K, "stelle": b, "treffer": treffer, "fehler": fehler, "urteil": urteil,
                               "x_hash": hashlib.sha256(bytes(x)).hexdigest()})
        return urteil


def stelle_laden(nummer: int, K: str, n: int, wurzel: bytes, t: int, n_val: int,
                 stichprobe: list[int], offen) -> "Annahmestelle":
    """DEZENTRALE AUSGABE: die Annahmestelle bekommt ihre Stichprobe nicht von
    einer Bank, sondern als Anteile von t Validatoren (wiesner_kette.Praegung,
    v = Basis + 2*Wert + 4*Salz) und prueft JEDE Stelle selbst gegen die
    Pruefsumme und die Merkle-Wurzel auf der Kette. Ein falscher Anteil oder
    Pfad: die Stelle laedt nicht (fail-closed) -- sie glaubt keinem Validator."""
    if len(offen) != len(stichprobe):
        raise Abgewiesen("Ladung: Zahl der Stellen")
    u, tt = [0] * n, [0] * n
    for i, (anteile, pfad) in zip(stichprobe, offen):
        xs = [x for x, _ in anteile]
        if len(anteile) != t or len(set(xs)) != t or any(not 1 <= x <= n_val for x in xs) \
                or any(not 0 <= y < wk.P for _, y in anteile):
            raise Abgewiesen("Ladung: Anteile")
        v = wk.zusammensetzen(anteile)
        if v >> (wk.SALZ_BITS + 2) or not wk.merkle_pruefen(wurzel, wk.pruefsumme(K, i, v), i, pfad, n):
            raise Abgewiesen("Ladung: Pruefsumme oder Pfad passt nicht")
        u[i], tt[i] = wk.zerlegen(v)
    return Annahmestelle(nummer, K, stichprobe, u, tt)


def abgleich(meldungen: list[dict]) -> dict[str, list[dict]]:
    """Spaeter auf der Kette: angenommene Vorlagen je Token. Mehr als eine
    bestandene Vorlage eines Tokens waere ein Doppelverbrauch."""
    je: dict[str, list[dict]] = {}
    for m in meldungen:
        if m["urteil"] is True:
            je.setdefault(m["K"], []).append(m)
    return je


def token(rng: random.Random, e: float = 0.007, k: int = K_STELLE):
    """Ein Token mit zwei Annahmestellen: (kette, K, u, t, stellen)."""
    n = 2 * k
    u, t = ausgabe(n, rng)
    K = hashlib.sha256(rng.randbytes(16)).hexdigest()
    kette = KetteS()
    kette.ausgeben(K, n, "anna")
    proben = stichproben(n, 2, k, rng)
    stellen = [Annahmestelle(b, K, proben[b], u, t) for b in (0, 1)]
    return kette, K, u, t, stellen


def festlegen(kette: KetteS, K: str, stellen, d: list[int], c: int, von: str = "anna"):
    kette.binden(K, von, d)
    kette.waehlen(K, von, c)
    for s in stellen:
        s.abgleichen(kette)


# ---------------------------------------------------------------- Selbsttest
def selbsttest() -> bool:
    fehler: list[str] = []

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    def abgewiesen(f, text):
        try:
            f()
            fehler.append(f"nicht abgewiesen: {text}")
        except Abgewiesen:
            pass

    try:
        rng = random.Random(2022)
        laeufe = 300
        # 1. Ehrlich: an der gewaehlten Stelle angenommen (0,7 % Hardware-Fehler).
        durch = 0
        for _ in range(laeufe):
            kette, K, u, t, st = token(rng)
            y, x = messen_ehrlich(u, t, rng, 0.007)
            z, b = rng.randrange(2), rng.randrange(2)
            festlegen(kette, K, st, [yk ^ z for yk in y], b ^ z)
            durch += st[b].pruefen(K, x) is True
        soll(durch == laeufe, f"ehrlich: {durch}/{laeufe}")

        # 2. Missbrauch -- an BEIDEN Stellen vorlegen.
        # a) ehrlich gemessen: an der anderen Stelle wird die Gegenbasis geprueft.
        # b) Breidbart (das Beste gegen zwei Basen): 14,6 % > 4 % -- faellt durch.
        beide_a = beide_b = 0
        for _ in range(laeufe):
            kette, K, u, t, st = token(rng)
            y, x = messen_ehrlich(u, t, rng, 0.007)
            z = rng.randrange(2)
            festlegen(kette, K, st, [yk ^ z for yk in y], 0 ^ z)
            beide_a += st[0].pruefen(K, x) is True and st[1].pruefen(K, x) is True
            kette, K, u, t, st = token(rng)
            xb = messen_breidbart(u, t, rng)
            festlegen(kette, K, st, [rng.randrange(2) for _ in u], rng.randrange(2))
            beide_b += st[0].pruefen(K, xb) is True and st[1].pruefen(K, xb) is True
        soll(beide_a == 0, f"ehrlich gemessen, an zwei Stellen angenommen: {beide_a}")
        soll(beide_b == 0, f"Breidbart an zwei Stellen angenommen: {beide_b}")
        # Gegenprobe: mit der ALTEN Toleranz (15 %) kommt Breidbart durch.
        alt = 0
        for _ in range(100):
            kette, K, u, t, st = token(rng, k=64)
            xb = messen_breidbart(u, t, rng)
            festlegen(kette, K, st, [rng.randrange(2) for _ in u], 0)
            alt += st[0].pruefen(K, xb, tol=15) is True and st[1].pruefen(K, xb, tol=15) is True
        soll(alt >= 10, f"Gegenprobe: Breidbart gegen 15 % nur {alt}/100 -- Test zu schwach?")
        # Gegenprobe zur Wahl auf der Kette: duerfte man c je Stelle selbst nennen,
        # kaeme ein ehrlich gemessener Token an BEIDEN Stellen durch.
        frei = 0
        for _ in range(50):
            kette, K, u, t, st = token(rng, k=64)
            y, x = messen_ehrlich(u, t, rng, 0.0)
            z = rng.randrange(2)
            d = [yk ^ z for yk in y]
            for b in (0, 1):
                st[b].stand[K] = (d, b ^ z)          # je Stelle ein anderes c
            frei += st[0].pruefen(K, x) is True and st[1].pruefen(K, x) is True
        soll(frei == 50, f"Gegenprobe c je Stelle: {frei}/50")

        # c) Bestochene Stelle 0 kennt (u, t) auf IHRER Stichprobe und ist selbst
        # Inhaberin: bei Stelle 1 kennt sie nichts -- abgewiesen.
        bestochen = 0
        for _ in range(laeufe):
            kette, K, u, t, st = token(rng)
            x = [rng.randrange(2) for _ in u]
            d = [rng.randrange(2) for _ in u]
            for i, (uk, tk) in st[0].daten.items():
                x[i], d[i] = tk, uk ^ 1            # fuer Stelle 1 mit c = 0: d^1 == u
            festlegen(kette, K, st, d, 0)
            bestochen += st[1].pruefen(K, x) is True
        soll(bestochen == 0, f"bestochene Stelle faelscht: {bestochen}")
        # Gegenprobe: kennt sie ALLES (keine Stichproben), kommt sie durch.
        kette, K, u, t, st = token(rng)
        festlegen(kette, K, st, [uk ^ 1 for uk in u], 0)
        soll(st[1].pruefen(K, list(t)) is True, "Gegenprobe: wer alles kennt, muss durchkommen")

        # d) Kette: nur der Inhaber, je einmal; Stelle ohne Abgleich nimmt nicht an.
        kette, K, u, t, st = token(rng, k=16)
        y, x = messen_ehrlich(u, t, rng, 0.0)
        abgewiesen(lambda: st[0].pruefen(K, x), "Vorlage ohne Abgleich")
        abgewiesen(lambda: kette.binden(K, "fremd", y), "fremde Bindung")
        abgewiesen(lambda: kette.waehlen(K, "anna", 0), "Wahl vor der Bindung")
        abgewiesen(lambda: kette.binden(K, "anna", y[:-1]), "kurze Bindung")
        kette.binden(K, "anna", y)
        abgewiesen(lambda: kette.binden(K, "anna", y), "zweite Bindung")
        abgewiesen(lambda: kette.waehlen(K, "anna", 2), "c = 2")
        kette.waehlen(K, "anna", 0)
        abgewiesen(lambda: kette.waehlen(K, "anna", 1), "zweite Wahl")
        abgewiesen(lambda: kette.ausgeben(K, 32, "x"), "K doppelt")
        for s_ in st:
            s_.abgleichen(kette)
        soll(st[0].pruefen(K, x) is True, "Gutfall klein")
        abgewiesen(lambda: st[0].pruefen(K, x), "zweite Vorlage an derselben Stelle")
        abgewiesen(lambda: st[1].pruefen("f" * 64, x), "fremder Token")
        abgewiesen(lambda: st[1].pruefen(K, x[:-1]), "kurze Vorlage")

        # 3. Abgleich: zwei bestandene Vorlagen desselben Tokens fielen auf
        # (nur mit vollem Wissen ueberhaupt moeglich).
        kette, K, u, t, st = token(rng, k=16)
        festlegen(kette, K, st, list(u), 0)
        st[0].pruefen(K, list(t))
        st[1].daten = {i: (1 - uk, tk) for i, (uk, tk) in st[1].daten.items()}  # Wissen: alles
        st[1].pruefen(K, list(t))
        soll(len(abgleich(st[0].meldungen + st[1].meldungen).get(K, [])) == 2, "Abgleich erkennt Doppelverbrauch nicht")
        # 4. Dezentrale Ausgabe: Stichproben als Anteile von 3 der 5 Validatoren,
        # jede Stelle prueft gegen die Wurzel auf der Kette.
        val = [f"v{j}" for j in range(1, 6)]
        n = 2 * 64
        u, t = ausgabe(n, rng)
        K = hashlib.sha256(rng.randbytes(16)).hexdigest()
        pr = wk.Praegung(K, u, t, 3, val, rng)
        proben = stichproben(n, 2, 64, rng)
        st = [stelle_laden(b, K, n, pr.wurzel, 3, 5, proben[b], pr.offenlegung(proben[b], [1, 3, 5] if b else [2, 4, 5]))
              for b in (0, 1)]
        soll(all(st[b].daten == {i: (u[i], t[i]) for i in proben[b]} for b in (0, 1)), "dezentral geladen: falsche Daten")
        kette = KetteS()
        kette.ausgeben(K, n, "anna")
        y, x = messen_ehrlich(u, t, rng, 0.0)
        festlegen(kette, K, st, list(y), 1)
        soll(st[1].pruefen(K, x) is True and st[0].pruefen(K, x) is False, "dezentral: Gutfall und Doppelvorlage")
        # Missbrauch: ein Validator liefert einen falschen Anteil, ein falscher Pfad,
        # zu wenige oder doppelte Anteile, fremde Wurzel -- die Stelle laedt nicht.
        of = pr.offenlegung(proben[0], [1, 2, 3])
        falsch = [([(a[0][0], (a[0][1] + 1) % wk.P)] + a[1:], p_) if j == 0 else (a, p_) for j, (a, p_) in enumerate(of)]
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], falsch), "falscher Anteil")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], [(a, p_[::-1]) for a, p_ in of]), "falscher Pfad")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], [(a[:2], p_) for a, p_ in of]), "zu wenige Anteile")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], [([a[0], a[0], a[1]], p_) for a, p_ in of]), "doppeltes x")
        abgewiesen(lambda: stelle_laden(0, K, n, wk.h(b"fremd"), 3, 5, proben[0], of), "fremde Wurzel")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of[:-1]), "Stelle fehlt")
        # Anteile, die ein richtiges v ergeben, aber gegen die Regel verstossen:
        # t+1 Anteile; ein Anteil an x = 9 (ausserhalb 1..v, aus dem Polynom
        # nachgerechnet). Die Pruefsumme allein faengt das NICHT -- die Regel schon.
        def am(anteile, x0):
            sm = 0
            for j, (xj, yj) in enumerate(anteile):
                z = nn = 1
                for m_, (xm, _) in enumerate(anteile):
                    if m_ != j:
                        z, nn = z * (x0 - xm) % wk.P, nn * (xj - xm) % wk.P
                sm = (sm + yj * z * pow(nn, -1, wk.P)) % wk.P
            return sm
        of4 = pr.offenlegung(proben[0], [1, 2, 3, 4])
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of4), "t+1 Anteile")
        aussen = [([(9, am(a, 9))] + a[1:], p_) for a, p_ in of]
        soll(all(wk.zusammensetzen(a) == wk.zusammensetzen(b) for (a, _), (b, _) in zip(aussen, of)), "Probe x = 9 falsch gebaut")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], aussen), "Anteil an x = 9")
        # Gegenprobe: dieselbe Ladung unveraendert geht durch.
        soll(stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of).daten == st[0].daten, "Gegenprobe Ladung")

        # 5. Einloesen auf der Kette: die Stelle reicht die unterschriebene
        # Vorlage ein, die Validatoren legen ihre Stichprobe offen, jeder Knoten
        # prueft; die Deckung geht an die Stelle -- genau einmal.
        n, k5 = 2 * 128, 128
        u, t = ausgabe(n, rng)
        K = hashlib.sha256(rng.randbytes(16)).hexdigest()
        pr = wk.Praegung(K, u, t, 3, val, rng)
        proben = stichproben(n, 2, k5, rng)
        kette = KetteS()
        for name in ("ausgabe", "anna", "laden0", "laden1"):
            kette.konto(name, 50_000_000 if name == "ausgabe" else 0, name.encode() * 4)
        kette.ausgeben(K, n, "anna", pr.wurzel, 10_000_000, [probe_hash(K, b, proben[b]) for b in (0, 1)], 3, 5, von="ausgabe")
        st = [stelle_laden(b, K, n, pr.wurzel, 3, 5, proben[b], pr.offenlegung(proben[b], [1, 2, 3])) for b in (0, 1)]
        y, x = messen_ehrlich(u, t, rng, 0.0)
        z = rng.randrange(2)
        festlegen(kette, K, st, [yk ^ z for yk in y], 1 ^ z)
        vs = sig(kette.konto_schluessel["anna"], vorlage_text(K, 1, x))
        soll(st[1].pruefen(K, x) is True, "Vorlage an Stelle 1")
        of1 = pr.offenlegung(proben[1], [2, 4, 5])
        # Missbrauch: ohne Unterschrift der Nutzerin (die Stelle kennt ihre
        # Stichprobe und koennte jeden Token selbst einloesen), mit fremder
        # Stichprobe, fuer die andere Stelle, mit falschem Anteil.
        xf = [t[i] if i in st[1].daten else 0 for i in range(n)]  # aus dem Wissen der Stelle gebaut
        abgewiesen(lambda: kette.einloesen(K, 1, "laden1", xf, sig(b"laden1" * 4, vorlage_text(K, 1, xf)), proben[1], of1),
                   "Stelle loest ohne Vorlage der Nutzerin ein")
        abgewiesen(lambda: kette.einloesen(K, 1, "laden1", x, vs, proben[0], pr.offenlegung(proben[0], [1, 2, 3])), "fremde Stichprobe")
        abgewiesen(lambda: kette.einloesen(K, 0, "laden0", x, vs, proben[0], pr.offenlegung(proben[0], [1, 2, 3])), "Vorlage fuer Stelle 1 bei Stelle 0")
        falsch = [([(a_[0][0], (a_[0][1] + 1) % wk.P)] + a_[1:], p_) if j == 0 else (a_, p_) for j, (a_, p_) in enumerate(of1)]
        abgewiesen(lambda: kette.einloesen(K, 1, "laden1", x, vs, proben[1], falsch), "falscher Anteil")
        abgewiesen(lambda: kette.einloesen(K, 1, "laden1", x, vs, proben[1], of1[:-1]), "Stelle fehlt")
        soll(kette.einloesen(K, 1, "laden1", x, vs, proben[1], of1) and kette.konten["laden1"] == 10_000_000 and kette.topf == 0,
             "Einloesen auf der Kette")
        abgewiesen(lambda: kette.einloesen(K, 1, "laden1", x, vs, proben[1], of1), "zweimal eingeloest")
        # Die Nutzerin unterschreibt einen geratenen Token fuer die andere
        # Stelle: die Physik weist ihn ab (Gegenbasis), auch auf der Kette.
        K2 = hashlib.sha256(rng.randbytes(16)).hexdigest()
        pr2 = wk.Praegung(K2, u, t, 3, val, rng)
        kette.konto("ausgabe", 50_000_000, b"ausgabe" * 4)
        kette.ausgeben(K2, n, "anna", pr2.wurzel, 10_000_000, [probe_hash(K2, b, proben[b]) for b in (0, 1)], 3, 5, von="ausgabe")
        kette.binden(K2, "anna", [yk ^ z for yk in y])
        kette.waehlen(K2, "anna", 1 ^ z)
        vs0 = sig(kette.konto_schluessel["anna"], vorlage_text(K2, 0, x))
        abgewiesen(lambda: kette.einloesen(K2, 0, "laden0", x, vs0, proben[0], pr2.offenlegung(proben[0], [1, 2, 3])),
                   "Token an der nicht gewaehlten Stelle eingeloest")

        # Stichproben disjunkt und vollstaendig.
        pr = stichproben(40, 2, 20, rng)
        soll(not set(pr[0]) & set(pr[1]) and len(set(pr[0]) | set(pr[1])) == 40, "Stichproben")
        try:
            stichproben(10, 2, 6, rng)
            fehler.append("zu grosse Stichproben angenommen")
        except ValueError:
            pass
    except Exception as ex:
        fehler.append(f"Ausnahme: {type(ex).__name__}: {ex}")
    for f in fehler:
        print("FEHLER:", f)
    print("s_kette: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def vektor() -> dict:
    """Fester Pruefvektor fuer den Knoten (quanten_s_test.go): Bindung der
    Stichprobe, Vorlage-Text, ein Einloesen mit offengelegten Anteilen und
    sein Ergebnis -- alles deterministisch."""
    rng = random.Random(1926)
    n, k, val = 40, 20, ["v1", "v2", "v3", "v4", "v5"]
    u, t = ausgabe(n, rng)
    K = hashlib.sha256(b"aequitas-quanten-s-vektor").hexdigest()
    pr = wk.Praegung(K, u, t, 3, val, rng)
    proben = stichproben(n, 2, k, rng)
    y = [rng.randrange(2) for _ in range(n)]
    x = [t[i] if y[i] == u[i] else rng.randrange(2) for i in range(n)]
    erster = next(i for i in proben[1] if y[i] == u[i])
    x_fehler = list(x)
    x_fehler[erster] ^= 1  # genau ein Fehler an einem Treffer
    z = 1
    d = [yi ^ z for yi in y]
    b, c = 1, 1 ^ z
    treffer = sum((d[i] ^ b ^ c) == u[i] for i in proben[1])
    fehler = sum((d[i] ^ b ^ c) == u[i] and x[i] != t[i] for i in proben[1])
    return {
        "K": K, "n": n, "t": 3, "n_val": 5, "tol": TOL_PROZENT,
        "wurzel": pr.wurzel.hex(),
        "proben": proben, "probe_hash": [probe_hash(K, bb, proben[bb]).hex() for bb in (0, 1)],
        "d": d, "c": c, "b": b, "x": x,
        "vorlage_text": vorlage_text(K, b, x),
        "offen": [{"anteile": [[xx, str(yy)] for xx, yy in a_], "pfad": [p_.hex() for p_ in pf]}
                  for a_, pf in pr.offenlegung(proben[1], [1, 3, 5])],
        # Die Stichprobe der ANDEREN Stelle mit gueltigen Anteilen: ohne die
        # Bindung bei der Ausgabe kaeme eine Stelle damit durch.
        "offen_andere": [{"anteile": [[xx, str(yy)] for xx, yy in a_], "pfad": [p_.hex() for p_ in pf]}
                         for a_, pf in pr.offenlegung(proben[0], [1, 3, 5])],
        "treffer": treffer, "fehler": fehler,
        "urteil": treffer >= wk.MIN_TREFFER and fehler * 100 <= TOL_PROZENT * treffer,
        # Ein Fehler bei ~10 Treffern liegt ueber 4 %: abgewiesen.
        "x_fehler": x_fehler, "urteil_fehler": treffer >= wk.MIN_TREFFER and 100 <= TOL_PROZENT * treffer,
    }


if __name__ == "__main__":
    if "--vektor" in sys.argv:
        import json
        print(json.dumps(vektor(), indent=1))
    else:
        sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else print(__doc__)
