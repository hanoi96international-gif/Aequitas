#!/usr/bin/env bash
# Umzug C1, ALTER Server: alles Noetige einpacken und VERSCHLUESSELT an den
# neuen Server schicken (stdout). Laeuft ueber .github/workflows/c1-umzug.yml:
#
#   ssh alt "MODUS=probe|ernst EMPF=age1... bash -s" < deploy/umzug-packen.sh \
#     | ssh neu "MODUS=... bash -s" < deploy/umzug-auspacken.sh
#
# VERSCHLUESSELUNG. EMPF ist der oeffentliche age-Schluessel, den der NEUE
# Server eben selbst erzeugt hat (umzug-auspacken.sh, Phase "schluessel");
# der private verlaesst ihn nie. Der Runner sieht nur Chiffrat, das Log nur
# Namen, Anzahlen und Groessen. age authentisiert: ein veraenderter oder
# abgeschnittener Strom wird beim Entpacken abgewiesen.
#
# WAS MIT GEHT: alle Volumes und gebundenen Pfade aller Container, die
# Konfiguration (deploy/validator/.env, /root/.*.env, /etc/aequitas-
# sicherung), docker inspect aller Container (zum Abgleich beim Neuaufbau).
#
# MODUS=probe: nichts wird angehalten (Transport-Probe; die Datenbank ist
# dann nicht konsistent -- sie wird auf dem neuen Server nur gezaehlt und
# wieder geloescht).
# MODUS=ernst: ALLE Container werden zuerst angehalten und ihr Neustart
# abgeschaltet (docker update --restart=no) -- sonst signierten alter und
# neuer C1 mit demselben Schluessel gleichzeitig. Die Liste der vorher
# laufenden Container steht in /root/.umzug/gestoppt (fuer den Rueckweg,
# c1-umzug.yml phase=zurueck). Geloescht wird auf dem alten Server NICHTS.
set -euo pipefail
umask 077
MODUS="${MODUS:-}"
EMPF="${EMPF:-}"
case "$MODUS" in probe|ernst) ;; *) echo "MODUS muss probe oder ernst sein" >&2; exit 1 ;; esac
REST="${EMPF#age1}"
if [ "${EMPF:0:4}" != age1 ] || [ "${#EMPF}" != 62 ] || [ -n "$(printf '%s' "$REST" | tr -d 'qpzry9x8gf2tvdw0s3jn54khce6mua7l')" ]; then
  echo "EMPF ist kein age-Empfaenger (age1..., 62 Zeichen)" >&2; exit 1
fi
AGE=/usr/local/lib/aequitas-sicherung/age
[ -x "$AGE" ] || AGE="$(command -v age || true)"
[ -n "$AGE" ] || { echo "age fehlt auf dem alten Server" >&2; exit 1; }

mkdir -p /root/.umzug
ARBEIT="$(mktemp -d /root/.umzug/paket.XXXXXX)"
trap 'rm -rf "$ARBEIT"' EXIT

ALLE="$(docker ps -a --format '{{.Names}}' | sort)"
if [ "$MODUS" = ernst ]; then
  # Je Zeile: Name und bisherige Neustart-Regel (fuer den Rueckweg).
  for c in $(docker ps --format '{{.Names}}' | sort); do
    echo "$c $(docker inspect "$c" --format '{{.HostConfig.RestartPolicy.Name}}')"
  done > /root/.umzug/gestoppt
  echo "Anhalten (Knoten zuerst): $(cut -d' ' -f1 /root/.umzug/gestoppt | tr '\n' ' ')" >&2
  for c in $ALLE; do docker update --restart=no "$c" >/dev/null 2>&1 || true; done
  docker stop -t 60 aequitas-node >/dev/null 2>&1 || true
  for c in $(cut -d' ' -f1 /root/.umzug/gestoppt); do docker stop -t 60 "$c" >/dev/null 2>&1 || true; done
  # Der Knoten darf nicht mehr antworten -- sonst kein Umzug.
  for i in 1 2 3 4 5; do
    if curl -fsS -m 2 http://127.0.0.1:8080/api/status >/dev/null 2>&1; then
      echo "Knoten antwortet noch -- Abbruch" >&2; exit 1
    fi
    sleep 2
  done
  [ -z "$(docker ps -q)" ] || { echo "Es laufen noch Container: $(docker ps --format '{{.Names}}' | tr '\n' ' ') -- Abbruch" >&2; exit 1; }
  echo "Alle Container angehalten, Neustart abgeschaltet." >&2
