#!/usr/bin/env python3
"""Jeder Job, der Server-Schluessel (secrets.*SSH*/*KEY*) benutzt, laeuft in der
GitHub-Umgebung `produktion` -- und nur deploy-c1-dann-c2.yml startet von selbst.

Warum (29.09.2026): 136 Workflows hatten Root-Zugang auf C1/C2, keiner brauchte
eine Freigabe, 21 starteten schon bei einem Push. Wer Schreibrechte auf GitHub
hatte, hatte damit Root auf beiden Servern. Die Umgebung `produktion` haelt die
Schluessel (Umgebungs-Secrets) und verlangt eine Freigabe; ein Job ohne sie
kommt an die Schluessel nicht heran. Diese Pruefung haelt das fuer jeden neuen
Workflow fest.
"""
import glob
import os
import re
import sys

import yaml

UMGEBUNG = "produktion"
DARF_PUSH = {"deploy-c1-dann-c2.yml"}
SCHLUESSEL = re.compile(r"secrets\.[A-Z0-9_]*(SSH|KEY)", re.I)
GEHEIM = re.compile(r"secrets\.")


def main():
    fehler = []
    for pfad in sorted(glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml")):
        name = os.path.basename(pfad)
        text = open(pfad).read()
        if not SCHLUESSEL.search(text):
            continue
        d = yaml.safe_load(text)
        on = d.get(True, d.get("on"))
        if isinstance(on, dict) and ("push" in on or "pull_request" in on or "pull_request_target" in on) \
                and name not in DARF_PUSH:
            fehler.append(f"{name}: startet von selbst (push/pull_request) -- Server-Workflows nur per workflow_dispatch")
        for job, inhalt in (d.get("jobs") or {}).items():
            if GEHEIM.search(yaml.dump(inhalt)) and (inhalt or {}).get("environment") != UMGEBUNG:
                fehler.append(f"{name}: Job '{job}' nutzt secrets ohne 'environment: {UMGEBUNG}'")
    for f in fehler:
        print(f"::error::{f}")
    print(f"{len(fehler)} Verstoesse")
    return 1 if fehler else 0


if __name__ == "__main__":
    sys.exit(main())
