#!/usr/bin/env python3
"""Die Server-Haertung prueft per Voreinstellung nur und kann C1 nicht aussperren.

server-haerten.yml fasst sshd, das root-Passwort und /etc/docker/daemon.json
auf C1 an -- mit Root-Rechten auf dem Validator. Diese Pruefung haelt fest
(Missbrauchstest nach AGENTS.md, Frage 5):

  1. Workflow: nur workflow_dispatch, Eingabe `modus` mit Voreinstellung
     `pruefen`; environment produktion, permissions {contents: read},
     timeout-minutes am Job und an jedem SSH-Schritt, Actions per SHA,
     Host-Schluessel nur ed25519 und genau eine Zeile.
  2. root-sperren laeuft nur bei `anwenden` und erst NACH einem neuen
     Schluessel-Login, der nur noch "publickey" angeboten bekommt.
  3. Weder Workflow noch Skript starten Docker oder Container neu, noch sshd
     (nur reload).
  4. Skript, ausgefuehrt gegen einen Attrappen-Baum:
     - ohne MODUS: Modus pruefen, nichts geschrieben, kein reload, kein passwd -l;
     - unbekannter MODUS: Abbruch, bevor irgendein Werkzeug laeuft;
     - anwenden ohne/mit leerer authorized_keys: kein Drop-in, kein reload;
     - anwenden mit scheiterndem `sshd -t`: Drop-in zurueckgenommen, kein reload;
     - anwenden im Gutfall: `sshd -t` vor dem reload, daemon.json
       zusammengefuehrt (fremde Schluessel bleiben), authorized_keys
       unveraendert und nie ausgegeben; zweiter Lauf laedt nicht erneut;
     - kaputte daemon.json wird nicht ueberschrieben;
     - root-sperren ohne UsePAM oder ohne SSH-Sitzung sperrt nicht.
"""
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile

import yaml

WF = pathlib.Path(".github/workflows/server-haerten.yml")
SKRIPT = pathlib.Path("deploy/server-haerten.sh").resolve()
GEHEIMER_SCHLUESSEL = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGeheimNichtAusgebenXYZ betreiber@laptop"

fehler = []


def f(msg):
    fehler.append(msg)


# ------------------------------------------------------------- 1. Workflow
wf_text = WF.read_text()
wf = yaml.safe_load(wf_text)
on = wf.get(True, wf.get("on"))
if not isinstance(on, dict) or set(on) != {"workflow_dispatch"}:
    f("server-haerten.yml darf nur per workflow_dispatch laufen")
else:
    modus = ((on["workflow_dispatch"] or {}).get("inputs") or {}).get("modus") or {}
    if modus.get("default") != "pruefen":
        f("Eingabe modus: Voreinstellung muss pruefen sein")
    if modus.get("type") != "choice" or modus.get("options") != ["pruefen", "anwenden"]:
        f("Eingabe modus: choice mit genau [pruefen, anwenden]")
if wf.get("permissions") != {"contents": "read"}:
    f("permissions muss genau {contents: read} sein")
if "'pruefen'" not in str((wf["jobs"].get("haerten") or {}).get("env", {}).get("MODUS", "")):
    f("MODUS im Job muss ohne Eingabe auf 'pruefen' fallen")
if "WURZEL" in wf_text:
    f("Workflow darf WURZEL nie setzen (nur fuer diese Pruefung)")
if "ssh-keyscan -t ed25519" not in wf_text or 'grep -c . ~/.ssh/kh_all)" = 1' not in wf_text:
    f("Host-Schluessel: nur ed25519 und genau eine Zeile, sonst reicht ein fremder ecdsa/rsa-Schluessel")
if "SHA256:MAkOsPuhsFZUZViP8NPq+WxW1k6ykAF4mOy81wYNURA" not in wf_text:
    f("Fingerabdruck von C1 fehlt")

