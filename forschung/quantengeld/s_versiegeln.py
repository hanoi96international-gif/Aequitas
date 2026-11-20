#!/usr/bin/env python3
"""Messprotokoll eines S-Abschnitts fuer genau einen Validator versiegeln.

Warum: Die Messung (u, t, y, x) ist die Praeparation des Tokens. Wer sie hat
und zugleich Inhaberin ist, koennte an beiden Annahmestellen bestehen. Der
Workflow quanten-s-ausgabe.yml gab sie bisher im Klartext als Job-Ausgabe vom
Mess-Job (Umgebung quantenhardware) an den Ausgabe-Job (produktion) weiter.
Jetzt verschluesselt der Mess-Job sie auf den Schluessel des Validators, der
den Abschnitt praepariert; nur dessen Knoten oeffnet sie wieder
(`aequitasd quanten-s-messung-oeffnen`, keeper/quanten_s_versiegeln.go).

Verfahren: ECIES genau wie go-ethereum (crypto/ecies, secp256k1,
ECIES_AES128_SHA256): ECDH (x-Koordinate, 32 Byte), Concat-KDF mit SHA-256
(Zaehler ab 1, ohne s1), Ke = K[0:16], Km = SHA-256(K[16:32]), AES-128-CTR
mit zufaelligem IV, Tag = HMAC-SHA-256(Km, IV || Chiffrat). Ergebnis:
R (65 Byte, unkomprimiert) || IV || Chiffrat || Tag.

Inhalt (Klartext): {"dom": "aequitas-qs-messung-v1", "an": <Adresse des
Validators>, "messung": <Datensatz>}. Der Knoten prueft dom, an (= er selbst)
und die Form des Datensatzes.

Aufrufe:
  s_versiegeln.py --selbsttest
  s_versiegeln.py --pruefen-schluessel PUBHEX --adresse 0x...   (Exit 0/1)
  s_versiegeln.py --fuer PUBHEX --adresse 0x... < messung.json > versiegelt.hex

Nur die Standardbibliothek und `cryptography` (schon in
hardware-anforderungen.txt, mit Hash festgelegt). Keccak-256 (fuer die
Adresse aus dem Schluessel) ist hier selbst geschrieben, weil hashlib nur
SHA3 kennt (andere Auffuellung); geprueft gegen bekannte Werte.
"""

import hashlib
import hmac
import json
import os
import sys

DOM = "aequitas-qs-messung-v1"
MESSUNG_MAX = 1 << 20  # Byte; ein Abschnitt mit n = 3.520 hat rund 15 KB
FELDER = ["job", "t", "u", "wo", "x", "y"]

# --- Keccak-256 (die Ethereum-Fassung, Auffuellung 0x01) -------------------

_RC = [
    0x0000000000000001, 0x0000000000008082, 0x800000000000808A, 0x8000000080008000,
    0x000000000000808B, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
    0x000000000000008A, 0x0000000000000088, 0x0000000080008009, 0x000000008000000A,
    0x000000008000808B, 0x800000000000008B, 0x8000000000008089, 0x8000000000008003,
    0x8000000000008002, 0x8000000000000080, 0x000000000000800A, 0x800000008000000A,
    0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
]
_ROT = [
    [0, 36, 3, 41, 18], [1, 44, 10, 45, 2], [62, 6, 43, 15, 61],
    [28, 55, 25, 21, 56], [27, 20, 39, 8, 14],
]
_M = (1 << 64) - 1


def _rol(v: int, n: int) -> int:
    return ((v << n) | (v >> (64 - n))) & _M if n else v


def _keccak_f(a: list) -> None:
    for rc in _RC:
        c = [a[x][0] ^ a[x][1] ^ a[x][2] ^ a[x][3] ^ a[x][4] for x in range(5)]
        d = [c[(x - 1) % 5] ^ _rol(c[(x + 1) % 5], 1) for x in range(5)]
        for x in range(5):
            for y in range(5):
                a[x][y] ^= d[x]
        b = [[0] * 5 for _ in range(5)]
        for x in range(5):
            for y in range(5):
                b[y][(2 * x + 3 * y) % 5] = _rol(a[x][y], _ROT[x][y])
        for x in range(5):
            for y in range(5):
                a[x][y] = b[x][y] ^ ((~b[(x + 1) % 5][y]) & b[(x + 2) % 5][y])
        a[0][0] ^= rc


