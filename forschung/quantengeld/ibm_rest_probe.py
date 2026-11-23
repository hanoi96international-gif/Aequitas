#!/usr/bin/env python3
"""Probe der IBM-REST-Schnittstelle fuer die App (Quantengeld Weg B, B4b-2).

Forschung, nicht im Knoten. Nur Standardbibliothek (kein qiskit).

Die App (Aequitas-App lib/ibmQuantum.ts) reicht die Schaltkreise eines Abschnitts
ohne qiskit direkt ueber die REST-Schnittstelle von IBM ein. Belegt sind die Form
aus dem Quelltext von qiskit-ibm-runtime 0.52.1 und aus der IBM-Doku "Sampler
with the REST API" (OpenQASM-3-Text in pubs; Ergebnis results[i].data.c.samples).
Der Python-Client schickt "support_qiskit": true und bekommt eine kodierte Form;
die App schickt den Schalter nicht und liest beide Formen. Diese Probe
schickt GENAU den Koerper der App (testdata/ibm_auftrag_vektor.json, Kopie aus der
App; nur "backend" wird durch das Geraet der Umgebung ersetzt) mit dem Konto des
Betreibers an echte Hardware, liest das Ergebnis so wie die App und prueft die
Physik: wo die Basis der Inhaberin passt (u = y), muss die Mehrheit t zeigen.

Zugang NUR ueber die Umgebung: IBM_QUANTUM_TOKEN (IBM-Cloud-API-Schluessel), wahlweise
IBM_QUANTUM_INSTANCE (CRN) und IBM_QUANTUM_BACKEND. Fehlen die beiden, findet die Probe
sie wie die App: Instanzen ueber IBM Cloud Global Search, Geraet mit der kuerzesten
Schlange. Schluessel und CRN werden nie ausgegeben.

Aufruf:
  python3 -I ibm_rest_probe.py --selbsttest        (ohne Netz: Lesen am App-Vektor)
  python3 -I ibm_rest_probe.py --probe AUSGABE.json (echte Hardware)
"""
from __future__ import annotations

import base64
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import zlib

HIER = os.path.dirname(os.path.abspath(__file__))
AUFTRAG = os.path.join(HIER, "testdata", "ibm_auftrag_vektor.json")
ERGEBNIS = os.path.join(HIER, "testdata", "ibm_ergebnis_vektor.json")
ERGEBNIS_EINFACH = os.path.join(HIER, "testdata", "ibm_ergebnis_einfach_vektor.json")

IAM_URL = "https://iam.cloud.ibm.com/identity/token"
# Wie qiskit-ibm-runtime (api/utils.py default_runtime_url_resolver): der Host folgt der Region im CRN.
# Feste Liste statt Region in den Hostnamen einsetzen (kein Host aus fremden Daten).
API_BASIS_JE_REGION = {"us-east": "https://quantum.cloud.ibm.com/api/v1", "eu-de": "https://eu-de.quantum.cloud.ibm.com/api/v1"}
API_VERSION = "2026-04-15"
SUCHE_URL = "https://api.global-search-tagging.cloud.ibm.com/v3/resources/search?limit=100"
CRN_FORM = re.compile(r"crn:v1:bluemix:public:quantum-computing:[a-z0-9-]{2,20}:a/[0-9a-f]{32}:[0-9a-f-]{36}::")
GERAET_FORM = re.compile(r"[a-z][a-z0-9_]{2,40}")
WARTEN_MAX_S = 3600
FEHLERRATE_MAX = 0.10   # Treffer (u = y), deren Mehrheit nicht t zeigt


class ProbeFehler(Exception):
    pass


# ------------------------------------------------------------ Lesen (wie die App)

def huelle(o, typ: str):
    if not isinstance(o, dict) or o.get("__type__") != typ or "__value__" not in o:
        raise ProbeFehler(f"Ergebnis: {typ} erwartet")
    return o["__value__"]


