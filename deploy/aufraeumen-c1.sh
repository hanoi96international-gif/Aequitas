#!/usr/bin/env bash
# Platz auf C1 freigeben -- nur, was KEIN Container braucht. Laeuft ueber
# .github/workflows/c1-aufraeumen.yml (MODUS=pruefen|loeschen bash -s).
#
# Geloescht wird (nur MODUS=loeschen):
#   - Volumes, die kein Container nennt (auch kein angehaltener), und nur
#     anonyme (64 Hex-Zeichen) -- benannte wie validator_pgdata nie;
#   - Images, die kein Container nutzt, AUSSER den geschuetzten Namen
#     (aequitas-node:vorher ist die Ruecknahme des Deploys, rueckrollen-c1.sh);
#   - der Build-Cache (macht nur den naechsten Bau langsamer).
# Laufende und angehaltene Container, ihre Images und Volumes bleiben.
# Das Log ist oeffentlich: nur Namen, Groessen, Anzahlen.
set -euo pipefail
MODUS="${MODUS:-pruefen}"
case "$MODUS" in pruefen|loeschen) ;; *) echo "MODUS: pruefen|loeschen" >&2; exit 1 ;; esac
GESCHUETZT='^(aequitas-node:(vorher|local)|aequitas-proof-server:latest)$'

echo "=== Vorher ==="
df -h / | tail -1 | awk '{print "  belegt " $3 " von " $2 " (" $5 ")"}'

# Was Container (alle, auch angehaltene) brauchen.
BENUTZT_VOL="$(docker ps -aq | xargs -r docker inspect --format '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}}{{"\n"}}{{end}}{{end}}' | sort -u)"
BENUTZT_IMG="$(docker ps -aq | xargs -r docker inspect --format '{{.Image}}' | sort -u)"

VOL_WEG=""; VOL_N=0
for v in $(docker volume ls -q); do
  printf '%s\n' "$BENUTZT_VOL" | grep -qx "$v" && continue
  [[ "$v" =~ ^[0-9a-f]{64}$ ]] || { echo "  bleibt (benannt, unbenutzt): $v"; continue; }
  VOL_WEG="$VOL_WEG $v"; VOL_N=$((VOL_N + 1))
done
VOL_GROESSE="$(for v in $VOL_WEG; do du -sb "$(docker volume inspect "$v" --format '{{.Mountpoint}}')" 2>/dev/null | cut -f1; done | awk '{s+=$1} END {printf "%.1f GB", s/1e9}')"
echo "=== Volumes: $VOL_N anonyme, unbenutzte -> $VOL_GROESSE ==="

IMG_WEG=""; IMG_N=0
while read -r id name; do
  printf '%s\n' "$BENUTZT_IMG" | grep -q "$id" && continue
  if [[ "$name" =~ $GESCHUETZT ]]; then echo "  bleibt (geschuetzt): $name"; continue; fi
  IMG_WEG="$IMG_WEG $id"; IMG_N=$((IMG_N + 1))
  echo "  Image weg: $name ($id)"
done < <(docker images --no-trunc --format '{{.ID}} {{.Repository}}:{{.Tag}}')
echo "=== Images: $IMG_N unbenutzte ==="
docker system df --format '  {{.Type}}: {{.Size}} (freigebbar {{.Reclaimable}})' 2>/dev/null || true

if [ "$MODUS" = pruefen ]; then
  echo "Nur geprueft -- nichts geloescht. Zum Loeschen: modus=loeschen, Bestaetigung AUFRAEUMEN."
  exit 0
fi

# Gegenprobe direkt vor dem Loeschen: der Knoten laeuft und nutzt validator_pgdata.
docker inspect aequitas-node --format '{{.State.Running}}' 2>/dev/null | grep -qx true || { echo "aequitas-node laeuft nicht -- Abbruch" >&2; exit 1; }
for v in $VOL_WEG; do docker volume rm "$v" >/dev/null 2>&1 || echo "  nicht geloescht (in Benutzung?): $v"; done
for i in $IMG_WEG; do docker rmi "$i" >/dev/null 2>&1 || echo "  Image nicht geloescht (abhaengig): $i"; done
docker builder prune -af >/dev/null 2>&1 || true
docker image prune -f >/dev/null 2>&1 || true
echo "=== Nachher ==="
df -h / | tail -1 | awk '{print "  belegt " $3 " von " $2 " (" $5 ")"}'
docker system df --format '  {{.Type}}: {{.Size}}' 2>/dev/null || true
curl -fsS -m 5 http://127.0.0.1:8080/api/status >/dev/null && echo "Knoten antwortet." || { echo "Knoten antwortet NICHT" >&2; exit 1; }
