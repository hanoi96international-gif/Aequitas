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
hardware.bezahlen),
--geschuetzt phase|bit|beide (Speicher mit Fehlerkorrektur: je Wartezeit
die ungeschuetzte Note gegen dieselbe Note im 3-Qubit-Code; ohne
--warten-reihe 0,20,50,100,200 us; hardware.praegen_geschuetzt),
--wiesner (Variante W, wiesner_hardware.py: BB84-Note praegen, halten,
mit --bezahlen auch teleportieren, klassisch pruefen; misst die Fehlerrate
je Treffer nach jeder Wartezeit und laesst die erste Note durch die Pruefung
der Kette auf Anteilen laufen; --qubits N je Note, Standard 6, hoechstens 16),
--wiesner --fern (W3, Praegen ohne Vertrauen: je Stelle ein verschraenktes
Paar, der Validator misst seine Haelfte in eigener Zufallsbasis; das Geheimnis
kennt nur er, nicht die Praegestelle; wiesner_hardware.note_fern),
--wiesner --steane (B6, Schutz je Stelle: dieselbe Note ungeschuetzt und mit
jeder Stelle im Steane-Code [[7,1,3]], transversal gemessen und klassisch
dekodiert; --qubits = Stellen, Standard und hoechstens 2, also 14 Qubits;
wiesner_hardware.note_steane),
--s-token [--k K] (Variante S, s_kette.py: 2K Qubits, sofort gemessen, kein
Speicher; Vorlage an der gewaehlten Annahmestelle und als Angriff an der
anderen; Standard K = 840, hoechstens 2048),
--s-abschnitte VERZEICHNIS [--k K] [--v V] [--ausgaben A] (Ausgabe in
Abschnitten: je Ausgabe und Validator ein eigener Auftrag fuer seinen
Abschnitt, mess_<x>.json je Validator mit allen A Abschnitten und
basen_<a>.json je Ausgabe fuer das Werkzeug; A hoechstens 4; DEZENTRAL.md 8-12),
--s-abschnitt VERZEICHNIS --basen Y (Betrieb: EIN Validator misst SEINEN
Abschnitt in den Basen Y der Inhaberin, Text aus 0 und 1, und legt die
Messung als neue Datei <sha256(Y)>.json in sein Messquellen-Verzeichnis
AEQUITAS_QS_MESSQUELLE; nie wird etwas ueberschrieben; DEZENTRAL.md 13),
--wiederholen R (Variante S, Standard 5, ungerade bis 15: je Stelle R Schuesse
in derselben Basis der Inhaberin, die Mehrheit entscheidet -- senkt den
Auslesefehler, ohne ueber t mehr zu verraten; s_messen_basen),
--s-messen DATEI [--k K] (nur die Messung von 2K Qubits fuer die echte
Ausgabe auf der Kette: u, t, y, x als JSON, Zufall aus dem Betriebssystem;
weiter mit aequitasd quanten-s-ausgabe, deploy/quanten-s-ausgabe.sh),
--wiesner --kette (W4 ohne Partner: der ganze Ablauf mit der Kettenseite
wiesner_kette.KetteW -- Praegen auf der Kette, Bezahlen per Teleportation,
Einloesen beim Empfaenger: Aufforderung aus dem Block, EIN Schuss als
Messung der Note, Offenlegen, Urteil, AEQ beim Empfaenger).

Mit echter Maschine oder Rauschmodell wird zusaetzlich die Auslese der
Pruefqubits kalibriert (2 Schaltkreise) und die Annahme auch
lesekorrigiert ausgegeben (hardware.lesekorrektur).

Ausgabe: je Fall die gemessene Annahmequote gegen ideal; beim Praegen die
Verteilung der Seriennummern; Tiefe und 2-Qubit-Gatter nach Uebersetzung.

Aufruf: python3 -I hardware_ibm.py --ideal | --rauschen FakeTorino | --ibm
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
import random
import secrets
import sys

_hier = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("hardware", os.path.join(_hier, "hardware.py"))
hw = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(hw)
_spec_w = importlib.util.spec_from_file_location("wiesner_hardware", os.path.join(_hier, "wiesner_hardware.py"))
wh = importlib.util.module_from_spec(_spec_w)
_spec_w.loader.exec_module(wh)
_spec_k = importlib.util.spec_from_file_location("wiesner_kette", os.path.join(_hier, "wiesner_kette.py"))
wk = importlib.util.module_from_spec(_spec_k)
_spec_k.loader.exec_module(wk)
_spec_s = importlib.util.spec_from_file_location("s_kette", os.path.join(os.path.dirname(os.path.abspath(__file__)), "s_kette.py"))
sk = importlib.util.module_from_spec(_spec_s)
_spec_s.loader.exec_module(sk)
MAX_S_STELLEN = 8192     # k je Annahmestelle; Token = 2k Qubits in Schaltkreisen zu 16
                         # (2k <= quantenWNMax = 16384; 3 von 4 Validatoren brauchen k = 5200)
MAX_S_AUSGABEN = 4       # --s-abschnitte: unabhaengige Ausgaben je Lauf (Pilot: eine je Test)
S_WIEDERHOLUNGEN = 5     # Variante S: Schuesse je Stelle, Mehrheit entscheidet (s_messen_basen)
MAX_S_WIEDERHOLUNGEN = 15

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
        elif n == "ccx":
            qc.ccx(qs[0], qs[1], qs[2])
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


