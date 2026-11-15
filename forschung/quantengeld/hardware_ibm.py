#!/usr/bin/env python3
"""Quantengeld-Schaltkreise (hardware.py) auf einem echten Quantenrechner.

Forschung, kein Produkt (BAUVORGABE §8).

Abhaengigkeit (nur hier, nie im Knoten): qiskit, qiskit-ibm-runtime, fuer
--rauschen zusaetzlich qiskit-aer. Begruendung: IBM nimmt nur Schaltkreise
an, die auf die Gatter und die Kopplung der jeweiligen Maschine uebersetzt
sind (ISA); das selbst zu schreiben waere mehr Code und mehr Fehlerquellen
als die Abhaengigkeit. Versionen fest in quantengeld-hardware.yml.

Modi:
  --ideal            Qiskit-Simulator ohne Rauschen (Gegenprobe zu hardware.py)
  --rauschen NAME    Rauschmodell einer echten IBM-Maschine (z. B. FakeTorino),
                     uebersetzt auf deren Gatter und Kopplung
  --ibm              echter Quantenrechner bei IBM. Zugang NUR ueber die
                     Umgebung: IBM_QUANTUM_TOKEN (API-Schluessel), optional
                     IBM_QUANTUM_INSTANCE (CRN) und IBM_QUANTUM_BACKEND.
                     Der Schluessel wird nie ausgegeben.
Weitere: --laeufe L (Schaltkreise je Fall, Standard 8), --schuesse S
(Standard 1000), --tests T (Standard 3), --json DATEI,
--unterdrueckung (nur --ibm: dynamische Entkopplung + Pauli-Twirling),
--dynamisch (Praegen mit halbklassischer QFT: Messen mitten im Lauf und
klassische Steuerung, ~40 % weniger 2-Qubit-Gatter; hardware.praegen_dynamisch),
--warten-reihe W1,W2,... (Speicherzeit: nur Praegen+Pruefen, die Note
liegt je W Mikrosekunden zwischen Praegen und Pruefen; hoechstens 8 Werte,
je 0..1000 us),
--bezahlen (Uebertragung: Praegen+Pruefen gegen Praegen+BEZAHLEN+Pruefen,
die Note wird per Teleportation an ein anderes Register geschickt;
hardware.bezahlen).

Mit echter Maschine oder Rauschmodell wird zusaetzlich die Auslese der
Pruefqubits kalibriert (2 Schaltkreise) und die Annahme auch
lesekorrigiert ausgegeben (hardware.lesekorrektur).

Ausgabe: je Fall die gemessene Annahmequote gegen ideal; beim Praegen die
Verteilung der Seriennummern; Tiefe und 2-Qubit-Gatter nach Uebersetzung.

Aufruf: python3 -I hardware_ibm.py --ideal | --rauschen FakeTorino | --ibm
"""
from __future__ import annotations

import importlib.util
import json
import os
import random
import sys

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("hardware", os.path.join(_hier, "hardware.py"))
hw = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(hw)

MAX_LAEUFE = 64      # Schaltkreise je Fall (Kosten auf echter Hardware!)
MAX_SCHUESSE = 4000
MAX_TESTS = 6
VERSUCHE_UEBERSETZUNG = 8
ZWEI_QUBIT = ("cx", "cz", "ecr", "cp", "rzz")


def zwei_qubit(qc) -> int:
    """2-Qubit-Gatter, auch in klassisch gesteuerten Bloecken."""
    z = 0
    for ins in qc.data:
        if ins.operation.name in ZWEI_QUBIT:
            z += 1
        for blk in getattr(ins.operation, "blocks", ()):
            z += zwei_qubit(blk)
    return z


def nach_qiskit(s):
    from qiskit import QuantumCircuit

    qc = QuantumCircuit(s.q, len(s.mess), name=s.name[:60])
    gatter = list(s.gatter)
    i = 0
    while i < len(gatter):
        n, t, *qs = gatter[i]
        if n == "wenn":
            # aufeinanderfolgende Phasen mit demselben Steuerbit in EINEN
            # klassisch gesteuerten Block (eine Rueckkopplung statt vieler)
            bit = t[0]
            block = []
            while i < len(gatter) and gatter[i][0] == "wenn" and gatter[i][1][0] == bit:
                block.append((gatter[i][1][1], gatter[i][2]))
                i += 1
            with qc.if_test((qc.clbits[bit], 1)):
                for w, a in block:
                    if w == "x":
                        qc.x(a)
                    else:
                        qc.p(w, a)
            continue
        i += 1
        if n == "h":
            qc.h(qs[0])
        elif n == "x":
            qc.x(qs[0])
        elif n == "cx":
            qc.cx(qs[0], qs[1])
        elif n == "p":
            qc.p(t, qs[0])
        elif n == "cp":
            qc.cp(t, qs[0], qs[1])
        elif n == "delay":
            for a in qs:
                qc.delay(t, a, unit="us")
        elif n == "barrier":
            qc.barrier(*qs)
        elif n == "measure":
            qc.measure(qs[0], t)
        else:
            raise ValueError(f"unbekanntes Gatter {n}")
    return qc


