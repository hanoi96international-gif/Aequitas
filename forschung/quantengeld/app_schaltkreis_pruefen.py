#!/usr/bin/env python3
"""Gegenprobe fuer die App (Aequitas-App lib/quantenSSchaltkreis.ts, B4b-1):
die Schaltkreise eines Abschnitts, die das Handy ohne qiskit in nativen Gattern
(rz, sx, x) als OpenQASM 3 schreibt, muessen vor dem Messen als Operator gleich
wiesner_hardware.note sein, und Messbit i misst Qubit i.

Forschung, nicht im Knoten. Braucht qiskit (Version wie hardware-anforderungen.txt)
und qiskit-qasm3-import. Aufruf:
  python3 -I app_schaltkreis_pruefen.py <App>/lib/__tests__/vektoren/quanten_s_schaltkreis_vektor.json
(oder eine Liste von Faellen {u, t, y, qasm}). Exit 0 nur ohne Abweichung und mit
allen 8 Kombinationen von (u, t, y).
"""
import json, os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from qiskit import qasm3, QuantumCircuit
from qiskit.quantum_info import Operator
import wiesner_hardware as wh
import hardware_ibm as hi
daten = json.load(open(sys.argv[1]))
faelle = daten["faelle"] if isinstance(daten, dict) else daten
fehler = 0
kombis = set()
for f in faelle:
    unser = qasm3.loads(f["qasm"])
    ref = hi.nach_qiskit(wh.note(f["u"], f["t"], f["y"], warten_us=0))
    def ohne_messen(qc):
        k = QuantumCircuit(qc.num_qubits)
        for ins in qc.data:
            if ins.operation.name in ("measure", "barrier", "delay"):
                continue
            k.append(ins.operation, [k.qubits[qc.find_bit(q).index] for q in ins.qubits])
        return k
    a, b = Operator(ohne_messen(unser)), Operator(ohne_messen(ref))
    if not a.equiv(b):
        fehler += 1
        print("ungleich:", f["u"], f["t"], f["y"])
    for i in range(len(f["u"])):
        kombis.add((f["u"][i], f["t"][i], f["y"][i]))
    # Messung: Bit i misst Qubit i
    m = [(qc_ins.clbits[0], qc_ins.qubits[0]) for qc_ins in unser.data if qc_ins.operation.name == "measure"]
    for c, q in m:
        if unser.find_bit(c).index != unser.find_bit(q).index:
            fehler += 1; print("Messbit falsch")
print(f"{len(faelle)} Faelle, {len(kombis)} von 8 Kombinationen (u,t,y), Fehler: {fehler}")
sys.exit(1 if fehler or len(kombis) < 8 else 0)
