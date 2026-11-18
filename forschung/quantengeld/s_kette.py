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

AUSGABE IN ABSCHNITTEN (Weg A, 18.11.2026, DEZENTRAL.md)
  Die Pruefgroesse k (Tabelle) setzt voraus, dass weniger als t Validatoren
  hoechstens den Anteil (t-1)/v der Stellen kennen. Die erste Ausgabe im
  Hauptnetz praeparierte aber ALLE Stellen in einem Auftrag: wer ihn absetzt,
  kennt den ganzen Token. Hier praepariert jeder Validator SEINEN Abschnitt
  selbst (Praeparator, AusgabeAbschnitte) und teilt dessen Geheimnis selbst
  auf; der Ausgeber sieht nur Blaetter. Die Kette bleibt, wie sie ist.

GRENZEN (ehrlich)
  - Kein Bargeld von Hand zu Hand: wer x weitergibt, kennt es weiter.
  - Gueltig an genau einer von 2^M vorher festgelegten Stellen (oder
    Zeitfenstern). Die Stellen muessen ihre Stichproben geheim halten und
    laden nur mit mindestens k_note Stellen (stelle_laden).
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
        self.grundeinkommen = 0

    def konto(self, name: str, guthaben: int, schluessel: bytes):
        self.konten[name] = guthaben
        self.konto_schluessel[name] = schluessel

    def ausgeben(self, K: str, n: int, inhaber: str, wurzel: bytes | None = None, deckung: int = 0,
                 proben: list[bytes] | None = None, t: int = 1, n_val: int = 1, tol: int = TOL_PROZENT,
                 von: str | None = None, stellen: list[str] | None = None):
        """Ausgabe. Mit Deckung (von zahlt sie in den Topf): Wurzel der
        Pruefsummen, je Stelle die Bindung ihrer Stichprobe, Schwelle t
        von n_val Validatoren, Toleranz."""
        if K in self.token:
            raise Abgewiesen("K schon da")
        # Die zwei Annahmestellen stehen bei der Ausgabe fest (im Knoten
        # Pflicht, quanten_w_s_kette.go): die Vorlage der Nutzerin bindet die
        # Adresse der Stelle nicht, und die Anmeldung legt die geheime
        # Stichprobe offen -- wer sie in der Warteschlange sieht, koennte sonst
        # zuerst anmelden und einloesen.
        if stellen is not None and (len(stellen) != 2 or stellen[0] == stellen[1] or inhaber in stellen):
            raise Abgewiesen("Ausgabe: Annahmestellen")
        if deckung:
            if von is None or self.konten.get(von, 0) < deckung or wurzel is None or not proben or len(proben) != 2:
                raise Abgewiesen("Ausgabe: Deckung, Wurzel oder Stichproben")
            self.konten[von] -= deckung
            self.topf += deckung
        self.token[K] = {"n": n, "inhaber": inhaber, "d": None, "c": None, "wurzel": wurzel, "deckung": deckung,
                         "proben": list(proben or []), "t": t, "n_val": n_val, "tol": tol, "eingeloest": False,
                         "tot": False, "fehl": 0, "anmeldung": None, "stellen": list(stellen) if stellen else None}

    # -- Einloesen auf der Kette (S2b): anmelden, dann Offenlegen in Teilen
    def anmelden(self, K: str, b: int, stelle_konto: str, x: list[int], vorlage_sig: str, probe: list[int],
                 zeit: int = 0):
        """Die Annahmestelle b meldet einen angenommenen Token zum Einloesen
        an: mit der Unterschrift der Nutzerin unter ihre Vorlage und ihrer
        Stichprobe (passt zur Bindung bei der Ausgabe). Je Token hoechstens
        eine offene Anmeldung."""
        e = self.token.get(K)
        if e is None or not e["deckung"] or e["eingeloest"] or e["tot"]:
            raise Abgewiesen("Einloesen: unbekannt, ohne Deckung, eingeloest oder tot")
        self._frist(e, zeit)
        if e["anmeldung"] is not None:
            raise Abgewiesen("Einloesen: schon eine offene Anmeldung")
        if e["d"] is None or e["c"] is None or b not in (0, 1) or stelle_konto not in self.konten:
            raise Abgewiesen("Einloesen: Bindung, Wahl, Stelle")
        if e["stellen"] is not None and stelle_konto != e["stellen"][b]:
            raise Abgewiesen("Einloesen: nicht die Annahmestelle b dieses Tokens")
        if len(x) != e["n"] or any(v not in (0, 1) for v in x):
            raise Abgewiesen("Einloesen: Form von x")
        if not hmac.compare_digest(sig(self.konto_schluessel[e["inhaber"]], vorlage_text(K, b, x)), vorlage_sig):
            raise Abgewiesen("Einloesen: Vorlage nicht von der Inhaberin unterschrieben")
        sortiert = sorted(probe)
        if len(set(sortiert)) != len(sortiert) or probe_hash(K, b, sortiert) != e["proben"][b]:
            raise Abgewiesen("Einloesen: Stichprobe passt nicht zur Ausgabe")
        e["anmeldung"] = {"b": b, "stelle": stelle_konto, "x": list(x), "probe": sortiert, "offen": 0,
                          "treffer": 0, "fehler": 0, "zeit": zeit}

    def offenlegen(self, K: str, ab: int, je_stelle, zeit: int = 0):
        """Ein Teil der Offenlegung (hoechstens wk.OFFEN_TEIL Stellen der Probe,
        der Reihe nach, jeder Teil ganz oder gar nicht). Rueckgabe "teil",
        solange Stellen fehlen; dann True (Deckung an die Stelle, Token
        verbraucht) oder False (nicht bestanden: Anmeldung weg, Fehlversuch;
        nach FEHL_MAX ist der Token tot und die Deckung geht ans Grundeinkommen)."""
        e = self.token.get(K)
        if e is None:
            raise Abgewiesen("Offenlegen: unbekannt")
        self._frist(e, zeit)
        a = e["anmeldung"]
        if a is None:
            raise Abgewiesen("Offenlegen: keine Anmeldung")
        if ab != a["offen"] or not 1 <= len(je_stelle) <= wk.OFFEN_TEIL or ab + len(je_stelle) > len(a["probe"]):
            raise Abgewiesen("Offenlegen: Teil (Beginn oder Laenge)")
        treffer = fehler = 0
        for j, (anteile, pfad) in enumerate(je_stelle, ab):
            i = a["probe"][j]
            xs = [xx for xx, _ in anteile]
            if len(anteile) != e["t"] or len(set(xs)) != len(xs) or any(not 1 <= xx <= e["n_val"] for xx in xs) \
                    or any(not 0 <= y < wk.P for _, y in anteile):
                raise Abgewiesen("Offenlegen: Anteile")
            v = wk.zusammensetzen(anteile)
            if v >> (wk.SALZ_BITS + 2) or not wk.merkle_pruefen(e["wurzel"], wk.pruefsumme(K, i, v), i, pfad, e["n"]):
                raise Abgewiesen("Offenlegen: Pruefsumme oder Pfad")
            uk, tk = wk.zerlegen(v)
            if (e["d"][i] ^ a["b"] ^ e["c"]) == uk:
                treffer += 1
                fehler += a["x"][i] != tk
        a["offen"] += len(je_stelle)
        a["treffer"] += treffer
        a["fehler"] += fehler
        if a["offen"] < len(a["probe"]):
            return "teil"
        e["anmeldung"] = None
        if a["treffer"] < wk.MIN_TREFFER or a["fehler"] * 100 > e["tol"] * a["treffer"]:
            e["fehl"] += 1
            if e["fehl"] >= wk.FEHL_MAX:
                e["tot"] = True
                self.topf -= e["deckung"]
                self.grundeinkommen += e["deckung"]
            return False
        e["eingeloest"] = True
        self.topf -= e["deckung"]
        self.konten[a["stelle"]] += e["deckung"]
        return True

    def _frist(self, e: dict, zeit: int):
        """Legen die Validatoren nicht in der Frist offen, endet die Anmeldung
        ohne Fehlversuch (nicht die Schuld der Stelle); sie darf neu anmelden."""
        a = e["anmeldung"]
        if a is not None and zeit > a["zeit"] + wk.FRIST_OFFENLEGEN:
            e["anmeldung"] = None

    def einloesen(self, K: str, b: int, stelle_konto: str, x: list[int], vorlage_sig: str,
                  probe: list[int], offen, zeit: int = 0) -> bool:
        """Bequemlichkeit: anmelden und alle Teile offenlegen. Nicht bestanden
        wirft Abgewiesen (wie bisher)."""
        self.anmelden(K, b, stelle_konto, x, vorlage_sig, probe, zeit)
        e = self.token[K]
        if len(offen) != len(e["anmeldung"]["probe"]):
            e["anmeldung"] = None
            raise Abgewiesen("Einloesen: Zahl der offengelegten Stellen")
        r = None
        try:
            for ab in range(0, len(offen), wk.OFFEN_TEIL):
                r = self.offenlegen(K, ab, offen[ab:ab + wk.OFFEN_TEIL], zeit)
        except Abgewiesen:
            e["anmeldung"] = None
            raise
        if r is not True:
            raise Abgewiesen("Einloesen: Pruefung nicht bestanden")
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


