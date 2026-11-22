#!/usr/bin/env bash
# Nur lesend: was liegt auf C1, das beim Umzug mit muss? Laeuft ueber
# .github/workflows/c1-umzug.yml (phase=inventar). Das Log ist oeffentlich:
# ausgegeben werden Namen, Pfade, Groessen und Image-Digests -- nie Werte von
# Umgebungsvariablen, nie Dateiinhalte.
set -uo pipefail
echo "=== Container (Name, Image, Zustand, Neustart-Regel) ==="
docker ps -a --format '{{.Names}}' | sort | while read -r c; do
  echo "--- $c"
  docker inspect "$c" --format '  Image: {{.Config.Image}} ({{.Image}})
  Zustand: {{.State.Status}}  Neustart: {{.HostConfig.RestartPolicy.Name}}
  Netze: {{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}
  Ports: {{range $p,$b := .HostConfig.PortBindings}}{{$p}} {{end}}' 2>/dev/null
  docker inspect "$c" --format '{{range .Mounts}}{{.Type}}|{{if .Name}}{{.Name}}{{else}}{{.Source}}{{end}}|{{.Destination}}{{println}}{{end}}' 2>/dev/null \
    | grep -v '^$' | while IFS='|' read -r typ quelle ziel; do
      if [ "$typ" = volume ]; then
        mp="$(docker volume inspect "$quelle" --format '{{.Mountpoint}}' 2>/dev/null)"
        echo "  Volume $quelle -> $ziel  ($(du -sh "$mp" 2>/dev/null | cut -f1))"
      else
        echo "  Pfad   $quelle -> $ziel  ($(du -sh "$quelle" 2>/dev/null | cut -f1))"
      fi
    done
  echo "  Umgebung (nur Namen): $(docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | cut -d= -f1 | grep -vE '^(PATH|HOME|HOSTNAME|LANG|GPG_KEY|PYTHON_[A-Z_]*|NODE_VERSION|YARN_VERSION|PG_[A-Z_]*)?$' | tr '\n' ' ')"
done
echo "=== Konfigurationsdateien (nur Namen und Groesse) ==="
for f in /root/Aequitas/deploy/validator/.env /root/.aequitas*.env /root/.*.env /etc/aequitas-sicherung/* /root/wachhund/env; do
  [ -e "$f" ] && echo "  $f ($(stat -c %s "$f") Bytes, Modus $(stat -c %a "$f"))"
done | sort -u
echo "=== Verzeichnisse unter /root (Groesse) ==="
du -sh /root/* /root/.[!.]* 2>/dev/null | sort -k2 | grep -vE '/root/\.(cache|npm|bash_history|lesshst|viminfo|wget-hsts)$'
echo "=== Crontab (nur Befehlsnamen) ==="
crontab -l 2>/dev/null | grep -v '^#' | awk 'NF {print "  " $1" "$2" "$3" "$4" "$5" "$6}'
echo "=== systemd-Einheiten mit aequitas/wachhund/sicherung ==="
systemctl list-unit-files 2>/dev/null | grep -iE 'aequitas|wachhund|sicherung' || echo "  keine"
echo "=== authorized_keys (nur Kommentare/Typen, fuer die Sicherung) ==="
awk '{print "  " $1 " " $NF}' /root/.ssh/authorized_keys 2>/dev/null | grep -v '^  *$'
echo "=== age ==="
[ -x /usr/local/lib/aequitas-sicherung/age ] && echo "  /usr/local/lib/aequitas-sicherung/age vorhanden" || echo "  $(command -v age || echo 'age fehlt')"