def npy_zeilen(roh: bytes) -> list[bytes]:
    if len(roh) < 10 or roh[:6] != b"\x93NUMPY":
        raise ProbeFehler("npy: Kopf")
    if roh[6] == 1:
        lang, ab = roh[8] | (roh[9] << 8), 10
    elif roh[6] in (2, 3):
        lang, ab = int.from_bytes(roh[8:12], "little"), 12
    else:
        raise ProbeFehler("npy: Version")
    kopf = roh[ab:ab + lang].decode("latin-1")
    if not re.search(r"'descr':\s*'\|u1'", kopf) or not re.search(r"'fortran_order':\s*False", kopf):
        raise ProbeFehler("npy: Typ")
    m = re.search(r"'shape':\s*\((\d+),\s*(\d+)\)", kopf)
    if not m:
        raise ProbeFehler("npy: Form")
    zeilen, breite = int(m.group(1)), int(m.group(2))
    daten = roh[ab + lang:]
    if not (1 <= zeilen <= 15 and 1 <= breite <= 8 and len(daten) == zeilen * breite):
        raise ProbeFehler("npy: Groesse")
    return [daten[i * breite:(i + 1) * breite] for i in range(zeilen)]


def ndarray_zeilen(nd) -> list[bytes]:
    v = huelle(nd, "ndarray")
    if isinstance(v, str):
        return npy_zeilen(zlib.decompress(base64.standard_b64decode(v)))
    if isinstance(v, list):
        return [bytes(z) for z in v]
    raise ProbeFehler("ndarray: Form")


def kodierte_schuesse(roh, kreise_n: list[int]) -> list[list[str]]:
    pubs = huelle(roh, "PrimitiveResult").get("pub_results")
    if not isinstance(pubs, list) or len(pubs) != len(kreise_n):
        raise ProbeFehler("Ergebnis: Zahl der Schaltkreise")
    out = []
    for p, n in zip(pubs, kreise_n):
        ba = huelle(huelle(huelle(p, "SamplerPubResult").get("data"), "DataBin").get("fields", {}).get("c"), "BitArray")
        if ba.get("num_bits") != n:
            raise ProbeFehler("Ergebnis: Zahl der Bits")
        schuesse = []
        for z in ndarray_zeilen(ba.get("array")):
            if len(z) != (n + 7) // 8:
                raise ProbeFehler("Ergebnis: Breite")
            v = int.from_bytes(z, "big")
            if v >> n:
                raise ProbeFehler("Ergebnis: Bits ueber n")
            schuesse.append(hex(v))
        out.append(schuesse)
    return out


HEX_SCHUSS = re.compile(r"0x[0-9a-fA-F]{1,4}")


def einfache_schuesse(roh, kreise_n: list[int]) -> list[list[str]]:
    """Form der IBM-Doku (ohne support_qiskit): results[i].data.c.samples."""
    res = roh.get("results")
    if not isinstance(res, list) or len(res) != len(kreise_n):
        raise ProbeFehler("Ergebnis: Zahl der Schaltkreise")
    out = []
    for r, n in zip(res, kreise_n):
        data = r.get("data") if isinstance(r, dict) else None
        if not isinstance(data, dict) or list(data) != ["c"]:
            raise ProbeFehler("Ergebnis: Register")
        c = data["c"]
        if not isinstance(c, dict) or ("num_bits" in c and c["num_bits"] != n):
            raise ProbeFehler("Ergebnis: Zahl der Bits")
        s = c.get("samples")
        if not isinstance(s, list) or not 1 <= len(s) <= 15:
            raise ProbeFehler("Ergebnis: Schuesse")
        schuesse = []
        for h in s:
            if not isinstance(h, str) or not HEX_SCHUSS.fullmatch(h):
                raise ProbeFehler("Ergebnis: Schuss")
            v = int(h, 16)
            if v >> n:
                raise ProbeFehler("Ergebnis: Bits ueber n")
            schuesse.append(hex(v))
        out.append(schuesse)
    return out


def ergebnis_form(roh) -> str:
    if isinstance(roh, dict) and "__type__" in roh:
        return "kodiert"
    if isinstance(roh, dict) and "results" in roh:
        return "einfach"
    raise ProbeFehler("Ergebnis: Form")


def ergebnis_schuesse(roh, kreise_n: list[int]) -> list[list[str]]:
    """Beide Formen, jede streng -- wie die App (lib/ibmQuantum.ts ergebnisSchuesse)."""
    if not kreise_n or len(kreise_n) > 64 or not all(isinstance(n, int) and 1 <= n <= 16 for n in kreise_n):
        raise ProbeFehler("Ergebnis: Schaltkreise")
    if ergebnis_form(roh) == "kodiert":
        return kodierte_schuesse(roh, kreise_n)
    return einfache_schuesse(roh, kreise_n)


