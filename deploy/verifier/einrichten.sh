#!/usr/bin/env bash
# Aequitas-Verifier einrichten -- ein Befehl, eine Frage.
#
#   cd Aequitas/deploy/verifier && bash einrichten.sh
#
# Ein Verifier (Vergleichsdienst) prueft bei jeder Registrierung, ob das
# Gesicht schon eingeschrieben ist. Was das Skript tut
# (docs/VERIFIER_EINRICHTEN.md):
#   1. prueft Docker, Compose, openssl und freie Ports 80/443,
#   2. fragt nach deiner Wallet-Adresse (der in der App registrierten) und
#      ermittelt die oeffentliche IP; die HTTPS-Adresse ist dann
#      https://<IP-mit-Strichen>.sslip.io -- keine eigene Domain noetig,
#   3. erzeugt ALLE Schluessel selbst, auf diesem Server (nie von jemand
#      anderem uebernehmen): Vorlagen-Schluessel, eigene Projektion,
#      Ed25519-Bezeugungsschluessel, internes Token -- .env nur fuer root,
#   4. holt die Liste der anerkannten Coordinatoren vom Netz,
#   5. startet Verifier + HTTPS (Caddy) und prueft beides,
#   6. zeigt die drei Zeilen fuer den Betreiber, der neue Verifier in die
#      Pruefung aufnimmt.
#
# Deine Wallet und ihr privater Schluessel bleiben bei dir; auf dem Server
# liegt nur ihre ADRESSE. Erneut aufrufen ist sicher: eine vorhandene .env
# bleibt, wie sie ist.
set -euo pipefail
cd "$(dirname "$0")"

# Das Image, das auch C1 fuer den Vergleichsdienst nutzt (festgelegt, nicht
# "latest": jeder Verifier soll dasselbe pruefen). Wird mit dem Repo
# aktualisiert.
VERIFIER_IMAGE_STANDARD="ghcr.io/hanoi96international-gif/aequitas-biometric-beta/matching:a9fc9815648f960d87b4641bbd50e412a8c0dcb1"
# Liste der anerkannten Coordinatoren: dort, wo das Netz sie selbst zeigt.
COORD_QUELLE="${AEQUITAS_COORD_QUELLE:-https://proof1.aequitas.digital/matching/health}"
NETZ="${AEQUITAS_NETZ:-https://aequitas.digital}"

rot() { printf '\n\033[31m%s\033[0m\n' "$*" >&2; }
gruen() { printf '\033[32m%s\033[0m\n' "$*"; }
schritt() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
setze() { # setze NAME WERT
  local tmp; tmp="$(mktemp)"
  grep -vE "^$1=" .env > "$tmp" || true
  printf '%s=%s\n' "$1" "$2" >> "$tmp"
  install -m 600 "$tmp" .env; rm -f "$tmp"
}
wert() { grep -E "^$1=" .env | head -1 | cut -d= -f2-; }
hex() { od -An -tx1 | tr -d ' \n'; }

schritt "1/6 Voraussetzungen / Requirements"
command -v docker >/dev/null || { rot "Docker fehlt / missing: curl -fsSL https://get.docker.com | sh"; exit 1; }
docker compose version >/dev/null 2>&1 || { rot "docker compose fehlt / missing (mit get.docker.com ist es dabei)"; exit 1; }
command -v openssl >/dev/null || { rot "openssl fehlt / missing: apt-get install -y openssl"; exit 1; }
command -v curl >/dev/null || { rot "curl fehlt / missing: apt-get install -y curl"; exit 1; }
if [ ! -f .env ] && command -v ss >/dev/null; then
  for p in 80 443; do
    if ss -ltn "( sport = :$p )" | grep -q LISTEN; then
      rot "Port $p ist belegt / in use. Laeuft schon ein Webserver? Der Verifier braucht 80 und 443."; exit 1
    fi
  done
