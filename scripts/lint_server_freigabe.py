#!/usr/bin/env python3
"""Jeder Job, der Server-Schluessel (secrets.*SSH*/*KEY*) benutzt, laeuft in der
GitHub-Umgebung `produktion` -- und nur deploy-c1.yml startet von selbst.

Warum (29.09.2026): 136 Workflows hatten Root-Zugang auf C1/C2, keiner brauchte
eine Freigabe, 21 starteten schon bei einem Push. Wer Schreibrechte auf GitHub
hatte, hatte damit Root auf beiden Servern. Die Umgebung `produktion` haelt die
Schluessel (Umgebungs-Secrets) und verlangt eine Freigabe; ein Job ohne sie
kommt an die Schluessel nicht heran. Diese Pruefung haelt das fuer jeden neuen
Workflow fest.

Auch die Klammerform zaehlt (24.10.2026): `secrets[matrix.secret_name]` nennt
keinen Schluesselnamen, und backup-ledger.yml holte so taeglich den
Root-Schluessel von C1 -- ohne `environment: produktion`, von dieser Pruefung
unbemerkt. `--selbsttest` prueft die Pruefung an erfundenen Workflows.
"""
import glob
import os
import re
import sys

import yaml

UMGEBUNG = "produktion"
DARF_PUSH = {"deploy-c1.yml"}
# Ein Secret per Name mit SSH/KEY -- oder per Klammer, deren Name erst zur
# Laufzeit feststeht (secrets[matrix.key]); die zaehlt immer als Schluessel.
SCHLUESSEL = re.compile(r"secrets\.[A-Z0-9_]*(SSH|KEY)|secrets\s*\[", re.I)
GEHEIM = re.compile(r"secrets\s*[.\[]")


def befunde(name, text):
    if not SCHLUESSEL.search(text):
        return []
    fehler = []
    d = yaml.safe_load(text)
    on = d.get(True, d.get("on"))
    if isinstance(on, dict) and ("push" in on or "pull_request" in on or "pull_request_target" in on) \
            and name not in DARF_PUSH:
        fehler.append(f"{name}: startet von selbst (push/pull_request) -- Server-Workflows nur per workflow_dispatch")
    for job, inhalt in (d.get("jobs") or {}).items():
        if GEHEIM.search(yaml.dump(inhalt)) and (inhalt or {}).get("environment") != UMGEBUNG:
            fehler.append(f"{name}: Job '{job}' nutzt secrets ohne 'environment: {UMGEBUNG}'")
    return fehler


def selbsttest():
    def wf(env_zeile, key):
        return ("on: {workflow_dispatch: {}}\njobs:\n  j:\n    runs-on: x\n" + env_zeile +
                "    steps:\n      - env: {KEY: '${{ " + key + " }}'}\n        run: echo\n")
    mit = "    environment: produktion\n"
    faelle = [
        (wf("", "secrets.CONTABO_SSH_KEY"), 1),
        (wf("", "secrets[matrix.secret_name]"), 1),
        (wf("", "secrets [ matrix.key ]"), 1),
        (wf(mit, "secrets[matrix.secret_name]"), 0),
        (wf(mit, "secrets.CONTABO_SSH_KEY"), 0),
        (wf("", "secrets.GITHUB_TOKEN"), 0),
        ("on: {push: {}}\njobs:\n  j:\n    runs-on: x\n" + mit +
         "    steps:\n      - env: {KEY: '${{ secrets[matrix.k] }}'}\n        run: echo\n", 1),
    ]
    falsch = 0
    for i, (text, n) in enumerate(faelle):
        b = befunde("test.yml", text)
        if len(b) != n:
            falsch += 1
            print(f"Selbsttest {i}: erwartet {n} Befund(e), bekommen {b}")
    print("Selbsttest bestanden." if not falsch else f"Selbsttest: {falsch} Fehler.")
    return 1 if falsch else 0


def main():
    if sys.argv[1:] == ["--selbsttest"]:
        return selbsttest()
    fehler = []
    for pfad in sorted(glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml")):
        fehler += befunde(os.path.basename(pfad), open(pfad).read())
    for f in fehler:
        print(f"::error::{f}")
    print(f"{len(fehler)} Verstoesse")
    return 1 if fehler else 0


if __name__ == "__main__":
    sys.exit(main())
