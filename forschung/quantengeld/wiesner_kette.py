#!/usr/bin/env python3
"""Variante W, Kettenseite (W2) als ausfuehrbare Spezifikation.

Forschung; der Knoten setzt genau diese Regeln um (quanten_w.go,
quanten_w_register.go). Keine Abhaengigkeiten. Wie bruecke_kette.py fuer die
oeffentlich pruefbare Note.

GRUNDIDEE: OFFENLEGEN STATT GEMEINSAM RECHNEN
  Beim Praegen steht fuer jede Stelle i der Note nur eine gesalzene
  Pruefsumme c_i = SHA-256(Domaene | K | i | v_i) auf der Kette, als
  Merkle-Wurzel. v_i = Basis + 2*Bit + 4*Salz (Salz 124 Bit) liegt nur als
  Shamir-Anteile (t von v, Koerper 2^127 - 1) bei den Validatoren.
  Eine Pruefung laeuft in vier Schritten:
    1. ANFRAGE (mit Pfand) des Anfragenden.
    2. ZUFALL: jeder Validator der Note legt den naechsten Wert seiner
       Hash-Kette offen (beim Praegen als Anker festgelegt). Sind alle da,
       folgen daraus Stellen und Basen -- vorher kennt sie niemand, und
       niemand kann sie waehlen: nicht der Anfragende, nicht der Erzeuger
       eines Blocks, kein einzelner Validator.
    3. ANTWORT des Anfragenden: die Messwerte.
    4. OFFENLEGEN: t Anteile je gepruefter Stelle plus Merkle-Pfad. JEDER
       Knoten setzt v_i zusammen, prueft die Pruefsumme und urteilt selbst.
  Die geprueften Stellen sind ohnehin verbraucht (gemessen); ihr Geheimnis
  offenzulegen verraet nichts ueber die anderen (eigenes Polynom je Stelle).
  So bleibt jede Pruefung bei jedem Knoten -- ohne verteilte Rechnung auf
  der Kette und ohne einem Validator zu glauben: falsche Anteile fallen an
  der Pruefsumme auf, falscher Zufall an der Hash-Kette.

ZUFALL OHNE STEUERUNG (W2b, ersetzt den Blockhash aus W2a)
  Jeder Validator x waehlt beim Praegen ein Geheimnis s_x und bestaetigt den
  Anker A_x = H^L(s_x) (L = ZUFALL_L). Fuer die Anfrage Nummer j legt er
  z_x(j) = H^(L-1-j)(s_x) offen; jeder Knoten prueft H^(j-m)(z) gegen den
  zuletzt bekannten Wert (Index m). Die Saat der Aufforderung ist
  SHA-256(Domaene | K | j | z_1 .. z_v). Alle Werte stehen seit dem Praegen
  fest: ein Erzeuger, der mehrere Bloecke versuchen koennte, aendert daran
  nichts (beim Blockhash konnte er waehlen). Fuer den Zufall gibt es KEINE
  Frist: die Anfrage bleibt offen und ihr Pfand gesperrt, bis er
  vollstaendig ist; die Antwortfrist laeuft erst ab dann. Mit einer Frist
  (Pfand zurueck, Stellen trotzdem verbraucht) haette ein Erzeuger, der einen
  Wert zurueckhaelt, eine Note kostenlos erschoepfen koennen: Einloesen
  anfragen, verfallen lassen, 64 Stellen weg, ohne Pfand.

REGELN (Missbrauch in Klammern)
  - Praegen nur mit Deckung und mit der Bestaetigung ALLER v Validatoren der
    Note (je mit ihrem Anker und der Praegezeit, hoechstens einen Tag alt),
    K neu und fuer immer belegt, k_note <= n <= N_MAX (tote Note mit alten
    Bestaetigungen neu praegen: ihr Zufall waere vorhersagbar).
  - SICHERE PRUEFGROESSE (W2c, 16.11.2026; ANALYSE.md „Befund Klonangriff“):
    jede Note hat ihre Toleranz tol (Prozent, aus TOLERANZEN; die
    Validatoren bestaetigen sie mit) und daraus ihre Pruefgroesse
    k_note = K_TABELLE[tol][f16], f16 = ceil(16 (t-1) / v): eine Absprache
    unter der Schwelle kennt hoechstens den Anteil (t-1)/v der Stellen. Die
    Tabelle rechnet wiesner_parameter.py exakt: ein Faelscher mit optimalem
    Kloner (14,6 % Fehler je Treffer) besteht hoechstens mit 2^-40. Ganze
    Zahlen, kein Gleitkomma im Konsens. 0 = unmoeglich (Praegen abgewiesen).
  - Eine offene Anfrage je Note; Pfand. Pfand = max(1 AEQ, Wert * k / n):
    wer Stellen verbraucht, setzt so viel ein, wie diese Stellen vom Wert
    der Note tragen (sonst: fremde Noten fuer 1 AEQ leerpruefen).
  - Pruefen: K_MIN <= k <= K_MAX (eine kleine Pruefung schuetzt nur den
    Pruefenden selbst schwaecher), und es bleiben k_note Stellen frei -- die
    Reserve fuer das Einloesen.
  - Einloesen: k = k_note, nicht waehlbar (sonst waehlt ein Faelscher ein
    kleines k).
  - Zufall: nur fuer die offene Anfrage, nur passende Hash-Kette, jeder
    Wert nur einmal (zu frueh, falscher Wert, Wiederholung); ohne Frist.
  - Antwort nur vom Anfragenden, erst nach dem Zufall, nur bis zur Frist
    (Wiederholung, fremde Antwort, Antwort vor der Aufforderung).
  - Offenlegen erst nach der Antwort; genau t Anteile mit verschiedenen
    x aus der Validatorenliste; Pruefsumme und Merkle-Pfad muessen passen
    (falsche Anteile, zu wenige, fremde x, falscher Pfad).
  - Urteil: Treffer = Stellen mit passender Basis. Weniger als MIN_TREFFER:
    UNENTSCHIEDEN -- Pfand zurueck, kein Fehlversuch, die Note bleibt (sonst
    faellt eine echte Note durch, nur weil zufaellig wenige Basen passten).
    Sonst angenommen, wenn Fehler * 100 <= tol * Treffer (tol der Note).
  - Offenlegen in Teilen zu hoechstens OFFEN_TEIL Stellen, der Reihe nach
    (ab = Zahl der schon offengelegten Stellen); geurteilt wird, wenn alle k
    da sind. Jeder Teil ganz oder gar nicht.
  - Bestanden: Pfand zurueck; beim Einloesen der Wert an das Ziel, die Note
    ist danach tot. Durchgefallen oder Antwortfrist verpasst -- beim Pruefen
    WIE beim Einloesen: Pfand ans Grundeinkommen, Fehlversuch, nach FEHL_MAX
    gesperrt (Einloesen geht weiter). Die Note stirbt NICHT (sonst toetet ein
    Fremder jede Note mit einem Einloese-Versuch).
  - Erschoepft: bleiben weniger als k_note freie Stellen, ist die Note tot,
    ihr Wert geht ans Grundeinkommen. Dahin kommt nur, wer Pfaender von
    zusammen etwa dem Wert der Note verloren hat.
  - Erhaltung: Summe aller Konten + Topf + Pfaender + Grundeinkommen bleibt
    gleich.

GRENZEN, EHRLICH
  - Der Zufall braucht ALLE v Validatoren der Note; faellt einer fuer
    immer aus, steht die Note (fail-closed: kein Wert geht verloren, aber
    keiner kommt heraus). Eine Schwellen-Signatur (t von v) waere
    lebendiger; das ist eine spaetere Stufe.
  - Unterschriften sind hier HMAC-Platzhalter; im Knoten EIP-191/ML-DSA.
  - Wer den Validatoren die Anteile gibt, regelt die Praegung
    (wiesner_praegen.py, W3).

Aufruf:  python3 -I wiesner_kette.py [--selbsttest]
"""
from __future__ import annotations

import hashlib
import hmac
import random
import sys

