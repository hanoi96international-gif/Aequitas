#!/usr/bin/env bash
# Gut- und Missbrauchsfaelle fuer deploy/quanten-c1-messverzeichnis.sh, ohne
# Server: Verzeichnisse unter mktemp. Als Nicht-root ist chown gefaelscht
# (schreibt mit); der Knotenbenutzer ist dann die eigene uid.
set -euo pipefail
cd "$(dirname "$0")/.."
SKRIPT="$PWD/deploy/quanten-c1-messverzeichnis.sh"
QUELLE="$PWD/deploy/validator/docker-compose.quanten.yml"
D=$(mktemp -d); trap 'rm -rf "$D"' EXIT
mkdir -p "$D/bin"
if [ "$(id -u)" = 0 ]; then
  ICH=4243   # als root wirklich setzen; das Skript weist uid 0 ab
else
  ICH=$(id -u)
  printf '#!/usr/bin/env bash\nprintf "%%s\\n" "$*" >> "%s/chown"\n' "$D" > "$D/bin/chown"
  chmod +x "$D/bin/chown"
fi
export PATH="$D/bin:$PATH"
MARKE='# Angelegt von deploy/quanten-c1-messverzeichnis.sh aus docker-compose.quanten.yml -- nicht von Hand aendern.'
neu() {
  rm -rf "$D/val" "$D/srv"; mkdir -p "$D/val" "$D/srv"
  printf 'ANNAHME_ROLLE=annehmend\n' > "$D/val/.env"
  cp "$QUELLE" "$D/val/docker-compose.quanten.yml"
}
lauf() { # erwartung beschreibung [env...]
  local soll=$1 was=$2; shift 2
  if env VAL="$D/val" QSD="$D/srv/q" QS_UID="$ICH" "$@" bash "$SKRIPT" >"$D/out" 2>"$D/err"; then e=ok; else e=halt; fi
  [ "$e" = "$soll" ] || { echo "FEHLER: $was -> $e, erwartet $soll"; cat "$D/err"; exit 1; }
  echo "ok: $was -> $e"
}
override_ok() {
  [ "$(head -n 1 "$D/val/docker-compose.override.yml")" = "$MARKE" ] || { echo "FEHLER: $1 -> Kennzeile fehlt"; exit 1; }
  diff <(tail -n +2 "$D/val/docker-compose.override.yml") "$QUELLE" >/dev/null || { echo "FEHLER: $1 -> override ist keine Kopie"; exit 1; }
  grep -q 'AEQUITAS_QS_MESSQUELLE: /data/quanten' "$D/val/docker-compose.override.yml" || { echo "FEHLER: $1 -> Messquelle fehlt"; exit 1; }
  [ ! -e "$D/val/docker-compose.override.yml.neu" ] || { echo "FEHLER: $1 -> .neu liegt noch"; exit 1; }
}
verz_ok() {
  [ -d "$D/srv/q" ] && [ ! -L "$D/srv/q" ] || { echo "FEHLER: $1 -> kein Verzeichnis"; exit 1; }
  [ "$(stat -c %a "$D/srv/q")" = 700 ] || { echo "FEHLER: $1 -> nicht 0700"; exit 1; }
  if [ "$(id -u)" = 0 ]; then [ "$(stat -c %u "$D/srv/q")" = "$ICH" ] || { echo "FEHLER: $1 -> Besitzer"; exit 1; }
  else grep -qx "$ICH:$ICH -- $D/srv/q" "$D/chown" || { echo "FEHLER: $1 -> chown fehlt"; exit 1; }; fi
}

neu; lauf ok "frisch: legt Verzeichnis und override an"; verz_ok "frisch"; override_ok "frisch"
lauf ok "zweimal (gleich)"; verz_ok "zweimal"; override_ok "zweimal"
# Altes eigenes override (andere Fassung von quanten.yml): wird ersetzt.
printf '%s\nalt: ja\n' "$MARKE" > "$D/val/docker-compose.override.yml"; lauf ok "eigenes altes override"; override_ok "eigenes altes override"
# Verzeichnis mit falschen Rechten: wird gerichtet.
chmod 755 "$D/srv/q"; lauf ok "Rechte 0755 werden 0700"; verz_ok "Rechte gerichtet"
neu; printf 'QS_VERZEICHNIS=%s\n' "$D/srv/q" >> "$D/val/.env"; lauf ok "QS_VERZEICHNIS in .env gleich"
neu; rm -rf "$D/srv"; lauf ok "Elternverzeichnis fehlt"; verz_ok "Elternverzeichnis fehlt"

# Abbrueche: nichts ueberschreiben
neu; printf 'services: {}\n' > "$D/val/docker-compose.override.yml"; lauf halt "fremdes override"
[ "$(cat "$D/val/docker-compose.override.yml")" = 'services: {}' ] || { echo "FEHLER: fremdes override veraendert"; exit 1; }
neu; printf '%s\n' "$MARKE" > "$D/echt.yml"; ln -s "$D/echt.yml" "$D/val/docker-compose.override.yml"; lauf halt "override als Verweis"
[ "$(cat "$D/echt.yml")" = "$MARKE" ] || { echo "FEHLER: Ziel des Verweises veraendert"; exit 1; }
neu; mkdir "$D/val/docker-compose.override.yml"; lauf halt "override ist ein Verzeichnis"
neu; mkdir "$D/echtq"; ln -s "$D/echtq" "$D/srv/q"; lauf halt "Messverzeichnis als Verweis"
[ ! -e "$D/val/docker-compose.override.yml" ] || { echo "FEHLER: override trotz Abbruch"; exit 1; }
neu; printf 'x' > "$D/srv/q"; lauf halt "Messverzeichnis ist eine Datei"
neu; printf 'QS_VERZEICHNIS=/anders\n' >> "$D/val/.env"; lauf halt "QS_VERZEICHNIS in .env anders"
[ ! -e "$D/srv/q" ] || { echo "FEHLER: Verzeichnis trotz Abbruch angelegt"; exit 1; }
neu; rm -f "$D/val/.env"; lauf halt "keine .env"
neu; rm -f "$D/val/docker-compose.quanten.yml"; lauf halt "keine docker-compose.quanten.yml"
for u in 0 '' x '10001;id' -1; do neu; lauf halt "QS_UID '$u'" QS_UID="$u"; done
echo "alle Faelle ok"
