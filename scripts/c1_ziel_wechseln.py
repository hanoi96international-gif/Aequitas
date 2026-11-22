#!/usr/bin/env python3
"""C1 zieht um: IP und Host-Schluessel in EINEM Schritt im ganzen Repo tauschen.

Umzug auf einen neuen Server (geplant 23.12.2026). Die IP von C1 steht in
rund 70 Dateien (Workflows, Seeds, Bootstrap-Adresse, Explorer), der
gepinnte Host-Schluessel (SHA256-Fingerabdruck) in rund 55 Workflows. Von
Hand vergisst man eine Stelle -- und genau die meldet sich dann bei einem
Fremden an oder scheitert mitten im Umzug.

    python3 scripts/c1_ziel_wechseln.py \\
        --neu-ip 203.0.113.7 --neu-fp SHA256:<43 Zeichen aus der Anbieter-Konsole>

Ohne --schreiben zeigt das Skript nur, was es aendern wuerde (Dateien und
Anzahl). Mit --schreiben ersetzt es, traegt die alte IP in
scripts/lint_alte_hosts.py als abgeschaltet ein (ausfuehrbare Zeilen duerfen
sie danach nicht mehr nennen) und ruft danach scripts/lint_c1_ziel.py auf.

Den NEUEN Fingerabdruck nie per ssh-keyscan vom Runner nehmen (das waere
"dem Erstbesten glauben"), sondern aus der Konsole des Anbieters bzw. aus
dem Lauf von neuer-c1-pruefen.yml, der ihn gegen die Eingabe vergleicht.

Ausgenommen: docs/ (Geschichte darf die alte IP nennen) und die
SSRF-Testliste (dort steht die IP nur als Beispiel einer oeffentlichen
Adresse). Die alte IP und der alte Fingerabdruck stehen unten fest: ein
Tippfehler beim Aufruf aendert nichts Falsches.
"""
import argparse
import ipaddress
import os
import re
import subprocess
import sys

ALT_IP = "188.172.229.121"
ALT_FP = "SHA256:MAkOsPuhsFZUZViP8NPq+WxW1k6ykAF4mOy81wYNURA"
FP_FORM = re.compile(r"^SHA256:[A-Za-z0-9+/]{43}$")
AUSGENOMMEN_PRAEFIX = ("docs/",)
AUSGENOMMEN_DATEI = {
    "scripts/c1_ziel_wechseln.py",
    "scripts/lint_c1_ziel.py",
    "scripts/test_c1_ziel_wechseln.py",
    "scripts/lint_alte_hosts.py",
    "x/humanity/keeper/ssrf_sonderadressen_test.go",
}
MAX_DATEI = 8 << 20  # groessere Dateien sind keine Quelltexte


def ip_pruefen(roh: str) -> str:
    try:
        ip = ipaddress.IPv4Address(roh)
    except ValueError:
        raise SystemExit(f"--neu-ip: keine IPv4-Adresse: {roh!r}")
    if not ip.is_global:
        raise SystemExit(f"--neu-ip: nicht oeffentlich erreichbar: {ip}")
    if str(ip) == ALT_IP:
        raise SystemExit("--neu-ip: das ist die alte IP")
    return str(ip)


def fp_pruefen(roh: str) -> str:
    if not FP_FORM.match(roh):
        raise SystemExit("--neu-fp: Form SHA256:<43 Zeichen base64 ohne '='> (ssh-keygen -lf ... -E sha256)")
    if roh == ALT_FP:
        raise SystemExit("--neu-fp: das ist der alte Fingerabdruck")
    return roh


def dateien(wurzel: str) -> list[str]:
    aus = subprocess.run(["git", "ls-files", "-z"], cwd=wurzel, check=True, capture_output=True).stdout
    liste = []
    for p in aus.decode().split("\0"):
        if not p or p.startswith(AUSGENOMMEN_PRAEFIX) or p in AUSGENOMMEN_DATEI:
            continue
        voll = os.path.join(wurzel, p)
        if os.path.islink(voll) or not os.path.isfile(voll) or os.path.getsize(voll) > MAX_DATEI:
            continue
        liste.append(p)
    return liste


def ersetzen(wurzel: str, neu_ip: str, neu_fp: str, schreiben: bool) -> dict[str, int]:
    # Die IP nur als ganze Adresse ersetzen (nicht 188.172.229.1210 o. ae.).
    ip_muster = re.compile(r"(?<![0-9.])" + re.escape(ALT_IP) + r"(?![0-9])")
    # Der Explorer nutzt die IP auch als Schluessel ("x-ip4-188-172-229-121").
    alt_strich = ALT_IP.replace(".", "-")
    neu_strich = neu_ip.replace(".", "-")
    treffer = {}
    for p in dateien(wurzel):
        voll = os.path.join(wurzel, p)
        with open(voll, "rb") as f:
            roh = f.read()
        try:
            text = roh.decode("utf-8")
        except UnicodeDecodeError:
            continue
        neu, n_ip = ip_muster.subn(neu_ip, text)
        n_fp = neu.count(ALT_FP)
        neu = neu.replace(ALT_FP, neu_fp)
        n_st = neu.count("x-ip4-" + alt_strich)
        neu = neu.replace("x-ip4-" + alt_strich, "x-ip4-" + neu_strich)
        n = n_ip + n_fp + n_st
        if n:
            treffer[p] = n
            if schreiben:
                with open(voll, "w", encoding="utf-8", newline="") as f:
                    f.write(neu)
    return treffer


def alte_ip_sperren(wurzel: str, neu_ip: str) -> None:
    pfad = os.path.join(wurzel, "scripts/lint_alte_hosts.py")
    with open(pfad, encoding="utf-8") as f:
        text = f.read()
    if f'"{ALT_IP}":' in text:
        return
    zeile = f'    "{ALT_IP}": "C1 bis zum Umzug (netcup), danach abgeschaltet (neuer C1: {neu_ip})",\n'
    marke = "ABGESCHALTET = {\n"
    if marke not in text:
        raise SystemExit("lint_alte_hosts.py: ABGESCHALTET nicht gefunden")
    with open(pfad, "w", encoding="utf-8") as f:
        f.write(text.replace(marke, marke + zeile, 1))


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--neu-ip", required=True)
    ap.add_argument("--neu-fp", required=True)
    ap.add_argument("--schreiben", action="store_true")
    ap.add_argument("--wurzel", default=os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    a = ap.parse_args(argv)
    neu_ip, neu_fp = ip_pruefen(a.neu_ip), fp_pruefen(a.neu_fp)
    treffer = ersetzen(a.wurzel, neu_ip, neu_fp, a.schreiben)
    for p in sorted(treffer):
        print(f"{treffer[p]:5d}  {p}")
    print(f"{sum(treffer.values())} Stellen in {len(treffer)} Dateien" + ("" if a.schreiben else " (nur angezeigt, --schreiben zum Aendern)"))
    if not a.schreiben:
        return 0
    alte_ip_sperren(a.wurzel, neu_ip)
    return subprocess.run([sys.executable, os.path.join(a.wurzel, "scripts/lint_c1_ziel.py"), "--wurzel", a.wurzel]).returncode


if __name__ == "__main__":
    sys.exit(main())
