#!/usr/bin/env python3
"""Quantengeld-Schaltkreise (hardware.py) auf einer IONENFALLE (Quantinuum,
ueber Quantinuum Nexus).

Forschung, kein Produkt (BAUVORGABE §8, HARDWARE_WEG.md Schritt 1).

WARUM IONEN
Auf supraleitenden Chips (IBM) lebt eine Note ~20 us: jedes Qubit zerfaellt
allein. In einer Ionenfalle liegt die Kohaerenzzeit im Bereich von Sekunden
(Quantinuum H2/Helios). Dieselbe Note, dieselbe Pruefung -- gemessen wird,
wie lange sie dort haelt: Millisekunden bis Sekunden statt Mikrosekunden.

ABHAENGIGKEITEN (nur hier, nie im Knoten): pytket (Schaltkreise) und
qnexus (Zugang zu Nexus). Fest gepinnt mit Hashes in
quantinuum-anforderungen.txt.

Modi:
  --selbsttest        ohne Netz: jeder Schaltkreis wird nach pytket
                      uebersetzt und ZURUECK, und der Zustandsvektor-Simulator
                      aus hardware.py muss fuer beide dieselbe Verteilung
                      liefern (faengt jeden Fehler bei Winkeln, Bits,
                      Bedingungen und Warten).
  --nexus GERAET      auf Quantinuum ueber Nexus. Zugang NUR ueber die
                      Umgebung: QUANTINUUM_TOKEN (Refresh-Token aus Nexus)
                      oder QUANTINUUM_EMAIL + QUANTINUUM_PASSWORT (Konto
                      ohne zweiten Faktor). Nichts davon wird ausgegeben oder
                      auf die Platte geschrieben (CONFIG.store_tokens = False).
                      GERAET z. B. H2-1SC (Syntaxpruefung, kostenlos),
                      H2-1E (Emulator), H2-1 (Maschine).
Weitere:
  --laeufe L          Schaltkreise je Fall (Standard 4, hoechstens 16)
  --schuesse S        Schuesse je Schaltkreis (Standard 200, hoechstens 2000)
  --tests T           Pruefungen je Note (Standard 3, hoechstens 6)
  --warten-ms W1,...  Speicherzeit: die gepraegte Note liegt W Millisekunden
                      in der Falle, bevor sie geprueft wird; hoechstens 8
                      Werte, je 0..10000 ms
  --geschuetzt phase|bit|beide   wie bei IBM: ungeschuetzt gegen 3-Qubit-Code
  --hqc-je-kreis K    Kostendeckel je Schaltkreis in HQC (Standard 50)
  --json DATEI

WARTEN: Quantinuum kennt `sleep(t) q;` (hqslib1_dev). In pytket ist das eine
Barriere mit dem Text "sleep(t)". Die Zeit t wird hier in SEKUNDEN gesetzt
(SLEEP_SEKUNDEN_JE_EINHEIT). Weil ein Export mit dem falschen Kopf das
sleep still zu einer blossen Barriere macht, prueft der Laeufer nach dem
Uebersetzen bei Nexus jeden Schaltkreis: fehlt ein sleep, wird NICHT
ausgefuehrt (keine Kosten, keine falschen Zahlen).

Aufruf: python3 -I hardware_quantinuum.py --selbsttest | --nexus H2-1SC
"""
from __future__ import annotations

import importlib.util
import json
import math
import os
import random
import re
import sys
import time

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("hardware", os.path.join(_hier, "hardware.py"))
hw = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(hw)

MAX_LAEUFE = 16
MAX_SCHUESSE = 2000
MAX_TESTS = 6
MAX_WARTEN = 8
MAX_WARTEN_MS = 10_000
MAX_HQC_JE_KREIS = 500
ZEIT_S = 6 * 3600            # hoechstens so lange auf das Ergebnis warten
GERAET_MUSTER = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{1,31}$")
PROJEKT = "aequitas-quantengeld"
# hqslib1_dev: sleep(t) mit t in Sekunden. Siehe Kopf: jeder Lauf prueft,
# dass das sleep die Uebersetzung ueberlebt.
SLEEP_SEKUNDEN_JE_EINHEIT = 1.0
SLEEP_MUSTER = re.compile(r"^sleep\(([0-9.e+-]+)\)$")