P = (1 << 127) - 1           # Mersenne-Primzahl, Koerper der Anteile
SALZ_BITS = 124
N_MAX = 16384                # Stellen je Note
K_MIN, K_MAX = 8, 8192       # Stellen je Pruefung (Einloesen: genau k_note)
OFFEN_TEIL = 64              # Stellen je Offenlegung (Teile der Reihe nach)
TOL_PROZENT = 4              # Standard-Toleranz einer Note (Prozent)
TOLERANZ = TOL_PROZENT / 100
MIN_TREFFER = 4
# K_TABELLE[tol][f16]: kleinste Pruefgroesse (Vielfaches von 8), mit der ein
# Faelscher mit optimalem Kloner -- Fehlerrate (1 - f16/16) * 14,64 % je
# Treffer -- hoechstens mit 2^-40 besteht (Regel der Kette, MIN_TREFFER 4,
# unentschieden darf er wiederholen). Exakt gerechnet mit
# wiesner_parameter.bedingt(); der Selbsttest prueft Stichproben nach.
# 0 = mit dieser Toleranz und Absprache nicht sicher (oder k > K_MAX).
K_TABELLE = {
    2: (552, 600, 672, 752, 856, 984, 1168, 1408, 1760),
    3: (672, 752, 856, 984, 1144, 1368, 1680, 2144, 2880),
    4: (840, 960, 1112, 1304, 1576, 1960, 2536, 3472, 5200),
    5: (1064, 1240, 1464, 1784, 2240, 2936, 4096, 6264, 0),
    6: (1360, 1632, 2000, 2536, 3360, 4752, 7416, 0, 0),
    8: (2440, 3144, 4256, 6184, 0, 0, 0, 0, 0),
    10: (5240, 7808, 0, 0, 0, 0, 0, 0, 0),
}
TOLERANZEN = tuple(K_TABELLE)
FEHL_MAX = 3
FRIST_ANTWORT = 600          # Sekunden nach dem vollstaendigen Zufall
FRIST_OFFENLEGEN = 1800      # Sekunden nach der Antwort
BLOCKZEIT = 5                # Sekunden (nur fuer warten() ohne Angabe)
PFAND = 1_000_000            # Mikro-AEQ (1 AEQ), kleinstes Pfand
PRAEGEN_VORAUS, PRAEGEN_ALT = 300, 86400  # Praegezeit hoechstens 5 min nach / 1 Tag vor dem Block
ZUFALL_L = 256               # Laenge der Hash-Kette je Validator = hoechstens Anfragen je Note
MIN_DECKUNG = 1_000_000
T_MAX, V_MAX = 16, 64
DOM_POS = b"aequitas-quanten-w-pos-v1|chain:1926|"
DOM_AUF = b"aequitas-quanten-w-aufforderung-v1|chain:1926|"
DOM_BEST = "aequitas-quanten-w-anteile-v3|chain:1926|"
DOM_ZK = b"aequitas-quanten-w-zufallskette-v1|"
DOM_ZUF = b"aequitas-quanten-w-zufall-v1|chain:1926|"


def h(*teile: bytes) -> bytes:
    return hashlib.sha256(b"".join(teile)).digest()


# ------------------------------------------------------------ Bausteine

def wert(basis: int, bit: int, salz: int) -> int:
    assert basis in (0, 1) and bit in (0, 1) and 0 <= salz < (1 << SALZ_BITS)
    return basis + 2 * bit + 4 * salz


def zerlegen(v: int) -> tuple[int, int]:
    return v & 1, (v >> 1) & 1


def pruefsumme(K: str, i: int, v: int) -> bytes:
    return h(DOM_POS, K.encode(), b"|", str(i).encode(), b"|", v.to_bytes(16, "big"))


def merkle_wurzel(blaetter: list[bytes]) -> bytes:
    ebene = list(blaetter)
    while len(ebene) > 1:
        if len(ebene) % 2:
            ebene.append(ebene[-1])
        ebene = [h(b"\x01", ebene[i], ebene[i + 1]) for i in range(0, len(ebene), 2)]
    return ebene[0]


def merkle_pfad(blaetter: list[bytes], i: int) -> list[bytes]:
    pfad, ebene = [], list(blaetter)
    while len(ebene) > 1:
        if len(ebene) % 2:
            ebene.append(ebene[-1])
        pfad.append(ebene[i ^ 1])
        ebene = [h(b"\x01", ebene[j], ebene[j + 1]) for j in range(0, len(ebene), 2)]
        i //= 2
    return pfad


def merkle_pruefen(wurzel: bytes, blatt: bytes, i: int, pfad: list[bytes], n: int) -> bool:
    tiefe = max(1, (n - 1).bit_length()) if n > 1 else 0
    if len(pfad) != tiefe:
        return False
    x = blatt
    for nachbar in pfad:
        x = h(b"\x01", x, nachbar) if i % 2 == 0 else h(b"\x01", nachbar, x)
        i //= 2
    return x == wurzel


def teilen(v: int, t: int, n_val: int, rng: random.Random) -> list[int]:
    koeff = [v] + [rng.randrange(P) for _ in range(t - 1)]
    out = []
    for x in range(1, n_val + 1):
        y = 0
        for c in reversed(koeff):
            y = (y * x + c) % P
        out.append(y)
    return out


def zusammensetzen(anteile: list[tuple[int, int]]) -> int:
    s = 0
    for xi, yi in anteile:
        z = n = 1
        for xj, _ in anteile:
            if xj != xi:
                z = z * (-xj) % P
                n = n * (xi - xj) % P
        s = (s + yi * z * pow(n, -1, P)) % P
    return s


def aufforderung(K: str, anfrage_id: str, saat: bytes, frei: list[int], k: int) -> tuple[list[int], list[int]]:
    """Stellen und Basen, deterministisch aus der Saat (zufall_saat)."""
    strom, zaehler, puffer = h(DOM_AUF, K.encode(), b"|", anfrage_id.encode(), b"|", saat), 0, b""

    def zahl(grenze: int) -> int:
        nonlocal zaehler, puffer
        while True:  # ohne Verzerrung: verwerfen ueber dem Vielfachen
            while len(puffer) < 8:
                puffer += h(strom, zaehler.to_bytes(8, "big"))
                zaehler += 1
            r, puffer = int.from_bytes(puffer[:8], "big"), puffer[8:]
            if r < (1 << 64) - ((1 << 64) % grenze):
                return r % grenze

    kand = sorted(frei)
    for j in range(k):  # Fisher-Yates, erste k
        m = j + zahl(len(kand) - j)
        kand[j], kand[m] = kand[m], kand[j]
    stellen = sorted(kand[:k])
    return stellen, [zahl(2) for _ in stellen]


def kette_vor(x: bytes, m: int) -> bytes:
    """m Schritte der Hash-Kette eines Validators."""
    for _ in range(m):
        x = h(DOM_ZK, x)
    return x


def kette_wert(geheim: bytes, j: int) -> bytes:
    """Was ein Validator fuer die Anfrage Nummer j offenlegt."""
    assert 0 <= j < ZUFALL_L
    return kette_vor(geheim, ZUFALL_L - 1 - j)


def kette_anker(geheim: bytes) -> bytes:
    return kette_vor(geheim, ZUFALL_L)


def zufall_saat(K: str, j: int, werte: list[bytes]) -> bytes:
    """Saat der Aufforderung j: die Werte aller Validatoren in ihrer Reihenfolge."""
    return h(DOM_ZUF, K.encode(), b"|", str(j).encode(), b"|", *werte)


