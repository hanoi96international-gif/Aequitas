#!/usr/bin/env bash
# Nur lesend: taugt der neue Server als C1? Laeuft ueber
# .github/workflows/neuer-c1-pruefen.yml auf dem NEUEN Server, vor dem Umzug.
# Ausgegeben werden nur Zahlen und feste Bezeichnungen (Log ist oeffentlich).
#
# Geprueft: Kerne, RAM, Platz, Betriebssystem, Docker vorhanden, Ports 8080 /
# 4001 / 80 / 443 frei, "Steal"-Zeit (Anteil, den der Anbieter einem anderen
# Kunden gibt -- hoch heisst geteilte vCPU) und eine kurze Signatur-Messung
# (openssl, ECDSA P-256 verify je Sekunde, alle Kerne) als grober Vergleich
# mit C1. Die eigentliche Messung des Knotens (leistungsnachweis.go) kommt
# erst, wenn er laeuft.
set -uo pipefail
echo "=== Neuer Server ==="
echo "Kerne: $(nproc)"
echo "RAM: $(awk '/MemTotal/ {printf "%.1f GB", $2/1048576}' /proc/meminfo)"
echo "Platz frei auf /: $(df -BG --output=avail / | tail -1 | tr -d ' ')"
echo "CPU: $(grep -m1 'model name' /proc/cpuinfo | cut -d: -f2 | sed 's/^ *//')"
. /etc/os-release 2>/dev/null && echo "System: ${PRETTY_NAME:-?}"
echo "Virtualisierung: $(systemd-detect-virt 2>/dev/null || echo ?)"
echo "Docker: $(docker --version 2>/dev/null || echo 'nicht installiert (neuer-server-einrichten.yml installiert es)')"

echo "=== Ports (muessen frei sein) ==="
for p in 80 443 4001 8080; do
  if ss -ltnH "( sport = :$p )" 2>/dev/null | grep -q .; then echo "  $p: BELEGT"; else echo "  $p: frei"; fi
done

echo "=== Steal-Zeit ueber 30 s (geteilte vCPU?) ==="
read -r _ u1 n1 s1 i1 w1 q1 sq1 st1 _ < /proc/stat
sleep 30
read -r _ u2 n2 s2 i2 w2 q2 sq2 st2 _ < /proc/stat
ges=$(( (u2+n2+s2+i2+w2+q2+sq2+st2) - (u1+n1+s1+i1+w1+q1+sq1+st1) ))
st=$(( st2 - st1 ))
[ "$ges" -gt 0 ] && echo "  Steal: $(( 1000 * st / ges / 10 )).$(( 1000 * st / ges % 10 )) %  (unter 1 % gut; ueber 5 % = geteilt/ueberbucht)"

echo "=== Signaturen (openssl, ECDSA P-256 verify, alle Kerne, grob) ==="
if command -v openssl >/dev/null; then
  openssl speed -seconds 5 -multi "$(nproc)" ecdsap256 2>/dev/null | grep -E '^ *256 bits ecdsa|nistp256' | tail -1 | awk '{print "  verify/s gesamt: " $NF}'
  echo "  (zum Vergleich auf C1 mit demselben Befehl messen; der Knoten nutzt secp256k1, die Groessenordnung passt)"
else
  echo "  openssl fehlt"
fi