def schutz_codes() -> list[str] | None:
    """--geschuetzt phase|bit|beide -> die Codes; None ohne die Option."""
    if "--geschuetzt" not in sys.argv:
        return None
    wert = sys.argv[sys.argv.index("--geschuetzt") + 1]
    if wert == "beide":
        return list(hw.CODES)
    if wert not in hw.CODES:
        raise SystemExit("--geschuetzt: phase, bit oder beide")
    return [wert]


def fall_fuer_ideal(name: str) -> str:
    """Fallname der Tabelle -> Fall fuer hw.erwartet. Jede echte, gepraegte
    Note (auch nach Warten, geschuetzt oder ungeschuetzt, bezahlt) hat ideal
    die Annahme der echten Note. Bis zum 15.11.2026 stuerzte die Auswertung
    bei --geschuetzt hier ab (KeyError 'ungeschuetzt nach 0 us') -- NACH dem
    Lauf auf der Hardware, also bezahlte Rechenzeit ohne Ergebnis."""
    basis = name.split(" nach ")[0]
    if basis.startswith(("praegen", "geschuetzt", "ungeschuetzt")):
        return "echt"
    return basis


def selbsttest() -> None:
    """Ohne qiskit: jede Fallbezeichnung, die main() bilden kann, hat einen
    idealen Wert (vor jedem Hardware-Lauf, damit die Auswertung nicht erst
    nach dem bezahlten Lauf scheitert)."""
    namen = ["echt", "fremd", "basis", "praegen+pruefen", "praegen+bezahlen+pruefen"]
    for w in (0, 20, 1000):
        namen += [f"praegen+pruefen nach {w:g} us", f"ungeschuetzt nach {w:g} us"]
        namen += [f"geschuetzt({c}) nach {w:g} us" for c in hw.CODES]
    for name in namen:
        hw.erwartet(fall_fuer_ideal(name), 3, 3)
    # Variante W: die Schaltkreise entstehen ohne qiskit und sind ideal fehlerfrei.
    assert wh.selbsttest()
    # Messquelle als Verzeichnis: Name wie im Knoten (quanten_s_messverzeichnis_test.go),
    # nur neu anlegen, nie ueberschreiben, nichts Halbes liegen lassen.
    assert s_messquelle_name("0110") == "a5c3dd48facf21ed5f916d0ae979091fead570e6aea6c1d8038d1f68b26fa51f.json"
    import tempfile
    with tempfile.TemporaryDirectory() as d:
        e = {"u": "0101", "t": "0011", "y": "0110", "x": "0010", "wo": "probe", "job": ""}
        name = s_messquelle_ablegen(d, e)
        try:
            s_messquelle_ablegen(d, dict(e, x="1111"))
            raise AssertionError("Messquelle: vorhandener Eintrag ueberschrieben")
        except FileExistsError:
            pass
        with open(os.path.join(d, name)) as fh:
            assert json.load(fh) == e
        assert os.listdir(d) == [name]
        assert os.stat(os.path.join(d, name)).st_mode & 0o777 == 0o600
    for falsch in ("", "012", "01 1", "1" * (2 * MAX_S_STELLEN + 1)):
        try:
            s_basen_lesen(falsch)
            raise AssertionError(f"Basen angenommen: {falsch[:8]!r}")
        except SystemExit:
            pass
    assert s_basen_lesen("0110") == [0, 1, 1, 0]
    # Mehrheit je Stelle; gerade Zahl oder ungleiche Laengen: Abbruch.
    assert mehrheit([[0, 1, 1], [0, 1, 0], [1, 1, 0]]) == [0, 1, 0]
    assert mehrheit([[1, 0]]) == [1, 0]
    for falsch in ([], [[0], [1]], [[0, 1], [1], [0, 1]]):
        try:
            mehrheit(falsch)
            raise AssertionError(f"Mehrheit angenommen: {falsch!r}")
        except SystemExit:
            pass
    # Bei falscher Basis bleibt die Mehrheit Zufall: unabhaengig von t
    # (gleichverteilte Schuesse -> gleichverteilte Mehrheit), exakt gezaehlt.
    from itertools import product
    for r in (3, 5):
        eins = sum(mehrheit([[b] for b in s])[0] for s in product((0, 1), repeat=r))
        assert eins * 2 == 2 ** r
    assert fall_fuer_ideal("ungeschuetzt nach 0 us") == "echt"
    assert fall_fuer_ideal("fremd") == "fremd"
    print("selbsttest ok")


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
            # dynamische Schaltkreise kann nur Aer (ohne Rauschen); die
            # Matrix-Produkt-Methode rechnet auch 48 Qubits, solange wenig
            # verschraenkt ist (Note mit Teleportation), exakt.
            from qiskit_aer.primitives import SamplerV2 as AerSampler

            return None, AerSampler(options={"backend_options": {"method": "matrix_product_state"}}), "ideal (Aer ohne Rauschen)"
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


