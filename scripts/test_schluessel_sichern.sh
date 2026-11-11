#!/usr/bin/env bash
# Prueft das Box-Skript aus .github/workflows/schluessel-sichern.yml mit einem
# nachgebauten docker und echtem age (Missbrauchstest nach AGENTS.md, Frage 5):
#
#   1. Gutfall: das Archiv laesst sich mit dem privaten Schluessel oeffnen und
#      enthaelt genau die Umgebungen; kein Klartext bleibt zurueck; im
#      Protokoll steht kein Wert.
#   2. Kein oder ein falscher Empfaenger (z. B. ein PRIVATER Schluessel):
#      Abbruch, nichts ausgegeben.
#   3. Die Ausgabe liegt nicht in ihrem eigenen Archiv.
set -euo pipefail
WURZEL=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
# age: zuerst aus dem Paketspiegel; faellt der aus (11.11.2026: auf den
# GitHub-Runnern war security.ubuntu.com nicht aufloesbar, dreimal in
# Folge), dieselbe Version ueber den Go-Modul-Proxy -- Pruefsummen gegen
# sum.golang.org. Ohne age kein Test: Abbruch, nicht ueberspringen.
if ! command -v age >/dev/null || ! command -v age-keygen >/dev/null; then
  if ! sudo apt-get install -y -qq age >/dev/null 2>&1; then
    GOBIN="$T/bin" go install filippo.io/age/cmd/age@v1.2.1 filippo.io/age/cmd/age-keygen@v1.2.1
    PATH="$T/bin:$PATH"
  fi
fi
command -v age >/dev/null && command -v age-keygen >/dev/null || { echo "FEHLER: age nicht verfuegbar"; exit 1; }
mkdir -p "$T/fake" "$T/etc" "$T/root/Aequitas/deploy/validator"
age-keygen -o "$T/offline.key" 2>/dev/null
grep -o 'age1[a-z0-9]*' "$T/offline.key" > "$T/etc/bio-empfaenger"
printf 'NODE_KEY=geheim-knoten\n' > "$T/root/Aequitas/deploy/validator/.env"
cat > "$T/fake/docker" <<'EOF'
#!/bin/bash
case "$2" in
  aequitas-coordinator) [ "${3:-}" = --format ] && printf 'COORDINATOR_SIGNING_KEY=geheim-a\n' ;;
  aequitas-matching) [ "${3:-}" = --format ] && printf 'FACE_SKETCH_SEED=geheim-seed\n' ;;
  proof-server|aequitas-node) [ "${3:-}" = --format ] && printf 'TOKEN=geheim-t\n' ;;
  *) exit 1 ;;
esac
exit 0
EOF
chmod +x "$T/fake/docker"
python3 - "$WURZEL/.github/workflows/schluessel-sichern.yml" > "$T/skript.sh" <<'PY'
import sys, yaml
w = yaml.safe_load(open(sys.argv[1]))
run = next(s["run"] for s in w["jobs"]["sichern"]["steps"] if "<<'SKRIPT'" in s.get("run", ""))
a = run.index("<<'SKRIPT'\n") + len("<<'SKRIPT'\n"); b = run.index("\nSKRIPT\n")
print(run[a:b])
PY
sed -i "s#/etc/aequitas-sicherung#$T/etc#g; s#/root/Aequitas#$T/root/Aequitas#g; s#/root/.aequitas-matching.env#$T/root/.aequitas-matching.env#g; s#mktemp -d /root/#mktemp -d $T/#; s#mktemp /root/#mktemp $T/#" "$T/skript.sh"

fehler=0
soll() { if ! eval "$2"; then echo "FEHLER: $1"; fehler=1; fi; }

PATH="$T/fake:$PATH" bash "$T/skript.sh" > "$T/out.age" 2> "$T/log"
INHALT=$(age -d -i "$T/offline.key" "$T/out.age" | tar tf - | sort | tr '\n' ' ')
soll "Archiv enthaelt die Umgebungen" '[ "$INHALT" = "./ ./STAND ./aequitas-coordinator.env ./aequitas-matching.env ./aequitas-node.env ./knoten-validator.env ./proof-server.env " ]'
soll "Seed im Archiv" 'age -d -i "$T/offline.key" "$T/out.age" | tar xOf - ./aequitas-matching.env | grep -q geheim-seed'
soll "kein Wert im Protokoll" '! grep -q geheim "$T/log"'
soll "kein Klartext zurueck" '[ -z "$(ls -A "$T" | grep schluessel-sichern || true)" ]'

echo "AGE-SECRET-KEY-1QQQQ" > "$T/etc/bio-empfaenger"
rc=0; PATH="$T/fake:$PATH" bash "$T/skript.sh" > "$T/out2" 2>/dev/null || rc=$?
soll "privater Schluessel als Empfaenger -> Abbruch" '[ "$rc" != 0 ] && [ ! -s "$T/out2" ]'
rm -f "$T/etc/bio-empfaenger"
rc=0; PATH="$T/fake:$PATH" bash "$T/skript.sh" > "$T/out3" 2>/dev/null || rc=$?
soll "kein Empfaenger -> Abbruch" '[ "$rc" != 0 ] && [ ! -s "$T/out3" ]'

[ "$fehler" = 0 ] && echo "test_schluessel_sichern: bestanden" || exit 1