def mehrheit_x(kreise_n: list[int], schuesse_hex: list[list[str]], r: int) -> list[int]:
    x = []
    for n, schuesse in zip(kreise_n, schuesse_hex):
        if len(schuesse) != r or r % 2 == 0:
            raise ProbeFehler(f"{len(schuesse)} statt {r} Schuesse")
        bits = [[(int(h, 16) >> i) & 1 for i in range(n)] for h in schuesse]
        x += [1 if 2 * sum(b[i] for b in bits) > r else 0 for i in range(n)]
    return x


# ------------------------------------------------------------ Selbsttest

def selbsttest() -> None:
    e = json.load(open(ERGEBNIS))
    soll = [f["schuesse_hex"] for f in e["erwartet"]]
    ist = ergebnis_schuesse(e["ergebnis"], [f["n"] for f in e["erwartet"]])
    assert ist == soll, "Lesen weicht vom Vektor ab"
    kaputt = json.loads(json.dumps(e["ergebnis"]))
    kaputt["__value__"]["pub_results"][0]["__value__"]["data"]["__value__"]["fields"]["c"]["__value__"]["num_bits"] = 15
    try:
        ergebnis_schuesse(kaputt, [f["n"] for f in e["erwartet"]])
        raise AssertionError("falsche Bitzahl angenommen")
    except ProbeFehler:
        pass
    einf = json.load(open(ERGEBNIS_EINFACH))
    assert ergebnis_schuesse(einf["ergebnis"], [f["n"] for f in einf["erwartet"]]) == soll, "einfache Form weicht ab"
    for name, aendern in [
        ("Bits ueber n", lambda r: r["results"][3]["data"]["c"]["samples"].__setitem__(0, "0x8")),
        ("anderes Register", lambda r: r["results"][0]["data"].__setitem__("meas", r["results"][0]["data"].pop("c"))),
        ("kein Hex", lambda r: r["results"][0]["data"]["c"]["samples"].__setitem__(0, "17")),
        ("zu lang", lambda r: r["results"][0]["data"]["c"]["samples"].__setitem__(0, "0x00001")),
        ("zu viele Schuesse", lambda r: r["results"][0]["data"]["c"].__setitem__("samples", ["0x1"] * 16)),
        ("ein Schaltkreis zu wenig", lambda r: r["results"].pop()),
        ("falsche Bitzahl", lambda r: r["results"][0]["data"]["c"].__setitem__("num_bits", 15)),
    ]:
        kaputt = json.loads(json.dumps(einf["ergebnis"]))
        aendern(kaputt)
        try:
            ergebnis_schuesse(kaputt, [f["n"] for f in einf["erwartet"]])
            raise AssertionError(f"einfache Form: {name} angenommen")
        except ProbeFehler:
            pass
    a = json.load(open(AUFTRAG))
    k = json.loads(a["koerper"])
    assert k["program_id"] == "sampler" and len(k["params"]["pubs"]) == len(a["kreise_n"])
    assert json.dumps(k, separators=(",", ":"), ensure_ascii=False) == a["koerper"], "Koerper nicht byte-gleich wiederherstellbar"
    suche_selbsttest()
    kennung_selbsttest()
    print(f"Selbsttest IBM-REST-Probe: gut ({len(soll)} Schaltkreise in beiden Formen gelesen, Koerper byte-gleich, Suche)")


def kennung_selbsttest() -> None:
    """Jede Anfrage traegt die eigene Kennung, auch wenn der Aufrufer Koepfe mitgibt."""
    gesehen: list = []

    class Antwort:
        status = 200

        def __enter__(self):
            return self

        def __exit__(self, *a):
            return False

        def read(self, n):
            return b"{}"

    alt = urllib.request.urlopen
    try:
        urllib.request.urlopen = lambda req, timeout: (gesehen.append(req.get_header("User-agent")), Antwort())[1]
        anfrage("https://quantum.cloud.ibm.com/api/v1/backends", "GET", {"Accept": "application/json"})
    finally:
        urllib.request.urlopen = alt
    assert gesehen == [USER_AGENT], f"Kennung falsch: {gesehen}"


