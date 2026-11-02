#!/usr/bin/env python3
"""Deepfake-Test auswerten: echte Aufnahmen gegen Angriffe (WP 4).

docs/LEBENDIGKEIT_GEGEN_DEEPFAKES.md, Abschnitt "WP 4", und
docs/DEEPFAKE_TEST.md. Der Coordinator legt jede Bewertung ohne Kennung in
/coordinator/health -> lebendigkeit.letzte (hoechstens 50, im Speicher; ein
Neustart leert sie). Dieses Skript liest diese Antwort und beantwortet die
drei Fragen aus WP 4:

  1. Landet JEDER Angriff gelb oder rot? (Schritt 2) -- sonst Exit 1.
  2. Trennen sich echte Aufnahmen und Angriffe im Score L? (Schritt 3)
  3. Welche Schwellen ergaeben sich daraus? gruen_L unter dem 5-%-Quantil der
     echten Aufnahmen, rot_L ueber dem hoechsten L der Angriffe. Vorschlag nur
     bei mindestens MIN_ECHTE echten Aufnahmen und ohne Ueberlappung --
     sonst ausdruecklich: NICHT scharf schalten.

Welche Eintraege Angriffe sind, sagt der Aufrufer: die letzten N
(--angriffe N). Ablauf beim Test: erst die echten Aufnahmen, dann genau N
Angriffe, dann sofort auswerten.

Eingabe: die JSON-Antwort von /coordinator/health auf stdin. Es wird nur
gelesen und gerechnet, nichts gesendet. Die Eintraege tragen keine Kennung
(lebendigkeit.py, merke), die Ausgabe also auch nicht.

  python3 scripts/lebendigkeit_auswerten.py --angriffe 3 < health.json
  python3 scripts/lebendigkeit_auswerten.py --selbsttest
"""
from __future__ import annotations

import argparse
import json
import math
import sys
import time

MIN_ECHTE = 20
ABSTAND = 0.02  # rot_L liegt so weit ueber dem hoechsten Angriff
# flash_relief ist nur gemessen (Coordinator NUR_GEMESSEN), zeigt hier aber,
# ob es trennt -- genau dafuer wird es gemessen.
SIGNALE = ("challenge", "flash", "parallax", "pulse", "antispoof", "flash_relief")
# Hoechstens so viele Eintraege werden gelesen (letzte haelt 50).
MAX_EINTRAEGE = 200


def _zahl(x) -> float | None:
    if isinstance(x, bool) or not isinstance(x, (int, float)):
        return None
    x = float(x)
    return x if math.isfinite(x) else None


def eintraege(health: dict, seit: float | None = None) -> list[dict]:
    """Nur wohlgeformte Eintraege, aelteste zuerst. Alles andere wird
    verworfen, nicht geraten."""
    leb = health.get("lebendigkeit") if isinstance(health, dict) else None
    roh = leb.get("letzte") if isinstance(leb, dict) else None
    if not isinstance(roh, list):
        raise ValueError("lebendigkeit.letzte fehlt -- Coordinator zu alt oder Antwort unvollstaendig")
    aus = []
    for e in roh[-MAX_EINTRAEGE:]:
        if not isinstance(e, dict):
            continue
        t = _zahl(e.get("t"))
        if t is None or (seit is not None and t < seit):
            continue
        klasse = e.get("klasse")
        if klasse not in ("gruen", "gelb", "rot", "unbestimmt"):
            continue
        signale = {}
        for name in SIGNALE:
            s = (e.get("signale") or {}).get(name) if isinstance(e.get("signale"), dict) else None
            if isinstance(s, dict) and (_zahl(s.get("geprueft")) or 0) > 0 and _zahl(s.get("wert")) is not None:
                signale[name] = _zahl(s.get("wert"))
        risiken = [str(r)[:40] for r in e.get("risiken", []) if isinstance(r, str)][:10]
        aus.append({"t": t, "zweck": str(e.get("zweck", ""))[:20], "klasse": klasse,
                    "L": _zahl(e.get("score")), "R": _zahl(e.get("risiko")),
                    "signale": signale, "risiken": risiken})
    aus.sort(key=lambda e: e["t"])
    return aus


def _quantil(werte: list[float], p: float) -> float:
    w = sorted(werte)
    return w[min(len(w) - 1, int(p * (len(w) - 1) + 0.5))]