def mess_qubits(qc) -> dict[int, int]:
    """klassisches Bit -> physisches Qubit im uebersetzten Schaltkreis."""
    out = {}
    for ins in qc.data:
        if ins.operation.name == "measure":
            out[qc.find_bit(ins.clbits[0]).index] = qc.find_bit(ins.qubits[0]).index
    return out


def kalibrierung(backend, sampler, qubits: list[int], schuesse: int) -> tuple[dict, dict]:
    """P(lese 0|0) und P(lese 1|1) je physischem Qubit: zwei Schaltkreise,
    alle in |0> bzw. alle in |1>, auf genau den Qubits der Pruefbits."""
    from qiskit import QuantumCircuit
    from qiskit.transpiler import generate_preset_pass_manager

    kreise = []
    for eins in (False, True):
        qc = QuantumCircuit(backend.num_qubits, len(qubits))
        for i, q in enumerate(qubits):
            if eins:
                qc.x(q)
        qc.barrier(*qubits)
        for i, q in enumerate(qubits):
            qc.measure(q, i)
        kreise.append(qc)
    pm = generate_preset_pass_manager(optimization_level=0, backend=backend,
                                      initial_layout=list(range(backend.num_qubits)))
    erg = sampler.run(pm.run(kreise), shots=schuesse).result()
    p0, p1 = {}, {}
    for eins, r in zip((False, True), erg):
        zaehl = r.data.c.get_counts()
        ges = sum(zaehl.values())
        for i, q in enumerate(qubits):
            richtig = sum(v for k, v in zaehl.items() if k[-1 - i] == ("1" if eins else "0"))
            (p1 if eins else p0)[q] = richtig / ges
    return p0, p1


MAX_WARTEN_WERTE = 8
MAX_WARTEN_US = 1000


def warten_reihe() -> list[float] | None:
    if "--warten-reihe" not in sys.argv:
        return None
    roh = sys.argv[sys.argv.index("--warten-reihe") + 1]
    werte = [float(x) for x in roh.split(",") if x.strip()]
    if not 1 <= len(werte) <= MAX_WARTEN_WERTE or any(not 0 <= w <= MAX_WARTEN_US for w in werte):
        raise SystemExit(f"--warten-reihe: 1..{MAX_WARTEN_WERTE} Werte, je 0..{MAX_WARTEN_US} us")
    return werte


def zahl(arg: str, standard: int, grenze: int) -> int:
    if arg not in sys.argv:
        return standard
    v = int(sys.argv[sys.argv.index(arg) + 1])
    if not 1 <= v <= grenze:
        raise SystemExit(f"{arg} muss zwischen 1 und {grenze} liegen")
    return v


def backend_und_sampler():
    if "--ideal" in sys.argv:
        if "--dynamisch" in sys.argv:
            # dynamische Schaltkreise kann nur Aer (ohne Rauschen)
            from qiskit_aer.primitives import SamplerV2 as AerSampler

            return None, AerSampler(), "ideal (Aer ohne Rauschen)"
        from qiskit.primitives import StatevectorSampler

        return None, StatevectorSampler(), "ideal (Qiskit StatevectorSampler)"
    if "--rauschen" in sys.argv:
        from qiskit_aer import AerSimulator
        from qiskit_aer.primitives import SamplerV2 as AerSampler
        import qiskit_ibm_runtime.fake_provider as fp

        name = sys.argv[sys.argv.index("--rauschen") + 1]
        if not name.startswith("Fake") or not hasattr(fp, name):
            raise SystemExit(f"unbekanntes Rauschmodell {name!r}")
        fake = getattr(fp, name)()
        # Mit dynamischen Schaltkreisen kuerzt Aer nicht auf die benutzten
        # Qubits; die Matrix-Produkt-Methode rechnet nur verschraenkte teuer.
        methode = "matrix_product_state" if "--dynamisch" in sys.argv else "automatic"
        sim = AerSimulator.from_backend(fake, method=methode)
        return sim, AerSampler.from_backend(sim), f"Rauschmodell {fake.name}"
    if "--ibm" in sys.argv:
        from qiskit_ibm_runtime import QiskitRuntimeService, SamplerV2

        token = os.environ.get("IBM_QUANTUM_TOKEN", "")
        if not token:
            raise SystemExit("IBM_QUANTUM_TOKEN fehlt (nur ueber die Umgebung, nie als Argument)")
        kw = {"channel": "ibm_quantum_platform", "token": token}
        if os.environ.get("IBM_QUANTUM_INSTANCE"):
            kw["instance"] = os.environ["IBM_QUANTUM_INSTANCE"]
        dienst = QiskitRuntimeService(**kw)
        wunsch = os.environ.get("IBM_QUANTUM_BACKEND", "")
        be = dienst.backend(wunsch) if wunsch else dienst.least_busy(operational=True, simulator=False)
        sampler = SamplerV2(mode=be)
        wo = f"IBM {be.name}"
        if "--unterdrueckung" in sys.argv:
            # Fehlerunterdrueckung: dynamische Entkopplung in Leerlaufzeiten,
            # Pauli-Twirling der 2-Qubit-Gatter und der Messungen.
            sampler.options.dynamical_decoupling.enable = True
            sampler.options.dynamical_decoupling.sequence_type = "XY4"
            sampler.options.twirling.enable_gates = True
            sampler.options.twirling.enable_measure = True
            wo += " mit Fehlerunterdrueckung (DD XY4, Twirling)"
        return be, sampler, wo
    raise SystemExit("Modus fehlt: --ideal | --rauschen NAME | --ibm")


