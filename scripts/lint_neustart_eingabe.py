#!/usr/bin/env python3
"""Die Neustart-Skripte (.github/neustart/*.sh) laufen als ssh ... bash -s:
das Skript selbst ist die Standardeingabe. Ein "docker exec -i" ohne eigene
Eingabe liest den Rest des Skripts weg (21.10.2026: leeren.sh endete gruen,
ohne zu loeschen). Jede Zeile mit "docker exec -i" braucht daher eine eigene
Eingabe: ein Heredoc (<<), eine Umleitung (<) oder eine Pipe davor (|).
fern.sh faengt ein verschlucktes Skript zur Laufzeit ab; dies hier schon
vor dem Merge."""
import pathlib
import re
import sys

WURZEL = pathlib.Path(__file__).resolve().parent.parent
MUSTER = re.compile(r"docker\s+exec\s+(?:-\w+\s+)*-\w*i\w*\b")


def befunde(text):
    for nr, zeile in enumerate(text.splitlines(), 1):
        code = zeile.split("#", 1)[0] if zeile.lstrip().startswith("#") else zeile
        m = MUSTER.search(code)
        if not m:
            continue
        vorher, nachher = code[: m.start()], code[m.end():]
        if "|" in vorher or "<" in nachher:
            continue
        yield nr, zeile.strip()


def main():
    fehler = 0
    for p in sorted((WURZEL / ".github" / "neustart").glob("*.sh")):
        for nr, zeile in befunde(p.read_text()):
            print(f"{p.relative_to(WURZEL)}:{nr}: docker exec -i ohne eigene Eingabe "
                  f"(verschluckt den Rest des Skripts): {zeile}")
            fehler = 1
    # Selbsttest: der Fehler vom 21.10.2026 wird erkannt, die erlaubten Formen nicht.
    falsch = '  docker exec -i "$PROOF" node -e \'console.log(1)\' || echo "?"'
    richtig = [
        'docker exec -i -e P="$X" "$M" python3 - <<\'PY\'',
        'A="$(printf x | docker exec -i "$P" node -e "")"',
        'docker exec -i "$P" sh </dev/null',
        'docker exec "$P" node -e "1"',
    ]
    if not list(befunde(falsch)) or any(list(befunde(r)) for r in richtig):
        print("Selbsttest der Pruefung gescheitert")
        fehler = 1
    if not fehler:
        print("Neustart-Skripte: jedes docker exec -i hat eine eigene Eingabe")
    return fehler


if __name__ == "__main__":
    sys.exit(main())
