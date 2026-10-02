#!/usr/bin/env python3
"""Kein Workflow spricht einen abgeschalteten Server an.

Der alte Contabo1 (173.249.37.118) ist seit dem 24.09.2026 abgeschaltet, die
IP vergibt der Anbieter neu. Am 02.10.2026 standen noch 127 Workflows mit
dieser Adresse im Repository -- ein manueller Lauf haette sich per SSH bei
einem Fremden angemeldet, ihm Befehle, Dateien oder Umgebungswerte geschickt
und seine Antworten als Messung genommen. Die Adresse ist dort durch einen
Namen ersetzt, der nie aufloest (.invalid, RFC 6761): ein solcher Lauf
scheitert, statt irgendwohin zu gehen.

Dieser Waechter haelt es so. Kommentare duerfen die Adresse nennen
(Geschichte), ausfuehrbare Zeilen nicht.
"""
import glob
import sys

ABGESCHALTET = {
    "173.249.37.118": "alter Contabo1, abgeschaltet seit 24.09.2026 (neuer C1: 188.172.229.121)",
}

fehler = []
for pfad in sorted(glob.glob(".github/workflows/*.yml") + glob.glob(".github/workflows/*.yaml")):
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
