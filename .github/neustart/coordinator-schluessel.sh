# Coordinator-Schluessel in die Vertrauensliste des Vergleichsdienstes
# (COORDINATOR_PUBLIC_KEYS) eintragen. Wird an gemeinsam.sh angehaengt.
#
# WARUM: Nach dem Neustart bei null ist das Coordinator-Register der Kette
# leer. Die Vergleichsdienste kannten die laufenden Coordinatoren nur von
# dort und lehnen sie jetzt mit 401 ab -- keine Registrierung kommt durch,
# und ohne registrierten Menschen kann sich kein Coordinator neu ins Register
# eintragen. Die Liste in der Env-Datei ist der dauerhafte Weg.
#
# Eingaben: BOX, MODUS (erkunden|ausfuehren), NEU (kommagetrennte
# oeffentliche Ed25519-Schluessel, je 64 Hex). Nur oeffentliche Schluessel,
# nie Geheimnisse; ausgegeben werden nur 12-Hex-Praefixe.
#
# Fail-closed: jede Abweichung vom erwarteten Aufbau bricht ab, bevor etwas
# geaendert wird. Kommt der neue Container nicht gesund hoch, laeuft wieder
# der alte (gleiche Env-Datei-Sicherung, gleicher Container).
set -euo pipefail
ENVFILE=/root/.aequitas-matching.env
M=aequitas-matching
ALT=aequitas-matching-vor-schluessel
echo "##### $BOX: Coordinator-Schluessel ($MODUS) #####"

praefixe() { tr ',' '\n' | sed 's/[[:space:]]//g' | grep -E . | cut -c1-12 | tr '\n' ' '; }

# Eingabe pruefen: nur 64-Hex-Eintraege, hoechstens 16.
n=0
for k in $(printf '%s' "$NEU" | tr ',' ' '); do
  [[ "$k" =~ ^[0-9a-f]{64}$ ]] || { echo "::error::kein oeffentlicher Schluessel: ${k:0:12}"; exit 1; }
  n=$((n+1))
done
[ "$n" -ge 1 ] && [ "$n" -le 16 ] || { echo "::error::$n Schluessel -- erwartet 1 bis 16"; exit 1; }

[ "$MATCH" = "$M" ] || { echo "::error::Vergleichsdienst heisst '$MATCH', erwartet $M"; exit 1; }
laeuft "$M" || { echo "::error::$M laeuft nicht"; exit 1; }
[ -f "$ENVFILE" ] || { echo "::error::$ENVFILE fehlt"; exit 1; }

JETZT="$(env_von "$M" COORDINATOR_PUBLIC_KEYS | tr 'A-F' 'a-f')"
echo "Vergleichsdienst traut heute: $(printf '%s' "$JETZT" | praefixe)"
if [ -n "$PROOF" ]; then
  echo "Proof-Server traut ($PROOF): $(env_von "$PROOF" COORDINATOR_PUBLIC_KEYS | tr 'A-F' 'a-f' | praefixe)"
fi

VEREINT="$(printf '%s,%s' "$JETZT" "$NEU" | tr ',' '\n' | sed 's/[[:space:]]//g' | grep -E '^[0-9a-f]{64}$' | awk '!s[$0]++' | paste -sd, -)"
FEHLT="$(printf '%s\n' "$NEU" | tr ',' '\n' | while read -r k; do printf '%s' "$JETZT" | grep -q "$k" || echo "$k"; done | paste -sd, -)"
if [ -n "$FEHLT" ]; then echo "Fehlt dem Vergleichsdienst: $(printf '%s' "$FEHLT" | praefixe)"; else echo "Fehlt dem Vergleichsdienst: nichts"; fi
echo "Danach: $(printf '%s' "$VEREINT" | praefixe)"

if [ -n "$PROOF" ]; then
  PJ="$(env_von "$PROOF" COORDINATOR_PUBLIC_KEYS | tr 'A-F' 'a-f')"
  for k in $(printf '%s' "$NEU" | tr ',' ' '); do
    printf '%s' "$PJ" | grep -q "$k" || echo "::warning::Proof-Server auf $BOX traut ${k:0:12} NICHT"
  done
