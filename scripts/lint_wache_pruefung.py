#!/usr/bin/env python3
"""Die Wache wird rot, wenn ein Tor vor dem Praegen offen steht.

.github/wache_pruefung.py liest /api/health/combined einer Box. Diese Pruefung
fuehrt es mit erfundenen Antworten aus (Missbrauchstest nach AGENTS.md,
Frage 5) und haelt fest:

  1. Gruen nur mit Proof-Server "required" UND prove_herkunft.verlangt = true.
  2. Rot bei verlangt = false (REQUIRE_PROVE_PROVENANCE=false), bei fehlendem
     Feld (Knoten zu alt -- fail-closed) und bei einem Wert, der nur so
     aussieht ("true" als Zeichenkette).
  3. Rot bei einem Proof-Server-Modus ausser "required".
  4. Rot, wenn der neueste Block mehr als 300 s hinter der Uhr der Wache liegt
     (Blockzeit zurueckgehalten oder Knoten haengt) oder das Feld keine
     Ganzzahl ist; fehlt es (Knoten vor dem Deploy), nur ein Hinweis.
"""
import json
import os
import pathlib
import subprocess
import sys

JETZT = 1_900_000_000  # die Uhr der Wache im Test (WACHE_JETZT)
SKRIPT = pathlib.Path(__file__).resolve().parent.parent / ".github" / "wache_pruefung.py"


def box(**ersetzt):
    d = {
        "proof_server": {"reachable": True, "last_status": {"durchsetzung": {"mode": "required"}}},
        "prove_herkunft": {"verlangt": True},
        "plattenplatz": {"belegt_pct": 40},
        "chain": {"neuester_block_zeit": JETZT - 2},
    }
    for k, v in ersetzt.items():
        if v is None:
            d.pop(k, None)
        else:
            d[k] = v
    return d


FAELLE = [
    ("gesund", box(), 0),
    ("Tor aus", box(prove_herkunft={"verlangt": False}), 1),
    ("Feld fehlt", box(prove_herkunft=None), 1),
    ("Zeichenkette statt bool", box(prove_herkunft={"verlangt": "true"}), 1),
    ("Proof-Server optional", box(proof_server={"reachable": True, "last_status": {"durchsetzung": {"mode": "optional"}}}), 1),
    ("Blockzeit 300 s zurueck", box(chain={"neuester_block_zeit": JETZT - 300}), 0),
    ("Blockzeit 301 s zurueck", box(chain={"neuester_block_zeit": JETZT - 301}), 1),
    ("Blockzeit eine Stunde zurueck", box(chain={"neuester_block_zeit": JETZT - 3600}), 1),
    ("Blockzeit als Zeichenkette", box(chain={"neuester_block_zeit": str(JETZT)}), 1),
    ("Blockzeit null", box(chain={"neuester_block_zeit": 0}), 1),
    ("Blockzeit fehlt (alter Knoten)", box(chain={}), 0),
]


def main():
    fehler = []
    for name, d, erwartet in FAELLE:
        env = dict(os.environ, WACHE_JETZT=str(JETZT))
        r = subprocess.run([sys.executable, str(SKRIPT)], input=json.dumps(d), text=True, capture_output=True, env=env)
        if r.returncode != erwartet:
            fehler.append(f"{name}: Exit {r.returncode}, erwartet {erwartet}\n{r.stdout}{r.stderr}")
    for f in fehler:
        print("✗ " + f)
    if not fehler:
        print(f"✓ wache_pruefung.py: {len(FAELLE)} Faelle, rot genau dann, wenn ein Tor offen ist oder die Blockzeit haengt")
    sys.exit(1 if fehler else 0)


if __name__ == "__main__":
    main()
