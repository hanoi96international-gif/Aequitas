#!/usr/bin/env bash
# Aequitas-Validator einrichten -- ein Befehl, zwei Fragen.
#
#   cd Aequitas/deploy/validator && bash einrichten.sh
#
# Was das Skript tut (docs/VALIDATOR_EINRICHTEN.md):
#   1. prueft Docker, Compose und git,
#   2. fragt nach deiner Wallet-Adresse (die in der App registrierte) und
#      ermittelt die oeffentliche IP dieses Servers,
#   3. schreibt .env (nur fuer root lesbar) mit einem zufaelligen
#      Datenbank-Passwort,
#   4. baut und startet den Knoten; der Knoten erzeugt beim ersten Start
#      seinen EIGENEN Signier- und P2P-Schluessel, das Skript traegt beide
#      dauerhaft in .env ein und startet neu,
#   5. zeigt die Signieradresse und die zwei Dinge, die danach noch zu tun
#      sind (Bindung, Aufnahme als Produzent).
#
# Deine Wallet und ihr privater Schluessel bleiben bei dir, auf dem Server
# liegt nur ihre ADRESSE. Wer den Server knackt, bekommt nicht dein Geld.
#
# Erneut aufrufen ist sicher: eine vorhandene .env wird nicht ueberschrieben.
set -euo pipefail
cd "$(dirname "$0")"

rot() { printf '\n\033[31m%s\033[0m\n' "$*" >&2; }
gruen() { printf '\033[32m%s\033[0m\n' "$*"; }
schritt() { printf '\n\033[1m== %s\033[0m\n' "$*"; }

schritt "1/6 Voraussetzungen"
command -v docker >/dev/null || { rot "Docker fehlt. Installieren: curl -fsSL https://get.docker.com | sh"; exit 1; }
docker compose version >/dev/null 2>&1 || { rot "Das Compose-Plugin fehlt (docker compose). Mit get.docker.com ist es dabei."; exit 1; }
command -v git >/dev/null || { rot "git fehlt: apt-get install -y git"; exit 1; }
command -v openssl >/dev/null || { rot "openssl fehlt: apt-get install -y openssl"; exit 1; }
gruen "Docker, Compose, git: vorhanden"

if [ -f .env ]; then
  gruen ".env gibt es schon -- sie bleibt, wie sie ist."