def auswerten(liste: list[dict], angriffe: int) -> dict:
    if angriffe < 0 or angriffe > len(liste):
        raise ValueError(f"--angriffe {angriffe}, aber nur {len(liste)} Eintraege")
    echte = liste[:len(liste) - angriffe]
    boese = liste[len(liste) - angriffe:]
    durchgelassen = [e for e in boese if e["klasse"] == "gruen"]
    ohne_score = [e for e in boese if e["L"] is None]
    echte_L = [e["L"] for e in echte if e["L"] is not None]
    boese_L = [e["L"] for e in boese if e["L"] is not None]

    ergebnis = {
        "echte": len(echte), "angriffe": len(boese),
        "angriffe_gruen": len(durchgelassen),
        "angriffe_ohne_score": len(ohne_score),
        "echte_klassen": _zaehle(e["klasse"] for e in echte),
        "angriff_klassen": _zaehle(e["klasse"] for e in boese),
        "signale": {},
        "vorschlag": None,
        "urteil": [],
    }
    for name in SIGNALE:
        ew = [e["signale"][name] for e in echte if name in e["signale"]]
        bw = [e["signale"][name] for e in boese if name in e["signale"]]
        ergebnis["signale"][name] = {
            "echte_n": len(ew), "echte_min": min(ew) if ew else None,
            "angriffe_n": len(bw), "angriffe_max": max(bw) if bw else None,
            "trennt": bool(ew and bw and min(ew) > max(bw)),
        }

    u = ergebnis["urteil"]
    if durchgelassen:
        u.append(f"ROT: {len(durchgelassen)} von {len(boese)} Angriffen gruen -- dieser Angriff kaeme "
                 "kalibriert ohne Staffel durch. Nicht scharf schalten; schwaches Signal verbessern.")
    elif boese:
        u.append(f"OK: alle {len(boese)} Angriffe gelb, rot oder unbestimmt.")
    if ohne_score:
        u.append(f"HINWEIS: {len(ohne_score)} Angriff(e) ohne Score (kein Signal geprueft) -- "
                 "kalibriert hiesse das 'unbestimmt' = sofort. Pruefen, ob die App die Aufnahme ueberhaupt sandte.")
    if len(echte_L) < MIN_ECHTE:
        u.append(f"ZU WENIG: {len(echte_L)} echte Aufnahmen mit Score, verlangt sind {MIN_ECHTE}. "
                 "Kein Schwellenvorschlag.")
    elif not boese_L:
        u.append("KEINE ANGRIFFE mit Score: rot_L laesst sich nicht ablesen. Kein Schwellenvorschlag.")
    else:
        gruen_L = round(_quantil(echte_L, 0.05) - 0.005, 3)
        rot_L = round(max(boese_L) + ABSTAND, 3)
        if rot_L >= gruen_L:
            u.append(f"UEBERLAPPUNG: hoechster Angriff L={max(boese_L):.3f}, 5-%-Quantil echt "
                     f"L={_quantil(echte_L, 0.05):.3f}. NICHT scharf schalten.")
        else:
            ergebnis["vorschlag"] = {"gruen_L": gruen_L, "rot_L": rot_L}
            u.append(f"VORSCHLAG: SCHWELLE_GRUEN_L={gruen_L}, SCHWELLE_ROT_L={rot_L} "
                     "(coordinator/app/lebendigkeit.py), danach LEBENDIGKEIT_KALIBRIERT=true.")
    return ergebnis


def _zaehle(it) -> dict:
    d: dict = {}
    for x in it:
        d[x] = d.get(x, 0) + 1
    return d


def bericht(liste: list[dict], angriffe: int, ergebnis: dict) -> str:
    zeilen = ["Nr  Zeit (UTC)           Art      Zweck        Klasse      L      R      Signale / Risiken"]
    grenze = len(liste) - angriffe
    for i, e in enumerate(liste):
        art = "ANGRIFF" if i >= grenze else "echt"
        zeit = time.strftime("%Y-%m-%d %H:%M:%S", time.gmtime(e["t"]))
        L = "  -  " if e["L"] is None else f"{e['L']:.3f}"
        R = "  -  " if e["R"] is None else f"{e['R']:.3f}"
        sig = " ".join(f"{k}={v:.2f}" for k, v in e["signale"].items())
        ris = (" | " + ",".join(e["risiken"])) if e["risiken"] else ""
        zeilen.append(f"{i + 1:>2}  {zeit}  {art:<8} {e['zweck']:<12} {e['klasse']:<10} {L}  {R}  {sig}{ris}")
    zeilen.append("")
    zeilen.append(f"echt: {ergebnis['echte']} {ergebnis['echte_klassen']}   "
                  f"Angriffe: {ergebnis['angriffe']} {ergebnis['angriff_klassen']}")
    zeilen.append("Signal      echt n / min   Angriff n / max   trennt")
    for name, s in ergebnis["signale"].items():
        em = "-" if s["echte_min"] is None else f"{s['echte_min']:.2f}"
        bm = "-" if s["angriffe_max"] is None else f"{s['angriffe_max']:.2f}"
        zeilen.append(f"{name:<11} {s['echte_n']:>3} / {em:<6}   {s['angriffe_n']:>3} / {bm:<6}      "
                      f"{'ja' if s['trennt'] else 'nein'}")
    zeilen.append("")
    zeilen.extend(ergebnis["urteil"])
    return "\n".join(zeilen)