def suche_selbsttest() -> None:
    """Instanzen und Geraete finden ohne Netz: anfrage durch feste Antworten ersetzt."""
    global anfrage
    c1 = "crn:v1:bluemix:public:quantum-computing:us-east:a/0123456789abcdef0123456789abcdef:01234567-89ab-cdef-0123-456789abcdef::"
    c2 = c1.replace("01234567-89ab", "11111111-89ab").replace(":us-east:", ":eu-de:")
    c3 = c1.replace(":us-east:", ":jp-tok:")
    dev = lambda n, q, **x: {"name": n, "status": {"name": "online"}, "queue_length": q, "qubits": 156, **x}
    antworten = {
        SUCHE_URL: (200, {"items": [{"crn": c2}, {"crn": c1}, {"crn": c3}, {"crn": "crn:v1:evil"}, {"name": "ohne"}]}),
        c1: (200, {"devices": [dev("ibm_a", 5), dev("ibm_b", 1, status={"name": "paused"}), dev("ibm_c", 0, **{"class": "mock"}),
                               dev("ibm/x", 0), dev("ibm_klein", 0, qubits=5)]}),
        c2: (200, {"devices": [dev("ibm_d", 3)]}),
    }
    alt = anfrage
    try:
        urls: list = []
        anfrage = lambda url, m, kopf, daten=None: (urls.append(url), (lambda st, j: (st, json.dumps(j).encode()))(*antworten[kopf.get("Service-CRN", url)]))[1]
        assert instanzen_suchen("t") == [c2, c1], "Instanzen falsch (unbekannte Region muss fehlen)"
        geraete_liste("t", c2)
        assert urls[-1] == "https://eu-de.quantum.cloud.ibm.com/api/v1/backends", f"Host fuer eu-de falsch: {urls[-1]}"
        geraete_liste("t", c1)
        assert urls[-1] == "https://quantum.cloud.ibm.com/api/v1/backends", f"Host fuer us-east falsch: {urls[-1]}"
        for falsch in (c3, "crn:v1:evil"):
            try:
                api_basis(falsch)
                raise AssertionError(f"api_basis angenommen: {falsch}")
            except ProbeFehler:
                pass
        st, liste = geraete_liste("t", c1)
        assert st == 200 and [n for _, n, _ in liste] == ["ibm_klein", "ibm_a"], f"Geraete falsch: {liste}"
        sicht: list = []
        geraete_liste("t", c1, sicht)
        assert [g["grund"] for g in sicht] == ["frei", "status", "attrappe", "name", "frei"], f"Sicht falsch: {sicht}"
        antworten[c1] = (200, {"devices": [dev("ibm_e", None), dev("ibm_f", 0, status="online"), "x", dev("ibm_g\n\x1b", 0)]})
        sicht = []
        assert geraete_liste("t", c1, sicht) == (200, [])
        assert [g["grund"] for g in sicht] == ["schlange", "status", "kein Objekt", "name"], f"Sicht falsch: {sicht}"
        assert sicht[0]["schlange"] == "NoneType" and sicht[3]["name"] == "ibm_g", f"Sicht nicht bereinigt: {sicht}"
        antworten[c1] = (200, {"devices": [dev("ibm_a", 5), dev("ibm_b", 1, status={"name": "paused"}), dev("ibm_c", 0, **{"class": "mock"}),
                               dev("ibm/x", 0), dev("ibm_klein", 0, qubits=5)]})
        for falsch in ({"items": [{"crn": "crn:v1:evil"}]}, {"items": [{"crn": c1}] * 101}, {"items": {}}):
            antworten[SUCHE_URL] = (200, falsch)
            try:
                instanzen_suchen("t")
                raise AssertionError(f"Suche angenommen: {str(falsch)[:40]}")
            except ProbeFehler:
                pass
        antworten[c1] = (403, {"errors": [{"message": "Forbidden\x1b[31m: kein Zugang"}]})
        sicht = []
        assert geraete_liste("t", c1, sicht) == (403, [])
        assert sicht == [{"grund": "http 403", "fehler": "Forbidden[31m: kein Zugang"}], f"403-Grund falsch: {sicht}"
        assert fehler_text(b"<html><body><h1>Access Denied</h1>\x00 Ref 1</body></html>") == "Access Denied Ref 1"
        antworten[c1] = (200, {"devices": [dev("ibm_a", 1)] * 257})
        try:
            geraete_liste("t", c1)
            raise AssertionError("zu lange Geraeteliste angenommen")
        except ProbeFehler:
            pass
    finally:
        anfrage = alt