def uebersetzen(backend, qcs):
    """Mehrere Uebersetzungen, je Schaltkreis die mit den wenigsten
    2-Qubit-Gattern (dann geringster Tiefe)."""
    from qiskit.transpiler import generate_preset_pass_manager

    beste = [None] * len(qcs)
    for seed in range(VERSUCHE_UEBERSETZUNG):
        pm = generate_preset_pass_manager(optimization_level=3, backend=backend, seed_transpiler=seed)
        for i, q in enumerate(pm.run(qcs)):
            guete = (zwei_qubit(q), q.depth())
            if beste[i] is None or guete < beste[i][0]:
                beste[i] = (guete, q)
    return [b[1] for b in beste]


def wiesner_lauf(laeufe: int, schuesse: int, rng: random.Random) -> None:
    """Variante W (HEUTE.md 3.3): Fehlerrate je Treffer und Annahme je
    Wartezeit, mit und ohne Bezahlen; die erste Note durch die Kette."""
    steane = "--steane" in sys.argv
    n = zahl("--qubits", 2, wh.MAX_STELLEN_STEANE) if steane else zahl("--qubits", 6, wh.MAX_QUBITS_NOTE)
    reihe = warten_reihe() or ([0, 5, 10, 20, 50] if steane else [0, 20, 50, 100])
    bez = "--bezahlen" in sys.argv
    if steane and (bez or "--fern" in sys.argv):
        sys.exit("--steane geht nur ohne --bezahlen und --fern")
    if bez and "--dynamisch" not in sys.argv:
        # Teleportation braucht Rueckkopplung im Lauf; im Rauschmodell rechnet
        # dann nur die Matrix-Produkt-Methode (backend_und_sampler).
        sys.argv.append("--dynamisch")
    fern = "--fern" in sys.argv
    kreise = wh.satz(n, reihe, laeufe, bez, rng, fern=fern, steane=steane)
    backend, sampler, wo = backend_und_sampler()
    qcs = [nach_qiskit(s) for s in kreise]
    if backend is not None:
        qcs = uebersetzen(backend, qcs)
    job = sampler.run(qcs, shots=schuesse)
    job_id = job.job_id() if hasattr(job, "job_id") else ""
    ergebnisse = job.result()
    faelle: dict[str, dict] = {}
    erste = None
    for s, qc, r in zip(kreise, qcs, ergebnisse):
        zaehl = r.data.c.get_counts()
        e = wh.auswerten(s, zaehl)
        vorsatz = "fern: " if fern else ("steane: " if s.bedeutung["fall"] == "wiesner-steane" else ("ungeschuetzt: " if steane else ""))
        name = vorsatz + ("praegen+bezahlen+pruefen" if s.bedeutung["bezahlt"] else "praegen+pruefen") + f" nach {s.bedeutung['warten_us']:g} us"
        f = faelle.setdefault(name, {"fehler": [], "annahme": [], "tiefe": 0, "zwei": 0, "qubits": s.q})
        f["fehler"].append(e["fehlerrate"])
        f["annahme"].append(e["annahme"])
        f["tiefe"] = max(f["tiefe"], qc.depth())
        f["zwei"] = max(f["zwei"], zwei_qubit(qc))
        if erste is None:
            # Note Nr. 1: das haeufigste Ergebnis ihres Laufs durch die Kette.
            k = max(zaehl.items(), key=lambda kv: kv[1])[0]
            geheim = wh.geheimnis_aus_lauf(s, k) if fern else None
            tr, fe, ok = wh.kette_pruefen(s, wh.antworten(s, k), rng, bits=geheim)
            erste = {"warten_us": s.bedeutung["warten_us"], "bezahlt": s.bedeutung["bezahlt"], "ergebnis": k,
                     "treffer": tr, "fehler": fe, "urteil": "echt" if ok else "abgewiesen"}
    zeilen = [f"## Variante W auf {wo}", "",
              (f"Note aus {n} Stellen (BB84), ungeschuetzt je Stelle 1 Qubit, im Steane-Code je Stelle 7 Qubits, "
               if steane else f"Note aus {n} Qubits (BB84), ") + f"{laeufe} Noten je Fall, {schuesse} Schuesse je Note, "
              f"Toleranz {wh.TOLERANZ:.0%} der Treffer" + (f", Job {job_id}" if job_id else "") + ".", "",
              "| Fall | Qubits | Tiefe | 2-Qubit-Gatter | Fehlerrate je Treffer | Annahme | ideal |",
              "|---|---|---|---|---|---|---|"]
    aus = {"wo": wo, "job": job_id, "qubits_je_note": n, "steane": steane, "laeufe": laeufe, "schuesse": schuesse,
           "toleranz": wh.TOLERANZ, "faelle": {}, "note_nr_1": erste}
    for name, f in faelle.items():
        fr = sum(f["fehler"]) / len(f["fehler"])
        an = sum(f["annahme"]) / len(f["annahme"])
        zeilen.append(f"| {name} | {f['qubits']} | {f['tiefe']} | {f['zwei']} | {fr:.3f} | {an:.3f} | 0,000 / 1,000 |")
        aus["faelle"][name] = {"fehlerrate": fr, "annahme": an, "tiefe": f["tiefe"], "zwei_qubit": f["zwei"]}
    if erste:
        zeilen += ["", f"**Note Nr. 1** (nach {erste['warten_us']:g} us{', bezahlt' if erste['bezahlt'] else ''}): "
                   f"Ergebnis {erste['ergebnis']}, von den Validatoren auf Anteilen gezaehlt: "
                   f"{erste['treffer']} Treffer, {erste['fehler']} Fehler -> {erste['urteil']}."]
    zeilen += ["", "Vorfuehrung, kein Schutz: die Note ist klein und lebt Mikrosekunden, Praegestelle und Halter "
               "sind derselbe Schaltkreis, den Lauf bezeugt nur der Betreiber. Die Fehlerrate je Treffer ist die "
               "Groesse, nach der sich Toleranz und Notengroesse der Variante W richten (wiesner_verteilt.py)."]
    text = "\n".join(zeilen)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as fh:
            fh.write(text + "\n")
    if "--json" in sys.argv:
        with open(sys.argv[sys.argv.index("--json") + 1], "w") as fh:
            json.dump(aus, fh, indent=2)