# ---------------------------------------------------------------- pytket

def nach_pytket(s):
    """hardware.Schaltkreis -> pytket.Circuit. Winkel in pytket in halben
    Umdrehungen (t/pi); warten (us) -> Barriere "sleep(Sekunden)"."""
    from pytket.circuit import Circuit, OpType

    c = Circuit(s.q, len(s.mess))
    c.name = s.name[:120]
    for g in s.gatter:
        n, t, *qs = g
        if n == "h":
            c.H(qs[0])
        elif n == "x":
            c.X(qs[0])
        elif n == "cx":
            c.CX(qs[0], qs[1])
        elif n == "ccx":
            c.CCX(qs[0], qs[1], qs[2])
        elif n == "p":
            c.add_gate(OpType.U1, [t / math.pi], [qs[0]])
        elif n == "cp":
            c.add_gate(OpType.CU1, [t / math.pi], [qs[0], qs[1]])
        elif n == "barrier":
            c.add_barrier(list(qs))
        elif n == "measure":
            c.Measure(qs[0], t)
        elif n == "delay":
            sek = t * 1e-6 / SLEEP_SEKUNDEN_JE_EINHEIT
            c.add_barrier(list(qs), data=f"sleep({sek!r})")
        elif n == "wenn":
            bit, w = t
            if w == "x":
                c.add_gate(OpType.X, [qs[0]], condition_bits=[bit], condition_value=1)
            else:
                c.add_gate(OpType.U1, [w / math.pi], [qs[0]], condition_bits=[bit], condition_value=1)
        else:
            raise ValueError(f"Gatter {n} unbekannt")
    return c


def von_pytket(c, vorlage):
    """pytket.Circuit -> hardware.Schaltkreis (fuer den Selbsttest: dieselbe
    Verteilung wie die Vorlage beweist die Uebersetzung)."""
    from pytket.circuit import OpType

    s = hw.Schaltkreis(vorlage.q, vorlage.name)
    s.mess = list(vorlage.mess)
    s.bedeutung = dict(vorlage.bedeutung)

    def idx(u):
        return u.index[0]

    for cmd in c.get_commands():
        op, args = cmd.op, cmd.args
        ty = op.type
        if ty == OpType.Conditional:
            assert op.width == 1 and op.value == 1, "nur Bedingung auf genau ein Bit == 1"
            bit, ziel = idx(args[0]), idx(args[1])
            inner = op.op
            if inner.type == OpType.X:
                s.gatter.append(("wenn", (bit, "x"), ziel))
            elif inner.type == OpType.U1:
                s.gatter.append(("wenn", (bit, (float(inner.params[0]) * math.pi) % hw.TAU), ziel))
            else:
                raise ValueError(f"bedingtes {inner.type} unbekannt")
            continue
        qs = [idx(a) for a in args]
        if ty == OpType.H:
            s.gatter.append(("h", None, qs[0]))
        elif ty == OpType.X:
            s.gatter.append(("x", None, qs[0]))
        elif ty == OpType.CX:
            s.gatter.append(("cx", None, qs[0], qs[1]))
        elif ty == OpType.CCX:
            s.gatter.append(("ccx", None, qs[0], qs[1], qs[2]))
        elif ty == OpType.U1:
            s.gatter.append(("p", (float(op.params[0]) * math.pi) % hw.TAU, qs[0]))
        elif ty == OpType.CU1:
            s.gatter.append(("cp", (float(op.params[0]) * math.pi) % hw.TAU, qs[0], qs[1]))
        elif ty == OpType.Barrier:
            m = SLEEP_MUSTER.match(op.data or "")
            if m:
                s.gatter.append(("delay", float(m.group(1)) * SLEEP_SEKUNDEN_JE_EINHEIT * 1e6, *qs))
            else:
                s.gatter.append(("barrier", None, *qs))
        elif ty == OpType.Measure:
            s.gatter.append(("measure", qs[1], qs[0]))
        else:
            raise ValueError(f"{ty} unbekannt")
    return s


def sleeps(c) -> list[float]:
    """Alle sleep-Dauern (Sekunden) eines pytket-Schaltkreises."""
    from pytket.circuit import OpType

    aus = []
    for cmd in c.get_commands():
        if cmd.op.type == OpType.Barrier:
            m = SLEEP_MUSTER.match(cmd.op.data or "")
            if m:
                aus.append(float(m.group(1)))
    return aus


