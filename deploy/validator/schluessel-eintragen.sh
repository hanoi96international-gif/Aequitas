#!/usr/bin/env bash
# Feste Schluessel eines neuen Knotens in die .env (Beta-Pruefung 14.10.2026).
#
# Frueher erzeugte der Knoten RELAYER_PRIVATE_KEY und NODE_KEY beim ersten
# Start und druckte sie ins Log; die Skripte fischten sie dort heraus. Jedes
# Log (docker logs, Log-Workflows) trug damit den privaten Schluessel. Jetzt
# erzeugt `aequitasd schluessel` sie VOR dem Start und schreibt sie nur auf
# seine Standardausgabe -- von hier direkt in die .env, nie auf den Schirm.
#
# Aufruf im Verzeichnis mit der docker-compose.yml, nachdem das Image gebaut
# ist:  bash schluessel-eintragen.sh /pfad/zur/.env
# Vorhandene Werte bleiben, wie sie sind.
set -euo pipefail
ENVF="${1:?Pfad zur .env}"
fehlt() { ! grep -qE "^$1=.+" "$ENVF"; }
if ! fehlt RELAYER_PRIVATE_KEY && ! fehlt NODE_KEY; then
  echo "Schluessel stehen schon in $ENVF"
  exit 0
fi
umask 077
NEU="$(mktemp "$(dirname "$ENVF")/.schluessel.XXXXXX")"; trap 'rm -f "$NEU" "$NEU.env"' EXIT
docker compose run --rm --no-deps -T node ./aequitasd schluessel > "$NEU"
grep -qE '^RELAYER_PRIVATE_KEY=0x[0-9a-f]{64}$' "$NEU" && grep -qE '^NODE_KEY=[A-Za-z0-9+/=]{40,}$' "$NEU" \
  || { echo "aequitasd schluessel hat keine gueltigen Schluessel geliefert" >&2; exit 1; }
for name in RELAYER_PRIVATE_KEY NODE_KEY; do
  if fehlt "$name"; then
    grep -vE "^$name=" "$ENVF" > "$NEU.env" || true
    grep -E "^$name=" "$NEU" >> "$NEU.env"
    install -m 600 "$NEU.env" "$ENVF"
  fi
done
echo "Schluessel in $ENVF eingetragen (nicht ausgegeben)"
