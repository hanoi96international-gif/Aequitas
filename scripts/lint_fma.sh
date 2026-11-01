#!/usr/bin/env bash
# Kein fusioniertes Multiplizieren-Addieren (FMA) im Ketten-Code.
#
# WARUM
#
# Die Go-Spezifikation erlaubt dem Compiler, x*y + z zu EINER Operation zu
# verschmelzen, die nur einmal rundet ("An implementation may combine multiple
# floating-point operations into a single fused operation, possibly across
# statements"). Auf arm64, riscv64, ppc64le und s390x tut er das, auch ueber
# eingebettete Funktionen hinweg; auf amd64 (GOAMD64=v1, unser Image) nie.
# Ein Validator auf ARM rechnete Umlauf, Grundeinkommen, Vermoegensgrenze oder
# Topfrest dann im letzten Bit anders als C1 -- und seit dem strengen
# Nachrechnen (01.11.2026) hiesse das: Block abgelehnt, Kette gespalten.
#
# Eine ausdrueckliche Umwandlung float64(x*y) rundet das Produkt und verbietet
# die Verschmelzung. Auf amd64 ist sie wirkungslos (dasselbe Ergebnis, Bit fuer
# Bit), auf den anderen Architekturen macht sie das Ergebnis gleich.
#
# WAS GEPRUEFT WIRD
#
# Alle Pakete dieses Moduls unter x/ und cmd/ werden fuer jede Architektur mit
# FMA-Verschmelzung uebersetzt (-gcflags=-S); jede FMA-Anweisung, deren Zeile
# in diesem Repo liegt, ist ein Fehler. tools/ (Messwerkzeuge) ist ausgenommen.
#
#   bash scripts/lint_fma.sh             # pruefen
#   bash scripts/lint_fma.sh --selbsttest
set -euo pipefail

MUSTER='[[:space:]](FMADDD|FMSUBD|FNMADDD|FNMSUBD|FMADDS|FMSUBS|FMADD|FMSUB|FNMADD|FNMSUB|WFMADB|WFMSDB|FMADDD)[[:space:]]'
ARCHS="arm64 riscv64 ppc64le s390x"

pruefe() { # $1 Modulwurzel, $2 Modulpfad, Rest: Paketmuster
  local wurzel=$1 modul=$2; shift 2
  local fehler=0 a aus
  for a in $ARCHS; do
    aus=$(mktemp)
    if ! (cd "$wurzel" && GOARCH=$a CGO_ENABLED=0 go build -gcflags="$modul/...=-S" "$@" 2>"$aus" >/dev/null); then
      # -S schreibt nach stderr; ein Uebersetzungsfehler steht dort auch.
      if grep -qE '^[^[:space:]].*\.go:[0-9]+:[0-9]+: ' "$aus"; then
        echo "::error::Uebersetzung fuer $a gescheitert"; grep -E '\.go:[0-9]+:[0-9]+: ' "$aus" | head -5
        rm -f "$aus"; return 2
      fi
    fi
    local treffer
    treffer=$(grep -E "$MUSTER" "$aus" | grep -oE "\($wurzel/[^)]*\.go:[0-9]+\)" | sort -u || true)
    rm -f "$aus"
    if [ -n "$treffer" ]; then
      echo "::error::FMA auf $a -- Produkt mit float64(...) umschliessen:"
      echo "$treffer" | sed "s|($wurzel/|  |; s|)$||"
      fehler=1
    fi
  done
  return $fehler
}

if [ "${1:-}" = "--selbsttest" ]; then
  tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
  mkdir -p "$tmp/x/geld"
  printf 'module beispiel.test/fma\n\ngo 1.22\n' > "$tmp/go.mod"
  # Angriff: das Muster, das die Kette spaltete (Produkt plus Summand).
  cat > "$tmp/x/geld/geld.go" <<'GO'
package geld

func Umlauf(stand, satz, rest float64) float64 { return stand*satz + rest }
GO
  rc=0; pruefe "$tmp" beispiel.test/fma ./x/... >/dev/null || rc=$?
  # Genau 1: erkannt. 0 waere uebersehen, 2 ein Uebersetzungsfehler.
  [ "$rc" = 1 ] || { echo "Selbsttest: FMA NICHT erkannt (rc=$rc)"; exit 1; }
  # Gutfall: dieselbe Rechnung mit ausdruecklicher Rundung.
  cat > "$tmp/x/geld/geld.go" <<'GO'
package geld

func Umlauf(stand, satz, rest float64) float64 { return float64(stand*satz) + rest }
GO
  if ! pruefe "$tmp" beispiel.test/fma ./x/... >/dev/null; then
    echo "Selbsttest: Gutfall als FMA gemeldet"; exit 1
  fi
  echo "Selbsttest bestanden"
  exit 0
fi

wurzel=$(cd "$(dirname "$0")/.." && pwd)
modul=$(cd "$wurzel" && go list -m)
if pruefe "$wurzel" "$modul" ./x/... ./cmd/...; then
  echo "ok: kein FMA in x/ und cmd/ auf $ARCHS"
else
  exit 1
fi
