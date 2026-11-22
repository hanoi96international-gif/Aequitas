#!/usr/bin/env bash
# Probe des C1-Umzugs mit echtem Docker (deploy/umzug-packen.sh und
# deploy/umzug-auspacken.sh), alter und neuer Server auf derselben Maschine.
# Braucht root und einen laufenden Docker-Daemon (im CI: sudo).
#
# Geprueft: Probe-Transport (zaehlt, loescht wieder), manipulierter und
# abgeschnittener Strom abgewiesen, Probe-Paket nie "ernst" ausgepackt (und
# kein Klartext bleibt liegen), Ernst haelt alle Container an und schaltet
# den Neustart ab, Volume und gebundene Pfade kommen byte-gleich an, ein
# zweites Auspacken und ein Auspacken neben einem laufenden aequitas-node
# werden verweigert.
set -euo pipefail
cd "$(dirname "$0")/.."
BILD="${BILD:-alpine:3.20}"
T="$(mktemp -d)"
fehl() { echo "FEHLER: $*"; exit 1; }
schritt() { echo "== $(date +%T) $*"; }
aufraeumen() {
  docker rm -f umzugtest-dienst aequitas-node >/dev/null 2>&1 || true
  docker volume rm umzugtest-vol >/dev/null 2>&1 || true
  rm -rf /root/umzugtest-daten /root/.umzug /root/umzug-probe "$T"
}
trap aufraeumen EXIT
aufraeumen; T="$(mktemp -d)"
schritt "age und Bild"
command -v age >/dev/null || { DEBIAN_FRONTEND=noninteractive apt-get update -qq >/dev/null; DEBIAN_FRONTEND=noninteractive apt-get install -y -qq age >/dev/null; }
docker image inspect "$BILD" >/dev/null 2>&1 || docker pull -q "$BILD" >/dev/null

mkdir -p /root/umzugtest-daten && echo geheim > /root/umzugtest-daten/a.txt
head -c 3145728 /dev/urandom > /root/umzugtest-daten/b.bin
docker volume create umzugtest-vol >/dev/null
docker run -d --name umzugtest-dienst --restart unless-stopped -v umzugtest-vol:/data -v /root/umzugtest-daten:/b "$BILD" \
  sh -c 'echo inhalt > /data/x; trap "exit 0" TERM; sleep 100000 & wait' >/dev/null
sleep 2

schritt "Schluessel auf dem neuen Server"
EMPF="$(MODUS=schluessel bash deploy/umzug-auspacken.sh)"
case "$EMPF" in age1*) ;; *) fehl "kein oeffentlicher Schluessel: $EMPF" ;; esac

schritt "Probe packen"
MODUS=probe EMPF="$EMPF" bash deploy/umzug-packen.sh 2>/dev/null > "$T/p.age"
MODUS=probe bash deploy/umzug-auspacken.sh < "$T/p.age" 2>/dev/null || fehl "Probe scheitert"
[ -e /root/umzug-probe ] && fehl "Probe nicht geloescht"

schritt "Manipulation"
python3 -c "import sys; d=bytearray(open(sys.argv[1],'rb').read()); d[len(d)//2]^=1; open(sys.argv[2],'wb').write(d)" "$T/p.age" "$T/kaputt.age"
MODUS=probe bash deploy/umzug-auspacken.sh < "$T/kaputt.age" >/dev/null 2>&1 && fehl "manipulierter Strom angenommen"
head -c 2000 "$T/p.age" > "$T/kurz.age"
MODUS=probe bash deploy/umzug-auspacken.sh < "$T/kurz.age" >/dev/null 2>&1 && fehl "abgeschnittener Strom angenommen"
MODUS=ernst bash deploy/umzug-auspacken.sh < "$T/p.age" >/dev/null 2>&1 && fehl "Probe-Paket ernst ausgepackt"
[ -e /root/.umzug/eingang ] && fehl "Klartext nach Abbruch liegen geblieben"
[ -e /root/.umzug/ausgepackt ] && fehl "Abbruch als ausgepackt markiert"
MODUS=probe EMPF="kein-schluessel" bash deploy/umzug-packen.sh >/dev/null 2>&1 && fehl "ungueltiger Empfaenger angenommen"

schritt "Ernst packen"
SUMME_VORHER="$(cd /root/umzugtest-daten && sha256sum ./* | sha256sum)"
MODUS=ernst EMPF="$EMPF" bash deploy/umzug-packen.sh 2>/dev/null > "$T/e.age" || fehl "ernstes Packen scheitert"
[ "$(docker inspect umzugtest-dienst --format '{{.State.Status}} {{.HostConfig.RestartPolicy.Name}}')" = "exited no" ] || fehl "Dienst laeuft noch oder startet neu"
grep -qx "umzugtest-dienst unless-stopped" /root/.umzug/gestoppt || fehl "Liste der gestoppten Container fehlt"

schritt "Rueckweg"
# Rueckweg: der alte Server startet wieder, was lief, mit der alten Regel.
bash deploy/umzug-zurueck.sh >/dev/null || fehl "Rueckweg scheitert"
[ "$(docker inspect umzugtest-dienst --format '{{.State.Status}} {{.HostConfig.RestartPolicy.Name}}')" = "running unless-stopped" ] || fehl "Rueckweg: Dienst nicht wie vorher"
printf 'umzugtest-dienst; rm -rf /tmp/x unless-stopped\n' > /root/.umzug/gestoppt
bash deploy/umzug-zurueck.sh >/dev/null 2>&1 && fehl "Rueckweg nahm eine verdorbene Liste an"
# Wieder ernst packen fuer den Rest der Probe.
MODUS=ernst EMPF="$EMPF" bash deploy/umzug-packen.sh 2>/dev/null > "$T/e.age" || fehl "zweites ernstes Packen scheitert"

schritt "Neuer Server auspacken"
# "Neuer Server": alles weg, dann auspacken.
docker rm -f umzugtest-dienst >/dev/null; docker volume rm umzugtest-vol >/dev/null; rm -rf /root/umzugtest-daten
docker run -d --name aequitas-node "$BILD" sleep 100000 >/dev/null
MODUS=ernst bash deploy/umzug-auspacken.sh < "$T/e.age" >/dev/null 2>&1 && fehl "ausgepackt neben laufendem aequitas-node"
docker rm -f aequitas-node >/dev/null
MODUS=ernst bash deploy/umzug-auspacken.sh < "$T/e.age" 2>/dev/null || fehl "ernstes Auspacken scheitert"
[ "$(cat /var/lib/docker/volumes/umzugtest-vol/_data/x)" = inhalt ] || fehl "Volume nicht wiederhergestellt"
[ "$(cd /root/umzugtest-daten && sha256sum ./* | sha256sum)" = "$SUMME_VORHER" ] || fehl "gebundene Daten weichen ab"
MODUS=ernst bash deploy/umzug-auspacken.sh < "$T/e.age" >/dev/null 2>&1 && fehl "zweites Auspacken angenommen"
MODUS=aufraeumen bash deploy/umzug-auspacken.sh 2>/dev/null
[ -e /root/.umzug/schluessel ] && fehl "Umzugsschluessel nicht geloescht"
echo "ok: Umzug geprobt (Probe, Manipulation, Ernst, Wiederherstellung, Sperren)"
