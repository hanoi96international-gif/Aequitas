#!/usr/bin/env bash
# Caddy auf dem neuen Server starten -- ERST wenn der Knoten gleichauf ist.
#
# Die Domain zeigt bereits hierher. Ein Knoten, der noch aufholt, antwortete
# der App mit leeren Kontostaenden (genau der Fehler vom 14.08.2026, siehe
# deploy/Caddyfile). Lieber kurz gar keine Antwort als eine falsche: Caddy
# startet erst, wenn die Hoehe hier hoechstens 50 Bloecke hinter C2 liegt.
#
# Laeuft ueber .github/workflows/neuer-server-caddy.yml. Mehrfach ausfuehrbar.

set -euo pipefail
C2=http://194.163.188.71:8080

hoehe() { curl -s -m 5 "$1/api/status" | grep -oE '"height":[0-9]+' | grep -oE '[0-9]+' || echo 0; }

echo "=== warten, bis der Knoten gleichauf ist (bis 45 min) ==="
for i in $(seq 1 270); do
  L="$(hoehe http://127.0.0.1:8080)"; N="$(hoehe $C2)"
  [ $((i % 6)) -eq 1 ] && echo "$(date +%H:%M:%S) lokal $L, C2 $N"
  if [ "${L:-0}" -gt 0 ] && [ "${N:-0}" -gt 0 ] && [ $((N - L)) -le 50 ]; then
    echo "gleichauf: lokal $L, C2 $N"; break
  fi
  if [ "$i" -eq 270 ]; then echo "noch nicht gleichauf -- Caddy NICHT gestartet"; exit 1; fi
  sleep 10
done

echo "=== Caddy ==="
mkdir -p /root/caddy
cp /root/Aequitas/deploy/Caddyfile.neuer-server /root/caddy/Caddyfile
docker run --rm -v /root/caddy/Caddyfile:/etc/caddy/Caddyfile:ro caddy:2 caddy validate --config /etc/caddy/Caddyfile
if docker ps -a --format '{{.Names}}' | grep -qx aequitas-caddy; then
  docker exec aequitas-caddy caddy reload --config /etc/caddy/Caddyfile
else
  docker run -d --name aequitas-caddy --restart unless-stopped \
    --network aequitas-net -p 80:80 -p 443:443 \
    -v /root/caddy/Caddyfile:/etc/caddy/Caddyfile:ro \
    -v caddy_data:/data -v caddy_config:/config -v caddy_logs:/var/log/caddy \
    caddy:2 >/dev/null
fi

echo "=== Zertifikat und Antwort pruefen (bis 3 min) ==="
for i in $(seq 1 36); do
  if curl -fsS -m 10 https://aequitas.digital/api/status >/dev/null 2>&1; then
    echo "https://aequitas.digital antwortet: $(curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+')"
    exit 0
  fi
  sleep 5
done
echo "Noch keine HTTPS-Antwort -- Caddy-Log:"
docker logs --tail 30 aequitas-caddy 2>&1 | tail -30
exit 1
