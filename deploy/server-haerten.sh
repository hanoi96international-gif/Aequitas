#!/usr/bin/env bash
# Server-Haertung C1 (docs/BETA_BEREITSCHAFT.md, Betreiber-Punkt 13).
#
# Laeuft ueber .github/workflows/server-haerten.yml auf C1 selbst, per stdin;
# MODUS kommt als export-Zeile davor. Ohne MODUS wird nur geprueft.
#
#   pruefen      (Voreinstellung) nur lesen: sshd-Einstellungen, Root-Passwort
#                (nur der Status), Docker-Log-Rotation, oeffentlich gebundene
#                Ports, Firewall -- mit einem Urteil je Punkt.
#   anwenden     SSH nur mit Schluessel (Drop-in, reload) und Docker-Log-
#                Rotation in /etc/docker/daemon.json. Idempotent, fail-closed.
#   root-sperren Root-Passwort sperren (passwd -l). Der Workflow ruft das erst
#                auf, nachdem ein NEUER Schluessel-Login nach dem reload
#                geklappt hat. Nicht als Eingabe des Workflows waehlbar.
#
# WAS NICHT GESCHIEHT, mit Absicht:
#   - authorized_keys wird nie veraendert und nie ausgegeben.
#   - sshd wird nur neu geladen (reload), nie neu gestartet: bestehende
#     Verbindungen -- auch diese -- bleiben.
#   - dockerd und die Container werden NICHT neu gestartet (die Kette liefe
#     sonst nicht). Die Log-Rotation in daemon.json gilt deshalb erst fuer
#     Container, die nach dem naechsten Start des Docker-Dienstes erstellt
#     werden; bestehende Container ohne max-size werden aufgelistet.
#   - Port 8080 wird NICHT geschlossen: Peers sprechen den Knoten direkt ueber
#     http://IP:8080 an (SELF_URL, PRIMARY_NODE_URLS; deploy/neuer-server.sh
#     oeffnet 8080 in ufw). Ihn zu schliessen ist ein Netzwechsel und bleibt
#     die Entscheidung des Betreibers. Hier wird nur berichtet.
#
# Grenzen (AGENTS.md Frage 2): jeder externe Befehl unter timeout, jede
# Ausgabe per head begrenzt, keine offenen Schleifen.
set -euo pipefail
umask 022

MODUS="${MODUS:-pruefen}"
case "$MODUS" in
  pruefen|anwenden|root-sperren) ;;
  *) echo "::error::Unbekannter MODUS '${MODUS:0:20}' -- erlaubt: pruefen, anwenden, root-sperren. Nichts geaendert."; exit 2 ;;
esac

# WURZEL nur fuer scripts/lint_server_haerten.py (Attrappen-Baum). Der
# Workflow setzt sie nie.
WURZEL="${WURZEL:-}"
if [ -z "$WURZEL" ] && [ "$(id -u)" != 0 ]; then
  echo "::error::Muss als root laufen."; exit 2
fi
SSHD_CONFIG="$WURZEL/etc/ssh/sshd_config"
DROPIN_DIR="$WURZEL/etc/ssh/sshd_config.d"
DROPIN="$DROPIN_DIR/00-aequitas-haerten.conf"
AUTHK="$WURZEL/root/.ssh/authorized_keys"
DJSON="$WURZEL/etc/docker/daemon.json"
T=20   # Sekunden je Befehl

ARBEIT="$(mktemp -d)"
aufraeumen() { rm -rf "$ARBEIT"; }
trap aufraeumen EXIT

URTEILE=""
urteil() { URTEILE="${URTEILE}$(printf '%-14s %-7s %s' "$1" "$2" "$3")"$'\n'; }

# Ganze Ausgabe erst einlesen, dann auswerten: ein frueh endendes awk/grep
# in einer Pipe liesse sshd an SIGPIPE scheitern (pipefail).
sshd_T() { timeout "$T" sshd -T 2>/dev/null || true; }

sshd_wert() {  # sshd_wert <schluessel> -> Wert aus sshd -T (effektive Datei-Konfiguration)
  awk -v k="$1" '$1 == k {print $2; exit}' <<<"$(sshd_T)"
}

kbd_schluessel() {  # OpenSSH >= 8.7: kbdinteractiveauthentication, davor challengeresponseauthentication
  if grep -q '^kbdinteractiveauthentication ' <<<"$(sshd_T)"; then
    echo kbdinteractiveauthentication
  else
    echo challengeresponseauthentication
  fi
}