def wiesner_kette_lauf(schuesse: int, rng: random.Random) -> None:
    """W4 ohne Partner (HEUTE.md, ANALYSE.md Stufenplan): Praegen auf der
    Kette -> Bezahlen (Teleportation) -> Einloesen beim Empfaenger. Die
    Praegestelle ist hier der Laeufer (vertraut, Stufe W4 Weg 1)."""
    n = zahl("--qubits", 16, wh.MAX_QUBITS_NOTE)
    if n < wk.K_MIN:
        raise SystemExit(f"--qubits: mindestens {wk.K_MIN} fuer die Pruefung")
    reihe = warten_reihe() or [20]
    warten = reihe[0]
    bez = "--bezahlen" in sys.argv
    if bez and "--dynamisch" not in sys.argv:
        sys.argv.append("--dynamisch")
    VAL = {f"v{j}": os.urandom(16) for j in range(1, 6)}
    vliste, t = list(VAL), 3
    # Labor: kleine Pruefgroesse (k = n, hoechstens 16 Qubits je Schaltkreis) --
    # NICHT sicher; sichere Groessen braucht Hunderte Stellen (wk.K_TABELLE).
    kette = wk.KetteW(VAL, labor_k=min(n, wk.K_MAX))
    sk_p, sk_e = os.urandom(16), os.urandom(16)
    kette.konto("praeger", 50_000_000, sk_p)
    k = min(n, wk.K_MAX)  # Einloesen: k fest, nicht waehlbar
    pfand = wk.pfand_fuer(10_000_000, k, n)
    kette.konto("empfaenger", pfand, sk_e)
    summe0 = kette.summe()
    log = []
    basen = [rng.randrange(2) for _ in range(n)]
    bits = [rng.randrange(2) for _ in range(n)]
    K = os.urandom(32).hex()
    pr = wk.Praegung(K, basen, bits, t, vliste, rng)
    kette.praegen("praeger", K, n, pr.wurzel, 10_000_000, vliste, t, pr.anker, pr.bestaetigungen(VAL))
    log.append(f"t={kette.zeit}s: Note {K[:12]}… gepraegt -- {n} Stellen, Deckung 10 AEQ, 5 Validatoren (je mit Anker ihrer Hash-Kette), Schwelle {t}, Merkle-Wurzel {pr.wurzel.hex()[:12]}…")
    kette.warten()
    kette.anfrage("empfaenger", K, k, wk.KetteW.sig(sk_e, f"anfrage|{K}|{k}|empfaenger|0"), "empfaenger")
    log.append(f"t={kette.zeit}s: Einloesen angefragt (Empfaenger, Pfand {pfand / 1e6:g} AEQ = Wert der {k} gepruefte Stellen)")
    kette.warten()
    kette.zufall(K, 0, pr.zufall(0))
    st, ch = kette.aufforderung_von(K)  # erst, wenn alle Validatoren ihre Hash-Kette offengelegt haben
    log.append(f"t={kette.zeit}s: Aufforderung aus den Hash-Ketten der 5 Validatoren: {k} Stellen, Basen {''.join('ZX'[b] for b in ch)}")
    aufforderung = [0] * n
    for i, beta in zip(st, ch):
        aufforderung[i] = beta
    s = wh.note(basen, bits, aufforderung, warten_us=warten, bezahlen=bez)
    backend, sampler, wo = backend_und_sampler()
    qc = nach_qiskit(s)
    if backend is not None:
        qc = uebersetzen(backend, [qc])[0]
    job = sampler.run([qc], shots=schuesse)
    job_id = job.job_id() if hasattr(job, "job_id") else ""
    r = job.result()[0]
    erster = r.data.c.get_bitstrings()[0]  # EIN Schuss = die Messung der Note
    gemessen = wh.antworten(s, erster)
    antwort = [gemessen[i] for i in st]
    kette.antwort("empfaenger", K, antwort, wk.KetteW.sig(sk_e, f"antwort|{K}|0|{antwort}"))
    log.append(f"t={kette.zeit}s: Antwort des Empfaengers -- Messwerte vom Quantenrechner ({wo}" + (f", Job {job_id}" if job_id else "") + ")")
    kette.warten()
    urteil = kette.offenlegen(K, pr.offenlegung(st, [1, 3, 5]))
    treffer = sum(basen[i] == b for i, b in zip(st, ch))
    fehler = sum(basen[i] == b and o != bits[i] for i, b, o in zip(st, ch, antwort))
    wort = {True: "ECHT -- 10 AEQ an den Empfaenger", False: "abgewiesen -- Pfand ans Grundeinkommen, Note bleibt", None: "unentschieden -- Pfand zurueck, Note bleibt"}[urteil]
    log.append(f"t={kette.zeit}s: Validatoren 1, 3, 5 legen ihre Anteile offen; jeder Knoten rechnet: {treffer} Treffer, {fehler} Fehler -> {wort}")
    stat = wh.auswerten(s, r.data.c.get_counts())
    zeilen = [f"## Quanten-Note von Anfang bis Ende auf {wo}", "",
              f"Note aus {n} Qubits (BB84), {warten:g} us gehalten" + (", per Teleportation bezahlt" if bez else "")
              + f", eingeloest mit Pruefung von {k} Stellen (Toleranz {wk.TOLERANZ:.0%}; LABORGROESSE, nicht sicher -- sicher erst ab Hunderten Stellen, wk.K_TABELLE).", ""]
    zeilen += [f"- {z}" for z in log]
    zeilen += ["", f"Kontostand Empfaenger: {kette.konten['empfaenger'] / 1e6:g} AEQ, Topf {kette.topf / 1e6:g} AEQ, "
               f"Summe erhalten: {'ja' if kette.summe() == summe0 else 'NEIN'}.",
               f"Alle {schuesse} Schuesse desselben Schaltkreises: Fehlerrate je Treffer {stat['fehlerrate']:.3f}, Annahme {stat['annahme']:.3f}.",
               "", "Vorfuehrung (Testnetz-Logik, Symbolwert): die Praegestelle ist hier der Laeufer (vertraut), die Note lebt "
               "Mikrosekunden, den Lauf bezeugt der Betreiber der Maschine. Ohne Vertrauen in die Praegestelle: "
               "Fern-Praeparation (wiesner_praegen.py, note_fern)."]
    text = "\n".join(zeilen)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as fh:
            fh.write(text + "\n")
    if "--json" in sys.argv:
        with open(sys.argv[sys.argv.index("--json") + 1], "w") as fh:
            json.dump({"wo": wo, "job": job_id, "n": n, "k": k, "warten_us": warten, "bezahlt": bez, "K": K,
                       "urteil": urteil, "treffer": treffer, "fehler": fehler, "erster_schuss": erster,
                       "fehlerrate_alle_schuesse": stat["fehlerrate"], "annahme_alle_schuesse": stat["annahme"],
                       "empfaenger_aeq": kette.konten["empfaenger"] / 1e6, "log": log}, fh, indent=2)