def keccak256(daten: bytes) -> bytes:
    rate = 136
    p = bytearray(daten) + b"\x01"
    p += b"\x00" * ((-len(p)) % rate)
    p[-1] |= 0x80
    a = [[0] * 5 for _ in range(5)]
    for i in range(0, len(p), rate):
        block = p[i:i + rate]
        for j in range(rate // 8):
            a[j % 5][j // 5] ^= int.from_bytes(block[8 * j:8 * j + 8], "little")
        _keccak_f(a)
    aus = b""
    for j in range(4):
        aus += a[j % 5][j // 5].to_bytes(8, "little")
    return aus


# --- Schluessel ------------------------------------------------------------

def _ec():
    from cryptography.hazmat.primitives.asymmetric import ec
    return ec


def schluessel_lesen(pub_hex: str):
    """Oeffentlicher secp256k1-Schluessel, unkomprimiert (04 || X || Y), als Hex."""
    t = pub_hex.strip().lower()
    if t.startswith("0x"):
        t = t[2:]
    if len(t) != 130 or not t.startswith("04") or any(c not in "0123456789abcdef" for c in t):
        raise ValueError("Schluessel: 65 Byte unkomprimiert (04...) als Hex")
    ec = _ec()
    return ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256K1(), bytes.fromhex(t))


def adresse_von(pub) -> str:
    from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat
    roh = pub.public_bytes(Encoding.X962, PublicFormat.UncompressedPoint)
    return "0x" + keccak256(roh[1:])[12:].hex()


def adresse_lesen(text: str) -> str:
    a = text.strip().lower()
    if len(a) != 42 or not a.startswith("0x") or any(c not in "0123456789abcdef" for c in a[2:]):
        raise ValueError("Adresse: 0x und 40 Hex-Zeichen")
    return a


# --- ECIES wie go-ethereum ------------------------------------------------

def _kdf(z: bytes, laenge: int) -> bytes:
    aus, zaehler = b"", 1
    while len(aus) < laenge:
        aus += hashlib.sha256(zaehler.to_bytes(4, "big") + z).digest()
        zaehler += 1
    return aus[:laenge]


def _ctr(ke: bytes, iv: bytes, daten: bytes) -> bytes:
    from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
    c = Cipher(algorithms.AES(ke), modes.CTR(iv)).encryptor()
    return c.update(daten) + c.finalize()


def verschluesseln(pub, klartext: bytes, _fest: tuple = None) -> bytes:
    """_fest = (ephemerer Schluessel als int, IV) nur fuer den Pruefvektor."""
    from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat
    ec = _ec()
    if _fest is None:
        r = ec.generate_private_key(ec.SECP256K1())
        iv = os.urandom(16)
    else:
        r = ec.derive_private_key(_fest[0], ec.SECP256K1())
        iv = _fest[1]
    z = r.exchange(ec.ECDH(), pub)
    k = _kdf(z, 32)
    ke, km = k[:16], hashlib.sha256(k[16:]).digest()
    em = iv + _ctr(ke, iv, klartext)
    tag = hmac.new(km, em, hashlib.sha256).digest()
    return r.public_key().public_bytes(Encoding.X962, PublicFormat.UncompressedPoint) + em + tag


def entschluesseln(priv, chiffrat: bytes) -> bytes:
    ec = _ec()
    if len(chiffrat) < 65 + 16 + 32 or chiffrat[0] != 4:
        raise ValueError("Chiffrat zu kurz oder ohne unkomprimierten Punkt")
    r = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256K1(), chiffrat[:65])
    z = priv.exchange(ec.ECDH(), r)
    k = _kdf(z, 32)
    ke, km = k[:16], hashlib.sha256(k[16:]).digest()
    em, tag = chiffrat[65:-32], chiffrat[-32:]
    if not hmac.compare_digest(hmac.new(km, em, hashlib.sha256).digest(), tag):
        raise ValueError("Tag passt nicht (falscher Schluessel oder veraendert)")
    return _ctr(ke, em[:16], em[16:])


# --- Messung ---------------------------------------------------------------

def messung_pruefen(m) -> None:
    """Genau die Felder, die der Dienst liest; Bits nur 0/1, alle gleich lang."""
    if not isinstance(m, dict) or sorted(m) != FELDER:
        raise ValueError(f"Messung: genau die Felder {FELDER}")
    n = len(m["y"]) if isinstance(m["y"], str) else 0
    for f in ("u", "t", "x", "y"):
        if not isinstance(m[f], str) or len(m[f]) != n or n < 2 or set(m[f]) - {"0", "1"}:
            raise ValueError(f"Messung: {f} sind keine {n} Bits")
    for f in ("wo", "job"):
        if not isinstance(m[f], str) or len(m[f]) > 512:
            raise ValueError(f"Messung: {f}")


def versiegeln(pub, an: str, messung: dict) -> bytes:
    messung_pruefen(messung)
    if adresse_von(pub) != an:
        raise ValueError("Schluessel gehoert nicht zur Adresse des Validators")
    roh = json.dumps({"dom": DOM, "an": an, "messung": messung}, separators=(",", ":"), sort_keys=True).encode()
    return verschluesseln(pub, roh)


# --- Selbsttest -------------------------------------------------------------

def _vektor_pfad(name: str) -> str:
    hier = os.path.dirname(os.path.abspath(__file__))
    return os.path.join(hier, "..", "..", "x", "humanity", "keeper", "testdata", name)


def selbsttest() -> None:
    ec = _ec()
    assert keccak256(b"").hex() == "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
    assert keccak256(b"abc").hex() == "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45"
    assert keccak256(b"a" * 200).hex() == keccak256(bytes(b"a" * 200)).hex()  # ueber zwei Bloecke
    # Gegen den Pruefvektor des Knotens (Go hat ihn erzeugt): Adresse aus dem
    # Schluessel, und go-ethereum-Chiffrate werden hier gelesen.
    with open(_vektor_pfad("quanten_s_inhaberin_vektor.json")) as fh:
        v = json.load(fh)
    inh = ec.derive_private_key(int(v["inhaberin_schluessel_nur_test"], 16), ec.SECP256K1())
    assert adresse_von(inh.public_key()) == v["inhaberin"], "Adresse aus dem Schluessel"
    assert adresse_von(schluessel_lesen(v["inhaberin_pub"])) == v["inhaberin"]
    for ab in v["abschnitte"]:
        klar = json.loads(entschluesseln(inh, bytes.fromhex(ab["abschnitt"]["fuer_inhaberin"])))
        assert klar["x"] == ab["x"] and klar["k"] == v["k"], "go-ethereum-Chiffrat gelesen"
    try:
        entschluesseln(inh, bytes.fromhex(v["falsch_fuer_inhaberin"]))
        raise AssertionError("Chiffrat fuer einen anderen Schluessel gelesen")
    except ValueError:
        pass
    # Eigener Vektor, den Go liest (quanten_s_versiegeln_test.go): fest
    # erzeugt, also hier byte-genau nachzurechnen.
    with open(_vektor_pfad("quanten_s_versiegelt_vektor.json")) as fh:
        w = json.load(fh)
    val = ec.derive_private_key(int(w["validator_schluessel_nur_test"], 16), ec.SECP256K1())
    assert adresse_von(val.public_key()) == w["an"]
    roh = json.dumps({"dom": DOM, "an": w["an"], "messung": w["messung"]}, separators=(",", ":"), sort_keys=True).encode()
    assert roh.decode() == w["klartext"], "Klartext"
    fest = verschluesseln(val.public_key(), roh, (int(w["ephemer_nur_test"], 16), bytes.fromhex(w["iv_nur_test"])))
    assert fest.hex() == w["chiffrat"], "Chiffrat weicht vom Vektor ab"
    assert entschluesseln(val, fest) == roh
    # Zufaellig: hin und zurueck, jede Aenderung faellt auf.
    c = versiegeln(val.public_key(), w["an"], w["messung"])
    assert json.loads(entschluesseln(val, c))["messung"] == w["messung"]
    for i in (0, 70, len(c) - 40, len(c) - 1):
        kaputt = bytearray(c)
        kaputt[i] ^= 1
        try:
            entschluesseln(val, bytes(kaputt))
            raise AssertionError(f"veraendertes Byte {i} nicht bemerkt")
        except ValueError:
            pass
    for kurz in (b"", b"\x04" * 112, b"\x02" + fest[1:]):
        try:
            entschluesseln(val, kurz)
            raise AssertionError("kurzes oder komprimiertes Chiffrat angenommen")
        except ValueError as e:
            assert "zu kurz" in str(e), e
    try:
        versiegeln(val.public_key(), "0x" + "11" * 20, w["messung"])
        raise AssertionError("fremde Adresse angenommen")
    except ValueError:
        pass
    for falsch in (dict(w["messung"], x=w["messung"]["x"][:-1]), dict(w["messung"], extra=""), dict(w["messung"], u="2" * len(w["messung"]["u"]))):
        try:
            messung_pruefen(falsch)
            raise AssertionError("kaputte Messung angenommen")
        except ValueError:
            pass
    print("s_versiegeln: Selbsttest bestanden (Keccak, Adresse, go-ethereum-ECIES in beide Richtungen, Manipulation, Form)")


def vektor_schreiben(pfad: str) -> None:
    """Der feste Pruefvektor fuer Go (nur Test-Schluessel, nie fuer echte Werte)."""
    ec = _ec()
    w = {"hinweis": "nur fuer Tests: feste Schluessel, fester ephemerer Schluessel und IV -- nie fuer echte Werte",
         "validator_schluessel_nur_test": "71" * 32, "ephemer_nur_test": "72" * 32, "iv_nur_test": "73" * 16,
         "messung": {"u": "01101001", "t": "11000101", "y": "10011100", "x": "01011101", "wo": "Pruefvektor", "job": "vektor-1"}}
    val = ec.derive_private_key(int(w["validator_schluessel_nur_test"], 16), ec.SECP256K1())
    w["an"] = adresse_von(val.public_key())
    from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat
    w["validator_pub"] = val.public_key().public_bytes(Encoding.X962, PublicFormat.UncompressedPoint).hex()
    w["name"] = hashlib.sha256(w["messung"]["y"].encode()).hexdigest()
    roh = json.dumps({"dom": DOM, "an": w["an"], "messung": w["messung"]}, separators=(",", ":"), sort_keys=True).encode()
    w["klartext"] = roh.decode()
    w["chiffrat"] = verschluesseln(val.public_key(), roh, (int(w["ephemer_nur_test"], 16), bytes.fromhex(w["iv_nur_test"]))).hex()
    with open(pfad, "w") as fh:
        json.dump(w, fh, indent=2, sort_keys=True)
        fh.write("\n")


def main() -> int:
    a = sys.argv[1:]
    if a == ["--selbsttest"]:
        selbsttest()
        return 0
    if len(a) == 2 and a[0] == "--vektor-schreiben":
        vektor_schreiben(a[1])
        return 0
    if len(a) == 4 and a[0] in ("--fuer", "--pruefen-schluessel") and a[2] == "--adresse":
        try:
            pub, an = schluessel_lesen(a[1]), adresse_lesen(a[3])
            if adresse_von(pub) != an:
                raise ValueError("Schluessel gehoert nicht zur Adresse des Validators")
            if a[0] == "--pruefen-schluessel":
                return 0
            roh = sys.stdin.buffer.read(MESSUNG_MAX + 1)
            if len(roh) > MESSUNG_MAX:
                raise ValueError("Messung zu gross")
            sys.stdout.write(versiegeln(pub, an, json.loads(roh)).hex())
            return 0
        except ValueError as e:
            print(f"ABBRUCH: {e}", file=sys.stderr)
            return 1
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