authk_ok() { [ -f "$AUTHK" ] && [ -s "$AUTHK" ] && grep -qE '^(ssh-|ecdsa-|sk-)' "$AUTHK"; }

root_status() { awk '{print $2; exit}' <<<"$(timeout "$T" passwd -S root 2>/dev/null || true)"; }

# ---------------------------------------------------------------- pruefen
bericht() {
  echo "== SSH (sshd -T, effektiv) =="
  local kk pa pr kb pk up
  kk="$(kbd_schluessel)"
  pa="$(sshd_wert passwordauthentication)"; pr="$(sshd_wert permitrootlogin)"
  kb="$(sshd_wert "$kk")"; pk="$(sshd_wert pubkeyauthentication)"; up="$(sshd_wert usepam)"
  printf '  passwordauthentication  %s\n  permitrootlogin         %s\n  %-23s %s\n  pubkeyauthentication    %s\n  usepam                  %s\n' \
    "${pa:-?}" "${pr:-?}" "$kk" "${kb:-?}" "${pk:-?}" "${up:-?}"
  if [ -f "$DROPIN" ]; then echo "  Drop-in vorhanden: $DROPIN"; else echo "  Drop-in fehlt: $DROPIN"; fi
  if authk_ok; then echo "  authorized_keys von root: vorhanden, $(grep -cE '^(ssh-|ecdsa-|sk-)' "$AUTHK") Schluessel (Inhalt wird nicht gezeigt)"; else echo "  authorized_keys von root: FEHLT oder leer"; fi
  if [ "$pa" = no ] && [ "$kb" = no ] && [ "$pk" = yes ] && { [ "$pr" = without-password ] || [ "$pr" = prohibit-password ]; }; then
    urteil "SSH" "OK" "nur Schluessel; root nur mit Schluessel"
  else
    urteil "SSH" "OFFEN" "Passwort/Tastatur-Login moeglich oder root mit Passwort (anwenden behebt das)"
  fi

  echo "== Root-Passwort (nur Status aus passwd -S) =="
  local rs; rs="$(root_status)"
  case "$rs" in
    L|LK) echo "  Status: gesperrt ($rs)"; urteil "Root-Passwort" "OK" "gesperrt" ;;
    NP)   echo "  Status: KEIN Passwort ($rs)"; urteil "Root-Passwort" "OFFEN" "leer -- sperren (anwenden)" ;;
    P|PS) echo "  Status: gesetzt ($rs)"; urteil "Root-Passwort" "OFFEN" "gesetzt -- sperren (anwenden)" ;;
    *)    echo "  Status: unbekannt"; urteil "Root-Passwort" "UNKLAR" "passwd -S lieferte nichts Lesbares" ;;
  esac

  echo "== Docker-Log-Rotation =="
  if [ -f "$DJSON" ]; then
    python3 -I - "$DJSON" <<'PY' || echo "  daemon.json: KEIN gueltiges JSON"
import json, sys
d = json.load(open(sys.argv[1]))
if not isinstance(d, dict):
    raise SystemExit(1)
