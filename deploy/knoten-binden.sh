#!/usr/bin/env bash
# Einen BESTEHENDEN Knoten (C1) mit der Wallet eines Menschen verbinden --
# derselbe Schritt wie "6/6" in deploy/validator/einrichten.sh, fuer Knoten,
# die nicht mit dem Skript eingerichtet wurden und darum nie einen QR-Code
# gezeigt haben. Laeuft ueber .github/workflows/knoten-binden.yml.
#
#   PHASE=link    [WALLET=0x..] gibt LINK=aequitasapp://knoten-binden?... aus;
#                              ohne WALLET die eingetragene Betreiber-Wallet
#   PHASE=setzen  WALLET=0x..  holt die Bindung SELBST beim Netz ab, prueft,
#                              dass sie genau diese Wallet und diese
#                              Signieradresse nennt, traegt sie ein und startet
#                              den Knoten neu (Sicherung .env.bak.<zeit>).
#
# Nichts davon ist geheim: Signieradresse, Wallet und die beiden Signaturen
# stehen ohnehin oeffentlich beim Netz. Keine Schluessel verlassen die Box.
set -euo pipefail
cd /root/Aequitas/deploy/validator
NETZ="${AEQUITAS_NETZ:-https://aequitas.digital}"

# Ohne WALLET gilt die eingetragene Betreiber-Wallet (NODE_OPERATOR_WALLET):
# fuer genau die bestaetigt der Knoten seinen Schluessel
# (validator_selfservice.go). Ein Laie muss nichts eingeben.
ALT="$(grep -E '^NODE_OPERATOR_WALLET=' .env | cut -d= -f2- | tr 'A-F' 'a-f' || true)"
WALLET="${WALLET:-$ALT}"
[[ "${WALLET:-}" =~ ^0x[0-9a-fA-F]{40}$ ]] || { echo "ABBRUCH: keine Wallet -- weder eingegeben noch als NODE_OPERATOR_WALLET eingetragen"; exit 1; }
WALLET="$(printf '%s' "$WALLET" | tr 'A-F' 'a-f')"

setze() {
  local tmp; tmp="$(mktemp)"
  grep -vE "^$1=" .env > "$tmp" || true
  printf '%s=%s\n' "$1" "$2" >> "$tmp"
  install -m 600 "$tmp" .env; rm -f "$tmp"
}
neustart() {
  docker compose up -d --no-deps node
  for i in $(seq 1 60); do
    H="$(curl -fsS -m 5 http://127.0.0.1:8080/api/status 2>/dev/null | grep -oE '"height": ?[0-9]+' | grep -oE '[0-9]+' || true)"
    [ -n "$H" ] && { echo "Knoten laeuft wieder, Hoehe $H"; return 0; }
    sleep 5
  done
  echo "ABBRUCH: Knoten antwortet nach dem Neustart nicht -- Rueckweg: cp .env.bak.<zeit> .env && docker compose up -d --no-deps node"
  exit 1
}

S="$(curl -fsS -m 10 http://127.0.0.1:8080/api/status || true)"
ADDR="$(printf '%s' "$S" | grep -oE '"validator_address": ?"0x[0-9a-fA-F]{40}"' | grep -oE '0x[0-9a-fA-F]{40}' | tr 'A-F' 'a-f' || true)"
[ -n "$ADDR" ] || { echo "ABBRUCH: Knoten nennt keine Signieradresse (/api/status)"; exit 1; }

case "${PHASE:-}" in
link)
  echo "Signieradresse: $ADDR"
  echo "Wallet bisher:  ${ALT:-(keine)}"
  echo "Wallet jetzt:   $WALLET"
  if [ "$WALLET" != "$ALT" ]; then
    # Andere Wallet eingegeben: erst eintragen -- der Knoten bestaetigt
    # seinen Schluessel nur fuer NODE_OPERATOR_WALLET. Die alte Bindung
    # gehoert zur alten Wallet und faellt weg.
    cp -a .env ".env.bak.$(date +%s)"
    setze NODE_OPERATOR_WALLET "$WALLET"
    grep -vE '^NODE_OPERATOR_BINDING_SIGNATURE=' .env > .env.tmp || true
    install -m 600 .env.tmp .env; rm -f .env.tmp
    echo "Betreiber-Wallet geaendert, Neustart des Knotens ..."
    neustart
  fi
  if grep -qE '^NODE_OPERATOR_BINDING_SIGNATURE=.+' .env; then echo "Bindung bisher: eingetragen"; else echo "Bindung bisher: keine"; fi
  P="$(curl -fsS -m 10 "http://127.0.0.1:8080/api/validator-selfproof?wallet=$WALLET" || true)"
  BEWEIS="$(printf '%s' "$P" | grep -oE '"signing_key_signature": ?"0x[0-9a-f]{130}"' | grep -oE '0x[0-9a-f]{130}' || true)"
  [ -n "$BEWEIS" ] || { echo "ABBRUCH: Knoten liefert keinen Schluesselnachweis"; exit 1; }
  echo "ADRESSE=$ADDR"
  echo "LINK=aequitasapp://knoten-binden?adresse=$ADDR&wallet=$WALLET&beweis=$BEWEIS"
  ;;
setzen)
  B="$(curl -fsS -m 10 "$NETZ/api/validator-binding?signing_address=$ADDR" || true)"
  W="$(printf '%s' "$B" | grep -oE '"human_wallet": ?"0x[0-9a-fA-F]{40}"' | grep -oE '0x[0-9a-fA-F]{40}' | tr 'A-F' 'a-f' || true)"
  SIG="$(printf '%s' "$B" | grep -oE '"human_signature": ?"0x[0-9a-fA-F]{130}"' | grep -oE '0x[0-9a-fA-F]{130}' || true)"
  # Fail closed: nur eine Bindung, die genau diese Wallet nennt.
  [ -n "$SIG" ] && [ "$W" = "$WALLET" ] || { echo "ABBRUCH: keine Bindung fuer $WALLET beim Netz (gefunden: ${W:-keine})"; exit 1; }
  cp -a .env ".env.bak.$(date +%s)"
  setze NODE_OPERATOR_WALLET "$WALLET"
  setze NODE_OPERATOR_BINDING_SIGNATURE "$SIG"
  unset SIG B
  echo "Eingetragen. Neustart des Knotens ..."
  neustart
  ;;
*)
  echo "ABBRUCH: PHASE=link oder PHASE=setzen"; exit 1 ;;
esac