fi

[ "$MODUS" = ausfuehren ] || { echo "(erkunden: nichts geaendert)"; exit 0; }
[ -n "$FEHLT" ] || { echo "nichts zu tun"; exit 0; }

# Aufbau pruefen, bevor irgendetwas angefasst wird.
NET="$(docker inspect "$M" --format '{{.HostConfig.NetworkMode}}')"
[ "$NET" = aequitas-net ] || { echo "::error::Netz $NET, erwartet aequitas-net"; exit 1; }
MNT="$(docker inspect "$M" --format '{{range .Mounts}}{{.Source}}:{{.Destination}} {{end}}')"
[ "$MNT" = "/root/aequitas-matching-data:/data " ] || { echo "::error::unerwartete Mounts: $MNT"; exit 1; }
IMG="$(docker inspect "$M" --format '{{.Config.Image}}')"
PORT="$(env_von "$M" PORT)"; [[ "$PORT" =~ ^[0-9]+$ ]] || { echo "::error::PORT unlesbar"; exit 1; }
docker inspect "$ALT" >/dev/null 2>&1 && { echo "::error::$ALT existiert schon (voriger Lauf?)"; exit 1; }

TS="$(date -u +%Y%m%dT%H%M%SZ)"
umask 077
cp -a "$ENVFILE" "$ENVFILE.vor-schluessel-$TS"
# Env-Datei dauerhaft anpassen (kuenftige Deploys uebernehmen sie).
grep -v '^COORDINATOR_PUBLIC_KEYS=' "$ENVFILE" > "$ENVFILE.neu" || true
echo "COORDINATOR_PUBLIC_KEYS=$VEREINT" >> "$ENVFILE.neu"
mv "$ENVFILE.neu" "$ENVFILE"; chmod 600 "$ENVFILE"

# Container-Umgebung vollstaendig uebernehmen (auch die -e-Werte des Deploys),
# nur die Liste ersetzt. Temporaere Datei nur fuer root, danach geloescht.
LAUF="$(mktemp /root/.matching-lauf.XXXXXX)"
trap 'rm -f "$LAUF"' EXIT
docker inspect "$M" --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E . | grep -v '^COORDINATOR_PUBLIC_KEYS=' > "$LAUF"
echo "COORDINATOR_PUBLIC_KEYS=$VEREINT" >> "$LAUF"

zurueck() {
  echo "::error::neuer Vergleichsdienst nicht gesund -- alter laeuft wieder"
  docker logs --tail 30 "$M" 2>&1 | grep -viE 'key|token|secret|seed' | cut -c1-200 || true
  docker rm -f "$M" >/dev/null 2>&1 || true
  docker rename "$ALT" "$M"; docker start "$M" >/dev/null
  cp -a "$ENVFILE.vor-schluessel-$TS" "$ENVFILE"
  exit 1
}

docker rename "$M" "$ALT"
docker stop -t 20 "$ALT" >/dev/null
docker run -d --name "$M" --network aequitas-net --restart unless-stopped \
  --env-file "$LAUF" --log-opt max-size=10m --log-opt max-file=3 \
  -v /root/aequitas-matching-data:/data "$IMG" >/dev/null || zurueck
rm -f "$LAUF"

bereit=0
for i in $(seq 1 60); do
  sleep 5
  if docker exec "$M" python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:$PORT/health', timeout=3)" >/dev/null 2>&1; then bereit=1; echo "antwortet nach $((i*5)) s"; break; fi
done
[ "$bereit" = 1 ] || zurueck
NACH="$(env_von "$M" COORDINATOR_PUBLIC_KEYS)"
[ "$NACH" = "$VEREINT" ] || zurueck
docker rm -f "$ALT" >/dev/null
echo "Vergleichsdienst traut jetzt: $(printf '%s' "$NACH" | praefixe)"