def s_wiederholungen() -> int:
    """--wiederholen R (ungerade, 1 bis MAX_S_WIEDERHOLUNGEN, Standard
    S_WIEDERHOLUNGEN): Schuesse je Stelle fuer die Mehrheit."""
    r = zahl("--wiederholen", S_WIEDERHOLUNGEN, MAX_S_WIEDERHOLUNGEN)
    if r % 2 == 0:
        raise SystemExit("--wiederholen muss ungerade sein (sonst keine Mehrheit)")
    return r


def mehrheit(schuesse: list[list[int]]) -> list[int]:
    """Je Stelle der Wert, den die Mehrheit der Schuesse zeigt. Ungerade Zahl
    gleich langer Schuesse, sonst Abbruch (fail-closed)."""
    if not schuesse or len(schuesse) % 2 == 0 or len({len(a) for a in schuesse}) != 1:
        raise SystemExit("Mehrheit: ungerade Zahl gleich langer Schuesse noetig")
    return [1 if 2 * sum(a[i] for a in schuesse) > len(schuesse) else 0 for i in range(len(schuesse[0]))]


def s_messen(n: int, rng: random.Random):
    """Ausgabe von n Qubits in zufaelligen BB84-Zustaenden (Basen u, Werte t),
    die Inhaberin misst SOFORT in zufaelligen Basen y (Ergebnisse x)."""
    y = [rng.randrange(2) for _ in range(n)]
    u, t, x, x1, kreise, job_id, wo = s_messen_basen(y, rng)
    return u, t, y, x, x1, kreise, job_id, wo


