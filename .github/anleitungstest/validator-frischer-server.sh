# Die Validator-Anleitung von der Website, Schritt fuer Schritt, auf einem
# FRISCHEN Ubuntu -- abgeschottet auf dieser Box:
#   - ein eigener Docker-Daemon (docker:dind) mit eigenem Netz und eigenem
#     Speicher; was dort laeuft, sieht den laufenden Knoten nicht und belegt
#     keine seiner Ports,
#   - darin ein nacktes Ubuntu 24.04, auf dem genau die drei Befehle der
#     Anleitung laufen,
#   - als oeffentliche IP wird eine Dokumentations-IP (203.0.113.10)
#     eingetragen, damit sich der Testknoten nie als dieser Server ausgibt,
#   - die Wallet ist 0x...dEaD: die Bindung (Schritt 6) braucht eine echte
#     Wallet in der App und wird hier NICHT gemacht; der Test endet, sobald
#     Schritt 6 erreicht ist (Schritte 1-5 sind geprueft),
#   - am Ende wird alles entfernt (Container, Volumes, Wegwerf-Schluessel).
set -euo pipefail
N=anleitungstest
LOG="$(mktemp)"
aufraeumen() {
  docker rm -f "$N-ubuntu" "$N-dind" >/dev/null 2>&1 || true
  docker volume rm -f "$N-docker" >/dev/null 2>&1 || true
  rm -f "$LOG"
}
trap aufraeumen EXIT
aufraeumen

echo "== Frischer Server (Ubuntu 24.04 mit eigenem Docker)"
docker run -d --name "$N-dind" --privileged -v "$N-docker:/var/lib/docker" -e DOCKER_TLS_CERTDIR= docker:27-dind >/dev/null
for i in $(seq 1 60); do docker exec "$N-dind" docker info >/dev/null 2>&1 && break; sleep 2; done
docker run -d --name "$N-ubuntu" --network "container:$N-dind" -e DOCKER_HOST=tcp://127.0.0.1:2375 ubuntu:24.04 sleep 7200 >/dev/null
# Was ein Ubuntu-Server von Haus aus mitbringt (das Container-Abbild ist nackter).
docker exec "$N-ubuntu" bash -c 'export DEBIAN_FRONTEND=noninteractive; apt-get update -qq && apt-get install -y -qq curl git openssl python3 iproute2 ca-certificates >/dev/null'

echo "== Befehl 1: curl -fsSL https://get.docker.com | sh"
# Im Container gibt es kein systemd; der Daemon kommt von docker:dind
# (DOCKER_HOST). Ein Fehler beim Starten des Dienstes ist hier erwartbar.
docker exec "$N-ubuntu" bash -c 'curl -fsSL https://get.docker.com | sh' > /dev/null 2>&1 || echo "  (Hinweis: Installer meldete einen Fehler -- im Container ohne systemd erwartbar)"
docker exec "$N-ubuntu" docker compose version

echo "== Befehl 2: git clone https://github.com/hanoi96international-gif/Aequitas.git"
docker exec "$N-ubuntu" bash -c 'cd /root && git clone -q https://github.com/hanoi96international-gif/Aequitas.git && git -C Aequitas log --oneline -1'

echo "== Befehl 3: cd Aequitas/deploy/validator && bash einrichten.sh"
# Eingaben: Wallet, "IP stimmt nicht", Dokumentations-IP.
( printf '0x000000000000000000000000000000000000dEaD\nn\n203.0.113.10\n' \
    | docker exec -i "$N-ubuntu" bash -c 'cd /root/Aequitas/deploy/validator && bash einrichten.sh' > "$LOG" 2>&1
  echo "EXIT=$?" >> "$LOG" ) &
for i in $(seq 1 270); do   # hoechstens 45 Minuten
  grep -q '^EXIT=' "$LOG" && break
  grep -q '== 6/6' "$LOG" && { sleep 20; break; }
  sleep 10
done
# Ausgabe ohne den QR-Code (Blockzeichen) und ohne Farbcodes.
sed -e 's/\x1b\[[0-9;]*m//g' "$LOG" | grep -vE '^[[:space:]]*[█▀▄ ]+[[:space:]]*$' | tail -80
echo
if grep -q '== 6/6' "$LOG"; then
  echo "ERGEBNIS: Schritte 1-5 bestanden; Schritt 6 (Bindung per App) erreicht -- hier bewusst nicht ausgefuehrt."
  docker exec "$N-dind" docker ps --format '{{.Names}} {{.Status}}' || true
elif grep -q '^EXIT=0' "$LOG"; then
  echo "ERGEBNIS: unerwartet ohne Schritt 6 beendet"
  exit 1
else
  echo "ERGEBNIS: abgebrochen (siehe oben)"
  exit 1
fi