# ------------------------------------------------------------ Probe

# Eigene Kennung statt "Python-urllib/x" (die weisen vorgeschaltete Schutzsysteme oft mit 403 ab).
USER_AGENT = "aequitas-ibm-rest-probe/1"
# Letzte Fehlerantwort (nur Status, Server-Kopf, bereinigter Textanfang) fuer den Bericht.
LETZTER_FEHLER: dict = {}


def anfrage(url: str, methode: str, kopf: dict, daten: bytes | None = None) -> tuple[int, bytes]:
    req = urllib.request.Request(url, data=daten, method=methode, headers={"User-Agent": USER_AGENT, **kopf})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, r.read(8 * 1024 * 1024 + 1)
    except urllib.error.HTTPError as e:
        roh = e.read(65536)
        LETZTER_FEHLER.clear()
        LETZTER_FEHLER.update({"http": e.code, "server": _kurz(e.headers.get("server", "")),
                               "text": fehler_text(roh)})
        return e.code, roh


def fehler_text(roh: bytes) -> str:
    """Grund aus einer Fehlerantwort: JSON-Meldung, sonst Textanfang ohne Tags; nur druckbar, gekuerzt."""
    g = ibm_grund(roh)
    if g:
        return g
    t = re.sub(r"<[^>]{0,200}>", " ", roh[:4096].decode("utf-8", "replace"))
    return re.sub(r"\s+", " ", re.sub(r"[^\x20-\x7e]", " ", t)).strip()[:200]


def ibm_grund(roh: bytes) -> str:
    try:
        j = json.loads(roh)
        m = (j.get("errors") or [{}])[0].get("message") or j.get("message") or j.get("errorMessage") or ""
    except Exception:
        m = ""
    return re.sub(r"[^\x20-\x7e]", "", str(m))[:300]


def api_basis(crn: str) -> str:
    """API-Host der Instanz nach ihrer Region; unbekannte Region wird abgewiesen."""
    if not CRN_FORM.fullmatch(crn):
        raise ProbeFehler("CRN: Form")
    basis = API_BASIS_JE_REGION.get(crn.split(":")[5])
    if not basis:
        raise ProbeFehler(f"CRN: unbekannte Region {_kurz(crn.split(':')[5])}")
    return basis


def instanzen_suchen(tok: str) -> list[str]:
    """Wie die App (instanzenSuchen): IBM Cloud Global Search nach Quantum-Instanzen."""
    st, roh = anfrage(SUCHE_URL, "POST", {"Authorization": f"Bearer {tok}", "Content-Type": "application/json", "Accept": "application/json"},
                      json.dumps({"query": "service_name:quantum-computing", "fields": ["crn", "name"]}).encode())
    if st != 200:
        raise ProbeFehler(f"Instanzen: HTTP {st} {ibm_grund(roh)}")
    items = json.loads(roh).get("items")
    if not isinstance(items, list) or len(items) > 100:
        raise ProbeFehler("Instanzen: Antwort")
    crns = sorted({i.get("crn") for i in items if isinstance(i, dict) and isinstance(i.get("crn"), str) and CRN_FORM.fullmatch(i["crn"])
             and i["crn"].split(":")[5] in API_BASIS_JE_REGION})
    if not crns:
        raise ProbeFehler("Instanzen: keine Quantum-Instanz im Konto")
    return crns[:8]


def _kurz(v) -> str:
    """Fremden Wert fuer den Bericht: nur druckbares ASCII, hoechstens 40 Zeichen."""
    return re.sub(r"[^\x20-\x7e]", "", v if isinstance(v, str) else type(v).__name__)[:40]