def s_messen_basen(y: list[int], rng: random.Random):
    """Wie s_messen, aber in den vorgegebenen Basen y der Inhaberin -- EIN
    eigener Auftrag beim Quantenrechner (Abschnitt eines Validators, A4).

    MESSFEHLER: Jeder Schaltkreis laeuft R-mal (--wiederholen, Standard 5):
    R Praeparationen desselben Zustands, JEDE in derselben Basis y gemessen;
    je Stelle entscheidet die Mehrheit. Passt die Basis (y = u), zeigt jeder
    Schuss t mit Auslesefehler p, die Mehrheit irrt nur noch mit etwa
    C(R, (R+1)/2) p^((R+1)/2) (R = 5: 10 p^3). Passt sie nicht (y != u), ist
    jeder Schuss unabhaengig von t gleichverteilt -- die Mehrheit ebenso: die
    Inhaberin erfaehrt ueber t nicht mehr als mit einem Schuss, und in der
    anderen Basis wird nie gemessen. Das geht nur, weil Praeparation und
    Messung im selben Auftrag laufen (DEZENTRAL.md 4); mit eigenem Empfaenger
    (Weg C) hat die Inhaberin ein Qubit. Rueckgabe: x (Mehrheit) und x1 (nur
    der erste Schuss, zum Vergleich der Fehlerrate)."""
    n = len(y)
    m = wh.MAX_QUBITS_NOTE
    wiederholen = s_wiederholungen()
    u = [rng.randrange(2) for _ in range(n)]
    t = [rng.randrange(2) for _ in range(n)]
    kreise = [wh.note(u[i:i + m], t[i:i + m], y[i:i + m], warten_us=0) for i in range(0, n, m)]
    backend, sampler, wo = backend_und_sampler()
    qcs = [nach_qiskit(s) for s in kreise]
    if backend is not None:
        qcs = uebersetzen(backend, qcs)
    job = sampler.run(qcs, shots=wiederholen)
    job_id = job.job_id() if hasattr(job, "job_id") else ""
    x: list[int] = []
    x1: list[int] = []
    for s, r in zip(kreise, job.result()):
        schuesse = [wh.antworten(s, b) for b in r.data.c.get_bitstrings()]
        if len(schuesse) != wiederholen:
            raise SystemExit(f"Messung: {len(schuesse)} statt {wiederholen} Schuesse")
        x += mehrheit(schuesse)
        x1 += schuesse[0]
    if len(x) != n or len(x1) != n:
        raise SystemExit(f"Messung: {len(x)} statt {n} Ergebnisse")
    if wiederholen > 1:
        wo += f" (Mehrheit aus {wiederholen} Schuessen)"
    return u, t, x, x1, kreise, job_id, wo


def s_fehler_text(y, u, t, x, x1) -> str:
    """Fehler bei passender Basis: mit Mehrheit und nur erster Schuss."""
    passend = [i for i in range(len(y)) if y[i] == u[i]]
    f = sum(x[i] != t[i] for i in passend)
    f1 = sum(x1[i] != t[i] for i in passend)
    return f"Fehler bei passender Basis {f}/{len(passend)} (ein Schuss: {f1}/{len(passend)})"