def zaehlung_als_text(counts, bits: int) -> dict[str, int]:
    """pytket-Zaehlung (Tupel in Bit-Reihenfolge c[0], c[1], ...) -> Schluessel
    wie Qiskit/hardware.verteilung: Bit 0 ganz rechts."""
    aus: dict[str, int] = {}
    for tup, v in counts.items():
        k = "".join(str(tup[i]) for i in reversed(range(bits)))
        aus[k] = aus.get(k, 0) + int(v)
    return aus


# ------------------------------------------------------------ Eingaben

def zahl(arg: str, standard: int, grenze: int) -> int:
    if arg not in sys.argv:
        return standard
    try:
        w = int(sys.argv[sys.argv.index(arg) + 1])
    except (ValueError, IndexError):
        raise SystemExit(f"{arg}: ganze Zahl erwartet")
    if not 1 <= w <= grenze:
        raise SystemExit(f"{arg}: 1..{grenze}")
    return w


def warten_ms(roh: str | None = None) -> list[float] | None:
    if roh is None:
        if "--warten-ms" not in sys.argv:
            return None
        try:
            roh = sys.argv[sys.argv.index("--warten-ms") + 1]
        except IndexError:
            raise SystemExit("--warten-ms: Werte fehlen")
    teile = [t.strip() for t in roh.split(",") if t.strip()]
    if not 1 <= len(teile) <= MAX_WARTEN:
        raise SystemExit(f"--warten-ms: 1..{MAX_WARTEN} Werte")
    werte = []
    for t in teile:
        try:
            w = float(t)
        except ValueError:
            raise SystemExit(f"--warten-ms: {t!r} ist keine Zahl")
        if not (0 <= w <= MAX_WARTEN_MS) or math.isnan(w):
            raise SystemExit(f"--warten-ms: 0..{MAX_WARTEN_MS}")
        werte.append(w)
    return werte


def schutz_codes() -> list[str] | None:
    if "--geschuetzt" not in sys.argv:
        return None
    wert = sys.argv[sys.argv.index("--geschuetzt") + 1]
    if wert == "beide":
        return list(hw.CODES)
    if wert not in hw.CODES:
        raise SystemExit("--geschuetzt: phase, bit oder beide")
    return [wert]


def geraet_pruefen(g: str) -> str:
    if not GERAET_MUSTER.match(g or ""):
        raise SystemExit("--nexus: Geraetename ungueltig (z. B. H2-1SC, H2-1E, H2-1)")
    return g


def kreise_bauen(n: int, T: int, laeufe: int, rng: random.Random, reihe_ms, codes):
    if codes is not None:
        reihe_ms = reihe_ms or [0, 10, 100, 1000]
        kreise = []
        for w in reihe_ms:
            for _ in range(laeufe):
                us = [rng.randrange(2 ** n) for _ in range(T)]
                kreise.append(hw.praegen_dynamisch(n, us, warten_us=w * 1000))
                kreise += [hw.praegen_geschuetzt(n, us, warten_us=w * 1000, code=c) for c in codes]
        return kreise
    if reihe_ms is not None:
        return [hw.praegen_dynamisch(n, [rng.randrange(2 ** n) for _ in range(T)], warten_us=w * 1000)
                for w in reihe_ms for _ in range(laeufe)]
    return hw.satz(n, T, laeufe, rng, dynamisch=True)


# ---------------------------------------------------------------- Nexus

