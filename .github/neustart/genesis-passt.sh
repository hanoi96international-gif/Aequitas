#!/usr/bin/env bash
# Sperre im normalen Deploy: die genesis.json im Repo muss zur LAUFENDEN
# Kette passen (Registervertrag und Genesis-Zeit, sichtbar in /api/status als
# register_vertrag und netz_kennung = aequitas-<chain>-<genesis-unix>).
# Eine neue Genesis auf der alten Datenbank ergaebe einen anderen
# Genesis-Block unter bestehenden Bloecken -- das darf nie ein gewoehnlicher
# Deploy tun. Der Wechsel geht nur ueber neustart-bei-null.yml (leeren, dann
# mit genau diesem Stand starten).
# Aufruf: genesis-passt.sh URL [URL ...] -- die erste Box, die antwortet, zaehlt.
set -euo pipefail
REPO_V=$(jq -r '.register_vertrag.version // "v7"' genesis.json)
REPO_T=$(date -u -d "$(jq -r '.genesis_time' genesis.json)" +%s)
for BASE in "$@"; do
  S=$(curl -s -m 10 "$BASE/api/status" || true)
  LIVE_V=$(printf '%s' "$S" | jq -r '.register_vertrag // empty' 2>/dev/null || true)
  LIVE_K=$(printf '%s' "$S" | jq -r '.netz_kennung // empty' 2>/dev/null || true)
  [ -n "$LIVE_V" ] && [ -n "$LIVE_K" ] || { echo "$BASE: kein Status"; continue; }
  LIVE_T=${LIVE_K##*-}
  echo "Repo: $REPO_V / $REPO_T   laufend ($BASE): $LIVE_V / $LIVE_T"
  if [ "$REPO_V" = "$LIVE_V" ] && [ "$REPO_T" = "$LIVE_T" ]; then
    echo "Genesis passt zur laufenden Kette."
    exit 0
  fi
  echo "::error::genesis.json im Repo passt nicht zur laufenden Kette. Ein gewoehnlicher Deploy wechselt die Genesis nie -- dafuer neustart-bei-null.yml (modus ausfuehren) starten."
  exit 1
done
echo "::error::Keine Box liefert register_vertrag und netz_kennung -- Genesis nicht pruefbar, Deploy angehalten."
exit 1
