#!/usr/bin/env bash
# Laeuft ueber .github/workflows/widersprueche-pruefen.yml auf der Box.
# AKTION, KENNUNG, ERGEBNIS und BOX kommen base64-kodiert im Kopf.
set -u
echo "##### ${BOX} #####"
if ! docker inspect aequitas-coordinator >/dev/null 2>&1; then
  echo "kein Coordinator-Container auf dieser Box"; exit 0
fi
if [ "$AKTION" = "liste" ]; then
  docker exec aequitas-coordinator python -c '
import time
from app import widerspruch as w
def iso(t):
    return time.strftime("%Y-%m-%d %H:%M UTC", time.gmtime(t)) if t else "-"
z = w.zaehlstand()
print("Vorgaenge gesamt: %s | offen (widersprochen, nicht entschieden): %s"
      % (z["vorgaenge_gesamt"], z["widersprueche_offen"]))
jetzt = time.time()
for v in w.offene_vorgaenge():
    frist = v["widerspruch_am"] + 30 * 86400
    rest = int((frist - jetzt) // 86400)
    sp = v.get("standpunkt") or ""
    # Standpunkt und getroffene Kennung werden NICHT ausgegeben.
    print("  %s | abgewiesen %s | widersprochen %s | %s | Aehnlichkeit %s | Frist %s (%s)%s"
          % (v["kennung"], iso(v["abgewiesen_am"]), iso(v["widerspruch_am"]),
             v["entscheidung"], v["aehnlichkeit"], iso(frist),
             ("noch %d Tage" % rest) if rest >= 0 else ("UEBERSCHRITTEN seit %d Tagen" % -rest),
             (" | Standpunkt in der DB: %d Zeichen, hier nicht gezeigt" % len(sp)) if sp else ""))
if not z["widersprueche_offen"]:
    print("  (keine offenen Widersprueche)")
'
else
  docker exec -e KENNUNG -e ERGEBNIS aequitas-coordinator python -c '
import os
from app import widerspruch as w
k = os.environ["KENNUNG"].strip()
if w.vorgang_abschliessen(k, os.environ["ERGEBNIS"]):
    print("abgeschlossen: %s -- der Mensch sieht die Entscheidung unter /coordinator/widerspruch/%s" % (k, k))
else:
    print("unbekannt auf dieser Box (liegt auf der anderen, oder die Kennung stimmt nicht)")
'
fi