for job_name, job in wf["jobs"].items():
    if job.get("environment") != "produktion":
        f(f"Job {job_name}: environment produktion fehlt")
    if not job.get("timeout-minutes"):
        f(f"Job {job_name}: timeout-minutes fehlt")
    steps = job.get("steps") or []
    login = sperren = None
    for i, s in enumerate(steps):
        run = s.get("run", "")
        uses = s.get("uses", "")
        if uses and not re.search(r"@[0-9a-f]{40}\b", uses):
            f(f"Action nicht per SHA gepinnt: {uses}")
        if re.search(r"\bssh -o", run) and not s.get("timeout-minutes"):
            f(f"SSH-Schritt '{s.get('name')}' ohne timeout-minutes")
        if re.search(r"set -x|echo \"?\$\{?KEY\b", run):
            f(f"Schritt '{s.get('name')}' gibt den Schluessel oder Befehle aus")
        if "MODUS=root-sperren" in run:
            sperren = i
            if s.get("if") != "env.MODUS == 'anwenden'":
                f("root-sperren laeuft nicht nur bei modus=anwenden")
        if '"Permission denied (publickey)"' in run and re.search(r"\bexit 1\b", run):
            login = i
            if not re.search(r"ssh -o BatchMode=yes[^\n|]*root@\"\$HOST\" true\n", run):
                f("Login-Schritt baut keine neue Schluessel-Verbindung auf")
    if sperren is None:
        f("kein Schritt sperrt das root-Passwort")
    elif login is None or login > sperren:
        f("root-sperren steht nicht hinter dem neuen Schluessel-Login mit 'publickey'-Pruefung")

# ------------------------------------------------- 3. kein Neustart, nirgends
text = SKRIPT.read_text()
code = "\n".join(z for z in text.splitlines() if not z.lstrip().startswith("#"))
wf_code = "\n".join(z for z in wf_text.splitlines() if not z.lstrip().startswith("#"))
for quelle, c in (("server-haerten.sh", code), ("server-haerten.yml", wf_code)):
    if re.search(r"systemctl\s+(restart|stop|kill)\b", c) or re.search(r"service\s+\S+\s+restart", c):
        f(f"{quelle}: systemctl restart/stop -- nur reload ist erlaubt")
    if re.search(r"docker\s+(restart|stop|kill|rm|compose\s+(up|down|restart))\b", c):
        f(f"{quelle}: startet Container neu oder stoppt sie")
    if re.search(r"(pkill|killall|kill -HUP)\s", c):
        f(f"{quelle}: Signale an Prozesse")
if re.search(r"^\s*set -[a-z]*x", text, re.M):
    f("server-haerten.sh nutzt set -x")
if not re.search(r'^MODUS="\$\{MODUS:-pruefen\}"$', text, re.M):
    f("server-haerten.sh: MODUS muss ohne Angabe pruefen sein")
if re.search(r"(>|tee |cp [^\n]*|install [^\n]*|mv [^\n]*)\s*\"?\$AUTHK", code):
    f("server-haerten.sh schreibt authorized_keys")
if re.search(r"(cat|head|tail)\s+\"?\$AUTHK", code):
    f("server-haerten.sh gibt authorized_keys aus")