def f16_von(t: int, v: int) -> int:
    """ceil(16 (t-1) / v): Anteil der Stellen (in Sechzehnteln), den eine
    Absprache unter der Schwelle hoechstens kennt."""
    return -(-16 * (t - 1) // v)


def k_note(tol: int, t: int, v: int) -> int:
    """Pruefgroesse der Note; 0 = nicht sicher moeglich."""
    f16 = f16_von(t, v)
    if tol not in K_TABELLE or not 0 <= f16 < len(K_TABELLE[tol]):
        return 0
    return K_TABELLE[tol][f16]


def bestaetigung_text(K: str, wurzel: bytes, n: int, t: int, validatoren: list[str], x: int, anker: bytes, zeit: int,
                      tol: int = TOL_PROZENT) -> str:
    """Was Validator x (1-basiert) beim Praegen unterschreibt: er haelt die
    Anteile zu dieser Wurzel, das ist sein Anker, und das ist die Praegezeit.
    Die Zeit macht alte Bestaetigungen nach einem Tag wertlos: eine tote Note
    laesst sich nicht mit denselben Ankern neu praegen (ihre Hash-Ketten
    waeren teils schon offen, der Zufall vorhersagbar)."""
    return f"{DOM_BEST}{K}|{wurzel.hex()}|{n}|{t}|{','.join(validatoren)}|{x}|{anker.hex()}|{zeit}|{tol}"


def pfand_fuer(wert_: int, k: int, n: int) -> int:
    """max(PFAND, ceil(Wert * k / n)), ganzzahlig."""
    return max(PFAND, -(-wert_ * k // n))


def reserve(e: dict) -> int:
    """So viele Stellen bleiben fuer das Einloesen frei: k_note der Note."""
    return e["k"]


# ------------------------------------------------------------ Kette

class Abgewiesen(Exception):
    pass


class KetteW:
    def __init__(self, validatoren: dict[str, bytes], labor_k: int | None = None):
        """labor_k: NUR Labor und Selbsttest der Ablaeufe -- ersetzt k_note
        durch eine kleine feste Zahl. Damit ist eine Note NICHT sicher; der
        Knoten kennt das nicht (nur einen Testschalter)."""
        self.labor_k = labor_k
        self.val_schluessel = dict(validatoren)    # Validator -> HMAC-Schluessel (Platzhalter)
        self.konten: dict[str, int] = {}
        self.konto_schluessel: dict[str, bytes] = {}
        self.topf = 0            # Deckung der Noten
        self.pfaender = 0        # offene Pfaender (im Knoten: eigener Topf)
        self.grundeinkommen = 0
        self.noten: dict[str, dict] = {}
        self.zeit = 0

    # -- Konten und Unterschriften (Platzhalter fuer EIP-191/ML-DSA)
    def konto(self, name: str, guthaben: int, schluessel: bytes):
        self.konten[name] = guthaben
        self.konto_schluessel[name] = schluessel

    @staticmethod
    def sig(schluessel: bytes, text: str) -> str:
        return hmac.new(schluessel, text.encode(), hashlib.sha256).hexdigest()

    def _sig_ok(self, schluessel: bytes | None, text: str, sig: str) -> bool:
        return schluessel is not None and hmac.compare_digest(self.sig(schluessel, text), sig)

    def summe(self) -> int:
        return sum(self.konten.values()) + self.topf + self.pfaender + self.grundeinkommen

    @staticmethod
    def frei(e: dict) -> list[int]:
        return [i for i in range(e["n"]) if i not in e["benutzt"]]

    def warten(self, sek: int = BLOCKZEIT):
        """Zeit vergeht; abgelaufene Fristen enden (im Knoten: vor jeder
        Transaktion an der Note und mit quanten_w_frist)."""
        self.zeit += sek
        for e in self.noten.values():
            self._frist(e)

    def _frist(self, e: dict):
        a = e["anfrage"]
        if a is None:
            return
        if a["stellen"] is None:
            return  # Zufall unvollstaendig: keine Frist (siehe oben)
        if a["antwort"] is None:
            if self.zeit > a["gezogen"] + FRIST_ANTWORT:
                self._durchgefallen(e)
        elif self.zeit > a["antwort_zeit"] + FRIST_OFFENLEGEN:
            # Die Validatoren haben nicht offengelegt: Pfand zurueck,
            # Stellen bleiben verbraucht.
            self._pfand_zurueck(a)
            e["anfrage"] = None
            self._erschoepft(e)

    def _pfand_zurueck(self, a: dict):
        self.pfaender -= a["pfand"]
        self.konten[a["von"]] += a["pfand"]

    def _durchgefallen(self, e: dict):
        a = e["anfrage"]
        self.pfaender -= a["pfand"]
        self.grundeinkommen += a["pfand"]
        e["fehl"] += 1
        if e["fehl"] >= FEHL_MAX:
            e["gesperrt"] = True
        e["anfrage"] = None
        self._erschoepft(e)

    def _erschoepft(self, e: dict):
        # Erschoepft: kein Einloesen mehr moeglich -- zu wenige freie Stellen
        # oder die Hash-Ketten der Validatoren sind verbraucht.
        if not e["tot"] and e["anfrage"] is None and (len(self.frei(e)) < e["k"] or e["zaehl"] >= ZUFALL_L):
            self.grundeinkommen += e["wert"]
            self.topf -= e["wert"]
            e["tot"] = True

    # -- 1. Praegen
    def praegen(self, von: str, K: str, n: int, wurzel: bytes, deckung: int,
                validatoren: list[str], t: int, anker: list[bytes], bestaetigungen: dict[str, str], zeit: int = 0,
                tol: int = TOL_PROZENT):
        # K bleibt fuer immer belegt, auch wenn die Note tot ist (im Knoten:
        # tot und im Fenster der Praegezeit gemerkt -- danach gilt keine
        # alte Bestaetigung mehr).
        if K in self.noten or len(K) != 64 or any(c not in "0123456789abcdef" for c in K):
            raise Abgewiesen("K ungueltig oder schon da")
        if not self.zeit - PRAEGEN_ALT <= zeit <= self.zeit + PRAEGEN_VORAUS:
            raise Abgewiesen("Praegezeit ausserhalb des Fensters")
        if not 1 <= n <= N_MAX or not 1 <= t <= T_MAX or not t <= len(validatoren) <= V_MAX:
            raise Abgewiesen("Groessen")
        # Sichere Pruefgroesse: aus Toleranz und Absprache-Grenze; n >= k,
        # sonst liesse sich die Note nie einloesen (Deckung fuer immer im Topf).
        kn = self.labor_k if self.labor_k is not None else k_note(tol, t, len(validatoren))
        if tol not in TOLERANZEN or kn <= 0 or not K_MIN <= kn <= K_MAX or n < kn:
            raise Abgewiesen(f"Toleranz {tol} %: Pruefgroesse {kn} (n = {n})")
        if len(set(validatoren)) != len(validatoren) or any(v not in self.val_schluessel for v in validatoren):
            raise Abgewiesen("Validatorenliste")
        if len(anker) != len(validatoren) or any(len(a) != 32 for a in anker):
            raise Abgewiesen("Anker")
        if deckung < MIN_DECKUNG or self.konten.get(von, 0) < deckung:
            raise Abgewiesen("Deckung")
        # ALLE Validatoren bestaetigen, jeder mit seinem Anker: ohne den Anker
        # eines Validators gaebe es fuer ihn keinen Zufall.
        for x, v in enumerate(validatoren, 1):
            if not self._sig_ok(self.val_schluessel[v], bestaetigung_text(K, wurzel, n, t, validatoren, x, anker[x - 1], zeit, tol),
                                bestaetigungen.get(v, "")):
                raise Abgewiesen("Bestaetigung der Validatoren fehlt oder passt nicht")
        self.konten[von] -= deckung
        self.topf += deckung
        self.noten[K] = {"n": n, "wurzel": wurzel, "wert": deckung, "validatoren": list(validatoren), "t": t,
                         "benutzt": set(), "fehl": 0, "gesperrt": False, "tot": False, "anfrage": None, "zaehl": 0,
                         "z": list(anker), "z_index": [-1] * len(validatoren), "tol": tol, "k": kn}

    # -- 2. Anfrage
    def anfrage(self, von: str, K: str, k: int, sig: str, einloesen_an: str | None = None) -> int:
        e = self.noten.get(K)
        if e is None or e["tot"]:
            raise Abgewiesen("Note unbekannt oder tot")
        self._frist(e)
        if e["anfrage"] is not None:
            raise Abgewiesen("schon eine offene Anfrage")
        if einloesen_an is None and e["gesperrt"]:
            raise Abgewiesen("Note gesperrt (Einloesen geht noch)")
        if e["zaehl"] >= ZUFALL_L:
            raise Abgewiesen("Hash-Ketten der Validatoren verbraucht")
        if not self._sig_ok(self.konto_schluessel.get(von), f"anfrage|{K}|{k}|{einloesen_an}|{e['zaehl']}", sig):
            raise Abgewiesen("Unterschrift der Anfrage")
        frei = len(self.frei(e))
        if einloesen_an is not None:
            if k != e["k"] or frei < k:
                raise Abgewiesen("k beim Einloesen fest: k_note")
            if einloesen_an not in self.konten:
                raise Abgewiesen("Ziel")
        elif not K_MIN <= k <= K_MAX or frei - k < reserve(e) or e["zaehl"] >= ZUFALL_L - 1:
            # Reserve fuer das Einloesen: k_note Stellen UND der letzte Wert
            # der Hash-Ketten (sonst pruefte jemand die Ketten leer).
            raise Abgewiesen("k (Reserve fuer das Einloesen)")
        pfand = pfand_fuer(e["wert"], k, e["n"])
        if self.konten.get(von, 0) < pfand:
            raise Abgewiesen("Pfand")
        self.konten[von] -= pfand
        self.pfaender += pfand
        j = e["zaehl"]
        e["zaehl"] += 1
        e["anfrage"] = {"von": von, "index": j, "k": k, "stellen": None, "basen": None, "pfand": pfand,
                        "zeit": self.zeit, "gezogen": None, "antwort": None, "antwort_zeit": None,
                        "einloesen_an": einloesen_an, "offen": 0, "treffer": 0, "fehler": 0}
        return j

    # -- 3. Zufall (jeder darf einreichen; die Hash-Kette beweist die Herkunft)
    def zufall(self, K: str, j: int, werte: dict[int, bytes]) -> bool:
        """Werte der Hash-Ketten (Validator-Index 1-basiert -> Wert) fuer die
        offene Anfrage j. True, sobald alle da sind: dann steht die
        Aufforderung, und ihre Stellen sind verbraucht."""
        e = self.noten.get(K)
        if e is None or e["tot"]:
            raise Abgewiesen("Note unbekannt oder tot")
        self._frist(e)
        a = e["anfrage"]
        if a is None or a["stellen"] is not None:
            raise Abgewiesen("kein Zufall faellig")
        if j != a["index"] or not werte:
            raise Abgewiesen("Zufall fuer eine andere Anfrage")
        z_neu, m_neu = list(e["z"]), list(e["z_index"])  # alles oder nichts
        for x, z in werte.items():
            if not 1 <= x <= len(e["validatoren"]) or len(z) != 32:
                raise Abgewiesen("Zufall: Validator oder Form")
            m = m_neu[x - 1]
            if m >= j or kette_vor(z, j - m) != z_neu[x - 1]:
                raise Abgewiesen("Zufall: passt nicht zur Hash-Kette oder schon da")
            z_neu[x - 1], m_neu[x - 1] = z, j
        e["z"], e["z_index"] = z_neu, m_neu
        if any(m < j for m in e["z_index"]):
            return False
        saat = zufall_saat(K, j, e["z"])
        a["stellen"], a["basen"] = aufforderung(K, f"{K}:{j}", saat, self.frei(e), a["k"])
        e["benutzt"].update(a["stellen"])  # ab jetzt verbraucht, auch ohne Antwort
        a["gezogen"] = self.zeit
        return True

    def aufforderung_von(self, K: str) -> tuple[list[int], list[int]]:
        """Die gezogene Aufforderung der offenen Anfrage (oeffentlich)."""
        e = self.noten.get(K)
        a = e and e["anfrage"]
        if not a or a["stellen"] is None:
            raise Abgewiesen("Aufforderung steht noch aus")
        return list(a["stellen"]), list(a["basen"])

    # -- 4. Antwort
    def antwort(self, von: str, K: str, bits: list[int], sig: str):
        e = self.noten.get(K)
        if e is not None:
            self._frist(e)
        a = e and e["anfrage"]
        if not a or a["antwort"] is not None:
            raise Abgewiesen("keine offene Anfrage")
        if von != a["von"] or not self._sig_ok(self.konto_schluessel.get(von), f"antwort|{K}|{a['index']}|{bits}", sig):
            raise Abgewiesen("nicht vom Anfragenden")
        if a["stellen"] is None:
            raise Abgewiesen("Aufforderung steht noch aus")
        if len(bits) != len(a["stellen"]) or any(b not in (0, 1) for b in bits):
            raise Abgewiesen("Form der Antwort")
        a["antwort"], a["antwort_zeit"] = list(bits), self.zeit

    # -- 5. Offenlegen und Urteil (jeder Knoten), in Teilen
    def offenlegen(self, K: str, je_stelle: list[tuple[list[tuple[int, int]], list[bytes]]], ab: int = 0):
        """Offenlegung der Stellen a["stellen"][ab : ab + len(je_stelle)].
        ab muss genau die Zahl der schon offengelegten Stellen sein. Jeder
        Teil ganz oder gar nicht. Rueckgabe: "teil", solange Stellen fehlen;
        dann True (bestanden), False (durchgefallen), None (unentschieden)."""
        e = self.noten.get(K)
        if e is not None:
            self._frist(e)
        a = e and e["anfrage"]
        if not a or a["antwort"] is None:
            raise Abgewiesen("erst die Antwort")
        if ab != a["offen"] or not 1 <= len(je_stelle) <= OFFEN_TEIL or ab + len(je_stelle) > len(a["stellen"]):
            raise Abgewiesen("Teil: Beginn oder Laenge")
        treffer = fehler = 0
        for j, (anteile, pfad) in enumerate(je_stelle, ab):
            i, beta, o = a["stellen"][j], a["basen"][j], a["antwort"][j]
            xs = [x for x, _ in anteile]
            if len(anteile) != e["t"] or len(set(xs)) != len(xs) or any(not 1 <= x <= len(e["validatoren"]) for x in xs):
                raise Abgewiesen("Anteile: Zahl oder x")
            if any(not 0 <= y < P for _, y in anteile):
                raise Abgewiesen("Anteile: Wert")
            v = zusammensetzen(anteile)
            if v >> (SALZ_BITS + 2):
                raise Abgewiesen("zusammengesetzter Wert zu gross")
            if not merkle_pruefen(e["wurzel"], pruefsumme(K, i, v), i, pfad, e["n"]):
                raise Abgewiesen("Pruefsumme oder Pfad passt nicht")
            basis, bit = zerlegen(v)
            if basis == beta:
                treffer += 1
                fehler += o != bit
        a["offen"] += len(je_stelle)
        a["treffer"] += treffer
        a["fehler"] += fehler
        if a["offen"] < len(a["stellen"]):
            return "teil"
        treffer, fehler = a["treffer"], a["fehler"]
        if treffer < MIN_TREFFER:
            # Unentschieden: zu wenige passende Basen, um zu urteilen.
            self._pfand_zurueck(a)
            e["anfrage"] = None
            self._erschoepft(e)
            return None
        if fehler * 100 > e["tol"] * treffer:
            self._durchgefallen(e)
            return False
        self._pfand_zurueck(a)
        e["anfrage"] = None
        if a["einloesen_an"] is not None:
            self.konten[a["einloesen_an"]] += e["wert"]
            self.topf -= e["wert"]
            e["tot"] = True
        else:
            self._erschoepft(e)
        return True

    def offenlegen_alle(self, K: str, je_stelle):
        """Bequemlichkeit: alle Stellen in Teilen zu OFFEN_TEIL."""
        r = None
        for ab in range(0, len(je_stelle), OFFEN_TEIL):
            r = self.offenlegen(K, je_stelle[ab:ab + OFFEN_TEIL], ab)
        return r


# ------------------------------------------------------------ Praegestelle (Labor, W4)

class Praegung:
    """Was die Praegestelle erzeugt: Geheimnis je Stelle, Pruefsummen,
    Anteile je Validator. In W3 (wiesner_praegen.py) entsteht das Geheimnis
    ohne dass eine Stelle es ganz kennt."""

    def __init__(self, K: str, basen: list[int], bits: list[int], t: int, validatoren: list[str], rng: random.Random):
        self.K, self.n, self.t, self.validatoren = K, len(basen), t, validatoren
        self.v = [wert(b, x, rng.randrange(1 << SALZ_BITS)) for b, x in zip(basen, bits)]
        self.blaetter = [pruefsumme(K, i, v) for i, v in enumerate(self.v)]
        self.wurzel = merkle_wurzel(self.blaetter)
        self.anteile = [teilen(v, t, len(validatoren), rng) for v in self.v]  # je Stelle, je Validator
        # Je Validator das Geheimnis seiner Hash-Kette (im Betrieb waehlt es
        # jeder Validator selbst; hier simuliert) und der Anker daraus.
        self.geheim = [h(b"zufall", rng.randbytes(32)) for _ in validatoren]
        self.anker = [kette_anker(g) for g in self.geheim]

    def bestaetigungen(self, schluessel: dict[str, bytes], zeit: int = 0, tol: int = TOL_PROZENT) -> dict[str, str]:
        """Jeder Validator bestaetigt Wurzel, seinen Anker, die Praegezeit und die Toleranz."""
        return {v: KetteW.sig(schluessel[v], bestaetigung_text(self.K, self.wurzel, self.n, self.t, self.validatoren, x, self.anker[x - 1], zeit, tol))
                for x, v in enumerate(self.validatoren, 1)}

    def zufall(self, j: int, wer: list[int] | None = None) -> dict[int, bytes]:
        """Die Werte der Hash-Ketten fuer Anfrage j (Validatoren 1-basiert)."""
        return {x: kette_wert(self.geheim[x - 1], j) for x in (wer or range(1, len(self.validatoren) + 1))}

    def offenlegung(self, stellen: list[int], wer: list[int]) -> list[tuple[list[tuple[int, int]], list[bytes]]]:
        """Die Validatoren mit Index (1-basiert) in `wer` legen ihre Anteile offen."""
        return [([(x, self.anteile[i][x - 1]) for x in wer], merkle_pfad(self.blaetter, i)) for i in stellen]


# ------------------------------------------------------------ Selbsttest

def selbsttest() -> bool:
    fehler: list[str] = []
    try:
        _selbsttest(fehler)
    except Exception as e:  # ein Absturz ist ein Fehler; die Befunde davor bleiben
        fehler.append(f"Ausnahme: {type(e).__name__}: {e}")
    for f in fehler:
        print("FEHLER:", f)
    print("wiesner_kette: Selbsttest", "bestanden" if not fehler else "NICHT bestanden")
    return not fehler


def _selbsttest(fehler: list[str]) -> None:
    rng = random.Random(1926)

    def soll(bed, text):
        if not bed:
            fehler.append(text)

    def abgewiesen(f, text):
        try:
            f()
            fehler.append(f"angenommen: {text}")
        except Abgewiesen:
            pass

    # Merkle: Pfade fuer jede Groesse und jede Stelle; falscher Pfad faellt auf.
    for n in (1, 2, 3, 7, 8, 33):
        bl = [h(bytes([i])) for i in range(n)]
        w = merkle_wurzel(bl)
        for i in range(n):
            soll(merkle_pruefen(w, bl[i], i, merkle_pfad(bl, i), n), f"Merkle n={n} i={i}")
        if n > 1:
            soll(not merkle_pruefen(w, bl[0], 1, merkle_pfad(bl, 0), n), f"Merkle falsche Stelle n={n}")
    # Aufforderung: deterministisch, ohne Wiederholung, haengt an der Saat.
    s1 = aufforderung("a" * 64, "x", b"B1", list(range(100)), 16)
    soll(s1 == aufforderung("a" * 64, "x", b"B1", list(range(100)), 16), "Aufforderung nicht deterministisch")
    soll(len(set(s1[0])) == 16 and s1 != aufforderung("a" * 64, "x", b"B2", list(range(100)), 16), "Aufforderung")
    # Hash-Kette: jeder Wert fuehrt zum Anker, ein Nachbarwert nicht.
    g = h(b"g")
    soll(all(kette_vor(kette_wert(g, j), j + 1) == kette_anker(g) for j in (0, 1, 7, ZUFALL_L - 1)), "Hash-Kette")
    soll(kette_vor(kette_wert(g, 5), 3) == kette_wert(g, 2) and kette_vor(kette_wert(g, 5), 5) != kette_anker(g), "Hash-Kette Abstand")
    soll(pfand_fuer(5_000_000, 64, 200) == 1_600_000 and pfand_fuer(5_000_000, 8, 2048) == PFAND
         and pfand_fuer(10, 1, 3) == PFAND and pfand_fuer(3_000_001, 1, 3) == 1_000_001, "Pfand")

    VAL = {f"v{j}": bytes([j]) * 16 for j in range(1, 6)}
    vliste = list(VAL)

    # Die Ablaeufe pruefen wir mit kleiner Pruefgroesse (labor_k = 64); die
    # echten Groessen (K_TABELLE) weiter unten.
    LK = 64

    def aufbau(n=200, t=3, wert_=5_000_000):
        k = KetteW(VAL, labor_k=LK)
        for name in ("anna", "bert", "fremd"):
            k.konto(name, 20_000_000, name.encode() * 4)
        K = hashlib.sha256(rng.randbytes(16)).hexdigest()
        basen = [rng.randrange(2) for _ in range(n)]
        bits = [rng.randrange(2) for _ in range(n)]
        pr = Praegung(K, basen, bits, t, vliste, rng)
        k.praegen("anna", K, n, pr.wurzel, wert_, vliste, t, pr.anker, pr.bestaetigungen(VAL))
        return k, K, pr, basen, bits

    def ehrlich(basen, bits, stellen, ch):  # Halter mit echter Note, ohne Rauschen
        return [bits[i] if beta == basen[i] else rng.randrange(2) for i, beta in zip(stellen, ch)]

    def sg(k, wer, text):
        return KetteW.sig(k.konto_schluessel[wer], text)

    def anfragen(k, K, wer, k_stellen=16, einloesen=None):
        e = k.noten[K]
        if einloesen is not None:
            k_stellen = e["k"]
        return k.anfrage(wer, K, k_stellen, sg(k, wer, f"anfrage|{K}|{k_stellen}|{einloesen}|{e['zaehl']}"), einloesen)

    def runde(k, K, pr, wer, antwort_fn, k_stellen=16, einloesen=None, wer_x=(1, 2, 3)):
        j = anfragen(k, K, wer, k_stellen, einloesen)
        k.warten()
        k.zufall(K, j, pr.zufall(j))
        st, ch = k.aufforderung_von(K)
        bits = antwort_fn(st, ch)
        k.antwort(wer, K, bits, sg(k, wer, f"antwort|{K}|{j}|{bits}"))
        k.warten()
        return k.offenlegen(K, pr.offenlegung(st, list(wer_x)))

    # Gutfall: praegen, pruefen (Weitergabe), einloesen; Erhaltung.
    k, K, pr, basen, bits = aufbau()
    summe0 = k.summe()
    soll(runde(k, K, pr, "bert", lambda s, c: ehrlich(basen, bits, s, c)), "Gutfall: Pruefung")
    soll(k.konten["bert"] == 20_000_000 and k.pfaender == 0, "Pfand nicht zurueck")
    soll(runde(k, K, pr, "bert", lambda s, c: ehrlich(basen, bits, s, c), einloesen="bert", wer_x=(2, 4, 5)), "Gutfall: Einloesen")
    soll(k.konten["bert"] == 25_000_000 and k.topf == 0 and k.noten[K]["tot"], "Einloesen bucht falsch")
    soll(k.summe() == summe0, "Erhaltung")
    abgewiesen(lambda: anfragen(k, K, "bert", einloesen="bert"), "zweimal einloesen")

    # Faelschung ohne Note: Zufallsantworten -> durchgefallen, Pfand weg, nach
    # FEHL_MAX gesperrt. Mit k = 64 (bei k = 16 besteht Raten in ~3 %).
    k, K, pr, basen, bits = aufbau(n=400)
    for j in range(FEHL_MAX):
        soll(runde(k, K, pr, "fremd", lambda s, c: [rng.randrange(2) for _ in s], k_stellen=LK) is False, f"Faelschung {j} bestanden")
    soll(k.noten[K]["gesperrt"] and k.konten["fremd"] == 20_000_000 - FEHL_MAX * PFAND, "Sperre/Pfand")
    abgewiesen(lambda: anfragen(k, K, "bert", 16), "Pruefung einer gesperrten Note")
    # A7: gesperrt, der echte Halter loest trotzdem ein -- genau einmal.
    soll(runde(k, K, pr, "anna", lambda s, c: ehrlich(basen, bits, s, c), einloesen="anna"), "A7: echter Halter")

    # Einloesen mit Unsinn toetet die Note NICHT: Pfand (anteilig am Wert)
    # ans Grundeinkommen, Fehlversuch; der echte Halter loest danach ein.
    k, K, pr, basen, bits = aufbau()
    summe0, ge0 = k.summe(), k.grundeinkommen
    p = pfand_fuer(5_000_000, LK, 200)
    soll(runde(k, K, pr, "fremd", lambda s, c: [rng.randrange(2) for _ in s], einloesen="fremd") is False, "Faelscher loest ein")
    e = k.noten[K]
    soll(not e["tot"] and e["fehl"] == 1 and k.grundeinkommen == ge0 + p and k.konten["fremd"] == 20_000_000 - p
         and k.topf == 5_000_000, "Faelscher-Einloesen: Note tot oder Pfand falsch")
    soll(runde(k, K, pr, "anna", lambda s, c: ehrlich(basen, bits, s, c), einloesen="anna"), "Halter nach Faelscher-Versuch")
    soll(k.summe() == summe0, "Erhaltung (Faelscher)")
    # Ein Faelscher kann die Pruefung nicht verkleinern.
    k, K, pr, basen, bits = aufbau()
    abgewiesen(lambda: k.anfrage("fremd", K, 8, sg(k, "fremd", f"anfrage|{K}|8|fremd|0"), "fremd"), "Einloesen mit k = 8")
    abgewiesen(lambda: k.anfrage("fremd", K, LK + 1, sg(k, "fremd", f"anfrage|{K}|{LK + 1}|fremd|0"), "fremd"), "Einloesen mit k > k_note")
    abgewiesen(lambda: k.anfrage("bert", K, K_MAX + 1, sg(k, "bert", f"anfrage|{K}|{K_MAX + 1}|None|0")), "Pruefung mit k > K_MAX")
    # Pruefen laesst die Reserve stehen: 200 Stellen, 64 Reserve.
    abgewiesen(lambda: k.anfrage("bert", K, 137, sg(k, "bert", f"anfrage|{K}|137|None|0")), "Pruefung frisst die Reserve (200)")
    k2, K2, pr2, b2, x2 = aufbau(n=72)
    abgewiesen(lambda: k2.anfrage("bert", K2, 9, sg(k2, "bert", f"anfrage|{K2}|9|None|0")), "Pruefung frisst die Reserve")

    # Erschoepfen kostet etwa den Wert: 72 Stellen; eine Pruefung (8), dann
    # ein falsches Einloesen (64) -> keine Stelle frei, Note tot, Wert ans
    # Grundeinkommen; der Stoerer hat dafuer Pfaender verloren.
    summe0, ge0 = k2.summe(), k2.grundeinkommen
    soll(runde(k2, K2, pr2, "fremd", lambda s, c: [rng.randrange(2) for _ in s], k_stellen=8) is not True, "Stoerer besteht")
    verlust0 = 20_000_000 - k2.konten["fremd"]
    soll(runde(k2, K2, pr2, "fremd", lambda s, c: [rng.randrange(2) for _ in s], einloesen="fremd") is False, "Stoerer loest ein")
    e2 = k2.noten[K2]
    soll(e2["tot"] and len(KetteW.frei(e2)) == 0 and k2.topf == 0 and k2.summe() == summe0, "erschoepft: nicht tot oder Wert falsch")
    soll(20_000_000 - k2.konten["fremd"] - verlust0 == pfand_fuer(5_000_000, LK, 72) >= 4_000_000, "Stoerer zahlt zu wenig")
    abgewiesen(lambda: anfragen(k2, K2, "anna", einloesen="anna"), "Anfrage an erschoepfte Note")

    # Zufall: zu frueh, falscher Wert, fremde Anfrage, Wiederholung; die
    # Aufforderung entsteht erst, wenn ALLE Validatoren offengelegt haben.
    k, K, pr, basen, bits = aufbau()
    e = k.noten[K]
    abgewiesen(lambda: k.zufall(K, 0, pr.zufall(0)), "Zufall vor der Anfrage")
    j = anfragen(k, K, "anna", 16)
    abgewiesen(lambda: k.zufall(K, 1, pr.zufall(1)), "Zufall fuer eine andere Anfrage")
    abgewiesen(lambda: k.zufall(K, 0, {1: kette_wert(pr.geheim[1], 0)}), "Wert eines anderen Validators")
    abgewiesen(lambda: k.zufall(K, 0, {1: kette_wert(pr.geheim[0], 1)}), "Wert der falschen Stelle der Kette")
    abgewiesen(lambda: k.zufall(K, 0, {6: kette_wert(pr.geheim[0], 0)}), "fremder Validator")
    abgewiesen(lambda: k.zufall(K, 0, {1: pr.zufall(0)[1], 2: b"x" * 32}), "ein falscher Wert unter richtigen")
    soll(e["z_index"] == [-1] * 5, "abgewiesener Zufall hat etwas geaendert")
    soll(k.zufall(K, 0, pr.zufall(0, [1, 2, 3, 4])) is False and e["anfrage"]["stellen"] is None and not e["benutzt"], "Teil-Zufall zieht")
    abgewiesen(lambda: k.zufall(K, 0, pr.zufall(0, [1])), "Zufall wiederholt")
    abgewiesen(lambda: k.antwort("anna", K, [0] * 16, sg(k, "anna", f"antwort|{K}|0|{[0] * 16}")), "Antwort vor dem Zufall")
    soll(k.zufall(K, 0, pr.zufall(0, [5])) is True, "voller Zufall zieht nicht")
    st, ch = k.aufforderung_von(K)
    soll((st, ch) == aufforderung(K, f"{K}:0", zufall_saat(K, 0, [kette_wert(g_, 0) for g_ in pr.geheim]), list(range(200)), 16),
         "Aufforderung nicht aus den Hash-Ketten")
    soll(set(st) <= e["benutzt"], "gezogene Stellen nicht verbraucht")
    abgewiesen(lambda: k.zufall(K, 0, pr.zufall(0)), "Zufall nach dem Ziehen")

    # Missbrauch der Ablaeufe.
    abgewiesen(lambda: anfragen(k, K, "bert", 16), "zweite offene Anfrage")
    ant = ehrlich(basen, bits, st, ch)
    abgewiesen(lambda: k.offenlegen(K, pr.offenlegung(st, [1, 2, 3])), "Offenlegen vor der Antwort")
    abgewiesen(lambda: k.antwort("fremd", K, ant, sg(k, "fremd", f"antwort|{K}|0|{ant}")), "fremde Antwort")
    abgewiesen(lambda: k.antwort("anna", K, ant, sg(k, "fremd", f"antwort|{K}|0|{ant}")), "falsche Unterschrift")
    abgewiesen(lambda: k.antwort("anna", K, ant, sg(k, "anna", f"antwort|{K}|1|{ant}")), "Unterschrift einer anderen Anfrage")
    abgewiesen(lambda: k.antwort("anna", K, ant[:-1], sg(k, "anna", f"antwort|{K}|0|{ant[:-1]}")), "kurze Antwort")
    k.antwort("anna", K, ant, sg(k, "anna", f"antwort|{K}|0|{ant}"))
    abgewiesen(lambda: k.antwort("anna", K, ant, sg(k, "anna", f"antwort|{K}|0|{ant}")), "Antwort wiederholt")
    k.warten()
    of = pr.offenlegung(st, [1, 2, 3])
    falsch = [([(x, (y + 1) % P) if j_ == 0 else (x, y) for j_, (x, y) in enumerate(a_)], p_) for a_, p_ in of]
    abgewiesen(lambda: k.offenlegen(K, falsch), "falscher Anteil")
    abgewiesen(lambda: k.offenlegen(K, [(a_[:2], p_) for a_, p_ in of]), "zu wenige Anteile")
    abgewiesen(lambda: k.offenlegen(K, [([a_[0], a_[0], a_[1]], p_) for a_, p_ in of]), "doppeltes x")
    abgewiesen(lambda: k.offenlegen(K, [([(9, a_[0][1])] + a_[1:], p_) for a_, p_ in of]), "fremdes x")
    abgewiesen(lambda: k.offenlegen(K, [(a_, p_[::-1]) for a_, p_ in of]), "falscher Pfad")
    abgewiesen(lambda: k.offenlegen(K, of[1:]), "Stelle uebersprungen (Teil verschoben)")
    abgewiesen(lambda: k.offenlegen(K, of[1:], 1), "Teil vor dem ersten")
    soll(k.offenlegen(K, of[:-1]) == "teil" and k.noten[K]["anfrage"]["offen"] == len(of) - 1, "Teil")
    abgewiesen(lambda: k.offenlegen(K, of[-1:], 0), "Teil wiederholt")
    soll(k.offenlegen(K, of[-1:], len(of) - 1), "gueltige Offenlegung nach den Angriffen")

    # Zufall zurueckhalten: KEINE Frist -- die Anfrage bleibt offen, das Pfand
    # gesperrt, nichts verbraucht, keine neue Anfrage. Nachgereicht: die
    # Aufforderung steht, die Antwortfrist laeuft erst ab dann. (Mit einer
    # Frist haette ein Erzeuger eine Note kostenlos erschoepfen koennen.)
    k, K, pr, basen, bits = aufbau()
    e = k.noten[K]
    v0 = k.konten["anna"]
    j = anfragen(k, K, "anna", 16)
    k.zufall(K, j, pr.zufall(j, [1, 2, 3, 4]))
    k.warten(30 * 86400)
    soll(e["anfrage"] is not None and e["fehl"] == 0 and k.konten["anna"] == v0 - PFAND and not e["benutzt"], "Zufall zurueckgehalten")
    abgewiesen(lambda: anfragen(k, K, "bert", 16), "neue Anfrage, waehrend der Zufall aussteht")
    abgewiesen(lambda: anfragen(k, K, "bert", einloesen="bert"), "Einloesen, waehrend der Zufall aussteht")
    soll(k.zufall(K, 0, pr.zufall(0, [5])) and e["anfrage"]["gezogen"] == k.zeit, "nachgereichter Zufall")
    k.warten(FRIST_ANTWORT)
    st, ch = k.aufforderung_von(K)
    ant = ehrlich(basen, bits, st, ch)
    k.antwort("anna", K, ant, sg(k, "anna", f"antwort|{K}|0|{ant}"))
    soll(k.offenlegen(K, pr.offenlegung(st, [1, 2, 3])) and k.konten["anna"] == v0, "Pruefung nach dem Warten")

    # Anfragen fischen: Aufforderung verfallen lassen kostet Pfand, verbraucht
    # die Stellen, zaehlt als Fehlversuch.
    k, K, pr, basen, bits = aufbau()
    j = anfragen(k, K, "fremd", 16)
    k.zufall(K, j, pr.zufall(j))
    st, ch = k.aufforderung_von(K)
    k.warten(FRIST_ANTWORT + 1)
    spaet = [0] * 16
    abgewiesen(lambda: k.antwort("fremd", K, spaet, sg(k, "fremd", f"antwort|{K}|0|{spaet}")), "Antwort nach der Frist")
    soll(k.noten[K]["anfrage"] is None and k.noten[K]["fehl"] == 1 and set(st) <= k.noten[K]["benutzt"]
         and k.konten["fremd"] == 20_000_000 - PFAND, "Fischen")
    # Validatoren legen nicht offen: Pfand zurueck, kein Fehlversuch.
    v0 = k.konten["anna"]
    j = anfragen(k, K, "anna", 16)
    k.zufall(K, j, pr.zufall(j))
    st, ch = k.aufforderung_von(K)
    ant = ehrlich(basen, bits, st, ch)
    k.antwort("anna", K, ant, sg(k, "anna", f"antwort|{K}|{j}|{ant}"))
    k.warten(FRIST_OFFENLEGEN + 1)
    soll(k.konten["anna"] == v0 and k.noten[K]["fehl"] == 1 and k.noten[K]["anfrage"] is None, "Offenlegungsfrist")
    # Hash-Ketten: der letzte Wert bleibt dem Einloesen vorbehalten.
    k.noten[K]["zaehl"] = ZUFALL_L - 1
    abgewiesen(lambda: anfragen(k, K, "anna", 16), "Pruefung mit dem letzten Wert der Hash-Ketten")
    j = anfragen(k, K, "anna", einloesen="anna")
    soll(j == ZUFALL_L - 1, "Einloesen mit dem letzten Wert der Hash-Ketten")
    k.warten(FRIST_ANTWORT + 1)  # Zufall fehlt: keine Frist, die Anfrage bleibt
    k.noten[K]["anfrage"] = None  # (nachgestellt: durchgefallen)
    k._erschoepft(k.noten[K])
    soll(k.noten[K]["tot"], "Hash-Ketten verbraucht: Note nicht erschoepft")

    # Unentschieden: eine echte Note mit zu wenigen passenden Basen faellt
    # NICHT durch -- Pfand zurueck, kein Fehlversuch, die Note bleibt.
    k, K, pr, basen, bits = aufbau(n=600)
    e = k.noten[K]
    gesehen = False
    for _ in range(60):
        if len(KetteW.frei(e)) - 8 < reserve(e):
            break
        v0, fehl0 = k.konten["anna"], e["fehl"]
        r = runde(k, K, pr, "anna", lambda s, c: ehrlich(basen, bits, s, c), k_stellen=8)
        if r is None:
            gesehen = True
            soll(k.konten["anna"] == v0 and e["fehl"] == fehl0 and not e["tot"], "Unentschieden bestraft")
        else:
            soll(r is True, "echte Note mit genug Treffern abgewiesen")
    soll(gesehen, "kein Fall mit wenigen Treffern erzeugt")

    # Praegen: Missbrauch (Ablaeufe mit labor_k = 8).
    k = KetteW(VAL, labor_k=8)
    k.konto("anna", 20_000_000, b"a" * 16)
    K = "b" * 64
    m = K_MIN
    pr = Praegung(K, [0] * m, [0] * m, 3, vliste, rng)
    best = pr.bestaetigungen(VAL)
    ohne = dict(best)
    del ohne["v5"]
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, ohne), "ein Validator bestaetigt nicht")
    fremd_anker = list(pr.anker)
    fremd_anker[2] = kette_anker(h(b"anderes Geheimnis"))
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, fremd_anker, best), "Anker ausgetauscht")
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker[:-1], best), "Anker fehlt")
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 50_000_000, vliste, 3, pr.anker, best), "ohne Deckung")
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste + ["v9"], 3, pr.anker + [pr.anker[0]], best), "fremder Validator")
    abgewiesen(lambda: k.praegen("anna", K, N_MAX + 1, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best), "zu gross")
    abgewiesen(lambda: k.praegen("anna", K, m - 1, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best),
               "zu klein (nie pruefbar, Deckung fuer immer im Topf)")
    abgewiesen(lambda: k.praegen("anna", "B" * 64, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best), "K nicht kanonisch")
    k.zeit = 10 * PRAEGEN_ALT
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best), "alte Bestaetigung (Praegezeit zu alt)")
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best, k.zeit), "Bestaetigung fuer eine andere Praegezeit")
    best = pr.bestaetigungen(VAL, k.zeit + PRAEGEN_VORAUS + 1)
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best, k.zeit + PRAEGEN_VORAUS + 1), "Praegezeit in der Zukunft")
    best = pr.bestaetigungen(VAL, k.zeit)
    k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best, k.zeit)  # Gegenprobe: so geht es durch
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best, k.zeit), "K doppelt")
    k.noten[K]["tot"] = True
    abgewiesen(lambda: k.praegen("anna", K, m, pr.wurzel, 5_000_000, vliste, 3, pr.anker, best, k.zeit), "tote Note neu gepraegt")

    # ECHTE Pruefgroessen (K_TABELLE): Toleranz, Absprache-Grenze, n >= k.
    soll(f16_von(3, 5) == 7 and f16_von(1, 1) == 0 and f16_von(9, 16) == 8 and f16_von(2, 64) == 1, "f16")
    soll(k_note(4, 3, 5) == 3472 and k_note(2, 1, 1) == 552 and k_note(10, 3, 5) == 0 and k_note(7, 1, 1) == 0, "k_note")
    soll(all(len(z) == 9 and all(x == 0 or (x % 8 == 0 and K_MIN <= x <= K_MAX) for x in z) for z in K_TABELLE.values()), "Form der Tabelle")
    soll(all(all(a <= b or b == 0 for a, b in zip(z, z[1:])) for z in K_TABELLE.values()), "Tabelle nicht monoton in f")
    k = KetteW(VAL)
    k.konto("anna", 20_000_000, b"a" * 16)
    n = 3472
    K = "c" * 64
    pr = Praegung(K, [rng.randrange(2) for _ in range(n)], [rng.randrange(2) for _ in range(n)], 3, vliste, rng)
    abgewiesen(lambda: k.praegen("anna", K, n - 1, pr.wurzel, 5_000_000, vliste, 3, pr.anker, pr.bestaetigungen(VAL)), "n < k_note")
    abgewiesen(lambda: k.praegen("anna", K, n, pr.wurzel, 5_000_000, vliste, 3, pr.anker, pr.bestaetigungen(VAL, tol=7), tol=7), "Toleranz 7 nicht erlaubt")
    abgewiesen(lambda: k.praegen("anna", K, n, pr.wurzel, 5_000_000, vliste, 3, pr.anker, pr.bestaetigungen(VAL, tol=10), tol=10), "Toleranz 10 bei f = 7/16 unsicher")
    abgewiesen(lambda: k.praegen("anna", K, n, pr.wurzel, 5_000_000, vliste, 3, pr.anker, pr.bestaetigungen(VAL, tol=4), tol=5), "Toleranz nicht bestaetigt")
    k.praegen("anna", K, n, pr.wurzel, 5_000_000, vliste, 3, pr.anker, pr.bestaetigungen(VAL))  # Gegenprobe
    soll(k.noten[K]["k"] == 3472 and k.noten[K]["tol"] == 4, "k_note gespeichert")
    # Einloesen in voller Groesse (t = 1, v = 1: f = 0, tol 2 -> k = 552), in Teilen.
    V1 = {"v1": VAL["v1"]}
    k = KetteW(V1)
    k.konto("bert", 20_000_000, b"b" * 16)
    n = 2 * 552
    K = "d" * 64
    basen = [rng.randrange(2) for _ in range(n)]
    bits = [rng.randrange(2) for _ in range(n)]
    pr = Praegung(K, basen, bits, 1, ["v1"], rng)
    k.praegen("bert", K, n, pr.wurzel, 5_000_000, ["v1"], 1, pr.anker, pr.bestaetigungen(V1, tol=2), tol=2)
    e = k.noten[K]
    abgewiesen(lambda: k.anfrage("bert", K, 64, KetteW.sig(b"b" * 16, f"anfrage|{K}|64|bert|0"), "bert"), "Einloesen mit k = 64 statt 552")
    abgewiesen(lambda: k.anfrage("bert", K, 553, KetteW.sig(b"b" * 16, f"anfrage|{K}|553|None|0")), "Pruefung frisst die Reserve (552)")
    j = k.anfrage("bert", K, 552, KetteW.sig(b"b" * 16, f"anfrage|{K}|552|bert|0"), "bert")
    k.zufall(K, j, pr.zufall(j))
    st, ch = k.aufforderung_von(K)
    ant = ehrlich(basen, bits, st, ch)
    k.antwort("bert", K, ant, KetteW.sig(b"b" * 16, f"antwort|{K}|{j}|{ant}"))
    of = pr.offenlegung(st, [1])
    abgewiesen(lambda: k.offenlegen(K, of[:OFFEN_TEIL + 1]), "Teil groesser als OFFEN_TEIL")
    teile = [k.offenlegen(K, of[ab:ab + OFFEN_TEIL], ab) for ab in range(0, len(of), OFFEN_TEIL)]
    soll(all(r == "teil" for r in teile[:-1]) and teile[-1] is True and e["tot"] and k.konten["bert"] == 20_000_000 and k.topf == 0,
         f"Einloesen in Teilen: {teile[-3:]}")
    # Frist mitten im Offenlegen: Pfand zurueck, kein Fehlversuch, Stellen verbraucht.
    k.konto("anna", 20_000_000, b"a" * 16)
    K = "e" * 64
    pr = Praegung(K, basen, bits, 1, ["v1"], rng)
    k.praegen("anna", K, n, pr.wurzel, 5_000_000, ["v1"], 1, pr.anker, pr.bestaetigungen(V1, tol=2), tol=2)
    e = k.noten[K]
    j = k.anfrage("anna", K, 552, KetteW.sig(b"a" * 16, f"anfrage|{K}|552|None|0"))
    k.zufall(K, j, pr.zufall(j))
    st, ch = k.aufforderung_von(K)
    ant = ehrlich(basen, bits, st, ch)
    k.antwort("anna", K, ant, KetteW.sig(b"a" * 16, f"antwort|{K}|{j}|{ant}"))
    of = pr.offenlegung(st, [1])
    soll(k.offenlegen(K, of[:OFFEN_TEIL]) == "teil", "erster Teil")
    v0 = k.konten["anna"]
    k.warten(FRIST_OFFENLEGEN + 1)
    soll(e["anfrage"] is None and e["fehl"] == 0 and k.konten["anna"] > v0 and len(KetteW.frei(e)) == 552, "Frist mitten im Offenlegen")
    abgewiesen(lambda: k.offenlegen(K, of[OFFEN_TEIL:2 * OFFEN_TEIL], OFFEN_TEIL), "Teil nach der Frist")
    # Tabelle gegen die exakte Rechnung (Stichproben): bei k haelt die
    # Schranke, bei k - 8 nicht.
    import importlib.util as _ilu
    import os as _os
    _sp = _ilu.spec_from_file_location("wiesner_parameter", _os.path.join(_os.path.dirname(_os.path.abspath(__file__)), "wiesner_parameter.py"))
    wp = _ilu.module_from_spec(_sp)
    _sp.loader.exec_module(wp)
    for tol, f16 in ((2, 0), (4, 0), (4, 7), (6, 3), (10, 1)):
        kk = K_TABELLE[tol][f16]
        c = (1 - f16 / 16) * wp.KLON_FEHLER
        soll(wp.bedingt(kk, c, tol, MIN_TREFFER) <= 2.0 ** -40 < wp.bedingt(kk - 8, c, tol, MIN_TREFFER),
             f"K_TABELLE[{tol}][{f16}] = {kk} passt nicht zur exakten Rechnung")
    # Unsicher heisst 0: tol 8 bei f = 4/16 (Faelscher 11,0 %) braucht k > K_MAX.
    soll(K_TABELLE[8][4] == 0 and K_TABELLE[10][2] == 0, "unsichere Felder nicht 0")


