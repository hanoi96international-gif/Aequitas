#!/usr/bin/env python3
"""Der Push-Deploy (deploy-c1.yml) startet nur fuer Pfade in seiner Liste.

Fehlt dort etwas, das im Knoten-Image landet, wird eine Aenderung daran nie
ausgerollt (so geschehen mit grundpfeiler.md: per go:embed im Knoten, aber
von "**.md" ausgeschlossen). Steht dort zu viel, wartet nach jeder Aenderung
an Tests, Skripten oder Doku ein Deploy auf Freigabe, der C1 ohne neuen Code
neu startet.

Geprueft wird:
  - jede Datei hinter einem //go:embed (ausser in Tests) loest den Deploy aus,
  - alles, was das Dockerfile aus dem Bau uebernimmt (COPY --from=builder),
  - Go-Quellen, go.mod, go.sum, Dockerfile und die Deploy-Skripte,
  - Tests, Hilfsskripte, Doku und Workflows loesen ihn NICHT aus.

Mustersprache wie bei GitHub: * ohne /, ** mit /, ! nimmt aus, das letzte
passende Muster entscheidet.

Aufruf: python3 scripts/lint_deploy_pfade.py [--selbsttest]
"""
import glob
import os
import re
import sys

import yaml

WORKFLOW = ".github/workflows/deploy-c1.yml"

MUSS = [
    "x/humanity/keeper/block.go",
    "cmd/aequitasd/main.go",
    "go.mod",
    "go.sum",
    "Dockerfile",
    ".dockerignore",
    "deploy/validator/docker-compose.yml",
    "deploy/deploy-c1.sh",
    "deploy/pruefe-knoten.sh",
    "deploy/rueckrollen-c1.sh",
]

NIE = [
    "x/humanity/keeper/block_test.go",
    "scripts/test_caddy_coordinator.sh",
    "scripts/lint_deploy_pfade.py",
    "deploy/caddy-neuer-server.sh",
    "docs/BETA_BEREITSCHAFT.md",
    "README.md",
    ".github/workflows/ci.yml",
    ".github/workflows/deploy-c1.yml",
]


def muster_zu_regex(m):
    r = ""
    i = 0
    while i < len(m):
        if m.startswith("**", i):
            r += ".*"
            i += 2
        elif m[i] == "*":
            r += "[^/]*"
            i += 1
        elif m[i] == "?":
            r += "[^/]"
            i += 1
        else:
            r += re.escape(m[i])
            i += 1
    return re.compile(r + r"\Z")


def loest_aus(pfad, muster):
    an = False
    for m in muster:
        neg = m.startswith("!")
        if muster_zu_regex(m[1:] if neg else m).match(pfad):
            an = not neg
    return an


def pfade_aus_workflow(text):
    w = yaml.safe_load(text)
    on = w.get("on", w.get(True))
    if not isinstance(on, dict) or not isinstance(on.get("push"), dict):
        return None
    return on["push"].get("paths")


def embed_dateien(wurzel="."):
    out = []
    for go in glob.glob(os.path.join(wurzel, "**", "*.go"), recursive=True):
        if go.endswith("_test.go") or "/node_modules/" in go:
            continue
        with open(go, encoding="utf-8", errors="replace") as f:
            for zeile in f:
                z = zeile.strip()
                if not z.startswith("//go:embed "):
                    continue
                for ziel in z[len("//go:embed "):].split():
                    basis = os.path.join(os.path.dirname(go), ziel)
                    treffer = [p for p in glob.glob(basis, recursive=True) if os.path.isfile(p)]
                    if os.path.isdir(basis):
                        treffer += [p for p in glob.glob(os.path.join(basis, "**"), recursive=True) if os.path.isfile(p)]
                    if not treffer:
                        out.append((go, ziel, None))
                    for t in treffer:
                        out.append((go, ziel, os.path.relpath(t, wurzel)))
    return out


def dockerfile_uebernahmen(text):
    out = []
    for z in text.splitlines():
        m = re.match(r"\s*COPY\s+--from=builder\s+/app/(\S+)\s+", z)
        if m and m.group(1) != "aequitasd":
            out.append(m.group(1))
    return out


def pruefe(muster, embeds, kopien):
    fehler = []
    if not muster:
        return ["deploy-c1.yml: on.push.paths fehlt -- jeder Push loeste den Deploy aus"]
    for p in MUSS:
        if not loest_aus(p, muster):
            fehler.append(f"{p}: muss den Deploy ausloesen")
    for p in NIE:
        if loest_aus(p, muster):
            fehler.append(f"{p}: darf den Deploy nicht ausloesen")
    for go, ziel, datei in embeds:
        if datei is None:
            fehler.append(f"{go}: go:embed {ziel} findet keine Datei")
        elif not loest_aus(datei, muster):
            fehler.append(f"{datei} (go:embed in {go}) loest den Deploy nicht aus")
    for k in kopien:
        probe = k if "." in os.path.basename(k) else k + "/x"
        if not loest_aus(probe, muster):
            fehler.append(f"{k} (Dockerfile, COPY --from=builder) loest den Deploy nicht aus")
    return fehler


def selbsttest():
    gut = ["**.go", "!**_test.go", "x/a/assets/**", "genesis.json", "downloads/**",
           "go.mod", "go.sum", "Dockerfile", ".dockerignore", "deploy/validator/**",
           "deploy/deploy-c1.sh", "deploy/pruefe-knoten.sh", "deploy/rueckrollen-c1.sh"]
    embeds = [("x/a/b.go", "assets/s.md", "x/a/assets/s.md")]
    kopien = ["genesis.json", "downloads"]
    faelle = [
        ("vollstaendig", gut, 0),
        ("ohne Liste", None, 1),
        ("Ausschlussliste wie bis 25.10.", ["**", "!.github/**", "!**.md", "!tools/**"], 1),
        ("Tests loesen aus", [m for m in gut if m != "!**_test.go"], 1),
        ("Asset fehlt", [m for m in gut if m != "x/a/assets/**"], 1),
        ("Kopie fehlt", [m for m in gut if m != "downloads/**"], 1),
        ("Skript fehlt", [m for m in gut if m != "deploy/rueckrollen-c1.sh"], 1),
    ]
    falsch = 0
    for name, muster, soll_fehler in faelle:
        f = pruefe(muster, embeds, kopien)
        if (len(f) > 0) != (soll_fehler > 0):
            print(f"SELBSTTEST FALSCH: {name}: {f}")
            falsch += 1
    if pruefe(gut, [("x/a/b.go", "fehlt.txt", None)], []) == []:
        print("SELBSTTEST FALSCH: go:embed ohne Datei nicht gemeldet")
        falsch += 1
    if falsch:
        sys.exit(1)
    print(f"Selbsttest: {len(faelle) + 1} Faelle richtig")


def main():
    if "--selbsttest" in sys.argv:
        selbsttest()
        return
    with open(WORKFLOW, encoding="utf-8") as f:
        muster = pfade_aus_workflow(f.read())
    with open("Dockerfile", encoding="utf-8") as f:
        kopien = dockerfile_uebernahmen(f.read())
    embeds = embed_dateien()
    fehler = pruefe(muster, embeds, kopien)
    for z in fehler:
        print(f"::error::{z}")
    if fehler:
        sys.exit(1)
    print(f"deploy-c1.yml: {len(embeds)} go:embed-Dateien, {len(kopien)} Uebernahmen aus dem Bau, "
          f"{len(MUSS)} Pflicht- und {len(NIE)} Ausschlusspfade passen")


if __name__ == "__main__":
    main()
