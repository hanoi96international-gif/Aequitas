#!/usr/bin/env bash
# Caddy auf dem neuen C1 mit deploy/Caddyfile.neuer-server (liegt schon auf
# der Box unter /root/Aequitas/deploy/). Prueft vor dem Umschalten, laedt
# ohne Neustart nach und prueft alle oeffentlichen Wege danach.
# Laeuft ueber .github/workflows/neuer-server-caddy.yml. Mehrfach ausfuehrbar.
set -euo pipefail
CF=/root/caddy/Caddyfile
NEU=/root/Aequitas/deploy/Caddyfile.neuer-server
mkdir -p /root/caddy
docker run --rm -v "$NEU":/etc/caddy/Caddyfile:ro caddy:2 caddy validate --config /etc/caddy/Caddyfile >/dev/null
if docker ps -a --format '{{.Names}}' | grep -qx aequitas-caddy; then
  [ -f "$CF" ] && cp -a "$CF" "$CF.bak-$(date +%s)"
  # Inhalt ersetzen, Datei behalten: sie ist als Datei eingehaengt, ein neuer
  # Inode wuerde im Container nicht ankommen.
  cat "$NEU" > "$CF"
  docker exec aequitas-caddy caddy reload --config /etc/caddy/Caddyfile
else
  cp "$NEU" "$CF"
  docker run -d --name aequitas-caddy --restart unless-stopped \
    --network aequitas-net -p 80:80 -p 443:443 \
    -v "$CF":/etc/caddy/Caddyfile:ro \
    -v caddy_data:/data -v caddy_config:/config -v caddy_logs:/var/log/caddy \
    caddy:2 >/dev/null
fi
sleep 5
fehler=0
for u in https://aequitas.digital/api/status https://proof1.aequitas.digital/health \
         https://proof1.aequitas.digital/matching/health https://proof1.aequitas.digital/coordinator/health https://proof1.aequitas.digital/coordinator-c1/health \
         https://proof1.aequitas.digital/api/status; do
  c=000
  for i in $(seq 1 12); do
    c=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$u" || echo 000)
    [ "$c" = 200 ] && break; sleep 5
  done
  echo "  $u -> $c"
  [ "$c" = 200 ] || fehler=1
done
[ "$fehler" = 0 ] || { docker logs --tail 30 aequitas-caddy 2>&1 | tail -30; exit 1; }
echo "alle Wege antworten"
