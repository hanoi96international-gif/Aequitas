#!/usr/bin/env python3
"""Strenge Host-Pruefung: Kommentarzeilen von ssh-keyscan vor dem Zaehlen weg.

Warum (24.10.2026): Seit `ubuntu-latest` auf Ubuntu 26.04 zeigt, schreibt
ssh-keyscan (OpenSSH 10.2) zusaetzlich eine Kommentarzeile
"# host:22 SSH-2.0-..." auf stdout. Die Pruefung "genau eine ed25519-Zeile"
(Pruefung von #382, F1: sonst reicht ein gefaelschter Schluessel neben dem
echten) zaehlte sie mit -- jeder Workflow mit dieser Pruefung brach ab, auch
die naechtlichen Sicherungen. Auf 24.04 (OpenSSH 9.6) ging derselbe Schritt
durch.

Die Pruefung bleibt so streng wie vorher: gezaehlt werden die Schluessel-
zeilen, und es muss genau eine sein. Nur Kommentare zaehlen nicht. Dieser
Waechter haelt fest: jede Datei, in die ssh-keyscan schreibt und die danach
mit `grep -c .` auf genau eine Zeile geprueft wird, bekommt die Ausgabe ohne
Kommentarzeilen. `--selbsttest` prueft den Waechter an erfundenen Schritten.

Zweite Regel (28.10.2026): Wird ein Fingerabdruck nur unter den Zeilen
gesucht (`ssh-keygen -lf DATEI | ... grep -q`), muss der Scan in DATEI auf
`-t ed25519` beschraenkt sein. Sonst reicht EIN passender Schluessel unter
allen Typen, die anderen landen ungeprueft in known_hosts, und ssh kann
einen davon aushandeln (gefunden in zwei aus privaten Repos uebernommenen
Workflows).
"""
import glob
import re
import sys

ZAEHLEN = re.compile(r'grep -c \. (~/\.ssh/[\w.-]+)\)" = 1')
SCAN = re.compile(r"ssh-keyscan [^\n]*?>\s*(~/\.ssh/[\w.-]+)")
FILTER = "grep -v '^#'"
IRGENDEINER = re.compile(r"ssh-keygen -lf (~/\.ssh/[\w.-]+)[^\n]*\|\s*grep -q")


def befunde(name, text):
    fehler = []
    for datei in set(ZAEHLEN.findall(text)):
        scans = [m for m in SCAN.finditer(text) if m.group(1) == datei]
        if not scans:
            fehler.append(f"{name}: {datei} wird gezaehlt, aber kein ssh-keyscan schreibt hinein")
        for m in scans:
            if FILTER not in m.group(0):
                fehler.append(f"{name}: ssh-keyscan nach {datei} ohne {FILTER} -- "
                              "OpenSSH 10 schreibt eine Kommentarzeile, die Zaehlung bricht ab")
    for datei in set(IRGENDEINER.findall(text)):
        for m in SCAN.finditer(text):
            if m.group(1) == datei and "-t ed25519" not in m.group(0):
                fehler.append(f"{name}: Fingerabdruck wird unter allen Schluesseltypen in {datei} gesucht -- "
                              "ssh-keyscan -t ed25519 verwenden, sonst landen ungepruefte Typen in known_hosts")
    return fehler


def selbsttest():
    zaehlen = '[ "$(grep -c . ~/.ssh/kh1)" = 1 ] || exit 1\n'
    faelle = [
        ("ssh-keyscan -t ed25519 \"$H\" 2>/dev/null > ~/.ssh/kh1\n" + zaehlen, 1),
        ("ssh-keyscan -t ed25519 \"$H\" 2>/dev/null | { grep -v '^#' || true; } > ~/.ssh/kh1\n" + zaehlen, 0),
        ("ssh-keyscan -t ed25519 \"$H\" 2>/dev/null | grep -v '^#' > ~/.ssh/kh1\n" + zaehlen, 0),
        # Ohne strenge Zaehlung (nur Fingerabdruck) ist der Filter nicht noetig.
        ("ssh-keyscan -t ed25519 \"$H\" 2>/dev/null > ~/.ssh/kh1\nssh-keygen -lf ~/.ssh/kh1\n", 0),
        # Gezaehlt wird eine Datei, in die nichts geschrieben wird.
        (zaehlen, 1),
        # Zwei Scans in dieselbe Datei, einer ohne Filter.
        ("ssh-keyscan a 2>/dev/null | grep -v '^#' > ~/.ssh/kh1\nssh-keyscan b > ~/.ssh/kh1\n" + zaehlen, 1),
        # Angriff: alle Typen gescannt, ein passender reicht -- ein fremder
        # RSA-Schluessel neben dem echten ed25519 kaeme durch.
        ("ssh-keyscan \"$H\" 2>/dev/null > ~/.ssh/kh\n"
         "ssh-keygen -lf ~/.ssh/kh | awk '{print $2}' | grep -qx \"$FP\"\n", 1),
        ("ssh-keyscan \"$H\" 2>/dev/null > ~/.ssh/known_hosts\n"
         "ssh-keygen -lf ~/.ssh/known_hosts | awk '{print $2}' | grep -qx \"$FP\" || exit 1\n", 1),
        # Auf ed25519 beschraenkt: in Ordnung.
        ("ssh-keyscan -t ed25519 \"$H\" 2>/dev/null > ~/.ssh/kh\n"
         "ssh-keygen -lf ~/.ssh/kh | awk '{print $2}' | grep -qx \"$FP\"\n", 0),
        # Alle Typen, aber nur die passende Zeile behalten (Vergleich mit =): in Ordnung.
        ("ssh-keyscan \"$H\" 2>/dev/null > ~/.ssh/alle\n"
         "f=\"$(printf '%s\\n' \"$z\" | ssh-keygen -lf - | awk '{print $2}')\"\n", 0),
    ]
    falsch = 0
    for i, (text, n) in enumerate(faelle):
        b = befunde("test.yml", text)
        if len(b) != n:
            falsch += 1
            print(f"Selbsttest {i}: erwartet {n} Befund(e), bekommen {b}")
    print("Selbsttest bestanden." if not falsch else f"Selbsttest: {falsch} Fehler.")
    return 1 if falsch else 0


def main():
    if sys.argv[1:] == ["--selbsttest"]:
        return selbsttest()
    fehler = []
    pfade = sorted(glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml"))
    for pfad in pfade:
        fehler += befunde(pfad, open(pfad).read())
    for f in fehler:
        print(f"::error::{f}")
    print(f"keyscan-Pruefung: {len(fehler)} Verstoesse in {len(pfade)} Workflows")
    return 1 if fehler else 0


if __name__ == "__main__":
    sys.exit(main())
