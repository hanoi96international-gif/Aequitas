# Startet den geleerten Knoten mit der V8-Genesis und prueft, dass er als
# neues Netz hochkommt (V8, null Menschen). C1 muss annehmen; C2 folgt C1.
# Danach Proof-Server und Coordinator neu starten (Zustand im Speicher).
set -euo pipefail
feld() { printf '%s' "$1" | grep -oE "\"$2\": ?(\"[^\"]*\"|[0-9]+|true|false)" | head -1 | sed -E 's/^"[^"]*": ?//; s/"//g'; }

echo "=== $NODE pruefen (vorher vom Deploy-Skript mit dem Stand von main gebaut) ==="
laeuft "$NODE" || docker start "$NODE" >/dev/null
docker exec "$NODE" cat genesis.json
S=""
for i in $(seq 1 120); do
  S=$(curl -s -m 5 http://127.0.0.1:8080/api/status || true)
  [ -n "$(feld "$S" register_vertrag)" ] && break
  sleep 5
done
V=$(feld "$S" register_vertrag); K=$(feld "$S" netz_kennung); M=$(feld "$S" total_humans); H=$(feld "$S" height)
echo "register_vertrag=$V netz_kennung=$K total_humans=$M height=$H"
[ "$V" = v8 ] || { echo "::error::Knoten meldet nicht v8"; docker logs --tail 60 "$NODE" 2>&1 | tail -60; exit 1; }
[ "$M" = 0 ] || { echo "::error::total_humans ist $M, nicht 0 -- die Datenbank war nicht leer"; exit 1; }

if [ "$BOX" = C1 ]; then
  for i in $(seq 1 60); do
    C=$(curl -s -m 5 http://127.0.0.1:8080/api/health/combined || true)
    echo "$C" | grep -qE '"nimmt_an": ?true' && break
    sleep 5
  done
  echo "$C" | grep -qE '"nimmt_an": ?true' || { echo "::error::C1 nimmt nicht an"; exit 1; }
  echo "C1 nimmt an."
fi

H0=$(feld "$(curl -s -m 5 http://127.0.0.1:8080/api/status)" height)
sleep 30
H1=$(feld "$(curl -s -m 5 http://127.0.0.1:8080/api/status)" height)
echo "Hoehe $H0 -> $H1"
[ "${H1:-0}" -gt "${H0:-0}" ] || echo "::warning::Hoehe steigt in 30 s nicht -- beobachten"

for c in "$PROOF" "$COORD"; do
  [ -n "$c" ] && docker inspect "$c" >/dev/null 2>&1 && docker restart "$c" >/dev/null && echo "$c neu gestartet"
done
echo "=== $BOX laeuft als neues Netz ($K) ==="