# ------------------------------------------- S3: Ausgabe ueber eine echte Leitung
#
# Bausteine fuer ANALYSE.md 5c (noch nicht in KetteS eingebunden; das kommt mit
# einem Geraet). Ueber eine Leitung kommt nur ein Teil der N Pulse an. Die
# Nutzerin meldet zuerst die Menge Lambda der gemessenen Pulse; erst danach zieht
# der Zufall der Kette daraus eine oeffentliche Teilmenge von hoechstens N_MAX
# Stellen, die den Token bildet. Die geheimen Stichproben liegen in ihr. Weil die
# Teilmenge nach der Meldung gezogen wird, kann eine Betruegerin den Anteil
# freier Stellen (Mehrphotonen-Pulse) darin nicht erhoehen; wiesner_parameter.
# s3_stichprobe rechnet die Pruefgroesse mit diesem Anteil.

DOM_S3_TEIL = b"aequitas-quanten-s3-teilmenge-v1|"
N_PULS_MAX = 1 << 26        # Pulse je Ausgabe (Kents Handsender: 4e7 in 4 s)


def s3_meldung(n_puls: int, gamma_det_ppm: int, meldung: list[int]) -> list[int]:
    """Prueft die Meldung Lambda: verschiedene Pulse in 0..n_puls-1, mindestens
    gamma_det * n_puls (ganzzahlig, ppm). Gibt sie sortiert zurueck."""
    if not (1 <= n_puls <= N_PULS_MAX and 1 <= gamma_det_ppm <= 1_000_000):
        raise Abgewiesen("S3-Meldung: Groessen")
    lam = sorted(meldung)
    if len(set(lam)) != len(lam) or any(not 0 <= i < n_puls for i in lam):
        raise Abgewiesen("S3-Meldung: doppelter oder fremder Puls")
    if len(lam) * 1_000_000 < gamma_det_ppm * n_puls:
        raise Abgewiesen(f"S3-Meldung: {len(lam)} Pulse, verlangt {gamma_det_ppm} ppm von {n_puls}")
    return lam


def s3_teilmenge(K: str, lam: list[int], zufall: bytes, meldung_hoehe: int, zufall_hoehe: int,
                 n_max: int = wk.N_MAX) -> list[int]:
    """Oeffentliche Teilmenge der gemeldeten Pulse, die den Token bildet.
    zufall muss aus einem Block NACH der Meldung stammen (sonst koennte die
    Nutzerin Lambda passend zur Teilmenge waehlen). Jeder Knoten rechnet sie
    gleich: die n_max Pulse mit den kleinsten h(dom|K|zufall|i), sortiert."""
    if zufall_hoehe <= meldung_hoehe:
        raise Abgewiesen("S3-Teilmenge: Zufall nicht nach der Meldung")
    if len(zufall) != 32 or not 1 <= n_max <= wk.N_MAX:
        raise Abgewiesen("S3-Teilmenge: Form")
    rang = sorted(lam, key=lambda i: wk.h(DOM_S3_TEIL, K.encode(), b"|", zufall, b"|", str(i).encode()))
    return sorted(rang[:n_max])


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
                 stichprobe: list[int], offen, tol: int = TOL_PROZENT,
                 labor_k: int | None = None) -> "Annahmestelle":
    """DEZENTRALE AUSGABE: die Annahmestelle bekommt ihre Stichprobe nicht von
    einer Bank, sondern als Anteile von t Validatoren (wiesner_kette.Praegung,
    v = Basis + 2*Wert + 4*Salz) und prueft JEDE Stelle selbst gegen die
    Pruefsumme und die Merkle-Wurzel auf der Kette. Ein falscher Anteil oder
    Pfad: die Stelle laedt nicht (fail-closed) -- sie glaubt keinem Validator.

    GROESSE: Die Stelle nimmt ohne Netz an und traegt damit das Risiko einer
    Doppelvorlage. Darum laedt sie nur eine Stichprobe mit mindestens
    k_note(tol, t, v) verschiedenen Stellen (wiesner_kette.K_TABELLE). Pruefen
    kann das NUR sie: die Stichprobe ist bis zur Einloesung geheim, und der
    Knoten koennte beim Anmelden nur noch die Auszahlung an die Stelle
    verweigern, die schon angenommen hat. labor_k: kleinere Groesse nur fuer
    Vorfuehrung und Tests."""
    k_soll = wk.k_note(tol, t, n_val) if labor_k is None else labor_k
    if k_soll <= 0 or len(set(stichprobe)) != len(stichprobe) or len(stichprobe) < k_soll \
            or any(not 0 <= i < n for i in stichprobe):
        raise Abgewiesen(f"Ladung: Stichprobe zu klein oder ungueltig ({len(stichprobe)} Stellen, verlangt {k_soll})")
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


