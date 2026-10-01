# Verwaiste Test-Einschreibung(en) aus dem Vergleichsdienst DIESER Box
# entfernen. Einmalig nach dem Neustart vom 30.09.2026: eine Registrierung
# scheiterte nach der Gesichtspruefung am Ketten-Schritt (MetaMask
# "active chainId"), die Einschreibung blieb liegen, und jeder neue Versuch
# desselben Menschen wird als Duplikat abgewiesen. Vom Betreiber am
# 01.10.2026 ausdruecklich freigegeben.
#
# Fail-closed, loescht NUR wenn alles zutrifft:
#   - die Kette zaehlt 0 registrierte Menschen (dann ist JEDE Einschreibung
#     verwaist -- niemand verliert eine gueltige Registrierung),
#   - die Realgalerie ist leer (echte biometrische Daten werden nie beruehrt),
#   - die Testgalerie hat hoechstens 5 Eintraege (Plausibilitaet).
# Ausgegeben werden nur Zahlen.
set -euo pipefail
MATCH="$(docker ps --format '{{.Names}}' | grep -E '^aequitas-matching$' || true)"
[ -n "$MATCH" ] || { echo "::error::kein Container aequitas-matching"; exit 1; }
MENSCHEN="$(curl -fsS -m 15 https://aequitas.digital/api/status | python3 -c 'import json,sys; print(int(json.load(sys.stdin)["total_humans"]))')"
echo "Kette: $MENSCHEN registrierte Menschen"
[ "$MENSCHEN" = 0 ] || { echo "::error::Die Kette kennt $MENSCHEN Menschen -- Abbruch, hier wird nichts geloescht."; exit 1; }
docker exec -i "$MATCH" python3 - <<'PY'
import json, os, sqlite3, sys, urllib.request
h = json.load(urllib.request.urlopen("http://127.0.0.1:%s/health" % os.environ.get("PORT", "8098"), timeout=10))
real, test = int(h.get("gallery_real", -1)), int(h.get("gallery_test", -1))
print("vorher: gallery_real=%d gallery_test=%d" % (real, test))
if real != 0 or test < 0 or test > 5:
    print("::error::Realgalerie nicht leer oder Testgalerie unplausibel -- Abbruch")
    sys.exit(1)
c = sqlite3.connect(os.environ.get("POH_DB_PATH", "/app/data/poh.db"))
ts = [t for (t,) in c.execute("select name from sqlite_master where type='table' and name not like 'sqlite_%'")]
for t in ts:
    c.execute('delete from "%s"' % t)
c.commit()
c.execute("vacuum")
print("geleert: %d Tabellen" % len(ts))
PY
docker restart "$MATCH" >/dev/null
for i in $(seq 1 30); do
  if docker exec "$MATCH" python3 -c "import urllib.request,json;h=json.load(urllib.request.urlopen('http://127.0.0.1:8098/health',timeout=3));print('nachher: gallery_real=%d gallery_test=%d'%(h['gallery_real'],h['gallery_test']))" 2>/dev/null; then exit 0; fi
  sleep 3
done
echo "::error::Vergleichsdienst antwortet nach dem Neustart nicht"; exit 1
