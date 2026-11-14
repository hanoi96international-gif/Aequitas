#!/usr/bin/env python3
"""Die Biometrie-Sicherung laedt nur age-Chiffrat hoch und schliesst ohne Empfaenger ab.

backup-biometrie-verschluesselt.yml holt biometrische Daten von C1. Das Repo
ist oeffentlich, jedes Artefakt fuer jeden angemeldeten GitHub-Nutzer lesbar.
Diese Pruefung haelt fest (Missbrauchstest nach AGENTS.md, Frage 5):

  1. Jeder Upload-Schritt laedt nur *.age hoch, hoechstens 30 Tage.
  2. Seit 24.10.2026 (Umgebung `sicherung`, deploy/sicherung/): der Workflow
     schickt KEINEN Empfaenger -- er liegt fest auf C1. Er prueft vor dem
     Sichern, dass C1 einen hat (bio_empfaenger=gesetzt), und ruft ueber SSH
     nur die festen Befehle des Sicherungsschluessels auf, nie `bash -s`.
     Die Eingabepruefung von sicherung-einrichten.yml, ueber die der
     Empfaenger auf C1 kommt, scheitert -- ausgefuehrt, nicht nur gelesen --
     bei einem privaten age-Schluessel und bei Unsinn; sie besteht nur mit
     einem age1-Schluessel oder leer (= unveraendert).
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
EINRICHTEN = pathlib.Path(".github/workflows/sicherung-einrichten.yml")
BEFEHLE = {"bio", "bio-datei", "bio-zusammenfassung", "pruefsummen"}
SKRIPT = pathlib.Path("deploy/backup-biometrie.sh")
# Ein beliebiger, gueltig geformter OEFFENTLICHER Schluessel (kein Geheimnis).
GUELTIG = "age1q5j4sy96gase6ulghewvzcya8upxrlqve6xm6dxn3v7umgcmgy7qqna70a"
# Quantensicher (age-keygen -pq, ML-KEM-768 + X25519; docs/QUANTENSICHERHEIT.md).
GUELTIG_PQ = "age1pq1t9tkdpt83l92gkfmtqcyzwffky6rwskfcsxt5c576dp6hqnk2weyzpzkdvc7ge6m75slc3m7xefhealxdn7erfrvnjngk3gtmx9yh9rzvuhy9fnqsmyc8z2eeqnrf0hq0p0fr3h4m2r0av40w693dvvt8rj5nw0r2rxgyg6wjg25kzd94tr3trhkazx54krnz6vkje39wr48sv35qwt7pqrd2ftzcfdf3uuhchedu5mprymccz48d3rnsu5rt3364f02se2v2qte4gtjqgj7ff5zjx7j87y9u7rhsx0mrd0whwgamsx6asg6k5gt9c2jqrhagazkz3y8u7g2ek4qyestnuvxpz6yex2qtxmrsg5nrunu9j2fj8uynd433zdpandcmagvvweern78zp7qcytf0faqjfxs9aq9gl80pzn87994mpn288kpkum0ps824dfl6gzextpug2zs8qp5eq2e7c8mmcr7z63qfh7s9p6hedmhked09fy6e0a8w93usjpr0nsxdxu45ptqzm6y00l6xk52q9wqgewmp0q2x858vge4nkcqj46edgn0gdp67vf2ywsnhsw0hn98yda2luzm3kycc34h3djwnq4jhvwzfxngqdurc6vye6px9r94pgcdkj93cq4f7nkg0nxe2t4kqzlxj0zfhe65jr784qh25hr9vdsynyu7406t8pvppjmcyywjaxxvu64daq4jdw9yy4h9dpe2wjzpsutsqugx44dhx79ssxtw2ypayr9krne4pucnh4pyw3hf5vslsceftx2x5frtxc2asm7pudh2qjsjdk220c33zvn28taukd85x6fxdqzzdtlrcwlkyptsyvmmlwzlu8r3dh6fwlf4sqqksyemtd48209qrx5jr6sh20787yeq4cjtzacznf2f9x4h28u92duyl7n2lxdg0xq6f7v4ymu0ndqy5xp7c9tzcy29t9ew4fe4hsy8uas2h4ur0wks4garzc3ygz5acun2np6ny73qtsy2qyyw6d7eu2284jv8s2pgrdcvg6ny2vv3ekkd7vj5u85s2uvu0dyvzs8eapfulfenpecvqddg89p8kzuvp3ys93n5ye3ggc0zq2vm7c590e3e4cpxjsjpxvt7cjvmnzagwvq3wrznwtdrcgz84jfrywvl6p47tzcqnd7nzkyj23c0rzq746pt6nj5jj5cttymc3vevprs0mrjmf2tp3shqtm7hpvysx2rzxyl85me0fcc4824gs6s7gxmj9k0ga78u2t5473l27q8wymjv5qdl0ju5tdcf9nd0j6g0fd08krxade2g6ae5p0ec3g5msspjcmnckyu87qknd5jz9smng8ml6vp06vgkyscs3dtvsj3a8z69qfzp6vg9q9x3ntdjx72ejhpqwqgvkvr3hgny6f0sm8st9as2gjkzza48grzcuzrqr0ckjgpxuzrvejkl3r5j545jnufs0pagccf8tzn5lg9sc34g2hp5fjhmc6sl6rfalk99axz93de257wlyurfrd3rdrnjk7wjg0lwx00edd8l3gslcpk0c265rgtv2zx0e9r7tsp9c0zzq5hye4zuk2320q5slpqkhvmqwhqyxyua9v2kwaplad2a2fkf477qj8dxaf8nc4k3q5e6de3wdjhg3ne8n8w3jj5guvkxzwkvs5hkrrzj9cguu22ummv2sg5wsj0z4dccpcegqv244t4xylhh3cejfrepf2zfwfp5m5h7mvc5hcfcg3ayl98a894syas3f72gqeytxl2jjv80jggq4gfjy28cntrck407klyrand5vuplxjyvlxtf27hlfu3pwd0s5yqh6ulyj0nlzrtx84eyye636lp7q4s842ugagmxqye9aaujjeke3mznhdm2wu3dj5d03csgl9axmfc793g0klj0qu4vpdy8gszp6ru"
UNGUELTIG = [
    "",
    "AGE-SECRET-KEY-1QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ",
    "age1kurz",
    GUELTIG + "x",
    GUELTIG.replace("q", "b", 1),  # b ist kein Bech32-Zeichen
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample",
    GUELTIG + "\nrm -rf /",
    GUELTIG_PQ[:-1],                    # ein Zeichen zu kurz
    GUELTIG_PQ + "q",                   # ein Zeichen zu lang
    GUELTIG_PQ[:-1] + "b",              # b ist kein Bech32-Zeichen
    "age1pq1" + GUELTIG[4:],            # PQ-Praefix mit klassischer Laenge
    GUELTIG_PQ + "\nrm -rf /",
    "AGE-SECRET-KEY-PQ-1" + "Q" * 60,   # privater PQ-Schluessel
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
    if job.get("environment") != "sicherung":
        f(f"Job {job_name}: environment sicherung fehlt")
    if not job.get("timeout-minutes"):
        f(f"Job {job_name}: timeout-minutes fehlt")
    steps = job.get("steps") or []
    if "BIO_BACKUP_AGE_RECIPIENT" in yaml.dump(job):
        f(f"Job {job_name}: schickt einen age-Empfaenger -- er liegt fest auf C1")
    # Jeder SSH-Aufruf: nur ein fester Befehl als letztes Wort vor einer
    # Umleitung, kein bash -s, kein scp.
    ssh_bio = None
    pruef = None
    for i, st in enumerate(steps):
        run = st.get("run", "")
        if re.search(r"\bscp\b|bash -s", run):
            f(f"Schritt '{st.get('name')}' nutzt scp oder bash -s -- nur feste Befehle")
        # ssh als Befehl, nicht ~/.ssh oder ssh-keyscan.
        for m in re.finditer(r"(?<![\w./~-])ssh\s([^\n|>)]*)", run):
            worte = m.group(1).split()
            if not worte or worte[-1] not in BEFEHLE:
                f(f"Schritt '{st.get('name')}': ssh mit {worte[-1:] or '?'} -- erlaubt: {sorted(BEFEHLE)}")
            elif worte[-1] == "bio" and ssh_bio is None:
                ssh_bio = i
        if "bio_empfaenger=gesetzt" in run and pruef is None:
            pruef = i
    if pruef is None:
        f("kein Schritt prueft, dass C1 einen age-Empfaenger hat (bio_empfaenger=gesetzt)")
    elif ssh_bio is None or pruef > ssh_bio:
        f("Empfaengerpruefung steht nicht vor dem Sichern")
    for s_ in steps:
        uses = s_.get("uses", "")
        if re.search(r"upload|release|cache/save", uses):
            uploads += 1
            w = s_.get("with") or {}
            pfade = [p.strip() for p in str(w.get("path", "")).splitlines() if p.strip()]
            if not pfade or any(not p.endswith(".age") or "*" in p for p in pfade):
                f(f"Upload-Schritt '{s_.get('name', uses)}' laedt nicht nur *.age hoch: {pfade}")
            if int(w.get("retention-days", 999)) > 30:
                f("retention-days > 30")
            if "if" in s_ and "always()" in str(s_["if"]):
                f("Upload darf nicht mit always() laufen")
        if uses and not re.search(r"@[0-9a-f]{40}\b", uses):
            f(f"Action nicht per SHA gepinnt: {uses}")
        if re.search(r"set -x|echo \"?\$\{?(R|BIO_BACKUP_AGE_RECIPIENT|KEY)\b", s_.get("run", "")):
            f(f"Schritt '{s_.get('name')}' gibt ein Secret oder Befehle aus")

# Die Eingabepruefung von sicherung-einrichten.yml ausfuehren.
ew = yaml.safe_load(EINRICHTEN.read_text())
esteps = [st for job in ew["jobs"].values() for st in (job.get("steps") or [])]
epruef = next((st for st in esteps if "inputs.bio_empfaenger" in str(st.get("env", {}))
               and "R" in (st.get("env") or {}) and not re.search(r"\bssh\b", st.get("run", ""))), None)
erster_ssh = next((i for i, st in enumerate(esteps) if re.search(r"\b(ssh|scp)\b", st.get("run", ""))), None)
if epruef is None:
    f("sicherung-einrichten.yml: kein Schritt prueft bio_empfaenger")
else:
    if erster_ssh is not None and esteps.index(epruef) > erster_ssh:
        f("sicherung-einrichten.yml: Empfaengerpruefung steht hinter dem ersten SSH-Schritt")
    run = epruef["run"]
    for wert in [w for w in UNGUELTIG if w]:
        p = subprocess.run(["bash", "-c", run], env={"PATH": os.environ["PATH"], "R": wert},
                           capture_output=True, text=True)
        if p.returncode == 0 or "::error" not in p.stdout:
            f(f"sicherung-einrichten.yml: Empfaengerpruefung laesst {wert[:20]!r} durch")
        if wert in p.stdout + p.stderr:
            f("sicherung-einrichten.yml: Empfaengerpruefung gibt den Wert aus")
    for wert in ("", GUELTIG, GUELTIG_PQ):
        p = subprocess.run(["bash", "-c", run], env={"PATH": os.environ["PATH"], "R": wert},
                           capture_output=True, text=True)
        if p.returncode != 0:
            f(f"sicherung-einrichten.yml: Empfaengerpruefung lehnt {wert[:12]!r} ab: {p.stdout}")
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

# Pruefung von #382 (F1-F3): festgenagelter Host-Schluessel nur als ed25519,
# Image per Digest, keine Fehlertexte der Datenbank ins oeffentliche Log.
wf_text = WF.read_text()
if "ssh-keyscan -t ed25519" not in wf_text or 'grep -c . ~/.ssh/kh_all)" = 1' not in wf_text:
    f("Host-Schluessel: nur ed25519 und genau eine Zeile, sonst reicht ein fremder ecdsa/rsa-Schluessel")
skript = SKRIPT.read_text()
m = re.search(r"^PG_IMAGE=(\S+)$", skript, re.M)
if not m or not re.search(r"@sha256:[0-9a-f]{64}$", m.group(1)):
    f("PG_IMAGE muss per Digest (@sha256:...) festgenagelt sein")
logisch = skript.replace("\\\n", " ")
for zeile in logisch.splitlines():
    z = zeile.strip()
    if z.startswith("#"):
        continue
    # Aufrufe, nicht Meldungen: pg_dump/pg_restore als Befehl (nach exec,
    # docker exec ... oder am Zeilenanfang), nicht in einem echo-Text.
    ohne_echo = re.sub(r'echo "[^"]*"', "", z)
    if re.search(r"(^|exec |\s)(pg_dump|pg_restore)\s", ohne_echo) and "2>>\"$FEHLERLOG\"" not in z:
        f(f"pg_dump/pg_restore ohne 2>>\"$FEHLERLOG\": {z[:70]!r}")
for m3 in re.finditer(r"^(kette|pq)\(\) \{.*$", skript, re.M):
    if "2>>\"$FEHLERLOG\"" not in m3.group(0) or "VERBOSITY=terse" not in m3.group(0):
        f(f"{m3.group(1)}(): psql-Fehler muessen in $FEHLERLOG, nicht ins Log")

for x in fehler:
    print(f"::error::{x}")
print(f"{len(fehler)} Verstoesse")
sys.exit(1 if fehler else 0)