fi

# Liste der Pfade (relativ zu /) und Manifest fuer das Auspacken.
: > "$ARBEIT/manifest"
: > "$ARBEIT/pfade"
for c in $ALLE; do
  docker inspect "$c" --format '{{range .Mounts}}{{.Type}}|{{.Name}}|{{.Source}}{{println}}{{end}}' | grep -v '^$' | while IFS='|' read -r typ name quelle; do
    case "$typ" in
      volume) echo "volume $name" >> "$ARBEIT/manifest"; echo "${quelle#/}" >> "$ARBEIT/pfade" ;;
      bind)   [ -e "$quelle" ] && { echo "pfad $quelle" >> "$ARBEIT/manifest"; echo "${quelle#/}" >> "$ARBEIT/pfade"; } ;;
    esac
  done
done
for f in /root/Aequitas/deploy/validator/.env /root/.*.env /etc/aequitas-sicherung; do
  [ -e "$f" ] && { echo "pfad $f" >> "$ARBEIT/manifest"; echo "${f#/}" >> "$ARBEIT/pfade"; }
done
# Docker-Sockel u. ae. nie; doppelte Eintraege weg.
grep -vE '^(var/run|run)/docker\.sock$' "$ARBEIT/pfade" | sort -u > "$ARBEIT/pfade.u"
sort -u "$ARBEIT/manifest" > "$ARBEIT/manifest.u"
docker inspect $ALLE > "$ARBEIT/container.json"
printf '%s\n' "$MODUS" > "$ARBEIT/modus"
date -u +%Y-%m-%dT%H:%M:%SZ > "$ARBEIT/stand"
curl -fsS -m 3 http://127.0.0.1:8080/api/status 2>/dev/null | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("height",""), d.get("latest_hash",""))' > "$ARBEIT/hoehe" 2>/dev/null || true

N="$(wc -l < "$ARBEIT/pfade.u")"
GROESSE="$(cd / && du -scb $(cat "$ARBEIT/pfade.u") 2>/dev/null | tail -1 | cut -f1)"
DATEIEN="$(cd / && find $(cat "$ARBEIT/pfade.u") -type f 2>/dev/null | wc -l)"
echo "Pakete: $N Pfade, $DATEIEN Dateien, $GROESSE Bytes (Modus $MODUS)" >&2
cut -d' ' -f1 "$ARBEIT/manifest.u" | sort | uniq -c | sed 's/^/  /' >&2
echo "ZAEHLUNG $DATEIEN $GROESSE" >&2

# Erst das Manifest (als Verzeichnis umzug-meta/), dann die Pfade -- ein Strom.
mkdir "$ARBEIT/umzug-meta"
mv "$ARBEIT/manifest.u" "$ARBEIT/umzug-meta/manifest"
mv "$ARBEIT/container.json" "$ARBEIT/modus" "$ARBEIT/stand" "$ARBEIT/hoehe" "$ARBEIT/umzug-meta/"
echo "$DATEIEN $GROESSE" > "$ARBEIT/umzug-meta/zaehlung"
{
  tar -C "$ARBEIT" -cf - umzug-meta
  tar -C / --numeric-owner -cf - -T "$ARBEIT/pfade.u"
} | "$AGE" -r "$EMPF"