def geraete_liste(tok: str, crn: str, sicht: list | None = None) -> tuple[int, list[tuple[int, str, int]]]:
    """Wie die App (geraeteListe): freie Geraete (online, keine Attrappe), kuerzeste Schlange zuerst.

    Mit sicht: je Geraet Name, Status, Schlange, Qubits und der Grund, warum es nicht frei ist
    (fuer den Bericht, wenn kein Geraet passt; oeffentliche Geraetedaten, nichts Geheimes)."""
    st, roh = anfrage(f"{api_basis(crn)}/backends", "GET", {"Authorization": f"Bearer {tok}", "Service-CRN": crn,
                                                       "IBM-API-Version": API_VERSION, "Accept": "application/json"})
    if st != 200:
        if sicht is not None:
            sicht.append({"grund": f"http {st}", "fehler": fehler_text(roh)})
        return st, []
    devices = json.loads(roh).get("devices")
    if not isinstance(devices, list) or len(devices) > 256:
        raise ProbeFehler("Geraete: Antwort")
    frei = []
    for d in devices:
        grund = ""
        if not isinstance(d, dict):
            grund = "kein Objekt"
        else:
            n, q, qb = d.get("name"), d.get("queue_length"), d.get("qubits")
            stat = d.get("status")
            if not isinstance(stat, dict) or stat.get("name") != "online":
                grund = "status"
            elif d.get("class") == "mock":
                grund = "attrappe"
            elif not (isinstance(n, str) and GERAET_FORM.fullmatch(n)):
                grund = "name"
            elif not (isinstance(q, int) and q >= 0):
                grund = "schlange"
            elif not (isinstance(qb, int) and qb >= 1):
                grund = "qubits"
            else:
                frei.append((q, n, qb))
        if sicht is not None:
            e = {"grund": grund or "frei"}
            if isinstance(d, dict):
                stat = d.get("status")
                e.update({"name": _kurz(d.get("name")), "status": _kurz(stat.get("name") if isinstance(stat, dict) else stat),
                          "schlange": d.get("queue_length") if isinstance(d.get("queue_length"), int) else _kurz(d.get("queue_length")),
                          "qubits": d.get("qubits") if isinstance(d.get("qubits"), int) else _kurz(d.get("qubits"))})
            sicht.append(e)
    return st, sorted(frei)