def main() -> None:
    laeufe = zahl("--laeufe", 8, MAX_LAEUFE)
    schuesse = zahl("--schuesse", 1000, MAX_SCHUESSE)
    T = zahl("--tests", 3, MAX_TESTS)
    n = 3
    rng = random.Random(int.from_bytes(os.urandom(8), "big"))
    dyn = "--dynamisch" in sys.argv
    reihe = warten_reihe()
    if "--bezahlen" in sys.argv:
        # je Lauf dieselben u: Praegen+Pruefen gegen Praegen+BEZAHLEN+Pruefen
        kreise = []
        for _ in range(laeufe):
            us = [rng.randrange(2 ** n) for _ in range(T)]
            kreise += [hw.praegen_dynamisch(n, us), hw.bezahlen(n, us)]
    elif reihe is None:
        kreise = hw.satz(n, T, laeufe, rng, dynamisch=dyn)
    else:
        bau = hw.praegen_dynamisch if dyn else hw.praegen_und_pruefen
        kreise = [bau(n, [rng.randrange(2 ** n) for _ in range(T)], warten_us=w)
                  for w in reihe for _ in range(laeufe)]
    backend, sampler, wo = backend_und_sampler()

    qcs = [nach_qiskit(s) for s in kreise]
    if backend is not None:
        from qiskit.transpiler import generate_preset_pass_manager

        # Mehrere Uebersetzungen (verschiedene Startwerte fuer Anordnung und
        # Wege), je Schaltkreis die mit den wenigsten 2-Qubit-Gattern (dann
        # geringster Tiefe). Rauschbewusste Qubit-Wahl macht der Uebersetzer
        # auf Stufe 3 selbst (Fehlerraten der Tageskalibrierung).
        beste = [None] * len(qcs)
        for seed in range(VERSUCHE_UEBERSETZUNG):
            pm = generate_preset_pass_manager(optimization_level=3, backend=backend, seed_transpiler=seed)
            for i, q in enumerate(pm.run(qcs)):
                guete = (zwei_qubit(q), q.depth())
                if beste[i] is None or guete < beste[i][0]:
                    beste[i] = (guete, q)
        qcs = [b[1] for b in beste]
    job = sampler.run(qcs, shots=schuesse)
    job_id = job.job_id() if hasattr(job, "job_id") else ""
    ergebnisse = job.result()

    # Lesekorrektur: Auslesefehler genau der Qubits, auf denen die
    # Pruefbits gemessen wurden (nur mit echter Maschine/Rauschmodell).
    p0 = p1 = None
    if backend is not None:
        alle = sorted({q for qc in qcs for b, q in mess_qubits(qc).items() if b < T})
        p0, p1 = kalibrierung(backend, sampler, alle, schuesse)

    faelle: dict[str, dict] = {}
    for s, qc, r in zip(kreise, qcs, ergebnisse):
        zaehl = r.data.c.get_counts()
        e = hw.auswerten(s, zaehl)
        name = "praegen+pruefen" if s.bedeutung["seriebits"] else s.bedeutung["fall"]
        if s.bedeutung["fall"] == "bezahlt":
            name = "praegen+bezahlen+pruefen"
        if reihe is not None:
            name += f" nach {s.bedeutung.get('warten_us', 0):g} us"
        f = faelle.setdefault(name, {"annahme": [], "korr": [], "serien": {}, "tiefe": 0, "zwei": 0, "qubits": s.q})
        f["annahme"].append(e["annahme"])
        if p0 is not None:
            mq = mess_qubits(qc)
            ges = sum(zaehl.values())
            rand: dict[str, float] = {}
            for k, v in zaehl.items():
                rand[k[-T:]] = rand.get(k[-T:], 0) + v / ges
            korr = hw.lesekorrektur(rand, [p0[mq[i]] for i in range(T)], [p1[mq[i]] for i in range(T)])
            f["korr"].append(korr.get("0" * T, 0.0))
        for k, v in e["serien"].items():
            f["serien"][k] = f["serien"].get(k, 0) + v
        f["tiefe"] = max(f["tiefe"], qc.depth())
        f["zwei"] = max(f["zwei"], zwei_qubit(qc))

    zeilen = [f"## Quantengeld auf {wo}", "",
              f"Zyklus N = {2 ** n}, {T} Tests je Pruefung, {laeufe} Schaltkreise je Fall, {schuesse} Schuesse je Schaltkreis"
              + (f", Job {job_id}" if job_id else "") + ".", "",
              "| Fall | Qubits | Tiefe | 2-Qubit-Gatter | Annahme roh | Annahme lesekorrigiert | ideal |",
              "|---|---|---|---|---|---|---|"]
    aus = {"wo": wo, "job": job_id, "n": n, "tests": T, "laeufe": laeufe, "schuesse": schuesse, "faelle": {}}
    for name, f in faelle.items():
        a = sum(f["annahme"]) / len(f["annahme"])
        k = sum(f["korr"]) / len(f["korr"]) if f["korr"] else None
        ideal = hw.erwartet("echt" if name.startswith("praegen") else name, n, T)
        ks = f"{k:.3f}" if k is not None else "-"
        zeilen.append(f"| {name} | {f['qubits']} | {f['tiefe']} | {f['zwei']} | {a:.3f} | {ks} | {ideal:.3f} |")
        aus["faelle"][name] = {"annahme": a, "lesekorrigiert": k, "ideal": ideal, "tiefe": f["tiefe"], "zwei_qubit": f["zwei"]}
        if f["serien"]:
            gesamt = sum(f["serien"].values())
            vert = {k: v / gesamt for k, v in sorted(f["serien"].items())}
            aus["faelle"][name]["serien"] = vert
    if reihe is None and "praegen+pruefen" in aus["faelle"] and "serien" in aus["faelle"]["praegen+pruefen"]:
        v = aus["faelle"]["praegen+pruefen"]["serien"]
        zeilen += ["", "Seriennummern beim Praegen (ideal je 1/8 = 0,125): "
                   + ", ".join(f"{k}: {p:.3f}" for k, p in v.items())]
    if reihe is not None:
        zeilen += ["", "Speicherzeit: Annahme der echten, gepraegten Note nach W Mikrosekunden im Speicher. "
                   "Faellt sie auf ~0,23 (Zufall bei 3 Tests: 1/8 + Rauschen), ist die Note verloren."]
    elif "echt" in aus["faelle"]:
        echt = aus["faelle"]["echt"]["annahme"]
        fremd = max(aus["faelle"].get("fremd", {}).get("annahme", 1), aus["faelle"].get("basis", {}).get("annahme", 1))
        aus["trennung"] = echt - fremd
        zeilen += ["", f"Trennung echt gegen beste Faelschung (roh): {echt:.3f} - {fremd:.3f} = {echt - fremd:+.3f} "
                   "(positiv: die Maschine unterscheidet echte Noten von Faelschungen)."]
    if p0 is not None and reihe is None:
        ke = aus["faelle"].get("echt", {}).get("lesekorrigiert") or 0
        kf = max(aus["faelle"].get("fremd", {}).get("lesekorrigiert") or 1, aus["faelle"].get("basis", {}).get("lesekorrigiert") or 1)
        aus["trennung_lesekorrigiert"] = ke - kf
        aus["auslese"] = {"p0": p0, "p1": p1}
        schlecht = min(min(p0.values()), min(p1.values()))
        zeilen += [f"Trennung lesekorrigiert: {ke:.3f} - {kf:.3f} = {ke - kf:+.3f}. "
                   f"Auslese der Pruefqubits: schlechtestes P(richtig) = {schlecht:.3f}. "
                   "Roh ist, was ein einzelner Pruefer erlebt; lesekorrigiert zeigt Zustand und Gatter ohne Auslesefehler."]
    text = "\n".join(zeilen)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as f:
            f.write(text + "\n")
    if "--json" in sys.argv:
        with open(sys.argv[sys.argv.index("--json") + 1], "w") as f:
            json.dump(aus, f, indent=2)


if __name__ == "__main__":
    main()
