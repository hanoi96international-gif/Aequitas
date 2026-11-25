#!/usr/bin/env bash
# Gut- und Missbrauchsfaelle fuer deploy/knoten/einrichten.sh, ohne Docker:
# eine Kopie des Skripts mit Attrappen fuer die beiden Teile (sie zeigen nur,
# was sie bekommen) und fuer nproc/df/curl.
#   - zu schwach (Kerne, RAM, Platz): abgelehnt, BEVOR gefragt wird; kein Teil laeuft
#   - Wallet/IP ungueltig: nie an einen Teil weitergegeben
#   - gut: Validator bekommt Wallet als Argument + AEQUITAS_IP, Verifier
#     AEQUITAS_WALLET + AEQUITAS_IP; Verifier erst, wenn der Validator fertig ist
set -euo pipefail
cd "$(dirname "$0")/.."
QUELLE="$PWD/deploy/knoten/einrichten.sh"
D="$(mktemp -d)"; trap 'rm -rf "$D"' EXIT
mkdir -p "$D/deploy/knoten" "$D/deploy/validator" "$D/deploy/verifier" "$D/bin"
cp "$QUELLE" "$D/deploy/knoten/einrichten.sh"
cat > "$D/deploy/validator/einrichten.sh" <<'EOF'
echo "VALIDATOR arg=$1 ip=${AEQUITAS_IP:-} wallet_env=${AEQUITAS_WALLET:-}" >> "$PROTOKOLL"
[ "${VALIDATOR_SCHEITERT:-0}" = 1 ] && exit 3
exit 0
EOF
cat > "$D/deploy/verifier/einrichten.sh" <<'EOF'
echo "VERIFIER arg=${1:-} ip=${AEQUITAS_IP:-} wallet_env=${AEQUITAS_WALLET:-}" >> "$PROTOKOLL"
exit 0
EOF
printf '#!/bin/sh\necho "${KERNE:-4}"\n' > "$D/bin/nproc"
printf '#!/bin/sh\nprintf "Filesystem 1024-blocks Used Available Capacity Mounted\\n/dev/x 1 1 %%s 1pct /\\n" "$((${PLATZ:-100} * 1048576))"\n' > "$D/bin/df"
printf '#!/bin/sh\necho "${IPIFY:-203.0.113.10}"\n' > "$D/bin/curl"
chmod +x "$D/bin/"*
mem() { printf 'MemTotal:       %s kB\n' "$(( $1 * 1024 * 1024 ))" > "$D/meminfo"; }
W=0x000000000000000000000000000000000000dEaD

lauf() { # erwartung eingabe [env...]
  local erw="$1" ein="$2"; shift 2
  : > "$D/protokoll"
  local rc=0
  printf '%b' "$ein" | env PATH="$D/bin:$PATH" PROTOKOLL="$D/protokoll" AEQUITAS_MEMINFO="$D/meminfo" "$@" \
    bash "$D/deploy/knoten/einrichten.sh" ${ARG:-} > "$D/ausgabe" 2>&1 || rc=$?
  if [ "$erw" = ok ] && [ "$rc" != 0 ]; then echo "FEHLER: erwartet ok, rc=$rc"; sed -e 's/\x1b\[[0-9;]*m//g' "$D/ausgabe"; exit 1; fi
  if [ "$erw" = halt ] && [ "$rc" = 0 ]; then echo "FEHLER: erwartet halt"; cat "$D/ausgabe"; exit 1; fi
}

# Zu schwach: abgelehnt, ohne Frage, ohne Teil.
for fall in "KERNE=2" "RAM=8" "PLATZ=50"; do
  mem 16
  case "$fall" in RAM=*) mem "${fall#RAM=}" ;; esac
  e=(KERNE=8 PLATZ=200); case "$fall" in KERNE=*|PLATZ=*) e+=("$fall") ;; esac
  lauf halt "" "${e[@]}"
  grep -q 'zu schwach' "$D/ausgabe" || { echo "FEHLER ($fall): keine Ablehnung"; exit 1; }
  ! grep -q 'Wallet-Adresse' "$D/ausgabe" || { echo "FEHLER ($fall): vor der Ablehnung gefragt"; exit 1; }
  [ ! -s "$D/protokoll" ] || { echo "FEHLER ($fall): ein Teil lief"; exit 1; }
  echo "ok: $fall -> abgelehnt, nichts gefragt, kein Teil"