# ---------------------------------------------------------------- Ausgabe in Abschnitten
# Weg A (DEZENTRAL.md): jeder Validator praepariert seinen Abschnitt selbst.
def abschnitte(n: int, n_val: int) -> list[list[int]]:
    """Die Stellen 0 .. n-1 in n_val zusammenhaengende Abschnitte, so gleich
    gross wie moeglich (hoechstens eine Stelle Unterschied); Validator x
    (1-basiert) praepariert abschnitte[x - 1]."""
    if n_val < 1 or n < n_val:
        raise ValueError("Abschnitte: weniger Stellen als Validatoren")
    return [list(range(j * n // n_val, (j + 1) * n // n_val)) for j in range(n_val)]


class Praeparator:
    """Validator x praepariert SEINEN Abschnitt in einem eigenen Auftrag beim
    Quantenrechner, mit eigenem Zufall: Basen u und Werte t nur dort, dazu die
    Messung der Inhaberin in den Basen y, die sie IHM nur fuer diesen Abschnitt
    nennt (eine Quantenleitung zu ihr gibt es heute nicht). Den Wert jeder
    seiner Stellen teilt er selbst t-von-v auf alle Validatoren auf. Ausserhalb
    seines Abschnitts kennt er nur je Stelle einen Anteil."""

    def __init__(self, x: int, K: str, abschnitt: list[int], t: int, n_val: int, rng: random.Random,
                 mit_stellen_bit: bool = False):
        if not 1 <= x <= n_val or not 1 <= t <= n_val or not abschnitt:
            raise ValueError("Praeparator: Nummer, Schwelle oder Abschnitt")
        self.x, self.K, self.abschnitt = x, K, list(abschnitt)
        self._u = {i: rng.randrange(2) for i in self.abschnitt}
        self._t = {i: rng.randrange(2) for i in self.abschnitt}
        # Stellen-Bit (A2): das unterste Bit des Salzes sagt, zu welcher
        # Annahmestelle die Stelle gehoert -- je Abschnitt genau zur Haelfte.
        bits = {i: rng.randrange(2) for i in self.abschnitt}
        if mit_stellen_bit:
            reihe = list(self.abschnitt)
            rng.shuffle(reihe)
            bits = {i: int(j >= len(reihe) // 2) for j, i in enumerate(reihe)}
        self._v = {i: wk.wert(self._u[i], self._t[i], bits[i] + 2 * rng.randrange(1 << (wk.SALZ_BITS - 1)))
                   for i in self.abschnitt}
        self.blaetter = {i: wk.pruefsumme(K, i, self._v[i]) for i in self.abschnitt}
        self._anteile = {i: wk.teilen(self._v[i], t, n_val, rng) for i in self.abschnitt}
        self._gemessen = False

    def messen(self, y: dict[int, int], e: float, geraet: random.Random) -> dict[int, int]:
        """Die Messung der Inhaberin auf diesem Abschnitt, im selben Auftrag wie
        die Praeparation. Genau einmal und genau die Stellen des Abschnitts
        (fail-closed): eine zweite Messung gaebe Ergebnisse in der anderen Basis."""
        if self._gemessen:
            raise Abgewiesen("Abschnitt: schon gemessen")
        if sorted(y) != self.abschnitt or any(b not in (0, 1) for b in y.values()):
            raise Abgewiesen("Abschnitt: Basen der Inhaberin passen nicht zum Abschnitt")
        self._gemessen = True
        return {i: int(self._t[i] ^ (geraet.random() < e)) if y[i] == self._u[i] else geraet.randrange(2)
                for i in self.abschnitt}

    def anteile_fuer(self, xv: int) -> dict[int, int]:
        """Was Validator xv aus diesem Abschnitt bekommt: je Stelle sein Anteil."""
        return {i: self._anteile[i][xv - 1] for i in self.abschnitt}

    def geheim(self) -> dict[int, tuple[int, int]]:
        """(u, t) dieses Abschnitts -- nur fuer die Rechnung, wer was weiss."""
        return {i: (self._u[i], self._t[i]) for i in self.abschnitt}

    def oeffnung(self, b: int) -> dict[int, int]:
        """Fuer Annahmestelle b, direkt und nur an sie: die gesalzenen Werte der
        Stellen seines Abschnitts mit Stellen-Bit b. Die Stelle prueft jeden
        Wert gegen sein Blatt -- ein Wert mit dem anderen Bit faellt auf."""
        if b not in (0, 1):
            raise Abgewiesen("Oeffnung: Stelle 0 oder 1")
        return {i: v for i, v in self._v.items() if stellen_bit(v) == b}


class AusgabeAbschnitte:
    """Ausgabe in Abschnitten. Der Ausgeber sieht nur die Blaetter (gesalzene
    Pruefsummen) und baut daraus die Wurzel. Jeder Validator haelt am Ende
    Anteile ALLER Stellen: seinen eigenen Abschnitt kennt er ganz, die anderen
    nur als je einen Anteil. Auf der Kette ist das nicht von der Ausgabe mit
    einem Auftrag zu unterscheiden (Wurzel, Anteile, Offenlegung gleich)."""

    def __init__(self, K: str, n: int, t: int, n_val: int, rngs: list[random.Random], mit_stellen_bit: bool = True):
        if len(rngs) != n_val or len({id(r) for r in rngs}) != n_val:
            raise ValueError("je Validator ein eigener Zufall")
        self.K, self.n, self.t, self.n_val = K, n, t, n_val
        self.abschnitte = abschnitte(n, n_val)
        self.praep = [Praeparator(x, K, self.abschnitte[x - 1], t, n_val, rngs[x - 1], mit_stellen_bit)
                      for x in range(1, n_val + 1)]
        blaetter: list[bytes | None] = [None] * n
        for p in self.praep:
            for i, b in p.blaetter.items():
                if blaetter[i] is not None:
                    raise Abgewiesen("Ausgabe: Stelle in zwei Abschnitten")
                blaetter[i] = b
        if any(b is None for b in blaetter):
            raise Abgewiesen("Ausgabe: Blatt fehlt")
        self.blaetter: list[bytes] = blaetter  # type: ignore[assignment]
        self.wurzel = wk.merkle_wurzel(self.blaetter)
        self.tresor: dict[int, dict[int, int]] = {xv: {} for xv in range(1, n_val + 1)}
        for p in self.praep:
            for xv in range(1, n_val + 1):
                self.tresor[xv].update(p.anteile_fuer(xv))

    def messen(self, y: list[int], e: float, geraet: random.Random) -> list[int]:
        """Die Inhaberin nennt jedem Validator nur y auf dessen Abschnitt und
        setzt x aus den Antworten zusammen."""
        if len(y) != self.n:
            raise Abgewiesen("Messung: Zahl der Basen")
        x = [0] * self.n
        for p in self.praep:
            for i, xi in p.messen({i: y[i] for i in p.abschnitt}, e, geraet).items():
                x[i] = xi
        return x

    def offenlegung(self, stellen: list[int], wer: list[int]):
        """Wie wiesner_kette.Praegung.offenlegung: je Stelle die Anteile der
        Validatoren `wer` (1-basiert) und der Merkle-Pfad."""
        return [([(xv, self.tresor[xv][i]) for xv in wer], wk.merkle_pfad(self.blaetter, i)) for i in stellen]

    def wissen(self, koalition: set[int]) -> dict[int, tuple[int, int]]:
        """Was eine Absprache der Validatoren `koalition` ueber (u, t) weiss:
        ihre eigenen Abschnitte ganz; ab t Validatoren jede Stelle (aus den
        Anteilen zusammengesetzt); darunter aus fremden Abschnitten nichts."""
        if len(koalition) >= self.t:
            wer = sorted(koalition)[: self.t]
            return {i: wk.zerlegen(wk.zusammensetzen([(xv, self.tresor[xv][i]) for xv in wer])) for i in range(self.n)}
        out: dict[int, tuple[int, int]] = {}
        for xv in koalition:
            out.update(self.praep[xv - 1].geheim())
        return out


def stellen_bit(v: int) -> int:
    """Bit 2 des gesalzenen Werts v = Basis + 2 Wert + 4 Salz, also das unterste
    Bit des Salzes: zu welcher der zwei Annahmestellen die Stelle gehoert.
    Fuer die Kette ist es Salz -- sie aendert sich dafuer nicht."""
    return (v >> 2) & 1


def stelle_laden_bit(nummer: int, K: str, n: int, wurzel: bytes, blaetter: list[bytes], t: int, n_val: int,
                     oeffnungen: dict[int, int], tol: int = TOL_PROZENT,
                     labor_k: int | None = None) -> "Annahmestelle":
    """STICHPROBEN OHNE VERTRAUTEN AUSGEBER (A2, DEZENTRAL.md 8). Die Stelle b
    bekommt von jedem Praeparator die Werte seiner Stellen mit Stellen-Bit b und
    prueft selbst: die Blaetter ergeben die Wurzel auf der Kette, jeder Wert
    passt zu seinem Blatt, und sein Stellen-Bit ist b. Damit sind die
    Stichproben der zwei Stellen disjunkt, ohne dass jemand beide kennt, und
    die Inhaberin kennt keine, bevor sie bindet (Befund 5g). Zu klein, ein
    fremdes Bit, ein falscher Wert: die Stelle laedt nicht (fail-closed)."""
    if nummer not in (0, 1) or len(blaetter) != n or wk.merkle_wurzel(blaetter) != wurzel:
        raise Abgewiesen("Ladung: Blaetter passen nicht zur Wurzel")
    k_soll = wk.k_note(tol, t, n_val) if labor_k is None else labor_k
    if k_soll <= 0 or len(oeffnungen) < k_soll:
        raise Abgewiesen(f"Ladung: Stichprobe zu klein ({len(oeffnungen)} Stellen, verlangt {k_soll})")
    u, tt = [0] * n, [0] * n
    for i, v in oeffnungen.items():
        if not 0 <= i < n or not 0 <= v < (1 << (wk.SALZ_BITS + 2)) or wk.pruefsumme(K, i, v) != blaetter[i]:
            raise Abgewiesen(f"Ladung: Stelle {i} passt nicht zu ihrem Blatt")
        if stellen_bit(v) != nummer:
            raise Abgewiesen(f"Ladung: Stelle {i} gehoert zur anderen Annahmestelle")
        u[i], tt[i] = wk.zerlegen(v)
    return Annahmestelle(nummer, K, sorted(oeffnungen), u, tt)


def fehler_je_abschnitt(stelle: "Annahmestelle", K: str, x: list[int], teile: list[list[int]]) -> list[tuple[int, int]]:
    """(Treffer, Fehler) der Stelle je Abschnitt, ohne Netz und ohne den Stand
    der Stelle zu aendern. Laesst ein Validator eine ehrliche Inhaberin mit
    falschen x durchfallen, haeufen sich die Fehler in SEINEM Abschnitt: so
    laesst sich die Stoerung ihm zuordnen."""
    d, c = stelle.stand[K]
    b = stelle.nummer
    out = []
    for teil in teile:
        tr = fe = 0
        for i in teil:
            if i in stelle.daten and (d[i] ^ b ^ c) == stelle.daten[i][0]:
                tr += 1
                fe += x[i] != stelle.daten[i][1]
        out.append((tr, fe))
    return out


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
        st = [stelle_laden(b, K, n, pr.wurzel, 3, 5, proben[b], pr.offenlegung(proben[b], [1, 3, 5] if b else [2, 4, 5]),
                          labor_k=64) for b in (0, 1)]
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
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], falsch, labor_k=64), "falscher Anteil")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], [(a, p_[::-1]) for a, p_ in of], labor_k=64), "falscher Pfad")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], [(a[:2], p_) for a, p_ in of], labor_k=64), "zu wenige Anteile")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], [([a[0], a[0], a[1]], p_) for a, p_ in of], labor_k=64), "doppeltes x")
        abgewiesen(lambda: stelle_laden(0, K, n, wk.h(b"fremd"), 3, 5, proben[0], of, labor_k=64), "fremde Wurzel")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of[:-1], labor_k=64), "Stelle fehlt")
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
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of4, labor_k=64), "t+1 Anteile")
        aussen = [([(9, am(a, 9))] + a[1:], p_) for a, p_ in of]
        soll(all(wk.zusammensetzen(a) == wk.zusammensetzen(b) for (a, _), (b, _) in zip(aussen, of)), "Probe x = 9 falsch gebaut")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], aussen, labor_k=64), "Anteil an x = 9")
        # Gegenprobe: dieselbe Ladung unveraendert geht durch.
        soll(stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of, labor_k=64).daten == st[0].daten, "Gegenprobe Ladung")
        # Missbrauch: zu kleine Stichprobe. Ohne Laborgroesse verlangt die Stelle
        # k_note(4 %, 3 von 5) = 3472 Stellen; 64 laedt sie nicht. Auch nicht mit
        # einer Stelle zu wenig oder mit einer doppelten Stelle, die die Zahl aufblaeht.
        soll(wk.k_note(TOL_PROZENT, 3, 5) == 3472, "k_note(4, 3, 5)")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of), "Stichprobe kleiner als k_note")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of, labor_k=65), "eine Stelle zu wenig")
        doppelt = proben[0][:-1] + proben[0][:1]          # 64 Eintraege, 63 verschiedene, Anteile passend
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, doppelt, pr.offenlegung(doppelt, [1, 2, 3]), labor_k=64),
                   "doppelte Stelle in der Stichprobe")
        abgewiesen(lambda: stelle_laden(0, K, n, pr.wurzel, 3, 5, proben[0], of, tol=15), "Toleranz nicht in der Tabelle")

        # 4b. S3-Bausteine: Meldung und oeffentliche Teilmenge.
        n_puls, ppm = 200_000, 18_000          # gamma_det = 1,8 % wie bei Kent
        gemeldet = sorted(rng.sample(range(n_puls), 3700))
        lam = s3_meldung(n_puls, ppm, gemeldet)
        abgewiesen(lambda: s3_meldung(n_puls, ppm, gemeldet[:3599]), "Meldung unter gamma_det")
        abgewiesen(lambda: s3_meldung(n_puls, ppm, gemeldet + gemeldet[:1]), "Meldung mit doppeltem Puls")
        abgewiesen(lambda: s3_meldung(n_puls, ppm, gemeldet[:-1] + [n_puls]), "Meldung mit fremdem Puls")
        z1, z2 = wk.h(b"block-101"), wk.h(b"block-102")
        abgewiesen(lambda: s3_teilmenge(K, lam, z1, 100, 100), "Zufall nicht nach der Meldung")
        abgewiesen(lambda: s3_teilmenge(K, lam, b"kurz", 100, 101), "Zufall in falscher Form")
        t1 = s3_teilmenge(K, lam, z1, 100, 101, n_max=2048)
        soll(len(t1) == 2048 and set(t1) <= set(lam) and t1 == sorted(t1), "Teilmenge: Groesse, in Lambda, sortiert")
        soll(t1 == s3_teilmenge(K, list(reversed(lam)), z1, 100, 101, n_max=2048), "Teilmenge haengt von der Reihenfolge ab")
        soll(t1 != s3_teilmenge(K, lam, z2, 100, 101, n_max=2048), "anderer Zufall, gleiche Teilmenge")
        soll(s3_teilmenge(K, lam[:100], z1, 100, 101, n_max=2048) == lam[:100], "kleines Lambda: alles")
        # Missbrauch: eine Betruegerin meldet alle 700 Mehrphotonen-Pulse plus 3000
        # andere (Anteil frei 19 %). Die Teilmenge, gezogen NACH der Meldung, hat
        # denselben Anteil (Abweichung im Bereich weniger Standardabweichungen) --
        # sie kann ihn nicht hochtreiben.
        mehr = set(rng.sample(range(n_puls), 700))
        lam_b = s3_meldung(n_puls, ppm, sorted(mehr) + sorted(rng.sample(sorted(set(range(n_puls)) - mehr), 3000)))
        anteil = len(mehr & set(s3_teilmenge(K, lam_b, z2, 100, 101, n_max=2048))) / 2048
        soll(abs(anteil - 700 / 3700) < 4 * math.sqrt(0.19 * 0.81 / 2048), f"Anteil frei in der Teilmenge {anteil:.3f}")

        # 5. Einloesen auf der Kette: die Stelle reicht die unterschriebene
        # Vorlage ein, die Validatoren legen ihre Stichprobe offen, jeder Knoten
        # prueft; die Deckung geht an die Stelle -- genau einmal.
        n, k5 = 2 * 128, 128
        u, t = ausgabe(n, rng)
        K = hashlib.sha256(rng.randbytes(16)).hexdigest()
        pr = wk.Praegung(K, u, t, 3, val, rng)
        proben = stichproben(n, 2, k5, rng)
        kette = KetteS()
        for name in ("ausgabe", "anna", "laden0", "laden1", "dieb"):
            kette.konto(name, 50_000_000 if name == "ausgabe" else 0, name.encode() * 4)
        abgewiesen(lambda: kette.ausgeben("f" * 64, n, "anna", pr.wurzel, 10_000_000, [b"x" * 32] * 2, 3, 5, von="ausgabe",
                                          stellen=["laden0", "laden0"]), "zweimal dieselbe Annahmestelle")
        kette.ausgeben(K, n, "anna", pr.wurzel, 10_000_000, [probe_hash(K, b, proben[b]) for b in (0, 1)], 3, 5, von="ausgabe",
                       stellen=["laden0", "laden1"])
        st = [stelle_laden(b, K, n, pr.wurzel, 3, 5, proben[b], pr.offenlegung(proben[b], [1, 2, 3]), labor_k=len(proben[b])) for b in (0, 1)]
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
        # Vorauslaufen: ein Dritter kopiert Vorlage und Stichprobe aus der
        # Warteschlange und meldet mit seinem Konto an.
        abgewiesen(lambda: kette.einloesen(K, 1, "dieb", x, vs, proben[1], of1), "Dritter mit kopierter Stichprobe")
        abgewiesen(lambda: kette.einloesen(K, 1, "laden0", x, vs, proben[1], of1), "andere Stelle mit kopierter Stichprobe")
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

        # 6. Einloesen in Teilen (S2b): 128 Stellen = zwei Teile; Missbrauch der
        # Teile; Frist ohne Fehlversuch; drei falsche Vorlagen -> tot.
        K3 = hashlib.sha256(rng.randbytes(16)).hexdigest()
        pr3 = wk.Praegung(K3, u, t, 3, val, rng)
        kette.konto("ausgabe", 50_000_000, b"ausgabe" * 4)
        kette.ausgeben(K3, n, "anna", pr3.wurzel, 10_000_000, [probe_hash(K3, bb, proben[bb]) for bb in (0, 1)], 3, 5, von="ausgabe")
        kette.binden(K3, "anna", [yk ^ z for yk in y])
        kette.waehlen(K3, "anna", 1 ^ z)
        vs3 = sig(kette.konto_schluessel["anna"], vorlage_text(K3, 1, x))
        of3 = pr3.offenlegung(proben[1], [1, 2, 3])
        kette.anmelden(K3, 1, "laden1", x, vs3, proben[1], 100)
        abgewiesen(lambda: kette.anmelden(K3, 1, "laden1", x, vs3, proben[1], 100), "zweite Anmeldung")
        abgewiesen(lambda: kette.offenlegen(K3, 64, of3[64:], 101), "Teil nicht der Reihe")
        abgewiesen(lambda: kette.offenlegen(K3, 0, of3[:65], 101), "Teil zu gross")
        soll(kette.offenlegen(K3, 0, of3[:64], 101) == "teil", "erster Teil")
        abgewiesen(lambda: kette.offenlegen(K3, 0, of3[:64], 101), "Teil wiederholt")
        # Frist: die Validatoren legen den Rest nicht offen -> Anmeldung weg, kein Fehlversuch.
        abgewiesen(lambda: kette.offenlegen(K3, 64, of3[64:], 100 + wk.FRIST_OFFENLEGEN + 1), "Teil nach der Frist")
        e3 = kette.token[K3]
        soll(e3["anmeldung"] is None and e3["fehl"] == 0 and not e3["eingeloest"], "Frist")
        # Neu anmelden, ganz offenlegen: eingeloest.
        kette.anmelden(K3, 1, "laden1", x, vs3, proben[1], 5000)
        soll(kette.offenlegen(K3, 0, of3[:64], 5001) == "teil" and kette.offenlegen(K3, 64, of3[64:], 5002) is True
             and e3["eingeloest"], "Einloesen in Teilen")
        # Drei falsche Vorlagen (geratenes x, von der Nutzerin unterschrieben) -> tot,
        # die Deckung geht ans Grundeinkommen.
        K4 = hashlib.sha256(rng.randbytes(16)).hexdigest()
        pr4 = wk.Praegung(K4, u, t, 3, val, rng)
        kette.konto("ausgabe", 50_000_000, b"ausgabe" * 4)
        kette.ausgeben(K4, n, "anna", pr4.wurzel, 10_000_000, [probe_hash(K4, bb, proben[bb]) for bb in (0, 1)], 3, 5, von="ausgabe")
        kette.binden(K4, "anna", [yk ^ z for yk in y])
        kette.waehlen(K4, "anna", 1 ^ z)
        of4 = pr4.offenlegung(proben[1], [1, 2, 3])
        ge0 = kette.grundeinkommen if hasattr(kette, "grundeinkommen") else 0
        for versuch in range(wk.FEHL_MAX):
            xr = [rng.randrange(2) for _ in range(n)]
            kette.anmelden(K4, 1, "laden1", xr, sig(kette.konto_schluessel["anna"], vorlage_text(K4, 1, xr)), proben[1], 9000)
            r = [kette.offenlegen(K4, ab, of4[ab:ab + 64], 9000) for ab in (0, 64)][-1]
            soll(r is False, f"geratene Vorlage {versuch} bestanden")
        e4 = kette.token[K4]
        soll(e4["tot"] and kette.grundeinkommen == ge0 + 10_000_000, "nach FEHL_MAX nicht tot")
        abgewiesen(lambda: kette.anmelden(K4, 1, "laden1", x, sig(kette.konto_schluessel["anna"], vorlage_text(K4, 1, x)), proben[1], 9001),
                   "Anmeldung an toten Token")

        # Stichproben disjunkt und vollstaendig.
        # 5. AUSGABE IN ABSCHNITTEN (Weg A, DEZENTRAL.md): jeder Validator
        # praepariert seinen Abschnitt selbst; der Ausgeber sieht nur Blaetter.
        # 5a. Abschnitte: disjunkt, decken alles, hoechstens eine Stelle Unterschied.
        for nn, vv in ((128, 4), (10, 3), (7, 7), (1680, 1)):
            ab = abschnitte(nn, vv)
            gr = [len(a) for a in ab]
            soll(sorted(i for a in ab for i in a) == list(range(nn)) and max(gr) - min(gr) <= 1, f"Abschnitte {nn}/{vv}")
        try:
            abschnitte(3, 4)
            fehler.append("Abschnitte mit weniger Stellen als Validatoren")
        except ValueError:
            pass
        # 5b. Gutfall mit 3 von 4: Stellen laden dezentral (stelle_laden), ehrliche
        # Vorlage an b besteht, Doppelvorlage an der anderen faellt durch.
        nv, ts, k = 4, 3, 64
        n = 2 * k
        K = hashlib.sha256(rng.randbytes(16)).hexdigest()
        aa = AusgabeAbschnitte(K, n, ts, nv, [random.Random(rng.randrange(1 << 62)) for _ in range(nv)])
        y = [rng.randrange(2) for _ in range(n)]
        x = aa.messen(y, 0.0, rng)
        proben = stichproben(n, 2, k, rng)
        st = [stelle_laden(b, K, n, aa.wurzel, ts, nv, proben[b], aa.offenlegung(proben[b], [1, 2, 4] if b else [2, 3, 4]),
                          labor_k=k) for b in (0, 1)]
        alle = {}
        for p_ in aa.praep:
            alle.update(p_.geheim())
        soll(all(st[b].daten == {i: alle[i] for i in proben[b]} for b in (0, 1)), "Abschnitte: Stellen falsch geladen")
        kette = KetteS()
        kette.ausgeben(K, n, "anna")
        festlegen(kette, K, st, list(y), 1)
        soll(st[1].pruefen(K, x) is True and st[0].pruefen(K, x) is False, "Abschnitte: Gutfall und Doppelvorlage")
        # 5c. Wer was weiss: jede Absprache von t-1 Validatoren kennt genau ihre
        # Abschnitte, also den Anteil (t-1)/v -- die Annahme der Tabelle; ab t
        # Validatoren alles (Schwelle); weniger als t Anteile ergeben nicht den Wert.
        for koal in ({1, 2}, {1, 3}, {2, 4}, {3, 4}):
            w = aa.wissen(koal)
            soll(set(w) == {i for xv in koal for i in aa.abschnitte[xv - 1]} and len(w) * nv <= (ts - 1) * n + nv,
                 f"Abschnitte: Wissen von {sorted(koal)}")
        soll(aa.wissen({1, 2, 3}) == alle, "Abschnitte: t Validatoren setzen alles zusammen")
        i0 = aa.abschnitte[3][0]
        soll(wk.zusammensetzen([(1, aa.tresor[1][i0]), (2, aa.tresor[2][i0])]) != aa.praep[3]._v[i0],
             "Abschnitte: t-1 Anteile ergeben den Wert")
        # 5d. Missbrauch in voller Groesse (k aus der Tabelle, 3 von 4): eine
        # Absprache von t-1 Validatoren verraet der Inhaberin ihre Abschnitte; den
        # Rest misst sie mit Breidbart (das Beste fuer zwei Stellen). Sie kommt
        # an keiner zweiten Stelle durch. GEGENPROBE: Kennt einer den ganzen Token
        # (ein Auftrag fuer alle Stellen, wie bei der ersten Ausgabe), kommt sie
        # an BEIDEN Stellen durch -- genau die Luecke, die die Abschnitte schliessen.
        kv = wk.k_note(TOL_PROZENT, ts, nv)
        soll(kv == 5200, f"k_note(4 %, 3 von 4) = {kv}")
        nb = 2 * kv
        Kb = hashlib.sha256(rng.randbytes(16)).hexdigest()
        ab4 = AusgabeAbschnitte(Kb, nb, ts, nv, [random.Random(rng.randrange(1 << 62)) for _ in range(nv)])
        wahr = {}
        for p_ in ab4.praep:
            wahr.update(p_.geheim())
        ub = [wahr[i][0] for i in range(nb)]
        tb = [wahr[i][1] for i in range(nb)]
        for runde in range(4):
            probe = stichproben(nb, 2, kv, rng)
            stb = [Annahmestelle(b, Kb, probe[b], ub, tb) for b in (0, 1)]
            kb = KetteS()
            kb.ausgeben(Kb, nb, "anna")
            yb = [rng.randrange(2) for _ in range(nb)]
            festlegen(kb, Kb, stb, yb, 1)
            koal = ({1, 2}, {3, 4}, {1, 4}, {2, 3})[runde]
            bekannt = ab4.wissen(koal)
            xb = messen_breidbart(ub, tb, rng)
            for i, (_, ti) in bekannt.items():
                xb[i] = ti
            soll(not (stb[0].pruefen(Kb, xb) and stb[1].pruefen(Kb, xb)),
                 f"Abschnitte: t-1 Validatoren {sorted(koal)} + Inhaberin an beiden Stellen durch")
        probe = stichproben(nb, 2, kv, rng)
        stb = [Annahmestelle(b, Kb, probe[b], ub, tb) for b in (0, 1)]
        kb = KetteS()
        kb.ausgeben(Kb, nb, "anna")
        festlegen(kb, Kb, stb, [rng.randrange(2) for _ in range(nb)], 1)
        soll(stb[0].pruefen(Kb, list(tb)) is True and stb[1].pruefen(Kb, list(tb)) is True,
             "Gegenprobe: wer den ganzen Token kennt, kommt an beiden Stellen durch")
        # 5e. Ein Validator meldet der ehrlichen Inhaberin falsche x auf seinem
        # Abschnitt: sie faellt durch (Stoerung, kein Faelschen), und die Fehler
        # liegen in SEINEM Abschnitt -- zuordenbar.
        K5 = hashlib.sha256(rng.randbytes(16)).hexdigest()
        a5 = AusgabeAbschnitte(K5, n, ts, nv, [random.Random(rng.randrange(1 << 62)) for _ in range(nv)])
        y5 = [rng.randrange(2) for _ in range(n)]
        x5 = a5.messen(y5, 0.0, rng)
        x5 = [1 - xi if i in set(a5.abschnitte[1]) else xi for i, xi in enumerate(x5)]
        pr5 = stichproben(n, 2, k, rng)
        g5 = {}
        for p_ in a5.praep:
            g5.update(p_.geheim())
        st5 = [Annahmestelle(b, K5, pr5[b], [g5[i][0] for i in range(n)], [g5[i][1] for i in range(n)]) for b in (0, 1)]
        k5 = KetteS()
        k5.ausgeben(K5, n, "anna")
        festlegen(k5, K5, st5, list(y5), 1)
        je = fehler_je_abschnitt(st5[1], K5, x5, a5.abschnitte)
        soll(st5[1].pruefen(K5, x5) is False, "Abschnitte: falsche x eines Validators nicht bemerkt")
        soll(je[1][1] == je[1][0] > 0 and all(fe == 0 for j, (_, fe) in enumerate(je) if j != 1),
             f"Abschnitte: Fehler nicht dem Validator 2 zuzuordnen: {je}")
        # 5f. Fail-closed: zweite Messung, fremde Stellen, falsche Zahl der Basen,
        # derselbe Zufall fuer zwei Validatoren.
        p6 = Praeparator(1, K, [0, 1, 2], 2, 3, random.Random(5))
        abgewiesen(lambda: p6.messen({0: 0, 1: 1}, 0.0, rng), "Basen fuer zu wenige Stellen")
        abgewiesen(lambda: p6.messen({0: 0, 1: 1, 2: 0, 3: 1}, 0.0, rng), "Basen fuer fremde Stellen")
        p6.messen({0: 0, 1: 1, 2: 0}, 0.0, rng)
        abgewiesen(lambda: p6.messen({0: 0, 1: 1, 2: 0}, 0.0, rng), "zweite Messung")
        abgewiesen(lambda: aa.messen(y[:-1], 0.0, rng), "Zahl der Basen")
        gleich = random.Random(7)
        try:
            AusgabeAbschnitte(K, n, ts, nv, [gleich] * nv)
            fehler.append("derselbe Zufall fuer alle Validatoren angenommen")
        except ValueError:
            pass
        # 5g. BEFUND fuer A2: Die Stichproben muessen geheim bleiben, bis die
        # Inhaberin ihre Basen gebunden hat. Kennt sie beide vorher, setzt sie d
        # so, dass an JEDER Stelle genau ihre gemessenen Basen geprueft werden
        # (d_i = y_i ^ c auf S_0, d_i = y_i ^ 1 ^ c auf S_1) -- und kommt an
        # beiden durch, ohne irgendein Geheimnis zu kennen. Heute zieht der
        # Ausgeber die Stichproben; dezentral muss das anders gehen (DEZENTRAL.md 8).
        K7 = hashlib.sha256(rng.randbytes(16)).hexdigest()
        u7, t7 = ausgabe(n, rng)
        y7, x7 = messen_ehrlich(u7, t7, rng, 0.0)
        pr7 = stichproben(n, 2, k, rng)
        st7 = [Annahmestelle(b, K7, pr7[b], u7, t7) for b in (0, 1)]
        k7 = KetteS()
        k7.ausgeben(K7, n, "anna")
        c7 = 0
        d7 = list(y7)
        for b in (0, 1):
            for i in pr7[b]:
                d7[i] = y7[i] ^ b ^ c7
        festlegen(k7, K7, st7, d7, c7)
        soll(st7[0].pruefen(K7, x7) is True and st7[1].pruefen(K7, x7) is True,
             "Befund: wer beide Stichproben vor der Bindung kennt, kommt an beiden Stellen durch")
        # 5h. LOESUNG fuer 5g: Stellen-Bit. Jeder Praeparator legt in seinem
        # Abschnitt je Stelle fest, zu welcher Annahmestelle sie gehoert (Bit im
        # Salz, je zur Haelfte), und oeffnet Stelle b nur ihre Stellen. Disjunkt
        # durch das Blatt, ohne dass jemand beide Stichproben kennt.
        K8 = hashlib.sha256(rng.randbytes(16)).hexdigest()
        a8 = AusgabeAbschnitte(K8, n, ts, nv, [random.Random(rng.randrange(1 << 62)) for _ in range(nv)])
        y8 = [rng.randrange(2) for _ in range(n)]
        x8 = a8.messen(y8, 0.0, rng)
        oe = [{}, {}]
        for p_ in a8.praep:
            for b in (0, 1):
                oe[b].update(p_.oeffnung(b))
        soll(not set(oe[0]) & set(oe[1]) and len(oe[0]) == len(oe[1]) == n // 2, "Stellen-Bit: nicht disjunkt oder nicht je zur Haelfte")
        st8 = [stelle_laden_bit(b, K8, n, a8.wurzel, a8.blaetter, ts, nv, oe[b], labor_k=k) for b in (0, 1)]
        k8 = KetteS()
        k8.ausgeben(K8, n, "anna")
        festlegen(k8, K8, st8, list(y8), 1)
        soll(st8[1].pruefen(K8, x8) is True and st8[0].pruefen(K8, x8) is False, "Stellen-Bit: Gutfall und Doppelvorlage")
        # Missbrauch: ein Praeparator schiebt Stelle 0 eine Stelle der anderen
        # unter; ein falscher Wert; Werte zurueckgehalten (zu klein); fremde Wurzel.
        fremd = dict(oe[0])
        i1 = next(iter(oe[1]))
        fremd[i1] = oe[1][i1]
        abgewiesen(lambda: stelle_laden_bit(0, K8, n, a8.wurzel, a8.blaetter, ts, nv, fremd, labor_k=k), "Stelle der anderen Annahmestelle")
        falsch8 = dict(oe[0])
        i0 = next(iter(falsch8))
        falsch8[i0] ^= 1
        abgewiesen(lambda: stelle_laden_bit(0, K8, n, a8.wurzel, a8.blaetter, ts, nv, falsch8, labor_k=k), "falscher Wert")
        weniger = dict(list(oe[0].items())[:-1])
        abgewiesen(lambda: stelle_laden_bit(0, K8, n, a8.wurzel, a8.blaetter, ts, nv, weniger, labor_k=k), "Werte zurueckgehalten")
        abgewiesen(lambda: stelle_laden_bit(0, K8, n, wk.h(b"fremd"), a8.blaetter, ts, nv, oe[0], labor_k=k), "fremde Wurzel")
        # Voller Umfang (k = 5.200 je Stelle, 3 von 4): t-1 Praeparatoren verraten
        # der Inhaberin ihre Abschnitte samt Stellen-Bits -- dort kennt sie die
        # Werte, wie immer geprueft wird --, den Rest misst sie nach Breidbart.
        # An der zweiten Stelle faellt sie trotzdem durch.
        K9 = hashlib.sha256(rng.randbytes(16)).hexdigest()
        a9 = AusgabeAbschnitte(K9, nb, ts, nv, [random.Random(rng.randrange(1 << 62)) for _ in range(nv)])
        oe9 = [{}, {}]
        for p_ in a9.praep:
            for b in (0, 1):
                oe9[b].update(p_.oeffnung(b))
        st9 = [stelle_laden_bit(b, K9, nb, a9.wurzel, a9.blaetter, ts, nv, oe9[b]) for b in (0, 1)]
        g9 = {}
        for p_ in a9.praep:
            g9.update(p_.geheim())
        u9, t9 = [g9[i][0] for i in range(nb)], [g9[i][1] for i in range(nb)]
        koal = {1, 2}
        bekannt9 = a9.wissen(koal)
        x9 = messen_breidbart(u9, t9, rng)
        for i, (_, ti) in bekannt9.items():
            x9[i] = ti
        k9 = KetteS()
        k9.ausgeben(K9, nb, "anna")
        festlegen(k9, K9, st9, [rng.randrange(2) for _ in range(nb)], 0)
        soll(not (st9[0].pruefen(K9, x9) and st9[1].pruefen(K9, x9)),
             "Stellen-Bit: t-1 Praeparatoren + Inhaberin an beiden Stellen durch")
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


def vektor_abschnitte() -> dict:
    """Pruefvektor fuer die Ausgabe in Abschnitten (A2, quanten_s_abschnitt_test.go):
    Grenzen der Abschnitte, gesalzene Werte mit Stellen-Bit, Blaetter, Wurzel und
    die Stichprobe jeder Annahmestelle -- alles deterministisch."""
    rng = random.Random(1926)
    n, n_val, t = 40, 4, 3
    K = hashlib.sha256(b"aequitas-quanten-s-abschnitt-vektor").hexdigest()
    aa = AusgabeAbschnitte(K, n, t, n_val, [random.Random(rng.randrange(1 << 62)) for _ in range(n_val)])
    werte = {}
    for p_ in aa.praep:
        werte.update(p_._v)
    oe = [{}, {}]
    for p_ in aa.praep:
        for b in (0, 1):
            oe[b].update(p_.oeffnung(b))
    return {
        "K": K, "n": n, "n_val": n_val, "t": t,
        "abschnitte": [[a[0], a[-1] + 1] for a in aa.abschnitte],
        "abschnitte_weitere": {f"{nn}/{vv}": [[a[0], a[-1] + 1] for a in abschnitte(nn, vv)]
                               for nn, vv in ((10, 3), (7, 7), (10400, 4), (3920, 10), (5, 1))},
        "werte": [str(werte[i]) for i in range(n)],
        "stellen_bit": [stellen_bit(werte[i]) for i in range(n)],
        "blaetter": [b.hex() for b in aa.blaetter],
        "wurzel": aa.wurzel.hex(),
        "stichprobe": [sorted(oe[b]) for b in (0, 1)],
    }


if __name__ == "__main__":
    if "--vektor-abschnitte" in sys.argv:
        import json
        print(json.dumps(vektor_abschnitte(), indent=1))
    elif "--vektor" in sys.argv:
        import json
        print(json.dumps(vektor(), indent=1))
    else:
        sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else print(__doc__)
