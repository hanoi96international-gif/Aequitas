#!/usr/bin/env python3
"""Wertet `govulncheck -format json` aus (stdin) und entscheidet ueber den Build.

Rot, wenn eine vom Code ERREICHBARE Schwachstelle gefunden wird, die nicht in
.github/govulncheck-ausnahmen.json steht, oder deren Ausnahme abgelaufen ist.
Ausnahmen, die nichts mehr treffen, werden gemeldet, damit die Liste nicht
veraltet. Erreichbar heisst: govulncheck hat eine Aufrufkette bis in die
betroffene Funktion gefunden (trace[0].function gesetzt) -- dieselbe Menge,
die der Textmodus als "Your code is affected" meldet.
"""
import datetime
import json
import os
import sys

AUSNAHMEN = os.path.join(os.path.dirname(__file__), "..", ".github", "govulncheck-ausnahmen.json")


def nachrichten(text):
    dec = json.JSONDecoder()
    i = 0
    while i < len(text):
        while i < len(text) and text[i].isspace():
            i += 1
        if i >= len(text):
            break
        obj, i = dec.raw_decode(text, i)
        yield obj


def main():
    heute = datetime.date.today()
    with open(AUSNAHMEN) as f:
        ausnahmen = {a["id"]: a for a in json.load(f)["ausnahmen"]}
    titel, erreichbar = {}, {}
    for m in nachrichten(sys.stdin.read()):
        if "osv" in m:
            titel[m["osv"]["id"]] = m["osv"].get("summary", "")
        f = m.get("finding")
        if f and f.get("trace") and f["trace"][0].get("function"):
            erreichbar.setdefault(f["osv"], f.get("fixed_version", "keine"))
    rot = False
    for vid, fix in sorted(erreichbar.items()):
        a = ausnahmen.get(vid)
        if a is None:
            print(f"::error::{vid} erreichbar, behoben in {fix}: {titel.get(vid, '')}")
            rot = True
        elif datetime.date.fromisoformat(a["bis"]) < heute:
            print(f"::error::{vid}: Ausnahme abgelaufen am {a['bis']} ({a['grund']})")
            rot = True
        else:
            print(f"Ausnahme bis {a['bis']}: {vid} -- {a['grund']}")
    for vid in sorted(set(ausnahmen) - set(erreichbar)):
        print(f"::warning::Ausnahme {vid} trifft nichts mehr -- aus .github/govulncheck-ausnahmen.json entfernen")
    print(f"{len(erreichbar)} erreichbare Schwachstellen, {sum(1 for v in erreichbar if v not in ausnahmen)} ohne Ausnahme")
    return 1 if rot else 0


if __name__ == "__main__":
    sys.exit(main())