done

# Gut: einmal gefragt, beide Teile mit genau diesen Angaben, Validator zuerst.
mem 16
lauf ok "$W\n\n" KERNE=8 PLATZ=200
[ "$(sed -n 1p "$D/protokoll")" = "VALIDATOR arg=$W ip=203.0.113.10 wallet_env=" ] || { echo "FEHLER: Validator bekam"; cat "$D/protokoll"; exit 1; }
[ "$(sed -n 2p "$D/protokoll")" = "VERIFIER arg= ip=203.0.113.10 wallet_env=$W" ] || { echo "FEHLER: Verifier bekam"; cat "$D/protokoll"; exit 1; }
# Genau zwei Eingaben (Wallet, IP bestaetigen) reichen: fragte ein Teil noch
# einmal, liefe read ins Leere und das Skript braeche ab (set -e).
echo "ok: gut -> Validator, dann Verifier, einmal gefragt"

# Wallet als Argument: keine Frage nach der Wallet.
# Nur die IP-Bestaetigung als Eingabe: wuerde trotz Argument nach der Wallet
# gefragt, waere die Eingabe danach leer und das Skript braeche ab.
ARG="$W" lauf ok "\n" KERNE=8 PLATZ=200
grep -q "VALIDATOR arg=$W" "$D/protokoll" || { echo "FEHLER: Argument nicht weitergegeben"; exit 1; }
echo "ok: Wallet als Argument, nicht gefragt"

# Ungueltige Wallet: erneut gefragt, die ungueltige erreicht nie einen Teil.
lauf ok "0x123\n$W\n\n" KERNE=8 PLATZ=200
! grep -q '0x123' "$D/protokoll" || { echo "FEHLER: ungueltige Wallet weitergegeben"; exit 1; }
ARG="0xboese;rm" lauf ok "$W\n\n" KERNE=8 PLATZ=200
! grep -q 'boese' "$D/protokoll" || { echo "FEHLER: ungueltiges Argument weitergegeben"; exit 1; }
echo "ok: ungueltige Wallet nie weitergegeben"

# IP: falsch erkannt, korrigiert auf etwas Ungueltiges -> halt, kein Teil.
lauf halt "$W\nn\n1.2.3\n" KERNE=8 PLATZ=200
[ ! -s "$D/protokoll" ] || { echo "FEHLER: ungueltige IP weitergegeben"; cat "$D/protokoll"; exit 1; }
lauf halt "$W\nn\n1.2.3.4;id\n" KERNE=8 PLATZ=200
[ ! -s "$D/protokoll" ] || { echo "FEHLER: IP mit Befehl weitergegeben"; exit 1; }
echo "ok: ungueltige IP -> halt, kein Teil"

# Validator scheitert: der Verifier laeuft nicht, Abbruch.
lauf halt "$W\n\n" KERNE=8 PLATZ=200 VALIDATOR_SCHEITERT=1
! grep -q VERIFIER "$D/protokoll" || { echo "FEHLER: Verifier trotz gescheitertem Validator"; exit 1; }
echo "ok: Validator scheitert -> kein Verifier"

# Schon eingerichtet (beide .env): keine Ausstattungs-Ablehnung (Aktualisieren).
touch "$D/deploy/validator/.env" "$D/deploy/verifier/.env"; mem 4
lauf ok "$W\n\n" KERNE=2 PLATZ=10
echo "ok: vorhandene Einrichtung -> aktualisieren ohne Ablehnung"
echo "alle Faelle gruen"
