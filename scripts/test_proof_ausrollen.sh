#!/usr/bin/env bash
# Missbrauchstest fuer deploy/proof-server/proof-ausrollen.sh: das Skript
# rollt nur ein Image aus GHCR mit festem Pfad und 40-stelligem SHA aus, baut
# auf der Box nie, und nimmt zurueck, wenn der Dienst danach nicht antwortet.
# Ohne IMAGE behaelt es das laufende Image und wendet nur die Umgebung neu an.
#
# Laeuft gegen ein Wegwerf-Verzeichnis: /root/ ist in einer Kopie ersetzt,
# docker ist eine Attrappe, die Container als Dateien fuehrt.
set -uo pipefail
cd "$(dirname "$0")/.."
T="$(mktemp -d)"
[ -n "$T" ] && [ -d "$T" ] || { echo "FEHLER: kein Wegwerf-Verzeichnis"; exit 1; }
trap 'rm -rf "$T"' EXIT
fehler=0
mkdir -p "$T/bin" "$T/root" "$T/st"
S="$T/ausrollen.sh"
sed -e "s#/root/#$T/root/#g" deploy/proof-server/proof-ausrollen.sh > "$S"
! grep -v '^\s*#' "$S" | sed "s#$T/root/##g" | grep -q '/root/' || { echo "FEHLER: /root/ nicht ersetzt"; exit 1; }

# Attrappe: Zustand in $ST, Gesundheit haengt am Image (kaputt-Marke).
cat > "$T/bin/docker" <<'DOCKERATTRAPPE'
#!/bin/bash
ST="${ATTRAPPE_ST:?}"
case "$1" in
  inspect)
    [ -f "$ST/image" ] || exit 1
    case "$*" in
      *Config.Image*) cat "$ST/image" ;;
      *Config.Env*) cat "$ST/env" ;;
    esac ;;
  pull) echo "pull $3" >> "$ST/log"; [ -f "$ST/pull_fehler" ] && exit 1; exit 0 ;;
  build) echo "build" >> "$ST/log"; exit 0 ;;
  rm) exit 0 ;;
  run)
    envf=""; img=""
    while [ $# -gt 0 ]; do
      case "$1" in
        --env-file) envf="$2"; shift ;;
        --network|--name|--restart|--log-opt) shift ;;
        -d|run) ;;
        *) img="$1" ;;
      esac
      shift
    done
    grep -v '^PATH=' "$envf" > "$ST/env"; printf '%s\n' "$img" > "$ST/image"; echo "run $img" >> "$ST/log" ;;
  exec)
    img=$(cat "$ST/image"); modus=$(grep '^ERWARTETER_BIO_MODUS=' "$ST/env" | cut -d= -f2)
    # Ein Dienst, der einen anderen Modus meldet als verlangt (dienst_modus).
    if [ -f "$ST/dienst_modus" ] && [ "$img" != aequitas-proof-server:alt ]; then modus=$(cat "$ST/dienst_modus"); fi
    if [ -f "$ST/kaputt" ] && [ "$img" = "$(cat "$ST/kaputt")" ]; then exit 1; fi
    echo "{\"status\":\"ok\",\"erwarteter_modus\":\"${modus:-real}\"}" ;;
esac
DOCKERATTRAPPE
chmod +x "$T/bin/docker"
export ATTRAPPE_ST="$T/st"

SHA=0123456789abcdef0123456789abcdef01234567
GUT="ghcr.io/hanoi96international-gif/aequitas-proof-server/proof-server:$SHA"
start() {
  find "$T/st" "$T/root" -mindepth 1 -delete
  printf 'aequitas-proof-server:alt\n' > "$T/st/image"
  printf 'DATABASE_URL=d\nCHAIN_SERVICE_TOKEN=t\nBIO_ATTESTATION_MODE=required\nCOORDINATOR_PUBLIC_KEYS=alt\nERWARTETER_BIO_MODUS=test\nPATH=/x\n' > "$T/st/env"
}
lauf() { PATH="$T/bin:$PATH" IMAGE="$1" ERWARTETER_BIO_MODUS="${2:-}" bash "$S" >"$T/out" 2>&1; echo $?; }
bild() { cat "$T/st/image"; }
pruef() {
  if [ "$2" = 0 ]; then echo "ok   $1"; else echo "FEHL $1"; sed 's/^/     /' "$T/out" | tail -5; fehler=1; fi
}

for boese in "ghcr.io/fremd/proof-server:$SHA" "$GUT;rm -rf /" \
             "ghcr.io/hanoi96international-gif/aequitas-proof-server/proof-server:latest" \
             "ghcr.io/hanoi96international-gif/aequitas-proof-server/proof-server:${SHA:0:39}" \
             "docker.io/hanoi96international-gif/aequitas-proof-server/proof-server:$SHA"; do
  start; rc=$(lauf "$boese" test)
  [ "$rc" != 0 ] && [ "$(bild)" = aequitas-proof-server:alt ] && [ ! -f "$T/st/log" ]; pruef "abgewiesen, nichts geaendert: $boese" $?
done

start; rc=$(lauf "$GUT" test)
[ "$rc" = 0 ] && [ "$(bild)" = "$GUT" ] && ! grep -q build "$T/st/log" && grep -q "pull $GUT" "$T/st/log"
pruef "Gutfall: Image gezogen und laeuft, nie gebaut" $?
grep -q '^DATABASE_URL=d$' "$T/st/env" && ! grep -q '^PATH=' "$T/st/env"
pruef "Gutfall: Umgebung uebernommen, PATH nicht" $?

start; printf 'COORDINATOR_PUBLIC_KEYS=neu\n' > "$T/root/proof-server.env.schluessel"; rc=$(lauf "" "")
[ "$rc" = 0 ] && [ "$(bild)" = aequitas-proof-server:alt ] && grep -q '^COORDINATOR_PUBLIC_KEYS=neu$' "$T/st/env" \
  && [ "$(grep -c '^COORDINATOR_PUBLIC_KEYS=' "$T/st/env")" = 1 ] && ! grep -qE 'pull|build' "$T/st/log"
pruef "ohne IMAGE: laufendes Image bleibt, Schluesselliste angewendet" $?

start; touch "$T/st/pull_fehler"; rc=$(lauf "$GUT" test)
[ "$rc" != 0 ] && [ "$(bild)" = aequitas-proof-server:alt ] && ! grep -q '^run' "$T/st/log"
pruef "Ziehen scheitert: nichts geaendert" $?

start; printf '%s\n' "$GUT" > "$T/st/kaputt"; rc=$(lauf "$GUT" test)
[ "$rc" != 0 ] && [ "$(bild)" = aequitas-proof-server:alt ]
pruef "antwortet nicht: altes Image wieder aktiv" $?

start; printf 'test\n' > "$T/st/dienst_modus"; rc=$(lauf "$GUT" real)
[ "$rc" != 0 ] && [ "$(bild)" = aequitas-proof-server:alt ]
pruef "falscher Modus nach dem Start: Ruecknahme" $?

[ "$fehler" = 0 ] && echo "proof-ausrollen: alle Faelle gruen" || { echo "proof-ausrollen: FEHLER"; exit 1; }
