#!/usr/bin/env bash
# Kontostaende dieses Knotens als sortierte Liste
# "adresse|saldo|tusd|lp|mensch" (Betraege in Mikro-AEQ) auf stdout -- fuer
# den Vergleich zweier Validatoren (konten-vergleich.yml). Nur lesen. Salden
# sind oeffentliche Kettendaten; sonst wird nichts ausgegeben.
set -euo pipefail
DATABASE_URL="$(grep -E '^DATABASE_URL=' /root/.aequitas.env 2>/dev/null | head -1 | cut -d= -f2- || true)"
if [ -z "$DATABASE_URL" ]; then
  DATABASE_URL="$(docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E '^DATABASE_URL=' | head -1 | cut -d= -f2-)"
fi
NET="$(docker inspect aequitas-node --format '{{range $n,$v := .NetworkSettings.Networks}}{{$n}}{{end}}')"
docker run --rm --network "$NET" -e DATABASE_URL="$DATABASE_URL" postgres:16-alpine sh -c 'psql "$DATABASE_URL" -t -A -F "|" -c "
  SELECT lower(address), round(balance::numeric*1000000), round(coalesce(tusd_balance,0)::numeric*1000000),
         round(coalesce(lp_shares,0)::numeric*1000000), is_human
  FROM chain_accounts
  WHERE is_human OR balance <> 0 OR coalesce(tusd_balance,0) <> 0 OR coalesce(lp_shares,0) <> 0
  ORDER BY 1"'