def nexus_lauf(kreise, geraet: str, schuesse: int, hqc_je_kreis: float):
    """Hochladen, bei Nexus uebersetzen, sleep pruefen, ausfuehren. Gibt je
    Schaltkreis die Zaehlung und die Job-Kennung zurueck."""
    import qnexus as qnx
    from qnexus.config import CONFIG

    token = os.environ.get("QUANTINUUM_TOKEN", "").strip()
    email = os.environ.get("QUANTINUUM_EMAIL", "").strip()
    passwort = os.environ.get("QUANTINUUM_PASSWORT", "")
    if not token and not (email and passwort):
        raise SystemExit("Zugang fehlt: QUANTINUUM_TOKEN oder QUANTINUUM_EMAIL + QUANTINUUM_PASSWORT "
                         "(Umgebung quantenhardware).")
    # Nichts auf die Platte (write_token ist dann ein No-op), nichts ins Log.
    CONFIG.store_tokens = False
    if token:
        qnx.login_with_token(token)
    else:
        # Ohne Rueckfrage. Verlangt das Konto einen zweiten Faktor, scheitert
        # das hier (keine Eingabe im Workflow) -- dann den Token nehmen.
        try:
            qnx.auth.login_no_interaction(user=email, pwd=passwort)
        except (EOFError, OSError):
            raise SystemExit("Anmeldung mit Passwort verlangt einen zweiten Faktor -- "
                             "bitte QUANTINUUM_TOKEN hinterlegen.")
    del token, passwort

    projekt = qnx.projects.get_or_create(name=PROJEKT)
    qnx.context.set_active_project(projekt)
    stempel = time.strftime("%Y%m%d-%H%M%S", time.gmtime())
    pt = [nach_pytket(s) for s in kreise]
    refs = [qnx.circuits.upload(circuit=c, name=f"{stempel}-{i}") for i, c in enumerate(pt)]
    cfg = qnx.QuantinuumConfig(device_name=geraet)
    uebersetzt = list(qnx.compile(programs=refs, backend_config=cfg, name=f"uebersetzen-{stempel}", optimisation_level=2))
    if len(uebersetzt) != len(pt):
        raise SystemExit(f"Nexus lieferte {len(uebersetzt)} statt {len(pt)} uebersetzte Schaltkreise")

    # Fail-closed: jedes sleep muss die Uebersetzung ueberleben.
    for i, (vor, ref) in enumerate(zip(pt, uebersetzt)):
        soll = sorted(sleeps(vor))
        if not soll:
            continue
        ist = sorted(sleeps(ref.download_circuit()))
        if len(ist) < len(soll) or any(abs(a - b) > 1e-12 for a, b in zip(sorted(set(ist)), sorted(set(soll)))):
            raise SystemExit(f"Schaltkreis {i}: sleep {soll} nach dem Uebersetzen {ist} -- nicht ausgefuehrt "
                             "(sonst waere die Speicherzeit nicht gemessen).")

    job = qnx.start_execute_job(programs=uebersetzt, n_shots=[schuesse] * len(uebersetzt), backend_config=cfg,
                                name=f"quantengeld-{stempel}", max_cost=[float(hqc_je_kreis)] * len(uebersetzt))
    qnx.jobs.wait_for(job, timeout=ZEIT_S)
    ergebnisse = list(qnx.jobs.results(job))
    nach_id = {str(r.get_input().id): r for r in ergebnisse}
    zaehl = []
    for s, ref in zip(kreise, uebersetzt):
        r = nach_id.get(str(ref.id))
        if r is None:
            raise SystemExit(f"kein Ergebnis fuer {ref.id}")
        res = r.download_result()
        from pytket.unit_id import Bit
        counts = res.get_counts(cbits=[Bit(i) for i in range(len(s.mess))])
        zaehl.append(zaehlung_als_text(counts, len(s.mess)))
    return zaehl, str(job.id)


# ---------------------------------------------------------------- main