# ---------------------------------------------- 4. ausfuehren mit Attrappen
SSHD = r'''#!/bin/sh
echo "sshd $*" >> "$MARKE"
case "$1" in
  -t) exit "${STUB_SSHD_T:-0}" ;;
  -T)
    if [ -n "${STUB_NUR_PASSWORT_AN:-}" ]; then
      printf 'passwordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin without-password\npubkeyauthentication yes\n'
    elif [ -f "$WURZEL/etc/ssh/sshd_config.d/00-aequitas-haerten.conf" ]; then
      printf 'passwordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin without-password\npubkeyauthentication yes\n'
    else
      printf 'passwordauthentication yes\nkbdinteractiveauthentication yes\npermitrootlogin yes\npubkeyauthentication yes\n'
    fi
    echo "usepam ${STUB_USEPAM:-yes}"
    exit 0 ;;
esac
exit 0
'''
PASSWD = r'''#!/bin/sh
echo "passwd $*" >> "$MARKE"
case "$1" in
  -S) if [ -f "$WURZEL/gesperrt" ]; then echo "root L 2026-10-17 0 99999 7 -1"; else echo "root P 2026-10-17 0 99999 7 -1"; fi ;;
  -l) : > "$WURZEL/gesperrt" ;;
esac
'''
DOCKER = r'''#!/bin/sh
echo "docker $*" >> "$MARKE"
case "$1" in
  ps) printf 'aaa\nbbb\n' ;;
  inspect) printf '/aequitas-node json-file \n/caddy json-file 10m\n' ;;
esac
'''
SS = r'''#!/bin/sh
echo "ss $*" >> "$MARKE"
printf 'LISTEN 0 4096 0.0.0.0:8080 0.0.0.0:* users:(("docker-proxy",pid=1,fd=4))\nLISTEN 0 4096 127.0.0.1:5432 0.0.0.0:* users:(("postgres",pid=2,fd=5))\n'
'''
LOGGER = '#!/bin/sh\necho "{n} $*" >> "$MARKE"\n{extra}\n'


def baum(tmp, authk=GEHEIMER_SCHLUESSEL + "\n", daemon=None):
    w = pathlib.Path(tmp, "wurzel")
    (w / "etc/ssh/sshd_config.d").mkdir(parents=True)
    (w / "etc/ssh/sshd_config").write_text("Include /etc/ssh/sshd_config.d/*.conf\nUsePAM yes\n")
    (w / "etc/docker").mkdir(parents=True)
    if daemon is not None:
        (w / "etc/docker/daemon.json").write_text(daemon)
    (w / "root/.ssh").mkdir(parents=True)
    if authk is not None:
        (w / "root/.ssh/authorized_keys").write_text(authk)
    b = pathlib.Path(tmp, "bin")
    b.mkdir()
    stubs = {"sshd": SSHD, "passwd": PASSWD, "docker": DOCKER, "ss": SS,
             "systemctl": LOGGER.format(n="systemctl", extra='[ "$1" = is-active ] && exit 0; exit 0'),
             "ufw": LOGGER.format(n="ufw", extra='echo "Status: active"'),
             "nft": LOGGER.format(n="nft", extra="exit 0")}
    for name, inhalt in stubs.items():
        p = b / name
        p.write_text(inhalt)
        p.chmod(0o755)
    return w, b


def lauf(tmp, w, b, modus=None, **extra):
    marke = pathlib.Path(tmp, "marke")
    env = {"PATH": f"{b}:{os.environ['PATH']}", "WURZEL": str(w), "MARKE": str(marke)}
    if modus is not None:
        env["MODUS"] = modus
    env.update(extra)
    p = subprocess.run(["bash", str(SKRIPT)], env=env, capture_output=True, text=True, timeout=60)
    aufrufe = marke.read_text().splitlines() if marke.exists() else []
    if marke.exists():
        marke.unlink()
    return p, aufrufe


def reloads(aufrufe):
    return [a for a in aufrufe if a.startswith("systemctl reload")]


DROPIN = "etc/ssh/sshd_config.d/00-aequitas-haerten.conf"

