#!/usr/bin/env bash
# Aequitas-Knoten einrichten -- Validator UND Verifier auf einem Server,
# ein Befehl, eine Frage (docs/KNOTEN_EINRICHTEN.md).
#
#   cd Aequitas/deploy/knoten && bash einrichten.sh [0xDEINE-WALLET]
#
# Ein Knotenbetreiber fuehrt die Kette mit (Validator) und schuetzt
# "ein Mensch, ein Konto" (Verifier). Beides auf einem Server war schon immer
# erlaubt; dieses Skript macht es in einem Gang:
#   1. prueft die GEMEINSAME Mindestausstattung (die Summe beider: 4 dedizierte
#      Kerne, 12 GB Arbeitsspeicher, 80 GB frei) -- zu schwach wird abgelehnt,
#      bevor irgendetwas gefragt oder geladen wird,
#   2. fragt EINMAL nach der Wallet-Adresse und laesst die IP bestaetigen,
#   3. richtet den Validator ein (deploy/validator/einrichten.sh: eigener
#      Schluessel, Leistung, Bindung an den Menschen in der App, Probe des Netzes),
#   4. richtet den Verifier ein (deploy/verifier/einrichten.sh: eigene Schluessel,
#      HTTPS, Bezeugungsnachweis).
# Jeder Teil prueft weiter alles selbst (Wallet, IP, Ausstattung, Ports);
# uebergeben werden nur Angaben, die hier schon streng geprueft sind.
#
# Deine Wallet und ihr privater Schluessel bleiben bei dir; auf dem Server liegt
# nur ihre ADRESSE. Erneut aufrufen ist sicher (vorhandene .env bleiben).
set -euo pipefail
cd "$(dirname "$0")"

rot() { printf '\n\033[31m%s\033[0m\n' "$*" >&2; }
gruen() { printf '\033[32m%s\033[0m\n' "$*"; }
schritt() { printf '\n\033[1m== %s\033[0m\n' "$*"; }

# Summe der Mindestwerte beider Teile (Validator 4 Kerne / 8 GB / 40 GB,
# Verifier 2 Kerne / 4 GB / 40 GB auf derselben Platte). Nie niedriger als
# die Teile selbst: jeder Teil prueft danach noch einmal seine eigenen Werte.
MIN_KERNE=4; MIN_RAM_MB=11500; MIN_PLATZ_GB=80
# Nur fuer den Test dieses Skripts (scripts/test_knoten_einrichten.sh): eine
# andere meminfo-Datei. Senkt keine Grenze -- die Teile lesen /proc/meminfo selbst.
MEMINFO="${AEQUITAS_MEMINFO:-/proc/meminfo}"

schritt "1/4 Server pruefen / Checking the server"
KERNE="$(nproc 2>/dev/null || echo 0)"
RAM_MB="$(awk '/MemTotal/ {print int($2/1024)}' "$MEMINFO" 2>/dev/null || echo 0)"
PLATZ_GB="$(df -Pk . 2>/dev/null | awk 'NR==2 {print int($4/1048576)}' || echo 0)"
echo "  Server: $KERNE Kerne/cores, $((RAM_MB/1024)) GB RAM, $PLATZ_GB GB frei/free"
NEU=1
[ -f ../validator/.env ] && [ -f ../verifier/.env ] && NEU=0
if [ "$NEU" = 1 ] && { [ "$KERNE" -lt "$MIN_KERNE" ] || [ "$RAM_MB" -lt "$MIN_RAM_MB" ] || [ "$PLATZ_GB" -lt "$MIN_PLATZ_GB" ]; }; then
  rot "Dieser Server ist zu schwach fuer Validator und Verifier zusammen -- abgelehnt.
This server is too weak for validator and verifier together -- rejected.
  Mindestens / at least: $MIN_KERNE dedizierte Kerne/dedicated cores, 12 GB RAM, $MIN_PLATZ_GB GB freie SSD/free SSD.
  Hier / here:           $KERNE Kerne/cores, $((RAM_MB/1024)) GB RAM, $PLATZ_GB GB.
Nur einen Teil? / Only one part? deploy/validator oder/or deploy/verifier (docs/VALIDATOR_EINRICHTEN.md, docs/VERIFIER_EINRICHTEN.md)."
  exit 1
fi
gruen "Ausstattung reicht / sufficient"

schritt "2/4 Eine Angabe / One question"
WALLET="${1:-}"
[ -z "$WALLET" ] || [[ "$WALLET" =~ ^0x[0-9a-fA-F]{40}$ ]] || { echo "  Das ist keine Wallet-Adresse: $WALLET"; WALLET=""; }
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
[[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { rot "Keine gueltige IPv4 / not a valid IPv4: $IP"; exit 1; }

schritt "3/4 Validator (fuehrt die Kette mit / runs the chain)"
( cd ../validator && AEQUITAS_IP="$IP" bash einrichten.sh "$WALLET" ) || {
  rot "Der Validator ist nicht fertig geworden (Meldung oben). Danach einfach noch einmal: bash einrichten.sh $WALLET
The validator did not finish (see above). Then simply run it again."
  exit 1
}

schritt "4/4 Verifier (ein Mensch, ein Konto / one human, one account)"
( cd ../verifier && AEQUITAS_WALLET="$WALLET" AEQUITAS_IP="$IP" bash einrichten.sh ) || {
  rot "Der Verifier ist nicht fertig geworden (Meldung oben); der Validator laeuft. Danach noch einmal: bash einrichten.sh $WALLET
The verifier did not finish (see above); the validator is running. Then run it again."
  exit 1
}

gruen "Knoten fertig: Validator und Verifier laufen / Node done: validator and verifier running."
cat <<TEXT

Du bist jetzt Knotenbetreiber. Was noch fehlt, geht in der App (Tab "Knoten"):
  - Der Validator ist an dich gebunden (Kontrollzahl oben) und laeuft als
    vollwertiger Beobachter mit; die Aufnahme als Blockproduzent folgt.
  - Fuer den Verifier: "Schluessel freigeben" und "Bewerben" (Karte "Pruefer").
    Stimmen zwei Drittel der zugelassenen Knotenbetreiber zu, bist du zugelassen.
You are now a node operator. The rest happens in the app ("Node" tab).

Sichere beide Dateien / Back up both files:
    Aequitas/deploy/validator/.env
    Aequitas/deploy/verifier/.env
TEXT
