#!/usr/bin/env bash
# Umzug C1, NEUER Server. Laeuft ueber .github/workflows/c1-umzug.yml.
#
#   MODUS=schluessel  age installieren, Umzugsschluessel erzeugen (falls
#                     noch keiner da ist), NUR den oeffentlichen Teil
#                     ausgeben (eine Zeile age1...).
#   MODUS=probe       Strom von stdin entschluesseln und nach /root/umzug-probe
#                     auspacken, zaehlen (Dateien, Bytes) und WIEDER LOESCHEN.
#   MODUS=ernst       Strom entschluesseln, Volumes anlegen und alles an
#                     seinen Ort auspacken. Weigert sich, wenn hier schon ein
#                     Knoten laeuft oder schon ein Umzug ausgepackt wurde.
#   MODUS=aufraeumen  Umzugsschluessel loeschen (nach der Abnahme).
#
# Das Log ist oeffentlich: nur Namen, Anzahlen, Groessen.
set -euo pipefail
umask 077
MODUS="${MODUS:-}"
SCHL=/root/.umzug/schluessel

age_da() {
  command -v age >/dev/null && command -v age-keygen >/dev/null && return 0
  DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq age >/dev/null
  command -v age >/dev/null
}

zaehlen() {  # Dateien und Bytes unter einem Wurzelverzeichnis fuer die Pfade aus dem Manifest
  local wurzel="$1" manifest="$2" n=0 b=0 p
  while read -r art rest; do
    case "$art" in
      volume) p="var/lib/docker/volumes/$rest/_data" ;;
      pfad)   p="${rest#/}" ;;
      *) continue ;;
    esac
    [ -e "$wurzel/$p" ] || continue
    n=$(( n + $(find "$wurzel/$p" -type f 2>/dev/null | wc -l) ))
    b=$(( b + $(du -sb "$wurzel/$p" 2>/dev/null | cut -f1) ))
  done < "$manifest"
  echo "$n $b"
}

case "$MODUS" in
  schluessel)
    age_da || { echo "age liess sich nicht installieren" >&2; exit 1; }
    mkdir -p /root/.umzug && chmod 700 /root/.umzug
    [ -s "$SCHL" ] || age-keygen -o "$SCHL" 2>/dev/null
    chmod 600 "$SCHL"
    age-keygen -y "$SCHL"
    ;;
  probe|ernst)
    [ -s "$SCHL" ] || { echo "kein Umzugsschluessel (erst MODUS=schluessel)" >&2; exit 1; }
    age_da
    if [ "$MODUS" = ernst ]; then
      if docker ps --format '{{.Names}}' 2>/dev/null | grep -qx aequitas-node; then
        echo "Hier laeuft schon ein Knoten -- Abbruch (nie zwei mit demselben Schluessel)" >&2; exit 1
      fi
      [ -e /root/.umzug/ausgepackt ] && { echo "Hier wurde schon ein Umzug ausgepackt ($(cat /root/.umzug/ausgepackt)) -- Abbruch" >&2; exit 1; }
      ZIEL=/root/.umzug/eingang
    else
      ZIEL=/root/umzug-probe
    fi
    rm -rf "$ZIEL"; mkdir -p "$ZIEL"
    # Der Zwischenstand enthaelt Klartext (Schluessel, Datenbank): weg, auch
    # bei jedem Abbruch.
    trap 'rm -rf "$ZIEL"' EXIT
    # -i: zwei tar-Stroeme hintereinander (Manifest, dann die Pfade).
    age -d -i "$SCHL" | tar -C "$ZIEL" --numeric-owner -xif -
    M="$ZIEL/umzug-meta"
    [ -s "$M/manifest" ] || { echo "Manifest fehlt -- Strom unvollstaendig" >&2; exit 1; }
    read -r SOLL_N SOLL_B < "$M/zaehlung"
    read -r IST_N IST_B < <(zaehlen "$ZIEL" "$M/manifest")
    echo "Modus der Quelle: $(cat "$M/modus"), Stand $(cat "$M/stand"), Hoehe $(cut -d' ' -f1 "$M/hoehe" 2>/dev/null)" >&2
    echo "Soll: $SOLL_N Dateien, $SOLL_B Bytes   Ist: $IST_N Dateien, $IST_B Bytes" >&2
    cut -d' ' -f1 "$M/manifest" | sort | uniq -c | sed 's/^/  /' >&2
    if [ "$SOLL_N" != "$IST_N" ]; then
      echo "ZAEHLUNG WEICHT AB -- nichts uebernommen" >&2
      [ "$MODUS" = probe ] && rm -rf "$ZIEL"
      exit 1
    fi
    if [ "$MODUS" = probe ]; then
      rm -rf "$ZIEL"
      echo "Probe gut: Strom vollstaendig, entschluesselt, gezaehlt -- und wieder geloescht." >&2
      exit 0
    fi
    [ "$(cat "$M/modus")" = ernst ] || { echo "Quelle war keine ernste Packung (Datenbank nicht konsistent) -- Abbruch" >&2; exit 1; }
    # Volumes anlegen, dann alles an seinen Ort.
    grep '^volume ' "$M/manifest" | cut -d' ' -f2 | while read -r v; do
      docker volume inspect "$v" >/dev/null 2>&1 || docker volume create "$v" >/dev/null
    done
    # NUR die Pfade aus dem Manifest, und vorhandene Verzeichnisse behalten
    # ihre Rechte (--no-overwrite-dir): sonst bekaemen /, /root, /var, /var/lib
    # die Rechte des Zwischenstands (0700) -- der Server waere fuer alle ausser
    # root kaputt (im CI am 22.11.2026 so passiert).
    awk '$1=="volume"{print "var/lib/docker/volumes/"$2"/_data"} $1=="pfad"{sub(/^\//,"",$2); print $2}' "$M/manifest" \
      | while read -r p; do [ -e "$ZIEL/$p" ] && printf '%s\n' "$p"; done > /root/.umzug/liste
    tar -C "$ZIEL" -cf - -T /root/.umzug/liste | tar -C / --numeric-owner --no-overwrite-dir -xpf -
    rm -f /root/.umzug/liste
    mkdir -p /root/.umzug/meta && cp -a "$M"/. /root/.umzug/meta/
    rm -rf "$ZIEL"
    cat /root/.umzug/meta/stand > /root/.umzug/ausgepackt
    echo "Ausgepackt. Container werden NICHT gestartet -- das tun die Einrichtungs-Workflows (Ablaufplan)." >&2
    ;;
  aufraeumen)
    rm -f "$SCHL"
    echo "Umzugsschluessel geloescht." >&2
    ;;
  *) echo "MODUS: schluessel|probe|ernst|aufraeumen" >&2; exit 1 ;;
esac
