# Registrierung-Diagnose (nur lesen). Wird an gemeinsam.sh angehaengt.
# Warum antwortet der Vergleichsdienst dem Coordinator mit 401?
# Ausgegeben werden nur Namen, Zahlen und kurze Fingerabdruecke:
#   - Token: die ersten 8 Hex von sha256(Token), nie das Token,
#   - oeffentliche Coordinator-Schluessel: die ersten 12 Hex (oeffentlich).
# Das Repo und damit diese Logs sind oeffentlich.
set -u
echo "##### $BOX: Registrierung-Diagnose #####"

PY_FP='import hashlib,os,sys
t=os.environ.get("SERVICE_AUTH_TOKEN","").strip()
print("  SERVICE_AUTH_TOKEN:", ("fp="+hashlib.sha256(t.encode()).hexdigest()[:8]) if t else "LEER")'

PY_COORD_PUB='import os
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization
r=os.environ.get("COORDINATOR_SIGNING_KEY","").strip()
if not r: print("  COORDINATOR_SIGNING_KEY: LEER")
else:
  try:
    k=Ed25519PrivateKey.from_private_bytes(bytes.fromhex(r))
    p=k.public_key().public_bytes(encoding=serialization.Encoding.Raw,format=serialization.PublicFormat.Raw).hex()
    print("  eigener oeffentlicher Schluessel:", p[:12])
  except Exception as e: print("  COORDINATOR_SIGNING_KEY: unlesbar", type(e).__name__)
print("  VALIDATOR_URLS:", os.environ.get("VALIDATOR_URLS","(nicht gesetzt)"))
print("  QUORUM_SIZE:", os.environ.get("QUORUM_SIZE","(nicht gesetzt)"))'

PY_MATCH='import os,json,urllib.request
roh=os.environ.get("COORDINATOR_PUBLIC_KEYS","")
env=[s.strip().lower() for s in roh.split(",") if s.strip()]
print("  COORDINATOR_PUBLIC_KEYS:", len(env), "Eintraege:", " ".join(k[:12] for k in env))
print("  REQUIRE_COORDINATOR_AUTH:", os.environ.get("REQUIRE_COORDINATOR_AUTH","(nicht gesetzt)"))
b=os.environ.get("CHAIN_BASE_URL","").rstrip("/")
print("  CHAIN_BASE_URL:", b or "(nicht gesetzt)")
if b:
  try:
    d=json.load(urllib.request.urlopen(b+"/api/coordinators",timeout=5))
    l=d if isinstance(d,list) else (d.get("coordinators") or [])
    ks=[str(x.get("public_key","") if isinstance(x,dict) else x)[:12] for x in l]
    print("  Kettenregister:", len(l), "Eintraege:", " ".join(ks))
  except Exception as e:
    print("  Kettenregister NICHT erreichbar:", type(e).__name__)'

for c in $(docker ps --format '{{.Names}}' | grep -E 'coordinator' || true); do
  echo "--- Coordinator $c ---"
  docker exec "$c" python -c "$PY_FP" 2>&1 | head -3
  docker exec "$c" python -c "$PY_COORD_PUB" 2>&1 | head -5
done

for c in $(docker ps --format '{{.Names}}' | grep -E 'matching' || true); do
  echo "--- Vergleichsdienst $c ---"
  docker exec "$c" python -c "$PY_FP" 2>&1 | head -3
  docker exec "$c" python -c "$PY_MATCH" 2>&1 | head -8
  echo "  Log (Auth, letzte 2h):"
  docker logs --since 2h "$c" 2>&1 | grep -E ' 401|unauthor|coordinator|missing_bearer' | tail -8 | cut -c1-200
done

# Netz: erreicht jeder Dienst den Knoten unter dem Namen aus CHAIN_BASE_URL?
# Nur Netznamen, Container-Namen und Ja/Nein -- keine Adressen von aussen.
echo "--- Netze ---"
docker network ls --format '  {{.Name}} ({{.Driver}})' | grep -vE ' (none|null)' || true
for c in "$NODE" "$MATCH" "$PROOF" "$COORD"; do
  [ -n "$c" ] || continue
  docker inspect "$c" >/dev/null 2>&1 || { echo "  $c: fehlt"; continue; }
  echo "  $c: Modus=$(docker inspect "$c" --format '{{.HostConfig.NetworkMode}}') Netze=$(docker inspect "$c" --format '{{range $n,$_ := .NetworkSettings.Networks}}{{$n}} {{end}}')"
done
echo "  $NODE Zustand: $(docker inspect "$NODE" --format 'status={{.State.Status}} neustarts={{.RestartCount}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}} seit={{.State.StartedAt}} policy={{.HostConfig.RestartPolicy.Name}}' 2>&1 | cut -c1-160)"
echo "  $NODE DNS-Namen: $(docker inspect "$NODE" --format '{{range $n,$e := .NetworkSettings.Networks}}{{$n}}=[{{range $e.Aliases}}{{.}} {{end}}{{range $e.DNSNames}}{{.}} {{end}}] {{end}}' 2>&1 | cut -c1-200)"
echo "  Wirt -> 127.0.0.1:8080/api/status: $(curl -s -o /dev/null -w '%{http_code}' -m 5 http://127.0.0.1:8080/api/status || echo keine-Antwort)"
if ! laeuft "$NODE" || [ "$(docker inspect "$NODE" --format '{{.State.Restarting}}' 2>/dev/null)" = true ]; then
  echo "  $NODE Log (letzte 25, gefiltert):"
  docker logs --tail 25 "$NODE" 2>&1 | grep -viE 'key|secret|token|seed|priv|passw|mnemonic|SAVE THIS' | cut -c1-200 | sed 's/^/    /'
fi
for c in "$MATCH" "$PROOF" "$COORD"; do
  [ -n "$c" ] && laeuft "$c" || continue
  u="$(env_von "$c" CHAIN_BASE_URL)"; [ -n "$u" ] || u="$(env_von "$c" CHAIN_URL)"
  [ -n "$u" ] || { echo "  $c -> (keine CHAIN_BASE_URL)"; continue; }
  if ! docker exec "$c" sh -c 'command -v python' >/dev/null 2>&1; then
    r="$(docker exec -e U="$u" "$c" node -e 'require("http").get(process.env.U.replace(/\/$/,"")+"/api/status",{timeout:5000},r=>{console.log("erreichbar");process.exit(0)}).on("error",e=>{console.log("NICHT erreichbar: "+e.code);process.exit(0)})' 2>&1 | tail -1 | cut -c1-80)"
    echo "  $c -> $u: $r"; continue
  fi
  r="$(docker exec -e U="$u" "$c" python -c 'import os,urllib.request
try:
  urllib.request.urlopen(os.environ["U"].rstrip("/")+"/api/status",timeout=5); print("erreichbar")
except Exception as e: print("NICHT erreichbar:", type(e).__name__)' 2>&1 | tail -1 | cut -c1-80)"
  echo "  $c -> $u: $r"
done
