#!/usr/bin/env python3
"""Alle Stellen meinen denselben C1: eine IP, ein gepinnter Host-Schluessel.

Massgeblich ist die Bootstrap-Adresse im Knoten (x/humanity/keeper/p2p.go,
defaultBootstrapNetcupC1). Geprueft wird:

1. Jede oeffentliche IPv4-Adresse in ausfuehrbaren Zeilen der Workflows und
   der Skripte, die sie auf Server schicken, ist diese IP -- ausser einer
   festen Liste fremder, harmloser Adressen (oeffentliche DNS-Resolver, der
   Webhoster der Website). Eine vergessene Stelle nach dem Umzug (oder ein
   Tippfehler) faellt so im CI auf, nicht mitten im Umzug.
2. Alle Workflows pinnen denselben Host-Schluessel (genau ein
   SHA256-Fingerabdruck). Zwei verschiedene hiesse: ein Teil der Workflows
   meldet sich bei einem anderen Rechner an als der Rest.
3. Die Seeds des Knotens (sync_blocks.go) und die Vorlage fuer neue
   Validatoren (deploy/validator/.env.example) nennen dieselbe IP.

Abgeschaltete Adressen prueft scripts/lint_alte_hosts.py.
Aufruf ohne Argumente im Repo; --selbsttest prueft den Waechter selbst.
"""
import argparse
import glob
import ipaddress
import os
import re
import sys
import tempfile

FREMD_ERLAUBT = {
    "1.1.1.1": "DNS-Resolver (Cloudflare)",
    "8.8.8.8": "DNS-Resolver (Google)",
    "9.9.9.9": "DNS-Resolver (Quad9)",
    "69.46.46.73": "Webhoster der Website (website-ansehen.yml)",
}
IP_MUSTER = re.compile(r"(?<![0-9.])((?:[0-9]{1,3}\.){3}[0-9]{1,3})(?![0-9])")
FP_MUSTER = re.compile(r"SHA256:[A-Za-z0-9+/]{43}")
BOOT_MUSTER = re.compile(r'defaultBootstrapNetcupC1\s*=\s*"/ip4/([0-9.]+)/tcp/')


def abgeschaltet(wurzel):
    pfad = os.path.join(wurzel, "scripts/lint_alte_hosts.py")
    try:
        with open(pfad, encoding="utf-8") as f:
            return set(re.findall(r'^\s*"([0-9.]+)":', f.read(), re.M))
    except OSError:
        return set()


def pruefen(wurzel):
    fehler = []
    try:
        with open(os.path.join(wurzel, "x/humanity/keeper/p2p.go"), encoding="utf-8") as f:
            m = BOOT_MUSTER.search(f.read())
    except OSError:
        m = None
    if not m:
        return ["p2p.go: defaultBootstrapNetcupC1 nicht gefunden"]
    c1 = m.group(1)
    alt = abgeschaltet(wurzel)
    if c1 in alt:
        fehler.append(f"p2p.go: C1 {c1} steht in lint_alte_hosts.py als abgeschaltet")

    wf = sorted(glob.glob(os.path.join(wurzel, ".github/workflows/*.yml")) + glob.glob(os.path.join(wurzel, ".github/workflows/*.yaml")))
    skripte = sorted(glob.glob(os.path.join(wurzel, ".github/neustart/*.sh")) + glob.glob(os.path.join(wurzel, "deploy/*.sh")))
    fps = {}
    for pfad in wf + skripte:
        rel = os.path.relpath(pfad, wurzel)
        with open(pfad, encoding="utf-8") as f:
            for nr, zeile in enumerate(f, 1):
                if zeile.lstrip().startswith("#"):
                    continue
                for ip in IP_MUSTER.findall(zeile):
                    try:
                        a = ipaddress.IPv4Address(ip)
                    except ValueError:
                        continue
                    if not a.is_global or ip == c1 or ip in FREMD_ERLAUBT or ip in alt:
                        continue
                    fehler.append(f"{rel}:{nr}: {ip} ist weder C1 ({c1}) noch eine erlaubte fremde Adresse")
                if pfad in wf:
                    for fp in FP_MUSTER.findall(zeile):
                        fps.setdefault(fp, []).append(f"{rel}:{nr}")
    if len(fps) > 1:
        for fp, orte in sorted(fps.items()):
            fehler.append(f"Host-Schluessel {fp}: {len(orte)} Stellen, z. B. {orte[0]}")
        fehler.append("Workflows pinnen verschiedene Host-Schluessel -- nach dem Umzug alle mit scripts/c1_ziel_wechseln.py tauschen")

    for rel, muss in (("x/humanity/keeper/sync_blocks.go", f"http://{c1}:8080"),
                      ("deploy/validator/.env.example", f"PRIMARY_NODE_URLS=http://{c1}:8080")):
        try:
            with open(os.path.join(wurzel, rel), encoding="utf-8") as f:
                if muss not in f.read():
                    fehler.append(f"{rel}: nennt nicht {muss}")
        except OSError:
            fehler.append(f"{rel}: fehlt")
    return fehler