def auswerten(kreise, zaehl, codes, reihe_ms, n, T, laeufe, schuesse, wo, job):
    faelle: dict[str, dict] = {}
    for s, z in zip(kreise, zaehl):
        e = hw.auswerten(s, z)
        name = "praegen+pruefen" if s.bedeutung["seriebits"] else s.bedeutung["fall"]
        if codes is not None:
            name = f"geschuetzt({s.bedeutung['code']})" if "code" in s.bedeutung else "ungeschuetzt"
        if reihe_ms is not None or codes is not None:
            name += f" nach {s.bedeutung.get('warten_us', 0) / 1000:g} ms"
        f = faelle.setdefault(name, {"annahme": [], "serien": {}, "qubits": s.q})
        f["annahme"].append(e["annahme"])
        for k, v in e["serien"].items():
            f["serien"][k] = f["serien"].get(k, 0) + v
    zeilen = [f"## Quantengeld auf der Ionenfalle ({wo})", "",
              f"Zyklus N = {2 ** n}, {T} Tests je Pruefung, {laeufe} Schaltkreise je Fall, {schuesse} Schuesse je Schaltkreis"
              + (f", Job {job}" if job else "") + ".", "",
              "| Fall | Qubits | Annahme | ideal |", "|---|---|---|---|"]
    aus = {"wo": wo, "job": job, "n": n, "tests": T, "laeufe": laeufe, "schuesse": schuesse, "faelle": {}}
    for name, f in faelle.items():
        a = sum(f["annahme"]) / len(f["annahme"])
        basis = name.split(" nach ")[0]
        ideal = hw.erwartet("echt" if basis.startswith(("praegen", "geschuetzt", "ungeschuetzt")) else basis, n, T)
        zeilen.append(f"| {name} | {f['qubits']} | {a:.3f} | {ideal:.3f} |")
        aus["faelle"][name] = {"annahme": a, "ideal": ideal}
    if reihe_ms is None and codes is None and "echt" in aus["faelle"]:
        echt = aus["faelle"]["echt"]["annahme"]
        fremd = max(aus["faelle"].get("fremd", {}).get("annahme", 1), aus["faelle"].get("basis", {}).get("annahme", 1))
        aus["trennung"] = echt - fremd
        zeilen += ["", f"Trennung echt gegen beste Faelschung: {echt:.3f} - {fremd:.3f} = {echt - fremd:+.3f}."]
    if reihe_ms is not None or codes is not None:
        zeilen += ["", "Speicherzeit: Annahme der echten Note nach W Millisekunden in der Falle. "
                   "Faellt sie auf ~0,125 (Zufall bei 3 Tests) plus Rauschen, ist die Note verloren. "
                   "Zum Vergleich: auf IBM-Chips ~20 Mikrosekunden."]
    return "\n".join(zeilen), aus


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    if "--nexus" not in sys.argv:
        raise SystemExit(__doc__)
    geraet = geraet_pruefen(sys.argv[sys.argv.index("--nexus") + 1] if sys.argv.index("--nexus") + 1 < len(sys.argv) else "")
    laeufe = zahl("--laeufe", 4, MAX_LAEUFE)
    schuesse = zahl("--schuesse", 200, MAX_SCHUESSE)
    T = zahl("--tests", 3, MAX_TESTS)
    hqc = zahl("--hqc-je-kreis", 50, MAX_HQC_JE_KREIS)
    reihe = warten_ms()
    codes = schutz_codes()
    n = 3
    rng = random.Random(int.from_bytes(os.urandom(8), "big"))
    kreise = kreise_bauen(n, T, laeufe, rng, reihe, codes)
    print(f"{len(kreise)} Schaltkreise, je {schuesse} Schuesse, Deckel {hqc} HQC je Schaltkreis "
          f"(hoechstens {hqc * len(kreise)} HQC) auf {geraet}.")
    zaehl, job = nexus_lauf(kreise, geraet, schuesse, hqc)
    text, aus = auswerten(kreise, zaehl, codes, reihe, n, T, laeufe, schuesse, geraet, job)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as f:
            f.write(text + "\n")
    if "--json" in sys.argv:
        with open(sys.argv[sys.argv.index("--json") + 1], "w") as f:
            json.dump(aus, f, indent=2)


# ------------------------------------------------------------ Selbsttest

