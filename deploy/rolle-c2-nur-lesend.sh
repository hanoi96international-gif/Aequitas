#!/usr/bin/env bash
# C2 nimmt keine Ueberweisungen mehr an; C1 (netcup) wird seine Quelle.
#
# Seit C1 am 26.09.2026 wieder produziert, nahmen BEIDE Knoten an (kein
# ANNAHME_ROLLE gesetzt) -- genau der Zustand, in dem Kontenstaende
# auseinanderlaufen (annahme_tor.go). Zuerst hier abschalten: danach nimmt
# hoechstens einer an, nie zwei.
#
# Oeffentliches Log: nur Variablennamen und oeffentliche Adressen.
set -euo pipefail
ENVD=/root/.aequitas.env
C1=http://188.172.229.121:8080
namen() { grep -E '^[A-Z_][A-Z0-9_]*=' | cut -d= -f1 | grep -vE '^(PATH|HOME|HOSTNAME)$' | sort -u; }

hoehe() { curl -s -m 5 "$1/api/status" | grep -oE '"height":[0-9]+' | grep -oE '[0-9]+' || echo 0; }
L=$(hoehe http://127.0.0.1:8080); N=$(hoehe $C1)
echo "Hoehe C2 $L, C1 $N"
if [ "${N:-0}" -le 0 ] || [ $((L - N)) -gt 50 ] || [ $((N - L)) -gt 50 ]; then
  echo "C1 nicht gleichauf -- nichts geaendert"; exit 1
fi

# env-setzen.sh baut den Container aus $ENVD neu. Nur wenn die Datei alles
# enthaelt, was der laufende Container hat, geht dabei nichts verloren.
# Namen, die das Image selbst setzt (Dockerfile ENV), zaehlen nicht.
IMG=$(docker inspect aequitas-node --format '{{.Config.Image}}')
FEHLT=$(comm -23 <(docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' | namen) \
  <( { namen < "$ENVD"; docker image inspect "$IMG" --format '{{range .Config.Env}}{{println .}}{{end}}' | namen; } | sort -u) || true)
if [ -n "$FEHLT" ]; then
  echo "ABBRUCH: im laufenden Container, aber nicht in $ENVD: $FEHLT"; exit 1
fi
echo "Datei und Container stimmen in den Namen ueberein"

# Schutzsperre von env-setzen.sh: RESYNC_FROM_SNAPSHOT muss ausdruecklich
# false sein. Das ist die sichere Richtung (kein Ersetzen des Zustands durch
# einen Snapshot der Quelle beim Neustart).
cp -a "$ENVD" "$ENVD.bak-rolle-$(date +%s)"
sed -i '/^RESYNC_FROM_SNAPSHOT=/d' "$ENVD"
echo 'RESYNC_FROM_SNAPSHOT=false' >> "$ENVD"

bash /root/env-setzen.sh ANNAHME_ROLLE=nur_lesend PRIMARY_NODE_URLS=$C1 IS_PRIMARY_NODE=

echo "=== im Prozess ==="
docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E '^(ANNAHME_ROLLE|PRIMARY_NODE_URLS|RESYNC_FROM_SNAPSHOT|IS_PRIMARY_NODE)=' | sort
for i in $(seq 1 30); do
  S=$(curl -s -m 5 http://127.0.0.1:8080/api/health/combined || true)
  echo "$S" | grep -q '"nimmt_an"' && break
  sleep 5
done
echo "$S" | grep -oE '"annahme[^}]*\}' | head -c 400; echo
echo "$S" | grep -qE '"nimmt_an": ?false' || { echo "FEHLER: C2 meldet nicht nimmt_an=false"; exit 1; }
echo "C2 nimmt NICHT mehr an. Hoehe C2 $(hoehe http://127.0.0.1:8080), C1 $(hoehe $C1)"
