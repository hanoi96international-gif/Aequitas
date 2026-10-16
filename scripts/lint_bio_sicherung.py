#!/usr/bin/env python3
"""Die Biometrie-Sicherung laedt nur age-Chiffrat hoch und schliesst ohne Empfaenger ab.

backup-biometrie-verschluesselt.yml holt biometrische Daten von C1. Das Repo
ist oeffentlich, jedes Artefakt fuer jeden angemeldeten GitHub-Nutzer lesbar.
Diese Pruefung haelt fest (Missbrauchstest nach AGENTS.md, Frage 5):

  1. Jeder Upload-Schritt laedt nur *.age hoch, hoechstens 30 Tage.
  2. Die Empfaengerpruefung steht vor jedem SSH-Schritt und scheitert --
     ausgefuehrt, nicht nur gelesen -- bei leerem Secret, bei einem privaten
     age-Schluessel, bei Unsinn; sie besteht nur mit einem age1-Schluessel.
  3. deploy/backup-biometrie.sh bricht ohne bzw. mit ungueltigem Empfaenger
     ab, BEVOR es docker oder apt-get aufruft (Attrappen beweisen es).
  4. Das Skript loescht den Klartext per trap und gibt nie Befehle aus (set -x).
"""
import os
import pathlib
import re
import subprocess
import sys
import tempfile

import yaml

WF = pathlib.Path(".github/workflows/backup-biometrie-verschluesselt.yml")
SKRIPT = pathlib.Path("deploy/backup-biometrie.sh")
# Ein beliebiger, gueltig geformter OEFFENTLICHER Schluessel (kein Geheimnis).
GUELTIG = "age1q5j4sy96gase6ulghewvzcya8upxrlqve6xm6dxn3v7umgcmgy7qqna70a"
UNGUELTIG = [
    "",
    "AGE-SECRET-KEY-1QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ",
    "age1kurz",
    GUELTIG + "x",
    GUELTIG.replace("q", "b", 1),  # b ist kein Bech32-Zeichen
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample",
    GUELTIG + "\nrm -rf /",
]

fehler = []


def f(msg):
    fehler.append(msg)


wf = yaml.safe_load(WF.read_text())
on = wf.get(True, wf.get("on"))
if not (isinstance(on, dict) and "schedule" in on and "workflow_dispatch" in on):
    f("Workflow muss per schedule und workflow_dispatch laufen")
if wf.get("permissions") != {"contents": "read"}:
    f("permissions muss genau {contents: read} sein")

uploads = 0
for job_name, job in wf["jobs"].items():
    if job.get("environment") != "produktion":
        f(f"Job {job_name}: environment produktion fehlt")
    if not job.get("timeout-minutes"):
        f(f"Job {job_name}: timeout-minutes fehlt")
    steps = job.get("steps") or []
    erster_ssh = next((i for i, s in enumerate(steps) if re.search(r"\b(ssh|scp)\b", s.get("run", ""))), None)
    pruef = next((i for i, s in enumerate(steps) if "BIO_BACKUP_AGE_RECIPIENT" in str(s.get("env", {})) and "R" in (s.get("env") or {})), None)
    if pruef is None:
        f("kein Schritt prueft BIO_BACKUP_AGE_RECIPIENT")
    elif erster_ssh is not None and pruef > erster_ssh:
        f("Empfaengerpruefung steht hinter dem ersten SSH-Schritt")
    for s in steps:
        uses = s.get("uses", "")
        if re.search(r"upload|release|cache/save", uses):
            uploads += 1
            w = s.get("with") or {}
            pfade = [p.strip() for p in str(w.get("path", "")).splitlines() if p.strip()]
            if not pfade or any(not p.endswith(".age") or "*" in p for p in pfade):
                f(f"Upload-Schritt '{s.get('name', uses)}' laedt nicht nur *.age hoch: {pfade}")
            if int(w.get("retention-days", 999)) > 30:
                f("retention-days > 30")
            if "if" in s and "always()" in str(s["if"]):
                f("Upload darf nicht mit always() laufen")
        if uses and not re.search(r"@[0-9a-f]{40}\b", uses):
            f(f"Action nicht per SHA gepinnt: {uses}")
        if re.search(r"set -x|echo \"?\$\{?(R|BIO_BACKUP_AGE_RECIPIENT|KEY)\b", s.get("run", "")):
            f(f"Schritt '{s.get('name')}' gibt ein Secret oder Befehle aus")
    # Die Pruefung ausfuehren.
    if pruef is not None:
        run = steps[pruef]["run"]
        for wert in UNGUELTIG:
            p = subprocess.run(["bash", "-c", run], env={"PATH": os.environ["PATH"], "R": wert},
                               capture_output=True, text=True)
            if p.returncode == 0 or "::error" not in p.stdout:
                f(f"Empfaengerpruefung laesst {wert[:20]!r} durch")
            if wert and wert in p.stdout + p.stderr:
                f("Empfaengerpruefung gibt den Wert aus")
        p = subprocess.run(["bash", "-c", run], env={"PATH": os.environ["PATH"], "R": GUELTIG},
                           capture_output=True, text=True)
        if p.returncode != 0:
            f(f"Empfaengerpruefung lehnt gueltigen Schluessel ab: {p.stdout}")
if uploads != 1:
    f(f"genau ein Upload-Schritt erwartet, gefunden: {uploads}")

# Das Skript auf der Box: fail-closed vor jeder Wirkung.
text = SKRIPT.read_text()
if re.search(r"^\s*set -[a-z]*x", text, re.M):
    f("backup-biometrie.sh nutzt set -x")
if not re.search(r"^trap aufraeumen EXIT", text, re.M) or 'rm -rf "$WORK"' not in text:
    f("backup-biometrie.sh loescht den Klartext nicht per trap")
code = "\n".join(z for z in text.splitlines() if not z.lstrip().startswith("#"))
if re.search(r"curl[^\n]*\|\s*(ba)?sh", code):
    f("backup-biometrie.sh installiert per curl|sh")
with tempfile.TemporaryDirectory() as tmp:
    marke = pathlib.Path(tmp, "aufgerufen")
    for werkzeug in ("docker", "apt-get", "age", "tar", "mkdir"):
        attrappe = pathlib.Path(tmp, werkzeug)
        attrappe.write_text(f"#!/bin/sh\necho {werkzeug} >> '{marke}'\nexit 0\n")
        attrappe.chmod(0o755)
    for wert in UNGUELTIG:
        env = {"PATH": f"{tmp}:{os.environ['PATH']}"}
        if wert:
            env["BIO_BACKUP_AGE_RECIPIENT"] = wert
        p = subprocess.run(["bash", str(SKRIPT)], env=env, capture_output=True, text=True)
        if p.returncode == 0 or "::error" not in p.stdout:
            f(f"backup-biometrie.sh laeuft mit Empfaenger {wert[:20]!r} weiter")
        if marke.exists():
            f(f"backup-biometrie.sh ruft vor der Empfaengerpruefung auf: {marke.read_text().split()}")
            marke.unlink()

for x in fehler:
    print(f"::error::{x}")
print(f"{len(fehler)} Verstoesse")
sys.exit(1 if fehler else 0)
