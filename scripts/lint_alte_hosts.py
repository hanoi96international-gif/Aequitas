#!/usr/bin/env python3
"""Kein Workflow spricht einen abgeschalteten Server an.

Der alte Contabo1 (173.249.37.118) ist seit dem 24.09.2026 abgeschaltet, die
IP vergibt der Anbieter neu. Am 02.10.2026 standen noch 127 Workflows mit
dieser Adresse im Repository -- ein manueller Lauf haette sich per SSH bei
einem Fremden angemeldet, ihm Befehle, Dateien oder Umgebungswerte geschickt
und seine Antworten als Messung genommen. Die Adresse ist dort durch einen
Namen ersetzt, der nie aufloest (.invalid, RFC 6761): ein solcher Lauf
scheitert, statt irgendwohin zu gehen.

C2 (194.163.188.71) ist seit dem 03.10.2026 abgeschaltet; am 21.10.2026
meldete die IP einen fremden Host-Schluessel, sie ist also neu vergeben. Am
23.10.2026 standen noch 259 ausfuehrbare Zeilen in 183 Workflows darauf,
83 ssh-action-Schritte ohne fingerprint. Der schwerste Fall: der
Push-Deploy deploy-c1.yml fragte bei schweigendem C1 die C2-IP nach
der Genesis -- ein Fremder mit den oeffentlichen Werten haette die Sperre
geoeffnet. Ersetzt durch alter-c2-abgeschaltet.invalid.

Dieser Waechter haelt es so. Kommentare duerfen die Adresse nennen
(Geschichte), ausfuehrbare Zeilen nicht.
"""
import glob
import sys

ABGESCHALTET = {
    "173.249.37.118": "alter Contabo1, abgeschaltet seit 24.09.2026 (neuer C1: 188.172.229.121)",
    "194.163.188.71": "Contabo2, abgeschaltet seit 03.10.2026, IP neu vergeben (fremder Host-Schluessel)",
}

fehler = []
# Auch die Skripte, die Workflows auf die Server schicken: deploy/server-
# starten.sh trug bis zum 23.10.2026 die C2-IP als Snapshot-Quelle.
DATEIEN = (glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml")
           + glob.glob(".github/neustart/*.sh") + glob.glob(".github/pruefstand/*.sh")
           + glob.glob("deploy/*.sh"))
for pfad in sorted(DATEIEN):
    with open(pfad, encoding="utf-8") as f:
        for nr, zeile in enumerate(f, 1):
            if zeile.lstrip().startswith("#"):
                continue
            for adresse, grund in ABGESCHALTET.items():
                if adresse in zeile:
                    fehler.append(f"{pfad}:{nr}: {adresse} -- {grund}")

if fehler:
    print("Workflows sprechen abgeschaltete Server an:")
    print("\n".join(fehler))
    sys.exit(1)
print("ok: kein Workflow spricht einen abgeschalteten Server an")