o = d.get("log-opts") or {}
print("  daemon.json log-driver: %s" % d.get("log-driver", "(nicht gesetzt = json-file)"))
print("  daemon.json max-size:   %s" % o.get("max-size", "(nicht gesetzt)"))
print("  daemon.json max-file:   %s" % o.get("max-file", "(nicht gesetzt)"))
PY
  else
    echo "  daemon.json fehlt (Docker-Voreinstellung: json-file OHNE Grenze)"
  fi
  local ohne=0 gesamt=0 zeile name typ ms
  # Hoechstens 200 Container (Grenze).
  while IFS= read -r zeile; do
    [ -n "$zeile" ] || continue
    gesamt=$((gesamt + 1))
    name="${zeile%% *}"; zeile="${zeile#* }"; typ="${zeile%% *}"; ms="${zeile#* }"
    [ "$ms" = "<no value>" ] && ms=""
    if [ "$typ" = json-file ] && [ -z "$ms" ]; then
      ohne=$((ohne + 1)); echo "  OHNE max-size: ${name#/} ($typ)"
    else
      echo "  ok:            ${name#/} ($typ${ms:+, max-size $ms})"
    fi
  done < <(timeout "$T" docker ps -aq 2>/dev/null | head -200 | xargs -r timeout "$T" docker inspect --format '{{.Name}} {{.HostConfig.LogConfig.Type}} {{index .HostConfig.LogConfig.Config "max-size"}}' 2>/dev/null || true)
  if [ "$ohne" = 0 ] && [ "$gesamt" -gt 0 ]; then
    urteil "Docker-Logs" "OK" "alle $gesamt Container begrenzt"
  elif [ "$gesamt" = 0 ]; then
    urteil "Docker-Logs" "UNKLAR" "keine Container gelesen"
  else
    urteil "Docker-Logs" "OFFEN" "$ohne von $gesamt Container ohne max-size (greift erst bei Neuerstellung nach Docker-Neustart)"
  fi

  echo "== Oeffentlich gebundene TCP-Ports (ss -ltnp, ohne Loopback) =="
  local p8080=nein adr prozess
  while IFS= read -r zeile; do
    [ -n "$zeile" ] || continue
    adr="$(awk '{print $4}' <<<"$zeile")"
    case "$adr" in 127.*|"[::1]":*|::1:*) continue ;; esac
    prozess="$(grep -oE 'users:\(\("[^"]+"' <<<"$zeile" | head -1 | sed 's/users:(("//; s/"$//' || true)"
    echo "  $adr  ${prozess:-?}"
    case "$adr" in *:8080) p8080=ja ;; esac
  done < <(timeout "$T" ss -H -ltnp 2>/dev/null | head -100 || true)
  echo "  Hinweis: von Docker veroeffentlichte Ports (docker-proxy) umgehen ufw ueber eigene iptables-Regeln."
  if [ "$p8080" = ja ]; then
    urteil "Port 8080" "BETREIBER" "oeffentlich -- bewusst: Peers nutzen http://IP:8080 (SELF_URL, PRIMARY_NODE_URLS); Schliessen = Netzwechsel"
  else
    urteil "Port 8080" "OK" "nicht oeffentlich gebunden"
  fi

  echo "== Firewall =="
  if command -v ufw >/dev/null 2>&1; then
    timeout "$T" ufw status verbose 2>/dev/null | head -40 | sed 's/^/  /' || echo "  ufw status nicht lesbar"
  else
    echo "  ufw nicht installiert"
  fi
  if command -v nft >/dev/null 2>&1; then
    echo "  nftables-Tabellen:"
    timeout "$T" nft list tables 2>/dev/null | head -30 | sed 's/^/    /' || echo "    nicht lesbar"
  fi
  if grep -q '^Status: active' <<<"$(timeout "$T" ufw status 2>/dev/null || true)"; then
    urteil "Firewall" "OK" "ufw aktiv"
  else
    urteil "Firewall" "OFFEN" "ufw nicht aktiv (oder nicht installiert) -- Betreiber-Entscheidung"
  fi

  echo
  echo "== Urteil =="
  printf '%s' "$URTEILE"
}

# ---------------------------------------------------------------- anwenden
ssh_anwenden() {
  echo "== SSH haerten =="
  # 1. Vor jeder Aenderung: es gibt einen Schluessel fuer root. Sonst sperrt
  #    "nur Schluessel" den Betreiber aus.
  if ! authk_ok; then
    echo "::error::$AUTHK fehlt, ist leer oder enthaelt keinen Schluessel -- SSH bleibt unveraendert."
    return 1
  fi
  # 2. Das Drop-in muss ueberhaupt gelesen werden.
  if ! grep -qE '^[[:space:]]*Include[[:space:]]+/etc/ssh/sshd_config\.d/\*\.conf' "$SSHD_CONFIG"; then
    echo "::error::$SSHD_CONFIG bindet sshd_config.d/*.conf nicht ein -- das Drop-in waere wirkungslos. Nichts geaendert."
    return 1
  fi
  local kk; kk="$(kbd_schluessel)"
  local kk_name=KbdInteractiveAuthentication
  [ "$kk" = challengeresponseauthentication ] && kk_name=ChallengeResponseAuthentication
  local neu="$ARBEIT/haerten.neu"
  {
    echo "# Verwaltet von deploy/server-haerten.sh (.github/workflows/server-haerten.yml)."
    echo "# 00-: sshd nimmt den ERSTEN Wert -- dieses Drop-in geht vor 50-cloud-init.conf."
    echo "PasswordAuthentication no"
    echo "$kk_name no"
    echo "PermitRootLogin prohibit-password"
    echo "PubkeyAuthentication yes"
  } > "$neu"
  if [ -f "$DROPIN" ] && cmp -s "$neu" "$DROPIN"; then
    echo "  Drop-in unveraendert -- kein reload noetig."
  else
    local alt="$ARBEIT/haerten.alt"
    if [ -f "$DROPIN" ]; then cp -p "$DROPIN" "$alt"; fi
    mkdir -p "$DROPIN_DIR"
    install -m 0644 "$neu" "$DROPIN" || { echo "::error::Drop-in nicht schreibbar."; return 1; }
    # 3. Syntax pruefen, BEVOR sshd neu laedt. Faellt sie durch: alten Stand
    #    zurueck, nichts neu laden.
    if ! timeout "$T" sshd -t; then
      if [ -f "$alt" ]; then install -m 0644 "$alt" "$DROPIN"; else rm -f "$DROPIN"; fi
      echo "::error::sshd -t lehnt die Konfiguration ab -- Drop-in zurueckgenommen, sshd NICHT neu geladen."
      return 1
    fi
    # 4. reload, nicht restart: bestehende Sitzungen bleiben. Unter Socket-
    #    Aktivierung (Ubuntu 24.04) liest jede neue Verbindung die Datei
    #    ohnehin neu.
    if timeout "$T" systemctl is-active --quiet ssh.service; then
      timeout "$T" systemctl reload ssh.service || { echo "::error::reload ssh.service fehlgeschlagen."; return 1; }
    elif timeout "$T" systemctl is-active --quiet sshd.service; then
      timeout "$T" systemctl reload sshd.service || { echo "::error::reload sshd.service fehlgeschlagen."; return 1; }
    elif timeout "$T" systemctl is-active --quiet ssh.socket; then
      echo "  Drop-in geschrieben; ssh.socket aktiv (Socket-Aktivierung) -- jede neue Verbindung liest es."
    else
      # Weder Dienst noch Socket aktiv: ein laufender sshd (andere Einheit,
      # ohne systemd) liefe mit dem alten Stand weiter, und sshd -T unten
      # liest nur die Dateien (Pruefung von #388). Nicht als Erfolg melden.
      echo "::error::Weder ssh.service, sshd.service noch ssh.socket aktiv -- kein reload moeglich. Drop-in bleibt geschrieben (nur haertend), Root wird NICHT gesperrt."
      return 1
    fi
    echo "  Drop-in geschrieben und angewendet."
  fi
  # 5. Wirkung pruefen.
  local pa pr kb pk
  pa="$(sshd_wert passwordauthentication)"; pr="$(sshd_wert permitrootlogin)"
  kb="$(sshd_wert "$kk")"; pk="$(sshd_wert pubkeyauthentication)"
  echo "  danach: passwordauthentication=$pa permitrootlogin=$pr $kk=$kb pubkeyauthentication=$pk"
  if [ "$pa" != no ] || [ "$kb" != no ] || [ "$pk" != yes ] || { [ "$pr" != without-password ] && [ "$pr" != prohibit-password ]; }; then
    echo "::error::sshd -T zeigt die gehaerteten Werte nicht -- ein frueherer Eintrag geht vor (sshd_config.d pruefen). Das Drop-in bleibt stehen (es haertet nur); Root wird NICHT gesperrt."
    return 1
  fi
}

docker_anwenden() {
  echo "== Docker-Log-Rotation =="
  # Setzt die systemd-Einheit schon --log-driver/--log-opt, verweigert
  # dockerd beim naechsten Start den Doppeleintrag in daemon.json -- und
  # dockerd --validate sieht die Flags der Einheit nicht (Pruefung von #388).
  if timeout "$T" systemctl cat docker 2>/dev/null | grep -Eq -- '--log-(driver|opt)'; then
    echo "::error::Die docker-Einheit setzt --log-driver/--log-opt selbst -- daemon.json nicht angefasst (sonst startet dockerd nicht mehr). Betreiber-Entscheidung."
    return 1
  fi
  local neu="$ARBEIT/daemon.json.neu" rc=0
  mkdir -p "$(dirname "$DJSON")"
  # Zusammenfuehren: vorhandene Schluessel bleiben, vorhandene log-opts
  # bleiben; nur fehlende max-size/max-file werden gesetzt. Ungueltiges JSON
  # oder ein anderer Log-Treiber: nichts schreiben (fail-closed).
  python3 -I - "$DJSON" "$neu" <<'PY' || rc=$?
import json, os, sys
src, dst = sys.argv[1], sys.argv[2]
d = {}
if os.path.exists(src):
    try:
        d = json.load(open(src))
    except ValueError:
        print("::error::daemon.json ist kein gueltiges JSON -- nicht ueberschrieben.")
        sys.exit(2)
if not isinstance(d, dict):
    print("::error::daemon.json ist kein JSON-Objekt -- nicht ueberschrieben.")
    sys.exit(2)
drv = d.get("log-driver", "json-file")
if drv != "json-file":
    print("  log-driver ist %r -- nicht angefasst (Betreiber-Entscheidung)." % drv)
    sys.exit(3)
o = d.get("log-opts") or {}
if not isinstance(o, dict):
    print("::error::log-opts ist kein Objekt -- nicht ueberschrieben.")
    sys.exit(2)
d["log-driver"] = "json-file"
o.setdefault("max-size", "50m")
o.setdefault("max-file", "5")
d["log-opts"] = o
with open(dst, "w") as f:
    json.dump(d, f, indent=2, sort_keys=True)
    f.write("\n")
json.load(open(dst))
PY
  case "$rc" in
    0) ;;
    3) return 0 ;;
    *) return 1 ;;
  esac
  if command -v dockerd >/dev/null 2>&1 && timeout "$T" dockerd --help 2>/dev/null | grep -q -- '--validate'; then
    if ! timeout "$T" dockerd --validate --config-file "$neu" >/dev/null 2>&1; then
      echo "::error::dockerd --validate lehnt die neue daemon.json ab -- nicht geschrieben."
      return 1
    fi
  fi
  if [ -f "$DJSON" ] && python3 -I -c 'import json,sys; sys.exit(0 if json.load(open(sys.argv[1])) == json.load(open(sys.argv[2])) else 1)' "$DJSON" "$neu"; then
    echo "  daemon.json unveraendert."
  else
    if [ -f "$DJSON" ]; then cp -p "$DJSON" "$DJSON.vor-haerten-$(date -u +%Y%m%dT%H%M%SZ)" || { echo "::error::Sicherung der alten daemon.json fehlgeschlagen -- nicht geschrieben."; return 1; }; fi
    install -m 0644 "$neu" "$DJSON" || { echo "::error::daemon.json nicht schreibbar."; return 1; }
    echo "  daemon.json geschrieben (Sicherung der alten Fassung daneben)."
  fi
  echo "  dockerd und Container werden NICHT neu gestartet. Die Rotation gilt fuer Container,"
  echo "  die nach dem naechsten Start des Docker-Dienstes erstellt werden -- Wartungsfenster"
  echo "  des Betreibers. Bestehende Container ohne max-size: siehe Bericht unten."
}