else
  schritt "2/6 Zwei Angaben"
  WALLET=""
  while ! [[ "$WALLET" =~ ^0x[0-9a-fA-F]{40}$ ]]; do
    read -r -p "Deine Wallet-Adresse aus der App / Your wallet address from the app (0x..., 42): " WALLET
    [[ "$WALLET" =~ ^0x[0-9a-fA-F]{40}$ ]] || echo "  Keine Adresse / Not an address: 0x + 40 Zeichen/characters (0-9, a-f)."
  done
  IP="$(curl -4 -fsS -m 10 https://api.ipify.org 2>/dev/null || true)"
  if ! [[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    read -r -p "Oeffentliche IPv4 dieses Servers / Public IPv4 of this server: " IP
  fi
  read -r -p "Oeffentliche IP / Public IP: $IP -- stimmt das? / correct? [J/n, Y/n] " OK
  case "${OK:-j}" in n|N) read -r -p "Richtige IPv4 / Correct IPv4: " IP ;; esac
  [[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { rot "Keine gueltige IPv4: $IP"; exit 1; }

  schritt "3/6 Konfiguration schreiben"
  umask 077
  sed -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$(openssl rand -hex 24)|" \
      -e "s|^SELF_URL=.*|SELF_URL=http://$IP:8080|" \
      -e "s|^NODE_OPERATOR_WALLET=.*|NODE_OPERATOR_WALLET=$WALLET|" \
      .env.example > .env
  chmod 600 .env
  gruen ".env geschrieben (nur fuer root lesbar)"
fi

schritt "4/6 Bauen und starten (dauert beim ersten Mal etwa 10 Minuten)"
GIT_COMMIT="$(git rev-parse --short=7 HEAD 2>/dev/null || echo unknown)" docker compose up -d --build

# Eigene Schluessel des Knotens dauerhaft machen. Ohne das bekaeme der Knoten
# bei jedem Neustart eine neue Identitaet, und das Netz saehe ihn als jemand
# anderen. Der Knoten druckt beide beim ersten Start genau einmal ins Log.
setze() { # setze NAME WERT
  local tmp; tmp="$(mktemp)"
  grep -vE "^$1=" .env > "$tmp" || true
  printf '%s=%s\n' "$1" "$2" >> "$tmp"
  install -m 600 "$tmp" .env; rm -f "$tmp"
}
fehlt() { ! grep -qE "^$1=.+" .env; }
if fehlt RELAYER_PRIVATE_KEY || fehlt NODE_KEY; then
  echo "Warte auf die Schluessel des Knotens ..."
  for i in $(seq 1 60); do
    LOG="$(docker compose logs --no-color node 2>/dev/null || true)"
    RK="$(printf '%s\n' "$LOG" | grep -A1 'SET THIS AS RELAYER_PRIVATE_KEY' | grep -oE '0x[0-9a-f]{64}' | tail -1 || true)"
    NK="$(printf '%s\n' "$LOG" | grep -A1 'SAVE THIS AS NODE_KEY' | tail -1 | grep -oE '[A-Za-z0-9+/=]{40,}' | tail -1 || true)"
    if { ! fehlt RELAYER_PRIVATE_KEY || [ -n "$RK" ]; } && { ! fehlt NODE_KEY || [ -n "$NK" ]; }; then break; fi
    sleep 5
  done
  if fehlt RELAYER_PRIVATE_KEY; then
    [ -n "$RK" ] || { rot "Der Knoten hat keinen Signierschluessel gemeldet. Log ansehen: docker compose logs node"; exit 1; }
    setze RELAYER_PRIVATE_KEY "$RK"
  fi
  if fehlt NODE_KEY; then
    [ -n "$NK" ] || { rot "Der Knoten hat keinen P2P-Schluessel gemeldet. Log ansehen: docker compose logs node"; exit 1; }
    setze NODE_KEY "$NK"
  fi
  unset RK NK LOG
  gruen "Schluessel dauerhaft in .env eingetragen -- Neustart mit fester Identitaet"
  docker compose up -d node
fi

schritt "5/6 Pruefen"
ADDR=""
for i in $(seq 1 60); do
  S="$(curl -fsS -m 5 http://127.0.0.1:8080/api/status 2>/dev/null || true)"
  ADDR="$(printf '%s' "$S" | grep -oE '"validator_address": ?"0x[0-9a-fA-F]{40}"' | grep -oE '0x[0-9a-fA-F]{40}' || true)"
  [ -n "$ADDR" ] && break
  sleep 5
done
[ -n "$ADDR" ] || { rot "Der Knoten antwortet nicht. Log ansehen: docker compose logs -f node"; exit 1; }
ADDR="$(printf '%s' "$ADDR" | tr 'A-F' 'a-f')"
gruen "Der Knoten laeuft. Signieradresse / signing address: $ADDR"

schritt "6/6 Mit deiner Wallet verbinden / Bind to your wallet"
# Ohne diese Bindung nimmt das Netz den Knoten nicht an (handlePeerRegister):
# die Wallet des Menschen muss diese Signieradresse ermaechtigen. Die Wallet
# liegt in der App, nicht hier -- also zeigt das Skript einen QR-Code, die App
# unterschreibt und reicht die Bindung bei $NETZ ein, und das Skript holt sie
# dort ab (GET /api/validator-binding). Nichts davon ist geheim.
NETZ="${AEQUITAS_NETZ:-https://aequitas.digital}"
WALLET_ENV="$(grep -E '^NODE_OPERATOR_WALLET=' .env | cut -d= -f2- | tr 'A-F' 'a-f')"
if ! fehlt NODE_OPERATOR_BINDING_SIGNATURE; then
  gruen "Bindung ist schon eingetragen / already bound."
else
  P="$(curl -fsS -m 10 "http://127.0.0.1:8080/api/validator-selfproof?wallet=$WALLET_ENV" 2>/dev/null || true)"
  BEWEIS="$(printf '%s' "$P" | grep -oE '"signing_key_signature": ?"0x[0-9a-f]{130}"' | grep -oE '0x[0-9a-f]{130}' || true)"
  [ -n "$BEWEIS" ] || { rot "Der Knoten liefert keinen Schluesselnachweis. Log: docker compose logs node"; exit 1; }
  LINK="aequitasapp://knoten-binden?adresse=$ADDR&wallet=$WALLET_ENV&beweis=$BEWEIS"
  if ! command -v qrencode >/dev/null; then
    { apt-get update -qq && apt-get install -y -qq qrencode; } >/dev/null 2>&1 || true
  fi
  cat <<TEXT

  Oeffne die Aequitas-App -> "Knoten" -> "Knoten binden (QR scannen)"
  und scanne diesen Code. Pruefe die Adresse und bestaetige.
  Kostet nichts, bewegt kein Geld.

  Open the Aequitas app -> "Node" -> "Bind node (scan QR)",
  scan this code, check the address and confirm. Free, moves no money.

TEXT
  if command -v qrencode >/dev/null; then
    qrencode -t ansiutf8 -m 2 "$LINK"
  else
    echo "  (qrencode fehlt -- den Code gibt es auch hier / QR also here:)"
  fi
  echo
  echo "  Signieradresse / signing address: $ADDR"
  echo "  Warte auf die Bestaetigung in der App (hoechstens 15 Minuten) ..."
  echo "  Waiting for the confirmation in the app (at most 15 minutes) ..."
  SIG=""
  for i in $(seq 1 180); do
    B="$(curl -fsS -m 10 "$NETZ/api/validator-binding?signing_address=$ADDR" 2>/dev/null || true)"
    W="$(printf '%s' "$B" | grep -oE '"human_wallet": ?"0x[0-9a-f]{40}"' | grep -oE '0x[0-9a-f]{40}' || true)"
    SIG="$(printf '%s' "$B" | grep -oE '"human_signature": ?"0x[0-9a-fA-F]{130}"' | grep -oE '0x[0-9a-fA-F]{130}' || true)"
    if [ -n "$SIG" ] && [ "$W" = "$WALLET_ENV" ]; then break; fi
    SIG=""
    sleep 5
  done
  if [ -z "$SIG" ]; then
    rot "Keine Bestaetigung aus der App angekommen. Einfach noch einmal starten: bash einrichten.sh
No confirmation from the app yet. Just run it again: bash einrichten.sh"
    exit 1
  fi
  setze NODE_OPERATOR_BINDING_SIGNATURE "$SIG"
  unset SIG B
  gruen "Verbunden / bound. Neustart mit Bindung ..."
  docker compose up -d node
fi

gruen "Fertig. Der Knoten holt jetzt das Netz ein / Done. The node is catching up."
cat <<TEXT

Deine Signieradresse / Your signing address (oeffentlich / public):

    $ADDR

Letzter Schritt: AUFNAHME als Blockproduzent.
Solange das Netz nicht jeden Wert eines Blocks selbst nachrechnet, nimmt der
Betreiber neue Blockproduzenten von Hand auf. Schick ihm die Signieradresse
(Telegram-Gruppe auf aequitas.digital). Bis dahin laeuft dein Knoten als
vollwertiger Beobachter mit und prueft jeden Block selbst nach.

Last step: ADMISSION as a block producer. Send the signing address to the
operator (Telegram group on aequitas.digital). Until then your node runs as
a full observer and checks every block itself.

Nuetzlich / Useful:
    docker compose logs -f node      Log (Strg/Ctrl+C beendet nur die Anzeige)
    curl -s localhost:8080/api/wache Selbstpruefung / self-check (200 = ok)
TEXT
