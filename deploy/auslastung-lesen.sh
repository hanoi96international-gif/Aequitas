#!/usr/bin/env bash
# Nur lesend: Auslastung von C1 je Container, ueber eine Messdauer -- fuer die
# Wahl des kleineren Servers beim Umzug (geplant 23.12.2026).
#
# Das Actions-Log ist oeffentlich: ausgegeben werden NUR Zahlen und
# Container-Namen (CPU in Kernen, RAM, Platz, Last), nie Umgebung, Dateien,
# Logs oder Adressen. Laeuft ueber .github/workflows/auslastung-c1.yml
# (DAUER=<Sekunden> bash -s < deploy/auslastung-lesen.sh).
#
# Gemessen wird alle 10 s, hoechstens 3600 s (feste Grenze, auch wenn DAUER
# groesser uebergeben wird); berichtet werden Mittel und Spitze je Container.
set -uo pipefail
DAUER="${DAUER:-600}"
case "$DAUER" in ''|*[!0-9]*) DAUER=600 ;; esac
[ "$DAUER" -gt 3600 ] && DAUER=3600
[ "$DAUER" -lt 30 ] && DAUER=30
TAKT=10

echo "=== Server ==="
echo "Kerne: $(nproc)  RAM gesamt: $(awk '/MemTotal/ {printf "%.1f GB", $2/1048576}' /proc/meminfo)  CPU: $(grep -m1 'model name' /proc/cpuinfo | cut -d: -f2 | sed 's/^ *//')"
echo "RAM jetzt (free -m):"; free -m | sed -n '1,3p'
echo "Platz /:"; df -h / | tail -1 | awk '{print "  belegt " $3 " von " $2 " (" $5 ")"}'
echo "Docker-Daten:"; docker system df --format '  {{.Type}}: {{.Size}} (davon freigebbar {{.Reclaimable}})' 2>/dev/null || true
echo "Volumes (Groesse):"
for v in $(docker volume ls -q); do
  mp="$(docker volume inspect "$v" --format '{{.Mountpoint}}' 2>/dev/null)"
  [ -n "$mp" ] && echo "  $v: $(du -sh "$mp" 2>/dev/null | cut -f1)"
done
echo "Last (1/5/15 min): $(cut -d' ' -f1-3 /proc/loadavg)"

echo "=== Messung: ${DAUER}s, alle ${TAKT}s ==="
TMP="$(mktemp)"; trap 'rm -f "$TMP"' EXIT
ENDE=$(( $(date +%s) + DAUER ))
while [ "$(date +%s)" -lt "$ENDE" ]; do
  # CPUPerc: 100% = ein Kern. MemUsage: "123.4MiB / 15.6GiB".
  docker stats --no-stream --format '{{.Name}} {{.CPUPerc}} {{.MemUsage}}' 2>/dev/null >> "$TMP"
  echo "LAST $(cut -d' ' -f1 /proc/loadavg)" >> "$TMP"
  sleep "$TAKT"
done

awk '
function mib(x,   z) {
  z = x; gsub(/[A-Za-z]/, "", z)
  if (x ~ /GiB/) return z*1024; if (x ~ /MiB/) return z; if (x ~ /KiB/) return z/1024; if (x ~ /B/) return z/1048576
  return z
}
$1 == "LAST" { n++; s += $2; if ($2 > m) m = $2; next }
{
  c = $2; gsub(/%/, "", c); c = c / 100
  r = mib($3)
  k = $1; N[k]++; C[k] += c; R[k] += r
  if (c > CM[k]) CM[k] = c; if (r > RM[k]) RM[k] = r
}
END {
  printf "%-28s %10s %10s %12s %12s\n", "Container", "CPU mittel", "CPU Spitze", "RAM mittel", "RAM Spitze"
  for (k in N) { printf "%-28s %9.2fK %9.2fK %10.0fMB %10.0fMB\n", k, C[k]/N[k], CM[k], R[k]/N[k], RM[k]; tc += C[k]/N[k]; tr += R[k]/N[k]; trm += RM[k] }
  printf "%-28s %9.2fK %10s %10.0fMB %10.0fMB\n", "SUMME", tc, "", tr, trm
  if (n) printf "Last: mittel %.2f, Spitze %.2f (Kerne in Arbeit/Warteschlange)\n", s/n, m
  print "(K = Kerne; 1.00K = ein Kern voll. RAM Spitze-Summe ist eine obere Schranke.)"
}' "$TMP"
