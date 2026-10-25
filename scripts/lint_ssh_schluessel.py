#!/usr/bin/env python3
"""Jeder SSH-Schluessel aus einem Secret wird mit derselben Reparatur geschrieben.

Warum (25.10.2026): Am Handy eingefuegt, verliert ein Secret seine
Zeilenumbrueche. `printf '%s\\n' "$KEY" > ~/.ssh/id_ed25519` schreibt dann eine
Zeile, die ssh nicht lesen kann -- jeder Server-Workflow scheiterte, und der
Tausch des Root-Schluessels von C1 stand still. Die Reparatur (REPARATUR unten,
getestet in scripts/test_ssh_schluessel_reparatur.sh) setzt Kopf, Rumpf und
Fuss wieder zusammen; ein richtig eingefuegter Schluessel bleibt byte-gleich,
alles andere bleibt unlesbar.

Dieser Waechter haelt fest: Wo eine Variable, die ein *SSH_KEY-Secret traegt,
in eine Datei geschrieben wird, steht genau diese Reparatur. `--reparatur`
gibt die Zeile fuer den Test aus, `--selbsttest` prueft den Waechter.
"""
import glob
import re
import sys

PERL = (r"s/\r//g; s/\s*(-----BEGIN OPENSSH PRIVATE KEY-----)\s*(.*?)\s*"
        r"(-----END OPENSSH PRIVATE KEY-----)\s*/my ($k, $b, $f) = ($1, $2, $3); "
        r"$b =~ s|\s||g; $b =~ s|(.{70})|$1\n|g; $b =~ s|\n\z||; "
        r'"$k\n$b\n$f\n"/se')


def reparatur(var):
    return f"printf '%s' \"${var}\" | perl -0777 -pe '{PERL}'"


# Variablen, die ein SSH-Schluessel-Secret tragen (Workflow-env).
TRAEGER = re.compile(r"^\s*(\w+):\s*\$\{\{\s*secrets(?:\.(\w*SSH_KEY)|\[matrix\.key\])\s*\}\}", re.M)
# Schreiben einer Variablen in eine Datei.
SCHREIBEN = re.compile(r"printf\s+'%s(?:\\n)?'\s+\"\$\{?(\w+)\}?\"(.*?)>\s*(\S+)")


def befunde(name, text, immer=()):
    vars_ = set(immer)
    for m in TRAEGER.finditer(text):
        if m.group(2) == "SICHERUNG_SSH_KEY":
            continue  # liest deploy/sicherung/schluessel-lesen.sh, mit eigener Pruefung
        vars_.add(m.group(1))
    fehler = []
    for nr, zeile in enumerate(text.splitlines(), 1):
        for m in SCHREIBEN.finditer(zeile):
            var = m.group(1)
            if var not in vars_:
                continue
            if reparatur(var) not in zeile:
                fehler.append(f"{name}:{nr}: ${var} wird ohne Reparatur geschrieben -- "
                              f"`{reparatur(var)} > {m.group(3)}` verwenden")
    return fehler


def selbsttest():
    env = "        env:\n          KEY: ${{ secrets.CONTABO_SSH_KEY }}\n"
    roh = "          printf '%s\\n' \"$KEY\" > ~/.ssh/id_ed25519 && chmod 600 ~/.ssh/id_ed25519\n"
    gut = f"          {reparatur('KEY')} > ~/.ssh/id_ed25519 && chmod 600 ~/.ssh/id_ed25519\n"
    faelle = [
        (env + roh, 1),
        (env + gut, 0),
        # Matrix-Secret
        ("          KEY: ${{ secrets[matrix.key] }}\n" + roh, 1),
        # Anderer Variablenname
        ("          K1: ${{ secrets.BOX_SSH_KEY }}\n          printf '%s\\n' \"$K1\" > ~/.ssh/c1\n", 1),
        # Kein Schluessel-Secret: kein Befund
        ("          T: ${{ secrets.TELEGRAM_BOT_TOKEN }}\n          printf '%s\\n' \"$T\" > t\n", 0),
        # Host-Zeile in known_hosts: kein Befund
        (env + gut + "          printf '%s\\n' \"$z\" >> ~/.ssh/known_hosts\n", 0),
        # Sicherungsschluessel hat eigenen Leser
        ("          KEY: ${{ secrets.SICHERUNG_SSH_KEY }}\n" + roh, 0),
        # Reparatur, aber fuer die falsche Variable
        ("          K1: ${{ secrets.CONTABO_SSH_KEY }}\n"
         f"          {reparatur('KEY')} > x; printf '%s\\n' \"$K1\" > ~/.ssh/c1\n", 1),
    ]
    falsch = 0
    for i, (text, n) in enumerate(faelle):
        b = befunde("test.yml", text)
        if len(b) != n:
            falsch += 1
            print(f"Selbsttest {i}: erwartet {n} Befund(e), bekommen {b}")
    # Shell-Skripte bekommen KEY von aussen.
    if len(befunde("x.sh", roh, immer=("KEY",))) != 1:
        falsch += 1
        print("Selbsttest sh: Rohschreiben in Skript nicht erkannt")
    print("Selbsttest bestanden." if not falsch else f"Selbsttest: {falsch} Fehler.")
    return 1 if falsch else 0


def main():
    if sys.argv[1:] == ["--reparatur"]:
        print(reparatur("KEY"))
        return 0
    if sys.argv[1:] == ["--selbsttest"]:
        return selbsttest()
    fehler = []
    pfade = sorted(glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml"))
    for pfad in pfade:
        fehler += befunde(pfad, open(pfad).read())
    # Skripte, die KEY aus dem Workflow bekommen.
    skripte = sorted(glob.glob(".github/neustart/*.sh"))
    for pfad in skripte:
        fehler += befunde(pfad, open(pfad).read(), immer=("KEY",))
    for f in fehler:
        print(f"::error::{f}")
    print(f"ssh-schluessel-Pruefung: {len(fehler)} Verstoesse in {len(pfade)} Workflows und {len(skripte)} Skripten")
    return 1 if fehler else 0


if __name__ == "__main__":
    sys.exit(main())