fi
SPEICHER_MB="$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo 2>/dev/null || echo 0)"
[ "${SPEICHER_MB:-0}" -ge 3500 ] || echo "  Hinweis / note: ${SPEICHER_MB} MB RAM -- empfohlen sind 4 GB / 4 GB recommended."
gruen "Docker, Compose, openssl: vorhanden / present"

if [ -f .env ]; then
  gruen ".env gibt es schon -- sie bleibt, wie sie ist / kept as is."
else
  schritt "2/6 Eine Angabe / One question"
  WALLET=""
  while ! [[ "$WALLET" =~ ^0x[0-9a-fA-F]{40}$ ]]; do
    read -r -p "Deine Wallet-Adresse aus der App / Your wallet address from the app (0x..., 42): " WALLET
    [[ "$WALLET" =~ ^0x[0-9a-fA-F]{40}$ ]] || echo "  Keine Adresse / Not an address: 0x + 40 Zeichen/characters (0-9, a-f)."
  done
  WALLET="$(printf '%s' "$WALLET" | tr 'A-F' 'a-f')"
  IP="$(curl -4 -fsS -m 10 https://api.ipify.org 2>/dev/null || true)"
  if ! [[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    read -r -p "Oeffentliche IPv4 dieses Servers / Public IPv4 of this server: " IP
  fi
  read -r -p "Oeffentliche IP / Public IP: $IP -- stimmt das? / correct? [J/n, Y/n] " OK
  case "${OK:-j}" in n|N) read -r -p "Richtige IPv4 / Correct IPv4: " IP ;; esac
  [[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { rot "Keine gueltige IPv4 / not a valid IPv4: $IP"; exit 1; }
  HOST="$(printf '%s' "$IP" | tr '.' '-').sslip.io"

  schritt "3/6 Schluessel erzeugen / Generating keys"
  # Alles hier auf diesem Server. Zwei Verifier mit demselben Geheimnis
  # zaehlen als einer -- deshalb nie Schluessel von jemand anderem nehmen.
  umask 077
  TMPK="$(mktemp)"
  trap 'rm -f "$TMPK"' EXIT
  openssl genpkey -algorithm ed25519 -out "$TMPK" 2>/dev/null
  # PKCS#8 fuer Ed25519: die letzten 32 Byte sind der rohe private Schluessel.
  SIGNIER="$(openssl pkey -in "$TMPK" -outform DER 2>/dev/null | tail -c 32 | hex)"
  rm -f "$TMPK"
  [[ "$SIGNIER" =~ ^[0-9a-f]{64}$ ]] || { rot "Schluessel konnte nicht erzeugt werden / key generation failed"; exit 1; }

  schritt "4/6 Anerkannte Coordinatoren / Recognised coordinators"
  KEYS="$(curl -fsS -m 15 "$COORD_QUELLE" 2>/dev/null | grep -oE '"coordinator_keys": ?\[[^]]*\]' | grep -oE '[0-9a-f]{64}' | paste -sd, - || true)"
  [ -n "$KEYS" ] || { rot "Liste der Coordinatoren nicht lesbar / coordinator list unreadable ($COORD_QUELLE). Spaeter erneut / retry later."; exit 1; }
  echo "  $(printf '%s\n' "$KEYS" | tr ',' '\n' | grep -c .) Coordinatoren / coordinators"

  cat > .env <<EOF
# Geschrieben von einrichten.sh -- nur fuer root lesbar. Enthaelt Geheimnisse.
VERIFIER_IMAGE=$VERIFIER_IMAGE_STANDARD
VERIFIER_HOST=$HOST
OPERATOR_WALLET=$WALLET
PORT=8098
POH_DB_PATH=/data/poh.db
TEMPLATE_ENCRYPTION_KEY=$(openssl rand -base64 32)
FACE_SKETCH_SEED=$(openssl rand -hex 32)
VALIDATOR_SIGNING_KEY=$SIGNIER
SERVICE_AUTH_TOKEN=$(openssl rand -hex 32)
REQUIRE_COORDINATOR_AUTH=true
COORDINATOR_PUBLIC_KEYS=$KEYS
CHAIN_BASE_URL=$NETZ
ALLOW_REAL_BIOMETRIC_DATA=false
TORCH_NUM_THREADS=$(nproc 2>/dev/null || echo 2)
EOF
  chmod 600 .env
  unset SIGNIER
  gruen ".env geschrieben (nur fuer root lesbar) / written (root only)"
fi

HOST="$(wert VERIFIER_HOST)"
WALLET="$(wert OPERATOR_WALLET)"
[ -n "$HOST" ] && [ -n "$WALLET" ] || { rot ".env unvollstaendig (VERIFIER_HOST/OPERATOR_WALLET) / incomplete"; exit 1; }
printf '%s {\n\treverse_proxy aequitas-verifier:8098\n}\n' "$HOST" > Caddyfile

schritt "5/6 Starten (erster Download etwa 5 GB) / Starting (first download ~5 GB)"
if ! docker compose pull -q verifier; then
  rot "Das Verifier-Image ist nicht abrufbar / image not available.
Ist es schon oeffentlich? / Is it public yet? Bitte im Telegram melden / please report in Telegram."
  exit 1
fi
docker compose up -d
bereit=0
for i in $(seq 1 90); do
  if docker compose exec -T verifier python -c "import urllib.request;urllib.request.urlopen('http://127.0.0.1:8098/health',timeout=3)" >/dev/null 2>&1; then
    bereit=1; break
  fi
  sleep 5
done
[ "$bereit" = 1 ] || { rot "Der Verifier antwortet nicht / not answering. Log: docker compose logs --tail 50 verifier"; exit 1; }
gruen "Verifier laeuft / running"
echo "  Warte auf das HTTPS-Zertifikat / waiting for the HTTPS certificate ..."
https_ok=0
for i in $(seq 1 36); do
  if curl -fsS -m 10 "https://$HOST/health" >/dev/null 2>&1; then https_ok=1; break; fi
  sleep 5
done
if [ "$https_ok" = 1 ]; then
  gruen "HTTPS: https://$HOST"
else
  rot "HTTPS noch nicht erreichbar / not reachable yet. Sind Port 80 und 443 beim Anbieter offen? / Are ports 80 and 443 open at your provider?
Log: docker compose logs --tail 50 caddy"
  exit 1
fi

schritt "6/6 Fuer den Betreiber / For the operator"
N="$(curl -fsS -m 15 "https://$HOST/bezeugungsnachweis?wallet=$WALLET" 2>/dev/null || true)"
PKEY="$(printf '%s' "$N" | grep -oE '"personhood_key": ?"[0-9a-f]{64}"' | grep -oE '[0-9a-f]{64}' || true)"
PSIG="$(printf '%s' "$N" | grep -oE '"personhood_signature": ?"[^"]+"' | cut -d'"' -f4 || true)"
[ -n "$PKEY" ] && [ -n "$PSIG" ] || { rot "Kein Bezeugungsnachweis / no witness proof. Log: docker compose logs --tail 50 verifier"; exit 1; }
gruen "Fertig / Done."
cat <<TEXT

Schick diese drei Zeilen in die Telegram-Gruppe (aequitas.digital). Der
Betreiber prueft sie und nimmt deinen Verifier in die Pruefung auf. Nichts
davon ist geheim.
Send these three lines to the Telegram group. The operator checks them and
admits your verifier. None of it is secret.

    Verifier: https://$HOST
    Wallet:   $WALLET
    Schluessel/key: $PKEY

Nuetzlich / Useful:
    docker compose logs -f verifier          Log (Strg/Ctrl+C beendet nur die Anzeige)
    curl -s https://$HOST/health             Zustand / status

Sichere die Datei .env (zum Beispiel / e.g. scp root@$(printf '%s' "$HOST" | sed 's/\.sslip\.io$//; s/-/./g'):Aequitas/deploy/verifier/.env .).
Geht sie verloren, ist dein Verifier ein neuer. / If it is lost, your verifier is a new one.
TEXT