def selbsttest():
    def baue(d, ip, wf_zeilen, fp_b=None):
        os.makedirs(os.path.join(d, "x/humanity/keeper"))
        os.makedirs(os.path.join(d, ".github/workflows"))
        os.makedirs(os.path.join(d, "deploy/validator"))
        os.makedirs(os.path.join(d, "scripts"))
        open(os.path.join(d, "x/humanity/keeper/p2p.go"), "w").write(f'defaultBootstrapNetcupC1 = "/ip4/{ip}/tcp/4001/p2p/x"\n')
        open(os.path.join(d, "x/humanity/keeper/sync_blocks.go"), "w").write(f'"http://{ip}:8080",\n')
        open(os.path.join(d, "deploy/validator/.env.example"), "w").write(f"PRIMARY_NODE_URLS=http://{ip}:8080\n")
        open(os.path.join(d, "scripts/lint_alte_hosts.py"), "w").write('ABGESCHALTET = {\n    "173.249.37.118": "alt",\n}\n')
        open(os.path.join(d, ".github/workflows/a.yml"), "w").write("\n".join(wf_zeilen) + "\n")
        fp_a = "SHA256:" + "A" * 43
        open(os.path.join(d, ".github/workflows/b.yml"), "w").write(f"fp: {fp_a}\n")
        if fp_b:
            open(os.path.join(d, ".github/workflows/c.yml"), "w").write(f"fp: {fp_b}\n")

    faelle = [
        ("gut", "93.184.216.34", ["C1: 93.184.216.34", "dig @1.1.1.1", "curl 127.0.0.1", "x 173.249.37.118"], None, True),
        ("vergessene alte IP", "93.184.216.34", ["C1: 45.33.32.156"], None, False),
        ("Kommentar darf", "93.184.216.34", ["# frueher 45.33.32.156"], None, True),
        ("zwei Host-Schluessel", "93.184.216.34", ["C1: 93.184.216.34"], "SHA256:" + "B" * 43, False),
    ]
    ok = True
    for name, ip, zeilen, fp_b, soll in faelle:
        with tempfile.TemporaryDirectory() as d:
            baue(d, ip, zeilen, fp_b)
            ist = not pruefen(d)
            if ist != soll:
                ok = False
                print(f"SELBSTTEST FALSCH: {name}: erwartet {'ok' if soll else 'Fehler'}")
    # Seeds zeigen woanders hin
    with tempfile.TemporaryDirectory() as d:
        baue(d, "93.184.216.34", ["C1: 93.184.216.34"])
        open(os.path.join(d, "x/humanity/keeper/sync_blocks.go"), "w").write('"http://45.33.32.156:8080",\n')
        if not pruefen(d):
            ok = False
            print("SELBSTTEST FALSCH: Seeds auf anderer IP nicht erkannt")
    print("Selbsttest ok" if ok else "Selbsttest FEHLGESCHLAGEN")
    return 0 if ok else 1


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--selbsttest", action="store_true")
    ap.add_argument("--wurzel", default=os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    a = ap.parse_args()
    if a.selbsttest:
        return selbsttest()
    fehler = pruefen(a.wurzel)
    if fehler:
        print("C1-Ziel uneinheitlich:")
        print("\n".join(fehler))
        return 1
    print("ok: alle Stellen meinen denselben C1 (eine IP, ein Host-Schluessel)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