def probe(ausgabe: str) -> None:
    schl, crn, backend = (os.environ.get(k, "").strip() for k in ("IBM_QUANTUM_TOKEN", "IBM_QUANTUM_INSTANCE", "IBM_QUANTUM_BACKEND"))
    if not schl:
        raise ProbeFehler("IBM_QUANTUM_TOKEN noetig")
    if crn and not CRN_FORM.fullmatch(crn):
        raise ProbeFehler("IBM_QUANTUM_INSTANCE: keine Quantum-CRN")
    if backend and not GERAET_FORM.fullmatch(backend):
        raise ProbeFehler("IBM_QUANTUM_BACKEND: Form")
    a = json.load(open(AUFTRAG))
    min_qubits = max(a["kreise_n"])

    st, roh = anfrage(IAM_URL, "POST", {"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
                      ("grant_type=urn%3Aibm%3Aparams%3Aoauth%3Agrant-type%3Aapikey&apikey=" + urllib.parse.quote(schl, safe="")).encode())
    if st != 200:
        raise ProbeFehler(f"IAM: HTTP {st}")
    tok = json.loads(roh)["access_token"]

    # Ohne Vorgabe wie die App: Instanz und Geraet selbst finden (kuerzeste Schlange, genug Qubits).
    bericht = {"api_version": API_VERSION, "instanz": "Variable" if crn else "Global Search",
               "geraet_quelle": "Variable" if backend else "kuerzeste Schlange"}
    if not crn or not backend:
        best = None
        bericht["min_qubits"] = min_qubits
        bericht["instanzen"] = []  # Nummer statt CRN
        for c in ([crn] if crn else instanzen_suchen(tok)):
            sicht: list = []
            st, liste = geraete_liste(tok, c, sicht)
            bericht["instanzen"].append({"region": _kurz(c.split(":")[5]), "http": st, "geraete": sicht})
            if st in (401, 403):
                continue
            if st != 200:
                json.dump(bericht, open(ausgabe, "w"), indent=1)
                raise ProbeFehler(f"Geraete: HTTP {st}")
            for q, n, qb in liste:
                if qb >= min_qubits and (not backend or n == backend):
                    if best is None or q < best[0]:
                        best = (q, n, c)
                    break
        if best is None:
            bericht["letzter_fehler"] = dict(LETZTER_FEHLER)
            json.dump(bericht, open(ausgabe, "w"), indent=1)
            uebersicht = "; ".join(f"Instanz {i + 1} ({x['region']}): HTTP {x['http']}, " + ", ".join(
                (f"{g.get('name', '?')}={g['grund']}/{g.get('status', '?')}/{g.get('qubits', '?')}q" if "fehler" not in g
                 else f"IBM: {g['fehler'] or '(leer)'}") for g in x["geraete"][:16])
                for i, x in enumerate(bericht["instanzen"]))
            raise ProbeFehler(f"Geraete: kein freies Geraet (mindestens {min_qubits} Qubits). {uebersicht}. "
                             f"Server: {LETZTER_FEHLER.get('server', '?')}"[:1500])
        bericht["schlange"], backend, crn = best
    bericht["backend"] = backend
    k = json.loads(a["koerper"])
    k["backend"] = backend
    koerper = json.dumps(k, separators=(",", ":"), ensure_ascii=False).encode()
    bericht["koerper_bytes"] = len(koerper)
    kopf = {"Authorization": f"Bearer {tok}", "Service-CRN": crn, "IBM-API-Version": API_VERSION, "Accept": "application/json"}
    st, roh = anfrage(f"{api_basis(crn)}/jobs", "POST", {**kopf, "Content-Type": "application/json"}, koerper)
    bericht["einreichen_http"] = st
    if st not in (200, 201):
        bericht["ibm_grund"] = ibm_grund(roh)
        json.dump(bericht, open(ausgabe, "w"), indent=1)
        raise ProbeFehler(f"Auftrag abgelehnt: HTTP {st} {bericht['ibm_grund']}")
    job = json.loads(roh)["id"]
    bericht["job"] = job
    ende = time.time() + WARTEN_MAX_S
    while True:
        st, roh = anfrage(f"{api_basis(crn)}/jobs/{job}", "GET", kopf)
        zustand = json.loads(roh).get("state", {}) if st == 200 else {}
        status = str(zustand.get("status", "")).upper()
        if status in ("COMPLETED", "FAILED", "CANCELLED") or time.time() > ende:
            break
        time.sleep(15)
    bericht["status"] = status
    if status != "COMPLETED":
        bericht["grund"] = re.sub(r"[^\x20-\x7e]", "", str(zustand.get("reason", "")))[:300]
        json.dump(bericht, open(ausgabe, "w"), indent=1)
        raise ProbeFehler(f"Auftrag {job}: {status} {bericht.get('grund', '')}")
    st, roh = anfrage(f"{api_basis(crn)}/jobs/{job}/results", "GET", kopf)
    if st != 200:
        raise ProbeFehler(f"Ergebnis: HTTP {st}")
    ergebnis = json.loads(roh)
    bericht["ergebnis"] = ergebnis  # nur Messdaten und Metadaten des Auftrags, nichts Geheimes
    bericht["ergebnis_form"] = ergebnis_form(ergebnis)
    schuesse = ergebnis_schuesse(ergebnis, a["kreise_n"])
    x = mehrheit_x(a["kreise_n"], schuesse, a["schuesse"])
    treffer = [i for i in range(len(x)) if a["u"][i] == a["y"][i]]
    fehler = sum(x[i] != a["t"][i] for i in treffer)
    bericht.update({"schuesse_hex": schuesse, "treffer": len(treffer), "fehler": fehler})
    json.dump(bericht, open(ausgabe, "w"), indent=1)
    text = (f"## IBM-REST-Probe auf {backend}\n\nAuftrag `{job}` (Koerper der App, {len(koerper)} Byte): angenommen, "
            f"{status}. Gelesen wie die App ({bericht['ergebnis_form']}e Form): {len(schuesse)} Schaltkreise. Physik: {fehler} Fehler bei {len(treffer)} Treffern "
            f"(Grenze {FEHLERRATE_MAX:.0%}).\n")
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        open(os.environ["GITHUB_STEP_SUMMARY"], "a").write(text)
    if not treffer or fehler > FEHLERRATE_MAX * len(treffer):
        raise ProbeFehler("Physik: zu viele Fehler bei passender Basis")


if __name__ == "__main__":
    try:
        if "--selbsttest" in sys.argv:
            selbsttest()
        elif "--probe" in sys.argv:
            probe(sys.argv[sys.argv.index("--probe") + 1])
        else:
            print(__doc__)
    except ProbeFehler as e:
        print(f"::error::{e}")
        sys.exit(1)