def vektor() -> dict:
    """Fester Pruefvektor fuer den Knoten (quanten_w_test.go): Pruefsummen,
    Merkle-Wurzel und Pfade, Anteile und Zusammensetzen, Hash-Ketten und
    Saat, Aufforderung, Urteil, Bestaetigung, Pfand -- alles deterministisch."""
    rng = random.Random(4096)
    K = hashlib.sha256(b"aequitas-quanten-w-vektor").hexdigest()
    n, t, val = 10, 3, ["v1", "v2", "v3", "v4", "v5"]
    basen = [rng.randrange(2) for _ in range(n)]
    bits = [rng.randrange(2) for _ in range(n)]
    pr = Praegung(K, basen, bits, t, val, rng)
    werte0 = [kette_wert(g, 0) for g in pr.geheim]
    saat = zufall_saat(K, 0, werte0)
    stellen, aufford = aufforderung(K, f"{K}:0", saat, list(range(n)), 8)
    antwort = [bits[i] if b == basen[i] else 1 - bits[i] for i, b in zip(stellen, aufford)]
    antwort[0] ^= 1  # genau ein Fehler, falls die erste Stelle ein Treffer ist
    treffer = sum(basen[i] == b for i, b in zip(stellen, aufford))
    fehler = sum(basen[i] == b and o != bits[i] for i, b, o in zip(stellen, aufford, antwort))
    return {
        "K": K, "n": n, "t": t, "validatoren": val,
        "werte": [str(v) for v in pr.v],
        "pruefsummen": [b.hex() for b in pr.blaetter],
        "wurzel": pr.wurzel.hex(),
        "pfade": [[x.hex() for x in merkle_pfad(pr.blaetter, i)] for i in range(n)],
        "anteile": [[str(y) for y in a] for a in pr.anteile],
        # Hash-Ketten: Anker je Validator, die Werte fuer Anfrage 0 und 3.
        "anker": [a.hex() for a in pr.anker],
        "zufall0": [z.hex() for z in werte0],
        "zufall3": [kette_wert(g, 3).hex() for g in pr.geheim],
        "saat": saat.hex(), "anfrage_id": f"{K}:0",
        "stellen": stellen, "aufforderung": aufford, "antwort": antwort,
        "treffer": treffer, "fehler": fehler,
        "tol": TOL_PROZENT,
        "urteil": None if treffer < MIN_TREFFER else fehler * 100 <= TOL_PROZENT * treffer,
        "bestaetigung2": bestaetigung_text(K, pr.wurzel, n, t, val, 2, pr.anker[1], 1794758400, TOL_PROZENT),
        # Fall 2: alle Stellen, Aufforderung = Basen (10 Treffer), Toleranz
        # 10 %: 1 Fehler ist erlaubt (100 <= 10 * 10), 2 nicht.
        "fall2_tol": 10,
        "fall2_aufforderung": list(basen),
        "fall2_antwort_1fehler": [1 - bits[0]] + bits[1:],
        "fall2_antwort_2fehler": [1 - bits[0], 1 - bits[1]] + bits[2:],
        # Sichere Pruefgroessen: Toleranz -> k je f16 (0..8).
        "k_tabelle": {str(t_): list(z) for t_, z in K_TABELLE.items()},
        "k_note": [[tol_, t_, v_, k_note(tol_, t_, v_)] for tol_, t_, v_ in
                   ((4, 3, 5), (2, 1, 1), (10, 3, 5), (4, 9, 16), (6, 2, 64), (7, 1, 1), (4, 17, 32))],
        # Pfand: [Wert, k, n, Pfand].
        "pfand": [[w, k_, n_, pfand_fuer(w, k_, n_)] for w, k_, n_ in
                  ((5_000_000, 64, 200), (5_000_000, 8, 2048), (3_000_001, 1, 3), (9_000_000_000_000_000, 64, 65))],
    }


if __name__ == "__main__":
    if "--vektor" in sys.argv:
        import json
        print(json.dumps(vektor(), indent=1))
    else:
        sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else print(__doc__)
