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
--unterdrueckung (nur --ibm: dynamische Entkopplung + Pauli-Twirling).

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


def nach_qiskit(s):
    from qiskit import QuantumCircuit

    qc = QuantumCircuit(s.q, len(s.mess), name=s.name[:60])
    for n, t, *qs in s.gatter:
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


def zahl(arg: str, standard: int, grenze: int) -> int:
    if arg not in sys.argv:
        return standard
    v = int(sys.argv[sys.argv.index(arg) + 1])
    if not 1 <= v <= grenze:
        raise SystemExit(f"{arg} muss zwischen 1 und {grenze} liegen")
    return v


def backend_und_sampler():
    if "--ideal" in sys.argv:
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
        sim = AerSimulator.from_backend(fake)
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
    kreise = hw.satz(n, T, laeufe, rng)
    backend, sampler, wo = backend_und_sampler()

    qcs = [nach_qiskit(s) for s in kreise]
    if backend is not None:
        from qiskit.transpiler import generate_preset_pass_manager

        pm = generate_preset_pass_manager(optimization_level=3, backend=backend, seed_transpiler=7)
        qcs = pm.run(qcs)
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
        ops = qc.count_ops()
        f["tiefe"] = max(f["tiefe"], qc.depth())
        f["zwei"] = max(f["zwei"], sum(v for k, v in ops.items() if k in ("cx", "cz", "ecr", "cp", "rzz")))

    zeilen = [f"## Quantengeld auf {wo}", "",
              f"Zyklus N = {2 ** n}, {T} Tests je Pruefung, {laeufe} Schaltkreise je Fall, {schuesse} Schuesse je Schaltkreis"
              + (f", Job {job_id}" if job_id else "") + ".", "",
              "| Fall | Qubits | Tiefe | 2-Qubit-Gatter | Annahme roh | Annahme lesekorrigiert | ideal |",
              "|---|---|---|---|---|---|---|"]
    aus = {"wo": wo, "job": job_id, "n": n, "tests": T, "laeufe": laeufe, "schuesse": schuesse, "faelle": {}}
    for name, f in faelle.items():
        a = sum(f["annahme"]) / len(f["annahme"])
        k = sum(f["korr"]) / len(f["korr"]) if f["korr"] else None
        ideal = hw.erwartet("echt" if name == "praegen+pruefen" else name, n, T)
        ks = f"{k:.3f}" if k is not None else "-"
        zeilen.append(f"| {name} | {f['qubits']} | {f['tiefe']} | {f['zwei']} | {a:.3f} | {ks} | {ideal:.3f} |")
        aus["faelle"][name] = {"annahme": a, "lesekorrigiert": k, "ideal": ideal, "tiefe": f["tiefe"], "zwei_qubit": f["zwei"]}
        if f["serien"]:
            gesamt = sum(f["serien"].values())
            vert = {k: v / gesamt for k, v in sorted(f["serien"].items())}
            aus["faelle"][name]["serien"] = vert
    if "praegen+pruefen" in aus["faelle"] and "serien" in aus["faelle"]["praegen+pruefen"]:
        v = aus["faelle"]["praegen+pruefen"]["serien"]
        zeilen += ["", "Seriennummern beim Praegen (ideal je 1/8 = 0,125): "
                   + ", ".join(f"{k}: {p:.3f}" for k, p in v.items())]
    echt = aus["faelle"].get("echt", {}).get("annahme", 0)
    fremd = max(aus["faelle"].get("fremd", {}).get("annahme", 1), aus["faelle"].get("basis", {}).get("annahme", 1))
    aus["trennung"] = echt - fremd
    zeilen += ["", f"Trennung echt gegen beste Faelschung (roh): {echt:.3f} - {fremd:.3f} = {echt - fremd:+.3f} "
               "(positiv: die Maschine unterscheidet echte Noten von Faelschungen)."]
    if p0 is not None:
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