def s_messen_lauf(datei: str) -> None:
    """--s-messen DATEI: NUR messen, fuer die echte Ausgabe auf der Kette
    (aequitasd quanten-s-ausgabe). Zufall aus dem Betriebssystem. Die Datei
    enthaelt u, t (das Geheimnis der Ausgabe) und x (den Token); sie wird nur
    fuer den Eigentuemer lesbar angelegt."""
    k = zahl("--k", sk.K_STELLE, MAX_S_STELLEN)
    n = 2 * k
    u, t, y, x, x1, kreise, job_id, wo = s_messen(n, random.SystemRandom())
    bits = lambda b: "".join(map(str, b))
    fd = os.open(datei, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as fh:
        json.dump({"n": n, "u": bits(u), "t": bits(t), "y": bits(y), "x": bits(x), "wo": wo, "job": job_id}, fh)
    text = (f"## S-Messung fuer die Ausgabe auf {wo}\n\n{n} Qubits in {len(kreise)} Schaltkreisen"
            + (f", Job {job_id}" if job_id else "") + f". {s_fehler_text(y, u, t, x, x1)}.")
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as fh:
            fh.write(text + "\n")


def s_abschnitte_lauf(verzeichnis: str) -> None:
    """--s-abschnitte VERZEICHNIS [--k K] [--v V] [--ausgaben A]: Messung fuer
    die Ausgabe in Abschnitten (forschung/quantengeld/DEZENTRAL.md 8-12). Je
    Ausgabe a und Validator x ein EIGENER Auftrag fuer seinen Abschnitt
    (s_kette.abschnitte), in Basen y, die hier fuer die Inhaberin gezogen
    werden. Schreibt mess_<x>.json (u, t, y, x aller A Ausgaben -- nur fuer
    Validator x, AEQUITAS_QS_MESSQUELLE; der Dienst verbraucht jeden Abschnitt
    genau einmal) und basen_<a>.json je Ausgabe (nur y je Abschnitt -- fuer
    das Werkzeug). Der Pilot braucht zwei Ausgaben, eine je Test. Im Pilot
    laufen alle Auftraege von einem Konto aus; im Betrieb misst jeder
    Validator selbst."""
    k = zahl("--k", sk.K_STELLE, MAX_S_STELLEN)
    v = zahl("--v", 4, 64)
    ausgaben = zahl("--ausgaben", 1, MAX_S_AUSGABEN)
    n = 2 * k
    rng = random.SystemRandom()
    bits = lambda b: "".join(map(str, b))
    abschnitte = sk.abschnitte(n, v)
    mess: list[list[dict]] = [[] for _ in abschnitte]
    basen_je: list[dict] = []
    zeilen, wo = [], ""
    for a in range(1, ausgaben + 1):
        basen, jobs = [], []
        for x, ab in enumerate(abschnitte, 1):
            y = [rng.randrange(2) for _ in ab]
            u, t, xs, x1, kreise, job_id, wo = s_messen_basen(y, rng)
            mess[x - 1].append({"u": bits(u), "t": bits(t), "y": bits(y), "x": bits(xs), "wo": wo, "job": job_id})
            basen.append(bits(y))
            jobs.append(job_id)
            zeilen.append(f"- Ausgabe {a}, Validator {x}: Stellen {ab[0]}..{ab[-1]} ({len(ab)} Qubits, "
                          f"{len(kreise)} Schaltkreise" + (f", Job {job_id}" if job_id else "")
                          + f"), {s_fehler_text(y, u, t, xs, x1)}")
        basen_je.append({"n": n, "basen": basen, "wo": wo, "job": ",".join(j for j in jobs if j)})
    # Erst schreiben, wenn alle Auftraege gemessen sind: ein Abbruch
    # hinterlaesst keine halbe Ausgabe (O_EXCL, nur fuer den Eigentuemer).
    os.makedirs(verzeichnis, mode=0o700, exist_ok=True)
    def neu(name: str, inhalt: dict) -> None:
        fd = os.open(os.path.join(verzeichnis, name), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as fh:
            json.dump(inhalt, fh)
    for x, eintraege in enumerate(mess, 1):
        neu(f"mess_{x}.json", {"abschnitte": eintraege})
    for a, b in enumerate(basen_je, 1):
        neu(f"basen_{a}.json", b)
    text = "\n".join([f"## S-Messung in {v} Abschnitten auf {wo}", "",
                      f"{ausgaben} Ausgabe(n) zu je {n} Qubits, je Ausgabe und Validator ein eigener Auftrag:"] + zeilen)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as fh:
            fh.write(text + "\n")


def s_messquelle_name(y_bits: str) -> str:
    """Dateiname der Messung zu den Basen y (wie quantenSMessName im Knoten)."""
    return hashlib.sha256(y_bits.encode("ascii")).hexdigest() + ".json"


def s_messquelle_ablegen(verzeichnis: str, eintrag: dict) -> str:
    """Legt EINE Messung als neue Datei <sha256(y)>.json ins Verzeichnis:
    erst unter einem eigenen, zufaelligen Namen (O_EXCL, 0600, auf die Platte
    geschrieben), dann per os.link auf den endgueltigen Namen. os.link
    scheitert, wenn es den Namen schon gibt -- nie wird eine Messung
    ueberschrieben, und der Dienst sieht nie eine halbe Datei (er liest nur
    den endgueltigen Namen; quanten_s_messverzeichnis.go)."""
    name = s_messquelle_name(eintrag["y"])
    tmp = os.path.join(verzeichnis, f".neu-{secrets.token_hex(8)}")
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(fd, "w") as fh:
            json.dump(eintrag, fh)
            fh.flush()
            os.fsync(fh.fileno())
        os.link(tmp, os.path.join(verzeichnis, name))
    finally:
        os.unlink(tmp)
    return name


def s_basen_lesen(text: str) -> list[int]:
    """Basen der Inhaberin fuer EINEN Abschnitt: nur 0 und 1, 1 bis 2 x
    MAX_S_STELLEN Zeichen."""
    if not 1 <= len(text) <= 2 * MAX_S_STELLEN or set(text) - {"0", "1"}:
        raise SystemExit("--basen: nur 0 und 1, 1 bis %d Zeichen" % (2 * MAX_S_STELLEN))
    return [int(c) for c in text]


def s_abschnitt_lauf(verzeichnis: str) -> None:
    """--s-abschnitt VERZEICHNIS --basen Y: Betrieb (DEZENTRAL.md 13). EIN
    Validator misst SEINEN Abschnitt in einem eigenen Auftrag, in den Basen Y,
    die ihm die Inhaberin fuer seinen Abschnitt geschickt hat, und legt die
    Messung (u, t, y, x) als neue Datei in sein Messquellen-Verzeichnis
    (AEQUITAS_QS_MESSQUELLE). Danach kann der Auftrag des Ausgebers kommen;
    der Dienst verwendet die Datei genau einmal. Zufall aus dem
    Betriebssystem. Das Verzeichnis muss es geben (es gehoert dem Knoten);
    gemessen wird als Benutzer des Knotens, sonst kann er die Datei nicht
    lesen (dann weist er ab)."""
    if "--basen" not in sys.argv:
        raise SystemExit("--s-abschnitt braucht --basen Y")
    y = s_basen_lesen(sys.argv[sys.argv.index("--basen") + 1])
    if not os.path.isdir(verzeichnis) or os.path.islink(verzeichnis):
        raise SystemExit(f"{verzeichnis}: kein Verzeichnis")
    bits = lambda b: "".join(map(str, b))
    if os.path.lexists(os.path.join(verzeichnis, s_messquelle_name(bits(y)))):
        raise SystemExit("zu diesen Basen liegt schon eine Messung (nicht ueberschrieben)")
    u, t, xs, x1, kreise, job_id, wo = s_messen_basen(y, random.SystemRandom())
    name = s_messquelle_ablegen(verzeichnis, {"u": bits(u), "t": bits(t), "y": bits(y), "x": bits(xs),
                                              "wo": wo, "job": job_id})
    text = (f"## S-Messung eines Abschnitts auf {wo}\n\n{len(y)} Qubits in {len(kreise)} Schaltkreisen"
            + (f", Job {job_id}" if job_id else "") + f". {s_fehler_text(y, u, t, xs, x1)}. "
            + f"Abgelegt als {name}.")
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as fh:
            fh.write(text + "\n")


def s_token_lauf(rng: random.Random) -> None:
    """Variante S auf echter Hardware (ANALYSE.md 5b, s_kette.py): Ausgabe von
    2k Qubits in zufaelligen BB84-Zustaenden, die Nutzerin misst SOFORT in
    zufaelligen Basen (kein Speicher), dann Bindung und Wahl auf der Kette,
    Vorlage an der gewaehlten Annahmestelle (ohne Netz) und -- als Angriff --
    derselbe Token an der anderen Stelle."""
    k = zahl("--k", sk.K_STELLE, MAX_S_STELLEN)
    n = 2 * k
    m = wh.MAX_QUBITS_NOTE
    u, t, y, x, x1, kreise, job_id, wo = s_messen(n, rng)
    K = os.urandom(32).hex()
    kette = sk.KetteS()
    kette.ausgeben(K, n, "anna")
    proben = sk.stichproben(n, 2, k, rng)
    stellen = [sk.Annahmestelle(b, K, proben[b], u, t) for b in (0, 1)]
    z, b = rng.randrange(2), rng.randrange(2)
    sk.festlegen(kette, K, stellen, [yk ^ z for yk in y], b ^ z)
    echt = stellen[b].pruefen(K, x)
    doppelt = stellen[1 - b].pruefen(K, x)
    m_b, m_a = stellen[b].meldungen[-1], stellen[1 - b].meldungen[-1]
    passend = [i for i in range(n) if y[i] == u[i]]
    fehlerrate = sum(x[i] != t[i] for i in passend) / max(1, len(passend))
    wort = {True: "ANGENOMMEN", False: "abgewiesen", None: "unentschieden"}
    zeilen = [f"## Quanten-Token ohne Speicher (Variante S) auf {wo}", "",
              f"{n} Qubits in zufaelligen BB84-Zustaenden, sofort gemessen ({len(kreise)} Schaltkreise zu {m}, "
              f"je Stelle die Mehrheit der Schuesse" + (f", Job {job_id}" if job_id else "") + "). Toleranz "
              f"{sk.TOL_PROZENT} %, je Annahmestelle eine geheime Stichprobe von {k} Stellen.", "",
              f"- Fehlerrate je passender Basis ueber alle Stellen: {fehlerrate:.2%} ({s_fehler_text(y, u, t, x, x1)})",
              f"- Gewaehlte Stelle {b}: {m_b['treffer']} Treffer, {m_b['fehler']} Fehler -> {wort[echt]}",
              f"- Derselbe Token an Stelle {1 - b} (Doppelvorlage): {m_a['treffer']} Treffer, {m_a['fehler']} Fehler -> {wort[doppelt]}",
              "", "Vorfuehrung: Ausgabe und Messen laufen im selben Schaltkreis beim Betreiber der Maschine; "
              "unfaelschbar wird es erst, wenn die Nutzerin selbst misst (QKD-Empfaenger)."]
    text = "\n".join(zeilen)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as fh:
            fh.write(text + "\n")
    if "--json" in sys.argv:
        with open(sys.argv[sys.argv.index("--json") + 1], "w") as fh:
            json.dump({"wo": wo, "job": job_id, "n": n, "k": k, "tol_prozent": sk.TOL_PROZENT,
                       "fehlerrate": fehlerrate, "stelle": b, "urteil": echt, "treffer": m_b["treffer"],
                       "fehler": m_b["fehler"], "doppelvorlage": doppelt, "doppel_treffer": m_a["treffer"],
                       "doppel_fehler": m_a["fehler"]}, fh, indent=2)


def main() -> None:
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    laeufe = zahl("--laeufe", 8, MAX_LAEUFE)
    schuesse = zahl("--schuesse", 1000, MAX_SCHUESSE)
    T = zahl("--tests", 3, MAX_TESTS)
    n = 3
    rng = random.Random(int.from_bytes(os.urandom(8), "big"))
    if "--s-messen" in sys.argv:
        s_messen_lauf(sys.argv[sys.argv.index("--s-messen") + 1])
        return
    if "--s-abschnitte" in sys.argv:
        s_abschnitte_lauf(sys.argv[sys.argv.index("--s-abschnitte") + 1])
        return
    if "--s-abschnitt" in sys.argv:
        s_abschnitt_lauf(sys.argv[sys.argv.index("--s-abschnitt") + 1])
        return
    if "--s-token" in sys.argv:
        s_token_lauf(rng)
        return
    if "--wiesner" in sys.argv:
        if "--kette" in sys.argv:
            wiesner_kette_lauf(schuesse, rng)
        else:
            wiesner_lauf(laeufe, schuesse, rng)
        return
    dyn = "--dynamisch" in sys.argv
    reihe = warten_reihe()
    codes = schutz_codes()
    if codes is not None:
        # je Wartezeit und Lauf dieselben u: ungeschuetzt gegen geschuetzt
        reihe = reihe or [0, 20, 50, 100, 200]
        kreise = []
        for w in reihe:
            for _ in range(laeufe):
                us = [rng.randrange(2 ** n) for _ in range(T)]
                kreise.append(hw.praegen_dynamisch(n, us, warten_us=w))
                kreise += [hw.praegen_geschuetzt(n, us, warten_us=w, code=c) for c in codes]
    elif "--bezahlen" in sys.argv:
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
        if codes is not None:
            name = f"geschuetzt({s.bedeutung['code']})" if "code" in s.bedeutung else "ungeschuetzt"
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
        ideal = hw.erwartet(fall_fuer_ideal(name), n, T)
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
