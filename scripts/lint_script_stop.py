#!/usr/bin/env python3
"""script_stop: true zerlegt Skripte, die mehr als eine Zeile brauchen.

WAS appleboy/ssh-action DAMIT TUT

Mit `script_stop: true` teilt drone-ssh das Skript an jedem Zeilenumbruch,
schneidet jede Zeile zurecht und haengt dahinter

    DRONE_SSH_PREV_COMMAND_EXIT_CODE=$? ; if [ ... -ne 0 ]; then exit ...; fi;

-- ausser die Zeile endet mit einem Backslash. Das ist kein `set -e`. Es trifft
Zeilen, die fuer sich genommen gar kein Befehl sind:

- hinter `else` steht der Exit-Code der GESCHEITERTEN Bedingung, also 1:
  jeder else-Zweig beendet den Lauf, bevor er beginnt;
- hinter `case ... in` entsteht ein Syntaxfehler;
- in einem Heredoc landet die Pruefzeile IN DER DATEI, die geschrieben wird.

BELEGT AM 23.09.2026

wachhund-telegram-installieren.yml schrieb so ein verstuemmeltes wachhund.sh
auf beide Boxen: im gruenen Fall stieg es hinter `else` mit 1 aus, bevor es
seinen Zustand schrieb -- "wieder gruen" haette es nie gemeldet. Lokal
nachgebaut: Original Exit 0, umgeformt Exit 1. rechtstexte-setzen.yml, der
Ein-Klick-Weg fuer Impressum und Datenschutz, hatte nach der Umformung einen
Syntaxfehler und waere beim ersten Klick gescheitert. 21 Schritte in 18
Dateien waren betroffen.

Diese Pruefung baut die Umformung nach und schlaegt fehl, sobald ein Schritt
mit script_stop: true eine solche Struktur enthaelt oder danach nicht mehr
parst. Abhilfe ist immer dieselbe: script_stop: false und `set -e` (oder
`set -euo pipefail`) als erste Zeile -- das ist, was gemeint war.
"""
import glob
import os
import re
import subprocess
import sys
import tempfile

import yaml

PRUEFZEILE = ("DRONE_SSH_PREV_COMMAND_EXIT_CODE=$? ; if [ $DRONE_SSH_PREV_COMMAND_EXIT_CODE "
              "-ne 0 ]; then exit $DRONE_SSH_PREV_COMMAND_EXIT_CODE; fi;")


def umformen(skript: str) -> str:
    aus = []
    for z in skript.split("\n"):
        z = z.strip()
        if not z:
            continue
        aus.append(z)
        if not z.endswith("\\"):
            aus.append(PRUEFZEILE)
    return "\n".join(aus) + "\n"


def befunde(skript: str) -> list[str]:
    s = re.sub(r"\$\{\{[^}]*\}\}", "X", skript)
    zeilen = [z.strip() for z in s.split("\n")]
    # Kommentarzeilen zaehlen nicht: "state-only dumps are <<200MB" in
    # backup-ledger.yml ist kein Heredoc (erster Fehlalarm, 23.09.2026).
    code = [z for z in zeilen if not z.startswith("#")]
    g = []
    if any(re.search(r"<<-?\s*['\"]?[A-Za-z_]\w*", z) for z in code):
        g.append("Heredoc (die Pruefzeilen landen in der geschriebenen Datei)")
    if any(z == "else" or z.startswith(("else ", "elif ")) for z in zeilen):
        g.append("else/elif (steigt hinter else mit dem Exit-Code der Bedingung aus)")
    if re.search(r"\bcase\b.*\bin\s*$", s, re.M):
        g.append("case (Syntaxfehler nach der Umformung)")
    if re.search(r"-c '\s*\n", s):
        g.append("mehrzeiliges -c '...' (Pruefzeilen landen im Programmtext)")
    with tempfile.NamedTemporaryFile("w", suffix=".sh", delete=False) as t:
        t.write(umformen(s))
    try:
        r = subprocess.run(["bash", "-n", t.name], capture_output=True, text=True)
    finally:
        os.unlink(t.name)
    if r.returncode:
        g.append("Syntaxfehler nach der Umformung: " + r.stderr.strip().splitlines()[0].split(": ", 1)[-1])
    return g


def main() -> int:
    fehler = 0
    dateien = sorted(glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml"))
    for f in dateien:
        d = yaml.safe_load(open(f, encoding="utf-8")) or {}
        for jn, j in (d.get("jobs") or {}).items():
            for st in j.get("steps") or []:
                w = st.get("with") or {}
                if "ssh-action" not in str(st.get("uses", "")) or "script" not in w:
                    continue
                if str(w.get("script_stop", "false")).strip().lower() != "true":
                    continue
                for b in befunde(str(w["script"])):
                    fehler += 1
                    print(f"::error file={f}::{jn} / {st.get('name', '?')}: script_stop: true mit {b}. "
                          f"Abhilfe: script_stop: false und set -e als erste Skriptzeile.")
    if fehler:
        print(f"{fehler} Befund(e). Hintergrund: Kopf von scripts/lint_script_stop.py")
        return 1
    print(f"script_stop-Pruefung bestanden ({len(dateien)} Workflow-Dateien).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