def _e(t, klasse, L, **signale):
    return {"t": t, "zweck": "register", "klasse": klasse, "score": L, "risiko": 0.0,
            "signale": {k: {"geprueft": 1, "wert": v} for k, v in signale.items()}, "risiken": [], "verbindlich": False}


def selbsttest() -> int:
    fehler = 0

    def soll(name, bedingung):
        nonlocal fehler
        if not bedingung:
            fehler += 1
            print(f"FEHLER: {name}")

    echt = [_e(1000 + i, "gruen", 0.85 + (i % 5) * 0.03, challenge=1.0, flash=0.9) for i in range(25)]
    # Angriff: Bildschirm vor der Kamera, Blitz scheitert.
    angriff = [_e(2000 + i, "gelb", 0.55, challenge=1.0, flash=0.0) for i in range(3)]
    r = auswerten(eintraege({"lebendigkeit": {"letzte": echt + angriff}}), 3)
    soll("Trennung gibt Vorschlag", r["vorschlag"] is not None and r["vorschlag"]["rot_L"] < r["vorschlag"]["gruen_L"])
    soll("Blitz trennt", r["signale"]["flash"]["trennt"])
    soll("Aufgabe trennt nicht", not r["signale"]["challenge"]["trennt"])
    soll("kein Angriff gruen", r["angriffe_gruen"] == 0)

    # Missbrauch 1: ein Angriff landet gruen -> Exit-relevanter Befund.
    durch = angriff[:2] + [_e(2003, "gruen", 0.9, challenge=1.0, flash=0.9)]
    r = auswerten(eintraege({"lebendigkeit": {"letzte": echt + durch}}), 3)
    soll("gruener Angriff gemeldet", r["angriffe_gruen"] == 1 and r["urteil"][0].startswith("ROT"))
    soll("Ueberlappung -> kein Vorschlag", r["vorschlag"] is None)

    # Missbrauch 2: zu wenig echte Aufnahmen -> nie ein Vorschlag.
    r = auswerten(eintraege({"lebendigkeit": {"letzte": echt[:5] + angriff}}), 3)
    soll("zu wenig echte", r["vorschlag"] is None and any(x.startswith("ZU WENIG") for x in r["urteil"]))

    # Missbrauch 3: kaputte und feindliche Eintraege werden verworfen, nicht geraten.
    kaputt = [None, "x", {"t": "jetzt", "klasse": "gruen"}, {"t": 5, "klasse": "lila"},
              _e(3000, "gruen", float("nan")), _e(3001, "gruen", True)]
    liste = eintraege({"lebendigkeit": {"letzte": kaputt}})
    soll("NaN/bool kein Score", all(e["L"] is None for e in liste) and len(liste) == 2)

    # Missbrauch 4: Antwort ohne letzte -> Fehler statt leerem "OK".
    try:
        eintraege({"lebendigkeit": {}})
        soll("fehlende letzte wirft", False)
    except ValueError:
        pass

    # Missbrauch 5: mehr Angriffe angegeben als Eintraege.
    try:
        auswerten(eintraege({"lebendigkeit": {"letzte": angriff}}), 5)
        soll("zu viele Angriffe wirft", False)
    except ValueError:
        pass

    # Angriff ohne Score (nichts geprueft) wird ausdruecklich genannt.
    r = auswerten(eintraege({"lebendigkeit": {"letzte": echt + [_e(4000, "unbestimmt", None)]}}), 1)
    soll("ohne Score genannt", r["angriffe_ohne_score"] == 1)

    # seit filtert alte Eintraege.
    soll("seit filtert", len(eintraege({"lebendigkeit": {"letzte": echt + angriff}}, seit=2000)) == 3)

    print("Selbsttest", "bestanden" if not fehler else f"mit {fehler} Fehler(n)")
    return 1 if fehler else 0


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--angriffe", type=int, default=0, help="die letzten N Eintraege sind Angriffe")
    p.add_argument("--seit", type=float, default=None, help="nur Eintraege ab dieser Unix-Zeit")
    p.add_argument("--selbsttest", action="store_true")
    a = p.parse_args()
    if a.selbsttest:
        return selbsttest()
    roh = sys.stdin.read(4 * 1024 * 1024 + 1)
    if len(roh) > 4 * 1024 * 1024:
        print("Antwort groesser als 4 MiB -- abgebrochen", file=sys.stderr)
        return 2
    try:
        liste = eintraege(json.loads(roh), a.seit)
        ergebnis = auswerten(liste, a.angriffe)
    except (ValueError, TypeError) as exc:
        print(f"Nicht auswertbar: {exc}", file=sys.stderr)
        return 2
    print(bericht(liste, a.angriffe, ergebnis))
    return 1 if ergebnis["angriffe_gruen"] else 0


if __name__ == "__main__":
    sys.exit(main())
