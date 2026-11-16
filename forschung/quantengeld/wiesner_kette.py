#!/usr/bin/env python3
"""Variante W, Kettenseite (W2) als ausfuehrbare Spezifikation.

Forschung; der Knoten setzt genau diese Regeln um (quanten_w.go). Keine
Abhaengigkeiten. Wie bruecke_kette.py fuer die oeffentlich pruefbare Note.

GRUNDIDEE: OFFENLEGEN STATT GEMEINSAM RECHNEN
  Beim Praegen steht fuer jede Stelle i der Note nur eine gesalzene
  Pruefsumme c_i = SHA-256(Domaene | K | i | v_i) auf der Kette, als
  Merkle-Wurzel. v_i = Basis + 2*Bit + 4*Salz (Salz 124 Bit) liegt nur als
  Shamir-Anteile (t von v, Koerper 2^127 - 1) bei den Validatoren.
  Eine Pruefung laeuft in drei Transaktionen:
    1. ANFRAGE (mit Pfand): die Kette leitet aus dem Block, der sie traegt,
       Stellen und Basen ab -- vorher kennt sie niemand.
    2. ANTWORT des Anfragenden: die Messwerte, in einem SPAETEREN Block.
    3. OFFENLEGEN: t Anteile je gepruefter Stelle plus Merkle-Pfad. JEDER
       Knoten setzt v_i zusammen, prueft die Pruefsumme und urteilt selbst.
  Die geprueften Stellen sind ohnehin verbraucht (gemessen); ihr Geheimnis
  offenzulegen verraet nichts ueber die anderen (eigenes Polynom je Stelle).
  So bleibt jede Pruefung bei jedem Knoten -- ohne verteilte Rechnung auf
  der Kette und ohne einem Validator zu glauben: falsche Anteile fallen an
  der Pruefsumme auf.

REGELN (Missbrauch in Klammern)
  - Praegen nur mit Deckung, mit Bestaetigung von mindestens t der v
    Validatoren der Note, K neu, n <= N_MAX.
  - Eine offene Anfrage je Note; Pfand; Stellen werden schon bei der
    ANFRAGE verbraucht (sonst: Anfragen fischen, bis die Aufforderung nur
    bekannte Stellen trifft, und die anderen verfallen lassen).
  - Antwort nur vom Anfragenden, nur in einem spaeteren Block, nur bis zur
    Frist (Wiederholung, fremde Antwort, Antwort vor der Aufforderung).
  - Offenlegen erst nach der Antwort; genau t Anteile mit verschiedenen
    x aus der Validatorenliste; Pruefsumme und Merkle-Pfad muessen passen
    (falsche Anteile, zu wenige, fremde x, falscher Pfad).
  - Urteil: Treffer = Stellen mit passender Basis. Weniger als MIN_TREFFER:
    UNENTSCHIEDEN -- Pfand zurueck, kein Fehlversuch, die Note bleibt (sonst
    faellt eine echte Note durch, nur weil zufaellig wenige Basen passten).
    Sonst angenommen, wenn Fehler <= floor(TOLERANZ * Treffer).
  - Pruefen bestanden: Pfand zurueck. Durchgefallen oder Frist verpasst:
    Pfand verfaellt ans Grundeinkommen, Fehlversuch; nach FEHL_MAX gesperrt.
  - Einloesen: auch fuer gesperrte Noten (sonst sperrt ein Fremder mit
    Unsinn), GENAU EIN Versuch: bestanden -> Wert an das Ziel; sonst ist die
    Note tot, ihr Wert geht ans Grundeinkommen (nicht an die anderen Noten).
  - Erhaltung: Summe aller Konten + Topf + Grundeinkommen bleibt gleich.

GRENZEN, EHRLICH
  - Die Aufforderung haengt am Blockhash; ein Erzeuger, der mit einem
    Faelscher zusammenarbeitet, koennte unter mehreren Bloecken waehlen.
    Mit einem einzigen Erzeuger bleibt das ein Restrisiko (spaeter:
    gemeinsamer Zufall der Validatoren).
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
N_MAX = 2048                 # Stellen je Note
K_MIN, K_MAX = 8, 64         # Stellen je Pruefung
TOLERANZ = 0.15
TOL_PROZENT = 15             # ganzzahlig, wie im Knoten: Fehler*100 <= 15*Treffer
MIN_TREFFER = 4
FEHL_MAX = 3
FRIST_ANTWORT = 10           # Bloecke
FRIST_OFFENLEGEN = 20        # Bloecke nach der Antwort
PFAND = 1_000_000            # Mikro-AEQ (1 AEQ)
MIN_DECKUNG = 1_000_000
T_MAX, V_MAX = 16, 64
DOM_POS = b"aequitas-quanten-w-pos-v1|chain:1926|"
DOM_AUF = b"aequitas-quanten-w-aufforderung-v1|chain:1926|"
DOM_BEST = "aequitas-quanten-w-anteile-v1|chain:1926|"


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


def aufforderung(K: str, anfrage_id: str, blockhash: bytes, frei: list[int], k: int) -> tuple[list[int], list[int]]:
    """Stellen und Basen, deterministisch aus dem Block der Anfrage."""
    strom, zaehler, puffer = h(DOM_AUF, K.encode(), b"|", anfrage_id.encode(), b"|", blockhash), 0, b""

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


# ------------------------------------------------------------ Kette

class Abgewiesen(Exception):
    pass


class KetteW:
    def __init__(self, validatoren: dict[str, bytes]):
        self.val_schluessel = dict(validatoren)    # Validator -> HMAC-Schluessel (Platzhalter)
        self.konten: dict[str, int] = {}
        self.konto_schluessel: dict[str, bytes] = {}
        self.topf = 0
        self.grundeinkommen = 0
        self.noten: dict[str, dict] = {}
        self.hoehe = 0
        self.blockhash = h(b"genesis")

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
        return sum(self.konten.values()) + self.topf + self.grundeinkommen

    def block(self, salz: bytes = b""):
        """Neuer Block: Hoehe +1, neuer Hash; Aufforderungen werden gezogen,
        Fristen laufen ab.

        WARUM ERST HIER: Die Aufforderung haengt am Hash eines Blocks, der beim
        Absenden der Anfrage noch nicht feststand (hier: der naechste; im
        Knoten: der Block, der die Anfrage enthaelt, gezogen zu Beginn des
        folgenden). Sonst koennte eine Absprache, die einen Teil der Stellen
        kennt, auf einen guenstigen Blockhash warten und nur dann anfragen."""
        self.hoehe += 1
        self.blockhash = h(self.blockhash, self.hoehe.to_bytes(8, "big"), salz)
        for K, e in self.noten.items():
            a = e["anfrage"]
            if not a:
                continue
            if a["stellen"] is None:
                frei = [i for i in range(e["n"]) if i not in e["benutzt"]]
                a["stellen"], a["basen"] = aufforderung(K, a["id"], self.blockhash, frei, a["k"])
                e["benutzt"].update(a["stellen"])  # ab jetzt verbraucht, auch ohne Antwort
            if a["antwort"] is None and self.hoehe > a["hoehe"] + FRIST_ANTWORT:
                self._durchgefallen(K, e, "Antwort-Frist verpasst")
            elif a["antwort"] is not None and self.hoehe > a["antwort_hoehe"] + FRIST_OFFENLEGEN:
                # Die Validatoren haben nicht offengelegt: nicht die Schuld
                # des Anfragenden -- Pfand zurueck, Stellen bleiben verbraucht.
                self.konten[a["von"]] += a["pfand"]
                e["anfrage"] = None

    # -- 1. Praegen
    def praegen(self, von: str, K: str, n: int, wurzel: bytes, deckung: int,
                validatoren: list[str], t: int, bestaetigungen: dict[str, str]):
        if K in self.noten or len(K) != 64 or any(c not in "0123456789abcdef" for c in K):
            raise Abgewiesen("K ungueltig oder schon da")
        # n >= K_MIN: sonst liesse sich die Note nie pruefen oder einloesen,
        # ihre Deckung laege fuer immer im Topf.
        if not K_MIN <= n <= N_MAX or not 1 <= t <= T_MAX or not t <= len(validatoren) <= V_MAX:
            raise Abgewiesen("Groessen")
        if len(set(validatoren)) != len(validatoren) or any(v not in self.val_schluessel for v in validatoren):
            raise Abgewiesen("Validatorenliste")
        if deckung < MIN_DECKUNG or self.konten.get(von, 0) < deckung:
            raise Abgewiesen("Deckung")
        text = f"{DOM_BEST}{K}|{wurzel.hex()}|{n}|{t}|{','.join(validatoren)}"
        gueltig = {v for v, s in bestaetigungen.items() if v in validatoren and self._sig_ok(self.val_schluessel[v], text, s)}
        if len(gueltig) < t:
            raise Abgewiesen("zu wenige Bestaetigungen der Validatoren")
        self.konten[von] -= deckung
        self.topf += deckung
        self.noten[K] = {"n": n, "wurzel": wurzel, "wert": deckung, "validatoren": list(validatoren), "t": t,
                         "benutzt": set(), "fehl": 0, "gesperrt": False, "tot": False, "anfrage": None, "zaehl": 0}

    # -- 2. Anfrage
    def anfrage(self, von: str, K: str, k: int, sig: str, einloesen_an: str | None = None) -> str:
        e = self.noten.get(K)
        if e is None or e["tot"]:
            raise Abgewiesen("Note unbekannt oder tot")
        if e["anfrage"] is not None:
            raise Abgewiesen("schon eine offene Anfrage")
        if einloesen_an is None and e["gesperrt"]:
            raise Abgewiesen("Note gesperrt (Einloesen geht noch)")
        if not self._sig_ok(self.konto_schluessel.get(von), f"anfrage|{K}|{k}|{einloesen_an}|{e['zaehl']}", sig):
            raise Abgewiesen("Unterschrift der Anfrage")
        frei = [i for i in range(e["n"]) if i not in e["benutzt"]]
        if not K_MIN <= k <= K_MAX or k > len(frei):
            raise Abgewiesen("k")
        if self.konten.get(von, 0) < PFAND:
            raise Abgewiesen("Pfand")
        if einloesen_an is not None and einloesen_an not in self.konten:
            raise Abgewiesen("Ziel")
        anfrage_id = f"{K}:{e['zaehl']}"
        self.konten[von] -= PFAND
        e["zaehl"] += 1
        # Stellen und Basen zieht der naechste Block (block()); ab dann sind
        # die Stellen verbraucht.
        e["anfrage"] = {"von": von, "id": anfrage_id, "k": k, "stellen": None, "basen": None, "pfand": PFAND,
                        "hoehe": self.hoehe, "antwort": None, "antwort_hoehe": None, "einloesen_an": einloesen_an}
        return anfrage_id

    def aufforderung_von(self, K: str) -> tuple[list[int], list[int]]:
        """Die gezogene Aufforderung der offenen Anfrage (oeffentlich)."""
        e = self.noten.get(K)
        a = e and e["anfrage"]
        if not a or a["stellen"] is None:
            raise Abgewiesen("Aufforderung steht noch aus")
        return list(a["stellen"]), list(a["basen"])

    # -- 3. Antwort
    def antwort(self, von: str, K: str, bits: list[int], sig: str):
        e = self.noten.get(K)
        a = e and e["anfrage"]
        if not a or a["antwort"] is not None:
            raise Abgewiesen("keine offene Anfrage")
        if von != a["von"] or not self._sig_ok(self.konto_schluessel.get(von), f"antwort|{K}|{e['zaehl'] - 1}|{bits}", sig):
            raise Abgewiesen("nicht vom Anfragenden")
        if self.hoehe <= a["hoehe"] or a["stellen"] is None:
            raise Abgewiesen("Antwort im selben Block wie die Anfrage")
        if self.hoehe > a["hoehe"] + FRIST_ANTWORT:
            raise Abgewiesen("Antwort nach der Frist (Anfrage schon beendet)")
        if len(bits) != len(a["stellen"]) or any(b not in (0, 1) for b in bits):
            raise Abgewiesen("Form der Antwort")
        a["antwort"], a["antwort_hoehe"] = list(bits), self.hoehe

    # -- 4. Offenlegen und Urteil (jeder Knoten)
    def offenlegen(self, K: str, je_stelle: list[tuple[list[tuple[int, int]], list[bytes]]]) -> bool | None:
        e = self.noten.get(K)
        a = e and e["anfrage"]
        if not a or a["antwort"] is None:
            raise Abgewiesen("erst die Antwort")
        if len(je_stelle) != len(a["stellen"]):
            raise Abgewiesen("Zahl der Stellen")
        treffer = fehler = 0
        for i, beta, o, (anteile, pfad) in zip(a["stellen"], a["basen"], a["antwort"], je_stelle):
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
        if treffer < MIN_TREFFER:
            # Unentschieden: zu wenige passende Basen, um zu urteilen.
            self.konten[a["von"]] += a["pfand"]
            e["anfrage"] = None
            return None
        ok = fehler * 100 <= TOL_PROZENT * treffer
        if a["einloesen_an"] is not None:
            self.konten[a["von"]] += a["pfand"]
            if ok:
                self.konten[a["einloesen_an"]] += e["wert"]
            else:
                self.grundeinkommen += e["wert"]
            self.topf -= e["wert"]
            e["tot"], e["anfrage"] = True, None
            return ok
        if ok:
            self.konten[a["von"]] += a["pfand"]
            e["anfrage"] = None
        else:
            self._durchgefallen(K, e, "durchgefallen")
        return ok

    def _durchgefallen(self, K, e, grund):
        a = e["anfrage"]
        self.grundeinkommen += a["pfand"]
        e["fehl"] += 1
        if e["fehl"] >= FEHL_MAX:
            e["gesperrt"] = True
        if a["einloesen_an"] is not None:  # Einloesen ohne Antwort: tot
            self.grundeinkommen += e["wert"]
            self.topf -= e["wert"]
            e["tot"] = True
        e["anfrage"] = None


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
    # Aufforderung: deterministisch, ohne Wiederholung, haengt am Block.
    s1 = aufforderung("a" * 64, "x", b"B1", list(range(100)), 16)
    soll(s1 == aufforderung("a" * 64, "x", b"B1", list(range(100)), 16), "Aufforderung nicht deterministisch")
    soll(len(set(s1[0])) == 16 and s1 != aufforderung("a" * 64, "x", b"B2", list(range(100)), 16), "Aufforderung")

    VAL = {f"v{j}": bytes([j]) * 16 for j in range(1, 6)}
    vliste = list(VAL)

    def aufbau(n=48, t=3, wert_=5_000_000):
        k = KetteW(VAL)
        for name in ("anna", "bert", "fremd"):
            k.konto(name, 20_000_000, name.encode() * 4)
        K = hashlib.sha256(rng.randbytes(16)).hexdigest()
        basen = [rng.randrange(2) for _ in range(n)]
        bits = [rng.randrange(2) for _ in range(n)]
        pr = Praegung(K, basen, bits, t, vliste, rng)
        text = f"{DOM_BEST}{K}|{pr.wurzel.hex()}|{n}|{t}|{','.join(vliste)}"
        best = {v: KetteW.sig(VAL[v], text) for v in vliste[:t]}
        k.praegen("anna", K, n, pr.wurzel, wert_, vliste, t, best)
        return k, K, pr, basen, bits

    def ehrlich(basen, bits, stellen, ch):  # Halter mit echter Note, ohne Rauschen
        return [bits[i] if beta == basen[i] else rng.randrange(2) for i, beta in zip(stellen, ch)]

    def runde(k, K, pr, wer, antwort_fn, k_stellen=16, einloesen=None, wer_x=(1, 2, 3)):
        e = k.noten[K]
        k.anfrage(wer, K, k_stellen, KetteW.sig(k.konto_schluessel[wer], f"anfrage|{K}|{k_stellen}|{einloesen}|{e['zaehl']}"), einloesen)
        k.block()
        st, ch = k.aufforderung_von(K)
        bits = antwort_fn(st, ch)
        k.antwort(wer, K, bits, KetteW.sig(k.konto_schluessel[wer], f"antwort|{K}|{e['zaehl'] - 1}|{bits}"))
        k.block()
        return k.offenlegen(K, pr.offenlegung(st, list(wer_x)))

    # Gutfall: praegen, pruefen (Weitergabe), einloesen; Erhaltung.
    k, K, pr, basen, bits = aufbau()
    summe0 = k.summe()
    soll(runde(k, K, pr, "bert", lambda s, c: ehrlich(basen, bits, s, c)), "Gutfall: Pruefung")
    soll(k.konten["bert"] == 20_000_000, "Pfand nicht zurueck")
    soll(runde(k, K, pr, "bert", lambda s, c: ehrlich(basen, bits, s, c), einloesen="bert", wer_x=(2, 4, 5)), "Gutfall: Einloesen")
    soll(k.konten["bert"] == 25_000_000 and k.topf == 0 and k.noten[K]["tot"], "Einloesen bucht falsch")
    soll(k.summe() == summe0, "Erhaltung")
    abgewiesen(lambda: k.anfrage("bert", K, 8, KetteW.sig(k.konto_schluessel["bert"], f"anfrage|{K}|8|bert|{k.noten[K]['zaehl']}"), "bert"), "zweimal einloesen")

    # Faelschung ohne Note: Zufallsantworten -> durchgefallen, Pfand weg, nach FEHL_MAX gesperrt.
    k, K, pr, basen, bits = aufbau(n=96)
    for j in range(FEHL_MAX):
        soll(not runde(k, K, pr, "fremd", lambda s, c: [rng.randrange(2) for _ in s]), f"Faelschung {j} bestanden")
    soll(k.noten[K]["gesperrt"] and k.konten["fremd"] == 20_000_000 - FEHL_MAX * PFAND, "Sperre/Pfand")
    # A7: gesperrt, der echte Halter loest trotzdem ein -- genau einmal.
    soll(runde(k, K, pr, "anna", lambda s, c: ehrlich(basen, bits, s, c), einloesen="anna"), "A7: echter Halter")
    # Faelscher bekommt beim Einloesen genau einen Versuch; der Wert geht ans Grundeinkommen.
    k, K, pr, basen, bits = aufbau()
    ge0 = k.grundeinkommen
    soll(not runde(k, K, pr, "fremd", lambda s, c: [rng.randrange(2) for _ in s], einloesen="fremd"), "Faelscher loest ein")
    soll(k.noten[K]["tot"] and k.grundeinkommen == ge0 + 5_000_000 and k.topf == 0, "toter Wert nicht ans Grundeinkommen")

    # Missbrauch der Ablaeufe.
    k, K, pr, basen, bits = aufbau()
    e = k.noten[K]
    sg = lambda wer, text: KetteW.sig(k.konto_schluessel[wer], text)
    abgewiesen(lambda: k.antwort("anna", K, [0] * 16, sg("anna", f"antwort|{K}|0|{[0] * 16}")), "Antwort ohne Anfrage")
    hash_vorher = k.blockhash
    k.anfrage("anna", K, 16, sg("anna", f"anfrage|{K}|16|None|0"))
    abgewiesen(lambda: k.aufforderung_von(K), "Aufforderung im Block der Anfrage")
    abgewiesen(lambda: k.anfrage("anna", K, 16, sg("anna", f"anfrage|{K}|16|None|1")), "zweite offene Anfrage")
    ant0 = [0] * 16
    abgewiesen(lambda: k.antwort("anna", K, ant0, sg("anna", f"antwort|{K}|0|{ant0}")), "Antwort im selben Block")
    soll(not e["benutzt"], "Stellen vor dem Ziehen verbraucht")
    k.block()
    st, ch = k.aufforderung_von(K)
    # Die Aufforderung haengt am NEUEN Blockhash, nicht an dem beim Absenden.
    soll((st, ch) != aufforderung(K, f"{K}:0", hash_vorher, list(range(e["n"])), 16)
         and (st, ch) == aufforderung(K, f"{K}:0", k.blockhash, list(range(e["n"])), 16), "Aufforderung aus dem alten Blockhash")
    soll(set(st) <= e["benutzt"], "gezogene Stellen nicht verbraucht")
    ant = ehrlich(basen, bits, st, ch)  # einmal erzeugt: Unterschrift und Antwort passen zusammen
    abgewiesen(lambda: k.offenlegen(K, pr.offenlegung(st, [1, 2, 3])), "Offenlegen vor der Antwort")
    abgewiesen(lambda: k.antwort("fremd", K, ant, sg("fremd", f"antwort|{K}|0|{ant}")), "fremde Antwort")
    abgewiesen(lambda: k.antwort("anna", K, ant, sg("fremd", f"antwort|{K}|0|{ant}")), "falsche Unterschrift")
    abgewiesen(lambda: k.antwort("anna", K, ant[:-1], sg("anna", f"antwort|{K}|0|{ant[:-1]}")), "kurze Antwort")
    k.antwort("anna", K, ant, sg("anna", f"antwort|{K}|0|{ant}"))
    abgewiesen(lambda: k.antwort("anna", K, ant, sg("anna", f"antwort|{K}|0|{ant}")), "Antwort wiederholt")
    k.block()
    of = pr.offenlegung(st, [1, 2, 3])
    falsch = [([(x, (y + 1) % P) if j == 0 else (x, y) for j, (x, y) in enumerate(a)], p) for a, p in of]
    abgewiesen(lambda: k.offenlegen(K, falsch), "falscher Anteil")
    abgewiesen(lambda: k.offenlegen(K, [(a[:2], p) for a, p in of]), "zu wenige Anteile")
    abgewiesen(lambda: k.offenlegen(K, [([a[0], a[0], a[1]], p) for a, p in of]), "doppeltes x")
    abgewiesen(lambda: k.offenlegen(K, [([(9, a[0][1])] + a[1:], p) for a, p in of]), "fremdes x")
    abgewiesen(lambda: k.offenlegen(K, [(a, p[::-1]) for a, p in of]), "falscher Pfad")
    abgewiesen(lambda: k.offenlegen(K, of[:-1]), "Stelle fehlt")
    soll(k.offenlegen(K, of), "gueltige Offenlegung nach den Angriffen")
    soll(set(st) <= e["benutzt"], "Stellen nicht verbraucht")

    # Anfragen fischen: verfallen lassen kostet Pfand, verbraucht die Stellen, zaehlt als Fehlversuch.
    k, K, pr, basen, bits = aufbau(n=40)
    k.anfrage("fremd", K, 16, KetteW.sig(k.konto_schluessel["fremd"], f"anfrage|{K}|16|None|0"))
    k.block()
    st, ch = k.aufforderung_von(K)
    for _ in range(FRIST_ANTWORT - 1):
        k.block()
    spaet = [0] * 16
    k.block()  # Hoehe = Anfrage + FRIST_ANTWORT + 1: block() hat die Anfrage schon beendet
    abgewiesen(lambda: k.antwort("fremd", K, spaet, KetteW.sig(k.konto_schluessel["fremd"], f"antwort|{K}|0|{spaet}")), "Antwort nach der Frist (Anfrage schon beendet)")
    soll(k.noten[K]["anfrage"] is None and k.noten[K]["fehl"] == 1 and set(st) <= k.noten[K]["benutzt"], "Fischen")
    # Mehr Pruefungen als Stellen: 40 Stellen, 2 x 16 verbraucht, 16 passen nicht mehr.
    k.anfrage("anna", K, 16, KetteW.sig(k.konto_schluessel["anna"], f"anfrage|{K}|16|None|1"))
    for _ in range(FRIST_ANTWORT + 1):
        k.block()
    abgewiesen(lambda: k.anfrage("anna", K, 16, KetteW.sig(k.konto_schluessel["anna"], f"anfrage|{K}|16|None|2")), "mehr Stellen als frei")
    # Validatoren legen nicht offen: Pfand zurueck, kein Fehlversuch.
    k, K, pr, basen, bits = aufbau()
    v0 = k.konten["anna"]
    k.anfrage("anna", K, 16, KetteW.sig(k.konto_schluessel["anna"], f"anfrage|{K}|16|None|0"))
    k.block()
    st, ch = k.aufforderung_von(K)
    ant = ehrlich(basen, bits, st, ch)
    k.antwort("anna", K, ant, KetteW.sig(k.konto_schluessel["anna"], f"antwort|{K}|0|{ant}"))
    for _ in range(FRIST_OFFENLEGEN + 1):
        k.block()
    soll(k.konten["anna"] == v0 and k.noten[K]["fehl"] == 0 and k.noten[K]["anfrage"] is None, "Offenlegungsfrist")

    # Unentschieden: eine echte Note mit zu wenigen passenden Basen faellt
    # NICHT durch -- Pfand zurueck, kein Fehlversuch, die Note bleibt.
    k, K, pr, basen, bits = aufbau(n=64)
    e = k.noten[K]
    gesehen = False
    for _ in range(60):
        if e["n"] - len(e["benutzt"]) < 8:
            break
        v0, fehl0 = k.konten["anna"], e["fehl"]
        k.anfrage("anna", K, 8, KetteW.sig(k.konto_schluessel["anna"], f"anfrage|{K}|8|None|{e['zaehl']}"))
        k.block()
        st, ch = k.aufforderung_von(K)
        ant = ehrlich(basen, bits, st, ch)
        k.antwort("anna", K, ant, KetteW.sig(k.konto_schluessel["anna"], f"antwort|{K}|{e['zaehl'] - 1}|{ant}"))
        k.block()
        r = k.offenlegen(K, pr.offenlegung(st, [1, 2, 3]))
        treffer = sum(basen[i] == b for i, b in zip(st, ch))
        if treffer < MIN_TREFFER:
            gesehen = True
            soll(r is None and k.konten["anna"] == v0 and e["fehl"] == fehl0 and not e["tot"], "Unentschieden bestraft")
        else:
            soll(r is True, "echte Note mit genug Treffern abgewiesen")
    soll(gesehen, "kein Fall mit wenigen Treffern erzeugt")

    # Praegen: Missbrauch.
    k = KetteW(VAL)
    k.konto("anna", 20_000_000, b"a" * 16)
    K = "b" * 64
    m = K_MIN
    w = merkle_wurzel([h(bytes([j])) for j in range(m)])
    text = f"{DOM_BEST}{K}|{w.hex()}|{m}|3|{','.join(vliste)}"
    best = {v: KetteW.sig(VAL[v], text) for v in vliste[:3]}
    abgewiesen(lambda: k.praegen("anna", K, m, w, 5_000_000, vliste, 3, dict(list(best.items())[:2])), "zu wenige Bestaetigungen")
    abgewiesen(lambda: k.praegen("anna", K, m, w, 50_000_000, vliste, 3, best), "ohne Deckung")
    abgewiesen(lambda: k.praegen("anna", K, m, w, 5_000_000, vliste + ["v9"], 3, best), "fremder Validator")
    abgewiesen(lambda: k.praegen("anna", K, N_MAX + 1, w, 5_000_000, vliste, 3, best), "zu gross")
    klein = f"{DOM_BEST}{K}|{w.hex()}|{m - 1}|3|{','.join(vliste)}"
    abgewiesen(lambda: k.praegen("anna", K, m - 1, w, 5_000_000, vliste, 3, {v: KetteW.sig(VAL[v], klein) for v in vliste[:3]}),
               "zu klein (nie pruefbar, Deckung fuer immer im Topf)")
    abgewiesen(lambda: k.praegen("anna", "B" * 64, m, w, 5_000_000, vliste, 3, best), "K nicht kanonisch")
    k.praegen("anna", K, m, w, 5_000_000, vliste, 3, best)  # Gegenprobe: so geht es durch
    abgewiesen(lambda: k.praegen("anna", K, m, w, 5_000_000, vliste, 3, best), "K doppelt")


def vektor() -> dict:
    """Fester Pruefvektor fuer den Knoten (quanten_w_test.go): Pruefsummen,
    Merkle-Wurzel und Pfade, Anteile und Zusammensetzen, Aufforderung aus
    einem Blockhash, Urteil -- alles deterministisch."""
    rng = random.Random(4096)
    K = hashlib.sha256(b"aequitas-quanten-w-vektor").hexdigest()
    n, t, val = 10, 3, ["v1", "v2", "v3", "v4", "v5"]
    basen = [rng.randrange(2) for _ in range(n)]
    bits = [rng.randrange(2) for _ in range(n)]
    pr = Praegung(K, basen, bits, t, val, rng)
    blockhash = h(b"vektor-block")
    stellen, aufford = aufforderung(K, f"{K}:0", blockhash, list(range(n)), 8)
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
        "blockhash": blockhash.hex(), "anfrage_id": f"{K}:0",
        "stellen": stellen, "aufforderung": aufford, "antwort": antwort,
        "treffer": treffer, "fehler": fehler,
        "urteil": None if treffer < MIN_TREFFER else fehler * 100 <= TOL_PROZENT * treffer,
        "bestaetigung": f"{DOM_BEST}{K}|{pr.wurzel.hex()}|{n}|{t}|{','.join(val)}",
        # Fall 2: alle Stellen, Aufforderung = Basen (10 Treffer); 1 Fehler
        # ist erlaubt (floor(0,15 * 10) = 1), 2 nicht.
        "fall2_aufforderung": list(basen),
        "fall2_antwort_1fehler": [1 - bits[0]] + bits[1:],
        "fall2_antwort_2fehler": [1 - bits[0], 1 - bits[1]] + bits[2:],
    }


if __name__ == "__main__":
    if "--vektor" in sys.argv:
        import json
        print(json.dumps(vektor(), indent=1))
    else:
        sys.exit(0 if selbsttest() else 1) if "--selbsttest" in sys.argv else print(__doc__)
