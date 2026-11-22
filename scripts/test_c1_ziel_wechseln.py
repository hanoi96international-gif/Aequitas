#!/usr/bin/env python3
"""Probe des Umzugs: c1_ziel_wechseln.py auf einer Kopie des Repos.

Prueft, dass nach dem Tausch (1) die alte IP in keiner ausfuehrbaren Zeile
mehr steht, (2) der alte Fingerabdruck nirgends mehr steht, (3)
lint_c1_ziel.py und lint_alte_hosts.py gruen sind, (4) der Explorer-Schluessel
mitgezogen ist, (5) docs/ unveraendert bleibt -- und dass ungueltige
Eingaben (private IP, alte IP, kaputter Fingerabdruck) abgewiesen werden,
ohne etwas zu aendern.
"""
import os
import shutil
import subprocess
import sys
import tempfile

WURZEL = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ALT_IP = "188.172.229.121"
ALT_FP = "SHA256:MAkOsPuhsFZUZViP8NPq+WxW1k6ykAF4mOy81wYNURA"
NEU_IP = "45.33.32.156"
NEU_FP = "SHA256:" + "Q" * 42 + "w"


def lauf(d, *args):
    return subprocess.run([sys.executable, "scripts/c1_ziel_wechseln.py", *args], cwd=d, capture_output=True, text=True)


def main():
    fehler = []
    with tempfile.TemporaryDirectory() as tmp:
        d = os.path.join(tmp, "repo")
        # Nur verfolgte Dateien kopieren (wie git ls-files sie sieht).
        subprocess.run(["git", "clone", "-q", "--no-hardlinks", WURZEL, d], check=True)
        for p in ("scripts/c1_ziel_wechseln.py", "scripts/lint_c1_ziel.py"):
            shutil.copy(os.path.join(WURZEL, p), os.path.join(d, p))
        subprocess.run(["git", "add", "-A"], cwd=d, check=True)
        docs_vorher = subprocess.run(["git", "grep", "-c", ALT_IP, "--", "docs/"], cwd=d, capture_output=True, text=True).stdout

        for schlecht in (["--neu-ip", "10.0.0.5", "--neu-fp", NEU_FP],
                         ["--neu-ip", ALT_IP, "--neu-fp", NEU_FP],
                         ["--neu-ip", NEU_IP, "--neu-fp", "SHA256:kurz"],
                         ["--neu-ip", NEU_IP, "--neu-fp", ALT_FP]):
            r = lauf(d, *schlecht, "--schreiben")
            if r.returncode == 0:
                fehler.append(f"angenommen: {schlecht}")
        if subprocess.run(["git", "diff", "--quiet"], cwd=d).returncode != 0:
            fehler.append("abgewiesene Eingabe hat Dateien geaendert")

        r = lauf(d, "--neu-ip", NEU_IP, "--neu-fp", NEU_FP)
        if r.returncode != 0 or subprocess.run(["git", "diff", "--quiet"], cwd=d).returncode != 0:
            fehler.append("Probelauf ohne --schreiben hat geaendert oder ist gescheitert")

        r = lauf(d, "--neu-ip", NEU_IP, "--neu-fp", NEU_FP, "--schreiben")
        if r.returncode != 0:
            fehler.append("Tausch gescheitert:\n" + r.stdout + r.stderr)
        for lint in ("scripts/lint_c1_ziel.py", "scripts/lint_alte_hosts.py"):
            r2 = subprocess.run([sys.executable, lint], cwd=d, capture_output=True, text=True)
            if r2.returncode != 0:
                fehler.append(f"{lint} nach dem Tausch rot:\n{r2.stdout}")
        rest_fp = subprocess.run(["git", "grep", "-l", ALT_FP, "--", ".", ":!docs/", ":!scripts/c1_ziel_wechseln.py", ":!scripts/test_c1_ziel_wechseln.py"], cwd=d, capture_output=True, text=True).stdout
        if rest_fp.strip():
            fehler.append("alter Fingerabdruck steht noch in:\n" + rest_fp)
        with open(os.path.join(d, "x/humanity/keeper/assets/explorer.js"), encoding="utf-8") as f:
            ex = f.read()
        if "x-ip4-45-33-32-156" not in ex or "x-ip4-188-172-229-121" in ex:
            fehler.append("Explorer-Schluessel nicht mitgezogen")
        with open(os.path.join(d, "x/humanity/keeper/p2p.go"), encoding="utf-8") as f:
            if f"/ip4/{NEU_IP}/tcp/4001" not in f.read():
                fehler.append("Bootstrap-Adresse nicht getauscht")
        docs_nachher = subprocess.run(["git", "grep", "-c", ALT_IP, "--", "docs/"], cwd=d, capture_output=True, text=True).stdout
        if docs_vorher != docs_nachher:
            fehler.append("docs/ wurde veraendert")
        # Zweiter Lauf: alte IP ist schon weg, nichts mehr zu tun, Waechter gruen.
        r = lauf(d, "--neu-ip", "45.33.32.157", "--neu-fp", "SHA256:" + "R" * 43)
        if "0 Stellen" not in r.stdout:
            fehler.append("zweiter Lauf fand noch alte Stellen:\n" + r.stdout)

    if fehler:
        print("FEHLER:\n" + "\n".join(fehler))
        return 1
    print("ok: Umzug von IP und Host-Schluessel auf einer Kopie geprobt")
    return 0


if __name__ == "__main__":
    sys.exit(main())
