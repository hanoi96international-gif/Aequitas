#!/usr/bin/env python3
"""Kein Workflow-Schritt mit appleboy/ssh-action darf `script_stop` setzen.

WARUM DIE EINGABE WEG MUSS (23.10.2026)

appleboy/ssh-action ab v1.2.x (wir nutzen v1.2.5, 0ff4204d) kennt
`script_stop` nicht mehr: die Eingabe steht nicht mehr in action.yml und wird
still ignoriert, GitHub schreibt nur eine Warnung ins Log. Ein Schritt, der
sich auf `script_stop: true` verlaesst, liefe nach einem gescheiterten Befehl
einfach weiter -- ein Deploy nach einem gescheiterten Build, ein Neustart nach
einer gescheiterten Sicherung. Das ist fail-open, und keine CI bemerkt es, weil
kein CI-Job die Aktion ausfuehrt.

Vor dem Update standen 37 Schritte auf `script_stop: true`. Sie brechen jetzt
im Skript selbst ab: `set -euo pipefail` (wo vorher `set -uo pipefail` stand)
oder `set -e` als erste Zeile -- das ist, was gemeint war. Die uebrigen
`script_stop: false` waren ohnehin die Voreinstellung und sind entfernt.

WAS script_stop: true IN v1.0.3 TAT (Vorgeschichte, 23.09.2026)

drone-ssh teilte das Skript an jedem Zeilenumbruch und haengte hinter jede
Zeile `DRONE_SSH_PREV_COMMAND_EXIT_CODE=$? ; if [ ... -ne 0 ]; then exit ...;
fi;`. Hinter `else` stand damit der Exit-Code der gescheiterten Bedingung,
`case ... in` wurde zum Syntaxfehler, in Heredocs landete die Pruefzeile in
der geschriebenen Datei. wachhund-telegram-installieren.yml und
rechtstexte-setzen.yml waren so zerlegt; 21 Schritte in 18 Dateien.

Diese Pruefung schlaegt fehl, sobald ein ssh-action-Schritt `script_stop`
ueberhaupt setzt, egal mit welchem Wert. Wer einen Abbruch beim ersten Fehler
will, schreibt `set -e` (oder `set -euo pipefail`) als erste Skriptzeile.

`--selbsttest` prueft die Pruefung an erfundenen Workflows.
"""
import glob
import sys

import yaml


def befunde(d: dict) -> list[str]:
    g = []
    for jn, j in (d.get("jobs") or {}).items():
        for st in (j or {}).get("steps") or []:
            if "ssh-action" not in str(st.get("uses", "")):
                continue
            w = st.get("with") or {}
            if "script_stop" in w:
                g.append(f"{jn} / {st.get('name', '?')}: script_stop: {w['script_stop']} "
                         f"(ab ssh-action v1.2 still ignoriert)")
    return g


def selbsttest() -> int:
    def wf(w):
        return {"jobs": {"j": {"steps": [{"name": "s", "uses": "appleboy/ssh-action@x", "with": w}]}}}
    faelle = [
        (wf({"script": "deploy.sh", "script_stop": True}), 1),
        (wf({"script": "deploy.sh", "script_stop": "true"}), 1),
        (wf({"script": "deploy.sh", "script_stop": False}), 1),
        (wf({"script": "set -e\ndeploy.sh"}), 0),
        ({"jobs": {"j": {"steps": [{"uses": "appleboy/scp-action@x", "with": {"script_stop": True}}]}}}, 0),
        ({"jobs": {"j": {"steps": [{"run": "true"}]}}}, 0),
    ]
    falsch = 0
    for i, (d, n) in enumerate(faelle):
        if len(befunde(d)) != n:
            falsch += 1
            print(f"Selbsttest {i}: erwartet {n} Befund(e), bekommen {befunde(d)}")
    print("Selbsttest bestanden." if not falsch else f"Selbsttest: {falsch} Fehler.")
    return 1 if falsch else 0


def main() -> int:
    if sys.argv[1:] == ["--selbsttest"]:
        return selbsttest()
    fehler = 0
    dateien = sorted(glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml"))
    for f in dateien:
        d = yaml.safe_load(open(f, encoding="utf-8")) or {}
        for b in befunde(d):
            fehler += 1
            print(f"::error file={f}::{b}. Abhilfe: Zeile streichen; fuer Abbruch beim ersten "
                  f"Fehler set -e als erste Skriptzeile.")
    if fehler:
        print(f"{fehler} Befund(e). Hintergrund: Kopf von scripts/lint_script_stop.py")
        return 1
    print(f"script_stop-Pruefung bestanden ({len(dateien)} Workflow-Dateien).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
