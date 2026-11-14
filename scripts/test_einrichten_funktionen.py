#!/usr/bin/env python3
"""Jede Funktion, die ein Einrichtungsskript aufruft, ist darin definiert.

Bis zum 07.11.2026 rief deploy/validator/einrichten.sh `fehlt` und `setze`
auf, ohne sie zu definieren. `if ! fehlt ...` galt damit als "schon gebunden",
und die Bindung an den Menschen wurde still uebersprungen. bash -n findet so
etwas nicht. Hier: der erste Befehl jedes Teilbefehls muss eine im Skript
definierte Funktion, ein Bash-Befehl oder ein bekanntes Programm sein.
Unbekanntes ist ein Fehler -- dann entweder definieren oder, wenn es ein
echtes Programm ist, in PROGRAMME eintragen.
"""
import re
import sys
from pathlib import Path

SKRIPTE = sorted(Path("deploy").glob("*/einrichten.sh")) + sorted(Path("deploy").glob("*/schluessel-eintragen.sh"))

BASH = set("""
if then else elif fi for while until do done case esac in function select time
! { } [ [[ ]] ] : . source alias bg bind break builtin caller cd command compgen complete
continue declare dirs disown echo enable eval exec exit export false fc fg getopts hash help history
jobs kill let local logout mapfile popd printf pushd pwd read readarray readonly return set shift shopt
suspend test times trap true type typeset ulimit umask unalias unset wait
""".split())

PROGRAMME = set("""
docker curl grep sed awk python3 git openssl mktemp install rm cat seq sleep tr cut head tail
nproc df sort uniq wc chmod chown mkdir cp mv ln ip ss hostname date id whoami sha256sum basename uname
dirname tee xargs env sudo systemctl apt-get timeout stat find touch ufw ssh-keygen jq base64 od
ls paste
""".split())


def code_ohne_text(text: str) -> str:
    """Ersetzt Zeichenketten durch '' und Heredocs durch nichts, behaelt aber
    Befehlsersetzungen $( ... ) auch innerhalb doppelter Anfuehrungszeichen --
    dort stehen Befehle (ERG="$(leistung)")."""
    out, i, n = [], 0, len(text)
    stapel = ["code"]  # code | dq | sub
    tiefe = []
    while i < n:
        c = text[i]
        art = stapel[-1]
        if art in ("code", "sub"):
            if c == "#" and (i == 0 or text[i - 1] in " \t\n;"):
                while i < n and text[i] != "\n":
                    i += 1
                continue
            if c == "'":
                j = text.index("'", i + 1)
                out.append("''" + "\n" * text[i:j].count("\n"))
                i = j + 1
                continue
            if c == '"':
                stapel.append("dq")
                out.append('"')
                i += 1
                continue
            if text.startswith("<<", i) and not text.startswith("<<<", i):
                m = re.match(r"<<-?\s*['\"]?([A-Za-z_]+)['\"]?", text[i:])
                if m:
                    zeilenende = text.index("\n", i)
                    out.append(text[i:zeilenende + 1].replace(m.group(0), ""))
                    ende = re.search(r"^\s*" + m.group(1) + r"\s*$", text[zeilenende + 1:], re.M)
                    weiter = zeilenende + 1 + (ende.end() if ende else 0)
                    out.append("\n" * text[zeilenende + 1:weiter].count("\n"))
                    i = weiter
                    continue
            if art == "sub":
                if c == "(":
                    tiefe[-1] += 1
                elif c == ")":
                    tiefe[-1] -= 1
                    if tiefe[-1] == 0:
                        stapel.pop()
                        tiefe.pop()
                        out.append(")")
                        i += 1
                        continue
            out.append(c)
            i += 1
            continue
        # dq
        if c == "\\":
            i += 2
            continue
        if c == "\n":
            out.append("\n")
            i += 1
            continue
        if c == '"':
            stapel.pop()
            out.append('"')
            i += 1
            continue
        if text.startswith("$(", i) and not text.startswith("$((", i):
            stapel.append("sub")
            tiefe.append(1)
            out.append(";(")
            i += 2
            continue
        i += 1
    return "".join(out)


def befehle(text: str):
    code = code_ohne_text(text)
    im_case = 0
    for nr, z in enumerate(code.split("\n"), 1):
        case_zeile = bool(re.search(r"\bcase\b.*\bin\b", z))
        if case_zeile:
            im_case += 1
        war_im_case = im_case > 0
        if re.search(r"\besac\b", z):
            im_case = max(0, im_case - 1)
        z = re.sub(r"\$\(\([^()]*\)\)", "0", z)  # Rechnung $(( ... ))
        if war_im_case:
            # Muster eines case-Zweigs: am Zeilenanfang, nach "in" oder nach ";;"
            z = re.sub(r"(^|\bin\b|;;)\s*[^()\s;][^();]*\)", r"\1 ", z)
        for teil in re.split(r"\|\||&&|;|\||\$\(|`|\(|\)", z):
            w = teil.strip().split()
            while w and (re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*=.*", w[0]) or w[0] in ("!", "then", "do", "else", "{", "}", "if", "while", "until", "time", "command", "exec", "elif")):
                w = w[1:]
            if w:
                yield nr, w[0]


def main() -> int:
    fehler = 0
    for p in SKRIPTE:
        text = p.read_text()
        definiert = set(re.findall(r"^\s*([a-z_][a-z0-9_]*)\(\)\s*\{", text, re.M))
        for nr, w in befehle(text):
            if not re.fullmatch(r"[a-z_][a-z0-9_.-]*", w) or w in definiert or w in BASH or w in PROGRAMME:
                continue
            if w.endswith(".sh") or w.startswith("./") or w in ("bash", "sh", "python"):
                continue
            print(f"{p}:{nr}: '{w}' ist weder definiert noch bekannt")
            fehler += 1
    if fehler:
        return 1
    print(f"ok: {len(SKRIPTE)} Skripte, jede aufgerufene Funktion definiert")
    return 0


if __name__ == "__main__":
    sys.exit(main())