# a) ohne MODUS: nur pruefen
with tempfile.TemporaryDirectory() as tmp:
    w, b = baum(tmp, daemon='{"data-root": "/var/lib/docker"}\n')
    p, a = lauf(tmp, w, b)
    if p.returncode != 0 or "Modus: pruefen" not in p.stdout:
        f(f"ohne MODUS laeuft nicht als pruefen: rc={p.returncode} {p.stdout[-300:]}{p.stderr[-300:]}")
    if (w / DROPIN).exists() or reloads(a) or any(x.startswith("passwd -l") for x in a):
        f(f"pruefen veraendert etwas: {a}")
    if json.loads((w / "etc/docker/daemon.json").read_text()) != {"data-root": "/var/lib/docker"}:
        f("pruefen schreibt daemon.json")
    for erwartet in ("OHNE max-size: aequitas-node", "Port 8080", "BETREIBER", "== Urteil =="):
        if erwartet not in p.stdout:
            f(f"Bericht ohne '{erwartet}'")
    if "0.0.0.0:8080" not in p.stdout or "127.0.0.1:5432" in p.stdout:
        f("Portbericht: oeffentliche Ports zeigen, Loopback weglassen")
    if GEHEIMER_SCHLUESSEL.split()[1] in p.stdout + p.stderr:
        f("pruefen gibt authorized_keys aus")

# b) unbekannter MODUS
with tempfile.TemporaryDirectory() as tmp:
    w, b = baum(tmp)
    for m in ("anwenden;rm -rf /", "ANWENDEN", "restart", "x"):
        p, a = lauf(tmp, w, b, modus=m)
        if p.returncode == 0 or a:
            f(f"unbekannter MODUS {m!r} laeuft weiter oder ruft Werkzeuge auf: {a}")

# c) anwenden ohne bzw. mit leerer authorized_keys
for authk in (None, "", "\n# kein Schluessel\n"):
    with tempfile.TemporaryDirectory() as tmp:
        w, b = baum(tmp, authk=authk)
        p, a = lauf(tmp, w, b, modus="anwenden")
        if p.returncode == 0 or "::error::" not in p.stdout:
            f(f"anwenden mit authorized_keys={authk!r} endet nicht rot")
        if (w / DROPIN).exists() or reloads(a):
            f(f"anwenden ohne Schluessel schreibt das Drop-in oder laedt sshd: {a}")

# d) anwenden, sshd -t scheitert
with tempfile.TemporaryDirectory() as tmp:
    w, b = baum(tmp)
    p, a = lauf(tmp, w, b, modus="anwenden", STUB_SSHD_T="1")
    if p.returncode == 0:
        f("anwenden mit scheiterndem sshd -t endet nicht rot")
    if (w / DROPIN).exists() or reloads(a):
        f(f"sshd -t scheitert, trotzdem Drop-in behalten oder reload: {a}")