def selbsttest() -> None:
    rng = random.Random(7)
    n, T = 3, 3
    kreise = list(hw.satz(n, T, 1, rng, dynamisch=True))
    us = [1, 2, 3]
    kreise += [hw.praegen_dynamisch(n, us, warten_us=w) for w in (0, 5_000, 2_000_000)]
    kreise += [hw.praegen_geschuetzt(n, us, warten_us=1_000, code=c) for c in hw.CODES]
    kreise.append(hw.bezahlen(n, us))
    # Jedes Gatter einmal, mit Winkeln, die in der Verteilung sichtbar sind
    # (Phase zwischen zwei H): faengt einen falschen Winkelmassstab.
    alle = hw.Schaltkreis(4, "alle Gatter")
    alle.h(0); alle.p(0.7, 0); alle.h(0)
    alle.h(1); alle.h(2); alle.cp(1.3, 1, 2); alle.h(2)
    alle.x(3); alle.ccx(1, 3, 0)
    alle.barriere([0, 1, 2, 3]); alle.warten(250, [0, 1])
    alle.messen([1], [0])
    alle.h(2); alle.wenn(0, 0.9, 2); alle.h(2); alle.wenn_x(0, 3)
    alle.messen([0, 2, 3], [1, 2, 3])
    alle.bedeutung = {"fall": "echt", "pruefbits": 1, "seriebits": 0}
    kreise.append(alle)
    for s in kreise:
        c = nach_pytket(s)
        zurueck = von_pytket(c, s)
        a, b = hw.verteilung(s), hw.verteilung(zurueck)
        schluessel = set(a) | set(b)
        diff = max(abs(a.get(k, 0) - b.get(k, 0)) for k in schluessel)
        assert diff < 1e-9, f"{s.name}: Verteilung weicht ab ({diff})"
        # Warten: Dauer exakt, in Sekunden.
        soll = [g[1] * 1e-6 for g in s.gatter if g[0] == "delay"]
        assert sorted(set(sleeps(c))) == sorted(set(soll)), (s.name, sleeps(c), soll)
    # Ideal: echte Note besteht, Faelschung nicht immer (wie hardware.py).
    s = hw.praegen_dynamisch(n, us, warten_us=5_000)
    assert abs(hw.auswerten(s, hw.verteilung(von_pytket(nach_pytket(s), s)))["annahme"] - 1) < 1e-9
    # 5 ms -> sleep(0.005)
    assert sleeps(nach_pytket(s)) == [0.005]
    # Export fuer Quantinuum: mit hqslib1_dev bleibt das sleep im Text.
    from pytket.qasm import circuit_to_qasm_str
    q = circuit_to_qasm_str(nach_pytket(s), header="hqslib1_dev")
    assert "sleep(0.005)" in q and "if(c[" in q, "Export ohne sleep/Bedingung"
    # Konvention von pytket selbst: U1-Parameter in halben Umdrehungen
    # (Export u1(0.25*pi) fuer p(pi/4)) -- ein Hin-und-Rueck-Test allein
    # faende einen Fehler nicht, der in beiden Richtungen gleich falsch ist.
    eins = hw.Schaltkreis(1, "konvention")
    eins.p(math.pi / 4, 0)
    assert "u1(0.25*pi) q[0];" in circuit_to_qasm_str(nach_pytket(eins), header="hqslib1_dev")
    # Zaehlung: Bit 0 rechts.
    assert zaehlung_als_text({(1, 0, 0): 3, (0, 0, 1): 2}, 3) == {"001": 3, "100": 2}
    # Eingaben: begrenzt, fail-closed.
    assert warten_ms("0,10,1000") == [0, 10, 1000]
    for schlecht in ("", "1,2,3,4,5,6,7,8,9", "-1", "10001", "nan", "x"):
        try:
            warten_ms(schlecht)
        except SystemExit:
            continue
        raise AssertionError(f"--warten-ms {schlecht!r} angenommen")
    for g in ("H2-1SC", "H2-1E", "Helios-1"):
        assert geraet_pruefen(g) == g
    for g in ("", "a", "x;rm -rf /", "H2 1", "../h", "-H2"):
        try:
            geraet_pruefen(g)
        except SystemExit:
            continue
        raise AssertionError(f"Geraet {g!r} angenommen")
    # Ohne Token: Abbruch vor jedem Netzzugriff.
    namen = ("QUANTINUUM_TOKEN", "QUANTINUUM_EMAIL", "QUANTINUUM_PASSWORT")
    alt = {k: os.environ.pop(k, None) for k in namen}
    try:
        for teil in ({}, {"QUANTINUUM_EMAIL": "a@b.de"}, {"QUANTINUUM_PASSWORT": "geheim-4711"}):
            os.environ.update(teil)
            try:
                nexus_lauf([], "H2-1SC", 1, 1)
            except SystemExit as e:
                assert "QUANTINUUM_TOKEN" in str(e) and "geheim-4711" not in str(e)
            else:
                raise AssertionError(f"ohne vollstaendigen Zugang kein Abbruch: {sorted(teil)}")
            for k in teil:
                os.environ.pop(k)
    finally:
        for k, v in alt.items():
            if v is not None:
                os.environ[k] = v
    print("selbsttest ok")


if __name__ == "__main__":
    main()