root_sperren() {
  echo "== Root-Passwort sperren =="
  # Nur, wenn wir nachweislich per SSH-Schluessel hier sind und Schluessel-
  # Login fuer root erlaubt bleibt. UsePAM yes ist Pflicht: ohne PAM lehnt
  # OpenSSH ein gesperrtes Konto ("!" im Hash) auch beim Schluessel-Login ab.
  [ -n "${SSH_CONNECTION:-}" ] || { echo "::error::keine SSH-Sitzung erkannt -- nicht gesperrt."; return 1; }
  authk_ok || { echo "::error::$AUTHK fehlt oder leer -- nicht gesperrt."; return 1; }
  local pa pr up
  pa="$(sshd_wert passwordauthentication)"; pr="$(sshd_wert permitrootlogin)"; up="$(sshd_wert usepam)"
  [ "$up" = yes ] || { echo "::error::UsePAM ist nicht yes -- ein gesperrtes root-Konto wuerde auch den Schluessel-Login sperren. Nicht gesperrt."; return 1; }
  [ "$pa" = no ] || { echo "::error::Passwort-Login noch an -- erst anwenden. Nicht gesperrt."; return 1; }
  case "$pr" in without-password|prohibit-password) ;; *) echo "::error::permitrootlogin=$pr -- nicht gesperrt."; return 1 ;; esac
  case "$(root_status)" in
    L|LK) echo "  bereits gesperrt -- unveraendert." ;;
    *) timeout "$T" passwd -l root >/dev/null
       case "$(root_status)" in L|LK) echo "  gesperrt." ;; *) echo "::error::passwd -l root hat nicht gewirkt."; return 1 ;; esac ;;
  esac
  echo "  Hinweis: auch die Konsole des Anbieters nimmt dann kein root-Passwort mehr; Notzugang ueber dessen Rettungssystem."
}

echo "Modus: $MODUS"
case "$MODUS" in
  pruefen) bericht ;;
  anwenden)
    fehler=0
    ssh_anwenden || fehler=1
    docker_anwenden || fehler=1
    echo
    bericht
    if [ "$fehler" != 0 ]; then echo "::error::Mindestens ein Schritt ist fehlgeschlagen (oben)."; exit 1; fi ;;
  root-sperren) root_sperren ;;
esac
