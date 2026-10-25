#!/usr/bin/env bash
# Caddy auf dem neuen C1 mit deploy/Caddyfile.neuer-server (liegt schon auf
# der Box unter /root/Aequitas/deploy/). Prueft vor dem Umschalten, laedt
# ohne Neustart nach und prueft alle oeffentlichen Wege danach.
# Laeuft ueber .github/workflows/neuer-server-caddy.yml. Mehrfach ausfuehrbar.
set -euo pipefail
CF=/root/caddy/Caddyfile
NEU=/root/Aequitas/deploy/Caddyfile.neuer-server
mkdir -p /root/caddy
# /coordinator/* zeigt auf den eigenen Coordinator. Nur umschalten, wenn
# Vergleichsdienst und Proof-Server genau den Schluessel anerkennen, mit dem
# er gerade unterschreibt -- sonst lehnen sie jede Bezeugung ab, und die
# Registrierung ueber proof1 faellt aus. Bis zum 25.10.2026 stand hier ein
# fester Schluessel und die Abfrage des Registers auf der Kette; das Register
# ist seit dem Neustart bei null leer (coordinator-auth-diagnose.yml), die
# Pruefung konnte nie gruen werden, und nach einem Notfall-Wechsel
# (coordinator-schluessel-wechseln.yml, biometric) waere der feste Wert
# veraltet. Gelesen wird nur auf C1, von den Containern selbst.
# --- coordinator_anerkannt anfang (scripts/test_caddy_coordinator.sh)
# $1: Schluessel des Coordinators, $2: COORDINATOR_PUBLIC_KEYS (Komma-Liste).
# Wahr nur, wenn $1 genau 64 Zeichen Hex (klein) ist und als ganzer Eintrag
# in der Liste steht -- kein Teilstueck, kein leerer Wert.
coordinator_anerkannt() {
  local k=$1 e teile
  [[ $k =~ ^[0-9a-f]{64}$ ]] || return 1
  IFS=, read -r -a teile <<<"$2"
  for e in "${teile[@]}"; do
    e=$(printf '%s' "$e" | tr -d '[:space:]' | tr 'A-F' 'a-f')
    [ "$e" = "$k" ] && return 0
  done
  return 1
}
# --- coordinator_anerkannt ende
# Nur noetig, wenn sich das Caddyfile aendert: ist es schon auf dem Stand des
# Repos (etwa beim reinen Neuanlegen fuer die Log-Rotation), aendert sich am
# Routing nichts.
if [ -f "$CF" ] && cmp -s "$NEU" "$CF"; then
  echo "  Caddyfile unveraendert -- Routing bleibt, keine Coordinator-Pruefung noetig"
else
  KOORD=$(docker exec aequitas-node wget -qO- -T 20 http://aequitas-coordinator:8200/inventory 2>/dev/null \
    | python3 -c "import sys,json; print(((json.load(sys.stdin).get('bereit_fuer_echtbetrieb') or {}).get('attestation_public_key') or '').strip().lower())" 2>/dev/null || true)
  [[ $KOORD =~ ^[0-9a-f]{64}$ ]] || { echo "::error::Coordinator auf C1 nennt keinen Schluessel (/inventory) -- Caddy bleibt unveraendert"; exit 1; }
  echo "  Coordinator unterschreibt mit ${KOORD:0:12}..."
  for c in aequitas-matching proof-server; do
    liste=$(docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
      | grep '^COORDINATOR_PUBLIC_KEYS=' | tail -1 | cut -d= -f2- || true)
    if coordinator_anerkannt "$KOORD" "$liste"; then
      echo "  $c erkennt ihn an"
    else
      echo "::error::$c erkennt den Coordinator nicht an (COORDINATOR_PUBLIC_KEYS) -- Caddy bleibt unveraendert"; exit 1
    fi
  done
fi
docker run --rm -v "$NEU":/etc/caddy/Caddyfile:ro caddy:2 caddy validate --config /etc/caddy/Caddyfile >/dev/null
# Log-Rotation am Container selbst (Server-Haertung, Betreiber-Punkt 13): die
# Vorgabe in daemon.json gilt erst nach einem Docker-Neustart, der alle
# Container anhielte. NEU_ANLEGEN=1 legt einen laufenden aequitas-caddy
# einmal neu an -- gleiche Volumes (Zertifikate bleiben), kurze Unterbrechung
# von HTTPS. Ohne den Schalter bleibt es beim Nachladen.
LOGS="--log-opt max-size=50m --log-opt max-file=5"
case "${NEU_ANLEGEN:-0}" in 0|1) ;; *) echo "::error::NEU_ANLEGEN muss 0 oder 1 sein"; exit 1 ;; esac
if [ "${NEU_ANLEGEN:-0}" = 1 ] && docker ps -a --format '{{.Names}}' | grep -qx aequitas-caddy; then
  if [ "$(docker inspect -f '{{index .HostConfig.LogConfig.Config "max-size"}}' aequitas-caddy)" != "" ]; then
    echo "  aequitas-caddy rotiert seine Logs schon -- nicht neu angelegt"
  else
    # Nur neu anlegen, wenn der Container genau so eingehaengt ist, wie ihn
    # dieses Skript anlegt -- sonst gingen Zertifikate oder Konfiguration
    # verloren.
    MOUNTS=$(docker inspect -f '{{range .Mounts}}{{if .Name}}{{.Name}}{{else}}{{.Source}}{{end}}:{{.Destination}} {{end}}' aequitas-caddy)
    for m in "$CF:/etc/caddy/Caddyfile" "caddy_data:/data" "caddy_config:/config"; do
      case " $MOUNTS " in *" $m "*) ;; *) echo "::error::aequitas-caddy hat $m nicht ($MOUNTS) -- nicht neu angelegt"; exit 1 ;; esac
    done
    [ -f "$CF" ] && cp -a "$CF" "$CF.bak-$(date +%s)"
    docker rm -f aequitas-caddy >/dev/null
    echo "  aequitas-caddy wird mit Log-Rotation neu angelegt"
  fi
fi
if docker ps -a --format '{{.Names}}' | grep -qx aequitas-caddy; then
  [ -f "$CF" ] && cp -a "$CF" "$CF.bak-$(date +%s)"
  # Inhalt ersetzen, Datei behalten: sie ist als Datei eingehaengt, ein neuer
  # Inode wuerde im Container nicht ankommen.
  cat "$NEU" > "$CF"
  docker exec aequitas-caddy caddy reload --config /etc/caddy/Caddyfile
else
  cp "$NEU" "$CF"
  docker run -d --name aequitas-caddy --restart unless-stopped $LOGS \
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