# e) anwenden im Gutfall, dann noch einmal
with tempfile.TemporaryDirectory() as tmp:
    w, b = baum(tmp, daemon='{"data-root": "/srv/docker", "log-opts": {"labels": "x"}}\n')
    vorher = (w / "root/.ssh/authorized_keys").read_text()
    p, a = lauf(tmp, w, b, modus="anwenden")
    if p.returncode != 0:
        f(f"anwenden im Gutfall rot: {p.stdout[-400:]}{p.stderr[-400:]}")
    t = [i for i, x in enumerate(a) if x == "sshd -t"]
    r = [i for i, x in enumerate(a) if x.startswith("systemctl reload")]
    if not t or not r or t[0] > r[0]:
        f(f"sshd -t muss vor dem reload laufen: {a}")
    if any(re.match(r"systemctl (restart|stop)|docker (restart|stop|kill|rm)", x) for x in a):
        f(f"anwenden startet etwas neu: {a}")
    if any(x.startswith("passwd -l") for x in a):
        f("anwenden sperrt root selbst -- das ist Sache von root-sperren nach dem Login-Test")
    dropin = (w / DROPIN).read_text() if (w / DROPIN).exists() else ""
    for zeile in ("PasswordAuthentication no", "KbdInteractiveAuthentication no",
                  "PermitRootLogin prohibit-password", "PubkeyAuthentication yes"):
        if zeile not in dropin.splitlines():
            f(f"Drop-in ohne '{zeile}'")
    d = json.loads((w / "etc/docker/daemon.json").read_text())
    if d != {"data-root": "/srv/docker", "log-driver": "json-file",
             "log-opts": {"labels": "x", "max-size": "50m", "max-file": "5"}}:
        f(f"daemon.json falsch zusammengefuehrt: {d}")
    if (w / "root/.ssh/authorized_keys").read_text() != vorher:
        f("anwenden veraendert authorized_keys")
    if GEHEIMER_SCHLUESSEL.split()[1] in p.stdout + p.stderr:
        f("anwenden gibt authorized_keys aus")
    p2, a2 = lauf(tmp, w, b, modus="anwenden")
    if p2.returncode != 0 or reloads(a2) or "unveraendert" not in p2.stdout:
        f(f"zweiter Lauf nicht idempotent: {a2}")
    if len(list((w / "etc/docker").glob("daemon.json.vor-haerten-*"))) != 1:
        f("daemon.json: genau eine Sicherung der alten Fassung erwartet")
    # root-sperren: ohne SSH-Sitzung nein, ohne UsePAM nein, sonst ja.
    p, a = lauf(tmp, w, b, modus="root-sperren")
    if p.returncode == 0 or any(x.startswith("passwd -l") for x in a):
        f("root-sperren ohne SSH-Sitzung sperrt")
    p, a = lauf(tmp, w, b, modus="root-sperren", SSH_CONNECTION="1.2.3.4 1 5.6.7.8 22", STUB_USEPAM="no")
    if p.returncode == 0 or any(x.startswith("passwd -l") for x in a):
        f("root-sperren ohne UsePAM sperrt (wuerde den Schluessel-Login mitsperren)")
    p, a = lauf(tmp, w, b, modus="root-sperren", SSH_CONNECTION="1.2.3.4 1 5.6.7.8 22")
    if p.returncode != 0 or "passwd -l root" not in a:
        f(f"root-sperren im Gutfall sperrt nicht: {p.stdout[-300:]}")

# f) root-sperren vor der SSH-Haertung: nein
with tempfile.TemporaryDirectory() as tmp:
    w, b = baum(tmp)
    for extra in ({}, {"STUB_NUR_PASSWORT_AN": "1"}):
        p, a = lauf(tmp, w, b, modus="root-sperren", SSH_CONNECTION="1.2.3.4 1 5.6.7.8 22", **extra)
        if p.returncode == 0 or any(x.startswith("passwd -l") for x in a):
            f(f"root-sperren bei noch erlaubtem Passwort- oder root-Login sperrt ({extra})")

# g) kaputte daemon.json bleibt, wie sie ist
with tempfile.TemporaryDirectory() as tmp:
    kaputt = '{"data-root": "/srv/docker",,}\n'
    w, b = baum(tmp, daemon=kaputt)
    p, a = lauf(tmp, w, b, modus="anwenden")
    if p.returncode == 0 or (w / "etc/docker/daemon.json").read_text() != kaputt:
        f("kaputte daemon.json wird ueberschrieben oder der Lauf bleibt gruen")

# Pruefung von #388: kein "neu geladen" ohne aktiven Dienst/Socket, und
# keine daemon.json-Aenderung, wenn die docker-Einheit Log-Flags setzt.
_sk = SKRIPT.read_text() if "SKRIPT" in globals() else open("deploy/server-haerten.sh").read()
if "systemctl is-active --quiet ssh.socket" not in _sk or "kein reload moeglich" not in _sk:
    f("sshd: ohne aktiven ssh/sshd-Dienst oder ssh.socket muss der Lauf rot enden, nicht 'neu geladen' melden")
_d = _sk[_sk.find("docker_anwenden() {"):]
_w = _d.find("python3")
_g = _d.find("--log-(driver|opt)")
if _g < 0 or (_w >= 0 and _g > _w):
    f("docker: Log-Flags der Einheit (systemctl cat docker) muessen VOR dem Schreiben von daemon.json geprueft werden")

for x in fehler:
    print(f"::error::{x}")
print(f"{len(fehler)} Verstoesse")
sys.exit(1 if fehler else 0)
